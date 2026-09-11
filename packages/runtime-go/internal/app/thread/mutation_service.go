package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const metadataPatchCASAttempts = 8

func (s *Service) patchUnderWriter(ctx context.Context, threadID string, patch map[string]any, initial map[string]any) (map[string]any, error) {
	if s.beginMetadataRead == nil || s.workspaceReader == nil || initial == nil {
		return nil, ErrThreadMutationTransition
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, s.workspaceSecurity.Identity)
	if err != nil {
		return nil, err
	}
	workspaceRealPath, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(initial, "workspace")))
	if err != nil {
		return nil, err
	}
	release, err := s.beginMetadataRead(
		ctx, strings.TrimSpace(threadID), workspaceRealPath, principal.TenantID, principal.UserID,
	)
	if err != nil || release == nil {
		return nil, errors.Join(ErrThreadMutationTransition, err)
	}
	defer release()
	for attempt := 0; attempt < metadataPatchCASAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		thread, digest, err := s.repository.ReadThreadMutationBaseline(threadID)
		if err != nil {
			return nil, err
		}
		if thread == nil {
			return nil, ErrThreadNotFound
		}
		currentWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(thread, "workspace")))
		if err != nil {
			return nil, err
		}
		if currentWorkspace != workspaceRealPath {
			return nil, ErrThreadMutationBaselineConflict
		}
		if _, err := FreezeThreadMutationScope(thread, threadID, s.workspaceReader, principal, time.Now().UTC(), s.workspaceSecurity); err != nil {
			return nil, err
		}
		thread, err = s.repository.PatchThreadIfBaseline(threadID, contracts.CloneMap(patch), digest)
		if errors.Is(err, ErrThreadMutationBaselineConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
			return nil, errors.New("patched thread identity mismatch")
		}
		return thread, nil
	}
	return nil, ErrThreadMutationBaselineConflict
}

func (s *Service) patchWorkspace(ctx context.Context, threadID, workspace string, patch map[string]any, initial map[string]any) (map[string]any, error) {
	if s.beginWorkspaceTransition == nil || s.workspaceReader == nil {
		return nil, ErrThreadMutationTransition
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, s.workspaceSecurity.Identity)
	if err != nil {
		return nil, err
	}
	if ThreadIsCaseBound(s.caseThreads, threadID, initial) {
		return nil, ErrCaseWorkspaceSignedRebind
	}
	previousWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(initial, "workspace")))
	if err != nil {
		return nil, err
	}
	targetWorkspace, err := s.workspaceReader.WorkspaceRealPath(workspace)
	if err != nil {
		return nil, err
	}
	transition, err := s.beginWorkspaceTransition(
		ctx, strings.TrimSpace(threadID), previousWorkspace, targetWorkspace,
		principal.TenantID, principal.UserID,
		func() error { return s.quiesceMutationTurns(ctx, threadID) },
	)
	if err != nil || transition == nil {
		return nil, errors.Join(ErrThreadMutationTransition, err)
	}
	defer transition.Abort()
	thread, digest, err := s.repository.ReadThreadMutationBaseline(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, ErrThreadNotFound
	}
	currentWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(thread, "workspace")))
	if err != nil {
		return nil, err
	}
	if currentWorkspace != previousWorkspace {
		return nil, ErrThreadMutationBaselineConflict
	}
	scopes, err := FreezeWorkspaceRebindScopes(thread, threadID, workspace, s.workspaceReader, principal, time.Now().UTC(), s.workspaceSecurity)
	if err != nil {
		return nil, err
	}
	if scopes.Previous.WorkspaceRealPath != previousWorkspace || scopes.Target.WorkspaceRealPath != targetWorkspace {
		return nil, ErrThreadMutationBaselineConflict
	}
	if ThreadIsCaseBound(s.caseThreads, threadID, thread) || domainsecurity.TurnSecurityContextIsCaseSensitive(scopes.Previous) ||
		domainsecurity.TurnSecurityContextIsCaseSensitive(scopes.Target) {
		return nil, ErrCaseWorkspaceSignedRebind
	}
	prepared, err := PrepareWorkspaceMutation(thread, scopes)
	if err != nil {
		return nil, err
	}
	if err := transition.Prepare(ctx, prepared.SecurityContext, s.mutationTimeout()); err != nil {
		return nil, err
	}
	thread, err = s.repository.CommitWorkspaceMutation(WorkspaceMutationCommitRequest{
		ExpectedBaselineDigest: digest, Prepared: prepared, Patch: contracts.CloneMap(patch),
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) ||
		strings.TrimSpace(stringField(thread, "workspace")) != prepared.Workspace {
		return nil, errors.New("workspace mutation durable readback is inconsistent")
	}
	if err := transition.Commit(); err != nil {
		return nil, err
	}
	return thread, nil
}

