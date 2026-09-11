package evidence

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSourceRowLedgerV1BuildsCanonicalPIIFreePagedRoot(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	first := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	second := sourceRowTestRecordV1(t, policy, binding, "file-a", 2, "artifact-a", "row-b", "lineage-b")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{first, second})
	indexPage := sourceRowTestIndexPageV1(t, policy, binding, root, page)
	dataDescriptor := indexPage.PageDescriptors[0]
	hostFileID := sourceRowTestHostFileIDV1("file-a")
	if root.IndexPageCount != 1 || root.PageCount != 1 || root.RecordCount != 2 ||
		root.IndexPageDescriptors[0].IndexPageDigest != indexPage.IndexPageDigest || dataDescriptor.PageDigest != page.PageDigest ||
		dataDescriptor.PageSHA256 == "" || dataDescriptor.PageByteLength == 0 ||
		dataDescriptor.FirstSourceFileIDDigest != first.Locator.SourceFileIDDigest ||
		dataDescriptor.FirstSourceRowNumber != first.Locator.SourceRowNumber ||
		dataDescriptor.LastSourceFileIDDigest != second.Locator.SourceFileIDDigest ||
		dataDescriptor.LastSourceRowNumber != second.Locator.SourceRowNumber {
		t.Fatalf("row ledger root did not content-address its page: %#v", root)
	}
	pageBody, err := SourceRowLedgerPageV1Bytes(page)
	if err != nil {
		t.Fatal(err)
	}
	rootBody, err := SourceRowLedgerRootV1Bytes(root)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, err := SourceRowLedgerIndexPageV1Bytes(indexPage)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{pageBody, indexBody, rootBody} {
		for _, forbidden := range []string{"datasetSnapshotId", "sourceManifestHash", "contextDigest", hostFileID, "file-a", "accountId", "entityId", "001234567890"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("PII or hash-cycle material entered row ledger metadata: %s", forbidden)
			}
		}
	}
	parsedPage, err := ParseSourceRowLedgerPageV1(pageBody)
	if err != nil || parsedPage.PageDigest != page.PageDigest {
		t.Fatalf("canonical page round-trip failed: %v", err)
	}
	parsedRoot, err := ParseSourceRowLedgerRootV1(rootBody)
	if err != nil || parsedRoot.RootDigest != root.RootDigest {
		t.Fatalf("canonical root round-trip failed: %v", err)
	}
	parsedIndex, err := ParseSourceRowLedgerIndexPageV1(indexBody)
	if err != nil || parsedIndex.IndexPageDigest != indexPage.IndexPageDigest {
		t.Fatalf("canonical index-page round-trip failed: %v", err)
	}
}

func TestSourceRowRecordIDV1UsesStableSourceLocatorButDetectsRowTamper(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	first := sourceRowTestRecordV1(t, policy, binding, "file-a", 7, "artifact-a", "row-a", "lineage-a")
	sameLocator := sourceRowTestRecordV1(t, policy, binding, "file-a", 7, "artifact-a", "row-a", "lineage-a")
	tamperedRow := sourceRowTestRecordV1(t, policy, binding, "file-a", 7, "artifact-a", "row-tampered", "lineage-a")
	changedArtifact := sourceRowTestRecordV1(t, policy, binding, "file-a", 7, "artifact-b", "row-a", "lineage-a")
	otherCaseBinding := sourceRowTestBindingV1(t, "case-b")
	otherCase := sourceRowTestRecordV1(t, policy, otherCaseBinding, "file-a", 7, "artifact-a", "row-a", "lineage-a")
	if first.SourceRecordID != sameLocator.SourceRecordID {
		t.Fatal("same source locator minted an alias")
	}
	if first.SourceRecordID != tamperedRow.SourceRecordID || first.CanonicalRowSHA256 == tamperedRow.CanonicalRowSHA256 {
		t.Fatal("row tamper did not preserve the conflict key or changed no row identity")
	}
	if first.SourceRecordID == changedArtifact.SourceRecordID || first.SourceRecordID == otherCase.SourceRecordID {
		t.Fatal("artifact or case binding change reused the prior record id")
	}
	if _, _, err := sourceRowTestLedgerV1Error(policy, binding, []SourceRowRecordV1{first, tamperedRow}); err == nil {
		t.Fatal("same source locator with different row content entered one ledger")
	}
}

func TestSourceRowRecordIDV1ExcludesPaginationButPagePathsRemainDeterministic(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 7, "artifact-a", "row-a", "lineage-a")
	pageOne, err := NewSourceRowLedgerPageV1(policy, binding, 1, []SourceRowRecordV1{record})
	if err != nil {
		t.Fatal(err)
	}
	prefix := sourceRowTestRecordV1(t, policy, binding, "file-a", 6, "artifact-a", "row-prefix", "lineage-prefix")
	pageNine, err := NewSourceRowLedgerPageV1(policy, binding, 9, []SourceRowRecordV1{prefix, record})
	if err != nil {
		t.Fatal(err)
	}
	if pageOne.Entries[0].Record.SourceRecordID != pageNine.Entries[1].Record.SourceRecordID ||
		pageNine.Entries[1].RecordPath != "/rows/1" {
		t.Fatal("pagination changed stable row identity or failed to derive transport paths")
	}
}

func TestSourceRowLedgerV1RejectsCrossPageDuplicateLocator(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	records := make([]SourceRowRecordV1, 0, policy.MaxRowsPerPage)
	for rowNumber := uint64(1); rowNumber <= uint64(policy.MaxRowsPerPage); rowNumber++ {
		seed := strconv.FormatUint(rowNumber, 10)
		records = append(records, sourceRowTestRecordV1(
			t, policy, binding, "file-a", rowNumber, "artifact-a", "row-"+seed, "lineage-a",
		))
	}
	first, err := NewSourceRowLedgerPageV1(policy, binding, 1, records)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSourceRowLedgerPageV1(policy, binding, 2, []SourceRowRecordV1{records[len(records)-1]})
	if err != nil {
		t.Fatal(err)
	}
	_, err = newSourceRowLedgerRootV1FromPages(policy, binding, []SourceRowLedgerPageV1{first, second})
	requireSourceRowErrorContainsV1(t, err, "locator boundaries are not strictly ordered")
}

