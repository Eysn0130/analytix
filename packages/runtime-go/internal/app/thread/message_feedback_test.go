package thread

import (
	"context"
	"errors"
	"testing"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type feedbackRepositoryStubV1 struct {
	repositoryStub
	before func()
	wrote  bool
}

func (r *feedbackRepositoryStubV1) ListMessageFeedbackV1(_ string, project func(map[string]any) (map[string]any, error)) ([]domainthread.MessageFeedbackItemV1, error) {
	if r.before != nil {
		r.before()
	}
	_, err := project(r.thread)
	return []domainthread.MessageFeedbackItemV1{}, err
}
func (r *feedbackRepositoryStubV1) PutMessageFeedbackV1(_ string, _ string, _ domainthread.MessageFeedbackPutRequestV1, project func(map[string]any) (map[string]any, error)) (domainthread.MessageFeedbackItemV1, error) {
	if r.before != nil {
		r.before()
	}
	if _, err := project(r.thread); err != nil {
		return domainthread.MessageFeedbackItemV1{}, err
	}
	r.wrote = true
	return domainthread.MessageFeedbackItemV1{}, nil
}
func (r *feedbackRepositoryStubV1) DeleteMessageFeedbackV1(_ string, _ string, _ string, project func(map[string]any) (map[string]any, error)) error {
	if r.before != nil {
		r.before()
	}
	if _, err := project(r.thread); err != nil {
		return err
	}
	r.wrote = true
	return nil
}

func TestMessageFeedbackUseCaseFailsClosedOnScopeChangeV1(t *testing.T) {
	for _, operation := range []string{"list", "put", "delete"} {
		t.Run(operation, func(t *testing.T) {
			repo := &feedbackRepositoryStubV1{repositoryStub: repositoryStub{thread: map[string]any{"id": "thread", "workspace": "/workspace", "createdAt": "2026-10-10T00:00:00Z", "status": "idle", "turns": []any{}}}}
			service := newMutationService(repo)
			released := false
			service.beginMetadataRead = func(context.Context, string, string, string, string) (func(), error) {
				return func() { released = true }, nil
			}
			repo.before = func() { repo.thread["workspace"] = "/workspace/changed" }
			var err error
			switch operation {
			case "list":
				_, err = service.ListMessageFeedbackV1(context.Background(), "thread")
			case "put":
				_, err = service.PutMessageFeedbackV1(context.Background(), "thread", "message", domainthread.MessageFeedbackPutRequestV1{Rating: "positive"})
			case "delete":
				err = service.DeleteMessageFeedbackV1(context.Background(), "thread", "message", "11111111-1111-4111-8111-111111111111")
			}
			if !errors.Is(err, ErrThreadMutationBaselineConflict) || repo.wrote || !released {
				t.Fatal("feedback crossed changed scope or leaked lease", err)
			}
		})
	}
}

func TestMessageFeedbackUseCaseRequiresCurrentAuthorityV1(t *testing.T) {
	repo := &feedbackRepositoryStubV1{repositoryStub: repositoryStub{thread: map[string]any{"id": "thread", "workspace": "/workspace", "createdAt": "2026-10-10T00:00:00Z", "status": "idle", "turns": []any{}}}}
	service := newMutationService(repo)
	// Retire authority after the initial baseline and metadata lease, at the
	// repository's current-state callback; no stale baseline may authorize a write.
	repo.before = func() { service.workspaceSecurity.Observer = mutationWorkspaceObserver{} }
	if _, err := service.PutMessageFeedbackV1(context.Background(), "thread", "message", domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}); err == nil || repo.wrote {
		t.Fatal("missing current authority reached write")
	}
	service.beginMetadataRead = nil
	if _, err := service.ListMessageFeedbackV1(context.Background(), "thread"); !errors.Is(err, ErrThreadMutationTransition) {
		t.Fatal("missing metadata lease was accepted", err)
	}
}
