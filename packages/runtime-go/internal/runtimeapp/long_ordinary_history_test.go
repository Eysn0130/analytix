//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	"analytix.local/runtime-go/internal/server"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Exercises the production Registry, terminal CAS, public GET, and the
// per-record projection boundary beyond the former 26-turn failure point.
func TestLongOrdinaryHistoryRunsThroughProtectedRuntime(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	for number := 1; number <= 30; number++ {
		prompt := "PR28 isolated long history fixture"
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
		latest, _ := turns[len(turns)-1].(map[string]any)
		if contracts.StringField(latest, "status") != "completed" {
			t.Fatalf("turn=%d durable_status=%s", number, contracts.StringField(latest, "status"))
		}
	}
}
