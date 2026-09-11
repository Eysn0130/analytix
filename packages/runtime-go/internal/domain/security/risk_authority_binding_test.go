package security

import (
	"bytes"
	"encoding/json"
	"testing"
)

type riskAuthorityBindingTestFixture struct {
	head        monotonicHeadTestFixture
	binding     RiskAuthorityBindingV1
	index       ThreadRiskAuthorityIndexV1
	request     MonotonicHeadObserveRequestV1
	observation MonotonicHeadObservationV1
}

func newRiskAuthorityBindingTestFixture(t *testing.T, threadID, workspace, riskClass, policyDigest string) riskAuthorityBindingTestFixture {
	t.Helper()
	fixture := newMonotonicHeadTestFixture(t)
	mutationID := SHA256Hex([]byte("risk-binding-mutation:" + threadID))
	entry := testThreadRiskAuthorityEntry(threadID, workspace, riskClass, policyDigest, "")
	index := newTestThreadRiskAuthorityIndex(t, fixture.authority, 1, "", mutationID, []ThreadRiskAuthorityEntryV1{entry})
	checkpoint := newTestMonotonicCheckpoint(
		t, fixture, index.Generation, index.IndexDigest, SHA256Hex([]byte("risk-binding-previous-state:"+threadID)),
		SHA256Hex([]byte("risk-binding-previous-checkpoint:"+threadID)), SHA256Hex([]byte("risk-binding-fence:"+threadID)), mutationID,
	)
	request := newTestMonotonicObserveRequest(
		t, fixture, ThreadRiskAuthorityNamespaceV1, SHA256Hex([]byte("risk-binding-fresh-challenge:"+threadID)),
	)
	observation, err := NewMonotonicHeadObservationV1(request, checkpoint, fixture.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewWitnessedRiskAuthorityBindingV1(index, request, observation)
	if err != nil {
		t.Fatal(err)
	}
	return riskAuthorityBindingTestFixture{head: fixture, binding: binding, index: index, request: request, observation: observation}
}

func mustTestWitnessedRiskAuthorityBindingV1(t *testing.T, threadID, workspace, riskClass, policyDigest string) RiskAuthorityBindingV1 {
	t.Helper()
	return newRiskAuthorityBindingTestFixture(t, threadID, workspace, riskClass, policyDigest).binding
}

func TestRiskAuthorityBindingV1RequiresExactFreshWitnessAuthority(t *testing.T) {
	fixture := newRiskAuthorityBindingTestFixture(
		t, "thread-risk-binding", "/workspace/risk-binding", RiskClassCase, SHA256Hex([]byte("risk-policy")),
	)
	if err := ValidateWitnessedRiskAuthorityBindingV1(fixture.binding, fixture.index, fixture.request, fixture.observation); err != nil {
		t.Fatal(err)
	}

	fakeDigest := fixture.binding
	fakeDigest.IndexDigest = SHA256Hex([]byte("fake-index"))
	if ValidateRiskAuthorityBindingV1(fakeDigest) != nil {
		t.Fatal("a syntactically valid digest fixture should demonstrate that non-empty digests are not authority")
	}
	if err := ValidateWitnessedRiskAuthorityBindingV1(fakeDigest, fixture.index, fixture.request, fixture.observation); err == nil {
		t.Fatal("fake index digest matched exact witnessed authority")
	}

	freshRequest := newTestMonotonicObserveRequest(
		t, fixture.head, ThreadRiskAuthorityNamespaceV1, SHA256Hex([]byte("next-fresh-request")),
	)
	// The old observation remains internally signed, but cannot answer the next
	// current-run challenge issued by the same installation authority.
	if _, err := NewWitnessedRiskAuthorityBindingV1(fixture.index, freshRequest, fixture.observation); err == nil {
		t.Fatal("stale observation replay produced witnessed authority")
	}

	wrongObservation := fixture.observation
	wrongObservation.ObservationDigest = SHA256Hex([]byte("forged-observation"))
	if _, err := NewWitnessedRiskAuthorityBindingV1(fixture.index, fixture.request, wrongObservation); err == nil {
		t.Fatal("forged observation digest produced witnessed authority")
	}
	wrongCheckpoint := newTestMonotonicCheckpoint(
		t, fixture.head, fixture.index.Generation, SHA256Hex([]byte("other-index")),
		SHA256Hex([]byte("other-previous-state")), SHA256Hex([]byte("other-previous-checkpoint")),
		SHA256Hex([]byte("other-fence")), fixture.index.MutationID,
	)
	validButMismatchedObservation, err := NewMonotonicHeadObservationV1(fixture.request, wrongCheckpoint, fixture.head.witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWitnessedRiskAuthorityBindingV1(fixture.index, fixture.request, validButMismatchedObservation); err == nil {
		t.Fatal("valid observation for a different checkpoint produced witnessed authority")
	}
}

func TestRiskAuthorityBindingV1ClosedStateJSON(t *testing.T) {
	fixture := newRiskAuthorityBindingTestFixture(
		t, "thread-risk-json", "/workspace/risk-json", RiskClassGeneral, SHA256Hex([]byte("risk-policy-json")),
	)
	body, err := json.Marshal(fixture.binding)
	if err != nil {
		t.Fatal(err)
	}
	var parsed RiskAuthorityBindingV1
	if err := json.Unmarshal(body, &parsed); err != nil || parsed != fixture.binding {
		t.Fatalf("witnessed binding round trip failed: parsed=%#v err=%v", parsed, err)
	}
	unknown := bytes.Replace(body, []byte(`"state":"witnessed"`), []byte(`"state":"witnessed","safeToAnswer":true`), 1)
	if err := json.Unmarshal(unknown, &parsed); err == nil {
		t.Fatal("unknown witnessed binding property was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"generation":1`), []byte(`"generation":1,"generation":2`), 1)
	if err := json.Unmarshal(duplicate, &parsed); err == nil {
		t.Fatal("duplicate witnessed binding property was accepted")
	}
	if err := json.Unmarshal(append(body, []byte(` {}`)...), &parsed); err == nil {
		t.Fatal("trailing witnessed binding JSON was accepted")
	}

	quarantined := NewQuarantinedRiskAuthorityBindingV1()
	quarantinedBody, err := json.Marshal(quarantined)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(quarantinedBody, []byte("indexDigest")) || bytes.Contains(quarantinedBody, []byte("generation")) ||
		bytes.Contains(quarantinedBody, []byte("checkpointDigest")) || bytes.Contains(quarantinedBody, []byte("observationDigest")) {
		t.Fatalf("quarantined binding serialized authority references: %s", quarantinedBody)
	}
	withAuthority := bytes.Replace(
		quarantinedBody, []byte(`"state":"quarantined"`),
		[]byte(`"state":"quarantined","indexDigest":"`+SHA256Hex([]byte("forged"))+`"`), 1,
	)
	if err := json.Unmarshal(withAuthority, &parsed); err == nil {
		t.Fatal("quarantined binding accepted authority references")
	}
}
