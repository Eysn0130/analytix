package workbookcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

func admissionPackage(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for address, value := range map[string]interface{}{"A1": 1, "A2": 2, "B1": "ordinary text"} {
		if err := f.SetCellValue("Sheet1", address, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Sheet1", "C1", "SUM(A1:A2)"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetPanes("Sheet1", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "B", "B", 24); err != nil {
		t.Fatal(err)
	}
	body, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func admissionRepack(t *testing.T, body []byte, change func(map[string][]byte)) []byte {
	t.Helper()
	parts := workbookZipEntries(t, body)
	change(parts)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, name := range names {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(parts[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestNativeEditingAdmissionSimpleWorkbookAndConventionalExportMetadata(t *testing.T) {
	ctx := context.Background()
	body := admissionPackage(t)
	if err := ValidateNativeEditingPackage(ctx, body); err != nil {
		t.Fatal("ordinary cells, text, formula, dimensions and frozen pane rejected", err)
	}
	conventional := scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string {
		return strings.Replace(s, "</worksheet>", `<printOptions headings="false" gridLines="false" gridLinesSet="true" horizontalCentered="false" verticalCentered="false"/><pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/><pageSetup paperSize="9" scale="100" firstPageNumber="1" fitToWidth="1" fitToHeight="1" pageOrder="downThenOver" orientation="portrait" blackAndWhite="false" draft="false" cellComments="none" useFirstPageNumber="false" horizontalDpi="300" verticalDpi="300" copies="1"/><headerFooter differentOddEven="false" differentFirst="false" scaleWithDoc="true" alignWithMargins="true"/><rowBreaks count="0" manualBreakCount="0"/><colBreaks count="0" manualBreakCount="0"/></worksheet>`, 1)
	})
	if err := ValidateNativeEditingPackage(ctx, conventional); err != nil {
		t.Fatal("conventional native export metadata prevents re-editing", err)
	}
	if NativeEditingReadOnlyReason(nil) != "" || NativeEditingReadOnlyReason(patchInvalid) != "" {
		t.Fatal("non-eligibility error exposed as feature reason")
	}
}

func TestNativeEditingAdmissionRejectsComplexTypedCaptureWithoutRestrictingReads(t *testing.T) {
	ctx := context.Background()
	body := admissionPackage(t)
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	if err := VerifySelectionPackage(ctx, body, selected); err != nil {
		t.Fatal("simple typed capture rejected", err)
	}
	for _, part := range []string{"xl/charts/chart1.xml", "xl/drawings/drawing1.xml", "xl/pivotTables/pivotTable1.xml", "xl/comments1.xml", "xl/threadedComments/threadedComment1.xml", "xl/vbaProject.bin", "customXml/item1.xml", "xl/unknown.xml", "xl/calcChain.xml"} {
		t.Run(part, func(t *testing.T) {
			complex := admissionRepack(t, body, func(parts map[string][]byte) {
				parts[part] = []byte("<unverified/>")
				if part == "xl/calcChain.xml" {
					parts[part] = []byte(`<calcChain xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"/>`)
				}
			})
			err := ValidateNativeEditingPackage(ctx, complex)
			if !errors.Is(err, ErrUnsupportedWorkbook) || NativeEditingReadOnlyReason(err) != "unsupported_package_part" || strings.Contains(err.Error(), part) {
				t.Fatal("missing safe read-only reason", err)
			}
			if err = VerifySelectionPackage(ctx, complex, selected); !errors.Is(err, ErrUnsupportedWorkbook) {
				t.Fatal("complex source granted typed capture authority", err)
			}
			reader, err := excelize.OpenReader(bytes.NewReader(complex))
			if err != nil {
				t.Fatal("complex source cannot be opened for reading", err)
			}
			defer reader.Close()
			value, err := reader.GetCellValue("Sheet1", "A1")
			if err != nil || value != "1" {
				t.Fatal("read-only source value unavailable", value, err)
			}
		})
	}
}

func TestNativeEditingAdmissionRejectsUnverifiedXMLAndRelationships(t *testing.T) {
	ctx := context.Background()
	body := admissionPackage(t)
	for name, feature := range map[string]string{
		"conditional_format": `<conditionalFormatting sqref="A1"><cfRule type="expression" priority="1"><formula>A1&gt;0</formula></cfRule></conditionalFormatting>`,
		"data_validation":    `<dataValidations count="1"><dataValidation type="whole" sqref="A1"><formula1>1</formula1></dataValidation></dataValidations>`,
		"protection":         `<sheetProtection sheet="true"/>`,
		"hyperlink":          `<hyperlinks><hyperlink ref="A1" location="B1"/></hyperlinks>`,
		"extension":          `<extLst><ext uri="unknown"/></extLst>`,
		"page_break":         `<rowBreaks count="1" manualBreakCount="1"><brk id="2" min="0" max="16383" man="true"/></rowBreaks>`,
	} {
		t.Run(name, func(t *testing.T) {
			bad := scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string { return strings.Replace(s, "</worksheet>", feature+"</worksheet>", 1) })
			if err := ValidateNativeEditingPackage(ctx, bad); !errors.Is(err, ErrUnsupportedWorkbook) || NativeEditingReadOnlyReason(err) == "" {
				t.Fatal("unverified worksheet feature admitted", err)
			}
		})
	}
	for name, change := range map[string]struct {
		part, old, new string
	}{
		"defined_name":            {"xl/workbook.xml", "</workbook>", `<definedNames><definedName name="UserRange">Sheet1!$A$1</definedName></definedNames></workbook>`},
		"shared_formula":          {"xl/worksheets/sheet1.xml", "<f>", `<f t="shared" si="0">`},
		"rich_text":               {"xl/sharedStrings.xml", "<si><t>", `<si><r><t>`},
		"foreign_element":         {"xl/worksheets/sheet1.xml", "</worksheet>", `<x:feature xmlns:x="urn:unknown"/></worksheet>`},
		"external_relationship":   {"xl/_rels/workbook.xml.rels", "</Relationships>", `<Relationship Id="foreign" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="https://example.invalid/sheet.xml" TargetMode="External"/></Relationships>`},
		"unknown_theme_extension": {"xl/theme/theme1.xml", "</a:theme>", `<a:extLst/></a:theme>`},
	} {
		t.Run(name, func(t *testing.T) {
			bad := scopeRewrite(t, body, change.part, func(s string) string {
				if !strings.Contains(s, change.old) {
					t.Fatal("fixture did not contain expected feature seam")
				}
				changed := strings.Replace(s, change.old, change.new, 1)
				if name == "rich_text" {
					changed = strings.Replace(changed, "</t></si>", "</t></r></si>", 1)
				}
				return changed
			})
			if err := ValidateNativeEditingPackage(ctx, bad); !errors.Is(err, ErrUnsupportedWorkbook) {
				t.Fatal("unverified feature admitted", err)
			}
		})
	}
}

func TestNativeEditingAdmissionEnforcesPhysicalBudgetAndExportGate(t *testing.T) {
	ctx := context.Background()
	body := admissionPackage(t)
	overflow := scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string {
		var rows strings.Builder
		for i := 0; i <= workbookScopeCells; i++ {
			rows.WriteString(`<row r="` + office.WorkbookCellAddress(0, i)[1:] + `"><c r="` + office.WorkbookCellAddress(0, i) + `"><v>1</v></c></row>`)
		}
		start, end := strings.Index(s, "<sheetData>"), strings.Index(s, "</sheetData>")
		if start < 0 || end < start {
			t.Fatal("missing sheet data fixture")
		}
		return s[:start] + "<sheetData>" + rows.String() + s[end:]
	})
	if err := ValidateNativeEditingPackage(ctx, overflow); !errors.Is(err, ErrUnsupportedWorkbook) || NativeEditingReadOnlyReason(err) != "bounded_scope_exceeded" {
		t.Fatal("physical cell budget not enforced", err)
	}
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil {
		t.Fatal(err)
	}
	candidate := scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string { return strings.Replace(s, "<v>1</v>", "<v>4</v>", 1) })
	for _, originalComplex := range []bool{false, true} {
		original, exported := body, candidate
		if originalComplex {
			original = admissionRepack(t, original, func(parts map[string][]byte) { parts["xl/comments1.xml"] = []byte("<comments/>") })
		} else {
			exported = admissionRepack(t, exported, func(parts map[string][]byte) { parts["xl/comments1.xml"] = []byte("<comments/>") })
		}
		if err = VerifyPatchedPackage(ctx, original, exported, review); !errors.Is(err, ErrUnsupportedWorkbook) {
			t.Fatal("commit bypassed source/candidate admission", err)
		}
	}
}

func TestNativeEditingVerifierPreservesExplicitPrintSettings(t *testing.T) {
	ctx := context.Background()
	before, after := scopePackage(t, 1, false), scopePackage(t, 4, false)
	withMargins := func(body []byte, left string) []byte {
		return scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string {
			return strings.Replace(s, "</worksheet>", `<pageMargins left="`+left+`" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/></worksheet>`, 1)
		})
	}
	before, after = withMargins(before, "0.5"), withMargins(after, "0.5")
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil || VerifyPatchedPackage(ctx, before, after, review) != nil {
		t.Fatal("preserved explicit margins rejected", err)
	}
	bad := scopeRewrite(t, after, "xl/worksheets/sheet1.xml", func(s string) string { return strings.Replace(s, `left="0.5"`, `left="0.8"`, 1) })
	if VerifyPatchedPackage(ctx, before, bad, review) == nil {
		t.Fatal("export changed user print margins")
	}
}

func TestNativeEditingVerifierResolvesThemeFontColor(t *testing.T) {
	ctx := context.Background()
	before, after := scopePackage(t, 1, false), scopePackage(t, 4, false)
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil {
		t.Fatal(err)
	}
	bad := scopeRewrite(t, after, "xl/theme/theme1.xml", func(s string) string {
		if !strings.Contains(s, `lastClr="000000"`) {
			t.Fatal("missing default font theme color")
		}
		return strings.Replace(s, `lastClr="000000"`, `lastClr="112233"`, 1)
	})
	if VerifyPatchedPackage(ctx, before, bad, review) == nil {
		t.Fatal("same theme index concealed a changed visible font color")
	}
	bad = scopeRewrite(t, after, "xl/theme/theme1.xml", func(s string) string {
		if !strings.Contains(s, `typeface="Calibri"`) {
			t.Fatal("missing theme font family")
		}
		return strings.Replace(s, `typeface="Calibri"`, `typeface="Different Font"`, 1)
	})
	if VerifyPatchedPackage(ctx, before, bad, review) == nil {
		t.Fatal("export changed theme font families")
	}
}
