package nativecomponent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OperationFundsTransactionSourceRowPageV1 = "funds.transaction_source_row_page_v1"

	TransactionSourceRowRelationV1                = "fc_transaction_raw"
	TransactionSourceRowParserIDV1                = "analytix.funds.transaction-row-parser"
	TransactionSourceRowParserVersionV1           = "analytix.funds.transaction-row-parser/v1"
	TransactionSourceRowLocatorOrderingV1         = "source_file_id_digest_asc+source_row_number_asc/v1"
	TransactionSourceRowProofStatusV1             = "duckdb_projection_requires_host_raw_replay"
	TransactionSourceRowHostReplayProfileV1       = "canonical_direct_csv_v1"
	TransactionSourceRowMaximumRowsV1             = uint16(100)
	TransactionSourceRowMaximumRequestBytesV1     = 4 * 1024
	TransactionSourceRowMaximumResultBytesV1      = 1024 * 1024
	TransactionSourceRowMaximumSnapshotBytesV1    = uint64(8 * 1024 * 1024 * 1024)
	TransactionSourceRowMaximumDurationV1         = 30 * time.Second
	transactionSourceRowMaximumInventoryBytesV1   = 512 * 1024
	transactionSourceRowMaximumInventoryEntriesV1 = 4096
	transactionSourceRowMaximumPrivateFileIDV1    = 4 * 1024
	transactionSourceRowMaximumCellBytesV1        = 16 * 1024
	transactionSourceRowMaximumJSONIntegerV1      = uint64(9_007_199_254_740_991)
)

var (
	transactionSourceRowSourceFileDigestDomainV1 = []byte("analytix.source-row-source-file-id/digest/v1\x00")
	transactionSourceRowCanonicalRowDomainV1     = []byte("analytix.parsed-canonical-row/digest/v1\x00")
	transactionSourceRowInventoryDomainV1        = []byte("analytix.funds-source-inventory/v1\x00")
	transactionSourceRowSnapshotDomainV1         = []byte("analytix.funds-source-snapshot/v1\x00")
	transactionSourceRowPageDomainV1             = []byte("analytix.funds-transaction-source-page/v1\x00")
)

type TransactionSourceRowCursorV1 struct {
	SourceFileIDDigest string `json:"sourceFileIdDigest"`
	SourceRowNumber    uint64 `json:"sourceRowNumber"`
}

type TransactionSourceRowPageArgumentsInputV1 struct {
	Binding                        domainsecurity.DatasetSnapshotBindingKeyV1
	ParsedGenerationIdentitySHA256 string
	Materialization                domainsecurity.FundsMaterializationResultV1
	InstalledSnapshot              domainfundsquerysource.ImmutableSnapshotObjectV1
	MaxRows                        uint16
	Cursor                         *TransactionSourceRowCursorV1
}

type transactionSourceRowExpectedSourceV1 struct {
	privateImportFileID string
	sha256              string
	rowCount            uint64
}

// TransactionSourceRowPageArgumentsV1 is assembled before DSV2 publication
// from the authoritative case binding, prepared parsed-generation identity,
// validated materializer result, and installed immutable DuckDB object. It is
// inert staging input, not query or publication authority. Only its closed,
// pathless Rust projection can be serialized.
type TransactionSourceRowPageArgumentsV1 struct {
	binding                        domainsecurity.DatasetSnapshotBindingKeyV1
	parsedGenerationIdentitySHA256 string
	relation                       string
	maxRows                        uint16
	cursor                         *TransactionSourceRowCursorV1
	installedSnapshot              domainfundsquerysource.ImmutableSnapshotObjectV1
	producerContentContract        string
	producerContentID              string
	producerManifestSHA256         string
	rawArtifactManifestSHA256      string
	duckDBContentSnapshotDigest    string
	duckDBSnapshotManifestSHA256   string
	materializationIdentity        string
	analyticalSchemaDigest         string
	sourceRevision                 uint64
	sourceRowCount                 uint64
	acceptedRowCount               uint64
	rejectedRowCount               uint64
	duplicateRowCount              uint64
	expectedSources                []transactionSourceRowExpectedSourceV1
}

type transactionSourceRowArgumentsWireV1 struct {
	CaseID                         string                        `json:"caseId"`
	BindingKeyDigest               string                        `json:"bindingKeyDigest"`
	ParsedGenerationIdentitySHA256 string                        `json:"parsedGenerationIdentitySha256"`
	Relation                       string                        `json:"relation"`
	MaxRows                        uint16                        `json:"maxRows"`
	Cursor                         *TransactionSourceRowCursorV1 `json:"cursor"`
}

func NewTransactionSourceRowPageArgumentsV1(
	input TransactionSourceRowPageArgumentsInputV1,
) (TransactionSourceRowPageArgumentsV1, error) {
	materialization, err := revalidateTransactionSourceRowMaterializationV1(input.Materialization)
	if err != nil {
		return TransactionSourceRowPageArgumentsV1{}, ErrRequestInvalid
	}
	binding := input.Binding
	installedSnapshot := input.InstalledSnapshot
	policy, policyOK := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsCanonicalTransactionSourceRowPolicyIDV1,
	)
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(installedSnapshot) != nil ||
		installedSnapshot.DuckDBByteLength > TransactionSourceRowMaximumSnapshotBytesV1 ||
		!policyOK || domainevidence.ValidateSourceRowProducerPolicyV1(policy) != nil ||
		policy.Operation != OperationFundsTransactionSourceRowPageV1 ||
		binding.CaseID != materialization.CaseID || binding.CaseID != installedSnapshot.CaseID ||
		!canonicalTransactionSourceRowDigestV1(input.ParsedGenerationIdentitySHA256) ||
		input.MaxRows == 0 || input.MaxRows > TransactionSourceRowMaximumRowsV1 ||
		validateTransactionSourceRowCursorV1(input.Cursor) != nil {
		return TransactionSourceRowPageArgumentsV1{}, ErrRequestInvalid
	}

	expectedSources := make([]transactionSourceRowExpectedSourceV1, len(materialization.RawSourceManifest))
	for index, source := range materialization.RawSourceManifest {
		expectedSources[index] = transactionSourceRowExpectedSourceV1{
			privateImportFileID: source.FileID,
			sha256:              source.SHA256,
			rowCount:            source.RowsImportedNorm,
		}
	}
	arguments := TransactionSourceRowPageArgumentsV1{
		binding:                        binding,
		parsedGenerationIdentitySHA256: input.ParsedGenerationIdentitySHA256,
		relation:                       TransactionSourceRowRelationV1,
		maxRows:                        input.MaxRows,
		cursor:                         cloneTransactionSourceRowCursorV1(input.Cursor),
		installedSnapshot:              installedSnapshot,
		producerContentContract:        materialization.ProducerContentManifest.Contract,
		producerContentID:              materialization.ProducerContentID,
		producerManifestSHA256:         materialization.ProducerContentManifestSHA256,
		rawArtifactManifestSHA256:      materialization.RawArtifactManifestSHA256,
		duckDBContentSnapshotDigest:    materialization.DuckDBContentSnapshotDigest,
		duckDBSnapshotManifestSHA256:   materialization.DuckDBSnapshotManifestSHA256,
		materializationIdentity:        materialization.MaterializationIdentity,
		analyticalSchemaDigest:         materialization.SchemaDigest,
		sourceRevision:                 materialization.ProducerContentManifest.SourceRevision,
		sourceRowCount:                 materialization.ProducerContentManifest.NormalizedRowCount,
		acceptedRowCount:               materialization.ProducerContentManifest.AcceptedRowCount,
		rejectedRowCount:               materialization.ProducerContentManifest.RejectedRowCount,
		duplicateRowCount:              materialization.ProducerContentManifest.DuplicateRowCount,
		expectedSources:                expectedSources,
	}
	if validateTransactionSourceRowArgumentsV1(arguments) != nil {
		return TransactionSourceRowPageArgumentsV1{}, ErrRequestInvalid
	}
	return arguments, nil
}

