//go:build !analytix_prod

package runtimego

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRuntimeServerPlanModeContinuesAfterReadBatchAndCreatePlan(t *testing.T) {
	dataDir := workspacetest.New(t)
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	for index, size := range []int{40_419, 1_053, 171_808, 46_353, 55_310} {
		name := []string{"one.txt", "two.txt", "three.txt", "four.txt", "five.txt"}[index]
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(strings.Repeat("x", size)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const (
		firstReasoning  = "DEEPSEEK_PRIVATE_PLAN_READ_BATCH_FIRST"
		secondReasoning = "DEEPSEEK_PRIVATE_PLAN_READ_BATCH_SECOND"
	)
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + firstReasoning + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_one","type":"function","function":{"name":"read","arguments":"{\"path\":\"one.txt\"}"}},{"index":1,"id":"call_read_two","type":"function","function":{"name":"read","arguments":"{\"path\":\"two.txt\"}"}},{"index":2,"id":"call_read_three","type":"function","function":{"name":"read","arguments":"{\"path\":\"three.txt\"}"}},{"index":3,"id":"call_read_four","type":"function","function":{"name":"read","arguments":"{\"path\":\"four.txt\"}"}},{"index":4,"id":"call_read_five","type":"function","function":{"name":"read","arguments":"{\"path\":\"five.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + secondReasoning + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_plan_after_reads","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# Read batch plan\\n\\nImplement the focused change, preserve the ` + "`http://guides.github.com/overviews/forking/`" + ` target, and run the focused test.\",\"operation\":\"draft\",\"source_request\":\"Plan the focused change\",\"title\":\"Read batch plan\",\"plan_id\":\"plan-read-batch\",\"plan_relative_path\":\".analytixsdd/plan/read-batch.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"plan chain completed"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "plan-read-provider", "plan-read-model", "deepseek-chat-completions",
		),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Plan read batch",
		"workspace":  workspace,
		"providerId": "plan-read-provider",
		"model":      "plan-read-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": ".analytixsdd/plan/read-batch.md",
		"planId": "plan-read-batch", "sourceRequest": "Plan the focused change", "title": "Read batch plan",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Plan the focused change", "mode": "plan", "guiPlan": guiPlan,
		"approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("read batch and create_plan chain made %d provider calls, want 3", provider.RequestCount())
	}
	thirdBody := provider.Body(2)
	var thirdRequest struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal([]byte(thirdBody), &thirdRequest); err != nil {
		t.Fatalf("decode third provider request: %v", err)
	}
	if len(thirdRequest.Tools) < 2 || !bytes.Contains([]byte(thirdBody), []byte(`"name":"read"`)) ||
		!bytes.Contains([]byte(thirdBody), []byte(`"name":"create_plan"`)) ||
		bytes.Contains([]byte(thirdBody), []byte(`"name":"bash"`)) {
		t.Fatalf("satisfied plan continuation lost its stable read-only tool catalog with %d schemas", len(thirdRequest.Tools))
	}
	if strings.Count(thirdBody, firstReasoning) != 1 || strings.Count(thirdBody, secondReasoning) != 1 {
		t.Fatalf("third provider request did not replay both exact reasoning segments")
	}
	for _, toolName := range []string{"read", "create_plan"} {
		if !strings.Contains(thirdBody, `"name":"`+toolName+`"`) {
			t.Fatalf("third provider request lost %s tool pairing", toolName)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "read-batch.md")); err != nil {
		t.Fatalf("create_plan did not persist the reserved plan: %v", err)
	}
	assertRuntimeFilesExcludeValues(t, dataDir, firstReasoning, secondReasoning)
	assertRuntimeFilesExcludeValues(t, durableRoot, firstReasoning, secondReasoning)
}

func TestRuntimeServerPlanModeRejectsDuplicateCreatePlanBeforeOneRetry(t *testing.T) {
	const (
		rejectedPlanOne   = "REJECTED_DUPLICATE_PLAN_ONE"
		rejectedPlanTwo   = "REJECTED_DUPLICATE_PLAN_TWO"
		rejectedReasoning = "PRIVATE_REJECTED_DUPLICATE_PLAN_REASONING"
		acceptedReasoning = "PRIVATE_ACCEPTED_SINGLE_PLAN_REASONING"
		acceptedPlan      = "# Accepted single plan\n\nRun the focused verification."
	)
	dataDir := workspacetest.New(t)
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + rejectedReasoning + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"duplicate_plan_one","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"` + rejectedPlanOne + `\",\"operation\":\"draft\"}"}},{"index":1,"id":"duplicate_plan_two","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"` + rejectedPlanTwo + `\",\"operation\":\"draft\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + acceptedReasoning + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"accepted_single_plan","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# Accepted single plan\\n\\nRun the focused verification.\",\"operation\":\"draft\",\"source_request\":\"Plan the focused repair\",\"title\":\"Accepted single plan\",\"plan_id\":\"accepted-single-plan\",\"plan_relative_path\":\".analytixsdd/plan/accepted-single-plan.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"single plan saved"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "duplicate-plan-provider", "duplicate-plan-model", "deepseek-chat-completions",
		),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Duplicate create plan",
		"workspace":  workspace,
		"providerId": "duplicate-plan-provider",
		"model":      "duplicate-plan-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": ".analytixsdd/plan/accepted-single-plan.md",
		"planId": "accepted-single-plan", "sourceRequest": "Plan the focused repair", "title": "Accepted single plan",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Plan the focused repair", "mode": "plan", "guiPlan": guiPlan,
		"approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("duplicate rejection and single retry made %d provider calls, want 3", provider.RequestCount())
	}
	recoveryBody := provider.Body(1)
	if !strings.Contains(recoveryBody, "exactly one create_plan") || strings.Contains(recoveryBody, rejectedReasoning) ||
		strings.Contains(recoveryBody, rejectedPlanOne) || strings.Contains(recoveryBody, rejectedPlanTwo) {
		t.Fatalf("duplicate correction request was not fixed and data-free: %s", recoveryBody)
	}
	planPath := filepath.Join(workspace, ".analytixsdd", "plan", "accepted-single-plan.md")
	planBytes, err := os.ReadFile(planPath)
	if err != nil || string(planBytes) != acceptedPlan {
		t.Fatalf("exactly one corrected plan was not persisted source-exactly: content=%q err=%v", planBytes, err)
	}
	threadDetail := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	threadJSON := string(mustJSON(t, threadDetail))
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	for _, rejected := range []string{rejectedPlanOne, rejectedPlanTwo} {
		if strings.Contains(threadJSON, rejected) || strings.Contains(replay, rejected) {
			t.Fatalf("rejected duplicate plan arguments crossed a public persistence seam")
		}
	}
	assertRuntimeFilesExcludeValues(t, dataDir, rejectedPlanOne, rejectedPlanTwo, rejectedReasoning, acceptedReasoning)
	assertRuntimeFilesExcludeValues(t, durableRoot, rejectedPlanOne, rejectedPlanTwo, rejectedReasoning, acceptedReasoning)
}

