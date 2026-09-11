package evidenceregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPrivateEvidenceRegistryPersistsReplaysAndRevokes(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	draft := storeTestDraft(t, securityContext, material)
	rawProviderID := "provider_call_6222020202020202020"
	unsafe := draft
	unsafe.ToolCallID = rawProviderID
	if receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: unsafe, CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err == nil || receipt.ReceiptID != "" || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider identity reached private evidence registry: receipt=%#v err=%v", receipt, err)
	}
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := store.registryPath(securityContext)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private registry permissions mismatch: info=%#v err=%v", info, err)
	}
	restarted, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := domainevidence.CanonicalEvidenceBytes(material)
	resolved, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID})
	if err != nil || string(resolved.CanonicalEvidence) != string(canonical) {
		t.Fatalf("receipt did not survive restart replay: resolved=%#v err=%v", resolved, err)
	}
	revokeInput := registryport.RevokeInput{
		Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
	}
	if err := restarted.Revoke(context.Background(), revokeInput); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revoke(context.Background(), revokeInput); err != nil {
		t.Fatalf("exact revocation retry was not idempotent: %v", err)
	}
	if replayed, err := restarted.Replay(context.Background(), securityContext); err != nil || replayed.Sequence != 2 {
		t.Fatalf("revocation retry appended a duplicate entry: sequence=%d err=%v", replayed.Sequence, err)
	}
	conflict := revokeInput
	conflict.ReasonCode = "different_reason"
	if err := restarted.Revoke(context.Background(), conflict); err == nil {
		t.Fatal("conflicting revocation retry was accepted")
	}
	if _, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID}); err == nil {
		t.Fatal("revoked receipt resolved after restart")
	}
}

func TestWitnessedAuthorityActivationRejectsGenerationZeroAndLegacyLineage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".registry.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil || prepared.ValidateSemantics(context.Background()) != nil || prepared.WitnessedV2ActivationAllowed() {
		t.Fatalf("empty legacy lock entered witnessed V2 activation: prepared=%#v err=%v", prepared, err)
	}
	legacy := filepath.Join(root, "thread-legacy")
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	prepared, err = PrepareRecoveryV2(context.Background(), root, access)
	if err != nil || prepared.ValidateSemantics(context.Background()) == nil || prepared.WitnessedV2ActivationAllowed() {
		t.Fatal("legacy registry lineage entered witnessed authority")
	}
	if err := os.Remove(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "indexes"), 0o700); err != nil {
		t.Fatal(err)
	}
	prepared, err = PrepareRecoveryV2(context.Background(), root, access)
	if err != nil || prepared.ValidateSemantics(context.Background()) == nil || prepared.WitnessedV2ActivationAllowed() {
		t.Fatal("partial V2 CAS leaf pair entered witnessed authority")
	}
}

