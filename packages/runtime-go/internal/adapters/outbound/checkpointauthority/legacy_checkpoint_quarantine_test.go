package checkpointauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestLegacyCheckpointSnapshotsAreAtomicallyQuarantinedAndRestartIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"threadId":"thread-one","relativePath":"evidence/account.txt","beforeContent":"6222020200000000000"}`)
	sourceFile := writeLegacyCheckpointSnapshotFixture(t, dataDir, original)

	if err := PreflightRecovery(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("read-only legacy checkpoint preflight failed: %v", err)
	}
	if body, err := os.ReadFile(sourceFile); err != nil || !bytes.Equal(body, original) {
		t.Fatalf("preflight mutated legacy bytes: body=%q err=%v", body, err)
	}
	quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if _, err := os.Lstat(quarantineRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight created quarantine state: %v", err)
	}

	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("legacy checkpoint quarantine recovery failed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy authority sidecar remained after quarantine: %v", err)
	}
	state, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), dataDir, access)
	if err != nil || state.Payload == nil {
		t.Fatalf("quarantine payload was not inspectable: state=%#v err=%v", state, err)
	}
	if !strings.HasPrefix(state.Payload.Name, "legacy-v1-"+state.Payload.Tree.SHA256+"-") {
		t.Fatalf("quarantine target is not content-bound: %q", state.Payload.Name)
	}
	if strings.HasSuffix(state.Payload.Name, strings.Repeat("0", 32)) {
		t.Fatalf("quarantine target does not contain an unpredictable nonce: %q", state.Payload.Name)
	}
	payloadFile := filepath.Join(
		quarantineRoot, persistencefs.LegacyCheckpointSnapshotPayloadsV1, state.Payload.Name,
		"thread-one", "checkpoint-one", filepath.Base(sourceFile),
	)
	if body, err := os.ReadFile(payloadFile); err != nil || !bytes.Equal(body, original) {
		t.Fatalf("quarantine did not preserve exact legacy bytes: body=%q err=%v", body, err)
	}
	auditRoot, _ := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	auditCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, access)
	if err != nil {
		t.Fatal(err)
	}
	audits, err := loadLegacyCheckpointSnapshotAudits(context.Background(), auditCAS)
	if err != nil || len(audits) != 1 {
		t.Fatalf("expected one private audit record: audits=%#v err=%v", audits, err)
	}
	if audits[0].AuthorityEligible || audits[0].PublicEventEligible || !audits[0].OriginalBytesPreserved {
		t.Fatalf("legacy audit could be promoted or published: %#v", audits[0])
	}

	firstPayload := state.Payload.Name
	if err := PreflightRecovery(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("restart preflight failed: %v", err)
	}
	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("restart recovery failed: %v", err)
	}
	restarted, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), dataDir, access)
	if err != nil || restarted.Payload == nil || restarted.Payload.Name != firstPayload {
		t.Fatalf("restart was not idempotent: state=%#v err=%v", restarted, err)
	}
	if audits, err = loadLegacyCheckpointSnapshotAudits(context.Background(), auditCAS); err != nil || len(audits) != 1 {
		t.Fatalf("idempotent no-target recovery revoked or changed the live CAS generation: audits=%#v err=%v", audits, err)
	}
	if err := auditCAS.Close(); err != nil {
		t.Fatal(err)
	}
	auditCAS, err = finalauthority.OpenSecurePrivateCASWithAccessAuthority(auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, access)
	if err != nil {
		t.Fatal(err)
	}
	audits, err = loadLegacyCheckpointSnapshotAudits(context.Background(), auditCAS)
	if err != nil || len(audits) != 1 {
		t.Fatalf("restart duplicated private audit state: audits=%#v err=%v", audits, err)
	}
	store, err := NewStoreContext(context.Background(), checkpointRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	intents, err := store.listIntents(context.Background())
	if err != nil || len(intents) != 0 {
		t.Fatalf("legacy sidecar was promoted into checkpoint authority: intents=%#v err=%v", intents, err)
	}
}

func TestLegacyCheckpointMigrationReportsOnlyObservedInventoryChanges(t *testing.T) {
	ctx := context.Background()
	t.Run("empty", func(t *testing.T) {
		dataDir := t.TempDir()
		checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
		access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := PrepareRecoveryV1(ctx, checkpointRoot, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.ValidateSemantics(ctx); err != nil {
			t.Fatal(err)
		}
		changed, err := prepared.ApplyLegacyCheckpointQuarantineMigrationV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Fatal("empty legacy checkpoint inventory reported a mutation")
		}
	})

	t.Run("source-then-idempotent", func(t *testing.T) {
		dataDir := t.TempDir()
		checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
		access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeExisted":true}`))
		prepared, err := PrepareRecoveryV1(ctx, checkpointRoot, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.ValidateSemantics(ctx); err != nil {
			t.Fatal(err)
		}
		changed, err := prepared.ApplyLegacyCheckpointQuarantineMigrationV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("legacy source migration did not report its inventory mutation")
		}

		refreshed, err := PrepareRecoveryV1(ctx, checkpointRoot, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := refreshed.ValidateSemantics(ctx); err != nil {
			t.Fatal(err)
		}
		changed, err = refreshed.ApplyLegacyCheckpointQuarantineMigrationV1(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if changed {
			t.Fatal("completed legacy checkpoint migration reported another mutation")
		}
	})
}

