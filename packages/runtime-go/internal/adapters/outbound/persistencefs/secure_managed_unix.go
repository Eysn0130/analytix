//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"golang.org/x/sys/unix"
)

func secureEnsureManagedRoot(authority *RootAuthority, root string) error {
	capability, err := authority.rootCapability(root)
	if err != nil {
		return err
	}
	if capability.RootIdentity == "" {
		fd, identity, err := secureCreateManagedRootExclusive(capability)
		if err != nil {
			return err
		}
		if err := authority.pinCreatedRoot(root, identity); err != nil {
			_ = unix.Close(fd)
			return err
		}
		return errors.Join(unix.Fsync(fd), unix.Close(fd))
	}
	fd, err := secureOpenAbsoluteDirectory(root, true)
	if err != nil {
		return err
	}
	identity, err := unixOpenedDirectoryIdentity(fd)
	if err != nil || authority.verifyOpenedRoot(root, identity) != nil {
		_ = unix.Close(fd)
		return errors.New("semantic startup opened a different managed root")
	}
	return errors.Join(unix.Fsync(fd), unix.Close(fd))
}

func secureCreateManagedRootExclusive(capability frozenRootCapability) (int, string, error) {
	if capability.RootIdentity != "" || capability.MissingSuffix == "" {
		return -1, "", errors.New("cold persistence root creation capability is invalid")
	}
	current, err := secureOpenAbsoluteDirectory(capability.Anchor, false)
	if err != nil {
		return -1, "", err
	}
	anchorIdentity, err := unixOpenedDirectoryIdentity(current)
	if err != nil || anchorIdentity != capability.AnchorIdentity {
		_ = unix.Close(current)
		return -1, "", errors.New("cold persistence root ancestor identity changed")
	}
	parts, err := secureRelativeComponents(capability.MissingSuffix)
	if err != nil {
		_ = unix.Close(current)
		return -1, "", err
	}
	for _, component := range parts {
		if err := unix.Mkdirat(current, component, 0o700); err != nil {
			_ = unix.Close(current)
			if errors.Is(err, unix.EEXIST) {
				return -1, "", errors.New("cold persistence root appeared after authority freeze")
			}
			return -1, "", err
		}
		if err := unix.Fsync(current); err != nil {
			_ = unix.Close(current)
			return -1, "", err
		}
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			_ = unix.Close(current)
			return -1, "", err
		}
		_ = unix.Close(current)
		current = next
	}
	identity, err := unixOpenedDirectoryIdentity(current)
	if err != nil {
		_ = unix.Close(current)
		return -1, "", err
	}
	return current, identity, nil
}

func secureManagedTargetMatches(authority *RootAuthority, root, relative string, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	parent, base, err := secureOpenManagedParent(authority, root, relative)
	if errors.Is(err, os.ErrNotExist) {
		return expected.Type == domainstartup.ManagedEntryTypeAbsent, nil
	}
	if err != nil {
		return false, err
	}
	defer unix.Close(parent)
	return secureTargetMatchesAt(parent, base, expected)
}

func secureManagedCreateDirectory(authority *RootAuthority, root, relative string, mode os.FileMode) error {
	parent, base, err := secureOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	if err := unix.Mkdirat(parent, base, uint32(mode.Perm())); err != nil {
		return err
	}
	return unix.Fsync(parent)
}

func secureManagedInstallFile(
	authority *RootAuthority,
	root, relative string,
	body []byte,
	mode os.FileMode,
	operationID string,
	before domainstartup.SemanticEntryStateV1,
	after domainstartup.SemanticEntryStateV1,
) error {
	if !domainsecurity.IsSHA256Hex(operationID) {
		return errors.New("semantic startup install operation identity is invalid")
	}
	parent, base, err := secureOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	if matches, err := secureTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup install target changed before write")
	}
	temporary := "." + base + ".startup-" + operationID + ".tmp"
	existing, exists, err := secureReadRegularAt(parent, temporary)
	if err != nil {
		return err
	}
	if exists && !bytes.Equal(existing, body) {
		if matches, matchErr := secureTargetMatchesAt(parent, base, before); matchErr != nil || !matches {
			return errors.New("semantic startup install temp conflicts with target state")
		}
		if err := unix.Unlinkat(parent, temporary, 0); err != nil {
			return err
		}
		if err := unix.Fsync(parent); err != nil {
			return err
		}
		exists = false
	}
	if !exists {
		if err := secureWriteExclusiveAt(parent, temporary, body, mode); err != nil {
			return err
		}
	}
	if matches, err := secureTargetMatchesAt(parent, base, after); err == nil && matches {
		_ = unix.Unlinkat(parent, temporary, 0)
		return unix.Fsync(parent)
	}
	if matches, err := secureTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup install target changed before commit")
	}
	if err := unix.Renameat(parent, temporary, parent, base); err != nil {
		return err
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	matches, err := secureTargetMatchesAt(parent, base, after)
	if err != nil || !matches {
		return errors.New("semantic startup secure install readback failed")
	}
	return nil
}

