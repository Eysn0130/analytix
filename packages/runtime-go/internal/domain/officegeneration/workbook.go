package officegeneration

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	WorkbookMaxInputBytes    = 16 << 20
	WorkbookMaxSheets        = 20
	WorkbookMaxCells         = 10000
	WorkbookMaxRows          = 100000
	WorkbookMaxColumns       = 1024
	WorkbookMaxCharts        = 100
	WorkbookMaxRangeCells    = 10000
	WorkbookMaxExpandedCells = 1000000
)

type Workbook struct {
	Sheets []Sheet `json:"sheets"`
	Pivots []Pivot `json:"pivots,omitempty"`
}
type Sheet struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Cells      []Cell    `json:"cells"`
	Columns    []float64 `json:"columns,omitempty"`
	FrozenRows int       `json:"frozenRows,omitempty"`
	Charts     []Chart   `json:"charts,omitempty"`
}
type Cell struct {
	Address string     `json:"address"`
	Type    string     `json:"type"`
	Value   any        `json:"value,omitempty"`
	Formula string     `json:"formula,omitempty"`
	Style   *CellStyle `json:"style,omitempty"`
}
type CellStyle struct {
	Bold         bool   `json:"bold,omitempty"`
	FontColor    string `json:"fontColor,omitempty"`
	Fill         string `json:"fill,omitempty"`
	Align        string `json:"align,omitempty"`
	Wrap         bool   `json:"wrap,omitempty"`
	NumberFormat string `json:"numberFormat,omitempty"`
}
type Chart struct {
	ID     string        `json:"id"`
	Type   string        `json:"type"`
	Title  string        `json:"title,omitempty"`
	Anchor string        `json:"anchor"`
	Series []ChartSeries `json:"series"`
}
type ChartSeries struct {
	// Name is a current-sheet single-cell reference to a non-empty text label.
	Name       string `json:"name"`
	Categories string `json:"categories"`
	Values     string `json:"values"`
}

var workbookIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var workbookColorPattern = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)
var workbookAddressPattern = regexp.MustCompile(`^\$?([A-Za-z]{1,3})\$?([1-9][0-9]{0,5})$`)
var workbookInvalid = errors.New("invalid or unsupported workbook")

// ParseWorkbook rejects duplicate, unknown, case-aliased and null fields before
// decoding. Cell values retain their explicit JSON type, including false/zero.
func ParseWorkbook(body []byte) (Workbook, error) {
	var result Workbook
	value, err := jsonstrict.DecodeObject(body, jsonstrict.Options{MaxBytes: WorkbookMaxInputBytes, MaxDepth: 12, MaxTokens: 250000, MaxStringBytes: 131068, MaxNumberBytes: 128, MaxAbsExponent: 308})
	if err != nil || workbookCheckShape(value, "workbook") != nil {
		return result, workbookInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Workbook{}, workbookInvalid
	}
	if err := result.Validate(); err != nil {
		return Workbook{}, err
	}
	return result, nil
}

func workbookCheckShape(v any, kind string) error {
	object, ok := v.(map[string]any)
	if !ok {
		return workbookInvalid
	}
	var required, optional string
	switch kind {
	case "workbook":
		required, optional = "sheets", "pivots"
	case "sheet":
		required, optional = "id name cells", "columns frozenRows charts"
	case "cell":
		required, optional = "address type", "value formula style"
	case "style":
		optional = "bold fontColor fill align wrap numberFormat"
	case "chart":
		required, optional = "id type anchor series", "title"
	case "series":
		required = "name categories values"
	case "pivot":
		required, optional = "id sourceSheetId sourceRange targetSheetId targetRange rows values", "columns filters"
	case "pivotValue":
		required = "field aggregate"
	default:
		return workbookInvalid
	}
	allowed := " " + required + " " + optional + " "
	for key, child := range object {
		if child == nil || !strings.Contains(allowed, " "+key+" ") {
			return workbookInvalid
		}
	}
	for _, key := range strings.Fields(required) {
		if _, ok := object[key]; !ok {
			return workbookInvalid
		}
	}
	if kind == "cell" {
		_, valuePresent := object["value"]
		_, formulaPresent := object["formula"]
		if object["type"] == "formula" {
			if valuePresent || !formulaPresent {
				return workbookInvalid
			}
		} else if !valuePresent || formulaPresent {
			return workbookInvalid
		}
	}
	if kind == "style" {
		for _, key := range []string{"fontColor", "fill", "align"} {
			if value, exists := object[key]; exists && value == "" {
				return workbookInvalid
			}
		}
	}
	children := map[string]string{"sheets": "sheet", "cells": "cell", "charts": "chart", "series": "series", "pivots": "pivot"}
	if kind == "pivot" {
		children["values"] = "pivotValue"
	}
	for key, childKind := range children {
		if child, ok := object[key]; ok {
			items, ok := child.([]any)
			if !ok {
				return workbookInvalid
			}
			for _, item := range items {
				if workbookCheckShape(item, childKind) != nil {
					return workbookInvalid
				}
			}
		}
	}
	if child, ok := object["style"]; ok {
		return workbookCheckShape(child, "style")
	}
	return nil
}

