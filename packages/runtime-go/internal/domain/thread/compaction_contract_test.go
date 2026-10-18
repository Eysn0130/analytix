package thread

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompactionSummaryFactsSealCompleteContentAndSelection(t *testing.T) {
	facts, err := SealCompactionSummaryFactsV1(CompactionSummaryFactsV1{CompactionID: "compaction_1", Summary: "Continue the accepted task.",
		SourceDigest: strings.Repeat("a", 64), SourceItemIDs: []string{"source_1"}, ShadowedItemCount: 2, ReplacedTokens: 321, FinishedAt: "2026-10-10T00:00:02Z"})
	if err != nil || ValidateCompactionSummaryFactsV1(facts) != nil {
		t.Fatalf("complete facts rejected: %v", err)
	}
	for _, mutate := range []func(*CompactionSummaryFactsV1){
		func(value *CompactionSummaryFactsV1) { value.Summary += " fabricated" },
		func(value *CompactionSummaryFactsV1) { value.SourceDigest = strings.Repeat("b", 64) },
		func(value *CompactionSummaryFactsV1) { value.ShadowedItemCount++ },
		func(value *CompactionSummaryFactsV1) { value.ReplacedTokens++ },
		func(value *CompactionSummaryFactsV1) { value.FinishedAt = "2026-10-10T00:00:03Z" },
	} {
		changed := facts
		mutate(&changed)
		if ValidateCompactionSummaryFactsV1(changed) == nil {
			t.Fatal("changed summary facts passed old checkpoint")
		}
	}
	for _, summary := range []string{"", "  ", strings.Repeat("x", 16*1024+1), string([]byte{0xff})} {
		changed := facts
		changed.Summary = summary
		if _, err := SealCompactionSummaryFactsV1(changed); err == nil {
			t.Fatal("empty, oversized, or invalid summary sealed")
		}
	}
}

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

func TestCompactionHistoryDerivationBindsDataWithoutExecutionFields(t *testing.T) {
	proof, err := SealCompactionHistoryDerivationV1(CompactionHistoryDerivationV1{Derivation: "fork", SourceThreadID: "source", SourceTurnID: "turn_source", SourceItemID: "compaction_source",
		SourceScopeDigest: strings.Repeat("a", 64), SourceItemDigest: strings.Repeat("b", 64), SourceSummaryDigest: strings.Repeat("c", 64), SourceCheckpointDigest: strings.Repeat("d", 64),
		ThreadID: "child", TurnID: "turn_source", ItemID: "compaction_source", ScopeDigest: strings.Repeat("e", 64), ItemDigest: strings.Repeat("f", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCompactionHistoryDerivationV1(proof); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"version", "scopeDigest", "itemDigest", "sourceCheckpointDigest", "securityContext", "executionGrant", "contextEpochSnapshot"} {
		t.Run(field, func(t *testing.T) {
			body, _ := json.Marshal(proof)
			var value map[string]any
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			value[field] = strings.Repeat("0", 64)
			if _, err := ParseCompactionHistoryDerivationV1(value); err == nil {
				t.Fatal("changed data or an execution authority field entered the closed derivation")
			}
		})
	}
	proof.Derivation = "fork-prefix"
	if _, err := SealCompactionHistoryDerivationV1(proof); err == nil {
		t.Fatal("a source prefix changed the data owner")
	}
	proof.ThreadID, proof.ScopeDigest, proof.ItemDigest = proof.SourceThreadID, proof.SourceScopeDigest, proof.SourceItemDigest
	if _, err := SealCompactionHistoryDerivationV1(proof); err != nil {
		t.Fatal("an exact verified source prefix was rejected", err)
	}
}
