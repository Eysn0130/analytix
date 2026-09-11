package thread

import (
	"context"
	"errors"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

type reviewTurnStarterStub struct {
	request controlapp.StartTurnRequest
	err     error
}

func (s *reviewTurnStarterStub) SendTurn(_ context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
	s.request = request
	if s.err != nil {
		return nil, s.err
	}
	return map[string]any{"turnId": "turn_review"}, nil
}

type reviewRepositoryStub struct {
	item   map[string]any
	event  map[string]any
	err    error
	events []map[string]any
}

func (r *reviewRepositoryStub) AppendItemToTurn(_ string, _ string, item map[string]any) error {
	if r.err != nil {
		return r.err
	}
	r.item = item
	return nil
}

func (r *reviewRepositoryStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	if r.err != nil {
		return nil, nil, r.err
	}
	event["seq"] = float64(len(r.events) + 1)
	r.events = append(r.events, event)
	r.event = event
	return event, nil, nil
}

func TestReviewServiceStartsTurnAndRecordsReviewItem(t *testing.T) {
	turns := &reviewTurnStarterStub{}
	repo := &reviewRepositoryStub{}
	service := NewReviewService(ReviewDependencies{
		Turns:      turns,
		Repository: repo,
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 1, 2, 3, 0, time.UTC)
		},
	})
	response, err := service.StartReview(context.Background(), "thr_1", ReviewRequest{
		Target:     map[string]any{"kind": "commit", "sha": "abc123"},
		Model:      "model-a",
		ProviderID: "provider-a",
	})
	if err != nil {
		t.Fatalf("start review: %v", err)
	}
	if turns.request.Prompt != "Review commit abc123." || turns.request.DisplayText != "Review commit abc123" {
		t.Fatalf("turn request mismatch: %#v", turns.request)
	}
	if turns.request.Model != "model-a" || turns.request.ProviderID != "provider-a" {
		t.Fatalf("provider request mismatch: %#v", turns.request)
	}
	if response["reviewItemId"] != "item_turn_review_review" {
		t.Fatalf("response mismatch: %#v", response)
	}
	if repo.item["kind"] != "review" || repo.item["createdAt"] != "2026-07-03T01:02:03Z" {
		t.Fatalf("review item mismatch: %#v", repo.item)
	}
	if repo.event["kind"] != "item_created" || repo.event["itemId"] != "item_turn_review_review" {
		t.Fatalf("review event mismatch: %#v", repo.event)
	}
}

func TestReviewServiceMapsMissingThread(t *testing.T) {
	service := NewReviewService(ReviewDependencies{
		Turns:      &reviewTurnStarterStub{err: controlapp.ErrThreadNotFound},
		Repository: &reviewRepositoryStub{},
	})
	_, err := service.StartReview(context.Background(), "missing", ReviewRequest{})
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("err = %v, want ErrThreadNotFound", err)
	}
}
