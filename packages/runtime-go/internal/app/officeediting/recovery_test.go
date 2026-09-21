package officeediting

import (
	"context"
	"reflect"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

// This fake owns approval correlation; plain fakeEditing intentionally does not
// implement NativeRecoveryService so missing-owner tests cannot pass by fallback.
type fakeNativeEditing struct {
	*fakeEditing
	draft                       fileport.NativeChangeDraft
	prepared, committed, undone bool
	prepareErr                  error
	onUndo                      func()
	onCommit                    func()
}

func (f *fakeNativeEditing) PrepareNativeChange(_ context.Context, id string, draft fileport.NativeChangeDraft) (fileport.NativeChangeStatus, error) {
	f.calls = append(f.calls, "native-prepare")
	if f.prepareErr != nil {
		return fileport.NativeChangeStatus{}, f.prepareErr
	}
	if id != f.document.SessionID {
		return fileport.NativeChangeStatus{}, editingapp.ErrSession
	}
	if f.prepared && f.draft != draft {
		return fileport.NativeChangeStatus{}, fileport.ErrOperationMismatch
	}
	f.draft = draft
	f.prepared = true
	return f.changeStatus(), nil
}
func (f *fakeNativeEditing) changeStatus() fileport.NativeChangeStatus {
	d := f.draft
	status := fileport.NativeChangeStatus{ChangeID: d.ChangeID, ThreadID: d.ThreadID, ProposalID: d.ProposalID, BaseRevision: d.BaseRevision, BeforeText: d.BeforeText, AfterText: d.AfterText, SaveOperationID: "native_save_" + d.ChangeID, UndoOperationID: "native_undo_" + d.ChangeID, Status: "prepared", CreatedAt: "2026-09-15T00:00:00Z"}
	status.CanCancel = !f.committed
	if f.committed {
		status.Status = "committed"
		status.Revision = f.receipt.Revision
		status.SavedAt = f.receipt.SavedAt
		status.CanUndo = true
	}
	if f.undone {
		status.Status = "undone"
		status.Revision = d.BaseRevision
		status.CanUndo = false
	}
	return status
}
func (f *fakeNativeEditing) NativeRecovery(_ context.Context, id, thread string) (fileport.NativeRecovery, error) {
	f.calls = append(f.calls, "native-recovery")
	f.args = []string{id, thread}
	if f.err != nil {
		return fileport.NativeRecovery{}, f.err
	}
	if !f.prepared || f.draft.ThreadID != thread {
		return fileport.NativeRecovery{}, nil
	}
	status := f.changeStatus()
	if f.committed {
		return fileport.NativeRecovery{Current: &status}, nil
	}
	return fileport.NativeRecovery{Pending: &status}, nil
}
func (f *fakeNativeEditing) CommitNativeChange(_ context.Context, id, thread, change, operation, revision, content string) (fileport.Receipt, error) {
	if f.onCommit != nil {
		f.onCommit()
	}
	f.calls = append(f.calls, "native-commit")
	f.args = []string{id, thread, change, operation, revision, content}
	if !f.prepared || f.draft.ThreadID != thread || f.draft.ChangeID != change || operation != "native_save_"+change || f.draft.BaseRevision != revision {
		return fileport.Receipt{}, fileport.ErrOperationMismatch
	}
	if f.err == nil && f.receipt.Status == fileport.StatusCommitted {
		f.committed = true
		f.document.Revision = f.receipt.Revision
	}
	return f.receipt, f.err
}
func (f *fakeNativeEditing) UndoNativeChange(_ context.Context, id, thread, change, revision string) (fileport.Receipt, error) {
	if f.onUndo != nil {
		f.onUndo()
	}
	f.calls = append(f.calls, "native-undo")
	f.args = []string{id, thread, change, revision}
	if !f.committed || f.draft.ThreadID != thread || f.draft.ChangeID != change || f.receipt.Revision != revision {
		return fileport.Receipt{}, fileport.ErrOperationMismatch
	}
	if f.err != nil {
		return f.receipt, f.err
	}
	f.undone = true
	f.document.Revision = f.draft.BaseRevision
	return fileport.Receipt{OperationID: "native_undo_" + change, Revision: f.draft.BaseRevision, Status: fileport.StatusCommitted, SavedAt: "2026-09-15T01:00:00Z"}, nil
}
func (f *fakeNativeEditing) ResumeNativeChange(ctx context.Context, id, thread, change, revision string) (fileport.Receipt, error) {
	return f.CommitNativeChange(ctx, id, thread, change, "native_save_"+change, revision, "stored-candidate")
}
func (f *fakeNativeEditing) CancelNativeChange(_ context.Context, id, thread, change, revision string) (fileport.NativeRecovery, error) {
	f.calls = append(f.calls, "native-cancel")
	f.args = []string{id, thread, change, revision}
	if !f.prepared || f.committed || f.draft.ThreadID != thread || f.draft.ChangeID != change || f.draft.BaseRevision != revision {
		return fileport.NativeRecovery{}, fileport.ErrOperationMismatch
	}
	f.prepared = false
	return fileport.NativeRecovery{}, nil
}

func nativeRecoveryFixture(t *testing.T, kind string) (*Adapter, *fakeNativeEditing, *nativeTestProjector) {
	t.Helper()
	base := &fakeEditing{document: editingapp.Opened{SessionID: strings.Repeat("b", 48), ObjectID: strings.Repeat("d", 64), Path: "/workspace/report." + kind, Revision: strings.Repeat("c", 64), Content: "private-base64"}}
	fake := &fakeNativeEditing{fakeEditing: base}
	projector := &nativeTestProjector{}
	a := New(kind, fake, func(context.Context) bool { return true })
	if err := a.BindSelectionHost(projector, func(_ context.Context, _, _ string, read func() error) (func(), error) {
		if err := read(); err != nil {
			return nil, err
		}
		return func() {}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if out := nativeInvoke(t, a, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": "report." + kind}}); out["ok"] != true {
		t.Fatal(out)
	}
	return a, fake, projector
}
func approvedNativeCommit(t *testing.T, a *Adapter, fake *fakeNativeEditing, kind string, raw []byte) map[string]any {
	t.Helper()
	scope := captured(t, a, fake.fakeEditing, true)
	proposal := proposed(t, a, scope)
	out := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal))
	if out["ok"] != true {
		t.Fatal(out)
	}
	replacement := out["replacement"].(map[string]any)
	fields := nativeCommitFields(kind, raw)
	fields["threadId"] = scope["threadId"]
	fields["changeId"] = replacement["changeId"]
	fields["operationId"] = replacement["saveOperationId"]
	fake.receipt.OperationID = fields["operationId"].(string)
	fake.calls = nil
	fake.args = nil
	return fields
}

func TestNativeMissingDurableOwnerCannotApproveOrCommit(t *testing.T) {
	a, base, _, _ := selectionFixture(t)
	a.service = base // Strip only the native durable extension, keeping ordinary preview service.
	scope := captured(t, a, base, true)
	proposal := proposed(t, a, scope)
	if out := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal)); out["code"] != "unavailable" {
		t.Fatal(out)
	}
	if a.scopes[scope["scopeId"].(string)].proposals[proposal["proposalId"].(string)].Status != "proposed" {
		t.Fatal("missing owner advanced approval")
	}
	base.calls = nil
	for operation, input := range map[string]any{
		"commit-object":   nativeCommitFields("docx", []byte("a")),
		"object-recovery": map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main"},
		"undo-change":     map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "changeId": strings.Repeat("a", 64), "baseRevision": base.document.Revision},
		"resume-change":   map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "changeId": strings.Repeat("a", 64), "baseRevision": base.document.Revision},
		"cancel-change":   map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "changeId": strings.Repeat("a", 64), "baseRevision": base.document.Revision},
	} {
		if out := nativeInvoke(t, a, operation, input); out["code"] != "unavailable" {
			t.Fatal("missing owner fallback", operation, out)
		}
	}
	if len(base.calls) != 0 {
		t.Fatal("missing owner used generic persistence", base.calls)
	}
}

