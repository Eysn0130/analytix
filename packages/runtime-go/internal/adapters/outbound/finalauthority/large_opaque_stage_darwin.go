//go:build darwin

package finalauthority

import (
	"context"
	"errors"
	"io"
	"os"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	"golang.org/x/sys/unix"
)

const largeOpaqueDarwinMinimumPinnedFD = 3

func largeOpaqueAnonymousPutSupported() bool { return false }

// Darwin has no O_TMPFILE equivalent, so an arbitrary stream cannot be
// committed without leaving a crash-recovery residue. The supported input is
// instead one already-complete exact file descriptor. fclonefileat identifies
// that source by descriptor and atomically creates a non-existing destination;
// this keeps the existing store generation and avoids a named staging object.
func largeOpaquePutInputSupported(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && file != nil
}

func largeOpaqueCreateStagingFile(int, string) (int, bool, uint64, error) {
	return -1, false, 0, ErrLargeOpaqueUnsupportedPlatform
}

func largeOpaquePutExactWithoutAnonymousStaging(
	ctx context.Context,
	authority privateCASRootAuthority,
	reference largeopaqueport.ExactRef,
	reader io.Reader,
) (_ largeopaqueport.PutOutcome, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	sourceFD, err := largeOpaqueDarwinPinSource(reader)
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(sourceFD)) }()
	sourceLinks, err := largeOpaqueDarwinSourceLinks(sourceFD)
	if err != nil {
		return 0, errors.Join(ErrLargeOpaqueSourceMismatch, err)
	}

	root, rootIdentity, err := largeOpaqueUnixOpenRoot(authority)
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(root)) }()
	shardName := reference.Address[:2]
	shard, shardIdentity, err := largeOpaqueUnixOpenShard(root, shardName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, errors.Join(
				ErrLargeOpaqueIndeterminate,
				errors.New("large opaque shard requires authenticated provisioning"),
				err,
			)
		}
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(shard)) }()

	initialSource, err := largeOpaqueDarwinVerifySource(
		ctx,
		sourceFD,
		reference,
		rootIdentity.device,
		sourceLinks,
	)
	if err != nil {
		return 0, err
	}
	if err := unix.Fsync(sourceFD); err != nil {
		return 0, errors.Join(ErrLargeOpaqueSourceRead, err)
	}
	stableSource, err := largeOpaqueUnixFileIdentityForRefWithLinks(
		sourceFD,
		reference,
		rootIdentity.device,
		sourceLinks,
	)
	if err != nil || stableSource != initialSource {
		return 0, errors.Join(
			ErrLargeOpaqueSourceMismatch,
			errors.New("large opaque source changed while becoming durable"),
			err,
		)
	}
	finalName := largeOpaqueCanonicalName(reference.Address)
	if !largeOpaqueValidateCanonicalName(reference.Address, finalName) {
		return 0, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque canonical name is invalid"))
	}

	cloneErr := unix.Fclonefileat(
		sourceFD,
		shard,
		finalName,
		unix.CLONE_NOOWNERCOPY,
	)
	if cloneErr != nil {
		if errors.Is(cloneErr, unix.EEXIST) {
			if err := largeOpaqueUnixVerifyNamed(
				ctx,
				authority,
				rootIdentity,
				shard,
				shardName,
				shardIdentity,
				finalName,
				reference,
				nil,
			); err != nil {
				if errors.Is(err, ErrLargeOpaqueIntegrity) || errors.Is(err, os.ErrNotExist) {
					return 0, errors.Join(
						ErrLargeOpaqueIntegrity,
						errors.New("large opaque existing object conflicts"),
						err,
					)
				}
				return 0, errors.Join(
					errors.New("large opaque existing object could not be verified"),
					err,
				)
			}
			if _, err := largeOpaqueDarwinMakeDurable(
				ctx,
				authority,
				rootIdentity,
				shard,
				shardName,
				shardIdentity,
				finalName,
				reference,
			); err != nil {
				return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
			}
			if err := largeOpaqueDarwinRevalidateSource(
				sourceFD,
				reference,
				rootIdentity.device,
				sourceLinks,
				initialSource,
			); err != nil {
				return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
			}
			return largeopaqueport.PutOutcomeExistingEqual, nil
		}
		if errors.Is(cloneErr, unix.ENOTSUP) || errors.Is(cloneErr, unix.EXDEV) || errors.Is(cloneErr, unix.ENOSYS) {
			return 0, errors.Join(ErrLargeOpaqueUnsupportedPlatform, cloneErr)
		}
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, cloneErr)
	}

	if _, err := largeOpaqueDarwinMakeDurable(
		ctx,
		authority,
		rootIdentity,
		shard,
		shardName,
		shardIdentity,
		finalName,
		reference,
	); err != nil {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
	}
	if err := largeOpaqueDarwinRevalidateSource(
		sourceFD,
		reference,
		rootIdentity.device,
		sourceLinks,
		initialSource,
	); err != nil {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
	}
	return largeopaqueport.PutOutcomeCreated, nil
}

