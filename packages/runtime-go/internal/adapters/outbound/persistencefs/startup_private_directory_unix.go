//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type startupPrivateDirectory struct {
	fd       int
	identity string
}

func secureStartupOpenRootDirectory(root startupAuthorityRoot) (*startupPrivateDirectory, error) {
	fd, err := root.open()
	if err != nil {
		return nil, err
	}
	identity, err := unixOpenedDirectoryIdentity(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &startupPrivateDirectory{fd: fd, identity: identity}, nil
}

func secureStartupCreateDirectory(root startupAuthorityRoot, name string) (*startupPrivateDirectory, error) {
	parent, err := root.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)
	return secureStartupCreateDirectoryAt(parent, name)
}

func secureStartupCreateDirectoryAt(parent int, name string) (*startupPrivateDirectory, error) {
	if !startupAuthorityNamedComponent(name) {
		return nil, errors.New("startup private directory name is invalid")
	}
	if err := unix.Mkdirat(parent, name, 0o700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, os.ErrExist
		}
		return nil, err
	}
	if err := unix.Fsync(parent); err != nil {
		return nil, err
	}
	directory, err := secureStartupOpenDirectoryAt(parent, name, "")
	if err != nil {
		return nil, err
	}
	if err := unix.Fsync(directory.fd); err != nil {
		_ = directory.Close()
		return nil, err
	}
	return directory, nil
}

func secureStartupOpenDirectory(root startupAuthorityRoot, name, expectedIdentity string) (*startupPrivateDirectory, error) {
	parent, err := root.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)
	return secureStartupOpenDirectoryAt(parent, name, expectedIdentity)
}

func secureStartupOpenDirectoryAt(parent int, name, expectedIdentity string) (*startupPrivateDirectory, error) {
	if !startupAuthorityNamedComponent(name) {
		return nil, errors.New("startup private directory name is invalid")
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return nil, errors.New("startup private directory is unsafe")
	}
	identity := unixDirectoryIdentity(stat)
	if expectedIdentity != "" && identity != expectedIdentity {
		_ = unix.Close(fd)
		return nil, errors.New("startup private directory identity changed")
	}
	return &startupPrivateDirectory{fd: fd, identity: identity}, nil
}

func (directory *startupPrivateDirectory) OpenDirectory(name, expectedIdentity string) (*startupPrivateDirectory, error) {
	if directory == nil || directory.fd < 0 {
		return nil, errors.New("startup private directory is unavailable")
	}
	return secureStartupOpenDirectoryAt(directory.fd, name, expectedIdentity)
}

func (directory *startupPrivateDirectory) CreateDirectory(name string) (*startupPrivateDirectory, error) {
	if directory == nil || directory.fd < 0 {
		return nil, errors.New("startup private directory is unavailable")
	}
	return secureStartupCreateDirectoryAt(directory.fd, name)
}

func (directory *startupPrivateDirectory) Close() error {
	if directory == nil || directory.fd < 0 {
		return nil
	}
	fd := directory.fd
	directory.fd = -1
	return unix.Close(fd)
}

func (directory *startupPrivateDirectory) Identity() string {
	if directory == nil {
		return ""
	}
	return directory.identity
}

func (directory *startupPrivateDirectory) ReadFile(name string, maxBytes int64, allowEmpty bool) ([]byte, string, error) {
	if directory == nil || directory.fd < 0 {
		return nil, "", errors.New("startup private directory is unavailable")
	}
	body, identity, err := secureStartupAuthorityReadAt(directory.fd, name, maxBytes)
	if allowEmpty && err != nil {
		fd, openErr := unix.Openat(directory.fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return nil, "", err
		}
		file := os.NewFile(uintptr(fd), name)
		if file == nil {
			_ = unix.Close(fd)
			return nil, "", errors.New("startup private empty file handle is invalid")
		}
		defer file.Close()
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o077 != 0 || stat.Nlink != 1 || stat.Size != 0 {
			return nil, "", errors.New("startup private empty file is unsafe")
		}
		return []byte{}, formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino)), nil
	}
	return body, identity, err
}

