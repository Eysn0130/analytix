package gatecontinuation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	runtimeports "analytix.local/runtime-go/internal/ports"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	grantregistryport "analytix.local/runtime-go/internal/ports/grantregistry"
)

const (
	statusOK       = 200
	statusNotFound = 404
	statusConflict = 409
	statusInternal = 500
)

type Store interface {
	grantregistryport.Reader
	AppendItemToTurn(string, string, map[string]any) error
	EnsureGateRequestItemExact(string, string, map[string]any) error
	PatchTurnItemStatus(string, string, string, string) error
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type Manager interface {
	RequestApproval(string, string)
	EnsureApprovalPending(string, string) bool
	ResolveApproval(string, string) (int, map[string]any)
	RequestUserInput(string, string)
	EnsureUserInputPending(string, string) bool
	SubmitUserInput(string, []map[string]string) (int, map[string]any)
	CancelUserInput(string) (int, map[string]any)
}

type Dependencies struct {
	Store                   Store
	WorkspaceReader         turnsecurityapp.WorkspaceReader
	SecurityAuthority       turnsecurityapp.WorkspaceSecurityAuthority
	Source                  runtimeports.MCPToolAdvertisementSource
	Registry                *controlapp.GateRegistry[appmodel.PendingToolCall]
	Manager                 Manager
	Continuations           *continuationapp.Service
	AcquireContextEffect    apploop.AcquireToolEffect
	AcquireOrdinaryEffect   apploop.AcquireToolEffect
	ValidateCatalog         func(appmodel.PendingToolCall) error
	ExecuteAndSettle        func(context.Context, appmodel.PendingToolCall) (apploop.SettledToolExecution, error)
	PersistToolResult       func(context.Context, appmodel.PendingToolCall, any, bool) error
	Complete                func(context.Context, appmodel.PendingToolCall, any, bool) (apploop.RuntimeAgentLoopResult, error)
	CompleteSettled         func(context.Context, appmodel.PendingToolCall, apploop.SettledToolExecution) (apploop.RuntimeAgentLoopResult, error)
	Finalize                func(context.Context, appmodel.PendingToolCall, apploop.RuntimeAgentLoopResult, evidenceapp.TerminalReason) error
	RecordFailure           func(appmodel.PendingToolCall, error) error
	TerminalStatus          func(string, string) (string, bool, error)
	RecordCancellations     func([]controlapp.PendingGateCancellation, string) error
	RecordRequest           func(map[string]any) error
	RecordGrantTransition   func(map[string]any) error
	EnsureGrantTransition   func(string, string, map[string]any) error
	RecordResolution        func(map[string]any) error
	TerminalTakeoverPending func(string, string) bool
}

type TurnExecutionController interface {
	RunRegisteredTurn(context.Context, string, string, func(context.Context) controlapp.ActionResult) controlapp.ActionResult
}

func ApproveRegistered(ctx context.Context, request controlapp.ApprovalDecision, dependencies Dependencies, controller TurnExecutionController) controlapp.ActionResult {
	_, pending, _, hasPending := dependencies.Registry.PeekApproval(request.ApprovalID)
	if !hasPending || controller == nil {
		return Approve(ctx, request, dependencies)
	}
	return controller.RunRegisteredTurn(ctx, pending.ThreadID, pending.TurnID, func(turnCtx context.Context) controlapp.ActionResult {
		return Approve(turnCtx, request, dependencies)
	})
}

func RespondUserInputRegistered(ctx context.Context, request controlapp.UserInputResponse, dependencies Dependencies, controller TurnExecutionController) controlapp.ActionResult {
	_, pending, _, hasPending := dependencies.Registry.PeekUserInput(request.InputID)
	if !hasPending || controller == nil {
		return RespondUserInput(ctx, request, dependencies)
	}
	return controller.RunRegisteredTurn(ctx, pending.ThreadID, pending.TurnID, func(turnCtx context.Context) controlapp.ActionResult {
		return RespondUserInput(turnCtx, request, dependencies)
	})
}

func RequestApproval(ctx context.Context, pending appmodel.PendingToolCall, dependencies Dependencies) (string, error) {
	if err := validateRequestDependencies(dependencies); err != nil {
		return "", err
	}
	effectCtx, release, err := acquirePendingEffect(ctx, pending, dependencies)
	if err != nil {
		return "", err
	}
	if effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return "", errors.New("approval request effect authority is unavailable")
	}
	defer release()
	if err := authorizePending(effectCtx, pending, "pending", dependencies); err != nil {
		return "", err
	}
	id := controlapp.SecureGateID("appr", pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	itemID := "item_" + id
	now := time.Now().UTC()
	receipt, err := dependencies.Continuations.IssueOrResolvePendingHost(domaincontinuation.KindApproval, id, itemID, pending, now)
	if err != nil {
		return "", err
	}
	projection, err := ProjectVerifiedGateRequestV1(receipt)
	projectionErr := validateGateRequestProjectionV1(projection)
	if err != nil || projectionErr != nil {
		return "", errors.Join(errors.New("approval request projection is invalid"), err, projectionErr)
	}
	if err := persistAndActivateGateRequest(projection, pending, dependencies); err != nil {
		return "", err
	}
	return id, nil
}

func RequestUserInput(ctx context.Context, pending appmodel.PendingToolCall, dependencies Dependencies) (string, error) {
	if err := validateRequestDependencies(dependencies); err != nil {
		return "", err
	}
	effectCtx, release, err := acquirePendingEffect(ctx, pending, dependencies)
	if err != nil {
		return "", err
	}
	if effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return "", errors.New("user-input request effect authority is unavailable")
	}
	defer release()
	if err := authorizePending(effectCtx, pending, "not_required", dependencies); err != nil {
		return "", err
	}
	id := controlapp.SecureGateID("input", pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	itemID := "item_" + id
	now := time.Now().UTC()
	receipt, err := dependencies.Continuations.IssueOrResolvePendingHost(domaincontinuation.KindUserInput, id, itemID, pending, now)
	if err != nil {
		return "", err
	}
	projection, err := ProjectVerifiedGateRequestV1(receipt)
	projectionErr := validateGateRequestProjectionV1(projection)
	if err != nil || projectionErr != nil {
		return "", errors.Join(errors.New("user-input request projection is invalid"), err, projectionErr)
	}
	if err := persistAndActivateGateRequest(projection, pending, dependencies); err != nil {
		return "", err
	}
	return id, nil
}

func acquirePendingEffect(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	dependencies Dependencies,
) (context.Context, func(), error) {
	acquire := dependencies.AcquireOrdinaryEffect
	if executiongrantapp.CallUsesCaseDataAuthority(pending.Call) || acquire == nil {
		acquire = dependencies.AcquireContextEffect
	}
	return acquire(ctx, pending.SecurityContext)
}

func persistAndActivateGateRequest(
	projection GateRequestProjectionV1,
	pending appmodel.PendingToolCall,
	dependencies Dependencies,
) error {
	if err := validateGateRequestProjectionV1(projection); err != nil {
		return err
	}
	reserve := false
	switch projection.Kind {
	case domaincontinuation.KindApproval:
		reserve = dependencies.Registry.ReservePendingApproval(projection.GateID, projection.Record, pending)
	case domaincontinuation.KindUserInput:
		reserve = dependencies.Registry.ReservePendingUserInput(projection.GateID, projection.Record, pending)
	default:
		return errors.New("gate request projection kind is unsupported")
	}
	if !reserve {
		return errors.New("gate request authority conflicts with an existing reservation")
	}
	// A failure after reservation deliberately retains a non-executable claim.
	// The owning turn's common terminal path drains and settles it; deleting it
	// here would recreate the receipt/item/event orphan window.
	if err := dependencies.Store.EnsureGateRequestItemExact(pending.ThreadID, pending.TurnID, projection.Item); err != nil {
		return fmt.Errorf("persist exact gate request item: %w", err)
	}
	if err := dependencies.RecordRequest(projection.Event); err != nil {
		return fmt.Errorf("persist exact gate request event: %w", err)
	}
	switch projection.Kind {
	case domaincontinuation.KindApproval:
		if !dependencies.Manager.EnsureApprovalPending(projection.GateID, projection.ManagerLabel) {
			return errors.New("approval manager projection conflicts with durable authority")
		}
		if !dependencies.Registry.ActivateReservedApproval(projection.GateID, pending) {
			return errors.New("approval request activation lost its exact reservation")
		}
	case domaincontinuation.KindUserInput:
		if !dependencies.Manager.EnsureUserInputPending(projection.GateID, projection.ManagerLabel) {
			return errors.New("user-input manager projection conflicts with durable authority")
		}
		if !dependencies.Registry.ActivateReservedUserInput(projection.GateID, pending) {
			return errors.New("user-input request activation lost its exact reservation")
		}
	}
	return nil
}

func Approve(ctx context.Context, request controlapp.ApprovalDecision, dependencies Dependencies) controlapp.ActionResult {
	id := request.ApprovalID
	_, _, exists, hasPending := dependencies.Registry.PeekApproval(id)
	if !exists {
		return action(statusNotFound, "not_found", "approval not found: "+id)
	}
	if !hasPending {
		return controlapp.ActionResult{StatusCode: statusConflict, Body: appturn.GateContinuationUnavailableResponse("approval", id)}
	}
	claim, claimed := dependencies.Registry.ClaimApproval(id)
	if !claimed {
		return controlapp.ActionResult{StatusCode: statusConflict, Body: appturn.GateContinuationUnavailableResponse("approval", id)}
	}
	record, pending := claim.Record, claim.Pending
	claimActive := true
	defer func() {
		if !claimActive {
			return
		}
		_, _ = dependencies.Registry.PromoteClaimToTerminal(claim)
	}()
	closeClaim := func(status, reason string, cause error) controlapp.ActionResult {
		claimActive = false
		return closeClaimFailure(ctx, claim, status, reason, cause, dependencies)
	}
	if terminalResult, terminal := terminalClaimResult(ctx, claim, dependencies); terminal {
		claimActive = false
		return terminalResult
	}
	if cause := operationContextCause(ctx, nil); cause != nil {
		return closeClaim(domaincontinuation.StatusInterrupted, terminalCauseReason(cause), cause)
	}
	if request.Decision != "allow" && (dependencies.PersistToolResult == nil || dependencies.Finalize == nil) {
		return closeClaim(domaincontinuation.StatusRejected, "terminal_authority_unavailable", errors.New("gate terminal authority is unavailable"))
	}
	if request.Decision == "allow" {
		if err := authorizePending(ctx, pending, "pending", dependencies); err != nil {
			cause := operationContextCause(ctx, err)
			status, reason := rejectedClaimDisposition(cause)
			return closeClaim(status, reason, cause)
		}
	}
	if cause := operationContextCause(ctx, nil); cause != nil {
		return closeClaim(domaincontinuation.StatusInterrupted, terminalCauseReason(cause), cause)
	}
	decisionAt := time.Now().UTC()
	if request.Decision == "allow" {
		var err error
		if err = dependencies.ValidateCatalog(pending); err == nil {
			_, err = executiongrantapp.AuthorizePendingApproval(
				ctx, dependencies.SecurityAuthority, dependencies.WorkspaceReader, dependencies.Store,
				dependencies.Source, pending, request.Decision, decisionAt,
			)
		}
		if err != nil {
			cause := operationContextCause(ctx, err)
			status, reason := rejectedClaimDisposition(cause)
			return closeClaim(status, reason, cause)
		}
	}
	resolutionStatus, resolutionReason := domaincontinuation.StatusDenied, "approval_denied"
	if request.Decision == "allow" {
		resolutionStatus, resolutionReason = domaincontinuation.StatusAllowed, "approval_allowed"
	}
	disposition, err := dependencies.Continuations.DisposePendingHostDisposition(
		id, resolutionStatus, resolutionReason, decisionAt, pending,
	)
	if err != nil {
		return closeClaim(domaincontinuation.StatusRejected, "continuation_receipt_claim_failed", err)
	}
	approvedGrant := pending.ExecutionGrant
	var transitionItem map[string]any
	var approvalTransition *domainsecurity.ApprovalGrantTransitionV1
	if request.Decision == "allow" {
		if dependencies.RecordGrantTransition == nil || dependencies.EnsureGrantTransition == nil {
			return closeClaim("", "", errors.New("approval grant transition authority is unavailable"))
		}
		receipt, trustedDisposition, derived, resolveErr := dependencies.Continuations.ResolveApprovedGrantHost(id, pending)
		if resolveErr != nil || trustedDisposition != disposition {
			return closeClaim("", "", errors.Join(errors.New("approval grant disposition readback failed"), resolveErr))
		}
		approvedGrant = derived
		transitionedAt, transitionTimeErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
		if transitionTimeErr != nil {
			return closeClaim("", "", transitionTimeErr)
		}
		transition, transitionErr := domainsecurity.NewApprovalGrantTransitionV1(domainsecurity.ApprovalGrantTransitionInputV1{
			Context: pending.SecurityContext, ApprovalID: id, ApprovalItemID: record.ItemID,
			ContinuationReceiptID: receipt.ReceiptID, ContinuationDispositionID: disposition.DispositionID,
			PendingGrant: pending.ExecutionGrant, ApprovedGrant: approvedGrant, TransitionedAt: transitionedAt,
		})
		if transitionErr != nil {
			return closeClaim("", "", transitionErr)
		}
		approvalTransition = &transition
		var transitionEvent map[string]any
		transitionItem, transitionEvent, transitionErr = executiongrantapp.ApprovalTransitionRecords(
			pending.SecurityContext, transition, pending.ExecutionGrant, approvedGrant,
		)
		if transitionErr != nil {
			return closeClaim("", "", transitionErr)
		}
		if transitionErr = dependencies.RecordGrantTransition(transitionEvent); transitionErr != nil {
			return closeClaim("", "", executiongrantapp.ValidationError{Code: "execution_grant_event_persist_failed"})
		}
	}
	marked, ok := dependencies.Registry.MarkClaimDisposition(claim, resolutionStatus, resolutionReason)
	if !ok {
		return closeClaim("", "", errors.New("gate resolution claim could not be retained"))
	}
	claim = marked
	status, _ := dependencies.Manager.ResolveApproval(id, request.Decision)
	if status != statusOK {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: "approval_state_transition_invalid"})
	}
	resolvedStatus := "denied"
	if request.Decision == "allow" {
		resolvedStatus = "allowed"
	}
	if dependencies.RecordResolution == nil {
		return closeClaim("", "", errors.New("approval resolution recorder is unavailable"))
	}
	if err := dependencies.RecordResolution(
		appturn.ApprovalResolvedEvent(controlapp.ApprovalGateCancellation(id, record).Record, resolvedStatus),
	); err != nil {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: "approval_resolution_event_persist_failed"})
	}
	if err := dependencies.Store.PatchTurnItemStatus(record.ThreadID, record.TurnID, record.ItemID, resolvedStatus); err != nil {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: "approval_resolution_item_persist_failed"})
	}
	if request.Decision == "allow" {
		if err := dependencies.EnsureGrantTransition(pending.ThreadID, pending.TurnID, transitionItem); err != nil {
			return closeClaim("", "", executiongrantapp.ValidationError{Code: "execution_grant_registry_transition_invalid"})
		}
	}
	output, isError := any(map[string]any{"code": "approval_denied", "error": "tool execution denied by user"}), true
	var settled apploop.SettledToolExecution
	if request.Decision == "allow" {
		approvalPending := pending
		pending.ExecutionGrant = approvedGrant
		pending.ApprovalTransition = approvalTransition
		if cause := operationContextCause(ctx, nil); cause != nil {
			return closeClaim("", "", cause)
		}
		var err error
		settled, err = executeApprovedTool(ctx, id, approvalPending, pending, dependencies)
		if err != nil {
			return closeClaim("", "", err)
		}
		output, isError = settled.Output, settled.IsError
	}
	if request.Decision == "allow" && isError && authorityFailure(output) {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: failureCode(output)})
	}
	if request.Decision != "allow" {
		if err := dependencies.PersistToolResult(ctx, pending, output, isError); err != nil {
			return closeClaim("", "", err)
		}
		if err := dependencies.Finalize(ctx, pending, apploop.RuntimeAgentLoopResult{}, evidenceapp.TerminalApprovalDenied); err != nil {
			return closeClaim("", "", err)
		}
		if !dependencies.Registry.CommitContinuationClaim(claim) {
			return action(statusInternal, "turn_failed", "approval claim commit failed")
		}
		claimActive = false
		return controlapp.ActionResult{StatusCode: statusOK, Body: map[string]any{
			"approvalId": id, "decision": request.Decision, "status": resolvedStatus,
		}}
	}
	loopResult, err := completeApprovedToolContinuation(ctx, pending, settled, dependencies)
	if err != nil {
		if held, finalizeErr := finalizeHeldRuntimeLoopFailure(ctx, pending, loopResult, err, dependencies); held {
			if finalizeErr != nil {
				return closeClaim("", "", errors.Join(err, finalizeErr))
			}
			if terminalResult, terminal := terminalClaimResult(ctx, claim, dependencies); terminal {
				claimActive = false
				return terminalResult
			}
			return closeClaim("", "", errors.Join(err, errors.New("claimed runtime failure did not reach a terminal state")))
		}
		return closeClaim("", "", err)
	}
	if !loopResult.Paused {
		if err := dependencies.Finalize(ctx, pending, loopResult, evidenceapp.TerminalApproval); err != nil {
			return closeClaim("", "", err)
		}
	}
	if !dependencies.Registry.CommitContinuationClaim(claim) {
		return action(statusInternal, "turn_failed", "approval claim commit failed")
	}
	claimActive = false
	return controlapp.ActionResult{StatusCode: statusOK, Body: map[string]any{
		"approvalId": id, "decision": request.Decision, "status": resolvedStatus,
	}}
}

