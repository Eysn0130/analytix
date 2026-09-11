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

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

const darwinImmutableInstallMinimumFDV1 = 8

type darwinImmutableInstallSourceIdentityV1 struct {
	dev       uint64
	ino       uint64
	size      int64
	mode      uint16
	nlink     uint64
	uid       uint32
	gid       uint32
	flags     uint32
	ctimeSec  int64
	ctimeNsec int64
}

func (source *Source) installExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	input *os.File,
) (_ fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, resultErr error) {
	inputFD, err := pinDarwinImmutableInstallSourceV1(input)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(inputFD)) }()

	directoryFD, err := source.openOrCreateSnapshotDirectoryDarwinV1(object.CaseID)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(directoryFD)) }()

	directoryIdentity, err := exactDarwinDirectoryIdentityV1(directoryFD)
	if err != nil {
		return "", err
	}
	sourceIdentity, err := verifyDarwinImmutableInstallSourceV1(ctx, inputFD, object, uint64(directoryIdentity.dev))
	if err != nil {
		return "", err
	}
	if err := unix.Fsync(inputFD); err != nil {
		return "", errors.Join(fundsquerysourceport.ErrUnavailable, err)
	}
	if current, identityErr := inspectDarwinImmutableInstallSourceV1(inputFD, object, uint64(directoryIdentity.dev)); identityErr != nil || current != sourceIdentity {
		return "", errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds immutable snapshot source changed before installation"),
			identityErr,
		)
	}

	name := object.DuckDBSHA256 + ".duckdb"
	cloneErr := unix.Fclonefileat(inputFD, directoryFD, name, unix.CLONE_NOOWNERCOPY)
	if cloneErr != nil && !errors.Is(cloneErr, unix.EEXIST) {
		if errors.Is(cloneErr, unix.ENOTSUP) || errors.Is(cloneErr, unix.EXDEV) || errors.Is(cloneErr, unix.ENOSYS) {
			return "", errors.Join(fundsquerysourceport.ErrUnavailable, cloneErr)
		}
		return "", errors.Join(fundsquerysourceport.ErrCorrupt, cloneErr)
	}

	if err := verifyDarwinInstalledSnapshotObjectV1(ctx, directoryFD, name, object); err != nil {
		return "", err
	}
	if err := unix.Fsync(directoryFD); err != nil {
		return "", errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	if err := verifyDarwinInstalledSnapshotObjectV1(ctx, directoryFD, name, object); err != nil {
		return "", err
	}
	if current, identityErr := verifyDarwinImmutableInstallSourceV1(
		ctx, inputFD, object, uint64(directoryIdentity.dev),
	); identityErr != nil || current != sourceIdentity {
		return "", errors.Join(
			fundsquerysourceport.ErrCorrupt,
			errors.New("funds immutable snapshot source changed during installation"),
			identityErr,
		)
	}
	heldDirectoryIdentity, err := exactDarwinDirectoryIdentityV1(directoryFD)
	if err != nil {
		return "", errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	currentDirectoryFD, currentDirectoryIdentity, err := source.openExistingSnapshotDirectoryDarwinV1(object.CaseID)
	if err != nil {
		return "", errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	currentVerifyErr := verifyDarwinInstalledSnapshotObjectV1(ctx, currentDirectoryFD, name, object)
	currentCloseErr := unix.Close(currentDirectoryFD)
	if currentVerifyErr != nil || currentCloseErr != nil || currentDirectoryIdentity != heldDirectoryIdentity {
		return "", errors.Join(
			fundsquerysourceport.ErrCorrupt,
			errors.New("funds immutable snapshot destination path changed during installation"),
			currentVerifyErr,
			currentCloseErr,
		)
	}
	if errors.Is(cloneErr, unix.EEXIST) {
		return fundsquerysourceport.ImmutableSnapshotInstallExistingEqualV1, nil
	}
	return fundsquerysourceport.ImmutableSnapshotInstallCreatedV1, nil
}

func pinDarwinImmutableInstallSourceV1(input *os.File) (int, error) {
	if input == nil {
		return -1, fundsquerysourceport.ErrMismatch
	}
	raw, err := input.SyscallConn()
	if err != nil {
		return -1, errors.Join(fundsquerysourceport.ErrUnavailable, err)
	}
	pinned := -1
	var pinErr error
	controlErr := raw.Control(func(fd uintptr) {
		pinned, pinErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, darwinImmutableInstallMinimumFDV1)
	})
	if controlErr != nil || pinErr != nil || pinned < 0 {
		if pinned >= 0 {
			_ = unix.Close(pinned)
		}
		return -1, errors.Join(fundsquerysourceport.ErrUnavailable, controlErr, pinErr)
	}
	return pinned, nil
}

func (source *Source) openOrCreateSnapshotDirectoryDarwinV1(caseID string) (int, error) {
	if source == nil || strings.Contains(caseID, "/") || strings.ContainsRune(caseID, 0) {
		return -1, fundsquerysourceport.ErrMismatch
	}
	currentFD, _, err := openAbsoluteDirectoryNoFollowDarwinV1(source.userDataRoot)
	if err != nil {
		return -1, mapOpenErrorV1(err)
	}
	for _, name := range []string{
		dataAnalysisDirectoryV1,
		casesDirectoryV1,
		caseID,
		snapshotsDirectoryV1,
	} {
		nextFD, openErr := openOrCreatePrivateDarwinDirectoryAtV1(currentFD, name)
		closeErr := unix.Close(currentFD)
		if openErr != nil || closeErr != nil {
			if nextFD >= 0 {
				_ = unix.Close(nextFD)
			}
			return -1, errors.Join(openErr, closeErr)
		}
		currentFD = nextFD
	}
	return currentFD, nil
}

func (source *Source) openExistingSnapshotDirectoryDarwinV1(
	caseID string,
) (int, darwinDirectoryIdentityV1, error) {
	if source == nil || strings.Contains(caseID, "/") || strings.ContainsRune(caseID, 0) {
		return -1, darwinDirectoryIdentityV1{}, fundsquerysourceport.ErrMismatch
	}
	currentFD, directoryIdentity, err := openAbsoluteDirectoryNoFollowDarwinV1(source.userDataRoot)
	if err != nil {
		return -1, darwinDirectoryIdentityV1{}, mapOpenErrorV1(err)
	}
	for _, name := range []string{
		dataAnalysisDirectoryV1,
		casesDirectoryV1,
		caseID,
		snapshotsDirectoryV1,
	} {
		nextFD, nextIdentity, openErr := openPrivateDirectoryAtDarwinV1(currentFD, name)
		closeErr := unix.Close(currentFD)
		if openErr != nil || closeErr != nil {
			if nextFD >= 0 {
				_ = unix.Close(nextFD)
			}
			return -1, darwinDirectoryIdentityV1{}, errors.Join(openErr, closeErr)
		}
		currentFD = nextFD
		directoryIdentity = nextIdentity
	}
	return currentFD, directoryIdentity, nil
}

func openOrCreatePrivateDarwinDirectoryAtV1(parentFD int, name string) (int, error) {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.ContainsRune(name, 0) {
		return -1, fundsquerysourceport.ErrMismatch
	}
	fd, _, err := openPrivateDirectoryAtDarwinV1(parentFD, name)
	if err == nil {
		return fd, nil
	}
	if !errors.Is(err, unix.ENOENT) {
		return -1, errors.Join(fundsquerysourceport.ErrMismatch, err)
	}
	created := false
	if mkdirErr := unix.Mkdirat(parentFD, name, 0o700); mkdirErr == nil {
		created = true
	} else if !errors.Is(mkdirErr, unix.EEXIST) {
		return -1, errors.Join(fundsquerysourceport.ErrUnavailable, mkdirErr)
	}
	if !created {
		fd, _, err = openPrivateDirectoryAtDarwinV1(parentFD, name)
		if err != nil {
			return -1, errors.Join(fundsquerysourceport.ErrMismatch, err)
		}
		return fd, nil
	}
	fd, err = unix.Openat(
		parentFD,
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return -1, errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	if chmodErr := unix.Fchmod(fd, 0o700); chmodErr != nil {
		_ = unix.Close(fd)
		return -1, errors.Join(fundsquerysourceport.ErrCorrupt, chmodErr)
	}
	if _, identityErr := exactDarwinDirectoryIdentityV1(fd); identityErr != nil {
		_ = unix.Close(fd)
		return -1, errors.Join(fundsquerysourceport.ErrCorrupt, identityErr)
	}
	if syncErr := errors.Join(unix.Fsync(fd), unix.Fsync(parentFD)); syncErr != nil {
		_ = unix.Close(fd)
		return -1, errors.Join(fundsquerysourceport.ErrCorrupt, syncErr)
	}
	return fd, nil
}

func inspectDarwinImmutableInstallSourceV1(
	fd int,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	expectedDevice uint64,
) (darwinImmutableInstallSourceIdentityV1, error) {
	statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	var stat unix.Stat_t
	if statusErr != nil || descriptorErr != nil || unix.Fstat(fd, &stat) != nil ||
		statusFlags&unix.O_ACCMODE != unix.O_RDONLY || descriptorFlags&unix.FD_CLOEXEC == 0 ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 0 ||
		stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) ||
		stat.Mode&0o7777 != 0o400 || stat.Flags != 0 || stat.Size <= 0 ||
		uint64(stat.Size) != object.DuckDBByteLength || uint64(stat.Dev) != expectedDevice ||
		!darwinSourceExtendedSecuritySafeV1(fd) || verifyDarwinFilesystemV1(fd) != nil {
		return darwinImmutableInstallSourceIdentityV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds immutable snapshot source identity is invalid"),
		)
	}
	return darwinImmutableInstallSourceIdentityV1{
		dev: uint64(stat.Dev), ino: uint64(stat.Ino), size: stat.Size,
		mode: stat.Mode, nlink: uint64(stat.Nlink), uid: stat.Uid, gid: stat.Gid,
		flags: stat.Flags, ctimeSec: stat.Ctim.Sec, ctimeNsec: stat.Ctim.Nsec,
	}, nil
}