func TestSourceRowLedgerV1RejectsNonTerminalPartialDataPage(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-page-partition")
	first := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	second := sourceRowTestRecordV1(t, policy, binding, "file-a", 2, "artifact-a", "row-b", "lineage-b")
	firstPage, err := NewSourceRowLedgerPageV1(policy, binding, 1, []SourceRowRecordV1{first})
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := NewSourceRowLedgerPageV1(policy, binding, 2, []SourceRowRecordV1{second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = newSourceRowLedgerRootV1FromPages(policy, binding, []SourceRowLedgerPageV1{firstPage, secondPage})
	requireSourceRowErrorContainsV1(t, err, "partial non-terminal data page")
}

func TestSourceRowLedgerV1RejectsUnsortedPageAndCrossPageRollback(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-ordering")
	first := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	second := sourceRowTestRecordV1(t, policy, binding, "file-a", 2, "artifact-a", "row-b", "lineage-b")
	if _, err := NewSourceRowLedgerPageV1(policy, binding, 1, []SourceRowRecordV1{second, first}); err == nil {
		t.Fatal("one page accepted descending source locators")
	}
	forward := make([]SourceRowRecordV1, 0, policy.MaxRowsPerPage)
	for rowNumber := uint64(2); rowNumber <= uint64(policy.MaxRowsPerPage)+1; rowNumber++ {
		seed := strconv.FormatUint(rowNumber, 10)
		forward = append(forward, sourceRowTestRecordV1(
			t, policy, binding, "file-a", rowNumber, "artifact-a", "row-"+seed, "lineage-a",
		))
	}
	firstPage, err := NewSourceRowLedgerPageV1(policy, binding, 1, forward)
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := NewSourceRowLedgerPageV1(policy, binding, 2, []SourceRowRecordV1{first})
	if err != nil {
		t.Fatal(err)
	}
	_, err = newSourceRowLedgerRootV1FromPages(policy, binding, []SourceRowLedgerPageV1{firstPage, secondPage})
	requireSourceRowErrorContainsV1(t, err, "locator boundaries are not strictly ordered")

	_, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{first, second})
	forgedBoundary := sourceRowTestCloneRootV1(t, root)
	forgedBoundary.IndexPageDescriptors[0].FirstSourceRowNumber++
	forgedBoundary.IndexPageDescriptors[0].DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(forgedBoundary.IndexPageDescriptors[0])
	forgedBoundary.RootDigest = sourceRowLedgerRootDigestV1(forgedBoundary)
	if ValidateSourceRowLedgerRootV1(policy, forgedBoundary) == nil {
		t.Fatal("descriptor boundary detached from its policy-derived locator digest")
	}
}

func TestSourceRowLedgerV1AllowsIdenticalRowsAtDifferentSourceOrdinals(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	first := sourceRowTestRecordV1(t, policy, binding, "file-a", 10, "artifact-a", "same-row", "lineage-a")
	second := sourceRowTestRecordV1(t, policy, binding, "file-a", 11, "artifact-a", "same-row", "lineage-a")
	if first.CanonicalRowSHA256 != second.CanonicalRowSHA256 || first.SourceRecordID == second.SourceRecordID {
		t.Fatal("source ordinal did not distinguish legitimate duplicate row content")
	}
	_, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{first, second})
	if root.RecordCount != 2 {
		t.Fatal("legitimate duplicate row content was dropped")
	}
}

func TestSourceRowLedgerV1RejectsPathPageRootAndUnknownPolicyTamper(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	tamperedPage := sourceRowTestClonePageV1(t, page)
	tamperedPage.Entries[0].RecordPath = "/_meta/rows/0"
	tamperedPage.Entries[0].EntryDigest = sourceRowLedgerPageEntryDigestV1(tamperedPage.Entries[0])
	tamperedPage.PageDigest = sourceRowLedgerPageDigestV1(tamperedPage)
	if ValidateSourceRowLedgerPageV1(policy, tamperedPage) == nil {
		t.Fatal("metadata path tamper passed the static path policy")
	}
	tamperedPage = sourceRowTestClonePageV1(t, page)
	tamperedPage.PageNumber = 2
	tamperedPage.PageDigest = sourceRowLedgerPageDigestV1(tamperedPage)
	if validateSourceRowLedgerRootWithPagesV1(policy, root, []SourceRowLedgerPageV1{tamperedPage}) == nil {
		t.Fatal("page/root numbering mismatch was accepted")
	}
	tamperedRoot := sourceRowTestCloneRootV1(t, root)
	tamperedRoot.IndexPageDescriptors[0].FirstDataPageNumber = 2
	tamperedRoot.IndexPageDescriptors[0].LastDataPageNumber = 2
	tamperedRoot.IndexPageDescriptors[0].DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(tamperedRoot.IndexPageDescriptors[0])
	tamperedRoot.RootDigest = sourceRowLedgerRootDigestV1(tamperedRoot)
	if ValidateSourceRowLedgerRootV1(policy, tamperedRoot) == nil {
		t.Fatal("non-contiguous ledger root page numbering was accepted")
	}
	unknown := policy
	unknown.PolicyID = "mcp.dynamic-policy/v1"
	unknown.PolicyDigest = sourceRowProducerPolicyDigestV1(unknown)
	if _, _, err := newSourceRowRecordWithLineageV1(sourceRowRecordWithLineageInputV1{
		Policy: unknown, Binding: binding,
		Locator:                           SourceRowLocatorInputV1{SourceFileID: strings.Repeat("a", 20), SourceRowNumber: 1},
		RawArtifactManifestDigest:         domainsecurity.SHA256Hex([]byte("raw-manifest-digest")),
		RawArtifactManifestSHA256:         domainsecurity.SHA256Hex([]byte("raw-manifest-bytes")),
		RawArtifactManifestByteLength:     512,
		RawArtifactManifestPageDigest:     domainsecurity.SHA256Hex([]byte("raw-manifest-page-digest")),
		RawArtifactManifestPageSHA256:     domainsecurity.SHA256Hex([]byte("raw-manifest-page-bytes")),
		RawArtifactManifestPageByteLength: 512,
		RawArtifactOrdinal:                1,
		RawArtifactEntryDigest:            domainsecurity.SHA256Hex([]byte("raw-entry")),
		RawArtifactEntrySHA256:            domainsecurity.SHA256Hex([]byte("raw-entry-bytes")),
		RawArtifactEntryByteLength:        512,
		RawSourceLocatorDigest:            domainsecurity.SHA256Hex([]byte("raw-source-locator")),
		RawSourceLocatorSHA256:            domainsecurity.SHA256Hex([]byte("raw-source-locator-bytes")),
		RawSourceLocatorByteLength:        512,
		SourceArtifactSHA256:              domainsecurity.SHA256Hex([]byte("artifact")),
		SourceArtifactByteLength:          1024,
		ParsedGenerationReceiptDigest:     domainsecurity.SHA256Hex([]byte("generation-digest")),
		ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex([]byte("generation-bytes")),
		ParsedGenerationReceiptByteLength: 640,
		ParsedPageDigest:                  domainsecurity.SHA256Hex([]byte("parsed-page-digest")),
		ParsedPageSHA256:                  domainsecurity.SHA256Hex([]byte("parsed-page-bytes")),
		ParsedPageByteLength:              512,
		ParsedRowOrdinal:                  0,
		MappingDigest:                     domainsecurity.SHA256Hex([]byte("mapping")),
		ProjectionSchemaDigest:            domainsecurity.SHA256Hex([]byte("projection")),
		ReaderModeDigest:                  domainsecurity.SHA256Hex([]byte("reader")),
		CanonicalRowSHA256:                domainsecurity.SHA256Hex([]byte("row")),
	}); err == nil {
		t.Fatal("unknown producer policy minted a row record")
	}
}