func TestFreshImportInventoryDoesNotChangeRecoveryActivation(t *testing.T) {
	for _, name := range []string{"absent", "empty owner", "empty pair", "legacy lock", "partial", "unknown", "symlink"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "registry")
			access, err := privatecastest.NewAccessAuthority(parent)
			if err != nil {
				t.Fatal(err)
			}
			if name != "absent" {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "empty pair":
				index, err := NewAuthorityIndexStoreV2(filepath.Join(root, "indexes"), access)
				if err != nil {
					t.Fatal(err)
				}
				defer index.Close()
				capsule, err := NewAuthorityCapsuleStoreV2(filepath.Join(root, "capsules"), access)
				if err != nil {
					t.Fatal(err)
				}
				defer capsule.Close()
			case "legacy lock":
				if err := os.WriteFile(filepath.Join(root, ".registry.lock"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "partial":
				if err := os.Mkdir(filepath.Join(root, "indexes"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "unknown":
				if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("retained"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(parent, filepath.Join(root, "indexes")); err != nil {
					t.Fatal(err)
				}
			}
			allowed := name == "absent" || name == "empty owner" || name == "empty pair"
			prepared, err := PrepareRecoveryV2(context.Background(), root, access)
			if err != nil {
				if allowed {
					t.Fatal(err)
				}
				return
			}
			semanticErr := prepared.ValidateSemantics(context.Background())
			if prepared.WitnessedV2ActivationAllowed() {
				t.Fatal("generation zero gained startup recovery authority")
			}
			if got := semanticErr == nil && prepared.FreshImportInventoryV2(); got != allowed {
				t.Fatalf("fresh physical prerequisite=%t want=%t", got, allowed)
			}
		})
	}
}

func TestRecoveryV2FreezesFixedV1FamilyWithoutPromotingIt(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".registry-capsules", ".registry-projections"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	indexBody := []byte(`{"schemaVersion":1,"purpose":"analytix.evidence-registry-index/v1"}`)
	if err := os.WriteFile(filepath.Join(root, ".registry-authority-index.json"), indexBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".registry.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatalf("fixed V1 family was not frozen by global recovery: %v", err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if prepared.WitnessedV2ActivationAllowed() {
		t.Fatal("fixed V1 family entered witnessed V2 activation")
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(filepath.Join(root, ".registry-authority-index.json"))
	if err != nil || !bytes.Equal(retained, indexBody) {
		t.Fatalf("V1 family freeze changed the authority index: %v", err)
	}
	for _, leaf := range []string{"indexes", "capsules"} {
		if _, err := os.Lstat(filepath.Join(root, leaf)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("global V1 freeze manufactured V2 leaf %s: %v", leaf, err)
		}
	}
}

func TestRecoveryV2LegacyFamilyMatchesCanonicalOwnerCatalog(t *testing.T) {
	var owner domainprivatecas.OwnerDirectoryGroupV1
	for _, candidate := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
		if candidate.RecoveryGroupID == "evidence-registry" {
			owner = candidate
			break
		}
	}
	if owner.OwnerRelativePath != "private/evidence-registry" ||
		!slices.Equal(owner.LeafComponents, []string{domainprivatecas.EvidenceRegistryCapsulesLeafV2, domainprivatecas.EvidenceRegistryIndexesLeafV2}) ||
		!slices.Equal(owner.FrozenDirectoryComponents, []string{domainprivatecas.EvidenceRegistryLegacyCapsulesLeafV1, domainprivatecas.EvidenceRegistryLegacyProjectionsV1}) ||
		!slices.Equal(owner.RegularSiblingComponents, []string{domainprivatecas.EvidenceRegistryLegacyAuthorityIndexV1, domainprivatecas.EvidenceRegistryLegacyLockV1}) {
		t.Fatalf("evidence registry recovery family drifted from canonical owner catalog: %#v", owner)
	}
}

func TestHasStateUsesExactOwnerContainerPresence(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		seed  func(*testing.T, string)
		state bool
	}{
		{name: "absent", state: false},
		{name: "empty-root", state: true, seed: func(t *testing.T, root string) { t.Helper(); mustMkdirRegistryTestV2(t, root) }},
		{name: "indexes-only", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, filepath.Join(root, "indexes"))
		}},
		{name: "capsules-only", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, filepath.Join(root, "capsules"))
		}},
		{name: "empty-pair", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, filepath.Join(root, "indexes"))
			mustMkdirRegistryTestV2(t, filepath.Join(root, "capsules"))
		}},
		{name: "empty-lock", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, root)
			if err := os.WriteFile(filepath.Join(root, ".registry.lock"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "nonempty-lock", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, root)
			if err := os.WriteFile(filepath.Join(root, ".registry.lock"), []byte("held"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "empty-unknown-safe", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, root)
			if err := os.WriteFile(filepath.Join(root, "unknown-empty"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "nonempty-unknown-safe", state: true, seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirRegistryTestV2(t, root)
			if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("frozen"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "registry")
			if testCase.seed != nil {
				testCase.seed(t, root)
			}
			access, err := privatecastest.NewAccessAuthority(base)
			if err != nil {
				t.Fatal(err)
			}
			state, err := HasState(context.Background(), root, access)
			if err != nil || state != testCase.state {
				t.Fatalf("HasState = %t, %v; want %t", state, err, testCase.state)
			}
		})
	}
}

func TestHasStateRejectsUnsafeInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires a real Windows privilege gate")
	}
	base := t.TempDir()
	root := filepath.Join(base, "registry")
	mustMkdirRegistryTestV2(t, root)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "unsafe")); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(base)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := HasState(context.Background(), root, access); err == nil || state {
		t.Fatalf("unsafe state probe = %t, %v; want fatal", state, err)
	}
}

