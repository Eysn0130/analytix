//go:build darwin

package securegeneration

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"
)

func clearCreatedExtendedSecurity(fd int) error {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return errors.Join(ErrUnsafeRoot, err)
	}
	if size == 0 {
		return nil
	}
	buffer := make([]byte, size)
	read, err := unix.Flistxattr(fd, buffer)
	if err != nil || read != size {
		return errors.Join(ErrUnsafeRoot, err)
	}
	for _, raw := range bytes.Split(buffer, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		if err := unix.Fremovexattr(fd, string(raw)); err != nil {
			if protectedDarwinProvenance(fd, string(raw)) {
				continue
			}
			return errors.Join(ErrUnsafeRoot, err)
		}
	}
	return nil
}

// privateExtendedSecuritySafe rejects every explicit macOS ACL. Mode bits do
// not reveal an inherited or deny-only ACL, so the check reads the descriptor
// itself through fgetattrlist and accepts only the kernel no-ACL marker.
func privateExtendedSecuritySafe(fd int) bool {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > 64<<10 {
		return false
	}
	if size > 0 {
		buffer := make([]byte, size)
		read, err := unix.Flistxattr(fd, buffer)
		if err != nil || read != size {
			return false
		}
		for _, raw := range bytes.Split(buffer, []byte{0}) {
			if len(raw) == 0 {
				continue
			}
			if !protectedDarwinProvenance(fd, string(raw)) {
				return false
			}
		}
	}
	const (
		bitmapCount        = 5
		attributeSetBytes  = 5 * 4
		attributeReference = 8
		fileSecurityHeader = 44
	)
	attributes := unix.Attrlist{
		Bitmapcount: bitmapCount,
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
	minimum := 4 + attributeSetBytes
	if total < minimum || total > len(buffer) {
		return false
	}
	returnedCommon := binary.LittleEndian.Uint32(buffer[4:8])
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return true
	}
	referenceOffset := 4 + attributeSetBytes
	if total < referenceOffset+attributeReference {
		return false
	}
	offset := int(int32(binary.LittleEndian.Uint32(buffer[referenceOffset : referenceOffset+4])))
	length := int(binary.LittleEndian.Uint32(buffer[referenceOffset+4 : referenceOffset+8]))
	start := referenceOffset + offset
	if offset < attributeReference || length < fileSecurityHeader || start < referenceOffset+attributeReference || start+length > total {
		return false
	}
	return binary.LittleEndian.Uint32(buffer[start+36:start+40]) == math.MaxUint32
}

func protectedDarwinProvenance(fd int, name string) bool {
	if name != "com.apple.provenance" {
		return false
	}
	provenanceSize, err := unix.Fgetxattr(fd, name, nil)
	if err != nil || provenanceSize != 11 {
		return false
	}
	provenance := make([]byte, provenanceSize)
	read, err := unix.Fgetxattr(fd, name, provenance)
	return err == nil && read == provenanceSize && bytes.Equal(provenance[:3], []byte{1, 2, 0})
}

func privateFileFlagsSafe(fd int) bool {
	var stat unix.Stat_t
	return unix.Fstat(fd, &stat) == nil && stat.Flags == 0
}
