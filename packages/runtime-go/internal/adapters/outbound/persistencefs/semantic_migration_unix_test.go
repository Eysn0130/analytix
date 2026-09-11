//go:build darwin || linux

package persistencefs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSignedSemanticJournalV3MigratesAtomicallyToV4OnUnix(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	directory, err := secureStartupCreateDirectory(authority.namespace.root, "journal")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := parseStrongDirectoryIdentity(directory.Identity())
	if err != nil {
		t.Fatal(err)
	}
	journal := semanticJournalV3ForTest(t, authority)
	journal.JournalRootIdentity = formatLegacyDirectoryIdentityV3(identity.Device, identity.Inode, 0)
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature, err = authority.signDomain(semanticJournalV3SignatureDomain, semanticJournalSigningBytes(journal))
	if err != nil {
		t.Fatal(err)
	}
	legacyBody, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := directory.WriteExclusive("journal.json", legacyBody, maxStrictJSONBytes); err != nil {
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := readSemanticJournal(filepath.Join(authority.namespace.path(), "journal"), authority)
	if err != nil {
		t.Fatalf("migrate signed V3 journal: %v", err)
	}
	if migrated.SchemaVersion != 4 {
		t.Fatalf("migrated journal schema = %d", migrated.SchemaVersion)
	}
	if _, err := parseStrongDirectoryIdentity(migrated.JournalRootIdentity); err != nil {
		t.Fatalf("migrated journal retained legacy root identity: %v", err)
	}
	root, err := secureStartupOpenDirectory(authority.namespace.root, "journal", migrated.JournalRootIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	body, _, err := root.ReadFile("journal.json", maxStrictJSONBytes, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == string(legacyBody) {
		t.Fatal("V3 migration did not atomically replace the journal")
	}
	if decoded, err := decodeSemanticJournal(body, authority); err != nil || decoded.SchemaVersion != 4 {
		t.Fatalf("migrated journal readback is not valid V4: schema=%d err=%v", decoded.SchemaVersion, err)
	}
}

func TestSemanticJournalV3MigrationCrashCutsConvergeWithoutEarlyManagedMutation(t *testing.T) {
	for _, cut := range []string{"after_journal_temp_sync", "after_journal_replace"} {
		t.Run(cut, func(t *testing.T) {
			roots := semanticRootsForTest(t)
			writeSemanticFixture(t, roots, "before")
			baseline, configurationDigest := semanticBaselineForTest(t, roots)
			builder := NewSemanticPlanBuilder(roots)
			builder.fault = stopSemanticJournalAfterPrepared
			prepared, err := builder.Prepare(context.Background(), baseline, configurationDigest, semanticSimulationForTest("after"))
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			if err := prepared.Apply(context.Background()); err == nil {
				t.Fatal("semantic apply did not stop at the prepared journal")
			}
			journalRoot, err := semanticJournalRoot(roots)
			if err != nil {
				t.Fatal(err)
			}
			namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := loadStartupJournalAuthority(namespace, journalRoot)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := readSemanticJournal(journalRoot, authority)
			if err != nil {
				t.Fatal(err)
			}
			legacy := downgradeSemanticJournalToV3ForUnix(t, journal, authority)
			directory, err := secureStartupOpenDirectory(namespace.root, filepath.Base(journalRoot), journal.JournalRootIdentity)
			if err != nil {
				t.Fatal(err)
			}
			legacyBody, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if err := directory.WriteReplace("journal.json", legacyBody, maxStrictJSONBytes, nil); err != nil {
				_ = directory.Close()
				t.Fatal(err)
			}
			if err := directory.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := CaptureStrict(roots)
			if err != nil {
				t.Fatal(err)
			}
			recovering := NewSemanticPlanBuilder(roots)
			recovering.fault = func(stage string, _ int) error {
				if stage == cut {
					return errors.New("migration crash cut")
				}
				return nil
			}
			if err := recovering.Recover(context.Background(), configurationDigest); err == nil {
				t.Fatalf("migration did not stop at %s", cut)
			}
			afterCut, err := CaptureStrict(roots)
			if err != nil || afterCut.SHA256 != before.SHA256 {
				t.Fatalf("migration cut changed managed state: before=%s after=%s err=%v", before.SHA256, afterCut.SHA256, err)
			}
			if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configurationDigest); err != nil {
				t.Fatalf("migration did not converge after %s: %v", cut, err)
			}
			body, err := os.ReadFile(filepath.Join(roots.DataDir, "private", "fixture.bin"))
			if err != nil || string(body) != "after" {
				t.Fatalf("converged migration did not apply the prepared plan: body=%q err=%v", body, err)
			}
		})
	}
}

func TestMissingFinalSignedV3TempUnixConvergesWithoutManagedMutation(t *testing.T) {
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
	namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := loadStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := secureStartupOpenDirectory(namespace.root, filepath.Base(journalRoot), "")
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
	if tempName == "" {
		_ = directory.Close()
		t.Fatal("missing signed journal temp fixture")
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
	legacy := downgradeSemanticJournalToV3ForUnix(t, journal, authority)
	legacyBody, err := json.Marshal(legacy)
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
	before, err := CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configurationDigest); err != nil {
		t.Fatalf("recover missing-final V3 temp: %v", err)
	}
	after, err := CaptureStrict(roots)
	if err != nil || after.SHA256 != before.SHA256 {
		t.Fatalf("missing-final V3 temp recovery changed managed state: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
	}
	if _, err := os.Lstat(journalRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback-safe V3 temp journal was not retired: %v", err)
	}
}

func TestSignedSemanticRetirementV1WithV3JournalConvergesOnUnix(t *testing.T) {
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
		if stage == "after_journal_retired" {
			return errors.New("stop with a retired V2 journal")
		}
		return nil
	}
	if err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("semantic apply did not stop after journal retirement")
	}
	namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	journalRoot := filepath.Join(namespace.path(), "journal")
	authority, err := loadStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(namespace.path())
	if err != nil {
		t.Fatal(err)
	}
	retiredName := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".retired-journal-") {
			retiredName = entry.Name()
		}
	}
	if retiredName == "" {
		t.Fatal("missing retired journal fixture")
	}
	retiredPath := filepath.Join(namespace.path(), retiredName)
	directory, err := secureStartupOpenDirectory(namespace.root, retiredName, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := directory.ReadFile("journal.json", maxStrictJSONBytes, false)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	journal, err := decodeSemanticJournal(body, authority)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	legacyJournal := downgradeSemanticJournalToV3ForUnix(t, journal, authority)
	legacyJournalBody, err := json.Marshal(legacyJournal)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(retiredPath, "journal.json"), legacyJournalBody, 0o600); err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	current, err := semanticRetirementRecordForDirectory(directory, authority, roots, configurationDigest)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	legacy := current
	legacy.SchemaVersion = 1
	legacy.JournalRootIdentity = unixLegacyIdentityForTest(t, current.JournalRootIdentity)
	legacy.JournalDigest = legacyJournal.JournalDigest
	directoryEntries, err := directory.Entries()
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	visible := directoryEntries[:0]
	for _, entry := range directoryEntries {
		if entry.Name() != semanticJournalRetirementFile {
			visible = append(visible, entry)
		}
	}
	residue, err := semanticRetirementResidueInventory(directory, authority, visible, current)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	for index := range residue {
		if residue[index].Identity != "" {
			residue[index].Identity = unixLegacyIdentityForTest(t, residue[index].Identity)
		}
	}
	residueBody, err := json.Marshal(residue)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	legacy.ResidueDigest = domainsecurity.SHA256Hex(residueBody)
	legacy.TombstoneDigest = semanticRetirementDigest(legacy)
	legacy.AuthoritySignature = ""
	legacy.AuthoritySignature, err = authority.signDomain(
		semanticJournalRetirementV1SignatureDomain,
		semanticRetirementSigningBytes(legacy),
	)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	legacyBody, err := json.Marshal(legacy)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(retiredPath, semanticJournalRetirementFile), legacyBody, 0o600); err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	migratedRetirement, _, err := decodeSemanticRetirementForDirectory(legacyBody, directory, authority, roots)
	if err != nil {
		_ = directory.Close()
		t.Fatalf("inspect retirement V1 migration: %v", err)
	}
	rebuiltRetirement, err := semanticRetirementRecordForDirectory(directory, authority, roots, configurationDigest)
	if err != nil || !sameSemanticRetirement(migratedRetirement, rebuiltRetirement) {
		_ = directory.Close()
		t.Fatalf("retirement V1 migration mismatch:\nmigrated=%#v\nrebuilt=%#v\nerr=%v", migratedRetirement, rebuiltRetirement, err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configurationDigest); err != nil {
		t.Fatalf("recover signed retirement V1: %v", err)
	}
	if _, err := os.Lstat(retiredPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retirement V1 directory was not safely consumed: %v", err)
	}
	assertSemanticFixture(t, roots, "after")
}