func mustMkdirRegistryTestV2(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryV2FreezesUnknownLegacyIndexCrashResidueWithoutSemanticActivation(t *testing.T) {
	root := t.TempDir()
	residue := filepath.Join(root, ".registry-authority-index-deadbeef.tmp")
	body := []byte("unknown-legacy-index-crash-residue")
	if err := os.WriteFile(residue, body, 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatalf("safe unknown V1 residue was not physically frozen: %v", err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("unknown V1 authority-index crash residue entered registry semantics")
	}
	if err := prepared.RevalidatePhysicalV2(context.Background()); err != nil {
		t.Fatalf("unknown V1 residue physical binding did not revalidate: %v", err)
	}
	retained, err := os.ReadFile(residue)
	if err != nil || !bytes.Equal(retained, body) {
		t.Fatalf("failed recovery deleted or changed unknown V1 crash residue: %v", err)
	}
}

func TestRecoveryV2FreezesUnknownLegacyThreadDirectoryWithoutSemanticActivation(t *testing.T) {
	root := t.TempDir()
	legacyThread := filepath.Join(root, "thread-frozen-v1")
	if err := os.Mkdir(legacyThread, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(legacyThread)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV2(context.Background(), root, access)
	if err != nil {
		t.Fatalf("safe legacy thread directory was not physically frozen: %v", err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err == nil {
		t.Fatal("unknown legacy thread directory entered registry semantics")
	}
	if err := prepared.RevalidatePhysicalV2(context.Background()); err != nil {
		t.Fatalf("legacy thread physical binding did not revalidate: %v", err)
	}
	after, err := os.Stat(legacyThread)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("legacy thread directory identity changed: %v", err)
	}
}

func TestRecoveryV2RejectsReservedCaseAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Indexes"), 0o700); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRecoveryV2(context.Background(), root, access); err == nil {
		t.Fatal("case-alias evidence-registry inventory was frozen as an optional semantic blocker")
	}
}

func TestValidRegistryPrefixTailDeletionPreservesTrustedCapsuleAuthority(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(context.Background(), registryport.RevokeInput{
		Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	path := store.registryPath(securityContext)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	firstRecordEnd := bytes.IndexByte(body, '\n')
	if firstRecordEnd < 0 || firstRecordEnd+1 >= len(body) {
		t.Fatalf("registry fixture does not contain issue+revoke records: %q", body)
	}
	if err := os.WriteFile(path, body[:firstRecordEnd+1], 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 2 || replayed.StateDigest == "" {
		t.Fatalf("trusted capsule did not preserve the current revoked registry over a short projection: registry=%#v err=%v", replayed, err)
	}
	if _, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID}); err == nil {
		t.Fatal("tail deletion resurrected a revoked evidence receipt")
	}
}

func TestCapsulePairRollbackCannotResurrectRevokedReceipt(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	projectionPath := store.registryPath(securityContext)
	capsuleProjectionPath := store.registryAuthoritySealPath(securityContext)
	issueLedger, err := os.ReadFile(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	issueCapsuleProjection, err := os.ReadFile(capsuleProjectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(context.Background(), registryport.RevokeInput{
		Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectionPath, issueLedger, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(capsuleProjectionPath, issueCapsuleProjection, 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 2 {
		t.Fatalf("root authority index did not dominate an older per-turn file pair: registry=%#v err=%v", replayed, err)
	}
	if _, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID}); err == nil {
		t.Fatal("restoring an older per-turn capsule and ledger resurrected a revoked receipt")
	}
}

func TestOlderRootIndexCannotHideNewerInstalledCapsuleBlob(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	oldIndex, err := os.ReadFile(store.authorityIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	parsedOldIndex, err := domainevidence.ParseEvidenceRegistryAuthorityIndex(oldIndex)
	if err != nil || len(parsedOldIndex.Entries) != 1 {
		t.Fatalf("old index fixture is invalid: index=%#v err=%v", parsedOldIndex, err)
	}
	oldBlobPath := store.authorityCapsuleBlobPath(parsedOldIndex.Entries[0].CapsuleSHA256)
	oldBlob, err := os.ReadFile(oldBlobPath)
	if err != nil {
		t.Fatal(err)
	}
	oldLedger, err := os.ReadFile(store.registryPath(securityContext))
	if err != nil {
		t.Fatal(err)
	}
	oldProjection, err := os.ReadFile(store.registryAuthoritySealPath(securityContext))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(context.Background(), registryport.RevokeInput{
		Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.authorityIndexPath(), oldIndex, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldBlobPath, oldBlob, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.registryPath(securityContext), oldLedger, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.registryAuthoritySealPath(securityContext), oldProjection, 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID}); err == nil {
		t.Fatal("older root index hid a newer installed revocation capsule")
	}
	if _, err := restarted.HasRecords(context.Background()); err == nil {
		t.Fatal("inventory did not report the newer capsule rollback witness")
	}
}

func TestProjectionInventoryRejectsCompositeSuffixAlias(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	key := domainevidence.EvidenceRegistryProjectionKey(securityContext.ThreadID, securityContext.TurnID)
	alias := filepath.Join(filepath.Dir(store.registryPath(securityContext)), key+".jsonl.head.json")
	if err := os.WriteFile(alias, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("composite projection suffix alias was silently ignored")
	}
}

func TestAuthorityIndexCommitSurvivesMissingDerivedProjections(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.registryPath(securityContext)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.registryAuthoritySealPath(securityContext)); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 1 {
		t.Fatalf("missing derived projections changed committed authority: registry=%#v err=%v", replayed, err)
	}
	if hasRecords, err := restarted.HasRecords(context.Background()); err != nil || !hasRecords {
		t.Fatalf("root index inventory lost authority without projections: hasRecords=%v err=%v", hasRecords, err)
	}
}

func TestStaleProjectionTempCannotBecomeAuthorityOrBrickRestart(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	projectionDir := filepath.Dir(store.registryPath(securityContext))
	stale := filepath.Join(projectionDir, "."+filepath.Base(store.registryPath(securityContext))+"-crash.tmp")
	if err := os.WriteFile(stale, []byte(`{"fabricated":"authority"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	if hasRecords, err := restarted.HasRecords(context.Background()); err != nil || !hasRecords {
		t.Fatalf("safe untrusted temp residue bricked restart: hasRecords=%v err=%v", hasRecords, err)
	}
	replayed, err := restarted.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 1 {
		t.Fatalf("temp residue influenced authority: registry=%#v err=%v", replayed, err)
	}
}

func TestCapsuleLinkUnlinkCrashResidueIsRepairableButNotAuthority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink crash residue requires a real Windows behavior gate")
	}
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	index, err := store.readAuthorityIndexLocked()
	if err != nil || len(index.Entries) != 1 {
		t.Fatalf("authority index fixture is invalid: index=%#v err=%v", index, err)
	}
	blob := store.authorityCapsuleBlobPath(index.Entries[0].CapsuleSHA256)
	temp := filepath.Join(store.authorityCapsuleDirectory(), ".capsule-crash-window.tmp")
	if err := os.Link(blob, temp); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	if hasRecords, err := restarted.HasRecords(context.Background()); err != nil || !hasRecords {
		t.Fatalf("repairable CAS link/unlink residue bricked restart: hasRecords=%v err=%v", hasRecords, err)
	}
	replayed, err := restarted.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 1 {
		t.Fatalf("CAS temp hardlink changed membership: registry=%#v err=%v", replayed, err)
	}
}

func TestCurrentCapsuleGCPreventsQuadraticHistoryBlobGrowth(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	for index := 0; index < 8; index++ {
		input := storeTestPreparedInputForLabel(t, securityContext, fmt.Sprintf("growth-%02d", index), fmt.Sprintf("%d", 4_200_000+index))
		receipt, err := store.CommitPrepared(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Revoke(context.Background(), registryport.RevokeInput{
			Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(store.authorityCapsuleDirectory())
	if err != nil {
		t.Fatal(err)
	}
	blobs := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			blobs++
		}
	}
	if blobs != 1 {
		t.Fatalf("same-turn capsule history retained %d blobs instead of one current authority blob", blobs)
	}
	replayed, err := store.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 16 {
		t.Fatalf("capsule GC changed registry history: sequence=%d err=%v", replayed.Sequence, err)
	}
}

func TestSignedCapsuleProjectionWithoutRootIndexIsNotSilentlyAdopted(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.authorityIndexPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replay(context.Background(), securityContext); err == nil {
		t.Fatal("a signed per-turn capsule projection was promoted after the root authority index was removed")
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("inventory accepted projections without a root authority index")
	}
}

func TestCommitPreparedHasNoHiddenCommittedError(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &registryStoreFaultHooks{AfterAuthorityIndexReplace: func() error { return errors.New("injected post-replace failure") }}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	draft := storeTestDraft(t, securityContext, material)
	if receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); !errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) || receipt.ReceiptID != "" {
		t.Fatalf("post-commit fault was not classified as indeterminate: receipt=%#v err=%v", receipt, err)
	}
	if _, err := store.Replay(context.Background(), securityContext); !errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) {
		t.Fatalf("poisoned store continued serving membership after an indeterminate commit: %v", err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: draft.ReceiptID})
	if err != nil || resolved.Receipt.ReceiptID != draft.ReceiptID {
		t.Fatalf("restart could not deterministically classify the visible committed generation: resolved=%#v err=%v", resolved, err)
	}
}

func TestAuthorityCommitFailureBeforeReplaceLeavesNoMembershipAndIsRetryable(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &registryStoreFaultHooks{BeforeAuthorityIndexReplace: func() error { return errors.New("injected pre-replace failure") }}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	input := registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}
	if _, err := store.CommitPrepared(context.Background(), input); err == nil || errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) {
		t.Fatalf("pre-commit fault was misclassified: %v", err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Replay(context.Background(), securityContext)
	if err != nil || replayed.Sequence != 0 {
		t.Fatalf("provably uncommitted capsule was not cleaned without changing membership: registry=%#v err=%v", replayed, err)
	}
	receipt, err := restarted.CommitPrepared(context.Background(), input)
	if err != nil || receipt.ReceiptID != input.Draft.ReceiptID {
		t.Fatalf("exact prepared retry did not commit after a pre-replace failure: receipt=%#v err=%v", receipt, err)
	}
}

func TestIndexSigningFailureLeavesNoPersistedCapsuleBlob(t *testing.T) {
	root := t.TempDir()
	base := storeTestAuthority(t, root)
	authority := &failNthSignAuthority{Authority: base, failAt: 2}
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err == nil {
		t.Fatal("injected authority-index signing failure was ignored")
	}
	if _, err := os.Lstat(store.authorityIndexPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authority index was persisted after its signing failed: %v", err)
	}
	if entries, err := os.ReadDir(store.authorityCapsuleDirectory()); err == nil && len(entries) != 0 {
		t.Fatalf("index signing failure left persisted capsule blobs: %#v", entries)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestCapsuleInstallFailureBeforeIndexReplaceCleansAndRetries(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &registryStoreFaultHooks{AfterAuthorityCapsuleInstall: func() error { return errors.New("injected capsule install failure") }}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	input := registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}
	if _, err := store.CommitPrepared(context.Background(), input); err == nil || errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) {
		t.Fatalf("pre-index capsule install failure was misclassified: %v", err)
	}
	if _, err := os.Lstat(store.authorityIndexPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capsule install failure created a root index: %v", err)
	}
	if files, err := os.ReadDir(store.authorityCapsuleDirectory()); err != nil || len(files) != 0 {
		t.Fatalf("capsule install failure left uncommitted CAS residue: files=%#v err=%v", files, err)
	}
	store.faults = nil
	receipt, err := store.CommitPrepared(context.Background(), input)
	if err != nil || receipt.ReceiptID != input.Draft.ReceiptID {
		t.Fatalf("exact retry failed after provable pre-index cleanup: receipt=%#v err=%v", receipt, err)
	}
}

func TestRevokeHasNoHiddenCommittedError(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &registryStoreFaultHooks{AfterAuthorityIndexReplace: func() error { return errors.New("injected revoke post-replace failure") }}
	if err := store.Revoke(context.Background(), registryport.RevokeInput{
		Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
	}); !errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) {
		t.Fatalf("post-commit revoke fault was not classified as indeterminate: %v", err)
	}
	if _, err := store.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID}); !errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) {
		t.Fatalf("poisoned store served a receipt after indeterminate revocation: %v", err)
	}
	restarted, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Resolve(context.Background(), registryport.MembershipQuery{Context: securityContext, ReceiptID: receipt.ReceiptID}); err == nil {
		t.Fatal("restart resurrected a revocation committed before the injected failure")
	}
}

func TestAuthorityIndexPredecessorCASRejectsLateReplacement(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store.faults = &registryStoreFaultHooks{BeforeAuthorityIndexReplace: func() error {
		return os.WriteFile(store.authorityIndexPath(), []byte(`{}`), 0o600)
	}}
	if err := store.Revoke(context.Background(), registryport.RevokeInput{
		Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
	}); err == nil {
		t.Fatal("late authority-index replacement was blindly overwritten")
	}
	body, err := os.ReadFile(store.authorityIndexPath())
	if err != nil || string(body) != `{}` {
		t.Fatalf("failed commit replaced the unexpected predecessor: body=%q err=%v", body, err)
	}
}

func TestRootIndexWriteRejectsCorruptReferencedOtherTurnCapsule(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	left := storeTestContext(t)
	leftInput := storeTestPreparedInputForLabel(t, left, "left-first", "4200001")
	if _, err := store.CommitPrepared(context.Background(), leftInput); err != nil {
		t.Fatal(err)
	}
	right := storeTestContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-b", TurnID: "turn-b", WorkspaceRealPath: "/workspace", CaseID: "case-b",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-b"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-b")), ContextEpoch: 1, IssuedAt: storeTestTime(),
	})
	if _, err := store.CommitPrepared(context.Background(), storeTestPreparedInputForLabel(t, right, "right-first", "5200001")); err != nil {
		t.Fatal(err)
	}
	beforeIndex, err := os.ReadFile(store.authorityIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainevidence.ParseEvidenceRegistryAuthorityIndex(beforeIndex)
	if err != nil {
		t.Fatal(err)
	}
	rightEntry, ok := domainevidence.EvidenceRegistryAuthorityIndexEntryForContext(index, right)
	if !ok {
		t.Fatal("right-turn index entry is missing")
	}
	if err := os.WriteFile(store.authorityCapsuleBlobPath(rightEntry.CapsuleSHA256), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPrepared(context.Background(), storeTestPreparedInputForLabel(t, left, "left-second", "4200002")); err == nil {
		t.Fatal("root index write continued after another referenced turn capsule was corrupted")
	}
	afterIndex, err := os.ReadFile(store.authorityIndexPath())
	if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatalf("failed global inventory check changed the signed root index: changed=%v err=%v", !bytes.Equal(beforeIndex, afterIndex), err)
	}
}

func TestRegistryProjectionExactPrefixRepairBoundaryAndInstallationTrust(t *testing.T) {
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	path := store.registryPath(securityContext)
	ledger, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if replayed, err := store.Replay(context.Background(), securityContext); err != nil || replayed.Sequence != 1 {
		t.Fatalf("missing derived projection over a trusted capsule was not recoverable: registry=%#v err=%v", replayed, err)
	}
	if err := os.WriteFile(path, ledger[:len(ledger)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replay(context.Background(), securityContext); err == nil {
		t.Fatal("projection truncated inside a canonical record was accepted")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if replayed, err := store.Replay(context.Background(), securityContext); err != nil || replayed.Sequence != 1 {
		t.Fatalf("zero-length derived projection over a trusted capsule changed membership: registry=%#v err=%v", replayed, err)
	}
	otherAuthorityPath := filepath.Join(t.TempDir(), "other-authority", "ed25519-v1.json")
	otherAuthority, err := finalauthority.OpenOrCreateFileAuthority(otherAuthorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	wrongInstallation, err := NewStore(root, otherAuthority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongInstallation.Replay(context.Background(), securityContext); err == nil {
		t.Fatal("registry capsule signed by another installation was accepted")
	}
}

func TestPrivateEvidenceRegistryCorruptionFailsClosed(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	draft := storeTestDraft(t, securityContext, material)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	path := store.registryPath(securityContext)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"schemaVersion":1`); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := store.Replay(context.Background(), securityContext); err == nil {
		t.Fatal("truncated registry line was silently skipped")
	}
}

func TestPrivateEvidenceRegistryRetriesExactRegistrationWithoutDuplicateAndRejectsUnknownVersion(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	draft := storeTestDraft(t, securityContext, material)
	input := registryport.CommitPreparedInput{Context: securityContext, Draft: draft, CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime()}
	first, err := store.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, err := store.CommitPrepared(context.Background(), input); err != nil || receipt.ReceiptID != first.ReceiptID {
		t.Fatalf("exact registration retry did not return the existing receipt: receipt=%#v err=%v", receipt, err)
	}
	if replayed, err := store.Replay(context.Background(), securityContext); err != nil || replayed.Sequence != 1 {
		t.Fatalf("exact registration retry appended a duplicate entry: sequence=%d err=%v", replayed.Sequence, err)
	}
	path := store.registryPath(securityContext)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), `"schemaVersion":2`, `"schemaVersion":99`, 1))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replay(context.Background(), securityContext); err == nil {
		t.Fatal("unknown registry entry version did not fail closed")
	}
}

func TestPrivateEvidenceRegistryInventoryIsStrictAndComplete(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := store.HasRecords(context.Background()); err != nil || exists {
		t.Fatalf("empty registry reported authority: exists=%v err=%v", exists, err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	if exists, err := store.HasRecords(context.Background()); err != nil || !exists {
		t.Fatalf("persisted registry was absent from inventory: exists=%v err=%v", exists, err)
	}
	records, err := store.ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{securityContext})
	if err != nil || len(records) != 1 || records[0].Registry.Sequence != 1 || records[0].Context != securityContext {
		t.Fatalf("registry inventory mismatch: records=%#v err=%v", records, err)
	}
	if _, err := store.ListRegistries(context.Background(), nil); err == nil {
		t.Fatal("registry detached from supplied durable contexts was accepted")
	}
}

func TestRegistryInventoryAndExistingConstructorPreserveDirectoryModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	root := t.TempDir()
	authority := storeTestAuthority(t, root)
	store, err := NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	frozen := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{Context: frozen, Draft: storeTestDraft(t, frozen, material), CanonicalEvidence: material, SettlementProof: storeTestProof(), RegisteredAt: storeTestTime()}); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"inventory", "constructor"} {
		t.Run(operation, func(t *testing.T) {
			if err := os.Chmod(root, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
			if operation == "inventory" {
				if _, err := store.ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{frozen}); err != nil {
					t.Fatal(err)
				}
			} else if _, err := NewStore(root, authority); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(root)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o500 {
				t.Fatalf("read/open changed original directory mode to %04o", info.Mode().Perm())
			}
		})
	}
}

func TestPrivateEvidenceRegistryInventoryUsesCollisionResistantPathsAndRejectsUnknownFilesAndHardlinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	left := storeTestContext(t)
	left.ThreadID = "thread/a"
	left.ContextDigest = ""
	left = storeTestContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: left.ThreadID, TurnID: left.TurnID, WorkspaceRealPath: left.WorkspaceRealPath, CaseID: left.CaseID,
		CaseBindingHash: left.CaseBindingHash, DatasetSnapshotID: left.DatasetSnapshotID, SourceManifestHash: left.SourceManifestHash,
		ContextEpoch: left.ContextEpoch, IssuedAt: storeTestTime(),
	})
	right := storeTestContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_a", TurnID: left.TurnID, WorkspaceRealPath: left.WorkspaceRealPath, CaseID: left.CaseID,
		CaseBindingHash: left.CaseBindingHash, DatasetSnapshotID: left.DatasetSnapshotID, SourceManifestHash: left.SourceManifestHash,
		ContextEpoch: left.ContextEpoch, IssuedAt: storeTestTime(),
	})
	if store.registryPath(left) == store.registryPath(right) {
		t.Fatal("distinct raw turn identities collided in the content-addressed projection path")
	}
	if records, err := store.ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{left, right}); err != nil || len(records) != 0 {
		t.Fatalf("collision-resistant empty inventory was rejected: records=%#v err=%v", records, err)
	}
	unknown := filepath.Join(store.root, "unknown")
	if err := os.WriteFile(unknown, []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("unknown registry root file was silently ignored")
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(store.registryPath(securityContext), filepath.Join(t.TempDir(), "registry-hardlink.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{securityContext}); err == nil {
		t.Fatal("hard-linked private registry authority was accepted")
	}
}

func TestRegistryLockHardlinkDoesNotMutateExternalTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix lock identity test")
	}
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "external-sentinel")
	want := []byte("external-content-must-not-change")
	if err := os.WriteFile(sentinel, want, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sentinel, filepath.Join(root, ".registry.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("hard-linked registry lock was accepted")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("registry lock rejection modified external content: got=%q err=%v", got, err)
	}
	info, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("registry lock rejection modified external permissions: mode=%v", info.Mode().Perm())
	}
}

