//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"sort"

	"golang.org/x/sys/unix"
)

func platformOpenSeparateOwnerRootDirectory(
	authority *SeparateOwnerRootAuthority,
	validate func() error,
) (*startupPrivateDirectory, SeparateOwnerDirectoryState, error) {
	binding, ok := authority.rootBinding()
	if !ok {
		return nil, SeparateOwnerDirectoryState{}, errors.New("separate-owner Unix root binding is unavailable")
	}
	opened, err := authority.openPinnedRoot()
	if err != nil {
		return nil, SeparateOwnerDirectoryState{}, err
	}
	state, err := separateOwnerUnixDirectoryState(opened.fd, false)
	if err != nil || state.Identity != binding.Identity || state.SecurityDigest != binding.SecurityDigest || validate() != nil {
		_ = opened.Close()
		return nil, SeparateOwnerDirectoryState{}, errors.New("separate-owner Unix root changed while opening")
	}
	return opened, state, nil
}

func platformValidateSeparateOwnerOpenedDirectory(
	directory *startupPrivateDirectory,
	expected SeparateOwnerDirectoryState,
	requireProtected bool,
) error {
	if directory == nil || directory.fd < 0 {
		return errors.New("separate-owner Unix directory is unavailable")
	}
	current, err := separateOwnerUnixDirectoryState(directory.fd, requireProtected)
	if err != nil || current != expected {
		return errors.New("separate-owner Unix directory identity or permissions changed")
	}
	return nil
}

func platformValidateSeparateOwnerChildDirectory(
	parent *startupPrivateDirectory,
	name string,
	expectedIdentity string,
) error {
	if parent == nil || parent.fd < 0 || !startupAuthorityNamedComponent(name) || expectedIdentity == "" {
		return errors.New("separate-owner Unix child directory binding is invalid")
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(parent.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		stat.Mode&unix.S_IFMT != unix.S_IFDIR || unixDirectoryIdentity(stat) != expectedIdentity {
		return errors.New("separate-owner Unix child directory name changed")
	}
	return nil
}

func separateOwnerUnixDirectoryState(fd int, requirePrivate bool) (SeparateOwnerDirectoryState, error) {
	before, err := captureSeparateOwnerUnixDirectoryState(fd, requirePrivate)
	if err != nil {
		return SeparateOwnerDirectoryState{}, err
	}
	after, err := captureSeparateOwnerUnixDirectoryState(fd, requirePrivate)
	if err != nil || after != before {
		return SeparateOwnerDirectoryState{}, errors.New("separate-owner Unix directory security changed while reading")
	}
	return after, nil
}

func captureSeparateOwnerUnixDirectoryState(fd int, requirePrivate bool) (SeparateOwnerDirectoryState, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		int(stat.Uid) != os.Geteuid() || stat.Mode&0o022 != 0 || stat.Mode&0o7000 != 0 ||
		requirePrivate && stat.Mode&0o777 != 0o700 {
		return SeparateOwnerDirectoryState{}, errors.New("separate-owner Unix directory owner or permissions are unsafe")
	}
	securityDigest, err := separateOwnerUnixSecurityDigest(fd, stat, "directory", separateOwnerObjectScope)
	if err != nil {
		return SeparateOwnerDirectoryState{}, err
	}
	return SeparateOwnerDirectoryState{
		Identity: unixDirectoryIdentity(stat), SecurityDigest: securityDigest,
		Protected: stat.Mode&0o777 == 0o700,
	}, nil
}

func (directory *SeparateOwnerDirectory) OpenDirectory(
	ctx context.Context,
	name string,
	requireProtected bool,
) (*SeparateOwnerDirectory, bool, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return nil, false, err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) {
		return nil, false, errors.New("separate-owner Unix directory open authority is invalid")
	}
	opened, err := directory.directory.OpenDirectory(name, "")
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	state, err := separateOwnerUnixDirectoryState(opened.fd, requireProtected)
	if err != nil || directory.Validate() != nil ||
		platformValidateSeparateOwnerChildDirectory(directory.directory, name, state.Identity) != nil {
		_ = opened.Close()
		return nil, false, errors.New("separate-owner Unix directory changed while opening")
	}
	return &SeparateOwnerDirectory{
		directory: opened, state: state, requireProtected: requireProtected, validateLease: directory.validateLease,
		parent: directory, name: name,
	}, true, nil
}

func (directory *SeparateOwnerDirectory) CreatePrivateDirectory(
	ctx context.Context,
	name string,
) (*SeparateOwnerDirectory, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return nil, err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) {
		return nil, errors.New("separate-owner Unix directory creation authority is invalid")
	}
	created, err := directory.directory.CreateDirectory(name)
	if err != nil {
		return nil, err
	}
	state, err := separateOwnerUnixDirectoryState(created.fd, true)
	if err != nil || directory.Validate() != nil ||
		platformValidateSeparateOwnerChildDirectory(directory.directory, name, state.Identity) != nil {
		_ = created.Close()
		return nil, errors.New("separate-owner Unix private directory creation failed verification")
	}
	return &SeparateOwnerDirectory{
		directory: created, state: state, requireProtected: true, validateLease: directory.validateLease,
		parent: directory, name: name,
	}, nil
}

