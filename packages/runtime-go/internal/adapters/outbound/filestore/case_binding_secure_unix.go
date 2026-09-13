//go:build !windows

package filestore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"golang.org/x/sys/unix"
)

const caseBindingFileName = "case-project.json"

func secureReadCaseBinding(workspaceRealPath string, maxBytes int, hook caseBindingReadHook) ([]byte, error) {
	workspace, err := openCaseBindingAbsoluteDirectory(workspaceRealPath)
	if err != nil {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
		}
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}
	defer unix.Close(workspace)
	var workspaceStat unix.Stat_t
	if err := unix.Fstat(workspace, &workspaceStat); err != nil || !caseBindingUnixDirectory(workspaceStat) {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, errors.New("case binding workspace handle is invalid"))
	}
	metadata, err := unix.Openat(workspace, workspaceHostMetadataDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, classifyCaseBindingUnixOpenError(err, true)
	}
	defer unix.Close(metadata)
	var metadataStat unix.Stat_t
	if err := unix.Fstat(metadata, &metadataStat); err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	}
	if !caseBindingUnixDirectory(metadataStat) || !caseBindingUnixPrivateOwner(metadataStat) {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, errors.New("case binding metadata directory authority is unsafe"))
	}
	body, fileStat, err := readCaseBindingUnixFile(metadata, maxBytes)
	if err != nil {
		return nil, err
	}
	if hook != nil {
		hook("after_initial_read")
	}
	if err := validateCaseBindingUnixPath(workspaceRealPath, workspace, workspaceStat, metadata, metadataStat, fileStat); err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}
	readback, readbackStat, err := readCaseBindingUnixFile(metadata, maxBytes)
	if err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}
	if !sameCaseBindingUnixFile(fileStat, readbackStat) || !bytes.Equal(body, readback) {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, errors.New("case binding changed during secure readback"))
	}
	if hook != nil {
		hook("before_final_validation")
	}
	if err := validateCaseBindingUnixPath(workspaceRealPath, workspace, workspaceStat, metadata, metadataStat, readbackStat); err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}
	return body, nil
}

func openCaseBindingAbsoluteDirectory(path string) (int, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return -1, errors.New("case binding directory path must be absolute")
	}
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	components := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." {
			continue
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		unix.Close(current)
		if openErr != nil {
			return -1, openErr
		}
		var stat unix.Stat_t
		if statErr := unix.Fstat(next, &stat); statErr != nil || !caseBindingUnixDirectory(stat) {
			unix.Close(next)
			return -1, errors.New("case binding directory traversal is unsafe")
		}
		current = next
	}
	return current, nil
}

func readCaseBindingUnixFile(metadata int, maxBytes int) ([]byte, unix.Stat_t, error) {
	fd, err := unix.Openat(metadata, caseBindingFileName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, unix.Stat_t{}, classifyCaseBindingUnixOpenError(err, true)
	}
	file := os.NewFile(uintptr(fd), caseBindingFileName)
	if file == nil {
		unix.Close(fd)
		return nil, unix.Stat_t{}, newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, errors.New("case binding file handle is invalid"))
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return nil, unix.Stat_t{}, newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	}
	if !caseBindingUnixRegularSingleLink(before) || !caseBindingUnixPrivateOwner(before) || before.Size <= 0 || before.Size > int64(maxBytes) {
		return nil, unix.Stat_t{}, newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, errors.New("case binding file is not a bounded, private, single-link regular file"))
	}
	body, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	var after unix.Stat_t
	statErr := unix.Fstat(fd, &after)
	if readErr != nil || statErr != nil {
		return nil, unix.Stat_t{}, newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, errors.Join(readErr, statErr))
	}
	if len(body) == 0 || len(body) > maxBytes || !sameCaseBindingUnixFile(before, after) || int64(len(body)) != after.Size {
		return nil, unix.Stat_t{}, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, errors.New("case binding file changed while being read"))
	}
	return body, after, nil
}

func validateCaseBindingUnixPath(workspaceRealPath string, workspace int, workspaceStat unix.Stat_t, metadata int, metadataStat unix.Stat_t, fileStat unix.Stat_t) error {
	var currentMetadata unix.Stat_t
	if err := unix.Fstatat(workspace, workspaceHostMetadataDir, &currentMetadata, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		!sameCaseBindingUnixDirectory(metadataStat, currentMetadata) {
		return errors.New("case binding metadata directory changed during read")
	}
	var currentFile unix.Stat_t
	if err := unix.Fstatat(metadata, caseBindingFileName, &currentFile, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		!caseBindingUnixRegularSingleLink(currentFile) || !sameCaseBindingUnixFile(fileStat, currentFile) {
		return errors.New("case binding path changed during read")
	}
	reopened, err := openCaseBindingAbsoluteDirectory(workspaceRealPath)
	if err != nil {
		return errors.New("case binding workspace path changed during read")
	}
	defer unix.Close(reopened)
	var reopenedStat unix.Stat_t
	if err := unix.Fstat(reopened, &reopenedStat); err != nil || !sameCaseBindingUnixDirectory(workspaceStat, reopenedStat) {
		return errors.New("case binding workspace path changed during read")
	}
	return nil
}

func caseBindingUnixDirectory(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR
}

func caseBindingUnixRegularSingleLink(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Nlink == 1
}

func caseBindingUnixPrivateOwner(stat unix.Stat_t) bool {
	return stat.Uid == uint32(os.Geteuid()) && stat.Mode&0o022 == 0
}

func classifyCaseBindingUnixOpenError(err error, missingIsBinding bool) error {
	switch {
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	case errors.Is(err, unix.ENOENT):
		if missingIsBinding {
			return newCaseBindingReadError(domainsecurity.CaseBindingStateMissing, err)
		}
		return newCaseBindingReadError(domainsecurity.CaseBindingStateWorkspaceMissing, err)
	case errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR):
		return newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, err)
	default:
		return newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	}
}

func sameCaseBindingUnixDirectory(left unix.Stat_t, right unix.Stat_t) bool {
	return caseBindingUnixDirectory(left) && caseBindingUnixDirectory(right) && left.Dev == right.Dev && left.Ino == right.Ino &&
		left.Mode == right.Mode && left.Uid == right.Uid && left.Gid == right.Gid
}

func sameCaseBindingUnixFile(left unix.Stat_t, right unix.Stat_t) bool {
	return caseBindingUnixRegularSingleLink(left) && caseBindingUnixRegularSingleLink(right) &&
		left.Dev == right.Dev && left.Ino == right.Ino && left.Size == right.Size && left.Mode == right.Mode &&
		left.Uid == right.Uid && left.Gid == right.Gid
}
