package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SourceRowLocatorSchemaVersionV1 = 1
	SourceRowLocatorPurposeV1       = "analytix.source-row-locator/v1"
	SourceRowRecordSchemaVersionV1  = 1
	SourceRowRecordPurposeV1        = "analytix.source-row-record/v1"

	SourceRowLedgerPageEntrySchemaVersionV1 = 1
	SourceRowLedgerPageEntryPurposeV1       = "analytix.source-row-ledger-page-entry/v1"
	SourceRowLedgerPageSchemaVersionV1      = 1
	SourceRowLedgerPagePurposeV1            = "analytix.source-row-ledger-page/v1"

	SourceRowLedgerPageDescriptorSchemaVersionV1 = 1
	SourceRowLedgerPageDescriptorPurposeV1       = "analytix.source-row-ledger-page-descriptor/v1"
	SourceRowLedgerRootSchemaVersionV1           = 1
	SourceRowLedgerRootPurposeV1                 = "analytix.source-row-ledger-root/v1"

	SourceRowWitnessSchemaVersionV1 = 1
	SourceRowWitnessPurposeV1       = "analytix.source-row-witness/v1"
	SourceRowWitnessAuthorityNoneV1 = "structural_comparison_only"

	SourceRowRecordIDPrefixV1 = "srow1_"

	maxSourceRowJSONIntegerV1              = uint64(9_007_199_254_740_991)
	maxSourceRowLedgerRootBytesV1          = 16 * 1024 * 1024
	maxSourceRowLedgerAggregatePageBytesV1 = uint64(1024 * 1024 * 1024 * 1024)
	maxSourceRowBindingTextBytesV1         = 32 * 1024
)

var (
	sourceRowLocatorDigestDomainV1          = []byte("analytix.source-row-locator/digest/v1\x00")
	sourceRowSourceFileIDDigestDomainV1     = []byte("analytix.source-row-source-file-id/digest/v1\x00")
	sourceRowRecordIDDomainV1               = []byte("analytix.source-row-record/id/v1\x00")
	sourceRowRecordDigestDomainV1           = []byte("analytix.source-row-record/digest/v1\x00")
	sourceRowRecordIdentityDigestDomainV1   = []byte("analytix.source-row-record/identity/v1\x00")
	sourceRowLedgerPageEntryDigestDomainV1  = []byte("analytix.source-row-ledger-page-entry/digest/v1\x00")
	sourceRowLedgerPageDigestDomainV1       = []byte("analytix.source-row-ledger-page/digest/v1\x00")
	sourceRowLedgerDescriptorDigestDomainV1 = []byte("analytix.source-row-ledger-page-descriptor/digest/v1\x00")
	sourceRowLedgerRootDigestDomainV1       = []byte("analytix.source-row-ledger-root/digest/v1\x00")
	sourceRowWitnessDigestDomainV1          = []byte("analytix.source-row-witness/digest/v1\x00")
	sourceRowWitnessIdentityDigestDomainV1  = []byte("analytix.source-row-witness/identity/v1\x00")
	sourceRowWitnessSetDigestDomainV1       = []byte("analytix.source-row-witness-set/v1\x00")
)

// SourceRowLocatorV1 is stable source identity. Pagination and JSON paths are
// deliberately absent, so changing page size cannot mint another record id.
type SourceRowLocatorV1 struct {
	SchemaVersion      int    `json:"schemaVersion"`
	Purpose            string `json:"purpose"`
	RelationID         string `json:"relationId"`
	SourceFileIDDigest string `json:"sourceFileIdDigest"`
	SourceRowNumber    uint64 `json:"sourceRowNumber"`
	LocatorDigest      string `json:"locatorDigest"`
}

type SourceRowLocatorInputV1 struct {
	SourceFileID    string
	SourceRowNumber uint64
}

// SourceRowRecordV1 stores restricted, pseudonymous private-CAS metadata. Its
// stable hashes can still enable cross-record correlation or dictionary
// confirmation and therefore must never enter ordinary UI/SSE/history/logs,
// reports, or exports. The legacy source row_hash is not CanonicalRowSHA256.
type SourceRowRecordV1 struct {
	SchemaVersion        int                `json:"schemaVersion"`
	Purpose              string             `json:"purpose"`
	PolicyID             string             `json:"policyId"`
	PolicyDigest         string             `json:"policyDigest"`
	BindingKeyDigest     string             `json:"bindingKeyDigest"`
	SourceRecordID       string             `json:"sourceRecordId"`
	Locator              SourceRowLocatorV1 `json:"locator"`
	SourceArtifactSHA256 string             `json:"sourceArtifactSha256"`
	CanonicalRowSHA256   string             `json:"canonicalRowSha256"`
	ParserID             string             `json:"parserId"`
	ParserVersion        string             `json:"parserVersion"`
	LineageDigest        string             `json:"lineageDigest"`
	LineageSHA256        string             `json:"lineageSha256"`
	LineageByteLength    uint64             `json:"lineageByteLength"`
	RecordDigest         string             `json:"recordDigest"`
}

type sourceRowRecordInputV1 struct {
	Policy               SourceRowProducerPolicyV1
	Binding              domainsecurity.DatasetSnapshotBindingKeyV1
	SourceArtifactSHA256 string
	CanonicalRowSHA256   string
	LineageDigest        string
	LineageSHA256        string
	LineageByteLength    uint64
	Locator              SourceRowLocatorInputV1
}

// SourceRowLedgerPageEntryV1 binds one stable record to a deterministic page
// position and the only registered output paths. The position is retrieval
// metadata and does not participate in SourceRecordID.
type SourceRowLedgerPageEntryV1 struct {
	SchemaVersion       int               `json:"schemaVersion"`
	Purpose             string            `json:"purpose"`
	PageOrdinal         uint32            `json:"pageOrdinal"`
	RecordPath          string            `json:"recordPath"`
	RecordIDPath        string            `json:"recordIdPath"`
	SourceFileIDPath    string            `json:"sourceFileIdPath"`
	SourceRowNumberPath string            `json:"sourceRowNumberPath"`
	Record              SourceRowRecordV1 `json:"record"`
	EntryDigest         string            `json:"entryDigest"`
}

type SourceRowLedgerPageV1 struct {
	SchemaVersion    int                          `json:"schemaVersion"`
	Purpose          string                       `json:"purpose"`
	PolicyID         string                       `json:"policyId"`
	PolicyDigest     string                       `json:"policyDigest"`
	BindingKeyDigest string                       `json:"bindingKeyDigest"`
	ParserID         string                       `json:"parserId"`
	ParserVersion    string                       `json:"parserVersion"`
	PageNumber       uint64                       `json:"pageNumber"`
	Entries          []SourceRowLedgerPageEntryV1 `json:"entries"`
	RecordCount      uint32                       `json:"recordCount"`
	PageDigest       string                       `json:"pageDigest"`
}

type SourceRowLedgerPageDescriptorV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	PageNumber              uint64 `json:"pageNumber"`
	PageDigest              string `json:"pageDigest"`
	PageSHA256              string `json:"pageSha256"`
	PageByteLength          uint64 `json:"pageByteLength"`
	RecordCount             uint32 `json:"recordCount"`
	FirstSourceFileIDDigest string `json:"firstSourceFileIdDigest"`
	FirstSourceRowNumber    uint64 `json:"firstSourceRowNumber"`
	FirstLocatorDigest      string `json:"firstLocatorDigest"`
	LastSourceFileIDDigest  string `json:"lastSourceFileIdDigest"`
	LastSourceRowNumber     uint64 `json:"lastSourceRowNumber"`
	LastLocatorDigest       string `json:"lastLocatorDigest"`
	FirstSourceRecordID     string `json:"firstSourceRecordId"`
	LastSourceRecordID      string `json:"lastSourceRecordId"`
	DescriptorDigest        string `json:"descriptorDigest"`
}

