//go:build darwin

package persistencefs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	separateOwnerDarwinAttrBitmapCount    = 5
	separateOwnerDarwinAttributeSetBytes  = 5 * 4
	separateOwnerDarwinAttributeReference = 8
	separateOwnerDarwinFileSecurityHeader = 44
	separateOwnerDarwinACEBytes           = 24
	separateOwnerDarwinMaxACLEntries      = 128
	separateOwnerDarwinProvenanceName     = "com.apple.provenance"
	separateOwnerDarwinProvenanceBytes    = 11
	separateOwnerDarwinFileSecurityMagic  = 0x012cc16d
	separateOwnerDarwinACEKindMask        = 0x0f
	separateOwnerDarwinACEDeny            = 0x02
	separateOwnerDarwinACEFlags           = 0x01f0
	separateOwnerDarwinACLPrivateFlags    = 0xffff
	separateOwnerDarwinACLDeferInherit    = 1 << 16
	separateOwnerDarwinACLNoInherit       = 1 << 17
)

func platformSeparateOwnerExtendedSecurity(
	fd int,
	scope separateOwnerUnixSecurityScope,
) (separateOwnerUnixExtendedSecurityV1, error) {
	if fd < 0 || scope != separateOwnerAncestorScope && scope != separateOwnerObjectScope {
		return separateOwnerUnixExtendedSecurityV1{}, errors.New("separate-owner Darwin security scope is invalid")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return separateOwnerUnixExtendedSecurityV1{}, err
	}
	aclKind, aclDigest, aclEntries, err := separateOwnerDarwinACL(fd)
	if err != nil {
		return separateOwnerUnixExtendedSecurityV1{}, err
	}
	xattrs, err := readSeparateOwnerUnixXattrs(fd)
	if err != nil {
		return separateOwnerUnixExtendedSecurityV1{}, err
	}
	result := separateOwnerUnixExtendedSecurityV1{
		Platform: "darwin", Flags: uint64(stat.Flags), ACLKind: aclKind,
		ACLSHA256: aclDigest, ACLEntries: aclEntries, Xattrs: xattrs,
	}
	if scope == separateOwnerAncestorScope {
		return result, nil
	}
	if aclKind != "none" || stat.Flags != 0 || !separateOwnerDarwinObjectXattrsSafe(xattrs) {
		clearSeparateOwnerUnixXattrs(xattrs)
		return separateOwnerUnixExtendedSecurityV1{}, errors.New("separate-owner Darwin object metadata is unsafe")
	}
	return result, nil
}

func separateOwnerDarwinObjectXattrsSafe(xattrs []separateOwnerUnixXattrV1) bool {
	if len(xattrs) == 0 {
		return true
	}
	if len(xattrs) != 1 || xattrs[0].Name != separateOwnerDarwinProvenanceName ||
		xattrs[0].Size != separateOwnerDarwinProvenanceBytes ||
		len(xattrs[0].value) != separateOwnerDarwinProvenanceBytes {
		return false
	}
	value := xattrs[0].value
	if !bytes.Equal(value[:3], []byte{0x01, 0x02, 0x00}) {
		return false
	}
	for _, octet := range value[3:] {
		if octet != 0 {
			return true
		}
	}
	return false
}

// separateOwnerDarwinACL reads and hash-binds the descriptor's canonical
// kauth_filesec record. Ancestor custody records retain an explicit ACL in the
// authority digest so normal macOS home-directory deny/delete ACLs remain
// observable. The caller still rejects every explicit ACL on an owned mutable
// object before a durable mutation can begin.
func separateOwnerDarwinACL(fd int) (string, string, uint32, error) {
	attributes := unix.Attrlist{
		Bitmapcount: separateOwnerDarwinAttrBitmapCount,
		Commonattr:  unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, 64<<10)
	_, _, errno := unix.Syscall6(
		unix.SYS_FGETATTRLIST,
		uintptr(fd), uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0,
	)
	if errno != 0 {
		return "", "", 0, errno
	}
	total := int(binary.LittleEndian.Uint32(buffer[:4]))
	minimum := 4 + separateOwnerDarwinAttributeSetBytes
	if total < minimum || total > len(buffer) {
		return "", "", 0, errors.New("separate-owner Darwin ACL response is malformed")
	}
	returnedCommon := binary.LittleEndian.Uint32(buffer[4:8])
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return "none", separateOwnerSHA256(nil), 0, nil
	}
	referenceOffset := 4 + separateOwnerDarwinAttributeSetBytes
	if total < referenceOffset+separateOwnerDarwinAttributeReference {
		return "", "", 0, errors.New("separate-owner Darwin ACL reference is truncated")
	}
	dataOffset := int(int32(binary.LittleEndian.Uint32(buffer[referenceOffset : referenceOffset+4])))
	dataLength := int(binary.LittleEndian.Uint32(buffer[referenceOffset+4 : referenceOffset+8]))
	dataStart := referenceOffset + dataOffset
	if dataOffset < separateOwnerDarwinAttributeReference ||
		dataLength < separateOwnerDarwinFileSecurityHeader ||
		dataStart < referenceOffset+separateOwnerDarwinAttributeReference || dataStart+dataLength > total {
		return "", "", 0, errors.New("separate-owner Darwin ACL bounds are invalid")
	}
	security := buffer[dataStart : dataStart+dataLength]
	if binary.LittleEndian.Uint32(security[:4]) != separateOwnerDarwinFileSecurityMagic {
		return "", "", 0, errors.New("separate-owner Darwin ACL magic is invalid")
	}
	entryCount := binary.LittleEndian.Uint32(security[36:40])
	aclFlags := binary.LittleEndian.Uint32(security[40:44])
	if aclFlags&separateOwnerDarwinACLDeferInherit != 0 ||
		aclFlags & ^uint32(separateOwnerDarwinACLPrivateFlags|separateOwnerDarwinACLNoInherit) != 0 {
		return "", "", 0, errors.New("separate-owner Darwin ACL flags are unsafe")
	}
	if entryCount == math.MaxUint32 {
		if dataLength != separateOwnerDarwinFileSecurityHeader {
			return "", "", 0, errors.New("separate-owner Darwin no-ACL record has trailing data")
		}
		return "none", separateOwnerSHA256(security), 0, nil
	}
	if entryCount > separateOwnerDarwinMaxACLEntries ||
		dataLength != separateOwnerDarwinFileSecurityHeader+int(entryCount)*separateOwnerDarwinACEBytes {
		return "", "", 0, errors.New("separate-owner Darwin ACL entry count is invalid")
	}
	return "filesec-bound", separateOwnerSHA256(security), entryCount, nil
}
