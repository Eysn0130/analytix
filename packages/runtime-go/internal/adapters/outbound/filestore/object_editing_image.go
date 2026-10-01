package filestore

import (
	"bytes"
	"context"
	"encoding/binary"
	"path/filepath"
	"strings"

	canvas "analytix.local/runtime-go/internal/domain/canvas"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

var _ editing.ImageFiles = (*ObjectEditingFiles)(nil)

// Browser animation and Go's first-frame PNG decoder cannot name the same
// selected pixels. Reject APNG instead of treating its first frame as authority.
func stillPNG(raw []byte) bool {
	for offset := 8; offset < len(raw); {
		if len(raw)-offset < 12 {
			return false
		}
		size := uint64(binary.BigEndian.Uint32(raw[offset : offset+4]))
		if size > uint64(len(raw)-offset-12) {
			return false
		}
		kind := string(raw[offset+4 : offset+8])
		if kind == "acTL" || kind == "fcTL" || kind == "fdAT" {
			return false
		}
		offset += int(size) + 12
		if kind == "IEND" {
			return offset == len(raw)
		}
	}
	return false
}
func (s *ObjectEditingFiles) readImageTarget(target objectEditingTarget) (editing.ImageDocument, error) {
	state, err := objectEditingInspectBounded(target.path, editing.MaxImageBytes)
	if err != nil {
		return editing.ImageDocument{}, err
	}
	raw := []byte(state.Content)
	mime := ""
	ext := strings.ToLower(filepath.Ext(target.path))
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) && ext == ".png" && stillPNG(raw):
		mime = "image/png"
	case bytes.HasPrefix(raw, []byte{0xff, 0xd8}) && (ext == ".jpg" || ext == ".jpeg"):
		mime = "image/jpeg"
	default:
		return editing.ImageDocument{}, editing.ErrNotText
	}
	dimensions, err := canvas.InspectImage(raw)
	if err != nil {
		return editing.ImageDocument{}, editing.ErrNotText
	}
	return editing.ImageDocument{Workspace: target.workspace, IdentityPath: target.identity, Path: target.path, Revision: digestAtomicText(raw), MIMEType: mime, Width: dimensions.Width, Height: dimensions.Height, Bytes: raw}, nil
}
func (s *ObjectEditingFiles) ReadImage(ctx context.Context, workspace, path string) (editing.ImageDocument, error) {
	if err := objectEditingLock(ctx); err != nil {
		return editing.ImageDocument{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(workspace, path)
	if err != nil {
		return editing.ImageDocument{}, err
	}
	image, err := s.readImageTarget(target)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return editing.ImageDocument{}, err
	}
	return image, nil
}
