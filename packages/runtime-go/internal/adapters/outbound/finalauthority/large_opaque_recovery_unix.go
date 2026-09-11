//go:build darwin || linux

package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

type largeOpaqueRecoveryAccumulator struct {
	xor [32]byte
	sum [32]byte
}

type largeOpaqueRecoveryShardObservation struct {
	present  bool
	identity largeOpaqueUnixDirectoryIdentity
}

type largeOpaqueRecoveryCreateResidueObservation struct {
	present  bool
	identity privateCASCreateResidueUnixExactIdentity
}

type largeOpaqueRecoveryObservation struct {
	root                largeOpaqueUnixDirectoryIdentity
	rootCreateResidue   largeOpaqueRecoveryCreateResidueObservation
	shards              [256]largeOpaqueRecoveryShardObservation
	shardCreateResidues [256]largeOpaqueRecoveryCreateResidueObservation
	shardCount          uint32
	createResidueCount  uint32
	objectCount         uint64
	tempCount           uint64
	committedObjects    largeOpaqueRecoveryAccumulator
	temporaryObjects    largeOpaqueRecoveryAccumulator
}

type largeOpaqueRecoveryEntryKind byte

const (
	largeOpaqueRecoveryEntryCommitted largeOpaqueRecoveryEntryKind = 1
	largeOpaqueRecoveryEntryTemporary largeOpaqueRecoveryEntryKind = 2
)

func largeOpaqueRecoveryPlatformSupported() bool { return true }

func largeOpaqueObserveRecoveryPlatform(
	ctx context.Context,
	binding privatecasport.RootBinding,
	maxBytes uint64,
	limits LargeOpaqueRecoveryLimitsV1,
) (_ privateCASRootAuthority, _ bool, _ largeOpaqueRecoveryObservation, resultErr error) {
	components, err := privateCASUnixRelativeComponents(binding.RelativePath)
	if err != nil || len(components) == 0 {
		return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{},
			errors.Join(errors.New("large opaque recovery root binding is invalid"), err)
	}
	rootCreateResidue, parentPresent, err := largeOpaqueObserveBoundRootCreateResidue(
		ctx, binding, components,
	)
	if err != nil {
		return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{}, err
	}
	authority, present, err := existingPrivateCASRootAuthority(binding)
	if err != nil {
		return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{}, err
	}
	if present && !parentPresent {
		return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{},
			errors.New("large opaque recovery root appeared below an absent bound parent")
	}
	if present && rootCreateResidue.present {
		return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{},
			errors.New("large opaque final root coexists with its create residue")
	}
	if !present {
		if rootCreateResidue.present && limits.MaxCreateResidues < 1 {
			return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{},
				errors.New("large opaque recovery create-residue bound exceeded")
		}
		return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{
			rootCreateResidue: rootCreateResidue,
			createResidueCount: func() uint32 {
				if rootCreateResidue.present {
					return 1
				}
				return 0
			}(),
		}, nil
	}
	observation, err := largeOpaqueObservePresentRecoveryPlatform(
		ctx, authority, maxBytes, limits,
	)
	return authority, true, observation, err
}

func largeOpaqueObserveBoundRootCreateResidue(
	ctx context.Context,
	binding privatecasport.RootBinding,
	components []string,
) (_ largeOpaqueRecoveryCreateResidueObservation, _ bool, resultErr error) {
	parent, err := privateCASUnixOpenFrozenBindingRoot(binding)
	if err != nil {
		return largeOpaqueRecoveryCreateResidueObservation{}, false, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(parent)) }()
	for _, component := range components[:len(components)-1] {
		if err := contextErr(ctx); err != nil {
			return largeOpaqueRecoveryCreateResidueObservation{}, false, err
		}
		next, present, err := privateCASUnixOpenExactBoundDirectory(
			parent, component, binding.RootIdentity.Device,
		)
		if err != nil {
			return largeOpaqueRecoveryCreateResidueObservation{}, false, err
		}
		if !present {
			return largeOpaqueRecoveryCreateResidueObservation{}, false, nil
		}
		if _, err := largeOpaqueUnixDirectoryIdentityOf(next, binding.RootIdentity.Device); err != nil {
			return largeOpaqueRecoveryCreateResidueObservation{}, false, errors.Join(err, unix.Close(next))
		}
		closeErr := unix.Close(parent)
		parent = next
		if closeErr != nil {
			return largeOpaqueRecoveryCreateResidueObservation{}, false, closeErr
		}
	}
	residue, err := largeOpaqueObserveRootCreateResidue(
		ctx, parent, components[len(components)-1], binding.RootIdentity.Device,
	)
	return residue, true, err
}

