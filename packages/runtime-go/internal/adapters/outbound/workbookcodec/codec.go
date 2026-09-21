// Package workbookcodec encodes a validated data workbook in memory. It does
// not open templates, access paths, resolve links, or invoke external programs.
package workbookcodec

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	officegeneration "analytix.local/runtime-go/internal/domain/officegeneration"
)

const MaxOutputBytes = 16 << 20

var workbookEncodingError = errors.New("workbook encoding failed")

// Encode preserves formula expressions and validates their real calculated
// values. Excelize's public calculator returns untyped strings and does not
// persist typed formula caches, so this encoder deliberately does not fabricate
// cached values. Native spreadsheet readers must recalculate on load.
func Encode(ctx context.Context, workbook officegeneration.Workbook) (body []byte, err error) {
	defer func() {
		if recover() != nil {
			body = nil
			err = workbookEncodingError
		}
	}()
	if ctx == nil {
		return nil, workbookEncodingError
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = workbook.Validate(); err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetZipWriter(func(writer io.Writer) excelize.ZipWriter {
		return zip.NewWriter(&workbookBoundedWriter{ctx: ctx, writer: writer, remaining: MaxOutputBytes})
	})
	// Create every sheet before formulas, including valid forward references.
	if err = f.SetSheetName(f.GetSheetName(0), workbook.Sheets[0].Name); err != nil {
		return nil, workbookEncodingError
	}
	for index, sheet := range workbook.Sheets {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if index > 0 {
			if _, err = f.NewSheet(sheet.Name); err != nil {
				return nil, workbookEncodingError
			}
		}
		id := sheet.ID
		if err = f.SetSheetProps(sheet.Name, &excelize.SheetPropsOptions{CodeName: &id}); err != nil {
			return nil, workbookEncodingError
		}
	}
	styles := map[string]int{}
	for _, sheet := range workbook.Sheets {
		cells := append([]officegeneration.Cell(nil), sheet.Cells...)
		// Excelize otherwise allocates the preceding row's column capacity for
		// every skipped row. Descending rows keep sparse gaps unallocated.
		sort.Slice(cells, func(i, j int) bool {
			ci, ri, _ := excelize.CellNameToCoordinates(cells[i].Address)
			cj, rj, _ := excelize.CellNameToCoordinates(cells[j].Address)
			if ri != rj {
				return ri > rj
			}
			return ci < cj
		})
		for _, cell := range cells {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			address := strings.ToUpper(cell.Address)
			switch cell.Type {
			case "text":
				err = f.SetCellStr(sheet.Name, address, cell.Value.(string))
			case "boolean":
				err = f.SetCellBool(sheet.Name, address, cell.Value.(bool))
			case "number":
				value, _ := officegeneration.WorkbookNumber(cell.Value)
				err = f.SetCellFloat(sheet.Name, address, value, -1, 64)
			case "formula":
				err = f.SetCellFormula(sheet.Name, address, strings.TrimPrefix(strings.TrimSpace(cell.Formula), "="))
			}
			if err != nil {
				return nil, workbookEncodingError
			}
			if cell.Style != nil {
				key, _ := json.Marshal(cell.Style)
				styleID, exists := styles[string(key)]
				if !exists {
					styleID, err = f.NewStyle(workbookStyle(*cell.Style))
					if err != nil {
						return nil, workbookEncodingError
					}
					styles[string(key)] = styleID
				}
				if err = f.SetCellStyle(sheet.Name, address, address, styleID); err != nil {
					return nil, workbookEncodingError
				}
			}
		}
		for index, width := range sheet.Columns {
			column, _ := excelize.ColumnNumberToName(index + 1)
			if err = f.SetColWidth(sheet.Name, column, column, width); err != nil {
				return nil, workbookEncodingError
			}
		}
		if sheet.FrozenRows > 0 {
			if err = f.SetPanes(sheet.Name, &excelize.Panes{Freeze: true, YSplit: sheet.FrozenRows, TopLeftCell: "A" + strconv.Itoa(sheet.FrozenRows+1), ActivePane: "bottomLeft"}); err != nil {
				return nil, workbookEncodingError
			}
		}
	}
	// Validate after all values and formulas are installed: a forward reference
	// must never be evaluated against a partially populated workbook.
	for _, sheet := range workbook.Sheets {
		for _, cell := range sheet.Cells {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			if cell.Type != "formula" {
				continue
			}
			value, calculationErr := f.CalcCellValue(sheet.Name, strings.ToUpper(cell.Address), excelize.Options{RawCellValue: true, MaxCalcIterations: 1})
			if calculationErr != nil {
				return nil, errors.New("workbook formula calculation failed")
			}
			// The calculator can return a non-finite numeric result without an
			// error. Its string-only API is ambiguous with literal "NaN"/"Inf"
			// results; reject those spellings rather than admit an overflow.
			if number, parseErr := strconv.ParseFloat(value, 64); parseErr == nil && (math.IsNaN(number) || math.IsInf(number, 0)) {
				return nil, errors.New("workbook formula calculation is non-finite")
			}
		}
	}
	for _, sheet := range workbook.Sheets {
		for _, chart := range sheet.Charts {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			kind := excelize.Bar
			if chart.Type == "line" {
				kind = excelize.Line
			}
			if chart.Type == "pie" {
				kind = excelize.Pie
			}
			native := excelize.Chart{Type: kind, Format: excelize.GraphicOptions{Name: chart.ID}, Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: chart.Title}}}}
			for _, series := range chart.Series {
				native.Series = append(native.Series, excelize.ChartSeries{Name: workbookChartReference(sheet.Name, series.Name), Categories: workbookChartReference(sheet.Name, series.Categories), Values: workbookChartReference(sheet.Name, series.Values)})
			}
			if err = f.AddChart(sheet.Name, strings.ToUpper(chart.Anchor), &native); err != nil {
				return nil, workbookEncodingError
			}
		}
	}
	sheetNames := map[string]string{}
	for _, sheet := range workbook.Sheets {
		sheetNames[sheet.ID] = sheet.Name
	}
	for _, pivot := range workbook.Pivots {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		// Excelize resolves pivot fields using formatted header values and can
		// silently omit an unmatched field. Refuse formats which rename a header.
		bounds := strings.Split(strings.ReplaceAll(pivot.SourceRange, "$", ""), ":")
		firstCol, firstRow, firstErr := excelize.CellNameToCoordinates(bounds[0])
		lastCol, _, lastErr := excelize.CellNameToCoordinates(bounds[len(bounds)-1])
		if firstErr != nil || lastErr != nil {
			return nil, workbookEncodingError
		}
		for col := firstCol; col <= lastCol; col++ {
			address, _ := excelize.CoordinatesToCellName(col, firstRow)
			raw, rawErr := f.GetCellValue(sheetNames[pivot.SourceSheetID], address, excelize.Options{RawCellValue: true})
			display, displayErr := f.GetCellValue(sheetNames[pivot.SourceSheetID], address)
			if rawErr != nil || displayErr != nil || raw != display {
				return nil, workbookEncodingError
			}
		}
		// Pivot ranges use Excelize's literal sheet-name syntax, not formula
		// quoting. Validation excludes ! names which that parser cannot encode.
		native := &excelize.PivotTableOptions{
			Name:            pivot.ID,
			DataRange:       sheetNames[pivot.SourceSheetID] + "!" + strings.ToUpper(pivot.SourceRange),
			PivotTableRange: sheetNames[pivot.TargetSheetID] + "!" + strings.ToUpper(pivot.TargetRange),
			Rows:            workbookPivotFields(pivot.Rows), Columns: workbookPivotFields(pivot.Columns), Filter: workbookPivotFields(pivot.Filters),
			RowGrandTotals: true, ColGrandTotals: true, ShowRowHeaders: true, ShowColHeaders: true,
			ClassicLayout: true,
		}
		for _, value := range pivot.Values {
			native.Data = append(native.Data, excelize.PivotTableField{Data: value.Field, Name: value.Aggregate + " " + value.Field, Subtotal: value.Aggregate})
		}
		// Excelize writes a real pivot/cache definition with saveData=false and
		// refreshOnLoad=true. It does not calculate the displayed pivot result or
		// persist cache records; consumers must refresh in a native spreadsheet.
		if err = f.AddPivotTable(native); err != nil {
			return nil, workbookEncodingError
		}
	}
	auto, on, off := "auto", true, false
	if err = f.SetCalcProps(&excelize.CalcPropsOptions{CalcMode: &auto, FullCalcOnLoad: &on, ForceFullCalc: &on, CalcOnSave: &on, CalcCompleted: &off, Iterate: &off}); err != nil {
		return nil, workbookEncodingError
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if buffer.Len() > MaxOutputBytes {
		return nil, errors.New("workbook output exceeds 16 MiB")
	}
	return buffer.Bytes(), nil
}

