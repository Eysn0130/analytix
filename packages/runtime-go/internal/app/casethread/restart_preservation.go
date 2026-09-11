package casethread

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrRestartPreserved = errors.New("case thread is preserved after restart")

// Every registry overlay shares this holder, including overlays constructed
// before installation. Installation can only narrow authority and is one-shot.
type restartPreservationV1 struct {
	mu              sync.RWMutex
	threads         map[string]bool
	originalRecords map[string]bool
}

// PreserveRestartScopeV1 installs the startup owner's previously verified
// original-thread scope. It does not classify threads as case or ordinary.
// The complete signed registry must still match its original observation.
func (registry *Registry) PreserveRestartScopeV1(ctx context.Context, threadIDs []string) error {
	if registry == nil || registry.restartPreserved == nil || ctx == nil {
		return errors.New("case restart preservation dependencies are unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	threads := make(map[string]bool, len(threadIDs))
	for _, id := range threadIDs {
		if id == "" || id != strings.TrimSpace(id) || threads[id] {
			return errors.New("case restart preservation scope is invalid")
		}
		threads[id] = true
	}
	registry.restartPreserved.mu.Lock()
	defer registry.restartPreserved.mu.Unlock()
	if registry.restartPreserved.threads != nil {
		return errors.New("case restart preservation is already installed")
	}
	records, err := registry.store.List(ctx)
	if err != nil {
		return err
	}
	observed := newRegistry(registry.authority, registry.store)
	if err := observed.addInventory(ctx, records); err != nil {
		return err
	}
	registry.mu.RLock()
	exact := reflect.DeepEqual(registry.byRecord, observed.byRecord)
	registry.mu.RUnlock()
	if !exact {
		return errors.New("case restart authority inventory changed before preservation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registry.restartPreserved.threads = threads
	registry.restartPreserved.originalRecords = make(map[string]bool, len(observed.byRecord))
	for id := range observed.byRecord {
		registry.restartPreserved.originalRecords[id] = true
	}
	return nil
}

// ObserveOriginalContextForRestartV1 proves only historical private authority
// for held audit observation. It does not grant publication or execution.
// Original membership is shared by all overlays; a planned record cannot
// become original merely by being applied before this observation.
func (registry *Registry) ObserveOriginalContextForRestartV1(ctx context.Context, frozen domainsecurity.TurnSecurityContext) error {
	if registry == nil || registry.restartPreserved == nil || registry.authority == nil || registry.store == nil || ctx == nil || !isCurrentCaseAuthorityContext(frozen) {
		return errors.New("original case context observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registry.restartPreserved.mu.RLock()
	defer registry.restartPreserved.mu.RUnlock()
	held := registry.restartPreserved.threads
	original := registry.restartPreserved.originalRecords
	if !held[frozen.ThreadID] || original == nil {
		return errors.New("original case context is outside restart preservation")
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	records, err := registry.store.List(ctx)
	if err != nil {
		return err
	}
	observed := newRegistry(registry.authority, registry.store)
	if err := observed.addInventory(ctx, records); err != nil {
		return err
	}
	for id := range original {
		if _, exists := observed.byRecord[id]; !exists {
			return errors.New("original case authority record disappeared")
		}
	}
	for id, record := range observed.byRecord {
		if !original[id] && (held[domainsecurity.CaseThreadAuthorityThreadID(record)] || held[strings.TrimSpace(record.ParentThreadID)]) {
			return errors.New("held case authority gained a non-original record")
		}
	}
	committed, found := observed.committed[committedTurnKey(frozen.ThreadID, frozen.TurnID)]
	exactCommitted := found && original[committed.RecordDigest] && committed.SecurityContext != nil && reflect.DeepEqual(*committed.SecurityContext, frozen)
	migration, migrated := registry.migration[frozen.ContextDigest]
	record, recorded := observed.byContext[frozen.ContextDigest]
	exactMigration := migrated && reflect.DeepEqual(migration, frozen) && recorded && original[record.RecordDigest] && record.SecurityContext != nil && reflect.DeepEqual(*record.SecurityContext, frozen)
	if !exactCommitted && !exactMigration {
		return errors.New("held case context lacks exact original committed or migration authority")
	}
	return ctx.Err()
}

func (registry *Registry) RestartPreservesThreadV1(threadID string) bool {
	if registry == nil || registry.restartPreserved == nil {
		return false
	}
	registry.restartPreserved.mu.RLock()
	defer registry.restartPreserved.mu.RUnlock()
	return registry.restartPreserved.threads[strings.TrimSpace(threadID)]
}

// WithRestartRecoveryV1 keeps a startup recovery/readback operation before the
// one-shot preservation installation, or rejects it after installation. The
// callback may read committed inventory, but must not reenter Registry writer,
// preservation, or execution/publication availability methods.
func (registry *Registry) WithRestartRecoveryV1(threadID string, recover func() error) error {
	if recover == nil {
		return errors.New("case restart recovery callback is unavailable")
	}
	unlock, err := registry.lockRestartWriteV1(threadID)
	if err != nil {
		return err
	}
	defer unlock()
	return recover()
}

func (registry *Registry) lockRestartWriteV1(threadIDs ...string) (func(), error) {
	if registry == nil || registry.restartPreserved == nil {
		return nil, errors.New("case restart authority is unavailable")
	}
	registry.restartPreserved.mu.RLock()
	for _, id := range threadIDs {
		if registry.restartPreserved.threads[strings.TrimSpace(id)] {
			registry.restartPreserved.mu.RUnlock()
			return nil, ErrRestartPreserved
		}
	}
	return registry.restartPreserved.mu.RUnlock, nil
}

func (registry *Registry) lockRestartPlanV1(plan MigrationPlan) (func(), error) {
	if err := validateMigrationPlan(plan); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(plan.Contexts))
	for _, frozen := range plan.Contexts {
		ids = append(ids, frozen.ThreadID)
	}
	return registry.lockRestartWriteV1(ids...)
}