func TestRuntimeServerPlanModeAllowsSequentialReadOnlyInvestigationBeforeCreatePlan(t *testing.T) {
	dataDir := workspacetest.New(t)
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"source.js":      "export const add = (left, right) => left - right\n",
		"source.test.js": "assert.equal(add(2, 3), 5)\n",
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"PRIVATE_SEQUENTIAL_READ_ONE"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_source","type":"function","function":{"name":"read","arguments":"{\"path\":\"source.js\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"reasoning_content":"PRIVATE_SEQUENTIAL_READ_TWO"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_test","type":"function","function":{"name":"read","arguments":"{\"path\":\"source.test.js\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"reasoning_content":"PRIVATE_SEQUENTIAL_CREATE_PLAN"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_plan_after_sequential_reads","type":"function","function":{"name":"create_plan","arguments":"{\"markdown\":\"# Sequential read plan\\n\\nFix the implementation and run the focused test.\",\"operation\":\"draft\",\"source_request\":\"Plan the focused repair\",\"title\":\"Sequential read plan\",\"plan_id\":\"plan-sequential-read\",\"plan_relative_path\":\".analytixsdd/plan/sequential-read.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"sequential plan completed"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "plan-sequential-provider", "plan-sequential-model", "deepseek-chat-completions",
		),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Sequential plan reads",
		"workspace":  workspace,
		"providerId": "plan-sequential-provider",
		"model":      "plan-sequential-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": ".analytixsdd/plan/sequential-read.md",
		"planId": "plan-sequential-read", "sourceRequest": "Plan the focused repair", "title": "Sequential read plan",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Read the implementation and its test before planning the focused repair.",
		"mode":   "plan", "guiPlan": guiPlan, "approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 4 {
		t.Fatalf("sequential read investigation made %d provider calls, want 4", provider.RequestCount())
	}
	for _, requestIndex := range []int{1, 2} {
		body := provider.Body(requestIndex)
		if !strings.Contains(body, `"name":"read"`) || !strings.Contains(body, `"name":"create_plan"`) ||
			strings.Contains(body, `"name":"bash"`) {
			t.Fatalf("unsatisfied plan request %d did not retain only bounded investigation tools", requestIndex+1)
		}
	}
	fourthBody := provider.Body(3)
	var fourthRequest struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal([]byte(fourthBody), &fourthRequest); err != nil {
		t.Fatalf("decode fourth provider request: %v", err)
	}
	if len(fourthRequest.Tools) < 2 || !bytes.Contains([]byte(fourthBody), []byte(`"name":"read"`)) ||
		!bytes.Contains([]byte(fourthBody), []byte(`"name":"create_plan"`)) ||
		bytes.Contains([]byte(fourthBody), []byte(`"name":"bash"`)) {
		t.Fatalf("satisfied sequential plan lost its stable read-only tool catalog with %d schemas", len(fourthRequest.Tools))
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "sequential-read.md")); err != nil {
		t.Fatalf("create_plan did not persist the sequential-read plan: %v", err)
	}
}

