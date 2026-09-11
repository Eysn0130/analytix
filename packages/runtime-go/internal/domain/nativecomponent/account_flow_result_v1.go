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
	"math/big"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	AccountFlowCoverageCompleteV1                    = "complete"
	AccountFlowCoveragePartialV1                     = "partial"
	AccountFlowCoverageObservedNoHitPendingBindingV1 = "observed_no_hit_pending_host_binding"

	AccountFlowGapRejectedSourceRowsV1  = "rejected_source_rows"
	AccountFlowGapDuplicateSourceRowsV1 = "duplicate_source_rows"
	AccountFlowGapUntimedSubjectRowsV1  = "untimed_subject_rows"
	AccountFlowGapEvidenceRowLimitV1    = "evidence_row_limit"

	AccountFlowDirectionInflowV1  = "inflow"
	AccountFlowDirectionOutflowV1 = "outflow"

	AccountFlowCurrentnessHostRevalidationRequiredV1 = "host_revalidation_required"
	AccountFlowSemanticHostResolutionRequiredV1      = "host_counterparty_resolution_required"

	accountFlowMaterializationIdentityPrefixV1 = "txn_daily_snapshot:v12:"
	accountFlowQueryHashDomainV1               = "analytix.account-flow-query-hash/v1"
	accountFlowResultHashDomainV1              = "analytix.account-flow-result-hash/v1"
)

type AccountFlowCoverageV1 struct {
	State                  string
	Gaps                   []string
	NormalizedSnapshotRows uint64
	AcceptedSnapshotRows   uint64
	RejectedSnapshotRows   uint64
	DuplicateSnapshotRows  uint64
	UntimedSubjectRows     uint64
	ObservedMatchingRows   uint64
}

type AccountFlowProvenanceV1 struct {
	DatasetSnapshotID              string
	ContextEpoch                   uint64
	ContextDigest                  string
	CaseBindingHash                string
	ExpectedProducerContentID      string
	ExpectedProducerManifestSHA256 string
	SubjectResolutionDigest        string
	DuckDBContentSnapshotDigest    string
	DuckDBSnapshotManifestSHA256   string
	MaterializationIdentity        string
	SourceSignature                string
	ResultSignature                string
	ProducerContentID              string
	ProducerManifestSHA256         string
	QueryContract                  string
	QuerySQLHash                   string
}

// AccountFlowEvidenceRowV1 is host-private until its source locator and
// counterparty fields are resolved to evidence/entity references. It cannot be
// JSON-marshaled or safely formatted by ordinary callers.
type AccountFlowEvidenceRowV1 struct {
	SubjectRef      string
	private         accountFlowEvidencePrivateV1
	SourceRowNumber uint64
	OccurredAt      string
	Direction       string
	AmountMinor     string
	Currency        string
	MinorUnitScale  uint8
}

type accountFlowEvidencePrivateValuesV1 struct {
	sourceFileID            string
	counterpartyKey         string
	counterpartyKeyPresent  bool
	counterpartyName        string
	counterpartyNamePresent bool
	counterpartyBank        string
	counterpartyBankPresent bool
}

// accountFlowEvidencePrivateV1 deliberately stores only a callback. Private
// row values live in the callback's closure, so a defined-type conversion of
// AccountFlowEvidenceRowV1 cannot make fmt or encoding/json disclose them.
type accountFlowEvidencePrivateV1 struct {
	use func(func(accountFlowEvidencePrivateValuesV1) error) error
}

func newAccountFlowEvidencePrivateV1(
	sourceFileID string,
	counterpartyKey *string,
	counterpartyName *string,
	counterpartyBank *string,
) accountFlowEvidencePrivateV1 {
	private := accountFlowEvidencePrivateValuesV1{sourceFileID: sourceFileID}
	if counterpartyKey != nil {
		private.counterpartyKey = *counterpartyKey
		private.counterpartyKeyPresent = true
	}
	if counterpartyName != nil {
		private.counterpartyName = *counterpartyName
		private.counterpartyNamePresent = true
	}
	if counterpartyBank != nil {
		private.counterpartyBank = *counterpartyBank
		private.counterpartyBankPresent = true
	}
	return accountFlowEvidencePrivateV1{
		use: func(consume func(accountFlowEvidencePrivateValuesV1) error) error {
			if consume == nil {
				return ErrResultInvalid
			}
			return consume(private)
		},
	}
}

func (private accountFlowEvidencePrivateV1) useExact(
	consume func(accountFlowEvidencePrivateValuesV1) error,
) error {
	if private.use == nil || consume == nil {
		return ErrResultInvalid
	}
	calls := 0
	err := private.use(func(values accountFlowEvidencePrivateValuesV1) error {
		calls++
		if calls != 1 {
			return ErrResultInvalid
		}
		return consume(values)
	})
	if err != nil || calls != 1 {
		return ErrResultInvalid
	}
	return nil
}

// accountFlowEvidenceRowsPrivateV1 similarly keeps the private row collection
// in a closure. count is safe metadata and the only directly stored value.
type accountFlowEvidenceRowsPrivateV1 struct {
	count int
	use   func(func(int, AccountFlowEvidenceRowV1) error) error
}

