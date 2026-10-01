//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

func TestNativeRecoveryDiscoversOriginalAndUndoWithoutRememberedOperation(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			before := officeEditingNativeBytes(t, kind, "Original")
			after := officeEditingNativeBytes(t, kind, "Updated")
			store, workspace, path := officeEditingNativeFixture(t, kind, before)
			input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
			draft := editing.NativeChangeDraft{ChangeID: strings.Repeat("c", 64), ThreadID: "thread-recovery", ProposalID: strings.Repeat("a", 48), BaseRevision: input.BaseRevision, BeforeText: "Original", AfterText: "Updated"}
			prepared, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, Draft: draft})
			if err != nil || prepared.Status != "prepared" || prepared.CanUndo {
				t.Fatal("prepare", prepared, err)
			}
			original, err := os.ReadFile(store.nativePath(input.ObjectIdentity, draft.ChangeID, ".before"))
			if err != nil || !bytes.Equal(original, before) {
				t.Fatal("Core original was not captured", err)
			}
			input.OperationID = prepared.SaveOperationID
			receipt, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: input, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID})
			if err != nil || receipt.Status != editing.StatusCommitted {
				t.Fatal("commit", receipt, err)
			}
			// This instance has no caller-maintained save/undo operation ID.
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := restarted.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, draft.ThreadID)
			if err != nil || recovered.Current == nil || recovered.Pending != nil || !recovered.Current.CanUndo || recovered.Current.AfterText != "Updated" {
				t.Fatal("recovery", recovered, err)
			}
			hidden, err := restarted.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, "other-thread")
			if err != nil || hidden.Current != nil || hidden.Pending != nil {
				t.Fatal("another thread received review text")
			}
			undo := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: recovered.Current.ChangeID, BaseRevision: recovered.Current.Revision}
			receipt, err = restarted.UndoNativeChange(ctx, undo)
			if err != nil || receipt.Status != editing.StatusCommitted {
				t.Fatal("undo", receipt, err)
			}
			officeEditingAssertBytes(t, path, before)
			again, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err = again.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, draft.ThreadID)
			if err != nil || recovered.Current == nil || recovered.Current.Status != "undone" || recovered.Current.CanUndo {
				t.Fatal("undo restart", recovered, err)
			}
			receipt, err = again.UndoNativeChange(ctx, undo)
			if err != nil || receipt.Status != editing.StatusCommitted {
				t.Fatal("same undo was not idempotent", receipt, err)
			}
			officeEditingAssertBytes(t, path, before)
		})
	}
}

func TestNativeRecoveryRepairsLostIndexCommitAndRejectsCorruptOriginal(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "docx", "Original")
	after := officeEditingNativeBytes(t, "docx", "Updated")
	store, workspace, path := officeEditingNativeFixture(t, "docx", before)
	input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
	draft := editing.NativeChangeDraft{ChangeID: strings.Repeat("d", 64), ThreadID: "thread-recovery", ProposalID: strings.Repeat("b", 48), BaseRevision: input.BaseRevision, BeforeText: "Original", AfterText: "Updated"}
	prepared, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
	store.replaceJournal = func(request atomicTextReplaceRequest) error {
		if request.Path == store.nativePath(input.ObjectIdentity, "index", ".json") && bytes.Contains(request.Content, []byte(`"current"`)) {
			return errors.New("synthetic lost final index write")
		}
		return atomicReplaceText(request)
	}
	input.OperationID = prepared.SaveOperationID
	_, err = store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: input, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID})
	if !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("missing uncertain save", err)
	}
	officeEditingAssertBytes(t, path, after)
	restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "docx")
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, draft.ThreadID)
	if err != nil || result.Pending != nil || result.Current == nil || !result.Current.CanUndo {
		t.Fatal("durable journal did not repair index", result, err)
	}
	if err = os.WriteFile(restarted.nativePath(input.ObjectIdentity, draft.ChangeID, ".before"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, draft.ThreadID); !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("corrupt original accepted", err)
	}
	if _, err = restarted.UndoNativeChange(ctx, editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: digestAtomicText(after)}); !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("corrupt original restored", err)
	}
	officeEditingAssertBytes(t, path, after)
}

