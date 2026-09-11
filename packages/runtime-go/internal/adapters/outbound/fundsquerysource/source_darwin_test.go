//go:build darwin

package fundsquerysource

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"golang.org/x/sys/unix"
)

func TestDarwinHostSourceCopiesExactImmutableSnapshotOnce(t *testing.T) {
	fixture := newDarwinSourceFixtureV1(t, "case-source-copy")
	destination := newDarwinExactDestinationV1(t)
	var retained fundsquerysourceport.ExactReadLease
	err := fixture.source.WithExact(
		context.Background(),
		fixture.descriptor,
		func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
			retained = lease
			return lease.CopyExactTo(ctx, destination)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readDarwinExactDestinationV1(t, destination), fixture.body) {
		t.Fatal("host source copied different immutable bytes")
	}
	if err := retained.CopyExactTo(context.Background(), destination); !errors.Is(err, fundsquerysourceport.ErrUnavailable) {
		t.Fatalf("retained lease error = %v", err)
	}
}

func TestDarwinExactLeaseAcceptsOnlyConcreteHostFileDestination(t *testing.T) {
	method, found := reflect.TypeOf((*fundsquerysourceport.ExactReadLease)(nil)).Elem().MethodByName("CopyExactTo")
	if !found || method.Type.NumIn() != 2 ||
		method.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() ||
		method.Type.In(1) != reflect.TypeOf((*os.File)(nil)) {
		t.Fatalf("exact lease destination type = %v", method.Type)
	}
}

func TestDarwinHostSourceRejectsAliasesWritableFilesAndWAL(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		fixture := newDarwinSourceFixtureV1(t, "case-source-symlink")
		target := fixture.path + ".target"
		if err := os.Rename(fixture.path, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, fixture.path); err != nil {
			t.Fatal(err)
		}
		assertDarwinSourceRejectedBeforeUseV1(t, fixture)
	})

	t.Run("hardlink", func(t *testing.T) {
		fixture := newDarwinSourceFixtureV1(t, "case-source-hardlink")
		if err := os.Link(fixture.path, fixture.path+".alias"); err != nil {
			t.Fatal(err)
		}
		assertDarwinSourceRejectedBeforeUseV1(t, fixture)
	})

	t.Run("writable", func(t *testing.T) {
		fixture := newDarwinSourceFixtureV1(t, "case-source-writable")
		if err := os.Chmod(fixture.path, 0o600); err != nil {
			t.Fatal(err)
		}
		assertDarwinSourceRejectedBeforeUseV1(t, fixture)
	})

	t.Run("wal", func(t *testing.T) {
		fixture := newDarwinSourceFixtureV1(t, "case-source-wal")
		if err := os.WriteFile(fixture.path+".wal", []byte("pending"), 0o400); err != nil {
			t.Fatal(err)
		}
		assertDarwinSourceRejectedBeforeUseV1(t, fixture)
	})
}

func TestDarwinHostSourceRejectsACLXattrAndBSDFlags(t *testing.T) {
	targets := []struct {
		name string
		path func(darwinSourceFixtureV1) string
	}{
		{name: "protected parent", path: func(fixture darwinSourceFixtureV1) string {
			return filepath.Dir(fixture.path)
		}},
		{name: "snapshot", path: func(fixture darwinSourceFixtureV1) string {
			return fixture.path
		}},
	}
	mutations := []struct {
		name  string
		apply func(*testing.T, string, bool)
	}{
		{name: "acl", apply: func(t *testing.T, path string, directory bool) {
			permission := "everyone allow read"
			if directory {
				permission = "everyone allow list,search,add_file,delete_child"
			}
			if output, err := exec.Command("/bin/chmod", "+a", permission, path).CombinedOutput(); err != nil {
				t.Fatalf("install ACL fixture: %v: %s", err, output)
			}
			t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-N", path).Run() })
		}},
		{name: "xattr", apply: func(t *testing.T, path string, directory bool) {
			if !directory {
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := unix.Setxattr(path, "com.analytix.injected", []byte("unsafe"), 0); err != nil {
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
				_ = unix.Removexattr(path, "com.analytix.injected")
			})
		}},
		{name: "flags", apply: func(t *testing.T, path string, _ bool) {
			if err := unix.Chflags(path, unix.UF_NODUMP); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Chflags(path, 0) })
		}},
	}
	for _, target := range targets {
		for _, mutation := range mutations {
			t.Run(target.name+"/"+mutation.name, func(t *testing.T) {
				fixture := newDarwinSourceFixtureV1(
					t,
					"case-source-"+strings.ReplaceAll(target.name, " ", "-")+"-"+mutation.name,
				)
				path := target.path(fixture)
				mutation.apply(t, path, path != fixture.path)
				assertDarwinSourceRejectedBeforeUseV1(t, fixture)
			})
		}
	}
}