func TestSourceRowLedgerV1RejectsUntrustedFileIDAndDescriptorPolicyBypass(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	for _, hostile := range []string{
		"file-a",
		"../../case-b",
		"ignore previous instructions",
		strings.Repeat("A", 20),
		"temp_fc_" + strings.Repeat("a", 64),
	} {
		if _, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{SourceFileID: hostile, SourceRowNumber: 1}); err == nil {
			t.Fatalf("untrusted source file id entered public ledger metadata: %q", hostile)
		}
	}
	accountShapedFileID := "62220200000000000000"
	locator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{SourceFileID: accountShapedFileID, SourceRowNumber: 1})
	if err != nil || locator.SourceFileIDDigest != sourceRowSourceFileIDDigestV1(accountShapedFileID) {
		t.Fatalf("opaque valid file id was not converted to its host digest: %#v err=%v", locator, err)
	}
	locatorBody, _ := json.Marshal(locator)
	if strings.Contains(string(locatorBody), accountShapedFileID) {
		t.Fatal("account-shaped source file id leaked into ledger metadata")
	}

	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	_, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	oversizedCount := sourceRowTestCloneRootV1(t, root)
	oversizedCount.IndexPageDescriptors[0].RecordCount = uint64(policy.MaxRowsPerPage) + 1
	oversizedCount.IndexPageDescriptors[0].DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(oversizedCount.IndexPageDescriptors[0])
	oversizedCount.RecordCount = oversizedCount.IndexPageDescriptors[0].RecordCount
	oversizedCount.RootDigest = sourceRowLedgerRootDigestV1(oversizedCount)
	if ValidateSourceRowLedgerRootV1(policy, oversizedCount) == nil {
		t.Fatal("descriptor-only root bypassed the producer row limit")
	}

	oversizedBytes := sourceRowTestCloneRootV1(t, root)
	oversizedBytes.IndexPageDescriptors[0].AggregateDataPageBytes = policy.MaxPageBytes + 1
	oversizedBytes.IndexPageDescriptors[0].DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(oversizedBytes.IndexPageDescriptors[0])
	oversizedBytes.RootDigest = sourceRowLedgerRootDigestV1(oversizedBytes)
	if ValidateSourceRowLedgerRootV1(policy, oversizedBytes) == nil {
		t.Fatal("descriptor-only root bypassed the producer byte limit")
	}

	overflowingCount := root.IndexPageDescriptors[0]
	overflowingCount.RecordCount = ^uint64(0)
	overflowingCount.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(overflowingCount)
	if _, err := NewSourceRowLedgerRootFromIndexDescriptorsV1(
		policy, binding, []SourceRowLedgerIndexPageDescriptorV1{overflowingCount},
	); err == nil {
		t.Fatal("root constructor accepted a descriptor count that underflows its aggregate guard")
	}
}

