package effectgate

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Gate serializes effect starts and authority transitions for both the parent
// thread and every thread sharing a workspace. Effects take read leases;
// context acceptance and pending-work disposition take write leases.
type Gate struct {
	mu     sync.Mutex
	scopes map[string]*scopeLock
}

type scopeLock struct {
	mu             sync.Mutex
	refs           int
	readers        int
	writer         bool
	waitingWriters int
	changed        chan struct{}
}

type effectLease struct {
	gate               *Gate
	contextDigest      string
	keyDigest          string
	threadKey          string
	workspaceKey       string
	workspaceAuthority string
	ordinaryOnly       bool
	mu                 sync.Mutex
	references         int
	unlock             func()
	released           atomic.Bool
}

type contextScope struct {
	keys               []string
	keyDigest          string
	threadKey          string
	workspaceKey       string
	workspaceAuthority string
}

// TransitionScope is the smallest host-resolved identity needed to acquire a
// context-transition writer before a TurnSecurityContext exists. It carries
// no case, snapshot, publication, or evidence authority and therefore cannot
// authorize an effect.
type TransitionScope struct {
	ThreadID          string
	WorkspaceRealPath string
	TenantID          string
	UserID            string
}

type effectLeaseContextKey struct{}

func New() *Gate {
	return &Gate{scopes: map[string]*scopeLock{}}
}

func (gate *Gate) AcquireEffect(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	return gate.acquireEffect(ctx, securityContext, false)
}

// AcquireOrdinaryEffect admits the permanent ordinary Agent capability base.
// Boundary-only contexts remain unable to acquire a strict effect lease, and
// an ordinary lease cannot be upgraded through nested re-entry.
func (gate *Gate) AcquireOrdinaryEffect(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	return gate.acquireEffect(ctx, securityContext, true)
}

func (gate *Gate) acquireEffect(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	ordinaryOnly bool,
) (context.Context, func(), error) {
	validate := domainsecurity.ValidateTurnSecurityContextForExecution
	if ordinaryOnly {
		validate = domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect
	}
	if validate(securityContext) != nil {
		return ctx, nil, errors.New("effect gate execution authority is invalid")
	}
	scope, err := gateContextScope(securityContext)
	if err != nil || gate == nil {
		return ctx, nil, errors.New("effect gate context authority is invalid")
	}
	if ctx == nil {
		return nil, nil, errors.New("effect gate context is required")
	}
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	if existing, ok := ctx.Value(effectLeaseContextKey{}).(*effectLease); ok && existing != nil &&
		existing.gate == gate && !existing.released.Load() {
		if !ordinaryOnly && existing.ordinaryOnly {
			return ctx, nil, errors.New("ordinary effect lease cannot upgrade to strict authority")
		}
		if existing.contextDigest == securityContext.ContextDigest && existing.keyDigest == scope.keyDigest {
			if !existing.retain() {
				return ctx, nil, errors.New("nested effect lease is no longer active")
			}
			var once sync.Once
			return ctx, func() { once.Do(existing.releaseReference) }, nil
		}
		if existing.threadKey == scope.threadKey || existing.workspaceKey != scope.workspaceKey || existing.workspaceAuthority != scope.workspaceAuthority {
			return ctx, nil, errors.New("nested effect crossed its frozen workspace authority")
		}
		if !existing.retain() {
			return ctx, nil, errors.New("parent effect lease is no longer active")
		}
		nestedCtx, release, acquireErr := gate.acquireEffectForScope(
			ctx,
			securityContext.ContextDigest,
			scope,
			[]string{scope.threadKey},
			existing.releaseReference,
			existing.ordinaryOnly || ordinaryOnly,
		)
		if acquireErr != nil {
			existing.releaseReference()
			return ctx, nil, acquireErr
		}
		return nestedCtx, release, nil
	}
	return gate.acquireEffectForScope(ctx, securityContext.ContextDigest, scope, scope.keys, nil, ordinaryOnly)
}

