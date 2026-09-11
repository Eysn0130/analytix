package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	controlapp "analytix.local/runtime-go/internal/app/control"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

func (h *runtimeServerHandler) handleThreadTurnAction(w http.ResponseWriter, r *http.Request, rest string) {
	httpapi.TurnActionHandlers{Control: h.runtimeControl()}.HandleThreadTurnAction(w, r, rest)
}

func (h *runtimeServerHandler) steerRuntimeTurn(ctx context.Context, request controlapp.SteerTurnRequest) (controlapp.ActionResult, error) {
	threadID := request.ThreadID
	turnID := request.TurnID
	if _, err := h.store.GetThread(threadID); errors.Is(err, os.ErrNotExist) {
		return controlapp.ActionResult{StatusCode: http.StatusNotFound, Body: map[string]any{"code": "not_found", "message": "thread not found"}}, nil
	} else if err != nil {
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "internal_error", "message": "thread state is unavailable"}}, nil
	}
	securityContext, securityErr := appturn.LoadFrozenSecurityContext(h.store, threadID, turnID)
	if securityErr != nil {
		h.runtimeControl().CancelRegisteredTurn(threadID, turnID)
		if errors.Is(securityErr, os.ErrNotExist) {
			return controlapp.ActionResult{StatusCode: http.StatusNotFound, Body: map[string]any{"code": "not_found", "message": "turn not found"}}, nil
		}
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
			"code": "security_context_invalid", "message": "turn is missing valid frozen security authority",
		}}, nil
	}
	admissionInput := controlapp.SteerSecurityAdmissionInput{
		Context: securityContext, RiskIntent: request.RiskIntent,
		LexicalCaseRisk:   apploop.PromptRequiresCaseRiskAdmission(request.Text + "\n" + request.DisplayText),
		ProtectedCaseData: domainsecurity.ContainsProtectedCaseFactCandidate(request.Text + "\n" + request.DisplayText),
		AttachmentCount:   len(request.AttachmentIDs), FileRefCount: len(request.FileReferences),
	}
	effectBinding := steeringLogicalEffectBinding(request.Text+"\n"+request.DisplayText, admissionInput)
	preAdmission, admissionErr := controlapp.EvaluateSteerSecurityAdmission(admissionInput)
	if admissionErr != nil {
		h.runtimeControl().CancelRegisteredTurn(threadID, turnID)
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
			"code": "security_context_invalid", "message": "turn is missing valid frozen security authority",
		}}, nil
	}
	// Rejection has no durable side effect and does not enter the effect gate.
	// The already-authorized ordinary execution owner remains valid; only this
	// higher-risk/context-changing steer must move to a new turn.
	if preAdmission.RequiresNewTurn {
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: controlapp.NewTurnRequiredResponse(
			threadID, turnID, preAdmission.BlockerCode,
		)}, nil
	}
	// Steering admission is an ordinary host persistence effect. The signed
	// logical binding is enforced later at each provider/tool/data effect, so a
	// lost dataset authority disables only protected case work instead of
	// removing the universal Agent's ordinary steering path.
	effectContext, releaseEffect, effectErr := h.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, securityContext, false)
	if effectErr != nil {
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
			"code": "security_context_invalid", "message": "turn security effect authority is unavailable",
		}}, nil
	}
	defer releaseEffect()
	if currentErr := turnsecurityapp.ValidateCurrentForEffect(turnsecurityapp.CurrentValidationInput{
		OperationContext: effectContext, Identity: h.turnSecurity.Identity,
		Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority,
		SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2,
		Context: securityContext, Workspace: securityContext.WorkspaceRealPath,
	}, false); currentErr != nil {
		h.runtimeControl().CancelRegisteredTurn(threadID, turnID)
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: controlapp.NewTurnRequiredResponse(
			threadID, turnID, turnsecurityapp.CurrentFailureCode(currentErr),
		)}, nil
	}
	admission, admissionErr := controlapp.EvaluateSteerSecurityAdmission(admissionInput)
	if admissionErr != nil {
		h.runtimeControl().CancelRegisteredTurn(threadID, turnID)
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
			"code": "security_context_invalid", "message": "turn is missing valid frozen security authority",
		}}, nil
	}
	if admission.RequiresNewTurn {
		return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: controlapp.NewTurnRequiredResponse(
			threadID, turnID, admission.BlockerCode,
		)}, nil
	}
	releaseSteerAdmission, reservationErr := h.runtimeControl().ReserveSteerAdmission(threadID, turnID)
	if reservationErr != nil || releaseSteerAdmission == nil {
		if errors.Is(reservationErr, controlapp.ErrRuntimeShuttingDown) {
			return controlapp.ActionResult{StatusCode: http.StatusServiceUnavailable, Body: map[string]any{
				"code": "runtime_shutting_down", "message": "runtime is shutting down",
			}}, nil
		}
		if errors.Is(reservationErr, controlapp.ErrTurnTerminalizing) || errors.Is(reservationErr, controlapp.ErrThreadTransition) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{
				"code": "turn_terminalizing", "message": "turn terminal publication is already active",
			}}, nil
		}
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
			"code": "internal_error", "message": "turn steering arbitration is unavailable",
		}}, nil
	}
	defer releaseSteerAdmission()
	request.Text, request.DisplayText, request.FileReferences = privacyprojectionapp.ProjectSteeringContent(request.Text, request.DisplayText, request.FileReferences)
	response, err := controlapp.AdmitSteeringTurn(controlapp.AdmitSteeringInput{
		Store:                 h.store,
		Request:               request,
		ExpectedContextDigest: securityContext.ContextDigest,
		EffectBinding:         effectBinding,
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errDurableTurnNotFound) {
			return controlapp.ActionResult{StatusCode: http.StatusNotFound, Body: map[string]any{"code": "not_found", "message": "turn not found"}}, nil
		}
		if errors.Is(err, errDurableExpectedTurnMismatch) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "turn_mismatch", "message": err.Error(), "threadId": threadID, "turnId": turnID}}, nil
		}
		if errors.Is(err, errDurableContextDigestMismatch) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: controlapp.NewTurnRequiredResponse(
				threadID, turnID, "turn_security_context_invalid",
			)}, nil
		}
		if errors.Is(err, errDurableSteeringProjectionInvalid) || errors.Is(err, errDurableSteeringReplayMismatch) ||
			errors.Is(err, errDurableSteeringIDConflict) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{
				"code": "conflict", "message": "steering admission conflicts with current host authority",
			}}, nil
		}
		if errors.Is(err, errDurableTurnInactive) || errors.Is(err, errDurableTurnNotLatest) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "turn_inactive", "message": err.Error(), "threadId": threadID, "turnId": turnID}}, nil
		}
		if errors.Is(err, errDurableTurnNotSteerable) {
			return controlapp.ActionResult{StatusCode: http.StatusConflict, Body: map[string]any{"code": "turn_not_steerable", "message": err.Error(), "threadId": threadID, "turnId": turnID}}, nil
		}
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "internal_error", "message": err.Error()}}, nil
	}
	return controlapp.ActionResult{StatusCode: http.StatusOK, Body: response}, nil
}

