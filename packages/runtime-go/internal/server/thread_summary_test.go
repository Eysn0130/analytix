package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	jobs "analytix.local/runtime-go/internal/jobs"
)

func TestThreadSummaryRoutesProjectGoRuntimeState(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(2 * time.Second) })
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title":     "Summary source",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create parent thread: %v", err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turnID := "turn_summary_1"
	bashCallID := serverTestHostToolCallID("thread-summary-bash")
	mcpCallID := serverTestHostToolCallID("thread-summary-mcp")
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id":        turnID,
		"threadId":  threadID,
		"status":    "completed",
		"createdAt": now,
		"items": []any{
			map[string]any{
				"id":        domaintoolcall.ToolCallItemIDV1(turnID, bashCallID),
				"turnId":    turnID,
				"threadId":  threadID,
				"kind":      "tool_call",
				"toolName":  "bash",
				"toolKind":  "command_execution",
				"callId":    bashCallID,
				"status":    "completed",
				"createdAt": now,
				"arguments": map[string]any{"command": "printf restarted", "cwd": workspace},
			},
			map[string]any{
				"id":         domaintoolresult.ToolResultItemIDV1(turnID, bashCallID),
				"turnId":     turnID,
				"threadId":   threadID,
				"kind":       "tool_result",
				"toolName":   "bash",
				"toolKind":   "command_execution",
				"callId":     bashCallID,
				"status":     "completed",
				"isError":    false,
				"createdAt":  now,
				"finishedAt": now,
				"output": domaintoolresult.PublicToolResultProjectionRecordV1(
					domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
				),
			},
			map[string]any{
				"id":        domaintoolcall.ToolCallItemIDV1(turnID, mcpCallID),
				"turnId":    turnID,
				"threadId":  threadID,
				"kind":      "tool_call",
				"toolName":  "mcp__docs__lookup",
				"callId":    mcpCallID,
				"status":    "completed",
				"createdAt": now,
				"arguments": map[string]any{"query": "summary"},
			},
		},
	}, "", nil); err != nil {
		t.Fatalf("append parent turn: %v", err)
	}

	child, err := handler.store.CreateThread(map[string]any{
		"title":     "Child agent: inspect",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create child thread: %v", err)
	}
	childID := stringField(child, "id")
	if _, err := handler.store.PatchThread(childID, map[string]any{"relation": "side"}); err != nil {
		t.Fatalf("mark child side thread: %v", err)
	}
	side, err := handler.store.ForkThread(threadID, map[string]any{"relation": "side", "title": "Side research"})
	if err != nil {
		t.Fatalf("create side chat thread: %v", err)
	}
	sideID := stringField(side, "id")
	subagent, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_summary",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolCallID: "call_subagent",
		ChildThreadID:    childID,
		ChildTurnID:      "turn_child_1",
		Kind:             "subagent",
		Label:            "Inspect source",
		ParallelIndex:    1,
		Status:           "completed",
		Model:            "deepseek-chat",
		ProviderID:       "anthropic-main",
		EndpointFormat:   "messages",
		Variant:          "claude-3-5-sonnet:202606",
		ModelSource:      "thread",
		ModelExecution: map[string]any{
			"providerId":            "anthropic-main",
			"modelId":               "deepseek-chat",
			"endpointFormat":        "messages",
			"variant":               "claude-3-5-sonnet:202606",
			"source":                "thread",
			"resolvedAt":            now,
			"capabilityFingerprint": strings.Repeat("b", 64),
		},
		ProfileName:           "reviewer",
		ToolPolicy:            "readOnly",
		DefaultModelInherited: true,
		Output:                "child summary",
		ToolInvocations:       2,
	})
	if err != nil {
		t.Fatalf("start subagent record: %v", err)
	}
	background, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_summary",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolCallID: "call_background",
		ChildThreadID:    childID,
		Kind:             "background-shell",
		Name:             "bash",
		Label:            "sleep 10",
		Status:           "running",
		Workspace:        workspace,
		Background:       true,
		Output:           "still running\n",
	})
	if err != nil {
		t.Fatalf("start background record: %v", err)
	}
	parallel, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_summary",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolCallID: "call_parallel",
		ChildThreadID:    childID,
		Kind:             "parallel_task",
		Label:            "Follow-up task",
		Status:           "completed",
		Output:           "parallel result",
	})
	if err != nil {
		t.Fatalf("start parallel task record: %v", err)
	}

	summary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary", nil, http.StatusOK)
	subagents := listAny(summary["subagents"])
	if len(subagents) == 0 || stringField(subagents[0].(map[string]any), "childThreadId") != childID {
		t.Fatalf("summary must expose child subagent thread: %#v", subagents)
	}
	if stringField(subagents[0].(map[string]any), "id") != "run:"+subagent.ID || subagents[0].(map[string]any)["canOpenThread"] != true {
		t.Fatalf("summary subagent must be openable and keyed by child run: %#v", subagents[0])
	}
	if subagents[0].(map[string]any)["canReadOutput"] != false || subagents[0].(map[string]any)["outputWithheld"] != true || subagents[0].(map[string]any)["factAnswerAllowed"] != false {
		t.Fatalf("summary subagent must be metadata-only and non-authoritative: %#v", subagents[0])
	}
	if stringField(subagents[0].(map[string]any), "displayName") != "Lagrange" ||
		stringField(subagents[0].(map[string]any), "agentNickname") != "Lagrange" ||
		stringField(subagents[0].(map[string]any), "title") != "Lagrange" ||
		stringField(subagents[0].(map[string]any), "label") != "Lagrange" {
		t.Fatalf("summary subagent display name mismatch: %#v", subagents[0])
	}
	if _, found := subagents[0].(map[string]any)["cacheHitRate"]; found {
		t.Fatalf("summary subagent must not duplicate cache authority from mutable job metadata: %#v", subagents[0])
	}
	if _, found := subagents[0].(map[string]any)["totalTokens"]; found {
		t.Fatalf("summary subagent must not duplicate token authority from mutable job metadata: %#v", subagents[0])
	}
	if stringField(subagents[0].(map[string]any), "providerId") != "anthropic-main" ||
		stringField(subagents[0].(map[string]any), "endpointFormat") != "messages" ||
		stringField(subagents[0].(map[string]any), "modelSource") != "thread" {
		t.Fatalf("summary subagent must expose provider execution metadata: %#v", subagents[0])
	}
	if _, found := subagents[0].(map[string]any)["modelExecution"]; found {
		t.Fatalf("summary subagent must not expose a parallel model-execution authority: %#v", subagents[0])
	}
	if stringField(subagents[0].(map[string]any), "variant") != "claude-3-5-sonnet:202606" {
		t.Fatalf("summary subagent variant metadata mismatch: %#v", subagents[0])
	}
	if len(subagents) != 1 {
		t.Fatalf("summary subagents must not include side chats or duplicate child threads: %#v", subagents)
	}
	sideChats := listAny(summary["sideChats"])
	if len(sideChats) != 1 || stringField(sideChats[0].(map[string]any), "threadId") != sideID {
		t.Fatalf("summary side chats must include only manual side chats: %#v", sideChats)
	}
	tasks := listAny(summary["tasks"])
	backgroundTask := assertThreadSummaryTask(t, tasks, "taskjob:"+background.ID, "taskJob", "active", false, false)
	if stringField(backgroundTask, "kind") != "background-shell" || stringField(backgroundTask, "status") != "running" ||
		backgroundTask["background"] != true || backgroundTask["terminal"] != false {
		t.Fatalf("background task lifecycle metadata mismatch: %#v", backgroundTask)
	}
	assertThreadSummaryTask(t, tasks, "command:"+bashCallID, "command", "done", false, false)
	parallelTask := assertThreadSummaryTask(t, tasks, "taskjob:"+parallel.ID, "taskJob", "done", false, false)
	if stringField(parallelTask, "kind") != "parallel_task" || stringField(parallelTask, "status") != "completed" ||
		parallelTask["background"] != false || parallelTask["terminal"] != true {
		t.Fatalf("parallel task projection mismatch: %#v", parallelTask)
	}
	if len(listAny(summary["outputs"])) != 0 {
		t.Fatalf("summary exposed legacy generated file or command output: %#v", summary["outputs"])
	}
	if len(listAny(summary["sources"])) != 0 {
		t.Fatalf("summary must not promote historical tool metadata into evidence sources: %#v", summary["sources"])
	}
	encodedSummary, _ := json.Marshal(summary)
	for _, childOutput := range []string{"child summary", "still running", "parallel result"} {
		if strings.Contains(string(encodedSummary), childOutput) {
			t.Fatalf("summary exposed historical child output %q: %s", childOutput, encodedSummary)
		}
	}

	output := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary/tasks/taskjob%3A"+background.ID+"/output?limit=5", nil, http.StatusNotFound)
	if stringField(output, "code") != "not_found" {
		t.Fatalf("legacy child output route must fail closed: %#v", output)
	}
	killed := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/summary/tasks/taskjob%3A"+background.ID+"/kill", nil, http.StatusNotFound)
	if stringField(killed, "code") != "not_found" {
		t.Fatalf("legacy child control route must fail closed: %#v", killed)
	}
	restarted := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/summary/tasks/command%3Acall_bash/restart", nil, http.StatusConflict)
	if stringField(restarted, "code") != "conflict" || stringField(restarted, "message") != "The request conflicts with the current runtime state." || strings.Contains(fmt.Sprint(restarted), "call_bash") {
		t.Fatalf("summary command restart must not recover private arguments: %#v", restarted)
	}
}

