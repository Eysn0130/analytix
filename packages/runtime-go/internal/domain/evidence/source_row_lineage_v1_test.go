package evidence

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSourceRowLineageV1BindsExactParsedOccurrenceWithoutPII(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-lineage")
	input := sourceRowLineageTestInputV1(policy, binding, "file-a", 7, "artifact-a", "row-a", "generation-a")
	record, lineage, err := newSourceRowRecordWithLineageV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRowRecordAgainstLineageV1(policy, binding, record, lineage); err != nil {
		t.Fatal(err)
	}
	body, err := SourceRowLineageV1Bytes(lineage)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSourceRowLineageV1(body)
	if err != nil || parsed.LineageDigest != lineage.LineageDigest {
		t.Fatalf("typed lineage did not round-trip canonically: %v", err)
	}
	hostFileID := sourceRowTestHostFileIDV1("file-a")
	for _, forbidden := range []string{
		hostFileID, "file-a", "0012345678901234567", "datasetSnapshotId", "contextDigest", "current", "rawPath", "errorMessage",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("PII, path, diagnostics, or authority state entered lineage material: %q", forbidden)
		}
	}
	if lineage.SourceRecordID != record.SourceRecordID || lineage.Locator != record.Locator ||
		lineage.SourceArtifactSHA256 != record.SourceArtifactSHA256 || lineage.CanonicalRowSHA256 != record.CanonicalRowSHA256 {
		t.Fatal("lineage did not bind the exact stable source occurrence")
	}
}

