//go:build linux

package finalauthority_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
)

func TestLargeOpaqueStoreNeverOverwritesConflictingAddress(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	frozen, ok := lease.FrozenRoots()
	if !ok {
		t.Fatal("persistence lease did not freeze roots")
	}
	store, err := finalauthority.NewLargeOpaqueStore(
		filepath.Join(frozen.DataDir, "large-opaque-conflict"), 4096, lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	firstBody := []byte("first-exact-body")
	secondBody := []byte("other-exact-body")
	address := exactSHA256([]byte("logical-address"))
	if err := os.MkdirAll(filepath.Join(frozen.DataDir, "large-opaque-conflict", address[:2]), 0o700); err != nil {
		t.Fatal(err)
	}
	firstRef := largeopaqueport.ExactRef{Address: address, SHA256: exactSHA256(firstBody), ByteLength: uint64(len(firstBody))}
	secondRef := largeopaqueport.ExactRef{Address: address, SHA256: exactSHA256(secondBody), ByteLength: uint64(len(secondBody))}

	if outcome, err := store.PutExact(context.Background(), firstRef, bytes.NewReader(firstBody)); err != nil || outcome != largeopaqueport.PutOutcomeCreated {
		t.Fatalf("first put outcome=%v err=%v", outcome, err)
	}
	if _, err := store.PutExact(context.Background(), secondRef, bytes.NewReader(secondBody)); !errors.Is(err, finalauthority.ErrLargeOpaqueIntegrity) {
		t.Fatalf("conflicting put error = %v", err)
	}
	var output bytes.Buffer
	if err := store.ReadExact(context.Background(), firstRef, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), firstBody) {
		t.Fatal("conflicting put overwrote the original body")
	}
}

func exactSHA256(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
