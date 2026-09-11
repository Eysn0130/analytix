package usage

import (
	"errors"
	"testing"
)

type queryRepositoryStub struct {
	exists        bool
	existsErr     error
	loadErr       error
	existsCalls   []string
	loadCalls     []string
	loadedRecords []Record
}

func (r *queryRepositoryStub) ThreadExists(threadID string) (bool, error) {
	r.existsCalls = append(r.existsCalls, threadID)
	if r.existsErr != nil {
		return false, r.existsErr
	}
	return r.exists, nil
}

func (r *queryRepositoryStub) LoadRecords(threadID string) ([]Record, error) {
	r.loadCalls = append(r.loadCalls, threadID)
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	return append([]Record(nil), r.loadedRecords...), nil
}

func TestQueryServiceRecordsReturnsEmptyForMissingThread(t *testing.T) {
	repo := &queryRepositoryStub{}
	records, err := (QueryService{Repository: repo}).Records(" thread-a ")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("missing thread should not load records: %#v", records)
	}
	if len(repo.existsCalls) != 1 || repo.existsCalls[0] != "thread-a" {
		t.Fatalf("thread existence should use trimmed id: %#v", repo.existsCalls)
	}
	if len(repo.loadCalls) != 0 {
		t.Fatalf("missing thread should not load usage records: %#v", repo.loadCalls)
	}
}

func TestQueryServiceRecordsLoadsAllWhenThreadIDIsBlank(t *testing.T) {
	repo := &queryRepositoryStub{loadedRecords: []Record{
		{ThreadID: "thread-b", Model: "model-b", CompletedAt: "2026-07-01T10:00:00Z"},
		{ThreadID: "thread-a", Model: "model-a", CompletedAt: "2026-07-01T09:00:00Z"},
	}}
	records, err := (QueryService{Repository: repo}).Records("   ")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if len(repo.existsCalls) != 0 {
		t.Fatalf("blank thread id should not check thread existence: %#v", repo.existsCalls)
	}
	if len(repo.loadCalls) != 1 || repo.loadCalls[0] != "" {
		t.Fatalf("blank thread id should load all records: %#v", repo.loadCalls)
	}
	if got := records[0].ThreadID; got != "thread-a" {
		t.Fatalf("records should be sorted by completion time: %#v", records)
	}
}

func TestQueryServiceRecordsSortsByCompletionThreadAndModel(t *testing.T) {
	repo := &queryRepositoryStub{
		exists: true,
		loadedRecords: []Record{
			{ThreadID: "thread-b", Model: "model-b", CompletedAt: "2026-07-01T10:00:00Z"},
			{ThreadID: "thread-a", Model: "model-c", CompletedAt: "2026-07-01T10:00:00Z"},
			{ThreadID: "thread-a", Model: "model-a", CompletedAt: "2026-07-01T10:00:00Z"},
			{ThreadID: "thread-z", Model: "model-z", CompletedAt: "2026-07-01T09:00:00Z"},
		},
	}
	records, err := (QueryService{Repository: repo}).Records("thread-a")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	got := []string{records[0].Model, records[1].Model, records[2].Model, records[3].Model}
	want := []string{"model-z", "model-a", "model-c", "model-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort order mismatch: got %#v want %#v", got, want)
		}
	}
}

func TestQueryServiceRecordsReturnsRepositoryErrors(t *testing.T) {
	existsErr := errors.New("exists failed")
	repo := &queryRepositoryStub{existsErr: existsErr}
	if _, err := (QueryService{Repository: repo}).Records("thread-a"); !errors.Is(err, existsErr) {
		t.Fatalf("existence error mismatch: %v", err)
	}

	loadErr := errors.New("load failed")
	repo = &queryRepositoryStub{exists: true, loadErr: loadErr}
	if _, err := (QueryService{Repository: repo}).Records("thread-a"); !errors.Is(err, loadErr) {
		t.Fatalf("load error mismatch: %v", err)
	}
}