func TestDarwinHostSourceRequiresExactProtectedDirectoryMode(t *testing.T) {
	fixture := newDarwinSourceFixtureV1(t, "case-source-parent-mode")
	if err := os.Chmod(filepath.Dir(fixture.path), 0o750); err != nil {
		t.Fatal(err)
	}
	assertDarwinSourceRejectedBeforeUseV1(t, fixture)
}

func TestDarwinHostSourceRejectsPostCopyReplacement(t *testing.T) {
	fixture := newDarwinSourceFixtureV1(t, "case-source-replace")
	replacement := filepath.Join(filepath.Dir(fixture.path), "replacement.duckdb")
	if err := os.WriteFile(replacement, fixture.body, 0o400); err != nil {
		t.Fatal(err)
	}
	err := fixture.source.WithExact(
		context.Background(),
		fixture.descriptor,
		func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
			if err := lease.CopyExactTo(ctx, newDarwinExactDestinationV1(t)); err != nil {
				return err
			}
			return os.Rename(replacement, fixture.path)
		},
	)
	if !errors.Is(err, fundsquerysourceport.ErrMismatch) {
		t.Fatalf("post-copy replacement error = %v", err)
	}
}

func TestDarwinHostSourceRejectsPostCopyMetadataDrift(t *testing.T) {
	fixture := newDarwinSourceFixtureV1(t, "case-source-post-copy-metadata")
	err := fixture.source.WithExact(
		context.Background(),
		fixture.descriptor,
		func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
			if err := lease.CopyExactTo(ctx, newDarwinExactDestinationV1(t)); err != nil {
				return err
			}
			if err := os.Chmod(fixture.path, 0o600); err != nil {
				return err
			}
			if err := unix.Setxattr(fixture.path, "com.analytix.injected", []byte("unsafe"), 0); err != nil {
				return err
			}
			if err := os.Chmod(fixture.path, 0o400); err != nil {
				return err
			}
			t.Cleanup(func() {
				_ = os.Chmod(fixture.path, 0o600)
				_ = unix.Removexattr(fixture.path, "com.analytix.injected")
			})
			return nil
		},
	)
	if !errors.Is(err, fundsquerysourceport.ErrMismatch) {
		t.Fatalf("post-copy metadata drift error = %v", err)
	}
}

func TestDarwinHostSourceRejectsNonFileAndLinkedDestinationsBeforeCopy(t *testing.T) {
	fixture := newDarwinSourceFixtureV1(t, "case-source-destination-shape")

	t.Run("pipe", func(t *testing.T) {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		err = fixture.source.WithExact(
			context.Background(),
			fixture.descriptor,
			func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
				return lease.CopyExactTo(ctx, writer)
			},
		)
		if !errors.Is(err, fundsquerysourceport.ErrMismatch) {
			t.Fatalf("pipe destination error = %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		if err != nil || len(body) != 0 {
			t.Fatalf("pipe received private bytes: len=%d err=%v", len(body), err)
		}
	})

	t.Run("linked regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "linked-destination")
		destination, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer destination.Close()
		err = fixture.source.WithExact(
			context.Background(),
			fixture.descriptor,
			func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
				return lease.CopyExactTo(ctx, destination)
			},
		)
		if !errors.Is(err, fundsquerysourceport.ErrMismatch) {
			t.Fatalf("linked destination error = %v", err)
		}
		info, statErr := destination.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() != 0 {
			t.Fatalf("linked destination received private bytes: size=%d", info.Size())
		}
	})
}

