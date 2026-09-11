package eventlog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domaineventlifecycle "analytix.local/runtime-go/internal/domain/eventlifecycle"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainrestrictedevidence "analytix.local/runtime-go/internal/domain/restrictedevidence"
)

type JSONLDiagnostic struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Error   string `json:"error"`
	Preview string `json:"preview"`
}

type LoadResult struct {
	Events      []map[string]any  `json:"events"`
	Diagnostics []JSONLDiagnostic `json:"diagnostics"`
}

type EventLogFrontierV1 struct {
	Exists bool
	SHA256 string
	Size   int64
}

var (
	ErrEventLogFrontierChanged = errors.New("event log frontier changed before atomic append")
	// ErrInvalidEventLifecycleV1 is a compatibility alias. The domain package
	// owns the sentinel and the lifecycle vocabulary.
	ErrInvalidEventLifecycleV1 = domaineventlifecycle.ErrInvalidEventLifecycleV1
)

type ThreadSearchResult struct {
	ThreadID   string          `json:"threadId"`
	Title      string          `json:"title,omitempty"`
	Status     string          `json:"status,omitempty"`
	Archived   bool            `json:"archived"`
	HighestSeq domainevent.Seq `json:"highestSeq"`
	Match      string          `json:"match,omitempty"`
}

func (r LoadResult) RuntimeEvents() []domainevent.RuntimeEvent {
	events := make([]domainevent.RuntimeEvent, 0, len(r.Events))
	for _, item := range r.Events {
		events = append(events, RuntimeEventFromMap(item))
	}
	return events
}

type Store struct {
	root              string
	restartPreserved  *SemanticRestartPreservationV1
	transactionRoot   string
	replaceFile       func(string, string) error
	syncParentFolder  func(string) error
	syncRecoveredFile func(*os.File) error
	atomicAppendCut   func(string) error
	published         sync.Map
}

const (
	PublicationNamespaceAcceptedFinal     = "accepted_final"
	PublicationNamespaceGeneralTerminal   = "general_terminal"
	PublicationNamespaceCheckpointCapture = "checkpoint_capture"
)

func NewStore(root string) *Store {
	return &Store{
		root:              root,
		transactionRoot:   canonicalEventlogTransactionRoot(root),
		replaceFile:       atomicReplaceFile,
		syncParentFolder:  syncParentDirectory,
		syncRecoveredFile: func(file *os.File) error { return file.Sync() },
	}
}

func (s *Store) PublicationMarked(namespace, id string) bool {
	if s == nil || strings.TrimSpace(namespace) == "" || strings.TrimSpace(id) == "" {
		return false
	}
	_, marked := s.published.Load(namespace + "\x00" + id)
	return marked
}

func (s *Store) MarkPublication(namespace, id string) bool {
	if s == nil || strings.TrimSpace(namespace) == "" || strings.TrimSpace(id) == "" {
		return false
	}
	_, loaded := s.published.LoadOrStore(namespace+"\x00"+id, struct{}{})
	return !loaded
}

type AtomicAppendError struct {
	Committed bool
	Cause     error
}

func (err *AtomicAppendError) Error() string {
	if err == nil || err.Cause == nil {
		return "atomic event append failed"
	}
	return err.Cause.Error()
}