func (w Workbook) Validate() error {
	if len(w.Sheets) < 1 || len(w.Sheets) > WorkbookMaxSheets {
		return workbookInvalid
	}
	ids, names, chartIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	totalCells, totalCharts, textBytes, expandedCells := 0, 0, 0, 0
	for _, sheet := range w.Sheets {
		key := workbookSheetKey(sheet.Name)
		if !workbookIDPattern.MatchString(sheet.ID) || ids[sheet.ID] || names[key] || !workbookSheetName(sheet.Name) {
			return workbookInvalid
		}
		ids[sheet.ID], names[key] = true, true
		totalCells += len(sheet.Cells)
		totalCharts += len(sheet.Charts)
		if totalCells > WorkbookMaxCells || totalCharts > WorkbookMaxCharts || len(sheet.Columns) > WorkbookMaxColumns || sheet.FrozenRows < 0 || sheet.FrozenRows >= WorkbookMaxRows {
			return workbookInvalid
		}
		for _, width := range sheet.Columns {
			if !workbookFinite(width) || width <= 0 || width > 255 {
				return workbookInvalid
			}
		}
		addresses := map[string]bool{}
		textCells := map[string]bool{}
		rowColumns := map[int]int{}
		for _, cell := range sheet.Cells {
			column, row, address, err := workbookAddress(cell.Address)
			if err != nil || strings.Contains(cell.Address, "$") || addresses[address] {
				return workbookInvalid
			}
			if column > rowColumns[row] {
				expandedCells += column - rowColumns[row]
				rowColumns[row] = column
			}
			if expandedCells > WorkbookMaxExpandedCells {
				return errors.New("workbook exceeds bounded sparse cell storage")
			}
			addresses[address] = true
			if cell.Type != "formula" && cell.Formula != "" {
				return workbookInvalid
			}
			switch cell.Type {
			case "text":
				value, ok := cell.Value.(string)
				if !ok || !workbookText(value, 32767) {
					return workbookInvalid
				}
				textBytes += len(value)
				textCells[address] = strings.TrimSpace(value) != ""
			case "number":
				if _, err := WorkbookNumber(cell.Value); err != nil {
					return workbookInvalid
				}
			case "boolean":
				if _, ok := cell.Value.(bool); !ok {
					return workbookInvalid
				}
			case "formula":
				if cell.Value != nil || len(cell.Formula) == 0 || len(cell.Formula) > 1024 || !workbookText(cell.Formula, 1024) {
					return workbookInvalid
				}
				textBytes += len(cell.Formula)
			default:
				return workbookInvalid
			}
			if style := cell.Style; style != nil {
				if (style.FontColor != "" && !workbookColorPattern.MatchString(style.FontColor)) || (style.Fill != "" && !workbookColorPattern.MatchString(style.Fill)) ||
					(style.Align != "" && style.Align != "left" && style.Align != "center" && style.Align != "right") || !workbookText(style.NumberFormat, 255) {
					return workbookInvalid
				}
				textBytes += len(style.NumberFormat)
			}
		}
		for _, chart := range sheet.Charts {
			if !workbookIDPattern.MatchString(chart.ID) || chartIDs[chart.ID] || !workbookText(chart.Title, 255) || len(chart.Series) < 1 || len(chart.Series) > 20 {
				return workbookInvalid
			}
			chartIDs[chart.ID] = true
			if chart.Type != "bar" && chart.Type != "line" && chart.Type != "pie" {
				return workbookInvalid
			}
			if chart.Type == "pie" && len(chart.Series) != 1 {
				return workbookInvalid
			}
			if _, _, _, err := workbookAddress(chart.Anchor); err != nil || strings.Contains(chart.Anchor, "$") {
				return workbookInvalid
			}
			for _, series := range chart.Series {
				_, _, labelAddress, labelErr := workbookAddress(series.Name)
				if labelErr != nil || !textCells[labelAddress] {
					return workbookInvalid
				}
				categories, err := workbookRange(series.Categories)
				if err != nil {
					return workbookInvalid
				}
				values, err := workbookRange(series.Values)
				if err != nil {
					return workbookInvalid
				}
				if !categories.vector() || !values.vector() || categories.size() != values.size() {
					return workbookInvalid
				}
			}
		}
	}
	if textBytes > WorkbookMaxInputBytes {
		return workbookInvalid
	}
	if err := workbookValidatePivots(w); err != nil {
		return err
	}
	return workbookValidateFormulas(w, names)
}