func TestDarwinHostSourceRejectsUnsafeDestinationStateWithoutCopy(t *testing.T) {
	fixture := newDarwinSourceFixtureV1(t, "case-source-unsafe-destination")
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *os.File)
	}{
		{
			name: "nonempty",
			mutate: func(t *testing.T, file *os.File) {
				if _, err := file.WriteAt([]byte("caller-state"), 0); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "mode",
			mutate: func(t *testing.T, file *os.File) {
				if err := file.Chmod(0o400); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "xattr",
			mutate: func(t *testing.T, file *os.File) {
				if err := unix.Fsetxattr(int(file.Fd()), "com.analytix.injected", []byte("unsafe"), 0); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "BSD flags",
			mutate: func(t *testing.T, file *os.File) {
				if err := unix.Fchflags(int(file.Fd()), unix.UF_NODUMP); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			destination := newDarwinExactDestinationV1(t)
			test.mutate(t, destination)
			err := fixture.source.WithExact(
				context.Background(),
				fixture.descriptor,
				func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
					return lease.CopyExactTo(ctx, destination)
				},
			)
			if !errors.Is(err, fundsquerysourceport.ErrMismatch) {
				t.Fatalf("unsafe destination error = %v", err)
			}
			info, statErr := destination.Stat()
			if statErr != nil {
				t.Fatal(statErr)
			}
			if test.name == "nonempty" {
				if got := readDarwinExactDestinationV1(t, destination); string(got) != "caller-state" {
					t.Fatalf("preexisting caller bytes changed: %q", got)
				}
			} else if info.Size() != 0 {
				t.Fatalf("unsafe destination received private bytes: size=%d", info.Size())
			}
		})
	}
}

func TestDarwinHostSourceCancellationAndHashFailureLeaveNoPrivateBytes(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		fixture := newDarwinSourceFixtureV1(t, "case-source-copy-cancel")
		destination := newDarwinExactDestinationV1(t)
		ctx, cancel := context.WithCancel(context.Background())
		err := fixture.source.WithExact(
			ctx,
			fixture.descriptor,
			func(_ context.Context, lease fundsquerysourceport.ExactReadLease) error {
				cancel()
				return lease.CopyExactTo(context.Background(), destination)
			},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled copy error = %v", err)
		}
		info, statErr := destination.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() != 0 {
			t.Fatalf("cancelled destination size=%d", info.Size())
		}
	})

	t.Run("cancellation during bounded copy", func(t *testing.T) {
		body := bytes.Repeat([]byte{0x5a}, darwinExactCopyChunkBytesV1*3)
		fixture := newDarwinSourceFixtureWithBodyV1(t, "case-source-copy-mid-cancel", body)
		destination := newDarwinExactDestinationV1(t)
		ctx := newDarwinSteppedCancellationContextV1()
		err := fixture.source.WithExact(
			ctx,
			fixture.descriptor,
			func(_ context.Context, lease fundsquerysourceport.ExactReadLease) error {
				ctx.cancelAfterAdditionalChecks(4)
				return lease.CopyExactTo(context.Background(), destination)
			},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("mid-copy cancellation error = %v", err)
		}
		info, statErr := destination.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() != 0 {
			t.Fatalf("mid-copy cancellation retained private bytes: size=%d", info.Size())
		}
	})

	t.Run("operation cancellation during bounded copy", func(t *testing.T) {
		body := bytes.Repeat([]byte{0x6b}, darwinExactCopyChunkBytesV1*3)
		fixture := newDarwinSourceFixtureWithBodyV1(t, "case-source-copy-operation-cancel", body)
		destination := newDarwinExactDestinationV1(t)
		operationContext := newDarwinSteppedCancellationContextV1()
		err := fixture.source.WithExact(
			context.Background(),
			fixture.descriptor,
			func(_ context.Context, lease fundsquerysourceport.ExactReadLease) error {
				operationContext.cancelAfterAdditionalChecks(4)
				return lease.CopyExactTo(operationContext, destination)
			},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("mid-copy operation cancellation error = %v", err)
		}
		info, statErr := destination.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() != 0 {
			t.Fatalf("operation-cancelled destination retained private bytes: size=%d", info.Size())
		}
	})

	t.Run("source digest mismatch after complete write", func(t *testing.T) {
		fixture := newDarwinSourceFixtureV1(t, "case-source-copy-scrub")
		mutated := append([]byte(nil), fixture.body...)
		mutated[0] ^= 0xff
		if err := os.Chmod(fixture.path, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixture.path, mutated, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(fixture.path, 0o400); err != nil {
			t.Fatal(err)
		}
		destination := newDarwinExactDestinationV1(t)
		err := fixture.source.WithExact(
			context.Background(),
			fixture.descriptor,
			func(ctx context.Context, lease fundsquerysourceport.ExactReadLease) error {
				return lease.CopyExactTo(ctx, destination)
			},
		)
		if !errors.Is(err, fundsquerysourceport.ErrMismatch) {
			t.Fatalf("digest mismatch error = %v", err)
		}
		info, statErr := destination.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() != 0 {
			t.Fatalf("failed exact copy retained private bytes: size=%d", info.Size())
		}
	})
}