func (err *AtomicAppendError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func AtomicAppendCommitted(err error) bool {
	var appendErr *AtomicAppendError
	return errors.As(err, &appendErr) && appendErr.Committed
}

func (s *Store) HighestSeqAfterCommittedTail(threadID string, expected []map[string]any) (int, error) {
	result, err := s.LoadSince(threadID, 0)
	if err != nil || len(result.Diagnostics) != 0 || len(result.Events) < len(expected) {
		return 0, errors.New("committed atomic event append readback is incomplete")
	}
	offset := len(result.Events) - len(expected)
	for index := range expected {
		if !sameEventJSON(result.Events[offset+index], expected[index]) {
			return 0, errors.New("committed atomic event append readback diverged")
		}
	}
	highest, ok := contracts.NumericSeq(result.Events[len(result.Events)-1]["seq"])
	if !ok {
		return 0, errors.New("committed atomic event append sequence is invalid")
	}
	return highest, nil
}

func (s *Store) ThreadDir(threadID string) string {
	return filepath.Join(s.root, "threads", contracts.SafeRecordID(threadID))
}

func (s *Store) EventsPath(threadID string) string {
	return filepath.Join(s.ThreadDir(threadID), "events.jsonl")
}

func (s *Store) AppendEvent(threadID string, event map[string]any) error {
	return s.AppendEventsAtomic(threadID, []map[string]any{event})
}

func (s *Store) Append(event domainevent.RuntimeEvent) error {
	record, threadID, err := RuntimeEventToMap(event)
	if err != nil {
		return err
	}
	return s.AppendEvent(threadID, record)
}

func (s *Store) AppendEvents(threadID string, events []map[string]any, atomic bool) error {
	if s.restartPreserved.OwnsThread(threadID) {
		return ErrRestartPreserved
	}
	if len(events) != 1 {
		if atomic {
			return s.AppendEventsAtomic(threadID, events)
		}
		return errors.New("non-atomic event append requires exactly one event")
	}
	err := s.appendSingleEventAtomic(threadID, events[0])
	if errors.Is(err, errAtomicSingleFallback) {
		if atomic {
			return s.AppendEventsAtomic(threadID, events)
		}
		return s.AppendEvent(threadID, events[0])
	}
	return err
}

// AppendEventsAtomic replaces the event log with its prior bytes plus the
// complete bundle in one target-native atomic replacement. The adapter
// serializes every process-local reader and writer targeting the same root and
// thread, so live intermediate files cannot be mistaken for crash residue.
// Replay observes either none or all of the new events after a process crash.
func (s *Store) AppendEventsAtomic(threadID string, events []map[string]any) error {
	return s.appendEventsAtomic(threadID, events, nil)
}

func (s *Store) AppendEventsAtomicAtFrontier(
	threadID string,
	events []map[string]any,
	expected EventLogFrontierV1,
) error {
	return s.appendEventsAtomic(threadID, events, &expected)
}

func (s *Store) appendEventsAtomic(
	threadID string,
	events []map[string]any,
	expected *EventLogFrontierV1,
) error {
	if s.restartPreserved.OwnsThread(threadID) {
		return ErrRestartPreserved
	}
	if validateExactEventThreadID(threadID) != nil || len(events) == 0 {
		return errors.New("threadId and at least one event are required")
	}
	bundleSequences := make([]int, len(events))
	for index, event := range events {
		if stringValue(event["threadId"]) != threadID {
			return errors.New("atomic event bundle contains a different threadId")
		}
		sequence, ok := exactEventSequenceV1(event["seq"])
		if !ok || index > 0 && sequence != bundleSequences[index-1]+1 {
			return errors.New("atomic event bundle sequence is invalid")
		}
		bundleSequences[index] = sequence
		if err := domainevent.ValidatePublicRecord(event); err != nil {
			return err
		}
		if err := domaineventlifecycle.ValidateNewEventLifecycleV1(event); err != nil {
			return err
		}
	}
	unlock := s.lockThreadTransaction(threadID)
	defer unlock()

	dir := s.ThreadDir(threadID)
	_, exists, err := validateEventThreadDirectory(dir)
	if err != nil {
		return err
	}
	if !exists {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	if _, exists, err := validateEventThreadDirectory(dir); err != nil || !exists {
		return errors.Join(err, errors.New("event log thread directory could not be established safely"))
	}
	if _, err := s.recoverAtomicBundle(threadID); err != nil {
		return err
	}
	current, validatedFrontier, err := s.loadSinceWithFrontierLocked(context.Background(), threadID, -1, false)
	if err != nil {
		return err
	}
	if len(current.Diagnostics) != 0 {
		return errors.New("event log requires semantic content migration before append")
	}
	if expected != nil && !sameEventLogFrontierV1(validatedFrontier, *expected) {
		return ErrEventLogFrontierChanged
	}
	nextSequence := 0
	if len(current.Events) > 0 {
		last, ok := exactEventSequenceV1(current.Events[len(current.Events)-1]["seq"])
		if !ok {
			return errors.New("event log sequence frontier is invalid")
		}
		nextSequence = last + 1
	}
	if nextSequence > 0 && bundleSequences[0] != nextSequence {
		return errors.New("atomic event bundle does not extend the exact sequence frontier")
	}
	path := s.EventsPath(threadID)
	var source *os.File
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if !info.Mode().IsRegular() {
			return errors.New("event log is not a regular file")
		}
		source, err = os.Open(path)
		if err != nil {
			return err
		}
		openedInfo, statErr := source.Stat()
		if statErr != nil || !os.SameFile(info, openedInfo) {
			_ = source.Close()
			return errors.New("event log identity changed before atomic append")
		}
		if openedInfo.Size() > 0 {
			last := []byte{0}
			if _, err := source.ReadAt(last, openedInfo.Size()-1); err != nil {
				_ = source.Close()
				return err
			}
			if last[0] != '\n' {
				_ = source.Close()
				return errors.New("event log is not newline terminated")
			}
			if _, err := source.Seek(0, io.SeekStart); err != nil {
				_ = source.Close()
				return err
			}
		}
	}
	currentFrontier := EventLogFrontierV1{Exists: false, SHA256: digestBytes(nil), Size: 0}
	if source != nil {
		digest, size, err := digestOpenEventLogSource(source)
		if err != nil {
			_ = source.Close()
			return err
		}
		currentFrontier = EventLogFrontierV1{Exists: true, SHA256: digest, Size: size}
	}
	if !sameEventLogFrontierV1(currentFrontier, validatedFrontier) {
		if source != nil {
			_ = source.Close()
		}
		return ErrEventLogFrontierChanged
	}
	if expected != nil && !sameEventLogFrontierV1(currentFrontier, *expected) {
		if source != nil {
			_ = source.Close()
		}
		return ErrEventLogFrontierChanged
	}

	tempPath := filepath.Join(dir, atomicBundleCandidateName)
	temp, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if source != nil {
			_ = source.Close()
		}
		return err
	}
	closed := false
	cleanup := true
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		if source != nil {
			_ = source.Close()
		}
		return err
	}
	candidateDigest := sha256.New()
	candidateWriter := io.MultiWriter(temp, candidateDigest)
	if source != nil {
		_, copyErr := io.Copy(candidateWriter, source)
		closeErr := source.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
	}
	encoder := json.NewEncoder(candidateWriter)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	candidateInfo, err := temp.Stat()
	if err != nil {
		return err
	}
	beforeDigest, beforeSize := currentFrontier.SHA256, currentFrontier.Size
	afterDigest, afterSize := hex.EncodeToString(candidateDigest.Sum(nil)), candidateInfo.Size()
	journal, err := newAtomicBundleJournal(threadID, source != nil, beforeDigest, beforeSize, afterDigest, afterSize)
	if err != nil {
		return err
	}
	// The candidate and journal temp have no recovery authority until the
	// journal rename is directory-synced. Flush those independent files in
	// parallel, then retain the original candidate -> journal -> target commit
	// order and every crash cut below.
	candidateSynced := make(chan error, 1)
	go func() { candidateSynced <- temp.Sync() }()
	journalErr := s.prepareAtomicBundleJournal(dir, journal)
	candidateErr := <-candidateSynced
	if err := errors.Join(candidateErr, journalErr); err != nil {
		cleanup = false
		return err
	}
	if err := temp.Close(); err != nil {
		cleanup = false
		return err
	}
	closed = true
	if err := s.runAtomicAppendCut(atomicBundleCutCandidateSynced, false); err != nil {
		cleanup = false
		return err
	}
	if err := s.runAtomicAppendCut(atomicBundleCutJournalTempSynced, false); err != nil {
		cleanup = false
		return err
	}
	if err := s.commitAtomicBundleJournal(dir); err != nil {
		cleanup = false
		return err
	}
	if err := s.runAtomicAppendCut(atomicBundleCutJournalCommitted, false); err != nil {
		cleanup = false
		return err
	}
	if err := s.replaceFile(tempPath, path); err != nil {
		cleanup = false
		committed, recoveryErr := s.recoverAtomicBundle(threadID)
		if recoveryErr != nil {
			return errors.Join(err, recoveryErr)
		}
		if committed {
			return &AtomicAppendError{Committed: true, Cause: err}
		}
		return err
	}
	cleanup = false
	if err := s.runAtomicAppendCut(atomicBundleCutTargetReplaced, true); err != nil {
		return err
	}
	if err := s.syncParentFolder(dir); err != nil {
		return &AtomicAppendError{Committed: true, Cause: err}
	}
	if err := s.runAtomicAppendCut(atomicBundleCutDirectorySynced, true); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, atomicBundleJournalName)); err != nil {
		return &AtomicAppendError{Committed: true, Cause: err}
	}
	if err := s.runAtomicAppendCut(atomicBundleCutJournalRemoved, true); err != nil {
		return err
	}
	// The target rename was already directory-synced above. Journal removal is
	// cleanup-only: if its directory entry survives a crash, recovery validates
	// the exact after-frontier and removes the stale journal. A second full media
	// flush here adds latency without strengthening the committed frontier.
	return nil
}

