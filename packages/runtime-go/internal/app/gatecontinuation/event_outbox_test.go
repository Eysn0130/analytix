package gatecontinuation

import (
	"errors"
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type eventOutboxFixture struct {
	thread      map[string]any
	events      []map[string]any
	recordCalls int
	ackLoss     bool
}

func (fixture *eventOutboxFixture) dependencies() EventOutboxDependencies {
	return EventOutboxDependencies{
		GetThread: func(threadID string) (map[string]any, error) {
			if threadID != contracts.StringField(fixture.thread, "id") {
				return nil, errors.New("thread not found")
			}
			return contracts.CloneMap(fixture.thread), nil
		},
		LoadEvents: func(threadID string) ([]map[string]any, error) {
			if threadID != contracts.StringField(fixture.thread, "id") {
				return nil, errors.New("thread not found")
			}
			return append([]map[string]any(nil), fixture.events...), nil
		},
		RecordEvent: func(event map[string]any) error {
			fixture.recordCalls++
			fixture.events = append(fixture.events, contracts.CloneMap(event))
			if fixture.ackLoss {
				return errors.New("append acknowledgement lost")
			}
			return nil
		},
	}
}

func TestPendingGateRequestOutboxRecoversAckLossAndRejectsConflicts(t *testing.T) {
	fixture := &eventOutboxFixture{thread: map[string]any{"id": "thread-a"}, ackLoss: true}
	event := map[string]any{
		"kind": "approval_requested", "threadId": "thread-a", "turnId": "turn-a",
		"itemId": "item-a", "approvalId": "approval-a", "status": "pending",
		"toolName": "write_file", "continuationReceiptId": "receipt-a",
	}
	if err := RecordPendingGateRequestV1(event, fixture.dependencies()); err != nil {
		t.Fatalf("committed append with lost acknowledgement was not recovered: %v", err)
	}
	if err := RecordPendingGateRequestV1(event, fixture.dependencies()); err != nil {
		t.Fatalf("exact retry was not idempotent: %v", err)
	}
	if fixture.recordCalls != 1 || len(fixture.events) != 1 {
		t.Fatalf("exact retry duplicated request: calls=%d events=%d", fixture.recordCalls, len(fixture.events))
	}
	conflict := contracts.CloneMap(event)
	conflict["itemId"] = "item-other"
	if err := RecordPendingGateRequestV1(conflict, fixture.dependencies()); err == nil {
		t.Fatal("same gate accepted a conflicting durable request")
	}
	invalid := contracts.CloneMap(event)
	delete(invalid, "approvalId")
	if err := RecordPendingGateRequestV1(invalid, fixture.dependencies()); err == nil {
		t.Fatal("request without an approval identity was accepted")
	}
}

func TestPendingGateResolutionAndGrantTransitionAreExact(t *testing.T) {
	t.Run("resolution", func(t *testing.T) {
		fixture := &eventOutboxFixture{thread: map[string]any{"id": "thread-a"}}
		event := map[string]any{
			"kind": "approval_resolved", "threadId": "thread-a", "turnId": "turn-a",
			"itemId": "item-a", "approvalId": "approval-a", "status": "allowed",
		}
		if err := RecordPendingGateResolutionV1(event, fixture.dependencies()); err != nil {
			t.Fatal(err)
		}
		conflict := contracts.CloneMap(event)
		conflict["status"] = "denied"
		if err := RecordPendingGateResolutionV1(conflict, fixture.dependencies()); err == nil {
			t.Fatal("same gate accepted a conflicting durable resolution")
		}
	})

	t.Run("grant_transition", func(t *testing.T) {
		fixture := &eventOutboxFixture{thread: map[string]any{"id": "thread-a"}}
		event := map[string]any{
			"kind": "execution_grant_approved", "threadId": "thread-a", "turnId": "turn-a",
			"itemId": "item-a", "approvalId": "approval-a",
			"approvalTransitionId": domainsecurity.SHA256Hex([]byte("transition-a")),
		}
		if err := RecordApprovalGrantTransitionV1(event, fixture.dependencies()); err != nil {
			t.Fatal(err)
		}
		conflict := contracts.CloneMap(event)
		conflict["approvalTransitionId"] = domainsecurity.SHA256Hex([]byte("transition-b"))
		if err := RecordApprovalGrantTransitionV1(conflict, fixture.dependencies()); err == nil {
			t.Fatal("same approval accepted a conflicting grant transition")
		}
		invalid := contracts.CloneMap(event)
		invalid["approvalTransitionId"] = "not-a-digest"
		if err := RecordApprovalGrantTransitionV1(invalid, fixture.dependencies()); err == nil {
			t.Fatal("grant transition without a canonical digest was accepted")
		}
	})
}

func TestExactGateEventProjectionIgnoresOnlyStorageMetadata(t *testing.T) {
	expected := map[string]any{"kind": "approval_requested", "threadId": "thread-a", "turnId": "turn-a", "approvalId": "approval-a"}
	existing := contracts.CloneMap(expected)
	existing["seq"] = 17
	existing["timestamp"] = "2026-07-18T00:00:00Z"
	if !ExactGateEventProjectionV1(existing, expected) {
		t.Fatal("storage-assigned sequence metadata changed the exact event projection")
	}
	existing["itemId"] = "item-forged"
	if ExactGateEventProjectionV1(existing, expected) {
		t.Fatal("non-storage event data was ignored by exact projection")
	}
}
