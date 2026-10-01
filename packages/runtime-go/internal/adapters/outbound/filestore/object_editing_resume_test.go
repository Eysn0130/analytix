//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

func nativeResumeFixture(t *testing.T, kind string) (*ObjectEditingFiles, editing.NativeCommitInput, editing.NativeUndoInput, []byte, []byte) {
	t.Helper()
	before := officeEditingNativeBytes(t, kind, "Before resume")
	after := officeEditingNativeBytes(t, kind, "After resume")
	store, workspace, path := officeEditingNativeFixture(t, kind, before)
	commit := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
	draft := nativeRecoveryTestDraft("a", commit.BaseRevision)
	if _, err := store.PrepareNativeChange(context.Background(), editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, Draft: draft}); err != nil {
		t.Fatal(err)
	}
	commit.OperationID = nativeSaveID(draft.ChangeID)
	return store, editing.NativeCommitInput{CommitInput: commit, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID}, editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: draft.BaseRevision}, before, after
}

func TestNativeResumeExplicitOnlyAndIdempotentAcrossRestart(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store, commit, resume, before, after := nativeResumeFixture(t, kind)
			store.replaceDocument = func(atomicTextReplaceRequest) error { return errors.New("synthetic interrupted save") }
			if receipt, err := store.CommitNativeChange(ctx, commit); err == nil || receipt.Status != editing.StatusUnknown {
				t.Fatal("save did not become uncertain", receipt, err)
			}
			candidatePath := store.nativePath(commit.ObjectIdentity, commit.ChangeID, ".after")
			candidate, err := os.ReadFile(candidatePath)
			if err != nil || !bytes.Equal(candidate, after) || objectEditingPrivateFile(candidatePath, MaxOfficeObjectBytes) != nil {
				t.Fatal("exact candidate was not durably captured privately", err)
			}
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			writes := 0
			restarted.replaceDocument = func(request atomicTextReplaceRequest) error { writes++; return atomicReplaceText(request) }
			recovered, err := restarted.NativeRecovery(ctx, resume.ObjectIdentity, resume.Workspace, resume.Path, resume.ThreadID)
			if err != nil || recovered.Pending == nil || !recovered.Pending.CanResume || recovered.Pending.CanCancel || recovered.Pending.Revision != digestAtomicText(after) || recovered.Pending.BaseRevision != digestAtomicText(before) {
				t.Fatal("pending candidate not discoverable", recovered, err)
			}
			if receipt, err := restarted.Status(ctx, commit.ObjectIdentity, commit.OperationID, commit.Workspace, commit.Path); err != nil || receipt.Status != editing.StatusUnknown {
				t.Fatal("ordinary status changed pending semantics", receipt, err)
			}
			if receipt, err := restarted.Commit(ctx, commit.CommitInput); err != nil || receipt.Status != editing.StatusUnknown {
				t.Fatal("ordinary commit retry changed pending semantics", receipt, err)
			}
			if receipt, err := restarted.CommitNativeChange(ctx, commit); err != nil || receipt.Status != editing.StatusUnknown {
				t.Fatal("native commit implicitly resumed", receipt, err)
			}
			if writes != 0 {
				t.Fatal("query/replay wrote the document", writes)
			}
			officeEditingAssertBytes(t, resume.Path, before)
			receipt, err := restarted.ResumeNativeChange(ctx, resume)
			if err != nil || receipt.Status != editing.StatusCommitted || receipt.Revision != digestAtomicText(after) || receipt.OperationID != commit.OperationID || writes != 1 {
				t.Fatal("explicit resume failed", receipt, err, writes)
			}
			again, err := restarted.ResumeNativeChange(ctx, resume)
			if err != nil || again != receipt || writes != 1 {
				t.Fatal("completed resume wrote twice", again, err, writes)
			}
			officeEditingAssertBytes(t, resume.Path, after)
			recovered, err = restarted.NativeRecovery(ctx, resume.ObjectIdentity, resume.Workspace, resume.Path, resume.ThreadID)
			if err != nil || recovered.Pending != nil || recovered.Current == nil || recovered.Current.CanResume || !recovered.Current.CanUndo {
				t.Fatal("resume did not settle current", recovered, err)
			}
		})
	}
}

