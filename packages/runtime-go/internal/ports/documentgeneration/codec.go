// Package documentgeneration defines the data-only document codec boundary.
// Codecs receive content already admitted by Core. They do not authorize paths,
// read workspace files, invoke Providers, or commit artifacts.
package documentgeneration

import (
	"context"
	"encoding/json"
	"errors"
)

const MaxDocumentBytes = 16 << 20

var ErrCodecUnavailable = errors.New("document codec unavailable")

type Image struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	DataBase64 string `json:"dataBase64"`
}

type Input struct {
	SchemaVersion int             `json:"schemaVersion"`
	Kind          string          `json:"kind"`
	Markdown      string          `json:"markdown,omitempty"`
	Title         string          `json:"title,omitempty"`
	Images        []Image         `json:"images,omitempty"`
	Workbook      json.RawMessage `json:"workbook,omitempty"`
	Presentation  json.RawMessage `json:"presentation,omitempty"`
}

type Codec interface {
	Encode(context.Context, Input) ([]byte, error)
}

// Supports preserves the original DOCX-only contract for injected legacy codecs.
func Supports(codec Codec, kind string) bool {
	if codec == nil {
		return false
	}
	if typed, ok := codec.(interface{ Supports(string) bool }); ok {
		return typed.Supports(kind)
	}
	return kind == "docx"
}
