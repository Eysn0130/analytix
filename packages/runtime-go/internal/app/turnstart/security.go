package turnstart

import (
	"context"
	"errors"
	"strings"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

type SecurityRecords struct {
	Turn             map[string]any
	TurnStartedEvent map[string]any
	ThreadPatch      map[string]any
}

type SecurityPreparation struct {
	SecurityContext domainsecurity.TurnSecurityContext
	ProviderContext domaincontextepoch.ProviderContext
	EpochState      domaincontextepoch.State
	SourceProbe     domainsecurity.VerifiedSourceProbe
	SourceReady     bool
	ExecutionReady  bool
	SourceError     error
}

// ChildTransitionAuthority is loaded from the durable host-owned child-run
// registry. Provider fields and request booleans cannot create this authority.
// Foreground children additionally prove the exact parent context digest to
// the effect gate; background children have no inherited lease and take the
// ordinary full transition writer after the same durable validation.
type ChildTransitionAuthority struct {
	ChildRunID     string
	ChildThreadID  string
	Background     bool
	Binding        *domainjob.SecurityBinding
	ExpectedRecord domainjob.Record
	frozenWitness  *childTransitionFrozenWitnessStateV1
}

// BeginSecurityTransition resolves only the canonical workspace concurrency
// identity before taking the writer. It must not read the case marker, derive
// a risk policy, select a dataset, probe a source, or mint a provisional TSC.
func BeginSecurityTransition(
	ctx context.Context,
	state *subagentapp.RuntimeState,
	identityAuthority identityport.Authority,
	principal domainidentity.PrincipalV1,
	reader turnsecurityapp.WorkspaceReader,
	source any,
	thread map[string]any,
	threadID, turnID, workspace string,
	issuedAt time.Time,
	childAuthority ...*ChildTransitionAuthority,
) (*subagentapp.SecurityContextTransition, error) {
	if state == nil || reader == nil {
		return nil, errors.New("turn start security transition dependencies are unavailable")
	}
	if err := turnsecurityapp.ValidateResolvedPrincipal(ctx, identityAuthority, principal); err != nil {
		return nil, err
	}
	workspaceRealPath, err := reader.WorkspaceRealPath(workspace)
	if err != nil {
		return nil, err
	}
	_ = source
	_ = thread
	_ = turnID
	_ = issuedAt
	scope := effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: workspaceRealPath,
		TenantID: principal.TenantID, UserID: principal.UserID,
	}
	if len(childAuthority) > 0 && childAuthority[0] != nil {
		authority := childAuthority[0]
		if strings.TrimSpace(authority.ChildRunID) == "" || authority.ChildRunID != strings.TrimSpace(authority.ChildRunID) ||
			authority.ChildThreadID != threadID || domainjob.ValidateSecurityBinding(authority.Binding) != nil ||
			authority.ExpectedRecord.ID != authority.ChildRunID || authority.ExpectedRecord.ChildThreadID != authority.ChildThreadID ||
			authority.ExpectedRecord.SecurityBinding == nil || authority.ExpectedRecord.SecurityBinding.BindingDigest != authority.Binding.BindingDigest ||
			authority.Binding.ParentWorkspaceRealPath != workspaceRealPath || authority.Binding.TenantID != scope.TenantID ||
			authority.Binding.UserID != scope.UserID {
			return nil, errors.New("durable child transition authority is invalid")
		}
		if authority.Background {
			return state.BeginSecurityScopeTransition(ctx, scope)
		}
		return state.BeginDelegatedChildSecurityScopeTransition(ctx, scope, authority.Binding.ParentContextDigest)
	}
	return state.BeginSecurityScopeTransition(ctx, scope)
}

// BeginSecurityTransitionWithBarrier is the parent-thread admission path used
// when an older paused gate or execution may still own the durable thread. It
// reserves the same canonical thread/workspace writer before barrier runs, so
// no new effect can enter while the host closes old authority.
func BeginSecurityTransitionWithBarrier(
	ctx context.Context,
	state *subagentapp.RuntimeState,
	identityAuthority identityport.Authority,
	principal domainidentity.PrincipalV1,
	reader turnsecurityapp.WorkspaceReader,
	threadID, workspace string,
	barrier func() error,
	timeout time.Duration,
) (*subagentapp.SecurityContextTransition, error) {
	if state == nil || reader == nil || barrier == nil || timeout <= 0 {
		return nil, errors.New("turn start security transition barrier dependencies are unavailable")
	}
	if err := turnsecurityapp.ValidateResolvedPrincipal(ctx, identityAuthority, principal); err != nil {
		return nil, err
	}
	workspaceRealPath, err := reader.WorkspaceRealPath(workspace)
	if err != nil {
		return nil, err
	}
	settleBackgroundTails := func() error {
		return state.CancelBoundBackgroundJobsForParentThreadAndWait(
			ctx, threadID, principal.TenantID, principal.UserID, timeout,
		)
	}
	transition, err := state.BeginSecurityScopeTransitionWithBarrier(ctx, effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: workspaceRealPath,
		TenantID: principal.TenantID, UserID: principal.UserID,
	}, func() error {
		if err := barrier(); err != nil {
			return err
		}
		return settleBackgroundTails()
	})
	if err != nil {
		return nil, err
	}
	// Sweep again after transition admission is marked. A direct registration
	// in the reserved-writer window cannot then cross the durable baseline.
	if err := settleBackgroundTails(); err != nil {
		transition.Abort()
		return nil, err
	}
	return transition, nil
}

