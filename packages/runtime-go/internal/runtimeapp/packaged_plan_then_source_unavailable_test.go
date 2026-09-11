package runtimeapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
)

const runtimeOptionalDomainPrivateCanary = "R129_SYNTHETIC_PRIVATE_DOMAIN_CANARY"

type packagedPlanThenProtectedReadbackV1 struct {
	hydration packagedSourceUnavailableHydrationSnapshotV1
	turn      map[string]any
}

func TestRuntimeHTTPPlanToolThenAsyncProtectedPromptPublishesSourceUnavailableAndHydratesAcrossRestart(t *testing.T) {
	runRuntimePlanThenProtectedOrdinaryRestartV1(t, nil)
}

func runRuntimePlanThenProtectedOrdinaryRestartV1(t *testing.T, prepare func(*testing.T, Config), configure ...func(*Config)) (Config, string, []string) {
	return runRuntimePlanThenProtectedOrdinaryRestartWithObservationV1(t, prepare, nil, configure...)
}

func runRuntimePlanThenProtectedOrdinaryRestartWithObservationV1(t *testing.T, prepare func(*testing.T, Config), observe func(*testing.T, *http.Client, string), configure ...func(*Config)) (Config, string, []string) {
	t.Helper()
	return runRuntimePlanThenProtectedOrdinaryRestartWithCheckpointsV1(t, prepare, observe, nil, configure...)
}

