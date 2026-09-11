package persistencefs

import (
	"testing"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func TestStrongDirectoryIdentityCanonicalGolden(t *testing.T) {
	unixIdentity := privatecasport.DirectoryIdentity{
		Kind: privatecasport.DirectoryIdentityUnix, Device: 0x0123456789abcdef, Inode: 0xfedcba9876543210,
	}
	windowsIdentity := privatecasport.DirectoryIdentity{
		Kind: privatecasport.DirectoryIdentityWindows, VolumeSerial: 0x0123456789abcdef,
		FileID: [16]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f},
	}
	for _, test := range []struct {
		name     string
		identity privatecasport.DirectoryIdentity
		expected string
	}{
		{
			name: "unix", identity: unixIdentity,
			expected: "unix-dev-inode-v1:0123456789abcdef:fedcba9876543210",
		},
		{
			name: "windows", identity: windowsIdentity,
			expected: "windows-file-id-128-v1:0123456789abcdef:000102030405060708090a0b0c0d0e0f",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := formatStrongDirectoryIdentity(test.identity)
			if err != nil || encoded != test.expected {
				t.Fatalf("canonical identity = %q, %v", encoded, err)
			}
			decoded, err := parseStrongDirectoryIdentity(encoded)
			if err != nil || decoded != test.identity {
				t.Fatalf("canonical identity round trip = %#v, %v", decoded, err)
			}
		})
	}
}

func TestStrongDirectoryIdentityRejectsNonCanonicalAndZeroValues(t *testing.T) {
	for _, value := range []string{
		"", "unix-dev-inode-v1:0000000000000000:0000000000000001",
		"unix-dev-inode-v1:1:0000000000000001",
		"UNIX-DEV-INODE-V1:0000000000000001:0000000000000001",
		"windows-file-id-128-v1:0000000000000001:00000000000000000000000000000000",
		"windows-file-id-128-v1:0000000000000001:000102030405060708090a0b0c0d0e",
		"1:2:3",
	} {
		if _, err := parseStrongDirectoryIdentity(value); err == nil {
			t.Fatalf("non-canonical identity %q was accepted", value)
		}
	}
}

func TestRootCapabilityDigestBindsCompleteWindowsFileID(t *testing.T) {
	firstID := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	secondID := firstID
	secondID[15] ^= 0xff
	first := frozenRootCapability{
		Root: "C:\\data", Anchor: "C:\\data",
		AnchorIdentity: formatWindowsObjectIdentity(7, firstID), RootIdentity: formatWindowsObjectIdentity(7, firstID),
	}
	second := first
	second.AnchorIdentity = formatWindowsObjectIdentity(7, secondID)
	second.RootIdentity = formatWindowsObjectIdentity(7, secondID)
	if rootCapabilityDigest(map[string]frozenRootCapability{"root": first}) ==
		rootCapabilityDigest(map[string]frozenRootCapability{"root": second}) {
		t.Fatal("root capability digest ignored the high half of the Windows FileID")
	}
}