// PrepareSecurityTransition repeats every authoritative freeze and live probe
// while the transition writer is held, then cancels exact old-turn mutators and
// performs a final binding/source revalidation before durable commit.
func PrepareSecurityTransition(
	ctx context.Context,
	transition *subagentapp.SecurityContextTransition,
	principal domainidentity.PrincipalV1,
	reader turnsecurityapp.WorkspaceReader,
	authority turnsecurityapp.WorkspaceSecurityAuthority,
	source any,
	thread map[string]any,
	threadID, turnID, workspace string,
	issuedAt time.Time,
	records SecurityRecords,
	timeout time.Duration,
	frozenHooks ...func(context.Context, domainsecurity.TurnSecurityContext) error,
) (SecurityPreparation, error) {
	if transition == nil || reader == nil || authority.Observer == nil ||
		records.Turn == nil || records.TurnStartedEvent == nil || records.ThreadPatch == nil {
		return SecurityPreparation{}, errors.New("turn start security preparation dependencies are unavailable")
	}
	if err := turnsecurityapp.ValidateResolvedPrincipal(ctx, authority.Identity, principal); err != nil {
		return SecurityPreparation{}, err
	}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: authority, Reader: reader, Thread: thread, ThreadID: threadID, TurnID: turnID,
		Workspace: workspace, Principal: principal, IssuedAt: issuedAt,
	})
	if err != nil {
		return SecurityPreparation{}, err
	}
	prepared, err := contextepochapp.PrepareAndAttachStartRecords(ctx, contextepochapp.PrepareStartRecordsInput{
		Thread: thread, SecurityContext: securityContext, Source: source, At: issuedAt,
		Turn: records.Turn, TurnStartedEvent: records.TurnStartedEvent, ThreadPatch: records.ThreadPatch,
	})
	if err != nil {
		return SecurityPreparation{}, err
	}
	securityContext = prepared.SecurityContext
	for _, hook := range frozenHooks {
		if hook != nil {
			if err := hook(ctx, securityContext); err != nil {
				return SecurityPreparation{}, err
			}
		}
	}
	executionReady := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) == nil
	sourceReady := executionReady && domainsecurity.TurnSecurityContextIsGeneral(securityContext)
	var sourceProbe domainsecurity.VerifiedSourceProbe
	var sourceErr error
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		blocker := strings.TrimSpace(securityContext.PublicationPolicy.BlockerCode)
		if blocker != "" && blocker != domainsecurity.PublicationBlockerNone {
			sourceErr = errors.New(blocker)
		}
		if blocker == domainsecurity.PublicationBlockerCaseWorkspaceUnavailable {
			executionReady = false
		}
	}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		binding, bindingErr := authority.Observer.Observe(workspace)
		if bindingErr != nil {
			sourceErr = errors.New("current case binding changed before source probe")
		} else {
			sourceProbe, sourceErr = turnsecurityapp.ProbeCurrentCaseSource(ctx, source, securityContext, binding)
		}
		sourceReady = sourceErr == nil && turnsecurityapp.SourceDiscoveryMatchesContext(sourceProbe, securityContext)
		if sourceErr == nil && !sourceReady {
			sourceErr = errors.New("current case source probe does not match frozen host authority")
		}
	}
	if err := transition.Prepare(ctx, securityContext, timeout); err != nil {
		return SecurityPreparation{}, err
	}
	currentInput := turnsecurityapp.CurrentValidationInput{
		OperationContext: ctx, Identity: authority.Identity, Observer: authority.Observer, RiskAuthority: authority.RiskAuthority,
		SnapshotAuthority: authority.SnapshotAuthority, SnapshotAuthorityV2: authority.SnapshotAuthorityV2,
		Reader: reader, Context: securityContext, Workspace: workspace,
	}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		if err := turnsecurityapp.ValidateCurrent(currentInput); err != nil {
			if turnsecurityapp.CurrentFailureCode(err) != "turn_security_dataset_snapshot_mismatch" {
				return SecurityPreparation{}, err
			}
			if ordinaryErr := turnsecurityapp.ValidateCurrentOrdinaryEffect(currentInput); ordinaryErr != nil {
				return SecurityPreparation{}, ordinaryErr
			}
			sourceReady = false
			if sourceErr == nil {
				sourceErr = err
			}
		}
	} else if executionReady {
		if err := turnsecurityapp.ValidateCurrentOrdinaryEffect(currentInput); err != nil {
			return SecurityPreparation{}, err
		}
	} else if domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		if err := turnsecurityapp.ValidateCurrentHostBoundary(currentInput); err != nil {
			return SecurityPreparation{}, err
		}
	} else {
		return SecurityPreparation{}, errors.New("turn security context is not host-finalizable")
	}
	return SecurityPreparation{
		SecurityContext: securityContext, ProviderContext: prepared.ProviderContext, EpochState: prepared.State,
		SourceProbe: sourceProbe, SourceReady: sourceReady, ExecutionReady: executionReady, SourceError: sourceErr,
	}, nil
}
