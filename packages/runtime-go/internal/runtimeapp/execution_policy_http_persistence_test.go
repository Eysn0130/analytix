//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
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

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestExecutionPolicyHTTPPersistenceCreatedEmptyThreadPatchAfterRestartV1(t *testing.T) {
	_, config := runtimeWitnessedRegistryConfigV2(t)
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	var handler http.Handler
	defer func() {
		if handler != nil {
			shutdownOwnedRuntimeHandler(t, handler)
		}
	}()
	var err error
	handler, err = NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	// This regression uses only the public create route; unlike the legacy
	// marker matrix below, it never seeds or edits a durable thread record.
	created := executionPolicyHTTPV1(t, handler, http.MethodPost, "/v1/threads", map[string]any{
		"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model,
		"approvalPolicy": "on-request", "sandboxMode": "workspace-write",
	}, http.StatusCreated)
	threadID := contracts.StringField(created, "id")
	primary := executionPolicyReadPrimaryV1(t, config, threadID)
	turns, ok := primary["turns"].([]any)
	if !ok || len(turns) != 0 || primary["securityState"] != nil || primary["contextEpochState"] != nil {
		t.Fatal("public creation did not persist an empty thread without turn authority or epoch")
	}
	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	handler, err = NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	restored := executionPolicyReadPrimaryV1(t, config, threadID)
	state, err := domaincontextepoch.ParseState(restored["contextEpochState"])
	if err != nil {
		t.Fatal("normal restart did not persist a valid default epoch for the empty thread")
	}
	at, err := time.Parse(time.RFC3339Nano, state.AcceptedSnapshot.AcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := contextepochapp.DefaultState(threadID, 1, at)
	if err != nil || state.ThreadID != threadID || state.StateDigest != initial.StateDigest || restored["securityState"] != nil {
		t.Fatal("normal restart did not produce the exact empty-thread default epoch shape")
	}
	epochBefore, err := json.Marshal(restored["contextEpochState"])
	if err != nil {
		t.Fatal(err)
	}
	patched := executionPolicyHTTPV1(t, handler, http.MethodPatch, "/v1/threads/"+threadID,
		map[string]any{"approvalPolicy": "always", "sandboxMode": "read-only"}, http.StatusOK)
	assertExecutionPolicyPairV1(t, patched, "always", "read-only")
	persisted := executionPolicyReadPrimaryV1(t, config, threadID)
	assertExecutionPolicyPairV1(t, persisted, "always", "read-only")
	epochAfter, err := json.Marshal(persisted["contextEpochState"])
	turns, ok = persisted["turns"].([]any)
	if err != nil || !bytes.Equal(epochBefore, epochAfter) || !ok || len(turns) != 0 ||
		persisted["securityState"] != nil || persisted["executionPolicyVersion"] != float64(2) {
		t.Fatal("metadata policy patch changed the restored epoch or invented turn authority/history")
	}
}

func TestExecutionPolicyHTTPPersistencePatchAcrossRestartAndNextStartV1(t *testing.T) {
	for _, scenario := range []struct {
		name              string
		version           int
		patch             map[string]any
		approval, sandbox string
		patchedVersion    int
	}{
		{"same-full-pair", 0, map[string]any{"approvalPolicy": "auto", "sandboxMode": "danger-full-access"}, "auto", "danger-full-access", 2},
		{"custom-pair", 0, map[string]any{"approvalPolicy": "always", "sandboxMode": "read-only"}, "always", "read-only", 2},
		{"restricted-pair", 0, map[string]any{"approvalPolicy": "on-request", "sandboxMode": "workspace-write"}, "on-request", "workspace-write", 2},
		{"future-version", 3, map[string]any{"approvalPolicy": "untrusted", "sandboxMode": "danger-full-access"}, "untrusted", "danger-full-access", 3},
		{"untouched-legacy", 0, map[string]any{"title": "Unrelated metadata edit"}, "on-request", "workspace-write", 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var providerCalls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				providerCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Synthetic policy test complete.\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
			}))
			defer provider.Close()
			_, config := runtimeWitnessedRegistryConfigV2(t)
			config.BaseURL = provider.URL + "/v1"
			seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
			workspace := workspacetest.New(t)
			const threadID = "thr_execution_policy_http"
			thread := threadapp.BuildThread(threadapp.CreateInput{
				ThreadID: threadID, FallbackWorkspace: workspace, Now: time.Now().UTC().Format(time.RFC3339Nano),
				Request: map[string]any{"title": "Synthetic legacy policy", "workspace": workspace,
					"providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "auto", "sandboxMode": "danger-full-access"},
			})
			// Seed a complete historical primary before constructing any store.
			// Startup owns creation of sidecars and indexes from this same marker.
			thread["executionPolicyVersion"] = float64(scenario.version)
			body, err := json.Marshal(thread)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(executionPolicyPrimaryPathV1(config, threadID)), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(executionPolicyPrimaryPathV1(config, threadID), body, 0o600); err != nil {
				t.Fatal(err)
			}
			var handler http.Handler
			defer func() {
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
			}()
			handler, err = NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			path := "/v1/threads/" + threadID
			before := executionPolicyHTTPV1(t, handler, http.MethodGet, path, nil, http.StatusOK)
			if before["executionPolicyVersion"] != float64(scenario.version) {
				t.Fatal("startup changed the historical policy marker before explicit intent")
			}
			if scenario.name == "same-full-pair" {
				for _, partial := range []map[string]any{{"approvalPolicy": "auto"}, {"sandboxMode": "danger-full-access"}} {
					accepted := executionPolicyHTTPV1(t, handler, http.MethodPatch, path, partial, http.StatusOK)
					assertExecutionPolicyPairV1(t, accepted, "auto", "danger-full-access")
					if accepted["executionPolicyVersion"] != float64(0) {
						t.Fatal("legal partial policy patch sealed an incomplete explicit intent")
					}
				}
				for _, invalid := range []map[string]any{
					{"approvalPolicy": "invalid", "sandboxMode": "workspace-write"},
					{"approvalPolicy": "on-request", "sandboxMode": "invalid"},
					{"approvalPolicy": "loading"}, {"sandboxMode": nil},
					{"approvalPolicy": float64(1), "sandboxMode": "read-only"},
					{"executionPolicyVersion": float64(2)},
					{"approvalPolicy": "auto", "sandboxMode": "danger-full-access", "executionPolicyVersion": float64(2)},
				} {
					unchanged := startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot)
					executionPolicyHTTPV1(t, handler, http.MethodPatch, path, invalid, http.StatusBadRequest)
					if startupWholeTreeDigest(t, config.DataDir, config.ProductionDurableRoot) != unchanged {
						t.Fatal("rejected public policy patch changed durable state")
					}
				}
			}
			patched := executionPolicyHTTPV1(t, handler, http.MethodPatch, path, scenario.patch, http.StatusOK)
			if patched["executionPolicyVersion"] != float64(scenario.patchedVersion) {
				t.Fatal("explicit policy intent did not seal the correct runtime-owned version")
			}
			if scenario.name == "untouched-legacy" {
				assertExecutionPolicyPairV1(t, patched, "auto", "danger-full-access")
			} else {
				assertExecutionPolicyPairV1(t, patched, scenario.approval, scenario.sandbox)
			}
			if providerCalls.Load() != 0 {
				t.Fatal("metadata patch invoked the Provider")
			}
			shutdownOwnedRuntimeHandler(t, handler)
			handler = nil
			handler, err = NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			reopened := executionPolicyHTTPV1(t, handler, http.MethodGet, path, nil, http.StatusOK)
			if reopened["executionPolicyVersion"] != float64(scenario.patchedVersion) || reopened["approvalPolicy"] != patched["approvalPolicy"] || reopened["sandboxMode"] != patched["sandboxMode"] {
				t.Fatal("reopen changed the persisted explicit policy intent")
			}
			// The real HTTP route returns 202 for both modes; async:false waits
			// for completeStartedRuntimeTurn rather than launching its goroutine.
			started := executionPolicyHTTPV1(t, handler, http.MethodPost, path+"/turns", map[string]any{"prompt": "Reply with a short ordinary greeting.", "async": false, "disableUserInput": true}, http.StatusAccepted)
			completed := executionPolicyHTTPV1(t, handler, http.MethodGet, path, nil, http.StatusOK)
			turnID := contracts.StringField(started, "turnId")
			publicTurn := executionPolicyTurnV1(t, completed, turnID)
			turn := executionPolicyTurnV1(t, executionPolicyReadPrimaryV1(t, config, threadID), turnID)
			assertExecutionPolicyPairV1(t, turn, scenario.approval, scenario.sandbox)
			assertExecutionPolicyPairV1(t, completed, scenario.approval, scenario.sandbox)
			wantVersion := scenario.patchedVersion
			if wantVersion == 0 {
				wantVersion = 2
			}
			if completed["executionPolicyVersion"] != float64(wantVersion) || publicTurn["status"] != "completed" || providerCalls.Load() != 1 {
				t.Fatal("actual next turn lost its effective pair, marker or completed Provider path")
			}
		})
	}
}

