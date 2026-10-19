package officeediting

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/workbookcodec"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

// This adapter fake proves protected-local response/decision correlation only.
// It deliberately does not claim a real file patch, native export or save.
func (f *fakeNativeEditing) BuildNativeWorkbookCandidate(_ context.Context, id, thread, change, operation, base string) (fileport.NativeWorkbookCandidate, error) {
	f.calls = append(f.calls, "workbook-build")
	if !f.prepared || f.committed || f.undone || f.draft.Workbook == nil || id != f.document.SessionID || thread != f.draft.ThreadID || change != f.draft.ChangeID || operation != "native_save_"+change || base != f.draft.BaseRevision {
		return fileport.NativeWorkbookCandidate{}, fileport.ErrOperationMismatch
	}
	body, err := base64.StdEncoding.Strict().DecodeString(f.document.Content)
	if err != nil {
		return fileport.NativeWorkbookCandidate{}, err
	}
	digest := sha256.Sum256(body)
	return fileport.NativeWorkbookCandidate{ChangeID: change, SaveOperationID: operation, BaseRevision: base,
		Content: fileport.NativeWorkbookCandidateContent{Encoding: "base64", Kind: "xlsx", ByteLength: len(body), SHA256: hex.EncodeToString(digest[:]), Data: f.document.Content}}, nil
}

type candidateFaultEditing struct {
	*fakeNativeEditing
	buildErr error
	corrupt  func(*fileport.NativeWorkbookCandidate)
}

type candidateWithoutBuilder struct {
	EditingService
	NativeRecoveryService
}

func TestWorkbookCandidateMissingBuilderRefusesBeforeDurableApproval(t *testing.T) {
	a, base, _, selected := typedSelectionFixture(t, 1)
	owner := a.service.(*fakeNativeEditing)
	scope, proposal := candidateProposal(t, a, base, selected)
	a.service = &candidateWithoutBuilder{EditingService: owner, NativeRecoveryService: owner}
	if out := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal)); out["code"] != "unavailable" || owner.prepared {
		t.Fatal("missing builder created unusable durable approval", out, owner.prepared)
	}
}

func (f *candidateFaultEditing) BuildNativeWorkbookCandidate(ctx context.Context, id, thread, change, operation, base string) (fileport.NativeWorkbookCandidate, error) {
	if f.buildErr != nil {
		return fileport.NativeWorkbookCandidate{}, f.buildErr
	}
	candidate, err := f.fakeNativeEditing.BuildNativeWorkbookCandidate(ctx, id, thread, change, operation, base)
	if err == nil && f.corrupt != nil {
		f.corrupt(&candidate)
	}
	return candidate, err
}

func candidateProposal(t *testing.T, a *Adapter, fake *fakeEditing, selected office.WorkbookSelection) (map[string]any, map[string]any) {
	t.Helper()
	fields := captureFields(fake, true)
	fields["text"], fields["workbook"] = "1", selected
	scope := nativeInvoke(t, a, "capture-selection", fields)["scope"].(map[string]any)
	input := modelFields(scope)
	value := 2.0
	input["operationId"], input["workbook"] = "candidate_propose_01", office.WorkbookPatch{Kind: "number", Value: &value}
	result := nativeInvoke(t, a, "model-selection-propose", input)
	if result["ok"] != true {
		t.Fatal(result)
	}
	return scope, result["result"].(map[string]any)
}

