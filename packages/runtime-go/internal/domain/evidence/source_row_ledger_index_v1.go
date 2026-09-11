package evidence

import (
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SourceRowLedgerIndexPageSchemaVersionV1 = 1
	SourceRowLedgerIndexPagePurposeV1       = "analytix.source-row-ledger-index-page/v1"

	SourceRowLedgerIndexPageDescriptorSchemaVersionV1 = 1
	SourceRowLedgerIndexPageDescriptorPurposeV1       = "analytix.source-row-ledger-index-page-descriptor/v1"

	maxSourceRowLedgerDataPagesPerIndexPageV1 = 256
	maxSourceRowLedgerIndexPageBytesV1        = 512 * 1024
	maxSourceRowLedgerRootIndexPageCountV1    = 4096
)

var (
	sourceRowLedgerIndexPageDigestDomainV1       = []byte("analytix.source-row-ledger-index-page/digest/v1\x00")
	sourceRowLedgerIndexDescriptorDigestDomainV1 = []byte("analytix.source-row-ledger-index-page-descriptor/digest/v1\x00")
)

// SourceRowLedgerIndexPageV1 is a bounded Merkle-index leaf over data-page
// descriptors. A streaming admission retains at most one such page while it
// writes and exact-reads data pages from private CAS.
type SourceRowLedgerIndexPageV1 struct {
	SchemaVersion    int                               `json:"schemaVersion"`
	Purpose          string                            `json:"purpose"`
	PolicyID         string                            `json:"policyId"`
	PolicyDigest     string                            `json:"policyDigest"`
	BindingKeyDigest string                            `json:"bindingKeyDigest"`
	ParserID         string                            `json:"parserId"`
	ParserVersion    string                            `json:"parserVersion"`
	IndexPageNumber  uint64                            `json:"indexPageNumber"`
	PageDescriptors  []SourceRowLedgerPageDescriptorV1 `json:"pageDescriptors"`
	PageCount        uint32                            `json:"pageCount"`
	RecordCount      uint64                            `json:"recordCount"`
	IndexPageDigest  string                            `json:"indexPageDigest"`
}

type SourceRowLedgerIndexPageDescriptorV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	IndexPageNumber         uint64 `json:"indexPageNumber"`
	IndexPageDigest         string `json:"indexPageDigest"`
	IndexPageSHA256         string `json:"indexPageSha256"`
	IndexPageByteLength     uint64 `json:"indexPageByteLength"`
	FirstDataPageNumber     uint64 `json:"firstDataPageNumber"`
	LastDataPageNumber      uint64 `json:"lastDataPageNumber"`
	DataPageCount           uint32 `json:"dataPageCount"`
	RecordCount             uint64 `json:"recordCount"`
	AggregateDataPageBytes  uint64 `json:"aggregateDataPageBytes"`
	FirstSourceFileIDDigest string `json:"firstSourceFileIdDigest"`
	FirstSourceRowNumber    uint64 `json:"firstSourceRowNumber"`
	FirstLocatorDigest      string `json:"firstLocatorDigest"`
	LastSourceFileIDDigest  string `json:"lastSourceFileIdDigest"`
	LastSourceRowNumber     uint64 `json:"lastSourceRowNumber"`
	LastLocatorDigest       string `json:"lastLocatorDigest"`
	DescriptorDigest        string `json:"descriptorDigest"`
}