func steeringLogicalEffectBinding(rawText string, admission controlapp.SteerSecurityAdmissionInput) domainsteering.EntryLogicalEffectBinding {
	return controlapp.SteeringLogicalEffectBindingV1(rawText, admission)
}

func (h *runtimeServerHandler) interruptRuntimeTurn(ctx context.Context, request controlapp.InterruptTurnRequest) (controlapp.ActionResult, error) {
	threadID := request.ThreadID
	turnID := request.TurnID
	discard := request.Discard
	securityContext, securityErr := appturn.LoadFrozenSecurityContext(h.store, threadID, turnID)
	if errors.Is(securityErr, appturn.ErrFrozenSecurityContextMissing) {
		caseSensitive, classifyErr := h.threadRequiresCaseFinalGate(threadID)
		if classifyErr != nil || caseSensitive {
			return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
				"code": "security_context_invalid", "message": "case-sensitive turn is missing its frozen security authority",
			}}, nil
		}
	}
	if securityErr != nil && !errors.Is(securityErr, appturn.ErrFrozenSecurityContextMissing) {
		if errors.Is(securityErr, os.ErrNotExist) {
			return controlapp.ActionResult{StatusCode: http.StatusNotFound, Body: map[string]any{"code": "not_found", "message": "turn not found"}}, nil
		}
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "security_context_invalid", "message": "turn security context is unavailable"}}, nil
	}
	if apploop.CasePublicationRequired(securityContext) {
		if err := h.runtimePublicationAuthority().ValidateCaseTerminalLineage(securityContext); err != nil {
			return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
				"code": "security_context_invalid", "message": "case terminal lineage authority is invalid",
			}}, nil
		}
	} else if err := h.runtimePublicationAuthority().ValidateGeneralTerminalLineage(securityContext); err != nil {
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{
			"code": "security_context_invalid", "message": "general terminal lineage authority is invalid",
		}}, nil
	}
	cancelled, releaseInterruptTerminal, quiesceErr := h.runtimeControl().ReserveInterruptTerminalAndWait(ctx, threadID, turnID)
	if quiesceErr != nil {
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "turn_quiescence_failed", "message": quiesceErr.Error()}}, nil
	}
	if releaseInterruptTerminal == nil {
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "turn_quiescence_failed", "message": "interrupt terminal reservation is unavailable"}}, nil
	}
	defer releaseInterruptTerminal()
	terminalEvents := []map[string]any(nil)
	if !discard && !apploop.CasePublicationRequired(securityContext) {
		terminalEvents = h.eventsForTerminalTurn(threadID, turnID)
	}
	drainedGates, gateCancellations, gateDispositionErr := h.settlePendingRuntimeGatesForTurn(threadID, turnID)
	if gateDispositionErr != nil {
		if len(drainedGates) > 0 && !h.gates.RestoreDrained(drainedGates) {
			gateDispositionErr = errors.Join(gateDispositionErr, errors.New("interrupt gate settlement claim could not be retained"))
		}
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "internal_error", "message": gateDispositionErr.Error()}}, nil
	}
	hostContext, hostCancel := newHostAuthorityContext()
	defer hostCancel()
	result, err := controlapp.InterruptActiveTurn(controlapp.InterruptActiveTurnInput{
		Context:                  hostContext,
		Store:                    h.store,
		ThreadID:                 threadID,
		TurnID:                   turnID,
		Discard:                  discard,
		Cancelled:                cancelled,
		TerminalEvents:           terminalEvents,
		GateCancellations:        gateCancellations,
		GateCancellationsSettled: true,
		CaseFinalizer:            h.caseFinalizer,
		CaseContext:              securityContext,
	})
	if err != nil && len(drainedGates) > 0 && !h.gates.RestoreDrained(drainedGates) {
		err = errors.Join(err, errors.New("interrupt terminal claim could not be retained"))
	}
	if err == nil && len(drainedGates) > 0 && !h.gates.CommitDrained(drainedGates) {
		err = errors.New("interrupt terminal claim could not be committed")
	}
	if errors.Is(err, os.ErrNotExist) {
		return controlapp.ActionResult{StatusCode: http.StatusNotFound, Body: map[string]any{"code": "not_found", "message": "turn not found"}}, nil
	} else if err != nil {
		return controlapp.ActionResult{StatusCode: http.StatusInternalServerError, Body: map[string]any{"code": "internal_error", "message": err.Error()}}, nil
	}
	if result.RecordError != nil {
		fmt.Fprintf(os.Stderr, "analytix runtime: failed to record interrupt events thread=%s turn=%s: %v\n", threadID, turnID, result.RecordError)
	}
	return controlapp.ActionResult{StatusCode: http.StatusOK, Body: result.Response}, nil
}

func (h *runtimeServerHandler) threadRequiresCaseFinalGate(threadID string) (bool, error) {
	if h != nil && h.caseThreads != nil && h.caseThreads.IsCaseThread(threadID) {
		return true, nil
	}
	thread, err := h.store.GetThread(threadID)
	if err != nil {
		return false, err
	}
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil {
		return true, err
	}
	return caseSensitive, nil
}
