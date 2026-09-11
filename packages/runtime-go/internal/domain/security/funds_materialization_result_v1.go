package security

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	FundsMaterializationIdentitySchemaVersionV1 = 2
	FundsMaterializationAggregateVersionV1      = 12
	FundsMaterializationIdentityPrefixV1        = "txn_daily_snapshot:v12:"
	FundsAnalyticalSchemaContractV1             = "analytix.funds-analytical-schema/v1"

	maxFundsMaterializationResultBytesV1 = 8 * 1024 * 1024
	maxFundsRawSourceManifestBytesV1     = 4 * 1024 * 1024
	maxFundsRawSourceEntriesV1           = 16 * 1024
)

// fundsAnalyticalSchemaCanonicalV1 is the package-owned logical schema needed
// by the fixed account-flow and private account-ingress operations. It names
// only required relations and semantic columns; callers cannot substitute an
// observed or caller-selected DuckDB schema digest.
const fundsAnalyticalSchemaCanonicalV1 = `{"contract":"analytix.funds-analytical-schema/v1","exactTypes":{"fc_transaction_norm.clean_amount":"VARCHAR","fc_transaction_norm.file_id":"VARCHAR","fc_transaction_norm.row_no":"BIGINT"},"operations":[{"name":"analyze_account_flows","relations":[{"columns":["case_id","clean_amount","file_id","id","row_no"],"name":"fc_transaction_norm"},{"columns":["acct_key","amount_parse_failed","amount_source_present","counterparty_bank","counterparty_name","cp_key","currency","dc_val","file_id","id","txn_ts"],"name":"analysis_txn_detail_idx"}]},{"name":"resolve_account_ingress","relations":[{"columns":["account_key","acct_type","bank_name"],"name":"analysis_account_dim"},{"columns":["acct_key","acct_no","card_no"],"name":"analysis_txn_detail_idx"}]}],"schemaVersion":1}`

