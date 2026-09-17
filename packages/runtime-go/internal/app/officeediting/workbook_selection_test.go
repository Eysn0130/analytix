package officeediting

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/workbookcodec"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

func typedSelectionFixture(t *testing.T, count int) (*Adapter, *fakeEditing, *nativeTestProjector, office.WorkbookSelection) {
	t.Helper()
	book := excelize.NewFile()
	defer book.Close()
	selected := office.WorkbookSelection{SheetName: "Sheet1", EndRow: count - 1}
	for row := 0; row < count; row++ {
		address := office.WorkbookCellAddress(0, row)
		if e := book.SetCellFloat("Sheet1", address, 1, -1, 64); e != nil {
			t.Fatal(e)
		}
		selected.Cells = append(selected.Cells, office.WorkbookSelectedCell{Row: row, ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true})
	}
	body, e := book.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	fake := &fakeEditing{document: editingapp.Opened{SessionID: strings.Repeat("b", 48), ObjectID: strings.Repeat("d", 64), Path: "/workspace/report.xlsx", Revision: strings.Repeat("c", 64), Content: base64.StdEncoding.EncodeToString(body.Bytes())}}
	projector := &nativeTestProjector{}
	adapter := New("xlsx", &fakeNativeEditing{fakeEditing: fake}, func(context.Context) bool { return true })
	if e = adapter.BindSelectionHost(projector, func(_ context.Context, _, _ string, read func() error) (func(), error) {
		if e := read(); e != nil {
			return nil, e
		}
		return func() {}, nil
	}); e != nil {
		t.Fatal(e)
	}
	if out := nativeInvoke(t, adapter, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": fake.document.Path}}); out["ok"] != true {
		t.Fatal(out)
	}
	return adapter, fake, projector, selected
}
func TestNativeTypedWorkbook256CellBudgetAndStrictBounds(t *testing.T) {
	a, fake, _, s := typedSelectionFixture(t, 256)
	fields := captureFields(fake, true)
	fields["text"] = "1"
	fields["workbook"] = s
	captured := nativeInvoke(t, a, "capture-selection", fields)
	if captured["ok"] != true {
		t.Fatal("256 complete cells rejected", captured)
	}
	scope := captured["scope"].(map[string]any)
	input := modelFields(scope)
	input["operationId"] = "typed_propose_256"
	edits := make([]office.WorkbookCellEdit, 256)
	v := 2.0
	for row := range edits {
		edits[row] = office.WorkbookCellEdit{RowOffset: row, Type: "number", Value: &v}
	}
	input["workbook"] = office.WorkbookPatch{Kind: "range", Cells: edits}
	out := nativeInvoke(t, a, "model-selection-propose", input)
	if out["ok"] != true {
		t.Fatal("256 bounded edits rejected", out)
	}
	raw, _ := json.Marshal(out)
	for _, local := range []string{"beforeText", "afterText", "results", "numberFormat"} {
		if strings.Contains(string(raw), local) {
			t.Fatal("local review escaped model lane", local)
		}
	}
	// New typed budgets remain operation-specific, strict and bounded.
	input["operationId"] = "typed_overflow_257"
	input["workbook"] = office.WorkbookPatch{Kind: "range", Cells: append(edits, office.WorkbookCellEdit{RowOffset: 256, Type: "number", Value: &v})}
	if out := nativeInvoke(t, a, "model-selection-propose", input); out["ok"] == true {
		t.Fatal("257 edits accepted")
	}
	input["workbook"] = map[string]any{"kind": "range", "cells": []any{map[string]any{"rowOffset": 0, "columnOffset": 0, "type": "number", "value": map[string]any{"nested": map[string]any{"nested": map[string]any{"nested": 1}}}}}}
	if out := nativeInvoke(t, a, "model-selection-propose", input); out["code"] != "invalid_request" {
		t.Fatal("deep malformed body accepted", out)
	}
	if out := nativeInvoke(t, a, "selection-read", map[string]any{"sessionId": fake.document.SessionID, "scopeId": scope["scopeId"], "workbook": s}); out["code"] != "invalid_request" {
		t.Fatal("other operation budget widened", out)
	}
}
func TestNativeTypedWorkbookRejectsUnsafeScopeAndProposal(t *testing.T) {
	for _, bad := range []string{"privacy", "hidden", "merged", "incomplete", "wrong_type"} {
		t.Run(bad, func(t *testing.T) {
			a, fake, _, s := typedSelectionFixture(t, 2)
			fields := captureFields(fake, true)
			fields["text"] = "1"
			switch bad {
			case "privacy":
				s.Cells[0].Text = "PRIVATE"
			case "hidden":
				s.Cells[0].RowVisible = false
			case "merged":
				s.Cells[0].Merged = true
			case "incomplete":
				s.Cells = s.Cells[:1]
			case "wrong_type":
				s.Cells[0].ValueType = "text"
			}
			fields["workbook"] = s
			if out := nativeInvoke(t, a, "capture-selection", fields); out["ok"] == true {
				t.Fatal("unsafe typed scope accepted", bad)
			}
		})
	}
	a, fake, projector, s := typedSelectionFixture(t, 2)
	fields := captureFields(fake, true)
	fields["text"] = "1"
	fields["workbook"] = s
	scope := nativeInvoke(t, a, "capture-selection", fields)["scope"].(map[string]any)
	for i, formula := range []string{"A3+1", "A1+1", "1/0", `IF(1,"PRIVATE","ok")`} {
		input := modelFields(scope)
		input["operationId"] = "typed_invalid_" + strings.Repeat("a", i+1)
		input["workbook"] = office.WorkbookPatch{Kind: "range", Cells: []office.WorkbookCellEdit{{Type: "formula", Formula: formula}}}
		if out := nativeInvoke(t, a, "model-selection-propose", input); out["ok"] == true {
			t.Fatal("invalid formula proposed", formula)
		}
	}
	projector.invalid = true
	if out := nativeInvoke(t, a, "model-selection-read", modelFields(scope)); out["code"] != "projection_unavailable" {
		t.Fatal("invalidated identity/scope still readable", out)
	}
}

func (f *fakeNativeEditing) ValidateNativeWorkbook(ctx context.Context, id, base string, selected office.WorkbookSelection, patch *office.WorkbookPatch) (*office.WorkbookReview, error) {
	if id != f.document.SessionID || base != f.document.Revision {
		return nil, editingapp.ErrDraftStale
	}
	body, err := base64.StdEncoding.DecodeString(f.document.Content)
	if err != nil || workbookcodec.VerifySelectionPackage(ctx, body, selected) != nil {
		return nil, editingapp.ErrDraftStale
	}
	if patch == nil {
		return nil, nil
	}
	review, err := workbookcodec.ValidatePatch(ctx, selected, *patch)
	return &review, err
}
