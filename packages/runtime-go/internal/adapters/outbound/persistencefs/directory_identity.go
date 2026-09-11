package persistencefs

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func formatUnixObjectIdentity(device, inode uint64) string {
	return fmt.Sprintf("%s:%016x:%016x", privatecasport.DirectoryIdentityUnix, device, inode)
}

func formatWindowsObjectIdentity(volumeSerial uint64, fileID [16]byte) string {
	return fmt.Sprintf("%s:%016x:%s", privatecasport.DirectoryIdentityWindows, volumeSerial, hex.EncodeToString(fileID[:]))
}

func formatStrongDirectoryIdentity(identity privatecasport.DirectoryIdentity) (string, error) {
	switch identity.Kind {
	case privatecasport.DirectoryIdentityUnix:
		if identity.Device == 0 || identity.Inode == 0 || identity.VolumeSerial != 0 || identity.FileID != ([16]byte{}) {
			return "", errors.New("Unix directory identity is invalid")
		}
		return formatUnixObjectIdentity(identity.Device, identity.Inode), nil
	case privatecasport.DirectoryIdentityWindows:
		if identity.Device != 0 || identity.Inode != 0 || identity.VolumeSerial == 0 || identity.FileID == ([16]byte{}) {
			return "", errors.New("Windows directory identity is invalid")
		}
		return formatWindowsObjectIdentity(identity.VolumeSerial, identity.FileID), nil
	default:
		return "", errors.New("directory identity kind is invalid")
	}
}

func parseStrongDirectoryIdentity(value string) (privatecasport.DirectoryIdentity, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 || value != strings.ToLower(strings.TrimSpace(value)) {
		return privatecasport.DirectoryIdentity{}, errors.New("directory identity is not canonical")
	}
	switch parts[0] {
	case privatecasport.DirectoryIdentityUnix:
		if len(parts[1]) != 16 || len(parts[2]) != 16 {
			return privatecasport.DirectoryIdentity{}, errors.New("Unix directory identity width is invalid")
		}
		device, deviceErr := strconv.ParseUint(parts[1], 16, 64)
		inode, inodeErr := strconv.ParseUint(parts[2], 16, 64)
		identity := privatecasport.DirectoryIdentity{Kind: parts[0], Device: device, Inode: inode}
		canonical, err := formatStrongDirectoryIdentity(identity)
		if deviceErr != nil || inodeErr != nil || err != nil || canonical != value {
			return privatecasport.DirectoryIdentity{}, errors.New("Unix directory identity is invalid")
		}
		return identity, nil
	case privatecasport.DirectoryIdentityWindows:
		if len(parts[1]) != 16 || len(parts[2]) != 32 {
			return privatecasport.DirectoryIdentity{}, errors.New("Windows directory identity width is invalid")
		}
		volume, volumeErr := strconv.ParseUint(parts[1], 16, 64)
		fileID, fileErr := hex.DecodeString(parts[2])
		identity := privatecasport.DirectoryIdentity{Kind: parts[0], VolumeSerial: volume}
		if len(fileID) == len(identity.FileID) {
			copy(identity.FileID[:], fileID)
		}
		canonical, err := formatStrongDirectoryIdentity(identity)
		if volumeErr != nil || fileErr != nil || err != nil || canonical != value {
			return privatecasport.DirectoryIdentity{}, errors.New("Windows directory identity is invalid")
		}
		return identity, nil
	default:
		return privatecasport.DirectoryIdentity{}, errors.New("directory identity kind is invalid")
	}
}

// formatLegacyDirectoryIdentityV3 is verify-only input for frozen V3 journal
// migration. Current authority must use the versioned full identity above.
func formatLegacyDirectoryIdentityV3(first, second, third uint64) string {
	return fmt.Sprintf("%d:%d:%d", first, second, third)
}
