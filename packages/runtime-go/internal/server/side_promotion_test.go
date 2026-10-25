package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finaladapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func finishSidePromotionTurnV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall) {
	t.Helper()
	finish := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: h.store, SecurityContext: pending.SecurityContext, ThreadID: pending.ThreadID,
		TurnID: pending.TurnID, Model: pending.Model, CreatedAt: finish, FinishedAt: finish,
		Telemetry: usageapp.NewTerminalTelemetryV1(domainmodel.Usage{}, nil),
	})
	if err != nil || !result.Changed {
		t.Fatal("ordinary promotion fixture did not settle its actual own turn", err)
	}
}

func ordinarySidePromotionFixtureV1(t *testing.T) (*runtimeServerHandler, runtimePendingToolCall, runtimePendingToolCall) {
	t.Helper()
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"})
	if err := os.WriteFile(filepath.Join(parent.Workspace, "code.go"), []byte("parent immutable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	finishSidePromotionTurnV1(t, h, parent)
	side, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "side"})
	if err != nil {
		t.Fatal(err)
	}
	own := protectedSeamPendingV1(t, h, side, "read", map[string]any{"path": "code.go"}, "turn_side_promotion")
	if err := os.WriteFile(filepath.Join(own.Workspace, "code.go"), []byte("own immutable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), own, nil, nil); err != nil {
		t.Fatal(err)
	}
	finishSidePromotionTurnV1(t, h, own)
	return h, own, parent
}

func observedSidePromotionServiceV1(h *runtimeServerHandler, repository threadapp.Repository, observe threadapp.ObservePromotionChildrenFunc) *threadapp.Service {
	return threadapp.NewService(threadapp.Dependencies{
		Repository: repository, PublicProjector: h.publicProjector, CaseThreads: h.caseThreads,
		WorkspaceReader: filestore.CaseBindingReader{}, WorkspaceSecurity: h.turnSecurity,
		BeginArchiveRead: h.beginRuntimeThreadArchiveRead, ObservePromotionChildren: observe,
	})
}

func TestOwnSidePromotionHTTPPreservesLineagePrivateHistoryAndRestart(t *testing.T) {
	h, own, parent := ordinarySidePromotionFixtureV1(t)
	before, _, err := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	promoted := requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+own.ThreadID,
		strings.NewReader(`{"promoteSide":true}`), http.StatusOK)
	if stringField(promoted, "id") != own.ThreadID || stringField(promoted, "relation") != "fork" ||
		stringField(promoted, "parentThreadId") != parent.ThreadID || stringField(promoted, "forkedFromThreadId") != parent.ThreadID {
		t.Fatal("promotion did not retain exact identity and lineage")
	}
	after, _, err := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	expected := contracts.CloneMap(before)
	expected["relation"] = "fork"
	delete(expected, "updatedAt")
	actual := contracts.CloneMap(after)
	delete(actual, "updatedAt")
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("promotion rewrote history, frozen authority or source metadata")
	}
	listed := requestArchiveIntentJSON(t, server.URL, http.MethodGet, "/v1/threads?limit=0", nil, http.StatusOK)
	visible := false
	for _, raw := range listAny(listed["threads"]) {
		entry, _ := raw.(map[string]any)
		if stringField(entry, "id") == own.ThreadID && stringField(entry, "relation") == "fork" {
			visible = true
		}
	}
	if !visible {
		t.Fatal("formal promotion was absent from the normal main list")
	}
	for _, pending := range []runtimePendingToolCall{own, parent} {
		if status, _, _ := readProtectedSeamV1(t, h, pending); status != http.StatusOK {
			t.Fatal("promotion lost own or parent immutable private history", status)
		}
	}
	inherited := parent
	inherited.ThreadID = own.ThreadID
	if status, _, _ := readProtectedSeamV1(t, h, inherited); status != http.StatusNotFound {
		t.Fatal("promotion granted private authority to inherited parent bytes")
	}
	reopened, err := NewTempDurableEventSessionStore(h.store.root)
	if err != nil {
		t.Fatal(err)
	}
	primaryReader, err := finaladapter.NewAcceptedFinalCASReader(reopened.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.BindPrimaryThreadReaderV1(primaryReader); err != nil {
		t.Fatal(err)
	}
	reopened.SetCaseThreadAuthority(h.caseThreads)
	h.store = reopened
	h.threads = nil
	persisted, err := reopened.GetThread(own.ThreadID)
	if err != nil || stringField(persisted, "relation") != "fork" || !reflect.DeepEqual(persisted["turns"], after["turns"]) {
		t.Fatal("promotion or original history did not survive a fresh durable reader", err)
	}
	if status, display, _ := readProtectedSeamV1(t, h, own); status != http.StatusOK || display.Capture.Body != "own immutable\n" {
		t.Fatal("reopened promoted Side lost its original protected result", status)
	}
}

