package turn

import "testing"

func TestBuildStartThreadPatchIncludesOnlyDurableOverrides(t *testing.T) {
	patch := BuildStartThreadPatch(StartThreadPatchInput{
		RequestApprovalPolicy: " on-request ",
		RequestSandboxMode:    " workspace-write ",
		Model:                 " model-a ",
		ReasoningEffort:       "high",
		EndpointFormat:        " responses ",
	})
	if patch["approvalPolicy"] != "on-request" ||
		patch["sandboxMode"] != "workspace-write" ||
		patch["model"] != "model-a" ||
		patch["reasoningEffort"] != "high" ||
		patch["endpointFormat"] != "responses" {
		t.Fatalf("thread patch mismatch: %#v", patch)
	}
	empty := BuildStartThreadPatch(StartThreadPatchInput{})
	if len(empty) != 0 {
		t.Fatalf("empty input should not persist thread patch: %#v", empty)
	}
}

func TestBuildStartRecordCarriesTurnContractFields(t *testing.T) {
	maxSteps := 7
	input := StartRecordInput{
		ThreadID:              " thread-a ",
		TurnID:                " turn-1 ",
		UserItemID:            "item-custom-user",
		Prompt:                "run analysis",
		DisplayText:           "shown prompt",
		Model:                 " model-a ",
		ProviderID:            " provider-a ",
		ReasoningEffort:       "high",
		ApprovalPolicy:        " on-request ",
		SandboxMode:           " workspace-write ",
		CreatedAt:             "2026-01-02T03:04:05Z",
		Mode:                  " plan ",
		AttachmentIDs:         []string{"att-1"},
		Attachments:           []map[string]any{{"id": "att-1", "localFilePath": "/tmp/a.png"}},
		AttachmentPipeline:    map[string]any{"usesVisionPayload": true},
		FileReferences:        []any{map[string]any{"path": "a.go"}},
		WorkspaceCheckpointID: "checkpoint-1",
		GUIPlan:               map[string]any{"enabled": true},
		DisableUserInput:      true,
		DisableUserInputSet:   true,
		MaxModelSteps:         &maxSteps,
	}
	record := BuildStartRecord(input)
	if record.UserItemID != "item-custom-user" {
		t.Fatalf("unexpected user item id: %s", record.UserItemID)
	}
	if got := stringField(record.UserItem, "threadId"); got != "thread-a" {
		t.Fatalf("user item thread id = %q", got)
	}
	if got := stringField(record.UserItem, "displayText"); got != "shown prompt" {
		t.Fatalf("displayText = %q", got)
	}
	if got := stringField(record.Turn, "model"); got != "model-a" {
		t.Fatalf("turn model = %q", got)
	}
	if got := stringField(record.TurnStartedEvent, "providerId"); got != "provider-a" {
		t.Fatalf("providerId = %q", got)
	}
	if got := record.Turn["maxModelSteps"]; got != float64(7) {
		t.Fatalf("maxModelSteps = %#v", got)
	}
	if got := record.Turn["disableUserInput"]; got != true {
		t.Fatalf("disableUserInput = %#v", got)
	}
	if _, ok := record.TurnStartedEvent["attachmentPipeline"].(map[string]any); !ok {
		t.Fatalf("turn_started missing attachment pipeline: %#v", record.TurnStartedEvent)
	}
	if got := stringField(record.UserItemCreatedEvent, "itemId"); got != "item-custom-user" {
		t.Fatalf("created item id = %q", got)
	}

	input.Attachments[0]["id"] = "mutated"
	itemAttachments, _ := record.UserItem["attachments"].([]any)
	first, _ := itemAttachments[0].(map[string]any)
	if got := stringField(first, "id"); got != "att-1" {
		t.Fatalf("attachments were not cloned: %q", got)
	}
}

