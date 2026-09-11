//go:build windows

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
	"golang.org/x/sys/windows"
)

type privateCASShardIdentity struct {
	id privateWindowsFileIDInfo
}

type privateCASRootAuthority struct {
	binding privatecasport.RootBinding
	id      privateWindowsFileIDInfo
}

type privateCASRootAnchor struct {
	handle   windows.Handle
	identity privateWindowsFileIDInfo
	valid    bool
}

type privateCASShardAnchor struct {
	handle   windows.Handle
	identity privateWindowsFileIDInfo
	valid    bool
}

func privateCASOpenRootAnchor(authority privateCASRootAuthority) (privateCASRootAnchor, error) {
	handle, err := authority.open()
	if err != nil {
		return privateCASRootAnchor{}, err
	}
	return privateCASRootAnchor{handle: handle, identity: authority.id, valid: true}, nil
}

func privateCASValidateRootAnchor(anchor privateCASRootAnchor, authority privateCASRootAuthority) error {
	if !anchor.valid || anchor.handle == 0 || anchor.identity != authority.id {
		return errors.New("private CAS Windows root anchor is invalid")
	}
	anchored, err := privateWindowsValidateAuthorityObject(anchor.handle, true, 0)
	if err != nil || anchored.id != anchor.identity {
		return errors.Join(errors.New("private CAS Windows root anchor changed"), err)
	}
	current, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(current)
	currentIdentity, err := privateWindowsValidateAuthorityObject(current, true, 0)
	if err != nil || currentIdentity.id != anchor.identity {
		return errors.Join(errors.New("private CAS Windows root path no longer names its anchor"), err)
	}
	return nil
}

func privateCASCloseRootAnchor(anchor privateCASRootAnchor) error {
	if !anchor.valid || anchor.handle == 0 {
		return nil
	}
	return windows.CloseHandle(anchor.handle)
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
	defer windows.CloseHandle(root)
	shard, err := privateWindowsOpenShard(root, name, false)
	if err != nil {
		return privateCASShardAnchor{}, err
	}
	current, err := privateCASWindowsIdentity(shard)
	if err != nil || current != identity {
		_ = windows.CloseHandle(shard)
		return privateCASShardAnchor{}, errors.Join(errors.New("private CAS Windows shard anchor identity changed"), err)
	}
	return privateCASShardAnchor{handle: shard, identity: identity.id, valid: true}, nil
}

func privateCASValidateShardAnchor(
	authority privateCASRootAuthority,
	name string,
	identity privateCASShardIdentity,
	anchor privateCASShardAnchor,
) error {
	if !anchor.valid || anchor.handle == 0 || anchor.identity != identity.id {
		return errors.New("private CAS Windows shard anchor is invalid")
	}
	anchored, err := privateWindowsValidateAuthorityObject(anchor.handle, true, 0)
	if err != nil || anchored.id != anchor.identity {
		return errors.Join(errors.New("private CAS Windows shard anchor changed"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	current, err := privateWindowsOpenShard(root, name, false)
	if err != nil {
		return err
	}
	currentIdentity, identityErr := privateCASWindowsIdentity(current)
	closeErr := windows.CloseHandle(current)
	if identityErr != nil || closeErr != nil || currentIdentity != identity {
		return errors.Join(errors.New("private CAS Windows shard path no longer names its anchor"), identityErr, closeErr)
	}
	return nil
}

func privateCASCloseShardAnchor(anchor privateCASShardAnchor) error {
	if !anchor.valid || anchor.handle == 0 {
		return nil
	}
	return windows.CloseHandle(anchor.handle)
}

type privateCASPreparedRecoveryPlan struct {
	rootIdentity           privateWindowsObjectIdentity
	shards                 []privateCASPreparedRecoveryShard
	originalCreateResidues []privateCASCreateResidueWindowsCandidate
}

type privateCASPreparedRecoveryShard struct {
	name           string
	identity       privateWindowsObjectIdentity
	entryNames     []string
	committedNames []string
	committed      []privateCASPreparedRecoveryRecord
	temps          []privateCASPreparedRecoveryTemp
}

type privateCASPreparedRecoveryRecord struct {
	name       string
	digest     string
	identity   privateWindowsObjectIdentity
	bodySHA256 [32]byte
	byteLength uint64
}

type privateCASPreparedRecoveryTemp struct {
	name          string
	originalName  string
	transactionID string
	phase         privateCASRecoveryQuarantinePhase
	identity      privateWindowsObjectIdentity
	bodySHA256    [32]byte
	linked        bool
}

func newPrivateCASRootAuthority(binding privatecasport.RootBinding) (privateCASRootAuthority, error) {
	root, err := privateCASWindowsOpenBoundRoot(binding, true)
	if err != nil {
		return privateCASRootAuthority{}, err
	}
	defer windows.CloseHandle(root)
	identity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return privateCASRootAuthority{}, err
	}
	if err := privateWindowsSyncDirectory(root); err != nil {
		return privateCASRootAuthority{}, err
	}
	return privateCASRootAuthority{binding: binding, id: identity.id}, nil
}

func existingPrivateCASRootAuthority(binding privatecasport.RootBinding) (privateCASRootAuthority, bool, error) {
	root, err := privateCASWindowsOpenBoundRoot(binding, false)
	if privateWindowsNotFound(err) {
		return privateCASRootAuthority{}, false, nil
	}
	if err != nil {
		return privateCASRootAuthority{}, false, err
	}
	defer windows.CloseHandle(root)
	identity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return privateCASRootAuthority{}, false, err
	}
	return privateCASRootAuthority{binding: binding, id: identity.id}, true, nil
}

func (authority privateCASRootAuthority) open() (windows.Handle, error) {
	root, err := privateCASWindowsOpenBoundRoot(authority.binding, false)
	if err != nil {
		return 0, err
	}
	identity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil || identity.id != authority.id {
		_ = windows.CloseHandle(root)
		return 0, errors.New("private CAS Windows root identity changed")
	}
	return root, nil
}

func privateCASWindowsOpenBoundRoot(binding privatecasport.RootBinding, create bool, initials ...privateCASOriginalLeafInitializationV1) (windows.Handle, error) {
	if len(initials) > 1 || len(initials) == 1 && !create {
		return 0, errors.New("original CAS Windows initialization binding is invalid")
	}
	if binding.RootIdentity.Kind != privatecasport.DirectoryIdentityWindows ||
		binding.RootIdentity.Device != 0 || binding.RootIdentity.Inode != 0 ||
		binding.RootIdentity.VolumeSerial == 0 || binding.RootIdentity.FileID == [16]byte{} ||
		!filepath.IsAbs(binding.RootPath) {
		return 0, errors.New("private CAS Windows root binding is invalid")
	}
	components, err := privateCASWindowsRelativeComponents(binding.RelativePath)
	if err != nil {
		return 0, err
	}
	current, err := privateWindowsOpenAbsoluteDirectory(binding.RootPath, false)
	if err != nil {
		return 0, err
	}
	rootIdentity, err := privateWindowsValidateAuthorityObject(current, true, 0)
	if err != nil || rootIdentity.id.VolumeSerialNumber != binding.RootIdentity.VolumeSerial ||
		rootIdentity.id.FileID != binding.RootIdentity.FileID {
		_ = windows.CloseHandle(current)
		return 0, errors.New("private CAS frozen Windows root identity changed")
	}
	currentPath := binding.RootPath
	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)
		next, present, openErr := privateCASWindowsOpenExactBoundDirectory(
			current,
			component,
			binding.RootIdentity.VolumeSerial,
		)
		created := false
		if openErr == nil && !present && create {
			if len(initials) == 1 {
				openErr = initials[0].validateCreatePathV1(currentPath)
				if openErr == nil {
					next, openErr = privateCASWindowsCreateIndependentOriginalDirectoryV1(current, component, binding.RootIdentity.VolumeSerial)
					if openErr == nil {
						openErr = initials[0].createdV1(currentPath)
						if openErr != nil {
							_ = windows.CloseHandle(next)
						}
					}
				}
			} else {
				next, openErr = privateCASWindowsCreateBoundDirectory(current, component, binding.RootIdentity.VolumeSerial)
			}
			created = openErr == nil
			present = created
		}
		if openErr == nil && !present {
			openErr = os.ErrNotExist
		}
		if openErr != nil {
			_ = windows.CloseHandle(current)
			return 0, openErr
		}
		identity, identityErr := privateWindowsValidateAuthorityObject(next, true, 0)
		if identityErr != nil || identity.id.VolumeSerialNumber != binding.RootIdentity.VolumeSerial {
			_ = windows.CloseHandle(next)
			_ = windows.CloseHandle(current)
			return 0, errors.New("private CAS Windows descendant is unsafe")
		}
		if created {
			if err := privateWindowsSyncDirectory(next); err != nil {
				_ = windows.CloseHandle(next)
				_ = windows.CloseHandle(current)
				return 0, err
			}
			if err := privateWindowsSyncDirectory(current); err != nil {
				_ = windows.CloseHandle(next)
				_ = windows.CloseHandle(current)
				return 0, err
			}
		}
		if err := privateWindowsVerifyRelativeDirectoryIdentity(current, component, next); err != nil {
			_ = windows.CloseHandle(next)
			_ = windows.CloseHandle(current)
			return 0, err
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	return current, nil
}

func privateCASWindowsCreateBoundDirectory(
	parent windows.Handle,
	component string,
	volume uint64,
) (windows.Handle, error) {
	temporary := privateCASCreateDirectoryResidueName(component)
	residue, present, err := privateCASWindowsOpenExactBoundDirectory(parent, temporary, volume)
	if err != nil {
		return 0, err
	}
	if present {
		_ = windows.CloseHandle(residue)
		return 0, errors.New("private CAS Windows pre-existing create residue requires global recovery")
	}
	staged, err := privateWindowsOpenRelative(
		parent,
		temporary,
		privateWindowsMutateDirectoryAccess,
		windows.FILE_CREATE,
		true,
	)
	if privateWindowsCollision(err) {
		return 0, errors.New("private CAS Windows create residue raced")
	}
	if err != nil {
		return 0, err
	}
	tempExists := true
	defer func() {
		if tempExists {
			_ = privateWindowsDeleteHandle(staged)
			_ = privateWindowsSyncDirectory(parent)
		}
		_ = windows.CloseHandle(staged)
	}()
	stagedIdentity, err := privateCASWindowsCreateRecoveryExactIdentity(staged, volume)
	if err != nil {
		return 0, err
	}
	if err := errors.Join(
		privateWindowsSyncDirectory(staged),
		privateWindowsSyncDirectory(parent),
	); err != nil {
		return 0, err
	}
	if err := privateWindowsRenameRelativeNoReplace(staged, parent, component); err != nil {
		if privateWindowsCollision(err) {
			return 0, errors.New("private CAS Windows descendant creation collided")
		}
		return 0, err
	}
	tempExists = false
	if err := privateWindowsSyncDirectory(parent); err != nil {
		return 0, err
	}
	installed, present, err := privateCASWindowsOpenExactBoundDirectory(parent, component, volume)
	if err != nil || !present {
		if err == nil {
			err = os.ErrNotExist
		}
		return 0, err
	}
	installedIdentity, err := privateCASWindowsCreateRecoveryExactIdentity(installed, volume)
	nameErr := privateWindowsVerifyRelativeDirectoryIdentity(parent, component, installed)
	if err != nil || installedIdentity.stable != stagedIdentity.stable || nameErr != nil {
		_ = windows.CloseHandle(installed)
		return 0, errors.Join(
			errors.New("private CAS Windows installed directory identity changed"),
			err,
			nameErr,
		)
	}
	return installed, nil
}

func privateCASWindowsRelativeComponents(relative string) ([]string, error) {
	if relative == "" || relative == "." || filepath.IsAbs(relative) {
		return nil, errors.New("private CAS Windows relative binding is invalid")
	}
	cleaned := filepath.Clean(relative)
	if cleaned != relative || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return nil, errors.New("private CAS Windows relative binding is not canonical")
	}
	components := strings.FieldsFunc(cleaned, func(char rune) bool { return char == '\\' || char == '/' })
	if len(components) == 0 {
		return nil, errors.New("private CAS Windows relative binding is empty")
	}
	for _, component := range components {
		if !privateWindowsComponent(component) {
			return nil, errors.New("private CAS Windows relative component is invalid")
		}
	}
	return components, nil
}