func TestSourceRowLedgerRootV1WriterAndReaderShareCanonicalByteLimit(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-root-byte-limit")
	_, base := sourceRowTestFullIndexPageV1(t, policy, binding)
	descriptors := make([]SourceRowLedgerIndexPageDescriptorV1, maxSourceRowLedgerRootIndexPageCountV1)
	for index := range descriptors {
		descriptor := base
		descriptor.IndexPageNumber = uint64(index + 1)
		descriptor.IndexPageDigest = domainsecurity.SHA256Hex([]byte("index-page-digest:" + strconv.Itoa(index)))
		descriptor.IndexPageSHA256 = domainsecurity.SHA256Hex([]byte("index-page-bytes:" + strconv.Itoa(index)))
		descriptor.FirstDataPageNumber = uint64(index*maxSourceRowLedgerDataPagesPerIndexPageV1 + 1)
		descriptor.LastDataPageNumber = descriptor.FirstDataPageNumber + maxSourceRowLedgerDataPagesPerIndexPageV1 - 1
		descriptor.DataPageCount = maxSourceRowLedgerDataPagesPerIndexPageV1
		descriptor.RecordCount = uint64(descriptor.DataPageCount) * uint64(policy.MaxRowsPerPage)
		descriptor.AggregateDataPageBytes = uint64(descriptor.DataPageCount) * policy.MaxPageBytes
		descriptor.FirstSourceRowNumber = uint64(index)*descriptor.RecordCount + 1
		descriptor.LastSourceRowNumber = uint64(index+1) * descriptor.RecordCount
		first := sourceRowLocatorForPolicyBoundaryV1(policy, descriptor.FirstSourceFileIDDigest, descriptor.FirstSourceRowNumber)
		last := sourceRowLocatorForPolicyBoundaryV1(policy, descriptor.LastSourceFileIDDigest, descriptor.LastSourceRowNumber)
		descriptor.FirstLocatorDigest = first.LocatorDigest
		descriptor.LastLocatorDigest = last.LocatorDigest
		descriptor.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(descriptor)
		descriptors[index] = descriptor
	}
	maxRoot, err := NewSourceRowLedgerRootFromIndexDescriptorsV1(policy, binding, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	body, err := SourceRowLedgerRootV1Bytes(maxRoot)
	if err != nil || len(body) > maxSourceRowLedgerRootBytesV1 || maxRoot.RecordCount < 2_645_472 {
		t.Fatalf("bounded Merkle root cannot represent funds-scale data: records=%d bytes=%d err=%v", maxRoot.RecordCount, len(body), err)
	}
	parsed, err := ParseSourceRowLedgerRootV1(body)
	if err != nil || parsed.RootDigest != maxRoot.RootDigest {
		t.Fatalf("writer emitted a root the shared parser could not read: %v", err)
	}
}

func TestSourceRowLedgerFullIndexPageV1WriterAndReaderShareCanonicalByteLimit(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-full-index-byte-limit")
	page, descriptor := sourceRowTestFullIndexPageV1(t, policy, binding)
	body, err := SourceRowLedgerIndexPageV1Bytes(page)
	if err != nil || len(body) > maxSourceRowLedgerIndexPageBytesV1 || page.PageCount != maxSourceRowLedgerDataPagesPerIndexPageV1 ||
		page.RecordCount != uint64(page.PageCount)*uint64(policy.MaxRowsPerPage) {
		t.Fatalf("full canonical index page exceeds its bounded encoding: bytes=%d page=%#v err=%v", len(body), page, err)
	}
	parsed, err := ParseSourceRowLedgerIndexPageV1(body)
	if err != nil || parsed.IndexPageDigest != page.IndexPageDigest || descriptor.IndexPageByteLength != uint64(len(body)) {
		t.Fatalf("full canonical index page did not round-trip exactly: %v", err)
	}
}

func TestSourceRowLedgerRootV1WriterAndReaderShareBindingStringLimit(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	workspace := "/" + strings.Repeat("w", 5*1024)
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID:                 domainsecurity.LocalTenantID,
		UserID:                   domainsecurity.LocalUserID,
		WorkspaceRealPath:        workspace,
		CaseID:                   "case-long-workspace",
		CaseBindingHash:          domainsecurity.SHA256Hex([]byte("binding:case-long-workspace")),
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("observation:case-long-workspace")),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	_, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	body, err := SourceRowLedgerRootV1Bytes(root)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSourceRowLedgerRootV1(body)
	if err != nil || parsed.Binding.WorkspaceRealPath != workspace {
		t.Fatalf("writer emitted a binding string rejected by the shared reader: %v", err)
	}
}

func TestSourceRowLedgerIndexV1RejectsReusedContentAddresses(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-reused-cas")
	records := make([]SourceRowRecordV1, 0, int(policy.MaxRowsPerPage)+1)
	for rowNumber := uint64(1); rowNumber <= uint64(policy.MaxRowsPerPage)+1; rowNumber++ {
		seed := strconv.FormatUint(rowNumber, 10)
		records = append(records, sourceRowTestRecordV1(
			t, policy, binding, "file-a", rowNumber, "artifact-a", "row-"+seed, "lineage-a",
		))
	}
	firstPage, err := NewSourceRowLedgerPageV1(policy, binding, 1, records[:policy.MaxRowsPerPage])
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := NewSourceRowLedgerPageV1(policy, binding, 2, records[policy.MaxRowsPerPage:])
	if err != nil {
		t.Fatal(err)
	}
	firstDescriptor, err := NewSourceRowLedgerPageDescriptorV1(policy, binding, firstPage)
	if err != nil {
		t.Fatal(err)
	}
	secondDescriptor, err := NewSourceRowLedgerPageDescriptorV1(policy, binding, secondPage)
	if err != nil {
		t.Fatal(err)
	}
	secondDescriptor.PageSHA256 = firstDescriptor.PageSHA256
	secondDescriptor.DescriptorDigest = sourceRowLedgerPageDescriptorDigestV1(secondDescriptor)
	_, err = NewSourceRowLedgerIndexPageV1(policy, binding, 1, []SourceRowLedgerPageDescriptorV1{firstDescriptor, secondDescriptor})
	requireSourceRowErrorContainsV1(t, err, "reuses a data page content address")

	baseIndex, err := NewSourceRowLedgerIndexPageV1(policy, binding, 1, []SourceRowLedgerPageDescriptorV1{firstDescriptor})
	if err != nil {
		t.Fatal(err)
	}
	baseRootDescriptor, err := NewSourceRowLedgerIndexPageDescriptorV1(policy, binding, baseIndex)
	if err != nil {
		t.Fatal(err)
	}
	firstRootDescriptor := baseRootDescriptor
	firstRootDescriptor.DataPageCount = maxSourceRowLedgerDataPagesPerIndexPageV1
	firstRootDescriptor.LastDataPageNumber = firstRootDescriptor.FirstDataPageNumber + uint64(firstRootDescriptor.DataPageCount) - 1
	firstRootDescriptor.RecordCount = uint64(firstRootDescriptor.DataPageCount) * uint64(policy.MaxRowsPerPage)
	firstRootDescriptor.AggregateDataPageBytes = uint64(firstRootDescriptor.DataPageCount) * firstDescriptor.PageByteLength
	firstRootDescriptor.LastSourceRowNumber = firstRootDescriptor.RecordCount
	firstRootDescriptor.LastLocatorDigest = sourceRowLocatorForPolicyBoundaryV1(
		policy, firstRootDescriptor.LastSourceFileIDDigest, firstRootDescriptor.LastSourceRowNumber,
	).LocatorDigest
	firstRootDescriptor.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(firstRootDescriptor)
	secondRootDescriptor := firstRootDescriptor
	secondRootDescriptor.IndexPageNumber = 2
	secondRootDescriptor.IndexPageDigest = domainsecurity.SHA256Hex([]byte("second-index-page-digest"))
	secondRootDescriptor.FirstDataPageNumber = firstRootDescriptor.LastDataPageNumber + 1
	secondRootDescriptor.LastDataPageNumber = secondRootDescriptor.FirstDataPageNumber
	secondRootDescriptor.DataPageCount = 1
	secondRootDescriptor.RecordCount = 1
	secondRootDescriptor.AggregateDataPageBytes = firstDescriptor.PageByteLength
	secondRootDescriptor.FirstSourceRowNumber = firstRootDescriptor.LastSourceRowNumber + 1
	secondRootDescriptor.LastSourceRowNumber = secondRootDescriptor.FirstSourceRowNumber
	secondRootDescriptor.FirstLocatorDigest = sourceRowLocatorForPolicyBoundaryV1(
		policy, secondRootDescriptor.FirstSourceFileIDDigest, secondRootDescriptor.FirstSourceRowNumber,
	).LocatorDigest
	secondRootDescriptor.LastLocatorDigest = secondRootDescriptor.FirstLocatorDigest
	secondRootDescriptor.IndexPageSHA256 = firstRootDescriptor.IndexPageSHA256
	secondRootDescriptor.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(secondRootDescriptor)
	_, err = NewSourceRowLedgerRootFromIndexDescriptorsV1(
		policy, binding, []SourceRowLedgerIndexPageDescriptorV1{firstRootDescriptor, secondRootDescriptor},
	)
	requireSourceRowErrorContainsV1(t, err, "reuses an index page content address")
}

