package workbookcodec

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

var patchInvalid = errors.New("workbook typed change invalid")

func ValidatePatch(ctx context.Context, before office.WorkbookSelection, patch office.WorkbookPatch) (review office.WorkbookReview, err error) {
	defer func() {
		if recover() != nil {
			review = office.WorkbookReview{}
			err = patchInvalid
		}
	}()
	after, err := office.ApplyWorkbookPatch(before, patch)
	if err != nil {
		return review, err
	}
	f := excelize.NewFile()
	defer f.Close()
	if err = f.SetSheetName("Sheet1", after.SheetName); err != nil {
		return review, patchInvalid
	}
	for _, c := range after.Cells {
		if ctx == nil || ctx.Err() != nil {
			return review, patchInvalid
		}
		address := office.WorkbookCellAddress(c.Column, c.Row)
		switch c.ValueType {
		case "number":
			err = f.SetCellFloat(after.SheetName, address, c.Value, -1, 64)
		case "formula":
			err = f.SetCellFormula(after.SheetName, address, office.CanonicalWorkbookFormula(c.Formula))
		default:
			err = f.SetCellStr(after.SheetName, address, c.Text)
		}
		if err != nil {
			return review, patchInvalid
		}
	}
	results := make([]string, len(after.Cells))
	for i, c := range after.Cells {
		if ctx.Err() != nil {
			return review, patchInvalid
		}
		if c.ValueType != "formula" {
			continue
		}
		value, e := f.CalcCellValue(after.SheetName, office.WorkbookCellAddress(c.Column, c.Row), excelize.Options{RawCellValue: true, MaxCalcIterations: 1})
		if e != nil {
			return review, patchInvalid
		}
		if n, e := strconv.ParseFloat(value, 64); e == nil && (math.IsNaN(n) || math.IsInf(n, 0)) {
			return review, patchInvalid
		}
		results[i] = value
	}
	return office.WorkbookReview{Before: before, After: after, Results: results}, nil
}
func openPatchWorkbook(body []byte) (*excelize.File, error) {
	if len(body) == 0 || len(body) > 16<<20 {
		return nil, patchInvalid
	}
	return excelize.OpenReader(bytes.NewReader(body), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 64 << 20, RawCellValue: true})
}
func VerifySelectionPackage(ctx context.Context, body []byte, s office.WorkbookSelection) (err error) {
	defer func() {
		if recover() != nil {
			err = patchInvalid
		}
	}()
	if _, e := workbookPhysical(ctx, body); e != nil {
		return e
	}
	if s.Validate() != nil {
		return patchInvalid
	}
	f, err := openPatchWorkbook(body)
	if err != nil {
		return patchInvalid
	}
	defer f.Close()
	return verifySelectedCells(ctx, f, s)
}
func verifySelectedCells(ctx context.Context, f *excelize.File, s office.WorkbookSelection) error {
	if f.GetSheetMap()[s.Sheet+1] != s.SheetName {
		return patchInvalid
	}
	merges, e := f.GetMergeCells(s.SheetName)
	if e != nil || len(merges) > 10000 {
		return patchInvalid
	}
	for _, m := range merges {
		c1, r1, e1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		c2, r2, e2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if e1 != nil || e2 != nil || c1 <= s.EndColumn+1 && c2 >= s.StartColumn+1 && r1 <= s.EndRow+1 && r2 >= s.StartRow+1 {
			return patchInvalid
		}
	}
	for _, c := range s.Cells {
		if ctx == nil || ctx.Err() != nil {
			return patchInvalid
		}
		a := office.WorkbookCellAddress(c.Column, c.Row)
		rowVisible, e1 := f.GetRowVisible(s.SheetName, c.Row+1)
		columnName, _ := excelize.ColumnNumberToName(c.Column + 1)
		columnVisible, e2 := f.GetColVisible(s.SheetName, columnName)
		if e1 != nil || e2 != nil || !rowVisible || !columnVisible {
			return patchInvalid
		}

		formula, e := f.GetCellFormula(s.SheetName, a)
		if e != nil {
			return patchInvalid
		}
		typ, e := f.GetCellType(s.SheetName, a)
		if e != nil {
			return patchInvalid
		}
		value, e := f.GetCellValue(s.SheetName, a, excelize.Options{RawCellValue: true})
		if e != nil {
			return patchInvalid
		}
		switch c.ValueType {
		case "formula":
			if formula == "" || office.CanonicalWorkbookFormula(formula) != office.CanonicalWorkbookFormula(c.Formula) {
				return patchInvalid
			}
		case "number":
			n, e := strconv.ParseFloat(value, 64)
			if formula != "" || e != nil || n != c.Value || (typ != excelize.CellTypeNumber && typ != excelize.CellTypeUnset) {
				return patchInvalid
			}
		case "text":
			if formula != "" || value != c.Text || (typ != excelize.CellTypeSharedString && typ != excelize.CellTypeInlineString) {
				return patchInvalid
			}
		case "empty":
			if formula != "" || value != "" {
				return patchInvalid
			}
		default:
			return patchInvalid
		}
	}
	return nil
}