func NewSourceRowLedgerIndexPageV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	indexPageNumber uint64,
	descriptors []SourceRowLedgerPageDescriptorV1,
) (SourceRowLedgerIndexPageV1, error) {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil ||
		indexPageNumber == 0 || indexPageNumber > maxSourceRowLedgerRootIndexPageCountV1 ||
		len(descriptors) == 0 || len(descriptors) > maxSourceRowLedgerDataPagesPerIndexPageV1 {
		return SourceRowLedgerIndexPageV1{}, errors.New("source row ledger index page input is invalid")
	}
	page := SourceRowLedgerIndexPageV1{
		SchemaVersion: SourceRowLedgerIndexPageSchemaVersionV1, Purpose: SourceRowLedgerIndexPagePurposeV1,
		PolicyID: policy.PolicyID, PolicyDigest: policy.PolicyDigest, BindingKeyDigest: binding.BindingKeyDigest,
		ParserID: policy.ParserID, ParserVersion: policy.ParserVersion, IndexPageNumber: indexPageNumber,
		PageDescriptors: append([]SourceRowLedgerPageDescriptorV1(nil), descriptors...), PageCount: uint32(len(descriptors)),
	}
	for _, descriptor := range page.PageDescriptors {
		if page.RecordCount > maxSourceRowJSONIntegerV1-uint64(descriptor.RecordCount) {
			return SourceRowLedgerIndexPageV1{}, errors.New("source row ledger index page record count exceeds the canonical range")
		}
		page.RecordCount += uint64(descriptor.RecordCount)
	}
	page.IndexPageDigest = sourceRowLedgerIndexPageDigestV1(page)
	if err := ValidateSourceRowLedgerIndexPageForBindingV1(policy, binding, page); err != nil {
		return SourceRowLedgerIndexPageV1{}, err
	}
	return page, nil
}

func ValidateSourceRowLedgerIndexPageV1(policy SourceRowProducerPolicyV1, page SourceRowLedgerIndexPageV1) error {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || page.SchemaVersion != SourceRowLedgerIndexPageSchemaVersionV1 ||
		page.Purpose != SourceRowLedgerIndexPagePurposeV1 || page.PolicyID != policy.PolicyID || page.PolicyDigest != policy.PolicyDigest ||
		!validSourceRowSHA256V1(page.BindingKeyDigest) || page.ParserID != policy.ParserID || page.ParserVersion != policy.ParserVersion ||
		page.IndexPageNumber == 0 || page.IndexPageNumber > maxSourceRowLedgerRootIndexPageCountV1 || len(page.PageDescriptors) == 0 ||
		len(page.PageDescriptors) > maxSourceRowLedgerDataPagesPerIndexPageV1 || page.PageCount != uint32(len(page.PageDescriptors)) ||
		page.RecordCount == 0 || page.RecordCount > maxSourceRowJSONIntegerV1 || !validSourceRowSHA256V1(page.IndexPageDigest) {
		return errors.New("source row ledger index page is invalid")
	}
	var recordCount uint64
	seenPageDigests := make(map[string]struct{}, len(page.PageDescriptors))
	seenPageSHA256 := make(map[string]struct{}, len(page.PageDescriptors))
	seenDescriptorDigests := make(map[string]struct{}, len(page.PageDescriptors))
	for index, descriptor := range page.PageDescriptors {
		if ValidateSourceRowLedgerPageDescriptorForPolicyV1(policy, descriptor) != nil ||
			index > 0 && descriptor.PageNumber != page.PageDescriptors[index-1].PageNumber+1 ||
			recordCount > maxSourceRowJSONIntegerV1-uint64(descriptor.RecordCount) {
			return errors.New("source row ledger index page descriptors are not canonical")
		}
		if index < len(page.PageDescriptors)-1 && descriptor.RecordCount != policy.MaxRowsPerPage {
			return errors.New("source row ledger index page contains a partial non-terminal data page")
		}
		if _, exists := seenPageDigests[descriptor.PageDigest]; exists {
			return errors.New("source row ledger index page reuses a data page digest")
		}
		if _, exists := seenPageSHA256[descriptor.PageSHA256]; exists {
			return errors.New("source row ledger index page reuses a data page content address")
		}
		if _, exists := seenDescriptorDigests[descriptor.DescriptorDigest]; exists {
			return errors.New("source row ledger index page reuses a data page descriptor")
		}
		if index > 0 && !sourceRowPageDescriptorStrictlyBeforeV1(page.PageDescriptors[index-1], descriptor) {
			return errors.New("source row ledger index page locator boundaries are not strictly ordered")
		}
		seenPageDigests[descriptor.PageDigest] = struct{}{}
		seenPageSHA256[descriptor.PageSHA256] = struct{}{}
		seenDescriptorDigests[descriptor.DescriptorDigest] = struct{}{}
		recordCount += uint64(descriptor.RecordCount)
	}
	body, err := json.Marshal(page)
	if err != nil || len(body) > maxSourceRowLedgerIndexPageBytesV1 || recordCount != page.RecordCount ||
		page.IndexPageDigest != sourceRowLedgerIndexPageDigestV1(page) {
		return errors.New("source row ledger index page content is invalid")
	}
	return nil
}

