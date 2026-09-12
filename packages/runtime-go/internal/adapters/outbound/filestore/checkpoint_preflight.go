package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	contracts "analytix.local/runtime-go/internal/contracts"
)

type CheckpointGitStatusProbe interface {
	PathHasStagedChanges(workspace string, relativePath string) (bool, error)
}

func PreflightCheckpointApplyFiles(
	workspace string,
	allowWriteRoots []string,
	plan map[string]any,
	snapshots map[string]map[string]any,
	gitStatus CheckpointGitStatusProbe,
) []checkpointapp.ApplyFilePreflight {
	items, _ := plan["files"].([]any)
	out := make([]checkpointapp.ApplyFilePreflight, 0, len(items))
	for _, item := range items {
		filePlan, _ := item.(map[string]any)
		if filePlan == nil {
			continue
		}
		snapshotKey := checkpointapp.SnapshotEvidenceKey(filePlan)
		out = append(out, PreflightCheckpointApplyFile(workspace, allowWriteRoots, filePlan, snapshots[snapshotKey], gitStatus))
	}
	return out
}

func PreflightCheckpointApplyFile(
	workspace string,
	allowWriteRoots []string,
	plan map[string]any,
	snapshot map[string]any,
	gitStatus CheckpointGitStatusProbe,
) checkpointapp.ApplyFilePreflight {
	relativePath := strings.TrimSpace(contracts.StringField(plan, "relativePath"))
	authorityRoot, absolutePath, err := resolveCheckpointRecordPath(workspace, allowWriteRoots, plan)
	if err != nil {
		return checkpointapp.BuildApplyFilePreflight(plan, snapshot, checkpointapp.ApplyFileState{
			AbsolutePath: filepath.Join(workspace, relativePath), PathError: err.Error(),
		})
	}
	snapshotRoot, snapshotPath, snapshotErr := resolveCheckpointRecordPath(workspace, allowWriteRoots, snapshot)
	if snapshotErr != nil || snapshotRoot != authorityRoot || filepath.Clean(snapshotPath) != filepath.Clean(absolutePath) {
		reason := "checkpoint snapshot path authority does not match the rewind plan"
		if snapshotErr != nil {
			reason = snapshotErr.Error()
		}
		return checkpointapp.BuildApplyFilePreflight(plan, snapshot, checkpointapp.ApplyFileState{
			AbsolutePath: absolutePath, PathError: reason,
		})
	}
	result := checkpointapp.BuildApplyFilePreflight(
		plan, snapshot, checkpointApplyFileState(authorityRoot, relativePath, absolutePath, gitStatus),
	)
	if version, ok := contracts.NumericSeq(plan["pathAuthoritySchemaVersion"]); ok && version == 1 {
		result.PathAuthoritySchemaVersion = version
		result.AuthorityKind = strings.TrimSpace(contracts.StringField(plan, "authorityKind"))
		result.AuthorityRoot = authorityRoot
		result.AuthorityRootIdentity = strings.TrimSpace(contracts.StringField(snapshot, "authorityRootIdentity"))
		result.AuthorityRootHash = strings.TrimSpace(contracts.StringField(plan, "authorityRootHash"))
	}
	return result
}

func checkpointApplyFileState(
	workspace string,
	relativePath string,
	absolutePath string,
	gitStatus CheckpointGitStatusProbe,
) checkpointapp.ApplyFileState {
	state := checkpointapp.ApplyFileState{AbsolutePath: absolutePath}
	if err := CheckpointPathSafeForMutation(workspace, relativePath, absolutePath); err != nil {
		state.MutationPathError = err.Error()
		return state
	}
	if gitStatus == nil {
		state.MutationPathError = "current workspace git status is unavailable"
		return state
	}
	staged, err := gitStatus.PathHasStagedChanges(workspace, relativePath)
	if err != nil {
		state.MutationPathError = "current workspace git status is unavailable"
		return state
	}
	if staged {
		state.HasStagedChanges = true
		return state
	}
	info, err := os.Lstat(absolutePath)
	if errors.Is(err, os.ErrNotExist) {
		return state
	}
	if err != nil {
		state.StatError = err.Error()
		return state
	}
	state.Exists = true
	if info.IsDir() {
		state.IsDir = true
		return state
	}
	current, err := ReadCheckpointTextFile(absolutePath, checkpointapp.DefaultSnapshotMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		state.Exists = false
		return state
	}
	if err != nil {
		state.ReadError = err.Error()
		return state
	}
	state.Current = checkpointapp.FileContent{
		Hash: current.Hash, Content: current.Content, Encoding: current.Encoding,
		RawBytes: append([]byte(nil), current.RawBytes...),
	}
	return state
}