func (directory *SeparateOwnerDirectory) CaptureFile(
	ctx context.Context,
	name string,
	maxBytes int64,
	allowEmpty bool,
	requireProtected bool,
) (SeparateOwnerFileState, []byte, bool, error) {
	return directory.captureFile(ctx, name, maxBytes, allowEmpty, requireProtected, nil)
}

func (directory *SeparateOwnerDirectory) captureFile(
	ctx context.Context,
	name string,
	maxBytes int64,
	allowEmpty bool,
	requireProtected bool,
	fault SeparateOwnerWriteFault,
) (SeparateOwnerFileState, []byte, bool, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return SeparateOwnerFileState{}, nil, false, err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) || maxBytes <= 0 {
		return SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Unix file read authority is invalid")
	}
	fd, err := unix.Openat(directory.directory.fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return SeparateOwnerFileState{}, nil, false, nil
	}
	if err != nil {
		return SeparateOwnerFileState{}, nil, false, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Unix file handle is invalid")
	}
	defer file.Close()
	before, err := separateOwnerUnixFileState(fd, file, maxBytes, allowEmpty, requireProtected, nil)
	if err != nil {
		return SeparateOwnerFileState{}, nil, false, err
	}
	hasher := sha256.New()
	buffer := bytes.NewBuffer(make([]byte, 0, min(int(before.Size), 64*1024)))
	written, err := io.Copy(io.MultiWriter(hasher, buffer), &separateOwnerContextReader{ctx: ctx, reader: io.LimitReader(file, maxBytes+1)})
	if err != nil || written != before.Size {
		return SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Unix file changed while reading")
	}
	body := buffer.Bytes()
	after, err := separateOwnerUnixFileState(fd, file, maxBytes, allowEmpty, requireProtected, hasher.Sum(nil))
	before.SHA256 = after.SHA256
	if err != nil || after != before {
		clear(body)
		return SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Unix file identity changed while reading")
	}
	if err := callSeparateOwnerFault(fault, "before_final_name_check"); err != nil {
		clear(body)
		return SeparateOwnerFileState{}, nil, false, err
	}
	if err := directory.Validate(); err != nil {
		clear(body)
		return SeparateOwnerFileState{}, nil, false, err
	}
	if err := separateOwnerUnixRequireFileNameMapping(directory.directory.fd, name, after.Identity); err != nil {
		clear(body)
		return SeparateOwnerFileState{}, nil, false, err
	}
	final, err := separateOwnerUnixFileState(fd, file, maxBytes, allowEmpty, requireProtected, hasher.Sum(nil))
	if err != nil || final != after || directory.Validate() != nil ||
		separateOwnerUnixRequireFileNameMapping(directory.directory.fd, name, after.Identity) != nil {
		clear(body)
		return SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Unix file name changed after reading")
	}
	return final, body, true, nil
}