func closeClaimFailure(
	ctx context.Context,
	claim controlapp.GateClaim[appmodel.PendingToolCall],
	terminalStatus, terminalReason string,
	cause error,
	dependencies Dependencies,
) controlapp.ActionResult {
	cause = operationContextCause(ctx, cause)
	if cause == nil {
		cause = errors.New("pending gate continuation failed")
	}
	drained, ok := dependencies.Registry.PromoteClaimToTerminalWithIntent(
		claim, terminalStatus, terminalReason,
	)
	if !ok {
		return action(statusInternal, "turn_failed", "gate terminal claim promotion failed")
	}
	if isContextTerminalCause(cause) && dependencies.TerminalTakeoverPending != nil &&
		dependencies.TerminalTakeoverPending(claim.Pending.ThreadID, claim.Pending.TurnID) {
		return stoppedClaimResponse(claim.Kind, claim.ID, cause)
	}
	fallbackReason := strings.TrimSpace(terminalReason)
	if fallbackReason == "" {
		fallbackReason = "resolved_continuation_failed"
	}
	hostContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	err := CloseDrainedForTerminal(hostContext, []controlapp.DrainedGate[appmodel.PendingToolCall]{drained}, fallbackReason, cause, dependencies)
	cancel()
	if err != nil {
		if !dependencies.Registry.RestoreDrained([]controlapp.DrainedGate[appmodel.PendingToolCall]{drained}) {
			err = errors.Join(err, errors.New("gate terminal claim could not be retained"))
		}
		return action(statusInternal, "turn_failed", "The pending operation could not be closed safely.")
	}
	if !dependencies.Registry.CommitDrained([]controlapp.DrainedGate[appmodel.PendingToolCall]{drained}) {
		return action(statusInternal, "turn_failed", "gate terminal claim commit failed")
	}
	if isContextTerminalCause(cause) {
		return stoppedClaimResponse(claim.Kind, claim.ID, cause)
	}
	if strings.TrimSpace(terminalStatus) == domaincontinuation.StatusRejected {
		code := executiongrantapp.PublicValidationCode(cause)
		return controlapp.ActionResult{StatusCode: statusConflict, Body: map[string]any{
			"code": code, "message": "The pending operation was rejected by the host security authority.",
			"gateId": claim.ID, "gateKind": claim.Kind, "turnClosed": true,
		}}
	}
	return action(statusInternal, "turn_failed", "The pending operation could not be closed safely.")
}

