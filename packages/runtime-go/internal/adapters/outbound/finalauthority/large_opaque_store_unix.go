//go:build darwin || linux

package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"

	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	"golang.org/x/sys/unix"
)

const largeOpaqueStreamBufferBytes = 64 << 10

type largeOpaqueUnixDirectoryIdentity struct {
	device uint64
	inode  uint64
	mode   uint32
	uid    uint32
	gid    uint32
}

type largeOpaqueUnixFileIdentity struct {
	device uint64
	inode  uint64
	mode   uint32
	links  uint64
	uid    uint32
	gid    uint32
	size   int64
	times  [32]byte
}

func largeOpaquePutExactPlatform(
	ctx context.Context,
	authority privateCASRootAuthority,
	reference largeopaqueport.ExactRef,
	reader io.Reader,
) (_ largeopaqueport.PutOutcome, resultErr error) {
	if !largeOpaqueAnonymousPutSupported() {
		return largeOpaquePutExactWithoutAnonymousStaging(
			ctx,
			authority,
			reference,
			reader,
		)
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

	finalName := largeOpaqueCanonicalName(reference.Address)
	if !largeOpaqueValidateCanonicalName(reference.Address, finalName) {
		return 0, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque canonical name is invalid"))
	}
	temporaryFD, temporaryNamed, stagingLinks, err := largeOpaqueCreateStagingFile(shard, "")
	if err != nil {
		return 0, err
	}
	temporaryOpen := true
	defer func() {
		if temporaryOpen {
			resultErr = errors.Join(resultErr, unix.Close(temporaryFD))
		}
	}()
	if temporaryNamed || stagingLinks != 0 {
		return 0, errors.Join(
			ErrLargeOpaqueIndeterminate,
			errors.New("large opaque live write requires anonymous staging"),
		)
	}
	createdIdentity, err := largeOpaqueUnixCreatedTempIdentity(
		temporaryFD, rootIdentity.device, stagingLinks,
	)
	if err != nil {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
	}
	if err := unix.Fchmod(temporaryFD, 0o600); err != nil {
		return 0, err
	}
	preparedIdentity, err := largeOpaqueUnixTempIdentity(
		temporaryFD, reference.ByteLength, rootIdentity.device, 0o600, stagingLinks,
	)
	if err != nil || preparedIdentity.device != createdIdentity.device || preparedIdentity.inode != createdIdentity.inode {
		return 0, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque created temp identity changed"), err)
	}

	if err := largeOpaqueUnixWriteReader(ctx, temporaryFD, reference, reader); err != nil {
		return 0, err
	}
	if err := unix.Fchmod(temporaryFD, 0o400); err != nil {
		return 0, err
	}
	if err := unix.Fsync(temporaryFD); err != nil {
		return 0, err
	}
	stagedIdentity, err := largeOpaqueUnixFileIdentityForRefWithLinks(
		temporaryFD,
		reference,
		rootIdentity.device,
		stagingLinks,
	)
	if err != nil || stagedIdentity.device != createdIdentity.device || stagedIdentity.inode != createdIdentity.inode {
		return 0, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque staged object identity changed"), err)
	}
	if _, err := largeOpaqueUnixVerifyFD(ctx, temporaryFD, reference, nil, stagingLinks); err != nil {
		return 0, err
	}

	committedSourceLinks, commitErr := largeOpaqueCommitNoReplace(
		shard, "", temporaryFD, finalName,
	)
	if errors.Is(commitErr, os.ErrExist) {
		if err := unix.Fsync(shard); err != nil {
			return 0, err
		}
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
				return 0, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque existing object conflicts"), err)
			}
			return 0, errors.Join(errors.New("large opaque existing object could not be verified"), err)
		}
		return largeopaqueport.PutOutcomeExistingEqual, nil
	}
	if commitErr != nil {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, commitErr)
	}
	if committedSourceLinks != 1 {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, errors.New("large opaque commit returned an invalid source link count"))
	}
	if err := largeOpaquePlatformValidateCommitWitness(
		shard, finalName, reference, stagedIdentity,
	); err != nil {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
	}
	if err := unix.Fsync(shard); err != nil {
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
	}
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
		return 0, errors.Join(ErrLargeOpaqueIndeterminate, err)
	}
	return largeopaqueport.PutOutcomeCreated, nil
}

