package evidence

import (
	"context"
	"errors"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ContinuationPersistenceStore interface {
	appturn.AcceptedFinalCompletionStore
	appturn.CompletionStore
}

type PersistContinuationInput struct {
	Finalizer               CasePublicationFinalizer
	Store                   ContinuationPersistenceStore
	Pending                 appmodel.PendingToolCall
	LoopResult              apploop.RuntimeAgentLoopResult
	TerminalReason          TerminalReason
	CacheDiagnostics        map[string]any
	Events                  []map[string]any
	AcceptedAt              time.Time
	FinalizeCase            func(context.Context, PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error)
	FinalizeGeneral         func(context.Context, appturn.FinalizeAfterLoopInput) error
	GeneralOperationContext context.Context
}

func PersistContinuation(ctx context.Context, input PersistContinuationInput) error {
	input.TerminalReason = TerminalReasonForRuntimeResult(input.LoopResult, input.TerminalReason)
	if apploop.CasePublicationRequired(input.Pending.SecurityContext) {
		sourceUnavailable := input.TerminalReason == TerminalSourceUnavailable ||
			input.Pending.SecurityContext.PublicationPolicy.BlockerCode == domainsecurity.PublicationBlockerCaseBindingMissing
		reportRequested := apploop.PromptLooksLikeCaseFundReportDelivery(input.Pending.Prompt)
		caseSlotIntent := CaseSlotRequestedV1
		if apploop.RuntimeResultCarriesOrdinaryCandidate(input.LoopResult) &&
			input.Pending.ProviderStepExact && input.Pending.LogicalEffect == domainsecurity.LogicalEffectOrdinary &&
			input.Pending.OrdinaryWork && !input.Pending.CaseSourceUnavailable && !reportRequested {
			caseSlotIntent = CaseSlotNotRequestedV1
		}
		caseInput := PersistCaseBoundaryInput{
			Store: input.Store, Context: input.Pending.SecurityContext, TerminalReason: input.TerminalReason,
			OrdinaryResult:    input.LoopResult.OrdinaryResult,
			CaseSlotIntent:    caseSlotIntent,
			SourceUnavailable: sourceUnavailable,
			ReportRequested:   reportRequested,
			ThreadID:          input.Pending.ThreadID, TurnID: input.Pending.TurnID, Model: input.Pending.Model,
			AcceptedAt: input.AcceptedAt,
			Telemetry:  appusage.NewTerminalTelemetryV1(input.LoopResult.LastResult.Usage, input.CacheDiagnostics),
		}
		if input.FinalizeCase != nil {
			_, err := input.FinalizeCase(ctx, caseInput)
			return err
		}
		if caseInput.OrdinaryResult != nil {
			return errors.New("case continuation ordinary result lacks candidate publication authority")
		}
		_, err := PersistCaseTerminalBoundary(ctx, input.Finalizer, caseInput)
		return err
	}
	if !appturn.GeneralTerminalReasonAllowsCandidateV1(string(input.TerminalReason)) {
		acceptedAt := input.AcceptedAt.UTC()
		if acceptedAt.IsZero() {
			acceptedAt = time.Now().UTC()
		}
		var interrupt *appturn.GeneralTerminalInterruptMetadata
		if input.TerminalReason == TerminalCancel {
			interrupt = &appturn.GeneralTerminalInterruptMetadata{Cancelled: true}
		}
		return appturn.PersistFailure(appturn.PersistFailureInput{
			Store: input.Store, SecurityContext: input.Pending.SecurityContext,
			TerminalReason: string(input.TerminalReason), ThreadID: input.Pending.ThreadID, TurnID: input.Pending.TurnID,
			Model: input.Pending.Model, Failure: GeneralTerminalHostFailure(input.TerminalReason),
			FinishedAt: acceptedAt.Format(time.RFC3339Nano), Events: input.Events,
			Result: input.LoopResult.LastResult, CacheDiagnostics: input.CacheDiagnostics, Interrupt: interrupt,
		})
	}
	if input.FinalizeGeneral == nil {
		return errors.New("current general publication finalizer is unavailable")
	}
	operationContext := input.GeneralOperationContext
	if operationContext == nil {
		return errors.New("current general publication operation context is unavailable")
	}
	if err := operationContext.Err(); err != nil {
		return err
	}
	return input.FinalizeGeneral(operationContext, appturn.FinalizeAfterLoopInput{
		Store: input.Store, SecurityContext: input.Pending.SecurityContext,
		ThreadID: input.Pending.ThreadID, TurnID: input.Pending.TurnID, Model: input.Pending.Model,
		Telemetry:      appusage.NewTerminalTelemetryV1(input.LoopResult.LastResult.Usage, input.CacheDiagnostics),
		TerminalReason: string(input.TerminalReason),
		OrdinaryResult: input.LoopResult.OrdinaryResult,
	})
}
