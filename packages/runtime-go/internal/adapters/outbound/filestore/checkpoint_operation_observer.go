package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

const checkpointPathAuthoritySchemaVersion = 1

type CheckpointOperationObserver struct {
	AllowWriteRoots   []string
	MutationAuthority ConditionalMutationAuthority
}

var _ checkpointfileport.Observer = CheckpointOperationObserver{}

func (observer CheckpointOperationObserver) CaptureBefore(_ context.Context, workspace, resolvedPath string) (checkpointfileport.BeforeState, error) {
	authority, err := observer.pathAuthority(workspace, resolvedPath)
	if err != nil {
		return checkpointfileport.BeforeState{}, err
	}
	if err := CheckpointPathSafeForMutation(authority.Root, authority.RelativePath, resolvedPath); err != nil {
		return checkpointfileport.BeforeState{}, err
	}
	before, err := ReadCheckpointTextFile(resolvedPath, checkpointapp.DefaultSnapshotMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return checkpointfileport.BeforeState{PathAuthority: authority}, nil
	}
	if err != nil {
		return checkpointfileport.BeforeState{}, err
	}
	return checkpointfileport.BeforeState{
		PathAuthority: authority, Existed: true, ContentAvailable: true,
		Hash: before.Hash, Encoding: before.Encoding, RawBytes: append([]byte(nil), before.RawBytes...),
	}, nil
}

func (observer CheckpointOperationObserver) Observe(
	_ context.Context,
	workspace string,
	authority checkpointfileport.PathAuthority,
	resolvedPath string,
) domaincheckpoint.ObservedOperationPathV2 {
	base := observedCheckpointPath(authority)
	if resolvedPath == "" || observer.validatePathAuthority(workspace, authority, resolvedPath) != nil ||
		CheckpointPathSafeForMutation(authority.Root, authority.RelativePath, resolvedPath) != nil {
		base.ObservationStatus = "unavailable"
		base.BlockerCode = "path_unsafe"
		return base
	}
	current, err := ReadCheckpointTextFile(resolvedPath, checkpointapp.DefaultSnapshotMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		base.ObservationStatus = "exact"
		return base
	}
	if err != nil {
		base.ObservationStatus = "unavailable"
		base.BlockerCode = checkpointObservationBlocker(err)
		return base
	}
	base.ObservationStatus = "exact"
	base.Existed = true
	base.Hash = current.Hash
	return base
}

func (observer CheckpointOperationObserver) ObserveRelative(
	ctx context.Context,
	workspace string,
	authority checkpointfileport.PathAuthority,
) domaincheckpoint.ObservedOperationPathV2 {
	resolvedPath, err := observer.resolvePathAuthority(workspace, authority)
	if err != nil {
		base := observedCheckpointPath(authority)
		base.ObservationStatus = "unavailable"
		base.BlockerCode = "path_unsafe"
		return base
	}
	return observer.Observe(ctx, workspace, authority, resolvedPath)
}

func observedCheckpointPath(authority checkpointfileport.PathAuthority) domaincheckpoint.ObservedOperationPathV2 {
	return domaincheckpoint.ObservedOperationPathV2{
		PathAuthoritySchemaVersion: authority.SchemaVersion,
		AuthorityKind:              authority.Kind, AuthorityRootHash: authority.RootHash,
		RelativePath: authority.RelativePath,
	}
}

func (observer CheckpointOperationObserver) pathAuthority(workspace, resolvedPath string) (checkpointfileport.PathAuthority, error) {
	workspaceRoot, err := WorkspaceRealPath(workspace)
	if err != nil || strings.TrimSpace(workspaceRoot) == "" {
		return checkpointfileport.PathAuthority{}, errors.New("checkpoint workspace authority is invalid")
	}
	if authority, ok := checkpointAuthorityWithinRoot("workspace", workspaceRoot, resolvedPath); ok {
		return authority, nil
	}
	roots := NormalizeRealRoots(observer.AllowWriteRoots)
	sort.SliceStable(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })
	for _, root := range roots {
		if authority, ok := checkpointAuthorityWithinRoot("allow_write", root, resolvedPath); ok {
			return authority, nil
		}
	}
	return checkpointfileport.PathAuthority{}, errors.New("checkpoint operation path is outside its host-authorized roots")
}

