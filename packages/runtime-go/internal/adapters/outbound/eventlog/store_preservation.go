package eventlog

import (
	"context"
	"errors"
)

var ErrRestartPreserved = errors.New("event log is preserved after restart")

// NewStoreWithPreservationV1 binds denial before any event writer or recovery
// can run. The immutable scope has no release or late-install operation.
func NewStoreWithPreservationV1(root string, preserved *SemanticRestartPreservationV1) (*Store, error) {
	if err := preserved.Revalidate(context.Background(), root); err != nil {
		return nil, err
	}
	store := NewStore(root)
	store.restartPreserved = preserved
	return store, nil
}