func (directory *startupPrivateDirectory) WriteExclusive(name string, body []byte, maxBytes int64) error {
	if directory == nil || directory.fd < 0 {
		return errors.New("startup private directory is unavailable")
	}
	temporary, err := startupPrivateTempName(name)
	if err != nil {
		return err
	}
	return secureStartupWriteExclusiveAt(directory.fd, name, temporary, body, maxBytes)
}

func (directory *startupPrivateDirectory) writeExclusiveWithTemporaryName(
	name string,
	temporary string,
	body []byte,
	maxBytes int64,
) error {
	if directory == nil || directory.fd < 0 {
		return errors.New("startup private directory is unavailable")
	}
	return secureStartupWriteExclusiveAt(directory.fd, name, temporary, body, maxBytes)
}

func secureStartupWriteExclusiveAt(parent int, name, temporary string, body []byte, maxBytes int64) error {
	if !startupAuthorityNamedComponent(name) || !startupAuthorityNamedComponent(temporary) || name == temporary ||
		maxBytes <= 0 || int64(len(body)) > maxBytes {
		return errors.New("startup private exclusive write input is invalid")
	}
	fd, err := unix.Openat(parent, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("startup private temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = unix.Unlinkat(parent, temporary, 0)
		}
	}()
	if err := startupPrivateWriteAndVerify(file, body, maxBytes); err != nil {
		return err
	}
	if err := secureStartupAuthorityCommitNoReplace(parent, temporary, name); err != nil {
		return err
	}
	tempExists = false
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	borrowed := &startupPrivateDirectory{fd: parent}
	written, _, err := borrowed.ReadFile(name, maxBytes, true)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup private exclusive write readback failed")
	}
	return nil
}

func (directory *startupPrivateDirectory) readStartupExclusiveWriteResidue(name string, maxBytes int64) ([]byte, string, error) {
	if directory == nil || directory.fd < 0 {
		return nil, "", errors.New("startup private directory is unavailable")
	}
	return secureStartupAuthorityReadAt(directory.fd, name, maxBytes)
}

func (directory *startupPrivateDirectory) removeStartupExclusiveWriteResidue(name, expectedIdentity string) error {
	if directory == nil || directory.fd < 0 || !startupAuthorityNamedComponent(name) || expectedIdentity == "" {
		return errors.New("startup private exclusive-write residue deletion input is invalid")
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(directory.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o077 != 0 || stat.Nlink != 1 ||
		formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino)) != expectedIdentity {
		return errors.New("startup private exclusive-write residue identity changed")
	}
	return unix.Unlinkat(directory.fd, name, 0)
}

func (directory *startupPrivateDirectory) syncStartupExclusiveWriteResidues() error {
	if directory == nil || directory.fd < 0 {
		return errors.New("startup private directory is unavailable")
	}
	return unix.Fsync(directory.fd)
}

