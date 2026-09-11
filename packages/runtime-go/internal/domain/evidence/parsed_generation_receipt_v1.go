package evidence

import (
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ParsedPageDescriptorSchemaVersionV1 = 1
	ParsedPageDescriptorPurposeV1       = "analytix.parsed-page-descriptor/v1"
	ParsedPageIndexSchemaVersionV1      = 1
	ParsedPageIndexPurposeV1            = "analytix.parsed-page-index/v1"
	ParsedPageIndexDescriptorVersionV1  = 1
	ParsedPageIndexDescriptorPurposeV1  = "analytix.parsed-page-index-descriptor/v1"
	ParsedGenerationReceiptVersionV1    = 1
	ParsedGenerationReceiptPurposeV1    = "analytix.parsed-generation-receipt/v1"

	ParsedGenerationTimezoneUnresolvedV1 = "unresolved"
	ParsedGenerationCurrencyUnresolvedV1 = "unresolved"

	maxParsedPagesPerIndexV1       = 256
	maxParsedPageIndexesV1         = 512
	maxParsedPageIndexBytesV1      = 512 * 1024
	maxParsedGenerationReceiptV1   = 1024 * 1024
	maxParsedGenerationPageCountV1 = uint64(maxParsedPagesPerIndexV1 * maxParsedPageIndexesV1)
)

var (
	parsedPageDescriptorDigestDomainV1      = []byte("analytix.parsed-page-descriptor/digest/v1\x00")
	parsedPageIndexDigestDomainV1           = []byte("analytix.parsed-page-index/digest/v1\x00")
	parsedPageIndexDescriptorDigestDomainV1 = []byte("analytix.parsed-page-index-descriptor/digest/v1\x00")
	parsedGenerationReceiptDigestDomainV1   = []byte("analytix.parsed-generation-receipt/digest/v1\x00")
)

type ParsedPageDescriptorV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Purpose                string `json:"purpose"`
	PageNumber             uint64 `json:"pageNumber"`
	PageDigest             string `json:"pageDigest"`
	PageSHA256             string `json:"pageSha256"`
	PageByteLength         uint64 `json:"pageByteLength"`
	OutcomeCount           uint32 `json:"outcomeCount"`
	AcceptedCount          uint32 `json:"acceptedCount"`
	RejectedCount          uint32 `json:"rejectedCount"`
	DuplicateCount         uint32 `json:"duplicateCount"`
	FirstOccurrenceOrdinal uint64 `json:"firstOccurrenceOrdinal"`
	LastOccurrenceOrdinal  uint64 `json:"lastOccurrenceOrdinal"`
	FirstLocatorDigest     string `json:"firstLocatorDigest"`
	LastLocatorDigest      string `json:"lastLocatorDigest"`
	FirstSourceRecordID    string `json:"firstSourceRecordId"`
	LastSourceRecordID     string `json:"lastSourceRecordId"`
	DescriptorDigest       string `json:"descriptorDigest"`
}

type ParsedPageIndexV1 struct {
	SchemaVersion   int                        `json:"schemaVersion"`
	Purpose         string                     `json:"purpose"`
	Identity        ParsedGenerationIdentityV1 `json:"identity"`
	IndexPageNumber uint64                     `json:"indexPageNumber"`
	PageDescriptors []ParsedPageDescriptorV1   `json:"pageDescriptors"`
	PageCount       uint32                     `json:"pageCount"`
	OutcomeCount    uint64                     `json:"outcomeCount"`
	AcceptedCount   uint64                     `json:"acceptedCount"`
	RejectedCount   uint64                     `json:"rejectedCount"`
	DuplicateCount  uint64                     `json:"duplicateCount"`
	IndexPageDigest string                     `json:"indexPageDigest"`
}

