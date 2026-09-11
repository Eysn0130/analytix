package eventlog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	atomicBundleJournalVersion = 1
	atomicBundleCandidateName  = ".events-bundle-v1.tmp"
	atomicBundleJournalName    = ".events-bundle-journal-v1.json"
	atomicBundleJournalTemp    = ".events-bundle-journal-v1.tmp"

	atomicBundleCutCandidateSynced   = "candidate_synced"
	atomicBundleCutJournalTempSynced = "journal_temp_synced"
	atomicBundleCutJournalCommitted  = "journal_committed"
	atomicBundleCutTargetReplaced    = "target_replaced"
	atomicBundleCutDirectorySynced   = "directory_synced"
	atomicBundleCutJournalRemoved    = "journal_removed"
)

type atomicBundleJournalV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	ThreadID      string `json:"threadId"`
	BeforeExists  bool   `json:"beforeExists"`
	BeforeSHA256  string `json:"beforeSha256"`
	BeforeSize    int64  `json:"beforeSize"`
	AfterSHA256   string `json:"afterSha256"`
	AfterSize     int64  `json:"afterSize"`
	Candidate     string `json:"candidate"`
	JournalDigest string `json:"journalDigest"`
}

func newAtomicBundleJournal(threadID string, beforeExists bool, beforeDigest string, beforeSize int64, afterDigest string, afterSize int64) (atomicBundleJournalV1, error) {
	journal := atomicBundleJournalV1{
		SchemaVersion: atomicBundleJournalVersion, ThreadID: strings.TrimSpace(threadID), BeforeExists: beforeExists,
		BeforeSHA256: beforeDigest, BeforeSize: beforeSize, AfterSHA256: afterDigest, AfterSize: afterSize,
		Candidate: atomicBundleCandidateName,
	}
	journal.JournalDigest = atomicBundleJournalDigest(journal)
	if err := validateAtomicBundleJournal(journal); err != nil {
		return atomicBundleJournalV1{}, err
	}
	return journal, nil
}

func validateAtomicBundleJournal(journal atomicBundleJournalV1) error {
	if journal.SchemaVersion != atomicBundleJournalVersion || strings.TrimSpace(journal.ThreadID) == "" ||
		journal.Candidate != atomicBundleCandidateName || !validDigest(journal.BeforeSHA256) || !validDigest(journal.AfterSHA256) ||
		journal.BeforeSize < 0 || journal.AfterSize <= 0 || journal.AfterSize < journal.BeforeSize ||
		(!journal.BeforeExists && (journal.BeforeSize != 0 || journal.BeforeSHA256 != digestBytes(nil))) ||
		!validDigest(journal.JournalDigest) || atomicBundleJournalDigest(journal) != journal.JournalDigest {
		return errors.New("atomic event bundle journal is invalid")
	}
	return nil
}

func atomicBundleJournalDigest(journal atomicBundleJournalV1) string {
	journal.JournalDigest = ""
	body, _ := json.Marshal(journal)
	return digestBytes(body)
}

func (s *Store) prepareAtomicBundleJournal(dir string, journal atomicBundleJournalV1) error {
	body, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	tempPath := filepath.Join(dir, atomicBundleJournalTemp)
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	return nil
}

func (s *Store) commitAtomicBundleJournal(dir string) error {
	if err := os.Rename(filepath.Join(dir, atomicBundleJournalTemp), filepath.Join(dir, atomicBundleJournalName)); err != nil {
		return err
	}
	return s.syncParentFolder(dir)
}

func (s *Store) runAtomicAppendCut(cut string, committed bool) error {
	if s == nil || s.atomicAppendCut == nil {
		return nil
	}
	err := s.atomicAppendCut(cut)
	if err != nil && committed {
		return &AtomicAppendError{Committed: true, Cause: err}
	}
	return err
}

