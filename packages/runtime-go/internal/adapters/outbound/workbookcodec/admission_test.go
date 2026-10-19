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
		"defined_name":             {"xl/workbook.xml", "</workbook>", `<definedNames><definedName name="UserRange">Sheet1!$A$1</definedName></definedNames></workbook>`},
		"shared_formula":           {"xl/worksheets/sheet1.xml", "<f>", `<f t="shared" si="0">`},
		"rich_text":                {"xl/sharedStrings.xml", "<si><t>", `<si><r><t>`},
		"foreign_element":          {"xl/worksheets/sheet1.xml", "</worksheet>", `<x:feature xmlns:x="urn:unknown"/></worksheet>`},
		"unsupported_relationship": {"xl/_rels/workbook.xml.rels", "</Relationships>", `<Relationship Id="foreign" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/unknownFeature" Target="worksheets/sheet1.xml"/></Relationships>`},
		"unknown_theme_extension":  {"xl/theme/theme1.xml", "</a:theme>", `<a:extLst/></a:theme>`},
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

func TestNativeEditingVerifierRejectsAddedNondefaultPrintSettings(t *testing.T) {
	ctx := context.Background()
	before, after := scopePackage(t, 1, false), scopePackage(t, 4, false)
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil {
		t.Fatal(err)
	}
	withSettings := func(body []byte, setting string) []byte {
		return scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string {
			return strings.Replace(s, "</worksheet>", setting+"</worksheet>", 1)
		})
	}
	for name, setting := range map[string]string{
		"margin":          `<pageMargins left="0.8" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>`,
		"orientation":     `<pageSetup orientation="landscape"/>`,
		"scale":           `<pageSetup scale="50"/>`,
		"unknown_default": `<pageSetup paperSize="9"/>`,
		"headings":        `<printOptions headings="1"/>`,
		"footer":          `<headerFooter><oddFooter>Added footer</oddFooter></headerFooter>`,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := withSettings(after, setting)
			if err := ValidateNativeEditingPackage(ctx, candidate); err != nil {
				t.Fatal("fixture must reach outside-scope preservation check", err)
			}
			if VerifyPatchedPackage(ctx, before, candidate, review) == nil {
				t.Fatal("single-cell edit added a nondefault print setting")
			}
		})
	}
	for name, setting := range map[string]string{
		"margins":       `<pageMargins left="0.7000000000" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>`,
		"page_setup":    `<pageSetup scale="100" firstPageNumber="1" fitToWidth="1" fitToHeight="1" pageOrder="downThenOver" orientation="default" usePrinterDefaults="1" blackAndWhite="0" draft="false" cellComments="none" useFirstPageNumber="0" errors="displayed" horizontalDpi="600" verticalDpi="600" copies="1"/>`,
		"print_options": `<printOptions headings="false" gridLines="0" gridLinesSet="1" horizontalCentered="0" verticalCentered="false"/>`,
		"empty_headers": `<headerFooter differentOddEven="0" differentFirst="false" scaleWithDoc="1" alignWithMargins="true"><oddHeader></oddHeader><oddFooter></oddFooter></headerFooter>`,
	} {
		t.Run("default_"+name, func(t *testing.T) {
			if err := VerifyPatchedPackage(ctx, before, withSettings(after, setting), review); err != nil {
				t.Fatal("equivalent explicit native defaults rejected", err)
			}
			if err := VerifyPatchedPackage(ctx, withSettings(before, setting), after, review); err != nil {
				t.Fatal("omission of equivalent defaults rejected", err)
			}
		})
	}
}

func admissionCalcProps(t *testing.T, body []byte, attributes string) []byte {
	t.Helper()
	return scopeRewrite(t, body, "xl/workbook.xml", func(s string) string {
		start := strings.Index(s, "<calcPr")
		if start < 0 {
			t.Fatal("missing calculation property fixture")
		}
		openingEnd := strings.Index(s[start:], ">")
		if openingEnd < 0 {
			t.Fatal("unclosed calculation property fixture")
		}
		end := start + openingEnd + 1
		if s[end-2] != '/' {
			closing := strings.Index(s[end:], "</calcPr>")
			if closing < 0 {
				t.Fatal("missing calculation property end element")
			}
			end += closing + len("</calcPr>")
		}
		replacement := ""
		if attributes != "" {
			replacement = "<calcPr " + attributes + "/>"
		}
		return s[:start] + replacement + s[end:]
	})
}