func TestSidePromotionRejectsMalformedIntentAndPublicRelationBypass(t *testing.T) {
	h, own, _ := ordinarySidePromotionFixtureV1(t)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	_, before, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	for _, body := range []string{
		`{"promoteSide":false}`, `{"promoteSide":null}`, `{"promoteSide":"true"}`,
		`{"promoteSide":true,"title":"changed"}`, `{"promoteSide":true,"unknown":true}`,
		`{"relation":"primary"}`, `{"relation":"fork"}`, `{"parentThreadId":null}`,
	} {
		requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+own.ThreadID, strings.NewReader(body), http.StatusBadRequest)
	}
	_, after, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if before != after {
		t.Fatal("rejected relation bypass changed the original Side")
	}
}

func TestSidePromotionRejectsBusyAndEmptyOwnOriginWithoutCancelling(t *testing.T) {
	h, own, parent := ordinarySidePromotionFixtureV1(t)
	cancelled := false
	control := h.runtimeControl()
	if err := control.RegisterTurnCancelWithError(own.ThreadID, "promotion-busy", func() { cancelled = true }); err != nil {
		t.Fatal(err)
	}
	_, before, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if _, err := h.runtimeThreadService().Patch(context.Background(), own.ThreadID, map[string]any{"promoteSide": true}); !errors.Is(err, threadapp.ErrThreadMutationTransition) {
		t.Fatal("promotion crossed a busy foreground owner", err)
	}
	control.UnregisterTurnCancel(own.ThreadID, "promotion-busy")
	_, after, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if cancelled || before != after {
		t.Fatal("promotion cancelled or rewrote busy work")
	}
	empty, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "side"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.runtimeThreadService().Patch(context.Background(), stringField(empty, "id"), map[string]any{"promoteSide": true}); !errors.Is(err, threadapp.ErrSidePromotionUnavailable) {
		t.Fatal("inherited parent authority or generated metadata scope became own origin", err)
	}
}

func TestSidePromotionRejectsMissingLegacyCorruptAndIncompleteObservations(t *testing.T) {
	h, own, _ := ordinarySidePromotionFixtureV1(t)
	_, before, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	for _, mode := range []string{"missing", "legacy", "corrupt", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			var observe threadapp.ObservePromotionChildrenFunc
			if mode != "missing" {
				observe = func(context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error) {
					switch mode {
					case "legacy":
						return pendingworkapp.TrustedInventoryV1{}, nil, true, nil
					case "corrupt":
						return pendingworkapp.TrustedInventoryV1{}, nil, false, errors.New("synthetic corrupt inventory")
					default:
						return pendingworkapp.TrustedInventoryV1{}, []domainjob.Record{{ID: "job-99", Kind: "subagent", ChildThreadID: own.ThreadID, ChildTurnID: own.TurnID}}, false, nil
					}
				}
			}
			if _, err := observedSidePromotionServiceV1(h, h.store, observe).Patch(context.Background(), own.ThreadID, map[string]any{"promoteSide": true}); !errors.Is(err, threadapp.ErrSidePromotionUnavailable) {
				t.Fatal("promotion accepted incomplete child authority", err)
			}
			_, after, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
			if after != before {
				t.Fatal("failed authority changed durable Side")
			}
		})
	}
}

type sidePromotionConflictRepositoryV1 struct {
	threadapp.Repository
	patches int
}

