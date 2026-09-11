//go:build darwin

package nativecomponentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"golang.org/x/sys/unix"
)

func privateSnapshotSetNonEffectiveGroup(t *testing.T, fd int) {
	t.Helper()
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal("read fixture groups")
	}
	for _, group := range groups {
		if group != os.Getegid() && unix.Fchown(fd, -1, group) == nil {
			var info unix.Stat_t
			if unix.Fstat(fd, &info) != nil || info.Gid == uint32(os.Getegid()) {
				t.Fatal("fixture did not establish a non-effective group")
			}
			return
		}
	}
	t.Skip("inherited-group counterexample unavailable: no usable non-effective group")
}

func TestPrivateSnapshotDirectoryRejectsInheritedGroup(t *testing.T) {
	root, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal("open group fixture")
	}
	defer root.Close()
	fd := int(root.Fd())
	before, err := validatePrivateSnapshotDirectory(fd)
	if err != nil {
		t.Fatal("valid group fixture rejected")
	}
	privateSnapshotSetNonEffectiveGroup(t, fd)
	if _, err := validatePrivateSnapshotDirectory(fd); !errors.Is(err, ErrRegistry) {
		t.Fatal("non-effective group passed the directory consumer guard")
	}
	if unix.Fchown(fd, -1, os.Getegid()) != nil || validatePrivateSnapshotDirectoryExact(fd, before) != nil {
		t.Fatal("restored directory group or identity rejected")
	}
}

func TestPrivateSnapshotFilesystemRequiresWritableOwnedLocalAPFS(t *testing.T) {
	var valid unix.Statfs_t
	copy(valid.Fstypename[:], "apfs")
	valid.Flags = unix.MNT_LOCAL
	valid.Fsid.Val[0] = 17
	valid.Fsid.Val[1] = 23
	identity, err := validatePrivateSnapshotFilesystem(valid)
	if err != nil || identity.fsid != [2]int64{17, 23} {
		t.Fatalf("valid APFS rejected: identity=%#v err=%v", identity, err)
	}

	for name, flag := range map[string]uint32{
		"read-only":        unix.MNT_RDONLY,
		"ignore-ownership": unix.MNT_IGNORE_OWNERSHIP,
		"automounted":      unix.MNT_AUTOMOUNTED,
		"union":            unix.MNT_UNION,
		"snapshot":         unix.MNT_SNAPSHOT,
	} {
		t.Run(name, func(t *testing.T) {
			unsafe := valid
			unsafe.Flags |= flag
			if _, err := validatePrivateSnapshotFilesystem(unsafe); !errors.Is(err, ErrRegistry) {
				t.Fatalf("unsafe filesystem flag %#x accepted: %v", flag, err)
			}
		})
	}
	missingLocal := valid
	missingLocal.Flags &^= unix.MNT_LOCAL
	if _, err := validatePrivateSnapshotFilesystem(missingLocal); !errors.Is(err, ErrRegistry) {
		t.Fatalf("non-local APFS accepted: %v", err)
	}
	nonAPFS := valid
	clear(nonAPFS.Fstypename[:])
	copy(nonAPFS.Fstypename[:], "hfs")
	if _, err := validatePrivateSnapshotFilesystem(nonAPFS); !errors.Is(err, ErrRegistry) {
		t.Fatalf("non-APFS filesystem accepted: %v", err)
	}
	removableLocal := valid
	removableLocal.Flags |= unix.MNT_REMOVABLE
	if _, err := validatePrivateSnapshotFilesystem(removableLocal); err != nil {
		t.Fatalf("writable owned local APFS removable volume rejected: %v", err)
	}
}