func TestNativePrepareFailureDoesNotApproveOrReleaseReplacement(t *testing.T) {
	a, fake, _ := nativeRecoveryFixture(t, "docx")
	scope := captured(t, a, fake.fakeEditing, true)
	proposal := proposed(t, a, scope)
	fake.prepareErr = fileport.ErrPersistence
	if out := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal)); out["code"] != "persistence_failure" || out["replacement"] != nil {
		t.Fatal(out)
	}
	stored := a.scopes[scope["scopeId"].(string)].proposals[proposal["proposalId"].(string)]
	if stored.Status != "proposed" || stored.decisionOperation != "" || fake.prepared {
		t.Fatal("failed preparation consumed approval")
	}
	fake.prepareErr = nil
	if out := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal)); out["ok"] != true {
		t.Fatal("same approval could not retry", out)
	}
}

func TestNativeRecoveryCommitAndUndoStayBoundToApprovedChange(t *testing.T) {
	a, fake, projector := nativeRecoveryFixture(t, "docx")
	if out := nativeInvoke(t, a, "commit-object", nativeCommitFields("docx", []byte("unapproved"))); out["code"] != "operation_mismatch" {
		t.Fatal("unapproved native commit", out)
	}
	fake.receipt = fileport.Receipt{Revision: strings.Repeat("e", 64), Status: fileport.StatusCommitted, SavedAt: "2026-09-15T00:00:00Z"}
	fields := approvedNativeCommit(t, a, fake, "docx", []byte("after"))
	if fake.draft.BeforeText != "Before PRIVATE after" || fake.draft.AfterText != "Updated PRIVATE after" {
		t.Fatal("durable review omitted original/private fields")
	}
	for key, value := range map[string]string{"threadId": "thread_other", "changeId": strings.Repeat("f", 64), "operationId": "native_other_operation"} {
		copy := map[string]any{}
		for k, v := range fields {
			copy[k] = v
		}
		copy[key] = value
		if out := nativeInvoke(t, a, "commit-object", copy); out["code"] != "operation_mismatch" {
			t.Fatal("approval binding bypass", key, out)
		}
	}
	if out := nativeInvoke(t, a, "commit-object", fields); out["ok"] != true {
		t.Fatal(out)
	}
	query := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread_main"}
	out := nativeInvoke(t, a, "object-recovery", query)
	if out["ok"] != true || out["recovery"].(map[string]any)["current"].(map[string]any)["canUndo"] != true {
		t.Fatal(out)
	}
	undo := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread_main", "changeId": fields["changeId"], "baseRevision": fake.receipt.Revision}
	fake.calls = nil
	projector.invalid = true
	if out := nativeInvoke(t, a, "undo-change", undo); out["code"] != "projection_unavailable" || len(fake.calls) != 0 {
		t.Fatal("revoked thread reached undo owner", out)
	}
	projector.invalid = false
	if out := nativeInvoke(t, a, "undo-change", undo); out["ok"] != true || out["receipt"].(map[string]any)["operationId"] != "native_undo_"+fake.draft.ChangeID {
		t.Fatal(out)
	}
	if !reflect.DeepEqual(fake.args, []string{fake.document.SessionID, "thread_main", fake.draft.ChangeID, fake.receipt.Revision}) {
		t.Fatal("undo accepted caller bytes/path", fake.args)
	}
	for _, call := range fake.calls {
		if call == "commit" {
			t.Fatal("undo used untracked generic commit")
		}
	}
}

