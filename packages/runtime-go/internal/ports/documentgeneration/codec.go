// Package documentgeneration defines the data-only document codec boundary.
// Codecs receive content already admitted by Core. They do not authorize paths,
// read workspace files, invoke Providers, or commit artifacts.
package documentgeneration

import (
	"context"
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
	SchemaVersion int     `json:"schemaVersion"`
	Kind          string  `json:"kind"`
	Markdown      string  `json:"markdown"`
	Title         string  `json:"title,omitempty"`
	Images        []Image `json:"images,omitempty"`
}

type Codec interface {
	Encode(context.Context, Input) ([]byte, error)
}
