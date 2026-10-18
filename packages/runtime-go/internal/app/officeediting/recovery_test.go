package officeediting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

// This fake owns approval correlation; plain fakeEditing intentionally does not
// implement NativeRecoveryService so missing-owner tests cannot pass by fallback.
type fakeNativeEditing struct {
	*fakeEditing
	draft                       fileport.NativeChangeDraft
	version                     int
	origin                      string
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

// Match the real Service: a historical committed receipt conflicts when its
// revision is no longer current, including a save receipt after successful Undo.
func (f *fakeNativeEditing) Status(ctx context.Context, id, operation string) (fileport.Receipt, error) {
	receipt, err := f.fakeEditing.Status(ctx, id, operation)
	if err == nil && receipt.Status == fileport.StatusCommitted && f.document.Revision != receipt.Revision {
		receipt.Status = fileport.StatusConflict
		return receipt, fileport.ErrConflict
	}
	return receipt, err
}
func (f *fakeNativeEditing) changeStatus() fileport.NativeChangeStatus {
	d := f.draft
	status := fileport.NativeChangeStatus{Version: f.version, Origin: f.origin, ChangeID: d.ChangeID, ThreadID: d.ThreadID, ProposalID: d.ProposalID, BaseRevision: d.BaseRevision, BeforeText: d.BeforeText, AfterText: d.AfterText, SaveOperationID: "native_save_" + d.ChangeID, UndoOperationID: "native_undo_" + d.ChangeID, Status: "prepared", CreatedAt: "2026-09-15T00:00:00Z"}
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

// This optional wrapper leaves the legacy approval fake and its readiness intact.
type fakeManualEditing struct{ *fakeNativeEditing }

func (f *fakeManualEditing) PrepareNativeManual(_ context.Context, id string, d fileport.NativeManualDraft) (fileport.NativeChangeStatus, error) {
	if f.prepareErr != nil {
		return fileport.NativeChangeStatus{}, f.prepareErr
	}
	if id != f.document.SessionID {
		return fileport.NativeChangeStatus{}, editingapp.ErrSession
	}
	f.draft = fileport.NativeChangeDraft{ChangeID: d.ChangeID, ThreadID: d.ThreadID, BaseRevision: d.BaseRevision}
	f.prepared = true
	f.version, f.origin = 2, "manual"
	status := f.changeStatus()
	status.Version, status.Origin = 2, "manual"
	return status, nil
}

// This package's diagnostic cases are sequential: restore stderr before any
// assertion can stop the subtest, and never capture document or transport bodies.
func captureManualBeginPhaseV1(t *testing.T, run func() map[string]any) (map[string]any, []byte) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal("diagnostic_pipe_unavailable")
	}
	original := os.Stderr
	defer func() { os.Stderr = original; _ = writer.Close(); _ = reader.Close() }()
	os.Stderr = writer
	out := run()
	os.Stderr = original
	if writer.Close() != nil {
		t.Fatal("diagnostic_writer_close_failed")
	}
	frame, err := io.ReadAll(io.LimitReader(reader, nativeManualBeginPhaseMaxBytesV1+1))
	if err != nil {
		t.Fatal("diagnostic_frame_read_failed")
	}
	return out, frame
}

