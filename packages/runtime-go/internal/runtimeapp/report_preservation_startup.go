package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"sort"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	usageindexfs "analytix.local/runtime-go/internal/adapters/outbound/usageindexfs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

type runtimeReportRestartPreservationV1 struct {
	report        *pendingworkapp.ReportRestartScopeV1
	durable       *eventlog.SemanticRestartPreservationV1
	core          *runtimeChildIdentityStartupV1
	terminal      *runtimeTerminalSemanticPreservationV1
	acceptedFinal *runtimeAcceptedFinalSemanticPreservationV1
	telemetry     *runtimeTelemetrySemanticPreservationV1
	caseThreads   *runtimeCaseThreadSemanticPreservationV1
	usage         *usageindexfs.RestartPreservationV1
	registry      *runtimeRegistrySemanticPreservationV1
	settlements   *runtimeSettlementSemanticPreservationV1
	attachments   *runtimeAttachmentSemanticPreservationV1
	associated    *runtimeAssociatedSemanticPreservationV1
	publication   *runtimePublicationSemanticPreservationV1
	history       *runtimeOriginalReportHistoryV1
	finalHistory  *runtimeOriginalFinalHistoryV1
}

func (preserved runtimeReportRestartPreservationV1) ValidateSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) error {
	if err := preserved.finalHistory.ValidateSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if preserved.publication != nil {
		if err := preserved.publication.ValidateSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
			return err
		}
	}
	if preserved.history != nil {
		if err := preserved.history.semantic.ValidateSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
			return err
		}
	}
	if preserved.publication != nil && preserved.publication.deferred != nil {
		if err := preserved.publication.deferred.validateSemanticV1(ctx, preserved, operations); err != nil {
			return err
		}
	}
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return nil
	}
	if preserved.durable == nil {
		return errors.New("original report semantic preservation is unavailable")
	}
	if err := preserved.report.RevalidatePrimary(ctx); err != nil {
		return err
	}
	if err := preserved.durable.ValidateSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateUsageSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateCaseThreadSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateChildSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validatePendingSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateTerminalSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateAcceptedFinalSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateTelemetrySemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateRegistrySemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateSettlementSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateAttachmentSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	if err := preserved.validateAssociatedSemanticOperationsV1(ctx, operations, readAfter, noWriteOperationID); err != nil {
		return err
	}
	return preserved.report.RevalidatePrimary(ctx)
}

func prepareRuntimeReportRestartPreservationV1(ctx context.Context, core *runtimeChildIdentityStartupV1) (runtimeReportRestartPreservationV1, error) {
	report, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	if report == nil || len(report.ThreadIDs()) == 0 {
		return runtimeReportRestartPreservationV1{report: report}, nil
	}
	durable, err := eventlog.PrepareSemanticRestartPreservationWithAbsentThreadsV1(ctx, core.roots.DurableDir, filepath.Join(core.roots.DurableDir, "thread_summaries.jsonl"), report.ThreadIDs(), report.AbsentThreadIDsV1(), report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	if err := validateRuntimeOriginalSummaryJournalV1(ctx, core, durable); err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	usage, err := usageindexfs.PrepareRestartPreservationV1(ctx, core.roots.DurableDir, report.DeniedThreadIDsV1())
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	if err := (runtimeReportRestartPreservationV1{core: core, report: report, usage: usage}).validateUsageSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	terminal, err := prepareRuntimeTerminalSemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	acceptedFinal, err := prepareRuntimeAcceptedFinalSemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	telemetry, err := prepareRuntimeTelemetrySemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	caseThreads, err := prepareRuntimeCaseThreadSemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	registry, err := prepareRuntimeRegistrySemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	settlements, err := prepareRuntimeSettlementSemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	attachments, err := prepareRuntimeAttachmentSemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	associated, err := prepareRuntimeAssociatedSemanticPreservationV1(ctx, core, report)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	if err := core.revalidate(ctx); err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	preserved := runtimeReportRestartPreservationV1{report: report, durable: durable, core: core, terminal: terminal, acceptedFinal: acceptedFinal, telemetry: telemetry, caseThreads: caseThreads, usage: usage, registry: registry, settlements: settlements, attachments: attachments, associated: associated}
	if err := preserved.validateChildSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	return preserved, nil
}

// prepareRuntimeReportRestartScopeV1 uses only the full pre-recovery Core
// observation and original primary files. Empty fresh state needs no signing
// key; nonempty state is reverified with its original grant/result graph.
// This prepares denial-only scope, never a capability or recovery decision.
func prepareRuntimeReportRestartScopeV1(ctx context.Context, core *runtimeChildIdentityStartupV1) (*pendingworkapp.ReportRestartScopeV1, error) {
	if ctx == nil {
		return nil, errors.New("report restart observation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if core == nil || core.primaries == nil {
		return nil, errors.New("report restart Core observation is unavailable")
	}
	if err := core.revalidate(ctx); err != nil {
		return nil, err
	}
	if len(core.pendingInventory.Receipts) == 0 && len(core.pendingInventory.Dispositions) == 0 {
		return nil, nil
	}
	if core.verification == nil {
		return nil, errors.New("report restart verification authority is unavailable")
	}
	dispositions := make([]domainpendingwork.PendingWorkDispositionV1, 0, len(core.pendingInventory.Dispositions))
	for _, disposition := range core.pendingInventory.Dispositions {
		dispositions = append(dispositions, disposition)
	}
	sort.Slice(dispositions, func(i, j int) bool { return dispositions[i].WorkID < dispositions[j].WorkID })
	scope, err := pendingworkapp.PlanReportRestartPreservationSnapshotV1(ctx, core.pendingInventory.Receipts, dispositions, core.verification, core.primaries, &runtimeReportInheritedHistoryV1{core: core})
	if err != nil {
		return nil, err
	}
	if len(scope.ThreadIDs()) != 0 {
		scope, err = pendingworkapp.ExtendReportRestartChildScopeV1(ctx, scope, core.pendingInventory, core.jobRecords, core.verification, core.primaries)
		if err != nil {
			return nil, err
		}
		if err := validateRuntimeOriginalScopeJournalV1(ctx, core, &scope); err != nil {
			return nil, err
		}
	}
	if err := core.revalidate(ctx); err != nil {
		return nil, err
	}
	return &scope, nil
}