func digestOpenEventLogSource(source *os.File) (string, int64, error) {
	if source == nil {
		return digestBytes(nil), 0, nil
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, source)
	if err != nil {
		return "", 0, err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func sameEventLogFrontierV1(left, right EventLogFrontierV1) bool {
	return left.Exists == right.Exists && left.SHA256 == right.SHA256 && left.Size == right.Size
}

func (s *Store) AppendRawLine(threadID string, line string) (int, bool, error) {
	if s.restartPreserved.OwnsThread(threadID) {
		return 0, false, ErrRestartPreserved
	}
	if err := validateExactEventThreadID(threadID); err != nil {
		return 0, false, err
	}
	raw := []byte(strings.TrimSpace(line))
	if len(raw) == 0 {
		return 0, false, errors.New("raw event line is empty")
	}
	if domainrestrictedevidence.ValidateCanonicalText(string(raw)) != nil {
		return 0, false, errors.New("raw event line contains restricted evidence")
	}
	if domainprivacy.ValidateOrdinaryText(string(raw)) != nil {
		return 0, false, errors.New("raw event line contains restricted PII")
	}
	event, err := domainjsonstrict.DecodeObject(raw, domainjsonstrict.Options{MaxBytes: maxEventJSONLLineBytesV1})
	if err != nil || event == nil {
		return 0, false, errors.New("raw event line is invalid")
	}
	if contracts.StringField(event, "threadId") != threadID {
		return 0, false, errors.New("raw event thread identity is invalid")
	}
	if err := domainevent.ValidatePublicRecord(event); err != nil {
		return 0, false, err
	}
	if err := domaineventlifecycle.ValidateNewEventLifecycleV1(event); err != nil {
		return 0, false, err
	}
	seq, ok := contracts.NumericSeq(event["seq"])
	if !ok {
		return 0, false, errors.New("raw event sequence is invalid")
	}
	if err := s.AppendEventsAtomic(threadID, []map[string]any{event}); err != nil {
		return 0, false, err
	}
	return seq, ok, nil
}

func (s *Store) LoadSince(threadID string, afterSeq int) (LoadResult, error) {
	return s.LoadSinceContext(context.Background(), threadID, afterSeq)
}

func (s *Store) LoadSinceContext(ctx context.Context, threadID string, afterSeq int) (LoadResult, error) {
	result, _, err := s.LoadSinceWithFrontierContext(ctx, threadID, afterSeq)
	return result, err
}

func (s *Store) LoadSinceWithFrontier(threadID string, afterSeq int) (LoadResult, EventLogFrontierV1, error) {
	return s.LoadSinceWithFrontierContext(context.Background(), threadID, afterSeq)
}

func (s *Store) LoadSinceWithFrontierContext(ctx context.Context, threadID string, afterSeq int) (LoadResult, EventLogFrontierV1, error) {
	if s.restartPreserved.OwnsThread(threadID) {
		return LoadResult{}, EventLogFrontierV1{}, ErrRestartPreserved
	}
	if ctx == nil {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log replay context is required")
	}
	if err := ctx.Err(); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	if err := validateExactEventThreadID(threadID); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	unlock := s.lockThreadTransaction(threadID)
	defer unlock()
	return s.loadSinceWithFrontierLocked(ctx, threadID, afterSeq, true)
}

// ObserveSinceWithFrontier reads only the committed log. A pending atomic
// transaction is not independent publication proof and is left byte-for-byte
// intact for its recovery owner. Ordinary LoadSince retains its recovery role.
func (s *Store) ObserveSinceWithFrontier(ctx context.Context, threadID string, afterSeq int) (LoadResult, EventLogFrontierV1, error) {
	if s == nil || ctx == nil {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	if err := validateExactEventThreadID(threadID); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	unlock := s.lockThreadTransaction(threadID)
	defer unlock()
	directories := []string{s.root, filepath.Join(s.root, "threads"), s.ThreadDir(threadID)}
	identities := make([]os.FileInfo, len(directories))
	for index, path := range directories {
		info, exists, err := validateEventThreadDirectory(path)
		if err != nil || !exists {
			return LoadResult{}, EventLogFrontierV1{}, errors.Join(err, errors.New("event log observation directory is unavailable"))
		}
		identities[index] = info
	}
	initial, initialErr := os.Lstat(s.EventsPath(threadID))
	if initialErr != nil && !errors.Is(initialErr, os.ErrNotExist) {
		return LoadResult{}, EventLogFrontierV1{}, initialErr
	}
	if initialErr == nil && (initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular()) {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log observation file is invalid")
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for index, path := range directories {
			info, exists, err := validateEventThreadDirectory(path)
			if err != nil || !exists || !os.SameFile(identities[index], info) {
				return errors.Join(err, errors.New("event log observation directory changed"))
			}
		}
		current, err := os.Lstat(s.EventsPath(threadID))
		if initialErr != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return errors.Join(err, errors.New("event log observation file appeared"))
			}
		} else if err != nil || !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(initial, current) || initial.Size() != current.Size() || !initial.ModTime().Equal(current.ModTime()) {
			return errors.Join(err, errors.New("event log observation file changed"))
		}
		entries, err := os.ReadDir(s.ThreadDir(threadID))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".events-bundle-") {
				return errors.New("event log observation requires a settled atomic transaction")
			}
		}
		return nil
	}
	if err := check(); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	result, frontier, err := s.loadSinceWithFrontierLocked(ctx, threadID, afterSeq, false)
	if err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	_, refreshed, err := s.loadSinceWithFrontierLocked(ctx, threadID, afterSeq, false)
	if err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	if refreshed != frontier {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log observation bytes changed")
	}
	if err := check(); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	return result, frontier, nil
}