func capturePrivateCASShardIdentities(authority privateCASRootAuthority) (map[string]privateCASShardIdentity, error) {
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	entries, err := privateCASWindowsReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return nil, err
	}
	pins := make(map[string]privateCASShardIdentity, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return nil, errors.New("private CAS inventory contains a non-canonical shard")
		}
		shard, err := privateWindowsOpenShard(root, entry.Name(), false)
		if err != nil {
			return nil, err
		}
		identity, identityErr := privateCASWindowsIdentity(shard)
		closeErr := windows.CloseHandle(shard)
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
	defer windows.CloseHandle(root)
	validateRoot := func() error {
		if handled, err := validateOriginalPrivateCASWriteRootV1(authority, pins, maxBytes, validations); handled {
			return err
		}
		return privateCASWindowsValidatePinnedRootStrictAt(root, pins)
	}
	if err := validateRoot(); err != nil {
		return err
	}
	shardName := digest[:2]
	shard, identity, shardCreated, err := privateCASWindowsOpenPinnedShardForCommit(root, pins, shardName, privateCASOriginalWriteHasTargetShardResidueV1(shardName, validations))
	if err != nil {
		return err
	}
	if shardCreatedOutput != nil {
		*shardCreatedOutput = shardCreated
	}
	defer windows.CloseHandle(shard)
	if err := privateCASOriginalBeforeRecordStageV1(validations); err != nil {
		return err
	}
	name := digest + ".json"
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"
	handle, err := privateWindowsOpenRelative(shard, temporary, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("private CAS Windows temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = privateWindowsDeleteRelative(shard, temporary, false)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("private CAS Windows write was incomplete")
		}
		written += count
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("private CAS Windows staged write verification failed")
	}
	stagedIdentity, err := privateWindowsValidateAuthorityObject(handle, false, uint64(maxBytes))
	if err != nil {
		return err
	}
	if beforeCommit != nil {
		beforeCommit()
	}
	if err := privateWindowsRenameRelativeNoReplace(handle, shard, name); err != nil {
		if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
			return os.ErrExist
		}
		return err
	}
	tempExists = false
	if err := file.Sync(); err != nil {
		return err
	}
	if err := privateWindowsSyncDirectory(shard); err != nil {
		return err
	}
	if beforeReadback != nil {
		beforeReadback()
	}
	written, committedIdentity, err := privateCASWindowsReadStableAt(shard, name, maxBytes)
	if err != nil || !bytes.Equal(written, body) || !privateWindowsSameObject(stagedIdentity, committedIdentity) {
		return errors.New("private CAS Windows committed write verification failed")
	}
	if err := privateCASWindowsVerifyCurrentShard(authority, shardName, identity); err != nil {
		return err
	}
	if err := validateRoot(); err != nil {
		return err
	}
	if additionOutput != nil {
		rootMetadata, err := privateCASWindowsSnapshotMetadata(root)
		if err != nil {
			return err
		}
		shardMetadata, err := privateCASWindowsSnapshotMetadata(shard)
		if err != nil {
			return err
		}
		recordMetadata, err := privateCASWindowsSnapshotMetadata(handle)
		if err != nil {
			return err
		}
		current, err := privateCASWindowsRecordIdentityAt(shard, name, maxBytes)
		if err != nil || current != stagedIdentity {
			return errors.Join(errors.New("private CAS Windows committed addition path no longer names the staged object"), err)
		}
		*additionOutput = privateCASAdditionObservation{
			body:                 written,
			rootIdentityDigest:   privateCASWindowsObjectIdentityDigest("root", authority.id),
			shardIdentityDigest:  privateCASWindowsObjectIdentityDigest("shard", identity.id),
			recordIdentityDigest: privateCASWindowsObjectIdentityDigest("record", stagedIdentity.id),
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
	defer windows.CloseHandle(root)
	if err := privateCASWindowsValidatePinnedRootStrictAt(root, pins); err != nil {
		return nil, err
	}
	shardName := digest[:2]
	shard, identity, err := privateCASWindowsOpenPinnedShard(root, pins, shardName, false)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(shard)
	body, err := privateCASWindowsReadAt(shard, digest+".json", maxBytes)
	if err != nil {
		return nil, err
	}
	if err := privateCASWindowsVerifyCurrentShard(authority, shardName, identity); err != nil {
		return nil, err
	}
	if err := privateCASWindowsValidatePinnedRootStrictAt(root, pins); err != nil {
		return nil, err
	}
	return body, nil
}

func privateCASWindowsValidatePinnedRootAt(root windows.Handle, pins map[string]privateCASShardIdentity) error {
	entries, err := privateCASWindowsReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !validPrivateShard(name) {
			return errors.New("private CAS Windows root inventory contains a non-canonical shard")
		}
		shard, err := privateWindowsOpenShard(root, name, false)
		if err != nil {
			return err
		}
		identity, identityErr := privateCASWindowsIdentity(shard)
		closeErr := windows.CloseHandle(shard)
		expected, pinned := pins[name]
		if identityErr != nil || closeErr != nil || pinned && identity != expected {
			return errors.Join(identityErr, closeErr, errors.New("private CAS Windows pinned shard identity changed"))
		}
		if !pinned {
			pins[name] = identity
		}
		seen[name] = struct{}{}
	}
	for name := range pins {
		if _, ok := seen[name]; !ok {
			return errors.New("private CAS Windows pinned shard disappeared")
		}
	}
	return nil
}

func privateCASWindowsValidatePinnedRootStrictAt(root windows.Handle, pins map[string]privateCASShardIdentity) error {
	entries, err := privateCASWindowsReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !validPrivateShard(name) {
			return errors.New("private CAS Windows strict root inventory contains a non-canonical shard")
		}
		expected, pinned := pins[name]
		if !pinned {
			return errors.New("private CAS Windows strict root inventory contains an unprepared shard")
		}
		shard, err := privateWindowsOpenShard(root, name, false)
		if err != nil {
			return err
		}
		identity, identityErr := privateCASWindowsIdentity(shard)
		closeErr := windows.CloseHandle(shard)
		if identityErr != nil || closeErr != nil || identity != expected {
			return errors.Join(identityErr, closeErr, errors.New("private CAS Windows strict pinned shard identity changed"))
		}
		seen[name] = struct{}{}
	}
	for name := range pins {
		if _, found := seen[name]; !found {
			return errors.New("private CAS Windows strict pinned shard disappeared")
		}
	}
	return nil
}

