package filestore

import (
	"bytes"

	canvasdomain "analytix.local/runtime-go/internal/domain/canvas"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

// Canvas objects reuse the existing native CAS, protected original and recovery
// journal. The historical officeKind/office-base64 names are private encodings;
// this constructor never admits a Canvas resource to the Office plugin host.
// JPEG imports must be explicitly converted into a new PNG, not overwritten
// with differently encoded bytes under their original extension.
func NewCanvasObjectEditingFiles(receiptRoot string, protectedRoots []string, kind string) (*ObjectEditingFiles, error) {
	if kind != "canvas" && kind != "png" {
		return nil, objectediting.ErrInvalidInput
	}
	files, err := NewObjectEditingFiles(receiptRoot, protectedRoots)
	if err != nil {
		return nil, err
	}
	files.officeKind = kind
	return files, nil
}

func (s *ObjectEditingFiles) validateNativeObject(raw []byte) error {
	switch s.officeKind {
	case "canvas":
		_, err := canvasdomain.ParseScene(raw)
		return err
	case "png":
		if !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
			return objectediting.ErrInvalidInput
		}
		_, err := canvasdomain.InspectImage(raw)
		return err
	default:
		return InspectOfficePackage(raw, s.officeKind)
	}
}
