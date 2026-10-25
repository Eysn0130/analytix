//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	"analytix.local/runtime-go/internal/server"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Keep the ordinary task's real 512-token input while observing both its
// terminal outcome and a same-root production Prepare after normal shutdown.
// Ordinary cumulative-budget semantics remain unresolved; this test does not
// assert a new cost limit or reinterpret the foreground output-budget contract.
func TestRuntimeBackgroundChildBudget512ThenShutdownPrepareV1(t *testing.T) {
	errorType := func(err error) string {
		var integrity persistencefs.IntegrityError
		var preparation *startupPreparationErrorV1
		switch {
		case err == nil:
			return "none"
		case errors.Is(err, context.Canceled):
			return "cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			return "deadline"
		case errors.Is(err, errRuntimeReportRestartReconciliationRequired):
			return "report_reconciliation_required"
		case errors.As(err, &integrity):
			return "persistence_integrity"
		case errors.As(err, &preparation):
			return "startup_preparation"
		default:
			return "unclassified"
		}
	}
	closedStatus := func(value string) string {
		switch value {
		case "queued", "running", "waiting", "completed", "failed", "killed", "aborted", "cancelled", "interrupted":
			return value
		default:
			return "unobserved"
		}
	}
	// Only fresh synthetic errors reach this observer. Fingerprints let the
	// task match fixed source literals without printing paths or error bodies.
	observePrepareCause := func(t *testing.T, err error, roots ...string) {
		var replacements []string
		for _, root := range roots {
			if root != "" {
				replacements = append(replacements, root, "<synthetic-root>")
			}
		}
		normalize := strings.NewReplacer(replacements...)
		var visit func(error, int)
		nodes := 0
		visit = func(current error, depth int) {
			if current == nil || depth > 16 || nodes >= 32 {
				return
			}
			nodes++
			kind := strings.NewReplacer("*", "pointer_", ".", "_", "/", "_").Replace(reflect.TypeOf(current).String())
			digest := sha256.Sum256([]byte(normalize.Replace(current.Error())))
			t.Logf("phase=restart_cause node=%d depth=%d type=%s class=%s normalized_sha=%x", nodes, depth, kind, errorType(current), digest)
			switch wrapped := current.(type) {
			case interface{ Unwrap() []error }:
				for _, cause := range wrapped.Unwrap() {
					visit(cause, depth+1)
				}
			case interface{ Unwrap() error }:
				visit(wrapped.Unwrap(), depth+1)
			}
		}
		visit(err, 0)
	}
	for _, cell := range []string{"parent_only", "background_512"} {
		t.Run(cell, func(t *testing.T) {
			const childPrompt = "BACKGROUND_BUDGET_512_CHILD_V1: return only a short synthetic reply; call no tools."
			var parentCalls, childCalls, taskResults, unexpected atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content any    `json:"content"`
					} `json:"messages"`
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" ||
					json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&request) != nil {
					unexpected.Add(1)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				lastUser, results := "", 0
				for _, message := range request.Messages {
					if message.Role == "user" {
						body, _ := json.Marshal(message.Content)
						lastUser = string(body)
					}
					if message.Role == "tool" {
						results++
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if strings.Contains(lastUser, "BACKGROUND_BUDGET_512_CHILD_V1") {
					childCalls.Add(1)
					runtimeOptionalLifecycleModelResponseV1(w, "", nil, "Synthetic child complete.")
					return
				}
				call := parentCalls.Add(1)
				if cell == "background_512" && call == 1 {
					runtimeOptionalLifecycleModelResponseV1(w, "task", map[string]any{
						"name": "Synthetic continuable child", "prompt": childPrompt,
						"toolPolicy": "readOnly", "run_in_background": true,
						"max_steps": 2, "token_budget": 512, "time_budget_ms": 15000,
						"auto_continue_parent": false,
					}, "")
					return
				}
				if cell == "background_512" {
					taskResults.Add(int32(results))
				}
				runtimeOptionalLifecycleModelResponseV1(w, "", nil, "Synthetic parent complete.")
			}))
			defer provider.Close()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			config.ProductionDurableRoot = runtimeWitnessedRegistryHostTempV2(t, "background-budget-durable")
			config.UserDataDir = runtimeWitnessedRegistryHostTempV2(t, "background-budget-ui")
			config.BaseURL = provider.URL + "/v1"
			config.MCPConfigJSON = `{"subagents":{"enabled":true,"default_tool_policy":"readOnly"}}`
			seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
			observations := make(chan server.AsyncTurnObservationV1, 16)
			var overflow atomic.Bool
			ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) {
				select {
				case observations <- o:
				default:
					overflow.Store(true)
				}
			})
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatalf("phase=initial_lease type=%s", errorType(err))
			}
			inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatalf("phase=initial_start code=%s type=%s", RuntimeStartupPreparationFailureCodeV1(err), errorType(err))
			}
			owned := &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
			active := true
			defer func() {
				if active {
					stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if err := owned.Shutdown(stop); err != nil {
						t.Errorf("phase=cleanup_shutdown type=%s", errorType(err))
					}
				}
			}()
			// Match the existing direct-mux HTTP seam, but never dump an error body.
			request := func(method, path string, body map[string]any) (int, map[string]any) {
				encoded, encodeErr := json.Marshal(body)
				if encodeErr != nil {
					t.Errorf("phase=request_encode type=unclassified")
					return 0, nil
				}
				req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
				req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				owned.ServeHTTP(rec, req)
				var result map[string]any
				if json.Unmarshal(rec.Body.Bytes(), &result) != nil {
					t.Errorf("phase=response_decode http=%d type=unclassified", rec.Code)
				}
				return rec.Code, result
			}
			status, thread := request(http.MethodPost, "/v1/threads", map[string]any{
				"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model,
			})
			threadID := contracts.StringField(thread, "id")
			if status != http.StatusCreated || threadID == "" {
				t.Fatalf("phase=parent_create http=%d", status)
			}
			prompt := "Complete one ordinary synthetic parent task."
			if cell == "background_512" {
				prompt = "Use the task tool to start one background subagent for a concise synthetic reply."
			}
			status, started := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
				"prompt": prompt, "mode": "agent", "async": true,
				"approvalPolicy": "auto", "sandboxMode": "read-only",
			})
			turnID := contracts.StringField(started, "turnId")
			if status != http.StatusAccepted || turnID == "" {
				t.Fatalf("phase=parent_start http=%d", status)
			}
			parentStatus, jobStatus, childStatus := "unobserved", "unobserved", "unobserved"
			parentFinished, jobTerminal := false, cell == "parent_only"
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) && (!parentFinished || !jobTerminal) {
				select {
				case o := <-observations:
					if o.ThreadID == threadID && o.TurnID == turnID && o.Stage == "finished" {
						parentFinished = true
						parentStatus = closedStatus(o.TerminalStatus)
						t.Logf("phase=parent_terminal status=%s completion_phase=%s completion_type=%s record_type=%s", parentStatus, o.CompletionPhase, o.CompletionErrorClass, o.FailureRecordErrorClass)
					}
				case <-time.After(10 * time.Millisecond):
				}
				if parentFinished && !jobTerminal {
					status, waited := request(http.MethodPost, "/v1/runtime/task-jobs/wait", map[string]any{"threadId": threadID, "timeoutMs": 100})
					jobs, _ := waited["jobs"].([]any)
					if status == http.StatusOK && len(jobs) == 1 {
						job, _ := jobs[0].(map[string]any)
						jobStatus = closedStatus(contracts.StringField(job, "status"))
						switch jobStatus {
						case "completed", "failed", "killed", "aborted", "cancelled", "interrupted":
							jobTerminal = true
						}
					}
				}
			}
			if cell == "background_512" {
				status, summary := request(http.MethodGet, "/v1/threads/"+threadID+"/summary", nil)
				subagents, _ := summary["subagents"].([]any)
				if status == http.StatusOK && len(subagents) == 1 {
					child, _ := subagents[0].(map[string]any)
					childID, childTurnID := contracts.StringField(child, "childThreadId"), contracts.StringField(child, "childTurnId")
					if childID != "" && childTurnID != "" {
						status, detail := request(http.MethodGet, "/v1/threads/"+childID, nil)
						if status == http.StatusOK {
							turn := packagedSourceUnavailableHydrationTurnV1(detail, childTurnID)
							childStatus = closedStatus(contracts.StringField(turn, "status"))
						}
					}
				}
			}
			t.Logf("phase=live_complete parent=%s child=%s job=%s parent_drained=%t job_terminal=%t parent_calls=%d child_calls=%d task_results=%d fixture_rejected=%d budget_semantics=unresolved", parentStatus, childStatus, jobStatus, parentFinished, jobTerminal, parentCalls.Load(), childCalls.Load(), taskResults.Load(), unexpected.Load())
			stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			shutdownErr := owned.Shutdown(stop)
			cancel()
			t.Logf("phase=shutdown type=%s", errorType(shutdownErr))
			if shutdownErr != nil {
				t.Errorf("normal shutdown failed; same-root Prepare remains blocked")
				return
			}
			active = false
			restartLease, leaseErr := AcquireRuntimePersistenceLease(config)
			if leaseErr != nil {
				t.Errorf("phase=restart_lease type=%s", errorType(leaseErr))
				return
			}
			beforePrepareCalls := parentCalls.Load() + childCalls.Load()
			prepareCtx, cancelPrepare := context.WithTimeout(context.Background(), 20*time.Second)
			prepared, prepareErr := PrepareRuntimeServerStartupWithPersistenceLeaseContextE(prepareCtx, config, restartLease)
			cancelPrepare()
			prepareCode := "none"
			if prepareErr != nil {
				prepareCode = RuntimeStartupPreparationFailureCodeV1(prepareErr)
			}
			t.Logf("phase=restart_prepare prepared=%t code=%s type=%s provider_calls_unchanged=%t", prepared != nil, prepareCode, errorType(prepareErr), beforePrepareCalls == parentCalls.Load()+childCalls.Load())
			observePrepareCause(t, prepareErr, config.ProductionDurableRoot, config.UserDataDir, config.DataDir)
			if closeErr := restartLease.Close(); closeErr != nil {
				t.Errorf("phase=restart_lease_close type=%s", errorType(closeErr))
			}
			// Record both outcomes before reporting any child regression failure.
			if !parentFinished || parentStatus != "completed" || overflow.Load() || unexpected.Load() != 0 {
				t.Errorf("ordinary parent or fixture did not complete cleanly")
			}
			if cell == "background_512" && (!jobTerminal || jobStatus != "completed" || childStatus != "completed" || childCalls.Load() != 1 || taskResults.Load() != 1) {
				t.Errorf("ordinary background child with unchanged 512 input did not complete")
			}
			if prepareErr != nil || prepared == nil || beforePrepareCalls != parentCalls.Load()+childCalls.Load() {
				t.Errorf("same-root Prepare after normal shutdown did not complete without provider work")
			}
		})
	}
}

