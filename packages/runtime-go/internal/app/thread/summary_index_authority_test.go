package thread

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
)

func summaryIndexMetadataFixtureV1() map[string]any {
	const stamp = "2026-09-13T07:17:19.123456789Z"
	const id = "thread-13812345678"
	thread := map[string]any{
		"id": id, "title": "Ordinary response", "workspace": "/tmp/workspace",
		"model": "synthetic/model", "providerId": "provider-a", "mode": "agent",
		"status": "idle", "executionPolicyVersion": float64(1), "approvalPolicy": "auto",
		"sandboxMode": "workspace-write", "relation": "fork", "parentThreadId": "parent-13812345678",
		"forkedFromThreadId": "source-13812345678", "forkedFromTitle": "Original",
		"forkedAt": stamp, "forkedFromMessageCount": float64(1), "forkedFromTurnCount": float64(1),
		"preview": "Completed", "createdAt": stamp, "updatedAt": stamp,
		"archived": false, "pinned": true,
		"turns": []any{map[string]any{"id": "turn-13812345678", "status": "aborted"}},
		"goal": map[string]any{
			"id": "goal-13812345678", "threadId": id, "objective": "Finish the task", "status": "active",
			"tokensUsed": float64(0), "timeUsedSeconds": float64(0), "createdAt": stamp, "updatedAt": stamp,
			"evidenceLedger": []any{map[string]any{
				"id": "evidence-13812345678", "turnId": "turn-13812345678", "step": "Verify the result",
				"evidence": []any{"The check passed"}, "createdAt": stamp,
			}},
		},
		"todos": map[string]any{
			"threadId": id, "updatedAt": stamp,
			"items": []any{map[string]any{
				"id": "todo-13812345678", "content": "Verify", "status": "pending",
				"createdAt": stamp, "updatedAt": stamp, "source": map[string]any{"kind": "manual"},
			}},
		},
	}
	return map[string]any{
		"schemaVersion": float64(1), "threadId": id, "summary": SummaryIndexProjection(thread),
		"updatedAt": stamp, "writtenAt": stamp,
	}
}

func TestSummaryIndexRecordKeepsTypedMetadataAndExistingProducerFields(t *testing.T) {
	record := summaryIndexMetadataFixtureV1()
	before := contracts.CloneMap(record)
	if err := ValidateSummaryIndexRecordV1(record); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record, before) {
		t.Fatal("summary validation mutated persisted metadata")
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSummaryIndexRecordV1(decoded); err != nil {
		t.Fatal(err)
	}
	caseRecord := contracts.CloneMap(record)
	caseSummary := caseRecord["summary"].(map[string]any)
	delete(caseSummary, "goal")
	delete(caseSummary, "todos")
	delete(caseSummary, "preview")
	caseSummary["caseId"] = "case-a"
	caseSummary["caseProjectId"] = "case-project"
	caseSummary["historyAuthority"] = "case_boundary_only_v1"
	if err := ValidateSummaryIndexRecordV1(caseRecord); err != nil {
		t.Fatal("case boundary summary metadata was rejected")
	}
	minimal := map[string]any{"schemaVersion": float64(1), "threadId": "thread-a", "summary": map[string]any{"id": "thread-a"}}
	if err := ValidateSummaryIndexRecordV1(minimal); err != nil {
		t.Fatal("legacy lookup-only row was rejected")
	}
}