type ParsedPageIndexDescriptorV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Purpose                string `json:"purpose"`
	IndexPageNumber        uint64 `json:"indexPageNumber"`
	IndexPageDigest        string `json:"indexPageDigest"`
	IndexPageSHA256        string `json:"indexPageSha256"`
	IndexPageByteLength    uint64 `json:"indexPageByteLength"`
	FirstPageNumber        uint64 `json:"firstPageNumber"`
	LastPageNumber         uint64 `json:"lastPageNumber"`
	PageCount              uint32 `json:"pageCount"`
	FirstOccurrenceOrdinal uint64 `json:"firstOccurrenceOrdinal"`
	LastOccurrenceOrdinal  uint64 `json:"lastOccurrenceOrdinal"`
	FirstLocatorDigest     string `json:"firstLocatorDigest"`
	LastLocatorDigest      string `json:"lastLocatorDigest"`
	FirstSourceRecordID    string `json:"firstSourceRecordId"`
	LastSourceRecordID     string `json:"lastSourceRecordId"`
	OutcomeCount           uint64 `json:"outcomeCount"`
	AcceptedCount          uint64 `json:"acceptedCount"`
	RejectedCount          uint64 `json:"rejectedCount"`
	DuplicateCount         uint64 `json:"duplicateCount"`
	DescriptorDigest       string `json:"descriptorDigest"`
}

// ParsedGenerationReceiptV1 is written only after all page-index objects. It
// is also the V1 classification-ledger root because every source occurrence's
// closed disposition lives in the indexed pages. This explicit same-object
// alias prevents a second caller-supplied classification digest.
type ParsedGenerationReceiptV1 struct {
	SchemaVersion        int                           `json:"schemaVersion"`
	Purpose              string                        `json:"purpose"`
	Identity             ParsedGenerationIdentityV1    `json:"identity"`
	IndexPageDescriptors []ParsedPageIndexDescriptorV1 `json:"indexPageDescriptors"`
	IndexPageCount       uint32                        `json:"indexPageCount"`
	PageCount            uint64                        `json:"pageCount"`
	OutcomeCount         uint64                        `json:"outcomeCount"`
	AcceptedCount        uint64                        `json:"acceptedCount"`
	RejectedCount        uint64                        `json:"rejectedCount"`
	DuplicateCount       uint64                        `json:"duplicateCount"`
	TimezoneSemantics    string                        `json:"timezoneSemantics"`
	CurrencySemantics    string                        `json:"currencySemantics"`
	ReceiptDigest        string                        `json:"receiptDigest"`
}

func newParsedPageDescriptorV1(page ParsedPageV1) (ParsedPageDescriptorV1, error) {
	if ValidateParsedPageV1(page) != nil {
		return ParsedPageDescriptorV1{}, errors.New("parsed page descriptor input is invalid")
	}
	body, err := ParsedPageV1Bytes(page)
	if err != nil {
		return ParsedPageDescriptorV1{}, err
	}
	first := page.Outcomes[0]
	last := page.Outcomes[len(page.Outcomes)-1]
	descriptor := ParsedPageDescriptorV1{
		SchemaVersion: ParsedPageDescriptorSchemaVersionV1, Purpose: ParsedPageDescriptorPurposeV1,
		PageNumber: page.PageNumber, PageDigest: page.PageDigest, PageSHA256: domainsecurity.SHA256Hex(body),
		PageByteLength: uint64(len(body)), OutcomeCount: page.OutcomeCount, AcceptedCount: page.AcceptedCount,
		RejectedCount: page.RejectedCount, DuplicateCount: page.DuplicateCount,
		FirstOccurrenceOrdinal: first.OccurrenceOrdinal, LastOccurrenceOrdinal: last.OccurrenceOrdinal,
		FirstLocatorDigest: first.Locator.LocatorDigest, LastLocatorDigest: last.Locator.LocatorDigest,
		FirstSourceRecordID: first.SourceRecordID, LastSourceRecordID: last.SourceRecordID,
	}
	descriptor.DescriptorDigest = parsedPageDescriptorDigestV1(descriptor)
	if ValidateParsedPageDescriptorV1(descriptor) != nil {
		return ParsedPageDescriptorV1{}, errors.New("parsed page descriptor is invalid")
	}
	return descriptor, nil
}

