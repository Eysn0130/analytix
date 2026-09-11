package nativecomponent

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OperationFundsDeterministicCleaningV1 = "funds.deterministic_cleaning_v1"
	DeterministicCleaningRuleContractV1   = `{"contract":"analytix.funds.deterministic-cleaning-rules/v1","accountIdentity":{"operation":"remove_ascii_space_hyphen_underscore","optional":["counterpartyAccount"],"requiredAny":["card","account"]},"currency":{"aliases":["CNY","RMB","人民币"],"output":"CNY"},"decimal":{"grammar":"^[+-]?[0-9]+(?:\\.[0-9]{0,2})?$","minorUnitScale":2,"numeric":"checked_i128","output":"canonical_plain_up_to_scale","required":["amount"]},"direction":{"in":["进","入","收入","贷","credit","in"],"out":["出","支出","借","debit","out"]},"forbiddenText":"control_or_bidi_format","null":"empty_to_canonical_null","order":["txn_ts","id","file_id","row_no"],"schemaVersion":1,"text":"trim_unicode","timestamp":{"formats":["%Y-%m-%d %H:%M:%S","%Y/%m/%d %H:%M:%S","%Y-%m-%dT%H:%M:%S"],"output":"%Y-%m-%d %H:%M:%S"}}`

	DeterministicCleaningMaximumSourceBytesV1  = 64 * 1024 * 1024
	DeterministicCleaningMaximumRowsV1         = uint64(100_000)
	DeterministicCleaningMaximumResultBytesV1  = 1 * 1024 * 1024
	DeterministicCleaningMaximumResultTokensV1 = 32_768
	DeterministicCleaningMaximumFrameBytesV1   = 16 * 1024

	deterministicCleaningResultBaseTokensV1 = 27
	deterministicCleaningResultRowTokensV1  = 9
	deterministicCleaningResultCellTokensV1 = 12
)

var deterministicCleaningGenerationDomainV1 = []byte("AnalytixFundsDeterministicCleaningGenerationV1\x00")
var deterministicCleaningResultDomainV1 = []byte("analytix.funds.deterministic-cleaning-result/v1\x00")

var deterministicCleaningFieldsV1 = []DirectSourcePreviewFieldV1{
	DirectSourcePreviewFieldTransactionTimeV1,
	DirectSourcePreviewFieldAccountV1,
	DirectSourcePreviewFieldCardV1,
	DirectSourcePreviewFieldAccountNameV1,
	DirectSourcePreviewFieldIdentityNumberV1,
	DirectSourcePreviewFieldAmountTextV1,
	DirectSourcePreviewFieldDirectionV1,
	DirectSourcePreviewFieldCounterpartyAccountV1,
	DirectSourcePreviewFieldCounterpartyNameV1,
	DirectSourcePreviewFieldCounterpartyIdentityNumberV1,
	DirectSourcePreviewFieldCounterpartyBankV1,
	DirectSourcePreviewFieldSummaryV1,
	DirectSourcePreviewFieldCurrencyV1,
	DirectSourcePreviewFieldMerchantNameV1,
	DirectSourcePreviewFieldRemarkV1,
}

func DeterministicCleaningRuleDigestV1() string {
	return domainsecurity.SHA256Hex([]byte(DeterministicCleaningRuleContractV1))
}

func DeterministicCleaningRuleGenerationV1() string {
	return "tlgen1_" + domainsecurity.SHA256Hex(append(
		append([]byte(nil), deterministicCleaningGenerationDomainV1...),
		[]byte(DeterministicCleaningRuleDigestV1())...,
	))
}

type DeterministicCleaningArgumentsV1 struct {
	CaseID                         string `json:"caseId"`
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	RuleGeneration                 string `json:"ruleGeneration"`
	RuleDigest                     string `json:"ruleDigest"`
	DatasetUTCOffsetMinutes        int16  `json:"datasetUtcOffsetMinutes"`
	ExpectedCurrency               string `json:"expectedCurrency"`
	MinorUnitScale                 uint8  `json:"minorUnitScale"`
}

func NewDeterministicCleaningArgumentsV1(
	descriptor domainfundsquerysource.DescriptorV1,
) (DeterministicCleaningArgumentsV1, error) {
	arguments := DeterministicCleaningArgumentsV1{
		CaseID: descriptor.CaseID, DatasetSnapshotID: descriptor.DatasetSnapshotID,
		CaseBindingHash:                descriptor.CaseBindingHash,
		ExpectedProducerContentID:      descriptor.FundsProducerContentID,
		ExpectedProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
		RuleGeneration:                 DeterministicCleaningRuleGenerationV1(),
		RuleDigest:                     DeterministicCleaningRuleDigestV1(),
		DatasetUTCOffsetMinutes:        descriptor.DatasetUTCOffsetMinutes,
		ExpectedCurrency:               descriptor.ExpectedCurrency, MinorUnitScale: descriptor.MinorUnitScale,
	}
	if ValidateDeterministicCleaningArgumentsAuthorityV1(arguments, descriptor) != nil {
		return DeterministicCleaningArgumentsV1{}, ErrRequestInvalid
	}
	return arguments, nil
}