func (s *Service) beginMutationWriter(ctx context.Context, threadID string, initial map[string]any, quiesceForeground bool) (CompactionTransition, map[string]any, string, domainidentity.PrincipalV1, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return nil, nil, "", domainidentity.PrincipalV1{}, err
	}
	if ctx == nil || s.beginScopeTransition == nil || s.workspaceReader == nil {
		return nil, nil, "", domainidentity.PrincipalV1{}, ErrThreadMutationTransition
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, s.workspaceSecurity.Identity)
	if err != nil {
		return nil, nil, "", domainidentity.PrincipalV1{}, err
	}
	if initial == nil {
		initial, _, err = s.repository.ReadThreadMutationBaseline(threadID)
		if err != nil {
			return nil, nil, "", domainidentity.PrincipalV1{}, err
		}
	}
	if initial == nil {
		return nil, nil, "", domainidentity.PrincipalV1{}, ErrThreadNotFound
	}
	initialWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(initial, "workspace")))
	if err != nil {
		return nil, nil, "", domainidentity.PrincipalV1{}, err
	}
	transition, err := s.beginScopeTransition(
		ctx, strings.TrimSpace(threadID), initialWorkspace, principal.TenantID, principal.UserID,
	)
	if err != nil || transition == nil {
		return nil, nil, "", domainidentity.PrincipalV1{}, errors.Join(ErrThreadMutationTransition, err)
	}
	if quiesceForeground {
		if err := s.quiesceMutationTurns(ctx, threadID); err != nil {
			transition.Abort()
			return nil, nil, "", domainidentity.PrincipalV1{}, err
		}
	}
	thread, digest, err := s.repository.ReadThreadMutationBaseline(threadID)
	if err != nil {
		transition.Abort()
		return nil, nil, "", domainidentity.PrincipalV1{}, err
	}
	if thread == nil {
		transition.Abort()
		return nil, nil, "", domainidentity.PrincipalV1{}, ErrThreadNotFound
	}
	currentWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(thread, "workspace")))
	if err != nil || currentWorkspace != initialWorkspace {
		transition.Abort()
		if err != nil {
			return nil, nil, "", domainidentity.PrincipalV1{}, err
		}
		return nil, nil, "", domainidentity.PrincipalV1{}, ErrThreadMutationBaselineConflict
	}
	currentScope, err := FreezeThreadMutationScope(thread, threadID, s.workspaceReader, principal, time.Now().UTC(), s.workspaceSecurity)
	if err != nil || currentScope.WorkspaceRealPath != initialWorkspace || currentScope.ThreadID != strings.TrimSpace(threadID) {
		transition.Abort()
		if err != nil {
			return nil, nil, "", domainidentity.PrincipalV1{}, err
		}
		return nil, nil, "", domainidentity.PrincipalV1{}, ErrThreadMutationBaselineConflict
	}
	return transition, thread, digest, principal, nil
}

func (s *Service) quiesceMutationTurns(ctx context.Context, threadID string) error {
	if s.quiesceThreadTurns == nil {
		return ErrThreadMutationTransition
	}
	if err := s.quiesceThreadTurns(ctx, strings.TrimSpace(threadID)); err != nil {
		return errors.Join(ErrThreadMutationTransition, err)
	}
	return nil
}

func (s *Service) mutationTimeout() time.Duration {
	if s.transitionTimeout > 0 {
		return s.transitionTimeout
	}
	return 5 * time.Second
}
