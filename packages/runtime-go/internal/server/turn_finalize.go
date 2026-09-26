package server

import (
	"context"
	"errors"
	"strings"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

func (h *runtimeServerHandler) runtimeCacheDiagnostics(threadID string, securityContext domainsecurity.TurnSecurityContext, result provider.Result) map[string]any {
	thread, err := h.store.GetThread(threadID)
	if err != nil || thread == nil {
		return provider.RuntimeCacheDiagnosticsWithPrevious(provider.PrefixShape{}, result)
	}
	current, observed := appusage.NewPrefixBaseline(securityContext, result.PrefixShape)
	previous := provider.PrefixShape{}
	var inputDiagnostics map[string]any
	if count := len(result.CacheObservations); count > 0 {
		shape := result.CacheObservations[count-1].Shape
		if shape.Validate() == nil {
			current.WireShape = shape
		}
	}
	if observed {
		h.mu.Lock()
		if h.cachePrefixShapes == nil {
			h.cachePrefixShapes = make(map[string]appusage.PrefixBaseline)
		}
		previous = appusage.ComparablePrefix(h.cachePrefixShapes[threadID], current)
		inputDiagnostics = appusage.ComparableModelInput(h.cachePrefixShapes[threadID], current)
		h.cachePrefixShapes[threadID] = current
		h.mu.Unlock()
	}
	diagnostics := provider.RuntimeCacheDiagnosticsWithPrevious(previous, result)
	for key, value := range inputDiagnostics {
		diagnostics[key] = value
	}
	for key, value := range appusage.ProviderAttemptDiagnostics(result.CacheObservations) {
		diagnostics[key] = value
	}
	if observed {
		diagnostics["cacheBaselineObserved"] = true
		for key, value := range appusage.PrefixBaselineDiagnostics(current) {
			diagnostics[key] = value
		}
	} else {
		diagnostics["cacheBaselineObserved"] = false
	}
	return contextepochapp.AppendThreadDiagnostics(diagnostics, thread)
}

func (h *runtimeServerHandler) latestRuntimeUsageCacheDiagnostics(threadID string, turnID string) (map[string]any, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, errors.New("child usage cache observation identity is unavailable")
	}
	result, err := h.store.LoadEventsSince(threadID, 0)
	if err != nil {
		return nil, err
	}
	if len(result.Diagnostics) != 0 {
		return nil, errors.New("child usage cache event history is unavailable")
	}
	return appusage.LatestCacheDiagnostics(result.Events, turnID), nil
}