func TestPrivateSnapshotLiveHierarchyRetainsExactSecurityIdentity(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.Chmod(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.Open(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	rootIdentity, err := validatePrivateSnapshotDirectory(int(root.Fd()))
	if err != nil {
		t.Fatalf("root authority: %v", err)
	}
	if err := unix.Mkdirat(int(root.Fd()), "operation", 0o700); err != nil {
		t.Fatal(err)
	}
	defer unix.Unlinkat(int(root.Fd()), "operation", unix.AT_REMOVEDIR)
	activeRoot, err := validatePrivateSnapshotDirectory(int(root.Fd()))
	if err != nil || !validPrivateSnapshotDirectoryCreationTransition(rootIdentity, activeRoot) {
		t.Fatalf("root transition: before=%#v after=%#v err=%v", rootIdentity, activeRoot, err)
	}
	operationFD, err := unix.Openat(
		int(root.Fd()), "operation", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	operation := os.NewFile(uintptr(operationFD), "operation")
	if operation == nil {
		_ = unix.Close(operationFD)
		t.Fatal("operation authority is nil")
	}
	defer operation.Close()
	operationIdentity, err := validatePrivateSnapshotDirectory(operationFD)
	if err != nil {
		t.Fatalf("operation authority: %v", err)
	}
	if operationIdentity.filesystem != activeRoot.filesystem ||
		operationIdentity.extendedSecurity != activeRoot.extendedSecurity {
		t.Fatalf("operation security drifted: root=%#v operation=%#v", activeRoot, operationIdentity)
	}
	fileFD, err := unix.Openat(
		operationFD, privateAccountFlowSnapshotBasename,
		unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600,
	)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(fileFD), "snapshot")
	if file == nil {
		_ = unix.Close(fileFD)
		t.Fatal("snapshot authority is nil")
	}
	defer file.Close()
	defer unix.Unlinkat(operationFD, privateAccountFlowSnapshotBasename, 0)
	fileIdentity, err := validatePrivateSnapshotFile(
		fileFD, 0, 1, 0o600, unix.O_RDWR,
		operationIdentity.filesystem, operationIdentity.extendedSecurity,
	)
	if err != nil {
		t.Fatalf("created snapshot authority: %v", err)
	}
	if err := validatePrivateSnapshotFileBinding(
		operationFD, privateAccountFlowSnapshotBasename, fileIdentity, 1, 0o600,
	); err != nil {
		t.Fatalf("created snapshot binding: %v", err)
	}
}

func TestPrivateSnapshotDirectoryRejectsACLXattrFlagsAndMetadataDrift(t *testing.T) {
	for _, mutation := range []string{"acl", "xattr", "flags", "mode", "links"} {
		t.Run(mutation, func(t *testing.T) {
			rootPath := t.TempDir()
			if err := os.Chmod(rootPath, 0o700); err != nil {
				t.Fatal(err)
			}
			root, err := os.Open(rootPath)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			identity, err := validatePrivateSnapshotDirectory(int(root.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			restore := func() {}
			switch mutation {
			case "acl":
				permission := "everyone allow list,search,add_file,delete_child"
				if output, err := exec.Command("/bin/chmod", "+a", permission, rootPath).CombinedOutput(); err != nil {
					t.Fatalf("install ACL fixture: %v: %s", err, output)
				}
				restore = func() { _, _ = exec.Command("/bin/chmod", "-N", rootPath).CombinedOutput() }
			case "xattr":
				if err := unix.Fsetxattr(int(root.Fd()), "com.analytix.injected", []byte("unsafe"), 0); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fremovexattr(int(root.Fd()), "com.analytix.injected") }
			case "flags":
				if err := unix.Fchflags(int(root.Fd()), unix.UF_HIDDEN); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchflags(int(root.Fd()), 0) }
			case "mode":
				if err := unix.Fchmod(int(root.Fd()), 0o710); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchmod(int(root.Fd()), 0o700) }
			case "links":
				if err := unix.Mkdirat(int(root.Fd()), "unexpected", 0o700); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Unlinkat(int(root.Fd()), "unexpected", unix.AT_REMOVEDIR) }
			}
			defer restore()
			if err := validatePrivateSnapshotDirectoryExact(int(root.Fd()), identity); !errors.Is(err, ErrRegistry) {
				t.Fatalf("%s drift was accepted: %v", mutation, err)
			}
			restore()
			restore = func() {}
			if err := validatePrivateSnapshotDirectoryExact(int(root.Fd()), identity); err != nil {
				t.Fatalf("restored authority rejected: %v", err)
			}
		})
	}
}

