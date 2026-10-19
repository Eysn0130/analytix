package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	controlapp "analytix.local/runtime-go/internal/app/control"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	jobs "analytix.local/runtime-go/internal/jobs"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Real HTTP Provider fixtures feed the normal Host Question issuer, request
// and resolution outbox, timer/response, GENERAL terminal bundle, committed
// JSONL reader and Fork HTTP. No preassembled continued item or live provider
// is used. Child restart/refork reads the existing creation seed.
func TestForkEventBoundaryReplaysProductionQuestionAndReforkV1(t *testing.T) {
	for _, mode := range []string{"legacy", "timed"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			resumed := make(chan []byte, 1)
			f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				call := calls.Add(1)
				if call == 1 {
					arguments := `{"prompt":"Optional synthetic question","questions":[{"question":"Choose later?"}]}`
					if mode == "timed" {
						arguments = `{"prompt":"Optional synthetic question","questions":[{"question":"Choose later?"}],"timeout":1}`
					}
					writeSyntheticQuestionCallV1(w, "request_user_input", arguments)
					return
				}
				if call == 2 {
					resumed <- body
				}
				writeInlineCompletionFixtureReply(w)
			})
			h := f.handler
			if mode == "timed" {
				h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
			}
			configureTimedQuestionHistoryAuthority(t, h)
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			t.Cleanup(func() {
				shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				if err := h.Shutdown(shutdown); err != nil {
					t.Error(err)
				}
			})
			started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Ask one optional question, then finish independent ordinary work.", Async: true})
			if err != nil {
				t.Fatal(err)
			}
			turnID := stringField(started, "turnId")
			selector := productionQuestionSelectorV1(t, h, f.request.ThreadID, turnID)
			wantStatus := domaincontinuation.StatusContinued
			if mode == "legacy" {
				// The durable request can be visible before its async producer releases the turn owner.
				if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
					t.Fatal(err)
				}
				view := readTimedQuestionPrivateDisplay(t, h, selector)
				if calls.Load() != 1 || view.Status != "pending" || view.TimedWait != nil {
					t.Fatal("default ordinary Question stopped blocking or gained a deadline")
				}
				answer := "Synthetic ordinary answer"
				result, err := h.runtimeControl().RespondUserInput(ctx, controlapp.UserInputResponse{InputID: selector.RequestID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: selector.RequestID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &answer}}})
				if err != nil || result.StatusCode != http.StatusOK {
					t.Fatalf("ordinary Host response failed: status=%d code=%s errorType=%T", result.StatusCode, stringField(result.Body, "code"), err)
				}
				wantStatus = domaincontinuation.StatusSubmitted
			}
			var actual []byte
			select {
			case actual = <-resumed:
			case <-ctx.Done():
				t.Fatal("actual Question lifecycle did not resume production HTTP Provider")
			}
			if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
				t.Fatal(err)
			}
			receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, selector.RequestID)
			if err != nil || disposition.Status != wantStatus || calls.Load() != 2 {
				t.Fatal("real signed Question lifecycle did not settle", err)
			}
			if mode == "timed" {
				if disposition.ReasonCode != domaincontinuation.ReasonUserInputContinuedV1 || receipt.Payload.UserInputTimedWait == nil || receipt.Payload.UserInputTimedWait.TimeoutSeconds != 1 || disposition.UserInputSubmission != nil {
					t.Fatal("continued Question did not come from the actual Host timer")
				}
				assertTimedQuestionPendingProviderResult(t, actual, receipt.Payload.CallID)
				assertTimedQuestionDurableResult(t, h, selector.ThreadID, turnID, receipt.Payload.CallID)
			} else if receipt.Payload.UserInputTimedWait != nil || disposition.UserInputSubmission == nil {
				t.Fatal("ordinary Question lost its submitted answer or became timed")
			}
			replay, err := h.store.LoadEventsSince(selector.ThreadID, 0)
			if err != nil || len(replay.Diagnostics) != 0 {
				t.Fatal("committed Question event log unavailable", err)
			}
			endSeq, requested, resolved := 0, 0, 0
			requestTime, resolutionTime := "", ""
			for _, event := range replay.Events {
				if stringField(event, "turnId") != turnID {
					continue
				}
				switch stringField(event, "kind") {
				case "user_input_requested":
					requested++
					requestTime = stringField(event, "timestamp")
				case "user_input_resolved":
					resolved++
					resolutionTime = stringField(event, "timestamp")
				case "turn_completed":
					endSeq, _ = contracts.NumericSeq(event["seq"])
				}
			}
			if requested != 1 || resolved != 1 || endSeq == 0 || requestTime == "" || resolutionTime == "" {
				t.Fatal("real Question/terminal producers did not bind E")
			}
			request := map[string]any{"turnId": turnID, "endSeq": endSeq}
			forkA := forkBoundaryHTTPV1(t, h, http.MethodPost, selector.ThreadID, request, http.StatusCreated)
			var question map[string]any
			for _, raw := range forkBoundaryHistoryV1(forkA) {
				item := raw.(map[string]any)
				if stringField(item, "id") == selector.ItemID {
					question = item
				}
			}
			if question == nil || question["status"] != wantStatus || question["createdAt"] != requestTime || question["finishedAt"] != resolutionTime || question["continuationReceiptId"] != nil {
				t.Fatal("Fork lost the actual committed Question identity/status/times or retained execution authority")
			}
			childSelector := selector
			childSelector.ThreadID = stringField(forkA, "id")
			childView := readTimedQuestionPrivateDisplay(t, h, childSelector)
			if !childView.ReadOnly || childView.TimedWait != nil || childView.Draft != nil || childView.Submission != nil {
				t.Fatal("inherited Question became actionable or exposed source private answer state")
			}
			if _, err := h.startRuntimeTurn(ctx, selector.ThreadID, startRuntimeTurnRequest{Prompt: "Later independent ordinary turn.", Async: true}); err != nil {
				t.Fatal(err)
			}
			if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
				t.Fatal(err)
			}
			forkB := forkBoundaryHTTPV1(t, h, http.MethodPost, selector.ThreadID, request, http.StatusCreated)
			if len(listAny(forkB["turns"])) != 1 || !reflect.DeepEqual(forkBoundaryHistoryV1(forkA), forkBoundaryHistoryV1(forkB)) {
				t.Fatal("same E inherited later turn or changed the Question")
			}
			metaBefore, err := os.ReadFile(h.store.metaPath())
			if err != nil {
				t.Fatal(err)
			}
			forkBoundaryHTTPV1(t, h, http.MethodPost, selector.ThreadID, map[string]any{"turnId": turnID, "endSeq": endSeq - 1}, http.StatusBadRequest)
			metaAfter, err := os.ReadFile(h.store.metaPath())
			if err != nil || !bytes.Equal(metaBefore, metaAfter) {
				t.Fatal("nonterminal boundary consumed a child identity")
			}
			assertForkSeedRestartV1(t, h.store, forkA, turnID)
			if calls.Load() != 3 {
				t.Fatal("Fork/restart/refork re-executed a Question or Provider step")
			}
		})
	}
}