// ValidateReview checks persisted structured metadata, including its declared calculations.
func ValidateReview(ctx context.Context, r office.WorkbookReview) error {
	if r.Before.Validate() != nil || r.After.Validate() != nil || len(r.Results) != len(r.After.Cells) {
		return patchInvalid
	}
	a, b := r.Before, r.After
	if a.Sheet != b.Sheet || a.SheetName != b.SheetName || a.StartColumn != b.StartColumn || a.EndColumn != b.EndColumn || a.StartRow != b.StartRow || a.EndRow != b.EndRow {
		return patchInvalid
	}
	edits := []office.WorkbookCellEdit{}
	for i, c := range b.Cells {
		old := a.Cells[i]
		if c.NumberFormat != old.NumberFormat {
			return patchInvalid
		}
		if reflect.DeepEqual(c, old) {
			continue
		}
		e := office.WorkbookCellEdit{RowOffset: c.Row - a.StartRow, ColumnOffset: c.Column - a.StartColumn, Type: c.ValueType}
		switch c.ValueType {
		case "number":
			v := c.Value
			e.Value = &v
		case "formula":
			e.Formula = c.Formula
		case "text":
			v := c.Text
			e.Text = &v
		default:
			return patchInvalid
		}
		edits = append(edits, e)
	}
	if len(edits) == 0 {
		return patchInvalid
	}
	computed, err := ValidatePatch(ctx, a, office.WorkbookPatch{Kind: "range", Cells: edits})
	if err != nil || !reflect.DeepEqual(computed.After, b) || !reflect.DeepEqual(computed.Results, r.Results) {
		return patchInvalid
	}
	return nil
}

// VerifyPatchedPackage reads exported bytes only; it never rewrites an imported workbook.
func VerifyPatchedPackage(ctx context.Context, original, candidate []byte, r office.WorkbookReview) (err error) {
	defer func() {
		if recover() != nil {
			err = patchInvalid
		}
	}()
	if ValidateReview(ctx, r) != nil || VerifySelectionPackage(ctx, original, r.Before) != nil {
		return patchInvalid
	}
	before, e := openPatchWorkbook(original)
	if e != nil {
		return patchInvalid
	}
	defer before.Close()
	after, e := openPatchWorkbook(candidate)
	if e != nil {
		return patchInvalid
	}
	defer after.Close()
	if e = verifyWorkbookOutsideScope(ctx, before, after, original, candidate, r.Before); e != nil {
		return e
	}
	if verifySelectedCells(ctx, after, r.After) != nil {
		return patchInvalid
	}
	for i, c := range r.After.Cells {
		a := office.WorkbookCellAddress(c.Column, c.Row)
		oldID, e1 := before.GetCellStyle(r.Before.SheetName, a)
		newID, e2 := after.GetCellStyle(r.After.SheetName, a)
		if e1 != nil || e2 != nil {
			return patchInvalid
		}
		oldStyle, e1 := before.GetStyle(oldID)
		newStyle, e2 := after.GetStyle(newID)
		if e1 != nil || e2 != nil || oldStyle.NumFmt != newStyle.NumFmt || !reflect.DeepEqual(oldStyle.CustomNumFmt, newStyle.CustomNumFmt) {
			return patchInvalid
		}
		if c.ValueType == "formula" {
			value, e := after.CalcCellValue(r.After.SheetName, a, excelize.Options{RawCellValue: true, MaxCalcIterations: 1})
			if e != nil || strings.TrimSpace(value) != strings.TrimSpace(r.Results[i]) {
				return patchInvalid
			}
		}
	}
	return nil
}