func terminalClaimResult(
	ctx context.Context,
	claim controlapp.GateClaim[appmodel.PendingToolCall],
	dependencies Dependencies,
) (controlapp.ActionResult, bool) {
	if dependencies.TerminalStatus == nil {
		return closeClaimFailure(
			ctx, claim, domaincontinuation.StatusRejected, "terminal_status_authority_unavailable",
			errors.New("turn terminal status authority is unavailable"), dependencies,
		), true
	}
	status, terminal, err := dependencies.TerminalStatus(claim.Pending.ThreadID, claim.Pending.TurnID)
	if err != nil {
		return closeClaimFailure(
			ctx, claim, domaincontinuation.StatusRejected, "terminal_status_observation_failed", err, dependencies,
		), true
	}
	if !terminal {
		return controlapp.ActionResult{}, false
	}
	reason := "turn_" + strings.TrimSpace(status)
	drained, ok := dependencies.Registry.PromoteClaimToTerminalWithIntent(
		claim, domaincontinuation.StatusRejected, reason,
	)
	if !ok {
		return action(statusInternal, "turn_failed", "terminal gate claim promotion failed"), true
	}
	_, settleErr := settleDrainedGateProjections(
		[]controlapp.DrainedGate[appmodel.PendingToolCall]{drained}, reason, dependencies, false,
	)
	if settleErr != nil {
		if !dependencies.Registry.RestoreDrained([]controlapp.DrainedGate[appmodel.PendingToolCall]{drained}) {
			settleErr = errors.Join(settleErr, errors.New("terminal gate claim could not be retained"))
		}
		return action(statusInternal, "turn_failed", settleErr.Error()), true
	}
	if !dependencies.Registry.CommitDrained([]controlapp.DrainedGate[appmodel.PendingToolCall]{drained}) {
		return action(statusInternal, "turn_failed", "terminal gate claim commit failed"), true
	}
	return controlapp.ActionResult{
		StatusCode: statusConflict,
		Body:       appturn.GateContinuationTerminalResponse(claim.Kind, claim.ID, status),
	}, true
}