// recoverAtomicBundle rolls back a prepared candidate or accepts an exact
// post-replace target. It never infers a commit from a temporary file alone.
func (s *Store) recoverAtomicBundle(threadID string) (bool, error) {
	if s.restartPreserved.OwnsThread(threadID) {
		return false, ErrRestartPreserved
	}
	dir := s.ThreadDir(threadID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	known := map[string]bool{
		atomicBundleCandidateName: true, atomicBundleJournalName: true, atomicBundleJournalTemp: true,
	}
	for _, entry := range entries {
		_, _, _, singleJournal := parseAtomicSingleJournalFileName(entry.Name())
		if strings.HasPrefix(entry.Name(), ".events-bundle-") && !known[entry.Name()] && !singleJournal {
			return false, errors.New("pending atomic event bundle requires fail-closed recovery")
		}
	}
	singleResidue := false
	bundleResidue := false
	for _, entry := range entries {
		switch entry.Name() {
		case atomicBundleCandidateName, atomicBundleJournalName, atomicBundleJournalTemp:
			bundleResidue = true
		default:
			_, _, _, singleJournal := parseAtomicSingleJournalFileName(entry.Name())
			if singleJournal {
				singleResidue = true
			}
		}
	}
	if singleResidue && bundleResidue {
		return false, errors.New("mixed atomic event transactions require fail-closed recovery")
	}
	if singleResidue {
		return s.recoverAtomicSingle(threadID)
	}
	journalPath := filepath.Join(dir, atomicBundleJournalName)
	journalTempPath := filepath.Join(dir, atomicBundleJournalTemp)
	candidatePath := filepath.Join(dir, atomicBundleCandidateName)
	journalExists, err := safeAtomicResidueExists(journalPath)
	if err != nil {
		return false, err
	}
	journalTempExists, err := safeAtomicResidueExists(journalTempPath)
	if err != nil {
		return false, err
	}
	candidateExists, err := safeAtomicResidueExists(candidatePath)
	if err != nil {
		return false, err
	}
	if !journalExists {
		if !journalTempExists && !candidateExists {
			return false, nil
		}
		for _, path := range []string{candidatePath, journalTempPath} {
			if err := removeIfExists(path); err != nil {
				return false, err
			}
		}
		return false, s.syncParentFolder(dir)
	}
	body, err := os.ReadFile(journalPath)
	if err != nil || len(body) == 0 || len(body) > 64*1024 {
		return false, errors.New("atomic event bundle journal cannot be read")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true,
		MaxBytes:      64 * 1024,
	}); err != nil {
		return false, errors.New("atomic event bundle journal is corrupt")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var journal atomicBundleJournalV1
	if err := decoder.Decode(&journal); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		validateAtomicBundleJournal(journal) != nil || journal.ThreadID != threadID {
		return false, errors.New("atomic event bundle journal is corrupt")
	}
	targetDigest, targetSize, targetErr := digestEventFile(s.EventsPath(threadID))
	targetMissing := errors.Is(targetErr, os.ErrNotExist)
	if targetErr != nil && !targetMissing {
		return false, targetErr
	}
	targetAfter := !targetMissing && targetDigest == journal.AfterSHA256 && targetSize == journal.AfterSize
	targetBefore := targetMissing && !journal.BeforeExists || !targetMissing && journal.BeforeExists &&
		targetDigest == journal.BeforeSHA256 && targetSize == journal.BeforeSize
	if !targetAfter && !targetBefore {
		return false, errors.New("atomic event bundle target matches neither journal frontier")
	}
	if candidateExists {
		candidateDigest, candidateSize, err := digestEventFile(candidatePath)
		if err != nil || candidateDigest != journal.AfterSHA256 || candidateSize != journal.AfterSize {
			return false, errors.New("atomic event bundle candidate does not match its journal")
		}
	}
	if targetBefore && !candidateExists {
		// A target at the before frontier is an uncommitted rollback even if the
		// candidate directory entry was not durable.
	}
	for _, path := range []string{candidatePath, journalTempPath, journalPath} {
		if err := removeIfExists(path); err != nil {
			return false, err
		}
	}
	if err := s.syncParentFolder(dir); err != nil {
		return targetAfter, err
	}
	return targetAfter, nil
}

func safeAtomicResidueExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return false, errors.New("atomic event bundle residue is unsafe")
	}
	return true, nil
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func digestEventFile(path string) (string, int64, error) {
	initial, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() {
		return "", 0, errors.New("atomic event bundle file is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, readErr := io.Copy(hash, file)
	opened, statErr := file.Stat()
	closeErr := file.Close()
	current, currentErr := os.Lstat(path)
	if readErr != nil || statErr != nil || closeErr != nil || currentErr != nil || !os.SameFile(initial, opened) || !os.SameFile(opened, current) || size != opened.Size() {
		return "", 0, errors.Join(readErr, statErr, closeErr, currentErr, errors.New("atomic event bundle file changed during read"))
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func digestBytes(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
