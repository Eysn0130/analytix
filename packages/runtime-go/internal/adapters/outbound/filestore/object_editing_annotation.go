package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"time"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

var _ editing.AnnotationDraftFiles = (*ObjectEditingFiles)(nil)

// A single current record retains its predecessor revision for lost-ack replay.
// Clearing keeps this small record so a stale writer cannot recreate old text.
type annotationRecord struct {
	Version               int                  `json:"version"`
	ObjectIdentity        string               `json:"objectIdentity"`
	PathBinding           string               `json:"pathBinding"`
	ThreadID              string               `json:"threadId"`
	ExpectedDraftRevision string               `json:"expectedDraftRevision"`
	Note                  string               `json:"note"`
	SourceRevision        string               `json:"sourceRevision"`
	UpdatedAt             string               `json:"updatedAt"`
	Kind                  string               `json:"kind,omitempty"`
	Width                 int                  `json:"width,omitempty"`
	Height                int                  `json:"height,omitempty"`
	Region                *editing.ImageRegion `json:"region,omitempty"`
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
	if err != nil || nativeDecode(body, &record, editing.MaxAnnotationRecordBytes) != nil || record.Version != 1 || record.Kind != "" || record.Width != 0 || record.Height != 0 || record.Region != nil || record.ObjectIdentity != input.ObjectIdentity || record.PathBinding != target.binding || record.ThreadID != input.ThreadID || !editing.ValidAnnotationWrite(record.ExpectedDraftRevision, record.Note, record.SourceRevision) {
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

func (s *ObjectEditingFiles) imageAnnotationTarget(input editing.AnnotationDraftTarget) (objectEditingTarget, editing.ImageDocument, error) {
	if !objectEditingHash(input.ObjectIdentity) || !editing.ValidAnnotationThread(input.ThreadID) {
		return objectEditingTarget{}, editing.ImageDocument{}, editing.ErrInvalidInput
	}
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return target, editing.ImageDocument{}, err
	}
	doc, err := s.readImageTarget(target)
	return target, doc, err
}
func (s *ObjectEditingFiles) imageAnnotationRecord(target objectEditingTarget, input editing.AnnotationDraftTarget) (annotationRecord, string, error) {
	body, hash, err := s.readObjectPrivate(s.annotationPath(input.ObjectIdentity, input.ThreadID), editing.MaxAnnotationRecordBytes)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return annotationRecord{}, "", nil
	}
	var record annotationRecord
	if err != nil || nativeDecode(body, &record, editing.MaxAnnotationRecordBytes) != nil || record.Version != 2 || record.Kind != "image-region" || record.ObjectIdentity != input.ObjectIdentity || record.ThreadID != input.ThreadID || record.PathBinding != target.binding || !editing.ValidImageAnnotationWrite(record.ExpectedDraftRevision, record.SourceRevision, record.Note, record.Region) || !editing.ValidImageRegion(editing.ImageRegion{Width: 1, Height: 1}, record.Width, record.Height) || record.Region != nil && !editing.ValidImageRegion(*record.Region, record.Width, record.Height) {
		return annotationRecord{}, "", editing.ErrPersistence
	}
	stamp, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil || stamp.UTC().Format(time.RFC3339Nano) != record.UpdatedAt {
		return annotationRecord{}, "", editing.ErrPersistence
	}
	return record, hash, nil
}
func imageAnnotationValue(input editing.AnnotationDraftTarget, record annotationRecord, hash string, doc editing.ImageDocument) editing.ImageAnnotation {
	var region *editing.ImageRegion
	if record.Region != nil {
		value := *record.Region
		region = &value
	}
	return editing.ImageAnnotation{ObjectID: input.ObjectIdentity, ThreadID: input.ThreadID, AnnotationRevision: hash, SourceRevision: record.SourceRevision, Width: record.Width, Height: record.Height, Region: region, Note: record.Note, UpdatedAt: record.UpdatedAt, Current: hash != "" && record.SourceRevision == doc.Revision && record.Width == doc.Width && record.Height == doc.Height}
}
func (s *ObjectEditingFiles) ReadImageAnnotation(ctx context.Context, input editing.AnnotationDraftTarget) (editing.ImageAnnotation, error) {
	if err := objectEditingLock(ctx); err != nil {
		return editing.ImageAnnotation{}, err
	}
	defer func() { <-objectEditingGate }()
	target, doc, err := s.imageAnnotationTarget(input)
	if err != nil {
		return editing.ImageAnnotation{}, err
	}
	record, hash, err := s.imageAnnotationRecord(target, input)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return editing.ImageAnnotation{}, err
	}
	return imageAnnotationValue(input, record, hash, doc), nil
}
func (s *ObjectEditingFiles) WriteImageAnnotation(ctx context.Context, input editing.ImageAnnotationWriteInput) (editing.ImageAnnotation, error) {
	if !editing.ValidImageAnnotationWrite(input.ExpectedAnnotationRevision, input.SourceRevision, input.Note, input.Region) {
		return editing.ImageAnnotation{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return editing.ImageAnnotation{}, err
	}
	defer func() { <-objectEditingGate }()
	target, doc, err := s.imageAnnotationTarget(input.AnnotationDraftTarget)
	if err != nil {
		return editing.ImageAnnotation{}, err
	}
	if doc.Revision != input.SourceRevision {
		return editing.ImageAnnotation{}, editing.ErrConflict
	}
	if input.Region != nil && !editing.ValidImageRegion(*input.Region, doc.Width, doc.Height) {
		return editing.ImageAnnotation{}, editing.ErrInvalidInput
	}
	old, hash, err := s.imageAnnotationRecord(target, input.AnnotationDraftTarget)
	if err != nil {
		return editing.ImageAnnotation{}, err
	}
	if hash != "" && old.ExpectedDraftRevision == input.ExpectedAnnotationRevision && old.Note == input.Note && old.SourceRevision == input.SourceRevision && reflect.DeepEqual(old.Region, input.Region) {
		return imageAnnotationValue(input.AnnotationDraftTarget, old, hash, doc), ctx.Err()
	}
	if hash != input.ExpectedAnnotationRevision {
		return editing.ImageAnnotation{}, editing.ErrConflict
	}
	record := annotationRecord{Version: 2, Kind: "image-region", ObjectIdentity: input.ObjectIdentity, PathBinding: target.binding, ThreadID: input.ThreadID, ExpectedDraftRevision: hash, Note: input.Note, SourceRevision: doc.Revision, Width: doc.Width, Height: doc.Height, Region: input.Region, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	body, err := json.Marshal(record)
	if err != nil {
		return editing.ImageAnnotation{}, editing.ErrPersistence
	}
	if err := ctx.Err(); err != nil {
		return editing.ImageAnnotation{}, err
	}
	if err := s.writeObjectPrivate(s.annotationPath(input.ObjectIdentity, input.ThreadID), body, hash, editing.MaxAnnotationRecordBytes); err != nil {
		return editing.ImageAnnotation{}, err
	}
	_, current, err := s.imageAnnotationTarget(input.AnnotationDraftTarget)
	if err != nil || current.Revision != doc.Revision {
		return editing.ImageAnnotation{}, editing.ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return editing.ImageAnnotation{}, err
	}
	return imageAnnotationValue(input.AnnotationDraftTarget, record, digestAtomicText(body), current), nil
}