func TestNativeManualBeginPhaseIsFailureOnlyClosedAndPreservesResult(t *testing.T) {
	for _, name := range []string{"dependencies", "capture_path", "baseline_open", "baseline_session", "baseline_revision", "prepare", "prepared_contract", "success"} {
		t.Run(name, func(t *testing.T) {
			a, base, _, leases := selectionFixture(t)
			fake := &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
			a.service = fake
			// Preserve the real Registry contract that any baseline callback error
			// is hidden by ErrCaptureFailed, without a server/bootstrap fixture.
			a.manualCapture = func(_ context.Context, _, _ string, read func() error) (func(), error) {
				if read() != nil {
					return nil, managededitingapp.ErrCaptureFailed
				}
				*leases++
				return func() { *leases-- }, nil
			}
			request := map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "operationId": "manual_phase_001", "baseRevision": base.document.Revision}
			want := nativeManualBeginPhaseV1{Version: 1, ManualServicePresent: true, CaptureBound: true}
			code := "unavailable"
			switch name {
			case "dependencies":
				a.service = base
				want.Phase, want.ErrorKind, want.ManualServicePresent = "dependencies", "unavailable", false
			case "capture_path":
				a.manualCapture = func(context.Context, string, string, func() error) (func(), error) {
					return nil, managededitingapp.ErrUnsafePath
				}
				want.Phase, want.ErrorKind, want.CaptureEntered = "capture", "unsafe_path", true
			case "baseline_open", "baseline_session", "baseline_revision":
				want.Phase, want.ErrorKind = name, "baseline_failed"
				want.CaptureEntered, want.BaselineEntered = true, true
				want.BaselineOpenOK, want.SessionMatch, want.RevisionMatch = true, true, true
				if name == "baseline_open" {
					base.err = fmt.Errorf("SYNTH_PRIVATE_MANUAL_TRACE: %w", os.ErrPermission)
					want.ErrorKind, want.BaselineOpenOK, want.SessionMatch, want.RevisionMatch = "permission", false, false, false
				} else if name == "baseline_session" {
					base.document.SessionID, want.SessionMatch = strings.Repeat("e", 48), false
				} else {
					base.document.Revision, want.RevisionMatch = strings.Repeat("e", 64), false
				}
			case "prepare", "prepared_contract", "success":
				want.Phase, want.ErrorKind = name, "other"
				want.CaptureEntered, want.BaselineEntered, want.BaselineOpenOK = true, true, true
				want.SessionMatch, want.RevisionMatch, want.CaptureAcquired, want.PrepareEntered = true, true, true, true
				if name == "prepare" {
					fake.prepareErr = errors.New("SYNTH_PRIVATE_MANUAL_TRACE /private/example token=hidden")
				} else if name == "prepared_contract" {
					fake.committed, want.PrepareOK, want.ErrorKind, code = true, true, "conflict", "conflict"
				}
			}
			base.calls = nil
			out, frame := captureManualBeginPhaseV1(t, func() map[string]any { return nativeInvoke(t, a, "begin-manual-edit", request) })
			if name == "success" {
				if out["ok"] != true || len(frame) != 0 || *leases != 1 || !fake.prepared {
					t.Fatal("successful_begin_changed_or_emitted_diagnostic")
				}
				return
			}
			if out["ok"] != false || out["code"] != code || *leases != 0 || len(out) != 3 {
				t.Fatal("diagnostic_changed_response_or_capture_cleanup")
			}
			if len(frame) > nativeManualBeginPhaseMaxBytesV1 || bytes.Count(frame, []byte("\n")) != 1 || !bytes.HasPrefix(frame, []byte(nativeManualBeginPhasePrefixV1)) {
				t.Fatal("diagnostic_not_one_bounded_prefixed_frame")
			}
			for _, private := range []string{"SYNTH_PRIVATE", "/workspace", "/private", request["sessionId"].(string), request["baseRevision"].(string), request["operationId"].(string), "thread_main"} {
				if bytes.Contains(frame, []byte(private)) {
					t.Fatal("diagnostic_leaked_private_value")
				}
			}
			body := bytes.TrimSuffix(bytes.TrimPrefix(frame, []byte(nativeManualBeginPhasePrefixV1)), []byte("\n"))
			var got nativeManualBeginPhaseV1
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()
			var fields map[string]any
			if decoder.Decode(&got) != nil || json.Unmarshal(body, &fields) != nil || len(fields) != 14 || got != want {
				t.Fatal("diagnostic_closed_shape_or_phase_mismatch")
			}
			opens := 0
			for _, call := range base.calls {
				if call == "open" {
					opens++
				}
			}
			if opens > 1 || (want.BaselineEntered && opens != 1) || (!want.BaselineEntered && opens != 0) {
				t.Fatal("diagnostic_added_baseline_reads")
			}
		})
	}
}

// Error text must never be evaluated by the classifier or fixed encoder.
type manualPhasePrivateErrorV1 struct{}

func (manualPhasePrivateErrorV1) Error() string { panic("private_error_text_evaluated") }

type manualPhaseFailingWriterV1 struct{ writes int }

