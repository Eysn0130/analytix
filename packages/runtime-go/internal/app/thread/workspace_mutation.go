package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type WorkspaceRebindScopes struct {
	Previous domainsecurity.TurnSecurityContext
	Target   domainsecurity.TurnSecurityContext
	At       time.Time
}

type PreparedWorkspaceMutation struct {
	ThreadID        string
	Workspace       string
	SecurityContext domainsecurity.TurnSecurityContext
	EpochState      domaincontextepoch.State
	TransitionTurn  map[string]any
	UpdatedAt       string
}

type WorkspaceMutationCommitRequest struct {
	ExpectedBaselineDigest string
	Prepared               PreparedWorkspaceMutation
	Patch                  map[string]any
}

type BeginScopeTransitionFunc func(context.Context, string, string, string, string) (CompactionTransition, error)
type BeginWorkspaceScopeTransitionFunc func(context.Context, string, string, string, string, string, func() error) (CompactionTransition, error)

func AdaptBeginScopeTransition[T CompactionTransition](begin func(context.Context, string, string, string, string) (T, error)) BeginScopeTransitionFunc {
	return func(ctx context.Context, threadID, workspace, tenantID, userID string) (CompactionTransition, error) {
		return begin(ctx, threadID, workspace, tenantID, userID)
	}
}

func AdaptBeginWorkspaceScopeTransition[T CompactionTransition](begin func(context.Context, string, string, string, string, string, func() error) (T, error)) BeginWorkspaceScopeTransitionFunc {
	return func(ctx context.Context, threadID, previous, target, tenantID, userID string, barrier func() error) (CompactionTransition, error) {
		return begin(ctx, threadID, previous, target, tenantID, userID, barrier)
	}
}

func FreezeThreadMutationScope(thread map[string]any, threadID string, reader turnsecurityapp.WorkspaceReader, principal domainidentity.PrincipalV1, at time.Time, authorities ...turnsecurityapp.WorkspaceSecurityAuthority) (domainsecurity.TurnSecurityContext, error) {
	threadID = strings.TrimSpace(threadID)
	authority, authorityErr := threadMutationSecurityAuthority(authorities)
	if thread == nil || reader == nil || authorityErr != nil || threadID == "" || strings.TrimSpace(stringField(thread, "id")) != threadID {
		return domainsecurity.TurnSecurityContext{}, ErrThreadMutationTransition
	}
	if err := turnsecurityapp.ValidateResolvedPrincipal(context.Background(), authority.Identity, principal); err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	workspace := strings.TrimSpace(stringField(thread, "workspace"))
	observation, err := authority.Observer.Observe(workspace)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil {
		return domainsecurity.TurnSecurityContext{}, errors.Join(errors.New("thread mutation case binding observation is invalid"), err)
	}
	current, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if found {
		if current.Version != domainsecurity.TurnSecurityContextVersionV2 || current.ThreadID != threadID ||
			current.WorkspaceRealPath != observation.WorkspaceRealPath ||
			turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{
				OperationContext: context.Background(), Identity: authority.Identity, Observer: authority.Observer,
				RiskAuthority: authority.RiskAuthority, SnapshotAuthority: authority.SnapshotAuthority,
				SnapshotAuthorityV2: authority.SnapshotAuthorityV2, Context: current, Workspace: workspace,
			}) != nil {
			return domainsecurity.TurnSecurityContext{}, errors.New("thread mutation current security authority is inconsistent")
		}
		return current, nil
	}
	if thread["contextEpochState"] != nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("thread mutation epoch exists without current security authority")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: authority, Reader: reader, Thread: thread, ThreadID: threadID,
		TurnID:    workspaceMutationTurnID(threadID, observation.WorkspaceRealPath, at),
		Workspace: workspace, Principal: principal, IssuedAt: at,
	})
}

