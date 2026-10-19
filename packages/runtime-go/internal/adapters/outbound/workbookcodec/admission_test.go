package workbookcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
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
	numericDefaults := strings.NewReplacer(`="false"`, `="0"`, `="true"`, `="1"`).Replace(defaults)
	if err := VerifyPatchedPackage(ctx, before, admissionCalcProps(t, after, numericDefaults), review); err != nil {
		t.Fatal("equivalent numeric boolean calculation defaults rejected", err)
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

func TestNativeEditingVerifierResolvesSourceThemeFillAndBorderColors(t *testing.T) {
	ctx := context.Background()
	withThemeColors := func(value float64) []byte {
		f := excelize.NewFile()
		defer f.Close()
		if err := f.SetCellFloat("Sheet1", "A1", value, -1, 64); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellFloat("Sheet1", "D4", 8, -1, 64); err != nil {
			t.Fatal(err)
		}
		id, err := f.NewStyle(&excelize.Style{
			Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FF0000"}},
			Border: []excelize.Border{{Type: "left", Color: "FF0000", Style: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellStyle("Sheet1", "D4", "D4", id); err != nil {
			t.Fatal(err)
		}
		body, err := f.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		return scopeRewrite(t, body.Bytes(), "xl/styles.xml", func(s string) string {
			if strings.Count(s, `rgb="FFFF0000"`) != 2 {
				t.Fatal("fixture must include both a fill and a border color")
			}
			return strings.ReplaceAll(s, `rgb="FFFF0000"`, `theme="4"`)
		})
	}
	before, after := withThemeColors(1), withThemeColors(4)
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	v := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &v})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPatchedPackage(ctx, before, after, review); err != nil {
		t.Fatal("preserved source theme fill and border colors rejected", err)
	}
	bad := scopeRewrite(t, after, "xl/theme/theme1.xml", func(s string) string {
		decoder := xml.NewDecoder(strings.NewReader(s))
		var out bytes.Buffer
		encoder := xml.NewEncoder(&out)
		var path []xml.Name
		changed := 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			switch element := token.(type) {
			case xml.StartElement:
				// Names are already namespace-resolved by the decoder. Let the
				// encoder emit declarations instead of replaying lexical prefixes.
				attributes := element.Attr[:0]
				for _, attribute := range element.Attr {
					if attribute.Name.Space != "xmlns" && !(attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
						attributes = append(attributes, attribute)
					}
				}
				element.Attr = attributes
				if len(path) > 0 && path[len(path)-1].Local == "accent1" && path[len(path)-1].Space == excelize.NameSpaceDrawingMLMain && element.Name.Local == "srgbClr" && element.Name.Space == excelize.NameSpaceDrawingMLMain {
					for i := range element.Attr {
						if element.Attr[i].Name.Local == "val" && element.Attr[i].Name.Space == "" && element.Attr[i].Value == "5B9BD5" {
							element.Attr[i].Value = "112233"
							changed++
						}
					}
				}
				path = append(path, element.Name)
				token = element
			case xml.EndElement:
				path = path[:len(path)-1]
			}
			if err := encoder.EncodeToken(token); err != nil {
				t.Fatal(err)
			}
		}
		if err := encoder.Flush(); err != nil || changed != 1 {
			t.Fatal("fixture must change exactly one source accent color", changed, err)
		}
		return out.String()
	})
	for name, fixture := range map[string]struct {
		body  []byte
		color string
	}{"before": {before, "5B9BD5"}, "changed_theme": {bad, "112233"}} {
		t.Run(name, func(t *testing.T) {
			f, err := openPatchWorkbook(fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			id, err := f.GetCellStyle("Sheet1", "D4")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := workbookSemanticStyle(f, id)
			if err != nil {
				t.Fatal(err)
			}
			var style excelize.Style
			if err := json.Unmarshal([]byte(raw), &style); err != nil {
				t.Fatal(err)
			}
			if len(style.Fill.Color) != 1 || style.Fill.Color[0] != fixture.color || len(style.Border) != 1 || style.Border[0].Color != fixture.color {
				t.Fatal("effective fill and border colors did not resolve from the source theme", style.Fill, style.Border)
			}
		})
	}
	if VerifyPatchedPackage(ctx, before, bad, review) == nil {
		t.Fatal("default projection theme concealed changed visible fill and border colors")
	}
}

func admissionNativeExportFixtures(t *testing.T) (original, candidate []byte) {
	t.Helper()
	decode := func(encoded, digest string, size int) []byte {
		body, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if len(body) != size || hex.EncodeToString(sum[:]) != digest {
			t.Fatal("native export fixture identity changed")
		}
		return body
	}
	return decode(admissionNativeOriginalBase64, "888bbedffed2e064e935c43a3dd5748ee00d26d41808834338d761e164cdf6f8", 5204),
		decode(admissionNativeCandidateBase64, "8a74c29183dffbb887ee107bb55a5327d2c0e26318b7029662637e7a867da49d", 5973)
}

func admissionNativeManualReview(t *testing.T) office.WorkbookReview {
	t.Helper()
	selected := office.WorkbookSelection{
		SheetName: "Sheet1", StartColumn: 2, EndColumn: 2, StartRow: 1, EndRow: 1,
		Cells: []office.WorkbookSelectedCell{{Column: 2, Row: 1, ValueType: "number", Value: 11, Text: "11", Formula: "11", RowVisible: true, ColumnVisible: true}},
	}
	text := "001"
	review, err := ValidatePatch(context.Background(), selected, office.WorkbookPatch{Kind: "range", Cells: []office.WorkbookCellEdit{{Type: "text", Text: &text}}})
	if err != nil {
		t.Fatal(err)
	}
	return review
}

func admissionNativeReplace(t *testing.T, body []byte, part, before, after string) []byte {
	t.Helper()
	return scopeRewrite(t, body, part, func(s string) string {
		if !strings.Contains(s, before) {
			t.Fatal("native fixture is missing the mutation seam", part)
		}
		return strings.Replace(s, before, after, 1)
	})
}

func TestNativeEditingMeasuredNativeExportAdmittedButRejectsChangedCharset(t *testing.T) {
	ctx := context.Background()
	original, candidate := admissionNativeExportFixtures(t)
	review := admissionNativeManualReview(t)
	for name, body := range map[string][]byte{"original": original, "native_export": candidate} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateNativeEditingPackage(ctx, body); err != nil {
				t.Fatal("measured bounded workbook rejected", err)
			}
		})
	}
	if err := VerifySelectionPackage(ctx, original, review.Before); err != nil {
		t.Fatal("actual original C2 value does not match the typed capture", err)
	}
	if err := VerifyPatchedPackage(ctx, original, candidate, review); !errors.Is(err, patchInvalid) || errors.Is(err, ErrUnsupportedWorkbook) {
		t.Fatal("actual native export must pass admission and still fail effective-font preservation", err)
	}
	if err := VerifySelectionPackage(ctx, candidate, review.After); err != nil {
		t.Fatal("native export typed target does not match the proposed text", err)
	}
	font := func(body []byte) *excelize.Font {
		f, err := openPatchWorkbook(body)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		id, err := f.GetCellStyle("Sheet1", "C2")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := workbookSemanticStyle(f, id)
		if err != nil {
			t.Fatal(err)
		}
		var style excelize.Style
		if err := json.Unmarshal([]byte(raw), &style); err != nil || style.Font == nil {
			t.Fatal("effective inherited font is missing", err)
		}
		return style.Font
	}
	beforeFont, afterFont := font(original), font(candidate)
	if beforeFont.Family != "Noto Sans" || beforeFont.Size != 10 || beforeFont.Charset != nil || afterFont.Family != "Noto Sans" || afterFont.Size != 10 || afterFont.Charset == nil || *afterFont.Charset != 1 {
		t.Fatal("inherited and explicit fonts must retain the actual absent-versus-1 charset difference", beforeFont, afterFont)
	}
}