func (w *manualPhaseFailingWriterV1) Write([]byte) (int, error) {
	w.writes++
	return 0, io.ErrClosedPipe
}

func TestNativeManualBeginPhaseEncoderRejectsUnboundedValuesAndIgnoresWriterFailure(t *testing.T) {
	if nativeManualBeginErrorKindV1(manualPhasePrivateErrorV1{}) != "other" || nativeManualBeginErrorKindV1(fmt.Errorf("private: %w", context.Canceled)) != "context_canceled" {
		t.Fatal("diagnostic_error_classification_changed")
	}
	valid := nativeManualBeginPhaseV1{Version: 1, Phase: "prepare", ErrorKind: "other"}
	for _, field := range []string{"phase", "errorKind", "version"} {
		invalid := valid
		switch field {
		case "phase":
			invalid.Phase = strings.Repeat("PRIVATE", 1024)
		case "errorKind":
			invalid.ErrorKind = "PRIVATE"
		case "version":
			invalid.Version = 2
		}
		var buffer bytes.Buffer
		emitNativeManualBeginPhaseV1(&buffer, invalid)
		if buffer.Len() != 0 {
			t.Fatal("unqualified_diagnostic_value_emitted")
		}
	}
	writer := &manualPhaseFailingWriterV1{}
	emitNativeManualBeginPhaseV1(writer, valid)
	if writer.writes != 1 {
		t.Fatal("diagnostic_retried_failed_write")
	}
	// Exercise the actual failure result when the process stderr handle is
	// closed; best-effort diagnostics must not become operation failures.
	a, base, _, leases := selectionFixture(t)
	reader, closedWriter, err := os.Pipe()
	if err != nil {
		t.Fatal("closed_diagnostic_pipe_unavailable")
	}
	if reader.Close() != nil || closedWriter.Close() != nil {
		t.Fatal("diagnostic_pipe_close_failed")
	}
	out := func() map[string]any {
		original := os.Stderr
		os.Stderr = closedWriter
		defer func() { os.Stderr = original }()
		return nativeInvoke(t, a, "begin-manual-edit", map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "operationId": "manual_closed_stderr_001", "baseRevision": base.document.Revision})
	}()
	if !reflect.DeepEqual(out, map[string]any{"ok": false, "code": "unavailable", "message": "The object operation could not be completed."}) || *leases != 0 {
		t.Fatal("failed_diagnostic_write_changed_operation_result")
	}
}

func TestNativeManualBeginRequiresOwnerAndRetainsOnlyUnsettledLease(t *testing.T) {
	a, base, _, leases := selectionFixture(t)
	request := map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "operationId": "manual_begin_001", "baseRevision": base.document.Revision}
	manualCapture := a.manualCapture
	a.manualCapture = nil
	a.service = &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	if out := nativeInvoke(t, a, "begin-manual-edit", request); out["code"] != "unavailable" || *leases != 0 {
		t.Fatal("missing explicit manual binding used strict fallback", out)
	}
	a.manualCapture = manualCapture
	a.service = a.service.(*fakeManualEditing).fakeNativeEditing
	if out := nativeInvoke(t, a, "begin-manual-edit", request); out["code"] != "unavailable" || *leases != 0 {
		t.Fatal("missing manual owner fallback", out)
	}
	fake := &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	a.service = fake
	out := nativeInvoke(t, a, "begin-manual-edit", request)
	if out["ok"] != true || *leases != 1 {
		t.Fatal("begin", out, *leases)
	}
	authority := out["manual"].(map[string]any)
	if authority["origin"] != "manual" || authority["version"] != float64(2) || authority["baseRevision"] != base.document.Revision || authority["saveOperationId"] != "native_save_"+authority["changeId"].(string) {
		t.Fatal("manual correlation", authority)
	}
	if replay := nativeInvoke(t, a, "begin-manual-edit", request); replay["ok"] != true || *leases != 1 {
		t.Fatal("begin replay reacquired lease", replay, *leases)
	}
	other := map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "operationId": "manual_begin_002", "baseRevision": base.document.Revision}
	if out := nativeInvoke(t, a, "begin-manual-edit", other); out["code"] != "conflict" {
		t.Fatal("different begin replaced lease", out)
	}
	fields := nativeCommitFields("docx", []byte("saved manual bytes"))
	fields["threadId"], fields["changeId"], fields["operationId"] = request["threadId"], authority["changeId"], authority["saveOperationId"]
	base.receipt = fileport.Receipt{OperationID: authority["saveOperationId"].(string), Revision: strings.Repeat("e", 64), Status: fileport.StatusCommitted, SavedAt: "2026-10-15T00:00:00Z"}
	if out := nativeInvoke(t, a, "commit-object", fields); out["ok"] != true || *leases != 0 {
		t.Fatal("acknowledged save retained stale inode lease", out, *leases)
	}
}

