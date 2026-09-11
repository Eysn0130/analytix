//go:build darwin

package fundsquerysource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

const (
	darwinExactDestinationMinimumFDV1 = 8
	darwinExactCopyChunkBytesV1       = 256 << 10
)

type darwinFileIdentityV1 struct {
	dev       uint64
	ino       uint64
	size      int64
	mode      uint16
	nlink     uint64
	uid       uint32
	flags     uint32
	ctimeSec  int64
	ctimeNsec int64
}

type darwinDirectoryIdentityV1 struct {
	dev       uint64
	ino       uint64
	mode      uint16
	nlink     uint64
	uid       uint32
	flags     uint32
	ctimeSec  int64
	ctimeNsec int64
}

type darwinDestinationFilesystemIdentityV1 struct {
	fsid [2]int32
}

type darwinDestinationSecurityIdentityV1 struct {
	present    bool
	provenance [darwinSourceProvenanceByteLengthV1]byte
}

type darwinDestinationIdentityV1 struct {
	dev        uint64
	ino        uint64
	uid        uint32
	gid        uint32
	mode       uint16
	nlink      uint64
	flags      uint32
	filesystem darwinDestinationFilesystemIdentityV1
	security   darwinDestinationSecurityIdentityV1
}

type exactReadLeaseV1 struct {
	mu        sync.Mutex
	condition *sync.Cond
	active    bool
	copying   bool
	completed bool
	ctx       context.Context
	file      *os.File
	expected  domainfundsquerysource.ImmutableSnapshotObjectV1
	identity  darwinFileIdentityV1
}

var _ fundsquerysourceport.ExactReadLease = (*exactReadLeaseV1)(nil)

func (source *Source) withExact(
	ctx context.Context,
	descriptor domainfundsquerysource.DescriptorV1,
	use func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	object, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		descriptor.CaseID,
		descriptor.DuckDBSHA256,
		descriptor.DuckDBByteLength,
	)
	if err != nil {
		return fundsquerysourceport.ErrMismatch
	}
	return source.withInstalledExact(ctx, object, use)
}

func (source *Source) withInstalledExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	use func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	directoryFD, file, directoryIdentity, fileIdentity, err := source.openExactDarwinV1(object)
	if err != nil {
		return err
	}
	defer unix.Close(directoryFD)
	defer file.Close()

	lease := &exactReadLeaseV1{
		active: true, ctx: ctx, file: file, expected: object, identity: fileIdentity,
	}
	lease.condition = sync.NewCond(&lease.mu)
	useErr := use(ctx, lease)
	lease.close()
	if useErr != nil {
		return useErr
	}
	if !lease.completedExactCopy() {
		return errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query source consumer did not complete one exact copy"),
		)
	}
	if err := verifyDarwinSnapshotAfterUseV1(
		ctx,
		source,
		directoryFD,
		file,
		directoryIdentity,
		fileIdentity,
		object,
	); err != nil {
		return err
	}
	return nil
}

func (source *Source) openExactDarwinV1(
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
) (int, *os.File, darwinDirectoryIdentityV1, darwinFileIdentityV1, error) {
	currentFD, directoryIdentity, err := source.openSnapshotDirectoryDarwinV1(object.CaseID)
	if err != nil {
		return -1, nil, darwinDirectoryIdentityV1{}, darwinFileIdentityV1{}, mapOpenErrorV1(err)
	}

	name := immutableSnapshotObjectFileNameV1(object)
	fileFD, err := unix.Openat(
		currentFD,
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		_ = unix.Close(currentFD)
		return -1, nil, darwinDirectoryIdentityV1{}, darwinFileIdentityV1{}, mapOpenErrorV1(err)
	}
	file := os.NewFile(uintptr(fileFD), "analytix-funds-query-snapshot")
	if file == nil {
		_ = unix.Close(fileFD)
		_ = unix.Close(currentFD)
		return -1, nil, darwinDirectoryIdentityV1{}, darwinFileIdentityV1{}, fundsquerysourceport.ErrUnavailable
	}
	identity, err := exactDarwinFileIdentityV1(fileFD, object)
	if err == nil {
		err = ensureNoDarwinWALV1(currentFD, name)
	}
	if err != nil {
		_ = file.Close()
		_ = unix.Close(currentFD)
		return -1, nil, darwinDirectoryIdentityV1{}, darwinFileIdentityV1{}, err
	}
	return currentFD, file, directoryIdentity, identity, nil
}

