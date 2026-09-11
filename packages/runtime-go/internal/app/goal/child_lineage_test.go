package goal

import (
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestParentIdentity(t *testing.T) {
	id, objective := ParentIdentity(map[string]any{"id": " goal_1 ", "objective": " finish it "}, "thread_1")
	if id != "goal_1" || objective != "finish it" {
		t.Fatalf("parent identity mismatch: id=%q objective=%q", id, objective)
	}
	id, objective = ParentIdentity(map[string]any{"objective": "fallback"}, "thread_1")
	if id != "goal_thread_1" || objective != "fallback" {
		t.Fatalf("fallback parent identity mismatch: id=%q objective=%q", id, objective)
	}
}

func TestBuildChildRunPipelineStageEvent(t *testing.T) {
	event := BuildChildRunPipelineStageEvent(ChildRunPipelineStageEventInput{
		ThreadID:            " thread_1 ",
		TurnID:              " turn_1 ",
		ParentGoalObjective: " objective ",
		ChildSeq:            7,
		TotalTokens:         123,
		CacheHitRate:        0.75,
		Record: domainjob.Record{
			ID:                    "job_1",
			ParentGoalID:          "goal_1",
			Kind:                  "child-run",
			Status:                "completed",
			LineageKey:            "lineage",
			Model:                 "model-a",
			Effort:                "high",
			ProfileSource:         "parent-default",
			DefaultModelInherited: true,
			ArtifactPath:          "artifact.json",
		},
	})
	if event["kind"] != "pipeline_stage" || event["threadId"] != "thread_1" || event["turnId"] != "turn_1" || event["stage"] != "response_received" {
		t.Fatalf("event identity mismatch: %#v", event)
	}
	child, _ := event["child"].(map[string]any)
	if child["parentGoalId"] != "goal_1" ||
		child["parentGoalObjective"] != "objective" ||
		child["childId"] != "job_1" ||
		child["childSeq"] != 7 ||
		child["totalTokens"] != 123 ||
		child["cacheHitRate"] != 0.75 ||
		child["topLevelSubagentRouteExposed"] != false {
		t.Fatalf("child payload mismatch: %#v", child)
	}
}
