//go:build !windows

package providerregistryfs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	"golang.org/x/sys/unix"
)

func TestReadLegacySourceFileRejectsPostObservationReplacement(t *testing.T) {
	const scenarioEnv = "ANALYTIX_TEST_LEGACY_SOURCE_SWAP_CASE"
	const rootEnv = "ANALYTIX_TEST_LEGACY_SOURCE_SWAP_ROOT"
	if scenario := os.Getenv(scenarioEnv); scenario != "" {
		parts := strings.Split(scenario, "/")
		if len(parts) != 2 || os.Getenv(rootEnv) == "" {
			t.Fatal("invalid replacement fixture invocation")
		}
		root := os.Getenv(rootEnv)
		path := filepath.Join(root, parts[0]+".json")
		if err := os.WriteFile(path, []byte("original synthetic record"), 0o600); err != nil {
			t.Fatal(err)
		}
		before, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		// Retain the observed inode so replacement cannot reuse its identity.
		held := path + ".held"
		if err := os.Rename(path, held); err != nil {
			t.Fatal(err)
		}
		switch parts[1] {
		case "fifo":
			if err := unix.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
		case "symlink":
			if err := os.Symlink(held, path); err != nil {
				t.Fatal(err)
			}
		case "regular":
			if err := os.WriteFile(path, []byte("replacement synthetic record"), 0o600); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("unknown replacement fixture")
		}
		limit := int64(registryport.MaxLegacySourceSnapshotBytes)
		if parts[0] == "owner" {
			limit = registryport.MaxLegacySourceLockOwnerBytes
		}
		loaded, err := readLegacySourceFile(path, before, limit)
		if !errors.Is(err, registryport.ErrVerification) || len(loaded) != 0 {
			t.Fatal("post-observation replacement returned data or escaped verification")
		}
		return
	}
	for _, kind := range []string{"source", "owner"} {
		for _, replacement := range []string{"fifo", "symlink", "regular"} {
			t.Run(kind+"/"+replacement, func(t *testing.T) {
				// A blocking FIFO open in a regression must not strand a test
				// goroutine or process. The parent owns and removes the fixture.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReadLegacySourceFileRejectsPostObservationReplacement$", "-test.count=1")
				command.Env = append(os.Environ(), scenarioEnv+"="+kind+"/"+replacement, rootEnv+"="+t.TempDir())
				output, err := command.CombinedOutput()
				if ctx.Err() != nil {
					t.Fatal("post-observation replacement blocked the bounded file read")
				}
				if err != nil {
					t.Fatalf("replacement subprocess failed: %v\n%s", err, output)
				}
			})
		}
	}
}