func checkpointAuthorityWithinRoot(kind, root, resolvedPath string) (checkpointfileport.PathAuthority, bool) {
	root, rootErr := WorkspaceRealPath(root)
	resolvedReal, resolvedErr := WorkspaceRealPath(resolvedPath)
	if rootErr != nil || resolvedErr != nil || !PathWithinRoot(root, resolvedReal) {
		return checkpointfileport.PathAuthority{}, false
	}
	relativePath, err := filepath.Rel(root, resolvedReal)
	if err != nil {
		return checkpointfileport.PathAuthority{}, false
	}
	relativePath = filepath.ToSlash(filepath.Clean(relativePath))
	if relativePath == "" || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, "../") {
		return checkpointfileport.PathAuthority{}, false
	}
	rootIdentity, err := checkpointRootIdentity(root)
	if err != nil || rootIdentity == "" {
		return checkpointfileport.PathAuthority{}, false
	}
	return checkpointfileport.PathAuthority{
		SchemaVersion: checkpointPathAuthoritySchemaVersion, Kind: kind,
		Root: root, RootIdentity: rootIdentity, RootHash: checkpointAuthorityRootHash(root, rootIdentity), RelativePath: relativePath,
	}, true
}

func (observer CheckpointOperationObserver) resolvePathAuthority(
	workspace string,
	authority checkpointfileport.PathAuthority,
) (string, error) {
	if err := observer.validatePathAuthority(workspace, authority, ""); err != nil {
		return "", err
	}
	return SafeCheckpointApplyPath(authority.Root, authority.RelativePath)
}

func (observer CheckpointOperationObserver) validatePathAuthority(
	workspace string,
	authority checkpointfileport.PathAuthority,
	resolvedPath string,
) error {
	if authority.SchemaVersion == 0 {
		return errors.New("legacy checkpoint path authority requires quarantine")
	}
	if authority.SchemaVersion != checkpointPathAuthoritySchemaVersion ||
		authority.RootIdentity == "" || checkpointAuthorityRootHash(authority.Root, authority.RootIdentity) != authority.RootHash {
		return errors.New("checkpoint path authority integrity is invalid")
	}
	expectedRoot := ""
	switch authority.Kind {
	case "workspace":
		expectedRoot, _ = WorkspaceRealPath(workspace)
	case "allow_write":
		for _, candidate := range NormalizeRealRoots(observer.AllowWriteRoots) {
			if candidate == authority.Root {
				expectedRoot = candidate
				break
			}
		}
	default:
		return errors.New("checkpoint path authority kind is invalid")
	}
	if expectedRoot == "" || expectedRoot != authority.Root {
		return errors.New("checkpoint path authority is no longer host-authorized")
	}
	currentIdentity, err := checkpointRootIdentity(expectedRoot)
	if err != nil || currentIdentity != authority.RootIdentity || checkpointAuthorityRootHash(expectedRoot, currentIdentity) != authority.RootHash {
		return errors.New("checkpoint path authority root identity changed")
	}
	expectedPath, err := SafeCheckpointApplyPath(authority.Root, authority.RelativePath)
	if err != nil {
		return err
	}
	if resolvedPath != "" {
		resolvedReal, realErr := WorkspaceRealPath(resolvedPath)
		if realErr != nil || filepath.Clean(expectedPath) != filepath.Clean(resolvedReal) {
			return errors.New("checkpoint resolved path does not match its frozen root binding")
		}
	}
	return nil
}

func checkpointAuthorityRootHash(root, identity string) string {
	return checkpointapp.HashBytes([]byte(root + "\x00" + identity))
}

func checkpointObservationBlocker(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "exceeds") {
		return "too_large"
	}
	if strings.Contains(message, "encoding") {
		return "unsupported_encoding"
	}
	return "read_failed"
}
