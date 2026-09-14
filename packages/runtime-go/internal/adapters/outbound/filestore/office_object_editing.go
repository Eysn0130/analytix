package filestore

import (
	"encoding/base64"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

const MaxOfficeObjectBytes = 16 << 20

// Native Office previews reuse principal-bound object sessions for reads.
// Base64 is only the protected-local transport representation. Office writes
// are forbidden by the codec even when callers bypass the preview adapter.
func NewOfficeObjectEditingFiles(receiptRoot string, protectedRoots []string, kind string) (*ObjectEditingFiles, error) {
	if kind != "docx" && kind != "xlsx" && kind != "pptx" {
		return nil, objectediting.ErrInvalidInput
	}
	files, err := NewObjectEditingFiles(receiptRoot, protectedRoots)
	if err != nil {
		return nil, err
	}
	files.officeKind = kind
	return files, nil
}

func (s *ObjectEditingFiles) maxObjectBytes() int64 {
	if s.officeKind != "" {
		return MaxOfficeObjectBytes
	}
	return objectediting.MaxTextBytes
}

func (s *ObjectEditingFiles) maxContentBytes() int {
	if s.officeKind != "" {
		return base64.StdEncoding.EncodedLen(MaxOfficeObjectBytes)
	}
	return objectediting.MaxTextBytes
}

func (s *ObjectEditingFiles) inspectObject(path string) (atomicTextState, error) {
	return objectEditingInspectBounded(path, s.maxObjectBytes())
}

func (s *ObjectEditingFiles) decodeObject(raw []byte) (string, string, error) {
	if s.officeKind == "" {
		return objectEditingDecode(raw)
	}
	if len(raw) > MaxOfficeObjectBytes {
		return "", "", objectediting.ErrTooLarge
	}
	if InspectOfficePackage(raw, s.officeKind) != nil {
		return "", "", objectediting.ErrNotText
	}
	return base64.StdEncoding.EncodeToString(raw), "office-base64", nil
}

func (s *ObjectEditingFiles) encodeObject(content, encoding string) ([]byte, error) {
	if s.officeKind == "" {
		return filetoolsapp.EncodeTextBytes(content, encoding), nil
	}
	return nil, objectediting.ErrForbidden
}
