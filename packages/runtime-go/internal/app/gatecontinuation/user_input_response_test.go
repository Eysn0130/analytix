package gatecontinuation

import (
	displayport "analytix.local/runtime-go/internal/ports/userinputdisplay"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	identityapp "analytix.local/runtime-go/internal/app/identity"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

type selectorRegistrationProbeV1 struct{ called bool }

func (p *selectorRegistrationProbeV1) RunRegisteredTurn(ctx context.Context, _, _ string, run func(context.Context) controlapp.ActionResult) controlapp.ActionResult {
	p.called = true
	return run(ctx)
}

func TestForkSelectorCannotRegisterOrClaimOriginalQuestion(t *testing.T) {
	id := "input_" + strings.Repeat("a", 64)
	for _, command := range []string{"submit", "cancel", "continue", "contradictory_record"} {
		t.Run(command, func(t *testing.T) {
			r := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			pending := appmodel.PendingToolCall{ThreadID: "source", TurnID: "turn", Call: domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(`{"prompt":"Synthetic question"}`)}}
			record := controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: "item_" + id}
			selector := displayport.SelectorV1{ThreadID: "child", TurnID: pending.TurnID, ItemID: record.ItemID, RequestID: id}
			if command == "contradictory_record" {
				record.ThreadID = "other"
				selector.ThreadID = pending.ThreadID
			}
			if !r.RegisterPendingUserInput(id, record, pending) {
				t.Fatal("register source")
			}
			request := controlapp.UserInputResponse{InputID: id, Selector: &selector, StructuredAnswers: []controlapp.UserInputAnswer{{ID: id + "_1", Kind: "skip"}}}
			if command == "cancel" {
				request.StructuredAnswers = nil
				request.Cancelled = true
			}
			if command == "continue" {
				request.StructuredAnswers = nil
				request.Continue = true
			}
			probe := &selectorRegistrationProbeV1{}
			result := RespondUserInputRegistered(context.Background(), request, Dependencies{Registry: r}, probe)
			if result.StatusCode != 409 || probe.called {
				t.Fatal("foreign selector registered original turn")
			}
			if _, retained, exists, hasPending := r.PeekUserInput(id); !exists || !hasPending || retained.ThreadID != "source" {
				t.Fatal("foreign selector consumed original gate")
			}
		})
	}
}

func TestInvalidUserInputAnswersLeaveOriginalGatePending(t *testing.T) {
	gateID := "input_host"
	pending := appmodel.PendingToolCall{Call: domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(`{"prompt":"Choose","questions":[{"question":"First","multi_select":true,"options":["A","B"]},{"question":"Second","options":["C","D"]}]}`)}}
	valid := []controlapp.UserInputAnswer{
		{ID: "input_host_1", Kind: "answer", OptionIDs: []string{"input_host_1_option_1"}},
		{ID: "input_host_2", Kind: "skip"},
	}
	for name, change := range map[string]func(*controlapp.UserInputResponse){
		"missing":          func(r *controlapp.UserInputResponse) { r.StructuredAnswers = r.StructuredAnswers[:1] },
		"duplicate":        func(r *controlapp.UserInputResponse) { r.StructuredAnswers[1] = r.StructuredAnswers[0] },
		"foreign question": func(r *controlapp.UserInputResponse) { r.StructuredAnswers[0].ID = "input_other_1" },
		"cross question option": func(r *controlapp.UserInputResponse) {
			r.StructuredAnswers[0].OptionIDs = []string{"input_host_2_option_1"}
		},
		"unknown option": func(r *controlapp.UserInputResponse) {
			r.StructuredAnswers[0].OptionIDs = []string{"input_host_1_option_9"}
		},
		"single option with custom": func(r *controlapp.UserInputResponse) {
			text := "custom"
			r.StructuredAnswers[1] = controlapp.UserInputAnswer{ID: "input_host_2", Kind: "answer", OptionIDs: []string{"input_host_2_option_1"}, CustomText: &text}
		},
		"single select multiple": func(r *controlapp.UserInputResponse) {
			r.StructuredAnswers[1] = controlapp.UserInputAnswer{ID: "input_host_2", Kind: "answer", OptionIDs: []string{"input_host_2_option_1", "input_host_2_option_2"}}
		},
		"cancel and answers": func(r *controlapp.UserInputResponse) { r.Cancelled = true },
	} {
		t.Run(name, func(t *testing.T) {
			registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			record := controlapp.GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item"}
			if !registry.RegisterPendingUserInput(gateID, record, pending) {
				t.Fatal("register")
			}
			manager := controlapp.NewApprovalUserInputManager()
			manager.RequestUserInput(gateID, "Choose")
			store := newCancellationGateStore()
			request := controlapp.UserInputResponse{InputID: gateID, StructuredAnswers: append([]controlapp.UserInputAnswer(nil), valid...)}
			change(&request)
			result := RespondUserInput(context.Background(), request, Dependencies{Registry: registry, Manager: manager, Store: store})
			if result.StatusCode != 400 || result.Body["code"] != "validation_error" {
				t.Fatalf("invalid batch response: %#v", result)
			}
			_, retained, exists, hasPending := registry.PeekUserInput(gateID)
			if !exists || !hasPending || string(retained.Call.Arguments) != string(pending.Call.Arguments) {
				t.Fatal("invalid batch consumed or replaced pending authority")
			}
			if status, exists := manager.UserInputDisposition(gateID); !exists || status != "pending" {
				t.Fatalf("invalid batch changed manager: %q %v", status, exists)
			}
			if store.PatchCalls() != 0 || store.EventCalls() != 0 {
				t.Fatal("invalid batch persisted side effects")
			}
		})
	}
}