func securePrivateCASList(ctx context.Context, authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, maxBytes int) ([]SecurePrivateCASFile, error) {
	return privateCASWindowsScanWithBounds(
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
	defer windows.CloseHandle(root)
	entries, err := privateCASWindowsReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return errors.New("private CAS Windows inventory contains a non-canonical shard")
		}
		name := entry.Name()
		names = append(names, name)
		shard, identity, err := privateCASWindowsOpenPinnedShard(root, pins, name, false)
		if err != nil {
			return err
		}
		baseline, _, baselineErr := privateCASWindowsScanShard(ctx, shard, name, maxBytes, false, 0, 0, false)
		if baselineErr != nil || baseline.records == 0 {
			_ = windows.CloseHandle(shard)
			return errors.Join(baselineErr, errors.New("private CAS Windows inventory contains an unreadable or empty shard"))
		}
		visited, visitErr := privateCASWindowsVisitShard(ctx, shard, name, maxBytes, visit)
		if visitErr != nil {
			_ = windows.CloseHandle(shard)
			return visitErr
		}
		verified, _, verifyErr := privateCASWindowsScanShard(ctx, shard, name, maxBytes, false, 0, 0, false)
		if visited != baseline || verifyErr != nil || verified != baseline {
			_ = windows.CloseHandle(shard)
			return errors.Join(verifyErr, errors.New("private CAS Windows shard inventory changed during visit"))
		}
		if err := privateCASWindowsVerifyCurrentShard(authority, name, identity); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		if err := windows.CloseHandle(shard); err != nil {
			return err
		}
	}
	sort.Strings(names)
	for name := range pins {
		index := sort.SearchStrings(names, name)
		if index >= len(names) || names[index] != name {
			return errors.New("private CAS Windows pinned shard disappeared")
		}
	}
	current, err := privateCASWindowsReadDirBounded(root, len(names))
	if err != nil || !privateCASWindowsSameCanonicalNames(names, current) {
		return errors.New("private CAS Windows inventory changed during visit")
	}
	return nil
}

func securePrivateCASValidateInventory(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, maxBytes int) error {
	_, err := privateCASWindowsScan(nil, authority, pins, maxBytes, false, 0, 0, false)
	return err
}

func privateCASWindowsScanWithBounds(
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
		return nil, errors.New("private CAS Windows inventory bounds are invalid")
	}
	return privateCASWindowsScan(
		ctx, authority, pins, maxBytes, collectBodies,
		uint64(maxRecords), uint64(maxAggregateBytes), true,
	)
}

func privateCASWindowsScan(
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
	defer windows.CloseHandle(root)
	entries, err := privateCASWindowsReadDirBounded(root, maxSecurePrivateCASShards)
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
		shard, identity, err := privateCASWindowsOpenPinnedShard(root, pins, name, false)
		if err != nil {
			return nil, err
		}
		remainingRecords, remainingBytes := uint64(0), uint64(0)
		if enforceMaterializationBounds {
			if totalRecords > maxRecords || totalBytes > maxAggregateBytes {
				_ = windows.CloseHandle(shard)
				return nil, errors.New("private CAS Windows materialization bound exceeded")
			}
			remainingRecords = maxRecords - totalRecords
			remainingBytes = maxAggregateBytes - totalBytes
		}
		fingerprint, shardFiles, scanErr := privateCASWindowsScanShard(
			ctx, shard, name, maxBytes, collectBodies,
			remainingRecords, remainingBytes, enforceMaterializationBounds,
		)
		if scanErr != nil || fingerprint.records == 0 {
			_ = windows.CloseHandle(shard)
			return nil, errors.Join(scanErr, errors.New("private CAS inventory contains an unreadable or empty shard"))
		}
		verified, _, verifyErr := privateCASWindowsScanShard(ctx, shard, name, maxBytes, false, 0, 0, false)
		if verifyErr != nil || verified != fingerprint {
			_ = windows.CloseHandle(shard)
			return nil, errors.Join(verifyErr, errors.New("private CAS Windows shard inventory changed during enumeration"))
		}
		if ^uint64(0)-totalRecords < fingerprint.records || ^uint64(0)-totalBytes < fingerprint.bytes {
			_ = windows.CloseHandle(shard)
			return nil, errors.New("private CAS Windows inventory counters overflowed")
		}
		totalRecords += fingerprint.records
		totalBytes += fingerprint.bytes
		files = append(files, shardFiles...)
		if err := privateCASWindowsVerifyCurrentShard(authority, name, identity); err != nil {
			_ = windows.CloseHandle(shard)
			return nil, err
		}
		if err := windows.CloseHandle(shard); err != nil {
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
	current, err := privateCASWindowsReadDirBounded(root, len(names))
	if err != nil || !privateCASWindowsSameCanonicalNames(names, current) {
		return nil, errors.New("private CAS inventory changed during enumeration")
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Digest < files[right].Digest })
	return files, nil
}

func privateCASWindowsScanShard(
	ctx context.Context,
	shard windows.Handle,
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
	err := privateCASWindowsWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		recordName := entry.Name()
		recordDigest := strings.TrimSuffix(recordName, ".json")
		if entry.IsDir() || filepath.Ext(recordName) != ".json" || !validPrivateDigest(recordDigest) || recordDigest[:2] != shardName {
			return errors.New("private CAS Windows inventory contains a non-canonical record")
		}
		if records == ^uint64(0) {
			return errors.New("private CAS Windows record counter overflowed")
		}
		if enforceMaterializationBounds && records >= maxRecords {
			return errors.Join(ErrSecurePrivateCASMaterializationLimit, errors.New("private CAS Windows record materialization bound exceeded"))
		}
		body, identity, err := privateCASWindowsReadStableAt(shard, recordName, maxBytes)
		if err != nil {
			return err
		}
		bodyBytes := uint64(len(body))
		if ^uint64(0)-totalBytes < bodyBytes {
			return errors.New("private CAS Windows byte counter overflowed")
		}
		if enforceMaterializationBounds && bodyBytes > maxAggregateBytes-totalBytes {
			return errors.Join(ErrSecurePrivateCASMaterializationLimit, errors.New("private CAS Windows aggregate byte materialization bound exceeded"))
		}
		records++
		totalBytes += bodyBytes
		privateCASWindowsFingerprintRecord(digest, recordName, body, identity)
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

func privateCASWindowsVisitShard(
	ctx context.Context,
	shard windows.Handle,
	shardName string,
	maxBytes int,
	visit func(SecurePrivateCASFile) error,
) (privateCASInventoryFingerprint, error) {
	digest := sha256.New()
	privateCASWriteFingerprintField(digest, []byte("analytix-private-cas-inventory-v1"))
	privateCASWriteFingerprintField(digest, []byte(shardName))
	var records uint64
	var totalBytes uint64
	err := privateCASWindowsWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		recordName := entry.Name()
		recordDigest := strings.TrimSuffix(recordName, ".json")
		if entry.IsDir() || filepath.Ext(recordName) != ".json" || !validPrivateDigest(recordDigest) || recordDigest[:2] != shardName {
			return errors.New("private CAS Windows inventory contains a non-canonical record")
		}
		if records == ^uint64(0) {
			return errors.New("private CAS Windows record counter overflowed")
		}
		body, identity, err := privateCASWindowsReadStableAt(shard, recordName, maxBytes)
		if err != nil {
			return err
		}
		bodyBytes := uint64(len(body))
		if ^uint64(0)-totalBytes < bodyBytes {
			return errors.New("private CAS Windows byte counter overflowed")
		}
		records++
		totalBytes += bodyBytes
		privateCASWindowsFingerprintRecord(digest, recordName, body, identity)
		return visit(SecurePrivateCASFile{Digest: recordDigest, Body: append([]byte(nil), body...)})
	})
	if err != nil {
		return privateCASInventoryFingerprint{}, err
	}
	var sum [32]byte
	copy(sum[:], digest.Sum(nil))
	return privateCASInventoryFingerprint{digest: sum, records: records, bytes: totalBytes}, nil
}

func privateCASWindowsFingerprintRecord(writer hash.Hash, name string, body []byte, identity privateWindowsObjectIdentity) {
	privateCASWindowsFingerprintRecordBodySHA256(writer, name, sha256.Sum256(body), identity)
}

func privateCASWindowsFingerprintRecordBodySHA256(
	writer hash.Hash,
	name string,
	bodySHA256 [32]byte,
	identity privateWindowsObjectIdentity,
) {
	privateCASWriteFingerprintField(writer, []byte(name))
	privateCASWriteFingerprintField(writer, bodySHA256[:])
	var metadata [64]byte
	binary.BigEndian.PutUint64(metadata[0:8], identity.id.VolumeSerialNumber)
	copy(metadata[8:24], identity.id.FileID[:])
	binary.BigEndian.PutUint32(metadata[24:28], identity.attributes)
	binary.BigEndian.PutUint32(metadata[28:32], identity.links)
	binary.BigEndian.PutUint32(metadata[32:36], identity.sizeHigh)
	binary.BigEndian.PutUint32(metadata[36:40], identity.sizeLow)
	binary.BigEndian.PutUint32(metadata[40:44], identity.creationTime.HighDateTime)
	binary.BigEndian.PutUint32(metadata[44:48], identity.creationTime.LowDateTime)
	binary.BigEndian.PutUint32(metadata[48:52], identity.lastWriteTime.HighDateTime)
	binary.BigEndian.PutUint32(metadata[52:56], identity.lastWriteTime.LowDateTime)
	privateCASWriteFingerprintField(writer, metadata[:56])
}

func privateCASWindowsWalkDir(parent windows.Handle, visit func(os.DirEntry) error) error {
	handle, err := privateWindowsReopenFile(
		parent,
		privateWindowsObserveDirectoryAccess,
		privateWindowsShare,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
	)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(handle), "private-cas-windows-streaming-directory")
	if directory == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("private CAS Windows streaming directory handle is invalid")
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
			walkErr = errors.New("private CAS Windows streaming directory enumeration made no progress")
			break
		}
	}
	return errors.Join(walkErr, directory.Close())
}

