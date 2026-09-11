package jobs

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

// ClaimChildRunStart performs the durable linearization step between job
// lifecycle control and the first child-turn side effect. The claim succeeds
// only for the exact running record/lease, or its process-local successful
// pending-queue advances. Other writes and restart never recreate that chain.
func (m *Manager) ClaimChildRunStart(expected Record) (Record, error) {
	if m == nil {
		return Record{}, errors.New("child run store is unavailable")
	}
	expected.ID = strings.TrimSpace(expected.ID)
	if expected.ID == "" {
		return Record{}, errors.New("child run id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != expected.ID {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), ErrRestartPreserved
		}
		expected = m.pendingSteerStartExpectedNoLock(record, expected)
		if reason := childRunStartClaimBlocker(record, expected, time.Now().UTC()); reason != "" {
			return cloneRecord(record), errors.New(reason)
		}
		now := nextRuntimeLeaseInstant(record.LastHeartbeatAt, time.Now().UTC())
		nowText := now.Format(time.RFC3339Nano)
		record.LastHeartbeatAt = nowText
		record.LeaseExpiresAt = now.Add(defaultTaskJobLeaseTimeout).Format(time.RFC3339Nano)
		record.UpdatedAt = nowText
		if err := m.writeRecordNoLock(&record); err != nil {
			return cloneRecord(m.jobs[index]), fmt.Errorf("persist child run start claim: %w", err)
		}
		m.jobs[index] = record
		return cloneRecord(record), nil
	}
	return Record{}, os.ErrNotExist
}

// ValidateChildRunStart repeats the exact durable start-claim comparison
// without refreshing or mutating the runtime lease. Callers use it after
// acquiring a later authority writer so a job that changed while waiting
// cannot start under a different case/workspace context.
func (m *Manager) ValidateChildRunStart(expected Record) (Record, error) {
	if m == nil {
		return Record{}, errors.New("child run store is unavailable")
	}
	expected.ID = strings.TrimSpace(expected.ID)
	if expected.ID == "" {
		return Record{}, errors.New("child run id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	for index := range m.jobs {
		if m.jobs[index].ID != expected.ID {
			continue
		}
		record := m.jobs[index]
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), ErrRestartPreserved
		}
		if reason := childRunStartClaimBlocker(record, expected, time.Now().UTC()); reason != "" {
			return cloneRecord(record), errors.New(reason)
		}
		return cloneRecord(record), nil
	}
	return Record{}, os.ErrNotExist
}

func childRunStartClaimBlocker(record Record, expected Record, now time.Time) string {
	status := strings.TrimSpace(record.Status)
	if status != string(domainjob.StatusRunning) {
		return "child run start status is not running: " + firstNonEmptyString(status, "missing")
	}
	if record.SecurityBinding != nil && strings.TrimSpace(record.Kind) == "subagent" &&
		domainjob.ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil {
		return "child run delegated tool manifest is invalid"
	}
	if err := domainjob.ValidateExecutableCaseDelegationV1(
		record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
	); err != nil {
		return "child run case delegation is invalid"
	}
	if record.LateCompletionSuppressed || record.Orphaned ||
		strings.TrimSpace(record.DeadLetterReason) != "" ||
		strings.TrimSpace(record.CompletionDeliveryStatus) == "dead_letter" ||
		strings.TrimSpace(record.RecoveryStatus) == "dead_lettered" {
		return "child run is dead-lettered or no longer startable"
	}
	if strings.TrimSpace(expected.Status) != status {
		return "child run start status was replaced"
	}
	if strings.TrimSpace(record.LeaseOwner) == "" || strings.TrimSpace(record.LeaseOwner) != defaultRuntimeLeaseOwner() {
		return "child run runtime lease owner was replaced"
	}
	if strings.TrimSpace(expected.LeaseOwner) != strings.TrimSpace(record.LeaseOwner) ||
		strings.TrimSpace(expected.LeaseExpiresAt) != strings.TrimSpace(record.LeaseExpiresAt) ||
		strings.TrimSpace(expected.LastHeartbeatAt) != strings.TrimSpace(record.LastHeartbeatAt) ||
		strings.TrimSpace(expected.UpdatedAt) != strings.TrimSpace(record.UpdatedAt) {
		return "child run runtime lease was replaced"
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(record.LeaseExpiresAt))
	if err != nil || expiresAt.IsZero() || !now.Before(expiresAt) {
		return "child run runtime lease is invalid or expired"
	}
	if !sameChildRunStartIdentity(record, expected) {
		return "child run identity was replaced"
	}
	return ""
}

func sameChildRunStartIdentity(record Record, expected Record) bool {
	if strings.TrimSpace(record.ParentGoalID) != strings.TrimSpace(expected.ParentGoalID) ||
		strings.TrimSpace(record.ParentThreadID) != strings.TrimSpace(expected.ParentThreadID) ||
		strings.TrimSpace(record.ParentTurnID) != strings.TrimSpace(expected.ParentTurnID) ||
		strings.TrimSpace(record.ParentToolCallID) != strings.TrimSpace(expected.ParentToolCallID) ||
		strings.TrimSpace(record.ChildThreadID) != strings.TrimSpace(expected.ChildThreadID) ||
		strings.TrimSpace(record.ChildTurnID) != strings.TrimSpace(expected.ChildTurnID) ||
		strings.TrimSpace(record.LineageKey) != strings.TrimSpace(expected.LineageKey) ||
		strings.TrimSpace(record.ToolSchemaHash) != strings.TrimSpace(expected.ToolSchemaHash) ||
		!sameStringSet(record.ToolScope, expected.ToolScope) ||
		!sameDelegatedToolManifest(record.DelegatedToolManifest, expected.DelegatedToolManifest) ||
		!sameCaseDelegation(record.CaseDelegation, expected.CaseDelegation) {
		return false
	}
	recordBinding := ""
	if record.SecurityBinding != nil {
		recordBinding = strings.TrimSpace(record.SecurityBinding.BindingDigest)
	}
	expectedBinding := ""
	if expected.SecurityBinding != nil {
		expectedBinding = strings.TrimSpace(expected.SecurityBinding.BindingDigest)
	}
	return recordBinding == expectedBinding
}

func sameCaseDelegation(left, right *domainjob.CaseDelegationContextV1) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.DelegationDigest == right.DelegationDigest && domainjob.CaseDelegationContextsEqualV1(left, right)
}

func sameStringSet(left []string, right []string) bool {
	leftHash, leftErr := domainjob.DelegatedToolScopeHashV1(left)
	rightHash, rightErr := domainjob.DelegatedToolScopeHashV1(right)
	return leftErr == nil && rightErr == nil && leftHash == rightHash
}

func sameDelegatedToolManifest(left *domainjob.DelegatedToolManifestV1, right *domainjob.DelegatedToolManifestV1) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nextRuntimeLeaseInstant(previous string, now time.Time) time.Time {
	now = now.UTC()
	if prior, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(previous)); err == nil && !now.After(prior) {
		return prior.Add(time.Nanosecond)
	}
	return now
}