func ValidateSourceRowLedgerIndexPageForBindingV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	page SourceRowLedgerIndexPageV1,
) error {
	if err := ValidateSourceRowLedgerIndexPageV1(policy, page); err != nil {
		return err
	}
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil || page.BindingKeyDigest != binding.BindingKeyDigest {
		return errors.New("source row ledger index page binding mismatch")
	}
	return nil
}

func NewSourceRowLedgerIndexPageDescriptorV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	page SourceRowLedgerIndexPageV1,
) (SourceRowLedgerIndexPageDescriptorV1, error) {
	if ValidateSourceRowLedgerIndexPageForBindingV1(policy, binding, page) != nil {
		return SourceRowLedgerIndexPageDescriptorV1{}, errors.New("source row ledger index descriptor input is invalid")
	}
	body, _ := json.Marshal(page)
	first := page.PageDescriptors[0]
	last := page.PageDescriptors[len(page.PageDescriptors)-1]
	descriptor := SourceRowLedgerIndexPageDescriptorV1{
		SchemaVersion: SourceRowLedgerIndexPageDescriptorSchemaVersionV1, Purpose: SourceRowLedgerIndexPageDescriptorPurposeV1,
		IndexPageNumber: page.IndexPageNumber, IndexPageDigest: page.IndexPageDigest,
		IndexPageSHA256: domainsecurity.SHA256Hex(body), IndexPageByteLength: uint64(len(body)),
		FirstDataPageNumber: first.PageNumber, LastDataPageNumber: last.PageNumber, DataPageCount: page.PageCount,
		RecordCount: page.RecordCount, FirstSourceFileIDDigest: first.FirstSourceFileIDDigest,
		FirstSourceRowNumber: first.FirstSourceRowNumber, FirstLocatorDigest: first.FirstLocatorDigest,
		LastSourceFileIDDigest: last.LastSourceFileIDDigest, LastSourceRowNumber: last.LastSourceRowNumber,
		LastLocatorDigest: last.LastLocatorDigest,
	}
	for _, dataPage := range page.PageDescriptors {
		if descriptor.AggregateDataPageBytes > maxSourceRowLedgerAggregatePageBytesV1-dataPage.PageByteLength {
			return SourceRowLedgerIndexPageDescriptorV1{}, errors.New("source row ledger index descriptor data bytes exceed the aggregate limit")
		}
		descriptor.AggregateDataPageBytes += dataPage.PageByteLength
	}
	descriptor.DescriptorDigest = sourceRowLedgerIndexDescriptorDigestV1(descriptor)
	if err := ValidateSourceRowLedgerIndexPageDescriptorForPolicyV1(policy, descriptor); err != nil {
		return SourceRowLedgerIndexPageDescriptorV1{}, err
	}
	return descriptor, nil
}

