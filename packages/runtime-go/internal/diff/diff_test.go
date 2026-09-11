package diff

import (
	"strings"
	"testing"
)

func TestBuildModifyUnifiedDiff(t *testing.T) {
	change := Build("notes.txt", "first\nold\nlast\n", "first\nnew\nlast\n", Modify)
	if change.Binary || change.Truncated {
		t.Fatalf("small text diff should be rendered: %#v", change)
	}
	if change.Added != 1 || change.Removed != 1 {
		t.Fatalf("unexpected tallies: %#v", change)
	}
	for _, expected := range []string{"--- a/notes.txt", "+++ b/notes.txt", "-old", "+new"} {
		if !strings.Contains(change.Diff, expected) {
			t.Fatalf("diff missing %q:\n%s", expected, change.Diff)
		}
	}
}

func TestBuildCreateUnifiedDiff(t *testing.T) {
	change := Build("new.txt", "", "alpha\nbeta\n", Create)
	if change.Kind != Create || change.Added != 2 || change.Removed != 0 {
		t.Fatalf("unexpected create diff: %#v", change)
	}
	if !strings.Contains(change.Diff, "+alpha") || !strings.Contains(change.Diff, "+beta") {
		t.Fatalf("create diff should include added lines:\n%s", change.Diff)
	}
}

func TestBuildBinaryOmitsDiff(t *testing.T) {
	change := Build("bin.dat", "old\x00", "new", Modify)
	if !change.Binary || change.Diff != "" || change.Added != 0 || change.Removed != 0 {
		t.Fatalf("binary diff should be omitted: %#v", change)
	}
}

func TestBuildLargeRewriteUsesBoundedSummary(t *testing.T) {
	oldLines := make([]string, maxDiffEdits+10)
	newLines := make([]string, maxDiffEdits+10)
	for i := range oldLines {
		oldLines[i] = "old-" + itoa(i)
		newLines[i] = "new-" + itoa(i)
	}
	change := Build("large.txt", strings.Join(oldLines, "\n")+"\n", strings.Join(newLines, "\n")+"\n", Modify)
	if !change.Truncated || !strings.Contains(change.Diff, "diff omitted") {
		t.Fatalf("large rewrite should use bounded summary: %#v", change)
	}
	if change.Added == 0 || change.Removed == 0 {
		t.Fatalf("large rewrite should keep tallies: %#v", change)
	}
}
