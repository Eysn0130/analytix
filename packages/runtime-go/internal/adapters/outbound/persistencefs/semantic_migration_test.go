package persistencefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func TestSignedSemanticJournalV3ReturnsTypedMigrationBlocker(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	journal := semanticJournalV3ForTest(t, authority)
	body, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeSemanticJournal(body, authority)
	if !errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("signed V3 journal error = %v", err)
	}
	var blocker SemanticStartupMigrationBlocker
	if !errors.As(err, &blocker) || blocker.Artifact != "journal" || blocker.FoundVersion != 3 ||
		blocker.RequiredVersion != semanticJournalSchemaVersion || blocker.Code != semanticJournalV3MigrationCode() {
		t.Fatalf("signed V3 migration blocker = %#v", blocker)
	}
}

func TestForgedSemanticJournalV3IsIntegrityErrorNotMigrationBlocker(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	journal := semanticJournalV3ForTest(t, authority)
	journal.AuthoritySignature = "forged"
	body, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeSemanticJournal(body, authority)
	if err == nil || errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("forged V3 journal classification = %v", err)
	}
}

func TestSemanticJournalV3AndV4SignatureDomainsAreSeparated(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	journal := semanticJournalV3ForTest(t, authority)
	message := semanticJournalSigningBytes(journal)
	if authority.verifyDomain(semanticJournalV3SignatureDomain, journal.AuthorityKeyID, message, journal.AuthoritySignature) != nil {
		t.Fatal("valid V3 signature was rejected by the V3 domain")
	}
	if authority.verifyDomain(semanticJournalSignatureDomain, journal.AuthorityKeyID, message, journal.AuthoritySignature) == nil {
		t.Fatal("V3 signature was accepted by the V4 domain")
	}
	v4 := semanticJournalV4ForTest(t, authority)
	v4Message := semanticJournalSigningBytes(v4)
	if authority.verifyDomain(semanticJournalSignatureDomain, v4.AuthorityKeyID, v4Message, v4.AuthoritySignature) != nil {
		t.Fatal("valid V4 signature was rejected by the V4 domain")
	}
	if authority.verifyDomain(semanticJournalV3SignatureDomain, v4.AuthorityKeyID, v4Message, v4.AuthoritySignature) == nil {
		t.Fatal("V4 signature was accepted by the V3 domain")
	}
}

func TestSignedSemanticRetirementV1ReturnsTypedMigrationBlocker(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	record := semanticRetirementV1ForTest(t, authority)
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeSemanticRetirement(body, authority)
	if !errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("signed retirement V1 error = %v", err)
	}
	var blocker SemanticStartupMigrationBlocker
	if !errors.As(err, &blocker) || blocker.Artifact != "retirement" || blocker.FoundVersion != 1 ||
		blocker.RequiredVersion != semanticJournalRetirementSchemaVersion || blocker.Code != semanticRetirementV1MigrationCode() {
		t.Fatalf("signed retirement V1 blocker = %#v", blocker)
	}
}

func TestForgedSemanticRetirementV1IsIntegrityErrorNotMigrationBlocker(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	record := semanticRetirementV1ForTest(t, authority)
	record.AuthoritySignature = "forged"
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeSemanticRetirement(body, authority)
	if err == nil || errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("forged retirement V1 classification = %v", err)
	}
}

func TestSemanticRetirementV1AndV2SignatureDomainsAreSeparated(t *testing.T) {
	authority := semanticMigrationAuthorityForTest(t)
	legacy := semanticRetirementV1ForTest(t, authority)
	message := semanticRetirementSigningBytes(legacy)
	if authority.verifyDomain(semanticJournalRetirementV1SignatureDomain, legacy.AuthorityKeyID, message, legacy.AuthoritySignature) != nil {
		t.Fatal("valid retirement V1 signature was rejected by the V1 domain")
	}
	if authority.verifyDomain(semanticJournalRetirementSignatureDomain, legacy.AuthorityKeyID, message, legacy.AuthoritySignature) == nil {
		t.Fatal("retirement V1 signature was accepted by the V2 domain")
	}
}

func TestSemanticPlanningV1MigrationIsPlatformSafe(t *testing.T) {
	marker := semanticPlanningMarkerV1{
		SchemaVersion: 1, Purpose: semanticPlanningMarkerV1Purpose,
		RootBindingDigest: domainsecurity.SHA256Hex([]byte("roots")),
		PlanningIdentity:  "1:2:0", StageName: "stage-legacy", StageIdentity: "1:3:0",
		DataIdentity: "1:4:0", DurableIdentity: "1:5:0",
	}
	marker.MarkerDigest = semanticPlanningMarkerDigest(marker)
	body, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := decodeSemanticPlanningMarker(body)
	if runtime.GOOS == "windows" {
		if !errors.Is(err, ErrSemanticStartupMigrationBlocked) {
			t.Fatalf("Windows planning V1 migration classification = %v", err)
		}
		return
	}
	if err != nil || migrated.SchemaVersion != semanticPlanningMarkerVersion || migrated.Purpose != semanticPlanningMarkerPurpose {
		t.Fatalf("Unix planning V1 migration = %#v, %v", migrated, err)
	}
	for _, identity := range []string{migrated.PlanningIdentity, migrated.StageIdentity, migrated.DataIdentity, migrated.DurableIdentity} {
		if _, err := parseStrongDirectoryIdentity(identity); err != nil {
			t.Fatalf("migrated planning identity %q is invalid: %v", identity, err)
		}
	}
}