// SourceRowLedgerRootV1 contains a case-binding key but intentionally carries
// neither datasetSnapshotId, sourceManifestHash, contextDigest, nor accepted
// time. A DSV2 manifest references this root after construction, avoiding a
// snapshot-id/manifest/row-root hash cycle.
type SourceRowLedgerRootV1 struct {
	SchemaVersion        int                                        `json:"schemaVersion"`
	Purpose              string                                     `json:"purpose"`
	Binding              domainsecurity.DatasetSnapshotBindingKeyV1 `json:"binding"`
	PolicyID             string                                     `json:"policyId"`
	PolicyDigest         string                                     `json:"policyDigest"`
	ParserID             string                                     `json:"parserId"`
	ParserVersion        string                                     `json:"parserVersion"`
	IndexPageDescriptors []SourceRowLedgerIndexPageDescriptorV1     `json:"indexPageDescriptors"`
	IndexPageCount       uint64                                     `json:"indexPageCount"`
	PageCount            uint64                                     `json:"pageCount"`
	RecordCount          uint64                                     `json:"recordCount"`
	RootDigest           string                                     `json:"rootDigest"`
}

// SourceRowWitnessV1 is sealed comparison material, never authority by
// itself. Factual use additionally requires a fresh registry-backed DSV2
// authority record and exact private-CAS row comparison in one app callback.
type SourceRowWitnessV1 struct {
	SchemaVersion                 int    `json:"schemaVersion"`
	Purpose                       string `json:"purpose"`
	AuthorityClass                string `json:"authorityClass"`
	FactAnswerAllowed             bool   `json:"factAnswerAllowed"`
	ThreadID                      string `json:"threadId"`
	TurnID                        string `json:"turnId"`
	CaseID                        string `json:"caseId"`
	CaseBindingHash               string `json:"caseBindingHash"`
	ContextEpoch                  uint64 `json:"contextEpoch"`
	ContextDigest                 string `json:"contextDigest"`
	DatasetSnapshotID             string `json:"datasetSnapshotId"`
	SourceManifestHash            string `json:"sourceManifestHash"`
	SnapshotAuthorityRecordDigest string `json:"snapshotAuthorityRecordDigest"`
	BindingKeyDigest              string `json:"bindingKeyDigest"`
	PolicyID                      string `json:"policyId"`
	PolicyDigest                  string `json:"policyDigest"`
	LedgerRootDigest              string `json:"ledgerRootDigest"`
	LedgerIndexPageDigest         string `json:"ledgerIndexPageDigest"`
	LedgerPageDigest              string `json:"ledgerPageDigest"`
	SourceRecordID                string `json:"sourceRecordId"`
	SourceArtifactSHA256          string `json:"sourceArtifactSha256"`
	CanonicalRowSHA256            string `json:"canonicalRowSha256"`
	LocatorDigest                 string `json:"locatorDigest"`
	ParserID                      string `json:"parserId"`
	ParserVersion                 string `json:"parserVersion"`
	LineageDigest                 string `json:"lineageDigest"`
	LineageSHA256                 string `json:"lineageSha256"`
	LineageByteLength             uint64 `json:"lineageByteLength"`
	WitnessDigest                 string `json:"witnessDigest"`
}

type SourceRowWitnessInputV1 struct {
	Context                       domainsecurity.TurnSecurityContext
	SnapshotAuthorityRecordDigest string
	Root                          SourceRowLedgerRootV1
	IndexPage                     SourceRowLedgerIndexPageV1
	Page                          SourceRowLedgerPageV1
	Record                        SourceRowRecordV1
}

func NewSourceRowLocatorV1(policy SourceRowProducerPolicyV1, input SourceRowLocatorInputV1) (SourceRowLocatorV1, error) {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || !validSourceFileIDForPolicyV1(policy, input.SourceFileID) ||
		input.SourceRowNumber == 0 || input.SourceRowNumber > maxSourceRowJSONIntegerV1 {
		return SourceRowLocatorV1{}, errors.New("source row locator input is invalid")
	}
	locator := SourceRowLocatorV1{
		SchemaVersion:      SourceRowLocatorSchemaVersionV1,
		Purpose:            SourceRowLocatorPurposeV1,
		RelationID:         policy.Relation,
		SourceFileIDDigest: sourceRowSourceFileIDDigestV1(input.SourceFileID),
		SourceRowNumber:    input.SourceRowNumber,
	}
	locator.LocatorDigest = sourceRowLocatorDigestV1(locator)
	if err := ValidateSourceRowLocatorV1(policy, locator); err != nil {
		return SourceRowLocatorV1{}, err
	}
	return locator, nil
}

func ValidateSourceRowLocatorV1(policy SourceRowProducerPolicyV1, locator SourceRowLocatorV1) error {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || locator.SchemaVersion != SourceRowLocatorSchemaVersionV1 ||
		locator.Purpose != SourceRowLocatorPurposeV1 || locator.RelationID != policy.Relation ||
		!validSourceRowSHA256V1(locator.SourceFileIDDigest) || locator.SourceRowNumber == 0 ||
		locator.SourceRowNumber > maxSourceRowJSONIntegerV1 || !validSourceRowSHA256V1(locator.LocatorDigest) ||
		locator.LocatorDigest != sourceRowLocatorDigestV1(locator) {
		return errors.New("source row locator is invalid")
	}
	return nil
}

func DeriveSourceRowRecordIDV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	sourceArtifactSHA256 string,
	locator SourceRowLocatorV1,
) (string, error) {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil ||
		!validSourceRowSHA256V1(sourceArtifactSHA256) || ValidateSourceRowLocatorV1(policy, locator) != nil {
		return "", errors.New("source row record identity input is invalid")
	}
	return deriveSourceRowRecordIDFromDigestsV1(policy.PolicyDigest, binding.BindingKeyDigest, sourceArtifactSHA256, locator), nil
}

func deriveSourceRowRecordIDFromDigestsV1(policyDigest, bindingKeyDigest, sourceArtifactSHA256 string, locator SourceRowLocatorV1) string {
	identity := struct {
		SchemaVersion        int    `json:"schemaVersion"`
		BindingKeyDigest     string `json:"bindingKeyDigest"`
		PolicyDigest         string `json:"policyDigest"`
		SourceArtifactSHA256 string `json:"sourceArtifactSha256"`
		RelationID           string `json:"relationId"`
		SourceFileIDDigest   string `json:"sourceFileIdDigest"`
		SourceRowNumber      uint64 `json:"sourceRowNumber"`
	}{1, bindingKeyDigest, policyDigest, sourceArtifactSHA256, locator.RelationID, locator.SourceFileIDDigest, locator.SourceRowNumber}
	body, _ := json.Marshal(identity)
	digest := domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowRecordIDDomainV1...), body...))
	return SourceRowRecordIDPrefixV1 + digest
}

func newSourceRowRecordV1(input sourceRowRecordInputV1) (SourceRowRecordV1, error) {
	policy := input.Policy
	if ValidateSourceRowProducerPolicyV1(policy) != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(input.Binding) != nil ||
		!validSourceRowSHA256V1(input.SourceArtifactSHA256) || !validSourceRowSHA256V1(input.CanonicalRowSHA256) ||
		!validSourceRowSHA256V1(input.LineageDigest) || !validSourceRowSHA256V1(input.LineageSHA256) ||
		input.LineageByteLength == 0 || input.LineageByteLength > maxSourceRowLineageBytesV1 {
		return SourceRowRecordV1{}, errors.New("source row record input is invalid")
	}
	locator, err := NewSourceRowLocatorV1(policy, input.Locator)
	if err != nil {
		return SourceRowRecordV1{}, err
	}
	recordID, err := DeriveSourceRowRecordIDV1(policy, input.Binding, input.SourceArtifactSHA256, locator)
	if err != nil {
		return SourceRowRecordV1{}, err
	}
	record := SourceRowRecordV1{
		SchemaVersion:        SourceRowRecordSchemaVersionV1,
		Purpose:              SourceRowRecordPurposeV1,
		PolicyID:             policy.PolicyID,
		PolicyDigest:         policy.PolicyDigest,
		BindingKeyDigest:     input.Binding.BindingKeyDigest,
		SourceRecordID:       recordID,
		Locator:              locator,
		SourceArtifactSHA256: input.SourceArtifactSHA256,
		CanonicalRowSHA256:   input.CanonicalRowSHA256,
		ParserID:             policy.ParserID,
		ParserVersion:        policy.ParserVersion,
		LineageDigest:        input.LineageDigest,
		LineageSHA256:        input.LineageSHA256,
		LineageByteLength:    input.LineageByteLength,
	}
	record.RecordDigest = sourceRowRecordDigestV1(record)
	if err := ValidateSourceRowRecordForBindingV1(policy, input.Binding, record); err != nil {
		return SourceRowRecordV1{}, err
	}
	return record, nil
}

