// Package workspaceread defines bounded local-display retrieval input. It does
// not grant permission; the caller must validate Core authority at every effect.
package workspaceread

import (
	"context"
	"errors"
	"time"
)

const (
	MaxFiles = 160
	MaxEntries = 8000
	MaxTextBytes = 600000
	MaxSnapshotBytes = 16 << 20
)

var ErrUnavailable = errors.New("workspace retrieval unavailable")

type Root struct {
	Workspace string
	Identity string
	Policy string
}

type File struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Revision string `json:"revision"`
	Content []byte `json:"content"`
}

type Files interface {
	InspectRoot(context.Context, string) (Root, error)
	Scan(context.Context, Root, bool, time.Duration, func() error) ([]File, error)
}
