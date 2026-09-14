package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

var _ editing.AnnotationDraftFiles = (*ObjectEditingFiles)(nil)

// A single current record retains its predecessor revision for lost-ack replay.
// Clearing keeps this small record so a stale writer cannot recreate old text.
type annotationRecord struct {
	Version               int    `json:"version"`
	ObjectIdentity        string `json:"objectIdentity"`
	PathBinding           string `json:"pathBinding"`
	ThreadID              string `json:"threadId"`
	ExpectedDraftRevision string `json:"expectedDraftRevision"`
	Note                  string `json:"note"`
	SourceRevision        string `json:"sourceRevision"`
	UpdatedAt             string `json:"updatedAt"`
}

func (s *ObjectEditingFiles) annotationPath(identity, thread string) string {
	return filepath.Join(s.receiptRoot, "annotation-"+digestAtomicText([]byte(identity+"\x00"+thread))+".json")
}

func (s *ObjectEditingFiles) annotationTarget(input editing.AnnotationDraftTarget) (objectEditingTarget, error) {
	if s.officeKind == "" || !objectEditingHash(input.ObjectIdentity) || !editing.ValidAnnotationThread(input.ThreadID) {
		return objectEditingTarget{}, editing.ErrInvalidInput
	}
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return objectEditingTarget{}, err
	}
	// Recheck the actual object, including its path and single-link policy. The
	// source revision deliberately need not match its current contents.
	if _, err := s.inspectObject(target.path); err != nil {
		return objectEditingTarget{}, err
	}
	return target, nil
}

func (s *ObjectEditingFiles) annotationRecord(target objectEditingTarget, input editing.AnnotationDraftTarget) (annotationRecord, string, error) {
	body, hash, err := s.readNativePrivate(s.annotationPath(input.ObjectIdentity, input.ThreadID), editing.MaxAnnotationRecordBytes)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return annotationRecord{}, "", nil
	}
	var record annotationRecord
	if err != nil || nativeDecode(body, &record, editing.MaxAnnotationRecordBytes) != nil || record.Version != 1 || record.ObjectIdentity != input.ObjectIdentity || record.PathBinding != target.binding || record.ThreadID != input.ThreadID || !editing.ValidAnnotationWrite(record.ExpectedDraftRevision, record.Note, record.SourceRevision) {
		return annotationRecord{}, "", editing.ErrPersistence
	}
	stamp, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil || stamp.UTC().Format(time.RFC3339Nano) != record.UpdatedAt {
		return annotationRecord{}, "", editing.ErrPersistence
	}
	return record, hash, nil
}

func annotationValue(input editing.AnnotationDraftTarget, record annotationRecord, hash string) editing.AnnotationDraft {
	return editing.AnnotationDraft{ObjectID: input.ObjectIdentity, ThreadID: input.ThreadID, DraftRevision: hash, Note: record.Note, SourceRevision: record.SourceRevision, UpdatedAt: record.UpdatedAt}
}

func (s *ObjectEditingFiles) ReadAnnotationDraft(ctx context.Context, input editing.AnnotationDraftTarget) (editing.AnnotationDraft, error) {
	if err := objectEditingLock(ctx); err != nil {
		return editing.AnnotationDraft{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.annotationTarget(input)
	if err != nil {
		return editing.AnnotationDraft{}, err
	}
	record, hash, err := s.annotationRecord(target, input)
	if err != nil {
		return editing.AnnotationDraft{}, err
	}
	if err := ctx.Err(); err != nil {
		return editing.AnnotationDraft{}, err
	}
	return annotationValue(input, record, hash), nil
}

func (s *ObjectEditingFiles) WriteAnnotationDraft(ctx context.Context, input editing.AnnotationDraftWriteInput) (editing.AnnotationDraft, error) {
	if !editing.ValidAnnotationWrite(input.ExpectedDraftRevision, input.Note, input.SourceRevision) {
		return editing.AnnotationDraft{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return editing.AnnotationDraft{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.annotationTarget(input.AnnotationDraftTarget)
	if err != nil {
		return editing.AnnotationDraft{}, err
	}
	old, hash, err := s.annotationRecord(target, input.AnnotationDraftTarget)
	if err != nil {
		return editing.AnnotationDraft{}, err
	}
	if hash != "" && old.ExpectedDraftRevision == input.ExpectedDraftRevision && old.Note == input.Note && old.SourceRevision == input.SourceRevision {
		return annotationValue(input.AnnotationDraftTarget, old, hash), nil
	}
	if hash != input.ExpectedDraftRevision {
		return editing.AnnotationDraft{}, editing.ErrConflict
	}
	record := annotationRecord{Version: 1, ObjectIdentity: input.ObjectIdentity, PathBinding: target.binding, ThreadID: input.ThreadID, ExpectedDraftRevision: hash, Note: input.Note, SourceRevision: input.SourceRevision, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	body, err := json.Marshal(record)
	if err != nil {
		return editing.AnnotationDraft{}, editing.ErrPersistence
	}
	if err := ctx.Err(); err != nil {
		return editing.AnnotationDraft{}, err
	}
	if err := s.writeNativePrivate(s.annotationPath(input.ObjectIdentity, input.ThreadID), body, hash, editing.MaxAnnotationRecordBytes); err != nil {
		return editing.AnnotationDraft{}, err
	}
	return annotationValue(input.AnnotationDraftTarget, record, digestAtomicText(body)), nil
}
