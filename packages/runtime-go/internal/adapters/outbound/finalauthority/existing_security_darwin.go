//go:build darwin

package finalauthority

import (
	"encoding/binary"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinAttrBitMapCount     = 5
	darwinAttributeSetBytes   = 5 * 4
	darwinAttributeReference  = 8
	darwinFileSecurityHeader  = 44
	darwinReturnedCommonAttrs = 4
)

// existingPrivateAuthorityExtendedSecuritySafe rejects any macOS extended
// ACL. It uses fgetattrlist directly so production builds remain CGO-free.
// A kauth_filesec entry count of UINT32_MAX is the kernel's canonical no-ACL
// marker; an empty or populated ACL is still explicit extended security and
// is rejected for installation private-key material.
func existingPrivateAuthorityExtendedSecuritySafe(fd int) bool {
	attributes := unix.Attrlist{
		Bitmapcount: darwinAttrBitMapCount,
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
	minimum := 4 + darwinAttributeSetBytes
	if total < minimum || total > len(buffer) {
		return false
	}
	returnedCommon := binary.LittleEndian.Uint32(buffer[darwinReturnedCommonAttrs : darwinReturnedCommonAttrs+4])
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return true
	}
	referenceOffset := 4 + darwinAttributeSetBytes
	if total < referenceOffset+darwinAttributeReference {
		return false
	}
	dataOffset := int(int32(binary.LittleEndian.Uint32(buffer[referenceOffset : referenceOffset+4])))
	dataLength := int(binary.LittleEndian.Uint32(buffer[referenceOffset+4 : referenceOffset+8]))
	dataStart := referenceOffset + dataOffset
	if dataOffset < darwinAttributeReference || dataLength < darwinFileSecurityHeader ||
		dataStart < referenceOffset+darwinAttributeReference || dataStart+dataLength > total {
		return false
	}
	entryCount := binary.LittleEndian.Uint32(buffer[dataStart+36 : dataStart+40])
	return entryCount == math.MaxUint32
}
