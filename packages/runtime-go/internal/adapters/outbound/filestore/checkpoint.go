package filestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	contracts "analytix.local/runtime-go/internal/contracts"
)

type HashedUTF8Content struct {
	Hash    string
	Content string
}

type CheckpointTextContent struct {
	Hash     string
	Content  string
	Encoding string
	RawBytes []byte
}

func CheckpointRescueDir(dataDir, threadID, checkpointID string) string {
	return filepath.Join(dataDir, "checkpoint-rescues", contracts.SafeRecordID(threadID), contracts.SafeRecordID(checkpointID))
}

func CheckpointRescuePath(dataDir, threadID, checkpointID, rescueID string) string {
	return filepath.Join(CheckpointRescueDir(dataDir, threadID, checkpointID), contracts.SafeRecordID(rescueID)+".json")
}

// WriteCheckpointRescueRecord persists the only full rewind rescue copy in a
// private, crash-safe sidecar. Ordinary events and HTTP responses receive only
// BuildRescueSummary output.
func WriteCheckpointRescueRecord(dataDir, threadID, checkpointID string, record map[string]any) error {
	if strings.TrimSpace(dataDir) == "" || !privateCheckpointStorageID(threadID, "") ||
		!privateCheckpointStorageID(checkpointID, "axcp_") {
		return errors.New("checkpoint rescue storage authority is invalid")
	}
	summary, ok := checkpointapp.BuildRescueSummary(record, 0)
	if !ok || contracts.StringField(summary, "threadId") != threadID || contracts.StringField(summary, "checkpointId") != checkpointID {
		return errors.New("checkpoint rescue record identity is invalid")
	}
	rescueID := contracts.StringField(summary, "rescueId")
	if !privateCheckpointStorageID(rescueID, "axrr_") {
		return errors.New("checkpoint rescue id is invalid")
	}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return WritePrivateFileAtomic(CheckpointRescuePath(dataDir, threadID, checkpointID, rescueID), body)
}

func ReadCheckpointRescueRecord(dataDir, threadID, checkpointID, rescueID string) (map[string]any, error) {
	if strings.TrimSpace(dataDir) == "" || !privateCheckpointStorageID(threadID, "") ||
		!privateCheckpointStorageID(checkpointID, "axcp_") || !privateCheckpointStorageID(rescueID, "axrr_") {
		return nil, errors.New("checkpoint rescue storage authority is invalid")
	}
	return ReadJSONMapFile(CheckpointRescuePath(dataDir, threadID, checkpointID, rescueID))
}

func privateCheckpointStorageID(value, prefix string) bool {
	if value == "" || strings.TrimSpace(value) != value || (prefix != "" && (!strings.HasPrefix(value, prefix) || len(value) <= len(prefix))) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func SafeCheckpointApplyPath(workspace string, relativePath string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", errors.New("checkpoint workspace is empty")
	}
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "", errors.New("checkpoint relativePath is empty")
	}
	if filepath.IsAbs(relativePath) {
		return "", errors.New("checkpoint relativePath must not be absolute")
	}
	cleanRelative := filepath.Clean(relativePath)
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", errors.New("checkpoint relativePath escapes workspace")
	}
	root := filepath.Clean(workspace)
	absolutePath := filepath.Join(root, cleanRelative)
	rel, err := filepath.Rel(root, absolutePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("checkpoint path escapes workspace")
	}
	return absolutePath, nil
}

func CheckpointPathSafeForMutation(workspace string, relativePath string, absolutePath string) error {
	root := filepath.Clean(workspace)
	cleanRelative := filepath.Clean(relativePath)
	dir := filepath.Dir(cleanRelative)
	if dir != "." {
		current := root
		for _, part := range strings.Split(dir, string(filepath.Separator)) {
			if part == "" || part == "." {
				continue
			}
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("checkpoint apply path contains symlink ancestor: %s", part)
			}
		}
	}
	info, err := os.Lstat(absolutePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("current workspace path is a symlink")
	}
	if info.IsDir() {
		return errors.New("current workspace path is a directory")
	}
	return nil
}

func ReadHashedUTF8File(path string, maxBytes int, hash func(string) string) (HashedUTF8Content, error) {
	state, err := inspectAtomicTextTarget(path, false)
	if err != nil {
		return HashedUTF8Content{}, err
	}
	if !state.Exists {
		return HashedUTF8Content{}, os.ErrNotExist
	}
	if len(state.Content) > maxBytes {
		return HashedUTF8Content{}, fmt.Errorf("checkpoint file exceeds %d byte automatic checkpoint apply limit", maxBytes)
	}
	if !utf8.Valid(state.Content) {
		return HashedUTF8Content{}, errors.New("checkpoint file is not valid UTF-8")
	}
	content := string(state.Content)
	return HashedUTF8Content{Hash: hash(content), Content: content}, nil
}

