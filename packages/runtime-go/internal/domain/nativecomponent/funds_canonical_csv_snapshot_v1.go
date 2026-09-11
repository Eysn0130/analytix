package nativecomponent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OperationFundsBuildCanonicalCSVSnapshotV1 = "funds.build_canonical_csv_snapshot_v1"
	FundsCanonicalDirectCSVProfileV1          = "canonical_direct_csv_v1"
	FundsCanonicalCSVRowHashContractV1        = "sha256(analytix.funds-canonical-typed-raw-occurrence/digest/v1\\0+go-json-v1)"

	FundsCanonicalCSVMaximumSourceBytesV1  = uint64(64 * 1024 * 1024)
	FundsCanonicalCSVMaximumSourceRowsV1   = uint64(100_000)
	FundsCanonicalCSVMaximumRequestBytesV1 = 16 * 1024
	FundsCanonicalCSVMaximumResultBytesV1  = 2 * 1024 * 1024
	FundsCanonicalCSVMaximumDurationV1     = 2 * time.Minute

	fundsCanonicalCSVPrivateImportFileIDBytesV1 = 20
	fundsCanonicalCSVMaximumCaseIDBytesV1       = 512
	fundsCanonicalCSVMaximumJSONIntegerV1       = uint64(9_007_199_254_740_991)
)

var errFundsCanonicalCSVSnapshotPrivateV1 = errors.New(
	"funds canonical CSV snapshot value is host-private",
)

// FundsCanonicalCSVSnapshotBuildInputV1 binds the staging-only native build
// to one exact case, raw graph and source object. It is not DSV2, query,
// evidence, publication, or PII-display authority.
type FundsCanonicalCSVSnapshotBuildInputV1 struct {
	Binding                   domainsecurity.DatasetSnapshotBindingKeyV1
	PrivateImportFileID       string
	SourceRevision            uint64
	RawArtifactManifestSHA256 string
	SourceArtifactSHA256      string
	SourceArtifactByteLength  uint64
	SourceRowCount            uint64
}

// FundsCanonicalCSVSnapshotBuildArgumentsV1 is deliberately opaque. Only the
// fixed native encoder may reveal its bounded staging payload to the pinned
// data-engine process.
type FundsCanonicalCSVSnapshotBuildArgumentsV1 struct {
	caseID                    string
	bindingKeyDigest          string
	privateImportFileID       string
	sourceRevision            uint64
	rawArtifactManifestSHA256 string
	sourceArtifactSHA256      string
	sourceArtifactByteLength  uint64
	sourceRowCount            uint64
}

type fundsCanonicalCSVSnapshotBuildPayloadWireV1 struct {
	CaseID                    string `json:"caseId"`
	Profile                   string `json:"profile"`
	PrivateImportFileID       string `json:"privateImportFileId"`
	SourceRevision            uint64 `json:"sourceRevision"`
	RawArtifactManifestSHA256 string `json:"rawArtifactManifestSha256"`
}

type fundsCanonicalCSVSnapshotBuildRequestWireV1 struct {
	RequestID string                                      `json:"request_id"`
	Command   string                                      `json:"command"`
	CaseID    string                                      `json:"case_id"`
	Payload   fundsCanonicalCSVSnapshotBuildPayloadWireV1 `json:"payload"`
}

func NewFundsCanonicalCSVSnapshotBuildArgumentsV1(
	input FundsCanonicalCSVSnapshotBuildInputV1,
) (FundsCanonicalCSVSnapshotBuildArgumentsV1, error) {
	arguments := FundsCanonicalCSVSnapshotBuildArgumentsV1{
		caseID:                    input.Binding.CaseID,
		bindingKeyDigest:          input.Binding.BindingKeyDigest,
		privateImportFileID:       input.PrivateImportFileID,
		sourceRevision:            input.SourceRevision,
		rawArtifactManifestSHA256: input.RawArtifactManifestSHA256,
		sourceArtifactSHA256:      input.SourceArtifactSHA256,
		sourceArtifactByteLength:  input.SourceArtifactByteLength,
		sourceRowCount:            input.SourceRowCount,
	}
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(input.Binding) != nil ||
		validateFundsCanonicalCSVSnapshotBuildArgumentsV1(arguments) != nil {
		return FundsCanonicalCSVSnapshotBuildArgumentsV1{}, ErrRequestInvalid
	}
	return arguments, nil
}

func (FundsCanonicalCSVSnapshotBuildArgumentsV1) MarshalJSON() ([]byte, error) {
	return nil, errFundsCanonicalCSVSnapshotPrivateV1
}

func (*FundsCanonicalCSVSnapshotBuildArgumentsV1) UnmarshalJSON([]byte) error {
	return errFundsCanonicalCSVSnapshotPrivateV1
}

