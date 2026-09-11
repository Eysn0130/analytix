package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONMapFileRoundTripAndNilMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "record.json")
	if err := WriteJSONMapFile(path, map[string]any{"id": "one", "count": float64(2)}); err != nil {
		t.Fatalf("write json map: %v", err)
	}
	record, err := ReadJSONMapFile(path)
	if err != nil {
		t.Fatalf("read json map: %v", err)
	}
	if record["id"] != "one" || record["count"] != float64(2) {
		t.Fatalf("record = %#v", record)
	}

	nullPath := filepath.Join(t.TempDir(), "null.json")
	if err := os.WriteFile(nullPath, []byte("null"), 0o600); err != nil {
		t.Fatalf("write null: %v", err)
	}
	nullRecord, err := ReadJSONMapFile(nullPath)
	if err != nil {
		t.Fatalf("read null map: %v", err)
	}
	if nullRecord == nil || len(nullRecord) != 0 {
		t.Fatalf("null record = %#v", nullRecord)
	}
}
