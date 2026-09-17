package officegeneration

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func workbookFixture() Workbook {
	return Workbook{Sheets: []Sheet{{ID: "data", Name: "中文 数据", Cells: []Cell{
		{Address: "A1", Type: "text", Value: "=SUM(B1:B2)"},
		{Address: "B1", Type: "number", Value: 12.5},
		{Address: "B2", Type: "number", Value: 0},
		{Address: "C1", Type: "boolean", Value: false},
		{Address: "D1", Type: "formula", Formula: "=SUM(B1:B2)"},
	}}}}
}

func TestParseWorkbookStrictTypedJSON(t *testing.T) {
	input := []byte(`{"sheets":[{"id":"data","name":"中文 数据","cells":[{"address":"A1","type":"text","value":"=SUM(B1:B2)"},{"address":"B1","type":"number","value":0},{"address":"C1","type":"boolean","value":false},{"address":"D1","type":"formula","formula":"SUM(B1:B2)"}]}]}`)
	workbook, err := ParseWorkbook(input)
	if err != nil {
		t.Fatal(err)
	}
	if workbook.Sheets[0].Cells[0].Value != "=SUM(B1:B2)" || workbook.Sheets[0].Cells[1].Value != json.Number("0") || workbook.Sheets[0].Cells[2].Value != false {
		t.Fatal("JSON cell types changed")
	}
	for name, body := range map[string]string{
		"unknown root":            `{"sheets":[],"extra":true}`,
		"duplicate root":          `{"sheets":[],"sheets":[]}`,
		"case alias":              strings.Replace(string(input), `"sheets"`, `"Sheets"`, 1),
		"duplicate nested":        strings.Replace(string(input), `"address":"A1"`, `"address":"A1","address":"B1"`, 1),
		"unknown nested":          strings.Replace(string(input), `"address":"A1"`, `"address":"A1","secret":true`, 1),
		"unknown style":           strings.Replace(string(input), `"address":"A1"`, `"style":{"future":true},"address":"A1"`, 1),
		"value null":              strings.Replace(string(input), `"value":false`, `"value":null`, 1),
		"null optional":           strings.Replace(string(input), `"address":"A1"`, `"style":null,"address":"A1"`, 1),
		"formula with value":      strings.Replace(string(input), `"formula":"SUM(B1:B2)"`, `"formula":"SUM(B1:B2)","value":0`, 1),
		"text with empty formula": strings.Replace(string(input), `"address":"A1"`, `"address":"A1","formula":""`, 1),
		"number string":           strings.Replace(string(input), `"value":0`, `"value":"0"`, 1),
		"boolean number":          strings.Replace(string(input), `"value":false`, `"value":0`, 1),
		"missing cells":           `{"sheets":[{"id":"data","name":"Data"}]}`,
		"trailing":                string(input) + `{}`,
		"invalid unicode":         strings.Replace(string(input), "中文 数据", `\ud800`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseWorkbook([]byte(body)); err == nil {
				t.Fatal("invalid JSON admitted")
			}
		})
	}
}

func TestWorkbookValidationBoundsAndIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*Workbook){
		"duplicate id": func(w *Workbook) { s := w.Sheets[0]; s.Name = "other"; w.Sheets = append(w.Sheets, s) },
		"duplicate case name": func(w *Workbook) {
			w.Sheets[0].Name = "DATA"
			s := w.Sheets[0]
			s.ID = "other"
			s.Name = "data"
			w.Sheets = append(w.Sheets, s)
		},
		"bad id":          func(w *Workbook) { w.Sheets[0].ID = "../data" },
		"bad name":        func(w *Workbook) { w.Sheets[0].Name = "bad/name" },
		"long UTF16 name": func(w *Workbook) { w.Sheets[0].Name = strings.Repeat("😀", 16) },
		"row limit":       func(w *Workbook) { w.Sheets[0].Cells[0].Address = "A100001" },
		"column limit":    func(w *Workbook) { w.Sheets[0].Cells[0].Address = "AMK1" },
		"duplicate cell": func(w *Workbook) {
			w.Sheets[0].Cells = append(w.Sheets[0].Cells, Cell{Address: "a1", Type: "text", Value: "duplicate"})
		},
		"nan":             func(w *Workbook) { w.Sheets[0].Cells[1].Value = math.NaN() },
		"inf":             func(w *Workbook) { w.Sheets[0].Cells[1].Value = math.Inf(1) },
		"style color":     func(w *Workbook) { w.Sheets[0].Cells[0].Style = &CellStyle{FontColor: "red"} },
		"style alignment": func(w *Workbook) { w.Sheets[0].Cells[0].Style = &CellStyle{Align: "justify"} },
		"text control":    func(w *Workbook) { w.Sheets[0].Cells[0].Value = "bad\x00text" },
		"too much text":   func(w *Workbook) { w.Sheets[0].Cells[0].Value = strings.Repeat("x", 32768) },
		"frozen rows":     func(w *Workbook) { w.Sheets[0].FrozenRows = WorkbookMaxRows },
		"column width":    func(w *Workbook) { w.Sheets[0].Columns = []float64{math.Inf(1)} },
		"too many sheets": func(w *Workbook) {
			for i := 1; i <= WorkbookMaxSheets; i++ {
				w.Sheets = append(w.Sheets, Sheet{ID: fmt.Sprintf("s%d", i), Name: fmt.Sprintf("s%d", i)})
			}
		},
		"too many cells": func(w *Workbook) {
			for i := 1; i <= WorkbookMaxCells; i++ {
				w.Sheets[0].Cells = append(w.Sheets[0].Cells, Cell{Address: fmt.Sprintf("Z%d", i), Type: "number", Value: 0})
			}
		},
		"sparse expansion": func(w *Workbook) {
			w.Sheets[0].Cells = nil
			for i := 1; i <= 977; i++ {
				w.Sheets[0].Cells = append(w.Sheets[0].Cells, Cell{Address: fmt.Sprintf("AMJ%d", i), Type: "number", Value: 0})
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := workbookFixture()
			mutate(&w)
			if err := w.Validate(); err == nil {
				t.Fatal("invalid workbook admitted")
			}
		})
	}
	w := workbookFixture()
	w.Sheets[0].Name = strings.Repeat("中", 31)
	w.Sheets[0].Cells[0].Address = "AMJ100000"
	if err := w.Validate(); err != nil {
		t.Fatal("valid coordinate or UTF16 boundary rejected", err)
	}
}

func TestWorkbookFormulaClosedGrammarAndDependencies(t *testing.T) {
	for _, formula := range []string{"1+2*3^2", "SUM($B$1:B2)", "AVERAGE(B1:B2)", "MIN(B1,B2)", "MAX(B1,B2)", "COUNT(B1:B2)", "COUNTA(A1:B2)", `COUNTIF(B1:B2,">0")`, `SUMIF(B1:B2,">0",B1:B2)`, `IF(B1>0,ROUND(ABS(-B1),2),0)`, "'中文 数据'!B1+1", "TRUE", `IF(TRUE,"a""b","c")`} {
		w := workbookFixture()
		w.Sheets[0].Cells[4].Formula = formula
		if err := w.Validate(); err != nil {
			t.Fatalf("valid formula %q: %v", formula, err)
		}
	}
	for _, formula := range []string{"", "=", "1+", "SUM()", "ROUND(1)", "ABS(1,2)", "UNKNOWN(1)", `WEBSERVICE("https://invalid.example")`, "[other.xlsx]Sheet1!A1", "'https://invalid.example'!A1", "cmd|' /C run'!A1", "Missing!A1", "SUM(A:A)", "SUM(1:100)", "SUM(A1:A10001)", "AMK1", "A100001", "SUM(A5:A1)", "D1+1", "IF(FALSE,D1,1)", "SUM(A1:D1)", "1e309", "A1#", "@A1", "SUM(A1:B2,C1:D1)", strings.Repeat("(", 65) + "1" + strings.Repeat(")", 65)} {
		t.Run(formula, func(t *testing.T) {
			w := workbookFixture()
			w.Sheets[0].Cells[4].Formula = formula
			if err := w.Validate(); err == nil {
				t.Fatal("unsafe/unsupported formula admitted")
			}
		})
	}
	w := workbookFixture()
	w.Sheets[0].Cells[4].Formula = "Other!A1"
	w.Sheets = append(w.Sheets, Sheet{ID: "other", Name: "Other", Cells: []Cell{{Address: "A1", Type: "formula", Formula: "'中文 数据'!D1"}}})
	if err := w.Validate(); err == nil {
		t.Fatal("cross-sheet cycle admitted")
	}
	w.Sheets[1].Cells[0] = Cell{Address: "A1", Type: "number", Value: 2}
	if err := w.Validate(); err != nil {
		t.Fatal("valid forward cross-sheet reference rejected", err)
	}
}

