//go:build darwin || linux

package filestore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProcessProtectedRootsCanonicalizeExistingAndAbsentAliasDescendants(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	physical, alias := filepath.Join(parent, "physical"), filepath.Join(parent, "alias")
	if err := os.MkdirAll(filepath.Join(physical, "existing"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	configured := []string{
		MandatoryProtectedRoot(filepath.Join(alias, "existing")),
		MandatoryProtectedRoot(filepath.Join(alias, "not-yet-created", "private")),
	}
	want := []string{filepath.Join(physical, "existing"), filepath.Join(physical, "not-yet-created", "private")}
	for _, actual := range [][]string{
		EffectiveProcessProtectedRoots(configured, "workspace-write"),
		EffectiveProcessProtectedRoots(configured, "danger-full-access"),
		MandatoryProcessProtectedRoots(configured),
	} {
		if !reflect.DeepEqual(actual, want) {
			t.Fatal("process projection did not preserve canonical mandatory roots")
		}
	}
}