func beginManualLeaseTest(t *testing.T, a *Adapter, base *fakeEditing, operation string) map[string]any {
	t.Helper()
	out := nativeInvoke(t, a, "begin-manual-edit", map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "operationId": operation, "baseRevision": base.document.Revision})
	if out["ok"] != true {
		t.Fatal("begin manual lease", out)
	}
	return out["manual"].(map[string]any)
}

func TestNativeManualOldSaveStatusCannotReleaseNewCapture(t *testing.T) {
	a, base, _, leases := selectionFixture(t)
	fake := &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	a.service = fake
	first := beginManualLeaseTest(t, a, base, "manual_first_001")
	fields := nativeCommitFields("docx", []byte("first saved manual bytes"))
	fields["changeId"], fields["operationId"] = first["changeId"], first["saveOperationId"]
	base.receipt = fileport.Receipt{OperationID: first["saveOperationId"].(string), Revision: strings.Repeat("e", 64), Status: fileport.StatusCommitted, SavedAt: "2026-10-15T00:00:00Z"}
	if out := nativeInvoke(t, a, "commit-object", fields); out["ok"] != true || *leases != 0 {
		t.Fatal("first save", out, *leases)
	}
	// The single-draft fake starts the next transaction; retain A's receipt for
	// the late status query. Real continuous-save persistence is covered in FS.
	fake.prepared, fake.committed, fake.undone = false, false, false
	second := beginManualLeaseTest(t, a, base, "manual_second_002")
	out := nativeInvoke(t, a, "object-status", map[string]any{"sessionId": base.document.SessionID, "operationId": first["saveOperationId"]})
	if out["ok"] != true || out["receipt"].(map[string]any)["operationId"] != first["saveOperationId"] {
		t.Fatal("late status lost completed receipt", out)
	}
	session := a.sessions[base.document.SessionID]
	if *leases != 1 || session.manualChangeID != second["changeId"] || session.release == nil {
		t.Fatal("old status retired the new capture", *leases, session.manualChangeID)
	}
	fields = nativeCommitFields("docx", []byte("second saved manual bytes"))
	fields["changeId"], fields["operationId"], fields["baseRevision"] = second["changeId"], second["saveOperationId"], base.document.Revision
	base.receipt = fileport.Receipt{OperationID: second["saveOperationId"].(string), Revision: strings.Repeat("f", 64), Status: fileport.StatusCommitted, SavedAt: "2026-10-15T00:01:00Z"}
	if out := nativeInvoke(t, a, "commit-object", fields); out["ok"] != true || *leases != 0 {
		t.Fatal("own acknowledged save did not release capture", out, *leases)
	}
}

func TestNativeManualOtherThreadRecoveryCannotReleaseCapture(t *testing.T) {
	a, base, _, leases := selectionFixture(t)
	a.service = &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	manual := beginManualLeaseTest(t, a, base, "manual_owner_001")
	out := nativeInvoke(t, a, "object-recovery", map[string]any{"sessionId": base.document.SessionID, "threadId": "other_legal_thread"})
	if out["ok"] != true {
		t.Fatal("other authorized thread query", out)
	}
	recovery := out["recovery"].(map[string]any)
	if recovery["current"] != nil || recovery["pending"] != nil {
		t.Fatal("other thread received manual state", out)
	}
	session := a.sessions[base.document.SessionID]
	if *leases != 1 || session.manualChangeID != manual["changeId"] || session.release == nil {
		t.Fatal("empty foreign-thread recovery retired capture", *leases)
	}
	out = nativeInvoke(t, a, "object-recovery", map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main"})
	if out["ok"] != true || out["recovery"].(map[string]any)["pending"].(map[string]any)["changeId"] != manual["changeId"] || *leases != 1 {
		t.Fatal("owner pending projection", out, *leases)
	}
	out = nativeInvoke(t, a, "cancel-change", map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "changeId": manual["changeId"], "baseRevision": base.document.Revision})
	if out["ok"] != true || *leases != 0 || session.manualChangeID != "" || session.release != nil {
		t.Fatal("own cancellation did not release capture", out, *leases)
	}
}

