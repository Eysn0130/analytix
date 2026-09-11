package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	DatasetSnapshotManifestSchemaVersionV2 = 2
	DatasetSnapshotManifestPurposeV2       = "analytix.dataset-snapshot-manifest/v2"

	maxDatasetSnapshotManifestBytesV2 = 512 * 1024
	// Every referenced manifest is one strict canonical private-CAS object.
	// Larger raw inputs must use bounded opaque chunks plus a manifest that
	// itself remains within this single-object admission limit.
	maxDatasetSnapshotReferencedManifestBytesV2  = 16 * 1024 * 1024
	maxDatasetSnapshotSourceRowLedgerRootBytesV2 = 16 * 1024 * 1024
	maxDatasetSnapshotAnalyticalDuckDBBytesV2    = 1 << 40
	maxDatasetSnapshotAnalyticalBindingBytesV2   = 8 * 1024
	minDatasetSnapshotUTCOffsetMinutesV2         = int16(-840)
	maxDatasetSnapshotUTCOffsetMinutesV2         = int16(840)
	datasetSnapshotAnalyticalMinorUnitScaleV2    = uint8(2)
)

var (
	datasetSnapshotSourceManifestHashDomainV2 = []byte("analytix.dataset-source-manifest/hash/v2\x00")
	datasetSnapshotManifestDigestDomainV2     = []byte("analytix.dataset-snapshot-manifest/digest/v2\x00")
)

// DatasetSnapshotManifestV2 is immutable and contains no source-row or direct
// PII payload. It does contain private case/workspace scope metadata and must
// never be projected to ordinary UI/SSE/history/logs. Its remaining fields are
// host-derived identities, content references, fixed producer contracts, and
// counts. It deliberately excludes transport
// readiness, MCP identity, connection epoch, thread/turn/context ids, a
// dataset snapshot id, and any "current" assertion.
//
// A structurally valid manifest is not authority. Factual use additionally
// requires exact CAS readback, a signed V2 authority record, membership in the
// existing monotonic dataset-snapshot index, current witness selection, and
// an app-layer producer/row-ledger cross-check.
type DatasetSnapshotManifestV2 struct {
	SchemaVersion int                         `json:"schemaVersion"`
	Purpose       string                      `json:"purpose"`
	Binding       DatasetSnapshotBindingKeyV1 `json:"binding"`

	AcquisitionMethod      string `json:"acquisitionMethod"`
	AcquiredAt             string `json:"acquiredAt"`
	AcquisitionActorDigest string `json:"acquisitionActorDigest"`

	RawArtifactManifestDigest     string `json:"rawArtifactManifestDigest"`
	RawArtifactManifestSHA256     string `json:"rawArtifactManifestSha256"`
	RawArtifactManifestByteLength uint64 `json:"rawArtifactManifestByteLength"`
	RawArtifactCount              uint64 `json:"rawArtifactCount"`

	ProducerContentContract            string `json:"producerContentContract"`
	ProducerContentID                  string `json:"producerContentId"`
	ProducerContentManifestSHA256      string `json:"producerContentManifestSha256"`
	ProducerContentManifestByteLength  uint64 `json:"producerContentManifestByteLength"`
	ProducerContentComponentID         string `json:"producerContentComponentId"`
	ProducerContentComponentVersion    string `json:"producerContentComponentVersion"`
	ProducerContentOperation           string `json:"producerContentOperation"`
	ProducerContentOperationSchemaHash string `json:"producerContentOperationSchemaHash"`
	ProducerContentEngine              string `json:"producerContentEngine"`
	ProducerContentEncoder             string `json:"producerContentEncoder"`

	SourceType                        string `json:"sourceType"`
	ProducerPolicyID                  string `json:"producerPolicyId"`
	ProducerPolicyDigest              string `json:"producerPolicyDigest"`
	ProducerComponentID               string `json:"producerComponentId"`
	ProducerComponentVersion          string `json:"producerComponentVersion"`
	ProducerOperation                 string `json:"producerOperation"`
	ProducerOperationSchemaHash       string `json:"producerOperationSchemaHash"`
	ParserID                          string `json:"parserId"`
	ParserVersion                     string `json:"parserVersion"`
	ParsedGenerationReceiptDigest     string `json:"parsedGenerationReceiptDigest"`
	ParsedGenerationReceiptSHA256     string `json:"parsedGenerationReceiptSha256"`
	ParsedGenerationReceiptByteLength uint64 `json:"parsedGenerationReceiptByteLength"`

	ClassificationLedgerDigest     string                                   `json:"classificationLedgerDigest"`
	ClassificationLedgerSHA256     string                                   `json:"classificationLedgerSha256"`
	ClassificationLedgerByteLength uint64                                   `json:"classificationLedgerByteLength"`
	TimezoneSemantics              string                                   `json:"timezoneSemantics"`
	CurrencySemantics              string                                   `json:"currencySemantics"`
	AnalyticalDuckDB               DatasetSnapshotAnalyticalDuckDBBindingV2 `json:"analyticalDuckdb,omitempty"`

	SourceManifestHash            string `json:"sourceManifestHash"`
	SourceRowLedgerRootDigest     string `json:"sourceRowLedgerRootDigest"`
	SourceRowLedgerRootSHA256     string `json:"sourceRowLedgerRootSha256"`
	SourceRowLedgerRootByteLength uint64 `json:"sourceRowLedgerRootByteLength"`
	SourceRowLedgerPageCount      uint64 `json:"sourceRowLedgerPageCount"`
	SourceRecordCount             uint64 `json:"sourceRecordCount"`
	AcceptedRecordCount           uint64 `json:"acceptedRecordCount"`
	RejectedRecordCount           uint64 `json:"rejectedRecordCount"`
	DuplicateRecordCount          uint64 `json:"duplicateRecordCount"`
	ManifestDigest                string `json:"manifestDigest"`
}