func admissionNativeExplicitCharsetSource(t *testing.T, original []byte) []byte {
	t.Helper()
	// This is a different codec-only source fixture, not the measured original.
	// It isolates the remaining preservation checks without treating an omitted
	// charset as equivalent to the explicit charset added by the native export.
	return admissionNativeReplace(t, original, "xl/styles.xml", `<name val="Noto Sans"/><family val="2"/>`, `<name val="Noto Sans"/><family val="2"/><charset val="1"/>`)
}

func TestNativeEditingNativeExportWithExplicitCharsetSourcePreservesWorkbook(t *testing.T) {
	ctx := context.Background()
	original, candidate := admissionNativeExportFixtures(t)
	original = admissionNativeExplicitCharsetSource(t, original)
	review := admissionNativeManualReview(t)
	if err := VerifyPatchedPackage(ctx, original, candidate, review); err != nil {
		t.Fatal("codec-only source with matching explicit charset did not preserve semantics and styles", err)
	}
	// Named styles refer to definitions, not fixed positions in cellStyleXfs.
	reordered := scopeRewrite(t, candidate, "xl/styles.xml", func(s string) string {
		return strings.NewReplacer(
			`<xf numFmtId="43" fontId="1"`, `<xf numFmtId="41" fontId="1"`,
			`<xf numFmtId="41" fontId="1"`, `<xf numFmtId="43" fontId="1"`,
			`name="Comma" xfId="15"`, `name="Comma" xfId="16"`,
			`name="Comma [0]" xfId="16"`, `name="Comma [0]" xfId="15"`,
		).Replace(s)
	})
	if err := VerifyPatchedPackage(ctx, original, reordered, review); err != nil {
		t.Fatal("equivalent unused built-in styles were tied to fixed XF indexes", err)
	}
}