func TestThreadSummaryProjectsCanceledAndTimeoutTaskJobsAsTerminal(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title":     "Terminal task jobs",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	appendThreadSummaryParentTurn(t, handler.store, threadID, "turn_terminal_tasks")
	jobIDsByStatus := map[string]string{}
	for _, status := range []string{"canceled", "timeout"} {
		record, err := handler.jobs.StartChildRun(jobs.StartRequest{
			ParentGoalID:     "goal_terminal_tasks",
			ParentThreadID:   threadID,
			ParentTurnID:     "turn_terminal_tasks",
			ParentToolCallID: "call_" + status,
			Kind:             "background-shell",
			Name:             "bash",
			Label:            "sleep 20",
			Status:           status,
			Workspace:        workspace,
			Background:       true,
			Output:           status + " output",
			Error:            status + " reason",
		})
		if err != nil {
			t.Fatalf("start %s task job: %v", status, err)
		}
		if strings.TrimSpace(record.FinishedAt) == "" {
			t.Fatalf("%s task job should persist finishedAt: %#v", status, record)
		}
		jobIDsByStatus[status] = record.ID
	}

	summary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary", nil, http.StatusOK)
	tasks := listAny(summary["tasks"])
	for _, status := range []string{"canceled", "timeout"} {
		task := assertThreadSummaryTask(t, tasks, "taskjob:"+jobIDsByStatus[status], "taskJob", "terminal", false, false)
		if stringField(task, "status") != status || !boolField(task, "terminal") {
			t.Fatalf("%s task job should be terminal and not killable: %#v", status, task)
		}
	}
}

