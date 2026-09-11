//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

func TestPrivateCASRecoveryJournalRejectsUnsafeAuthenticatedResidueLeafTypes(t *testing.T) {
	for _, test := range []struct {
		name   string
		create func(*testing.T, string, []byte)
	}{
		{
			name: "symlink",
			create: func(t *testing.T, path string, body []byte) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, body, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hardlink",
			create: func(t *testing.T, path string, body []byte) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, body, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory",
			create: func(t *testing.T, path string, _ []byte) {
				t.Helper()
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "fifo",
			create: func(t *testing.T, path string, _ []byte) {
				t.Helper()
				if err := unix.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "group readable",
			create: func(t *testing.T, path string, body []byte) {
				t.Helper()
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o640); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "empty",
			create: func(t *testing.T, path string, _ []byte) {
				t.Helper()
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "oversize",
			create: func(t *testing.T, path string, _ []byte) {
				t.Helper()
				body := bytes.Repeat([]byte{'x'}, domainprivatecas.MaxRecoveryJournalRecordBytesV1+1)
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
			fixture.appendCompletedLifecycle(t)
			body, err := os.ReadFile(filepath.Join(fixture.activePath(), privateCASRecoveryManifestFileV1))
			if err != nil {
				t.Fatal(err)
			}
			directory, authority := openPrivateCASRecoveryResidueTestDirectoryV1(t, fixture, false)
			name, err := newPrivateCASRecoveryAuthenticatedTempNameV1(
				directory, authority, privateCASRecoveryManifestFileV1, body,
			)
			if closeErr := directory.Close(); err != nil || closeErr != nil {
				t.Fatal(errors.Join(err, closeErr))
			}
			path := filepath.Join(fixture.activePath(), name)
			test.create(t, path, body)
			if _, err := fixture.journal.Load(context.Background()); err == nil || errors.Is(err, privatecasport.ErrJournalAbsent) {
				t.Fatalf("Load() accepted unsafe %s residue: %v", test.name, err)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("unsafe %s residue was mutated: %v", test.name, err)
			}
		})
	}
}

func TestPrivateCASRecoveryJournalRejectsAuthenticatedResidueFromOtherDirectoryIdentity(t *testing.T) {
	fixture := newPrivateCASRecoveryJournalTestFixtureV1(t)
	fixture.appendCompletedLifecycle(t)
	body, err := os.ReadFile(filepath.Join(fixture.activePath(), privateCASRecoveryManifestFileV1))
	if err != nil {
		t.Fatal(err)
	}
	targets, authority := openPrivateCASRecoveryResidueTestDirectoryV1(t, fixture, true)
	name, err := newPrivateCASRecoveryAuthenticatedTempNameV1(
		targets, authority, privateCASRecoveryManifestFileV1, body,
	)
	if closeErr := targets.Close(); err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	path := filepath.Join(fixture.activePath(), name)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.journal.Load(context.Background()); err == nil || errors.Is(err, privatecasport.ErrJournalAbsent) {
		t.Fatalf("Load() accepted residue signed for another directory identity: %v", err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("cross-directory residue was mutated: %v", err)
	}
}
