package server

import (
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	displayport "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

func (h *runtimeServerHandler) handleApprovalPath(w http.ResponseWriter, r *http.Request) {
	httpapi.GateHandlers{Control: h.runtimeControl()}.HandleApproval(w, r)
}

func (h *runtimeServerHandler) approveRuntimeTool(ctx context.Context, request controlapp.ApprovalDecision) (controlapp.ActionResult, error) {
	return gatecontinuationapp.ApproveRegistered(ctx, request, h.runtimeGateContinuationDependencies(), h.runtimeControl()), nil
}

func (h *runtimeServerHandler) handleUserInputPath(w http.ResponseWriter, r *http.Request, prefix string) {
	httpapi.GateHandlers{Control: h.runtimeControl(), Wait: h}.HandleUserInput(w, r, prefix)
}

func (h *runtimeServerHandler) respondRuntimeUserInput(ctx context.Context, request controlapp.UserInputResponse) (controlapp.ActionResult, error) {
	if request.Continue {
		record, pending, _, hasPending := h.gates.PeekUserInput(request.InputID)
		if !hasPending || request.Selector != nil && (record.ThreadID != pending.ThreadID || record.TurnID != pending.TurnID) ||
			!controlapp.UserInputResponseMatchesOwnerV1(request, pending.ThreadID, pending.TurnID, record.ItemID, request.InputID) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "user_input_owner_mismatch", "message": "user input owner does not match"}}, nil
		}
		if !h.gates.TriggerUserInputWaitExact(request.InputID, record, pending) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "timed_wait_unavailable", "message": "timed foreground wait is unavailable"}}, nil
		}
		// Transport scheduling acknowledgement only. The real signed resolution
		// and settlement are produced by the registered Host worker below.
		return controlapp.ActionResult{StatusCode: http.StatusAccepted, Body: map[string]any{"inputId": request.InputID, "scheduled": true}}, nil
	}
	if request.LateReply != nil {
		return h.respondRuntimeLateUserInput(ctx, request)
	}
	return gatecontinuationapp.RespondUserInputRegistered(ctx, request, h.runtimeGateContinuationDependencies(), h.runtimeControl()), nil
}

func (h *runtimeServerHandler) runtimeGateContinuationDependencies() gatecontinuationapp.Dependencies {
	return gatecontinuationapp.Dependencies{
		UserInputConfig:          h.userInputConfig,
		ScheduleUserInputWait:    h.scheduleRuntimeUserInputWait,
		CaptureUserInputProducer: h.runtimeControl().CaptureTurnExecutionOwnerV1,
		Store:                    h.store,
		WorkspaceReader:          filestore.CaseBindingReader{},
		SecurityAuthority:        h.turnSecurity,
		Source:                   h.mcp,
		Registry:                 h.gates,
		Manager:                  h.gate,
		Continuations:            h.continuations,
		AcquireContextEffect:     h.runtimeSubagentState().AcquireContextEffect,
		AcquireOrdinaryEffect:    h.runtimeSubagentState().AcquireOrdinaryContextEffect,
		ValidateCatalog:          h.validateRuntimePendingCatalog,
		ExecuteAndSettle: func(ctx context.Context, pending runtimePendingToolCall) (apploop.SettledToolExecution, error) {
			return h.executeAndSettleRuntimeTool(ctx, pending, nil, nil)
		},
		PersistToolResult: func(ctx context.Context, pending runtimePendingToolCall, output any, isError bool) error {
			_, err := h.persistRuntimeToolResultAndMessage(ctx, pending, output, isError)
			return err
		},
		SettleTerminalQuestion: h.persistTerminalQuestionResult,
		Complete:               h.completeRuntimeContinuation,
		CompleteSettled:        h.completeRuntimeContinuationWithSettled,
		Finalize: func(ctx context.Context, pending runtimePendingToolCall, result runtimeAgentLoopResult, reason evidenceapp.TerminalReason) error {
			return h.finalizeRuntimeTurnAfterLoop(ctx, pending, result, reason)
		},
		RecordFailure:           h.recordRuntimeTurnFailureForPending,
		TerminalStatus:          h.runtimeTurnTerminalStatus,
		RecordCancellations:     h.recordPendingGateCancellations,
		RecordRequest:           h.recordPendingGateRequest,
		RecordGrantTransition:   h.recordApprovalGrantTransition,
		EnsureGrantTransition:   h.store.EnsureApprovalTransitionItemExact,
		RecordResolution:        h.recordPendingGateResolution,
		TerminalTakeoverPending: h.runtimeControl().HostTerminalTakeoverReserved,
	}
}