func TestDarwinDestinationIdentityDriftFailsClosed(t *testing.T) {
	destination := newDarwinExactDestinationV1(t)
	pinnedFD, identity, err := openPinnedDarwinDestinationV1(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(pinnedFD)
	if err := unix.Fchmod(pinnedFD, 0o400); err != nil {
		t.Fatal(err)
	}
	defer unix.Fchmod(pinnedFD, 0o600)
	if err := verifyDarwinDestinationV1(pinnedFD, 0, identity); !errors.Is(err, fundsquerysourceport.ErrMismatch) {
		t.Fatalf("destination identity drift error = %v", err)
	}
}

func TestDarwinSourceFilesystemPolicyRequiresOwnershipEnabledWritableLocalAPFS(t *testing.T) {
	var safe unix.Statfs_t
	copy(safe.Fstypename[:], "apfs")
	safe.Flags = unix.MNT_LOCAL
	if !darwinSourceFilesystemSafeV1(safe) {
		t.Fatal("ownership-enabled writable local APFS was rejected")
	}

	notAPFS := safe
	clear(notAPFS.Fstypename[:])
	copy(notAPFS.Fstypename[:], "hfs")
	notLocal := safe
	notLocal.Flags &^= unix.MNT_LOCAL
	for name, info := range map[string]unix.Statfs_t{
		"non APFS":         notAPFS,
		"non local":        notLocal,
		"read only":        darwinSourceStatfsWithFlagV1(safe, unix.MNT_RDONLY),
		"ignore ownership": darwinSourceStatfsWithFlagV1(safe, unix.MNT_IGNORE_OWNERSHIP),
		"automounted":      darwinSourceStatfsWithFlagV1(safe, unix.MNT_AUTOMOUNTED),
		"union":            darwinSourceStatfsWithFlagV1(safe, unix.MNT_UNION),
		"snapshot":         darwinSourceStatfsWithFlagV1(safe, unix.MNT_SNAPSHOT),
	} {
		t.Run(name, func(t *testing.T) {
			if darwinSourceFilesystemSafeV1(info) {
				t.Fatal("unsafe funds source filesystem was accepted")
			}
		})
	}
}

func TestDarwinSourceFilesystemNameAndProvenanceAreClosed(t *testing.T) {
	var canonical [16]byte
	copy(canonical[:], "apfs")
	if name, ok := darwinSourceFilesystemNameV1(canonical); !ok || name != "apfs" {
		t.Fatalf("canonical filesystem name = %q, %v", name, ok)
	}
	unterminated := [16]byte{'a', 'p', 'f', 's', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x'}
	uppercase := canonical
	uppercase[0] = 'A'
	residue := canonical
	residue[5] = 'x'
	for name, raw := range map[string][16]byte{
		"unterminated": unterminated,
		"uppercase":    uppercase,
		"residue":      residue,
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := darwinSourceFilesystemNameV1(raw); ok {
				t.Fatal("unsafe filesystem name was accepted")
			}
		})
	}

	valid := []byte{0x01, 0x02, 0x00, 1, 2, 3, 4, 5, 6, 7, 8}
	if !validDarwinSourceProvenanceV1(valid) {
		t.Fatal("trusted Darwin provenance shape was rejected")
	}
	for name, value := range map[string][]byte{
		"short":       valid[:len(valid)-1],
		"version":     append([]byte{0x02}, valid[1:]...),
		"record type": append([]byte{0x01, 0x03}, valid[2:]...),
		"reserved":    append([]byte{0x01, 0x02, 0x01}, valid[3:]...),
		"zero token":  {0x01, 0x02, 0x00, 0, 0, 0, 0, 0, 0, 0, 0},
	} {
		t.Run("provenance/"+name, func(t *testing.T) {
			if validDarwinSourceProvenanceV1(value) {
				t.Fatalf("invalid provenance was accepted: %x", value)
			}
		})
	}
}