func separateOwnerUnixRequireFileNameMapping(parent int, name string, expectedIdentity string) error {
	var mapped unix.Stat_t
	if parent < 0 || !startupAuthorityNamedComponent(name) || expectedIdentity == "" ||
		unix.Fstatat(parent, name, &mapped, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		mapped.Mode&unix.S_IFMT != unix.S_IFREG || mapped.Nlink != 1 ||
		formatUnixObjectIdentity(uint64(mapped.Dev), uint64(mapped.Ino)) != expectedIdentity {
		return errors.New("separate-owner Unix file name no longer maps to the verified object")
	}
	return nil
}

// ProjectProtectedFileStateExact computes the one canonical security state
// that ProtectFileExact may produce without mutating the verified file.
func (directory *SeparateOwnerDirectory) ProjectProtectedFileStateExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
) (SeparateOwnerFileState, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return SeparateOwnerFileState{}, err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) || !expected.Valid() {
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix file protection projection authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	current, _, present, err := directory.CaptureFile(ctx, name, maxBytes, expected.Size == 0, false)
	if err != nil || !present || current != expected {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Unix file changed before protection projection"), err)
	}
	fd, err := unix.Openat(directory.directory.fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix protection projection handle is invalid")
	}
	defer file.Close()
	before, err := separateOwnerUnixFileState(fd, file, maxBytes, expected.Size == 0, false, nil)
	before.SHA256 = expected.SHA256
	var stat unix.Stat_t
	if err != nil || before != expected || unix.Fstat(fd, &stat) != nil ||
		separateOwnerUnixRequireFileNameMapping(directory.directory.fd, name, expected.Identity) != nil {
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix file changed during protection projection")
	}
	projectedStat := stat
	projectedStat.Mode &^= 0o7777
	projectedStat.Mode |= 0o600
	securityDigest, err := separateOwnerUnixSecurityDigest(
		fd, projectedStat, "file", separateOwnerObjectScope,
	)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	projected := expected
	projected.SecurityDigest = securityDigest
	final, err := separateOwnerUnixFileState(fd, file, maxBytes, expected.Size == 0, false, nil)
	final.SHA256 = expected.SHA256
	if err != nil || final != expected || directory.Validate() != nil ||
		separateOwnerUnixRequireFileNameMapping(directory.directory.fd, name, expected.Identity) != nil {
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix file changed after protection projection")
	}
	return projected, nil
}

// ProtectFileExact narrows one already-verified owner file to host-private
// custody without changing its identity or contents. The returned state must
// exactly match the projection authenticated by the migration journal.
func (directory *SeparateOwnerDirectory) ProtectFileExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
) (SeparateOwnerFileState, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return SeparateOwnerFileState{}, err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) || !expected.Valid() {
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix file protection authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	current, _, present, err := directory.CaptureFile(ctx, name, maxBytes, expected.Size == 0, false)
	if err != nil || !present || current != expected {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Unix file changed before protection"), err)
	}
	fd, err := unix.Openat(directory.directory.fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix protection handle is invalid")
	}
	defer file.Close()
	before, err := separateOwnerUnixFileState(fd, file, maxBytes, expected.Size == 0, false, nil)
	before.SHA256 = expected.SHA256
	var mapped unix.Stat_t
	if err != nil || before != expected || unix.Fstatat(directory.directory.fd, name, &mapped, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		formatUnixObjectIdentity(uint64(mapped.Dev), uint64(mapped.Ino)) != expected.Identity || mapped.Nlink != 1 {
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix file changed while opening for protection")
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return SeparateOwnerFileState{}, err
	}
	if err := errors.Join(unix.Fsync(fd), unix.Fsync(directory.directory.fd)); err != nil {
		return SeparateOwnerFileState{}, err
	}
	after, err := separateOwnerUnixFileState(fd, file, maxBytes, expected.Size == 0, true, nil)
	after.SHA256 = expected.SHA256
	if err != nil || after.Identity != expected.Identity || after.Size != expected.Size ||
		after.ModifiedUnixNano != expected.ModifiedUnixNano || directory.Validate() != nil || contextSeparateOwnerError(ctx) != nil {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Unix protected file state changed"), err)
	}
	protected, _, present, err := directory.CaptureFile(ctx, name, maxBytes, expected.Size == 0, true)
	if err != nil || !present || protected != after || protected.SHA256 != expected.SHA256 {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Unix protected file readback failed"), err)
	}
	return protected, nil
}

func separateOwnerUnixFileState(
	fd int,
	file *os.File,
	maxBytes int64,
	allowEmpty bool,
	requireProtected bool,
	digest []byte,
) (SeparateOwnerFileState, error) {
	var stat unix.Stat_t
	info, infoErr := file.Stat()
	if err := unix.Fstat(fd, &stat); err != nil || infoErr != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Nlink != 1 || int(stat.Uid) != os.Geteuid() || stat.Mode&0o022 != 0 || stat.Mode&0o7000 != 0 ||
		requireProtected && stat.Mode&0o777 != 0o600 || stat.Size < 0 || stat.Size > maxBytes || !allowEmpty && stat.Size == 0 {
		return SeparateOwnerFileState{}, errors.New("separate-owner Unix file owner, links, or permissions are unsafe")
	}
	securityDigest, err := separateOwnerUnixSecurityDigest(fd, stat, "file", separateOwnerObjectScope)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	sha := ""
	if digest != nil {
		sha = hex.EncodeToString(digest)
	}
	return SeparateOwnerFileState{
		Identity:       formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino)),
		SecurityDigest: securityDigest,
		Size:           stat.Size, ModifiedUnixNano: info.ModTime().UnixNano(), SHA256: sha,
	}, nil
}

