package usage

import (
	"errors"
	"testing"
)

type runtimeRepositoryStub struct {
	queryRepositoryStub
	threadIDs []string
	listErr   error
}

func (r *runtimeRepositoryStub) ListRuntimeThreadIDs() ([]string, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]string(nil), r.threadIDs...), nil
}

func TestRuntimeServiceBuildsRuntimeResponseFromThreadIDs(t *testing.T) {
	service := RuntimeService{
		Repository: &runtimeRepositoryStub{
			threadIDs: []string{"thr_a", "thr_b"},
		},
	}
	response, err := service.RuntimeResponse([]Record{
		{ThreadID: "thr_a", Usage: Snapshot{TotalTokens: 12, Turns: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	total, _ := response["total"].(map[string]any)
	perThread, _ := response["perThread"].([]any)
	if len(perThread) != 2 || total["turns"] != 1 || total["totalTokens"] != 12 {
		t.Fatalf("runtime response mismatch: %#v", response)
	}
}

func TestRuntimeServicePreservesThreadListErrorsForRuntimeResponse(t *testing.T) {
	cause := errors.New("synthetic thread inventory unavailable")
	service := RuntimeService{Repository: &runtimeRepositoryStub{listErr: cause}}
	response, err := service.RuntimeResponse(nil)
	if response != nil || !errors.Is(err, cause) {
		t.Fatalf("thread list error became successful usage: response=%v err=%v", response, err)
	}
}

func TestRuntimeServiceThreadSnapshotSumsRecords(t *testing.T) {
	service := RuntimeService{
		Repository: &runtimeRepositoryStub{
			queryRepositoryStub: queryRepositoryStub{
				exists: true,
				loadedRecords: []Record{
					{ThreadID: "thr_a", Usage: Snapshot{TotalTokens: 3, Turns: 1}},
					{ThreadID: "thr_a", Usage: Snapshot{TotalTokens: 4, Turns: 1}},
				},
			},
		},
	}
	snapshot, err := service.ThreadSnapshot(" thr_a ")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TotalTokens != 7 || snapshot.Turns != 2 {
		t.Fatalf("snapshot mismatch: %#v", snapshot)
	}
}

func TestRuntimeServiceThreadSnapshotPreservesReadErrors(t *testing.T) {
	for _, source := range []string{"primary", "usage"} {
		t.Run(source, func(t *testing.T) {
			cause := errors.New("synthetic usage observation unavailable")
			repo := &runtimeRepositoryStub{queryRepositoryStub: queryRepositoryStub{exists: true}}
			if source == "primary" {
				repo.existsErr = cause
			} else {
				repo.loadErr = cause
			}
			if _, err := (RuntimeService{Repository: repo}).ThreadSnapshot("thread-synthetic"); !errors.Is(err, cause) {
				t.Fatalf("read error became zero usage: %v", err)
			}
		})
	}
}