func TestSourceRowLineageV1KeepsStableRecordKeyButDetectsGenerationRebind(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-lineage-rebind")
	firstInput := sourceRowLineageTestInputV1(policy, binding, "file-a", 7, "artifact-a", "row-a", "generation-a")
	secondInput := sourceRowLineageTestInputV1(policy, binding, "file-a", 7, "artifact-a", "row-a", "generation-b")
	firstRecord, firstLineage, err := newSourceRowRecordWithLineageV1(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	secondRecord, secondLineage, err := newSourceRowRecordWithLineageV1(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if firstRecord.SourceRecordID != secondRecord.SourceRecordID || firstLineage.LineageDigest == secondLineage.LineageDigest ||
		firstRecord.RecordDigest == secondRecord.RecordDigest {
		t.Fatal("generation rebind did not preserve the conflict key and change the typed identity")
	}
	if ValidateSourceRowRecordAgainstLineageV1(policy, binding, firstRecord, secondLineage) == nil {
		t.Fatal("one generation borrowed another generation's stable source record")
	}

	tampered := firstLineage
	tampered.ParsedRowOrdinal++
	tampered.LineageDigest = sourceRowLineageDigestV1(tampered)
	if ValidateSourceRowLineageV1(tampered) != nil {
		t.Fatal("test failed to construct structurally valid hostile lineage")
	}
	if ValidateSourceRowRecordAgainstLineageV1(policy, binding, firstRecord, tampered) == nil {
		t.Fatal("record accepted a different parsed page ordinal")
	}
}

func TestSourceRowLineageV1RejectsUnknownFieldsAndNonCanonicalEncoding(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-lineage-encoding")
	_, lineage, err := newSourceRowRecordWithLineageV1(
		sourceRowLineageTestInputV1(policy, binding, "file-a", 1, "artifact-a", "row-a", "generation-a"),
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := SourceRowLineageV1Bytes(lineage)
	if err != nil {
		t.Fatal(err)
	}
	var external map[string]any
	if err := json.Unmarshal(body, &external); err != nil {
		t.Fatal(err)
	}
	external["mcpReportedAuthority"] = true
	unknown, _ := json.Marshal(external)
	if _, err := ParseSourceRowLineageV1(unknown); err == nil {
		t.Fatal("unknown external field entered typed row lineage")
	}
	pretty, _ := json.MarshalIndent(lineage, "", "  ")
	if _, err := ParseSourceRowLineageV1(pretty); err == nil {
		t.Fatal("noncanonical row lineage encoding was accepted")
	}
	tampered := lineage
	tampered.ParsedPageByteLength = policy.MaxPageBytes + 1
	tampered.LineageDigest = sourceRowLineageDigestV1(tampered)
	if ValidateSourceRowLineageV1(tampered) == nil {
		t.Fatal("lineage referenced a parsed page larger than the closed producer budget")
	}
}

func TestSourceRowLineageV1ConsumesExactRawHierarchyAndClosedSnapshotProducer(t *testing.T) {
	hierarchy := rawArtifactHierarchyForTestV1(t, 1, "lineage-exact")
	binding := sourceRowTestBindingV1(t, "case-raw-lineage-exact")
	policy := sourceRowTestPolicyV1(t)
	manifestBody, _ := RawArtifactManifestV1Bytes(hierarchy.manifest)
	pageBody, _ := RawArtifactManifestPageV1Bytes(hierarchy.pages[0])
	entryBody, _ := RawArtifactEntryV1Bytes(hierarchy.entries[0])
	locatorBody, _ := RawArtifactSourceLocatorV1Bytes(hierarchy.locators[0])
	sourceFileID := domainsecurity.SHA256Hex([]byte("raw-file:lineage-exact:0"))[:20]
	parsedIdentity, err := newParsedGenerationIdentityV1(parsedGenerationIdentityInputV1{
		Policy: policy, Binding: binding,
		AcquisitionIntentDigest:     hierarchy.intent.IntentDigest,
		AcquisitionIntentSHA256:     domainsecurity.SHA256Hex(mustRawArtifactIntentBytesV1(t, hierarchy.intent)),
		AcquisitionIntentByteLength: uint64(len(mustRawArtifactIntentBytesV1(t, hierarchy.intent))),
		RawArtifactManifestDigest:   hierarchy.manifest.ManifestDigest,
		RawArtifactManifestSHA256:   domainsecurity.SHA256Hex(manifestBody),
		RawArtifactManifestLength:   uint64(len(manifestBody)),
		MappingDigest:               domainsecurity.SHA256Hex([]byte("lineage-exact-mapping")),
		ProjectionSchemaDigest:      domainsecurity.SHA256Hex([]byte("lineage-exact-projection")),
		ReaderModeDigest:            domainsecurity.SHA256Hex([]byte("lineage-exact-reader")),
		ConfigurationDigest:         domainsecurity.SHA256Hex([]byte("lineage-exact-configuration-digest")),
		ConfigurationSHA256:         domainsecurity.SHA256Hex([]byte("lineage-exact-configuration-body")),
		ConfigurationByteLength:     1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsedLocator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{SourceFileID: sourceFileID, SourceRowNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	parsedFields := []ParsedTypedFieldV1{
		{Name: "account", Scalar: ParsedTypedScalarV1{Kind: "text", Value: "0012345678901234567"}},
		{Name: "amount", Scalar: ParsedTypedScalarV1{Kind: "decimal", Value: "10.25"}},
	}
	parsedRecordID, err := DeriveSourceRowRecordIDV1(policy, binding, hierarchy.contentRoots[0].FullSHA256, parsedLocator)
	if err != nil {
		t.Fatal(err)
	}
	parsedOutcome := ParsedOutcomeV1{
		SchemaVersion: ParsedOutcomeSchemaVersionV1, Purpose: ParsedOutcomePurposeV1,
		Disposition: ParsedOutcomeAcceptedV1, OccurrenceOrdinal: 1,
		RawArtifactOrdinal:     hierarchy.entries[0].ArtifactOrdinal,
		RawArtifactEntryDigest: hierarchy.entries[0].EntryDigest,
		SourceRecordID:         parsedRecordID, Locator: parsedLocator,
		SourceArtifactSHA256: hierarchy.contentRoots[0].FullSHA256,
		ReaderRecordSHA256:   domainsecurity.SHA256Hex([]byte("lineage-exact-reader-record")),
		CanonicalTypedRow:    parsedFields, CanonicalRowSHA256: parsedCanonicalRowSHA256V1(parsedFields),
		DuplicateKeyDigest: domainsecurity.SHA256Hex([]byte("lineage-exact-dedupe-key")),
	}
	parsedPage, err := newParsedPageV1(parsedIdentity, 1, []ParsedOutcomeV1{parsedOutcome})
	if err != nil {
		t.Fatal(err)
	}
	parsedIndex, err := newParsedPageIndexV1(parsedIdentity, 1, []ParsedPageV1{parsedPage})
	if err != nil {
		t.Fatal(err)
	}
	parsedReceipt, err := newParsedGenerationReceiptV1(parsedIdentity, []ParsedPageIndexV1{parsedIndex})
	if err != nil {
		t.Fatal(err)
	}
	parsedReceiptBody, _ := ParsedGenerationReceiptV1Bytes(parsedReceipt)
	parsedPageBody, _ := ParsedPageV1Bytes(parsedPage)
	input := sourceRowRecordWithLineageInputV1{
		Policy: policy, Binding: binding,
		Locator:                           SourceRowLocatorInputV1{SourceFileID: sourceFileID, SourceRowNumber: 1},
		RawArtifactManifestDigest:         hierarchy.manifest.ManifestDigest,
		RawArtifactManifestSHA256:         domainsecurity.SHA256Hex(manifestBody),
		RawArtifactManifestByteLength:     uint64(len(manifestBody)),
		RawArtifactManifestPageDigest:     hierarchy.pages[0].PageDigest,
		RawArtifactManifestPageSHA256:     domainsecurity.SHA256Hex(pageBody),
		RawArtifactManifestPageByteLength: uint64(len(pageBody)),
		RawArtifactOrdinal:                hierarchy.entries[0].ArtifactOrdinal,
		RawArtifactEntryDigest:            hierarchy.entries[0].EntryDigest,
		RawArtifactEntrySHA256:            domainsecurity.SHA256Hex(entryBody),
		RawArtifactEntryByteLength:        uint64(len(entryBody)),
		RawSourceLocatorDigest:            hierarchy.locators[0].LocatorDigest,
		RawSourceLocatorSHA256:            domainsecurity.SHA256Hex(locatorBody),
		RawSourceLocatorByteLength:        uint64(len(locatorBody)),
		SourceArtifactSHA256:              hierarchy.contentRoots[0].FullSHA256,
		SourceArtifactByteLength:          hierarchy.contentRoots[0].ArtifactByteLength,
		ParsedGenerationReceiptDigest:     parsedReceipt.ReceiptDigest,
		ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex(parsedReceiptBody),
		ParsedGenerationReceiptByteLength: uint64(len(parsedReceiptBody)),
		ParsedPageDigest:                  parsedPage.PageDigest,
		ParsedPageSHA256:                  domainsecurity.SHA256Hex(parsedPageBody),
		ParsedPageByteLength:              uint64(len(parsedPageBody)),
		ParsedRowOrdinal:                  0,
		MappingDigest:                     parsedIdentity.MappingDigest,
		ProjectionSchemaDigest:            parsedIdentity.ProjectionSchemaDigest,
		ReaderModeDigest:                  parsedIdentity.ReaderModeDigest,
		CanonicalRowSHA256:                parsedOutcome.CanonicalRowSHA256,
	}
	record, lineage, err := newSourceRowRecordWithLineageV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRowLineageAgainstRawArtifactV1(
		lineage, hierarchy.intent, hierarchy.manifest, hierarchy.pages, hierarchy.entries[0], hierarchy.locators[0], hierarchy.contentRoots[0],
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRowLineageAgainstParsedGenerationV1(lineage, parsedReceipt, parsedIndex, parsedPage); err != nil {
		t.Fatal(err)
	}

	wrongLocator := hierarchy.locators[0]
	wrongLocator.SourceFileIDDigest = domainsecurity.SHA256Hex([]byte("other-file"))
	wrongLocator.LocatorDigest = rawArtifactSourceLocatorDigestV1(wrongLocator)
	if ValidateSourceRowLineageAgainstRawArtifactV1(
		lineage, hierarchy.intent, hierarchy.manifest, hierarchy.pages, hierarchy.entries[0], wrongLocator, hierarchy.contentRoots[0],
	) == nil {
		t.Fatal("lineage accepted a substituted source locator")
	}

	ledgerPage, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	snapshotInput := sourceRowManifestInputV2ForTest(t, root, []SourceRowLedgerPageV1{ledgerPage})
	acquiredAt, err := time.Parse(time.RFC3339Nano, hierarchy.manifest.AcquiredAt)
	if err != nil {
		t.Fatal(err)
	}
	snapshotInput.AcquiredAt = acquiredAt
	snapshotInput.AcquisitionActorDigest = hierarchy.manifest.AcquisitionActorDigest
	snapshotInput.RawArtifactManifestDigest = hierarchy.manifest.ManifestDigest
	snapshotInput.RawArtifactManifestSHA256 = domainsecurity.SHA256Hex(manifestBody)
	snapshotInput.RawArtifactManifestByteLength = uint64(len(manifestBody))
	snapshotInput.RawArtifactCount = hierarchy.manifest.ArtifactCount
	snapshotInput.ParsedReceipt = parsedReceipt
	snapshotInput.FundsProducerContentManifest = sourceRowFundsProducerContentManifestV1ForTest(t, root, parsedReceipt)
	snapshot, err := newSourceRowDatasetSnapshotManifestV2(snapshotInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRowLineageAgainstSnapshotManifestV2(lineage, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotManifestV2RawArtifactHierarchyV1(
		snapshot, hierarchy.intent, hierarchy.manifest, hierarchy.pages,
	); err != nil {
		t.Fatal(err)
	}

	hostileInput := sourceRowSecurityManifestInputV2ForTest(t, snapshot, snapshotInput.FundsProducerContentManifest)
	hostileInput.ProducerComponentVersion = "mcp.self-reported/v1"
	hostileProducer, err := domainsecurity.NewDatasetSnapshotManifestV2(hostileInput)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateSourceRowLineageAgainstSnapshotManifestV2(lineage, hostileProducer) == nil {
		t.Fatal("lineage accepted a structurally valid snapshot with a non-registered producer component")
	}

	wrongCountInput := sourceRowSecurityManifestInputV2ForTest(t, snapshot, snapshotInput.FundsProducerContentManifest)
	wrongCountInput.RawArtifactCount++
	wrongCount, err := domainsecurity.NewDatasetSnapshotManifestV2(wrongCountInput)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateDatasetSnapshotManifestV2RawArtifactHierarchyV1(
		wrongCount, hierarchy.intent, hierarchy.manifest, hierarchy.pages,
	) == nil {
		t.Fatal("snapshot raw artifact count detached from the exact manifest hierarchy")
	}
}

func mustRawArtifactIntentBytesV1(t *testing.T, intent RawArtifactAcquisitionIntentV1) []byte {
	t.Helper()
	body, err := RawArtifactAcquisitionIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func sourceRowLineageTestInputV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	fileSeed string,
	rowNumber uint64,
	artifactSeed string,
	rowSeed string,
	generationSeed string,
) sourceRowRecordWithLineageInputV1 {
	return sourceRowRecordWithLineageInputV1{
		Policy: policy, Binding: binding,
		Locator:                           SourceRowLocatorInputV1{SourceFileID: sourceRowTestHostFileIDV1(fileSeed), SourceRowNumber: rowNumber},
		RawArtifactManifestDigest:         domainsecurity.SHA256Hex([]byte("raw-manifest-digest:" + artifactSeed)),
		RawArtifactManifestSHA256:         domainsecurity.SHA256Hex([]byte("raw-manifest-bytes:" + artifactSeed)),
		RawArtifactManifestByteLength:     512,
		RawArtifactManifestPageDigest:     domainsecurity.SHA256Hex([]byte("raw-manifest-page-digest:" + artifactSeed)),
		RawArtifactManifestPageSHA256:     domainsecurity.SHA256Hex([]byte("raw-manifest-page-bytes:" + artifactSeed)),
		RawArtifactManifestPageByteLength: 512,
		RawArtifactOrdinal:                1,
		RawArtifactEntryDigest:            domainsecurity.SHA256Hex([]byte("raw-entry:" + artifactSeed)),
		RawArtifactEntrySHA256:            domainsecurity.SHA256Hex([]byte("raw-entry-bytes:" + artifactSeed)),
		RawArtifactEntryByteLength:        512,
		RawSourceLocatorDigest:            domainsecurity.SHA256Hex([]byte("raw-source-locator:" + artifactSeed)),
		RawSourceLocatorSHA256:            domainsecurity.SHA256Hex([]byte("raw-source-locator-bytes:" + artifactSeed)),
		RawSourceLocatorByteLength:        512,
		SourceArtifactSHA256:              domainsecurity.SHA256Hex([]byte(artifactSeed)),
		SourceArtifactByteLength:          1024,
		ParsedGenerationReceiptDigest:     domainsecurity.SHA256Hex([]byte("generation-digest:" + generationSeed)),
		ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex([]byte("generation-bytes:" + generationSeed)),
		ParsedGenerationReceiptByteLength: 640,
		ParsedPageDigest:                  domainsecurity.SHA256Hex([]byte("parsed-page-digest:" + generationSeed)),
		ParsedPageSHA256:                  domainsecurity.SHA256Hex([]byte("parsed-page-bytes:" + generationSeed)),
		ParsedPageByteLength:              512,
		ParsedRowOrdinal:                  uint32((rowNumber - 1) % uint64(policy.MaxRowsPerPage)),
		MappingDigest:                     domainsecurity.SHA256Hex([]byte("mapping:" + generationSeed)),
		ProjectionSchemaDigest:            domainsecurity.SHA256Hex([]byte("projection:" + generationSeed)),
		ReaderModeDigest:                  domainsecurity.SHA256Hex([]byte("reader:" + generationSeed)),
		CanonicalRowSHA256:                domainsecurity.SHA256Hex([]byte(rowSeed)),
	}
}
