package thread

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
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

func TestArchiveIntentPublicPatchIsSoleBoolean(t *testing.T) {
	for _, archived := range []bool{false, true} {
		if err := ValidatePublicPatch(map[string]any{"archived": archived}); err != nil {
			t.Fatalf("boolean archive intent was rejected: %v", err)
		}
	}
	for name, patch := range map[string]map[string]any{
		"null":            {"archived": nil},
		"string":          {"archived": "true"},
		"number":          {"archived": float64(1)},
		"object":          {"archived": map[string]any{}},
		"mixed-title":     {"archived": true, "title": "changed"},
		"mixed-workspace": {"archived": false, "workspace": "/other"},
		"mixed-unknown":   {"archived": true, "unknown": true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicPatch(patch); err == nil {
				t.Fatal("invalid or mixed archive intent was accepted")
			}
			thread := BuildThread(CreateInput{ThreadID: "thr_archive", Now: "before"})
			before := contracts.CloneMap(thread)
			if next, err := ApplyPatchMutation(thread, "thr_archive", patch, "", "after"); err == nil || next != nil {
				t.Fatal("durable mutation accepted invalid archive intent")
			}
			if !reflect.DeepEqual(thread, before) {
				t.Fatal("rejected archive intent changed its input")
			}
		})
	}
	for _, status := range []any{"idle", "archived", "running", "deleted", nil, true} {
		for _, patch := range []map[string]any{{"status": status}, {"archived": true, "status": status}} {
			if err := ValidatePublicPatch(patch); !errors.Is(err, ErrThreadStatusRuntimeOwned) {
				t.Fatalf("public status write escaped runtime ownership: %v", err)
			}
		}
	}
}

func TestArchiveIntentDurableMutationPreservesStateAndChecksBaseline(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     string
		turnStatus string
		archived   bool
		want       string
	}{
		{name: "archive-idle", status: "idle", archived: true, want: "archived"},
		{name: "archive-idempotent", status: "archived", archived: true, want: "archived"},
		{name: "restore", status: "archived", archived: false, want: "idle"},
		{name: "restore-idempotent", status: "idle", archived: false, want: "idle"},
		{name: "completed-history", status: "idle", turnStatus: "completed", archived: true, want: "archived"},
		{name: "running-thread", status: "running", archived: true},
		{name: "running-turn", status: "idle", turnStatus: "running", archived: true},
		{name: "queued-turn", status: "idle", turnStatus: "queued", archived: true},
		{name: "restore-queued-turn", status: "archived", turnStatus: "queued", archived: false},
		{name: "deleted-archive", status: "deleted", archived: true},
		{name: "deleted-restore", status: "deleted", archived: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread := BuildThread(CreateInput{Request: map[string]any{"title": "preserved", "workspace": "/workspace"}, ThreadID: "thr_archive", Now: "before"})
			thread["status"] = test.status
			if test.turnStatus != "" {
				thread["turns"] = []any{map[string]any{"id": "turn_archive", "status": test.turnStatus, "items": []any{}}}
			}
			before := contracts.CloneMap(thread)
			digest, err := MutationBaselineDigest(thread)
			if err != nil {
				t.Fatal(err)
			}
			next, err := ApplyPatchMutation(thread, "thr_archive", map[string]any{"archived": test.archived}, digest, "after")
			if test.want == "" {
				if !errors.Is(err, ErrThreadMutationTransition) || next != nil {
					t.Fatalf("busy/deleted state admitted archive intent: err=%v", err)
				}
			} else {
				if err != nil || next["status"] != test.want || next["updatedAt"] != "after" {
					t.Fatalf("archive transition did not derive runtime status: err=%v", err)
				}
				if _, persisted := next["archived"]; persisted {
					t.Fatal("archive intent changed the persisted thread shape")
				}
				if next["title"] != before["title"] || next["workspace"] != before["workspace"] || !reflect.DeepEqual(next["turns"], before["turns"]) {
					t.Fatal("archive transition changed thread content or turn history")
				}
			}
			if !reflect.DeepEqual(thread, before) {
				t.Fatal("archive mutation changed its input")
			}
		})
	}
	thread := BuildThread(CreateInput{ThreadID: "thr_archive", Now: "before"})
	digest, err := MutationBaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	thread["title"] = "concurrent metadata"
	if next, err := ApplyPatchMutation(thread, "thr_archive", map[string]any{"archived": true}, digest, "after"); !errors.Is(err, ErrThreadMutationBaselineConflict) || next != nil {
		t.Fatalf("archive ignored its stale durable baseline: err=%v", err)
	}
}