// Authentic terminal records are produced through production HTTP and a
// loopback provider. After all runtime owners stop, model the CAS/event cuts
// and let fresh production assembly repair or reject them before HTTP exposure.
func TestRuntimeAsyncTerminalRecoveryPreservesAuthenticOutcome(t *testing.T) {
	for _, cut := range []string{"success_missing_bundle", "failure_missing_bundle", "usage_missing", "partial_bundle", "forged_terminal"} {
		t.Run(cut, func(t *testing.T) {
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if strings.HasPrefix(cut, "failure") {
					w.WriteHeader(400)
					_, _ = w.Write([]byte(`{"error":{"message":"/private/terminal-recovery-sentinel"}}`))
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"The ordinary task is complete.\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"))
			}))
			defer provider.Close()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			config.BaseURL = provider.URL + "/v1"
			seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
			observations := make(chan server.AsyncTurnObservationV1, 4)
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) { observations <- o })
			inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			var handler http.Handler = &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
			defer func() {
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
			}()
			request := func(method, path string, body map[string]any, want int) map[string]any {
				t.Helper()
				encoded, _ := json.Marshal(body)
				req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
				req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != want {
					t.Fatalf("HTTP status=%d want=%d route=%s", rec.Code, want, path)
				}
				if strings.Contains(rec.Body.String(), "terminal-recovery-sentinel") || strings.Contains(rec.Body.String(), "/private/") {
					t.Fatal("HTTP exposed private failure")
				}
				var result map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			thread := request(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
			threadID := contracts.StringField(thread, "id")
			started := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Complete a concise ordinary response.", "async": true}, http.StatusAccepted)
			turnID := contracts.StringField(started, "turnId")
			for {
				select {
				case o := <-observations:
					if o.Stage == "finished" {
						goto drained
					}
				case <-time.After(20 * time.Second):
					t.Fatal("async producer did not complete")
				}
			}
		drained:
			before := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
			wantStatus := "completed"
			if strings.HasPrefix(cut, "failure") {
				wantStatus = "failed"
			}
			turns, ok := before["turns"].([]any)
			if turnID == "" || !ok || len(turns) != 1 || contracts.StringField(turns[0].(map[string]any), "id") != turnID || contracts.StringField(turns[0].(map[string]any), "status") != wantStatus {
				t.Fatal("producer did not commit the required authentic outcome")
			}
			shutdownOwnedRuntimeHandler(t, handler)
			handler = nil
			if calls.Load() != 1 {
				t.Fatalf("provider calls=%d want=1", calls.Load())
			}
			threadPath := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "thread.json")
			eventPath := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "events.jsonl")
			originalThread, err := os.ReadFile(threadPath)
			if err != nil {
				t.Fatal(err)
			}
			originalEvents, err := os.ReadFile(eventPath)
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(originalEvents), []byte("\n"))
			first := -1
			for index, line := range lines {
				var event map[string]any
				if json.Unmarshal(line, &event) != nil {
					t.Fatal("invalid event fixture")
				}
				if contracts.StringField(event, "generalTerminalEventId") != "" && first < 0 {
					first = index
				}
			}
			if first < 0 || len(lines)-first != 3 {
				t.Fatal("actual turn did not yield a final three-event bundle")
			}
			cutEvents := originalEvents
			switch cut {
			case "success_missing_bundle", "failure_missing_bundle":
				cutEvents = append(bytes.Join(lines[:first], []byte("\n")), '\n')
			case "partial_bundle":
				cutEvents = append(bytes.Join(lines[:first+1], []byte("\n")), '\n')
			case "usage_missing":
				if err := os.Remove(filepath.Join(config.ProductionDurableRoot, "usage_events", "index.jsonl")); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(config.ProductionDurableRoot, "usage_events", "threads", threadID+".jsonl")); err != nil {
					t.Fatal(err)
				}
			case "forged_terminal":
				var archived map[string]any
				if json.Unmarshal(originalThread, &archived) != nil {
					t.Fatal("invalid archive")
				}
				turns := archived["turns"].([]any)
				turns[len(turns)-1].(map[string]any)["status"] = "failed"
				altered, _ := json.Marshal(archived)
				if err := os.WriteFile(threadPath, altered, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(eventPath, cutEvents, 0600); err != nil {
				t.Fatal(err)
			}
			checkpoint := startupWholeTreeDigest(t, config.ProductionDurableRoot, config.DataDir)
			restarted, err := NewRuntimeServerHandlerE(config)
			if cut == "partial_bundle" || cut == "forged_terminal" {
				if err == nil || restarted != nil {
					if restarted != nil {
						shutdownOwnedRuntimeHandler(t, restarted)
					}
					t.Fatal("corrupt terminal authority activated HTTP")
				}
				if startupWholeTreeDigest(t, config.ProductionDurableRoot, config.DataDir) != checkpoint {
					t.Fatal("rejected terminal authority mutated persistence")
				}
				if calls.Load() != 1 {
					t.Fatal("rejected recovery called provider")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			handler = restarted
			after := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
			if !reflect.DeepEqual(before["turns"], after["turns"]) {
				t.Fatal("restart changed authentic public terminal")
			}
			recovered, err := os.ReadFile(eventPath)
			if err != nil || !bytes.Equal(originalEvents, recovered) {
				t.Fatal("restart lost, duplicated or changed terminal event bytes")
			}
			current, err := os.ReadFile(threadPath)
			if err != nil || !bytes.Equal(originalThread, current) {
				t.Fatal("restart changed canonical CAS winner")
			}
			// Read the real SSE route through its terminal bundle, then close the
			// connection; raw event equality alone does not prove public replay.
			live := httptest.NewServer(handler)
			req, err := http.NewRequest(http.MethodGet, live.URL+"/v1/threads/"+threadID+"/events?since_seq=0", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Do(req)
			if err != nil {
				live.Close()
				t.Fatal(err)
			}
			if response.StatusCode != 200 {
				response.Body.Close()
				live.Close()
				t.Fatal("SSE recovery unavailable")
			}
			publicBytes, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			live.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			public := string(publicBytes)
			if strings.Contains(public, "terminal-recovery-sentinel") || strings.Contains(public, "/private/") {
				t.Fatal("SSE exposed raw failure")
			}
			if !strings.Contains(public, "general_terminal_batch") {
				var kinds []string
				for _, line := range strings.Split(public, "\n") {
					if strings.HasPrefix(line, "data:") {
						var event map[string]any
						_ = json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event)
						kinds = append(kinds, contracts.StringField(event, "kind")+":"+contracts.StringField(event, "code"))
					}
				}
				t.Fatalf("SSE omitted terminal bundle: kinds=%v", kinds)
			}
			if !strings.Contains(public, turnID) {
				t.Fatal("SSE terminal belongs to another turn")
			}
			shutdownOwnedRuntimeHandler(t, handler)
			handler = nil
			handler, err = NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			again, err := os.ReadFile(eventPath)
			if err != nil || !bytes.Equal(originalEvents, again) {
				t.Fatal("second production restart duplicated terminal")
			}
			if calls.Load() != 1 {
				t.Fatal("restart replay fabricated new provider work")
			}
		})
	}
}

