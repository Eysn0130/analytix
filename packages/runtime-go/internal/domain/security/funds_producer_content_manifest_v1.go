package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	FundsProducerContentManifestSchemaVersionV1 = 1
	FundsProducerContentManifestContractV1      = "analytix.funds-producer-content-manifest/v1"
	FundsProducerContentIDPrefixV1              = "fpc1_"

	FundsProducerComponentIDV1             = "analysis-compute"
	FundsProducerComponentVersionV1        = "0.1.0"
	FundsProducerOperationV1               = "materialize-txn-daily"
	FundsProducerOperationSchemaV1         = `{"additionalProperties":false,"properties":{"caseId":{"type":"string"},"sourceMaxId":{"minimum":0,"type":"integer"},"sourceMaxTxnTs":{"type":"string"},"sourceRevision":{"minimum":1,"type":"integer"},"sourceRowCount":{"minimum":0,"type":"integer"}},"required":["caseId","sourceMaxId","sourceMaxTxnTs","sourceRevision","sourceRowCount"],"type":"object"}`
	FundsProducerOperationSchemaHashV1     = "c8947b87b6962283023215b4b1e30c704b955d1bb7c8473cc1a0e76b7ba47f36"
	FundsProducerDuckDBVersionV1           = "v1.5.4"
	FundsProducerCanonicalEncoderV1        = "analytix.duckdb-content-manifest/v1"
	maxFundsProducerContentManifestBytesV1 = 128 * 1024
)

var fundsProducerContentDigestDomainV1 = []byte("AnalytixFundsProducerContentManifestV1\x00")

// FundsProducerContentManifestV1 is the exact canonical content payload
// emitted by the admitted funds materializer. It is a content identity, not a
// host dataset snapshot, source-readiness assertion, receipt, or publication
// authority. In particular, its fpc1_ id must never be substituted for a
// dsv2_ id.
//
// Field order is part of the cross-language canonical encoding and mirrors
// the Rust producer declaration.
type FundsProducerContentManifestV1 struct {
	AcceptedRowCount            uint64 `json:"acceptedRowCount"`
	AccountContentSHA256        string `json:"accountContentSha256"`
	AccountRowCount             uint64 `json:"accountRowCount"`
	AggregateContentSHA256      string `json:"aggregateContentSha256"`
	AggregateRowCount           uint64 `json:"aggregateRowCount"`
	CanonicalEncoder            string `json:"canonicalEncoder"`
	CaseID                      string `json:"caseId"`
	Contract                    string `json:"contract"`
	DetailContentSHA256         string `json:"detailContentSha256"`
	DetailRowCount              uint64 `json:"detailRowCount"`
	DuckDBVersion               string `json:"duckdbVersion"`
	DuplicateRowCount           uint64 `json:"duplicateRowCount"`
	KeywordContentSHA256        string `json:"keywordContentSha256"`
	KeywordRowCount             uint64 `json:"keywordRowCount"`
	NormalizedContentSHA256     string `json:"normalizedContentSha256"`
	NormalizedRowCount          uint64 `json:"normalizedRowCount"`
	ProducerComponentID         string `json:"producerComponentId"`
	ProducerComponentVersion    string `json:"producerComponentVersion"`
	ProducerOperation           string `json:"producerOperation"`
	ProducerOperationSchemaHash string `json:"producerOperationSchemaHash"`
	RawManifestSHA256           string `json:"rawManifestSha256"`
	RejectedRowCount            uint64 `json:"rejectedRowCount"`
	SchemaVersion               int    `json:"schemaVersion"`
	SourceRevision              uint64 `json:"sourceRevision"`
}

type FundsProducerContentManifestInputV1 struct {
	CaseID         string
	SourceRevision uint64

	RawManifestSHA256       string
	NormalizedContentSHA256 string
	DetailContentSHA256     string
	AggregateContentSHA256  string
	KeywordContentSHA256    string
	AccountContentSHA256    string

	NormalizedRowCount uint64
	AcceptedRowCount   uint64
	RejectedRowCount   uint64
	DuplicateRowCount  uint64
	DetailRowCount     uint64
	AggregateRowCount  uint64
	KeywordRowCount    uint64
	AccountRowCount    uint64
}