func TestSourceRowLedgerRootV1RejectsNonCanonicalIndexPartition(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-index-partition")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	indexPage := sourceRowTestIndexPageV1(t, policy, binding, root, page)
	first, err := NewSourceRowLedgerIndexPageDescriptorV1(policy, binding, indexPage)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.IndexPageNumber = 2
	second.IndexPageDigest = domainsecurity.SHA256Hex([]byte("second-index-page"))
	second.IndexPageSHA256 = domainsecurity.SHA256Hex([]byte("second-index-page-bytes"))
	second.FirstDataPageNumber = 2
	second.LastDataPageNumber = 2
	second.FirstSourceRowNumber = 2
	second.LastSourceRowNumber = 2
	second.FirstLocatorDigest = sourceRowLocatorForPolicyBoundaryV1(policy, second.FirstSourceFileIDDigest, 2).LocatorDigest
	second.LastLocatorDigest = second.FirstLocatorDigest
	second.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(second)
	_, err = NewSourceRowLedgerRootFromIndexDescriptorsV1(
		policy, binding, []SourceRowLedgerIndexPageDescriptorV1{first, second},
	)
	requireSourceRowErrorContainsV1(t, err, "partial non-terminal index page")
}

func TestSourceRowLedgerRootV1RejectsLocatorRollbackAcrossIndexPages(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-index-rollback")
	_, first := sourceRowTestFullIndexPageV1(t, policy, binding)
	second := first
	second.IndexPageNumber = 2
	second.IndexPageDigest = domainsecurity.SHA256Hex([]byte("rollback-index-digest"))
	second.IndexPageSHA256 = domainsecurity.SHA256Hex([]byte("rollback-index-bytes"))
	second.FirstDataPageNumber = first.LastDataPageNumber + 1
	second.LastDataPageNumber = second.FirstDataPageNumber
	second.DataPageCount = 1
	second.RecordCount = 1
	second.AggregateDataPageBytes = policy.MaxPageBytes
	second.FirstSourceRowNumber = first.LastSourceRowNumber
	second.LastSourceRowNumber = second.FirstSourceRowNumber
	second.FirstLocatorDigest = sourceRowLocatorForPolicyBoundaryV1(
		policy, second.FirstSourceFileIDDigest, second.FirstSourceRowNumber,
	).LocatorDigest
	second.LastLocatorDigest = second.FirstLocatorDigest
	second.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(second)
	_, err := NewSourceRowLedgerRootFromIndexDescriptorsV1(
		policy, binding, []SourceRowLedgerIndexPageDescriptorV1{first, second},
	)
	requireSourceRowErrorContainsV1(t, err, "index boundaries are not strictly ordered")
}

func TestSourceRowWitnessLedgerMaterialV1BindsRootIndexPageAndRecord(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-witness-material")
	records := make([]SourceRowRecordV1, 0, int(policy.MaxRowsPerPage)+1)
	for rowNumber := uint64(1); rowNumber <= uint64(policy.MaxRowsPerPage)+1; rowNumber++ {
		seed := strconv.FormatUint(rowNumber, 10)
		records = append(records, sourceRowTestRecordV1(
			t, policy, binding, "file-a", rowNumber, "artifact-a", "row-"+seed, "lineage-a",
		))
	}
	firstPage, err := NewSourceRowLedgerPageV1(policy, binding, 1, records[:policy.MaxRowsPerPage])
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := NewSourceRowLedgerPageV1(policy, binding, 2, records[policy.MaxRowsPerPage:])
	if err != nil {
		t.Fatal(err)
	}
	indexPages, root, err := newSourceRowLedgerHierarchyV1FromPages(
		policy, binding, []SourceRowLedgerPageV1{firstPage, secondPage},
	)
	if err != nil || len(indexPages) != 1 {
		t.Fatalf("test hierarchy is invalid: %v", err)
	}
	indexPage := indexPages[0]
	target := records[len(records)-1]
	if err := ValidateSourceRowWitnessLedgerMaterialV1(policy, root, indexPage, secondPage, target); err != nil {
		t.Fatalf("exact root/index/page/record material was rejected: %v", err)
	}
	forgedRoot := sourceRowTestCloneRootV1(t, root)
	forgedRoot.IndexPageDescriptors[0].IndexPageSHA256 = domainsecurity.SHA256Hex([]byte("forged-index-page-bytes"))
	forgedRoot.IndexPageDescriptors[0].DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(forgedRoot.IndexPageDescriptors[0])
	forgedRoot.RootDigest = sourceRowLedgerRootDigestV1(forgedRoot)
	err = ValidateSourceRowWitnessLedgerMaterialV1(policy, forgedRoot, indexPage, secondPage, target)
	requireSourceRowErrorContainsV1(t, err, "index page descriptor mismatch")
	tamperedTarget := sourceRowTestRecordV1(
		t, policy, binding, "file-a", uint64(policy.MaxRowsPerPage)+1, "artifact-a", "tampered-row", "lineage-a",
	)
	tamperedPage, err := NewSourceRowLedgerPageV1(policy, binding, 2, []SourceRowRecordV1{tamperedTarget})
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateSourceRowWitnessLedgerMaterialV1(policy, root, indexPage, tamperedPage, tamperedTarget)
	requireSourceRowErrorContainsV1(t, err, "page descriptor mismatch")
	err = ValidateSourceRowWitnessLedgerMaterialV1(policy, root, indexPage, firstPage, target)
	requireSourceRowErrorContainsV1(t, err, "record is absent")
	outside := sourceRowTestRecordV1(
		t, policy, binding, "file-a", uint64(policy.MaxRowsPerPage)+2, "artifact-a", "outside-row", "lineage-a",
	)
	err = ValidateSourceRowWitnessLedgerMaterialV1(policy, root, indexPage, secondPage, outside)
	requireSourceRowErrorContainsV1(t, err, "record is absent")
}

