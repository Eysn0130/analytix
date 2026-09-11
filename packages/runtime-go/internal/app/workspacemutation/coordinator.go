package workspacemutation

import (
	"context"
	"errors"
	"sync"
)

// Coordinator is the single host-owned exclusion boundary for provider file
// mutations and checkpoint rewind. A global lease is intentionally stricter
// than per-path locking: danger-full-access paths can cross workspace roots,
// so a workspace-only key would leave an alias/TOCTOU gap.
type Coordinator struct {
	once  sync.Once
	lease chan struct{}
}

func NewCoordinator() *Coordinator {
	coordinator := &Coordinator{}
	coordinator.initialize()
	return coordinator
}

func (coordinator *Coordinator) Acquire(ctx context.Context) (func(), error) {
	if coordinator == nil {
		return nil, errors.New("workspace mutation coordinator is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("workspace mutation context is required")
	}
	coordinator.initialize()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case coordinator.lease <- struct{}{}:
	}
	var once sync.Once
	return func() {
		once.Do(func() { <-coordinator.lease })
	}, nil
}

func (coordinator *Coordinator) initialize() {
	coordinator.once.Do(func() {
		coordinator.lease = make(chan struct{}, 1)
	})
}
