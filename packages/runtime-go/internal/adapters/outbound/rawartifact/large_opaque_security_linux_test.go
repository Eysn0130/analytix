//go:build linux

package rawartifact_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	rawartifactadapter "analytix.local/runtime-go/internal/adapters/outbound/rawartifact"
	"golang.org/x/sys/unix"
)

func TestLargeOpaqueLinuxRejectsExtendedSecurityAtEveryAuthorityLevel(t *testing.T) {
	mutations := []struct {
		name  string
		apply func(*testing.T, string, bool)
	}{
		{name: "unexpected-xattr", apply: applyLargeOpaqueLinuxUnexpectedXattr},
		{name: "inode-flags", apply: applyLargeOpaqueLinuxInodeFlags},
		{name: "posix-acl", apply: applyLargeOpaqueLinuxPOSIXACL},
	}
	for _, mutation := range mutations {
		for _, level := range []string{"root", "shard", "blob"} {
			t.Run(mutation.name+"-"+level, func(t *testing.T) {
				store, root, lease := openProvisionedLargeOpaqueChunkStore(t)
				body := bytes.Repeat([]byte("linux-extended-security"), 256)
				descriptor := buildLargeOpaqueDescriptor(t, mutation.name+"-"+level, body)
				if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
					t.Fatal(err)
				}
				path := root
				directory := true
				switch level {
				case "shard":
					path = filepath.Join(root, descriptor.DescriptorDigest[:2])
				case "blob":
					path = largeOpaqueChunkPath(root, descriptor)
					directory = false
				}
				mutation.apply(t, path, directory)

				var output bytes.Buffer
				if err := store.ReadExact(context.Background(), descriptor, &output); err == nil {
					t.Fatalf("%s %s metadata was accepted by exact read", mutation.name, level)
				}
				if output.Len() != 0 {
					t.Fatalf("%s %s metadata released %d bytes", mutation.name, level, output.Len())
				}
				if _, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
					context.Background(), lease,
				); err == nil {
					t.Fatalf("%s %s metadata was accepted by recovery preflight", mutation.name, level)
				}
			})
		}
	}
}

func applyLargeOpaqueLinuxUnexpectedXattr(t *testing.T, path string, directory bool) {
	t.Helper()
	const name = "user.analytix_large_opaque_test"
	if !directory {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := unix.Setxattr(path, name, []byte("unexpected"), 0); err != nil {
		if !directory {
			_ = os.Chmod(path, 0o400)
		}
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			t.Fatalf("release test filesystem does not support required user xattr validation: %v", err)
		}
		t.Fatal(err)
	}
	if !directory {
		if err := os.Chmod(path, 0o400); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if !directory {
			_ = os.Chmod(path, 0o600)
		}
		_ = unix.Removexattr(path, name)
		if !directory {
			_ = os.Chmod(path, 0o400)
		}
	})
}

func applyLargeOpaqueLinuxInodeFlags(t *testing.T, path string, _ bool) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	const fsNodumpFlag = 0x40
	setErr := unix.IoctlSetPointerInt(int(file.Fd()), unix.FS_IOC_SETFLAGS, fsNodumpFlag)
	closeErr := file.Close()
	if setErr != nil || closeErr != nil {
		if errors.Is(setErr, unix.ENOTTY) || errors.Is(setErr, unix.EOPNOTSUPP) {
			t.Fatalf("release test filesystem does not support required inode-flag validation: %v", setErr)
		}
		t.Fatalf("set inode flags=%v close=%v", setErr, closeErr)
	}
	t.Cleanup(func() {
		file, openErr := os.Open(path)
		if openErr == nil {
			_ = unix.IoctlSetPointerInt(int(file.Fd()), unix.FS_IOC_SETFLAGS, 0)
			_ = file.Close()
		}
	})
}

func applyLargeOpaqueLinuxPOSIXACL(t *testing.T, path string, _ bool) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	acl := buildLargeOpaqueLinuxPOSIXACL(before.Mode().Perm())
	if err := unix.Setxattr(path, "system.posix_acl_access", acl, 0); err != nil {
		t.Fatalf("release test filesystem cannot install the required POSIX ACL fixture: %v", err)
	}
	t.Cleanup(func() { _ = unix.Removexattr(path, "system.posix_acl_access") })
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("POSIX ACL fixture changed mode: before=%#o after=%#o", before.Mode().Perm(), after.Mode().Perm())
	}
	if size, err := unix.Getxattr(path, "system.posix_acl_access", nil); err != nil || size <= 0 {
		t.Fatalf("POSIX ACL xattr missing: size=%d err=%v", size, err)
	}
}

func buildLargeOpaqueLinuxPOSIXACL(mode os.FileMode) []byte {
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