func TestSourceRowWitnessV1CannotBeConstructedFromLegacyOrBareDSV2Context(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	indexPage := sourceRowTestIndexPageV1(t, policy, binding, root, page)
	snapshotRecordDigest := domainsecurity.SHA256Hex([]byte("snapshot-authority-record"))
	legacyInput := sourceRowTestContextInputV1("thread-a", "turn-legacy", "case-a", 4, domainsecurity.DatasetSnapshotIDPrefixV1)
	legacyContext := domainsecurity.NewTurnSecurityContext(legacyInput)
	if err := domainsecurity.ValidateTurnSecurityContext(legacyContext); err != nil {
		t.Fatalf("legacy audit context is invalid: %v", err)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(legacyContext); err == nil {
		t.Fatal("legacy audit context unexpectedly became executable")
	}
	if err := ValidateSourceRowLedgerRootV1(policy, root); err != nil {
		t.Fatalf("valid structural root was rejected: %v", err)
	}
	if err := ValidateSourceRowLedgerRootForContextV1(root, legacyContext); err == nil {
		t.Fatal("legacy audit context unexpectedly authorized the row root")
	}
	tamperedRoot := sourceRowTestCloneRootV1(t, root)
	tamperedRoot.RootDigest = domainsecurity.SHA256Hex([]byte("tampered-root"))
	if ValidateSourceRowLedgerRootV1(policy, tamperedRoot) == nil {
		t.Fatal("structural validation accepted a tampered row root")
	}
	if _, err := NewSourceRowWitnessV1(SourceRowWitnessInputV1{
		Context: legacyContext, SnapshotAuthorityRecordDigest: snapshotRecordDigest,
		Root: root, IndexPage: indexPage, Page: page, Record: record,
	}); err == nil {
		t.Fatal("legacy DSV1 minted a source row witness")
	}
	dsv2Input := sourceRowTestContextInputV1("thread-a", "turn-dsv2", "case-a", 4, domainsecurity.DatasetSnapshotIDPrefixV2)
	dsv2Context, err := domainsecurity.NewTurnSecurityContextV2(dsv2Input)
	if err != nil {
		t.Fatalf("bare DSV2 structural context was rejected: %v", err)
	}
	if domainsecurity.DatasetSnapshotFactAuthorityBlocker(dsv2Input.DatasetSnapshotID) == "" {
		t.Fatal("bare DSV2 unexpectedly acquired fact authority")
	}
	if _, err := NewSourceRowWitnessV1(SourceRowWitnessInputV1{
		Context: dsv2Context, SnapshotAuthorityRecordDigest: snapshotRecordDigest,
		Root: root, IndexPage: indexPage, Page: page, Record: record,
	}); err == nil {
		t.Fatal("bare DSV2 context minted a source row witness without an active host producer policy")
	}
}

func TestSourceRowWitnessSetV1RejectsCrossReceiptRebindAndLocatorAlias(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, root := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	snapshotRecordDigest := domainsecurity.SHA256Hex([]byte("snapshot-authority-record"))
	first := sourceRowTestStructuralWitnessV1(t, root, page, record, snapshotRecordDigest, "turn-a")
	second := sourceRowTestStructuralWitnessV1(t, root, page, record, snapshotRecordDigest, "turn-b")
	if SourceRowWitnessCanAuthorizeFactsV1(first) {
		t.Fatal("structural source row material acquired factual authority")
	}
	forgedAuthority := first
	forgedAuthority.AuthorityClass = "registry_backed"
	forgedAuthority.FactAnswerAllowed = true
	forgedAuthority.WitnessDigest = sourceRowWitnessDigestV1(forgedAuthority)
	if ValidateSourceRowWitnessV1(forgedAuthority) == nil {
		t.Fatal("self-reported factual authority entered structural witness material")
	}
	if _, err := CanonicalSourceRowWitnessesV1([]SourceRowWitnessV1{second, first}); err != nil {
		t.Fatalf("idempotent same-row use across turns was rejected: %v", err)
	}
	rebound := second
	rebound.CanonicalRowSHA256 = domainsecurity.SHA256Hex([]byte("tampered-row"))
	rebound.WitnessDigest = sourceRowWitnessDigestV1(rebound)
	if ValidateSourceRowWitnessV1(rebound) != nil {
		t.Fatal("test did not construct a structurally valid hostile witness")
	}
	if _, err := CanonicalSourceRowWitnessesV1([]SourceRowWitnessV1{first, rebound}); err == nil {
		t.Fatal("same snapshot/record id was rebound to another row hash")
	}
	locatorAlias := second
	locatorAlias.SourceRecordID = SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("attacker-alias"))
	locatorAlias.WitnessDigest = sourceRowWitnessDigestV1(locatorAlias)
	if ValidateSourceRowWitnessV1(locatorAlias) != nil {
		t.Fatal("test did not construct a structurally valid locator alias")
	}
	if _, err := CanonicalSourceRowWitnessesV1([]SourceRowWitnessV1{first, locatorAlias}); err == nil {
		t.Fatal("one exact source locator mapped to two source record ids")
	}
	otherArtifactRecord := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-b", "row-a", "lineage-a")
	otherPage, otherRoot := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{otherArtifactRecord})
	otherArtifact := sourceRowTestStructuralWitnessV1(t, otherRoot, otherPage, otherArtifactRecord, snapshotRecordDigest, "turn-c")
	if _, err := CanonicalSourceRowWitnessesV1([]SourceRowWitnessV1{first, otherArtifact}); err == nil {
		t.Fatal("same snapshot locator was rebound to another source artifact")
	}
}