func stoppedClaimResponse(kind, id string, cause error) controlapp.ActionResult {
	code := "turn_cancelled"
	if errors.Is(cause, context.DeadlineExceeded) {
		code = "turn_timed_out"
	}
	return controlapp.ActionResult{StatusCode: statusConflict, Body: map[string]any{
		"code": code, "message": "The pending operation was closed by the host terminal authority.",
		"gateId": id, "gateKind": kind, "turnClosed": true,
	}}
}

func terminalCauseReason(cause error) string {
	if errors.Is(cause, context.DeadlineExceeded) {
		return "turn_timed_out"
	}
	return "turn_cancelled"
}

func rejectedClaimDisposition(cause error) (string, string) {
	if isContextTerminalCause(cause) {
		return domaincontinuation.StatusInterrupted, terminalCauseReason(cause)
	}
	return domaincontinuation.StatusRejected, "validation_" + executiongrantapp.PublicValidationCode(cause)
}

func completeApprovedToolContinuation(ctx context.Context, pending appmodel.PendingToolCall, settled apploop.SettledToolExecution, dependencies Dependencies) (apploop.RuntimeAgentLoopResult, error) {
	if decision := apploop.EvaluateProviderContinuation(pending.SecurityContext, pending.ExecutionGrant, pending.Call, settled); decision.Blocked {
		return apploop.RuntimeAgentLoopResult{AssistantText: decision.Boundary}, nil
	}
	if dependencies.CompleteSettled == nil {
		return apploop.RuntimeAgentLoopResult{}, errors.New("approval continuation settlement authority is unavailable")
	}
	return dependencies.CompleteSettled(ctx, pending, settled)
}

func finalizeHeldRuntimeLoopFailure(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	result apploop.RuntimeAgentLoopResult,
	cause error,
	dependencies Dependencies,
) (bool, error) {
	if result.CandidateTerminalContext == nil && result.ReleaseCandidateTerminal == nil {
		return false, nil
	}
	if dependencies.Finalize == nil {
		if result.ReleaseCandidateTerminal != nil {
			result.ReleaseCandidateTerminal()
		}
		return true, errors.New("claimed runtime failure finalizer is unavailable")
	}
	return true, dependencies.Finalize(
		ctx, pending, result, evidenceapp.TerminalReasonForFailure(cause),
	)
}

func executeApprovedTool(
	ctx context.Context,
	approvalID string,
	original appmodel.PendingToolCall,
	pending appmodel.PendingToolCall,
	dependencies Dependencies,
) (apploop.SettledToolExecution, error) {
	if dependencies.ExecuteAndSettle == nil {
		return apploop.SettledToolExecution{}, errors.New("approval tool settlement authority is unavailable")
	}
	_, _, approved, err := dependencies.Continuations.ResolveApprovedGrantHost(approvalID, original)
	if err != nil || approved != pending.ExecutionGrant {
		return apploop.SettledToolExecution{}, errors.New("approved execution lacks its exact signed disposition authority")
	}
	return dependencies.ExecuteAndSettle(ctx, pending)
}

