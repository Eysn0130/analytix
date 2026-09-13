//go:build darwin && !analytix_prod

package runtimego

import (
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	"analytix.local/runtime-go/internal/jobs"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These unchanged protected-process positives run in the required platform-root lane.
func TestRuntimeServerBashApprovalDenyAndAllowControlExecution(t *testing.T) {
	dataDir := t.TempDir()
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_deny","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf hi > bash.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_allow","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf hi > bash.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"bash handled"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "bash-provider", "bash-model"),
	}))
	t.Cleanup(server.Close)

	runApprovalCase := func(t *testing.T, decision string) string {
		t.Helper()
		workspace := t.TempDir()
		beforeRequests := provider.RequestCount()
		thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"title":          "Bash " + decision,
			"workspace":      workspace,
			"providerId":     "bash-provider",
			"model":          "bash-model",
			"approvalPolicy": "on-request",
			"sandboxMode":    "danger-full-access",
		}), http.StatusCreated)
		threadID := stringField(thread, "id")
		start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Run bash after approval."}), http.StatusAccepted)
		if start["pendingKind"] != "approval" {
			t.Fatalf("bash should request approval: %#v", start)
		}
		pendingID := stringField(start, "pendingId")
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/approvals/"+pendingID, DefaultRuntimeToken, mustJSON(t, map[string]string{"decision": decision}), http.StatusOK)
		expectedRequests := 2
		if decision == "deny" {
			expectedRequests = 1
		}
		if got := provider.RequestCount() - beforeRequests; got != expectedRequests {
			t.Fatalf("approval %s provider request count mismatch: got=%d want=%d", decision, got, expectedRequests)
		}
		if decision == "deny" {
			replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
			if !strings.Contains(replay, "approval_denied") ||
				!strings.Contains(replay, "no unverified final response was published") ||
				strings.Contains(replay, "bash handled") {
				t.Fatalf("denied bash approval did not end at the fixed host boundary:\n%s", replay)
			}
		}
		return workspace
	}

	deniedWorkspace := runApprovalCase(t, "deny")
	if _, err := os.Stat(filepath.Join(deniedWorkspace, "bash.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied bash approval must not execute command, stat err=%v", err)
	}
	allowedWorkspace := runApprovalCase(t, "allow")
	data, err := os.ReadFile(filepath.Join(allowedWorkspace, "bash.txt"))
	if err != nil || string(data) != "hi" {
		t.Fatalf("allowed bash approval should execute command, data=%q err=%v", string(data), err)
	}
}

func TestRuntimeServerBashReportsExitCodeAndDiagnostics(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_exit","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf hi; exit 7\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"exit handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "bash-exit-provider", "bash-exit-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Bash exit diagnostics",
		"workspace":      workspace,
		"providerId":     "bash-exit-provider",
		"model":          "bash-exit-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Run a command that exits nonzero.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("bash failure should execute tool and continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, bashResult := providerHostToolResultForName(t, body, "bash")
	resultJSON := string(mustJSON(t, bashResult))
	for _, needle := range []string{
		`"status":"failed"`,
		`"exitCode":7`,
		`"timedOut":false`,
		`"output":"hi"`,
		`"outputBytes":2`,
		`"maxOutputBytes":32768`,
		`"durationMs"`,
	} {
		if !strings.Contains(resultJSON, needle) {
			t.Fatalf("host-bound bash continuation missing diagnostic %q: result=%s body=%s", needle, resultJSON, body)
		}
	}
}

func TestRuntimeServerBashTimeoutKillsProcessGroupGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by taskkill fallback; Windows Job Object parity is tracked separately")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_timeout","type":"function","function":{"name":"bash","arguments":"{\"command\":\"sleep 20 & echo $! > child.pid; wait\",\"timeout\":1}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"timeout handled"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "bash-timeout-provider", "bash-timeout-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Bash process group timeout",
		"workspace":      workspace,
		"providerId":     "bash-timeout-provider",
		"model":          "bash-timeout-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Run a command that times out with a background child.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("bash timeout should execute tool and continue provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, bashResult := providerHostToolResultForName(t, body, "bash")
	resultJSON := string(mustJSON(t, bashResult))
	for _, needle := range []string{
		`"status":"timeout"`,
		`"timedOut":true`,
		`"exitCode":-1`,
		`"outputBytes"`,
		`"maxOutputBytes":32768`,
		`"durationMs"`,
	} {
		if !strings.Contains(resultJSON, needle) {
			t.Fatalf("host-bound timeout continuation missing bash diagnostic %q: result=%s body=%s", needle, resultJSON, body)
		}
	}
	pidBytes, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("timed out command should have recorded child pid: %v", err)
	}
	childPID := strings.TrimSpace(string(pidBytes))
	if childPID == "" {
		t.Fatalf("child pid file was empty")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", childPID).Run() != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("background child process %s survived bash timeout; process group kill did not run", childPID)
}

func TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	bashArgsFirst := mustJSONString(t, map[string]any{"command": "printf x >> repeat.txt"})
	bashArgsSecond := mustJSONString(t, map[string]any{
		"command": " printf x >> repeat.txt ", "timeout": terminalapp.DefaultBashTimeoutSeconds, "runInBackground": false,
	})
	provider := newCompleteProviderServer(t, [][]string{
		{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "call_repeat_write_1",
					"type":  "function",
					"function": map[string]any{
						"name":      "bash",
						"arguments": bashArgsFirst,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		},
		{
			"data: " + mustJSONString(t, map[string]any{"choices": []map[string]any{{
				"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "call_repeat_write_2",
					"type":  "function",
					"function": map[string]any{
						"name":      "bash",
						"arguments": bashArgsSecond,
					},
				}}},
				"finish_reason": "tool_calls",
			}}}),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"duplicate write blocked"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "repeat-guard-provider", "repeat-guard-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Repeat write guard",
		"workspace":      workspace,
		"providerId":     "repeat-guard-provider",
		"model":          "repeat-guard-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Try repeating the same write command.",
	}), http.StatusAccepted)
	if provider.RequestCount() != 3 {
		t.Fatalf("side-effect gate should continue after blocking the second write, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "repeat.txt"))
	if err != nil || string(data) != "x" {
		t.Fatalf("second equivalent bash write must be blocked before execution, data=%q err=%v", string(data), err)
	}
	body := provider.Body(2)
	bashCallIDs := providerHostToolCallIDsForName(t, body, "bash")
	if len(bashCallIDs) != 2 {
		t.Fatalf("repeat guard provider history lost host-bound bash calls: ids=%#v body=%s", bashCallIDs, body)
	}
	secondCallID := bashCallIDs[1]
	resultJSON := string(mustJSON(t, providerToolResultForCall(t, body, secondCallID)))
	for _, expected := range []string{
		`"code":"side_effect_duplicate"`,
		`"executed":false`,
		`"intentStatus":"closed"`,
		`"factAnswerAllowed":false`,
		`"evidenceAuthority":false`,
	} {
		if !strings.Contains(resultJSON, expected) {
			t.Fatalf("host-bound final provider result missing duplicate block %q: result=%s body=%s", expected, resultJSON, body)
		}
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, secondCallID) || !strings.Contains(replay, "side_effect_duplicate") || strings.Contains(replay, "call_repeat_write_2") {
		t.Fatalf("side-effect duplicate result should be visible in replay:\n%s", replay)
	}
}

func TestRuntimeServerBashRunInBackgroundWithholdsOutputAndSupportsKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("background shell process-group kill uses the POSIX shell path in this contract test")
	}
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_bg","type":"function","function":{"name":"bash","arguments":"{\"command\":\"sleep 20 & echo $! > child.pid; printf started; wait\",\"run_in_background\":true,\"timeout\":120}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"background bash accepted"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "background-shell-provider", "background-shell-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Background shell",
		"workspace":      workspace,
		"providerId":     "background-shell-provider",
		"model":          "background-shell-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Start background shell.",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("background bash should start job and continue parent provider loop, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	if !strings.Contains(provider.Body(0), "run_in_background") || !strings.Contains(provider.Body(0), "runInBackground") {
		t.Fatalf("parent bash schema should advertise background shell controls:\n%s", provider.Body(0))
	}
	hostCallID, bashResult := providerHostToolResultForName(t, provider.Body(1), "bash")
	assertSecurityBoundChildMetadata(t, bashResult, "", "running", true)
	if strings.Contains(provider.Body(1), "artifactPath") || strings.Contains(provider.Body(1), "background_shell") {
		t.Fatalf("parent continuation exposed private background shell metadata:\n%s", provider.Body(1))
	}

	var output map[string]any
	var internalRun jobs.Record
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		output = assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/output", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"jobId":    "job-1",
			"threadId": threadID,
		}), http.StatusOK)
		assertSecurityBoundTaskOutputV1(t, output, "job-1", "running")
		internalRuns := runtimeServerInternalChildRuns(t, dataDir, threadID)
		if len(internalRuns) == 1 {
			internalRun = internalRuns[0]
		}
		if _, err := os.Stat(filepath.Join(workspace, "child.pid")); err == nil && internalRun.ID != "" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if internalRun.Output != "" || internalRun.Error != "" || internalRun.Status != "running" || internalRun.Kind != "background-shell" || internalRun.SecurityBinding == nil {
		t.Fatalf("background bash must retain only bound control metadata: public=%#v internal=%#v", output, internalRun)
	}
	pidBytes, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("background shell should record child pid: %v", err)
	}
	childPID := strings.TrimSpace(string(pidBytes))
	if childPID == "" {
		t.Fatalf("background child pid file was empty")
	}
	killed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/runtime/task-jobs/kill", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"jobId":    "job-1",
		"threadId": threadID,
		"reason":   "test cleanup",
	}), http.StatusOK)
	killedJob := mapField(t, killed, "job")
	assertSecurityBoundChildMetadata(t, killedJob, "job-1", "killed", true)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", childPID).Run() != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if exec.Command("kill", "-0", childPID).Run() == nil {
		t.Fatalf("background shell child process %s survived kill_shell cancellation", childPID)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	progress := runtimeServerEventWithKindAndChildStatus(t, events, "tool_progress", "running")
	child := mapField(t, progress, "child")
	if stringField(progress, "callId") != hostCallID || strings.Contains(replay, "call_bash_bg") {
		t.Fatalf("background bash replay did not preserve its host call id: progress=%#v", progress)
	}
	assertSecurityBoundChildMetadata(t, child, "job-1", "running", true)
}