func TestThreadSummaryRecoversSubagentNameFromChildPrompt(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title":     "Summary source",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create parent thread: %v", err)
	}
	threadID := stringField(thread, "id")
	appendThreadSummaryParentTurn(t, handler.store, threadID, "turn_parent_generic")
	child, err := handler.store.CreateThread(map[string]any{
		"title":     "Child agent: task",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create child thread: %v", err)
	}
	childID := stringField(child, "id")
	if _, err := handler.store.PatchThread(childID, map[string]any{"relation": "side"}); err != nil {
		t.Fatalf("mark child side thread: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := handler.store.AppendTurnToThread(childID, map[string]any{
		"id":        "turn_child_prompt",
		"threadId":  childID,
		"status":    "completed",
		"createdAt": now,
		"items": []any{
			map[string]any{
				"id":        "item_child_prompt",
				"turnId":    "turn_child_prompt",
				"threadId":  childID,
				"kind":      "user_message",
				"text":      "Inspect model capability boundaries",
				"createdAt": now,
			},
		},
	}, "", nil); err != nil {
		t.Fatalf("append child prompt: %v", err)
	}
	record, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_generic",
		ParentThreadID:   threadID,
		ParentTurnID:     "turn_parent_generic",
		ParentToolCallID: "call_child_generic",
		ChildThreadID:    childID,
		Kind:             "subagent",
		Name:             "task",
		Label:            "task",
		Status:           "completed",
		ProfileName:      "reviewer",
		ParallelIndex:    1,
		Output:           "done",
	})
	if err != nil {
		t.Fatalf("start generic subagent record: %v", err)
	}

	summary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary", nil, http.StatusOK)
	subagents := listAny(summary["subagents"])
	if len(subagents) != 1 {
		t.Fatalf("summary subagents mismatch: %#v", subagents)
	}
	subagent := subagents[0].(map[string]any)
	displayName := stringField(subagent, "displayName")
	if stringField(subagent, "id") != "run:"+record.ID ||
		displayName == "" ||
		displayName != "Lagrange" ||
		displayName == "task" ||
		displayName == "reviewer" ||
		displayName == "Inspect model capability boundaries" ||
		stringField(subagent, "agentNickname") != displayName ||
		stringField(subagent, "title") != displayName ||
		stringField(subagent, "label") != displayName {
		t.Fatalf("summary must generate a nickname instead of profile or child prompt: %#v", subagent)
	}
}

