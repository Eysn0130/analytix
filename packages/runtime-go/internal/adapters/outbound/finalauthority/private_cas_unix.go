//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

type privateCASShardIdentity struct {
	dev uint64
	ino uint64
}

type privateCASRootAuthority struct {
	binding privatecasport.RootBinding
	dev     uint64
	ino     uint64
}

type privateCASRootAnchor struct {
	fd       int
	identity privateCASShardIdentity
	valid    bool
}

type privateCASShardAnchor struct {
	fd       int
	identity privateCASShardIdentity
	valid    bool
}

func privateCASOpenRootAnchor(authority privateCASRootAuthority) (privateCASRootAnchor, error) {
	fd, err := authority.open()
	if err != nil {
		return privateCASRootAnchor{}, err
	}
	identity := privateCASShardIdentity{dev: authority.dev, ino: authority.ino}
	return privateCASRootAnchor{fd: fd, identity: identity, valid: true}, nil
}

func privateCASValidateRootAnchor(anchor privateCASRootAnchor, authority privateCASRootAuthority) error {
	if !anchor.valid || anchor.fd < 0 || anchor.identity != (privateCASShardIdentity{dev: authority.dev, ino: authority.ino}) {
		return errors.New("private CAS Unix root anchor is invalid")
	}
	var anchored unix.Stat_t
	if err := unix.Fstat(anchor.fd, &anchored); err != nil ||
		!existingPrivateAuthorityRootSafe(anchored, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(anchor.fd) ||
		(privateCASShardIdentity{dev: uint64(anchored.Dev), ino: anchored.Ino}) != anchor.identity {
		return errors.Join(errors.New("private CAS Unix root anchor changed"), err)
	}
	current, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(current)
	var currentStat unix.Stat_t
	if err := unix.Fstat(current, &currentStat); err != nil ||
		(privateCASShardIdentity{dev: uint64(currentStat.Dev), ino: currentStat.Ino}) != anchor.identity {
		return errors.Join(errors.New("private CAS Unix root path no longer names its anchor"), err)
	}
	return nil
}

func privateCASCloseRootAnchor(anchor privateCASRootAnchor) error {
	if !anchor.valid || anchor.fd < 0 {
		return nil
	}
	return unix.Close(anchor.fd)
}

func privateCASOpenShardAnchor(
	authority privateCASRootAuthority,
	name string,
	identity privateCASShardIdentity,
) (privateCASShardAnchor, error) {
	root, err := authority.open()
	if err != nil {
		return privateCASShardAnchor{}, err
	}
	defer unix.Close(root)
	shard, err := securePrivateOpenShard(root, name, false)
	if err != nil {
		return privateCASShardAnchor{}, err
	}
	current, err := privateCASUnixIdentity(shard, authority.dev)
	if err != nil || current != identity {
		_ = unix.Close(shard)
		return privateCASShardAnchor{}, errors.Join(errors.New("private CAS Unix shard anchor identity changed"), err)
	}
	return privateCASShardAnchor{fd: shard, identity: identity, valid: true}, nil
}

func privateCASValidateShardAnchor(
	authority privateCASRootAuthority,
	name string,
	identity privateCASShardIdentity,
	anchor privateCASShardAnchor,
) error {
	if !anchor.valid || anchor.fd < 0 || anchor.identity != identity {
		return errors.New("private CAS Unix shard anchor is invalid")
	}
	anchored, err := privateCASUnixIdentity(anchor.fd, authority.dev)
	if err != nil || anchored != identity {
		return errors.Join(errors.New("private CAS Unix shard anchor changed"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	current, err := securePrivateOpenShard(root, name, false)
	if err != nil {
		return err
	}
	currentIdentity, identityErr := privateCASUnixIdentity(current, authority.dev)
	closeErr := unix.Close(current)
	if identityErr != nil || closeErr != nil || currentIdentity != identity {
		return errors.Join(errors.New("private CAS Unix shard path no longer names its anchor"), identityErr, closeErr)
	}
	return nil
}

func privateCASCloseShardAnchor(anchor privateCASShardAnchor) error {
	if !anchor.valid || anchor.fd < 0 {
		return nil
	}
	return unix.Close(anchor.fd)
}

type privateCASPreparedRecoveryPlan struct {
	shards                 []privateCASPreparedRecoveryShard
	originalCreateResidues []privateCASCreateResidueUnixCandidate
}

type privateCASPreparedRecoveryShard struct {
	name           string
	identity       privateCASShardIdentity
	stat           unix.Stat_t
	entryNames     []string
	committedNames []string
	committed      []privateCASPreparedRecoveryRecord
	temps          []privateCASPreparedRecoveryTemp
}

type privateCASPreparedRecoveryRecord struct {
	name       string
	digest     string
	identity   unix.Stat_t
	bodySHA256 [32]byte
	byteLength uint64
}

type privateCASPreparedRecoveryTemp struct {
	name          string
	originalName  string
	transactionID string
	phase         privateCASRecoveryQuarantinePhase
	identity      unix.Stat_t
	bodySHA256    [32]byte
	linked        bool
}

func newPrivateCASRootAuthority(binding privatecasport.RootBinding) (privateCASRootAuthority, error) {
	root, err := privateCASUnixOpenBoundRoot(binding, true)
	if err != nil {
		return privateCASRootAuthority{}, err
	}
	defer unix.Close(root)
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(root) {
		return privateCASRootAuthority{}, errors.New("private CAS root permissions are unsafe")
	}
	if err := unix.Fsync(root); err != nil {
		return privateCASRootAuthority{}, err
	}
	return privateCASRootAuthority{binding: binding, dev: uint64(stat.Dev), ino: stat.Ino}, nil
}

func existingPrivateCASRootAuthority(binding privatecasport.RootBinding) (privateCASRootAuthority, bool, error) {
	root, err := privateCASUnixOpenBoundRoot(binding, false)
	if errors.Is(err, unix.ENOENT) {
		return privateCASRootAuthority{}, false, nil
	}
	if err != nil {
		return privateCASRootAuthority{}, false, err
	}
	defer unix.Close(root)
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(root) {
		return privateCASRootAuthority{}, false, errors.New("private CAS existing root permissions are unsafe")
	}
	return privateCASRootAuthority{binding: binding, dev: uint64(stat.Dev), ino: stat.Ino}, true, nil
}

func (authority privateCASRootAuthority) open() (int, error) {
	root, err := privateCASUnixOpenBoundRoot(authority.binding, false)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(root) ||
		uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino {
		_ = unix.Close(root)
		return -1, errors.New("private CAS root identity or permissions changed")
	}
	return root, nil
}

func privateCASUnixOpenBoundRoot(binding privatecasport.RootBinding, create bool, initials ...privateCASOriginalLeafInitializationV1) (int, error) {
	if len(initials) > 1 || len(initials) == 1 && !create {
		return -1, errors.New("original CAS Unix initialization binding is invalid")
	}
	components, err := privateCASUnixRelativeComponents(binding.RelativePath)
	if err != nil {
		return -1, err
	}
	current, err := privateCASUnixOpenFrozenBindingRoot(binding)
	if err != nil {
		return -1, err
	}
	currentPath := binding.RootPath
	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)
		next, present, handled, openErr := platformPrivateCASUnixOpenExactBoundDirectory(
			current, component, binding.RootIdentity.Device,
		)
		if !handled {
			next, present, openErr = privateCASUnixOpenExactBoundDirectory(
				current,
				component,
				binding.RootIdentity.Device,
			)
		}
		if openErr == nil && !present && create {
			if len(initials) == 1 {
				openErr = initials[0].validateCreatePathV1(currentPath)
				if openErr == nil {
					next, openErr = privateCASUnixCreateIndependentOriginalDirectoryV1(current, component, binding.RootIdentity.Device)
					if openErr == nil {
						openErr = initials[0].createdV1(currentPath)
						if openErr != nil {
							_ = unix.Close(next)
						}
					}
				}
			} else {
				next, openErr = privateCASUnixCreateBoundDirectory(current, component, binding.RootIdentity.Device)
			}
			present = openErr == nil
		}
		if openErr == nil && !present {
			openErr = unix.ENOENT
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, openErr
		}
		var stat unix.Stat_t
		if err := unix.Fstat(next, &stat); err != nil ||
			!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
			!existingPrivateAuthorityExtendedSecuritySafe(next) || uint64(stat.Dev) != binding.RootIdentity.Device {
			_ = unix.Close(next)
			_ = unix.Close(current)
			return -1, errors.New("private CAS Unix descendant is unsafe")
		}
		if len(initials) == 1 {
			named, present, nameErr := privateCASUnixOpenExactBoundDirectory(current, component, binding.RootIdentity.Device)
			if nameErr == nil && present {
				var namedStat unix.Stat_t
				nameErr = errors.Join(unix.Fstat(named, &namedStat), unix.Close(named))
				if nameErr == nil && !privateCASUnixExactObject(stat, namedStat) {
					nameErr = errors.New("original CAS initialized directory identity changed")
				}
			}
			if nameErr != nil || !present {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, errors.Join(errors.New("original CAS initialized directory name changed"), nameErr)
			}
		}
		_ = unix.Close(current)
		current = next
	}
	return current, nil
}

func privateCASUnixOpenFrozenBindingRoot(binding privatecasport.RootBinding) (int, error) {
	if binding.RootIdentity.Kind != privatecasport.DirectoryIdentityUnix || binding.RootIdentity.Device == 0 ||
		binding.RootIdentity.Inode == 0 || binding.RootIdentity.VolumeSerial != 0 ||
		binding.RootIdentity.FileID != [16]byte{} || !filepath.IsAbs(binding.RootPath) {
		return -1, errors.New("private CAS Unix root binding is invalid")
	}
	current, err := securePrivateOpenAbsoluteDirectory(binding.RootPath, false)
	if err != nil {
		return -1, err
	}
	var rootStat unix.Stat_t
	if err := unix.Fstat(current, &rootStat); err != nil || rootStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(current)
		return -1, errors.New("private CAS frozen Unix root is unavailable")
	}
	if !privateCASUnixFrozenRootSafe(rootStat, uint32(os.Geteuid())) {
		_ = unix.Close(current)
		return -1, errors.New("private CAS frozen Unix root permissions are unsafe")
	}
	if !existingPrivateAuthorityExtendedSecuritySafe(current) {
		_ = unix.Close(current)
		return -1, errors.New("private CAS frozen Unix root extended ACL is unsafe")
	}
	if uint64(rootStat.Dev) != binding.RootIdentity.Device || rootStat.Ino != binding.RootIdentity.Inode {
		_ = unix.Close(current)
		return -1, errors.New("private CAS frozen Unix root identity changed")
	}
	return current, nil
}

func privateCASUnixFrozenRootSafe(stat unix.Stat_t, hostUID uint32) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Uid == hostUID && stat.Mode&0o7022 == 0
}