func TestNativeEditingVerifierPreservesCalculationSettings(t *testing.T) {
	ctx := context.Background()
	before, after := scopePackage(t, 1, false), scopePackage(t, 4, false)
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil {
		t.Fatal(err)
	}
	for name, attributes := range map[string]string{
		"manual": `calcMode="manual"`, "full_on_load": `fullCalcOnLoad="true"`,
		"force_full": `forceFullCalc="1"`, "on_save": `calcOnSave="false"`,
		"completed": `calcCompleted="false"`, "concurrent": `concurrentCalc="false"`,
		"unknown_concurrency_default": `concurrentManualCount="2"`,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := admissionCalcProps(t, after, attributes)
			if err := ValidateNativeEditingPackage(ctx, candidate); err != nil {
				t.Fatal("calculation fixture must reach preservation check", err)
			}
			if VerifyPatchedPackage(ctx, before, candidate, review) == nil {
				t.Fatal("single-cell edit changed a calculation setting")
			}
		})
	}
	defaults := `calcId="999999" calcMode="auto" refMode="A1" fullCalcOnLoad="false" forceFullCalc="0" calcCompleted="true" calcOnSave="1" concurrentCalc="true" iterate="false" iterateCount="100" iterateDelta="0.001"`
	if err := VerifyPatchedPackage(ctx, before, admissionCalcProps(t, after, defaults), review); err != nil {
		t.Fatal("equivalent explicit calculation defaults or changed engine calcId rejected", err)
	}
	if err := VerifyPatchedPackage(ctx, admissionCalcProps(t, before, defaults), admissionCalcProps(t, after, ""), review); err != nil {
		t.Fatal("omission of equivalent calculation defaults rejected", err)
	}
}

func TestNativeEditingVerifierAllowsUnchangedOutsideBooleanAndErrorCells(t *testing.T) {
	ctx := context.Background()
	withFlags := func(body []byte) []byte {
		return scopeRewrite(t, body, "xl/worksheets/sheet1.xml", func(s string) string {
			return strings.Replace(s, "</sheetData>", `<row r="6"><c r="D6" t="b"><v>1</v></c></row><row r="7"><c r="D7" t="e"><v>#N/A</v></c></row></sheetData>`, 1)
		})
	}
	before, after := withFlags(scopePackage(t, 1, false)), withFlags(scopePackage(t, 4, false))
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySelectionPackage(ctx, before, selected); err != nil {
		t.Fatal("unrelated boolean/error cells made numeric target read-only", err)
	}
	if err := VerifyPatchedPackage(ctx, before, after, review); err != nil {
		t.Fatal("unchanged outside boolean/error values rejected", err)
	}
	for _, change := range [][2]string{{`<v>1</v></c></row><row r="7">`, `<v>0</v></c></row><row r="7">`}, {"#N/A", "#VALUE!"}} {
		bad := scopeRewrite(t, after, "xl/worksheets/sheet1.xml", func(s string) string {
			if !strings.Contains(s, change[0]) {
				t.Fatal("outside typed fixture missing")
			}
			return strings.Replace(s, change[0], change[1], 1)
		})
		if VerifyPatchedPackage(ctx, before, bad, review) == nil {
			t.Fatal("outside boolean/error value changed during numeric edit")
		}
	}
	unsupportedTarget := office.WorkbookSelection{SheetName: "Sheet1", StartColumn: 3, EndColumn: 3, StartRow: 5, EndRow: 5, Cells: []office.WorkbookSelectedCell{{Column: 3, Row: 5, ValueType: "boolean", Text: "TRUE", RowVisible: true, ColumnVisible: true}}}
	if VerifySelectionPackage(ctx, before, unsupportedTarget) == nil {
		t.Fatal("outside-value admission widened typed target types")
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