func largeOpaqueDarwinPinSource(reader io.Reader) (int, error) {
	source, ok := reader.(*os.File)
	if !ok || source == nil {
		return -1, ErrLargeOpaqueUnsupportedPlatform
	}
	raw, err := source.SyscallConn()
	if err != nil {
		return -1, errors.Join(ErrLargeOpaqueSourceRead, err)
	}
	pinned := -1
	var pinErr error
	controlErr := raw.Control(func(fd uintptr) {
		pinned, pinErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, largeOpaqueDarwinMinimumPinnedFD)
	})
	if controlErr != nil || pinErr != nil || pinned < 0 {
		if pinned >= 0 {
			_ = unix.Close(pinned)
		}
		return -1, errors.Join(
			ErrLargeOpaqueSourceRead,
			errors.New("large opaque Darwin source descriptor could not be pinned"),
			controlErr,
			pinErr,
		)
	}
	return pinned, nil
}

func largeOpaqueDarwinRevalidateSource(
	fd int,
	reference largeopaqueport.ExactRef,
	expectedDevice uint64,
	expectedLinks uint64,
	expected largeOpaqueUnixFileIdentity,
) error {
	current, err := largeOpaqueUnixFileIdentityForRefWithLinks(
		fd,
		reference,
		expectedDevice,
		expectedLinks,
	)
	if err != nil || current != expected {
		return errors.Join(errors.New("large opaque source changed during atomic clone"), err)
	}
	return nil
}

func largeOpaqueDarwinMakeDurable(
	ctx context.Context,
	authority privateCASRootAuthority,
	rootIdentity largeOpaqueUnixDirectoryIdentity,
	shard int,
	shardName string,
	shardIdentity largeOpaqueUnixDirectoryIdentity,
	name string,
	reference largeopaqueport.ExactRef,
) (largeOpaqueUnixFileIdentity, error) {
	committedIdentity, err := largeOpaqueDarwinSyncClonedFile(
		ctx,
		authority,
		rootIdentity,
		shard,
		shardName,
		shardIdentity,
		name,
		reference,
	)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	if err := unix.Fsync(shard); err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	if err := largeOpaqueUnixRevalidateTopology(
		authority,
		rootIdentity,
		shardName,
		shardIdentity,
		name,
		reference,
		committedIdentity,
	); err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	return committedIdentity, nil
}

func largeOpaqueDarwinSyncClonedFile(
	ctx context.Context,
	authority privateCASRootAuthority,
	rootIdentity largeOpaqueUnixDirectoryIdentity,
	shard int,
	shardName string,
	shardIdentity largeOpaqueUnixDirectoryIdentity,
	name string,
	reference largeopaqueport.ExactRef,
) (_ largeOpaqueUnixFileIdentity, resultErr error) {
	fd, err := largeOpaqueUnixOpenExactNamedFile(shard, name)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	initial, err := largeOpaqueUnixVerifyFD(ctx, fd, reference, nil, 1)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	if err := unix.Fsync(fd); err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	current, err := largeOpaqueUnixFileIdentityForRef(
		fd,
		reference,
		rootIdentity.device,
	)
	if err != nil || current != initial {
		return largeOpaqueUnixFileIdentity{}, errors.Join(
			errors.New("large opaque cloned file changed while becoming durable"),
			err,
		)
	}
	if err := largeOpaqueUnixRevalidateTopology(
		authority,
		rootIdentity,
		shardName,
		shardIdentity,
		name,
		reference,
		current,
	); err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	return current, nil
}

func largeOpaqueDarwinSourceLinks(fd int) (uint64, error) {
	statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	var stat unix.Stat_t
	if statusErr != nil || descriptorErr != nil || unix.Fstat(fd, &stat) != nil ||
		statusFlags&unix.O_ACCMODE != unix.O_RDONLY || descriptorFlags&unix.FD_CLOEXEC == 0 ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) ||
		uint32(stat.Mode)&0o7777 != 0o400 || (stat.Nlink != 0 && stat.Nlink != 1) ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) || !largeOpaquePlatformObjectSafe(fd, stat) {
		return 0, errors.New("large opaque Darwin source descriptor is unsafe")
	}
	return uint64(stat.Nlink), nil
}

func largeOpaqueDarwinVerifySource(
	ctx context.Context,
	fd int,
	reference largeopaqueport.ExactRef,
	expectedDevice uint64,
	expectedLinks uint64,
) (largeOpaqueUnixFileIdentity, error) {
	identity, err := largeOpaqueUnixFileIdentityForRefWithLinks(
		fd,
		reference,
		expectedDevice,
		expectedLinks,
	)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueSourceMismatch, err)
	}
	verified, err := largeOpaqueUnixVerifyFD(ctx, fd, reference, nil, expectedLinks)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return largeOpaqueUnixFileIdentity{}, ctxErr
		}
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueSourceMismatch, err)
	}
	if verified != identity || verified.device != expectedDevice {
		return largeOpaqueUnixFileIdentity{}, errors.Join(
			ErrLargeOpaqueSourceMismatch,
			errors.New("large opaque Darwin source identity changed during verification"),
		)
	}
	return verified, nil
}