// AcquireTransitionScopeRead serializes host-only metadata reads with context
// transitions without granting an effect capability. It deliberately returns
// no lease-bearing context, so callers cannot use it to authorize provider,
// tool, source, attachment, or publication work.
func (gate *Gate) AcquireTransitionScopeRead(ctx context.Context, transition TransitionScope) (func(), error) {
	scope, err := gateTransitionScope(transition)
	if err != nil || gate == nil {
		return nil, errors.New("effect gate metadata scope is invalid")
	}
	if ctx == nil {
		return nil, errors.New("effect gate metadata context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	locks := gate.scopeLocks(scope.keys)
	acquired := 0
	for index, lock := range locks {
		if err := lock.acquireRead(ctx); err != nil {
			for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].releaseRead()
			}
			gate.releaseScopeLocks(scope.keys, locks)
			return nil, err
		}
		acquired = index + 1
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].releaseRead()
			}
			gate.releaseScopeLocks(scope.keys, locks)
		})
	}, nil
}

func (gate *Gate) acquireEffectForScope(
	ctx context.Context,
	contextDigest string,
	scope contextScope,
	keys []string,
	afterUnlock func(),
	ordinaryOnly bool,
) (context.Context, func(), error) {
	locks := gate.scopeLocks(keys)
	acquired := 0
	for index, lock := range locks {
		if err := lock.acquireRead(ctx); err != nil {
			for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].releaseRead()
			}
			gate.releaseScopeLocks(keys, locks)
			return ctx, nil, err
		}
		acquired = index + 1
	}
	lease := &effectLease{
		gate: gate, contextDigest: contextDigest, keyDigest: scope.keyDigest, threadKey: scope.threadKey,
		workspaceKey: scope.workspaceKey, workspaceAuthority: scope.workspaceAuthority,
		ordinaryOnly: ordinaryOnly, references: 1,
	}
	lease.unlock = func() {
		for index := len(locks) - 1; index >= 0; index-- {
			locks[index].releaseRead()
		}
		gate.releaseScopeLocks(keys, locks)
		if afterUnlock != nil {
			afterUnlock()
		}
	}
	var once sync.Once
	release := func() { once.Do(lease.releaseReference) }
	return context.WithValue(ctx, effectLeaseContextKey{}, lease), release, nil
}

func (lease *effectLease) retain() bool {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.references <= 0 || lease.released.Load() {
		return false
	}
	lease.references++
	return true
}

func (lease *effectLease) releaseReference() {
	lease.mu.Lock()
	if lease.references <= 0 {
		lease.mu.Unlock()
		panic("effect gate lease reference underflow")
	}
	lease.references--
	if lease.references > 0 {
		lease.mu.Unlock()
		return
	}
	lease.released.Store(true)
	unlock := lease.unlock
	lease.unlock = nil
	lease.mu.Unlock()
	if unlock != nil {
		unlock()
	}
}

func (gate *Gate) AcquireTransition(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (func(), error) {
	scope, err := gateContextScope(securityContext)
	if err != nil || gate == nil {
		return nil, errors.New("effect gate transition authority is invalid")
	}
	if ctx == nil {
		return nil, errors.New("effect gate transition context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keys := scope.keys
	var parentLease *effectLease
	if existing, ok := ctx.Value(effectLeaseContextKey{}).(*effectLease); ok && existing != nil && existing.gate == gate && !existing.released.Load() {
		if existing.threadKey == scope.threadKey || existing.workspaceKey != scope.workspaceKey || existing.workspaceAuthority != scope.workspaceAuthority {
			return nil, errors.New("effect lease cannot cross or upgrade its authority transition")
		}
		if !existing.retain() {
			return nil, errors.New("parent effect lease is no longer active")
		}
		parentLease = existing
		keys = []string{scope.threadKey}
	}
	locks := gate.scopeLocks(keys)
	acquired := 0
	for index, lock := range locks {
		if err := lock.acquireWrite(ctx); err != nil {
			for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].releaseWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			if parentLease != nil {
				parentLease.releaseReference()
			}
			return nil, err
		}
		acquired = index + 1
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].releaseWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			if parentLease != nil {
				parentLease.releaseReference()
			}
		})
	}, nil
}

