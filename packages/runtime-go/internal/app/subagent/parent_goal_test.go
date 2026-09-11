package subagent

import "testing"

func TestParentGoalIdentity(t *testing.T) {
	goalID, objective := ParentGoalIdentity(map[string]any{
		"id":        " goal_1 ",
		"objective": " Finish ",
	}, "thread_1")
	if goalID != "goal_1" || objective != "Finish" {
		t.Fatalf("goal identity mismatch: id=%q objective=%q", goalID, objective)
	}

	goalID, objective = ParentGoalIdentity(map[string]any{"objective": "Fallback"}, "thread_1")
	if goalID != "goal_thread_1" || objective != "Fallback" {
		t.Fatalf("fallback goal identity mismatch: id=%q objective=%q", goalID, objective)
	}

	goalID, objective = ParentGoalIdentity(nil, "thread_1")
	if goalID != "thread_thread_1" || objective != "" {
		t.Fatalf("thread fallback identity mismatch: id=%q objective=%q", goalID, objective)
	}
}