// ReadCheckpointTextFile captures the exact bytes that the atomic mutation
// primitive compares and writes. The decoded text and encoding are metadata;
// Hash is always the SHA-256 of RawBytes, never a hash of a transcoded string.
func ReadCheckpointTextFile(path string, maxBytes int) (CheckpointTextContent, error) {
	state, err := inspectAtomicTextTarget(path, false)
	if err != nil {
		return CheckpointTextContent{}, err
	}
	if !state.Exists {
		return CheckpointTextContent{}, os.ErrNotExist
	}
	if len(state.Content) > maxBytes {
		return CheckpointTextContent{}, fmt.Errorf("checkpoint file exceeds %d byte automatic checkpoint apply limit", maxBytes)
	}
	content, encoding, ok := filetoolsapp.DecodeTextBytes(state.Content)
	if !ok {
		return CheckpointTextContent{}, errors.New("checkpoint file encoding is not supported")
	}
	return CheckpointTextContent{
		Hash: checkpointapp.HashBytes(state.Content), Content: content, Encoding: encoding,
		RawBytes: append([]byte(nil), state.Content...),
	}, nil
}

func ApplyCheckpointFileMutation(file checkpointapp.ApplyFilePreflight) error {
	return ApplyCheckpointFileMutationWithAuthority(file, ConditionalMutationAuthority{})
}

// ApplyCheckpointFileMutationWithAuthority applies an exact rewind mutation.
// Deletes require a host-private, same-filesystem journal/quarantine root;
// restoring bytes continues to use the no-follow atomic replacement primitive.
func ApplyCheckpointFileMutationWithAuthority(file checkpointapp.ApplyFilePreflight, mutationAuthority ConditionalMutationAuthority) error {
	targetBytes := file.TargetBytes
	if targetBytes == nil && file.TargetContent != "" {
		targetBytes = []byte(file.TargetContent)
	}
	request := atomicTextReplaceRequest{
		Path: file.AbsolutePath, ExpectedHash: file.CurrentHash,
		PreserveMode: true, DefaultMode: 0o644, MutationAuthority: mutationAuthority,
	}
	switch file.Action {
	case "delete_created_file":
		if file.Status != "apply" || file.CurrentHash == "" || file.CurrentHash != file.AfterHash {
			return errors.New("checkpoint delete lacks exact current hash authority")
		}
		request.ExpectedExists = true
		request.Delete = true
	case "restore_previous_version":
		if file.Status != "apply" || file.CurrentHash == "" || file.CurrentHash != file.AfterHash || checkpointapp.HashBytes(targetBytes) != file.BeforeHash {
			return errors.New("checkpoint previous-version restore lacks exact before/after hash authority")
		}
		request.ExpectedExists = true
		request.Content = append([]byte(nil), targetBytes...)
	case "restore_deleted_file":
		if file.Status != "apply" || file.CurrentHash != "" || checkpointapp.HashBytes(targetBytes) != file.BeforeHash {
			return errors.New("checkpoint deleted-file restore lacks exact absence/before hash authority")
		}
		request.ExpectedExists = false
		request.ExpectedHash = ""
		request.Content = append([]byte(nil), targetBytes...)
		request.CreateParents = true
	default:
		return errors.New("checkpoint file mutation action is unsupported")
	}
	return atomicReplaceText(request)
}

func CheckpointApplyRouteMismatch(threadID, checkpointID, workspace string, plan map[string]any) string {
	if plan == nil {
		return "checkpoint rewind apply requires a plan"
	}
	if value := strings.TrimSpace(contracts.StringField(plan, "threadId")); value != "" && value != threadID {
		return "rewind plan thread mismatch: " + value
	}
	if value := strings.TrimSpace(contracts.StringField(plan, "checkpointId")); value != "" && value != checkpointID {
		return "rewind plan checkpoint mismatch: " + value
	}
	if value := strings.TrimSpace(contracts.StringField(plan, "workspace")); value != "" {
		planWorkspace, planErr := WorkspaceRealPath(value)
		routeWorkspace, routeErr := WorkspaceRealPath(workspace)
		if planErr != nil || routeErr != nil || filepath.Clean(planWorkspace) != filepath.Clean(routeWorkspace) {
			return "rewind plan workspace mismatch: " + value
		}
	}
	if mode := strings.TrimSpace(contracts.StringField(plan, "applyMode")); mode != "" && mode != "plan_only" {
		return "rewind apply must be based on a plan_only CheckpointRewindPlan"
	}
	if destructive, ok := plan["destructive"]; ok && checkpointapp.BoolValue(destructive) {
		return "rewind apply plan must be non-destructive"
	}
	return ""
}

