//go:build darwin || linux

package userconfigtest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsolationCanonicalizesTemporaryAncestorAlias(t *testing.T) {
	parent := t.TempDir()
	alias := filepath.Join(parent, "temporary-alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", alias)
	isolation, err := newIsolation()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := isolation.cleanup(); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{isolatedRootEnvironment, "HOME", "XDG_CONFIG_HOME", "TMPDIR", "GOTMPDIR"} {
		path := isolation.environment[name]
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || canonical != path {
			t.Errorf("%s retained an ancestor alias: resolve_error=%v", name, err)
		}
	}
}