func TestNativeRecoveryAcceptsWorstCaseCanonicalReviewText(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "docx", "Original")
	after := officeEditingNativeBytes(t, "docx", "Updated")
	store, workspace, path := officeEditingNativeFixture(t, "docx", before)
	input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
	draft := editing.NativeChangeDraft{ChangeID: strings.Repeat("e", 64), ThreadID: "thread-recovery", ProposalID: strings.Repeat("c", 48), BaseRevision: input.BaseRevision, BeforeText: strings.Repeat("<", 65536), AfterText: strings.Repeat(">", 65536)}
	prepared, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, Draft: draft})
	if err != nil {
		t.Fatal("accepted text could not be prepared", err)
	}
	input.OperationID = prepared.SaveOperationID
	if _, err = store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: input, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID}); err != nil {
		t.Fatal("later commit fields exceeded admitted metadata", err)
	}
	recovered, err := store.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, draft.ThreadID)
	if err != nil || recovered.Current == nil || recovered.Current.BeforeText != draft.BeforeText || recovered.Current.AfterText != draft.AfterText {
		t.Fatal("review text changed", err)
	}
}

func nativeRecoveryTestDraft(id string, base string) editing.NativeChangeDraft {
	return editing.NativeChangeDraft{ChangeID: strings.Repeat(id, 64), ThreadID: "thread-recovery", ProposalID: strings.Repeat("a", 48), BaseRevision: base, BeforeText: strings.Repeat("before", 1000), AfterText: strings.Repeat("after", 1000)}
}

func assertNativeRetired(t *testing.T, store *ObjectEditingFiles, workspace, path, identity, change, disposition string) {
	t.Helper()
	target, err := store.target(workspace, path)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := store.nativeRecord(target, identity, change)
	if err != nil || record.Disposition != disposition || record.Draft.BeforeText != "" || record.Draft.AfterText != "" || !objectEditingHash(record.DraftHash) {
		t.Fatal("retirement did not preserve compact replay identity", err)
	}
	body, err := os.ReadFile(store.nativePath(identity, change, ".before"))
	if !errors.Is(err, os.ErrNotExist) && (err != nil || len(body) != 0) {
		t.Fatal("retired original still retained", err)
	}
}

func TestNativeRecoverySupersededPrepareCannotReclaimPending(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "docx", "Original")
	after := officeEditingNativeBytes(t, "docx", "Updated")
	store, workspace, path := officeEditingNativeFixture(t, "docx", before)
	commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
	a := editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: nativeRecoveryTestDraft("a", commit.BaseRevision)}
	b := a
	b.Draft = nativeRecoveryTestDraft("b", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareNativeChange(ctx, b); err != nil {
		t.Fatal(err)
	}
	assertNativeRetired(t, store, workspace, path, commit.ObjectIdentity, a.Draft.ChangeID, "superseded")
	if _, err := store.PrepareNativeChange(ctx, a); !errors.Is(err, editing.ErrOperationMismatch) {
		t.Fatal("old approval reclaimed pending", err)
	}
	commit.OperationID = nativeSaveID(a.Draft.ChangeID)
	if _, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: commit, ChangeID: a.Draft.ChangeID, ThreadID: a.Draft.ThreadID}); !errors.Is(err, editing.ErrOperationMismatch) {
		t.Fatal("superseded approval saved", err)
	}
	recovered, err := store.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, a.Draft.ThreadID)
	if err != nil || recovered.Pending == nil || recovered.Pending.ChangeID != b.Draft.ChangeID || !recovered.Pending.CanCancel {
		t.Fatal("new pending lost", recovered, err)
	}
	cancel := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: b.Draft.ThreadID, ChangeID: b.Draft.ChangeID, BaseRevision: commit.BaseRevision}
	wrong := cancel
	wrong.ThreadID = "other-thread"
	if _, err = store.CancelNativeChange(ctx, wrong); !errors.Is(err, editing.ErrOperationMismatch) {
		t.Fatal("wrong thread cancelled approval", err)
	}
	wrong = cancel
	wrong.BaseRevision = strings.Repeat("f", 64)
	if _, err = store.CancelNativeChange(ctx, wrong); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("stale cancellation admitted", err)
	}
	if recovered, err = store.CancelNativeChange(ctx, cancel); err != nil || recovered.Pending != nil {
		t.Fatal("cancel failed", recovered, err)
	}
	assertNativeRetired(t, store, workspace, path, commit.ObjectIdentity, b.Draft.ChangeID, "cancelled")
	if _, err = store.PrepareNativeChange(ctx, b); !errors.Is(err, editing.ErrOperationMismatch) {
		t.Fatal("cancelled approval reactivated", err)
	}
	if _, err = store.CancelNativeChange(ctx, cancel); err != nil {
		t.Fatal("cancel replay failed", err)
	}
	officeEditingAssertBytes(t, path, before)
}

