package eventlog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domaineventlifecycle "analytix.local/runtime-go/internal/domain/eventlifecycle"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	atomicSingleJournalVersion = 1
	atomicSingleJournalPrefix  = ".events-bundle-single-journal-v1-"
	atomicSingleJournalSuffix  = ".json"
	maxAtomicSingleEventBytes  = 32 * 1024

	atomicSingleCutJournalFileSynced = "single_journal_file_synced"
	atomicSingleCutJournalCommitted  = "single_journal_committed"
	atomicSingleCutTargetWritten     = "single_target_written"
	atomicSingleCutTargetSynced      = "single_target_synced"
	atomicSingleCutJournalRemoved    = "single_journal_removed"
)

var errAtomicSingleFallback = errors.New("single event requires whole-log atomic replacement")

type atomicSingleJournalV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	ThreadID      string `json:"threadId"`
	BeforeSHA256  string `json:"beforeSha256"`
	BeforeSize    int64  `json:"beforeSize"`
	AppendBase64  string `json:"appendBase64"`
	AppendSHA256  string `json:"appendSha256"`
	JournalDigest string `json:"journalDigest"`
}

func newAtomicSingleJournal(threadID string, frontier EventLogFrontierV1, line []byte) (atomicSingleJournalV1, error) {
	journal := atomicSingleJournalV1{
		SchemaVersion: atomicSingleJournalVersion,
		ThreadID:      strings.TrimSpace(threadID),
		BeforeSHA256:  frontier.SHA256,
		BeforeSize:    frontier.Size,
		AppendBase64:  base64.RawStdEncoding.EncodeToString(line),
		AppendSHA256:  digestBytes(line),
	}
	journal.JournalDigest = atomicSingleJournalDigest(journal)
	if _, err := validateAtomicSingleJournal(journal); err != nil {
		return atomicSingleJournalV1{}, err
	}
	return journal, nil
}

func validateAtomicSingleJournal(journal atomicSingleJournalV1) ([]byte, error) {
	line, decodeErr := base64.RawStdEncoding.DecodeString(journal.AppendBase64)
	if decodeErr != nil || journal.SchemaVersion != atomicSingleJournalVersion ||
		validateExactEventThreadID(journal.ThreadID) != nil || journal.BeforeSize < 0 ||
		!validDigest(journal.BeforeSHA256) || len(line) == 0 || len(line) > maxAtomicSingleEventBytes ||
		line[len(line)-1] != '\n' || digestBytes(line) != journal.AppendSHA256 ||
		!validDigest(journal.JournalDigest) || atomicSingleJournalDigest(journal) != journal.JournalDigest {
		return nil, errors.New("atomic single-event journal is invalid")
	}
	event, err := domainjsonstrict.DecodeObject(line[:len(line)-1], domainjsonstrict.Options{
		MaxBytes: maxAtomicSingleEventBytes,
	})
	if err != nil || event == nil || stringValue(event["threadId"]) != journal.ThreadID ||
		domainevent.ValidatePublicRecord(event) != nil || domaineventlifecycle.ValidateNewEventLifecycleV1(event) != nil {
		return nil, errors.New("atomic single-event journal is invalid")
	}
	return line, nil
}

func atomicSingleJournalDigest(journal atomicSingleJournalV1) string {
	journal.JournalDigest = ""
	body, _ := json.Marshal(journal)
	return digestBytes(body)
}

func (s *Store) writeAtomicSingleJournal(dir string, journal atomicSingleJournalV1) (string, error) {
	body, err := json.Marshal(journal)
	if err != nil {
		return "", err
	}
	name := atomicSingleJournalFileName(journal.BeforeSize, journal.BeforeSHA256, digestBytes(body))
	journalPath := filepath.Join(dir, name)
	file, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := file.Write(body); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := s.runAtomicAppendCut(atomicSingleCutJournalFileSynced, false); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	closed = true
	if err := s.syncParentFolder(dir); err != nil {
		return "", err
	}
	return journalPath, nil
}

