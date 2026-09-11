//go:build !analytix_prod

package runtimego

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	runtimeapp "analytix.local/runtime-go/internal/runtimeapp"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeProviderContinuationUsesSignedPendingWorkAuthority(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("PRIVATE_NOTE_CONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := newCompleteProviderServer(t, [][]string{
		{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		},
		{
			`data: {"choices":[{"delta":{"content":"read complete"}}]}`,
			`data: [DONE]`,
		},
	})
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "pending-provider", "pending-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Pending authority", "workspace": workspace, "providerId": "pending-provider", "model": "pending-model",
	}), http.StatusCreated)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+stringField(thread, "id")+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read note.txt."}), http.StatusAccepted)
	if provider.RequestCount() != 2 {
		t.Fatalf("tool continuation request count mismatch: %d", provider.RequestCount())
	}
	receipts, dispositions := loadPendingWorkRecords(t, dataDir)
	assertClosedPendingKinds(t, receipts, dispositions, map[string]int{
		domainpendingwork.KindToolBatch: 1, domainpendingwork.KindProviderContinuation: 1,
	})
	for _, receipt := range receipts {
		body, _ := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
		if strings.Contains(string(body), "PRIVATE_NOTE_CONTENT") || strings.Contains(string(body), "Read note.txt") {
			t.Fatalf("pending-work authority leaked private semantic bytes: %s", body)
		}
		if receipt.Kind == domainpendingwork.KindProviderContinuation &&
			(len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].ResultItemDigest == "") {
			t.Fatalf("provider continuation omitted exact durable result authority: %#v", receipt.GrantMembers)
		}
	}
}

