package largeopaque

import (
	"context"
	"errors"
	"io"
)

var (
	ErrIntegrity           = errors.New("large opaque store integrity validation failed")
	ErrIndeterminate       = errors.New("large opaque store commit outcome is indeterminate")
	ErrSourceMismatch      = errors.New("large opaque store source bytes mismatched")
	ErrSourceRead          = errors.New("large opaque store source read failed")
	ErrDelivery            = errors.New("large opaque store downstream delivery failed")
	ErrUnsupportedPlatform = errors.New("large opaque store platform is unsupported")
)

// ExactRef is a caller-supplied immutable object address and its independently
// expected byte identity. Address is a logical SHA-256-shaped key; it need not
// equal SHA256(body).
type ExactRef struct {
	Address    string
	SHA256     string
	ByteLength uint64
}

type PutOutcome uint8

const (
	PutOutcomeCreated PutOutcome = iota + 1
	PutOutcomeExistingEqual
)

// Store is a non-enumerable streaming substrate. It deliberately exposes no
// List, Visit, Latest, Current, Active, path, or handle method. A successful
// storage operation is not evidence, acquisition, or snapshot authority.
type Store interface {
	PutExact(context.Context, ExactRef, io.Reader) (PutOutcome, error)
	ReadExact(context.Context, ExactRef, io.Writer) error
}
