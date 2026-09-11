//go:build darwin

package processauthority

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInspectSuspendedDarwinExecutableRejectsPathSwapBackWhileCodesignRemainsPIDBound(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()

	executablePath := filepath.Join(t.TempDir(), "analytix-native-inspection-fixture")
	payload, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executablePath, payload, 0o500); err != nil {
		t.Fatal(err)
	}
	entitlements := filepath.Join(t.TempDir(), "empty.plist")
	if err := os.WriteFile(entitlements, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict></dict></plist>
`), 0o400); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(
		"/usr/bin/codesign", "--force", "--sign", "-", "--timestamp=none",
		"--options", "runtime", "--entitlements", entitlements, executablePath,
	).CombinedOutput(); err != nil {
		t.Fatalf("codesign fixture: %v\n%s", err, output)
	}
	opened, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	info, err := opened.Stat()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	digest, err := sha256OpenedFile(ctx, opened)
	if err != nil {
		t.Fatal(err)
	}
	inspected := false
	err = InspectSuspendedDarwinExecutable(ctx, SuspendedInspectionConfig{
		Executable: opened, ExpectedExecutableSHA256: hex.EncodeToString(digest[:]),
		ExpectedExecutableSize: info.Size(), StagingRoot: root, StagingRootAuthority: authority,
	}, func(_ context.Context, identity SuspendedProcessIdentity) error {
		inspected = true
		if len(identity.CDHash) != cdHashBytes*2 {
			t.Fatalf("loaded CDHash = %q", identity.CDHash)
		}
		pid := identity.PID
		pidTarget := strconv.Itoa(pid)
		if output, commandErr := exec.Command("/usr/bin/codesign", "--verify", "--strict", pidTarget).CombinedOutput(); commandErr != nil {
			t.Fatalf("verify suspended process before swap: %v\n%s", commandErr, output)
		}
		assertDynamicCDHash := func(label string) {
			t.Helper()
			output, commandErr := exec.Command("/usr/bin/codesign", "--display", "--verbose=4", "+"+pidTarget).CombinedOutput()
			if commandErr != nil || !strings.Contains(string(output), "Format=pid diskrep") ||
				!strings.Contains(string(output), "CDHash="+identity.CDHash) {
				t.Fatalf("%s dynamic identity mismatch: %v\n%s", label, commandErr, output)
			}
		}
		assertDynamicCDHash("before swap")
		loadedPath, pathErr := loadedDarwinProcessPath(pid)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		retainedPath := loadedPath + ".retained"
		if renameErr := os.Rename(loadedPath, retainedPath); renameErr != nil {
			t.Fatal(renameErr)
		}
		restored := false
		defer func() {
			if !restored {
				_ = os.Remove(loadedPath)
				_ = os.Rename(retainedPath, loadedPath)
			}
		}()
		if writeErr := os.WriteFile(loadedPath, []byte("unsigned replacement"), 0o500); writeErr != nil {
			t.Fatal(writeErr)
		}
		if commandErr := exec.Command("/usr/bin/codesign", "--verify", "--strict", loadedPath).Run(); commandErr == nil {
			t.Fatal("unsigned path replacement unexpectedly passed codesign")
		}
		if output, commandErr := exec.Command("/usr/bin/codesign", "--verify", "--strict", pidTarget).CombinedOutput(); commandErr != nil {
			t.Fatalf("numeric pid inspection followed the swapped path: %v\n%s", commandErr, output)
		}
		assertDynamicCDHash("during swap")
		if removeErr := os.Remove(loadedPath); removeErr != nil {
			t.Fatal(removeErr)
		}
		if renameErr := os.Rename(retainedPath, loadedPath); renameErr != nil {
			t.Fatal(renameErr)
		}
		restored = true
		return nil
	})
	if !errors.Is(err, ErrExecutableIdentity) {
		t.Fatalf("swap-back inspection error = %v", err)
	}
	if !inspected {
		t.Fatal("suspended inspector was not invoked")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inspection staging residue = %#v, err=%v", entries, err)
	}
}