func TestNativeRecoveryIncompletePreparationCanRetryOrCancel(t *testing.T) {
	for _, stage := range []string{"metadata", "blob", "publish"} {
		for _, retry := range []bool{false, true} {
			t.Run(stage+map[bool]string{false: "-cancel", true: "-retry"}[retry], func(t *testing.T) {
				ctx := context.Background()
				before := officeEditingNativeBytes(t, "xlsx", "Original")
				store, workspace, path := officeEditingNativeFixture(t, "xlsx", before)
				commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(before))
				input := editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: nativeRecoveryTestDraft("c", commit.BaseRevision)}
				store.replaceJournal = func(request atomicTextReplaceRequest) error {
					fail := stage == "metadata" && strings.HasSuffix(request.Path, ".change.json") || stage == "blob" && strings.HasSuffix(request.Path, ".before") || stage == "publish" && request.Path == store.nativePath(commit.ObjectIdentity, "index", ".json") && bytes.Contains(request.Content, []byte(`"pending"`))
					if fail {
						return errors.New("synthetic prepare interruption")
					}
					return atomicReplaceText(request)
				}
				if _, err := store.PrepareNativeChange(ctx, input); !errors.Is(err, editing.ErrPersistence) {
					t.Fatal("fault not observed", err)
				}
				restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "xlsx")
				if err != nil {
					t.Fatal(err)
				}
				recovered, err := restarted.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, input.Draft.ThreadID)
				if err != nil || recovered.Pending == nil || !recovered.Pending.CanCancel {
					t.Fatal("partial preparation not discoverable", recovered, err)
				}
				if retry {
					if prepared, err := restarted.PrepareNativeChange(ctx, input); err != nil || prepared.Status != "prepared" {
						t.Fatal("same approval cannot finish preparation", prepared, err)
					}
				}
				cancel := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: input.Draft.ThreadID, ChangeID: input.Draft.ChangeID, BaseRevision: commit.BaseRevision}
				if _, err = restarted.CancelNativeChange(ctx, cancel); err != nil {
					t.Fatal(err)
				}
				assertNativeRetired(t, restarted, workspace, path, commit.ObjectIdentity, input.Draft.ChangeID, "cancelled")
				if _, err = restarted.PrepareNativeChange(ctx, input); !errors.Is(err, editing.ErrOperationMismatch) {
					t.Fatal("cancelled partial preparation reactivated", err)
				}
				officeEditingAssertBytes(t, path, before)
			})
		}
	}
}

func TestNativeRecoveryRetirementFailureIsResumedBeforeAnotherPrepare(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "pptx", "Original")
	store, workspace, path := officeEditingNativeFixture(t, "pptx", before)
	commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(before))
	a := editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: nativeRecoveryTestDraft("d", commit.BaseRevision)}
	b := a
	b.Draft = nativeRecoveryTestDraft("e", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(ctx, a); err != nil {
		t.Fatal(err)
	}
	store.replaceJournal = func(request atomicTextReplaceRequest) error {
		if strings.HasSuffix(request.Path, ".before") && len(request.Content) == 0 {
			return errors.New("synthetic retirement interruption")
		}
		return atomicReplaceText(request)
	}
	if _, err := store.PrepareNativeChange(ctx, b); !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("cleanup failure not observed", err)
	}
	c := b
	c.Draft = nativeRecoveryTestDraft("f", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(ctx, c); !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("admitted more blobs while cleanup blocked", err)
	}
	if _, err := os.Stat(store.nativePath(commit.ObjectIdentity, c.Draft.ChangeID, ".before")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unbounded partial blob admitted", err)
	}
	restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "pptx")
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, a.Draft.ThreadID)
	if err != nil || result.Pending == nil || result.Pending.ChangeID != b.Draft.ChangeID {
		t.Fatal("retirement restart lost active approval", result, err)
	}
	assertNativeRetired(t, restarted, workspace, path, commit.ObjectIdentity, a.Draft.ChangeID, "superseded")
	if _, err = restarted.PrepareNativeChange(ctx, a); !errors.Is(err, editing.ErrOperationMismatch) {
		t.Fatal("retired replay admitted", err)
	}
}