func TestStartPersistenceRejectsNonCanonicalReasoningEffort(t *testing.T) {
	for _, effort := range []string{" high ", "HIGH", "none", "SOL_PRIVATE_REASONING_SENTINEL_7F3C"} {
		patch := BuildStartThreadPatch(StartThreadPatchInput{ReasoningEffort: effort})
		if _, ok := patch["reasoningEffort"]; ok {
			t.Fatalf("invalid reasoning effort entered thread patch: effort=%q patch=%#v", effort, patch)
		}
		record := BuildStartRecord(StartRecordInput{
			ThreadID: "thread-a", TurnID: "turn-1", Prompt: "hello", CreatedAt: "2026-01-02T03:04:05Z",
			ReasoningEffort: effort,
		})
		if _, ok := record.Turn["reasoningEffort"]; ok {
			t.Fatalf("invalid reasoning effort entered durable turn: effort=%q turn=%#v", effort, record.Turn)
		}
		startedTurn, _ := record.TurnStartedEvent["turn"].(map[string]any)
		if _, ok := startedTurn["reasoningEffort"]; ok {
			t.Fatalf("invalid reasoning effort entered start event: effort=%q event=%#v", effort, record.TurnStartedEvent)
		}
	}
}

func TestBuildStartRecordUsesDefaultUserItemID(t *testing.T) {
	record := BuildStartRecord(StartRecordInput{
		ThreadID:  "thread-a",
		TurnID:    "turn-1",
		Prompt:    "hello",
		CreatedAt: "2026-01-02T03:04:05Z",
	})
	if record.UserItemID != "item_turn-1_user" {
		t.Fatalf("default user item id = %q", record.UserItemID)
	}
	if _, ok := record.UserItem["displayText"]; ok {
		t.Fatalf("displayText should be omitted when empty")
	}
	if _, ok := record.Turn["reasoningEffort"]; ok {
		t.Fatalf("unset reasoningEffort must not enter durable turn state: %#v", record.Turn)
	}
	startedTurn, _ := record.TurnStartedEvent["turn"].(map[string]any)
	if _, ok := startedTurn["reasoningEffort"]; ok {
		t.Fatalf("unset reasoningEffort must not enter the start event: %#v", record.TurnStartedEvent)
	}
}

func TestBuildStartPlanResolvesRuntimePoliciesAndResponse(t *testing.T) {
	maxSteps := 3
	plan := BuildStartPlan(StartPlanInput{
		ThreadID:              " thread-a ",
		TurnNumber:            42,
		Prompt:                "hello",
		Model:                 "model-a",
		ProviderID:            "provider-a",
		ReasoningEffort:       "high",
		EndpointFormat:        "responses",
		RequestApprovalPolicy: "bogus",
		ThreadApprovalPolicy:  " never ",
		RuntimeApprovalPolicy: "on-request",
		RequestSandboxMode:    " danger-full-access ",
		ThreadSandboxMode:     "read-only",
		RuntimeSandboxMode:    "workspace-write",
		CreatedAt:             "2026-01-02T03:04:05Z",
		AttachmentIDs:         []string{"att-1"},
		Attachments: AttachmentPlan{
			IDs:                  []string{"att-1"},
			Metadata:             []map[string]any{{"id": "att-1"}},
			ModelInputModalities: []string{"text"},
			ModelMessageParts:    []string{"text"},
		},
		MaxModelSteps: &maxSteps,
	})
	if plan.ThreadID != "thread-a" || plan.TurnID != "turn_42" || plan.UserItemID != "item_turn_42_user" {
		t.Fatalf("unexpected start identity: %#v", plan)
	}
	if plan.ApprovalPolicy != "never" {
		t.Fatalf("approval policy = %q", plan.ApprovalPolicy)
	}
	if plan.SandboxMode != "danger-full-access" {
		t.Fatalf("sandbox mode = %q", plan.SandboxMode)
	}
	if got := plan.Record.Turn["maxModelSteps"]; got != float64(3) {
		t.Fatalf("maxModelSteps = %#v", got)
	}
	if got := plan.ThreadPatch["endpointFormat"]; got != "responses" {
		t.Fatalf("endpoint patch = %#v", got)
	}
	response := plan.Response()
	if response["threadId"] != "thread-a" || response["turnId"] != "turn_42" || response["userMessageItemId"] != "item_turn_42_user" {
		t.Fatalf("response mismatch: %#v", response)
	}
}

