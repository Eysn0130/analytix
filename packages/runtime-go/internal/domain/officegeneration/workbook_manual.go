package officegeneration

// WorkbookWriteTarget identifies the one cell the user explicitly chose to edit.
// Read-only formula dependencies never enlarge this write target.
type WorkbookWriteTarget struct {
	Sheet     int    `json:"sheet"`
	SheetName string `json:"sheetName"`
	Column    int    `json:"column"`
	Row       int    `json:"row"`
}

// WorkbookManualEdit carries a typed local edit, with no selection offsets or
// model-proposed content. Empty is an explicit clearing operation.
type WorkbookManualEdit struct {
	Type    string   `json:"type"`
	Value   *float64 `json:"value,omitempty"`
	Formula string   `json:"formula,omitempty"`
	Text    *string  `json:"text,omitempty"`
}

// WorkbookDependencyBounds is the complete native rectangle Core needs to
// validate an edit. Its cells are read-only except for WorkbookWriteTarget.
type WorkbookDependencyBounds struct {
	Sheet       int `json:"sheet"`
	StartColumn int `json:"startColumn"`
	StartRow    int `json:"startRow"`
	EndColumn   int `json:"endColumn"`
	EndRow      int `json:"endRow"`
}

func ParseWorkbookWriteTarget(body []byte) (WorkbookWriteTarget, error) {
	var target WorkbookWriteTarget
	v, err := workbookPatchDecode(body, &target)
	if err != nil || !workbookPatchKeys(v, "sheet sheetName column row", "") {
		return target, workbookInvalid
	}
	return target, target.Validate()
}

func (target WorkbookWriteTarget) Validate() error {
	if target.Sheet < 0 || target.Sheet >= WorkbookMaxSheets || !workbookSheetName(target.SheetName) || target.Column < 0 || target.Column >= WorkbookMaxColumns || target.Row < 0 || target.Row >= WorkbookMaxRows {
		return workbookInvalid
	}
	return nil
}

func ParseWorkbookManualEdit(body []byte) (WorkbookManualEdit, error) {
	var edit WorkbookManualEdit
	v, err := workbookPatchDecode(body, &edit)
	if err != nil {
		return edit, err
	}
	required := "type"
	switch edit.Type {
	case "number":
		required += " value"
	case "formula":
		required += " formula"
	case "text":
		required += " text"
	case "empty":
	default:
		return edit, workbookInvalid
	}
	if !workbookPatchKeys(v, required, "") {
		return edit, workbookInvalid
	}
	return edit, edit.validate()
}

func (edit WorkbookManualEdit) validate() error {
	switch edit.Type {
	case "number":
		if edit.Value == nil || !workbookFinite(*edit.Value) || edit.Formula != "" || edit.Text != nil {
			return workbookInvalid
		}
	case "formula":
		if edit.Value != nil || edit.Text != nil || len(edit.Formula) > 1024 || !workbookText(edit.Formula, 1024) || CanonicalWorkbookFormula(edit.Formula) == "" {
			return workbookInvalid
		}
	case "text":
		if edit.Text == nil || *edit.Text == "" || edit.Value != nil || edit.Formula != "" || !workbookText(*edit.Text, 4096) {
			return workbookInvalid
		}
	case "empty":
		if edit.Value != nil || edit.Formula != "" || edit.Text != nil {
			return workbookInvalid
		}
	default:
		return workbookInvalid
	}
	return nil
}

// PlanWorkbookManual uses the existing bounded formula parser. It returns only
// a same-sheet rectangle of at most 256 cells, including the original target.
func PlanWorkbookManual(target WorkbookWriteTarget, edit WorkbookManualEdit) (WorkbookDependencyBounds, error) {
	bounds := WorkbookDependencyBounds{Sheet: target.Sheet, StartColumn: target.Column, StartRow: target.Row, EndColumn: target.Column, EndRow: target.Row}
	if err := target.Validate(); err != nil {
		return WorkbookDependencyBounds{}, err
	}
	if err := edit.validate(); err != nil {
		return WorkbookDependencyBounds{}, err
	}
	if edit.Type != "formula" {
		return bounds, nil
	}
	tokens, err := workbookFormulaTokens(CanonicalWorkbookFormula(edit.Formula))
	if err != nil {
		return WorkbookDependencyBounds{}, err
	}
	sheet := workbookSheetKey(target.SheetName)
	parser := workbookFormulaParser{tokens: tokens, sheet: sheet, names: map[string]bool{sheet: true}}
	if err := parser.expression(0); err != nil {
		return WorkbookDependencyBounds{}, err
	}
	if parser.current().kind != "end" {
		return WorkbookDependencyBounds{}, workbookInvalid
	}
	for _, ref := range parser.references {
		if ref.sheet != sheet {
			return WorkbookDependencyBounds{}, workbookInvalid
		}
		bounds.StartColumn = min(bounds.StartColumn, ref.area.c1-1)
		bounds.StartRow = min(bounds.StartRow, ref.area.r1-1)
		bounds.EndColumn = max(bounds.EndColumn, ref.area.c2-1)
		bounds.EndRow = max(bounds.EndRow, ref.area.r2-1)
		if (bounds.EndColumn-bounds.StartColumn+1)*(bounds.EndRow-bounds.StartRow+1) > 256 {
			return WorkbookDependencyBounds{}, workbookFormulaBounds
		}
	}
	return bounds, nil
}

// WorkbookManualPatch binds a fresh, complete native capture to the planned
// dependency rectangle and emits one range edit for the original write target.
func WorkbookManualPatch(selected WorkbookSelection, target WorkbookWriteTarget, edit WorkbookManualEdit) (WorkbookPatch, error) {
	bounds, err := PlanWorkbookManual(target, edit)
	if err != nil {
		return WorkbookPatch{}, err
	}
	if err := selected.Validate(); err != nil {
		return WorkbookPatch{}, err
	}
	if selected.SheetName != target.SheetName || selected.Sheet != bounds.Sheet || selected.StartColumn != bounds.StartColumn || selected.StartRow != bounds.StartRow || selected.EndColumn != bounds.EndColumn || selected.EndRow != bounds.EndRow {
		return WorkbookPatch{}, workbookInvalid
	}
	patch := WorkbookPatch{Kind: "range", Cells: []WorkbookCellEdit{{RowOffset: target.Row - bounds.StartRow, ColumnOffset: target.Column - bounds.StartColumn, Type: edit.Type, Value: edit.Value, Formula: edit.Formula, Text: edit.Text}}}
	if _, err := ApplyWorkbookPatch(selected, patch); err != nil {
		return WorkbookPatch{}, err
	}
	return patch, nil
}
