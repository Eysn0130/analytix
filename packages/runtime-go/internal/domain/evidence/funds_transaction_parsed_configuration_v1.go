package evidence

import (
	"encoding/json"
	"errors"
	"reflect"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	FundsTransactionParsedConfigurationSchemaVersionV1 = 1
	FundsTransactionParsedConfigurationPurposeV1       = "analytix.funds-transaction-canonical-direct-csv-configuration/v1"

	FundsTransactionCSVReplayProfileV1        = "canonical_direct_csv_v1"
	FundsTransactionCSVEncodingV1             = "utf-8-no-bom"
	FundsTransactionCSVDelimiterV1            = "comma"
	FundsTransactionCSVHeaderModeV1           = "exact_ordered_standard_fc_transaction"
	FundsTransactionCSVBlankLineModeV1        = "ignored"
	FundsTransactionCSVRecordWidthV1          = "header_exact"
	FundsTransactionCSVCellModeV1             = "trim_canonical_empty_is_null"
	FundsTransactionCSVDuplicateModeV1        = "source_locator_unique_all_occurrences_accepted"
	FundsTransactionCSVNewlineModeV1          = "go_encoding_csv_crlf_normalized_quoted_multiline_rejected_by_cell_policy"
	FundsTransactionCSVNormalizationSourceV1  = "go_host_deterministic_canonical_direct_csv_normalizer_v1"
	FundsTransactionCSVSourceRowNumberModeV1  = "data_occurrence_ordinal_1_based"
	FundsTransactionNativeSourceProofStatusV1 = "duckdb_projection_requires_host_raw_replay"

	FundsTransactionCSVMaxArtifactBytesV1 = uint64(64 * 1024 * 1024)
	FundsTransactionCSVMaxRowsV1          = uint64(100_000)
	FundsTransactionCSVMaxCellBytesV1     = uint64(16 * 1024)
	FundsTransactionCSVMaxHeaderBytesV1   = uint64(16 * 1024)
	FundsTransactionCSVMaxDecimalBytesV1  = uint64(128)

	maxFundsTransactionParsedConfigurationBytesV1 = 128 * 1024
)

var (
	fundsTransactionParsedConfigurationDigestDomainV1 = []byte("analytix.funds-transaction-canonical-direct-csv-configuration/digest/v1\x00")
	fundsTransactionProjectionSchemaContractV1        = []byte(`{"fields":{"norm.clean_acct_no":"text","norm.clean_amount":"decimal","norm.clean_balance":"decimal","norm.clean_card_no":"text","norm.clean_dc_flag":"text","norm.clean_failed":"integer","norm.clean_invalid":"integer","norm.clean_reversal":"integer","norm.txn_ts":"text","raw.account_open_name":"text","raw.acct_no":"text","raw.amount":"text","raw.balance":"text","raw.branch_code":"text","raw.branch_name":"text","raw.card_no":"text","raw.cash_flag":"text","raw.counterparty_acct":"text","raw.counterparty_balance":"text","raw.counterparty_bank":"text","raw.counterparty_id_no":"text","raw.counterparty_name":"text","raw.currency":"text","raw.dc_flag":"text","raw.ip_addr":"text","raw.is_success":"text","raw.location":"text","raw.log_id":"text","raw.mac_addr":"text","raw.merchant_name":"text","raw.merchant_no":"text","raw.opener_id_no":"text","raw.query_feedback_reason":"text","raw.remark":"text","raw.summary":"text","raw.teller_no":"text","raw.terminal_no":"text","raw.txn_id":"text","raw.txn_time":"text","raw.txn_type":"text","raw.voucher_id":"text","raw.voucher_no":"text","raw.voucher_type":"text"},"fieldOrder":"name_ascending","schemaVersion":1}`)
	fundsTransactionReaderModeContractV1              = []byte(`{"blankLines":"ignored","cellMode":"trim_canonical_empty_is_null_controls_and_cf_rejected","comments":"disabled","crlf":"normalized","delimiter":"comma","fieldsPerRecord":"exact_34","header":"exact_ordered_standard_fc_transaction","lazyQuotes":false,"normalizationSource":"go_host_deterministic_canonical_direct_csv_normalizer_v1","quotedMultiline":"rejected_by_cell_policy","reuseRecord":false,"schemaVersion":1,"sourceRowNumber":"data_occurrence_ordinal_1_based","trimLeadingSpace":false}`)
	fundsTransactionHostNormalizerContractV1          = []byte(`{"accountIdentity":{"cleanValue":"exact_raw","forbiddenCharacters":["unicode_whitespace","-","_"],"required":"card_or_account"},"amount":{"cleanValue":"exact_raw","input":"canonical_signed_decimal_scale_0_to_2","maximumUtf8Bytes":128,"required":true,"requiresFiniteFloat64Projection":true},"balance":{"cleanValue":"exact_raw","input":"canonical_signed_decimal_scale_0_to_2_or_null","maximumUtf8Bytes":128,"requiresFiniteFloat64Projection":true},"cleanFlags":{"cleanFailed":0,"cleanInvalid":0,"cleanReversal":0},"currency":{"required":"CNY"},"direction":{"cleanValue":"exact_raw","enum":["进","出"],"required":true},"schemaVersion":1,"timestamp":{"format":"YYYY-MM-DD HH:MM:SS","required":true,"roundTrip":true}}`)
)