func (h *runtimeServerHandler) finalizeRuntimeTurnAfterLoop(ctx context.Context, pending runtimePendingToolCall, loopResult runtimeAgentLoopResult, reason evidenceapp.TerminalReason) error {
	terminalOperationContext, releaseCandidateTerminal, err := takeRuntimeCandidateTerminal(ctx, &loopResult)
	if releaseCandidateTerminal != nil {
		defer releaseCandidateTerminal()
	}
	if err != nil {
		return err
	}
	terminalEnteredAt := time.Now()
	reason = evidenceapp.TerminalReasonForRuntimeResult(loopResult, reason)
	if ctx == nil {
		reason = evidenceapp.TerminalCancel
		loopResult.AssistantText = ""
		loopResult.OrdinaryResult = nil
		loopResult.CandidateUsesCaseData = false
		loopResult.CandidateOrdinaryWork = false
		loopResult.CandidateInputClass = ""
	} else if ctx.Err() != nil {
		reason = evidenceapp.TerminalReasonForFailure(ctx.Err())
		loopResult.AssistantText = ""
		loopResult.OrdinaryResult = nil
		loopResult.CandidateUsesCaseData = false
		loopResult.CandidateOrdinaryWork = false
		loopResult.CandidateInputClass = ""
	} else if apploop.CasePublicationRequired(pending.SecurityContext) && loopResult.CaseSourceUnavailable {
		reason = evidenceapp.TerminalSourceUnavailable
	}
	hostContext, cancel := h.newRuntimeTerminalContextV1(
		terminalOperationContext, terminalEnteredAt, pending.SecurityContext, loopResult, reason, releaseCandidateTerminal,
	)
	defer cancel()
	candidateContext := terminalOperationContext
	if releaseCandidateTerminal != nil {
		candidateContext = hostContext
	}
	if err := requireRuntimeCandidateTerminalHandoffForResult(reason, loopResult, terminalOperationContext, releaseCandidateTerminal); err != nil {
		return err
	}
	var finalizedCase *evidenceapp.PersistCaseBoundaryResult
	var publicationTiming appturn.PublicationTiming
	input := evidenceapp.PersistContinuationInput{
		Finalizer: h.caseFinalizer, Store: h.store, Pending: pending, LoopResult: loopResult, TerminalReason: reason,
		CacheDiagnostics: h.runtimeCacheDiagnostics(pending.ThreadID, pending.SecurityContext, loopResult.LastResult),
		Events:           h.eventsForTerminalTurn(pending.ThreadID, pending.TurnID), AcceptedAt: time.Now().UTC(),
		FinalizeCase: func(_ context.Context, caseInput evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
			var result evidenceapp.PersistCaseBoundaryResult
			var persistErr error
			if appturn.GeneralTerminalReasonAllowsCandidateV1(string(caseInput.TerminalReason)) ||
				apploop.RuntimeResultCarriesOrdinaryCandidate(loopResult) {
				result, persistErr = h.runtimePublicationAuthority().PersistCurrentCaseCandidate(
					candidateContext, caseInput, loopResult.CandidateUsesCaseData,
				)
			} else {
				result, persistErr = h.runtimePublicationAuthority().PersistCurrentCaseFixed(hostContext, caseInput)
			}
			if persistErr == nil {
				copy := result
				finalizedCase = &copy
				publicationTiming = result.Persistence.Timing
			}
			return result, persistErr
		},
		FinalizeGeneral: func(ctx context.Context, general appturn.FinalizeAfterLoopInput) error {
			general.Timing = &publicationTiming
			return h.runtimePublicationAuthority().FinalizeCurrentGeneralAfterLoop(ctx, general)
		}, GeneralOperationContext: candidateContext,
	}
	defer func() {
		// A fixed failure/cancellation terminal is not a successful candidate
		// publication, even when its durable terminal record was committed.
		if appturn.GeneralTerminalReasonAllowsCandidateV1(string(input.TerminalReason)) {
			recordRuntimePublicationTrace(pending.ThreadID, pending.TurnID, loopResult, terminalEnteredAt, publicationTiming)
		}
	}()
	if apploop.CasePublicationRequired(pending.SecurityContext) {
		err := evidenceapp.PersistContinuation(hostContext, input)
		if errors.Is(err, context.Canceled) && h.runtimeControl().TurnInterruptReserved(pending.ThreadID, pending.TurnID) {
			return nil
		}
		if err != nil {
			return err
		}
		return caseentityapp.AppendFinalizedCaseLongitudinalStateV1(
			hostContext, h.caseEntities, h.store, pending.ThreadID, pending.SecurityContext, finalizedCase,
		)
	}
	persistFixed := func() error {
		input.FinalizeGeneral = nil
		input.GeneralOperationContext = nil
		return h.runtimePublicationAuthority().WithCurrentGeneralFixedTerminal(hostContext, pending.SecurityContext, func(persistContext context.Context) error {
			return evidenceapp.PersistContinuation(persistContext, input)
		})
	}
	if !appturn.GeneralTerminalReasonAllowsCandidateV1(string(reason)) {
		err := persistFixed()
		if errors.Is(err, context.Canceled) && h.runtimeControl().TurnInterruptReserved(pending.ThreadID, pending.TurnID) {
			return nil
		}
		return err
	}
	err = evidenceapp.PersistContinuation(hostContext, input)
	if !errors.Is(err, context.Canceled) {
		return err
	}
	if h.runtimeControl().TurnInterruptReserved(pending.ThreadID, pending.TurnID) {
		return nil
	}
	input.TerminalReason = evidenceapp.TerminalCancel
	input.LoopResult.AssistantText = ""
	input.LoopResult.OrdinaryResult = nil
	input.LoopResult.CandidateUsesCaseData = false
	input.LoopResult.CandidateOrdinaryWork = false
	input.LoopResult.CandidateInputClass = ""
	err = persistFixed()
	if errors.Is(err, context.Canceled) && h.runtimeControl().TurnInterruptReserved(pending.ThreadID, pending.TurnID) {
		return nil
	}
	return err
}
