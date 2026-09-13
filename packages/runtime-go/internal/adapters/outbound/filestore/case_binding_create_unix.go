//go:build darwin || linux

package filestore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"golang.org/x/sys/unix"
)

func createCaseBindingPlatform(ctx context.Context, workspacePath string, expected domainsecurity.CaseBindingObservationV1, expectedWorkspaceIdentity string, caseID string, body []byte, hook caseBindingReadHook) (domainsecurity.CaseBindingObservationV1, error) {
	unavailable := domainsecurity.CaseBindingObservationV1{}
	workspace, err := openCaseBindingAbsoluteDirectory(workspacePath)
	if err != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	defer unix.Close(workspace)
	var workspaceStat unix.Stat_t
	if unix.Fstat(workspace, &workspaceStat) != nil || !caseBindingUnixDirectory(workspaceStat) || caseBindingCreateWorkspaceIdentity(workspacePath, workspaceStat) != expectedWorkspaceIdentity {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	if hook != nil {
		hook("after_open_workspace")
	}
	if err := validateCaseBindingCreateWorkspace(ctx, workspacePath, workspaceStat); err != nil {
		return unavailable, err
	}
	metadata, err := unix.Openat(workspace, workspaceHostMetadataDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		// Exclusive creation: a competing creator is rejected, never adopted or
		// chmodded. A new empty directory may remain after later cancellation.
		if err := unix.Mkdirat(workspace, workspaceHostMetadataDir, 0o700); err != nil {
			return unavailable, ErrCaseBindingCreateUnavailable
		}
		if unix.Fsync(workspace) != nil {
			return unavailable, ErrCaseBindingCreateUnavailable
		}
		metadata, err = unix.Openat(workspace, workspaceHostMetadataDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	defer unix.Close(metadata)
	var metadataStat unix.Stat_t
	if unix.Fstat(metadata, &metadataStat) != nil || !caseBindingUnixDirectory(metadataStat) ||
		metadataStat.Uid != uint32(os.Geteuid()) || metadataStat.Mode&0o077 != 0 {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	if hook != nil {
		hook("after_open_metadata")
	}
	validateMissing := func() error {
		if err := validateCaseBindingCreateWorkspace(ctx, workspacePath, workspaceStat); err != nil {
			return err
		}
		var linked unix.Stat_t
		if unix.Fstatat(workspace, workspaceHostMetadataDir, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil ||
			!sameCaseBindingUnixDirectory(metadataStat, linked) {
			return ErrCaseBindingCreateUnavailable
		}
		if err := unix.Fstatat(metadata, caseBindingFileName, &linked, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
			return ErrCaseBindingCreateUnavailable
		}
		current, err := (CaseBindingReader{}).Observe(workspacePath)
		if err != nil || current != expected {
			return ErrCaseBindingCreateUnavailable
		}
		return ctx.Err()
	}
	if err := validateMissing(); err != nil {
		return unavailable, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	tempName := ".case-binding-create-" + hex.EncodeToString(nonce[:])
	fd, err := unix.Openat(metadata, tempName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	defer unix.Close(fd)
	var tempStat unix.Stat_t
	if unix.Fstat(fd, &tempStat) != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	installed := false
	defer func() {
		if !installed {
			var linked unix.Stat_t
			if unix.Fstatat(metadata, tempName, &linked, unix.AT_SYMLINK_NOFOLLOW) == nil && linked.Dev == tempStat.Dev && linked.Ino == tempStat.Ino {
				_ = unix.Unlinkat(metadata, tempName, 0)
				_ = unix.Fsync(metadata)
			}
		}
	}()
	for remaining := body; len(remaining) > 0; {
		if err := ctx.Err(); err != nil {
			return unavailable, err
		}
		written, err := unix.Write(fd, remaining)
		if err != nil || written <= 0 {
			return unavailable, ErrCaseBindingCreateUnavailable
		}
		remaining = remaining[written:]
	}
	if unix.Fsync(fd) != nil || unix.Fstat(fd, &tempStat) != nil ||
		!caseBindingUnixRegularSingleLink(tempStat) || tempStat.Mode&0o777 != 0o600 || tempStat.Uid != uint32(os.Geteuid()) {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	if hook != nil {
		hook("before_commit")
	}
	if err := validateMissing(); err != nil {
		return unavailable, err
	}
	var linkedTemp unix.Stat_t
	if unix.Fstatat(metadata, tempName, &linkedTemp, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameCaseBindingUnixFile(tempStat, linkedTemp) {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	// Existing OS-specific NOREPLACE/RENAME_EXCL primitive is the only commit.
	// Once committed, a later cancellation or validation failure returns no
	// authority; the marker is preserved for an explicit subsequent inspection.
	if err := installAtomicUnixText(metadata, tempName, caseBindingFileName, false); err != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	installed = true
	if unix.Fsync(metadata) != nil || unix.Fsync(workspace) != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	if hook != nil {
		hook("after_commit")
	}
	if err := ctx.Err(); err != nil {
		return unavailable, err
	}
	if validateCaseBindingUnixPath(workspacePath, workspace, workspaceStat, metadata, metadataStat, tempStat) != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	current, err := (CaseBindingReader{}).Observe(workspacePath)
	if err != nil || current.State != domainsecurity.CaseBindingStateValid || current.CaseID != caseID ||
		current.BindingSHA256 != domainsecurity.SHA256Hex(body) {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return unavailable, err
	}
	if validateCaseBindingUnixPath(workspacePath, workspace, workspaceStat, metadata, metadataStat, tempStat) != nil {
		return unavailable, ErrCaseBindingCreateUnavailable
	}
	return current, nil
}

func validateCaseBindingCreateWorkspace(ctx context.Context, path string, expected unix.Stat_t) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, err := openCaseBindingAbsoluteDirectory(path)
	if err != nil {
		return ErrCaseBindingCreateUnavailable
	}
	defer unix.Close(fd)
	var current unix.Stat_t
	if unix.Fstat(fd, &current) != nil || !sameCaseBindingUnixDirectory(expected, current) {
		return ErrCaseBindingCreateUnavailable
	}
	return nil
}

func observeMissingImportWorkspacePlatform(workspacePath string) (domainsecurity.CaseBindingObservationV1, string, error) {
	fd, err := openCaseBindingAbsoluteDirectory(workspacePath)
	if err != nil {
		return domainsecurity.CaseBindingObservationV1{}, "", ErrCaseBindingCreateUnavailable
	}
	defer unix.Close(fd)
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !caseBindingUnixDirectory(before) {
		return domainsecurity.CaseBindingObservationV1{}, "", ErrCaseBindingCreateUnavailable
	}
	observation, err := (CaseBindingReader{}).Observe(workspacePath)
	if err != nil || observation.State != domainsecurity.CaseBindingStateMissing ||
		validateCaseBindingCreateWorkspace(context.Background(), workspacePath, before) != nil {
		return domainsecurity.CaseBindingObservationV1{}, "", ErrCaseBindingCreateUnavailable
	}
	return observation, caseBindingCreateWorkspaceIdentity(workspacePath, before), nil
}

func caseBindingCreateWorkspaceIdentity(path string, stat unix.Stat_t) string {
	body, _ := json.Marshal(struct {
		Purpose string `json:"purpose"`
		Path    string `json:"path"`
		Device  uint64 `json:"device"`
		Inode   uint64 `json:"inode"`
		Owner   uint32 `json:"owner"`
		Group   uint32 `json:"group"`
		Mode    uint32 `json:"mode"`
	}{"analytix.main-selected-import-workspace/v1", path, uint64(stat.Dev), uint64(stat.Ino), stat.Uid, stat.Gid, uint32(stat.Mode)})
	return domainsecurity.SHA256Hex(body)
}
