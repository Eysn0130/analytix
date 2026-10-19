package workbookcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

// These limits apply to physical cells/rows, not the worksheet's empty grid.
// Validation is read-only; files outside this finite profile are unsupported.
var ErrUnsupportedWorkbook = errors.New("workbook exceeds bounded typed editing scope")

const workbookScopeCells = 10000

type workbookAdmissionError string

func (e workbookAdmissionError) Error() string { return "workbook is read-only: " + string(e) }
func (e workbookAdmissionError) Unwrap() error { return ErrUnsupportedWorkbook }

// NativeEditingReadOnlyReason returns a bounded, content-free reason suitable for
// the editing eligibility projection. It never returns an input part name.
func NativeEditingReadOnlyReason(err error) string {
	var reason workbookAdmissionError
	if errors.As(err, &reason) {
		return string(reason)
	}
	if errors.Is(err, ErrUnsupportedWorkbook) {
		return "bounded_scope_exceeded"
	}
	return ""
}

// ValidateNativeEditingPackage admits the bounded XLSX editing feature profile.
// The filestore entry points must first run their existing InspectOfficePackage
// structural preflight on these OPC bytes; this gate does not repeat ZIP/XML,
// content-type or relationship safety validation. Unsupported features remain
// readable, and workbookPhysical independently enforces the editing cell budget.
func ValidateNativeEditingPackage(ctx context.Context, body []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = patchInvalid
		}
	}()
	if ctx == nil || ctx.Err() != nil || len(body) == 0 {
		return patchInvalid
	}
	if len(body) > MaxOutputBytes {
		return workbookAdmissionError("bounded_scope_exceeded")
	}
	entries, err := workbookPackageEntries(body)
	if err != nil {
		return err
	}
	for name, entry := range entries {
		kind := workbookAdmissionPart(name)
		if kind == "" {
			return workbookAdmissionError("unsupported_package_part")
		}
		raw, e := workbookXML(ctx, entry)
		if e != nil {
			return e
		}
		switch kind {
		case "relationships":
			if e = workbookAdmissionRelationships(raw, name, entries); e != nil {
				return e
			}
		case "content-types":
			if e = workbookAdmissionContentTypes(raw, entries); e != nil {
				return e
			}
		case "workbook", "worksheet", "styles", "strings":
			if e = workbookAdmissionXML(ctx, raw, kind); e != nil {
				return e
			}
		case "metadata", "theme":
			if e = workbookAdmissionAncillaryXML(ctx, raw, name); e != nil {
				return e
			}
		case "empty-custom-properties":
			if e = workbookAdmissionEmptyCustomProperties(raw); e != nil {
				return e
			}
		}
	}
	sheetParts := 0
	themes := 0
	for name := range entries {
		kind := workbookAdmissionPart(name)
		if kind == "worksheet" {
			sheetParts++
		}
		if kind == "theme" {
			themes++
		}
	}
	if themes > 1 {
		return workbookAdmissionError("unsupported_package_part")
	}
	sheets, err := workbookPhysical(ctx, body)
	if err == nil && len(sheets) != sheetParts {
		return workbookAdmissionError("unsupported_package_part")
	}
	if err == nil {
		_, err = workbookThemeFonts(ctx, body)
	}
	if err == nil {
		err = workbookAdmissionPassivePackage(ctx, body, entries, sheets)
	}
	return err
}