// FundsTransactionCSVColumnV1 fixes one exact source-header to private typed
// evidence-field mapping. It contains schema metadata only, never source data.
type FundsTransactionCSVColumnV1 struct {
	Header string `json:"header"`
	Field  string `json:"field"`
}

// FundsTransactionParsedConfigurationV1 is the exact, closed parser profile
// for the first canonical direct-CSV funds slice. Aliases, positional fallback,
// caller-selected mappings, and lossy cleanup are intentionally absent.
type FundsTransactionParsedConfigurationV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	PolicyID      string `json:"policyId"`
	ParserID      string `json:"parserId"`
	ParserVersion string `json:"parserVersion"`

	ReplayProfile             string `json:"replayProfile"`
	Encoding                  string `json:"encoding"`
	Delimiter                 string `json:"delimiter"`
	HeaderMode                string `json:"headerMode"`
	BlankLineMode             string `json:"blankLineMode"`
	RecordWidthMode           string `json:"recordWidthMode"`
	CellMode                  string `json:"cellMode"`
	DuplicateMode             string `json:"duplicateMode"`
	NewlineMode               string `json:"newlineMode"`
	NormalizationSource       string `json:"normalizationSource"`
	SourceRowNumberMode       string `json:"sourceRowNumberMode"`
	NativeOperation           string `json:"nativeOperation"`
	NativeOperationSchemaHash string `json:"nativeOperationSchemaHash"`
	NativeLocatorOrdering     string `json:"nativeLocatorOrdering"`
	NativeSourceProofStatus   string `json:"nativeSourceProofStatus"`

	LazyQuotes       bool   `json:"lazyQuotes"`
	TrimLeadingSpace bool   `json:"trimLeadingSpace"`
	ReuseRecord      bool   `json:"reuseRecord"`
	MaxArtifactBytes uint64 `json:"maxArtifactBytes"`
	MaxRows          uint64 `json:"maxRows"`
	MaxCellBytes     uint64 `json:"maxCellBytes"`
	MaxHeaderBytes   uint64 `json:"maxHeaderBytes"`
	MaxDecimalBytes  uint64 `json:"maxDecimalBytes"`

	Columns                []FundsTransactionCSVColumnV1 `json:"columns"`
	MappingDigest          string                        `json:"mappingDigest"`
	ProjectionSchemaDigest string                        `json:"projectionSchemaDigest"`
	ReaderModeDigest       string                        `json:"readerModeDigest"`
	NormalizationDigest    string                        `json:"normalizationDigest"`
	ConfigurationDigest    string                        `json:"configurationDigest"`
}

