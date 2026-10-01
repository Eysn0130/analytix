package workbookcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	officegeneration "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

func workbookCodecFixture() officegeneration.Workbook {
	return officegeneration.Workbook{Sheets: []officegeneration.Sheet{
		{ID: "sales", Name: "销售 数据", Columns: []float64{22, 14}, FrozenRows: 1, Cells: []officegeneration.Cell{
			{Address: "A1", Type: "text", Value: "月份"},
			{Address: "A2", Type: "text", Value: "一月"},
			{Address: "A3", Type: "text", Value: "二月"},
			{Address: "B2", Type: "number", Value: 12.5, Style: &officegeneration.CellStyle{Bold: true, FontColor: "ff0000", Fill: "DDEEFF", Align: "center", Wrap: true, NumberFormat: "0.00"}},
			{Address: "B3", Type: "number", Value: 7.5},
			{Address: "C2", Type: "boolean", Value: false},
			{Address: "C3", Type: "boolean", Value: true},
			{Address: "D2", Type: "formula", Formula: "=SUM(B2:B3)"},
			{Address: "D3", Type: "formula", Formula: "IF(B2>0,TRUE,FALSE)"},
			{Address: "E2", Type: "text", Value: "=SUM(B2:B3)"},
			{Address: "E3", Type: "formula", Formula: "'其他'!B1+1"},
			{Address: "F3", Type: "formula", Formula: `IF(TRUE,"中文结果","空")`},
			{Address: "B1", Type: "text", Value: "收入"},
		}, Charts: []officegeneration.Chart{
			{ID: "bar", Type: "bar", Title: "销售柱图", Anchor: "G2", Series: []officegeneration.ChartSeries{{Name: "B1", Categories: "A2:A3", Values: "B2:B3"}}},
			{ID: "line", Type: "line", Title: "趋势", Anchor: "G20", Series: []officegeneration.ChartSeries{{Name: "B1", Categories: "A2:A3", Values: "B2:B3"}}},
			{ID: "pie", Type: "pie", Title: "占比", Anchor: "G38", Series: []officegeneration.ChartSeries{{Name: "B1", Categories: "A2:A3", Values: "B2:B3"}}},
		}},
		{ID: "other", Name: "其他", Cells: []officegeneration.Cell{{Address: "B1", Type: "number", Value: 4}}},
	}}
}

func workbookZipEntries(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = data
	}
	return entries
}

