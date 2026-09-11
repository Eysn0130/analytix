//go:build darwin

package finalauthority_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func TestLargeOpaqueDarwinAtomicallyClonesExactFileDescriptor(t *testing.T) {
	store, sourceRoot := largeOpaqueDarwinStore(t)
	body := []byte("darwin-atomic-clone-source")
	reference := largeOpaqueDarwinReference(body, body)
	sourcePath, source := largeOpaqueDarwinSource(t, sourceRoot, "source.duckdb", body, 0o400)

	outcome, err := store.PutExact(context.Background(), reference, source)
	if err != nil || outcome != largeopaqueport.PutOutcomeCreated {
		t.Fatalf("first exact clone outcome=%v err=%v", outcome, err)
	}
	if outcome, err = store.PutExact(context.Background(), reference, source); err != nil || outcome != largeopaqueport.PutOutcomeExistingEqual {
		t.Fatalf("equal exact clone outcome=%v err=%v", outcome, err)
	}
	var output bytes.Buffer
	if err := store.ReadExact(context.Background(), reference, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), body) {
		t.Fatal("Darwin exact clone changed source bytes")
	}

	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sourcePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, bytes.Repeat([]byte{'x'}, len(body)), 0o600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := store.ReadExact(context.Background(), reference, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), body) {
		t.Fatal("source mutation changed the committed copy-on-write object")
	}
}

func TestLargeOpaqueDarwinClonesUnlinkedExactFileDescriptor(t *testing.T) {
	store, sourceRoot := largeOpaqueDarwinStore(t)
	body := []byte("darwin-unlinked-atomic-clone-source")
	reference := largeOpaqueDarwinReference(body, body)
	sourcePath, source := largeOpaqueDarwinSource(t, sourceRoot, "unlinked.duckdb", body, 0o400)
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if outcome, err := store.PutExact(context.Background(), reference, source); err != nil || outcome != largeopaqueport.PutOutcomeCreated {
		t.Fatalf("unlinked Darwin clone outcome=%v err=%v", outcome, err)
	}
	var output bytes.Buffer
	if err := store.ReadExact(context.Background(), reference, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), body) {
		t.Fatal("unlinked Darwin exact clone changed source bytes")
	}
}

func TestLargeOpaqueDarwinRejectsUnsafeOrMismatchedSourceBeforeClone(t *testing.T) {
	store, sourceRoot := largeOpaqueDarwinStore(t)
	body := []byte("darwin-source-body")
	reference := largeOpaqueDarwinReference(body, bytes.Repeat([]byte{'z'}, len(body)))
	_, source := largeOpaqueDarwinSource(t, sourceRoot, "mismatched.duckdb", body, 0o400)
	if _, err := store.PutExact(context.Background(), reference, source); !errors.Is(err, finalauthority.ErrLargeOpaqueSourceMismatch) {
		t.Fatalf("mismatched Darwin source error=%v", err)
	}
	if err := store.ReadExact(context.Background(), reference, &bytes.Buffer{}); err == nil {
		t.Fatal("mismatched Darwin source created a destination")
	}

	writableBody := []byte("darwin-writable-source")
	writableReference := largeOpaqueDarwinReference(writableBody, writableBody)
	_, writable := largeOpaqueDarwinSource(t, sourceRoot, "writable.duckdb", writableBody, 0o600)
	if _, err := store.PutExact(context.Background(), writableReference, writable); !errors.Is(err, finalauthority.ErrLargeOpaqueSourceMismatch) {
		t.Fatalf("writable Darwin source error=%v", err)
	}
	if err := store.ReadExact(context.Background(), writableReference, &bytes.Buffer{}); err == nil {
		t.Fatal("writable Darwin source created a destination")
	}
}

