package skill

import (
	"bytes"
	"strings"
	"testing"
)

func TestSkillPackageSnapshotDigestBindsEveryConsumedByte(t *testing.T) {
	left, err := NewPackageSnapshot("SKILL.md", []FileInput{
		{RelativePath: "SKILL.md", Bytes: []byte("body")},
		{RelativePath: "references/a.md", Bytes: []byte("reference")},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewPackageSnapshot("SKILL.md", []FileInput{
		{RelativePath: "references/a.md", Bytes: []byte("reference")},
		{RelativePath: "SKILL.md", Bytes: []byte("body")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if left.Digest() != right.Digest() {
		t.Fatal("snapshot digest must be independent of discovery order")
	}
	changed, err := NewPackageSnapshot("SKILL.md", []FileInput{
		{RelativePath: "SKILL.md", Bytes: []byte("body")},
		{RelativePath: "references/a.md", Bytes: []byte("changed")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Digest() == left.Digest() {
		t.Fatal("changing a consumed reference byte must change the package digest")
	}
	entry, ok := left.File("SKILL.md")
	if !ok {
		t.Fatal("snapshot entry missing")
	}
	entry[0] = 'X'
	again, _ := left.File("SKILL.md")
	if !bytes.Equal(again, []byte("body")) {
		t.Fatal("snapshot file bytes must be immutable through callers")
	}
}

func TestPackageSnapshotRejectsTraversalAndMissingEntry(t *testing.T) {
	for _, candidate := range []string{"../outside.md", "a/../../outside.md", "/tmp/outside.md", `a\\outside.md`, "./SKILL.md"} {
		_, err := NewPackageSnapshot(candidate, []FileInput{{RelativePath: "SKILL.md", Bytes: []byte("body")}})
		if err == nil {
			t.Fatalf("expected invalid entry %q to be rejected", candidate)
		}
	}
	_, err := NewPackageSnapshot("PLAYBOOK.md", []FileInput{{RelativePath: "SKILL.md", Bytes: []byte("body")}})
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("expected missing entry rejection, got %v", err)
	}
}
