package nativecomponent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"reflect"
	"strings"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const AccountFlowProviderModelPurposeV1 = "analytix.account-flow-provider-semantics/v3"

// AccountFlowProviderModelOutputV1 is the closed, provider-visible semantic
// envelope emitted only after the host has validated the exact funds result.
// It deliberately has no field for raw identifiers, paths, SQL, source
// locators, grants, or private authority material.
type AccountFlowProviderModelOutputV1 struct {
	SchemaVersion  int                                 `json:"schemaVersion"`
	Purpose        string                              `json:"purpose"`
	SemanticStatus domainevidence.SemanticStatus       `json:"semanticStatus"`
	Data           AccountFlowProviderSemanticResultV1 `json:"data"`
}

// CanonicalAccountFlowProviderModelOutputV1 accepts only the exact closed
// account-flow semantic envelope and returns its canonical JSON. The caller
// can bind these bytes to an attempt-local message without granting a generic
// exception from provider privacy projection.
func CanonicalAccountFlowProviderModelOutputV1(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil || domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxDepth: 128,
		MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}) != nil {
		return nil, ErrResultInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var envelope AccountFlowProviderModelOutputV1
	if err := decoder.Decode(&envelope); err != nil ||
		!errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		envelope.SchemaVersion != 3 ||
		envelope.Purpose != AccountFlowProviderModelPurposeV1 ||
		(envelope.SemanticStatus != domainevidence.SemanticSuccess &&
			envelope.SemanticStatus != domainevidence.SemanticPartial) {
		return nil, ErrResultInvalid
	}
	if ValidateAccountFlowProviderSemanticResultV1(
		envelope.Data,
		envelope.Data.EvidenceRowLimit,
		envelope.SemanticStatus,
	) != nil || validateAccountFlowProviderFreeTextV1(envelope.Data) != nil {
		return nil, ErrResultInvalid
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || len(canonical) == 0 ||
		domainsecret.ValidateValueV1(json.RawMessage(canonical)) != nil {
		return nil, ErrResultInvalid
	}
	return canonical, nil
}

// ValidateAccountFlowProviderSemanticResultV1 rechecks the complete semantic
// result independently of the private native carrier. evidenceRowLimit is the
// already-authorized provider-visible query bound used to prove row coverage.
func ValidateAccountFlowProviderSemanticResultV1(
	semantic AccountFlowProviderSemanticResultV1,
	expectedEvidenceRowLimit uint32,
	status domainevidence.SemanticStatus,
) error {
	start, startOK := canonicalAccountFlowTimestampV1(semantic.StartInclusive)
	end, endOK := canonicalAccountFlowTimestampV1(semantic.EndInclusive)
	if domaincaseentity.ValidateModelEntityAliasV1(semantic.SubjectAlias) != nil ||
		!startOK || !endOK || start != semantic.StartInclusive || end != semantic.EndInclusive || start > end ||
		!validAccountFlowProviderTimezoneV1(semantic.Timezone) || !validAccountFlowProviderCurrencyV1(semantic.Currency) ||
		semantic.MinorUnitScale != AccountFlowMinorUnitScaleV1 ||
		semantic.TransactionCount > uint64(AccountFlowMaximumScanRowsV1) ||
		semantic.EvidenceRowLimit == 0 ||
		semantic.EvidenceRowLimit > AccountFlowMaximumEvidenceRowsV1 ||
		semantic.EvidenceRowLimit != expectedEvidenceRowLimit ||
		semantic.Currentness != AccountFlowProviderCurrentnessCurrentV1 ||
		semantic.Transactions == nil ||
		semantic.EvidenceTransactionCount != uint64(len(semantic.Transactions)) ||
		semantic.EvidenceTransactionCount > uint64(semantic.EvidenceRowLimit) ||
		!domainsecurity.IsSHA256Hex(semantic.QueryHash) ||
		!domainsecurity.IsSHA256Hex(semantic.ResultHash) ||
		ValidateAccountFlowProviderOutcomeV1(semantic) != nil {
		return ErrResultInvalid
	}
	inflow, inflowOK := canonicalAccountFlowIntegerV1(semantic.InflowMinor, false)
	outflow, outflowOK := canonicalAccountFlowIntegerV1(semantic.OutflowMinor, false)
	net, netOK := canonicalAccountFlowIntegerV1(semantic.NetMinor, true)
	if !inflowOK || !outflowOK || !netOK ||
		new(big.Int).Sub(new(big.Int).Set(inflow), outflow).Cmp(net) != 0 {
		return ErrResultInvalid
	}
	wantEvidenceCount := semantic.TransactionCount
	if wantEvidenceCount > uint64(semantic.EvidenceRowLimit) {
		wantEvidenceCount = uint64(semantic.EvidenceRowLimit)
	}
	if semantic.EvidenceTransactionCount != wantEvidenceCount ||
		semantic.EvidenceRowsComplete != (semantic.TransactionCount <= uint64(semantic.EvidenceRowLimit)) {
		return ErrResultInvalid
	}

	wantCounterpartySemanticsComplete := true
	counterpartySemantics := make(map[domaincaseentity.ModelEntityAliasV1]AccountFlowProviderCounterpartyV1, len(semantic.Transactions))
	for _, row := range semantic.Transactions {
		if ValidateAccountFlowProviderCounterpartyV1(row.Counterparty) != nil {
			return ErrResultInvalid
		}
		if row.Counterparty.Status != AccountFlowCounterpartyResolvedV1 {
			wantCounterpartySemanticsComplete = false
		}
		if row.Counterparty.Alias != "" {
			if previous, exists := counterpartySemantics[row.Counterparty.Alias]; exists &&
				(previous.EntityType != row.Counterparty.EntityType ||
					previous.AccountType != row.Counterparty.AccountType) {
				return ErrResultInvalid
			}
			counterpartySemantics[row.Counterparty.Alias] = row.Counterparty
		}
	}
	if semantic.CounterpartySemanticsComplete != wantCounterpartySemanticsComplete {
		return ErrResultInvalid
	}

	coverage := semantic.Coverage
	if coverage.Gaps == nil || coverage.AcceptedSnapshotRows > coverage.NormalizedSnapshotRows ||
		coverage.RejectedSnapshotRows > coverage.NormalizedSnapshotRows-coverage.AcceptedSnapshotRows ||
		coverage.DuplicateSnapshotRows != coverage.NormalizedSnapshotRows-coverage.AcceptedSnapshotRows-coverage.RejectedSnapshotRows ||
		coverage.UntimedSubjectRows > coverage.AcceptedSnapshotRows ||
		coverage.ObservedMatchingRows != semantic.TransactionCount ||
		coverage.ObservedMatchingRows > coverage.AcceptedSnapshotRows {
		return ErrResultInvalid
	}
	wantAggregateComplete := coverage.RejectedSnapshotRows == 0 &&
		coverage.DuplicateSnapshotRows == 0 && coverage.UntimedSubjectRows == 0
	if semantic.AggregateComplete != wantAggregateComplete {
		return ErrResultInvalid
	}
	wantGaps := make([]string, 0, 5)
	if coverage.RejectedSnapshotRows != 0 {
		wantGaps = append(wantGaps, AccountFlowGapRejectedSourceRowsV1)
	}
	if coverage.DuplicateSnapshotRows != 0 {
		wantGaps = append(wantGaps, AccountFlowGapDuplicateSourceRowsV1)
	}
	if coverage.UntimedSubjectRows != 0 {
		wantGaps = append(wantGaps, AccountFlowGapUntimedSubjectRowsV1)
	}
	if !semantic.EvidenceRowsComplete {
		wantGaps = append(wantGaps, AccountFlowGapEvidenceRowLimitV1)
	}
	if !semantic.CounterpartySemanticsComplete {
		wantGaps = append(wantGaps, AccountFlowGapCounterpartyResolutionV1)
	}
	wantCoverageState := AccountFlowCoverageCompleteV1
	if !semantic.AggregateComplete || !semantic.EvidenceRowsComplete ||
		!semantic.CounterpartySemanticsComplete {
		wantCoverageState = AccountFlowCoveragePartialV1
	} else if semantic.TransactionCount == 0 {
		wantCoverageState = AccountFlowCoverageObservedNoHitPendingBindingV1
	}
	if coverage.State != wantCoverageState || !reflect.DeepEqual(coverage.Gaps, wantGaps) {
		return ErrResultInvalid
	}
	partial := !semantic.AggregateComplete || !semantic.EvidenceRowsComplete ||
		!semantic.CounterpartySemanticsComplete
	expectedStatus := domainevidence.SemanticSuccess
	if partial {
		expectedStatus = domainevidence.SemanticPartial
	}
	if status != expectedStatus ||
		(semantic.EvidenceRowsComplete && semantic.EvidenceTransactionCount != semantic.TransactionCount) {
		return ErrResultInvalid
	}

	startTime, startErr := time.Parse("2006-01-02T15:04:05.000000Z", semantic.StartInclusive)
	endTime, endErr := time.Parse("2006-01-02T15:04:05.000000Z", semantic.EndInclusive)
	if startErr != nil || endErr != nil {
		return ErrResultInvalid
	}
	previous := time.Time{}
	rowInflow := new(big.Int)
	rowOutflow := new(big.Int)
	seenEvidenceRefs := make(map[string]struct{}, len(semantic.Transactions))
	for _, row := range semantic.Transactions {
		occurredAt, err := time.Parse("2006-01-02T15:04:05.000000Z", row.OccurredAt)
		amount, amountOK := canonicalAccountFlowIntegerV1(row.AmountMinor, false)
		if !validAccountFlowSourceRecordIDV1(row.EvidenceRef) || err != nil ||
			occurredAt.Before(startTime) || occurredAt.After(endTime) ||
			(!previous.IsZero() && occurredAt.Before(previous)) || !amountOK ||
			(row.Direction != AccountFlowDirectionInflowV1 && row.Direction != AccountFlowDirectionOutflowV1) ||
			row.Currency != semantic.Currency || row.MinorUnitScale != semantic.MinorUnitScale ||
			ValidateAccountFlowProviderCounterpartyV1(row.Counterparty) != nil {
			return ErrResultInvalid
		}
		if _, duplicate := seenEvidenceRefs[row.EvidenceRef]; duplicate {
			return ErrResultInvalid
		}
		seenEvidenceRefs[row.EvidenceRef] = struct{}{}
		previous = occurredAt
		if row.Direction == AccountFlowDirectionInflowV1 {
			rowInflow.Add(rowInflow, amount)
		} else {
			rowOutflow.Add(rowOutflow, amount)
		}
	}
	if semantic.EvidenceRowsComplete &&
		(rowInflow.Cmp(inflow) != 0 || rowOutflow.Cmp(outflow) != 0) {
		return ErrResultInvalid
	}
	return nil
}

func validateAccountFlowProviderFreeTextV1(semantic AccountFlowProviderSemanticResultV1) error {
	for _, row := range semantic.Transactions {
		if row.Counterparty.Alias == "" {
			continue
		}
		for _, value := range []string{
			string(row.Counterparty.Alias),
			row.Counterparty.EntityType,
			row.Counterparty.AccountType,
		} {
			if domainsecret.ValidateValueV1(value) != nil ||
				domainprivacy.ValidateOrdinaryText(value) != nil ||
				providerSemanticLooksLikePathOrSQLV1(value) {
				return ErrResultInvalid
			}
		}
	}
	return nil
}

func providerSemanticLooksLikePathOrSQLV1(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return false
	}
	// Provider-visible labels are a closed semantic vocabulary, not arbitrary
	// source text. Reject path separators anywhere: a relative value such as
	// "private/case.duckdb/ledger" discloses a protected locator just as an
	// absolute path does.
	if strings.ContainsAny(trimmed, `/\\`) ||
		strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") ||
		strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "~/") ||
		strings.Contains(trimmed, `:\`) || strings.Contains(trimmed, "://") ||
		strings.Contains(trimmed, "/users/") || strings.Contains(trimmed, "/volumes/") ||
		strings.HasSuffix(trimmed, ".duckdb") || strings.HasSuffix(trimmed, ".db") ||
		strings.HasSuffix(trimmed, ".sqlite") || strings.HasSuffix(trimmed, ".csv") ||
		strings.HasSuffix(trimmed, ".parquet") {
		return true
	}
	for _, marker := range []string{"--", "/*", "*/", ";"} {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return false
	}
	switch fields[0] {
	case "select", "insert", "update", "delete", "drop", "alter", "create", "with", "pragma", "attach", "detach", "copy":
		return true
	default:
		return false
	}
}

func validAccountFlowProviderCurrencyV1(value string) bool {
	if len(value) != 3 {
		return false
	}
	for index := range value {
		if value[index] < 'A' || value[index] > 'Z' {
			return false
		}
	}
	return true
}

func validAccountFlowProviderTimezoneV1(value string) bool {
	if value == "Z" {
		return true
	}
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return false
	}
	if value[1] < '0' || value[1] > '9' || value[2] < '0' || value[2] > '9' ||
		value[4] < '0' || value[4] > '9' || value[5] < '0' || value[5] > '9' {
		return false
	}
	hour := int(value[1]-'0')*10 + int(value[2]-'0')
	minute := int(value[4]-'0')*10 + int(value[5]-'0')
	return minute < 60 && (hour < 14 || (hour == 14 && minute == 0))
}
