package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// Two 64 KiB local strings can expand sixfold under canonical JSON escaping.
// This limit includes both strings and later commit fields; v1 receipts stay 16 KiB.
const nativeChangeMetadataBytes = 1 << 20

var nativeRecoveryThread = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var nativeRecoveryProposal = regexp.MustCompile(`^[a-f0-9]{48}$`)

type nativeRecoveryIndex struct {
	Version        int    `json:"version"`
	ObjectIdentity string `json:"objectIdentity"`
	PathBinding    string `json:"pathBinding"`
	Current        string `json:"current,omitempty"`
	Pending        string `json:"pending,omitempty"`
	// Reserve an ID before any large file is written. The reservation contains
	// only a draft digest and short identity fields, never review text.
	Preparing *nativeRecoveryRecord `json:"preparing,omitempty"`
	Retiring  []string              `json:"retiring,omitempty"`
}
type nativeRecoveryRecord struct {
	Version        int                       `json:"version"`
	ObjectIdentity string                    `json:"objectIdentity"`
	PathBinding    string                    `json:"pathBinding"`
	Kind           string                    `json:"kind"`
	Draft          editing.NativeChangeDraft `json:"draft"`
	AfterHash      string                    `json:"afterHash,omitempty"`
	UndoStarted    bool                      `json:"undoStarted"`
	CreatedAt      string                    `json:"createdAt"`
	DraftHash      string                    `json:"draftHash,omitempty"`
	Disposition    string                    `json:"disposition,omitempty"`
}

func nativeDraftHash(d editing.NativeChangeDraft) string {
	body, _ := json.Marshal(d)
	return digestAtomicText(body)
}
func nativeRecordDraftHash(r nativeRecoveryRecord) string {
	if r.DraftHash != "" {
		return r.DraftHash
	}
	return nativeDraftHash(r.Draft)
}
func nativeCompact(r nativeRecoveryRecord, disposition string) nativeRecoveryRecord {
	r.DraftHash = nativeRecordDraftHash(r)
	r.Draft.BeforeText, r.Draft.AfterText, r.Disposition = "", "", disposition
	return r
}

