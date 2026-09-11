package usageindexfs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

type Dependencies struct {
	Root                          string
	Owner                         sync.Locker
	ReadThread                    func(string) (map[string]any, error)
	LoadEvents                    func(string, int) (eventlog.LoadResult, error)
	PendingUsageEventsOwnerLocked func(string) []map[string]any
	BeforeBuildThread             func(string)
	Now                           func() time.Time
	RestartPreservation           *RestartPreservationV1
}

type Stats struct {
	Reads          int
	Backfills      int
	AppendFailures int
	RebuildActive  bool
}

// Only failures of the derived projection authorize rebuilding it. Canonical
// thread reads and physical I/O errors retain their original failure semantics.
var errRepairableProjection = errors.New("usage index derived projection needs repair")

type Store struct {
	root                          string
	owner                         sync.Locker
	readThread                    func(string) (map[string]any, error)
	loadEvents                    func(string, int) (eventlog.LoadResult, error)
	pendingUsageEventsOwnerLocked func(string) []map[string]any
	beforeBuildThread             func(string)
	now                           func() time.Time
	restartPreserved              *RestartPreservationV1

	buildMu        sync.Mutex
	reads          int
	backfills      int
	appendFailures int
	rebuildActive  bool
	deferredEvents []map[string]any
}

type buildResult struct {
	records          []usageapp.IndexRecord
	previousByThread map[string]usageapp.Snapshot
	// Include processed no-op usage events: replaying them can change the raw
	// cumulative baseline even though they did not produce an index row.
	processedUsageSeqs map[string]map[int]bool
}