func ValidateSourceRowRecordV1(policy SourceRowProducerPolicyV1, record SourceRowRecordV1) error {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || record.SchemaVersion != SourceRowRecordSchemaVersionV1 ||
		record.Purpose != SourceRowRecordPurposeV1 || record.PolicyID != policy.PolicyID || record.PolicyDigest != policy.PolicyDigest ||
		!validSourceRowSHA256V1(record.BindingKeyDigest) || !validSourceRowRecordIDV1(record.SourceRecordID) ||
		ValidateSourceRowLocatorV1(policy, record.Locator) != nil || !validSourceRowSHA256V1(record.SourceArtifactSHA256) ||
		!validSourceRowSHA256V1(record.CanonicalRowSHA256) || record.ParserID != policy.ParserID ||
		record.ParserVersion != policy.ParserVersion || !validSourceRowSHA256V1(record.LineageDigest) ||
		!validSourceRowSHA256V1(record.LineageSHA256) || record.LineageByteLength == 0 ||
		record.LineageByteLength > maxSourceRowLineageBytesV1 ||
		!validSourceRowSHA256V1(record.RecordDigest) || record.RecordDigest != sourceRowRecordDigestV1(record) {
		return errors.New("source row record is invalid")
	}
	expectedID := deriveSourceRowRecordIDFromDigestsV1(record.PolicyDigest, record.BindingKeyDigest, record.SourceArtifactSHA256, record.Locator)
	if record.SourceRecordID != expectedID {
		return errors.New("source row record id is not bound to its source locator")
	}
	return nil
}

func ValidateSourceRowRecordForBindingV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	record SourceRowRecordV1,
) error {
	if err := ValidateSourceRowRecordV1(policy, record); err != nil {
		return err
	}
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil || record.BindingKeyDigest != binding.BindingKeyDigest {
		return errors.New("source row record binding mismatch")
	}
	return nil
}

func SourceRowRecordIdentityDigestV1(record SourceRowRecordV1) (string, error) {
	policy, ok := ResolveSourceRowProducerPolicyV1(record.PolicyID)
	if !ok || ValidateSourceRowRecordV1(policy, record) != nil {
		return "", errors.New("source row record identity is invalid")
	}
	identity := struct {
		BindingKeyDigest     string `json:"bindingKeyDigest"`
		PolicyDigest         string `json:"policyDigest"`
		SourceArtifactSHA256 string `json:"sourceArtifactSha256"`
		CanonicalRowSHA256   string `json:"canonicalRowSha256"`
		LocatorDigest        string `json:"locatorDigest"`
		ParserID             string `json:"parserId"`
		ParserVersion        string `json:"parserVersion"`
		LineageDigest        string `json:"lineageDigest"`
		LineageSHA256        string `json:"lineageSha256"`
		LineageByteLength    uint64 `json:"lineageByteLength"`
	}{record.BindingKeyDigest, record.PolicyDigest, record.SourceArtifactSHA256, record.CanonicalRowSHA256,
		record.Locator.LocatorDigest, record.ParserID, record.ParserVersion, record.LineageDigest,
		record.LineageSHA256, record.LineageByteLength}
	body, _ := json.Marshal(identity)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowRecordIdentityDigestDomainV1...), body...)), nil
}

func NewSourceRowLedgerPageV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	pageNumber uint64,
	records []SourceRowRecordV1,
) (SourceRowLedgerPageV1, error) {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil ||
		pageNumber == 0 || pageNumber > maxSourceRowJSONIntegerV1 || len(records) == 0 || len(records) > int(policy.MaxRowsPerPage) {
		return SourceRowLedgerPageV1{}, errors.New("source row ledger page input is invalid")
	}
	page := SourceRowLedgerPageV1{
		SchemaVersion:    SourceRowLedgerPageSchemaVersionV1,
		Purpose:          SourceRowLedgerPagePurposeV1,
		PolicyID:         policy.PolicyID,
		PolicyDigest:     policy.PolicyDigest,
		BindingKeyDigest: binding.BindingKeyDigest,
		ParserID:         policy.ParserID,
		ParserVersion:    policy.ParserVersion,
		PageNumber:       pageNumber,
		Entries:          make([]SourceRowLedgerPageEntryV1, 0, len(records)),
		RecordCount:      uint32(len(records)),
	}
	for index, record := range records {
		entry, err := newSourceRowLedgerPageEntryV1(policy, binding, uint32(index), record)
		if err != nil {
			return SourceRowLedgerPageV1{}, err
		}
		page.Entries = append(page.Entries, entry)
	}
	page.PageDigest = sourceRowLedgerPageDigestV1(page)
	if err := ValidateSourceRowLedgerPageForBindingV1(policy, binding, page); err != nil {
		return SourceRowLedgerPageV1{}, err
	}
	return page, nil
}

func newSourceRowLedgerPageEntryV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	ordinal uint32,
	record SourceRowRecordV1,
) (SourceRowLedgerPageEntryV1, error) {
	if ordinal >= policy.MaxRowsPerPage || ValidateSourceRowRecordForBindingV1(policy, binding, record) != nil {
		return SourceRowLedgerPageEntryV1{}, errors.New("source row ledger page entry input is invalid")
	}
	recordPath, recordErr := expandSourceRowPathTemplateV1(policy.RecordPathTemplate, ordinal)
	recordIDPath, recordIDErr := expandSourceRowPathTemplateV1(policy.RecordIDPathTemplate, ordinal)
	fileIDPath, fileIDErr := expandSourceRowPathTemplateV1(policy.SourceFileIDPathTemplate, ordinal)
	rowNumberPath, rowNumberErr := expandSourceRowPathTemplateV1(policy.SourceRowNumberPathTemplate, ordinal)
	if errors.Join(recordErr, recordIDErr, fileIDErr, rowNumberErr) != nil {
		return SourceRowLedgerPageEntryV1{}, errors.New("source row ledger page entry paths are invalid")
	}
	entry := SourceRowLedgerPageEntryV1{
		SchemaVersion:       SourceRowLedgerPageEntrySchemaVersionV1,
		Purpose:             SourceRowLedgerPageEntryPurposeV1,
		PageOrdinal:         ordinal,
		RecordPath:          recordPath,
		RecordIDPath:        recordIDPath,
		SourceFileIDPath:    fileIDPath,
		SourceRowNumberPath: rowNumberPath,
		Record:              record,
	}
	entry.EntryDigest = sourceRowLedgerPageEntryDigestV1(entry)
	return entry, ValidateSourceRowLedgerPageEntryV1(policy, entry)
}

func ValidateSourceRowLedgerPageEntryV1(policy SourceRowProducerPolicyV1, entry SourceRowLedgerPageEntryV1) error {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || entry.SchemaVersion != SourceRowLedgerPageEntrySchemaVersionV1 ||
		entry.Purpose != SourceRowLedgerPageEntryPurposeV1 || entry.PageOrdinal >= policy.MaxRowsPerPage ||
		ValidateSourceRowRecordV1(policy, entry.Record) != nil || !validSourceRowSHA256V1(entry.EntryDigest) ||
		entry.EntryDigest != sourceRowLedgerPageEntryDigestV1(entry) {
		return errors.New("source row ledger page entry is invalid")
	}
	recordPath, _ := expandSourceRowPathTemplateV1(policy.RecordPathTemplate, entry.PageOrdinal)
	recordIDPath, _ := expandSourceRowPathTemplateV1(policy.RecordIDPathTemplate, entry.PageOrdinal)
	fileIDPath, _ := expandSourceRowPathTemplateV1(policy.SourceFileIDPathTemplate, entry.PageOrdinal)
	rowNumberPath, _ := expandSourceRowPathTemplateV1(policy.SourceRowNumberPathTemplate, entry.PageOrdinal)
	if entry.RecordPath != recordPath || entry.RecordIDPath != recordIDPath || entry.SourceFileIDPath != fileIDPath ||
		entry.SourceRowNumberPath != rowNumberPath {
		return errors.New("source row ledger page entry paths do not match the registered policy")
	}
	return nil
}

