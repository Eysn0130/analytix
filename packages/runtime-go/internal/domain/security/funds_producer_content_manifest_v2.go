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
	FundsProducerContentManifestSchemaVersionV2 = 2
	FundsProducerContentManifestContractV2      = "analytix.funds-producer-content-manifest/v2"
	FundsProducerContentIDPrefixV2              = "fpc2_"

	FundsProducerCapabilityCountCaseRowsV2 = "count_case_rows"
	FundsProducerComponentIDV2             = "runtime-go"
	FundsProducerComponentVersionV2        = "1.0.0"
	FundsProducerEngineV2                  = "analytix.runtime-go.strict-csv"
	FundsProducerOperationV2               = "build-count-case-rows-snapshot"
	FundsProducerOperationSchemaV2         = `{"additionalProperties":false,"properties":{"caseId":{"type":"string"},"rawArtifactManifestSha256":{"pattern":"^[0-9a-f]{64}$","type":"string"},"sourceRevision":{"minimum":1,"type":"integer"},"sourceRowCount":{"minimum":0,"type":"integer"}},"required":["caseId","rawArtifactManifestSha256","sourceRevision","sourceRowCount"],"type":"object"}`
	FundsProducerOperationSchemaHashV2     = "361bb839d98ba47106a0d8cab020de70552cf1290557718f5827d7ab4e328988"
	FundsProducerParserIDV2                = "analytix.strict-utf8-csv"
	FundsProducerParserVersionV2           = "1"
	FundsProducerTransformationIDV2        = "analytix.count-case-rows"
	FundsProducerTransformationVersionV2   = "1"
	FundsProducerCanonicalEncoderV2        = "analytix.go-funds-content-manifest/v2"

	maxFundsProducerContentManifestBytesV2 = 64 * 1024
)

var fundsProducerContentDigestDomainV2 = []byte("AnalytixFundsProducerContentManifestV2\x00")

// FundsProducerContentManifestV2 is the PII-free content identity emitted by
// the exact Go strict-CSV count-only producer. It is not dataset, execution,
// receipt, currentness, or publication authority.
//
// Field order is part of the canonical encoding.
type FundsProducerContentManifestV2 struct {
	AcceptedRowCount            uint64    `json:"acceptedRowCount"`
	CanonicalEncoder            string    `json:"canonicalEncoder"`
	Capabilities                [1]string `json:"capabilities"`
	CaseID                      string    `json:"caseId"`
	Contract                    string    `json:"contract"`
	DetailContentSHA256         string    `json:"detailContentSha256"`
	DetailRowCount              uint64    `json:"detailRowCount"`
	DuplicateRowCount           uint64    `json:"duplicateRowCount"`
	NormalizedContentSHA256     string    `json:"normalizedContentSha256"`
	ParserID                    string    `json:"parserId"`
	ParserVersion               string    `json:"parserVersion"`
	ProducerComponentID         string    `json:"producerComponentId"`
	ProducerComponentVersion    string    `json:"producerComponentVersion"`
	ProducerEngine              string    `json:"producerEngine"`
	ProducerOperation           string    `json:"producerOperation"`
	ProducerOperationSchemaHash string    `json:"producerOperationSchemaHash"`
	RawArtifactManifestSHA256   string    `json:"rawArtifactManifestSha256"`
	RejectedRowCount            uint64    `json:"rejectedRowCount"`
	SchemaVersion               int       `json:"schemaVersion"`
	SourceRevision              uint64    `json:"sourceRevision"`
	SourceRowCount              uint64    `json:"sourceRowCount"`
	TransformationID            string    `json:"transformationId"`
	TransformationVersion       string    `json:"transformationVersion"`
}

type FundsProducerContentManifestInputV2 struct {
	CaseID                    string
	SourceRevision            uint64
	RawArtifactManifestSHA256 string
	NormalizedContentSHA256   string
	DetailContentSHA256       string
	SourceRowCount            uint64
	AcceptedRowCount          uint64
	RejectedRowCount          uint64
	DuplicateRowCount         uint64
	DetailRowCount            uint64
}

type fundsProducerContentReferenceV2 struct {
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
	Capability          string
}

