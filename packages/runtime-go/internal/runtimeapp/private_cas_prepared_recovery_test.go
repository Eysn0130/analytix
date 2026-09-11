package runtimeapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type countingRuntimePrivateCASAccessAuthority struct {
	inner finalauthority.SecurePrivateCASRecoveryAccessAuthority
	calls int
}

func (authority *countingRuntimePrivateCASAccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	authority.calls++
	return authority.inner.WithPrivateCASAccess(ctx, requestedRoot, access)
}

func (authority *countingRuntimePrivateCASAccessAuthority) WithExistingPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	authority.calls++
	return authority.inner.WithExistingPrivateCASAccess(ctx, requestedRoot, access)
}

func TestRuntimePrivateCASOwnerManifestBindsEveryExactProductionRoot(t *testing.T) {
	dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
	legacyAudit, err := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	privateRoot := filepath.Join(dataDir, "private")
	expected := map[string][]string{
		"backend-generation": {
			filepath.Join(privateRoot, "runtime-sidecar-authority-v1", "allocations"),
		},
		"accepted-finals": {
			filepath.Join(privateRoot, "accepted-finals", "records"),
			filepath.Join(privateRoot, "accepted-finals", "dispositions"),
		},
		"case-entity": {
			filepath.Join(privateRoot, "case-entity", "bindings-v1"),
			filepath.Join(privateRoot, "case-entity", "ingress-v1"),
			filepath.Join(privateRoot, "case-entity", "thread-context-v1"),
		},
		"case-thread-authority": {
			filepath.Join(privateRoot, "case-thread-authority"),
		},
		"gate-continuations": {
			filepath.Join(privateRoot, "gate-continuations", "receipts-v2"),
			filepath.Join(privateRoot, "gate-continuations", "dispositions-v2"),
		},
		"pending-work": {
			filepath.Join(privateRoot, "pending-work", "receipts"),
			filepath.Join(privateRoot, "pending-work", "dispositions"),
		},
		"provider-cache-telemetry": {
			filepath.Join(privateRoot, "provider-cache-telemetry", "attempts"),
			filepath.Join(privateRoot, "provider-cache-telemetry", "settlements"),
			filepath.Join(privateRoot, "provider-cache-telemetry", "turn-closures"),
		},
		"turn-terminal-authority": {
			filepath.Join(privateRoot, "turn-terminal-authority", "intents"),
			filepath.Join(privateRoot, "turn-terminal-authority", "dispositions"),
		},
		"attachment-authority": {
			filepath.Join(privateRoot, "attachment-authority", "owners"),
			filepath.Join(privateRoot, "attachment-authority", "use-receipts"),
			filepath.Join(privateRoot, "attachment-authority", "use-dispositions"),
			filepath.Join(privateRoot, "attachment-authority", "upload-intents"),
			filepath.Join(privateRoot, "attachment-authority", "upload-dispositions"),
		},
		"authority-advance": {
			filepath.Join(privateRoot, "authority-advance", "v2", "intents"),
			filepath.Join(privateRoot, "authority-advance", "v2", "settlements"),
		},
		"evidence-authority": {
			filepath.Join(privateRoot, "evidence-authority", "bundles"),
			filepath.Join(privateRoot, "evidence-authority", "observations"),
		},
		"evidence-registry": {
			filepath.Join(privateRoot, "evidence-registry", "capsules"),
			filepath.Join(privateRoot, "evidence-registry", "indexes"),
		},
		"dataset-snapshot-authority": {
			filepath.Join(privateRoot, "dataset-snapshot-authority", "legacy-records"),
			filepath.Join(privateRoot, "dataset-snapshot-authority", "authority-bundles-v2"),
			filepath.Join(privateRoot, "dataset-snapshot-authority", "indexes"),
			filepath.Join(privateRoot, "dataset-snapshot-authority", "materials"),
		},
		"thread-risk-policy": {
			filepath.Join(privateRoot, "thread-risk-policy"),
		},
		"pii-authorization": {
			filepath.Join(privateRoot, "pii-authorization", "grants"),
		},
		"report-publication": {
			filepath.Join(privateRoot, "report-publication", "receipts"),
			filepath.Join(privateRoot, "report-publication", "attempts"),
			filepath.Join(privateRoot, "report-publication", "commit-receipts"),
			filepath.Join(privateRoot, "report-publication", "commit-selections"),
			filepath.Join(privateRoot, "report-publication", "delivery-decisions"),
			filepath.Join(privateRoot, "report-publication", "grant-settlements"),
			filepath.Join(privateRoot, "report-publication", "stage-completions"),
			filepath.Join(privateRoot, "report-publication", "delivery-projections"),
			filepath.Join(privateRoot, "report-publication", "indexes"),
			filepath.Join(privateRoot, "report-publication", "claim-ledgers"),
			filepath.Join(privateRoot, "report-publication", "pii-projections"),
			filepath.Join(privateRoot, "report-publication", "render-inspections"),
			filepath.Join(privateRoot, "report-publication", "artifacts"),
		},
		"controlled-artifact-access": {
			filepath.Join(privateRoot, "controlled-artifact-access", "access-receipts"),
			filepath.Join(privateRoot, "controlled-artifact-access", "access-dispositions"),
		},
		"controlled-artifact-access-v2": {
			filepath.Join(privateRoot, "controlled-artifact-access-v2", "access-receipts"),
			filepath.Join(privateRoot, "controlled-artifact-access-v2", "access-dispositions"),
		},
		"checkpoint-authority": {
			filepath.Join(privateRoot, "checkpoint-authority", "snapshot-intents"),
			filepath.Join(privateRoot, "checkpoint-authority", "snapshot-completions"),
			filepath.Join(privateRoot, "checkpoint-authority", "snapshot-dispositions"),
			filepath.Join(privateRoot, "checkpoint-authority", "operation-group-intents-v2"),
			filepath.Join(privateRoot, "checkpoint-authority", "operation-group-terminals-v2"),
			legacyAudit,
		},
	}
	owners := runtimePrivateCASOwnerRecoveries(dataDir, access)
	if len(owners) != len(expected) {
		t.Fatalf("runtime private CAS owner count = %d, want %d", len(owners), len(expected))
	}
	for _, owner := range owners {
		want, found := expected[owner.name]
		if !found || !sameRuntimePrivateCASRootSet(owner.expectedRoots, want) {
			t.Fatalf("runtime private CAS owner %s manifest = %v, want %v", owner.name, owner.expectedRoots, want)
		}
		prepared, err := owner.prepare(context.Background())
		if err != nil {
			t.Fatalf("prepare owner %s: %v", owner.name, err)
		}
		if err := prepared.ValidateSemantics(context.Background()); err != nil {
			t.Fatalf("validate owner %s semantics: %v", owner.name, err)
		}
		plans := prepared.SecurePrivateCASRecoveryPlansV2()
		if err := validateRuntimePrivateCASOwnerRootManifest(owner, plans); err != nil {
			t.Fatalf("validate owner %s exact manifest: %v", owner.name, err)
		}
		topologies := prepared.PrivateCASRecoveryTopologiesV3()
		if err := validateRuntimePrivateCASOwnerTopologyManifest(owner, plans, topologies); err != nil {
			t.Fatalf("validate owner %s topology manifest: %v", owner.name, err)
		}
		wantTopologyCount := 1
		if owner.standaloneCASRoots {
			wantTopologyCount = 0
		} else if owner.name == "checkpoint-authority" {
			wantTopologyCount = 2
		}
		if len(topologies) != wantTopologyCount {
			t.Fatalf("owner %s topology count = %d, want %d", owner.name, len(topologies), wantTopologyCount)
		}
		if len(topologies) > 0 {
			changed := append([]finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3(nil), topologies...)
			changed[0] = runtimePrivateCASTestTopology(filepath.Join(dataDir, "unrelated-owner"))
			if err := validateRuntimePrivateCASOwnerTopologyManifest(owner, plans, changed); err == nil {
				t.Fatalf("owner %s accepted a topology that covers no exact recovery root", owner.name)
			}
		}
		actual := make([]string, 0, len(plans))
		for _, plan := range plans {
			actual = append(actual, plan.RootPath())
		}
		if !sameRuntimePrivateCASRootSet(actual, want) {
			t.Fatalf("owner %s prepared roots = %v, want %v", owner.name, actual, want)
		}
		tampered := owner
		tampered.expectedRoots = append([]string(nil), owner.expectedRoots...)
		tampered.expectedRoots[0] += "-unexpected"
		if err := validateRuntimePrivateCASOwnerRootManifest(tampered, plans); err == nil {
			t.Fatalf("owner %s accepted a changed exact root manifest", owner.name)
		}
	}
}