func privateCASUnixCreateBoundDirectory(parent int, component string, device uint64) (int, error) {
	temporary := privateCASCreateDirectoryResidueName(component)
	residue, present, err := privateCASUnixOpenExactBoundDirectory(parent, temporary, device)
	if err == nil && present {
		_ = unix.Close(residue)
		return -1, errors.New("private CAS Unix pre-existing create residue requires global recovery")
	}
	if err != nil {
		return -1, err
	}
	if err := unix.Mkdirat(parent, temporary, 0o700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return -1, errors.New("private CAS Unix create residue raced")
		}
		return -1, err
	}
	tempExists := true
	var staged unix.Stat_t
	stagedBound := false
	defer func() {
		if tempExists && stagedBound {
			_ = privateCASUnixRemoveOwnedCreateResidue(parent, temporary, device, staged)
		}
	}()
	temporaryFD, err := unix.Openat(parent, temporary, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	defer unix.Close(temporaryFD)
	if err := unix.Fstat(temporaryFD, &staged); err != nil ||
		!existingPrivateAuthorityRootSafe(staged, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(temporaryFD) || uint64(staged.Dev) != device {
		return -1, errors.New("private CAS Unix staged directory is unsafe")
	}
	stagedBound = true
	if err := unix.Fsync(temporaryFD); err != nil {
		return -1, err
	}
	if err := securePrivateCommitNoReplace(parent, temporary, component); err != nil {
		return -1, err
	}
	tempExists = false
	if err := unix.Fsync(parent); err != nil {
		return -1, err
	}
	installed, present, err := privateCASUnixOpenExactBoundDirectory(parent, component, device)
	if err != nil || !present {
		if err == nil {
			err = os.ErrNotExist
		}
		return -1, err
	}
	var current unix.Stat_t
	if err := unix.Fstat(installed, &current); err != nil || current.Dev != staged.Dev || current.Ino != staged.Ino {
		_ = unix.Close(installed)
		return -1, errors.New("private CAS Unix installed directory identity changed")
	}
	return installed, nil
}

func privateCASUnixRemoveOwnedCreateResidue(
	parent int,
	name string,
	device uint64,
	expected unix.Stat_t,
) error {
	current, present, err := privateCASUnixOpenExactBoundDirectory(parent, name, device)
	if err == nil && !present {
		return nil
	}
	if err != nil {
		return err
	}
	var stat unix.Stat_t
	statErr := unix.Fstat(current, &stat)
	extendedSecuritySafe := existingPrivateAuthorityExtendedSecuritySafe(current)
	entries, emptyErr := privateCASUnixReadDirBounded(current, 0)
	if statErr != nil {
		return errors.Join(statErr, emptyErr, unix.Close(current))
	}
	if emptyErr != nil || len(entries) != 0 ||
		!privateCASUnixExactObject(stat, expected) ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!extendedSecuritySafe || uint64(stat.Dev) != device {
		return errors.Join(
			errors.New("private CAS Unix owned create residue changed before cleanup"),
			emptyErr,
			unix.Close(current),
		)
	}
	if err := unix.Unlinkat(parent, name, unix.AT_REMOVEDIR); err != nil {
		_ = unix.Close(current)
		return err
	}
	var held unix.Stat_t
	heldErr := unix.Fstat(current, &held)
	var heldIdentityErr error
	if heldErr == nil && (held.Dev != expected.Dev || held.Ino != expected.Ino) {
		heldIdentityErr = errors.New("private CAS Unix owned create residue identity changed during cleanup")
	}
	closeErr := unix.Close(current)
	stillNamed, stillPresent, absenceErr := privateCASUnixOpenExactBoundDirectory(parent, name, device)
	if stillPresent {
		_ = unix.Close(stillNamed)
		absenceErr = errors.New("private CAS Unix owned create-residue name survived cleanup")
	}
	return errors.Join(
		heldErr,
		heldIdentityErr,
		closeErr,
		absenceErr,
		unix.Fsync(parent),
	)
}

func privateCASUnixRelativeComponents(relative string) ([]string, error) {
	if relative == "" || relative == "." || filepath.IsAbs(relative) {
		return nil, errors.New("private CAS Unix relative binding is invalid")
	}
	cleaned := filepath.Clean(relative)
	if cleaned != relative || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return nil, errors.New("private CAS Unix relative binding is not canonical")
	}
	components := strings.Split(cleaned, string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." || component == ".." || strings.IndexByte(component, 0) >= 0 {
			return nil, errors.New("private CAS Unix relative component is invalid")
		}
	}
	return components, nil
}

func capturePrivateCASShardIdentities(authority privateCASRootAuthority) (map[string]privateCASShardIdentity, error) {
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	entries, err := privateCASUnixReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return nil, err
	}
	pins := make(map[string]privateCASShardIdentity, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return nil, errors.New("private CAS inventory contains a non-canonical shard")
		}
		shard, err := securePrivateOpenShard(root, entry.Name(), false)
		if err != nil {
			return nil, err
		}
		identity, identityErr := privateCASUnixIdentity(shard, authority.dev)
		closeErr := unix.Close(shard)
		if identityErr != nil || closeErr != nil {
			return nil, errors.Join(identityErr, closeErr)
		}
		pins[entry.Name()] = identity
	}
	return pins, nil
}

func securePrivateCASWrite(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, digest string, body []byte, maxBytes int) error {
	return securePrivateCASWriteWithHooks(authority, pins, digest, body, maxBytes, nil, nil, nil, nil)
}

func securePrivateCASWriteWithStageHook(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, digest string, body []byte, maxBytes int, beforeCommit func()) error {
	return securePrivateCASWriteWithHooks(authority, pins, digest, body, maxBytes, beforeCommit, nil, nil, nil)
}

func securePrivateCASWriteWithHook(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, digest string, body []byte, maxBytes int, beforeReadback func()) error {
	return securePrivateCASWriteWithHooks(authority, pins, digest, body, maxBytes, nil, beforeReadback, nil, nil)
}

func securePrivateCASWriteWithAdditionReceipt(
	authority privateCASRootAuthority,
	pins map[string]privateCASShardIdentity,
	digest string,
	body []byte,
	maxBytes int,
	beforeCommit func(),
	validations ...privateCASOriginalWriteValidationV1,
) (privateCASAdditionObservation, bool, error) {
	shardCreated := false
	var observation privateCASAdditionObservation
	if err := securePrivateCASWriteWithHooks(
		authority, pins, digest, body, maxBytes, beforeCommit, nil, &shardCreated, &observation, validations...,
	); err != nil {
		return privateCASAdditionObservation{}, false, err
	}
	return observation, shardCreated, nil
}

func securePrivateCASWriteWithHooks(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, digest string, body []byte, maxBytes int, beforeCommit, beforeReadback func(), shardCreatedOutput *bool, additionOutput *privateCASAdditionObservation, validations ...privateCASOriginalWriteValidationV1) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	validateRoot := func() error {
		if handled, err := validateOriginalPrivateCASWriteRootV1(authority, pins, maxBytes, validations); handled {
			return err
		}
		return privateCASUnixValidatePinnedRootStrictAt(root, pins)
	}
	if err := validateRoot(); err != nil {
		return err
	}
	shardName := digest[:2]
	shard, identity, shardCreated, err := privateCASUnixOpenPinnedShardForCommit(root, pins, shardName, privateCASOriginalWriteHasTargetShardResidueV1(shardName, validations))
	if err != nil {
		return err
	}
	if shardCreatedOutput != nil {
		*shardCreatedOutput = shardCreated
	}
	defer unix.Close(shard)
	if err := privateCASOriginalBeforeRecordStageV1(validations); err != nil {
		return err
	}
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
		return errors.New("private CAS temp handle is invalid")
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
			return errors.New("private CAS write was incomplete")
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
		return errors.New("private CAS staged write verification failed")
	}
	stagedStat, err := privateCASUnixRecordStat(fd, maxBytes, identity.dev)
	if err != nil {
		return err
	}
	if beforeCommit != nil {
		beforeCommit()
	}
	if err := securePrivateCommitNoReplace(shard, temporary, name); err != nil {
		return err
	}
	tempExists = false
	if err := unix.Fsync(shard); err != nil {
		return err
	}
	if beforeReadback != nil {
		beforeReadback()
	}
	written, committedStat, err := privateCASUnixReadStableAt(shard, name, maxBytes)
	if err != nil || !bytes.Equal(written, body) || !privateCASUnixSameObject(stagedStat, committedStat) {
		return errors.New("private CAS committed write verification failed")
	}
	if err := privateCASUnixVerifyCurrentShard(authority, shardName, identity); err != nil {
		return err
	}
	if err := validateRoot(); err != nil {
		return err
	}
	if additionOutput != nil {
		rootMetadata, err := privateCASUnixSnapshotMetadata(root)
		if err != nil {
			return err
		}
		shardMetadata, err := privateCASUnixSnapshotMetadata(shard)
		if err != nil {
			return err
		}
		recordMetadata, err := privateCASUnixSnapshotMetadata(fd)
		if err != nil {
			return err
		}
		current, err := privateCASUnixRecordIdentityAt(shard, name, maxBytes, uint64(stagedStat.Dev))
		if err != nil || !privateCASUnixSameObject(stagedStat, current) {
			return errors.Join(errors.New("private CAS committed addition path no longer names the staged object"), err)
		}
		*additionOutput = privateCASAdditionObservation{
			body:                 written,
			rootIdentityDigest:   privateCASUnixObjectIdentityDigest("root", authority.dev, authority.ino),
			shardIdentityDigest:  privateCASUnixObjectIdentityDigest("shard", identity.dev, identity.ino),
			recordIdentityDigest: privateCASUnixObjectIdentityDigest("record", uint64(stagedStat.Dev), stagedStat.Ino),
			rootMetadata:         rootMetadata, shardMetadata: shardMetadata, recordMetadata: recordMetadata,
		}
	}
	return nil
}

func securePrivateCASRead(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, digest string, maxBytes int) ([]byte, error) {
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	if err := privateCASUnixValidatePinnedRootStrictAt(root, pins); err != nil {
		return nil, err
	}
	shardName := digest[:2]
	shard, identity, err := privateCASUnixOpenPinnedShard(root, pins, shardName, false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(shard)
	body, err := privateCASUnixReadAt(shard, digest+".json", maxBytes)
	if err != nil {
		return nil, err
	}
	if err := privateCASUnixVerifyCurrentShard(authority, shardName, identity); err != nil {
		return nil, err
	}
	if err := privateCASUnixValidatePinnedRootStrictAt(root, pins); err != nil {
		return nil, err
	}
	return body, nil
}

func privateCASUnixValidatePinnedRootAt(root int, pins map[string]privateCASShardIdentity) error {
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil || rootStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("private CAS root device is unavailable")
	}
	rootDevice := uint64(rootStat.Dev)
	entries, err := privateCASUnixReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !validPrivateShard(name) {
			return errors.New("private CAS root inventory contains a non-canonical shard")
		}
		shard, err := securePrivateOpenShard(root, name, false)
		if err != nil {
			return err
		}
		identity, identityErr := privateCASUnixIdentity(shard, rootDevice)
		closeErr := unix.Close(shard)
		expected, pinned := pins[name]
		if identityErr != nil || closeErr != nil || pinned && identity != expected {
			return errors.Join(identityErr, closeErr, errors.New("private CAS pinned shard identity changed"))
		}
		if !pinned {
			pins[name] = identity
		}
		seen[name] = struct{}{}
	}
	for name := range pins {
		if _, ok := seen[name]; !ok {
			return errors.New("private CAS pinned shard disappeared")
		}
	}
	return nil
}

