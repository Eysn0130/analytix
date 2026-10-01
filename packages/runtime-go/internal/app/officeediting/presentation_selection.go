package officeediting

import (
	"context"
	"encoding/json"
	"fmt"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
)

func (a *Adapter) capturePresentation(ctx context.Context, session *nativeSession, scope *nativeScope, value any) error {
	if a.kind != "pptx" || !scope.Editable {
		return editingapp.ErrScope
	}
	body, err := json.Marshal(value)
	if err != nil {
		return editingapp.ErrScope
	}
	selected, err := office.ParsePresentationSelection(body)
	if err != nil {
		return editingapp.ErrScope
	}
	// The complete snapshot is local authority evidence, not model context.
	// Selected text is projected separately through the existing parts route.
	_, err = a.projector.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: a.selectionAuthority(session, scope), Text: string(body)})
	if err != nil {
		return editingapp.ErrProjection
	}
	service, ok := a.service.(NativePresentationService)
	if !ok {
		return editingapp.ErrUnavailable
	}
	if _, err = service.ValidateNativePresentation(ctx, session.document.SessionID, scope.BaseRevision, selected, nil); err != nil {
		return editingapp.ErrDraftStale
	}
	scope.Presentation = &selected
	return nil
}

func (a *Adapter) presentationProposal(ctx context.Context, scope *nativeScope, patch *office.PresentationPatch) (*office.PresentationReview, error) {
	if scope.Presentation == nil || patch == nil {
		return nil, editingapp.ErrProposal
	}
	body, err := json.Marshal(patch)
	if err != nil || !nativeLiteral(string(body)) {
		return nil, editingapp.ErrProjection
	}
	protected, err := a.projector.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: a.selectionAuthority(a.sessions[scope.SessionID], scope), Text: string(body)})
	if err != nil || len(protected) != 0 {
		return nil, editingapp.ErrProjection
	}
	service, ok := a.service.(NativePresentationService)
	if !ok {
		return nil, editingapp.ErrUnavailable
	}
	review, err := service.ValidateNativePresentation(ctx, scope.SessionID, scope.BaseRevision, *scope.Presentation, patch)
	if err != nil || review == nil || office.ValidatePresentationReview(*review) != nil {
		return nil, editingapp.ErrProposal
	}
	return review, nil
}

func presentationReviewText(s office.PresentationSelection) string {
	shape := s.Shapes[s.TargetShapeIndex]
	return fmt.Sprintf("Slide %d, shape %d (%s)\nPosition: %d, %d; size: %d × %d (1/100 mm)\nSolid fill: %s", s.PageIndex+1, s.TargetShapeIndex+1, shape.Kind, shape.X100thMM, shape.Y100thMM, shape.Width100thMM, shape.Height100thMM, shape.FillRGB)
}

// Geometry/fill proposals need no original names or text. The complete capture
// remains protected-local; this closed projection is the model read surface.
func presentationModelSelection(s office.PresentationSelection) map[string]any {
	shapes := make([]any, 0, len(s.Shapes))
	for _, shape := range s.Shapes {
		shapes = append(shapes, map[string]any{"shapeIndex": shape.ShapeIndex, "kind": shape.Kind,
			"x100thMm": shape.X100thMM, "y100thMm": shape.Y100thMM,
			"width100thMm": shape.Width100thMM, "height100thMm": shape.Height100thMM, "fillRGB": shape.FillRGB})
	}
	return map[string]any{"pageIndex": s.PageIndex, "targetShapeIndex": s.TargetShapeIndex,
		"pageWidth100thMm": s.PageWidth100thMM, "pageHeight100thMm": s.PageHeight100thMM, "shapes": shapes}
}