func ValidateParsedPageDescriptorV1(descriptor ParsedPageDescriptorV1) error {
	if descriptor.SchemaVersion != ParsedPageDescriptorSchemaVersionV1 || descriptor.Purpose != ParsedPageDescriptorPurposeV1 ||
		descriptor.PageNumber == 0 || descriptor.PageNumber > maxParsedGenerationPageCountV1 ||
		!validSourceRowSHA256V1(descriptor.PageDigest) || !validSourceRowSHA256V1(descriptor.PageSHA256) ||
		descriptor.PageByteLength == 0 || descriptor.PageByteLength > 1024*1024 ||
		descriptor.OutcomeCount == 0 || descriptor.OutcomeCount > 500 ||
		descriptor.AcceptedCount > descriptor.OutcomeCount ||
		descriptor.RejectedCount > descriptor.OutcomeCount-descriptor.AcceptedCount ||
		descriptor.DuplicateCount != descriptor.OutcomeCount-descriptor.AcceptedCount-descriptor.RejectedCount ||
		descriptor.FirstOccurrenceOrdinal == 0 || descriptor.LastOccurrenceOrdinal < descriptor.FirstOccurrenceOrdinal ||
		descriptor.LastOccurrenceOrdinal-descriptor.FirstOccurrenceOrdinal+1 != uint64(descriptor.OutcomeCount) ||
		!validSourceRowSHA256V1(descriptor.FirstLocatorDigest) || !validSourceRowSHA256V1(descriptor.LastLocatorDigest) ||
		!validSourceRowRecordIDV1(descriptor.FirstSourceRecordID) || !validSourceRowRecordIDV1(descriptor.LastSourceRecordID) ||
		!validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != parsedPageDescriptorDigestV1(descriptor) {
		return errors.New("parsed page descriptor is invalid")
	}
	return nil
}

func ValidateParsedPageAgainstDescriptorV1(descriptor ParsedPageDescriptorV1, page ParsedPageV1) error {
	expected, err := newParsedPageDescriptorV1(page)
	if err != nil || descriptor != expected {
		return errors.New("parsed page does not match its exact descriptor")
	}
	return nil
}

func newParsedPageIndexV1(
	identity ParsedGenerationIdentityV1,
	indexPageNumber uint64,
	pages []ParsedPageV1,
) (ParsedPageIndexV1, error) {
	if len(pages) == 0 || len(pages) > maxParsedPagesPerIndexV1 {
		return ParsedPageIndexV1{}, errors.New("parsed page index input is invalid")
	}
	index := ParsedPageIndexV1{
		SchemaVersion: ParsedPageIndexSchemaVersionV1, Purpose: ParsedPageIndexPurposeV1,
		Identity: identity, IndexPageNumber: indexPageNumber, PageCount: uint32(len(pages)),
		PageDescriptors: make([]ParsedPageDescriptorV1, len(pages)),
	}
	for pageIndex := range pages {
		descriptor, err := newParsedPageDescriptorV1(pages[pageIndex])
		if err != nil {
			return ParsedPageIndexV1{}, err
		}
		index.PageDescriptors[pageIndex] = descriptor
		index.OutcomeCount += uint64(descriptor.OutcomeCount)
		index.AcceptedCount += uint64(descriptor.AcceptedCount)
		index.RejectedCount += uint64(descriptor.RejectedCount)
		index.DuplicateCount += uint64(descriptor.DuplicateCount)
	}
	index.IndexPageDigest = parsedPageIndexDigestV1(index)
	if ValidateParsedPageIndexWithPagesV1(index, pages) != nil {
		return ParsedPageIndexV1{}, errors.New("parsed page index hierarchy is invalid")
	}
	return index, nil
}

