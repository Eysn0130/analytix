package privatecastopology

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
)

func TestRuntimePrivateCASTopologyIsExactAndHasFourOpaqueRoots(t *testing.T) {
	if err := ValidateRuntimeRootSpecsV1(); err != nil {
		t.Fatal(err)
	}
	specs := RuntimeRootSpecsV1()
	if len(specs) != 56 {
		t.Fatalf("root count = %d", len(specs))
	}
	groups := map[string]int{}
	opaque := []string{}
	for _, spec := range specs {
		groups[spec.RecoveryGroupID]++
		if spec.SnapshotBodyPolicy == SnapshotOpaqueBytesV1 {
			opaque = append(opaque, spec.RelativeCASRoot)
		}
	}
	wantOpaque := []string{
		"evidence-registry/capsules",
		"evidence-registry/indexes",
		"dataset-snapshot-authority/materials",
		"report-publication/artifacts",
	}
	if len(groups) != 19 || !slices.Equal(opaque, wantOpaque) {
		t.Fatalf("topology groups/opaque roots changed: groups=%#v opaque=%#v", groups, opaque)
	}
	if roots := RootsForRecoveryGroupV1("checkpoint-authority"); len(roots) != 6 {
		t.Fatalf("checkpoint roots = %d", len(roots))
	}
}

func TestPrivateCASResidueGrammarCoversWriteAndRecoveryTransactions(t *testing.T) {
	digest := strings.Repeat("a", 64)
	write := "." + digest + ".json-0123456789abcdef01234567.tmp"
	classified, ok := ClassifyRecordResidueNameV1(write, "aa")
	if !ok || classified.Kind != ResidueOrdinaryWriteV1 || classified.OriginalName != write {
		t.Fatalf("write residue = %#v, %v", classified, ok)
	}
	transactionID := strings.Repeat("b", 64)
	for _, kind := range []ResidueKindV1{ResidueRecoveryStageV1, ResidueRecoveryCommitV1} {
		name, ok := RecoveryQuarantineNameV1(kind, transactionID, write, "aa")
		if !ok {
			t.Fatalf("could not build %s residue", kind)
		}
		recovered, ok := ClassifyRecordResidueNameV1(name, "aa")
		if !ok || recovered.Kind != kind || recovered.TransactionID != transactionID ||
			recovered.OriginalName != write {
			t.Fatalf("recovery residue = %#v, %v", recovered, ok)
		}
	}
	malformed := RecoveryStagePrefixV1 + transactionID + "-" +
		base64.RawURLEncoding.EncodeToString([]byte("../"+write))
	if _, ok := ClassifyRecordResidueNameV1(malformed, "aa"); ok {
		t.Fatal("path-bearing recovery residue was accepted")
	}
}

func TestPrivateCASCreateResidueBindsExactComponentOrShard(t *testing.T) {
	owner := CreateDirectoryResidueNameV1("report-publication")
	if !CreateDirectoryResidueMatchesComponentV1(owner, "report-publication") ||
		CreateDirectoryResidueMatchesComponentV1(owner, "Report-Publication") {
		t.Fatal("create residue did not bind the exact component")
	}
	shard := CreateDirectoryResidueNameV1("af")
	if !CreateDirectoryResidueMatchesShardV1(shard) {
		t.Fatal("create residue did not bind a canonical shard")
	}
	if CreateDirectoryResidueMatchesShardV1(CreateDirectoryResidueNameV1("zz")) {
		t.Fatal("create residue accepted a non-shard component")
	}
	for _, name := range []string{
		owner,
		strings.ToUpper(owner),
		".analytix-cas-create-not-a-digest.tmp",
		".analytix-cas-create",
	} {
		if !LooksLikeCreateDirectoryResidueNameV1(name) {
			t.Fatalf("reserved create-residue lookalike was missed: %s", name)
		}
	}
	if LooksLikeCreateDirectoryResidueNameV1(".unrelated-create-residue.tmp") {
		t.Fatal("unrelated directory entered the private CAS create-residue namespace")
	}
}

