package toolresultsnapshot

import (
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

type localSnapshotFixtureStore struct {
	snapshot domaintoolresult.ProtectedSnapshotV1
	order    *[]string
}

func (s localSnapshotFixtureStore) Put(context.Context, domaintoolresult.ProtectedSnapshotV1) (domaintoolresult.ProtectedSnapshotBindingV1, error) {
	panic("read cannot write")
}
func (s localSnapshotFixtureStore) Read(context.Context, domaintoolresult.ProtectedSnapshotBindingV1) (domaintoolresult.ProtectedSnapshotV1, error) {
	*s.order = append(*s.order, "read")
	return s.snapshot, nil
}

type localSnapshotFixtureAccess struct {
	authorized      snapshotport.AuthorizedV1
	order           *[]string
	checks          int
	revokeAfterRead bool
}

func (a *localSnapshotFixtureAccess) AcquireCurrent(context.Context, snapshotport.SelectorV1) (snapshotport.AuthorizedV1, func(), error) {
	*a.order = append(*a.order, "acquire")
	return a.authorized, func() { *a.order = append(*a.order, "release") }, nil
}
func (a *localSnapshotFixtureAccess) ValidateCurrent(context.Context, snapshotport.AuthorizedV1) error {
	a.checks++
	*a.order = append(*a.order, "validate")
	if a.revokeAfterRead && a.checks == 2 {
		return snapshotport.ErrUnavailable
	}
	return nil
}
func TestProtectedSnapshotReadHoldsFreshAuthorityThroughRelease(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	resultID := domaintoolresult.ToolResultItemIDV1("turn-a", callID)
	snapshot := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: resultID, ToolName: "read", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Capture: domaintoolresult.ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "package main\r\n", Label: "code.go", StartLine: 1, EndLine: 1, TotalLines: 1}}
	snapshot.Capture.ReadTarget = &domaintoolresult.ProtectedReadTargetV1{Path: "/synthetic/workspace/actual.go", StartLine: 1, ContentDigest: strings.Repeat("f", 64)}
	binding, err := domaintoolresult.ProtectedSnapshotBindingForV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	selector := snapshotport.SelectorV1{ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: resultID}
	for _, revoke := range []bool{false, true} {
		order := []string{}
		access := &localSnapshotFixtureAccess{order: &order, revokeAfterRead: revoke, authorized: snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: snapshot.Workspace, ToolName: "read", ContextDigest: snapshot.ContextDigest, ContextEpoch: 1, ExecutionGrantID: snapshot.ExecutionGrantID, CaseBindingHash: snapshot.CaseBindingHash, Binding: binding}}
		service := &Service{Store: localSnapshotFixtureStore{snapshot, &order}, Authority: access}
		display, err := service.Read(context.Background(), selector)
		if revoke {
			if err == nil || display.Capture.Body != "" {
				t.Fatal("fresh revocation released private body")
			}
		} else if err != nil || display.Capture.Body != "package main\r\n" || display.Capture.ReadTarget != nil || display.ReadTarget == nil || display.ReadTarget.Path != snapshot.Capture.ReadTarget.Path || display.ReadTarget.WorkspaceRoot != snapshot.Workspace {
			t.Fatal("legitimate body was overfiltered")
		}
		if !reflect.DeepEqual(order, []string{"acquire", "validate", "read", "validate", "release"}) {
			t.Fatal("lease or fresh check order changed")
		}
	}
}

func TestProtectedWebDisplaySeparatesMainTargetsAndHoldsFreshLeaseV1(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	resultID := domaintoolresult.ToolResultItemIDV1("turn-a", callID)
	status := 404
	truncated := false
	snapshot := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: resultID, ToolName: "web_fetch", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Capture: domaintoolresult.ProtectedCaptureV1{Kind: "web_fetch", Status: "completed", Body: strings.Repeat("body", 17500), Label: "Fetch", Web: &domaintoolresult.ProtectedWebCaptureV1{RequestURL: "https://example.test/request", FinalURL: "https://example.test/final", InputJSON: `{"url":"https://example.test/request"}`, StatusCode: &status, FetchTruncated: &truncated, ResponseComplete: true, ToolIsError: true, StartedAt: "2026-10-08T10:11:12Z"}, WebTargets: &domaintoolresult.ProtectedWebTargetsV1{RequestURL: "https://example.test/request", ResultURL: "https://example.test/final"}}}
	binding, err := domaintoolresult.ProtectedSnapshotBindingForV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	selector := snapshotport.SelectorV1{ThreadID: snapshot.ThreadID, TurnID: snapshot.TurnID, CallID: snapshot.CallID, ResultItemID: snapshot.ResultItemID}
	for _, revoke := range []bool{false, true} {
		order := []string{}
		access := &localSnapshotFixtureAccess{order: &order, revokeAfterRead: revoke, authorized: snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: snapshot.Workspace, ToolName: snapshot.ToolName, ContextDigest: snapshot.ContextDigest, ContextEpoch: snapshot.ContextEpoch, ExecutionGrantID: snapshot.ExecutionGrantID, CaseBindingHash: snapshot.CaseBindingHash, Binding: binding}}
		service := &Service{Store: localSnapshotFixtureStore{snapshot, &order}, Authority: access}
		display, err := service.Read(context.Background(), selector)
		if revoke {
			if err == nil || display.Capture.Body != "" || display.WebTargets != nil {
				t.Fatal("revoked Web read released bytes or effect authority")
			}
		} else if err != nil || display.Capture.Body != snapshot.Capture.Body || display.Capture.WebTargets != nil || display.WebTargets == nil || *display.WebTargets != *snapshot.Capture.WebTargets {
			t.Fatal("Web display lost full body or failed to separate Main-only targets")
		}
		if !reflect.DeepEqual(order, []string{"acquire", "validate", "read", "validate", "release"}) {
			t.Fatal("Web read changed fresh lease checks")
		}
	}
}

