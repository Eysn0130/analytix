package objectediting

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImageNoteProjectionKeepsOnlyPermittedText(t *testing.T) {
	const note = "Allowed prefix alice@example.com suffix"
	start := strings.Index(note, "alice")
	parts, err := imageNoteParts(note, []ProtectedRange{{StartByte: start, EndByte: start + len("alice@example.com")}})
	if err != nil || len(parts) != 3 || parts[0].Text != "Allowed prefix " || parts[1].Kind != "protected" || parts[1].ProtectedRef == "" || parts[2].Text != " suffix" {
		t.Fatal("projection", parts, err)
	}
	raw, _ := json.Marshal(parts)
	if strings.Contains(string(raw), "alice@example.com") {
		t.Fatal("protected raw persisted in scope")
	}
	for _, ranges := range [][]ProtectedRange{{{StartByte: -1, EndByte: 2}}, {{StartByte: 3, EndByte: 2}}, {{StartByte: 0, EndByte: 1000}}, {{StartByte: 2, EndByte: 8}, {StartByte: 4, EndByte: 10}}} {
		if _, err := imageNoteParts(note, ranges); err == nil {
			t.Fatal("malformed projection accepted")
		}
	}
	if _, err := imageNoteParts("a😀b", []ProtectedRange{{StartByte: 2, EndByte: 3}}); err == nil {
		t.Fatal("partial utf8 span")
	}
	if _, err := imageNoteParts("protected protected", []ProtectedRange{{StartByte: 0, EndByte: 9}}); err == nil {
		t.Fatal("protected value repeated in literal")
	}
}
