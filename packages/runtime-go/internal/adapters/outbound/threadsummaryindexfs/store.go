package threadsummaryindexfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

type Dependencies struct {
	Root                string
	Owner               sync.Locker
	ReadThread          func(string) (map[string]any, error)
	Now                 func() time.Time
	RestartPreservation *PreservedRecordsV1
}

type Stats struct {
	Reads          int
	Backfills      int
	BackfillActive bool
	RebuildActive  bool
}

type Store struct {
	root             string
	owner            sync.Locker
	readThread       func(string) (map[string]any, error)
	now              func() time.Time
	restartPreserved *PreservedRecordsV1

	buildMu        sync.Mutex
	reads          int
	backfills      int
	backfillActive bool
	rebuildActive  bool
	deferred       []record
}

type record struct {
	SchemaVersion float64        `json:"schemaVersion"`
	ThreadID      string         `json:"threadId"`
	Summary       map[string]any `json:"summary"`
	UpdatedAt     string         `json:"updatedAt"`
	WrittenAt     string         `json:"writtenAt"`
	Deleted       bool           `json:"deleted,omitempty"`
}

func New(deps Dependencies) (*Store, error) {
	root := strings.TrimSpace(deps.Root)
	if root == "" || deps.Owner == nil || deps.ReadThread == nil {
		return nil, errors.New("thread summary index dependencies are incomplete")
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	if deps.RestartPreservation != nil {
		if deps.RestartPreservation.path != filepath.Join(root, "thread_summaries.jsonl") {
			return nil, errors.New("summary preservation belongs to another store")
		}
		if err := deps.RestartPreservation.Revalidate(context.Background()); err != nil {
			return nil, err
		}
	}
	return &Store{root: root, owner: deps.Owner, readThread: deps.ReadThread, now: now, restartPreserved: deps.RestartPreservation}, nil
}

func (s *Store) Ensure() error {
	if s == nil {
		return errors.New("thread summary index store is unavailable")
	}
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	s.owner.Lock()
	s.rebuildActive = true
	s.deferred = nil
	s.owner.Unlock()
	records, err := s.buildRecordsReadOnly()
	s.owner.Lock()
	defer s.owner.Unlock()
	defer func() {
		s.rebuildActive = false
		s.deferred = nil
	}()
	if err != nil {
		return err
	}
	records = mergeDeferredRecords(records, s.deferred)
	if err := s.writeRecordsOwnerLocked(records); err != nil {
		return err
	}
	s.backfills++
	return nil
}

func (s *Store) List(
	archivedOnly bool,
	includeArchived bool,
	includeSide bool,
	search string,
	limit int,
) ([]map[string]any, bool, error) {
	if s == nil {
		return nil, true, errors.New("thread summary index store is unavailable")
	}
	s.owner.Lock()
	s.reads++
	summaries, exists, err := s.loadSummariesOwnerLocked()
	s.owner.Unlock()
	if err != nil {
		return nil, true, err
	}
	if !exists {
		s.scheduleBackfill()
		return []map[string]any{}, true, nil
	}
	summaries, err = threadapp.RehydrateSummaryIndex(summaries, s.readThread)
	if err != nil {
		return nil, true, err
	}
	needle := strings.ToLower(strings.TrimSpace(search))
	filtered := make([]map[string]any, 0, len(summaries))
	for _, summary := range summaries {
		if strings.EqualFold(contracts.StringField(summary, "status"), "deleted") {
			continue
		}
		if !threadapp.IncludeThreadInList(summary, summary, threadapp.ListProjectionFilter{
			ArchivedOnly: archivedOnly, IncludeArchived: includeArchived, IncludeSide: includeSide, Search: needle,
		}) {
			continue
		}
		filtered = append(filtered, contracts.CloneMap(summary))
	}
	threadapp.SortThreadSummaries(filtered)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, true, nil
}

// AppendOwnerLocked appends the latest summary projection after the canonical
// thread and its metadata/message sidecars are durable. The caller holds Owner.
func (s *Store) AppendOwnerLocked(thread map[string]any) error {
	if s == nil {
		return errors.New("thread summary index store is unavailable")
	}
	if s.restartPreserved.OwnsThread(strings.TrimSpace(contracts.StringField(thread, "id"))) {
		return ErrRestartPreserved
	}
	record := s.recordForThread(thread)
	if record.ThreadID == "" {
		return nil
	}
	projected, err := validatedRecordMapV1(record)
	if err != nil {
		return err
	}
	if s.restartPreserved != nil {
		if err := s.restartPreserved.AppendIndependent(context.Background(), projected); err != nil {
			return err
		}
	} else {
		if err := filestore.AppendJSONLRecord(s.Path(), record); err != nil {
			return err
		}
	}
	if s.rebuildActive {
		s.deferred = append(s.deferred, record)
	}
	return nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.root, "thread_summaries.jsonl")
}

func (s *Store) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	s.owner.Lock()
	defer s.owner.Unlock()
	return s.StatsOwnerLocked()
}

func (s *Store) StatsOwnerLocked() Stats {
	if s == nil {
		return Stats{}
	}
	return Stats{Reads: s.reads, Backfills: s.backfills, BackfillActive: s.backfillActive, RebuildActive: s.rebuildActive}
}

