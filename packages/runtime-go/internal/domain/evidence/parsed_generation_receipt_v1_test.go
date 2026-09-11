package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestParsedGenerationReceiptV1IndexesCompleteAcyclicHierarchy(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "receipt")
	fileIDs := parsedOrderedSourceFileIDsCountV1(t, policy, 101)
	outcomes := make([]ParsedOutcomeV1, len(fileIDs))
	for index, fileID := range fileIDs {
		outcomes[index] = parsedOutcomeFixtureV1(
			t, identity, policy, binding, uint64(index+1), fileID, ParsedOutcomeAcceptedV1,
		)
	}
	pageOne, err := newParsedPageV1(identity, 1, outcomes[:100])
	if err != nil {
		t.Fatal(err)
	}
	pageTwo, err := newParsedPageV1(identity, 2, outcomes[100:])
	if err != nil {
		t.Fatal(err)
	}
	pages := []ParsedPageV1{pageOne, pageTwo}
	index, err := newParsedPageIndexV1(identity, 1, pages)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newParsedGenerationReceiptV1(identity, []ParsedPageIndexV1{index})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PageCount != 2 || receipt.OutcomeCount != 101 || receipt.AcceptedCount != 101 ||
		receipt.RejectedCount != 0 || receipt.DuplicateCount != 0 ||
		ValidateParsedGenerationHierarchyV1(receipt, []ParsedPageIndexV1{index}, pages) != nil {
		t.Fatalf("unexpected parsed generation receipt: %#v", receipt)
	}
	receiptBody, err := ParsedGenerationReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	digest, sha256, length, err := ParsedGenerationReceiptClassificationTripleV1(receipt)
	if err != nil || digest != receipt.ReceiptDigest || sha256 != domainsecurity.SHA256Hex(receiptBody) || length != uint64(len(receiptBody)) {
		t.Fatalf("classification alias does not bind the exact receipt: digest=%s sha=%s len=%d err=%v", digest, sha256, length, err)
	}
	if bytes.Contains(receiptBody, []byte("lineageDigest")) || bytes.Contains(receiptBody, []byte("datasetSnapshotId")) {
		t.Fatal("parsed generation receipt reintroduced a downstream dependency")
	}
	if maxParsedGenerationPageCountV1*uint64(policy.MaxRowsPerPage) <= 2_645_472 {
		t.Fatal("bounded parsed generation hierarchy cannot represent the incident-scale record count")
	}
}

func TestParsedGenerationReceiptV1RejectsSubstitutionOmissionAndCountRelabel(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "receipt-hostile")
	fileID := parsedOrderedSourceFileIDsCountV1(t, policy, 1)[0]
	outcome := parsedOutcomeFixtureV1(t, identity, policy, binding, 1, fileID, ParsedOutcomeAcceptedV1)
	page, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{outcome})
	if err != nil {
		t.Fatal(err)
	}
	index, err := newParsedPageIndexV1(identity, 1, []ParsedPageV1{page})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newParsedGenerationReceiptV1(identity, []ParsedPageIndexV1{index})
	if err != nil {
		t.Fatal(err)
	}

	tamperedPage := page
	tamperedPage.Outcomes = append([]ParsedOutcomeV1(nil), page.Outcomes...)
	tamperedPage.Outcomes[0].CanonicalTypedRow = append([]ParsedTypedFieldV1(nil), page.Outcomes[0].CanonicalTypedRow...)
	tamperedPage.Outcomes[0].CanonicalTypedRow[1].Scalar.Value = "99.5"
	tamperedPage.Outcomes[0].CanonicalRowSHA256 = parsedCanonicalRowSHA256V1(tamperedPage.Outcomes[0].CanonicalTypedRow)
	tamperedPage.PageDigest = parsedPageDigestV1(tamperedPage)
	if ValidateParsedGenerationHierarchyV1(receipt, []ParsedPageIndexV1{index}, []ParsedPageV1{tamperedPage}) == nil {
		t.Fatal("parsed page substitution passed an old exact descriptor")
	}
	if ValidateParsedGenerationHierarchyV1(receipt, []ParsedPageIndexV1{index}, nil) == nil {
		t.Fatal("parsed generation accepted omitted page material")
	}

	relabelled := receipt
	relabelled.AcceptedCount = 0
	relabelled.RejectedCount = 1
	relabelled.ReceiptDigest = parsedGenerationReceiptDigestV1(relabelled)
	if ValidateParsedGenerationReceiptV1(relabelled) == nil {
		t.Fatal("parsed generation receipt accepted relabelled outcome totals")
	}

	wrongIndex := index
	wrongIndex.PageDescriptors = append([]ParsedPageDescriptorV1(nil), index.PageDescriptors...)
	wrongIndex.PageDescriptors[0].PageSHA256 = domainsecurity.SHA256Hex([]byte("substituted-page-body"))
	wrongIndex.PageDescriptors[0].DescriptorDigest = parsedPageDescriptorDigestV1(wrongIndex.PageDescriptors[0])
	wrongIndex.IndexPageDigest = parsedPageIndexDigestV1(wrongIndex)
	if ValidateParsedGenerationReceiptHierarchyV1(receipt, []ParsedPageIndexV1{wrongIndex}) == nil {
		t.Fatal("receipt accepted a page-index body substitution")
	}
}

