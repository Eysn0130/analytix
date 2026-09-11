package jobs

import (
	"encoding/json"
	"reflect"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// This process-local chain remembers exact successful pending queue writes.
// It is not execution authority, a persisted receipt, or a restart token. The
// host still owns first-turn observation and the shared queue/start barrier.
// One digest per appended pending message bounds storage by the job's queue;
// no prompt, projection, or historical record body is retained here.
type pendingSteerStartChainV1 struct {
	before map[string]bool
	after  string
}

func pendingSteerStartRecordDigestV1(record Record) string {
	// Both caller records and manager records use the same derived UI state.
	body, err := json.Marshal(cloneRecord(record))
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

func (m *Manager) recordPendingSteerStartAdvanceNoLock(before, after Record, prior *pendingSteerStartChainV1) {
	if before.Kind != "subagent" || before.Status != "running" || !before.Background || before.ChildTurnID == "" ||
		domainjob.ValidateSecurityBinding(before.SecurityBinding) != nil || len(after.Steers) != len(before.Steers)+1 {
		return
	}
	left, right := cloneRecord(before), cloneRecord(after)
	// Confirm normalization introduced no change outside the exact queue write
	// set. Other lifecycle writers are never permitted to publish this chain.
	left.Steers, right.Steers = nil, nil
	left.SteerState, right.SteerState = domainjob.ChildRunState{}, domainjob.ChildRunState{}
	left.UpdatedAt, right.UpdatedAt = "", ""
	if !reflect.DeepEqual(left, right) {
		return
	}
	beforeDigest, afterDigest := pendingSteerStartRecordDigestV1(before), pendingSteerStartRecordDigestV1(after)
	if beforeDigest == "" || afterDigest == "" || beforeDigest == afterDigest {
		return
	}
	chain := &pendingSteerStartChainV1{before: map[string]bool{}, after: afterDigest}
	if prior != nil && prior.after == beforeDigest {
		for digest := range prior.before {
			chain.before[digest] = true
		}
	}
	chain.before[beforeDigest] = true
	if m.pendingSteerStarts == nil {
		m.pendingSteerStarts = make(map[string]*pendingSteerStartChainV1)
	}
	m.pendingSteerStarts[after.ID] = chain
}

func (m *Manager) pendingSteerStartExpectedNoLock(current, expected Record) Record {
	chain := m.pendingSteerStarts[current.ID]
	if chain != nil && chain.after == pendingSteerStartRecordDigestV1(current) &&
		chain.before[pendingSteerStartRecordDigestV1(expected)] {
		return cloneRecord(current)
	}
	return expected
}