func largeOpaqueReadExactPlatform(
	ctx context.Context,
	authority privateCASRootAuthority,
	reference largeopaqueport.ExactRef,
	writer io.Writer,
) (resultErr error) {
	root, rootIdentity, err := largeOpaqueUnixOpenRoot(authority)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(root)) }()
	shardName := reference.Address[:2]
	shard, shardIdentity, err := largeOpaqueUnixOpenShard(root, shardName)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(shard)) }()
	return largeOpaqueUnixVerifyNamed(
		ctx,
		authority,
		rootIdentity,
		shard,
		shardName,
		shardIdentity,
		largeOpaqueCanonicalName(reference.Address),
		reference,
		writer,
	)
}

func largeOpaqueUnixOpenRoot(
	authority privateCASRootAuthority,
) (int, largeOpaqueUnixDirectoryIdentity, error) {
	root, err := authority.open()
	if err != nil {
		return -1, largeOpaqueUnixDirectoryIdentity{}, err
	}
	identity, err := largeOpaqueUnixDirectoryIdentityOf(root, authority.dev)
	if err != nil {
		_ = unix.Close(root)
		return -1, largeOpaqueUnixDirectoryIdentity{}, err
	}
	return root, identity, nil
}

func largeOpaqueUnixOpenShard(
	root int,
	name string,
) (int, largeOpaqueUnixDirectoryIdentity, error) {
	if !validPrivateShard(name) {
		return -1, largeOpaqueUnixDirectoryIdentity{}, errors.New("large opaque shard name is invalid")
	}
	device, err := privateCASUnixDirectoryDevice(root)
	if err != nil {
		return -1, largeOpaqueUnixDirectoryIdentity{}, err
	}
	shard, present, err := privateCASUnixOpenExactBoundDirectory(root, name, device)
	if err != nil {
		return -1, largeOpaqueUnixDirectoryIdentity{}, err
	}
	if !present {
		return -1, largeOpaqueUnixDirectoryIdentity{}, os.ErrNotExist
	}
	identity, err := largeOpaqueUnixDirectoryIdentityOf(shard, device)
	if err != nil {
		_ = unix.Close(shard)
		return -1, largeOpaqueUnixDirectoryIdentity{}, err
	}
	return shard, identity, nil
}

func largeOpaqueUnixDirectoryIdentityOf(
	fd int,
	expectedDevice uint64,
) (largeOpaqueUnixDirectoryIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(os.Geteuid()) || uint32(stat.Mode)&0o7077 != 0 ||
		uint64(stat.Dev) != expectedDevice || !existingPrivateAuthorityExtendedSecuritySafe(fd) ||
		!largeOpaquePlatformObjectSafe(fd, stat) {
		return largeOpaqueUnixDirectoryIdentity{}, errors.New("large opaque directory identity or permissions are unsafe")
	}
	return largeOpaqueUnixDirectoryIdentity{
		device: uint64(stat.Dev),
		inode:  stat.Ino,
		mode:   uint32(stat.Mode),
		uid:    stat.Uid,
		gid:    stat.Gid,
	}, nil
}

func largeOpaqueUnixFileIdentityForRef(
	fd int,
	reference largeopaqueport.ExactRef,
	expectedDevice uint64,
) (largeOpaqueUnixFileIdentity, error) {
	return largeOpaqueUnixFileIdentityForRefWithLinks(fd, reference, expectedDevice, 1)
}

