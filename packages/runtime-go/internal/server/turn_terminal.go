package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

var errRuntimeCandidateTerminalHandoffRequired = apploop.ErrCandidateTerminalHandoffRequired

func newHostAuthorityContext() (context.Context, context.CancelFunc) {
	return newHostAuthorityContextFrom(context.Background())
}

// newHostAuthorityContextFrom preserves an already-acquired host effect lease
// while shielding the atomic terminal tail from a late provider cancellation.
// The caller still owns and must release the lease after this context returns.
func newHostAuthorityContextFrom(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
}

// The existing terminal entry supplies the origin before result normalization.
// Budget selection neither acquires authority nor renews a publication deadline.
func (h *runtimeServerHandler) newRuntimeTerminalContextV1(
	parent context.Context,
	enteredAt time.Time,
	securityContext domainsecurity.TurnSecurityContext,
	result runtimeAgentLoopResult,
	reason evidenceapp.TerminalReason,
	releaseCandidate func(),
) (context.Context, context.CancelFunc) {
	timeout := 15 * time.Second
	if parent != nil && parent.Err() == nil && releaseCandidate != nil && h != nil && h.control != nil &&
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) == nil &&
		result.CandidateUsesCaseData && !result.CaseSourceUnavailable && !result.Paused && result.OrdinaryResult == nil &&
		appturn.GeneralTerminalReasonAllowsCandidateV1(string(reason)) &&
		h.control.HasLiveCandidateTerminalForLoop(parent, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest) {
		timeout = 60 * time.Second
	}
	if parent == nil {
		parent = context.Background()
	}
	return context.WithDeadline(context.WithoutCancel(parent), enteredAt.Add(timeout))
}

func takeRuntimeCandidateTerminal(
	fallback context.Context,
	result *runtimeAgentLoopResult,
) (context.Context, func(), error) {
	return apploop.TakeCandidateTerminalHandoff(fallback, result)
}

func requireRuntimeCandidateTerminalHandoff(
	reason evidenceapp.TerminalReason,
	terminalContext context.Context,
	release func(),
) error {
	return apploop.RequireCandidateTerminalHandoff(
		appturn.GeneralTerminalReasonAllowsCandidateV1(string(reason)), terminalContext, release,
	)
}

func requireRuntimeCandidateTerminalHandoffForResult(
	reason evidenceapp.TerminalReason,
	result runtimeAgentLoopResult,
	terminalContext context.Context,
	release func(),
) error {
	return apploop.RequireCandidateTerminalHandoffForResult(
		appturn.GeneralTerminalReasonAllowsCandidateV1(string(reason)), result, terminalContext, release,
	)
}

func (h *runtimeServerHandler) recordRuntimeTurnFailureForPending(pending runtimePendingToolCall, cause error) error {
	return h.recordRuntimeTurnFailureForInput(runtimeAgentLoopInput{ThreadID: pending.ThreadID, TurnID: pending.TurnID, Model: pending.Model, SecurityContext: pending.SecurityContext, Request: startRuntimeTurnRequest{Prompt: pending.Prompt}}, cause)
}

func (h *runtimeServerHandler) recordRuntimeTurnFailureForInput(input runtimeAgentLoopInput, cause error) error {
	hostContext, cancel := newHostAuthorityContext()
	defer cancel()
	return h.persistRuntimeTurnFailureForInput(hostContext, input, cause)
}

func (h *runtimeServerHandler) persistRuntimeTurnFailureForInput(
	persistContext context.Context,
	input runtimeAgentLoopInput,
	cause error,
) error {
	if persistContext == nil {
		return errors.New("runtime failure persistence context is unavailable")
	}
	if errors.Is(cause, context.Canceled) && h.runtimeControl().TurnInterruptReserved(input.ThreadID, input.TurnID) {
		return nil
	}
	persistInput := evidenceapp.PersistRuntimeFailureInput{
		Finalizer: h.caseFinalizer, Store: h.store, Context: input.SecurityContext,
		ThreadID: input.ThreadID, TurnID: input.TurnID, Model: input.Model, Prompt: input.Request.Prompt,
		UsageSource: input.UsageSource, ChildRunID: input.ChildRunID, Cause: cause,
		CacheDiagnostics: func(result provider.Result) map[string]any {
			return h.runtimeCacheDiagnostics(input.ThreadID, input.SecurityContext, result)
		},
		Events: func() []map[string]any { return h.eventsForTerminalTurn(input.ThreadID, input.TurnID) },
		FinalizeCase: func(persistContext context.Context, caseInput evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
			return h.runtimePublicationAuthority().PersistCurrentCaseFixed(persistContext, caseInput)
		},
	}
	if apploop.CasePublicationRequired(input.SecurityContext) {
		err := evidenceapp.PersistRuntimeFailure(persistContext, persistInput)
		if errors.Is(err, context.Canceled) && h.runtimeControl().TurnInterruptReserved(input.ThreadID, input.TurnID) {
			return nil
		}
		return err
	}
	err := h.runtimePublicationAuthority().WithCurrentGeneralFixedTerminal(persistContext, input.SecurityContext, func(fixedContext context.Context) error {
		return evidenceapp.PersistRuntimeFailure(fixedContext, persistInput)
	})
	if errors.Is(err, context.Canceled) && h.runtimeControl().TurnInterruptReserved(input.ThreadID, input.TurnID) {
		return nil
	}
	return err
}

func (h *runtimeServerHandler) eventsForTerminalTurn(threadID, turnID string) []map[string]any {
	result, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_TERMINAL_EVENT_INVENTORY_FAILED")
		return nil
	}
	return result.Events
}