var errPrivateCASWindowsRecoveryBatchFull = errors.New("private CAS Windows recovery batch is full")

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
	defer windows.CloseHandle(root)
	entryBound := maxSecurePrivateCASShards
	if allowPreparedCreateResidues {
		entryBound *= 2
	}
	entries, err := privateCASWindowsReadDirBounded(root, entryBound)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		if allowPreparedCreateResidues && entry.IsDir() &&
			domainprivatecas.CreateDirectoryResidueMatchesShardV1(entry.Name()) {
			if err := privateCASWindowsValidatePreparedShardCreateResidueV4(
				root, entry.Name(), authority.id.VolumeSerialNumber,
			); err != nil {
				return err
			}
			continue
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return errors.New("private CAS Windows recovery contains a non-canonical shard")
		}
		shard, err := privateWindowsOpenShard(root, entry.Name(), false)
		if err != nil {
			return err
		}
		preflightErr := privateCASWindowsPreflightRecoveryShard(
			ctx, shard, entry.Name(), maxBytes, rejectTransactionPhases,
		)
		if closeErr := windows.CloseHandle(shard); preflightErr != nil || closeErr != nil {
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
	first, err := privateCASWindowsObserveRecoveryOnce(ctx, authority, maxBytes)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	second, err := privateCASWindowsObserveRecoveryOnce(ctx, authority, maxBytes)
	if err != nil || first.fingerprint != second.fingerprint {
		return privateCASRecoveryObservation{}, errors.Join(errors.New("private CAS Windows recovery inventory changed during observation"), err)
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
	first, err := privateCASWindowsObserveRecoveryOnce(ctx, authority, maxBytes, true)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	second, err := privateCASWindowsObserveRecoveryOnce(ctx, authority, maxBytes, true)
	if err != nil || first.fingerprint != second.fingerprint {
		return privateCASRecoveryObservation{}, errors.Join(errors.New("private CAS Windows recovery inventory changed during observation"), err)
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
		return nil, errors.New("private CAS Windows prepared recovery record digest is invalid")
	}
	shardName := digest[:2]
	shardIndex := sort.Search(len(expected.plan.shards), func(index int) bool {
		return expected.plan.shards[index].name >= shardName
	})
	if shardIndex >= len(expected.plan.shards) || expected.plan.shards[shardIndex].name != shardName {
		return nil, errors.New("private CAS Windows prepared recovery record is outside the frozen inventory")
	}
	preparedShard := expected.plan.shards[shardIndex]
	recordIndex := sort.Search(len(preparedShard.committed), func(index int) bool {
		return preparedShard.committed[index].digest >= digest
	})
	if recordIndex >= len(preparedShard.committed) || preparedShard.committed[recordIndex].digest != digest {
		return nil, errors.New("private CAS Windows prepared recovery record is outside the frozen inventory")
	}
	preparedRecord := preparedShard.committed[recordIndex]
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	shard, err := privateWindowsOpenShard(root, shardName, false)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(shard)
	currentShard, err := privateWindowsValidateAuthorityObject(shard, true, 0)
	if err != nil || currentShard != preparedShard.identity {
		return nil, errors.Join(errors.New("private CAS Windows prepared recovery shard identity changed"), err)
	}
	body, identity, bodySHA256, byteLength, err := privateCASWindowsReadRecoveryBodyStableAt(
		ctx, shard, preparedRecord.name, maxBytes, false,
	)
	if err != nil {
		return nil, err
	}
	if identity != preparedRecord.identity || bodySHA256 != preparedRecord.bodySHA256 ||
		byteLength != preparedRecord.byteLength {
		return nil, errors.New("private CAS Windows prepared recovery record identity or body changed")
	}
	currentShard, err = privateWindowsValidateAuthorityObject(shard, true, 0)
	if err != nil || currentShard != preparedShard.identity {
		return nil, errors.Join(errors.New("private CAS Windows prepared recovery shard changed during read"), err)
	}
	if err := privateCASWindowsVerifyCurrentShard(
		authority,
		shardName,
		privateCASShardIdentity{id: preparedShard.identity.id},
	); err != nil {
		return nil, err
	}
	return body, nil
}

func privateCASWindowsObserveRecoveryOnce(
	ctx context.Context,
	authority privateCASRootAuthority,
	maxBytes int,
	allowOriginalCreates ...bool,
) (privateCASRecoveryObservation, error) {
	root, err := authority.open()
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	defer windows.CloseHandle(root)
	rootIdentity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	allowCreates := len(allowOriginalCreates) == 1 && allowOriginalCreates[0]
	entryBound := maxSecurePrivateCASShards
	if allowCreates {
		entryBound *= 2
	}
	entries, err := privateCASWindowsReadDirBounded(root, entryBound)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	fingerprint := sha256.New()
	privateCASWriteFingerprintField(fingerprint, []byte("analytix-private-cas-recovery-plan-v1"))
	privateCASWindowsFingerprintRecord(fingerprint, "root", nil, rootIdentity)
	plan := privateCASPreparedRecoveryPlan{rootIdentity: rootIdentity, shards: make([]privateCASPreparedRecoveryShard, 0, len(entries))}
	committedMaterials := make([]SecurePrivateCASPreparedMaterialV1, 0)
	var recordCount uint64
	var aggregateBytes uint64
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return privateCASRecoveryObservation{}, err
		}
		shardName := entry.Name()
		if allowCreates && entry.IsDir() && domainprivatecas.CreateDirectoryResidueMatchesShardV1(shardName) {
			residue, err := privateCASWindowsOpenObservedCreateRecoveryDirectory(root, shardName, authority.id.VolumeSerialNumber)
			if err != nil {
				return privateCASRecoveryObservation{}, err
			}
			identity, identityErr := privateCASWindowsCreateRecoveryExactIdentity(residue, authority.id.VolumeSerialNumber)
			empty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(residue)
			closeErr := windows.CloseHandle(residue)
			if identityErr != nil || emptyErr != nil || !empty || closeErr != nil {
				return privateCASRecoveryObservation{}, errors.Join(errors.New("original CAS creation residue changed"), identityErr, emptyErr, closeErr)
			}
			privateCASWindowsFingerprintRecord(fingerprint, "original-create:"+shardName, nil, identity.object)
			plan.originalCreateResidues = append(plan.originalCreateResidues, privateCASCreateResidueWindowsCandidate{name: shardName, identity: identity})
			continue
		}
		if !entry.IsDir() || !validPrivateShard(shardName) {
			return privateCASRecoveryObservation{}, errors.New("private CAS Windows recovery contains a non-canonical shard")
		}
		shard, err := privateWindowsOpenShard(root, shardName, false)
		if err != nil {
			return privateCASRecoveryObservation{}, err
		}
		shardIdentity, identityErr := privateWindowsValidateAuthorityObject(shard, true, 0)
		if identityErr != nil || shardIdentity.id.VolumeSerialNumber != authority.id.VolumeSerialNumber {
			_ = windows.CloseHandle(shard)
			return privateCASRecoveryObservation{}, errors.New("private CAS Windows recovery shard identity is unsafe")
		}
		privateCASWindowsFingerprintRecord(fingerprint, "shard:"+shardName, nil, shardIdentity)
		shardEntries, scanErr := privateCASWindowsReadDirBounded(shard, maxSecurePrivateCASListRecords)
		if scanErr != nil {
			_ = windows.CloseHandle(shard)
			return privateCASRecoveryObservation{}, scanErr
		}
		sort.Slice(shardEntries, func(i, j int) bool { return shardEntries[i].Name() < shardEntries[j].Name() })
		preparedShard := privateCASPreparedRecoveryShard{
			name: shardName, identity: shardIdentity,
			entryNames: make([]string, 0, len(shardEntries)), committedNames: make([]string, 0, len(shardEntries)),
			committed: make([]privateCASPreparedRecoveryRecord, 0, len(shardEntries)),
			temps:     make([]privateCASPreparedRecoveryTemp, 0),
		}
		seenFolded := map[string]struct{}{}
		for _, shardEntry := range shardEntries {
			if err := privateCASContextError(ctx); err != nil {
				_ = windows.CloseHandle(shard)
				return privateCASRecoveryObservation{}, err
			}
			if recordCount >= maxSecurePrivateCASListRecords {
				_ = windows.CloseHandle(shard)
				return privateCASRecoveryObservation{}, ErrSecurePrivateCASMaterializationLimit
			}
			recordCount++
			name := shardEntry.Name()
			preparedShard.entryNames = append(preparedShard.entryNames, name)
			folded := strings.ToLower(name)
			if _, duplicate := seenFolded[folded]; duplicate {
				_ = windows.CloseHandle(shard)
				return privateCASRecoveryObservation{}, errors.New("private CAS Windows recovery aliases an entry by case")
			}
			seenFolded[folded] = struct{}{}
			kind := "temp"
			allowEmpty := true
			recordDigest := ""
			originalName, transactionID, phase := name, "", privateCASRecoveryQuarantinePlain
			if filepath.Ext(name) == ".json" {
				digest := strings.TrimSuffix(name, ".json")
				if shardEntry.IsDir() || !validPrivateDigest(digest) || digest[:2] != shardName {
					_ = windows.CloseHandle(shard)
					return privateCASRecoveryObservation{}, errors.New("private CAS Windows recovery contains a non-canonical record")
				}
				kind, allowEmpty = "record", false
				recordDigest = digest
				preparedShard.committedNames = append(preparedShard.committedNames, name)
			} else {
				var valid bool
				originalName, transactionID, phase, valid = privateCASRecoveryTempName(name, shardName)
				if shardEntry.IsDir() || !valid {
					_ = windows.CloseHandle(shard)
					return privateCASRecoveryObservation{}, errors.New("private CAS Windows recovery contains unknown residue")
				}
			}
			identity, bodySHA256, byteLength, readErr := privateCASWindowsObserveRecoveryStableAt(
				ctx, shard, name, maxBytes, allowEmpty,
			)
			if readErr != nil {
				_ = windows.CloseHandle(shard)
				return privateCASRecoveryObservation{}, readErr
			}
			if ^uint64(0)-aggregateBytes < byteLength ||
				aggregateBytes+byteLength > maxSecurePrivateCASListAggregateBytes {
				_ = windows.CloseHandle(shard)
				return privateCASRecoveryObservation{}, ErrSecurePrivateCASMaterializationLimit
			}
			aggregateBytes += byteLength
			privateCASWindowsFingerprintRecordBodySHA256(
				fingerprint, kind+":"+shardName+":"+name, bodySHA256, identity,
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
				linked, err := privateCASWindowsValidateRecoveryTemp(shard, shardName, name, originalName, maxBytes)
				if err != nil {
					_ = windows.CloseHandle(shard)
					return privateCASRecoveryObservation{}, err
				}
				preparedShard.temps = append(preparedShard.temps, privateCASPreparedRecoveryTemp{
					name: name, originalName: originalName, transactionID: transactionID, phase: phase,
					identity: identity, bodySHA256: bodySHA256, linked: linked,
				})
			}
		}
		plan.shards = append(plan.shards, preparedShard)
		if err := windows.CloseHandle(shard); err != nil {
			return privateCASRecoveryObservation{}, err
		}
	}
	current, err := privateCASWindowsReadDirBounded(root, len(entries))
	namesMatch := privateCASWindowsSameCanonicalNames(privateCASWindowsEntryNames(entries), current)
	if allowCreates {
		namesMatch = privateCASOriginalRecoveryRootNamesEqualV1(privateCASWindowsEntryNames(entries), current)
	}
	if err != nil || !namesMatch {
		return privateCASRecoveryObservation{}, errors.Join(errors.New("private CAS Windows recovery root inventory changed"), err)
	}
	var sum [32]byte
	copy(sum[:], fingerprint.Sum(nil))
	return privateCASRecoveryObservation{
		fingerprint: sum, committedMaterials: committedMaterials, plan: plan,
	}, nil
}