func NewFundsTransactionParsedConfigurationV1() FundsTransactionParsedConfigurationV1 {
	columns := fundsTransactionCSVColumnsV1()
	mappingBody, _ := json.Marshal(columns)
	configuration := FundsTransactionParsedConfigurationV1{
		SchemaVersion: FundsTransactionParsedConfigurationSchemaVersionV1,
		Purpose:       FundsTransactionParsedConfigurationPurposeV1,
		PolicyID:      FundsCanonicalTransactionSourceRowPolicyIDV1,
		ParserID:      SourceRowProducerParserIDFundsV1,
		ParserVersion: SourceRowProducerParserFundsV1,

		ReplayProfile:             FundsTransactionCSVReplayProfileV1,
		Encoding:                  FundsTransactionCSVEncodingV1,
		Delimiter:                 FundsTransactionCSVDelimiterV1,
		HeaderMode:                FundsTransactionCSVHeaderModeV1,
		BlankLineMode:             FundsTransactionCSVBlankLineModeV1,
		RecordWidthMode:           FundsTransactionCSVRecordWidthV1,
		CellMode:                  FundsTransactionCSVCellModeV1,
		DuplicateMode:             FundsTransactionCSVDuplicateModeV1,
		NewlineMode:               FundsTransactionCSVNewlineModeV1,
		NormalizationSource:       FundsTransactionCSVNormalizationSourceV1,
		SourceRowNumberMode:       FundsTransactionCSVSourceRowNumberModeV1,
		NativeOperation:           SourceRowProducerOperationFundsPageV1,
		NativeOperationSchemaHash: fundsTransactionSourceRowOperationSchemaHashV1(),
		NativeLocatorOrdering:     SourceRowProducerLocatorOrderingV1,
		NativeSourceProofStatus:   FundsTransactionNativeSourceProofStatusV1,

		MaxArtifactBytes: FundsTransactionCSVMaxArtifactBytesV1,
		MaxRows:          FundsTransactionCSVMaxRowsV1,
		MaxCellBytes:     FundsTransactionCSVMaxCellBytesV1,
		MaxHeaderBytes:   FundsTransactionCSVMaxHeaderBytesV1,
		MaxDecimalBytes:  FundsTransactionCSVMaxDecimalBytesV1,
		Columns:          columns,
		MappingDigest:    domainsecurity.SHA256Hex(mappingBody),
		ProjectionSchemaDigest: domainsecurity.SHA256Hex(
			fundsTransactionProjectionSchemaContractV1,
		),
		ReaderModeDigest:    domainsecurity.SHA256Hex(fundsTransactionReaderModeContractV1),
		NormalizationDigest: domainsecurity.SHA256Hex(fundsTransactionHostNormalizerContractV1),
	}
	configuration.ConfigurationDigest = fundsTransactionParsedConfigurationDigestV1(configuration)
	return configuration
}

func ValidateFundsTransactionParsedConfigurationV1(
	configuration FundsTransactionParsedConfigurationV1,
) error {
	expected := NewFundsTransactionParsedConfigurationV1()
	if !reflect.DeepEqual(configuration, expected) ||
		configuration.ConfigurationDigest != fundsTransactionParsedConfigurationDigestV1(configuration) {
		return errors.New("funds transaction parsed configuration v1 is invalid")
	}
	return nil
}

func ParseFundsTransactionParsedConfigurationV1(
	raw []byte,
) (FundsTransactionParsedConfigurationV1, error) {
	var configuration FundsTransactionParsedConfigurationV1
	if err := parseCanonicalSourceRowContractV1(
		raw,
		&configuration,
		maxFundsTransactionParsedConfigurationBytesV1,
		2_048,
		maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return FundsTransactionParsedConfigurationV1{}, err
	}
	return configuration, ValidateFundsTransactionParsedConfigurationV1(configuration)
}

func FundsTransactionParsedConfigurationV1Bytes(
	configuration FundsTransactionParsedConfigurationV1,
) ([]byte, error) {
	if err := ValidateFundsTransactionParsedConfigurationV1(configuration); err != nil {
		return nil, err
	}
	body, err := json.Marshal(configuration)
	if err != nil || len(body) > maxFundsTransactionParsedConfigurationBytesV1 {
		return nil, errors.New("funds transaction parsed configuration exceeds its canonical byte limit")
	}
	return body, nil
}