// AcquireTransitionScope serializes the host reads that mint a new immutable
// security context. It exists specifically to avoid fabricating a provisional
// TSC (and accidentally mutating risk authority) merely to acquire locks.
// A provider effect lease cannot be upgraded through this pre-context API.
func (gate *Gate) AcquireTransitionScope(ctx context.Context, transition TransitionScope) (func(), error) {
	return gate.acquireTransitionScope(ctx, transition, nil)
}

// AcquireTransitionScopeWithBarrier reserves the thread/workspace writers
// before invoking barrier. New effects are excluded immediately, while
// already-admitted effects can receive cancellation and finish before the
// reserved writer is acquired. The barrier must not acquire a new effect.
func (gate *Gate) AcquireTransitionScopeWithBarrier(
	ctx context.Context,
	transition TransitionScope,
	barrier func() error,
) (func(), error) {
	if barrier == nil {
		return nil, errors.New("effect gate transition barrier is unavailable")
	}
	return gate.acquireTransitionScope(ctx, transition, barrier)
}

func (gate *Gate) acquireTransitionScope(ctx context.Context, transition TransitionScope, barrier func() error) (func(), error) {
	scope, err := gateTransitionScope(transition)
	if err != nil || gate == nil {
		return nil, errors.New("effect gate pre-context transition scope is invalid")
	}
	if ctx == nil {
		return nil, errors.New("effect gate transition context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if existing, ok := ctx.Value(effectLeaseContextKey{}).(*effectLease); ok && existing != nil && existing.gate == gate && !existing.released.Load() {
		return nil, errors.New("effect lease cannot upgrade to a pre-context authority transition")
	}
	locks := gate.scopeLocks(scope.keys)
	if barrier != nil {
		reserved := 0
		for index, lock := range locks {
			if err := lock.reserveWrite(ctx); err != nil {
				for releaseIndex := reserved - 1; releaseIndex >= 0; releaseIndex-- {
					locks[releaseIndex].cancelReservedWrite()
				}
				gate.releaseScopeLocks(scope.keys, locks)
				return nil, err
			}
			reserved = index + 1
		}
		if err := barrier(); err != nil {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].cancelReservedWrite()
			}
			gate.releaseScopeLocks(scope.keys, locks)
			return nil, err
		}
		acquired := 0
		for index, lock := range locks {
			if err := lock.acquireReservedWrite(ctx); err != nil {
				for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
					locks[releaseIndex].releaseWrite()
				}
				for cancelIndex := index + 1; cancelIndex < len(locks); cancelIndex++ {
					locks[cancelIndex].cancelReservedWrite()
				}
				gate.releaseScopeLocks(scope.keys, locks)
				return nil, err
			}
			acquired = index + 1
		}
		return transitionScopeRelease(gate, scope.keys, locks), nil
	}
	acquired := 0
	for index, lock := range locks {
		if err := lock.acquireWrite(ctx); err != nil {
			for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].releaseWrite()
			}
			gate.releaseScopeLocks(scope.keys, locks)
			return nil, err
		}
		acquired = index + 1
	}
	return transitionScopeRelease(gate, scope.keys, locks), nil
}

func transitionScopeRelease(gate *Gate, keys []string, locks []*scopeLock) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].releaseWrite()
			}
			gate.releaseScopeLocks(keys, locks)
		})
	}
}

