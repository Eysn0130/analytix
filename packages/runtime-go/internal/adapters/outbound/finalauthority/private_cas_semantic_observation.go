package finalauthority

import (
	"context"
	"errors"
	"sync"
)

type privateCASSemanticObservationContextKeyV1 struct{}

// This scope admits only fresh prepared inventory observation while its owner
// already holds recovery exclusion. It supplies no filesystem access, live
// generation, mutation permission or recovery authority.
type privateCASSemanticObservationScopeV1 struct {
	mu      sync.Mutex
	barrier *privateCASLiveRecoveryBarrier
	active  bool
	readers int
	drained chan struct{}
}

func (barrier *privateCASLiveRecoveryBarrier) withSemanticObservationV1(ctx context.Context, apply func(context.Context) error) error {
	barrier.mu.Lock()
	held := barrier.recovery
	barrier.mu.Unlock()
	if !held || apply == nil {
		return errors.New("private CAS semantic observation requires held recovery exclusion")
	}
	scope := &privateCASSemanticObservationScopeV1{barrier: barrier, active: true, drained: make(chan struct{})}
	observationContext, cancel := context.WithCancel(ctx)
	defer func() {
		scope.mu.Lock()
		scope.active = false
		if scope.readers == 0 {
			close(scope.drained)
		}
		scope.mu.Unlock()
		cancel()
		// Never release the owner's exclusion while an admitted observer can
		// still touch its bound files, including error and panic unwinding.
		<-scope.drained
	}()
	return apply(context.WithValue(observationContext, privateCASSemanticObservationContextKeyV1{}, scope))
}

func (barrier *privateCASLiveRecoveryBarrier) acquireObservationV1(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, errors.New("private CAS observation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value := ctx.Value(privateCASSemanticObservationContextKeyV1{})
	if value == nil {
		if err := barrier.acquireLive(ctx); err != nil {
			return nil, err
		}
		return barrier.releaseLive, nil
	}
	scope, ok := value.(*privateCASSemanticObservationScopeV1)
	if !ok || scope == nil || scope.barrier != barrier {
		return nil, errors.New("private CAS semantic observation scope is invalid")
	}
	scope.mu.Lock()
	if !scope.active {
		scope.mu.Unlock()
		return nil, errors.New("private CAS semantic observation scope expired")
	}
	scope.readers++
	scope.mu.Unlock()
	return func() {
		scope.mu.Lock()
		defer scope.mu.Unlock()
		scope.readers--
		if scope.readers == 0 && !scope.active {
			close(scope.drained)
		}
	}, nil
}