func ValidateParsedPageIndexV1(index ParsedPageIndexV1) error {
	if index.SchemaVersion != ParsedPageIndexSchemaVersionV1 || index.Purpose != ParsedPageIndexPurposeV1 ||
		ValidateParsedGenerationIdentityV1(index.Identity) != nil || index.IndexPageNumber == 0 ||
		index.IndexPageNumber > maxParsedPageIndexesV1 || len(index.PageDescriptors) == 0 ||
		len(index.PageDescriptors) > maxParsedPagesPerIndexV1 || index.PageCount != uint32(len(index.PageDescriptors)) ||
		index.OutcomeCount == 0 || index.OutcomeCount > maxSourceRowJSONIntegerV1 ||
		index.AcceptedCount > index.OutcomeCount || index.RejectedCount > index.OutcomeCount-index.AcceptedCount ||
		index.DuplicateCount != index.OutcomeCount-index.AcceptedCount-index.RejectedCount ||
		!validSourceRowSHA256V1(index.IndexPageDigest) {
		return errors.New("parsed page index is invalid")
	}
	expectedFirstPage := (index.IndexPageNumber-1)*maxParsedPagesPerIndexV1 + 1
	var outcomes, accepted, rejected, duplicate uint64
	for descriptorIndex, descriptor := range index.PageDescriptors {
		if ValidateParsedPageDescriptorV1(descriptor) != nil ||
			descriptor.PageNumber != expectedFirstPage+uint64(descriptorIndex) ||
			descriptorIndex > 0 && descriptor.FirstOccurrenceOrdinal != index.PageDescriptors[descriptorIndex-1].LastOccurrenceOrdinal+1 {
			return errors.New("parsed page index descriptors are not contiguous")
		}
		outcomes += uint64(descriptor.OutcomeCount)
		accepted += uint64(descriptor.AcceptedCount)
		rejected += uint64(descriptor.RejectedCount)
		duplicate += uint64(descriptor.DuplicateCount)
	}
	if outcomes != index.OutcomeCount || accepted != index.AcceptedCount || rejected != index.RejectedCount ||
		duplicate != index.DuplicateCount || index.IndexPageDigest != parsedPageIndexDigestV1(index) {
		return errors.New("parsed page index totals or digest are invalid")
	}
	body, err := json.Marshal(index)
	if err != nil || len(body) > maxParsedPageIndexBytesV1 {
		return errors.New("parsed page index exceeds its canonical byte limit")
	}
	return nil
}

func ValidateParsedPageIndexWithPagesV1(index ParsedPageIndexV1, pages []ParsedPageV1) error {
	if ValidateParsedPageIndexV1(index) != nil || len(pages) != len(index.PageDescriptors) {
		return errors.New("parsed page index hierarchy is invalid")
	}
	for pageIndex := range pages {
		if pages[pageIndex].Identity != index.Identity ||
			ValidateParsedPageAgainstDescriptorV1(index.PageDescriptors[pageIndex], pages[pageIndex]) != nil {
			return errors.New("parsed page index contains a mismatched page")
		}
	}
	return nil
}

func ParseParsedPageIndexV1(raw []byte) (ParsedPageIndexV1, error) {
	var index ParsedPageIndexV1
	if err := parseCanonicalSourceRowContractV1(raw, &index, maxParsedPageIndexBytesV1, 100_000, maxSourceRowPolicyTextBytesV1); err != nil {
		return ParsedPageIndexV1{}, err
	}
	return index, ValidateParsedPageIndexV1(index)
}

func ParsedPageIndexV1Bytes(index ParsedPageIndexV1) ([]byte, error) {
	if ValidateParsedPageIndexV1(index) != nil {
		return nil, errors.New("parsed page index is invalid")
	}
	return json.Marshal(index)
}