func TestSourceRowContractsRejectUnknownFieldsNonCanonicalEncodingAndTrimmedHashes(t *testing.T) {
	policy := sourceRowTestPolicyV1(t)
	binding := sourceRowTestBindingV1(t, "case-a")
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, _ := sourceRowTestLedgerV1(t, policy, binding, []SourceRowRecordV1{record})
	body, _ := SourceRowLedgerPageV1Bytes(page)
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	value["mcpAuthority"] = true
	withUnknown, _ := json.Marshal(value)
	if _, err := ParseSourceRowLedgerPageV1(withUnknown); err == nil {
		t.Fatal("unknown external field was accepted")
	}
	pretty, _ := json.MarshalIndent(page, "", "  ")
	if _, err := ParseSourceRowLedgerPageV1(pretty); err == nil {
		t.Fatal("noncanonical persisted page encoding was accepted")
	}
	tampered := record
	tampered.CanonicalRowSHA256 += " "
	tampered.RecordDigest = sourceRowRecordDigestV1(tampered)
	if ValidateSourceRowRecordV1(policy, tampered) == nil {
		t.Fatal("trim-tolerant SHA validation entered the row ledger contract")
	}
}

func sourceRowTestPolicyV1(t *testing.T) SourceRowProducerPolicyV1 {
	t.Helper()
	policy, ok := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1)
	if !ok || ValidateSourceRowProducerPolicyV1(policy) != nil {
		t.Fatal("source row test policy is unavailable")
	}
	return policy
}

