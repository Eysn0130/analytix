//go:build windows

package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSeparateOwnerWindowsUsesSIDAndDACLInsteadOfUnixMode(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	ownerRoot := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, ownerRoot} {
		// Windows ignores the Unix permission bits supplied to Mkdir. The owner
		// authority must therefore be established from SID/DACL state.
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, ok := lease.FrozenSeparateOwnerAuthority(ownerRoot); !ok {
		t.Fatal("SID/DACL-bound Windows owner authority is unavailable")
	}

	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatal("current owner SID is unavailable")
	}
	world, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	if separateOwnerWindowsTrustedSID(world, user.User.Sid) {
		t.Fatal("Everyone SID was accepted as owner authority")
	}
	unsafeDescriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;;GW;;;WD)",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := separateOwnerWindowsRejectUntrustedWriters(unsafeDescriptor, user.User.Sid); err == nil {
		t.Fatal("world-writable DACL was accepted")
	}
}

func TestSeparateOwnerWindowsRejectsAlternateDataStream(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	ownerRoot := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, ownerRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(ownerRoot, "background-tasks.json")
	if err := os.WriteFile(target, []byte("opaque"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+":unexpected", []byte("hidden"), 0o600); err != nil {
		t.Skipf("alternate data streams unavailable: %v", err)
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	authority, ok := lease.FrozenSeparateOwnerAuthority(ownerRoot)
	if !ok {
		t.Fatal("Windows owner authority is unavailable")
	}
	err = lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		directory, err := access.Directory()
		if err != nil {
			return err
		}
		_, _, _, captureErr := directory.CaptureFile(
			context.Background(), "background-tasks.json", 1024, true, false,
		)
		if captureErr == nil {
			return errors.New("alternate data stream was accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSeparateOwnerWindowsProtectsBeforePrivateMove(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	durable := filepath.Join(base, "durable")
	ownerRoot := filepath.Join(base, "owner")
	for _, root := range []string{data, durable, ownerRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	targetName := "background-tasks.json"
	if err := os.WriteFile(filepath.Join(ownerRoot, targetName), []byte("opaque"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := ResolveRootSet(data, durable)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLeaseWithSeparateOwnerRoots(roots, ownerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	authority, ok := lease.FrozenSeparateOwnerAuthority(ownerRoot)
	if !ok {
		t.Fatal("Windows owner authority is unavailable")
	}
	err = lease.WithSeparateOwnerRootAccess(context.Background(), authority, func(access *SeparateOwnerRootAccess) error {
		directory, err := access.Directory()
		if err != nil {
			return err
		}
		initial, _, present, err := directory.CaptureFile(context.Background(), targetName, 1024, false, false)
		if err != nil || !present {
			return errors.Join(errors.New("Windows source capture failed"), err)
		}
		projected, err := directory.ProjectProtectedFileStateExact(context.Background(), targetName, initial)
		if err != nil {
			return err
		}
		protected, err := directory.ProtectFileExact(context.Background(), targetName, initial)
		if err != nil {
			return err
		}
		if protected != projected {
			return errors.New("Windows protected readback differs from the authenticated projection")
		}
		privateDirectory, err := directory.CreatePrivateDirectory(context.Background(), "journal")
		if err != nil {
			return err
		}
		defer privateDirectory.Close()
		if err := directory.MoveFileNoReplace(
			context.Background(), targetName, privateDirectory, "payload", protected, true,
		); err != nil {
			return err
		}
		moved, _, present, err := privateDirectory.CaptureFile(context.Background(), "payload", 1024, false, true)
		if err != nil || !present || moved != protected {
			return errors.Join(errors.New("Windows protected move readback failed"), err)
		}
		payloadPath := filepath.Join(ownerRoot, "journal", "payload")
		aliasPath := filepath.Join(base, "late-alias")
		err = privateDirectory.removeFileExact(context.Background(), "payload", moved, true, func(stage string) error {
			if stage == "before_unlink" {
				return os.Link(payloadPath, aliasPath)
			}
			return nil
		})
		if err == nil {
			return errors.New("Windows late hardlink was accepted as durable absence")
		}
		body, readErr := os.ReadFile(aliasPath)
		if readErr != nil || string(body) != "opaque" {
			return errors.Join(errors.New("Windows late hardlink content changed"), readErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