func (directory *startupPrivateDirectory) WriteReplace(name string, body []byte, maxBytes int64, fault func(string) error) error {
	if directory == nil || directory.fd < 0 || !startupAuthorityNamedComponent(name) || len(body) == 0 || int64(len(body)) > maxBytes {
		return errors.New("startup private replacement input is invalid")
	}
	_, previousIdentity, previousErr := secureStartupAuthorityReadAt(directory.fd, name, maxBytes)
	previousExists := previousErr == nil
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return errors.New("startup private replacement target is unsafe")
	}
	temporary, err := startupPrivateRandomName(semanticJournalTempPrefix)
	if err != nil {
		return err
	}
	temporary += semanticJournalTempSuffix
	fd, err := unix.Openat(directory.fd, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("startup private replacement handle is invalid")
	}
	preserve := false
	defer func() {
		_ = file.Close()
		if !preserve {
			_ = unix.Unlinkat(directory.fd, temporary, 0)
		}
	}()
	if err := startupPrivateWriteAndVerify(file, body, maxBytes); err != nil {
		return err
	}
	if fault != nil {
		if err := fault("after_journal_temp_sync"); err != nil {
			preserve = true
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, currentIdentity, currentErr := secureStartupAuthorityReadAt(directory.fd, name, maxBytes)
	if previousExists {
		if currentErr != nil || currentIdentity != previousIdentity {
			return errors.New("startup private replacement target changed before commit")
		}
		if err := unix.Renameat(directory.fd, temporary, directory.fd, name); err != nil {
			return err
		}
	} else {
		if !errors.Is(currentErr, os.ErrNotExist) {
			return errors.New("startup private replacement target appeared before commit")
		}
		if err := secureStartupAuthorityCommitNoReplace(directory.fd, temporary, name); err != nil {
			return err
		}
	}
	preserve = true
	if fault != nil {
		if err := fault("after_journal_replace"); err != nil {
			return err
		}
	}
	if err := unix.Fsync(directory.fd); err != nil {
		return err
	}
	written, _, err := secureStartupAuthorityReadAt(directory.fd, name, maxBytes)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup private replacement readback failed")
	}
	return nil
}

func (directory *startupPrivateDirectory) Entries() ([]os.DirEntry, error) {
	return directory.ReadEntriesBounded(maxStartupPrivateEntries)
}

func (directory *startupPrivateDirectory) ReadEntriesBounded(limit int) ([]os.DirEntry, error) {
	return directory.ReadEntriesBoundedContext(context.Background(), limit)
}

func (directory *startupPrivateDirectory) ReadEntriesBoundedContext(ctx context.Context, limit int) ([]os.DirEntry, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if directory == nil || directory.fd < 0 {
		return nil, errors.New("startup private directory is unavailable")
	}
	if limit < 0 || limit > maxStartupPrivateEntries {
		return nil, errors.New("startup private directory entry limit is invalid")
	}
	duplicate, err := unix.Openat(directory.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	identity, err := unixOpenedDirectoryIdentity(duplicate)
	if err != nil || identity != directory.identity {
		_ = unix.Close(duplicate)
		return nil, errors.New("startup private directory changed before inventory")
	}
	file := os.NewFile(uintptr(duplicate), "startup-private-directory")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, errors.New("startup private directory duplicate is invalid")
	}
	entries := make([]os.DirEntry, 0, min(limit, startupPrivateEntryPage))
	for {
		if err := contextError(ctx); err != nil {
			_ = file.Close()
			return nil, err
		}
		page, readErr := file.ReadDir(startupPrivateEntryPage)
		if len(entries)+len(page) > limit {
			_ = file.Close()
			return nil, StartupResourceLimitError{Code: "private_directory_entries", Limit: int64(limit)}
		}
		entries = append(entries, page...)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return nil, readErr
		}
	}
	closeErr := file.Close()
	if closeErr != nil {
		return nil, closeErr
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	current, err := unix.Openat(directory.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("startup private directory changed after inventory")
	}
	currentIdentity, identityErr := unixOpenedDirectoryIdentity(current)
	closeErr = unix.Close(current)
	if identityErr != nil || closeErr != nil || currentIdentity != directory.identity {
		return nil, errors.New("startup private directory changed after inventory")
	}
	return entries, nil
}

func (directory *startupPrivateDirectory) PreflightTree() error {
	return directory.PreflightTreeContext(context.Background())
}

func (directory *startupPrivateDirectory) PreflightTreeContext(ctx context.Context) error {
	if directory == nil || directory.fd < 0 {
		return errors.New("startup private directory is unavailable")
	}
	count := 0
	return preflightUnixPrivateTree(ctx, directory.fd, 0, &count)
}

func secureStartupRetireDirectory(root startupAuthorityRoot, name, expectedIdentity, prefix string) (string, *startupPrivateDirectory, error) {
	if !startupAuthorityNamedComponent(name) || !strings.HasPrefix(prefix, ".retired-") {
		return "", nil, errors.New("startup private retirement input is invalid")
	}
	parent, err := root.open()
	if err != nil {
		return "", nil, err
	}
	defer unix.Close(parent)
	return secureStartupRetireDirectoryAt(parent, name, expectedIdentity, prefix)
}

