package workbookcodec

import (
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"context"
	"github.com/xuri/excelize/v2"
	"testing"
)

func patchSelection() office.WorkbookSelection {
	return office.WorkbookSelection{SheetName: "Sheet1", EndRow: 2, Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}, {Row: 1, ValueType: "number", Value: 2, Text: "2", Formula: "2", RowVisible: true, ColumnVisible: true}, {Row: 2, ValueType: "empty", RowVisible: true, ColumnVisible: true}}}
}
func TestTypedWorkbookActualCalculationAndExportValidation(t *testing.T) {
	ctx := context.Background()
	s := patchSelection()
	r, e := ValidatePatch(ctx, s, office.WorkbookPatch{Kind: "range", Cells: []office.WorkbookCellEdit{{RowOffset: 2, Type: "formula", Formula: "SUM(A1:A2)"}}})
	if e != nil || r.Results[2] != "3" {
		t.Fatal(r, e)
	}
	if _, e = ValidatePatch(ctx, s, office.WorkbookPatch{Kind: "range", Cells: []office.WorkbookCellEdit{{RowOffset: 2, Type: "formula", Formula: "1/0"}}}); e == nil {
		t.Fatal("formula error accepted")
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellFloat("Sheet1", "A1", 1, -1, 64)
	f.SetCellFloat("Sheet1", "A2", 2, -1, 64)
	f.SetCellStr("Sheet1", "D4", "outside")
	original, _ := f.WriteToBuffer()
	if e = VerifySelectionPackage(ctx, original.Bytes(), s); e != nil {
		t.Fatal(e)
	}
	f.SetCellFormula("Sheet1", "A3", "SUM(A1:A2)")
	candidate, _ := f.WriteToBuffer()
	if e = VerifyPatchedPackage(ctx, original.Bytes(), candidate.Bytes(), r); e != nil {
		t.Fatal(e)
	}
	f.SetCellStr("Sheet1", "A3", "=SUM(A1:A2)")
	bad, _ := f.WriteToBuffer()
	if VerifyPatchedPackage(ctx, original.Bytes(), bad.Bytes(), r) == nil {
		t.Fatal("formula was coerced to text")
	}
	f.SetRowVisible("Sheet1", 1, false)
	bad, _ = f.WriteToBuffer()
	if VerifySelectionPackage(ctx, bad.Bytes(), s) == nil {
		t.Fatal("hidden source admitted")
	}
}