func ValidateSourceRowLedgerPageV1(policy SourceRowProducerPolicyV1, page SourceRowLedgerPageV1) error {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || page.SchemaVersion != SourceRowLedgerPageSchemaVersionV1 ||
		page.Purpose != SourceRowLedgerPagePurposeV1 || page.PolicyID != policy.PolicyID || page.PolicyDigest != policy.PolicyDigest ||
		!validSourceRowSHA256V1(page.BindingKeyDigest) || page.ParserID != policy.ParserID || page.ParserVersion != policy.ParserVersion ||
		page.PageNumber == 0 || page.PageNumber > maxSourceRowJSONIntegerV1 || len(page.Entries) == 0 ||
		len(page.Entries) > int(policy.MaxRowsPerPage) || page.RecordCount != uint32(len(page.Entries)) ||
		!validSourceRowSHA256V1(page.PageDigest) || page.PageDigest != sourceRowLedgerPageDigestV1(page) {
		return errors.New("source row ledger page is invalid")
	}
	seenIDs := make(map[string]struct{}, len(page.Entries))
	seenLocators := make(map[string]struct{}, len(page.Entries))
	for index, entry := range page.Entries {
		if ValidateSourceRowLedgerPageEntryV1(policy, entry) != nil || entry.PageOrdinal != uint32(index) ||
			entry.Record.BindingKeyDigest != page.BindingKeyDigest {
			return errors.New("source row ledger page entries are not canonical")
		}
		if index > 0 && !sourceRowLocatorStrictlyBeforeV1(page.Entries[index-1].Record.Locator, entry.Record.Locator) {
			return errors.New("source row ledger page source locators are not strictly ordered")
		}
		if _, exists := seenIDs[entry.Record.SourceRecordID]; exists {
			return errors.New("source row ledger page contains duplicate record ids")
		}
		if _, exists := seenLocators[entry.Record.Locator.LocatorDigest]; exists {
			return errors.New("source row ledger page contains duplicate source locators")
		}
		seenIDs[entry.Record.SourceRecordID] = struct{}{}
		seenLocators[entry.Record.Locator.LocatorDigest] = struct{}{}
	}
	body, err := json.Marshal(page)
	if err != nil || uint64(len(body)) > policy.MaxPageBytes {
		return errors.New("source row ledger page exceeds its policy limit")
	}
	return nil
}

func ValidateSourceRowLedgerPageForBindingV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	page SourceRowLedgerPageV1,
) error {
	if err := ValidateSourceRowLedgerPageV1(policy, page); err != nil {
		return err
	}
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil || page.BindingKeyDigest != binding.BindingKeyDigest {
		return errors.New("source row ledger page binding mismatch")
	}
	return nil
}

func NewSourceRowLedgerPageDescriptorV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	page SourceRowLedgerPageV1,
) (SourceRowLedgerPageDescriptorV1, error) {
	if ValidateSourceRowLedgerPageForBindingV1(policy, binding, page) != nil {
		return SourceRowLedgerPageDescriptorV1{}, errors.New("source row ledger page descriptor input is invalid")
	}
	pageBody, _ := json.Marshal(page)
	first := page.Entries[0].Record
	last := page.Entries[len(page.Entries)-1].Record
	descriptor := SourceRowLedgerPageDescriptorV1{
		SchemaVersion:           SourceRowLedgerPageDescriptorSchemaVersionV1,
		Purpose:                 SourceRowLedgerPageDescriptorPurposeV1,
		PageNumber:              page.PageNumber,
		PageDigest:              page.PageDigest,
		PageSHA256:              domainsecurity.SHA256Hex(pageBody),
		PageByteLength:          uint64(len(pageBody)),
		RecordCount:             page.RecordCount,
		FirstSourceFileIDDigest: first.Locator.SourceFileIDDigest,
		FirstSourceRowNumber:    first.Locator.SourceRowNumber,
		FirstLocatorDigest:      first.Locator.LocatorDigest,
		LastSourceFileIDDigest:  last.Locator.SourceFileIDDigest,
		LastSourceRowNumber:     last.Locator.SourceRowNumber,
		LastLocatorDigest:       last.Locator.LocatorDigest,
		FirstSourceRecordID:     first.SourceRecordID,
		LastSourceRecordID:      last.SourceRecordID,
	}
	descriptor.DescriptorDigest = sourceRowLedgerPageDescriptorDigestV1(descriptor)
	if err := ValidateSourceRowLedgerPageDescriptorForPolicyV1(policy, descriptor); err != nil {
		return SourceRowLedgerPageDescriptorV1{}, err
	}
	return descriptor, nil
}

func ValidateSourceRowLedgerPageDescriptorV1(descriptor SourceRowLedgerPageDescriptorV1) error {
	if descriptor.SchemaVersion != SourceRowLedgerPageDescriptorSchemaVersionV1 ||
		descriptor.Purpose != SourceRowLedgerPageDescriptorPurposeV1 || descriptor.PageNumber == 0 ||
		descriptor.PageNumber > maxSourceRowJSONIntegerV1 || descriptor.RecordCount == 0 || descriptor.PageByteLength == 0 ||
		descriptor.PageByteLength > 1024*1024 || !validSourceRowSHA256V1(descriptor.PageDigest) ||
		!validSourceRowSHA256V1(descriptor.PageSHA256) || !validSourceRowSHA256V1(descriptor.FirstSourceFileIDDigest) ||
		descriptor.FirstSourceRowNumber == 0 || descriptor.FirstSourceRowNumber > maxSourceRowJSONIntegerV1 ||
		!validSourceRowSHA256V1(descriptor.FirstLocatorDigest) || !validSourceRowSHA256V1(descriptor.LastSourceFileIDDigest) ||
		descriptor.LastSourceRowNumber == 0 || descriptor.LastSourceRowNumber > maxSourceRowJSONIntegerV1 ||
		!validSourceRowSHA256V1(descriptor.LastLocatorDigest) || !validSourceRowRecordIDV1(descriptor.FirstSourceRecordID) ||
		!validSourceRowRecordIDV1(descriptor.LastSourceRecordID) || !validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != sourceRowLedgerPageDescriptorDigestV1(descriptor) {
		return errors.New("source row ledger page descriptor is invalid")
	}
	first := sourceRowLocatorFromDescriptorBoundaryV1(descriptor.FirstSourceFileIDDigest, descriptor.FirstSourceRowNumber)
	last := sourceRowLocatorFromDescriptorBoundaryV1(descriptor.LastSourceFileIDDigest, descriptor.LastSourceRowNumber)
	if descriptor.RecordCount == 1 {
		if first.SourceFileIDDigest != last.SourceFileIDDigest || first.SourceRowNumber != last.SourceRowNumber ||
			descriptor.FirstLocatorDigest != descriptor.LastLocatorDigest || descriptor.FirstSourceRecordID != descriptor.LastSourceRecordID {
			return errors.New("single-record source row ledger descriptor has inconsistent boundaries")
		}
	} else if !sourceRowLocatorStrictlyBeforeV1(first, last) {
		return errors.New("source row ledger descriptor boundaries are not strictly ordered")
	}
	return nil
}

// ValidateSourceRowLedgerPageDescriptorForPolicyV1 prevents a descriptor-only
// root from claiming pages larger than the closed producer contract. The
// generic structural validator is insufficient because policy limits are part
// of evidence authority, not advisory transport metadata.
func ValidateSourceRowLedgerPageDescriptorForPolicyV1(
	policy SourceRowProducerPolicyV1,
	descriptor SourceRowLedgerPageDescriptorV1,
) error {
	first := sourceRowLocatorForPolicyBoundaryV1(policy, descriptor.FirstSourceFileIDDigest, descriptor.FirstSourceRowNumber)
	last := sourceRowLocatorForPolicyBoundaryV1(policy, descriptor.LastSourceFileIDDigest, descriptor.LastSourceRowNumber)
	if ValidateSourceRowProducerPolicyV1(policy) != nil || ValidateSourceRowLedgerPageDescriptorV1(descriptor) != nil ||
		descriptor.RecordCount > policy.MaxRowsPerPage || descriptor.PageByteLength > policy.MaxPageBytes ||
		first.LocatorDigest != descriptor.FirstLocatorDigest || last.LocatorDigest != descriptor.LastLocatorDigest {
		return errors.New("source row ledger page descriptor exceeds its registered policy")
	}
	return nil
}

