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
	OperationFundsDirectSourcePreview = "funds.direct_source_preview"

	DirectSourcePreviewPurposeV1             = "analytix.direct-source-preview/v1"
	DirectSourcePreviewViewTransactionsV1    = "transactions"
	DirectSourcePreviewCurrentnessRequiredV1 = "host_revalidation_required"

	DirectSourcePreviewMaximumRowsV1                = uint16(100)
	DirectSourcePreviewMaximumOffsetV1              = uint32(100_000)
	DirectSourcePreviewMaximumCellBytesV1           = 4 * 1024
	DirectSourcePreviewMaximumResultBytesV1         = 1 * 1024 * 1024
	DirectSourcePreviewMaximumNativeFrameBytesV1    = 32 * 1024
	DirectSourcePreviewMaximumNativeResponseBytesV1 = DirectSourcePreviewMaximumResultBytesV1 + 16*1024
)

type DirectSourcePreviewFieldV1 string

const (
	DirectSourcePreviewFieldTransactionTimeV1            DirectSourcePreviewFieldV1 = "transactionTime"
	DirectSourcePreviewFieldAccountV1                    DirectSourcePreviewFieldV1 = "account"
	DirectSourcePreviewFieldCardV1                       DirectSourcePreviewFieldV1 = "card"
	DirectSourcePreviewFieldAccountNameV1                DirectSourcePreviewFieldV1 = "accountName"
	DirectSourcePreviewFieldIdentityNumberV1             DirectSourcePreviewFieldV1 = "identityNumber"
	DirectSourcePreviewFieldAmountTextV1                 DirectSourcePreviewFieldV1 = "amountText"
	DirectSourcePreviewFieldDirectionV1                  DirectSourcePreviewFieldV1 = "direction"
	DirectSourcePreviewFieldCounterpartyAccountV1        DirectSourcePreviewFieldV1 = "counterpartyAccount"
	DirectSourcePreviewFieldCounterpartyNameV1           DirectSourcePreviewFieldV1 = "counterpartyName"
	DirectSourcePreviewFieldCounterpartyIdentityNumberV1 DirectSourcePreviewFieldV1 = "counterpartyIdentityNumber"
	DirectSourcePreviewFieldCounterpartyBankV1           DirectSourcePreviewFieldV1 = "counterpartyBank"
	DirectSourcePreviewFieldSummaryV1                    DirectSourcePreviewFieldV1 = "summary"
	DirectSourcePreviewFieldCurrencyV1                   DirectSourcePreviewFieldV1 = "currency"
	DirectSourcePreviewFieldMerchantNameV1               DirectSourcePreviewFieldV1 = "merchantName"
	DirectSourcePreviewFieldRemarkV1                     DirectSourcePreviewFieldV1 = "remark"
)

var directSourcePreviewFieldsV1 = map[DirectSourcePreviewFieldV1]struct{}{
	DirectSourcePreviewFieldTransactionTimeV1: {}, DirectSourcePreviewFieldAccountV1: {},
	DirectSourcePreviewFieldCardV1: {}, DirectSourcePreviewFieldAccountNameV1: {},
	DirectSourcePreviewFieldIdentityNumberV1: {}, DirectSourcePreviewFieldAmountTextV1: {},
	DirectSourcePreviewFieldDirectionV1: {}, DirectSourcePreviewFieldCounterpartyAccountV1: {},
	DirectSourcePreviewFieldCounterpartyNameV1: {}, DirectSourcePreviewFieldCounterpartyIdentityNumberV1: {},
	DirectSourcePreviewFieldCounterpartyBankV1: {}, DirectSourcePreviewFieldSummaryV1: {},
	DirectSourcePreviewFieldCurrencyV1: {}, DirectSourcePreviewFieldMerchantNameV1: {},
	DirectSourcePreviewFieldRemarkV1: {},
}

