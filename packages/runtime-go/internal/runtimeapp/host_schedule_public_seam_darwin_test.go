//go:build darwin

package runtimeapp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

const (
	hostSchedulePublicToolV1       = "mcp__gui_schedule__gui_schedule_list"
	hostScheduleReadToolV1         = "read_file"
	hostScheduleTestSecretV1       = "schedule-test-secret-v1"
	hostScheduleFirstFinalV1       = "SCHEDULE_LIST_HTTP_OK"
	hostScheduleSecondFinalV1      = "GENERAL_AGENT_HTTP_OK"
	hostScheduleOrdinaryFileBodyV1 = "ordinary_file_ok_v1"
)

func TestRuntimeHTTPHostScheduleListUsesExactContainedLoopback(t *testing.T) {
	node := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_SCHEDULE_MCP_NODE_V1"))
	entrypoint := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_SCHEDULE_MCP_ENTRYPOINT_V1"))
	if node == "" || entrypoint == "" {
		t.Skip("formal schedule MCP build inputs are not configured")
	}
	hostScheduleRequireExecutableV1(t, node)
	hostScheduleRequireRegularFileV1(t, entrypoint)

	var scheduleCalls atomic.Int64
	var scheduleRequestValid atomic.Bool
	scheduleBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<10))
		scheduleCalls.Add(1)
		valid := r.Method == http.MethodPost && r.URL.Path == "/schedule/internal/list" &&
			r.Header.Get("Authorization") == "Bearer "+hostScheduleTestSecretV1 &&
			string(body) == "{}"
		scheduleRequestValid.Store(valid)
		w.Header().Set("Content-Type", "application/json")
		if !valid {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tasks": []any{}})
	}))
	defer scheduleBackend.Close()

	scheduleSpec := domainmcp.ServerSpec{
		ID: "gui_schedule", Transport: "stdio", Command: node,
		Args: []string{
			entrypoint,
			"--gui-schedule-mcp-server",
			"--base-url",
			scheduleBackend.URL,
			"--secret",
			hostScheduleTestSecretV1,
		},
		Env:        map[string]string{"ELECTRON_RUN_AS_NODE": "1"},
		TrustScope: "user", TimeoutMS: 5000,
	}
	configBytes, err := json.Marshal(map[string]any{
		"capabilities": map[string]any{
			"mcp": map[string]any{
				"enabled": true,
				"servers": map[string]any{
					"gui_schedule": map[string]any{
						"enabled": true, "transport": scheduleSpec.Transport,
						"command": scheduleSpec.Command, "args": scheduleSpec.Args,
						"env": scheduleSpec.Env, "trustScope": scheduleSpec.TrustScope,
						"timeoutMs": scheduleSpec.TimeoutMS,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var providerCalls atomic.Int64
	var firstCatalogValid atomic.Bool
	var scheduleResultReturned atomic.Bool
	var ordinaryResultReturned atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		call := providerCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch call {
		case 1:
			firstCatalogValid.Store(
				bytes.Contains(body, []byte(hostSchedulePublicToolV1)) &&
					bytes.Contains(body, []byte(`"name":"`+hostScheduleReadToolV1+`"`)),
			)
			hostScheduleProviderSSEV1(w,
				`{"tool_calls":[{"index":0,"id":"call_schedule_public_seam","type":"function","function":{"name":"mcp__gui_schedule__gui_schedule_list","arguments":"{}"}}]}`,
				"tool_calls",
			)
		case 2:
			scheduleResultReturned.Store(hostScheduleProviderResultValidV1(
				body,
				"No scheduled tasks are configured.",
			))
			hostScheduleProviderSSEV1(w, `{"content":"`+hostScheduleFirstFinalV1+`"}`, "stop")
		case 3:
			hostScheduleProviderSSEV1(w,
				`{"tool_calls":[{"index":0,"id":"call_general_read_public_seam","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"ordinary.txt\"}"}}]}`,
				"tool_calls",
			)
		case 4:
			ordinaryResultReturned.Store(bytes.Contains(body, []byte(hostScheduleOrdinaryFileBodyV1)))
			hostScheduleProviderSSEV1(w, `{"content":"`+hostScheduleSecondFinalV1+`"}`, "stop")
		default:
			hostScheduleProviderSSEV1(w, `{"content":"unexpected provider request"}`, "stop")
		}
	}))
	defer provider.Close()

	workspace := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspace, "ordinary.txt"),
		[]byte(hostScheduleOrdinaryFileBodyV1+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	binding := scheduleSpec
	binding.Args = append([]string(nil), scheduleSpec.Args...)
	binding.Env = map[string]string{"ELECTRON_RUN_AS_NODE": "1"}
	handler, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(),
		DataDir: t.TempDir(), UserDataDir: t.TempDir(),
		ProtectedReadDirs: []string{t.TempDir()},
		ProviderID:        "schedule-public-provider", BaseURL: provider.URL + "/v1",
		APIKey: "test-only", Model: "schedule-public-model",
		EndpointFormat: "chat_completions", MCPConfigJSON: string(configBytes),
		HostScheduleMCPServer: &binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		server.Close()
		shutdownOwnedRuntimeHandler(t, handler)
	}()

	status, tools := hostScheduleRuntimeHTTPV1(
		t, server.URL, http.MethodGet, "/v1/runtime/tools?refresh=1", nil,
	)
	if status != http.StatusOK || !hostScheduleDiagnosticsValidV1(tools) {
		t.Fatalf("host schedule tool was not connected through runtime HTTP: status=%d", status)
	}
	status, thread := hostScheduleRuntimeHTTPV1(t, server.URL, http.MethodPost, "/v1/threads", map[string]any{
		"title": "schedule public seam", "workspace": workspace,
		"providerId": "schedule-public-provider", "model": "schedule-public-model",
		"mode": "agent",
	})
	threadID := hostScheduleStringFieldV1(thread, "id")
	if status != http.StatusCreated || threadID == "" {
		t.Fatalf("create thread status=%d", status)
	}

	status, firstTurn := hostScheduleRuntimeHTTPV1(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
			"prompt":     "Call mcp__gui_schedule__gui_schedule_list exactly once and report the result.",
			"providerId": "schedule-public-provider", "model": "schedule-public-model",
			"approvalPolicy": "never", "sandboxMode": "read-only", "disableUserInput": true,
		},
	)
	firstTurnID := hostScheduleStringFieldV1(firstTurn, "turnId")
	if status != http.StatusAccepted || firstTurnID == "" {
		t.Fatalf("start schedule turn status=%d", status)
	}
	hostScheduleWaitForTurnV1(t, server.URL, threadID, firstTurnID, hostScheduleFirstFinalV1)

	status, secondTurn := hostScheduleRuntimeHTTPV1(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
			"prompt":     "Read ordinary.txt with read_file and report the exact marker.",
			"providerId": "schedule-public-provider", "model": "schedule-public-model",
			"approvalPolicy": "never", "sandboxMode": "read-only", "disableUserInput": true,
		},
	)
	secondTurnID := hostScheduleStringFieldV1(secondTurn, "turnId")
	if status != http.StatusAccepted || secondTurnID == "" {
		t.Fatalf("start ordinary turn status=%d", status)
	}
	hostScheduleWaitForTurnV1(t, server.URL, threadID, secondTurnID, hostScheduleSecondFinalV1)

	healthStatus, _ := hostScheduleRuntimeHTTPV1(t, server.URL, http.MethodGet, "/health", nil)
	if providerCalls.Load() != 4 || !firstCatalogValid.Load() ||
		!scheduleResultReturned.Load() || !ordinaryResultReturned.Load() ||
		scheduleCalls.Load() != 1 || !scheduleRequestValid.Load() ||
		healthStatus != http.StatusOK {
		t.Fatalf(
			"schedule/additive public seam incomplete: provider_calls=%d catalog=%t schedule_result=%t ordinary_result=%t schedule_calls=%d schedule_request=%t health=%d",
			providerCalls.Load(), firstCatalogValid.Load(), scheduleResultReturned.Load(),
			ordinaryResultReturned.Load(), scheduleCalls.Load(), scheduleRequestValid.Load(), healthStatus,
		)
	}
}