func TestCurrentNamedSemanticRetirementV1ResumesBeforeJournalMigrationOnUnix(t *testing.T) {
	roots := semanticRootsForTest(t)
	_, configurationDigest := semanticBaselineForTest(t, roots)
	journalRoot, err := semanticJournalRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := openOrCreateStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := secureStartupCreateDirectory(namespace.root, "journal")
	if err != nil {
		t.Fatal(err)
	}
	identity := directory.Identity()
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	fired := false
	err = retireSemanticJournal(namespace, identity, roots, configurationDigest, func(stage string, _ int) error {
		if stage == "after_journal_retirement_tombstone" {
			fired = true
			return errors.New("stop before journal rename")
		}
		return nil
	})
	if err == nil || !fired {
		t.Fatalf("current retirement fixture did not stop: fired=%v err=%v", fired, err)
	}
	directory, err = secureStartupOpenDirectory(namespace.root, "journal", identity)
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := directory.ReadFile(semanticJournalRetirementFile, maxSemanticRetirementBytes, false)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	record, err := decodeSemanticRetirement(body, authority)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	record.SchemaVersion = 1
	record.JournalRootIdentity = unixLegacyIdentityForTest(t, record.JournalRootIdentity)
	record.TombstoneDigest = semanticRetirementDigest(record)
	record.AuthoritySignature = ""
	record.AuthoritySignature, err = authority.signDomain(
		semanticJournalRetirementV1SignatureDomain,
		semanticRetirementSigningBytes(record),
	)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	legacyBody, err := json.Marshal(record)
	if err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalRoot, semanticJournalRetirementFile), legacyBody, 0o600); err != nil {
		_ = directory.Close()
		t.Fatal(err)
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}
	if err := NewSemanticPlanBuilder(roots).Recover(context.Background(), configurationDigest); err != nil {
		t.Fatalf("recover current named retirement V1: %v", err)
	}
	if _, err := os.Lstat(journalRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current named retirement V1 was not consumed: %v", err)
	}
}