func privateCASUnixValidatePinnedRootStrictAt(root int, pins map[string]privateCASShardIdentity) error {
	entries, err := privateCASUnixReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !validPrivateShard(name) {
			return errors.New("private CAS strict root inventory contains a non-canonical shard")
		}
		expected, pinned := pins[name]
		if !pinned {
			return errors.New("private CAS strict root inventory contains an unprepared shard")
		}
		shard, err := securePrivateOpenShard(root, name, false)
		if err != nil {
			return err
		}
		identity, identityErr := privateCASUnixIdentity(shard, expected.dev)
		closeErr := unix.Close(shard)
		if identityErr != nil || closeErr != nil || identity != expected {
			return errors.Join(identityErr, closeErr, errors.New("private CAS strict pinned shard identity changed"))
		}
		seen[name] = struct{}{}
	}
	for name := range pins {
		if _, found := seen[name]; !found {
			return errors.New("private CAS strict pinned shard disappeared")
		}
	}
	return nil
}

func securePrivateCASList(ctx context.Context, authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, maxBytes int) ([]SecurePrivateCASFile, error) {
	return privateCASUnixScanWithBounds(
		ctx, authority, pins, maxBytes, true,
		maxSecurePrivateCASListRecords, maxSecurePrivateCASListAggregateBytes,
	)
}

func securePrivateCASVisit(
	ctx context.Context,
	authority privateCASRootAuthority,
	pins map[string]privateCASShardIdentity,
	maxBytes int,
	visit func(SecurePrivateCASFile) error,
) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	entries, err := privateCASUnixReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return errors.New("private CAS inventory contains a non-canonical shard")
		}
		name := entry.Name()
		names = append(names, name)
		shard, identity, err := privateCASUnixOpenPinnedShard(root, pins, name, false)
		if err != nil {
			return err
		}
		baseline, _, baselineErr := privateCASUnixScanShard(ctx, shard, name, maxBytes, false, 0, 0, false)
		if baselineErr != nil || baseline.records == 0 {
			_ = unix.Close(shard)
			return errors.Join(baselineErr, errors.New("private CAS inventory contains an unreadable or empty shard"))
		}
		visited, visitErr := privateCASUnixVisitShard(ctx, shard, name, maxBytes, visit)
		if visitErr != nil {
			_ = unix.Close(shard)
			return visitErr
		}
		verified, _, verifyErr := privateCASUnixScanShard(ctx, shard, name, maxBytes, false, 0, 0, false)
		if visited != baseline || verifyErr != nil || verified != baseline {
			_ = unix.Close(shard)
			return errors.Join(verifyErr, errors.New("private CAS shard inventory changed during visit"))
		}
		if err := privateCASUnixVerifyCurrentShard(authority, name, identity); err != nil {
			_ = unix.Close(shard)
			return err
		}
		if err := unix.Close(shard); err != nil {
			return err
		}
	}
	sort.Strings(names)
	for name := range pins {
		index := sort.SearchStrings(names, name)
		if index >= len(names) || names[index] != name {
			return errors.New("private CAS pinned shard disappeared")
		}
	}
	current, err := privateCASUnixReadDirBounded(root, len(names))
	if err != nil || !privateCASUnixSameCanonicalNames(names, current) {
		return errors.New("private CAS inventory changed during visit")
	}
	return nil
}

func securePrivateCASValidateInventory(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, maxBytes int) error {
	_, err := privateCASUnixScan(nil, authority, pins, maxBytes, false, 0, 0, false)
	return err
}

func privateCASUnixScanWithBounds(
	ctx context.Context,
	authority privateCASRootAuthority,
	pins map[string]privateCASShardIdentity,
	maxBytes int,
	collectBodies bool,
	maxRecords int,
	maxAggregateBytes int,
) ([]SecurePrivateCASFile, error) {
	if maxRecords <= 0 || maxRecords > maxSecurePrivateCASListRecords ||
		maxAggregateBytes <= 0 || maxAggregateBytes > maxSecurePrivateCASListAggregateBytes {
		return nil, errors.New("private CAS inventory bounds are invalid")
	}
	return privateCASUnixScan(
		ctx, authority, pins, maxBytes, collectBodies,
		uint64(maxRecords), uint64(maxAggregateBytes), true,
	)
}

func privateCASUnixScan(
	ctx context.Context,
	authority privateCASRootAuthority,
	pins map[string]privateCASShardIdentity,
	maxBytes int,
	collectBodies bool,
	maxRecords uint64,
	maxAggregateBytes uint64,
	enforceMaterializationBounds bool,
) ([]SecurePrivateCASFile, error) {
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	entries, err := privateCASUnixReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	files := []SecurePrivateCASFile(nil)
	var totalRecords uint64
	var totalBytes uint64
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return nil, errors.New("private CAS inventory contains a non-canonical shard")
		}
		name := entry.Name()
		names = append(names, name)
		shard, identity, err := privateCASUnixOpenPinnedShard(root, pins, name, false)
		if err != nil {
			return nil, err
		}
		remainingRecords, remainingBytes := uint64(0), uint64(0)
		if enforceMaterializationBounds {
			if totalRecords > maxRecords || totalBytes > maxAggregateBytes {
				_ = unix.Close(shard)
				return nil, errors.New("private CAS materialization bound exceeded")
			}
			remainingRecords = maxRecords - totalRecords
			remainingBytes = maxAggregateBytes - totalBytes
		}
		fingerprint, shardFiles, scanErr := privateCASUnixScanShard(
			ctx, shard, name, maxBytes, collectBodies,
			remainingRecords, remainingBytes, enforceMaterializationBounds,
		)
		if scanErr != nil || fingerprint.records == 0 {
			_ = unix.Close(shard)
			return nil, errors.Join(scanErr, errors.New("private CAS inventory contains an unreadable or empty shard"))
		}
		verified, _, verifyErr := privateCASUnixScanShard(ctx, shard, name, maxBytes, false, 0, 0, false)
		if verifyErr != nil || verified != fingerprint {
			_ = unix.Close(shard)
			return nil, errors.Join(verifyErr, errors.New("private CAS shard inventory changed during enumeration"))
		}
		if ^uint64(0)-totalRecords < fingerprint.records || ^uint64(0)-totalBytes < fingerprint.bytes {
			_ = unix.Close(shard)
			return nil, errors.New("private CAS inventory counters overflowed")
		}
		totalRecords += fingerprint.records
		totalBytes += fingerprint.bytes
		files = append(files, shardFiles...)
		if err := privateCASUnixVerifyCurrentShard(authority, name, identity); err != nil {
			_ = unix.Close(shard)
			return nil, err
		}
		if err := unix.Close(shard); err != nil {
			return nil, err
		}
	}
	sort.Strings(names)
	for name := range pins {
		index := sort.SearchStrings(names, name)
		if index >= len(names) || names[index] != name {
			return nil, errors.New("private CAS pinned shard disappeared")
		}
	}
	current, err := privateCASUnixReadDirBounded(root, len(names))
	if err != nil || !privateCASUnixSameCanonicalNames(names, current) {
		return nil, errors.New("private CAS inventory changed during enumeration")
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Digest < files[right].Digest })
	return files, nil
}

func privateCASUnixScanShard(
	ctx context.Context,
	shard int,
	shardName string,
	maxBytes int,
	collectBodies bool,
	maxRecords uint64,
	maxAggregateBytes uint64,
	enforceMaterializationBounds bool,
) (privateCASInventoryFingerprint, []SecurePrivateCASFile, error) {
	digest := sha256.New()
	privateCASWriteFingerprintField(digest, []byte("analytix-private-cas-inventory-v1"))
	privateCASWriteFingerprintField(digest, []byte(shardName))
	files := []SecurePrivateCASFile(nil)
	var records uint64
	var totalBytes uint64
	err := privateCASUnixWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		recordName := entry.Name()
		recordDigest := strings.TrimSuffix(recordName, ".json")
		if entry.IsDir() || filepath.Ext(recordName) != ".json" || !validPrivateDigest(recordDigest) || recordDigest[:2] != shardName {
			return errors.New("private CAS inventory contains a non-canonical record")
		}
		if records == ^uint64(0) {
			return errors.New("private CAS record counter overflowed")
		}
		if enforceMaterializationBounds && records >= maxRecords {
			return errors.Join(ErrSecurePrivateCASMaterializationLimit, errors.New("private CAS record materialization bound exceeded"))
		}
		body, identity, err := privateCASUnixReadStableAt(shard, recordName, maxBytes)
		if err != nil {
			return err
		}
		bodyBytes := uint64(len(body))
		if ^uint64(0)-totalBytes < bodyBytes {
			return errors.New("private CAS byte counter overflowed")
		}
		if enforceMaterializationBounds && bodyBytes > maxAggregateBytes-totalBytes {
			return errors.Join(ErrSecurePrivateCASMaterializationLimit, errors.New("private CAS aggregate byte materialization bound exceeded"))
		}
		records++
		totalBytes += bodyBytes
		privateCASUnixFingerprintRecord(digest, recordName, body, identity)
		if collectBodies {
			files = append(files, SecurePrivateCASFile{Digest: recordDigest, Body: body})
		}
		return nil
	})
	if err != nil {
		return privateCASInventoryFingerprint{}, nil, err
	}
	var sum [32]byte
	copy(sum[:], digest.Sum(nil))
	return privateCASInventoryFingerprint{digest: sum, records: records, bytes: totalBytes}, files, nil
}

func privateCASUnixVisitShard(
	ctx context.Context,
	shard int,
	shardName string,
	maxBytes int,
	visit func(SecurePrivateCASFile) error,
) (privateCASInventoryFingerprint, error) {
	digest := sha256.New()
	privateCASWriteFingerprintField(digest, []byte("analytix-private-cas-inventory-v1"))
	privateCASWriteFingerprintField(digest, []byte(shardName))
	var records uint64
	var totalBytes uint64
	err := privateCASUnixWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		recordName := entry.Name()
		recordDigest := strings.TrimSuffix(recordName, ".json")
		if entry.IsDir() || filepath.Ext(recordName) != ".json" || !validPrivateDigest(recordDigest) || recordDigest[:2] != shardName {
			return errors.New("private CAS inventory contains a non-canonical record")
		}
		if records == ^uint64(0) {
			return errors.New("private CAS record counter overflowed")
		}
		body, identity, err := privateCASUnixReadStableAt(shard, recordName, maxBytes)
		if err != nil {
			return err
		}
		bodyBytes := uint64(len(body))
		if ^uint64(0)-totalBytes < bodyBytes {
			return errors.New("private CAS byte counter overflowed")
		}
		records++
		totalBytes += bodyBytes
		privateCASUnixFingerprintRecord(digest, recordName, body, identity)
		return visit(SecurePrivateCASFile{Digest: recordDigest, Body: append([]byte(nil), body...)})
	})
	if err != nil {
		return privateCASInventoryFingerprint{}, err
	}
	var sum [32]byte
	copy(sum[:], digest.Sum(nil))
	return privateCASInventoryFingerprint{digest: sum, records: records, bytes: totalBytes}, nil
}