func (directory *SeparateOwnerDirectory) WriteExclusiveAtomic(
	ctx context.Context,
	name string,
	body []byte,
	maxBytes int64,
	fault SeparateOwnerWriteFault,
) error {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || !directory.requireProtected || !startupAuthorityNamedComponent(name) ||
		len(body) == 0 || int64(len(body)) > maxBytes {
		return errors.New("separate-owner Unix atomic write authority is invalid")
	}
	temporary, err := startupPrivateTempName(name)
	if err != nil {
		return err
	}
	fd, err := unix.Openat(directory.directory.fd, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("separate-owner Unix atomic temp handle is invalid")
	}
	preserve := false
	defer func() {
		_ = file.Close()
		if !preserve {
			_ = unix.Unlinkat(directory.directory.fd, temporary, 0)
		}
	}()
	if err := startupPrivateWriteAndVerify(file, body, maxBytes); err != nil {
		return err
	}
	if fault != nil {
		if err := fault("after_temp_sync"); err != nil {
			preserve = true
			return err
		}
	}
	if err := directory.Validate(); err != nil {
		return err
	}
	if err := secureStartupAuthorityCommitNoReplace(directory.directory.fd, temporary, name); err != nil {
		return err
	}
	preserve = true
	if fault != nil {
		if err := fault("after_publish"); err != nil {
			return err
		}
	}
	if err := unix.Fsync(directory.directory.fd); err != nil {
		return err
	}
	if fault != nil {
		if err := fault("after_directory_sync"); err != nil {
			return err
		}
	}
	written, _, present, err := directory.CaptureFile(ctx, name, maxBytes, false, true)
	if err != nil || !present || written.SHA256 != separateOwnerSHA256(body) || written.Size != int64(len(body)) {
		return errors.New("separate-owner Unix atomic write readback failed")
	}
	return nil
}

func (directory *SeparateOwnerDirectory) MoveFileNoReplace(
	ctx context.Context,
	name string,
	destination *SeparateOwnerDirectory,
	destinationName string,
	expected SeparateOwnerFileState,
	requireProtected bool,
) error {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || destination == nil || destination.Validate() != nil ||
		!startupAuthorityNamedComponent(name) || !startupAuthorityNamedComponent(destinationName) || !expected.Valid() {
		return errors.New("separate-owner Unix move authority is invalid")
	}
	current, _, present, err := directory.CaptureFile(ctx, name, expected.Size+1, expected.Size == 0, requireProtected)
	if err != nil || !present || current != expected {
		return errors.New("separate-owner Unix move source changed")
	}
	if err := platformSeparateOwnerRenameNoReplace(directory.directory.fd, name, destination.directory.fd, destinationName); err != nil {
		return err
	}
	if err := errors.Join(unix.Fsync(directory.directory.fd), unix.Fsync(destination.directory.fd)); err != nil {
		return err
	}
	moved, _, present, err := destination.CaptureFile(ctx, destinationName, expected.Size+1, expected.Size == 0, requireProtected)
	if err != nil || !present || moved != expected {
		rollbackErr := platformSeparateOwnerRenameNoReplace(destination.directory.fd, destinationName, directory.directory.fd, name)
		return errors.Join(errors.New("separate-owner Unix moved object identity changed"), rollbackErr)
	}
	return nil
}

func (directory *SeparateOwnerDirectory) RemoveFileExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
	requireProtected bool,
) error {
	return directory.removeFileExact(ctx, name, expected, requireProtected, nil)
}

