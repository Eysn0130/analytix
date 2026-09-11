package thread

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBuildThreadDefaults(t *testing.T) {
	thread := BuildThread(CreateInput{
		Request: map[string]any{
			"autoTitle": true,
		},
		ThreadID:          "thr_1",
		FallbackWorkspace: " /workspace ",
		Now:               "now",
	})
	if thread["id"] != "thr_1" || thread["title"] != "New thread" || thread["workspace"] != "/workspace" {
		t.Fatalf("thread defaults mismatch: %#v", thread)
	}
	if thread["mode"] != "agent" || thread["approvalPolicy"] != "on-request" || thread["sandboxMode"] != "workspace-write" {
		t.Fatalf("thread runtime defaults mismatch: %#v", thread)
	}
	if thread["executionPolicyVersion"] != float64(2) {
		t.Fatalf("execution policy marker mismatch: %#v", thread)
	}
	if _, ok := thread["providerId"]; ok {
		t.Fatalf("empty providerId should be omitted: %#v", thread)
	}
	if thread["autoTitle"] != true {
		t.Fatalf("autoTitle should be preserved: %#v", thread)
	}
}

func TestBuildThreadPreservesPlanAndProvider(t *testing.T) {
	thread := BuildThread(CreateInput{
		Request: map[string]any{
			"title":          "Plan",
			"workspace":      "/explicit",
			"providerId":     "openai-compatible",
			"model":          "gpt-compatible",
			"mode":           "plan",
			"approvalPolicy": "never",
			"sandboxMode":    "read-only",
			"relation":       "side",
			"parentThreadId": "thr_parent",
		},
		ThreadID: "thr_2",
		Now:      "now",
	})
	if thread["providerId"] != "openai-compatible" || thread["mode"] != "plan" {
		t.Fatalf("thread request fields mismatch: %#v", thread)
	}
	if thread["approvalPolicy"] != "never" || thread["sandboxMode"] != "read-only" {
		t.Fatalf("thread policy fields mismatch: %#v", thread)
	}
	if thread["relation"] != "side" || thread["parentThreadId"] != "thr_parent" {
		t.Fatalf("thread lineage fields mismatch: %#v", thread)
	}
}

func TestApplyPatchAndMarkDeleted(t *testing.T) {
	thread := map[string]any{
		"id": "thr_1", "title": "Old", "status": "idle", "executionPolicyVersion": float64(2), "updatedAt": "old",
		"turns": []any{map[string]any{
			"id": "turn_1",
			"items": []any{map[string]any{
				"id": "result_1", "hostEvidenceSettlement": map[string]any{"settlementId": "settlement-a"},
			}},
		}},
	}
	patched := ApplyPatch(thread, map[string]any{"title": "New", "parentThreadId": "thr_parent", "executionPolicyVersion": float64(99), "unknown": "skip"}, "now")
	if patched["title"] != "New" || patched["updatedAt"] != "now" || patched["unknown"] != nil {
		t.Fatalf("patch mismatch: %#v", patched)
	}
	if patched["parentThreadId"] != "thr_parent" {
		t.Fatalf("parentThreadId patch mismatch: %#v", patched)
	}
	if patched["executionPolicyVersion"] != float64(2) {
		t.Fatalf("public lifecycle patch must preserve the runtime-owned marker: %#v", patched)
	}
	if err := ValidatePublicPatch(map[string]any{"executionPolicyVersion": float64(2)}); !errors.Is(err, ErrExecutionPolicyVersionRuntimeOwned) {
		t.Fatalf("runtime-owned public patch should be rejected: %v", err)
	}
	if err := ValidatePublicPatch(map[string]any{"contextEpochState": map[string]any{}}); !errors.Is(err, ErrContextEpochStateRuntimeOwned) {
		t.Fatalf("context epoch state must be runtime-owned: %v", err)
	}
	if err := ValidatePublicPatch(map[string]any{"status": "idle"}); !errors.Is(err, ErrThreadStatusRuntimeOwned) {
		t.Fatalf("thread status must be runtime-owned: %v", err)
	}
	if thread["title"] != "Old" || thread["updatedAt"] != "old" {
		t.Fatalf("patch should not mutate input: %#v", thread)
	}
	deleted := MarkDeleted(thread, "later")
	if deleted["status"] != "deleted" || deleted["updatedAt"] != "later" {
		t.Fatalf("delete marker mismatch: %#v", deleted)
	}
	deletedBody, _ := json.Marshal(deleted)
	if !strings.Contains(string(deletedBody), "settlement-a") {
		t.Fatal("logical deletion discarded durable evidence settlement authority")
	}
}

func TestBuildUpdatedEvent(t *testing.T) {
	event := BuildUpdatedEvent(" thr_1 ", " New title ", " idle ")
	if event["kind"] != "thread_updated" ||
		event["threadId"] != "thr_1" ||
		event["title"] != nil ||
		event["status"] != "idle" {
		t.Fatalf("thread updated event mismatch: %#v", event)
	}
}

func TestThreadLifecycleProjectsRestrictedPIIOnlyFromDisplayTitle(t *testing.T) {
	thread := BuildThread(CreateInput{
		Request: map[string]any{
			"title":     "账号 6222020000000000000",
			"workspace": "/cases/6222020000000000000",
		},
		ThreadID: "thread_6222020000000000000",
		Now:      "now",
	})
	if thread["title"] != "账号 [ACCOUNT]" {
		t.Fatalf("thread title projection = %#v", thread)
	}
	if thread["id"] != "thread_6222020000000000000" || thread["workspace"] != "/cases/6222020000000000000" {
		t.Fatalf("privacy projection mutated authority fields: %#v", thread)
	}
	patched := ApplyPatch(thread, map[string]any{"title": "电话 13800138000"}, "later")
	if patched["title"] != "电话 [PHONE]" {
		t.Fatalf("patched title projection = %#v", patched)
	}
	event := BuildUpdatedEvent("thread_6222020000000000000", "卡号 6222020000000000000", "idle")
	if event["threadId"] != "thread_6222020000000000000" || event["title"] != nil {
		t.Fatalf("thread event projection = %#v", event)
	}
}