func TestDarwinHostSourceMissingCapabilityDoesNotAffectOrdinaryRuntime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "user-data")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := darwinSourceDescriptorV1(t, "case-source-missing", []byte("missing"))
	called := false
	err = source.WithExact(
		context.Background(), descriptor,
		func(context.Context, fundsquerysourceport.ExactReadLease) error {
			called = true
			return nil
		},
	)
	if !errors.Is(err, fundsquerysourceport.ErrNotFound) || called {
		t.Fatalf("missing source crossed callback: called=%v err=%v", called, err)
	}
}

type darwinSourceFixtureV1 struct {
	source     *Source
	descriptor domainfundsquerysource.DescriptorV1
	body       []byte
	path       string
}

func newDarwinExactDestinationV1(t *testing.T) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exact-destination")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 0 ||
		stat.Mode&0o7777 != 0o600 || stat.Uid != uint32(os.Geteuid()) {
		_ = file.Close()
		t.Fatalf("unsafe exact destination fixture: %#v err=%v", stat, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func readDarwinExactDestinationV1(t *testing.T, file *os.File) []byte {
	t.Helper()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, info.Size())
	if len(body) == 0 {
		return body
	}
	read, err := file.ReadAt(body, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if read != len(body) {
		t.Fatalf("exact destination short read: got=%d want=%d", read, len(body))
	}
	return body
}

func newDarwinSourceFixtureV1(t *testing.T, caseID string) darwinSourceFixtureV1 {
	t.Helper()
	body := []byte("immutable-DuckDB-snapshot:" + caseID)
	return newDarwinSourceFixtureWithBodyV1(t, caseID, body)
}

func newDarwinSourceFixtureWithBodyV1(
	t *testing.T,
	caseID string,
	body []byte,
) darwinSourceFixtureV1 {
	t.Helper()
	root := filepath.Join(t.TempDir(), "user-data")
	body = append([]byte(nil), body...)
	descriptor := darwinSourceDescriptorV1(t, caseID, body)
	directory := filepath.Join(
		root,
		dataAnalysisDirectoryV1,
		casesDirectoryV1,
		caseID,
		snapshotsDirectoryV1,
	)
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
	path := filepath.Join(directory, immutableSnapshotFileNameV1(descriptor))
	if err := os.WriteFile(path, body, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	source, err := NewHostExactSource(root)
	if err != nil {
		t.Fatal(err)
	}
	return darwinSourceFixtureV1{source: source, descriptor: descriptor, body: body, path: path}
}

type darwinSteppedCancellationContextV1 struct {
	mu       sync.Mutex
	checks   int
	cancelAt int
	done     chan struct{}
	canceled bool
}

func newDarwinSteppedCancellationContextV1() *darwinSteppedCancellationContextV1 {
	return &darwinSteppedCancellationContextV1{done: make(chan struct{})}
}

func (ctx *darwinSteppedCancellationContextV1) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (ctx *darwinSteppedCancellationContextV1) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *darwinSteppedCancellationContextV1) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.checks++
	if !ctx.canceled && ctx.cancelAt > 0 && ctx.checks >= ctx.cancelAt {
		ctx.canceled = true
		close(ctx.done)
	}
	if ctx.canceled {
		return context.Canceled
	}
	return nil
}

func (*darwinSteppedCancellationContextV1) Value(any) any {
	return nil
}

func (ctx *darwinSteppedCancellationContextV1) cancelAfterAdditionalChecks(additional int) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.cancelAt = ctx.checks + additional
}

