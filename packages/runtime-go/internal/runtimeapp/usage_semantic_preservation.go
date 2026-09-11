package runtimeapp

import (
	"context"
	"errors"
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func (preserved runtimeReportRestartPreservationV1) validateUsageSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved.core == nil || preserved.report == nil || preserved.usage == nil {
		return errors.New("original usage preservation is unavailable")
	}
	if err := preserved.usage.Revalidate(ctx, preserved.core.roots.DurableDir); err != nil {
		return err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, preserved.core.roots, preserved.core.originalCreateProofV1())
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, preserved.usage.Revalidate(ctx, preserved.core.roots.DurableDir), preserved.report.RevalidatePrimary(ctx))
		if journal != nil {
			resultErr = errors.Join(resultErr, journal.Revalidate(ctx))
		}
	}()
	validate := func(operation domainstartup.SemanticStartupOperationV1, read func(domainstartup.SemanticStartupOperationV1) ([]byte, error)) error {
		const root = "durable/usage_events"
		if operation.Path != root && !strings.HasPrefix(operation.Path, root+"/") && !strings.HasPrefix(root, operation.Path+"/") {
			return nil
		}
		var body []byte
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			if read == nil {
				return errors.New("usage semantic After reader is unavailable")
			}
			var err error
			body, err = read(operation)
			if err != nil {
				return err
			}
		}
		return preserved.usage.ValidateOriginalSemanticOperationV1(ctx, operation, body)
	}
	if journal != nil {
		for _, operation := range journal.OperationsV1() {
			if err := validate(operation, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return err
			}
		}
	}
	for _, operation := range operations {
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue
		}
		if err := validate(operation, readAfter); err != nil {
			return err
		}
	}
	return nil
}
