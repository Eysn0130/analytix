package filetools

import (
	"testing"

	runtimediff "analytix.local/runtime-go/internal/diff"
)

func TestMergeDiffOutputAddsStableDiffFields(t *testing.T) {
	output := map[string]any{}
	MergeDiffOutput(output, "notes.txt", "hello\n", "hello\nworld\n", runtimediff.Modify)
	if output["diff_kind"] != "modify" || output["added"] != float64(1) || output["removed"] != float64(0) {
		t.Fatalf("diff output fields = %#v", output)
	}
	if output["diff"] == "" {
		t.Fatalf("expected inline diff, got %#v", output)
	}
}