// Synthetic inputs exercise normal Go records, GENERAL CAS/outbox, real late
// job admission, public HTTP and restart. No provider or native UI is invoked.
func TestForkEventBoundaryExcludesDurableLateCallbackBeforeDelivery(t *testing.T) {
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	source, err := store.CreateThread(map[string]any{"title": "Synthetic event boundary", "workspace": workspace, "model": "synthetic-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(source, "id"), "turn_fork_boundary"
	parent := appendForkLoggedParentV1(t, store, threadID, turnID, workspace)
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := jobs.StartRequest{ParentGoalID: "goal_boundary", ParentThreadID: threadID, ParentTurnID: turnID, ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID, Kind: "subagent", Status: "running", Background: true, SecurityBinding: parent.Binding}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &start)
	stale, err := manager.StartChildRun(start)
	if err != nil {
		t.Fatal(err)
	}
	h := &runtimeServerHandler{store: store, jobs: manager, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	pending := runtimePendingToolCall{ThreadID: threadID, TurnID: turnID, ToolCallItemID: parent.ItemID, Call: domainmodel.ToolCall{ID: parent.CallID, Name: "task"}}
	h.recordRuntimeSubagentEvent(pending, stale, "running", "")
	if _, err := manager.UpdateChildRun(stale.ID, domainjob.UpdateRequest{Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	completion, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID, Model: "synthetic-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success"})
	if err != nil || !completion.Changed || completion.PublishedText == "" {
		t.Fatalf("real GENERAL completion: changed=%t err=%v", completion.Changed, err)
	}
	endSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	sourceAtE := forkBoundaryHTTPV1(t, h, http.MethodGet, threadID, nil, http.StatusOK)
	request := map[string]any{"turnId": turnID, "endSeq": endSeq}
	forkA := forkBoundaryHTTPV1(t, h, http.MethodPost, threadID, request, http.StatusCreated)
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	h.recordRuntimeSubagentEvent(pending, stale, "completed", "")
	updated, err := manager.LoadChildRun(stale.ID)
	if err != nil || updated.CompletionDeliveryStatus != "delivered" {
		t.Fatalf("real late admission: status=%s err=%v", updated.CompletionDeliveryStatus, err)
	}
	late, err := store.LoadEventsSince(threadID, endSeq)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, late.Events, updated, "")
	assertCanonicalBackgroundLifecycleBundle(t, late.Events, updated)
	if len(live) == 0 {
		t.Fatal("no real withheld publisher delivery")
	}
	forkB := forkBoundaryHTTPV1(t, h, http.MethodPost, threadID, request, http.StatusCreated)
	if !reflect.DeepEqual(forkBoundaryHistoryV1(forkA), forkBoundaryHistoryV1(forkB)) {
		t.Fatal("same captured E inherited the durably late callback")
	}
	for _, value := range forkBoundaryHistoryV1(forkB) {
		item := value.(map[string]any)
		if stringField(item, "kind") == "tool_progress" {
			t.Fatal("late job notification entered bounded fork")
		}
	}
	if len(forkBoundaryHistoryV1(forkA)) != 3 {
		t.Fatalf("lost native user/call/result prefix: count=%d", len(forkBoundaryHistoryV1(forkA)))
	}
	current := forkBoundaryHTTPV1(t, h, http.MethodGet, threadID, nil, http.StatusOK)
	if len(forkBoundaryHistoryV1(current)) <= len(forkBoundaryHistoryV1(sourceAtE)) {
		t.Fatal("source did not retain late history")
	}
	// A legacy turn-only request also resolves the terminal E, not latestSeq.
	legacy := forkBoundaryHTTPV1(t, h, http.MethodPost, threadID, map[string]any{"turnId": turnID}, http.StatusCreated)
	if !reflect.DeepEqual(forkBoundaryHistoryV1(forkA), forkBoundaryHistoryV1(legacy)) {
		t.Fatal("turn-only compatibility fork used present-day items")
	}
	// The control retains the normal whole-thread snapshot behavior.
	whole := forkBoundaryHTTPV1(t, h, http.MethodPost, threadID, map[string]any{}, http.StatusCreated)
	if len(forkBoundaryHistoryV1(whole)) <= len(forkBoundaryHistoryV1(forkA)) {
		t.Fatal("whole-thread fork lost current late history")
	}
	for len(live) > 0 {
		<-live
	}
	assertForkSeedRestartV1(t, store, forkA, turnID)
}

func TestForkEventBoundaryValidatesHistoricalTurnBeforeIdentityMutation(t *testing.T) {
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	source, err := store.CreateThread(map[string]any{"title": "Historical boundary", "workspace": workspace, "model": "synthetic-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(source, "id")
	first := appendForkLoggedParentV1(t, store, id, "turn_first_boundary", workspace)
	finishForkFixtureV1(t, store, first.Context)
	firstE, _ := store.HighestSeq(id)
	second := appendForkLoggedParentV1(t, store, id, "turn_second_boundary", workspace)
	finishForkFixtureV1(t, store, second.Context)
	secondE, _ := store.HighestSeq(id)
	h := &runtimeServerHandler{store: store, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	metaBefore, err := os.ReadFile(store.metaPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []map[string]any{{"turnId": first.Context.TurnID, "endSeq": firstE - 1}, {"turnId": first.Context.TurnID, "endSeq": secondE}, {"endSeq": firstE}, {"turnId": first.Context.TurnID, "endSeq": nil}} {
		forkBoundaryHTTPV1(t, h, http.MethodPost, id, request, http.StatusBadRequest)
	}
	metaAfter, err := os.ReadFile(store.metaPath())
	if err != nil || !bytes.Equal(metaBefore, metaAfter) {
		t.Fatal("invalid boundary consumed durable identity or metadata")
	}
	fork := forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": first.Context.TurnID, "endSeq": firstE}, http.StatusCreated)
	if len(listAny(fork["turns"])) != 1 {
		t.Fatal("valid historical E included later turn")
	}
	// Removing the current context uses the existing admitted mutation
	// producer and exact durable readback, not the legacy no-replacement API.
	current, err := store.GetThread(id)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := threadapp.PrepareRewindMutation(current, id, second.Context.TurnID, second.Context, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitRewindMutation(threadapp.RewindMutationCommitRequest{
		ThreadID: id, RewindTurnID: second.Context.TurnID, Stamp: prepared.Stamp,
		ExpectedBaselineDigest: prepared.BaselineDigest, ExpectedContextDigest: prepared.SecurityContext.ContextDigest, CurrentContext: second.Context,
	})
	if err != nil || !committed.Committed {
		t.Fatalf("real rewind mutation did not commit: %v", err)
	}
	metaBefore, err = os.ReadFile(store.metaPath())
	if err != nil {
		t.Fatal(err)
	}
	forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": second.Context.TurnID, "endSeq": secondE}, http.StatusNotFound)
	metaAfter, err = os.ReadFile(store.metaPath())
	if err != nil || !bytes.Equal(metaBefore, metaAfter) {
		t.Fatal("removed history consumed identity or was recovered from its old event log")
	}
}

func TestForkEventBoundaryReportsOnlyUnprovableLegacyHistoryUnavailable(t *testing.T) {
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	source, err := store.CreateThread(map[string]any{"title": "Legacy synthetic history", "workspace": workspace, "model": "synthetic-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	id, turnID := stringField(source, "id"), "turn_legacy_boundary"
	now := time.Now().UTC().Truncate(time.Second)
	frozen := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: id, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: now})
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: id, TurnID: turnID, Prompt: "legacy synthetic input", Model: "synthetic-model", ApprovalPolicy: "on-request", SandboxMode: "workspace-write", CreatedAt: now.Format(time.RFC3339Nano)})
	start.Turn["securityContext"] = turnsecurityapp.PublicRecord(frozen)
	// Model the real legacy format: primary history existed, but its initial
	// producer events were never in the durable log. Do not invent those events.
	if err := store.AppendTurnToThread(id, start.Turn, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(frozen)}); err != nil {
		t.Fatal(err)
	}
	finishForkFixtureV1(t, store, frozen)
	endSeq, _ := store.HighestSeq(id)
	h := &runtimeServerHandler{store: store, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": turnID, "endSeq": endSeq}, http.StatusConflict)
	forkBoundaryHTTPV1(t, h, http.MethodGet, id, nil, http.StatusOK)
	forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{}, http.StatusCreated)
}

func appendForkLoggedParentV1(t *testing.T, store *DurableEventSessionStore, threadID, turnID, workspace string, leaveRunning ...bool) serverBoundJobParent {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	frozen := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: now})
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: threadID, TurnID: turnID, Prompt: "bounded background work", Model: "synthetic-model", ProviderID: "provider_1", ApprovalPolicy: "on-request", SandboxMode: "workspace-write", CreatedAt: now.Format(time.RFC3339Nano)})
	start.Turn["securityContext"] = turnsecurityapp.PublicRecord(frozen)
	if err := store.AppendTurnToThread(threadID, start.Turn, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(frozen)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RecordEventsAtomic([]map[string]any{start.TurnStartedEvent, start.UserItemCreatedEvent}); err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("fork-boundary-" + turnID), Name: "task", Arguments: json.RawMessage(`{"prompt":"bounded background work"}`)}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: frozen, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour)})
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, call.ID)
	if _, err := turnapp.PersistToolCallReady(store, turnapp.ToolCallReadyInput{ThreadID: threadID, TurnID: turnID, ItemID: itemID, CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "subagent", Context: frozen, Grant: grant, ReadyCount: 1}); err != nil {
		t.Fatal(err)
	}
	started, ok := toolcatalogapp.BuildToolStartedEvent(toolcatalogapp.ToolStartedEventInput{ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: call.ID, ToolName: call.Name, CreatedAt: now.Format(time.RFC3339Nano)})
	if !ok {
		t.Fatal("normal tool started record missing")
	}
	if _, _, err := store.RecordEvent(started); err != nil {
		t.Fatal(err)
	}
	binding, err := domainjob.NewSecurityBinding(frozen, grant, call.ID)
	if err != nil {
		t.Fatal(err)
	}
	parent := serverBoundJobParent{Context: frozen, Grant: grant, Binding: binding, ItemID: itemID, CallID: call.ID}
	if len(leaveRunning) == 1 && leaveRunning[0] {
		return parent
	}
	result, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{ThreadID: threadID, TurnID: turnID, CreatedAt: now.Format(time.RFC3339Nano), FinishedAt: now.Format(time.RFC3339Nano), Call: call, Projection: domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"), ContextDigest: frozen.ContextDigest, ContextEpoch: frozen.ContextEpoch, ExecutionGrantID: grant.GrantID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureOrdinaryToolResultSettlementExact(context.Background(), threadID, turnID, itemID, result.ResultItem); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RecordEvent(result.Event); err != nil {
		t.Fatal(err)
	}
	return parent
}