func darwinSourceDescriptorV1(
	t *testing.T,
	caseID string,
	body []byte,
) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	digest := func(seed string) string {
		return domainsecurity.SHA256Hex([]byte("darwin-host-source:\x00" + seed))
	}
	descriptor, err := domainfundsquerysource.NewDescriptorV1(
		domainfundsquerysource.DescriptorInputV1{
			SnapshotRecordDigest:     digest("snapshot-record:" + caseID),
			DatasetSnapshotID:        domainsecurity.DatasetSnapshotIDPrefixV2 + digest("snapshot:"+caseID),
			SourceManifestHash:       digest("source-manifest:" + caseID),
			CaseID:                   caseID,
			CaseBindingHash:          digest("case-binding:" + caseID),
			DatasetBindingDigest:     digest("dataset-binding:" + caseID),
			BindingObservationDigest: digest("observation:" + caseID),
			FundsProducerContentID: domainsecurity.FundsProducerContentIDPrefixV1 +
				digest("producer:"+caseID),
			FundsProducerContentManifestSHA256:     digest("producer-manifest:" + caseID),
			FundsProducerContentManifestByteLength: 1024,
			DuckDBSHA256:                           domainsecurity.SHA256Hex(body),
			DuckDBByteLength:                       uint64(len(body)),
			DuckDBContentSnapshotDigest:            digest("content-snapshot:" + caseID),
			DuckDBSnapshotManifestSHA256:           digest("duckdb-manifest:" + caseID),
			MaterializationIdentity:                "txn_daily_snapshot:v12:" + digest("source-signature:"+caseID),
			SchemaDigest:                           domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
			DatasetUTCOffsetMinutes:                0,
			ExpectedCurrency:                       "CNY",
			MinorUnitScale:                         domainfundsquerysource.AccountFlowMinorUnitScaleV1,
			QueryProfileDigest:                     domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func assertDarwinSourceRejectedBeforeUseV1(t *testing.T, fixture darwinSourceFixtureV1) {
	t.Helper()
	called := false
	err := fixture.source.WithExact(
		context.Background(), fixture.descriptor,
		func(context.Context, fundsquerysourceport.ExactReadLease) error {
			called = true
			return nil
		},
	)
	if err == nil || called ||
		(!errors.Is(err, fundsquerysourceport.ErrMismatch) &&
			!errors.Is(err, fundsquerysourceport.ErrUnavailable)) ||
		strings.Contains(err.Error(), fixture.path) {
		t.Fatalf("unsafe source result: called=%v err=%v", called, err)
	}
}

func darwinSourceStatfsWithFlagV1(info unix.Statfs_t, flag uint32) unix.Statfs_t {
	info.Flags |= flag
	return info
}
