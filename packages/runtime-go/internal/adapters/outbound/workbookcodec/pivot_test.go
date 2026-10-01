package workbookcodec

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	officegeneration "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

func codecPivotFixture() officegeneration.Workbook {
	w := officegeneration.Workbook{Sheets: []officegeneration.Sheet{{ID: "data", Name: "销售 O'Brien", Cells: []officegeneration.Cell{}}, {ID: "summary", Name: "汇总", Cells: []officegeneration.Cell{}}}}
	for row, values := range [][]any{{"Region", "Quarter", "Channel", "Sales"}, {"East", "Q1", "Direct", 12.5}, {"East", "Q1", "Online", 7.5}, {"West", "Q2", "Direct", 4.0}} {
		for col, value := range values {
			kind := "text"
			if _, ok := value.(float64); ok {
				kind = "number"
			}
			w.Sheets[0].Cells = append(w.Sheets[0].Cells, officegeneration.Cell{Address: fmt.Sprintf("%c%d", 'A'+col, row+1), Type: kind, Value: value})
		}
	}
	w.Pivots = []officegeneration.Pivot{{ID: "SalesPivot", SourceSheetID: "data", SourceRange: "$A$1:$D$4", TargetSheetID: "summary", TargetRange: "A4:E12", Rows: []string{"Region"}, Columns: []string{"Quarter"}, Filters: []string{"Channel"}, Values: []officegeneration.PivotValue{{Field: "Sales", Aggregate: "sum"}}}}
	return w
}

func TestEncodePivotRejectsDisplayRenamedHeader(t *testing.T) {
	w := codecPivotFixture()
	w.Sheets[0].Cells[0].Style = &officegeneration.CellStyle{NumberFormat: `"prefix"@`}
	if body, err := Encode(context.Background(), w); err == nil || len(body) != 0 {
		t.Fatal("display-renamed header silently lost its pivot field")
	}
}

