//go:build darwin || linux

package secureconfigfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type unixIdentity struct {
	device uint64
	inode  uint64
	size   int64
	fs     secureFilesystemIdentity
}

type secureFilesystemIdentity struct {
	kind    string
	fsid    [2]int64
	mountID uint64
}

type secureFilesystemProbe func(int) (secureFilesystemIdentity, error)

func readBundle(input normalizedBundle) (map[string][]byte, error) {
	return readBundleWithFilesystemProbe(input, probeSecureFilesystem)
}

func readBundleWithFilesystemProbe(
	input normalizedBundle,
	filesystemProbe secureFilesystemProbe,
) (map[string][]byte, error) {
	if filesystemProbe == nil {
		return nil, errors.New("secure configuration filesystem authority is unavailable")
	}
	failed := true
	var bodies map[string][]byte
	defer func() {
		if failed {
			wipeSensitiveBundle(input.files, bodies)
		}
	}()
	root, err := openAbsoluteDirectory(input.root)
	if err != nil {
		return nil, fmt.Errorf("open secure configuration root: %w", err)
	}
	defer unix.Close(root)
	rootIdentity, err := validateRoot(root, filesystemProbe)
	if err != nil {
		return nil, err
	}
	if err := validateInventory(root, input.expectedNames); err != nil {
		return nil, err
	}
	bodies = make(map[string][]byte, len(input.files))
	identities := make(map[string]unixIdentity, len(input.files))
	var totalBytes int64
	for _, target := range input.files {
		body, identity, readErr := readNamed(root, target.Name, target.MaxBytes, rootIdentity, filesystemProbe)
		if readErr != nil {
			return nil, readErr
		}
		if int64(len(body)) > input.maxTotalBytes-totalBytes {
			return nil, errors.New("secure configuration bundle exceeds total size limit")
		}
		totalBytes += int64(len(body))
		bodies[target.Name] = body
		identities[target.Name] = identity
	}
	for _, target := range input.files {
		readback, current, readErr := readNamed(root, target.Name, target.MaxBytes, rootIdentity, filesystemProbe)
		equal := readErr == nil && identities[target.Name] == current && bytes.Equal(bodies[target.Name], readback)
		if target.Sensitive {
			clear(readback)
		}
		if !equal {
			return nil, errors.New("secure configuration bundle changed during read")
		}
	}
	if err := validateInventory(root, input.expectedNames); err != nil {
		return nil, errors.New("secure configuration inventory changed during read")
	}
	reopened, err := openAbsoluteDirectory(input.root)
	if err != nil {
		return nil, errors.New("secure configuration root changed during read")
	}
	defer unix.Close(reopened)
	currentRoot, err := validateRoot(reopened, filesystemProbe)
	if err != nil || currentRoot != rootIdentity || validateInventory(reopened, input.expectedNames) != nil {
		return nil, errors.New("secure configuration root changed during read")
	}
	failed = false
	return bodies, nil
}

func wipeSensitiveBundle(files []BundleFile, bodies map[string][]byte) {
	for _, file := range files {
		if file.Sensitive {
			clear(bodies[file.Name])
		}
	}
}

func openAbsoluteDirectory(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, errors.New("secure configuration root is not absolute and clean")
	}
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	components := strings.FieldsFunc(strings.TrimPrefix(path, string(filepath.Separator)), func(char rune) bool {
		return char == filepath.Separator
	})
	for _, component := range components {
		if !safeName(component) {
			_ = unix.Close(current)
			return -1, errors.New("secure configuration root component is invalid")
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return -1, openErr
		}
		current = next
	}
	return current, nil
}

func validateRoot(fd int, filesystemProbe secureFilesystemProbe) (unixIdentity, error) {
	var stat unix.Stat_t
	filesystem, filesystemErr := filesystemProbe(fd)
	if err := unix.Fstat(fd, &stat); filesystemErr != nil || err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) ||
		stat.Mode&0o077 != 0 || stat.Mode&0o7000 != 0 || !extendedSecuritySafe(fd) {
		return unixIdentity{}, errors.New("secure configuration root ownership, mode, type, or ACL is unsafe")
	}
	return unixIdentity{device: uint64(stat.Dev), inode: stat.Ino, fs: filesystem}, nil
}

func validateInventory(root int, expected []string) error {
	duplicate, err := unix.Openat(root, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(duplicate), "secure-config-root")
	if directory == nil {
		_ = unix.Close(duplicate)
		return errors.New("secure configuration directory handle is invalid")
	}
	names, readErr := directory.Readdirnames(-1)
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	sort.Strings(names)
	if len(names) != len(expected) {
		return errors.New("secure configuration root contains unknown or missing files")
	}
	for index := range names {
		if names[index] != expected[index] {
			return errors.New("secure configuration root contains unknown or missing files")
		}
	}
	return nil
}

func readNamed(
	root int,
	name string,
	maxBytes int64,
	rootIdentity unixIdentity,
	filesystemProbe secureFilesystemProbe,
) ([]byte, unixIdentity, error) {
	fd, err := unix.Openat(root, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, unixIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, unixIdentity{}, errors.New("secure configuration file handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	filesystem, filesystemErr := filesystemProbe(fd)
	if err := unix.Fstat(fd, &stat); filesystemErr != nil || filesystem != rootIdentity.fs || err != nil ||
		uint64(stat.Dev) != rootIdentity.device ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 ||
		stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 || stat.Mode&0o7000 != 0 ||
		stat.Size <= 0 || stat.Size > maxBytes || !extendedSecuritySafe(fd) {
		return nil, unixIdentity{}, errors.New("secure configuration file ownership, mode, type, links, size, or ACL is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, unixIdentity{}, errors.New("secure configuration exact read failed")
	}
	return body, unixIdentity{device: uint64(stat.Dev), inode: stat.Ino, size: stat.Size, fs: filesystem}, nil
}