func largeOpaqueUnixFileIdentityForRefWithLinks(
	fd int,
	reference largeopaqueport.ExactRef,
	expectedDevice uint64,
	expectedLinks uint64,
) (largeOpaqueUnixFileIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Geteuid()) || uint32(stat.Mode)&0o7777 != 0o400 || uint64(stat.Nlink) != expectedLinks ||
		stat.Size != int64(reference.ByteLength) || expectedDevice != 0 && uint64(stat.Dev) != expectedDevice ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) || !largeOpaquePlatformObjectSafe(fd, stat) {
		return largeOpaqueUnixFileIdentity{}, errors.New("large opaque file identity, links, size, mode, or security metadata are unsafe")
	}
	return largeOpaqueUnixFileIdentity{
		device: uint64(stat.Dev),
		inode:  stat.Ino,
		mode:   uint32(stat.Mode),
		links:  uint64(stat.Nlink),
		uid:    stat.Uid,
		gid:    stat.Gid,
		size:   stat.Size,
		times:  privateCASUnixStatTimes(stat),
	}, nil
}

func largeOpaqueUnixTempIdentity(
	fd int,
	maxBytes uint64,
	expectedDevice uint64,
	expectedMode uint32,
	expectedLinks uint64,
) (largeOpaqueUnixFileIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Geteuid()) || uint32(stat.Mode)&0o7777 != expectedMode || uint64(stat.Nlink) != expectedLinks ||
		stat.Size < 0 || uint64(stat.Size) > maxBytes || expectedDevice != 0 && uint64(stat.Dev) != expectedDevice ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) || !largeOpaquePlatformObjectSafe(fd, stat) {
		return largeOpaqueUnixFileIdentity{}, errors.New("large opaque temp identity, links, size, mode, or security metadata are unsafe")
	}
	return largeOpaqueUnixFileIdentity{
		device: uint64(stat.Dev),
		inode:  stat.Ino,
		mode:   uint32(stat.Mode),
		links:  uint64(stat.Nlink),
		uid:    stat.Uid,
		gid:    stat.Gid,
		size:   stat.Size,
		times:  privateCASUnixStatTimes(stat),
	}, nil
}

func largeOpaqueUnixCreatedTempIdentity(
	fd int,
	expectedDevice uint64,
	expectedLinks uint64,
) (largeOpaqueUnixFileIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Geteuid()) || uint64(stat.Nlink) != expectedLinks || stat.Size != 0 ||
		uint64(stat.Dev) != expectedDevice {
		return largeOpaqueUnixFileIdentity{}, errors.New("large opaque created temp identity is unsafe")
	}
	return largeOpaqueUnixFileIdentity{
		device: uint64(stat.Dev),
		inode:  stat.Ino,
		mode:   uint32(stat.Mode),
		links:  uint64(stat.Nlink),
		uid:    stat.Uid,
		gid:    stat.Gid,
		size:   stat.Size,
		times:  privateCASUnixStatTimes(stat),
	}, nil
}

func largeOpaqueUnixWriteReader(
	ctx context.Context,
	fd int,
	reference largeopaqueport.ExactRef,
	reader io.Reader,
) error {
	hasher := sha256.New()
	buffer := make([]byte, largeOpaqueStreamBufferBytes)
	defer clear(buffer)
	remaining := reference.ByteLength
	sawEOF := false
	for remaining > 0 {
		if err := contextErr(ctx); err != nil {
			return err
		}
		want := uint64(len(buffer))
		if remaining < want {
			want = remaining
		}
		read, readErr := reader.Read(buffer[:int(want)])
		if read < 0 || read > int(want) {
			return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source reader returned an invalid byte count"))
		}
		if read > 0 {
			if err := largeOpaqueUnixWriteAll(ctx, fd, buffer[:read]); err != nil {
				return err
			}
			_, _ = hasher.Write(buffer[:read])
			remaining -= uint64(read)
		}
		if readErr != nil {
			if readErr != io.EOF || remaining != 0 {
				if readErr == io.EOF {
					return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source ended before its exact length"))
				}
				if err := contextErr(ctx); err != nil {
					return err
				}
				return errors.Join(ErrLargeOpaqueSourceRead, errors.New("large opaque source reader returned an error"))
			}
			sawEOF = true
			break
		}
		if read == 0 {
			return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source reader made no progress"))
		}
	}
	if !sawEOF {
		var extra [1]byte
		read, readErr := reader.Read(extra[:])
		switch {
		case read > 0:
			return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source exceeded its exact length"))
		case read < 0 || read > len(extra):
			return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source reader returned an invalid extra byte count"))
		case readErr == io.EOF:
		case readErr != nil:
			if err := contextErr(ctx); err != nil {
				return err
			}
			return errors.Join(ErrLargeOpaqueSourceRead, errors.New("large opaque source extra-byte probe returned an error"))
		default:
			return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source extra-byte probe made no progress"))
		}
	}
	if hex.EncodeToString(hasher.Sum(nil)) != reference.SHA256 {
		return errors.Join(ErrLargeOpaqueSourceMismatch, errors.New("large opaque source SHA-256 mismatched"))
	}
	return nil
}