func (source *Source) openSnapshotDirectoryDarwinV1(
	caseID string,
) (int, darwinDirectoryIdentityV1, error) {
	if source == nil {
		return -1, darwinDirectoryIdentityV1{}, unix.EINVAL
	}
	currentFD, directoryIdentity, err := openAbsoluteDirectoryNoFollowDarwinV1(source.userDataRoot)
	if err != nil {
		return -1, darwinDirectoryIdentityV1{}, err
	}
	for _, name := range []string{
		dataAnalysisDirectoryV1,
		casesDirectoryV1,
		caseID,
		snapshotsDirectoryV1,
	} {
		nextFD, nextIdentity, openErr := openPrivateDirectoryAtDarwinV1(currentFD, name)
		_ = unix.Close(currentFD)
		if openErr != nil {
			return -1, darwinDirectoryIdentityV1{}, openErr
		}
		currentFD = nextFD
		directoryIdentity = nextIdentity
	}
	return currentFD, directoryIdentity, nil
}

func openAbsoluteDirectoryNoFollowDarwinV1(path string) (int, darwinDirectoryIdentityV1, error) {
	currentFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, darwinDirectoryIdentityV1{}, err
	}
	if !traversedDarwinDirectorySafeV1(currentFD) {
		_ = unix.Close(currentFD)
		return -1, darwinDirectoryIdentityV1{}, unix.EPERM
	}
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." || strings.ContainsRune(component, 0) {
			_ = unix.Close(currentFD)
			return -1, darwinDirectoryIdentityV1{}, unix.EINVAL
		}
		nextFD, openErr := openTraversedDirectoryAtDarwinV1(currentFD, component)
		_ = unix.Close(currentFD)
		if openErr != nil {
			return -1, darwinDirectoryIdentityV1{}, openErr
		}
		currentFD = nextFD
	}
	identity, err := exactDarwinDirectoryIdentityV1(currentFD)
	if err != nil {
		_ = unix.Close(currentFD)
		return -1, darwinDirectoryIdentityV1{}, err
	}
	return currentFD, identity, nil
}

func openTraversedDirectoryAtDarwinV1(parentFD int, name string) (int, error) {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.ContainsRune(name, 0) {
		return -1, unix.EINVAL
	}
	fd, err := unix.Openat(
		parentFD,
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return -1, err
	}
	if !traversedDarwinDirectorySafeV1(fd) {
		_ = unix.Close(fd)
		return -1, unix.EPERM
	}
	return fd, nil
}

func openPrivateDirectoryAtDarwinV1(
	parentFD int,
	name string,
) (int, darwinDirectoryIdentityV1, error) {
	fd, err := openTraversedDirectoryAtDarwinV1(parentFD, name)
	if err != nil {
		return -1, darwinDirectoryIdentityV1{}, err
	}
	identity, err := exactDarwinDirectoryIdentityV1(fd)
	if err != nil {
		_ = unix.Close(fd)
		return -1, darwinDirectoryIdentityV1{}, err
	}
	return fd, identity, nil
}

func traversedDarwinDirectorySafeV1(fd int) bool {
	var stat unix.Stat_t
	return unix.Fstat(fd, &stat) == nil && stat.Mode&unix.S_IFMT == unix.S_IFDIR &&
		(stat.Uid == 0 || stat.Uid == uint32(unix.Geteuid())) &&
		stat.Nlink > 0 && stat.Mode&0o022 == 0
}

func exactDarwinDirectoryIdentityV1(fd int) (darwinDirectoryIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(unix.Geteuid()) || stat.Nlink == 0 ||
		stat.Mode&0o7777 != 0o700 || stat.Flags != 0 {
		return darwinDirectoryIdentityV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query protected directory identity is invalid"),
		)
	}
	identity := darwinDirectoryIdentityV1{
		dev: uint64(stat.Dev), ino: uint64(stat.Ino), mode: stat.Mode,
		nlink: uint64(stat.Nlink), uid: stat.Uid, flags: stat.Flags,
		ctimeSec: stat.Ctim.Sec, ctimeNsec: stat.Ctim.Nsec,
	}
	var confirmed unix.Stat_t
	if !darwinSourceExtendedSecuritySafeV1(fd) || verifyDarwinFilesystemV1(fd) != nil ||
		unix.Fstat(fd, &confirmed) != nil ||
		!darwinDirectoryStatMatchesIdentityV1(confirmed, identity) {
		return darwinDirectoryIdentityV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query protected directory identity is unstable"),
		)
	}
	return identity, nil
}