func newParsedPageIndexDescriptorV1(index ParsedPageIndexV1) (ParsedPageIndexDescriptorV1, error) {
	if ValidateParsedPageIndexV1(index) != nil {
		return ParsedPageIndexDescriptorV1{}, errors.New("parsed page index descriptor input is invalid")
	}
	body, err := ParsedPageIndexV1Bytes(index)
	if err != nil {
		return ParsedPageIndexDescriptorV1{}, err
	}
	first := index.PageDescriptors[0]
	last := index.PageDescriptors[len(index.PageDescriptors)-1]
	descriptor := ParsedPageIndexDescriptorV1{
		SchemaVersion: ParsedPageIndexDescriptorVersionV1, Purpose: ParsedPageIndexDescriptorPurposeV1,
		IndexPageNumber: index.IndexPageNumber, IndexPageDigest: index.IndexPageDigest,
		IndexPageSHA256: domainsecurity.SHA256Hex(body), IndexPageByteLength: uint64(len(body)),
		FirstPageNumber: first.PageNumber, LastPageNumber: last.PageNumber, PageCount: index.PageCount,
		FirstOccurrenceOrdinal: first.FirstOccurrenceOrdinal, LastOccurrenceOrdinal: last.LastOccurrenceOrdinal,
		FirstLocatorDigest: first.FirstLocatorDigest, LastLocatorDigest: last.LastLocatorDigest,
		FirstSourceRecordID: first.FirstSourceRecordID, LastSourceRecordID: last.LastSourceRecordID,
		OutcomeCount: index.OutcomeCount, AcceptedCount: index.AcceptedCount, RejectedCount: index.RejectedCount,
		DuplicateCount: index.DuplicateCount,
	}
	descriptor.DescriptorDigest = parsedPageIndexDescriptorDigestV1(descriptor)
	if ValidateParsedPageIndexDescriptorV1(descriptor) != nil {
		return ParsedPageIndexDescriptorV1{}, errors.New("parsed page index descriptor is invalid")
	}
	return descriptor, nil
}

func ValidateParsedPageIndexDescriptorV1(descriptor ParsedPageIndexDescriptorV1) error {
	if descriptor.SchemaVersion != ParsedPageIndexDescriptorVersionV1 || descriptor.Purpose != ParsedPageIndexDescriptorPurposeV1 ||
		descriptor.IndexPageNumber == 0 || descriptor.IndexPageNumber > maxParsedPageIndexesV1 ||
		!validSourceRowSHA256V1(descriptor.IndexPageDigest) || !validSourceRowSHA256V1(descriptor.IndexPageSHA256) ||
		descriptor.IndexPageByteLength == 0 || descriptor.IndexPageByteLength > maxParsedPageIndexBytesV1 ||
		descriptor.FirstPageNumber == 0 || descriptor.LastPageNumber < descriptor.FirstPageNumber ||
		descriptor.PageCount == 0 || descriptor.PageCount > maxParsedPagesPerIndexV1 ||
		descriptor.LastPageNumber-descriptor.FirstPageNumber+1 != uint64(descriptor.PageCount) ||
		descriptor.FirstOccurrenceOrdinal == 0 || descriptor.LastOccurrenceOrdinal < descriptor.FirstOccurrenceOrdinal ||
		!validSourceRowSHA256V1(descriptor.FirstLocatorDigest) || !validSourceRowSHA256V1(descriptor.LastLocatorDigest) ||
		!validSourceRowRecordIDV1(descriptor.FirstSourceRecordID) || !validSourceRowRecordIDV1(descriptor.LastSourceRecordID) ||
		descriptor.OutcomeCount == 0 || descriptor.AcceptedCount > descriptor.OutcomeCount ||
		descriptor.RejectedCount > descriptor.OutcomeCount-descriptor.AcceptedCount ||
		descriptor.DuplicateCount != descriptor.OutcomeCount-descriptor.AcceptedCount-descriptor.RejectedCount ||
		!validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != parsedPageIndexDescriptorDigestV1(descriptor) {
		return errors.New("parsed page index descriptor is invalid")
	}
	return nil
}

func ValidateParsedPageIndexAgainstDescriptorV1(
	descriptor ParsedPageIndexDescriptorV1,
	index ParsedPageIndexV1,
) error {
	expected, err := newParsedPageIndexDescriptorV1(index)
	if err != nil || descriptor != expected {
		return errors.New("parsed page index does not match its exact descriptor")
	}
	return nil
}