func runRuntimePlanThenProtectedOrdinaryRestartWithCheckpointsV1(t *testing.T, prepare func(*testing.T, Config), observe func(*testing.T, *http.Client, string), checkpoint func(string), configure ...func(*Config)) (Config, string, []string) {
	t.Helper()
	if checkpoint == nil {
		checkpoint = func(string) {}
	}
	const (
		planPrompt         = "Work only in this pre-existing isolated non-case code repository and inspect the task through tools. The user request is: Update Express content-type normalization so the reserved HTTP quality parameter name is handled case-insensitively. Add a focused regression test covering both lowercase and uppercase quality parameter names while preserving ordinary parameters, then run the focused real project test. Call read exactly once for each contract-required task inspection path, with only its relative path and no optional offset or limit: \"lib/utils.js\", \"test/utils.js\", \"package.json\". Do not read the contract-bound long-context file in this planning turn; the acceptance harness validates it in a dedicated later continuation. Use no tools other than those exact read calls and one create_plan call. Do not call list, search, code index, web fetch, bash, Todo, subagent/delegation, skill, MCP, or read any other path. Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call. Complete those exact read calls before recording the plan. Call create_plan exactly once after those reads complete. Record a concrete implementation and verification plan at the GUI-reserved plan path. After create_plan returns successfully, end the turn immediately without another tool call. If every required read succeeds, do not send a final response or the completion marker until create_plan has returned successfully. In the final response include the exact marker MILESTONE_A_PLAN_OK. Do not edit files, run the test, delegate, or run /compact in this planning turn."
		planPath           = ".analytixsdd/plan/async-protected-follow-up.md"
		planBody           = "# Focused plan\n\nHandle the reserved HTTP quality parameter case-insensitively, add the focused regression, and report any test failure."
		planMarker         = "MILESTONE_A_PLAN_OK"
		ordinaryReadPath   = "acceptance-context.txt"
		ordinaryReadMarker = "MILESTONE_A_POST_DENIAL_ORDINARY_OK"
	)
	var providerCalls atomic.Int64
	var ordinaryReadObserved atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read synthetic provider request: %v", err)
			return
		}
		if bytes.Contains(requestBody, []byte(runtimeOptionalDomainPrivateCanary)) {
			t.Error("private domain canary reached model-visible request")
			return
		}
		call := providerCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch call {
		case 1:
			arguments, err := json.Marshal(map[string]any{
				"markdown": planBody, "operation": "draft", "source_request": planPrompt,
				"title": "Focused plan", "plan_id": "plan-async-protected-follow-up",
				"plan_relative_path": planPath,
			})
			if err != nil {
				t.Errorf("encode create_plan arguments: %v", err)
				return
			}
			chunk, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_plan_async_protected_follow_up", "type": "function",
						"function": map[string]any{"name": "create_plan", "arguments": string(arguments)},
					}}},
					"finish_reason": "tool_calls",
				}},
			})
			if err != nil {
				t.Errorf("encode create_plan chunk: %v", err)
				return
			}
			_, _ = w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
		case 2:
			chunk, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"content": planMarker}, "finish_reason": "stop",
				}},
			})
			if err != nil {
				t.Errorf("encode plan final chunk: %v", err)
				return
			}
			_, _ = w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
		case 3:
			arguments, err := json.Marshal(map[string]any{"path": ordinaryReadPath, "limit": 1})
			if err != nil {
				t.Errorf("encode ordinary read arguments: %v", err)
				return
			}
			chunk, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_post_denial_ordinary_read", "type": "function",
						"function": map[string]any{"name": "read", "arguments": string(arguments)},
					}}},
					"finish_reason": "tool_calls",
				}},
			})
			if err != nil {
				t.Errorf("encode ordinary read chunk: %v", err)
				return
			}
			_, _ = w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
		case 4:
			request := map[string]any{}
			if err := json.Unmarshal(requestBody, &request); err != nil {
				t.Errorf("decode ordinary continuation request: %v", err)
				return
			}
			messages, _ := request["messages"].([]any)
			for _, raw := range messages {
				message, _ := raw.(map[string]any)
				if contracts.StringField(message, "role") == "tool" &&
					strings.Contains(contracts.StringField(message, "content"), ordinaryReadMarker) {
					ordinaryReadObserved.Store(true)
				}
			}
			chunk, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"content": ordinaryReadMarker}, "finish_reason": "stop",
				}},
			})
			if err != nil {
				t.Errorf("encode ordinary read final chunk: %v", err)
				return
			}
			_, _ = w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
		default:
			t.Errorf("unexpected provider call after protected follow-up: call=%d", call)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer provider.Close()

	root := t.TempDir()
	workspace := filepath.Join(root, "ordinary-workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ordinaryReadPath), []byte(ordinaryReadMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: filepath.Join(root, "durable"),
		DataDir: filepath.Join(root, "runtime-data"), UserDataDir: filepath.Join(root, "user-data"),
		ProviderID: "plan-protected-follow-up-provider", BaseURL: provider.URL + "/v1", APIKey: "test-only",
		Model: "plan-protected-follow-up-model", EndpointFormat: "chat_completions",
	}
	for _, configureRuntime := range configure {
		configureRuntime(&config)
	}
	seedProviderRegistryExecutionAuthorityV1(
		t, config.DataDir, config.ProviderID, config.BaseURL,
		[]string{config.Model}, config.Model, "test-only",
	)
	if prepare != nil {
		prepare(t, config)
	}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		if server != nil {
			server.Close()
			shutdownOwnedRuntimeHandler(t, handler)
		}
	}()
	client := &http.Client{Timeout: 30 * time.Second, Transport: runtimeOptionalPrivacyTransport{t: t}}
	checkpoint("startup")
	if observe != nil {
		observe(t, client, server.URL)
	}

	status, created := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodPost, "/v1/threads", map[string]any{
		"title": "plan then protected follow-up", "workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model,
	})
	threadID := contracts.StringField(created, "id")
	if status != http.StatusCreated || threadID == "" {
		t.Fatalf("create thread status=%d body=%#v", status, created)
	}
	guiPlan := map[string]any{
		"operation": "draft", "workspaceRoot": workspace, "relativePath": planPath,
		"planId": "plan-async-protected-follow-up", "sourceRequest": planPrompt, "title": "Focused plan",
	}
	status, started := packagedSourceUnavailableHydrationHTTPJSONV1(
		t, client, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		map[string]any{
			"prompt": planPrompt, "mode": "plan", "guiPlan": guiPlan,
			"approvalPolicy": "auto", "sandboxMode": "workspace-write",
		},
	)
	planTurnID := contracts.StringField(started, "turnId")
	if status != http.StatusAccepted || planTurnID == "" {
		t.Fatalf("plan turn status=%d body=%#v", status, started)
	}
	t.Log("ordinary joint phase: plan hydration")
	planTurn := packagedPlanThenProtectedWaitTurnV1(t, client, server.URL, threadID, planTurnID)
	packagedPlanThenProtectedValidatePlanV1(t, planTurn, planMarker)
	if got := providerCalls.Load(); got != 2 {
		t.Fatalf("plan turn provider calls=%d want=2", got)
	}
	if body, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(planPath))); err != nil || string(body) != planBody {
		t.Fatalf("create_plan output mismatch: err=%v", err)
	}
	checkpoint("plan-completed")

	status, protected := packagedSourceUnavailableHydrationHTTPJSONV1(
		t, client, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		map[string]any{
			"prompt": packagedSourceUnavailableHydrationPromptV1, "mode": "agent", "async": true,
		},
	)
	protectedTurnID := contracts.StringField(protected, "turnId")
	if status != http.StatusAccepted || protectedTurnID == "" {
		t.Fatalf("protected turn status=%d body=%#v", status, protected)
	}
	t.Log("ordinary joint phase: protected boundary terminal")
	packagedPlanThenProtectedWaitSSETerminalV1(t, client, server.URL, threadID, protectedTurnID)
	first := packagedPlanThenProtectedReadV1(t, client, server.URL, threadID, protectedTurnID)
	packagedPlanThenProtectedValidateZeroFactArtifactsV1(t, first.turn)
	if got := providerCalls.Load(); got != 2 {
		t.Fatalf("protected source-unavailable turn reached provider: calls=%d want=2", got)
	}
	checkpoint("protected-source-unavailable")

	ordinaryPrompt := `Continue in this exact thread. Before answering, make exactly one tool call: read with exactly these JSON arguments: {"path":"acceptance-context.txt","limit":1}. Use no additional arguments and make no other tool call. Return the exact content marker MILESTONE_A_POST_DENIAL_ORDINARY_OK. Preserve all existing thread state.`
	status, ordinary := packagedSourceUnavailableHydrationHTTPJSONV1(
		t, client, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		map[string]any{
			"prompt": ordinaryPrompt, "mode": "agent", "async": true,
			"approvalPolicy": "never", "sandboxMode": "read-only",
		},
	)
	ordinaryTurnID := contracts.StringField(ordinary, "turnId")
	if status != http.StatusAccepted || ordinaryTurnID == "" {
		t.Fatalf("post-denial ordinary turn status=%d body=%#v", status, ordinary)
	}
	t.Log("ordinary joint phase: post-denial ordinary hydration")
	firstOrdinary := packagedPlanThenProtectedWaitTurnV1(t, client, server.URL, threadID, ordinaryTurnID)
	packagedPlanThenProtectedValidateOrdinaryReadV1(t, firstOrdinary, ordinaryReadMarker)
	if got := providerCalls.Load(); got != 4 || !ordinaryReadObserved.Load() {
		t.Fatalf("post-denial ordinary read provider calls=%d resultObserved=%t want=4,true", got, ordinaryReadObserved.Load())
	}
	firstProtectedAfterOrdinary := packagedPlanThenProtectedWaitTurnV1(t, client, server.URL, threadID, protectedTurnID)
	packagedPlanThenProtectedValidateZeroFactArtifactsV1(t, firstProtectedAfterOrdinary)
	checkpoint("ordinary-read-completed")

	server.Close()
	shutdownOwnedRuntimeHandler(t, handler)
	server = nil
	checkpoint("shutdown")

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	restartedServer := httptest.NewServer(restarted)
	defer func() {
		restartedServer.Close()
		shutdownOwnedRuntimeHandler(t, restarted)
	}()
	checkpoint("restarted")
	if observe != nil {
		observe(t, client, restartedServer.URL)
	}
	t.Log("ordinary joint phase: restarted history hydration")
	secondProtected := packagedPlanThenProtectedWaitTurnV1(t, client, restartedServer.URL, threadID, protectedTurnID)
	packagedPlanThenProtectedValidateZeroFactArtifactsV1(t, secondProtected)
	if !packagedSourceUnavailableHydrationSameJSONV1(firstProtectedAfterOrdinary, secondProtected) {
		t.Fatalf("protected source-unavailable turn changed across restart: first=%#v second=%#v", firstProtectedAfterOrdinary, secondProtected)
	}
	secondOrdinary := packagedPlanThenProtectedWaitTurnV1(t, client, restartedServer.URL, threadID, ordinaryTurnID)
	packagedPlanThenProtectedValidateOrdinaryReadV1(t, secondOrdinary, ordinaryReadMarker)
	if !packagedSourceUnavailableHydrationSameJSONV1(firstOrdinary, secondOrdinary) {
		t.Fatalf("post-denial ordinary hydration changed across restart: first=%#v second=%#v", firstOrdinary, secondOrdinary)
	}
	if got := providerCalls.Load(); got != 4 {
		t.Fatalf("accepted-final readback reached provider: calls=%d want=4", got)
	}
	return config, threadID, []string{planTurnID, ordinaryTurnID}
}