// newSourceRowLedgerRootV1FromPages is package-private because production
// admission must visit bounded pages through private CAS with cancellation;
// it must not retain a caller-owned full dataset in memory.
func newSourceRowLedgerRootV1FromPages(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	pages []SourceRowLedgerPageV1,
) (SourceRowLedgerRootV1, error) {
	_, root, err := newSourceRowLedgerHierarchyV1FromPages(policy, binding, pages)
	return root, err
}

func newSourceRowLedgerHierarchyV1FromPagesWithoutFinalValidation(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	pages []SourceRowLedgerPageV1,
) ([]SourceRowLedgerIndexPageV1, SourceRowLedgerRootV1, error) {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil ||
		len(pages) == 0 || len(pages) > maxSourceRowLedgerDataPagesPerIndexPageV1*maxSourceRowLedgerRootIndexPageCountV1 {
		return nil, SourceRowLedgerRootV1{}, errors.New("source row ledger hierarchy input is invalid")
	}
	indexPages := make([]SourceRowLedgerIndexPageV1, 0, (len(pages)+maxSourceRowLedgerDataPagesPerIndexPageV1-1)/maxSourceRowLedgerDataPagesPerIndexPageV1)
	indexDescriptors := make([]SourceRowLedgerIndexPageDescriptorV1, 0, cap(indexPages))
	for offset := 0; offset < len(pages); offset += maxSourceRowLedgerDataPagesPerIndexPageV1 {
		end := offset + maxSourceRowLedgerDataPagesPerIndexPageV1
		if end > len(pages) {
			end = len(pages)
		}
		dataDescriptors := make([]SourceRowLedgerPageDescriptorV1, 0, end-offset)
		for _, page := range pages[offset:end] {
			descriptor, err := NewSourceRowLedgerPageDescriptorV1(policy, binding, page)
			if err != nil {
				return nil, SourceRowLedgerRootV1{}, err
			}
			dataDescriptors = append(dataDescriptors, descriptor)
		}
		indexPage, err := NewSourceRowLedgerIndexPageV1(policy, binding, uint64(len(indexPages)+1), dataDescriptors)
		if err != nil {
			return nil, SourceRowLedgerRootV1{}, err
		}
		indexDescriptor, err := NewSourceRowLedgerIndexPageDescriptorV1(policy, binding, indexPage)
		if err != nil {
			return nil, SourceRowLedgerRootV1{}, err
		}
		indexPages = append(indexPages, indexPage)
		indexDescriptors = append(indexDescriptors, indexDescriptor)
	}
	root, err := NewSourceRowLedgerRootFromIndexDescriptorsV1(policy, binding, indexDescriptors)
	if err != nil {
		return nil, SourceRowLedgerRootV1{}, err
	}
	return indexPages, root, nil
}

func newSourceRowLedgerHierarchyV1FromPages(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	pages []SourceRowLedgerPageV1,
) ([]SourceRowLedgerIndexPageV1, SourceRowLedgerRootV1, error) {
	indexPages, root, err := newSourceRowLedgerHierarchyV1FromPagesWithoutFinalValidation(policy, binding, pages)
	if err != nil {
		return nil, SourceRowLedgerRootV1{}, errors.Join(errors.New("source row ledger hierarchy is invalid"), err)
	}
	if validationErr := validateSourceRowLedgerRootWithHierarchyV1(policy, root, indexPages, pages); validationErr != nil {
		return nil, SourceRowLedgerRootV1{}, errors.Join(errors.New("source row ledger hierarchy is invalid"), validationErr)
	}
	return indexPages, root, nil
}

// NewSourceRowLedgerRootFromIndexDescriptorsV1 is the bounded streaming
// finalizer. Index pages must already have been written and exact-read from
// private CAS; this structural constructor alone never grants authority.
func NewSourceRowLedgerRootFromIndexDescriptorsV1(
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	descriptors []SourceRowLedgerIndexPageDescriptorV1,
) (SourceRowLedgerRootV1, error) {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil ||
		len(descriptors) == 0 || len(descriptors) > maxSourceRowLedgerRootIndexPageCountV1 {
		return SourceRowLedgerRootV1{}, errors.New("source row ledger root index input is invalid")
	}
	root := SourceRowLedgerRootV1{
		SchemaVersion: SourceRowLedgerRootSchemaVersionV1, Purpose: SourceRowLedgerRootPurposeV1, Binding: binding,
		PolicyID: policy.PolicyID, PolicyDigest: policy.PolicyDigest, ParserID: policy.ParserID, ParserVersion: policy.ParserVersion,
		IndexPageDescriptors: append([]SourceRowLedgerIndexPageDescriptorV1(nil), descriptors...),
		IndexPageCount:       uint64(len(descriptors)),
	}
	for _, descriptor := range root.IndexPageDescriptors {
		if ValidateSourceRowLedgerIndexPageDescriptorForPolicyV1(policy, descriptor) != nil {
			return SourceRowLedgerRootV1{}, errors.New("source row ledger root index descriptor is invalid")
		}
		if root.PageCount > maxSourceRowJSONIntegerV1-uint64(descriptor.DataPageCount) ||
			root.RecordCount > maxSourceRowJSONIntegerV1-descriptor.RecordCount {
			return SourceRowLedgerRootV1{}, errors.New("source row ledger root counts exceed the canonical range")
		}
		root.PageCount += uint64(descriptor.DataPageCount)
		root.RecordCount += descriptor.RecordCount
	}
	root.RootDigest = sourceRowLedgerRootDigestV1(root)
	if err := ValidateSourceRowLedgerRootV1(policy, root); err != nil {
		return SourceRowLedgerRootV1{}, err
	}
	return root, nil
}

func ValidateSourceRowLedgerRootV1(policy SourceRowProducerPolicyV1, root SourceRowLedgerRootV1) error {
	if ValidateSourceRowProducerPolicyV1(policy) != nil || root.SchemaVersion != SourceRowLedgerRootSchemaVersionV1 ||
		root.Purpose != SourceRowLedgerRootPurposeV1 || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(root.Binding) != nil ||
		root.PolicyID != policy.PolicyID || root.PolicyDigest != policy.PolicyDigest || root.ParserID != policy.ParserID ||
		root.ParserVersion != policy.ParserVersion || len(root.IndexPageDescriptors) == 0 ||
		root.IndexPageCount != uint64(len(root.IndexPageDescriptors)) || root.IndexPageCount > maxSourceRowLedgerRootIndexPageCountV1 ||
		root.PageCount == 0 || root.PageCount > uint64(maxSourceRowLedgerDataPagesPerIndexPageV1)*root.IndexPageCount ||
		root.RecordCount == 0 || root.RecordCount > maxSourceRowJSONIntegerV1 || !validSourceRowSHA256V1(root.RootDigest) {
		return errors.New("source row ledger root is invalid")
	}
	rootBody, err := json.Marshal(root)
	if err != nil || len(rootBody) > maxSourceRowLedgerRootBytesV1 {
		return errors.New("source row ledger root exceeds its canonical byte limit")
	}
	var dataPageCount uint64
	var recordCount uint64
	var aggregatePageBytes uint64
	seenIndexPageDigests := make(map[string]struct{}, len(root.IndexPageDescriptors))
	seenIndexPageSHA256 := make(map[string]struct{}, len(root.IndexPageDescriptors))
	seenDescriptorDigests := make(map[string]struct{}, len(root.IndexPageDescriptors))
	for index, descriptor := range root.IndexPageDescriptors {
		if ValidateSourceRowLedgerIndexPageDescriptorForPolicyV1(policy, descriptor) != nil ||
			descriptor.IndexPageNumber != uint64(index+1) ||
			dataPageCount > maxSourceRowJSONIntegerV1-uint64(descriptor.DataPageCount) ||
			recordCount > maxSourceRowJSONIntegerV1-descriptor.RecordCount ||
			aggregatePageBytes > maxSourceRowLedgerAggregatePageBytesV1-descriptor.AggregateDataPageBytes {
			return errors.New("source row ledger root index descriptors are not canonical")
		}
		if index < len(root.IndexPageDescriptors)-1 &&
			(descriptor.DataPageCount != maxSourceRowLedgerDataPagesPerIndexPageV1 ||
				descriptor.RecordCount != uint64(descriptor.DataPageCount)*uint64(policy.MaxRowsPerPage)) {
			return errors.New("source row ledger root contains a partial non-terminal index page")
		}
		if _, exists := seenIndexPageDigests[descriptor.IndexPageDigest]; exists {
			return errors.New("source row ledger root reuses an index page digest")
		}
		if _, exists := seenIndexPageSHA256[descriptor.IndexPageSHA256]; exists {
			return errors.New("source row ledger root reuses an index page content address")
		}
		if _, exists := seenDescriptorDigests[descriptor.DescriptorDigest]; exists {
			return errors.New("source row ledger root reuses an index page descriptor")
		}
		if index == 0 {
			if descriptor.FirstDataPageNumber != 1 {
				return errors.New("source row ledger root data pages do not begin at one")
			}
		} else {
			previous := root.IndexPageDescriptors[index-1]
			if descriptor.FirstDataPageNumber != previous.LastDataPageNumber+1 ||
				!sourceRowIndexDescriptorStrictlyBeforeV1(previous, descriptor) {
				return errors.New("source row ledger root index boundaries are not strictly ordered")
			}
		}
		seenIndexPageDigests[descriptor.IndexPageDigest] = struct{}{}
		seenIndexPageSHA256[descriptor.IndexPageSHA256] = struct{}{}
		seenDescriptorDigests[descriptor.DescriptorDigest] = struct{}{}
		dataPageCount += uint64(descriptor.DataPageCount)
		recordCount += descriptor.RecordCount
		aggregatePageBytes += descriptor.AggregateDataPageBytes
	}
	if dataPageCount != root.PageCount || recordCount != root.RecordCount || aggregatePageBytes == 0 ||
		aggregatePageBytes > maxSourceRowLedgerAggregatePageBytesV1 ||
		root.RecordCount > root.PageCount*uint64(policy.MaxRowsPerPage) ||
		root.RootDigest != sourceRowLedgerRootDigestV1(root) {
		return errors.New("source row ledger root aggregate counts are inconsistent")
	}
	return nil
}

