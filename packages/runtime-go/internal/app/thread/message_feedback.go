package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// A narrow extension keeps feedback out of the general thread patch contract.
type MessageFeedbackRepositoryV1 interface {
	ListMessageFeedbackV1(string, func(map[string]any) (map[string]any, error)) ([]domainthread.MessageFeedbackItemV1, error)
	PutMessageFeedbackV1(string, string, domainthread.MessageFeedbackPutRequestV1, func(map[string]any) (map[string]any, error)) (domainthread.MessageFeedbackItemV1, error)
	DeleteMessageFeedbackV1(string, string, string, func(map[string]any) (map[string]any, error)) error
}

func (s *Service) messageFeedbackScopeV1(ctx context.Context, threadID string) (MessageFeedbackRepositoryV1, func(map[string]any) (map[string]any, error), func(), error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return nil, nil, nil, err
	}
	repository, ok := s.repository.(MessageFeedbackRepositoryV1)
	if !ok || ctx == nil || s.beginMetadataRead == nil || s.workspaceReader == nil {
		return nil, nil, nil, ErrThreadMutationTransition
	}
	initial, _, err := s.repository.ReadThreadMutationBaseline(threadID)
	if err != nil {
		return nil, nil, nil, err
	}
	if initial == nil || stringField(initial, "status") == "deleted" {
		return nil, nil, nil, ErrThreadNotFound
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, s.workspaceSecurity.Identity)
	if err != nil {
		return nil, nil, nil, err
	}
	workspace := strings.TrimSpace(stringField(initial, "workspace"))
	realPath, err := s.workspaceReader.WorkspaceRealPath(workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	release, err := s.beginMetadataRead(ctx, threadID, realPath, principal.TenantID, principal.UserID)
	if err != nil || release == nil {
		return nil, nil, nil, errors.Join(ErrThreadMutationTransition, err)
	}
	project := func(thread map[string]any) (map[string]any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if stringField(thread, "id") != threadID || strings.TrimSpace(stringField(thread, "workspace")) != workspace || stringField(thread, "createdAt") != stringField(initial, "createdAt") {
			return nil, ErrThreadMutationBaselineConflict
		}
		currentWorkspace, err := s.workspaceReader.WorkspaceRealPath(strings.TrimSpace(stringField(thread, "workspace")))
		if err != nil {
			return nil, err
		}
		if currentWorkspace != realPath {
			return nil, ErrThreadMutationBaselineConflict
		}
		if _, err := FreezeThreadMutationScope(thread, threadID, s.workspaceReader, principal, time.Now().UTC(), s.workspaceSecurity); err != nil {
			return nil, err
		}
		return s.publicProjector.ProjectThread(thread)
	}
	return repository, project, release, nil
}

func (s *Service) ListMessageFeedbackV1(ctx context.Context, threadID string) ([]domainthread.MessageFeedbackItemV1, error) {
	repository, project, release, err := s.messageFeedbackScopeV1(ctx, threadID)
	if err != nil {
		return nil, err
	}
	defer release()
	return repository.ListMessageFeedbackV1(threadID, project)
}

func (s *Service) PutMessageFeedbackV1(ctx context.Context, threadID, messageID string, request domainthread.MessageFeedbackPutRequestV1) (domainthread.MessageFeedbackItemV1, error) {
	repository, project, release, err := s.messageFeedbackScopeV1(ctx, threadID)
	if err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	defer release()
	return repository.PutMessageFeedbackV1(threadID, messageID, request, project)
}

func (s *Service) DeleteMessageFeedbackV1(ctx context.Context, threadID, messageID, version string) error {
	repository, project, release, err := s.messageFeedbackScopeV1(ctx, threadID)
	if err != nil {
		return err
	}
	defer release()
	return repository.DeleteMessageFeedbackV1(threadID, messageID, version, project)
}