func TestSummaryIndexRecordRejectsUnsafeContentAndMetadataLookalikes(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"title PII":   func(_ map[string]any, summary map[string]any) { summary["title"] = "13812345678" },
		"preview PII": func(_ map[string]any, summary map[string]any) { summary["preview"] = "13812345678" },
		"secret": func(_ map[string]any, summary map[string]any) {
			summary["preview"] = "api_key=synthetic-secret-material"
		},
		"unknown envelope": func(record map[string]any, _ map[string]any) { record["extra"] = "injected" },
		"unknown summary":  func(_ map[string]any, summary map[string]any) { summary["extra"] = "injected" },
		"invalid timestamp": func(_ map[string]any, summary map[string]any) {
			summary["createdAt"] = "2026-99-13T07:17:19.123456789Z"
		},
		"phone timestamp": func(_ map[string]any, summary map[string]any) { summary["updatedAt"] = "13812345678" },
		"metadata object": func(_ map[string]any, summary map[string]any) {
			summary["latestTurnId"] = map[string]any{"text": "13812345678"}
		},
		"identity mismatch": func(record map[string]any, _ map[string]any) { record["threadId"] = "another-thread" },
		"goal prose": func(_ map[string]any, summary map[string]any) {
			summary["goal"].(map[string]any)["objective"] = "13812345678"
		},
		"unknown goal": func(_ map[string]any, summary map[string]any) { summary["goal"].(map[string]any)["extra"] = "injected" },
		"todo prose": func(_ map[string]any, summary map[string]any) {
			summary["todos"].(map[string]any)["items"].([]any)[0].(map[string]any)["content"] = "13812345678"
		},
		"unknown todo": func(_ map[string]any, summary map[string]any) {
			summary["todos"].(map[string]any)["items"].([]any)[0].(map[string]any)["extra"] = "injected"
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := summaryIndexMetadataFixtureV1()
			mutate(record, record["summary"].(map[string]any))
			if err := ValidateSummaryIndexRecordV1(record); err == nil {
				t.Fatal("unsafe summary record gained a metadata exemption")
			}
		})
	}
}

func TestSummaryIndexProducerKeepsDurableGoalDetailsOutsidePublicHints(t *testing.T) {
	record := summaryIndexMetadataFixtureV1()
	canonical := contracts.CloneMap(record["summary"].(map[string]any))
	goal := canonical["goal"].(map[string]any)
	goal["research"] = map[string]any{"enabled": true, "stateRefDigest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "requirementCount": float64(1)}
	entry := goal["evidenceLedger"].([]any)[0].(map[string]any)
	entry["evidenceDetails"] = []any{map[string]any{"kind": "manual"}}
	before := contracts.CloneMap(canonical)
	record["summary"] = SummaryIndexProjection(canonical)
	if err := ValidateSummaryIndexRecordV1(record); err != nil {
		t.Fatalf("legitimate durable goal prevented summary persistence: %v", err)
	}
	publicGoal := record["summary"].(map[string]any)["goal"].(map[string]any)
	publicEntry := publicGoal["evidenceLedger"].([]any)[0].(map[string]any)
	if publicGoal["research"] != nil || publicEntry["evidenceDetails"] != nil || !reflect.DeepEqual(canonical, before) {
		t.Fatal("index either retained private goal details or mutated their canonical owner")
	}
}

func TestSummaryIndexTodoMetadataDoesNotBecomeProse(t *testing.T) {
	record := summaryIndexMetadataFixtureV1()
	item := record["summary"].(map[string]any)["todos"].(map[string]any)["items"].([]any)[0].(map[string]any)
	item["evidenceIds"] = []any{"goal_ev_13812345678"}
	item["source"] = map[string]any{"kind": "plan", "planId": "plan-13812345678", "relativePath": "docs/plan.md", "contentHash": strings.Repeat("a", 53) + "13812345678"}
	before := contracts.CloneMap(record)
	if err := ValidateSummaryIndexRecordV1(record); err != nil || !reflect.DeepEqual(before, record) {
		t.Fatalf("typed todo evidence metadata was rejected or changed: %v", err)
	}
	for _, field := range []string{"contentHash", "relativePath"} {
		bad := contracts.CloneMap(record)
		source := bad["summary"].(map[string]any)["todos"].(map[string]any)["items"].([]any)[0].(map[string]any)["source"].(map[string]any)
		source[field] = "contact 13812345678"
		if ValidateSummaryIndexRecordV1(bad) == nil {
			t.Fatal("metadata lookalike bypassed prose validation")
		}
	}
}
