//go:build darwin

package fundsquerysource

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"unsafe"

	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

const (
	darwinSourceMaximumXattrBytesV1    = 64 << 10
	darwinSourceAttrBitmapCountV1      = 5
	darwinSourceAttributeSetBytesV1    = 5 * 4
	darwinSourceAttributeReferenceV1   = 8
	darwinSourceFileSecurityHeaderV1   = 44
	darwinSourceProvenanceNameV1       = "com.apple.provenance"
	darwinSourceProvenanceByteLengthV1 = 11
)

func verifyDarwinFilesystemV1(fd int) error {
	var info unix.Statfs_t
	if fd < 0 || unix.Fstatfs(fd, &info) != nil || !darwinSourceFilesystemSafeV1(info) {
		return errors.Join(
			fundsquerysourceport.ErrUnavailable,
			errors.New("funds query immutable snapshot filesystem is invalid"),
		)
	}
	return nil
}

func darwinSourceFilesystemSafeV1(info unix.Statfs_t) bool {
	kind, ok := darwinSourceFilesystemNameV1(info.Fstypename)
	forbidden := uint32(
		unix.MNT_RDONLY |
			unix.MNT_IGNORE_OWNERSHIP |
			unix.MNT_AUTOMOUNTED |
			unix.MNT_UNION |
			unix.MNT_SNAPSHOT,
	)
	return ok && kind == "apfs" && info.Flags&unix.MNT_LOCAL != 0 &&
		info.Flags&forbidden == 0
}

func darwinSourceFilesystemNameV1(raw [16]byte) (string, bool) {
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

// darwinSourceExtendedSecuritySafeV1 rejects every explicit ACL and every
// xattr except Darwin's fixed-format kernel provenance record. Mode bits and
// ownership alone do not describe all effective access on macOS.
func darwinSourceExtendedSecuritySafeV1(fd int) bool {
	return fd >= 0 && darwinSourceXattrsSafeV1(fd) && darwinSourceHasNoExtendedACLV1(fd)
}

func darwinSourceXattrsSafeV1(fd int) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > darwinSourceMaximumXattrBytesV1 {
		return false
	}
	if size == 0 {
		return true
	}
	names := make([]byte, size)
	read, err := unix.Flistxattr(fd, names)
	if err != nil || read != size ||
		!bytes.Equal(names, append([]byte(darwinSourceProvenanceNameV1), 0)) {
		return false
	}
	valueSize, err := unix.Fgetxattr(fd, darwinSourceProvenanceNameV1, nil)
	if err != nil || valueSize != darwinSourceProvenanceByteLengthV1 {
		return false
	}
	value := make([]byte, valueSize)
	read, err = unix.Fgetxattr(fd, darwinSourceProvenanceNameV1, value)
	return err == nil && read == len(value) && validDarwinSourceProvenanceV1(value)
}

// validDarwinSourceProvenanceV1 matches the repository's trusted Darwin
// source semantics: version 1, record type 2, a zero reserved byte, and a
// non-zero opaque kernel token. The token is not caller authority.
func validDarwinSourceProvenanceV1(value []byte) bool {
	if len(value) != darwinSourceProvenanceByteLengthV1 ||
		!bytes.Equal(value[:3], []byte{0x01, 0x02, 0x00}) {
		return false
	}
	for _, octet := range value[3:] {
		if octet != 0 {
			return true
		}
	}
	return false
}

func darwinSourceHasNoExtendedACLV1(fd int) bool {
	attributes := unix.Attrlist{
		Bitmapcount: darwinSourceAttrBitmapCountV1,
		Commonattr:  unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, darwinSourceMaximumXattrBytesV1)
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
	minimum := 4 + darwinSourceAttributeSetBytesV1
	if total < minimum || total > len(buffer) {
		return false
	}
	returnedCommon := binary.LittleEndian.Uint32(buffer[4:8])
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return true
	}
	referenceOffset := 4 + darwinSourceAttributeSetBytesV1
	if total < referenceOffset+darwinSourceAttributeReferenceV1 {
		return false
	}
	dataOffset := int(int32(binary.LittleEndian.Uint32(
		buffer[referenceOffset : referenceOffset+4],
	)))
	dataLength := int(binary.LittleEndian.Uint32(
		buffer[referenceOffset+4 : referenceOffset+8],
	))
	dataStart := referenceOffset + dataOffset
	if dataOffset < darwinSourceAttributeReferenceV1 ||
		dataLength < darwinSourceFileSecurityHeaderV1 ||
		dataStart < referenceOffset+darwinSourceAttributeReferenceV1 ||
		dataStart+dataLength > total {
		return false
	}
	return binary.LittleEndian.Uint32(buffer[dataStart+36:dataStart+40]) == math.MaxUint32
}