func TestNativeManualLeaseAllowsDiscussionButCannotAuthorizeAgentEdits(t *testing.T) {
	a, base, _, leases := selectionFixture(t)
	fake := &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	a.service = fake
	manual := beginManualLeaseTest(t, a, base, "manual_purpose_001")
	fields := captureFields(base, false)
	out := nativeInvoke(t, a, "capture-selection", fields)
	if out["ok"] != true || *leases != 1 {
		t.Fatal("manual lease blocked read-only discussion", out)
	}
	scope := out["scope"].(map[string]any)
	if out := nativeInvoke(t, a, "model-selection-read", modelFields(scope)); out["ok"] != true {
		t.Fatal("discussion read lost its route", out)
	}
	fields["editable"] = true
	if out := nativeInvoke(t, a, "capture-selection", fields); out["code"] != "conflict" || *leases != 1 {
		t.Fatal("editable reference inherited manual lease", out)
	}
	// Even a retained editable flag cannot substitute for the strict lease.
	a.scopes[scope["scopeId"].(string)].Editable = true
	model := modelFields(scope)
	model["operationId"], model["parts"] = "agent_proposal_001", []editingapp.PatchPart{{Kind: "literal", Text: "Replacement"}}
	if out := nativeInvoke(t, a, "model-selection-propose", model); out["code"] != "scope_invalid" {
		t.Fatal("AI proposal inherited manual purpose", out)
	}
	// The same in-memory manual lease cannot grant a legacy persisted change.
	fake.version, fake.origin = 0, ""
	commit := nativeCommitFields("docx", []byte("candidate"))
	commit["changeId"], commit["operationId"] = manual["changeId"], manual["saveOperationId"]
	before := len(fake.calls)
	if out := nativeInvoke(t, a, "commit-object", commit); out["code"] != "conflict" {
		t.Fatal("legacy mutation inherited manual lease", out)
	}
	for _, called := range fake.calls[before:] {
		if called == "native-commit" {
			t.Fatal("wrong-purpose mutation reached persistence")
		}
	}
}

