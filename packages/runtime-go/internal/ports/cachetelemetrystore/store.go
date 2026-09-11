package cachetelemetrystore

import (
	"context"
	"errors"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
)

var ErrNotFound = errors.New("provider cache telemetry record not found")

type Store interface {
	CommitIntentIfAbsent(context.Context, domaincachetelemetry.ProviderAttemptIntentV1) (domaincachetelemetry.ProviderAttemptIntentV1, error)
	ReadIntent(context.Context, string) (domaincachetelemetry.ProviderAttemptIntentV1, error)
	VisitInventory(
		context.Context,
		func(domaincachetelemetry.ProviderAttemptIntentV1) error,
		func(domaincachetelemetry.ProviderAttemptSettlementV1) error,
		func(domaincachetelemetry.ProviderTurnClosureV1) error,
	) error
	VisitIntents(context.Context, func(domaincachetelemetry.ProviderAttemptIntentV1) error) error
	CommitSettlementIfAbsent(context.Context, domaincachetelemetry.ProviderAttemptSettlementV1) (domaincachetelemetry.ProviderAttemptSettlementV1, error)
	ReadSettlement(context.Context, string) (domaincachetelemetry.ProviderAttemptSettlementV1, error)
	VisitSettlements(context.Context, func(domaincachetelemetry.ProviderAttemptSettlementV1) error) error
	CommitClosureIfAbsent(context.Context, domaincachetelemetry.ProviderTurnClosureV1) (domaincachetelemetry.ProviderTurnClosureV1, error)
	ReadClosure(context.Context, string) (domaincachetelemetry.ProviderTurnClosureV1, error)
	VisitClosures(context.Context, func(domaincachetelemetry.ProviderTurnClosureV1) error) error
	HasRecords(context.Context) (bool, error)
}