func workbookAdmissionNumberedPart(name, prefix, suffix string) bool {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	number := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if number == "" {
		return false
	}
	for _, c := range number {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func workbookAdmissionPart(name string) string {
	switch name {
	case "[Content_Types].xml":
		return "content-types"
	case "_rels/.rels", "xl/_rels/workbook.xml.rels":
		return "relationships"
	case "docProps/app.xml", "docProps/core.xml":
		return "metadata"
	case "docProps/custom.xml":
		return "empty-custom-properties"
	case "xl/workbook.xml":
		return "workbook"
	case "xl/styles.xml":
		return "styles"
	case "xl/sharedStrings.xml":
		return "strings"
	}
	if workbookAdmissionNumberedPart(name, "xl/worksheets/sheet", ".xml") {
		return "worksheet"
	}
	if workbookAdmissionNumberedPart(name, "xl/theme/theme", ".xml") {
		return "theme"
	}
	return ""
}

func workbookPackageEntries(body []byte) (map[string]*zip.File, error) {
	z, e := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if e != nil {
		return nil, patchInvalid
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if f.UncompressedSize64 > 64<<20 || total > (64<<20)-f.UncompressedSize64 || len(entries) >= 4096 {
			return nil, ErrUnsupportedWorkbook
		}
		total += f.UncompressedSize64
		if entries[f.Name] != nil || f.Name != path.Clean(f.Name) || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "\\") {
			return nil, patchInvalid
		}
		entries[f.Name] = f
	}
	return entries, nil
}

// OPC target resolution, IDs and external-link safety are owned by the existing
// filestore preflight. Only relationship feature kinds are restricted here.
func workbookAdmissionRelationships(raw []byte, part string, entries map[string]*zip.File) error {
	var rels struct {
		Items []struct {
			XMLName    xml.Name
			ID         string     `xml:"Id,attr"`
			Type       string     `xml:"Type,attr"`
			Target     string     `xml:"Target,attr"`
			TargetMode string     `xml:"TargetMode,attr"`
			Other      []xml.Attr `xml:",any,attr"`
			Inner      string     `xml:",innerxml"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(raw, &rels) != nil {
		return patchInvalid
	}
	custom := 0
	for _, r := range rels.Items {
		switch r.Type {
		case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet",
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles",
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme",
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings",
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument",
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties",
			"http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties":
		case "http://schemas.openxmlformats.org/officeDocument/2006/relationships/custom-properties":
			custom++
			if part != "_rels/.rels" || r.XMLName.Space != "http://schemas.openxmlformats.org/package/2006/relationships" || r.ID == "" || r.Target != "docProps/custom.xml" || r.TargetMode != "" || len(r.Other) != 0 || strings.TrimSpace(r.Inner) != "" || entries["docProps/custom.xml"] == nil || custom != 1 {
				return workbookAdmissionError("unsupported_relationship")
			}
		default:
			return workbookAdmissionError("unsupported_relationship")
		}
	}
	if part == "_rels/.rels" && entries["docProps/custom.xml"] != nil && custom != 1 {
		return workbookAdmissionError("unsupported_relationship")
	}
	return nil
}

func workbookAdmissionXML(ctx context.Context, raw []byte, kind string) error {
	// This is the finite editable feature allowlist, not an OOXML schema. The
	// filestore preflight owns root counts, duplicate attributes and XML safety.
	profiles := map[string]map[string]string{
		"workbook": {
			"workbook": "", "workbook/fileVersion": "appName lastEdited lowestEdited rupBuild codeName",
			"workbook/workbookPr": "date1904 filterPrivacy defaultThemeVersion backupFile showObjects updateLinks saveExternalLinkValues",
			"workbook/bookViews":  "", "workbook/bookViews/workbookView": "xWindow yWindow windowWidth windowHeight activeTab firstSheet tabRatio visibility minimized showHorizontalScroll showVerticalScroll showSheetTabs autoFilterDateGrouping",
			"workbook/sheets": "", "workbook/sheets/sheet": "name sheetId state",
			"workbook/definedNames":       "",
			"workbook/workbookProtection": "",
			"workbook/extLst":             "", "workbook/extLst/ext": "uri",
			"workbook/extLst/ext/extCalcPr": "stringRefSyntax",
			"workbook/calcPr":               "calcId calcMode fullCalcOnLoad forceFullCalc calcCompleted calcOnSave concurrentCalc concurrentManualCount iterate iterateCount iterateDelta refMode",
		},
		"worksheet": {
			"worksheet": "", "worksheet/sheetPr": "codeName filterMode published enableFormatConditionsCalculation",
			"worksheet/sheetPr/pageSetUpPr": "autoPageBreaks fitToPage", "worksheet/sheetPr/outlinePr": "applyStyles summaryBelow summaryRight showOutlineSymbols",
			"worksheet/dimension": "ref", "worksheet/sheetViews": "",
			"worksheet/sheetViews/sheetView":           "workbookViewId tabSelected topLeftCell view zoomScale zoomScaleNormal zoomScalePageLayoutView zoomScaleSheetLayoutView showGridLines showRowColHeaders showZeros showFormulas rightToLeft showRuler showWhiteSpace defaultGridColor colorId windowProtection showOutlineSymbols",
			"worksheet/sheetViews/sheetView/pane":      "xSplit ySplit topLeftCell activePane state",
			"worksheet/sheetViews/sheetView/selection": "pane activeCell activeCellId sqref",
			"worksheet/sheetFormatPr":                  "baseColWidth defaultColWidth defaultRowHeight customHeight zeroHeight outlineLevelRow outlineLevelCol",
			"worksheet/cols":                           "", "worksheet/cols/col": "min max width style hidden bestFit customWidth outlineLevel collapsed phonetic",
			"worksheet/sheetData": "", "worksheet/sheetData/row": "r s customFormat ht hidden customHeight outlineLevel collapsed spans thickTop thickBot",
			"worksheet/sheetData/row/c": "r s t", "worksheet/sheetData/row/c/v": "", "worksheet/sheetData/row/c/f": "t aca",
			"worksheet/sheetData/row/c/is": "", "worksheet/sheetData/row/c/is/t": "",
			"worksheet/mergeCells": "count", "worksheet/mergeCells/mergeCell": "ref",
			"worksheet/printOptions":           "headings gridLines gridLinesSet horizontalCentered verticalCentered",
			"worksheet/pageMargins":            "left right top bottom header footer",
			"worksheet/pageSetup":              "paperSize scale firstPageNumber fitToWidth fitToHeight pageOrder orientation usePrinterDefaults blackAndWhite draft cellComments useFirstPageNumber errors horizontalDpi verticalDpi copies",
			"worksheet/headerFooter":           "differentOddEven differentFirst scaleWithDoc alignWithMargins",
			"worksheet/headerFooter/oddHeader": "", "worksheet/headerFooter/oddFooter": "", "worksheet/headerFooter/evenHeader": "", "worksheet/headerFooter/evenFooter": "", "worksheet/headerFooter/firstHeader": "", "worksheet/headerFooter/firstFooter": "",
			"worksheet/rowBreaks": "count manualBreakCount", "worksheet/colBreaks": "count manualBreakCount",
		},
		"strings": {"sst": "count uniqueCount", "sst/si": "", "sst/si/t": ""},
		"styles": {
			"styleSheet": "", "styleSheet/numFmts": "count", "styleSheet/numFmts/numFmt": "numFmtId formatCode",
			"styleSheet/fonts": "count", "styleSheet/fonts/font": "",
			"styleSheet/fills": "count", "styleSheet/fills/fill": "", "styleSheet/fills/fill/patternFill": "patternType",
			"styleSheet/fills/fill/patternFill/fgColor": "rgb indexed auto theme tint", "styleSheet/fills/fill/patternFill/bgColor": "rgb indexed auto theme tint",
			"styleSheet/borders": "count", "styleSheet/borders/border": "diagonalUp diagonalDown outline",
			"styleSheet/cellStyleXfs": "count", "styleSheet/cellXfs": "count",
			"styleSheet/cellStyles": "count", "styleSheet/cellStyles/cellStyle": "name xfId builtinId customBuiltin",
			"styleSheet/dxfs": "count", "styleSheet/tableStyles": "count defaultTableStyle defaultPivotStyle",
		},
	}
	profile := profiles[kind]
	if kind == "styles" {
		for _, name := range []string{"b", "i", "u", "strike", "vertAlign", "sz", "name", "family", "charset", "scheme"} {
			profile["styleSheet/fonts/font/"+name] = "val"
		}
		profile["styleSheet/fonts/font/color"] = "rgb indexed auto theme tint"
		for _, name := range []string{"left", "right", "top", "bottom", "diagonal", "vertical", "horizontal"} {
			profile["styleSheet/borders/border/"+name] = "style"
			profile["styleSheet/borders/border/"+name+"/color"] = "rgb indexed auto theme tint"
		}
		for _, collection := range []string{"cellStyleXfs", "cellXfs"} {
			prefix := "styleSheet/" + collection + "/xf"
			profile[prefix] = "numFmtId fontId fillId borderId xfId applyNumberFormat applyFont applyFill applyBorder applyAlignment applyProtection quotePrefix pivotButton"
			profile[prefix+"/alignment"] = "horizontal vertical textRotation wrapText shrinkToFit indent relativeIndent justifyLastLine readingOrder"
			profile[prefix+"/protection"] = "locked hidden"
		}
	}
	reason := workbookAdmissionError("unsupported_xml_feature")
	if kind == "worksheet" {
		reason = workbookAdmissionError("unsupported_worksheet_feature")
	} else if kind == "workbook" {
		reason = workbookAdmissionError("unsupported_workbook_feature")
	}
	d := xml.NewDecoder(bytes.NewReader(raw))
	stack := []string{}
	for {
		if ctx == nil || ctx.Err() != nil {
			return patchInvalid
		}
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return patchInvalid
		}
		switch n := t.(type) {
		case xml.StartElement:
			stack = append(stack, n.Name.Local)
			p := strings.Join(stack, "/")
			if n.Name.Space != "http://schemas.openxmlformats.org/spreadsheetml/2006/main" && !(kind == "workbook" && p == "workbook/extLst/ext/extCalcPr" && n.Name.Space == "http://schemas.libreoffice.org/") {
				return reason
			}
			allowed, ok := profile[p]
			if !ok {
				if strings.Contains(p, "/c/") || kind == "strings" {
					return workbookAdmissionError("unsupported_cell_feature")
				}
				return reason
			}
			for _, a := range n.Attr {
				if a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns" {
					continue
				}
				if len(stack) == 1 && (a.Name.Space == "http://schemas.openxmlformats.org/markup-compatibility/2006" && a.Name.Local == "Ignorable" || a.Name.Space == "http://schemas.microsoft.com/office/spreadsheetml/2014/revision" && a.Name.Local == "uid") {
					continue // Namespace/identity metadata does not admit extension payloads.
				}
				if p == "styleSheet/fonts" && a.Name.Space == "http://schemas.microsoft.com/office/spreadsheetml/2009/9/ac" && a.Name.Local == "knownFonts" {
					continue
				}
				if (p == "worksheet/sheetFormatPr" || p == "worksheet/sheetData/row") && a.Name.Space == "http://schemas.microsoft.com/office/spreadsheetml/2009/9/ac" && a.Name.Local == "dyDescent" {
					continue // Font descent bookkeeping; row height is verified separately.
				}
				if n.Name.Local == "t" && a.Name.Space == "http://www.w3.org/XML/1998/namespace" && a.Name.Local == "space" && (a.Value == "preserve" || a.Value == "default") {
					continue
				}
				if p == "workbook/sheets/sheet" && a.Name.Space == "http://schemas.openxmlformats.org/officeDocument/2006/relationships" && a.Name.Local == "id" {
					continue
				}
				if a.Name.Space != "" || !strings.Contains(" "+allowed+" ", " "+a.Name.Local+" ") {
					return reason
				}
				if workbookAdmissionAttributeUnsupported(p, a.Name.Local, a.Value) {
					return reason
				}
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	return nil
}

func workbookAdmissionAttributeUnsupported(p, name, value string) bool {
	truthy := value != "0" && value != "false"
	switch {
	case p == "workbook/workbookPr" && name == "date1904":
		return truthy
	case p == "workbook/workbookPr" && (name == "backupFile" || name == "saveExternalLinkValues"):
		return truthy
	case p == "workbook/workbookPr" && name == "showObjects":
		return value != "all"
	case p == "workbook/workbookPr" && name == "updateLinks":
		return value != "userSet"
	case p == "workbook/calcPr" && name == "iterate":
		return truthy
	case p == "workbook/calcPr" && name == "refMode":
		return value != "A1"
	case p == "worksheet/sheetPr" && name == "filterMode":
		return truthy
	case p == "worksheet/sheetPr" && (name == "published" || name == "enableFormatConditionsCalculation"):
		return !truthy
	case p == "worksheet/sheetViews/sheetView":
		switch name {
		case "rightToLeft", "windowProtection", "showFormulas":
			return truthy
		case "showGridLines", "showRowColHeaders", "showZeros", "showRuler", "showWhiteSpace", "defaultGridColor", "showOutlineSymbols":
			return !truthy
		case "view":
			return value != "normal"
		case "colorId":
			return value != "64"
		}
	case p == "worksheet/cols/col" && (name == "collapsed" || name == "phonetic" || name == "bestFit"), p == "worksheet/sheetData/row" && (name == "collapsed" || name == "thickTop" || name == "thickBot"):
		return truthy
	case p == "worksheet/sheetFormatPr" && (name == "outlineLevelRow" || name == "outlineLevelCol"), p == "worksheet/rowBreaks", p == "worksheet/colBreaks":
		return value != "0"
	case p == "worksheet/sheetData/row/c" && name == "t":
		return value != "n" && value != "s" && value != "inlineStr" && value != "str" && value != "b" && value != "e"
	case p == "worksheet/sheetData/row/c/f" && name == "t":
		return value != "normal"
	case p == "worksheet/sheetData/row/c/f" && name == "aca":
		return value != "false"
	case p == "styleSheet/cellStyles" && name == "count":
		return value != "1" && value != "6"
	case p == "styleSheet/cellStyles/cellStyle":
		// Identity, definitions and references are checked together against the
		// finite passive catalog, using Excelize's already parsed Styles.
		return name == "customBuiltin" && truthy
	case (p == "styleSheet/dxfs" || p == "styleSheet/tableStyles") && name == "count":
		return value != "0"
	case strings.HasSuffix(p, "/xf") && (name == "quotePrefix" || name == "pivotButton"):
		return truthy
	}
	return false
}

func workbookAdmissionAncillaryXML(ctx context.Context, raw []byte, part string) error {
	root, namespaces, elements := "", "", ""
	switch part {
	case "docProps/core.xml":
		root = "coreProperties"
		namespaces = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties http://purl.org/dc/elements/1.1/ http://purl.org/dc/terms/"
		elements = "coreProperties category contentStatus created creator description identifier keywords language lastModifiedBy lastPrinted modified revision subject title version"
	case "docProps/app.xml":
		root = "Properties"
		namespaces = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"
		elements = "Properties Template Manager Company Pages Words Characters PresentationFormat Lines Paragraphs Slides Notes TotalTime HiddenSlides MMClips ScaleCrop HeadingPairs TitlesOfParts LinksUpToDate CharactersWithSpaces SharedDoc HyperlinkBase HLinks HyperlinksChanged Application AppVersion DocSecurity vector variant lpstr lpwstr i4 ui4 bool"
	default:
		root = "theme"
		namespaces = "http://schemas.openxmlformats.org/drawingml/2006/main"
		// Only passive theme definitions, never image fills or extension payloads.
		elements = "theme themeElements clrScheme dk1 lt1 dk2 lt2 accent1 accent2 accent3 accent4 accent5 accent6 hlink folHlink sysClr srgbClr schemeClr prstClr scrgbClr hslClr fontScheme majorFont minorFont latin ea cs font fmtScheme fillStyleLst bgFillStyleLst solidFill noFill gradFill gsLst gs lin path fillToRect tileRect lnStyleLst ln prstDash custDash ds round bevel miter headEnd tailEnd effectStyleLst effectStyle effectLst effectDag outerShdw innerShdw glow softEdge reflection blur scene3d camera lightRig rot sp3d bevelT bevelB contourClr extrusionClr objectDefaults extraClrSchemeLst tint shade satMod satOff lumMod lumOff alpha alphaMod alphaOff hue hueMod hueOff comp inv gray gamma invGamma red redMod redOff green greenMod greenOff blue blueMod blueOff"
	}
	d := xml.NewDecoder(bytes.NewReader(raw))
	seen := false
	for {
		if ctx == nil || ctx.Err() != nil {
			return patchInvalid
		}
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return patchInvalid
		}
		switch n := t.(type) {
		case xml.StartElement:
			if !seen && n.Name.Local != root || !strings.Contains(" "+namespaces+" ", " "+n.Name.Space+" ") || !strings.Contains(" "+elements+" ", " "+n.Name.Local+" ") {
				return workbookAdmissionError("unsupported_xml_feature")
			}
			seen = true
		}
	}
	return nil
}

// The preflight already validates declarations against archive entries. These
// are only the passive content types supported by this editing feature profile.
func workbookAdmissionContentTypes(raw []byte, entries map[string]*zip.File) error {
	var types struct {
		Items []struct {
			XMLName   xml.Name
			Type      string     `xml:"ContentType,attr"`
			Extension string     `xml:"Extension,attr"`
			PartName  string     `xml:"PartName,attr"`
			Other     []xml.Attr `xml:",any,attr"`
			Inner     string     `xml:",innerxml"`
		} `xml:",any"`
	}
	if xml.Unmarshal(raw, &types) != nil {
		return patchInvalid
	}
	custom := 0
	for _, item := range types.Items {
		switch item.Type {
		case "application/xml", "application/vnd.openxmlformats-package.relationships+xml",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml",
			"application/vnd.openxmlformats-officedocument.theme+xml",
			"application/vnd.openxmlformats-package.core-properties+xml",
			"application/vnd.openxmlformats-officedocument.extended-properties+xml":
		case "application/vnd.openxmlformats-officedocument.custom-properties+xml":
			custom++
			if item.XMLName.Local != "Override" || item.XMLName.Space != "http://schemas.openxmlformats.org/package/2006/content-types" || item.PartName != "/docProps/custom.xml" || item.Extension != "" || len(item.Other) != 0 || strings.TrimSpace(item.Inner) != "" || entries["docProps/custom.xml"] == nil || custom != 1 {
				return workbookAdmissionError("unsupported_package_part")
			}
		case "image/png", "image/jpeg":
			extension := "png"
			if item.Type == "image/jpeg" {
				extension = "jpeg"
			}
			if item.XMLName.Local != "Default" || item.XMLName.Space != "http://schemas.openxmlformats.org/package/2006/content-types" || item.Extension != extension || item.PartName != "" || len(item.Other) != 0 || strings.TrimSpace(item.Inner) != "" {
				return workbookAdmissionError("unsupported_package_part")
			}
			for name := range entries {
				if strings.HasPrefix(name, "xl/media/") {
					return workbookAdmissionError("unsupported_package_part")
				}
			}
		default:
			return workbookAdmissionError("unsupported_package_part")
		}
	}
	if entries["docProps/custom.xml"] != nil && custom != 1 {
		return workbookAdmissionError("unsupported_package_part")
	}
	return nil
}

const workbookPassiveCalcURI = "{7626C862-2A13-11E5-B345-FEFF819CDC9F}"
const workbookPassiveCalcNamespace = "http://schemas.libreoffice.org/"

func workbookAdmissionExactAttributes(attrs []xml.Attr, expected map[string]string) bool {
	count := 0
	for _, a := range attrs {
		if a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns" {
			continue
		}
		value, ok := expected[a.Name.Local]
		if a.Name.Space != "" || !ok || a.Value != value {
			return false
		}
		count++
	}
	return count == len(expected)
}

func workbookAdmissionEmptyCustomProperties(raw []byte) error {
	d := xml.NewDecoder(bytes.NewReader(raw))
	seen, depth := false, 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			if seen && depth == 0 {
				return nil
			}
			return patchInvalid
		}
		if err != nil {
			return patchInvalid
		}
		switch n := token.(type) {
		case xml.StartElement:
			if seen || n.Name.Local != "Properties" || n.Name.Space != "http://schemas.openxmlformats.org/officeDocument/2006/custom-properties" || !workbookAdmissionExactAttributes(n.Attr, nil) {
				return workbookAdmissionError("unsupported_package_part")
			}
			seen, depth = true, 1
		case xml.EndElement:
			depth--
		case xml.CharData:
			if strings.TrimSpace(string(n)) != "" {
				return workbookAdmissionError("unsupported_package_part")
			}
		case xml.Comment, xml.Directive:
			return workbookAdmissionError("unsupported_package_part")
		case xml.ProcInst:
			if seen || n.Target != "xml" {
				return workbookAdmissionError("unsupported_package_part")
			}
		}
	}
}

// Only the observed empty protection marker and one passive Calc syntax marker
// are admitted. The syntax marker requires formulas without string literals
// that also pass Core's existing closed formula grammar.
func workbookAdmissionPassiveWorkbook(raw []byte) (bool, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	stack := []string{}
	protection, lists, extensions, calculation := 0, 0, 0, 0
	reject := workbookAdmissionError("unsupported_workbook_feature")
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, patchInvalid
		}
		switch n := token.(type) {
		case xml.StartElement:
			stack = append(stack, n.Name.Local)
			p := strings.Join(stack, "/")
			switch p {
			case "workbook/workbookProtection":
				protection++
				if protection != 1 || !workbookAdmissionExactAttributes(n.Attr, nil) {
					return false, reject
				}
			case "workbook/extLst":
				lists++
				if lists != 1 || !workbookAdmissionExactAttributes(n.Attr, nil) {
					return false, reject
				}
			case "workbook/extLst/ext":
				extensions++
				if extensions != 1 || !workbookAdmissionExactAttributes(n.Attr, map[string]string{"uri": workbookPassiveCalcURI}) {
					return false, reject
				}
			case "workbook/extLst/ext/extCalcPr":
				calculation++
				if calculation != 1 || n.Name.Space != workbookPassiveCalcNamespace || !workbookAdmissionExactAttributes(n.Attr, map[string]string{"stringRefSyntax": "CalcA1ExcelA1"}) {
					return false, reject
				}
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			p := strings.Join(stack, "/")
			if (strings.HasPrefix(p, "workbook/extLst") || strings.HasPrefix(p, "workbook/workbookProtection")) && strings.TrimSpace(string(n)) != "" {
				return false, reject
			}
		case xml.Comment, xml.Directive, xml.ProcInst:
			p := strings.Join(stack, "/")
			if strings.HasPrefix(p, "workbook/extLst") || strings.HasPrefix(p, "workbook/workbookProtection") {
				return false, reject
			}
		}
	}
	if lists != 0 && (extensions != 1 || calculation != 1) {
		return false, reject
	}
	return lists == 1, nil
}

func workbookAdmissionPassivePackage(ctx context.Context, body []byte, entries map[string]*zip.File, sheets []workbookPhysicalSheet) error {
	raw, err := workbookXML(ctx, entries["xl/workbook.xml"])
	if err != nil {
		return err
	}
	hasCalcMarker, err := workbookAdmissionPassiveWorkbook(raw)
	if err != nil {
		return err
	}
	f, err := openPatchWorkbook(body)
	if err != nil {
		return patchInvalid
	}
	defer f.Close()
	if err = workbookAdmissionNamedStyles(f); err != nil {
		return err
	}
	if !hasCalcMarker {
		return nil
	}
	w := office.Workbook{}
	for i, physical := range sheets {
		sheet := office.Sheet{ID: "sheet" + strconv.Itoa(i), Name: physical.Name}
		for address := range physical.Cells {
			if ctx.Err() != nil {
				return patchInvalid
			}
			formula, e := f.GetCellFormula(physical.Name, address)
			if e != nil {
				return patchInvalid
			}
			if strings.Contains(formula, `"`) {
				// The extension changes string-reference syntax. Keep its passive
				// profile to formulas without string literals; Core still owns
				// the closed grammar and supported functions below.
				return workbookAdmissionError("unsupported_workbook_feature")
			}
			if formula != "" {
				sheet.Cells = append(sheet.Cells, office.Cell{Address: address, Type: "formula", Formula: formula})
			}
		}
		w.Sheets = append(w.Sheets, sheet)
	}
	if w.Validate() != nil {
		return workbookAdmissionError("unsupported_workbook_feature")
	}
	return nil
}

