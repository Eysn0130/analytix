//go:build darwin

package nativecomponentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

const (
	privateSnapshotDuplicateMinimumFD = 8

	privateAccountFlowSnapshotBasename = "3"

	// Fixed native funds operations have a 30-second execution window. Keep the
	// callback-scoped exact copy bounded to the source-row product cap instead
	// of inheriting the descriptor format's 1 TiB envelope.
	privateAccountFlowSnapshotMaximumBytes = int64(domainnative.TransactionSourceRowMaximumSnapshotBytesV1)

	// Admission must leave room for runtime state, query spill, and orderly
	// termination after the exact snapshot allocation has been reserved.
	privateAccountFlowSnapshotFreeSpaceReserveBytes = uint64(1024 * 1024 * 1024)
)

type privateSnapshotIdentity struct {
	device           uint64
	inode            uint64
	size             int64
	filesystem       privateSnapshotFilesystemIdentity
	extendedSecurity privateSnapshotExtendedSecurityIdentity
}

type privateSnapshotDirectoryIdentity struct {
	device           uint64
	inode            uint64
	links            uint64
	uid              uint32
	gid              uint32
	filesystem       privateSnapshotFilesystemIdentity
	extendedSecurity privateSnapshotExtendedSecurityIdentity
}

type privateAccountFlowSnapshot struct {
	file                       *os.File
	identity                   privateSnapshotIdentity
	sha256                     string
	size                       int64
	stagingAuthority           *os.File
	stagingIdentity            privateSnapshotDirectoryIdentity
	stagingRetiredLinks        uint64
	operationDirectory         *os.File
	operationDirectoryIdentity privateSnapshotDirectoryIdentity
	operationDirectoryName     string
}

type privateSnapshotProcessInput struct {
	file   *os.File
	sha256 string
	size   int64
}

func materializePrivateAccountFlowSnapshot(
	ctx context.Context,
	stagingAuthority *os.File,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (_ *privateAccountFlowSnapshot, resultErr error) {
	if domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil {
		return nil, ErrRequestInvalid
	}
	object, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		descriptor.CaseID,
		descriptor.DuckDBSHA256,
		descriptor.DuckDBByteLength,
	)
	if err != nil {
		return nil, ErrRequestInvalid
	}
	return materializePrivateImmutableSnapshot(ctx, stagingAuthority, object, source)
}

