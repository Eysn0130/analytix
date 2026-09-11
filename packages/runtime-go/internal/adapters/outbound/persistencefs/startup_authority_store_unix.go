//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type startupAuthorityRoot struct {
	path string
	dev  uint64
	ino  uint64
}

func (root startupAuthorityRoot) pathValue() string { return root.path }

func captureStartupAuthorityRoot(path string) (startupAuthorityRoot, error) {
	fd, err := secureOpenAbsoluteDirectory(path, false)
	if err != nil {
		return startupAuthorityRoot{}, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 {
		return startupAuthorityRoot{}, errors.New("startup journal namespace permissions are unsafe")
	}
	if err := unix.Fsync(fd); err != nil {
		return startupAuthorityRoot{}, err
	}
	return startupAuthorityRoot{path: filepath.Clean(path), dev: uint64(stat.Dev), ino: uint64(stat.Ino)}, nil
}

func (root startupAuthorityRoot) open() (int, error) {
	fd, err := secureOpenAbsoluteDirectory(root.path, false)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 ||
		uint64(stat.Dev) != root.dev || uint64(stat.Ino) != root.ino {
		_ = unix.Close(fd)
		return -1, errors.New("startup journal namespace identity changed")
	}
	return fd, nil
}

func validateStartupAuthorityRoot(root startupAuthorityRoot) error {
	fd, err := root.open()
	if err != nil {
		return err
	}
	return unix.Close(fd)
}

func secureStartupAuthorityRead(root startupAuthorityRoot, name string, maxBytes int64) ([]byte, string, error) {
	if !startupAuthorityNamedComponent(name) || maxBytes <= 0 {
		return nil, "", errors.New("startup authority named read input is invalid")
	}
	directory, err := root.open()
	if err != nil {
		return nil, "", err
	}
	defer unix.Close(directory)
	return secureStartupAuthorityReadAt(directory, name, maxBytes)
}

func secureStartupAuthorityReadAt(directory int, name string, maxBytes int64) ([]byte, string, error) {
	fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, "", os.ErrNotExist
	}
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, "", errors.New("startup authority file handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o077 != 0 ||
		stat.Nlink != 1 || stat.Size <= 0 || stat.Size > maxBytes {
		return nil, "", errors.New("startup authority file is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, "", errors.New("startup authority file read failed")
	}
	return body, formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino)), nil
}

func secureStartupAuthorityWriteExclusive(root startupAuthorityRoot, name string, body []byte, maxBytes int64) error {
	if !startupAuthorityNamedComponent(name) || len(body) == 0 || int64(len(body)) > maxBytes {
		return errors.New("startup authority named write input is invalid")
	}
	directory, err := root.open()
	if err != nil {
		return err
	}
	defer unix.Close(directory)
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary, err := startupAuthorityCreateTempName(name, body, hex.EncodeToString(suffix))
	if err != nil {
		return err
	}
	fd, err := unix.Openat(directory, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("startup authority temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = unix.Unlinkat(directory, temporary, 0)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("startup authority write was incomplete")
		}
		written += count
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("startup authority staged write verification failed")
	}
	if err := secureStartupAuthorityCommitNoReplace(directory, temporary, name); err != nil {
		return err
	}
	tempExists = false
	if err := unix.Fsync(directory); err != nil {
		return err
	}
	written, _, err := secureStartupAuthorityReadAt(directory, name, maxBytes)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup authority committed write verification failed")
	}
	return nil
}

func secureStartupAuthorityCleanCreateResidue(root startupAuthorityRoot, name string, maxBytes int64, validate func([]byte) error) error {
	return cleanStartupAuthorityCreateResidue(root, name, maxBytes, validate)
}

func secureStartupAuthorityReadTempAt(directory int, name string, maxBytes int64) ([]byte, error) {
	fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("startup authority temp handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o077 != 0 ||
		stat.Nlink != 1 || stat.Size < 0 || stat.Size > maxBytes {
		return nil, errors.New("startup authority temp is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, errors.New("startup authority temp read failed")
	}
	return body, nil
}

func startupAuthorityNamedComponent(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsRune(name, 0)
}
