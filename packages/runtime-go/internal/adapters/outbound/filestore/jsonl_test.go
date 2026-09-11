package filestore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJSONLLinesHandlesFinalLineWithoutNewline(t *testing.T) {
	var lines []string
	err := ReadJSONLLines(strings.NewReader("{\"a\":1}\n{\"b\":2}"), func(lineNumber int, line string) error {
		lines = append(lines, line)
		if lineNumber != len(lines) {
			t.Fatalf("unexpected line number: got=%d want=%d", lineNumber, len(lines))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read jsonl lines: %v", err)
	}
	if len(lines) != 2 || lines[0] != "{\"a\":1}\n" || lines[1] != "{\"b\":2}" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestReadJSONLLinesStopsOnCallbackError(t *testing.T) {
	expected := errors.New("stop")
	err := ReadJSONLLines(strings.NewReader("one\ntwo\n"), func(lineNumber int, line string) error {
		if lineNumber == 2 {
			return expected
		}
		return nil
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestPreviewLineKeepsBoundedDiagnosticText(t *testing.T) {
	short := strings.Repeat("a", 120)
	if got := PreviewLine(short); got != short {
		t.Fatalf("short preview changed: %q", got)
	}
	long := strings.Repeat("b", 121)
	got := PreviewLine(long)
	if len(got) != 120 || got != strings.Repeat("b", 120) {
		t.Fatalf("unexpected preview: len=%d value=%q", len(got), got)
	}
}

type jsonlTestRecord struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

func TestJSONLFileRecordHelpers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records", "index.jsonl")
	if err := AppendJSONLRecord(path, jsonlTestRecord{ID: "one", Count: 1}); err != nil {
		t.Fatalf("append one: %v", err)
	}
	if err := AppendJSONLRecord(path, jsonlTestRecord{ID: "two", Count: 2}); err != nil {
		t.Fatalf("append two: %v", err)
	}
	records, err := ReadJSONLFileRecords[jsonlTestRecord](path, func(record jsonlTestRecord) bool {
		return record.Count > 1
	})
	if err != nil {
		t.Fatalf("read records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "two" {
		t.Fatalf("records = %#v", records)
	}
	if err := WriteJSONLFileAtomic(path, ".records-*.tmp", []jsonlTestRecord{{ID: "three", Count: 3}}); err != nil {
		t.Fatalf("write atomic: %v", err)
	}
	records, err = ReadJSONLFileRecords[jsonlTestRecord](path, nil)
	if err != nil {
		t.Fatalf("read atomic records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "three" {
		t.Fatalf("atomic records = %#v", records)
	}
}
