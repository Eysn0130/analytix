package thread

import (
	"context"
	"testing"
)

func TestHydrateLatestAcceptedFinalDeliveryReadsExactOrdinaryFrontierV1(t *testing.T) {
	thread := map[string]any{
		"id": "thread-hydration-service",
		"turns": []any{map[string]any{
			"id": "turn-hydration-service", "threadId": "thread-hydration-service",
			"items": []any{},
		}},
		"latestSeq": float64(1),
	}
	authorityReads, eventReads := 0, 0
	delivery, err := HydrateLatestAcceptedFinalDeliveryV1(
		context.Background(),
		"thread-hydration-service",
		thread,
		AcceptedFinalHydrationDependenciesV1{
			ReadAuthorityThread: func(threadID string) (map[string]any, error) {
				authorityReads++
				return thread, nil
			},
			ReadDurableEvents: func(context.Context, string) ([]map[string]any, int, error) {
				eventReads++
				return []map[string]any{{
					"kind": "turn_started", "threadId": "thread-hydration-service",
					"turnId": "turn-hydration-service", "seq": float64(1),
				}}, 0, nil
			},
			Projector: NewTrustedPublicProjector(nil),
		},
	)
	if err != nil || delivery != nil || authorityReads != 1 || eventReads != 1 {
		t.Fatalf(
			"ordinary hydration changed or skipped exact reads: delivery=%#v authorityReads=%d eventReads=%d err=%v",
			delivery, authorityReads, eventReads, err,
		)
	}
}

func TestHydrateLatestAcceptedFinalDeliveryRejectsReplayDiagnosticsV1(t *testing.T) {
	projected := map[string]any{"id": "thread-hydration-diagnostic", "turns": []any{}, "latestSeq": float64(0)}
	delivery, err := HydrateLatestAcceptedFinalDeliveryV1(
		context.Background(),
		"thread-hydration-diagnostic",
		projected,
		AcceptedFinalHydrationDependenciesV1{
			ReadAuthorityThread: func(string) (map[string]any, error) { return projected, nil },
			ReadDurableEvents: func(context.Context, string) ([]map[string]any, int, error) {
				return nil, 1, nil
			},
			Projector: NewTrustedPublicProjector(nil),
		},
	)
	if err == nil || delivery != nil {
		t.Fatalf("diagnostic replay did not fail closed: delivery=%#v err=%v", delivery, err)
	}
}