func TestThreadSummaryExposesHistoricalChildTaskJobsAsSubagents(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title":     "Historical task parent",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create parent thread: %v", err)
	}
	threadID := stringField(thread, "id")
	appendThreadSummaryParentTurn(t, handler.store, threadID, "turn_historical_task")
	child, err := handler.store.CreateThread(map[string]any{
		"title":          "Child agent: design-reviewer",
		"workspace":      workspace,
		"model":          "deepseek-chat",
		"mode":           "agent",
		"relation":       "side",
		"parentThreadId": threadID,
	}, workspace)
	if err != nil {
		t.Fatalf("create child thread: %v", err)
	}
	childID := stringField(child, "id")
	record, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_historical_task",
		ParentThreadID:   threadID,
		ParentTurnID:     "turn_historical_task",
		ParentToolCallID: "call_historical_task",
		ChildThreadID:    childID,
		Kind:             "parallel_task",
		Name:             "design-reviewer",
		Label:            "基于目录结构和媒体文件信息生成摘要",
		Status:           "completed",
		ProfileName:      "design-reviewer",
		ParallelIndex:    2,
		Output:           "done",
	})
	if err != nil {
		t.Fatalf("start historical child task record: %v", err)
	}

	summary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary", nil, http.StatusOK)
	subagents := listAny(summary["subagents"])
	if len(subagents) != 1 {
		t.Fatalf("summary subagents mismatch: %#v", subagents)
	}
	subagent := subagents[0].(map[string]any)
	if stringField(subagent, "id") != "run:"+record.ID ||
		stringField(subagent, "childThreadId") != childID ||
		stringField(subagent, "displayName") != "Nash" ||
		stringField(subagent, "agentNickname") != "Nash" ||
		floatFromAny(subagent["parallelIndex"]) != 2 {
		t.Fatalf("summary must expose historical child task job with generated nickname: %#v", subagent)
	}
}

func TestThreadSummaryDoesNotUsePromptLikeChildName(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title":     "Summary source",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create parent thread: %v", err)
	}
	threadID := stringField(thread, "id")
	appendThreadSummaryParentTurn(t, handler.store, threadID, "turn_parent_prompt_name")
	child, err := handler.store.CreateThread(map[string]any{
		"title":     "Child agent: task",
		"workspace": workspace,
		"model":     "deepseek-chat",
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create child thread: %v", err)
	}
	childID := stringField(child, "id")
	if _, err := handler.store.PatchThread(childID, map[string]any{"relation": "side"}); err != nil {
		t.Fatalf("mark child side thread: %v", err)
	}
	record, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_prompt_name",
		ParentThreadID:   threadID,
		ParentTurnID:     "turn_parent_prompt_name",
		ParentToolCallID: "call_child_prompt_name",
		ChildThreadID:    childID,
		Kind:             "subagent",
		Name:             "分析桌面文件",
		Label:            "task",
		Status:           "completed",
		Output:           "done",
	})
	if err != nil {
		t.Fatalf("start prompt-name subagent record: %v", err)
	}

	summary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/summary", nil, http.StatusOK)
	subagents := listAny(summary["subagents"])
	if len(subagents) != 1 {
		t.Fatalf("summary subagents mismatch: %#v", subagents)
	}
	subagent := subagents[0].(map[string]any)
	displayName := stringField(subagent, "displayName")
	if stringField(subagent, "id") != "run:"+record.ID || displayName == "" || displayName == "task" || displayName == "分析桌面文件" {
		t.Fatalf("summary must generate a nickname instead of task prompt text: %#v", subagent)
	}
}

