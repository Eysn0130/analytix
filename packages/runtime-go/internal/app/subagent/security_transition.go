package subagent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// SecurityContextTransition is a two-phase host authority transaction. The
// effect-gate writer and admission markers are acquired before live case/source
// reads. Prepare cancels and waits for every invalidated mutator without
// publishing tentative current authority. Commit publishes only after the
// caller has durably committed the exact turn and case authority; Abort leaves
// the previous current authority intact.
type SecurityContextTransition struct {
	state         *RuntimeState
	scope         effectgateapp.TransitionScope
	releaseGate   func()
	threadKey     string
	workspaceKeys []string

	mu       sync.Mutex
	prepared bool
	finished bool
	current  domainsecurity.TurnSecurityContext
}

func (s *RuntimeState) BeginSecurityContextTransition(ctx context.Context, scope domainsecurity.TurnSecurityContext) (*SecurityContextTransition, error) {
	if s == nil || domainsecurity.ValidateTurnSecurityContext(scope) != nil ||
		scope.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return nil, errors.New("security context transition scope is invalid")
	}
	gate := s.EffectGate()
	if gate == nil {
		return nil, errors.New("security context effect gate is unavailable")
	}
	release, err := gate.AcquireTransition(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(effectgateapp.TransitionScope{
		ThreadID: scope.ThreadID, WorkspaceRealPath: scope.WorkspaceRealPath, TenantID: scope.TenantID, UserID: scope.UserID,
	}, release, scope.WorkspaceRealPath)
}

// BeginSecurityContextTransitionWithBarrier reserves the target concurrency
// scope before the host drains foreground owners and paused gates. It is used
// by thread mutations whose prepared context already exists but must not wait
// behind an uncancelled provider read lease.
func (s *RuntimeState) BeginSecurityContextTransitionWithBarrier(
	ctx context.Context,
	scope domainsecurity.TurnSecurityContext,
	barrier func() error,
) (*SecurityContextTransition, error) {
	if s == nil || domainsecurity.ValidateTurnSecurityContext(scope) != nil ||
		scope.Version != domainsecurity.TurnSecurityContextVersionV2 || barrier == nil {
		return nil, errors.New("security context transition scope is invalid")
	}
	transitionScope := effectgateapp.TransitionScope{
		ThreadID: scope.ThreadID, WorkspaceRealPath: scope.WorkspaceRealPath, TenantID: scope.TenantID, UserID: scope.UserID,
	}
	release, err := s.EffectGate().AcquireTransitionScopeWithBarrier(ctx, transitionScope, barrier)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(transitionScope, release, scope.WorkspaceRealPath)
}

// BeginSecurityScopeTransition acquires the shared thread/workspace writer
// before any case-binding, risk-witness, snapshot, source, provider, or
// attachment read. The scope is concurrency identity only and is never an
// execution or publication credential.
func (s *RuntimeState) BeginSecurityScopeTransition(ctx context.Context, scope effectgateapp.TransitionScope) (*SecurityContextTransition, error) {
	if s == nil {
		return nil, errors.New("security context transition state is unavailable")
	}
	gate := s.EffectGate()
	if gate == nil {
		return nil, errors.New("security context effect gate is unavailable")
	}
	release, err := gate.AcquireTransitionScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(scope, release, scope.WorkspaceRealPath)
}

func (s *RuntimeState) BeginSecurityScopeTransitionWithBarrier(
	ctx context.Context,
	scope effectgateapp.TransitionScope,
	barrier func() error,
) (*SecurityContextTransition, error) {
	if s == nil || barrier == nil {
		return nil, errors.New("security context transition state is unavailable")
	}
	release, err := s.EffectGate().AcquireTransitionScopeWithBarrier(ctx, scope, barrier)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(scope, release, scope.WorkspaceRealPath)
}

// BeginDelegatedChildSecurityScopeTransition is the host-only child-turn admission
// path. It retains the parent workspace effect authority while taking the new
// child thread writer, so nested child startup cannot deadlock or upgrade the
// parent thread itself.
func (s *RuntimeState) BeginDelegatedChildSecurityScopeTransition(ctx context.Context, scope effectgateapp.TransitionScope, parentContextDigest string) (*SecurityContextTransition, error) {
	if s == nil {
		return nil, errors.New("child security context transition state is unavailable")
	}
	gate := s.EffectGate()
	if gate == nil {
		return nil, errors.New("security context effect gate is unavailable")
	}
	release, err := gate.AcquireDelegatedChildTransitionScope(ctx, scope, parentContextDigest)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(scope, release, scope.WorkspaceRealPath)
}

func (s *RuntimeState) BeginSecurityScopeTransitionIdentity(
	ctx context.Context,
	threadID, workspaceRealPath, tenantID, userID string,
) (*SecurityContextTransition, error) {
	return s.BeginSecurityScopeTransition(ctx, effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: workspaceRealPath, TenantID: tenantID, UserID: userID,
	})
}