type runtimePrivateCASTestTopology string

func (root runtimePrivateCASTestTopology) PrivateCASRecoveryTopologyRootV3() string {
	return string(root)
}

func (runtimePrivateCASTestTopology) RevalidatePrivateCASRecoveryTopologyV3(context.Context) error {
	return nil
}

func sameRuntimePrivateCASRootSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		seen[value]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func TestPreparedRuntimePrivateCASLateSemanticCorruptionPreservesEarlierResidue(t *testing.T) {
	dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
	initializeControlledArtifactAccessRecoveryOwner(t, dataDir)
	earlyTemp := runtimePrivateCASRecoveryTemp(t, dataDir, "accepted-finals", "records", "01"+strings.Repeat("a", 62))

	lateDigest := "fe" + strings.Repeat("b", 62)
	lateShard := filepath.Join(dataDir, "private", "controlled-artifact-access", "access-receipts", lateDigest[:2])
	if err := os.Mkdir(lateShard, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lateShard, lateDigest+".json"), []byte(`{"schemaVersion":"corrupt"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := recoverRuntimePrivateCASOwners(context.Background(), dataDir, access, nil); err == nil {
		t.Fatal("late semantic corruption passed the global prepared recovery barrier")
	}
	if body, err := os.ReadFile(earlyTemp); err != nil || string(body) != "prepared-residue" {
		t.Fatalf("late semantic corruption consumed earlier residue: body=%q err=%v", body, err)
	}
}

func TestPreparedRuntimePrivateCASRejectsUnknownAndCaseAliasedOwnerLeaves(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "unknown-leaf",
			mutate: func(t *testing.T, checkpointRoot string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(checkpointRoot, "unexpected-authority"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "case-aliased-leaf",
			mutate: func(t *testing.T, checkpointRoot string) {
				t.Helper()
				original := filepath.Join(checkpointRoot, "snapshot-intents")
				alias := filepath.Join(checkpointRoot, "Snapshot-Intents")
				if err := os.Rename(original, alias); err != nil {
					t.Skipf("case alias rename unavailable: %v", err)
				}
				entries, err := os.ReadDir(checkpointRoot)
				if err != nil {
					t.Fatal(err)
				}
				visible := false
				for _, entry := range entries {
					visible = visible || entry.Name() == "Snapshot-Intents"
				}
				if !visible {
					t.Skip("filesystem did not expose the case-only rename")
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
			earlyTemp := runtimePrivateCASRecoveryTemp(t, dataDir, "accepted-finals", "records", "02"+strings.Repeat("c", 62))
			fixture.mutate(t, filepath.Join(dataDir, "private", "checkpoint-authority"))
			if err := recoverRuntimePrivateCASOwners(context.Background(), dataDir, access, nil); err == nil {
				t.Fatal("non-exact owner leaf topology passed prepared recovery")
			}
			if _, err := os.Lstat(earlyTemp); err != nil {
				t.Fatalf("owner topology failure consumed earlier residue: %v", err)
			}
		})
	}
}

func TestPreparedRuntimePrivateCASClassifiesPartialEvidenceRegistryV2AsBoundary(t *testing.T) {
	dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
	root := filepath.Join(dataDir, "private", "evidence-registry")
	presentPath := filepath.Join(root, "indexes")
	indexes, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		presentPath, 256<<10, access,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := indexes.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(presentPath)
	if err != nil {
		t.Fatal(err)
	}
	owners := runtimePrivateCASOwnerRecoveries(dataDir, access)
	prepared, err := prepareAndValidateRuntimePrivateCASOwners(context.Background(), owners, true, nil)
	if err != nil {
		t.Fatalf("safe partial evidence registry blocked prepared owner recovery: %v", err)
	}
	boundaryFound := false
	for _, authority := range prepared {
		if _, ok := authority.(*runtimeEvidenceRegistryBoundaryRecoveryV1); ok {
			boundaryFound = true
			break
		}
	}
	if !boundaryFound {
		t.Fatal("partial evidence registry was not classified as an optional boundary")
	}
	participants, err := runtimePrivateCASRecoveryParticipantsV4(dataDir, owners, prepared, true)
	if err != nil {
		t.Fatalf("partial evidence registry was dropped from the V4 authority set: %v", err)
	}
	if len(participants) != len(owners) {
		t.Fatalf("optional registry changed the V4 participant denominator: participants=%d owners=%d", len(participants), len(owners))
	}
	rootCount := 0
	registryParticipantFound := false
	for _, participant := range participants {
		rootCount += len(participant.Roots)
		if participant.ParticipantID == runtimePrivateCASEvidenceRegistryOwnerV1 {
			registryParticipantFound = true
			if len(participant.Roots) != 2 {
				t.Fatalf("optional registry V4 root manifest = %d, want 2", len(participant.Roots))
			}
		}
	}
	if !registryParticipantFound || rootCount != len(domainprivatecas.RuntimeRootSpecsV1()) {
		t.Fatalf("optional registry was not retained in the complete V4 manifest: found=%v roots=%d", registryParticipantFound, rootCount)
	}
	after, err := os.Stat(presentPath)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("prepared recovery changed the partial evidence registry leaf: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "capsules")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prepared recovery manufactured the missing evidence registry leaf: %v", err)
	}
}

func TestPreparedRuntimePrivateCASLateLeafSwapFailsBeforeAnyApply(t *testing.T) {
	dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
	earlyTemp := runtimePrivateCASRecoveryTemp(t, dataDir, "accepted-finals", "records", "03"+strings.Repeat("d", 62))
	owners := runtimePrivateCASOwnerRecoveries(dataDir, access)
	prepared := make([]runtimePreparedPrivateCASOwnerRecovery, 0, len(owners))
	for _, owner := range owners {
		plan, err := owner.prepare(context.Background())
		if err != nil {
			t.Fatalf("prepare %s: %v", owner.name, err)
		}
		prepared = append(prepared, plan)
	}
	for index, plan := range prepared {
		if err := plan.ValidateSemantics(context.Background()); err != nil {
			t.Fatalf("validate %s: %v", owners[index].name, err)
		}
	}

	leaf := filepath.Join(dataDir, "private", "checkpoint-authority", "snapshot-intents")
	moved := leaf + ".original"
	if err := os.Rename(leaf, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(leaf, 0o700); err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, plan := range prepared {
		if err := plan.Revalidate(context.Background()); err != nil {
			failed = true
			break
		}
	}
	if !failed {
		t.Fatal("late A-to-B owner leaf swap passed the global revalidation barrier")
	}
	if body, err := os.ReadFile(earlyTemp); err != nil || string(body) != "prepared-residue" {
		t.Fatalf("late leaf swap consumed earlier residue: body=%q err=%v", body, err)
	}
}

func TestRuntimePrivateCASSemanticApplyRootIDsUseExactPathIntersection(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "descendant",
			path: "data/private/turn-terminal-authority/dispositions/aa/record.json",
			want: []string{"turn-terminal-authority/dispositions"},
		},
		{
			name: "owner ancestor",
			path: "data/private/turn-terminal-authority",
			want: []string{
				"turn-terminal-authority/dispositions",
				"turn-terminal-authority/intents",
			},
		},
		{
			name: "unrelated durable",
			path: "durable/threads/thread-1/events.jsonl",
			want: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := domainstartup.NewSemanticStartupPlanV1(
				digest, digest, digest,
				[]domainstartup.SemanticStartupOperationV1{{
					Kind: domainstartup.SemanticOperationInstallFile,
					Path: test.path,
					Before: domainstartup.SemanticEntryStateV1{
						Type: domainstartup.ManagedEntryTypeAbsent,
					},
					After: domainstartup.SemanticEntryStateV1{
						Type: domainstartup.ManagedEntryTypeFile,
						Mode: 0o600, Size: 1, SHA256: digest,
					},
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			got, err := runtimePrivateCASSemanticApplyRootIDs(plan)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("affected roots = %v, want %v", got, test.want)
			}
		})
	}

	allPlan, err := domainstartup.NewSemanticStartupPlanV1(
		digest, digest, digest,
		[]domainstartup.SemanticStartupOperationV1{{
			Kind: domainstartup.SemanticOperationSetMode,
			Path: "data/private",
			Before: domainstartup.SemanticEntryStateV1{
				Type: domainstartup.ManagedEntryTypeDirectory, Mode: 0o755,
			},
			After: domainstartup.SemanticEntryStateV1{
				Type: domainstartup.ManagedEntryTypeDirectory, Mode: 0o700,
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	all, err := runtimePrivateCASSemanticApplyRootIDs(allPlan)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(domainprivatecas.RuntimeRootSpecsV1()) {
		t.Fatalf("private ancestor affected %d roots, want %d", len(all), len(domainprivatecas.RuntimeRootSpecsV1()))
	}
}

func TestRuntimePrivateCASRecoveryDoesNotEnumerateOwnersBelowAbsentPrivateRoot(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	frozen, held := lease.FrozenRoots()
	if !held {
		t.Fatal("persistence lease did not freeze roots")
	}
	journal, err := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
	if err != nil {
		t.Fatal(err)
	}
	authority := &countingRuntimePrivateCASAccessAuthority{inner: lease}
	if err := recoverRuntimePrivateCASOwners(context.Background(), frozen.DataDir, authority, nil, journal); err != nil {
		t.Fatal(err)
	}
	if authority.calls != 4 {
		t.Fatalf("absent private recovery state access calls = %d, want 4 stable parent observations", authority.calls)
	}
	if _, err := os.Lstat(filepath.Join(frozen.DataDir, "private")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent private root recovery promoted private state: %v", err)
	}
}

func TestRuntimeSemanticApplyWithoutPrivateCASOperationsDoesNotPrepareOwners(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error(err)
		}
	})
	frozen, held := lease.FrozenRoots()
	if !held {
		t.Fatal("persistence lease did not freeze roots")
	}
	authority := &countingRuntimePrivateCASAccessAuthority{inner: lease}
	applied := false
	if err := withRetiredRuntimePrivateCASOwnerGenerationsForSemanticApply(
		context.Background(), frozen.DataDir, authority, nil, plan,
		func(context.Context) error {
			applied = true
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("semantic apply callback was not invoked")
	}
	if authority.calls != 0 {
		t.Fatalf("no-private semantic apply prepared private CAS owners: calls=%d", authority.calls)
	}
}

func initializedRuntimePrivateCASRecoveryTest(t *testing.T) (string, *privatecastest.AccessAuthority) {
	t.Helper()
	dataDir := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: t.TempDir()}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("initialize runtime private owners: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	access, err := privatecastest.NewAccessAuthority(filepath.Join(dataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	return dataDir, access
}

func runtimePrivateCASRecoveryTemp(
	t *testing.T,
	dataDir string,
	owner string,
	leaf string,
	digest string,
) string {
	t.Helper()
	shard := filepath.Join(dataDir, "private", owner, leaf, digest[:2])
	if err := os.Mkdir(shard, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	temp := filepath.Join(shard, fmt.Sprintf(".%s.json-%024x.tmp", digest, 1))
	if err := os.WriteFile(temp, []byte("prepared-residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	return temp
}