func TestRuntimeProviderRetryReusesOnePendingWorkAuthority(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("retry authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requestCount atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := requestCount.Add(1)
		if attempt == 2 {
			http.Error(w, "temporary provider failure", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
		} else {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"retry complete"},"finish_reason":"stop"}]}` + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "retry-provider", "retry-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Provider retry authority", "workspace": workspace, "providerId": "retry-provider", "model": "retry-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	response, responseBody := liveRequest(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read note.txt and retry safely."}),
	)
	if response.StatusCode != http.StatusAccepted {
		replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
		t.Fatalf(
			"provider retry turn status mismatch: got=%d want=%d requests=%d body=%s events=%s",
			response.StatusCode, http.StatusAccepted, requestCount.Load(), responseBody, replay,
		)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("provider retry request count mismatch: %d", requestCount.Load())
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if !strings.Contains(replay, `"stage":"provider_retrying"`) || strings.Contains(replay, `"code":"sse_setup_error"`) {
		t.Fatalf("provider retry did not remain durable and replayable: %s", replay)
	}
	receipts, dispositions := loadPendingWorkRecords(t, dataDir)
	assertClosedPendingKinds(t, receipts, dispositions, map[string]int{
		domainpendingwork.KindToolBatch: 1, domainpendingwork.KindProviderContinuation: 1,
	})
}

func TestRuntimeProviderPreOutputReconnectRevalidatesOnePendingWorkAuthority(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("reconnect authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requestCount atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := requestCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch attempt {
		case 1:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case 2:
			_, _ = w.Write([]byte("data: "))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				connection, _, _ := hijacker.Hijack()
				_ = connection.Close()
			}
		default:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"reconnect complete"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "reconnect-provider", "reconnect-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Provider reconnect authority", "workspace": workspace, "providerId": "reconnect-provider", "model": "reconnect-model",
	}), http.StatusCreated)
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+stringField(thread, "id")+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read note.txt and reconnect safely."}), http.StatusAccepted)
	if requestCount.Load() != 3 {
		t.Fatalf("provider reconnect request count mismatch: %d", requestCount.Load())
	}
	receipts, dispositions := loadPendingWorkRecords(t, dataDir)
	assertClosedPendingKinds(t, receipts, dispositions, map[string]int{
		domainpendingwork.KindToolBatch: 1, domainpendingwork.KindProviderContinuation: 1,
	})
}

func TestRuntimeProviderPreOutputReconnectRejectsClosedAuthority(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("closed authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	var lease *persistencefs.CompositeLease
	var requestCount atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := requestCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		if attempt == 2 {
			pendingStore, openErr := newRuntimeHostBoundPendingWorkStore(t, dataDir, lease)
			if openErr != nil {
				t.Errorf("open pending work: %v", openErr)
				return
			}
			receipts, listErr := pendingStore.ListReceipts(context.Background())
			if listErr != nil {
				t.Errorf("list pending work: %v", listErr)
				return
			}
			var continuation domainpendingwork.PendingWorkReceiptV1
			for _, receipt := range receipts {
				if receipt.Kind == domainpendingwork.KindProviderContinuation {
					continuation = receipt
				}
			}
			if continuation.WorkID == "" {
				t.Error("provider continuation receipt was not durable before send")
				return
			}
			disposition, dispositionErr := domainpendingwork.NewPendingWorkDispositionV1(
				continuation, domainpendingwork.StatusRejected, "test_revoked", time.Now().UTC(),
				authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
					return authority.Sign(context.Background(), message)
				},
			)
			if dispositionErr != nil || pendingStore.PutDispositionIfAbsent(context.Background(), disposition) != nil {
				t.Errorf("revoke provider continuation: %v", dispositionErr)
				return
			}
			_, _ = w.Write([]byte("data: "))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				connection, _, _ := hijacker.Hijack()
				_ = connection.Close()
			}
			return
		}
		t.Error("closed continuation authority allowed another physical provider request")
	}))
	defer provider.Close()
	config := RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "closed-provider", "closed-model"),
	}
	lease, err = runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := runtimeapp.NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if lifecycle, ok := handler.(interface{ Shutdown(context.Context) error }); ok {
			if err := lifecycle.Shutdown(context.Background()); err != nil {
				t.Errorf("shutdown runtime test handler: %v", err)
			}
		}
		if err := lease.Close(); err != nil {
			t.Errorf("close runtime test persistence lease: %v", err)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Closed provider authority", "workspace": workspace, "providerId": "closed-provider", "model": "closed-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	failed := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read note.txt without bypassing closed authority."}), http.StatusInternalServerError)
	if stringField(failed, "code") != "turn_failed" {
		t.Fatalf("closed continuation authority returned the wrong failure: %#v", failed)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("closed continuation authority did not stop reconnect: requests=%d", requestCount.Load())
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	if strings.Contains(replay, "reconnect complete") || !strings.Contains(replay, "turn_failed") {
		t.Fatalf("closed continuation authority did not fail the turn closed: %s", replay)
	}
}

func TestRuntimeProviderStreamRecoveryClosesEachLogicalContinuation(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(dataDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("stream recovery authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requestCount atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := requestCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		switch attempt {
		case 1:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case 2:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"partial before recovery "}}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				connection, _, _ := hijacker.Hijack()
				_ = connection.Close()
			}
		default:
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"recovered final"},"finish_reason":"stop"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer provider.Close()
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
		ModelProvidersJSON: testModelProvidersJSON(provider.URL, "recovery-provider", "recovery-model"),
	}))
	defer server.Close()
	thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title": "Provider stream recovery authority", "workspace": workspace, "providerId": "recovery-provider", "model": "recovery-model",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	started := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken,
		mustJSON(t, map[string]any{"prompt": "Read note.txt and recover the stream safely."}), http.StatusAccepted)
	turnID := stringField(started, "turnId")
	if requestCount.Load() != 3 {
		t.Fatalf("provider stream recovery request count mismatch: %d", requestCount.Load())
	}
	receipts, dispositions := loadPendingWorkRecords(t, dataDir)
	if len(receipts) != 3 || len(dispositions) != 3 {
		t.Fatalf("stream recovery pending work inventory mismatch: receipts=%#v dispositions=%#v", receipts, dispositions)
	}
	statusByWork := map[string]string{}
	for _, disposition := range dispositions {
		statusByWork[disposition.WorkID] = disposition.Status
	}
	statusesByKind := map[string]map[string]int{}
	for _, receipt := range receipts {
		status := statusByWork[receipt.WorkID]
		if receipt.Kind != domainpendingwork.KindToolBatch && receipt.Kind != domainpendingwork.KindProviderContinuation {
			t.Fatalf("unexpected pending work kind: %s", receipt.Kind)
		}
		if status == "" {
			t.Fatalf("logical pending work remained open after stream recovery: kind=%s work=%s", receipt.Kind, receipt.WorkID)
		}
		if statusesByKind[receipt.Kind] == nil {
			statusesByKind[receipt.Kind] = map[string]int{}
		}
		statusesByKind[receipt.Kind][status]++
	}
	if statusesByKind[domainpendingwork.KindToolBatch][domainpendingwork.StatusCompleted] != 1 ||
		statusesByKind[domainpendingwork.KindProviderContinuation][domainpendingwork.StatusFailed] != 1 ||
		statusesByKind[domainpendingwork.KindProviderContinuation][domainpendingwork.StatusCompleted] != 1 {
		t.Fatalf("stream recovery omitted an exact logical pending-work settlement: %#v", statusesByKind)
	}
	replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	usage := firstRuntimeServerEvent(t, runtimeServerEventsForTurn(t, replay, turnID), "usage")
	diagnostics := mapField(t, usage, "cacheDiagnostics")
	statuses := mapField(t, diagnostics, "providerAttemptStatuses")
	if diagnostics["providerAttemptTelemetryValid"] != true ||
		diagnostics["providerAttemptCount"] != float64(3) ||
		diagnostics["providerLogicalCallCount"] != float64(3) ||
		statuses["streamAborted"] != float64(1) || statuses["succeeded"] != float64(2) {
		t.Fatalf("provider tool-loop, abort, and recovery telemetry was not aggregated distinctly: %#v", diagnostics)
	}
}

func TestRuntimePendingGateRetainsBatchRefsForApprovalAndUserInput(t *testing.T) {
	for _, test := range []struct {
		name, toolCall, pendingKind, resolvePath string
		resolveBody                              map[string]any
		expectedProviderRequests                 int
		expectedPendingKinds                     map[string]int
		expectedContinuationMembers              int
	}{
		{
			name: "approval deny", pendingKind: "approval", resolvePath: "/v1/approvals/",
			toolCall:                 `{"index":1,"id":"call_write","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"out.txt\",\"content\":\"blocked\"}"}}`,
			resolveBody:              map[string]any{"decision": "deny"},
			expectedProviderRequests: 1,
			expectedPendingKinds: map[string]int{
				domainpendingwork.KindToolBatch: 1,
			},
		},
		{
			name: "user input", pendingKind: "user_input", resolvePath: "/v1/user-inputs/",
			toolCall:                 `{"index":1,"id":"call_input","type":"function","function":{"name":"request_user_input","arguments":"{\"prompt\":\"Choose\"}"}}`,
			resolveBody:              map[string]any{"answers": []map[string]string{{"id": "q1", "label": "Choice", "value": "continue"}}},
			expectedProviderRequests: 2,
			expectedPendingKinds: map[string]int{
				domainpendingwork.KindToolBatch: 1, domainpendingwork.KindProviderContinuation: 1,
			},
			expectedContinuationMembers: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			workspace := filepath.Join(dataDir, "workspace")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("prior batch"), 0o600); err != nil {
				t.Fatal(err)
			}
			provider := newCompleteProviderServer(t, [][]string{
				{
					`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"note.txt\"}"}},` + test.toolCall + `]},"finish_reason":"tool_calls"}]}`,
					`data: [DONE]`,
				},
				{`data: {"choices":[{"delta":{"content":"continued"}}]}`, `data: [DONE]`},
			})
			server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
				RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
				ModelProvidersJSON: testModelProvidersJSON(provider.URL(), "gate-provider", "gate-model"),
			}))
			defer server.Close()
			thread := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
				"title": test.name, "workspace": workspace, "providerId": "gate-provider", "model": "gate-model",
				"approvalPolicy": "on-request", "sandboxMode": "workspace-write",
			}), http.StatusCreated)
			threadID := stringField(thread, "id")
			response, responseBody := liveRequest(t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken,
				mustJSON(t, map[string]any{"prompt": "Use the requested tools."}))
			if response.StatusCode != http.StatusAccepted {
				replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
				t.Fatalf("pending gate turn status mismatch: got=%d want=%d body=%s events=%s",
					response.StatusCode, http.StatusAccepted, responseBody, replay)
			}
			var start map[string]any
			if err := json.Unmarshal(responseBody, &start); err != nil {
				t.Fatalf("decode pending gate turn response: %v body=%s", err, responseBody)
			}
			if stringField(start, "pendingKind") != test.pendingKind {
				t.Fatalf("pending gate mismatch: %#v", start)
			}
			assertLiveJSON(t, server.URL, http.MethodPost, test.resolvePath+stringField(start, "pendingId"), DefaultRuntimeToken,
				mustJSON(t, test.resolveBody), http.StatusOK)
			if provider.RequestCount() != test.expectedProviderRequests {
				t.Fatalf("gate continuation request count mismatch: %d", provider.RequestCount())
			}
			receipts, dispositions := loadPendingWorkRecords(t, dataDir)
			assertClosedPendingKinds(t, receipts, dispositions, test.expectedPendingKinds)
			for _, receipt := range receipts {
				if receipt.Kind == domainpendingwork.KindProviderContinuation && len(receipt.GrantMembers) != test.expectedContinuationMembers {
					t.Fatalf("gate continuation lost prior batch or current gate result: %#v", receipt.GrantMembers)
				}
			}
			replay := liveSSE(t, server.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
			if strings.Contains(replay, "priorSettledToolRefs") {
				t.Fatalf("private settled references leaked into public SSE: %s", replay)
			}
		})
	}
}

func TestRuntimeStartupClosesOpenPendingWorkBeforeRecovery(t *testing.T) {
	dataDir := t.TempDir()
	authority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-crashed", TurnID: "turn-crashed", WorkspaceRealPath: filepath.Join(dataDir, "workspace"),
		ContextEpoch: 1, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindToolBatch, SecurityContext: securityContext, GrantRegistrySequence: 1,
		GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry")),
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant")), RegistrySequence: 1,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("payload")), RouteHash: domainsecurity.SHA256Hex([]byte("route")),
		IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatalf("seed open pending work: receiptErr=%v", err)
	}
	config := RuntimeServerContractConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: dataDir,
	}
	lease, err := runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	seedRuntimePendingWorkReceipt(t, dataDir, lease, receipt)
	handler, err := runtimeapp.NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if lifecycle, ok := handler.(interface{ Shutdown(context.Context) error }); ok {
			if err := lifecycle.Shutdown(context.Background()); err != nil {
				t.Errorf("shutdown runtime test handler: %v", err)
			}
		}
		if err := lease.Close(); err != nil {
			t.Errorf("close runtime test persistence lease: %v", err)
		}
	})
	store, err := newRuntimeHostBoundPendingWorkStore(t, dataDir, lease)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := store.ReadDisposition(context.Background(), receipt.WorkID)
	if err != nil || disposition.Status != domainpendingwork.StatusRestartInvalid || disposition.ReasonCode != "restart_invalid" {
		t.Fatalf("startup did not close open pending work before recovery: disposition=%#v err=%v", disposition, err)
	}
}

func loadPendingWorkRecords(t *testing.T, dataDir string) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1) {
	t.Helper()
	receipts := []domainpendingwork.PendingWorkReceiptV1{}
	dispositions := []domainpendingwork.PendingWorkDispositionV1{}
	root := filepath.Join(dataDir, "private", "pending-work")
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(path, string(filepath.Separator)+"receipts"+string(filepath.Separator)) {
			receipt, parseErr := domainpendingwork.ParsePendingWorkReceiptV1(body)
			if parseErr != nil {
				return parseErr
			}
			receipts = append(receipts, receipt)
		} else if strings.Contains(path, string(filepath.Separator)+"dispositions"+string(filepath.Separator)) {
			disposition, parseErr := domainpendingwork.ParsePendingWorkDispositionV1(body)
			if parseErr != nil {
				return parseErr
			}
			dispositions = append(dispositions, disposition)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].WorkID < receipts[j].WorkID })
	sort.Slice(dispositions, func(i, j int) bool { return dispositions[i].WorkID < dispositions[j].WorkID })
	return receipts, dispositions
}

func assertClosedPendingKinds(t *testing.T, receipts []domainpendingwork.PendingWorkReceiptV1, dispositions []domainpendingwork.PendingWorkDispositionV1, expected map[string]int) {
	t.Helper()
	counts := map[string]int{}
	closed := map[string]domainpendingwork.PendingWorkDispositionV1{}
	for _, disposition := range dispositions {
		closed[disposition.WorkID] = disposition
	}
	for _, receipt := range receipts {
		counts[receipt.Kind]++
		disposition, found := closed[receipt.WorkID]
		if !found || disposition.Status != domainpendingwork.StatusCompleted {
			t.Fatalf("pending work remained open or non-completed: receipt=%#v disposition=%#v", receipt, disposition)
		}
	}
	if len(receipts) != len(dispositions) || len(counts) != len(expected) {
		t.Fatalf("pending work inventory mismatch: counts=%#v receipts=%d dispositions=%d", counts, len(receipts), len(dispositions))
	}
	for kind, count := range expected {
		if counts[kind] != count {
			t.Fatalf("pending work kind count mismatch kind=%s got=%d want=%d all=%#v", kind, counts[kind], count, counts)
		}
	}
}
