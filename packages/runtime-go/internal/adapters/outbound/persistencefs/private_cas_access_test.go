package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func TestCompositeLeasePrivateCASAccessScopeIsStrictAndLive(t *testing.T) {
	base := t.TempDir()
	roots := RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	roots, held := lease.FrozenRoots()
	if !held {
		t.Fatal("persistence lease did not expose frozen roots")
	}
	called := false
	if err := lease.WithPrivateCASAccess(
		context.Background(), filepath.Join(roots.DataDir, "private", "authority"),
		func(privatecasport.RootBinding) error { called = true; return nil },
	); err != nil || !called {
		t.Fatalf("leased descendant access failed: called=%v err=%v", called, err)
	}
	for _, root := range []string{roots.DataDir, filepath.Join(base, "outside")} {
		called = false
		err := lease.WithPrivateCASAccess(context.Background(), root, func(privatecasport.RootBinding) error {
			called = true
			return nil
		})
		if err == nil || called {
			t.Fatalf("out-of-scope CAS access ran: root=%s called=%v err=%v", root, called, err)
		}
	}
}

func TestCompositeLeaseExistingPrivateCASAccessBindsColdRootWithoutCreatingIt(t *testing.T) {
	base := t.TempDir()
	roots := RootSet{
		DataDir: filepath.Join(base, "cold-data"), DurableDir: filepath.Join(base, "cold-durable"),
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	frozen, ok := lease.FrozenRoots()
	if !ok {
		t.Fatal("cold-root lease did not expose frozen roots")
	}
	requested := filepath.Join(frozen.DataDir, "private", "authority")
	called := false
	if err := lease.WithExistingPrivateCASAccess(context.Background(), requested, func(binding privatecasport.RootBinding) error {
		called = true
		if filepath.Join(binding.RootPath, binding.RelativePath) != requested {
			t.Fatalf("cold-root binding mismatch: root=%q relative=%q", binding.RootPath, binding.RelativePath)
		}
		return nil
	}); err != nil || !called {
		t.Fatalf("cold-root existing access failed: called=%v err=%v", called, err)
	}
	if _, err := os.Lstat(frozen.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cold-root existing access created its root: %v", err)
	}
}

func TestCompositeLeaseExistingPrivateCASAccessRejectsColdRootAppearance(t *testing.T) {
	for _, test := range []struct {
		name   string
		appear func(string) error
	}{
		{
			name: "partial suffix",
			appear: func(root string) error {
				return os.Mkdir(filepath.Dir(root), 0o700)
			},
		},
		{
			name: "complete root",
			appear: func(root string) error {
				return os.MkdirAll(root, 0o700)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			roots := RootSet{
				DataDir: filepath.Join(base, "missing-parent", "cold-data"), DurableDir: t.TempDir(),
			}
			lease, err := AcquireCompositeLease(roots)
			if err != nil {
				t.Fatal(err)
			}
			frozen, ok := lease.FrozenRoots()
			if !ok {
				_ = lease.Close()
				t.Fatal("cold-root lease did not expose frozen roots")
			}
			if err := test.appear(frozen.DataDir); err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			called := false
			requested := filepath.Join(frozen.DataDir, "private", "authority")
			err = lease.WithExistingPrivateCASAccess(context.Background(), requested, func(privatecasport.RootBinding) error {
				called = true
				return nil
			})
			if err == nil || called {
				t.Fatalf("post-freeze cold-root appearance received existing access: called=%v err=%v", called, err)
			}
			if _, statErr := os.Lstat(requested); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("rejected cold-root appearance mutated requested authority: %v", statErr)
			}
			if err := os.RemoveAll(filepath.Join(base, "missing-parent")); err != nil {
				t.Fatal(err)
			}
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompositeLeaseCloseWaitsForPrivateCASAccessAndRejectsNewWork(t *testing.T) {
	base := t.TempDir()
	roots := RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	roots, held := lease.FrozenRoots()
	if !held {
		t.Fatal("persistence lease did not expose frozen roots")
	}
	root := filepath.Join(roots.DataDir, "private", "authority")
	entered := make(chan struct{})
	release := make(chan struct{})
	accessDone := make(chan error, 1)
	go func() {
		accessDone <- lease.WithPrivateCASAccess(context.Background(), root, func(privatecasport.RootBinding) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- lease.Close() }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, held := lease.FrozenRoots(); !held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lease never entered the closing state")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-closeDone:
		t.Fatalf("lease closed before the active CAS access drained: %v", err)
	default:
	}
	called := false
	if err := lease.WithPrivateCASAccess(context.Background(), root, func(privatecasport.RootBinding) error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatalf("closing lease admitted a new CAS access: called=%v err=%v", called, err)
	}
	close(release)
	if err := <-accessDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := lease.WithPrivateCASAccess(context.Background(), root, func(privatecasport.RootBinding) error {
		return errors.New("must not run")
	}); err == nil {
		t.Fatal("closed lease admitted a CAS access")
	}
}

func TestSemanticStagePrivateCASAccessAuthorityIsStageScopedAndClosable(t *testing.T) {
	base := t.TempDir()
	activeRoots := RootSet{
		DataDir: filepath.Join(base, "active-data"), DurableDir: filepath.Join(base, "active-durable"),
	}
	stageRoots := RootSet{
		DataDir: filepath.Join(base, "stage-data"), DurableDir: filepath.Join(base, "stage-durable"),
	}
	for _, root := range []string{stageRoots.DataDir, stageRoots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rootAuthority, err := FreezeRootAuthority(stageRoots)
	if err != nil {
		t.Fatal(err)
	}
	stageRoots, err = ResolveRootSet(stageRoots.DataDir, stageRoots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewSemanticStagePrivateCASAccessAuthority(stageRoots, rootAuthority)
	if err != nil {
		t.Fatal(err)
	}
	stageCASRoot := filepath.Join(stageRoots.DataDir, "private", "authority")
	called := false
	if err := authority.WithPrivateCASAccess(context.Background(), stageCASRoot, func(privatecasport.RootBinding) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("stage-scoped access failed: called=%v err=%v", called, err)
	}
	for _, root := range []string{
		stageRoots.DataDir,
		filepath.Join(activeRoots.DataDir, "private", "authority"),
		filepath.Join(base, "outside"),
	} {
		called = false
		err := authority.WithPrivateCASAccess(context.Background(), root, func(privatecasport.RootBinding) error {
			called = true
			return nil
		})
		if err == nil || called {
			t.Fatalf("out-of-stage access ran: root=%s called=%v err=%v", root, called, err)
		}
	}
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	called = false
	if err := authority.WithPrivateCASAccess(context.Background(), stageCASRoot, func(privatecasport.RootBinding) error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatalf("closed stage authority admitted access: called=%v err=%v", called, err)
	}
}

func TestSemanticStagePrivateCASAccessAuthorityValidatesAllRootsAtClose(t *testing.T) {
	base := t.TempDir()
	roots := RootSet{
		DataDir: filepath.Join(base, "stage-data"), DurableDir: filepath.Join(base, "stage-durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rootAuthority, err := FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	roots, err = ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewSemanticStagePrivateCASAccessAuthority(roots, rootAuthority)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.WithPrivateCASAccess(
		context.Background(), filepath.Join(roots.DataDir, "private", "authority"),
		func(privatecasport.RootBinding) error { return nil },
	); err != nil {
		t.Fatal(err)
	}
	moved := roots.DurableDir + ".moved"
	if err := os.Rename(roots.DurableDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(roots.DurableDir, 0o700); err != nil {
		_ = os.Rename(moved, roots.DurableDir)
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(roots.DurableDir)
		_ = os.Rename(moved, roots.DurableDir)
	}()
	if err := authority.Close(); err == nil {
		t.Fatal("semantic stage close accepted an unrelated root replacement")
	}
}

func TestCompositeLeaseRevalidatesBindingWhenAccessFails(t *testing.T) {
	base := t.TempDir()
	roots := RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	frozen, held := lease.FrozenRoots()
	if !held {
		_ = lease.Close()
		t.Fatal("persistence lease did not expose frozen roots")
	}
	moved := frozen.DataDir + ".moved"
	replacementInstalled := false
	defer func() {
		if replacementInstalled {
			_ = os.RemoveAll(frozen.DataDir)
			_ = os.Rename(moved, frozen.DataDir)
		}
		_ = lease.Close()
	}()
	sentinel := errors.New("access failed after an external cut")
	err = lease.WithPrivateCASAccess(
		context.Background(), filepath.Join(frozen.DataDir, "private", "authority"),
		func(privatecasport.RootBinding) error {
			if err := os.Rename(frozen.DataDir, moved); err != nil {
				return err
			}
			if err := os.Mkdir(frozen.DataDir, 0o700); err != nil {
				_ = os.Rename(moved, frozen.DataDir)
				return err
			}
			replacementInstalled = true
			return sentinel
		},
	)
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "persistence root changed during access") {
		t.Fatalf("failed callback skipped binding revalidation: %v", err)
	}
}
