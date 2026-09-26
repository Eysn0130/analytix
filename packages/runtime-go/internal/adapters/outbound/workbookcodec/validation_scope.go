package workbookcodec

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
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

type workbookPhysicalSheet struct {
	Name, ID, State, CodeName, Filter string
	ZeroHeight                        bool
	Cells                             map[string]bool
	Rows                              map[int]int
	Columns                           map[int]bool
	Merges                            map[string]bool
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
	z, e := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if e != nil {
		return nil, patchInvalid
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if f.UncompressedSize64 > 64<<20 {
			return nil, ErrUnsupportedWorkbook
		}
		total += f.UncompressedSize64
		if total > 64<<20 || len(entries) >= 4096 {
			return nil, ErrUnsupportedWorkbook
		}
		if entries[f.Name] != nil {
			return nil, patchInvalid
		}
		entries[f.Name] = f
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
		s := workbookPhysicalSheet{Name: sheet.Name, ID: sheet.ID, State: sheet.State, Cells: map[string]bool{}, Rows: map[int]int{}, Columns: map[int]bool{1: true, 16384: true}, Merges: map[string]bool{}}
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
			case "sheetFormatPr":
				s.ZeroHeight = attrs["zeroHeight"] == "1" || attrs["zeroHeight"] == "true"
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
func workbookSemanticStyle(f *excelize.File, id int) (string, error) {
	s, e := f.GetStyle(id)
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
	return string(raw), e
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
func verifyWorkbookOutsideScope(ctx context.Context, before, after *excelize.File, original, candidate []byte, scope office.WorkbookSelection) error {
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
		if left.Name != right.Name || left.ID != right.ID || left.State != right.State || left.CodeName != right.CodeName || left.Filter != right.Filter || left.ZeroHeight != right.ZeroHeight || !reflect.DeepEqual(left.Merges, right.Merges) {
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
			h1, e1 := before.GetRowHeight(left.Name, r)
			h2, e2 := after.GetRowHeight(left.Name, r)
			v1, e3 := before.GetRowVisible(left.Name, r)
			v2, e4 := after.GetRowVisible(left.Name, r)
			o1, e5 := before.GetRowOutlineLevel(left.Name, r)
			o2, e6 := after.GetRowOutlineLevel(left.Name, r)
			s1, e7 := workbookSemanticStyle(before, left.Rows[r])
			s2, e8 := workbookSemanticStyle(after, right.Rows[r])
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil || e8 != nil || h1 != h2 || v1 != v2 || o1 != o2 || s1 != s2 {
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
			s1, e9 := workbookSemanticStyle(before, id1)
			s2, e10 := workbookSemanticStyle(after, id2)
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
			s1, e3 := workbookSemanticStyle(before, id1)
			s2, e4 := workbookSemanticStyle(after, id2)
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