func TestProtectedPresentDisplaySeparatesCurrentFileTargetsAndFreshLeaseV1(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	description := "actual description"
	snapshot := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID,
		ResultItemID: domaintoolresult.ToolResultItemIDV1("turn-a", callID), ToolName: "present", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Capture: domaintoolresult.ProtectedCaptureV1{Kind: "present", Status: "completed", Present: &domaintoolresult.ProtectedPresentReceiptV1{Files: []domaintoolresult.ProtectedPresentedFileV1{{Index: 0, Path: "result.pdf", Description: &description}}}, PresentTargets: []domaintoolresult.ProtectedPresentTargetV1{{Index: 0, Path: "/synthetic/workspace/result.pdf"}}},
		Inspect: &domaintoolresult.ProtectedInspectV1{InputJSON: `{"files":[{"path":"result.pdf","description":"actual description"}]}`}}
	binding, err := domaintoolresult.ProtectedSnapshotBindingForV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	selector := snapshotport.SelectorV1{ThreadID: snapshot.ThreadID, TurnID: snapshot.TurnID, CallID: callID, ResultItemID: snapshot.ResultItemID}
	for _, mode := range []string{"authorized", "revoked_after_read", "canceled_before_read", "canceled_during_read"} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		revoke := mode == "revoked_after_read"
		order := []string{}
		access := &localSnapshotFixtureAccess{order: &order, revokeAfterRead: revoke, authorized: snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: snapshot.Workspace, ToolName: "present", ContextDigest: snapshot.ContextDigest, ContextEpoch: 1, ExecutionGrantID: snapshot.ExecutionGrantID, CaseBindingHash: snapshot.CaseBindingHash, Binding: binding}}
		var store snapshotport.Store = localSnapshotFixtureStore{snapshot, &order}
		if mode == "canceled_before_read" {
			cancel()
		} else if mode == "canceled_during_read" {
			store = presentCancelDuringReadStoreV1{localSnapshotFixtureStore{snapshot, &order}, cancel}
		}
		service := &Service{Store: store, Authority: access}
		display, err := service.Read(ctx, selector)
		if mode != "authorized" {
			if err == nil || display.Capture.Present != nil || display.PresentTargets != nil {
				t.Fatal("revoked authority released declarations or targets")
			}
		} else if err != nil || display.Capture.Present == nil || display.Capture.PresentTargets != nil || display.PresentTargets == nil || display.PresentTargets.WorkspaceRoot != snapshot.Workspace || !reflect.DeepEqual(display.PresentTargets.Files, snapshot.Capture.PresentTargets) {
			t.Fatal("authorized declaration/target separation failed")
		}
		if mode == "canceled_before_read" {
			if len(order) != 0 {
				t.Fatal("canceled read acquired private authority")
			}
		} else if !reflect.DeepEqual(order, []string{"acquire", "validate", "read", "validate", "release"}) {
			t.Fatal("lease/fresh-check order changed")
		}
	}
}

type presentCancelDuringReadStoreV1 struct {
	localSnapshotFixtureStore
	cancel context.CancelFunc
}

func (s presentCancelDuringReadStoreV1) Read(ctx context.Context, binding domaintoolresult.ProtectedSnapshotBindingV1) (domaintoolresult.ProtectedSnapshotV1, error) {
	snapshot, err := s.localSnapshotFixtureStore.Read(ctx, binding)
	s.cancel()
	return snapshot, err
}

type presentHistoryFixtureAccessV1 struct {
	localSnapshotFixtureAccess
	history       snapshotport.PresentHistoryAuthorizedV1
	historyChecks int
	finalRevoke   bool
	cancel        context.CancelFunc
}