func unixLegacyIdentityForTest(t *testing.T, value string) string {
	t.Helper()
	identity, err := parseStrongDirectoryIdentity(value)
	if err != nil || identity.Device == 0 || identity.Inode == 0 {
		t.Fatalf("cannot downgrade non-Unix identity %q: %v", value, err)
	}
	return formatLegacyDirectoryIdentityV3(identity.Device, identity.Inode, 0)
}

func downgradeSemanticJournalToV3ForUnix(t *testing.T, journal semanticJournalV1, authority *startupJournalAuthority) semanticJournalV1 {
	t.Helper()
	legacyIdentity := func(value string) string {
		return unixLegacyIdentityForTest(t, value)
	}
	journal.SchemaVersion = 3
	journal.JournalRootIdentity = legacyIdentity(journal.JournalRootIdentity)
	if journal.JournalStageIdentity != "" {
		journal.JournalStageIdentity = legacyIdentity(journal.JournalStageIdentity)
	}
	capabilities := make(map[string]frozenRootCapability, len(journal.RootCapabilities))
	for index := range journal.RootCapabilities {
		capability := &journal.RootCapabilities[index]
		capability.AnchorIdentity = legacyIdentity(capability.AnchorIdentity)
		if capability.RootIdentity != "" {
			capability.RootIdentity = legacyIdentity(capability.RootIdentity)
		}
		capabilities[canonicalPathKey(capability.Root)] = *capability
	}
	journal.RootCapabilityDigest = rootCapabilityDigest(capabilities)
	journal.RootPromotionIntents = rootPromotionIntents(journal.RootCapabilities)
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature = ""
	var err error
	journal.AuthoritySignature, err = authority.signDomain(semanticJournalV3SignatureDomain, semanticJournalSigningBytes(journal))
	if err != nil {
		t.Fatal(err)
	}
	return journal
}
