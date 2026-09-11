//go:build darwin

package secureconfigfs

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinFilesystemNameRequiresCanonicalNULTerminatedLowercaseName(t *testing.T) {
	var valid [16]byte
	copy(valid[:], "apfs")
	if name, ok := darwinFilesystemName(valid); !ok || name != "apfs" {
		t.Fatalf("canonical APFS name = %q, %v", name, ok)
	}
	unterminated := [16]byte{'a', 'p', 'f', 's', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x'}
	noncanonical := valid
	noncanonical[0] = 'A'
	residue := valid
	residue[5] = 'x'
	for name, raw := range map[string][16]byte{
		"unterminated": unterminated, "uppercase": noncanonical, "post-NUL residue": residue,
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := darwinFilesystemName(raw); ok {
				t.Fatal("unsafe Darwin filesystem name was accepted")
			}
		})
	}
}

func TestDarwinSecureFilesystemPolicyRequiresManagedLocalAPFS(t *testing.T) {
	var managed unix.Statfs_t
	copy(managed.Fstypename[:], "apfs")
	managed.Flags = unix.MNT_LOCAL
	if identity, err := validateDarwinSecureFilesystem(managed); err != nil || identity.kind != "apfs" {
		t.Fatalf("managed local APFS was rejected: identity=%#v err=%v", identity, err)
	}

	notAPFS := managed
	clear(notAPFS.Fstypename[:])
	copy(notAPFS.Fstypename[:], "hfs")
	missingLocal := managed
	missingLocal.Flags &^= unix.MNT_LOCAL
	for name, stat := range map[string]unix.Statfs_t{
		"non APFS":     notAPFS,
		"non local":    missingLocal,
		"no ownership": darwinFilesystemWithFlag(managed, unix.MNT_IGNORE_OWNERSHIP),
		"automounted":  darwinFilesystemWithFlag(managed, unix.MNT_AUTOMOUNTED),
		"removable":    darwinFilesystemWithFlag(managed, unix.MNT_REMOVABLE),
		"union":        darwinFilesystemWithFlag(managed, unix.MNT_UNION),
		"snapshot":     darwinFilesystemWithFlag(managed, unix.MNT_SNAPSHOT),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateDarwinSecureFilesystem(stat); err == nil {
				t.Fatal("unsafe Darwin filesystem was accepted")
			}
		})
	}
}

func darwinFilesystemWithFlag(stat unix.Statfs_t, flag uint32) unix.Statfs_t {
	stat.Flags |= flag
	return stat
}