func New(deps Dependencies) (*Store, error) {
	root := strings.TrimSpace(deps.Root)
	if root == "" || deps.Owner == nil || deps.ReadThread == nil || deps.LoadEvents == nil || deps.PendingUsageEventsOwnerLocked == nil {
		return nil, errors.New("usage index dependencies are incomplete")
	}
	if err := deps.RestartPreservation.Revalidate(context.Background(), root); err != nil {
		return nil, err
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Store{
		root: root, owner: deps.Owner, readThread: deps.ReadThread, loadEvents: deps.LoadEvents,
		pendingUsageEventsOwnerLocked: deps.PendingUsageEventsOwnerLocked,
		beforeBuildThread:             deps.BeforeBuildThread,
		now:                           now,
		restartPreserved:              deps.RestartPreservation,
	}, nil
}

func (s *Store) Ensure() error {
	_, err := s.ensureRecords()
	return err
}

func (s *Store) ensureRecords() ([]usageapp.IndexRecord, error) {
	if s == nil {
		return nil, errors.New("usage index store is unavailable")
	}
	s.buildMu.Lock()
	defer s.buildMu.Unlock()

	s.owner.Lock()
	if err := s.restartPreserved.Revalidate(context.Background(), s.root); err != nil {
		s.owner.Unlock()
		return nil, err
	}
	s.rebuildActive = true
	s.deferredEvents = nil
	s.owner.Unlock()

	build, buildErr := s.buildRecordsReadOnly()

	s.owner.Lock()
	defer s.owner.Unlock()
	defer func() {
		s.rebuildActive = false
		s.deferredEvents = nil
	}()
	if buildErr != nil {
		return nil, buildErr
	}
	if err := s.mergeDeferredOwnerLocked(&build); err != nil {
		return nil, err
	}
	if err := s.writeRecordsOwnerLocked(build.records); err != nil {
		return nil, err
	}
	return build.records, nil
}

func (s *Store) LoadRecords(threadID string) ([]usageapp.Record, error) {
	if s == nil {
		return nil, errors.New("usage index store is unavailable")
	}
	threadID = strings.TrimSpace(threadID)
	if s.restartPreserved != nil {
		if err := s.restartPreserved.Revalidate(context.Background(), s.root); err != nil {
			return nil, err
		}
		if threadID == "" || s.restartPreserved.OwnsThread(threadID) {
			return nil, ErrRestartPreserved
		}
	}
	s.owner.Lock()
	s.reads++
	indexed, err := s.loadIndex(threadID)
	s.owner.Unlock()
	if errors.Is(err, os.ErrNotExist) {
		// A new thread without usage has no per-thread index. Use the verified
		// rebuild result, which excludes restart-preserved rows, and avoid
		// reopening an absent file or racing another projection replacement.
		indexed, err = s.ensureRecords()
		if err == nil && threadID != "" {
			indexed = recordsForThread(indexed, threadID)
		}
	}
	if err != nil {
		return nil, err
	}
	indexedSeqs := map[string]map[int]bool{}
	previousByThread := map[string]usageapp.Snapshot{}
	for _, record := range indexed {
		if indexedSeqs[record.ThreadID] == nil {
			indexedSeqs[record.ThreadID] = map[int]bool{}
		}
		indexedSeqs[record.ThreadID][record.Seq] = true
		if record.RawUsage.HasUsage() {
			previousByThread[record.ThreadID] = record.RawUsage
		}
	}
	s.owner.Lock()
	pending, pendingErr := s.pendingRecordsOwnerLocked(threadID, previousByThread, indexedSeqs)
	s.owner.Unlock()
	if pendingErr != nil {
		return nil, pendingErr
	}
	indexed = append(indexed, pending...)
	return usageapp.RecordsFromIndex(indexed, s.now().UTC().Format(time.RFC3339Nano)), nil
}

// AppendEventOwnerLocked updates the derived usage index after the canonical
// event append. The caller must hold Owner for the entire call.
func (s *Store) AppendEventOwnerLocked(event map[string]any) error {
	if s == nil {
		return errors.New("usage index store is unavailable")
	}
	if err := s.checkRestartEffectV1(contracts.StringField(event, "threadId")); err != nil {
		return err
	}
	if s.rebuildActive {
		s.deferredEvents = append(s.deferredEvents, contracts.CloneMap(event))
		return nil
	}
	record, ok, err := s.recordForEvent(event, nil, nil)
	if err != nil || !ok {
		if err != nil {
			s.appendFailures++
		}
		return err
	}
	if s.restartPreserved != nil {
		return s.restartPreserved.writeIndependent(context.Background(), []usageapp.IndexRecord{record}, true)
	}
	if err := s.appendRecord(s.Path(), record); err != nil {
		s.appendFailures++
		return err
	}
	if err := s.appendRecord(s.ThreadPath(record.ThreadID), record); err != nil {
		s.appendFailures++
		return err
	}
	return nil
}

// SettleTerminalEventOwnerLocked proves the global and per-thread projections
// contain the exact terminal usage record. Missing or divergent projections
// are rebuilt and then verified before publication may continue.
func (s *Store) SettleTerminalEventOwnerLocked(event map[string]any) error {
	if s == nil {
		return errors.New("usage index store is unavailable")
	}
	if err := s.checkRestartEffectV1(contracts.StringField(event, "threadId")); err != nil {
		return err
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(event, "turnId"))
	seq, seqOK := contracts.NumericSeq(event["seq"])
	rawUsage, _ := event["usage"].(map[string]any)
	if threadID == "" || turnID == "" || !seqOK || seq <= 0 || rawUsage == nil {
		return errors.New("general terminal usage event is invalid")
	}
	if s.rebuildActive {
		queued := false
		for _, deferred := range s.deferredEvents {
			deferredSeq, _ := contracts.NumericSeq(deferred["seq"])
			if contracts.StringField(deferred, "threadId") == threadID && deferredSeq == seq {
				queued = true
				break
			}
		}
		if !queued {
			s.deferredEvents = append(s.deferredEvents, contracts.CloneMap(event))
		}
		// The reader may retain an older canonical prefix. Keep its deferred
		// merge intact, but prove settlement now under Owner before publishing.
		// Never wait for buildMu here: its reader needs Owner to finish.
	}
	if err := s.verifyTerminalEventOwnerLocked(event); err == nil {
		return nil
	} else if !errors.Is(err, errRepairableProjection) {
		return err
	}
	if err := s.rebuildOwnerLocked(); err != nil {
		return err
	}
	if err := s.verifyTerminalEventOwnerLocked(event); err != nil {
		return errors.Join(err, errors.New("general terminal usage index did not converge after rebuild"))
	}
	return nil
}

func (s *Store) VerifyTerminalEvent(event map[string]any) error {
	if s == nil {
		return errors.New("usage index store is unavailable")
	}
	s.owner.Lock()
	defer s.owner.Unlock()
	if err := s.checkRestartEffectV1(contracts.StringField(event, "threadId")); err != nil {
		return err
	}
	return s.verifyTerminalEventOwnerLocked(event)
}

func (s *Store) checkRestartEffectV1(threadID string) error {
	if s.restartPreserved == nil {
		return nil
	}
	if err := s.restartPreserved.Revalidate(context.Background(), s.root); err != nil {
		return err
	}
	if s.restartPreserved.OwnsThread(strings.TrimSpace(threadID)) {
		return ErrRestartPreserved
	}
	return nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.root, "usage_events", "index.jsonl")
}