// A real archive I/O failure occurs while the async producer is in flight.
// Restart owns an authenticated restart disposition for the uncommitted turn;
// provider failure is not evidence that a failed CAS existed before restart.
func TestRuntimeAsyncTerminalPrecommitFailureGetsAuthenticRestartDisposition(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"/private/precommit-recovery-sentinel"}}`))
	}))
	defer provider.Close()
	_, config := runtimeWitnessedRegistryConfigV2(t)
	config.BaseURL = provider.URL + "/v1"
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	observations := make(chan server.AsyncTurnObservationV1, 4)
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) { observations <- o })
	inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	var handler http.Handler = &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
	var restore func()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		if handler != nil {
			shutdownOwnedRuntimeHandler(t, handler)
		}
		if restore != nil {
			restore()
		}
	}()
	thread := runtimeStartupJSON(t, handler, http.MethodPost, "/v1/threads", map[string]any{"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
	threadID := contracts.StringField(thread, "id")
	started := runtimeStartupJSON(t, handler, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Complete an ordinary response.", "async": true}, http.StatusAccepted)
	turnID := contracts.StringField(started, "turnId")
	select {
	case <-entered:
	case <-time.After(20 * time.Second):
		t.Fatal("provider did not reach controlled precommit barrier")
	}
	threadPath := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "thread.json")
	backupPath := threadPath + ".precommit-fixture"
	if err := os.Rename(threadPath, backupPath); err != nil {
		t.Fatal(err)
	}
	restore = func() {
		if err := os.Remove(threadPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Rename(backupPath, threadPath); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(threadPath, 0700); err != nil {
		t.Fatal(err)
	}
	close(release)
	for {
		select {
		case o := <-observations:
			if o.Stage == "finished" {
				if o.CompletionErrorClass == "none" || o.FailureRecordErrorClass == "none" || o.TerminalStatus != "unobservable" {
					t.Fatal("precommit archive failure was not fully observed")
				}
				goto drained
			}
		case <-time.After(20 * time.Second):
			t.Fatal("precommit failure did not drain")
		}
	}
drained:
	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	restore()
	restore = nil
	precommit, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawBefore map[string]any
	if json.Unmarshal(precommit, &rawBefore) != nil {
		t.Fatal("invalid precommit archive")
	}
	beforeTurn := rawBefore["turns"].([]any)[0].(map[string]any)
	if contracts.StringField(beforeTurn, "status") != "running" || beforeTurn["generalTerminalPublication"] != nil {
		t.Fatal("precommit failure unexpectedly committed a terminal")
	}
	handler, err = NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	detail := runtimeStartupJSON(t, handler, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
	turns := detail["turns"].([]any)
	if turnID == "" || len(turns) != 1 || contracts.StringField(turns[0].(map[string]any), "id") != turnID || contracts.StringField(turns[0].(map[string]any), "status") != "aborted" {
		t.Fatal("restart lost the uncommitted turn or exposed another outcome")
	}
	rawBytes, err := os.ReadFile(threadPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if json.Unmarshal(rawBytes, &raw) != nil {
		t.Fatal("invalid restart archive")
	}
	eventPath := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "events.jsonl")
	eventBytes, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(eventBytes), []byte("\n")) {
		var event map[string]any
		if json.Unmarshal(line, &event) != nil {
			t.Fatal("invalid restart event inventory")
		}
		events = append(events, event)
	}
	entries, err := appturn.PreflightGeneralTerminalPublicationInventoryV1(raw, events)
	if err != nil || len(entries) != 1 || entries[0].TurnID != turnID || entries[0].State != appturn.GeneralTerminalPublicationCompleteV1 || entries[0].Commit.TerminalReason != "restart" || entries[0].Commit.TerminalStatus != "aborted" {
		t.Fatal("restart terminal lacks its complete authentic CAS/event authority")
	}
	public, _ := json.Marshal(detail)
	if bytes.Contains(public, []byte("precommit-recovery-sentinel")) || bytes.Contains(public, []byte(config.ProductionDurableRoot)) {
		t.Fatal("recovered HTTP exposed private failure input")
	}
	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	checkpoint := startupWholeTreeDigest(t, config.ProductionDurableRoot)
	checkpointRecords := startupWholeTreeRecordMapForTest(t, config.ProductionDurableRoot)
	handler, err = NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	treeChanged := startupWholeTreeDigest(t, config.ProductionDurableRoot) != checkpoint
	if treeChanged || calls.Load() != 1 {
		currentRecords := startupWholeTreeRecordMapForTest(t, config.ProductionDurableRoot)
		changedKeys := changedWholeTreeRecordKeysForTest(checkpointRecords, currentRecords)
		regularChanged, directoryOrPresenceChanged := 0, 0
		for _, key := range changedKeys {
			if len(strings.Split(checkpointRecords[key], ":")) == 3 || len(strings.Split(currentRecords[key], ":")) == 3 {
				regularChanged++
			} else {
				directoryOrPresenceChanged++
			}
		}
		changed := func(relative string) bool {
			key := "0:" + filepath.ToSlash(relative)
			before, beforePresent := checkpointRecords[key]
			after, afterPresent := currentRecords[key]
			return beforePresent != afterPresent || before != after
		}
		currentBytes, readErr := os.ReadFile(threadPath)
		var current map[string]any
		primaryValid := readErr == nil && json.Unmarshal(currentBytes, &current) == nil
		var currentTerminal any
		currentTurns, _ := current["turns"].([]any)
		for _, value := range currentTurns {
			turn, _ := value.(map[string]any)
			if contracts.StringField(turn, "id") == turnID {
				currentTerminal = turn["generalTerminalPublication"]
			}
		}
		originalTerminal := raw["turns"].([]any)[0].(map[string]any)["generalTerminalPublication"]
		// Only fixed field names, booleans and counts leave the fixture. Paths,
		// IDs, hashes, terminal payloads and event bodies remain private.
		t.Fatalf("second restart changed the authenticated terminal or reexecuted the producer: treeChanged=%t providerCalls=%d changedEntries=%d regularChanged=%d directoryOrPresenceChanged=%d rootEntryChanged=%t threadDirectoryChanged=%t threadJSONChanged=%t eventsChanged=%t metadataChanged=%t messagesChanged=%t summariesChanged=%t usageIndexChanged=%t primaryValid=%t terminalChanged=%t",
			treeChanged, calls.Load(), len(changedKeys), regularChanged, directoryOrPresenceChanged,
			changed("."), changed(filepath.Join("threads", threadID)),
			changed(filepath.Join("threads", threadID, "thread.json")),
			changed(filepath.Join("threads", threadID, "events.jsonl")),
			changed(filepath.Join("threads", threadID, "metadata.jsonl")),
			changed(filepath.Join("threads", threadID, "messages.jsonl")),
			changed("thread_summaries.jsonl"), changed(filepath.Join("usage_events", "index.jsonl")),
			primaryValid, !reflect.DeepEqual(originalTerminal, currentTerminal))
	}
}