func (s *Store) loadSinceWithFrontierLocked(ctx context.Context, threadID string, afterSeq int, recoverBundle bool) (LoadResult, EventLogFrontierV1, error) {
	threadInfo, exists, err := validateEventThreadDirectory(s.ThreadDir(threadID))
	if err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	if !exists {
		return LoadResult{Events: []map[string]any{}, Diagnostics: []JSONLDiagnostic{}}, EventLogFrontierV1{
			Exists: false, SHA256: digestBytes(nil), Size: 0,
		}, nil
	}
	if recoverBundle {
		if _, err := s.recoverAtomicBundle(threadID); err != nil {
			return LoadResult{}, EventLogFrontierV1{}, err
		}
	}
	threadInfo, exists, err = validateEventThreadDirectory(s.ThreadDir(threadID))
	if err != nil || !exists {
		return LoadResult{}, EventLogFrontierV1{}, errors.Join(err, errors.New("event log thread directory changed during recovery"))
	}
	path := s.EventsPath(threadID)
	pathInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return LoadResult{Events: []map[string]any{}, Diagnostics: []JSONLDiagnostic{}}, EventLogFrontierV1{
			Exists: false, SHA256: digestBytes(nil), Size: 0,
		}, nil
	}
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return LoadResult{}, EventLogFrontierV1{}, errors.Join(err, errors.New("event log is not a regular file"))
	}
	file, err := os.Open(path)
	if err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return LoadResult{}, EventLogFrontierV1{}, errors.Join(err, errors.New("event log is not a regular file"))
	}
	return loadOpenEventLogV1(ctx, threadID, path, threadInfo, file, openedInfo, afterSeq)
}