func RespondUserInput(ctx context.Context, request controlapp.UserInputResponse, dependencies Dependencies) controlapp.ActionResult {
	id := request.InputID
	request.Answers = privacyprojectionapp.ProjectStringRecords(request.Answers)
	_, _, exists, hasPending := dependencies.Registry.PeekUserInput(id)
	if !exists {
		return action(statusNotFound, "not_found", "user input not found: "+id)
	}
	if !hasPending {
		return controlapp.ActionResult{StatusCode: statusConflict, Body: appturn.GateContinuationUnavailableResponse("user_input", id)}
	}
	claim, claimed := dependencies.Registry.ClaimUserInput(id)
	if !claimed {
		return controlapp.ActionResult{StatusCode: statusConflict, Body: appturn.GateContinuationUnavailableResponse("user_input", id)}
	}
	record, pending := claim.Record, claim.Pending
	claimActive := true
	defer func() {
		if claimActive {
			_, _ = dependencies.Registry.PromoteClaimToTerminal(claim)
		}
	}()
	closeClaim := func(status, reason string, cause error) controlapp.ActionResult {
		claimActive = false
		return closeClaimFailure(ctx, claim, status, reason, cause, dependencies)
	}
	if terminalResult, terminal := terminalClaimResult(ctx, claim, dependencies); terminal {
		claimActive = false
		return terminalResult
	}
	if cause := operationContextCause(ctx, nil); cause != nil {
		return closeClaim(domaincontinuation.StatusInterrupted, terminalCauseReason(cause), cause)
	}
	if request.Cancelled && (dependencies.PersistToolResult == nil || dependencies.Finalize == nil) {
		return closeClaim(domaincontinuation.StatusRejected, "terminal_authority_unavailable", errors.New("gate terminal authority is unavailable"))
	}
	if !request.Cancelled {
		if err := authorizePending(ctx, pending, "not_required", dependencies); err != nil {
			cause := operationContextCause(ctx, err)
			status, reason := rejectedClaimDisposition(cause)
			return closeClaim(status, reason, cause)
		}
	}
	if cause := operationContextCause(ctx, nil); cause != nil {
		return closeClaim(domaincontinuation.StatusInterrupted, terminalCauseReason(cause), cause)
	}
	receiptStatus, receiptReason := domaincontinuation.StatusSubmitted, "user_input_submitted"
	if request.Cancelled {
		receiptStatus, receiptReason = domaincontinuation.StatusCancelled, "user_input_cancelled"
	}
	if err := dependencies.Continuations.DisposePendingHost(id, receiptStatus, receiptReason, time.Now().UTC(), pending); err != nil {
		return closeClaim(domaincontinuation.StatusRejected, "continuation_receipt_claim_failed", err)
	}
	marked, ok := dependencies.Registry.MarkClaimDisposition(claim, receiptStatus, receiptReason)
	if !ok {
		return closeClaim("", "", errors.New("gate resolution claim could not be retained"))
	}
	claim = marked
	resolvedStatus := "submitted"
	status, response := statusOK, map[string]any{}
	if request.Cancelled {
		status, response = dependencies.Manager.CancelUserInput(id)
		resolvedStatus = "cancelled"
	} else {
		status, response = dependencies.Manager.SubmitUserInput(id, request.Answers)
	}
	if status != statusOK {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: "user_input_state_transition_invalid"})
	}
	if dependencies.RecordResolution == nil {
		return closeClaim("", "", errors.New("user-input resolution recorder is unavailable"))
	}
	if err := dependencies.RecordResolution(appturn.UserInputResolvedEvent(controlapp.UserInputGateCancellation(id, record).Record, resolvedStatus)); err != nil {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: "user_input_resolution_event_persist_failed"})
	}
	if err := dependencies.Store.PatchTurnItemStatus(record.ThreadID, record.TurnID, record.ItemID, resolvedStatus); err != nil {
		return closeClaim("", "", executiongrantapp.ValidationError{Code: "user_input_resolution_item_persist_failed"})
	}
	output := appturn.UserInputResolvedOutput(resolvedStatus, request.Answers, request.Cancelled)
	if request.Cancelled {
		if err := dependencies.PersistToolResult(ctx, pending, output, true); err != nil {
			return closeClaim("", "", err)
		}
		if err := dependencies.Finalize(ctx, pending, apploop.RuntimeAgentLoopResult{}, evidenceapp.TerminalInputCancelled); err != nil {
			return closeClaim("", "", err)
		}
		if !dependencies.Registry.CommitContinuationClaim(claim) {
			return action(statusInternal, "turn_failed", "user-input claim commit failed")
		}
		claimActive = false
		return controlapp.ActionResult{StatusCode: statusOK, Body: response}
	}
	loopResult, err := dependencies.Complete(ctx, pending, output, request.Cancelled)
	if err != nil {
		if held, finalizeErr := finalizeHeldRuntimeLoopFailure(ctx, pending, loopResult, err, dependencies); held {
			if finalizeErr != nil {
				return closeClaim("", "", errors.Join(err, finalizeErr))
			}
			if terminalResult, terminal := terminalClaimResult(ctx, claim, dependencies); terminal {
				claimActive = false
				return terminalResult
			}
			return closeClaim("", "", errors.Join(err, errors.New("claimed runtime failure did not reach a terminal state")))
		}
		return closeClaim("", "", err)
	}
	if !loopResult.Paused {
		if err := dependencies.Finalize(ctx, pending, loopResult, evidenceapp.TerminalUserInput); err != nil {
			return closeClaim("", "", err)
		}
	}
	if !dependencies.Registry.CommitContinuationClaim(claim) {
		return action(statusInternal, "turn_failed", "user-input claim commit failed")
	}
	claimActive = false
	return controlapp.ActionResult{StatusCode: statusOK, Body: response}
}

// CloseDrainedForShutdown settles paused gates only after active turn workers
// have crossed the shutdown barrier. It records cancellations before closing
// each distinct turn, ensuring case turns still reach the Final Evidence Gate.
func CloseDrainedForShutdown(ctx context.Context, drained []controlapp.DrainedGate[appmodel.PendingToolCall], dependencies Dependencies) error {
	return CloseDrainedForTerminal(ctx, drained, "runtime_shutdown", context.Canceled, dependencies)
}

// CloseDrainedForTerminal performs the common host-owned paused-gate close
// used by shutdown and security-context mutations. Gate projections and
// cancellation events settle before each distinct turn reaches its fixed
// Final Evidence Gate path; no model continuation is invoked.
func CloseDrainedForTerminal(
	ctx context.Context,
	drained []controlapp.DrainedGate[appmodel.PendingToolCall],
	reason string,
	cause error,
	dependencies Dependencies,
) error {
	if len(drained) == 0 {
		return nil
	}
	if err := validateSettlementDependencies(dependencies); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("gate terminal settlement reason is unavailable")
	}
	if cause == nil {
		cause = context.Canceled
	}
	bound, ok := dependencies.Registry.BindTerminalIntent(
		drained, domaincontinuation.StatusInterrupted, reason,
	)
	if !ok {
		return errors.New("gate terminal intent could not be frozen")
	}
	drained = bound
	if _, err := SettleDrainedForTerminal(drained, reason, dependencies); err != nil {
		return err
	}
	pendingByTurn := make(map[string]appmodel.PendingToolCall, len(drained))
	turnOrder := make([]string, 0, len(drained))
	for _, gate := range drained {
		turnKey := controlapp.TurnKey(gate.Pending.ThreadID, gate.Pending.TurnID)
		if _, exists := pendingByTurn[turnKey]; !exists {
			pendingByTurn[turnKey] = gate.Pending
			turnOrder = append(turnOrder, turnKey)
		}
	}
	for _, turnKey := range turnOrder {
		pending := pendingByTurn[turnKey]
		if apploop.CasePublicationRequired(pending.SecurityContext) {
			if err := dependencies.Finalize(ctx, pending, apploop.RuntimeAgentLoopResult{}, evidenceapp.TerminalReasonForFailure(cause)); err != nil {
				return err
			}
			continue
		}
		failureCause := cause
		if !isContextTerminalCause(cause) {
			failureCause = apploop.TurnFailureError{
				Message: "pending tool continuation failed current host revalidation",
				Code:    "tool_pending_continuation_invalid", Severity: "warning",
			}
		}
		if err := dependencies.RecordFailure(pending, failureCause); err != nil {
			return err
		}
	}
	return nil
}

// SettleDrainedForTerminal completes the retry-safe gate side of a terminal
// claim before the terminal CAS runs. A consumed continuation receipt, an
// already-resolved UI manager entry, or an already-patched durable item is an
// idempotent retry observation; any other persistence failure is returned so
// the caller can restore the drained in-memory claim.
func SettleDrainedForTerminal(
	drained []controlapp.DrainedGate[appmodel.PendingToolCall],
	reason string,
	dependencies Dependencies,
) ([]controlapp.PendingGateCancellation, error) {
	return settleDrainedGateProjections(drained, reason, dependencies, true)
}