func largeOpaqueObservePresentRecoveryPlatform(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes uint64,
	limits LargeOpaqueRecoveryLimitsV1,
) (_ largeOpaqueRecoveryObservation, resultErr error) {
	root, rootIdentity, err := largeOpaqueUnixOpenRoot(authority)
	if err != nil {
		return largeOpaqueRecoveryObservation{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(root)) }()
	observation := largeOpaqueRecoveryObservation{root: rootIdentity}
	err = privateCASUnixWalkDir(root, func(entry os.DirEntry) error {
		if err := contextErr(ctx); err != nil {
			return err
		}
		name := entry.Name()
		if !entry.IsDir() {
			return errors.New("large opaque recovery root contains a non-directory entry")
		}
		if !validPrivateShard(name) {
			index, ok := largeOpaqueRecoveryShardResidueIndex(name)
			if !ok || observation.shardCreateResidues[index].present {
				return errors.New("large opaque recovery root contains an unknown or aliased entry")
			}
			residue, err := largeOpaqueObserveCreateResidueDirectory(
				root, name, rootIdentity.device,
			)
			if err != nil {
				return err
			}
			if observation.createResidueCount >= limits.MaxCreateResidues {
				return errors.New("large opaque recovery create-residue bound exceeded")
			}
			observation.shardCreateResidues[index] = residue
			observation.createResidueCount++
			return nil
		}
		index, err := strconv.ParseUint(name, 16, 8)
		if err != nil || observation.shards[index].present {
			return errors.New("large opaque recovery root contains an aliased shard")
		}
		shard, identity, err := largeOpaqueUnixOpenShard(root, name)
		if err != nil {
			return err
		}
		committed, temporary, committedAccumulator, temporaryAccumulator, scanErr :=
			largeOpaqueObserveRecoveryShard(ctx, shard, name, rootIdentity.device, maxBytes, limits)
		closeErr := unix.Close(shard)
		if scanErr != nil || closeErr != nil {
			return errors.Join(scanErr, closeErr)
		}
		if committed > limits.MaxCommittedObjects-observation.objectCount {
			return errors.New("large opaque recovery committed-object bound exceeded")
		}
		if temporary > limits.MaxTemporaryObjects-observation.tempCount {
			return errors.New("large opaque recovery temporary-object bound exceeded")
		}
		observation.shards[index] = largeOpaqueRecoveryShardObservation{present: true, identity: identity}
		observation.shardCount++
		observation.objectCount += committed
		observation.tempCount += temporary
		observation.committedObjects.merge(committedAccumulator)
		observation.temporaryObjects.merge(temporaryAccumulator)
		return nil
	})
	if err != nil {
		return largeOpaqueRecoveryObservation{}, err
	}
	for index := range observation.shards {
		if observation.shards[index].present && observation.shardCreateResidues[index].present {
			return largeOpaqueRecoveryObservation{}, errors.New("large opaque final shard coexists with its create residue")
		}
	}
	return observation, nil
}

func largeOpaqueObserveRootCreateResidue(
	ctx context.Context,
	parent int,
	component string,
	device uint64,
) (largeOpaqueRecoveryCreateResidueObservation, error) {
	target := privateCASCreateDirectoryResidueName(component)
	var observed largeOpaqueRecoveryCreateResidueObservation
	count := 0
	err := privateCASUnixWalkDir(parent, func(entry os.DirEntry) error {
		if err := contextErr(ctx); err != nil {
			return err
		}
		count++
		if count > maxPrivateCASCreateRecoveryDirectoryEntries {
			return errors.New("large opaque recovery parent entry bound exceeded")
		}
		name := entry.Name()
		if !domainprivatecas.LooksLikeCreateDirectoryResidueNameV1(name) {
			return nil
		}
		if name != target || observed.present || !entry.IsDir() {
			return errors.New("large opaque recovery parent contains an unknown, aliased, or unsafe create residue")
		}
		residue, err := largeOpaqueObserveCreateResidueDirectory(parent, name, device)
		if err != nil {
			return err
		}
		observed = residue
		return nil
	})
	return observed, err
}

func largeOpaqueObserveCreateResidueDirectory(
	parent int,
	name string,
	device uint64,
) (_ largeOpaqueRecoveryCreateResidueObservation, resultErr error) {
	residue, present, err := privateCASUnixOpenExactBoundDirectory(parent, name, device)
	if err != nil || !present {
		if err == nil {
			err = os.ErrNotExist
		}
		return largeOpaqueRecoveryCreateResidueObservation{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(residue)) }()
	identity, err := privateCASUnixCreateRecoveryExactIdentity(residue, device)
	if err != nil {
		return largeOpaqueRecoveryCreateResidueObservation{}, err
	}
	var stat unix.Stat_t
	empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(residue)
	if statErr := unix.Fstat(residue, &stat); statErr != nil || emptyErr != nil || !empty ||
		!largeOpaquePlatformObjectSafe(residue, stat) {
		return largeOpaqueRecoveryCreateResidueObservation{}, errors.Join(
			errors.New("large opaque create residue is non-empty or unsafe"), statErr, emptyErr,
		)
	}
	return largeOpaqueRecoveryCreateResidueObservation{present: true, identity: identity}, nil
}

