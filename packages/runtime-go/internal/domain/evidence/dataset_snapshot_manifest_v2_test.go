package evidence

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestDatasetSnapshotManifestV2MatchesClosedSourceRowLedgerButGrantsNoClaims(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-manifest-v2")
	first := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	second := sourceRowTestRecordV1(t, policy, binding, "file-a", 2, "artifact-a", "row-b", "lineage-b")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{first, second})
	pages := []SourceRowLedgerPageV1{page}
	manifest, err := newSourceRowDatasetSnapshotManifestV2(sourceRowManifestInputV2ForTest(t, root, pages))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotManifestV2RowRootStructureV1(manifest, root); err != nil {
		t.Fatal(err)
	}
	rootBody, _ := SourceRowLedgerRootV1Bytes(root)
	if manifest.Binding != binding || manifest.ProducerPolicyDigest != policy.PolicyDigest ||
		manifest.ProducerOperationSchemaHash != policy.OperationSchemaHash || manifest.ParserVersion != policy.ParserVersion ||
		manifest.SourceRowLedgerRootDigest != root.RootDigest || manifest.SourceRowLedgerRootSHA256 != domainsecurity.SHA256Hex(rootBody) ||
		manifest.SourceRowLedgerRootByteLength != uint64(len(rootBody)) || manifest.SourceRecordCount != root.RecordCount {
		t.Fatalf("manifest did not bind exact closed producer and row root: %#v", manifest)
	}
	if DatasetSnapshotManifestV2CanAuthorizeClaims(manifest, root) {
		t.Fatal("inactive manifest acquired claim authority without app callback, private CAS, and current witness")
	}
	if domainsecurity.DatasetSnapshotFactAuthorityBlocker(domainsecurity.DeriveDatasetSnapshotIDV2(manifest)) == "" {
		t.Fatal("manifest-derived bare DSV2 id acquired fact authority")
	}
}

func TestDatasetSnapshotManifestV2RejectsWrongPolicyRootAndClassification(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-manifest-v2")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	pages := []SourceRowLedgerPageV1{page}
	input := sourceRowManifestInputV2ForTest(t, root, pages)
	manifest, err := newSourceRowDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	missingPages := input
	missingPages.Pages = nil
	if _, err := newSourceRowDatasetSnapshotManifestV2(missingPages); err == nil {
		t.Fatal("descriptor-only row root was sealed without exact page validation")
	}

	otherRecord := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-other", "lineage-a")
	_, otherRoot := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{otherRecord})
	if ValidateDatasetSnapshotManifestV2RowRootStructureV1(manifest, otherRoot) == nil {
		t.Fatal("manifest matched a different canonical row root with the same counts")
	}

	securityInput := sourceRowSecurityManifestInputV2ForTest(t, manifest, input.FundsProducerContentManifest)
	securityInput.ProducerPolicyDigest = domainsecurity.SHA256Hex([]byte("mcp-self-reported-policy"))
	hostilePolicy, err := domainsecurity.NewDatasetSnapshotManifestV2(securityInput)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateDatasetSnapshotManifestV2RowRootStructureV1(hostilePolicy, root) == nil {
		t.Fatal("structurally valid self-reported producer policy matched the host registry")
	}

	inconsistent := input
	inconsistent.ParsedReceipt.AcceptedCount = 0
	inconsistent.ParsedReceipt.RejectedCount = inconsistent.ParsedReceipt.OutcomeCount
	inconsistent.ParsedReceipt.ReceiptDigest = parsedGenerationReceiptDigestV1(inconsistent.ParsedReceipt)
	if _, err := newSourceRowDatasetSnapshotManifestV2(inconsistent); err == nil {
		t.Fatal("row classification counts exceeding the exact ledger were accepted")
	}

	oversizedReferencedManifest := sourceRowSecurityManifestInputV2ForTest(t, manifest, input.FundsProducerContentManifest)
	oversizedReferencedManifest.RawArtifactManifestByteLength = 16*1024*1024 + 1
	if _, err := domainsecurity.NewDatasetSnapshotManifestV2(oversizedReferencedManifest); err == nil {
		t.Fatal("dataset snapshot referenced a manifest larger than one private-CAS object")
	}
}

