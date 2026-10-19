package filestore

import (
	"context"
	"errors"

	"analytix.local/runtime-go/internal/adapters/outbound/workbookcodec"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

func (s *ObjectEditingFiles) nativeWorkbookEditingBytes(input editing.NativeWorkbookEditingInput) ([]byte, error) {
	if s.officeKind != "xlsx" || !objectEditingHash(input.ObjectIdentity) || !objectEditingHash(input.BaseRevision) {
		return nil, editing.ErrInvalidInput
	}
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return nil, err
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return nil, err
	}
	if digestAtomicText(state.Content) != input.BaseRevision {
		return nil, editing.ErrConflict
	}
	return state.Content, nil
}

// Eligibility reads the original through the live object authority. Its static
// failure code does not disclose workbook content or package member names.
func (s *ObjectEditingFiles) NativeWorkbookEditing(ctx context.Context, input editing.NativeWorkbookEditingInput) error {
	if err := objectEditingLock(ctx); err != nil {
		return err
	}
	defer func() { <-objectEditingGate }()
	body, err := s.nativeWorkbookEditingBytes(input)
	if err != nil {
		return err
	}
	if err = workbookcodec.ValidateNativeEditingPackage(ctx, body); errors.Is(err, workbookcodec.ErrUnsupportedWorkbook) {
		return editing.ErrTooLarge
	} else if err != nil {
		return editing.ErrInvalidInput
	}
	return nil
}

func (s *ObjectEditingFiles) ValidateNativeWorkbookManual(ctx context.Context, input editing.NativeWorkbookManualValidationInput) (*office.WorkbookReview, error) {
	patch, err := office.WorkbookManualPatch(input.Selection, input.WriteTarget, input.Edit)
	if err != nil {
		return nil, editing.ErrInvalidInput
	}
	if err = objectEditingLock(ctx); err != nil {
		return nil, err
	}
	defer func() { <-objectEditingGate }()
	body, err := s.nativeWorkbookEditingBytes(editing.NativeWorkbookEditingInput{Workspace: input.Workspace, Path: input.Path, ObjectIdentity: input.ObjectIdentity, BaseRevision: input.BaseRevision})
	if err != nil {
		return nil, err
	}
	if err = workbookcodec.ValidateNativeEditingPackage(ctx, body); errors.Is(err, workbookcodec.ErrUnsupportedWorkbook) {
		return nil, editing.ErrTooLarge
	} else if err != nil {
		return nil, editing.ErrInvalidInput
	}
	if err = workbookcodec.VerifySelectionPackage(ctx, body, input.Selection); errors.Is(err, workbookcodec.ErrUnsupportedWorkbook) {
		return nil, editing.ErrTooLarge
	} else if err != nil {
		return nil, editing.ErrInvalidInput
	}
	review, err := workbookcodec.ValidatePatch(ctx, input.Selection, patch)
	if err != nil || workbookcodec.ValidateReview(ctx, review) != nil {
		return nil, editing.ErrInvalidInput
	}
	return &review, nil
}