// AcquireDelegatedChildTransitionScope is the only pre-context transition allowed to
// borrow an active effect lease. It exists for a host-created child thread in
// the exact tenant/user/workspace already frozen by its parent effect. The
// parent workspace read lease remains retained while only the distinct child
// thread writer is acquired. Same-thread upgrades and every cross-workspace
// or cross-principal transition remain fail-closed.
func (gate *Gate) AcquireDelegatedChildTransitionScope(ctx context.Context, transition TransitionScope, parentContextDigest string) (func(), error) {
	scope, err := gateTransitionScope(transition)
	parentContextDigest = strings.TrimSpace(parentContextDigest)
	if err != nil || gate == nil || !domainsecurity.IsSHA256Hex(parentContextDigest) {
		return nil, errors.New("effect gate child pre-context transition scope is invalid")
	}
	if ctx == nil {
		return nil, errors.New("effect gate child transition context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	existing, ok := ctx.Value(effectLeaseContextKey{}).(*effectLease)
	if !ok || existing == nil || existing.gate != gate || existing.released.Load() {
		return nil, errors.New("effect gate child transition requires an active parent effect lease")
	}
	if existing.contextDigest != parentContextDigest || existing.threadKey == scope.threadKey || existing.workspaceKey != scope.workspaceKey {
		return nil, errors.New("effect gate child transition crossed its frozen workspace authority")
	}
	if !existing.retain() {
		return nil, errors.New("parent effect lease is no longer active")
	}
	// Child security-context minting is serialized per workspace without
	// attempting to upgrade the parent workspace read lease. This keeps the
	// RuntimeState workspace high-water transition single-writer while allowing
	// child provider execution to proceed in parallel after the short commit.
	keys := []string{"child-transition\x00" + scope.workspaceKey, scope.threadKey}
	locks := gate.scopeLocks(keys)
	acquired := 0
	for index, lock := range locks {
		if err := lock.acquireWrite(ctx); err != nil {
			for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].releaseWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			existing.releaseReference()
			return nil, err
		}
		acquired = index + 1
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].releaseWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			existing.releaseReference()
		})
	}, nil
}

// AcquireWorkspaceRebindTransition acquires one writer over the union of the
// old and new workspace authorities plus the shared thread. Workspace locks
// are globally ordered before thread locks so opposite A->B and B->A rebinds
// cannot deadlock. Rebind is a host route operation and cannot upgrade a
// provider effect lease.
func (gate *Gate) AcquireWorkspaceRebindTransition(ctx context.Context, previous, target domainsecurity.TurnSecurityContext) (func(), error) {
	if gate == nil || domainsecurity.ValidateTurnSecurityContext(previous) != nil || domainsecurity.ValidateTurnSecurityContext(target) != nil ||
		previous.Version != domainsecurity.TurnSecurityContextVersionV2 || target.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		previous.ThreadID != target.ThreadID ||
		previous.TenantID != target.TenantID || previous.UserID != target.UserID {
		return nil, errors.New("effect gate workspace rebind authority is invalid")
	}
	return gate.AcquireWorkspaceRebindScopeTransition(ctx, TransitionScope{
		ThreadID: previous.ThreadID, WorkspaceRealPath: previous.WorkspaceRealPath,
		TenantID: previous.TenantID, UserID: previous.UserID,
	}, TransitionScope{
		ThreadID: target.ThreadID, WorkspaceRealPath: target.WorkspaceRealPath,
		TenantID: target.TenantID, UserID: target.UserID,
	})
}

// AcquireWorkspaceRebindScopeTransition acquires the old/new workspace and
// shared-thread writers before any case, risk, snapshot, or source read.
// TransitionScope is concurrency identity only and grants no effect authority.
func (gate *Gate) AcquireWorkspaceRebindScopeTransition(ctx context.Context, previous, target TransitionScope) (func(), error) {
	return gate.acquireWorkspaceRebindScopeTransition(ctx, previous, target, nil)
}

// AcquireWorkspaceRebindScopeTransitionWithBarrier first reserves every old,
// new, and shared-thread writer gate. Once all new readers are excluded it
// runs barrier, which may cancel and wait for already active foreground work,
// then acquires the reserved writers. The barrier itself grants no authority.
func (gate *Gate) AcquireWorkspaceRebindScopeTransitionWithBarrier(
	ctx context.Context,
	previous, target TransitionScope,
	barrier func() error,
) (func(), error) {
	if barrier == nil {
		return nil, errors.New("effect gate workspace rebind barrier is unavailable")
	}
	return gate.acquireWorkspaceRebindScopeTransition(ctx, previous, target, barrier)
}