func settleDrainedGateProjections(
	drained []controlapp.DrainedGate[appmodel.PendingToolCall],
	reason string,
	dependencies Dependencies,
	patchItem bool,
) ([]controlapp.PendingGateCancellation, error) {
	if len(drained) == 0 {
		return nil, nil
	}
	reason = strings.TrimSpace(reason)
	if dependencies.Store == nil || dependencies.Manager == nil || dependencies.Continuations == nil ||
		!dependencies.Continuations.Available() || dependencies.RecordCancellations == nil || reason == "" {
		return nil, errors.New("gate terminal settlement dependencies are unavailable")
	}
	if err := validateDrainedTerminalClaims(drained, dependencies.Continuations); err != nil {
		return nil, err
	}
	if dependencies.RecordRequest != nil {
		for _, gate := range drained {
			if err := ensureDrainedGateRequestOutbox(gate, dependencies); err != nil {
				return nil, err
			}
		}
	}
	cancellations := make([]controlapp.PendingGateCancellation, 0, len(drained))
	cancellationsByReason := map[string][]controlapp.PendingGateCancellation{}
	reasonOrder := make([]string, 0, len(drained))
	type gateStatusPatch struct {
		record controlapp.GateRecord
		status string
	}
	patches := make([]gateStatusPatch, 0, len(drained))
	for _, gate := range drained {
		gate, hasDisposition, err := bindTrustedDisposition(gate, dependencies.Continuations)
		if err != nil {
			return nil, err
		}
		if gate.ResolutionStatus != "" || gate.ResolutionReasonCode != "" {
			if err := settleResolvedClaimProjection(gate, dependencies, true); err != nil {
				return nil, err
			}
			continue
		}
		resolvedStatus := "expired"
		var cancellation controlapp.PendingGateCancellation
		switch gate.Kind {
		case "approval":
			// Manager state is a derived UI projection. Ensure repairs an absent
			// request; an existing terminal projection is validated below.
			_ = dependencies.Manager.EnsureApprovalPending(gate.ID, controlapp.GateRecordToolName(gate.Record))
			if err := settleTerminalManager(dependencies.Manager, gate.Kind, gate.ID); err != nil {
				return nil, err
			}
			cancellation = controlapp.ApprovalGateCancellation(gate.ID, gate.Record)
		case "user_input":
			resolvedStatus = "cancelled"
			_ = dependencies.Manager.EnsureUserInputPending(gate.ID, controlapp.GateRecordPrompt(gate.Record))
			if err := settleTerminalManager(dependencies.Manager, gate.Kind, gate.ID); err != nil {
				return nil, err
			}
			cancellation = controlapp.UserInputGateCancellation(gate.ID, gate.Record)
		default:
			return nil, errors.New("terminal settlement encountered an unknown gate kind")
		}
		terminalStatus := domaincontinuation.StatusInterrupted
		terminalReason := reason
		if strings.TrimSpace(gate.TerminalStatus) != "" || strings.TrimSpace(gate.TerminalReasonCode) != "" {
			terminalStatus = strings.TrimSpace(gate.TerminalStatus)
			terminalReason = strings.TrimSpace(gate.TerminalReasonCode)
			if terminalStatus == "" || terminalReason == "" {
				return nil, errors.New("terminal claim disposition intent is incomplete")
			}
		}
		if err := ensureTerminalDisposition(
			gate, terminalStatus, terminalReason, hasDisposition, dependencies.Continuations,
		); err != nil {
			return nil, err
		}
		if patchItem {
			patches = append(patches, gateStatusPatch{record: gate.Record, status: resolvedStatus})
		}
		cancellations = append(cancellations, cancellation)
		if _, exists := cancellationsByReason[terminalReason]; !exists {
			reasonOrder = append(reasonOrder, terminalReason)
		}
		cancellationsByReason[terminalReason] = append(cancellationsByReason[terminalReason], cancellation)
	}
	for _, cancellationReason := range reasonOrder {
		if err := dependencies.RecordCancellations(cancellationsByReason[cancellationReason], cancellationReason); err != nil {
			return nil, err
		}
	}
	for _, patch := range patches {
		alreadyPatched, err := gateItemHasStatus(dependencies.Store, patch.record, patch.status)
		if err != nil {
			return nil, err
		}
		if !alreadyPatched {
			if err := dependencies.Store.PatchTurnItemStatus(
				patch.record.ThreadID, patch.record.TurnID, patch.record.ItemID, patch.status,
			); err != nil {
				return nil, err
			}
		}
	}
	return cancellations, nil
}

func ensureDrainedGateRequestOutbox(
	gate controlapp.DrainedGate[appmodel.PendingToolCall],
	dependencies Dependencies,
) error {
	receipt, err := dependencies.Continuations.ResolvePendingReceiptHost(
		gate.ID, gate.Record.ContinuationReceiptID, gate.Kind, gate.Record.ItemID, gate.Pending,
	)
	if err != nil {
		return err
	}
	projection, err := ProjectVerifiedGateRequestV1(receipt)
	if err != nil || projection.Record != gate.Record {
		return errors.Join(errors.New("terminal gate request projection conflicts with signed authority"), err)
	}
	thread, err := dependencies.Store.GetThread(gate.Record.ThreadID)
	if err != nil {
		return err
	}
	found, err := exactGateRequestItemExists(thread, gate.Record.TurnID, projection.Item)
	if err != nil {
		return err
	}
	if !found {
		if err := dependencies.Store.EnsureGateRequestItemExact(
			gate.Record.ThreadID, gate.Record.TurnID, projection.Item,
		); err != nil {
			return err
		}
	}
	return dependencies.RecordRequest(projection.Event)
}

func exactGateRequestItemExists(thread map[string]any, turnID string, expected map[string]any) (bool, error) {
	expectedID := strings.TrimSpace(firstText(expected["id"]))
	matches := 0
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if strings.TrimSpace(firstText(turn["id"])) != strings.TrimSpace(turnID) {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if strings.TrimSpace(firstText(item["id"])) != expectedID {
				continue
			}
			if !sameGateRequestItemAuthority(item, expected) {
				return false, errors.New("terminal gate request item conflicts with signed authority")
			}
			matches++
		}
		break
	}
	if matches > 1 {
		return false, errors.New("terminal gate request item is duplicated")
	}
	return matches == 1, nil
}