func privateCASUnixFingerprintRecord(writer hash.Hash, name string, body []byte, identity unix.Stat_t) {
	privateCASUnixFingerprintRecordBodySHA256(writer, name, sha256.Sum256(body), identity)
}

func privateCASUnixFingerprintRecordBodySHA256(
	writer hash.Hash,
	name string,
	bodySHA256 [32]byte,
	identity unix.Stat_t,
) {
	privateCASWriteFingerprintField(writer, []byte(name))
	privateCASWriteFingerprintField(writer, bodySHA256[:])
	var metadata [56]byte
	binary.BigEndian.PutUint64(metadata[0:8], uint64(identity.Dev))
	binary.BigEndian.PutUint64(metadata[8:16], identity.Ino)
	binary.BigEndian.PutUint64(metadata[16:24], uint64(identity.Mode))
	binary.BigEndian.PutUint64(metadata[24:32], uint64(identity.Nlink))
	binary.BigEndian.PutUint64(metadata[32:40], uint64(identity.Uid))
	binary.BigEndian.PutUint64(metadata[40:48], uint64(identity.Gid))
	binary.BigEndian.PutUint64(metadata[48:56], uint64(identity.Size))
	privateCASWriteFingerprintField(writer, metadata[:])
	times := privateCASUnixStatTimes(identity)
	privateCASWriteFingerprintField(writer, times[:])
}

func privateCASUnixExactObject(left, right unix.Stat_t) bool {
	return privateCASUnixSameObject(left, right) && privateCASUnixStatTimes(left) == privateCASUnixStatTimes(right)
}

func privateCASUnixWalkDir(parent int, visit func(os.DirEntry) error) error {
	fd, err := unix.Openat(
		parent,
		".",
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(fd), "private-cas-streaming-directory")
	if directory == nil {
		_ = unix.Close(fd)
		return errors.New("private CAS streaming directory handle is invalid")
	}
	var walkErr error
	for {
		entries, readErr := directory.ReadDir(privateCASScanPageEntries)
		for _, entry := range entries {
			if err := visit(entry); err != nil {
				walkErr = err
				break
			}
		}
		if walkErr != nil || errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			walkErr = readErr
			break
		}
		if len(entries) == 0 {
			walkErr = errors.New("private CAS streaming directory enumeration made no progress")
			break
		}
	}
	return errors.Join(walkErr, directory.Close())
}

var errPrivateCASUnixRecoveryBatchFull = errors.New("private CAS recovery batch is full")

func securePreflightPrivateCASRoot(ctx context.Context, authority privateCASRootAuthority, maxBytes int) error {
	return securePreflightPrivateCASRootV4(ctx, authority, maxBytes, false, false)
}

func securePreflightPrivateCASRootV4(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes int,
	allowPreparedCreateResidues bool,
	rejectTransactionPhases bool,
) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	entryBound := maxSecurePrivateCASShards
	if allowPreparedCreateResidues {
		entryBound *= 2
	}
	entries, err := privateCASUnixReadDirBounded(root, entryBound)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		if allowPreparedCreateResidues && entry.IsDir() &&
			domainprivatecas.CreateDirectoryResidueMatchesShardV1(entry.Name()) {
			if err := privateCASUnixValidatePreparedShardCreateResidueV4(root, entry.Name(), authority.dev); err != nil {
				return err
			}
			continue
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return errors.New("private CAS recovery contains a non-canonical shard")
		}
		shard, err := securePrivateOpenShard(root, entry.Name(), false)
		if err != nil {
			return err
		}
		preflightErr := privateCASUnixPreflightRecoveryShard(
			ctx, shard, entry.Name(), maxBytes, rejectTransactionPhases,
		)
		if closeErr := unix.Close(shard); preflightErr != nil || closeErr != nil {
			return errors.Join(preflightErr, closeErr)
		}
	}
	return nil
}

func securePrivateCASValidateNoUnsignedRecoveryPhasesV4(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes int,
	allowPreparedCreateResidues bool,
) error {
	if err := securePreflightPrivateCASRootV4(
		ctx, authority, maxBytes, allowPreparedCreateResidues, true,
	); err != nil {
		return err
	}
	return securePreflightPrivateCASRootV4(
		ctx, authority, maxBytes, allowPreparedCreateResidues, true,
	)
}

func securePrivateCASObserveRecovery(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes int,
) (privateCASRecoveryObservation, error) {
	if err := securePreflightPrivateCASRoot(ctx, authority, maxBytes); err != nil {
		return privateCASRecoveryObservation{}, err
	}
	first, err := privateCASUnixObserveRecoveryOnce(ctx, authority, maxBytes)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	second, err := privateCASUnixObserveRecoveryOnce(ctx, authority, maxBytes)
	if err != nil || first.fingerprint != second.fingerprint {
		return privateCASRecoveryObservation{}, errors.Join(errors.New("private CAS recovery inventory changed during observation"), err)
	}
	return second, nil
}

func securePrivateCASObserveRecoveryIncludingOriginalCreatesV1(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes int,
) (privateCASRecoveryObservation, error) {
	if err := securePreflightPrivateCASRootV4(ctx, authority, maxBytes, true, true); err != nil {
		return privateCASRecoveryObservation{}, err
	}
	first, err := privateCASUnixObserveRecoveryOnce(ctx, authority, maxBytes, true)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	second, err := privateCASUnixObserveRecoveryOnce(ctx, authority, maxBytes, true)
	if err != nil || first.fingerprint != second.fingerprint {
		return privateCASRecoveryObservation{}, errors.Join(errors.New("private CAS recovery inventory changed during observation"), err)
	}
	return second, nil
}

