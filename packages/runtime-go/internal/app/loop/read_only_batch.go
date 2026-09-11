package loop

import (
	"context"
	"errors"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

// PersistReadOnlyBatchResults attempts every durable settlement even when one
// member fails. The caller must close the enclosing batch authority as failed
// when the returned error is non-nil; a partially settled batch can never
// authorize a provider continuation.
func PersistReadOnlyBatchResults(
	ctx context.Context,
	results []toolcatalogapp.BatchResult[appmodel.PendingToolCall],
	persist func(context.Context, appmodel.PendingToolCall, any, bool) (domainmodel.Message, error),
) ([]domainmodel.Message, error) {
	messages := make([]domainmodel.Message, 0, len(results))
	errs := []error{}
	for _, result := range results {
		message, err := persist(ctx, result.Call, result.Output, result.IsError)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		messages = append(messages, message)
	}
	return messages, errors.Join(errs...)
}
