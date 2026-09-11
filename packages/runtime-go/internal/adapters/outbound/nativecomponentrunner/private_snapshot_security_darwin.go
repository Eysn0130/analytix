//go:build darwin

package nativecomponentrunner

import (
	"bytes"
	"encoding/binary"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	privateSnapshotDarwinAttrBitmapCount     = 5
	privateSnapshotDarwinAttributeSetBytes   = 5 * 4
	privateSnapshotDarwinAttributeReference  = 8
	privateSnapshotDarwinFileSecurityHeader  = 44
	privateSnapshotDarwinReturnedCommonAttrs = 4
	privateSnapshotDarwinProvenanceName      = "com.apple.provenance"
	privateSnapshotDarwinProvenanceBytes     = 11
)

type privateSnapshotFilesystemIdentity struct {
	fsid [2]int64
}

type privateSnapshotExtendedSecurityIdentity struct {
	present bool
	value   [privateSnapshotDarwinProvenanceBytes]byte
}

func probePrivateSnapshotFilesystem(fd int) (privateSnapshotFilesystemIdentity, error) {
	if fd < 0 {
		return privateSnapshotFilesystemIdentity{}, ErrRegistry
	}
	var stat unix.Statfs_t
	if err := unix.Fstatfs(fd, &stat); err != nil {
		return privateSnapshotFilesystemIdentity{}, ErrRegistry
	}
	return validatePrivateSnapshotFilesystem(stat)
}

func validatePrivateSnapshotFilesystem(stat unix.Statfs_t) (privateSnapshotFilesystemIdentity, error) {
	kind, ok := privateSnapshotDarwinFilesystemName(stat.Fstypename)
	forbidden := uint32(
		unix.MNT_RDONLY |
			unix.MNT_IGNORE_OWNERSHIP |
			unix.MNT_AUTOMOUNTED |
			unix.MNT_UNION |
			unix.MNT_SNAPSHOT,
	)
	if !ok || kind != "apfs" || stat.Flags&unix.MNT_LOCAL == 0 || stat.Flags&forbidden != 0 {
		return privateSnapshotFilesystemIdentity{}, ErrRegistry
	}
	return privateSnapshotFilesystemIdentity{
		fsid: [2]int64{int64(stat.Fsid.Val[0]), int64(stat.Fsid.Val[1])},
	}, nil
}

func privateSnapshotDarwinFilesystemName(raw [16]byte) (string, bool) {
	end := -1
	for index, value := range raw {
		if value == 0 {
			end = index
			break
		}
		if value < 'a' || value > 'z' {
			return "", false
		}
	}
	if end <= 0 {
		return "", false
	}
	for _, value := range raw[end:] {
		if value != 0 {
			return "", false
		}
	}
	return string(raw[:end]), true
}

// privateSnapshotExtendedSecurity accepts no xattrs or exactly Darwin's
// kernel-managed provenance record. The provenance value is returned as part
// of the object identity so every descendant and every later use can require
// the same opaque token rather than accepting a merely well-formed substitute.
func privateSnapshotExtendedSecurity(fd int) (privateSnapshotExtendedSecurityIdentity, error) {
	if fd < 0 || !privateSnapshotHasNoExtendedACL(fd) {
		return privateSnapshotExtendedSecurityIdentity{}, ErrRegistry
	}
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return privateSnapshotExtendedSecurityIdentity{}, ErrRegistry
	}
	if size == 0 {
		return privateSnapshotExtendedSecurityIdentity{}, nil
	}
	names := make([]byte, size)
	read, err := unix.Flistxattr(fd, names)
	if err != nil || read != size || !bytes.Equal(names, append([]byte(privateSnapshotDarwinProvenanceName), 0)) {
		return privateSnapshotExtendedSecurityIdentity{}, ErrRegistry
	}
	valueSize, err := unix.Fgetxattr(fd, privateSnapshotDarwinProvenanceName, nil)
	if err != nil || valueSize != privateSnapshotDarwinProvenanceBytes {
		return privateSnapshotExtendedSecurityIdentity{}, ErrRegistry
	}
	value := make([]byte, valueSize)
	read, err = unix.Fgetxattr(fd, privateSnapshotDarwinProvenanceName, value)
	if err != nil || read != len(value) || !validPrivateSnapshotDarwinProvenance(value) {
		return privateSnapshotExtendedSecurityIdentity{}, ErrRegistry
	}
	identity := privateSnapshotExtendedSecurityIdentity{present: true}
	copy(identity.value[:], value)
	return identity, nil
}

func validPrivateSnapshotDarwinProvenance(value []byte) bool {
	if len(value) != privateSnapshotDarwinProvenanceBytes ||
		value[0] != 0x01 || value[1] != 0x02 || value[2] != 0x00 {
		return false
	}
	for _, octet := range value[3:] {
		if octet != 0 {
			return true
		}
	}
	return false
}

// privateSnapshotHasNoExtendedACL reads the Darwin file-security descriptor
// directly. UINT32_MAX is the kernel no-ACL marker; an empty explicit ACL is
// still extended security and is rejected.
func privateSnapshotHasNoExtendedACL(fd int) bool {
	if fd < 0 {
		return false
	}
	attributes := unix.Attrlist{
		Bitmapcount: privateSnapshotDarwinAttrBitmapCount,
		Commonattr:  unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, 64<<10)
	_, _, errno := unix.Syscall6(
		unix.SYS_FGETATTRLIST,
		uintptr(fd),
		uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		0,
		0,
	)
	if errno != 0 {
		return false
	}
	total := int(binary.LittleEndian.Uint32(buffer[:4]))
	minimum := 4 + privateSnapshotDarwinAttributeSetBytes
	if total < minimum || total > len(buffer) {
		return false
	}
	returnedCommon := binary.LittleEndian.Uint32(
		buffer[privateSnapshotDarwinReturnedCommonAttrs : privateSnapshotDarwinReturnedCommonAttrs+4],
	)
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return true
	}
	referenceOffset := 4 + privateSnapshotDarwinAttributeSetBytes
	if total < referenceOffset+privateSnapshotDarwinAttributeReference {
		return false
	}
	dataOffset := int(int32(binary.LittleEndian.Uint32(buffer[referenceOffset : referenceOffset+4])))
	dataLength := int(binary.LittleEndian.Uint32(buffer[referenceOffset+4 : referenceOffset+8]))
	dataStart := referenceOffset + dataOffset
	if dataOffset < privateSnapshotDarwinAttributeReference ||
		dataLength < privateSnapshotDarwinFileSecurityHeader ||
		dataStart < referenceOffset+privateSnapshotDarwinAttributeReference ||
		dataStart+dataLength > total {
		return false
	}
	entryCount := binary.LittleEndian.Uint32(buffer[dataStart+36 : dataStart+40])
	return entryCount == math.MaxUint32
}