func TestApplyPatchSealsExplicitCompleteExecutionPolicy(t *testing.T) {
	for _, version := range []struct {
		name  string
		value any
		want  float64
	}{
		{name: "missing", want: 2},
		{name: "v0", value: float64(0), want: 2},
		{name: "v1", value: float64(1), want: 2},
		{name: "v2", value: float64(2), want: 2},
		{name: "future", value: float64(3), want: 3},
	} {
		for _, policy := range []struct {
			name         string
			approval     string
			sandbox      string
			wantApproval string
			wantSandbox  string
		}{
			{name: "same-legacy-full", approval: "auto", sandbox: "danger-full-access", wantApproval: "auto", wantSandbox: "danger-full-access"},
			{name: "read-only", approval: "never", sandbox: "read-only", wantApproval: "never", wantSandbox: "read-only"},
			{name: "custom", approval: "untrusted", sandbox: "external-sandbox", wantApproval: "untrusted", wantSandbox: "external-sandbox"},
			{name: "canonical", approval: "  suggest ", sandbox: " workspace-write  ", wantApproval: "suggest", wantSandbox: "workspace-write"},
		} {
			t.Run(version.name+"/"+policy.name, func(t *testing.T) {
				thread := map[string]any{
					"id": "thr_policy", "status": "running", "updatedAt": "before",
					"approvalPolicy": "auto", "sandboxMode": "danger-full-access",
					"pendingApprovalIds": []any{"approval_pending"}, "pendingUserInputIds": []any{"input_pending"},
					"turns": []any{map[string]any{
						"id": "turn_current", "status": "running", "approvalPolicy": "on-request", "sandboxMode": "workspace-write",
						"items": []any{map[string]any{"id": "tool_pending", "kind": "tool_call", "status": "running"}},
					}},
				}
				if version.value != nil {
					thread["executionPolicyVersion"] = version.value
				}
				patch := map[string]any{"approvalPolicy": policy.approval, "sandboxMode": policy.sandbox}
				before, beforePatch := contracts.CloneMap(thread), contracts.CloneMap(patch)
				if err := ValidatePublicPatch(patch); err != nil {
					t.Fatalf("explicit valid pair was rejected: %v", err)
				}
				next := ApplyPatch(thread, patch, "after")
				if next["approvalPolicy"] != policy.wantApproval || next["sandboxMode"] != policy.wantSandbox || next["executionPolicyVersion"] != version.want {
					t.Fatalf("explicit pair did not preserve its canonical selection/version: %#v", next)
				}
				if next["updatedAt"] != "after" || next["status"] != "running" || !reflect.DeepEqual(next["turns"], before["turns"]) ||
					!reflect.DeepEqual(next["pendingApprovalIds"], before["pendingApprovalIds"]) || !reflect.DeepEqual(next["pendingUserInputIds"], before["pendingUserInputIds"]) {
					t.Fatal("future default metadata changed the running turn or pending work")
				}
				if !reflect.DeepEqual(thread, before) || !reflect.DeepEqual(patch, beforePatch) {
					t.Fatal("explicit policy mutation changed its input records")
				}
			})
		}
	}
}