func NewFundsProducerContentManifestV2(
	input FundsProducerContentManifestInputV2,
) (FundsProducerContentManifestV2, error) {
	manifest := FundsProducerContentManifestV2{
		AcceptedRowCount:            input.AcceptedRowCount,
		CanonicalEncoder:            FundsProducerCanonicalEncoderV2,
		Capabilities:                [1]string{FundsProducerCapabilityCountCaseRowsV2},
		CaseID:                      input.CaseID,
		Contract:                    FundsProducerContentManifestContractV2,
		DetailContentSHA256:         input.DetailContentSHA256,
		DetailRowCount:              input.DetailRowCount,
		DuplicateRowCount:           input.DuplicateRowCount,
		NormalizedContentSHA256:     input.NormalizedContentSHA256,
		ParserID:                    FundsProducerParserIDV2,
		ParserVersion:               FundsProducerParserVersionV2,
		ProducerComponentID:         FundsProducerComponentIDV2,
		ProducerComponentVersion:    FundsProducerComponentVersionV2,
		ProducerEngine:              FundsProducerEngineV2,
		ProducerOperation:           FundsProducerOperationV2,
		ProducerOperationSchemaHash: FundsProducerOperationSchemaHashV2,
		RawArtifactManifestSHA256:   input.RawArtifactManifestSHA256,
		RejectedRowCount:            input.RejectedRowCount,
		SchemaVersion:               FundsProducerContentManifestSchemaVersionV2,
		SourceRevision:              input.SourceRevision,
		SourceRowCount:              input.SourceRowCount,
		TransformationID:            FundsProducerTransformationIDV2,
		TransformationVersion:       FundsProducerTransformationVersionV2,
	}
	if err := ValidateFundsProducerContentManifestV2(manifest); err != nil {
		return FundsProducerContentManifestV2{}, err
	}
	return manifest, nil
}

func ValidateFundsProducerContentManifestV2(manifest FundsProducerContentManifestV2) error {
	if manifest.SchemaVersion != FundsProducerContentManifestSchemaVersionV2 ||
		manifest.Contract != FundsProducerContentManifestContractV2 ||
		manifest.Capabilities != [1]string{FundsProducerCapabilityCountCaseRowsV2} ||
		manifest.ProducerComponentID != FundsProducerComponentIDV2 ||
		manifest.ProducerComponentVersion != FundsProducerComponentVersionV2 ||
		manifest.ProducerEngine != FundsProducerEngineV2 ||
		manifest.ProducerOperation != FundsProducerOperationV2 ||
		SHA256Hex([]byte(FundsProducerOperationSchemaV2)) != FundsProducerOperationSchemaHashV2 ||
		manifest.ProducerOperationSchemaHash != FundsProducerOperationSchemaHashV2 ||
		manifest.ParserID != FundsProducerParserIDV2 ||
		manifest.ParserVersion != FundsProducerParserVersionV2 ||
		manifest.TransformationID != FundsProducerTransformationIDV2 ||
		manifest.TransformationVersion != FundsProducerTransformationVersionV2 ||
		manifest.CanonicalEncoder != FundsProducerCanonicalEncoderV2 ||
		!canonicalDatasetSnapshotManifestTextV2(manifest.CaseID) ||
		manifest.CaseID == UnboundCaseID ||
		manifest.SourceRevision == 0 ||
		manifest.SourceRevision > maxJSONSafeUint64 ||
		!isCanonicalSHA256Hex(manifest.RawArtifactManifestSHA256) ||
		!isCanonicalSHA256Hex(manifest.NormalizedContentSHA256) ||
		!isCanonicalSHA256Hex(manifest.DetailContentSHA256) ||
		manifest.SourceRowCount > maxJSONSafeUint64 ||
		manifest.AcceptedRowCount > maxJSONSafeUint64 ||
		manifest.RejectedRowCount > maxJSONSafeUint64 ||
		manifest.DuplicateRowCount > maxJSONSafeUint64 ||
		manifest.DetailRowCount > maxJSONSafeUint64 {
		return errors.New("funds producer content manifest v2 is incomplete")
	}
	if manifest.AcceptedRowCount > manifest.SourceRowCount ||
		manifest.RejectedRowCount > manifest.SourceRowCount-manifest.AcceptedRowCount ||
		manifest.DuplicateRowCount != manifest.SourceRowCount-manifest.AcceptedRowCount-manifest.RejectedRowCount ||
		manifest.DetailRowCount != manifest.AcceptedRowCount {
		return errors.New("funds producer content manifest v2 row coverage is inconsistent")
	}
	return nil
}