func ValidateParsedGenerationIdentityForFundsTransactionConfigurationV1(
	identity ParsedGenerationIdentityV1,
	configuration FundsTransactionParsedConfigurationV1,
) error {
	if ValidateParsedGenerationIdentityV1(identity) != nil ||
		ValidateFundsTransactionParsedConfigurationV1(configuration) != nil {
		return errors.New("funds transaction parsed generation configuration is invalid")
	}
	body, err := FundsTransactionParsedConfigurationV1Bytes(configuration)
	if err != nil || identity.PolicyID != configuration.PolicyID ||
		identity.ParserID != configuration.ParserID ||
		identity.ParserVersion != configuration.ParserVersion ||
		identity.MappingDigest != configuration.MappingDigest ||
		identity.ProjectionSchemaDigest != configuration.ProjectionSchemaDigest ||
		identity.ReaderModeDigest != configuration.ReaderModeDigest ||
		identity.ConfigurationDigest != configuration.ConfigurationDigest ||
		identity.ConfigurationSHA256 != domainsecurity.SHA256Hex(body) ||
		identity.ConfigurationByteLength != uint64(len(body)) {
		return errors.New("funds transaction parsed generation identity does not match its exact configuration")
	}
	return nil
}

func fundsTransactionCSVColumnsV1() []FundsTransactionCSVColumnV1 {
	return []FundsTransactionCSVColumnV1{
		{Header: "交易卡号", Field: "raw.card_no"},
		{Header: "交易账号", Field: "raw.acct_no"},
		{Header: "账户开户名称", Field: "raw.account_open_name"},
		{Header: "开户人证件号码", Field: "raw.opener_id_no"},
		{Header: "交易时间", Field: "raw.txn_time"},
		{Header: "交易金额", Field: "raw.amount"},
		{Header: "交易余额", Field: "raw.balance"},
		{Header: "收付标志", Field: "raw.dc_flag"},
		{Header: "交易对手账卡号", Field: "raw.counterparty_acct"},
		{Header: "现金标志", Field: "raw.cash_flag"},
		{Header: "对手户名", Field: "raw.counterparty_name"},
		{Header: "对手身份证号", Field: "raw.counterparty_id_no"},
		{Header: "对手开户银行", Field: "raw.counterparty_bank"},
		{Header: "摘要说明", Field: "raw.summary"},
		{Header: "交易币种", Field: "raw.currency"},
		{Header: "交易网点名称", Field: "raw.branch_name"},
		{Header: "交易网点代码", Field: "raw.branch_code"},
		{Header: "交易发生地", Field: "raw.location"},
		{Header: "交易是否成功", Field: "raw.is_success"},
		{Header: "传票号", Field: "raw.voucher_no"},
		{Header: "终端号", Field: "raw.terminal_no"},
		{Header: "IP地址", Field: "raw.ip_addr"},
		{Header: "MAC地址", Field: "raw.mac_addr"},
		{Header: "对手交易余额", Field: "raw.counterparty_balance"},
		{Header: "交易流水号", Field: "raw.txn_id"},
		{Header: "日志号", Field: "raw.log_id"},
		{Header: "凭证种类", Field: "raw.voucher_type"},
		{Header: "凭证号", Field: "raw.voucher_id"},
		{Header: "交易柜员号", Field: "raw.teller_no"},
		{Header: "商户名称", Field: "raw.merchant_name"},
		{Header: "商户号", Field: "raw.merchant_no"},
		{Header: "备注", Field: "raw.remark"},
		{Header: "交易类型", Field: "raw.txn_type"},
		{Header: "查询反馈结果原因", Field: "raw.query_feedback_reason"},
	}
}

// FundsTransactionCSVColumnsV1 returns the closed canonical 34-column Funds
// mapping without exposing source data or making the mapping caller-owned.
func FundsTransactionCSVColumnsV1() []FundsTransactionCSVColumnV1 {
	return append([]FundsTransactionCSVColumnV1(nil), fundsTransactionCSVColumnsV1()...)
}

func fundsTransactionParsedConfigurationDigestV1(
	configuration FundsTransactionParsedConfigurationV1,
) string {
	configuration.ConfigurationDigest = ""
	body, _ := json.Marshal(configuration)
	return domainsecurity.SHA256Hex(append(
		append([]byte(nil), fundsTransactionParsedConfigurationDigestDomainV1...),
		body...,
	))
}