func newParsedGenerationReceiptV1(
	identity ParsedGenerationIdentityV1,
	indexes []ParsedPageIndexV1,
) (ParsedGenerationReceiptV1, error) {
	if len(indexes) == 0 || len(indexes) > maxParsedPageIndexesV1 {
		return ParsedGenerationReceiptV1{}, errors.New("parsed generation receipt input is invalid")
	}
	receipt := ParsedGenerationReceiptV1{
		SchemaVersion: ParsedGenerationReceiptVersionV1, Purpose: ParsedGenerationReceiptPurposeV1,
		Identity: identity, IndexPageCount: uint32(len(indexes)),
		IndexPageDescriptors: make([]ParsedPageIndexDescriptorV1, len(indexes)),
		TimezoneSemantics:    ParsedGenerationTimezoneUnresolvedV1,
		CurrencySemantics:    ParsedGenerationCurrencyUnresolvedV1,
	}
	for index := range indexes {
		descriptor, err := newParsedPageIndexDescriptorV1(indexes[index])
		if err != nil {
			return ParsedGenerationReceiptV1{}, err
		}
		receipt.IndexPageDescriptors[index] = descriptor
		receipt.PageCount += uint64(descriptor.PageCount)
		receipt.OutcomeCount += descriptor.OutcomeCount
		receipt.AcceptedCount += descriptor.AcceptedCount
		receipt.RejectedCount += descriptor.RejectedCount
		receipt.DuplicateCount += descriptor.DuplicateCount
	}
	receipt.ReceiptDigest = parsedGenerationReceiptDigestV1(receipt)
	if ValidateParsedGenerationReceiptHierarchyV1(receipt, indexes) != nil {
		return ParsedGenerationReceiptV1{}, errors.New("parsed generation receipt hierarchy is invalid")
	}
	return receipt, nil
}

func ValidateParsedGenerationReceiptV1(receipt ParsedGenerationReceiptV1) error {
	if receipt.SchemaVersion != ParsedGenerationReceiptVersionV1 || receipt.Purpose != ParsedGenerationReceiptPurposeV1 ||
		ValidateParsedGenerationIdentityV1(receipt.Identity) != nil || len(receipt.IndexPageDescriptors) == 0 ||
		len(receipt.IndexPageDescriptors) > maxParsedPageIndexesV1 || receipt.IndexPageCount != uint32(len(receipt.IndexPageDescriptors)) ||
		receipt.PageCount == 0 || receipt.PageCount > maxParsedGenerationPageCountV1 ||
		receipt.OutcomeCount == 0 || receipt.OutcomeCount > maxSourceRowJSONIntegerV1 ||
		receipt.AcceptedCount > receipt.OutcomeCount || receipt.RejectedCount > receipt.OutcomeCount-receipt.AcceptedCount ||
		receipt.DuplicateCount != receipt.OutcomeCount-receipt.AcceptedCount-receipt.RejectedCount ||
		receipt.TimezoneSemantics != ParsedGenerationTimezoneUnresolvedV1 ||
		receipt.CurrencySemantics != ParsedGenerationCurrencyUnresolvedV1 ||
		!validSourceRowSHA256V1(receipt.ReceiptDigest) {
		return errors.New("parsed generation receipt is invalid")
	}
	var pages, outcomes, accepted, rejected, duplicate uint64
	for index, descriptor := range receipt.IndexPageDescriptors {
		if ValidateParsedPageIndexDescriptorV1(descriptor) != nil || descriptor.IndexPageNumber != uint64(index+1) ||
			index > 0 && (descriptor.FirstPageNumber != receipt.IndexPageDescriptors[index-1].LastPageNumber+1 ||
				descriptor.FirstOccurrenceOrdinal != receipt.IndexPageDescriptors[index-1].LastOccurrenceOrdinal+1) ||
			index < len(receipt.IndexPageDescriptors)-1 && descriptor.PageCount != maxParsedPagesPerIndexV1 {
			return errors.New("parsed generation receipt index descriptors are not complete and contiguous")
		}
		pages += uint64(descriptor.PageCount)
		outcomes += descriptor.OutcomeCount
		accepted += descriptor.AcceptedCount
		rejected += descriptor.RejectedCount
		duplicate += descriptor.DuplicateCount
	}
	if pages != receipt.PageCount || outcomes != receipt.OutcomeCount || accepted != receipt.AcceptedCount ||
		rejected != receipt.RejectedCount || duplicate != receipt.DuplicateCount ||
		receipt.ReceiptDigest != parsedGenerationReceiptDigestV1(receipt) {
		return errors.New("parsed generation receipt totals or digest are invalid")
	}
	body, err := json.Marshal(receipt)
	if err != nil || len(body) > maxParsedGenerationReceiptV1 {
		return errors.New("parsed generation receipt exceeds its canonical byte limit")
	}
	return nil
}