func materializePrivateImmutableSnapshot(
	ctx context.Context,
	stagingAuthority *os.File,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	source fundsquerysourceport.ExactReadLease,
) (_ *privateAccountFlowSnapshot, resultErr error) {
	if ctx == nil || stagingAuthority == nil || source == nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return nil, ErrRequestInvalid
	}
	expectedSize := int64(object.DuckDBByteLength)
	if uint64(expectedSize) != object.DuckDBByteLength ||
		expectedSize > privateAccountFlowSnapshotMaximumBytes {
		return nil, ErrRequestInvalid
	}
	directoryFD := int(stagingAuthority.Fd())
	directoryIdentity, err := validatePrivateSnapshotDirectory(directoryFD)
	if err != nil {
		return nil, ErrRegistry
	}
	if err := admitPrivateAccountFlowSnapshotDiskSpace(
		directoryFD, expectedSize, directoryIdentity.filesystem,
	); err != nil {
		return nil, err
	}
	if err := validatePrivateSnapshotDirectoryExact(directoryFD, directoryIdentity); err != nil {
		return nil, ErrRegistry
	}
	operationID, err := randomProtocolID()
	if err != nil {
		return nil, ErrUnavailable
	}
	// The pinned macOS DuckDB canonicalizer resolves /dev/fd/3 through the
	// inherited vnode's former basename. Isolate each invocation in its own
	// private directory so the exclusive source inode can be named exactly "3"
	// before open+unlink without creating a shared or reusable path.
	operationDirectoryName := ".account-flow-" + operationID
	if err := unix.Mkdirat(directoryFD, operationDirectoryName, 0o700); err != nil {
		return nil, ErrUnavailable
	}
	operationDirectoryLinked := true
	var activeDirectoryIdentity privateSnapshotDirectoryIdentity
	activeDirectoryIdentityKnown := false
	var operationDirectory *os.File
	var operationDirectoryIdentity privateSnapshotDirectoryIdentity
	var operationDirectoryRetiredLinks uint64
	var created *os.File
	var readOnly *os.File
	var linkedFileIdentity privateSnapshotIdentity
	linkedFileIdentityKnown := false
	fileLinked := false
	defer func() {
		if resultErr == nil {
			return
		}
		cleanupErr := error(nil)
		if operationDirectory != nil && fileLinked && linkedFileIdentityKnown &&
			validatePrivateSnapshotDirectoryExact(
				int(operationDirectory.Fd()), operationDirectoryIdentity,
			) == nil &&
			created != nil && privateSnapshotFileMatchesExact(
			int(created.Fd()), linkedFileIdentity, 1, 0o600, unix.O_RDWR,
		) &&
			validatePrivateSnapshotFileBinding(
				int(operationDirectory.Fd()), privateAccountFlowSnapshotBasename, linkedFileIdentity,
				1, 0o600,
			) == nil {
			unlinkErr := unix.Unlinkat(
				int(operationDirectory.Fd()), privateAccountFlowSnapshotBasename, 0,
			)
			cleanupErr = errors.Join(cleanupErr, unlinkErr)
			if unlinkErr == nil {
				fileLinked = false
				restoredOperation, restoreErr := validatePrivateSnapshotDirectory(
					int(operationDirectory.Fd()),
				)
				if restoreErr != nil || !validPrivateSnapshotDirectoryRemovalTransition(
					operationDirectoryIdentity,
					restoredOperation,
					operationDirectoryRetiredLinks,
				) {
					cleanupErr = errors.Join(cleanupErr, ErrRegistry)
				} else {
					operationDirectoryIdentity = restoredOperation
				}
				cleanupErr = errors.Join(cleanupErr, unix.Fsync(int(operationDirectory.Fd())))
				if !privateSnapshotFileMatchesExact(
					int(created.Fd()), linkedFileIdentity, 0, 0o600, unix.O_RDWR,
				) {
					cleanupErr = errors.Join(cleanupErr, ErrRegistry)
				}
			}
		} else if fileLinked {
			cleanupErr = errors.Join(cleanupErr, ErrRegistry)
		}
		if created != nil {
			cleanupErr = errors.Join(cleanupErr, created.Close())
		}
		if readOnly != nil {
			cleanupErr = errors.Join(cleanupErr, readOnly.Close())
		}
		if operationDirectory != nil {
			if validatePrivateSnapshotDirectoryExact(
				int(operationDirectory.Fd()), operationDirectoryIdentity,
			) != nil {
				cleanupErr = errors.Join(cleanupErr, ErrRegistry)
			} else {
				cleanupErr = errors.Join(cleanupErr, unix.Fsync(int(operationDirectory.Fd())))
				cleanupErr = errors.Join(cleanupErr, validatePrivateSnapshotDirectoryExact(
					int(operationDirectory.Fd()), operationDirectoryIdentity,
				))
			}
		}
		if operationDirectoryLinked && activeDirectoryIdentityKnown &&
			validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) == nil &&
			operationDirectory != nil && validatePrivateSnapshotDirectoryBinding(
			directoryFD, operationDirectoryName, operationDirectoryIdentity,
		) == nil && validatePrivateSnapshotDirectoryExact(
			int(operationDirectory.Fd()), operationDirectoryIdentity,
		) == nil {
			removeErr := unix.Unlinkat(
				directoryFD, operationDirectoryName, unix.AT_REMOVEDIR,
			)
			cleanupErr = errors.Join(cleanupErr, removeErr)
			if removeErr == nil {
				operationDirectoryLinked = false
				unlinked, unlinkValidationErr := validatePrivateSnapshotUnlinkedDirectory(
					int(operationDirectory.Fd()),
				)
				if unlinkValidationErr != nil || unlinked != operationDirectoryIdentity {
					cleanupErr = errors.Join(cleanupErr, ErrRegistry)
				}
			}
			cleanupErr = errors.Join(cleanupErr, unix.Fsync(directoryFD))
		} else if operationDirectoryLinked {
			cleanupErr = errors.Join(cleanupErr, ErrRegistry)
		}
		if !operationDirectoryLinked {
			if current, err := validatePrivateSnapshotDirectory(directoryFD); err != nil ||
				!validPrivateSnapshotDirectoryRemovalTransition(
					activeDirectoryIdentity, current, directoryIdentity.links,
				) {
				cleanupErr = errors.Join(cleanupErr, ErrRegistry)
			}
		}
		if operationDirectory != nil {
			cleanupErr = errors.Join(cleanupErr, operationDirectory.Close())
		}
		if cleanupErr != nil {
			resultErr = ErrTermination
		}
	}()
	activeDirectoryIdentity, err = validatePrivateSnapshotDirectory(directoryFD)
	if err != nil || !validPrivateSnapshotDirectoryCreationTransition(
		directoryIdentity, activeDirectoryIdentity,
	) {
		return nil, ErrRegistry
	}
	activeDirectoryIdentityKnown = true
	if err := unix.Fsync(directoryFD); err != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil {
		return nil, ErrTermination
	}
	operationDirectoryFD, err := unix.Openat(
		directoryFD,
		operationDirectoryName,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	operationDirectory = os.NewFile(uintptr(operationDirectoryFD), "private-account-flow-operation")
	if operationDirectory == nil {
		_ = unix.Close(operationDirectoryFD)
		return nil, ErrTermination
	}
	operationDirectoryIdentity, err = validatePrivateSnapshotDirectory(operationDirectoryFD)
	if err != nil || operationDirectoryIdentity.filesystem != activeDirectoryIdentity.filesystem ||
		operationDirectoryIdentity.extendedSecurity != activeDirectoryIdentity.extendedSecurity ||
		operationDirectoryIdentity.uid != activeDirectoryIdentity.uid ||
		operationDirectoryIdentity.gid != activeDirectoryIdentity.gid ||
		validatePrivateSnapshotDirectoryBinding(
			directoryFD, operationDirectoryName, operationDirectoryIdentity,
		) != nil {
		return nil, ErrRegistry
	}
	operationDirectoryRetiredLinks = operationDirectoryIdentity.links
	if err := validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity); err != nil ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil {
		return nil, ErrRegistry
	}
	createdFD, err := unix.Openat(
		operationDirectoryFD,
		privateAccountFlowSnapshotBasename,
		unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	fileLinked = true
	operationDirectoryWithFile, err := validatePrivateSnapshotDirectory(operationDirectoryFD)
	if err != nil || !validPrivateSnapshotDirectoryCreationTransition(
		operationDirectoryIdentity, operationDirectoryWithFile,
	) {
		return nil, ErrRegistry
	}
	operationDirectoryIdentity = operationDirectoryWithFile
	created = os.NewFile(uintptr(createdFD), "private-account-flow-snapshot-stage")
	if created == nil {
		_ = unix.Close(createdFD)
		return nil, ErrTermination
	}
	createdIdentity, err := validatePrivateSnapshotFile(
		createdFD, 0, 1, 0o600, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil {
		return nil, ErrRegistry
	}
	if unix.Fchmod(createdFD, 0o600) != nil {
		return nil, ErrUnavailable
	}
	if current, validationErr := validatePrivateSnapshotFile(
		createdFD, 0, 1, 0o600, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	); validationErr != nil || current != createdIdentity {
		return nil, ErrRegistry
	}
	initialIdentity := createdIdentity
	linkedFileIdentity = initialIdentity
	linkedFileIdentityKnown = true
	if err := validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity); err != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryBinding(
			directoryFD, operationDirectoryName, operationDirectoryIdentity,
		) != nil ||
		validatePrivateSnapshotFileBinding(
			operationDirectoryFD, privateAccountFlowSnapshotBasename, initialIdentity, 1, 0o600,
		) != nil {
		return nil, ErrRegistry
	}
	readOnlyFD, err := unix.Openat(
		operationDirectoryFD,
		privateAccountFlowSnapshotBasename,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	readOnly = os.NewFile(uintptr(readOnlyFD), "private-account-flow-snapshot")
	if readOnly == nil {
		_ = unix.Close(readOnlyFD)
		return nil, ErrTermination
	}
	readOnlyIdentity, err := validatePrivateSnapshotFile(
		readOnlyFD, 0, 1, 0o600, unix.O_RDONLY,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || readOnlyIdentity != initialIdentity ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil ||
		validatePrivateSnapshotFileBinding(
			operationDirectoryFD, privateAccountFlowSnapshotBasename, initialIdentity, 1, 0o600,
		) != nil {
		return nil, ErrRegistry
	}

	// No DuckDB byte may ever have a linked pathname. Keep both the eventual
	// read-only process handle and the fill-only O_RDWR handle open, unlink the
	// still-empty inode, and make that unlink durable before reserving space or
	// invoking the source lease.
	if err := unix.Unlinkat(operationDirectoryFD, privateAccountFlowSnapshotBasename, 0); err != nil {
		return nil, ErrTermination
	}
	fileLinked = false
	restoredOperationDirectory, err := validatePrivateSnapshotDirectory(operationDirectoryFD)
	if err != nil || !validPrivateSnapshotDirectoryRemovalTransition(
		operationDirectoryIdentity,
		restoredOperationDirectory,
		operationDirectoryRetiredLinks,
	) {
		return nil, ErrRegistry
	}
	operationDirectoryIdentity = restoredOperationDirectory
	if err := unix.Fsync(operationDirectoryFD); err != nil ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil {
		return nil, ErrTermination
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationDirectoryFD); err != nil || !empty {
		return nil, ErrRegistry
	}
	createdIdentity, err = validatePrivateSnapshotFile(
		createdFD, 0, 0, 0o600, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || createdIdentity != initialIdentity {
		return nil, ErrRegistry
	}
	readOnlyIdentity, err = validatePrivateSnapshotFile(
		readOnlyFD, 0, 0, 0o600, unix.O_RDONLY,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || readOnlyIdentity != initialIdentity {
		return nil, ErrRegistry
	}
	if err := preallocatePrivateAccountFlowSnapshot(
		createdFD, expectedSize, operationDirectoryIdentity.filesystem,
	); err != nil {
		return nil, err
	}
	createdIdentity, err = validatePrivateSnapshotFile(
		createdFD, 0, 0, 0o600, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || createdIdentity != initialIdentity {
		return nil, ErrRegistry
	}
	readOnlyIdentity, err = validatePrivateSnapshotFile(
		readOnlyFD, 0, 0, 0o600, unix.O_RDONLY,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || readOnlyIdentity != initialIdentity ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil {
		return nil, ErrRegistry
	}
	if err := source.CopyExactTo(ctx, created); err != nil {
		if ctx.Err() != nil {
			return nil, contextError(ctx)
		}
		return nil, ErrRegistry
	}
	createdIdentity, err = validatePrivateSnapshotFile(
		createdFD, expectedSize, 0, 0o600, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || !samePrivateSnapshotObject(createdIdentity, initialIdentity) ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil {
		return nil, ErrRegistry
	}
	contentSHA256, err := hashPrivateAccountFlowSnapshot(
		ctx,
		readOnlyFD,
		expectedSize,
		createdIdentity,
	)
	if err != nil || contentSHA256 != object.DuckDBSHA256 ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil {
		if ctx.Err() != nil {
			return nil, contextError(ctx)
		}
		return nil, ErrRegistry
	}
	if err := created.Sync(); err != nil {
		return nil, ErrUnavailable
	}
	if current, validationErr := validatePrivateSnapshotFile(
		createdFD, expectedSize, 0, 0o600, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	); validationErr != nil || current != createdIdentity {
		return nil, ErrRegistry
	}
	if unix.Fchmod(createdFD, 0o400) != nil {
		return nil, ErrUnavailable
	}
	createdIdentity, err = validatePrivateSnapshotFile(
		createdFD, expectedSize, 0, 0o400, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || !samePrivateSnapshotObject(createdIdentity, initialIdentity) {
		return nil, ErrRegistry
	}
	if err := created.Sync(); err != nil {
		return nil, ErrUnavailable
	}
	identity, err := validatePrivateSnapshotFile(
		createdFD, expectedSize, 0, 0o400, unix.O_RDWR,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || identity != createdIdentity {
		return nil, ErrRegistry
	}
	readOnlyIdentity, err = validatePrivateSnapshotFile(
		readOnlyFD, expectedSize, 0, 0o400, unix.O_RDONLY,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || readOnlyIdentity != identity {
		return nil, ErrRegistry
	}
	if err := created.Close(); err != nil {
		created = nil
		return nil, ErrTermination
	}
	created = nil
	readOnlyIdentity, err = validatePrivateSnapshotFile(
		readOnlyFD, expectedSize, 0, 0o400, unix.O_RDONLY,
		operationDirectoryIdentity.filesystem, operationDirectoryIdentity.extendedSecurity,
	)
	if err != nil || readOnlyIdentity != identity ||
		validatePrivateSnapshotDirectoryExact(directoryFD, activeDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(operationDirectoryFD, operationDirectoryIdentity) != nil {
		return nil, ErrRegistry
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationDirectoryFD); err != nil || !empty {
		return nil, ErrRegistry
	}
	snapshot := &privateAccountFlowSnapshot{
		file: readOnly, identity: identity, sha256: object.DuckDBSHA256, size: expectedSize,
		stagingAuthority: stagingAuthority, stagingIdentity: activeDirectoryIdentity,
		stagingRetiredLinks: directoryIdentity.links,
		operationDirectory:  operationDirectory, operationDirectoryIdentity: operationDirectoryIdentity,
		operationDirectoryName: operationDirectoryName,
	}
	readOnly = nil
	operationDirectory = nil
	operationDirectoryLinked = false
	return snapshot, nil
}

func hashPrivateAccountFlowSnapshot(
	ctx context.Context,
	fd int,
	expectedSize int64,
	expectedFile privateSnapshotIdentity,
) (string, error) {
	if ctx == nil || fd < 0 || expectedSize <= 0 || expectedFile.size != expectedSize {
		return "", ErrRegistry
	}
	hasher := sha256.New()
	buffer := make([]byte, 256<<10)
	var offset int64
	for offset < expectedSize {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !privateSnapshotFileMatchesExact(fd, expectedFile, 0, 0o600, unix.O_RDONLY) {
			return "", ErrRegistry
		}
		remaining := expectedSize - offset
		chunk := buffer
		if remaining < int64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr := unix.Pread(fd, chunk, offset)
		if read > 0 {
			_, _ = hasher.Write(chunk[:read])
			offset += int64(read)
		}
		if readErr != nil && !errors.Is(readErr, unix.EINTR) {
			return "", ErrRegistry
		}
		if read == 0 && readErr == nil {
			return "", ErrRegistry
		}
		if !privateSnapshotFileMatchesExact(fd, expectedFile, 0, 0o600, unix.O_RDONLY) {
			return "", ErrRegistry
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	extra := []byte{0}
	read, readErr := unix.Pread(fd, extra, expectedSize)
	if read != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) ||
		!privateSnapshotFileMatchesExact(fd, expectedFile, 0, 0o600, unix.O_RDONLY) {
		return "", ErrRegistry
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func admitPrivateAccountFlowSnapshotDiskSpace(
	directoryFD int,
	expectedSize int64,
	expectedFilesystem privateSnapshotFilesystemIdentity,
) error {
	if directoryFD < 0 || expectedSize <= 0 || expectedSize > privateAccountFlowSnapshotMaximumBytes {
		return ErrRequestInvalid
	}
	var stat unix.Statfs_t
	filesystem, filesystemErr := probePrivateSnapshotFilesystem(directoryFD)
	if filesystemErr != nil || filesystem != expectedFilesystem {
		return ErrRegistry
	}
	if unix.Fstatfs(directoryFD, &stat) != nil ||
		!privateAccountFlowSnapshotHasAvailableBytes(
			stat,
			uint64(expectedSize)+privateAccountFlowSnapshotFreeSpaceReserveBytes,
		) {
		return ErrUnavailable
	}
	return nil
}

func preallocatePrivateAccountFlowSnapshot(
	fd int,
	expectedSize int64,
	expectedFilesystem privateSnapshotFilesystemIdentity,
) error {
	if fd < 0 || expectedSize <= 0 || expectedSize > privateAccountFlowSnapshotMaximumBytes {
		return ErrRequestInvalid
	}
	filesystemBefore, filesystemBeforeErr := probePrivateSnapshotFilesystem(fd)
	if filesystemBeforeErr != nil || filesystemBefore != expectedFilesystem {
		return ErrRegistry
	}
	allocation := unix.Fstore_t{
		Flags:   unix.F_ALLOCATEALL,
		Posmode: unix.F_PEOFPOSMODE,
		Offset:  0,
		Length:  expectedSize,
	}
	if err := unix.FcntlFstore(uintptr(fd), unix.F_PREALLOCATE, &allocation); err != nil ||
		allocation.Bytesalloc < expectedSize {
		return ErrUnavailable
	}
	var stat unix.Statfs_t
	filesystem, filesystemErr := probePrivateSnapshotFilesystem(fd)
	if filesystemErr != nil || filesystem != expectedFilesystem {
		return ErrRegistry
	}
	if unix.Fstatfs(fd, &stat) != nil ||
		!privateAccountFlowSnapshotHasAvailableBytes(
			stat,
			privateAccountFlowSnapshotFreeSpaceReserveBytes,
		) {
		return ErrUnavailable
	}
	return nil
}

func privateAccountFlowSnapshotHasAvailableBytes(stat unix.Statfs_t, required uint64) bool {
	if stat.Bsize == 0 || required == 0 {
		return false
	}
	blockSize := uint64(stat.Bsize)
	requiredBlocks := required / blockSize
	if required%blockSize != 0 {
		requiredBlocks++
	}
	return stat.Bavail >= requiredBlocks
}

func (snapshot *privateAccountFlowSnapshot) duplicateForProcess() (*privateSnapshotProcessInput, error) {
	if snapshot == nil || snapshot.file == nil || snapshot.size <= 0 ||
		len(snapshot.sha256) != sha256.Size*2 {
		return nil, ErrRegistry
	}
	if err := snapshot.validatePostExecution(); err != nil {
		return nil, err
	}
	duplicateFD, err := unix.FcntlInt(
		snapshot.file.Fd(),
		unix.F_DUPFD_CLOEXEC,
		privateSnapshotDuplicateMinimumFD,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	duplicate := os.NewFile(uintptr(duplicateFD), "private-account-flow-snapshot-process-input")
	if duplicate == nil {
		_ = unix.Close(duplicateFD)
		return nil, ErrTermination
	}
	identity, err := validatePrivateSnapshotFile(
		duplicateFD, snapshot.size, 0, 0o400, unix.O_RDONLY,
		snapshot.identity.filesystem, snapshot.identity.extendedSecurity,
	)
	if err != nil || identity != snapshot.identity || snapshot.validatePostExecution() != nil {
		_ = duplicate.Close()
		return nil, ErrRegistry
	}
	return &privateSnapshotProcessInput{
		file: duplicate, sha256: snapshot.sha256, size: snapshot.size,
	}, nil
}

func (snapshot *privateAccountFlowSnapshot) validatePostExecution() error {
	if snapshot == nil || snapshot.file == nil || snapshot.size <= 0 ||
		snapshot.stagingAuthority == nil || snapshot.operationDirectory == nil ||
		snapshot.operationDirectoryName == "" || snapshot.stagingRetiredLinks == 0 {
		return ErrRegistry
	}
	identity, err := validatePrivateSnapshotFile(
		int(snapshot.file.Fd()), snapshot.size, 0, 0o400, unix.O_RDONLY,
		snapshot.identity.filesystem, snapshot.identity.extendedSecurity,
	)
	if err != nil || identity != snapshot.identity ||
		validatePrivateSnapshotDirectoryExact(
			int(snapshot.stagingAuthority.Fd()), snapshot.stagingIdentity,
		) != nil ||
		validatePrivateSnapshotDirectoryExact(
			int(snapshot.operationDirectory.Fd()), snapshot.operationDirectoryIdentity,
		) != nil ||
		validatePrivateSnapshotDirectoryBinding(
			int(snapshot.stagingAuthority.Fd()),
			snapshot.operationDirectoryName,
			snapshot.operationDirectoryIdentity,
		) != nil {
		return ErrRegistry
	}
	empty, inventoryErr := privateSnapshotDirectoryEmpty(int(snapshot.operationDirectory.Fd()))
	if inventoryErr != nil || !empty ||
		validatePrivateSnapshotDirectoryExact(
			int(snapshot.stagingAuthority.Fd()), snapshot.stagingIdentity,
		) != nil ||
		validatePrivateSnapshotDirectoryExact(
			int(snapshot.operationDirectory.Fd()), snapshot.operationDirectoryIdentity,
		) != nil ||
		!privateSnapshotFileMatchesExact(
			int(snapshot.file.Fd()), snapshot.identity, 0, 0o400, unix.O_RDONLY,
		) {
		return ErrRegistry
	}
	return nil
}

func (snapshot *privateAccountFlowSnapshot) settle() error {
	if snapshot == nil {
		return nil
	}
	validationErr := snapshot.validatePostExecution()
	closeErr := error(nil)
	if snapshot.file != nil {
		closeErr = snapshot.file.Close()
		snapshot.file = nil
	}
	retirementErr := snapshot.retireOperationDirectory()
	if closeErr != nil || retirementErr != nil {
		return ErrTermination
	}
	return validationErr
}

func (snapshot *privateAccountFlowSnapshot) retireOperationDirectory() (resultErr error) {
	if snapshot == nil || snapshot.stagingAuthority == nil || snapshot.operationDirectory == nil ||
		snapshot.operationDirectoryName == "" {
		return ErrTermination
	}
	operationDirectory := snapshot.operationDirectory
	snapshot.operationDirectory = nil
	defer func() {
		if err := operationDirectory.Close(); err != nil {
			resultErr = ErrTermination
		}
	}()
	stagingFD := int(snapshot.stagingAuthority.Fd())
	operationFD := int(operationDirectory.Fd())
	if validatePrivateSnapshotDirectoryExact(stagingFD, snapshot.stagingIdentity) != nil {
		return ErrTermination
	}
	if validatePrivateSnapshotDirectoryExact(operationFD, snapshot.operationDirectoryIdentity) != nil {
		return ErrTermination
	}
	empty, err := privateSnapshotDirectoryEmpty(operationFD)
	if err != nil || !empty || unix.Fsync(operationFD) != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, snapshot.operationDirectoryIdentity) != nil ||
		validatePrivateSnapshotDirectoryExact(stagingFD, snapshot.stagingIdentity) != nil {
		return ErrTermination
	}
	if validatePrivateSnapshotDirectoryBinding(
		stagingFD, snapshot.operationDirectoryName, snapshot.operationDirectoryIdentity,
	) != nil {
		return ErrTermination
	}
	if err := unix.Unlinkat(stagingFD, snapshot.operationDirectoryName, unix.AT_REMOVEDIR); err != nil {
		return ErrTermination
	}
	unlinkedOperationIdentity, err := validatePrivateSnapshotUnlinkedDirectory(operationFD)
	if err != nil || unlinkedOperationIdentity != snapshot.operationDirectoryIdentity {
		return ErrTermination
	}
	if err := unix.Fsync(stagingFD); err != nil {
		return ErrTermination
	}
	var pathStat unix.Stat_t
	if err := unix.Fstatat(stagingFD, snapshot.operationDirectoryName, &pathStat, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return ErrTermination
	}
	current, err := validatePrivateSnapshotDirectory(stagingFD)
	if err != nil || !validPrivateSnapshotDirectoryRemovalTransition(
		snapshot.stagingIdentity, current, snapshot.stagingRetiredLinks,
	) {
		return ErrTermination
	}
	snapshot.operationDirectoryName = ""
	snapshot.stagingAuthority = nil
	return nil
}

func validatePrivateSnapshotDirectoryBinding(
	parent int,
	name string,
	identity privateSnapshotDirectoryIdentity,
) error {
	if parent < 0 || name == "" || name == "." || name == ".." ||
		identity.links == 0 {
		return ErrRegistry
	}
	filesystem, filesystemErr := probePrivateSnapshotFilesystem(parent)
	var stat unix.Stat_t
	if filesystemErr != nil || filesystem != identity.filesystem ||
		unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o7777 != 0o700 ||
		stat.Uid != identity.uid || stat.Gid != identity.gid || uint64(stat.Nlink) != identity.links ||
		stat.Flags != 0 || uint64(stat.Dev) != identity.device || stat.Ino != identity.inode {
		return ErrRegistry
	}
	return nil
}

func validatePrivateSnapshotFileBinding(
	parent int,
	name string,
	identity privateSnapshotIdentity,
	expectedLinks uint16,
	expectedMode uint16,
) error {
	if parent < 0 || name == "" || name == "." || name == ".." || expectedLinks == 0 {
		return ErrRegistry
	}
	filesystem, filesystemErr := probePrivateSnapshotFilesystem(parent)
	var stat unix.Stat_t
	if filesystemErr != nil || filesystem != identity.filesystem ||
		unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o7777 != expectedMode ||
		stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) ||
		stat.Nlink != expectedLinks || stat.Size != identity.size || stat.Flags != 0 ||
		uint64(stat.Dev) != identity.device || stat.Ino != identity.inode {
		return ErrRegistry
	}
	return nil
}

func privateSnapshotDirectoryEmpty(fd int) (bool, error) {
	before, err := validatePrivateSnapshotDirectory(fd)
	if err != nil {
		return false, err
	}
	duplicate, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	directory := os.NewFile(uintptr(duplicate), "private-account-flow-operation-inventory")
	if directory == nil {
		_ = unix.Close(duplicate)
		return false, ErrTermination
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return false, errors.Join(readErr, closeErr)
	}
	if err := validatePrivateSnapshotDirectoryExact(fd, before); err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func validatePrivateSnapshotDirectory(fd int) (privateSnapshotDirectoryIdentity, error) {
	return inspectPrivateSnapshotDirectory(fd, false)
}

func validatePrivateSnapshotUnlinkedDirectory(fd int) (privateSnapshotDirectoryIdentity, error) {
	// APFS retains the open directory vnode's pre-removal link count. Callers
	// must require exact identity and independently prove the parent binding is
	// absent; a zero-link assumption would reject the supported filesystem.
	return inspectPrivateSnapshotDirectory(fd, true)
}

func inspectPrivateSnapshotDirectory(
	fd int,
	allowUnlinked bool,
) (privateSnapshotDirectoryIdentity, error) {
	if fd < 0 {
		return privateSnapshotDirectoryIdentity{}, ErrRegistry
	}
	filesystemBefore, filesystemErr := probePrivateSnapshotFilesystem(fd)
	var before unix.Stat_t
	statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	securityBefore, securityErr := privateSnapshotExtendedSecurity(fd)
	if filesystemErr != nil || unix.Fstat(fd, &before) != nil || statusErr != nil || descriptorErr != nil ||
		securityErr != nil || before.Mode&unix.S_IFMT != unix.S_IFDIR ||
		before.Uid != uint32(os.Geteuid()) || before.Gid != uint32(os.Getegid()) ||
		before.Mode&0o7777 != 0o700 || before.Flags != 0 ||
		(!allowUnlinked && before.Nlink == 0) || statusFlags&unix.O_ACCMODE != unix.O_RDONLY ||
		descriptorFlags&unix.FD_CLOEXEC == 0 {
		return privateSnapshotDirectoryIdentity{}, ErrRegistry
	}
	filesystemAfter, filesystemAfterErr := probePrivateSnapshotFilesystem(fd)
	securityAfter, securityAfterErr := privateSnapshotExtendedSecurity(fd)
	var after unix.Stat_t
	if filesystemAfterErr != nil || filesystemAfter != filesystemBefore || securityAfterErr != nil ||
		securityAfter != securityBefore || unix.Fstat(fd, &after) != nil ||
		!samePrivateSnapshotSecurityStat(before, after) {
		return privateSnapshotDirectoryIdentity{}, ErrRegistry
	}
	return privateSnapshotDirectoryIdentity{
		device: uint64(after.Dev), inode: after.Ino, links: uint64(after.Nlink),
		uid: after.Uid, gid: after.Gid, filesystem: filesystemAfter,
		extendedSecurity: securityAfter,
	}, nil
}

func validatePrivateSnapshotDirectoryExact(
	fd int,
	expected privateSnapshotDirectoryIdentity,
) error {
	current, err := validatePrivateSnapshotDirectory(fd)
	if err != nil || current != expected {
		return ErrRegistry
	}
	return nil
}

func validatePrivateSnapshotFile(
	fd int,
	expectedSize int64,
	expectedLinks uint16,
	expectedMode uint16,
	expectedAccess int,
	expectedFilesystem privateSnapshotFilesystemIdentity,
	expectedExtendedSecurity privateSnapshotExtendedSecurityIdentity,
) (privateSnapshotIdentity, error) {
	if fd < 0 || expectedSize < 0 {
		return privateSnapshotIdentity{}, ErrRegistry
	}
	filesystemBefore, filesystemErr := probePrivateSnapshotFilesystem(fd)
	var before unix.Stat_t
	statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	securityBefore, securityErr := privateSnapshotExtendedSecurity(fd)
	if filesystemErr != nil || filesystemBefore != expectedFilesystem || unix.Fstat(fd, &before) != nil ||
		statusErr != nil || descriptorErr != nil || securityErr != nil ||
		securityBefore != expectedExtendedSecurity || before.Mode&unix.S_IFMT != unix.S_IFREG ||
		before.Uid != uint32(os.Geteuid()) || before.Gid != uint32(os.Getegid()) ||
		before.Size != expectedSize || before.Nlink != expectedLinks ||
		before.Mode&0o7777 != expectedMode || before.Flags != 0 ||
		statusFlags&unix.O_ACCMODE != expectedAccess || descriptorFlags&unix.FD_CLOEXEC == 0 {
		return privateSnapshotIdentity{}, ErrRegistry
	}
	filesystemAfter, filesystemAfterErr := probePrivateSnapshotFilesystem(fd)
	securityAfter, securityAfterErr := privateSnapshotExtendedSecurity(fd)
	var after unix.Stat_t
	if filesystemAfterErr != nil || filesystemAfter != filesystemBefore ||
		securityAfterErr != nil || securityAfter != securityBefore ||
		unix.Fstat(fd, &after) != nil || !samePrivateSnapshotSecurityStat(before, after) {
		return privateSnapshotIdentity{}, ErrRegistry
	}
	return privateSnapshotIdentity{
		device: uint64(after.Dev), inode: after.Ino, size: after.Size,
		filesystem: filesystemAfter, extendedSecurity: securityAfter,
	}, nil
}

func privateSnapshotFileMatchesExact(
	fd int,
	expected privateSnapshotIdentity,
	expectedLinks uint16,
	expectedMode uint16,
	expectedAccess int,
) bool {
	current, err := validatePrivateSnapshotFile(
		fd,
		expected.size,
		expectedLinks,
		expectedMode,
		expectedAccess,
		expected.filesystem,
		expected.extendedSecurity,
	)
	return err == nil && current == expected
}

func samePrivateSnapshotSecurityStat(left unix.Stat_t, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino && left.Mode == right.Mode &&
		left.Nlink == right.Nlink && left.Uid == right.Uid && left.Gid == right.Gid &&
		left.Size == right.Size && left.Flags == right.Flags
}

func samePrivateSnapshotObject(left privateSnapshotIdentity, right privateSnapshotIdentity) bool {
	return left.device == right.device && left.inode == right.inode &&
		left.filesystem == right.filesystem && left.extendedSecurity == right.extendedSecurity
}

func samePrivateSnapshotDirectoryObject(
	left privateSnapshotDirectoryIdentity,
	right privateSnapshotDirectoryIdentity,
) bool {
	return left.device == right.device && left.inode == right.inode &&
		left.uid == right.uid && left.gid == right.gid &&
		left.filesystem == right.filesystem && left.extendedSecurity == right.extendedSecurity
}

func validPrivateSnapshotDirectoryCreationTransition(
	before privateSnapshotDirectoryIdentity,
	after privateSnapshotDirectoryIdentity,
) bool {
	// APFS accounts for each linked child in the containing directory's link
	// count, including a regular file. Accept only the exact one-entry change.
	return before.links > 0 && samePrivateSnapshotDirectoryObject(before, after) &&
		after.links == before.links+1
}

func validPrivateSnapshotDirectoryRemovalTransition(
	before privateSnapshotDirectoryIdentity,
	after privateSnapshotDirectoryIdentity,
	expectedRetiredLinks uint64,
) bool {
	return before.links > 0 && expectedRetiredLinks > 0 &&
		samePrivateSnapshotDirectoryObject(before, after) && after.links == expectedRetiredLinks &&
		before.links == after.links+1
}
