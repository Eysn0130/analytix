//go:build darwin

package processauthority

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinBuildProbeAttrBitMapCount     = 5
	darwinBuildProbeAttributeSetBytes   = 5 * 4
	darwinBuildProbeAttributeReference  = 8
	darwinBuildProbeFileSecurityHeader  = 44
	darwinBuildProbeReturnedCommonAttrs = 4
	darwinBuildProbeProvenanceName      = "com.apple.provenance"
	darwinBuildProbeProvenanceBytes     = 11
)

// darwinBuildProbeObjectSafe is the strict metadata boundary used for build
// probe directories and staged files. Exact modes are intentional: umask,
// inherited ACLs, xattrs, and BSD flags must never silently broaden or alter
// the private staging authority.
func darwinBuildProbeObjectSafe(file *os.File, objectType uint16, permissions uint16, owner uint32) bool {
	if file == nil {
		return false
	}
	return darwinBuildProbeFDObjectSafe(int(file.Fd()), objectType, permissions, owner)
}

func darwinBuildProbeFDObjectSafe(fd int, objectType uint16, permissions uint16, owner uint32) bool {
	if !darwinBuildProbeFDObjectMetadataSafe(fd, objectType, permissions, owner) {
		return false
	}
	provenance, ok := darwinBuildProbeProvenance(fd)
	return ok && len(provenance) == 0
}

func darwinBuildProbeCreatedObjectSafe(
	file *os.File,
	objectType uint16,
	permissions uint16,
	owner uint32,
	expectedProvenance []byte,
) bool {
	if file == nil {
		return false
	}
	return darwinBuildProbeCreatedFDObjectSafe(
		int(file.Fd()), objectType, permissions, owner, expectedProvenance,
	)
}

func darwinBuildProbeCreatedFDObjectSafe(
	fd int,
	objectType uint16,
	permissions uint16,
	owner uint32,
	expectedProvenance []byte,
) bool {
	if !darwinBuildProbeFDObjectMetadataSafe(fd, objectType, permissions, owner) {
		return false
	}
	provenance, ok := darwinBuildProbeProvenance(fd)
	return ok && bytes.Equal(provenance, expectedProvenance)
}

func darwinBuildProbeFDObjectMetadataSafe(fd int, objectType uint16, permissions uint16, owner uint32) bool {
	if fd < 0 {
		return false
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != objectType ||
		stat.Mode&0o7777 != permissions || stat.Uid != owner || stat.Flags != 0 {
		return false
	}
	if objectType == unix.S_IFREG && stat.Nlink != 1 {
		return false
	}
	return darwinBuildProbeHasNoExtendedACL(fd)
}

// darwinBuildProbeProvenance accepts either no xattrs or exactly Darwin's
// kernel-managed provenance xattr. The format is version 1, record type 2,
// zero reserved byte, and an opaque non-zero 64-bit kernel token. Callers bind
// every created object to the exact first token; no other name or value is an
// accepted metadata channel.
func darwinBuildProbeProvenance(fd int) ([]byte, bool) {
	if fd < 0 {
		return nil, false
	}
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return nil, false
	}
	if size == 0 {
		return nil, true
	}
	names := make([]byte, size)
	read, err := unix.Flistxattr(fd, names)
	if err != nil || read != size {
		return nil, false
	}
	wantName := append([]byte(darwinBuildProbeProvenanceName), 0)
	if !bytes.Equal(names, wantName) {
		return nil, false
	}
	valueSize, err := unix.Fgetxattr(fd, darwinBuildProbeProvenanceName, nil)
	if err != nil || valueSize != darwinBuildProbeProvenanceBytes {
		return nil, false
	}
	value := make([]byte, valueSize)
	read, err = unix.Fgetxattr(fd, darwinBuildProbeProvenanceName, value)
	if err != nil || read != len(value) || !validDarwinBuildProbeProvenance(value) {
		return nil, false
	}
	return value, true
}

func validDarwinBuildProbeProvenance(value []byte) bool {
	if len(value) != darwinBuildProbeProvenanceBytes || value[0] != 0x01 || value[1] != 0x02 || value[2] != 0x00 {
		return false
	}
	for _, octet := range value[3:] {
		if octet != 0 {
			return true
		}
	}
	return false
}

// darwinBuildProbeHasNoExtendedACL uses fgetattrlist so the authority remains
// CGO-free. UINT32_MAX is Darwin's canonical no-ACL marker; an empty explicit
// ACL is still extended security and is rejected.
func darwinBuildProbeHasNoExtendedACL(fd int) bool {
	if fd < 0 {
		return false
	}
	attributes := unix.Attrlist{
		Bitmapcount: darwinBuildProbeAttrBitMapCount,
		Commonattr:  unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, 64<<10)
	_, _, errno := unix.Syscall6(
		unix.SYS_FGETATTRLIST,
		uintptr(fd), uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0,
	)
	if errno != 0 {
		return false
	}
	total := int(binary.LittleEndian.Uint32(buffer[:4]))
	minimum := 4 + darwinBuildProbeAttributeSetBytes
	if total < minimum || total > len(buffer) {
		return false
	}
	returnedCommon := binary.LittleEndian.Uint32(
		buffer[darwinBuildProbeReturnedCommonAttrs : darwinBuildProbeReturnedCommonAttrs+4],
	)
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return true
	}
	referenceOffset := 4 + darwinBuildProbeAttributeSetBytes
	if total < referenceOffset+darwinBuildProbeAttributeReference {
		return false
	}
	dataOffset := int(int32(binary.LittleEndian.Uint32(buffer[referenceOffset : referenceOffset+4])))
	dataLength := int(binary.LittleEndian.Uint32(buffer[referenceOffset+4 : referenceOffset+8]))
	dataStart := referenceOffset + dataOffset
	if dataOffset < darwinBuildProbeAttributeReference || dataLength < darwinBuildProbeFileSecurityHeader ||
		dataStart < referenceOffset+darwinBuildProbeAttributeReference || dataStart+dataLength > total {
		return false
	}
	entryCount := binary.LittleEndian.Uint32(buffer[dataStart+36 : dataStart+40])
	return entryCount == math.MaxUint32
}