func hostScheduleRequireExecutableV1(t *testing.T, path string) {
	t.Helper()
	hostScheduleRequireRegularFileV1(t, path)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatal("formal schedule MCP node path is not executable")
	}
}

func hostScheduleRequireRegularFileV1(t *testing.T, path string) {
	t.Helper()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("formal schedule MCP build path is not canonical")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("formal schedule MCP build path is not a regular file")
	}
}

func hostScheduleDiagnosticsValidV1(body []byte) bool {
	var decoded struct {
		MCPServers []struct {
			ID                          string `json:"id"`
			Status                      string `json:"status"`
			Available                   bool   `json:"available"`
			Connected                   bool   `json:"connected"`
			ToolCount                   int    `json:"toolCount"`
			ToolContractQuarantineCount int    `json:"toolContractQuarantineCount"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(body, &decoded) != nil || len(decoded.MCPServers) != 1 {
		return false
	}
	server := decoded.MCPServers[0]
	return server.ID == "gui_schedule" && server.Status == "connected" &&
		server.Available && server.Connected && server.ToolCount == 8 &&
		server.ToolContractQuarantineCount == 0
}

func hostScheduleProviderSSEV1(w io.Writer, delta string, finishReason string) {
	_, _ = io.WriteString(w, strings.Join([]string{
		`data: {"choices":[{"delta":` + delta + `,"finish_reason":"` + finishReason + `"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":16,"completion_tokens":4,"total_tokens":20}}`,
		"data: [DONE]",
	}, "\n\n"))
}

func hostScheduleProviderResultValidV1(body []byte, expectedMessage string) bool {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil {
		return false
	}
	for index := len(request.Messages) - 1; index >= 0; index-- {
		message := request.Messages[index]
		if message.Role != "tool" {
			continue
		}
		var output map[string]any
		if json.Unmarshal([]byte(message.Content), &output) != nil ||
			hostScheduleStringValueV1(output["transportStatus"]) != "success" ||
			hostScheduleStringValueV1(output["semanticStatus"]) != "success" {
			return false
		}
		result, _ := output["result"].(map[string]any)
		structured, _ := result["structuredContent"].(map[string]any)
		return hostScheduleStringValueV1(result["text"]) == expectedMessage &&
			hostScheduleStringValueV1(structured["status"]) == "success" &&
			hostScheduleStringValueV1(structured["message"]) == expectedMessage
	}
	return false
}

func hostScheduleStringValueV1(value any) string {
	text, _ := value.(string)
	return text
}

func hostScheduleWaitForTurnV1(
	t *testing.T,
	baseURL string,
	threadID string,
	turnID string,
	finalMarker string,
) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		status, events := hostScheduleRuntimeHTTPV1(
			t, baseURL, http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil,
		)
		if status == http.StatusOK && hostScheduleTargetTurnCompleteV1(events, turnID, finalMarker) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for schedule/additive HTTP turn completion")
}

func hostScheduleTargetTurnCompleteV1(events []byte, turnID string, finalMarker string) bool {
	markerObserved := false
	completionObserved := false
	for _, line := range strings.Split(string(events), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		event := map[string]any{}
		if json.Unmarshal([]byte(payload), &event) != nil ||
			hostScheduleStringValueV1(event["turnId"]) != turnID {
			continue
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			continue
		}
		markerObserved = markerObserved || bytes.Contains(encoded, []byte(finalMarker))
		completionObserved = completionObserved ||
			hostScheduleEventCompletesTurnV1(event, turnID)
	}
	return markerObserved && completionObserved
}

func hostScheduleEventCompletesTurnV1(event map[string]any, turnID string) bool {
	if hostScheduleStringValueV1(event["kind"]) == "turn_completed" {
		return true
	}
	nested, _ := event["events"].([]any)
	for _, value := range nested {
		child, _ := value.(map[string]any)
		if hostScheduleStringValueV1(child["turnId"]) == turnID &&
			hostScheduleStringValueV1(child["kind"]) == "turn_completed" {
			return true
		}
	}
	return false
}

func hostScheduleRuntimeHTTPV1(
	t *testing.T,
	baseURL string,
	method string,
	path string,
	body map[string]any,
) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}

func hostScheduleStringFieldV1(body []byte, field string) string {
	var decoded map[string]any
	if json.Unmarshal(body, &decoded) != nil {
		return ""
	}
	value, _ := decoded[field].(string)
	return value
}