// Native export adds exactly this unused built-in catalog. Resolve every
// referenced definition instead of trusting a name or export-specific XF index.
func workbookAdmissionNamedStyles(f *excelize.File) error {
	reject := workbookAdmissionError("unsupported_xml_feature")
	if _, err := f.GetStyle(0); err != nil {
		return patchInvalid
	}
	s := f.Styles
	if s.CellStyles == nil {
		return nil
	}
	if s.CellStyles.Count != len(s.CellStyles.CellStyle) || s.CellStyles.Count != 1 && s.CellStyles.Count != 6 || s.CellStyleXfs == nil || s.CellStyleXfs.Count != len(s.CellStyleXfs.Xf) {
		return reject
	}
	catalog := map[string]struct{ builtin, format int }{
		"Comma": {3, 43}, "Comma [0]": {6, 41}, "Currency": {4, 44}, "Currency [0]": {7, 42}, "Percent": {5, 9},
	}
	seen := map[string]bool{}
	passiveXfs := map[int]bool{}
	normal := false
	for _, named := range s.CellStyles.CellStyle {
		if named == nil || seen[named.Name] || named.XfID < 0 || named.XfID >= len(s.CellStyleXfs.Xf) || named.ILevel != nil || named.Hidden != nil || named.CustomBuiltIn != nil && *named.CustomBuiltIn {
			return reject
		}
		seen[named.Name] = true
		if named.Name == "Normal" || named.Name == "Default" {
			if normal || s.CellStyles.Count == 6 && named.Name != "Normal" || named.XfID != 0 || named.BuiltInID != nil && *named.BuiltInID != 0 {
				return reject
			}
			normal = true
			continue
		}
		expected, ok := catalog[named.Name]
		if !ok || s.CellStyles.Count != 6 || named.BuiltInID == nil || *named.BuiltInID != expected.builtin || passiveXfs[named.XfID] || named.XfID == 0 {
			return reject
		}
		xf := s.CellStyleXfs.Xf[named.XfID]
		if xf.NumFmtID == nil || *xf.NumFmtID != expected.format || xf.FontID == nil || xf.FillID == nil || xf.BorderID == nil || xf.XfID != nil || xf.QuotePrefix != nil || xf.PivotButton != nil || xf.ApplyNumberFormat != nil || xf.ApplyFill != nil || xf.ApplyFont == nil || !*xf.ApplyFont || xf.ApplyBorder == nil || *xf.ApplyBorder || xf.ApplyAlignment == nil || *xf.ApplyAlignment || xf.ApplyProtection == nil || *xf.ApplyProtection || xf.Alignment != nil || xf.Protection != nil {
			return reject
		}
		if s.Fonts == nil || s.Fills == nil || s.Borders == nil || s.Fonts.Count != len(s.Fonts.Font) || s.Fills.Count != len(s.Fills.Fill) || s.Borders.Count != len(s.Borders.Border) || *xf.FontID < 0 || *xf.FontID >= len(s.Fonts.Font) || *xf.FillID < 0 || *xf.FillID >= len(s.Fills.Fill) || *xf.BorderID < 0 || *xf.BorderID >= len(s.Borders.Border) {
			return reject
		}
		font, fill, border := s.Fonts.Font[*xf.FontID], s.Fills.Fill[*xf.FillID], s.Borders.Border[*xf.BorderID]
		if font == nil || font.Name == nil || font.Name.Val == nil || *font.Name.Val != "Arial" || font.Sz == nil || font.Sz.Val == nil || *font.Sz.Val != 10 || font.Family == nil || font.Family.Val == nil || *font.Family.Val != 0 || font.Charset != nil || font.B != nil || font.I != nil || font.Strike != nil || font.Outline != nil || font.Shadow != nil || font.Condense != nil || font.Extend != nil || font.Color != nil || font.U != nil || font.VertAlign != nil || font.Scheme != nil {
			return reject
		}
		if fill == nil || fill.GradientFill != nil || fill.PatternFill == nil || fill.PatternFill.PatternType != "none" || fill.PatternFill.FgColor != nil || fill.PatternFill.BgColor != nil || border == nil || border.DiagonalDown || border.DiagonalUp || border.Outline || border.Vertical != nil || border.Horizontal != nil {
			return reject
		}
		if border.Left == nil || border.Right == nil || border.Top == nil || border.Bottom == nil || border.Diagonal == nil {
			return reject
		}
		if border.Left.Style != "" || border.Left.Color != nil || border.Right.Style != "" || border.Right.Color != nil || border.Top.Style != "" || border.Top.Color != nil || border.Bottom.Style != "" || border.Bottom.Color != nil || border.Diagonal.Style != "" || border.Diagonal.Color != nil {
			return reject
		}
		if s.NumFmts != nil {
			if s.NumFmts.Count != len(s.NumFmts.NumFmt) {
				return reject
			}
			for _, format := range s.NumFmts.NumFmt {
				if format == nil || format.NumFmtID == expected.format {
					return reject
				}
			}
		}
		passiveXfs[named.XfID] = true
	}
	if !normal || s.CellStyles.Count == 6 && len(passiveXfs) != len(catalog) {
		return reject
	}
	if s.CellXfs == nil || s.CellXfs.Count != len(s.CellXfs.Xf) {
		return reject
	}
	for _, xf := range s.CellXfs.Xf {
		if xf.XfID != nil && passiveXfs[*xf.XfID] {
			return reject
		}
	}
	for _, xf := range s.CellStyleXfs.Xf {
		if xf.XfID != nil && passiveXfs[*xf.XfID] {
			return reject
		}
	}
	return nil
}