func TestApplyPatchDoesNotSealIncompleteOrInvalidExecutionPolicy(t *testing.T) {
	for name, patch := range map[string]map[string]any{
		"empty":               {},
		"title":               {"title": "loading"},
		"approval-only":       {"approvalPolicy": "never"},
		"sandbox-only":        {"sandboxMode": "read-only"},
		"loading-approval":    {"approvalPolicy": "loading", "sandboxMode": "read-only"},
		"loading-sandbox":     {"approvalPolicy": "never", "sandboxMode": "loading"},
		"blank-approval":      {"approvalPolicy": " ", "sandboxMode": "read-only"},
		"null-sandbox":        {"approvalPolicy": "never", "sandboxMode": nil},
		"non-string-approval": {"approvalPolicy": true, "sandboxMode": "read-only"},
		"client-marker-only":  {"executionPolicyVersion": float64(2)},
	} {
		t.Run(name, func(t *testing.T) {
			thread := map[string]any{"id": "thr_policy", "approvalPolicy": "auto", "sandboxMode": "danger-full-access", "updatedAt": "before"}
			before := contracts.CloneMap(thread)
			next := ApplyPatch(thread, patch, "after")
			if _, sealed := next["executionPolicyVersion"]; sealed {
				t.Fatalf("incomplete/invalid metadata manufactured a policy receipt: %#v", next)
			}
			for _, field := range []string{"approvalPolicy", "sandboxMode"} {
				if value, present := patch[field]; present && !reflect.DeepEqual(next[field], value) {
					t.Fatalf("direct malformed leaf call substituted a policy default for %s: %#v", field, next[field])
				}
			}
			if !reflect.DeepEqual(thread, before) {
				t.Fatal("incomplete/invalid direct leaf call changed its input")
			}
		})
	}
}

func TestValidatePublicPatchRejectsEveryPresentInvalidPolicyField(t *testing.T) {
	invalid := []any{nil, true, float64(1), []any{}, map[string]any{}, "", "  ", "loading", "unknown"}
	for _, field := range []string{"approvalPolicy", "sandboxMode"} {
		for index, value := range invalid {
			for _, complete := range []bool{false, true} {
				patch := map[string]any{}
				if complete {
					patch["approvalPolicy"], patch["sandboxMode"] = "never", "read-only"
				}
				patch[field] = value
				if err := ValidatePublicPatch(patch); !errors.Is(err, ErrInvalidThreadExecutionPolicy) {
					t.Fatalf("invalid %s[%d] (complete=%v) escaped the public policy boundary: %v", field, index, complete, err)
				}
			}
		}
	}
	for _, patch := range []map[string]any{
		{"approvalPolicy": "never"}, {"sandboxMode": "read-only"},
		{"approvalPolicy": " always ", "sandboxMode": " external-sandbox "}, {"title": "loading"},
	} {
		if err := ValidatePublicPatch(patch); err != nil {
			t.Fatalf("valid partial/custom or unrelated metadata was rejected: %v", err)
		}
	}
	for _, marker := range []any{nil, float64(0), float64(1), float64(2), float64(3), "2"} {
		patch := map[string]any{"executionPolicyVersion": marker, "approvalPolicy": "never", "sandboxMode": "read-only"}
		if err := ValidatePublicPatch(patch); !errors.Is(err, ErrExecutionPolicyVersionRuntimeOwned) {
			t.Fatalf("complete pair allowed a client-owned policy marker: %v", err)
		}
	}
}

func TestApplyPatchPreservesFuturePolicyVersionAcrossJSONNumericInputs(t *testing.T) {
	for name, version := range map[string]any{"int": 3, "int64": int64(3), "json-number": json.Number("3"), "float": float64(3)} {
		t.Run(name, func(t *testing.T) {
			thread := map[string]any{"id": "thr_future", "executionPolicyVersion": version, "approvalPolicy": "auto", "sandboxMode": "danger-full-access"}
			before, err := json.Marshal(thread)
			if err != nil {
				t.Fatal(err)
			}
			next := ApplyPatch(thread, map[string]any{"approvalPolicy": " untrusted ", "sandboxMode": " read-only "}, "after")
			if next["executionPolicyVersion"] != float64(3) || next["approvalPolicy"] != "untrusted" || next["sandboxMode"] != "read-only" {
				t.Fatalf("future numeric policy version was downgraded or canonical pair was lost: %#v", next)
			}
			after, err := json.Marshal(thread)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("future version input record changed")
			}
		})
	}
}