func TestExecutionPolicyHTTPPersistencePreservesActualPendingCallV1(t *testing.T) {
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := providerCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{`data: {"type":"message_start","message":{"id":"synthetic_policy_message","model":"policy-model","usage":{"input_tokens":2,"output_tokens":0}}}`}
		if call == 1 {
			args, _ := json.Marshal(`{"path":"policy-denied.txt","content":"must not be written"}`)
			frames = append(frames,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"wire_policy_write","name":"write","input":{}}}`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+string(args)+`}}`,
				`data: {"type":"content_block_stop","index":0}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`)
		} else {
			frames = append(frames,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"Synthetic denial handled."}}`,
				`data: {"type":"content_block_stop","index":0}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
		}
		frames = append(frames, `data: {"type":"message_stop"}`)
		_, _ = io.WriteString(w, strings.Join(frames, "\n\n")+"\n\n")
	}))
	defer provider.Close()
	_, config := runtimeWitnessedRegistryConfigV2(t)
	config.BaseURL, config.EndpointFormat = provider.URL+"/v1", "messages"
	config.Model = "policy-model"
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only", "anthropic-compatible")
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOwnedRuntimeHandler(t, handler)
	workspace := workspacetest.New(t)
	thread := executionPolicyHTTPV1(t, handler, http.MethodPost, "/v1/threads", map[string]any{"workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "on-request", "sandboxMode": "workspace-write"}, http.StatusCreated)
	threadID := contracts.StringField(thread, "id")
	path := "/v1/threads/" + threadID
	started := executionPolicyHTTPV1(t, handler, http.MethodPost, path+"/turns", map[string]any{"prompt": "Write a short ordinary note to policy-denied.txt.", "async": false, "maxModelSteps": 3}, http.StatusAccepted)
	turnID := contracts.StringField(started, "turnId")
	paused := executionPolicyHTTPV1(t, handler, http.MethodGet, path, nil, http.StatusOK)
	turn := executionPolicyTurnV1(t, paused, turnID)
	durableTurn := executionPolicyTurnV1(t, executionPolicyReadPrimaryV1(t, config, threadID), turnID)
	assertExecutionPolicyPairV1(t, durableTurn, "on-request", "workspace-write")
	var gate map[string]any
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["kind"] == "approval" && item["status"] == "pending" {
			if gate != nil {
				t.Fatal("actual turn emitted duplicate pending approvals")
			}
			gate = item
		}
	}
	approvalID := contracts.StringField(gate, "approvalId")
	if approvalID == "" || providerCalls.Load() != 1 {
		t.Fatal("actual write did not stop at the existing approval owner")
	}
	var privateGate map[string]any
	privateItems, _ := durableTurn["items"].([]any)
	for _, raw := range privateItems {
		item, _ := raw.(map[string]any)
		if item["kind"] == "approval" && item["status"] == "pending" && item["approvalId"] == approvalID {
			if privateGate != nil {
				t.Fatal("actual durable turn emitted duplicate pending approvals")
			}
			privateGate = item
		}
	}
	if privateGate == nil || privateGate["id"] != gate["id"] {
		t.Fatal("public approval handle does not identify the actual private pending gate")
	}
	digest := strings.TrimPrefix(approvalID, "appr_")
	if len(digest) != 64 {
		t.Fatal("actual approval has no canonical gate identity")
	}
	receiptPath := filepath.Join(config.DataDir, "private", "gate-continuations", "receipts-v2", digest[:2], digest+".json")
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domaincontinuation.ParseReceipt(receiptBefore)
	if err != nil {
		t.Fatal("actual pending receipt is invalid")
	}
	_, publicKey, signature, err := domaincontinuation.AuthorityMaterial(receipt)
	if err != nil || !ed25519.Verify(publicKey, domaincontinuation.ReceiptSigningBytes(receipt), signature) {
		t.Fatal("actual pending receipt signature is invalid")
	}
	if receipt.ReceiptID != privateGate["continuationReceiptId"] || receipt.Payload.ItemID != privateGate["id"] || receipt.Payload.GateID != approvalID || receipt.Payload.ThreadID != threadID || receipt.Payload.TurnID != turnID || receipt.Payload.ToolName != "write" || receipt.Payload.ApprovalPolicy != durableTurn["approvalPolicy"] || receipt.Payload.SandboxMode != durableTurn["sandboxMode"] {
		t.Fatal("actual pending call did not capture the same frozen turn policy")
	}
	patched := executionPolicyHTTPV1(t, handler, http.MethodPatch, path, map[string]any{"approvalPolicy": "auto", "sandboxMode": "danger-full-access"}, http.StatusOK)
	assertExecutionPolicyPairV1(t, patched, "auto", "danger-full-access")
	assertExecutionPolicyPairV1(t, executionPolicyTurnV1(t, executionPolicyReadPrimaryV1(t, config, threadID), turnID), "on-request", "workspace-write")
	receiptAfter, err := os.ReadFile(receiptPath)
	if err != nil || !bytes.Equal(receiptBefore, receiptAfter) || providerCalls.Load() != 1 {
		t.Fatal("thread metadata patch replaced or resumed the actual captured pending call")
	}
	if _, err := os.Stat(filepath.Join(workspace, "policy-denied.txt")); !os.IsNotExist(err) {
		t.Fatal("pending write acquired a file effect before approval")
	}
	executionPolicyHTTPV1(t, handler, http.MethodPost, "/v1/approvals/"+approvalID, map[string]any{"decision": "deny"}, http.StatusOK)
	if _, err := os.Stat(filepath.Join(workspace, "policy-denied.txt")); !os.IsNotExist(err) {
		t.Fatal("denied original pending call wrote its target")
	}
	final := executionPolicyHTTPV1(t, handler, http.MethodGet, path, nil, http.StatusOK)
	assertExecutionPolicyPairV1(t, final, "auto", "danger-full-access")
	assertExecutionPolicyPairV1(t, executionPolicyTurnV1(t, executionPolicyReadPrimaryV1(t, config, threadID), turnID), "on-request", "workspace-write")
}

func executionPolicyHTTPV1(t *testing.T, handler http.Handler, method, path string, payload map[string]any, want int) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != want {
		t.Fatalf("synthetic execution-policy HTTP %s %s status=%d want=%d code=%q message=%q reasonCode=%q", method, path, rec.Code, want,
			contracts.StringField(result, "code"), contracts.StringField(result, "message"), contracts.StringField(result, "reasonCode"))
	}
	return result
}

func assertExecutionPolicyPairV1(t *testing.T, record map[string]any, approval, sandbox string) {
	t.Helper()
	if record["approvalPolicy"] != approval || record["sandboxMode"] != sandbox {
		t.Fatalf("execution-policy pair changed at the observed runtime seam: approvalPolicy=%q sandboxMode=%q wantApprovalPolicy=%q wantSandboxMode=%q",
			contracts.StringField(record, "approvalPolicy"), contracts.StringField(record, "sandboxMode"), approval, sandbox)
	}
}

func executionPolicyPrimaryPathV1(config Config, threadID string) string {
	return filepath.Join(config.ProductionDurableRoot, "threads", threadID, "thread.json")
}

func executionPolicyReadPrimaryV1(t *testing.T, config Config, threadID string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(executionPolicyPrimaryPathV1(config, threadID))
	if err != nil {
		t.Fatal(err)
	}
	var thread map[string]any
	if err := json.Unmarshal(body, &thread); err != nil {
		t.Fatal(err)
	}
	if contracts.StringField(thread, "id") != threadID {
		t.Fatal("actual durable thread identity differs from the primary path")
	}
	return thread
}

func executionPolicyTurnV1(t *testing.T, thread map[string]any, turnID string) map[string]any {
	t.Helper()
	if turnID == "" {
		t.Fatal("actual HTTP start returned no turn identity")
	}
	turns, _ := thread["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if turn["id"] == turnID {
			return turn
		}
	}
	t.Fatal("actual started turn is absent from the current thread")
	return nil
}
