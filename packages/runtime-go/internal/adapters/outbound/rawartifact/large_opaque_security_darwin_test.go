//go:build darwin

package rawartifact_test

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	rawartifactadapter "analytix.local/runtime-go/internal/adapters/outbound/rawartifact"
	"golang.org/x/sys/unix"
)

func TestLargeOpaqueDarwinRejectsExtendedSecurityAtEveryAuthorityLevel(t *testing.T) {
	mutations := []struct {
		name  string
		apply func(*testing.T, string, bool)
	}{
		{name: "unexpected-xattr", apply: func(t *testing.T, path string, directory bool) {
			const name = "com.analytix.large-opaque-test"
			if !directory {
				if err := unix.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := unix.Setxattr(path, name, []byte("unexpected"), 0); err != nil {
				if !directory {
					_ = unix.Chmod(path, 0o400)
				}
				t.Fatal(err)
			}
			if !directory {
				if err := unix.Chmod(path, 0o400); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if !directory {
					_ = unix.Chmod(path, 0o600)
				}
				_ = unix.Removexattr(path, name)
				if !directory {
					_ = unix.Chmod(path, 0o400)
				}
			})
		}},
		{name: "bsd-flags", apply: func(t *testing.T, path string, _ bool) {
			if err := unix.Chflags(path, unix.UF_NODUMP); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unix.Chflags(path, 0) })
		}},
		{name: "extended-acl", apply: func(t *testing.T, path string, directory bool) {
			permission := "everyone allow read,write"
			if directory {
				permission = "everyone allow list,search,add_file,delete_child"
			}
			if output, err := exec.Command("/bin/chmod", "+a", permission, path).CombinedOutput(); err != nil {
				t.Fatalf("install extended ACL: %v: %s", err, output)
			}
			t.Cleanup(func() { _, _ = exec.Command("/bin/chmod", "-N", path).CombinedOutput() })
		}},
	}
	for _, mutation := range mutations {
		for _, level := range []string{"root", "shard", "blob"} {
			t.Run(mutation.name+"-"+level, func(t *testing.T) {
				store, root, lease := openLargeOpaqueChunkStore(t)
				body := bytes.Repeat([]byte("darwin-extended-security"), 256)
				descriptor := buildLargeOpaqueDescriptor(t, mutation.name+"-"+level, body)
				seedLargeOpaqueCommittedForRecovery(t, store, root, descriptor, body)
				assertLargeOpaqueRead(t, store, descriptor, body)
				prepared, err := rawartifactadapter.PrepareLargeOpaqueChunkStoreRecoveryV1(
					context.Background(), lease,
				)
				if err != nil {
					t.Fatalf("safe baseline fixture failed recovery preflight: %v", err)
				}
				if _, guard, err := prepared.Apply(context.Background()); err != nil || guard == nil {
					t.Fatalf("safe baseline fixture failed recovery apply: guard=%v err=%v", guard != nil, err)
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