func securePrivateCASReadPreparedCommitted(
	ctx context.Context,
	authority privateCASRootAuthority,
	expected privateCASRecoveryObservation,
	maxBytes int,
	digest string,
) ([]byte, error) {
	if err := privateCASContextError(ctx); err != nil {
		return nil, err
	}
	if !validPrivateDigest(digest) {
		return nil, errors.New("private CAS prepared recovery record digest is invalid")
	}
	shardName := digest[:2]
	shardIndex := sort.Search(len(expected.plan.shards), func(index int) bool {
		return expected.plan.shards[index].name >= shardName
	})
	if shardIndex >= len(expected.plan.shards) || expected.plan.shards[shardIndex].name != shardName {
		return nil, errors.New("private CAS prepared recovery record is outside the frozen inventory")
	}
	preparedShard := expected.plan.shards[shardIndex]
	recordIndex := sort.Search(len(preparedShard.committed), func(index int) bool {
		return preparedShard.committed[index].digest >= digest
	})
	if recordIndex >= len(preparedShard.committed) || preparedShard.committed[recordIndex].digest != digest {
		return nil, errors.New("private CAS prepared recovery record is outside the frozen inventory")
	}
	preparedRecord := preparedShard.committed[recordIndex]
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer unix.Close(root)
	shard, err := securePrivateOpenShard(root, shardName, false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(shard)
	currentShard, err := privateCASUnixIdentity(shard, authority.dev)
	if err != nil || currentShard != preparedShard.identity {
		return nil, errors.Join(errors.New("private CAS prepared recovery shard identity changed"), err)
	}
	var shardStat unix.Stat_t
	if err := unix.Fstat(shard, &shardStat); err != nil ||
		!privateCASUnixExactObject(shardStat, preparedShard.stat) {
		return nil, errors.Join(errors.New("private CAS prepared recovery shard metadata changed"), err)
	}
	body, identity, bodySHA256, byteLength, err := privateCASUnixReadRecoveryBodyStableAt(
		ctx, shard, preparedRecord.name, maxBytes, false,
	)
	if err != nil {
		return nil, err
	}
	if !privateCASUnixExactObject(identity, preparedRecord.identity) ||
		bodySHA256 != preparedRecord.bodySHA256 || byteLength != preparedRecord.byteLength {
		return nil, errors.New("private CAS prepared recovery record identity or body changed")
	}
	if err := unix.Fstat(shard, &shardStat); err != nil ||
		!privateCASUnixExactObject(shardStat, preparedShard.stat) {
		return nil, errors.Join(errors.New("private CAS prepared recovery shard changed during read"), err)
	}
	if err := privateCASUnixVerifyCurrentShard(authority, shardName, preparedShard.identity); err != nil {
		return nil, err
	}
	return body, nil
}

func privateCASUnixObserveRecoveryOnce(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes int,
	allowOriginalCreates ...bool,
) (privateCASRecoveryObservation, error) {
	root, err := authority.open()
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	defer unix.Close(root)
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil {
		return privateCASRecoveryObservation{}, err
	}
	allowCreates := len(allowOriginalCreates) == 1 && allowOriginalCreates[0]
	entryBound := maxSecurePrivateCASShards
	if allowCreates {
		entryBound *= 2
	}
	entries, err := privateCASUnixReadDirBounded(root, entryBound)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	fingerprint := sha256.New()
	privateCASWriteFingerprintField(fingerprint, []byte("analytix-private-cas-recovery-plan-v1"))
	privateCASUnixFingerprintRecoveryObject(fingerprint, "root", rootStat, nil)
	plan := privateCASPreparedRecoveryPlan{shards: make([]privateCASPreparedRecoveryShard, 0, len(entries))}
	committedMaterials := make([]SecurePrivateCASPreparedMaterialV1, 0)
	var recordCount uint64
	var aggregateBytes uint64
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return privateCASRecoveryObservation{}, err
		}
		shardName := entry.Name()
		if allowCreates && entry.IsDir() && domainprivatecas.CreateDirectoryResidueMatchesShardV1(shardName) {
			residue, err := privateCASUnixOpenObservedCreateRecoveryDirectory(root, shardName, authority.dev)
			if err != nil {
				return privateCASRecoveryObservation{}, err
			}
			var identity unix.Stat_t
			statErr := unix.Fstat(residue, &identity)
			exact, exactErr := privateCASUnixCreateRecoveryExactIdentity(residue, authority.dev)
			empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(residue)
			closeErr := unix.Close(residue)
			if statErr != nil || exactErr != nil || emptyErr != nil || !empty || closeErr != nil {
				return privateCASRecoveryObservation{}, errors.Join(errors.New("original CAS creation residue changed"), statErr, exactErr, emptyErr, closeErr)
			}
			privateCASUnixFingerprintRecoveryObject(fingerprint, "original-create:"+shardName, identity, nil)
			plan.originalCreateResidues = append(plan.originalCreateResidues, privateCASCreateResidueUnixCandidate{name: shardName, identity: exact})
			continue
		}
		if !entry.IsDir() || !validPrivateShard(shardName) {
			return privateCASRecoveryObservation{}, errors.New("private CAS recovery contains a non-canonical shard")
		}
		shard, err := securePrivateOpenShard(root, shardName, false)
		if err != nil {
			return privateCASRecoveryObservation{}, err
		}
		var shardStat unix.Stat_t
		if err := unix.Fstat(shard, &shardStat); err != nil || uint64(shardStat.Dev) != authority.dev {
			_ = unix.Close(shard)
			return privateCASRecoveryObservation{}, errors.New("private CAS recovery shard identity is unsafe")
		}
		privateCASUnixFingerprintRecoveryObject(fingerprint, "shard:"+shardName, shardStat, nil)
		shardEntries, scanErr := privateCASUnixReadDirBounded(shard, maxSecurePrivateCASListRecords)
		if scanErr != nil {
			_ = unix.Close(shard)
			return privateCASRecoveryObservation{}, scanErr
		}
		sort.Slice(shardEntries, func(i, j int) bool { return shardEntries[i].Name() < shardEntries[j].Name() })
		preparedShard := privateCASPreparedRecoveryShard{
			name: shardName, identity: privateCASShardIdentity{dev: uint64(shardStat.Dev), ino: shardStat.Ino}, stat: shardStat,
			entryNames: make([]string, 0, len(shardEntries)), committedNames: make([]string, 0, len(shardEntries)),
			committed: make([]privateCASPreparedRecoveryRecord, 0, len(shardEntries)),
			temps:     make([]privateCASPreparedRecoveryTemp, 0),
		}
		for _, shardEntry := range shardEntries {
			if err := privateCASContextError(ctx); err != nil {
				_ = unix.Close(shard)
				return privateCASRecoveryObservation{}, err
			}
			if recordCount >= maxSecurePrivateCASListRecords {
				_ = unix.Close(shard)
				return privateCASRecoveryObservation{}, ErrSecurePrivateCASMaterializationLimit
			}
			recordCount++
			name := shardEntry.Name()
			preparedShard.entryNames = append(preparedShard.entryNames, name)
			kind := "temp"
			allowEmpty := true
			recordDigest := ""
			originalName, transactionID, phase := name, "", privateCASRecoveryQuarantinePlain
			if filepath.Ext(name) == ".json" {
				digest := strings.TrimSuffix(name, ".json")
				if shardEntry.IsDir() || !validPrivateDigest(digest) || digest[:2] != shardName {
					_ = unix.Close(shard)
					return privateCASRecoveryObservation{}, errors.New("private CAS recovery contains a non-canonical record")
				}
				kind, allowEmpty = "record", false
				recordDigest = digest
				preparedShard.committedNames = append(preparedShard.committedNames, name)
			} else {
				var valid bool
				originalName, transactionID, phase, valid = privateCASRecoveryTempName(name, shardName)
				if shardEntry.IsDir() || !valid {
					_ = unix.Close(shard)
					return privateCASRecoveryObservation{}, errors.New("private CAS recovery contains unknown residue")
				}
			}
			identity, bodySHA256, byteLength, readErr := privateCASUnixObserveRecoveryStableAt(
				ctx, shard, name, maxBytes, allowEmpty,
			)
			if readErr != nil {
				_ = unix.Close(shard)
				return privateCASRecoveryObservation{}, readErr
			}
			if ^uint64(0)-aggregateBytes < byteLength ||
				aggregateBytes+byteLength > maxSecurePrivateCASListAggregateBytes {
				_ = unix.Close(shard)
				return privateCASRecoveryObservation{}, ErrSecurePrivateCASMaterializationLimit
			}
			aggregateBytes += byteLength
			privateCASUnixFingerprintRecoveryObjectBodySHA256(
				fingerprint, kind+":"+shardName+":"+name, identity, bodySHA256,
			)
			if kind == "record" {
				preparedShard.committed = append(preparedShard.committed, privateCASPreparedRecoveryRecord{
					name:       name,
					digest:     recordDigest,
					identity:   identity,
					bodySHA256: bodySHA256,
					byteLength: byteLength,
				})
				committedMaterials = append(committedMaterials, SecurePrivateCASPreparedMaterialV1{
					Digest: recordDigest, BodySHA256: hex.EncodeToString(bodySHA256[:]), ByteLength: byteLength,
				})
			} else {
				linked, err := privateCASUnixValidateRecoveryTemp(shard, shardName, name, originalName, maxBytes)
				if err != nil {
					_ = unix.Close(shard)
					return privateCASRecoveryObservation{}, err
				}
				preparedShard.temps = append(preparedShard.temps, privateCASPreparedRecoveryTemp{
					name: name, originalName: originalName, transactionID: transactionID, phase: phase,
					identity: identity, bodySHA256: bodySHA256, linked: linked,
				})
			}
		}
		plan.shards = append(plan.shards, preparedShard)
		if err := unix.Close(shard); err != nil {
			return privateCASRecoveryObservation{}, err
		}
	}
	current, err := privateCASUnixReadDirBounded(root, len(entries))
	namesMatch := privateCASUnixSameCanonicalNames(privateCASUnixEntryNames(entries), current)
	if allowCreates {
		namesMatch = privateCASOriginalRecoveryRootNamesEqualV1(privateCASUnixEntryNames(entries), current)
	}
	if err != nil || !namesMatch {
		return privateCASRecoveryObservation{}, errors.Join(errors.New("private CAS recovery root inventory changed"), err)
	}
	var sum [32]byte
	copy(sum[:], fingerprint.Sum(nil))
	return privateCASRecoveryObservation{
		fingerprint: sum, committedMaterials: committedMaterials, plan: plan,
	}, nil
}

func privateCASUnixValidatePreparedShardCreateResidueV4(root int, name string, device uint64) error {
	residue, err := privateCASUnixOpenObservedCreateRecoveryDirectory(root, name, device)
	if err != nil {
		return errors.Join(errors.New("private CAS recovery create residue is unsafe"), err)
	}
	_, identityErr := privateCASUnixCreateRecoveryExactIdentity(residue, device)
	empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(residue)
	closeErr := unix.Close(residue)
	if identityErr != nil || emptyErr != nil || !empty || closeErr != nil {
		return errors.Join(
			errors.New("private CAS recovery create residue is non-empty or unsafe"),
			identityErr,
			emptyErr,
			closeErr,
		)
	}
	return nil
}

func privateCASUnixEntryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	sort.Strings(names)
	return names
}

func privateCASUnixFingerprintRecoveryObject(writer hash.Hash, name string, stat unix.Stat_t, body []byte) {
	privateCASUnixFingerprintRecoveryObjectBodySHA256(writer, name, stat, sha256.Sum256(body))
}

func privateCASUnixFingerprintRecoveryObjectBodySHA256(
	writer hash.Hash,
	name string,
	stat unix.Stat_t,
	bodySHA256 [32]byte,
) {
	privateCASWriteFingerprintField(writer, []byte(name))
	privateCASUnixFingerprintRecordBodySHA256(writer, name, bodySHA256, stat)
}

type privateCASUnixOpenRecoveryFile struct {
	file           *os.File
	identity       unix.Stat_t
	maxBytes       int
	expectedDevice uint64
	allowEmpty     bool
}

func privateCASUnixOpenRecoveryFileAt(
	parent int,
	name string,
	maxBytes int,
	allowEmpty bool,
) (*privateCASUnixOpenRecoveryFile, error) {
	expectedDevice, err := privateCASUnixDirectoryDevice(parent)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("private CAS recovery file handle is invalid")
	}
	identity, err := privateCASUnixRecoveryStat(fd, maxBytes, expectedDevice, allowEmpty)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &privateCASUnixOpenRecoveryFile{
		file: file, identity: identity, maxBytes: maxBytes,
		expectedDevice: expectedDevice, allowEmpty: allowEmpty,
	}, nil
}

func (opened *privateCASUnixOpenRecoveryFile) close() error {
	if opened == nil || opened.file == nil {
		return nil
	}
	return opened.file.Close()
}

func (opened *privateCASUnixOpenRecoveryFile) revalidate() error {
	if opened == nil || opened.file == nil {
		return errors.New("private CAS recovery file handle is unavailable")
	}
	current, err := privateCASUnixRecoveryStat(
		int(opened.file.Fd()), opened.maxBytes, opened.expectedDevice, opened.allowEmpty,
	)
	if err != nil || !privateCASUnixExactObject(opened.identity, current) {
		return errors.Join(errors.New("private CAS recovery file changed during read"), err)
	}
	return nil
}

func (opened *privateCASUnixOpenRecoveryFile) hash(ctx context.Context) ([32]byte, error) {
	if opened == nil || opened.file == nil {
		return [32]byte{}, errors.New("private CAS recovery file handle is unavailable")
	}
	digest, err := privateCASHashExact(ctx, opened.file, opened.identity.Size)
	if err != nil {
		return [32]byte{}, err
	}
	if err := opened.revalidate(); err != nil {
		return [32]byte{}, err
	}
	return digest, nil
}

func (opened *privateCASUnixOpenRecoveryFile) body(ctx context.Context) ([]byte, [32]byte, error) {
	if opened == nil || opened.file == nil {
		return nil, [32]byte{}, errors.New("private CAS recovery file handle is unavailable")
	}
	body, digest, err := privateCASReadBodyExact(ctx, opened.file, opened.identity.Size)
	if err != nil {
		return nil, [32]byte{}, err
	}
	if err := opened.revalidate(); err != nil {
		return nil, [32]byte{}, err
	}
	return body, digest, nil
}

func privateCASUnixObserveRecoveryStableAt(
	ctx context.Context,
	parent int,
	name string,
	maxBytes int,
	allowEmpty bool,
) (unix.Stat_t, [32]byte, uint64, error) {
	first, err := privateCASUnixOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return unix.Stat_t{}, [32]byte{}, 0, err
	}
	defer first.close()
	firstSHA256, err := first.hash(ctx)
	if err != nil {
		return unix.Stat_t{}, [32]byte{}, 0, err
	}
	second, err := privateCASUnixOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return unix.Stat_t{}, [32]byte{}, 0, err
	}
	defer second.close()
	secondSHA256, err := second.hash(ctx)
	if err != nil || !privateCASUnixExactObject(first.identity, second.identity) ||
		firstSHA256 != secondSHA256 {
		return unix.Stat_t{}, [32]byte{}, 0, errors.Join(
			errors.New("private CAS recovery file name changed during stable observation"), err,
		)
	}
	if err := first.revalidate(); err != nil {
		return unix.Stat_t{}, [32]byte{}, 0, err
	}
	return first.identity, firstSHA256, uint64(first.identity.Size), nil
}