func (s *Store) ThreadPath(threadID string) string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.root, "usage_events", "threads", contracts.SafeRecordID(threadID)+".jsonl")
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
	return Stats{Reads: s.reads, Backfills: s.backfills, AppendFailures: s.appendFailures, RebuildActive: s.rebuildActive}
}

func (s *Store) pendingRecordsOwnerLocked(
	threadID string,
	previousByThread map[string]usageapp.Snapshot,
	indexedSeqs map[string]map[int]bool,
) ([]usageapp.IndexRecord, error) {
	events := s.pendingUsageEventsOwnerLocked(threadID)
	sort.SliceStable(events, func(i, j int) bool {
		left, _ := contracts.NumericSeq(events[i]["seq"])
		right, _ := contracts.NumericSeq(events[j]["seq"])
		return left < right
	})
	records := make([]usageapp.IndexRecord, 0, len(events))
	for _, event := range events {
		pendingThreadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
		if err := s.checkRestartEffectV1(pendingThreadID); err != nil {
			return nil, err
		}
		seq, _ := contracts.NumericSeq(event["seq"])
		if indexedSeqs[pendingThreadID] != nil && indexedSeqs[pendingThreadID][seq] {
			continue
		}
		thread, err := s.readThread(pendingThreadID)
		if err != nil {
			return nil, err
		}
		record, ok, err := s.recordForEvent(event, thread, previousByThread)
		if err != nil || !ok {
			return records, err
		}
		records = append(records, record)
		if indexedSeqs[pendingThreadID] == nil {
			indexedSeqs[pendingThreadID] = map[int]bool{}
		}
		indexedSeqs[pendingThreadID][seq] = true
	}
	return records, nil
}

func (s *Store) recordForEvent(
	event map[string]any,
	thread map[string]any,
	previousByThread map[string]usageapp.Snapshot,
) (usageapp.IndexRecord, bool, error) {
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	if err := s.checkRestartEffectV1(threadID); err != nil {
		return usageapp.IndexRecord{}, false, err
	}
	if threadID == "" {
		return usageapp.IndexRecord{}, false, nil
	}
	rawUsageMap, _ := event["usage"].(map[string]any)
	if rawUsageMap == nil {
		return usageapp.IndexRecord{}, false, nil
	}
	previous, hasPrevious := usageapp.Snapshot{}, false
	if previousByThread != nil {
		previous, hasPrevious = previousByThread[threadID]
	} else if prior, ok, err := s.latestRaw(threadID); err != nil {
		return usageapp.IndexRecord{}, false, err
	} else if ok {
		previous, hasPrevious = prior, true
	}
	result := usageapp.BuildIndexRecord(usageapp.IndexRecordInput{
		Event: event, Thread: thread, Previous: previous, HasPrevious: hasPrevious,
		CompletedAtFallback: s.now().UTC().Format(time.RFC3339Nano),
	})
	if previousByThread != nil && result.UpdatePrevious {
		previousByThread[threadID] = result.Current
	}
	return result.Record, result.OK, nil
}