// FundsMaterializationResultV1 is the exact host-private response emitted by
// funds.materialize_txn_daily_v1. The embedded producer and raw-source bytes
// are untrusted until ParseFundsMaterializationResultV1 has revalidated every
// redundant identity. This contract is producer evidence only; it is not a
// DatasetSnapshotAuthorityV2, EvidenceReceipt, or fact publication authority.
type FundsMaterializationResultV1 struct {
	CaseID                               string `json:"caseId"`
	RowCount                             uint64 `json:"rowCount"`
	AggregateName                        string `json:"aggName"`
	AggregateVersion                     int    `json:"aggVersion"`
	MaterializationIdentity              string `json:"materializationIdentity"`
	MaterializationIdentitySchemaVersion int    `json:"materializationIdentitySchemaVersion"`
	ProducerContentID                    string `json:"producerContentId"`
	ProducerContentManifestBase64        string `json:"producerContentManifestBase64"`
	ProducerContentManifestSHA256        string `json:"producerContentManifestSha256"`
	ProducerContentManifestByteLength    uint64 `json:"producerContentManifestByteLength"`
	RawArtifactManifestSHA256            string `json:"rawArtifactManifestSha256"`
	// RawSourceManifest* binds the producer-private simplified import-file
	// inventory below. It is not the canonical typed RawArtifactManifestV1 and
	// must never be substituted for RawArtifactManifestSHA256.
	RawSourceManifestBase64      string `json:"rawSourceManifestBase64"`
	RawSourceManifestSHA256      string `json:"rawSourceManifestSha256"`
	RawSourceManifestByteLength  uint64 `json:"rawSourceManifestByteLength"`
	DuckDBContentSnapshotDigest  string `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256 string `json:"duckdbSnapshotManifestSha256"`
	SchemaDigest                 string `json:"schemaDigest"`

	ProducerContentManifest FundsProducerContentManifestV1 `json:"-"`
	RawSourceManifest       FundsRawSourceManifestV1       `json:"-"`
}

type FundsRawSourceIdentityV1 struct {
	CleanedStatus    string `json:"cleanedStatus"`
	FileID           string `json:"fileId"`
	RowsImportedNorm uint64 `json:"rowsImportedNorm"`
	SHA256           string `json:"sha256"`
	Status           string `json:"status"`
}

type FundsRawSourceManifestV1 []FundsRawSourceIdentityV1

func ParseFundsMaterializationResultV1(raw []byte) (FundsMaterializationResultV1, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxFundsMaterializationResultBytesV1, MaxDepth: 4,
		MaxTokens: 512, MaxStringBytes: maxFundsRawSourceManifestBytesV1 * 2,
	}); err != nil {
		return FundsMaterializationResultV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result FundsMaterializationResultV1
	if err := decoder.Decode(&result); err != nil {
		return FundsMaterializationResultV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return FundsMaterializationResultV1{}, errors.New("funds materialization result contains trailing JSON")
	}
	producerBody, err := decodeCanonicalBase64V1(
		result.ProducerContentManifestBase64,
		result.ProducerContentManifestByteLength,
		maxFundsProducerContentManifestBytesV1,
	)
	if err != nil || SHA256Hex(producerBody) != result.ProducerContentManifestSHA256 {
		return FundsMaterializationResultV1{}, errors.New("funds producer manifest wire identity is invalid")
	}
	producer, err := ParseFundsProducerContentManifestV1(producerBody)
	if err != nil {
		return FundsMaterializationResultV1{}, err
	}
	reencodedProducer, err := FundsProducerContentManifestV1Bytes(producer)
	if err != nil || !bytes.Equal(producerBody, reencodedProducer) ||
		DeriveFundsProducerContentIDV1(producer) != result.ProducerContentID {
		return FundsMaterializationResultV1{}, errors.New("funds producer manifest canonical identity is invalid")
	}
	rawSourceBody, err := decodeCanonicalBase64V1(
		result.RawSourceManifestBase64,
		result.RawSourceManifestByteLength,
		maxFundsRawSourceManifestBytesV1,
	)
	if err != nil || SHA256Hex(rawSourceBody) != result.RawSourceManifestSHA256 {
		return FundsMaterializationResultV1{}, errors.New("funds raw-source manifest wire identity is invalid")
	}
	rawSources, err := ParseFundsRawSourceManifestV1(rawSourceBody, producer.NormalizedRowCount)
	if err != nil {
		return FundsMaterializationResultV1{}, err
	}
	if result.AggregateVersion != FundsMaterializationAggregateVersionV1 ||
		result.MaterializationIdentitySchemaVersion != FundsMaterializationIdentitySchemaVersionV1 ||
		result.CaseID != producer.CaseID || result.RowCount != producer.AggregateRowCount ||
		!isCanonicalSHA256Hex(result.RawArtifactManifestSHA256) ||
		result.RawArtifactManifestSHA256 != producer.RawManifestSHA256 ||
		result.RawArtifactManifestSHA256 == result.RawSourceManifestSHA256 ||
		!isCanonicalSHA256Hex(result.DuckDBContentSnapshotDigest) ||
		!isCanonicalSHA256Hex(result.DuckDBSnapshotManifestSHA256) ||
		result.SchemaDigest != FixedFundsAnalyticalSchemaDigestV1() ||
		result.AggregateName != result.MaterializationIdentity ||
		!canonicalFundsMaterializationIdentityV1(result.MaterializationIdentity) {
		return FundsMaterializationResultV1{}, errors.New("funds materialization result binding is invalid")
	}
	result.ProducerContentManifest = producer
	result.RawSourceManifest = rawSources
	return result, nil
}

func FixedFundsAnalyticalSchemaDigestV1() string {
	return SHA256Hex([]byte(fundsAnalyticalSchemaCanonicalV1))
}

func ParseFundsRawSourceManifestV1(raw []byte, expectedRows uint64) (FundsRawSourceManifestV1, error) {
	if expectedRows > maxJSONSafeUint64 || len(raw) == 0 || len(raw) > maxFundsRawSourceManifestBytesV1 {
		return nil, errors.New("funds raw-source manifest bounds are invalid")
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		MaxBytes: maxFundsRawSourceManifestBytesV1, MaxDepth: 3,
		MaxTokens: maxFundsRawSourceEntriesV1 * 16, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest FundsRawSourceManifestV1
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("funds raw-source manifest contains trailing JSON")
	}
	if len(manifest) == 0 || len(manifest) > maxFundsRawSourceEntriesV1 {
		return nil, errors.New("funds raw-source manifest entry count is invalid")
	}
	canonical, err := fundsRawSourceManifestV1Bytes(manifest)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, errors.New("funds raw-source manifest is not canonically encoded")
	}
	var total uint64
	previousFileID := ""
	for _, source := range manifest {
		if !canonicalDatasetSnapshotManifestTextV2(source.FileID) || source.FileID <= previousFileID ||
			!isCanonicalSHA256Hex(source.SHA256) || source.Status != "已完成" || source.CleanedStatus != "done" ||
			source.RowsImportedNorm > maxJSONSafeUint64 || total > maxJSONSafeUint64-source.RowsImportedNorm {
			return nil, errors.New("funds raw-source manifest entry is invalid")
		}
		total += source.RowsImportedNorm
		previousFileID = source.FileID
	}
	if total != expectedRows {
		return nil, errors.New("funds raw-source manifest row coverage is inconsistent")
	}
	return manifest, nil
}

func fundsMaterializationResultV1Bytes(result FundsMaterializationResultV1) ([]byte, error) {
	result.ProducerContentManifest = FundsProducerContentManifestV1{}
	result.RawSourceManifest = nil
	return canonicalJSONWithoutHTMLEscapeV1(result)
}

func fundsRawSourceManifestV1Bytes(manifest FundsRawSourceManifestV1) ([]byte, error) {
	return canonicalJSONWithoutHTMLEscapeV1(manifest)
}

func canonicalJSONWithoutHTMLEscapeV1(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

func decodeCanonicalBase64V1(value string, expectedLength uint64, maximum int) ([]byte, error) {
	if value == "" || value != strings.TrimSpace(value) || expectedLength == 0 ||
		expectedLength > uint64(maximum) || len(value) > base64.StdEncoding.EncodedLen(maximum) {
		return nil, errors.New("funds materialization base64 bounds are invalid")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || uint64(len(decoded)) != expectedLength || base64.StdEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("funds materialization base64 is invalid")
	}
	return decoded, nil
}

func canonicalFundsMaterializationIdentityV1(value string) bool {
	if !strings.HasPrefix(value, FundsMaterializationIdentityPrefixV1) {
		return false
	}
	return isCanonicalSHA256Hex(strings.TrimPrefix(value, FundsMaterializationIdentityPrefixV1))
}