func TestWorkbookFormulaAggregateBudgets(t *testing.T) {
	w := Workbook{Sheets: []Sheet{{ID: "data", Name: "Data"}}}
	for i := 1; i <= 101; i++ {
		w.Sheets[0].Cells = append(w.Sheets[0].Cells, Cell{Address: fmt.Sprintf("B%d", i), Type: "formula", Formula: "SUM(A1:A10000)"})
	}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "bounded formula") {
		t.Fatal("aggregate reference budget not enforced", err)
	}
	w.Sheets[0].Cells = nil
	for i := 1; i <= 65; i++ {
		w.Sheets[0].Cells = append(w.Sheets[0].Cells, Cell{Address: fmt.Sprintf("A%d", i), Type: "formula", Formula: fmt.Sprintf("A%d+1", i+1)})
	}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "bounded formula") {
		t.Fatal("dependency depth budget not enforced", err)
	}
}

func TestWorkbookNativeChartBounds(t *testing.T) {
	for name, mutate := range map[string]func(*Chart){
		"unknown":        func(c *Chart) { c.Type = "radar" },
		"external":       func(c *Chart) { c.Series[0].Values = "[remote.xlsx]Sheet1!A1:A2" },
		"other sheet":    func(c *Chart) { c.Series[0].Categories = "Other!A1:A2" },
		"count mismatch": func(c *Chart) { c.Series[0].Values = "B1:B3" },
		"matrix":         func(c *Chart) { c.Series[0].Values = "B1:C2" },
		"range budget":   func(c *Chart) { c.Series[0].Values = "B1:B10001" },
		"name reference": func(c *Chart) { c.Series[0].Name = "Sheet1!A1" },
		"name literal":   func(c *Chart) { c.Series[0].Name = "收入" },
		"name missing":   func(c *Chart) { c.Series[0].Name = "A99" },
		"name numeric":   func(c *Chart) { c.Series[0].Name = "B1" },
		"name boolean":   func(c *Chart) { c.Series[0].Name = "C1" },
		"name formula":   func(c *Chart) { c.Series[0].Name = "D1" },
		"pie series":     func(c *Chart) { c.Type = "pie"; c.Series = append(c.Series, c.Series[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			w := workbookFixture()
			c := Chart{ID: "chart", Type: "bar", Anchor: "F2", Series: []ChartSeries{{Name: "A1", Categories: "A1:A2", Values: "B1:B2"}}}
			mutate(&c)
			w.Sheets[0].Charts = []Chart{c}
			if err := w.Validate(); err == nil {
				t.Fatal("invalid chart admitted")
			}
		})
	}
	w := workbookFixture()
	w.Sheets[0].Charts = []Chart{{ID: "chart", Type: "bar", Anchor: "F2", Series: []ChartSeries{{Name: "$A$1", Categories: "A1:A2", Values: "B1:B2"}}}}
	if err := w.Validate(); err != nil {
		t.Fatal("existing text label reference rejected", err)
	}
	for _, value := range []string{"", "  "} {
		w.Sheets[0].Cells[0].Value = value
		if err := w.Validate(); err == nil {
			t.Fatal("empty label reference admitted")
		}
	}
}

func TestWorkbookSUMIFRejectsImplicitRangeExpansion(t *testing.T) {
	for _, expression := range []string{`SUMIF(A1:A2,">0",B1:B1)`, `SUMIF(A1:A2,">0",B1:C1)`, `SUMIF(IF(1,A1:A2,A1:A2),">0",B1:B2)`} {
		w := Workbook{Sheets: []Sheet{{ID: "data", Name: "Data", Cells: []Cell{{Address: "A1", Type: "number", Value: 1}, {Address: "A2", Type: "number", Value: 1}, {Address: "B1", Type: "number", Value: 5}, {Address: "B2", Type: "formula", Formula: expression}}}}}
		if w.Validate() == nil {
			t.Fatal("implicit or dynamic SUMIF range was admitted", expression)
		}
	}
}

func TestWorkbookRejectsUnicodeSheetIdentityAliases(t *testing.T) {
	for _, names := range [][2]string{{"Σ", "ς"}, {"S", "ſ"}} {
		w := Workbook{Sheets: []Sheet{{ID: "one", Name: names[0], Cells: []Cell{{Address: "A1", Type: "number", Value: 1}}}, {ID: "two", Name: names[1], Cells: []Cell{{Address: "A1", Type: "number", Value: 2}}}}}
		if w.Validate() == nil {
			t.Fatal("EqualFold sheet collision admitted")
		}
	}
	w := Workbook{Sheets: []Sheet{{ID: "one", Name: "Σ", Cells: []Cell{{Address: "A1", Type: "number", Value: 1}, {Address: "B1", Type: "formula", Formula: "'ς'!A1+1"}}}}}
	if err := w.Validate(); err != nil {
		t.Fatal("same sheet alias rejected", err)
	}
	w.Sheets[0].Cells[1].Formula = "'ς'!B1+1"
	if w.Validate() == nil {
		t.Fatal("alias bypassed circular reference check")
	}
}