func (s *Store) latestRaw(threadID string) (usageapp.Snapshot, bool, error) {
	records, err := s.loadIndex(threadID)
	if errors.Is(err, os.ErrNotExist) {
		return usageapp.Snapshot{}, false, nil
	}
	if err != nil {
		return usageapp.Snapshot{}, false, err
	}
	if len(records) == 0 {
		return usageapp.Snapshot{}, false, nil
	}
	return records[len(records)-1].RawUsage, true, nil
}

func (s *Store) buildRecordsReadOnly() (buildResult, error) {
	threadIDs, err := s.allThreadIDsReadOnly()
	if err != nil {
		return buildResult{}, err
	}
	build := buildResult{
		records: []usageapp.IndexRecord{}, previousByThread: map[string]usageapp.Snapshot{},
		processedUsageSeqs: map[string]map[int]bool{},
	}
	previousByThread := map[string]usageapp.Snapshot{}
	for _, threadID := range threadIDs {
		if s.beforeBuildThread != nil {
			s.beforeBuildThread(threadID)
		}
		thread, err := s.readThread(threadID)
		if err != nil || thread == nil {
			return buildResult{}, errors.Join(err, errors.New("usage index cannot read a canonical thread"))
		}
		result, err := s.loadEvents(threadID, 0)
		if err != nil {
			return buildResult{}, err
		}
		if len(result.Diagnostics) != 0 {
			return buildResult{}, errors.New("usage index cannot rebuild from an event log with diagnostics")
		}
		for _, event := range result.Events {
			if contracts.StringField(event, "kind") != "usage" {
				continue
			}
			record, ok, err := s.recordForEvent(event, thread, previousByThread)
			if err != nil {
				return buildResult{}, err
			}
			seq, _ := contracts.NumericSeq(event["seq"])
			if build.processedUsageSeqs[threadID] == nil {
				build.processedUsageSeqs[threadID] = map[int]bool{}
			}
			build.processedUsageSeqs[threadID][seq] = true
			if ok {
				build.records = append(build.records, record)
			}
		}
	}
	for threadID, previous := range previousByThread {
		build.previousByThread[threadID] = previous
	}
	usageapp.SortIndexRecords(build.records)
	return build, nil
}

func (s *Store) mergeDeferredOwnerLocked(build *buildResult) error {
	if len(s.deferredEvents) == 0 {
		return nil
	}
	events := make([]map[string]any, 0, len(s.deferredEvents))
	for _, event := range s.deferredEvents {
		events = append(events, contracts.CloneMap(event))
	}
	sort.SliceStable(events, func(i, j int) bool {
		leftThread := contracts.StringField(events[i], "threadId")
		rightThread := contracts.StringField(events[j], "threadId")
		if leftThread != rightThread {
			return leftThread < rightThread
		}
		leftSeq, _ := contracts.NumericSeq(events[i]["seq"])
		rightSeq, _ := contracts.NumericSeq(events[j]["seq"])
		return leftSeq < rightSeq
	})
	for _, event := range events {
		threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
		if err := s.checkRestartEffectV1(threadID); err != nil {
			return err
		}
		seq, _ := contracts.NumericSeq(event["seq"])
		if build.processedUsageSeqs[threadID] != nil && build.processedUsageSeqs[threadID][seq] {
			continue
		}
		thread, err := s.readThread(threadID)
		if err != nil {
			return err
		}
		record, ok, err := s.recordForEvent(event, thread, build.previousByThread)
		if err != nil {
			return err
		}
		if build.processedUsageSeqs[threadID] == nil {
			build.processedUsageSeqs[threadID] = map[int]bool{}
		}
		build.processedUsageSeqs[threadID][seq] = true
		if ok {
			build.records = append(build.records, record)
		}
	}
	usageapp.SortIndexRecords(build.records)
	return nil
}

