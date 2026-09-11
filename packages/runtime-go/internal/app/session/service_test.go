package session

import (
	"errors"
	"testing"
)

type repositoryStub struct {
	request map[string]any
	err     error
}

func (r *repositoryStub) ResumeSession(sessionID string, request map[string]any) (map[string]any, error) {
	r.request = request
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{
		"thread_id":     "thr_resume",
		"session_id":    sessionID,
		"message_count": float64(2),
		"summary":       "resumed",
	}, nil
}

func TestServiceResumeClonesRequestAndResponse(t *testing.T) {
	repo := &repositoryStub{}
	service := NewService(Dependencies{Repository: repo})
	request := map[string]any{"workspace": "/tmp/work"}
	response, err := service.Resume("thr_source", request)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	request["workspace"] = "/tmp/mutated"
	if repo.request["workspace"] != "/tmp/work" {
		t.Fatalf("request was not cloned: %#v", repo.request)
	}
	response["thread_id"] = "mutated"
	second, err := service.Resume("thr_source", map[string]any{})
	if err != nil {
		t.Fatalf("second resume: %v", err)
	}
	if second["thread_id"] != "thr_resume" {
		t.Fatalf("response was not cloned: %#v", second)
	}
}

func TestServiceResumePropagatesErrors(t *testing.T) {
	expected := errors.New("missing session")
	service := NewService(Dependencies{Repository: &repositoryStub{err: expected}})
	if _, err := service.Resume("thr_source", map[string]any{}); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestServiceResumeRequiresRepository(t *testing.T) {
	service := NewService(Dependencies{})
	if _, err := service.Resume("thr_source", map[string]any{}); err == nil {
		t.Fatal("expected missing repository error")
	}
}
