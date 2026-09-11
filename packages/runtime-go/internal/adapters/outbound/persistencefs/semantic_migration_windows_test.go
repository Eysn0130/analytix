//go:build windows

package persistencefs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSignedSemanticJournalV3WindowsBlocksWithoutRewriting(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	directory, err := secureStartupCreateDirectory(authority.namespace.root, "journal")
	if err != nil {
		t.Fatal(err)
	}
	journal := semanticJournalV3ForTest(t, authority)
	body, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.WriteExclusive("journal.json", body, maxStrictJSONBytes); err != nil {
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = readSemanticJournal(filepath.Join(authority.namespace.path(), "journal"), authority)
	if !errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("Windows V3 migration classification = %v", err)
	}
	reopened, err := secureStartupOpenDirectory(authority.namespace.root, "journal", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	readback, _, err := reopened.ReadFile("journal.json", maxStrictJSONBytes, false)
	if err != nil || string(readback) != string(body) {
		t.Fatalf("Windows V3 blocker rewrote journal: err=%v", err)
	}
	entries, err := reopened.Entries()
	if err != nil || len(entries) != 1 || entries[0].Name() != "journal.json" {
		t.Fatalf("Windows V3 blocker changed journal inventory: entries=%v err=%v", entries, err)
	}
}

func TestMissingFinalSignedV3TempWindowsBlocksWithoutJournalMutation(t *testing.T) {
	roots := semanticRootsForTest(t)
	writeSemanticFixture(t, roots, "before")
	baseline, configurationDigest := semanticBaselineForTest(t, roots)
	builder := NewSemanticPlanBuilder(roots)
	prepared, err := builder.Prepare(context.Background(), baseline, configurationDigest, semanticSimulationForTest("after"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	builder.fault = func(stage string, _ int) error {
		if stage == "after_journal_temp_sync" {
			return errors.New("stop with only a signed journal temp")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop with a journal temp")
	}
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := loadStartupJournalAuthority(builder.journalAuthority, journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := secureStartupOpenDirectory(builder.journalAuthority.root, filepath.Base(journalRoot), "")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := directory.Entries()
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	tempName := ""
	for _, entry := range entries {
		if validSemanticJournalTempName(entry) {
			tempName = entry.Name()
		}
	}
	body, _, err := directory.ReadFile(tempName, maxStrictJSONBytes, false)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	journal, err := decodeSemanticJournal(body, authority)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	journal.SchemaVersion = 3
	journal.JournalRootIdentity = "1:2:3"
	journal.JournalStageIdentity = ""
	for index := range journal.RootCapabilities {
		journal.RootCapabilities[index].AnchorIdentity = "1:2:3"
		if journal.RootCapabilities[index].RootIdentity != "" {
			journal.RootCapabilities[index].RootIdentity = "1:2:3"
		}
	}
	capabilities := make(map[string]frozenRootCapability, len(journal.RootCapabilities))
	for _, capability := range journal.RootCapabilities {
		capabilities[canonicalPathKey(capability.Root)] = capability
	}
	journal.RootCapabilityDigest = rootCapabilityDigest(capabilities)
	journal.RootPromotionIntents = rootPromotionIntents(journal.RootCapabilities)
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature = ""
	journal.AuthoritySignature, err = authority.signDomain(semanticJournalV3SignatureDomain, semanticJournalSigningBytes(journal))
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	legacyBody, err := json.Marshal(journal)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalRoot, tempName), legacyBody, 0o600); err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	err = recoverSemanticJournalWithHook(context.Background(), roots, builder.rootAuthority, builder.journalAuthority, configurationDigest, nil)
	if !errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("Windows missing-final V3 classification = %v", err)
	}
	reopened, err := secureStartupOpenDirectory(builder.journalAuthority.root, filepath.Base(journalRoot), "")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	readback, _, err := reopened.ReadFile(tempName, maxStrictJSONBytes, false)
	if err != nil || string(readback) != string(legacyBody) {
		t.Fatalf("Windows V3 temp blocker rewrote candidate: err=%v", err)
	}
	afterEntries, err := reopened.Entries()
	if err != nil || len(afterEntries) != 1 || afterEntries[0].Name() != tempName {
		t.Fatalf("Windows V3 temp blocker changed journal inventory: entries=%v err=%v", afterEntries, err)
	}
}

func TestSignedSemanticRetirementV1WindowsBlocksWithoutRewriting(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	directory, err := secureStartupCreateDirectory(authority.namespace.root, "journal")
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	record := semanticRetirementV1ForTest(t, authority)
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.WriteExclusive(semanticJournalRetirementFile, body, maxStrictJSONBytes); err != nil {
		t.Fatal(err)
	}
	_, _, err = decodeSemanticRetirementForDirectory(body, directory, authority, RootSet{})
	if !errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("Windows retirement V1 classification = %v", err)
	}
	readback, _, readErr := directory.ReadFile(semanticJournalRetirementFile, maxStrictJSONBytes, false)
	if readErr != nil || string(readback) != string(body) {
		t.Fatalf("Windows retirement V1 blocker rewrote tombstone: err=%v", readErr)
	}
}
