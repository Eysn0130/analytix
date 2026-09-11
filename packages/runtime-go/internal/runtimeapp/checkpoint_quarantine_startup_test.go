package runtimeapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func TestRuntimeStartupPreparationQuarantinesLegacyCheckpointSidecarBeforeListenerActivation(t *testing.T) {
	dataDir := t.TempDir()
	durableDir := t.TempDir()
	body := []byte(`{"threadId":"legacy-thread","relativePath":"evidence/bank.csv","bankAccount":"TEST-ACCOUNT-0001"}`)
	digest := sha256.Sum256([]byte("evidence/bank.csv"))
	sourceFile := filepath.Join(
		dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1,
		"legacy-thread", "legacy-checkpoint", hex.EncodeToString(digest[:])+".json",
	)
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceFile, body, 0o600); err != nil {
		t.Fatal(err)
	}

	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableDir}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	leaseClosed := false
	t.Cleanup(func() {
		if !leaseClosed {
			_ = lease.Close()
		}
	})
	prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if err != nil {
		t.Fatalf("pre-listener startup preparation did not quarantine legacy checkpoint sidecar: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)); !os.IsNotExist(err) {
		t.Fatalf("startup preparation completed while legacy checkpoint sidecar remained: %v", err)
	}
	quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	payloadRoot := filepath.Join(quarantineRoot, persistencefs.LegacyCheckpointSnapshotPayloadsV1)
	payloads, err := os.ReadDir(payloadRoot)
	if err != nil || len(payloads) != 1 || !payloads[0].IsDir() ||
		!persistencefs.ValidLegacyCheckpointSnapshotPayloadNameV1(payloads[0].Name()) {
		t.Fatalf("startup preparation did not create one content-bound quarantine payload: payloads=%v err=%v", payloads, err)
	}
	payloadName := payloads[0].Name()
	quarantinedFile := filepath.Join(
		payloadRoot, payloadName, "legacy-thread", "legacy-checkpoint", filepath.Base(sourceFile),
	)
	if got, err := os.ReadFile(quarantinedFile); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("startup preparation changed legacy bytes: got=%q err=%v", got, err)
	}
	handler, err := prepared.Activate(config)
	if err != nil {
		t.Fatalf("prepared runtime did not activate after checkpoint quarantine: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	leaseClosed = true

	if err := filepath.WalkDir(durableDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(content, body) {
			t.Fatalf("legacy quarantine bytes leaked into public durable state: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("idempotent startup after checkpoint quarantine failed: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	payloads, err = os.ReadDir(payloadRoot)
	if err != nil || len(payloads) != 1 || payloads[0].Name() != payloadName {
		t.Fatalf("restart duplicated or replaced quarantine payload: payloads=%v err=%v", payloads, err)
	}
}

func TestRuntimeStartupPreparationRejectsUnsafeLegacyCheckpointBeforeListenerActivation(t *testing.T) {
	dataDir := t.TempDir()
	durableDir := t.TempDir()
	legacyRoot := filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)
	unknown := filepath.Join(legacyRoot, "unknown-sidecar.txt")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unknown, []byte("must remain untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableDir}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease); err == nil {
		t.Fatalf("unsafe legacy checkpoint reached listener-ready startup state: %#v", prepared)
	}
	if body, err := os.ReadFile(unknown); err != nil || string(body) != "must remain untouched" {
		t.Fatalf("failed startup changed unknown legacy bytes: body=%q err=%v", body, err)
	}
	quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if _, err := os.Lstat(quarantineRoot); !os.IsNotExist(err) {
		t.Fatalf("failed read-only preflight created quarantine state: %v", err)
	}
}
