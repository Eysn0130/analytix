package officegeneration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

// WorkbookSelection is a complete, engine-owned rectangle, never a model selector.
type WorkbookSelection struct {
	Sheet       int                    `json:"sheet"`
	SheetName   string                 `json:"sheetName"`
	StartColumn int                    `json:"startColumn"`
	StartRow    int                    `json:"startRow"`
	EndColumn   int                    `json:"endColumn"`
	EndRow      int                    `json:"endRow"`
	Cells       []WorkbookSelectedCell `json:"cells"`
}
type WorkbookSelectedCell struct {
	Sheet         int     `json:"sheet"`
	Column        int     `json:"column"`
	Row           int     `json:"row"`
	Text          string  `json:"text"`
	Formula       string  `json:"formula"`
	Value         float64 `json:"value"`
	ValueType     string  `json:"valueType"`
	NumberFormat  int     `json:"numberFormat"`
	RowVisible    bool    `json:"rowVisible"`
	ColumnVisible bool    `json:"columnVisible"`
	Merged        bool    `json:"merged"`
}
type WorkbookCellEdit struct {
	RowOffset    int      `json:"rowOffset"`
	ColumnOffset int      `json:"columnOffset"`
	Type         string   `json:"type"`
	Value        *float64 `json:"value,omitempty"`
	Formula      string   `json:"formula,omitempty"`
	Text         *string  `json:"text,omitempty"`
}
type WorkbookPatch struct {
	Kind    string             `json:"kind"`
	Value   *float64           `json:"value,omitempty"`
	Formula string             `json:"formula,omitempty"`
	Cells   []WorkbookCellEdit `json:"cells,omitempty"`
}
type WorkbookReview struct {
	Before WorkbookSelection `json:"before"`
	After  WorkbookSelection `json:"after"`
	// Actual Core calculation results, local display only; not native cached values.
	Results []string `json:"results"`
}

func workbookPatchKeys(v map[string]any, required string, optional string) bool {
	allowed := map[string]bool{}
	for _, k := range strings.Fields(required) {
		if value, ok := v[k]; !ok || value == nil {
			return false
		}
		allowed[k] = true
	}
	for _, k := range strings.Fields(optional) {
		allowed[k] = true
	}
	for k := range v {
		if !allowed[k] {
			return false
		}
	}
	return true
}
func workbookPatchDecode(body []byte, target any) (map[string]any, error) {
	v, err := jsonstrict.DecodeObject(body, jsonstrict.Options{MaxBytes: 512 << 10, MaxDepth: 6, MaxTokens: 16384, MaxStringBytes: 65536, MaxNumberBytes: 128, MaxAbsExponent: 308})
	if err != nil {
		return nil, workbookInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return nil, workbookInvalid
	}
	return v, nil
}
func ParseWorkbookSelection(body []byte) (WorkbookSelection, error) {
	var s WorkbookSelection
	v, err := workbookPatchDecode(body, &s)
	if err != nil || !workbookPatchKeys(v, "sheet sheetName startColumn startRow endColumn endRow cells", "") {
		return s, workbookInvalid
	}
	cells, ok := v["cells"].([]any)
	if !ok {
		return s, workbookInvalid
	}
	for _, c := range cells {
		m, ok := c.(map[string]any)
		if !ok || !workbookPatchKeys(m, "sheet column row text formula value valueType numberFormat rowVisible columnVisible merged", "") {
			return s, workbookInvalid
		}
	}
	return s, s.Validate()
}
func ParseWorkbookPatch(body []byte) (WorkbookPatch, error) {
	var p WorkbookPatch
	v, err := workbookPatchDecode(body, &p)
	if err != nil {
		return p, err
	}
	switch p.Kind {
	case "number":
		if !workbookPatchKeys(v, "kind value", "") || p.Value == nil {
			return p, workbookInvalid
		}
	case "formula":
		if !workbookPatchKeys(v, "kind formula", "") || p.Formula == "" {
			return p, workbookInvalid
		}
	case "range":
		if !workbookPatchKeys(v, "kind cells", "") || len(p.Cells) == 0 || len(p.Cells) > 256 {
			return p, workbookInvalid
		}
		for _, raw := range v["cells"].([]any) {
			m, ok := raw.(map[string]any)
			if !ok {
				return p, workbookInvalid
			}
			field := map[string]string{"number": "value", "formula": "formula", "text": "text"}[fmt.Sprint(m["type"])]
			if field == "" || !workbookPatchKeys(m, "rowOffset columnOffset type "+field, "") || m[field] == nil {
				return p, workbookInvalid
			}
		}
	default:
		return p, workbookInvalid
	}
	return p, nil
}