func (arguments FundsCanonicalCSVSnapshotBuildArgumentsV1) String() string {
	return fmt.Sprintf(
		"FundsCanonicalCSVSnapshotBuildArgumentsV1{sourceRows:%d,sourceBytes:%d,privateBindings:[REDACTED]}",
		arguments.sourceRowCount,
		arguments.sourceArtifactByteLength,
	)
}

func (arguments FundsCanonicalCSVSnapshotBuildArgumentsV1) GoString() string {
	return arguments.String()
}

func (arguments FundsCanonicalCSVSnapshotBuildArgumentsV1) CaseIDV1() string {
	if validateFundsCanonicalCSVSnapshotBuildArgumentsV1(arguments) != nil {
		return ""
	}
	return arguments.caseID
}

func (arguments FundsCanonicalCSVSnapshotBuildArgumentsV1) SourceIdentityV1() (string, uint64) {
	if validateFundsCanonicalCSVSnapshotBuildArgumentsV1(arguments) != nil {
		return "", 0
	}
	return arguments.sourceArtifactSHA256, arguments.sourceArtifactByteLength
}

func validateFundsCanonicalCSVSnapshotBuildArgumentsV1(
	arguments FundsCanonicalCSVSnapshotBuildArgumentsV1,
) error {
	if !canonicalFundsCanonicalCSVCaseIDV1(arguments.caseID) ||
		!canonicalFundsCanonicalCSVDigestV1(arguments.bindingKeyDigest) ||
		!canonicalFundsCanonicalCSVPrivateImportFileIDV1(arguments.privateImportFileID) ||
		arguments.sourceRevision == 0 ||
		arguments.sourceRevision > fundsCanonicalCSVMaximumJSONIntegerV1 ||
		!canonicalFundsCanonicalCSVDigestV1(arguments.rawArtifactManifestSHA256) ||
		!canonicalFundsCanonicalCSVDigestV1(arguments.sourceArtifactSHA256) ||
		arguments.sourceArtifactByteLength == 0 ||
		arguments.sourceArtifactByteLength > FundsCanonicalCSVMaximumSourceBytesV1 ||
		arguments.sourceRowCount == 0 ||
		arguments.sourceRowCount > FundsCanonicalCSVMaximumSourceRowsV1 {
		return ErrRequestInvalid
	}
	return nil
}

// EncodeFundsBuildCanonicalCSVSnapshotNativeRequestFrameV1 emits no source or
// output path. The process authority supplies both exact private objects as
// inherited descriptor capabilities outside the JSON protocol.
func EncodeFundsBuildCanonicalCSVSnapshotNativeRequestFrameV1(
	requestID string,
	arguments FundsCanonicalCSVSnapshotBuildArgumentsV1,
) ([]byte, error) {
	if !canonicalFundsCanonicalCSVDigestV1(requestID) ||
		validateFundsCanonicalCSVSnapshotBuildArgumentsV1(arguments) != nil {
		return nil, ErrRequestInvalid
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fundsCanonicalCSVSnapshotBuildRequestWireV1{
		RequestID: requestID,
		Command:   OperationFundsBuildCanonicalCSVSnapshotV1,
		CaseID:    arguments.caseID,
		Payload: fundsCanonicalCSVSnapshotBuildPayloadWireV1{
			CaseID:                    arguments.caseID,
			Profile:                   FundsCanonicalDirectCSVProfileV1,
			PrivateImportFileID:       arguments.privateImportFileID,
			SourceRevision:            arguments.sourceRevision,
			RawArtifactManifestSHA256: arguments.rawArtifactManifestSHA256,
		},
	}); err != nil || output.Len() == 0 ||
		output.Len() > FundsCanonicalCSVMaximumRequestBytesV1 {
		return nil, ErrRequestInvalid
	}
	frame := output.Bytes()
	if frame[len(frame)-1] != '\n' || domainjsonstrict.Validate(
		frame[:len(frame)-1],
		domainjsonstrict.Options{
			RequireObject:  true,
			MaxBytes:       FundsCanonicalCSVMaximumRequestBytesV1 - 1,
			MaxDepth:       4,
			MaxTokens:      32,
			MaxStringBytes: fundsCanonicalCSVMaximumCaseIDBytesV1,
			MaxNumberBytes: 32,
			MaxAbsExponent: 1,
		},
	) != nil {
		return nil, ErrRequestInvalid
	}
	return append([]byte(nil), frame...), nil
}

