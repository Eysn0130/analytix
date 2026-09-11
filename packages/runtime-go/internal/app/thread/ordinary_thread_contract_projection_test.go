package thread

import (
	"encoding/json"
	"strings"
	"testing"
)

type singleThreadProjectionRepository struct {
	*repositoryStub
}

func (r *singleThreadProjectionRepository) ListThreads(bool, bool, bool, string) ([]map[string]any, error) {
	return []map[string]any{{"id": r.thread["id"]}}, nil
}

func TestOrdinaryThreadProjectionDropsPrivateAndUnknownAuthorityFields(t *testing.T) {
	thread, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	thread["contextEpochState"] = map[string]any{"state": "internal"}
	thread["endpointFormat"] = "chat_completions"
	thread["reasoningEffort"] = "high"
	thread["unknownSafeRoot"] = "must-not-cross"

	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["contextEpochSnapshot"] = map[string]any{"state": "internal"}
	turn["approvalPolicy"] = "auto"
	turn["sandboxMode"] = "danger-full-access"
	turn["unknownSafeTurn"] = "must-not-cross"

	projected, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"securityState", "contextEpochState", "endpointFormat", "reasoningEffort", "unknownSafeRoot",
	} {
		if _, present := projected[field]; present {
			t.Fatalf("private or unknown root field %q crossed the ordinary thread contract", field)
		}
	}
	projectedTurns, _ := projected["turns"].([]any)
	if len(projectedTurns) != 1 {
		t.Fatalf("projected turn count = %d, want 1", len(projectedTurns))
	}
	projectedTurn, _ := projectedTurns[0].(map[string]any)
	for _, field := range []string{
		"securityContext", "contextEpochSnapshot", "approvalPolicy", "sandboxMode", "unknownSafeTurn",
	} {
		if _, present := projectedTurn[field]; present {
			t.Fatalf("private or unknown turn field %q crossed the ordinary thread contract", field)
		}
	}
	if projected["id"] != thread["id"] || projectedTurn["id"] != turn["id"] || projectedTurn["items"] == nil {
		t.Fatalf("canonical public thread fields were not preserved: root=%#v turn=%#v", projected, projectedTurn)
	}
}

func TestOrdinaryThreadProjectionClosesGoalEvidenceMetadata(t *testing.T) {
	const privateSentinel = "PRIVATE_GOAL_EVIDENCE_DETAILS_SENTINEL"
	thread, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	threadID, _ := thread["id"].(string)
	timestamp := "2026-08-04T00:00:00Z"
	thread["goal"] = map[string]any{
		"id": "goal_1", "threadId": threadID, "objective": "verify the public goal projection",
		"status": "active", "tokensUsed": float64(1), "timeUsedSeconds": float64(2),
		"createdAt": timestamp, "updatedAt": timestamp, "unknownGoalField": privateSentinel,
		"research": map[string]any{
			"enabled": true, "stateRefDigest": strings.Repeat("a", 64), "requirementCount": float64(1),
		},
		"evidenceLedger": []any{map[string]any{
			"id": "goal_ev_1", "turnId": "turn-general", "step": "Run the focused test",
			"evidence": []any{"focused test passed"}, "createdAt": timestamp,
			"evidenceDetails":      []any{map[string]any{"hostVerified": true, "private": privateSentinel}},
			"unknownEvidenceField": privateSentinel,
		}},
	}

	projected, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	goal, _ := projected["goal"].(map[string]any)
	if goal == nil || goal["id"] != "goal_1" || goal["threadId"] != threadID {
		t.Fatalf("public goal identity was not preserved: %#v", goal)
	}
	if _, present := goal["unknownGoalField"]; present {
		t.Fatalf("unknown goal metadata crossed the public projection: %#v", goal)
	}
	if _, present := goal["research"]; present {
		t.Fatalf("private research state crossed the public projection: %#v", goal)
	}
	ledger, _ := goal["evidenceLedger"].([]any)
	if len(ledger) != 1 {
		t.Fatalf("public evidence ledger length = %d, want 1", len(ledger))
	}
	entry, _ := ledger[0].(map[string]any)
	if entry == nil || entry["id"] != "goal_ev_1" || entry["step"] != "Run the focused test" {
		t.Fatalf("accepted goal evidence was not preserved: %#v", entry)
	}
	for _, field := range []string{"evidenceDetails", "unknownEvidenceField"} {
		if _, present := entry[field]; present {
			t.Fatalf("private goal evidence field %q crossed the public projection: %#v", field, entry)
		}
	}
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), privateSentinel) {
		t.Fatalf("private goal evidence sentinel crossed the public projection")
	}
	summaryBody, err := json.Marshal(SummaryIndexProjection(projected))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(summaryBody), privateSentinel) || strings.Contains(string(summaryBody), "evidenceDetails") {
		t.Fatal("private goal evidence metadata crossed the public list projection")
	}
	sourceGoal := thread["goal"].(map[string]any)
	sourceEntry := sourceGoal["evidenceLedger"].([]any)[0].(map[string]any)
	if _, present := sourceEntry["evidenceDetails"]; !present {
		t.Fatal("public projection mutated durable goal evidence")
	}

	service := NewService(Dependencies{
		Repository: &singleThreadProjectionRepository{repositoryStub: &repositoryStub{thread: thread}},
	})
	listed, err := service.List(ListInput{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ordinary public list projection failed: count=%d err=%v", len(listed), err)
	}
	detail, err := service.Get(threadID)
	if err != nil || detail == nil {
		t.Fatalf("ordinary public detail projection failed: detail=%#v err=%v", detail, err)
	}
	for label, value := range map[string]any{"list": listed, "detail": detail} {
		publicBody, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(publicBody), privateSentinel) || strings.Contains(string(publicBody), "evidenceDetails") {
			t.Fatalf("private goal evidence metadata crossed the %s service projection", label)
		}
	}
}

