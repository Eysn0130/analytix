package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestCheckpointQuarantineKeepsExistingPrivateCASOwnerRecoveryOrder(t *testing.T) {
	owners := runtimePrivateCASOwnerRecoveries("/frozen-data-root", nil)
	names := make([]string, 0, len(owners))
	for _, owner := range owners {
		names = append(names, owner.name)
	}
	want := []string{
		"backend-generation",
		"accepted-finals",
		"case-entity",
		"case-thread-authority",
		"gate-continuations",
		"pending-work",
		"provider-cache-telemetry",
		"turn-terminal-authority",
		"attachment-authority",
		"authority-advance",
		"evidence-authority",
		"evidence-registry",
		"dataset-snapshot-authority",
		"thread-risk-policy",
		"pii-authorization",
		"report-publication",
		"controlled-artifact-access",
		"controlled-artifact-access-v2",
		"checkpoint-authority",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("private CAS owner recovery order changed: got=%v want=%v", names, want)
	}
}

func TestCheckpointPartialLayoutBlocksEarlierOwnerCleanup(t *testing.T) {
	dataDir := t.TempDir()
	privateRoot := filepath.Join(dataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	acceptedRoot := filepath.Join(privateRoot, "accepted-finals")
	if _, err := finalauthority.NewPrivateStore(acceptedRoot, access); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	shard := filepath.Join(acceptedRoot, "records", digest[:2])
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(shard, "."+digest+".json-recovery.tmp")
	if err := os.WriteFile(residue, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpointRoot := filepath.Join(privateRoot, "checkpoint-authority")
	if _, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(checkpointRoot, "snapshot-intents"), 4096, access,
	); err != nil {
		t.Fatal(err)
	}
	if err := recoverRuntimePrivateCASOwners(context.Background(), dataDir, access, nil); err == nil {
		t.Fatal("partial checkpoint authority passed the global owner preflight")
	}
	if body, err := os.ReadFile(residue); err != nil || string(body) != "orphan" {
		t.Fatalf("later-owner failure cleaned an earlier owner residue: body=%q err=%v", body, err)
	}
	if _, err := os.Lstat(filepath.Join(checkpointRoot, "snapshot-completions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial checkpoint authority was repaired during preflight: %v", err)
	}
}