func secureManagedSetMode(authority *RootAuthority, root, relative string, before domainstartup.SemanticEntryStateV1, mode os.FileMode) error {
	parent, base, err := secureOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	if matches, err := secureTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup mode target changed before update")
	}
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || (before.Type == domainstartup.ManagedEntryTypeFile && stat.Nlink != 1) {
		return errors.New("semantic startup mode target is not single-link authority")
	}
	if err := unix.Fchmod(fd, uint32(mode.Perm())); err != nil {
		return err
	}
	if err := unix.Fsync(fd); err != nil {
		return err
	}
	return unix.Fsync(parent)
}

func secureManagedRemove(authority *RootAuthority, root, relative string, before domainstartup.SemanticEntryStateV1) error {
	parent, base, err := secureOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	if matches, err := secureTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup remove target changed before delete")
	}
	flags := 0
	if before.Type == domainstartup.ManagedEntryTypeDirectory {
		flags = unix.AT_REMOVEDIR
	}
	if err := unix.Unlinkat(parent, base, flags); err != nil {
		return err
	}
	return unix.Fsync(parent)
}

func secureOpenManagedParent(authority *RootAuthority, root, relative string) (int, string, error) {
	if err := authority.validateRoot(root); err != nil {
		return -1, "", err
	}
	parts, err := secureRelativeComponents(relative)
	if err != nil {
		return -1, "", err
	}
	current, err := secureOpenAbsoluteDirectory(root, false)
	if err != nil {
		return -1, "", err
	}
	identity, err := unixOpenedDirectoryIdentity(current)
	if err != nil || authority.verifyOpenedRoot(root, identity) != nil {
		_ = unix.Close(current)
		return -1, "", errors.New("semantic startup opened a different managed root")
	}
	for _, component := range parts[:len(parts)-1] {
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(current)
			return -1, "", openErr
		}
		_ = unix.Close(current)
		current = next
	}
	return current, parts[len(parts)-1], nil
}

func unixOpenedDirectoryIdentity(fd int) (string, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", errors.New("semantic startup managed root identity is unavailable")
	}
	return unixDirectoryIdentity(stat), nil
}

func secureOpenAbsoluteDirectory(path string, create bool) (int, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return -1, errors.New("semantic startup secure root is not absolute")
	}
	if !create {
		if fd, handled, err := platformSecureOpenExistingAbsoluteDirectory(path); handled {
			return fd, err
		}
	}
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && create {
			if mkdirErr := unix.Mkdirat(current, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, mkdirErr
			}
			if syncErr := unix.Fsync(current); syncErr != nil {
				_ = unix.Close(current)
				return -1, syncErr
			}
			next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, openErr
		}
		_ = unix.Close(current)
		current = next
	}
	return current, nil
}

func secureRelativeComponents(relative string) ([]string, error) {
	relative = filepath.Clean(relative)
	if relative == "" || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("semantic startup secure relative path is invalid")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for _, component := range parts {
		if component == "" || component == "." || component == ".." || strings.IndexByte(component, 0) >= 0 {
			return nil, errors.New("semantic startup secure path component is invalid")
		}
	}
	return parts, nil
}

func secureTargetMatchesAt(parent int, base string, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return expected.Type == domainstartup.ManagedEntryTypeAbsent, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), base)
	if file == nil {
		_ = unix.Close(fd)
		return false, errors.New("semantic startup secure target handle is invalid")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return false, errors.New("semantic startup managed target is not single-link authority")
	}
	if expected.Type == domainstartup.ManagedEntryTypeAbsent || uint32(info.Mode()) != expected.Mode {
		return false, nil
	}
	switch expected.Type {
	case domainstartup.ManagedEntryTypeDirectory:
		return info.IsDir(), nil
	case domainstartup.ManagedEntryTypeFile:
		if !info.Mode().IsRegular() || stat.Nlink != 1 || info.Size() != expected.Size {
			return false, nil
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(hasher, file)
		if copyErr != nil || written != expected.Size {
			return false, copyErr
		}
		return hex.EncodeToString(hasher.Sum(nil)) == expected.SHA256, nil
	default:
		return false, errors.New("semantic startup secure expected state is invalid")
	}
}

func secureReadRegularAt(parent int, base string) ([]byte, bool, error) {
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(fd), base)
	if file == nil {
		_ = unix.Close(fd)
		return nil, false, errors.New("semantic startup secure temp handle is invalid")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, errors.New("semantic startup secure temp is not regular")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Nlink != 1 {
		return nil, false, errors.New("semantic startup secure temp is not single-link")
	}
	body, err := io.ReadAll(io.LimitReader(file, 64*1024*1024+1))
	if err != nil || len(body) > 64*1024*1024 {
		return nil, false, errors.New("semantic startup secure temp is unreadable")
	}
	return body, true, nil
}

func secureWriteExclusiveAt(parent int, base string, body []byte, mode os.FileMode) error {
	fd, err := unix.Openat(parent, base, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), base)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("semantic startup secure write handle is invalid")
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("semantic startup secure write was incomplete")
		}
		written += count
	}
	if err := unix.Fchmod(fd, uint32(mode.Perm())); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	return unix.Fsync(parent)
}