func ParseFundsProducerContentManifestV2(raw []byte) (FundsProducerContentManifestV2, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxFundsProducerContentManifestBytesV2,
		MaxDepth:       4,
		MaxTokens:      256,
		MaxStringBytes: 32 * 1024,
	}); err != nil {
		return FundsProducerContentManifestV2{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest FundsProducerContentManifestV2
	if err := decoder.Decode(&manifest); err != nil {
		return FundsProducerContentManifestV2{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return FundsProducerContentManifestV2{}, errors.New("funds producer content manifest v2 contains trailing JSON")
	}
	if err := ValidateFundsProducerContentManifestV2(manifest); err != nil {
		return FundsProducerContentManifestV2{}, err
	}
	canonical, err := fundsProducerContentManifestV2Bytes(manifest)
	if err != nil || !bytes.Equal(raw, canonical) {
		return FundsProducerContentManifestV2{}, errors.New("funds producer content manifest v2 is not canonically encoded")
	}
	return manifest, nil
}

func FundsProducerContentManifestV2Bytes(manifest FundsProducerContentManifestV2) ([]byte, error) {
	if err := ValidateFundsProducerContentManifestV2(manifest); err != nil {
		return nil, err
	}
	return fundsProducerContentManifestV2Bytes(manifest)
}

func FundsProducerContentManifestV2SHA256(manifest FundsProducerContentManifestV2) (string, error) {
	body, err := FundsProducerContentManifestV2Bytes(manifest)
	if err != nil {
		return "", err
	}
	return SHA256Hex(body), nil
}

func DeriveFundsProducerContentIDV2(manifest FundsProducerContentManifestV2) string {
	body, err := FundsProducerContentManifestV2Bytes(manifest)
	if err != nil {
		return ""
	}
	material := append(append([]byte(nil), fundsProducerContentDigestDomainV2...), body...)
	return FundsProducerContentIDPrefixV2 + SHA256Hex(material)
}

// IsFundsProducerContentIDV2Syntax recognizes only the reserved identifier
// shape. It does not establish content equality, dataset admission,
// currentness, evidence, receipt, or publication authority.
func IsFundsProducerContentIDV2Syntax(value string) bool {
	return strings.HasPrefix(value, FundsProducerContentIDPrefixV2) &&
		isCanonicalSHA256Hex(strings.TrimPrefix(value, FundsProducerContentIDPrefixV2))
}

func newFundsProducerContentReferenceV2(
	manifest FundsProducerContentManifestV2,
) (fundsProducerContentReferenceV2, error) {
	body, err := FundsProducerContentManifestV2Bytes(manifest)
	if err != nil {
		return fundsProducerContentReferenceV2{}, err
	}
	id := DeriveFundsProducerContentIDV2(manifest)
	if !IsFundsProducerContentIDV2Syntax(id) {
		return fundsProducerContentReferenceV2{}, errors.New("funds producer content id v2 is invalid")
	}
	return fundsProducerContentReferenceV2{
		CaseID:              manifest.CaseID,
		Contract:            manifest.Contract,
		ID:                  id,
		ManifestSHA256:      SHA256Hex(body),
		ManifestByteLength:  uint64(len(body)),
		ComponentID:         manifest.ProducerComponentID,
		ComponentVersion:    manifest.ProducerComponentVersion,
		Operation:           manifest.ProducerOperation,
		OperationSchemaHash: manifest.ProducerOperationSchemaHash,
		Engine:              manifest.ProducerEngine,
		Encoder:             manifest.CanonicalEncoder,
		Capability:          manifest.Capabilities[0],
	}, nil
}

func fundsProducerContentManifestV2Bytes(manifest FundsProducerContentManifestV2) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	body := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	body = bytes.ReplaceAll(body, []byte(`\u2028`), []byte("\u2028"))
	body = bytes.ReplaceAll(body, []byte(`\u2029`), []byte("\u2029"))
	return body, nil
}