func TestLegacyCheckpointQuarantineResumesMovedPayloadWithoutAudit(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeExisted":true}`))
	state, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(context.Background(), dataDir, access)
	if err != nil || !state.SourcePresent {
		t.Fatalf("legacy source was not inspectable: state=%#v err=%v", state, err)
	}
	auditRoot, _ := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	if _, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, access,
	); err != nil {
		t.Fatal(err)
	}
	target, err := newLegacyCheckpointSnapshotPayloadName(state.Source.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistencefs.MoveLegacyCheckpointSnapshotsToQuarantineV1(
		context.Background(), dataDir, target, state.Source, access,
	); err != nil {
		t.Fatal(err)
	}
	if err := PreflightRecovery(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("moved-without-audit crash state did not preflight: %v", err)
	}
	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("moved-without-audit crash state did not resume: %v", err)
	}
	auditCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, access)
	if err != nil {
		t.Fatal(err)
	}
	audits, err := loadLegacyCheckpointSnapshotAudits(context.Background(), auditCAS)
	if err != nil || len(audits) != 1 || audits[0].PayloadName != target {
		t.Fatalf("resumed audit did not bind moved payload: audits=%#v err=%v", audits, err)
	}
}

func TestLegacyCheckpointQuarantineRejectsLinksUnknownEntriesAndConflictsBeforeMutation(t *testing.T) {
	t.Run("source-symlink", func(t *testing.T) {
		dataDir := t.TempDir()
		checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
		access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		if err := os.Symlink(target, filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if err := PreflightRecovery(context.Background(), checkpointRoot, access); err == nil {
			t.Fatal("legacy checkpoint symlink passed startup preflight")
		}
		if info, err := os.Lstat(filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("failed preflight mutated source symlink: info=%v err=%v", info, err)
		}
	})

	t.Run("unknown-source-entry", func(t *testing.T) {
		dataDir := t.TempDir()
		checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
		access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		sourceRoot := filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)
		if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		unknown := filepath.Join(sourceRoot, "README.txt")
		if err := os.WriteFile(unknown, []byte("not a legacy record"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := PreflightRecovery(context.Background(), checkpointRoot, access); err == nil {
			t.Fatal("unknown legacy checkpoint entry passed startup preflight")
		}
		if body, err := os.ReadFile(unknown); err != nil || string(body) != "not a legacy record" {
			t.Fatalf("failed preflight mutated unknown source: body=%q err=%v", body, err)
		}
	})

	t.Run("existing-quarantine-payload", func(t *testing.T) {
		dataDir := t.TempDir()
		checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
		access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
		if err != nil {
			t.Fatal(err)
		}
		writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeExisted":false}`))
		quarantineRoot, _ := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
		conflictName := "legacy-v1-" + strings.Repeat("0", 64) + "-" + strings.Repeat("0", 32)
		conflict := filepath.Join(quarantineRoot, persistencefs.LegacyCheckpointSnapshotPayloadsV1, conflictName)
		if err := os.MkdirAll(conflict, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := PreflightRecovery(context.Background(), checkpointRoot, access); err == nil {
			t.Fatal("existing quarantine conflict passed startup preflight")
		}
		if _, err := os.Lstat(filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1)); err != nil {
			t.Fatalf("conflict preflight moved legacy source: %v", err)
		}
	})
}

func TestLegacyCheckpointQuarantineRejectsAmbiguousAuditAndCancellation(t *testing.T) {
	dataDir := t.TempDir()
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	writeLegacyCheckpointSnapshotFixture(t, dataDir, []byte(`{"beforeAvailable":true}`))
	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err != nil {
		t.Fatal(err)
	}
	auditRoot, _ := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	auditCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := auditCAS.PutIfAbsent(context.Background(), strings.Repeat("f", 64), []byte(`{"forged":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := PreflightRecovery(context.Background(), checkpointRoot, access); err != nil {
		t.Fatalf("structurally valid audit CAS did not complete read-only owner preflight: %v", err)
	}
	if err := recoverIfPresentV4ForTest(context.Background(), checkpointRoot, access); err == nil {
		t.Fatal("ambiguous private quarantine audit passed pre-listener recovery")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := PreflightRecovery(cancelled, checkpointRoot, access); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled checkpoint preflight did not fail closed: %v", err)
	}
}

func writeLegacyCheckpointSnapshotFixture(t *testing.T, dataDir string, body []byte) string {
	t.Helper()
	digest := sha256.Sum256([]byte("evidence/account.txt"))
	path := filepath.Join(
		dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1,
		"thread-one", "checkpoint-one", hex.EncodeToString(digest[:])+".json",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
