package objectediting

import (
	"context"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

func (s *Service) ReadAnnotationDraft(ctx context.Context, id, thread string) (fileport.AnnotationDraft, error) {
	return s.annotationDraft(ctx, id, thread, nil)
}

func (s *Service) WriteAnnotationDraft(ctx context.Context, id, thread, expected, note, source string) (fileport.AnnotationDraft, error) {
	if !fileport.ValidAnnotationWrite(expected, note, source) {
		return fileport.AnnotationDraft{}, fileport.ErrInvalidInput
	}
	return s.annotationDraft(ctx, id, thread, &fileport.AnnotationDraftWriteInput{ExpectedDraftRevision: expected, Note: note, SourceRevision: source})
}

// Thread discuss authorization is checked by the native Host adapter. This
// service independently binds the file owner to the live Core principal/session.
func (s *Service) annotationDraft(ctx context.Context, id, thread string, write *fileport.AnnotationDraftWriteInput) (fileport.AnnotationDraft, error) {
	if s == nil {
		return fileport.AnnotationDraft{}, ErrUnavailable
	}
	if !fileport.ValidAnnotationThread(thread) {
		return fileport.AnnotationDraft{}, fileport.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.AnnotationDraft{}, err
	}
	files, ok := s.files.(fileport.AnnotationDraftFiles)
	if !ok {
		return fileport.AnnotationDraft{}, ErrUnavailable
	}
	target := fileport.AnnotationDraftTarget{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: thread}
	var result fileport.AnnotationDraft
	if write == nil {
		result, err = files.ReadAnnotationDraft(ctx, target)
	} else {
		write.AnnotationDraftTarget = target
		result, err = files.WriteAnnotationDraft(ctx, *write)
	}
	if s.identity.ValidateCurrent(ctx, current.principal) != nil {
		return fileport.AnnotationDraft{}, ErrUnavailable
	}
	if err != nil {
		return fileport.AnnotationDraft{}, err
	}
	if result.ObjectID != current.objectID || result.ThreadID != thread {
		return fileport.AnnotationDraft{}, ErrUnavailable
	}
	return result, nil
}
