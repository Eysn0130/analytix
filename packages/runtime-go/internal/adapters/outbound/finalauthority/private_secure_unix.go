//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type privateRootAuthority struct {
	path string
	dev  uint64
	ino  uint64
}

func newPrivateNamedRootAuthority(path, name string, maxBytes int) (privateRootAuthority, error) {
	if !securePrivateNamedComponent(name) || maxBytes <= 0 {
		return privateRootAuthority{}, errors.New("private named authority root input is invalid")
	}
	authority, err := capturePrivateRootAuthority(path)
	if err != nil {
		return privateRootAuthority{}, err
	}
	if err := secureRecoverPrivateNamedRoot(authority, name, int64(maxBytes)); err != nil {
		return privateRootAuthority{}, err
	}
	return authority, nil
}

func capturePrivateRootAuthority(path string) (privateRootAuthority, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return privateRootAuthority{}, errors.New("private authority root is invalid")
	}
	absolute, err = canonicalPrivateRootPath(absolute)
	if err != nil {
		return privateRootAuthority{}, err
	}
	fd, err := securePrivateOpenAbsoluteDirectory(absolute, true)
	if err != nil {
		return privateRootAuthority{}, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) {
		return privateRootAuthority{}, errors.New("private authority root permissions are unsafe")
	}
	authority := privateRootAuthority{path: filepath.Clean(absolute), dev: uint64(stat.Dev), ino: stat.Ino}
	if err := unix.Fsync(fd); err != nil {
		return privateRootAuthority{}, err
	}
	return authority, nil
}