func privateCASUnixReadRecoveryBodyStableAt(
	ctx context.Context,
	parent int,
	name string,
	maxBytes int,
	allowEmpty bool,
) ([]byte, unix.Stat_t, [32]byte, uint64, error) {
	first, err := privateCASUnixOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return nil, unix.Stat_t{}, [32]byte{}, 0, err
	}
	defer first.close()
	body, firstSHA256, err := first.body(ctx)
	if err != nil {
		return nil, unix.Stat_t{}, [32]byte{}, 0, err
	}
	second, err := privateCASUnixOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return nil, unix.Stat_t{}, [32]byte{}, 0, err
	}
	defer second.close()
	secondSHA256, err := second.hash(ctx)
	if err != nil || !privateCASUnixExactObject(first.identity, second.identity) ||
		firstSHA256 != secondSHA256 {
		return nil, unix.Stat_t{}, [32]byte{}, 0, errors.Join(
			errors.New("private CAS recovery file name changed during stable body read"), err,
		)
	}
	if err := first.revalidate(); err != nil {
		return nil, unix.Stat_t{}, [32]byte{}, 0, err
	}
	return body, first.identity, firstSHA256, uint64(first.identity.Size), nil
}

func privateCASUnixRecoveryStat(fd int, maxBytes int, expectedDevice uint64, allowEmpty bool) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !privateStatRegular(stat) || stat.Uid != uint32(os.Geteuid()) ||
		stat.Mode&0o077 != 0 || stat.Mode&0o7000 != 0 || stat.Nlink == 0 || stat.Nlink > 2 ||
		stat.Size > int64(maxBytes) || !allowEmpty && stat.Size == 0 || uint64(stat.Dev) != expectedDevice ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) {
		return unix.Stat_t{}, errors.New("private CAS recovery file ownership, mode, links, size, or ACL is unsafe")
	}
	return stat, nil
}

func securePrivateCASCreateRecoveryMarker(
	ctx context.Context,
	authority privateCASRootAuthority,
	observation *privateCASRecoveryObservation,
	maxBytes int,
	transactionID string,
) (bool, error) {
	if observation == nil || len(observation.plan.shards) == 0 {
		return false, nil
	}
	current, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil || current.fingerprint != observation.fingerprint {
		return false, errors.Join(errors.New("private CAS recovery changed before marker creation"), err)
	}
	for _, preparedShard := range observation.plan.shards {
		if len(preparedShard.committedNames) != 0 || len(preparedShard.temps) != 0 {
			continue
		}
		root, err := authority.open()
		if err != nil {
			return false, err
		}
		shard, err := securePrivateOpenShard(root, preparedShard.name, false)
		if err != nil {
			_ = unix.Close(root)
			return false, err
		}
		digest := preparedShard.name + transactionID[2:]
		name := "." + digest + ".json-" + transactionID[:24] + ".tmp"
		fd, createErr := unix.Openat(shard, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		if createErr == nil {
			createErr = errors.Join(unix.Fsync(fd), unix.Close(fd), unix.Fsync(shard))
		}
		closeErr := errors.Join(unix.Close(shard), unix.Close(root))
		if createErr != nil || closeErr != nil {
			return false, errors.Join(createErr, closeErr)
		}
		refreshed, refreshErr := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
		if refreshErr != nil {
			return false, refreshErr
		}
		*observation = refreshed
		return true, nil
	}
	return false, nil
}

func securePrivateCASStagePreparedRecovery(
	ctx context.Context,
	authority privateCASRootAuthority,
	observation *privateCASRecoveryObservation,
	maxBytes int,
	transactionID string,
) error {
	if observation == nil || len(observation.plan.shards) == 0 {
		return nil
	}
	current, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil || current.fingerprint != observation.fingerprint {
		return errors.Join(errors.New("private CAS recovery changed before quarantine staging"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		if len(preparedShard.temps) == 0 {
			continue
		}
		shard, err := securePrivateOpenShard(root, preparedShard.name, false)
		if err != nil {
			return err
		}
		if err := privateCASUnixRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
			_ = unix.Close(shard)
			return err
		}
		for tempIndex := range preparedShard.temps {
			temp := &preparedShard.temps[tempIndex]
			if temp.phase == privateCASRecoveryQuarantineCommitted {
				_ = unix.Close(shard)
				return errors.New("private CAS recovery commit marker cannot be restaged")
			}
			if err := privateCASUnixValidatePreparedRecoveryTemp(shard, preparedShard.name, *temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return err
			}
			target, ok := privateCASRecoveryQuarantineName(
				privateCASRecoveryQuarantineStaged, transactionID, temp.originalName, preparedShard.name,
			)
			if !ok {
				_ = unix.Close(shard)
				return errors.New("private CAS recovery stage name is invalid")
			}
			if temp.name != target {
				if err := securePrivateCommitNoReplace(shard, temp.name, target); err != nil {
					_ = unix.Close(shard)
					return err
				}
				privateCASReplacePreparedRecoveryEntry(preparedShard.entryNames, temp.name, target)
				temp.name = target
				if err := privateCASUnixRefreshPreparedRecoveryTemp(shard, temp, maxBytes); err != nil {
					_ = unix.Close(shard)
					return err
				}
			}
			temp.phase = privateCASRecoveryQuarantineStaged
			temp.transactionID = transactionID
		}
		if err := unix.Fsync(shard); err != nil {
			_ = unix.Close(shard)
			return err
		}
		if err := unix.Close(shard); err != nil {
			return err
		}
		if err := privateCASRecoveryTransactionTestCut("after_shard_stage", shardIndex); err != nil {
			return err
		}
	}
	refreshed, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil {
		return err
	}
	*observation = refreshed
	return nil
}

func securePrivateCASRollbackPreparedRecovery(
	ctx context.Context,
	authority privateCASRootAuthority,
	observation *privateCASRecoveryObservation,
	maxBytes int,
) error {
	if observation == nil || len(observation.plan.shards) == 0 {
		return nil
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		shard, err := securePrivateOpenShard(root, preparedShard.name, false)
		if err != nil {
			return err
		}
		if err := privateCASUnixRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
			_ = unix.Close(shard)
			return err
		}
		changed := false
		for tempIndex := range preparedShard.temps {
			temp := &preparedShard.temps[tempIndex]
			if temp.phase == privateCASRecoveryQuarantinePlain {
				continue
			}
			if err := privateCASUnixValidatePreparedRecoveryTemp(shard, preparedShard.name, *temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return err
			}
			if err := securePrivateCommitNoReplace(shard, temp.name, temp.originalName); err != nil {
				_ = unix.Close(shard)
				return err
			}
			privateCASReplacePreparedRecoveryEntry(preparedShard.entryNames, temp.name, temp.originalName)
			temp.name = temp.originalName
			if err := privateCASUnixRefreshPreparedRecoveryTemp(shard, temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return err
			}
			temp.phase = privateCASRecoveryQuarantinePlain
			temp.transactionID = ""
			changed = true
		}
		if changed {
			if err := unix.Fsync(shard); err != nil {
				_ = unix.Close(shard)
				return err
			}
		}
		if err := unix.Close(shard); err != nil {
			return err
		}
	}
	refreshed, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil {
		return err
	}
	*observation = refreshed
	return nil
}

func securePrivateCASMarkPreparedRecoveryCommit(
	ctx context.Context,
	authority privateCASRootAuthority,
	observation *privateCASRecoveryObservation,
	maxBytes int,
	transactionID string,
) (bool, error) {
	if observation == nil || len(observation.plan.shards) == 0 {
		return false, nil
	}
	current, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil || current.fingerprint != observation.fingerprint {
		return false, errors.Join(errors.New("private CAS recovery changed before commit marker"), err)
	}
	root, err := authority.open()
	if err != nil {
		return false, err
	}
	defer unix.Close(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		for tempIndex := range preparedShard.temps {
			temp := &preparedShard.temps[tempIndex]
			if temp.phase != privateCASRecoveryQuarantineStaged || temp.transactionID != transactionID {
				continue
			}
			shard, err := securePrivateOpenShard(root, preparedShard.name, false)
			if err != nil {
				return false, err
			}
			if err := privateCASUnixRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
				_ = unix.Close(shard)
				return false, err
			}
			if err := privateCASUnixValidatePreparedRecoveryTemp(shard, preparedShard.name, *temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return false, err
			}
			target, ok := privateCASRecoveryQuarantineName(
				privateCASRecoveryQuarantineCommitted, transactionID, temp.originalName, preparedShard.name,
			)
			if !ok {
				_ = unix.Close(shard)
				return false, errors.New("private CAS recovery commit marker name is invalid")
			}
			if err := securePrivateCommitNoReplace(shard, temp.name, target); err != nil {
				_ = unix.Close(shard)
				return false, err
			}
			privateCASReplacePreparedRecoveryEntry(preparedShard.entryNames, temp.name, target)
			temp.name = target
			markerRenamed := true
			if err := privateCASRecoveryTransactionTestCut("after_commit_marker_rename", shardIndex); err != nil {
				_ = unix.Close(shard)
				return markerRenamed, err
			}
			if err := privateCASUnixRefreshPreparedRecoveryTemp(shard, temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return markerRenamed, err
			}
			temp.phase = privateCASRecoveryQuarantineCommitted
			if err := unix.Fsync(shard); err != nil {
				_ = unix.Close(shard)
				return markerRenamed, err
			}
			if err := privateCASRecoveryTransactionTestCut("after_commit_marker_sync", shardIndex); err != nil {
				_ = unix.Close(shard)
				return markerRenamed, err
			}
			if err := unix.Close(shard); err != nil {
				return markerRenamed, err
			}
			refreshed, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
			if err != nil {
				return markerRenamed, err
			}
			*observation = refreshed
			return markerRenamed, nil
		}
	}
	return false, nil
}

