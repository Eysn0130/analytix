package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
)

type checkpointGitProbeStub struct {
	staged bool
	err    error
}

func (probe checkpointGitProbeStub) PathHasStagedChanges(string, string) (bool, error) {
	return probe.staged, probe.err
}

func TestCheckpointGitProbeFailureBlocksMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		probe  CheckpointGitStatusProbe
		status string
		reason string
	}{
		{"clean", checkpointGitProbeStub{}, "apply", "current file matches checkpoint afterHash and can restore before snapshot"},
		{"staged", checkpointGitProbeStub{staged: true}, "blocked", "current workspace path has staged git changes"},
		{"unavailable", checkpointGitProbeStub{err: errors.New("private-path-and-command-output")}, "blocked", "current workspace git status is unavailable"},
		{"missing", nil, "blocked", "current workspace git status is unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "notes.txt")
			before, after := []byte("before\n"), []byte("after\n")
			if err := os.WriteFile(path, after, 0o600); err != nil {
				t.Fatal(err)
			}
			root, err := WorkspaceRealPath(workspace)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := checkpointRootIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			rootHash := checkpointAuthorityRootHash(root, identity)
			plan := map[string]any{
				"relativePath": "notes.txt", "action": "restore_previous_version", "status": "ready",
				"beforeHash": checkpointapp.HashBytes(before), "afterHash": checkpointapp.HashBytes(after),
				"pathAuthoritySchemaVersion": float64(1), "authorityKind": "workspace", "authorityRootHash": rootHash,
			}
			snapshot := map[string]any{
				"relativePath": "notes.txt", "pathAuthoritySchemaVersion": float64(1), "authorityKind": "workspace",
				"authorityRoot": root, "authorityRootIdentity": identity, "authorityRootHash": rootHash,
				"before": map[string]any{"hash": checkpointapp.HashBytes(before), "content": string(before), "encoding": "utf8"},
			}
			result := PreflightCheckpointApplyFile(workspace, nil, plan, snapshot, test.probe)
			if result.Status != test.status || result.Reason != test.reason {
				t.Fatalf("checkpoint Git preflight status=%s reason=%s", result.Status, result.Reason)
			}
			if test.status == "apply" {
				if err := ApplyCheckpointFileMutation(result); err != nil {
					t.Fatal(err)
				}
				after = before
			} else if err := ApplyCheckpointFileMutation(result); err == nil {
				t.Fatal("blocked Git preflight authorized a file mutation")
			}
			current, err := os.ReadFile(path)
			if err != nil || string(current) != string(after) {
				t.Fatal("checkpoint preflight or permitted mutation changed unexpected file bytes")
			}
		})
	}
}
