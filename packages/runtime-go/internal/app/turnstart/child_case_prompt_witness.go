package turnstart

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// This process state records the frozen validation under the transition
// writer. The root still proves the original private allocation and exact
// durable turn absence before that validation; a fresh Resolve is not a
// restart execution capability. Copies share validation and issuance state.
type childTransitionFrozenWitnessStateV1 struct {
	mu           sync.Mutex
	validated    bool
	issued       bool
	frozen       domainsecurity.TurnSecurityContext
	recordDigest string
}

func (state *childTransitionFrozenWitnessStateV1) validateOnceV1(frozen domainsecurity.TurnSecurityContext, record domainjob.Record) error {
	if state == nil {
		return errors.New("child frozen validation state is unavailable")
	}
	body, err := json.Marshal(record)
	if err != nil {
		return errors.New("child frozen record is invalid")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.validated {
		return errors.New("child frozen validation was already consumed")
	}
	state.frozen, state.recordDigest, state.validated = frozen, domainsecurity.SHA256Hex(body), true
	return nil
}

func (state *childTransitionFrozenWitnessStateV1) claimV1(frozen domainsecurity.TurnSecurityContext, digest string) error {
	if state == nil {
		return errors.New("child frozen witness state is unavailable")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.validated || state.issued || state.frozen != frozen || state.recordDigest != digest {
		return errors.New("child frozen witness state changed or was consumed")
	}
	state.issued = true
	return nil
}

// HostChildCasePromptFrozenWitnessV1 is a one-use, process-local witness for
// the exact case-child prompt validated under the turn transition writer. It
// has no serializable fields, so restart, replay, fork, and compaction cannot
// reconstruct the provider-attempt authority from durable prompt text.
type HostChildCasePromptFrozenWitnessV1 struct {
	used *atomic.Bool
	use  func(func(
		string,
		domainsecurity.TurnSecurityContext,
		domainjob.Record,
		string,
		string,
		string,
		*domainjob.SecurityBinding,
		*domainjob.CaseDelegationContextV1,
	) error) error
}

// NewHostChildCasePromptFrozenWitnessV1 issues the private witness only after
// the existing frozen validator has matched the exact durable child record,
// child TSC, parent grant/call binding, and canonical delegation prompt.
func NewHostChildCasePromptFrozenWitnessV1(
	authority *ChildTransitionAuthority,
	frozen domainsecurity.TurnSecurityContext,
	expectedTurnID string,
	candidate string,
) (string, *HostChildCasePromptFrozenWitnessV1, error) {
	canonical, err := ValidateHostChildCasePromptFrozenV1(authority, frozen, expectedTurnID, candidate)
	if err != nil || authority == nil || authority.Background || authority.ExpectedRecord.CaseDelegation == nil ||
		strings.TrimSpace(authority.ExpectedRecord.Status) != string(domainjob.StatusRunning) ||
		authority.ExpectedRecord.ChildTurnID != expectedTurnID {
		return "", nil, errors.New("host child case prompt frozen witness is unavailable")
	}
	recordBytes, err := json.Marshal(authority.ExpectedRecord)
	if err != nil || len(recordBytes) == 0 {
		return "", nil, errors.New("host child case prompt durable authority is invalid")
	}
	recordDigest := domainsecurity.SHA256Hex(recordBytes)
	if !domainsecurity.IsSHA256Hex(recordDigest) {
		return "", nil, errors.New("host child case prompt durable authority is invalid")
	}
	childRunID := authority.ChildRunID
	childThreadID := authority.ChildThreadID
	binding := domainjob.CloneSecurityBinding(authority.ExpectedRecord.SecurityBinding)
	delegation := domainjob.CloneCaseDelegationContextV1(authority.ExpectedRecord.CaseDelegation)
	var expectedRecord domainjob.Record
	if json.Unmarshal(recordBytes, &expectedRecord) != nil {
		return "", nil, errors.New("host child case prompt durable authority is invalid")
	}
	if err := authority.frozenWitness.claimV1(frozen, recordDigest); err != nil {
		return "", nil, err
	}
	witness := &HostChildCasePromptFrozenWitnessV1{used: &atomic.Bool{}}
	witness.use = func(use func(
		string,
		domainsecurity.TurnSecurityContext,
		domainjob.Record,
		string,
		string,
		string,
		*domainjob.SecurityBinding,
		*domainjob.CaseDelegationContextV1,
	) error) error {
		if use == nil {
			return errors.New("host child case prompt witness consumer is unavailable")
		}
		return use(
			canonical,
			frozen,
			expectedRecord,
			recordDigest,
			childRunID,
			childThreadID,
			domainjob.CloneSecurityBinding(binding),
			domainjob.CloneCaseDelegationContextV1(delegation),
		)
	}
	return canonical, witness, nil
}

// UseCurrentAttemptV1 burns the witness before invoking the consumer. A
// malformed or cross-scope attempt cannot retry the same private authority.
func (witness *HostChildCasePromptFrozenWitnessV1) UseCurrentAttemptV1(use func(
	string,
	domainsecurity.TurnSecurityContext,
	domainjob.Record,
	string,
	string,
	string,
	*domainjob.SecurityBinding,
	*domainjob.CaseDelegationContextV1,
) error) error {
	if witness == nil || witness.use == nil || witness.used == nil || use == nil || !witness.used.CompareAndSwap(false, true) {
		return errors.New("host child case prompt witness is unavailable or already consumed")
	}
	return witness.use(use)
}