func securePrivateCASCommitPreparedRecovery(
	ctx context.Context,
	authority privateCASRootAuthority,
	observation *privateCASRecoveryObservation,
	maxBytes int,
	transactionID string,
	preserveMarker bool,
) error {
	if observation == nil || len(observation.plan.shards) == 0 {
		return nil
	}
	current, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil || current.fingerprint != observation.fingerprint {
		return errors.Join(errors.New("private CAS recovery changed before committed cleanup"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		shard, err := securePrivateOpenShard(root, preparedShard.name, false)
		if err != nil {
			return err
		}
		if err := privateCASUnixRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
			_ = unix.Close(shard)
			return err
		}
		remaining := append([]string(nil), preparedShard.committedNames...)
		for _, temp := range preparedShard.temps {
			if temp.transactionID != transactionID ||
				(temp.phase != privateCASRecoveryQuarantineStaged && temp.phase != privateCASRecoveryQuarantineCommitted) {
				_ = unix.Close(shard)
				return errors.New("private CAS committed cleanup contains an unrelated residue")
			}
			if preserveMarker && temp.phase == privateCASRecoveryQuarantineCommitted {
				remaining = append(remaining, temp.name)
				continue
			}
			if err := privateCASUnixValidatePreparedRecoveryTemp(shard, preparedShard.name, temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return err
			}
			if err := unix.Unlinkat(shard, temp.name, 0); err != nil {
				_ = unix.Close(shard)
				return err
			}
		}
		if err := unix.Fsync(shard); err != nil {
			_ = unix.Close(shard)
			return err
		}
		if err := privateCASUnixRequireExactEntryNames(shard, remaining); err != nil {
			_ = unix.Close(shard)
			return err
		}
		empty := len(remaining) == 0
		if err := unix.Close(shard); err != nil {
			return err
		}
		if empty {
			if err := privateCASUnixVerifyCurrentShard(authority, preparedShard.name, preparedShard.identity); err != nil {
				return err
			}
			if err := unix.Unlinkat(root, preparedShard.name, unix.AT_REMOVEDIR); err != nil {
				return err
			}
		}
	}
	if err := unix.Fsync(root); err != nil {
		return err
	}
	refreshed, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil {
		return err
	}
	*observation = refreshed
	return nil
}

func securePrivateCASFinalizePreparedRecoveryCommit(
	ctx context.Context,
	authority privateCASRootAuthority,
	observation *privateCASRecoveryObservation,
	maxBytes int,
	transactionID string,
) error {
	if observation == nil {
		return errors.New("private CAS recovery commit marker plan is invalid")
	}
	current, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil || current.fingerprint != observation.fingerprint {
		return errors.Join(errors.New("private CAS recovery changed before commit marker retirement"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	found := false
	for _, preparedShard := range observation.plan.shards {
		for _, temp := range preparedShard.temps {
			if temp.phase != privateCASRecoveryQuarantineCommitted || temp.transactionID != transactionID || found {
				continue
			}
			shard, err := securePrivateOpenShard(root, preparedShard.name, false)
			if err != nil {
				return err
			}
			if err := privateCASUnixRequireExactEntryNames(shard, append(append([]string(nil), preparedShard.committedNames...), temp.name)); err != nil {
				_ = unix.Close(shard)
				return err
			}
			if err := privateCASUnixValidatePreparedRecoveryTemp(shard, preparedShard.name, temp, maxBytes); err != nil {
				_ = unix.Close(shard)
				return err
			}
			if err := unix.Unlinkat(shard, temp.name, 0); err != nil {
				_ = unix.Close(shard)
				return err
			}
			if err := unix.Fsync(shard); err != nil {
				_ = unix.Close(shard)
				return err
			}
			empty := len(preparedShard.committedNames) == 0
			if err := unix.Close(shard); err != nil {
				return err
			}
			if empty {
				if err := privateCASUnixVerifyCurrentShard(authority, preparedShard.name, preparedShard.identity); err != nil {
					return err
				}
				if err := unix.Unlinkat(root, preparedShard.name, unix.AT_REMOVEDIR); err != nil {
					return err
				}
			}
			found = true
		}
	}
	if !found {
		return errors.New("private CAS recovery commit marker was not found")
	}
	if err := unix.Fsync(root); err != nil {
		return err
	}
	refreshed, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil {
		return err
	}
	*observation = refreshed
	return nil
}

func privateCASUnixValidatePreparedRecoveryTemp(
	shard int,
	shardName string,
	temp privateCASPreparedRecoveryTemp,
	maxBytes int,
) error {
	linked, err := privateCASUnixValidateRecoveryTemp(shard, shardName, temp.name, temp.originalName, maxBytes)
	if err != nil || linked != temp.linked {
		return errors.Join(errors.New("private CAS prepared recovery temp topology changed"), err)
	}
	identity, bodySHA256, _, err := privateCASUnixObserveRecoveryStableAt(
		context.Background(), shard, temp.name, maxBytes, true,
	)
	if err != nil || !privateCASUnixExactObject(identity, temp.identity) || bodySHA256 != temp.bodySHA256 {
		return errors.Join(errors.New("private CAS prepared recovery temp identity changed"), err)
	}
	return nil
}

func privateCASUnixRefreshPreparedRecoveryTemp(
	shard int,
	temp *privateCASPreparedRecoveryTemp,
	maxBytes int,
) error {
	if temp == nil {
		return errors.New("private CAS prepared recovery temp is unavailable")
	}
	identity, bodySHA256, _, err := privateCASUnixObserveRecoveryStableAt(
		context.Background(), shard, temp.name, maxBytes, true,
	)
	if err != nil || !privateCASUnixSameObject(identity, temp.identity) || bodySHA256 != temp.bodySHA256 {
		return errors.Join(errors.New("private CAS prepared recovery rename changed the residue object"), err)
	}
	temp.identity = identity
	return nil
}

func privateCASUnixRequireExactEntryNames(parent int, expected []string) error {
	actualEntries, err := privateCASUnixReadDirBounded(parent, len(expected))
	if err != nil {
		return err
	}
	actual := make([]string, len(actualEntries))
	for index, entry := range actualEntries {
		actual[index] = entry.Name()
	}
	wanted := append([]string(nil), expected...)
	sort.Strings(actual)
	sort.Strings(wanted)
	if len(actual) != len(wanted) || strings.Join(actual, "\x00") != strings.Join(wanted, "\x00") {
		return errors.New("private CAS prepared recovery inventory no longer matches its exact plan")
	}
	return nil
}

func privateCASUnixPreflightRecoveryShard(
	ctx context.Context,
	shard int,
	shardName string,
	maxBytes int,
	rejectTransactionPhases bool,
) error {
	var linkedRecords uint64
	var linkedTemps uint64
	err := privateCASUnixWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			digest := strings.TrimSuffix(name, ".json")
			if entry.IsDir() || !validPrivateDigest(digest) || digest[:2] != shardName {
				return errors.New("private CAS recovery contains a non-canonical record")
			}
			stat, err := securePrivateStatAt(shard, name)
			if err != nil || stat.Size <= 0 || stat.Size > int64(maxBytes) || stat.Nlink == 0 || stat.Nlink > 2 {
				return errors.New("private CAS recovery record is unsafe")
			}
			if stat.Nlink == 2 {
				linkedRecords++
			}
			return nil
		}
		originalName, _, phase, valid := privateCASRecoveryTempName(name, shardName)
		if !valid {
			return errors.New("private CAS recovery contains unknown residue")
		}
		if rejectTransactionPhases && phase != privateCASRecoveryQuarantinePlain {
			return errors.New("private CAS staged or committed recovery residue has no signed journal")
		}
		linked, err := privateCASUnixValidateRecoveryTemp(shard, shardName, name, originalName, maxBytes)
		if err != nil {
			return err
		}
		if linked {
			linkedTemps++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if linkedRecords != linkedTemps {
		return errors.New("private CAS recovery contains an untracked record hardlink")
	}
	return nil
}

func privateCASUnixValidateRecoveredShard(ctx context.Context, shard int, shardName string, maxBytes int) (bool, error) {
	var records uint64
	err := privateCASUnixWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		name := entry.Name()
		digest := strings.TrimSuffix(name, ".json")
		if entry.IsDir() || filepath.Ext(name) != ".json" || !validPrivateDigest(digest) || digest[:2] != shardName {
			return errors.New("private CAS recovery did not converge to canonical records")
		}
		stat, err := securePrivateStatAt(shard, name)
		if err != nil || stat.Size <= 0 || stat.Size > int64(maxBytes) || stat.Nlink != 1 {
			return errors.New("private CAS recovered record is unsafe")
		}
		records++
		return nil
	})
	return records == 0, err
}

func privateCASUnixValidateRecoveryTemp(shard int, shardName, name, originalName string, maxBytes int) (bool, error) {
	derived, _, _, valid := privateCASRecoveryTempName(name, shardName)
	if !valid || derived != originalName || !privateWriteTempName(originalName, shardName) {
		return false, errors.New("private CAS recovery temp name is invalid")
	}
	tempStat, err := securePrivateStatAt(shard, name)
	if err != nil || !privateStatRegular(tempStat) || tempStat.Size > int64(maxBytes) {
		return false, errors.Join(err, errors.New("private CAS recovery temp is unsafe"))
	}
	marker := strings.Index(originalName, ".json-")
	finalName := strings.TrimPrefix(originalName[:marker+len(".json")], ".")
	finalStat, finalErr := securePrivateStatAt(shard, finalName)
	if finalErr == nil {
		sameFile := tempStat.Dev == finalStat.Dev && tempStat.Ino == finalStat.Ino
		if sameFile && (tempStat.Nlink != 2 || finalStat.Nlink != 2) ||
			!sameFile && (tempStat.Nlink != 1 || finalStat.Nlink != 1) {
			return false, errors.New("private CAS recovery temp has an unsafe link topology")
		}
		return sameFile, nil
	}
	if finalErr != nil && !errors.Is(finalErr, os.ErrNotExist) {
		return false, finalErr
	}
	if errors.Is(finalErr, os.ErrNotExist) && tempStat.Nlink != 1 {
		return false, errors.New("private CAS recovery orphan temp is not single-link")
	}
	return false, nil
}

func privateCASUnixReadDirBounded(parent int, limit int) ([]os.DirEntry, error) {
	if limit < 0 || limit >= int(^uint(0)>>1) {
		return nil, errors.New("private CAS directory bound is invalid")
	}
	fd, err := unix.Openat(
		parent,
		".",
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(fd), "private-cas-bounded-directory")
	if directory == nil {
		_ = unix.Close(fd)
		return nil, errors.New("private CAS bounded directory handle is invalid")
	}
	entries, readErr := directory.ReadDir(limit + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = directory.Close()
		return nil, readErr
	}
	if len(entries) > limit {
		_ = directory.Close()
		return nil, errors.New("private CAS directory entry bound exceeded")
	}
	if readErr == nil {
		extra, extraErr := directory.ReadDir(1)
		if len(extra) != 0 || !errors.Is(extraErr, io.EOF) {
			_ = directory.Close()
			return nil, errors.Join(errors.New("private CAS directory entry bound exceeded"), extraErr)
		}
	}
	if closeErr := directory.Close(); closeErr != nil {
		return nil, closeErr
	}
	return entries, nil
}

func privateCASUnixCanonicalRecordNames(shard string, entries []os.DirEntry) ([]string, error) {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		digest := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || !validPrivateDigest(digest) || digest[:2] != shard {
			return nil, errors.New("private CAS inventory contains a non-canonical record")
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func privateCASUnixSameRecordNames(shard string, expected []string, entries []os.DirEntry) bool {
	actual, err := privateCASUnixCanonicalRecordNames(shard, entries)
	return err == nil && len(expected) == len(actual) && strings.Join(expected, "\x00") == strings.Join(actual, "\x00")
}

func privateCASUnixOpenPinnedShard(root int, pins map[string]privateCASShardIdentity, shardName string, create bool) (int, privateCASShardIdentity, error) {
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil || rootStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return -1, privateCASShardIdentity{}, errors.New("private CAS root device is unavailable")
	}
	rootDevice := uint64(rootStat.Dev)
	shard, err := securePrivateOpenShard(root, shardName, false)
	if errors.Is(err, os.ErrNotExist) && create {
		shard, err = privateCASUnixCreateBoundDirectory(root, shardName, rootDevice)
	}
	if err != nil {
		return -1, privateCASShardIdentity{}, err
	}
	identity, err := privateCASUnixIdentity(shard, rootDevice)
	if err != nil {
		_ = unix.Close(shard)
		return -1, privateCASShardIdentity{}, err
	}
	if pinned, ok := pins[shardName]; ok && pinned != identity {
		_ = unix.Close(shard)
		return -1, privateCASShardIdentity{}, errors.New("private CAS shard identity changed")
	}
	if _, ok := pins[shardName]; !ok {
		pins[shardName] = identity
	}
	return shard, identity, nil
}

// privateCASUnixOpenPinnedShardForCommit distinguishes a shard installed by
// this call from one that appeared concurrently. The exclusive no-replace
// directory commit fails on a collision instead of retrospectively treating
// the raced shard as host-created.
func privateCASUnixOpenPinnedShardForCommit(
	root int,
	pins map[string]privateCASShardIdentity,
	shardName string,
	preserveOriginalCreation ...bool,
) (int, privateCASShardIdentity, bool, error) {
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil || rootStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return -1, privateCASShardIdentity{}, false, errors.New("private CAS root device is unavailable")
	}
	rootDevice := uint64(rootStat.Dev)
	pinned, hadPin := pins[shardName]
	shard, err := securePrivateOpenShard(root, shardName, false)
	created := false
	if err == nil && !hadPin {
		_ = unix.Close(shard)
		return -1, privateCASShardIdentity{}, false, errors.New("private CAS unprepared shard appeared before commit")
	}
	if errors.Is(err, os.ErrNotExist) && hadPin {
		return -1, privateCASShardIdentity{}, false, errors.New("private CAS pinned shard disappeared before commit")
	}
	if errors.Is(err, os.ErrNotExist) {
		if len(preserveOriginalCreation) == 1 && preserveOriginalCreation[0] {
			shard, err = privateCASUnixCreateIndependentOriginalShardV1(root, shardName, rootDevice)
		} else {
			shard, err = privateCASUnixCreateBoundDirectory(root, shardName, rootDevice)
		}
		created = err == nil
	}
	if err != nil {
		return -1, privateCASShardIdentity{}, false, err
	}
	identity, err := privateCASUnixIdentity(shard, rootDevice)
	if err != nil {
		_ = unix.Close(shard)
		return -1, privateCASShardIdentity{}, false, err
	}
	if hadPin && pinned != identity {
		_ = unix.Close(shard)
		return -1, privateCASShardIdentity{}, false, errors.New("private CAS shard identity changed")
	}
	if _, ok := pins[shardName]; !ok {
		pins[shardName] = identity
	}
	return shard, identity, created, nil
}

func privateCASUnixIdentity(fd int, expectedDevice uint64) (privateCASShardIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil ||
		!existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(fd) || uint64(stat.Dev) != expectedDevice {
		return privateCASShardIdentity{}, errors.New("private CAS shard is unsafe")
	}
	return privateCASShardIdentity{dev: uint64(stat.Dev), ino: stat.Ino}, nil
}

func privateCASUnixVerifyCurrentShard(authority privateCASRootAuthority, shardName string, expected privateCASShardIdentity) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	shard, err := securePrivateOpenShard(root, shardName, false)
	if err != nil {
		return err
	}
	defer unix.Close(shard)
	current, err := privateCASUnixIdentity(shard, authority.dev)
	if err != nil || current != expected {
		return errors.New("private CAS shard path identity changed")
	}
	return nil
}

func privateCASUnixReadAt(parent int, name string, maxBytes int) ([]byte, error) {
	body, _, err := privateCASUnixReadStableAt(parent, name, maxBytes)
	return body, err
}

func securePrivateCASObserveAddition(
	authority privateCASRootAuthority,
	pins map[string]privateCASShardIdentity,
	digest string,
	maxBytes int,
	validations ...privateCASOriginalWriteValidationV1,
) (privateCASAdditionObservation, error) {
	root, err := authority.open()
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	defer unix.Close(root)
	validateRoot := func() error {
		if handled, err := validateOriginalPrivateCASWriteRootV1(authority, pins, maxBytes, validations); handled {
			return err
		}
		return privateCASUnixValidatePinnedRootStrictAt(root, pins)
	}
	if err := validateRoot(); err != nil {
		return privateCASAdditionObservation{}, err
	}
	shard, shardIdentity, err := privateCASUnixOpenPinnedShard(root, pins, digest[:2], false)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	defer unix.Close(shard)
	body, recordStat, err := privateCASUnixReadStableAt(shard, digest+".json", maxBytes)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	rootMetadata, err := privateCASUnixSnapshotMetadata(root)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	shardMetadata, err := privateCASUnixSnapshotMetadata(shard)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	record, err := unix.Openat(shard, digest+".json", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	currentRecord, statErr := privateCASUnixRecordStat(record, maxBytes, uint64(recordStat.Dev))
	recordMetadata, metadataErr := privateCASUnixSnapshotMetadata(record)
	closeErr := unix.Close(record)
	if statErr != nil || metadataErr != nil || closeErr != nil ||
		!acceptedFinalCASSameUnixFile(recordStat, currentRecord) || recordStat.Uid != currentRecord.Uid || recordStat.Gid != currentRecord.Gid {
		return privateCASAdditionObservation{}, errors.Join(
			errors.New("private CAS addition record identity changed during observation"), statErr, metadataErr, closeErr,
		)
	}
	if err := privateCASUnixVerifyCurrentShard(authority, digest[:2], shardIdentity); err != nil {
		return privateCASAdditionObservation{}, err
	}
	if err := validateRoot(); err != nil {
		return privateCASAdditionObservation{}, err
	}
	return privateCASAdditionObservation{
		body:                 body,
		rootIdentityDigest:   privateCASUnixObjectIdentityDigest("root", authority.dev, authority.ino),
		shardIdentityDigest:  privateCASUnixObjectIdentityDigest("shard", shardIdentity.dev, shardIdentity.ino),
		recordIdentityDigest: privateCASUnixObjectIdentityDigest("record", uint64(recordStat.Dev), recordStat.Ino),
		rootMetadata:         rootMetadata, shardMetadata: shardMetadata, recordMetadata: recordMetadata,
	}, nil
}

func privateCASUnixSnapshotMetadata(fd int) (SecurePrivateCASSnapshotMetadataV1, error) {
	duplicate, err := unix.Dup(fd)
	if err != nil {
		return SecurePrivateCASSnapshotMetadataV1{}, err
	}
	file := os.NewFile(uintptr(duplicate), "private-cas-snapshot-metadata")
	if file == nil {
		_ = unix.Close(duplicate)
		return SecurePrivateCASSnapshotMetadataV1{}, errors.New("private CAS metadata handle is invalid")
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil {
		return SecurePrivateCASSnapshotMetadataV1{}, errors.Join(statErr, closeErr)
	}
	return SecurePrivateCASSnapshotMetadataV1{
		Mode: uint32(info.Mode()), Size: info.Size(), ModTimeUnixNano: info.ModTime().UnixNano(),
	}, nil
}

func privateCASUnixObjectIdentityDigest(kind string, device, inode uint64) string {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix-private-cas-object-identity-v1"))
	privateCASWriteFingerprintField(hasher, []byte(kind))
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], device)
	privateCASWriteFingerprintField(hasher, number[:])
	binary.BigEndian.PutUint64(number[:], inode)
	privateCASWriteFingerprintField(hasher, number[:])
	return hex.EncodeToString(hasher.Sum(nil))
}

func privateCASUnixReadStableAt(parent int, name string, maxBytes int) ([]byte, unix.Stat_t, error) {
	return privateCASUnixReadStableAtWithHook(parent, name, maxBytes, nil)
}

func privateCASUnixReadStableAtWithHook(
	parent int,
	name string,
	maxBytes int,
	betweenReads func(),
) ([]byte, unix.Stat_t, error) {
	expectedDevice, err := privateCASUnixDirectoryDevice(parent)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	body, identity, err := privateCASUnixReadOnceAt(parent, name, maxBytes, expectedDevice)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	if betweenReads != nil {
		betweenReads()
	}
	readback, current, err := privateCASUnixReadOnceAt(parent, name, maxBytes, expectedDevice)
	if err != nil || !acceptedFinalCASSameUnixFile(identity, current) || identity.Uid != current.Uid ||
		identity.Gid != current.Gid || !bytes.Equal(body, readback) {
		return nil, unix.Stat_t{}, errors.New("private CAS record changed during stable read")
	}
	finalIdentity, err := privateCASUnixRecordIdentityAt(parent, name, maxBytes, expectedDevice)
	if err != nil || !acceptedFinalCASSameUnixFile(current, finalIdentity) || current.Uid != finalIdentity.Uid ||
		current.Gid != finalIdentity.Gid {
		return nil, unix.Stat_t{}, errors.New("private CAS record name changed after stable read")
	}
	return body, identity, nil
}

func privateCASUnixRecordIdentityAt(parent int, name string, maxBytes int, expectedDevice uint64) (unix.Stat_t, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return unix.Stat_t{}, os.ErrNotExist
	}
	if err != nil {
		return unix.Stat_t{}, err
	}
	defer unix.Close(fd)
	return privateCASUnixRecordStat(fd, maxBytes, expectedDevice)
}

