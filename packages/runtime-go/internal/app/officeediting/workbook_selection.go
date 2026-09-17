package officeediting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

func (a *Adapter) captureWorkbook(ctx context.Context, session *nativeSession, scope *nativeScope, value any) error {
	if a.kind != "xlsx" || !scope.Editable {
		return editingapp.ErrScope
	}
	body, err := json.Marshal(value)
	if err != nil {
		return editingapp.ErrScope
	}
	selected, err := office.ParseWorkbookSelection(body)
	if err != nil {
		return editingapp.ErrScope
	}
	if !nativeLiteral(string(body)) {
		return editingapp.ErrProjection
	}
	protected, err := a.projector.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: a.selectionAuthority(session, scope), Text: string(body)})
	if err != nil || len(protected) != 0 {
		return editingapp.ErrProjection
	}
	service, ok := a.service.(NativeRecoveryService)
	if !ok {
		return editingapp.ErrUnavailable
	}
	if _, err = service.ValidateNativeWorkbook(ctx, session.document.SessionID, scope.BaseRevision, selected, nil); errors.Is(err, fileport.ErrTooLarge) {
		return err
	} else if err != nil {
		return editingapp.ErrDraftStale
	}
	scope.Workbook = &selected
	return nil
}
func (a *Adapter) workbookProposal(ctx context.Context, scope *nativeScope, patch *office.WorkbookPatch) (*office.WorkbookReview, error) {
	if scope.Workbook == nil || patch == nil {
		return nil, editingapp.ErrProposal
	}
	body, _ := json.Marshal(patch)
	if !nativeLiteral(string(body)) {
		return nil, editingapp.ErrProjection
	}
	protected, err := a.projector.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: a.selectionAuthority(a.sessions[scope.SessionID], scope), Text: string(body)})
	if err != nil || len(protected) != 0 {
		return nil, editingapp.ErrProjection
	}
	service, ok := a.service.(NativeRecoveryService)
	if !ok {
		return nil, editingapp.ErrUnavailable
	}
	review, err := service.ValidateNativeWorkbook(ctx, scope.SessionID, scope.BaseRevision, *scope.Workbook, patch)
	if errors.Is(err, fileport.ErrTooLarge) {
		return nil, err
	}
	if err != nil || review == nil {
		return nil, editingapp.ErrProposal
	}
	if len(workbookReviewText(review.Before, nil)) > editingapp.MaxSelectionBytes || len(workbookReviewText(review.After, review.Results)) > editingapp.MaxSelectionBytes {
		return nil, editingapp.ErrProposal
	}
	return review, nil
}
func workbookReviewText(s office.WorkbookSelection, results []string) string {
	var b strings.Builder
	b.WriteString(s.SheetName + "\n")
	for i, c := range s.Cells {
		value := c.Text
		if c.ValueType == "formula" {
			value = c.Formula
		} else if c.ValueType == "number" {
			value = fmt.Sprint(c.Value)
		}
		fmt.Fprintf(&b, "%s [%s] %s", office.WorkbookCellAddress(c.Column, c.Row), c.ValueType, value)
		if i < len(results) && results[i] != "" {
			fmt.Fprintf(&b, " (Core calculation: %s)", results[i])
		}
		b.WriteByte('\n')
	}
	return b.String()
}