type workbookPhysicalSheet struct {
	Name, ID, State, CodeName, Filter string
	ZeroHeight                        bool
	DefaultRowHeight                  float64
	Cells                             map[string]bool
	Rows                              map[int]int
	RowHeights                        map[int]float64
	Columns                           map[int]bool
	Merges                            map[string]bool
	Settings                          map[string]string
}

func workbookXML(ctx context.Context, z *zip.File) ([]byte, error) {
	if z == nil || z.UncompressedSize64 > 64<<20 {
		return nil, ErrUnsupportedWorkbook
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, patchInvalid
	}
	r, e := z.Open()
	if e != nil {
		return nil, patchInvalid
	}
	defer r.Close()
	body, e := io.ReadAll(io.LimitReader(r, (64<<20)+1))
	if e != nil || len(body) > 64<<20 {
		return nil, ErrUnsupportedWorkbook
	}
	return body, nil
}
func workbookPhysical(ctx context.Context, body []byte) ([]workbookPhysicalSheet, error) {
	entries, e := workbookPackageEntries(body)
	if e != nil {
		return nil, e
	}
	var book struct {
		Sheets []struct {
			Name  string `xml:"name,attr"`
			ID    string `xml:"sheetId,attr"`
			Rel   string `xml:"id,attr"`
			State string `xml:"state,attr"`
		} `xml:"sheets>sheet"`
	}
	raw, e := workbookXML(ctx, entries["xl/workbook.xml"])
	if e != nil {
		return nil, e
	}
	if xml.Unmarshal(raw, &book) != nil || len(book.Sheets) == 0 || len(book.Sheets) > 20 {
		return nil, ErrUnsupportedWorkbook
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
			Type   string `xml:"Type,attr"`
		} `xml:"Relationship"`
	}
	raw, e = workbookXML(ctx, entries["xl/_rels/workbook.xml.rels"])
	if e != nil {
		return nil, e
	}
	if xml.Unmarshal(raw, &rels) != nil {
		return nil, patchInvalid
	}
	targets := map[string]string{}
	for _, r := range rels.Items {
		if targets[r.ID] != "" {
			return nil, patchInvalid
		}
		if r.Mode == "" && strings.HasSuffix(r.Type, "/worksheet") {
			target := path.Clean(path.Join("xl", r.Target))
			if strings.HasPrefix(r.Target, "/") {
				target = strings.TrimPrefix(path.Clean(r.Target), "/")
			}
			if !strings.HasPrefix(target, "xl/") {
				return nil, patchInvalid
			}
			targets[r.ID] = target
		}
	}
	result := make([]workbookPhysicalSheet, 0, len(book.Sheets))
	cells, rows, budget := 0, 0, 0
	for _, sheet := range book.Sheets {
		raw, e = workbookXML(ctx, entries[targets[sheet.Rel]])
		if e != nil {
			return nil, e
		}
		s := workbookPhysicalSheet{Name: sheet.Name, ID: sheet.ID, State: sheet.State, DefaultRowHeight: 15, Cells: map[string]bool{}, Rows: map[int]int{}, RowHeights: map[int]float64{}, Columns: map[int]bool{1: true, 16384: true}, Merges: map[string]bool{}, Settings: map[string]string{}}
		if s.State == "" {
			s.State = "visible"
		}
		d := xml.NewDecoder(bytes.NewReader(raw))
		maxCols := map[int]int{}
		for {
			if ctx.Err() != nil {
				return nil, patchInvalid
			}
			token, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, patchInvalid
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			attrs := map[string]string{}
			for _, a := range start.Attr {
				attrs[a.Name.Local] = a.Value
			}
			switch start.Name.Local {
			case "pageSetUpPr", "outlinePr", "printOptions", "pageMargins", "pageSetup", "headerFooter":
				for name, value := range attrs {
					s.Settings[start.Name.Local+"/"+name] = value
				}
			case "oddHeader", "oddFooter", "evenHeader", "evenFooter", "firstHeader", "firstFooter":
				var text string
				if d.DecodeElement(&text, &start) != nil {
					return nil, patchInvalid
				}
				s.Settings[start.Name.Local+"/text"] = text
			case "sheetFormatPr":
				s.ZeroHeight = attrs["zeroHeight"] == "1" || attrs["zeroHeight"] == "true"
				if value := attrs["defaultRowHeight"]; value != "" {
					height, err := strconv.ParseFloat(value, 64)
					if err != nil || math.IsNaN(height) || math.IsInf(height, 0) || height < 0 {
						return nil, patchInvalid
					}
					s.DefaultRowHeight = height
				}
			case "sheetPr":
				s.CodeName = attrs["codeName"]
			case "autoFilter":
				s.Filter = attrs["ref"]
			case "mergeCell":
				s.Merges[attrs["ref"]] = true
				if len(s.Merges) > 10000 {
					return nil, ErrUnsupportedWorkbook
				}
			case "row":
				r, err := strconv.Atoi(attrs["r"])
				if err != nil || r < 1 || r > 100000 {
					return nil, ErrUnsupportedWorkbook
				}
				if _, exists := s.Rows[r]; exists {
					return nil, patchInvalid
				}
				style := 0
				if attrs["s"] != "" {
					style, err = strconv.Atoi(attrs["s"])
					if err != nil || style < 0 {
						return nil, patchInvalid
					}
				}
				s.Rows[r] = style
				if value := attrs["ht"]; value != "" {
					height, err := strconv.ParseFloat(value, 64)
					if err != nil || math.IsNaN(height) || math.IsInf(height, 0) || height < 0 {
						return nil, patchInvalid
					}
					s.RowHeights[r] = height
				}
				rows++
				if rows > 10000 {
					return nil, ErrUnsupportedWorkbook
				}
			case "col":
				a, e1 := strconv.Atoi(attrs["min"])
				b, e2 := strconv.Atoi(attrs["max"])
				if e1 != nil || e2 != nil || a < 1 || b < a || b > 16384 {
					return nil, patchInvalid
				}
				s.Columns[a] = true
				if b < 16384 {
					s.Columns[b+1] = true
				}
				if len(s.Columns) > 4096 {
					return nil, ErrUnsupportedWorkbook
				}
			case "c":
				ref := attrs["r"]
				c, r, err := excelize.CellNameToCoordinates(ref)
				if err != nil || r > 100000 || c > 16384 {
					return nil, ErrUnsupportedWorkbook
				}
				if s.Cells[ref] {
					return nil, patchInvalid
				}
				s.Cells[ref] = true
				cells++
				if cells > workbookScopeCells {
					return nil, ErrUnsupportedWorkbook
				}
				if c > maxCols[r] {
					maxCols[r] = c
				}
			}
		}
		for _, col := range maxCols {
			budget += col
			if budget > 1000000 {
				return nil, ErrUnsupportedWorkbook
			}
		}
		result = append(result, s)
	}
	return result, nil
}