func privateCASWindowsValidatePreparedShardCreateResidueV4(
	root windows.Handle,
	name string,
	volume uint64,
) error {
	residue, err := privateCASWindowsOpenObservedCreateRecoveryDirectory(root, name, volume)
	if err != nil {
		return errors.Join(errors.New("private CAS Windows recovery create residue is unsafe"), err)
	}
	_, identityErr := privateCASWindowsCreateRecoveryExactIdentity(residue, volume)
	empty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(residue)
	closeErr := windows.CloseHandle(residue)
	if identityErr != nil || emptyErr != nil || !empty || closeErr != nil {
		return errors.Join(
			errors.New("private CAS Windows recovery create residue is non-empty or unsafe"),
			identityErr,
			emptyErr,
			closeErr,
		)
	}
	return nil
}

func privateCASWindowsEntryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	sort.Strings(names)
	return names
}

type privateCASWindowsOpenRecoveryFile struct {
	file       *os.File
	identity   privateWindowsObjectIdentity
	maxBytes   int
	allowEmpty bool
}

func privateCASWindowsOpenRecoveryFileAt(
	parent windows.Handle,
	name string,
	maxBytes int,
	allowEmpty bool,
) (*privateCASWindowsOpenRecoveryFile, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private CAS Windows recovery file handle is invalid")
	}
	identity, err := privateWindowsValidateAuthorityObjectWithOptions(
		handle, false, uint64(maxBytes), allowEmpty, true,
	)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &privateCASWindowsOpenRecoveryFile{
		file: file, identity: identity, maxBytes: maxBytes, allowEmpty: allowEmpty,
	}, nil
}

func (opened *privateCASWindowsOpenRecoveryFile) close() error {
	if opened == nil || opened.file == nil {
		return nil
	}
	return opened.file.Close()
}

func (opened *privateCASWindowsOpenRecoveryFile) byteLength() uint64 {
	if opened == nil {
		return 0
	}
	return uint64(opened.identity.sizeHigh)<<32 | uint64(opened.identity.sizeLow)
}

func (opened *privateCASWindowsOpenRecoveryFile) revalidate() error {
	if opened == nil || opened.file == nil {
		return errors.New("private CAS Windows recovery file handle is unavailable")
	}
	current, err := privateWindowsValidateAuthorityObjectWithOptions(
		windows.Handle(opened.file.Fd()), false, uint64(opened.maxBytes), opened.allowEmpty, true,
	)
	if err != nil || current != opened.identity {
		return errors.Join(errors.New("private CAS Windows recovery file changed during read"), err)
	}
	return nil
}

func (opened *privateCASWindowsOpenRecoveryFile) hash(ctx context.Context) ([32]byte, error) {
	if opened == nil || opened.file == nil {
		return [32]byte{}, errors.New("private CAS Windows recovery file handle is unavailable")
	}
	digest, err := privateCASHashExact(ctx, opened.file, int64(opened.byteLength()))
	if err != nil {
		return [32]byte{}, err
	}
	if err := opened.revalidate(); err != nil {
		return [32]byte{}, err
	}
	return digest, nil
}

func (opened *privateCASWindowsOpenRecoveryFile) body(ctx context.Context) ([]byte, [32]byte, error) {
	if opened == nil || opened.file == nil {
		return nil, [32]byte{}, errors.New("private CAS Windows recovery file handle is unavailable")
	}
	body, digest, err := privateCASReadBodyExact(ctx, opened.file, int64(opened.byteLength()))
	if err != nil {
		return nil, [32]byte{}, err
	}
	if err := opened.revalidate(); err != nil {
		return nil, [32]byte{}, err
	}
	return body, digest, nil
}

