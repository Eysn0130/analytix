//go:build darwin || linux

package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeConfigSnapshotRejectsSymlinkWithoutBreakingInlinePrecedence(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.json")
	link := filepath.Join(directory, "runtime.json")
	if err := os.WriteFile(source, []byte(`{"runtime":{"source":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	if snapshot, err := LoadRuntimeConfigSnapshotV1("", link); err == nil || snapshot.Present() {
		t.Fatalf("symlinked runtime configuration was accepted: present=%v err=%v", snapshot.Present(), err)
	}
	snapshot, err := LoadRuntimeConfigSnapshotV1(`{"runtime":{"inline":true}}`, link)
	if err != nil || !snapshot.Present() {
		t.Fatalf("authoritative inline configuration consulted path: present=%v err=%v", snapshot.Present(), err)
	}
	document, ok, err := snapshot.Document()
	if err != nil || !ok || document["runtime"].(map[string]any)["inline"] != true {
		t.Fatalf("inline precedence changed: document=%#v ok=%v err=%v", document, ok, err)
	}
}