func BuildCheckpointRestoreFilePlan(workspace string, allowWriteRoots []string, changed map[string]any) map[string]any {
	relativePath := strings.TrimSpace(contracts.StringField(changed, "relativePath"))
	pathError := ""
	authorityRoot, absolutePath, err := resolveCheckpointRecordPath(workspace, allowWriteRoots, changed)
	if err != nil {
		pathError = err.Error()
	} else if err := CheckpointPathSafeForMutation(authorityRoot, relativePath, absolutePath); err != nil {
		pathError = err.Error()
	}
	return checkpointapp.BuildRestoreFilePlan(checkpointapp.RestoreFilePlanInput{
		Changed: changed, PathError: pathError,
	})
}

func resolveCheckpointRecordPath(workspace string, allowWriteRoots []string, record map[string]any) (string, string, error) {
	relativePath := strings.TrimSpace(contracts.StringField(record, "relativePath"))
	version, versionOK := contracts.NumericSeq(record["pathAuthoritySchemaVersion"])
	if !versionOK || version == 0 {
		return "", "", errors.New("legacy checkpoint path authority requires manual review")
	}
	if version != checkpointPathAuthoritySchemaVersion {
		return "", "", errors.New("checkpoint path authority version is unsupported")
	}
	kind := strings.TrimSpace(contracts.StringField(record, "authorityKind"))
	rootHash := strings.TrimSpace(contracts.StringField(record, "authorityRootHash"))
	privateRootIdentity := strings.TrimSpace(contracts.StringField(record, "authorityRootIdentity"))
	root := ""
	rootIdentity := ""
	switch kind {
	case "workspace":
		root, _ = WorkspaceRealPath(workspace)
		if root != "" {
			rootIdentity, _ = checkpointRootIdentity(root)
		}
	case "allow_write":
		for _, candidate := range NormalizeRealRoots(allowWriteRoots) {
			candidateIdentity, err := checkpointRootIdentity(candidate)
			if err == nil && checkpointAuthorityRootHash(candidate, candidateIdentity) == rootHash {
				root = candidate
				rootIdentity = candidateIdentity
				break
			}
		}
	default:
		return "", "", errors.New("checkpoint path authority kind is invalid")
	}
	if root == "" || rootIdentity == "" || checkpointAuthorityRootHash(root, rootIdentity) != rootHash {
		return "", "", errors.New("checkpoint path authority is not currently host-authorized")
	}
	if privateRoot := contracts.StringField(record, "authorityRoot"); privateRoot != "" && privateRoot != root {
		return "", "", errors.New("checkpoint private root does not match its host authority")
	}
	if privateRootIdentity != "" && privateRootIdentity != rootIdentity {
		return "", "", errors.New("checkpoint private root identity does not match its host authority")
	}
	path, err := SafeCheckpointApplyPath(root, relativePath)
	return root, path, err
}

func CreateCheckpointRescueRecord(threadID, checkpointID, planID, workspace, createdAt string, files []checkpointapp.ApplyFilePreflight) map[string]any {
	rescueFiles := []checkpointapp.RescueFile{}
	for _, file := range files {
		entry := checkpointapp.RescueFile{
			RelativePath:               file.RelativePath,
			PathAuthoritySchemaVersion: file.PathAuthoritySchemaVersion,
			AuthorityKind:              file.AuthorityKind, AuthorityRoot: file.AuthorityRoot,
			AuthorityRootIdentity: file.AuthorityRootIdentity, AuthorityRootHash: file.AuthorityRootHash,
		}
		current, err := ReadCheckpointTextFile(file.AbsolutePath, checkpointapp.DefaultSnapshotMaxBytes)
		if errors.Is(err, os.ErrNotExist) {
			entry.Existed = false
		} else if err == nil {
			entry.Existed = true
			entry.Hash = current.Hash
			entry.Content = current.Content
			entry.Encoding = current.Encoding
			entry.RawBytes = append([]byte(nil), current.RawBytes...)
		} else {
			entry.Existed = false
			entry.Error = err.Error()
		}
		rescueFiles = append(rescueFiles, entry)
	}
	return checkpointapp.BuildRescueRecord(checkpointapp.RescueRecordInput{
		ThreadID:     threadID,
		CheckpointID: checkpointID,
		PlanID:       planID,
		Workspace:    workspace,
		CreatedAt:    createdAt,
		Files:        rescueFiles,
	})
}