func TestEncodeWorkbookOOXMLAndReopenedCalculations(t *testing.T) {
	workbook := workbookCodecFixture()
	before, _ := json.Marshal(workbook)
	body, err := Encode(context.Background(), workbook)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(workbook)
	if !bytes.Equal(before, after) {
		t.Fatal("encoder mutated caller workbook")
	}
	if len(body) == 0 || len(body) > MaxOutputBytes {
		t.Fatal("invalid output size")
	}
	entries := workbookZipEntries(t, body)
	var worksheet struct {
		Cells []struct {
			Address string `xml:"r,attr"`
			Type    string `xml:"t,attr"`
			Style   string `xml:"s,attr"`
			Formula string `xml:"f"`
			Value   string `xml:"v"`
		} `xml:"sheetData>row>c"`
	}
	if err := xml.Unmarshal(entries["xl/worksheets/sheet1.xml"], &worksheet); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, cell := range worksheet.Cells {
		seen[cell.Address] = true
		switch cell.Address {
		case "B2":
			if cell.Value != "12.5" || (cell.Type != "" && cell.Type != "n") || cell.Style == "" {
				t.Fatal("numeric value/style lost", cell)
			}
		case "C2":
			if cell.Type != "b" || cell.Value != "0" {
				t.Fatal("false boolean type lost", cell)
			}
		case "C3":
			if cell.Type != "b" || cell.Value != "1" {
				t.Fatal("true boolean type lost", cell)
			}
		case "D2":
			if cell.Formula != "SUM(B2:B3)" || cell.Value != "" {
				t.Fatal("formula lost or untyped cache fabricated", cell)
			}
		case "D3", "E3", "F3":
			if cell.Formula == "" || cell.Value != "" {
				t.Fatal("formula cache contract changed", cell)
			}
		case "E2":
			if cell.Formula != "" || (cell.Type != "s" && cell.Type != "inlineStr") {
				t.Fatal("leading equals text executed as formula", cell)
			}
		}
	}
	for _, address := range []string{"B2", "C2", "C3", "D2", "D3", "E2", "E3", "F3"} {
		if !seen[address] {
			t.Fatal("missing cell", address)
		}
	}
	for part, fragments := range map[string][]string{
		"xl/workbook.xml":          {`calcMode="auto"`, `fullCalcOnLoad="true"`, `forceFullCalc="true"`},
		"xl/worksheets/sheet1.xml": {`state="frozen"`, `ySplit="1"`, `topLeftCell="A2"`, `width="22"`},
		"xl/styles.xml":            {`FF0000`, `DDEEFF`, `formatCode="0.00"`, `horizontal="center"`, `wrapText="true"`},
		"xl/charts/chart1.xml":     {"barChart"},
		"xl/charts/chart2.xml":     {"lineChart"},
		"xl/charts/chart3.xml":     {"pieChart"},
	} {
		for _, fragment := range fragments {
			if !bytes.Contains(entries[part], []byte(fragment)) {
				t.Fatalf("missing %q in %s", fragment, part)
			}
		}
	}
	for path, data := range entries {
		if strings.Contains(path, "externalLink") || strings.Contains(path, "vbaProject") || bytes.Contains(data, []byte(`TargetMode="External"`)) {
			t.Fatal("unexpected executable or external part", path)
		}
	}
	for _, id := range []string{"bar", "line", "pie"} {
		if !bytes.Contains(entries["xl/drawings/drawing1.xml"], []byte(`name="`+id+`"`)) {
			t.Fatal("chart stable ID missing", id)
		}
	}
	for _, part := range []string{"xl/charts/chart1.xml", "xl/charts/chart2.xml", "xl/charts/chart3.xml"} {
		decoder := xml.NewDecoder(bytes.NewReader(entries[part]))
		var formulas []string
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if start, ok := token.(xml.StartElement); ok && start.Name.Local == "f" {
				var reference string
				if err := decoder.DecodeElement(&reference, &start); err != nil {
					t.Fatal(err)
				}
				formulas = append(formulas, reference)
			}
		}
		if len(formulas) != 3 || formulas[0] != "'销售 数据'!$B$1" || formulas[1] != "'销售 数据'!$A$2:$A$3" || formulas[2] != "'销售 数据'!$B$2:$B$3" {
			t.Fatal("chart label/data references are not exact local absolute references", part, formulas)
		}
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, sheet := range workbook.Sheets {
		properties, err := f.GetSheetProps(sheet.Name)
		if err != nil || properties.CodeName == nil || *properties.CodeName != sheet.ID {
			t.Fatal("sheet stable ID missing", err)
		}
	}
	for address, expected := range map[string]string{"D2": "20", "D3": "1", "E3": "5", "F3": "中文结果"} {
		value, err := f.CalcCellValue("销售 数据", address, excelize.Options{RawCellValue: true})
		if err != nil || value != expected {
			t.Fatalf("reopened %s = %q, expected %q: %v", address, value, expected, err)
		}
	}
	if value, err := f.GetCellValue("销售 数据", "E2"); err != nil || value != "=SUM(B2:B3)" {
		t.Fatal("literal formula-looking text changed", value, err)
	}
	styleID, err := f.GetCellStyle("销售 数据", "B2")
	if err != nil {
		t.Fatal(err)
	}
	style, err := f.GetStyle(styleID)
	if err != nil || style.Font == nil || !style.Font.Bold {
		t.Fatal("native bold style missing", err)
	}
}

