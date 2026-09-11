//go:build linux

package persistencefs

import (
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	separateOwnerLinuxImmutable = 0x00000010
	separateOwnerLinuxAppend    = 0x00000020
)

func platformSeparateOwnerExtendedSecurity(
	fd int,
	scope separateOwnerUnixSecurityScope,
) (separateOwnerUnixExtendedSecurityV1, error) {
	if fd < 0 || scope != separateOwnerAncestorScope && scope != separateOwnerObjectScope {
		return separateOwnerUnixExtendedSecurityV1{}, errors.New("separate-owner Linux security scope is invalid")
	}
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return separateOwnerUnixExtendedSecurityV1{}, errors.Join(errors.New("separate-owner Linux inode flags are unavailable"), err)
	}
	xattrs, err := readSeparateOwnerUnixXattrs(fd)
	if err != nil {
		return separateOwnerUnixExtendedSecurityV1{}, err
	}
	aclKind, aclDigest, err := separateOwnerLinuxACLBinding(xattrs)
	if err != nil {
		clearSeparateOwnerUnixXattrs(xattrs)
		return separateOwnerUnixExtendedSecurityV1{}, err
	}
	result := separateOwnerUnixExtendedSecurityV1{
		Platform: "linux", Flags: uint64(uint32(flags)), ACLKind: aclKind,
		ACLSHA256: aclDigest, Xattrs: xattrs,
	}
	if scope == separateOwnerAncestorScope {
		return result, nil
	}
	if flags&(separateOwnerLinuxImmutable|separateOwnerLinuxAppend) != 0 ||
		!separateOwnerLinuxObjectXattrsSafe(xattrs) {
		clearSeparateOwnerUnixXattrs(xattrs)
		return separateOwnerUnixExtendedSecurityV1{}, errors.New("separate-owner Linux object metadata is unsafe")
	}
	return result, nil
}

func separateOwnerLinuxACLBinding(xattrs []separateOwnerUnixXattrV1) (string, string, error) {
	acl := make([]separateOwnerUnixXattrV1, 0, 2)
	for _, xattr := range xattrs {
		if xattr.Name == "system.posix_acl_access" || xattr.Name == "system.posix_acl_default" ||
			xattr.Name == "system.nfs4_acl" || strings.Contains(xattr.Name, "richacl") {
			acl = append(acl, separateOwnerUnixXattrV1{Name: xattr.Name, Size: xattr.Size, SHA256: xattr.SHA256})
		}
	}
	if len(acl) == 0 {
		return "none", separateOwnerSHA256(nil), nil
	}
	body, err := json.Marshal(acl)
	if err != nil {
		return "", "", err
	}
	return "xattr-bound", separateOwnerSHA256(body), nil
}

func separateOwnerLinuxObjectXattrsSafe(xattrs []separateOwnerUnixXattrV1) bool {
	for _, xattr := range xattrs {
		switch xattr.Name {
		case "security.selinux", "security.SMACK64", "security.SMACK64EXEC", "security.SMACK64MMAP", "security.SMACK64TRANSMUTE":
			if xattr.Size == 0 || len(xattr.value) != xattr.Size {
				return false
			}
		default:
			return false
		}
	}
	return true
}