type DirectSourcePreviewArgumentsV1 struct {
	CaseID                         string                       `json:"caseId"`
	DatasetSnapshotID              string                       `json:"datasetSnapshotId"`
	CaseBindingHash                string                       `json:"caseBindingHash"`
	ExpectedProducerContentID      string                       `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string                       `json:"expectedProducerManifestSha256"`
	Fields                         []DirectSourcePreviewFieldV1 `json:"fields"`
	RowOffset                      uint32                       `json:"rowOffset"`
	RowLimit                       uint16                       `json:"rowLimit"`
	DatasetUTCOffsetMinutes        int16                        `json:"datasetUtcOffsetMinutes"`
	ExpectedCurrency               string                       `json:"expectedCurrency"`
	MinorUnitScale                 uint8                        `json:"minorUnitScale"`
}

func NewDirectSourcePreviewArgumentsV1(
	descriptor domainfundsquerysource.DescriptorV1,
	fields []DirectSourcePreviewFieldV1,
	rowOffset uint32,
	rowLimit uint16,
) (DirectSourcePreviewArgumentsV1, error) {
	arguments := DirectSourcePreviewArgumentsV1{
		CaseID: descriptor.CaseID, DatasetSnapshotID: descriptor.DatasetSnapshotID,
		CaseBindingHash:                descriptor.CaseBindingHash,
		ExpectedProducerContentID:      descriptor.FundsProducerContentID,
		ExpectedProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
		Fields:                         append([]DirectSourcePreviewFieldV1(nil), fields...),
		RowOffset:                      rowOffset, RowLimit: rowLimit,
		DatasetUTCOffsetMinutes: descriptor.DatasetUTCOffsetMinutes,
		ExpectedCurrency:        descriptor.ExpectedCurrency, MinorUnitScale: descriptor.MinorUnitScale,
	}
	if ValidateDirectSourcePreviewArgumentsAuthorityV1(arguments, descriptor) != nil {
		return DirectSourcePreviewArgumentsV1{}, ErrRequestInvalid
	}
	return arguments, nil
}

func ValidateDirectSourcePreviewArgumentsV1(arguments DirectSourcePreviewArgumentsV1) error {
	if arguments.CaseID == "" || arguments.CaseID != strings.TrimSpace(arguments.CaseID) ||
		len(arguments.CaseID) > 512 || strings.IndexFunc(arguments.CaseID, unicode.IsControl) >= 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(arguments.DatasetSnapshotID) ||
		!canonicalLowerDigestV1(arguments.CaseBindingHash) ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(arguments.ExpectedProducerContentID) ||
		!canonicalLowerDigestV1(arguments.ExpectedProducerManifestSHA256) ||
		len(arguments.Fields) == 0 || len(arguments.Fields) > len(directSourcePreviewFieldsV1) ||
		arguments.RowOffset > DirectSourcePreviewMaximumOffsetV1 ||
		arguments.RowLimit == 0 || arguments.RowLimit > DirectSourcePreviewMaximumRowsV1 ||
		arguments.DatasetUTCOffsetMinutes < -840 || arguments.DatasetUTCOffsetMinutes > 840 ||
		len(arguments.ExpectedCurrency) != 3 || arguments.MinorUnitScale != 2 {
		return ErrRequestInvalid
	}
	for _, character := range arguments.ExpectedCurrency {
		if character < 'A' || character > 'Z' {
			return ErrRequestInvalid
		}
	}
	seen := make(map[DirectSourcePreviewFieldV1]bool, len(arguments.Fields))
	for _, field := range arguments.Fields {
		if _, ok := directSourcePreviewFieldsV1[field]; !ok || seen[field] {
			return ErrRequestInvalid
		}
		seen[field] = true
	}
	return nil
}

func ValidateDirectSourcePreviewArgumentsAuthorityV1(
	arguments DirectSourcePreviewArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
) error {
	if ValidateDirectSourcePreviewArgumentsV1(arguments) != nil ||
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

type directSourcePreviewNativeRequestFrameWireV1 struct {
	RequestID string                         `json:"request_id"`
	Command   string                         `json:"command"`
	CaseID    string                         `json:"case_id"`
	Payload   DirectSourcePreviewArgumentsV1 `json:"payload"`
}

func EncodeDirectSourcePreviewNativeRequestFrameV1(
	requestID string,
	arguments DirectSourcePreviewArgumentsV1,
) ([]byte, error) {
	if !canonicalLowerDigestV1(requestID) || ValidateDirectSourcePreviewArgumentsV1(arguments) != nil {
		return nil, ErrRequestInvalid
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(directSourcePreviewNativeRequestFrameWireV1{
		RequestID: requestID, Command: OperationFundsDirectSourcePreview,
		CaseID: arguments.CaseID, Payload: arguments,
	}) != nil || output.Len() == 0 || output.Len() > DirectSourcePreviewMaximumNativeFrameBytesV1 {
		return nil, ErrRequestInvalid
	}
	frame := output.Bytes()
	if frame[len(frame)-1] != '\n' || bytes.IndexByte(frame[:len(frame)-1], '\n') >= 0 ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		return nil, ErrRequestInvalid
	}
	return append([]byte(nil), frame...), nil
}

type directSourcePreviewCellWireV1 struct {
	Field DirectSourcePreviewFieldV1 `json:"field"`
	Value string                     `json:"value"`
}

type directSourcePreviewRowWireV1 struct {
	RowIndex uint32                          `json:"rowIndex"`
	Cells    []directSourcePreviewCellWireV1 `json:"cells"`
}

type directSourcePreviewResultWireV1 struct {
	SchemaVersion     uint8                          `json:"schemaVersion"`
	Purpose           string                         `json:"purpose"`
	DatasetSnapshotID string                         `json:"datasetSnapshotId"`
	Fields            []DirectSourcePreviewFieldV1   `json:"fields"`
	RowOffset         uint32                         `json:"rowOffset"`
	RowLimit          uint16                         `json:"rowLimit"`
	Rows              []directSourcePreviewRowWireV1 `json:"rows"`
	HasMore           bool                           `json:"hasMore"`
	QueryHash         string                         `json:"queryHash"`
	ResultHash        string                         `json:"resultHash"`
	Currentness       string                         `json:"currentness"`
}

type directSourcePreviewResultHashMaterialV1 struct {
	SchemaVersion     uint8                          `json:"schemaVersion"`
	Purpose           string                         `json:"purpose"`
	DatasetSnapshotID string                         `json:"datasetSnapshotId"`
	Fields            []DirectSourcePreviewFieldV1   `json:"fields"`
	RowOffset         uint32                         `json:"rowOffset"`
	RowLimit          uint16                         `json:"rowLimit"`
	Rows              []directSourcePreviewRowWireV1 `json:"rows"`
	HasMore           bool                           `json:"hasMore"`
	QueryHash         string                         `json:"queryHash"`
	Currentness       string                         `json:"currentness"`
}

type DirectSourcePreviewCellV1 struct {
	Field DirectSourcePreviewFieldV1
	value string
}

func (cell DirectSourcePreviewCellV1) UseExactV1(use func(string) error) error {
	if _, ok := directSourcePreviewFieldsV1[cell.Field]; !ok || use == nil ||
		!utf8.ValidString(cell.value) || len(cell.value) > DirectSourcePreviewMaximumCellBytesV1 {
		return ErrResultInvalid
	}
	return use(cell.value)
}

func (DirectSourcePreviewCellV1) MarshalJSON() ([]byte, error) { return nil, ErrResultInvalid }
func (*DirectSourcePreviewCellV1) UnmarshalJSON([]byte) error  { return ErrResultInvalid }
func (cell DirectSourcePreviewCellV1) String() string {
	return fmt.Sprintf("DirectSourcePreviewCellV1{field:%q,value:[PRIVATE]}", cell.Field)
}
func (cell DirectSourcePreviewCellV1) GoString() string { return cell.String() }

type DirectSourcePreviewRowV1 struct {
	RowIndex uint32
	Cells    []DirectSourcePreviewCellV1
}

type DirectSourcePreviewResultV1 struct {
	SchemaVersion     uint8
	Purpose           string
	DatasetSnapshotID string
	Fields            []DirectSourcePreviewFieldV1
	RowOffset         uint32
	RowLimit          uint16
	Rows              []DirectSourcePreviewRowV1
	HasMore           bool
	QueryHash         string
	ResultHash        string
	Currentness       string
}

func (DirectSourcePreviewResultV1) MarshalJSON() ([]byte, error) { return nil, ErrResultInvalid }
func (*DirectSourcePreviewResultV1) UnmarshalJSON([]byte) error  { return ErrResultInvalid }
func (result DirectSourcePreviewResultV1) String() string {
	return fmt.Sprintf(
		"DirectSourcePreviewResultV1{datasetSnapshotId:%q,fieldCount:%d,rowOffset:%d,rowLimit:%d,rowCount:%d,hasMore:%t,currentness:%q,rows:[PRIVATE]}",
		result.DatasetSnapshotID, len(result.Fields), result.RowOffset, result.RowLimit,
		len(result.Rows), result.HasMore, result.Currentness,
	)
}
func (result DirectSourcePreviewResultV1) GoString() string { return result.String() }

func ParseDirectSourcePreviewResultV1(
	raw []byte,
	arguments DirectSourcePreviewArgumentsV1,
) (DirectSourcePreviewResultV1, error) {
	if len(raw) == 0 || len(raw) > DirectSourcePreviewMaximumResultBytesV1 ||
		ValidateDirectSourcePreviewArgumentsV1(arguments) != nil ||
		domainjsonstrict.Validate(raw, domainjsonstrict.Options{
			RequireObject: true, MaxBytes: DirectSourcePreviewMaximumResultBytesV1,
			MaxDepth: 8, MaxTokens: 16_384, MaxStringBytes: DirectSourcePreviewMaximumCellBytesV1,
			MaxNumberBytes: 32, MaxAbsExponent: 1,
		}) != nil {
		return DirectSourcePreviewResultV1{}, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire directSourcePreviewResultWireV1
	if decoder.Decode(&wire) != nil {
		return DirectSourcePreviewResultV1{}, ErrResultInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) ||
		validateDirectSourcePreviewResultWireV1(wire, arguments) != nil {
		return DirectSourcePreviewResultV1{}, ErrResultInvalid
	}
	rows := make([]DirectSourcePreviewRowV1, len(wire.Rows))
	for rowIndex, row := range wire.Rows {
		cells := make([]DirectSourcePreviewCellV1, len(row.Cells))
		for cellIndex, cell := range row.Cells {
			cells[cellIndex] = DirectSourcePreviewCellV1{Field: cell.Field, value: cell.Value}
		}
		rows[rowIndex] = DirectSourcePreviewRowV1{RowIndex: row.RowIndex, Cells: cells}
	}
	return DirectSourcePreviewResultV1{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		DatasetSnapshotID: wire.DatasetSnapshotID,
		Fields:            append([]DirectSourcePreviewFieldV1(nil), wire.Fields...),
		RowOffset:         wire.RowOffset, RowLimit: wire.RowLimit, Rows: rows,
		HasMore: wire.HasMore, QueryHash: wire.QueryHash,
		ResultHash: wire.ResultHash, Currentness: wire.Currentness,
	}, nil
}

func validateDirectSourcePreviewResultWireV1(
	wire directSourcePreviewResultWireV1,
	arguments DirectSourcePreviewArgumentsV1,
) error {
	if wire.SchemaVersion != 1 || wire.Purpose != DirectSourcePreviewPurposeV1 ||
		wire.DatasetSnapshotID != arguments.DatasetSnapshotID ||
		wire.RowOffset != arguments.RowOffset || wire.RowLimit != arguments.RowLimit ||
		len(wire.Fields) != len(arguments.Fields) || len(wire.Rows) > int(arguments.RowLimit) ||
		wire.Currentness != DirectSourcePreviewCurrentnessRequiredV1 ||
		!canonicalLowerDigestV1(wire.QueryHash) || !canonicalLowerDigestV1(wire.ResultHash) {
		return ErrResultInvalid
	}
	for index, field := range wire.Fields {
		if field != arguments.Fields[index] {
			return ErrResultInvalid
		}
	}
	for index, row := range wire.Rows {
		if row.RowIndex > DirectSourcePreviewMaximumOffsetV1 ||
			row.RowIndex != arguments.RowOffset+uint32(index) || len(row.Cells) != len(wire.Fields) {
			return ErrResultInvalid
		}
		for cellIndex, cell := range row.Cells {
			if cell.Field != wire.Fields[cellIndex] || !utf8.ValidString(cell.Value) ||
				len(cell.Value) > DirectSourcePreviewMaximumCellBytesV1 {
				return ErrResultInvalid
			}
		}
	}
	material := directSourcePreviewResultHashMaterialV1{
		SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose,
		DatasetSnapshotID: wire.DatasetSnapshotID, Fields: wire.Fields,
		RowOffset: wire.RowOffset, RowLimit: wire.RowLimit, Rows: wire.Rows,
		HasMore: wire.HasMore, QueryHash: wire.QueryHash, Currentness: wire.Currentness,
	}
	if wire.ResultHash != directSourcePreviewResultHashV1(material) {
		return ErrResultInvalid
	}
	return nil
}

func directSourcePreviewResultHashV1(material directSourcePreviewResultHashMaterialV1) string {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(material) != nil {
		return ""
	}
	encoded := bytes.TrimSuffix(body.Bytes(), []byte{'\n'})
	encoded = bytes.ReplaceAll(encoded, []byte(`\u2028`), []byte("\u2028"))
	encoded = bytes.ReplaceAll(encoded, []byte(`\u2029`), []byte("\u2029"))
	hasher := sha256.New()
	writeDirectSourcePreviewHashFrameV1(hasher, []byte("analytix.direct-source-preview-result-hash/v1"))
	writeDirectSourcePreviewHashFrameV1(hasher, encoded)
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeDirectSourcePreviewHashFrameV1(writer io.Writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(value)
}