func TestPrivateSnapshotFileRejectsACLXattrFlagsAndMetadataDrift(t *testing.T) {
	for _, mutation := range []string{"acl", "xattr", "flags", "mode", "links"} {
		t.Run(mutation, func(t *testing.T) {
			rootPath := t.TempDir()
			if err := os.Chmod(rootPath, 0o700); err != nil {
				t.Fatal(err)
			}
			root, err := os.Open(rootPath)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			rootIdentity, err := validatePrivateSnapshotDirectory(int(root.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			filePath := filepath.Join(rootPath, "snapshot")
			file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			identity, err := validatePrivateSnapshotFile(
				int(file.Fd()), 0, 1, 0o600, unix.O_RDWR,
				rootIdentity.filesystem, rootIdentity.extendedSecurity,
			)
			if err != nil {
				t.Fatal(err)
			}
			restore := func() {}
			switch mutation {
			case "acl":
				permission := "everyone allow read,readattr,readextattr"
				if output, err := exec.Command("/bin/chmod", "+a", permission, filePath).CombinedOutput(); err != nil {
					t.Fatalf("install ACL fixture: %v: %s", err, output)
				}
				restore = func() { _, _ = exec.Command("/bin/chmod", "-N", filePath).CombinedOutput() }
			case "xattr":
				if err := unix.Fsetxattr(int(file.Fd()), "com.analytix.injected", []byte("unsafe"), 0); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fremovexattr(int(file.Fd()), "com.analytix.injected") }
			case "flags":
				if err := unix.Fchflags(int(file.Fd()), unix.UF_NODUMP); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchflags(int(file.Fd()), 0) }
			case "mode":
				if err := unix.Fchmod(int(file.Fd()), 0o640); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchmod(int(file.Fd()), 0o600) }
			case "links":
				alias := filepath.Join(rootPath, "alias")
				if err := os.Link(filePath, alias); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = os.Remove(alias) }
			}
			defer restore()
			if privateSnapshotFileMatchesExact(int(file.Fd()), identity, 1, 0o600, unix.O_RDWR) {
				t.Fatalf("%s drift was accepted", mutation)
			}
			restore()
			restore = func() {}
			if !privateSnapshotFileMatchesExact(int(file.Fd()), identity, 1, 0o600, unix.O_RDWR) {
				t.Fatal("restored file authority was rejected")
			}
		})
	}
}

