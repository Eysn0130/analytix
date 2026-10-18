package officeediting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	docapp "analytix.local/runtime-go/internal/app/documentgeneration"
	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	docport "analytix.local/runtime-go/internal/ports/documentgeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// One fixed failure-only stderr frame. This is diagnostic output, never part of
// HTTP results, readiness, mutation authority or persisted document history.
const nativeManualBeginPhasePrefixV1 = "ANALYTIX_NATIVE_MANUAL_BEGIN_PHASE_V1 "
const nativeManualBeginPhaseMaxBytesV1 = 1024

type nativeManualBeginPhaseV1 struct {
	Version              int    `json:"version"`
	Phase                string `json:"phase"`
	ErrorKind            string `json:"errorKind"`
	ManualServicePresent bool   `json:"manualServicePresent"`
	CaptureBound         bool   `json:"captureBound"`
	CaptureEntered       bool   `json:"captureEntered"`
	CaptureReused        bool   `json:"captureReused"`
	BaselineEntered      bool   `json:"baselineEntered"`
	BaselineOpenOK       bool   `json:"baselineOpenOK"`
	SessionMatch         bool   `json:"sessionMatch"`
	RevisionMatch        bool   `json:"revisionMatch"`
	CaptureAcquired      bool   `json:"captureAcquired"`
	PrepareEntered       bool   `json:"prepareEntered"`
	PrepareOK            bool   `json:"prepareOK"`
}

func nativeManualBeginErrorKindV1(err error) string {
	for _, known := range [...]struct {
		cause error
		kind  string
	}{
		{context.Canceled, "context_canceled"}, {context.DeadlineExceeded, "context_deadline"},
		{managededitingapp.ErrUnavailable, "capture_unavailable"}, {managededitingapp.ErrUnsafePath, "unsafe_path"},
		{managededitingapp.ErrSessionMismatch, "capture_session_mismatch"}, {managededitingapp.ErrUncontainedExecution, "opaque_execution"},
		{managededitingapp.ErrCaptureFailed, "baseline_failed"}, {os.ErrNotExist, "not_found"}, {os.ErrPermission, "permission"},
		{fileport.ErrInvalidInput, "invalid_request"}, {editingapp.ErrSession, "session_invalid"},
		{editingapp.ErrScope, "scope_invalid"}, {editingapp.ErrProjection, "projection_unavailable"},
		{editingapp.ErrDraftStale, "draft_stale"}, {fileport.ErrConflict, "conflict"},
		{fileport.ErrPersistence, "persistence_failure"}, {fileport.ErrOperationMismatch, "operation_mismatch"},
		{fileport.ErrOperationNotFound, "operation_not_found"}, {fileport.ErrForbidden, "forbidden"},
		{fileport.ErrNotText, "not_text"}, {fileport.ErrTooLarge, "too_large"}, {editingapp.ErrCapacity, "capacity"},
		{ErrUnavailable, "unavailable"}, {editingapp.ErrUnavailable, "unavailable"},
	} {
		if errors.Is(err, known.cause) {
			return known.kind
		}
	}
	return "other"
}

func emitNativeManualBeginPhaseV1(output io.Writer, state nativeManualBeginPhaseV1) {
	if output == nil || state.Version != 1 {
		return
	}
	switch state.Phase {
	case "input", "authority", "dependencies", "capture_conflict", "capture", "baseline_open", "baseline_session", "baseline_revision", "prepare", "prepared_contract":
	default:
		return
	}
	switch state.ErrorKind {
	case "invalid_request", "session_invalid", "scope_invalid", "projection_unavailable", "draft_stale", "conflict", "persistence_failure", "operation_mismatch", "operation_not_found", "forbidden", "not_text", "too_large", "capacity", "unavailable", "capture_unavailable", "unsafe_path", "capture_session_mismatch", "opaque_execution", "baseline_failed", "context_canceled", "context_deadline", "not_found", "permission", "other":
	default:
		return
	}
	body, err := json.Marshal(state)
	if err != nil || len(nativeManualBeginPhasePrefixV1)+len(body)+1 > nativeManualBeginPhaseMaxBytesV1 {
		return
	}
	frame := append([]byte(nativeManualBeginPhasePrefixV1), body...)
	frame = append(frame, '\n')
	// One bounded write prevents interleaving fields. A diagnostic write failure
	// is ignored and never changes the operation's result or cleanup.
	_, _ = output.Write(frame)
}

type nativeManualService interface {
	PrepareNativeManual(context.Context, string, fileport.NativeManualDraft) (fileport.NativeChangeStatus, error)
}

func (a *Adapter) BindUserCreator(creator *docapp.UserCreator) error {
	if a == nil || a.kind != "docx" || creator == nil || a.creator != nil || len(a.sessions) != 0 {
		return ErrBinding
	}
	a.creator = creator
	return nil
}
func (a *Adapter) BindUserCreationHost(resolver docapp.UserWorkspaceResolver, codec docport.Codec, acquire func(context.Context) (func(), error)) error {
	if a.kind != "docx" || a.creator == nil {
		return nil
	}
	return a.creator.BindHost(resolver, codec, acquire)
}

// BindManualHost is a separate trusted composition step, never a fallback from
// strict capture refusal or a purpose supplied by protected-local JSON.
func (a *Adapter) BindManualHost(capture func(context.Context, string, string, func() error) (func(), error)) error {
	if a == nil || capture == nil {
		return ErrBinding
	}
	if a.kind != "docx" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.projector == nil || a.manualCapture != nil || len(a.sessions) != 0 {
		return ErrBinding
	}
	a.manualCapture = capture
	return nil
}

