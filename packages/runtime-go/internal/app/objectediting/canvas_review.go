package objectediting

import (
	"context"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

func (s *Service) ReadCanvasReview(ctx context.Context, id, thread string) (fileport.CanvasReviewSnapshot, error) {
	return s.canvasReview(ctx, id, thread, nil)
}
func (s *Service) WriteCanvasReview(ctx context.Context, id, thread, expected string, intents []fileport.CanvasReviewIntent) (fileport.CanvasReviewSnapshot, error) {
	if !fileport.ValidCanvasReview(expected, intents) {
		return fileport.CanvasReviewSnapshot{}, fileport.ErrInvalidInput
	}
	return s.canvasReview(ctx, id, thread, &fileport.CanvasReviewWriteInput{ExpectedRevision: expected, Intents: intents})
}

// The Canvas Host checks the current thread/epoch/purpose. The object service
// independently derives the storage target from its live, principal-owned file.
func (s *Service) canvasReview(ctx context.Context, id, thread string, write *fileport.CanvasReviewWriteInput) (fileport.CanvasReviewSnapshot, error) {
	if s == nil {
		return fileport.CanvasReviewSnapshot{}, ErrUnavailable
	}
	if !fileport.ValidAnnotationThread(thread) {
		return fileport.CanvasReviewSnapshot{}, fileport.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.currentLocked(ctx, id)
	if err != nil {
		return fileport.CanvasReviewSnapshot{}, err
	}
	files, ok := s.files.(fileport.CanvasReviewFiles)
	if !ok {
		return fileport.CanvasReviewSnapshot{}, ErrUnavailable
	}
	target := fileport.CanvasReviewTarget{Workspace: current.workspace, Path: current.path, ObjectIdentity: current.objectID, ThreadID: thread}
	var result fileport.CanvasReviewSnapshot
	if write == nil {
		result, err = files.ReadCanvasReview(ctx, target)
	} else {
		write.CanvasReviewTarget = target
		result, err = files.WriteCanvasReview(ctx, *write)
	}
	if s.identity.ValidateCurrent(ctx, current.principal) != nil || ctx.Err() != nil {
		return fileport.CanvasReviewSnapshot{}, ErrUnavailable
	}
	if err != nil {
		return fileport.CanvasReviewSnapshot{}, err
	}
	if result.ObjectID != current.objectID || result.ThreadID != thread {
		return fileport.CanvasReviewSnapshot{}, ErrUnavailable
	}
	return result, nil
}