func verifyDarwinImmutableInstallSourceV1(
	ctx context.Context,
	fd int,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	expectedDevice uint64,
) (darwinImmutableInstallSourceIdentityV1, error) {
	identity, err := inspectDarwinImmutableInstallSourceV1(fd, object, expectedDevice)
	if err != nil {
		return darwinImmutableInstallSourceIdentityV1{}, err
	}
	if err := verifyDarwinInstallDigestV1(ctx, fd, object.DuckDBByteLength, object.DuckDBSHA256); err != nil {
		return darwinImmutableInstallSourceIdentityV1{}, err
	}
	confirmed, err := inspectDarwinImmutableInstallSourceV1(fd, object, expectedDevice)
	if err != nil || confirmed != identity {
		return darwinImmutableInstallSourceIdentityV1{}, errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("funds immutable snapshot source identity is unstable"),
			err,
		)
	}
	return identity, nil
}

func verifyDarwinInstalledSnapshotObjectV1(
	ctx context.Context,
	directoryFD int,
	name string,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
) (resultErr error) {
	fd, err := unix.Openat(directoryFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) ||
		stat.Mode&0o7777 != 0o400 || stat.Flags != 0 || stat.Size <= 0 ||
		uint64(stat.Size) != object.DuckDBByteLength ||
		!darwinSourceExtendedSecuritySafeV1(fd) || verifyDarwinFilesystemV1(fd) != nil ||
		ensureNoDarwinWALV1(directoryFD, name) != nil {
		return errors.Join(
			fundsquerysourceport.ErrCorrupt,
			errors.New("funds immutable snapshot destination identity is invalid"),
		)
	}
	if err := unix.Fsync(fd); err != nil {
		return errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	if err := verifyDarwinInstallDigestV1(ctx, fd, object.DuckDBByteLength, object.DuckDBSHA256); err != nil {
		return errors.Join(fundsquerysourceport.ErrCorrupt, err)
	}
	return nil
}

func verifyDarwinInstallDigestV1(
	ctx context.Context,
	fd int,
	expectedLength uint64,
	expectedSHA256 string,
) error {
	if ctx == nil || expectedLength == 0 || expectedLength > uint64(^uint64(0)>>1) {
		return fundsquerysourceport.ErrMismatch
	}
	hasher := sha256.New()
	buffer := make([]byte, darwinExactCopyChunkBytesV1)
	var offset int64
	for uint64(offset) < expectedLength {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := int64(expectedLength) - offset
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
			if errors.Is(readErr, io.EOF) {
				return fundsquerysourceport.ErrMismatch
			}
			return errors.Join(fundsquerysourceport.ErrUnavailable, readErr)
		}
		if read == 0 && readErr == nil {
			return fundsquerysourceport.ErrMismatch
		}
	}
	extra := []byte{0}
	if read, readErr := unix.Pread(fd, extra, int64(expectedLength)); read != 0 ||
		(readErr != nil && !errors.Is(readErr, io.EOF)) {
		return fundsquerysourceport.ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedSHA256 {
		return fundsquerysourceport.ErrMismatch
	}
	return nil
}