func ValidateDeterministicCleaningArgumentsV1(arguments DeterministicCleaningArgumentsV1) error {
	if arguments.CaseID == "" || arguments.CaseID != strings.TrimSpace(arguments.CaseID) ||
		len(arguments.CaseID) > 512 || strings.IndexFunc(arguments.CaseID, unicode.IsControl) >= 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(arguments.DatasetSnapshotID) ||
		!canonicalLowerDigestV1(arguments.CaseBindingHash) ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(arguments.ExpectedProducerContentID) ||
		!canonicalLowerDigestV1(arguments.ExpectedProducerManifestSHA256) ||
		arguments.RuleGeneration != DeterministicCleaningRuleGenerationV1() ||
		arguments.RuleDigest != DeterministicCleaningRuleDigestV1() ||
		arguments.DatasetUTCOffsetMinutes < -840 || arguments.DatasetUTCOffsetMinutes > 840 ||
		arguments.ExpectedCurrency != "CNY" || arguments.MinorUnitScale != 2 {
		return ErrRequestInvalid
	}
	return nil
}

func ValidateDeterministicCleaningArgumentsAuthorityV1(
	arguments DeterministicCleaningArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
) error {
	if ValidateDeterministicCleaningArgumentsV1(arguments) != nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil ||
		arguments.CaseID != descriptor.CaseID ||
		arguments.DatasetSnapshotID != descriptor.DatasetSnapshotID ||
		arguments.CaseBindingHash != descriptor.CaseBindingHash ||
		arguments.ExpectedProducerContentID != descriptor.FundsProducerContentID ||
		arguments.ExpectedProducerManifestSHA256 != descriptor.FundsProducerContentManifestSHA256 ||
		arguments.DatasetUTCOffsetMinutes != descriptor.DatasetUTCOffsetMinutes ||
		arguments.ExpectedCurrency != descriptor.ExpectedCurrency ||
		arguments.MinorUnitScale != descriptor.MinorUnitScale ||
		!domainfundsquerysource.QueryProfileSupportsDirectSourcePreviewV1(descriptor.QueryProfileDigest) {
		return ErrContextMismatch
	}
	return nil
}

type deterministicCleaningNativeRequestFrameV1 struct {
	RequestID string                           `json:"request_id"`
	Command   string                           `json:"command"`
	CaseID    string                           `json:"case_id"`
	Payload   DeterministicCleaningArgumentsV1 `json:"payload"`
}

func EncodeDeterministicCleaningNativeRequestFrameV1(
	requestID string,
	arguments DeterministicCleaningArgumentsV1,
) ([]byte, error) {
	if !canonicalLowerDigestV1(requestID) || ValidateDeterministicCleaningArgumentsV1(arguments) != nil {
		return nil, ErrRequestInvalid
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(deterministicCleaningNativeRequestFrameV1{
		RequestID: requestID, Command: OperationFundsDeterministicCleaningV1,
		CaseID: arguments.CaseID, Payload: arguments,
	}) != nil || output.Len() == 0 || output.Len() > DeterministicCleaningMaximumFrameBytesV1 {
		return nil, ErrRequestInvalid
	}
	frame := output.Bytes()
	if frame[len(frame)-1] != '\n' || bytes.IndexByte(frame[:len(frame)-1], '\n') >= 0 ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		return nil, ErrRequestInvalid
	}
	return append([]byte(nil), frame...), nil
}

type deterministicCleaningChangedCellWireV1 struct {
	Field       DirectSourcePreviewFieldV1 `json:"field"`
	BeforeValue string                     `json:"beforeValue"`
	AfterValue  string                     `json:"afterValue"`
	BeforeState string                     `json:"beforeState"`
	AfterState  string                     `json:"afterState"`
}

type deterministicCleaningChangedRowWireV1 struct {
	RowIndex uint32                                   `json:"rowIndex"`
	Status   string                                   `json:"status"`
	Cells    []deterministicCleaningChangedCellWireV1 `json:"cells"`
}