func TestRuntimePrivateCASDirectoryTopologyIsExact(t *testing.T) {
	if err := ValidateRuntimeDirectorySlotsV1(); err != nil {
		t.Fatal(err)
	}
	slots := FixedDirectorySlotsV1()
	fixedParents := FixedDirectoryParentPathsV1()
	shardParents := ShardParentPathsV1()
	scanParents := CreateRecoveryScanParentPathsV1()
	shards := CanonicalShardComponentsV1()
	ownerGroups := RecoverableOwnerDirectoryGroupsV1()
	if len(slots) != 76 || len(fixedParents) != 21 || len(shardParents) != 56 ||
		len(scanParents) != 77 || len(shards) != 256 || len(ownerGroups) != 15 ||
		MaximumCreateResidueCandidateLocationsV1() != 14_412 {
		t.Fatalf(
			"directory topology counts changed: slots=%d fixedParents=%d shardParents=%d scanParents=%d shards=%d owners=%d candidates=%d",
			len(slots),
			len(fixedParents),
			len(shardParents),
			len(scanParents),
			len(shards),
			len(ownerGroups),
			MaximumCreateResidueCandidateLocationsV1(),
		)
	}
	if slots[0] != (DirectorySlotV1{
		RelativePath:       "private",
		ParentRelativePath: ".",
		Component:          "private",
	}) {
		t.Fatalf("first directory slot = %#v", slots[0])
	}
	byPath := make(map[string]DirectorySlotV1, len(slots))
	for _, slot := range slots {
		byPath[slot.RelativePath] = slot
	}
	for relative, expected := range map[string]DirectorySlotV1{
		"private/case-entity": {
			RelativePath:       "private/case-entity",
			ParentRelativePath: "private",
			Component:          "case-entity",
		},
		"private/case-entity/bindings-v1": {
			RelativePath:       "private/case-entity/bindings-v1",
			ParentRelativePath: "private/case-entity",
			Component:          "bindings-v1",
			CASRoot:            true,
		},
		"private/case-thread-authority": {
			RelativePath:       "private/case-thread-authority",
			ParentRelativePath: "private",
			Component:          "case-thread-authority",
			CASRoot:            true,
		},
		"private/report-publication": {
			RelativePath:       "private/report-publication",
			ParentRelativePath: "private",
			Component:          "report-publication",
		},
		"private/report-publication/artifacts": {
			RelativePath:       "private/report-publication/artifacts",
			ParentRelativePath: "private/report-publication",
			Component:          "artifacts",
			CASRoot:            true,
		},
	} {
		if got := byPath[relative]; got != expected {
			t.Fatalf("directory slot %q = %#v, want %#v", relative, got, expected)
		}
	}
	for _, spec := range RuntimeRootSpecsV1() {
		relative := "private/" + spec.RelativeCASRoot
		if slot, found := byPath[relative]; !found || !slot.CASRoot {
			t.Fatalf("CAS root slot %q = %#v, %v", relative, slot, found)
		}
	}
	if !slices.Contains(fixedParents, ".") ||
		!slices.Contains(scanParents, "private/report-publication/artifacts") ||
		shards[0] != "00" || shards[len(shards)-1] != "ff" {
		t.Fatalf(
			"directory topology boundary changed: fixedParents=%#v scanParents=%#v firstShard=%q lastShard=%q",
			fixedParents,
			scanParents,
			shards[0],
			shards[len(shards)-1],
		)
	}
	reportOwner := OwnerDirectoryGroupV1{}
	continuationOwner := OwnerDirectoryGroupV1{}
	caseEntityOwner := OwnerDirectoryGroupV1{}
	evidenceRegistryOwner := OwnerDirectoryGroupV1{}
	for _, group := range ownerGroups {
		if group.RecoveryGroupID == "case-entity" {
			caseEntityOwner = group
		}
		if group.RecoveryGroupID == "report-publication" {
			reportOwner = group
		}
		if group.RecoveryGroupID == "gate-continuations" {
			continuationOwner = group
		}
		if group.RecoveryGroupID == "evidence-registry" {
			evidenceRegistryOwner = group
		}
	}
	if caseEntityOwner.OwnerRelativePath != "private/case-entity" ||
		!slices.Equal(caseEntityOwner.LeafComponents, []string{"bindings-v1", "ingress-v1", "thread-context-v1"}) ||
		len(caseEntityOwner.MigrationLeafComponents) != 0 {
		t.Fatalf("case entity owner topology = %#v", caseEntityOwner)
	}
	if reportOwner.OwnerRelativePath != "private/report-publication" ||
		len(reportOwner.LeafComponents) != 13 ||
		reportOwner.LeafComponents[0] != "artifacts" ||
		reportOwner.LeafComponents[len(reportOwner.LeafComponents)-1] != "stage-completions" {
		t.Fatalf("report publication owner topology = %#v", reportOwner)
	}
	if continuationOwner.OwnerRelativePath != "private/gate-continuations" ||
		!slices.Equal(continuationOwner.MigrationLeafComponents, []string{"dispositions", "receipts"}) {
		t.Fatalf("continuation migration owner topology = %#v", continuationOwner)
	}
	if evidenceRegistryOwner.OwnerRelativePath != "private/evidence-registry" ||
		!slices.Equal(evidenceRegistryOwner.LeafComponents, []string{"capsules", "indexes"}) ||
		!slices.Equal(evidenceRegistryOwner.FrozenDirectoryComponents, []string{".registry-capsules", ".registry-projections"}) ||
		!slices.Equal(evidenceRegistryOwner.RegularSiblingComponents, []string{".registry-authority-index.json", ".registry.lock"}) ||
		!evidenceRegistryOwner.DeferOrphanRecovery {
		t.Fatalf("evidence registry owner topology = %#v", evidenceRegistryOwner)
	}
}