func largeOpaqueUnixWriteAll(ctx context.Context, fd int, body []byte) error {
	for len(body) > 0 {
		if err := contextErr(ctx); err != nil {
			return err
		}
		written, err := unix.Write(fd, body)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if written <= 0 || written > len(body) {
			return errors.New("large opaque file write made invalid progress")
		}
		body = body[written:]
	}
	return nil
}

func largeOpaqueUnixVerifyNamed(
	ctx context.Context,
	authority privateCASRootAuthority,
	rootIdentity largeOpaqueUnixDirectoryIdentity,
	shard int,
	shardName string,
	shardIdentity largeOpaqueUnixDirectoryIdentity,
	name string,
	reference largeopaqueport.ExactRef,
	writer io.Writer,
) (resultErr error) {
	if !largeOpaqueValidateCanonicalName(reference.Address, name) {
		return errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque exact filename is invalid"))
	}
	fd, err := largeOpaqueUnixOpenExactNamedFile(shard, name)
	if errors.Is(err, unix.ENOENT) {
		return os.ErrNotExist
	}
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	initial, err := largeOpaqueUnixFileIdentityForRef(fd, reference, rootIdentity.device)
	if err != nil {
		return errors.Join(ErrLargeOpaqueIntegrity, err)
	}
	verified, err := largeOpaqueUnixVerifyFD(ctx, fd, reference, nil, 1)
	if err != nil {
		return err
	}
	if verified != initial {
		return errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque pre-read verification changed identity"))
	}
	if writer != nil {
		verified, err = largeOpaqueUnixVerifyFD(ctx, fd, reference, writer, 1)
		if err != nil {
			return err
		}
		if verified != initial {
			return errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque delivery verification changed identity"))
		}
	}
	if err := largeOpaqueUnixRevalidateTopology(
		authority,
		rootIdentity,
		shardName,
		shardIdentity,
		name,
		reference,
		initial,
	); err != nil {
		return errors.Join(ErrLargeOpaqueIntegrity, err)
	}
	return nil
}

func largeOpaqueUnixVerifyFD(
	ctx context.Context,
	fd int,
	reference largeopaqueport.ExactRef,
	writer io.Writer,
	expectedLinks uint64,
) (largeOpaqueUnixFileIdentity, error) {
	initial, err := largeOpaqueUnixFileIdentityForRefWithLinks(fd, reference, 0, expectedLinks)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, err)
	}
	if _, err := unix.Seek(fd, 0, io.SeekStart); err != nil {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, err)
	}
	hasher := sha256.New()
	buffer := make([]byte, largeOpaqueStreamBufferBytes)
	defer clear(buffer)
	remaining := reference.ByteLength
	for remaining > 0 {
		if err := contextErr(ctx); err != nil {
			return largeOpaqueUnixFileIdentity{}, err
		}
		want := uint64(len(buffer))
		if remaining < want {
			want = remaining
		}
		read, readErr := unix.Read(fd, buffer[:int(want)])
		if errors.Is(readErr, unix.EINTR) {
			continue
		}
		if readErr != nil {
			return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, readErr)
		}
		if read <= 0 || read > int(want) {
			return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque file ended before its exact length"))
		}
		_, _ = hasher.Write(buffer[:read])
		if writer != nil {
			written, writeErr := writer.Write(buffer[:read])
			if writeErr != nil {
				return largeOpaqueUnixFileIdentity{}, errors.Join(
					ErrLargeOpaqueDelivery,
					errors.New("large opaque downstream writer returned an error"),
				)
			}
			if written != read {
				return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueDelivery, io.ErrShortWrite)
			}
		}
		remaining -= uint64(read)
	}
	var extra [1]byte
	read, readErr := unix.Read(fd, extra[:])
	if errors.Is(readErr, unix.EINTR) {
		read, readErr = unix.Read(fd, extra[:])
	}
	if read != 0 || readErr != nil {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque file exceeded its exact length"), readErr)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != reference.SHA256 {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque file SHA-256 mismatched"))
	}
	current, err := largeOpaqueUnixFileIdentityForRefWithLinks(fd, reference, initial.device, expectedLinks)
	if err != nil || current != initial {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, errors.New("large opaque file changed during exact read"), err)
	}
	return current, nil
}

