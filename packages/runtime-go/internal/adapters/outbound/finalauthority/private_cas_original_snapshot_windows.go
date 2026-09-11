//go:build windows

package finalauthority

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Go File.Stat projects only READONLY and DIRECTORY into Perm for the
// non-reparse objects admitted by the CAS. Compare actual metadata with the
// frozen native attributes; neither DACLs nor a second transient Stat select
// an original mode.
func validatePrivateCASOriginalWindowsModeV1(mode uint32, identity privateWindowsObjectIdentity) error {
	if identity.attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("original private CAS mode has reparse attributes")
	}
	expected := uint32(0o666)
	if identity.attributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		expected = 0o444
	}
	if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		expected |= 0o111
	}
	if mode&uint32(os.ModePerm) != expected {
		return errors.New("original private CAS mode disagrees with frozen attributes")
	}
	return nil
}

func privateCASOriginalRootModeV1(authority privateCASRootAuthority) (mode uint32, resultErr error) {
	root, err := authority.open()
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(root)) }()
	metadata, err := privateCASWindowsSnapshotMetadata(root)
	return metadata.Mode & uint32(os.ModePerm), err
}

func securePrivateCASOriginalSnapshotV1(ctx context.Context, authority privateCASRootAuthority, expected privateCASRecoveryObservation, maxBytes int, allowCreates bool) (entries map[string]SecurePrivateCASOriginalEntryV1, resultErr error) {
	if len(expected.plan.originalCreateResidues) != 0 && !allowCreates {
		return nil, errors.New("original creation directories require their dedicated topology observation")
	}
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, windows.CloseHandle(root))
		if resultErr != nil {
			entries = nil
		}
	}()
	metadata, err := privateCASWindowsSnapshotMetadata(root)
	if err != nil {
		return nil, err
	}
	if err := validatePrivateCASOriginalWindowsModeV1(metadata.Mode, expected.plan.rootIdentity); err != nil {
		return nil, err
	}
	entries = map[string]SecurePrivateCASOriginalEntryV1{".": {Directory: true, Mode: metadata.Mode & uint32(os.ModePerm)}}
	var bytes int64
	for _, frozen := range expected.plan.shards {
		if err := privateCASContextError(ctx); err != nil {
			return nil, err
		}
		if err := func() (resultErr error) {
			shard, err := privateWindowsOpenShard(root, frozen.name, false)
			if err != nil {
				return err
			}
			defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(shard)) }()
			identity, err := privateWindowsValidateAuthorityObject(shard, true, 0)
			if err != nil || identity != frozen.identity {
				return errors.Join(errors.New("original private CAS shard changed"), err)
			}
			metadata, err := privateCASWindowsSnapshotMetadata(shard)
			if err != nil {
				return err
			}
			if err := validatePrivateCASOriginalWindowsModeV1(metadata.Mode, frozen.identity); err != nil {
				return err
			}
			if err := putPrivateCASOriginalEntryV1(entries, frozen.name, SecurePrivateCASOriginalEntryV1{Directory: true, Mode: metadata.Mode & uint32(os.ModePerm)}, &bytes); err != nil {
				return err
			}
			read := func(name string, identity privateWindowsObjectIdentity, hash [32]byte, allowEmpty bool) error {
				body, observed, observedHash, _, err := privateCASWindowsReadRecoveryBodyStableAt(ctx, shard, name, maxBytes, allowEmpty)
				if err != nil || observed != identity || observedHash != hash {
					return errors.Join(errors.New("original private CAS body changed"), err)
				}
				handle, err := privateWindowsOpenRelative(shard, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
				if err != nil {
					return err
				}
				metadata, metadataErr := privateCASWindowsSnapshotMetadata(handle)
				named, identityErr := privateWindowsValidateAuthorityObjectWithOptions(handle, false, uint64(maxBytes), allowEmpty, true)
				closeErr := windows.CloseHandle(handle)
				if err := errors.Join(metadataErr, identityErr, closeErr); err != nil {
					return err
				}
				if named != identity {
					return errors.New("original private CAS mode reader identity changed")
				}
				if err := validatePrivateCASOriginalWindowsModeV1(metadata.Mode, identity); err != nil {
					return err
				}
				return putPrivateCASOriginalEntryV1(entries, frozen.name+"/"+name, SecurePrivateCASOriginalEntryV1{Mode: metadata.Mode & uint32(os.ModePerm), Body: body}, &bytes)
			}
			for _, record := range frozen.committed {
				if err := read(record.name, record.identity, record.bodySHA256, false); err != nil {
					return err
				}
			}
			for _, residue := range frozen.temps {
				if err := read(residue.name, residue.identity, residue.bodySHA256, true); err != nil {
					return err
				}
			}
			identity, err = privateWindowsValidateAuthorityObject(shard, true, 0)
			if err != nil || identity != frozen.identity {
				return errors.Join(errors.New("original private CAS shard changed during read"), err)
			}
			return privateCASWindowsVerifyCurrentShard(authority, frozen.name, privateCASShardIdentity{id: frozen.identity.id})
		}(); err != nil {
			return nil, err
		}
	}
	return entries, nil
}