func ValidateParsedGenerationReceiptHierarchyV1(
	receipt ParsedGenerationReceiptV1,
	indexes []ParsedPageIndexV1,
) error {
	if ValidateParsedGenerationReceiptV1(receipt) != nil || len(indexes) != len(receipt.IndexPageDescriptors) {
		return errors.New("parsed generation receipt hierarchy is invalid")
	}
	for index := range indexes {
		if indexes[index].Identity != receipt.Identity ||
			ValidateParsedPageIndexAgainstDescriptorV1(receipt.IndexPageDescriptors[index], indexes[index]) != nil {
			return errors.New("parsed generation receipt contains a mismatched page index")
		}
	}
	return nil
}

func ValidateParsedGenerationHierarchyV1(
	receipt ParsedGenerationReceiptV1,
	indexes []ParsedPageIndexV1,
	pages []ParsedPageV1,
) error {
	if ValidateParsedGenerationReceiptHierarchyV1(receipt, indexes) != nil ||
		ValidateParsedPageSequenceV1(pages) != nil || uint64(len(pages)) != receipt.PageCount {
		return errors.New("parsed generation hierarchy is invalid")
	}
	pageOffset := 0
	for index := range indexes {
		pageCount := len(indexes[index].PageDescriptors)
		if pageOffset > len(pages)-pageCount ||
			ValidateParsedPageIndexWithPagesV1(indexes[index], pages[pageOffset:pageOffset+pageCount]) != nil {
			return errors.New("parsed generation hierarchy page membership is incomplete")
		}
		pageOffset += pageCount
	}
	if pageOffset != len(pages) {
		return errors.New("parsed generation hierarchy contains unindexed pages")
	}
	return nil
}

func ParseParsedGenerationReceiptV1(raw []byte) (ParsedGenerationReceiptV1, error) {
	var receipt ParsedGenerationReceiptV1
	if err := parseCanonicalSourceRowContractV1(raw, &receipt, maxParsedGenerationReceiptV1, 100_000, maxSourceRowPolicyTextBytesV1); err != nil {
		return ParsedGenerationReceiptV1{}, err
	}
	return receipt, ValidateParsedGenerationReceiptV1(receipt)
}

func ParsedGenerationReceiptV1Bytes(receipt ParsedGenerationReceiptV1) ([]byte, error) {
	if ValidateParsedGenerationReceiptV1(receipt) != nil {
		return nil, errors.New("parsed generation receipt is invalid")
	}
	return json.Marshal(receipt)
}

func ParsedGenerationReceiptClassificationTripleV1(
	receipt ParsedGenerationReceiptV1,
) (digest string, sha256 string, byteLength uint64, err error) {
	body, err := ParsedGenerationReceiptV1Bytes(receipt)
	if err != nil {
		return "", "", 0, err
	}
	return receipt.ReceiptDigest, domainsecurity.SHA256Hex(body), uint64(len(body)), nil
}

func parsedPageDescriptorDigestV1(descriptor ParsedPageDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedPageDescriptorDigestDomainV1...), body...))
}

func parsedPageIndexDigestV1(index ParsedPageIndexV1) string {
	index.IndexPageDigest = ""
	body, _ := json.Marshal(index)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedPageIndexDigestDomainV1...), body...))
}

func parsedPageIndexDescriptorDigestV1(descriptor ParsedPageIndexDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedPageIndexDescriptorDigestDomainV1...), body...))
}

func parsedGenerationReceiptDigestV1(receipt ParsedGenerationReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedGenerationReceiptDigestDomainV1...), body...))
}
