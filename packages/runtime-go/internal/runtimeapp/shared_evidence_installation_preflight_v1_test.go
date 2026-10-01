package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

func TestDesktopInstallationKeyPreflightPreparesAndReusesCanonicalKey(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "runtime-data")
	durableDir := filepath.Join(base, "runtime-durable")
	userDataDir := filepath.Join(base, "electron-user-data")
	for _, root := range []string{dataDir, durableDir, userDataDir} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: dataDir,
		DurableTempDir: durableDir, UserDataDir: userDataDir,
	}
	keyPath := filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json")
	if _, err := finalauthority.OpenExistingFileAuthority(keyPath); err == nil {
		t.Fatal("existing-only key API accepted an absent authority root")
	}
	if _, err := os.Lstat(filepath.Dir(keyPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("existing-only key API created a missing parent: %v", err)
	}
	if err := RunDesktopInstallationKeyPreflightV1(context.Background(), config); err != nil {
		t.Fatalf("fresh preflight: %v", err)
	}
	initial, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenExistingFileAuthority(keyPath)
	if err != nil || key.KeyID() == "" || len(key.PublicKey()) == 0 {
		t.Fatalf("prepared key is unavailable: %v", err)
	}
	if err := RunDesktopInstallationKeyPreflightV1(context.Background(), config); err != nil {
		t.Fatalf("repeat preflight: %v", err)
	}
	repeated, err := os.ReadFile(keyPath)
	if err != nil || !bytes.Equal(initial, repeated) {
		t.Fatalf("repeat preflight changed installation key: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(userDataDir, "runtime-authority-bootstrap-v1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("key-only preflight published a Main bootstrap: %v", err)
	}

	registryRoot := filepath.Join(dataDir, "private", "evidence-registry")
	if err := os.MkdirAll(filepath.Join(registryRoot, "thread-a"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registryRoot, "thread-a", "turn-a.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := RunDesktopInstallationKeyPreflightV1(context.Background(), config); err == nil {
		t.Fatal("preflight recreated a missing key beside authority state")
	}
	if _, err := os.Lstat(keyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing installation key was recreated: %v", err)
	}
	if _, err := finalauthority.OpenExistingFileAuthority(keyPath); err == nil {
		t.Fatal("existing-only key API created a missing key")
	}
}

func TestDesktopInstallationKeyPreflightRejectsOverlappingOwnerRoot(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "runtime-data")
	durableDir := filepath.Join(base, "runtime-durable")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(durableDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: dataDir,
		DurableTempDir: durableDir, UserDataDir: filepath.Join(dataDir, "electron-user-data"),
	}
	if err := RunDesktopInstallationKeyPreflightV1(context.Background(), config); err == nil {
		t.Fatal("preflight accepted a separate owner inside the Go data root")
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "private")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("overlapping-root preflight changed Go authority state: %v", err)
	}
}