func largeOpaqueRecoveryShardResidueIndex(name string) (uint8, bool) {
	if !domainprivatecas.LooksLikeCreateDirectoryResidueNameV1(name) {
		return 0, false
	}
	for index := 0; index < 256; index++ {
		shard := strings.ToLower(strconv.FormatUint(uint64(index), 16))
		if len(shard) == 1 {
			shard = "0" + shard
		}
		if name == privateCASCreateDirectoryResidueName(shard) {
			return uint8(index), true
		}
	}
	return 0, false
}

func largeOpaqueApplyRecoveryPlatform(
	ctx context.Context,
	binding privatecasport.RootBinding,
	authority privateCASRootAuthority,
	present bool,
	expected largeOpaqueRecoveryObservation,
	maxBytes uint64,
	limits LargeOpaqueRecoveryLimitsV1,
) (LargeOpaqueRecoveryReportV1, largeOpaqueRecoveryObservation, error) {
	currentAuthority, currentPresent, current, err := largeOpaqueObserveRecoveryPlatform(
		ctx, binding, maxBytes, limits,
	)
	if err != nil || current != expected || currentPresent != present || currentAuthority != authority {
		return LargeOpaqueRecoveryReportV1{}, largeOpaqueRecoveryObservation{}, errors.Join(
			errors.New("large opaque recovery changed before clean-state issuance"),
			err,
		)
	}
	// POSIX unlinkat removes the directory entry currently bound to a name; it
	// does not bind deletion to the file descriptor whose identity was checked.
	// A same-UID process can therefore replace a prepared residue between the
	// last identity check and unlink, causing recovery to delete an unobserved
	// object. Until each supported platform supplies an identity-bound deletion
	// transaction, recovery is observation-only and residues fail closed.
	if expected.createResidueCount != 0 || expected.tempCount != 0 {
		return LargeOpaqueRecoveryReportV1{}, largeOpaqueRecoveryObservation{},
			errors.New("large opaque recovery residues require identity-bound operator remediation")
	}
	report := LargeOpaqueRecoveryReportV1{
		Present: present, ShardCount: expected.shardCount, ObjectCount: expected.objectCount,
	}
	if !present {
		return report, current, nil
	}
	return report, current, nil
}

func largeOpaqueObserveRecoveryShard(
	ctx context.Context,
	shard int,
	shardName string,
	expectedDevice uint64,
	maxBytes uint64,
	limits LargeOpaqueRecoveryLimitsV1,
) (_ uint64, _ uint64, _ largeOpaqueRecoveryAccumulator, _ largeOpaqueRecoveryAccumulator, resultErr error) {
	var committed uint64
	var temporary uint64
	var committedAccumulator largeOpaqueRecoveryAccumulator
	var temporaryAccumulator largeOpaqueRecoveryAccumulator
	err := privateCASUnixWalkDir(shard, func(entry os.DirEntry) error {
		if err := contextErr(ctx); err != nil {
			return err
		}
		kind, address, ok := largeOpaqueRecoveryClassifyName(entry.Name(), shardName)
		if !ok || entry.IsDir() {
			return errors.New("large opaque shard contains an unknown or non-file entry")
		}
		identity, err := largeOpaqueRecoveryOpenFile(
			shard, entry.Name(), kind, expectedDevice, maxBytes,
		)
		if err != nil {
			return err
		}
		switch kind {
		case largeOpaqueRecoveryEntryCommitted:
			if committed >= limits.MaxCommittedObjects {
				return errors.New("large opaque recovery committed object bound exceeded")
			}
			committed++
			committedAccumulator.add(kind, address, entry.Name(), identity)
		case largeOpaqueRecoveryEntryTemporary:
			if temporary >= limits.MaxTemporaryObjects {
				return errors.New("large opaque recovery temp object bound exceeded")
			}
			temporary++
			temporaryAccumulator.add(kind, address, entry.Name(), identity)
		default:
			return errors.New("large opaque recovery entry kind is invalid")
		}
		return nil
	})
	return committed, temporary, committedAccumulator, temporaryAccumulator, err
}