func TestRuntimeServerInterruptStopsLaterToolsInSameProviderStep(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_cancel","type":"function","function":{"name":"bash","arguments":"{\"command\":\"printf started > started.txt; sleep 30\",\"timeout\":60}"}},{"index":1,"id":"call_write_after_cancel","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"must not write\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"should not continue after tool cancel"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "tool-cancel-provider", "tool-cancel-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":          "Interrupt tool batch",
		"workspace":      workspace,
		"providerId":     "tool-cancel-provider",
		"model":          "tool-cancel-model",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt": "Run a long bash then write.",
		"async":  true,
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if turnID == "" {
		t.Fatalf("async turn start missing turn id: %#v", start)
	}
	startedPath := filepath.Join(workspace, "started.txt")
	startDeadline := time.Now().Add(runtimeServerPositiveTestTimeout)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(startDeadline) {
			t.Fatalf("bash tool did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	interrupt := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/interrupt", DefaultRuntimeToken, mustJSON(t, map[string]any{}), http.StatusOK)
	if interrupt["status"] != "aborted" || interrupt["cancelled"] != true {
		t.Fatalf("interrupt should abort active tool turn: %#v", interrupt)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if _, err := os.Stat(filepath.Join(workspace, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("write_file after tool cancel must not execute, stat err=%v replay=%s", err, replay)
	}
	if !hasRuntimeServerEvent(runtimeServerEventsForTurn(t, replay, turnID), "turn_aborted") ||
		strings.Contains(replay, "call_write_after_cancel") || strings.Contains(replay, "tool_cancelled") {
		t.Fatalf("terminal turn must reject all late tool and cancellation results:\n%s", replay)
	}
}

func TestRuntimeServerGoalTodoCompleteStepAndFinalReadiness(t *testing.T) {
	dataDir := t.TempDir()
	todoArgs := string(mustJSON(t, map[string]any{
		"todos": []map[string]any{{"content": "Audit contract", "status": "in_progress"}},
	}))
	stepArgs := string(mustJSON(t, map[string]any{
		"step":   "Audit contract",
		"result": "contract audited",
		"evidence": []map[string]any{{
			"kind":    "verification",
			"summary": "runtime contract verification ran",
			"command": "printf verified",
		}},
	}))
	bashArgs := `{"command":"printf verified"}`
	completeArgs := `{"status":"complete"}`
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_todo_write","type":"function","function":{"name":"todo_write","arguments":%q}},{"index":1,"id":"call_bash_verify","type":"function","function":{"name":"bash","arguments":%q}},{"index":2,"id":"call_complete_step","type":"function","function":{"name":"complete_step","arguments":%q}},{"index":3,"id":"call_goal_complete","type":"function","function":{"name":"update_goal","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, todoArgs, bashArgs, stepArgs, completeArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"goal complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "goal-provider", "goal-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Goal todo",
		"workspace":  dataDir,
		"providerId": "goal-provider",
		"model":      "goal-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Complete with evidence",
	}), http.StatusOK)
	start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Update todos, verify, sign off the step, then complete the goal.",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusAccepted)
	turnID := stringField(start, "turnId")
	if provider.RequestCount() != 2 {
		t.Fatalf("goal todo tool loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	resultsJSON := ""
	for _, toolName := range []string{"todo_write", "bash", "complete_step", "update_goal"} {
		_, result := providerHostToolResultForName(t, body, toolName)
		resultsJSON += string(mustJSON(t, result))
	}
	for _, expected := range []string{"Goal achieved", "hostVerified"} {
		if !strings.Contains(resultsJSON, expected) {
			t.Fatalf("host-bound goal/todo results missing %s: results=%s body=%s", expected, resultsJSON, body)
		}
	}
	goal := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, nil, http.StatusOK)
	goalBody := mapField(t, goal, "goal")
	ledger, _ := goalBody["evidenceLedger"].([]any)
	if goalBody["status"] != "complete" || len(ledger) != 1 {
		t.Fatalf("goal should be complete with exactly one evidence entry: %#v", goal)
	}
	todos := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, nil, http.StatusOK)
	todoItems, _ := mapField(t, todos, "todos")["items"].([]any)
	if len(todoItems) != 1 {
		t.Fatalf("expected one todo item: %#v", todos)
	}
	todoItem, _ := todoItems[0].(map[string]any)
	if stringField(todoItem, "status") != "completed" {
		t.Fatalf("complete_step should advance the matching todo to completed: %#v", todos)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := runtimeServerEventsForTurn(t, replay, turnID)
	for _, kind := range []string{"tool_call_finished", "item_completed", "turn_completed"} {
		if !hasRuntimeServerEvent(events, kind) {
			t.Fatalf("goal/todo loop replay missing %s:\n%s", kind, replay)
		}
	}
	if strings.Contains(replay, "event: goal_updated") || strings.Contains(replay, "event: todos_updated") || strings.Contains(replay, "runtime contract verification ran") {
		t.Fatalf("goal/todo durable replay exposed untyped goal/todo/evidence prose:\n%s", replay)
	}
}

func TestRuntimeServerCompleteStepRejectsMismatchedTodoIndexBeforeEvidence(t *testing.T) {
	dataDir := t.TempDir()
	stepArgs := string(mustJSON(t, map[string]any{
		"step":       "Wrong contract",
		"step_index": 1,
		"evidence": []map[string]any{{
			"kind":    "verification",
			"summary": "runtime contract verification ran",
			"command": "printf verified",
		}},
	}))
	bashArgs := `{"command":"printf verified"}`
	provider := newCompleteProviderServer(t, [][]string{
		{
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_bash_verify","type":"function","function":{"name":"bash","arguments":%q}},{"index":1,"id":"call_complete_step","type":"function","function":{"name":"complete_step","arguments":%q}}]},"finish_reason":"tool_calls"}]}`, bashArgs, stepArgs),
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"todo still active"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     t.TempDir(),
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "goal-provider", "goal-model"),
	}))
	defer server.Close()

	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Goal todo mismatch",
		"workspace":  dataDir,
		"providerId": "goal-provider",
		"model":      "goal-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Complete only matching todos",
	}), http.StatusOK)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"todos": []map[string]any{{"content": "Audit contract", "status": "in_progress"}},
	}), http.StatusOK)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":         "Verify but try to sign off the wrong todo text.",
		"approvalPolicy": "auto",
		"sandboxMode":    "danger-full-access",
	}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("goal todo mismatch loop should call provider twice, got %d bodies=%#v", provider.RequestCount(), provider.Bodies())
	}
	body := provider.Body(1)
	_, stepResult := providerHostToolResultForName(t, body, "complete_step")
	resultJSON := string(mustJSON(t, stepResult))
	if !strings.Contains(resultJSON, "todo_step_mismatch") {
		t.Fatalf("mismatched complete_step should return a host-bound tool error: result=%s body=%s", resultJSON, body)
	}
	goal := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, nil, http.StatusOK)
	ledger, _ := mapField(t, goal, "goal")["evidenceLedger"].([]any)
	if len(ledger) != 0 {
		t.Fatalf("mismatched complete_step must not append evidence before todo identity is validated: %#v", goal)
	}
	todos := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, nil, http.StatusOK)
	todoItems, _ := mapField(t, todos, "todos")["items"].([]any)
	if len(todoItems) != 1 {
		t.Fatalf("expected one todo item: %#v", todos)
	}
	todoItem, _ := todoItems[0].(map[string]any)
	if stringField(todoItem, "status") != "in_progress" {
		t.Fatalf("mismatched complete_step must leave the matched index unfinished: %#v", todos)
	}
}