func ValidateSourceRowLedgerIndexPageDescriptorV1(descriptor SourceRowLedgerIndexPageDescriptorV1) error {
	if descriptor.SchemaVersion != SourceRowLedgerIndexPageDescriptorSchemaVersionV1 ||
		descriptor.Purpose != SourceRowLedgerIndexPageDescriptorPurposeV1 || descriptor.IndexPageNumber == 0 ||
		descriptor.IndexPageNumber > maxSourceRowLedgerRootIndexPageCountV1 || !validSourceRowSHA256V1(descriptor.IndexPageDigest) ||
		!validSourceRowSHA256V1(descriptor.IndexPageSHA256) || descriptor.IndexPageByteLength == 0 ||
		descriptor.IndexPageByteLength > maxSourceRowLedgerIndexPageBytesV1 || descriptor.FirstDataPageNumber == 0 ||
		descriptor.LastDataPageNumber < descriptor.FirstDataPageNumber || descriptor.DataPageCount == 0 ||
		descriptor.DataPageCount > maxSourceRowLedgerDataPagesPerIndexPageV1 ||
		descriptor.LastDataPageNumber-descriptor.FirstDataPageNumber+1 != uint64(descriptor.DataPageCount) ||
		descriptor.RecordCount == 0 || descriptor.RecordCount > maxSourceRowJSONIntegerV1 || descriptor.AggregateDataPageBytes == 0 ||
		descriptor.AggregateDataPageBytes > maxSourceRowLedgerAggregatePageBytesV1 ||
		!validSourceRowSHA256V1(descriptor.FirstSourceFileIDDigest) || descriptor.FirstSourceRowNumber == 0 ||
		descriptor.FirstSourceRowNumber > maxSourceRowJSONIntegerV1 || !validSourceRowSHA256V1(descriptor.FirstLocatorDigest) ||
		!validSourceRowSHA256V1(descriptor.LastSourceFileIDDigest) || descriptor.LastSourceRowNumber == 0 ||
		descriptor.LastSourceRowNumber > maxSourceRowJSONIntegerV1 || !validSourceRowSHA256V1(descriptor.LastLocatorDigest) ||
		!validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != sourceRowLedgerIndexDescriptorDigestV1(descriptor) {
		return errors.New("source row ledger index page descriptor is invalid")
	}
	first := sourceRowLocatorFromDescriptorBoundaryV1(descriptor.FirstSourceFileIDDigest, descriptor.FirstSourceRowNumber)
	last := sourceRowLocatorFromDescriptorBoundaryV1(descriptor.LastSourceFileIDDigest, descriptor.LastSourceRowNumber)
	if descriptor.RecordCount == 1 {
		if first.SourceFileIDDigest != last.SourceFileIDDigest || first.SourceRowNumber != last.SourceRowNumber ||
			descriptor.FirstLocatorDigest != descriptor.LastLocatorDigest {
			return errors.New("single-record source row ledger index descriptor has inconsistent boundaries")
		}
	} else if !sourceRowLocatorStrictlyBeforeV1(first, last) {
		return errors.New("source row ledger index descriptor boundaries are not strictly ordered")
	}
	return nil
}

func ValidateSourceRowLedgerIndexPageDescriptorForPolicyV1(
	policy SourceRowProducerPolicyV1,
	descriptor SourceRowLedgerIndexPageDescriptorV1,
) error {
	first := sourceRowLocatorForPolicyBoundaryV1(policy, descriptor.FirstSourceFileIDDigest, descriptor.FirstSourceRowNumber)
	last := sourceRowLocatorForPolicyBoundaryV1(policy, descriptor.LastSourceFileIDDigest, descriptor.LastSourceRowNumber)
	if ValidateSourceRowProducerPolicyV1(policy) != nil || ValidateSourceRowLedgerIndexPageDescriptorV1(descriptor) != nil ||
		descriptor.RecordCount > uint64(descriptor.DataPageCount)*uint64(policy.MaxRowsPerPage) ||
		descriptor.AggregateDataPageBytes > uint64(descriptor.DataPageCount)*policy.MaxPageBytes ||
		first.LocatorDigest != descriptor.FirstLocatorDigest || last.LocatorDigest != descriptor.LastLocatorDigest {
		return errors.New("source row ledger index page descriptor exceeds its registered policy")
	}
	return nil
}

func SourceRowLedgerIndexPageV1Bytes(page SourceRowLedgerIndexPageV1) ([]byte, error) {
	policy, ok := ResolveSourceRowProducerPolicyV1(page.PolicyID)
	if !ok || ValidateSourceRowLedgerIndexPageV1(policy, page) != nil {
		return nil, errors.New("source row ledger index page is invalid")
	}
	body, err := json.Marshal(page)
	if err != nil || len(body) > maxSourceRowLedgerIndexPageBytesV1 {
		return nil, errors.New("source row ledger index page exceeds its canonical byte limit")
	}
	return body, nil
}