// workbookStyleResolver uses Excelize's parsed styles and public style decoder.
// GetStyle alone omits components whose cell-XF apply flag is false; that does
// not mean the effective font/border/etc. is absent. Resolve the named-style
// parent first, without changing either the source styles or any package bytes.
type workbookStyleResolver struct {
	source     *excelize.File
	projection *excelize.File
	cache      map[int]string
}

func newWorkbookStyleResolver(f *excelize.File) (*workbookStyleResolver, error) {
	if _, err := f.GetStyle(0); err != nil || f.Styles == nil || f.Styles.CellXfs == nil || f.Styles.CellStyleXfs == nil || f.Styles.Fonts == nil || f.Styles.Fills == nil || f.Styles.Borders == nil {
		return nil, patchInvalid
	}
	projection := excelize.NewFile()
	// Do not copy File/style-sheet mutexes or alter the source's shared records.
	// Only the cell-XF slice is private; referenced definitions remain read-only.
	cellXfs := *f.Styles.CellXfs
	cellXfs.Xf = append(cellXfs.Xf[:0:0], cellXfs.Xf...)
	projection.Styles.CellXfs = &cellXfs
	projection.Styles.CellStyleXfs = f.Styles.CellStyleXfs
	projection.Styles.NumFmts = f.Styles.NumFmts
	projection.Styles.Fonts = f.Styles.Fonts
	projection.Styles.Fills = f.Styles.Fills
	projection.Styles.Borders = f.Styles.Borders
	projection.Styles.Colors = f.Styles.Colors
	projection.Theme = f.Theme
	return &workbookStyleResolver{source: f, projection: projection, cache: map[int]string{}}, nil
}

