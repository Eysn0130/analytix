package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeConfigDocumentReadsInlineAndPath(t *testing.T) {
	document, ok, err := RuntimeConfigDocument(`{"runtime":{"enabled":true}}`, "")
	if err != nil || !ok {
		t.Fatalf("inline document should load: ok=%v err=%v", ok, err)
	}
	runtimeConfig, _ := document["runtime"].(map[string]any)
	if runtimeConfig["enabled"] != true {
		t.Fatalf("inline document mismatch: %#v", document)
	}

	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, []byte(`{"skills":{"enabled":true}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	document, ok, err = RuntimeConfigDocument("", path)
	if err != nil || !ok {
		t.Fatalf("path document should load: ok=%v err=%v", ok, err)
	}
	skills, _ := document["skills"].(map[string]any)
	if skills["enabled"] != true {
		t.Fatalf("path document mismatch: %#v", document)
	}
}

func TestRuntimeConfigDocumentToleratesUTF8BOM(t *testing.T) {
	document, ok, err := RuntimeConfigDocument("\ufeff{\"runtime\":{\"enabled\":true}}", "")
	if err != nil || !ok {
		t.Fatalf("inline BOM document should load: ok=%v err=%v", ok, err)
	}
	runtimeConfig, _ := document["runtime"].(map[string]any)
	if runtimeConfig["enabled"] != true {
		t.Fatalf("inline BOM document mismatch: %#v", document)
	}

	path := filepath.Join(t.TempDir(), "runtime-bom.json")
	if err := os.WriteFile(path, []byte{0xEF, 0xBB, 0xBF, '{', '"', 's', 'k', 'i', 'l', 'l', 's', '"', ':', '{', '"', 'e', 'n', 'a', 'b', 'l', 'e', 'd', '"', ':', 't', 'r', 'u', 'e', '}', '}'}, 0o600); err != nil {
		t.Fatalf("write BOM config: %v", err)
	}
	document, ok, err = RuntimeConfigDocument("", path)
	if err != nil || !ok {
		t.Fatalf("path BOM document should load: ok=%v err=%v", ok, err)
	}
	skills, _ := document["skills"].(map[string]any)
	if skills["enabled"] != true {
		t.Fatalf("path BOM document mismatch: %#v", document)
	}
}

func TestRuntimeConfigDocumentMissingPathIsAbsent(t *testing.T) {
	document, ok, err := RuntimeConfigDocument("", filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || ok || document != nil {
		t.Fatalf("missing config should be absent: document=%#v ok=%v err=%v", document, ok, err)
	}
}

func TestRuntimeConfigSnapshotIsImmutableAfterSourceReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, []byte(`{"servers":{"old":{"command":"old"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadRuntimeConfigSnapshotV1("", path)
	if err != nil || !snapshot.Present() {
		t.Fatalf("load snapshot: present=%v err=%v", snapshot.Present(), err)
	}
	firstDigest := snapshot.Digest()
	if err := os.WriteFile(path, []byte(`{"servers":{"new":{"command":"new"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	document, ok, err := snapshot.Document()
	if err != nil || !ok {
		t.Fatalf("decode frozen snapshot: ok=%v err=%v", ok, err)
	}
	servers, _ := document["servers"].(map[string]any)
	if _, old := servers["old"]; !old || servers["new"] != nil || snapshot.Digest() != firstDigest {
		t.Fatalf("snapshot followed source replacement: document=%#v", document)
	}
	bytes := snapshot.Bytes()
	bytes[0] = '['
	second, _, err := snapshot.Document()
	if err != nil || second["servers"] == nil {
		t.Fatalf("caller mutated snapshot bytes: document=%#v err=%v", second, err)
	}
}

func TestRuntimeConfigDocumentRejectsStrictBoundaryViolations(t *testing.T) {
	for _, body := range []string{
		`{"runtime":{},"runtime":{}}`,
		`{"outer":{"id":1,"id":2}}`,
		`{"items":[{"id":1,"id":2}]}`,
		`{"runtime":{}} {"runtime":{}}`,
		`null`,
		`[]`,
	} {
		if document, ok, err := RuntimeConfigDocument(body, ""); err == nil || ok || document != nil {
			t.Fatalf("invalid runtime configuration passed: body=%s document=%#v ok=%v err=%v", body, document, ok, err)
		}
	}
}
