//go:build !analytix_prod

package runtimego

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type g1ContractFixture struct {
	RuntimeToken             string             `json:"runtimeToken"`
	StartedAt                string             `json:"startedAt"`
	ProductBoundary          map[string]any     `json:"productBoundary"`
	Health                   g1ContractResponse `json:"health"`
	RuntimeInfoUnauthorized  g1ContractResponse `json:"runtimeInfoUnauthorized"`
	RuntimeInfo              g1ContractResponse `json:"runtimeInfo"`
	RuntimeToolsUnauthorized g1ContractResponse `json:"runtimeToolsUnauthorized"`
	RuntimeTools             g1ContractResponse `json:"runtimeTools"`
}

type g1ContractResponse struct {
	Status                int            `json:"status"`
	SchemaVersion         int            `json:"schemaVersion,omitempty"`
	BodyHash              string         `json:"bodyHash,omitempty"`
	ForbiddenTopLevelKeys []string       `json:"forbiddenTopLevelKeys,omitempty"`
	ForbiddenJSONTokens   []string       `json:"forbiddenJSONTokens,omitempty"`
	Body                  map[string]any `json:"body,omitempty"`
}

type g2ContractFixture struct {
	RuntimeToken    string              `json:"runtimeToken"`
	ProductBoundary map[string]any      `json:"productBoundary"`
	Routes          []G2RouteReplayCase `json:"routes"`
}

func TestG1ShadowRoutesMatchTypeScriptContract(t *testing.T) {
	contract := loadG1Contract(t)
	handler := NewG1ShadowHandler(G1ShadowConfig{
		RuntimeToken: contract.RuntimeToken,
		StartedAt:    contract.StartedAt,
	})

	assertRoute(t, handler, http.MethodGet, "/health", "", contract.Health)
	assertRoute(t, handler, http.MethodGet, "/v1/runtime/info", "", contract.RuntimeInfoUnauthorized)
	assertRoute(t, handler, http.MethodGet, "/v1/runtime/info", contract.RuntimeToken, contract.RuntimeInfo)
	assertRoute(t, handler, http.MethodGet, "/v1/runtime/tools", "", contract.RuntimeToolsUnauthorized)
	assertRoute(t, handler, http.MethodGet, "/v1/runtime/tools", contract.RuntimeToken, contract.RuntimeTools)
}

func TestG1ShadowProductBoundaryMatchesContract(t *testing.T) {
	contract := loadG1Contract(t)
	actual := decodeToMap(t, G1ShadowProductBoundary())
	if !reflect.DeepEqual(actual, contract.ProductBoundary) {
		t.Fatalf("product boundary mismatch\nactual: %#v\ncontract: %#v", actual, contract.ProductBoundary)
	}
}

func TestG1ShadowDoesNotExposeRendererVisibleGoRoute(t *testing.T) {
	handler := NewG1ShadowHandler(G1ShadowConfig{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/runtime/go", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected /v1/runtime/go to remain unexposed, got %d", recorder.Code)
	}
}

func TestG2ShadowRoutesReplayTypeScriptContract(t *testing.T) {
	contract := loadG2Contract(t)
	handler := NewG2ShadowHandler(G2ShadowConfig{
		RuntimeToken: contract.RuntimeToken,
		Routes:       contract.Routes,
	})

	for _, route := range contract.Routes {
		t.Run(route.ID, func(t *testing.T) {
			request := httptest.NewRequest(route.Method, route.Path, bytes.NewReader(route.Body))
			if route.Auth == "runtime-token" {
				request.Header.Set("Authorization", "Bearer "+contract.RuntimeToken)
			}
			if len(route.Body) > 0 {
				request.Header.Set("Content-Type", "application/json")
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != route.Response.Status {
				t.Fatalf("%s %s status mismatch: got %d want %d", route.Method, route.Path, recorder.Code, route.Response.Status)
			}
			if route.ResponseKind == "sse" {
				if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
					t.Fatalf("%s %s content-type mismatch: got %q", route.Method, route.Path, got)
				}
				if frames := splitSSEFrames(recorder.Body.String()); !reflect.DeepEqual(frames, route.SSEFrames) {
					t.Fatalf("%s %s frames mismatch\nactual: %#v\ncontract: %#v", route.Method, route.Path, frames, route.SSEFrames)
				}
				return
			}
			assertRawJSONEqual(t, recorder.Body.Bytes(), route.Response.Body, route.Method+" "+route.Path)
		})
	}
}

func TestG2ShadowProductBoundaryMatchesContract(t *testing.T) {
	contract := loadG2Contract(t)
	actual := decodeToMap(t, G2ShadowProductBoundary())
	if !reflect.DeepEqual(actual, contract.ProductBoundary) {
		t.Fatalf("product boundary mismatch\nactual: %#v\ncontract: %#v", actual, contract.ProductBoundary)
	}
}

func TestG2ShadowDoesNotExposeRendererVisibleGoRoute(t *testing.T) {
	contract := loadG2Contract(t)
	handler := NewG2ShadowHandler(G2ShadowConfig{
		RuntimeToken: contract.RuntimeToken,
		Routes:       contract.Routes,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/go", nil)
	request.Header.Set("Authorization", "Bearer "+contract.RuntimeToken)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected /v1/runtime/go to remain unexposed, got %d", recorder.Code)
	}
}

func TestRuntimeServerAllowsEmptyTokenInInsecureMode(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	handler := newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:          "",
		Insecure:              true,
		StartedAt:             g1.StartedAt,
		Routes:                g2.Routes,
		Host:                  "127.0.0.1",
		DataDir:               t.TempDir(),
		ProductionDurableRoot: t.TempDir(),
	})

	info := httptest.NewRecorder()
	handler.ServeHTTP(info, httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil))
	if info.Code != http.StatusOK {
		t.Fatalf("expected insecure runtime info without bearer auth, got %d: %s", info.Code, info.Body.String())
	}
	var infoBody map[string]any
	if err := json.Unmarshal(info.Body.Bytes(), &infoBody); err != nil {
		t.Fatalf("decode runtime info: %v", err)
	}
	if infoBody["insecure"] != true {
		t.Fatalf("expected runtime info to record insecure mode, got %#v", infoBody["insecure"])
	}

	threads := httptest.NewRecorder()
	handler.ServeHTTP(threads, httptest.NewRequest(http.MethodGet, "/v1/threads?limit=1", nil))
	if threads.Code != http.StatusOK {
		t.Fatalf("expected insecure thread list without bearer auth, got %d: %s", threads.Code, threads.Body.String())
	}
}

