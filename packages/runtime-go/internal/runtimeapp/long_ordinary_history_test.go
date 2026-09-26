//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	"analytix.local/runtime-go/internal/server"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Exercises the production Registry, terminal CAS, public GET, and the
// per-record projection boundary beyond the former 26-turn failure point.
// Reopened handlers must also continue once with the existing credential and
// preserve that new turn through another recovery. This is not an OS-process
// or installed Electron/Keychain acceptance test.
func TestLongOrdinaryHistoryRunsThroughProtectedRuntime(t *testing.T) {
	var providerRequests atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerRequests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"LOCAL QA OK\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3,\"total_tokens\":11}}\n\ndata: [DONE]\n\n"))
	}))
	defer provider.Close()
	_, config := runtimeWitnessedRegistryConfigV2(t)
	config.ProductionDurableRoot = runtimeWitnessedRegistryHostTempV2(t, "long-history")
	config.BaseURL = provider.URL + "/v1"
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	observations := make(chan server.AsyncTurnObservationV1, 256)
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
	request := func(method, path string, body map[string]any) (int, map[string]any) {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var result map[string]any
		if json.Unmarshal(rec.Body.Bytes(), &result) != nil {
			t.Fatal("invalid_public_response")
		}
		return rec.Code, result
	}
	status, created := request(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspacetest.New(t), "providerId": config.ProviderID, "model": config.Model})
	if status != http.StatusCreated {
		t.Fatalf("thread_create_status=%d code=%s", status, contracts.StringField(created, "code"))
	}
	threadID := contracts.StringField(created, "id")
	turnIDs := make([]string, 0, 31)
	assertHistory := func(detail map[string]any) {
		t.Helper()
		turns, ok := detail["turns"].([]any)
		if contracts.StringField(detail, "id") != threadID || !ok || len(turns) != len(turnIDs) {
			t.Fatalf("history_identity_or_count_invalid count=%d expected=%d", len(turns), len(turnIDs))
		}
		seen := make(map[string]bool, len(turns))
		for index, raw := range turns {
			turn, ok := raw.(map[string]any)
			id := contracts.StringField(turn, "id")
			if !ok || id == "" || seen[id] || id != turnIDs[index] || contracts.StringField(turn, "status") != "completed" {
				t.Fatalf("history_order_identity_or_status_invalid index=%d", index)
			}
			seen[id] = true
		}
	}
	completeTurn := func(number int) {
		t.Helper()
		prompt := fmt.Sprintf("PR28 isolated long history fixture %d", number)
		status, started := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": prompt, "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "never", "sandboxMode": "read-only", "disableUserInput": true})
		if status != http.StatusAccepted {
			readStatus, readback := request(http.MethodGet, "/v1/threads/"+threadID, nil)
			readTurns, _ := readback["turns"].([]any)
			t.Fatalf("turn=%d create_status=%d code=%s reason=%s read_status=%d read_code=%s read_turn_count=%d", number, status, contracts.StringField(started, "code"), contracts.StringField(started, "reasonCode"), readStatus, contracts.StringField(readback, "code"), len(readTurns))
		}
		turnID := contracts.StringField(started, "turnId")
		if turnID == "" {
			t.Fatalf("turn=%d missing_id", number)
		}
		turnIDs = append(turnIDs, turnID)
		var finish server.AsyncTurnObservationV1
		deadline := time.After(20 * time.Second)
		for finish.Stage != "finished" {
			select {
			case observation := <-observations:
				if observation.TurnID == turnID && observation.Stage == "finished" {
					finish = observation
				}
			case <-deadline:
				t.Fatalf("turn=%d observer_timeout", number)
			}
		}
		status, detail := request(http.MethodGet, "/v1/threads/"+threadID, nil)
		turns, _ := detail["turns"].([]any)
		if finish.CompletionErrorClass != "none" || status != http.StatusOK || len(turns) != number {
			t.Fatalf("turn=%d completion_class=%s detail_class=%s record_class=%s record_detail=%s read_status=%d read_code=%s turn_count=%d", number,
				finish.CompletionErrorClass, finish.CompletionDetailClass, finish.FailureRecordErrorClass, finish.FailureRecordDetailClass,
				status, contracts.StringField(detail, "code"), len(turns))
		}
		assertHistory(detail)
	}
	for number := 1; number <= 30; number++ {
		completeTurn(number)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	lease, err = AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	inner, err = newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	handler = &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
	status, recovered := request(http.MethodGet, "/v1/threads/"+threadID, nil)
	if status != http.StatusOK {
		t.Fatalf("restarted_history_status=%d code=%s", status, contracts.StringField(recovered, "code"))
	}
	assertHistory(recovered)

	// Do not reseed the Registry or replay a failed POST. The same protected
	// authority must service one unique continuation after handler recovery.
	requestsBeforeContinuation := providerRequests.Load()
	completeTurn(31)
	if providerRequests.Load() != requestsBeforeContinuation+1 {
		t.Fatal("restarted_continuation_provider_request_count_invalid")
	}

	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	lease, err = AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	inner, err = newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
	if err != nil {
		_ = lease.Close()
		t.Fatal(err)
	}
	handler = &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
	status, recovered = request(http.MethodGet, "/v1/threads/"+threadID, nil)
	if status != http.StatusOK {
		t.Fatalf("continued_history_recovery_status=%d code=%s", status, contracts.StringField(recovered, "code"))
	}
	assertHistory(recovered)
}
