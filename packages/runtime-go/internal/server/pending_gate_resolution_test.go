package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	apploop "analytix.local/runtime-go/internal/app/loop"
	modelapp "analytix.local/runtime-go/internal/app/model"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	appturn "analytix.local/runtime-go/internal/app/turn"
	displayapp "analytix.local/runtime-go/internal/app/userinputdisplay"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	jobs "analytix.local/runtime-go/internal/jobs"
	mcp "analytix.local/runtime-go/internal/mcp"
	displayport "analytix.local/runtime-go/internal/ports/userinputdisplay"
	runtimeprovider "analytix.local/runtime-go/internal/provider"

	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRecordPendingGateResolutionIsIdempotentAndRejectsConflicts(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "provider-a", BaseURL: "https://provider.invalid", APIKey: "test-only",
		Model: "model-a", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	event := map[string]any{
		"kind": "approval_resolved", "threadId": threadID, "turnId": "turn-a",
		"itemId": "item-a", "approvalId": "approval-a", "status": "allowed",
	}
	if err := handler.recordPendingGateResolution(event); err != nil {
		t.Fatal(err)
	}
	if err := handler.recordPendingGateResolution(event); err != nil {
		t.Fatalf("exact resolution retry was not idempotent: %v", err)
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Events) != 1 {
		t.Fatalf("exact retry duplicated the event: events=%#v err=%v", replay.Events, err)
	}
	for name, patch := range map[string]map[string]any{
		"status":      {"status": "denied"},
		"cancelledBy": {"cancelledBy": "runtime_shutdown"},
		"item":        {"itemId": "item-other"},
	} {
		conflicting := make(map[string]any, len(event)+1)
		for key, value := range event {
			conflicting[key] = value
		}
		for key, value := range patch {
			conflicting[key] = value
		}
		if err := handler.recordPendingGateResolution(conflicting); err == nil {
			t.Fatalf("%s conflict was accepted: %#v", name, conflicting)
		}
	}
	for _, key := range []string{"threadId", "turnId", "itemId", "approvalId", "status"} {
		invalid := make(map[string]any, len(event))
		for field, value := range event {
			invalid[field] = value
		}
		delete(invalid, key)
		if err := handler.recordPendingGateResolution(invalid); err == nil {
			t.Fatalf("resolution without %s was accepted", key)
		}
	}
}

// This direct admission fixture uses the installation's real signed Host
// receipt, disposition and private capture APIs under an actual running turn.
// It does not claim timer, Gate settlement, promotion or restart acceptance;
// the separate production-chain fixture supplies those seams.
func TestLateQuestionHostIdentityDoesNotRaiseCaseRiskOrChangeCorrelation(t *testing.T) {
	startedProvider, release := make(chan struct{}), make(chan struct{})
	var providerOnce, releaseOnce sync.Once
	var providerCancelled atomic.Bool
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		providerOnce.Do(func() { close(startedProvider) })
		select {
		case <-release:
		case <-r.Context().Done():
			providerCancelled.Store(true)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Synthetic diagnostic complete.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	h := f.handler
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Errorf("diagnostic shutdown failed: errorType=%T", err)
		}
	})
	started, err := h.startRuntimeTurn(context.Background(), f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Continue ordinary synthetic work.", Async: true})
	if err != nil {
		t.Fatalf("diagnostic ordinary turn failed to start: errorType=%T", err)
	}
	turnID := stringField(started, "turnId")
	select {
	case <-startedProvider:
	case <-time.After(15 * time.Second):
		t.Fatal("diagnostic production Provider did not start")
	}
	frozen, err := appturn.LoadFrozenSecurityContext(h.store, f.request.ThreadID, turnID)
	if err != nil || domainsecurity.TurnSecurityContextAllowsCaseEvidence(frozen) {
		t.Fatalf("diagnostic turn lacks actual ordinary authority: errorType=%T", err)
	}

	const numericMarker = "123456789012"
	safeDigest := strings.Repeat("a", 64)
	numericDigest := strings.Repeat("a", 26) + numericMarker + strings.Repeat("a", 26)
	large := strings.Repeat("ordinary synthetic answer ", 900)
	for index, test := range []struct {
		name, callDigest, rawAnswer, detail string
		questionHasRisk, wantCaseRisk       bool
	}{
		{name: "safe_host_ids", callDigest: safeDigest, rawAnswer: large},
		{name: "numeric_call_id", callDigest: numericDigest, rawAnswer: large},
		{name: "numeric_question_id", callDigest: safeDigest, questionHasRisk: true, rawAnswer: large},
		{name: "numeric_user_content_control", callDigest: safeDigest, rawAnswer: large + numericMarker, wantCaseRisk: true},
		{name: "numeric_detail_content_control", callDigest: safeDigest, rawAnswer: large, detail: numericMarker, wantCaseRisk: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			callID := "call_host_" + test.callDigest
			clientID := fmt.Sprintf("00000000-0000-4000-8000-%012d", index+11)
			receipt := timedQuestionSignedAdmissionSource(t, h, frozen, callID, test.questionHasRisk, index, test.detail)
			inputID, questionID := receipt.Payload.GateID, receipt.Payload.GateID+"_1"
			binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: receipt.Payload.ThreadID, TurnID: receipt.Payload.TurnID, ItemID: receipt.Payload.ItemID, InputID: inputID, ReceiptID: receipt.ReceiptID, SecurityContext: receipt.Payload.SecurityContext}
			var args map[string]any
			if json.Unmarshal(receipt.Payload.Arguments, &args) != nil {
				t.Fatal("signed synthetic source arguments are invalid")
			}
			questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, inputID, "User input required")
			answerRequest := controlapp.UserInputResponse{StructuredAnswers: []controlapp.UserInputAnswer{{ID: questionID, Kind: "answer", OptionIDs: []string{}, CustomText: &test.rawAnswer}}}
			_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(answerRequest, questions, "")
			if err != nil || submission == nil {
				t.Fatalf("synthetic capture normalization failed: errorType=%T", err)
			}
			capture, err := h.continuations.CaptureLateUserInputSubmissionHost(context.Background(), binding, clientID, *submission)
			if err != nil {
				t.Fatalf("real private capture failed: errorType=%T", err)
			}
			carrier, err := h.verifiedLateUserInputSteeringV1(context.Background(), binding, clientID, capture.ReceiptID, turnID)
			if err != nil {
				t.Fatalf("signed carrier construction failed: errorType=%T", err)
			}
			for name, wrong := range map[string]continuationapp.UserInputLocalDisplayBindingV1{"wrong_receipt": binding, "wrong_thread": binding} {
				if name == "wrong_receipt" {
					wrong.ReceiptID = strings.Repeat("b", 64)
				} else {
					wrong.ThreadID = "foreign-synthetic-thread"
				}
				if _, err := h.verifiedLateUserInputSteeringV1(context.Background(), wrong, clientID, capture.ReceiptID, turnID); err == nil {
					t.Errorf("%s acquired typed metadata authority", name)
				}
			}
			if _, err := h.verifiedLateUserInputSteeringV1(context.Background(), binding, clientID, strings.Repeat("c", 64), turnID); err == nil {
				t.Error("wrong capture acquired typed metadata authority")
			}
			text, projected := carrier.text, carrier.projectedText
			rawBound := carrier.identity != nil && carrier.identity.CallID == callID && len(carrier.identity.Questions) == 1 && carrier.identity.Questions[0].ID == questionID && !strings.Contains(text, callID) && !strings.Contains(text, questionID)
			projectedBound := rawBound && !strings.Contains(projected, callID) && !strings.Contains(projected, questionID)
			lexicalRisk := apploop.PromptRequiresCaseRiskAdmission(carrier.content)
			protectedRisk := domainsecurity.ContainsProtectedCaseData(carrier.content)
			carrierContext := context.WithValue(context.Background(), lateUserInputSteeringContextKeyV1{}, carrier)
			beforeEntries, beforePromoted, beforeEvents, _ := timedQuestionDiagnosticSteeringEffects(t, h, f.request.ThreadID, turnID, clientID)
			for _, change := range []func(*controlapp.SteerTurnRequest){
				func(r *controlapp.SteerTurnRequest) { r.ExpectedTurnID = "obsolete-synthetic-turn" },
				func(r *controlapp.SteerTurnRequest) { r.TurnID = "obsolete-synthetic-turn" },
				func(r *controlapp.SteerTurnRequest) { r.Text += "forged-synthetic-content" },
				func(r *controlapp.SteerTurnRequest) { r.UserInputCaptureReceiptID = strings.Repeat("b", 64) },
			} {
				wrong := carrier.requestV1()
				change(&wrong)
				rejected, err := h.runtimeControl().SteerTurn(carrierContext, wrong)
				if err != nil || rejected.StatusCode != http.StatusConflict || stringField(rejected.Body, "code") != "late_reply_unavailable" {
					t.Error("altered typed carrier request was not rejected")
				}
			}
			for _, forgedContext := range []context.Context{context.Background(), context.WithValue(context.Background(), lateUserInputSteeringContextKeyV1{}, map[string]any{"callId": callID})} {
				rejected, err := h.runtimeControl().SteerTurn(forgedContext, carrier.requestV1())
				if err != nil || rejected.StatusCode != http.StatusConflict || stringField(rejected.Body, "code") != "late_reply_unavailable" {
					t.Error("untyped carrier acquired Host metadata authority")
				}
			}
			result, err := h.runtimeControl().SteerTurn(carrierContext, carrier.requestV1())
			entries, promoted, events, state := timedQuestionDiagnosticSteeringEffects(t, h, f.request.ThreadID, turnID, clientID)
			code, blocker := stringField(result.Body, "code"), stringField(result.Body, "blockerCode")
			t.Logf("host_id_diagnostic case=%s status=%d code=%s blocker=%s errorType=%T callHash=%x questionHash=%x turnHash=%x attemptHash=%x rawAnswerBytes=%d normalizedAnswerBytes=%d rawEnvelopeBytes=%d projectedBytes=%d rawBound=%t projectedBound=%t lexicalRisk=%t protectedRisk=%t queueEntries=%d promotedItems=%d steeredEvents=%d turnState=%s providerCancelled=%t", test.name, result.StatusCode, code, blocker, err, sha256.Sum256([]byte(callID)), sha256.Sum256([]byte(questionID)), sha256.Sum256([]byte(turnID)), sha256.Sum256([]byte(clientID)), len(test.rawAnswer), len(strings.TrimSpace(test.rawAnswer)), len(text), len(projected), rawBound, projectedBound, lexicalRisk, protectedRisk, entries, promoted, events, state, providerCancelled.Load())
			if !rawBound {
				t.Error("Host late-envelope producer changed original correlation")
			}
			if !test.wantCaseRisk && !projectedBound {
				t.Error("typed steering projection changed Host correlation")
			}
			if lexicalRisk != test.wantCaseRisk || protectedRisk != test.wantCaseRisk {
				t.Errorf("Host metadata changed user-content classification: lexical=%t protected=%t want=%t", lexicalRisk, protectedRisk, test.wantCaseRisk)
			}

			if state != "running" || providerCancelled.Load() {
				t.Fatal("diagnostic ordinary turn authority changed; later cases cannot share it")
			}
			delivery, deliveryErr := h.store.readUserInputLateReplyV1(f.request.ThreadID, inputID, clientID)
			if test.wantCaseRisk {
				if err != nil || result.StatusCode != http.StatusConflict || code != "new_turn_required" || blocker != controlapp.SteerBlockerCaseRiskRaise {
					t.Error("actual user-content risk bypassed the existing authority boundary")
				}
				if entries != beforeEntries || promoted != beforePromoted || events != beforeEvents || deliveryErr != nil || delivery != nil {
					t.Error("rejected diagnostic steer produced a queue, promotion or event effect")
				}
			} else {
				if err != nil || result.StatusCode != http.StatusOK || code != "" || entries != beforeEntries+1 || promoted != beforePromoted || events != beforeEvents+1 || deliveryErr != nil || delivery == nil || delivery.TurnID != turnID || delivery.State != "queued" {
					t.Errorf("legal Host Question envelope lost exact queued admission: status=%d code=%s", result.StatusCode, code)
				}
			}
		})
		if providerCancelled.Load() {
			break
		}
	}
	// The Question-specific budget never grants ordinary steering a larger cap.
	ordinaryID := "00000000-0000-4000-8000-000000000023"
	beforeEntries, beforePromoted, beforeEvents, _ := timedQuestionDiagnosticSteeringEffects(t, h, f.request.ThreadID, turnID, ordinaryID)
	ordinary, err := h.runtimeControl().SteerTurn(context.Background(), controlapp.SteerTurnRequest{ThreadID: f.request.ThreadID, TurnID: turnID, ExpectedTurnID: turnID, Text: large, ClientUserMessageID: ordinaryID})
	entries, promoted, events, state := timedQuestionDiagnosticSteeringEffects(t, h, f.request.ThreadID, turnID, ordinaryID)
	if err != nil || ordinary.StatusCode != http.StatusConflict || stringField(ordinary.Body, "code") != "conflict" || entries != beforeEntries || promoted != beforePromoted || events != beforeEvents || state != "running" || providerCancelled.Load() {
		t.Fatal("Question budget widened ordinary steering or changed active authority")
	}
	generic := `{"kind":"answer_to_pending_question","callId":"call_host_` + numericDigest + `","questions":[],"answers":[]}`
	result, err := h.runtimeControl().SteerTurn(context.Background(), controlapp.SteerTurnRequest{ThreadID: f.request.ThreadID, TurnID: turnID, ExpectedTurnID: turnID, Text: generic, ClientUserMessageID: "00000000-0000-4000-8000-000000000021"})
	if err != nil || result.StatusCode != http.StatusConflict || stringField(result.Body, "blockerCode") != controlapp.SteerBlockerCaseRiskRaise {
		t.Error("public JSON-shaped metadata bypassed ordinary text classification")
	}
	forged := httptest.NewRecorder()
	body := `{"text":"ordinary synthetic input","clientUserMessageId":"00000000-0000-4000-8000-000000000022","lateUserInputSteering":{"callId":"synthetic"}}`
	h.handleThreadTurnAction(forged, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), f.request.ThreadID+"/turns/"+turnID+"/steer")
	if forged.Code != http.StatusBadRequest {
		t.Errorf("public carrier forgery was not rejected: status=%d", forged.Code)
	}

}