func TestLiveLocalSidecarIsolatedMutatingG2LifecycleMatchesTypeScriptContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	g3 := loadG3ProviderContract(t)
	g5 := loadG5FullLoopContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:     g1.RuntimeToken,
		StartedAt:        g1.StartedAt,
		Routes:           g2.Routes,
		ProviderContract: g3,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundary := LiveLocalSidecarProductBoundary()
	g5Boundary := g5.ControlExecutableCases.ProductBoundary.Expected
	if boundary.Mode != "live-local-isolated-g2-g3-g4-provider-cache-manager-prototype" ||
		!boundary.TestConformanceOnly ||
		boundary.ReadOnlyRouteReplayOnly ||
		!boundary.IsolatedMutableG2LifecyclePrototype ||
		!boundary.IsolatedInMemoryStoreOnly ||
		!boundary.FixtureBackedProviderG3Prototype ||
		!boundary.FixtureBackedProviderOnly ||
		!boundary.IsolatedG4ManagerPrototype ||
		!boundary.FixtureBackedG4ManagerOnly {
		t.Fatalf("live-local sidecar boundary should be test/conformance-only isolated mutable G2 plus fixture-backed G3/G4: %#v", boundary)
	}
	if boundary.ElectronMainConnected != g5Boundary.ElectronMainConnected ||
		boundary.DefaultGoBackendEnabled != g5Boundary.DefaultGoBackendEnabled ||
		boundary.RendererVisibleGoRoutesAllowed != g5Boundary.RendererVisibleGoRoutesAllowed {
		t.Fatalf("live-local sidecar rollback boundary drifted\nactual: %#v\nG5 contract: %#v", boundary, g5Boundary)
	}
	if boundary.ProviderLiveCallsAllowed || boundary.ExternalNetworkAllowed ||
		boundary.ProviderCredentialsAllowed || boundary.APIKeyReadAllowed ||
		boundary.ToolExecutionAllowed || boundary.ApprovalExecutionAllowed ||
		boundary.MCPConnectionAllowed || boundary.MCPCredentialsAllowed ||
		boundary.CredentialReadAllowed || boundary.FileMutationAllowed ||
		boundary.EventsJSONLMutationAllowed || boundary.RealWorkspaceMutationAllowed ||
		boundary.ReasonixPublicProtocolAllowed {
		t.Fatalf("live-local sidecar exposed forbidden execution surface: %#v", boundary)
	}

	assertLiveG1Route(t, server.URL, http.MethodGet, "/health", "", g1.Health)
	assertLiveG1Route(t, server.URL, http.MethodGet, "/v1/runtime/info", "", g1.RuntimeInfoUnauthorized)
	assertLiveG1Route(t, server.URL, http.MethodGet, "/v1/runtime/info", g1.RuntimeToken, g1.RuntimeInfo)
	assertLiveG1Route(t, server.URL, http.MethodGet, "/v1/runtime/tools", "", g1.RuntimeToolsUnauthorized)
	assertLiveG1Route(t, server.URL, http.MethodGet, "/v1/runtime/tools", g1.RuntimeToken, g1.RuntimeTools)

	mutatingRoutes := mutatingG2Routes(g2.Routes)
	if len(mutatingRoutes) != 4 {
		t.Fatalf("expected four mutating G2 live-local routes, got %d", len(mutatingRoutes))
	}
	for _, route := range g2.Routes {
		t.Run("live-local/"+route.ID, func(t *testing.T) {
			response, body := liveRequest(t, server.URL, route.Method, route.Path, tokenForRoute(route, g2.RuntimeToken), route.Body)
			if response.StatusCode != route.Response.Status {
				t.Fatalf("%s %s status mismatch: got %d want %d", route.Method, route.Path, response.StatusCode, route.Response.Status)
			}
			if route.ResponseKind == "sse" {
				if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
					t.Fatalf("%s %s content-type mismatch: got %q", route.Method, route.Path, got)
				}
				if frames := splitSSEFrames(string(body)); !reflect.DeepEqual(frames, route.SSEFrames) {
					t.Fatalf("%s %s frames mismatch\nactual: %#v\ncontract: %#v", route.Method, route.Path, frames, route.SSEFrames)
				}
				return
			}
			assertRawJSONEqual(t, body, route.Response.Body, route.Method+" "+route.Path)
		})
	}

	snapshot := harness.Snapshot()
	if !reflect.DeepEqual(snapshot.MutatedRouteIDs, []string{
		"thread-archive-patch",
		"thread-update-title-workspace",
		"thread-fork-side",
		"session-resume-thread",
	}) {
		t.Fatalf("live-local sidecar mutation sequence mismatch: %#v", snapshot.MutatedRouteIDs)
	}
	if !reflect.DeepEqual(snapshot.ArchivedThreadIDs, []string{"thr_g2_beta"}) {
		t.Fatalf("expected isolated archived thread state, got %#v", snapshot.ArchivedThreadIDs)
	}
	if !reflect.DeepEqual(snapshot.ForkThreadIDs, []string{"thr_1"}) {
		t.Fatalf("expected isolated fork thread state, got %#v", snapshot.ForkThreadIDs)
	}
	if !reflect.DeepEqual(snapshot.ResumedSessionIDs, []string{"thr_g2_source"}) {
		t.Fatalf("expected isolated resumed session state, got %#v", snapshot.ResumedSessionIDs)
	}
	if snapshot.ProviderCallAttempts != 0 ||
		snapshot.ApprovalExecutionAttempts != 0 ||
		snapshot.ToolExecutionAttempts != 0 ||
		snapshot.MCPConnectionAttempts != 0 ||
		snapshot.MCPCredentialAttempts != 0 ||
		snapshot.CredentialReadAttempts != 0 ||
		snapshot.FileMutationAttempts != 0 ||
		snapshot.EventsJSONLWriteAttempts != 0 ||
		snapshot.RealWorkspaceWriteAttempts != 0 {
		t.Fatalf("live-local sidecar attempted forbidden external mutation: %#v", snapshot)
	}

	assertLiveStatus(t, server.URL, http.MethodPost, "/v1/approvals/approval_live_local", g2.RuntimeToken, json.RawMessage(`{"decision":"allow"}`), http.StatusNotFound)
	for _, forbidden := range []string{
		"/v1/runtime/go",
		"/v1/reasonix",
		"/session-api",
		"/v1/workflow",
		"/v1/workflows",
		"/v1/create-loop",
		"/v1/subagents",
		"/v1/autoresearch",
		"/v1/mcp-indexer",
	} {
		assertLiveStatus(t, server.URL, http.MethodGet, forbidden, g2.RuntimeToken, nil, http.StatusNotFound)
	}
}

func TestLiveLocalSidecarMutationsDoNotTouchFilesystem(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	workspace := t.TempDir()
	eventsPath := filepath.Join(workspace, "events.jsonl")
	sentinel := []byte("sentinel-event-log\n")
	if err := os.WriteFile(eventsPath, sentinel, 0o600); err != nil {
		t.Fatalf("write sentinel events log: %v", err)
	}
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken: g1.RuntimeToken,
		StartedAt:    g1.StartedAt,
		Routes:       g2.Routes,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	for _, route := range mutatingG2Routes(g2.Routes) {
		response, body := liveRequest(t, server.URL, route.Method, route.Path, tokenForRoute(route, g2.RuntimeToken), route.Body)
		if response.StatusCode != route.Response.Status {
			t.Fatalf("%s %s status mismatch: got %d want %d", route.Method, route.Path, response.StatusCode, route.Response.Status)
		}
		assertRawJSONEqual(t, body, route.Response.Body, route.Method+" "+route.Path)
	}

	after, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read sentinel events log: %v", err)
	}
	if !bytes.Equal(after, sentinel) {
		t.Fatalf("events.jsonl sentinel changed: got %q want %q", string(after), string(sentinel))
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatalf("read sentinel workspace: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "events.jsonl" {
		t.Fatalf("live-local sidecar touched sentinel workspace: %#v", entries)
	}
	assertNoRuntimeGoFilesystemAPIs(t)
}

func TestLiveLocalSidecarG3ProviderUsageAndCacheReplayMatchesTypeScriptContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	g3 := loadG3ProviderContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:     g1.RuntimeToken,
		StartedAt:        g1.StartedAt,
		Routes:           g2.Routes,
		ProviderContract: g3,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundaryResponse := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g3/provider/boundary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, boundaryResponse, "fixtureOnly", true)
	assertBoolField(t, boundaryResponse, "externalNetworkUsed", false)
	assertBoolField(t, boundaryResponse, "providerCredentialsUsed", false)
	assertBoolField(t, boundaryResponse, "apiKeyRead", false)
	assertBoolField(t, boundaryResponse, "liveProviderCall", false)

	for _, item := range g3.ProviderUsageMatrix {
		t.Run("g3-usage/"+item.ID, func(t *testing.T) {
			body := mustJSON(t, map[string]string{"caseId": item.ID})
			response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/g3/provider/usage", g1.RuntimeToken, body, http.StatusOK)
			assertBoolField(t, response, "fixtureOnly", true)
			assertBoolField(t, response, "externalNetworkUsed", false)
			assertBoolField(t, response, "apiKeyRead", false)
			assertBoolField(t, response, "matchesExpectedUsage", true)
			if response["baseUrl"] != item.BaseURL {
				t.Fatalf("usage replay baseUrl mismatch: got %#v want %q", response["baseUrl"], item.BaseURL)
			}
			assertMapMatchesUsage(t, mapField(t, response, "parsedUsage"), item.ExpectedUsage)
		})
	}

	accounting := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g3/provider/cache-accounting", g1.RuntimeToken, nil, http.StatusOK)
	assertRawMapEqual(t, accounting, decodeToMap(t, buildG3ProviderCacheAccounting(g3.ProviderUsageMatrix)), "G3 cache accounting replay")

	drift := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g3/provider/cache-drift", g1.RuntimeToken, nil, http.StatusOK)
	assertRawMapEqual(t, drift, decodeToMap(t, buildProviderDriftAttribution(g3.CacheDriftAttribution)), "G3 cache drift replay")

	diagnostics := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g3/provider/cache-diagnostics", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, diagnostics, "fixtureOnly", true)
	assertBoolField(t, diagnostics, "dynamicStateInStablePrefix", false)
	assertBoolField(t, diagnostics, "diagnosticsLeakForbiddenSubstring", false)
	assertBoolField(t, diagnostics, "externalNetworkUsed", false)
	assertBoolField(t, diagnostics, "providerCredentialsUsed", false)
	assertBoolField(t, diagnostics, "apiKeyRead", false)
	for _, forbidden := range g3.CacheDiagnostics.ForbiddenDiagnosticsSubstrings {
		if strings.Contains(mustJSONText(t, diagnostics), forbidden) {
			t.Fatalf("G3 cache diagnostics leaked forbidden substring %q", forbidden)
		}
	}

	snapshot := harness.Snapshot()
	if snapshot.ProviderCallAttempts != 0 {
		t.Fatalf("G3 live-local provider replay attempted real provider call: %#v", snapshot)
	}
	if snapshot.StubProviderUsageReplays != len(g3.ProviderUsageMatrix) ||
		snapshot.StubProviderCacheReplays != 3 {
		t.Fatalf("G3 live-local provider replay counters mismatch: %#v", snapshot)
	}
}

