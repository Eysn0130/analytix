package eventlog

import (
	"context"
	"errors"
	"strings"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// ValidateSemanticOperationsV1 preserves both original and absent shadow
// families. Independent writes to a shared index are allowed only when every
// original held row remains byte-identical. No signed operation is filtered.
func (preserved *SemanticRestartPreservationV1) ValidateSemanticOperationsV1(ctx context.Context, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) error {
	if preserved == nil {
		return errors.New("semantic restart preservation is unavailable")
	}
	if err := preserved.Revalidate(ctx, preserved.root); err != nil {
		return err
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
			continue // Exact After is rechecked by the executor; it cannot write.
		}
		for id := range preserved.threads {
			for _, path := range []string{"durable/threads/" + id, "durable/runtime-go/threads/" + id} {
				if operation.Path == path || strings.HasPrefix(operation.Path, path+"/") {
					return ErrRestartPreserved
				}
				if strings.HasPrefix(path, operation.Path+"/") &&
					(operation.Kind != domainstartup.SemanticOperationCreateDirectory || operation.Before.Type != domainstartup.ManagedEntryTypeAbsent || operation.After.Type != domainstartup.ManagedEntryTypeDirectory) {
					return ErrRestartPreserved
				}
			}
		}
		if operation.Path != "durable/thread_summaries.jsonl" {
			continue
		}
		var body []byte
		switch operation.Kind {
		case domainstartup.SemanticOperationInstallFile:
			if readAfter == nil {
				return errors.New("semantic preserved index after bytes are unavailable")
			}
			var err error
			body, err = readAfter(operation)
			if err != nil {
				return err
			}
		case domainstartup.SemanticOperationRemoveFile:
		case domainstartup.SemanticOperationSetMode:
			continue
		default:
			return ErrRestartPreserved
		}
		if err := preserved.summaries.ValidateReplacementV1(ctx, body); err != nil {
			return err
		}
	}
	return preserved.Revalidate(ctx, preserved.root)
}