func normalizeReplaySequence(event map[string]any) (int, bool) {
	parsed, ok := exactEventSequenceV1(event["seq"])
	if !ok {
		return 0, false
	}
	event["seq"] = float64(parsed)
	return parsed, true
}

func sameEventJSON(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftJSON, rightJSON)
}

func (s *Store) ReadFromSeq(threadID string, afterSeq domainevent.Seq) ([]domainevent.RuntimeEvent, []JSONLDiagnostic, error) {
	result, err := s.LoadSince(threadID, int(afterSeq))
	if err != nil {
		return nil, nil, err
	}
	return result.RuntimeEvents(), result.Diagnostics, nil
}

func (s *Store) ReadAggregate(threadID string) ([]domainevent.RuntimeEvent, []JSONLDiagnostic, error) {
	return s.ReadFromSeq(threadID, domainevent.Seq(-1))
}

func (s *Store) Fork(sourceThreadID string, targetThreadID string, throughSeq domainevent.Seq) (int, error) {
	if s.restartPreserved.OwnsThread(sourceThreadID) || s.restartPreserved.OwnsThread(targetThreadID) {
		return 0, ErrRestartPreserved
	}
	if err := validateExactEventThreadID(sourceThreadID); err != nil {
		return 0, errors.New("source threadId is invalid")
	}
	if err := validateExactEventThreadID(targetThreadID); err != nil {
		return 0, errors.New("target threadId is invalid")
	}
	events, _, err := s.ReadAggregate(sourceThreadID)
	if err != nil {
		return 0, err
	}
	nextSeq := domainevent.Seq(1)
	copied := 0
	for _, event := range events {
		if throughSeq > 0 && event.Seq > throughSeq {
			continue
		}
		payload := contracts.CloneMap(event.Payload)
		payload["threadId"] = targetThreadID
		payload["sourceThreadId"] = sourceThreadID
		payload["sourceSeq"] = float64(event.Seq)
		event.Payload = payload
		event.Seq = nextSeq
		event.ThreadID = targetThreadID
		if err := s.Append(event); err != nil {
			return copied, err
		}
		nextSeq++
		copied++
	}
	return copied, nil
}