func TestLiveLocalSidecarG3ProviderRequestShapeReplayMatchesTypeScriptContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	g3 := loadG3ProviderContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:     g1.RuntimeToken,
		StartedAt:        g1.StartedAt,
		Routes:           g2.Routes,
		ProviderContract: g3,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	for _, item := range g3.RequestShapeMatrix {
		t.Run("g3-request-shape/"+item.ID, func(t *testing.T) {
			body := mustJSON(t, map[string]string{"caseId": item.ID})
			response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/g3/provider/request-shape", g1.RuntimeToken, body, http.StatusOK)
			assertBoolField(t, response, "fixtureOnly", true)
			assertBoolField(t, response, "externalNetworkUsed", false)
			assertBoolField(t, response, "apiKeyRead", false)
			assertBoolField(t, response, "urlMatches", true)
			assertBoolField(t, response, "headerShapeMatches", true)
			assertBoolField(t, response, "bodyShapeMatches", true)
			assertBoolField(t, response, "toolShapeMatches", true)
			if response["derivedUrl"] != item.ExpectedURL {
				t.Fatalf("derived URL mismatch: got %#v want %q", response["derivedUrl"], item.ExpectedURL)
			}
			if item.EndpointFormat == "custom_endpoint" {
				assertBoolField(t, response, "customFullEndpointExactUrl", true)
			}
		})
	}

	snapshot := harness.Snapshot()
	if snapshot.ProviderCallAttempts != 0 {
		t.Fatalf("G3 request-shape replay attempted real provider call: %#v", snapshot)
	}
	if snapshot.StubProviderShapeReplays != len(g3.RequestShapeMatrix) {
		t.Fatalf("G3 request-shape replay counter mismatch: %#v", snapshot)
	}
}

func TestLiveLocalSidecarG3ProviderStreamingReplayMatchesTypeScriptContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	g3 := loadG3ProviderContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:     g1.RuntimeToken,
		StartedAt:        g1.StartedAt,
		Routes:           g2.Routes,
		ProviderContract: g3,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	path := "/v1/conformance/g3/provider/stream?thread_id=" + g3.Streaming.ThreadID + "&since_seq=2"
	response, body := liveRequest(t, server.URL, http.MethodGet, path, g1.RuntimeToken, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("G3 provider stream status mismatch: got %d want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("G3 provider stream content-type mismatch: got %q", got)
	}
	frames := splitSSEFrames(string(body))
	if !reflect.DeepEqual(frames, g3.Streaming.SSEFrames) {
		t.Fatalf("G3 provider stream frames mismatch\nactual: %#v\ncontract: %#v", frames, g3.Streaming.SSEFrames)
	}
	if kinds := sseEventNames(frames); !reflect.DeepEqual(kinds, g3.Streaming.ExpectedKindsInOrder) {
		t.Fatalf("G3 provider stream event order mismatch\nactual: %#v\ncontract: %#v", kinds, g3.Streaming.ExpectedKindsInOrder)
	}
	expectedUsage := g3ProviderUsageCaseByID(g3.ProviderUsageMatrix, g3.Streaming.ExpectedUsageCaseID)
	if !usagePayloadMatchesExpected(usagePayloadFromSSEFrames(frames), expectedUsage.ExpectedUsage) {
		t.Fatalf("G3 provider stream usage event does not match expected usage case %q", g3.Streaming.ExpectedUsageCaseID)
	}

	snapshot := harness.Snapshot()
	if snapshot.ProviderCallAttempts != 0 || snapshot.StubProviderStreamReplays != 1 {
		t.Fatalf("G3 provider streaming replay counters mismatch: %#v", snapshot)
	}
}

