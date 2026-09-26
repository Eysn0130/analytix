//go:build !analytix_prod

package runtimego

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// This fixture examines the production wire before deciding what to do. It
// cannot produce the correct file effect when continuation sources are lost.
func TestPostC8ContinuationSourcesDriveRealToolEffectsAfterTwoCompactionsAndRestart(t *testing.T) {
	dataDir := workspacetest.New(t)
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A.txt", "B.txt", "C.txt"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte("untouched"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	executing, reads, writes := false, 0, 0
	var failure string
	compactionDigests := map[string]bool{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid synthetic request", 400)
			return
		}
		messages := anyList(body["messages"])
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emitText := func(value string) {
			encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": value}, "finish_reason": "stop"}}})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
		}
		emitTool := func(name string, args map[string]any) {
			encodedArgs, _ := json.Marshal(args)
			encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("synthetic_%d_%d", reads, writes), "type": "function", "function": map[string]any{"name": name, "arguments": string(encodedArgs)}}}}, "finish_reason": "tool_calls"}}})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
		}
		var sources []any
		var toolText strings.Builder
		for _, raw := range messages {
			msg, _ := raw.(map[string]any)
			content := stringField(msg, "content")
			if stringField(msg, "role") == "tool" {
				toolText.WriteString(content)
			}
			if strings.HasPrefix(content, "[Analytix task continuation snapshot;") {
				_, payload, ok := strings.Cut(content, "\n")
				if ok {
					var snapshot map[string]any
					if json.Unmarshal([]byte(payload), &snapshot) == nil {
						if digest := stringField(snapshot, "sourceSnapshotReference"); domainthread.ValidContinuationProviderReferenceV1(digest) {
							compactionDigests[digest] = true
						}
						history, _ := snapshot["userHistory"].(map[string]any)
						sources = anyList(history["sources"])
					}
				}
			}
		}
		if !executing {
			emitText("Recorded the current user message.")
			return
		}
		if len(sources) < 9 {
			failure = "production request lost chronological source references"
			emitText("Source references unavailable.")
			return
		}
		if reads < 2 {
			index := 0
			if reads == 1 {
				index = 8
			}
			source, _ := sources[index].(map[string]any)
			ref := stringField(source, "reference")
			if !domainthread.ValidContinuationProviderReferenceV1(ref) {
				failure = "source reference is not resolvable"
				emitText("Invalid source reference.")
				return
			}
			reads++
			emitTool("read_task_history", map[string]any{"reference": ref, "offset": 0, "limit": 160})
			return
		}
		observed := toolText.String()
		if !strings.Contains(observed, "Only modify A.txt; never modify B.txt") || !strings.Contains(observed, "Correction: use C.txt instead of A.txt") {
			failure = "authorized history tool did not return early instruction and correction"
			emitText("Source content unavailable.")
			return
		}
		if writes == 0 {
			writes++
			emitTool("write_file", map[string]any{"path": "C.txt", "content": "corrected task completed"})
			return
		}
		if !strings.Contains(observed, "C.txt") {
			failure = "write result missing"
		}
		emitText("Applied the newer correction to C.txt and preserved A.txt and B.txt.")
	}))
	defer provider.Close()
	var providers map[string]any
	if err := json.Unmarshal([]byte(testModelProvidersJSON(provider.URL, "continuation-provider", "continuation-model")), &providers); err != nil {
		t.Fatal(err)
	}
	configuredProvider := anyList(providers["providers"])[0].(map[string]any)
	configuredProvider["modelProfiles"] = map[string]any{"continuation-model": map[string]any{"contextWindowTokens": 20000}}
	config := RuntimeServerContractConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: dataDir, ModelProvidersJSON: string(mustJSON(t, providers))}
	handler := newRuntimeServerProviderReadyTestHandler(t, config)
	server := httptest.NewServer(handler)
	defer func() { server.Close() }()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{"title": "No Goal continuation", "workspace": workspace, "providerId": "continuation-provider", "model": "continuation-model", "approvalPolicy": "auto", "sandboxMode": "workspace-write"}), http.StatusCreated)
	threadID := stringField(thread, "id")
	sent := 0
	send := func(prompt string) {
		t.Helper()
		assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": prompt, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}), http.StatusAccepted)
		sent++
		t.Logf("completed seeded/live request %d", sent)
	}
	send("Only modify A.txt; never modify B.txt. " + strings.Repeat("Bounded original user background. ", 700))
	for i := 1; i < 8; i++ {
		send(fmt.Sprintf("Continue recording context %d without changing prior instructions. ", i) + strings.Repeat("inert background ", 250))
	}
	// Cross the real configured soft limit twice. The production turn-start
	// compactor, not the manual /compact route, creates signed V4 snapshots.
	send("Correction: use C.txt instead of A.txt; B.txt remains forbidden. " + strings.Repeat("more inert context ", 1300))
	send("Keep the most recent correction in chronological order. " + strings.Repeat("final inert context ", 1300))
	send("Check the retained context without changing any file or instruction.")
	// Compaction can retire older replay events. Observe distinct sealed
	// snapshot identities at the actual Provider consumer across the journey.
	mu.Lock()
	compactionCount := len(compactionDigests)
	mu.Unlock()
	if compactionCount < 2 {
		t.Fatalf("expected two real automatic compactions at the consumer, got %d", compactionCount)
	}
	t.Log("closing Core before recovery")
	server.Close()
	shutdownRuntimeTestHandler(t, handler)
	t.Log("reopening Core")
	handler = newRuntimeServerProviderReadyTestHandler(t, config)
	server = httptest.NewServer(handler)
	mu.Lock()
	executing = true
	mu.Unlock()
	send("Use the history source tool to recover the original file constraint and its later correction, then write the requested result once.")
	mu.Lock()
	failureCopy, readCount, writeCount := failure, reads, writes
	mu.Unlock()
	if failureCopy != "" || readCount != 2 || writeCount != 1 {
		t.Fatalf("consumer/effect path failed: %s reads=%d writes=%d", failureCopy, readCount, writeCount)
	}
	for name, want := range map[string]string{"A.txt": "untouched", "B.txt": "untouched", "C.txt": "corrected task completed"} {
		got, err := os.ReadFile(filepath.Join(workspace, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s effect mismatch: %q %v", name, got, err)
		}
	}
	// Readback alone cannot cause a completed write to run again.
	t.Log("closing Core before recovery")
	server.Close()
	shutdownRuntimeTestHandler(t, handler)
	t.Log("reopening Core")
	handler = newRuntimeServerProviderReadyTestHandler(t, config)
	server = httptest.NewServer(handler)
	recovered := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	if recovered["goal"] != nil {
		t.Fatal("no-Goal journey acquired a Goal")
	}
	for _, rawTurn := range anyList(recovered["turns"]) {
		turn := rawTurn.(map[string]any)
		for _, field := range []string{"steering", "attachmentIds", "activeSkillIds", "injectedMemoryIds"} {
			if values, ok := turn[field].([]any); !ok || values == nil {
				t.Fatalf("recovered public turn missing exact schema array %s", field)
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 1 {
		t.Fatal("completed effect replayed on recovery")
	}
}

func TestPostC8DeepSeekMessagesRegistryToWire(t *testing.T) {
	dataDir := workspacetest.New(t)
	var mu sync.Mutex
	var bodies []map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(bodyBytes, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "wrong endpoint", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic-observed-model\",\"usage\":{\"input_tokens\":8,\"cache_read_input_tokens\":0,\"cache_creation_input_tokens\":0}}}\n\n")
		thinking, _ := body["thinking"].(map[string]any)
		if thinking["type"] != "disabled" {
			fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Synthetic private reasoning.\"}}\n\n")
		}
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Messages baseline reached through Core Registry.\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":9}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer provider.Close()
	providerConfig := map[string]any{"defaultProviderId": "messages-provider", "providers": []any{map[string]any{"id": "messages-provider", "apiKey": "synthetic-key", "baseUrl": provider.URL + "/v1", "endpointFormat": "messages", "models": []string{"deepseek-flash"}, "modelProfiles": map[string]any{"deepseek-flash": map[string]any{"reasoning": map[string]any{"requestProtocol": "deepseek-messages", "supportedEfforts": []string{"off", "auto", "low", "medium", "high", "max"}, "defaultEffort": "high"}}}}}}
	server := httptest.NewServer(newRuntimeServerProviderReadyTestHandler(t, RuntimeServerContractConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), Host: "127.0.0.1", DataDir: dataDir, ModelProvidersJSON: string(mustJSON(t, providerConfig))}))
	defer server.Close()
	for _, effort := range []string{"low", "medium", "high", "max", "off", "auto"} {
		t.Run(effort, func(t *testing.T) {
			thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{"title": "Messages wire", "workspace": dataDir}), http.StatusCreated)
			start := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+stringField(thread, "id")+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{"prompt": "Explain this baseline in one sentence.", "reasoningEffort": effort}), http.StatusAccepted)
			state := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/threads/"+stringField(thread, "id"), DefaultRuntimeToken, nil, http.StatusOK)
			stateBytes := string(mustJSON(t, state))
			if !strings.Contains(stateBytes, "Messages baseline reached through Core Registry.") || strings.Contains(stateBytes, "Synthetic private reasoning.") {
				t.Fatalf("Messages completion/privacy projection failed: completed=%v private=%v", strings.Contains(stateBytes, "Messages baseline reached through Core Registry."), strings.Contains(stateBytes, "Synthetic private reasoning."))
			}
			replay := liveSSE(t, server.URL, "/v1/threads/"+stringField(thread, "id")+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
			events := runtimeServerEventsForTurn(t, replay, stringField(start, "turnId"))
			usage := firstRuntimeServerEvent(t, events, "usage")
			if observation := mapField(t, usage, "cacheDiagnostics")["responseModelObservation"]; observation != "differs_resolved" {
				t.Fatalf("response model observation failed: %v", observation)
			}
			mu.Lock()
			body := bodies[len(bodies)-1]
			mu.Unlock()
			if body["model"] != "deepseek-flash" || body["max_tokens"] != float64(4096) {
				t.Fatalf("Registry/cap did not reach wire: model=%v cap=%v", body["model"], body["max_tokens"])
			}
			thinking, _ := body["thinking"].(map[string]any)
			output, _ := body["output_config"].(map[string]any)
			switch effort {
			case "auto":
				if thinking != nil || output != nil {
					t.Fatal("auto overrode protocol default")
				}
			case "off":
				if thinking["type"] != "disabled" || output != nil {
					t.Fatal("off not disabled")
				}
			default:
				want := effort
				if effort == "medium" {
					want = "high"
				}
				if thinking["type"] != "enabled" || output["effort"] != want {
					t.Fatal("effort encoding differs from public Messages semantics")
				}
			}
			encoded, _ := json.Marshal(body)
			for _, absent := range []string{"cache_control", "tool_update", "system_prompt_update", "session_log", "user_id"} {
				if strings.Contains(string(encoded), absent) {
					t.Fatalf("unapproved extension %s sent", absent)
				}
			}
		})
	}
}