// A bounded fixture selection derives the Question ID from its real context,
// grant and Host call. Only the selected candidate is signed/persisted. This
// direct Host API fixture does not create a timer or claim a UI continuation.
func timedQuestionSignedPendingAdmissionSource(t *testing.T, h *runtimeServerHandler, frozen domainsecurity.TurnSecurityContext, callID string, questionHasRisk bool, caseIndex int, detail string) (domaincontinuation.Receipt, runtimePendingToolCall) {
	t.Helper()
	now := time.Now().UTC()
	arguments := json.RawMessage(`{"prompt":"Choose a synthetic answer","questions":[{"question":"What should happen next?"}],"timeout":120}`)
	if detail != "" {
		body, err := json.Marshal(map[string]any{"prompt": "Choose a synthetic answer", "questions": []any{map[string]any{"question": "What should happen next?", "detail": detail}}, "timeout": 120})
		if err != nil {
			t.Fatalf("synthetic detail encoding failed: errorType=%T", err)
		}
		arguments = body
	}
	call := domainmodel.ToolCall{ID: callID, Name: "request_user_input", Arguments: arguments}
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, nil, false, false, "ordinary synthetic work", false, nil)
	scope := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		scope = append(scope, schema.Name)
	}
	var pending runtimePendingToolCall
	var inputID string
	found := false
	for candidate := 0; candidate < 1024; candidate++ {
		issued := now.Add(-time.Duration(candidate+caseIndex*1024) * time.Nanosecond)
		grant, err := executiongrantapp.IssueProvider(frozen, "openai", call, schemas, scope, true, "not_required", 0, "", issued)
		if err != nil {
			code := "unknown"
			if value, ok := err.(executiongrantapp.ValidationError); ok {
				code = value.Code
			}
			t.Fatalf("synthetic Host grant failed: errorType=%T code=%s", err, code)
		}
		inputID = controlapp.SecureGateID(domaincontinuation.KindUserInput, frozen.ThreadID, frozen.TurnID, frozen.ContextDigest, grant.GrantID, call.ID)
		if domainsecurity.ContainsProtectedCaseData(inputID+"_1") != questionHasRisk {
			continue
		}
		pending = runtimePendingToolCall{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Workspace: frozen.WorkspaceRealPath,
			ProviderConfig: domainmodel.TurnConfig{ProviderID: "openai", Model: "fixture-model", APIKey: "synthetic-only", BaseURL: "http://127.0.0.1:1/v1", EndpointFormat: "chat_completions"}, ProviderID: "openai", Model: "fixture-model", Prompt: "ordinary synthetic work", ApprovalPolicy: "never", SandboxMode: "workspace-write", Call: call, ToolCallItemID: domaintoolcall.ToolCallItemIDV1(frozen.TurnID, call.ID), ToolScope: scope, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true, OrdinaryResultInputIsolated: true, ProviderNamespace: domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil), SecurityContext: frozen, ExecutionGrant: grant, UserInputTimedWait: &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 120}}
		found = true
		break
	}
	if !found {
		t.Fatal("bounded synthetic grant selection did not derive the required Question ID shape")
	}
	receipt, err := h.continuations.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatalf("synthetic signed source issuance failed: errorType=%T", err)
	}
	return receipt, pending
}

func timedQuestionSignedAdmissionSource(t *testing.T, h *runtimeServerHandler, frozen domainsecurity.TurnSecurityContext, callID string, questionHasRisk bool, caseIndex int, detail string) domaincontinuation.Receipt {
	t.Helper()
	receipt, pending := timedQuestionSignedPendingAdmissionSource(t, h, frozen, callID, questionHasRisk, caseIndex, detail)
	if err := h.continuations.DisposePendingHost(receipt.Payload.GateID, domaincontinuation.StatusContinued, "user_input_continued", time.Now().UTC(), pending); err != nil {
		t.Fatalf("synthetic signed disposition failed: errorType=%T", err)
	}
	return receipt
}

// Persist the verified signed-source request through the same exact projection
// and outbox owners as the Host lifecycle. The disposition supplies the status;
// this fixture does not synthesize a continued item or claim tool settlement.
func persistTimedQuestionSignedSourceProjection(t *testing.T, h *runtimeServerHandler, source domaincontinuation.Receipt) {
	t.Helper()
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(context.Background(), source.Payload.GateID)
	if err != nil || receipt.ReceiptID != source.ReceiptID {
		t.Fatal("small source signed authority unavailable for persistence")
	}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		t.Fatal("small source request projection failed")
	}
	if err := h.store.EnsureGateRequestItemExact(receipt.Payload.ThreadID, receipt.Payload.TurnID, projection.Item); err != nil {
		t.Fatal("small source exact request item failed")
	}
	if err := h.recordPendingGateRequest(projection.Event); err != nil {
		t.Fatal("small source request outbox failed")
	}
	status, cancelledBy, err := gatecontinuationapp.RestartDispositionProjectionV1(disposition)
	if err != nil {
		t.Fatal("small source disposition projection failed")
	}
	if err := h.recordPendingGateResolution(gatecontinuationapp.RestartResolutionEventV1(projection, status, cancelledBy)); err != nil {
		t.Fatal("small source resolution outbox failed")
	}
	if err := h.store.PatchTurnItemStatus(receipt.Payload.ThreadID, receipt.Payload.TurnID, receipt.Payload.ItemID, status); err != nil {
		t.Fatal("small source disposition item projection failed")
	}
}

func timedQuestionEnvelopeIdentityMatches(text, callID, questionID string) bool {
	var body struct {
		CallID    string `json:"callId"`
		Questions []struct {
			ID string `json:"id"`
		} `json:"questions"`
		Answers []struct {
			ID string `json:"id"`
		} `json:"answers"`
	}
	return json.Unmarshal([]byte(text), &body) == nil && body.CallID == callID && len(body.Questions) == 1 && len(body.Answers) == 1 && body.Questions[0].ID == questionID && body.Answers[0].ID == questionID
}

func timedQuestionDiagnosticSteeringEffects(t *testing.T, h *runtimeServerHandler, threadID, turnID, clientID string) (int, int, int, string) {
	t.Helper()
	thread, err := h.store.GetThread(threadID)
	if err != nil {
		t.Fatalf("diagnostic primary state read failed: errorType=%T", err)
	}
	turns, _ := thread["turns"].([]any)
	entries, promoted, state := 0, 0, "missing"
	for _, raw := range turns {
		turn, ok := raw.(map[string]any)
		if !ok || stringField(turn, "id") != turnID {
			continue
		}
		state = stringField(turn, "status")
		steering, _ := turn["steering"].([]any)
		entries = len(steering)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == domainsteering.EntryIDV1(turnID, clientID) {
				promoted++
			}
		}
	}
	replay, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("diagnostic event read failed: errorType=%T", err)
	}
	events := 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == "turn_steered" {
			events++
		}
	}
	return entries, promoted, events, state
}