func TestDatasetSnapshotManifestV2DerivesOccurrenceCountsFromTypedReceipt(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-manifest-v2-counts")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	ledgerPage, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	input := sourceRowManifestInputV2ForTest(t, root, []SourceRowLedgerPageV1{ledgerPage})
	identity := input.ParsedReceipt.Identity
	fileIDs := parsedOrderedSourceFileIDsCountV1(t, policy, 2)
	accepted := parsedOutcomeFixtureV1(t, identity, policy, binding, 1, fileIDs[0], ParsedOutcomeAcceptedV1)
	accepted.RawArtifactOrdinal = 1
	rejected := parsedOutcomeFixtureV1(t, identity, policy, binding, 2, fileIDs[1], ParsedOutcomeRejectedV1)
	rejected.RawArtifactOrdinal = 1
	parsedPage, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{accepted, rejected})
	if err != nil {
		t.Fatal(err)
	}
	parsedIndex, err := newParsedPageIndexV1(identity, 1, []ParsedPageV1{parsedPage})
	if err != nil {
		t.Fatal(err)
	}
	input.ParsedReceipt, err = newParsedGenerationReceiptV1(identity, []ParsedPageIndexV1{parsedIndex})
	if err != nil {
		t.Fatal(err)
	}
	input.FundsProducerContentManifest = sourceRowFundsProducerContentManifestV1ForTest(t, root, input.ParsedReceipt)
	manifest, err := newSourceRowDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SourceRecordCount != 2 || manifest.AcceptedRecordCount != root.RecordCount ||
		manifest.RejectedRecordCount != 1 || manifest.DuplicateRecordCount != 0 {
		t.Fatalf("snapshot counts were not derived from the exact receipt: %#v", manifest)
	}
	if err := ValidateDatasetSnapshotManifestV2ParsedGenerationV1(manifest, input.ParsedReceipt); err != nil {
		t.Fatal(err)
	}
	if manifest.ClassificationLedgerDigest != manifest.ParsedGenerationReceiptDigest ||
		manifest.ClassificationLedgerSHA256 != manifest.ParsedGenerationReceiptSHA256 ||
		manifest.ClassificationLedgerByteLength != manifest.ParsedGenerationReceiptByteLength {
		t.Fatal("DSV2 accepted a detached classification ledger triple")
	}

	otherReceipt := input.ParsedReceipt
	otherReceipt.RejectedCount = 0
	otherReceipt.DuplicateCount = 1
	otherReceipt.ReceiptDigest = parsedGenerationReceiptDigestV1(otherReceipt)
	if ValidateDatasetSnapshotManifestV2ParsedGenerationV1(manifest, otherReceipt) == nil {
		t.Fatal("DSV2 accepted caller-relabelled parsed classification counts")
	}
}

func sourceRowManifestInputV2ForTest(
	t *testing.T,
	root SourceRowLedgerRootV1,
	pages []SourceRowLedgerPageV1,
) sourceRowDatasetSnapshotManifestInputV2 {
	t.Helper()
	rawManifestDigest := domainsecurity.SHA256Hex([]byte("source-row-raw-manifest-digest"))
	rawManifestSHA256 := domainsecurity.SHA256Hex([]byte("source-row-raw-manifest-bytes"))
	rawManifestByteLength := uint64(512)
	receipt := sourceRowParsedReceiptForTestV1(
		t, root, rawManifestDigest, rawManifestSHA256, rawManifestByteLength,
	)
	return sourceRowDatasetSnapshotManifestInputV2{
		Root: root, Pages: pages,
		FundsProducerContentManifest:  sourceRowFundsProducerContentManifestV1ForTest(t, root, receipt),
		AcquiredAt:                    time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC),
		AcquisitionActorDigest:        domainsecurity.SHA256Hex([]byte("source-row-acquisition-actor")),
		RawArtifactManifestDigest:     rawManifestDigest,
		RawArtifactManifestSHA256:     rawManifestSHA256,
		RawArtifactManifestByteLength: rawManifestByteLength,
		RawArtifactCount:              1,
		ParsedReceipt:                 receipt,
	}
}

