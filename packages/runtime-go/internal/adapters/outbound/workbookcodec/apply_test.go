package workbookcodec

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

func applyFixtureAgentReview(t *testing.T) office.WorkbookReview {
	t.Helper()
	before := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 2, Text: "2", Formula: "2", RowVisible: true, ColumnVisible: true}}}
	value := 42.0
	review, err := ValidatePatch(context.Background(), before, office.WorkbookPatch{Kind: "number", Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	return review
}

func TestApplyReviewRealFixtureManualAndAgentTypedChanges(t *testing.T) {
	ctx := context.Background()
	original, _ := admissionNativeExportFixtures(t)
	unchanged := append([]byte(nil), original...)
	for name, review := range map[string]office.WorkbookReview{
		"manual_C2_number11_to_text001": admissionNativeManualReview(t),
		"agent_A1_number2_to_number42":  applyFixtureAgentReview(t),
	} {
		t.Run(name, func(t *testing.T) {
			output, err := ApplyReview(ctx, original, review)
			if err != nil {
				t.Fatal("typed source-package application rejected the actual bounded fixture", err)
			}
			if len(output) == 0 || len(output) > MaxOutputBytes || !bytes.Equal(original, unchanged) {
				t.Fatal("typed application mutated the input or escaped the output budget")
			}
			if err := VerifyPatchedPackage(ctx, original, output, review); err != nil {
				t.Fatal("returned output failed the complete preservation verifier", err)
			}
			if err := VerifySelectionPackage(ctx, output, review.After); err != nil {
				t.Fatal("returned output cannot be captured for a subsequent typed edit", err)
			}
			before, err := openPatchWorkbook(original)
			if err != nil {
				t.Fatal(err)
			}
			defer before.Close()
			after, err := openPatchWorkbook(output)
			if err != nil {
				t.Fatal(err)
			}
			defer after.Close()
			for _, address := range []string{"A1", "C2"} {
				oldID, e1 := before.GetCellStyle("Sheet1", address)
				newID, e2 := after.GetCellStyle("Sheet1", address)
				oldStyle, e3 := workbookSemanticStyle(before, oldID)
				newStyle, e4 := workbookSemanticStyle(after, newID)
				if e1 != nil || e2 != nil || e3 != nil || e4 != nil || oldStyle != newStyle {
					t.Fatal("typed source application changed an effective font or style", address, e1, e2, e3, e4)
				}
			}
		})
	}
}

func TestApplyReviewOutputCanBeNextInput(t *testing.T) {
	ctx := context.Background()
	original, _ := admissionNativeExportFixtures(t)
	first := admissionNativeManualReview(t)
	output, err := ApplyReview(ctx, original, first)
	if err != nil {
		t.Fatal(err)
	}
	value := 12.0
	second, err := ValidatePatch(ctx, first.After, office.WorkbookPatch{Kind: "number", Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	next, err := ApplyReview(ctx, output, second)
	if err != nil {
		t.Fatal("the first typed output cannot be edited again", err)
	}
	if err := VerifyPatchedPackage(ctx, output, next, second); err != nil {
		t.Fatal(err)
	}
	if err := VerifySelectionPackage(ctx, next, second.After); err != nil {
		t.Fatal(err)
	}
}

func TestApplyReviewTypedSettersClearFormulaAndCachedValue(t *testing.T) {
	ctx := context.Background()
	original, _ := admissionNativeExportFixtures(t)
	before := office.WorkbookSelection{SheetName: "Sheet1", EndColumn: 2, Cells: []office.WorkbookSelectedCell{
		{ValueType: "number", Value: 2, Text: "2", Formula: "2", RowVisible: true, ColumnVisible: true},
		{Column: 1, ValueType: "number", Value: 7, Text: "7", Formula: "7", RowVisible: true, ColumnVisible: true},
		{Column: 2, ValueType: "formula", Value: 9, Text: "9", Formula: "=SUM(A1:B1)", RowVisible: true, ColumnVisible: true},
	}}
	value, text := 12.0, "=SUM(A1:B1)"
	for name, edit := range map[string]office.WorkbookCellEdit{
		"empty":                {ColumnOffset: 2, Type: "empty"},
		"text_formula_literal": {ColumnOffset: 2, Type: "text", Text: &text},
		"number":               {ColumnOffset: 2, Type: "number", Value: &value},
		"formula":              {ColumnOffset: 2, Type: "formula", Formula: "=A1+B1+1"},
	} {
		t.Run(name, func(t *testing.T) {
			review, err := ValidatePatch(ctx, before, office.WorkbookPatch{Kind: "range", Cells: []office.WorkbookCellEdit{edit}})
			if err != nil {
				t.Fatal(err)
			}
			output, err := ApplyReview(ctx, original, review)
			if err != nil {
				t.Fatal("typed setter did not produce a completely verified workbook", err)
			}
			f, err := openPatchWorkbook(output)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			formula, err := f.GetCellFormula("Sheet1", "C1")
			if err != nil {
				t.Fatal(err)
			}
			if edit.Type == "formula" {
				cached, err := f.GetCellValue("Sheet1", "C1", excelize.Options{RawCellValue: true})
				if err != nil || formula != "A1+B1+1" || cached != "" {
					t.Fatal("formula replacement retained the old cached value", formula, cached, err)
				}
			} else if formula != "" {
				t.Fatal("non-formula typed setter retained a formula", formula)
			}
			if err := VerifySelectionPackage(ctx, output, review.After); err != nil {
				t.Fatal("setter changed the declared output type", err)
			}
		})
	}
}

func TestApplyReviewRejectsUnboundInvalidAndUnsupportedChanges(t *testing.T) {
	ctx := context.Background()
	original, _ := admissionNativeExportFixtures(t)
	for name, review := range map[string]office.WorkbookReview{
		"unbound_source": func() office.WorkbookReview {
			review := admissionNativeManualReview(t)
			review.Before.Cells[0].Value = 99
			return review
		}(),
		"nonfinite_number": func() office.WorkbookReview {
			review := applyFixtureAgentReview(t)
			review.After.Cells[0].Value = math.Inf(1)
			return review
		}(),
		"changed_number_format": func() office.WorkbookReview {
			review := admissionNativeManualReview(t)
			review.After.Cells[0].NumberFormat++
			return review
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			body, err := ApplyReview(ctx, original, review)
			if err == nil || body != nil {
				t.Fatal("invalid typed review returned candidate bytes", err)
			}
		})
	}
	complex := admissionRepack(t, original, func(parts map[string][]byte) { parts["xl/comments1.xml"] = []byte("<comments/>") })
	if body, err := ApplyReview(ctx, complex, admissionNativeManualReview(t)); body != nil || !errors.Is(err, ErrUnsupportedWorkbook) {
		t.Fatal("typed source application widened feature admission", err)
	}
	if body, err := ApplyReview(nil, original, admissionNativeManualReview(t)); body != nil || err == nil {
		t.Fatal("nil context returned candidate bytes", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if body, err := ApplyReview(canceled, original, admissionNativeManualReview(t)); body != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled application returned candidate bytes", err)
	}
}

func TestApplyReviewRejectsUnchangedTypedSemantics(t *testing.T) {
	ctx := context.Background()
	original, _ := admissionNativeExportFixtures(t)
	before := applyFixtureAgentReview(t).Before
	before.Cells[0].Text = "2.00"
	value := 2.0
	review, err := ValidatePatch(ctx, before, office.WorkbookPatch{Kind: "number", Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	output, err := ApplyReview(ctx, original, review)
	if !errors.Is(err, patchInvalid) || output != nil {
		t.Fatal("display-only metadata produced a same-revision save candidate", err)
	}
}
