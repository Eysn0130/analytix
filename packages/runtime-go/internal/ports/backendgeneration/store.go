package backendgeneration

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("backend generation record not found")
	ErrConflict = errors.New("backend generation record conflicts")
	ErrCorrupt  = errors.New("backend generation store is corrupt")
)

type StoredAllocationV1 struct {
	Digest string
	Body   []byte
}

type TransactionV1 interface {
	Visit(context.Context, func(StoredAllocationV1) error) error
	PutIfAbsent(context.Context, string, []byte) error
	Resolve(context.Context, string) ([]byte, error)
}

type StoreV1 interface {
	WithExclusive(context.Context, func(TransactionV1) error) error
}