func TestNativeResumeCandidatePersistenceFailureBoundaries(t *testing.T) {
	for _, stage := range []string{"metadata", "before-candidate", "after-candidate", "before-journal"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			store, commit, resume, before, after := nativeResumeFixture(t, "docx")
			store.replaceJournal = func(request atomicTextReplaceRequest) error {
				candidate := strings.HasSuffix(request.Path, ".after")
				if stage == "metadata" && strings.HasSuffix(request.Path, ".change.json") && bytes.Contains(request.Content, []byte(`"afterHash"`)) || stage == "before-candidate" && candidate || stage == "before-journal" && request.Path == store.recordPath(commit.ObjectIdentity, commit.OperationID) {
					return errors.New("synthetic failure before private persistence")
				}
				if err := atomicReplaceText(request); err != nil {
					return err
				}
				if stage == "after-candidate" && candidate {
					return errors.New("synthetic lost candidate receipt")
				}
				return nil
			}
			if _, err := store.CommitNativeChange(ctx, commit); err == nil {
				t.Fatal("persistence fault not observed")
			}
			if _, err := os.Stat(store.recordPath(commit.ObjectIdentity, commit.OperationID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("journal appeared before durable candidate", err)
			}
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "docx")
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := restarted.NativeRecovery(ctx, resume.ObjectIdentity, resume.Workspace, resume.Path, resume.ThreadID)
			wantResume := stage == "after-candidate" || stage == "before-journal"
			if err != nil || recovered.Pending == nil || recovered.Pending.CanResume != wantResume {
				t.Fatal("partial candidate advertised incorrect capability", recovered, err)
			}
			receipt, err := restarted.ResumeNativeChange(ctx, resume)
			if wantResume {
				if err != nil || receipt.Status != editing.StatusCommitted {
					t.Fatal("durable candidate could not resume without journal", receipt, err)
				}
				officeEditingAssertBytes(t, resume.Path, after)
			} else {
				if err == nil {
					t.Fatal("missing candidate resumed")
				}
				officeEditingAssertBytes(t, resume.Path, before)
			}
		})
	}
}

func TestNativeResumeLostReceiptAfterRenameDoesNotRewrite(t *testing.T) {
	for _, fault := range []string{"journal", "index"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			store, commit, resume, _, after := nativeResumeFixture(t, "xlsx")
			store.replaceJournal = func(request atomicTextReplaceRequest) error {
				if fault == "journal" && request.Path == store.recordPath(commit.ObjectIdentity, commit.OperationID) && bytes.Contains(request.Content, []byte(`"status":"committed"`)) || fault == "index" && request.Path == store.nativePath(commit.ObjectIdentity, "index", ".json") && bytes.Contains(request.Content, []byte(`"current"`)) {
					return errors.New("synthetic lost final receipt")
				}
				return atomicReplaceText(request)
			}
			if _, err := store.CommitNativeChange(ctx, commit); !errors.Is(err, editing.ErrPersistence) {
				t.Fatal("final receipt fault not observed", err)
			}
			officeEditingAssertBytes(t, resume.Path, after)
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "xlsx")
			if err != nil {
				t.Fatal(err)
			}
			restarted.replaceDocument = func(atomicTextReplaceRequest) error {
				t.Error("completed save was rewritten")
				return errors.New("unexpected write")
			}
			if receipt, err := restarted.ResumeNativeChange(ctx, resume); err != nil || receipt.Status != editing.StatusCommitted || receipt.Revision != digestAtomicText(after) {
				t.Fatal("lost receipt was not recovered", receipt, err)
			}
			if receipt, err := restarted.ResumeNativeChange(ctx, resume); err != nil || receipt.Status != editing.StatusCommitted {
				t.Fatal("recovered receipt not idempotent", receipt, err)
			}
		})
	}
}