// CanonicalWorkbookFormula translates only separators outside quoted literals.
// Reference spelling is preserved; the bounded parser validates scope separately.
func CanonicalWorkbookFormula(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "=")
	var out strings.Builder
	quoted := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '"' {
			if quoted && i+1 < len(value) && value[i+1] == '"' {
				out.WriteString("\"\"")
				i++
				continue
			}
			quoted = !quoted
		}
		if c == ';' && !quoted {
			c = ','
		}
		out.WriteByte(c)
	}
	return out.String()
}
func (s WorkbookSelection) Validate() error {
	if s.Sheet < 0 || s.Sheet >= 20 || !workbookSheetName(s.SheetName) || s.StartColumn < 0 || s.EndColumn >= WorkbookMaxColumns || s.StartRow < 0 || s.EndRow >= WorkbookMaxRows || s.EndColumn < s.StartColumn || s.EndRow < s.StartRow {
		return workbookInvalid
	}
	count := (s.EndColumn - s.StartColumn + 1) * (s.EndRow - s.StartRow + 1)
	if count < 1 || count > 256 || len(s.Cells) != count {
		return workbookInvalid
	}
	total := 0
	for i, c := range s.Cells {
		if c.Sheet != s.Sheet || c.Column != s.StartColumn+i%(s.EndColumn-s.StartColumn+1) || c.Row != s.StartRow+i/(s.EndColumn-s.StartColumn+1) || c.NumberFormat < 0 || c.NumberFormat > 9007199254740991 || !c.RowVisible || !c.ColumnVisible || c.Merged || math.IsNaN(c.Value) || math.IsInf(c.Value, 0) || !workbookText(c.Text, 4096) || !workbookText(c.Formula, 4096) {
			return workbookInvalid
		}
		total += len(c.Text) + len(c.Formula)
		switch c.ValueType {
		case "empty":
			if c.Text != "" || c.Formula != "" || c.Value != 0 {
				return workbookInvalid
			}
		case "text", "number":
		case "formula":
			if len(c.Formula) > 1024 || CanonicalWorkbookFormula(c.Formula) == "" {
				return workbookInvalid
			}
		default:
			return workbookInvalid
		}
	}
	if total > 65536 {
		return workbookInvalid
	}
	return nil
}
func (s WorkbookSelection) Workbook() Workbook {
	cells := make([]Cell, 0, len(s.Cells))
	for _, c := range s.Cells {
		v := Cell{Address: WorkbookCellAddress(c.Column, c.Row), Type: c.ValueType}
		switch c.ValueType {
		case "empty":
			v.Type = "text"
			v.Value = ""
		case "text":
			v.Value = c.Text
		case "number":
			v.Value = c.Value
		case "formula":
			v.Formula = CanonicalWorkbookFormula(c.Formula)
		}
		cells = append(cells, v)
	}
	return Workbook{Sheets: []Sheet{{ID: "selection", Name: s.SheetName, Cells: cells}}}
}
func WorkbookCellAddress(column, row int) string {
	letters := ""
	for column++; column > 0; column = (column - 1) / 26 {
		letters = string(rune('A'+(column-1)%26)) + letters
	}
	return fmt.Sprintf("%s%d", letters, row+1)
}
func ValidateWorkbookSelectionFormulas(s WorkbookSelection) error {
	if err := s.Validate(); err != nil {
		return err
	}
	w := s.Workbook()
	if err := w.Validate(); err != nil {
		return err
	}
	names := map[string]bool{workbookSheetKey(s.SheetName): true}
	for _, c := range s.Cells {
		if c.ValueType != "formula" {
			continue
		}
		tokens, err := workbookFormulaTokens(CanonicalWorkbookFormula(c.Formula))
		if err != nil {
			return err
		}
		p := workbookFormulaParser{tokens: tokens, sheet: workbookSheetKey(s.SheetName), names: names}
		if err := p.expression(0); err != nil {
			return err
		}
		for _, ref := range p.references {
			if ref.sheet != workbookSheetKey(s.SheetName) || ref.area.c1 < s.StartColumn+1 || ref.area.c2 > s.EndColumn+1 || ref.area.r1 < s.StartRow+1 || ref.area.r2 > s.EndRow+1 {
				return workbookInvalid
			}
		}
	}
	return nil
}
func ApplyWorkbookPatch(s WorkbookSelection, p WorkbookPatch) (WorkbookSelection, error) {
	if err := s.Validate(); err != nil {
		return s, err
	}
	after := s
	after.Cells = append([]WorkbookSelectedCell{}, s.Cells...)
	edits := p.Cells
	if p.Kind != "range" {
		if len(s.Cells) != 1 {
			return s, workbookInvalid
		}
		edits = []WorkbookCellEdit{{Type: p.Kind, Value: p.Value, Formula: p.Formula}}
	}
	if len(edits) == 0 || len(edits) > 256 {
		return s, workbookInvalid
	}
	seen := map[int]bool{}
	for _, e := range edits {
		if e.RowOffset < 0 || e.ColumnOffset < 0 || e.RowOffset > s.EndRow-s.StartRow || e.ColumnOffset > s.EndColumn-s.StartColumn {
			return s, workbookInvalid
		}
		i := e.RowOffset*(s.EndColumn-s.StartColumn+1) + e.ColumnOffset
		if seen[i] {
			return s, workbookInvalid
		}
		seen[i] = true
		c := &after.Cells[i]
		switch e.Type {
		case "number":
			if e.Value == nil || math.IsNaN(*e.Value) || math.IsInf(*e.Value, 0) || e.Formula != "" || e.Text != nil {
				return s, workbookInvalid
			}
			c.Value = *e.Value
			c.Text = fmt.Sprint(*e.Value)
			c.Formula = c.Text
		case "formula":
			if e.Value != nil || e.Text != nil || len(e.Formula) > 1024 || CanonicalWorkbookFormula(e.Formula) == "" {
				return s, workbookInvalid
			}
			c.Formula = "=" + CanonicalWorkbookFormula(e.Formula)
			c.Text = ""
			c.Value = 0
		case "text":
			if e.Text == nil || e.Value != nil || e.Formula != "" || (c.ValueType != "empty" && c.ValueType != "text") {
				return s, workbookInvalid
			}
			c.Text = *e.Text
			c.Formula = c.Text
			c.Value = 0
		default:
			return s, workbookInvalid
		}
		c.ValueType = e.Type
	}
	return after, ValidateWorkbookSelectionFormulas(after)
}