func TestNativeRecoveryUndoIntentWithoutJournalCanRetryAfterRestart(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "docx", "Original")
	after := officeEditingNativeBytes(t, "docx", "Updated")
	store, workspace, path := officeEditingNativeFixture(t, "docx", before)
	commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
	draft := nativeRecoveryTestDraft("1", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
		t.Fatal(err)
	}
	commit.OperationID = nativeSaveID(draft.ChangeID)
	if _, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: commit, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID}); err != nil {
		t.Fatal(err)
	}
	undo := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: digestAtomicText(after)}
	store.replaceJournal = func(request atomicTextReplaceRequest) error {
		if request.Path == store.recordPath(commit.ObjectIdentity, nativeUndoID(draft.ChangeID)) {
			return errors.New("synthetic failure before undo journal creation")
		}
		return atomicReplaceText(request)
	}
	if _, err := store.UndoNativeChange(ctx, undo); !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("undo failure not observed", err)
	}
	restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "docx")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, draft.ThreadID)
	if err != nil || recovered.Current == nil || !recovered.Current.CanRetryUndo || recovered.Current.CanUndo || recovered.Current.Status != "unknown" || recovered.Current.Revision != digestAtomicText(after) {
		t.Fatal("undo intent cannot be resumed from recovery", recovered, err)
	}
	undo.BaseRevision = recovered.Current.Revision
	if receipt, err := restarted.UndoNativeChange(ctx, undo); err != nil || receipt.Status != editing.StatusCommitted {
		t.Fatal("safe undo retry failed", receipt, err)
	}
	if receipt, err := restarted.UndoNativeChange(ctx, undo); err != nil || receipt.Status != editing.StatusCommitted {
		t.Fatal("completed undo replay failed", receipt, err)
	}
	officeEditingAssertBytes(t, path, before)
}

func TestNativeRecoveryNeverCancelsUncertainOrCommittedSave(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(map[bool]string{false: "original-remains", true: "third-revision"}[drift], func(t *testing.T) {
			ctx := context.Background()
			before := officeEditingNativeBytes(t, "docx", "Original")
			after := officeEditingNativeBytes(t, "docx", "Updated")
			store, workspace, path := officeEditingNativeFixture(t, "docx", before)
			commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
			draft := nativeRecoveryTestDraft("2", commit.BaseRevision)
			if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
				t.Fatal(err)
			}
			commit.OperationID = nativeSaveID(draft.ChangeID)
			store.replaceDocument = func(atomicTextReplaceRequest) error { return errors.New("synthetic unknown save") }
			if _, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: commit, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID}); err == nil {
				t.Fatal("save failure not observed")
			}
			current := before
			if drift {
				current = officeEditingNativeBytes(t, "docx", "External")
				if err := os.WriteFile(path, current, 0600); err != nil {
					t.Fatal(err)
				}
			}
			cancel := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: digestAtomicText(current)}
			if _, err := store.CancelNativeChange(ctx, cancel); !errors.Is(err, editing.ErrConflict) {
				t.Fatal("uncertain save was cancelled", err)
			}
			recovered, err := store.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, draft.ThreadID)
			if err != nil || recovered.Pending == nil || recovered.Pending.CanCancel {
				t.Fatal("uncertain save lost retention", recovered, err)
			}
			original, err := os.ReadFile(store.nativePath(commit.ObjectIdentity, draft.ChangeID, ".before"))
			if err != nil || !bytes.Equal(original, before) {
				t.Fatal("uncertain save original discarded", err)
			}
			officeEditingAssertBytes(t, path, current)
		})
	}
}

