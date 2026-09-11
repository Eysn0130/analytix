package evidence

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAccountFlowTypedOutcomeBindsGroupQueryScopeToClaimScopeOnNewAndParse(t *testing.T) {
	securityContext := evidenceReceiptTestContext(
		t, "thread-final-answer-flow", "turn-final-answer-flow", "case-final-answer-flow", "snapshot-final-answer-flow", 3,
	)
	const subject = "entity-final-answer-flow"
	scope := rendererAccountFlowScope(subject, "final-answer-flow-filter")
	scope.SourceIDs = []string{"qscope1_" + domainsecurity.SHA256Hex([]byte("final-answer-flow-scope"))}
	claims := rendererAccountFlowClaims(t, subject, scope, scope)
	claimIDs := make([]string, len(claims))
	for index, claim := range claims {
		claimIDs[index] = claim.ClaimID
	}
	sort.Strings(claimIDs)
	queryHash := domainsecurity.SHA256Hex([]byte("final-answer-flow-query"))
	resultHash := domainsecurity.SHA256Hex([]byte("final-answer-flow-result"))
	sourceField, err := NewAccountFlowTypedSourceFieldReferenceV1(
		queryHash, resultHash, AcceptedSlotSourceFieldAccountV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongScopeRef := "qscope1_" + domainsecurity.SHA256Hex([]byte("final-answer-flow-other-scope"))
	newOutcome := func(queryScopeRef string) AccountFlowTypedAnswerOutcomeV1 {
		t.Helper()
		outcome, outcomeErr := NewAccountFlowTypedAnswerOutcomeV1([]AccountFlowTypedAnswerGroupV1{{
			EvidenceReceiptID: "receipt-renderer-flow", ClaimIDs: claimIDs, QueryScopeRef: queryScopeRef,
			QueryHash: queryHash, ResultHash: resultHash, SourceFieldReference: sourceField,
		}})
		if outcomeErr != nil {
			t.Fatal(outcomeErr)
		}
		return outcome
	}
	newEnvelope := func(outcome AccountFlowTypedAnswerOutcomeV1) (FinalAnswerEnvelope, error) {
		return NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
			Variant: EvidenceBackedAnswer, Context: securityContext, TerminalReason: "success",
			AccountFlowOutcome: &outcome, Claims: claims, EvidenceReceiptIDs: []string{"receipt-renderer-flow"},
			CheckedScope: &scope, IssuedAt: time.Unix(1_750_000_100, 0).UTC(),
		})
	}

	if envelope, err := newEnvelope(newOutcome(wrongScopeRef)); err == nil || envelope.EnvelopeDigest != "" {
		t.Fatal("new final answer admitted a valid-format group query scope outside the claims")
	}

	valid, err := newEnvelope(newOutcome(scope.SourceIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	tampered := valid
	tampered.AccountFlowOutcome = cloneAccountFlowTypedAnswerOutcomeV1(valid.AccountFlowOutcome)
	tampered.AccountFlowOutcome.Groups[0].QueryScopeRef = wrongScopeRef
	tampered.EnvelopeDigest = finalAnswerEnvelopeDigest(tampered)
	raw, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseFinalAnswerEnvelope(raw); err == nil || parsed.EnvelopeDigest != "" {
		t.Fatal("parsed final answer admitted a rehashed valid-format group query scope outside the claims")
	}
}
