package officegeneration

import (
	"encoding/json"
	"testing"
)

func selectedWorkbook() WorkbookSelection {
	return WorkbookSelection{SheetName: "统计", EndRow: 2, Cells: []WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}, {Row: 1, ValueType: "number", Value: 2, Text: "2", Formula: "2", RowVisible: true, ColumnVisible: true}, {Row: 2, ValueType: "empty", RowVisible: true, ColumnVisible: true}}}
}
func TestWorkbookTypedPatchScopesAndGrammar(t *testing.T) {
	s := selectedWorkbook()
	p := WorkbookPatch{Kind: "range", Cells: []WorkbookCellEdit{{RowOffset: 2, Type: "formula", Formula: "SUM(A1:A2)"}}}
	if _, e := ApplyWorkbookPatch(s, p); e != nil {
		t.Fatal(e)
	}
	for _, formula := range []string{"SUM(A1:A4)", "A3+1", "UNKNOWN(A1)", "[other.xlsx]Sheet!A1", `WEBSERVICE("https://invalid.example")`} {
		p.Cells[0].Formula = formula
		if _, e := ApplyWorkbookPatch(s, p); e == nil {
			t.Fatalf("accepted %s", formula)
		}
	}
	for _, raw := range []string{`{"kind":"number","value":1,"formula":"1"}`, `{"kind":"number","value":null}`, `{"kind":"number","value":1,"value":2}`, `{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"text","text":null}]}`} {
		if _, e := ParseWorkbookPatch([]byte(raw)); e == nil {
			t.Fatal("bad patch", raw)
		}
	}
	p = WorkbookPatch{Kind: "range", Cells: []WorkbookCellEdit{{RowOffset: 2, Type: "formula", Formula: "1"}, {RowOffset: 2, Type: "formula", Formula: "2"}}}
	if _, e := ApplyWorkbookPatch(s, p); e == nil {
		t.Fatal("duplicate target")
	}
	p.Cells = p.Cells[:1]
	p.Cells[0].RowOffset = 3
	if _, e := ApplyWorkbookPatch(s, p); e == nil {
		t.Fatal("escaped scope")
	}
	text := "=1+1"
	p.Cells[0] = WorkbookCellEdit{Type: "text", Text: &text}
	after, err := ApplyWorkbookPatch(s, p)
	if err != nil || after.Cells[0].ValueType != "text" || after.Cells[0].Text != text || after.Cells[0].Value != 0 {
		t.Fatal("explicit text edit was not preserved as text", err)
	}
	for _, mutate := range []func(*WorkbookSelection){func(s *WorkbookSelection) { s.Cells[0].Merged = true }, func(s *WorkbookSelection) { s.Cells[0].RowVisible = false }, func(s *WorkbookSelection) { s.Cells[1].Row = 0 }, func(s *WorkbookSelection) { s.Cells = s.Cells[:2] }} {
		bad := selectedWorkbook()
		mutate(&bad)
		body, _ := json.Marshal(bad)
		if _, e := ParseWorkbookSelection(body); e == nil {
			t.Fatal("bad selection")
		}
	}
}

func TestWorkbookTypedPatchExplicitClear(t *testing.T) {
	patch, err := ParseWorkbookPatch([]byte(`{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"empty"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, previousType := range []string{"empty", "number", "text", "formula"} {
		t.Run(previousType, func(t *testing.T) {
			selected := WorkbookSelection{SheetName: "数据", Cells: []WorkbookSelectedCell{{ValueType: previousType, NumberFormat: 14, RowVisible: true, ColumnVisible: true}}}
			switch previousType {
			case "number":
				selected.Cells[0].Value, selected.Cells[0].Text, selected.Cells[0].Formula = 7, "7", "7"
			case "text":
				selected.Cells[0].Text, selected.Cells[0].Formula = "original", "original"
			case "formula":
				selected.Cells[0].Formula = "=1+1"
			}
			after, err := ApplyWorkbookPatch(selected, patch)
			if err != nil {
				t.Fatal(err)
			}
			cell := after.Cells[0]
			if cell.ValueType != "empty" || cell.Text != "" || cell.Formula != "" || cell.Value != 0 || cell.NumberFormat != 14 || !cell.RowVisible || !cell.ColumnVisible {
				t.Fatalf("incomplete clearing or metadata changed: %#v", cell)
			}
			if selected.Cells[0].ValueType != previousType {
				t.Fatal("mutated the before selection")
			}
		})
	}
	for _, raw := range []string{
		`{"kind":"empty"}`,
		`{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"empty","text":""}]}`,
		`{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"empty","formula":""}]}`,
		`{"kind":"range","cells":[{"rowOffset":0,"columnOffset":0,"type":"empty","value":null}]}`,
		`{"kind":"range","cells":[{"columnOffset":0,"type":"empty"}]}`,
	} {
		if _, err := ParseWorkbookPatch([]byte(raw)); err == nil {
			t.Fatalf("accepted implicit or ambiguous clearing: %s", raw)
		}
	}
	text, number := "", float64(0)
	for _, edit := range []WorkbookCellEdit{{Type: "empty", Text: &text}, {Type: "empty", Value: &number}, {Type: "empty", Formula: "1"}} {
		if _, err := ApplyWorkbookPatch(selectedWorkbook(), WorkbookPatch{Kind: "range", Cells: []WorkbookCellEdit{edit}}); err == nil {
			t.Fatal("accepted clearing with value fields")
		}
	}
	if _, err := ApplyWorkbookPatch(WorkbookSelection{SheetName: "数据", Cells: []WorkbookSelectedCell{{ValueType: "empty", RowVisible: true, ColumnVisible: true}}}, WorkbookPatch{Kind: "empty"}); err == nil {
		t.Fatal("accepted a new top-level patch kind")
	}
}

func TestWorkbookTypedPatchTextReplacesEachValueType(t *testing.T) {
	text := "=SUM(A1:A2)"
	for _, previousType := range []string{"empty", "number", "text", "formula"} {
		t.Run(previousType, func(t *testing.T) {
			selected := WorkbookSelection{SheetName: "数据", Cells: []WorkbookSelectedCell{{ValueType: previousType, RowVisible: true, ColumnVisible: true}}}
			if previousType == "formula" {
				selected.Cells[0].Formula = "=1"
			}
			after, err := ApplyWorkbookPatch(selected, WorkbookPatch{Kind: "range", Cells: []WorkbookCellEdit{{Type: "text", Text: &text}}})
			if err != nil || after.Cells[0].ValueType != "text" || after.Cells[0].Text != text || after.Cells[0].Formula != text || after.Cells[0].Value != 0 {
				t.Fatal("explicit text edit was coerced or rejected", err)
			}
		})
	}
}
