package turnterminal

import (
	"context"
	"errors"
	"reflect"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrRestartPreserved = errors.New("turn terminal authority is preserved after restart")

type restartPreservationV1 struct {
	threads  map[string]bool
	contexts map[string]domainsecurity.TurnSecurityContext
}

// PreserveRestartContextsV1 propagates a complete scope already proved by the
// startup owner. It supplies denial only, before any terminal consumer starts;
// it neither certifies the dependency graph nor changes durable authority.
func (coordinator *Coordinator) PreserveRestartContextsV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) error {
	if coordinator == nil || ctx == nil {
		return errors.New("turn terminal preservation owner is unavailable")
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.restartPreserved != nil || coordinator.restartEffectsStarted {
		return errors.New("turn terminal preservation installation is closed")
	}
	scope := &restartPreservationV1{threads: map[string]bool{}, contexts: map[string]domainsecurity.TurnSecurityContext{}}
	for _, frozen := range contexts {
		if err := domainsecurity.ValidateTurnSecurityContext(frozen); err != nil || frozen.Version != domainsecurity.TurnSecurityContextVersionV2 {
			return errors.New("turn terminal preservation context is invalid")
		}
		if _, duplicate := scope.contexts[frozen.ContextDigest]; duplicate {
			return errors.New("turn terminal preservation context is duplicated")
		}
		scope.contexts[frozen.ContextDigest] = frozen
		scope.threads[frozen.ThreadID] = true
	}
	if err := VerifyTrustedInventoryV1(ctx, coordinator.terminals, coordinator.authority); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	coordinator.restartPreserved = scope
	return nil
}

func (scope *restartPreservationV1) ownsThread(threadID string) bool {
	return scope != nil && scope.threads[threadID]
}

func (scope *restartPreservationV1) validateContext(frozen domainsecurity.TurnSecurityContext) error {
	if !scope.ownsThread(frozen.ThreadID) {
		return nil
	}
	if expected, found := scope.contexts[frozen.ContextDigest]; !found || !reflect.DeepEqual(expected, frozen) {
		return errors.New("turn terminal preserved context is outside the proved original inventory")
	}
	return nil
}

func (coordinator *Coordinator) beginRestartRecoveryV1() *restartPreservationV1 {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.restartEffectsStarted = true
	return coordinator.restartPreserved // Immutable after this phase closes installation.
}