func TestRuntimeServerPlanModeContinuesAfterMaterializedCreatePlan(t *testing.T) {
	dataDir := workspacetest.New(t)
	durableRoot := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	for index, size := range []int{40_419, 1_053, 171_808, 46_353, 55_310} {
		name := []string{"one.txt", "two.txt", "three.txt", "four.txt", "five.txt"}[index]
		sourceLine := "def take(n, iterable):\n    \"\"\"Return a bounded result without consuming early.\"\"\"\n    return list(iterable)\nhttps://example.invalid/docs\n"
		content := strings.Repeat(sourceLine, size/len(sourceLine)+1)[:size]
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const (
		materializedReadReasoning = "DEEPSEEK_PRIVATE_MATERIALIZED_PLAN_READ"
		materializedPlanReasoning = "DEEPSEEK_PRIVATE_MATERIALIZED_PLAN_TEXT"
	)
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + materializedReadReasoning + `"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read_one","type":"function","function":{"name":"read","arguments":"{\"path\":\"one.txt\"}"}},{"index":1,"id":"call_read_two","type":"function","function":{"name":"read","arguments":"{\"path\":\"two.txt\"}"}},{"index":2,"id":"call_read_three","type":"function","function":{"name":"read","arguments":"{\"path\":\"three.txt\"}"}},{"index":3,"id":"call_read_four","type":"function","function":{"name":"read","arguments":"{\"path\":\"four.txt\"}"}},{"index":4,"id":"call_read_five","type":"function","function":{"name":"read","arguments":"{\"path\":\"five.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"reasoning_content":"` + materializedPlanReasoning + `"}}]}`,
			`data: {"choices":[{"delta":{"content":"# Read batch plan\n\nImplement the focused change and run the focused test."},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"MILESTONE_A_PLAN_OK"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        dataDir,
		ModelProvidersJSON: testModelProvidersJSONWithReasoningProtocol(
			provider.URL(), "materialized-plan-provider", "materialized-plan-model", "deepseek-chat-completions",
		),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Materialized plan read batch",
		"workspace":  workspace,
		"providerId": "materialized-plan-provider",
		"model":      "materialized-plan-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": ".analytixsdd/plan/materialized-read-batch.md",
		"planId": "materialized-plan-read-batch", "sourceRequest": "Plan the focused change", "title": "Materialized read batch plan",
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Plan the focused change", "mode": "plan", "guiPlan": guiPlan,
		"approvalPolicy": "auto", "sandboxMode": "workspace-write",
	}), http.StatusAccepted)

	if provider.RequestCount() != 3 {
		t.Fatalf("read batch and materialized create_plan chain made %d provider calls, want 3", provider.RequestCount())
	}
	thirdBody := provider.Body(2)
	var thirdRequest struct {
		Messages []struct {
			Role      string          `json:"role"`
			Content   any             `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		} `json:"messages"`
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal([]byte(thirdBody), &thirdRequest); err != nil {
		t.Fatalf("decode third provider request: %v", err)
	}
	if len(thirdRequest.Tools) < 2 || !bytes.Contains([]byte(thirdBody), []byte(`"name":"read"`)) ||
		!bytes.Contains([]byte(thirdBody), []byte(`"name":"create_plan"`)) ||
		bytes.Contains([]byte(thirdBody), []byte(`"name":"bash"`)) {
		t.Fatalf("satisfied materialized plan lost its stable read-only tool catalog with %d schemas", len(thirdRequest.Tools))
	}
	var safeHistory strings.Builder
	for _, message := range thirdRequest.Messages {
		if message.Role == "tool" || len(message.ToolCalls) != 0 {
			t.Fatalf("third provider request retained provider-private tool wire")
		}
		if content, ok := message.Content.(string); ok {
			safeHistory.WriteString(content)
		}
	}
	if strings.Count(safeHistory.String(), `"tool":"read"`) != 5 ||
		strings.Count(safeHistory.String(), `"tool":"create_plan"`) != 1 {
		t.Fatalf("third provider request lost the bounded semantic tool history")
	}
	if strings.Contains(thirdBody, materializedReadReasoning) || strings.Contains(thirdBody, materializedPlanReasoning) {
		t.Fatal("third provider request retained private reasoning after host plan materialization")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "materialized-read-batch.md")); err != nil {
		t.Fatalf("materialized create_plan did not persist the reserved plan: %v", err)
	}
	threadDetail := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if !strings.Contains(string(mustJSON(t, threadDetail)), "MILESTONE_A_PLAN_OK") {
		t.Fatal("materialized plan continuation did not reach its terminal provider marker")
	}
	receipts, dispositions := loadPendingWorkRecords(t, dataDir)
	statusByWorkID := map[string]string{}
	for _, disposition := range dispositions {
		statusByWorkID[disposition.WorkID] = disposition.Status
	}
	continuationCount := 0
	fiveMemberContinuation := false
	oneMemberContinuation := false
	for _, receipt := range receipts {
		if receipt.Kind != domainpendingwork.KindProviderContinuation {
			continue
		}
		continuationCount++
		if statusByWorkID[receipt.WorkID] != domainpendingwork.StatusCompleted {
			t.Fatalf("provider continuation was not completed: %s", statusByWorkID[receipt.WorkID])
		}
		fiveMemberContinuation = fiveMemberContinuation || len(receipt.GrantMembers) == 5
		oneMemberContinuation = oneMemberContinuation || len(receipt.GrantMembers) == 1
	}
	if continuationCount != 2 || !fiveMemberContinuation || !oneMemberContinuation {
		t.Fatalf("materialized plan did not close both exact provider continuations")
	}
	assertRuntimeFilesExcludeValues(t, dataDir, materializedReadReasoning, materializedPlanReasoning)
	assertRuntimeFilesExcludeValues(t, durableRoot, materializedReadReasoning, materializedPlanReasoning)
}