type DatasetSnapshotManifestInputV2 struct {
	Binding DatasetSnapshotBindingKeyV1

	AcquisitionMethod      string
	AcquiredAt             time.Time
	AcquisitionActorDigest string

	RawArtifactManifestDigest     string
	RawArtifactManifestSHA256     string
	RawArtifactManifestByteLength uint64
	RawArtifactCount              uint64

	FundsProducerContentManifest FundsProducerContentManifestV1

	SourceType                        string
	ProducerPolicyID                  string
	ProducerPolicyDigest              string
	ProducerComponentID               string
	ProducerComponentVersion          string
	ProducerOperation                 string
	ProducerOperationSchemaHash       string
	ParserID                          string
	ParserVersion                     string
	ParsedGenerationReceiptDigest     string
	ParsedGenerationReceiptSHA256     string
	ParsedGenerationReceiptByteLength uint64

	ClassificationLedgerDigest     string
	ClassificationLedgerSHA256     string
	ClassificationLedgerByteLength uint64
	TimezoneSemantics              string
	CurrencySemantics              string
	AnalyticalDuckDB               DatasetSnapshotAnalyticalDuckDBBindingV2

	SourceRowLedgerRootDigest     string
	SourceRowLedgerRootSHA256     string
	SourceRowLedgerRootByteLength uint64
	SourceRowLedgerPageCount      uint64
	SourceRecordCount             uint64
	AcceptedRecordCount           uint64
	RejectedRecordCount           uint64
	DuplicateRecordCount          uint64
}

// DatasetSnapshotAnalyticalDuckDBBindingV2 is the canonical, path-free
// identity of one immutable analytical DuckDB object admitted with an FPC1
// dataset snapshot. The opaque string representation keeps the containing
// manifest comparable while MarshalJSON emits the typed object below; the
// zero value is omitted so existing DSV2 bytes and hashes remain unchanged.
//
// This value is source identity only. It grants no file access, query,
// currentness, evidence, or publication authority.
type DatasetSnapshotAnalyticalDuckDBBindingV2 string

type DatasetSnapshotAnalyticalDuckDBBindingInputV2 struct {
	DuckDBSHA256                 string
	DuckDBByteLength             uint64
	DuckDBContentSnapshotDigest  string
	DuckDBSnapshotManifestSHA256 string
	MaterializationIdentity      string
	SchemaDigest                 string
	QueryProfileDigest           string
	DatasetUTCOffsetMinutes      int16
	ExpectedCurrency             string
	MinorUnitScale               uint8
}