func TestBuildStartPlanMigratesIdleLegacyThreadPolicyOnce(t *testing.T) {
	plan := BuildStartPlan(StartPlanInput{
		ThreadID:                     "thread-legacy",
		TurnNumber:                   1,
		Prompt:                       "hello",
		ThreadApprovalPolicy:         "auto",
		ThreadSandboxMode:            "danger-full-access",
		RuntimeApprovalPolicy:        "on-request",
		RuntimeSandboxMode:           "workspace-write",
		ThreadExecutionPolicyVersion: 0,
		ThreadPolicyMigrationAllowed: true,
	})
	if plan.ApprovalPolicy != "on-request" || plan.SandboxMode != "workspace-write" {
		t.Fatalf("legacy policy should migrate to safe pair: %#v", plan)
	}
	if plan.ThreadPatch["executionPolicyVersion"] != float64(2) ||
		plan.ThreadPatch["approvalPolicy"] != "on-request" ||
		plan.ThreadPatch["sandboxMode"] != "workspace-write" {
		t.Fatalf("migration patch mismatch: %#v", plan.ThreadPatch)
	}
}

func TestBuildStartPlanRequestOverrideWinsLegacyMigration(t *testing.T) {
	plan := BuildStartPlan(StartPlanInput{
		ThreadID:                     "thread-legacy",
		TurnNumber:                   1,
		Prompt:                       "hello",
		RequestApprovalPolicy:        "auto",
		RequestSandboxMode:           "danger-full-access",
		ThreadApprovalPolicy:         "auto",
		ThreadSandboxMode:            "danger-full-access",
		RuntimeApprovalPolicy:        "on-request",
		RuntimeSandboxMode:           "workspace-write",
		ThreadExecutionPolicyVersion: 0,
		ThreadPolicyMigrationAllowed: true,
	})
	if plan.ApprovalPolicy != "auto" || plan.SandboxMode != "danger-full-access" {
		t.Fatalf("request override should win migration: %#v", plan)
	}
	if plan.ThreadPatch["executionPolicyVersion"] != float64(2) ||
		plan.ThreadPatch["approvalPolicy"] != "auto" ||
		plan.ThreadPatch["sandboxMode"] != "danger-full-access" {
		t.Fatalf("override migration patch mismatch: %#v", plan.ThreadPatch)
	}
}

func TestBuildStartPlanDoesNotMigrateUnsafeThreadState(t *testing.T) {
	plan := BuildStartPlan(StartPlanInput{
		ThreadID:                     "thread-pending",
		TurnNumber:                   1,
		Prompt:                       "hello",
		ThreadApprovalPolicy:         "auto",
		ThreadSandboxMode:            "danger-full-access",
		RuntimeApprovalPolicy:        "on-request",
		RuntimeSandboxMode:           "workspace-write",
		ThreadExecutionPolicyVersion: 0,
		ThreadPolicyMigrationAllowed: false,
	})
	if plan.ApprovalPolicy != "auto" || plan.SandboxMode != "danger-full-access" {
		t.Fatalf("unsafe thread state should retain existing resolved policy: %#v", plan)
	}
	if _, ok := plan.ThreadPatch["executionPolicyVersion"]; ok {
		t.Fatalf("unsafe thread state must not be rewritten: %#v", plan.ThreadPatch)
	}
}

func TestBuildStartPlanDoesNotTreatVersionedThreadAsUnversioned(t *testing.T) {
	for _, version := range []int{1, 3} {
		plan := BuildStartPlan(StartPlanInput{
			ThreadID:                     "thread-versioned",
			TurnNumber:                   1,
			Prompt:                       "hello",
			ThreadApprovalPolicy:         "auto",
			ThreadSandboxMode:            "danger-full-access",
			RuntimeApprovalPolicy:        "on-request",
			RuntimeSandboxMode:           "workspace-write",
			ThreadExecutionPolicyVersion: version,
			ThreadPolicyMigrationAllowed: true,
		})
		if plan.ApprovalPolicy != "auto" || plan.SandboxMode != "danger-full-access" {
			t.Fatalf("version %d must not be treated as an absent marker: %#v", version, plan)
		}
		if _, exists := plan.ThreadPatch["executionPolicyVersion"]; exists {
			t.Fatalf("version %d must not be rewritten: %#v", version, plan.ThreadPatch)
		}
	}
}

func TestResolveStartPoliciesFallBackToRuntimeDefault(t *testing.T) {
	if got := ResolveStartApprovalPolicy("bad", "", " on-request "); got != "on-request" {
		t.Fatalf("approval fallback = %q", got)
	}
	if got := ResolveStartSandboxMode("", "bad", " workspace-write "); got != "workspace-write" {
		t.Fatalf("sandbox fallback = %q", got)
	}
	if got := ResolveStartApprovalPolicy("bad", "also-bad", "nope"); got != "" {
		t.Fatalf("invalid approval should be empty: %q", got)
	}
}