func (gate *Gate) acquireWorkspaceRebindScopeTransition(
	ctx context.Context,
	previous, target TransitionScope,
	barrier func() error,
) (func(), error) {
	previousScope, previousErr := gateTransitionScope(previous)
	targetScope, targetErr := gateTransitionScope(target)
	if gate == nil || previousErr != nil || targetErr != nil || previous.ThreadID != target.ThreadID ||
		previous.TenantID != target.TenantID || previous.UserID != target.UserID ||
		previous.WorkspaceRealPath == target.WorkspaceRealPath {
		return nil, errors.New("effect gate workspace rebind scope is invalid")
	}
	if ctx == nil {
		return nil, errors.New("effect gate workspace rebind context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if existing, ok := ctx.Value(effectLeaseContextKey{}).(*effectLease); ok && existing != nil && existing.gate == gate && !existing.released.Load() {
		return nil, errors.New("effect lease cannot upgrade to a workspace rebind transition")
	}
	workspaceKeys := uniqueSortedKeys([]string{previousScope.workspaceKey, targetScope.workspaceKey})
	threadKeys := uniqueSortedKeys([]string{previousScope.threadKey, targetScope.threadKey})
	keys := append(workspaceKeys, threadKeys...)
	locks := gate.scopeLocks(keys)
	reserved := 0
	for index, lock := range locks {
		if err := lock.reserveWrite(ctx); err != nil {
			for releaseIndex := reserved - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].cancelReservedWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			return nil, err
		}
		reserved = index + 1
	}
	if barrier != nil {
		if err := barrier(); err != nil {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].cancelReservedWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			return nil, err
		}
	}
	acquired := 0
	for index, lock := range locks {
		if err := lock.acquireReservedWrite(ctx); err != nil {
			for releaseIndex := acquired - 1; releaseIndex >= 0; releaseIndex-- {
				locks[releaseIndex].releaseWrite()
			}
			for cancelIndex := index + 1; cancelIndex < len(locks); cancelIndex++ {
				locks[cancelIndex].cancelReservedWrite()
			}
			gate.releaseScopeLocks(keys, locks)
			return nil, err
		}
		acquired = index + 1
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for index := len(locks) - 1; index >= 0; index-- {
				locks[index].releaseWrite()
			}
			gate.releaseScopeLocks(keys, locks)
		})
	}, nil
}

