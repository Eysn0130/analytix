package persistencefs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	security "analytix.local/runtime-go/internal/domain/security"
)

func TestClosedSemanticStageAuthorityInvalidatesExistingCASHandle(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "stage-data"), DurableDir: filepath.Join(base, "stage-durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	roots, err = persistencefs.ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	accessAuthority, err := persistencefs.NewSemanticStagePrivateCASAccessAuthority(roots, rootAuthority)
	if err != nil {
		t.Fatal(err)
	}
	store, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(roots.DataDir, "private", "authority"), 4096, accessAuthority,
	)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1}`)
	digest := security.SHA256Hex(body)
	if err := store.PutIfAbsent(context.Background(), digest, body); err != nil {
		t.Fatal(err)
	}
	if err := accessAuthority.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), digest, body); err == nil {
		t.Fatal("CAS handle retained write authority after its semantic stage closed")
	}
	if _, err := store.Read(context.Background(), digest); err == nil {
		t.Fatal("CAS handle retained read authority after its semantic stage closed")
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("CAS handle retained list authority after its semantic stage closed")
	}
}
