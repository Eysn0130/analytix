//go:build darwin

package persistencefs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSeparateOwnerDarwinDenyACLIsRejectedBeforeAuthority(t *testing.T) {
	data, durable, owner := newSeparateOwnerDarwinRoots(t)
	addDarwinACL(t, owner, "everyone deny delete")
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner); err == nil {
		_ = lease.Close()
		t.Fatal("deny-only ACL entered separate-owner authority")
	}
}

func TestSeparateOwnerDarwinPermitACLIsRejected(t *testing.T) {
	data, durable, owner := newSeparateOwnerDarwinRoots(t)
	addDarwinACL(t, owner, "everyone allow read,write")
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner); err == nil {
		_ = lease.Close()
		t.Fatal("permit ACL entered separate-owner authority")
	}
}

func TestSeparateOwnerDarwinAncestorACLIsBoundWithoutBlockingOwnerCustody(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	ancestor := filepath.Join(base, "home")
	owner := filepath.Join(ancestor, "Library", "Application Support", "analytix")
	for _, root := range []string{data, durable, owner} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	addDarwinACL(t, ancestor, "everyone deny delete")
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if output, err := exec.Command("/bin/chmod", "-N", ancestor).CombinedOutput(); err != nil {
		t.Fatalf("clear Darwin ancestor ACL: %v: %s", err, output)
	}
	if _, held := lease.FrozenSeparateOwnerAuthority(owner); held {
		t.Fatal("ancestor ACL change retained stale separate-owner authority")
	}
}

func TestSeparateOwnerDarwinUnknownOwnerXattrIsRejected(t *testing.T) {
	data, durable, owner := newSeparateOwnerDarwinRoots(t)
	const name = "com.analytix.separate-owner-test"
	if err := unix.Setxattr(owner, name, []byte("unsafe"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Removexattr(owner, name) })
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner); err == nil {
		_ = lease.Close()
		t.Fatal("unknown owner xattr entered separate-owner authority")
	}
}

func TestSeparateOwnerDarwinAncestorXattrChangeInvalidatesLease(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	owner := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, owner} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	const name = "com.analytix.separate-owner-ancestor-test"
	if err := unix.Setxattr(base, name, []byte("changed"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Removexattr(base, name) })
	if _, held := lease.FrozenSeparateOwnerAuthority(owner); held {
		t.Fatal("ancestor xattr change retained stale separate-owner authority")
	}
}

func TestSeparateOwnerDarwinProvenanceValidation(t *testing.T) {
	valid := separateOwnerUnixXattrV1{
		Name: separateOwnerDarwinProvenanceName, Size: separateOwnerDarwinProvenanceBytes,
		value: []byte{0x01, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0},
	}
	if !separateOwnerDarwinObjectXattrsSafe([]separateOwnerUnixXattrV1{valid}) {
		t.Fatal("canonical Darwin provenance was rejected")
	}
	mutated := valid
	mutated.value = append([]byte(nil), valid.value...)
	mutated.value[2] = 0x01
	if separateOwnerDarwinObjectXattrsSafe([]separateOwnerUnixXattrV1{mutated}) {
		t.Fatal("mutated Darwin provenance was accepted")
	}
	zeroToken := valid
	zeroToken.value = []byte{0x01, 0x02, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}
	if separateOwnerDarwinObjectXattrsSafe([]separateOwnerUnixXattrV1{zeroToken}) {
		t.Fatal("zero-token Darwin provenance was accepted")
	}
}

func newSeparateOwnerDarwinRoots(t *testing.T) (string, string, string) {
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

func addDarwinACL(t *testing.T, path string, entry string) {
	t.Helper()
	command := exec.Command("/bin/chmod", "+a", entry, path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("add Darwin ACL: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("/bin/chmod", "-N", path).CombinedOutput(); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("clear Darwin ACL: %v: %s", err, output)
		}
	})
}