func sameGateRequestItemAuthority(existing, expected map[string]any) bool {
	left := cloneGateRequestMap(existing)
	right := cloneGateRequestMap(expected)
	delete(left, "status")
	delete(left, "finishedAt")
	delete(right, "status")
	delete(right, "finishedAt")
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func cloneGateRequestMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

type approvalDispositionObserver interface {
	ApprovalDisposition(string) (status, decision string, exists bool)
}

type userInputDispositionObserver interface {
	UserInputDisposition(string) (status string, exists bool)
}

func settleResolvedClaimProjection(
	gate controlapp.DrainedGate[appmodel.PendingToolCall],
	dependencies Dependencies,
	patchItem bool,
) error {
	if dependencies.RecordResolution == nil {
		return errors.New("gate resolution projection recorder is unavailable")
	}
	_, disposition, err := dependencies.Continuations.ResolveTrustedDisposition(context.Background(), gate.ID)
	if err != nil || disposition.Status != gate.ResolutionStatus || disposition.ReasonCode != gate.ResolutionReasonCode {
		return errors.New("gate resolution claim does not match its signed disposition")
	}
	projection, ok := domaincontinuation.ResolveProjection(gate.Kind, disposition.Status, disposition.ReasonCode)
	if !ok {
		return errors.New("gate resolution disposition has an invalid kind, status, or reason")
	}
	resolvedStatus := projection.PublicStatus
	var event map[string]any
	switch gate.Kind {
	case "approval":
		decision := projection.Decision
		managerPending := dependencies.Manager.EnsureApprovalPending(gate.ID, controlapp.GateRecordToolName(gate.Record))
		status := statusConflict
		if managerPending {
			status, _ = dependencies.Manager.ResolveApproval(gate.ID, decision)
		}
		if status != statusOK {
			observer, ok := dependencies.Manager.(approvalDispositionObserver)
			observedStatus, observedDecision, exists := "", "", false
			if ok {
				observedStatus, observedDecision, exists = observer.ApprovalDisposition(gate.ID)
			}
			if !exists || observedStatus != resolvedStatus || observedDecision != decision {
				return errors.New("approval manager disposition does not match signed authority")
			}
		}
		event = appturn.ApprovalResolvedEvent(controlapp.ApprovalGateCancellation(gate.ID, gate.Record).Record, resolvedStatus)
	case "user_input":
		managerPending := dependencies.Manager.EnsureUserInputPending(gate.ID, controlapp.GateRecordPrompt(gate.Record))
		switch resolvedStatus {
		case "submitted":
			status := statusNotFound
			if managerPending {
				status, _ = dependencies.Manager.SubmitUserInput(gate.ID, nil)
			}
			if status != statusOK && !managerUserInputMatches(dependencies.Manager, gate.ID, resolvedStatus) {
				return errors.New("user-input manager disposition does not match signed authority")
			}
		case "cancelled":
			status := statusNotFound
			if managerPending {
				status, _ = dependencies.Manager.CancelUserInput(gate.ID)
			}
			if status != statusOK && !managerUserInputMatches(dependencies.Manager, gate.ID, resolvedStatus) {
				return errors.New("user-input manager disposition does not match signed authority")
			}
		default:
			return errors.New("user-input resolution claim has an invalid signed status")
		}
		event = appturn.UserInputResolvedEvent(controlapp.UserInputGateCancellation(gate.ID, gate.Record).Record, resolvedStatus)
	default:
		return errors.New("resolved continuation claim has an unknown gate kind")
	}
	if err := dependencies.RecordResolution(event); err != nil {
		return err
	}
	if patchItem {
		alreadyPatched, err := gateItemHasStatus(dependencies.Store, gate.Record, resolvedStatus)
		if err != nil {
			return err
		}
		if !alreadyPatched {
			if err := dependencies.Store.PatchTurnItemStatus(
				gate.Record.ThreadID, gate.Record.TurnID, gate.Record.ItemID, resolvedStatus,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindTrustedDisposition(
	gate controlapp.DrainedGate[appmodel.PendingToolCall],
	continuations *continuationapp.Service,
) (controlapp.DrainedGate[appmodel.PendingToolCall], bool, error) {
	_, disposition, err := continuations.ResolveTrustedDisposition(context.Background(), gate.ID)
	if errors.Is(err, continuationstoreport.ErrNotFound) {
		if gate.ResolutionStatus != "" || gate.ResolutionReasonCode != "" {
			return gate, false, errors.New("gate resolution claim has no signed disposition")
		}
		return gate, false, nil
	}
	if err != nil {
		return gate, false, err
	}
	projection, isResolution := domaincontinuation.ResolveProjection(gate.Kind, disposition.Status, disposition.ReasonCode)
	if gate.ResolutionStatus != "" || gate.ResolutionReasonCode != "" {
		if !isResolution || disposition.Status != gate.ResolutionStatus || disposition.ReasonCode != gate.ResolutionReasonCode {
			return gate, true, errors.New("gate resolution claim does not match its signed disposition")
		}
		return gate, true, nil
	}
	if isResolution {
		_ = projection
		gate.ResolutionStatus = disposition.Status
		gate.ResolutionReasonCode = disposition.ReasonCode
	}
	return gate, true, nil
}

func ensureTerminalDisposition(
	gate controlapp.DrainedGate[appmodel.PendingToolCall],
	status, reason string,
	hasDisposition bool,
	continuations *continuationapp.Service,
) error {
	if !hasDisposition {
		err := continuations.DisposePendingHost(gate.ID, status, reason, time.Now().UTC(), gate.Pending)
		if err == nil {
			return nil
		}
		if !errors.Is(err, continuationapp.ErrReceiptConsumed) {
			return err
		}
	}
	_, disposition, err := continuations.ResolveTrustedDisposition(context.Background(), gate.ID)
	if err != nil {
		return err
	}
	if disposition.Status != strings.TrimSpace(status) || disposition.ReasonCode != strings.TrimSpace(reason) {
		return errors.New("terminal claim conflicts with its signed disposition")
	}
	return nil
}

func managerUserInputMatches(manager Manager, id, expectedStatus string) bool {
	observer, ok := manager.(userInputDispositionObserver)
	if !ok {
		return false
	}
	status, exists := observer.UserInputDisposition(id)
	return exists && status == expectedStatus
}

func validateDrainedTerminalClaims(
	drained []controlapp.DrainedGate[appmodel.PendingToolCall],
	continuations *continuationapp.Service,
) error {
	seen := make(map[string]struct{}, len(drained))
	for _, gate := range drained {
		if gate.ClaimToken == 0 {
			return errors.New("terminal claim token is unavailable")
		}
		if _, exists := seen[strings.TrimSpace(gate.ID)]; exists {
			return errors.New("terminal claim batch contains a duplicate gate")
		}
		seen[strings.TrimSpace(gate.ID)] = struct{}{}
		if (strings.TrimSpace(gate.ResolutionStatus) == "") != (strings.TrimSpace(gate.ResolutionReasonCode) == "") {
			return errors.New("terminal claim resolution intent is incomplete")
		}
		if gate.ResolutionStatus != "" {
			if _, ok := domaincontinuation.ResolveProjection(gate.Kind, gate.ResolutionStatus, gate.ResolutionReasonCode); !ok {
				return errors.New("terminal claim resolution intent is invalid")
			}
		}
		if (strings.TrimSpace(gate.TerminalStatus) == "") != (strings.TrimSpace(gate.TerminalReasonCode) == "") {
			return errors.New("terminal claim disposition intent is incomplete")
		}
		if gate.TerminalStatus != "" && gate.TerminalStatus != domaincontinuation.StatusInterrupted && gate.TerminalStatus != domaincontinuation.StatusRejected {
			return errors.New("terminal claim disposition status is invalid")
		}
		kind := ""
		switch gate.Kind {
		case "approval":
			kind = domaincontinuation.KindApproval
			if strings.TrimSpace(gate.Record.ToolName) != strings.TrimSpace(gate.Pending.Call.Name) {
				return errors.New("approval terminal claim tool identity is invalid")
			}
		case "user_input":
			kind = domaincontinuation.KindUserInput
		default:
			return errors.New("terminal settlement encountered an unknown gate kind")
		}
		if strings.TrimSpace(gate.ID) == "" ||
			strings.TrimSpace(gate.Record.ThreadID) != strings.TrimSpace(gate.Pending.ThreadID) ||
			strings.TrimSpace(gate.Record.TurnID) != strings.TrimSpace(gate.Pending.TurnID) ||
			strings.TrimSpace(gate.Record.ItemID) == "" ||
			strings.TrimSpace(gate.Record.ContinuationReceiptID) == "" {
			return errors.New("terminal claim record does not match its pending authority")
		}
		if err := continuations.ValidatePendingHost(
			gate.ID, gate.Record.ContinuationReceiptID, kind, gate.Record.ItemID, gate.Pending,
		); err != nil {
			return fmt.Errorf("terminal claim signed authority is invalid: %w", err)
		}
	}
	return nil
}

func settleTerminalManager(manager Manager, kind, id string) error {
	if manager == nil {
		return errors.New("gate terminal manager is unavailable")
	}
	status := 0
	body := map[string]any(nil)
	switch kind {
	case "approval":
		status, body = manager.ResolveApproval(id, "deny")
		if status == statusOK {
			return nil
		}
		if status == statusConflict && strings.TrimSpace(fmt.Sprint(body["code"])) == "approval_not_pending" {
			observer, ok := manager.(approvalDispositionObserver)
			observedStatus, decision, exists := "", "", false
			if ok {
				observedStatus, decision, exists = observer.ApprovalDisposition(id)
			}
			if exists && observedStatus == "denied" && decision == "deny" {
				return nil
			}
		}
	case "user_input":
		status, body = manager.CancelUserInput(id)
		if status == statusOK {
			return nil
		}
		if status == statusNotFound && strings.TrimSpace(fmt.Sprint(body["code"])) == "user_input_not_pending" &&
			managerUserInputMatches(manager, id, "cancelled") {
			return nil
		}
	default:
		return errors.New("terminal settlement encountered an unknown gate kind")
	}
	return fmt.Errorf("gate terminal manager settlement failed: kind=%s status=%d", kind, status)
}

func gateItemHasStatus(store Store, record controlapp.GateRecord, expected string) (bool, error) {
	thread, err := store.GetThread(record.ThreadID)
	if err != nil {
		return false, err
	}
	for _, turn := range gateRecordList(thread["turns"]) {
		if strings.TrimSpace(fmt.Sprint(turn["id"])) != strings.TrimSpace(record.TurnID) {
			continue
		}
		for _, item := range gateRecordList(turn["items"]) {
			if strings.TrimSpace(fmt.Sprint(item["id"])) == strings.TrimSpace(record.ItemID) {
				return strings.TrimSpace(fmt.Sprint(item["status"])) == strings.TrimSpace(expected), nil
			}
		}
		return false, errors.New("pending gate item is unavailable")
	}
	return false, errors.New("pending gate turn is unavailable")
}

func gateRecordList(value any) []map[string]any {
	switch records := value.(type) {
	case []map[string]any:
		return records
	case []any:
		out := make([]map[string]any, 0, len(records))
		for _, raw := range records {
			if record, ok := raw.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func operationContextCause(ctx context.Context, fallback error) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return fallback
}

func isContextTerminalCause(cause error) bool {
	return errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded)
}

func authorizePending(ctx context.Context, pending appmodel.PendingToolCall, approvalState string, dependencies Dependencies) error {
	if err := validatePriorSettledToolRefs(pending, dependencies); err != nil {
		return err
	}
	if err := dependencies.ValidateCatalog(pending); err != nil {
		return err
	}
	return executiongrantapp.AuthorizePending(ctx, dependencies.SecurityAuthority, dependencies.WorkspaceReader, dependencies.Store, dependencies.Source, pending, approvalState, time.Now().UTC())
}

func validatePriorSettledToolRefs(pending appmodel.PendingToolCall, dependencies Dependencies) error {
	thread, err := dependencies.Store.GetThread(pending.ThreadID)
	if err != nil {
		return err
	}
	return executiongrantapp.ValidatePriorSettledToolReferences(pending.ThreadID, thread, pending.SecurityContext, pending.ExecutionGrant, pending.PriorSettledToolRefs)
}

func authorityFailure(output any) bool {
	code := failureCode(output)
	return strings.HasPrefix(code, "execution_grant_") || strings.HasPrefix(code, "turn_security_") || code == "mcp_host_context_invalid" ||
		code == "mcp_connection_epoch_mismatch" || code == "mcp_source_probe_mismatch"
}

func failureCode(output any) string {
	record, _ := output.(map[string]any)
	code, _ := record["code"].(string)
	return strings.TrimSpace(code)
}

func validateRequestDependencies(dependencies Dependencies) error {
	if dependencies.Store == nil || dependencies.SecurityAuthority.Observer == nil || dependencies.Registry == nil || dependencies.Manager == nil || dependencies.Continuations == nil || !dependencies.Continuations.Available() || dependencies.AcquireContextEffect == nil || dependencies.ValidateCatalog == nil || dependencies.RecordRequest == nil {
		return errors.New("gate continuation dependencies are unavailable")
	}
	return nil
}

func validateSettlementDependencies(dependencies Dependencies) error {
	if dependencies.Store == nil || dependencies.Registry == nil || dependencies.Manager == nil ||
		dependencies.Continuations == nil || !dependencies.Continuations.Available() ||
		dependencies.Finalize == nil || dependencies.RecordFailure == nil || dependencies.RecordCancellations == nil {
		return errors.New("gate continuation settlement dependencies are unavailable")
	}
	return nil
}

func action(status int, code, message string) controlapp.ActionResult {
	if status >= statusInternal {
		message = controlapp.SafeInternalControlMessage
	}
	return controlapp.ActionResult{StatusCode: status, Body: map[string]any{"code": code, "message": message}}
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}