func (h *runtimeServerHandler) runtimeGateEventOutboxDependencies() gatecontinuationapp.EventOutboxDependencies {
	return gatecontinuationapp.EventOutboxDependencies{
		GetThread: h.store.GetThread,
		LoadEvents: func(threadID string) ([]map[string]any, error) {
			replay, err := h.store.LoadEventsSince(threadID, 0)
			return replay.Events, err
		},
		RecordEvent: func(event map[string]any) error {
			_, _, err := h.store.RecordEvent(event)
			return err
		},
	}
}

func (h *runtimeServerHandler) recordApprovalGrantTransition(event map[string]any) error {
	h.gateCancellationMu.Lock()
	defer h.gateCancellationMu.Unlock()
	return gatecontinuationapp.RecordApprovalGrantTransitionV1(event, h.runtimeGateEventOutboxDependencies())
}

// The timer continuation is owned by the existing registered turn worker.
// HTTP/attachment cancellation releases only the wait claim, never this work.
func (h *runtimeServerHandler) scheduleRuntimeUserInputWait(id string, wait *domaincontinuation.UserInputTimedWaitV1, issuedAt string) {
	h.gates.StartUserInputWait(id, wait, issuedAt, func() {
		result := gatecontinuationapp.RespondUserInputRegistered(context.Background(), controlapp.UserInputResponse{InputID: id, Continue: true}, h.runtimeGateContinuationDependencies(), h.runtimeControl())
		if !h.runtimeControl().ShuttingDown() && ((result.StatusCode == http.StatusConflict && stringField(result.Body, "code") == "turn_execution_conflict") || (result.StatusCode == http.StatusServiceUnavailable && stringField(result.Body, "code") == "runtime_shutting_down")) {
			h.gates.RetryUserInputWait(id)
		}
	})
}

func (h *runtimeServerHandler) AttachUserInputWait(ctx context.Context, id string, snapshot func(*int64) error) error {
	if ctx == nil || snapshot == nil || h.runtimeControl().ShuttingDown() {
		return errors.New("timed wait is unavailable")
	}
	record, pending, exists, active := h.gates.PeekUserInput(id)
	if !exists || !active || pending.UserInputTimedWait == nil {
		return errors.New("timed wait is unavailable")
	}
	receipt, err := h.continuations.ResolvePendingReceiptHost(id, record.ContinuationReceiptID, domaincontinuation.KindUserInput, record.ItemID, pending)
	if err != nil {
		return errors.New("timed wait is unavailable")
	}
	h.scheduleRuntimeUserInputWait(id, pending.UserInputTimedWait, receipt.Payload.IssuedAt)
	remaining, ended, release, attached := h.gates.AttachUserInputWait(id)
	if !attached || release == nil {
		return errors.New("timed wait is unavailable")
	}
	defer release()
	if err := snapshot(remaining); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ended:
		return nil
	}
}

type lateUserInputWakeContextKeyV1 struct{}
type lateUserInputWakeV1 struct {
	host                                                                       *runtimeServerHandler
	binding                                                                    continuationapp.UserInputLocalDisplayBindingV1
	attemptID, captureReceiptID, expectedLatestTurnID, baselineDigest, content string
}

