package officegeneration

import "strings"

const (
	WorkbookMaxPivots     = 20
	WorkbookMaxPivotCells = 100000
)

// Pivot is a refreshable native pivot, not a precomputed summary. This bounded
// version supports one row field, an optional column field, up to two report
// filters (initially all items), and one value field. Sheet references use IDs.
type Pivot struct {
	ID            string       `json:"id"`
	SourceSheetID string       `json:"sourceSheetId"`
	SourceRange   string       `json:"sourceRange"`
	TargetSheetID string       `json:"targetSheetId"`
	TargetRange   string       `json:"targetRange"`
	Rows          []string     `json:"rows"`
	Columns       []string     `json:"columns,omitempty"`
	Filters       []string     `json:"filters,omitempty"`
	Values        []PivotValue `json:"values"`
}

type PivotValue struct {
	Field     string `json:"field"`
	Aggregate string `json:"aggregate"`
}

type workbookPivotArea struct {
	sheetID string
	rect    workbookRectangle
}

func (r workbookRectangle) overlaps(other workbookRectangle) bool {
	return r.c1 <= other.c2 && other.c1 <= r.c2 && r.r1 <= other.r2 && other.r1 <= r.r2
}

func workbookValidatePivots(w Workbook) error {
	if len(w.Pivots) == 0 {
		return nil
	}
	if len(w.Pivots) > WorkbookMaxPivots {
		return workbookInvalid
	}
	sheets := map[string]Sheet{}
	cells := map[string]map[[2]int]Cell{}
	for _, sheet := range w.Sheets {
		sheets[sheet.ID] = sheet
		cells[sheet.ID] = map[[2]int]Cell{}
		for _, cell := range sheet.Cells {
			col, row, _, _ := workbookAddress(cell.Address)
			cells[sheet.ID][[2]int{col, row}] = cell
		}
	}
	ids := map[string]bool{}
	var sources, targets []workbookPivotArea
	budget := 0
	for _, pivot := range w.Pivots {
		key := workbookSheetKey(pivot.ID)
		if !workbookIDPattern.MatchString(pivot.ID) || ids[key] || len(pivot.Rows) != 1 || len(pivot.Columns) > 1 || len(pivot.Filters) > 2 || len(pivot.Values) != 1 {
			return workbookInvalid
		}
		ids[key] = true
		sourceSheet, sourceOK := sheets[pivot.SourceSheetID]
		targetSheet, targetOK := sheets[pivot.TargetSheetID]
		// Excelize's pivot range parser uses a literal ! delimiter; unlike the
		// formula parser it cannot represent sheet names containing that character.
		if !sourceOK || !targetOK || strings.ContainsAny(sourceSheet.Name+targetSheet.Name, "!") {
			return workbookInvalid
		}
		source, err := workbookRange(pivot.SourceRange)
		if err != nil || source.r2 <= source.r1 {
			return workbookInvalid
		}
		target, err := workbookRange(pivot.TargetRange)
		if err != nil {
			return workbookInvalid
		}
		budget += source.size() + target.size()
		if budget > WorkbookMaxPivotCells {
			return workbookInvalid
		}
		headers := map[string]int{}
		headerKeys := map[string]bool{}
		for col := source.c1; col <= source.c2; col++ {
			cell := cells[pivot.SourceSheetID][[2]int{col, source.r1}]
			label, ok := cell.Value.(string)
			if cell.Type != "text" || !ok || strings.TrimSpace(label) != label || label == "" || !workbookText(label, 240) || headerKeys[workbookSheetKey(label)] {
				return workbookInvalid
			}
			headers[label], headerKeys[workbookSheetKey(label)] = col, true
			columnType := ""
			for row := source.r1 + 1; row <= source.r2; row++ {
				cell, present := cells[pivot.SourceSheetID][[2]int{col, row}]
				// Formula results have no reliable typed persisted cache in this
				// encoder. Never synthesize one or silently aggregate blank caches.
				if !present || cell.Type == "formula" || (columnType != "" && columnType != cell.Type) {
					return workbookInvalid
				}
				if cell.Type == "text" && strings.TrimSpace(cell.Value.(string)) == "" {
					return workbookInvalid
				}
				columnType = cell.Type
			}
		}
		used := map[string]bool{}
		for _, fields := range [][]string{pivot.Rows, pivot.Columns, pivot.Filters} {
			for _, field := range fields {
				if _, ok := headers[field]; !ok || used[field] {
					return workbookInvalid
				}
				used[field] = true
			}
		}
		value := pivot.Values[0]
		valueColumn, ok := headers[value.Field]
		if !ok || used[value.Field] {
			return workbookInvalid
		}
		switch value.Aggregate {
		case "sum", "average", "min", "max":
			if cells[pivot.SourceSheetID][[2]int{valueColumn, source.r1 + 1}].Type != "number" {
				return workbookInvalid
			}
		case "count":
		default:
			return workbookInvalid
		}
		// Reserve enough room for every group, totals and headers. Filters may
		// be placed above the pivot body by native readers, so protect that area too.
		rowGroups := workbookPivotGroups(cells[pivot.SourceSheetID], source, headers[pivot.Rows[0]])
		colGroups := 1
		if len(pivot.Columns) > 0 {
			colGroups = workbookPivotGroups(cells[pivot.SourceSheetID], source, headers[pivot.Columns[0]])
		}
		if target.r2-target.r1+1 < rowGroups+3 || target.c2-target.c1+1 < colGroups+2 {
			return workbookInvalid
		}
		reserved := target
		if len(pivot.Filters) > 0 {
			reserved.r1 -= len(pivot.Filters) + 1
		}
		if reserved.r1 < 1 {
			return workbookInvalid
		}
		for coordinates := range cells[pivot.TargetSheetID] {
			if reserved.overlaps(workbookRectangle{coordinates[0], coordinates[1], coordinates[0], coordinates[1]}) {
				return workbookInvalid
			}
		}
		sources = append(sources, workbookPivotArea{pivot.SourceSheetID, source})
		targets = append(targets, workbookPivotArea{pivot.TargetSheetID, reserved})
	}
	for i, target := range targets {
		for _, source := range sources {
			if target.sheetID == source.sheetID && target.rect.overlaps(source.rect) {
				return workbookInvalid
			}
		}
		for j := 0; j < i; j++ {
			if target.sheetID == targets[j].sheetID && target.rect.overlaps(targets[j].rect) {
				return workbookInvalid
			}
		}
	}
	return nil
}

func workbookPivotGroups(cells map[[2]int]Cell, area workbookRectangle, column int) int {
	groups := map[any]bool{}
	for row := area.r1 + 1; row <= area.r2; row++ {
		cell := cells[[2]int{column, row}]
		value := cell.Value
		if cell.Type == "number" {
			value, _ = WorkbookNumber(value)
		}
		groups[value] = true
	}
	return len(groups)
}