func TestNativeResumeRejectsMismatchedAuthorityAndUnsafeBytes(t *testing.T) {
	for _, fault := range []string{"thread", "identity", "change", "revision", "path", "candidate", "candidate-mode", "candidate-link", "candidate-size", "original", "target-link", "drift"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			store, commit, resume, before, _ := nativeResumeFixture(t, "pptx")
			store.replaceDocument = func(atomicTextReplaceRequest) error { return errors.New("synthetic pending save") }
			if _, err := store.CommitNativeChange(ctx, commit); err == nil {
				t.Fatal("save fault not observed")
			}
			candidate := store.nativePath(commit.ObjectIdentity, commit.ChangeID, ".after")
			targetPath := resume.Path
			current := before
			var mutationErr error
			switch fault {
			case "thread":
				resume.ThreadID = "wrong-thread"
			case "identity":
				resume.ObjectIdentity = strings.Repeat("f", 64)
			case "change":
				resume.ChangeID = strings.Repeat("f", 64)
			case "revision":
				resume.BaseRevision = strings.Repeat("f", 64)
			case "path":
				resume.Path = "../outside.pptx"
			case "candidate":
				mutationErr = os.WriteFile(candidate, []byte("tampered"), 0600)
			case "candidate-mode":
				mutationErr = os.Chmod(candidate, 0644)
			case "candidate-link":
				mutationErr = os.Link(candidate, candidate+".alias")
			case "candidate-size":
				mutationErr = os.Truncate(candidate, MaxOfficeObjectBytes+1)
			case "original":
				mutationErr = os.WriteFile(store.nativePath(commit.ObjectIdentity, commit.ChangeID, ".before"), []byte("tampered"), 0600)
			case "target-link":
				mutationErr = os.Link(targetPath, targetPath+".alias")
			case "drift":
				current = officeEditingNativeBytes(t, "pptx", "External")
				mutationErr = os.WriteFile(targetPath, current, 0600)
			}
			if mutationErr != nil {
				t.Fatal(mutationErr)
			}
			store.replaceDocument = func(atomicTextReplaceRequest) error {
				t.Error("unsafe resume reached replacement")
				return errors.New("unexpected write")
			}
			if _, err := store.ResumeNativeChange(ctx, resume); err == nil {
				t.Fatal("unsafe resume admitted")
			}
			officeEditingAssertBytes(t, targetPath, current)
			if fault == "drift" {
				if err := os.WriteFile(targetPath, before, 0600); err != nil {
					t.Fatal(err)
				}
				if receipt, err := store.ResumeNativeChange(ctx, resume); !errors.Is(err, editing.ErrConflict) || receipt.Status != editing.StatusConflict {
					t.Fatal("persisted conflict was automatically revived", receipt, err)
				}
			}
		})
	}
}

func TestNativeResumeLegacyRecordsWithoutCandidateRemainQueryAndUndoCompatible(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "committed"}[committed], func(t *testing.T) {
			ctx := context.Background()
			store, commit, resume, before, after := nativeResumeFixture(t, "docx")
			target, err := store.target(resume.Workspace, resume.Path)
			if err != nil {
				t.Fatal(err)
			}
			record, hash, err := store.nativeRecord(target, commit.ObjectIdentity, commit.ChangeID)
			if err != nil {
				t.Fatal(err)
			}
			record.AfterHash = digestAtomicText(after)
			if err = store.writeNativeRecord(record, hash); err != nil {
				t.Fatal(err)
			}
			if !committed {
				store.replaceDocument = func(atomicTextReplaceRequest) error { return errors.New("legacy pending save") }
			}
			_, err = store.Commit(ctx, commit.CommitInput)
			if committed && err != nil || !committed && err == nil {
				t.Fatal("legacy journal setup failed", err)
			}
			recovered, err := store.NativeRecovery(ctx, resume.ObjectIdentity, resume.Workspace, resume.Path, resume.ThreadID)
			if err != nil {
				t.Fatal("legacy record became unreadable", err)
			}
			if committed {
				if recovered.Current == nil || recovered.Current.CanResume || !recovered.Current.CanUndo {
					t.Fatal("legacy committed capability changed", recovered)
				}
				if receipt, err := store.ResumeNativeChange(ctx, resume); err != nil || receipt.Status != editing.StatusCommitted {
					t.Fatal("legacy committed receipt unavailable", receipt, err)
				}
				resume.BaseRevision = digestAtomicText(after)
				if receipt, err := store.UndoNativeChange(ctx, resume); err != nil || receipt.Status != editing.StatusCommitted {
					t.Fatal("legacy undo failed", receipt, err)
				}
			} else {
				if recovered.Pending == nil || recovered.Pending.CanResume {
					t.Fatal("legacy missing candidate advertised resume", recovered)
				}
				if _, err := store.ResumeNativeChange(ctx, resume); err == nil {
					t.Fatal("legacy missing candidate resumed")
				}
			}
			officeEditingAssertBytes(t, resume.Path, before)
		})
	}
}

func TestNativeResumeConcurrentExplicitRequestsWriteOnce(t *testing.T) {
	ctx := context.Background()
	store, commit, resume, _, after := nativeResumeFixture(t, "docx")
	store.replaceDocument = func(atomicTextReplaceRequest) error { return errors.New("synthetic pending save") }
	if _, err := store.CommitNativeChange(ctx, commit); err == nil {
		t.Fatal("pending setup failed")
	}
	writes := 0
	store.replaceDocument = func(request atomicTextReplaceRequest) error { writes++; return atomicReplaceText(request) }
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if receipt, err := store.ResumeNativeChange(ctx, resume); err != nil || receipt.Status != editing.StatusCommitted {
				t.Error("concurrent resume failed", receipt, err)
			}
		}()
	}
	group.Wait()
	if writes != 1 {
		t.Fatal("concurrent resumes repeated replacement", writes)
	}
	officeEditingAssertBytes(t, resume.Path, after)
}