func (a *presentHistoryFixtureAccessV1) AcquirePresentHistoryCurrentV1(context.Context, snapshotport.PresentHistorySelectorV1) (snapshotport.PresentHistoryAuthorizedV1, func(), error) {
	*a.order = append(*a.order, "acquire")
	return a.history, func() { *a.order = append(*a.order, "release") }, nil
}
func (a *presentHistoryFixtureAccessV1) ValidatePresentHistoryCurrentV1(context.Context, snapshotport.PresentHistoryAuthorizedV1) error {
	a.historyChecks++
	*a.order = append(*a.order, "validate")
	if a.historyChecks == 2 {
		if a.cancel != nil {
			a.cancel()
		}
		if a.finalRevoke {
			return snapshotport.ErrUnavailable
		}
	}
	return nil
}

func TestProtectedPresentHistoryClosedIdentitiesAndFinalFreshLeaseV1(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	original := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/synthetic/workspace", TenantID: principal.TenantID, UserID: principal.UserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: strings.Repeat("b", 64), DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID,
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: 1, IssuedAt: time.Unix(1, 0).UTC(),
	})
	if err := domainsecurity.ValidateTurnSecurityContext(original); err != nil {
		t.Fatal(err)
	}
	selector := snapshotport.PresentHistorySelectorV1{Kind: snapshotport.PresentHistoryKindV1, ThreadID: original.ThreadID, TurnID: original.TurnID}
	callID := "call_host_" + strings.Repeat("c", 64)
	items := []snapshotport.PresentHistoryItemV1{{Kind: "tool_result", ItemID: domaintoolresult.ToolResultItemIDV1(selector.TurnID, callID), CallID: callID, ToolName: "present"}, {Kind: "assistant_text", ItemID: "item_turn-a_assistant"}}
	for _, mode := range []string{"authorized", "empty", "final_revoked", "final_canceled", "already_canceled", "wrong_selector", "duplicate", "wrong_call", "mixed_assistant", "bad_source_hash", "unknown_kind"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			order := []string{}
			access := &presentHistoryFixtureAccessV1{localSnapshotFixtureAccess: localSnapshotFixtureAccess{order: &order},
				history: snapshotport.PresentHistoryAuthorizedV1{Selector: selector, Principal: principal, Original: original, Workspace: original.WorkspaceRealPath, ThreadFileSHA256: strings.Repeat("d", 64), OrderedItems: append([]snapshotport.PresentHistoryItemV1(nil), items...)}}
			switch mode {
			case "empty":
				access.history.OrderedItems = nil
			case "final_revoked":
				access.finalRevoke = true
			case "final_canceled":
				access.cancel = cancel
			case "already_canceled":
				cancel()
			case "wrong_selector":
				access.history.Selector.TurnID = "turn-other"
			case "duplicate":
				access.history.OrderedItems = append(access.history.OrderedItems, items[0])
			case "wrong_call":
				access.history.OrderedItems[0].CallID = "call_host_" + strings.Repeat("f", 64)
			case "mixed_assistant":
				access.history.OrderedItems[1].CallID = callID
			case "bad_source_hash":
				access.history.ThreadFileSHA256 = "invalid"
			case "unknown_kind":
				access.history.OrderedItems[0].Kind = "raw"
			}
			service := &Service{Authority: access}
			display, err := service.ReadPresentHistoryV1(ctx, selector)
			if mode == "authorized" || mode == "empty" {
				if err != nil || display.SchemaVersion != 1 || display.Kind != snapshotport.PresentHistoryKindV1 || display.ThreadID != selector.ThreadID || display.TurnID != selector.TurnID || display.OrderedItems == nil {
					t.Fatal("valid history lost exact identity or empty array", err)
				}
				if mode == "authorized" && !reflect.DeepEqual(display.OrderedItems, items) {
					t.Fatal("primary order was changed")
				}
				if mode == "empty" && len(display.OrderedItems) != 0 {
					t.Fatal("empty history acquired identities")
				}
			} else if err == nil || display.OrderedItems != nil || display.ThreadID != "" {
				t.Fatal("denied history released a partial identity set")
			}
			if mode == "already_canceled" {
				if len(order) != 0 {
					t.Fatal("canceled history acquired authority")
				}
			} else if mode == "authorized" || mode == "empty" || mode == "final_revoked" || mode == "final_canceled" {
				if !reflect.DeepEqual(order, []string{"acquire", "validate", "validate", "release"}) {
					t.Fatal("history did not retain final fresh lease")
				}
			} else if !reflect.DeepEqual(order, []string{"acquire", "release"}) {
				t.Fatal("invalid authority was used")
			}
		})
	}
}
