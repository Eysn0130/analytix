package fundsquerysource

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	DescriptorSchemaVersionV1 = 1
	DescriptorContractV1      = "analytix.funds-query-source-descriptor/v1"

	FixedFundsQueryProfileContractV1             = "analytix.funds-query-profile/fixed-readonly/v1"
	FixedAccountFlowQuerySQLHashV1               = "d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51"
	FixedAccountIngressResolutionSQLHashV1       = "5fef7c06fe02fd67de5699e1a826f02103fe7c872f586b1f902ff967cf33f18d"
	FixedAccountFlowResultContractV1             = "analytix.account-flow-result/v1"
	FixedAccountIngressResultContractV1          = "analytix.account-ingress-resolution-result/v1"
	FixedDirectSourcePreviewResultContractV1     = "analytix.direct-source-preview/v1"
	FixedAccountFlowQueryProfileContractV1       = "analytix.funds-query-profile/analyze-account-flows/v1"
	FixedFundsLocalDisplayQueryProfileContractV1 = "analytix.funds-query-profile/fixed-readonly-local-display/v1"
	FixedFundsAnalyticalSchemaContractV1         = domainsecurity.FundsAnalyticalSchemaContractV1

	MaxDuckDBByteLengthV1        = uint64(1 << 40)
	AccountFlowMinorUnitScaleV1  = uint8(2)
	minDatasetUTCOffsetMinutesV1 = int16(-840)
	maxDatasetUTCOffsetMinutesV1 = int16(840)

	maxDescriptorBytesV1      = 64 * 1024
	maxPrivateIdentifierBytes = 4096
)

const fixedAccountFlowQueryProfileCanonicalV1 = `{"arbitrarySql":false,"completeIdentifierInput":false,"contract":"analytix.funds-query-profile/analyze-account-flows/v1","databasePathInput":false,"operation":"analyze_account_flows","querySqlHash":"d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51","readOnly":true,"resultContract":"analytix.account-flow-result/v1","schemaVersion":1}`

const fixedFundsQueryProfileCanonicalV1 = `{"arbitrarySql":false,"contract":"analytix.funds-query-profile/fixed-readonly/v1","databasePathInput":false,"hostPrivateCompleteIdentifierInput":true,"operations":[{"operation":"analyze_account_flows","querySqlHash":"d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51","resultContract":"analytix.account-flow-result/v1"},{"operation":"resolve_account_ingress","querySqlHash":"5fef7c06fe02fd67de5699e1a826f02103fe7c872f586b1f902ff967cf33f18d","resultContract":"analytix.account-ingress-resolution-result/v1"}],"providerCompleteIdentifierInput":false,"readOnly":true,"schemaVersion":1}`

const fixedFundsLocalDisplayQueryProfileCanonicalV1 = `{"arbitrarySql":false,"contract":"analytix.funds-query-profile/fixed-readonly-local-display/v1","databasePathInput":false,"hostPrivateCompleteIdentifierInput":true,"operations":[{"operation":"analyze_account_flows","querySqlHash":"d06b98ac697a24f6cca52d33ead6839ad9d2eed6ce9147f967d1a0a4e79a5b51","resultContract":"analytix.account-flow-result/v1"},{"operation":"direct_source_preview","fieldAllowlist":["transactionTime","account","card","accountName","identityNumber","amountText","direction","counterpartyAccount","counterpartyName","counterpartyIdentityNumber","counterpartyBank","summary","currency","merchantName","remark"],"resultContract":"analytix.direct-source-preview/v1"},{"operation":"resolve_account_ingress","querySqlHash":"5fef7c06fe02fd67de5699e1a826f02103fe7c872f586b1f902ff967cf33f18d","resultContract":"analytix.account-ingress-resolution-result/v1"}],"providerCompleteIdentifierInput":false,"readOnly":true,"schemaVersion":1}`

var (
	descriptorDigestDomainV1 = []byte("AnalytixFundsQuerySourceDescriptorV1\x00")
	caseIDPatternV1          = regexp.MustCompile(`^[A-Za-z0-9_-]{4,80}$`)
	materializationPatternV1 = regexp.MustCompile(`^txn_daily_snapshot:v12:[a-f0-9]{64}$`)
)

