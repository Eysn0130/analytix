package contextepoch

import (
	"context"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrepareTurnBootstrapsAndReusesOneSecurityEpoch(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	bindingA := domainsecurity.SHA256Hex([]byte("binding-a"))
	manifestA := domainsecurity.SHA256Hex([]byte("manifest-a"))
	context := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: bindingA, DatasetSnapshotID: "snapshot-a", SourceManifestHash: manifestA, ContextEpoch: 3, IssuedAt: now,
	})
	prepared, err := PrepareTurn(PrepareTurnInput{Thread: map[string]any{}, SecurityContext: context, At: now})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Initialized || prepared.SecurityContext.ContextEpoch != 3 || prepared.State.AcceptedSnapshot.Epoch != 3 {
		t.Fatalf("bootstrap must reuse the security epoch: %#v", prepared)
	}
	if prepared.SecurityContext.IssuedAt != context.IssuedAt || prepared.SecurityContext.ContextDigest != context.ContextDigest {
		t.Fatalf("epoch bookkeeping must not reissue an unchanged turn security context: before=%#v after=%#v", context, prepared.SecurityContext)
	}
	turn := map[string]any{}
	event := map[string]any{}
	patch := map[string]any{}
	AttachStartRecords(turn, event, patch, prepared.State)
	thread := map[string]any{"contextEpochState": patch["contextEpochState"]}
	nextContext := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-2", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: bindingA, DatasetSnapshotID: "snapshot-a", SourceManifestHash: manifestA, ContextEpoch: 3, IssuedAt: now.Add(time.Second),
	})
	next, err := PrepareTurn(PrepareTurnInput{Thread: thread, SecurityContext: nextContext, At: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if next.Changed || next.SecurityContext.ContextEpoch != 3 || next.State.AcceptedSnapshot.Epoch != 3 {
		t.Fatalf("unchanged binding must not bump the shared epoch: %#v", next)
	}
	if turn["contextEpochSnapshot"] == nil || event["contextEpochSnapshot"] == nil || patch["contextEpochState"] == nil {
		t.Fatalf("context epoch records were not attached: turn=%#v event=%#v patch=%#v", turn, event, patch)
	}
}

func TestPrepareTurnBindingChangeBumpsSharedEpoch(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	bindingA := domainsecurity.SHA256Hex([]byte("binding-a"))
	bindingB := domainsecurity.SHA256Hex([]byte("binding-b"))
	manifestA := domainsecurity.SHA256Hex([]byte("manifest-a"))
	firstContext := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: bindingA, SourceManifestHash: manifestA, ContextEpoch: 1, IssuedAt: now,
	})
	first, err := PrepareTurn(PrepareTurnInput{Thread: map[string]any{}, SecurityContext: firstContext, At: now})
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{"contextEpochState": PublicState(first.State)}
	changedContext := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-2", WorkspaceRealPath: "/workspace", CaseID: "case-b",
		CaseBindingHash: bindingB, SourceManifestHash: manifestA, ContextEpoch: 1, IssuedAt: now.Add(time.Second),
	})
	changed, err := PrepareTurn(PrepareTurnInput{Thread: thread, SecurityContext: changedContext, At: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if !changed.Changed || changed.State.AcceptedSnapshot.Epoch != 2 || changed.SecurityContext.ContextEpoch != 2 {
		t.Fatalf("binding change must bump the one shared epoch: %#v", changed)
	}
}

func TestPrepareAndAttachStartRecordsFinalizesEpochWithoutProbingSource(t *testing.T) {
	now := time.Date(2026, 7, 11, 3, 0, 0, 0, time.UTC)
	manifest := domainsecurity.SHA256Hex([]byte("manifest"))
	binding := domainsecurity.SHA256Hex([]byte("binding-a"))
	firstContext := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: binding, DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: manifest, ContextEpoch: 1, IssuedAt: now,
	})
	first, err := PrepareTurn(PrepareTurnInput{Thread: map[string]any{}, SecurityContext: firstContext, At: now})
	if err != nil {
		t.Fatal(err)
	}
	changedContext := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-2", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: binding, DatasetSnapshotID: "snapshot-b",
		SourceManifestHash: manifest, ContextEpoch: 1, IssuedAt: now.Add(time.Second),
	})
	turn, event, patch := map[string]any{}, map[string]any{}, map[string]any{}
	prepared, err := PrepareAndAttachStartRecords(context.Background(), PrepareStartRecordsInput{
		Thread: map[string]any{"contextEpochState": PublicState(first.State)}, SecurityContext: changedContext,
		At: now.Add(time.Second), Turn: turn, TurnStartedEvent: event, ThreadPatch: patch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.SecurityContext.ContextEpoch != 2 {
		t.Fatalf("snapshot change did not finalize the shared epoch: prepared=%#v", prepared)
	}
	for name, raw := range map[string]any{"turn": turn["securityContext"], "event": event["securityContext"], "thread": patch["securityState"]} {
		attached, err := domainsecurity.ParseTurnSecurityContext(raw)
		if err != nil || attached != prepared.SecurityContext {
			t.Fatalf("%s did not persist the final turn security context: attached=%#v err=%v", name, attached, err)
		}
	}
}

func TestPrepareTurnMigratesPreviousSecurityBindingBeforeCurrentReconcile(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	manifest := domainsecurity.SHA256Hex([]byte("manifest"))
	previous := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), SourceManifestHash: manifest, ContextEpoch: 5, IssuedAt: now,
	})
	current := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-2", WorkspaceRealPath: "/workspace", CaseID: "case-b",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")), SourceManifestHash: manifest, ContextEpoch: 5, IssuedAt: now.Add(time.Second),
	})
	prepared, err := PrepareTurn(PrepareTurnInput{
		Thread: map[string]any{"securityState": turnSecurityRecordForTest(previous)}, SecurityContext: current, At: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Initialized || !prepared.Changed || prepared.State.AcceptedSnapshot.Epoch != 6 || prepared.SecurityContext.ContextEpoch != 6 {
		t.Fatalf("legacy security state must be the baseline before a current binding change: %#v", prepared)
	}
}

func TestPrepareTurnRejectsIndependentSecurityEpoch(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	context := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", IssuedAt: now,
	})
	first, err := PrepareTurn(PrepareTurnInput{Thread: map[string]any{}, SecurityContext: context, At: now})
	if err != nil {
		t.Fatal(err)
	}
	forged := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-2", WorkspaceRealPath: "/workspace", ContextEpoch: first.State.AcceptedSnapshot.Epoch + 4, IssuedAt: now.Add(time.Second),
	})
	if _, err := PrepareTurn(PrepareTurnInput{Thread: map[string]any{"contextEpochState": PublicState(first.State)}, SecurityContext: forged, At: now.Add(time.Second)}); err == nil {
		t.Fatal("turn security must not introduce an independent epoch counter")
	}
}

func turnSecurityRecordForTest(context domainsecurity.TurnSecurityContext) map[string]any {
	return map[string]any{
		"version": context.Version, "threadId": context.ThreadID, "turnId": context.TurnID,
		"workspaceRealPath": context.WorkspaceRealPath, "tenantId": context.TenantID, "userId": context.UserID,
		"caseId": context.CaseID, "caseBindingHash": context.CaseBindingHash, "datasetSnapshotId": context.DatasetSnapshotID,
		"sourceManifestHash": context.SourceManifestHash, "contextEpoch": context.ContextEpoch, "issuedAt": context.IssuedAt,
		"contextDigest": context.ContextDigest, "publicationPolicy": context.PublicationPolicy,
		"riskAuthorityBinding": context.RiskAuthorityBinding,
	}
}