// This fixture enters the actual production HTTP Provider -> tool dispatch ->
// signed Gate -> real timer -> continuation -> steering promotion chain. The
// upstream is synthetic and local; no live Provider/key or hand-built
// continued item supplies the result under test.
func TestTimedQuestionProductionTimeoutLateQueuePromotionAndRestart(t *testing.T) {
	var calls atomic.Int32
	continuing, promoted, release := make(chan []byte, 1), make(chan []byte, 1), make(chan struct{})
	var releaseOnce sync.Once
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		call := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			args := `{"prompt":"Choose a synthetic answer","questions":[{"question":"What should happen next?"}],"timeout":1}`
			chunk := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "synthetic-wire-question", "type": "function", "function": map[string]any{"name": "request_user_input", "arguments": args}}}}, "finish_reason": "tool_calls"}}}
			encoded, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
			return
		}
		if call == 2 {
			continuing <- body
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		if call == 3 {
			promoted <- body
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"The synthetic task is complete.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	h := f.handler
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureHistory := configureTimedQuestionHistoryAuthority(t, h)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	started, err := h.startRuntimeTurn(context.Background(), f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Ask one optional question, then continue independent ordinary work.", Async: true})
	if err != nil {
		t.Fatal(err)
	}
	turnID := stringField(started, "turnId")
	var inputID, itemID string
	waitTimedQuestionCondition(t, func() bool {
		replay, err := h.store.LoadEventsSince(f.request.ThreadID, 0)
		if err != nil {
			return false
		}
		for _, event := range replay.Events {
			if stringField(event, "kind") == "user_input_requested" {
				inputID, itemID = stringField(event, "inputId"), stringField(event, "itemId")
				return true
			}
		}
		return false
	})
	var resumed []byte
	select {
	case resumed = <-continuing:
	case <-time.After(15 * time.Second):
		t.Fatal("real timer did not resume production HTTP Provider")
	}
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(context.Background(), inputID)
	if err != nil || disposition.Status != domaincontinuation.StatusContinued || receipt.Payload.UserInputTimedWait == nil || receipt.Payload.UserInputTimedWait.TimeoutSeconds != 1 || disposition.UserInputSubmission != nil {
		t.Fatal("actual timer lacked continued signed authority", err)
	}
	assertTimedQuestionPendingProviderResult(t, resumed, receipt.Payload.CallID)
	assertTimedQuestionDurableResult(t, h, f.request.ThreadID, turnID, receipt.Payload.CallID)
	// This privately capturable answer exceeds the unchanged real prose-leaf
	// capacity. Rejection follows private capture and precedes queue mutation.
	tooBigText := strings.Repeat("x", (1<<20)+1024)
	tooBigRequest := controlapp.UserInputResponse{InputID: inputID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: inputID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &tooBigText}}, LateReply: &controlapp.UserInputLateReplyV1{ClientUserMessageID: "00000000-0000-4000-8000-000000000009", ExpectedTurnID: turnID}}
	tooBigResult, err := h.runtimeControl().RespondUserInput(context.Background(), tooBigRequest)
	if err != nil || tooBigResult.StatusCode != http.StatusConflict || stringField(tooBigResult.Body, "code") != "late_reply_too_large" {
		t.Fatal("oversize reply lacked closed inline capacity failure", err)
	}
	if delivery, err := h.store.readUserInputLateReplyV1(f.request.ThreadID, inputID, tooBigRequest.LateReply.ClientUserMessageID); err != nil || delivery != nil {
		t.Fatal("oversize answer mutated queue", err)
	}
	tooBigSelector := displayport.SelectorV1{ThreadID: f.request.ThreadID, TurnID: turnID, ItemID: itemID, RequestID: inputID}
	tooBigDisplay := readTimedQuestionPrivateDisplay(t, h, tooBigSelector)
	if tooBigDisplay.LateReply != nil || tooBigDisplay.Submission == nil || !bytes.Contains(tooBigDisplay.Submission.Answers, []byte(tooBigText)) {
		t.Fatal("unadmitted complete original answer was shortened or lost")
	}
	// Exercise the complete real envelope with a legal answer larger than the
	// unchanged 16KiB ordinary steering budget. No shortened/hydrated body.
	original := "  " + strings.Repeat("ordinary synthetic answer ", 900) + "\n  "
	request := controlapp.UserInputResponse{InputID: inputID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: inputID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &original}}, LateReply: &controlapp.UserInputLateReplyV1{ClientUserMessageID: "00000000-0000-4000-8000-000000000001", ExpectedTurnID: turnID}}
	result, err := h.runtimeControl().RespondUserInput(context.Background(), request)
	if err != nil || result.StatusCode != http.StatusOK || stringField(result.Body, "status") != "continued" {
		t.Fatalf("late reply failed: errorType=%T status=%d code=%s blocker=%s", err, result.StatusCode, stringField(result.Body, "code"), stringField(result.Body, "blockerCode"))
	}
	assertTimedQuestionLateReplyResponse(t, result, inputID, request.LateReply.ClientUserMessageID, turnID, "queued")
	delivery, err := h.store.readUserInputLateReplyV1(f.request.ThreadID, inputID, request.LateReply.ClientUserMessageID)
	if err != nil || delivery == nil || delivery.State != "queued" {
		t.Fatal("enqueue was falsely accepted", err)
	}
	selector := displayport.SelectorV1{ThreadID: f.request.ThreadID, TurnID: turnID, ItemID: itemID, RequestID: inputID}
	display := readTimedQuestionPrivateDisplay(t, h, selector)
	if display.LateReply == nil || display.LateReply.State != "queued" || display.Submission == nil || !bytes.Contains(display.Submission.Answers, []byte("ordinary synthetic answer")) {
		t.Fatal("queued private view lost original answer")
	}
	// Same UUID and exact raw answers must not duplicate either queue or event.
	retry, err := h.runtimeControl().RespondUserInput(context.Background(), request)
	if err != nil {
		t.Fatal("exact late retry failed", err)
	}
	assertTimedQuestionLateReplyResponse(t, retry, inputID, request.LateReply.ClientUserMessageID, turnID, "queued")
	assertTimedQuestionSingleSteeredEvent(t, h, f.request.ThreadID, request.LateReply.ClientUserMessageID)
	releaseOnce.Do(func() { close(release) })
	waitTimedQuestionCondition(t, func() bool {
		delivery, err := h.store.readUserInputLateReplyV1(f.request.ThreadID, inputID, request.LateReply.ClientUserMessageID)
		return err == nil && delivery != nil && delivery.State == "accepted"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
		t.Fatal(err)
	}
	var admittedRequest []byte
	select {
	case admittedRequest = <-promoted:
	case <-ctx.Done():
		t.Fatal("actual promoted reply did not reach subsequent production step")
	}
	assertTimedQuestionPromotedProviderReply(t, admittedRequest, receipt.Payload.CallID, inputID, strings.TrimSpace(original))
	assertTimedQuestionPendingProviderResult(t, admittedRequest, receipt.Payload.CallID)
	assertTimedQuestionDurableResult(t, h, f.request.ThreadID, turnID, receipt.Payload.CallID)
	_, originalDisposition, err := h.continuations.ResolveTrustedDispositionForAudit(context.Background(), inputID)
	if err != nil || originalDisposition.Status != domaincontinuation.StatusContinued || originalDisposition.UserInputSubmission != nil {
		t.Fatal("late admission changed the original pending result disposition", err)
	}
	if calls.Load() != 3 {
		t.Fatal("unexpected extra production step after one late reply")
	}
	display = readTimedQuestionPrivateDisplay(t, h, selector)
	if display.LateReply == nil || display.LateReply.State != "accepted" || display.Submission == nil {
		t.Fatal("actual admission did not settle late delivery")
	}
	if err := h.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	// Reopen actual durable primary + private stores with the installation key.
	// A new ordinary turn must replay the original timeout result alongside the
	// separately admitted user message, without invoking the old Question again.
	replayBody := make(chan []byte, 1)
	var restartedHTTPCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		restartedHTTPCalls.Add(1)
		select {
		case replayBody <- body:
		case <-r.Context().Done():
			return
		}
		writeInlineCompletionFixtureReply(w)
	}))
	t.Cleanup(upstream.Close)
	restarted := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: f.durableRoot, DataDir: h.dataDir, ProviderID: "openai", BaseURL: upstream.URL + "/v1", APIKey: "synthetic-fixture-key", Model: "fixture-model", EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, restarted)
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal(err)
	}
	restarted.turnSecurity.RiskAuthority = risk
	configureHistory(restarted)
	installHandlerProviderExecutionResolverForTest(restarted)
	restarted.provider = h.provider
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := restarted.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	restored := readTimedQuestionPrivateDisplay(t, restarted, selector)
	if restored.LateReply == nil || restored.LateReply.State != "accepted" || restored.Submission == nil || !bytes.Equal(restored.Submission.Answers, display.Submission.Answers) {
		t.Fatal("restart lost private original/actual admission")
	}
	if inventory, err := restarted.continuations.TrustedInventoryV1(context.Background()); err != nil || len(inventory.Receipts) != 1 {
		t.Fatal("capture became extra Gate on restart", err)
	}
	retry, err = restarted.runtimeControl().RespondUserInput(context.Background(), request)
	if err != nil {
		t.Fatal("terminal/restart duplicate failed", err)
	}
	assertTimedQuestionLateReplyResponse(t, retry, inputID, request.LateReply.ClientUserMessageID, turnID, "accepted")
	if restartedHTTPCalls.Load() != 0 {
		t.Fatal("terminal callback invoked Provider")
	}
	assertTimedQuestionSingleSteeredEvent(t, restarted, f.request.ThreadID, request.LateReply.ClientUserMessageID)
	wrong := selector
	wrong.ThreadID = "foreign-thread"
	body, _ := json.Marshal(wrong)
	recorder := httptest.NewRecorder()
	restarted.UserInputLocalDisplayV1().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/local-display/user-input", bytes.NewReader(body)))
	if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "original synthetic answer") {
		t.Fatal("raw late capture crossed original thread authority")
	}
	if _, err := restarted.startRuntimeTurn(context.Background(), f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Continue independent ordinary synthetic work.", Async: true}); err != nil {
		t.Fatalf("pending-history replay start failed: errorType=%T", err)
	}
	replayCtx, replayCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer replayCancel()
	select {
	case body := <-replayBody:
		assertTimedQuestionPendingProviderResult(t, body, receipt.Payload.CallID)
		assertTimedQuestionPromotedProviderReply(t, body, receipt.Payload.CallID, inputID, strings.TrimSpace(original))
	case <-replayCtx.Done():
		t.Fatal("new ordinary turn did not replay the actual durable timeout result")
	}
	if err := restarted.runtimeControl().WaitForTurnOperations(replayCtx); err != nil {
		t.Fatal("pending-history replay did not quiesce", err)
	}
	if restartedHTTPCalls.Load() != 1 {
		t.Fatal("replayed timeout caused an extra Provider step or re-executed Question")
	}
	assertTimedQuestionDurableResult(t, restarted, f.request.ThreadID, turnID, receipt.Payload.CallID)
	assertTimedQuestionSingleSteeredEvent(t, restarted, f.request.ThreadID, request.LateReply.ClientUserMessageID)
	events, err := restarted.store.LoadEventsSince(f.request.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	requested := 0
	for _, event := range events.Events {
		if stringField(event, "kind") == "user_input_requested" {
			requested++
		}
	}
	if requested != 1 {
		t.Fatal("replayed original Question produced another request")
	}
}

func waitTimedQuestionCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed Question production condition did not occur")
}

func readTimedQuestionPrivateDisplay(t *testing.T, h *runtimeServerHandler, selector displayport.SelectorV1) displayapp.DisplayV1 {
	t.Helper()
	body, _ := json.Marshal(selector)
	recorder := httptest.NewRecorder()
	h.UserInputLocalDisplayV1().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/local-display/user-input", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("private display unavailable: status=%d", recorder.Code)
	}
	var display displayapp.DisplayV1
	if json.Unmarshal(recorder.Body.Bytes(), &display) != nil || !displayapp.ValidateDisplayV1(display, selector) {
		t.Fatal("invalid private display DTO")
	}
	return display
}

func assertTimedQuestionLateReplyResponse(t *testing.T, result controlapp.ActionResult, inputID, attemptID, turnID, state string) {
	t.Helper()
	if result.StatusCode != http.StatusOK || len(result.Body) != 3 || stringField(result.Body, "inputId") != inputID || stringField(result.Body, "status") != "continued" {
		t.Fatal("late reply response lost its closed original request binding", result.StatusCode, stringField(result.Body, "code"))
	}
	delivery, ok := result.Body["lateReply"].(map[string]any)
	if !ok || len(delivery) != 3 || stringField(delivery, "clientUserMessageId") != attemptID || stringField(delivery, "turnId") != turnID || stringField(delivery, "state") != state {
		t.Fatal("late reply response lost its exact queued or accepted delivery")
	}
}

func assertTimedQuestionSingleSteeredEvent(t *testing.T, h *runtimeServerHandler, threadID, attemptID string) {
	t.Helper()
	replay, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range replay.Events {
		if stringField(event, "kind") == "turn_steered" && stringField(event, "clientUserMessageId") == attemptID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("late retry duplicated delivery event: count=%d", count)
	}
}

// Install the same real history/Registry authorities on both quiescent durable
// handler openings. This does not replace the production private-display guard.
func configureTimedQuestionHistoryAuthority(t *testing.T, first *runtimeServerHandler) func(*runtimeServerHandler) {
	t.Helper()
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(t.TempDir(), "case-contexts"))
	if err != nil {
		t.Fatal(err)
	}
	configure := func(h *runtimeServerHandler) {
		registry, err := casethreadapp.NewRegistry(context.Background(), key, caseStore)
		if err != nil {
			t.Fatal(err)
		}
		h.caseThreads = registry
		h.store.SetCaseThreadAuthority(registry)
		h.toolSnapshotHistoryValidate = func(ctx context.Context, thread map[string]any, original domainsecurity.TurnSecurityContext) error {
			return snapshotapp.ValidateHistoricalScopeV1(ctx, thread, original, snapshotapp.HistoricalScopeDependenciesV1{Primary: h.store, ValidateCurrentOrigin: protectedSeamCurrentOriginV1(h),
				ObserveCaseContexts: func(ctx context.Context) (*casethreadapp.VerifiedCommittedContextInventoryV1, error) {
					records, err := caseStore.List(ctx)
					if err != nil {
						return nil, err
					}
					return casethreadapp.VerifyCommittedContextInventoryV1(ctx, records, key)
				},
				ObserveChildren: func(ctx context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error) {
					inventory, err := h.pendingWork.TrustedInventoryV1(ctx)
					if err != nil {
						return inventory, nil, false, err
					}
					runs, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(h.dataDir, "child-runs"))
					return inventory, runs.Records, runs.HasLegacyTypeScript, err
				},
			})
		}
	}
	configure(first)
	return configure
}