func TestPrivateSnapshotUnlinkedFileRetainsExactSecurityAcrossPreallocation(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.Chmod(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.Open(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	rootBefore, err := validatePrivateSnapshotDirectory(int(root.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkdirat(int(root.Fd()), "operation", 0o700); err != nil {
		t.Fatal(err)
	}
	rootActive, err := validatePrivateSnapshotDirectory(int(root.Fd()))
	if err != nil || !validPrivateSnapshotDirectoryCreationTransition(rootBefore, rootActive) {
		t.Fatalf("root creation transition: %#v %#v %v", rootBefore, rootActive, err)
	}
	operationFD, err := unix.Openat(
		int(root.Fd()), "operation", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	operation := os.NewFile(uintptr(operationFD), "operation")
	if operation == nil {
		_ = unix.Close(operationFD)
		t.Fatal("operation authority is nil")
	}
	defer operation.Close()
	defer unix.Unlinkat(int(root.Fd()), "operation", unix.AT_REMOVEDIR)
	operationIdentity, err := validatePrivateSnapshotDirectory(operationFD)
	if err != nil || operationIdentity.filesystem != rootActive.filesystem ||
		operationIdentity.extendedSecurity != rootActive.extendedSecurity {
		t.Fatalf("operation identity: %#v %v", operationIdentity, err)
	}
	createdFD, err := unix.Openat(
		operationFD, privateAccountFlowSnapshotBasename,
		unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600,
	)
	if err != nil {
		t.Fatal(err)
	}
	created := os.NewFile(uintptr(createdFD), "created")
	if created == nil {
		_ = unix.Close(createdFD)
		t.Fatal("created authority is nil")
	}
	defer created.Close()
	createdIdentity, err := validatePrivateSnapshotFile(
		createdFD, 0, 1, 0o600, unix.O_RDWR,
		operationIdentity.filesystem, operationIdentity.extendedSecurity,
	)
	if err != nil {
		t.Fatalf("created identity: %v", err)
	}
	readOnlyFD, err := unix.Openat(
		operationFD, privateAccountFlowSnapshotBasename,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	readOnly := os.NewFile(uintptr(readOnlyFD), "read-only")
	if readOnly == nil {
		_ = unix.Close(readOnlyFD)
		t.Fatal("read-only authority is nil")
	}
	defer readOnly.Close()
	if err := unix.Unlinkat(operationFD, privateAccountFlowSnapshotBasename, 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fsync(operationFD); err != nil {
		t.Fatal(err)
	}
	if empty, err := privateSnapshotDirectoryEmpty(operationFD); err != nil || !empty {
		t.Fatalf("unlinked operation inventory: empty=%v err=%v", empty, err)
	}
	if err := validatePrivateSnapshotDirectoryExact(int(root.Fd()), rootActive); err != nil ||
		validatePrivateSnapshotDirectoryExact(operationFD, operationIdentity) != nil {
		t.Fatalf("directory identity after unlink: %v", err)
	}
	if current, err := validatePrivateSnapshotFile(
		createdFD, 0, 0, 0o600, unix.O_RDWR,
		operationIdentity.filesystem, operationIdentity.extendedSecurity,
	); err != nil || current != createdIdentity {
		t.Fatalf("unlinked created identity: %#v %v", current, err)
	}
	if current, err := validatePrivateSnapshotFile(
		readOnlyFD, 0, 0, 0o600, unix.O_RDONLY,
		operationIdentity.filesystem, operationIdentity.extendedSecurity,
	); err != nil || current != createdIdentity {
		t.Fatalf("unlinked read-only identity: %#v %v", current, err)
	}
	const expectedSize = int64(4096)
	if err := preallocatePrivateAccountFlowSnapshot(
		createdFD, expectedSize, operationIdentity.filesystem,
	); err != nil {
		t.Fatalf("preallocate: %v", err)
	}
	if current, err := validatePrivateSnapshotFile(
		createdFD, 0, 0, 0o600, unix.O_RDWR,
		operationIdentity.filesystem, operationIdentity.extendedSecurity,
	); err != nil || current != createdIdentity {
		t.Fatalf("preallocated created identity: %#v %v", current, err)
	}
}

func TestPrivateSnapshotPostMaterializationRejectsAuthorityDrift(t *testing.T) {
	for _, mutation := range []string{"staging-xattr", "operation-acl", "file-xattr", "file-flags"} {
		t.Run(mutation, func(t *testing.T) {
			snapshotBody := []byte("private snapshot security drift fixture")
			descriptor := privateSnapshotSecurityDescriptor(t, mutation, snapshotBody)
			stagingRoot := t.TempDir()
			if err := os.Chmod(stagingRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			stagingAuthority, err := os.Open(stagingRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer stagingAuthority.Close()
			if err := domainfundsquerysource.ValidateDescriptorV1(descriptor); err != nil {
				t.Fatalf("descriptor validation: %v", err)
			}
			stagingIdentity, err := validatePrivateSnapshotDirectory(int(stagingAuthority.Fd()))
			if err != nil {
				var stat unix.Stat_t
				_ = unix.Fstat(int(stagingAuthority.Fd()), &stat)
				t.Fatalf("staging validation: %v stat=%#o uid=%d gid=%d links=%d flags=%#x", err, stat.Mode, stat.Uid, stat.Gid, stat.Nlink, stat.Flags)
			}
			if err := admitPrivateAccountFlowSnapshotDiskSpace(
				int(stagingAuthority.Fd()), int64(len(snapshotBody)), stagingIdentity.filesystem,
			); err != nil {
				t.Fatalf("disk admission: %v", err)
			}
			lease := &privateSnapshotSecurityLease{body: snapshotBody}
			snapshot, err := materializePrivateAccountFlowSnapshot(
				context.Background(), stagingAuthority, descriptor, lease,
			)
			if err != nil {
				t.Fatalf("materialize secure snapshot: %v copies=%d", err, lease.calls)
			}
			restore := func() {}
			switch mutation {
			case "staging-xattr":
				if err := unix.Fsetxattr(int(stagingAuthority.Fd()), "com.analytix.injected", []byte("unsafe"), 0); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fremovexattr(int(stagingAuthority.Fd()), "com.analytix.injected") }
			case "operation-acl":
				path := filepath.Join(stagingRoot, snapshot.operationDirectoryName)
				permission := "everyone allow list,search,add_file,delete_child"
				if output, err := exec.Command("/bin/chmod", "+a", permission, path).CombinedOutput(); err != nil {
					t.Fatalf("install operation ACL fixture: %v: %s", err, output)
				}
				restore = func() { _, _ = exec.Command("/bin/chmod", "-N", path).CombinedOutput() }
			case "file-xattr":
				if err := unix.Fchmod(int(snapshot.file.Fd()), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := unix.Fsetxattr(int(snapshot.file.Fd()), "com.analytix.injected", []byte("unsafe"), 0); err != nil {
					t.Fatal(err)
				}
				if err := unix.Fchmod(int(snapshot.file.Fd()), 0o400); err != nil {
					t.Fatal(err)
				}
				restore = func() {
					_ = unix.Fchmod(int(snapshot.file.Fd()), 0o600)
					_ = unix.Fremovexattr(int(snapshot.file.Fd()), "com.analytix.injected")
					_ = unix.Fchmod(int(snapshot.file.Fd()), 0o400)
				}
			case "file-flags":
				if err := unix.Fchflags(int(snapshot.file.Fd()), unix.UF_NODUMP); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchflags(int(snapshot.file.Fd()), 0) }
			}
			if err := snapshot.validatePostExecution(); !errors.Is(err, ErrRegistry) {
				t.Fatalf("%s drift was accepted: %v", mutation, err)
			}
			restore()
			if err := snapshot.validatePostExecution(); err != nil {
				t.Fatalf("restored snapshot rejected: %v", err)
			}
			if err := snapshot.settle(); err != nil {
				t.Fatalf("settle restored snapshot: %v", err)
			}
		})
	}
}

type privateSnapshotSecurityLease struct {
	body  []byte
	calls int
}

func (lease *privateSnapshotSecurityLease) CopyExactTo(
	ctx context.Context,
	destination *os.File,
) error {
	if lease == nil || ctx == nil || ctx.Err() != nil || len(lease.body) == 0 ||
		destination == nil || lease.calls != 0 {
		return errors.New("invalid private snapshot security lease")
	}
	lease.calls++
	written, err := destination.WriteAt(lease.body, 0)
	if err != nil {
		return err
	}
	if written != len(lease.body) {
		return errors.New("short private snapshot security write")
	}
	return nil
}

func privateSnapshotSecurityDescriptor(
	t *testing.T,
	name string,
	body []byte,
) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	digest := sha256.Sum256(body)
	caseID := "case-private-snapshot-security-" + name
	datasetSnapshotID := securitytest.DatasetSnapshotID("private-snapshot-security:" + name)
	descriptor, err := domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:                   domainsecurity.SHA256Hex([]byte("snapshot-record:" + name)),
		DatasetSnapshotID:                      datasetSnapshotID,
		SourceManifestHash:                     domainsecurity.SHA256Hex([]byte("source-manifest:" + name)),
		CaseID:                                 caseID,
		CaseBindingHash:                        domainsecurity.SHA256Hex([]byte("case-binding:" + name)),
		DatasetBindingDigest:                   domainsecurity.SHA256Hex([]byte("dataset-binding:" + name)),
		BindingObservationDigest:               domainsecurity.SHA256Hex([]byte("binding-observation:" + name)),
		FundsProducerContentID:                 domainsecurity.FundsProducerContentIDPrefixV1 + strings.Repeat("4", 64),
		FundsProducerContentManifestSHA256:     strings.Repeat("5", 64),
		FundsProducerContentManifestByteLength: 1,
		DuckDBSHA256:                           hex.EncodeToString(digest[:]),
		DuckDBByteLength:                       uint64(len(body)),
		DuckDBContentSnapshotDigest:            strings.Repeat("b", 64),
		DuckDBSnapshotManifestSHA256:           strings.Repeat("c", 64),
		MaterializationIdentity:                "txn_daily_snapshot:v12:" + strings.Repeat("a", 64),
		SchemaDigest:                           domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
		DatasetUTCOffsetMinutes:                0,
		ExpectedCurrency:                       "CNY",
		MinorUnitScale:                         domainfundsquerysource.AccountFlowMinorUnitScaleV1,
		QueryProfileDigest:                     domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func TestPrivateSnapshotDarwinProvenanceRequiresCanonicalNonzeroToken(t *testing.T) {
	valid := []byte{0x01, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0}
	if !validPrivateSnapshotDarwinProvenance(valid) {
		t.Fatal("canonical provenance was rejected")
	}
	for _, invalid := range [][]byte{
		nil,
		{0x01, 0x02, 0x00},
		{0x01, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0},
		{0x01, 0x02, 0x00, 0, 0, 0, 0, 0, 0, 0, 0},
	} {
		if validPrivateSnapshotDarwinProvenance(invalid) {
			t.Fatalf("invalid provenance accepted: %x", invalid)
		}
	}
}