// Change identity binds principal, object, thread and begin operation. A late
// callback for another transaction cannot retire the active engine capture.
func (a *Adapter) releaseManualCapture(id, expectedChange string) {
	if session := a.sessions[id]; session != nil && session.leasePurpose == nativeLeaseManual && expectedChange != "" && session.manualChangeID == expectedChange {
		if session.release != nil {
			session.release()
			session.release = nil
		}
		session.manualChangeID, session.manualBaseRevision, session.leasePurpose = "", "", nativeLeaseNone
	}
}
func (a *Adapter) invokeManual(ctx context.Context, call adapterport.Call, input map[string]any) (adapterport.Result, error) {
	_, manualServicePresent := a.service.(nativeManualService)
	trace := nativeManualBeginPhaseV1{Version: 1, ManualServicePresent: manualServicePresent, CaptureBound: a.manualCapture != nil}
	fail := func(phase string, err error) (adapterport.Result, error) {
		if call.Operation == "begin-manual-edit" {
			trace.Phase = phase
			if trace.ErrorKind == "" {
				trace.ErrorKind = nativeManualBeginErrorKindV1(err)
			}
			emitNativeManualBeginPhaseV1(os.Stderr, trace)
		}
		return failure(err)
	}
	if a.kind != "docx" {
		return fail("input", fileport.ErrInvalidInput)
	}
	thread, _ := input["threadId"].(string)
	operation, _ := input["operationId"].(string)
	if !nativeThread.MatchString(thread) || !operationPattern.MatchString(operation) {
		return fail("input", fileport.ErrInvalidInput)
	}
	if call.Operation == "create-object" {
		name, _ := input["fileName"].(string)
		if !exactKeys(input, "threadId", "operationId", "fileName") || a.creator == nil {
			return fail("input", fileport.ErrInvalidInput)
		}
		created, err := a.creator.Create(ctx, call.Principal, thread, operation, name)
		if err != nil {
			var unknown *docport.UserCreationUnknown
			if errors.As(err, &unknown) {
				return failureReceipt(fileport.ErrPersistence, fileport.Receipt{OperationID: unknown.OperationID, Revision: unknown.Revision, Status: fileport.StatusUnknown})
			}
			return fail("input", err)
		}
		return output(map[string]any{"ok": true, "creation": created})
	}
	id, _ := input["sessionId"].(string)
	base, _ := input["baseRevision"].(string)
	if !exactKeys(input, "sessionId", "threadId", "operationId", "baseRevision") || !sessionPattern.MatchString(id) || !revisionPattern.MatchString(base) {
		return fail("input", fileport.ErrInvalidInput)
	}
	if err := a.validateRecoveryThread(ctx, call, id, thread, "edit"); err != nil {
		return fail("authority", err)
	}
	service, ok := a.service.(nativeManualService)
	if !ok || a.manualCapture == nil {
		return fail("dependencies", ErrUnavailable)
	}
	session := a.sessions[id]
	digest := sha256.Sum256([]byte("native-manual/v2\x00" + call.Principal.PrincipalDigest + "\x00" + session.document.ObjectID + "\x00" + thread + "\x00" + operation))
	change := hex.EncodeToString(digest[:])
	// Reuse a lease only for replay of this exact manual begin. An AI capture
	// must be explicitly settled instead of silently becoming manual authority.
	if session.release != nil && (session.leasePurpose != nativeLeaseManual || session.manualChangeID != change || session.manualBaseRevision != base) {
		return fail("capture_conflict", fileport.ErrConflict)
	}
	trace.CaptureReused = session.release != nil
	acquired := false
	if session.release == nil {
		trace.CaptureEntered, trace.Phase = true, "capture"
		release, err := a.manualCapture(ctx, id, session.document.Path, func() error {
			trace.BaselineEntered, trace.Phase = true, "baseline_open"
			current, err := a.service.Open(ctx, session.workspace, session.document.Path)
			trace.BaselineOpenOK = err == nil
			trace.SessionMatch = err == nil && current.SessionID == id
			trace.RevisionMatch = err == nil && current.Revision == base
			if err != nil {
				trace.ErrorKind = nativeManualBeginErrorKindV1(err)
				return editingapp.ErrDraftStale
			}
			if !trace.SessionMatch {
				trace.Phase = "baseline_session"
				return editingapp.ErrDraftStale
			}
			if !trace.RevisionMatch {
				trace.Phase = "baseline_revision"
				return editingapp.ErrDraftStale
			}
			trace.Phase = "capture"
			return nil
		})
		if err != nil {
			return fail(trace.Phase, err)
		}
		trace.CaptureAcquired, trace.ErrorKind = true, ""
		session.release, session.manualChangeID, session.manualBaseRevision, session.leasePurpose, acquired = release, change, base, nativeLeaseManual, true
	}
	trace.PrepareEntered = true
	prepared, err := service.PrepareNativeManual(ctx, id, fileport.NativeManualDraft{ChangeID: change, ThreadID: thread, BaseRevision: base, BeginOperationID: operation})
	trace.PrepareOK = err == nil
	if err != nil {
		if acquired {
			a.releaseManualCapture(id, change)
		}
		return fail("prepare", err)
	}
	if prepared.Version != 2 || prepared.Origin != "manual" || prepared.Status != "prepared" || prepared.ChangeID != change || prepared.ThreadID != thread || prepared.BaseRevision != base {
		if acquired {
			a.releaseManualCapture(id, change)
		}
		return fail("prepared_contract", fileport.ErrConflict)
	}
	a.invalidateSelection(id)
	return output(map[string]any{"ok": true, "manual": map[string]any{"version": 2, "origin": "manual", "changeId": change, "baseRevision": base, "saveOperationId": prepared.SaveOperationID, "undoOperationId": prepared.UndoOperationID}})
}