func ParseSourceRowLedgerIndexPageV1(raw []byte) (SourceRowLedgerIndexPageV1, error) {
	var page SourceRowLedgerIndexPageV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &page, maxSourceRowLedgerIndexPageBytesV1, 250_000, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return SourceRowLedgerIndexPageV1{}, err
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(page.PolicyID)
	if !ok {
		return SourceRowLedgerIndexPageV1{}, errors.New("source row ledger index page policy is unknown")
	}
	return page, ValidateSourceRowLedgerIndexPageV1(policy, page)
}

func sourceRowLedgerIndexPageDigestV1(page SourceRowLedgerIndexPageV1) string {
	page.IndexPageDigest = ""
	body, _ := json.Marshal(page)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLedgerIndexPageDigestDomainV1...), body...))
}

func sourceRowLedgerIndexDescriptorDigestV1(descriptor SourceRowLedgerIndexPageDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLedgerIndexDescriptorDigestDomainV1...), body...))
}

func sourceRowPageDescriptorStrictlyBeforeV1(left, right SourceRowLedgerPageDescriptorV1) bool {
	leftLast := sourceRowLocatorFromDescriptorBoundaryV1(left.LastSourceFileIDDigest, left.LastSourceRowNumber)
	rightFirst := sourceRowLocatorFromDescriptorBoundaryV1(right.FirstSourceFileIDDigest, right.FirstSourceRowNumber)
	return sourceRowLocatorStrictlyBeforeV1(leftLast, rightFirst)
}

func sourceRowIndexDescriptorStrictlyBeforeV1(left, right SourceRowLedgerIndexPageDescriptorV1) bool {
	leftLast := sourceRowLocatorFromDescriptorBoundaryV1(left.LastSourceFileIDDigest, left.LastSourceRowNumber)
	rightFirst := sourceRowLocatorFromDescriptorBoundaryV1(right.FirstSourceFileIDDigest, right.FirstSourceRowNumber)
	return sourceRowLocatorStrictlyBeforeV1(leftLast, rightFirst)
}

func validateSourceRowLedgerRootWithHierarchyV1(
	policy SourceRowProducerPolicyV1,
	root SourceRowLedgerRootV1,
	indexPages []SourceRowLedgerIndexPageV1,
	dataPages []SourceRowLedgerPageV1,
) error {
	if ValidateSourceRowLedgerRootV1(policy, root) != nil || len(indexPages) != len(root.IndexPageDescriptors) ||
		len(dataPages) != int(root.PageCount) {
		return errors.New("source row ledger hierarchy shape is invalid")
	}
	dataOffset := 0
	for index, indexPage := range indexPages {
		if ValidateSourceRowLedgerIndexPageForBindingV1(policy, root.Binding, indexPage) != nil {
			return errors.New("source row ledger hierarchy contains an invalid index page")
		}
		descriptor, err := NewSourceRowLedgerIndexPageDescriptorV1(policy, root.Binding, indexPage)
		if err != nil || descriptor != root.IndexPageDescriptors[index] {
			return errors.New("source row ledger root index descriptor mismatch")
		}
		for descriptorIndex, dataDescriptor := range indexPage.PageDescriptors {
			if dataOffset >= len(dataPages) {
				return errors.New("source row ledger hierarchy data page count is truncated")
			}
			page := dataPages[dataOffset]
			if ValidateSourceRowLedgerPageForBindingV1(policy, root.Binding, page) != nil {
				return errors.New("source row ledger hierarchy contains an invalid data page")
			}
			exact, exactErr := NewSourceRowLedgerPageDescriptorV1(policy, root.Binding, page)
			if exactErr != nil || exact != dataDescriptor || descriptorIndex >= int(indexPage.PageCount) {
				return errors.New("source row ledger index page data descriptor mismatch")
			}
			dataOffset++
		}
	}
	if dataOffset != len(dataPages) {
		return errors.New("source row ledger hierarchy contains unindexed data pages")
	}
	return nil
}
