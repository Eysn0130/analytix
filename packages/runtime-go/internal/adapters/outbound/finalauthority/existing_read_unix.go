//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func secureReadExistingPrivateNamedFile(path, name string, maxBytes int) ([]byte, existingFileAuthorityIdentity, error) {
	if !securePrivateNamedComponent(name) || maxBytes <= 0 {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private named authority input is invalid")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority root is invalid")
	}
	absolute = filepath.Clean(absolute)
	root, err := securePrivateOpenAbsoluteDirectory(absolute, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, existingFileAuthorityIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, fmt.Errorf("open existing private authority root: %w", err)
	}
	defer unix.Close(root)
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil || !existingPrivateAuthorityRootSafe(rootStat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(root) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority root ownership or permissions are unsafe")
	}
	entries, err := privateCASUnixReadDirBounded(root, 1)
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, fmt.Errorf("read existing private authority inventory: %w", err)
	}
	if len(entries) == 0 {
		return nil, existingFileAuthorityIdentity{}, os.ErrNotExist
	}
	if !existingPrivateAuthorityInventoryExact(entries, name) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority root contains recovery residue or unknown state")
	}
	body, identity, err := secureReadExistingPrivateNamedAt(root, name, int64(maxBytes))
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, fmt.Errorf("read existing private authority target: %w", err)
	}
	readback, current, err := secureReadExistingPrivateNamedAt(root, name, int64(maxBytes))
	if err != nil || identity != current || !bytes.Equal(body, readback) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority changed during read")
	}
	entries, err = privateCASUnixReadDirBounded(root, 1)
	if err != nil || !existingPrivateAuthorityInventoryExact(entries, name) ||
		unix.Fstat(root, &rootStat) != nil || !existingPrivateAuthorityRootSafe(rootStat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(root) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority root changed during read")
	}
	return body, existingFileAuthorityIdentity{
		rootVolume: uint64(rootStat.Dev), rootObjectLow: rootStat.Ino,
		fileVolume: identity.dev, fileObjectLow: identity.ino, fileSize: uint64(identity.size),
	}, nil
}

type existingPrivateNamedIdentity struct {
	dev  uint64
	ino  uint64
	size int64
}

func secureReadExistingPrivateNamedAt(parent int, name string, maxBytes int64) ([]byte, existingPrivateNamedIdentity, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, existingPrivateNamedIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return nil, existingPrivateNamedIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, existingPrivateNamedIdentity{}, errors.New("existing private authority file handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !existingPrivateAuthorityFileSafe(stat, uint32(os.Geteuid()), maxBytes) ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) {
		return nil, existingPrivateNamedIdentity{}, errors.New("existing private authority file ownership, mode, type, links, or size are unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, existingPrivateNamedIdentity{}, errors.New("existing private authority exact read failed")
	}
	return body, existingPrivateNamedIdentity{dev: uint64(stat.Dev), ino: stat.Ino, size: stat.Size}, nil
}

func existingPrivateAuthorityRootSafe(stat unix.Stat_t, hostUID uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Mode&0o077 == 0 && stat.Uid == hostUID
}

func existingPrivateAuthorityFileSafe(stat unix.Stat_t, hostUID uint32, maxBytes int64) bool {
	return privateStatRegular(stat) && stat.Nlink == 1 && stat.Uid == hostUID && stat.Size > 0 && stat.Size <= maxBytes
}

func existingPrivateAuthorityInventoryExact(entries []os.DirEntry, name string) bool {
	return len(entries) == 1 && entries[0].Name() == name && !entries[0].IsDir()
}
