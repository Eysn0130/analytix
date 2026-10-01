package filestore

import (
	"encoding/base64"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

const MaxOfficeObjectBytes = 16 << 20

// Native Office editing reuses the existing principal-bound object authority.
// Base64 is only the protected-local transport representation; the Office codec
// validates every package before the existing CAS journal and atomic replacement.
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
	if s.validateNativeObject(raw) != nil {
		return "", "", objectediting.ErrNotText
	}
	return base64.StdEncoding.EncodeToString(raw), "office-base64", nil
}

func (s *ObjectEditingFiles) encodeObject(content, encoding string) ([]byte, error) {
	if s.officeKind == "" {
		return filetoolsapp.EncodeTextBytes(content, encoding), nil
	}
	if encoding != "office-base64" {
		return nil, objectediting.ErrInvalidInput
	}
	if len(content) > base64.StdEncoding.EncodedLen(MaxOfficeObjectBytes) {
		return nil, objectediting.ErrTooLarge
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(content)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != content {
		return nil, objectediting.ErrInvalidInput
	}
	if len(raw) > MaxOfficeObjectBytes {
		return nil, objectediting.ErrTooLarge
	}
	if s.validateNativeObject(raw) != nil {
		return nil, objectediting.ErrNotText
	}
	return raw, nil
}
