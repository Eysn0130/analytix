package thread

import (
	"context"
	"errors"

	"analytix.local/runtime-go/internal/contracts"
)

type AcceptedFinalHydrationDependenciesV1 struct {
	ReadAuthorityThread func(string) (map[string]any, error)
	ReadDurableEvents   func(context.Context, string) ([]map[string]any, int, error)
	Projector           PublicProjector
}

// HydrateAcceptedFinalDeliveriesV1 owns the application-level read and
// projection sequence while leaving durable I/O behind injected host seams.
func HydrateAcceptedFinalDeliveriesV1(
	ctx context.Context,
	threadID string,
	projectedThread map[string]any,
	deps AcceptedFinalHydrationDependenciesV1,
) (AcceptedFinalHydrationProjectionV1, error) {
	empty := AcceptedFinalHydrationProjectionV1{}
	if ctx == nil || ctx.Err() != nil || deps.ReadAuthorityThread == nil ||
		deps.ReadDurableEvents == nil || deps.Projector == nil {
		return empty, errors.New("accepted final hydration composition is unavailable")
	}
	latestSeq, ok := contracts.NumericSeq(projectedThread["latestSeq"])
	if !ok || latestSeq < 0 {
		return empty, errors.New("accepted final hydration snapshot frontier is invalid")
	}
	authorityThread, err := deps.ReadAuthorityThread(threadID)
	if err != nil || authorityThread == nil {
		return empty, errors.Join(errors.New("accepted final hydration thread readback is unavailable"), err)
	}
	events, diagnosticCount, err := deps.ReadDurableEvents(ctx, threadID)
	if err != nil || diagnosticCount != 0 {
		return empty, errors.Join(errors.New("accepted final hydration durable replay is unavailable"), err)
	}
	projection, _, err := BuildAcceptedFinalHydrationProjectionV1(AcceptedFinalHydrationInputV1{
		Context: ctx, RouteThreadID: threadID, SnapshotLatestSeq: latestSeq,
		AuthorityThread: authorityThread, ProjectedThread: projectedThread,
		DurableEvents: events, Projector: deps.Projector,
	})
	return projection, err
}

// HydrateLatestAcceptedFinalDeliveryV1 preserves the singular application seam
// while production thread detail uses the complete per-turn projection above.
func HydrateLatestAcceptedFinalDeliveryV1(
	ctx context.Context,
	threadID string,
	projectedThread map[string]any,
	deps AcceptedFinalHydrationDependenciesV1,
) (map[string]any, error) {
	projection, err := HydrateAcceptedFinalDeliveriesV1(ctx, threadID, projectedThread, deps)
	return projection.Latest, err
}
