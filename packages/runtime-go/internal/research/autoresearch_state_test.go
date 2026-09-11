package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoResearchStateAuditEventContainsOnlyClosedMetadata(t *testing.T) {
	snapshot := AutoResearchSnapshot{
		Descriptor: AutoResearchDescriptor{
			StateRelativePath: ".analytix/autoresearch/thread-1",
			TaskSpecPath:      ".analytix/autoresearch/thread-1/task_spec.md",
			ProgressPath:      ".analytix/autoresearch/thread-1/progress.json",
		},
		Progress: AutoResearchProgress{Requirements: []AutoResearchRequirement{
			{ID: "r1", Status: AutoResearchRequirementPending},
		}},
	}
	event := AutoResearchStateAuditEvent(snapshot, 1, "thread-1", "turn-1")
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"stateRelativePath", "taskSpecPath", "progressPath", "requiredFiles", ".analytix"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("public audit exposed operational path field %q: %s", forbidden, body)
		}
	}
	if event["fileCount"] != 5 || event["result"] != "pivot_required" {
		t.Fatalf("closed audit metadata mismatch: %#v", event)
	}
}

func TestAutoResearchStoreNeverCreatesAMissingWorkspaceRoot(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "missing-workspace")
	store := NewAutoResearchProjectStore(nil)
	if _, err := store.CreateOrResume(workspace, "thread-1", "objective", []string{"objective"}); err == nil ||
		!strings.Contains(err.Error(), "workspace is unavailable") {
		t.Fatalf("missing workspace did not fail closed: %v", err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("autoresearch created an unobserved workspace root, stat err=%v", err)
	}
}

func TestAutoResearchStoreUsesAnExistingResolvedWorkspace(t *testing.T) {
	workspace := t.TempDir()
	store := NewAutoResearchProjectStore(nil)
	snapshot, err := store.CreateOrResume(workspace, "thread-1", "objective", []string{"objective"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Descriptor.StateRelativePath != ".analytix/autoresearch/thread-1" {
		t.Fatalf("autoresearch descriptor mismatch: %#v", snapshot.Descriptor)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(snapshot.Descriptor.ProgressPath))); err != nil {
		t.Fatalf("autoresearch did not write inside the existing workspace: %v", err)
	}
}