func exactDarwinFileIdentityV1(
	fd int,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
) (darwinFileIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Nlink != 1 || stat.Uid != uint32(unix.Geteuid()) ||
		stat.Mode&0o7777 != 0o400 || stat.Flags != 0 || stat.Size <= 0 ||
		uint64(stat.Size) != object.DuckDBByteLength {
		return darwinFileIdentityV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query immutable snapshot identity is invalid"),
		)
	}
	identity := darwinFileIdentityV1{
		dev: uint64(stat.Dev), ino: uint64(stat.Ino), size: stat.Size,
		mode: stat.Mode, nlink: uint64(stat.Nlink), uid: stat.Uid, flags: stat.Flags,
		ctimeSec: stat.Ctim.Sec, ctimeNsec: stat.Ctim.Nsec,
	}
	var confirmed unix.Stat_t
	if !darwinSourceExtendedSecuritySafeV1(fd) || verifyDarwinFilesystemV1(fd) != nil ||
		unix.Fstat(fd, &confirmed) != nil ||
		!darwinPathStatMatchesFileIdentityV1(confirmed, identity) {
		return darwinFileIdentityV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds query immutable snapshot identity is unstable"),
		)
	}
	return identity, nil
}

func ensureNoDarwinWALV1(directoryFD int, name string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(directoryFD, name+".wal", &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query snapshot WAL state is unavailable"),
		)
	}
	return errors.Join(
		fundsquerysourceport.ErrMismatch,
		errors.New("funds query immutable snapshot has a WAL sidecar"),
	)
}

func (lease *exactReadLeaseV1) CopyExactTo(
	operationContext context.Context,
	destination *os.File,
) (resultErr error) {
	if lease == nil || operationContext == nil || destination == nil || !lease.beginCopy() {
		return fundsquerysourceport.ErrUnavailable
	}
	defer lease.endCopy()
	if err := exactReadContextsErrorV1(lease.ctx, operationContext); err != nil {
		return err
	}
	expectedSize := int64(lease.expected.DuckDBByteLength)
	if expectedSize <= 0 || uint64(expectedSize) != lease.expected.DuckDBByteLength {
		return fundsquerysourceport.ErrMismatch
	}
	destinationFD, destinationIdentity, err := openPinnedDarwinDestinationV1(destination)
	if err != nil {
		return err
	}
	defer unix.Close(destinationFD)
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := scrubDarwinDestinationV1(destinationFD, destinationIdentity); err != nil {
			resultErr = errors.Join(resultErr, fundsquerysourceport.ErrCorrupt, err)
		}
	}()

	hasher := sha256.New()
	buffer := make([]byte, darwinExactCopyChunkBytesV1)
	var copied int64
	for copied < expectedSize {
		if err := exactReadContextsErrorV1(lease.ctx, operationContext); err != nil {
			return err
		}
		if verifyDarwinFileIdentityV1(int(lease.file.Fd()), lease.identity) != nil ||
			verifyDarwinDestinationV1(destinationFD, copied, destinationIdentity) != nil {
			return fundsquerysourceport.ErrMismatch
		}
		remaining := expectedSize - copied
		chunk := buffer
		if remaining < int64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr := unix.Pread(int(lease.file.Fd()), chunk, copied)
		if read > 0 {
			payload := chunk[:read]
			_, _ = hasher.Write(payload)
			written := 0
			for written < len(payload) {
				if err := exactReadContextsErrorV1(lease.ctx, operationContext); err != nil {
					return err
				}
				count, writeErr := unix.Pwrite(destinationFD, payload[written:], copied+int64(written))
				if count > 0 {
					written += count
					if verifyDarwinDestinationV1(
						destinationFD,
						copied+int64(written),
						destinationIdentity,
					) != nil {
						return fundsquerysourceport.ErrMismatch
					}
				}
				if writeErr != nil {
					if errors.Is(writeErr, unix.EINTR) {
						continue
					}
					return fundsquerysourceport.ErrUnavailable
				}
				if count == 0 {
					return fundsquerysourceport.ErrUnavailable
				}
			}
			copied += int64(read)
		}
		if readErr != nil && !errors.Is(readErr, unix.EINTR) {
			if errors.Is(readErr, io.EOF) {
				return fundsquerysourceport.ErrMismatch
			}
			return fundsquerysourceport.ErrUnavailable
		}
		if read == 0 && readErr == nil {
			return fundsquerysourceport.ErrMismatch
		}
		if verifyDarwinFileIdentityV1(int(lease.file.Fd()), lease.identity) != nil ||
			verifyDarwinDestinationV1(destinationFD, copied, destinationIdentity) != nil {
			return fundsquerysourceport.ErrMismatch
		}
	}
	if err := exactReadContextsErrorV1(lease.ctx, operationContext); err != nil {
		return err
	}
	extra := []byte{0}
	if count, readErr := unix.Pread(int(lease.file.Fd()), extra, expectedSize); count != 0 || readErr != nil {
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fundsquerysourceport.ErrUnavailable
		}
		if count != 0 {
			return fundsquerysourceport.ErrMismatch
		}
	}
	if unix.Fsync(destinationFD) != nil ||
		hex.EncodeToString(hasher.Sum(nil)) != lease.expected.DuckDBSHA256 ||
		verifyDarwinFileIdentityV1(int(lease.file.Fd()), lease.identity) != nil ||
		verifyDarwinDestinationV1(destinationFD, expectedSize, destinationIdentity) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	if err := exactReadContextsErrorV1(lease.ctx, operationContext); err != nil {
		return err
	}
	lease.mu.Lock()
	lease.completed = true
	lease.mu.Unlock()
	committed = true
	return nil
}

