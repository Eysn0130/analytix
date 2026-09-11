package checkpoint

import (
	"context"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ApplyThreadReader interface {
	GetThread(string) (map[string]any, error)
}

type ApplyCaseContextAuthority interface {
	IsCaseThread(string) bool
	ContainsContext(domainsecurity.TurnSecurityContext) bool
}

type ApplyEffectAcquirer func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) (context.Context, func(), error)

type ApplyAuthorityInput struct {
	Context           context.Context
	ThreadID          string
	CheckpointID      string
	Workspace         string
	Threads           ApplyThreadReader
	Snapshots         SnapshotAuthority
	TurnSecurity      turnsecurityapp.WorkspaceSecurityAuthority
	CaseContexts      ApplyCaseContextAuthority
	AcquireEffect     ApplyEffectAcquirer
	WorkspaceRealPath func(string) (string, error)
}

// AcquireApplyAuthority serializes a rewind with foreground effects, then
// revalidates the newest host context and the historical checkpoint context
// under that lock. The returned release function must be held through file
// mutation and its terminal audit append.
func AcquireApplyAuthority(input ApplyAuthorityInput) (func(), string) {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Threads == nil || !input.Snapshots.Available() || input.AcquireEffect == nil || input.WorkspaceRealPath == nil {
		return func() {}, "checkpoint rewind authority dependencies are unavailable"
	}
	loadCurrent := func() (domainsecurity.TurnSecurityContext, string) {
		thread, err := input.Threads.GetThread(input.ThreadID)
		if err != nil || thread == nil {
			return domainsecurity.TurnSecurityContext{}, "checkpoint rewind current thread authority is unavailable"
		}
		current, found, err := turnsecurityapp.LatestContext(thread)
		if err != nil || !found || current.ThreadID != input.ThreadID ||
			domainsecurity.ValidateTurnSecurityContextForExecution(current) != nil {
			return domainsecurity.TurnSecurityContext{}, "checkpoint rewind requires a current executable turn authority"
		}
		workspace, err := input.WorkspaceRealPath(input.Workspace)
		if err != nil || workspace != current.WorkspaceRealPath {
			return domainsecurity.TurnSecurityContext{}, "checkpoint rewind current workspace authority does not match the route"
		}
		return current, ""
	}
	current, mismatch := loadCurrent()
	if mismatch != "" {
		return func() {}, mismatch
	}
	effectContext, release, err := input.AcquireEffect(ctx, current)
	if err != nil {
		return func() {}, "checkpoint rewind effect authority is unavailable"
	}
	currentAfterLock, mismatch := loadCurrent()
	if mismatch != "" || currentAfterLock.ContextDigest != current.ContextDigest {
		release()
		if mismatch != "" {
			return func() {}, mismatch
		}
		return func() {}, "checkpoint rewind current turn authority changed while waiting"
	}
	if err := turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
		OperationContext: effectContext, Identity: input.TurnSecurity.Identity,
		Observer: input.TurnSecurity.Observer, RiskAuthority: input.TurnSecurity.RiskAuthority,
		SnapshotAuthority: input.TurnSecurity.SnapshotAuthority, SnapshotAuthorityV2: input.TurnSecurity.SnapshotAuthorityV2,
		Context: currentAfterLock, Workspace: currentAfterLock.WorkspaceRealPath,
	}); err != nil {
		release()
		return func() {}, "checkpoint rewind current case, dataset, or risk authority is stale"
	}
	frozen, err := input.Snapshots.FrozenContext(effectContext, input.ThreadID, input.CheckpointID)
	if err != nil {
		release()
		return func() {}, "checkpoint rewind frozen snapshot authority is unavailable"
	}
	if mismatch := RewindContextMismatch(frozen, currentAfterLock); mismatch != "" {
		release()
		return func() {}, mismatch
	}
	if input.CaseContexts != nil && input.CaseContexts.IsCaseThread(input.ThreadID) &&
		(!input.CaseContexts.ContainsContext(frozen) || !input.CaseContexts.ContainsContext(currentAfterLock)) {
		release()
		return func() {}, "checkpoint rewind case contexts are not members of host authority"
	}
	return release, ""
}