func TestWorkbookCandidateBuildFailureRetainsApprovedDecisionForSameOperationAtCapacity(t *testing.T) {
	a, base, _, selected := typedSelectionFixture(t, 1)
	owner := a.service.(*fakeNativeEditing)
	fault := &candidateFaultEditing{fakeNativeEditing: owner, buildErr: fileport.ErrInvalidInput}
	a.service = fault
	scope, proposal := candidateProposal(t, a, base, selected)
	captured := a.scopes[scope["scopeId"].(string)]
	for i := 0; len(captured.operations) < editingapp.MaxProposalOperations-1; i++ {
		captured.operations["previous_"+strings.Repeat("a", i+1)] = strings.Repeat("d", 64)
	}
	decision := decisionFields(scope, proposal)
	if out := nativeInvoke(t, a, "proposal-accept", decision); out["ok"] == true || out["replacement"] != nil {
		t.Fatal("build failure exposed a replacement", out)
	}
	stored := a.scopes[scope["scopeId"].(string)].proposals[proposal["proposalId"].(string)]
	if !owner.prepared || owner.committed || stored.Status != "approved" || stored.decisionOperation != decision["operationId"] {
		t.Fatal("durable preparation/approval was hidden by build failure", owner.prepared, stored.Status)
	}
	if len(captured.operations) != editingapp.MaxProposalOperations {
		t.Fatal("fixture did not fill the last decision slot", len(captured.operations))
	}
	for _, call := range owner.calls {
		if call == "native-commit" || call == "commit" {
			t.Fatal("build failure silently persisted", owner.calls)
		}
	}
	fault.buildErr = nil
	out := nativeInvoke(t, a, "proposal-accept", decision)
	if out["ok"] != true {
		t.Fatal("same decision operation could not retry candidate generation", out)
	}
	if len(captured.operations) != editingapp.MaxProposalOperations {
		t.Fatal("same operation consumed another capacity slot", len(captured.operations))
	}
	replacement := out["replacement"].(map[string]any)
	candidate := replacement["candidate"].(map[string]any)
	if candidate["changeId"] != replacement["changeId"] || candidate["saveOperationId"] != replacement["saveOperationId"] || candidate["baseRevision"] != replacement["baseRevision"] {
		t.Fatal("candidate escaped its approved binding", replacement)
	}
	for _, operation := range []string{"model-selection-read", "proposal-read", "object-recovery"} {
		input := modelFields(scope)
		if operation == "proposal-read" {
			input = map[string]any{"sessionId": scope["sessionId"], "scopeId": scope["scopeId"]}
		}
		if operation == "object-recovery" {
			input = map[string]any{"sessionId": scope["sessionId"], "threadId": scope["threadId"]}
		}
		body, _ := json.Marshal(nativeInvoke(t, a, operation, input))
		if strings.Contains(string(body), `"candidate"`) || strings.Contains(string(body), `"sha256"`) {
			t.Fatal("candidate bytes leaked beyond protected-local acceptance", operation, string(body))
		}
	}
}

func TestWorkbookCandidateResponseRejectsForgedBindingOrHash(t *testing.T) {
	for name, corrupt := range map[string]func(*fileport.NativeWorkbookCandidate){
		"change": func(c *fileport.NativeWorkbookCandidate) { c.ChangeID = strings.Repeat("a", 64) },
		"save":   func(c *fileport.NativeWorkbookCandidate) { c.SaveOperationID += "_other" },
		"base":   func(c *fileport.NativeWorkbookCandidate) { c.BaseRevision = strings.Repeat("a", 64) },
		"digest": func(c *fileport.NativeWorkbookCandidate) { c.Content.SHA256 = strings.Repeat("a", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			a, base, _, selected := typedSelectionFixture(t, 1)
			owner := a.service.(*fakeNativeEditing)
			a.service = &candidateFaultEditing{fakeNativeEditing: owner, corrupt: corrupt}
			scope, proposal := candidateProposal(t, a, base, selected)
			if out := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal)); out["ok"] == true || out["replacement"] != nil {
				t.Fatal("forged candidate response accepted", out)
			}
		})
	}
}

type manualCandidateEditing struct {
	*candidateFaultEditing
	manual *fileport.NativeWorkbookManualDraft
}