func TestThreadSummaryRestartGuardsPolicyAndWorkspace(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := t.TempDir()
	policyThread, err := handler.store.CreateThread(map[string]any{
		"title":          "Policy blocked",
		"workspace":      workspace,
		"model":          "deepseek-chat",
		"mode":           "agent",
		"approvalPolicy": "never",
		"sandboxMode":    "workspace-write",
	}, workspace)
	if err != nil {
		t.Fatalf("create policy thread: %v", err)
	}
	policyThreadID := stringField(policyThread, "id")
	policyCallID := appendThreadSummaryCommandTurn(t, handler, policyThreadID, workspace, "printf blocked")
	policySummary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+policyThreadID+"/summary", nil, http.StatusOK)
	assertThreadSummaryTask(t, listAny(policySummary["tasks"]), "command:"+policyCallID, "command", "done", false, false)
	policyDenied := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+policyThreadID+"/summary/tasks/command%3A"+policyCallID+"/restart", nil, http.StatusConflict)
	if stringField(policyDenied, "message") != "The request conflicts with the current runtime state." || strings.Contains(fmt.Sprint(policyDenied), "printf blocked") {
		t.Fatalf("private command arguments were recovered for restart: %#v", policyDenied)
	}

	outsideThread, err := handler.store.CreateThread(map[string]any{
		"title":          "Outside blocked",
		"workspace":      workspace,
		"model":          "deepseek-chat",
		"mode":           "agent",
		"approvalPolicy": "auto",
		"sandboxMode":    "workspace-write",
	}, workspace)
	if err != nil {
		t.Fatalf("create outside thread: %v", err)
	}
	outsideThreadID := stringField(outsideThread, "id")
	outsideCallID := appendThreadSummaryCommandTurn(t, handler, outsideThreadID, t.TempDir(), "printf outside")
	outsideSummary := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+outsideThreadID+"/summary", nil, http.StatusOK)
	assertThreadSummaryTask(t, listAny(outsideSummary["tasks"]), "command:"+outsideCallID, "command", "done", false, false)
	outsideDenied := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+outsideThreadID+"/summary/tasks/command%3A"+outsideCallID+"/restart", nil, http.StatusConflict)
	if stringField(outsideDenied, "message") != "The request conflicts with the current runtime state." || strings.Contains(fmt.Sprint(outsideDenied), "printf outside") {
		t.Fatalf("private outside-workspace arguments were recovered for restart: %#v", outsideDenied)
	}
}

func appendThreadSummaryCommandTurn(t *testing.T, handler *runtimeServerHandler, threadID string, cwd string, command string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turnID := "turn_restart"
	callID := serverTestHostToolCallID("thread-summary-restart:" + threadID)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id":        turnID,
		"threadId":  threadID,
		"status":    "completed",
		"createdAt": now,
		"items": []any{
			map[string]any{
				"id":        domaintoolcall.ToolCallItemIDV1(turnID, callID),
				"turnId":    turnID,
				"threadId":  threadID,
				"kind":      "tool_call",
				"toolName":  "bash",
				"toolKind":  "command_execution",
				"callId":    callID,
				"status":    "completed",
				"createdAt": now,
				"arguments": map[string]any{"command": command, "cwd": cwd},
			},
			map[string]any{
				"id":         domaintoolresult.ToolResultItemIDV1(turnID, callID),
				"turnId":     turnID,
				"threadId":   threadID,
				"kind":       "tool_result",
				"toolName":   "bash",
				"toolKind":   "command_execution",
				"callId":     callID,
				"status":     "completed",
				"isError":    false,
				"createdAt":  now,
				"finishedAt": now,
				"output": domaintoolresult.PublicToolResultProjectionRecordV1(
					domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
				),
			},
		},
	}, "", nil); err != nil {
		t.Fatalf("append command turn: %v", err)
	}
	return callID
}

func appendThreadSummaryParentTurn(t *testing.T, store *DurableEventSessionStore, threadID, turnID string) {
	t.Helper()
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed", "items": []any{},
	}, "", nil); err != nil {
		t.Fatalf("append summary parent turn: %v", err)
	}
}

func requestThreadSummaryJSON(t *testing.T, baseURL string, method string, path string, body io.Reader, status int) map[string]any {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != status {
		t.Fatalf("status for %s %s = %d want %d body=%s", method, path, response.StatusCode, status, string(data))
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode response: %v\n%s", err, string(data))
	}
	return decoded
}