func TestLargeOpaqueDarwinNeverOverwritesConflictingAddress(t *testing.T) {
	store, sourceRoot := largeOpaqueDarwinStore(t)
	firstBody := []byte("darwin-first-exact-body")
	secondBody := []byte("darwin-other-exact-body")
	addressBody := []byte("darwin-logical-address")
	firstReference := largeOpaqueDarwinReference(addressBody, firstBody)
	secondReference := largeOpaqueDarwinReference(addressBody, secondBody)
	_, first := largeOpaqueDarwinSource(t, sourceRoot, "first.duckdb", firstBody, 0o400)
	_, second := largeOpaqueDarwinSource(t, sourceRoot, "second.duckdb", secondBody, 0o400)

	if outcome, err := store.PutExact(context.Background(), firstReference, first); err != nil || outcome != largeopaqueport.PutOutcomeCreated {
		t.Fatalf("first Darwin clone outcome=%v err=%v", outcome, err)
	}
	if _, err := store.PutExact(context.Background(), secondReference, second); !errors.Is(err, finalauthority.ErrLargeOpaqueIntegrity) {
		t.Fatalf("conflicting Darwin clone error=%v", err)
	}
	var output bytes.Buffer
	if err := store.ReadExact(context.Background(), firstReference, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), firstBody) {
		t.Fatal("conflicting Darwin clone overwrote the existing object")
	}
}

func TestLargeOpaqueDarwinArbitraryStreamFailsBeforeFilesystemAuthority(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "data", "darwin-stream-disabled")
	authority := &largeOpaqueDarwinAccessWitness{}
	store, err := finalauthority.NewLargeOpaqueStore(root, 4096, authority)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("darwin-stream-must-not-mutate")
	reference := largeOpaqueDarwinReference(body, body)
	sourcePath := filepath.Join(base, "wrapped-source.duckdb")
	if err := os.WriteFile(sourcePath, body, 0o400); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	for name, reader := range map[string]io.Reader{
		"memory stream": bytes.NewReader(body),
		"wrapped file":  struct{ io.Reader }{Reader: source},
	} {
		if _, err := store.PutExact(context.Background(), reference, reader); !errors.Is(err, finalauthority.ErrLargeOpaqueUnsupportedPlatform) {
			t.Fatalf("%s error=%v", name, err)
		}
	}
	if authority.writeCalls != 0 || authority.existingCalls != 0 {
		t.Fatalf("unsupported Darwin stream entered authority callbacks: write=%d existing=%d", authority.writeCalls, authority.existingCalls)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported Darwin stream mutated its root: %v", err)
	}
}

func largeOpaqueDarwinStore(t *testing.T) (*finalauthority.LargeOpaqueStore, string) {
	t.Helper()
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
	root := filepath.Join(frozen.DataDir, "large-opaque-darwin")
	for index := 0; index < 256; index++ {
		if err := os.MkdirAll(filepath.Join(root, hex.EncodeToString([]byte{byte(index)})), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := finalauthority.NewLargeOpaqueStore(root, 4096, lease)
	if err != nil {
		t.Fatal(err)
	}
	return store, frozen.DataDir
}

func largeOpaqueDarwinSource(
	t *testing.T,
	root string,
	name string,
	body []byte,
	mode os.FileMode,
) (string, *os.File) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return path, file
}

func largeOpaqueDarwinReference(addressBody []byte, body []byte) largeopaqueport.ExactRef {
	address := sha256.Sum256(addressBody)
	digest := sha256.Sum256(body)
	return largeopaqueport.ExactRef{
		Address:    hex.EncodeToString(address[:]),
		SHA256:     hex.EncodeToString(digest[:]),
		ByteLength: uint64(len(body)),
	}
}

type largeOpaqueDarwinAccessWitness struct {
	writeCalls    int
	existingCalls int
}

func (witness *largeOpaqueDarwinAccessWitness) WithPrivateCASAccess(
	context.Context,
	string,
	func(privatecasport.RootBinding) error,
) error {
	witness.writeCalls++
	return errors.New("unsupported Darwin stream entered write authority")
}

func (witness *largeOpaqueDarwinAccessWitness) WithExistingPrivateCASAccess(
	context.Context,
	string,
	func(privatecasport.RootBinding) error,
) error {
	witness.existingCalls++
	return errors.New("unsupported Darwin stream entered existing authority")
}