func TestLiveLocalSidecarG4ApprovalUserInputReplayMatchesTypeScriptContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	g4 := loadG4ToolsContract(t)
	approvalUserInput := loadApprovalUserInputRouteContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:              g1.RuntimeToken,
		StartedAt:                 g1.StartedAt,
		Routes:                    g2.Routes,
		G4Contract:                g4,
		ApprovalUserInputContract: approvalUserInput,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundary := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/boundary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, boundary, "fixtureOnly", true)
	assertBoolField(t, boundary, "approvalExecutionAllowed", false)
	assertBoolField(t, boundary, "toolExecutionAllowed", false)
	assertBoolField(t, boundary, "mcpConnectionAllowed", false)
	assertBoolField(t, boundary, "credentialReadAllowed", false)
	assertBoolField(t, boundary, "fileMutationAllowed", false)

	catalog := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/tool-catalog", g1.RuntimeToken, nil, http.StatusOK)
	if !reflect.DeepEqual(stringSliceField(t, catalog, "toolNames"), g4.ToolCatalog.AdvertisedToolNames) {
		t.Fatalf("G4 tool catalog mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, catalog, "toolNames"), g4.ToolCatalog.AdvertisedToolNames)
	}
	if !reflect.DeepEqual(stringSliceField(t, catalog, "forbiddenTopLevelRoutes"), g4.ToolCatalog.ForbiddenTopLevelRoutes) {
		t.Fatalf("G4 forbidden top-level routes mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, catalog, "forbiddenTopLevelRoutes"), g4.ToolCatalog.ForbiddenTopLevelRoutes)
	}

	approvalResponse := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/g4/manager/approval/decision",
		g1.RuntimeToken,
		mustJSON(t, approvalUserInput.Approval.DecisionRequest),
		approvalUserInput.Approval.ExpectedResponse.Status,
	)
	assertRawMapEqual(t, approvalResponse, approvalUserInput.Approval.ExpectedResponse.Body, "G4 approval decision response")
	assertLiveStatus(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/g4/manager/approval/decision",
		g1.RuntimeToken,
		mustJSON(t, approvalUserInput.Approval.DecisionRequest),
		approvalUserInput.Approval.SecondDecisionStatus,
	)
	approvalReplay := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/approval/replay", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, approvalReplay, "deniedNoExecute", true)
	if jsonIntField(t, approvalReplay, "pendingBefore") != approvalUserInput.Approval.PendingBefore ||
		jsonIntField(t, approvalReplay, "pendingAfter") != approvalUserInput.Approval.PendingAfter {
		t.Fatalf("G4 approval pending counters mismatch: %#v", approvalReplay)
	}
	if !reflect.DeepEqual(stringSliceField(t, approvalReplay, "replayKindsInOrder"), approvalUserInput.Replay.ExpectedKindsInOrder) {
		t.Fatalf("G4 approval replay kinds mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, approvalReplay, "replayKindsInOrder"), approvalUserInput.Replay.ExpectedKindsInOrder)
	}

	cancelResponse := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/g4/manager/user-input/cancel",
		g1.RuntimeToken,
		mustJSON(t, approvalUserInput.UserInput.ResolveRequest),
		approvalUserInput.UserInput.ExpectedResponse.Status,
	)
	assertRawMapEqual(t, cancelResponse, approvalUserInput.UserInput.ExpectedResponse.Body, "G4 user-input cancel response")
	assertLiveStatus(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/g4/manager/user-input/cancel",
		g1.RuntimeToken,
		mustJSON(t, approvalUserInput.UserInput.ResolveRequest),
		approvalUserInput.UserInput.SecondResolveStatus,
	)
	submitResponse := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/g4/manager/user-input/submit",
		g1.RuntimeToken,
		mustJSON(t, approvalUserInput.SubmittedUserInput.ResolveRequest),
		approvalUserInput.SubmittedUserInput.ExpectedResponse.Status,
	)
	assertRawMapEqual(t, submitResponse, approvalUserInput.SubmittedUserInput.ExpectedResponse.Body, "G4 user-input submit response")

	for _, invalidCase := range g4.UserInput.StructuredChoiceValidation.InvalidCases {
		body := mustJSON(t, map[string]string{"caseId": invalidCase})
		validation := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/g4/manager/user-input/validate", g1.RuntimeToken, body, http.StatusBadRequest)
		if validation["code"] != g4.UserInput.StructuredChoiceValidation.InvalidResultCode {
			t.Fatalf("G4 validation code mismatch: got %#v want %q", validation["code"], g4.UserInput.StructuredChoiceValidation.InvalidResultCode)
		}
		assertBoolField(t, validation, "opensGate", false)
	}

	userInputReplay := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/user-input/replay", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, userInputReplay, "submittedAnswersEchoed", true)
	assertBoolField(t, userInputReplay, "resolvedEventIncludesAnswers", false)
	assertBoolField(t, userInputReplay, "resolvedEventOmitsAnswers", true)
	assertBoolField(t, userInputReplay, "remoteDisableUserInputPreserved", true)
	replayText := mustJSONText(t, userInputReplay)
	for _, answer := range approvalUserInput.SubmittedUserInput.ExpectedResolution.Answers {
		if strings.Contains(replayText, answer.Value) {
			t.Fatalf("G4 user-input replay leaked submitted answer value %q", answer.Value)
		}
	}

	remote := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/remote-entry", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, remote, "disableUserInputPreserved", true)
	assertBoolField(t, remote, "reasonixControlPlaneExposed", false)
	if !reflect.DeepEqual(stringSliceField(t, remote, "expectedPortKeys"), approvalUserInput.RemoteEntry.ExpectedPortKeys) {
		t.Fatalf("G4 remote expected ports mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, remote, "expectedPortKeys"), approvalUserInput.RemoteEntry.ExpectedPortKeys)
	}
	if !reflect.DeepEqual(stringSliceField(t, remote, "forbiddenPortKeys"), approvalUserInput.RemoteEntry.ForbiddenPortKeys) {
		t.Fatalf("G4 remote forbidden ports mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, remote, "forbiddenPortKeys"), approvalUserInput.RemoteEntry.ForbiddenPortKeys)
	}

	snapshot := harness.Snapshot()
	if snapshot.ApprovalExecutionAttempts != 0 ||
		snapshot.ToolExecutionAttempts != 0 ||
		snapshot.MCPConnectionAttempts != 0 ||
		snapshot.CredentialReadAttempts != 0 ||
		snapshot.FileMutationAttempts != 0 ||
		snapshot.EventsJSONLWriteAttempts != 0 ||
		snapshot.RealWorkspaceWriteAttempts != 0 {
		t.Fatalf("G4 approval/user-input replay attempted forbidden work: %#v", snapshot)
	}
	if snapshot.StubG4ApprovalReplays == 0 ||
		snapshot.StubG4UserInputReplays == 0 ||
		snapshot.StubG4ValidationReplays == 0 {
		t.Fatalf("G4 approval/user-input replay counters did not advance: %#v", snapshot)
	}
}

func TestLiveLocalSidecarG4MCPReplayMatchesTypeScriptContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	g4 := loadG4ToolsContract(t)
	mcp := loadMCPToolLifecycleContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:             g1.RuntimeToken,
		StartedAt:                g1.StartedAt,
		Routes:                   g2.Routes,
		G4Contract:               g4,
		MCPToolLifecycleContract: mcp,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	lifecycle := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/mcp/lifecycle", g1.RuntimeToken, nil, http.StatusOK)
	if lifecycle["providerId"] != mcp.ProviderID {
		t.Fatalf("G4 MCP provider mismatch: got %#v want %q", lifecycle["providerId"], mcp.ProviderID)
	}
	if !reflect.DeepEqual(stringSliceField(t, lifecycle, "connectToolNames"), mcp.Connect.ToolNames) {
		t.Fatalf("G4 MCP connect tools mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, lifecycle, "connectToolNames"), mcp.Connect.ToolNames)
	}
	if !reflect.DeepEqual(stringSliceField(t, lifecycle, "reloadToolNames"), mcp.Reload.ToolNames) {
		t.Fatalf("G4 MCP reload tools mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, lifecycle, "reloadToolNames"), mcp.Reload.ToolNames)
	}
	assertBoolField(t, lifecycle, "cancelExecuted", false)
	assertBoolField(t, lifecycle, "mcpConnectionUsed", false)
	assertBoolField(t, lifecycle, "credentialRead", false)

	annotations := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/mcp/approval-annotations", g1.RuntimeToken, nil, http.StatusOK)
	if annotations["normalizedToolName"] != mcp.ApprovalAnnotations.NormalizedToolName {
		t.Fatalf("G4 MCP approval annotation tool mismatch: got %#v want %q", annotations["normalizedToolName"], mcp.ApprovalAnnotations.NormalizedToolName)
	}
	assertBoolField(t, annotations, "executed", false)
	assertBoolField(t, annotations, "deniedNoExecute", true)

	search := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/mcp/search", g1.RuntimeToken, nil, http.StatusOK)
	if !reflect.DeepEqual(stringSliceField(t, search, "toolNames"), mcp.SearchMetaTools.ToolNames) {
		t.Fatalf("G4 MCP search tool names mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, search, "toolNames"), mcp.SearchMetaTools.ToolNames)
	}
	assertBoolField(t, search, "refreshToolAdvertised", true)
	assertBoolField(t, search, "deniedNoExecute", true)
	if jsonIntField(t, search, "untrustedSearchedTools") != mcp.SearchMetaTools.UntrustedSearchedTools {
		t.Fatalf("G4 MCP untrusted search count mismatch: %#v", search)
	}
	untrustedPath := "/v1/conformance/g4/manager/mcp/search?workspace=" + mcp.SearchMetaTools.UntrustedWorkspace
	untrustedSearch := assertLiveJSON(t, server.URL, http.MethodGet, untrustedPath, g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, untrustedSearch, "untrustedWorkspaceSelected", true)
	if jsonIntField(t, untrustedSearch, "untrustedSearchedTools") != 0 {
		t.Fatalf("G4 MCP untrusted workspace searched tools: %#v", untrustedSearch)
	}
	refreshDrift := mapField(t, search, "refreshDrift")
	if jsonIntField(t, refreshDrift, "totalIndexed") != mcp.SearchMetaTools.RefreshDrift.ExpectedTotalIndexed {
		t.Fatalf("G4 MCP refresh drift total mismatch: %#v", refreshDrift)
	}
	assertBoolField(t, refreshDrift, "catalogDrift", true)

	callReconnect := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/mcp/call-reconnect", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, callReconnect, "transportErrorRetried", true)
	assertBoolField(t, callReconnect, "protocolErrorRetried", false)
	assertBoolField(t, callReconnect, "staleCallSucceeded", true)
	assertBoolField(t, callReconnect, "protocolCallReturnedError", true)
	assertBoolField(t, callReconnect, "mcpConnectionUsed", false)
	if jsonIntField(t, callReconnect, "staleFactoryAttempts") != mcp.CallReconnect.StaleConnection.FactoryAttempts ||
		jsonIntField(t, callReconnect, "protocolFactoryAttempts") != mcp.CallReconnect.ProtocolFailure.FactoryAttempts {
		t.Fatalf("G4 MCP call-time reconnect attempts mismatch: %#v", callReconnect)
	}

	background := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/mcp/background-reconnect", g1.RuntimeToken, nil, http.StatusOK)
	if !reflect.DeepEqual(stringSliceField(t, background, "failedServerIds"), mcp.BackgroundReconnect.FailedServerIDs) {
		t.Fatalf("G4 MCP background failed servers mismatch\nactual: %#v\ncontract: %#v", stringSliceField(t, background, "failedServerIds"), mcp.BackgroundReconnect.FailedServerIDs)
	}
	assertBoolField(t, background, "requiresRuntimeRestart", false)
	assertBoolField(t, background, "mcpConnectionUsed", false)

	diagnostics := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/g4/manager/mcp/diagnostics", g1.RuntimeToken, nil, http.StatusOK)
	if diagnostics["knownOverride"] != mcp.KnownOverride.Diagnostic.KnownOverride {
		t.Fatalf("G4 MCP known override mismatch: got %#v want %q", diagnostics["knownOverride"], mcp.KnownOverride.Diagnostic.KnownOverride)
	}
	assertBoolField(t, diagnostics, "lowPriority", true)
	assertBoolField(t, diagnostics, "backgroundStart", true)
	assertBoolField(t, diagnostics, "secretLeaked", false)
	assertBoolField(t, diagnostics, "credentialRead", false)
	if strings.Contains(mustJSONText(t, diagnostics), mcp.DiagnosticsRedaction.Secret) {
		t.Fatalf("G4 MCP diagnostics leaked secret %q", mcp.DiagnosticsRedaction.Secret)
	}

	snapshot := harness.Snapshot()
	if snapshot.ApprovalExecutionAttempts != 0 ||
		snapshot.ToolExecutionAttempts != 0 ||
		snapshot.MCPConnectionAttempts != 0 ||
		snapshot.MCPCredentialAttempts != 0 ||
		snapshot.CredentialReadAttempts != 0 ||
		snapshot.FileMutationAttempts != 0 ||
		snapshot.EventsJSONLWriteAttempts != 0 ||
		snapshot.RealWorkspaceWriteAttempts != 0 {
		t.Fatalf("G4 MCP replay attempted forbidden work: %#v", snapshot)
	}
	if snapshot.StubG4MCPReplays == 0 {
		t.Fatalf("G4 MCP replay counter did not advance: %#v", snapshot)
	}
}

func assertNoRuntimeGoFilesystemAPIs(t *testing.T) {
	t.Helper()
	for _, path := range []string{"internal/conformance/livelocal/harness.go", "internal/conformance/livelocal/store.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range []string{"os.", "os/", "path/filepath", "events.jsonl"} {
			if bytes.Contains(source, []byte(token)) {
				t.Fatalf("%s contains forbidden filesystem token %q", path, token)
			}
		}
	}
}

func TestG3ProviderConformanceOutputMatchesTypeScriptContract(t *testing.T) {
	contract := loadG3ProviderContract(t)
	actual := decodeToMap(t, BuildG3ProviderConformanceOutput(contract))
	if !reflect.DeepEqual(actual, contract.ExpectedOutput) {
		t.Fatalf("G3 provider output mismatch\nactual: %#v\ncontract: %#v", actual, contract.ExpectedOutput)
	}
}

func TestG4ToolsConformanceOutputMatchesTypeScriptContract(t *testing.T) {
	contract := loadG4ToolsContract(t)
	actual := decodeToMap(t, BuildG4ToolsConformanceOutput(contract))
	if !reflect.DeepEqual(actual, contract.ExpectedOutput) {
		t.Fatalf("G4 tools output mismatch\nactual: %#v\ncontract: %#v", actual, contract.ExpectedOutput)
	}
}

func TestG5FullLoopConformanceOutputMatchesTypeScriptContract(t *testing.T) {
	contract := loadG5FullLoopContract(t)
	actual := decodeToMap(t, BuildG5FullLoopConformanceOutput(contract))
	if !reflect.DeepEqual(actual, contract.ExpectedOutput) {
		t.Fatalf("G5 full-loop output mismatch\nactual: %#v\ncontract: %#v", actual, contract.ExpectedOutput)
	}
}

func TestG5ShadowSlicesOutputMatchesTypeScriptContract(t *testing.T) {
	contract := loadG5FullLoopContract(t)
	sources := G5ShadowSourceFixtures{
		TaskJobOrchestration: loadTaskJobOrchestrationContract(t),
		ProviderCache:        loadProviderCacheContract(t),
		ProviderStreaming:    loadG3ProviderContract(t),
		G2Routes:             loadG2Contract(t).Routes,
		ApprovalUserInput:    loadApprovalUserInputRouteContract(t),
		MCPLifecycle:         loadMCPToolLifecycleContract(t),
	}
	actual := decodeToMap(t, BuildG5ShadowSlicesOutput(contract, sources))
	if !reflect.DeepEqual(actual, contract.ShadowSlicesExpectedOutput) {
		t.Fatalf("G5 shadow slices output mismatch\nactual: %#v\ncontract: %#v", actual, contract.ShadowSlicesExpectedOutput)
	}
}

func TestG5ControlExecutableOutputMatchesTypeScriptContract(t *testing.T) {
	contract := loadG5FullLoopContract(t)
	actual := BuildG5ControlExecutableOutput(contract.ControlExecutableCases)
	if !reflect.DeepEqual(actual.ProductBoundary, contract.ControlExecutableCases.ProductBoundary.Expected) {
		t.Fatalf("G5 product boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.ProductBoundary, contract.ControlExecutableCases.ProductBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.PackageRuntimeIdentity, contract.ControlExecutableCases.PackageRuntimeIdentity.Expected) {
		t.Fatalf("G5 package runtime identity executable mismatch\nactual: %#v\ncontract: %#v", actual.PackageRuntimeIdentity, contract.ControlExecutableCases.PackageRuntimeIdentity.Expected)
	}
	if !reflect.DeepEqual(actual.RuntimeHTTPRouteSovereignty, contract.ControlExecutableCases.RuntimeHTTPRouteSovereignty.Expected) {
		t.Fatalf("G5 runtime HTTP route sovereignty executable mismatch\nactual: %#v\ncontract: %#v", actual.RuntimeHTTPRouteSovereignty, contract.ControlExecutableCases.RuntimeHTTPRouteSovereignty.Expected)
	}
	if !reflect.DeepEqual(actual.DesktopSovereignty, contract.ControlExecutableCases.DesktopSovereignty.Expected) {
		t.Fatalf("G5 desktop sovereignty executable mismatch\nactual: %#v\ncontract: %#v", actual.DesktopSovereignty, contract.ControlExecutableCases.DesktopSovereignty.Expected)
	}
	if !reflect.DeepEqual(actual.DesktopMainIpcBoundary, contract.ControlExecutableCases.DesktopMainIpcBoundary.Expected) {
		t.Fatalf("G5 desktop main IPC boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.DesktopMainIpcBoundary, contract.ControlExecutableCases.DesktopMainIpcBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.RendererRouteSurface, contract.ControlExecutableCases.RendererRouteSurface.Expected) {
		t.Fatalf("G5 renderer route surface executable mismatch\nactual: %#v\ncontract: %#v", actual.RendererRouteSurface, contract.ControlExecutableCases.RendererRouteSurface.Expected)
	}
	if !reflect.DeepEqual(actual.GoalPersistenceOffLock, contract.ControlExecutableCases.GoalPersistenceOffLock.Expected) {
		t.Fatalf("G5 goal persistence off-lock executable mismatch\nactual: %#v\ncontract: %#v", actual.GoalPersistenceOffLock, contract.ControlExecutableCases.GoalPersistenceOffLock.Expected)
	}
	if !reflect.DeepEqual(actual.ToolResultFileImageBoundary, contract.ControlExecutableCases.ToolResultFileImageBoundary.Expected) {
		t.Fatalf("G5 tool result file/image boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.ToolResultFileImageBoundary, contract.ControlExecutableCases.ToolResultFileImageBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.EventJsonlReplayBoundary, contract.ControlExecutableCases.EventJsonlReplayBoundary.Expected) {
		t.Fatalf("G5 event JSONL replay boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.EventJsonlReplayBoundary, contract.ControlExecutableCases.EventJsonlReplayBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.MCPMalformedSchemaBoundary, contract.ControlExecutableCases.MCPMalformedSchemaBoundary.Expected) {
		t.Fatalf("G5 MCP malformed schema boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPMalformedSchemaBoundary, contract.ControlExecutableCases.MCPMalformedSchemaBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.Cancel.Results, contract.ControlExecutableCases.Cancel.ExpectedResults) {
		t.Fatalf("G5 cancel executable mismatch\nactual: %#v\ncontract: %#v", actual.Cancel.Results, contract.ControlExecutableCases.Cancel.ExpectedResults)
	}
	if actual.Cancel.ScheduledAfterCancel != contract.ControlExecutableCases.Cancel.ScheduledAfterCancel {
		t.Fatalf("G5 scheduledAfterCancel mismatch: got %d want %d", actual.Cancel.ScheduledAfterCancel, contract.ControlExecutableCases.Cancel.ScheduledAfterCancel)
	}
	if !reflect.DeepEqual(actual.TaskJobs.ToolContractBoundary, contract.ControlExecutableCases.TaskJobs.ToolContractBoundary.Expected) {
		t.Fatalf("G5 task job tool contract boundary mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.ToolContractBoundary, contract.ControlExecutableCases.TaskJobs.ToolContractBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.TranscriptIdentity, contract.ControlExecutableCases.TaskJobs.TranscriptIdentity.Expected) {
		t.Fatalf("G5 task job transcript identity mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.TranscriptIdentity, contract.ControlExecutableCases.TaskJobs.TranscriptIdentity.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.NestedSSEMetadata, contract.ControlExecutableCases.TaskJobs.NestedSSEMetadata.Expected) {
		t.Fatalf("G5 task job nested SSE metadata mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.NestedSSEMetadata, contract.ControlExecutableCases.TaskJobs.NestedSSEMetadata.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.Jobs, contract.ControlExecutableCases.TaskJobs.ExpectedJobs) {
		t.Fatalf("G5 task job executable mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.Jobs, contract.ControlExecutableCases.TaskJobs.ExpectedJobs)
	}
	if !reflect.DeepEqual(actual.TaskJobs.StaleReconciled, contract.ControlExecutableCases.TaskJobs.StaleReconcile.ExpectedJobs) {
		t.Fatalf("G5 task job stale reconcile mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.StaleReconciled, contract.ControlExecutableCases.TaskJobs.StaleReconcile.ExpectedJobs)
	}
	if !reflect.DeepEqual(actual.TaskJobs.RestartDrill, contract.ControlExecutableCases.TaskJobs.RestartDrill.Expected) {
		t.Fatalf("G5 task job restart drill mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.RestartDrill, contract.ControlExecutableCases.TaskJobs.RestartDrill.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.RouteExecutable, contract.ControlExecutableCases.TaskJobs.RouteExecutable.Expected) {
		t.Fatalf("G5 task job route executable mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.RouteExecutable, contract.ControlExecutableCases.TaskJobs.RouteExecutable.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.Lifecycle, contract.ControlExecutableCases.TaskJobs.Lifecycle.Expected) {
		t.Fatalf("G5 task job lifecycle mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.Lifecycle, contract.ControlExecutableCases.TaskJobs.Lifecycle.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.PlannerExecutor, contract.ControlExecutableCases.TaskJobs.PlannerExecutor.Expected) {
		t.Fatalf("G5 task job planner-executor mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.PlannerExecutor, contract.ControlExecutableCases.TaskJobs.PlannerExecutor.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.PlannerToolsetInventory, contract.ControlExecutableCases.TaskJobs.PlannerToolsetInventory.Expected) {
		t.Fatalf("G5 task job planner toolset inventory mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.PlannerToolsetInventory, contract.ControlExecutableCases.TaskJobs.PlannerToolsetInventory.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.ParentGoal, contract.ControlExecutableCases.TaskJobs.ParentGoalEvidence.Expected) {
		t.Fatalf("G5 task job parent goal evidence mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.ParentGoal, contract.ControlExecutableCases.TaskJobs.ParentGoalEvidence.Expected)
	}
	if !reflect.DeepEqual(actual.TaskJobs.ParallelValidation, contract.ControlExecutableCases.TaskJobs.ParallelValidation.Expected) {
		t.Fatalf("G5 task job parallel validation mismatch\nactual: %#v\ncontract: %#v", actual.TaskJobs.ParallelValidation, contract.ControlExecutableCases.TaskJobs.ParallelValidation.Expected)
	}
	if !reflect.DeepEqual(actual.Approval, contract.ControlExecutableCases.Approval.Expected) {
		t.Fatalf("G5 approval deny executable mismatch\nactual: %#v\ncontract: %#v", actual.Approval, contract.ControlExecutableCases.Approval.Expected)
	}
	if !reflect.DeepEqual(actual.UserInput, contract.ControlExecutableCases.UserInput.Expected) {
		t.Fatalf("G5 user-input executable mismatch\nactual: %#v\ncontract: %#v", actual.UserInput, contract.ControlExecutableCases.UserInput.Expected)
	}
	if !reflect.DeepEqual(actual.ApprovalUserInputRoute, contract.ControlExecutableCases.ApprovalUserInputRoute.Expected) {
		t.Fatalf("G5 approval/user-input route replay mismatch\nactual: %#v\ncontract: %#v", actual.ApprovalUserInputRoute, contract.ControlExecutableCases.ApprovalUserInputRoute.Expected)
	}
	if !reflect.DeepEqual(actual.ApprovalUserInputInventory, contract.ControlExecutableCases.ApprovalUserInputInventory.Expected) {
		t.Fatalf("G5 approval/user-input inventory mismatch\nactual: %#v\ncontract: %#v", actual.ApprovalUserInputInventory, contract.ControlExecutableCases.ApprovalUserInputInventory.Expected)
	}
	if !reflect.DeepEqual(actual.AbortCleanup, contract.ControlExecutableCases.AbortCleanup.Expected) {
		t.Fatalf("G5 abort cleanup executable mismatch\nactual: %#v\ncontract: %#v", actual.AbortCleanup, contract.ControlExecutableCases.AbortCleanup.Expected)
	}
	if !reflect.DeepEqual(actual.ResumeGates, contract.ControlExecutableCases.ResumeGates.Expected) {
		t.Fatalf("G5 resume pending gates executable mismatch\nactual: %#v\ncontract: %#v", actual.ResumeGates, contract.ControlExecutableCases.ResumeGates.Expected)
	}
	if !reflect.DeepEqual(actual.AutoResearch, contract.ControlExecutableCases.AutoResearch.Expected) {
		t.Fatalf("G5 AutoResearch executable mismatch\nactual: %#v\ncontract: %#v", actual.AutoResearch, contract.ControlExecutableCases.AutoResearch.Expected)
	}
	if !reflect.DeepEqual(actual.MCPLifecycle, contract.ControlExecutableCases.MCPLifecycle.Expected) {
		t.Fatalf("G5 MCP lifecycle executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPLifecycle, contract.ControlExecutableCases.MCPLifecycle.Expected)
	}
	if !reflect.DeepEqual(actual.MCPCoreLifecycle, contract.ControlExecutableCases.MCPCoreLifecycle.Expected) {
		t.Fatalf("G5 MCP core lifecycle executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPCoreLifecycle, contract.ControlExecutableCases.MCPCoreLifecycle.Expected)
	}
	if !reflect.DeepEqual(actual.MCPBackgroundReconnect, contract.ControlExecutableCases.MCPBackgroundReconnect.Expected) {
		t.Fatalf("G5 MCP background reconnect executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPBackgroundReconnect, contract.ControlExecutableCases.MCPBackgroundReconnect.Expected)
	}
	if !reflect.DeepEqual(actual.MCPCallReconnect, contract.ControlExecutableCases.MCPCallReconnect.Expected) {
		t.Fatalf("G5 MCP call reconnect executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPCallReconnect, contract.ControlExecutableCases.MCPCallReconnect.Expected)
	}
	if !reflect.DeepEqual(actual.MCPKnownOverrideDiagnostics, contract.ControlExecutableCases.MCPKnownOverrideDiagnostics.Expected) {
		t.Fatalf("G5 MCP known override diagnostics executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPKnownOverrideDiagnostics, contract.ControlExecutableCases.MCPKnownOverrideDiagnostics.Expected)
	}
	if !reflect.DeepEqual(actual.MCPLiveLocalIndexer, contract.ControlExecutableCases.MCPLiveLocalIndexer.Expected) {
		t.Fatalf("G5 MCP live-local indexer executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPLiveLocalIndexer, contract.ControlExecutableCases.MCPLiveLocalIndexer.Expected)
	}
	if !reflect.DeepEqual(actual.MCPApprovalAnnotations, contract.ControlExecutableCases.MCPApprovalAnnotations.Expected) {
		t.Fatalf("G5 MCP approval annotation executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPApprovalAnnotations, contract.ControlExecutableCases.MCPApprovalAnnotations.Expected)
	}
	if !reflect.DeepEqual(actual.MCPSearchMetaTools, contract.ControlExecutableCases.MCPSearchMetaTools.Expected) {
		t.Fatalf("G5 MCP search meta-tools executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPSearchMetaTools, contract.ControlExecutableCases.MCPSearchMetaTools.Expected)
	}
	if !reflect.DeepEqual(actual.MCPSearchRefreshDrift, contract.ControlExecutableCases.MCPSearchRefreshDrift.Expected) {
		t.Fatalf("G5 MCP search refresh drift executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPSearchRefreshDrift, contract.ControlExecutableCases.MCPSearchRefreshDrift.Expected)
	}
	if !reflect.DeepEqual(actual.MCPSearchWorkspace, contract.ControlExecutableCases.MCPSearchWorkspace.Expected) {
		t.Fatalf("G5 MCP search workspace boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.MCPSearchWorkspace, contract.ControlExecutableCases.MCPSearchWorkspace.Expected)
	}
	if !reflect.DeepEqual(actual.Checkpoint, contract.ControlExecutableCases.Checkpoint.Expected) {
		t.Fatalf("G5 checkpoint rewind executable mismatch\nactual: %#v\ncontract: %#v", actual.Checkpoint, contract.ControlExecutableCases.Checkpoint.Expected)
	}
	if !reflect.DeepEqual(actual.RemoteEntry, contract.ControlExecutableCases.RemoteEntry.Expected) {
		t.Fatalf("G5 remote-entry executable mismatch\nactual: %#v\ncontract: %#v", actual.RemoteEntry, contract.ControlExecutableCases.RemoteEntry.Expected)
	}
	if !reflect.DeepEqual(actual.HistoryRepair, contract.ControlExecutableCases.HistoryRepair.Expected) {
		t.Fatalf("G5 history repair executable mismatch\nactual: %#v\ncontract: %#v", actual.HistoryRepair, contract.ControlExecutableCases.HistoryRepair.Expected)
	}
	if !reflect.DeepEqual(actual.CompactionBoundary, contract.ControlExecutableCases.CompactionBoundary.Expected) {
		t.Fatalf("G5 compaction boundary executable mismatch\nactual: %#v\ncontract: %#v", actual.CompactionBoundary, contract.ControlExecutableCases.CompactionBoundary.Expected)
	}
	if !reflect.DeepEqual(actual.StepLimits, contract.ControlExecutableCases.StepLimits.Expected) {
		t.Fatalf("G5 step-limit executable mismatch\nactual: %#v\ncontract: %#v", actual.StepLimits, contract.ControlExecutableCases.StepLimits.Expected)
	}
	if !reflect.DeepEqual(actual.Planner.Step0Advertised, contract.ControlExecutableCases.Planner.ExpectedStep0Advertised) {
		t.Fatalf("G5 planner step0 advertised mismatch\nactual: %#v\ncontract: %#v", actual.Planner.Step0Advertised, contract.ControlExecutableCases.Planner.ExpectedStep0Advertised)
	}
	if !reflect.DeepEqual(actual.Planner.Step1Advertised, contract.ControlExecutableCases.Planner.ExpectedStep1Advertised) {
		t.Fatalf("G5 planner step1 advertised mismatch\nactual: %#v\ncontract: %#v", actual.Planner.Step1Advertised, contract.ControlExecutableCases.Planner.ExpectedStep1Advertised)
	}
	if !reflect.DeepEqual(actual.Planner.RejectedCall, contract.ControlExecutableCases.Planner.ExpectedRejectedCall) {
		t.Fatalf("G5 planner rejection mismatch\nactual: %#v\ncontract: %#v", actual.Planner.RejectedCall, contract.ControlExecutableCases.Planner.ExpectedRejectedCall)
	}
	if !reflect.DeepEqual(actual.Planner.RejectedCalls, contract.ControlExecutableCases.Planner.ExpectedRejectedCalls) {
		t.Fatalf("G5 planner rejection matrix mismatch\nactual: %#v\ncontract: %#v", actual.Planner.RejectedCalls, contract.ControlExecutableCases.Planner.ExpectedRejectedCalls)
	}
	if !reflect.DeepEqual(actual.Planner.Gate, contract.ControlExecutableCases.Planner.Expected) {
		t.Fatalf("G5 planner gate output mismatch\nactual: %#v\ncontract: %#v", actual.Planner.Gate, contract.ControlExecutableCases.Planner.Expected)
	}
	if !reflect.DeepEqual(actual.AutoRouter, contract.ControlExecutableCases.AutoRouter.Expected) {
		t.Fatalf("G5 auto-router classifier mismatch\nactual: %#v\ncontract: %#v", actual.AutoRouter, contract.ControlExecutableCases.AutoRouter.Expected)
	}
	if !reflect.DeepEqual(actual.Combined, contract.ControlExecutableCases.Combined.Expected) {
		t.Fatalf("G5 combined control executable mismatch\nactual: %#v\ncontract: %#v", actual.Combined, contract.ControlExecutableCases.Combined.Expected)
	}
	if !reflect.DeepEqual(actual.PlanStepCancelCache, contract.ControlExecutableCases.PlanStepCancelCache.Expected) {
		t.Fatalf("G5 plan step/cancel/cache executable mismatch\nactual: %#v\ncontract: %#v", actual.PlanStepCancelCache, contract.ControlExecutableCases.PlanStepCancelCache.Expected)
	}
	if !reflect.DeepEqual(actual.PlanCancelStateReset, contract.ControlExecutableCases.PlanCancelStateReset.Expected) {
		t.Fatalf("G5 plan cancel state reset executable mismatch\nactual: %#v\ncontract: %#v", actual.PlanCancelStateReset, contract.ControlExecutableCases.PlanCancelStateReset.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderCacheReleaseGuard, contract.ControlExecutableCases.ProviderCacheReleaseGuard.Expected) {
		t.Fatalf("G5 provider cache release guard mismatch\nactual: %#v\ncontract: %#v", actual.ProviderCacheReleaseGuard, contract.ControlExecutableCases.ProviderCacheReleaseGuard.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderUsageParser, contract.ControlExecutableCases.ProviderUsageParser.Expected) {
		t.Fatalf("G5 provider usage parser mismatch\nactual: %#v\ncontract: %#v", actual.ProviderUsageParser, contract.ControlExecutableCases.ProviderUsageParser.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderRequestShape, contract.ControlExecutableCases.ProviderRequestShape.Expected) {
		t.Fatalf("G5 provider request shape mismatch\nactual: %#v\ncontract: %#v", actual.ProviderRequestShape, contract.ControlExecutableCases.ProviderRequestShape.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderLiveLocalHTTP, contract.ControlExecutableCases.ProviderLiveLocalHTTP.Expected) {
		t.Fatalf("G5 provider live-local HTTP mismatch\nactual: %#v\ncontract: %#v", actual.ProviderLiveLocalHTTP, contract.ControlExecutableCases.ProviderLiveLocalHTTP.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderCacheCoverageFloor, contract.ControlExecutableCases.ProviderCacheCoverageFloor.Expected) {
		t.Fatalf("G5 provider cache coverage floor mismatch\nactual: %#v\ncontract: %#v", actual.ProviderCacheCoverageFloor, contract.ControlExecutableCases.ProviderCacheCoverageFloor.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderDriftAttribution, contract.ControlExecutableCases.ProviderDriftAttribution.Expected) {
		t.Fatalf("G5 provider drift attribution mismatch\nactual: %#v\ncontract: %#v", actual.ProviderDriftAttribution, contract.ControlExecutableCases.ProviderDriftAttribution.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderCacheAccounting, contract.ControlExecutableCases.ProviderCacheAccounting.Expected) {
		t.Fatalf("G5 provider cache accounting mismatch\nactual: %#v\ncontract: %#v", actual.ProviderCacheAccounting, contract.ControlExecutableCases.ProviderCacheAccounting.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderOfflineParitySeal, contract.ControlExecutableCases.ProviderOfflineParitySeal.Expected) {
		t.Fatalf("G5 provider offline parity seal mismatch\nactual: %#v\ncontract: %#v", actual.ProviderOfflineParitySeal, contract.ControlExecutableCases.ProviderOfflineParitySeal.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderCachePrivacy, contract.ControlExecutableCases.ProviderCachePrivacy.Expected) {
		t.Fatalf("G5 provider cache privacy mismatch\nactual: %#v\ncontract: %#v", actual.ProviderCachePrivacy, contract.ControlExecutableCases.ProviderCachePrivacy.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderCacheInventory, contract.ControlExecutableCases.ProviderCacheInventory.Expected) {
		t.Fatalf("G5 provider cache inventory mismatch\nactual: %#v\ncontract: %#v", actual.ProviderCacheInventory, contract.ControlExecutableCases.ProviderCacheInventory.Expected)
	}
	if !reflect.DeepEqual(actual.ProviderStreaming, contract.ControlExecutableCases.ProviderStreaming.Expected) {
		t.Fatalf("G5 provider streaming mismatch\nactual: %#v\ncontract: %#v", actual.ProviderStreaming, contract.ControlExecutableCases.ProviderStreaming.Expected)
	}
	if !reflect.DeepEqual(actual.SessionRouteStatus, contract.ControlExecutableCases.SessionRouteStatus.Expected) {
		t.Fatalf("G5 session route status mismatch\nactual: %#v\ncontract: %#v", actual.SessionRouteStatus, contract.ControlExecutableCases.SessionRouteStatus.Expected)
	}
	if !reflect.DeepEqual(actual.SessionRouteInventory, contract.ControlExecutableCases.SessionRouteInventory.Expected) {
		t.Fatalf("G5 session route inventory mismatch\nactual: %#v\ncontract: %#v", actual.SessionRouteInventory, contract.ControlExecutableCases.SessionRouteInventory.Expected)
	}
	if !reflect.DeepEqual(actual.SessionRouteReplay, contract.ControlExecutableCases.SessionRouteReplay.Expected) {
		t.Fatalf("G5 session route replay mismatch\nactual: %#v\ncontract: %#v", actual.SessionRouteReplay, contract.ControlExecutableCases.SessionRouteReplay.Expected)
	}
}

func assertRoute(t *testing.T, handler http.Handler, method string, path string, token string, contract g1ContractResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	handler.ServeHTTP(recorder, request)
	if recorder.Code != contract.Status {
		t.Fatalf("%s %s status mismatch: got %d want %d", method, path, recorder.Code, contract.Status)
	}
	assertG1ContractBody(t, recorder.Body.Bytes(), contract, method+" "+path)
}

func assertLiveG1Route(t *testing.T, serverURL, method, path, token string, contract g1ContractResponse) {
	t.Helper()
	response, body := liveRequest(t, serverURL, method, path, token, nil)
	if response.StatusCode != contract.Status {
		t.Fatalf("%s %s status mismatch: got %d want %d", method, path, response.StatusCode, contract.Status)
	}
	assertG1ContractBody(t, body, contract, method+" "+path)
}

func assertG1ContractBody(t *testing.T, raw []byte, contract g1ContractResponse, label string) {
	t.Helper()
	bodyBytes := bytes.TrimSpace(raw)
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("%s returned invalid JSON: %v", label, err)
	}
	if contract.BodyHash == "" {
		if !reflect.DeepEqual(body, contract.Body) {
			t.Fatalf("%s body mismatch\nactual: %#v\ncontract: %#v", label, body, contract.Body)
		}
		return
	}
	sum := sha256.Sum256(bodyBytes)
	if actual := fmt.Sprintf("%x", sum); actual != contract.BodyHash {
		t.Fatalf("%s body hash mismatch: got %s want %s\nbody: %s", label, actual, contract.BodyHash, bodyBytes)
	}
	if version, _ := body["schemaVersion"].(float64); int(version) != contract.SchemaVersion {
		t.Fatalf("%s schema version mismatch: got %#v want %d", label, body["schemaVersion"], contract.SchemaVersion)
	}
	for _, key := range contract.ForbiddenTopLevelKeys {
		if _, exists := body[key]; exists {
			t.Fatalf("%s exposed forbidden top-level key %q: %s", label, key, bodyBytes)
		}
	}
	for _, token := range contract.ForbiddenJSONTokens {
		if bytes.Contains(bodyBytes, []byte(token)) {
			t.Fatalf("%s exposed forbidden JSON token %q: %s", label, token, bodyBytes)
		}
	}
}

func assertStatus(t *testing.T, handler http.Handler, method string, path string, token string, status int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	handler.ServeHTTP(recorder, request)
	if recorder.Code != status {
		t.Fatalf("%s %s status mismatch: got %d want %d", method, path, recorder.Code, status)
	}
}

func assertLiveStatus(t *testing.T, serverURL string, method string, path string, token string, body json.RawMessage, status int) {
	t.Helper()
	response, _ := liveRequest(t, serverURL, method, path, token, body)
	if response.StatusCode != status {
		t.Fatalf("%s %s status mismatch: got %d want %d", method, path, response.StatusCode, status)
	}
}

func assertLiveJSONMap(t *testing.T, serverURL string, method string, path string, token string, body json.RawMessage, status int, contract map[string]any) {
	t.Helper()
	response, data := liveRequest(t, serverURL, method, path, token, body)
	if response.StatusCode != status {
		t.Fatalf("%s %s status mismatch: got %d want %d body=%s", method, path, response.StatusCode, status, data)
	}
	var actual map[string]any
	if err := json.Unmarshal(data, &actual); err != nil {
		t.Fatalf("%s %s returned invalid JSON: %v", method, path, err)
	}
	if !reflect.DeepEqual(actual, contract) {
		t.Fatalf("%s %s body mismatch\nactual: %#v\ncontract: %#v", method, path, actual, contract)
	}
}

func assertLiveJSON(t *testing.T, serverURL string, method string, path string, token string, body json.RawMessage, status int) map[string]any {
	t.Helper()
	response, data := liveRequest(t, serverURL, method, path, token, body)
	if response.StatusCode != status {
		t.Fatalf("%s %s status mismatch: got %d want %d body=%s", method, path, response.StatusCode, status, data)
	}
	var actual map[string]any
	if err := json.Unmarshal(data, &actual); err != nil {
		t.Fatalf("%s %s returned invalid JSON: %v", method, path, err)
	}
	return actual
}

func assertBoolField(t *testing.T, record map[string]any, field string, expected bool) {
	t.Helper()
	actual, ok := record[field].(bool)
	if !ok || actual != expected {
		t.Fatalf("field %q mismatch: got %#v want %v", field, record[field], expected)
	}
}

func mapField(t *testing.T, record map[string]any, field string) map[string]any {
	t.Helper()
	value, ok := record[field].(map[string]any)
	if !ok {
		t.Fatalf("field %q is not an object: %#v", field, record[field])
	}
	return value
}

func jsonIntField(t *testing.T, record map[string]any, field string) int {
	t.Helper()
	value, ok := record[field].(float64)
	if !ok {
		t.Fatalf("field %q is not a JSON number: %#v", field, record[field])
	}
	return int(value)
}

func stringSliceField(t *testing.T, record map[string]any, field string) []string {
	t.Helper()
	values, ok := record[field].([]any)
	if !ok {
		t.Fatalf("field %q is not an array: %#v", field, record[field])
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok {
			t.Fatalf("field %q contains non-string value: %#v", field, value)
		}
		out = append(out, item)
	}
	return out
}

func assertMapMatchesUsage(t *testing.T, record map[string]any, expected G3ProviderUsageSummary) {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal usage record: %v", err)
	}
	var actual G3ProviderUsageSummary
	if err := json.Unmarshal(data, &actual); err != nil {
		t.Fatalf("decode usage record: %v", err)
	}
	if !usageSummaryMatchesExpected(actual, expected) {
		t.Fatalf("usage mismatch\nactual: %#v\nexpected: %#v", actual, expected)
	}
}

func assertRawMapEqual(t *testing.T, actual map[string]any, expected map[string]any, label string) {
	t.Helper()
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("%s mismatch\nactual: %#v\nexpected: %#v", label, actual, expected)
	}
}

func liveRequest(t *testing.T, serverURL string, method string, path string, token string, body json.RawMessage) (*http.Response, []byte) {
	t.Helper()
	var reader *bytes.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	request, err := http.NewRequest(method, serverURL+path, reader)
	if err != nil {
		t.Fatalf("build live request: %v", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("run live request: %v", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read live response body: %v", err)
	}
	return response, data
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return json.RawMessage(data)
}

func mustJSONText(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON text: %v", err)
	}
	return string(data)
}

func tokenForRoute(route G2RouteReplayCase, token string) string {
	if route.Auth == "runtime-token" {
		return token
	}
	return ""
}

func loadG1Contract(t *testing.T) g1ContractFixture {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-g1-shadow-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read G1 contract fixture: %v", err)
	}
	var fixture g1ContractFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode G1 contract fixture: %v", err)
	}
	return fixture
}

func loadG2Contract(t *testing.T) g2ContractFixture {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-g2-route-replay-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read G2 contract fixture: %v", err)
	}
	var fixture g2ContractFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode G2 contract fixture: %v", err)
	}
	return fixture
}

func loadG3ProviderContract(t *testing.T) G3ProviderConformanceContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-g3-provider-streaming-usage-cache-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read G3 contract fixture: %v", err)
	}
	var fixture G3ProviderConformanceContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode G3 contract fixture: %v", err)
	}
	return fixture
}

func loadG4ToolsContract(t *testing.T) G4ToolsConformanceContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-g4-tools-approval-user-input-mcp-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read G4 contract fixture: %v", err)
	}
	var fixture G4ToolsConformanceContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode G4 contract fixture: %v", err)
	}
	return fixture
}

func loadG5FullLoopContract(t *testing.T) G5FullLoopConformanceContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-g5-full-loop-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read G5 contract fixture: %v", err)
	}
	var fixture G5FullLoopConformanceContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode G5 contract fixture: %v", err)
	}
	return fixture
}

func loadTaskJobOrchestrationContract(t *testing.T) TaskJobOrchestrationContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "task-job-orchestration-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read task-job contract fixture: %v", err)
	}
	var fixture TaskJobOrchestrationContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode task-job contract fixture: %v", err)
	}
	return fixture
}

func loadProviderCacheContract(t *testing.T) ProviderCacheContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "provider-cache-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider-cache contract fixture: %v", err)
	}
	var fixture ProviderCacheContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode provider-cache contract fixture: %v", err)
	}
	return fixture
}

func loadApprovalUserInputRouteContract(t *testing.T) ApprovalUserInputRouteContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "approval-user-input-route-contract.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read approval/user-input contract fixture: %v", err)
	}
	var fixture ApprovalUserInputRouteContract
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("decode approval/user-input contract fixture: %v", err)
	}
	return fixture
}

func loadMCPToolLifecycleContract(t *testing.T) MCPToolLifecycleContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "mcp-tool-lifecycle-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read MCP lifecycle contract fixture: %v", err)
	}
	var fixture MCPToolLifecycleContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode MCP lifecycle contract fixture: %v", err)
	}
	return fixture
}

func decodeToMap(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	return decoded
}

func assertRawJSONEqual(t *testing.T, actual []byte, contract json.RawMessage, label string) {
	t.Helper()
	var actualValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("%s returned invalid JSON: %v", label, err)
	}
	var contractValue any
	if err := json.Unmarshal(contract, &contractValue); err != nil {
		t.Fatalf("%s contract contains invalid JSON: %v", label, err)
	}
	if !reflect.DeepEqual(actualValue, contractValue) {
		t.Fatalf("%s body mismatch\nactual: %#v\ncontract: %#v", label, actualValue, contractValue)
	}
}

func splitSSEFrames(body string) []string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "\n\n")
}