func TestForeignLegacySingletonIDCannotConsumeGate(t *testing.T) {
	gateID := "input_host"
	registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
	pending := appmodel.PendingToolCall{Call: domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(`{"prompt":"Choose","questions":[{"id":"old-question","question":"Choose"}]}`)}}
	if !registry.RegisterPendingUserInput(gateID, controlapp.GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item"}, pending) {
		t.Fatal("register")
	}
	result := RespondUserInput(context.Background(), controlapp.UserInputResponse{InputID: gateID, Answers: []map[string]string{{"id": "foreign", "value": "text"}}}, Dependencies{Registry: registry})
	if result.StatusCode != 400 {
		t.Fatalf("foreign legacy alias accepted: %#v", result)
	}
	if _, _, exists, hasPending := registry.PeekUserInput(gateID); !exists || !hasPending {
		t.Fatal("foreign answer consumed gate")
	}
}

func TestPrivateAnswerLimitsLeaveGatePendingBeforeClaim(t *testing.T) {
	for name, text := range map[string]string{"non UTF8": string([]byte{0xff}), "JSON byte limit": strings.Repeat("x", domaincontinuation.MaxUserInputSubmissionBytesV1)} {
		t.Run(name, func(t *testing.T) {
			gateID := "input_host"
			registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			store := newCancellationGateStore()
			pending := appmodel.PendingToolCall{Call: domainmodel.ToolCall{Name: "request_user_input", Arguments: json.RawMessage(`{"prompt":"Question"}`)}}
			if !registry.RegisterPendingUserInput(gateID, controlapp.GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item"}, pending) {
				t.Fatal("register")
			}
			result := RespondUserInput(context.Background(), controlapp.UserInputResponse{InputID: gateID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: gateID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &text}}}, Dependencies{Registry: registry, Store: store})
			if result.StatusCode != 400 {
				t.Fatal("invalid private source acknowledged")
			}
			if _, _, exists, pending := registry.PeekUserInput(gateID); !exists || !pending {
				t.Fatal("invalid private source claimed gate")
			}
			if store.PatchCalls() != 0 || store.EventCalls() != 0 || len(store.dispositions) != 0 {
				t.Fatal("invalid private source had durable effects")
			}
		})
	}
}

