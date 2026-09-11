//go:build darwin || linux

package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestSemanticStartupOperationsNeverEscapeSwappedManagedAncestor(t *testing.T) {
	for _, test := range []struct {
		name string
		kind string
		path string
	}{
		{name: "create directory", kind: domainstartup.SemanticOperationCreateDirectory, path: "data/private/created"},
		{name: "install file", kind: domainstartup.SemanticOperationInstallFile, path: "data/private/fixture.bin"},
		{name: "set mode", kind: domainstartup.SemanticOperationSetMode, path: "data/private/mode.bin"},
		{name: "remove file", kind: domainstartup.SemanticOperationRemoveFile, path: "data/private/remove.bin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			private := filepath.Join(roots.DataDir, "private")
			if err := os.MkdirAll(private, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{"fixture.bin": "before", "mode.bin": "mode", "remove.bin": "remove"} {
				if err := os.WriteFile(filepath.Join(private, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			baseline, configDigest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			prepared, err := builder.Prepare(context.Background(), baseline, configDigest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				stagePrivate := filepath.Join(stage.DataDir, "private")
				if err := os.Mkdir(filepath.Join(stagePrivate, "created"), 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(stagePrivate, "fixture.bin"), []byte("after"), 0o600); err != nil {
					return err
				}
				if err := os.Chmod(filepath.Join(stagePrivate, "mode.bin"), 0o640); err != nil {
					return err
				}
				return os.Remove(filepath.Join(stagePrivate, "remove.bin"))
			})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			operationIndex := -1
			for index, operation := range prepared.Plan().Operations {
				if operation.Kind == test.kind && operation.Path == test.path {
					operationIndex = index
				}
			}
			if operationIndex < 0 {
				t.Fatalf("operation is missing: kind=%s path=%s operations=%#v", test.kind, test.path, prepared.Plan().Operations)
			}
			external := filepath.Join(t.TempDir(), "external")
			if err := os.Mkdir(external, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{"fixture.bin": "before", "mode.bin": "mode", "remove.bin": "remove"} {
				if err := os.WriteFile(filepath.Join(external, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			held := filepath.Join(roots.DataDir, "private-held")
			builder.fault = func(stage string, operation int) error {
				if stage != "before_operation" || operation != operationIndex {
					return nil
				}
				if err := os.Rename(private, held); err != nil {
					return err
				}
				return os.Symlink(external, private)
			}
			if err := prepared.Apply(context.Background()); err == nil {
				t.Fatal("swapped managed ancestor was accepted")
			}
			assertExternalSemanticFixtureUnchanged(t, external)
			if err := os.Remove(private); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(held, private); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertExternalSemanticFixtureUnchanged(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, "created")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("create-directory escaped managed root: %v", err)
	}
	for name, want := range map[string]string{"fixture.bin": "before", "mode.bin": "mode", "remove.bin": "remove"} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(body) != want {
			t.Fatalf("managed operation escaped into %s: body=%q err=%v", name, body, err)
		}
	}
	mode, err := os.Stat(filepath.Join(root, "mode.bin"))
	if err != nil || mode.Mode().Perm() != 0o600 {
		t.Fatalf("set-mode escaped managed root: mode=%v err=%v", mode, err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".*.startup-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("install temp escaped managed root: matches=%#v err=%v", matches, err)
	}
}