type fundsProducerContentReferenceV1 struct {
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

func NewFundsProducerContentManifestV1(input FundsProducerContentManifestInputV1) (FundsProducerContentManifestV1, error) {
	manifest := FundsProducerContentManifestV1{
		AcceptedRowCount:            input.AcceptedRowCount,
		AccountContentSHA256:        input.AccountContentSHA256,
		AccountRowCount:             input.AccountRowCount,
		AggregateContentSHA256:      input.AggregateContentSHA256,
		AggregateRowCount:           input.AggregateRowCount,
		CanonicalEncoder:            FundsProducerCanonicalEncoderV1,
		CaseID:                      input.CaseID,
		Contract:                    FundsProducerContentManifestContractV1,
		DetailContentSHA256:         input.DetailContentSHA256,
		DetailRowCount:              input.DetailRowCount,
		DuckDBVersion:               FundsProducerDuckDBVersionV1,
		DuplicateRowCount:           input.DuplicateRowCount,
		KeywordContentSHA256:        input.KeywordContentSHA256,
		KeywordRowCount:             input.KeywordRowCount,
		NormalizedContentSHA256:     input.NormalizedContentSHA256,
		NormalizedRowCount:          input.NormalizedRowCount,
		ProducerComponentID:         FundsProducerComponentIDV1,
		ProducerComponentVersion:    FundsProducerComponentVersionV1,
		ProducerOperation:           FundsProducerOperationV1,
		ProducerOperationSchemaHash: FundsProducerOperationSchemaHashV1,
		RawManifestSHA256:           input.RawManifestSHA256,
		RejectedRowCount:            input.RejectedRowCount,
		SchemaVersion:               FundsProducerContentManifestSchemaVersionV1,
		SourceRevision:              input.SourceRevision,
	}
	if err := ValidateFundsProducerContentManifestV1(manifest); err != nil {
		return FundsProducerContentManifestV1{}, err
	}
	return manifest, nil
}

func ValidateFundsProducerContentManifestV1(manifest FundsProducerContentManifestV1) error {
	if manifest.SchemaVersion != FundsProducerContentManifestSchemaVersionV1 ||
		manifest.Contract != FundsProducerContentManifestContractV1 ||
		manifest.ProducerComponentID != FundsProducerComponentIDV1 ||
		manifest.ProducerComponentVersion != FundsProducerComponentVersionV1 ||
		manifest.ProducerOperation != FundsProducerOperationV1 ||
		SHA256Hex([]byte(FundsProducerOperationSchemaV1)) != FundsProducerOperationSchemaHashV1 ||
		manifest.ProducerOperationSchemaHash != FundsProducerOperationSchemaHashV1 ||
		manifest.DuckDBVersion != FundsProducerDuckDBVersionV1 ||
		manifest.CanonicalEncoder != FundsProducerCanonicalEncoderV1 ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.CaseID) || manifest.CaseID == UnboundCaseID ||
		manifest.SourceRevision == 0 || manifest.SourceRevision > maxJSONSafeUint64 ||
		!isCanonicalSHA256Hex(manifest.RawManifestSHA256) ||
		!isCanonicalSHA256Hex(manifest.NormalizedContentSHA256) ||
		!isCanonicalSHA256Hex(manifest.DetailContentSHA256) ||
		!isCanonicalSHA256Hex(manifest.AggregateContentSHA256) ||
		!isCanonicalSHA256Hex(manifest.KeywordContentSHA256) ||
		!isCanonicalSHA256Hex(manifest.AccountContentSHA256) ||
		manifest.NormalizedRowCount > maxJSONSafeUint64 || manifest.AcceptedRowCount > maxJSONSafeUint64 ||
		manifest.RejectedRowCount > maxJSONSafeUint64 || manifest.DuplicateRowCount > maxJSONSafeUint64 ||
		manifest.DetailRowCount > maxJSONSafeUint64 || manifest.AggregateRowCount > maxJSONSafeUint64 ||
		manifest.KeywordRowCount > maxJSONSafeUint64 || manifest.AccountRowCount > maxJSONSafeUint64 {
		return errors.New("funds producer content manifest v1 is incomplete")
	}
	if manifest.AcceptedRowCount > manifest.NormalizedRowCount ||
		manifest.RejectedRowCount > manifest.NormalizedRowCount-manifest.AcceptedRowCount ||
		manifest.DuplicateRowCount != manifest.NormalizedRowCount-manifest.AcceptedRowCount-manifest.RejectedRowCount ||
		manifest.DetailRowCount != manifest.AcceptedRowCount {
		return errors.New("funds producer content manifest v1 row coverage is inconsistent")
	}
	return nil
}