func (TransactionSourceRowPageArgumentsV1) MarshalJSON() ([]byte, error) {
	return nil, ErrRequestInvalid
}

func (*TransactionSourceRowPageArgumentsV1) UnmarshalJSON([]byte) error {
	return ErrRequestInvalid
}

func (arguments TransactionSourceRowPageArgumentsV1) String() string {
	return fmt.Sprintf(
		"TransactionSourceRowPageArgumentsV1{maxRows:%d,cursorPresent:%t,privateBindings:[REDACTED]}",
		arguments.maxRows,
		arguments.cursor != nil,
	)
}

func (arguments TransactionSourceRowPageArgumentsV1) GoString() string {
	return arguments.String()
}

func ValidateTransactionSourceRowPageArgumentsAuthorityV1(
	arguments TransactionSourceRowPageArgumentsV1,
	installedSnapshot domainfundsquerysource.ImmutableSnapshotObjectV1,
) error {
	if validateTransactionSourceRowArgumentsV1(arguments) != nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(installedSnapshot) != nil ||
		arguments.installedSnapshot != installedSnapshot {
		return ErrContextMismatch
	}
	return nil
}

func validateTransactionSourceRowArgumentsV1(arguments TransactionSourceRowPageArgumentsV1) error {
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(arguments.binding) != nil ||
		domainfundsquerysource.ValidateImmutableSnapshotObjectV1(arguments.installedSnapshot) != nil ||
		arguments.installedSnapshot.DuckDBByteLength > TransactionSourceRowMaximumSnapshotBytesV1 ||
		arguments.binding.CaseID != arguments.installedSnapshot.CaseID ||
		!canonicalTransactionSourceRowDigestV1(arguments.parsedGenerationIdentitySHA256) ||
		arguments.relation != TransactionSourceRowRelationV1 ||
		arguments.maxRows == 0 || arguments.maxRows > TransactionSourceRowMaximumRowsV1 ||
		validateTransactionSourceRowCursorV1(arguments.cursor) != nil ||
		arguments.producerContentContract != domainsecurity.FundsProducerContentManifestContractV1 ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(arguments.producerContentID) ||
		!canonicalTransactionSourceRowDigestV1(arguments.producerManifestSHA256) ||
		!canonicalTransactionSourceRowDigestV1(arguments.rawArtifactManifestSHA256) ||
		!canonicalTransactionSourceRowDigestV1(arguments.duckDBContentSnapshotDigest) ||
		!canonicalTransactionSourceRowDigestV1(arguments.duckDBSnapshotManifestSHA256) ||
		!strings.HasPrefix(arguments.materializationIdentity, domainsecurity.FundsMaterializationIdentityPrefixV1) ||
		!canonicalTransactionSourceRowDigestV1(strings.TrimPrefix(arguments.materializationIdentity, domainsecurity.FundsMaterializationIdentityPrefixV1)) ||
		arguments.analyticalSchemaDigest != domainsecurity.FixedFundsAnalyticalSchemaDigestV1() ||
		arguments.sourceRevision == 0 || arguments.sourceRevision > transactionSourceRowMaximumJSONIntegerV1 ||
		arguments.sourceRowCount == 0 || arguments.sourceRowCount > transactionSourceRowMaximumJSONIntegerV1 ||
		arguments.acceptedRowCount != arguments.sourceRowCount || arguments.rejectedRowCount != 0 || arguments.duplicateRowCount != 0 ||
		len(arguments.expectedSources) == 0 || len(arguments.expectedSources) > transactionSourceRowMaximumInventoryEntriesV1 {
		return ErrRequestInvalid
	}
	var total uint64
	previous := ""
	for _, source := range arguments.expectedSources {
		if !canonicalTransactionSourceRowPrivateTextV1(source.privateImportFileID, transactionSourceRowMaximumPrivateFileIDV1) ||
			source.privateImportFileID <= previous ||
			!canonicalTransactionSourceRowDigestV1(source.sha256) || source.rowCount == 0 ||
			source.rowCount > transactionSourceRowMaximumJSONIntegerV1 || total > transactionSourceRowMaximumJSONIntegerV1-source.rowCount {
			return ErrRequestInvalid
		}
		total += source.rowCount
		previous = source.privateImportFileID
	}
	if total != arguments.sourceRowCount {
		return ErrRequestInvalid
	}
	return nil
}

func transactionSourceRowArgumentsWireFromV1(
	arguments TransactionSourceRowPageArgumentsV1,
) (transactionSourceRowArgumentsWireV1, error) {
	if validateTransactionSourceRowArgumentsV1(arguments) != nil {
		return transactionSourceRowArgumentsWireV1{}, ErrRequestInvalid
	}
	return transactionSourceRowArgumentsWireV1{
		CaseID:                         arguments.binding.CaseID,
		BindingKeyDigest:               arguments.binding.BindingKeyDigest,
		ParsedGenerationIdentitySHA256: arguments.parsedGenerationIdentitySHA256,
		Relation:                       arguments.relation,
		MaxRows:                        arguments.maxRows,
		Cursor:                         cloneTransactionSourceRowCursorV1(arguments.cursor),
	}, nil
}