// WorkbookNumber converts only native numeric values and JSON numbers. It never
// coerces text or booleans into numeric cells.
func WorkbookNumber(value any) (float64, error) {
	var number float64
	switch v := value.(type) {
	case json.Number:
		var err error
		number, err = v.Float64()
		if err != nil {
			return 0, workbookInvalid
		}
	case float64:
		number = v
	case float32:
		number = float64(v)
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	case int32:
		number = float64(v)
	case uint:
		number = float64(v)
	case uint64:
		number = float64(v)
	case uint32:
		number = float64(v)
	default:
		return 0, workbookInvalid
	}
	if !workbookFinite(number) {
		return 0, workbookInvalid
	}
	return number, nil
}
func workbookFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func workbookText(value string, limit int) bool {
	if !utf8.ValidString(value) || len(utf16.Encode([]rune(value))) > limit {
		return false
	}
	for _, r := range value {
		if (r < 32 && r != '\t' && r != '\n' && r != '\r') || r == 0xfffe || r == 0xffff {
			return false
		}
	}
	return true
}
func workbookSheetName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && workbookText(name, 31) && !strings.ContainsAny(name, ":\\/?*[]\t\r\n") && !strings.HasPrefix(name, "'") && !strings.HasSuffix(name, "'")
}
func workbookAddress(address string) (int, int, string, error) {
	parts := workbookAddressPattern.FindStringSubmatch(address)
	if parts == nil {
		return 0, 0, "", workbookInvalid
	}
	column := 0
	for _, c := range strings.ToUpper(parts[1]) {
		column = column*26 + int(c-'A'+1)
	}
	row, err := strconv.Atoi(parts[2])
	if err != nil || column > WorkbookMaxColumns || row > WorkbookMaxRows {
		return 0, 0, "", workbookInvalid
	}
	return column, row, strings.ToUpper(parts[1]) + parts[2], nil
}

type workbookRectangle struct{ c1, r1, c2, r2 int }

func (r workbookRectangle) size() int    { return (r.c2 - r.c1 + 1) * (r.r2 - r.r1 + 1) }
func (r workbookRectangle) vector() bool { return r.c1 == r.c2 || r.r1 == r.r2 }
func workbookRange(value string) (workbookRectangle, error) {
	parts := strings.Split(value, ":")
	if len(parts) > 2 {
		return workbookRectangle{}, workbookInvalid
	}
	c1, r1, _, err := workbookAddress(parts[0])
	if err != nil {
		return workbookRectangle{}, err
	}
	c2, r2 := c1, r1
	if len(parts) == 2 {
		c2, r2, _, err = workbookAddress(parts[1])
		if err != nil {
			return workbookRectangle{}, err
		}
	}
	r := workbookRectangle{c1, r1, c2, r2}
	if c2 < c1 || r2 < r1 || r.size() > WorkbookMaxRangeCells {
		return workbookRectangle{}, workbookInvalid
	}
	return r, nil
}

// Match Excelize's EqualFold sheet identity, including non-ASCII fold aliases.
func workbookSheetKey(name string) string {
	return strings.Map(func(r rune) rune {
		smallest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < smallest {
				smallest = next
			}
		}
		return smallest
	}, name)
}