// DescriptorV1 is a temporary path-free projection of one exact FPC1 DSV2
// selection and its analytical DuckDB binding. It is neither persisted
// registry state nor an authority: the existing DSV2 capability must remain
// live while the host opens and copies the exact immutable source.
//
// Field order is part of the private canonical encoding.
type DescriptorV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Contract      string `json:"contract"`

	SnapshotRecordDigest     string `json:"snapshotRecordDigest"`
	DatasetSnapshotID        string `json:"datasetSnapshotId"`
	SourceManifestHash       string `json:"sourceManifestHash"`
	CaseID                   string `json:"caseId"`
	CaseBindingHash          string `json:"caseBindingHash"`
	DatasetBindingDigest     string `json:"datasetBindingDigest"`
	BindingObservationDigest string `json:"bindingObservationDigest"`

	FundsProducerContentID                 string `json:"fundsProducerContentId"`
	FundsProducerContentManifestSHA256     string `json:"fundsProducerContentManifestSha256"`
	FundsProducerContentManifestByteLength uint64 `json:"fundsProducerContentManifestByteLength"`

	DuckDBSHA256                 string `json:"duckdbSha256"`
	DuckDBByteLength             uint64 `json:"duckdbByteLength"`
	DuckDBContentSnapshotDigest  string `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256 string `json:"duckdbSnapshotManifestSha256"`
	MaterializationIdentity      string `json:"materializationIdentity"`
	SchemaDigest                 string `json:"schemaDigest"`
	DatasetUTCOffsetMinutes      int16  `json:"datasetUtcOffsetMinutes"`
	ExpectedCurrency             string `json:"expectedCurrency"`
	MinorUnitScale               uint8  `json:"minorUnitScale"`
	QueryProfileDigest           string `json:"queryProfileDigest"`
	DescriptorDigest             string `json:"descriptorDigest"`
}

type DescriptorInputV1 struct {
	SnapshotRecordDigest     string
	DatasetSnapshotID        string
	SourceManifestHash       string
	CaseID                   string
	CaseBindingHash          string
	DatasetBindingDigest     string
	BindingObservationDigest string

	FundsProducerContentID                 string
	FundsProducerContentManifestSHA256     string
	FundsProducerContentManifestByteLength uint64

	DuckDBSHA256                 string
	DuckDBByteLength             uint64
	DuckDBContentSnapshotDigest  string
	DuckDBSnapshotManifestSHA256 string
	MaterializationIdentity      string
	SchemaDigest                 string
	DatasetUTCOffsetMinutes      int16
	ExpectedCurrency             string
	MinorUnitScale               uint8
	QueryProfileDigest           string
}

func NewDescriptorV1(input DescriptorInputV1) (DescriptorV1, error) {
	descriptor := DescriptorV1{
		SchemaVersion: DescriptorSchemaVersionV1,
		Contract:      DescriptorContractV1,

		SnapshotRecordDigest:     input.SnapshotRecordDigest,
		DatasetSnapshotID:        input.DatasetSnapshotID,
		SourceManifestHash:       input.SourceManifestHash,
		CaseID:                   input.CaseID,
		CaseBindingHash:          input.CaseBindingHash,
		DatasetBindingDigest:     input.DatasetBindingDigest,
		BindingObservationDigest: input.BindingObservationDigest,

		FundsProducerContentID:                 input.FundsProducerContentID,
		FundsProducerContentManifestSHA256:     input.FundsProducerContentManifestSHA256,
		FundsProducerContentManifestByteLength: input.FundsProducerContentManifestByteLength,

		DuckDBSHA256:                 input.DuckDBSHA256,
		DuckDBByteLength:             input.DuckDBByteLength,
		DuckDBContentSnapshotDigest:  input.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256: input.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:      input.MaterializationIdentity,
		SchemaDigest:                 input.SchemaDigest,
		DatasetUTCOffsetMinutes:      input.DatasetUTCOffsetMinutes,
		ExpectedCurrency:             input.ExpectedCurrency,
		MinorUnitScale:               input.MinorUnitScale,
		QueryProfileDigest:           input.QueryProfileDigest,
	}
	descriptor.DescriptorDigest = descriptorDigestV1(descriptor)
	if err := ValidateDescriptorV1(descriptor); err != nil {
		return DescriptorV1{}, err
	}
	return descriptor, nil
}

func ValidateDescriptorV1(descriptor DescriptorV1) error {
	if descriptor.SchemaVersion != DescriptorSchemaVersionV1 ||
		descriptor.Contract != DescriptorContractV1 ||
		!canonicalDigestV1(descriptor.SnapshotRecordDigest) ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(descriptor.DatasetSnapshotID) ||
		!canonicalDigestV1(descriptor.SourceManifestHash) ||
		!caseIDPatternV1.MatchString(descriptor.CaseID) ||
		descriptor.CaseID == domainsecurity.UnboundCaseID ||
		!canonicalDigestV1(descriptor.CaseBindingHash) ||
		!canonicalDigestV1(descriptor.DatasetBindingDigest) ||
		!canonicalDigestV1(descriptor.BindingObservationDigest) ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(descriptor.FundsProducerContentID) ||
		!canonicalDigestV1(descriptor.FundsProducerContentManifestSHA256) ||
		descriptor.FundsProducerContentManifestByteLength == 0 ||
		descriptor.FundsProducerContentManifestByteLength > maxDescriptorBytesV1 ||
		!canonicalDigestV1(descriptor.DuckDBSHA256) ||
		descriptor.DuckDBByteLength == 0 ||
		descriptor.DuckDBByteLength > MaxDuckDBByteLengthV1 ||
		!canonicalDigestV1(descriptor.DuckDBContentSnapshotDigest) ||
		!canonicalDigestV1(descriptor.DuckDBSnapshotManifestSHA256) ||
		!materializationPatternV1.MatchString(descriptor.MaterializationIdentity) ||
		descriptor.SchemaDigest != FixedFundsAnalyticalSchemaDigestV1() ||
		descriptor.DatasetUTCOffsetMinutes < minDatasetUTCOffsetMinutesV1 ||
		descriptor.DatasetUTCOffsetMinutes > maxDatasetUTCOffsetMinutesV1 ||
		!canonicalCurrencyV1(descriptor.ExpectedCurrency) ||
		descriptor.MinorUnitScale != AccountFlowMinorUnitScaleV1 ||
		!QueryProfileSupportsAccountFlowV1(descriptor.QueryProfileDigest) ||
		descriptor.DescriptorDigest != descriptorDigestV1(descriptor) {
		return errors.New("funds query source descriptor is invalid")
	}
	return nil
}

func DescriptorV1Bytes(descriptor DescriptorV1) ([]byte, error) {
	if err := ValidateDescriptorV1(descriptor); err != nil {
		return nil, err
	}
	return canonicalJSONV1(descriptor)
}

func ParseDescriptorV1(raw []byte) (DescriptorV1, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxDescriptorBytesV1,
		MaxDepth:       3,
		MaxTokens:      128,
		MaxStringBytes: maxPrivateIdentifierBytes * 2,
	}); err != nil {
		return DescriptorV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var descriptor DescriptorV1
	if err := decoder.Decode(&descriptor); err != nil {
		return DescriptorV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DescriptorV1{}, errors.New("funds query source descriptor contains trailing JSON")
	}
	if err := ValidateDescriptorV1(descriptor); err != nil {
		return DescriptorV1{}, err
	}
	canonical, err := DescriptorV1Bytes(descriptor)
	if err != nil || !bytes.Equal(raw, canonical) {
		return DescriptorV1{}, errors.New("funds query source descriptor is not canonically encoded")
	}
	return descriptor, nil
}

func FixedAccountFlowQueryProfileDigestV1() string {
	return domainsecurity.SHA256Hex([]byte(fixedAccountFlowQueryProfileCanonicalV1))
}

func FixedFundsQueryProfileDigestV1() string {
	return domainsecurity.SHA256Hex([]byte(fixedFundsQueryProfileCanonicalV1))
}

func FixedFundsLocalDisplayQueryProfileDigestV1() string {
	return domainsecurity.SHA256Hex([]byte(fixedFundsLocalDisplayQueryProfileCanonicalV1))
}

func FixedFundsAnalyticalSchemaDigestV1() string {
	return domainsecurity.FixedFundsAnalyticalSchemaDigestV1()
}

func QueryProfileSupportsAccountFlowV1(digest string) bool {
	return digest == FixedAccountFlowQueryProfileDigestV1() ||
		digest == FixedFundsQueryProfileDigestV1() ||
		digest == FixedFundsLocalDisplayQueryProfileDigestV1()
}

func QueryProfileSupportsAccountIngressResolutionV1(digest string) bool {
	return digest == FixedFundsQueryProfileDigestV1() ||
		digest == FixedFundsLocalDisplayQueryProfileDigestV1()
}

func QueryProfileSupportsDirectSourcePreviewV1(digest string) bool {
	return digest == FixedFundsLocalDisplayQueryProfileDigestV1()
}

func descriptorDigestV1(descriptor DescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, err := canonicalJSONV1(descriptor)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(append(
		append([]byte(nil), descriptorDigestDomainV1...),
		body...,
	))
}

func canonicalDigestV1(value string) bool {
	return domainsecurity.IsSHA256Hex(value) && value == strings.ToLower(value)
}

func canonicalCurrencyV1(value string) bool {
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

func canonicalJSONV1(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	body := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	body = bytes.ReplaceAll(body, []byte(`\u2028`), []byte("\u2028"))
	body = bytes.ReplaceAll(body, []byte(`\u2029`), []byte("\u2029"))
	return body, nil
}