func newAccountFlowEvidenceRowsPrivateV1(rows []AccountFlowEvidenceRowV1) accountFlowEvidenceRowsPrivateV1 {
	cloned := append([]AccountFlowEvidenceRowV1(nil), rows...)
	return accountFlowEvidenceRowsPrivateV1{
		count: len(cloned),
		use: func(consume func(int, AccountFlowEvidenceRowV1) error) error {
			if consume == nil {
				return ErrResultInvalid
			}
			for index, row := range cloned {
				if err := consume(index, row); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func (private accountFlowEvidenceRowsPrivateV1) initialized() bool {
	return private.use != nil && private.count >= 0
}

func (private accountFlowEvidenceRowsPrivateV1) useExact(
	consume func(int, AccountFlowEvidenceRowV1) error,
) error {
	if !private.initialized() || consume == nil {
		return ErrResultInvalid
	}
	calls := 0
	err := private.use(func(index int, row AccountFlowEvidenceRowV1) error {
		if index != calls || calls >= private.count {
			return ErrResultInvalid
		}
		calls++
		return consume(index, row)
	})
	if err != nil || calls != private.count {
		return ErrResultInvalid
	}
	return nil
}

// AnalyzeAccountFlowsResultV1 is the exact host-private Rust result. It is not
// provider-safe and is not an EvidenceReceipt, ClaimRecord or UI result.
// Callers must resolve its private fields and revalidate current authority
// before any later projection.
type AnalyzeAccountFlowsResultV1 struct {
	SubjectRef              string
	StartInclusive          string
	EndInclusive            string
	Timezone                string
	Currency                string
	MinorUnitScale          uint8
	InflowMinor             string
	OutflowMinor            string
	NetMinor                string
	TransactionCount        uint64
	AggregateComplete       bool
	EvidenceRowsComplete    bool
	Coverage                AccountFlowCoverageV1
	Provenance              AccountFlowProvenanceV1
	Currentness             string
	SemanticProjectionState string
	evidenceRows            accountFlowEvidenceRowsPrivateV1
	QueryHash               string
	ResultHash              string
}

type accountFlowCoverageWireV1 struct {
	State                  string   `json:"state"`
	Gaps                   []string `json:"gaps"`
	NormalizedSnapshotRows uint64   `json:"normalizedSnapshotRows"`
	AcceptedSnapshotRows   uint64   `json:"acceptedSnapshotRows"`
	RejectedSnapshotRows   uint64   `json:"rejectedSnapshotRows"`
	DuplicateSnapshotRows  uint64   `json:"duplicateSnapshotRows"`
	UntimedSubjectRows     uint64   `json:"untimedSubjectRows"`
	ObservedMatchingRows   uint64   `json:"observedMatchingRows"`
}

type accountFlowProvenanceWireV1 struct {
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	ContextDigest                  string `json:"contextDigest"`
	CaseBindingHash                string `json:"caseBindingHash"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
	DuckDBContentSnapshotDigest    string `json:"duckdbContentSnapshotDigest"`
	DuckDBSnapshotManifestSHA256   string `json:"duckdbSnapshotManifestSha256"`
	MaterializationIdentity        string `json:"materializationIdentity"`
	SourceSignature                string `json:"sourceSignature"`
	ResultSignature                string `json:"resultSignature"`
	ProducerContentID              string `json:"producerContentId"`
	ProducerManifestSHA256         string `json:"producerManifestSha256"`
	QueryContract                  string `json:"queryContract"`
	QuerySQLHash                   string `json:"querySqlHash"`
}

type accountFlowEvidenceRowWireV1 struct {
	SubjectRef       string  `json:"subjectRef"`
	SourceFileID     string  `json:"sourceFileId"`
	SourceRowNumber  uint64  `json:"sourceRowNumber"`
	CounterpartyKey  *string `json:"counterpartyKey"`
	CounterpartyName *string `json:"counterpartyName"`
	CounterpartyBank *string `json:"counterpartyBank"`
	OccurredAt       string  `json:"occurredAt"`
	Direction        string  `json:"direction"`
	AmountMinor      string  `json:"amountMinor"`
	Currency         string  `json:"currency"`
	MinorUnitScale   uint8   `json:"minorUnitScale"`
}

type analyzeAccountFlowsResultWireV1 struct {
	SubjectRef              string                         `json:"subjectRef"`
	StartInclusive          string                         `json:"startInclusive"`
	EndInclusive            string                         `json:"endInclusive"`
	Timezone                string                         `json:"timezone"`
	Currency                string                         `json:"currency"`
	MinorUnitScale          uint8                          `json:"minorUnitScale"`
	InflowMinor             string                         `json:"inflowMinor"`
	OutflowMinor            string                         `json:"outflowMinor"`
	NetMinor                string                         `json:"netMinor"`
	TransactionCount        uint64                         `json:"transactionCount"`
	AggregateComplete       bool                           `json:"aggregateComplete"`
	EvidenceRowsComplete    bool                           `json:"evidenceRowsComplete"`
	Coverage                accountFlowCoverageWireV1      `json:"coverage"`
	Provenance              accountFlowProvenanceWireV1    `json:"provenance"`
	Currentness             string                         `json:"currentness"`
	SemanticProjectionState string                         `json:"semanticProjectionState"`
	EvidenceRows            []accountFlowEvidenceRowWireV1 `json:"evidenceRows"`
	QueryHash               string                         `json:"queryHash"`
	ResultHash              string                         `json:"resultHash"`
}

type accountFlowResultHashMaterialWireV1 struct {
	SubjectRef              string                         `json:"subjectRef"`
	StartInclusive          string                         `json:"startInclusive"`
	EndInclusive            string                         `json:"endInclusive"`
	Timezone                string                         `json:"timezone"`
	Currency                string                         `json:"currency"`
	MinorUnitScale          uint8                          `json:"minorUnitScale"`
	InflowMinor             string                         `json:"inflowMinor"`
	OutflowMinor            string                         `json:"outflowMinor"`
	NetMinor                string                         `json:"netMinor"`
	TransactionCount        uint64                         `json:"transactionCount"`
	AggregateComplete       bool                           `json:"aggregateComplete"`
	EvidenceRowsComplete    bool                           `json:"evidenceRowsComplete"`
	Coverage                accountFlowCoverageWireV1      `json:"coverage"`
	Provenance              accountFlowProvenanceWireV1    `json:"provenance"`
	Currentness             string                         `json:"currentness"`
	SemanticProjectionState string                         `json:"semanticProjectionState"`
	EvidenceRows            []accountFlowEvidenceRowWireV1 `json:"evidenceRows"`
	QueryHash               string                         `json:"queryHash"`
}

// The field order is lexicographic because serde_json's map representation is
// a BTreeMap without preserve_order in the pinned Rust crate.
type accountFlowQueryScopeWireV1 struct {
	CaseBindingHash                string `json:"caseBindingHash"`
	ContextDigest                  string `json:"contextDigest"`
	ContextEpoch                   uint64 `json:"contextEpoch"`
	Contract                       string `json:"contract"`
	DatasetSnapshotID              string `json:"datasetSnapshotId"`
	DatasetUTCOffsetMinutes        int16  `json:"datasetUtcOffsetMinutes"`
	EndInclusive                   string `json:"endInclusive"`
	EvidenceRowLimit               uint32 `json:"evidenceRowLimit"`
	ExpectedCurrency               string `json:"expectedCurrency"`
	ExpectedProducerContentID      string `json:"expectedProducerContentId"`
	ExpectedProducerManifestSHA256 string `json:"expectedProducerManifestSha256"`
	MinorUnitScale                 uint8  `json:"minorUnitScale"`
	QuerySQLHash                   string `json:"querySqlHash"`
	ScanCap                        uint32 `json:"scanCap"`
	StartInclusive                 string `json:"startInclusive"`
	SubjectRef                     string `json:"subjectRef"`
	SubjectResolutionDigest        string `json:"subjectResolutionDigest"`
}

func ParseAnalyzeAccountFlowsResultV1(
	body []byte,
	arguments AnalyzeAccountFlowsArgumentsV1,
) (AnalyzeAccountFlowsResultV1, error) {
	if validateAnalyzeAccountFlowsArgumentsV1(arguments) != nil ||
		domainjsonstrict.Validate(body, domainjsonstrict.Options{
			RequireObject:  true,
			MaxBytes:       AccountFlowMaximumResultBytesV1,
			MaxDepth:       8,
			MaxTokens:      64_000,
			MaxStringBytes: accountFlowMaximumPrivateTextBytesV1,
			MaxNumberBytes: 32,
			MaxAbsExponent: 1,
		}) != nil || validateAccountFlowResultJSONShapeV1(body) != nil {
		return AnalyzeAccountFlowsResultV1{}, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var wire analyzeAccountFlowsResultWireV1
	if err := decoder.Decode(&wire); err != nil {
		return AnalyzeAccountFlowsResultV1{}, ErrResultInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return AnalyzeAccountFlowsResultV1{}, ErrResultInvalid
	}
	result := accountFlowResultFromWireV1(wire)
	if ValidateAnalyzeAccountFlowsResultV1(result, arguments) != nil {
		return AnalyzeAccountFlowsResultV1{}, ErrResultInvalid
	}
	return result, nil
}

func ValidateAnalyzeAccountFlowsResultV1(
	result AnalyzeAccountFlowsResultV1,
	arguments AnalyzeAccountFlowsArgumentsV1,
) error {
	if validateAnalyzeAccountFlowsArgumentsV1(arguments) != nil ||
		result.SubjectRef != arguments.subjectRef ||
		result.StartInclusive != arguments.startInclusive ||
		result.EndInclusive != arguments.endInclusive ||
		result.Timezone != accountFlowTimezoneV1(arguments.datasetUTCOffsetMinutes) ||
		result.Currency != arguments.expectedCurrency ||
		result.MinorUnitScale != arguments.minorUnitScale ||
		result.TransactionCount > uint64(arguments.scanCap) ||
		result.Currentness != AccountFlowCurrentnessHostRevalidationRequiredV1 ||
		result.SemanticProjectionState != AccountFlowSemanticHostResolutionRequiredV1 ||
		!canonicalAccountFlowDigestV1(result.QueryHash) ||
		!canonicalAccountFlowDigestV1(result.ResultHash) ||
		!result.evidenceRows.initialized() || result.evidenceRows.count > int(arguments.evidenceRowLimit) ||
		uint64(result.evidenceRows.count) > result.TransactionCount ||
		validateAccountFlowCoverageV1(result) != nil ||
		validateAccountFlowProvenanceV1(result.Provenance, arguments) != nil ||
		validateAccountFlowAmountsAndRowsV1(result, arguments) != nil {
		return ErrResultInvalid
	}
	wantQueryHash := accountFlowQueryHashV1(arguments, result.Provenance.DuckDBContentSnapshotDigest)
	wantResultHash := accountFlowResultHashV1(result)
	if wantQueryHash == "" || result.QueryHash != wantQueryHash ||
		wantResultHash == "" || result.ResultHash != wantResultHash {
		return ErrResultInvalid
	}
	return nil
}

func (result AnalyzeAccountFlowsResultV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}

func (result AnalyzeAccountFlowsResultV1) String() string {
	return fmt.Sprintf(
		"AnalyzeAccountFlowsResultV1{subjectRef:[REDACTED],range:%q..%q,currency:%q,transactionCount:%d,evidenceRows:%d,privateFields:[REDACTED],queryHash:%q,resultHash:%q}",
		result.StartInclusive,
		result.EndInclusive,
		result.Currency,
		result.TransactionCount,
		result.evidenceRows.count,
		result.QueryHash,
		result.ResultHash,
	)
}

func (result AnalyzeAccountFlowsResultV1) GoString() string {
	return result.String()
}

func (row AccountFlowEvidenceRowV1) MarshalJSON() ([]byte, error) {
	return nil, ErrResultInvalid
}

func (row AccountFlowEvidenceRowV1) String() string {
	return "AccountFlowEvidenceRowV1{hostPrivate:true,values:[REDACTED]}"
}

func (row AccountFlowEvidenceRowV1) GoString() string {
	return row.String()
}

// EvidenceRowCountV1 is provider-safe metadata. Source locators and
// counterparty values remain package-private until a later in-authority
// evidence projection resolves them to stable references.
func (result AnalyzeAccountFlowsResultV1) EvidenceRowCountV1() int {
	if !result.evidenceRows.initialized() {
		return 0
	}
	return result.evidenceRows.count
}

func validateAccountFlowCoverageV1(result AnalyzeAccountFlowsResultV1) error {
	coverage := result.Coverage
	wantAggregateComplete := coverage.RejectedSnapshotRows == 0 &&
		coverage.DuplicateSnapshotRows == 0 && coverage.UntimedSubjectRows == 0
	coverageRemainder := coverage.NormalizedSnapshotRows
	if coverage.AcceptedSnapshotRows > coverageRemainder {
		return ErrResultInvalid
	}
	coverageRemainder -= coverage.AcceptedSnapshotRows
	if coverage.RejectedSnapshotRows > coverageRemainder {
		return ErrResultInvalid
	}
	coverageRemainder -= coverage.RejectedSnapshotRows
	if result.AggregateComplete != wantAggregateComplete ||
		coverage.ObservedMatchingRows != result.TransactionCount ||
		coverage.DuplicateSnapshotRows != coverageRemainder ||
		coverage.UntimedSubjectRows > coverage.AcceptedSnapshotRows ||
		coverage.ObservedMatchingRows > coverage.AcceptedSnapshotRows ||
		coverage.Gaps == nil {
		return ErrResultInvalid
	}
	wantGaps := make([]string, 0, 4)
	if coverage.RejectedSnapshotRows != 0 {
		wantGaps = append(wantGaps, AccountFlowGapRejectedSourceRowsV1)
	}
	if coverage.DuplicateSnapshotRows != 0 {
		wantGaps = append(wantGaps, AccountFlowGapDuplicateSourceRowsV1)
	}
	if coverage.UntimedSubjectRows != 0 {
		wantGaps = append(wantGaps, AccountFlowGapUntimedSubjectRowsV1)
	}
	if !result.EvidenceRowsComplete {
		wantGaps = append(wantGaps, AccountFlowGapEvidenceRowLimitV1)
	}
	if !equalAccountFlowStringsV1(coverage.Gaps, wantGaps) {
		return ErrResultInvalid
	}
	wantState := AccountFlowCoverageCompleteV1
	if !result.AggregateComplete || !result.EvidenceRowsComplete {
		wantState = AccountFlowCoveragePartialV1
	} else if result.TransactionCount == 0 {
		wantState = AccountFlowCoverageObservedNoHitPendingBindingV1
	}
	if coverage.State != wantState {
		return ErrResultInvalid
	}
	return nil
}

func validateAccountFlowProvenanceV1(
	provenance AccountFlowProvenanceV1,
	arguments AnalyzeAccountFlowsArgumentsV1,
) error {
	if provenance.DatasetSnapshotID != arguments.datasetSnapshotID ||
		provenance.ContextEpoch != arguments.contextEpoch ||
		provenance.ContextDigest != arguments.contextDigest ||
		provenance.CaseBindingHash != arguments.caseBindingHash ||
		provenance.ExpectedProducerContentID != arguments.expectedProducerContentID ||
		provenance.ExpectedProducerManifestSHA256 != arguments.expectedProducerManifestSHA256 ||
		provenance.SubjectResolutionDigest != arguments.subjectResolutionDigest ||
		provenance.DuckDBContentSnapshotDigest != arguments.expectedDuckDBContentSnapshotDigest ||
		provenance.DuckDBSnapshotManifestSHA256 != arguments.expectedDuckDBSnapshotManifestSHA256 ||
		provenance.MaterializationIdentity != arguments.expectedMaterializationIdentity ||
		!canonicalAccountFlowDigestV1(provenance.SourceSignature) ||
		!canonicalAccountFlowDigestV1(provenance.ResultSignature) ||
		provenance.MaterializationIdentity != accountFlowMaterializationIdentityPrefixV1+provenance.SourceSignature ||
		provenance.ProducerContentID != arguments.expectedProducerContentID ||
		provenance.ProducerManifestSHA256 != arguments.expectedProducerManifestSHA256 ||
		provenance.QueryContract != AccountFlowQueryContractV1 ||
		provenance.QuerySQLHash != accountFlowQuerySQLHashV1 {
		return ErrResultInvalid
	}
	return nil
}

func validateAccountFlowAmountsAndRowsV1(
	result AnalyzeAccountFlowsResultV1,
	arguments AnalyzeAccountFlowsArgumentsV1,
) error {
	inflow, inflowOK := canonicalAccountFlowIntegerV1(result.InflowMinor, false)
	outflow, outflowOK := canonicalAccountFlowIntegerV1(result.OutflowMinor, false)
	net, netOK := canonicalAccountFlowIntegerV1(result.NetMinor, true)
	if !inflowOK || !outflowOK || !netOK || new(big.Int).Sub(inflow, outflow).Cmp(net) != 0 {
		return ErrResultInvalid
	}
	wantEvidenceComplete := result.TransactionCount <= uint64(arguments.evidenceRowLimit)
	wantRows := result.TransactionCount
	if wantRows > uint64(arguments.evidenceRowLimit) {
		wantRows = uint64(arguments.evidenceRowLimit)
	}
	if result.EvidenceRowsComplete != wantEvidenceComplete || uint64(result.evidenceRows.count) != wantRows {
		return ErrResultInvalid
	}
	sumInflow := new(big.Int)
	sumOutflow := new(big.Int)
	seen := make(map[string]struct{}, result.evidenceRows.count)
	previousTime := time.Time{}
	if err := result.evidenceRows.useExact(func(_ int, row AccountFlowEvidenceRowV1) error {
		occurredAt, timestampOK := canonicalAccountFlowTimestampV1(row.OccurredAt)
		parsedOccurredAt, parseErr := time.Parse(time.RFC3339Nano, row.OccurredAt)
		amount, amountOK := canonicalAccountFlowIntegerV1(row.AmountMinor, false)
		if row.SubjectRef != result.SubjectRef ||
			row.SourceRowNumber == 0 || row.SourceRowNumber > accountFlowMaximumSourceRowNumberV1 ||
			!timestampOK || occurredAt != row.OccurredAt || parseErr != nil ||
			parsedOccurredAt.Before(mustAccountFlowTimeV1(arguments.startInclusive)) ||
			parsedOccurredAt.After(mustAccountFlowTimeV1(arguments.endInclusive)) ||
			!amountOK || row.Currency != result.Currency || row.MinorUnitScale != result.MinorUnitScale {
			return ErrResultInvalid
		}
		if err := row.private.useExact(func(private accountFlowEvidencePrivateValuesV1) error {
			locator := private.sourceFileID + "\x00" + fmt.Sprintf("%d", row.SourceRowNumber)
			if !canonicalBoundedAccountFlowTextV1(private.sourceFileID, accountFlowMaximumPrivateTextBytesV1) ||
				!canonicalOptionalAccountFlowPrivateValueV1(private.counterpartyKey, private.counterpartyKeyPresent) ||
				!canonicalOptionalAccountFlowPrivateValueV1(private.counterpartyName, private.counterpartyNamePresent) ||
				!canonicalOptionalAccountFlowPrivateValueV1(private.counterpartyBank, private.counterpartyBankPresent) {
				return ErrResultInvalid
			}
			if _, duplicate := seen[locator]; duplicate {
				return ErrResultInvalid
			}
			seen[locator] = struct{}{}
			return nil
		}); err != nil {
			return ErrResultInvalid
		}
		if !previousTime.IsZero() && parsedOccurredAt.Before(previousTime) {
			return ErrResultInvalid
		}
		previousTime = parsedOccurredAt
		switch row.Direction {
		case AccountFlowDirectionInflowV1:
			sumInflow.Add(sumInflow, amount)
		case AccountFlowDirectionOutflowV1:
			sumOutflow.Add(sumOutflow, amount)
		default:
			return ErrResultInvalid
		}
		return nil
	}); err != nil {
		return ErrResultInvalid
	}
	if result.EvidenceRowsComplete && (sumInflow.Cmp(inflow) != 0 || sumOutflow.Cmp(outflow) != 0) {
		return ErrResultInvalid
	}
	return nil
}

func accountFlowQueryHashV1(arguments AnalyzeAccountFlowsArgumentsV1, duckDBDigest string) string {
	if validateAnalyzeAccountFlowsArgumentsV1(arguments) != nil || !canonicalAccountFlowDigestV1(duckDBDigest) {
		return ""
	}
	body, err := marshalAccountFlowRustJSONV1(accountFlowQueryScopeWireV1{
		CaseBindingHash:                arguments.caseBindingHash,
		ContextDigest:                  arguments.contextDigest,
		ContextEpoch:                   arguments.contextEpoch,
		Contract:                       AccountFlowQueryContractV1,
		DatasetSnapshotID:              arguments.datasetSnapshotID,
		DatasetUTCOffsetMinutes:        arguments.datasetUTCOffsetMinutes,
		EndInclusive:                   arguments.endInclusive,
		EvidenceRowLimit:               arguments.evidenceRowLimit,
		ExpectedCurrency:               arguments.expectedCurrency,
		ExpectedProducerContentID:      arguments.expectedProducerContentID,
		ExpectedProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
		MinorUnitScale:                 arguments.minorUnitScale,
		QuerySQLHash:                   accountFlowQuerySQLHashV1,
		ScanCap:                        arguments.scanCap,
		StartInclusive:                 arguments.startInclusive,
		SubjectRef:                     arguments.subjectRef,
		SubjectResolutionDigest:        arguments.subjectResolutionDigest,
	})
	if err != nil {
		return ""
	}
	return accountFlowFramedHashV1(accountFlowQueryHashDomainV1, []byte(duckDBDigest), body)
}

func accountFlowResultHashV1(result AnalyzeAccountFlowsResultV1) string {
	wire := accountFlowResultToWireV1(result)
	body, err := marshalAccountFlowRustJSONV1(accountFlowResultHashMaterialWireV1{
		SubjectRef:              wire.SubjectRef,
		StartInclusive:          wire.StartInclusive,
		EndInclusive:            wire.EndInclusive,
		Timezone:                wire.Timezone,
		Currency:                wire.Currency,
		MinorUnitScale:          wire.MinorUnitScale,
		InflowMinor:             wire.InflowMinor,
		OutflowMinor:            wire.OutflowMinor,
		NetMinor:                wire.NetMinor,
		TransactionCount:        wire.TransactionCount,
		AggregateComplete:       wire.AggregateComplete,
		EvidenceRowsComplete:    wire.EvidenceRowsComplete,
		Coverage:                wire.Coverage,
		Provenance:              wire.Provenance,
		Currentness:             wire.Currentness,
		SemanticProjectionState: wire.SemanticProjectionState,
		EvidenceRows:            wire.EvidenceRows,
		QueryHash:               wire.QueryHash,
	})
	if err != nil {
		return ""
	}
	return accountFlowFramedHashV1(accountFlowResultHashDomainV1, body)
}

func marshalAccountFlowRustJSONV1(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	body := bytes.TrimSuffix(output.Bytes(), []byte{'\n'})
	if len(body) == 0 {
		return nil, ErrResultInvalid
	}
	return append([]byte(nil), body...), nil
}

func accountFlowFramedHashV1(domain string, values ...[]byte) string {
	hasher := sha256.New()
	for _, value := range append([][]byte{[]byte(domain)}, values...) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write(value)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func canonicalAccountFlowIntegerV1(value string, signed bool) (*big.Int, bool) {
	if value == "" || len(value) > 128 || strings.HasPrefix(value, "+") ||
		strings.HasPrefix(value, "-") && (!signed || value == "-0") {
		return nil, false
	}
	digits := value
	if strings.HasPrefix(digits, "-") {
		digits = digits[1:]
	}
	if digits == "" || len(digits) > 1 && digits[0] == '0' {
		return nil, false
	}
	for index := range digits {
		if digits[index] < '0' || digits[index] > '9' {
			return nil, false
		}
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, false
	}
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
	if new(big.Int).Abs(new(big.Int).Set(parsed)).Cmp(maximum) > 0 {
		return nil, false
	}
	return parsed, true
}

func canonicalOptionalAccountFlowPrivateValueV1(value string, present bool) bool {
	return !present && value == "" || present && canonicalBoundedAccountFlowTextV1(value, accountFlowMaximumPrivateTextBytesV1)
}

func mustAccountFlowTimeV1(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func equalAccountFlowStringsV1(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func accountFlowResultFromWireV1(wire analyzeAccountFlowsResultWireV1) AnalyzeAccountFlowsResultV1 {
	rows := make([]AccountFlowEvidenceRowV1, len(wire.EvidenceRows))
	for index, row := range wire.EvidenceRows {
		rows[index] = AccountFlowEvidenceRowV1{
			SubjectRef: row.SubjectRef,
			private: newAccountFlowEvidencePrivateV1(
				row.SourceFileID,
				row.CounterpartyKey,
				row.CounterpartyName,
				row.CounterpartyBank,
			),
			SourceRowNumber: row.SourceRowNumber, OccurredAt: row.OccurredAt, Direction: row.Direction,
			AmountMinor: row.AmountMinor, Currency: row.Currency, MinorUnitScale: row.MinorUnitScale,
		}
	}
	return AnalyzeAccountFlowsResultV1{
		SubjectRef: wire.SubjectRef, StartInclusive: wire.StartInclusive, EndInclusive: wire.EndInclusive,
		Timezone: wire.Timezone, Currency: wire.Currency, MinorUnitScale: wire.MinorUnitScale,
		InflowMinor: wire.InflowMinor, OutflowMinor: wire.OutflowMinor, NetMinor: wire.NetMinor,
		TransactionCount: wire.TransactionCount, AggregateComplete: wire.AggregateComplete,
		EvidenceRowsComplete: wire.EvidenceRowsComplete,
		Coverage: AccountFlowCoverageV1{
			State: wire.Coverage.State, Gaps: cloneAccountFlowStringsV1(wire.Coverage.Gaps),
			NormalizedSnapshotRows: wire.Coverage.NormalizedSnapshotRows,
			AcceptedSnapshotRows:   wire.Coverage.AcceptedSnapshotRows,
			RejectedSnapshotRows:   wire.Coverage.RejectedSnapshotRows,
			DuplicateSnapshotRows:  wire.Coverage.DuplicateSnapshotRows,
			UntimedSubjectRows:     wire.Coverage.UntimedSubjectRows,
			ObservedMatchingRows:   wire.Coverage.ObservedMatchingRows,
		},
		Provenance: AccountFlowProvenanceV1{
			DatasetSnapshotID: wire.Provenance.DatasetSnapshotID, ContextEpoch: wire.Provenance.ContextEpoch,
			ContextDigest: wire.Provenance.ContextDigest, CaseBindingHash: wire.Provenance.CaseBindingHash,
			ExpectedProducerContentID:      wire.Provenance.ExpectedProducerContentID,
			ExpectedProducerManifestSHA256: wire.Provenance.ExpectedProducerManifestSHA256,
			SubjectResolutionDigest:        wire.Provenance.SubjectResolutionDigest,
			DuckDBContentSnapshotDigest:    wire.Provenance.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:   wire.Provenance.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:        wire.Provenance.MaterializationIdentity,
			SourceSignature:                wire.Provenance.SourceSignature, ResultSignature: wire.Provenance.ResultSignature,
			ProducerContentID:      wire.Provenance.ProducerContentID,
			ProducerManifestSHA256: wire.Provenance.ProducerManifestSHA256,
			QueryContract:          wire.Provenance.QueryContract, QuerySQLHash: wire.Provenance.QuerySQLHash,
		},
		Currentness: wire.Currentness, SemanticProjectionState: wire.SemanticProjectionState,
		evidenceRows: newAccountFlowEvidenceRowsPrivateV1(rows), QueryHash: wire.QueryHash, ResultHash: wire.ResultHash,
	}
}

func accountFlowResultToWireV1(result AnalyzeAccountFlowsResultV1) analyzeAccountFlowsResultWireV1 {
	var rows []accountFlowEvidenceRowWireV1
	if result.evidenceRows.initialized() {
		rows = make([]accountFlowEvidenceRowWireV1, result.evidenceRows.count)
		if err := result.evidenceRows.useExact(func(index int, row AccountFlowEvidenceRowV1) error {
			return row.private.useExact(func(private accountFlowEvidencePrivateValuesV1) error {
				rows[index] = accountFlowEvidenceRowWireV1{
					SubjectRef: row.SubjectRef, SourceFileID: private.sourceFileID, SourceRowNumber: row.SourceRowNumber,
					CounterpartyKey:  optionalAccountFlowPrivateTextV1(private.counterpartyKey, private.counterpartyKeyPresent),
					CounterpartyName: optionalAccountFlowPrivateTextV1(private.counterpartyName, private.counterpartyNamePresent),
					CounterpartyBank: optionalAccountFlowPrivateTextV1(private.counterpartyBank, private.counterpartyBankPresent),
					OccurredAt:       row.OccurredAt, Direction: row.Direction,
					AmountMinor: row.AmountMinor, Currency: row.Currency, MinorUnitScale: row.MinorUnitScale,
				}
				return nil
			})
		}); err != nil {
			rows = nil
		}
	}
	return analyzeAccountFlowsResultWireV1{
		SubjectRef: result.SubjectRef, StartInclusive: result.StartInclusive, EndInclusive: result.EndInclusive,
		Timezone: result.Timezone, Currency: result.Currency, MinorUnitScale: result.MinorUnitScale,
		InflowMinor: result.InflowMinor, OutflowMinor: result.OutflowMinor, NetMinor: result.NetMinor,
		TransactionCount: result.TransactionCount, AggregateComplete: result.AggregateComplete,
		EvidenceRowsComplete: result.EvidenceRowsComplete,
		Coverage: accountFlowCoverageWireV1{
			State: result.Coverage.State, Gaps: cloneAccountFlowStringsV1(result.Coverage.Gaps),
			NormalizedSnapshotRows: result.Coverage.NormalizedSnapshotRows,
			AcceptedSnapshotRows:   result.Coverage.AcceptedSnapshotRows,
			RejectedSnapshotRows:   result.Coverage.RejectedSnapshotRows,
			DuplicateSnapshotRows:  result.Coverage.DuplicateSnapshotRows,
			UntimedSubjectRows:     result.Coverage.UntimedSubjectRows,
			ObservedMatchingRows:   result.Coverage.ObservedMatchingRows,
		},
		Provenance: accountFlowProvenanceWireV1{
			DatasetSnapshotID: result.Provenance.DatasetSnapshotID, ContextEpoch: result.Provenance.ContextEpoch,
			ContextDigest: result.Provenance.ContextDigest, CaseBindingHash: result.Provenance.CaseBindingHash,
			ExpectedProducerContentID:      result.Provenance.ExpectedProducerContentID,
			ExpectedProducerManifestSHA256: result.Provenance.ExpectedProducerManifestSHA256,
			SubjectResolutionDigest:        result.Provenance.SubjectResolutionDigest,
			DuckDBContentSnapshotDigest:    result.Provenance.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:   result.Provenance.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:        result.Provenance.MaterializationIdentity,
			SourceSignature:                result.Provenance.SourceSignature, ResultSignature: result.Provenance.ResultSignature,
			ProducerContentID:      result.Provenance.ProducerContentID,
			ProducerManifestSHA256: result.Provenance.ProducerManifestSHA256,
			QueryContract:          result.Provenance.QueryContract, QuerySQLHash: result.Provenance.QuerySQLHash,
		},
		Currentness: result.Currentness, SemanticProjectionState: result.SemanticProjectionState,
		EvidenceRows: rows, QueryHash: result.QueryHash, ResultHash: result.ResultHash,
	}
}

func optionalAccountFlowPrivateTextV1(value string, present bool) *string {
	if !present {
		return nil
	}
	cloned := value
	return &cloned
}

func validateAccountFlowResultJSONShapeV1(body []byte) error {
	top, err := exactAccountFlowJSONObjectV1(body, []string{
		"subjectRef", "startInclusive", "endInclusive", "timezone", "currency", "minorUnitScale",
		"inflowMinor", "outflowMinor", "netMinor", "transactionCount", "aggregateComplete",
		"evidenceRowsComplete", "coverage", "provenance", "currentness", "semanticProjectionState",
		"evidenceRows", "queryHash", "resultHash",
	}, nil)
	if err != nil {
		return err
	}
	if _, err := exactAccountFlowJSONObjectV1(top["coverage"], []string{
		"state", "gaps", "normalizedSnapshotRows", "acceptedSnapshotRows", "rejectedSnapshotRows",
		"duplicateSnapshotRows", "untimedSubjectRows", "observedMatchingRows",
	}, nil); err != nil {
		return err
	}
	if _, err := exactAccountFlowJSONObjectV1(top["provenance"], []string{
		"datasetSnapshotId", "contextEpoch", "contextDigest", "caseBindingHash",
		"expectedProducerContentId", "expectedProducerManifestSha256", "subjectResolutionDigest",
		"duckdbContentSnapshotDigest", "duckdbSnapshotManifestSha256", "materializationIdentity",
		"sourceSignature", "resultSignature", "producerContentId", "producerManifestSha256",
		"queryContract", "querySqlHash",
	}, nil); err != nil {
		return err
	}
	var rows []json.RawMessage
	if json.Unmarshal(top["evidenceRows"], &rows) != nil || rows == nil {
		return ErrResultInvalid
	}
	for _, row := range rows {
		if _, err := exactAccountFlowJSONObjectV1(row, []string{
			"subjectRef", "sourceFileId", "sourceRowNumber", "counterpartyKey", "counterpartyName",
			"counterpartyBank", "occurredAt", "direction", "amountMinor", "currency", "minorUnitScale",
		}, map[string]bool{"counterpartyKey": true, "counterpartyName": true, "counterpartyBank": true}); err != nil {
			return err
		}
	}
	return nil
}

func exactAccountFlowJSONObjectV1(
	body []byte,
	keys []string,
	nullable map[string]bool,
) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || len(object) != len(keys) {
		return nil, ErrResultInvalid
	}
	for _, key := range keys {
		value, ok := object[key]
		if !ok || len(bytes.TrimSpace(value)) == 0 ||
			bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !nullable[key] {
			return nil, ErrResultInvalid
		}
	}
	return object, nil
}

func cloneAccountFlowStringsV1(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