func (s *Store) Archive(threadID string, seq domainevent.Seq, reason string) error {
	return s.Append(domainevent.RuntimeEvent{
		Seq:      seq,
		Kind:     "thread_archived",
		ThreadID: threadID,
		Payload: map[string]any{
			"reason": strings.TrimSpace(reason),
		},
	})
}

func (s *Store) Compact(threadID string, seq domainevent.Seq, summary string) error {
	return s.Append(domainevent.RuntimeEvent{
		Seq:      seq,
		Kind:     "eventlog_compacted",
		ThreadID: threadID,
		Payload: map[string]any{
			"summary": strings.TrimSpace(summary),
		},
	})
}

func (s *Store) Search(query string, includeArchived bool) ([]ThreadSearchResult, []JSONLDiagnostic, error) {
	threadsRoot := filepath.Join(s.root, "threads")
	rootInfo, err := os.Lstat(threadsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []ThreadSearchResult{}, []JSONLDiagnostic{}, nil
	}
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, nil, errors.Join(err, errors.New("event log threads root is unsafe"))
	}
	entries, err := os.ReadDir(threadsRoot)
	if err != nil {
		return nil, nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	results := []ThreadSearchResult{}
	diagnostics := []JSONLDiagnostic{}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() && validateExactEventThreadID(entry.Name()) != nil {
			return nil, nil, errors.New("event log inventory contains an unsafe thread directory")
		}
		if !entry.IsDir() {
			continue
		}
		events, eventDiagnostics, err := s.ReadAggregate(entry.Name())
		if err != nil {
			return nil, nil, err
		}
		if len(eventDiagnostics) != 0 {
			return nil, nil, errors.New("event log search contains invalid records")
		}
		diagnostics = append(diagnostics, eventDiagnostics...)
		if len(events) == 0 {
			continue
		}
		result, matched := projectThreadSearchResult(entry.Name(), events, needle)
		if result.ThreadID == "" {
			continue
		}
		if result.Archived && !includeArchived {
			continue
		}
		if needle != "" && !matched {
			continue
		}
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].HighestSeq == results[j].HighestSeq {
			return results[i].ThreadID > results[j].ThreadID
		}
		return results[i].HighestSeq > results[j].HighestSeq
	})
	return results, diagnostics, nil
}