func TestNativeReopenedUndoHoldsFreshCaptureAndReplaysWithoutNewBytes(t *testing.T) {
	a, fake, _ := nativeRecoveryFixture(t, "docx")
	fake.receipt = fileport.Receipt{Revision: strings.Repeat("e", 64), Status: fileport.StatusCommitted, SavedAt: "2026-09-15T00:00:00Z"}
	fields := approvedNativeCommit(t, a, fake, "docx", []byte("after"))
	if out := nativeInvoke(t, a, "commit-object", fields); out["ok"] != true {
		t.Fatal(out)
	}
	if out := nativeInvoke(t, a, "close-object", map[string]any{"sessionId": fake.document.SessionID}); out["ok"] != true {
		t.Fatal(out)
	}
	a = New("docx", fake, func(context.Context) bool { return true })
	active, captures := 0, 0
	if err := a.BindSelectionHost(&nativeTestProjector{}, func(_ context.Context, id, path string, read func() error) (func(), error) {
		if id != fake.document.SessionID || path != fake.document.Path {
			t.Fatal("reopened capture authority changed")
		}
		captures++
		if err := read(); err != nil {
			return nil, err
		}
		active++
		return func() { active-- }, nil
	}); err != nil {
		t.Fatal(err)
	}
	if out := nativeInvoke(t, a, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": "report.docx"}}); out["ok"] != true {
		t.Fatal(out)
	}
	fake.onUndo = func() {
		if active != 1 {
			t.Fatal("undo did not hold managed capture")
		}
	}
	undo := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread_main", "changeId": fields["changeId"], "baseRevision": fake.receipt.Revision}
	for index := 0; index < 2; index++ {
		if out := nativeInvoke(t, a, "undo-change", undo); out["ok"] != true {
			t.Fatal("reopened undo/replay rejected", out)
		}
		if active != 0 || captures != index+1 {
			t.Fatal("capture did not release", active, captures)
		}
	}
	fake.calls = nil
	a.capture = nil
	if out := nativeInvoke(t, a, "undo-change", undo); out["code"] != "projection_unavailable" || len(fake.calls) != 0 {
		t.Fatal("missing capture owner reached undo", out)
	}
}

