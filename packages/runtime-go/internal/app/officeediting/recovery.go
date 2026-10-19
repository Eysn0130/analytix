package officeediting

import (
	"context"
	"errors"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// NativeRecoveryService is an optional native-only extension. A missing durable
// owner never falls back to a Main-supplied original or untracked native commit.
type NativeRecoveryService interface {
	ValidateNativeWorkbook(context.Context, string, string, office.WorkbookSelection, *office.WorkbookPatch) (*office.WorkbookReview, error)
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
		// An empty projection for another authorized thread is not settlement
		// evidence for the active change.
		if current := recovered.Current; current != nil && (current.Version == 2 && a.kind == "docx" || current.Version == 3 && a.kind == "xlsx") && current.Origin == "manual" && current.ThreadID == thread && (current.Status == "committed" || current.Status == "undone") {
			if session := a.sessions[id]; session != nil && session.manualBaseRevision == current.BaseRevision {
				a.releaseManualCapture(id, current.ChangeID)
			}
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
		a.releaseManualCapture(id, change)
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
		if receipt.Status == fileport.StatusCommitted {
			a.releaseManualCapture(id, change)
		}
		return output(map[string]any{"ok": true, "receipt": receiptValue(receipt)})
	}
	return failure(fileport.ErrInvalidInput)
}

// Mutation purpose comes only from Core's persisted native record. The client
// cannot choose optimistic admission by omitting a proposal or retrying refusal.
func nativeMutationPurpose(state fileport.NativeRecovery, thread, change, revision, replayStatus string) (nativeLeasePurpose, *fileport.NativeChangeStatus, error) {
	var matched *fileport.NativeChangeStatus
	for _, candidate := range []*fileport.NativeChangeStatus{state.Pending, state.Current} {
		if candidate != nil && candidate.ChangeID == change && candidate.ThreadID == thread {
			if matched != nil {
				return nativeLeaseNone, nil, fileport.ErrOperationMismatch
			}
			matched = candidate
		}
	}
	if matched == nil || !revisionPattern.MatchString(matched.BaseRevision) {
		return nativeLeaseNone, nil, fileport.ErrOperationMismatch
	}
	if (matched.Version == 2 || matched.Version == 3) && matched.Origin == "manual" {
		if matched.SaveOperationID != "native_save_"+change || matched.UndoOperationID != "native_undo_"+change ||
			(replayStatus == "committed" && matched.BaseRevision != revision) ||
			(replayStatus == "undone" && matched.Status != "undone" && matched.Revision != revision) {
			return nativeLeaseNone, nil, fileport.ErrOperationMismatch
		}
		return nativeLeaseManual, matched, nil
	}
	if (matched.Version != 0 && matched.Version != 1) || matched.Origin != "" {
		return nativeLeaseNone, nil, fileport.ErrOperationMismatch
	}
	return nativeLeaseStrict, matched, nil
}

// Reopened sessions obtain a fresh purpose-bound lease. Both purposes share
// filesystem identity and the mutation coordinator; native CAS/journal remains
// the final version authority, including conflicts and unknown outcomes.
func (a *Adapter) captureNativeMutation(ctx context.Context, id, thread, change, revision, replayStatus string) (func(), error) {
	session := a.sessions[id]
	service, ok := a.service.(NativeRecoveryService)
	if session == nil || !ok || (a.capture == nil && a.manualCapture == nil) {
		return nil, editingapp.ErrProjection
	}
	// A live strict lease already has the original Agent admission. Preserve
	// that path without rediscovering its persisted purpose. XLSX also has a
	// typed manual variant, so even a strict lease must match its durable change:
	// another thread's pending manual record may be absent from its projection.
	if session.release != nil && session.leasePurpose == nativeLeaseStrict && a.capture != nil && a.kind != "xlsx" {
		return func() {}, nil
	}
	state, err := service.NativeRecovery(ctx, id, thread)
	if err != nil {
		return nil, err
	}
	purpose, record, err := nativeMutationPurpose(state, thread, change, revision, replayStatus)
	if err != nil {
		return nil, err
	}
	capture := a.capture
	if purpose == nativeLeaseManual {
		if record.Version != 2 || a.kind != "docx" {
			if record.Version != 3 || a.kind != "xlsx" {
				return nil, fileport.ErrOperationMismatch
			}
		}
		// Undo's input base is the saved revision, not the original manual base.
		// Bind it to the trusted save journal even after an idempotent Undo has
		// made the visible recovery revision equal the original again.
		if replayStatus == "undone" {
			saved, err := a.service.Status(ctx, id, record.SaveOperationID)
			// Status also checks current bytes. After a completed Undo, the
			// historical save correctly conflicts with the restored original.
			// Only a trusted settled Undo may use that historical hash; capture
			// below must still confirm the original is current before replay.
			undoneReplay := state.Current == record && record.Status == "undone" && errors.Is(err, fileport.ErrConflict) && saved.Status == fileport.StatusConflict
			if err != nil && !undoneReplay {
				return nil, err
			}
			if saved.OperationID != record.SaveOperationID || saved.Revision != revision || saved.Status != fileport.StatusCommitted && !undoneReplay {
				return nil, fileport.ErrOperationMismatch
			}
		}
		capture = a.manualCapture
	}
	if capture == nil {
		return nil, editingapp.ErrProjection
	}
	if session.release != nil {
		if session.leasePurpose != purpose || purpose == nativeLeaseManual && (session.manualChangeID != change || session.manualBaseRevision != record.BaseRevision) {
			return nil, fileport.ErrConflict
		}
		return func() {}, nil
	}
	var baselineConflict bool
	release, captureErr := capture(ctx, id, session.document.Path, func() error {
		current, err := a.service.Open(ctx, session.workspace, session.document.Path)
		if err != nil || current.SessionID != id {
			return editingapp.ErrDraftStale
		}
		if current.Revision == revision {
			return nil
		}
		if state.Current != nil && state.Current == record && record.Status == replayStatus {
			expected := record.Revision
			if replayStatus == "undone" {
				expected = record.BaseRevision
			}
			if current.Revision == expected {
				return nil
			}
		}
		baselineConflict = purpose == nativeLeaseManual
		return editingapp.ErrDraftStale
	})
	if captureErr != nil && baselineConflict {
		return nil, fileport.ErrConflict
	}
	return release, captureErr
}

// NativePresentationService shares the existing native original/recovery owner.
type NativePresentationService interface {
	ValidateNativePresentation(context.Context, string, string, office.PresentationSelection, *office.PresentationPatch) (*office.PresentationReview, error)
}
