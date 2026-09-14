package workbookcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

func scopePackage(t *testing.T, value float64, styleOrder bool) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellFloat("Sheet1", "A1", value, -1, 64)
	f.SetCellFloat("Sheet1", "D4", 8, -1, 64)
	f.SetCellFormula("Sheet1", "D5", "A1+1")
	if styleOrder {
		f.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true}})
	}
	id, e := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if e != nil {
		t.Fatal(e)
	}
	f.SetCellStyle("Sheet1", "D4", "D4", id)
	b, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func scopeRewrite(t *testing.T, body []byte, part string, edit func(string) string) []byte {
	t.Helper()
	r, e := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, entry := range r.File {
		rd, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(rd)
		rd.Close()
		if e != nil {
			t.Fatal(e)
		}
		if entry.Name == part {
			raw = []byte(edit(string(raw)))
		}
		dst, e := w.Create(entry.Name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = dst.Write(raw); e != nil {
			t.Fatal(e)
		}
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
func TestTypedWorkbookOutsideCellSemanticsAndRecalculationCache(t *testing.T) {
	ctx := context.Background()
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, e := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if e != nil {
		t.Fatal(e)
	}
	original, candidate := scopePackage(t, 1, false), scopePackage(t, 4, true)
	if e = VerifyPatchedPackage(ctx, original, candidate, review); e != nil {
		t.Fatal("semantic styles with changed XF IDs rejected", e)
	}
	for name, edit := range map[string]func(string) string{
		"outside_number":  func(s string) string { return strings.Replace(s, "<v>8</v>", "<v>9</v>", 1) },
		"outside_formula": func(s string) string { return strings.Replace(s, "<f>A1+1</f>", "<f>A1+2</f>", 1) },
		"outside_format":  func(s string) string { return strings.Replace(s, `r="D4" s="2"`, `r="D4" s="1"`, 1) },
		"hidden_row":      func(s string) string { return strings.Replace(s, `<row r="4"`, `<row hidden="true" r="4"`, 1) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := scopeRewrite(t, candidate, "xl/worksheets/sheet1.xml", edit)
			if bytes.Equal(bad, candidate) {
				t.Fatal("fixture did not mutate")
			}
			if VerifyPatchedPackage(ctx, original, bad, review) == nil {
				t.Fatal("outside-scope change admitted")
			}
		})
	}
	cachedBefore := scopeRewrite(t, original, "xl/worksheets/sheet1.xml", func(s string) string { return strings.Replace(s, "<f>A1+1</f>", "<f>A1+1</f><v>2</v>", 1) })
	cachedAfter := scopeRewrite(t, candidate, "xl/worksheets/sheet1.xml", func(s string) string { return strings.Replace(s, "<f>A1+1</f>", "<f>A1+1</f><v>5</v>", 1) })
	if e = VerifyPatchedPackage(ctx, cachedBefore, cachedAfter, review); e != nil {
		t.Fatal("legal outside formula cache recalc rejected", e)
	}
	overflow := scopeRewrite(t, original, "xl/worksheets/sheet1.xml", func(s string) string { return strings.Replace(s, `r="D4"`, `r="D100001"`, 1) })
	if e = VerifySelectionPackage(ctx, overflow, selected); e != ErrUnsupportedWorkbook {
		t.Fatal("physical row budget not explicit", e)
	}
}
