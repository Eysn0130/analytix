package filestore

import (
	"context"
	"encoding/base64"
	"errors"

	"analytix.local/runtime-go/internal/adapters/outbound/workbookcodec"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// BuildNativeWorkbookCandidate reads only an already prepared typed change. The
// returned bytes are not a save attempt: CommitNativeChange alone establishes
// AfterHash, the immutable recovery candidate and the existing CAS receipt.
func (s *ObjectEditingFiles) BuildNativeWorkbookCandidate(ctx context.Context, input editing.NativeWorkbookCandidateInput) (editing.NativeWorkbookCandidate, error) {
	var result editing.NativeWorkbookCandidate
	if s == nil || ctx == nil || s.officeKind != "xlsx" || !objectEditingHash(input.ObjectIdentity) || !objectEditingHash(input.ChangeID) || !objectEditingHash(input.BaseRevision) || !nativeRecoveryThread.MatchString(input.ThreadID) || input.OperationID != nativeSaveID(input.ChangeID) {
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
	// Unlike nativeState, this read cannot finish an interrupted preparation or
	// retire another change while constructing a response.
	index, _, err := s.nativeIndex(target, input.ObjectIdentity)
	if err != nil {
		return result, err
	}
	if index.Preparing != nil || len(index.Retiring) != 0 {
		return result, editing.ErrConflict
	}
	if index.Pending != input.ChangeID {
		return result, editing.ErrOperationMismatch
	}
	record, _, err := s.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
	if err != nil {
		return result, err
	}
	if record.Draft.ThreadID != input.ThreadID || record.Draft.BaseRevision != input.BaseRevision || record.AfterHash != "" || record.UndoStarted || record.Disposition != "" {
		return result, editing.ErrOperationMismatch
	}
	if (record.Version != 1 && record.Version != 3) || record.Draft.Workbook == nil || record.Draft.Presentation != nil {
		return result, editing.ErrOperationMismatch
	}
	if _, _, journalErr := s.readRecord(input.ObjectIdentity, input.OperationID); !errors.Is(journalErr, editing.ErrOperationNotFound) {
		if journalErr != nil {
			return result, journalErr
		}
		return result, editing.ErrOperationMismatch
	}
	if err = s.nativeCurrentUndoSettled(target, index); err != nil {
		return result, err
	}
	current, err := s.inspectObject(target.path)
	if err != nil {
		return result, err
	}
	if digestAtomicText(current.Content) != input.BaseRevision {
		return result, editing.ErrConflict
	}
	original, err := s.nativeOriginal(record)
	if err != nil {
		return result, err
	}
	review := *record.Draft.Workbook
	if err = workbookcodec.ValidateReview(ctx, review); err != nil {
		return result, nativeWorkbookCandidateError(ctx, err)
	}
	candidate, err := workbookcodec.ApplyReview(ctx, original, review)
	if err != nil {
		return result, nativeWorkbookCandidateError(ctx, err)
	}
	if len(candidate) == 0 {
		return result, editing.ErrInvalidInput
	}
	if len(candidate) > MaxOfficeObjectBytes {
		return result, editing.ErrTooLarge
	}
	if s.validateNativeObject(candidate) != nil {
		return result, editing.ErrInvalidInput
	}
	if err = workbookcodec.VerifyPatchedPackage(ctx, original, candidate, review); err != nil {
		return result, nativeWorkbookCandidateError(ctx, err)
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	// An external writer does not share objectEditingGate. A changed source is
	// already a conflict here; the later explicit commit still performs its CAS.
	current, err = s.inspectObject(target.path)
	if err != nil {
		return result, err
	}
	if digestAtomicText(current.Content) != input.BaseRevision {
		return result, editing.ErrConflict
	}
	return editing.NativeWorkbookCandidate{
		ChangeID: input.ChangeID, SaveOperationID: input.OperationID, BaseRevision: input.BaseRevision,
		Content: editing.NativeWorkbookCandidateContent{
			Encoding: "base64", Kind: "xlsx", ByteLength: len(candidate),
			SHA256: digestAtomicText(candidate), Data: base64.StdEncoding.EncodeToString(candidate),
		},
	}, nil
}

func nativeWorkbookCandidateError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, workbookcodec.ErrUnsupportedWorkbook) {
		return editing.ErrTooLarge
	}
	return editing.ErrInvalidInput
}
