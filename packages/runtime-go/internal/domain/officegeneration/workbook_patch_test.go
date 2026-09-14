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
	if _, e := ApplyWorkbookPatch(s, p); e == nil {
		t.Fatal("coerced number to text")
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