func sourceRowParsedReceiptForTestV1(
	t *testing.T,
	root SourceRowLedgerRootV1,
	rawManifestDigest string,
	rawManifestSHA256 string,
	rawManifestByteLength uint64,
) ParsedGenerationReceiptV1 {
	t.Helper()
	policy, ok := ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok {
		t.Fatal("source row policy is unavailable")
	}
	identity, err := newParsedGenerationIdentityV1(parsedGenerationIdentityInputV1{
		Policy: policy, Binding: root.Binding,
		AcquisitionIntentDigest:     domainsecurity.SHA256Hex([]byte("source-row-intent-digest")),
		AcquisitionIntentSHA256:     domainsecurity.SHA256Hex([]byte("source-row-intent-body")),
		AcquisitionIntentByteLength: 512,
		RawArtifactManifestDigest:   rawManifestDigest, RawArtifactManifestSHA256: rawManifestSHA256,
		RawArtifactManifestLength: rawManifestByteLength,
		MappingDigest:             domainsecurity.SHA256Hex([]byte("source-row-mapping")),
		ProjectionSchemaDigest:    domainsecurity.SHA256Hex([]byte("source-row-projection")),
		ReaderModeDigest:          domainsecurity.SHA256Hex([]byte("source-row-reader")),
		ConfigurationDigest:       domainsecurity.SHA256Hex([]byte("source-row-configuration-digest")),
		ConfigurationSHA256:       domainsecurity.SHA256Hex([]byte("source-row-configuration-body")),
		ConfigurationByteLength:   1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	fileIDs := parsedOrderedSourceFileIDsCountV1(t, policy, int(root.RecordCount))
	outcomes := make([]ParsedOutcomeV1, len(fileIDs))
	for index, fileID := range fileIDs {
		outcomes[index] = parsedOutcomeFixtureV1(
			t, identity, policy, root.Binding, uint64(index+1), fileID, ParsedOutcomeAcceptedV1,
		)
		outcomes[index].RawArtifactOrdinal = 1
	}
	pages := make([]ParsedPageV1, 0, (len(outcomes)+int(policy.MaxRowsPerPage)-1)/int(policy.MaxRowsPerPage))
	for start := 0; start < len(outcomes); start += int(policy.MaxRowsPerPage) {
		end := start + int(policy.MaxRowsPerPage)
		if end > len(outcomes) {
			end = len(outcomes)
		}
		page, err := newParsedPageV1(identity, uint64(len(pages)+1), outcomes[start:end])
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, page)
	}
	indexes := make([]ParsedPageIndexV1, 0, (len(pages)+maxParsedPagesPerIndexV1-1)/maxParsedPagesPerIndexV1)
	for start := 0; start < len(pages); start += maxParsedPagesPerIndexV1 {
		end := start + maxParsedPagesPerIndexV1
		if end > len(pages) {
			end = len(pages)
		}
		index, err := newParsedPageIndexV1(identity, uint64(len(indexes)+1), pages[start:end])
		if err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, index)
	}
	receipt, err := newParsedGenerationReceiptV1(identity, indexes)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func sourceRowSecurityManifestInputV2ForTest(
	t *testing.T,
	manifest domainsecurity.DatasetSnapshotManifestV2,
	producer domainsecurity.FundsProducerContentManifestV1,
) domainsecurity.DatasetSnapshotManifestInputV2 {
	t.Helper()
	acquiredAt, err := time.Parse(time.RFC3339Nano, manifest.AcquiredAt)
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.DatasetSnapshotManifestInputV2{
		Binding:                           manifest.Binding,
		FundsProducerContentManifest:      producer,
		AcquisitionMethod:                 manifest.AcquisitionMethod,
		AcquiredAt:                        acquiredAt,
		AcquisitionActorDigest:            manifest.AcquisitionActorDigest,
		RawArtifactManifestDigest:         manifest.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:         manifest.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:     manifest.RawArtifactManifestByteLength,
		RawArtifactCount:                  manifest.RawArtifactCount,
		SourceType:                        manifest.SourceType,
		ProducerPolicyID:                  manifest.ProducerPolicyID,
		ProducerPolicyDigest:              manifest.ProducerPolicyDigest,
		ProducerComponentID:               manifest.ProducerComponentID,
		ProducerComponentVersion:          manifest.ProducerComponentVersion,
		ProducerOperation:                 manifest.ProducerOperation,
		ProducerOperationSchemaHash:       manifest.ProducerOperationSchemaHash,
		ParserID:                          manifest.ParserID,
		ParserVersion:                     manifest.ParserVersion,
		ParsedGenerationReceiptDigest:     manifest.ParsedGenerationReceiptDigest,
		ParsedGenerationReceiptSHA256:     manifest.ParsedGenerationReceiptSHA256,
		ParsedGenerationReceiptByteLength: manifest.ParsedGenerationReceiptByteLength,
		ClassificationLedgerDigest:        manifest.ClassificationLedgerDigest,
		ClassificationLedgerSHA256:        manifest.ClassificationLedgerSHA256,
		ClassificationLedgerByteLength:    manifest.ClassificationLedgerByteLength,
		TimezoneSemantics:                 manifest.TimezoneSemantics,
		CurrencySemantics:                 manifest.CurrencySemantics,
		SourceRowLedgerRootDigest:         manifest.SourceRowLedgerRootDigest,
		SourceRowLedgerRootSHA256:         manifest.SourceRowLedgerRootSHA256,
		SourceRowLedgerRootByteLength:     manifest.SourceRowLedgerRootByteLength,
		SourceRowLedgerPageCount:          manifest.SourceRowLedgerPageCount,
		SourceRecordCount:                 manifest.SourceRecordCount,
		AcceptedRecordCount:               manifest.AcceptedRecordCount,
		RejectedRecordCount:               manifest.RejectedRecordCount,
		DuplicateRecordCount:              manifest.DuplicateRecordCount,
	}
}

func sourceRowFundsProducerContentManifestV1ForTest(
	t *testing.T,
	root SourceRowLedgerRootV1,
	receipt ParsedGenerationReceiptV1,
) domainsecurity.FundsProducerContentManifestV1 {
	t.Helper()
	manifest, err := domainsecurity.NewFundsProducerContentManifestV1(
		domainsecurity.FundsProducerContentManifestInputV1{
			CaseID: root.Binding.CaseID, SourceRevision: 1,
			RawManifestSHA256:       receipt.Identity.RawArtifactManifestSHA256,
			NormalizedContentSHA256: domainsecurity.SHA256Hex([]byte("normalized:" + receipt.ReceiptDigest)),
			DetailContentSHA256:     domainsecurity.SHA256Hex([]byte("detail:" + root.RootDigest)),
			AggregateContentSHA256:  domainsecurity.SHA256Hex([]byte("aggregate:" + root.RootDigest)),
			KeywordContentSHA256:    domainsecurity.SHA256Hex([]byte("keyword:" + root.RootDigest)),
			AccountContentSHA256:    domainsecurity.SHA256Hex([]byte("account:" + root.RootDigest)),
			NormalizedRowCount:      receipt.OutcomeCount,
			AcceptedRowCount:        receipt.AcceptedCount,
			RejectedRowCount:        receipt.RejectedCount,
			DuplicateRowCount:       receipt.DuplicateCount,
			DetailRowCount:          receipt.AcceptedCount,
			AggregateRowCount:       1,
			KeywordRowCount:         1,
			AccountRowCount:         1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
