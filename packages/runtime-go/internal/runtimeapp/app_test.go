package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestRuntimeUserInputConfigurationReachesProductionAssembly(t *testing.T) {
	// Keep the production diagnostic probe, but make every binary unavailable.
	// This test observes the real tool catalog without launching subprocesses.
	t.Setenv("PATH", t.TempDir())
	catalog := func(document string) (int, string) {
		t.Helper()
		handler, err := NewRuntimeServerHandlerE(Config{
			RuntimeToken:   DefaultRuntimeToken,
			DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
			MCPConfigJSON: document,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer shutdownOwnedRuntimeHandler(t, handler)
		request := httptest.NewRequest(http.MethodGet, "/v1/runtime/tools", nil)
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("tool catalog status = %d", response.Code)
		}
		var tools struct {
			SchemaVersion int `json:"schemaVersion"`
			ToolContracts struct {
				Count       int    `json:"count"`
				CatalogHash string `json:"catalogHash"`
			} `json:"toolContracts"`
			Commands []struct {
				Found bool `json:"found"`
			} `json:"commands"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &tools); err != nil {
			t.Fatal(err)
		}
		if tools.SchemaVersion != 2 || tools.ToolContracts.Count == 0 || len(tools.ToolContracts.CatalogHash) != 64 || len(tools.Commands) == 0 {
			t.Fatal("production tool catalog is unavailable")
		}
		for _, command := range tools.Commands {
			if command.Found {
				t.Fatal("isolated diagnostic unexpectedly found an executable")
			}
		}
		return tools.ToolContracts.Count, tools.ToolContracts.CatalogHash
	}
	defaultCount, defaultHash := catalog("")
	legacyCount, legacyHash := catalog(`{"runtime":{"userInput":{"policy":"legacy","defaultTimeoutSeconds":7}}}`)
	timedCount, timedHash := catalog(`{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":7}}}`)
	if defaultCount != legacyCount || defaultHash != legacyHash {
		t.Fatal("explicit legacy timeout changed the ordinary Question catalog")
	}
	if timedCount != legacyCount || timedHash == legacyHash {
		t.Fatal("timed policy did not reach the production tool catalog")
	}
}

func TestProductionMuxMaintenanceFencesPrivateLocalDisplayRoutes(t *testing.T) {
	handler, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOwnedRuntimeHandler(t, handler)
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/quiescence", bytes.NewBufferString(`{"operation":"prepare"}`))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("prepare status = %d", response.Code)
	}
	var ready struct {
		Lease string `json:"lease"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &ready); err != nil || len(ready.Lease) != 43 {
		t.Fatalf("prepare lease unavailable: %v", err)
	}
	private := httptest.NewRequest(http.MethodPost, "/v1/local-display/funds-import/confirm", bytes.NewBufferString(`{}`))
	private.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	private.Header.Set("X-Analytix-Local-Display", "typed-v1")
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, private)
	if blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("private route bypassed production maintenance fence: %d", blocked.Code)
	}
	release := httptest.NewRequest(http.MethodPost, "/v1/runtime/quiescence",
		bytes.NewBufferString(`{"operation":"release","lease":"`+ready.Lease+`"}`))
	release.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	released := httptest.NewRecorder()
	handler.ServeHTTP(released, release)
	if released.Code != http.StatusOK {
		t.Fatalf("release status = %d", released.Code)
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

func TestRuntimeUserInputConfigurationRejectedByProductionAssembly(t *testing.T) {
	for _, document := range []string{
		`{"runtime":{"userInput":null}}`,
		`{"runtime":{"userInput":{"defaultTimeoutSeconds":7}}}`,
		`{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":0}}}`,
		`{"runtime":{"userInput":{"policy":"legacy","unknown":true}}}`,
	} {
		t.Run(document, func(t *testing.T) {
			handler, err := NewRuntimeServerHandlerE(Config{
				RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(), MCPConfigJSON: document,
			})
			if handler != nil {
				shutdownOwnedRuntimeHandler(t, handler)
			}
			if err == nil || !strings.HasPrefix(err.Error(), "runtime.userInput") {
				t.Fatalf("production composition silently dropped an invalid leaf: %v", err)
			}
		})
	}
}

func TestProductionUsageQueryIncludesTrustedEmptyConversationProjection(t *testing.T) {
	// Observe the real producer composition without binaries or Provider calls.
	t.Setenv("PATH", t.TempDir())
	h, err := NewRuntimeServerHandlerE(Config{Insecure: true, DurableTempDir: t.TempDir(), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle, ok := h.(interface{ Shutdown(context.Context) error }); ok {
		defer func() {
			if err := lifecycle.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	create := httptest.NewRecorder()
	h.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/v1/threads", strings.NewReader(`{"title":"synthetic usage assembly"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("thread create status=%d body=%s", create.Code, create.Body.String())
	}
	var thread map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &thread); err != nil {
		t.Fatal(err)
	}
	id, _ := thread["id"].(string)
	if id == "" {
		t.Fatal("missing synthetic thread id")
	}
	query := httptest.NewRecorder()
	h.ServeHTTP(query, httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=thread&thread_id="+id, nil))
	if query.Code != http.StatusOK {
		t.Fatalf("usage query status=%d body=%s", query.Code, query.Body.String())
	}
	var response struct {
		Buckets []struct {
			ConversationUsage struct {
				SchemaVersion string `json:"schemaVersion"`
				Billing       struct {
					InventoryComplete bool   `json:"inventoryComplete"`
					AttemptCount      uint64 `json:"attemptCount"`
				}
			} `json:"conversationUsage"`
		}
	}
	if err := json.Unmarshal(query.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Buckets) != 1 || response.Buckets[0].ConversationUsage.SchemaVersion != "conversation-usage.v1" || !response.Buckets[0].ConversationUsage.Billing.InventoryComplete || response.Buckets[0].ConversationUsage.Billing.AttemptCount != 0 {
		t.Fatalf("production Usage observer was not consumed: %s", query.Body.String())
	}
}