func secureWritePrivateNamedFileExclusive(authority privateRootAuthority, name string, body []byte, maxBytes int) error {
	if !securePrivateNamedComponent(name) || len(body) == 0 || maxBytes <= 0 || len(body) > maxBytes {
		return errors.New("private named authority write input is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"
	fd, err := unix.Openat(root, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("private named authority temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = unix.Unlinkat(root, temporary, 0)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("private named authority write was incomplete")
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
	staged, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("private named authority staged write verification failed")
	}
	if err := securePrivateCommitNoReplace(root, temporary, name); err != nil {
		return err
	}
	tempExists = false
	if err := unix.Fsync(root); err != nil {
		return err
	}
	written, err := securePrivateReadNamedAt(root, name, int64(maxBytes))
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("private named authority committed write verification failed")
	}
	return nil
}

func secureReadPrivateNamedFile(authority privateRootAuthority, name string, maxBytes int) ([]byte, error) {
	if !securePrivateNamedComponent(name) || maxBytes <= 0 {
		return nil, errors.New("private named authority read input is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	return securePrivateReadNamedAt(root, name, int64(maxBytes))
}

func secureRecoverPrivateNamedRoot(authority privateRootAuthority, name string, maxBytes int64) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	if err := secureValidatePrivateNamedRoot(root, name, maxBytes); err != nil {
		return err
	}
	changed := false
	for batch := 0; batch < maxPrivateCASRecoveryBatches; batch++ {
		temps := make([]string, 0, privateCASScanPageEntries)
		targetSeen := false
		walkErr := privateCASUnixWalkDir(root, func(entry os.DirEntry) error {
			if entry.Name() == name {
				if entry.IsDir() || targetSeen {
					return errors.New("private named authority target is ambiguous")
				}
				targetSeen = true
				return nil
			}
			if entry.IsDir() || !privateNamedWriteTempName(entry.Name(), name) {
				return errors.New("private named authority root contains unknown residue")
			}
			if len(temps) == cap(temps) {
				return errPrivateNamedRecoveryBatchFull
			}
			temps = append(temps, entry.Name())
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, errPrivateNamedRecoveryBatchFull) {
			return walkErr
		}
		if len(temps) == 0 {
			if walkErr != nil {
				return walkErr
			}
			if targetSeen {
				if _, err := securePrivateReadNamedAt(root, name, maxBytes); err != nil {
					return err
				}
			}
			if changed {
				return unix.Fsync(root)
			}
			return nil
		}
		for _, temporary := range temps {
			stat, err := securePrivateStatAt(root, temporary)
			if err != nil || !privateStatRegular(stat) || stat.Nlink != 1 || stat.Size < 0 || stat.Size > maxBytes {
				return errors.New("private named authority temp is unsafe")
			}
			if err := unix.Unlinkat(root, temporary, 0); err != nil {
				return err
			}
			changed = true
		}
		if err := unix.Fsync(root); err != nil {
			return err
		}
	}
	return errors.New("private named authority recovery did not reach a fixed point")
}

var errPrivateNamedRecoveryBatchFull = errors.New("private named authority recovery batch is full")

func secureValidatePrivateNamedRoot(root int, name string, maxBytes int64) error {
	targetSeen := false
	tempCount := 0
	err := privateCASUnixWalkDir(root, func(entry os.DirEntry) error {
		if entry.Name() == name {
			if entry.IsDir() || targetSeen {
				return errors.New("private named authority target is ambiguous")
			}
			targetSeen = true
			return nil
		}
		if entry.IsDir() || !privateNamedWriteTempName(entry.Name(), name) {
			return errors.New("private named authority root contains unknown residue")
		}
		if tempCount == privateCASScanPageEntries*maxPrivateCASRecoveryBatches {
			return errors.New("private named authority recovery entry bound exceeded")
		}
		stat, err := securePrivateStatAt(root, entry.Name())
		if err != nil || stat.Nlink != 1 || stat.Size < 0 || stat.Size > maxBytes {
			return errors.New("private named authority temp is unsafe")
		}
		tempCount++
		return nil
	})
	if err != nil {
		return err
	}
	if targetSeen {
		if _, err := securePrivateReadNamedAt(root, name, maxBytes); err != nil {
			return err
		}
	}
	return nil
}

func securePrivateReadNamedAt(parent int, name string, maxBytes int64) ([]byte, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("private named authority file handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !privateStatRegular(stat) || stat.Nlink != 1 || stat.Size <= 0 || stat.Size > maxBytes {
		return nil, errors.New("private named authority file is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, errors.New("private named authority file read failed")
	}
	return body, nil
}

func securePrivateNamedComponent(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsRune(name, 0)
}

func canonicalPrivateRootPath(path string) (string, error) {
	original := filepath.Clean(path)
	current := original
	missing := []string{}
	for {
		if info, err := os.Lstat(current); err == nil {
			if current == original && info.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("private authority root cannot be a symlink")
			}
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("private authority root has no existing ancestor")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
	}
	return filepath.Clean(resolved), nil
}

func secureWritePrivateFileExclusive(authority privateRootAuthority, digest string, body []byte) error {
	if !validPrivateDigest(digest) || len(body) == 0 || len(body) > maxPrivateAcceptedFinalBytes {
		return errors.New("private authority secure write input is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	shard, err := securePrivateOpenShard(root, digest[:2], true)
	if err != nil {
		return err
	}
	defer unix.Close(shard)
	name := digest + ".json"
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"
	fd, err := unix.Openat(shard, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("private authority secure temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = unix.Unlinkat(shard, temporary, 0)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("private authority secure write was incomplete")
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
	staged, err := io.ReadAll(io.LimitReader(file, maxPrivateAcceptedFinalBytes+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("private authority secure staged write verification failed")
	}
	if err := securePrivateCommitNoReplace(shard, temporary, name); err != nil {
		return err
	}
	tempExists = false
	if err := unix.Fsync(shard); err != nil {
		return err
	}
	written, err := securePrivateReadAt(shard, name)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("private authority secure committed write verification failed")
	}
	return nil
}

func secureReadPrivateFile(authority privateRootAuthority, digest string) ([]byte, error) {
	if !validPrivateDigest(digest) {
		return nil, errors.New("private authority content address is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	shard, err := securePrivateOpenShard(root, digest[:2], false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(shard)
	return securePrivateReadAt(shard, digest+".json")
}

func secureListPrivateFiles(ctx context.Context, authority privateRootAuthority) ([]securePrivateFile, error) {
	files := make([]securePrivateFile, 0)
	var aggregateBytes uint64
	err := secureVisitPrivateFiles(ctx, authority, func(file securePrivateFile) error {
		if len(files) == maxSecurePrivateCASListRecords || uint64(len(file.Body)) > maxSecurePrivateCASListAggregateBytes-aggregateBytes {
			return ErrSecurePrivateCASMaterializationLimit
		}
		aggregateBytes += uint64(len(file.Body))
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Digest < files[right].Digest })
	return files, nil
}

func secureVisitPrivateFiles(ctx context.Context, authority privateRootAuthority, visit func(securePrivateFile) error) error {
	if visit == nil {
		return errors.New("private authority visitor is required")
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	entries, err := privateCASUnixReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := contextErr(ctx); err != nil {
			return err
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return errors.New("private authority inventory contains a non-canonical shard")
		}
		shard, err := securePrivateOpenShard(root, entry.Name(), false)
		if err != nil {
			return err
		}
		var records uint64
		walkErr := privateCASUnixWalkDir(shard, func(child os.DirEntry) error {
			if err := contextErr(ctx); err != nil {
				return err
			}
			digest := strings.TrimSuffix(child.Name(), ".json")
			if child.IsDir() || filepath.Ext(child.Name()) != ".json" || !validPrivateDigest(digest) || digest[:2] != entry.Name() {
				return errors.New("private authority inventory contains a non-canonical record")
			}
			body, err := securePrivateReadAt(shard, child.Name())
			if err != nil {
				return err
			}
			records++
			return visit(securePrivateFile{Digest: digest, Body: body})
		})
		if walkErr != nil {
			_ = unix.Close(shard)
			return walkErr
		}
		if records == 0 {
			_ = unix.Close(shard)
			return errors.New("private authority inventory contains an empty shard")
		}
		if err := unix.Close(shard); err != nil {
			return err
		}
	}
	return nil
}

func (authority privateRootAuthority) open() (int, error) {
	fd, err := securePrivateOpenAbsoluteDirectory(authority.path, false)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) ||
		uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino {
		_ = unix.Close(fd)
		return -1, errors.New("private authority root identity or permissions changed")
	}
	return fd, nil
}

func securePrivateOpenAbsoluteDirectory(path string, create bool) (int, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return -1, errors.New("private authority secure root is not absolute")
	}
	if !create {
		if fd, handled, err := platformSecurePrivateOpenExistingAbsoluteDirectory(path); handled {
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

func securePrivateOpenShard(root int, shard string, create bool) (int, error) {
	if !validPrivateShard(shard) {
		return -1, errors.New("private authority shard is invalid")
	}
	device, err := privateCASUnixDirectoryDevice(root)
	if err != nil {
		return -1, err
	}
	fd, present, err := privateCASUnixOpenExactBoundDirectory(root, shard, device)
	if err == nil && !present && create {
		if err := unix.Mkdirat(root, shard, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
			return -1, err
		}
		if err := unix.Fsync(root); err != nil {
			return -1, err
		}
		fd, present, err = privateCASUnixOpenExactBoundDirectory(root, shard, device)
	}
	if err == nil && !present {
		return -1, os.ErrNotExist
	}
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) {
		_ = unix.Close(fd)
		return -1, errors.New("private authority shard permissions are unsafe")
	}
	return fd, nil
}

func securePrivateReadAt(parent int, name string) ([]byte, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("private authority secure file handle is invalid")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !privateStatRegular(stat) || stat.Nlink != 1 || stat.Size <= 0 || stat.Size > maxPrivateAcceptedFinalBytes {
		return nil, errors.New("private authority secure file is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxPrivateAcceptedFinalBytes+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, errors.New("private authority secure file read failed")
	}
	return body, nil
}

func securePrivateStatAt(parent int, name string) (unix.Stat_t, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return unix.Stat_t{}, os.ErrNotExist
	}
	if err != nil {
		return unix.Stat_t{}, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !privateStatRegular(stat) ||
		stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 || stat.Mode&0o7000 != 0 ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) {
		return unix.Stat_t{}, errors.New("private authority file ownership, mode, type, or ACL is unsafe")
	}
	return stat, nil
}

func securePrivateReadDir(parent int) ([]os.DirEntry, error) {
	fd, err := unix.Dup(parent)
	if err != nil {
		return nil, err
	}
	if _, err := unix.Seek(fd, 0, io.SeekStart); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	directory := os.NewFile(uintptr(fd), "private-authority-directory")
	if directory == nil {
		_ = unix.Close(fd)
		return nil, errors.New("private authority directory handle is invalid")
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	return entries, errors.Join(readErr, closeErr)
}

func privateStatRegular(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Mode&0o077 == 0
}