func TestUserInputCancellationNeverCapturesAnswerPayloadOrContinuesModel(t *testing.T) {
	t.Run("legacy_absent_selector", func(t *testing.T) { testUserInputCancellationOwnerV1(t, false) })
	t.Run("exact_original_selector", func(t *testing.T) { testUserInputCancellationOwnerV1(t, true) })
}

func testUserInputCancellationOwnerV1(t *testing.T, withSelector bool) {
	now := time.Now().UTC()
	pending := cancellationPendingFixture(t, domaincontinuation.KindUserInput, now)
	store := newCancellationGateStore()
	registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
	manager := controlapp.NewApprovalUserInputManager()
	continuations := continuationapp.NewService(newCancellationAuthority(), store)
	gateID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, err := continuations.IssuePendingHost(domaincontinuation.KindUserInput, gateID, "item_"+gateID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	record := controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: "item_" + gateID, Prompt: "Continue?", ContinuationReceiptID: receipt.ReceiptID}
	if !registry.RegisterPendingUserInput(gateID, record, pending) {
		t.Fatal("register")
	}
	manager.RequestUserInput(gateID, "Continue?")
	completeCalls, persistCalls := 0, 0
	dependencies := Dependencies{Store: store, Registry: registry, Manager: manager, Continuations: continuations, TerminalStatus: func(string, string) (string, bool, error) { return "running", false, nil }, RecordResolution: func(event map[string]any) error {
		if _, present := event["answers"]; present {
			t.Fatal("resolution leaked answers")
		}
		return nil
	}, PersistToolResult: func(_ context.Context, _ appmodel.PendingToolCall, output any, _ bool) error {
		persistCalls++
		if _, present := output.(map[string]any)["answers"]; present {
			t.Fatal("cancel output captured answers")
		}
		return nil
	}, Complete: func(context.Context, appmodel.PendingToolCall, any, bool) (apploop.RuntimeAgentLoopResult, error) {
		completeCalls++
		return apploop.RuntimeAgentLoopResult{}, nil
	}, Finalize: func(context.Context, appmodel.PendingToolCall, apploop.RuntimeAgentLoopResult, evidenceapp.TerminalReason) error {
		return nil
	}}
	request := controlapp.UserInputResponse{InputID: gateID, Cancelled: true}
	if withSelector {
		request.Selector = &displayport.SelectorV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: record.ItemID, RequestID: gateID}
	}
	result := RespondUserInput(context.Background(), request, dependencies)
	if result.StatusCode != 200 || completeCalls != 0 || persistCalls != 1 {
		t.Fatal("cancellation settlement changed", result.StatusCode)
	}
	disposition, err := store.ResolveDisposition(context.Background(), gateID)
	if err != nil || disposition.Status != domaincontinuation.StatusCancelled || disposition.UserInputSubmission != nil {
		t.Fatal("cancellation minted private answers", err)
	}
	if result := RespondUserInput(context.Background(), request, dependencies); result.StatusCode != 409 {
		t.Fatal("duplicate cancellation reused executable claim")
	}
}