func workbookInheritedStyleID(local, parent *int, apply, parentApply *bool, count int, same func(int, int) bool) (*int, error) {
	index := func(value *int) int {
		if value == nil {
			return 0
		}
		return *value
	}
	a, b := index(local), index(parent)
	if a < 0 || b < 0 || count >= 0 && (a >= count || b >= count) {
		return nil, patchInvalid
	}
	if apply != nil && *apply {
		return &a, nil
	}
	// Cell-XF apply flags default to false. A local component that disagrees
	// with its parent must explicitly opt out of inheritance; do not guess which
	// contradictory definition a reader would display. Style-XF flags default
	// to true; an explicitly disabled parent component is outside this resolver.
	if parentApply != nil && !*parentApply || local != nil && !same(a, b) {
		return nil, patchInvalid
	}
	return &b, nil
}

func (r *workbookStyleResolver) semantic(id int) (string, error) {
	if value, ok := r.cache[id]; ok {
		return value, nil
	}
	styles := r.source.Styles
	if id < 0 || id >= len(styles.CellXfs.Xf) {
		return "", patchInvalid
	}
	xf := styles.CellXfs.Xf[id]
	parentID := 0
	if xf.XfID != nil {
		parentID = *xf.XfID
	}
	if parentID < 0 || parentID >= len(styles.CellStyleXfs.Xf) {
		return "", patchInvalid
	}
	base := styles.CellStyleXfs.Xf[parentID]
	if base.XfID != nil {
		return "", patchInvalid // Named-style XFs do not form an inheritance graph.
	}
	var err error
	if xf.FontID, err = workbookInheritedStyleID(xf.FontID, base.FontID, xf.ApplyFont, base.ApplyFont, len(styles.Fonts.Font), func(a, b int) bool { return reflect.DeepEqual(styles.Fonts.Font[a], styles.Fonts.Font[b]) }); err != nil {
		return "", err
	}
	if xf.FillID, err = workbookInheritedStyleID(xf.FillID, base.FillID, xf.ApplyFill, base.ApplyFill, len(styles.Fills.Fill), func(a, b int) bool { return reflect.DeepEqual(styles.Fills.Fill[a], styles.Fills.Fill[b]) }); err != nil {
		return "", err
	}
	if xf.BorderID, err = workbookInheritedStyleID(xf.BorderID, base.BorderID, xf.ApplyBorder, base.ApplyBorder, len(styles.Borders.Border), func(a, b int) bool { return reflect.DeepEqual(styles.Borders.Border[a], styles.Borders.Border[b]) }); err != nil {
		return "", err
	}
	if xf.NumFmtID, err = workbookInheritedStyleID(xf.NumFmtID, base.NumFmtID, xf.ApplyNumberFormat, base.ApplyNumberFormat, -1, func(a, b int) bool { return a == b }); err != nil {
		return "", err
	}
	if xf.ApplyAlignment == nil || !*xf.ApplyAlignment {
		// Excelize emits an empty alignment element for an ordinary NewStyle.
		// Its zero-valued properties equal the absent default, not an override.
		localDefault := xf.Alignment == nil || reflect.ValueOf(xf.Alignment).Elem().IsZero()
		parentDefault := base.Alignment == nil || reflect.ValueOf(base.Alignment).Elem().IsZero()
		if base.ApplyAlignment != nil && !*base.ApplyAlignment || xf.Alignment != nil && !reflect.DeepEqual(xf.Alignment, base.Alignment) && !(localDefault && parentDefault) {
			return "", patchInvalid
		}
		xf.Alignment = base.Alignment
	}
	if xf.ApplyProtection == nil || !*xf.ApplyProtection {
		defaultValues := func(hidden, locked *bool) bool { return (hidden == nil || !*hidden) && (locked == nil || *locked) }
		localDefault := xf.Protection == nil || defaultValues(xf.Protection.Hidden, xf.Protection.Locked)
		parentDefault := base.Protection == nil || defaultValues(base.Protection.Hidden, base.Protection.Locked)
		if base.ApplyProtection != nil && !*base.ApplyProtection || xf.Protection != nil && !reflect.DeepEqual(xf.Protection, base.Protection) && !(localDefault && parentDefault) {
			return "", patchInvalid
		}
		xf.Protection = base.Protection
	}
	if xf.Protection != nil && xf.Protection.Locked == nil {
		// Excel treats omitted locked as true, including an empty protection
		// element. Excelize's decoder otherwise turns that element into false.
		protection := *xf.Protection
		locked := true
		protection.Locked = &locked
		xf.Protection = &protection
	}
	// These flags belong only to the in-memory decoder projection. They expose
	// every resolved component to GetStyle; the real cell-XF and output remain
	// intact. Charset presence/value and every other public Font field are kept.
	apply := true
	xf.ApplyFont, xf.ApplyFill, xf.ApplyBorder = &apply, &apply, &apply
	xf.ApplyAlignment, xf.ApplyProtection, xf.ApplyNumberFormat = &apply, &apply, &apply
	r.projection.Styles.CellXfs.Xf[id] = xf
	s, e := r.projection.GetStyle(id)
	if e != nil {
		return "", patchInvalid
	}
	// Resolve the public semantic style, not the export-specific XF index.
	if s.Alignment == nil {
		s.Alignment = &excelize.Alignment{}
	}
	if s.Protection == nil {
		s.Protection = &excelize.Protection{Locked: true}
	}
	if s.Font != nil {
		color := r.source.GetBaseColor(s.Font.Color, s.Font.ColorIndexed, s.Font.ColorTheme)
		if color != "" {
			color = excelize.ThemeColor(color, s.Font.ColorTint)
			if len(color) == 8 {
				color = color[2:]
			}
		}
		s.Font.Color = strings.ToUpper(color)
		s.Font.ColorIndexed, s.Font.ColorTheme, s.Font.ColorTint = 0, nil, 0
	}
	if s.CustomNumFmt != nil {
		s.NumFmt = 0
	}
	if len(s.Fill.Color) == 0 {
		s.Fill.Color = nil
	}
	borders := s.Border[:0]
	for _, b := range s.Border {
		if b.Style != 0 {
			borders = append(borders, b)
		}
	}
	s.Border = borders
	sort.Slice(s.Border, func(i, j int) bool { return s.Border[i].Type < s.Border[j].Type })
	raw, e := json.Marshal(s)
	if e == nil {
		r.cache[id] = string(raw)
	}
	return string(raw), e
}