func (s *RuntimeState) BeginSecurityScopeTransitionIdentityWithBarrier(
	ctx context.Context,
	threadID, workspaceRealPath, tenantID, userID string,
	barrier func() error,
) (*SecurityContextTransition, error) {
	return s.BeginSecurityScopeTransitionWithBarrier(ctx, effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: workspaceRealPath, TenantID: tenantID, UserID: userID,
	}, barrier)
}

func (s *RuntimeState) BeginWorkspaceSecurityScopeTransition(
	ctx context.Context,
	threadID, previousWorkspaceRealPath, targetWorkspaceRealPath, tenantID, userID string,
	barrier func() error,
) (*SecurityContextTransition, error) {
	if s == nil {
		return nil, errors.New("workspace security context transition state is unavailable")
	}
	previous := effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: previousWorkspaceRealPath, TenantID: tenantID, UserID: userID,
	}
	target := effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: targetWorkspaceRealPath, TenantID: tenantID, UserID: userID,
	}
	release, err := s.EffectGate().AcquireWorkspaceRebindScopeTransitionWithBarrier(ctx, previous, target, barrier)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(target, release, previousWorkspaceRealPath, targetWorkspaceRealPath)
}

// BeginWorkspaceSecurityContextTransition holds the shared thread writer and
// both workspace writers across a host-authorized rebind transaction.
func (s *RuntimeState) BeginWorkspaceSecurityContextTransition(ctx context.Context, previous, target domainsecurity.TurnSecurityContext) (*SecurityContextTransition, error) {
	if s == nil || domainsecurity.ValidateTurnSecurityContext(previous) != nil || domainsecurity.ValidateTurnSecurityContext(target) != nil ||
		previous.Version != domainsecurity.TurnSecurityContextVersionV2 || target.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		previous.ThreadID != target.ThreadID || previous.TenantID != target.TenantID || previous.UserID != target.UserID ||
		previous.WorkspaceRealPath == target.WorkspaceRealPath {
		return nil, errors.New("workspace security context transition scope is invalid")
	}
	gate := s.EffectGate()
	if gate == nil {
		return nil, errors.New("security context effect gate is unavailable")
	}
	release, err := gate.AcquireWorkspaceRebindTransition(ctx, previous, target)
	if err != nil {
		return nil, err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		release()
		return nil, err
	}
	return s.beginSecurityContextTransition(effectgateapp.TransitionScope{
		ThreadID: target.ThreadID, WorkspaceRealPath: target.WorkspaceRealPath, TenantID: target.TenantID, UserID: target.UserID,
	}, release, previous.WorkspaceRealPath, target.WorkspaceRealPath)
}

func (s *RuntimeState) beginSecurityContextTransition(scope effectgateapp.TransitionScope, release func(), workspaces ...string) (*SecurityContextTransition, error) {
	workspaceKeys := make([]string, 0, len(workspaces))
	seen := map[string]bool{}
	for _, workspace := range workspaces {
		key := securityAuthorityMapKey("workspace", scope.TenantID, scope.UserID, workspace)
		if !seen[key] {
			workspaceKeys = append(workspaceKeys, key)
			seen[key] = true
		}
	}
	transition := &SecurityContextTransition{
		state: s, scope: scope, releaseGate: release,
		threadKey: securityAuthorityMapKey("thread", scope.TenantID, scope.UserID, scope.ThreadID), workspaceKeys: workspaceKeys,
	}
	s.mu.Lock()
	if s.transitionThreads == nil {
		s.transitionThreads = map[string]bool{}
	}
	if s.transitionWorkspaces == nil {
		s.transitionWorkspaces = map[string]bool{}
	}
	conflict := s.transitionThreads[transition.threadKey]
	for _, workspaceKey := range transition.workspaceKeys {
		conflict = conflict || s.transitionWorkspaces[workspaceKey]
	}
	if conflict {
		s.mu.Unlock()
		release()
		return nil, errors.New("security context transition is already active")
	}
	s.transitionThreads[transition.threadKey] = true
	for _, workspaceKey := range transition.workspaceKeys {
		s.transitionWorkspaces[workspaceKey] = true
	}
	s.mu.Unlock()
	return transition, nil
}