func TestNativeReopenedPreparedCommitHoldsCaptureAndReplays(t *testing.T) {
	for _, operation := range []string{"commit-object", "resume-change"} {
		t.Run(operation, func(t *testing.T) {
			a, fake, _ := nativeRecoveryFixture(t, "docx")
			fake.receipt = fileport.Receipt{Revision: strings.Repeat("e", 64), Status: fileport.StatusCommitted, SavedAt: "2026-09-15T00:00:00Z"}
			fields := approvedNativeCommit(t, a, fake, "docx", []byte("after"))
			if out := nativeInvoke(t, a, "close-object", map[string]any{"sessionId": fake.document.SessionID}); out["ok"] != true {
				t.Fatal(out)
			}
			a = New("docx", fake, func(context.Context) bool { return true })
			active, captures := 0, 0
			if err := a.BindSelectionHost(&nativeTestProjector{}, func(_ context.Context, _, _ string, read func() error) (func(), error) {
				captures++
				if err := read(); err != nil {
					return nil, err
				}
				active++
				return func() { active-- }, nil
			}); err != nil {
				t.Fatal(err)
			}
			if out := nativeInvoke(t, a, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": "report.docx"}}); out["ok"] != true {
				t.Fatal(out)
			}
			fake.onCommit = func() {
				if active != 1 {
					t.Fatal("reopened commit did not hold managed capture")
				}
			}
			if operation == "resume-change" {
				delete(fields, "content")
				delete(fields, "operationId")
			}
			for index := 0; index < 2; index++ {
				if out := nativeInvoke(t, a, operation, fields); out["ok"] != true {
					t.Fatal("prepared commit/replay rejected", out)
				}
				if active != 0 || captures != index+1 {
					t.Fatal("commit capture leaked", active, captures)
				}
			}
			fake.document.Revision = strings.Repeat("f", 64)
			fake.calls = nil
			if out := nativeInvoke(t, a, operation, fields); out["ok"] != false || active != 0 {
				t.Fatal("replay accepted unrelated revision", out)
			}
			for _, call := range fake.calls {
				if call == "native-commit" {
					t.Fatal("stale replay reached commit owner")
				}
			}
			a.capture = nil
			fake.calls = nil
			if out := nativeInvoke(t, a, operation, fields); out["code"] != "projection_unavailable" || len(fake.calls) != 0 {
				t.Fatal("missing capture owner reached commit", out)
			}
		})
	}
}

func TestNativePreparedChangeCancellationIsThreadBound(t *testing.T) {
	a, fake, projector := nativeRecoveryFixture(t, "docx")
	fields := approvedNativeCommit(t, a, fake, "docx", []byte("after"))
	cancel := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread_other", "changeId": fields["changeId"], "baseRevision": fake.document.Revision}
	if out := nativeInvoke(t, a, "cancel-change", cancel); out["code"] != "operation_mismatch" || !fake.prepared {
		t.Fatal("cross-thread cancellation", out)
	}
	cancel["threadId"] = "thread_main"
	projector.invalid = true
	if out := nativeInvoke(t, a, "cancel-change", cancel); out["code"] != "projection_unavailable" || !fake.prepared {
		t.Fatal("revoked cancellation", out)
	}
	projector.invalid = false
	if out := nativeInvoke(t, a, "cancel-change", cancel); out["ok"] != true || fake.prepared {
		t.Fatal("explicit cancellation failed", out)
	}
	for _, call := range fake.calls {
		if call == "commit" || call == "native-commit" {
			t.Fatal("cancellation wrote document")
		}
	}
}