func validateSourceRowLedgerRootWithPagesV1(
	policy SourceRowProducerPolicyV1,
	root SourceRowLedgerRootV1,
	pages []SourceRowLedgerPageV1,
) error {
	indexPages, expected, err := newSourceRowLedgerHierarchyV1FromPagesWithoutFinalValidation(policy, root.Binding, pages)
	expectedBody, expectedErr := SourceRowLedgerRootV1Bytes(expected)
	rootBody, rootErr := SourceRowLedgerRootV1Bytes(root)
	if err != nil || expectedErr != nil || rootErr != nil || !bytes.Equal(expectedBody, rootBody) {
		return errors.New("source row ledger root pages are invalid")
	}
	return validateSourceRowLedgerRootWithHierarchyV1(policy, root, indexPages, pages)
}

func ValidateSourceRowLedgerRootForContextV1(root SourceRowLedgerRootV1, context domainsecurity.TurnSecurityContext) error {
	policy, ok := ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok || ValidateSourceRowLedgerRootV1(policy, root) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		root.Binding.TenantID != context.TenantID || root.Binding.UserID != context.UserID ||
		root.Binding.WorkspaceRealPath != context.WorkspaceRealPath || root.Binding.CaseID != context.CaseID ||
		root.Binding.CaseBindingHash != context.CaseBindingHash ||
		root.Binding.BindingObservationDigest != context.PublicationPolicy.BindingObservationDigest {
		return errors.New("source row ledger root does not match the turn binding")
	}
	return nil
}

// ValidateExactHierarchyV1 validates every index page and data page against
// this immutable root. The receiver keeps the operation bound to an already
// parsed root; it neither constructs ledger material nor confers snapshot or
// publication authority.
func (root SourceRowLedgerRootV1) ValidateExactHierarchyV1(
	indexPages []SourceRowLedgerIndexPageV1,
	dataPages []SourceRowLedgerPageV1,
) error {
	policy, ok := ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok || policy.PolicyDigest != root.PolicyDigest {
		return errors.New("source row ledger root policy is unavailable")
	}
	return validateSourceRowLedgerRootWithHierarchyV1(policy, root, indexPages, dataPages)
}