func TestEncodeNativePivotDefinitionsAndRefreshContract(t *testing.T) {
	w := codecPivotFixture()
	body, err := Encode(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	entries := workbookZipEntries(t, body)
	var pivot struct {
		Name     string `xml:"name,attr"`
		CacheID  int    `xml:"cacheId,attr"`
		Location struct {
			Ref string `xml:"ref,attr"`
		} `xml:"location"`
		Fields []struct {
			Axis string `xml:"axis,attr"`
		} `xml:"pivotFields>pivotField"`
		Data []struct {
			Field    int    `xml:"fld,attr"`
			Subtotal string `xml:"subtotal,attr"`
		} `xml:"dataFields>dataField"`
	}
	if err := xml.Unmarshal(entries["xl/pivotTables/pivotTable1.xml"], &pivot); err != nil {
		t.Fatal(err)
	}
	if pivot.Name != "SalesPivot" || pivot.CacheID == 0 || pivot.Location.Ref != "A4:E12" || len(pivot.Fields) != 4 || len(pivot.Data) != 1 || pivot.Data[0].Field != 3 || pivot.Data[0].Subtotal != "sum" {
		t.Fatalf("unexpected pivot definition: %+v", pivot)
	}
	if pivot.Fields[0].Axis != "axisRow" || pivot.Fields[1].Axis != "axisCol" || pivot.Fields[2].Axis != "axisPage" {
		t.Fatalf("axes: %+v", pivot.Fields)
	}
	var cache struct {
		Save    bool `xml:"saveData,attr"`
		Refresh bool `xml:"refreshOnLoad,attr"`
		Source  struct {
			Sheet string `xml:"sheet,attr"`
			Ref   string `xml:"ref,attr"`
		} `xml:"cacheSource>worksheetSource"`
		Fields []struct {
			Name string `xml:"name,attr"`
		} `xml:"cacheFields>cacheField"`
	}
	if err := xml.Unmarshal(entries["xl/pivotCache/pivotCacheDefinition1.xml"], &cache); err != nil {
		t.Fatal(err)
	}
	if cache.Save || !cache.Refresh || cache.Source.Sheet != w.Sheets[0].Name || cache.Source.Ref != "A1:D4" || len(cache.Fields) != 4 || cache.Fields[3].Name != "Sales" {
		t.Fatalf("unexpected cache: %+v", cache)
	}
	for path, marker := range map[string]string{
		"xl/_rels/workbook.xml.rels":                "pivotCache/pivotCacheDefinition1.xml",
		"xl/pivotTables/_rels/pivotTable1.xml.rels": "../pivotCache/pivotCacheDefinition1.xml",
		"xl/worksheets/_rels/sheet2.xml.rels":       "../pivotTables/pivotTable1.xml",
		"[Content_Types].xml":                       "application/vnd.openxmlformats-officedocument.spreadsheetml.pivotTable+xml",
		"xl/workbook.xml":                           "pivotCaches",
	} {
		if !bytes.Contains(entries[path], []byte(marker)) {
			t.Fatalf("missing relationship/content type %s: %s", path, marker)
		}
	}
	for path := range entries {
		if strings.Contains(path, "pivotCacheRecords") {
			t.Fatal("unexpected fabricated result records")
		}
	}
	reopened, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	definitions, err := reopened.GetPivotTables("汇总")
	if err != nil || len(definitions) != 1 {
		t.Fatalf("reopen pivot: %v %+v", err, definitions)
	}
	if definitions[0].Name != "SalesPivot" || definitions[0].Rows[0].Data != "Region" || definitions[0].Columns[0].Data != "Quarter" || definitions[0].Filter[0].Data != "Channel" || definitions[0].Data[0].Subtotal != "Sum" {
		t.Fatalf("reopened definition: %+v", definitions[0])
	}
	if value, err := reopened.GetCellValue("汇总", "B5"); err != nil || value != "" {
		t.Fatalf("should require native refresh, got %q %v", value, err)
	}

	// Independent expected summaries from the reopened source cells. These
	// prove fixture values and aggregation expectations, not native refresh.
	groups := map[string][]float64{}
	for row := 2; row <= 4; row++ {
		region, _ := reopened.GetCellValue(w.Sheets[0].Name, fmt.Sprintf("A%d", row))
		quarter, _ := reopened.GetCellValue(w.Sheets[0].Name, fmt.Sprintf("B%d", row))
		raw, err := reopened.GetCellValue(w.Sheets[0].Name, fmt.Sprintf("D%d", row), excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatal(err)
		}
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			t.Fatal(err)
		}
		groups[region+"/"+quarter] = append(groups[region+"/"+quarter], n)
	}
	expected := map[string][5]float64{"East/Q1": {20, 2, 10, 7.5, 12.5}, "West/Q2": {4, 1, 4, 4, 4}}
	actual := map[string][5]float64{}
	for key, values := range groups {
		sum, min, max := 0.0, values[0], values[0]
		for _, n := range values {
			sum += n
			if n < min {
				min = n
			}
			if n > max {
				max = n
			}
		}
		actual[key] = [5]float64{sum, float64(len(values)), sum / float64(len(values)), min, max}
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("summary oracle: %v want %v", actual, expected)
	}
}

func TestEncodeNativePivotAggregatesAndMultiplePivots(t *testing.T) {
	w := codecPivotFixture()
	w.Pivots = nil
	for index, aggregate := range []string{"sum", "count", "average", "min", "max"} {
		pivot := codecPivotFixture().Pivots[0]
		pivot.ID = "Pivot_" + aggregate
		pivot.TargetRange = fmt.Sprintf("A%d:E%d", 4+index*15, 12+index*15)
		pivot.Values[0].Aggregate = aggregate
		w.Pivots = append(w.Pivots, pivot)
	}
	body, err := Encode(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pivots, err := f.GetPivotTables("汇总")
	if err != nil || len(pivots) != 5 {
		t.Fatalf("%v pivots=%d", err, len(pivots))
	}
	for i, p := range pivots {
		if !strings.EqualFold(p.Data[0].Subtotal, w.Pivots[i].Values[0].Aggregate) || p.Name != w.Pivots[i].ID {
			t.Fatalf("pivot %d: %+v", i, p)
		}
	}
}

func TestEncodeRejectsPivotFormulaSourceWithoutCreatingCache(t *testing.T) {
	w := codecPivotFixture()
	w.Sheets[0].Cells[7] = officegeneration.Cell{Address: "D2", Type: "formula", Formula: "10+2.5"}
	if body, err := Encode(context.Background(), w); err == nil || len(body) != 0 {
		t.Fatal("formula source admitted without a typed persisted cache")
	}
}
