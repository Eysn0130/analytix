//go:build windows

package finalauthority

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
)

func observeSecurePrivateCASOwnerContainerV1(
	authority privateCASRootAuthority,
	expected []string,
	regularFiles map[string]int64,
) (securePrivateCASOwnerContainerObservationV1, error) {
	root, err := authority.open()
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, err
	}
	defer windows.CloseHandle(root)
	rootIdentity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, err
	}
	entries, err := privateCASWindowsReadDirBounded(root, len(expected))
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, err
	}
	actual := make([]string, 0, len(entries))
	seenFolded := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		folded := strings.ToLower(name)
		_, regular := regularFiles[name]
		if entry.IsDir() == regular {
			return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS Windows owner container contains a non-directory leaf")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS Windows owner container aliases a leaf by case")
		}
		seenFolded[folded] = struct{}{}
		actual = append(actual, name)
	}
	sort.Strings(actual)
	wanted := append([]string(nil), expected...)
	sort.Strings(wanted)
	if len(actual) != len(wanted) || strings.Join(actual, "\x00") != strings.Join(wanted, "\x00") {
		return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS Windows owner container topology is not exact")
	}
	fingerprint := sha256.New()
	privateCASWriteFingerprintField(fingerprint, []byte("analytix-private-cas-owner-container-v1"))
	privateCASWindowsFingerprintRecord(fingerprint, "root", nil, rootIdentity)
	durable := sha256.New()
	privateCASWriteFingerprintField(durable, []byte("analytix-private-cas-owner-container-durable/v4"))
	privateCASWriteFingerprintField(durable, []byte("root"))
	var rootStable [28]byte
	binary.BigEndian.PutUint64(rootStable[0:8], rootIdentity.id.VolumeSerialNumber)
	copy(rootStable[8:24], rootIdentity.id.FileID[:])
	binary.BigEndian.PutUint32(rootStable[24:28], rootIdentity.attributes)
	privateCASWriteFingerprintField(durable, rootStable[:])
	for _, name := range actual {
		expectedSize, regular := regularFiles[name]
		leaf, err := privateWindowsOpenRelative(root, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, !regular)
		if err != nil {
			return securePrivateCASOwnerContainerObservationV1{}, err
		}
		var identity privateWindowsObjectIdentity
		var identityErr error
		if regular {
			identity, identityErr = privateWindowsValidateAuthorityObjectWithOptions(
				leaf, false, uint64(expectedSize)+1, true, false,
			)
		} else {
			identity, identityErr = privateWindowsValidateAuthorityObject(leaf, true, 0)
		}
		actualSize := uint64(identity.sizeHigh)<<32 | uint64(identity.sizeLow)
		if identityErr != nil || identity.id.VolumeSerialNumber != authority.id.VolumeSerialNumber ||
			regular && actualSize != uint64(expectedSize) {
			_ = windows.CloseHandle(leaf)
			return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS Windows owner container leaf identity is unsafe")
		}
		if regular {
			privateCASWriteFingerprintField(fingerprint, []byte("regular-file:"+name))
			privateCASWriteFingerprintField(durable, []byte("regular-file:"+name))
			var stable [40]byte
			binary.BigEndian.PutUint64(stable[0:8], identity.id.VolumeSerialNumber)
			copy(stable[8:24], identity.id.FileID[:])
			binary.BigEndian.PutUint32(stable[24:28], identity.attributes)
			binary.BigEndian.PutUint32(stable[28:32], identity.links)
			binary.BigEndian.PutUint32(stable[32:36], identity.sizeHigh)
			binary.BigEndian.PutUint32(stable[36:40], identity.sizeLow)
			privateCASWriteFingerprintField(fingerprint, stable[:])
			privateCASWriteFingerprintField(durable, stable[:])
		} else {
			privateCASWriteFingerprintField(fingerprint, []byte("leaf:"+name))
			privateCASWriteFingerprintField(durable, []byte("leaf:"+name))
			var stable [28]byte
			binary.BigEndian.PutUint64(stable[0:8], identity.id.VolumeSerialNumber)
			copy(stable[8:24], identity.id.FileID[:])
			binary.BigEndian.PutUint32(stable[24:28], identity.attributes)
			privateCASWriteFingerprintField(fingerprint, stable[:])
			privateCASWriteFingerprintField(durable, stable[:])
		}
		if err := windows.CloseHandle(leaf); err != nil {
			return securePrivateCASOwnerContainerObservationV1{}, err
		}
	}
	var observation securePrivateCASOwnerContainerObservationV1
	metadata, err := privateCASWindowsSnapshotMetadata(root)
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, err
	}
	if err := validatePrivateCASOriginalWindowsModeV1(metadata.Mode, rootIdentity); err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, err
	}
	observation.originalRootMode = metadata.Mode & 0o777
	copy(observation.fingerprint[:], fingerprint.Sum(nil))
	copy(observation.durableFingerprint[:], durable.Sum(nil))
	return observation, nil
}