func NewSourceRowWitnessV1(input SourceRowWitnessInputV1) (SourceRowWitnessV1, error) {
	context := input.Context
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(context.DatasetSnapshotID) ||
		!validSourceRowSHA256V1(input.SnapshotAuthorityRecordDigest) ||
		ValidateSourceRowLedgerRootForContextV1(input.Root, context) != nil {
		return SourceRowWitnessV1{}, errors.New("source row witness context authority is invalid")
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(input.Record.PolicyID)
	if !ok || policy.ActivationState != "active" || !policy.CanMintClaims ||
		ValidateSourceRowWitnessLedgerMaterialV1(policy, input.Root, input.IndexPage, input.Page, input.Record) != nil {
		return SourceRowWitnessV1{}, errors.New("source row witness ledger material is invalid")
	}
	witness := SourceRowWitnessV1{
		SchemaVersion:                 SourceRowWitnessSchemaVersionV1,
		Purpose:                       SourceRowWitnessPurposeV1,
		AuthorityClass:                SourceRowWitnessAuthorityNoneV1,
		FactAnswerAllowed:             false,
		ThreadID:                      context.ThreadID,
		TurnID:                        context.TurnID,
		CaseID:                        context.CaseID,
		CaseBindingHash:               context.CaseBindingHash,
		ContextEpoch:                  context.ContextEpoch,
		ContextDigest:                 context.ContextDigest,
		DatasetSnapshotID:             context.DatasetSnapshotID,
		SourceManifestHash:            context.SourceManifestHash,
		SnapshotAuthorityRecordDigest: input.SnapshotAuthorityRecordDigest,
		BindingKeyDigest:              input.Root.Binding.BindingKeyDigest,
		PolicyID:                      policy.PolicyID,
		PolicyDigest:                  policy.PolicyDigest,
		LedgerRootDigest:              input.Root.RootDigest,
		LedgerIndexPageDigest:         input.IndexPage.IndexPageDigest,
		LedgerPageDigest:              input.Page.PageDigest,
		SourceRecordID:                input.Record.SourceRecordID,
		SourceArtifactSHA256:          input.Record.SourceArtifactSHA256,
		CanonicalRowSHA256:            input.Record.CanonicalRowSHA256,
		LocatorDigest:                 input.Record.Locator.LocatorDigest,
		ParserID:                      input.Record.ParserID,
		ParserVersion:                 input.Record.ParserVersion,
		LineageDigest:                 input.Record.LineageDigest,
		LineageSHA256:                 input.Record.LineageSHA256,
		LineageByteLength:             input.Record.LineageByteLength,
	}
	witness.WitnessDigest = sourceRowWitnessDigestV1(witness)
	if err := ValidateSourceRowWitnessForContextAndLedgerV1(
		witness, context, input.SnapshotAuthorityRecordDigest, input.Root, input.IndexPage, input.Page, input.Record,
	); err != nil {
		return SourceRowWitnessV1{}, err
	}
	return witness, nil
}

func ValidateSourceRowWitnessV1(witness SourceRowWitnessV1) error {
	policy, ok := ResolveSourceRowProducerPolicyV1(witness.PolicyID)
	if !ok || witness.SchemaVersion != SourceRowWitnessSchemaVersionV1 || witness.Purpose != SourceRowWitnessPurposeV1 ||
		witness.AuthorityClass != SourceRowWitnessAuthorityNoneV1 || witness.FactAnswerAllowed ||
		!validSourceRowPolicyTextV1(witness.ThreadID) || !validSourceRowPolicyTextV1(witness.TurnID) ||
		!validSourceRowPolicyTextV1(witness.CaseID) || witness.CaseID == domainsecurity.UnboundCaseID ||
		!validSourceRowSHA256V1(witness.CaseBindingHash) || witness.ContextEpoch == 0 || witness.ContextEpoch > maxSourceRowJSONIntegerV1 ||
		!validSourceRowSHA256V1(witness.ContextDigest) || !domainsecurity.IsDatasetSnapshotIDV2Syntax(witness.DatasetSnapshotID) ||
		witness.DatasetSnapshotID != strings.TrimSpace(witness.DatasetSnapshotID) || !validSourceRowSHA256V1(witness.SourceManifestHash) ||
		!validSourceRowSHA256V1(witness.SnapshotAuthorityRecordDigest) || !validSourceRowSHA256V1(witness.BindingKeyDigest) ||
		witness.PolicyDigest != policy.PolicyDigest || !validSourceRowSHA256V1(witness.LedgerRootDigest) ||
		!validSourceRowSHA256V1(witness.LedgerIndexPageDigest) || !validSourceRowSHA256V1(witness.LedgerPageDigest) ||
		!validSourceRowRecordIDV1(witness.SourceRecordID) ||
		!validSourceRowSHA256V1(witness.SourceArtifactSHA256) || !validSourceRowSHA256V1(witness.CanonicalRowSHA256) ||
		!validSourceRowSHA256V1(witness.LocatorDigest) || witness.ParserID != policy.ParserID ||
		witness.ParserVersion != policy.ParserVersion || !validSourceRowSHA256V1(witness.LineageDigest) ||
		!validSourceRowSHA256V1(witness.LineageSHA256) || witness.LineageByteLength == 0 ||
		witness.LineageByteLength > maxSourceRowLineageBytesV1 ||
		!validSourceRowSHA256V1(witness.WitnessDigest) || witness.WitnessDigest != sourceRowWitnessDigestV1(witness) {
		return errors.New("source row witness is invalid")
	}
	return nil
}

func ValidateSourceRowWitnessLedgerMaterialV1(
	policy SourceRowProducerPolicyV1,
	root SourceRowLedgerRootV1,
	indexPage SourceRowLedgerIndexPageV1,
	page SourceRowLedgerPageV1,
	record SourceRowRecordV1,
) error {
	if ValidateSourceRowLedgerRootV1(policy, root) != nil || ValidateSourceRowLedgerPageForBindingV1(policy, root.Binding, page) != nil ||
		ValidateSourceRowLedgerIndexPageForBindingV1(policy, root.Binding, indexPage) != nil ||
		ValidateSourceRowRecordForBindingV1(policy, root.Binding, record) != nil ||
		indexPage.IndexPageNumber > uint64(len(root.IndexPageDescriptors)) ||
		page.PageNumber < indexPage.PageDescriptors[0].PageNumber ||
		page.PageNumber > indexPage.PageDescriptors[len(indexPage.PageDescriptors)-1].PageNumber {
		return errors.New("source row witness ledger material is invalid")
	}
	indexDescriptor, err := NewSourceRowLedgerIndexPageDescriptorV1(policy, root.Binding, indexPage)
	if err != nil || indexDescriptor != root.IndexPageDescriptors[indexPage.IndexPageNumber-1] {
		return errors.New("source row witness index page descriptor mismatch")
	}
	descriptor, err := NewSourceRowLedgerPageDescriptorV1(policy, root.Binding, page)
	pageOffset := page.PageNumber - indexPage.PageDescriptors[0].PageNumber
	if err != nil || pageOffset >= uint64(len(indexPage.PageDescriptors)) || descriptor != indexPage.PageDescriptors[pageOffset] {
		return errors.New("source row witness page descriptor mismatch")
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Record == record {
			found = true
			break
		}
	}
	if !found {
		return errors.New("source row witness record is absent from its page")
	}
	return nil
}

func ValidateSourceRowWitnessForContextAndLedgerV1(
	witness SourceRowWitnessV1,
	context domainsecurity.TurnSecurityContext,
	snapshotAuthorityRecordDigest string,
	root SourceRowLedgerRootV1,
	indexPage SourceRowLedgerIndexPageV1,
	page SourceRowLedgerPageV1,
	record SourceRowRecordV1,
) error {
	if ValidateSourceRowWitnessV1(witness) != nil || !domainsecurity.IsDatasetSnapshotIDV2Syntax(context.DatasetSnapshotID) ||
		!validSourceRowSHA256V1(snapshotAuthorityRecordDigest) || ValidateSourceRowLedgerRootForContextV1(root, context) != nil {
		return errors.New("source row witness authority comparison is invalid")
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(witness.PolicyID)
	if !ok || ValidateSourceRowWitnessLedgerMaterialV1(policy, root, indexPage, page, record) != nil ||
		witness.ThreadID != context.ThreadID || witness.TurnID != context.TurnID || witness.CaseID != context.CaseID ||
		witness.CaseBindingHash != context.CaseBindingHash || witness.ContextEpoch != context.ContextEpoch ||
		witness.ContextDigest != context.ContextDigest || witness.DatasetSnapshotID != context.DatasetSnapshotID ||
		witness.SourceManifestHash != context.SourceManifestHash || witness.SnapshotAuthorityRecordDigest != snapshotAuthorityRecordDigest ||
		witness.BindingKeyDigest != root.Binding.BindingKeyDigest || witness.PolicyDigest != root.PolicyDigest ||
		witness.LedgerRootDigest != root.RootDigest || witness.LedgerIndexPageDigest != indexPage.IndexPageDigest ||
		witness.LedgerPageDigest != page.PageDigest ||
		witness.SourceRecordID != record.SourceRecordID || witness.SourceArtifactSHA256 != record.SourceArtifactSHA256 ||
		witness.CanonicalRowSHA256 != record.CanonicalRowSHA256 || witness.LocatorDigest != record.Locator.LocatorDigest ||
		witness.ParserID != record.ParserID || witness.ParserVersion != record.ParserVersion || witness.LineageDigest != record.LineageDigest {
		return errors.New("source row witness does not match current context and ledger")
	}
	if witness.LineageSHA256 != record.LineageSHA256 || witness.LineageByteLength != record.LineageByteLength {
		return errors.New("source row witness does not match current context and ledger")
	}
	return nil
}

func CanonicalSourceRowWitnessesV1(witnesses []SourceRowWitnessV1) ([]SourceRowWitnessV1, error) {
	if len(witnesses) == 0 {
		return nil, errors.New("source row witness set is empty")
	}
	canonical := append([]SourceRowWitnessV1(nil), witnesses...)
	for _, witness := range canonical {
		if ValidateSourceRowWitnessV1(witness) != nil {
			return nil, errors.New("source row witness set contains an invalid witness")
		}
	}
	sort.Slice(canonical, func(left, right int) bool {
		return canonical[left].WitnessDigest < canonical[right].WitnessDigest
	})
	recordIdentityByKey := make(map[string]string, len(canonical))
	recordIDByLocator := make(map[string]string, len(canonical))
	for index, witness := range canonical {
		if index > 0 && canonical[index-1].WitnessDigest == witness.WitnessDigest {
			return nil, errors.New("source row witness set contains a duplicate witness")
		}
		identity, _ := SourceRowWitnessIdentityDigestV1(witness)
		recordKey := canonicalSourceRowKeyV1(struct {
			DatasetSnapshotID string `json:"datasetSnapshotId"`
			SourceRecordID    string `json:"sourceRecordId"`
		}{witness.DatasetSnapshotID, witness.SourceRecordID})
		locatorKey := canonicalSourceRowKeyV1(struct {
			DatasetSnapshotID string `json:"datasetSnapshotId"`
			BindingKeyDigest  string `json:"bindingKeyDigest"`
			PolicyDigest      string `json:"policyDigest"`
			LocatorDigest     string `json:"locatorDigest"`
		}{witness.DatasetSnapshotID, witness.BindingKeyDigest, witness.PolicyDigest, witness.LocatorDigest})
		if previous, exists := recordIdentityByKey[recordKey]; exists && previous != identity {
			return nil, errors.New("source row witness set rebinds a source record")
		}
		if previous, exists := recordIDByLocator[locatorKey]; exists && previous != witness.SourceRecordID {
			return nil, errors.New("source row witness set maps one locator to multiple record ids")
		}
		recordIdentityByKey[recordKey] = identity
		recordIDByLocator[locatorKey] = witness.SourceRecordID
	}
	return canonical, nil
}

func SourceRowWitnessIdentityDigestV1(witness SourceRowWitnessV1) (string, error) {
	if ValidateSourceRowWitnessV1(witness) != nil {
		return "", errors.New("source row witness identity is invalid")
	}
	identity := struct {
		BindingKeyDigest     string `json:"bindingKeyDigest"`
		PolicyDigest         string `json:"policyDigest"`
		LedgerRootDigest     string `json:"ledgerRootDigest"`
		LedgerPageDigest     string `json:"ledgerPageDigest"`
		SourceArtifactSHA256 string `json:"sourceArtifactSha256"`
		CanonicalRowSHA256   string `json:"canonicalRowSha256"`
		LocatorDigest        string `json:"locatorDigest"`
		ParserID             string `json:"parserId"`
		ParserVersion        string `json:"parserVersion"`
		LineageDigest        string `json:"lineageDigest"`
		LineageSHA256        string `json:"lineageSha256"`
		LineageByteLength    uint64 `json:"lineageByteLength"`
	}{witness.BindingKeyDigest, witness.PolicyDigest, witness.LedgerRootDigest, witness.LedgerPageDigest,
		witness.SourceArtifactSHA256, witness.CanonicalRowSHA256, witness.LocatorDigest,
		witness.ParserID, witness.ParserVersion, witness.LineageDigest, witness.LineageSHA256, witness.LineageByteLength}
	body, _ := json.Marshal(identity)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowWitnessIdentityDigestDomainV1...), body...)), nil
}

