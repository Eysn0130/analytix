package persistencefs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectEditingReceiptsParticipateInManagedSnapshot(t *testing.T) {
	roots := testRootSet(t)
	before, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := snapshotEntry(before, "data/object-editing"); !ok || entry.Type != "absent" {
		t.Fatal("absent receipt storage was not bound")
	}
	root := filepath.Join(roots.DataDir, "object-editing")
	mustMkdirAll(t, root)
	path := filepath.Join(root, strings.Repeat("a", 64)+".json")
	mustWrite(t, path, `{"version":1,"status":"pending"}`)
	after, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if before.SHA256 == after.SHA256 {
		t.Fatal("receipt state missing from startup snapshot")
	}
	mustWrite(t, path, `{"version":1,"status":"pending","status":"committed"}`)
	_, err = CaptureStrict(roots)
	assertIntegrityCode(t, err, "invalid_json")
}