func (transition *SecurityContextTransition) Prepare(ctx context.Context, current domainsecurity.TurnSecurityContext, timeout time.Duration) error {
	if transition == nil || transition.state == nil || domainsecurity.ValidateTurnSecurityContext(current) != nil ||
		current.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		timeout <= 0 || !transition.sameScope(current) {
		return errors.New("security context transition authority is invalid")
	}
	if err := contextErrorIfDone(ctx); err != nil {
		return err
	}
	transition.mu.Lock()
	if transition.finished || transition.prepared {
		transition.mu.Unlock()
		return errors.New("security context transition is not preparable")
	}
	transition.mu.Unlock()

	state := transition.state
	state.mu.Lock()
	markersPresent := state.transitionThreads[transition.threadKey]
	for _, workspaceKey := range transition.workspaceKeys {
		markersPresent = markersPresent && state.transitionWorkspaces[workspaceKey]
	}
	if !markersPresent {
		state.mu.Unlock()
		return errors.New("security context transition admission authority was lost")
	}
	controls := make([]*backgroundJobControl, 0)
	for _, control := range state.backgroundJobs {
		if control == nil || control.cancel == nil || control.binding == nil {
			continue
		}
		binding := control.binding
		// A new turn invalidates every old exact thread/turn/context mutator,
		// even when case, snapshot, and epoch text happen to be unchanged.
		invalidThread := domainjob.SecurityBindingSharesThread(binding, current) && !domainjob.SecurityBindingMatchesContext(binding, current)
		invalidWorkspace := domainjob.SecurityBindingSharesWorkspace(binding, current) && !domainjob.SecurityBindingMatchesWorkspaceScope(binding, current)
		if invalidThread || invalidWorkspace {
			controls = append(controls, control)
		}
	}
	state.mu.Unlock()
	for _, control := range controls {
		control.requestCancel()
	}
	if err := waitForInvalidatedControls(ctx, controls, timeout); err != nil {
		return err
	}
	if err := contextErrorIfDone(ctx); err != nil {
		return err
	}
	transition.mu.Lock()
	defer transition.mu.Unlock()
	if transition.finished || transition.prepared {
		return errors.New("security context transition changed while preparing")
	}
	transition.current = current
	transition.prepared = true
	return nil
}

func (transition *SecurityContextTransition) Commit() error {
	if transition == nil || transition.state == nil {
		return errors.New("security context transition is unavailable")
	}
	transition.mu.Lock()
	if transition.finished || !transition.prepared || domainsecurity.ValidateTurnSecurityContext(transition.current) != nil ||
		transition.current.Version != domainsecurity.TurnSecurityContextVersionV2 {
		transition.mu.Unlock()
		return errors.New("security context transition is not committable")
	}
	current := transition.current
	transition.finished = true
	transition.mu.Unlock()

	state := transition.state
	state.mu.Lock()
	if state.currentByThread == nil {
		state.currentByThread = map[string]domainsecurity.TurnSecurityContext{}
	}
	if state.currentByWorkspace == nil {
		state.currentByWorkspace = map[string]domainsecurity.TurnSecurityContext{}
	}
	state.currentByThread[transition.threadKey] = current
	targetWorkspaceKey := securityAuthorityMapKey("workspace", current.TenantID, current.UserID, current.WorkspaceRealPath)
	state.currentByWorkspace[targetWorkspaceKey] = current
	delete(state.transitionThreads, transition.threadKey)
	for _, workspaceKey := range transition.workspaceKeys {
		delete(state.transitionWorkspaces, workspaceKey)
	}
	state.mu.Unlock()
	transition.release()
	return nil
}

func (transition *SecurityContextTransition) Abort() {
	if transition == nil || transition.state == nil {
		return
	}
	transition.mu.Lock()
	if transition.finished {
		transition.mu.Unlock()
		return
	}
	transition.finished = true
	transition.mu.Unlock()
	state := transition.state
	state.mu.Lock()
	delete(state.transitionThreads, transition.threadKey)
	for _, workspaceKey := range transition.workspaceKeys {
		delete(state.transitionWorkspaces, workspaceKey)
	}
	state.mu.Unlock()
	transition.release()
}

func (transition *SecurityContextTransition) sameScope(current domainsecurity.TurnSecurityContext) bool {
	return transition.scope.ThreadID == current.ThreadID && transition.scope.WorkspaceRealPath == current.WorkspaceRealPath &&
		transition.scope.TenantID == current.TenantID && transition.scope.UserID == current.UserID
}

// ScopeWorkspaceRealPath returns the immutable concurrency identity acquired
// by this transition. It is not an execution or publication credential; turn
// start uses it only to constrain host workspace preparation before the final
// authoritative freeze.
func (transition *SecurityContextTransition) ScopeWorkspaceRealPath() (string, error) {
	if transition == nil || strings.TrimSpace(transition.scope.WorkspaceRealPath) == "" {
		return "", errors.New("security context transition workspace scope is unavailable")
	}
	return transition.scope.WorkspaceRealPath, nil
}

func (transition *SecurityContextTransition) release() {
	transition.mu.Lock()
	release := transition.releaseGate
	transition.releaseGate = nil
	transition.mu.Unlock()
	if release != nil {
		release()
	}
}

func waitForInvalidatedControls(ctx context.Context, controls []*backgroundJobControl, timeout time.Duration) error {
	if len(controls) == 0 {
		return contextErrorIfDone(ctx)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for _, control := range controls {
		select {
		case <-control.done:
		case <-timer.C:
			return errors.New("old context background jobs did not stop before context acceptance")
		case <-contextDone(ctx):
			return contextError(ctx)
		}
	}
	return contextErrorIfDone(ctx)
}

func contextErrorIfDone(ctx context.Context) error {
	if ctx == nil {
		return errors.New("security context transition context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