func SourceRowWitnessSetDigestV1(witnesses []SourceRowWitnessV1) (string, error) {
	canonical, err := CanonicalSourceRowWitnessesV1(witnesses)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(canonical)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowWitnessSetDigestDomainV1...), body...)), nil
}

func ParseSourceRowLedgerPageV1(raw []byte) (SourceRowLedgerPageV1, error) {
	var page SourceRowLedgerPageV1
	if err := parseCanonicalSourceRowContractV1(raw, &page, 1024*1024, 200_000, maxSourceRowPolicyTextBytesV1); err != nil {
		return SourceRowLedgerPageV1{}, err
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(page.PolicyID)
	if !ok {
		return SourceRowLedgerPageV1{}, errors.New("source row ledger page policy is unknown")
	}
	return page, ValidateSourceRowLedgerPageV1(policy, page)
}

func ParseSourceRowLedgerRootV1(raw []byte) (SourceRowLedgerRootV1, error) {
	var root SourceRowLedgerRootV1
	if err := parseCanonicalSourceRowContractV1(raw, &root, maxSourceRowLedgerRootBytesV1, 1_000_000, maxSourceRowBindingTextBytesV1); err != nil {
		return SourceRowLedgerRootV1{}, err
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok {
		return SourceRowLedgerRootV1{}, errors.New("source row ledger root policy is unknown")
	}
	return root, ValidateSourceRowLedgerRootV1(policy, root)
}

func ParseSourceRowWitnessV1(raw []byte) (SourceRowWitnessV1, error) {
	var witness SourceRowWitnessV1
	if err := parseCanonicalSourceRowContractV1(raw, &witness, 128*1024, 512, maxSourceRowPolicyTextBytesV1); err != nil {
		return SourceRowWitnessV1{}, err
	}
	return witness, ValidateSourceRowWitnessV1(witness)
}

// SourceRowWitnessCanAuthorizeFactsV1 is deliberately constant-false. This
// wire value is structural comparison material; only a future app-layer
// callback that revalidates a fresh registry-backed DSV2 record and private
// CAS bytes may grant temporary factual use.
func SourceRowWitnessCanAuthorizeFactsV1(SourceRowWitnessV1) bool {
	return false
}

func SourceRowLedgerPageV1Bytes(page SourceRowLedgerPageV1) ([]byte, error) {
	policy, ok := ResolveSourceRowProducerPolicyV1(page.PolicyID)
	if !ok || ValidateSourceRowLedgerPageV1(policy, page) != nil {
		return nil, errors.New("source row ledger page is invalid")
	}
	return json.Marshal(page)
}

func SourceRowLedgerRootV1Bytes(root SourceRowLedgerRootV1) ([]byte, error) {
	policy, ok := ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok || ValidateSourceRowLedgerRootV1(policy, root) != nil {
		return nil, errors.New("source row ledger root is invalid")
	}
	body, err := json.Marshal(root)
	if err != nil || len(body) > maxSourceRowLedgerRootBytesV1 {
		return nil, errors.New("source row ledger root exceeds its canonical byte limit")
	}
	return body, nil
}

func SourceRowWitnessV1Bytes(witness SourceRowWitnessV1) ([]byte, error) {
	if ValidateSourceRowWitnessV1(witness) != nil {
		return nil, errors.New("source row witness is invalid")
	}
	return json.Marshal(witness)
}

func sourceRowLocatorDigestV1(locator SourceRowLocatorV1) string {
	locator.LocatorDigest = ""
	body, _ := json.Marshal(locator)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLocatorDigestDomainV1...), body...))
}

func sourceRowSourceFileIDDigestV1(sourceFileID string) string {
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowSourceFileIDDigestDomainV1...), []byte(sourceFileID)...))
}

func sourceRowLocatorFromDescriptorBoundaryV1(sourceFileIDDigest string, sourceRowNumber uint64) SourceRowLocatorV1 {
	return SourceRowLocatorV1{SourceFileIDDigest: sourceFileIDDigest, SourceRowNumber: sourceRowNumber}
}

func sourceRowLocatorForPolicyBoundaryV1(
	policy SourceRowProducerPolicyV1,
	sourceFileIDDigest string,
	sourceRowNumber uint64,
) SourceRowLocatorV1 {
	locator := SourceRowLocatorV1{
		SchemaVersion:      SourceRowLocatorSchemaVersionV1,
		Purpose:            SourceRowLocatorPurposeV1,
		RelationID:         policy.Relation,
		SourceFileIDDigest: sourceFileIDDigest,
		SourceRowNumber:    sourceRowNumber,
	}
	locator.LocatorDigest = sourceRowLocatorDigestV1(locator)
	return locator
}

func sourceRowLocatorStrictlyBeforeV1(left, right SourceRowLocatorV1) bool {
	if left.SourceFileIDDigest != right.SourceFileIDDigest {
		return left.SourceFileIDDigest < right.SourceFileIDDigest
	}
	return left.SourceRowNumber < right.SourceRowNumber
}

func sourceRowRecordDigestV1(record SourceRowRecordV1) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowRecordDigestDomainV1...), body...))
}

func sourceRowLedgerPageEntryDigestV1(entry SourceRowLedgerPageEntryV1) string {
	entry.EntryDigest = ""
	body, _ := json.Marshal(entry)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLedgerPageEntryDigestDomainV1...), body...))
}

func sourceRowLedgerPageDigestV1(page SourceRowLedgerPageV1) string {
	page.PageDigest = ""
	body, _ := json.Marshal(page)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLedgerPageDigestDomainV1...), body...))
}

func sourceRowLedgerPageDescriptorDigestV1(descriptor SourceRowLedgerPageDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLedgerDescriptorDigestDomainV1...), body...))
}

func sourceRowLedgerRootDigestV1(root SourceRowLedgerRootV1) string {
	root.RootDigest = ""
	body, _ := json.Marshal(root)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowLedgerRootDigestDomainV1...), body...))
}

func sourceRowWitnessDigestV1(witness SourceRowWitnessV1) string {
	witness.WitnessDigest = ""
	body, _ := json.Marshal(witness)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), sourceRowWitnessDigestDomainV1...), body...))
}

func validSourceRowRecordIDV1(value string) bool {
	return value == strings.TrimSpace(value) && strings.HasPrefix(value, SourceRowRecordIDPrefixV1) &&
		domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, SourceRowRecordIDPrefixV1))
}

func validSourceRowSHA256V1(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}

func canonicalSourceRowKeyV1(value any) string {
	body, _ := json.Marshal(value)
	return string(body)
}

func parseCanonicalSourceRowContractV1(raw []byte, target any, maxBytes, maxTokens, maxStringBytes int) error {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxBytes,
		MaxDepth:       16,
		MaxTokens:      maxTokens,
		MaxStringBytes: maxStringBytes,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("source row contract contains trailing JSON")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(raw, canonical) {
		return errors.New("source row contract is not canonically encoded")
	}
	return nil
}