func (s *Store) scheduleBackfill() {
	s.owner.Lock()
	if s.backfillActive {
		s.owner.Unlock()
		return
	}
	s.backfillActive = true
	s.owner.Unlock()
	go func() {
		_ = s.Ensure()
		s.owner.Lock()
		s.backfillActive = false
		s.owner.Unlock()
	}()
}

func (s *Store) buildRecordsReadOnly() ([]record, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "threads"))
	if errors.Is(err, os.ErrNotExist) {
		return []record{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := []record{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if s.restartPreserved.OwnsThread(entry.Name()) {
			continue
		}
		thread, err := s.readThread(entry.Name())
		if err != nil || thread == nil {
			continue
		}
		record := s.recordForThread(thread)
		if record.ThreadID != "" {
			records = append(records, record)
		}
	}
	sortRecords(records)
	return records, nil
}

func (s *Store) writeRecordsOwnerLocked(records []record) error {
	sortRecords(records)
	projected := make([]map[string]any, 0, len(records))
	for _, record := range records {
		value, err := validatedRecordMapV1(record)
		if err != nil {
			return err
		}
		projected = append(projected, value)
	}
	if s.restartPreserved != nil {
		return s.restartPreserved.ReplaceIndependent(context.Background(), projected)
	}
	return filestore.WriteJSONLFileAtomic(s.Path(), ".thread-summaries-*.tmp", records)
}

func validatedRecordMapV1(record record) (map[string]any, error) {
	value, err := preservationRecordMapV1(record)
	if err != nil {
		return nil, err
	}
	if err := threadapp.ValidateSummaryIndexRecordV1(value); err != nil {
		return nil, err
	}
	return value, nil
}

func preservationRecordMapV1(record record) (map[string]any, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{RequireObject: true})
}

func (s *Store) loadSummariesOwnerLocked() ([]map[string]any, bool, error) {
	file, err := os.Open(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	defer file.Close()
	records := []record{}
	if err := filestore.ReadJSONLLines(file, func(lineNumber int, rawLine string) error {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			return nil
		}
		body := []byte(line)
		if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true}); err != nil {
			return fmt.Errorf("thread summary index line %d is invalid: %w", lineNumber, err)
		}
		var candidate record
		if err := json.Unmarshal(body, &candidate); err != nil {
			return fmt.Errorf("thread summary index line %d is invalid: %w", lineNumber, err)
		}
		if candidate.SchemaVersion != 1 || candidate.Summary == nil {
			return fmt.Errorf("thread summary index line %d has an invalid schema", lineNumber)
		}
		records = append(records, candidate)
		return nil
	}); err != nil {
		return nil, true, err
	}
	latestByID := map[string]record{}
	for index, record := range records {
		threadID := strings.TrimSpace(record.ThreadID)
		if threadID == "" {
			threadID = strings.TrimSpace(contracts.StringField(record.Summary, "id"))
		}
		if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
			return nil, true, fmt.Errorf("thread summary index record %d has an invalid thread id", index+1)
		}
		record.ThreadID = threadID
		latestByID[threadID] = record
	}
	summaries := make([]map[string]any, 0, len(latestByID))
	for _, record := range latestByID {
		if record.Deleted {
			continue
		}
		summary := contracts.CloneMap(record.Summary)
		if strings.TrimSpace(contracts.StringField(summary, "id")) == "" {
			summary["id"] = record.ThreadID
		}
		summaries = append(summaries, summary)
	}
	threadapp.SortThreadSummaries(summaries)
	return summaries, true, nil
}

func (s *Store) recordForThread(thread map[string]any) record {
	summary := threadapp.SummaryIndexProjection(thread)
	status := strings.TrimSpace(contracts.StringField(summary, "status"))
	threadID := strings.TrimSpace(contracts.StringField(summary, "id"))
	return record{
		SchemaVersion: 1, ThreadID: threadID, Summary: summary,
		UpdatedAt: contracts.StringField(summary, "updatedAt"),
		WrittenAt: s.now().UTC().Format(time.RFC3339Nano),
		Deleted:   strings.EqualFold(status, "deleted"),
	}
}

func sortRecords(records []record) {
	for index := range records {
		if strings.TrimSpace(records[index].UpdatedAt) == "" && records[index].Summary != nil {
			records[index].UpdatedAt = contracts.StringField(records[index].Summary, "updatedAt")
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].UpdatedAt == records[j].UpdatedAt {
			return records[i].ThreadID > records[j].ThreadID
		}
		return records[i].UpdatedAt > records[j].UpdatedAt
	})
}

func mergeDeferredRecords(records []record, deferred []record) []record {
	latestDeferred := map[string]record{}
	for _, candidate := range deferred {
		if threadID := strings.TrimSpace(candidate.ThreadID); threadID != "" {
			latestDeferred[threadID] = candidate
		}
	}
	merged := make([]record, 0, len(records)+len(latestDeferred))
	for _, candidate := range records {
		if _, replaced := latestDeferred[strings.TrimSpace(candidate.ThreadID)]; replaced {
			continue
		}
		merged = append(merged, candidate)
	}
	for _, candidate := range latestDeferred {
		merged = append(merged, candidate)
	}
	sortRecords(merged)
	return merged
}