func uniqueSortedKeys(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (gate *Gate) scopeLocks(keys []string) []*scopeLock {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.scopes == nil {
		gate.scopes = map[string]*scopeLock{}
	}
	locks := make([]*scopeLock, 0, len(keys))
	for _, key := range keys {
		lock := gate.scopes[key]
		if lock == nil {
			lock = &scopeLock{changed: make(chan struct{})}
			gate.scopes[key] = lock
		}
		lock.refs++
		locks = append(locks, lock)
	}
	return locks
}

func (gate *Gate) releaseScopeLocks(keys []string, locks []*scopeLock) {
	if gate == nil || len(keys) != len(locks) {
		panic("effect gate scope reference mismatch")
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	for index, key := range keys {
		lock := locks[index]
		if lock == nil || lock.refs <= 0 || gate.scopes[key] != lock {
			panic("effect gate scope reference underflow")
		}
		lock.refs--
		if lock.refs == 0 {
			delete(gate.scopes, key)
		}
	}
}

func (lock *scopeLock) acquireRead(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		lock.mu.Lock()
		if err := ctx.Err(); err != nil {
			lock.mu.Unlock()
			return err
		}
		if !lock.writer && lock.waitingWriters == 0 {
			lock.readers++
			lock.mu.Unlock()
			return nil
		}
		changed := lock.changed
		lock.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (lock *scopeLock) acquireWrite(ctx context.Context) error {
	if err := lock.reserveWrite(ctx); err != nil {
		return err
	}
	return lock.acquireReservedWrite(ctx)
}

func (lock *scopeLock) reserveWrite(ctx context.Context) error {
	lock.mu.Lock()
	if err := ctx.Err(); err != nil {
		lock.mu.Unlock()
		return err
	}
	lock.waitingWriters++
	lock.broadcastLocked()
	lock.mu.Unlock()
	return nil
}

func (lock *scopeLock) acquireReservedWrite(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			lock.mu.Lock()
			lock.waitingWriters--
			lock.broadcastLocked()
			lock.mu.Unlock()
			return err
		}
		lock.mu.Lock()
		if err := ctx.Err(); err != nil {
			lock.waitingWriters--
			lock.broadcastLocked()
			lock.mu.Unlock()
			return err
		}
		if !lock.writer && lock.readers == 0 {
			lock.waitingWriters--
			lock.writer = true
			lock.broadcastLocked()
			lock.mu.Unlock()
			return nil
		}
		changed := lock.changed
		lock.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
	}
}

func (lock *scopeLock) cancelReservedWrite() {
	lock.mu.Lock()
	if lock.waitingWriters <= 0 {
		lock.mu.Unlock()
		panic("effect gate writer reservation underflow")
	}
	lock.waitingWriters--
	lock.broadcastLocked()
	lock.mu.Unlock()
}

func (lock *scopeLock) releaseRead() {
	lock.mu.Lock()
	if lock.readers <= 0 {
		lock.mu.Unlock()
		panic("effect gate read lease underflow")
	}
	lock.readers--
	lock.broadcastLocked()
	lock.mu.Unlock()
}

func (lock *scopeLock) releaseWrite() {
	lock.mu.Lock()
	if !lock.writer {
		lock.mu.Unlock()
		panic("effect gate write lease underflow")
	}
	lock.writer = false
	lock.broadcastLocked()
	lock.mu.Unlock()
}

func (lock *scopeLock) broadcastLocked() {
	close(lock.changed)
	lock.changed = make(chan struct{})
}

func gateKeys(securityContext domainsecurity.TurnSecurityContext) ([]string, string, error) {
	scope, err := gateContextScope(securityContext)
	return scope.keys, scope.keyDigest, err
}

func gateContextScope(securityContext domainsecurity.TurnSecurityContext) (contextScope, error) {
	if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil ||
		securityContext.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return contextScope{}, errors.New("effect gate security context is invalid")
	}
	threadKey := "thread\x00" + securityContext.ThreadID
	workspaceKey := "workspace\x00" + securityContext.TenantID + "\x00" + securityContext.UserID + "\x00" + securityContext.WorkspaceRealPath
	// Every full acquisition takes the workspace before the thread. Nested
	// child operations already hold the parent workspace read lease and only
	// acquire the child thread. Reversing this order lets an independent child
	// transition hold the child thread while waiting for the parent workspace,
	// deadlocking the nested child operation that must finish before the parent
	// workspace lease can be released.
	keys := []string{workspaceKey, threadKey}
	workspaceAuthority := strings.Join([]string{
		securityContext.TenantID, securityContext.UserID, securityContext.WorkspaceRealPath, securityContext.CaseID,
		securityContext.CaseBindingHash, securityContext.DatasetSnapshotID, securityContext.SourceManifestHash,
	}, "\x00")
	return contextScope{
		keys: keys, keyDigest: strings.Join(keys, "\x01"), threadKey: threadKey,
		workspaceKey: workspaceKey, workspaceAuthority: workspaceAuthority,
	}, nil
}

func gateTransitionScope(transition TransitionScope) (contextScope, error) {
	if transition.ThreadID == "" || transition.ThreadID != strings.TrimSpace(transition.ThreadID) ||
		transition.WorkspaceRealPath == "" || transition.WorkspaceRealPath != strings.TrimSpace(transition.WorkspaceRealPath) ||
		transition.TenantID == "" || transition.TenantID != strings.TrimSpace(transition.TenantID) ||
		transition.UserID == "" || transition.UserID != strings.TrimSpace(transition.UserID) {
		return contextScope{}, errors.New("effect gate transition identity is incomplete")
	}
	threadKey := "thread\x00" + transition.ThreadID
	workspaceKey := "workspace\x00" + transition.TenantID + "\x00" + transition.UserID + "\x00" + transition.WorkspaceRealPath
	return contextScope{
		keys: []string{workspaceKey, threadKey}, keyDigest: strings.Join([]string{workspaceKey, threadKey}, "\x01"),
		threadKey: threadKey, workspaceKey: workspaceKey,
	}, nil
}
