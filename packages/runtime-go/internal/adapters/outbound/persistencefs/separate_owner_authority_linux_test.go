//go:build linux

package persistencefs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSeparateOwnerLinuxPOSIXACLIsRejected(t *testing.T) {
	data, durable, owner := newSeparateOwnerLinuxRoots(t)
	info, err := os.Stat(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(owner, "system.posix_acl_access", separateOwnerLinuxPOSIXACL(info.Mode().Perm()), 0); err != nil {
		t.Fatalf("install POSIX ACL fixture: %v", err)
	}
	t.Cleanup(func() { _ = unix.Removexattr(owner, "system.posix_acl_access") })
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner); err == nil {
		_ = lease.Close()
		t.Fatal("POSIX ACL entered separate-owner authority")
	}
}

func TestSeparateOwnerLinuxUnknownOwnerXattrIsRejected(t *testing.T) {
	data, durable, owner := newSeparateOwnerLinuxRoots(t)
	const name = "user.analytix_separate_owner_test"
	if err := unix.Setxattr(owner, name, []byte("unsafe"), 0); err != nil {
		t.Fatalf("install unknown xattr fixture: %v", err)
	}
	t.Cleanup(func() { _ = unix.Removexattr(owner, name) })
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner); err == nil {
		_ = lease.Close()
		t.Fatal("unknown xattr entered separate-owner authority")
	}
}

func newSeparateOwnerLinuxRoots(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	owner := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, owner} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return data, durable, owner
}

func separateOwnerLinuxPOSIXACL(mode os.FileMode) []byte {
	const (
		aclVersion  = uint32(0x0002)
		aclUserObj  = uint16(0x0001)
		aclUser     = uint16(0x0002)
		aclGroupObj = uint16(0x0004)
		aclMask     = uint16(0x0010)
		aclOther    = uint16(0x0020)
		aclNoID     = uint32(0xffffffff)
	)
	acl := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(acl[:4], aclVersion)
	entries := []struct {
		tag  uint16
		perm uint16
		id   uint32
	}{
		{tag: aclUserObj, perm: uint16(mode.Perm() >> 6 & 0x7), id: aclNoID},
		{tag: aclUser, perm: 0, id: 65534},
		{tag: aclGroupObj, perm: uint16(mode.Perm() >> 3 & 0x7), id: aclNoID},
		{tag: aclMask, perm: uint16(mode.Perm() >> 3 & 0x7), id: aclNoID},
		{tag: aclOther, perm: uint16(mode.Perm() & 0x7), id: aclNoID},
	}
	for index, entry := range entries {
		offset := 4 + index*8
		binary.LittleEndian.PutUint16(acl[offset:offset+2], entry.tag)
		binary.LittleEndian.PutUint16(acl[offset+2:offset+4], entry.perm)
		binary.LittleEndian.PutUint32(acl[offset+4:offset+8], entry.id)
	}
	return acl
}