func (h *runtimeServerHandler) respondRuntimeLateUserInput(ctx context.Context, request controlapp.UserInputResponse) (controlapp.ActionResult, error) {
	var wake *lateUserInputWakeV1
	result, err := h.prepareRuntimeLateUserInputV1(ctx, request, &wake)
	if err != nil || wake == nil {
		return result, err
	}
	// Preparation has released all old effect/read/outbox locks. Start uses
	// normal current admission and allocates a real turn before queue admission.
	wakeContext := context.WithValue(ctx, lateUserInputWakeContextKeyV1{}, wake)
	_, err = h.startRuntimeTurn(wakeContext, wake.binding.ThreadID, startRuntimeTurnRequest{Async: true})
	if errors.Is(err, domainsecret.ErrProjectionLimitV1) {
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "late_reply_too_large", "message": "The complete Question reply exceeds the current turn resource capacity. Original answers remain saved privately; edit and submit a new attempt.", "inputId": request.InputID}}, nil
	}
	if err != nil {
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "late_reply_unavailable", "message": "late Question idle wake conflicts with current Host authority"}}, nil
	}
	delivery, err := h.store.readUserInputLateReplyV1(wake.binding.ThreadID, wake.binding.InputID, wake.attemptID)
	if err != nil || delivery == nil || delivery.CaptureReceiptID != wake.captureReceiptID {
		return controlapp.ActionResult{}, errors.New("admitted Question wake delivery is unavailable")
	}
	return runtimeLateUserInputResponse(request.InputID, delivery), nil
}

