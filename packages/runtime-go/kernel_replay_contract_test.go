//go:build !analytix_prod

package runtimego

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLiveLocalKernelScaffoldRoutesMatchContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	kernel := loadKernelLiveScaffoldContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:              g1.RuntimeToken,
		StartedAt:                 g1.StartedAt,
		Routes:                    g2.Routes,
		ProviderContract:          loadG3ProviderContract(t),
		G4Contract:                loadG4ToolsContract(t),
		ApprovalUserInputContract: loadApprovalUserInputRouteContract(t),
		MCPToolLifecycleContract:  loadMCPToolLifecycleContract(t),
		DurableTempDir:            t.TempDir(),
		LoopContract:              loadMinimalAgentLoopContract(t),
		KernelContract:            kernel,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundary := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/kernel/boundary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, boundary, "fixtureOnly", true)
	assertBoolField(t, boundary, "kernelLiveScaffoldPrototype", true)
	assertBoolField(t, boundary, "fixtureBackedKernelOnly", true)
	assertBoolField(t, boundary, "internalGateOnly", true)
	assertBoolField(t, boundary, "durableStoreAvailable", true)
	assertBoolField(t, boundary, "minimalLoopAvailable", true)
	assertBoolField(t, boundary, "providerCacheReplayAvailable", true)
	assertBoolField(t, boundary, "approvalUserInputGateAvailable", true)
	assertBoolField(t, boundary, "mcpCatalogRecoveryAvailable", true)
	assertBoolField(t, boundary, "jobSubagentFixtureAvailable", true)
	assertBoolField(t, boundary, "providerLiveCallsAllowed", false)
	assertBoolField(t, boundary, "toolExecutionAllowed", false)
	assertBoolField(t, boundary, "approvalExecutionAllowed", false)
	assertBoolField(t, boundary, "mcpConnectionAllowed", false)
	assertBoolField(t, boundary, "defaultGoBackendEnabled", false)
	assertBoolField(t, boundary, "rendererVisibleGoRoutesAllowed", false)
	assertBoolField(t, boundary, "reasonixPublicProtocolAllowed", false)
	productBoundary := mapField(t, boundary, "productBoundary")
	assertBoolField(t, productBoundary, "kernelLiveScaffoldPrototype", true)
	assertBoolField(t, productBoundary, "fixtureBackedKernelOnly", true)
	assertBoolField(t, productBoundary, "internalGateOnly", true)

	capabilities := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/kernel/capabilities", g1.RuntimeToken, nil, http.StatusOK)
	if jsonIntField(t, capabilities, "componentCount") != kernel.Expected.ComponentCount ||
		jsonIntField(t, capabilities, "liveComponentCount") != kernel.Expected.LiveComponentCount {
		t.Fatalf("kernel component counts mismatch: %#v", capabilities)
	}
	assertBoolField(t, capabilities, "allExpectedLive", true)
	components := arrayField(t, capabilities, "components")
	if len(components) != kernel.Expected.ComponentCount {
		t.Fatalf("kernel component array mismatch: %#v", components)
	}
	for _, raw := range components {
		component := raw.(map[string]any)
		assertBoolField(t, component, "matchesExpected", true)
		assertBoolField(t, component, "sideEffectsAllowed", false)
	}
	assertSideEffectsMatch(t, mapField(t, capabilities, "sideEffects"), kernel.Expected.SideEffects)

	absorption := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/kernel/absorption-contract", g1.RuntimeToken, nil, http.StatusOK)
	if !reflect.DeepEqual(stringSliceField(t, absorption, "sourceContractIds"), kernel.SourceContractIDs) {
		t.Fatalf("kernel source contract ids mismatch: %#v", absorption)
	}
	if jsonIntField(t, absorption, "replaceOrPortCount") != kernel.Expected.ReplaceOrPortCount ||
		jsonIntField(t, absorption, "contractReimplementCount") != kernel.Expected.ContractReimplementCount ||
		jsonIntField(t, absorption, "deferCount") != kernel.Expected.DeferCount ||
		jsonIntField(t, absorption, "rejectCount") != kernel.Expected.RejectCount {
		t.Fatalf("kernel absorption counts mismatch: %#v", absorption)
	}
	assertBoolField(t, absorption, "matchesExpected", true)

	job := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/kernel/job-orchestration", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, job, "fixtureOnly", true)
	assertBoolField(t, job, "parentGoalRequired", true)
	assertBoolField(t, job, "topLevelRoutesExposed", false)
	assertBoolField(t, job, "publicSubagentProtocolAllowed", false)
	assertBoolField(t, job, "jobManagerLive", false)
	assertBoolField(t, job, "backgroundExecutionAllowed", false)
	if !reflect.DeepEqual(stringSliceField(t, job, "expectedStatuses"), kernel.JobOrchestration.ExpectedStatuses) {
		t.Fatalf("kernel job statuses mismatch: %#v", job)
	}
	if !reflect.DeepEqual(stringSliceField(t, job, "expectedTools"), kernel.JobOrchestration.ExpectedTools) {
		t.Fatalf("kernel job tools mismatch: %#v", job)
	}
	if jsonIntField(t, job, "forbiddenSurfaceCount") != kernel.Expected.ForbiddenSurfaceCount {
		t.Fatalf("kernel forbidden surface count mismatch: %#v", job)
	}
	assertBoolField(t, job, "matchesExpected", true)

	cleanup := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/kernel/g6-retirement-cleanup", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, cleanup, "fixtureOnly", true)
	assertBoolField(t, cleanup, "g6Ready", false)
	assertBoolField(t, cleanup, "noImmediateProductionDelete", true)
	assertBoolField(t, cleanup, "tsRuntimeRetained", true)
	if jsonIntField(t, cleanup, "currentRetainedCount") != kernel.Expected.CurrentRetainedCount ||
		jsonIntField(t, cleanup, "deleteAfterG6Count") != kernel.Expected.DeleteAfterG6Count ||
		jsonIntField(t, cleanup, "forbiddenRedundancyCount") != kernel.Expected.ForbiddenRedundancyCount ||
		jsonIntField(t, cleanup, "presentForbiddenRedundancyCount") != kernel.Expected.PresentForbiddenRedundancyCount ||
		jsonIntField(t, cleanup, "g6BlockerCount") != kernel.Expected.G6BlockerCount {
		t.Fatalf("kernel cleanup counts mismatch: %#v", cleanup)
	}
	assertBoolField(t, cleanup, "matchesExpected", true)

	snapshot := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/kernel/snapshot", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, snapshot, "durableStoreAvailable", true)
	assertBoolField(t, snapshot, "minimalLoopAvailable", true)
	assertBoolField(t, snapshot, "providerCacheReplayAvailable", true)
	assertBoolField(t, snapshot, "g4ManagerAvailable", true)
	assertSideEffectsMatch(t, mapField(t, snapshot, "sideEffects"), kernel.Expected.SideEffects)

	harnessSnapshot := harness.Snapshot()
	if harnessSnapshot.StubKernelScaffoldReplays < 5 {
		t.Fatalf("kernel replay counter was not reflected in harness snapshot: %#v", harnessSnapshot)
	}
}

func loadKernelLiveScaffoldContract(t *testing.T) GoKernelLiveScaffoldContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-kernel-live-scaffold-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read kernel live scaffold contract fixture: %v", err)
	}
	var fixture GoKernelLiveScaffoldContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode kernel live scaffold contract fixture: %v", err)
	}
	return fixture
}

func assertSideEffectsMatch(t *testing.T, actual map[string]any, expected map[string]int) {
	t.Helper()
	for key, value := range expected {
		if jsonIntField(t, actual, key) != value {
			t.Fatalf("side effect %s mismatch: got %#v want %d in %#v", key, actual[key], value, actual)
		}
	}
}