func TestParsedGenerationReceiptV1StrictCanonicalParsing(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "receipt-strict")
	fileID := parsedOrderedSourceFileIDsCountV1(t, policy, 1)[0]
	page, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{
		parsedOutcomeFixtureV1(t, identity, policy, binding, 1, fileID, ParsedOutcomeAcceptedV1),
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := newParsedPageIndexV1(identity, 1, []ParsedPageV1{page})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newParsedGenerationReceiptV1(identity, []ParsedPageIndexV1{index})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ParsedGenerationReceiptV1Bytes(receipt)
	parsed, err := ParseParsedGenerationReceiptV1(body)
	if err != nil || parsed.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatalf("receipt did not round trip: parsed=%#v err=%v", parsed, err)
	}
	unknown := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"unknown":true}`)...)
	if _, err := ParseParsedGenerationReceiptV1(unknown); err == nil {
		t.Fatal("receipt accepted an unknown field")
	}
	duplicate := bytes.Replace(body, []byte(`"pageCount":1`), []byte(`"pageCount":1,"pageCount":1`), 1)
	if _, err := ParseParsedGenerationReceiptV1(duplicate); err == nil {
		t.Fatal("receipt accepted a duplicate key")
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "  "); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseParsedGenerationReceiptV1(indented.Bytes()); err == nil {
		t.Fatal("receipt accepted non-canonical JSON bytes")
	}
}

func TestSourceRowLineageConsumesExactAcceptedParsedOutcome(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "lineage-exact")
	fileID := parsedOrderedSourceFileIDsCountV1(t, policy, 1)[0]
	outcome := parsedOutcomeFixtureV1(t, identity, policy, binding, 1, fileID, ParsedOutcomeAcceptedV1)
	page, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{outcome})
	if err != nil {
		t.Fatal(err)
	}
	index, err := newParsedPageIndexV1(identity, 1, []ParsedPageV1{page})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newParsedGenerationReceiptV1(identity, []ParsedPageIndexV1{index})
	if err != nil {
		t.Fatal(err)
	}
	receiptBody, _ := ParsedGenerationReceiptV1Bytes(receipt)
	pageBody, _ := ParsedPageV1Bytes(page)
	input := sourceRowRecordWithLineageInputV1{
		Policy: policy, Binding: binding,
		Locator:                           SourceRowLocatorInputV1{SourceFileID: fileID, SourceRowNumber: outcome.Locator.SourceRowNumber},
		RawArtifactManifestDigest:         identity.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:         identity.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:     identity.RawArtifactManifestLength,
		RawArtifactManifestPageDigest:     domainsecurity.SHA256Hex([]byte("raw-page-digest")),
		RawArtifactManifestPageSHA256:     domainsecurity.SHA256Hex([]byte("raw-page-body")),
		RawArtifactManifestPageByteLength: 512,
		RawArtifactOrdinal:                outcome.RawArtifactOrdinal,
		RawArtifactEntryDigest:            outcome.RawArtifactEntryDigest,
		RawArtifactEntrySHA256:            domainsecurity.SHA256Hex([]byte("raw-entry-body")),
		RawArtifactEntryByteLength:        512,
		RawSourceLocatorDigest:            domainsecurity.SHA256Hex([]byte("raw-source-locator")),
		RawSourceLocatorSHA256:            domainsecurity.SHA256Hex([]byte("raw-source-locator-body")),
		RawSourceLocatorByteLength:        512,
		SourceArtifactSHA256:              outcome.SourceArtifactSHA256, SourceArtifactByteLength: 1024,
		ParsedGenerationReceiptDigest:     receipt.ReceiptDigest,
		ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex(receiptBody),
		ParsedGenerationReceiptByteLength: uint64(len(receiptBody)),
		ParsedPageDigest:                  page.PageDigest, ParsedPageSHA256: domainsecurity.SHA256Hex(pageBody),
		ParsedPageByteLength: uint64(len(pageBody)), ParsedRowOrdinal: 0,
		MappingDigest: identity.MappingDigest, ProjectionSchemaDigest: identity.ProjectionSchemaDigest,
		ReaderModeDigest: identity.ReaderModeDigest, CanonicalRowSHA256: outcome.CanonicalRowSHA256,
	}
	_, lineage, err := newSourceRowRecordWithLineageV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRowLineageAgainstParsedGenerationV1(lineage, receipt, index, page); err != nil {
		t.Fatal(err)
	}

	fakeCitationPage := page
	fakeCitationPage.Outcomes = append([]ParsedOutcomeV1(nil), page.Outcomes...)
	fakeCitationPage.Outcomes[0].CanonicalTypedRow = append([]ParsedTypedFieldV1(nil), page.Outcomes[0].CanonicalTypedRow...)
	fakeCitationPage.Outcomes[0].CanonicalTypedRow[1].Scalar.Value = "999.25"
	fakeCitationPage.Outcomes[0].CanonicalRowSHA256 = parsedCanonicalRowSHA256V1(fakeCitationPage.Outcomes[0].CanonicalTypedRow)
	fakeCitationPage.PageDigest = parsedPageDigestV1(fakeCitationPage)
	if ValidateSourceRowLineageAgainstParsedGenerationV1(lineage, receipt, index, fakeCitationPage) == nil {
		t.Fatal("lineage accepted a semantically mismatched cited parsed row")
	}

	rejectedOutcome := outcome
	rejectedOutcome.Disposition = ParsedOutcomeRejectedV1
	rejectedOutcome.CanonicalTypedRow = nil
	rejectedOutcome.CanonicalRowSHA256 = ""
	rejectedOutcome.DuplicateKeyDigest = ""
	rejectedOutcome.RejectionStage = ParsedRejectionStageSchemaV1
	rejectedOutcome.RejectionCode = ParsedRejectionMissingRequiredFieldV1
	rejectedPage, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{rejectedOutcome})
	if err != nil {
		t.Fatal(err)
	}
	rejectedIndex, err := newParsedPageIndexV1(identity, 1, []ParsedPageV1{rejectedPage})
	if err != nil {
		t.Fatal(err)
	}
	rejectedReceipt, err := newParsedGenerationReceiptV1(identity, []ParsedPageIndexV1{rejectedIndex})
	if err != nil {
		t.Fatal(err)
	}
	if ValidateSourceRowLineageAgainstParsedGenerationV1(lineage, rejectedReceipt, rejectedIndex, rejectedPage) == nil {
		t.Fatal("lineage upgraded a rejected parsed outcome to accepted evidence")
	}
}

func parsedOrderedSourceFileIDsCountV1(
	t *testing.T,
	policy SourceRowProducerPolicyV1,
	count int,
) []string {
	t.Helper()
	type candidateV1 struct {
		fileID string
		digest string
	}
	candidates := make([]candidateV1, count)
	for index := range candidates {
		fileID := fmt.Sprintf("%020x", index+1)
		locator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{
			SourceFileID: fileID, SourceRowNumber: uint64(index + 1),
		})
		if err != nil {
			t.Fatal(err)
		}
		candidates[index] = candidateV1{fileID: fileID, digest: locator.SourceFileIDDigest}
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].digest < candidates[right].digest })
	ordered := make([]string, len(candidates))
	for index := range candidates {
		ordered[index] = candidates[index].fileID
	}
	return ordered
}