func TestOrdinaryThreadProjectionRejectsMalformedGoalLedger(t *testing.T) {
	timestamp := "2026-08-04T00:00:00Z"
	validGoal := func(threadID string) map[string]any {
		return map[string]any{
			"id": "goal_1", "threadId": threadID, "objective": "verify the public goal projection",
			"status": "active", "tokensUsed": float64(1), "timeUsedSeconds": float64(2),
			"createdAt": timestamp, "updatedAt": timestamp,
			"evidenceLedger": []any{map[string]any{
				"id": "goal_ev_1", "step": "Run the focused test",
				"evidence": []any{"focused test passed"}, "createdAt": timestamp,
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing-required-goal-field", mutate: func(goal map[string]any) { delete(goal, "threadId") }},
		{name: "invalid-status", mutate: func(goal map[string]any) { goal["status"] = "unknown" }},
		{name: "negative-usage", mutate: func(goal map[string]any) { goal["tokensUsed"] = float64(-1) }},
		{name: "non-array-ledger", mutate: func(goal map[string]any) { goal["evidenceLedger"] = "invalid" }},
		{name: "non-object-entry", mutate: func(goal map[string]any) { goal["evidenceLedger"] = []any{"invalid"} }},
		{name: "missing-entry-field", mutate: func(goal map[string]any) {
			delete(goal["evidenceLedger"].([]any)[0].(map[string]any), "step")
		}},
		{name: "non-string-evidence", mutate: func(goal map[string]any) {
			goal["evidenceLedger"].([]any)[0].(map[string]any)["evidence"] = []any{map[string]any{"rawPath": "must-not-pass"}}
		}},
		{name: "empty-evidence", mutate: func(goal map[string]any) {
			goal["evidenceLedger"].([]any)[0].(map[string]any)["evidence"] = []any{}
		}},
		{name: "oversized-ledger", mutate: func(goal map[string]any) {
			entry := goal["evidenceLedger"].([]any)[0]
			ledger := make([]any, 501)
			for index := range ledger {
				ledger[index] = entry
			}
			goal["evidenceLedger"] = ledger
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			thread, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
			threadID, _ := thread["id"].(string)
			goal := validGoal(threadID)
			test.mutate(goal)
			thread["goal"] = goal
			if projected, err := ProjectPublicThread(thread); err == nil || projected != nil {
				t.Fatalf("malformed goal crossed the public projection: projected=%#v err=%v", projected, err)
			}
		})
	}

	thread, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	threadID, _ := thread["id"].(string)
	thread["goal"] = validGoal(threadID)
	thread["goal"].(map[string]any)["evidenceLedger"].([]any)[0].(map[string]any)["evidence"] = []any{map[string]any{"rawPath": "must-not-pass"}}
	service := NewService(Dependencies{
		Repository: &singleThreadProjectionRepository{repositoryStub: &repositoryStub{thread: thread}},
	})
	if listed, err := service.List(ListInput{}); err == nil || listed != nil {
		t.Fatalf("malformed goal evidence crossed the public list service: listed=%#v err=%v", listed, err)
	}
	if detail, err := service.Get(threadID); err == nil || detail != nil {
		t.Fatalf("malformed goal evidence crossed the public detail service: detail=%#v err=%v", detail, err)
	}
}