func privateCASWindowsObserveRecoveryStableAt(
	ctx context.Context,
	parent windows.Handle,
	name string,
	maxBytes int,
	allowEmpty bool,
) (privateWindowsObjectIdentity, [32]byte, uint64, error) {
	first, err := privateCASWindowsOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	defer first.close()
	firstSHA256, err := first.hash(ctx)
	if err != nil {
		return privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	second, err := privateCASWindowsOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	defer second.close()
	secondSHA256, err := second.hash(ctx)
	if err != nil || first.identity != second.identity || firstSHA256 != secondSHA256 {
		return privateWindowsObjectIdentity{}, [32]byte{}, 0, errors.Join(
			errors.New("private CAS Windows recovery file name changed during stable observation"), err,
		)
	}
	if err := first.revalidate(); err != nil {
		return privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	return first.identity, firstSHA256, first.byteLength(), nil
}

func privateCASWindowsReadRecoveryBodyStableAt(
	ctx context.Context,
	parent windows.Handle,
	name string,
	maxBytes int,
	allowEmpty bool,
) ([]byte, privateWindowsObjectIdentity, [32]byte, uint64, error) {
	first, err := privateCASWindowsOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return nil, privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	defer first.close()
	body, firstSHA256, err := first.body(ctx)
	if err != nil {
		return nil, privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	second, err := privateCASWindowsOpenRecoveryFileAt(parent, name, maxBytes, allowEmpty)
	if err != nil {
		return nil, privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	defer second.close()
	secondSHA256, err := second.hash(ctx)
	if err != nil || first.identity != second.identity || firstSHA256 != secondSHA256 {
		return nil, privateWindowsObjectIdentity{}, [32]byte{}, 0, errors.Join(
			errors.New("private CAS Windows recovery file name changed during stable body read"), err,
		)
	}
	if err := first.revalidate(); err != nil {
		return nil, privateWindowsObjectIdentity{}, [32]byte{}, 0, err
	}
	return body, first.identity, firstSHA256, first.byteLength(), nil
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
		return false, errors.Join(errors.New("private CAS Windows recovery changed before marker creation"), err)
	}
	for _, preparedShard := range observation.plan.shards {
		if len(preparedShard.committedNames) != 0 || len(preparedShard.temps) != 0 {
			continue
		}
		root, err := authority.open()
		if err != nil {
			return false, err
		}
		shard, err := privateWindowsOpenShard(root, preparedShard.name, false)
		if err != nil {
			_ = windows.CloseHandle(root)
			return false, err
		}
		digest := preparedShard.name + transactionID[2:]
		name := "." + digest + ".json-" + transactionID[:24] + ".tmp"
		handle, createErr := privateWindowsOpenRelative(
			shard, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false,
		)
		if createErr == nil {
			_, identityErr := privateWindowsValidateAuthorityObjectWithOptions(handle, false, uint64(maxBytes), true, true)
			createErr = errors.Join(identityErr, windows.FlushFileBuffers(handle), windows.CloseHandle(handle), privateWindowsSyncDirectory(shard))
		}
		closeErr := errors.Join(windows.CloseHandle(shard), windows.CloseHandle(root))
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
		return errors.Join(errors.New("private CAS Windows recovery changed before quarantine staging"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		if len(preparedShard.temps) == 0 {
			continue
		}
		shard, err := privateWindowsOpenShard(root, preparedShard.name, false)
		if err != nil {
			return err
		}
		if err := privateCASWindowsRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		for tempIndex := range preparedShard.temps {
			temp := &preparedShard.temps[tempIndex]
			if temp.phase == privateCASRecoveryQuarantineCommitted {
				_ = windows.CloseHandle(shard)
				return errors.New("private CAS Windows recovery commit marker cannot be restaged")
			}
			if err := privateCASWindowsValidatePreparedRecoveryTemp(shard, preparedShard.name, *temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			target, ok := privateCASRecoveryQuarantineName(
				privateCASRecoveryQuarantineStaged, transactionID, temp.originalName, preparedShard.name,
			)
			if !ok {
				_ = windows.CloseHandle(shard)
				return errors.New("private CAS Windows recovery stage name is invalid")
			}
			if temp.name != target {
				if err := privateCASWindowsRenamePreparedTemp(shard, preparedShard.name, *temp, target, maxBytes); err != nil {
					_ = windows.CloseHandle(shard)
					return err
				}
				privateCASReplacePreparedRecoveryEntry(preparedShard.entryNames, temp.name, target)
				temp.name = target
				if err := privateCASWindowsRefreshPreparedRecoveryTemp(shard, temp, maxBytes); err != nil {
					_ = windows.CloseHandle(shard)
					return err
				}
			}
			temp.phase = privateCASRecoveryQuarantineStaged
			temp.transactionID = transactionID
		}
		if err := privateWindowsSyncDirectory(shard); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		if err := windows.CloseHandle(shard); err != nil {
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
	defer windows.CloseHandle(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		shard, err := privateWindowsOpenShard(root, preparedShard.name, false)
		if err != nil {
			return err
		}
		if err := privateCASWindowsRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		changed := false
		for tempIndex := range preparedShard.temps {
			temp := &preparedShard.temps[tempIndex]
			if temp.phase == privateCASRecoveryQuarantinePlain {
				continue
			}
			if err := privateCASWindowsValidatePreparedRecoveryTemp(shard, preparedShard.name, *temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			if err := privateCASWindowsRenamePreparedTemp(shard, preparedShard.name, *temp, temp.originalName, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			privateCASReplacePreparedRecoveryEntry(preparedShard.entryNames, temp.name, temp.originalName)
			temp.name = temp.originalName
			if err := privateCASWindowsRefreshPreparedRecoveryTemp(shard, temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			temp.phase = privateCASRecoveryQuarantinePlain
			temp.transactionID = ""
			changed = true
		}
		if changed {
			if err := privateWindowsSyncDirectory(shard); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
		}
		if err := windows.CloseHandle(shard); err != nil {
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
		return false, errors.Join(errors.New("private CAS Windows recovery changed before commit marker"), err)
	}
	root, err := authority.open()
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		for tempIndex := range preparedShard.temps {
			temp := &preparedShard.temps[tempIndex]
			if temp.phase != privateCASRecoveryQuarantineStaged || temp.transactionID != transactionID {
				continue
			}
			shard, err := privateWindowsOpenShard(root, preparedShard.name, false)
			if err != nil {
				return false, err
			}
			if err := privateCASWindowsRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
				_ = windows.CloseHandle(shard)
				return false, err
			}
			if err := privateCASWindowsValidatePreparedRecoveryTemp(shard, preparedShard.name, *temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return false, err
			}
			target, ok := privateCASRecoveryQuarantineName(
				privateCASRecoveryQuarantineCommitted, transactionID, temp.originalName, preparedShard.name,
			)
			if !ok {
				_ = windows.CloseHandle(shard)
				return false, errors.New("private CAS Windows recovery commit marker name is invalid")
			}
			markerRenamed, renameErr := privateCASWindowsRenamePreparedTempOutcome(shard, preparedShard.name, *temp, target, maxBytes)
			if renameErr != nil {
				_ = windows.CloseHandle(shard)
				return markerRenamed, renameErr
			}
			privateCASReplacePreparedRecoveryEntry(preparedShard.entryNames, temp.name, target)
			temp.name = target
			if err := privateCASRecoveryTransactionTestCut("after_commit_marker_rename", shardIndex); err != nil {
				_ = windows.CloseHandle(shard)
				return markerRenamed, err
			}
			if err := privateCASWindowsRefreshPreparedRecoveryTemp(shard, temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return markerRenamed, err
			}
			temp.phase = privateCASRecoveryQuarantineCommitted
			if err := privateWindowsSyncDirectory(shard); err != nil {
				_ = windows.CloseHandle(shard)
				return markerRenamed, err
			}
			if err := privateCASRecoveryTransactionTestCut("after_commit_marker_sync", shardIndex); err != nil {
				_ = windows.CloseHandle(shard)
				return markerRenamed, err
			}
			if err := windows.CloseHandle(shard); err != nil {
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
		return errors.Join(errors.New("private CAS Windows recovery changed before committed cleanup"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	for shardIndex := range observation.plan.shards {
		preparedShard := &observation.plan.shards[shardIndex]
		shard, err := privateWindowsOpenShard(root, preparedShard.name, false)
		if err != nil {
			return err
		}
		if err := privateCASWindowsRequireExactEntryNames(shard, preparedShard.entryNames); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		remaining := append([]string(nil), preparedShard.committedNames...)
		for _, temp := range preparedShard.temps {
			if temp.transactionID != transactionID ||
				(temp.phase != privateCASRecoveryQuarantineStaged && temp.phase != privateCASRecoveryQuarantineCommitted) {
				_ = windows.CloseHandle(shard)
				return errors.New("private CAS Windows committed cleanup contains an unrelated residue")
			}
			if preserveMarker && temp.phase == privateCASRecoveryQuarantineCommitted {
				remaining = append(remaining, temp.name)
				continue
			}
			if err := privateCASWindowsDeletePreparedTemp(shard, preparedShard.name, temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
		}
		if err := privateWindowsSyncDirectory(shard); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		if err := privateCASWindowsRequireExactEntryNames(shard, remaining); err != nil {
			_ = windows.CloseHandle(shard)
			return err
		}
		empty := len(remaining) == 0
		if empty {
			if err := privateWindowsVerifyRelativeDirectoryIdentity(root, preparedShard.name, shard); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			deleteErr := privateWindowsDeleteHandle(shard)
			closeErr := windows.CloseHandle(shard)
			if deleteErr != nil || closeErr != nil {
				return errors.Join(deleteErr, closeErr)
			}
		} else if err := windows.CloseHandle(shard); err != nil {
			return err
		}
	}
	if err := privateWindowsSyncDirectory(root); err != nil {
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
		return errors.New("private CAS Windows recovery commit marker plan is invalid")
	}
	current, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil || current.fingerprint != observation.fingerprint {
		return errors.Join(errors.New("private CAS Windows recovery changed before commit marker retirement"), err)
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	found := false
	for _, preparedShard := range observation.plan.shards {
		for _, temp := range preparedShard.temps {
			if temp.phase != privateCASRecoveryQuarantineCommitted || temp.transactionID != transactionID || found {
				continue
			}
			shard, err := privateWindowsOpenShard(root, preparedShard.name, false)
			if err != nil {
				return err
			}
			if err := privateCASWindowsRequireExactEntryNames(shard, append(append([]string(nil), preparedShard.committedNames...), temp.name)); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			if err := privateCASWindowsDeletePreparedTemp(shard, preparedShard.name, temp, maxBytes); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			if err := privateWindowsSyncDirectory(shard); err != nil {
				_ = windows.CloseHandle(shard)
				return err
			}
			empty := len(preparedShard.committedNames) == 0
			if empty {
				if err := privateWindowsVerifyRelativeDirectoryIdentity(root, preparedShard.name, shard); err != nil {
					_ = windows.CloseHandle(shard)
					return err
				}
				deleteErr := privateWindowsDeleteHandle(shard)
				closeErr := windows.CloseHandle(shard)
				if deleteErr != nil || closeErr != nil {
					return errors.Join(deleteErr, closeErr)
				}
			} else if err := windows.CloseHandle(shard); err != nil {
				return err
			}
			found = true
		}
	}
	if !found {
		return errors.New("private CAS Windows recovery commit marker was not found")
	}
	if err := privateWindowsSyncDirectory(root); err != nil {
		return err
	}
	refreshed, err := securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	if err != nil {
		return err
	}
	*observation = refreshed
	return nil
}

func privateCASWindowsValidatePreparedRecoveryTemp(
	shard windows.Handle,
	shardName string,
	temp privateCASPreparedRecoveryTemp,
	maxBytes int,
) error {
	linked, err := privateCASWindowsValidateRecoveryTemp(shard, shardName, temp.name, temp.originalName, maxBytes)
	if err != nil || linked != temp.linked {
		return errors.Join(errors.New("private CAS Windows prepared recovery temp topology changed"), err)
	}
	identity, bodySHA256, _, err := privateCASWindowsObserveRecoveryStableAt(
		context.Background(), shard, temp.name, maxBytes, true,
	)
	if err != nil || identity != temp.identity || bodySHA256 != temp.bodySHA256 {
		return errors.Join(errors.New("private CAS Windows prepared recovery temp identity changed"), err)
	}
	return nil
}

func privateCASWindowsRefreshPreparedRecoveryTemp(
	shard windows.Handle,
	temp *privateCASPreparedRecoveryTemp,
	maxBytes int,
) error {
	if temp == nil {
		return errors.New("private CAS Windows prepared recovery temp is unavailable")
	}
	identity, bodySHA256, _, err := privateCASWindowsObserveRecoveryStableAt(
		context.Background(), shard, temp.name, maxBytes, true,
	)
	if err != nil || identity.id != temp.identity.id || identity.attributes != temp.identity.attributes ||
		identity.links != temp.identity.links || identity.sizeHigh != temp.identity.sizeHigh ||
		identity.sizeLow != temp.identity.sizeLow || bodySHA256 != temp.bodySHA256 {
		return errors.Join(errors.New("private CAS Windows prepared recovery rename changed the residue object"), err)
	}
	temp.identity = identity
	return nil
}

func privateCASWindowsRenamePreparedTemp(
	shard windows.Handle,
	shardName string,
	temp privateCASPreparedRecoveryTemp,
	target string,
	maxBytes int,
) error {
	_, err := privateCASWindowsRenamePreparedTempOutcome(shard, shardName, temp, target, maxBytes)
	return err
}

func privateCASWindowsRenamePreparedTempOutcome(
	shard windows.Handle,
	shardName string,
	temp privateCASPreparedRecoveryTemp,
	target string,
	maxBytes int,
) (bool, error) {
	if err := privateCASWindowsValidatePreparedRecoveryTemp(shard, shardName, temp, maxBytes); err != nil {
		return false, err
	}
	handle, err := privateWindowsOpenRelative(shard, temp.name, windows.FILE_GENERIC_READ|windows.DELETE, windows.FILE_OPEN, false)
	if err != nil {
		return false, err
	}
	identity, identityErr := privateWindowsValidateAuthorityObjectWithOptions(handle, false, uint64(maxBytes), true, true)
	if identityErr != nil || identity != temp.identity {
		_ = windows.CloseHandle(handle)
		return false, errors.Join(errors.New("private CAS Windows recovery rename handle changed"), identityErr)
	}
	renameErr := privateWindowsRenameRelativeNoReplace(handle, shard, target)
	closeErr := windows.CloseHandle(handle)
	if renameErr != nil {
		return false, errors.Join(renameErr, closeErr)
	}
	return true, closeErr
}

func privateCASWindowsDeletePreparedTemp(
	shard windows.Handle,
	shardName string,
	temp privateCASPreparedRecoveryTemp,
	maxBytes int,
) error {
	if err := privateCASWindowsValidatePreparedRecoveryTemp(shard, shardName, temp, maxBytes); err != nil {
		return err
	}
	handle, err := privateWindowsOpenRelative(shard, temp.name, windows.FILE_GENERIC_READ|windows.DELETE, windows.FILE_OPEN, false)
	if err != nil {
		return err
	}
	identity, identityErr := privateWindowsValidateAuthorityObjectWithOptions(handle, false, uint64(maxBytes), true, true)
	if identityErr != nil || identity != temp.identity {
		_ = windows.CloseHandle(handle)
		return errors.Join(errors.New("private CAS Windows recovery delete handle changed"), identityErr)
	}
	deleteErr := privateWindowsDeleteHandle(handle)
	closeErr := windows.CloseHandle(handle)
	return errors.Join(deleteErr, closeErr)
}

func privateCASWindowsRequireExactEntryNames(parent windows.Handle, expected []string) error {
	actualEntries, err := privateCASWindowsReadDirBounded(parent, len(expected))
	if err != nil {
		return err
	}
	actual := make([]string, len(actualEntries))
	seenFolded := make(map[string]struct{}, len(actualEntries))
	for index, entry := range actualEntries {
		actual[index] = entry.Name()
		folded := strings.ToLower(actual[index])
		if _, duplicate := seenFolded[folded]; duplicate {
			return errors.New("private CAS Windows prepared recovery aliases an entry by case")
		}
		seenFolded[folded] = struct{}{}
	}
	wanted := append([]string(nil), expected...)
	sort.Strings(actual)
	sort.Strings(wanted)
	if len(actual) != len(wanted) || strings.Join(actual, "\x00") != strings.Join(wanted, "\x00") {
		return errors.New("private CAS Windows prepared recovery inventory no longer matches its exact plan")
	}
	return nil
}

func privateCASWindowsPreflightRecoveryShard(
	ctx context.Context,
	shard windows.Handle,
	shardName string,
	maxBytes int,
	rejectTransactionPhases bool,
) error {
	var linkedRecords uint64
	var linkedTemps uint64
	err := privateCASWindowsWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			digest := strings.TrimSuffix(name, ".json")
			if entry.IsDir() || !validPrivateDigest(digest) || digest[:2] != shardName {
				return errors.New("private CAS Windows recovery contains a non-canonical record")
			}
			identity, err := privateCASWindowsRecoveryIdentityAt(shard, name, false, maxBytes)
			if err != nil {
				return errors.New("private CAS Windows recovery record is unsafe")
			}
			if identity.links == 2 {
				linkedRecords++
			}
			return nil
		}
		originalName, _, phase, valid := privateCASRecoveryTempName(name, shardName)
		if !valid {
			return errors.New("private CAS Windows recovery contains unknown residue")
		}
		if rejectTransactionPhases && phase != privateCASRecoveryQuarantinePlain {
			return errors.New("private CAS Windows staged or committed recovery residue has no signed journal")
		}
		linked, err := privateCASWindowsValidateRecoveryTemp(shard, shardName, name, originalName, maxBytes)
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
		return errors.New("private CAS Windows recovery contains an untracked record hardlink")
	}
	return nil
}

func privateCASWindowsValidateRecoveredShard(ctx context.Context, shard windows.Handle, shardName string, maxBytes int) (bool, error) {
	var records uint64
	err := privateCASWindowsWalkDir(shard, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		name := entry.Name()
		digest := strings.TrimSuffix(name, ".json")
		if entry.IsDir() || filepath.Ext(name) != ".json" || !validPrivateDigest(digest) || digest[:2] != shardName {
			return errors.New("private CAS Windows recovery did not converge to canonical records")
		}
		if _, err := privateCASWindowsRecordIdentityAt(shard, name, maxBytes); err != nil {
			return errors.New("private CAS Windows recovered record is unsafe")
		}
		records++
		return nil
	})
	return records == 0, err
}

func privateCASWindowsRecoveryIdentityAt(parent windows.Handle, name string, allowEmpty bool, maxBytes int) (privateWindowsObjectIdentity, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if privateWindowsNotFound(err) {
		return privateWindowsObjectIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return privateWindowsObjectIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	return privateWindowsValidateAuthorityObjectWithOptions(
		handle, false, uint64(maxBytes), allowEmpty, true,
	)
}

func privateCASWindowsValidateRecoveryTemp(shard windows.Handle, shardName, name, originalName string, maxBytes int) (bool, error) {
	derived, _, _, valid := privateCASRecoveryTempName(name, shardName)
	if !valid || derived != originalName || !privateWriteTempName(originalName, shardName) {
		return false, errors.New("private CAS Windows recovery temp name is invalid")
	}
	tempIdentity, err := privateCASWindowsRecoveryIdentityAt(shard, name, true, maxBytes)
	if err != nil {
		return false, errors.Join(err, errors.New("private CAS Windows recovery temp is unsafe"))
	}
	marker := strings.Index(originalName, ".json-")
	finalName := strings.TrimPrefix(originalName[:marker+len(".json")], ".")
	finalIdentity, finalErr := privateCASWindowsRecoveryIdentityAt(shard, finalName, false, maxBytes)
	if finalErr == nil {
		sameFile := tempIdentity.id == finalIdentity.id
		if sameFile && (tempIdentity.links != 2 || finalIdentity.links != 2) ||
			!sameFile && (tempIdentity.links != 1 || finalIdentity.links != 1) {
			return false, errors.New("private CAS Windows recovery temp has an unsafe link topology")
		}
		return sameFile, nil
	}
	if finalErr != nil && !privateWindowsNotFound(finalErr) {
		return false, finalErr
	}
	if privateWindowsNotFound(finalErr) && tempIdentity.links != 1 {
		return false, errors.New("private CAS Windows recovery orphan temp is not single-link")
	}
	return false, nil
}

func privateCASWindowsReadDirBounded(parent windows.Handle, limit int) ([]os.DirEntry, error) {
	if limit < 0 || limit >= int(^uint(0)>>1) {
		return nil, errors.New("private CAS Windows directory bound is invalid")
	}
	handle, err := privateWindowsReopenFile(
		parent,
		privateWindowsObserveDirectoryAccess,
		privateWindowsShare,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
	)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(handle), "private-cas-windows-bounded-directory")
	if directory == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private CAS Windows bounded directory handle is invalid")
	}
	entries, readErr := directory.ReadDir(limit + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = directory.Close()
		return nil, readErr
	}
	if len(entries) > limit {
		_ = directory.Close()
		return nil, errors.New("private CAS Windows directory entry bound exceeded")
	}
	if readErr == nil {
		extra, extraErr := directory.ReadDir(1)
		if len(extra) != 0 || !errors.Is(extraErr, io.EOF) {
			_ = directory.Close()
			return nil, errors.Join(errors.New("private CAS Windows directory entry bound exceeded"), extraErr)
		}
	}
	if closeErr := directory.Close(); closeErr != nil {
		return nil, closeErr
	}
	return entries, nil
}

func privateCASWindowsCanonicalRecordNames(shard string, entries []os.DirEntry) ([]string, error) {
	names := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		digest := strings.TrimSuffix(entry.Name(), ".json")
		folded := strings.ToUpper(entry.Name())
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || !validPrivateDigest(digest) || digest[:2] != shard {
			return nil, errors.New("private CAS inventory contains a non-canonical record")
		}
		if _, duplicate := seen[folded]; duplicate {
			return nil, errors.New("private CAS Windows inventory aliases a record by case")
		}
		seen[folded] = struct{}{}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func privateCASWindowsSameRecordNames(shard string, expected []string, entries []os.DirEntry) bool {
	actual, err := privateCASWindowsCanonicalRecordNames(shard, entries)
	return err == nil && len(expected) == len(actual) && strings.Join(expected, "\x00") == strings.Join(actual, "\x00")
}

func privateCASWindowsOpenPinnedShard(root windows.Handle, pins map[string]privateCASShardIdentity, shardName string, create bool) (windows.Handle, privateCASShardIdentity, error) {
	if !validPrivateShard(shardName) {
		return 0, privateCASShardIdentity{}, errors.New("private CAS Windows shard name is invalid")
	}
	rootIdentity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return 0, privateCASShardIdentity{}, err
	}
	shard, present, err := privateCASWindowsOpenExactBoundDirectory(
		root,
		shardName,
		rootIdentity.id.VolumeSerialNumber,
	)
	created := false
	if err == nil && !present && create {
		shard, err = privateCASWindowsCreateBoundDirectory(
			root,
			shardName,
			rootIdentity.id.VolumeSerialNumber,
		)
		created = err == nil
		present = created
	}
	if err == nil && !present {
		return 0, privateCASShardIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return 0, privateCASShardIdentity{}, err
	}
	identity, err := privateCASWindowsIdentity(shard)
	if err != nil || identity.id.VolumeSerialNumber != rootIdentity.id.VolumeSerialNumber {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, errors.Join(err, errors.New("private CAS Windows shard crossed its root volume"))
	}
	if created {
		if err := errors.Join(privateWindowsSyncDirectory(shard), privateWindowsSyncDirectory(root)); err != nil {
			_ = windows.CloseHandle(shard)
			return 0, privateCASShardIdentity{}, err
		}
	}
	if err := privateWindowsVerifyRelativeDirectoryIdentity(root, shardName, shard); err != nil {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, err
	}
	if pinned, ok := pins[shardName]; ok && pinned != identity {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, errors.New("private CAS shard identity changed")
	}
	if _, ok := pins[shardName]; !ok {
		pins[shardName] = identity
	}
	return shard, identity, nil
}

func privateCASWindowsOpenPinnedShardForCommit(
	root windows.Handle,
	pins map[string]privateCASShardIdentity,
	shardName string,
	preserveOriginalCreation ...bool,
) (windows.Handle, privateCASShardIdentity, bool, error) {
	if !validPrivateShard(shardName) {
		return 0, privateCASShardIdentity{}, false, errors.New("private CAS Windows shard name is invalid")
	}
	rootIdentity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return 0, privateCASShardIdentity{}, false, err
	}
	pinned, hadPin := pins[shardName]
	shard, present, err := privateCASWindowsOpenExactBoundDirectory(
		root,
		shardName,
		rootIdentity.id.VolumeSerialNumber,
	)
	created := false
	if err == nil && present && !hadPin {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, false, errors.New("private CAS Windows unprepared shard appeared before commit")
	}
	if err == nil && !present && hadPin {
		return 0, privateCASShardIdentity{}, false, errors.New("private CAS Windows pinned shard disappeared before commit")
	}
	if err == nil && !present {
		if len(preserveOriginalCreation) == 1 && preserveOriginalCreation[0] {
			shard, err = privateCASWindowsCreateIndependentOriginalShardV1(root, shardName, rootIdentity.id.VolumeSerialNumber)
		} else {
			shard, err = privateCASWindowsCreateBoundDirectory(root, shardName, rootIdentity.id.VolumeSerialNumber)
		}
		created = err == nil
		present = created
	}
	if err != nil {
		return 0, privateCASShardIdentity{}, false, err
	}
	identity, err := privateCASWindowsIdentity(shard)
	if err != nil || identity.id.VolumeSerialNumber != rootIdentity.id.VolumeSerialNumber {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, false, errors.Join(err, errors.New("private CAS Windows shard crossed its root volume"))
	}
	if created {
		if err := errors.Join(privateWindowsSyncDirectory(shard), privateWindowsSyncDirectory(root)); err != nil {
			_ = windows.CloseHandle(shard)
			return 0, privateCASShardIdentity{}, false, err
		}
	}
	if err := privateWindowsVerifyRelativeDirectoryIdentity(root, shardName, shard); err != nil {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, false, err
	}
	if hadPin && pinned != identity {
		_ = windows.CloseHandle(shard)
		return 0, privateCASShardIdentity{}, false, errors.New("private CAS shard identity changed")
	}
	if _, ok := pins[shardName]; !ok {
		pins[shardName] = identity
	}
	return shard, identity, created, nil
}

func privateCASWindowsIdentity(handle windows.Handle) (privateCASShardIdentity, error) {
	identity, err := privateWindowsValidateAuthorityObject(handle, true, 0)
	if err != nil {
		return privateCASShardIdentity{}, errors.New("private CAS Windows shard is unsafe")
	}
	return privateCASShardIdentity{id: identity.id}, nil
}

func privateCASWindowsVerifyCurrentShard(authority privateCASRootAuthority, shardName string, expected privateCASShardIdentity) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	shard, err := privateWindowsOpenShard(root, shardName, false)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(shard)
	current, err := privateCASWindowsIdentity(shard)
	if err != nil || current != expected {
		return errors.New("private CAS Windows shard path identity changed")
	}
	return nil
}

func privateCASWindowsReadAt(parent windows.Handle, name string, maxBytes int) ([]byte, error) {
	body, _, err := privateCASWindowsReadStableAt(parent, name, maxBytes)
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
	defer windows.CloseHandle(root)
	validateRoot := func() error {
		if handled, err := validateOriginalPrivateCASWriteRootV1(authority, pins, maxBytes, validations); handled {
			return err
		}
		return privateCASWindowsValidatePinnedRootStrictAt(root, pins)
	}
	if err := validateRoot(); err != nil {
		return privateCASAdditionObservation{}, err
	}
	shard, shardIdentity, err := privateCASWindowsOpenPinnedShard(root, pins, digest[:2], false)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	defer windows.CloseHandle(shard)
	body, recordIdentity, err := privateCASWindowsReadStableAt(shard, digest+".json", maxBytes)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	rootMetadata, err := privateCASWindowsSnapshotMetadata(root)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	shardMetadata, err := privateCASWindowsSnapshotMetadata(shard)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	record, err := privateWindowsOpenRelative(shard, digest+".json", windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return privateCASAdditionObservation{}, err
	}
	currentRecord, identityErr := privateWindowsValidateAuthorityObject(record, false, uint64(maxBytes))
	recordMetadata, metadataErr := privateCASWindowsSnapshotMetadata(record)
	closeErr := windows.CloseHandle(record)
	if identityErr != nil || metadataErr != nil || closeErr != nil || currentRecord != recordIdentity {
		return privateCASAdditionObservation{}, errors.Join(
			errors.New("private CAS Windows addition record identity changed during observation"), identityErr, metadataErr, closeErr,
		)
	}
	if err := privateCASWindowsVerifyCurrentShard(authority, digest[:2], shardIdentity); err != nil {
		return privateCASAdditionObservation{}, err
	}
	if err := validateRoot(); err != nil {
		return privateCASAdditionObservation{}, err
	}
	return privateCASAdditionObservation{
		body:                 body,
		rootIdentityDigest:   privateCASWindowsObjectIdentityDigest("root", authority.id),
		shardIdentityDigest:  privateCASWindowsObjectIdentityDigest("shard", shardIdentity.id),
		recordIdentityDigest: privateCASWindowsObjectIdentityDigest("record", recordIdentity.id),
		rootMetadata:         rootMetadata, shardMetadata: shardMetadata, recordMetadata: recordMetadata,
	}, nil
}

func privateCASWindowsSnapshotMetadata(handle windows.Handle) (SecurePrivateCASSnapshotMetadataV1, error) {
	process := windows.CurrentProcess()
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(process, handle, process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return SecurePrivateCASSnapshotMetadataV1{}, err
	}
	file := os.NewFile(uintptr(duplicate), "private-cas-windows-snapshot-metadata")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return SecurePrivateCASSnapshotMetadataV1{}, errors.New("private CAS Windows metadata handle is invalid")
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

func privateCASWindowsObjectIdentityDigest(kind string, identity privateWindowsFileIDInfo) string {
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix-private-cas-object-identity-v1"))
	privateCASWriteFingerprintField(hasher, []byte(kind))
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], identity.VolumeSerialNumber)
	privateCASWriteFingerprintField(hasher, number[:])
	privateCASWriteFingerprintField(hasher, identity.FileID[:])
	return hex.EncodeToString(hasher.Sum(nil))
}

func privateCASWindowsReadStableAt(
	parent windows.Handle,
	name string,
	maxBytes int,
) ([]byte, privateWindowsObjectIdentity, error) {
	return privateCASWindowsReadStableAtWithHook(parent, name, maxBytes, nil)
}

func privateCASWindowsReadStableAtWithHook(
	parent windows.Handle,
	name string,
	maxBytes int,
	betweenReads func(),
) ([]byte, privateWindowsObjectIdentity, error) {
	body, identity, err := privateCASWindowsReadOnceAt(parent, name, maxBytes)
	if err != nil {
		return nil, privateWindowsObjectIdentity{}, err
	}
	if betweenReads != nil {
		betweenReads()
	}
	readback, current, err := privateCASWindowsReadOnceAt(parent, name, maxBytes)
	if err != nil || identity != current || !bytes.Equal(body, readback) {
		return nil, privateWindowsObjectIdentity{}, errors.New("private CAS Windows record changed during stable read")
	}
	finalIdentity, err := privateCASWindowsRecordIdentityAt(parent, name, maxBytes)
	if err != nil || finalIdentity != current {
		return nil, privateWindowsObjectIdentity{}, errors.New("private CAS Windows record name changed after stable read")
	}
	return body, identity, nil
}

func privateCASWindowsRecordIdentityAt(
	parent windows.Handle,
	name string,
	maxBytes int,
) (privateWindowsObjectIdentity, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if privateWindowsNotFound(err) {
		return privateWindowsObjectIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return privateWindowsObjectIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	return privateWindowsValidateAuthorityObject(handle, false, uint64(maxBytes))
}

func privateCASWindowsReadOnceAt(
	parent windows.Handle,
	name string,
	maxBytes int,
) ([]byte, privateWindowsObjectIdentity, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if privateWindowsNotFound(err) {
		return nil, privateWindowsObjectIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return nil, privateWindowsObjectIdentity{}, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, privateWindowsObjectIdentity{}, errors.New("private CAS Windows record handle is invalid")
	}
	defer file.Close()
	identity, err := privateWindowsValidateAuthorityObject(handle, false, uint64(maxBytes))
	if err != nil {
		return nil, privateWindowsObjectIdentity{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	size := uint64(identity.sizeHigh)<<32 | uint64(identity.sizeLow)
	if err != nil || uint64(len(body)) != size {
		return nil, privateWindowsObjectIdentity{}, errors.New("private CAS Windows record read failed")
	}
	current, err := privateWindowsValidateAuthorityObject(handle, false, uint64(maxBytes))
	if err != nil || current != identity {
		return nil, privateWindowsObjectIdentity{}, errors.New("private CAS Windows record changed during read")
	}
	return body, identity, nil
}

func privateCASWindowsSameCanonicalNames(expected []string, entries []os.DirEntry) bool {
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
