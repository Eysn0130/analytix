package officeediting

import (
	"context"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// NativeRecoveryService is an optional native-only extension. A missing durable
// owner never falls back to a Main-supplied original or untracked native commit.
type NativeRecoveryService interface {
	PrepareNativeChange(context.Context, string, fileport.NativeChangeDraft) (fileport.NativeChangeStatus, error)
	NativeRecovery(context.Context, string, string) (fileport.NativeRecovery, error)
	CommitNativeChange(context.Context, string, string, string, string, string, string) (fileport.Receipt, error)
	UndoNativeChange(context.Context, string, string, string, string) (fileport.Receipt, error)
	CancelNativeChange(context.Context, string, string, string, string) (fileport.NativeRecovery, error)
	ResumeNativeChange(context.Context, string, string, string, string) (fileport.Receipt, error)
}

func (a *Adapter) validateRecoveryThread(ctx context.Context, call adapterport.Call, sessionID, thread, purpose string) error {
	session := a.sessions[sessionID]
	if session == nil || !identitydomain.SamePrincipalV1(session.principal, call.Principal) {
		return editingapp.ErrSession
	}
	if !nativeThread.MatchString(thread) || a.projector == nil {
		return editingapp.ErrScope
	}
	return a.projector.ValidateCurrent(ctx, editingapp.ScopeAuthority{Principal: session.principal, ObjectID: session.document.ObjectID, ThreadID: thread, Purpose: purpose, Workspace: session.workspace, Path: session.document.Path})
}
func (a *Adapter) invokeRecovery(ctx context.Context, call adapterport.Call, input map[string]any) (adapterport.Result, error) {
	service, ok := a.service.(NativeRecoveryService)
	if !ok {
		return failure(ErrUnavailable)
	}
	id, _ := input["sessionId"].(string)
	thread, _ := input["threadId"].(string)
	if !sessionPattern.MatchString(id) {
		return failure(fileport.ErrInvalidInput)
	}
	purpose := "discuss"
	if call.Operation == "undo-change" || call.Operation == "cancel-change" || call.Operation == "resume-change" {
		purpose = "edit"
	}
	if err := a.validateRecoveryThread(ctx, call, id, thread, purpose); err != nil {
		return failure(err)
	}
	switch call.Operation {
	case "object-recovery":
		if !exactKeys(input, "sessionId", "threadId") {
			return failure(fileport.ErrInvalidInput)
		}
		recovered, err := service.NativeRecovery(ctx, id, thread)
		if err != nil {
			return failure(err)
		}
		return output(map[string]any{"ok": true, "recovery": recovered})
	case "cancel-change":
		change, _ := input["changeId"].(string)
		revision, _ := input["baseRevision"].(string)
		if !exactKeys(input, "sessionId", "threadId", "changeId", "baseRevision") || !revisionPattern.MatchString(change) || !revisionPattern.MatchString(revision) {
			return failure(fileport.ErrInvalidInput)
		}
		recovered, err := service.CancelNativeChange(ctx, id, thread, change, revision)
		if err != nil {
			return failure(err)
		}
		a.invalidateSelection(id)
		return output(map[string]any{"ok": true, "recovery": recovered})
	case "undo-change", "resume-change":
		change, _ := input["changeId"].(string)
		revision, _ := input["baseRevision"].(string)
		if !exactKeys(input, "sessionId", "threadId", "changeId", "baseRevision") || !revisionPattern.MatchString(change) || !revisionPattern.MatchString(revision) {
			return failure(fileport.ErrInvalidInput)
		}
		replayStatus := "undone"
		if call.Operation == "resume-change" {
			replayStatus = "committed"
		}
		release, err := a.captureNativeMutation(ctx, id, thread, change, revision, replayStatus)
		if err != nil {
			return failure(err)
		}
		defer release()
		var receipt fileport.Receipt
		if call.Operation == "resume-change" {
			receipt, err = service.ResumeNativeChange(ctx, id, thread, change, revision)
		} else {
			receipt, err = service.UndoNativeChange(ctx, id, thread, change, revision)
		}
		if err != nil {
			return failureReceipt(err, receipt)
		}
		a.invalidateSelection(id)
		return output(map[string]any{"ok": true, "receipt": receiptValue(receipt)})
	}
	return failure(fileport.ErrInvalidInput)
}

// Reopened sessions do not inherit selection captures. Every native file write
// participates in the shared managed-file coordinator as well as binary CAS.
func (a *Adapter) captureNativeMutation(ctx context.Context, id, thread, change, revision, replayStatus string) (func(), error) {
	session := a.sessions[id]
	if session == nil || a.capture == nil {
		return nil, editingapp.ErrProjection
	}
	if session.release != nil {
		return func() {}, nil
	}
	return a.capture(ctx, id, session.document.Path, func() error {
		current, err := a.service.Open(ctx, session.workspace, session.document.Path)
		if err != nil {
			return editingapp.ErrDraftStale
		}
		if current.Revision == revision {
			return nil
		}
		service, ok := a.service.(NativeRecoveryService)
		if !ok {
			return editingapp.ErrProjection
		}
		recovered, err := service.NativeRecovery(ctx, id, thread)
		if err != nil || recovered.Current == nil || recovered.Current.ChangeID != change || recovered.Current.Status != replayStatus {
			return editingapp.ErrDraftStale
		}
		expected := recovered.Current.Revision
		if replayStatus == "undone" {
			expected = recovered.Current.BaseRevision
		}
		if current.Revision != expected {
			return editingapp.ErrDraftStale
		}
		return nil
	})
}