type deterministicCleaningResultWireV1 struct {
	SchemaVersion            uint8                                   `json:"schemaVersion"`
	Operation                string                                  `json:"operation"`
	InputDatasetSnapshotID   string                                  `json:"inputDatasetSnapshotId"`
	RuleGeneration           string                                  `json:"ruleGeneration"`
	RuleDigest               string                                  `json:"ruleDigest"`
	OutputArtifactSHA256     string                                  `json:"outputArtifactSha256"`
	OutputArtifactByteLength uint64                                  `json:"outputArtifactByteLength"`
	RowCount                 uint64                                  `json:"rowCount"`
	ChangedRowCount          uint64                                  `json:"changedRowCount"`
	UnchangedRowCount        uint64                                  `json:"unchangedRowCount"`
	ChangedRows              []deterministicCleaningChangedRowWireV1 `json:"changedRows"`
	ResultDigest             string                                  `json:"resultDigest"`
}

type deterministicCleaningResultDigestMaterialV1 struct {
	SchemaVersion            uint8                                   `json:"schemaVersion"`
	Operation                string                                  `json:"operation"`
	InputDatasetSnapshotID   string                                  `json:"inputDatasetSnapshotId"`
	RuleGeneration           string                                  `json:"ruleGeneration"`
	RuleDigest               string                                  `json:"ruleDigest"`
	OutputArtifactSHA256     string                                  `json:"outputArtifactSha256"`
	OutputArtifactByteLength uint64                                  `json:"outputArtifactByteLength"`
	RowCount                 uint64                                  `json:"rowCount"`
	ChangedRowCount          uint64                                  `json:"changedRowCount"`
	UnchangedRowCount        uint64                                  `json:"unchangedRowCount"`
	ChangedRows              []deterministicCleaningChangedRowWireV1 `json:"changedRows"`
}

type DeterministicCleaningChangedCellV1 struct {
	Field       DirectSourcePreviewFieldV1
	beforeValue string
	afterValue  string
	beforeState string
	afterState  string
}

func (cell DeterministicCleaningChangedCellV1) UseExactV1(
	use func(field DirectSourcePreviewFieldV1, beforeValue, afterValue, beforeState, afterState string) error,
) error {
	if use == nil || !validDeterministicCleaningCellV1(cell) {
		return ErrResultInvalid
	}
	return use(cell.Field, cell.beforeValue, cell.afterValue, cell.beforeState, cell.afterState)
}

func (DeterministicCleaningChangedCellV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}
func (*DeterministicCleaningChangedCellV1) UnmarshalJSON([]byte) error { return ErrResultInvalid }

func (cell DeterministicCleaningChangedCellV1) String() string {
	return fmt.Sprintf("DeterministicCleaningChangedCellV1{field:%q,values:[PRIVATE]}", cell.Field)
}
func (cell DeterministicCleaningChangedCellV1) GoString() string { return cell.String() }

type DeterministicCleaningChangedRowV1 struct {
	RowIndex uint32
	Status   string
	Cells    []DeterministicCleaningChangedCellV1
}

func (DeterministicCleaningChangedRowV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}
func (*DeterministicCleaningChangedRowV1) UnmarshalJSON([]byte) error { return ErrResultInvalid }
func (row DeterministicCleaningChangedRowV1) String() string {
	return fmt.Sprintf(
		"DeterministicCleaningChangedRowV1{rowIndex:%d,status:%q,cellCount:%d,cells:[PRIVATE]}",
		row.RowIndex, row.Status, len(row.Cells),
	)
}
func (row DeterministicCleaningChangedRowV1) GoString() string { return row.String() }

type DeterministicCleaningResultV1 struct {
	SchemaVersion            uint8
	Operation                string
	InputDatasetSnapshotID   string
	RuleGeneration           string
	RuleDigest               string
	OutputArtifactSHA256     string
	OutputArtifactByteLength uint64
	RowCount                 uint64
	ChangedRowCount          uint64
	UnchangedRowCount        uint64
	ChangedRows              []DeterministicCleaningChangedRowV1
	ResultDigest             string
}

func (DeterministicCleaningResultV1) MarshalJSON() ([]byte, error) { return nil, ErrResultInvalid }
func (*DeterministicCleaningResultV1) UnmarshalJSON([]byte) error  { return ErrResultInvalid }
func (result DeterministicCleaningResultV1) String() string {
	return fmt.Sprintf(
		"DeterministicCleaningResultV1{schemaVersion:%d,operation:%q,inputDatasetSnapshotId:%q,ruleGeneration:%q,ruleDigest:%q,outputArtifactSha256:%q,outputArtifactByteLength:%d,rowCount:%d,changedRowCount:%d,unchangedRowCount:%d,resultDigest:%q,rows:[PRIVATE]}",
		result.SchemaVersion, result.Operation, result.InputDatasetSnapshotID,
		result.RuleGeneration, result.RuleDigest, result.OutputArtifactSHA256,
		result.OutputArtifactByteLength, result.RowCount, result.ChangedRowCount,
		result.UnchangedRowCount, result.ResultDigest,
	)
}
func (result DeterministicCleaningResultV1) GoString() string { return result.String() }