func assertThreadSummaryStoredChildEvent(t *testing.T, handler *runtimeServerHandler, threadID string, childStatus string, childRunID string) map[string]any {
	t.Helper()
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("load stored events: %v", err)
	}
	for _, event := range replay.Events {
		child, _ := event["child"].(map[string]any)
		if child == nil {
			continue
		}
		if stringField(child, "childStatus") == childStatus && stringField(child, "childRunId") == childRunID {
			return event
		}
	}
	t.Fatalf("missing stored child event status=%s childRunId=%s in %#v", childStatus, childRunID, replay.Events)
	return nil
}

func assertThreadSummaryTask(t *testing.T, tasks []any, id string, source string, status string, canRestart bool, canKill bool) map[string]any {
	t.Helper()
	task := findThreadSummaryTask(t, tasks, id)
	if canRestart || canKill {
		t.Fatalf("closed task summary cannot expose restart/kill authority: id=%s", id)
	}
	allowed := map[string]struct{}{
		"schemaVersion": {}, "id": {}, "kind": {}, "status": {}, "background": {}, "terminal": {},
		"outputWithheld": {}, "outputTrustStatus": {}, "factAnswerAllowed": {}, "evidenceAuthority": {},
		"canReadOutput": {}, "canContinueParent": {},
	}
	if source == "command" {
		allowed["active"] = struct{}{}
	}
	if len(task) != len(allowed) {
		t.Fatalf("task %s must use exact closed metadata root: %#v", id, task)
	}
	for key := range task {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("task %s exposed forbidden field %q: %#v", id, key, task)
		}
	}
	if task["schemaVersion"] != float64(1) || stringField(task, "id") != id ||
		task["outputWithheld"] != true || task["factAnswerAllowed"] != false || task["evidenceAuthority"] != false ||
		task["canReadOutput"] != false || task["canContinueParent"] != false {
		t.Fatalf("task %s closed authority metadata mismatch: %#v", id, task)
	}
	if source == "command" {
		if stringField(task, "kind") != "command" || stringField(task, "outputTrustStatus") != "private_tool_output" || task["background"] != false {
			t.Fatalf("command task %s metadata mismatch: %#v", id, task)
		}
	} else if source == "taskJob" {
		if stringField(task, "kind") == "command" || stringField(task, "outputTrustStatus") != "untrusted_child_output" {
			t.Fatalf("task-job %s metadata mismatch: %#v", id, task)
		}
	} else {
		t.Fatalf("unknown task source expectation %q", source)
	}
	switch status {
	case "active":
		if boolField(task, "terminal") {
			t.Fatalf("task %s should remain active metadata: %#v", id, task)
		}
	case "done":
		if !boolField(task, "terminal") {
			t.Fatalf("task %s should be completed metadata: %#v", id, task)
		}
	case "terminal":
		if !boolField(task, "terminal") {
			t.Fatalf("task %s should be terminal metadata: %#v", id, task)
		}
	default:
		t.Fatalf("unknown task status expectation %q", status)
	}
	return task
}

func findThreadSummaryTask(t *testing.T, tasks []any, id string) map[string]any {
	t.Helper()
	for _, raw := range tasks {
		task, _ := raw.(map[string]any)
		if stringField(task, "id") == id {
			return task
		}
	}
	ids := []string{}
	for _, raw := range tasks {
		task, _ := raw.(map[string]any)
		ids = append(ids, stringField(task, "id"))
	}
	t.Fatalf("task %s not found in %s", id, strings.Join(ids, ", "))
	return nil
}

func waitForThreadSummaryTaskTerminal(t *testing.T, baseURL string, threadID string, taskID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		summary := requestThreadSummaryJSON(t, baseURL, http.MethodGet, "/v1/threads/"+threadID+"/summary", nil, http.StatusOK)
		task := findThreadSummaryTask(t, listAny(summary["tasks"]), taskID)
		if boolField(task, "terminal") {
			// Terminal status is written just before the background child emits
			// its final parent progress item. Give that synchronous cleanup path
			// a brief chance to finish before the test TempDir is removed.
			time.Sleep(25 * time.Millisecond)
			return task
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for summary task %s to settle: %#v", taskID, task)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