type securePrivateCASOwnerWindowsDiscoveryStateV2 struct {
	discovery   *securePrivateCASOwnerDiscoveryV2
	fingerprint hash.Hash
	durable     hash.Hash
	entries     int
	bytes       int64
}

func discoverSecurePrivateCASOwnerContainerV2(
	authority privateCASRootAuthority,
	discovery *securePrivateCASOwnerDiscoveryV2,
) (securePrivateCASOwnerContainerObservationV1, []SecurePrivateCASOwnerEntryV2, error) {
	if discovery == nil || discovery.maxEntries <= 0 || discovery.maxRegularBytes < 0 {
		return securePrivateCASOwnerContainerObservationV1{}, nil, errors.New("private CAS Windows owner discovery is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, nil, err
	}
	defer windows.CloseHandle(root)
	rootIdentity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil || rootIdentity.id.VolumeSerialNumber != authority.id.VolumeSerialNumber {
		return securePrivateCASOwnerContainerObservationV1{}, nil, errors.Join(errors.New("private CAS Windows discovered owner root is unsafe"), err)
	}
	fingerprint := sha256.New()
	durable := sha256.New()
	privateCASWriteFingerprintField(fingerprint, []byte("analytix-private-cas-owner-discovered/v2"))
	privateCASWindowsFingerprintRecord(fingerprint, "root", nil, rootIdentity)
	privateCASWriteFingerprintField(durable, []byte("analytix-private-cas-owner-discovered-durable/v2"))
	privateCASWindowsFingerprintRecord(durable, "root", nil, rootIdentity)
	state := &securePrivateCASOwnerWindowsDiscoveryStateV2{
		discovery: discovery, fingerprint: fingerprint, durable: durable,
	}
	inventory, err := state.walk(root, "", true, authority.id.VolumeSerialNumber)
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, nil, err
	}
	var observation securePrivateCASOwnerContainerObservationV1
	metadata, err := privateCASWindowsSnapshotMetadata(root)
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, nil, err
	}
	if err := validatePrivateCASOriginalWindowsModeV1(metadata.Mode, rootIdentity); err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, nil, err
	}
	observation.originalRootMode = metadata.Mode & 0o777
	copy(observation.fingerprint[:], fingerprint.Sum(nil))
	copy(observation.durableFingerprint[:], durable.Sum(nil))
	return observation, inventory, nil
}

