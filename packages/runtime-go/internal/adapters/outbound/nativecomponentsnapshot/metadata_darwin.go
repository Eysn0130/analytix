//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentsnapshot

import (
	"bytes"
	"encoding/binary"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	sourceAttrBitmapCountV1    = 5
	sourceAttributeSetBytesV1  = 5 * 4
	sourceAttributeReferenceV1 = 8
	sourceFileSecurityHeaderV1 = 44
	sourceProvenanceNameV1     = "com.apple.provenance"
	sourceProvenanceBytesV1    = 11
)

// sourceExtendedSecuritySafeV1 rejects BSD flags, every explicit ACL, and
// every xattr except Darwin's fixed-format kernel provenance record. Mode bits
// alone are not a complete write-authority boundary on macOS.
func sourceExtendedSecuritySafeV1(fd int) bool {
	if fd < 0 {
		return false
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Flags != 0 || !sourceProvenanceSafeV1(fd) {
		return false
	}
	attributes := unix.Attrlist{
		Bitmapcount: sourceAttrBitmapCountV1,
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
	minimum := 4 + sourceAttributeSetBytesV1
	if total < minimum || total > len(buffer) {
		return false
	}
	returnedCommon := binary.LittleEndian.Uint32(buffer[4:8])
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return true
	}
	referenceOffset := 4 + sourceAttributeSetBytesV1
	if total < referenceOffset+sourceAttributeReferenceV1 {
		return false
	}
	offset := int(int32(binary.LittleEndian.Uint32(buffer[referenceOffset : referenceOffset+4])))
	length := int(binary.LittleEndian.Uint32(buffer[referenceOffset+4 : referenceOffset+8]))
	start := referenceOffset + offset
	if offset < sourceAttributeReferenceV1 || length < sourceFileSecurityHeaderV1 ||
		start < referenceOffset+sourceAttributeReferenceV1 || start+length > total {
		return false
	}
	return binary.LittleEndian.Uint32(buffer[start+36:start+40]) == math.MaxUint32
}

func sourceProvenanceSafeV1(fd int) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return false
	}
	if size == 0 {
		return true
	}
	names := make([]byte, size)
	read, err := unix.Flistxattr(fd, names)
	if err != nil || read != size || !bytes.Equal(names, append([]byte(sourceProvenanceNameV1), 0)) {
		return false
	}
	valueSize, err := unix.Fgetxattr(fd, sourceProvenanceNameV1, nil)
	if err != nil || valueSize != sourceProvenanceBytesV1 {
		return false
	}
	value := make([]byte, valueSize)
	read, err = unix.Fgetxattr(fd, sourceProvenanceNameV1, value)
	if err != nil || read != len(value) || !bytes.Equal(value[:3], []byte{1, 2, 0}) {
		return false
	}
	for _, octet := range value[3:] {
		if octet != 0 {
			return true
		}
	}
	return false
}