type transactionSourceRowNativeRequestFrameWireV1 struct {
	RequestID string                              `json:"request_id"`
	Command   string                              `json:"command"`
	CaseID    string                              `json:"case_id"`
	Payload   transactionSourceRowArgumentsWireV1 `json:"payload"`
}

func EncodeTransactionSourceRowPageNativeRequestFrameV1(
	requestID string,
	arguments TransactionSourceRowPageArgumentsV1,
) ([]byte, error) {
	if !canonicalTransactionSourceRowDigestV1(requestID) {
		return nil, ErrRequestInvalid
	}
	payload, err := transactionSourceRowArgumentsWireFromV1(arguments)
	if err != nil {
		return nil, ErrRequestInvalid
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(transactionSourceRowNativeRequestFrameWireV1{
		RequestID: requestID,
		Command:   OperationFundsTransactionSourceRowPageV1,
		CaseID:    arguments.binding.CaseID,
		Payload:   payload,
	}) != nil || output.Len() == 0 || output.Len() > TransactionSourceRowMaximumRequestBytesV1 {
		return nil, ErrRequestInvalid
	}
	frame := output.Bytes()
	if frame[len(frame)-1] != '\n' || bytes.IndexByte(frame[:len(frame)-1], '\n') >= 0 ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		return nil, ErrRequestInvalid
	}
	return append([]byte(nil), frame...), nil
}

type transactionSourceRowSnapshotWireV1 struct {
	SourceRevision       uint64 `json:"sourceRevision"`
	SourceRowCount       uint64 `json:"sourceRowCount"`
	SourceMaxTxnTS       string `json:"sourceMaxTxnTs"`
	SourceMaxID          uint64 `json:"sourceMaxId"`
	AcceptedRowCount     uint64 `json:"acceptedRowCount"`
	RejectedRowCount     uint64 `json:"rejectedRowCount"`
	DuplicateRowCount    uint64 `json:"duplicateRowCount"`
	InventoryDigest      string `json:"inventoryDigest"`
	SourceSnapshotDigest string `json:"sourceSnapshotDigest"`
}

type transactionSourceRowInventoryWireV1 struct {
	SourceFileID         string `json:"sourceFileId"`
	SourceFileIDDigest   string `json:"sourceFileIdDigest"`
	PrivateImportFileID  string `json:"privateImportFileId"`
	SourceArtifactSHA256 string `json:"sourceArtifactSha256"`
	FileType             string `json:"fileType"`
	RowCount             uint64 `json:"rowCount"`
	AcceptedRowCount     uint64 `json:"acceptedRowCount"`
	RejectedRowCount     uint64 `json:"rejectedRowCount"`
}

type transactionSourceRowScalarWireV1 struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type transactionSourceRowTypedFieldWireV1 struct {
	Name   string                           `json:"name"`
	Scalar transactionSourceRowScalarWireV1 `json:"scalar"`
}

type transactionSourceRowWireV1 struct {
	SourceFileID         string                                 `json:"sourceFileId"`
	SourceFileIDDigest   string                                 `json:"sourceFileIdDigest"`
	SourceRowNumber      uint64                                 `json:"sourceRowNumber"`
	SourceArtifactSHA256 string                                 `json:"sourceArtifactSha256"`
	CanonicalRowSHA256   string                                 `json:"canonicalRowSha256"`
	CanonicalTypedRow    []transactionSourceRowTypedFieldWireV1 `json:"canonicalTypedRow"`
	Disposition          string                                 `json:"disposition"`
}