func (h *runtimeServerHandler) prepareRuntimeLateUserInputV1(ctx context.Context, request controlapp.UserInputResponse, wake **lateUserInputWakeV1) (controlapp.ActionResult, error) {
	unavailable := func() (controlapp.ActionResult, error) {
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "late_reply_unavailable", "message": "late Question reply is unavailable under current Host authority"}}, nil
	}
	if ctx == nil || request.LateReply == nil || request.Continue || request.Cancelled || !controlapp.IsOpaqueClientUserMessageID(request.LateReply.ClientUserMessageID) || strings.TrimSpace(request.LateReply.ExpectedTurnID) == "" {
		return unavailable()
	}
	receipt, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, request.InputID)
	if err != nil || receipt.Payload.UserInputReplyCapture != nil || receipt.Payload.Kind != domaincontinuation.KindUserInput || disposition.Status != domaincontinuation.StatusContinued || !domaincontinuation.ValidUserInputContinuedReasonV1(receipt.Payload.UserInputTimedWait, disposition.ReasonCode) {
		return unavailable()
	}
	if !controlapp.UserInputResponseMatchesOwnerV1(request, receipt.Payload.ThreadID, receipt.Payload.TurnID, receipt.Payload.ItemID, receipt.Payload.GateID) {
		return unavailable()
	}
	// Acquire the existing current steering effect before the metadata read
	// and outbox lock. Its exact scope is retained by private view reads and by
	// nested steering admission, avoiding a metadata-read/effect lock upgrade.
	thread, _, err := h.store.ReadThreadStartBaseline(receipt.Payload.ThreadID)
	if err != nil {
		return unavailable()
	}
	turns, _ := thread["turns"].([]any)
	if len(turns) == 0 {
		return unavailable()
	}
	latest, _ := turns[len(turns)-1].(map[string]any)
	latestID := stringField(latest, "id")
	// Exact delivery retry is read under current own authority, even when its
	// original idle expected anchor was superseded by this very wake.
	currentContext, err := appturn.LoadFrozenSecurityContext(h.store, receipt.Payload.ThreadID, latestID)
	if err != nil {
		return unavailable()
	}
	effectContext, releaseEffect, err := h.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, currentContext, false)
	if err != nil || releaseEffect == nil {
		return unavailable()
	}
	defer releaseEffect()
	ctx = effectContext
	selector := displayport.SelectorV1{ThreadID: receipt.Payload.ThreadID, TurnID: receipt.Payload.TurnID, ItemID: receipt.Payload.ItemID, RequestID: receipt.Payload.GateID}
	access := runtimeUserInputDisplayAccessV1{h}
	authorized, release, err := access.acquireCurrent(ctx, selector, &currentContext)
	if err != nil || release == nil {
		return unavailable()
	}
	defer release()
	if authorized.ReceiptID != receipt.ReceiptID || authorized.Status != "continued" || access.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	// One existing Host outbox mutex serializes late attempts and event repair.
	// The durable store mutex separately owns the cross-turn queue reservation.
	h.gateCancellationMu.Lock()
	defer h.gateCancellationMu.Unlock()
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, InputID: selector.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original}
	args := map[string]any{}
	if json.Unmarshal(receipt.Payload.Arguments, &args) != nil {
		return unavailable()
	}
	questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, request.InputID, "User input required")
	alias := ""
	if raw, ok := args["questions"].([]any); ok && len(raw) == 1 {
		if question, ok := raw[0].(map[string]any); ok {
			alias, _ = question["id"].(string)
		}
	}
	_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(request, questions, alias)
	if err != nil || submission == nil {
		return controlapp.ActionResult{StatusCode: http.StatusBadRequest, Body: map[string]any{"code": "validation_error", "message": "invalid late Question answers"}}, nil
	}
	previous, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, request.LateReply.ClientUserMessageID)
	if err != nil {
		return unavailable()
	}
	capture, err := h.continuations.CaptureLateUserInputSubmissionHost(ctx, binding, request.LateReply.ClientUserMessageID, *submission)
	if err != nil {
		return unavailable()
	}
	if previous != nil {
		if previous.CaptureReceiptID != capture.ReceiptID || access.ValidateCurrent(ctx, authorized) != nil {
			return unavailable()
		}
		if err := h.ensureRuntimeLateUserInputEvent(selector.ThreadID, previous); err != nil {
			return unavailable()
		}
		return runtimeLateUserInputResponse(request.InputID, previous), nil
	}
	if request.LateReply.ExpectedTurnID != latestID {
		return unavailable()
	}
	carrier, err := h.verifiedLateUserInputSteeringV1(ctx, binding, request.LateReply.ClientUserMessageID, capture.ReceiptID, latestID)
	if errors.Is(err, domainsteering.ErrUserInputReplyCapacityV1) {
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "late_reply_too_large", "message": "The complete Question reply exceeds the existing inline message capacity. Original answers remain saved privately.", "inputId": request.InputID}}, nil
	}
	if err != nil || access.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	currentThread, baselineDigest, err := h.store.ReadThreadStartBaseline(binding.ThreadID)
	if err != nil {
		return unavailable()
	}
	if runtimeLateQuestionIdleAnchorV1(currentThread, binding.ThreadID, latestID) {
		*wake = &lateUserInputWakeV1{host: h, binding: binding, attemptID: carrier.attemptID, captureReceiptID: carrier.captureReceiptID, expectedLatestTurnID: latestID, baselineDigest: baselineDigest, content: carrier.content}
		return controlapp.ActionResult{}, nil
	}
	// The existing steering path owns classification, current effect authority,
	// terminal arbitration, projection and queue CAS. Only this private carrier
	// marks verified Host metadata; public JSON grants no such exception.
	steeringContext := context.WithValue(ctx, lateUserInputSteeringContextKeyV1{}, carrier)
	result, err := h.runtimeControl().SteerTurn(steeringContext, carrier.requestV1())
	if err != nil || result.StatusCode != http.StatusOK {
		return result, err
	}
	delivery, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, request.LateReply.ClientUserMessageID)
	if err != nil || delivery == nil || delivery.CaptureReceiptID != capture.ReceiptID || access.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	if err := h.ensureRuntimeLateUserInputEvent(selector.ThreadID, delivery); err != nil {
		return unavailable()
	}
	return runtimeLateUserInputResponse(request.InputID, delivery), nil
}

