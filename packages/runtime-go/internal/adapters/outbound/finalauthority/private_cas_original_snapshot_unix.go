//go:build darwin || linux

package finalauthority

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func privateCASOriginalRootModeV1(authority privateCASRootAuthority) (mode uint32, resultErr error) {
	root, err := authority.open()
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, unix.Close(root)) }()
	metadata, err := privateCASUnixSnapshotMetadata(root)
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
		resultErr = errors.Join(resultErr, unix.Close(root))
		if resultErr != nil {
			entries = nil
		}
	}()
	metadata, err := privateCASUnixSnapshotMetadata(root)
	if err != nil {
		return nil, err
	}
	entries = map[string]SecurePrivateCASOriginalEntryV1{".": {Directory: true, Mode: metadata.Mode & uint32(os.ModePerm)}}
	var bytes int64
	for _, frozen := range expected.plan.shards {
		if err := privateCASContextError(ctx); err != nil {
			return nil, err
		}
		if err := func() (resultErr error) {
			shard, err := securePrivateOpenShard(root, frozen.name, false)
			if err != nil {
				return err
			}
			defer func() { resultErr = errors.Join(resultErr, unix.Close(shard)) }()
			var stat unix.Stat_t
			if err := unix.Fstat(shard, &stat); err != nil || !privateCASUnixExactObject(stat, frozen.stat) {
				return errors.Join(errors.New("original private CAS shard changed"), err)
			}
			metadata, err := privateCASUnixSnapshotMetadata(shard)
			if err != nil {
				return err
			}
			if err := putPrivateCASOriginalEntryV1(entries, frozen.name, SecurePrivateCASOriginalEntryV1{Directory: true, Mode: metadata.Mode & uint32(os.ModePerm)}, &bytes); err != nil {
				return err
			}
			read := func(name string, identity unix.Stat_t, hash [32]byte, allowEmpty bool) error {
				body, observed, observedHash, _, err := privateCASUnixReadRecoveryBodyStableAt(ctx, shard, name, maxBytes, allowEmpty)
				if err != nil || !privateCASUnixExactObject(observed, identity) || observedHash != hash {
					return errors.Join(errors.New("original private CAS body changed"), err)
				}
				// Stat_t came from the same pinned native read and is compared in
				// full above; snapshot metadata supplies the portable mode.
				fd, err := unix.Openat(shard, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
				if err != nil {
					return err
				}
				metadata, metadataErr := privateCASUnixSnapshotMetadata(fd)
				var named unix.Stat_t
				statErr := unix.Fstat(fd, &named)
				closeErr := unix.Close(fd)
				if err := errors.Join(metadataErr, statErr, closeErr); err != nil {
					return err
				}
				if !privateCASUnixExactObject(named, identity) {
					return errors.New("original private CAS mode reader identity changed")
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
			if err := unix.Fstat(shard, &stat); err != nil || !privateCASUnixExactObject(stat, frozen.stat) {
				return errors.Join(errors.New("original private CAS shard changed during read"), err)
			}
			return privateCASUnixVerifyCurrentShard(authority, frozen.name, frozen.identity)
		}(); err != nil {
			return nil, err
		}
	}
	return entries, nil
}
