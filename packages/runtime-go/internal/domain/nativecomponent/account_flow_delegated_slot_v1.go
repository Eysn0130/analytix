package nativecomponent

import (
	"math/big"
	"reflect"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const MaxAccountFlowDelegatedGapsV1 = 5

// AccountFlowDelegatedAnswerSlotV1 is the closed, row-free projection that a
// foreground case child may select. It carries typed aggregate candidates and
// value-free source-field lineage, but no transaction row, evidence locator,
// AuthorityEntityRef, path, provider prose, or factual authority.
type AccountFlowDelegatedAnswerSlotV1 struct {
	SubjectAlias                  string                       `json:"subjectAlias"`
	StartInclusive                string                       `json:"startInclusive"`
	EndInclusive                  string                       `json:"endInclusive"`
	Timezone                      string                       `json:"timezone"`
	Currency                      string                       `json:"currency"`
	MinorUnitScale                uint8                        `json:"minorUnitScale"`
	InflowMinor                   string                       `json:"inflowMinor"`
	OutflowMinor                  string                       `json:"outflowMinor"`
	NetMinor                      string                       `json:"netMinor"`
	TransactionCount              uint64                       `json:"transactionCount"`
	AggregateComplete             bool                         `json:"aggregateComplete"`
	EvidenceRowsComplete          bool                         `json:"evidenceRowsComplete"`
	CounterpartySemanticsComplete bool                         `json:"counterpartySemanticsComplete"`
	Gaps                          []string                     `json:"gaps"`
	Currentness                   string                       `json:"currentness"`
	QueryScopeRef                 string                       `json:"queryScopeRef"`
	QueryHash                     string                       `json:"queryHash"`
	ResultHash                    string                       `json:"resultHash"`
	Outcome                       AccountFlowProviderOutcomeV1 `json:"outcome"`
}

func NewAccountFlowDelegatedAnswerSlotV1(
	contextDigest string,
	output AccountFlowProviderModelOutputV1,
) (AccountFlowDelegatedAnswerSlotV1, error) {
	if _, err := CanonicalAccountFlowProviderModelOutputV1(output); err != nil {
		return AccountFlowDelegatedAnswerSlotV1{}, ErrResultInvalid
	}
	queryScopeRef, err := domainevidence.NewAccountFlowQueryScopeRefV1(contextDigest, output.Data.QueryHash)
	if err != nil {
		return AccountFlowDelegatedAnswerSlotV1{}, ErrResultInvalid
	}
	data := output.Data
	slot := AccountFlowDelegatedAnswerSlotV1{
		SubjectAlias: data.SubjectAlias, StartInclusive: data.StartInclusive, EndInclusive: data.EndInclusive,
		Timezone: data.Timezone, Currency: data.Currency, MinorUnitScale: data.MinorUnitScale,
		InflowMinor: data.InflowMinor, OutflowMinor: data.OutflowMinor, NetMinor: data.NetMinor,
		TransactionCount: data.TransactionCount, AggregateComplete: data.AggregateComplete,
		EvidenceRowsComplete:          data.EvidenceRowsComplete,
		CounterpartySemanticsComplete: data.CounterpartySemanticsComplete,
		Gaps:                          append([]string{}, data.Coverage.Gaps...), Currentness: data.Currentness,
		QueryScopeRef: queryScopeRef, QueryHash: data.QueryHash, ResultHash: data.ResultHash,
		Outcome: data.Outcome,
	}
	if err := ValidateAccountFlowDelegatedAnswerSlotV1(slot); err != nil {
		return AccountFlowDelegatedAnswerSlotV1{}, err
	}
	return slot, nil
}

func ValidateAccountFlowDelegatedAnswerSlotV1(slot AccountFlowDelegatedAnswerSlotV1) error {
	start, startOK := canonicalAccountFlowTimestampV1(slot.StartInclusive)
	end, endOK := canonicalAccountFlowTimestampV1(slot.EndInclusive)
	inflow, inflowOK := canonicalAccountFlowIntegerV1(slot.InflowMinor, false)
	outflow, outflowOK := canonicalAccountFlowIntegerV1(slot.OutflowMinor, false)
	net, netOK := canonicalAccountFlowIntegerV1(slot.NetMinor, true)
	if !startOK || !endOK || start != slot.StartInclusive || end != slot.EndInclusive || start > end ||
		!validAccountFlowProviderTimezoneV1(slot.Timezone) || !validAccountFlowProviderCurrencyV1(slot.Currency) ||
		slot.MinorUnitScale != AccountFlowMinorUnitScaleV1 || slot.TransactionCount > uint64(AccountFlowMaximumScanRowsV1) ||
		!inflowOK || !outflowOK || !netOK || new(big.Int).Sub(new(big.Int).Set(inflow), outflow).Cmp(net) != 0 ||
		slot.Currentness != AccountFlowProviderCurrentnessCurrentV1 ||
		slot.Gaps == nil || !domainsecurity.IsSHA256Hex(slot.QueryHash) || !domainsecurity.IsSHA256Hex(slot.ResultHash) ||
		domainevidence.ValidateAccountFlowQueryScopeRefV1(slot.QueryScopeRef) != nil {
		return ErrResultInvalid
	}
	wantOutcome, err := NewAccountFlowProviderOutcomeV1(
		slot.SubjectAlias, slot.AggregateComplete, slot.EvidenceRowsComplete, slot.QueryHash, slot.ResultHash,
	)
	if err != nil || slot.Outcome != wantOutcome ||
		!reflect.DeepEqual(slot.Gaps, canonicalDelegatedAccountFlowGapsV1(slot.Gaps)) {
		return ErrResultInvalid
	}
	wantGaps := make([]string, 0, 5)
	if !slot.AggregateComplete {
		// The exact row-quality causes remain closed in Gaps. At least one must
		// explain an incomplete aggregate.
		if !containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapRejectedSourceRowsV1) &&
			!containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapDuplicateSourceRowsV1) &&
			!containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapUntimedSubjectRowsV1) {
			return ErrResultInvalid
		}
	} else if containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapRejectedSourceRowsV1) ||
		containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapDuplicateSourceRowsV1) ||
		containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapUntimedSubjectRowsV1) {
		return ErrResultInvalid
	}
	if !slot.EvidenceRowsComplete {
		wantGaps = append(wantGaps, AccountFlowGapEvidenceRowLimitV1)
	}
	if !slot.CounterpartySemanticsComplete {
		wantGaps = append(wantGaps, AccountFlowGapCounterpartyResolutionV1)
	}
	for _, gap := range wantGaps {
		if !containsDelegatedAccountFlowGapV1(slot.Gaps, gap) {
			return ErrResultInvalid
		}
	}
	if slot.EvidenceRowsComplete && containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapEvidenceRowLimitV1) ||
		slot.CounterpartySemanticsComplete && containsDelegatedAccountFlowGapV1(slot.Gaps, AccountFlowGapCounterpartyResolutionV1) {
		return ErrResultInvalid
	}
	return nil
}

func canonicalDelegatedAccountFlowGapsV1(values []string) []string {
	if values == nil {
		return nil
	}
	rank := map[string]int{
		AccountFlowGapRejectedSourceRowsV1: 1, AccountFlowGapDuplicateSourceRowsV1: 2,
		AccountFlowGapUntimedSubjectRowsV1: 3, AccountFlowGapEvidenceRowLimitV1: 4,
		AccountFlowGapCounterpartyResolutionV1: 5,
	}
	out := make([]string, len(values))
	copy(out, values)
	previous := 0
	for _, value := range out {
		current := rank[value]
		if current == 0 || current <= previous {
			return nil
		}
		previous = current
	}
	return out
}

func containsDelegatedAccountFlowGapV1(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