func workbookSemanticStyle(f *excelize.File, id int) (string, error) {
	resolver, err := newWorkbookStyleResolver(f)
	if err != nil {
		return "", err
	}
	defer resolver.projection.Close()
	return resolver.semantic(id)
}
func workbookCellSemantic(f *excelize.File, sheet, address string) (string, error) {
	formula, e := f.GetCellFormula(sheet, address)
	if e != nil {
		return "", patchInvalid
	}
	if formula != "" {
		return "formula:" + office.CanonicalWorkbookFormula(formula), nil
	}
	typ, e := f.GetCellType(sheet, address)
	if e != nil {
		return "", patchInvalid
	}
	v, e := f.GetCellValue(sheet, address, excelize.Options{RawCellValue: true})
	if e != nil {
		return "", patchInvalid
	}
	switch typ {
	case excelize.CellTypeUnset, excelize.CellTypeNumber:
		if v == "" {
			return "empty", nil
		}
		n, e := strconv.ParseFloat(v, 64)
		if e != nil {
			return "", patchInvalid
		}
		return "number:" + strconv.FormatFloat(n, 'g', -1, 64), nil
	case excelize.CellTypeSharedString, excelize.CellTypeInlineString:
		if v == "" {
			return "empty", nil
		}
		return "text:" + v, nil
	default:
		return strconv.Itoa(int(typ)) + ":" + v, nil
	}
}

func workbookSameExplicitSettings(before, after map[string]string) bool {
	defaults := map[string]string{
		"pageSetUpPr/autoPageBreaks": "true", "pageSetUpPr/fitToPage": "false",
		"outlinePr/applyStyles": "false", "outlinePr/summaryBelow": "true", "outlinePr/summaryRight": "true", "outlinePr/showOutlineSymbols": "true",
		"printOptions/headings": "false", "printOptions/gridLines": "false", "printOptions/gridLinesSet": "true", "printOptions/horizontalCentered": "false", "printOptions/verticalCentered": "false",
		"pageMargins/left": "0.7", "pageMargins/right": "0.7", "pageMargins/top": "0.75", "pageMargins/bottom": "0.75", "pageMargins/header": "0.3", "pageMargins/footer": "0.3",
		"pageSetup/scale": "100", "pageSetup/firstPageNumber": "1", "pageSetup/fitToWidth": "1", "pageSetup/fitToHeight": "1", "pageSetup/pageOrder": "downThenOver", "pageSetup/orientation": "default", "pageSetup/usePrinterDefaults": "true", "pageSetup/blackAndWhite": "false", "pageSetup/draft": "false", "pageSetup/cellComments": "none", "pageSetup/useFirstPageNumber": "false", "pageSetup/errors": "displayed", "pageSetup/horizontalDpi": "600", "pageSetup/verticalDpi": "600", "pageSetup/copies": "1",
		"headerFooter/differentOddEven": "false", "headerFooter/differentFirst": "false", "headerFooter/scaleWithDoc": "true", "headerFooter/alignWithMargins": "true",
		"oddHeader/text": "", "oddFooter/text": "", "evenHeader/text": "", "evenFooter/text": "", "firstHeader/text": "", "firstFooter/text": "",
	}
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	for key := range keys {
		left, leftExists := before[key]
		right, rightExists := after[key]
		if !leftExists || !rightExists {
			value, known := defaults[key]
			if !known {
				return false
			}
			if !leftExists {
				left = value
			}
			if !rightExists {
				right = value
			}
		}
		if left == right {
			continue
		}
		if strings.HasSuffix(key, "/text") {
			return false
		}
		if defaults[key] == "true" || defaults[key] == "false" {
			if left == "0" && right == "false" || left == "false" && right == "0" || left == "1" && right == "true" || left == "true" && right == "1" {
				continue
			}
			return false
		}
		a, e1 := strconv.ParseFloat(left, 64)
		b, e2 := strconv.ParseFloat(right, 64)
		if e1 != nil || e2 != nil || math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) || math.Abs(a-b) > 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b))) {
			return false
		}
	}
	return true
}

func workbookThemeFonts(ctx context.Context, body []byte) (map[string]string, error) {
	entries, err := workbookPackageEntries(body)
	if err != nil {
		return nil, err
	}
	fonts := map[string]string{}
	for name, entry := range entries {
		if workbookAdmissionPart(name) != "theme" {
			continue
		}
		raw, err := workbookXML(ctx, entry)
		if err != nil {
			return nil, err
		}
		d := xml.NewDecoder(bytes.NewReader(raw))
		family := ""
		for {
			if ctx == nil || ctx.Err() != nil {
				return nil, patchInvalid
			}
			t, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, patchInvalid
			}
			switch n := t.(type) {
			case xml.StartElement:
				switch n.Name.Local {
				case "majorFont", "minorFont":
					family = n.Name.Local
				case "latin", "ea", "cs", "font":
					if family == "" {
						continue
					}
					attrs := map[string]string{}
					for _, a := range n.Attr {
						attrs[a.Name.Local] = a.Value
					}
					key := family + "/" + n.Name.Local
					if n.Name.Local == "font" {
						key += "/" + attrs["script"]
					}
					if _, exists := fonts[key]; exists {
						return nil, patchInvalid
					}
					fonts[key] = attrs["typeface"]
				}
			case xml.EndElement:
				if n.Name.Local == "majorFont" || n.Name.Local == "minorFont" {
					family = ""
				}
			}
		}
	}
	return fonts, nil
}