func lateUserInputContentV1(receipt domaincontinuation.Receipt, questions []map[string]any, answers []map[string]string, selectedOptionIDs map[string][]string, project bool) (domainsteering.UserInputReplyIdentityV1, string, error) {
	identity := domainsteering.UserInputReplyIdentityV1{CallID: receipt.Payload.CallID, ToolName: receipt.Payload.ToolName, Questions: []domainsteering.UserInputReplyQuestionIdentityV1{}}
	content := domainsteering.UserInputReplyContentV1{Kind: domainsteering.UserInputReplyContentKindV1, Questions: []domainsteering.UserInputReplyQuestionContentV1{}, Answers: []domainsteering.UserInputReplyAnswerContentV1{}}
	projectText := func(value string) string {
		if project {
			return privacyprojectionapp.ProjectOrdinaryText(value)
		}
		return value
	}
	if len(questions) != len(answers) {
		return identity, "", errors.New("late Question batch association is invalid")
	}
	for index, question := range questions {
		id, _ := question["id"].(string)
		ids := domainsteering.UserInputReplyQuestionIdentityV1{ID: id, OptionIDs: []string{}, SelectedOptionIDs: []string{}}
		questionText, _ := question["question"].(string)
		header, _ := question["header"].(string)
		detail, _ := question["detail"].(string)
		multiSelect, _ := question["multiSelect"].(bool)
		item := domainsteering.UserInputReplyQuestionContentV1{Question: projectText(questionText), Header: projectText(header), Detail: projectText(detail), MultiSelect: multiSelect, Options: []domainsteering.UserInputReplyOptionContentV1{}}
		options, _ := question["options"].([]map[string]string)
		for _, option := range options {
			ids.OptionIDs = append(ids.OptionIDs, option["id"])
			item.Options = append(item.Options, domainsteering.UserInputReplyOptionContentV1{Label: projectText(option["label"]), Description: projectText(option["description"])})
		}
		ids.SelectedOptionIDs = append(ids.SelectedOptionIDs, selectedOptionIDs[id]...)
		answer := answers[index]
		if answer["id"] != id {
			return identity, "", errors.New("late Question answer identity is invalid")
		}
		value := domainsteering.UserInputReplyAnswerContentV1{Selected: []string{}}
		if answer["selectedOptions"] != "" {
			var selected []map[string]string
			if json.Unmarshal([]byte(answer["selectedOptions"]), &selected) != nil {
				return identity, "", errors.New("late Question selected options are invalid")
			}
			for _, option := range selected {
				value.Selected = append(value.Selected, projectText(option["label"]))
			}
		}
		if answer["kind"] != "skip" {
			custom := answer["customText"]
			if answer["kind"] == "" {
				custom = answer["value"]
			}
			value.Custom = projectText(custom)
		}
		identity.Questions = append(identity.Questions, ids)
		content.Questions = append(content.Questions, item)
		content.Answers = append(content.Answers, value)
	}
	identity, err := domainsteering.ParseUserInputReplyIdentityV1(domainsteering.UserInputReplyIdentityMapV1(identity), receipt.Payload.GateID)
	if err != nil {
		return identity, "", err
	}
	encoded, err := json.Marshal(content)
	if err == nil {
		_, err = domainsteering.ParseUserInputReplyContentV1(string(encoded), identity)
	}
	return identity, string(encoded), err
}

// This process-only carrier distinguishes verified Host correlation metadata
// from content. Public JSON and ordinary steering never construct this type.
type lateUserInputSteeringContextKeyV1 struct{}
type lateUserInputSteeringV1 struct {
	identity                                           *domainsteering.UserInputReplyIdentityV1
	host                                               *runtimeServerHandler
	binding                                            continuationapp.UserInputLocalDisplayBindingV1
	attemptID, captureReceiptID, turnID, contextDigest string
	capturedAt                                         time.Time
	text, projectedText, content                       string
}