func (s *Store) rebuildOwnerLocked() error {
	build, err := s.buildRecordsReadOnly()
	if err != nil {
		return err
	}
	return s.writeRecordsOwnerLocked(build.records)
}

func (s *Store) writeRecordsOwnerLocked(records []usageapp.IndexRecord) error {
	usageapp.SortIndexRecords(records)
	if s.restartPreserved != nil {
		if err := s.restartPreserved.writeIndependent(context.Background(), records, false); err != nil {
			return err
		}
		s.backfills++
		return nil
	}
	if err := filestore.RemoveAll(filepath.Join(s.root, "usage_events", "threads")); err != nil {
		return err
	}
	if err := filestore.WriteJSONLFileAtomic(s.Path(), ".usage-index-*.tmp", records); err != nil {
		return err
	}
	for _, record := range records {
		if err := s.appendRecord(s.ThreadPath(record.ThreadID), record); err != nil {
			return err
		}
	}
	s.backfills++
	return nil
}

func (s *Store) loadIndex(threadID string) ([]usageapp.IndexRecord, error) {
	path := s.Path()
	if strings.TrimSpace(threadID) != "" {
		path = s.ThreadPath(threadID)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	records, readErr := readUsageIndexV1(file, threadID)
	if closeErr := file.Close(); closeErr != nil {
		if readErr != nil && !errors.Is(readErr, errRepairableProjection) {
			return nil, errors.Join(readErr, closeErr)
		}
		return nil, closeErr
	}
	return records, readErr
}

func readUsageIndexV1(reader io.Reader, threadID string) ([]usageapp.IndexRecord, error) {
	out := []usageapp.IndexRecord{}
	var projectionErr error
	err := filestore.ReadJSONLLines(reader, func(lineNumber int, rawLine string) error {
		// Finish the physical read even after malformed derived content. A
		// truncated line and its I/O failure must never authorize a rebuild.
		if projectionErr != nil {
			return nil
		}
		line := strings.TrimSpace(rawLine)
		if line == "" {
			return nil
		}
		body := []byte(line)
		if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true}); err != nil {
			projectionErr = fmt.Errorf("usage index line %d is invalid: %w", lineNumber, err)
			return nil
		}
		var record usageapp.IndexRecord
		if err := json.Unmarshal(body, &record); err != nil {
			projectionErr = fmt.Errorf("usage index line %d is invalid: %w", lineNumber, err)
			return nil
		}
		if threadID != "" && record.ThreadID != threadID {
			projectionErr = fmt.Errorf("usage index line %d belongs to another thread", lineNumber)
			return nil
		}
		out = append(out, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if projectionErr != nil {
		return nil, errors.Join(errRepairableProjection, projectionErr)
	}
	usageapp.SortIndexRecords(out)
	return out, nil
}

func (s *Store) appendRecord(path string, record usageapp.IndexRecord) error {
	if strings.TrimSpace(record.ThreadID) == "" {
		return nil
	}
	return filestore.AppendJSONLRecord(path, record)
}

func (s *Store) allThreadIDsReadOnly() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "threads"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := strings.TrimSpace(entry.Name())
		if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
			return nil, errors.New("usage index found an unsafe thread directory")
		}
		if s.restartPreserved.OwnsThread(threadID) {
			continue
		}
		thread, err := s.readThread(threadID)
		if err != nil || thread == nil {
			return nil, errors.Join(err, errors.New("usage index cannot read a canonical thread"))
		}
		ids = append(ids, threadID)
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) verifyTerminalEventOwnerLocked(event map[string]any) error {
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	seq, seqOK := contracts.NumericSeq(event["seq"])
	if threadID == "" || !seqOK || seq <= 0 {
		return errors.New("general terminal usage event identity is invalid")
	}
	global, globalErr := s.loadIndex("")
	threadRecords, threadErr := s.loadIndex(threadID)
	if globalErr != nil || threadErr != nil {
		// A missing or malformed projection cannot mask a separate read failure.
		var readErrors []error
		for _, err := range []error{globalErr, threadErr} {
			if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errRepairableProjection) {
				readErrors = append(readErrors, err)
			}
		}
		if len(readErrors) != 0 {
			return errors.Join(readErrors...)
		}
		return errors.Join(errRepairableProjection, globalErr, threadErr)
	}
	expected, expectedErr := s.expectedTerminalRecordOwnerLocked(event, threadRecords)
	if expectedErr != nil {
		return expectedErr
	}
	globalForThread := recordsForThread(global, threadID)
	if !reflect.DeepEqual(globalForThread, threadRecords) {
		return errors.Join(errRepairableProjection, errors.New("general terminal usage index projections diverged"))
	}
	globalMatches := recordsAt(global, threadID, seq)
	threadMatches := recordsAt(threadRecords, threadID, seq)
	if len(globalMatches) != 1 || len(threadMatches) != 1 ||
		!reflect.DeepEqual(globalMatches[0], threadMatches[0]) || !recordEqual(globalMatches[0], expected) {
		return errors.Join(errRepairableProjection, errors.New("general terminal usage index does not contain the exact record"))
	}
	return nil
}

