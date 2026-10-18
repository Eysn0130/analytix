package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// The v1 struct order and draft hash remain unchanged. v2 has a separate tagged
// encoding, including when embedded in the existing preparing reservation.
type nativeManualRecordV2 struct {
	Version        int                       `json:"version"`
	Origin         string                    `json:"origin"`
	ObjectIdentity string                    `json:"objectIdentity"`
	PathBinding    string                    `json:"pathBinding"`
	Kind           string                    `json:"kind"`
	Draft          editing.NativeManualDraft `json:"draft"`
	AfterHash      string                    `json:"afterHash,omitempty"`
	UndoStarted    bool                      `json:"undoStarted"`
	CreatedAt      string                    `json:"createdAt"`
	DraftHash      string                    `json:"draftHash,omitempty"`
	Disposition    string                    `json:"disposition,omitempty"`
}

func (r nativeRecoveryRecord) MarshalJSON() ([]byte, error) {
	type legacy nativeRecoveryRecord
	if r.Version != 2 {
		return json.Marshal(legacy(r))
	}
	if r.Manual == nil {
		return nil, editing.ErrPersistence
	}
	return json.Marshal(nativeManualRecordV2{2, "manual", r.ObjectIdentity, r.PathBinding, r.Kind, *r.Manual, r.AfterHash, r.UndoStarted, r.CreatedAt, r.DraftHash, r.Disposition})
}
func (r *nativeRecoveryRecord) UnmarshalJSON(body []byte) error {
	var header struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(body, &header) != nil {
		return editing.ErrPersistence
	}
	if header.Version == 1 {
		type legacy nativeRecoveryRecord
		var value legacy
		if nativeDecode(body, &value, nativeChangeMetadataBytes) != nil {
			return editing.ErrPersistence
		}
		*r = nativeRecoveryRecord(value)
		return nil
	}
	var value nativeManualRecordV2
	if header.Version != 2 || nativeDecode(body, &value, nativeChangeMetadataBytes) != nil || value.Origin != "manual" {
		return editing.ErrPersistence
	}
	*r = nativeRecoveryRecord{Version: 2, Manual: &value.Draft, ObjectIdentity: value.ObjectIdentity, PathBinding: value.PathBinding, Kind: value.Kind,
		Draft:     editing.NativeChangeDraft{ChangeID: value.Draft.ChangeID, ThreadID: value.Draft.ThreadID, BaseRevision: value.Draft.BaseRevision},
		AfterHash: value.AfterHash, UndoStarted: value.UndoStarted, CreatedAt: value.CreatedAt, DraftHash: value.DraftHash, Disposition: value.Disposition}
	return nil
}
func nativeManualValid(d editing.NativeManualDraft) bool {
	return objectEditingHash(d.ChangeID) && objectEditingHash(d.BaseRevision) && nativeRecoveryThread.MatchString(d.ThreadID) && objectEditingOperationID.MatchString(d.BeginOperationID)
}
func nativeRecordVariantValid(r nativeRecoveryRecord, kind string) bool {
	if r.Version == 1 {
		return r.Manual == nil && nativeDraftValid(r.Draft)
	}
	return r.Version == 2 && kind == "docx" && r.Manual != nil && nativeManualValid(*r.Manual) &&
		r.Draft == (editing.NativeChangeDraft{ChangeID: r.Manual.ChangeID, ThreadID: r.Manual.ThreadID, BaseRevision: r.Manual.BaseRevision})
}
func nativeRecordContentHash(r nativeRecoveryRecord) string {
	if r.Version != 2 {
		return nativeDraftHash(r.Draft)
	}
	body, _ := json.Marshal(struct {
		Version int                        `json:"version"`
		Origin  string                     `json:"origin"`
		Draft   *editing.NativeManualDraft `json:"draft"`
	}{2, "manual", r.Manual})
	return digestAtomicText(body)
}

// Begin captures Core's original before the native engine is enabled. Existing
// pending authority, including an uncertain save, must be explicitly resolved.
func (s *ObjectEditingFiles) PrepareNativeManual(ctx context.Context, input editing.NativeManualInput) (editing.NativeChangeStatus, error) {
	if s.officeKind != "docx" || !objectEditingHash(input.ObjectIdentity) || !nativeManualValid(input.Draft) {
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
	if _, err = s.nativeRecoveryLocked(ctx, target, input.ObjectIdentity, input.Draft.ThreadID); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	index, indexHash, err := s.nativeState(target, input.ObjectIdentity)
	if err != nil {
		return editing.NativeChangeStatus{}, err
	}
	record := nativeRecoveryRecord{Version: 2, Manual: &input.Draft, ObjectIdentity: input.ObjectIdentity, PathBinding: target.binding, Kind: "docx",
		Draft: editing.NativeChangeDraft{ChangeID: input.Draft.ChangeID, ThreadID: input.Draft.ThreadID, BaseRevision: input.Draft.BaseRevision}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	record.DraftHash = nativeRecordContentHash(record)
	stored, _, readErr := s.nativeRecord(target, input.ObjectIdentity, input.Draft.ChangeID)
	if readErr == nil {
		if stored.Version != 2 || stored.Disposition != "" || nativeRecordDraftHash(stored) != record.DraftHash || stored.AfterHash != "" {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
		if index.Pending == input.Draft.ChangeID {
			return s.nativeStatus(ctx, target, stored)
		}
		if index.Preparing == nil || index.Preparing.Draft.ChangeID != input.Draft.ChangeID {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
		record = stored
	} else if !errors.Is(readErr, editing.ErrOperationNotFound) {
		return editing.NativeChangeStatus{}, readErr
	}
	if index.Pending != "" || index.Preparing != nil && index.Preparing.Draft.ChangeID != input.Draft.ChangeID {
		return editing.NativeChangeStatus{}, editing.ErrConflict
	}
	if err = s.nativeCurrentUndoSettled(target, index); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return editing.NativeChangeStatus{}, err
	}
	if digestAtomicText(state.Content) != input.Draft.BaseRevision {
		return editing.NativeChangeStatus{}, editing.ErrConflict
	}
	if s.validateNativeObject(state.Content) != nil {
		return editing.NativeChangeStatus{}, editing.ErrNotText
	}
	if index.Preparing == nil {
		reservation := nativeCompact(record, "")
		index.Preparing = &reservation
		if err = s.storeNativeIndex(index, &indexHash); err != nil {
			return editing.NativeChangeStatus{}, err
		}
	} else if index.Preparing.DraftHash != record.DraftHash {
		return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
	}
	if errors.Is(readErr, editing.ErrOperationNotFound) {
		if err = s.writeNativeRecord(record, ""); err != nil {
			return editing.NativeChangeStatus{}, err
		}
	}
	path := s.nativePath(input.ObjectIdentity, input.Draft.ChangeID, ".before")
	if original, _, readErr := s.readNativePrivate(path, MaxOfficeObjectBytes); readErr == nil {
		if !bytes.Equal(original, state.Content) {
			return editing.NativeChangeStatus{}, editing.ErrOperationMismatch
		}
	} else if !errors.Is(readErr, editing.ErrOperationNotFound) {
		return editing.NativeChangeStatus{}, readErr
	} else if err = s.writeNativePrivate(path, state.Content, "", MaxOfficeObjectBytes); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	if _, _, err = s.nativeState(target, input.ObjectIdentity); err != nil {
		return editing.NativeChangeStatus{}, err
	}
	return s.nativeStatus(ctx, target, record)
}