func packagedPlanThenProtectedReadV1(
	t *testing.T,
	client *http.Client,
	serverURL string,
	threadID string,
	turnID string,
) packagedPlanThenProtectedReadbackV1 {
	t.Helper()
	status, detail := packagedSourceUnavailableHydrationHTTPJSONV1(
		t, client, serverURL, http.MethodGet, "/v1/threads/"+threadID, nil,
	)
	turn := packagedSourceUnavailableHydrationTurnV1(detail, turnID)
	if status != http.StatusOK || turn == nil || contracts.StringField(turn, "status") != "completed" ||
		turn["acceptedFinal"] != nil || turn["acceptedFinalView"] == nil {
		t.Fatalf("accepted-final thread readback status=%d body=%#v", status, detail)
	}
	return packagedPlanThenProtectedReadbackV1{
		hydration: packagedSourceUnavailableHydrationValidateV1(t, detail, turn),
		turn:      turn,
	}
}

func packagedPlanThenProtectedWaitSSETerminalV1(
	t *testing.T,
	client *http.Client,
	serverURL string,
	threadID string,
	turnID string,
) {
	t.Helper()
	// Keep the event wait beyond the host terminal tail's 15-second budget.
	deadline := time.Now().Add(30 * time.Second)
	lastEvents := []string{}
	for {
		request, err := http.NewRequest(http.MethodGet, serverURL+"/v1/threads/"+threadID+"/events?since_seq=0", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode == http.StatusOK {
			lastEvents = lastEvents[:0]
			for _, line := range strings.Split(string(body), "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				event := map[string]any{}
				if json.Unmarshal([]byte(payload), &event) != nil {
					continue
				}
				lastEvents = append(lastEvents, contracts.StringField(event, "kind")+":"+contracts.StringField(event, "turnId"))
				if contracts.StringField(event, "turnId") == turnID &&
					contracts.StringField(event, "kind") == "accepted_final_batch" {
					return
				}
			}
		} else if response.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("protected terminal SSE status=%d", response.StatusCode)
		}
		if time.Now().After(deadline) {
			status, detail := packagedSourceUnavailableHydrationHTTPJSONV1(
				t, client, serverURL, http.MethodGet, "/v1/threads/"+threadID, nil,
			)
			t.Fatalf("timed out waiting for terminal SSE: turn=%s events=%v threadStatus=%d detail=%#v", turnID, lastEvents, status, detail)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func packagedPlanThenProtectedValidateZeroFactArtifactsV1(t *testing.T, turn map[string]any) {
	t.Helper()
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		switch contracts.StringField(item, "kind") {
		case "tool_call", "tool_result":
			t.Fatalf("source-unavailable boundary dispatched a tool artifact: %#v", item)
		}
	}
}

func packagedPlanThenProtectedWaitTurnV1(
	t *testing.T,
	client *http.Client,
	serverURL string,
	threadID string,
	turnID string,
) map[string]any {
	t.Helper()
	// The host terminal tail has its own 15-second budget. Allow that full
	// bounded operation plus the preceding synthetic provider/tool work.
	deadline := time.Now().Add(30 * time.Second)
	lastStatus := 0
	lastDetail := map[string]any{}
	for {
		status, detail := packagedSourceUnavailableHydrationHTTPJSONV1(
			t, client, serverURL, http.MethodGet, "/v1/threads/"+threadID, nil,
		)
		lastStatus, lastDetail = status, detail
		if status == http.StatusOK {
			turn := packagedSourceUnavailableHydrationTurnV1(detail, turnID)
			if turn != nil && contracts.StringField(turn, "status") == "completed" {
				return turn
			}
		} else if status != http.StatusServiceUnavailable {
			t.Fatalf("thread hydration status=%d body=%#v", status, detail)
		}
		if time.Now().After(deadline) {
			stack := make([]byte, 2<<20)
			stack = stack[:runtime.Stack(stack, true)]
			for _, goroutine := range strings.Split(string(stack), "\n\n") {
				if strings.Contains(goroutine, "analytix.local/runtime-go/internal/app/") ||
					strings.Contains(goroutine, "analytix.local/runtime-go/internal/server/") {
					t.Logf("pending hydration synthetic goroutine:\n%s", goroutine)
				}
			}
			t.Fatalf("timed out waiting for turn hydration: status=%d body=%#v", lastStatus, lastDetail)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func packagedPlanThenProtectedValidatePlanV1(t *testing.T, turn map[string]any, marker string) {
	t.Helper()
	items, _ := turn["items"].([]any)
	var toolCall, toolResult, final bool
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		switch contracts.StringField(item, "kind") {
		case "tool_call":
			toolCall = toolCall || contracts.StringField(item, "toolName") == "create_plan"
		case "tool_result":
			toolResult = toolResult || contracts.StringField(item, "toolName") == "create_plan"
		case "assistant_text":
			final = final || strings.Contains(contracts.StringField(item, "text"), marker)
		}
	}
	if !toolCall || !toolResult || !final {
		t.Fatalf("plan turn is not a completed tool/result/final chain: toolCall=%t toolResult=%t final=%t turn=%#v", toolCall, toolResult, final, turn)
	}
}

func packagedPlanThenProtectedValidateOrdinaryReadV1(t *testing.T, turn map[string]any, marker string) {
	t.Helper()
	items, _ := turn["items"].([]any)
	var final bool
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if contracts.StringField(item, "kind") == "assistant_text" {
			final = final || strings.Contains(contracts.StringField(item, "text"), marker)
		}
	}
	if !final {
		t.Fatalf("post-denial ordinary read did not publish its accepted final: turn=%#v", turn)
	}
}

// Inspect the actual response bytes used by the public HTTP/SSE assertions,
// including history hydration, without logging a rejected private payload.
type runtimeOptionalPrivacyTransport struct{ t *testing.T }

func (transport runtimeOptionalPrivacyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte(runtimeOptionalDomainPrivateCanary)) {
		transport.t.Error("private domain canary reached public HTTP/SSE response")
		return nil, errors.New("private domain canary escaped public projection")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}