type datasetSnapshotAnalyticalDuckDBBindingWireV2 struct {
	DuckDBSHA256                 string `json:"duckdbSha256"`
	DuckDBByteLength             uint64 `json:"duckdbByteLength"`
	DuckDBContentSnapshotDigest  string `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256 string `json:"duckdbSnapshotManifestSha256"`
	MaterializationIdentity      string `json:"materializationIdentity"`
	SchemaDigest                 string `json:"schemaDigest"`
	QueryProfileDigest           string `json:"queryProfileDigest"`
	DatasetUTCOffsetMinutes      int16  `json:"datasetUtcOffsetMinutes"`
	ExpectedCurrency             string `json:"expectedCurrency"`
	MinorUnitScale               uint8  `json:"minorUnitScale"`
}

func NewDatasetSnapshotAnalyticalDuckDBBindingV2(
	input DatasetSnapshotAnalyticalDuckDBBindingInputV2,
) (DatasetSnapshotAnalyticalDuckDBBindingV2, error) {
	wire := datasetSnapshotAnalyticalDuckDBBindingWireV2{
		DuckDBSHA256:                 input.DuckDBSHA256,
		DuckDBByteLength:             input.DuckDBByteLength,
		DuckDBContentSnapshotDigest:  input.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256: input.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:      input.MaterializationIdentity,
		SchemaDigest:                 input.SchemaDigest,
		QueryProfileDigest:           input.QueryProfileDigest,
		DatasetUTCOffsetMinutes:      input.DatasetUTCOffsetMinutes,
		ExpectedCurrency:             input.ExpectedCurrency,
		MinorUnitScale:               input.MinorUnitScale,
	}
	if err := validateDatasetSnapshotAnalyticalDuckDBBindingWireV2(wire); err != nil {
		return "", err
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	return DatasetSnapshotAnalyticalDuckDBBindingV2(body), nil
}

func ParseDatasetSnapshotAnalyticalDuckDBBindingV2(
	raw []byte,
) (DatasetSnapshotAnalyticalDuckDBBindingV2, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxDatasetSnapshotAnalyticalBindingBytesV2,
		MaxDepth:       2,
		MaxTokens:      32,
		MaxStringBytes: 512,
	}); err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire datasetSnapshotAnalyticalDuckDBBindingWireV2
	if err := decoder.Decode(&wire); err != nil {
		return "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("dataset snapshot analytical DuckDB binding contains trailing JSON")
	}
	if err := validateDatasetSnapshotAnalyticalDuckDBBindingWireV2(wire); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(wire)
	if err != nil || !bytes.Equal(raw, canonical) {
		return "", errors.New("dataset snapshot analytical DuckDB binding is not canonically encoded")
	}
	return DatasetSnapshotAnalyticalDuckDBBindingV2(canonical), nil
}

func ValidateDatasetSnapshotAnalyticalDuckDBBindingV2(
	binding DatasetSnapshotAnalyticalDuckDBBindingV2,
) error {
	_, err := ParseDatasetSnapshotAnalyticalDuckDBBindingV2([]byte(binding))
	return err
}

func (binding DatasetSnapshotAnalyticalDuckDBBindingV2) Values() (
	DatasetSnapshotAnalyticalDuckDBBindingInputV2,
	error,
) {
	parsed, err := ParseDatasetSnapshotAnalyticalDuckDBBindingV2([]byte(binding))
	if err != nil {
		return DatasetSnapshotAnalyticalDuckDBBindingInputV2{}, err
	}
	var wire datasetSnapshotAnalyticalDuckDBBindingWireV2
	if err := json.Unmarshal([]byte(parsed), &wire); err != nil {
		return DatasetSnapshotAnalyticalDuckDBBindingInputV2{}, err
	}
	return DatasetSnapshotAnalyticalDuckDBBindingInputV2{
		DuckDBSHA256:                 wire.DuckDBSHA256,
		DuckDBByteLength:             wire.DuckDBByteLength,
		DuckDBContentSnapshotDigest:  wire.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256: wire.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:      wire.MaterializationIdentity,
		SchemaDigest:                 wire.SchemaDigest,
		QueryProfileDigest:           wire.QueryProfileDigest,
		DatasetUTCOffsetMinutes:      wire.DatasetUTCOffsetMinutes,
		ExpectedCurrency:             wire.ExpectedCurrency,
		MinorUnitScale:               wire.MinorUnitScale,
	}, nil
}

func (binding DatasetSnapshotAnalyticalDuckDBBindingV2) MarshalJSON() ([]byte, error) {
	parsed, err := ParseDatasetSnapshotAnalyticalDuckDBBindingV2([]byte(binding))
	if err != nil {
		return nil, err
	}
	return []byte(parsed), nil
}

func (binding *DatasetSnapshotAnalyticalDuckDBBindingV2) UnmarshalJSON(raw []byte) error {
	if binding == nil {
		return errors.New("dataset snapshot analytical DuckDB binding receiver is nil")
	}
	parsed, err := ParseDatasetSnapshotAnalyticalDuckDBBindingV2(raw)
	if err != nil {
		return err
	}
	*binding = parsed
	return nil
}

func NewDatasetSnapshotManifestV2(input DatasetSnapshotManifestInputV2) (DatasetSnapshotManifestV2, error) {
	producerContent, err := newFundsProducerContentReferenceV1(input.FundsProducerContentManifest)
	if err != nil || producerContent.CaseID != input.Binding.CaseID {
		return DatasetSnapshotManifestV2{}, errors.New("dataset snapshot manifest v2 funds producer content is invalid")
	}
	manifest, err := newDatasetSnapshotManifestV2(
		input,
		datasetSnapshotProducerContentReferenceV2{
			CaseID: producerContent.CaseID, Contract: producerContent.Contract,
			ID: producerContent.ID, ManifestSHA256: producerContent.ManifestSHA256,
			ManifestByteLength: producerContent.ManifestByteLength,
			ComponentID:        producerContent.ComponentID, ComponentVersion: producerContent.ComponentVersion,
			Operation: producerContent.Operation, OperationSchemaHash: producerContent.OperationSchemaHash,
			Engine: producerContent.Engine, Encoder: producerContent.Encoder,
		},
	)
	if err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	if err := ValidateDatasetSnapshotManifestV2FundsProducerContentV1(
		manifest,
		input.FundsProducerContentManifest,
	); err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	return manifest, nil
}

// NewDatasetSnapshotManifestForFundsProducerContentV2 constructs a DSV2
// manifest for the exact Go strict-CSV count-only producer. The legacy V1
// field in input must be empty so a caller cannot supply two producer
// identities and rely on one being ignored.
func NewDatasetSnapshotManifestForFundsProducerContentV2(
	input DatasetSnapshotManifestInputV2,
	producer FundsProducerContentManifestV2,
) (DatasetSnapshotManifestV2, error) {
	if input.FundsProducerContentManifest != (FundsProducerContentManifestV1{}) {
		return DatasetSnapshotManifestV2{}, errors.New("dataset snapshot manifest v2 producer selection is ambiguous")
	}
	producerContent, err := newFundsProducerContentReferenceV2(producer)
	if err != nil || producerContent.CaseID != input.Binding.CaseID {
		return DatasetSnapshotManifestV2{}, errors.New("dataset snapshot manifest v2 funds producer content v2 is invalid")
	}
	manifest, err := newDatasetSnapshotManifestV2(
		input,
		datasetSnapshotProducerContentReferenceV2{
			CaseID: producerContent.CaseID, Contract: producerContent.Contract,
			ID: producerContent.ID, ManifestSHA256: producerContent.ManifestSHA256,
			ManifestByteLength: producerContent.ManifestByteLength,
			ComponentID:        producerContent.ComponentID, ComponentVersion: producerContent.ComponentVersion,
			Operation: producerContent.Operation, OperationSchemaHash: producerContent.OperationSchemaHash,
			Engine: producerContent.Engine, Encoder: producerContent.Encoder,
		},
	)
	if err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	if err := ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, producer); err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	return manifest, nil
}

type datasetSnapshotProducerContentReferenceV2 struct {
	CaseID              string
	Contract            string
	ID                  string
	ManifestSHA256      string
	ManifestByteLength  uint64
	ComponentID         string
	ComponentVersion    string
	Operation           string
	OperationSchemaHash string
	Engine              string
	Encoder             string
}

func newDatasetSnapshotManifestV2(
	input DatasetSnapshotManifestInputV2,
	producerContent datasetSnapshotProducerContentReferenceV2,
) (DatasetSnapshotManifestV2, error) {
	acquiredAt := input.AcquiredAt.UTC()
	manifest := DatasetSnapshotManifestV2{
		SchemaVersion: DatasetSnapshotManifestSchemaVersionV2,
		Purpose:       DatasetSnapshotManifestPurposeV2,
		Binding:       input.Binding,

		AcquisitionMethod:      strings.TrimSpace(input.AcquisitionMethod),
		AcquisitionActorDigest: strings.TrimSpace(input.AcquisitionActorDigest),

		RawArtifactManifestDigest:     strings.TrimSpace(input.RawArtifactManifestDigest),
		RawArtifactManifestSHA256:     strings.TrimSpace(input.RawArtifactManifestSHA256),
		RawArtifactManifestByteLength: input.RawArtifactManifestByteLength,
		RawArtifactCount:              input.RawArtifactCount,

		ProducerContentContract:            producerContent.Contract,
		ProducerContentID:                  producerContent.ID,
		ProducerContentManifestSHA256:      producerContent.ManifestSHA256,
		ProducerContentManifestByteLength:  producerContent.ManifestByteLength,
		ProducerContentComponentID:         producerContent.ComponentID,
		ProducerContentComponentVersion:    producerContent.ComponentVersion,
		ProducerContentOperation:           producerContent.Operation,
		ProducerContentOperationSchemaHash: producerContent.OperationSchemaHash,
		ProducerContentEngine:              producerContent.Engine,
		ProducerContentEncoder:             producerContent.Encoder,

		SourceType:                        strings.TrimSpace(input.SourceType),
		ProducerPolicyID:                  strings.TrimSpace(input.ProducerPolicyID),
		ProducerPolicyDigest:              strings.TrimSpace(input.ProducerPolicyDigest),
		ProducerComponentID:               strings.TrimSpace(input.ProducerComponentID),
		ProducerComponentVersion:          strings.TrimSpace(input.ProducerComponentVersion),
		ProducerOperation:                 strings.TrimSpace(input.ProducerOperation),
		ProducerOperationSchemaHash:       strings.TrimSpace(input.ProducerOperationSchemaHash),
		ParserID:                          strings.TrimSpace(input.ParserID),
		ParserVersion:                     strings.TrimSpace(input.ParserVersion),
		ParsedGenerationReceiptDigest:     strings.TrimSpace(input.ParsedGenerationReceiptDigest),
		ParsedGenerationReceiptSHA256:     strings.TrimSpace(input.ParsedGenerationReceiptSHA256),
		ParsedGenerationReceiptByteLength: input.ParsedGenerationReceiptByteLength,

		ClassificationLedgerDigest:     strings.TrimSpace(input.ClassificationLedgerDigest),
		ClassificationLedgerSHA256:     strings.TrimSpace(input.ClassificationLedgerSHA256),
		ClassificationLedgerByteLength: input.ClassificationLedgerByteLength,
		TimezoneSemantics:              strings.TrimSpace(input.TimezoneSemantics),
		CurrencySemantics:              strings.TrimSpace(input.CurrencySemantics),
		AnalyticalDuckDB:               input.AnalyticalDuckDB,

		SourceRowLedgerRootDigest:     strings.TrimSpace(input.SourceRowLedgerRootDigest),
		SourceRowLedgerRootSHA256:     strings.TrimSpace(input.SourceRowLedgerRootSHA256),
		SourceRowLedgerRootByteLength: input.SourceRowLedgerRootByteLength,
		SourceRowLedgerPageCount:      input.SourceRowLedgerPageCount,
		SourceRecordCount:             input.SourceRecordCount,
		AcceptedRecordCount:           input.AcceptedRecordCount,
		RejectedRecordCount:           input.RejectedRecordCount,
		DuplicateRecordCount:          input.DuplicateRecordCount,
	}
	if !acquiredAt.IsZero() {
		manifest.AcquiredAt = acquiredAt.Format(time.RFC3339Nano)
	}
	manifest.SourceManifestHash = datasetSnapshotSourceManifestHashV2(manifest)
	manifest.ManifestDigest = datasetSnapshotManifestDigestV2(manifest)
	if err := ValidateDatasetSnapshotManifestV2(manifest); err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	return manifest, nil
}

func ValidateDatasetSnapshotManifestV2(manifest DatasetSnapshotManifestV2) error {
	acquiredAt, acquiredAtErr := time.Parse(time.RFC3339Nano, manifest.AcquiredAt)
	if manifest.SchemaVersion != DatasetSnapshotManifestSchemaVersionV2 || manifest.Purpose != DatasetSnapshotManifestPurposeV2 ||
		ValidateDatasetSnapshotBindingKeyV1(manifest.Binding) != nil ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.AcquisitionMethod) || acquiredAtErr != nil || acquiredAt.IsZero() ||
		acquiredAt.UTC().Format(time.RFC3339Nano) != manifest.AcquiredAt || !isCanonicalSHA256Hex(manifest.AcquisitionActorDigest) ||
		!isCanonicalSHA256Hex(manifest.RawArtifactManifestDigest) || !isCanonicalSHA256Hex(manifest.RawArtifactManifestSHA256) ||
		!validDatasetSnapshotManifestByteLengthV2(manifest.RawArtifactManifestByteLength) || manifest.RawArtifactCount == 0 ||
		manifest.RawArtifactCount > maxJSONSafeUint64 ||
		!validDatasetSnapshotProducerContentReferenceV2(manifest) ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.SourceType) || !canonicalDatasetSnapshotManifestTextV2(manifest.ProducerPolicyID) ||
		!isCanonicalSHA256Hex(manifest.ProducerPolicyDigest) || !canonicalDatasetSnapshotManifestTextV2(manifest.ProducerComponentID) ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.ProducerComponentVersion) || !canonicalDatasetSnapshotManifestTextV2(manifest.ProducerOperation) ||
		!isCanonicalSHA256Hex(manifest.ProducerOperationSchemaHash) || !canonicalDatasetSnapshotManifestTextV2(manifest.ParserID) ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.ParserVersion) || !isCanonicalSHA256Hex(manifest.ParsedGenerationReceiptDigest) ||
		!isCanonicalSHA256Hex(manifest.ParsedGenerationReceiptSHA256) ||
		!validDatasetSnapshotManifestByteLengthV2(manifest.ParsedGenerationReceiptByteLength) ||
		!isCanonicalSHA256Hex(manifest.ClassificationLedgerDigest) || !isCanonicalSHA256Hex(manifest.ClassificationLedgerSHA256) ||
		!validDatasetSnapshotManifestByteLengthV2(manifest.ClassificationLedgerByteLength) ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.TimezoneSemantics) || !canonicalDatasetSnapshotManifestTextV2(manifest.CurrencySemantics) ||
		!isCanonicalSHA256Hex(manifest.SourceManifestHash) || !isCanonicalSHA256Hex(manifest.SourceRowLedgerRootDigest) ||
		!isCanonicalSHA256Hex(manifest.SourceRowLedgerRootSHA256) ||
		!validDatasetSnapshotSourceRowLedgerRootByteLengthV2(manifest.SourceRowLedgerRootByteLength) ||
		manifest.SourceRowLedgerPageCount > maxJSONSafeUint64 ||
		manifest.SourceRecordCount > maxJSONSafeUint64 ||
		manifest.AcceptedRecordCount > maxJSONSafeUint64 || manifest.RejectedRecordCount > maxJSONSafeUint64 ||
		manifest.DuplicateRecordCount > maxJSONSafeUint64 || !isCanonicalSHA256Hex(manifest.ManifestDigest) {
		return errors.New("dataset snapshot manifest v2 is incomplete")
	}
	if (manifest.SourceRecordCount == 0) != (manifest.SourceRowLedgerPageCount == 0) {
		return errors.New("dataset snapshot manifest v2 empty hierarchy is inconsistent")
	}
	if manifest.AnalyticalDuckDB != "" &&
		(manifest.ProducerContentContract != FundsProducerContentManifestContractV1 ||
			ValidateDatasetSnapshotAnalyticalDuckDBBindingV2(manifest.AnalyticalDuckDB) != nil) {
		return errors.New("dataset snapshot manifest v2 analytical DuckDB binding is invalid")
	}
	if manifest.AcceptedRecordCount > manifest.SourceRecordCount ||
		manifest.RejectedRecordCount > manifest.SourceRecordCount-manifest.AcceptedRecordCount ||
		manifest.DuplicateRecordCount != manifest.SourceRecordCount-manifest.AcceptedRecordCount-manifest.RejectedRecordCount {
		return errors.New("dataset snapshot manifest v2 record classification is inconsistent")
	}
	if manifest.SourceManifestHash != datasetSnapshotSourceManifestHashV2(manifest) {
		return errors.New("dataset snapshot manifest v2 source identity is invalid")
	}
	if manifest.ManifestDigest != datasetSnapshotManifestDigestV2(manifest) {
		return errors.New("dataset snapshot manifest v2 digest is invalid")
	}
	return nil
}

// ValidateDatasetSnapshotManifestV2FundsProducerContentV1 requires the exact
// canonical producer payload that the DSV2 manifest references. A non-empty or
// syntactically valid fpc1_ id is deliberately insufficient.
//
// This is still structural binding only. Factual authority additionally
// requires exact private-CAS readback, the signed authority record and index,
// and a fresh monotonic witness selected by the app layer.
func ValidateDatasetSnapshotManifestV2FundsProducerContentV1(
	manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV1,
) error {
	if ValidateDatasetSnapshotManifestV2(manifest) != nil ||
		ValidateFundsProducerContentManifestV1(producer) != nil ||
		manifest.Binding.CaseID != producer.CaseID ||
		manifest.RawArtifactManifestSHA256 != producer.RawManifestSHA256 ||
		manifest.SourceRecordCount != producer.NormalizedRowCount ||
		manifest.AcceptedRecordCount != producer.AcceptedRowCount ||
		manifest.RejectedRecordCount != producer.RejectedRowCount ||
		manifest.DuplicateRecordCount != producer.DuplicateRowCount {
		return errors.New("dataset snapshot funds producer content is invalid")
	}
	reference, err := newFundsProducerContentReferenceV1(producer)
	if err != nil || manifest.ProducerContentContract != reference.Contract ||
		manifest.ProducerContentID != reference.ID ||
		manifest.ProducerContentManifestSHA256 != reference.ManifestSHA256 ||
		manifest.ProducerContentManifestByteLength != reference.ManifestByteLength ||
		manifest.ProducerContentComponentID != reference.ComponentID ||
		manifest.ProducerContentComponentVersion != reference.ComponentVersion ||
		manifest.ProducerContentOperation != reference.Operation ||
		manifest.ProducerContentOperationSchemaHash != reference.OperationSchemaHash ||
		manifest.ProducerContentEngine != reference.Engine ||
		manifest.ProducerContentEncoder != reference.Encoder {
		return errors.New("dataset snapshot funds producer content binding mismatch")
	}
	return nil
}

// ValidateDatasetSnapshotManifestV2FundsProducerContentV2 binds the DSV2
// manifest to the exact canonical Go count-only producer payload, including
// the exact raw-artifact manifest SHA sealed by DSV2.
//
// This is structural binding only. Factual authority additionally requires
// exact private-CAS readback, the signed authority record and index, and a
// fresh monotonic witness selected by the app layer.
func ValidateDatasetSnapshotManifestV2FundsProducerContentV2(
	manifest DatasetSnapshotManifestV2,
	producer FundsProducerContentManifestV2,
) error {
	if ValidateDatasetSnapshotManifestV2(manifest) != nil ||
		ValidateFundsProducerContentManifestV2(producer) != nil ||
		manifest.Binding.CaseID != producer.CaseID ||
		manifest.RawArtifactManifestSHA256 != producer.RawArtifactManifestSHA256 ||
		manifest.SourceRecordCount != producer.SourceRowCount ||
		manifest.AcceptedRecordCount != producer.AcceptedRowCount ||
		manifest.RejectedRecordCount != producer.RejectedRowCount ||
		manifest.DuplicateRecordCount != producer.DuplicateRowCount {
		return errors.New("dataset snapshot funds producer content v2 is invalid")
	}
	reference, err := newFundsProducerContentReferenceV2(producer)
	if err != nil || manifest.ProducerContentContract != reference.Contract ||
		manifest.ProducerContentID != reference.ID ||
		manifest.ProducerContentManifestSHA256 != reference.ManifestSHA256 ||
		manifest.ProducerContentManifestByteLength != reference.ManifestByteLength ||
		manifest.ProducerContentComponentID != reference.ComponentID ||
		manifest.ProducerContentComponentVersion != reference.ComponentVersion ||
		manifest.ProducerContentOperation != reference.Operation ||
		manifest.ProducerContentOperationSchemaHash != reference.OperationSchemaHash ||
		manifest.ProducerContentEngine != reference.Engine ||
		manifest.ProducerContentEncoder != reference.Encoder {
		return errors.New("dataset snapshot funds producer content v2 binding mismatch")
	}
	return nil
}

func ParseDatasetSnapshotManifestV2(raw []byte) (DatasetSnapshotManifestV2, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxDatasetSnapshotManifestBytesV2, MaxDepth: 8, MaxTokens: 512, MaxStringBytes: 32 * 1024,
	}); err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest DatasetSnapshotManifestV2
	if err := decoder.Decode(&manifest); err != nil {
		return DatasetSnapshotManifestV2{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DatasetSnapshotManifestV2{}, errors.New("dataset snapshot manifest v2 contains trailing JSON")
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(raw, canonical) {
		return DatasetSnapshotManifestV2{}, errors.New("dataset snapshot manifest v2 is not canonically encoded")
	}
	return manifest, ValidateDatasetSnapshotManifestV2(manifest)
}

func DatasetSnapshotManifestV2Bytes(manifest DatasetSnapshotManifestV2) ([]byte, error) {
	if err := ValidateDatasetSnapshotManifestV2(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func DatasetSnapshotManifestV2SHA256(manifest DatasetSnapshotManifestV2) (string, error) {
	body, err := DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		return "", err
	}
	return SHA256Hex(body), nil
}

type datasetSnapshotSourceManifestIdentityV2 struct {
	SchemaVersion                      int                                      `json:"schemaVersion"`
	Purpose                            string                                   `json:"purpose"`
	BindingKeyDigest                   string                                   `json:"bindingKeyDigest"`
	AcquisitionMethod                  string                                   `json:"acquisitionMethod"`
	AcquiredAt                         string                                   `json:"acquiredAt"`
	AcquisitionActorDigest             string                                   `json:"acquisitionActorDigest"`
	RawArtifactManifestDigest          string                                   `json:"rawArtifactManifestDigest"`
	RawArtifactManifestSHA256          string                                   `json:"rawArtifactManifestSha256"`
	RawArtifactManifestByteLength      uint64                                   `json:"rawArtifactManifestByteLength"`
	RawArtifactCount                   uint64                                   `json:"rawArtifactCount"`
	ProducerContentContract            string                                   `json:"producerContentContract"`
	ProducerContentID                  string                                   `json:"producerContentId"`
	ProducerContentManifestSHA256      string                                   `json:"producerContentManifestSha256"`
	ProducerContentManifestByteLength  uint64                                   `json:"producerContentManifestByteLength"`
	ProducerContentComponentID         string                                   `json:"producerContentComponentId"`
	ProducerContentComponentVersion    string                                   `json:"producerContentComponentVersion"`
	ProducerContentOperation           string                                   `json:"producerContentOperation"`
	ProducerContentOperationSchemaHash string                                   `json:"producerContentOperationSchemaHash"`
	ProducerContentEngine              string                                   `json:"producerContentEngine"`
	ProducerContentEncoder             string                                   `json:"producerContentEncoder"`
	SourceType                         string                                   `json:"sourceType"`
	ProducerPolicyID                   string                                   `json:"producerPolicyId"`
	ProducerPolicyDigest               string                                   `json:"producerPolicyDigest"`
	ProducerComponentID                string                                   `json:"producerComponentId"`
	ProducerComponentVersion           string                                   `json:"producerComponentVersion"`
	ProducerOperation                  string                                   `json:"producerOperation"`
	ProducerOperationSchemaHash        string                                   `json:"producerOperationSchemaHash"`
	ParserID                           string                                   `json:"parserId"`
	ParserVersion                      string                                   `json:"parserVersion"`
	ParsedGenerationReceiptDigest      string                                   `json:"parsedGenerationReceiptDigest"`
	ParsedGenerationReceiptSHA256      string                                   `json:"parsedGenerationReceiptSha256"`
	ParsedGenerationReceiptByteLength  uint64                                   `json:"parsedGenerationReceiptByteLength"`
	ClassificationLedgerDigest         string                                   `json:"classificationLedgerDigest"`
	ClassificationLedgerSHA256         string                                   `json:"classificationLedgerSha256"`
	ClassificationLedgerByteLength     uint64                                   `json:"classificationLedgerByteLength"`
	TimezoneSemantics                  string                                   `json:"timezoneSemantics"`
	CurrencySemantics                  string                                   `json:"currencySemantics"`
	AnalyticalDuckDB                   DatasetSnapshotAnalyticalDuckDBBindingV2 `json:"analyticalDuckdb,omitempty"`
}

func datasetSnapshotSourceManifestHashV2(manifest DatasetSnapshotManifestV2) string {
	identity := datasetSnapshotSourceManifestIdentityV2{
		SchemaVersion: DatasetSnapshotManifestSchemaVersionV2, Purpose: "analytix.dataset-source-manifest/v2",
		BindingKeyDigest:  manifest.Binding.BindingKeyDigest,
		AcquisitionMethod: manifest.AcquisitionMethod, AcquiredAt: manifest.AcquiredAt,
		AcquisitionActorDigest:             manifest.AcquisitionActorDigest,
		RawArtifactManifestDigest:          manifest.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:          manifest.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:      manifest.RawArtifactManifestByteLength,
		RawArtifactCount:                   manifest.RawArtifactCount,
		ProducerContentContract:            manifest.ProducerContentContract,
		ProducerContentID:                  manifest.ProducerContentID,
		ProducerContentManifestSHA256:      manifest.ProducerContentManifestSHA256,
		ProducerContentManifestByteLength:  manifest.ProducerContentManifestByteLength,
		ProducerContentComponentID:         manifest.ProducerContentComponentID,
		ProducerContentComponentVersion:    manifest.ProducerContentComponentVersion,
		ProducerContentOperation:           manifest.ProducerContentOperation,
		ProducerContentOperationSchemaHash: manifest.ProducerContentOperationSchemaHash,
		ProducerContentEngine:              manifest.ProducerContentEngine,
		ProducerContentEncoder:             manifest.ProducerContentEncoder,
		SourceType:                         manifest.SourceType, ProducerPolicyID: manifest.ProducerPolicyID,
		ProducerPolicyDigest: manifest.ProducerPolicyDigest, ProducerComponentID: manifest.ProducerComponentID,
		ProducerComponentVersion: manifest.ProducerComponentVersion, ProducerOperation: manifest.ProducerOperation,
		ProducerOperationSchemaHash: manifest.ProducerOperationSchemaHash, ParserID: manifest.ParserID,
		ParserVersion: manifest.ParserVersion, ParsedGenerationReceiptDigest: manifest.ParsedGenerationReceiptDigest,
		ParsedGenerationReceiptSHA256:     manifest.ParsedGenerationReceiptSHA256,
		ParsedGenerationReceiptByteLength: manifest.ParsedGenerationReceiptByteLength,
		ClassificationLedgerDigest:        manifest.ClassificationLedgerDigest,
		ClassificationLedgerSHA256:        manifest.ClassificationLedgerSHA256,
		ClassificationLedgerByteLength:    manifest.ClassificationLedgerByteLength,
		TimezoneSemantics:                 manifest.TimezoneSemantics, CurrencySemantics: manifest.CurrencySemantics,
		AnalyticalDuckDB: manifest.AnalyticalDuckDB,
	}
	body, _ := json.Marshal(identity)
	return SHA256Hex(append(append([]byte(nil), datasetSnapshotSourceManifestHashDomainV2...), body...))
}

func datasetSnapshotManifestDigestV2(manifest DatasetSnapshotManifestV2) string {
	manifest.ManifestDigest = ""
	body, _ := json.Marshal(manifest)
	return SHA256Hex(append(append([]byte(nil), datasetSnapshotManifestDigestDomainV2...), body...))
}

func validDatasetSnapshotManifestByteLengthV2(value uint64) bool {
	return value > 0 && value <= maxDatasetSnapshotReferencedManifestBytesV2
}

func validFundsProducerContentManifestByteLengthV1(value uint64) bool {
	return value > 0 && value <= maxFundsProducerContentManifestBytesV1
}

func validFundsProducerContentManifestByteLengthV2(value uint64) bool {
	return value > 0 && value <= maxFundsProducerContentManifestBytesV2
}

func validDatasetSnapshotProducerContentReferenceV2(manifest DatasetSnapshotManifestV2) bool {
	if !isCanonicalSHA256Hex(manifest.ProducerContentManifestSHA256) {
		return false
	}
	switch manifest.ProducerContentContract {
	case FundsProducerContentManifestContractV1:
		return IsFundsProducerContentIDV1Syntax(manifest.ProducerContentID) &&
			validFundsProducerContentManifestByteLengthV1(manifest.ProducerContentManifestByteLength) &&
			manifest.ProducerContentComponentID == FundsProducerComponentIDV1 &&
			manifest.ProducerContentComponentVersion == FundsProducerComponentVersionV1 &&
			manifest.ProducerContentOperation == FundsProducerOperationV1 &&
			manifest.ProducerContentOperationSchemaHash == FundsProducerOperationSchemaHashV1 &&
			manifest.ProducerContentEngine == FundsProducerDuckDBVersionV1 &&
			manifest.ProducerContentEncoder == FundsProducerCanonicalEncoderV1
	case FundsProducerContentManifestContractV2:
		return IsFundsProducerContentIDV2Syntax(manifest.ProducerContentID) &&
			validFundsProducerContentManifestByteLengthV2(manifest.ProducerContentManifestByteLength) &&
			manifest.ProducerContentComponentID == FundsProducerComponentIDV2 &&
			manifest.ProducerContentComponentVersion == FundsProducerComponentVersionV2 &&
			manifest.ProducerContentOperation == FundsProducerOperationV2 &&
			manifest.ProducerContentOperationSchemaHash == FundsProducerOperationSchemaHashV2 &&
			manifest.ProducerContentEngine == FundsProducerEngineV2 &&
			manifest.ProducerContentEncoder == FundsProducerCanonicalEncoderV2
	default:
		return false
	}
}

func validDatasetSnapshotSourceRowLedgerRootByteLengthV2(value uint64) bool {
	return value > 0 && value <= maxDatasetSnapshotSourceRowLedgerRootBytesV2
}

func validateDatasetSnapshotAnalyticalDuckDBBindingWireV2(
	binding datasetSnapshotAnalyticalDuckDBBindingWireV2,
) error {
	if !isCanonicalSHA256Hex(binding.DuckDBSHA256) ||
		binding.DuckDBByteLength == 0 || binding.DuckDBByteLength > maxDatasetSnapshotAnalyticalDuckDBBytesV2 ||
		!isCanonicalSHA256Hex(binding.DuckDBContentSnapshotDigest) ||
		!isCanonicalSHA256Hex(binding.DuckDBSnapshotManifestSHA256) ||
		!canonicalFundsMaterializationIdentityV1(binding.MaterializationIdentity) ||
		!isCanonicalSHA256Hex(binding.SchemaDigest) ||
		!isCanonicalSHA256Hex(binding.QueryProfileDigest) ||
		binding.DatasetUTCOffsetMinutes < minDatasetSnapshotUTCOffsetMinutesV2 ||
		binding.DatasetUTCOffsetMinutes > maxDatasetSnapshotUTCOffsetMinutesV2 ||
		!canonicalDatasetSnapshotCurrencyV2(binding.ExpectedCurrency) ||
		binding.MinorUnitScale != datasetSnapshotAnalyticalMinorUnitScaleV2 {
		return errors.New("dataset snapshot analytical DuckDB binding is invalid")
	}
	return nil
}

func canonicalDatasetSnapshotCurrencyV2(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func canonicalDatasetSnapshotManifestTextV2(value string) bool {
	if value == "" || len(value) > 4*1024 || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}
