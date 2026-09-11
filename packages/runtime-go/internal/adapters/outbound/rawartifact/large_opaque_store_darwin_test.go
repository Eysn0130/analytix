//go:build darwin

package rawartifact_test

import (
	"context"
	"errors"
	"os"
	"testing"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
)

func TestLargeOpaqueChunkStoreDarwinPutFailsBeforeReadOrFilesystemMutation(t *testing.T) {
	store, root, _ := openLargeOpaqueChunkStore(t)
	body := []byte("Darwin writes require frozen direct-final provisioning")
	descriptor := buildLargeOpaqueDescriptor(t, "darwin-put-disabled", body)
	reader := &neverReadLargeOpaqueSource{}
	if _, err := store.PutExact(context.Background(), descriptor, reader); !errors.Is(err, largeopaqueport.ErrUnsupportedPlatform) ||
		!errors.Is(err, rawartifactport.ErrUnavailable) {
		t.Fatalf("disabled Darwin put error = %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("disabled Darwin put read the source %d times", reader.calls)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled Darwin put mutated its owner root: %v", err)
	}
}

type neverReadLargeOpaqueSource struct {
	calls int
}

func (reader *neverReadLargeOpaqueSource) Read([]byte) (int, error) {
	reader.calls++
	return 0, errors.New("disabled Darwin put reached its source")
}