type sidePromotionFailingRepositoryV1 struct {
	threadapp.Repository
}

func (r sidePromotionFailingRepositoryV1) PatchThreadIfBaseline(string, map[string]any, string) (map[string]any, error) {
	return nil, errors.New("PRIVATE_SYNTHETIC_PROFILE_PATH: durable write unavailable")
}

func TestSidePromotionWriteFailureHasFixedPublicConflictWithoutPrivateReason(t *testing.T) {
	h, own, _ := ordinarySidePromotionFixtureV1(t)
	h.threads = observedSidePromotionServiceV1(h, sidePromotionFailingRepositoryV1{Repository: h.store},
		func(ctx context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error) {
			inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
			return inventory, nil, false, err
		})
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	_, before, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	response := requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+own.ThreadID,
		strings.NewReader(`{"promoteSide":true}`), http.StatusConflict)
	if len(response) != 2 || response["code"] != "conflict" ||
		response["message"] != "The request conflicts with the current runtime state." {
		t.Fatal("private durable error entered public promotion response")
	}
	_, after, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if before != after {
		t.Fatal("failed write changed original Side")
	}
}

func (r *sidePromotionConflictRepositoryV1) PatchThreadIfBaseline(id string, patch map[string]any, digest string) (map[string]any, error) {
	r.patches++
	if r.patches == 1 {
		return nil, threadapp.ErrThreadMutationBaselineConflict
	}
	return r.Repository.PatchThreadIfBaseline(id, patch, digest)
}

func TestSidePromotionReobservesChildRoleAfterCASConflict(t *testing.T) {
	h, own, _ := ordinarySidePromotionFixtureV1(t)
	reads := 0
	observe := func(ctx context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error) {
		reads++
		inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
		return inventory, nil, reads > 1, err
	}
	repository := &sidePromotionConflictRepositoryV1{Repository: h.store}
	_, before, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if _, err := observedSidePromotionServiceV1(h, repository, observe).Patch(context.Background(), own.ThreadID, map[string]any{"promoteSide": true}); !errors.Is(err, threadapp.ErrSidePromotionUnavailable) {
		t.Fatal("CAS retry reused an obsolete role observation", err)
	}
	_, after, _ := h.store.ReadThreadMutationBaseline(own.ThreadID)
	if reads != 2 || repository.patches != 1 || before != after {
		t.Fatal("CAS retry bypassed fresh authority or mutated denied Side")
	}
}

func TestSidePromotionRefusesSignedCaseLineageAndMalformedMarker(t *testing.T) {
	h, parent, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "code.go"}, true)
	if err := os.WriteFile(filepath.Join(parent.Workspace, "code.go"), []byte("case own source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil {
		t.Fatal(err)
	}
	side, err := h.store.ForkThread(parent.ThreadID, map[string]any{"relation": "fork"})
	if err != nil || stringField(side, "activeInheritedHistoryReceipt") == "" {
		t.Fatal("synthetic CASE fork has no actual signed inherited history", err)
	}
	// Formal CASE derivation only admits fork. A mutable Side-looking record
	// cannot replace that existing signed relation or gain promotion authority.
	side["relation"] = "side"
	id := stringField(side, "id")
	for _, mode := range []string{"signed", "marker missing", "marker malformed"} {
		t.Run(mode, func(t *testing.T) {
			candidate := contracts.CloneMap(side)
			if mode == "marker missing" {
				delete(candidate, "activeInheritedHistoryReceipt")
			} else if mode == "marker malformed" {
				candidate["activeInheritedHistoryReceipt"] = ""
			}
			if err := h.store.ReplaceThreadForAuthorityRepair(id, candidate); err != nil {
				t.Fatal(err)
			}
			_, before, _ := h.store.ReadThreadMutationBaseline(id)
			if _, err := h.runtimeThreadService().Patch(context.Background(), id, map[string]any{"promoteSide": true}); err == nil {
				t.Fatal("promotion rewrote or replaced CASE signed lineage")
			}
			_, after, _ := h.store.ReadThreadMutationBaseline(id)
			if before != after {
				t.Fatal("rejected CASE promotion changed original history")
			}
		})
	}
}
