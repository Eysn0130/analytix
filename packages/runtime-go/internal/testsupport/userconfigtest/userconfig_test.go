package userconfigtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateChildEnvironmentOwnsIndependentCleanup(t *testing.T) {
	environment, cleanup, err := CreateChildEnvironment([]string{
		"HOME=/untrusted/shared-home",
		"XDG_CONFIG_HOME=/untrusted/shared-config",
		"ANALYTIX_GO_TEST_USER_CONFIG_ROOT=/untrusted/shared-root",
		"KEEP=value",
	})
	if err != nil {
		t.Fatalf("CreateChildEnvironment: %v", err)
	}
	values := make(map[string]string)
	counts := make(map[string]int)
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		values[name] = value
		counts[name]++
	}
	root := values[isolatedRootEnvironment]
	if root == "" || !pathWithinTemporaryRoot(root) {
		t.Fatalf("isolated root = %q", root)
	}
	for _, name := range []string{isolatedRootEnvironment, "HOME", "XDG_CONFIG_HOME"} {
		if counts[name] != 1 {
			t.Fatalf("%s count = %d, want 1", name, counts[name])
		}
	}
	if values["HOME"] != filepath.Join(root, "home") || values["XDG_CONFIG_HOME"] != filepath.Join(root, "config") {
		t.Fatalf("child paths are not bound to root %q: %#v", root, values)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GOTMPDIR"} {
		if values[name] != filepath.Join(root, "tmp") || counts[name] != 1 {
			t.Fatalf("%s is not uniquely bound to child temp root %q: %#v", name, root, values)
		}
	}
	if values["KEEP"] != "value" {
		t.Fatalf("unrelated environment changed: %#v", values)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("isolated root still exists after cleanup: %v", err)
	}
}