func TestRegistryLockSymlinkCannotEscapeRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix symlink lock identity test")
	}
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "external-sentinel")
	if err := os.WriteFile(sentinel, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(root, ".registry.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.HasRecords(context.Background()); err == nil {
		t.Fatal("symlinked registry lock escaped the frozen registry root")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "external" {
		t.Fatalf("symlink lock rejection modified external target: got=%q err=%v", got, err)
	}
}

func TestColdRegistryRootRejectsSymlinkAncestorBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation behavior requires a real Windows host gate")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(alias, "registry")
	authority := storeTestAuthority(t, filepath.Join(base, "authority-root"))
	access, accessErr := privatecastest.NewAccessAuthority(base)
	if accessErr != nil {
		t.Fatal(accessErr)
	}
	if exists, err := HasState(context.Background(), root, access); err == nil || exists {
		t.Fatalf("cold symlink ancestor was not rejected by the read-only state probe: exists=%v err=%v", exists, err)
	}
	if _, err := NewStore(root, authority); err == nil {
		t.Fatal("cold registry root traversed a symlink ancestor")
	}
	if _, err := os.Lstat(filepath.Join(target, "registry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registry rejection mutated the symlink target: %v", err)
	}
}

func TestPrivateEvidenceRegistryRejectsEmptyCreateResidueAndIncompleteFinalRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	path := store.registryPath(securityContext)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListRegistries(context.Background(), []domainsecurity.TurnSecurityContext{securityContext}); err == nil {
		t.Fatal("empty create residue was accepted as an authoritative empty registry")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	material := storeTestMaterial(t)
	if _, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 || body[len(body)-1] != '\n' {
		t.Fatalf("registry fixture lacks a complete record: len=%d err=%v", len(body), err)
	}
	if err := os.WriteFile(path, body[:len(body)-1], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replay(context.Background(), securityContext); err == nil {
		t.Fatal("registry JSON without a final commit newline was accepted")
	}
}

func TestLockedSnapshotLinearizesCrossStoreRevocationAndHistoricalReplay(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := storeTestContext(t)
	material := storeTestMaterial(t)
	receipt, err := store.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: storeTestDraft(t, securityContext, material), CanonicalEvidence: material,
		SettlementProof: storeTestProof(), RegisteredAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRuntime, err := NewStore(root, storeTestAuthority(t, root))
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	snapshotDone := make(chan error, 1)
	go func() {
		snapshotDone <- store.WithLockedSnapshot(context.Background(), securityContext, func(snapshot domainevidence.EvidenceReceiptRegistry) error {
			if snapshot.Sequence != 1 {
				return errors.New("unexpected locked registry sequence")
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	revokeDone := make(chan error, 1)
	go func() {
		revokeDone <- otherRuntime.Revoke(context.Background(), registryport.RevokeInput{
			Context: securityContext, ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted", RevokedAt: storeTestTime().Add(time.Minute),
		})
	}()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation crossed a locked publication snapshot: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-snapshotDone; err != nil {
		t.Fatal(err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatal(err)
	}
	historical, err := store.ReplayAt(context.Background(), securityContext, 1)
	if err != nil || historical.Sequence != 1 {
		t.Fatalf("historical registry head was not replayable: sequence=%d err=%v", historical.Sequence, err)
	}
	if _, err := domainevidence.VerifyEvidenceReceiptMembership(historical, securityContext, receipt.ReceiptID); err != nil {
		t.Fatalf("historical accepted receipt was not valid at its sealed head: %v", err)
	}
	current, err := store.Replay(context.Background(), securityContext)
	if err != nil || current.Sequence != 2 {
		t.Fatalf("current registry did not include later revocation: sequence=%d err=%v", current.Sequence, err)
	}
	if _, err := domainevidence.VerifyEvidenceReceiptMembership(current, securityContext, receipt.ReceiptID); err == nil {
		t.Fatal("current registry accepted a revoked receipt")
	}
}

func storeTestContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	return storeTestContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: storeTestTime(),
	})
}

func storeTestContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	policyDigest := domainsecurity.SHA256Hex([]byte("evidence-registry-store-policy:\x00" + input.ThreadID + "\x00" + input.WorkspaceRealPath))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("evidence-registry-store-binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(input.ThreadID, input.WorkspaceRealPath, domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	input.TenantID = domainsecurity.LocalTenantID
	input.UserID = domainsecurity.LocalUserID
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = binding
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		t.Fatal(err)
	}
	return securityContext
}

func storeTestDraft(t *testing.T, securityContext domainsecurity.TurnSecurityContext, material json.RawMessage) domainevidence.EvidenceReceipt {
	t.Helper()
	canonical, err := domainevidence.CanonicalEvidenceBytes(material)
	if err != nil {
		t.Fatal(err)
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix-fund-analysis", "1.0.0", domainsecurity.SHA256Hex([]byte("evidence-registry-store-test-instance")), 3)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(storeTestProof().SettlementID), Context: securityContext, ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")), ToolCallID: storeTestHostToolCallID("draft-a"),
		ServerIdentity: serverIdentity, ServerVersion: "1.0.0", ConnectionEpoch: 3,
		ToolName: "mcp__analytix_funds__query", ArgsHash: domainsecurity.SHA256Hex([]byte("args")), ResultHash: domainsecurity.CanonicalJSONHash(canonical),
		SourceType: "transactions", DatasetSnapshotID: securityContext.DatasetSnapshotID, QueryHash: domainsecurity.SHA256Hex([]byte("query")),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{"00123456789012345678"}, Directions: []string{"out"},
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"flow-a"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("filters")),
		},
		Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{"row-a"}, RawSHA256: domainsecurity.SHA256Hex([]byte("raw")), TransformationLineage: []domainevidence.TransformationLineageStep{},
		PIIClassification: domainevidence.PIIMasked, IssuedAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func storeTestMaterial(t *testing.T) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-a", ClaimType: domainevidence.ClaimAmount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: "entity-a", EntityID: "entity-a", AccountID: "00123456789012345678",
				AmountMinor: "4200000", Currency: "CNY", Direction: "out",
				StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func storeTestProof() domainevidence.EvidenceSettlementProof {
	return domainevidence.EvidenceSettlementProof{
		SettlementID:         domainsecurity.SHA256Hex([]byte("settlement-a")),
		PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("prepared-a")),
	}
}