func finishForkFixtureV1(t *testing.T, store *DurableEventSessionStore, frozen domainsecurity.TurnSecurityContext) {
	t.Helper()
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: store, SecurityContext: frozen, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Model: "synthetic-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil || !result.Changed {
		t.Fatalf("normal terminal producer: changed=%t err=%v", result.Changed, err)
	}
}

func assertForkSeedRestartV1(t *testing.T, original *DurableEventSessionStore, first map[string]any, inheritedTurnID string) {
	t.Helper()
	id := stringField(first, "id")
	reopened, err := newForkBoundaryStoreV1(t, original.root)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := reopened.LoadEventsSince(id, 0)
	if err != nil || len(replay.Events) < 1 || replay.Events[0]["inheritedHistorySeed"] == nil {
		t.Fatalf("durable creation seed missing: events=%d err=%v", len(replay.Events), err)
	}
	h := &runtimeServerHandler{store: reopened, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	second := forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": inheritedTurnID, "endSeq": len(replay.Events)}, http.StatusCreated)
	if !reflect.DeepEqual(forkBoundaryHistoryV1(first), forkBoundaryHistoryV1(second)) {
		t.Fatal("fork/restart/replay/fork lost inherited history")
	}
	workspace := stringField(first, "workspace")
	own := appendForkLoggedParentV1(t, reopened, id, "turn_after_restart", workspace)
	finishForkFixtureV1(t, reopened, own.Context)
	endSeq, _ := reopened.HighestSeq(id)
	third := forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": own.Context.TurnID, "endSeq": endSeq}, http.StatusCreated)
	if len(listAny(third["turns"])) != 2 || len(forkBoundaryHistoryV1(third)) != len(forkBoundaryHistoryV1(first))+3 {
		t.Fatal("inherited seed plus new real prefix was incomplete")
	}
}

// This actual pending-call response crosses the same strict Main schema in
// analytix-adapter.test.ts as the production Question/fork/restart responses.
func TestForkEventBoundaryReplaysStartedToolBeforeCompletionV1(t *testing.T) {
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	source, err := store.CreateThread(map[string]any{"title": "Started tool at E", "workspace": workspace, "model": "synthetic-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	id, turnID := stringField(source, "id"), "turn_started_tool_boundary"
	parent := appendForkLoggedParentV1(t, store, id, turnID, workspace, true)
	finishForkFixtureV1(t, store, parent.Context)
	endSeq, err := store.HighestSeq(id)
	if err != nil {
		t.Fatal(err)
	}
	h := &runtimeServerHandler{store: store, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	sourceAtE := forkBoundaryHTTPV1(t, h, http.MethodGet, id, nil, http.StatusOK)
	startedAtE := false
	for _, raw := range forkBoundaryHistoryV1(sourceAtE) {
		item := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" {
			t.Fatal("source call completed before E")
		}
		if stringField(item, "id") == parent.ItemID {
			startedAtE = true
			if item["toolKind"] != "subagent" || item["status"] != "pending" || item["finishedAt"] != nil {
				t.Fatal("primary ready call changed without a completion")
			}
		}
	}
	if !startedAtE {
		t.Fatal("source started call missing at E")
	}
	committed, err := store.eventLog.LoadSince(id, 0)
	if err != nil || len(committed.Diagnostics) != 0 {
		t.Fatal("committed started log unavailable", err)
	}
	startedCount := 0
	for _, event := range committed.Events {
		seq, ok := contracts.NumericSeq(event["seq"])
		item, _ := event["item"].(map[string]any)
		if !ok || seq > endSeq || stringField(item, "callId") != parent.CallID {
			continue
		}
		if stringField(event, "kind") == "tool_call_finished" {
			t.Fatal("committed tool completion entered E")
		}
		if stringField(event, "kind") != "tool_call_started" {
			continue
		}
		startedCount++
		if seq <= 0 || event["threadId"] != id || event["turnId"] != turnID || event["itemId"] != parent.ItemID ||
			item["id"] != parent.ItemID || item["threadId"] != id || item["turnId"] != turnID || item["toolName"] != "task" ||
			item["toolKind"] != "subagent" || item["status"] != "running" || item["finishedAt"] != nil {
			t.Fatal("real committed start identity/kind/unfinished lifecycle was lost at E")
		}
	}
	if startedCount != 1 {
		t.Fatal("E did not include exactly one real start producer")
	}
	fullSource, err := store.GetThread(id)
	if err != nil {
		t.Fatal(err)
	}
	prefix, prefixSeq, err := threadapp.BuildForkEventPrefixV1(threadapp.ForkEventPrefixInputV1{Source: fullSource, Events: committed.Events, Boundary: threadapp.ForkBoundaryV1{TurnID: turnID, EndSeq: endSeq}, Projector: h.publicProjector})
	if err != nil || prefixSeq != endSeq {
		t.Fatal("real committed start prefix unavailable", err)
	}
	prefixStarted := false
	for _, raw := range forkBoundaryHistoryV1(prefix) {
		item := raw.(map[string]any)
		if item["kind"] == "tool_result" {
			t.Fatal("prefix manufactured a tool completion")
		}
		if item["id"] == parent.ItemID {
			prefixStarted = true
			if item["toolKind"] != "subagent" || item["status"] != "running" || item["finishedAt"] != nil {
				t.Fatal("E prefix lost the unfinished started producer")
			}
		}
	}
	if !prefixStarted {
		t.Fatal("E prefix omitted the started call")
	}
	fork := forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": turnID, "endSeq": endSeq}, http.StatusCreated)
	found := false
	for _, raw := range forkBoundaryHistoryV1(fork) {
		item := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" {
			t.Fatal("unfinished call gained a completion")
		}
		if stringField(item, "id") == parent.ItemID {
			found = true
			// Existing CloneItemForThread makes inherited in-flight items
			// inert. The prefix test separately proves running/no completion at E.
			if item["toolKind"] != "subagent" || item["status"] != "completed" || item["finishedAt"] != fork["createdAt"] {
				t.Fatal("real started kind or existing inert fork lifecycle was lost")
			}
		}
	}
	if !found {
		t.Fatal("real started call was lost")
	}
}

func forkBoundaryHTTPV1(t *testing.T, h *runtimeServerHandler, method, id string, body map[string]any, status int) map[string]any {
	t.Helper()
	path := "/v1/threads/" + id
	if method == http.MethodPost {
		path += "/fork"
	}
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	before, err := os.ReadFile(h.store.metaPath())
	if err != nil {
		t.Fatal(err)
	}
	var beforeMeta durableMeta
	if json.Unmarshal(before, &beforeMeta) != nil {
		t.Fatal("invalid pre-fork metadata")
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("normal Fork HTTP code=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	var value map[string]any
	if json.Unmarshal(response.Body.Bytes(), &value) != nil {
		t.Fatal("invalid public JSON")
	}
	if method == http.MethodPost && status == http.StatusCreated {
		after, err := os.ReadFile(h.store.metaPath())
		if err != nil {
			t.Fatal(err)
		}
		var afterMeta durableMeta
		if json.Unmarshal(after, &afterMeta) != nil {
			t.Fatal("invalid post-fork metadata")
		}
		beforeIDs := map[string]bool{}
		for _, existing := range beforeMeta.ListedThreadIDs {
			beforeIDs[existing] = true
		}
		created := []string{}
		for _, existing := range afterMeta.ListedThreadIDs {
			if !beforeIDs[existing] {
				created = append(created, existing)
			}
		}
		if len(created) != 1 || created[0] != stringField(value, "id") || len(afterMeta.ListedThreadIDs) != len(beforeMeta.ListedThreadIDs)+1 {
			t.Fatal("one successful Fork HTTP request did not create exactly one branch")
		}
		if target := os.Getenv("ANALYTIX_FORK_MAIN_WITNESS_FILE"); target != "" {
			// Test-only export of exact HTTP bytes, never a product entry or an
			// expected DTO assembled independently of the Go handler.
			records := []map[string]any{}
			if existing, err := os.ReadFile(target); err == nil {
				if json.Unmarshal(existing, &records) != nil {
					t.Fatal("invalid Main witness")
				}
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			records = append(records, map[string]any{"test": t.Name(), "path": path, "request": body, "status": response.Code, "body": response.Body.String(), "createdBranches": len(created)})
			encoded, err := json.Marshal(records)
			if err != nil || os.WriteFile(target, encoded, 0600) != nil {
				t.Fatal("Main witness export failed")
			}
		}
	}
	return value
}

func forkBoundaryHistoryV1(thread map[string]any) []any {
	items := []any{}
	for _, raw := range listAny(thread["turns"]) {
		turn := raw.(map[string]any)
		for _, value := range listAny(turn["items"]) {
			item := cloneMap(value.(map[string]any))
			item["threadId"] = "derived-owner"
			items = append(items, item)
		}
	}
	return items
}

func newForkBoundaryStoreV1(t *testing.T, root string) (*DurableEventSessionStore, error) {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		return nil, err
	}
	primary, err := finalauthorityadapter.NewAcceptedFinalCASReader(store.root)
	if err != nil {
		return nil, err
	}
	if err := store.BindPrimaryThreadReaderV1(primary); err != nil {
		return nil, err
	}
	return store, nil
}

func TestForkEventBoundaryIncludesAdmittedCompletionBeforeE(t *testing.T) {
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	source, err := store.CreateThread(map[string]any{"title": "Completion before boundary", "workspace": workspace, "model": "synthetic-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	id, turnID := stringField(source, "id"), "turn_completion_before_boundary"
	parent := appendForkLoggedParentV1(t, store, id, turnID, workspace)
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := jobs.StartRequest{ParentGoalID: "goal_boundary", ParentThreadID: id, ParentTurnID: turnID, ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID, Kind: "subagent", Status: "running", Background: true, SecurityBinding: parent.Binding}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &start)
	stale, err := manager.StartChildRun(start)
	if err != nil {
		t.Fatal(err)
	}
	h := &runtimeServerHandler{store: store, jobs: manager, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	pending := runtimePendingToolCall{ThreadID: id, TurnID: turnID, ToolCallItemID: parent.ItemID, Call: domainmodel.ToolCall{ID: parent.CallID, Name: "task"}}
	h.recordRuntimeSubagentEvent(pending, stale, "running", "")
	completed, err := manager.UpdateChildRun(stale.ID, domainjob.UpdateRequest{Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	h.recordRuntimeSubagentEvent(pending, stale, "completed", "")
	admitted, err := manager.LoadChildRun(stale.ID)
	if err != nil || admitted.CompletionDeliveryStatus != "delivered" {
		t.Fatalf("completion admission: err=%v", err)
	}
	finishForkFixtureV1(t, store, parent.Context)
	endSeq, err := store.HighestSeq(id)
	if err != nil {
		t.Fatal(err)
	}
	fork := forkBoundaryHTTPV1(t, h, http.MethodPost, id, map[string]any{"turnId": turnID, "endSeq": endSeq}, http.StatusCreated)
	history := forkBoundaryHistoryV1(fork)
	if len(history) != 4 {
		t.Fatalf("before-E producer item lost: count=%d", len(history))
	}
	item := history[3].(map[string]any)
	if item["kind"] != "tool_progress" || item["createdAt"] != completed.FinishedAt {
		t.Fatal("before-E notification identity or actual finishedAt changed")
	}
	assertForkSeedRestartV1(t, store, fork, turnID)
}