func workbookPivotFields(fields []string) []excelize.PivotTableField {
	result := make([]excelize.PivotTableField, 0, len(fields))
	for _, field := range fields {
		result = append(result, excelize.PivotTableField{Data: field, ShowAll: true})
	}
	return result
}

func workbookStyle(style officegeneration.CellStyle) *excelize.Style {
	native := &excelize.Style{Font: &excelize.Font{Bold: style.Bold, Color: strings.ToUpper(style.FontColor)}, Alignment: &excelize.Alignment{Horizontal: style.Align, WrapText: style.Wrap}}
	if style.Fill != "" {
		native.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{strings.ToUpper(style.Fill)}}
	}
	if style.NumberFormat != "" {
		native.CustomNumFmt = &style.NumberFormat
	}
	return native
}
func workbookChartReference(sheet, area string) string {
	parts := strings.Split(area, ":")
	for index, part := range parts {
		column, row, _ := excelize.CellNameToCoordinates(strings.ReplaceAll(part, "$", ""))
		name, _ := excelize.ColumnNumberToName(column)
		parts[index] = "$" + name + "$" + strconv.Itoa(row)
	}
	return "'" + strings.ReplaceAll(sheet, "'", "''") + "'!" + strings.Join(parts, ":")
}

type workbookBoundedWriter struct {
	ctx       context.Context
	writer    io.Writer
	remaining int
}

func (w *workbookBoundedWriter) Write(body []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(body) > w.remaining {
		return 0, errors.New("workbook output exceeds 16 MiB")
	}
	n, err := w.writer.Write(body)
	w.remaining -= n
	return n, err
}