func (s *Store) HighestSeq(threadID string) (int, error) {
	result, err := s.LoadSince(threadID, -1)
	if err != nil {
		return 0, err
	}
	maxSeq := 0
	for _, event := range result.Events {
		if seq, ok := contracts.NumericSeq(event["seq"]); ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq, nil
}

func projectThreadSearchResult(fallbackThreadID string, events []domainevent.RuntimeEvent, needle string) (ThreadSearchResult, bool) {
	result := ThreadSearchResult{
		ThreadID: strings.TrimSpace(fallbackThreadID),
		Status:   "idle",
	}
	matched := needle == ""
	for _, event := range events {
		if event.ThreadID != "" {
			result.ThreadID = event.ThreadID
		}
		if event.Seq > result.HighestSeq {
			result.HighestSeq = event.Seq
		}
		title := strings.TrimSpace(contracts.StringField(event.Payload, "title"))
		status := strings.TrimSpace(contracts.StringField(event.Payload, "status"))
		switch event.Kind {
		case "thread_created", "thread_updated":
			if title != "" {
				result.Title = title
			}
			if status != "" {
				result.Status = status
			}
		case "thread_archived":
			result.Status = "archived"
			result.Archived = true
		}
		if status == "archived" {
			result.Archived = true
		}
		if !matched && valueContainsSearch(event.Payload, needle) {
			matched = true
			result.Match = event.Kind
		}
	}
	if result.Title == "" {
		result.Title = result.ThreadID
	}
	if result.Status == "archived" {
		result.Archived = true
	}
	return result, matched
}

func (s *Store) NewlineTerminated(threadID string) bool {
	if validateExactEventThreadID(threadID) != nil {
		return false
	}
	unlock := s.lockThreadTransaction(threadID)
	defer unlock()
	data, err := os.ReadFile(s.EventsPath(threadID))
	return err == nil && len(data) > 0 && data[len(data)-1] == '\n'
}

func RuntimeEventFromMap(input map[string]any) domainevent.RuntimeEvent {
	payload := contracts.CloneMap(input)
	seq, _ := contracts.NumericSeq(input["seq"])
	return domainevent.RuntimeEvent{
		Seq:       domainevent.Seq(seq),
		Kind:      contracts.StringField(input, "kind"),
		ThreadID:  contracts.StringField(input, "threadId"),
		TurnID:    contracts.StringField(input, "turnId"),
		Timestamp: contracts.StringField(input, "timestamp"),
		Terminal:  boolField(input, "terminal"),
		Payload:   payload,
	}
}

func RuntimeEventToMap(input domainevent.RuntimeEvent) (map[string]any, string, error) {
	payload := contracts.CloneMap(input.Payload)
	if payload == nil {
		payload = map[string]any{}
	}
	threadID := strings.TrimSpace(input.ThreadID)
	if threadID == "" {
		threadID = strings.TrimSpace(contracts.StringField(payload, "threadId"))
	}
	if threadID == "" {
		return nil, "", errors.New("threadId is required")
	}
	if input.Seq <= 0 {
		return nil, "", errors.New("seq is required")
	}
	payload["seq"] = float64(input.Seq)
	payload["threadId"] = threadID
	if input.Kind != "" {
		payload["kind"] = input.Kind
	}
	if input.TurnID != "" {
		payload["turnId"] = input.TurnID
	}
	if input.Timestamp != "" {
		payload["timestamp"] = input.Timestamp
	}
	if input.Terminal {
		payload["terminal"] = input.Terminal
	}
	return payload, threadID, nil
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func valueContainsSearch(value any, needle string) bool {
	if needle == "" {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.Contains(strings.ToLower(typed), needle)
	case []any:
		for _, item := range typed {
			if valueContainsSearch(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if valueContainsSearch(item, needle) {
				return true
			}
		}
	}
	return false
}