func (directory *SeparateOwnerDirectory) removeFileExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
	requireProtected bool,
	fault SeparateOwnerWriteFault,
) (resultErr error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) || !expected.Valid() {
		return errors.New("separate-owner Unix remove authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	fd, err := unix.Openat(directory.directory.fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("separate-owner Unix remove handle is invalid")
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	hashFile := func() (string, error) {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		hasher := sha256.New()
		limit := maxBytes
		if limit < math.MaxInt64 {
			limit++
		}
		written, err := io.Copy(hasher, &separateOwnerContextReader{
			ctx: ctx, reader: io.LimitReader(file, limit),
		})
		if err != nil || written != expected.Size {
			return "", errors.Join(errors.New("separate-owner Unix remove target changed while hashing"), err)
		}
		return hex.EncodeToString(hasher.Sum(nil)), nil
	}
	digest, err := hashFile()
	if err != nil {
		return err
	}
	current, err := separateOwnerUnixFileState(fd, file, maxBytes, expected.Size == 0, requireProtected, nil)
	current.SHA256 = digest
	if err != nil || current != expected {
		return errors.Join(errors.New("separate-owner Unix remove target changed"), err)
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(directory.directory.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		formatUnixObjectIdentity(uint64(stat.Dev), uint64(stat.Ino)) != expected.Identity || stat.Nlink != 1 {
		return errors.New("separate-owner Unix remove target identity changed")
	}
	if err := directory.Validate(); err != nil {
		return err
	}
	if err := callSeparateOwnerFault(fault, "before_unlink"); err != nil {
		return err
	}
	current, err = separateOwnerUnixFileState(fd, file, maxBytes, expected.Size == 0, requireProtected, nil)
	current.SHA256 = expected.SHA256
	if err != nil || current != expected || directory.Validate() != nil ||
		separateOwnerUnixRequireFileNameMapping(directory.directory.fd, name, expected.Identity) != nil {
		return errors.New("separate-owner Unix remove target changed in the final window")
	}
	if err := platformSeparateOwnerUnlinkFile(directory.directory.fd, name); err != nil {
		return err
	}
	if err := unix.Fsync(directory.directory.fd); err != nil {
		return err
	}
	if err := callSeparateOwnerFault(fault, "after_unlink"); err != nil {
		return err
	}
	var after unix.Stat_t
	info, infoErr := file.Stat()
	if err := unix.Fstat(fd, &after); err != nil || infoErr != nil || after.Mode&unix.S_IFMT != unix.S_IFREG ||
		after.Nlink != 0 || int(after.Uid) != os.Geteuid() || after.Mode&0o022 != 0 || after.Mode&0o7000 != 0 ||
		requireProtected && after.Mode&0o777 != 0o600 || after.Size != expected.Size ||
		formatUnixObjectIdentity(uint64(after.Dev), uint64(after.Ino)) != expected.Identity ||
		info.ModTime().UnixNano() != expected.ModifiedUnixNano {
		return errors.New("separate-owner Unix removed object retained a link or changed")
	}
	afterDigest, err := hashFile()
	if err != nil || afterDigest != expected.SHA256 || directory.Validate() != nil || contextSeparateOwnerError(ctx) != nil {
		return errors.Join(errors.New("separate-owner Unix removed object content changed"), err)
	}
	return nil
}

func (directory *SeparateOwnerDirectory) RemoveEmptyDirectoryExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerDirectoryState,
) error {
	return directory.removeEmptyDirectoryExact(ctx, name, expected, nil)
}

func (directory *SeparateOwnerDirectory) removeEmptyDirectoryExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerDirectoryState,
	fault SeparateOwnerWriteFault,
) error {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || !startupAuthorityNamedComponent(name) || !expected.Valid() {
		return errors.New("separate-owner Unix directory removal authority is invalid")
	}
	opened, present, err := directory.OpenDirectory(ctx, name, expected.Protected)
	if err != nil || !present || opened.state != expected {
		return errors.New("separate-owner Unix directory removal target changed")
	}
	defer opened.close()
	entries, err := opened.Entries(ctx, 1)
	if err != nil || len(entries) != 0 {
		return errors.Join(errors.New("separate-owner Unix directory is not empty"), err)
	}
	if err := callSeparateOwnerFault(fault, "before_rmdir"); err != nil {
		return err
	}
	if err := opened.Validate(); err != nil || directory.Validate() != nil ||
		platformValidateSeparateOwnerChildDirectory(directory.directory, name, expected.Identity) != nil {
		return errors.New("separate-owner Unix directory identity changed before removal")
	}
	if err := unix.Unlinkat(directory.directory.fd, name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(directory.directory.fd)
}

func (directory *SeparateOwnerDirectory) Sync() error {
	if err := directory.Validate(); err != nil {
		return err
	}
	return unix.Fsync(directory.directory.fd)
}

func (directory *SeparateOwnerDirectory) RemoveKnownTempFiles(
	ctx context.Context,
	targets map[string]SeparateOwnerFileState,
) error {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := directory.RemoveFileExact(ctx, name, targets[name], true); err != nil {
			return err
		}
	}
	return nil
}

type separateOwnerContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *separateOwnerContextReader) Read(body []byte) (int, error) {
	if err := contextSeparateOwnerError(reader.ctx); err != nil {
		return 0, err
	}
	return reader.reader.Read(body)
}