type transactionSourceRowPageWireV1 struct {
	SchemaVersion                  uint8                                 `json:"schemaVersion"`
	Operation                      string                                `json:"operation"`
	CaseID                         string                                `json:"caseId"`
	BindingKeyDigest               string                                `json:"bindingKeyDigest"`
	ParsedGenerationIdentitySHA256 string                                `json:"parsedGenerationIdentitySha256"`
	Relation                       string                                `json:"relation"`
	MaterializationIdentity        string                                `json:"materializationIdentity"`
	ProducerContentContract        string                                `json:"producerContentContract"`
	ProducerContentID              string                                `json:"producerContentId"`
	ProducerContentManifestSHA256  string                                `json:"producerContentManifestSha256"`
	RawArtifactManifestSHA256      string                                `json:"rawArtifactManifestSha256"`
	DuckDBContentSnapshotDigest    string                                `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256   string                                `json:"duckdbSnapshotManifestSha256"`
	AnalyticalSchemaDigest         string                                `json:"analyticalSchemaDigest"`
	ParserID                       string                                `json:"parserId"`
	ParserVersion                  string                                `json:"parserVersion"`
	LocatorOrdering                string                                `json:"locatorOrdering"`
	SourceProofStatus              string                                `json:"sourceProofStatus"`
	RequiredHostRawReplayProfile   string                                `json:"requiredHostRawReplayProfile"`
	HostRawReplayRequired          bool                                  `json:"hostRawReplayRequired"`
	SourceSnapshot                 transactionSourceRowSnapshotWireV1    `json:"sourceSnapshot"`
	Inventory                      []transactionSourceRowInventoryWireV1 `json:"inventory"`
	Rows                           []transactionSourceRowWireV1          `json:"rows"`
	NextCursor                     *TransactionSourceRowCursorV1         `json:"nextCursor"`
	Complete                       bool                                  `json:"complete"`
	PageDigest                     string                                `json:"pageDigest"`
}

type TransactionSourceRowPageHostV1 struct {
	SchemaVersion                  uint8
	Operation                      string
	CaseID                         string
	BindingKeyDigest               string
	ParsedGenerationIdentitySHA256 string
	Relation                       string
	MaterializationIdentity        string
	ProducerContentContract        string
	ProducerContentID              string
	ProducerContentManifestSHA256  string
	RawArtifactManifestSHA256      string
	DuckDBContentSnapshotDigest    string
	DuckDBSnapshotManifestSHA256   string
	AnalyticalSchemaDigest         string
	ParserID                       string
	ParserVersion                  string
	LocatorOrdering                string
	SourceProofStatus              string
	RequiredHostRawReplayProfile   string
	HostRawReplayRequired          bool
	SourceSnapshot                 TransactionSourceRowSnapshotHostV1
	Inventory                      []TransactionSourceRowInventoryHostV1
	Rows                           []TransactionSourceRowHostV1
	NextCursor                     *TransactionSourceRowCursorV1
	Complete                       bool
	PageDigest                     string
}

type TransactionSourceRowSnapshotHostV1 struct {
	SourceRevision       uint64
	SourceRowCount       uint64
	SourceMaxTxnTS       string
	SourceMaxID          uint64
	AcceptedRowCount     uint64
	RejectedRowCount     uint64
	DuplicateRowCount    uint64
	InventoryDigest      string
	SourceSnapshotDigest string
}

type TransactionSourceRowInventoryHostV1 struct {
	SourceFileID         string
	SourceFileIDDigest   string
	PrivateImportFileID  string
	SourceArtifactSHA256 string
	FileType             string
	RowCount             uint64
	AcceptedRowCount     uint64
	RejectedRowCount     uint64
}

type TransactionSourceRowScalarHostV1 struct {
	Kind  string
	Value string
}

type TransactionSourceRowTypedFieldHostV1 struct {
	Name   string
	Scalar TransactionSourceRowScalarHostV1
}

type TransactionSourceRowHostV1 struct {
	SourceFileID         string
	SourceFileIDDigest   string
	SourceRowNumber      uint64
	SourceArtifactSHA256 string
	CanonicalRowSHA256   string
	CanonicalTypedRow    []TransactionSourceRowTypedFieldHostV1
	Disposition          string
}

// TransactionSourceRowPageV1 is deliberately non-serializable. Its host view
// includes complete typed source values and may be used only synchronously by
// the trusted staging composer. PrivateImportFileID is an FPC1-bound lookup
// key, never a path: raw replay must resolve it through separate authoritative
// fileID+SHA256 admission to a callback-scoped exact FD lease.
type TransactionSourceRowPageV1 struct {
	private TransactionSourceRowPageHostV1
}

func (TransactionSourceRowPageV1) MarshalJSON() ([]byte, error) { return nil, ErrResultInvalid }
func (*TransactionSourceRowPageV1) UnmarshalJSON([]byte) error  { return ErrResultInvalid }

func (page TransactionSourceRowPageV1) String() string {
	return fmt.Sprintf(
		"TransactionSourceRowPageV1{rows:%d,inventory:%d,complete:%t,hostPrivate:[REDACTED]}",
		len(page.private.Rows), len(page.private.Inventory), page.private.Complete,
	)
}

func (page TransactionSourceRowPageV1) GoString() string { return page.String() }

func (TransactionSourceRowPageHostV1) MarshalJSON() ([]byte, error) { return nil, ErrResultInvalid }
func (TransactionSourceRowInventoryHostV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}
func (TransactionSourceRowHostV1) MarshalJSON() ([]byte, error) { return nil, ErrResultInvalid }
func (TransactionSourceRowTypedFieldHostV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}
func (TransactionSourceRowScalarHostV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}

func (view TransactionSourceRowPageHostV1) String() string {
	return fmt.Sprintf(
		"TransactionSourceRowPageHostV1{rows:%d,inventory:%d,complete:%t,privateValues:[REDACTED]}",
		len(view.Rows), len(view.Inventory), view.Complete,
	)
}

func (view TransactionSourceRowPageHostV1) GoString() string { return view.String() }

func (entry TransactionSourceRowInventoryHostV1) String() string {
	return fmt.Sprintf(
		"TransactionSourceRowInventoryHostV1{rowCount:%d,hostPrivate:[REDACTED]}",
		entry.RowCount,
	)
}

func (entry TransactionSourceRowInventoryHostV1) GoString() string { return entry.String() }

func (row TransactionSourceRowHostV1) String() string {
	return fmt.Sprintf(
		"TransactionSourceRowHostV1{typedFields:%d,hostPrivate:[REDACTED]}",
		len(row.CanonicalTypedRow),
	)
}

func (row TransactionSourceRowHostV1) GoString() string { return row.String() }

func (field TransactionSourceRowTypedFieldHostV1) String() string {
	return "TransactionSourceRowTypedFieldHostV1{hostPrivate:[REDACTED]}"
}

func (field TransactionSourceRowTypedFieldHostV1) GoString() string { return field.String() }

func (scalar TransactionSourceRowScalarHostV1) String() string {
	return "TransactionSourceRowScalarHostV1{hostPrivate:[REDACTED]}"
}

func (scalar TransactionSourceRowScalarHostV1) GoString() string { return scalar.String() }

func (page TransactionSourceRowPageV1) UseExact(
	consume func(TransactionSourceRowPageHostV1) error,
) error {
	if consume == nil || page.private.SchemaVersion != 1 {
		return ErrResultInvalid
	}
	if err := consume(cloneTransactionSourceRowHostViewV1(page.private)); err != nil {
		return ErrResultInvalid
	}
	return nil
}

// UseFundsCanonicalCSVAdmissionRowsV1 is the fixed bridge from the validated
// native page contract into the opaque evidence admission row carrier. It
// prevents app and inbound layers from importing parsed evidence field types.
func (page TransactionSourceRowPageV1) UseFundsCanonicalCSVAdmissionRowsV1(
	consume func(
		[]domainevidence.FundsCanonicalCSVHostRowV1,
		*TransactionSourceRowCursorV1,
		bool,
	) error,
) error {
	if consume == nil || page.private.SchemaVersion != 1 {
		return ErrResultInvalid
	}
	rows := make([]domainevidence.FundsCanonicalCSVHostRowV1, 0, len(page.private.Rows))
	for _, nativeRow := range page.private.Rows {
		fields := make([]domainevidence.FundsCanonicalCSVHostTypedFieldV1, len(nativeRow.CanonicalTypedRow))
		for index, field := range nativeRow.CanonicalTypedRow {
			fields[index] = domainevidence.FundsCanonicalCSVHostTypedFieldV1{
				Name:  field.Name,
				Kind:  field.Scalar.Kind,
				Value: field.Scalar.Value,
			}
		}
		row, err := domainevidence.NewFundsCanonicalCSVHostRowV1(
			nativeRow.SourceFileID,
			nativeRow.SourceRowNumber,
			nativeRow.SourceArtifactSHA256,
			nativeRow.CanonicalRowSHA256,
			fields,
		)
		for index := range fields {
			fields[index].Name = ""
			fields[index].Kind = ""
			fields[index].Value = ""
		}
		if err != nil {
			return ErrResultInvalid
		}
		rows = append(rows, row)
	}
	var next *TransactionSourceRowCursorV1
	if page.private.NextCursor != nil {
		copy := *page.private.NextCursor
		next = &copy
	}
	if err := consume(rows, next, page.private.Complete); err != nil {
		return ErrResultInvalid
	}
	return nil
}

func (page TransactionSourceRowPageV1) RowCountV1() int      { return len(page.private.Rows) }
func (page TransactionSourceRowPageV1) CompleteV1() bool     { return page.private.Complete }
func (page TransactionSourceRowPageV1) PageDigestV1() string { return page.private.PageDigest }

func ParseTransactionSourceRowPageResultV1(
	raw []byte,
	arguments TransactionSourceRowPageArgumentsV1,
) (TransactionSourceRowPageV1, error) {
	if len(raw) == 0 || len(raw) > TransactionSourceRowMaximumResultBytesV1 ||
		validateTransactionSourceRowArgumentsV1(arguments) != nil ||
		domainjsonstrict.Validate(raw, domainjsonstrict.Options{
			RequireObject:  true,
			MaxBytes:       TransactionSourceRowMaximumResultBytesV1,
			MaxDepth:       16,
			MaxTokens:      160_000,
			MaxStringBytes: transactionSourceRowMaximumCellBytesV1,
			MaxNumberBytes: 32,
			MaxAbsExponent: 1,
		}) != nil || validateTransactionSourceRowOrderedShapeV1(raw) != nil {
		return TransactionSourceRowPageV1{}, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire transactionSourceRowPageWireV1
	if decoder.Decode(&wire) != nil {
		return TransactionSourceRowPageV1{}, ErrResultInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) ||
		validateTransactionSourceRowPageWireV1(raw, wire, arguments) != nil {
		return TransactionSourceRowPageV1{}, ErrResultInvalid
	}
	return TransactionSourceRowPageV1{private: transactionSourceRowHostViewFromWireV1(wire)}, nil
}

func validateTransactionSourceRowPageWireV1(
	raw []byte,
	wire transactionSourceRowPageWireV1,
	arguments TransactionSourceRowPageArgumentsV1,
) error {
	if wire.SchemaVersion != 1 || wire.Operation != OperationFundsTransactionSourceRowPageV1 ||
		wire.CaseID != arguments.binding.CaseID || wire.BindingKeyDigest != arguments.binding.BindingKeyDigest ||
		wire.ParsedGenerationIdentitySHA256 != arguments.parsedGenerationIdentitySHA256 || wire.Relation != arguments.relation ||
		wire.MaterializationIdentity != arguments.materializationIdentity ||
		wire.ProducerContentContract != arguments.producerContentContract ||
		wire.ProducerContentID != arguments.producerContentID ||
		wire.ProducerContentManifestSHA256 != arguments.producerManifestSHA256 ||
		wire.RawArtifactManifestSHA256 != arguments.rawArtifactManifestSHA256 ||
		wire.DuckDBContentSnapshotDigest != arguments.duckDBContentSnapshotDigest ||
		wire.DuckDBSnapshotManifestSHA256 != arguments.duckDBSnapshotManifestSHA256 ||
		wire.AnalyticalSchemaDigest != arguments.analyticalSchemaDigest ||
		wire.ParserID != TransactionSourceRowParserIDV1 ||
		wire.ParserVersion != TransactionSourceRowParserVersionV1 ||
		wire.LocatorOrdering != TransactionSourceRowLocatorOrderingV1 ||
		wire.SourceProofStatus != TransactionSourceRowProofStatusV1 ||
		wire.RequiredHostRawReplayProfile != TransactionSourceRowHostReplayProfileV1 ||
		!wire.HostRawReplayRequired || len(wire.Inventory) == 0 ||
		len(wire.Inventory) > transactionSourceRowMaximumInventoryEntriesV1 ||
		len(wire.Rows) == 0 || len(wire.Rows) > int(arguments.maxRows) ||
		wire.Complete != (wire.NextCursor == nil) ||
		!canonicalTransactionSourceRowDigestV1(wire.PageDigest) {
		return ErrResultInvalid
	}

	if validateTransactionSourceRowInventoryV1(wire.Inventory, arguments) != nil ||
		validateTransactionSourceRowSnapshotV1(wire.SourceSnapshot, wire.Inventory, arguments) != nil ||
		validateTransactionSourceRowsV1(wire.Rows, wire.Inventory, arguments.cursor) != nil {
		return ErrResultInvalid
	}
	if wire.NextCursor != nil {
		last := wire.Rows[len(wire.Rows)-1]
		if validateTransactionSourceRowCursorV1(wire.NextCursor) != nil ||
			wire.NextCursor.SourceFileIDDigest != last.SourceFileIDDigest ||
			wire.NextCursor.SourceRowNumber != last.SourceRowNumber {
			return ErrResultInvalid
		}
	}
	wantPageDigest, err := transactionSourceRowPageDigestV1(raw)
	if err != nil || wire.PageDigest != wantPageDigest {
		return ErrResultInvalid
	}
	return nil
}

func validateTransactionSourceRowInventoryV1(
	inventory []transactionSourceRowInventoryWireV1,
	arguments TransactionSourceRowPageArgumentsV1,
) error {
	body, err := json.Marshal(inventory)
	if err != nil || len(body) == 0 || len(body) > transactionSourceRowMaximumInventoryBytesV1 {
		return ErrResultInvalid
	}
	expected := make(map[string]transactionSourceRowExpectedSourceV1, len(arguments.expectedSources))
	for _, source := range arguments.expectedSources {
		expected[source.privateImportFileID] = source
	}
	seen := make(map[string]struct{}, len(inventory))
	previousDigest := ""
	for _, entry := range inventory {
		source, ok := expected[entry.PrivateImportFileID]
		wantSourceFileID := deriveTransactionSourceRowSourceFileIDV1(arguments.binding.CaseID, entry.PrivateImportFileID)
		if !ok || entry.SourceFileID != wantSourceFileID ||
			entry.SourceFileIDDigest != deriveTransactionSourceRowSourceFileDigestV1(wantSourceFileID) ||
			entry.SourceFileIDDigest <= previousDigest ||
			!canonicalTransactionSourceRowPrivateTextV1(entry.PrivateImportFileID, transactionSourceRowMaximumPrivateFileIDV1) ||
			entry.SourceArtifactSHA256 != source.sha256 ||
			entry.FileType != "CSV" || entry.RowCount != source.rowCount ||
			entry.AcceptedRowCount != source.rowCount || entry.RejectedRowCount != 0 {
			return ErrResultInvalid
		}
		if _, duplicate := seen[entry.PrivateImportFileID]; duplicate {
			return ErrResultInvalid
		}
		seen[entry.PrivateImportFileID] = struct{}{}
		previousDigest = entry.SourceFileIDDigest
	}
	if len(seen) != len(expected) {
		return ErrResultInvalid
	}
	return nil
}

func validateTransactionSourceRowSnapshotV1(
	snapshot transactionSourceRowSnapshotWireV1,
	inventory []transactionSourceRowInventoryWireV1,
	arguments TransactionSourceRowPageArgumentsV1,
) error {
	wantInventoryDigest, err := transactionSourceRowDomainDigestV1(transactionSourceRowInventoryDomainV1, inventory)
	if err != nil || snapshot.SourceRevision != arguments.sourceRevision ||
		snapshot.SourceRowCount != arguments.sourceRowCount || snapshot.SourceMaxID == 0 ||
		snapshot.SourceMaxID > transactionSourceRowMaximumJSONIntegerV1 ||
		!canonicalTransactionSourceRowOptionalTimestampV1(snapshot.SourceMaxTxnTS) ||
		snapshot.AcceptedRowCount != arguments.acceptedRowCount ||
		snapshot.RejectedRowCount != arguments.rejectedRowCount ||
		snapshot.DuplicateRowCount != arguments.duplicateRowCount ||
		snapshot.InventoryDigest != wantInventoryDigest ||
		!canonicalTransactionSourceRowDigestV1(snapshot.SourceSnapshotDigest) {
		return ErrResultInvalid
	}
	withoutDigest := snapshot
	withoutDigest.SourceSnapshotDigest = ""
	wantSnapshotDigest, err := transactionSourceRowDomainDigestV1(transactionSourceRowSnapshotDomainV1, withoutDigest)
	if err != nil || snapshot.SourceSnapshotDigest != wantSnapshotDigest {
		return ErrResultInvalid
	}
	return nil
}

func validateTransactionSourceRowsV1(
	rows []transactionSourceRowWireV1,
	inventory []transactionSourceRowInventoryWireV1,
	cursor *TransactionSourceRowCursorV1,
) error {
	byDigest := make(map[string]transactionSourceRowInventoryWireV1, len(inventory))
	for _, entry := range inventory {
		byDigest[entry.SourceFileIDDigest] = entry
	}
	previousDigest := ""
	previousRow := uint64(0)
	if cursor != nil {
		previousDigest = cursor.SourceFileIDDigest
		previousRow = cursor.SourceRowNumber
	}
	for _, row := range rows {
		entry, ok := byDigest[row.SourceFileIDDigest]
		if !ok || row.SourceFileID != entry.SourceFileID ||
			row.SourceArtifactSHA256 != entry.SourceArtifactSHA256 ||
			row.SourceRowNumber == 0 || row.SourceRowNumber > transactionSourceRowMaximumJSONIntegerV1 ||
			(row.SourceFileIDDigest < previousDigest ||
				row.SourceFileIDDigest == previousDigest && row.SourceRowNumber <= previousRow) ||
			row.Disposition != "accepted" ||
			!canonicalTransactionSourceRowDigestV1(row.CanonicalRowSHA256) ||
			validateTransactionSourceRowTypedFieldsV1(row.CanonicalTypedRow) != nil {
			return ErrResultInvalid
		}
		wantRowDigest, err := transactionSourceRowDomainDigestV1(transactionSourceRowCanonicalRowDomainV1, row.CanonicalTypedRow)
		if err != nil || row.CanonicalRowSHA256 != wantRowDigest {
			return ErrResultInvalid
		}
		previousDigest = row.SourceFileIDDigest
		previousRow = row.SourceRowNumber
	}
	return nil
}

var transactionSourceRowTypedFieldKindsV1 = map[string]string{
	"norm.clean_acct_no": "text", "norm.clean_amount": "decimal", "norm.clean_balance": "decimal",
	"norm.clean_card_no": "text", "norm.clean_dc_flag": "text", "norm.clean_failed": "integer",
	"norm.clean_invalid": "integer", "norm.clean_reversal": "integer", "norm.txn_ts": "text",
	"raw.account_open_name": "text", "raw.acct_no": "text", "raw.amount": "text", "raw.balance": "text",
	"raw.branch_code": "text", "raw.branch_name": "text", "raw.card_no": "text", "raw.cash_flag": "text",
	"raw.counterparty_acct": "text", "raw.counterparty_balance": "text", "raw.counterparty_bank": "text",
	"raw.counterparty_id_no": "text", "raw.counterparty_name": "text", "raw.currency": "text",
	"raw.dc_flag": "text", "raw.ip_addr": "text", "raw.is_success": "text", "raw.location": "text",
	"raw.log_id": "text", "raw.mac_addr": "text", "raw.merchant_name": "text", "raw.merchant_no": "text",
	"raw.opener_id_no": "text", "raw.query_feedback_reason": "text", "raw.remark": "text", "raw.summary": "text",
	"raw.teller_no": "text", "raw.terminal_no": "text", "raw.txn_id": "text", "raw.txn_time": "text",
	"raw.txn_type": "text", "raw.voucher_id": "text", "raw.voucher_no": "text", "raw.voucher_type": "text",
}

func validateTransactionSourceRowTypedFieldsV1(fields []transactionSourceRowTypedFieldWireV1) error {
	if len(fields) != len(transactionSourceRowTypedFieldKindsV1) {
		return ErrResultInvalid
	}
	previous := ""
	for _, field := range fields {
		expectedKind, ok := transactionSourceRowTypedFieldKindsV1[field.Name]
		if !ok || field.Name <= previous {
			return ErrResultInvalid
		}
		scalar := field.Scalar
		switch scalar.Kind {
		case "null":
			if scalar.Value != "" || expectedKind == "integer" {
				return ErrResultInvalid
			}
		case expectedKind:
			switch expectedKind {
			case "text":
				if !utf8.ValidString(scalar.Value) || len(scalar.Value) > transactionSourceRowMaximumCellBytesV1 || strings.ContainsRune(scalar.Value, 0) {
					return ErrResultInvalid
				}
			case "decimal":
				if canonicalTransactionSourceRowDecimalV1(scalar.Value) != scalar.Value {
					return ErrResultInvalid
				}
			case "integer":
				if scalar.Value != "0" && scalar.Value != "1" {
					return ErrResultInvalid
				}
			}
		default:
			return ErrResultInvalid
		}
		previous = field.Name
	}
	return nil
}

func transactionSourceRowPageDigestV1(raw []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if decoder.Decode(&value) != nil {
		return "", ErrResultInvalid
	}
	if _, found := value["pageDigest"]; !found {
		return "", ErrResultInvalid
	}
	delete(value, "pageDigest")
	return transactionSourceRowDomainDigestV1(transactionSourceRowPageDomainV1, value)
}

func transactionSourceRowDomainDigestV1(domain []byte, value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write(domain)
	_, _ = hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func deriveTransactionSourceRowSourceFileIDV1(caseID, privateFileID string) string {
	value, _ := domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(caseID, privateFileID)
	return value
}

func deriveTransactionSourceRowSourceFileDigestV1(sourceFileID string) string {
	hash := sha256.New()
	_, _ = hash.Write(transactionSourceRowSourceFileDigestDomainV1)
	_, _ = hash.Write([]byte(sourceFileID))
	return hex.EncodeToString(hash.Sum(nil))
}

func revalidateTransactionSourceRowMaterializationV1(
	materialization domainsecurity.FundsMaterializationResultV1,
) (domainsecurity.FundsMaterializationResultV1, error) {
	body, err := json.Marshal(materialization)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, err
	}
	return domainsecurity.ParseFundsMaterializationResultV1(body)
}

func validateTransactionSourceRowCursorV1(cursor *TransactionSourceRowCursorV1) error {
	if cursor == nil {
		return nil
	}
	if !canonicalTransactionSourceRowDigestV1(cursor.SourceFileIDDigest) ||
		cursor.SourceRowNumber == 0 || cursor.SourceRowNumber > transactionSourceRowMaximumJSONIntegerV1 {
		return ErrRequestInvalid
	}
	return nil
}

func cloneTransactionSourceRowCursorV1(cursor *TransactionSourceRowCursorV1) *TransactionSourceRowCursorV1 {
	if cursor == nil {
		return nil
	}
	clone := *cursor
	return &clone
}

func canonicalTransactionSourceRowDigestV1(value string) bool {
	return domainsecurity.IsSHA256Hex(value) && value == strings.ToLower(value)
}

func canonicalTransactionSourceRowPrivateTextV1(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func canonicalTransactionSourceRowOptionalTimestampV1(value string) bool {
	if len(value) > 64 || value != strings.TrimSpace(value) {
		return false
	}
	for index := range value {
		character := value[index]
		if (character < '0' || character > '9') && !strings.ContainsRune("-:. +TZ", rune(character)) {
			return false
		}
	}
	return true
}

func canonicalTransactionSourceRowDecimalV1(value string) string {
	if value == "" || len(value) > 128 || value != strings.TrimSpace(value) {
		return ""
	}
	negative := false
	unsigned := value
	if strings.HasPrefix(unsigned, "-") {
		negative = true
		unsigned = unsigned[1:]
	} else if strings.HasPrefix(unsigned, "+") {
		unsigned = unsigned[1:]
	}
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || len(parts) == 0 || parts[0] == "" ||
		len(parts) == 2 && parts[1] == "" || !allTransactionSourceRowDigitsV1(parts[0]) ||
		len(parts) == 2 && !allTransactionSourceRowDigitsV1(parts[1]) {
		return ""
	}
	integer := strings.TrimLeft(parts[0], "0")
	if integer == "" {
		integer = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = strings.TrimRight(parts[1], "0")
	}
	var result strings.Builder
	if negative && (integer != "0" || fraction != "") {
		result.WriteByte('-')
	}
	result.WriteString(integer)
	if fraction != "" {
		result.WriteByte('.')
		result.WriteString(fraction)
	}
	return result.String()
}

func allTransactionSourceRowDigitsV1(value string) bool {
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return value != ""
}

func cloneTransactionSourceRowHostViewV1(view TransactionSourceRowPageHostV1) TransactionSourceRowPageHostV1 {
	clone := view
	clone.NextCursor = cloneTransactionSourceRowCursorV1(view.NextCursor)
	clone.Inventory = append([]TransactionSourceRowInventoryHostV1(nil), view.Inventory...)
	clone.Rows = make([]TransactionSourceRowHostV1, len(view.Rows))
	for index, row := range view.Rows {
		clone.Rows[index] = row
		clone.Rows[index].CanonicalTypedRow = append([]TransactionSourceRowTypedFieldHostV1(nil), row.CanonicalTypedRow...)
	}
	return clone
}

func transactionSourceRowHostViewFromWireV1(wire transactionSourceRowPageWireV1) TransactionSourceRowPageHostV1 {
	view := TransactionSourceRowPageHostV1{
		SchemaVersion: wire.SchemaVersion, Operation: wire.Operation, CaseID: wire.CaseID,
		BindingKeyDigest: wire.BindingKeyDigest, ParsedGenerationIdentitySHA256: wire.ParsedGenerationIdentitySHA256,
		Relation: wire.Relation, MaterializationIdentity: wire.MaterializationIdentity,
		ProducerContentContract: wire.ProducerContentContract, ProducerContentID: wire.ProducerContentID,
		ProducerContentManifestSHA256: wire.ProducerContentManifestSHA256,
		RawArtifactManifestSHA256:     wire.RawArtifactManifestSHA256,
		DuckDBContentSnapshotDigest:   wire.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256:  wire.DuckDBSnapshotManifestSHA256,
		AnalyticalSchemaDigest:        wire.AnalyticalSchemaDigest, ParserID: wire.ParserID,
		ParserVersion: wire.ParserVersion, LocatorOrdering: wire.LocatorOrdering,
		SourceProofStatus: wire.SourceProofStatus, RequiredHostRawReplayProfile: wire.RequiredHostRawReplayProfile,
		HostRawReplayRequired: wire.HostRawReplayRequired,
		SourceSnapshot: TransactionSourceRowSnapshotHostV1{
			SourceRevision: wire.SourceSnapshot.SourceRevision, SourceRowCount: wire.SourceSnapshot.SourceRowCount,
			SourceMaxTxnTS: wire.SourceSnapshot.SourceMaxTxnTS, SourceMaxID: wire.SourceSnapshot.SourceMaxID,
			AcceptedRowCount: wire.SourceSnapshot.AcceptedRowCount, RejectedRowCount: wire.SourceSnapshot.RejectedRowCount,
			DuplicateRowCount: wire.SourceSnapshot.DuplicateRowCount, InventoryDigest: wire.SourceSnapshot.InventoryDigest,
			SourceSnapshotDigest: wire.SourceSnapshot.SourceSnapshotDigest,
		},
		NextCursor: cloneTransactionSourceRowCursorV1(wire.NextCursor), Complete: wire.Complete, PageDigest: wire.PageDigest,
	}
	view.Inventory = make([]TransactionSourceRowInventoryHostV1, len(wire.Inventory))
	for index, entry := range wire.Inventory {
		view.Inventory[index] = TransactionSourceRowInventoryHostV1{
			SourceFileID: entry.SourceFileID, SourceFileIDDigest: entry.SourceFileIDDigest,
			PrivateImportFileID: entry.PrivateImportFileID, SourceArtifactSHA256: entry.SourceArtifactSHA256,
			FileType: entry.FileType, RowCount: entry.RowCount, AcceptedRowCount: entry.AcceptedRowCount,
			RejectedRowCount: entry.RejectedRowCount,
		}
	}
	view.Rows = make([]TransactionSourceRowHostV1, len(wire.Rows))
	for index, row := range wire.Rows {
		view.Rows[index] = TransactionSourceRowHostV1{
			SourceFileID: row.SourceFileID, SourceFileIDDigest: row.SourceFileIDDigest,
			SourceRowNumber: row.SourceRowNumber, SourceArtifactSHA256: row.SourceArtifactSHA256,
			CanonicalRowSHA256: row.CanonicalRowSHA256, Disposition: row.Disposition,
			CanonicalTypedRow: make([]TransactionSourceRowTypedFieldHostV1, len(row.CanonicalTypedRow)),
		}
		for fieldIndex, field := range row.CanonicalTypedRow {
			view.Rows[index].CanonicalTypedRow[fieldIndex] = TransactionSourceRowTypedFieldHostV1{
				Name: field.Name, Scalar: TransactionSourceRowScalarHostV1{Kind: field.Scalar.Kind, Value: field.Scalar.Value},
			}
		}
	}
	return view
}

type transactionSourceRowOrderedNodeV1 struct {
	fields []transactionSourceRowOrderedFieldV1
	array  *transactionSourceRowOrderedNodeV1
	union  *transactionSourceRowOrderedNodeV1
}

type transactionSourceRowOrderedFieldV1 struct {
	name string
	node *transactionSourceRowOrderedNodeV1
}

func transactionSourceRowObjectOrderV1(names ...string) *transactionSourceRowOrderedNodeV1 {
	node := &transactionSourceRowOrderedNodeV1{fields: make([]transactionSourceRowOrderedFieldV1, len(names))}
	for index, name := range names {
		node.fields[index] = transactionSourceRowOrderedFieldV1{name: name}
	}
	return node
}

func validateTransactionSourceRowOrderedShapeV1(raw []byte) error {
	cursor := transactionSourceRowObjectOrderV1("sourceFileIdDigest", "sourceRowNumber")
	snapshot := transactionSourceRowObjectOrderV1(
		"sourceRevision", "sourceRowCount", "sourceMaxTxnTs", "sourceMaxId", "acceptedRowCount",
		"rejectedRowCount", "duplicateRowCount", "inventoryDigest", "sourceSnapshotDigest",
	)
	inventoryEntry := transactionSourceRowObjectOrderV1(
		"sourceFileId", "sourceFileIdDigest", "privateImportFileId", "sourceArtifactSha256", "fileType",
		"rowCount", "acceptedRowCount", "rejectedRowCount",
	)
	scalar := transactionSourceRowObjectOrderV1("kind", "value")
	field := transactionSourceRowObjectOrderV1("name", "scalar")
	field.fields[1].node = scalar
	row := transactionSourceRowObjectOrderV1(
		"sourceFileId", "sourceFileIdDigest", "sourceRowNumber", "sourceArtifactSha256", "canonicalRowSha256",
		"canonicalTypedRow", "disposition",
	)
	row.fields[5].node = &transactionSourceRowOrderedNodeV1{array: field}
	top := transactionSourceRowObjectOrderV1(
		"schemaVersion", "operation", "caseId", "bindingKeyDigest", "parsedGenerationIdentitySha256", "relation",
		"materializationIdentity", "producerContentContract", "producerContentId", "producerContentManifestSha256",
		"rawArtifactManifestSha256", "duckdbContentSnapshotDigest", "duckdbSnapshotManifestSha256",
		"analyticalSchemaDigest", "parserId", "parserVersion", "locatorOrdering", "sourceProofStatus",
		"requiredHostRawReplayProfile", "hostRawReplayRequired", "sourceSnapshot", "inventory", "rows",
		"nextCursor", "complete", "pageDigest",
	)
	top.fields[20].node = snapshot
	top.fields[21].node = &transactionSourceRowOrderedNodeV1{array: inventoryEntry}
	top.fields[22].node = &transactionSourceRowOrderedNodeV1{array: row}
	top.fields[23].node = &transactionSourceRowOrderedNodeV1{union: cursor}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if validateTransactionSourceRowOrderedNodeV1(decoder, top) != nil {
		return ErrResultInvalid
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return ErrResultInvalid
	}
	return nil
}

func validateTransactionSourceRowOrderedNodeV1(
	decoder *json.Decoder,
	node *transactionSourceRowOrderedNodeV1,
) error {
	if decoder == nil || node == nil {
		return ErrResultInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return ErrResultInvalid
	}
	if node.union != nil {
		if token == nil {
			return nil
		}
		return validateTransactionSourceRowOrderedObjectAfterOpenV1(decoder, node.union, token)
	}
	if node.array != nil {
		delimiter, ok := token.(json.Delim)
		if !ok || delimiter != '[' {
			return ErrResultInvalid
		}
		for decoder.More() {
			if validateTransactionSourceRowOrderedNodeV1(decoder, node.array) != nil {
				return ErrResultInvalid
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return ErrResultInvalid
		}
		return nil
	}
	if len(node.fields) > 0 {
		return validateTransactionSourceRowOrderedObjectAfterOpenV1(decoder, node, token)
	}
	return nil
}

func validateTransactionSourceRowOrderedObjectAfterOpenV1(
	decoder *json.Decoder,
	node *transactionSourceRowOrderedNodeV1,
	opening json.Token,
) error {
	delimiter, ok := opening.(json.Delim)
	if !ok || delimiter != '{' {
		return ErrResultInvalid
	}
	for _, field := range node.fields {
		if !decoder.More() {
			return ErrResultInvalid
		}
		name, err := decoder.Token()
		if err != nil || name != field.name {
			return ErrResultInvalid
		}
		if field.node != nil {
			if validateTransactionSourceRowOrderedNodeV1(decoder, field.node) != nil {
				return ErrResultInvalid
			}
			continue
		}
		if _, err := decoder.Token(); err != nil {
			return ErrResultInvalid
		}
	}
	if decoder.More() {
		return ErrResultInvalid
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return ErrResultInvalid
	}
	return nil
}
