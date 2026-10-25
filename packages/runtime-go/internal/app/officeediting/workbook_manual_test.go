package officeediting

import (
	"context"
	"strings"
	"testing"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

func TestReopenedWorkbookManualPendingCannotBecomeAgentEditAuthority(t *testing.T) {
	a, base, _, selected := typedSelectionFixture(t, 1)
	fake := a.service.(*fakeNativeEditing)
	fake.prepared, fake.version, fake.origin = true, 3, "manual"
	fake.draft = fileport.NativeChangeDraft{ChangeID: strings.Repeat("e", 64), ThreadID: "thread_main", BaseRevision: base.document.Revision}
	fields := captureFields(base, true)
	fields["text"], fields["workbook"] = "1", selected
	if out := nativeInvoke(t, a, "capture-selection", fields); out["code"] != "conflict" {
		t.Fatal("reopened manual pending inherited strict capture", out)
	}
	// Discussion still has its ordinary protected projection and no edit lease.
	delete(fields, "workbook")
	fields["editable"] = false
	if out := nativeInvoke(t, a, "capture-selection", fields); out["ok"] != true {
		t.Fatal("pending manual blocked discussion", out)
	}
	if a.sessions[base.document.SessionID].release != nil {
		t.Fatal("discussion gained mutation lease")
	}
}

func TestWorkbookManualOperationsDoNotFallBackToDocxManualOrModel(t *testing.T) {
	a, base, _, _ := selectionFixture(t)
	input := map[string]any{"sessionId": base.document.SessionID, "threadId": "thread_main", "baseRevision": base.document.Revision, "writeTarget": map[string]any{"sheet": 0, "sheetName": "Sheet1", "column": 0, "row": 0}, "edit": map[string]any{"type": "empty"}}
	if out := nativeInvoke(t, a, "plan-workbook-manual", input); out["code"] != "invalid_request" {
		t.Fatal("Word widened to workbook manual", out)
	}
	input["operationId"], input["selectionToken"], input["changeSequence"], input["workbook"] = "manual_workbook_01", "native_manual_001", 0, map[string]any{}
	if out := nativeInvoke(t, a, "prepare-workbook-manual", input); out["code"] != "invalid_request" {
		t.Fatal("Word widened to workbook prepare", out)
	}
	if out := nativeInvoke(t, a, "model-selection-propose", input); out["ok"] == true {
		t.Fatal("local edit impersonated model proposal", out)
	}
}

func TestWorkbookManualChangeCannotInheritAnotherThreadsStrictLease(t *testing.T) {
	a, base, _, _ := typedSelectionFixture(t, 1)
	fake := a.service.(*fakeNativeEditing)
	fake.prepared, fake.version, fake.origin = true, 3, "manual"
	fake.draft = fileport.NativeChangeDraft{ChangeID: strings.Repeat("e", 64), ThreadID: "thread_main", BaseRevision: base.document.Revision}
	session := a.sessions[base.document.SessionID]
	session.release, session.leasePurpose = func() {}, nativeLeaseStrict
	if release, err := a.captureNativeMutation(context.Background(), base.document.SessionID, "thread_main", fake.draft.ChangeID, base.document.Revision, "committed"); err == nil || release != nil {
		t.Fatal("manual record inherited unrelated strict lease", err)
	}
}

func TestWorkbookReadonlyPartialMetadataSurvivesModelReadWithoutEditAuthority(t *testing.T) {
	a, base, _, _ := typedSelectionFixture(t, 1)
	text := "Workbook selection: \"Sheet1\"!A1:A1048576\nCapture: partial; values below are not the entire selection; do not infer whole-range totals; captured cells: 1; maximum 256. Read-only discussion reference; no write authority.\n1"
	fields := captureFields(base, false)
	fields["text"] = text
	captured := nativeInvoke(t, a, "capture-selection", fields)
	if captured["ok"] != true {
		t.Fatal("readonly native metadata capture rejected", captured)
	}
	scope := captured["scope"].(map[string]any)
	read := nativeInvoke(t, a, "model-selection-read", modelFields(scope))
	if read["ok"] != true {
		t.Fatal("readonly metadata model read rejected", read)
	}
	result := read["result"].(map[string]any)
	var projected strings.Builder
	for _, raw := range result["parts"].([]any) {
		part := raw.(map[string]any)
		if part["kind"] != "literal" {
			t.Fatal("safe reference metadata unexpectedly transformed", part)
		}
		projected.WriteString(part["text"].(string))
	}
	if projected.String() != text || !strings.Contains(projected.String(), `"Sheet1"!A1:A1048576`) || !strings.Contains(projected.String(), "partial") || !strings.Contains(projected.String(), "do not infer whole-range totals") {
		t.Fatal("model read lost the original range or partial warning", projected.String())
	}
	for _, value := range []map[string]any{scope, result} {
		if value["editable"] != false {
			t.Fatal("discussion reference became editable", value)
		}
		for _, key := range []string{"workbook", "proposalId", "changeId", "saveOperationId", "undoOperationId"} {
			if value[key] != nil {
				t.Fatal("discussion metadata acquired write authority", key)
			}
		}
	}
	proposal := modelFields(scope)
	proposal["operationId"] = "readonly_propose_01"
	proposal["parts"] = []any{map[string]any{"kind": "literal", "text": "Changed"}}
	if out := nativeInvoke(t, a, "model-selection-propose", proposal); out["code"] != "scope_invalid" {
		t.Fatal("readonly metadata authorized a model edit", out)
	}
	session := a.sessions[base.document.SessionID]
	if session.release != nil || session.leasePurpose != nativeLeaseNone || len(a.scopes[scope["scopeId"].(string)].proposals) != 0 {
		t.Fatal("readonly reference obtained a lease or proposal")
	}
}
