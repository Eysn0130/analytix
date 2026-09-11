package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestLegacyCheckpointQuarantineMoveRejectsForgedTreeBeforeMutation(t *testing.T) {
	dataDir := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	recordDigest := sha256.Sum256([]byte("evidence.txt"))
	sourceFile := filepath.Join(
		dataDir, LegacyCheckpointSnapshotDirectoryV1, "thread-one", "checkpoint-one",
		hex.EncodeToString(recordDigest[:])+".json",
	)
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceFile, []byte(`{"beforeContent":"exact"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), dataDir, access)
	if err != nil || !state.SourcePresent {
		t.Fatalf("legacy source inventory failed: state=%#v err=%v", state, err)
	}
	forged := state.Source
	forged.FileCount++
	target := "legacy-v1-" + forged.SHA256 + "-" + strings.Repeat("1", 32)
	if _, err := MoveLegacyCheckpointSnapshotsToQuarantineV1(
		context.Background(), dataDir, target, forged, access,
	); err == nil {
		t.Fatal("forged aggregate metadata moved legacy checkpoint bytes")
	}
	if body, err := os.ReadFile(sourceFile); err != nil || string(body) != `{"beforeContent":"exact"}` {
		t.Fatalf("rejected move changed legacy bytes: body=%q err=%v", body, err)
	}
	quarantineRoot, _ := LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if _, err := os.Lstat(quarantineRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected move created quarantine state: %v", err)
	}
}

func TestLegacyCheckpointQuarantineSupportsEqualDataAndDurableRoot(t *testing.T) {
	root := t.TempDir()
	roots, err := ResolveRootSet(root, root)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	// Exercise the caller-visible macOS /var alias as well as the frozen
	// /private/var authority path. The quarantine must bind the canonical root
	// without weakening descendant or identity checks.
	if _, err := InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), root, lease); err != nil {
		t.Fatalf("equal data/durable root did not authorize private quarantine: %v", err)
	}
}

func TestLegacyCheckpointQuarantineProvesFrozenColdRootAbsence(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "cold", "data")
	roots, err := ResolveRootSet(root, root)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if state, err := InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), root, lease); err != nil || state.SourcePresent {
		t.Fatalf("frozen cold root absence was not accepted: state=%#v err=%v", state, err)
	}
	if err := os.Mkdir(filepath.Join(base, "cold"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), root, lease); err == nil {
		t.Fatal("partially materialized cold root bypassed legacy checkpoint preflight")
	}
}
