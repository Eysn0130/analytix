package nativecomponentregistry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrozenManifestV4AcceptsOnlyExactCanonicalRegistry(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "scripts", "native-components.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ParseFrozenManifestV4(body)
	if err != nil || len(manifest.Components) != 4 || manifest.Components[3].SourceRoot != "tools/data_engine" {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	for _, mutated := range [][]byte{
		append(append([]byte(nil), body...), '\n'),
		[]byte(`{"schemaVersion":4,"receiptSchemaVersion":5,"components":[],"unknown":true}`),
	} {
		if parsed, err := ParseFrozenManifestV4(mutated); err == nil || len(parsed.Components) != 0 {
			t.Fatalf("mutated manifest accepted: %#v", parsed)
		}
	}
}
