package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func TestDataDirPrivateCASScopeBindsOneExactNamespace(t *testing.T) {
	base := t.TempDir()
	roots := RootSet{
		DataDir:    filepath.Join(base, "data"),
		DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	scope, err := lease.IssueDataDirPrivateCASScope("raw-artifact-chunks-v1")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := canonicalPathWithoutCreate(filepath.Join(roots.DataDir, "raw-artifact-chunks-v1"))
	if err != nil {
		t.Fatal(err)
	}
	expectedDataDir, err := canonicalPathWithoutCreate(roots.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if root, err := scope.Root(); err != nil || canonicalPathKey(root) != canonicalPathKey(expected) {
		t.Fatalf("scope root=%q err=%v", root, err)
	}

	called := false
	err = scope.WithExistingPrivateCASAccess(context.Background(), expected, func(binding privatecasport.RootBinding) error {
		called = true
		if canonicalPathKey(binding.RootPath) != canonicalPathKey(expectedDataDir) ||
			binding.RelativePath != "raw-artifact-chunks-v1" {
			t.Fatalf("scope binding=%+v", binding)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("exact DataDir scope access called=%v err=%v", called, err)
	}

	for _, rejected := range []string{
		filepath.Join(roots.DurableDir, "raw-artifact-chunks-v1"),
		filepath.Join(roots.DataDir, "other-owner"),
		filepath.Join(expected, "nested"),
	} {
		called = false
		err := scope.WithExistingPrivateCASAccess(context.Background(), rejected, func(privatecasport.RootBinding) error {
			called = true
			return nil
		})
		if err == nil || called {
			t.Fatalf("redirected root %q called=%v err=%v", rejected, called, err)
		}
	}
}

func TestDataDirPrivateCASScopeRejectsUnissuedOrDeadAuthority(t *testing.T) {
	var zero DataDirPrivateCASScope
	if _, err := zero.Root(); err == nil {
		t.Fatal("zero-value DataDir scope was accepted")
	}
	if _, err := (*CompositeLease)(nil).IssueDataDirPrivateCASScope("raw"); err == nil {
		t.Fatal("nil lease issued a DataDir scope")
	}

	base := t.TempDir()
	roots := RootSet{DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable")}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"", ".", "../escape", "nested/escape", "nested\\escape"} {
		if _, err := lease.IssueDataDirPrivateCASScope(namespace); err == nil {
			t.Fatalf("invalid namespace %q was accepted", namespace)
		}
	}
	scope, err := lease.IssueDataDirPrivateCASScope("raw")
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := scope.Root(); err == nil {
		t.Fatal("scope survived its owning lease")
	}
	called := false
	err = scope.WithExistingPrivateCASAccess(context.Background(), filepath.Join(roots.DataDir, "raw"), func(privatecasport.RootBinding) error {
		called = true
		return errors.New("must not run")
	})
	if err == nil || called {
		t.Fatalf("dead scope access called=%v err=%v", called, err)
	}
}