func largeOpaqueRecoveryOpenFile(
	shard int,
	name string,
	kind largeOpaqueRecoveryEntryKind,
	expectedDevice uint64,
	maxBytes uint64,
) (_ largeOpaqueUnixFileIdentity, resultErr error) {
	fd, err := unix.Openat(
		shard,
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return largeOpaqueUnixFileIdentity{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(fd)) }()
	var stat unix.Stat_t
	mode := uint32(0)
	allowEmpty := false
	switch kind {
	case largeOpaqueRecoveryEntryCommitted:
		mode = 0o400
	case largeOpaqueRecoveryEntryTemporary:
		allowEmpty = true
	default:
		return largeOpaqueUnixFileIdentity{}, errors.New("large opaque recovery file kind is invalid")
	}
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || stat.Size < 0 ||
		!allowEmpty && stat.Size == 0 || uint64(stat.Size) > maxBytes || uint64(stat.Dev) != expectedDevice ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) || !largeOpaquePlatformObjectSafe(fd, stat) {
		return largeOpaqueUnixFileIdentity{}, errors.New("large opaque recovery file is unsafe")
	}
	actualMode := uint32(stat.Mode) & 0o7777
	if kind == largeOpaqueRecoveryEntryCommitted && actualMode != mode ||
		kind == largeOpaqueRecoveryEntryTemporary && actualMode != 0o600 && actualMode != 0o400 {
		return largeOpaqueUnixFileIdentity{}, errors.New("large opaque recovery file mode is unsafe")
	}
	return largeOpaqueUnixFileIdentity{
		device: uint64(stat.Dev), inode: stat.Ino, mode: uint32(stat.Mode), links: uint64(stat.Nlink),
		uid: stat.Uid, gid: stat.Gid, size: stat.Size, times: privateCASUnixStatTimes(stat),
	}, nil
}

func largeOpaqueRecoveryClassifyName(
	name string,
	shard string,
) (largeOpaqueRecoveryEntryKind, string, bool) {
	if len(name) == 64+len(".blob") && strings.HasSuffix(name, ".blob") {
		address := strings.TrimSuffix(name, ".blob")
		if validPrivateDigest(address) && strings.HasPrefix(address, shard) {
			return largeOpaqueRecoveryEntryCommitted, address, true
		}
		return 0, "", false
	}
	const temporaryLength = 1 + 64 + 1 + 24 + 4
	if len(name) != temporaryLength || name[0] != '.' || name[65] != '-' || name[90:] != ".tmp" {
		return 0, "", false
	}
	address := name[1:65]
	if !validPrivateDigest(address) || !strings.HasPrefix(address, shard) || !largeOpaqueLowerHex(name[66:90]) {
		return 0, "", false
	}
	return largeOpaqueRecoveryEntryTemporary, address, true
}

func largeOpaqueLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range []byte(value) {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

func largeOpaqueRecoveryCommittedEqual(
	left largeOpaqueRecoveryObservation,
	right largeOpaqueRecoveryObservation,
) bool {
	return left.root == right.root && left.shards == right.shards &&
		left.shardCount == right.shardCount && left.objectCount == right.objectCount &&
		left.committedObjects == right.committedObjects
}

func (accumulator *largeOpaqueRecoveryAccumulator) add(
	kind largeOpaqueRecoveryEntryKind,
	address string,
	name string,
	identity largeOpaqueUnixFileIdentity,
) {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte{byte(kind)})
	largeOpaqueRecoveryWriteField(hasher, []byte(address))
	largeOpaqueRecoveryWriteField(hasher, []byte(name))
	var number [8]byte
	for _, value := range []uint64{
		identity.device, identity.inode, uint64(identity.mode), identity.links,
		uint64(identity.uid), uint64(identity.gid), uint64(identity.size),
	} {
		binary.BigEndian.PutUint64(number[:], value)
		_, _ = hasher.Write(number[:])
	}
	_, _ = hasher.Write(identity.times[:])
	var digest [32]byte
	copy(digest[:], hasher.Sum(nil))
	accumulator.addDigest(digest)
}

func (accumulator *largeOpaqueRecoveryAccumulator) merge(other largeOpaqueRecoveryAccumulator) {
	accumulator.addToSum(other.sum)
	for index := range accumulator.xor {
		accumulator.xor[index] ^= other.xor[index]
	}
}

func (accumulator *largeOpaqueRecoveryAccumulator) addDigest(digest [32]byte) {
	for index := range accumulator.xor {
		accumulator.xor[index] ^= digest[index]
	}
	accumulator.addToSum(digest)
}

func (accumulator *largeOpaqueRecoveryAccumulator) addToSum(digest [32]byte) {
	carry := uint16(0)
	for index := len(accumulator.sum) - 1; index >= 0; index-- {
		value := uint16(accumulator.sum[index]) + uint16(digest[index]) + carry
		accumulator.sum[index] = byte(value)
		carry = value >> 8
	}
}

func largeOpaqueRecoveryWriteField(writer io.Writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(value)
}