func ParseDeterministicCleaningResultV1(
	raw []byte,
	arguments DeterministicCleaningArgumentsV1,
) (DeterministicCleaningResultV1, error) {
	if len(raw) == 0 || len(raw) > DeterministicCleaningMaximumResultBytesV1 ||
		ValidateDeterministicCleaningArgumentsV1(arguments) != nil ||
		domainjsonstrict.Validate(raw, domainjsonstrict.Options{
			RequireObject: true, MaxBytes: DeterministicCleaningMaximumResultBytesV1,
			MaxDepth: 8, MaxTokens: DeterministicCleaningMaximumResultTokensV1,
			MaxStringBytes: DirectSourcePreviewMaximumCellBytesV1,
			MaxNumberBytes: 32, MaxAbsExponent: 1,
		}) != nil {
		return DeterministicCleaningResultV1{}, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire deterministicCleaningResultWireV1
	if decoder.Decode(&wire) != nil {
		return DeterministicCleaningResultV1{}, ErrResultInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) ||
		validateDeterministicCleaningResultWireV1(wire, arguments) != nil {
		return DeterministicCleaningResultV1{}, ErrResultInvalid
	}
	rows := make([]DeterministicCleaningChangedRowV1, len(wire.ChangedRows))
	for rowIndex, row := range wire.ChangedRows {
		cells := make([]DeterministicCleaningChangedCellV1, len(row.Cells))
		for cellIndex, cell := range row.Cells {
			cells[cellIndex] = DeterministicCleaningChangedCellV1{
				Field: cell.Field, beforeValue: cell.BeforeValue, afterValue: cell.AfterValue,
				beforeState: cell.BeforeState, afterState: cell.AfterState,
			}
		}
		rows[rowIndex] = DeterministicCleaningChangedRowV1{RowIndex: row.RowIndex, Status: row.Status, Cells: cells}
	}
	return DeterministicCleaningResultV1{
		SchemaVersion: wire.SchemaVersion, Operation: wire.Operation,
		InputDatasetSnapshotID: wire.InputDatasetSnapshotID,
		RuleGeneration:         wire.RuleGeneration, RuleDigest: wire.RuleDigest,
		OutputArtifactSHA256:     wire.OutputArtifactSHA256,
		OutputArtifactByteLength: wire.OutputArtifactByteLength,
		RowCount:                 wire.RowCount, ChangedRowCount: wire.ChangedRowCount,
		UnchangedRowCount: wire.UnchangedRowCount, ChangedRows: rows,
		ResultDigest: wire.ResultDigest,
	}, nil
}

func validateDeterministicCleaningResultWireV1(
	wire deterministicCleaningResultWireV1,
	arguments DeterministicCleaningArgumentsV1,
) error {
	if wire.SchemaVersion != 1 || wire.Operation != OperationFundsDeterministicCleaningV1 ||
		wire.InputDatasetSnapshotID != arguments.DatasetSnapshotID ||
		wire.RuleGeneration != arguments.RuleGeneration || wire.RuleDigest != arguments.RuleDigest ||
		!canonicalLowerDigestV1(wire.OutputArtifactSHA256) ||
		wire.OutputArtifactByteLength == 0 || wire.OutputArtifactByteLength > DeterministicCleaningMaximumSourceBytesV1 ||
		wire.RowCount == 0 || wire.RowCount > DeterministicCleaningMaximumRowsV1 ||
		wire.ChangedRowCount > wire.RowCount || wire.UnchangedRowCount != wire.RowCount-wire.ChangedRowCount ||
		uint64(len(wire.ChangedRows)) != wire.ChangedRowCount || !canonicalLowerDigestV1(wire.ResultDigest) {
		return ErrResultInvalid
	}
	var previous uint32
	fieldOrder := make(map[DirectSourcePreviewFieldV1]int, len(deterministicCleaningFieldsV1))
	for index, field := range deterministicCleaningFieldsV1 {
		fieldOrder[field] = index
	}
	for rowIndex, row := range wire.ChangedRows {
		if uint64(row.RowIndex) >= wire.RowCount || rowIndex > 0 && row.RowIndex <= previous ||
			row.Status == "unchanged" || !validCleaningStatusV1(row.Status) ||
			len(row.Cells) == 0 || len(row.Cells) > len(deterministicCleaningFieldsV1) ||
			!cleaningStatusMatchesCellsV1(row) {
			return ErrResultInvalid
		}
		previous = row.RowIndex
		seen := make(map[DirectSourcePreviewFieldV1]bool, len(row.Cells))
		previousField := -1
		for _, cell := range row.Cells {
			candidate := DeterministicCleaningChangedCellV1{
				Field: cell.Field, beforeValue: cell.BeforeValue, afterValue: cell.AfterValue,
				beforeState: cell.BeforeState, afterState: cell.AfterState,
			}
			order, ok := fieldOrder[cell.Field]
			if !ok || seen[cell.Field] || order <= previousField || !validDeterministicCleaningCellV1(candidate) {
				return ErrResultInvalid
			}
			seen[cell.Field] = true
			previousField = order
		}
	}
	canonicalWire, err := json.Marshal(wire)
	if err != nil || len(canonicalWire) > DeterministicCleaningMaximumResultBytesV1 ||
		deterministicCleaningResultTokenCountV1(wire.ChangedRows) > DeterministicCleaningMaximumResultTokensV1 {
		return ErrResultInvalid
	}
	material := deterministicCleaningResultDigestMaterialV1{
		SchemaVersion: wire.SchemaVersion, Operation: wire.Operation,
		InputDatasetSnapshotID: wire.InputDatasetSnapshotID,
		RuleGeneration:         wire.RuleGeneration, RuleDigest: wire.RuleDigest,
		OutputArtifactSHA256:     wire.OutputArtifactSHA256,
		OutputArtifactByteLength: wire.OutputArtifactByteLength,
		RowCount:                 wire.RowCount, ChangedRowCount: wire.ChangedRowCount,
		UnchangedRowCount: wire.UnchangedRowCount, ChangedRows: wire.ChangedRows,
	}
	if deterministicCleaningResultDigestV1(material) != wire.ResultDigest {
		return ErrResultInvalid
	}
	return nil
}

func cleaningStatusMatchesCellsV1(row deterministicCleaningChangedRowWireV1) bool {
	if len(row.Cells) == 0 {
		return false
	}
	invalid := false
	added := true
	removed := true
	for _, cell := range row.Cells {
		invalid = invalid || cell.BeforeState == "invalid" || cell.AfterState == "invalid"
		added = added && (cell.BeforeState == "missing" || cell.BeforeState == "null") && cell.AfterState == "value"
		removed = removed && cell.BeforeState == "value" && (cell.AfterState == "missing" || cell.AfterState == "null")
	}
	expected := "changed"
	if invalid {
		expected = "invalid"
	} else if added {
		expected = "added"
	} else if removed {
		expected = "removed"
	}
	return row.Status == expected
}

func deterministicCleaningResultTokenCountV1(rows []deterministicCleaningChangedRowWireV1) int {
	tokens := deterministicCleaningResultBaseTokensV1
	for _, row := range rows {
		tokens += deterministicCleaningResultRowTokensV1 + deterministicCleaningResultCellTokensV1*len(row.Cells)
	}
	return tokens
}

func validDeterministicCleaningCellV1(cell DeterministicCleaningChangedCellV1) bool {
	if _, ok := directSourcePreviewFieldsV1[cell.Field]; !ok ||
		!utf8.ValidString(cell.beforeValue) || !utf8.ValidString(cell.afterValue) ||
		len(cell.beforeValue) > DirectSourcePreviewMaximumCellBytesV1 ||
		len(cell.afterValue) > DirectSourcePreviewMaximumCellBytesV1 {
		return false
	}
	for _, state := range []string{cell.beforeState, cell.afterState} {
		if state != "value" && state != "missing" && state != "null" && state != "invalid" {
			return false
		}
	}
	if (cell.beforeState == "missing" || cell.beforeState == "null") && cell.beforeValue != "" ||
		(cell.afterState == "missing" || cell.afterState == "null") && cell.afterValue != "" ||
		cell.beforeState == cell.afterState && cell.beforeValue == cell.afterValue {
		return false
	}
	return true
}

func validCleaningStatusV1(status string) bool {
	return status == "unchanged" || status == "changed" || status == "added" ||
		status == "removed" || status == "invalid"
}

func deterministicCleaningResultDigestV1(material deterministicCleaningResultDigestMaterialV1) string {
	body, err := json.Marshal(material)
	if err != nil {
		return ""
	}
	hasher := sha256.New()
	_, _ = hasher.Write(deterministicCleaningResultDomainV1)
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(body)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write(body)
	return hex.EncodeToString(hasher.Sum(nil))
}
