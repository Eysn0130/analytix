package workbookcodec

import (
	"archive/zip"
	"context"
	"io"
	"math"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	"github.com/xuri/excelize/v2"
)

// ApplyReview applies only the typed semantic differences in an already
// validated review to the actual source package. It returns no bytes unless the
// existing bounded feature admission and complete preservation verifier pass.
func ApplyReview(ctx context.Context, original []byte, review office.WorkbookReview) (body []byte, err error) {
	defer func() {
		if recover() != nil {
			body, err = nil, patchInvalid
		}
	}()
	if ctx == nil {
		return nil, patchInvalid
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if ValidateReview(ctx, review) != nil {
		return nil, patchInvalid
	}
	if err = ValidateNativeEditingPackage(ctx, original); err != nil {
		return nil, err
	}
	f, err := openPatchWorkbook(original)
	if err != nil {
		return nil, patchInvalid
	}
	defer f.Close()
	if verifySelectedCells(ctx, f, review.Before) != nil {
		return nil, patchInvalid
	}
	changed := false
	for i, cell := range review.After.Cells {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if workbookSameTypedCell(review.Before.Cells[i], cell) {
			continue
		}
		changed = true
		address := office.WorkbookCellAddress(cell.Column, cell.Row)
		switch cell.ValueType {
		case "number":
			// Excelize converts non-finite numbers to strings. Never let that
			// convenience reinterpret the Core's declared type.
			if math.IsNaN(cell.Value) || math.IsInf(cell.Value, 0) {
				return nil, patchInvalid
			}
			err = f.SetCellFloat(review.After.SheetName, address, cell.Value, -1, 64)
		case "text":
			err = f.SetCellStr(review.After.SheetName, address, cell.Text)
		case "empty":
			err = f.SetCellDefault(review.After.SheetName, address, "")
		case "formula":
			// SetCellFormula alone can retain a previous value/cache. Clear the
			// old typed cell first and let the existing verifier recalculate.
			err = f.SetCellDefault(review.After.SheetName, address, "")
			if err == nil {
				err = f.SetCellFormula(review.After.SheetName, address, office.CanonicalWorkbookFormula(cell.Formula))
			}
		default:
			return nil, patchInvalid
		}
		if err != nil {
			return nil, patchInvalid
		}
	}
	if !changed {
		// Saving an identical hash cannot yield the changed revision expected
		// by the receipt contract. Reject a display-only/no-op review before
		// producing a candidate rather than manufacturing a successful save.
		return nil, patchInvalid
	}
	f.SetZipWriter(func(writer io.Writer) excelize.ZipWriter {
		return zip.NewWriter(&workbookBoundedWriter{ctx: ctx, writer: writer, remaining: MaxOutputBytes})
	})
	output, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if output.Len() == 0 || output.Len() > MaxOutputBytes {
		return nil, ErrUnsupportedWorkbook
	}
	if err = VerifyPatchedPackage(ctx, original, output.Bytes(), review); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func workbookSameTypedCell(before, after office.WorkbookSelectedCell) bool {
	if before.ValueType != after.ValueType {
		return false
	}
	switch after.ValueType {
	case "empty":
		return true
	case "number":
		return before.Value == after.Value
	case "text":
		return before.Text == after.Text
	case "formula":
		return office.CanonicalWorkbookFormula(before.Formula) == office.CanonicalWorkbookFormula(after.Formula)
	}
	return false
}