func (state *securePrivateCASOwnerWindowsDiscoveryStateV2) walk(
	parent windows.Handle,
	relative string,
	direct bool,
	volume uint64,
) ([]SecurePrivateCASOwnerEntryV2, error) {
	remaining := state.discovery.maxEntries - state.entries
	entries, err := privateCASWindowsReadDirBounded(parent, remaining)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	seenFolded := make(map[string]struct{}, len(entries))
	inventory := make([]SecurePrivateCASOwnerEntryV2, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		folded := strings.ToLower(name)
		if !privateWindowsComponent(name) {
			return nil, errors.New("private CAS Windows discovered owner entry name is invalid")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return nil, errors.New("private CAS Windows discovered owner aliases an entry by case")
		}
		seenFolded[folded] = struct{}{}
		state.entries++
		if state.entries > state.discovery.maxEntries {
			return nil, errors.New("private CAS Windows discovered owner entry bound exceeded")
		}
		entryRelative := name
		if relative != "" {
			entryRelative = relative + "/" + name
		}
		mutable := false
		if direct {
			_, mutable = state.discovery.mutableDirectories[name]
		}
		if entry.IsDir() {
			child, openErr := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
			if openErr != nil {
				return nil, openErr
			}
			identity, identityErr := privateWindowsValidateAuthorityObject(child, true, 0)
			if identityErr != nil || identity.id.VolumeSerialNumber != volume {
				return nil, errors.Join(errors.New("private CAS Windows discovered owner directory is unsafe"), identityErr, windows.CloseHandle(child))
			}
			privateCASWindowsFingerprintRecord(state.fingerprint, "directory:"+entryRelative, nil, identity)
			privateCASWindowsFingerprintRecord(state.durable, "directory:"+entryRelative, nil, identity)
			if direct {
				inventory = append(inventory, SecurePrivateCASOwnerEntryV2{Name: name, Directory: true})
			}
			if !mutable {
				if _, walkErr := state.walk(child, entryRelative, false, volume); walkErr != nil {
					_ = windows.CloseHandle(child)
					return nil, walkErr
				}
			}
			if closeErr := windows.CloseHandle(child); closeErr != nil {
				return nil, closeErr
			}
			continue
		}
		if mutable {
			return nil, errors.New("private CAS Windows discovered mutable owner leaf is not a directory")
		}
		file, openErr := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
		if openErr != nil {
			return nil, openErr
		}
		before, identityErr := privateWindowsValidateAuthorityObjectWithOptions(
			file, false, uint64(state.discovery.maxRegularBytes)+1, true, false,
		)
		size := int64(uint64(before.sizeHigh)<<32 | uint64(before.sizeLow))
		if identityErr != nil || before.id.VolumeSerialNumber != volume || size < 0 ||
			size > state.discovery.maxRegularBytes-state.bytes {
			_ = windows.CloseHandle(file)
			return nil, errors.New("private CAS Windows discovered owner regular file is unsafe or exceeds its byte bound")
		}
		digest := sha256.New()
		buffer := make([]byte, 32*1024)
		for {
			var count uint32
			readErr := windows.ReadFile(file, buffer, &count, nil)
			if count > 0 {
				_, _ = digest.Write(buffer[:count])
			}
			if readErr != nil {
				_ = windows.CloseHandle(file)
				return nil, readErr
			}
			if count == 0 {
				break
			}
		}
		after, afterErr := privateWindowsValidateAuthorityObjectWithOptions(
			file, false, uint64(state.discovery.maxRegularBytes)+1, true, false,
		)
		if afterErr != nil || before != after {
			_ = windows.CloseHandle(file)
			return nil, errors.New("private CAS Windows discovered owner regular file changed while hashing")
		}
		if err := windows.CloseHandle(file); err != nil {
			return nil, err
		}
		state.bytes += size
		var bodySHA256 [32]byte
		copy(bodySHA256[:], digest.Sum(nil))
		privateCASWindowsFingerprintRecordBodySHA256(state.fingerprint, "regular-file:"+entryRelative, bodySHA256, before)
		privateCASWindowsFingerprintRecordBodySHA256(state.durable, "regular-file:"+entryRelative, bodySHA256, before)
		if direct {
			inventory = append(inventory, SecurePrivateCASOwnerEntryV2{Name: name, ByteLength: size})
		}
	}
	return inventory, nil
}