func TestEncodeWorkbookSupportedFormulaFunctions(t *testing.T) {
	for formula, expected := range map[string]string{
		"1+2*3": "7", "AVERAGE(B2:B3)": "10", "MIN(B2:B3)": "7.5", "MAX(B2:B3)": "12.5", "COUNT(B2:B3)": "2", "COUNTA(A1:A3)": "3",
		`COUNTIF(B2:B3,">10")`: "1", `SUMIF(B2:B3,">10",B2:B3)`: "12.5", "ROUND(ABS(-1.234),2)": "1.23",
	} {
		t.Run(formula, func(t *testing.T) {
			workbook := workbookCodecFixture()
			workbook.Sheets[0].Charts = nil
			workbook.Sheets[0].Cells[7].Formula = formula
			body, err := Encode(context.Background(), workbook)
			if err != nil {
				t.Fatal(err)
			}
			f, err := excelize.OpenReader(bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			value, err := f.CalcCellValue("销售 数据", "D2", excelize.Options{RawCellValue: true})
			if err != nil || value != expected {
				t.Fatalf("got %q, expected %q: %v", value, expected, err)
			}
		})
	}
}

func TestEncodeWorkbookRejectsCalculationErrors(t *testing.T) {
	for _, formula := range []string{"1/0", "1e308*1e308", "AVERAGE(Z1:Z2)", `ABS("bad")`, "D2+1", `WEBSERVICE("https://invalid.example")`, "[remote.xlsx]Sheet1!A1", "Missing!A1"} {
		t.Run(formula, func(t *testing.T) {
			w := workbookCodecFixture()
			w.Sheets[0].Cells[7].Formula = formula
			if body, err := Encode(context.Background(), w); err == nil || body != nil {
				t.Fatal("invalid formula produced workbook")
			}
		})
	}
}

func TestEncodeWorkbookSparseBoundaryAndDefaultSheetNames(t *testing.T) {
	w := officegeneration.Workbook{Sheets: []officegeneration.Sheet{
		{ID: "first", Name: "_analytix_tmp_1", Cells: []officegeneration.Cell{{Address: "AMJ1", Type: "number", Value: 1}, {Address: "A100000", Type: "number", Value: 2}}},
		{ID: "second", Name: "_analytix_tmp_0", Cells: []officegeneration.Cell{}},
		{ID: "third", Name: "Sheet1", Cells: []officegeneration.Cell{}},
	}}
	body, err := Encode(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(f.GetSheetList()) != 3 {
		t.Fatal("temporary sheet leaked")
	}
	for address, expected := range map[string]string{"AMJ1": "1", "A100000": "2"} {
		if got, err := f.GetCellValue(w.Sheets[0].Name, address); err != nil || got != expected {
			t.Fatal("sparse value lost", address, got, err)
		}
	}
}

func TestWorkbookContextAndBoundedOutputWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if body, err := Encode(ctx, workbookCodecFixture()); !errors.Is(err, context.Canceled) || body != nil {
		t.Fatal("cancelled encode produced output", err)
	}
	var buffer bytes.Buffer
	writer := workbookBoundedWriter{ctx: context.Background(), writer: &buffer, remaining: 4}
	if n, err := writer.Write([]byte("1234")); err != nil || n != 4 {
		t.Fatal(err)
	}
	if n, err := writer.Write([]byte("5")); err == nil || n != 0 || buffer.String() != "1234" {
		t.Fatal("output bound leaked bytes")
	}
	writer = workbookBoundedWriter{ctx: ctx, writer: &buffer, remaining: 100}
	if n, err := writer.Write([]byte("cancelled")); !errors.Is(err, context.Canceled) || n != 0 {
		t.Fatal("writer ignored cancellation", err)
	}
}

func TestEncodeWorkbookQuotedSheetReference(t *testing.T) {
	w := officegeneration.Workbook{Sheets: []officegeneration.Sheet{
		{ID: "data", Name: "O'Brien", Cells: []officegeneration.Cell{{Address: "A1", Type: "number", Value: 3}}},
		{ID: "summary", Name: "汇总", Cells: []officegeneration.Cell{{Address: "A1", Type: "formula", Formula: "'O''Brien'!A1+2"}}},
	}}
	body, err := Encode(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	value, err := f.CalcCellValue("汇总", "A1", excelize.Options{RawCellValue: true})
	if err != nil || value != "5" {
		t.Fatal("quoted sheet reference changed", value, err)
	}
}

func TestWorkbookSUMIFAndUnicodeAliasRecalculateFromSameCells(t *testing.T) {
	w := officegeneration.Workbook{Sheets: []officegeneration.Sheet{{ID: "source", Name: "Σ", Cells: []officegeneration.Cell{
		{Address: "A1", Type: "number", Value: 1}, {Address: "A2", Type: "number", Value: 1}, {Address: "B1", Type: "number", Value: 5}, {Address: "B2", Type: "number", Value: 7},
		{Address: "C1", Type: "formula", Formula: `SUMIF(A1:A2,">0",B1:B2)`}, {Address: "C2", Type: "formula", Formula: `'ς'!B2+1`},
	}}}}
	body, err := Encode(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for cell, want := range map[string]string{"C1": "12", "C2": "8"} {
		got, err := f.CalcCellValue("Σ", cell)
		if err != nil || got != want {
			t.Fatal(cell, got, err)
		}
	}
}
