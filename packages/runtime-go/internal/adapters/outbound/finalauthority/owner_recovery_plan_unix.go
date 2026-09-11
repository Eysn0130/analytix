//go:build darwin || linux

package finalauthority

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
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
	defer unix.Close(root)
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, err
	}
	entries, err := privateCASUnixReadDirBounded(root, len(expected))
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
			return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS owner container contains a non-directory leaf")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS owner container aliases a leaf by case")
		}
		seenFolded[folded] = struct{}{}
		actual = append(actual, name)
	}
	sort.Strings(actual)
	wanted := append([]string(nil), expected...)
	sort.Strings(wanted)
	if len(actual) != len(wanted) || strings.Join(actual, "\x00") != strings.Join(wanted, "\x00") {
		return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS owner container topology is not exact")
	}
	fingerprint := sha256.New()
	privateCASWriteFingerprintField(fingerprint, []byte("analytix-private-cas-owner-container-v1"))
	privateCASUnixFingerprintRecoveryObject(fingerprint, "root", rootStat, nil)
	durable := sha256.New()
	privateCASWriteFingerprintField(durable, []byte("analytix-private-cas-owner-container-durable/v4"))
	privateCASWriteFingerprintField(durable, []byte("root"))
	var rootStable [40]byte
	binary.BigEndian.PutUint64(rootStable[0:8], uint64(rootStat.Dev))
	binary.BigEndian.PutUint64(rootStable[8:16], rootStat.Ino)
	binary.BigEndian.PutUint64(rootStable[16:24], uint64(rootStat.Mode))
	binary.BigEndian.PutUint64(rootStable[24:32], uint64(rootStat.Uid))
	binary.BigEndian.PutUint64(rootStable[32:40], uint64(rootStat.Gid))
	privateCASWriteFingerprintField(durable, rootStable[:])
	for _, name := range actual {
		expectedSize, regular := regularFiles[name]
		openFlags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if regular {
			// A caller's regular-file classification is untrusted. Nonblocking
			// open lets the handle-bound Fstat reject a FIFO/device/socket
			// without waiting on the special object's data-plane semantics.
			openFlags |= unix.O_NONBLOCK
		} else {
			openFlags |= unix.O_DIRECTORY
		}
		leaf, err := unix.Openat(root, name, openFlags, 0)
		if err != nil {
			return securePrivateCASOwnerContainerObservationV1{}, err
		}
		var stat unix.Stat_t
		statErr := unix.Fstat(leaf, &stat)
		safe := statErr == nil && uint64(stat.Dev) == authority.dev && existingPrivateAuthorityExtendedSecuritySafe(leaf)
		if regular {
			safe = safe && privateStatRegular(stat) && stat.Mode&0o7077 == 0 && stat.Nlink == 1 &&
				stat.Uid == uint32(os.Geteuid()) && stat.Size == expectedSize
		} else {
			safe = safe && existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid()))
		}
		if !safe {
			_ = unix.Close(leaf)
			return securePrivateCASOwnerContainerObservationV1{}, errors.New("private CAS owner container leaf identity is unsafe")
		}
		if regular {
			privateCASWriteFingerprintField(fingerprint, []byte("regular-file:"+name))
			privateCASWriteFingerprintField(durable, []byte("regular-file:"+name))
			var stable [56]byte
			binary.BigEndian.PutUint64(stable[0:8], uint64(stat.Dev))
			binary.BigEndian.PutUint64(stable[8:16], stat.Ino)
			binary.BigEndian.PutUint64(stable[16:24], uint64(stat.Mode))
			binary.BigEndian.PutUint64(stable[24:32], uint64(stat.Uid))
			binary.BigEndian.PutUint64(stable[32:40], uint64(stat.Gid))
			binary.BigEndian.PutUint64(stable[40:48], uint64(stat.Nlink))
			binary.BigEndian.PutUint64(stable[48:56], uint64(stat.Size))
			privateCASWriteFingerprintField(fingerprint, stable[:])
			privateCASWriteFingerprintField(durable, stable[:])
		} else {
			privateCASWriteFingerprintField(fingerprint, []byte("leaf:"+name))
			privateCASWriteFingerprintField(durable, []byte("leaf:"+name))
			var stable [40]byte
			binary.BigEndian.PutUint64(stable[0:8], uint64(stat.Dev))
			binary.BigEndian.PutUint64(stable[8:16], stat.Ino)
			binary.BigEndian.PutUint64(stable[16:24], uint64(stat.Mode))
			binary.BigEndian.PutUint64(stable[24:32], uint64(stat.Uid))
			binary.BigEndian.PutUint64(stable[32:40], uint64(stat.Gid))
			privateCASWriteFingerprintField(fingerprint, stable[:])
			privateCASWriteFingerprintField(durable, stable[:])
		}
		if err := unix.Close(leaf); err != nil {
			return securePrivateCASOwnerContainerObservationV1{}, err
		}
	}
	var observation securePrivateCASOwnerContainerObservationV1
	copy(observation.fingerprint[:], fingerprint.Sum(nil))
	copy(observation.durableFingerprint[:], durable.Sum(nil))
	return observation, nil
}