func FreezeWorkspaceRebindScopes(thread map[string]any, threadID, targetWorkspace string, reader turnsecurityapp.WorkspaceReader, principal domainidentity.PrincipalV1, at time.Time, authorities ...turnsecurityapp.WorkspaceSecurityAuthority) (WorkspaceRebindScopes, error) {
	threadID = strings.TrimSpace(threadID)
	targetWorkspace = strings.TrimSpace(targetWorkspace)
	authority, authorityErr := threadMutationSecurityAuthority(authorities)
	if thread == nil || reader == nil || authorityErr != nil || threadID == "" || targetWorkspace == "" || strings.TrimSpace(stringField(thread, "id")) != threadID {
		return WorkspaceRebindScopes{}, errors.New("workspace rebind scope authority is unavailable")
	}
	if err := turnsecurityapp.ValidateResolvedPrincipal(context.Background(), authority.Identity, principal); err != nil {
		return WorkspaceRebindScopes{}, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	at = at.UTC()
	currentWorkspace := strings.TrimSpace(stringField(thread, "workspace"))
	currentObservation, err := authority.Observer.Observe(currentWorkspace)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(currentObservation) != nil {
		return WorkspaceRebindScopes{}, errors.Join(errors.New("workspace rebind case binding observation is invalid"), err)
	}
	previous, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil {
		return WorkspaceRebindScopes{}, err
	}
	if found {
		if previous.Version != domainsecurity.TurnSecurityContextVersionV2 || previous.ThreadID != threadID ||
			previous.WorkspaceRealPath != currentObservation.WorkspaceRealPath ||
			turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{
				OperationContext: context.Background(), Identity: authority.Identity, Observer: authority.Observer,
				RiskAuthority: authority.RiskAuthority, SnapshotAuthority: authority.SnapshotAuthority,
				SnapshotAuthorityV2: authority.SnapshotAuthorityV2, Context: previous, Workspace: currentWorkspace,
			}) != nil {
			return WorkspaceRebindScopes{}, errors.New("workspace rebind current security authority is inconsistent")
		}
	} else {
		if thread["contextEpochState"] != nil {
			return WorkspaceRebindScopes{}, errors.New("workspace rebind epoch exists without current security authority")
		}
		previous, err = turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
			Context: context.Background(), Authority: authority, Reader: reader, Thread: thread, ThreadID: threadID,
			TurnID:    workspaceMutationTurnID(threadID, currentObservation.WorkspaceRealPath, at.Add(-time.Nanosecond)),
			Workspace: currentWorkspace, Principal: principal, IssuedAt: at.Add(-time.Nanosecond),
		})
		if err != nil {
			return WorkspaceRebindScopes{}, err
		}
	}
	target, err := turnsecurityapp.FreezeWorkspaceRebindTarget(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: authority, Reader: reader, Thread: thread, ThreadID: threadID,
		TurnID:    workspaceMutationTurnID(threadID, targetWorkspace, at),
		Workspace: targetWorkspace, Principal: principal, IssuedAt: at,
	}, previous)
	if err != nil {
		return WorkspaceRebindScopes{}, err
	}
	if target.WorkspaceRealPath == previous.WorkspaceRealPath {
		return WorkspaceRebindScopes{}, errors.New("workspace rebind target matches current workspace")
	}
	if !domainsecurity.TurnSecurityContextIsGeneral(previous) || !domainsecurity.TurnSecurityContextIsGeneral(target) {
		return WorkspaceRebindScopes{}, ErrCaseWorkspaceSignedRebind
	}
	return WorkspaceRebindScopes{Previous: previous, Target: target, At: at}, nil
}

func MutationInvalidationTarget(current domainsecurity.TurnSecurityContext, cause string, at time.Time) domainsecurity.TurnSecurityContext {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	target, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: workspaceMutationTurnID(current.ThreadID, cause, at),
		WorkspaceRealPath: current.WorkspaceRealPath, TenantID: current.TenantID, UserID: current.UserID,
		CaseID: current.CaseID, CaseBindingHash: current.CaseBindingHash, DatasetSnapshotID: current.DatasetSnapshotID,
		SourceManifestHash: current.SourceManifestHash, ContextEpoch: current.ContextEpoch, IssuedAt: at,
		PublicationPolicy: current.PublicationPolicy, RiskAuthorityBinding: current.RiskAuthorityBinding,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}
	}
	return target
}

