package runtimeapp

import (
	"context"
	"errors"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func validateRuntimeOriginalSummaryJournalV1(ctx context.Context, core *runtimeChildIdentityStartupV1, durable *eventlog.SemanticRestartPreservationV1) (resultErr error) {
	if core == nil || durable == nil || durable.SummaryRecordsV1() == nil {
		return errors.New("original summary semantic authority is unavailable")
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return err
	}
	if journal == nil {
		return nil
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Revalidate(ctx)) }()
	for _, operation := range journal.OperationsV1() {
		if operation.Path != "durable/thread_summaries.jsonl" {
			continue
		}
		var after []byte
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			after, err = journal.ReadAfterV1(ctx, operation)
			if err != nil {
				return err
			}
		}
		if err := durable.SummaryRecordsV1().ValidateOriginalSemanticOperationV1(ctx, operation, after); err != nil {
			return err
		}
	}
	return nil
}