func TestNativeRecoveryNewApprovalCannotRetireUnsettledUndo(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "docx", "Original")
	after := officeEditingNativeBytes(t, "docx", "Updated")
	store, workspace, path := officeEditingNativeFixture(t, "docx", before)
	commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
	draft := nativeRecoveryTestDraft("3", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
		t.Fatal(err)
	}
	commit.OperationID = nativeSaveID(draft.ChangeID)
	if _, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: commit, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID}); err != nil {
		t.Fatal(err)
	}
	undo := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: digestAtomicText(after)}
	store.replaceJournal = func(request atomicTextReplaceRequest) error {
		if request.Path == store.recordPath(commit.ObjectIdentity, nativeUndoID(draft.ChangeID)) {
			return errors.New("synthetic undo interruption")
		}
		return atomicReplaceText(request)
	}
	if _, err := store.UndoNativeChange(ctx, undo); err == nil {
		t.Fatal("undo fault not observed")
	}
	store.replaceJournal = nil
	pending := editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: nativeRecoveryTestDraft("4", digestAtomicText(after))}
	pending.Draft.ThreadID = "another-thread"
	if _, err := store.PrepareNativeChange(ctx, pending); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("new approval hid unsettled undo", err)
	}
	original, err := os.ReadFile(store.nativePath(commit.ObjectIdentity, draft.ChangeID, ".before"))
	if err != nil || !bytes.Equal(original, before) {
		t.Fatal("unsettled undo lost original", err)
	}
	// A historical candidate may already have another thread's pending entry.
	// Its existence must suppress both advertised undo actions without exposing
	// that thread's review text or collecting the current original.
	target, err := store.target(workspace, path)
	if err != nil {
		t.Fatal(err)
	}
	index, indexHash, err := store.nativeIndex(target, commit.ObjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	record := nativeRecoveryRecord{Version: 1, ObjectIdentity: commit.ObjectIdentity, PathBinding: target.binding, Kind: "docx", Draft: pending.Draft, CreatedAt: "2026-09-15T00:00:00Z"}
	if err = store.writeNativeRecord(record, ""); err != nil {
		t.Fatal(err)
	}
	if err = store.writeNativePrivate(store.nativePath(commit.ObjectIdentity, pending.Draft.ChangeID, ".before"), after, "", MaxOfficeObjectBytes); err != nil {
		t.Fatal(err)
	}
	index.Pending = pending.Draft.ChangeID
	if err = store.writeNativeIndex(index, indexHash); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, draft.ThreadID)
	if err != nil || recovered.Current == nil || recovered.Pending != nil || recovered.Current.CanUndo || recovered.Current.CanRetryUndo {
		t.Fatal("another thread's pending failed to suppress undo", recovered, err)
	}
	otherCommit := commit
	otherCommit.OperationID = nativeSaveID(pending.Draft.ChangeID)
	otherCommit.BaseRevision = digestAtomicText(after)
	if _, err = store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: otherCommit, ChangeID: pending.Draft.ChangeID, ThreadID: pending.Draft.ThreadID}); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("preexisting pending retired unsettled undo", err)
	}
	if _, err = os.Stat(store.recordPath(commit.ObjectIdentity, otherCommit.OperationID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("blocked commit created a journal", err)
	}
}