func TestNativeResumeCandidateCollectedWithRetiredChange(t *testing.T) {
	ctx := context.Background()
	store, first, resume, _, after := nativeResumeFixture(t, "docx")
	if _, err := store.CommitNativeChange(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := nativeRecoveryTestDraft("b", digestAtomicText(after))
	if _, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: first.Workspace, Path: first.Path, ObjectIdentity: first.ObjectIdentity, Draft: second}); err != nil {
		t.Fatal(err)
	}
	latest := officeEditingNativeBytes(t, "docx", "Latest update")
	commit := first
	commit.ChangeID, commit.OperationID, commit.BaseRevision, commit.Content = second.ChangeID, nativeSaveID(second.ChangeID), second.BaseRevision, base64.StdEncoding.EncodeToString(latest)
	if _, err := store.CommitNativeChange(ctx, commit); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".before", ".after"} {
		body, err := os.ReadFile(store.nativePath(first.ObjectIdentity, first.ChangeID, suffix))
		if err != nil || len(body) != 0 {
			t.Fatal("retired recovery bytes retained", suffix, err)
		}
	}
	store.replaceDocument = func(atomicTextReplaceRequest) error {
		t.Error("historical save replay rewrote document")
		return errors.New("unexpected write")
	}
	if receipt, err := store.ResumeNativeChange(ctx, resume); err != nil || receipt.Status != editing.StatusCommitted || receipt.Revision != digestAtomicText(after) {
		t.Fatal("historical committed resume lost its receipt", receipt, err)
	}
	officeEditingAssertBytes(t, resume.Path, latest)
}

func TestNativeResumeMissingJournalDoesNotPermitOrdinaryCommitReplay(t *testing.T) {
	for _, candidatePresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "candidate-missing", true: "candidate-durable"}[candidatePresent], func(t *testing.T) {
			ctx := context.Background()
			store, commit, resume, before, after := nativeResumeFixture(t, "docx")
			store.replaceJournal = func(request atomicTextReplaceRequest) error {
				if !candidatePresent && strings.HasSuffix(request.Path, ".after") || candidatePresent && request.Path == store.recordPath(commit.ObjectIdentity, commit.OperationID) {
					return errors.New("synthetic interruption before journal")
				}
				return atomicReplaceText(request)
			}
			if _, err := store.CommitNativeChange(ctx, commit); err == nil {
				t.Fatal("interrupted first request unexpectedly completed")
			}
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "docx")
			if err != nil {
				t.Fatal(err)
			}
			privateWrites, documentWrites := 0, 0
			restarted.replaceJournal = func(request atomicTextReplaceRequest) error { privateWrites++; return atomicReplaceText(request) }
			restarted.replaceDocument = func(request atomicTextReplaceRequest) error { documentWrites++; return atomicReplaceText(request) }
			if receipt, err := restarted.CommitNativeChange(ctx, commit); err != nil || receipt.Status != editing.StatusUnknown || receipt.OperationID != commit.OperationID {
				t.Fatal("repeated first request did not remain unknown", receipt, err)
			}
			if privateWrites != 0 || documentWrites != 0 {
				t.Fatal("ordinary replay created candidate/journal or wrote document", privateWrites, documentWrites)
			}
			if _, err = os.Stat(restarted.recordPath(commit.ObjectIdentity, commit.OperationID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("ordinary replay created a save journal", err)
			}
			officeEditingAssertBytes(t, resume.Path, before)
			recovered, err := restarted.NativeRecovery(ctx, resume.ObjectIdentity, resume.Workspace, resume.Path, resume.ThreadID)
			if err != nil || recovered.Pending == nil || recovered.Pending.CanResume != candidatePresent {
				t.Fatal("candidate absence was hidden by replay", recovered, err)
			}
			receipt, err := restarted.ResumeNativeChange(ctx, resume)
			if candidatePresent {
				if err != nil || receipt.Status != editing.StatusCommitted || documentWrites != 1 {
					t.Fatal("explicit resume did not save durable candidate", receipt, err, documentWrites)
				}
				officeEditingAssertBytes(t, resume.Path, after)
			} else if err == nil || documentWrites != 0 {
				t.Fatal("missing candidate became resumable", err, documentWrites)
			}
		})
	}
}
