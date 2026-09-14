package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"time"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// The candidate is private recovery data, not a caller-replay buffer. Absence
// remains valid for old native records, but never grants resume capability.
func (s *ObjectEditingFiles) nativeCandidate(record nativeRecoveryRecord) ([]byte, error) {
	body, hash, err := s.readNativePrivate(s.nativePath(record.ObjectIdentity, record.Draft.ChangeID, ".after"), MaxOfficeObjectBytes)
	if err != nil {
		return nil, err
	}
	if !objectEditingHash(record.AfterHash) || hash != record.AfterHash || InspectOfficePackage(body, record.Kind) != nil {
		return nil, editing.ErrPersistence
	}
	return body, nil
}

func (s *ObjectEditingFiles) storeNativeCandidate(record nativeRecoveryRecord, body []byte) error {
	if int64(len(body)) > MaxOfficeObjectBytes || digestAtomicText(body) != record.AfterHash {
		return editing.ErrPersistence
	}
	previous, err := s.nativeCandidate(record)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return s.writeNativePrivate(s.nativePath(record.ObjectIdentity, record.Draft.ChangeID, ".after"), body, "", MaxOfficeObjectBytes)
	}
	if err != nil || !bytes.Equal(previous, body) {
		return editing.ErrPersistence
	}
	return nil
}

func nativeSaveRequestHash(record nativeRecoveryRecord, candidate []byte) string {
	return digestAtomicText([]byte(record.Draft.BaseRevision + "\x00" + base64.StdEncoding.EncodeToString(candidate)))
}

func nativeSaveJournalMatches(record nativeRecoveryRecord, journal objectEditingRecord) bool {
	return journal.ObjectIdentity == record.ObjectIdentity && journal.OperationID == nativeSaveID(record.Draft.ChangeID) && journal.PathBinding == record.PathBinding && journal.BeforeHash == record.Draft.BaseRevision && journal.AfterHash == record.AfterHash
}

func (s *ObjectEditingFiles) nativeCanResume(record nativeRecoveryRecord) (bool, error) {
	candidate, err := s.nativeCandidate(record)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	journal, _, err := s.readRecord(record.ObjectIdentity, nativeSaveID(record.Draft.ChangeID))
	if errors.Is(err, editing.ErrOperationNotFound) {
		return true, nil
	}
	if err != nil || !nativeSaveJournalMatches(record, journal) || journal.RequestHash != nativeSaveRequestHash(record, candidate) {
		return false, editing.ErrPersistence
	}
	return journal.Status == editing.StatusPending, nil
}

func (s *ObjectEditingFiles) finishNativeSave(target objectEditingTarget, index nativeRecoveryIndex, indexHash, change string, receipt editing.Receipt, commitErr error) (editing.Receipt, error) {
	if receipt.Status == editing.StatusCommitted && index.Pending == change {
		if err := s.nativeCurrentUndoSettled(target, index); err != nil {
			return receipt, err
		}
		if index.Current != "" {
			index.Retiring = append(index.Retiring, index.Current)
		}
		index.Current, index.Pending = change, ""
		if err := s.storeNativeIndex(index, &indexHash); err != nil {
			return receipt, editing.ErrPersistence
		}
		if err := s.drainNativeRetiring(target, &index, &indexHash); err != nil {
			return receipt, err
		}
	}
	return receipt, commitErr
}

// ResumeNativeChange is the only path that may retry a pending save's actual
// replacement. The caller provides fresh explicit authorization, not file bytes.
// Existing Commit/Status continue to reconcile v1 journals without rewriting.
func (s *ObjectEditingFiles) ResumeNativeChange(ctx context.Context, input editing.NativeUndoInput) (editing.Receipt, error) {
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
	index, indexHash, err := s.nativeState(target, input.ObjectIdentity)
	if err != nil {
		return editing.Receipt{}, err
	}
	record, _, err := s.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
	if err != nil {
		return editing.Receipt{}, err
	}
	if record.Draft.ThreadID != input.ThreadID || record.Draft.BaseRevision != input.BaseRevision || record.Disposition == "cancelled" || record.AfterHash == "" {
		return editing.Receipt{}, editing.ErrOperationMismatch
	}
	operation := nativeSaveID(input.ChangeID)
	journal, journalHash, journalErr := s.readRecord(input.ObjectIdentity, operation)
	if journalErr == nil {
		if !nativeSaveJournalMatches(record, journal) {
			return editing.Receipt{}, editing.ErrPersistence
		}
		if journal.Status == editing.StatusCommitted {
			// Historical completed operations are query-only, even when the
			// candidate has been collected or the document changed afterwards.
			receipt, err := s.reconcile(target, journal, journalHash)
			return s.finishNativeSave(target, index, indexHash, input.ChangeID, receipt, err)
		}
	} else if !errors.Is(journalErr, editing.ErrOperationNotFound) {
		return editing.Receipt{}, journalErr
	}
	if index.Pending != input.ChangeID || index.Preparing != nil || record.Disposition != "" || record.UndoStarted {
		return editing.Receipt{}, editing.ErrOperationMismatch
	}
	if err = s.nativeCurrentUndoSettled(target, index); err != nil {
		return editing.Receipt{}, err
	}
	if _, err = s.nativeOriginal(record); err != nil {
		return editing.Receipt{}, err
	}
	candidate, err := s.nativeCandidate(record)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return editing.Receipt{}, editing.ErrConflict
	}
	if err != nil {
		return editing.Receipt{}, err
	}
	requestHash := nativeSaveRequestHash(record, candidate)
	if journalErr == nil {
		if journal.RequestHash != requestHash {
			return editing.Receipt{}, editing.ErrPersistence
		}
		receipt, err := s.reconcile(target, journal, journalHash)
		if err != nil || receipt.Status == editing.StatusCommitted {
			return s.finishNativeSave(target, index, indexHash, input.ChangeID, receipt, err)
		}
		if receipt.Status == editing.StatusConflict {
			return receipt, editing.ErrConflict
		}
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return editing.Receipt{}, err
	}
	if digestAtomicText(state.Content) != input.BaseRevision {
		return editing.Receipt{OperationID: operation, Status: editing.StatusConflict}, editing.ErrConflict
	}
	// No absence-of-commit claim is made here: this is a newly authorized CAS
	// attempt against the same original base and immutable candidate.
	if err = ctx.Err(); err != nil {
		return editing.Receipt{}, err
	}
	if errors.Is(journalErr, editing.ErrOperationNotFound) {
		journal = objectEditingRecord{Version: 1, ObjectIdentity: input.ObjectIdentity, OperationID: operation, PathBinding: target.binding, RequestHash: requestHash, BeforeHash: input.BaseRevision, AfterHash: record.AfterHash, Status: editing.StatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if err = s.writeRecord(journal, ""); err != nil {
			return editing.Receipt{OperationID: operation, Status: editing.StatusUnknown}, err
		}
	}
	if err = ctx.Err(); err != nil {
		return editing.Receipt{OperationID: operation, Status: editing.StatusUnknown}, err
	}
	commit := editing.CommitInput{Workspace: input.Workspace, Path: input.Path, ObjectIdentity: input.ObjectIdentity, OperationID: operation, BaseRevision: input.BaseRevision, Content: base64.StdEncoding.EncodeToString(candidate)}
	receipt, err := s.replaceObjectLocked(target, commit, state, candidate)
	return s.finishNativeSave(target, index, indexHash, input.ChangeID, receipt, err)
}