func TestOriginalAnswerCaptureDoesNotEnterPublicReplayOrModelCarrier(t *testing.T) {
	const rawEmail = "synthetic.question.answer@example.com"
	inputID := "input_host"
	questions := []map[string]any{{"id": inputID + "_1", "question": "Question", "multiSelect": false, "options": []map[string]string{}}}
	text := "  " + rawEmail + "  "
	request := controlapp.UserInputResponse{StructuredAnswers: []controlapp.UserInputAnswer{{ID: inputID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &text}}}
	normalized, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(request, questions, "")
	if err != nil || submission == nil || !bytes.Contains(submission.Answers, []byte(rawEmail)) {
		t.Fatal("private fixture lost source answer", err)
	}
	projected := privacyprojectionapp.ProjectHostValidatedUserInputAnswers(inputID, questions, normalized)
	output := appturn.UserInputResolvedOutputForQuestions("submitted", projected, false, inputID, questions)
	persistable := toolcatalogapp.PersistableToolOutput("request_user_input", output)
	public, model := visionbridgeapp.PrepareToolResultForModel(visionbridgeapp.ToolResultProjectionInput{ToolName: "request_user_input", RawOutput: output, PersistOutput: persistable})
	manager := controlapp.NewApprovalUserInputManager()
	manager.RequestUserInput(inputID, "Question")
	if status, _ := manager.SubmitUserInput(inputID, projected); status != 200 {
		t.Fatal("manager submit")
	}
	resolution := appturn.UserInputResolvedEvent(controlapp.UserInputGateCancellation(inputID, controlapp.GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item", Prompt: "Question"}).Record, "submitted")
	publicBody, err := json.Marshal([]any{public, manager.Replay(), resolution})
	if err != nil || bytes.Contains(publicBody, []byte(rawEmail)) || bytes.Contains(publicBody, []byte("userInputSubmission")) || bytes.Contains(publicBody, []byte("customText")) || bytes.Contains(publicBody, []byte("answers")) {
		t.Fatal("private source answer entered public replay/projection")
	}
	if strings.Contains(model, rawEmail) || strings.Contains(model, "userInputSubmission") || strings.Contains(model, `"format":"structured"`) {
		t.Fatal("private original capture replaced projected model carrier")
	}
}

// This deterministic app seam uses the real signed Continue producer and
// settlement/projection owners. It does not run an HTTP timer or Provider.
func TestTimedQuestionPendingHostIdentityAtProviderBoundary(t *testing.T) {
	for name, callID := range map[string]string{
		"alphabetic":  "call_host_" + strings.Repeat("a", 64),
		"numeric_run": "call_host_" + strings.Repeat("a", 26) + "123456789012" + strings.Repeat("a", 26),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC()
			workspace := t.TempDir()
			observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
				WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateMissing,
			})
			if err != nil {
				t.Fatal(err)
			}
			workspaceAuthority := pendingIdentityWorkspaceV1{observation: observation}
			identity, err := identityapp.NewInstallationLocalAuthority(domainsecurity.SHA256Hex([]byte("synthetic-pending-question-installation")))
			if err != nil {
				t.Fatal(err)
			}
			risk, err := threadriskauthorityapp.NewGeneralOnlyAuthority(workspaceAuthority)
			if err != nil {
				t.Fatal(err)
			}
			securityAuthority := turnsecurityapp.WorkspaceSecurityAuthority{Identity: identity, Observer: workspaceAuthority, RiskAuthority: risk}
			principal, err := identity.ResolveCurrent(ctx)
			if err != nil {
				t.Fatal(err)
			}
			frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
				Context: ctx, Authority: securityAuthority, Thread: map[string]any{},
				ThreadID: "thread-pending-identity", TurnID: "turn-pending-identity",
				Workspace: workspace, Principal: principal, IssuedAt: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			call := domainmodel.ToolCall{ID: callID, Name: "request_user_input", Arguments: json.RawMessage(
				`{"prompt":"Choose","questions":[{"question":"What next?"}],"timeout":120}`)}
			var schemas []domainmodel.ToolSchema
			for _, schema := range toolcatalogapp.UserInputToolSchemas(toolcatalogapp.UserInputConfig{Mode: "timed"}) {
				if schema.Name == call.Name {
					schemas = append(schemas, schema)
				}
			}
			scope := []string{call.Name}
			grant, err := executiongrantapp.IssueHost(frozen, "synthetic", call, schemas, scope, true, "not_required", 0, "host:builtin", now)
			if err != nil {
				t.Fatal(err)
			}
			config := domainmodel.TurnConfig{ProviderID: "synthetic", Model: "synthetic", BaseURL: "https://provider.invalid", EndpointFormat: "openai-chat-completions"}
			pending := appmodel.PendingToolCall{
				ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Workspace: workspace,
				ProviderConfig: config, ProviderID: config.ProviderID, Model: config.Model, Prompt: "synthetic independent work",
				Call: call, ToolCallItemID: domaintoolcall.ToolCallItemIDV1(frozen.TurnID, callID),
				ToolScope: scope, SecurityContext: frozen, ExecutionGrant: grant,
				ApprovalPolicy: "never", SandboxMode: "workspace-write",
				LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
				ProviderStepExact: true, OrdinaryResultInputIsolated: true,
				ProviderNamespace:  domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
				UserInputTimedWait: &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 120},
			}
			epoch, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, nil, now)
			if err != nil {
				t.Fatal(err)
			}
			grantBody, err := json.Marshal(grant)
			if err != nil {
				t.Fatal(err)
			}
			grantRecord := map[string]any{}
			if err := json.Unmarshal(grantBody, &grantRecord); err != nil {
				t.Fatal(err)
			}
			callItem := map[string]any{
				"id": pending.ToolCallItemID, "kind": "tool_call", "status": "running",
				"threadId": pending.ThreadID, "turnId": pending.TurnID, "callId": callID, "toolName": call.Name,
				"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
				"createdAt": grant.IssuedAt, "contextEpoch": float64(frozen.ContextEpoch), "contextDigest": frozen.ContextDigest,
				"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
			}
			store := newCancellationGateStore()
			continuations := continuationapp.NewService(newCancellationAuthority(), store)
			gateID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, frozen.ContextDigest, grant.GrantID, callID)
			receipt, err := continuations.IssuePendingHost(domaincontinuation.KindUserInput, gateID, "item_"+gateID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			gateItem := map[string]any{"id": receipt.Payload.ItemID, "kind": "user_input", "inputId": gateID, "status": "pending", "continuationReceiptId": receipt.ReceiptID}
			thread := map[string]any{
				"id": pending.ThreadID, "workspace": workspace, "securityState": turnsecurityapp.PublicRecord(frozen),
				"contextEpochState": contextepochapp.PublicState(epoch),
				"turns":             []any{map[string]any{"id": pending.TurnID, "status": "running", "securityContext": turnsecurityapp.PublicRecord(frozen), "items": []any{callItem, gateItem}}},
			}
			store.getThread = func(threadID string) (map[string]any, error) {
				if threadID != pending.ThreadID {
					return nil, errors.New("foreign synthetic thread")
				}
				return thread, nil
			}
			registry := controlapp.NewGateRegistry[appmodel.PendingToolCall]()
			manager := controlapp.NewApprovalUserInputManager()
			record := controlapp.GateRecord{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: receipt.Payload.ItemID, Prompt: "Choose", ContinuationReceiptID: receipt.ReceiptID}
			if !registry.RegisterPendingUserInput(gateID, record, pending) {
				t.Fatal("register signed pending gate")
			}
			manager.RequestUserInput(gateID, "Choose")
			completeCalls := 0
			var final domainmodel.Request
			dependencies := Dependencies{
				UserInputConfig: toolcatalogapp.UserInputConfig{Mode: "timed"},
				Store:           store, Registry: registry, Manager: manager, Continuations: continuations,
				WorkspaceReader: workspaceAuthority, SecurityAuthority: securityAuthority,
				ValidateCatalog: func(value appmodel.PendingToolCall) error {
					return executiongrantapp.ValidatePendingCatalog(value, schemas, true)
				},
				TerminalStatus: func(string, string) (string, bool, error) { return "running", false, nil },
				RecordResolution: func(event map[string]any) error {
					if event["status"] != "continued" || event["inputId"] != gateID {
						t.Fatal("Continue lost original resolution identity")
					}
					if _, present := event["answers"]; present {
						t.Fatal("Continue acquired an answer")
					}
					gateItem["status"] = "continued"
					return nil
				},
				Complete: func(ctx context.Context, value appmodel.PendingToolCall, output any, isError bool) (apploop.RuntimeAgentLoopResult, error) {
					completeCalls++
					t.Logf("synthetic gate=%s originalHostCall=%s producerOutput=%s", gateID, receipt.Payload.CallID, appmodel.ToolResultContent(output))
					raw, ok := output.(map[string]any)
					if !ok || isError || len(raw) != 3 || raw["pending"] != true || raw["callId"] != receipt.Payload.CallID {
						t.Fatal("real Continue producer lost closed original-call output")
					}
					trusted, disposition, err := continuations.ResolveTrustedDispositionForAudit(ctx, gateID)
					if err != nil || trusted.ReceiptID != receipt.ReceiptID || disposition.Status != domaincontinuation.StatusContinued || disposition.UserInputSubmission != nil {
						t.Fatal("Continue lacks current-installation signed no-answer disposition")
					}
					attempt := apploop.CaptureToolResultProviderAttemptV1(apploop.ToolResultProviderAttemptInputV1{Pending: value, Output: output, IsError: isError})
					defer attempt.DiscardPrivate()
					settlement, err := evidenceapp.PrepareToolSettlement(ctx, nil, securityAuthority, workspaceAuthority, store, nil, value, output, isError)
					if err != nil || !settlement.SettlementAuthorized || settlement.IsError {
						t.Fatal("real settlement did not authorize original pending output")
					}
					prepared, err := attempt.PrepareSettlementV1(settlement.Output, settlement.IsError)
					if err != nil {
						return apploop.RuntimeAgentLoopResult{}, err
					}
					applicable, err := appmodel.BindPendingUserInputProviderMessageV1(ctx, continuations, value, &prepared.Message)
					if err != nil || !applicable || prepared.Message.PrivateUserInputPendingBinding == nil {
						t.Fatal("real signed pending settlement did not bind Provider identity", err)
					}
					final, err = privacyprojectionapp.ProjectProviderRequestForEffect(frozen, domainmodel.Request{Messages: []domainmodel.Message{prepared.Message}}, true)
					return apploop.RuntimeAgentLoopResult{Paused: true}, err
				},
				Finalize: func(context.Context, appmodel.PendingToolCall, apploop.RuntimeAgentLoopResult, evidenceapp.TerminalReason) error {
					t.Fatal("isolated pending fixture unexpectedly finalized a turn")
					return nil
				},
			}
			result := RespondUserInput(ctx, controlapp.UserInputResponse{InputID: gateID, Continue: true}, dependencies)
			if result.StatusCode != 200 || completeCalls != 1 || len(final.Messages) != 1 {
				t.Fatalf("signed Continue did not reach final Provider projection: status=%d completions=%d messages=%d", result.StatusCode, completeCalls, len(final.Messages))
			}
			if status, exists := manager.UserInputDisposition(gateID); !exists || status != "continued" {
				t.Fatal("Continue did not consume exactly one pending manager state")
			}
			message := final.Messages[0]
			t.Logf("synthetic expectedHostCall=%s finalOuterHostCall=%s finalBody=%s", callID, message.ToolCallID, message.Content)
			if message.ToolCallID != callID {
				t.Error("final projection changed outer Host identity")
			}
			var body map[string]any
			if json.Unmarshal([]byte(message.Content), &body) != nil || len(body) != 3 || body["pending"] != true || body["callId"] != callID || body["message"] != "The question remains open for a later answer. Continue independent work; no answer or approval was supplied." {
				t.Error("final projection lost closed original-call pending semantics")
			}
		})
	}
}

type pendingIdentityWorkspaceV1 struct {
	observation domainsecurity.CaseBindingObservationV1
}

func (workspace pendingIdentityWorkspaceV1) Observe(path string) (domainsecurity.CaseBindingObservationV1, error) {
	if path != workspace.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("foreign synthetic workspace")
	}
	return workspace.observation, nil
}

func (workspace pendingIdentityWorkspaceV1) WorkspaceRealPath(path string) (string, error) {
	if path != workspace.observation.WorkspaceRealPath {
		return "", errors.New("foreign synthetic workspace")
	}
	return path, nil
}

func (workspace pendingIdentityWorkspaceV1) ReadOptional(path string) (domainsecurity.CaseBinding, bool, error) {
	if _, err := workspace.WorkspaceRealPath(path); err != nil {
		return domainsecurity.CaseBinding{}, false, err
	}
	return domainsecurity.CaseBinding{}, false, nil
}