func TestForgedSemanticPlanningV1IsNotMigrationBlocker(t *testing.T) {
	marker := semanticPlanningMarkerV1{
		SchemaVersion: 1, Purpose: semanticPlanningMarkerV1Purpose,
		RootBindingDigest: domainsecurity.SHA256Hex([]byte("roots")),
		PlanningIdentity:  "1:2:0", StageName: "stage-legacy", StageIdentity: "1:3:0",
		DataIdentity: "1:4:0", DurableIdentity: "1:5:0", MarkerDigest: domainsecurity.SHA256Hex([]byte("forged")),
	}
	body, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeSemanticPlanningMarker(body)
	if err == nil || errors.Is(err, ErrSemanticStartupMigrationBlocked) {
		t.Fatalf("forged planning V1 classification = %v", err)
	}
}

func semanticMigrationAuthorityForTest(t *testing.T) *startupJournalAuthority {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "startup-authority")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := canonicalPathWithoutCreate(directory)
	if err != nil {
		t.Fatal(err)
	}
	root, err := captureStartupAuthorityRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	namespace := &JournalNamespaceAuthority{root: root}
	authority, err := openOrCreateStartupJournalAuthority(namespace, filepath.Join(directory, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func semanticJournalV3ForTest(t *testing.T, authority *startupJournalAuthority) semanticJournalV1 {
	t.Helper()
	digest := func(value string) string { return domainsecurity.SHA256Hex([]byte(value)) }
	plan, err := domainstartup.NewSemanticStartupPlanV1(digest("baseline"), digest("configuration"), digest("final"), nil)
	if err != nil {
		t.Fatal(err)
	}
	capability := frozenRootCapability{
		Root: "/legacy/data", Anchor: "/legacy/data", AnchorIdentity: "1:2:0", RootIdentity: "1:2:0",
	}
	journal := semanticJournalV1{
		SchemaVersion: 3, State: semanticJournalPreparing,
		RootBindingDigest: digest("roots"), RootCapabilityDigest: rootCapabilityDigest(map[string]frozenRootCapability{"root": capability}),
		RootCapabilities: []frozenRootCapability{capability}, Plan: plan, JournalRootIdentity: "1:3:0",
		AuthorityKeyID: authority.keyID,
	}
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature, err = authority.signDomain(semanticJournalV3SignatureDomain, semanticJournalSigningBytes(journal))
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func semanticJournalV4ForTest(t *testing.T, authority *startupJournalAuthority) semanticJournalV1 {
	t.Helper()
	journal := semanticJournalV3ForTest(t, authority)
	identity := formatUnixObjectIdentity(1, 2)
	journal.SchemaVersion = semanticJournalSchemaVersion
	journal.JournalRootIdentity = formatUnixObjectIdentity(1, 3)
	journal.RootCapabilities[0].AnchorIdentity = identity
	journal.RootCapabilities[0].RootIdentity = identity
	journal.RootCapabilityDigest = rootCapabilityDigest(map[string]frozenRootCapability{"root": journal.RootCapabilities[0]})
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature = ""
	var err error
	journal.AuthoritySignature, err = authority.sign(semanticJournalSigningBytes(journal))
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func semanticRetirementV1ForTest(t *testing.T, authority *startupJournalAuthority) semanticJournalRetirementV1 {
	t.Helper()
	emptyDigest := domainsecurity.SHA256Hex(nil)
	record := semanticJournalRetirementV1{
		SchemaVersion: 1, Disposition: semanticJournalRetirementRollback,
		JournalRootIdentity: "1:2:0", RootBindingDigest: domainsecurity.SHA256Hex([]byte("roots")),
		ConfigurationDigest: domainsecurity.SHA256Hex([]byte("configuration")),
		JournalBodyDigest:   emptyDigest, JournalDigest: emptyDigest,
		JournalState: semanticJournalRetirementEmpty, PlanDigest: emptyDigest,
		ResidueDigest: domainsecurity.SHA256Hex([]byte("[]")), AuthorityKeyID: authority.keyID,
	}
	record.TombstoneDigest = semanticRetirementDigest(record)
	var err error
	record.AuthoritySignature, err = authority.signDomain(
		semanticJournalRetirementV1SignatureDomain,
		semanticRetirementSigningBytes(record),
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