func PrepareWorkspaceMutation(thread map[string]any, scopes WorkspaceRebindScopes) (PreparedWorkspaceMutation, error) {
	if !domainsecurity.TurnSecurityContextIsGeneral(scopes.Previous) || !domainsecurity.TurnSecurityContextIsGeneral(scopes.Target) {
		return PreparedWorkspaceMutation{}, ErrCaseWorkspaceSignedRebind
	}
	prepared, err := contextepochapp.PrepareWorkspaceRebind(thread, scopes.Previous, scopes.Target, scopes.At)
	if err != nil {
		return PreparedWorkspaceMutation{}, err
	}
	contextRecord := turnsecurityapp.PublicRecord(prepared.SecurityContext)
	epochSnapshot := contextepochapp.PublicSnapshot(prepared.State.AcceptedSnapshot)
	now := scopes.At.UTC().Format(time.RFC3339Nano)
	transitionTurn := map[string]any{
		"id": prepared.SecurityContext.TurnID, "threadId": prepared.SecurityContext.ThreadID, "kind": "workspace_transition",
		"status": "completed", "createdAt": now, "completedAt": now, "securityContext": contextRecord,
		"contextEpochSnapshot": epochSnapshot, "items": []any{},
	}
	return PreparedWorkspaceMutation{
		ThreadID: prepared.SecurityContext.ThreadID, Workspace: prepared.SecurityContext.WorkspaceRealPath,
		SecurityContext: prepared.SecurityContext, EpochState: prepared.State, TransitionTurn: transitionTurn, UpdatedAt: now,
	}, nil
}

func ApplyWorkspaceMutationCommit(thread map[string]any, request WorkspaceMutationCommitRequest) (map[string]any, error) {
	prepared := request.Prepared
	if err := ValidateMutationBaseline(thread, prepared.ThreadID, request.ExpectedBaselineDigest); err != nil {
		return nil, err
	}
	if domainsecurity.ValidateTurnSecurityContext(prepared.SecurityContext) != nil || !domainsecurity.TurnSecurityContextIsGeneral(prepared.SecurityContext) ||
		prepared.SecurityContext.ThreadID != prepared.ThreadID || prepared.SecurityContext.WorkspaceRealPath != prepared.Workspace ||
		domaincontextepoch.ValidateState(prepared.EpochState) != nil || prepared.EpochState.ThreadID != prepared.ThreadID ||
		prepared.EpochState.AcceptedSnapshot.Epoch != prepared.SecurityContext.ContextEpoch {
		return nil, errors.New("workspace mutation commit authority is invalid")
	}
	turnID := strings.TrimSpace(stringField(prepared.TransitionTurn, "id"))
	if turnID == "" || turnID != prepared.SecurityContext.TurnID {
		return nil, errors.New("workspace mutation transition turn is invalid")
	}
	next := ApplyPatch(thread, request.Patch, prepared.UpdatedAt)
	turns, _ := next["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID {
			return nil, errors.New("workspace mutation transition turn already exists")
		}
	}
	next["workspace"] = prepared.Workspace
	next["securityState"] = turnsecurityapp.PublicRecord(prepared.SecurityContext)
	next["contextEpochState"] = contextepochapp.PublicState(prepared.EpochState)
	next["turns"] = append(turns, contracts.CloneMap(prepared.TransitionTurn))
	next["status"] = "idle"
	next["updatedAt"] = prepared.UpdatedAt
	return next, nil
}

func threadMutationSecurityAuthority(authorities []turnsecurityapp.WorkspaceSecurityAuthority) (turnsecurityapp.WorkspaceSecurityAuthority, error) {
	if len(authorities) != 1 || authorities[0].Identity == nil || authorities[0].Observer == nil || authorities[0].RiskAuthority == nil {
		return turnsecurityapp.WorkspaceSecurityAuthority{}, errors.New("thread mutation V2 identity, observer, and risk policy authority are required")
	}
	return authorities[0], nil
}

func mutationObservationMatchesContext(observation domainsecurity.CaseBindingObservationV1, securityContext domainsecurity.TurnSecurityContext) bool {
	if observation.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
		return false
	}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		return observation.State == domainsecurity.CaseBindingStateValid && observation.CaseID == securityContext.CaseID &&
			observation.CaseBindingHash == securityContext.CaseBindingHash
	}
	return true
}

func workspaceMutationTurnID(threadID, workspace string, at time.Time) string {
	digest := domainsecurity.SHA256Hex([]byte(strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(workspace) + "\x00" + at.UTC().Format(time.RFC3339Nano)))
	return "turn_workspace_" + digest[:24]
}
