package officeediting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type nativeWorkbookManualService interface {
	NativeWorkbookEditing(context.Context, string, string) error
	ValidateNativeWorkbookManual(context.Context, string, string, office.WorkbookSelection, office.WorkbookWriteTarget, office.WorkbookManualEdit) (*office.WorkbookReview, error)
	PrepareNativeWorkbookManual(context.Context, string, fileport.NativeWorkbookManualDraft) (fileport.NativeChangeStatus, error)
}

// Both operations are protected-local only. Planning uses Core's bounded parser;
// preparing verifies the native capture against the original before recording a
// one-cell write. Neither operation creates or impersonates an Agent proposal.
func (a *Adapter) invokeWorkbookManual(ctx context.Context, call adapterport.Call, input map[string]any) (adapterport.Result, error) {
	if a.kind != "xlsx" {
		return failure(fileport.ErrInvalidInput)
	}
	keys := []string{"sessionId", "threadId", "baseRevision", "writeTarget", "edit"}
	if call.Operation == "prepare-workbook-manual" {
		keys = append(keys, "operationId", "selectionToken", "changeSequence", "workbook")
	}
	if !exactKeys(input, keys...) {
		return failure(fileport.ErrInvalidInput)
	}
	id, _ := input["sessionId"].(string)
	thread, _ := input["threadId"].(string)
	base, _ := input["baseRevision"].(string)
	if !sessionPattern.MatchString(id) || !nativeThread.MatchString(thread) || !revisionPattern.MatchString(base) {
		return failure(fileport.ErrInvalidInput)
	}
	if err := a.validateRecoveryThread(ctx, call, id, thread, "edit"); err != nil {
		return failure(err)
	}
	service, ok := a.service.(nativeWorkbookManualService)
	if !ok || a.manualCapture == nil {
		return failure(ErrUnavailable)
	}
	body, _ := json.Marshal(input["writeTarget"])
	target, err := office.ParseWorkbookWriteTarget(body)
	if err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	body, _ = json.Marshal(input["edit"])
	edit, err := office.ParseWorkbookManualEdit(body)
	if err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	bounds, err := office.PlanWorkbookManual(target, edit)
	if err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	if call.Operation == "plan-workbook-manual" {
		if err = service.NativeWorkbookEditing(ctx, id, base); err != nil {
			return failure(err)
		}
		return output(map[string]any{"ok": true, "dependencyBounds": bounds})
	}
	operation, _ := input["operationId"].(string)
	token, _ := input["selectionToken"].(string)
	sequence, numberOK := input["changeSequence"].(json.Number)
	seq, seqErr := sequence.Int64()
	if !operationPattern.MatchString(operation) || !nativeSelectionToken.MatchString(token) || !numberOK || seqErr != nil || seq < 0 || seq > 9007199254740991 {
		return failure(fileport.ErrInvalidInput)
	}
	body, _ = json.Marshal(input["workbook"])
	selected, err := office.ParseWorkbookSelection(body)
	if err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	if _, err = office.WorkbookManualPatch(selected, target, edit); err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	session := a.sessions[id]
	digest := sha256.Sum256([]byte("native-workbook-manual/v3\x00" + call.Principal.PrincipalDigest + "\x00" + session.document.ObjectID + "\x00" + thread + "\x00" + operation))
	change := hex.EncodeToString(digest[:])
	if session.release != nil && (session.leasePurpose != nativeLeaseManual || session.manualChangeID != change || session.manualBaseRevision != base) {
		return failure(fileport.ErrConflict)
	}
	acquired := false
	if session.release == nil {
		release, err := a.manualCapture(ctx, id, session.document.Path, func() error {
			current, err := a.service.Open(ctx, session.workspace, session.document.Path)
			if err != nil || current.SessionID != id || current.Revision != base {
				return editingapp.ErrDraftStale
			}
			return nil
		})
		if err != nil {
			return failure(err)
		}
		session.release, session.manualChangeID, session.manualBaseRevision, session.leasePurpose, acquired = release, change, base, nativeLeaseManual, true
	}
	fail := func(err error) (adapterport.Result, error) {
		if acquired {
			a.releaseManualCapture(id, change)
		}
		return failure(err)
	}
	review, err := service.ValidateNativeWorkbookManual(ctx, id, base, selected, target, edit)
	if err != nil {
		return fail(err)
	}
	if review == nil {
		return fail(fileport.ErrInvalidInput)
	}
	prepared, err := service.PrepareNativeWorkbookManual(ctx, id, fileport.NativeWorkbookManualDraft{NativeManualDraft: fileport.NativeManualDraft{ChangeID: change, ThreadID: thread, BaseRevision: base, BeginOperationID: operation}, WriteTarget: target, Edit: edit, Workbook: review})
	if err != nil {
		return fail(err)
	}
	if prepared.Version != 3 || prepared.Origin != "manual" || prepared.Status != "prepared" || prepared.ChangeID != change || prepared.ThreadID != thread || prepared.BaseRevision != base || prepared.Workbook == nil {
		return fail(fileport.ErrConflict)
	}
	a.invalidateSelection(id)
	return output(map[string]any{"ok": true, "workbookManual": map[string]any{"version": 3, "origin": "manual", "changeId": change, "baseRevision": base, "saveOperationId": prepared.SaveOperationID, "undoOperationId": prepared.UndoOperationID, "workbook": prepared.Workbook}})
}
