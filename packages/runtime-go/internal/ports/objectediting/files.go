// Package objectediting defines host-owned text object persistence boundaries.
package objectediting

import (
	"context"
	"errors"
)

const MaxTextBytes = 1572864

const (
	StatusPending   = "pending"
	StatusCommitted = "committed"
	StatusConflict  = "conflict"
	StatusUnknown   = "unknown"
)

var (
	ErrInvalidInput      = errors.New("invalid text object request")
	ErrForbidden         = errors.New("text object path is not permitted")
	ErrNotText           = errors.New("text object is not supported text")
	ErrTooLarge          = errors.New("text object exceeds the size limit")
	ErrConflict          = errors.New("text object revision conflict")
	ErrOperationMismatch = errors.New("text object operation payload does not match")
	ErrOperationNotFound = errors.New("text object operation was not found")
	ErrPersistence       = errors.New("text object persistence could not be confirmed")
)

type Document struct {
	Workspace    string `json:"-"`
	IdentityPath string `json:"-"`
	Path         string
	Content      string
	Revision     string
	Encoding     string
}

type CommitInput struct {
	Workspace      string
	Path           string
	BaseRevision   string
	Content        string
	OperationID    string
	ObjectIdentity string
}

// Receipt describes an operation, not necessarily the file's current revision.
// SavedAt is a UTC RFC3339Nano confirmation time, including recovered commits.
type Receipt struct {
	OperationID string
	Revision    string
	Status      string
	SavedAt     string
}

type Files interface {
	Read(context.Context, string, string) (Document, error)
	Commit(context.Context, CommitInput) (Receipt, error)
	Status(context.Context, string, string, string, string) (Receipt, error)
}