func largeOpaqueUnixRevalidateTopology(
	authority privateCASRootAuthority,
	rootIdentity largeOpaqueUnixDirectoryIdentity,
	shardName string,
	shardIdentity largeOpaqueUnixDirectoryIdentity,
	name string,
	reference largeopaqueport.ExactRef,
	fileIdentity largeOpaqueUnixFileIdentity,
) (resultErr error) {
	root, currentRoot, err := largeOpaqueUnixOpenRoot(authority)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(root)) }()
	if currentRoot != rootIdentity {
		return errors.New("large opaque root identity changed")
	}
	shard, currentShard, err := largeOpaqueUnixOpenShard(root, shardName)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(shard)) }()
	if currentShard != shardIdentity {
		return errors.New("large opaque shard identity changed")
	}
	fd, err := largeOpaqueUnixOpenExactNamedFile(shard, name)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	currentFile, err := largeOpaqueUnixFileIdentityForRef(fd, reference, rootIdentity.device)
	if err != nil || currentFile != fileIdentity {
		return errors.Join(errors.New("large opaque exact name changed identity"), err)
	}
	return nil
}

func largeOpaqueUnixCommitWitnessIdentity(
	parent int,
	name string,
	reference largeopaqueport.ExactRef,
	expectedDevice uint64,
) (_ largeOpaqueUnixFileIdentity, resultErr error) {
	fd, err := largeOpaqueUnixOpenExactNamedFile(parent, name)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	identity, err := largeOpaqueUnixFileIdentityForRef(fd, reference, expectedDevice)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, errors.Join(ErrLargeOpaqueIntegrity, err)
	}
	return identity, nil
}

// largeOpaqueUnixOpenExactNamedFile opens only the supplied ASCII basename,
// then asks the platform-specific implementation to prove that the kernel did
// not resolve it through case-folding or a differently-cased alias.
func largeOpaqueUnixOpenExactNamedFile(parent int, name string) (int, error) {
	fd, err := unix.Openat(
		parent,
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if errors.Is(err, unix.ENOENT) {
		alias := strings.ToUpper(name)
		if alias == name {
			return -1, errors.New("large opaque exact basename has no independent case probe")
		}
		var aliasStat unix.Stat_t
		aliasErr := unix.Fstatat(parent, alias, &aliasStat, unix.AT_SYMLINK_NOFOLLOW)
		if aliasErr == nil {
			return -1, errors.Join(
				ErrLargeOpaqueIntegrity,
				errors.New("large opaque exact name aliases a differently-cased entry"),
			)
		}
		if errors.Is(aliasErr, unix.ENOENT) {
			return -1, os.ErrNotExist
		}
		return -1, aliasErr
	}
	if err != nil {
		var stat unix.Stat_t
		statErr := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		if statErr == nil {
			return -1, errors.Join(
				ErrLargeOpaqueIntegrity,
				errors.New("large opaque exact name exists but is not safely openable"),
				err,
			)
		}
		if !errors.Is(statErr, unix.ENOENT) {
			return -1, errors.Join(err, statErr)
		}
		return -1, err
	}
	if err := largeOpaquePlatformExactBasename(parent, fd, name); err != nil {
		return -1, errors.Join(ErrLargeOpaqueIntegrity, err, unix.Close(fd))
	}
	return fd, nil
}
