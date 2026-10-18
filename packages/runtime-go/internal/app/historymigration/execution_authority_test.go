package historymigration

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestProjectAuthorityFreeSidecarItemPreservesCanonicalLegacyItemIdentity(t *testing.T) {
	item := map[string]any{
		"id": "legacy-call-item", "threadId": "source-thread", "turnId": "turn-1",
		"kind": "tool_call", "status": "completed", "toolName": "read_file",
		"callId": "legacy-provider-call", "arguments": map[string]any{"path": "private"},
		"contextDigest": "untrusted-context", "executionGrantId": "untrusted-grant",
	}

	projected, keep := ProjectAuthorityFreeSidecarItem("target-thread", item, "2026-07-23T00:00:00Z")
	if !keep || projected == nil {
		t.Fatal("canonical legacy item was dropped")
	}
	if projected["id"] != "legacy-call-item" || projected["threadId"] != "target-thread" {
		t.Fatalf("legacy rendering identity was not preserved: %#v", projected)
	}
	if projected["contextDigest"] != nil || projected["executionGrantId"] != nil {
		t.Fatalf("legacy execution authority survived projection: %#v", projected)
	}
	arguments, _ := projected["arguments"].(map[string]any)
	if arguments == nil || arguments["messageKey"] != "tool_arguments_withheld" {
		t.Fatalf("legacy tool arguments were not closed: %#v", projected)
	}
}

func TestProjectAuthorityFreeSidecarItemDoesNotPreserveInvalidLegacyItemIdentity(t *testing.T) {
	projected, keep := ProjectAuthorityFreeSidecarItem("target-thread", map[string]any{
		"id": "../invalid", "turnId": "turn-1", "kind": "tool_call", "callId": "legacy-provider-call",
	}, "2026-07-23T00:00:00Z")
	if !keep || projected == nil {
		t.Fatal("closed legacy item projection was unexpectedly dropped")
	}
	if projected["id"] != nil {
		t.Fatalf("invalid legacy item identity survived projection: %#v", projected)
	}
}

func TestProjectAuthorityFreeSidecarItemPreservesQuestionHistoryIdentityV1(t *testing.T) {
	inputID := "input_" + strings.Repeat("a", 64)
	at := "2026-10-10T00:00:00Z"
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		wantID bool
		status string
	}{
		{name: "continued", wantID: true, status: "continued"},
		{name: "pending_closes", mutate: func(item map[string]any) { item["status"] = "pending" }, wantID: true, status: "cancelled"},
		{name: "invalid_hash", mutate: func(item map[string]any) { item["inputId"] = "input_invalid"; item["id"] = "item_input_invalid" }, status: "continued"},
		{name: "wrong_item_id", mutate: func(item map[string]any) { item["id"] = "other_item" }, status: "continued"},
		{name: "non_question", mutate: func(item map[string]any) { item["kind"] = "user_message" }, status: "continued"},
		{name: "missing_input_id", mutate: func(item map[string]any) { delete(item, "inputId") }, status: "continued"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := map[string]any{
				"id": "item_" + inputID, "inputId": inputID, "kind": "user_input", "status": "continued",
				"threadId": "source", "turnId": "turn_question", "questions": []any{},
				"continuationReceiptId": "source_receipt", "approvalId": "source_approval",
				"executionGrantId": "source_grant", "executionGrant": map[string]any{"grantId": "source_grant"},
				"parentGrantId": "source_parent_grant", "contextDigest": "source_context", "contextEpoch": float64(1),
				"hostEvidenceSettlement": map[string]any{"settlementId": "source_settlement"},
				"approvalTransitionId":   "source_transition", "continuationDispositionId": "source_disposition",
			}
			if test.mutate != nil {
				test.mutate(item)
			}
			before, _ := json.Marshal(item)
			projected, keep := ProjectAuthorityFreeSidecarItem("child", item, at)
			if !keep || projected == nil || projected["threadId"] != "child" || projected["status"] != test.status {
				t.Fatal("history_projection_or_lifecycle_invalid")
			}
			if test.wantID {
				if projected["inputId"] != inputID || projected["id"] != "item_"+inputID {
					t.Fatal("canonical_question_identity_lost")
				}
			} else if _, present := projected["inputId"]; present {
				t.Fatal("invalid_question_identity_promoted")
			}
			for _, key := range []string{"continuationReceiptId", "approvalId", "executionGrantId", "executionGrant", "parentGrantId", "contextDigest", "contextEpoch", "hostEvidenceSettlement", "approvalTransitionId", "continuationDispositionId"} {
				if _, present := projected[key]; present {
					t.Fatal("history_inherited_execution_or_answer_authority")
				}
			}
			after, _ := json.Marshal(item)
			if !bytes.Equal(before, after) {
				t.Fatal("projection_mutated_original_question")
			}
			if test.status == "cancelled" && projected["finishedAt"] != at {
				t.Fatal("pending_history_did_not_close_at_migration_time")
			}
		})
	}
}

func TestStripUntrustedExecutionAuthorityQuestionHistoryFixedPointV1(t *testing.T) {
	inputID := "input_" + strings.Repeat("b", 64)
	thread := map[string]any{
		"id": "child", "forkedFromThreadId": "source", "forkedAt": "2026-10-10T00:00:00Z",
		"turns": []any{map[string]any{
			"id": "turn_question", "threadId": "child", "status": "completed",
			"items": []any{map[string]any{
				"id": "item_" + inputID, "inputId": inputID, "kind": "user_input", "status": "continued",
				"threadId": "child", "turnId": "turn_question", "questions": []any{},
				"continuationReceiptId": "source_receipt", "executionGrantId": "source_grant",
			}},
		}},
	}
	before, _ := json.Marshal(thread)
	first, changed, err := StripUntrustedExecutionAuthority(thread)
	if err != nil || !changed {
		t.Fatal("first_migration_did_not_strip_untrusted_authority")
	}
	item := first["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["inputId"] != inputID || item["status"] != "continued" || item["continuationReceiptId"] != nil || item["executionGrantId"] != nil {
		t.Fatal("migration_changed_history_or_retained_authority")
	}
	second, changed, err := StripUntrustedExecutionAuthority(first)
	if err != nil || changed || !reflect.DeepEqual(first, second) {
		t.Fatal("question_history_migration_has_no_fixed_point")
	}
	after, _ := json.Marshal(thread)
	if !bytes.Equal(before, after) {
		t.Fatal("migration_mutated_original_thread")
	}
}

func TestStripUntrustedExecutionAuthorityQuestionHistoryKeepsOwnedContextFailClosedV1(t *testing.T) {
	inputID := "input_" + strings.Repeat("c", 64)
	thread := map[string]any{
		"id": "child", "forkedFromThreadId": "source",
		"turns": []any{map[string]any{
			"id": "turn_question", "threadId": "child", "securityContext": map[string]any{"version": 2},
			"items": []any{map[string]any{
				"id": "item_" + inputID, "inputId": inputID, "kind": "user_input", "status": "continued",
			}},
		}},
	}
	before, _ := json.Marshal(thread)
	projected, changed, err := StripUntrustedExecutionAuthority(thread)
	if err == nil || changed || projected != nil {
		t.Fatal("invalid_owned_context_became_authority_free_history")
	}
	after, _ := json.Marshal(thread)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected_migration_mutated_owned_context")
	}
}