func sourceRowTestBindingV1(t *testing.T, caseID string) domainsecurity.DatasetSnapshotBindingKeyV1 {
	t.Helper()
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID:                 domainsecurity.LocalTenantID,
		UserID:                   domainsecurity.LocalUserID,
		WorkspaceRealPath:        "/workspace",
		CaseID:                   caseID,
		CaseBindingHash:          domainsecurity.SHA256Hex([]byte("binding:" + caseID)),
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("source-row-binding-observation:" + caseID)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func sourceRowTestRecordV1(
	t *testing.T,
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	fileID string,
	rowNumber uint64,
	artifactSeed string,
	rowSeed string,
	lineageSeed string,
) SourceRowRecordV1 {
	t.Helper()
	hostFileID := sourceRowTestHostFileIDV1(fileID)
	record, _, err := newSourceRowRecordWithLineageV1(sourceRowRecordWithLineageInputV1{
		Policy: policy, Binding: binding,
		Locator:                           SourceRowLocatorInputV1{SourceFileID: hostFileID, SourceRowNumber: rowNumber},
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
		ParsedGenerationReceiptDigest:     domainsecurity.SHA256Hex([]byte("generation-digest:" + lineageSeed)),
		ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex([]byte("generation-bytes:" + lineageSeed)),
		ParsedGenerationReceiptByteLength: 640,
		ParsedPageDigest:                  domainsecurity.SHA256Hex([]byte("parsed-page-digest:" + lineageSeed + ":" + rowSeed)),
		ParsedPageSHA256:                  domainsecurity.SHA256Hex([]byte("parsed-page-bytes:" + lineageSeed + ":" + rowSeed)),
		ParsedPageByteLength:              512,
		ParsedRowOrdinal:                  uint32((rowNumber - 1) % uint64(policy.MaxRowsPerPage)),
		MappingDigest:                     domainsecurity.SHA256Hex([]byte("mapping:" + lineageSeed)),
		ProjectionSchemaDigest:            domainsecurity.SHA256Hex([]byte("projection:" + lineageSeed)),
		ReaderModeDigest:                  domainsecurity.SHA256Hex([]byte("reader:" + lineageSeed)),
		CanonicalRowSHA256:                domainsecurity.SHA256Hex([]byte(rowSeed)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func sourceRowTestHostFileIDV1(seed string) string {
	return domainsecurity.SHA256Hex([]byte("source-file-id:" + seed))[:20]
}

func sourceRowTestClonePageV1(t *testing.T, page SourceRowLedgerPageV1) SourceRowLedgerPageV1 {
	t.Helper()
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var clone SourceRowLedgerPageV1
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func sourceRowTestCloneRootV1(t *testing.T, root SourceRowLedgerRootV1) SourceRowLedgerRootV1 {
	t.Helper()
	body, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var clone SourceRowLedgerRootV1
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func sourceRowTestLedgerV1(
	t *testing.T,
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	records []SourceRowRecordV1,
) (SourceRowLedgerPageV1, SourceRowLedgerRootV1) {
	t.Helper()
	page, root, err := sourceRowTestLedgerV1Error(policy, binding, records)
	if err != nil {
		t.Fatal(err)
	}
	return page, root
}

func sourceRowTestIndexPageV1(
	t *testing.T,
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	root SourceRowLedgerRootV1,
	pages ...SourceRowLedgerPageV1,
) SourceRowLedgerIndexPageV1 {
	t.Helper()
	indexPages, built, err := newSourceRowLedgerHierarchyV1FromPages(policy, binding, pages)
	if err != nil || built.RootDigest != root.RootDigest || len(indexPages) != 1 {
		t.Fatalf("source row test index hierarchy is invalid: pages=%d err=%v", len(indexPages), err)
	}
	return indexPages[0]
}

func sourceRowTestFullIndexPageV1(
	t *testing.T,
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
) (SourceRowLedgerIndexPageV1, SourceRowLedgerIndexPageDescriptorV1) {
	t.Helper()
	record := sourceRowTestRecordV1(t, policy, binding, "file-a", 1, "artifact-a", "row-a", "lineage-a")
	page, err := NewSourceRowLedgerPageV1(policy, binding, 1, []SourceRowRecordV1{record})
	if err != nil {
		t.Fatal(err)
	}
	base, err := NewSourceRowLedgerPageDescriptorV1(policy, binding, page)
	if err != nil {
		t.Fatal(err)
	}
	descriptors := make([]SourceRowLedgerPageDescriptorV1, maxSourceRowLedgerDataPagesPerIndexPageV1)
	for index := range descriptors {
		descriptor := base
		descriptor.PageNumber = uint64(index + 1)
		descriptor.PageDigest = domainsecurity.SHA256Hex([]byte("full-data-page-digest:" + strconv.Itoa(index)))
		descriptor.PageSHA256 = domainsecurity.SHA256Hex([]byte("full-data-page-bytes:" + strconv.Itoa(index)))
		descriptor.PageByteLength = policy.MaxPageBytes
		descriptor.RecordCount = policy.MaxRowsPerPage
		descriptor.FirstSourceRowNumber = uint64(index)*uint64(policy.MaxRowsPerPage) + 1
		descriptor.LastSourceRowNumber = uint64(index+1) * uint64(policy.MaxRowsPerPage)
		descriptor.FirstLocatorDigest = sourceRowLocatorForPolicyBoundaryV1(
			policy, descriptor.FirstSourceFileIDDigest, descriptor.FirstSourceRowNumber,
		).LocatorDigest
		descriptor.LastLocatorDigest = sourceRowLocatorForPolicyBoundaryV1(
			policy, descriptor.LastSourceFileIDDigest, descriptor.LastSourceRowNumber,
		).LocatorDigest
		descriptor.FirstSourceRecordID = SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex(
			[]byte("full-data-page-first-record:"+strconv.Itoa(index)),
		)
		descriptor.LastSourceRecordID = SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex(
			[]byte("full-data-page-last-record:"+strconv.Itoa(index)),
		)
		descriptor.DescriptorDigest = sourceRowLedgerPageDescriptorDigestV1(descriptor)
		descriptors[index] = descriptor
	}
	indexPage, err := NewSourceRowLedgerIndexPageV1(policy, binding, 1, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	indexDescriptor, err := NewSourceRowLedgerIndexPageDescriptorV1(policy, binding, indexPage)
	if err != nil {
		t.Fatal(err)
	}
	return indexPage, indexDescriptor
}

func requireSourceRowErrorContainsV1(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("source row contract error = %v, want substring %q", err, want)
	}
}

func sourceRowTestLedgerV1Error(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	records []SourceRowRecordV1,
) (SourceRowLedgerPageV1, SourceRowLedgerRootV1, error) {
	page, err := NewSourceRowLedgerPageV1(policy, binding, 1, records)
	if err != nil {
		return SourceRowLedgerPageV1{}, SourceRowLedgerRootV1{}, err
	}
	root, err := newSourceRowLedgerRootV1FromPages(policy, binding, []SourceRowLedgerPageV1{page})
	return page, root, err
}

func sourceRowTestContextInputV1(threadID, turnID, caseID string, epoch uint64, snapshotPrefix string) domainsecurity.TurnSecurityContextInput {
	policyDigest := domainsecurity.SHA256Hex([]byte("source-row-test-policy:" + threadID))
	bindingObservationDigest := domainsecurity.SHA256Hex([]byte("source-row-binding-observation:" + caseID))
	publicationPolicy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   policyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: bindingObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		panic(err)
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding(threadID, "/workspace", domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		panic(err)
	}
	return domainsecurity.TurnSecurityContextInput{
		ThreadID:             threadID,
		TurnID:               turnID,
		WorkspaceRealPath:    "/workspace",
		TenantID:             domainsecurity.LocalTenantID,
		UserID:               domainsecurity.LocalUserID,
		CaseID:               caseID,
		CaseBindingHash:      domainsecurity.SHA256Hex([]byte("binding:" + caseID)),
		DatasetSnapshotID:    snapshotPrefix + domainsecurity.SHA256Hex([]byte("snapshot:"+caseID)),
		SourceManifestHash:   domainsecurity.SHA256Hex([]byte("manifest:" + caseID)),
		ContextEpoch:         epoch,
		IssuedAt:             time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC),
		PublicationPolicy:    publicationPolicy,
		RiskAuthorityBinding: riskBinding,
	}
}

func sourceRowTestStructuralWitnessV1(
	t *testing.T,
	root SourceRowLedgerRootV1,
	page SourceRowLedgerPageV1,
	record SourceRowRecordV1,
	snapshotRecordDigest string,
	turnID string,
) SourceRowWitnessV1 {
	t.Helper()
	witness := SourceRowWitnessV1{
		SchemaVersion:                 SourceRowWitnessSchemaVersionV1,
		Purpose:                       SourceRowWitnessPurposeV1,
		AuthorityClass:                SourceRowWitnessAuthorityNoneV1,
		FactAnswerAllowed:             false,
		ThreadID:                      "thread-a",
		TurnID:                        turnID,
		CaseID:                        root.Binding.CaseID,
		CaseBindingHash:               root.Binding.CaseBindingHash,
		ContextEpoch:                  4,
		ContextDigest:                 domainsecurity.SHA256Hex([]byte("structural-context:" + turnID)),
		DatasetSnapshotID:             domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot:"+root.Binding.CaseID)),
		SourceManifestHash:            domainsecurity.SHA256Hex([]byte("manifest:" + root.Binding.CaseID)),
		SnapshotAuthorityRecordDigest: snapshotRecordDigest,
		BindingKeyDigest:              root.Binding.BindingKeyDigest,
		PolicyID:                      record.PolicyID,
		PolicyDigest:                  record.PolicyDigest,
		LedgerRootDigest:              root.RootDigest,
		LedgerIndexPageDigest:         root.IndexPageDescriptors[0].IndexPageDigest,
		LedgerPageDigest:              page.PageDigest,
		SourceRecordID:                record.SourceRecordID,
		SourceArtifactSHA256:          record.SourceArtifactSHA256,
		CanonicalRowSHA256:            record.CanonicalRowSHA256,
		LocatorDigest:                 record.Locator.LocatorDigest,
		ParserID:                      record.ParserID,
		ParserVersion:                 record.ParserVersion,
		LineageDigest:                 record.LineageDigest,
		LineageSHA256:                 record.LineageSHA256,
		LineageByteLength:             record.LineageByteLength,
	}
	witness.WitnessDigest = sourceRowWitnessDigestV1(witness)
	if err := ValidateSourceRowWitnessV1(witness); err != nil {
		t.Fatal(err)
	}
	return witness
}