type securePrivateCASOwnerUnixDiscoveryStateV2 struct {
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
		return securePrivateCASOwnerContainerObservationV1{}, nil, errors.New("private CAS Unix owner discovery is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, nil, err
	}
	defer unix.Close(root)
	var rootStat unix.Stat_t
	if err := unix.Fstat(root, &rootStat); err != nil || uint64(rootStat.Dev) != authority.dev ||
		!existingPrivateAuthorityRootSafe(rootStat, uint32(os.Geteuid())) || !existingPrivateAuthorityExtendedSecuritySafe(root) {
		return securePrivateCASOwnerContainerObservationV1{}, nil, errors.Join(errors.New("private CAS Unix discovered owner root is unsafe"), err)
	}
	fingerprint := sha256.New()
	durable := sha256.New()
	privateCASWriteFingerprintField(fingerprint, []byte("analytix-private-cas-owner-discovered/v2"))
	privateCASUnixFingerprintRecoveryObject(fingerprint, "root", rootStat, nil)
	privateCASWriteFingerprintField(durable, []byte("analytix-private-cas-owner-discovered-durable/v2"))
	privateCASUnixFingerprintRecoveryObject(durable, "root", rootStat, nil)
	state := &securePrivateCASOwnerUnixDiscoveryStateV2{
		discovery: discovery, fingerprint: fingerprint, durable: durable,
	}
	inventory, err := state.walk(root, "", true, authority.dev)
	if err != nil {
		return securePrivateCASOwnerContainerObservationV1{}, nil, err
	}
	var observation securePrivateCASOwnerContainerObservationV1
	copy(observation.fingerprint[:], fingerprint.Sum(nil))
	copy(observation.durableFingerprint[:], durable.Sum(nil))
	return observation, inventory, nil
}

func (state *securePrivateCASOwnerUnixDiscoveryStateV2) walk(
	parent int,
	relative string,
	direct bool,
	device uint64,
) ([]SecurePrivateCASOwnerEntryV2, error) {
	remaining := state.discovery.maxEntries - state.entries
	entries, err := privateCASUnixReadDirBounded(parent, remaining)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	seenFolded := make(map[string]struct{}, len(entries))
	inventory := make([]SecurePrivateCASOwnerEntryV2, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		folded := strings.ToLower(name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
			return nil, errors.New("private CAS Unix discovered owner entry name is invalid")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return nil, errors.New("private CAS Unix discovered owner aliases an entry by case")
		}
		seenFolded[folded] = struct{}{}
		state.entries++
		if state.entries > state.discovery.maxEntries {
			return nil, errors.New("private CAS Unix discovered owner entry bound exceeded")
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
			child, openErr := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if openErr != nil {
				return nil, openErr
			}
			var stat unix.Stat_t
			statErr := unix.Fstat(child, &stat)
			safe := statErr == nil && uint64(stat.Dev) == device &&
				existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid())) && existingPrivateAuthorityExtendedSecuritySafe(child)
			if !safe {
				return nil, errors.Join(errors.New("private CAS Unix discovered owner directory is unsafe"), statErr, unix.Close(child))
			}
			privateCASUnixFingerprintRecoveryObject(state.fingerprint, "directory:"+entryRelative, stat, nil)
			privateCASUnixFingerprintRecoveryObject(state.durable, "directory:"+entryRelative, stat, nil)
			if direct {
				inventory = append(inventory, SecurePrivateCASOwnerEntryV2{Name: name, Directory: true})
			}
			if !mutable {
				if _, walkErr := state.walk(child, entryRelative, false, device); walkErr != nil {
					_ = unix.Close(child)
					return nil, walkErr
				}
			}
			if closeErr := unix.Close(child); closeErr != nil {
				return nil, closeErr
			}
			continue
		}
		if mutable {
			return nil, errors.New("private CAS Unix discovered mutable owner leaf is not a directory")
		}
		file, openErr := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if openErr != nil {
			return nil, openErr
		}
		var before unix.Stat_t
		statErr := unix.Fstat(file, &before)
		safe := statErr == nil && uint64(before.Dev) == device && privateStatRegular(before) &&
			before.Nlink == 1 && before.Uid == uint32(os.Geteuid()) && existingPrivateAuthorityExtendedSecuritySafe(file) &&
			before.Size >= 0 && before.Size <= state.discovery.maxRegularBytes-state.bytes
		if !safe {
			_ = unix.Close(file)
			return nil, errors.New("private CAS Unix discovered owner regular file is unsafe or exceeds its byte bound")
		}
		digest := sha256.New()
		buffer := make([]byte, 32*1024)
		for {
			count, readErr := unix.Read(file, buffer)
			if count > 0 {
				_, _ = digest.Write(buffer[:count])
			}
			if readErr != nil {
				_ = unix.Close(file)
				return nil, readErr
			}
			if count == 0 {
				break
			}
		}
		var after unix.Stat_t
		if err := unix.Fstat(file, &after); err != nil || !privateCASUnixExactObject(before, after) {
			_ = unix.Close(file)
			return nil, errors.New("private CAS Unix discovered owner regular file changed while hashing")
		}
		if err := unix.Close(file); err != nil {
			return nil, err
		}
		state.bytes += before.Size
		var bodySHA256 [32]byte
		copy(bodySHA256[:], digest.Sum(nil))
		privateCASUnixFingerprintRecoveryObjectBodySHA256(state.fingerprint, "regular-file:"+entryRelative, before, bodySHA256)
		privateCASUnixFingerprintRecoveryObjectBodySHA256(state.durable, "regular-file:"+entryRelative, before, bodySHA256)
		if direct {
			inventory = append(inventory, SecurePrivateCASOwnerEntryV2{Name: name, ByteLength: before.Size})
		}
	}
	return inventory, nil
}
