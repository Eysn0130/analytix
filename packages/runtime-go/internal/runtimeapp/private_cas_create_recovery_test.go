package runtimeapp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
)

func TestRuntimeCreateResiduesRecoverBeforeSemanticJournal(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	artifactRoot := filepath.Join(dataDir, "private", "report-publication", "artifacts")
	residue := filepath.Join(
		artifactRoot,
		domainprivatecas.CreateDirectoryResidueNameV1("ab"),
	)
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("topology-bound create residue reached semantic journal recovery: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-journal create residue survived runtime restart: %v", err)
	}
}

func TestRuntimeOrphanTopologyRecoveryRemovesOnlyEmptyCanonicalShard(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	caseThreadRoot := filepath.Join(dataDir, "private", "case-thread-authority")
	emptyShard := filepath.Join(caseThreadRoot, "ac")
	if err := os.Mkdir(emptyShard, 0o700); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("empty canonical shard was not recovered before owner preparation: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	if _, err := os.Lstat(emptyShard); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty canonical shard survived runtime restart: %v", err)
	}
}

func TestRuntimeCreateResidueGlobalPreflightPreventsOwnerCleanup(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	digest := "01" + strings.Repeat("a", 62)
	shard := filepath.Join(dataDir, "private", "accepted-finals", "records", digest[:2])
	if err := os.Mkdir(shard, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	recordResidue := filepath.Join(shard, fmt.Sprintf(".%s.json-%024x.tmp", digest, 1))
	if err := os.WriteFile(recordResidue, []byte("owner-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	createResidue := filepath.Join(
		dataDir,
		"private",
		"report-publication",
		domainprivatecas.CreateDirectoryResidueNameV1("artifacts"),
	)
	if err := os.Mkdir(createResidue, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(createResidue, "unsafe"), []byte("unsafe"), 0o600); err != nil {
		t.Fatal(err)
	}

	if restarted, err := NewRuntimeServerHandlerE(config); err == nil {
		shutdownOwnedRuntimeHandler(t, restarted)
		t.Fatal("non-empty create residue passed runtime global preflight")
	}
	for _, retained := range []string{
		recordResidue,
		createResidue,
		filepath.Join(createResidue, "unsafe"),
	} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("failed create-residue preflight partially cleaned runtime state: path=%s err=%v", retained, err)
		}
	}
}

func TestRuntimeUnsignedTransactionPreflightPreventsEmptyCreateResidueCleanup(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	digest := "01" + strings.Repeat("b", 62)
	shard := filepath.Join(dataDir, "private", "accepted-finals", "records", digest[:2])
	if err := os.Mkdir(shard, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	originalName := fmt.Sprintf(".%s.json-%024x.tmp", digest, 2)
	originalPath := filepath.Join(shard, originalName)
	if err := os.WriteFile(originalPath, []byte("unsigned-staged-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	stagedName, ok := domainprivatecas.RecoveryQuarantineNameV1(
		domainprivatecas.ResidueRecoveryStageV1,
		strings.Repeat("c", 64),
		originalName,
		digest[:2],
	)
	if !ok {
		t.Fatal("could not construct unsigned staged residue")
	}
	stagedPath := filepath.Join(shard, stagedName)
	if err := os.Rename(originalPath, stagedPath); err != nil {
		t.Fatal(err)
	}
	createResidue := filepath.Join(
		dataDir,
		"private",
		"report-publication",
		"artifacts",
		domainprivatecas.CreateDirectoryResidueNameV1("ab"),
	)
	if err := os.Mkdir(createResidue, 0o700); err != nil {
		t.Fatal(err)
	}

	if restarted, err := NewRuntimeServerHandlerE(config); err == nil {
		shutdownOwnedRuntimeHandler(t, restarted)
		t.Fatal("unsigned staged residue passed the pre-create-cleanup transaction probe")
	}
	for _, retained := range []string{stagedPath, createResidue} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("unsigned transaction preflight partially cleaned %s: %v", retained, err)
		}
	}
}

func TestRuntimeOrphanTopologyRecoveryRepairsEmptyPartialOwnerBeforeOwnerPreparation(t *testing.T) {
	dataDir := t.TempDir()
	config := Config{
		RuntimeToken:   DefaultRuntimeToken,
		DataDir:        dataDir,
		DurableTempDir: t.TempDir(),
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)

	owner := filepath.Join(dataDir, "private", "accepted-finals")
	missingLeaf := filepath.Join(owner, "dispositions")
	if err := os.Remove(missingLeaf); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("recursive-empty partial owner was not repaired before owner preparation: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	for _, restored := range []string{
		owner,
		filepath.Join(owner, "records"),
		filepath.Join(owner, "dispositions"),
	} {
		info, err := os.Lstat(restored)
		if err != nil || !info.IsDir() {
			t.Fatalf("runtime did not rebuild the recovered owner topology: path=%s info=%v err=%v", restored, info, err)
		}
	}
}