func storeTestPreparedInputForLabel(t *testing.T, securityContext domainsecurity.TurnSecurityContext, label, amountMinor string) registryport.CommitPreparedInput {
	t.Helper()
	material, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-" + label, ClaimType: domainevidence.ClaimAmount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: "entity-a", EntityID: "entity-a", AccountID: "00123456789012345678", AmountMinor: amountMinor,
				Currency: "CNY", Direction: "out", StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(material)
	if err != nil {
		t.Fatal(err)
	}
	proof := domainevidence.EvidenceSettlementProof{
		SettlementID: domainsecurity.SHA256Hex([]byte("settlement-" + label)), PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("prepared-" + label)),
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix-fund-analysis", "1.0.0", domainsecurity.SHA256Hex([]byte("growth-instance")), 3)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(proof.SettlementID), Context: securityContext,
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant-" + label)), ToolCallID: storeTestHostToolCallID(label),
		ServerIdentity: serverIdentity, ServerVersion: "1.0.0", ConnectionEpoch: 3, ToolName: "mcp__analytix_funds__query",
		ArgsHash: domainsecurity.SHA256Hex([]byte("args-" + label)), ResultHash: domainsecurity.CanonicalJSONHash(canonical),
		SourceType: "transactions", DatasetSnapshotID: securityContext.DatasetSnapshotID, QueryHash: domainsecurity.SHA256Hex([]byte("query-" + label)),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-a"}, AccountIDs: []string{"00123456789012345678"}, Directions: []string{"out"},
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", SourceIDs: []string{"flow-a"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("filters-" + label)),
		},
		Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{"row-" + label}, RawSHA256: domainsecurity.SHA256Hex([]byte("raw-" + label)),
		TransformationLineage: []domainevidence.TransformationLineageStep{}, PIIClassification: domainevidence.PIIMasked, IssuedAt: storeTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return registryport.CommitPreparedInput{Context: securityContext, Draft: draft, CanonicalEvidence: material, SettlementProof: proof, RegisteredAt: storeTestTime()}
}

func storeTestTime() time.Time {
	return time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
}

func storeTestAuthority(t *testing.T, registryRoot string) *finalauthority.FileAuthority {
	t.Helper()
	path := filepath.Join(filepath.Dir(registryRoot), filepath.Base(registryRoot)+"-authority", "ed25519-v1.json")
	authority, err := finalauthority.OpenOrCreateFileAuthority(path, false)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

type failNthSignAuthority struct {
	finalauthorityport.Authority
	signCount int
	failAt    int
}

func (authority *failNthSignAuthority) Sign(ctx context.Context, message []byte) ([]byte, error) {
	authority.signCount++
	if authority.signCount == authority.failAt {
		return nil, errors.New("injected authority signing failure")
	}
	return authority.Authority.Sign(ctx, message)
}