type fundsCanonicalCSVSnapshotBuildResultWireV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Operation                string `json:"operation"`
	Profile                  string `json:"profile"`
	PrivateImportFileID      string `json:"privateImportFileId"`
	SourceArtifactSHA256     string `json:"sourceArtifactSha256"`
	SourceArtifactByteLength uint64 `json:"sourceArtifactByteLength"`
	SourceRowCount           uint64 `json:"sourceRowCount"`
	SourceMaxTxnTS           string `json:"sourceMaxTxnTs"`
	SourceMaxID              uint64 `json:"sourceMaxId"`
	RowHashContract          string `json:"rowHashContract"`

	domainsecurity.FundsMaterializationResultV1
}

// FundsCanonicalCSVSnapshotBuildResultV1 carries only host-private hashes,
// counts and the already-existing FPC1 materialization contract. It contains
// no source path or complete source-row value and has no JSON representation.
type FundsCanonicalCSVSnapshotBuildResultV1 struct {
	materialization          domainsecurity.FundsMaterializationResultV1
	sourceArtifactSHA256     string
	sourceArtifactByteLength uint64
	sourceRowCount           uint64
	sourceMaxTxnTS           string
	sourceMaxID              uint64
}

func (FundsCanonicalCSVSnapshotBuildResultV1) MarshalJSON() ([]byte, error) {
	return nil, errFundsCanonicalCSVSnapshotPrivateV1
}

func (*FundsCanonicalCSVSnapshotBuildResultV1) UnmarshalJSON([]byte) error {
	return errFundsCanonicalCSVSnapshotPrivateV1
}

func (result FundsCanonicalCSVSnapshotBuildResultV1) String() string {
	return fmt.Sprintf(
		"FundsCanonicalCSVSnapshotBuildResultV1{sourceRows:%d,sourceBytes:%d,privateBindings:[REDACTED]}",
		result.sourceRowCount,
		result.sourceArtifactByteLength,
	)
}

func (result FundsCanonicalCSVSnapshotBuildResultV1) GoString() string {
	return result.String()
}

func (result FundsCanonicalCSVSnapshotBuildResultV1) MaterializationV1() (
	domainsecurity.FundsMaterializationResultV1,
	error,
) {
	if validateFundsCanonicalCSVSnapshotBuildResultV1(result) != nil {
		return domainsecurity.FundsMaterializationResultV1{}, ErrResultInvalid
	}
	body, err := json.Marshal(result.materialization)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, ErrResultInvalid
	}
	materialization, err := domainsecurity.ParseFundsMaterializationResultV1(body)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, ErrResultInvalid
	}
	return materialization, nil
}

func (result FundsCanonicalCSVSnapshotBuildResultV1) SourceStatsV1() (
	sha256 string,
	byteLength uint64,
	rowCount uint64,
	maxTxnTS string,
	maxID uint64,
) {
	if validateFundsCanonicalCSVSnapshotBuildResultV1(result) != nil {
		return "", 0, 0, "", 0
	}
	return result.sourceArtifactSHA256,
		result.sourceArtifactByteLength,
		result.sourceRowCount,
		result.sourceMaxTxnTS,
		result.sourceMaxID
}