func (s *Store) appendSingleEventAtomic(threadID string, event map[string]any) error {
	if s.restartPreserved.OwnsThread(threadID) {
		return ErrRestartPreserved
	}
	if validateExactEventThreadID(threadID) != nil || stringValue(event["threadId"]) != threadID {
		return errors.New("atomic single event threadId is invalid")
	}
	sequence, ok := exactEventSequenceV1(event["seq"])
	if !ok {
		return errors.New("atomic single event sequence is invalid")
	}
	if err := domainevent.ValidatePublicRecord(event); err != nil {
		return err
	}
	if err := domaineventlifecycle.ValidateNewEventLifecycleV1(event); err != nil {
		return err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if len(line) > maxAtomicSingleEventBytes {
		return errAtomicSingleFallback
	}

	unlock := s.lockThreadTransaction(threadID)
	defer unlock()
	dir := s.ThreadDir(threadID)
	if _, exists, err := validateEventThreadDirectory(dir); err != nil {
		return err
	} else if !exists {
		return errAtomicSingleFallback
	}
	if _, err := s.recoverAtomicBundle(threadID); err != nil {
		return err
	}
	current, frontier, err := s.loadSinceWithFrontierLocked(context.Background(), threadID, -1, false)
	if err != nil {
		return err
	}
	if len(current.Diagnostics) != 0 {
		return errors.New("event log requires semantic content migration before append")
	}
	if !frontier.Exists {
		return errAtomicSingleFallback
	}
	nextSequence := 0
	if len(current.Events) > 0 {
		last, valid := exactEventSequenceV1(current.Events[len(current.Events)-1]["seq"])
		if !valid {
			return errors.New("event log sequence frontier is invalid")
		}
		nextSequence = last + 1
	}
	if sequence != nextSequence {
		return errors.New("atomic single event does not extend the exact sequence frontier")
	}

	journal, err := newAtomicSingleJournal(threadID, frontier, line)
	if err != nil {
		return err
	}
	journalPath, err := s.writeAtomicSingleJournal(dir, journal)
	if err != nil {
		return err
	}
	if err := s.runAtomicAppendCut(atomicSingleCutJournalCommitted, false); err != nil {
		return err
	}
	path := s.EventsPath(threadID)
	initial, err := os.Lstat(path)
	if err != nil || initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() {
		return errors.Join(err, errors.New("event log is not a regular file"))
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(initial, opened) || opened.Size() != frontier.Size {
		_ = file.Close()
		return errors.Join(statErr, ErrEventLogFrontierChanged)
	}
	digest, size, digestErr := digestOpenEventLogSource(file)
	if digestErr != nil || digest != frontier.SHA256 || size != frontier.Size {
		_ = file.Close()
		return errors.Join(digestErr, ErrEventLogFrontierChanged)
	}
	currentInfo, currentErr := os.Lstat(path)
	if currentErr != nil || !os.SameFile(opened, currentInfo) {
		_ = file.Close()
		return errors.Join(currentErr, ErrEventLogFrontierChanged)
	}
	written, writeErr := file.Write(line)
	if writeErr != nil || written != len(line) {
		closeErr := file.Close()
		committed, recoveryErr := s.recoverAtomicSingle(threadID)
		cause := errors.Join(writeErr, closeErr, recoveryErr)
		if written != len(line) {
			cause = errors.Join(cause, io.ErrShortWrite)
		}
		if committed {
			return &AtomicAppendError{Committed: true, Cause: cause}
		}
		return cause
	}
	if err := s.runAtomicAppendCut(atomicSingleCutTargetWritten, true); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return &AtomicAppendError{Committed: true, Cause: err}
	}
	if err := file.Close(); err != nil {
		return &AtomicAppendError{Committed: true, Cause: err}
	}
	after, afterErr := os.Lstat(path)
	if afterErr != nil || !os.SameFile(opened, after) || after.Size() != frontier.Size+int64(len(line)) {
		return &AtomicAppendError{Committed: true, Cause: errors.Join(afterErr, ErrEventLogFrontierChanged)}
	}
	if err := s.runAtomicAppendCut(atomicSingleCutTargetSynced, true); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil {
		return &AtomicAppendError{Committed: true, Cause: err}
	}
	if err := s.runAtomicAppendCut(atomicSingleCutJournalRemoved, true); err != nil {
		return err
	}
	return nil
}

func (s *Store) recoverAtomicSingle(threadID string) (bool, error) {
	if s.restartPreserved.OwnsThread(threadID) {
		return false, ErrRestartPreserved
	}
	dir := s.ThreadDir(threadID)
	names, err := atomicSingleJournalResidueNames(dir)
	if err != nil {
		return false, err
	}
	if len(names) == 0 {
		return false, nil
	}
	if len(names) != 1 {
		return false, errors.New("multiple atomic single-event journals require fail-closed recovery")
	}
	journalName := names[0]
	beforeSize, beforeDigest, bodyDigest, validName := parseAtomicSingleJournalFileName(journalName)
	if !validName {
		return false, errors.New("atomic single-event journal name is corrupt")
	}
	journalPath := filepath.Join(dir, journalName)
	if _, err := safeAtomicResidueExists(journalPath); err != nil {
		return false, err
	}
	body, err := os.ReadFile(journalPath)
	if err != nil {
		return false, errors.New("atomic single-event journal cannot be read")
	}
	if len(body) == 0 || len(body) > 64*1024 || digestBytes(body) != bodyDigest {
		matches, matchErr := eventFileMatchesFrontier(s.EventsPath(threadID), beforeSize, beforeDigest)
		if matchErr != nil || !matches {
			return false, errors.Join(matchErr, errors.New("incomplete atomic single-event journal does not match its before frontier"))
		}
		if err := removeIfExists(journalPath); err != nil {
			return false, err
		}
		return false, s.syncParentFolder(dir)
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: 64 * 1024}); err != nil {
		return false, errors.New("atomic single-event journal is corrupt")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var journal atomicSingleJournalV1
	if err := decoder.Decode(&journal); err != nil || decoder.Decode(&struct{}{}) != io.EOF || journal.ThreadID != threadID {
		return false, errors.New("atomic single-event journal is corrupt")
	}
	if journal.BeforeSize != beforeSize || journal.BeforeSHA256 != beforeDigest {
		return false, errors.New("atomic single-event journal name binding is corrupt")
	}
	line, err := validateAtomicSingleJournal(journal)
	if err != nil {
		return false, errors.New("atomic single-event journal is corrupt")
	}
	path := s.EventsPath(threadID)
	initial, err := os.Lstat(path)
	if err != nil || initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() {
		return false, errors.Join(err, errors.New("atomic single-event target is unsafe"))
	}
	if initial.Size() < journal.BeforeSize || initial.Size() > journal.BeforeSize+int64(len(line)) {
		return false, errors.New("atomic single-event target is outside its journal frontier")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(initial, opened) {
		_ = file.Close()
		return false, errors.Join(statErr, errors.New("atomic single-event target identity changed"))
	}
	hasher := sha256.New()
	if _, err := io.CopyN(hasher, file, journal.BeforeSize); err != nil || hex.EncodeToString(hasher.Sum(nil)) != journal.BeforeSHA256 {
		_ = file.Close()
		return false, errors.Join(err, errors.New("atomic single-event target before frontier changed"))
	}
	tail, err := io.ReadAll(io.LimitReader(file, int64(len(line))+1))
	if err != nil || !bytes.HasPrefix(line, tail) {
		_ = file.Close()
		return false, errors.Join(err, errors.New("atomic single-event target matches no recoverable frontier"))
	}
	committed := len(tail) == len(line)
	if !committed && len(tail) > 0 {
		if err := file.Truncate(journal.BeforeSize); err != nil {
			_ = file.Close()
			return false, err
		}
	}
	// A full tail may have reached the page cache before the previous process
	// stopped at target_written. Make the accepted frontier durable before
	// deleting its only recovery record. The same sync also commits a rollback
	// of a torn tail.
	if err := s.syncRecoveredFile(file); err != nil {
		_ = file.Close()
		return committed, err
	}
	closeErr := file.Close()
	current, currentErr := os.Lstat(path)
	if closeErr != nil || currentErr != nil || !os.SameFile(opened, current) {
		return false, errors.Join(closeErr, currentErr, errors.New("atomic single-event target identity changed during recovery"))
	}
	if err := removeIfExists(journalPath); err != nil {
		return committed, err
	}
	if err := s.syncParentFolder(dir); err != nil {
		return committed, err
	}
	return committed, nil
}

func atomicSingleJournalFileName(beforeSize int64, beforeDigest, bodyDigest string) string {
	return fmt.Sprintf("%s%d-%s-%s%s", atomicSingleJournalPrefix, beforeSize, beforeDigest, bodyDigest, atomicSingleJournalSuffix)
}

func parseAtomicSingleJournalFileName(name string) (int64, string, string, bool) {
	if !strings.HasPrefix(name, atomicSingleJournalPrefix) || !strings.HasSuffix(name, atomicSingleJournalSuffix) {
		return 0, "", "", false
	}
	bound := strings.TrimSuffix(strings.TrimPrefix(name, atomicSingleJournalPrefix), atomicSingleJournalSuffix)
	parts := strings.Split(bound, "-")
	if len(parts) != 3 || !validDigest(parts[1]) || !validDigest(parts[2]) {
		return 0, "", "", false
	}
	size, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || size < 0 || strconv.FormatInt(size, 10) != parts[0] {
		return 0, "", "", false
	}
	return size, parts[1], parts[2], true
}

func atomicSingleJournalResidueNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), atomicSingleJournalPrefix) {
			if _, _, _, valid := parseAtomicSingleJournalFileName(entry.Name()); !valid {
				return nil, errors.New("unknown atomic single-event journal requires fail-closed recovery")
			}
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func eventFileMatchesFrontier(path string, expectedSize int64, expectedDigest string) (bool, error) {
	initial, err := os.Lstat(path)
	if err != nil || initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() || initial.Size() != expectedSize {
		return false, errors.Join(err, errors.New("atomic single-event before frontier is unavailable"))
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	digest, size, digestErr := digestOpenEventLogSource(file)
	opened, statErr := file.Stat()
	closeErr := file.Close()
	current, currentErr := os.Lstat(path)
	if digestErr != nil || statErr != nil || closeErr != nil || currentErr != nil ||
		!os.SameFile(initial, opened) || !os.SameFile(opened, current) {
		return false, errors.Join(digestErr, statErr, closeErr, currentErr, errors.New("atomic single-event before frontier identity changed"))
	}
	return size == expectedSize && digest == expectedDigest, nil
}
