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

var _ editing.CanvasReviewFiles = (*ObjectEditingFiles)(nil)

type canvasReviewRecord struct {
	Version          int                          `json:"version"`
	ObjectIdentity   string                       `json:"objectIdentity"`
	PathBinding      string                       `json:"pathBinding"`
	ThreadID         string                       `json:"threadId"`
	ExpectedRevision string                       `json:"expectedRevision"`
	Intents          []editing.CanvasReviewIntent `json:"intents"`
	UpdatedAt        string                       `json:"updatedAt"`
}

func (s *ObjectEditingFiles) canvasReviewPath(identity, thread string) string {
	return filepath.Join(s.receiptRoot, "canvas-review-"+digestAtomicText([]byte(identity+"\x00"+thread))+".json")
}
func (s *ObjectEditingFiles) canvasReviewRecord(target objectEditingTarget, input editing.CanvasReviewTarget) (canvasReviewRecord, string, error) {
	body, hash, err := s.readNativePrivate(s.canvasReviewPath(input.ObjectIdentity, input.ThreadID), editing.MaxCanvasReviewRecordBytes)
	if errors.Is(err, editing.ErrOperationNotFound) {
		return canvasReviewRecord{Intents: []editing.CanvasReviewIntent{}}, "", nil
	}
	var record canvasReviewRecord
	if err != nil || nativeDecode(body, &record, editing.MaxCanvasReviewRecordBytes) != nil || record.Version != 1 || record.ObjectIdentity != input.ObjectIdentity || record.PathBinding != target.binding || record.ThreadID != input.ThreadID || !editing.ValidCanvasReview(record.ExpectedRevision, record.Intents) {
		return canvasReviewRecord{}, "", editing.ErrPersistence
	}
	stamp, err := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if err != nil || stamp.UTC().Format(time.RFC3339Nano) != record.UpdatedAt {
		return canvasReviewRecord{}, "", editing.ErrPersistence
	}
	return record, hash, nil
}
func canvasReviewValue(input editing.CanvasReviewTarget, record canvasReviewRecord, hash string) editing.CanvasReviewSnapshot {
	return editing.CanvasReviewSnapshot{ObjectID: input.ObjectIdentity, ThreadID: input.ThreadID, Revision: hash, Intents: record.Intents}
}
func (s *ObjectEditingFiles) canvasReviewTarget(input editing.CanvasReviewTarget) (objectEditingTarget, error) {
	if s.officeKind != "canvas" && s.officeKind != "png" {
		return objectEditingTarget{}, editing.ErrForbidden
	}
	return s.annotationTarget(input)
}
func (s *ObjectEditingFiles) ReadCanvasReview(ctx context.Context, input editing.CanvasReviewTarget) (editing.CanvasReviewSnapshot, error) {
	if err := objectEditingLock(ctx); err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.canvasReviewTarget(input)
	if err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	record, hash, err := s.canvasReviewRecord(target, input)
	if err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	if err = ctx.Err(); err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	return canvasReviewValue(input, record, hash), nil
}
func (s *ObjectEditingFiles) WriteCanvasReview(ctx context.Context, input editing.CanvasReviewWriteInput) (editing.CanvasReviewSnapshot, error) {
	if !editing.ValidCanvasReview(input.ExpectedRevision, input.Intents) {
		return editing.CanvasReviewSnapshot{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.canvasReviewTarget(input.CanvasReviewTarget)
	if err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	old, hash, err := s.canvasReviewRecord(target, input.CanvasReviewTarget)
	if err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	// A lost acknowledgement is the same transition, never permission to replace
	// a different successor. Even clearing retains the predecessor CAS record.
	if hash != "" && old.ExpectedRevision == input.ExpectedRevision && reflect.DeepEqual(old.Intents, input.Intents) {
		return canvasReviewValue(input.CanvasReviewTarget, old, hash), nil
	}
	if hash != input.ExpectedRevision {
		return editing.CanvasReviewSnapshot{}, editing.ErrConflict
	}
	record := canvasReviewRecord{Version: 1, ObjectIdentity: input.ObjectIdentity, PathBinding: target.binding, ThreadID: input.ThreadID, ExpectedRevision: hash, Intents: input.Intents, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	body, err := json.Marshal(record)
	if err != nil {
		return editing.CanvasReviewSnapshot{}, editing.ErrPersistence
	}
	if err = ctx.Err(); err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	if err = s.writeNativePrivate(s.canvasReviewPath(input.ObjectIdentity, input.ThreadID), body, hash, editing.MaxCanvasReviewRecordBytes); err != nil {
		return editing.CanvasReviewSnapshot{}, err
	}
	return canvasReviewValue(input.CanvasReviewTarget, record, digestAtomicText(body)), nil
}