func ParseFundsProducerContentManifestV1(raw []byte) (FundsProducerContentManifestV1, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxFundsProducerContentManifestBytesV1, MaxDepth: 4, MaxTokens: 256, MaxStringBytes: 32 * 1024,
	}); err != nil {
		return FundsProducerContentManifestV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest FundsProducerContentManifestV1
	if err := decoder.Decode(&manifest); err != nil {
		return FundsProducerContentManifestV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return FundsProducerContentManifestV1{}, errors.New("funds producer content manifest v1 contains trailing JSON")
	}
	if err := ValidateFundsProducerContentManifestV1(manifest); err != nil {
		return FundsProducerContentManifestV1{}, err
	}
	canonical, err := fundsProducerContentManifestV1Bytes(manifest)
	if err != nil || !bytes.Equal(raw, canonical) {
		return FundsProducerContentManifestV1{}, errors.New("funds producer content manifest v1 is not canonically encoded")
	}
	return manifest, nil
}

func FundsProducerContentManifestV1Bytes(manifest FundsProducerContentManifestV1) ([]byte, error) {
	if err := ValidateFundsProducerContentManifestV1(manifest); err != nil {
		return nil, err
	}
	return fundsProducerContentManifestV1Bytes(manifest)
}

func FundsProducerContentManifestV1SHA256(manifest FundsProducerContentManifestV1) (string, error) {
	body, err := FundsProducerContentManifestV1Bytes(manifest)
	if err != nil {
		return "", err
	}
	return SHA256Hex(body), nil
}

func DeriveFundsProducerContentIDV1(manifest FundsProducerContentManifestV1) string {
	body, err := FundsProducerContentManifestV1Bytes(manifest)
	if err != nil {
		return ""
	}
	material := append(append([]byte(nil), fundsProducerContentDigestDomainV1...), body...)
	return FundsProducerContentIDPrefixV1 + SHA256Hex(material)
}

// IsFundsProducerContentIDV1Syntax recognizes only the reserved identifier
// shape. It does not establish content equality, host admission, currentness,
// source readiness, evidence, or fact authority.
func IsFundsProducerContentIDV1Syntax(value string) bool {
	return strings.HasPrefix(value, FundsProducerContentIDPrefixV1) &&
		isCanonicalSHA256Hex(strings.TrimPrefix(value, FundsProducerContentIDPrefixV1))
}

func newFundsProducerContentReferenceV1(
	manifest FundsProducerContentManifestV1,
) (fundsProducerContentReferenceV1, error) {
	body, err := FundsProducerContentManifestV1Bytes(manifest)
	if err != nil {
		return fundsProducerContentReferenceV1{}, err
	}
	id := DeriveFundsProducerContentIDV1(manifest)
	if !IsFundsProducerContentIDV1Syntax(id) {
		return fundsProducerContentReferenceV1{}, errors.New("funds producer content id is invalid")
	}
	return fundsProducerContentReferenceV1{
		CaseID:              manifest.CaseID,
		Contract:            manifest.Contract,
		ID:                  id,
		ManifestSHA256:      SHA256Hex(body),
		ManifestByteLength:  uint64(len(body)),
		ComponentID:         manifest.ProducerComponentID,
		ComponentVersion:    manifest.ProducerComponentVersion,
		Operation:           manifest.ProducerOperation,
		OperationSchemaHash: manifest.ProducerOperationSchemaHash,
		Engine:              manifest.DuckDBVersion,
		Encoder:             manifest.CanonicalEncoder,
	}, nil
}

func fundsProducerContentManifestV1Bytes(manifest FundsProducerContentManifestV1) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	body := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	// encoding/json always escapes these two valid JSON code points while
	// serde_json emits UTF-8. Normalize them so the Go identity is byte-equal
	// to the producer identity for the complete admitted input domain.
	body = bytes.ReplaceAll(body, []byte(`\u2028`), []byte("\u2028"))
	body = bytes.ReplaceAll(body, []byte(`\u2029`), []byte("\u2029"))
	return body, nil
}
