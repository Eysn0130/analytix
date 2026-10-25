package officegeneration

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestParseWorkbookManualEditAndWriteTarget(t *testing.T) {
	for _, raw := range []string{
		`{"type":"number","value":0}`,
		`{"type":"formula","formula":"=SUM(A1:A2)"}`,
		`{"type":"text","text":"=1+1"}`,
		`{"type":"empty"}`,
	} {
		if _, err := ParseWorkbookManualEdit([]byte(raw)); err != nil {
			t.Fatalf("rejected %s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		`{"type":"number","value":null}`,
		`{"type":"number","value":"1"}`,
		`{"type":"number","value":1,"value":2}`,
		`{"type":"number","value":1e309}`,
		`{"type":"number","value":1,"text":"1"}`,
		`{"type":"formula","formula":"="}`,
		`{"type":"formula","formula":"1","value":null}`,
		`{"type":"text","text":null}`,
		`{"type":"text","text":""}`,
		`{"type":"text","text":"literal","formula":""}`,
		`{"type":"empty","text":""}`,
		`{"type":"empty","value":null}`,
		`{"type":"empty","rowOffset":0}`,
		`{"type":"empty","columnOffset":0}`,
		`{"type":"boolean","value":true}`,
	} {
		if _, err := ParseWorkbookManualEdit([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, text := range []string{strings.Repeat("x", 4097), "bad\x00text"} {
		body, _ := json.Marshal(map[string]any{"type": "text", "text": text})
		if _, err := ParseWorkbookManualEdit(body); err == nil {
			t.Fatal("accepted unsupported text")
		}
	}
	target, err := ParseWorkbookWriteTarget([]byte(`{"sheet":0,"sheetName":"统计","column":0,"row":2}`))
	if err != nil || target != (WorkbookWriteTarget{SheetName: "统计", Row: 2}) {
		t.Fatal("invalid parsed target", target, err)
	}
	for _, raw := range []string{
		`{"sheet":0,"sheetName":"统计","column":0}`,
		`{"sheet":0,"sheetName":"统计","column":0,"row":null}`,
		`{"sheet":0,"sheetName":"统计","column":0,"row":2,"row":3}`,
		`{"sheet":0,"sheetName":"统计","column":0,"row":2,"endRow":3}`,
		`{"sheet":0,"sheetName":"统计","column":0,"row":2.5}`,
		`{"sheet":20,"sheetName":"统计","column":0,"row":2}`,
		`{"sheet":0,"sheetName":"统计","column":-1,"row":2}`,
		`{"sheet":0,"sheetName":" 统计","column":0,"row":2}`,
	} {
		if _, err := ParseWorkbookWriteTarget([]byte(raw)); err == nil {
			t.Fatalf("accepted target %s", raw)
		}
	}
}

func TestPlanWorkbookManualBounds(t *testing.T) {
	target := WorkbookWriteTarget{Sheet: 1, SheetName: "统计", Column: 2, Row: 2}
	for _, formula := range []string{"SUM(A1:B2)", "=SUM($A$1:$B$2)", "SUM('统计'!A1:B2)", "SUM(A1;B2)", `SUMIF(A1:A2,">0",B1:B2)`} {
		bounds, err := PlanWorkbookManual(target, WorkbookManualEdit{Type: "formula", Formula: formula})
		if err != nil || bounds != (WorkbookDependencyBounds{Sheet: 1, EndColumn: 2, EndRow: 2}) {
			t.Fatalf("unexpected dependency rectangle for %s: %#v %v", formula, bounds, err)
		}
	}
	number, text := float64(3), "=A1"
	for _, edit := range []WorkbookManualEdit{{Type: "number", Value: &number}, {Type: "text", Text: &text}, {Type: "empty"}, {Type: "formula", Formula: "1+2"}} {
		bounds, err := PlanWorkbookManual(target, edit)
		if err != nil || bounds != (WorkbookDependencyBounds{Sheet: 1, StartColumn: 2, StartRow: 2, EndColumn: 2, EndRow: 2}) {
			t.Fatalf("non-reference edit enlarged capture: %#v %v", bounds, err)
		}
	}
	for _, formula := range []string{"Other!A1", "SUM(A:A)", "SUM(1:2)", "UNKNOWN(A1)", `WEBSERVICE("https://invalid.example")`, "[other.xlsx]统计!A1", "SUM(A1:A256)", "A1 B2", "SUMIF(A1:A2,1,B1)", "SUM(A1:A2))"} {
		if _, err := PlanWorkbookManual(target, WorkbookManualEdit{Type: "formula", Formula: formula}); err == nil {
			t.Fatalf("accepted unsupported formula %s", formula)
		}
	}
	target = WorkbookWriteTarget{SheetName: "统计", Row: 255}
	if bounds, err := PlanWorkbookManual(target, WorkbookManualEdit{Type: "formula", Formula: "SUM(A1:A255)"}); err != nil || bounds.StartRow != 0 || bounds.EndRow != 255 {
		t.Fatal("rejected exact 256-cell rectangle", bounds, err)
	}
	target.Row++
	if _, err := PlanWorkbookManual(target, WorkbookManualEdit{Type: "formula", Formula: "SUM(A1:A255)"}); err == nil {
		t.Fatal("accepted rectangle of 257 cells")
	}
	for _, invalid := range []WorkbookWriteTarget{{Sheet: -1, SheetName: "统计"}, {SheetName: "统计", Column: WorkbookMaxColumns}, {SheetName: "统计", Row: WorkbookMaxRows}, {SheetName: ""}} {
		if _, err := PlanWorkbookManual(invalid, WorkbookManualEdit{Type: "empty"}); err == nil {
			t.Fatal("accepted invalid target", invalid)
		}
	}
	number = math.Inf(1)
	if _, err := PlanWorkbookManual(target, WorkbookManualEdit{Type: "number", Value: &number}); err == nil {
		t.Fatal("accepted nonfinite typed value")
	}
	text = ""
	if _, err := PlanWorkbookManual(target, WorkbookManualEdit{Type: "text", Text: &text}); err == nil {
		t.Fatal("empty manual text must use the explicit empty operation")
	}
}

func TestWorkbookManualPatchKeepsWriteTargetSeparate(t *testing.T) {
	selected := selectedWorkbook()
	target := WorkbookWriteTarget{SheetName: selected.SheetName, Row: 2}
	edit := WorkbookManualEdit{Type: "formula", Formula: "SUM(A1:A2)"}
	patch, err := WorkbookManualPatch(selected, target, edit)
	if err != nil || patch.Kind != "range" || len(patch.Cells) != 1 || patch.Cells[0].RowOffset != 2 || patch.Cells[0].ColumnOffset != 0 || patch.Cells[0].Type != "formula" {
		t.Fatal("manual patch did not retain the one-cell write target", patch, err)
	}
	after, err := ApplyWorkbookPatch(selected, patch)
	if err != nil || !reflect.DeepEqual(after.Cells[:2], selected.Cells[:2]) || after.Cells[2].Formula != "=SUM(A1:A2)" || selected.Cells[2].ValueType != "empty" {
		t.Fatal("dependency cells or before selection changed", err)
	}
	for _, mutate := range []func(*WorkbookSelection){
		func(s *WorkbookSelection) { s.SheetName = "其他" },
		func(s *WorkbookSelection) { s.Sheet++ },
		func(s *WorkbookSelection) { s.EndRow--; s.Cells = s.Cells[:2] },
		func(s *WorkbookSelection) {
			s.EndRow++
			s.Cells = append(s.Cells, WorkbookSelectedCell{Row: 3, ValueType: "empty", RowVisible: true, ColumnVisible: true})
		},
		func(s *WorkbookSelection) { s.Cells[0].Merged = true },
		func(s *WorkbookSelection) { s.Cells[0].ColumnVisible = false },
	} {
		bad := selectedWorkbook()
		mutate(&bad)
		if _, err := WorkbookManualPatch(bad, target, edit); err == nil {
			t.Fatal("accepted mismatched or incomplete native capture")
		}
	}
	for _, formula := range []string{"A3+1", "SUM(A1:A4)"} {
		if _, err := WorkbookManualPatch(selected, target, WorkbookManualEdit{Type: "formula", Formula: formula}); err == nil {
			t.Fatalf("accepted circular or uncaptured dependency %s", formula)
		}
	}
	single := WorkbookSelection{SheetName: target.SheetName, StartRow: 2, EndRow: 2, Cells: append([]WorkbookSelectedCell{}, selected.Cells[2])}
	if _, err := WorkbookManualPatch(single, target, WorkbookManualEdit{Type: "formula", Formula: "A3+1"}); err == nil {
		t.Fatal("accepted circular formula in an exact native capture")
	}
}

func TestWorkbookManualPatchOffsetsWithinShiftedCapture(t *testing.T) {
	selected := WorkbookSelection{Sheet: 2, SheetName: "统计", StartColumn: 1, StartRow: 2, EndColumn: 3, EndRow: 4}
	for row := selected.StartRow; row <= selected.EndRow; row++ {
		for column := selected.StartColumn; column <= selected.EndColumn; column++ {
			selected.Cells = append(selected.Cells, WorkbookSelectedCell{Sheet: 2, Column: column, Row: row, ValueType: "empty", RowVisible: true, ColumnVisible: true})
		}
	}
	target := WorkbookWriteTarget{Sheet: 2, SheetName: "统计", Column: 3, Row: 4}
	patch, err := WorkbookManualPatch(selected, target, WorkbookManualEdit{Type: "formula", Formula: "SUM(B3:C4)"})
	if err != nil || len(patch.Cells) != 1 || patch.Cells[0].RowOffset != 2 || patch.Cells[0].ColumnOffset != 2 {
		t.Fatal("write offset was not relative to the dependency capture", patch, err)
	}
	after, err := ApplyWorkbookPatch(selected, patch)
	if err != nil || !reflect.DeepEqual(after.Cells[:8], selected.Cells[:8]) || after.Cells[8].Formula != "=SUM(B3:C4)" {
		t.Fatal("write escaped the original target", err)
	}
}

func TestWorkbookManualReplacingOldFormulaNeedsOnlyTarget(t *testing.T) {
	target := WorkbookWriteTarget{SheetName: "Sheet1", Row: 2}
	selected := WorkbookSelection{SheetName: "Sheet1", StartRow: 2, EndRow: 2, Cells: []WorkbookSelectedCell{{Row: 2, ValueType: "formula", Formula: "=SUM(A1:A2)", RowVisible: true, ColumnVisible: true}}}
	value, text := float64(4), "literal"
	for _, edit := range []WorkbookManualEdit{{Type: "number", Value: &value}, {Type: "text", Text: &text}, {Type: "empty"}} {
		bounds, err := PlanWorkbookManual(target, edit)
		if err != nil || bounds.StartRow != 2 || bounds.EndRow != 2 {
			t.Fatal("replacement read a retired formula dependency", bounds, err)
		}
		patch, err := WorkbookManualPatch(selected, target, edit)
		if err != nil {
			t.Fatal("retired formula blocked replacement", edit.Type, err)
		}
		after, err := ApplyWorkbookPatch(selected, patch)
		if err != nil || len(after.Cells) != 1 || after.Cells[0].ValueType != edit.Type || after.Cells[0].Formula == selected.Cells[0].Formula {
			t.Fatal("replacement retained old formula or expanded selection", after, err)
		}
	}
}