func (directory *startupPrivateDirectory) RetireDirectory(name, expectedIdentity, prefix string) (string, *startupPrivateDirectory, error) {
	if directory == nil || directory.fd < 0 {
		return "", nil, errors.New("startup private directory is unavailable")
	}
	return secureStartupRetireDirectoryAt(directory.fd, name, expectedIdentity, prefix)
}

func secureStartupRetireDirectoryAt(parent int, name, expectedIdentity, prefix string) (string, *startupPrivateDirectory, error) {
	current, err := secureStartupOpenDirectoryAt(parent, name, expectedIdentity)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	_ = current.Close()
	retired, err := startupPrivateRandomName(prefix)
	if err != nil {
		return "", nil, err
	}
	if err := secureStartupAuthorityCommitNoReplace(parent, name, retired); err != nil {
		return "", nil, err
	}
	if err := unix.Fsync(parent); err != nil {
		return "", nil, err
	}
	opened, err := secureStartupOpenDirectoryAt(parent, retired, expectedIdentity)
	if err != nil {
		return "", nil, err
	}
	return retired, opened, nil
}

func secureStartupRemoveRetiredDirectory(root startupAuthorityRoot, name string, directory *startupPrivateDirectory) error {
	return secureStartupRemoveRetiredDirectoryContext(context.Background(), root, name, directory)
}

func secureStartupRemoveRetiredDirectoryContext(ctx context.Context, root startupAuthorityRoot, name string, directory *startupPrivateDirectory) error {
	if directory == nil || directory.fd < 0 || !strings.HasPrefix(name, ".retired-") {
		return errors.New("startup retired directory input is invalid")
	}
	parent, err := root.open()
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	return secureStartupRemoveRetiredDirectoryAtContext(ctx, parent, name, directory)
}

func (directory *startupPrivateDirectory) RemoveRetiredDirectory(name string, retired *startupPrivateDirectory) error {
	return directory.RemoveRetiredDirectoryContext(context.Background(), name, retired)
}

func (directory *startupPrivateDirectory) RemoveRetiredDirectoryContext(ctx context.Context, name string, retired *startupPrivateDirectory) error {
	if directory == nil || directory.fd < 0 {
		return errors.New("startup private directory is unavailable")
	}
	return secureStartupRemoveRetiredDirectoryAtContext(ctx, directory.fd, name, retired)
}

func secureStartupRemoveRetiredDirectoryAt(parent int, name string, directory *startupPrivateDirectory) error {
	return secureStartupRemoveRetiredDirectoryAtContext(context.Background(), parent, name, directory)
}

func secureStartupRemoveRetiredDirectoryAtContext(ctx context.Context, parent int, name string, directory *startupPrivateDirectory) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	opened, err := secureStartupOpenDirectoryAt(parent, name, directory.identity)
	if err != nil {
		return err
	}
	defer opened.Close()
	if err := secureRemoveUnixDirectoryContents(ctx, opened.fd); err != nil {
		return err
	}
	if err := opened.Close(); err != nil {
		return err
	}
	if err := unix.Unlinkat(parent, name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(parent)
}

func startupPrivateWriteAndVerify(file *os.File, body []byte, maxBytes int64) error {
	for written := 0; written < len(body); {
		count, err := file.Write(body[written:])
		if err != nil {
			return err
		}
		if count <= 0 {
			return errors.New("startup private write was incomplete")
		}
		written += count
	}
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	written, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup private staged write verification failed")
	}
	return nil
}

func startupPrivateTempName(name string) (string, error) {
	random, err := startupPrivateRandomName("")
	if err != nil {
		return "", err
	}
	return "." + name + "-" + random + ".tmp", nil
}

func startupPrivateRandomName(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}

var _ = filepath.Base
