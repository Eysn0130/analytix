package evidence

import (
	"context"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type RuntimeFailureStore interface {
	appturn.AcceptedFinalCompletionStore
	appturn.FailureStore
}

type PersistRuntimeFailureInput struct {
	Finalizer        CasePublicationFinalizer
	Store            RuntimeFailureStore
	Context          domainsecurity.TurnSecurityContext
	ThreadID         string
	TurnID           string
	Model            string
	Prompt           string
	UsageSource      string
	ChildRunID       string
	Cause            error
	At               time.Time
	CacheDiagnostics func(domainmodel.Result) map[string]any
	Events           func() []map[string]any
	FinalizeCase     func(context.Context, PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error)
}

func PersistRuntimeFailure(ctx context.Context, input PersistRuntimeFailureInput) error {
	failure := apploop.NormalizeRuntimeFailure(input.Cause)
	terminalReason := TerminalReasonForFailure(failure.Cause)
	at := input.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	cacheDiagnostics := map[string]any{}
	result := failure.Result.LastResult
	if input.CacheDiagnostics != nil && runtimeResultHasDiagnostics(result) {
		cacheDiagnostics = input.CacheDiagnostics(result)
	}
	if apploop.CasePublicationRequired(input.Context) {
		caseInput := PersistCaseBoundaryInput{
			Store: input.Store, Context: input.Context, TerminalReason: terminalReason,
			SourceUnavailable: terminalReason == TerminalSourceUnavailable,
			ReportRequested:   apploop.PromptLooksLikeCaseFundReportDelivery(input.Prompt),
			ThreadID:          input.ThreadID, TurnID: input.TurnID, Model: input.Model, AcceptedAt: at,
			Telemetry:   appusage.NewTerminalTelemetryV1(result.Usage, cacheDiagnostics),
			UsageSource: input.UsageSource, ChildRunID: input.ChildRunID,
		}
		if input.FinalizeCase != nil {
			_, err := input.FinalizeCase(ctx, caseInput)
			return err
		}
		_, err := PersistCaseTerminalBoundary(ctx, input.Finalizer, caseInput)
		return err
	}
	events := []map[string]any(nil)
	if input.Events != nil {
		events = input.Events()
	}
	var interrupt *appturn.GeneralTerminalInterruptMetadata
	if terminalReason == TerminalCancel {
		interrupt = &appturn.GeneralTerminalInterruptMetadata{Cancelled: true}
	}
	return appturn.PersistFailure(appturn.PersistFailureInput{
		Store: input.Store, SecurityContext: input.Context, TerminalReason: string(terminalReason),
		ThreadID: input.ThreadID, TurnID: input.TurnID, Model: input.Model,
		Failure:    failure.Public,
		FinishedAt: at.Format(time.RFC3339Nano), Events: events, Result: result, CacheDiagnostics: cacheDiagnostics,
		UsageSource: input.UsageSource, ChildRunID: input.ChildRunID, Interrupt: interrupt,
	})
}

func runtimeResultHasDiagnostics(result domainmodel.Result) bool {
	return result.PrefixShape.PrefixHash != "" || result.Usage.TotalTokens > 0 || result.Usage.PromptTokens > 0 || result.Usage.CompletionTokens > 0 ||
		len(result.CacheObservations) > 0
}
