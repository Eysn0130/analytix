package evidence

import (
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SourceRowLineageSchemaVersionV1 = 1
	SourceRowLineagePurposeV1       = "analytix.source-row-lineage/v1"

	maxSourceRowLineageBytesV1        = 64 * 1024
	maxParsedGenerationReceiptBytesV1 = maxParsedGenerationReceiptV1
)

var sourceRowLineageDigestDomainV1 = []byte("analytix.source-row-lineage/digest/v1\x00")

// SourceRowLineageV1 is restricted, pseudonymous private-CAS comparison
// material for one exact parsed row. Stable hashes remain correlatable and
// must not enter ordinary UI/SSE/history/logs/reports/exports. It deliberately
// excludes dataset snapshot ids, currentness, context ids, raw paths,
// free-form diagnostics, and factual authority.
//
// Structural validity is not proof that any referenced private-CAS object
// exists. Production admission must exact-read and re-parse the raw manifest,
// raw entry, generation receipt, and parsed page before using this material.
type SourceRowLineageV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`

	PolicyID         string             `json:"policyId"`
	PolicyDigest     string             `json:"policyDigest"`
	BindingKeyDigest string             `json:"bindingKeyDigest"`
	SourceRecordID   string             `json:"sourceRecordId"`
	Locator          SourceRowLocatorV1 `json:"locator"`

	RawArtifactManifestDigest         string `json:"rawArtifactManifestDigest"`
	RawArtifactManifestSHA256         string `json:"rawArtifactManifestSha256"`
	RawArtifactManifestByteLength     uint64 `json:"rawArtifactManifestByteLength"`
	RawArtifactManifestPageDigest     string `json:"rawArtifactManifestPageDigest"`
	RawArtifactManifestPageSHA256     string `json:"rawArtifactManifestPageSha256"`
	RawArtifactManifestPageByteLength uint64 `json:"rawArtifactManifestPageByteLength"`
	RawArtifactOrdinal                uint64 `json:"rawArtifactOrdinal"`
	RawArtifactEntryDigest            string `json:"rawArtifactEntryDigest"`
	RawArtifactEntrySHA256            string `json:"rawArtifactEntrySha256"`
	RawArtifactEntryByteLength        uint64 `json:"rawArtifactEntryByteLength"`
	RawSourceLocatorDigest            string `json:"rawSourceLocatorDigest"`
	RawSourceLocatorSHA256            string `json:"rawSourceLocatorSha256"`
	RawSourceLocatorByteLength        uint64 `json:"rawSourceLocatorByteLength"`
	SourceArtifactSHA256              string `json:"sourceArtifactSha256"`
	SourceArtifactByteLength          uint64 `json:"sourceArtifactByteLength"`

	ParsedGenerationReceiptDigest     string `json:"parsedGenerationReceiptDigest"`
	ParsedGenerationReceiptSHA256     string `json:"parsedGenerationReceiptSha256"`
	ParsedGenerationReceiptByteLength uint64 `json:"parsedGenerationReceiptByteLength"`
	ParsedPageDigest                  string `json:"parsedPageDigest"`
	ParsedPageSHA256                  string `json:"parsedPageSha256"`
	ParsedPageByteLength              uint64 `json:"parsedPageByteLength"`
	ParsedRowOrdinal                  uint32 `json:"parsedRowOrdinal"`

	ParserID               string `json:"parserId"`
	ParserVersion          string `json:"parserVersion"`
	MappingDigest          string `json:"mappingDigest"`
	ProjectionSchemaDigest string `json:"projectionSchemaDigest"`
	ReaderModeDigest       string `json:"readerModeDigest"`
	CanonicalRowSHA256     string `json:"canonicalRowSha256"`
	LineageDigest          string `json:"lineageDigest"`
}

type sourceRowLineageInputV1 struct {
	Policy  SourceRowProducerPolicyV1
	Binding domainsecurity.DatasetSnapshotBindingKeyV1

	SourceRecordID string
	Locator        SourceRowLocatorV1

	RawArtifactManifestDigest         string
	RawArtifactManifestSHA256         string
	RawArtifactManifestByteLength     uint64
	RawArtifactManifestPageDigest     string
	RawArtifactManifestPageSHA256     string
	RawArtifactManifestPageByteLength uint64
	RawArtifactOrdinal                uint64
	RawArtifactEntryDigest            string
	RawArtifactEntrySHA256            string
	RawArtifactEntryByteLength        uint64
	RawSourceLocatorDigest            string
	RawSourceLocatorSHA256            string
	RawSourceLocatorByteLength        uint64
	SourceArtifactSHA256              string
	SourceArtifactByteLength          uint64

	ParsedGenerationReceiptDigest     string
	ParsedGenerationReceiptSHA256     string
	ParsedGenerationReceiptByteLength uint64
	ParsedPageDigest                  string
	ParsedPageSHA256                  string
	ParsedPageByteLength              uint64
	ParsedRowOrdinal                  uint32

	MappingDigest          string
	ProjectionSchemaDigest string
	ReaderModeDigest       string
	CanonicalRowSHA256     string
}

type sourceRowRecordWithLineageInputV1 struct {
	Policy  SourceRowProducerPolicyV1
	Binding domainsecurity.DatasetSnapshotBindingKeyV1
	Locator SourceRowLocatorInputV1

	RawArtifactManifestDigest         string
	RawArtifactManifestSHA256         string
	RawArtifactManifestByteLength     uint64
	RawArtifactManifestPageDigest     string
	RawArtifactManifestPageSHA256     string
	RawArtifactManifestPageByteLength uint64
	RawArtifactOrdinal                uint64
	RawArtifactEntryDigest            string
	RawArtifactEntrySHA256            string
	RawArtifactEntryByteLength        uint64
	RawSourceLocatorDigest            string
	RawSourceLocatorSHA256            string
	RawSourceLocatorByteLength        uint64
	SourceArtifactSHA256              string
	SourceArtifactByteLength          uint64

	ParsedGenerationReceiptDigest     string
	ParsedGenerationReceiptSHA256     string
	ParsedGenerationReceiptByteLength uint64
	ParsedPageDigest                  string
	ParsedPageSHA256                  string
	ParsedPageByteLength              uint64
	ParsedRowOrdinal                  uint32

	MappingDigest          string
	ProjectionSchemaDigest string
	ReaderModeDigest       string
	CanonicalRowSHA256     string
}

// newSourceRowRecordWithLineageV1 is package-private until a bounded app-layer
// admission callback exact-reads every referenced private-CAS object. Keeping
// it private prevents an MCP/provider caller from turning self-reported hashes
// into host row authority.
func newSourceRowRecordWithLineageV1(
	input sourceRowRecordWithLineageInputV1,
) (SourceRowRecordV1, SourceRowLineageV1, error) {
	locator, err := NewSourceRowLocatorV1(input.Policy, input.Locator)
	if err != nil {
		return SourceRowRecordV1{}, SourceRowLineageV1{}, err
	}
	recordID, err := DeriveSourceRowRecordIDV1(input.Policy, input.Binding, input.SourceArtifactSHA256, locator)
	if err != nil {
		return SourceRowRecordV1{}, SourceRowLineageV1{}, err
	}
	lineage, err := newSourceRowLineageV1(sourceRowLineageInputV1{
		Policy: input.Policy, Binding: input.Binding, SourceRecordID: recordID, Locator: locator,
		RawArtifactManifestDigest:         input.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:         input.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:     input.RawArtifactManifestByteLength,
		RawArtifactManifestPageDigest:     input.RawArtifactManifestPageDigest,
		RawArtifactManifestPageSHA256:     input.RawArtifactManifestPageSHA256,
		RawArtifactManifestPageByteLength: input.RawArtifactManifestPageByteLength,
		RawArtifactOrdinal:                input.RawArtifactOrdinal,
		RawArtifactEntryDigest:            input.RawArtifactEntryDigest, RawArtifactEntrySHA256: input.RawArtifactEntrySHA256,
		RawArtifactEntryByteLength: input.RawArtifactEntryByteLength, RawSourceLocatorDigest: input.RawSourceLocatorDigest,
		RawSourceLocatorSHA256: input.RawSourceLocatorSHA256, RawSourceLocatorByteLength: input.RawSourceLocatorByteLength,
		SourceArtifactSHA256: input.SourceArtifactSHA256, SourceArtifactByteLength: input.SourceArtifactByteLength,
		ParsedGenerationReceiptDigest:     input.ParsedGenerationReceiptDigest,
		ParsedGenerationReceiptSHA256:     input.ParsedGenerationReceiptSHA256,
		ParsedGenerationReceiptByteLength: input.ParsedGenerationReceiptByteLength,
		ParsedPageDigest:                  input.ParsedPageDigest, ParsedPageSHA256: input.ParsedPageSHA256,
		ParsedPageByteLength: input.ParsedPageByteLength, ParsedRowOrdinal: input.ParsedRowOrdinal,
		MappingDigest: input.MappingDigest, ProjectionSchemaDigest: input.ProjectionSchemaDigest,
		ReaderModeDigest: input.ReaderModeDigest, CanonicalRowSHA256: input.CanonicalRowSHA256,
	})
	if err != nil {
		return SourceRowRecordV1{}, SourceRowLineageV1{}, err
	}
	lineageBody, err := SourceRowLineageV1Bytes(lineage)
	if err != nil {
		return SourceRowRecordV1{}, SourceRowLineageV1{}, err
	}
	record, err := newSourceRowRecordV1(sourceRowRecordInputV1{
		Policy: input.Policy, Binding: input.Binding, SourceArtifactSHA256: input.SourceArtifactSHA256,
		CanonicalRowSHA256: input.CanonicalRowSHA256, LineageDigest: lineage.LineageDigest,
		LineageSHA256: domainsecurity.SHA256Hex(lineageBody), LineageByteLength: uint64(len(lineageBody)), Locator: input.Locator,
	})
	if err != nil {
		return SourceRowRecordV1{}, SourceRowLineageV1{}, err
	}
	if err := ValidateSourceRowRecordAgainstLineageV1(input.Policy, input.Binding, record, lineage); err != nil {
		return SourceRowRecordV1{}, SourceRowLineageV1{}, err
	}
	return record, lineage, nil
}

func newSourceRowLineageV1(input sourceRowLineageInputV1) (SourceRowLineageV1, error) {
	lineage := SourceRowLineageV1{
		SchemaVersion: SourceRowLineageSchemaVersionV1, Purpose: SourceRowLineagePurposeV1,
		PolicyID: input.Policy.PolicyID, PolicyDigest: input.Policy.PolicyDigest,
		BindingKeyDigest: input.Binding.BindingKeyDigest, SourceRecordID: input.SourceRecordID, Locator: input.Locator,
		RawArtifactManifestDigest:         input.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:         input.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:     input.RawArtifactManifestByteLength,
		RawArtifactManifestPageDigest:     input.RawArtifactManifestPageDigest,
		RawArtifactManifestPageSHA256:     input.RawArtifactManifestPageSHA256,
		RawArtifactManifestPageByteLength: input.RawArtifactManifestPageByteLength,
		RawArtifactOrdinal:                input.RawArtifactOrdinal,
		RawArtifactEntryDigest:            input.RawArtifactEntryDigest, RawArtifactEntrySHA256: input.RawArtifactEntrySHA256,
		RawArtifactEntryByteLength: input.RawArtifactEntryByteLength, RawSourceLocatorDigest: input.RawSourceLocatorDigest,
		RawSourceLocatorSHA256: input.RawSourceLocatorSHA256, RawSourceLocatorByteLength: input.RawSourceLocatorByteLength,
		SourceArtifactSHA256: input.SourceArtifactSHA256, SourceArtifactByteLength: input.SourceArtifactByteLength,
		ParsedGenerationReceiptDigest:     input.ParsedGenerationReceiptDigest,
		ParsedGenerationReceiptSHA256:     input.ParsedGenerationReceiptSHA256,
		ParsedGenerationReceiptByteLength: input.ParsedGenerationReceiptByteLength,
		ParsedPageDigest:                  input.ParsedPageDigest, ParsedPageSHA256: input.ParsedPageSHA256,
		ParsedPageByteLength: input.ParsedPageByteLength, ParsedRowOrdinal: input.ParsedRowOrdinal,
		ParserID: input.Policy.ParserID, ParserVersion: input.Policy.ParserVersion,
		MappingDigest: input.MappingDigest, ProjectionSchemaDigest: input.ProjectionSchemaDigest,
		ReaderModeDigest: input.ReaderModeDigest, CanonicalRowSHA256: input.CanonicalRowSHA256,
	}
	lineage.LineageDigest = sourceRowLineageDigestV1(lineage)
	if err := ValidateSourceRowLineageForBindingV1(input.Policy, input.Binding, lineage); err != nil {
		return SourceRowLineageV1{}, err
	}
	return lineage, nil
}

func ValidateSourceRowLineageV1(lineage SourceRowLineageV1) error {
	policy, ok := ResolveSourceRowProducerPolicyV1(lineage.PolicyID)
	if !ok || lineage.SchemaVersion != SourceRowLineageSchemaVersionV1 || lineage.Purpose != SourceRowLineagePurposeV1 ||
		lineage.PolicyDigest != policy.PolicyDigest || !validSourceRowSHA256V1(lineage.BindingKeyDigest) ||
		!validSourceRowRecordIDV1(lineage.SourceRecordID) || ValidateSourceRowLocatorV1(policy, lineage.Locator) != nil ||
		!validSourceRowSHA256V1(lineage.RawArtifactManifestDigest) || !validSourceRowSHA256V1(lineage.RawArtifactManifestSHA256) ||
		lineage.RawArtifactManifestByteLength == 0 || lineage.RawArtifactManifestByteLength > maxRawArtifactManifestBytesV1 ||
		!validSourceRowSHA256V1(lineage.RawArtifactManifestPageDigest) ||
		!validSourceRowSHA256V1(lineage.RawArtifactManifestPageSHA256) || lineage.RawArtifactManifestPageByteLength == 0 ||
		lineage.RawArtifactManifestPageByteLength > maxRawArtifactManifestPageBytesV1 ||
		lineage.RawArtifactOrdinal == 0 || lineage.RawArtifactOrdinal > maxRawArtifactCountV1 ||
		!validSourceRowSHA256V1(lineage.RawArtifactEntryDigest) || !validSourceRowSHA256V1(lineage.RawArtifactEntrySHA256) ||
		lineage.RawArtifactEntryByteLength == 0 || lineage.RawArtifactEntryByteLength > maxRawArtifactEntryBytesV1 ||
		!validSourceRowSHA256V1(lineage.RawSourceLocatorDigest) ||
		!validSourceRowSHA256V1(lineage.RawSourceLocatorSHA256) || lineage.RawSourceLocatorByteLength == 0 ||
		lineage.RawSourceLocatorByteLength > maxRawArtifactSourceLocatorBytesV1 ||
		!validSourceRowSHA256V1(lineage.SourceArtifactSHA256) ||
		lineage.SourceArtifactByteLength == 0 || lineage.SourceArtifactByteLength > maxRawArtifactContentBytesV1 ||
		!validSourceRowSHA256V1(lineage.ParsedGenerationReceiptDigest) ||
		!validSourceRowSHA256V1(lineage.ParsedGenerationReceiptSHA256) || lineage.ParsedGenerationReceiptByteLength == 0 ||
		lineage.ParsedGenerationReceiptByteLength > maxParsedGenerationReceiptBytesV1 ||
		!validSourceRowSHA256V1(lineage.ParsedPageDigest) ||
		!validSourceRowSHA256V1(lineage.ParsedPageSHA256) || lineage.ParsedPageByteLength == 0 ||
		lineage.ParsedPageByteLength > policy.MaxPageBytes || lineage.ParsedRowOrdinal >= policy.MaxRowsPerPage ||
		lineage.ParserID != policy.ParserID || lineage.ParserVersion != policy.ParserVersion ||
		!validSourceRowSHA256V1(lineage.MappingDigest) || !validSourceRowSHA256V1(lineage.ProjectionSchemaDigest) ||
		!validSourceRowSHA256V1(lineage.ReaderModeDigest) || !validSourceRowSHA256V1(lineage.CanonicalRowSHA256) ||
		!validSourceRowSHA256V1(lineage.LineageDigest) || lineage.LineageDigest != sourceRowLineageDigestV1(lineage) {
		return errors.New("source row lineage is invalid")
	}
	expectedRecordID := deriveSourceRowRecordIDFromDigestsV1(
		lineage.PolicyDigest, lineage.BindingKeyDigest, lineage.SourceArtifactSHA256, lineage.Locator,
	)
	if lineage.SourceRecordID != expectedRecordID {
		return errors.New("source row lineage record id is detached from its source occurrence")
	}
	return nil
}

func ValidateSourceRowLineageForBindingV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	lineage SourceRowLineageV1,
) error {
	if err := ValidateSourceRowLineageV1(lineage); err != nil {
		return err
	}
	if ValidateSourceRowProducerPolicyV1(policy) != nil || lineage.PolicyID != policy.PolicyID ||
		domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil || lineage.BindingKeyDigest != binding.BindingKeyDigest {
		return errors.New("source row lineage binding mismatch")
	}
	return nil
}

// ValidateSourceRowRecordAgainstLineageV1 is an exact structural comparison.
// It does not prove CAS membership, current snapshot selection, or factual
// authority; those checks remain app-layer callback obligations.
func ValidateSourceRowRecordAgainstLineageV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	record SourceRowRecordV1,
	lineage SourceRowLineageV1,
) error {
	if err := ValidateSourceRowRecordForBindingV1(policy, binding, record); err != nil {
		return err
	}
	if err := ValidateSourceRowLineageForBindingV1(policy, binding, lineage); err != nil {
		return err
	}
	lineageBody, err := SourceRowLineageV1Bytes(lineage)
	if err != nil {
		return err
	}
	if record.PolicyID != lineage.PolicyID || record.PolicyDigest != lineage.PolicyDigest ||
		record.BindingKeyDigest != lineage.BindingKeyDigest || record.SourceRecordID != lineage.SourceRecordID ||
		record.Locator != lineage.Locator || record.SourceArtifactSHA256 != lineage.SourceArtifactSHA256 ||
		record.CanonicalRowSHA256 != lineage.CanonicalRowSHA256 || record.ParserID != lineage.ParserID ||
		record.ParserVersion != lineage.ParserVersion || record.LineageDigest != lineage.LineageDigest ||
		record.LineageSHA256 != domainsecurity.SHA256Hex(lineageBody) || record.LineageByteLength != uint64(len(lineageBody)) {
		return errors.New("source row record does not match its typed lineage")
	}
	return nil
}

// ValidateSourceRowLineageAgainstParsedGenerationV1 binds one lineage to an
// exact receipt, exact page-index member, exact parsed page, and accepted
// outcome. The caller must already have validated the complete receipt/index
// hierarchy once and must supply these objects from parent-addressed private
// storage; this helper never enumerates or selects current state.
func ValidateSourceRowLineageAgainstParsedGenerationV1(
	lineage SourceRowLineageV1,
	receipt ParsedGenerationReceiptV1,
	index ParsedPageIndexV1,
	page ParsedPageV1,
) error {
	if ValidateSourceRowLineageV1(lineage) != nil || ValidateParsedGenerationReceiptV1(receipt) != nil ||
		ValidateParsedPageIndexV1(index) != nil || ValidateParsedPageV1(page) != nil ||
		page.Identity != receipt.Identity || index.Identity != receipt.Identity {
		return errors.New("source row parsed generation hierarchy is invalid")
	}
	receiptBody, err := ParsedGenerationReceiptV1Bytes(receipt)
	if err != nil || lineage.ParsedGenerationReceiptDigest != receipt.ReceiptDigest ||
		lineage.ParsedGenerationReceiptSHA256 != domainsecurity.SHA256Hex(receiptBody) ||
		lineage.ParsedGenerationReceiptByteLength != uint64(len(receiptBody)) {
		return errors.New("source row lineage does not bind the exact parsed generation receipt")
	}
	if page.PageNumber == 0 || page.PageNumber > maxParsedGenerationPageCountV1 {
		return errors.New("source row parsed page number is invalid")
	}
	indexNumber := (page.PageNumber-1)/maxParsedPagesPerIndexV1 + 1
	if index.IndexPageNumber != indexNumber || indexNumber > uint64(len(receipt.IndexPageDescriptors)) ||
		ValidateParsedPageIndexAgainstDescriptorV1(receipt.IndexPageDescriptors[indexNumber-1], index) != nil {
		return errors.New("source row parsed page index is not an exact receipt member")
	}
	pageOffset := (page.PageNumber - 1) % maxParsedPagesPerIndexV1
	if pageOffset >= uint64(len(index.PageDescriptors)) ||
		ValidateParsedPageAgainstDescriptorV1(index.PageDescriptors[pageOffset], page) != nil {
		return errors.New("source row parsed page is not an exact index member")
	}
	pageBody, err := ParsedPageV1Bytes(page)
	if err != nil || lineage.ParsedPageDigest != page.PageDigest ||
		lineage.ParsedPageSHA256 != domainsecurity.SHA256Hex(pageBody) ||
		lineage.ParsedPageByteLength != uint64(len(pageBody)) ||
		uint64(lineage.ParsedRowOrdinal) >= uint64(len(page.Outcomes)) {
		return errors.New("source row lineage does not bind the exact parsed page position")
	}
	outcome := page.Outcomes[lineage.ParsedRowOrdinal]
	identity := receipt.Identity
	if outcome.Disposition != ParsedOutcomeAcceptedV1 ||
		lineage.PolicyID != identity.PolicyID || lineage.PolicyDigest != identity.PolicyDigest ||
		lineage.BindingKeyDigest != identity.BindingKeyDigest ||
		lineage.RawArtifactManifestDigest != identity.RawArtifactManifestDigest ||
		lineage.RawArtifactManifestSHA256 != identity.RawArtifactManifestSHA256 ||
		lineage.RawArtifactManifestByteLength != identity.RawArtifactManifestLength ||
		lineage.ParserID != identity.ParserID || lineage.ParserVersion != identity.ParserVersion ||
		lineage.MappingDigest != identity.MappingDigest ||
		lineage.ProjectionSchemaDigest != identity.ProjectionSchemaDigest ||
		lineage.ReaderModeDigest != identity.ReaderModeDigest ||
		lineage.SourceRecordID != outcome.SourceRecordID || lineage.Locator != outcome.Locator ||
		lineage.RawArtifactOrdinal != outcome.RawArtifactOrdinal ||
		lineage.RawArtifactEntryDigest != outcome.RawArtifactEntryDigest ||
		lineage.SourceArtifactSHA256 != outcome.SourceArtifactSHA256 ||
		lineage.CanonicalRowSHA256 != outcome.CanonicalRowSHA256 {
		return errors.New("source row lineage does not match its accepted parsed outcome")
	}
	return nil
}

// ValidateSourceRowLineageAgainstRawArtifactV1 checks exact canonical objects
// supplied by a private-CAS callback. It does not select current state or read
// storage by itself.
func ValidateSourceRowLineageAgainstRawArtifactV1(
	lineage SourceRowLineageV1,
	intent RawArtifactAcquisitionIntentV1,
	manifest RawArtifactManifestV1,
	manifestPages []RawArtifactManifestPageV1,
	entry RawArtifactEntryV1,
	sourceLocator RawArtifactSourceLocatorV1,
	contentRoot RawArtifactContentRootV1,
) error {
	if ValidateSourceRowLineageV1(lineage) != nil || ValidateRawArtifactManifestHierarchyV1(intent, manifest, manifestPages) != nil ||
		entry.ArtifactOrdinal == 0 {
		return errors.New("source row lineage raw artifact material is invalid")
	}
	pageIndex := (entry.ArtifactOrdinal - 1) / maxRawArtifactsPerManifestPageV1
	if pageIndex >= uint64(len(manifestPages)) {
		return errors.New("source row lineage raw artifact page is absent")
	}
	manifestPage := manifestPages[pageIndex]
	if ValidateRawArtifactManifestEntryMembershipV1(manifest, manifestPage, entry) != nil ||
		ValidateRawArtifactEntryAgainstSourceLocatorV1(intent, entry, sourceLocator) != nil ||
		ValidateRawArtifactEntryWithContentRootV1(entry, contentRoot) != nil {
		return errors.New("source row lineage raw artifact material is invalid")
	}
	manifestBody, manifestErr := RawArtifactManifestV1Bytes(manifest)
	pageBody, pageErr := RawArtifactManifestPageV1Bytes(manifestPage)
	entryBody, entryErr := RawArtifactEntryV1Bytes(entry)
	locatorBody, locatorErr := RawArtifactSourceLocatorV1Bytes(sourceLocator)
	if manifestErr != nil || pageErr != nil || entryErr != nil || locatorErr != nil ||
		lineage.BindingKeyDigest != manifest.BindingKeyDigest || lineage.BindingKeyDigest != entry.BindingKeyDigest ||
		lineage.RawArtifactManifestDigest != manifest.ManifestDigest ||
		lineage.RawArtifactManifestSHA256 != domainsecurity.SHA256Hex(manifestBody) ||
		lineage.RawArtifactManifestByteLength != uint64(len(manifestBody)) ||
		lineage.RawArtifactManifestPageDigest != manifestPage.PageDigest ||
		lineage.RawArtifactManifestPageSHA256 != domainsecurity.SHA256Hex(pageBody) ||
		lineage.RawArtifactManifestPageByteLength != uint64(len(pageBody)) ||
		lineage.RawArtifactOrdinal != entry.ArtifactOrdinal || lineage.RawArtifactEntryDigest != entry.EntryDigest ||
		lineage.RawArtifactEntrySHA256 != domainsecurity.SHA256Hex(entryBody) ||
		lineage.RawArtifactEntryByteLength != uint64(len(entryBody)) ||
		lineage.RawSourceLocatorDigest != entry.SourceLocatorDigest ||
		lineage.RawSourceLocatorSHA256 != domainsecurity.SHA256Hex(locatorBody) ||
		lineage.RawSourceLocatorByteLength != uint64(len(locatorBody)) ||
		lineage.Locator.SourceFileIDDigest != entry.SourceFileIDDigest ||
		lineage.SourceArtifactSHA256 != entry.FullSHA256 || lineage.SourceArtifactByteLength != entry.ArtifactByteLength {
		return errors.New("source row lineage does not match its exact raw artifact hierarchy")
	}
	return nil
}

// ValidateSourceRowLineageAgainstSnapshotManifestV2 compares typed lineage
// references with a structural DSV2 manifest. Current witnessed selection and
// exact generation/page readback remain mandatory app checks.
func ValidateSourceRowLineageAgainstSnapshotManifestV2(
	lineage SourceRowLineageV1,
	manifest domainsecurity.DatasetSnapshotManifestV2,
) error {
	if ValidateSourceRowLineageV1(lineage) != nil || domainsecurity.ValidateDatasetSnapshotManifestV2(manifest) != nil {
		return errors.New("source row lineage snapshot manifest material is invalid")
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(lineage.PolicyID)
	if !ok || ValidateSourceRowProducerPolicyV1(policy) != nil {
		return errors.New("source row lineage producer policy is unavailable")
	}
	if lineage.BindingKeyDigest != manifest.Binding.BindingKeyDigest || lineage.PolicyID != manifest.ProducerPolicyID ||
		lineage.PolicyDigest != manifest.ProducerPolicyDigest || manifest.SourceType != policy.SourceType ||
		manifest.ProducerComponentID != policy.ProducerComponentID ||
		manifest.ProducerComponentVersion != policy.ProducerComponentVersion ||
		manifest.ProducerOperation != policy.Operation || manifest.ProducerOperationSchemaHash != policy.OperationSchemaHash ||
		lineage.ParserID != manifest.ParserID ||
		lineage.ParserVersion != manifest.ParserVersion || lineage.RawArtifactManifestDigest != manifest.RawArtifactManifestDigest ||
		lineage.RawArtifactManifestSHA256 != manifest.RawArtifactManifestSHA256 ||
		lineage.RawArtifactManifestByteLength != manifest.RawArtifactManifestByteLength ||
		lineage.ParsedGenerationReceiptDigest != manifest.ParsedGenerationReceiptDigest ||
		lineage.ParsedGenerationReceiptSHA256 != manifest.ParsedGenerationReceiptSHA256 ||
		lineage.ParsedGenerationReceiptByteLength != manifest.ParsedGenerationReceiptByteLength {
		return errors.New("source row lineage does not match the dataset snapshot manifest")
	}
	return nil
}

func ParseSourceRowLineageV1(raw []byte) (SourceRowLineageV1, error) {
	var lineage SourceRowLineageV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &lineage, maxSourceRowLineageBytesV1, 2_048, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return SourceRowLineageV1{}, err
	}
	return lineage, ValidateSourceRowLineageV1(lineage)
}

func SourceRowLineageV1Bytes(lineage SourceRowLineageV1) ([]byte, error) {
	if err := ValidateSourceRowLineageV1(lineage); err != nil {
		return nil, err
	}
	body, err := json.Marshal(lineage)
	if err != nil || len(body) > maxSourceRowLineageBytesV1 {
		return nil, errors.New("source row lineage exceeds its canonical byte limit")
	}
	return body, nil
}

func sourceRowLineageDigestV1(lineage SourceRowLineageV1) string {
	lineage.LineageDigest = ""
	body, _ := json.Marshal(lineage)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLineageDigestDomainV1...), body...))
}