func ParseFundsCanonicalCSVSnapshotBuildResultV1(
	raw []byte,
	arguments FundsCanonicalCSVSnapshotBuildArgumentsV1,
) (FundsCanonicalCSVSnapshotBuildResultV1, error) {
	if validateFundsCanonicalCSVSnapshotBuildArgumentsV1(arguments) != nil ||
		domainjsonstrict.Validate(raw, domainjsonstrict.Options{
			RequireObject:  true,
			MaxBytes:       FundsCanonicalCSVMaximumResultBytesV1,
			MaxDepth:       5,
			MaxTokens:      512,
			MaxStringBytes: 256 * 1024,
			MaxNumberBytes: 32,
			MaxAbsExponent: 1,
		}) != nil {
		return FundsCanonicalCSVSnapshotBuildResultV1{}, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire fundsCanonicalCSVSnapshotBuildResultWireV1
	if err := decoder.Decode(&wire); err != nil {
		return FundsCanonicalCSVSnapshotBuildResultV1{}, ErrResultInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return FundsCanonicalCSVSnapshotBuildResultV1{}, ErrResultInvalid
	}
	materializationBody, err := json.Marshal(wire.FundsMaterializationResultV1)
	if err != nil {
		return FundsCanonicalCSVSnapshotBuildResultV1{}, ErrResultInvalid
	}
	materialization, err := domainsecurity.ParseFundsMaterializationResultV1(materializationBody)
	if err != nil || wire.SchemaVersion != 1 ||
		wire.Operation != OperationFundsBuildCanonicalCSVSnapshotV1 ||
		wire.Profile != FundsCanonicalDirectCSVProfileV1 ||
		wire.PrivateImportFileID != arguments.privateImportFileID ||
		wire.SourceArtifactSHA256 != arguments.sourceArtifactSHA256 ||
		wire.SourceArtifactByteLength != arguments.sourceArtifactByteLength ||
		wire.SourceRowCount != arguments.sourceRowCount ||
		wire.SourceMaxID != wire.SourceRowCount ||
		!canonicalFundsCanonicalCSVTimestampV1(wire.SourceMaxTxnTS) ||
		wire.RowHashContract != FundsCanonicalCSVRowHashContractV1 ||
		materialization.CaseID != arguments.caseID ||
		materialization.RawArtifactManifestSHA256 != arguments.rawArtifactManifestSHA256 ||
		len(materialization.RawSourceManifest) != 1 ||
		materialization.RawSourceManifest[0].FileID != arguments.privateImportFileID ||
		materialization.RawSourceManifest[0].SHA256 != arguments.sourceArtifactSHA256 ||
		materialization.RawSourceManifest[0].RowsImportedNorm != arguments.sourceRowCount ||
		materialization.ProducerContentManifest.SourceRevision != arguments.sourceRevision ||
		materialization.ProducerContentManifest.NormalizedRowCount != arguments.sourceRowCount ||
		materialization.ProducerContentManifest.AcceptedRowCount != arguments.sourceRowCount ||
		materialization.ProducerContentManifest.RejectedRowCount != 0 ||
		materialization.ProducerContentManifest.DuplicateRowCount != 0 ||
		materialization.ProducerContentManifest.DetailRowCount != arguments.sourceRowCount {
		return FundsCanonicalCSVSnapshotBuildResultV1{}, ErrResultInvalid
	}
	result := FundsCanonicalCSVSnapshotBuildResultV1{
		materialization:          materialization,
		sourceArtifactSHA256:     wire.SourceArtifactSHA256,
		sourceArtifactByteLength: wire.SourceArtifactByteLength,
		sourceRowCount:           wire.SourceRowCount,
		sourceMaxTxnTS:           wire.SourceMaxTxnTS,
		sourceMaxID:              wire.SourceMaxID,
	}
	if validateFundsCanonicalCSVSnapshotBuildResultV1(result) != nil {
		return FundsCanonicalCSVSnapshotBuildResultV1{}, ErrResultInvalid
	}
	return result, nil
}

func validateFundsCanonicalCSVSnapshotBuildResultV1(
	result FundsCanonicalCSVSnapshotBuildResultV1,
) error {
	body, err := json.Marshal(result.materialization)
	if err != nil {
		return ErrResultInvalid
	}
	materialization, err := domainsecurity.ParseFundsMaterializationResultV1(body)
	if err != nil ||
		!canonicalFundsCanonicalCSVDigestV1(result.sourceArtifactSHA256) ||
		result.sourceArtifactByteLength == 0 ||
		result.sourceArtifactByteLength > FundsCanonicalCSVMaximumSourceBytesV1 ||
		result.sourceRowCount == 0 ||
		result.sourceRowCount > FundsCanonicalCSVMaximumSourceRowsV1 ||
		result.sourceMaxID != result.sourceRowCount ||
		!canonicalFundsCanonicalCSVTimestampV1(result.sourceMaxTxnTS) ||
		len(materialization.RawSourceManifest) != 1 ||
		materialization.RawSourceManifest[0].SHA256 != result.sourceArtifactSHA256 ||
		materialization.RawSourceManifest[0].RowsImportedNorm != result.sourceRowCount ||
		materialization.ProducerContentManifest.NormalizedRowCount != result.sourceRowCount {
		return ErrResultInvalid
	}
	return nil
}

func canonicalFundsCanonicalCSVCaseIDV1(value string) bool {
	return value != "" && value != domainsecurity.UnboundCaseID &&
		value == strings.TrimSpace(value) &&
		len(value) <= fundsCanonicalCSVMaximumCaseIDBytesV1 &&
		utf8.ValidString(value) && !strings.ContainsRune(value, 0) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func canonicalFundsCanonicalCSVPrivateImportFileIDV1(value string) bool {
	if len(value) != fundsCanonicalCSVPrivateImportFileIDBytesV1 {
		return false
	}
	for index := range value {
		if value[index] < '0' ||
			(value[index] > '9' && value[index] < 'a') ||
			value[index] > 'f' {
			return false
		}
	}
	return true
}

func canonicalFundsCanonicalCSVDigestV1(value string) bool {
	return domainsecurity.IsSHA256Hex(value) && value == strings.ToLower(value)
}

func canonicalFundsCanonicalCSVTimestampV1(value string) bool {
	parsed, err := time.Parse("2006-01-02 15:04:05", value)
	return err == nil && parsed.Format("2006-01-02 15:04:05") == value
}