func exactReadContextsErrorV1(
	authorityContext context.Context,
	operationContext context.Context,
) error {
	if authorityContext == nil || operationContext == nil {
		return fundsquerysourceport.ErrUnavailable
	}
	if err := authorityContext.Err(); err != nil {
		return err
	}
	return operationContext.Err()
}

func openPinnedDarwinDestinationV1(
	destination *os.File,
) (int, darwinDestinationIdentityV1, error) {
	if destination == nil {
		return -1, darwinDestinationIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	callerFD := int(destination.Fd())
	identity, err := inspectDarwinDestinationV1(callerFD, 0)
	if err != nil {
		return -1, darwinDestinationIdentityV1{}, err
	}
	pinnedFD, err := unix.FcntlInt(
		destination.Fd(),
		unix.F_DUPFD_CLOEXEC,
		darwinExactDestinationMinimumFDV1,
	)
	if err != nil {
		return -1, darwinDestinationIdentityV1{}, fundsquerysourceport.ErrUnavailable
	}
	pinnedIdentity, err := inspectDarwinDestinationV1(pinnedFD, 0)
	if err != nil || pinnedIdentity != identity ||
		verifyDarwinDestinationV1(callerFD, 0, identity) != nil {
		_ = unix.Close(pinnedFD)
		return -1, darwinDestinationIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	return pinnedFD, identity, nil
}

func inspectDarwinDestinationV1(
	fd int,
	expectedSize int64,
) (darwinDestinationIdentityV1, error) {
	if fd < 0 || expectedSize < 0 {
		return darwinDestinationIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	filesystemBefore, filesystemErr := inspectDarwinDestinationFilesystemV1(fd)
	securityBefore, securityErr := inspectDarwinDestinationSecurityV1(fd)
	var before unix.Stat_t
	statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	offset, offsetErr := unix.Seek(fd, 0, io.SeekCurrent)
	if filesystemErr != nil || securityErr != nil || unix.Fstat(fd, &before) != nil ||
		statusErr != nil || descriptorErr != nil || offsetErr != nil || offset != 0 ||
		before.Mode&unix.S_IFMT != unix.S_IFREG || before.Size != expectedSize ||
		before.Nlink != 0 || before.Uid != uint32(os.Geteuid()) || before.Gid != uint32(os.Getegid()) ||
		before.Mode&0o7777 != 0o600 || before.Flags != 0 ||
		statusFlags&unix.O_ACCMODE != unix.O_RDWR ||
		statusFlags&(unix.O_APPEND|unix.O_NONBLOCK) != 0 || descriptorFlags&unix.FD_CLOEXEC == 0 {
		return darwinDestinationIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	filesystemAfter, filesystemAfterErr := inspectDarwinDestinationFilesystemV1(fd)
	securityAfter, securityAfterErr := inspectDarwinDestinationSecurityV1(fd)
	var after unix.Stat_t
	if filesystemAfterErr != nil || filesystemAfter != filesystemBefore ||
		securityAfterErr != nil || securityAfter != securityBefore ||
		unix.Fstat(fd, &after) != nil || !darwinDestinationStatStableV1(before, after) {
		return darwinDestinationIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	return darwinDestinationIdentityV1{
		dev: uint64(after.Dev), ino: uint64(after.Ino), uid: after.Uid, gid: after.Gid,
		mode: after.Mode, nlink: uint64(after.Nlink), flags: after.Flags,
		filesystem: filesystemAfter, security: securityAfter,
	}, nil
}

func verifyDarwinDestinationV1(
	fd int,
	expectedSize int64,
	expected darwinDestinationIdentityV1,
) error {
	current, err := inspectDarwinDestinationV1(fd, expectedSize)
	if err != nil || current != expected {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}

func inspectDarwinDestinationFilesystemV1(
	fd int,
) (darwinDestinationFilesystemIdentityV1, error) {
	var info unix.Statfs_t
	if fd < 0 || unix.Fstatfs(fd, &info) != nil || !darwinSourceFilesystemSafeV1(info) {
		return darwinDestinationFilesystemIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	return darwinDestinationFilesystemIdentityV1{fsid: info.Fsid.Val}, nil
}

func inspectDarwinDestinationSecurityV1(
	fd int,
) (darwinDestinationSecurityIdentityV1, error) {
	if fd < 0 || !darwinSourceHasNoExtendedACLV1(fd) {
		return darwinDestinationSecurityIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > darwinSourceMaximumXattrBytesV1 {
		return darwinDestinationSecurityIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	if size == 0 {
		return darwinDestinationSecurityIdentityV1{}, nil
	}
	names := make([]byte, size)
	read, err := unix.Flistxattr(fd, names)
	expectedNames := append([]byte(darwinSourceProvenanceNameV1), 0)
	if err != nil || read != size || string(names) != string(expectedNames) {
		return darwinDestinationSecurityIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	valueSize, err := unix.Fgetxattr(fd, darwinSourceProvenanceNameV1, nil)
	if err != nil || valueSize != darwinSourceProvenanceByteLengthV1 {
		return darwinDestinationSecurityIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	value := make([]byte, valueSize)
	read, err = unix.Fgetxattr(fd, darwinSourceProvenanceNameV1, value)
	if err != nil || read != len(value) || !validDarwinSourceProvenanceV1(value) {
		return darwinDestinationSecurityIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	identity := darwinDestinationSecurityIdentityV1{present: true}
	copy(identity.provenance[:], value)
	return identity, nil
}

func darwinDestinationStatStableV1(left unix.Stat_t, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino && left.Size == right.Size &&
		left.Mode == right.Mode && left.Nlink == right.Nlink && left.Uid == right.Uid &&
		left.Gid == right.Gid && left.Flags == right.Flags
}

func scrubDarwinDestinationV1(fd int, expected darwinDestinationIdentityV1) error {
	if fd < 0 {
		return fundsquerysourceport.ErrCorrupt
	}
	truncateErr := unix.Ftruncate(fd, 0)
	syncErr := unix.Fsync(fd)
	validationErr := verifyDarwinDestinationV1(fd, 0, expected)
	if truncateErr != nil || syncErr != nil || validationErr != nil {
		return errors.Join(truncateErr, syncErr, validationErr)
	}
	return nil
}

func (lease *exactReadLeaseV1) beginCopy() bool {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.active || lease.copying || lease.completed || lease.ctx == nil {
		return false
	}
	lease.copying = true
	return true
}

func (lease *exactReadLeaseV1) endCopy() {
	lease.mu.Lock()
	lease.copying = false
	lease.condition.Broadcast()
	lease.mu.Unlock()
}

func (lease *exactReadLeaseV1) close() {
	lease.mu.Lock()
	lease.active = false
	for lease.copying {
		lease.condition.Wait()
	}
	lease.mu.Unlock()
}

func (lease *exactReadLeaseV1) completedExactCopy() bool {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return lease.completed
}

func verifyDarwinSnapshotAfterUseV1(
	ctx context.Context,
	source *Source,
	directoryFD int,
	file *os.File,
	directoryIdentity darwinDirectoryIdentityV1,
	fileIdentity darwinFileIdentityV1,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if verifyDarwinDirectoryIdentityV1(directoryFD, directoryIdentity) != nil ||
		verifyDarwinFileIdentityV1(int(file.Fd()), fileIdentity) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	name := immutableSnapshotObjectFileNameV1(object)
	var pathStat unix.Stat_t
	if unix.Fstatat(directoryFD, name, &pathStat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		pathStat.Mode&unix.S_IFMT != unix.S_IFREG ||
		!darwinPathStatMatchesFileIdentityV1(pathStat, fileIdentity) ||
		ensureNoDarwinWALV1(directoryFD, name) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fundsquerysourceport.ErrUnavailable
	}
	hasher := sha256.New()
	read, err := io.Copy(hasher, io.LimitReader(file, int64(object.DuckDBByteLength)+1))
	if err != nil || read != int64(object.DuckDBByteLength) ||
		hex.EncodeToString(hasher.Sum(nil)) != object.DuckDBSHA256 ||
		verifyDarwinDirectoryIdentityV1(directoryFD, directoryIdentity) != nil ||
		verifyDarwinFileIdentityV1(int(file.Fd()), fileIdentity) != nil ||
		verifyCurrentDarwinSnapshotPathV1(source, object, directoryIdentity, fileIdentity) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}

func verifyDarwinFileIdentityV1(fd int, expected darwinFileIdentityV1) error {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || !darwinPathStatMatchesFileIdentityV1(stat, expected) ||
		stat.Flags != 0 || !darwinSourceExtendedSecuritySafeV1(fd) ||
		verifyDarwinFilesystemV1(fd) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	var confirmed unix.Stat_t
	if unix.Fstat(fd, &confirmed) != nil ||
		!darwinPathStatMatchesFileIdentityV1(confirmed, expected) {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}

func verifyDarwinDirectoryIdentityV1(fd int, expected darwinDirectoryIdentityV1) error {
	current, err := exactDarwinDirectoryIdentityV1(fd)
	if err != nil || current != expected {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}

func darwinPathStatMatchesFileIdentityV1(stat unix.Stat_t, expected darwinFileIdentityV1) bool {
	return uint64(stat.Dev) == expected.dev && uint64(stat.Ino) == expected.ino &&
		stat.Size == expected.size && stat.Mode == expected.mode &&
		uint64(stat.Nlink) == expected.nlink && stat.Uid == expected.uid &&
		stat.Flags == expected.flags && stat.Ctim.Sec == expected.ctimeSec &&
		stat.Ctim.Nsec == expected.ctimeNsec
}

func darwinDirectoryStatMatchesIdentityV1(stat unix.Stat_t, expected darwinDirectoryIdentityV1) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR &&
		uint64(stat.Dev) == expected.dev && uint64(stat.Ino) == expected.ino &&
		stat.Mode == expected.mode && uint64(stat.Nlink) == expected.nlink &&
		stat.Uid == expected.uid && stat.Flags == expected.flags &&
		stat.Ctim.Sec == expected.ctimeSec && stat.Ctim.Nsec == expected.ctimeNsec
}

func verifyCurrentDarwinSnapshotPathV1(
	source *Source,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	expectedDirectory darwinDirectoryIdentityV1,
	expectedFile darwinFileIdentityV1,
) error {
	if source == nil {
		return fundsquerysourceport.ErrMismatch
	}
	directoryFD, directoryIdentity, err := source.openSnapshotDirectoryDarwinV1(object.CaseID)
	if err != nil {
		return fundsquerysourceport.ErrMismatch
	}
	defer unix.Close(directoryFD)
	if directoryIdentity != expectedDirectory {
		return fundsquerysourceport.ErrMismatch
	}
	name := immutableSnapshotObjectFileNameV1(object)
	var pathStat unix.Stat_t
	if unix.Fstatat(directoryFD, name, &pathStat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		pathStat.Mode&unix.S_IFMT != unix.S_IFREG ||
		!darwinPathStatMatchesFileIdentityV1(pathStat, expectedFile) ||
		ensureNoDarwinWALV1(directoryFD, name) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}

func mapOpenErrorV1(err error) error {
	if errors.Is(err, unix.ENOENT) {
		return errors.Join(fundsquerysourceport.ErrNotFound, errors.New("funds query immutable snapshot is absent"))
	}
	return errors.Join(fundsquerysourceport.ErrUnavailable, errors.New("funds query immutable snapshot cannot be opened"))
}