func (h *runtimeServerHandler) verifiedLateUserInputSteeringV1(ctx context.Context, binding continuationapp.UserInputLocalDisplayBindingV1, attemptID, captureReceiptID, turnID string) (*lateUserInputSteeringV1, error) {
	unavailable := errors.New("late Question steering provenance is unavailable")
	if ctx == nil || ctx.Err() != nil || h == nil || h.continuations == nil || !controlapp.IsOpaqueClientUserMessageID(attemptID) {
		return nil, unavailable
	}
	source, disposition, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, binding.InputID)
	if err != nil || source.ReceiptID != binding.ReceiptID || source.Payload.UserInputReplyCapture != nil || disposition.Status != domaincontinuation.StatusContinued {
		return nil, unavailable
	}
	submission, err := h.continuations.ReadLateUserInputSubmissionHost(ctx, binding, attemptID, captureReceiptID)
	if err != nil {
		return nil, unavailable
	}
	captureID := domaincontinuation.UserInputReplyCaptureGateIDV1(binding.InputID, binding.ReceiptID, attemptID)
	capture, _, err := h.continuations.ResolveTrustedDispositionForAudit(ctx, captureID)
	if err != nil || capture.ReceiptID != captureReceiptID || domaincontinuation.ValidateUserInputReplyCaptureSourceV1(capture, source) != nil {
		return nil, unavailable
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, capture.Payload.UserInputReplyCapture.CapturedAt)
	if err != nil {
		return nil, unavailable
	}
	current, err := appturn.LoadFrozenSecurityContext(h.store, binding.ThreadID, turnID)
	if err != nil || current.ThreadID != binding.ThreadID || current.TenantID != binding.SecurityContext.TenantID || current.UserID != binding.SecurityContext.UserID || current.WorkspaceRealPath != binding.SecurityContext.WorkspaceRealPath {
		return nil, unavailable
	}
	args := map[string]any{}
	if json.Unmarshal(source.Payload.Arguments, &args) != nil {
		return nil, unavailable
	}
	questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, binding.InputID, "User input required")
	request := controlapp.UserInputResponse{}
	var content []string
	// Only content leaves enter classification. Host-derived question/option
	// identifiers are still validated by the original receipt/capture authority.
	for _, question := range questions {
		for _, key := range []string{"question", "header", "detail"} {
			value, _ := question[key].(string)
			content = append(content, value)
		}
		options, _ := question["options"].([]map[string]string)
		for _, option := range options {
			content = append(content, option["label"], option["description"])
		}
	}
	if submission.Format == "structured" {
		if json.Unmarshal(submission.Answers, &request.StructuredAnswers) != nil {
			return nil, unavailable
		}
		for _, answer := range request.StructuredAnswers {
			if answer.CustomText != nil {
				content = append(content, *answer.CustomText)
			}
		}
	} else if submission.Format == "legacy" {
		var legacy []domaincontinuation.UserInputLegacyAnswerV1
		if json.Unmarshal(submission.Answers, &legacy) != nil {
			return nil, unavailable
		}
		request.Answers = make([]map[string]string, 0, len(legacy))
		for _, answer := range legacy {
			record := map[string]string{"id": answer.QuestionID, "value": answer.Value}
			content = append(content, answer.Value)
			if answer.Label != nil {
				record["label"] = *answer.Label
				content = append(content, *answer.Label)
			}
			request.Answers = append(request.Answers, record)
		}
	} else {
		return nil, unavailable
	}
	answers, err := controlapp.NormalizeUserInputAnswers(request, questions, "")
	if err != nil {
		return nil, unavailable
	}
	selectedOptionIDs := map[string][]string{}
	for _, answer := range request.StructuredAnswers {
		selectedOptionIDs[answer.ID] = append([]string{}, answer.OptionIDs...)
	}
	identity, text, err := lateUserInputContentV1(source, questions, answers, selectedOptionIDs, false)
	if err != nil {
		if errors.Is(err, domainsteering.ErrUserInputReplyCapacityV1) {
			return nil, err
		}
		return nil, unavailable
	}
	_, projected, err := lateUserInputContentV1(source, questions, answers, selectedOptionIDs, true)
	if err != nil {
		return nil, unavailable
	}
	return &lateUserInputSteeringV1{identity: &identity, host: h, binding: binding, attemptID: attemptID, captureReceiptID: captureReceiptID, turnID: turnID, contextDigest: current.ContextDigest, capturedAt: capturedAt, text: text, projectedText: projected, content: strings.Join(content, "\n")}, nil
}