func TestNativeRecoveryRetiredCommittedOperationsRemainIdempotent(t *testing.T) {
	for _, undoFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "save", true: "undo"}[undoFirst], func(t *testing.T) {
			ctx := context.Background()
			before := officeEditingNativeBytes(t, "docx", "Original")
			after := officeEditingNativeBytes(t, "docx", "First update")
			latest := officeEditingNativeBytes(t, "docx", "Second update")
			store, workspace, path := officeEditingNativeFixture(t, "docx", before)
			commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
			draft := nativeRecoveryTestDraft("5", commit.BaseRevision)
			if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
				t.Fatal(err)
			}
			commit.OperationID = nativeSaveID(draft.ChangeID)
			if _, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: commit, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID}); err != nil {
				t.Fatal(err)
			}
			undo := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: digestAtomicText(after)}
			base := after
			if undoFirst {
				if _, err := store.UndoNativeChange(ctx, undo); err != nil {
					t.Fatal(err)
				}
				base = before
			}
			second := nativeRecoveryTestDraft("6", digestAtomicText(base))
			if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: second}); err != nil {
				t.Fatal(err)
			}
			secondCommit := commit
			secondCommit.OperationID, secondCommit.BaseRevision, secondCommit.Content = nativeSaveID(second.ChangeID), second.BaseRevision, base64.StdEncoding.EncodeToString(latest)
			if _, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: secondCommit, ChangeID: second.ChangeID, ThreadID: second.ThreadID}); err != nil {
				t.Fatal(err)
			}
			assertNativeRetired(t, store, workspace, path, commit.ObjectIdentity, draft.ChangeID, "superseded")
			var receipt editing.Receipt
			var err error
			if undoFirst {
				receipt, err = store.UndoNativeChange(ctx, undo)
			} else {
				receipt, err = store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: commit, ChangeID: draft.ChangeID, ThreadID: draft.ThreadID})
			}
			if err != nil || receipt.Status != editing.StatusCommitted {
				t.Fatal("retired completed operation lost its receipt", receipt, err)
			}
			officeEditingAssertBytes(t, path, latest)
			recovered, err := store.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, draft.ThreadID)
			if err != nil || recovered.Current == nil || recovered.Current.ChangeID != second.ChangeID || !recovered.Current.CanUndo {
				t.Fatal("historical replay displaced current", recovered, err)
			}
			cancel := undo
			cancel.ChangeID, cancel.BaseRevision = second.ChangeID, digestAtomicText(latest)
			if _, err = store.CancelNativeChange(ctx, cancel); err == nil {
				t.Fatal("committed change was cancelled")
			}
		})
	}
}

func TestNativeRecoveryPreparedConflictCanBeExplicitlyCancelled(t *testing.T) {
	ctx := context.Background()
	before := officeEditingNativeBytes(t, "xlsx", "Original")
	external := officeEditingNativeBytes(t, "xlsx", "External change")
	store, workspace, path := officeEditingNativeFixture(t, "xlsx", before)
	commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(before))
	draft := nativeRecoveryTestDraft("7", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, external, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.NativeRecovery(ctx, commit.ObjectIdentity, workspace, path, draft.ThreadID)
	if err != nil || recovered.Pending == nil || recovered.Pending.Status != "conflict" || !recovered.Pending.CanCancel {
		t.Fatal("unattempted drift cannot be cancelled", recovered, err)
	}
	cancel := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: digestAtomicText(external)}
	if _, err = store.CancelNativeChange(ctx, cancel); err != nil {
		t.Fatal(err)
	}
	assertNativeRetired(t, store, workspace, path, commit.ObjectIdentity, draft.ChangeID, "cancelled")
	officeEditingAssertBytes(t, path, external)
}

func TestNativeRecoveryRetirementRejectsUnsafeOrChangedBlob(t *testing.T) {
	for _, fault := range []string{"mode", "hardlink", "content"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			before := officeEditingNativeBytes(t, "docx", "Original")
			store, workspace, path := officeEditingNativeFixture(t, "docx", before)
			commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(before))
			draft := nativeRecoveryTestDraft("8", commit.BaseRevision)
			if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
				t.Fatal(err)
			}
			blob := store.nativePath(commit.ObjectIdentity, draft.ChangeID, ".before")
			switch fault {
			case "mode":
				if err := os.Chmod(blob, 0644); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(blob, blob+".alias"); err != nil {
					t.Fatal(err)
				}
			case "content":
				if err := os.WriteFile(blob, []byte("tampered"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			draft = nativeRecoveryTestDraft("9", commit.BaseRevision)
			if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); !errors.Is(err, editing.ErrPersistence) {
				t.Fatal("unsafe retirement accepted", err)
			}
			body, err := os.ReadFile(blob)
			if err != nil || len(body) == 0 {
				t.Fatal("unsafe blob was destroyed", err)
			}
			officeEditingAssertBytes(t, path, before)
		})
	}
}
