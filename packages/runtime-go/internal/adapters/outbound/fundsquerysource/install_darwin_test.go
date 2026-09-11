//go:build darwin

package fundsquerysource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

func TestDarwinImmutableSnapshotInstallerRejectsWritableAncestor(t *testing.T) {
	for _, test := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "sticky-writable", mode: 0o777 | os.ModeSticky},
		{name: "private", mode: 0o700},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			ancestor := filepath.Join(base, "ancestor")
			if os.Mkdir(ancestor, 0o700) != nil || os.Chmod(ancestor, test.mode) != nil {
				t.Fatal("prepare task-owned ancestor fixture")
			}
			root := filepath.Join(ancestor, "user-data")
			if os.Mkdir(root, 0o700) != nil {
				t.Fatal("prepare private profile fixture")
			}
			var parentStat, profileStat unix.Stat_t
			if unix.Lstat(ancestor, &parentStat) != nil || unix.Lstat(root, &profileStat) != nil ||
				profileStat.Mode&0o7777 != 0o700 ||
				(test.name == "sticky-writable" && parentStat.Mode&0o7777 != 0o1777) {
				t.Fatal("fixture did not establish the ancestor permission contrast")
			}
			source, err := NewHostExactSource(root)
			if err != nil {
				t.Fatal("construct real immutable installer")
			}
			body := []byte("closed-analytical-ancestor-contract-fixture")
			object := newDarwinImmutableInstallObjectV1(t, "case-ancestor-contract", body)
			input := newDarwinImmutableInstallInputV1(t, base, "input.duckdb", body)
			var inputStat unix.Stat_t
			access, accessErr := unix.FcntlInt(input.Fd(), unix.F_GETFL, 0)
			if unix.Fstat(int(input.Fd()), &inputStat) != nil || inputStat.Dev != profileStat.Dev ||
				inputStat.Uid != uint32(os.Geteuid()) || inputStat.Gid != uint32(os.Getegid()) ||
				inputStat.Mode&0o7777 != 0o400 || inputStat.Nlink != 0 || accessErr != nil ||
				access&unix.O_ACCMODE != unix.O_RDONLY {
				t.Fatal("install input did not satisfy the same-volume sealed-file contract")
			}
			disposition, err := source.InstallExact(context.Background(), object, input)
			if test.name == "sticky-writable" {
				if !errors.Is(err, fundsquerysourceport.ErrUnavailable) || disposition != "" {
					t.Fatal("writable ancestor reached immutable installation")
				}
				if _, err := os.Lstat(filepath.Join(root, dataAnalysisDirectoryV1)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("rejected ancestor created the immutable namespace")
				}
				return
			}
			if err != nil || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 {
				t.Fatal("safe ancestor did not create the immutable object")
			}
			if disposition, err := source.InstallExact(context.Background(), object, input); err != nil ||
				disposition != fundsquerysourceport.ImmutableSnapshotInstallExistingEqualV1 {
				t.Fatal("safe ancestor did not reuse the exact immutable object")
			}
			destination := newDarwinExactDestinationV1(t)
			if err := source.WithExact(context.Background(), darwinSourceDescriptorV1(t, object.CaseID, body),
				func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
					return lease.CopyExactTo(ctx, destination)
				}); err != nil || !bytes.Equal(readDarwinExactDestinationV1(t, destination), body) {
				t.Fatal("safe ancestor immutable object was not readable through WithExact")
			}
		})
	}
}

func TestDarwinImmutableSnapshotInstallerCreatesAndReusesExactObject(t *testing.T) {
	root := newDarwinImmutableInstallRootV1(t)
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("closed-analytical-duckdb-object")
	object := newDarwinImmutableInstallObjectV1(t, "case-install-001", body)
	input := newDarwinImmutableInstallInputV1(t, root, "first.duckdb", body)

	disposition, err := source.InstallExact(context.Background(), object, input)
	if err != nil || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 {
		t.Fatalf("create immutable snapshot disposition=%q err=%v", disposition, err)
	}
	disposition, err = source.InstallExact(context.Background(), object, input)
	if err != nil || disposition != fundsquerysourceport.ImmutableSnapshotInstallExistingEqualV1 {
		t.Fatalf("reuse immutable snapshot disposition=%q err=%v", disposition, err)
	}
	path := filepath.Join(root, dataAnalysisDirectoryV1, casesDirectoryV1, object.CaseID, snapshotsDirectoryV1, object.DuckDBSHA256+".duckdb")
	readback, err := os.ReadFile(path)
	if err != nil || string(readback) != string(body) {
		t.Fatalf("installed immutable snapshot readback=%q err=%v", readback, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o400 {
		t.Fatalf("installed immutable snapshot mode=%v err=%v", info, err)
	}
	descriptor := darwinSourceDescriptorV1(t, object.CaseID, body)
	destination := newDarwinExactDestinationV1(t)
	if err := source.WithExact(
		context.Background(),
		descriptor,
		func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
			return lease.CopyExactTo(ctx, destination)
		},
	); err != nil {
		t.Fatalf("installed immutable snapshot is not readable through the DSV2 source seam: %v", err)
	}
	if readback := readDarwinExactDestinationV1(t, destination); string(readback) != string(body) {
		t.Fatalf("DSV2 source seam changed installed bytes: %q", readback)
	}
}

func TestDarwinImmutableSnapshotPrepublicationSourceIsExactAndCallbackScoped(t *testing.T) {
	root := newDarwinImmutableInstallRootV1(t)
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("closed-prepublication-analytical-duckdb-object")
	object := newDarwinImmutableInstallObjectV1(t, "case-install-prepublication-001", body)
	input := newDarwinImmutableInstallInputV1(t, root, "prepublication.duckdb", body)
	if disposition, installErr := source.InstallExact(context.Background(), object, input); installErr != nil || disposition != fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 {
		t.Fatalf("install immutable snapshot disposition=%q err=%v", disposition, installErr)
	}

	destination := newDarwinExactDestinationV1(t)
	var retained fundsquerysourceport.ExactReadLease
	if err := source.WithInstalledExact(
		context.Background(),
		object,
		func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
			retained = lease
			return lease.CopyExactTo(ctx, destination)
		},
	); err != nil {
		t.Fatalf("installed immutable snapshot is not readable before DSV2 publication: %v", err)
	}
	if readback := readDarwinExactDestinationV1(t, destination); !bytes.Equal(readback, body) {
		t.Fatalf("prepublication source seam changed installed bytes: %q", readback)
	}
	if err := retained.CopyExactTo(context.Background(), destination); !errors.Is(err, fundsquerysourceport.ErrUnavailable) {
		t.Fatalf("retained prepublication lease error=%v", err)
	}

	mismatched := object
	mismatched.CaseID = "case-install-prepublication-002"
	called := false
	if err := source.WithInstalledExact(
		context.Background(),
		mismatched,
		func(context.Context, fundsquerysourceport.ExactReadLease) error {
			called = true
			return nil
		},
	); !errors.Is(err, fundsquerysourceport.ErrNotFound) {
		t.Fatalf("mismatched prepublication object error=%v", err)
	}
	if called {
		t.Fatal("mismatched prepublication object reached consumer callback")
	}
}

