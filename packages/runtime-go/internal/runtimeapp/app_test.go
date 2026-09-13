package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	threadriskpolicystore "analytix.local/runtime-go/internal/adapters/outbound/threadriskpolicy"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestNewRuntimeServerHandlerAssemblesRunnableHandler(t *testing.T) {
	handler := NewRuntimeServerHandler(Config{
		Insecure:       true,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	if handler == nil {
		t.Fatal("handler is nil")
	}
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeAppRequiresTokenUnlessInsecureIsExplicit(t *testing.T) {
	if _, err := NewRuntimeServerHandlerE(Config{
		DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}); err == nil {
		t.Fatal("production composition accepted a static implicit runtime token")
	}
}

func TestRuntimeHostPolicyProductionCallerIsReachedThroughHTTPCaseTurn(t *testing.T) {
	workspace := workspacetest.New(t)
	metadata := filepath.Join(workspace, ".analytix")
	if err := os.Mkdir(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	binding, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_http_host_policy_001",
		"source": "analytix-data-analysis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "case-project.json"), binding, 0o644); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "source-unavailable case turn must not reach the provider"}})
	}))
	defer provider.Close()
	handler, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: t.TempDir(), DataDir: dataDir,
		ProviderID: "host-policy-http", BaseURL: provider.URL + "/v1", APIKey: "test-only",
		Model: "host-policy-http-model", EndpointFormat: "chat_completions",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	serverClosed := false
	defer func() {
		if !serverClosed {
			server.Close()
		}
	}()
	shutdown := func() {
		server.Close()
		serverClosed = true
		lifecycle, ok := handler.(interface{ Shutdown(context.Context) error })
		if !ok {
			t.Fatal("runtime handler has no shutdown lifecycle")
		}
		if err := lifecycle.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	requestJSON := func(method, path string, body map[string]any, want int) map[string]any {
		t.Helper()
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		request, requestErr := http.NewRequest(method, server.URL+path, bytes.NewReader(encoded))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer response.Body.Close()
		decoded := map[string]any{}
		if decodeErr := json.NewDecoder(response.Body).Decode(&decoded); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s status=%d body=%#v", method, path, response.StatusCode, decoded)
		}
		return decoded
	}
	thread := requestJSON(http.MethodPost, "/v1/threads", map[string]any{
		"title": "host policy HTTP seam", "workspace": workspace,
		"providerId": "host-policy-http", "model": "host-policy-http-model",
	}, http.StatusCreated)
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		t.Fatalf("thread response has no id: %#v", thread)
	}
	turn := requestJSON(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
		"prompt": "核验当前案件资料", "riskIntent": "case",
	}, http.StatusAccepted)
	turnID, _ := turn["turnId"].(string)
	if turnID == "" {
		t.Fatalf("case turn response has no id: %#v", turn)
	}
	if got := providerCalls.Load(); got != 0 {
		t.Fatalf("source-unavailable case turn reached the provider: calls=%d", got)
	}
	detail := requestJSON(http.MethodGet, "/v1/threads/"+threadID, map[string]any{}, http.StatusOK)
	persistedTurn := packagedSourceUnavailableHydrationTurnV1(detail, turnID)
	if persistedTurn == nil {
		t.Fatalf("source-unavailable case turn is missing from thread detail: %#v", detail)
	}
	if _, privateRecordPresent := persistedTurn["acceptedFinal"]; privateRecordPresent {
		t.Fatalf("case turn exposed the private accepted-final record in generic thread detail: %#v", persistedTurn)
	}
	if _, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(persistedTurn["acceptedFinalView"]); err != nil {
		t.Fatalf("case turn did not publish a strict V3 public accepted-final view: %v", err)
	}

	health, healthErr := http.Get(server.URL + "/health")
	if healthErr != nil {
		t.Fatal(healthErr)
	}
	health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("case boundary disabled ordinary runtime health: %d", health.StatusCode)
	}
	shutdown()

	policyRoot := filepath.Join(dataDir, "private", "thread-risk-policy")
	access, err := privatecastest.NewAccessAuthority(policyRoot)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := threadriskpolicystore.NewStore(policyRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	if exists, readErr := policies.HasRecords(context.Background()); readErr != nil || !exists {
		t.Fatalf("HTTP case turn did not reach the production HostPolicy caller: exists=%v err=%v", exists, readErr)
	}
}