func (carrier *lateUserInputSteeringV1) requestV1() controlapp.SteerTurnRequest {
	return controlapp.NormalizeSteerTurnRequest(controlapp.SteerTurnRequest{ThreadID: carrier.binding.ThreadID, TurnID: carrier.turnID, ExpectedTurnID: carrier.turnID, Text: carrier.text, ClientUserMessageID: carrier.attemptID, UserInputID: carrier.binding.InputID, UserInputCaptureReceiptID: carrier.captureReceiptID, UserInputCapturedAt: carrier.capturedAt, UserInputReplyIdentity: carrier.identity})
}

func runtimeLateUserInputResponse(inputID string, delivery *userInputLateReplyRecordV1) controlapp.ActionResult {
	return controlapp.ActionResult{StatusCode: http.StatusOK, Body: map[string]any{"inputId": inputID, "status": "continued", "lateReply": map[string]any{"clientUserMessageId": stringField(delivery.Entry, "clientUserMessageId"), "turnId": delivery.TurnID, "state": delivery.State}}}
}

func (h *runtimeServerHandler) ensureRuntimeLateUserInputEvent(threadID string, delivery *userInputLateReplyRecordV1) error {
	// This event records the original admission. The verified delivery read
	// retains its immutable fields after signed promotion/cancellation; those
	// later states do not change the one admitted event during exact replay.
	admitted := cloneMap(delivery.Entry)
	admitted["status"] = "pending"
	event := controlapp.BuildTurnSteeredEvent(controlapp.TurnSteeredEventInput{Entry: admitted, ThreadID: threadID, TurnID: delivery.TurnID, Text: stringField(delivery.Entry, "text"), DisplayText: stringField(delivery.Entry, "displayText"), ClientUserMessageID: stringField(delivery.Entry, "clientUserMessageId")})
	thread, err := h.store.GetThread(threadID)
	if err != nil {
		return err
	}
	expected, err := appturn.SanitizeCaseEventPublication(thread, event)
	if err != nil {
		return err
	}
	match := func() (bool, error) {
		replay, err := h.store.LoadEventsSince(threadID, 0)
		if err != nil {
			return false, err
		}
		count := 0
		for _, prior := range replay.Events {
			if stringField(prior, "kind") != "turn_steered" || stringField(prior, "threadId") != threadID || stringField(prior, "turnId") != delivery.TurnID || stringField(prior, "clientUserMessageId") != stringField(delivery.Entry, "clientUserMessageId") {
				continue
			}
			candidate := expected
			if prior["steeringEntry"] == nil {
				// Old exact delivery remains idempotent; absent producer metadata
				// is never invented from current entry state during replay.
				candidate = cloneMap(expected)
				delete(candidate, "steeringEntry")
			}
			if !gatecontinuationapp.ExactGateEventProjectionV1(prior, candidate) {
				return false, errors.New("late reply event conflicts")
			}
			count++
		}
		if count > 1 {
			return false, errors.New("late reply has duplicate delivery events")
		}
		return count == 1, nil
	}
	if found, err := match(); err != nil || found {
		return err
	}
	_, _, writeErr := h.store.RecordEvent(event)
	if found, err := match(); err == nil && found {
		return nil
	}
	if writeErr != nil {
		return writeErr
	}
	return errors.New("late reply event readback failed")
}

func runtimeLateQuestionIdleAnchorV1(thread map[string]any, threadID, expectedTurnID string) bool {
	if stringField(thread, "id") != threadID || stringField(thread, "status") != "idle" || expectedTurnID == "" || appturn.ThreadHasActiveTurn(thread) {
		return false
	}
	turns, ok := thread["turns"].([]any)
	if !ok || len(turns) == 0 {
		return false
	}
	latest, ok := turns[len(turns)-1].(map[string]any)
	return ok && stringField(latest, "id") == expectedTurnID && appturn.IsTerminalStatus(stringField(latest, "status"))
}