func (f *manualCandidateEditing) NativeWorkbookEditing(_ context.Context, id, base string) error {
	if id != f.document.SessionID || base != f.document.Revision {
		return editingapp.ErrDraftStale
	}
	return nil
}
func (f *manualCandidateEditing) ValidateNativeWorkbookManual(ctx context.Context, id, base string, selected office.WorkbookSelection, target office.WorkbookWriteTarget, edit office.WorkbookManualEdit) (*office.WorkbookReview, error) {
	if err := f.NativeWorkbookEditing(ctx, id, base); err != nil {
		return nil, err
	}
	patch, err := office.WorkbookManualPatch(selected, target, edit)
	if err != nil {
		return nil, err
	}
	review, err := workbookcodec.ValidatePatch(ctx, selected, patch)
	return &review, err
}
func (f *manualCandidateEditing) PrepareNativeWorkbookManual(_ context.Context, id string, draft fileport.NativeWorkbookManualDraft) (fileport.NativeChangeStatus, error) {
	if id != f.document.SessionID || f.manual != nil && !reflect.DeepEqual(*f.manual, draft) {
		return fileport.NativeChangeStatus{}, fileport.ErrOperationMismatch
	}
	f.manual = &draft
	f.prepared, f.version, f.origin = true, 3, "manual"
	f.draft = fileport.NativeChangeDraft{ChangeID: draft.ChangeID, ThreadID: draft.ThreadID, BaseRevision: draft.BaseRevision, Workbook: draft.Workbook}
	status := f.changeStatus()
	status.Workbook = draft.Workbook
	return status, nil
}

func TestWorkbookManualCandidateFailureKeepsPreparedLeaseAndExactRetry(t *testing.T) {
	_, base, projector, selected := typedSelectionFixture(t, 1)
	owner := &manualCandidateEditing{candidateFaultEditing: &candidateFaultEditing{fakeNativeEditing: &fakeNativeEditing{fakeEditing: base}, buildErr: errors.New("candidate construction failed")}}
	// Reuse only the bounded workbook/document data. A fresh adapter installs
	// both trusted bindings before its first session is opened.
	a := New("xlsx", owner, func(context.Context) bool { return true })
	capture := func(_ context.Context, id, path string, read func() error) (func(), error) {
		if id != base.document.SessionID || path != base.document.Path {
			return nil, editingapp.ErrSession
		}
		if err := read(); err != nil {
			return nil, err
		}
		return func() {}, nil
	}
	if err := a.BindSelectionHost(projector, capture); err != nil {
		t.Fatal(err)
	}
	if err := a.BindManualHost(capture); err != nil {
		t.Fatal(err)
	}
	if out := nativeInvoke(t, a, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": base.document.Path}}); out["ok"] != true {
		t.Fatal(out)
	}
	input := map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "baseRevision": base.document.Revision,
		"operationId": "manual_candidate_01", "selectionToken": "native_selection_01", "changeSequence": 3, "workbook": selected,
		"writeTarget": office.WorkbookWriteTarget{SheetName: "Sheet1"}, "edit": map[string]any{"type": "text", "text": "001"}}
	if out := nativeInvoke(t, a, "prepare-workbook-manual", input); out["ok"] == true {
		t.Fatal("failed candidate claimed prepared success", out)
	}
	session := a.sessions[base.document.SessionID]
	if !owner.prepared || owner.manual == nil || session.release == nil || session.leasePurpose != nativeLeaseManual || session.manualChangeID != owner.manual.ChangeID {
		t.Fatal("durably prepared manual lost its lease or identity")
	}
	owner.buildErr = nil
	out := nativeInvoke(t, a, "prepare-workbook-manual", input)
	if out["ok"] != true || out["workbookManual"].(map[string]any)["candidate"] == nil {
		t.Fatal("exact manual operation could not retry", out)
	}
	input["operationId"] = "manual_candidate_other"
	if out := nativeInvoke(t, a, "prepare-workbook-manual", input); out["code"] != "conflict" {
		t.Fatal("different manual operation stole pending authority", out)
	}
	for _, call := range owner.calls {
		if call == "native-commit" || call == "commit" {
			t.Fatal("prepare wrote a file", owner.calls)
		}
	}
	if out := nativeInvoke(t, a, "cancel-change", map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "changeId": owner.manual.ChangeID, "baseRevision": base.document.Revision}); out["ok"] != true || session.release != nil {
		t.Fatal("pending manual could not cancel", out)
	}
}
