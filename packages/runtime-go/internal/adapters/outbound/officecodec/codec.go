// Package officecodec composes data-only native Office writers. It never resolves
// workspace paths or grants file authority; Core owns artifact persistence.
package officecodec

import (
	"analytix.local/runtime-go/internal/adapters/outbound/workbookcodec"
	"analytix.local/runtime-go/internal/domain/officegeneration"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	"context"
)

type Codec struct{ node codecport.Codec }

func New(node codecport.Codec) *Codec { return &Codec{node: node} }
func (c *Codec) Supports(kind string) bool {
	return c != nil && (kind == "xlsx" || codecport.Supports(c.node, kind))
}
func (c *Codec) Encode(ctx context.Context, input codecport.Input) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || input.SchemaVersion != 1 || !c.Supports(input.Kind) {
		return nil, codecport.ErrCodecUnavailable
	}
	if input.Kind != "xlsx" {
		return c.node.Encode(ctx, input)
	}
	if input.Markdown != "" || len(input.Presentation) > 0 || len(input.Images) > 0 {
		return nil, codecport.ErrCodecUnavailable
	}
	workbook, err := officegeneration.ParseWorkbook(input.Workbook)
	if err != nil {
		return nil, codecport.ErrCodecUnavailable
	}
	body, err := workbookcodec.Encode(ctx, workbook)
	if err != nil {
		return nil, codecport.ErrCodecUnavailable
	}
	return body, nil
}