func privateCASUnixReadOnceAt(parent int, name string, maxBytes int, expectedDevice uint64) ([]byte, unix.Stat_t, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, unix.Stat_t{}, os.ErrNotExist
	}
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, unix.Stat_t{}, errors.New("private CAS record handle is invalid")
	}
	defer file.Close()
	initial, err := privateCASUnixRecordStat(fd, maxBytes, expectedDevice)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || int64(len(body)) != initial.Size {
		return nil, unix.Stat_t{}, errors.New("private CAS record read failed")
	}
	current, err := privateCASUnixRecordStat(fd, maxBytes, expectedDevice)
	if err != nil || !acceptedFinalCASSameUnixFile(initial, current) || initial.Uid != current.Uid || initial.Gid != current.Gid {
		return nil, unix.Stat_t{}, errors.New("private CAS record changed during read")
	}
	return body, initial, nil
}

func privateCASUnixRecordStat(fd int, maxBytes int, expectedDevice uint64) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil ||
		!existingPrivateAuthorityFileSafe(stat, uint32(os.Geteuid()), int64(maxBytes)) ||
		stat.Mode&0o7000 != 0 || !existingPrivateAuthorityExtendedSecuritySafe(fd) || uint64(stat.Dev) != expectedDevice {
		return unix.Stat_t{}, errors.New("private CAS record ownership, mode, type, links, size, or ACL is unsafe")
	}
	return stat, nil
}

func privateCASUnixDirectoryDevice(fd int) (uint64, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Dev == 0 {
		return 0, errors.New("private CAS directory device is unavailable")
	}
	return uint64(stat.Dev), nil
}

func privateCASUnixSameObject(left, right unix.Stat_t) bool {
	return uint64(left.Dev) == uint64(right.Dev) && left.Ino == right.Ino && left.Mode == right.Mode &&
		left.Nlink == right.Nlink && left.Uid == right.Uid && left.Gid == right.Gid && left.Size == right.Size
}

func privateCASUnixSameCanonicalNames(expected []string, entries []os.DirEntry) bool {
	actual := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return false
		}
		actual = append(actual, entry.Name())
	}
	sort.Strings(actual)
	return len(expected) == len(actual) && strings.Join(expected, "\x00") == strings.Join(actual, "\x00")
}