func nativeDraftValid(d editing.NativeChangeDraft) bool {
	return objectEditingHash(d.ChangeID) && objectEditingHash(d.BaseRevision) && nativeRecoveryThread.MatchString(d.ThreadID) && nativeRecoveryProposal.MatchString(d.ProposalID) &&
		len(d.BeforeText) <= 65536 && len(d.AfterText) <= 65536 && utf8.ValidString(d.BeforeText) && utf8.ValidString(d.AfterText) && !strings.ContainsRune(d.BeforeText+d.AfterText, 0)
}
func (s *ObjectEditingFiles) nativePath(identity, change, suffix string) string {
	return filepath.Join(s.receiptRoot, "native-"+digestAtomicText([]byte(identity+"\x00"+change))+suffix)
}
func (s *ObjectEditingFiles) readNativePrivate(path string, maxBytes int64) ([]byte, string, error) {
	if s.officeKind == "" || s.checkRoot() != nil {
		return nil, "", editing.ErrForbidden
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil, "", editing.ErrOperationNotFound
	}
	if objectEditingPrivateFile(path, maxBytes) != nil {
		return nil, "", editing.ErrPersistence
	}
	state, err := inspectAtomicTextTargetWithPolicy(path, false, maxBytes, atomicTextReadPolicy{RequireSingleLink: true})
	if err != nil || !state.Exists || s.checkRoot() != nil || objectEditingPrivateFile(path, maxBytes) != nil {
		return nil, "", editing.ErrPersistence
	}
	return state.Content, digestAtomicText(state.Content), nil
}
func (s *ObjectEditingFiles) writeNativePrivate(path string, body []byte, expected string, maxBytes int64) error {
	if s.officeKind == "" || s.checkRoot() != nil || int64(len(body)) > maxBytes {
		return editing.ErrPersistence
	}
	replace := s.replaceJournal
	if replace == nil {
		replace = atomicReplaceText
	}
	if replace(atomicTextReplaceRequest{Path: path, Content: body, MaxBytes: maxBytes, ReadPolicy: atomicTextReadPolicy{RequireSingleLink: true}, ExpectedExists: expected != "", ExpectedHash: expected, DefaultMode: 0600}) != nil {
		return editing.ErrPersistence
	}
	actual, _, err := s.readNativePrivate(path, maxBytes)
	if err != nil || !bytes.Equal(actual, body) {
		return editing.ErrPersistence
	}
	return nil
}
func nativeDecode(body []byte, destination any, maxBytes int) error {
	if jsonstrict.Validate(body, jsonstrict.Options{RequireObject: true, MaxBytes: maxBytes, MaxDepth: 3, MaxTokens: 128, MaxStringBytes: 65536}) != nil {
		return editing.ErrPersistence
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return editing.ErrPersistence
	}
	canonical, err := json.Marshal(destination)
	if err != nil || !bytes.Equal(canonical, body) {
		return editing.ErrPersistence
	}
	return nil
}
func (s *ObjectEditingFiles) nativeIndex(target objectEditingTarget, identity string) (nativeRecoveryIndex, string, error) {
	body, hash, err := s.readNativePrivate(s.nativePath(identity, "index", ".json"), 16384)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return nativeRecoveryIndex{Version: 1, ObjectIdentity: identity, PathBinding: target.binding}, "", nil
	}
	var index nativeRecoveryIndex
	if err != nil || nativeDecode(body, &index, 16384) != nil || index.Version != 1 || index.ObjectIdentity != identity || index.PathBinding != target.binding || (index.Current != "" && !objectEditingHash(index.Current)) || (index.Pending != "" && !objectEditingHash(index.Pending)) || index.Pending != "" && index.Current == index.Pending {
		return index, "", editing.ErrPersistence
	}
	seen := map[string]bool{index.Current: true, index.Pending: true}
	if index.Preparing != nil {
		r := index.Preparing
		if !nativeRecordValid(*r, target, identity, r.Draft.ChangeID, s.officeKind) || r.DraftHash == "" || r.Draft.BeforeText != "" || r.Draft.AfterText != "" || r.AfterHash != "" || r.UndoStarted || r.Disposition != "" || seen[r.Draft.ChangeID] {
			return index, "", editing.ErrPersistence
		}
		seen[r.Draft.ChangeID] = true
	}
	if len(index.Retiring) > 3 {
		return index, "", editing.ErrPersistence
	}
	for _, id := range index.Retiring {
		if !objectEditingHash(id) || seen[id] {
			return index, "", editing.ErrPersistence
		}
		seen[id] = true
	}
	return index, hash, nil
}
func (s *ObjectEditingFiles) writeNativeIndex(index nativeRecoveryIndex, expected string) error {
	body, err := json.Marshal(index)
	if err != nil {
		return editing.ErrPersistence
	}
	return s.writeNativePrivate(s.nativePath(index.ObjectIdentity, "index", ".json"), body, expected, 16384)
}
func (s *ObjectEditingFiles) nativeRecord(target objectEditingTarget, identity, change string) (nativeRecoveryRecord, string, error) {
	body, hash, err := s.readNativePrivate(s.nativePath(identity, change, ".change.json"), nativeChangeMetadataBytes)
	var record nativeRecoveryRecord
	if errors.Is(err, editing.ErrOperationNotFound) {
		return record, "", err
	}
	if err != nil || nativeDecode(body, &record, nativeChangeMetadataBytes) != nil || !nativeRecordValid(record, target, identity, change, s.officeKind) {
		return record, "", editing.ErrPersistence
	}
	if record.Disposition == "" && record.DraftHash != "" && record.DraftHash != nativeDraftHash(record.Draft) {
		return record, "", editing.ErrPersistence
	}
	return record, hash, nil
}
func nativeRecordValid(record nativeRecoveryRecord, target objectEditingTarget, identity, change, kind string) bool {
	if record.Version != 1 || record.ObjectIdentity != identity || record.PathBinding != target.binding || record.Kind != kind || record.Draft.ChangeID != change || !nativeDraftValid(record.Draft) || (record.AfterHash != "" && !objectEditingHash(record.AfterHash)) || (record.DraftHash != "" && !objectEditingHash(record.DraftHash)) || (record.UndoStarted && record.AfterHash == "") {
		return false
	}
	if record.Disposition != "" && (record.Disposition != "cancelled" && record.Disposition != "superseded" || record.DraftHash == "" || record.Draft.BeforeText != "" || record.Draft.AfterText != "") {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	return err == nil
}
func (s *ObjectEditingFiles) writeNativeRecord(record nativeRecoveryRecord, expected string) error {
	body, err := json.Marshal(record)
	if err != nil {
		return editing.ErrPersistence
	}
	return s.writeNativePrivate(s.nativePath(record.ObjectIdentity, record.Draft.ChangeID, ".change.json"), body, expected, nativeChangeMetadataBytes)
}
func nativeSaveID(change string) string { return "native_save_" + change }
func nativeUndoID(change string) string { return "native_undo_" + change }
func (s *ObjectEditingFiles) nativeOriginal(record nativeRecoveryRecord) ([]byte, error) {
	body, _, err := s.readNativePrivate(s.nativePath(record.ObjectIdentity, record.Draft.ChangeID, ".before"), MaxOfficeObjectBytes)
	if err != nil || digestAtomicText(body) != record.Draft.BaseRevision || InspectOfficePackage(body, s.officeKind) != nil {
		return nil, editing.ErrPersistence
	}
	return body, nil
}

func (s *ObjectEditingFiles) storeNativeIndex(index nativeRecoveryIndex, hash *string) error {
	if err := s.writeNativeIndex(index, *hash); err != nil {
		return err
	}
	body, _ := json.Marshal(index)
	*hash = digestAtomicText(body)
	return nil
}

// Empty the private blob through the same bounded CAS primitive rather than
// unlinking a path that could have changed after inspection. Small tombstones
// and empty blob markers retain replay identity without retaining source bytes.
func (s *ObjectEditingFiles) retireNativeRecord(target objectEditingTarget, identity, change, disposition string) error {
	r, hash, err := s.nativeRecord(target, identity, change)
	if err != nil {
		return err
	}
	if r.Disposition == "" {
		r = nativeCompact(r, disposition)
		if err = s.writeNativeRecord(r, hash); err != nil {
			return err
		}
	}
	path := s.nativePath(identity, change, ".before")
	body, blobHash, err := s.readNativePrivate(path, MaxOfficeObjectBytes)
	if errors.Is(err, editing.ErrOperationNotFound) || err == nil && len(body) == 0 {
		return nil
	}
	if err != nil || blobHash != r.Draft.BaseRevision {
		return editing.ErrPersistence
	}
	return s.writeNativePrivate(path, []byte{}, blobHash, MaxOfficeObjectBytes)
}

func (s *ObjectEditingFiles) drainNativeRetiring(target objectEditingTarget, index *nativeRecoveryIndex, hash *string) error {
	for len(index.Retiring) > 0 {
		if err := s.retireNativeRecord(target, index.ObjectIdentity, index.Retiring[0], "superseded"); err != nil {
			return err
		}
		index.Retiring = index.Retiring[1:]
		if err := s.storeNativeIndex(*index, hash); err != nil {
			return err
		}
	}
	return nil
}

// Finish interrupted index transitions before admitting another change. A
// preparing reservation bounds partial writes to one additional blob and keeps
// incomplete preparations discoverable/cancellable after a restart.
func (s *ObjectEditingFiles) nativeState(target objectEditingTarget, identity string) (nativeRecoveryIndex, string, error) {
	index, hash, err := s.nativeIndex(target, identity)
	if err != nil {
		return index, hash, err
	}
	if err = s.drainNativeRetiring(target, &index, &hash); err != nil {
		return index, hash, err
	}
	if index.Preparing != nil {
		reserved := *index.Preparing
		r, _, readErr := s.nativeRecord(target, identity, reserved.Draft.ChangeID)
		if readErr != nil && !errors.Is(readErr, editing.ErrOperationNotFound) {
			return index, hash, readErr
		}
		if readErr == nil {
			if nativeRecordDraftHash(r) != reserved.DraftHash {
				return index, hash, editing.ErrPersistence
			}
			complete := r.Disposition != ""
			if !complete {
				_, _, blobErr := s.readNativePrivate(s.nativePath(identity, r.Draft.ChangeID, ".before"), MaxOfficeObjectBytes)
				if blobErr == nil {
					if _, err = s.nativeOriginal(r); err != nil {
						return index, hash, err
					}
					complete = true
				} else if !errors.Is(blobErr, editing.ErrOperationNotFound) {
					return index, hash, blobErr
				}
			}
			if complete {
				index.Preparing = nil
				if r.Disposition != "" {
					index.Retiring = append(index.Retiring, r.Draft.ChangeID)
				} else {
					if index.Pending != "" {
						index.Retiring = append(index.Retiring, index.Pending)
					}
					index.Pending = r.Draft.ChangeID
				}
				if err = s.storeNativeIndex(index, &hash); err != nil {
					return index, hash, err
				}
			}
		}
	}
	if index.Pending != "" {
		r, _, readErr := s.nativeRecord(target, identity, index.Pending)
		if readErr != nil {
			return index, hash, readErr
		}
		if r.Disposition != "" {
			index.Retiring = append(index.Retiring, index.Pending)
			index.Pending = ""
			if err = s.storeNativeIndex(index, &hash); err != nil {
				return index, hash, err
			}
		}
	}
	err = s.drainNativeRetiring(target, &index, &hash)
	return index, hash, err
}

// An interrupted undo still owns its original. A later approval must not hide
// that operation and then retire its only recovery bytes when it commits.
func (s *ObjectEditingFiles) nativeCurrentUndoSettled(target objectEditingTarget, index nativeRecoveryIndex) error {
	if index.Current == "" {
		return nil
	}
	record, _, err := s.nativeRecord(target, index.ObjectIdentity, index.Current)
	if err != nil {
		return err
	}
	if !record.UndoStarted {
		return nil
	}
	journal, _, err := s.readRecord(index.ObjectIdentity, nativeUndoID(index.Current))
	if err != nil || journal.Status != editing.StatusCommitted || journal.PathBinding != target.binding || journal.BeforeHash != record.AfterHash || journal.AfterHash != record.Draft.BaseRevision {
		return editing.ErrConflict
	}
	return nil
}

// PrepareNativeChange captures the original from Core's authorized target. A
// replacement pending review is admitted only before any save was attempted.
func (s *ObjectEditingFiles) PrepareNativeChange(ctx context.Context, input editing.NativeChangeInput) (editing.NativeChangeStatus, error) {
	if !objectEditingHash(input.ObjectIdentity) || !nativeDraftValid(input.Draft) || s.officeKind == "" {
		return editing.NativeChangeStatus{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return editing.NativeChangeStatus{}, err
	}
	index, indexHash, err := s.nativeState(target, input.ObjectIdentity)
	if err != nil {
		return editing.NativeChangeStatus{}, err
	}
	record, _, recordErr := s.nativeRecord(target, input.ObjectIdentity, input.Draft.ChangeID)
	if recordErr == nil {
		if nativeRecordDraftHash(record) != nativeDraftHash(input.Draft) || record.Disposition != "" {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
		if index.Current == input.Draft.ChangeID || index.Pending == input.Draft.ChangeID {
			return s.nativeStatus(ctx, target, record)
		}
		if index.Preparing == nil || index.Preparing.Draft.ChangeID != input.Draft.ChangeID {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
	} else if !errors.Is(recordErr, editing.ErrOperationNotFound) {
		return editing.NativeChangeStatus{}, recordErr
	}
	if err = s.nativeCurrentUndoSettled(target, index); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	if index.Pending != "" {
		previous, _, err := s.nativeRecord(target, input.ObjectIdentity, index.Pending)
		if err != nil {
			return editing.NativeChangeStatus{}, err
		}
		if previous.AfterHash != "" {
			return editing.NativeChangeStatus{}, editing.ErrConflict
		}
	}
	if index.Preparing != nil && index.Preparing.Draft.ChangeID != input.Draft.ChangeID {
		if err = s.retireNativePreparing(target, &index, &indexHash, "superseded"); err != nil {
			return editing.NativeChangeStatus{}, err
		}
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return editing.NativeChangeStatus{}, err
	}
	if digestAtomicText(state.Content) != input.Draft.BaseRevision {
		return editing.NativeChangeStatus{}, editing.ErrConflict
	}
	if InspectOfficePackage(state.Content, s.officeKind) != nil {
		return editing.NativeChangeStatus{}, editing.ErrNotText
	}
	if index.Preparing == nil {
		record = nativeRecoveryRecord{Version: 1, ObjectIdentity: input.ObjectIdentity, PathBinding: target.binding, Kind: s.officeKind, Draft: input.Draft, DraftHash: nativeDraftHash(input.Draft), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		reservation := nativeCompact(record, "")
		index.Preparing = &reservation
		if err = s.storeNativeIndex(index, &indexHash); err != nil {
			return editing.NativeChangeStatus{}, err
		}
	} else {
		if index.Preparing.DraftHash != nativeDraftHash(input.Draft) {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
		record = *index.Preparing
		record.Draft = input.Draft
	}
	if errors.Is(recordErr, editing.ErrOperationNotFound) {
		if err = s.writeNativeRecord(record, ""); err != nil {
			return editing.NativeChangeStatus{}, err
		}
	}
	blobPath := s.nativePath(input.ObjectIdentity, input.Draft.ChangeID, ".before")
	if old, _, readErr := s.readNativePrivate(blobPath, MaxOfficeObjectBytes); readErr == nil {
		if !bytes.Equal(old, state.Content) {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
	} else if !errors.Is(readErr, editing.ErrOperationNotFound) {
		return editing.NativeChangeStatus{}, readErr
	} else if err = s.writeNativePrivate(blobPath, state.Content, "", MaxOfficeObjectBytes); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	if _, _, err = s.nativeState(target, input.ObjectIdentity); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	return s.nativeStatus(ctx, target, record)
}

func (s *ObjectEditingFiles) retireNativePreparing(target objectEditingTarget, index *nativeRecoveryIndex, indexHash *string, disposition string) error {
	reserved := *index.Preparing
	record, hash, err := s.nativeRecord(target, index.ObjectIdentity, reserved.Draft.ChangeID)
	if errors.Is(err, editing.ErrOperationNotFound) {
		record, hash = reserved, ""
	} else if err != nil {
		return err
	}
	if nativeRecordDraftHash(record) != reserved.DraftHash || record.AfterHash != "" || record.UndoStarted {
		return editing.ErrPersistence
	}
	record = nativeCompact(record, disposition)
	if err = s.writeNativeRecord(record, hash); err != nil {
		return err
	}
	index.Preparing = nil
	index.Retiring = append(index.Retiring, record.Draft.ChangeID)
	if err = s.storeNativeIndex(*index, indexHash); err != nil {
		return err
	}
	return s.drainNativeRetiring(target, index, indexHash)
}

func (s *ObjectEditingFiles) nativeStatus(ctx context.Context, target objectEditingTarget, record nativeRecoveryRecord) (editing.NativeChangeStatus, error) {
	d := record.Draft
	status := editing.NativeChangeStatus{ChangeID: d.ChangeID, ThreadID: d.ThreadID, ProposalID: d.ProposalID, BaseRevision: d.BaseRevision, Status: "prepared", BeforeText: d.BeforeText, AfterText: d.AfterText, SaveOperationID: nativeSaveID(d.ChangeID), UndoOperationID: nativeUndoID(d.ChangeID), CreatedAt: record.CreatedAt}
	if ctx.Err() != nil {
		return status, ctx.Err()
	}
	if record.Disposition != "" {
		status.Status = record.Disposition
		return status, nil
	}
	if _, err := s.nativeOriginal(record); err != nil {
		return status, err
	}
	operation := status.SaveOperationID
	canCancel, missingJournal := record.AfterHash == "", false
	if record.UndoStarted {
		operation = status.UndoOperationID
	}
	if record.AfterHash != "" {
		journal, journalHash, err := s.readRecord(record.ObjectIdentity, operation)
		if errors.Is(err, editing.ErrOperationNotFound) {
			status.Status = "unknown"
			missingJournal = true
		} else if err != nil {
			return status, err
		} else {
			expectedBefore, expectedAfter := d.BaseRevision, record.AfterHash
			if record.UndoStarted {
				expectedBefore, expectedAfter = record.AfterHash, d.BaseRevision
			}
			if journal.BeforeHash != expectedBefore || journal.AfterHash != expectedAfter {
				return status, editing.ErrPersistence
			}
			receipt, err := s.reconcile(target, journal, journalHash)
			if err != nil {
				return status, err
			}
			status.Status, status.Revision, status.SavedAt = receipt.Status, receipt.Revision, receipt.SavedAt
			// v1 conflict can also be reconciled from an uncertain pending write.
			// It is not proof that the document was never committed.
			canCancel = false
			if record.UndoStarted && receipt.Status == editing.StatusCommitted {
				status.Status = "undone"
			}
		}
	}
	current, err := s.inspectObject(target.path)
	if err != nil {
		return status, err
	}
	revision := digestAtomicText(current.Content)
	if missingJournal && !record.UndoStarted && revision == d.BaseRevision {
		canCancel = true
	}
	if missingJournal && record.UndoStarted && revision == record.AfterHash {
		status.CanRetryUndo, status.Revision = true, record.AfterHash
	}
	status.CanCancel = canCancel
	switch status.Status {
	case "prepared":
		if revision != d.BaseRevision {
			status.Status = "conflict"
		}
	case "committed":
		if revision != record.AfterHash {
			status.Status = "conflict"
		} else {
			status.CanUndo = true
		}
	case "undone":
		if revision != d.BaseRevision {
			status.Status = "conflict"
		}
	}
	if ctx.Err() != nil {
		return status, ctx.Err()
	}
	return status, nil
}

func (s *ObjectEditingFiles) NativeRecovery(ctx context.Context, identity, workspace, path, thread string) (editing.NativeRecovery, error) {
	var result editing.NativeRecovery
	if !objectEditingHash(identity) || !nativeRecoveryThread.MatchString(thread) || s.officeKind == "" {
		return result, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return result, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(workspace, path)
	if err != nil {
		return result, err
	}
	return s.nativeRecoveryLocked(ctx, target, identity, thread)
}

func (s *ObjectEditingFiles) nativeRecoveryLocked(ctx context.Context, target objectEditingTarget, identity, thread string) (editing.NativeRecovery, error) {
	var result editing.NativeRecovery
	index, indexHash, err := s.nativeState(target, identity)
	if err != nil {
		return result, err
	}
	if index.Pending != "" {
		record, _, err := s.nativeRecord(target, identity, index.Pending)
		if err != nil {
			return result, err
		}
		status, err := s.nativeStatus(ctx, target, record)
		if err != nil {
			return result, err
		}
		if status.Status == "committed" || status.Status == "undone" {
			if err = s.nativeCurrentUndoSettled(target, index); err != nil {
				return result, err
			}
			if index.Current != "" {
				index.Retiring = append(index.Retiring, index.Current)
			}
			index.Current, index.Pending = index.Pending, ""
			if err = s.storeNativeIndex(index, &indexHash); err != nil {
				return result, err
			}
			if err = s.drainNativeRetiring(target, &index, &indexHash); err != nil {
				return result, err
			}
		} else if record.Draft.ThreadID == thread {
			result.Pending = &status
		}
	}
	if index.Current != "" {
		record, _, err := s.nativeRecord(target, identity, index.Current)
		if err != nil {
			return result, err
		}
		if record.Draft.ThreadID == thread {
			status, err := s.nativeStatus(ctx, target, record)
			if err != nil {
				return result, err
			}
			status.CanCancel = false
			if index.Pending != "" || index.Preparing != nil {
				status.CanUndo, status.CanRetryUndo = false, false
			}
			result.Current = &status
		}
	}
	if index.Preparing != nil && index.Preparing.Draft.ThreadID == thread {
		r := index.Preparing
		result.Pending = &editing.NativeChangeStatus{ChangeID: r.Draft.ChangeID, ThreadID: thread, ProposalID: r.Draft.ProposalID, BaseRevision: r.Draft.BaseRevision, Status: "unknown", SaveOperationID: nativeSaveID(r.Draft.ChangeID), UndoOperationID: nativeUndoID(r.Draft.ChangeID), CreatedAt: r.CreatedAt, CanCancel: true}
	}
	return result, nil
}

func (s *ObjectEditingFiles) CommitNativeChange(ctx context.Context, input editing.NativeCommitInput) (editing.Receipt, error) {
	if !objectEditingHash(input.ObjectIdentity) || !objectEditingHash(input.ChangeID) || input.OperationID != nativeSaveID(input.ChangeID) || s.officeKind == "" {
		return editing.Receipt{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return editing.Receipt{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return editing.Receipt{}, err
	}
	index, indexHash, err := s.nativeState(target, input.ObjectIdentity)
	if err != nil {
		return editing.Receipt{}, err
	}
	record, recordHash, err := s.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
	if err != nil {
		return editing.Receipt{}, err
	}
	if record.Draft.BaseRevision != input.BaseRevision || record.Draft.ThreadID != input.ThreadID || record.UndoStarted {
		return editing.Receipt{}, editing.ErrOperationMismatch
	}
	if record.Disposition != "" {
		if record.Disposition != "superseded" {
			return editing.Receipt{}, editing.ErrOperationMismatch
		}
		journal, hash, err := s.readRecord(input.ObjectIdentity, input.OperationID)
		if err != nil || journal.Status != editing.StatusCommitted || journal.RequestHash != digestAtomicText([]byte(input.BaseRevision+"\x00"+input.Content)) || journal.BeforeHash != record.Draft.BaseRevision || journal.AfterHash != record.AfterHash {
			return editing.Receipt{}, editing.ErrOperationMismatch
		}
		return s.reconcile(target, journal, hash)
	}
	if index.Pending != input.ChangeID && index.Current != input.ChangeID {
		return editing.Receipt{}, editing.ErrOperationMismatch
	}
	if index.Pending == input.ChangeID {
		if err = s.nativeCurrentUndoSettled(target, index); err != nil {
			return editing.Receipt{}, err
		}
	}
	if _, err := s.nativeOriginal(record); err != nil {
		return editing.Receipt{}, err
	}
	encoded, err := s.encodeObject(input.Content, "office-base64")
	if err != nil {
		return editing.Receipt{}, err
	}
	afterHash := digestAtomicText(encoded)
	if record.AfterHash != "" && record.AfterHash != afterHash {
		return editing.Receipt{}, editing.ErrOperationMismatch
	}
	if record.AfterHash == "" {
		record.AfterHash = afterHash
		if err = s.writeNativeRecord(record, recordHash); err != nil {
			return editing.Receipt{}, err
		}
	}
	receipt, commitErr := s.commitLocked(ctx, input.CommitInput)
	if receipt.Status == editing.StatusCommitted && index.Pending == input.ChangeID {
		if index.Current != "" {
			index.Retiring = append(index.Retiring, index.Current)
		}
		index.Current, index.Pending = input.ChangeID, ""
		if err = s.storeNativeIndex(index, &indexHash); err != nil {
			return receipt, editing.ErrPersistence
		}
		if err = s.drainNativeRetiring(target, &index, &indexHash); err != nil {
			return receipt, err
		}
	}
	return receipt, commitErr
}

func (s *ObjectEditingFiles) UndoNativeChange(ctx context.Context, input editing.NativeUndoInput) (editing.Receipt, error) {
	if !objectEditingHash(input.ObjectIdentity) || !objectEditingHash(input.ChangeID) || !objectEditingHash(input.BaseRevision) || !nativeRecoveryThread.MatchString(input.ThreadID) || s.officeKind == "" {
		return editing.Receipt{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return editing.Receipt{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return editing.Receipt{}, err
	}
	index, _, err := s.nativeState(target, input.ObjectIdentity)
	if err != nil {
		return editing.Receipt{}, err
	}
	record, recordHash, err := s.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
	if err != nil {
		return editing.Receipt{}, err
	}
	if record.Draft.ThreadID != input.ThreadID || record.AfterHash != input.BaseRevision {
		return editing.Receipt{}, editing.ErrOperationMismatch
	}
	if record.Disposition != "" {
		journal, hash, err := s.readRecord(input.ObjectIdentity, nativeUndoID(input.ChangeID))
		if record.Disposition != "superseded" || !record.UndoStarted || err != nil || journal.Status != editing.StatusCommitted || journal.BeforeHash != record.AfterHash || journal.AfterHash != record.Draft.BaseRevision {
			return editing.Receipt{}, editing.ErrOperationMismatch
		}
		return s.reconcile(target, journal, hash)
	}
	if index.Current != input.ChangeID || index.Pending != "" || index.Preparing != nil {
		return editing.Receipt{}, editing.ErrConflict
	}
	original, err := s.nativeOriginal(record)
	if err != nil {
		return editing.Receipt{}, err
	}
	if !record.UndoStarted {
		status, err := s.nativeStatus(ctx, target, record)
		if err != nil {
			return editing.Receipt{}, err
		}
		if !status.CanUndo {
			return editing.Receipt{}, editing.ErrConflict
		}
		record.UndoStarted = true
		if err = s.writeNativeRecord(record, recordHash); err != nil {
			return editing.Receipt{}, err
		}
	}
	content, _, err := s.decodeObject(original)
	if err != nil {
		return editing.Receipt{}, err
	}
	return s.commitLocked(ctx, editing.CommitInput{Workspace: input.Workspace, Path: input.Path, ObjectIdentity: input.ObjectIdentity, OperationID: nativeUndoID(input.ChangeID), BaseRevision: input.BaseRevision, Content: content})
}

func (s *ObjectEditingFiles) CancelNativeChange(ctx context.Context, input editing.NativeUndoInput) (editing.NativeRecovery, error) {
	var result editing.NativeRecovery
	if !objectEditingHash(input.ObjectIdentity) || !objectEditingHash(input.ChangeID) || !objectEditingHash(input.BaseRevision) || !nativeRecoveryThread.MatchString(input.ThreadID) || s.officeKind == "" {
		return result, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return result, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return result, err
	}
	index, indexHash, err := s.nativeState(target, input.ObjectIdentity)
	if err != nil {
		return result, err
	}
	current, err := s.inspectObject(target.path)
	if err != nil {
		return result, err
	}
	if digestAtomicText(current.Content) != input.BaseRevision {
		return result, editing.ErrConflict
	}
	if index.Preparing != nil && index.Preparing.Draft.ChangeID == input.ChangeID {
		if index.Preparing.Draft.ThreadID != input.ThreadID {
			return result, editing.ErrOperationMismatch
		}
		if err = s.retireNativePreparing(target, &index, &indexHash, "cancelled"); err != nil {
			return result, err
		}
		return s.nativeRecoveryLocked(ctx, target, input.ObjectIdentity, input.ThreadID)
	}
	record, recordHash, err := s.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
	if err != nil {
		return result, err
	}
	if record.Draft.ThreadID != input.ThreadID {
		return result, editing.ErrOperationMismatch
	}
	if record.Disposition == "cancelled" {
		return s.nativeRecoveryLocked(ctx, target, input.ObjectIdentity, input.ThreadID)
	}
	if index.Pending != input.ChangeID || record.Disposition != "" {
		return result, editing.ErrOperationMismatch
	}
	status, err := s.nativeStatus(ctx, target, record)
	if err != nil {
		return result, err
	}
	if !status.CanCancel {
		return result, editing.ErrConflict
	}
	if current, err = s.inspectObject(target.path); err != nil || digestAtomicText(current.Content) != input.BaseRevision {
		return result, editing.ErrConflict
	}
	record = nativeCompact(record, "cancelled")
	if err = s.writeNativeRecord(record, recordHash); err != nil {
		return result, err
	}
	index.Pending = ""
	index.Retiring = append(index.Retiring, input.ChangeID)
	if err = s.storeNativeIndex(index, &indexHash); err != nil {
		return result, err
	}
	if err = s.drainNativeRetiring(target, &index, &indexHash); err != nil {
		return result, err
	}
	return s.nativeRecoveryLocked(ctx, target, input.ObjectIdentity, input.ThreadID)
}
