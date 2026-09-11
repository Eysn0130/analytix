package turnterminalstore

import (
	"context"
	"errors"

	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var ErrNotFound = errors.New("turn terminal authority record not found")

type Store interface {
	PutIntentIfAbsent(context.Context, domainturnterminal.TurnTerminalIntentV1) error
	ReadIntent(context.Context, string) (domainturnterminal.TurnTerminalIntentV1, error)
	VisitIntents(context.Context, func(domainturnterminal.TurnTerminalIntentV1) error) error
	PutDispositionIfAbsent(context.Context, domainturnterminal.TurnTerminalDispositionV1) error
	ReadDisposition(context.Context, string) (domainturnterminal.TurnTerminalDispositionV1, error)
	VisitDispositions(context.Context, func(domainturnterminal.TurnTerminalDispositionV1) error) error
	HasRecords(context.Context) (bool, error)
}