func TestNativeEditingMeasuredExportCanBeNextCodecInput(t *testing.T) {
	ctx := context.Background()
	_, original := admissionNativeExportFixtures(t)
	selected := admissionNativeManualReview(t).After
	value := 12.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySelectionPackage(ctx, original, review.Before); err != nil {
		t.Fatal("measured native export cannot authorize a subsequent typed capture", err)
	}
	f, err := openPatchWorkbook(original)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.SetCellFloat("Sheet1", "C2", value, -1, 64); err != nil {
		t.Fatal(err)
	}
	output, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	// This next output is constructed by Excelize for a codec regression. It is
	// not a second native measurement or evidence of a successful Core save.
	if err := VerifyPatchedPackage(ctx, original, output.Bytes(), review); err != nil {
		t.Fatal("native export used as the next codec input did not preserve the workbook", err)
	}
	if err := VerifySelectionPackage(ctx, output.Bytes(), review.After); err != nil {
		t.Fatal("next codec output cannot authorize another typed capture", err)
	}
}

func TestNativeEditingVerifierPreservesDefaultParentProtection(t *testing.T) {
	ctx := context.Background()
	withParentProtection := func(value float64, locked *bool, unlockedOutside bool) []byte {
		f := excelize.NewFile()
		defer f.Close()
		if err := f.SetCellFloat("Sheet1", "A1", value, -1, 64); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellFloat("Sheet1", "D4", 8, -1, 64); err != nil {
			t.Fatal(err)
		}
		id, err := f.NewStyle(&excelize.Style{Protection: &excelize.Protection{Locked: true}})
		if err != nil || f.Styles.CellXfs.Xf[id].Protection == nil {
			t.Fatal("protection fixture definition missing", err)
		}
		protection := *f.Styles.CellXfs.Xf[id].Protection
		protection.Locked, protection.Hidden = locked, nil
		f.Styles.CellStyleXfs.Xf[0].Protection = &protection
		if unlockedOutside {
			id, err = f.NewStyle(&excelize.Style{Protection: &excelize.Protection{Locked: false}})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellStyle("Sheet1", "D4", "D4", id); err != nil {
				t.Fatal(err)
			}
		}
		body, err := f.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		return body.Bytes()
	}
	locked := true
	before, after := withParentProtection(1, nil, false), withParentProtection(4, &locked, false)
	selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
	value := 4.0
	review, err := ValidatePatch(ctx, selected, office.WorkbookPatch{Kind: "number", Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPatchedPackage(ctx, before, after, review); err != nil {
		t.Fatal("omitted parent protection lock differs from its true default", err)
	}
	if err := VerifyPatchedPackage(ctx, withParentProtection(1, &locked, false), withParentProtection(4, nil, false), review); err != nil {
		t.Fatal("omission of an explicit true parent protection lock rejected", err)
	}
	unlocked := withParentProtection(4, &locked, true)
	if err := ValidateNativeEditingPackage(ctx, unlocked); err != nil {
		t.Fatal("unlocked fixture must reach effective protection comparison", err)
	}
	if VerifyPatchedPackage(ctx, before, unlocked, review) == nil {
		t.Fatal("outside cell changed from inherited locked to explicitly unlocked")
	}
}

func TestNativeEditingVerifierPreservesEffectiveRowHeight(t *testing.T) {
	ctx := context.Background()
	original, candidate := admissionNativeExportFixtures(t)
	original = admissionNativeExplicitCharsetSource(t, original)
	review := admissionNativeManualReview(t)
	if err := VerifyPatchedPackage(ctx, original, candidate, review); err != nil {
		t.Fatal("inherited row height and equivalent explicit native row height differ", err)
	}
	for name, change := range map[string][2]string{
		"default": {`defaultRowHeight="12.8"`, `defaultRowHeight="13.8"`},
		"row":     {`ht="12.8"`, `ht="13.8"`},
	} {
		t.Run(name, func(t *testing.T) {
			bad := admissionNativeReplace(t, candidate, "xl/worksheets/sheet1.xml", change[0], change[1])
			if err := ValidateNativeEditingPackage(ctx, bad); err != nil {
				t.Fatal("row-height fixture must reach preservation verification", err)
			}
			if VerifyPatchedPackage(ctx, original, bad, review) == nil {
				t.Fatal("typed edit changed an effective row height")
			}
		})
	}
}

func TestNativeEditingMeasuredNativeExportPassiveFeaturesStayBounded(t *testing.T) {
	ctx := context.Background()
	original, candidate := admissionNativeExportFixtures(t)
	review := admissionNativeManualReview(t)
	for name, change := range map[string]struct{ part, before, after string }{
		"nonempty_custom_properties":        {"docProps/custom.xml", "</Properties>", `<property fmtid="{D5CDD505-2E9C-101B-9397-08002B2CF9AE}" pid="2" name="Added"><vt:lpwstr>value</vt:lpwstr></property></Properties>`},
		"custom_properties_attribute":       {"docProps/custom.xml", "<Properties ", `<Properties active="true" `},
		"active_workbook_protection":        {"xl/workbook.xml", "<workbookProtection/>", `<workbookProtection lockStructure="true"/>`},
		"protection_child":                  {"xl/workbook.xml", "<workbookProtection/>", `<workbookProtection><feature/></workbookProtection>`},
		"unknown_extension_uri":             {"xl/workbook.xml", "{7626C862-2A13-11E5-B345-FEFF819CDC9F}", "{00000000-0000-0000-0000-000000000000}"},
		"unknown_extension_namespace":       {"xl/workbook.xml", `xmlns:loext="http://schemas.libreoffice.org/"`, `xmlns:loext="urn:unknown"`},
		"different_string_reference_syntax": {"xl/workbook.xml", `stringRefSyntax="CalcA1ExcelA1"`, `stringRefSyntax="CalcA1"`},
		"unknown_extension_attribute":       {"xl/workbook.xml", `stringRefSyntax="CalcA1ExcelA1"`, `stringRefSyntax="CalcA1ExcelA1" active="true"`},
		"duplicate_extension":               {"xl/workbook.xml", "</extLst>", `<ext xmlns:loext="http://schemas.libreoffice.org/" uri="{7626C862-2A13-11E5-B345-FEFF819CDC9F}"><loext:extCalcPr stringRefSyntax="CalcA1ExcelA1"/></ext></extLst>`},
		"unknown_extension_child":           {"xl/workbook.xml", `<loext:extCalcPr stringRefSyntax="CalcA1ExcelA1"/>`, `<loext:extCalcPr stringRefSyntax="CalcA1ExcelA1"><loext:feature/></loext:extCalcPr>`},
		"always_calculate_array":            {"xl/worksheets/sheet1.xml", `aca="false"`, `aca="true"`},
		"nonprofile_array_flag":             {"xl/worksheets/sheet1.xml", `aca="false"`, `aca="0"`},
		"unsupported_reference_formula":     {"xl/worksheets/sheet1.xml", "SUM(A1:B1)", `INDIRECT("A1")`},
		"string_reference_formula":          {"xl/worksheets/sheet1.xml", "SUM(A1:B1)", `COUNTIF("A1",">0")`},
		"dynamic_reference_formula":         {"xl/worksheets/sheet1.xml", "SUM(A1:B1)", "OFFSET(A1,0,0)"},
		"named_style_name":                  {"xl/styles.xml", `name="Comma" xfId="15" builtinId="3"`, `name="UserStyle" xfId="15" builtinId="3"`},
		"named_style_builtin":               {"xl/styles.xml", `name="Comma" xfId="15" builtinId="3"`, `name="Comma" xfId="15" builtinId="4"`},
		"named_style_xf":                    {"xl/styles.xml", `name="Comma" xfId="15" builtinId="3"`, `name="Comma" xfId="16" builtinId="3"`},
		"named_style_number_format":         {"xl/styles.xml", `<xf numFmtId="43" fontId="1"`, `<xf numFmtId="44" fontId="1"`},
		"named_style_custom_number_format":  {"xl/styles.xml", `<numFmts count="1">`, `<numFmts count="2"><numFmt numFmtId="43" formatCode="0.00"/>`},
		"named_style_font":                  {"xl/styles.xml", `<name val="Arial"/>`, `<name val="Different Font"/>`},
		"named_style_font_size":             {"xl/styles.xml", `<font><sz val="10"/><name val="Arial"/>`, `<font><sz val="12"/><name val="Arial"/>`},
		"named_style_font_extra_field":      {"xl/styles.xml", `<font><sz val="10"/><name val="Arial"/>`, `<font><b/><sz val="10"/><name val="Arial"/>`},
		"named_style_fill":                  {"xl/styles.xml", `<patternFill patternType="none"/>`, `<patternFill patternType="solid"/>`},
		"named_style_border":                {"xl/styles.xml", `diagonalUp="false"`, `diagonalUp="true"`},
		"named_style_xf_apply":              {"xl/styles.xml", `<xf numFmtId="43" fontId="1" fillId="0" borderId="0" applyFont="true" applyBorder="false"`, `<xf numFmtId="43" fontId="1" fillId="0" borderId="0" applyFont="true" applyBorder="true"`},
		"named_style_applied":               {"xl/styles.xml", `xfId="0" applyFont="true"`, `xfId="15" applyFont="true"`},
		"named_style_count":                 {"xl/styles.xml", `<cellStyles count="6">`, `<cellStyles count="5">`},
		"image_content_type":                {"[Content_Types].xml", `Extension="png" ContentType="image/png"`, `Extension="png" ContentType="image/svg+xml"`},
	} {
		t.Run(name, func(t *testing.T) {
			bad := admissionNativeReplace(t, candidate, change.part, change.before, change.after)
			if err := ValidateNativeEditingPackage(ctx, bad); !errors.Is(err, ErrUnsupportedWorkbook) {
				t.Fatal("native passive feature profile admitted an active or unknown feature", err)
			}
			if VerifyPatchedPackage(ctx, original, bad, review) == nil {
				t.Fatal("full commit verification bypassed the native export profile")
			}
		})
	}
	for _, part := range []string{"xl/media/image1.png", "xl/media/image1.jpeg"} {
		t.Run(part, func(t *testing.T) {
			bad := admissionRepack(t, candidate, func(parts map[string][]byte) { parts[part] = []byte("image bytes") })
			if err := ValidateNativeEditingPackage(ctx, bad); !errors.Is(err, ErrUnsupportedWorkbook) {
				t.Fatal("unused image declarations granted image-part editing authority", err)
			}
		})
	}
}

func TestNativeEditingNativeExportWithExplicitCharsetSourceRejectsChanges(t *testing.T) {
	ctx := context.Background()
	original, candidate := admissionNativeExportFixtures(t)
	original = admissionNativeExplicitCharsetSource(t, original)
	review := admissionNativeManualReview(t)
	for name, change := range map[string]struct{ part, before, after string }{
		"outside_number":       {"xl/worksheets/sheet1.xml", `<c r="B1" s="0" t="n"><v>7</v>`, `<c r="B1" s="0" t="n"><v>8</v>`},
		"outside_formula":      {"xl/worksheets/sheet1.xml", "SUM(A1:B1)", "SUM(A1:B2)"},
		"direct_font_name":     {"xl/styles.xml", `<name val="Noto Sans"/>`, `<name val="Different Font"/>`},
		"direct_font_size":     {"xl/styles.xml", `<sz val="10"/>`, `<sz val="12"/>`},
		"direct_font_charset":  {"xl/styles.xml", `<charset val="1"/>`, `<charset val="2"/>`},
		"removed_font_charset": {"xl/styles.xml", `<charset val="1"/>`, ""},
		"number_format":        {"xl/styles.xml", `numFmtId="164" formatCode="General"`, `numFmtId="164" formatCode="0.00"`},
	} {
		t.Run(name, func(t *testing.T) {
			bad := admissionNativeReplace(t, candidate, change.part, change.before, change.after)
			if err := ValidateNativeEditingPackage(ctx, bad); err != nil {
				t.Fatal("fixture must reach the effective style/content preservation check", err)
			}
			if VerifyPatchedPackage(ctx, original, bad, review) == nil {
				t.Fatal("native text edit changed effective formatting or outside contents")
			}
		})
	}
	// The original cell XF has applyFont=false. Changing only its inherited
	// cellStyleXf font must remain visible to the semantic comparator.
	changedOriginal := admissionNativeReplace(t, original, "xl/styles.xml", `<xf numFmtId="164" fontId="0" fillId="0" borderId="0" applyFont="true"`, `<xf numFmtId="164" fontId="1" fillId="0" borderId="0" applyFont="true"`)
	if err := ValidateNativeEditingPackage(ctx, changedOriginal); err != nil {
		t.Fatal("inherited font fixture must reach semantic comparison", err)
	}
	if VerifyPatchedPackage(ctx, changedOriginal, candidate, review) == nil {
		t.Fatal("inherited cellStyleXf font change was ignored")
	}
	for name, change := range map[string][2]string{
		"font":       {`fontId="0" fillId="0" borderId="0" xfId="0" applyFont="false"`, `fontId="1" fillId="0" borderId="0" xfId="0" applyFont="false"`},
		"alignment":  {`xfId="0" applyFont="false" applyBorder="false" applyAlignment="false" applyProtection="false"><alignment horizontal="general"`, `xfId="0" applyFont="false" applyBorder="false" applyAlignment="false" applyProtection="false"><alignment horizontal="right"`},
		"protection": {`<protection locked="true" hidden="false"/></xf><xf numFmtId="165"`, `<protection locked="false" hidden="false"/></xf><xf numFmtId="165"`},
	} {
		t.Run("conflicting_inheritance_"+name, func(t *testing.T) {
			conflicting := admissionNativeReplace(t, original, "xl/styles.xml", change[0], change[1])
			if VerifyPatchedPackage(ctx, conflicting, candidate, review) == nil {
				t.Fatal("false apply flag silently ignored conflicting local and parent style fields")
			}
		})
	}
}

// These unmodified ZIP packages are the synthetic 3x3 manual-edit fixture and
// its sole measured native number-11-to-text-001 export (2026-10-19), produced
// from source 2a6cf7ca55385bc20430faeec59deb4a756a28e7 and fixed native engine
// build efaf0670b4d055f838a2849becb10f08aa06a257. Their hashes are checked above.
// They are codec regression evidence, not a successful Core save receipt: the
// measured export adds an explicit font charset and must fail preservation.
const admissionNativeOriginalBase64 = "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" +
	"ZXNdLnhtbFBLBQYAAAAACgAKAIACAAC+EQAAAAA="

const admissionNativeCandidateBase64 = "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"
