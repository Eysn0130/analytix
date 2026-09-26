package officegeneration

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func pivotWorkbookFixture() Workbook {
	w := Workbook{Sheets: []Sheet{{ID: "source", Name: "销售 '源' 数据", Cells: []Cell{}}, {ID: "summary", Name: "Summary", Cells: []Cell{}}}}
	for row, values := range [][]any{{"Region", "Quarter", "Channel", "Sales"}, {"East", "Q1", "Direct", 12.5}, {"East", "Q2", "Online", 7.5}, {"West", "Q1", "Direct", 4.0}} {
		for col, value := range values {
			kind := "text"
			if _, ok := value.(float64); ok {
				kind = "number"
			}
			w.Sheets[0].Cells = append(w.Sheets[0].Cells, Cell{Address: fmt.Sprintf("%c%d", 'A'+col, row+1), Type: kind, Value: value})
		}
	}
	w.Pivots = []Pivot{{ID: "SalesPivot", SourceSheetID: "source", SourceRange: "A1:D4", TargetSheetID: "summary", TargetRange: "A4:E12", Rows: []string{"Region"}, Columns: []string{"Quarter"}, Filters: []string{"Channel"}, Values: []PivotValue{{Field: "Sales", Aggregate: "sum"}}}}
	return w
}

func TestWorkbookPivotStrictParsingAndValidation(t *testing.T) {
	fixture := pivotWorkbookFixture()
	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseWorkbook(body); err != nil {
		t.Fatal(err)
	}
	for name, modify := range map[string]func(*Workbook){
		"duplicate ID": func(w *Workbook) {
			p := w.Pivots[0]
			p.ID = "salespivot"
			p.TargetRange = "G4:K12"
			w.Pivots = append(w.Pivots, p)
		},
		"empty ID":                    func(w *Workbook) { w.Pivots[0].ID = "" },
		"unknown sheet":               func(w *Workbook) { w.Pivots[0].SourceSheetID = "absent" },
		"unknown target":              func(w *Workbook) { w.Pivots[0].TargetSheetID = "absent" },
		"unsupported sheet delimiter": func(w *Workbook) { w.Sheets[0].Name = "Sales!" },
		"unknown field":               func(w *Workbook) { w.Pivots[0].Rows = []string{"Missing"} },
		"empty field":                 func(w *Workbook) { w.Pivots[0].Rows = []string{""} },
		"duplicate axis":              func(w *Workbook) { w.Pivots[0].Columns = []string{"Region"} },
		"value axis overlap":          func(w *Workbook) { w.Pivots[0].Values[0].Field = "Region" },
		"multiple rows":               func(w *Workbook) { w.Pivots[0].Rows = []string{"Region", "Channel"} },
		"no values":                   func(w *Workbook) { w.Pivots[0].Values = nil },
		"unsupported aggregate":       func(w *Workbook) { w.Pivots[0].Values[0].Aggregate = "SUMIF" },
		"aggregate case":              func(w *Workbook) { w.Pivots[0].Values[0].Aggregate = "Sum" },
		"nonnumeric sum":              func(w *Workbook) { w.Pivots[0].Columns = nil; w.Pivots[0].Values[0].Field = "Quarter" },
		"duplicate header":            func(w *Workbook) { w.Sheets[0].Cells[1].Value = "region" },
		"blank header":                func(w *Workbook) { w.Sheets[0].Cells[1].Value = " " },
		"numeric header":              func(w *Workbook) { w.Sheets[0].Cells[1].Type = "number"; w.Sheets[0].Cells[1].Value = 1 },
		"mixed types":                 func(w *Workbook) { w.Sheets[0].Cells[7].Type = "text"; w.Sheets[0].Cells[7].Value = "12.5" },
		"blank source":                func(w *Workbook) { w.Sheets[0].Cells = w.Sheets[0].Cells[:15] },
		"empty source text":           func(w *Workbook) { w.Sheets[0].Cells[4].Value = "" },
		"formula source": func(w *Workbook) {
			w.Sheets[0].Cells[7].Type = "formula"
			w.Sheets[0].Cells[7].Value = nil
			w.Sheets[0].Cells[7].Formula = "1+2"
		},
		"header only":           func(w *Workbook) { w.Pivots[0].SourceRange = "A1:D1" },
		"reversed source":       func(w *Workbook) { w.Pivots[0].SourceRange = "D4:A1" },
		"range budget":          func(w *Workbook) { w.Pivots[0].SourceRange = "A1:D10000" },
		"row boundary":          func(w *Workbook) { w.Pivots[0].TargetRange = "A99998:E100001" },
		"small target":          func(w *Workbook) { w.Pivots[0].TargetRange = "A4:B5" },
		"filter headroom":       func(w *Workbook) { w.Pivots[0].TargetRange = "A1:E12" },
		"occupied target":       func(w *Workbook) { w.Sheets[1].Cells = []Cell{{Address: "B5", Type: "text", Value: "keep"}} },
		"occupied filter area":  func(w *Workbook) { w.Sheets[1].Cells = []Cell{{Address: "B2", Type: "number", Value: 0}} },
		"source target overlap": func(w *Workbook) { w.Pivots[0].TargetSheetID = "source" },
		"pivot overlap":         func(w *Workbook) { p := w.Pivots[0]; p.ID = "Other"; w.Pivots = append(w.Pivots, p) },
		"pivot count budget":    func(w *Workbook) { w.Pivots = make([]Pivot, WorkbookMaxPivots+1) },
	} {
		t.Run(name, func(t *testing.T) {
			w := pivotWorkbookFixture()
			modify(&w)
			if w.Validate() == nil {
				t.Fatal("accepted invalid pivot")
			}
		})
	}
	for _, replacement := range []string{
		`"aggregate":null`, `"aggregate":"sum","aggregate":"count"`, `"Aggregate":"sum"`, `"aggregate":"sum","unknown":true`,
	} {
		bad := strings.Replace(string(body), `"aggregate":"sum"`, replacement, 1)
		if _, err := ParseWorkbook([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", replacement)
		}
	}
	for _, aggregate := range []string{"sum", "count", "average", "min", "max"} {
		w := pivotWorkbookFixture()
		w.Pivots[0].Values[0].Aggregate = aggregate
		if err := w.Validate(); err != nil {
			t.Fatalf("%s: %v", aggregate, err)
		}
	}
	// Report filters expose native all-items selectors; no implicit filtering.
	w := pivotWorkbookFixture()
	w.Pivots[0].Columns = nil
	w.Pivots[0].Values = []PivotValue{{Field: "Quarter", Aggregate: "count"}}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkbookPivotBudgetsAndSameSheetTarget(t *testing.T) {
	w := pivotWorkbookFixture()
	w.Pivots[0].TargetSheetID = "source"
	w.Pivots[0].TargetRange = "F4:J12"
	if err := w.Validate(); err != nil {
		t.Fatalf("disjoint same-sheet pivot: %v", err)
	}
	w = pivotWorkbookFixture()
	pivot := w.Pivots[0]
	w.Pivots = nil
	for i := 0; i < WorkbookMaxPivots; i++ {
		p := pivot
		p.ID = fmt.Sprintf("Pivot%d", i)
		p.TargetRange = fmt.Sprintf("A%d:E%d", 4+i*15, 12+i*15)
		w.Pivots = append(w.Pivots, p)
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("bounded maximum pivot count: %v", err)
	}
	for i := range w.Pivots {
		w.Pivots[i].TargetRange = fmt.Sprintf("A%d:CV%d", 4+i*110, 103+i*110)
	}
	if w.Validate() == nil {
		t.Fatal("accepted aggregate pivot cell budget overflow")
	}
}