func TestDarwinImmutableSnapshotInstallerRejectsMismatchBeforePublication(t *testing.T) {
	root := newDarwinImmutableInstallRootV1(t)
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("closed-analytical-duckdb-object")
	object := newDarwinImmutableInstallObjectV1(t, "case-install-002", []byte("different-object"))
	input := newDarwinImmutableInstallInputV1(t, root, "mismatch.duckdb", body)
	if _, err := source.InstallExact(context.Background(), object, input); !errors.Is(err, fundsquerysourceport.ErrMismatch) {
		t.Fatalf("mismatched immutable snapshot error=%v", err)
	}
	path := filepath.Join(root, dataAnalysisDirectoryV1, casesDirectoryV1, object.CaseID, snapshotsDirectoryV1, object.DuckDBSHA256+".duckdb")
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mismatched immutable snapshot created destination: %v", err)
	}
}

func TestDarwinImmutableSnapshotInstallerNeverReplacesConflictingObject(t *testing.T) {
	root := newDarwinImmutableInstallRootV1(t)
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("closed-analytical-duckdb-object")
	object := newDarwinImmutableInstallObjectV1(t, "case-install-003", body)
	directory := filepath.Join(root, dataAnalysisDirectoryV1, casesDirectoryV1, object.CaseID, snapshotsDirectoryV1)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for current := directory; current != filepath.Dir(root); current = filepath.Dir(current) {
		if err := os.Chmod(current, 0o700); err != nil {
			t.Fatal(err)
		}
		if current == root {
			break
		}
	}
	path := filepath.Join(directory, object.DuckDBSHA256+".duckdb")
	conflict := bytes.Repeat([]byte{'x'}, len(body))
	if err := os.WriteFile(path, conflict, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	input := newDarwinImmutableInstallInputV1(t, root, "conflict.duckdb", body)
	if _, err := source.InstallExact(context.Background(), object, input); !errors.Is(err, fundsquerysourceport.ErrCorrupt) {
		t.Fatalf("conflicting immutable snapshot error=%v", err)
	}
	readback, err := os.ReadFile(path)
	if err != nil || string(readback) != string(conflict) {
		t.Fatalf("conflicting immutable snapshot was replaced: readback=%q err=%v", readback, err)
	}
}

func TestDarwinImmutableSnapshotInstallerRejectsLinkedOrCanceledInput(t *testing.T) {
	root := newDarwinImmutableInstallRootV1(t)
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("closed-analytical-duckdb-object")
	object := newDarwinImmutableInstallObjectV1(t, "case-install-004", body)
	linkedPath := filepath.Join(root, "linked.duckdb")
	if err := os.WriteFile(linkedPath, body, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(linkedPath, 0o400); err != nil {
		t.Fatal(err)
	}
	linked, err := os.Open(linkedPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = linked.Close() })
	if _, err := source.InstallExact(context.Background(), object, linked); !errors.Is(err, fundsquerysourceport.ErrMismatch) {
		t.Fatalf("linked immutable snapshot input error=%v", err)
	}

	input := newDarwinImmutableInstallInputV1(t, root, "canceled.duckdb", body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.InstallExact(ctx, object, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled immutable snapshot install error=%v", err)
	}
}

func newDarwinImmutableInstallRootV1(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "user-data")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func newDarwinImmutableInstallObjectV1(
	t *testing.T,
	caseID string,
	body []byte,
) domainfundsquerysource.ImmutableSnapshotObjectV1 {
	t.Helper()
	digest := sha256.Sum256(body)
	object, err := domainfundsquerysource.NewImmutableSnapshotObjectV1(
		caseID,
		hex.EncodeToString(digest[:]),
		uint64(len(body)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func newDarwinImmutableInstallInputV1(
	t *testing.T,
	root string,
	name string,
	body []byte,
) *os.File {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, body, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}