func TestNativeMutationAdmissionUsesPersistedPurposeAndBindings(t *testing.T) {
	for _, operation := range []string{"commit-object", "resume-change", "undo-change"} {
		for _, variant := range []string{"manual", "agent_v1", "missing_origin", "wrong_version", "wrong_thread", "wrong_base"} {
			t.Run(operation+"/"+variant, func(t *testing.T) {
				base := &fakeEditing{document: editingapp.Opened{SessionID: strings.Repeat("b", 48), ObjectID: strings.Repeat("d", 64), Path: "/workspace/report.docx", Revision: strings.Repeat("c", 64)}}
				change := strings.Repeat("a", 64)
				fake := &fakeNativeEditing{fakeEditing: base, prepared: true, version: 2, origin: "manual", draft: fileport.NativeChangeDraft{ChangeID: change, ThreadID: "thread_main", BaseRevision: base.document.Revision}}
				base.receipt = fileport.Receipt{OperationID: "native_save_" + change, Revision: strings.Repeat("e", 64), Status: fileport.StatusCommitted}
				input := nativeCommitFields("docx", []byte("candidate"))
				input["changeId"], input["operationId"] = change, base.receipt.OperationID
				if operation != "commit-object" {
					delete(input, "content")
					delete(input, "operationId")
				}
				if operation == "undo-change" {
					fake.committed = true
					base.document.Revision = base.receipt.Revision
					input["baseRevision"] = base.receipt.Revision
				}
				switch variant {
				case "agent_v1":
					fake.version, fake.origin = 0, ""
				case "missing_origin":
					fake.origin = ""
				case "wrong_version":
					fake.version = 1
				case "wrong_thread":
					fake.draft.ThreadID = "thread_other"
				case "wrong_base":
					if operation == "undo-change" {
						input["baseRevision"] = strings.Repeat("f", 64)
					} else {
						fake.draft.BaseRevision = strings.Repeat("f", 64)
					}
				}
				a := New("docx", fake, func(context.Context) bool { return true })
				strictCalls, manualCalls := 0, 0
				if err := a.BindSelectionHost(&nativeTestProjector{}, func(context.Context, string, string, func() error) (func(), error) {
					strictCalls++
					return nil, managededitingapp.ErrUncontainedExecution
				}); err != nil {
					t.Fatal(err)
				}
				if err := a.BindManualHost(func(_ context.Context, _, _ string, read func() error) (func(), error) {
					manualCalls++
					if err := read(); err != nil {
						return nil, err
					}
					return func() {}, nil
				}); err != nil {
					t.Fatal(err)
				}
				if out := nativeInvoke(t, a, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": "report.docx"}}); out["ok"] != true {
					t.Fatal(out)
				}
				out := nativeInvoke(t, a, operation, input)
				if variant == "manual" {
					if out["ok"] != true || manualCalls != 1 || strictCalls != 0 {
						t.Fatal("trusted manual record did not select manual purpose", out, manualCalls, strictCalls)
					}
					if replay := nativeInvoke(t, a, operation, input); replay["ok"] != true || manualCalls != 2 || strictCalls != 0 {
						t.Fatal("manual replay lost persisted purpose binding", replay, manualCalls, strictCalls)
					}
					if operation == "undo-change" {
						input["baseRevision"] = strings.Repeat("f", 64)
						if wrong := nativeInvoke(t, a, operation, input); wrong["ok"] != false || manualCalls != 2 {
							t.Fatal("settled Undo admitted an unrelated save revision", wrong, manualCalls)
						}
						input["baseRevision"] = base.receipt.Revision
						base.document.Revision = strings.Repeat("f", 64)
						callsBefore := len(fake.calls)
						if changed := nativeInvoke(t, a, operation, input); changed["code"] != "conflict" {
							t.Fatal("settled Undo replay ignored external bytes", changed)
						}
						for _, called := range fake.calls[callsBefore:] {
							if called == "native-undo" {
								t.Fatal("external version conflict reached Undo")
							}
						}
					}
				} else {
					if out["ok"] != false || manualCalls != 0 || (variant == "agent_v1" && strictCalls != 1) || (variant != "agent_v1" && strictCalls != 0) {
						t.Fatal("untrusted purpose reached optimistic admission", out, manualCalls, strictCalls)
					}
					for _, called := range fake.calls {
						if called == "native-commit" || called == "native-undo" {
							t.Fatal("rejected authority reached mutation")
						}
					}
				}
			})
		}
	}
}

func TestNativeManualConflictAndUnknownRetainTransaction(t *testing.T) {
	a, base, _, leases := selectionFixture(t)
	fake := &fakeManualEditing{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	a.service = fake
	manual := beginManualLeaseTest(t, a, base, "manual_retained_001")
	input := nativeCommitFields("docx", []byte("manual candidate"))
	input["changeId"], input["operationId"] = manual["changeId"], manual["saveOperationId"]
	fake.onCommit = func() { fake.err = fileport.ErrConflict }
	if out := nativeInvoke(t, a, "commit-object", input); out["code"] != "conflict" {
		t.Fatal("version conflict changed", out)
	}
	fake.err = nil
	fake.onCommit = nil
	fake.receipt = fileport.Receipt{OperationID: manual["saveOperationId"].(string), Status: fileport.StatusUnknown}
	out := nativeInvoke(t, a, "commit-object", input)
	if out["ok"] != true || out["receipt"].(map[string]any)["status"] != "unknown" {
		t.Fatal("unknown became a successful save", out)
	}
	if !fake.prepared || fake.committed || *leases != 1 || a.sessions[base.document.SessionID].leasePurpose != nativeLeaseManual || a.sessions[base.document.SessionID].manualChangeID != manual["changeId"] {
		t.Fatal("conflict/unknown discarded manual draft or authority")
	}
}