func workbookCalculationSettings(ctx context.Context, f *excelize.File, body []byte) (map[string]string, error) {
	props, err := f.GetCalcProps()
	if err != nil {
		return nil, patchInvalid
	}
	entries, err := workbookPackageEntries(body)
	if err != nil {
		return nil, err
	}
	raw, err := workbookXML(ctx, entries["xl/workbook.xml"])
	if err != nil {
		return nil, err
	}
	var book struct {
		CalcPr struct {
			Attributes []xml.Attr `xml:",any,attr"`
		} `xml:"calcPr"`
	}
	if xml.Unmarshal(raw, &book) != nil {
		return nil, patchInvalid
	}
	present := map[string]bool{}
	for _, a := range book.CalcPr.Attributes {
		present[a.Name.Local] = true
	}
	values := map[string]string{}
	putBool := func(name string, value *bool) {
		if value != nil {
			values[name] = strconv.FormatBool(*value)
		} else if present[name] {
			// Excelize v2.11.0's getter omits scalar zero values. Original
			// XML presence distinguishes explicit false from an absent default.
			values[name] = "false"
		}
	}
	putUint := func(name string, value *uint) {
		if value != nil {
			values[name] = strconv.FormatUint(uint64(*value), 10)
		}
	}
	putBool("fullCalcOnLoad", props.FullCalcOnLoad)
	putBool("forceFullCalc", props.ForceFullCalc)
	putBool("calcCompleted", props.CalcCompleted)
	putBool("calcOnSave", props.CalcOnSave)
	putBool("concurrentCalc", props.ConcurrentCalc)
	putBool("iterate", props.Iterate)
	putUint("concurrentManualCount", props.ConcurrentManualCount)
	putUint("iterateCount", props.IterateCount)
	if props.CalcMode != nil {
		values["calcMode"] = *props.CalcMode
	} else if present["calcMode"] {
		values["calcMode"] = ""
	}
	if props.RefMode != nil {
		values["refMode"] = *props.RefMode
	} else if present["refMode"] {
		values["refMode"] = ""
	}
	if props.IterateDelta != nil {
		values["iterateDelta"] = strconv.FormatFloat(*props.IterateDelta, 'g', -1, 64)
	} else if present["iterateDelta"] {
		values["iterateDelta"] = "0"
	}
	// GetCalcProps exposes parsed typed values but not attribute presence. Fill
	// absent OOXML properties from their known defaults, including true booleans.
	for name, value := range map[string]string{
		"calcMode": "auto", "refMode": "A1", "fullCalcOnLoad": "false", "forceFullCalc": "false",
		"calcCompleted": "true", "calcOnSave": "true", "concurrentCalc": "true", "iterate": "false", "iterateCount": "100", "iterateDelta": "0.001",
	} {
		if !present[name] {
			values[name] = value
		}
	}
	if !present["concurrentManualCount"] {
		delete(values, "concurrentManualCount") // No fixed default: do not invent one.
	}
	// calcId identifies the writing engine's calculation version; it is not a
	// user calculation setting and may change during the verified native export.
	return values, nil
}

func verifyWorkbookOutsideScope(ctx context.Context, before, after *excelize.File, original, candidate []byte, scope office.WorkbookSelection) error {
	beforeStyles, err := newWorkbookStyleResolver(before)
	if err != nil {
		return err
	}
	defer beforeStyles.projection.Close()
	afterStyles, err := newWorkbookStyleResolver(after)
	if err != nil {
		return err
	}
	defer afterStyles.projection.Close()
	calcBefore, e1 := workbookCalculationSettings(ctx, before, original)
	calcAfter, e2 := workbookCalculationSettings(ctx, after, candidate)
	if e1 != nil || e2 != nil || !reflect.DeepEqual(calcBefore, calcAfter) {
		return patchInvalid
	}
	fontsBefore, e1 := workbookThemeFonts(ctx, original)
	fontsAfter, e2 := workbookThemeFonts(ctx, candidate)
	if e1 != nil || e2 != nil || len(fontsBefore) != 0 && !reflect.DeepEqual(fontsBefore, fontsAfter) {
		return patchInvalid
	}
	a, e := workbookPhysical(ctx, original)
	if e != nil {
		return e
	}
	b, e := workbookPhysical(ctx, candidate)
	if e != nil {
		return e
	}
	if len(a) != len(b) {
		return patchInvalid
	}
	for i, left := range a {
		right := b[i]
		if left.Name != right.Name || left.ID != right.ID || left.State != right.State || left.CodeName != right.CodeName || left.Filter != right.Filter || left.ZeroHeight != right.ZeroHeight || left.DefaultRowHeight != right.DefaultRowHeight || !reflect.DeepEqual(left.Merges, right.Merges) || !workbookSameExplicitSettings(left.Settings, right.Settings) {
			return patchInvalid
		}
		p1, e1 := before.GetPanes(left.Name)
		p2, e2 := after.GetPanes(left.Name)
		if e1 != nil || e2 != nil {
			return patchInvalid
		}
		p1.ActivePane = ""
		p2.ActivePane = ""
		p1.Selection = nil
		p2.Selection = nil
		if !reflect.DeepEqual(p1, p2) {
			return patchInvalid
		}
		rows := map[int]bool{1: true}
		for r := range left.Rows {
			rows[r] = true
		}
		for r := range right.Rows {
			rows[r] = true
		}
		for r := range rows {
			if ctx.Err() != nil {
				return patchInvalid
			}
			// GetRowHeight ignores sheet defaults unless customHeight=true.
			// Compare the actual declared default and optional row override so
			// native export materializing the same height stays equivalent.
			h1, ok := left.RowHeights[r]
			if !ok {
				h1 = left.DefaultRowHeight
			}
			h2, ok := right.RowHeights[r]
			if !ok {
				h2 = right.DefaultRowHeight
			}
			v1, e3 := before.GetRowVisible(left.Name, r)
			v2, e4 := after.GetRowVisible(left.Name, r)
			o1, e5 := before.GetRowOutlineLevel(left.Name, r)
			o2, e6 := after.GetRowOutlineLevel(left.Name, r)
			s1, e7 := beforeStyles.semantic(left.Rows[r])
			s2, e8 := afterStyles.semantic(right.Rows[r])
			if e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil || e8 != nil || h1 != h2 || v1 != v2 || o1 != o2 || s1 != s2 {
				return patchInvalid
			}
		}
		for col := range right.Columns {
			left.Columns[col] = true
		}
		for col := range left.Columns {
			if ctx.Err() != nil {
				return patchInvalid
			}
			name, _ := excelize.ColumnNumberToName(col)
			w1, e1 := before.GetColWidth(left.Name, name)
			w2, e2 := after.GetColWidth(left.Name, name)
			v1, e3 := before.GetColVisible(left.Name, name)
			v2, e4 := after.GetColVisible(left.Name, name)
			o1, e5 := before.GetColOutlineLevel(left.Name, name)
			o2, e6 := after.GetColOutlineLevel(left.Name, name)
			id1, e7 := before.GetColStyle(left.Name, name)
			id2, e8 := after.GetColStyle(left.Name, name)
			s1, e9 := beforeStyles.semantic(id1)
			s2, e10 := afterStyles.semantic(id2)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil || e8 != nil || e9 != nil || e10 != nil || w1 != w2 || v1 != v2 || o1 != o2 || s1 != s2 {
				return patchInvalid
			}
		}
		for address := range right.Cells {
			left.Cells[address] = true
		}
		for address := range left.Cells {
			if ctx.Err() != nil {
				return patchInvalid
			}
			col, row, _ := excelize.CellNameToCoordinates(address)
			id1, e1 := before.GetCellStyle(left.Name, address)
			id2, e2 := after.GetCellStyle(left.Name, address)
			s1, e3 := beforeStyles.semantic(id1)
			s2, e4 := afterStyles.semantic(id2)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || s1 != s2 {
				return patchInvalid
			}
			if i == scope.Sheet && col >= scope.StartColumn+1 && col <= scope.EndColumn+1 && row >= scope.StartRow+1 && row <= scope.EndRow+1 {
				continue
			}
			v1, e1 := workbookCellSemantic(before, left.Name, address)
			v2, e2 := workbookCellSemantic(after, left.Name, address)
			if e1 != nil || e2 != nil || v1 != v2 {
				return patchInvalid
			}
		}
	}
	return nil
}
