//go:build !analytix_prod

package runtimeapp

import (
	"context"
	"path/filepath"
	"testing"

	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func TestCheckpointQuarantineSupportsEqualDataAndCandidateDurableRoot(t *testing.T) {
	root := t.TempDir()
	config := Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: root, CandidateDurableRoot: root,
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), root, lease); err != nil {
		t.Fatalf("equal-root direct quarantine inspection failed: %v", err)
	}
	checkpointRoot := filepath.Join(root, "private", "checkpoint-authority")
	if err := checkpointauthority.PreflightRecovery(context.Background(), checkpointRoot, lease); err != nil {
		t.Fatalf("equal-root checkpoint preflight rejected quarantine: %v", err)
	}
	if err := recoverRuntimePrivateCASOwners(context.Background(), root, lease, nil); err != nil {
		t.Fatalf("equal-root private CAS recovery rejected checkpoint quarantine: %v", err)
	}
	if _, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease); err != nil {
		t.Fatalf("equal-root startup preparation rejected checkpoint quarantine: %v", err)
	}
}
