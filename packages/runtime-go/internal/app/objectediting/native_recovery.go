package objectediting

import (
	"context"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

func (s *Service) PrepareNativeChange(ctx context.Context, id string, draft fileport.NativeChangeDraft) (fileport.NativeChangeStatus, error) {
	if s == nil {
		return fileport.NativeChangeStatus{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.NativeChangeStatus{}, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return fileport.NativeChangeStatus{}, ErrUnavailable
	}
	result, err := files.PrepareNativeChange(ctx, fileport.NativeChangeInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, Draft: draft})
	if s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return fileport.NativeChangeStatus{}, ErrUnavailable
	}
	return result, err
}

func (s *Service) NativeRecovery(ctx context.Context, id, thread string) (fileport.NativeRecovery, error) {
	if s == nil {
		return fileport.NativeRecovery{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.NativeRecovery{}, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return fileport.NativeRecovery{}, ErrUnavailable
	}
	result, err := files.NativeRecovery(ctx, current.objectID, current.workspace, current.path, thread)
	if s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return fileport.NativeRecovery{}, ErrUnavailable
	}
	return result, err
}

func (s *Service) CommitNativeChange(ctx context.Context, id, thread, change, operation, baseRevision, content string) (fileport.Receipt, error) {
	if s == nil {
		return fileport.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.Receipt{}, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return fileport.Receipt{}, ErrUnavailable
	}
	receipt, err := files.CommitNativeChange(ctx, fileport.NativeCommitInput{ChangeID: change, ThreadID: thread, CommitInput: fileport.CommitInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, OperationID: operation, BaseRevision: baseRevision, Content: content}})
	if err != nil {
		return receipt, err
	}
	return s.verifyCurrentReceipt(ctx, current, receipt)
}

func (s *Service) UndoNativeChange(ctx context.Context, id, thread, change, baseRevision string) (fileport.Receipt, error) {
	if s == nil {
		return fileport.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.Receipt{}, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return fileport.Receipt{}, ErrUnavailable
	}
	receipt, err := files.UndoNativeChange(ctx, fileport.NativeUndoInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: thread, ChangeID: change, BaseRevision: baseRevision})
	if err != nil {
		return receipt, err
	}
	return s.verifyCurrentReceipt(ctx, current, receipt)
}

func (s *Service) CancelNativeChange(ctx context.Context, id, thread, change, baseRevision string) (fileport.NativeRecovery, error) {
	if s == nil {
		return fileport.NativeRecovery{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.NativeRecovery{}, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return fileport.NativeRecovery{}, ErrUnavailable
	}
	result, err := files.CancelNativeChange(ctx, fileport.NativeUndoInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: thread, ChangeID: change, BaseRevision: baseRevision})
	if s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return fileport.NativeRecovery{}, ErrUnavailable
	}
	return result, err
}

func (s *Service) ResumeNativeChange(ctx context.Context, id, thread, change, baseRevision string) (fileport.Receipt, error) {
	if s == nil {
		return fileport.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.Receipt{}, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return fileport.Receipt{}, ErrUnavailable
	}
	receipt, err := files.ResumeNativeChange(ctx, fileport.NativeUndoInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: thread, ChangeID: change, BaseRevision: baseRevision})
	if err != nil {
		return receipt, err
	}
	return s.verifyCurrentReceipt(ctx, current, receipt)
}

// ValidateNativeWorkbook resolves paths and principal only from the live session.
func (s *Service) ValidateNativeWorkbook(ctx context.Context, id, base string, selected office.WorkbookSelection, patch *office.WorkbookPatch) (*office.WorkbookReview, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	files, ok := s.files.(fileport.NativeRecoveryFiles)
	if !ok {
		return nil, ErrUnavailable
	}
	result, err := files.ValidateNativeWorkbook(ctx, fileport.NativeWorkbookInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, BaseRevision: base, Selection: selected, Patch: patch})
	if s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return nil, ErrUnavailable
	}
	return result, err
}

func (s *Service) ValidateNativePresentation(ctx context.Context, id, base string, selected office.PresentationSelection, patch *office.PresentationPatch) (*office.PresentationReview, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	files, ok := s.files.(fileport.NativePresentationFiles)
	if !ok {
		return nil, ErrUnavailable
	}
	result, err := files.ValidateNativePresentation(ctx, fileport.NativePresentationInput{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, BaseRevision: base, Selection: selected, Patch: patch})
	if s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return nil, ErrUnavailable
	}
	return result, err
}
