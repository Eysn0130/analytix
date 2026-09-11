package thread

import (
	"strings"
	"testing"
)

func TestTaskContinuationSnapshotV1RejectsTamperingAndEvidenceUpgrade(t *testing.T) {
	snapshot, err := SealTaskContinuationSnapshotV1(TaskContinuationSnapshotV1{
		Goal:                  &TaskContinuationGoalV1{GoalID: "goal_1", Objective: "finish safely", Status: "active", StateDigest: testContinuationDigestV1("goal")},
		Todos:                 []TaskContinuationTodoV1{{TodoID: "todo_1", Content: "run tests", Status: "failed", StatusReasonCode: "verification_failed", StateDigest: testContinuationDigestV1("todo")}},
		LatestUserConstraints: []string{"do not publish unsupported facts"},
		EvidenceReferences:    []TaskContinuationEvidenceReferenceV1{{ReferenceDigest: testContinuationDigestV1("evidence"), SupportStatus: TaskContinuationEvidenceStateV1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := TaskContinuationSnapshotMapV1(snapshot)
	if _, err := ParseTaskContinuationSnapshotV1(encoded); err != nil {
		t.Fatalf("sealed continuation rejected: %v", err)
	}
	encoded["evidenceAuthority"] = "verified"
	if _, err := ParseTaskContinuationSnapshotV1(encoded); err == nil {
		t.Fatal("evidence authority upgrade was accepted")
	}
	encoded = TaskContinuationSnapshotMapV1(snapshot)
	encoded["latestUserConstraints"].([]any)[0] = "changed"
	if _, err := ParseTaskContinuationSnapshotV1(encoded); err == nil {
		t.Fatal("digest-mismatched continuation was accepted")
	}
}

func testContinuationDigestV1(value string) string {
	return strings.Repeat(map[string]string{"goal": "a", "todo": "b", "evidence": "c"}[value], 64)
}