func timedQuestionProviderMessages(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var request struct {
		Messages []map[string]any `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil || len(request.Messages) == 0 {
		t.Fatal("invalid actual Provider request")
	}
	return request.Messages
}

func assertTimedQuestionPendingProviderResult(t *testing.T, body []byte, callID string) {
	t.Helper()
	count := 0
	for _, message := range timedQuestionProviderMessages(t, body) {
		if stringField(message, "role") != "tool" || stringField(message, "tool_call_id") != callID {
			continue
		}
		count++
		var output map[string]any
		if json.Unmarshal([]byte(stringField(message, "content")), &output) != nil || len(output) != 3 || output["pending"] != true || stringField(output, "callId") != callID || stringField(output, "message") != "The question remains open for a later answer. Continue independent work; no answer or approval was supplied." {
			t.Fatal("actual continuation lost closed original-call pending result")
		}
	}
	if count != 1 {
		t.Fatalf("actual pending tool result pairing count=%d", count)
	}
}

func assertTimedQuestionDurableResult(t *testing.T, h *runtimeServerHandler, threadID, turnID, callID string) {
	t.Helper()
	thread, err := h.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") != "tool_result" || stringField(item, "callId") != callID {
				continue
			}
			count++
			output, _ := item["output"].(map[string]any)
			if stringField(item, "status") != "completed" || item["isError"] != false || stringField(item, "contextDigest") == "" || output["factAnswerAllowed"] != false || output["evidenceAuthority"] != false {
				t.Fatal("real timeout result lost durable settlement or granted answer authority")
			}
		}
	}
	if count != 1 {
		t.Fatalf("real timeout durable tool result count=%d", count)
	}
}

func assertTimedQuestionPromotedProviderReply(t *testing.T, body []byte, callID, inputID, custom string) {
	t.Helper()
	count := 0
	for _, message := range timedQuestionProviderMessages(t, body) {
		if stringField(message, "role") != "user" {
			continue
		}
		content := stringField(message, "content")
		if !strings.Contains(content, "answer_to_pending_question") {
			continue
		}
		prefix := strings.TrimSuffix(modelapp.SteeringProviderContent("payload", nil), "payload")
		if prefix == "" || !strings.HasPrefix(content, prefix) {
			t.Fatal("actual promoted reply lost existing mid-turn steering attribution")
		}
		payload := strings.TrimPrefix(content, prefix)
		var reply map[string]any
		if json.Unmarshal([]byte(payload), &reply) != nil || len(reply) != 5 || stringField(reply, "kind") != "answer_to_pending_question" || stringField(reply, "tool") != "request_user_input" || stringField(reply, "callId") != callID {
			t.Fatal("promoted answer lost original pending Question identity")
		}
		questions := listAny(reply["questions"])
		if len(questions) != 1 {
			t.Fatal("promoted original question batch changed")
		}
		question, _ := questions[0].(map[string]any)
		if stringField(question, "id") != inputID+"_1" || !strings.Contains(payload, callID) || !strings.Contains(payload, inputID+"_1") {
			t.Fatal("actual Provider envelope changed exact Host correlation bytes")
		}
		answers := listAny(reply["answers"])
		if len(answers) != 1 {
			t.Fatal("promoted answer batch changed")
		}
		answer, _ := answers[0].(map[string]any)
		if stringField(answer, "id") != inputID+"_1" || stringField(answer, "custom") != custom || len(listAny(answer["selected"])) != 0 {
			t.Fatal("actual promotion lost answer projection")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("actual promoted late answer count=%d", count)
	}
}

// The source here enters the real signed Host APIs directly and disposes the
// request as continued. It is not timer evidence. Queue, promotion, final HTTP
// Provider serialization and quiescent restart/replay below use production paths.
func TestLateQuestionSmallNumericHostIDsQueuePromoteAndReplay(t *testing.T) {
	for _, test := range []struct {
		name, callID    string
		numericQuestion bool
	}{
		{"safe", "call_host_" + strings.Repeat("a", 64), false},
		{"numeric_call", "call_host_" + strings.Repeat("a", 26) + "123456789012" + strings.Repeat("a", 26), false},
		{"numeric_question", "call_host_" + strings.Repeat("a", 64), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			startedProvider, promotedBody, release := make(chan struct{}), make(chan []byte, 1), make(chan struct{})
			var releaseOnce sync.Once
			f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if calls.Add(1) == 1 {
					close(startedProvider)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				} else {
					select {
					case promotedBody <- body:
					case <-r.Context().Done():
						return
					}
				}
				writeInlineCompletionFixtureReply(w)
			})
			h := f.handler
			h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
			configureHistory := configureTimedQuestionHistoryAuthority(t, h)
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := h.Shutdown(ctx); err != nil {
					t.Errorf("small admission shutdown failed: errorType=%T", err)
				}
			})
			started, err := h.startRuntimeTurn(context.Background(), f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Continue ordinary synthetic work.", Async: true})
			if err != nil {
				t.Fatalf("small admission start failed: errorType=%T", err)
			}
			turnID := stringField(started, "turnId")
			select {
			case <-startedProvider:
			case <-time.After(15 * time.Second):
				t.Fatal("small admission initial production Provider did not start")
			}
			frozen, err := appturn.LoadFrozenSecurityContext(h.store, f.request.ThreadID, turnID)
			if err != nil {
				t.Fatal("small admission frozen authority unavailable")
			}
			receipt := timedQuestionSignedAdmissionSource(t, h, frozen, test.callID, test.numericQuestion, 7, "")
			persistTimedQuestionSignedSourceProjection(t, h, receipt)
			inputID := receipt.Payload.GateID
			binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: receipt.Payload.ThreadID, TurnID: receipt.Payload.TurnID, ItemID: receipt.Payload.ItemID, InputID: inputID, ReceiptID: receipt.ReceiptID, SecurityContext: receipt.Payload.SecurityContext}
			var args map[string]any
			_ = json.Unmarshal(receipt.Payload.Arguments, &args)
			questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, inputID, "User input required")
			custom := "ordinary small synthetic answer"
			answer := controlapp.UserInputResponse{StructuredAnswers: []controlapp.UserInputAnswer{{ID: inputID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &custom}}}
			_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(answer, questions, "")
			if err != nil || submission == nil {
				t.Fatal("small capture normalization failed")
			}
			attempt := "00000000-0000-4000-8000-000000000083"
			capture, err := h.continuations.CaptureLateUserInputSubmissionHost(context.Background(), binding, attempt, *submission)
			if err != nil {
				t.Fatal("small private capture failed")
			}
			carrier, err := h.verifiedLateUserInputSteeringV1(context.Background(), binding, attempt, capture.ReceiptID, turnID)
			if err != nil {
				t.Fatal("small Host carrier failed")
			}
			result, err := h.runtimeControl().SteerTurn(context.WithValue(context.Background(), lateUserInputSteeringContextKeyV1{}, carrier), carrier.requestV1())
			if err != nil || result.StatusCode != http.StatusOK {
				t.Fatalf("legal small Host answer was not queued: status=%d code=%s errorType=%T", result.StatusCode, stringField(result.Body, "code"), err)
			}
			delivery, err := h.store.readUserInputLateReplyV1(f.request.ThreadID, inputID, attempt)
			if err != nil || delivery == nil || delivery.State != "queued" {
				t.Fatal("admission did not remain queued before promotion")
			}
			if strings.Contains(stringField(delivery.Entry, "text"), test.callID) || strings.Contains(stringField(delivery.Entry, "text"), inputID+"_1") {
				t.Fatal("Host identity entered persisted prose")
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case body := <-promotedBody:
				assertTimedQuestionPromotedProviderReply(t, body, test.callID, inputID, custom)
			case <-time.After(15 * time.Second):
				t.Fatal("small late answer did not reach production HTTP Provider")
			}
			waitTimedQuestionCondition(t, func() bool {
				d, err := h.store.readUserInputLateReplyV1(f.request.ThreadID, inputID, attempt)
				return err == nil && d != nil && d.State == "accepted"
			})
			waitTimedQuestionCondition(t, func() bool {
				thread, err := h.store.GetThread(f.request.ThreadID)
				if err != nil {
					return false
				}
				for _, raw := range listAny(thread["turns"]) {
					turn, _ := raw.(map[string]any)
					if stringField(turn, "id") == turnID {
						return stringField(turn, "status") == "completed"
					}
				}
				return false
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err = h.Shutdown(ctx)
			cancel()
			if err != nil {
				t.Fatal("small source did not quiesce before restart")
			}
			replayBody := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				select {
				case replayBody <- body:
				case <-r.Context().Done():
					return
				}
				writeInlineCompletionFixtureReply(w)
			}))
			t.Cleanup(upstream.Close)
			restarted := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: f.durableRoot, DataDir: h.dataDir, ProviderID: "openai", BaseURL: upstream.URL + "/v1", APIKey: "synthetic-fixture-key", Model: "fixture-model", EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
			configureServerGeneralExecution(t, restarted)
			risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
			if err != nil {
				t.Fatal(err)
			}
			restarted.turnSecurity.RiskAuthority = risk
			configureHistory(restarted)
			installHandlerProviderExecutionResolverForTest(restarted)
			restarted.provider = h.provider
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := restarted.Shutdown(ctx); err != nil {
					t.Errorf("small replay shutdown failed: errorType=%T", err)
				}
			})
			if _, err := restarted.startRuntimeTurn(context.Background(), f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Continue independent ordinary synthetic work.", Async: true}); err != nil {
				t.Fatalf("small replay start failed: errorType=%T", err)
			}
			select {
			case body := <-replayBody:
				assertTimedQuestionPromotedProviderReply(t, body, test.callID, inputID, custom)
			case <-time.After(15 * time.Second):
				t.Fatal("quiescent restart did not replay exact late IDs into production Provider")
			}
			assertTimedQuestionSingleSteeredEvent(t, restarted, f.request.ThreadID, attempt)
		})
	}
}

// This fixture issues a real legacy (untimed) Host Question and persists its
// verified request through the existing item/outbox owners. It creates no
// timer, model reply or handwritten continued state.
func privateQuestionDraftPendingFixtureV1(t *testing.T) (*runtimeServerHandler, runtimePendingToolCall, domaincontinuation.Receipt) {
	t.Helper()
	h, pending, _, _ := protectedSeamFixtureV1(t, "request_user_input", map[string]any{"prompt": "Two synthetic questions", "questions": []any{map[string]any{"question": "First synthetic question", "options": []any{map[string]any{"label": "One"}, map[string]any{"label": "Two"}}, "multi_select": true}, map[string]any{"question": "Second synthetic question"}}})
	pending.ProviderConfig = domainmodel.TurnConfig{ProviderID: "synthetic", Model: "synthetic", APIKey: "synthetic-only", BaseURL: "http://127.0.0.1:1/v1", EndpointFormat: "chat_completions"}
	pending.LogicalEffect = domainsecurity.LogicalEffectOrdinary
	pending.OrdinaryWork = true
	pending.ProviderStepExact = true
	pending.OrdinaryResultInputIsolated = true
	pending.ProviderNamespace = domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	inputID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := h.continuations.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, time.Now().UTC())
	if err != nil {
		t.Fatal("legacy source issuance failed", err)
	}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.EnsureGateRequestItemExact(pending.ThreadID, pending.TurnID, projection.Item); err != nil {
		t.Fatal(err)
	}
	if err := h.recordPendingGateRequest(projection.Event); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if h.runtimeControl().ShuttingDown() {
			return
		}
		_, _ = h.runtimeControl().InterruptTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: pending.ThreadID, TurnID: pending.TurnID})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Errorf("draft fixture shutdown errorType=%T", err)
		}
	})
	return h, pending, source
}

func writeSyntheticQuestionCallV1(w http.ResponseWriter, name, arguments string) {
	w.Header().Set("Content-Type", "text/event-stream")
	chunk := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "synthetic-original-question", "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}}}, "finish_reason": "tool_calls"}}}
	body, _ := json.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
}

func productionQuestionSelectorV1(t *testing.T, h *runtimeServerHandler, threadID, turnID string) displayport.SelectorV1 {
	t.Helper()
	var selector displayport.SelectorV1
	waitTimedQuestionCondition(t, func() bool {
		events, err := h.store.LoadEventsSince(threadID, 0)
		if err != nil {
			return false
		}
		for _, event := range events.Events {
			if stringField(event, "kind") == "user_input_requested" && stringField(event, "turnId") == turnID {
				selector = displayport.SelectorV1{ThreadID: threadID, TurnID: turnID, ItemID: stringField(event, "itemId"), RequestID: stringField(event, "inputId")}
				return true
			}
		}
		return false
	})
	return selector
}

func assertQuestionWakeOnlyAdmittedHumanV1(t *testing.T, h *runtimeServerHandler, threadID, turnID string, body []byte) {
	t.Helper()
	for _, message := range timedQuestionProviderMessages(t, body) {
		if stringField(message, "role") == "user" && strings.TrimSpace(stringField(message, "content")) == "" {
			t.Fatal("Host wake injected a fake empty human Provider message")
		}
	}
	thread, err := h.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := modelapp.TurnByID(thread, turnID)
	if !ok {
		t.Fatal("real wake turn missing")
	}
	humans := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "user_message" {
			humans++
			if stringField(item, "clientUserMessageId") == "" {
				t.Fatal("Host wake invented ordinary human item")
			}
		}
	}
	if humans != 1 {
		t.Fatalf("wake actual admitted human count=%d", humans)
	}
}

func TestTimedQuestionProductionIdleLateReplyStartsNormallyBeforeProvider(t *testing.T) {
	var calls atomic.Int32
	independentBody := make(chan []byte, 1)
	wakeBody := make(chan []byte, 1)
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		call := calls.Add(1)
		if call == 1 {
			writeSyntheticQuestionCallV1(w, "request_user_input", `{"prompt":"Optional synthetic question","questions":[{"question":"Choose later?"}],"timeout":1}`)
			return
		}
		if call == 2 {
			independentBody <- body
		}
		if call == 3 {
			wakeBody <- body
		}
		writeInlineCompletionFixtureReply(w)
	})
	h := f.handler
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureTimedQuestionHistoryAuthority(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	t.Cleanup(func() {
		if err := h.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Ask an optional question and finish independent ordinary work.", Async: true})
	if err != nil {
		t.Fatal(err)
	}
	originalTurnID := stringField(started, "turnId")
	selector := productionQuestionSelectorV1(t, h, f.request.ThreadID, originalTurnID)
	select {
	case <-independentBody:
	case <-ctx.Done():
		t.Fatal("actual foreground timeout did not resume independent production Provider work")
	}
	if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("foreground timeout did not finish independent work")
	}
	thread, _, err := h.store.ReadThreadStartBaseline(selector.ThreadID)
	if err != nil || !runtimeLateQuestionIdleAnchorV1(thread, selector.ThreadID, originalTurnID) {
		t.Fatal("real original turn did not become idle")
	}
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, selector.RequestID)
	if err != nil || disposition.ReasonCode != domaincontinuation.ReasonUserInputContinuedV1 {
		t.Fatal("fixture did not use actual timer", err)
	}
	answer := "actual synthetic idle answer"
	request := controlapp.UserInputResponse{InputID: selector.RequestID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: selector.RequestID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &answer}}, LateReply: &controlapp.UserInputLateReplyV1{ClientUserMessageID: "00000000-0000-4000-8000-000000000081", ExpectedTurnID: originalTurnID}}
	result, err := h.runtimeControl().RespondUserInput(ctx, request)
	if err != nil || result.StatusCode != http.StatusOK {
		t.Fatalf("idle late normal admission failed: status=%d errorType=%T", result.StatusCode, err)
	}
	delivery, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, request.LateReply.ClientUserMessageID)
	if err != nil || delivery == nil || delivery.TurnID == originalTurnID {
		t.Fatal("idle answer did not allocate a real new turn", err)
	}
	newTurnID := delivery.TurnID
	var actual []byte
	select {
	case actual = <-wakeBody:
	case <-ctx.Done():
		t.Fatal("idle normal admitted answer did not reach first production Provider request")
	}
	if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
		t.Fatal(err)
	}
	assertTimedQuestionPendingProviderResult(t, actual, receipt.Payload.CallID)
	assertTimedQuestionPromotedProviderReply(t, actual, receipt.Payload.CallID, selector.RequestID, answer)
	assertQuestionWakeOnlyAdmittedHumanV1(t, h, selector.ThreadID, newTurnID, actual)
	retry, err := h.runtimeControl().RespondUserInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertTimedQuestionLateReplyResponse(t, retry, selector.RequestID, request.LateReply.ClientUserMessageID, newTurnID, "accepted")
	assertTimedQuestionSingleSteeredEvent(t, h, selector.ThreadID, request.LateReply.ClientUserMessageID)
	if calls.Load() != 3 {
		t.Fatal("idle exact retry started a second worker")
	}
}

type questionProcessCrashManifestV1 struct {
	DurableRoot, DataDir, ThreadID, TurnID, CallID string
	Selector                                       displayport.SelectorV1
}

// This self-reentering fixture exits an actual process with a real native -1
// Question paused and unsatisfied. No Shutdown or fabricated continued state
// is the input to recovery. Every path is beneath the parent's synthetic temp.
func TestTimedQuestionProductionUnknownAfterProcessCrashAndIdleWake(t *testing.T) {
	const childEnv = "ANALYTIX_QUESTION_CRASH_CHILD_ROOT"
	if root := os.Getenv(childEnv); root != "" {
		f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
			writeSyntheticQuestionCallV1(w, "request_user_input", `{"prompt":"Question interrupted by process exit","questions":[{"question":"Answer after restart?"}],"timeout":-1}`)
		})
		h := f.handler
		h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
		configureTimedQuestionHistoryAuthority(t, h)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Ask an optional unlimited synthetic question.", Async: true})
		if err != nil {
			t.Fatal(err)
		}
		turnID := stringField(started, "turnId")
		selector := productionQuestionSelectorV1(t, h, f.request.ThreadID, turnID)
		if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
			t.Fatal(err)
		}
		inventory, err := h.continuations.TrustedInventoryV1(ctx)
		if err != nil || len(inventory.Receipts) != 1 || len(inventory.Dispositions) != 0 {
			t.Fatal("real paused Question was already disposed", err)
		}
		receipt := inventory.Receipts[0]
		thread, err := h.store.GetThread(f.request.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		registry, err := executiongrantapp.RegistryFromThread(f.request.ThreadID, thread, turnID)
		if err != nil {
			t.Fatal(err)
		}
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, receipt.Payload.ExecutionGrant.GrantID)
		if !found || entry.Status != domainsecurity.GrantRegistryActive || entry.Grant.ReadOnly || receipt.Payload.UserInputTimedWait == nil || receipt.Payload.UserInputTimedWait.TimeoutSeconds != -1 {
			t.Fatal("fixture lacked actual writable native paused dispatch")
		}
		turn, _ := modelapp.TurnByID(thread, turnID)
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == receipt.Payload.CallID {
				t.Fatal("child already had a terminal tool result")
			}
		}
		manifest := questionProcessCrashManifestV1{DurableRoot: f.durableRoot, DataDir: h.dataDir, ThreadID: f.request.ThreadID, TurnID: turnID, CallID: receipt.Payload.CallID, Selector: selector}
		for _, path := range []string{manifest.DurableRoot, manifest.DataDir} {
			rel, err := filepath.Rel(root, path)
			if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				t.Fatal("synthetic child escaped task root")
			}
		}
		body, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(root, "manifest.json"), body, 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	root := workspacetest.New(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, exe, "-test.run=^TestTimedQuestionProductionUnknownAfterProcessCrashAndIdleWake$", "-test.timeout=30s")
	for _, name := range []string{"home", "config", "tmp"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(root, "home")
	config := filepath.Join(root, "config")
	temporary := filepath.Join(root, "tmp")
	// Bind the complete inherited TestMain isolation profile for this child.
	// Parent environment and the production restore guards remain unchanged.
	command.Env = append(os.Environ(), childEnv+"="+root,
		"ANALYTIX_GO_TEST_USER_CONFIG_ROOT="+root,
		"HOME="+home, "USERPROFILE="+home,
		"XDG_CONFIG_HOME="+config, "APPDATA="+config, "LOCALAPPDATA="+config,
		"TMPDIR="+temporary, "TMP="+temporary, "TEMP="+temporary, "GOTMPDIR="+temporary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("actual paused Question child failed: %v %s", err, output)
	}
	body, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest questionProcessCrashManifestV1
	if json.Unmarshal(body, &manifest) != nil {
		t.Fatal("invalid task-owned crash manifest")
	}
	wakeBody := make(chan []byte, 2)
	var calls atomic.Int32
	providerFixture := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls.Add(1)
		wakeBody <- body
		writeInlineCompletionFixtureReply(w)
	})
	base := providerFixture.handler
	configureHistory := configureTimedQuestionHistoryAuthority(t, base)
	t.Cleanup(func() {
		if err := base.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	baseURL := base.providerConfig.TurnConfig("openai", "fixture-model").BaseURL
	open := func() *runtimeServerHandler {
		config := RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, Host: "127.0.0.1", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), DurableTempDir: manifest.DurableRoot, DataDir: manifest.DataDir, ProviderID: "openai", BaseURL: baseURL, APIKey: "synthetic-fixture-key", Model: "fixture-model", EndpointFormat: "chat_completions"}
		store, err := newRuntimeEventSessionStore(config)
		if err != nil {
			t.Fatal(err)
		}
		primary, err := finalauthority.NewAcceptedFinalCASReader(store.root)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.BindPrimaryThreadReaderV1(primary); err != nil {
			t.Fatal(err)
		}
		signer, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(manifest.DataDir, "private", "authority", "continuation-ed25519-v1.json"), true)
		if err != nil {
			t.Fatal(err)
		}
		continuations, err := newServerTestContinuationService(t, filepath.Join(manifest.DataDir, "private", "gate-continuations"), signer)
		if err != nil {
			t.Fatal(err)
		}
		pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(manifest.DataDir, "private", "pending-work"))
		if err != nil {
			t.Fatal(err)
		}
		state := subagentapp.NewRuntimeState()
		native, err := nativecomponentapp.NewUnavailableRuntimeAuthority(nativecomponentapp.HealthDependencies{Store: store, DurableAuthority: compatibilityNativeDurableAuthority{}, AcquireEffect: state.AcquireContextEffect, Now: time.Now})
		if err != nil {
			t.Fatal(err)
		}
		// Each reopen owns a fresh manager so production composition binds the
		// current handler's managed-editing authority before connecting.
		manager := mcp.NewProductionManagerWithOptions(nil, mcp.ProductionManagerOptions{CacheDir: filepath.Join(manifest.DataDir, "mcp-schema-cache"), ProtectedReadDirs: base.protectedReadDirs})
		// The production composer restores before returning. Original stores and
		// installation signer, plus the legal empty General lineage registry,
		// are installed before that call; no restore guard is patched or skipped.
		value, err := NewRuntimeServerHandlerFromComponents(config, RuntimeServerComponents{
			UserInputConfig: toolcatalogapp.UserInputConfig{Mode: "timed"}, Store: store, CaseThreads: base.caseThreads, TurnSecurity: base.turnSecurity,
			Attachments: base.attachments, AttachmentAccess: base.attachmentAccess, AttachmentUses: base.attachmentUses,
			// This fixture has no attachments or child jobs, but the normal
			// worker still requires the same ordinary runtime services as base.
			Jobs: base.jobs, ChildCompletions: base.childCompletions, ForegroundHandoffs: base.foregroundHandoffs,
			MCP: manager, MCPSearch: base.mcpSearch, Skills: base.skills, Subagents: base.subagents, AutoResearch: base.autoResearch,
			ToolResultSnapshots: base.toolSnapshots, ToolResultSnapshotHistoryValidate: base.toolSnapshotHistoryValidate,
			CommandProbe: base.commandProbe, CommandHomeDir: base.commandHomeDir, WorkspaceProbe: base.workspaceProbe,
			ShellRunner: base.shellRunner, GitStatusProbe: base.gitStatusProbe, WorktreeManager: base.worktreeManager,
			AllowWriteRoots: base.allowWriteRoots, ProtectedReadDirs: base.protectedReadDirs,
			Web: base.web, VisionBridge: base.visionBridge,
			Continuations: continuations, PendingWork: pendingworkapp.NewService(signer, pendingStore, store), SteeringAuthority: steeringauthorityapp.NewService(signer),
			CaseFinalizer: base.caseFinalizer, Checkpoints: base.checkpoints, Memories: base.memories, Provider: base.provider, ProviderConfig: base.providerConfig,
			NativeAuthority: native, SubagentState: state, G6Readiness: base.g6Readiness, StepLimits: base.stepLimits, ApprovalPolicy: base.approvalPolicy, SandboxMode: base.sandboxMode,
		})
		if err != nil {
			t.Fatal(err)
		}
		h := value.(*runtimeServerHandler)
		configureHistory(h)
		installHandlerProviderExecutionResolverForTest(h)
		return h
	}
	h := open()
	t.Cleanup(func() {
		if err := h.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, manifest.Selector.RequestID)
	if err != nil || disposition.Status != domaincontinuation.StatusContinued || disposition.ReasonCode != domaincontinuation.ReasonUserInputOutcomeUnknownAfterRestartV1 || disposition.UserInputSubmission != nil {
		t.Fatal("real process interruption was not closed as signed Question unknown", err)
	}
	assertUnknown := func(h *runtimeServerHandler) {
		thread, err := h.store.GetThread(manifest.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		turn, _ := modelapp.TurnByID(thread, manifest.TurnID)
		if stringField(turn, "status") != "aborted" {
			t.Fatal("old crashed turn stayed live")
		}
		count := 0
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") != "tool_result" || stringField(item, "callId") != manifest.CallID {
				continue
			}
			count++
			projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
			if err != nil || projection != domaintoolresult.OutcomeUnknownAfterRestartProjectionV1() || item["isError"] != true || stringField(item, "status") != "failed" || item["hostEvidenceSettlement"] != nil {
				t.Fatal("restart fabricated successful pending, cancellation or evidence")
			}
		}
		if count != 1 {
			t.Fatalf("real unknown result count=%d", count)
		}
		registry, err := executiongrantapp.RegistryFromThread(manifest.ThreadID, thread, manifest.TurnID)
		if err != nil || domainsecurity.VerifyExecutionGrantMembership(registry, manifest.ThreadID, manifest.TurnID, receipt.Payload.ExecutionGrant, domainsecurity.GrantRegistrySettled) != nil {
			t.Fatal("old unknown grant was not exactly settled", err)
		}
	}
	assertUnknown(h)
	display := readTimedQuestionPrivateDisplay(t, h, manifest.Selector)
	if display.Status != "continued" || display.ContinuedReason == nil || *display.ContinuedReason != domaincontinuation.ReasonUserInputOutcomeUnknownAfterRestartV1 || display.TimedWait == nil || display.TimedWait.TimeoutSeconds != -1 || display.AnswerState != "unavailable" {
		t.Fatal("actual unlimited interrupted request could not reopen")
	}
	answer := "actual answer after native process exit"
	request := controlapp.UserInputResponse{InputID: manifest.Selector.RequestID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: manifest.Selector.RequestID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &answer}}, LateReply: &controlapp.UserInputLateReplyV1{ClientUserMessageID: "00000000-0000-4000-8000-000000000091", ExpectedTurnID: manifest.TurnID}}
	result, err := h.runtimeControl().RespondUserInput(ctx, request)
	if err != nil || result.StatusCode != http.StatusOK {
		t.Fatalf("actual interrupted original late admission failed: status=%d errorType=%T", result.StatusCode, err)
	}
	var actual []byte
	select {
	case actual = <-wakeBody:
	case <-ctx.Done():
		t.Fatal("real crash late answer did not reach production Provider")
	}
	if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
		t.Fatal(err)
	}
	delivery, err := h.store.readUserInputLateReplyV1(manifest.ThreadID, manifest.Selector.RequestID, request.LateReply.ClientUserMessageID)
	if err != nil || delivery == nil || delivery.TurnID == manifest.TurnID || delivery.State != "accepted" {
		t.Fatal("late answer did not enter real new-turn promotion", err)
	}
	assertTimedQuestionPromotedProviderReply(t, actual, manifest.CallID, manifest.Selector.RequestID, answer)
	assertQuestionWakeOnlyAdmittedHumanV1(t, h, manifest.ThreadID, delivery.TurnID, actual)
	unknownPair := 0
	for _, message := range timedQuestionProviderMessages(t, actual) {
		if stringField(message, "role") == "tool" && stringField(message, "tool_call_id") == manifest.CallID {
			unknownPair++
			if !strings.Contains(stringField(message, "content"), "tool_outcome_unknown_after_restart") || strings.Contains(stringField(message, "content"), `"pending":true`) {
				t.Fatal("unknown history was disguised as successful timeout pending")
			}
		}
	}
	if unknownPair != 1 {
		t.Fatal("unknown original tool pairing lost")
	}
	if err := h.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	h = open()
	assertUnknown(h)
	_, again, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, manifest.Selector.RequestID)
	if err != nil || again.DispositionID != disposition.DispositionID {
		t.Fatal("repeat recovery changed original signed outcome", err)
	}
	retry, err := h.runtimeControl().RespondUserInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertTimedQuestionLateReplyResponse(t, retry, request.InputID, request.LateReply.ClientUserMessageID, delivery.TurnID, "accepted")
	assertTimedQuestionSingleSteeredEvent(t, h, manifest.ThreadID, request.LateReply.ClientUserMessageID)
	if calls.Load() != 1 {
		t.Fatal("restart/retry resumed old Question or duplicated new worker")
	}
}

func TestPlanReviewProductionUsesSignedOriginalIntentAndApproval(t *testing.T) {
	plan := "# Full synthetic plan\n\nFirst inspect; then make the authorized synthetic edit.\n"
	var calls atomic.Int32
	nextBody := make(chan []byte, 1)
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if calls.Add(1) == 1 {
			args, _ := json.Marshal(map[string]any{"plan": plan})
			writeSyntheticQuestionCallV1(w, "exit_plan_mode", string(args))
			return
		}
		nextBody <- body
		writeInlineCompletionFixtureReply(w)
	})
	h := f.handler
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureTimedQuestionHistoryAuthority(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Cleanup(func() {
		if err := h.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "Present the complete synthetic plan for review.", Mode: "plan", Async: true})
	if err != nil {
		t.Fatal(err)
	}
	turnID := stringField(started, "turnId")
	selector := productionQuestionSelectorV1(t, h, f.request.ThreadID, turnID)
	if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
		t.Fatal(err)
	}
	display := readTimedQuestionPrivateDisplay(t, h, selector)
	if display.TimedWait != nil || display.CallID == nil || len(display.Questions) != 1 || display.Questions[0].Intent == nil || display.Questions[0].Detail == nil || *display.Questions[0].Detail != plan {
		t.Fatal("actual review lacked original full plan intent or became timed")
	}
	thread, err := h.store.GetThread(selector.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := h.continuations.TrustedInventoryV1(ctx)
	if err != nil || len(inventory.Receipts) != 1 {
		t.Fatal("actual plan review receipt missing", err)
	}
	original := inventory.Receipts[0]
	checked := false
	reason := h.continuations.RestartDispositionReason(selector.RequestID, original.ReceiptID, domaincontinuation.KindUserInput, selector.ThreadID, selector.TurnID, selector.ItemID, thread, h.providerConfig, func(pending modelapp.PendingToolCall) error {
		checked = true
		if pending.Call.Name != "exit_plan_mode" || pending.Call.ID != *display.CallID || !bytes.Equal(pending.Call.Arguments, original.Payload.Arguments) {
			return fmt.Errorf("plan review reconstruction changed immutable original call")
		}
		return nil
	})
	if !checked || reason != "restart_revalidation_passed_nonresumable" {
		t.Fatalf("real paused plan review could not reconstruct its original gate: reason=%s", reason)
	}
	question := display.Questions[0]
	request := controlapp.UserInputResponse{InputID: selector.RequestID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: question.ID, Kind: "answer", OptionIDs: []string{question.Intent.ApproveOptionID}}}}
	result, err := h.runtimeControl().RespondUserInput(ctx, request)
	if err != nil || result.StatusCode != http.StatusOK {
		t.Fatalf("actual plan approval failed: status=%d errorType=%T", result.StatusCode, err)
	}
	var actual []byte
	select {
	case actual = <-nextBody:
	case <-ctx.Done():
		t.Fatal("review approval did not reach next production step")
	}
	if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
		t.Fatal(err)
	}
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, selector.RequestID)
	if err != nil || receipt.Payload.ToolName != "exit_plan_mode" || receipt.Payload.CallID != *display.CallID || !toolcatalogapp.PlanReviewApprovedSubmissionV1(receipt.Payload.ToolName, selector.RequestID, disposition.UserInputSubmission) {
		t.Fatal("approval lost original signed call/selection", err)
	}
	approved := 0
	for _, message := range timedQuestionProviderMessages(t, actual) {
		if stringField(message, "role") == "tool" && stringField(message, "tool_call_id") == receipt.Payload.CallID {
			var output map[string]any
			if json.Unmarshal([]byte(stringField(message, "content")), &output) != nil || output["approved"] != true {
				t.Fatal("next production step did not receive actual approved result")
			}
			approved++
		}
	}
	if approved != 1 {
		t.Fatal("original approved tool result did not pair once")
	}
	var decoded struct {
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	if json.Unmarshal(actual, &decoded) != nil {
		t.Fatal("invalid actual next request")
	}
	for _, tool := range decoded.Tools {
		if tool.Function.Name == "exit_plan_mode" || tool.Function.Name == "create_plan" {
			t.Fatal("actual approved next step stayed in plan-mode tool catalog")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("plan review caused unexpected extra Provider step")
	}
}

// The real finite wait, HTTP capture, signed queue, promotion and Provider
// history must preserve complete legal batches. Aggregate wrappers do not gain
// prose-field limits; actual leaf, turn and encoded-event limits still apply.
func TestTimedQuestionProductionBatchCapacityIsRecoverable(t *testing.T) {
	for _, fixture := range []struct {
		name                string
		size                int
		escaped             bool
		contextWindowTokens int
	}{
		{"400000", 400000, false, syntheticQuestionContextWindowV1},
		{"900000", 900000, false, syntheticQuestionContextWindowV1},
		{"escaped_event_overflow", 400000, true, syntheticQuestionContextWindowV1},
		{"undeclared_context_window", 400000, false, 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			runTimedQuestionProductionBatchCapacityV1(t, fixture.size, fixture.escaped, fixture.contextWindowTokens)
		})
	}
}

func postTimedQuestionBatchHTTPV1(t *testing.T, baseURL string, request controlapp.UserInputResponse) controlapp.ActionResult {
	t.Helper()
	wireAnswers := make([]map[string]any, 0, len(request.StructuredAnswers))
	for _, answer := range request.StructuredAnswers {
		wire := map[string]any{"id": answer.ID, "kind": answer.Kind}
		if answer.Kind == "answer" {
			options := answer.OptionIDs
			if options == nil {
				options = []string{}
			}
			wire["optionIds"] = options
			if answer.CustomText != nil {
				wire["customText"] = *answer.CustomText
			}
		}
		wireAnswers = append(wireAnswers, wire)
	}
	body, err := json.Marshal(map[string]any{"answers": wireAnswers, "lateReply": request.LateReply})
	if err != nil {
		t.Fatal(err)
	}
	post, err := http.NewRequest(http.MethodPost, baseURL+"/v1/user-inputs/"+request.InputID, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	post.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	post.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if json.NewDecoder(response.Body).Decode(&result) != nil {
		t.Fatal("non-JSON Host response", response.StatusCode)
	}
	if response.StatusCode == http.StatusConflict {
		messages := map[string]string{
			"timed_wait_unavailable": "The timed Question foreground wait is unavailable.",
			"late_reply_too_large":   "The complete Question reply exceeds the current turn resource capacity. Original answers remain saved privately; edit and submit a new attempt.",
			"late_reply_unavailable": "The late Question reply is unavailable under current host authority.",
		}
		if expected, known := messages[stringField(result, "code")]; known && (len(result) != 2 || result["message"] != expected) {
			t.Fatal("actual HTTP Question failure did not preserve the exact closed message and two-field envelope")
		}
	}
	return controlapp.ActionResult{StatusCode: response.StatusCode, Body: result}
}

func runTimedQuestionProductionBatchCapacityV1(t *testing.T, size int, escaped bool, contextWindowTokens int) {
	twoSources := size == 900000 && !escaped
	independentCall, promotedCall := int32(2), int32(3)
	if twoSources {
		independentCall, promotedCall = 3, 4
	}
	var calls atomic.Int32
	independentBody, promotedBody := make(chan []byte, 1), make(chan []byte, 1)
	release := make(chan struct{})
	var once sync.Once
	arguments := `{"prompt":"Optional three-question batch","questions":[{"question":"First detail?"},{"question":"Second detail?"},{"question":"Third detail?"}],"timeout":1}`
	f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		call := calls.Add(1)
		if call == 1 {
			writeSyntheticQuestionCallV1(w, "request_user_input", arguments)
			return
		}
		if twoSources && call == 2 {
			// A second real dispatch and timer supply a different source.
			w.Header().Set("Content-Type", "text/event-stream")
			chunk := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "synthetic-second-question", "type": "function", "function": map[string]any{"name": "request_user_input", "arguments": arguments}}}}, "finish_reason": "tool_calls"}}}
			encoded, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
			return
		}
		if call == independentCall {
			independentBody <- body
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		if call == promotedCall {
			promotedBody <- body
		}
		writeInlineCompletionFixtureReply(w)
	})
	h := f.handler
	configureSyntheticQuestionModelV1(t, h, contextWindowTokens)
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureHistory := configureTimedQuestionHistoryAuthority(t, h)
	extendedOperation := twoSources && contextWindowTokens == syntheticQuestionContextWindowV1
	operationBudget := 90 * time.Second
	if extendedOperation {
		operationBudget = 240 * time.Second
	}
	operationStarted := time.Now()
	phase, phaseStarted, phaseBudget := "initial", operationStarted, operationBudget
	logStage := func(stage string) {
		if extendedOperation {
			t.Logf("synthetic900 phase=%s stage=%s phaseElapsedSeconds=%.3f totalElapsedSeconds=%.3f phaseBudgetSeconds=%.0f", phase, stage, time.Since(phaseStarted).Seconds(), time.Since(operationStarted).Seconds(), phaseBudget.Seconds())
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), operationBudget)
	defer cancel()
	defer logStage("operation_finished")
	logStage("operation_started")
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		if err := h.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	startPrompt := "Ask the optional batch and continue independent ordinary work."
	if twoSources {
		// Normal start persists prompt and user-item text independently; the
		// sidecar clears both. A Provider draft is not durable occupancy.
		startPrompt += strings.Repeat("d", 400000)
	}
	started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: startPrompt, Model: syntheticQuestionCapacityModelV1, Async: true})
	if err != nil {
		t.Fatal(err)
	}
	logStage("turn_started")
	turnID := stringField(started, "turnId")
	selector := productionQuestionSelectorV1(t, h, f.request.ThreadID, turnID)
	select {
	case <-independentBody:
	case <-ctx.Done():
		t.Fatal("real timer did not resume production work")
	}
	logStage("timers_resumed_independent_provider")
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, selector.RequestID)
	if err != nil || disposition.ReasonCode != domaincontinuation.ReasonUserInputContinuedV1 {
		t.Fatal("missing actual signed timeout", err)
	}
	var secondSelector displayport.SelectorV1
	if twoSources {
		events, err := h.store.LoadEventsSince(selector.ThreadID, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events.Events {
			if stringField(event, "kind") == "user_input_requested" && stringField(event, "inputId") != selector.RequestID {
				secondSelector = displayport.SelectorV1{ThreadID: selector.ThreadID, TurnID: turnID, ItemID: stringField(event, "itemId"), RequestID: stringField(event, "inputId")}
			}
		}
		_, secondDisposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, secondSelector.RequestID)
		if err != nil || secondDisposition.ReasonCode != domaincontinuation.ReasonUserInputContinuedV1 {
			t.Fatal("second Question did not really time out")
		}
	}
	logStage("timeout_receipts_verified")
	answers := make([]controlapp.UserInputAnswer, 3)
	for i := range answers {
		atom := string(rune('a' + i))
		if escaped {
			atom = "\""
		}
		value := strings.Repeat(atom, size)
		if privacyprojectionapp.ProjectOrdinaryText(value) != value {
			t.Fatal("fixture leaf is changed by ordinary projection")
		}
		answers[i] = controlapp.UserInputAnswer{ID: fmt.Sprintf("%s_%d", selector.RequestID, i+1), Kind: "answer", OptionIDs: []string{}, CustomText: &value}
	}
	request := controlapp.UserInputResponse{InputID: selector.RequestID, StructuredAnswers: answers, LateReply: &controlapp.UserInputLateReplyV1{ClientUserMessageID: "00000000-0000-4000-8000-0000000000a1", ExpectedTurnID: turnID}}
	listener := httptest.NewServer(h)
	t.Cleanup(listener.Close)
	primaryBefore, err := os.ReadFile(h.store.threadPath(selector.ThreadID))
	if err != nil {
		t.Fatal(err)
	}
	if twoSources {
		assertTimedQuestionDurableOccupancyV1(t, primaryBefore, turnID, startPrompt)
	}
	eventsBefore, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := postTimedQuestionBatchHTTPV1(t, listener.URL, request)
	if result.StatusCode != http.StatusOK {
		// Diagnose only through the same normal Host action and UUID retry.
		// Never print private bodies, raw messages or scanner input.
		private := readTimedQuestionPrivateDisplay(t, h, selector)
		assertTimedQuestionPrivateBatchV1(t, private.Submission, answers)
		diagnostic, diagnosticErr := h.runtimeControl().RespondUserInput(ctx, request)
		t.Logf("actual HTTP refusal with complete private capture; Host same-UUID retry status=%d code=%s errorType=%T", diagnostic.StatusCode, stringField(diagnostic.Body, "code"), diagnosticErr)
	}
	if escaped {
		if result.StatusCode != http.StatusConflict || stringField(result.Body, "code") != "late_reply_too_large" {
			t.Fatalf("actual escaped capacity: status=%d code=%s", result.StatusCode, stringField(result.Body, "code"))
		}
		primaryAfter, err := os.ReadFile(h.store.threadPath(selector.ThreadID))
		if err != nil || !bytes.Equal(primaryBefore, primaryAfter) {
			t.Fatal("capacity changed primary")
		}
		if delivery, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, request.LateReply.ClientUserMessageID); err != nil || delivery != nil {
			t.Fatal("capacity admitted a queue effect", err)
		}
		eventsAfter, err := h.store.LoadEventsSince(selector.ThreadID, 0)
		if err != nil || len(eventsBefore.Events) != len(eventsAfter.Events) || calls.Load() != independentCall {
			t.Fatal("capacity emitted or cancelled work", err)
		}
		display := readTimedQuestionPrivateDisplay(t, h, selector)
		assertTimedQuestionPrivateBatchV1(t, display.Submission, answers)
		t.Log("actual encoded-event overflow refused before primary, queue or event; complete private capture retained")
		return
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("real legal whole Question must enter queue: size=%d status=%d code=%s", size, result.StatusCode, stringField(result.Body, "code"))
	}
	assertTimedQuestionLateReplyResponse(t, result, selector.RequestID, request.LateReply.ClientUserMessageID, turnID, "queued")
	retry := postTimedQuestionBatchHTTPV1(t, listener.URL, request)
	assertTimedQuestionLateReplyResponse(t, retry, selector.RequestID, request.LateReply.ClientUserMessageID, turnID, "queued")
	assertTimedQuestionSingleSteeredEvent(t, h, selector.ThreadID, request.LateReply.ClientUserMessageID)
	logStage("first_admission_queued_and_retry_verified")
	if size == 900000 {
		before, err := os.ReadFile(h.store.threadPath(selector.ThreadID))
		if err != nil {
			t.Fatal(err)
		}
		assertTimedQuestionDurableOccupancyV1(t, before, turnID, startPrompt)
		pending, pendingErr := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, request.LateReply.ClientUserMessageID)
		if pendingErr != nil || pending == nil || pending.State != "queued" {
			t.Fatal("second source preflight did not preserve the first actual pending batch")
		}
		beforeEvents, err := h.store.LoadEventsSince(selector.ThreadID, 0)
		if err != nil {
			t.Fatal(err)
		}
		secondAnswers := make([]controlapp.UserInputAnswer, 3)
		for index := range secondAnswers {
			value := strings.Repeat(string(rune('a'+index)), 450000)
			secondAnswers[index] = controlapp.UserInputAnswer{ID: fmt.Sprintf("%s_%d", secondSelector.RequestID, index+1), Kind: "answer", OptionIDs: []string{}, CustomText: &value}
		}
		second := controlapp.UserInputResponse{InputID: secondSelector.RequestID, StructuredAnswers: secondAnswers}
		second.LateReply = &controlapp.UserInputLateReplyV1{ClientUserMessageID: "00000000-0000-4000-8000-0000000000a2", ExpectedTurnID: turnID}
		rejected := postTimedQuestionBatchHTTPV1(t, listener.URL, second)
		if rejected.StatusCode != http.StatusConflict || stringField(rejected.Body, "code") != "late_reply_too_large" {
			t.Fatalf("final promotion occupancy: status=%d code=%s", rejected.StatusCode, stringField(rejected.Body, "code"))
		}
		after, err := os.ReadFile(h.store.threadPath(selector.ThreadID))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("promotion preflight changed primary")
		}
		if delivery, err := h.store.readUserInputLateReplyV1(secondSelector.ThreadID, secondSelector.RequestID, second.LateReply.ClientUserMessageID); err != nil || delivery != nil {
			t.Fatal("future overflow already queued", err)
		}
		afterEvents, err := h.store.LoadEventsSince(selector.ThreadID, 0)
		if err != nil || len(beforeEvents.Events) != len(afterEvents.Events) || calls.Load() != independentCall {
			t.Fatal("overflow published an effect", err)
		}
		display := readTimedQuestionPrivateDisplay(t, h, secondSelector)
		assertTimedQuestionPrivateBatchV1(t, display.Submission, secondAnswers)
		logStage("second_overflow_rejected_with_zero_effects")
	}
	once.Do(func() { close(release) })
	logStage("independent_work_released")
	if contextWindowTokens == 0 {
		if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != independentCall {
			t.Fatal("undeclared context window sent another Provider request")
		}
		entries, promoted, steered, _ := timedQuestionDiagnosticSteeringEffects(t, h, selector.ThreadID, turnID, request.LateReply.ClientUserMessageID)
		if entries != 1 || promoted != 1 || steered != 1 {
			t.Fatal("default context guard ran without actual unique promotion")
		}
		display := readTimedQuestionPrivateDisplay(t, h, selector)
		assertTimedQuestionPrivateBatchV1(t, display.Submission, answers)
		assertTimedQuestionPromotedBatchV1(t, h, selector, request, answers)
		assertTimedQuestionContextGuardV1(t, h, selector.ThreadID)
		t.Log("actual timer continuation promoted complete batch; undeclared synthetic context window rejected the next Provider request")
		return
	}
	operationsDone := make(chan error, 1)
	go func() { operationsDone <- h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID) }()
	var actual []byte
	select {
	case actual = <-promotedBody:
	case operationErr := <-operationsDone:
		select {
		case actual = <-promotedBody:
		default:
			if operationErr != nil {
				t.Fatalf("fixture operation wait ended: code=%s", asyncTurnErrorClassV1(operationErr))
			}
			logTimedQuestionUnreachedProviderV1(t, h, selector, request.LateReply.ClientUserMessageID)
			t.Fatalf("actual worker ended before complete batch reached Provider: errorType=%T", operationErr)
		}
	case <-ctx.Done():
		logTimedQuestionUnreachedProviderV1(t, h, selector, request.LateReply.ClientUserMessageID)
		t.Fatal("complete batch did not reach actual Provider")
	}
	logStage("first_promoted_provider_received")
	if err := h.runtimeControl().WaitForThreadTurns(ctx, selector.ThreadID); err != nil {
		t.Fatal(err)
	}
	logStage("first_turn_completed")
	assertTimedQuestionProviderBatchV1(t, actual, receipt.Payload.CallID, answers)
	assertSyntheticQuestionWireCapacityV1(t, actual, contextWindowTokens)
	assertTimedQuestionPendingProviderResult(t, actual, receipt.Payload.CallID)
	if calls.Load() != promotedCall {
		t.Fatal("unexpected Provider count", calls.Load())
	}
	logStage("first_provider_full_batch_verified")
	if err := h.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	logStage("original_handler_shutdown")
	replayBody := make(chan []byte, 1)
	var restartCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		restartCalls.Add(1)
		replayBody <- body
		writeInlineCompletionFixtureReply(w)
	}))
	t.Cleanup(upstream.Close)
	restarted := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: f.durableRoot, DataDir: h.dataDir, ProviderID: "openai", BaseURL: upstream.URL + "/v1", APIKey: "synthetic-fixture-key", Model: syntheticQuestionCapacityModelV1, EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
	configureSyntheticQuestionModelV1(t, restarted, contextWindowTokens)
	configureServerGeneralExecution(t, restarted)
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal(err)
	}
	restarted.turnSecurity.RiskAuthority = risk
	configureHistory(restarted)
	installHandlerProviderExecutionResolverForTest(restarted)
	restarted.provider = h.provider
	t.Cleanup(func() {
		if err := restarted.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	logStage("durable_handler_reconstructed")
	reopenedHTTP := httptest.NewServer(restarted)
	t.Cleanup(reopenedHTTP.Close)
	retry = postTimedQuestionBatchHTTPV1(t, reopenedHTTP.URL, request)
	assertTimedQuestionLateReplyResponse(t, retry, selector.RequestID, request.LateReply.ClientUserMessageID, turnID, "accepted")
	if restartCalls.Load() != 0 {
		t.Fatal("reopen duplicate ran Provider")
	}
	assertTimedQuestionSingleSteeredEvent(t, restarted, selector.ThreadID, request.LateReply.ClientUserMessageID)
	logStage("reopen_original_uuid_accepted")
	// Durable retry acceptance closes the initial phase. Replay is a separate
	// bounded operation and retains the ordinary 90-second fixture budget.
	phase, phaseStarted, phaseBudget = "replay", time.Now(), 90*time.Second
	replayCtx, replayCancel := context.WithTimeout(context.Background(), phaseBudget)
	defer replayCancel()
	logStage("replay_started")
	if _, err := restarted.startRuntimeTurn(replayCtx, selector.ThreadID, startRuntimeTurnRequest{Prompt: "Continue ordinary synthetic work.", Model: syntheticQuestionCapacityModelV1, Async: true}); err != nil {
		t.Fatalf("replay start errorType=%T code=%s", err, asyncTurnErrorClassV1(err))
	}
	logStage("new_turn_started")
	select {
	case replay := <-replayBody:
		assertTimedQuestionProviderBatchV1(t, replay, receipt.Payload.CallID, answers)
		assertSyntheticQuestionWireCapacityV1(t, replay, contextWindowTokens)
		assertTimedQuestionPendingProviderResult(t, replay, receipt.Payload.CallID)
	case <-replayCtx.Done():
		t.Fatal("restored batch did not reach new Provider listener")
	}
	if err := restarted.runtimeControl().WaitForTurnOperations(replayCtx); err != nil {
		t.Fatal(err)
	}
	if restartCalls.Load() != 1 {
		t.Fatal("restart duplicated Provider effect")
	}
	logStage("new_turn_replay_full_batch_verified")
	t.Logf("actual HTTP complete batch bytes=%d; Provider, durable reopen and UUID retry preserved", 3*size)
}

func assertTimedQuestionPrivateBatchV1(t *testing.T, submission *domaincontinuation.UserInputSubmissionV1, answers []controlapp.UserInputAnswer) {
	t.Helper()
	if submission == nil {
		t.Fatal("missing complete private capture")
	}
	var originals []domaincontinuation.UserInputStructuredAnswerV1
	if json.Unmarshal(submission.Answers, &originals) != nil || len(originals) != len(answers) {
		t.Fatal("private batch association changed")
	}
	for index, answer := range originals {
		if answer.CustomText == nil || *answer.CustomText != *answers[index].CustomText {
			t.Fatal("private original was shortened", index)
		}
	}
}

func assertTimedQuestionProviderBatchV1(t *testing.T, actual []byte, callID string, answers []controlapp.UserInputAnswer) {
	t.Helper()
	count := 0
	for _, message := range timedQuestionProviderMessages(t, actual) {
		if stringField(message, "role") != "user" {
			continue
		}
		content := stringField(message, "content")
		if !strings.Contains(content, "answer_to_pending_question") {
			continue
		}
		prefix := strings.TrimSuffix(modelapp.SteeringProviderContent("payload", nil), "payload")
		if !strings.HasPrefix(content, prefix) {
			t.Fatal("missing steering attribution")
		}
		var reply map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(content, prefix)), &reply) != nil || len(reply) != 5 || stringField(reply, "callId") != callID || stringField(reply, "tool") != "request_user_input" {
			t.Fatal("lost original Host call relation")
		}
		questions, got := listAny(reply["questions"]), listAny(reply["answers"])
		if len(questions) != len(answers) || len(got) != len(answers) {
			t.Fatal("lost complete batch ordering")
		}
		for index, raw := range got {
			answer, _ := raw.(map[string]any)
			question, _ := questions[index].(map[string]any)
			if stringField(answer, "id") != answers[index].ID || stringField(question, "id") != answers[index].ID || stringField(answer, "custom") != *answers[index].CustomText {
				t.Fatal("actual Provider answer shortened or rebound", index)
			}
		}
		count++
	}
	if count != 1 {
		t.Fatal("actual Provider batch count", count)
	}
}

func logTimedQuestionUnreachedProviderV1(t *testing.T, h *runtimeServerHandler, selector displayport.SelectorV1, attempt string) {
	t.Helper()
	entries, promoted, steered, state := timedQuestionDiagnosticSteeringEffects(t, h, selector.ThreadID, selector.TurnID, attempt)
	t.Logf("actual post-admission worker state=%s pending=%d promoted=%d steered=%d", state, entries, promoted, steered)
	events, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Logf("worker diagnostic event read errorType=%T", err)
		return
	}
	for _, event := range events.Events {
		if stringField(event, "kind") == "pipeline_stage" && (stringField(event, "stage") == "provider_error" || stringField(event, "stage") == "provider_admission_rejected") {
			details, _ := event["details"].(map[string]any)
			// This is the closed Host reason enum, never a raw cause/message.
			code := domainfailure.New(stringField(details, "reasonCode"), nil).Code()
			t.Logf("actual post-admission Provider failure code=%s", code)
		}
	}
}

const (
	syntheticQuestionCapacityModelV1 = "synthetic-question-capacity"
	// A synthetic test capability, never a real model claim. The largest
	// payload includes 2.7MB answers and 0.8MB ordinary history. Captured
	// request estimates below must leave at least 0.5M tokens beneath the
	// unchanged 85% Host guard.
	syntheticQuestionContextWindowV1 = 2_000_000
)

func configureSyntheticQuestionModelV1(t *testing.T, h *runtimeServerHandler, window int) {
	t.Helper()
	original := h.providerConfig.TurnConfigForExecution("openai", "")
	declared := map[string]any{"id": "openai", "baseUrl": original.BaseURL, "apiKey": "synthetic-fixture-key",
		"endpointFormat": "chat_completions", "models": []string{syntheticQuestionCapacityModelV1}}
	if window > 0 {
		declared["modelProfiles"] = map[string]any{syntheticQuestionCapacityModelV1: map[string]any{"contextWindowTokens": window}}
	}
	raw, err := json.Marshal(map[string]any{"defaultProviderId": "openai", "providers": []any{declared}})
	if err != nil {
		t.Fatal(err)
	}
	h.providerConfig = runtimeprovider.NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{
		DefaultProviderID: "openai", DefaultModel: syntheticQuestionCapacityModelV1,
		DefaultBaseURL: original.BaseURL, DefaultAPIKey: "synthetic-fixture-key",
		DefaultEndpointFormat: "chat_completions", ModelProvidersJSON: string(raw)})
	if err := h.providerConfig.ConfigurationError(); err != nil {
		t.Fatal("synthetic capability configuration is invalid")
	}
	resolved, err := h.providerConfig.ResolveTurnExecution(modelapp.TurnExecutionInput{RequestProviderID: "openai", RequestModel: syntheticQuestionCapacityModelV1})
	if err != nil || resolved.Config.ContextWindowTokens != window || resolved.Model != syntheticQuestionCapacityModelV1 || resolved.Config.BaseURL != original.BaseURL {
		t.Fatal("production capability input lost the synthetic declaration or local route")
	}
}

// Decode only the actual text-only chat fixture, including nested function
// schemas and tool calls. Its sole system message also accounts for the
// independent SystemPrompt overhead in the Host estimate. Compact wire schema
// text is not a general provider inverse; production admission remains the
// authority. Never omit array content or a tool body silently.
func assertSyntheticQuestionWireCapacityV1(t *testing.T, body []byte, window int) {
	t.Helper()
	var wire map[string]any
	if json.Unmarshal(body, &wire) != nil || stringField(wire, "model") != syntheticQuestionCapacityModelV1 {
		t.Fatal("invalid actual synthetic Provider wire request")
	}
	request := domainmodel.Request{}
	systems := 0
	for _, raw := range timedQuestionProviderMessages(t, body) {
		content, contentOK := raw["content"].(string)
		calls := listAny(raw["tool_calls"])
		if !contentOK && (raw["content"] != nil || stringField(raw, "role") != "assistant" || len(calls) == 0) {
			t.Fatal("unaccounted fixture wire content")
		}
		message := domainmodel.Message{Role: stringField(raw, "role"), Content: content, ToolCallID: stringField(raw, "tool_call_id")}
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			function, _ := call["function"].(map[string]any)
			arguments := stringField(function, "arguments")
			if stringField(call, "type") != "function" || stringField(function, "name") == "" || !json.Valid([]byte(arguments)) {
				t.Fatal("unaccounted fixture wire tool call")
			}
			message.ToolCalls = append(message.ToolCalls, domainmodel.ToolCall{ID: stringField(call, "id"), Name: stringField(function, "name"), Arguments: json.RawMessage(arguments)})
		}
		if message.Role == "system" {
			systems++
			request.SystemPrompt = message.Content
		}
		request.Messages = append(request.Messages, message)
	}
	for _, rawTool := range listAny(wire["tools"]) {
		tool, _ := rawTool.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		parameters, err := json.Marshal(function["parameters"])
		if stringField(tool, "type") != "function" || stringField(function, "name") == "" || function["parameters"] == nil || err != nil {
			t.Fatal("unaccounted fixture wire tool schema")
		}
		request.Tools = append(request.Tools, domainmodel.ToolSchema{Name: stringField(function, "name"), Description: stringField(function, "description"), Parameters: parameters})
	}
	thresholds := modelapp.ResolveContextThresholdsV1(window)
	measured := modelapp.EstimateProviderRequestTokensV1(request)
	if systems != 1 || thresholds.UsedFallback || measured >= thresholds.HardThresholdTokens || thresholds.HardThresholdTokens-measured < window/4 {
		t.Fatal("declared synthetic profile lacks measured safe request headroom")
	}
	t.Logf("actual synthetic wire estimatedTokens=%d declaredWindow=%d hardThreshold=%d headroom=%d", measured, window, thresholds.HardThresholdTokens, thresholds.HardThresholdTokens-measured)
}

func assertTimedQuestionContextGuardV1(t *testing.T, h *runtimeServerHandler, threadID string) {
	t.Helper()
	events, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	observed := 0
	for _, event := range events.Events {
		if stringField(event, "kind") != "pipeline_stage" || stringField(event, "stage") != "provider_admission_rejected" {
			continue
		}
		details, _ := event["details"].(map[string]any)
		projected, projectedOK := contracts.NumericSeq(details["projectedRequestTokens"])
		hard, hardOK := contracts.NumericSeq(details["hardThresholdTokens"])
		attempts, attemptsOK := contracts.NumericSeq(details["providerAttemptCount"])
		if len(details) != 4 || stringField(details, "reasonCode") != "context_window_hard_limit" || !attemptsOK || attempts != 0 || !projectedOK || !hardOK || hard != modelapp.ResolveContextThresholdsV1(0).HardThresholdTokens || projected < hard {
			t.Fatal("actual default model guard lost its closed pre-send refusal")
		}
		observed++
		t.Logf("actual undeclared model guard projectedTokens=%d hardThreshold=%d providerAttemptCount=0", int(projected), int(hard))
	}
	if observed != 1 {
		t.Fatal("missing unique actual context_window_hard_limit admission")
	}
}

func assertTimedQuestionDurableOccupancyV1(t *testing.T, primary []byte, turnID, prompt string) {
	t.Helper()
	var thread map[string]any
	if json.Unmarshal(primary, &thread) != nil || len(prompt) < 400000 {
		t.Fatal("invalid actual ordinary occupancy fixture")
	}
	turns, userItems := 0, 0
	promptMatched := false
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		turns++
		promptMatched = stringField(turn, "prompt") == prompt
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "user_message" && stringField(item, "role") == "user" && stringField(item, "text") == prompt {
				userItems++
			}
		}
	}
	if turns != 1 || !promptMatched || userItems != 1 {
		t.Fatal("normal production start did not persist both ordinary occupancy fields")
	}
	t.Logf("actual ordinary start occupancy promptMatched=true uniqueUserItems=%d scalarBytes=%d", userItems, len(prompt))
}

func assertTimedQuestionPromotedBatchV1(t *testing.T, h *runtimeServerHandler, selector displayport.SelectorV1, request controlapp.UserInputResponse, answers []controlapp.UserInputAnswer) {
	t.Helper()
	delivery, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, request.LateReply.ClientUserMessageID)
	if err != nil || delivery == nil || delivery.State != "accepted" {
		t.Fatal("default guard did not follow actual accepted promotion")
	}
	identity, present, err := domainsteering.UserInputReplyIdentityFromEntryV1(delivery.Entry)
	if err != nil || !present {
		t.Fatal("default guard lost original signed Question identity")
	}
	content, err := domainsteering.ParseUserInputReplyContentV1(stringField(delivery.Entry, "text"), identity)
	if err != nil || len(content.Answers) != len(answers) {
		t.Fatal("default guard lost complete promoted answer body")
	}
	for index, actual := range content.Answers {
		if answers[index].CustomText == nil || actual.Custom != *answers[index].CustomText || len(actual.Selected) != 0 {
			t.Fatal("default guard shortened or reordered a promoted answer")
		}
	}
	thread, err := h.store.GetThread(selector.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	matched := 0
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") != selector.TurnID {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "clientUserMessageId") == request.LateReply.ClientUserMessageID && stringField(item, "kind") == "user_message" && stringField(item, "text") == stringField(delivery.Entry, "text") {
				matched++
			}
		}
	}
	if matched != 1 {
		t.Fatal("default guard did not follow exactly one complete primary promotion")
	}
}
