package thread

import (
	"context"
	"errors"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	contracts "analytix.local/runtime-go/internal/contracts"
)

type ReviewRequest struct {
	Target     map[string]any
	Model      string
	ProviderID string
}

type ReviewTurnStarter interface {
	SendTurn(context.Context, controlapp.StartTurnRequest) (map[string]any, error)
}

type ReviewRepository interface {
	AppendItemToTurn(threadID, turnID string, item map[string]any) error
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type ReviewService struct {
	turns      ReviewTurnStarter
	repository ReviewRepository
	now        func() time.Time
}

type ReviewDependencies struct {
	Turns      ReviewTurnStarter
	Repository ReviewRepository
	Now        func() time.Time
}

func NewReviewService(deps ReviewDependencies) ReviewService {
	return ReviewService{
		turns:      deps.Turns,
		repository: deps.Repository,
		now:        deps.Now,
	}
}

func (s ReviewService) StartReview(ctx context.Context, threadID string, request ReviewRequest) (map[string]any, error) {
	if s.turns == nil {
		return nil, errors.New("review turn starter is required")
	}
	if s.repository == nil {
		return nil, errors.New("review repository is required")
	}
	response, err := s.turns.SendTurn(
		ctx,
		controlapp.StartTurnRequest{
			ThreadID:    threadID,
			Prompt:      ReviewPrompt(request.Target),
			DisplayText: ReviewTitle(request.Target),
			Model:       request.Model,
			ProviderID:  request.ProviderID,
		},
	)
	if errors.Is(err, controlapp.ErrThreadNotFound) {
		return nil, ErrThreadNotFound
	}
	if err != nil {
		return nil, err
	}
	turnID := contracts.StringField(response, "turnId")
	reviewItemID := "item_" + turnID + "_review"
	now := s.nowRFC3339Nano()
	item := map[string]any{
		"id":         reviewItemID,
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "assistant",
		"status":     "completed",
		"kind":       "review",
		"title":      ReviewTitle(request.Target),
		"reviewText": "Go runtime default review boundary recorded.",
		"target":     contracts.CloneMap(request.Target),
		"createdAt":  now,
		"finishedAt": now,
	}
	if err := s.repository.AppendItemToTurn(threadID, turnID, item); err != nil {
		return nil, err
	}
	if _, _, err := s.repository.RecordEvent(map[string]any{
		"kind":     "item_created",
		"threadId": threadID,
		"turnId":   turnID,
		"itemId":   reviewItemID,
		"item":     item,
	}); err != nil {
		return nil, err
	}
	response["reviewItemId"] = reviewItemID
	return response, nil
}

func (s ReviewService) nowRFC3339Nano() string {
	now := s.now
	if now == nil {
		now = time.Now
	}
	return now().UTC().Format(time.RFC3339Nano)
}