func (s *Store) expectedTerminalRecordOwnerLocked(
	event map[string]any,
	threadRecords []usageapp.IndexRecord,
) (usageapp.IndexRecord, error) {
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	seq, seqOK := contracts.NumericSeq(event["seq"])
	if threadID == "" || !seqOK || seq <= 0 {
		return usageapp.IndexRecord{}, errors.New("general terminal usage event identity is invalid")
	}
	thread, err := s.readThread(threadID)
	if err != nil || thread == nil {
		return usageapp.IndexRecord{}, errors.Join(err, errors.New("general terminal usage index cannot read its canonical thread"))
	}
	seenSeq := map[int]bool{}
	previous := usageapp.Snapshot{}
	hasPrevious := false
	previousSeq := 0
	for _, record := range threadRecords {
		if record.ThreadID != threadID || record.Seq <= 0 || seenSeq[record.Seq] {
			return usageapp.IndexRecord{}, errors.Join(errRepairableProjection, errors.New("general terminal usage index thread projection is invalid"))
		}
		seenSeq[record.Seq] = true
		if record.Seq < seq && record.Seq > previousSeq {
			previous = record.RawUsage
			hasPrevious = true
			previousSeq = record.Seq
		}
	}
	result := usageapp.BuildIndexRecord(usageapp.IndexRecordInput{
		Event: event, Thread: thread, Previous: previous, HasPrevious: hasPrevious,
		CompletedAtFallback: strings.TrimSpace(contracts.StringField(event, "timestamp")),
	})
	if !result.OK {
		return usageapp.IndexRecord{}, errors.New("general terminal usage event cannot produce an index record")
	}
	return result.Record, nil
}

func recordsAt(records []usageapp.IndexRecord, threadID string, seq int) []usageapp.IndexRecord {
	out := make([]usageapp.IndexRecord, 0, 1)
	for _, record := range records {
		if record.ThreadID == threadID && record.Seq == seq {
			out = append(out, record)
		}
	}
	return out
}

func recordsForThread(records []usageapp.IndexRecord, threadID string) []usageapp.IndexRecord {
	out := make([]usageapp.IndexRecord, 0)
	for _, record := range records {
		if record.ThreadID == threadID {
			out = append(out, record)
		}
	}
	return out
}

func recordEqual(left, right usageapp.IndexRecord) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
