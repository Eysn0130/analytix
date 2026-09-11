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

func TestLiveLocalMinimalAgentLoopRoutesMatchContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	loop := loadMinimalAgentLoopContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:              g1.RuntimeToken,
		StartedAt:                 g1.StartedAt,
		Routes:                    g2.Routes,
		ProviderContract:          loadG3ProviderContract(t),
		G4Contract:                loadG4ToolsContract(t),
		ApprovalUserInputContract: loadApprovalUserInputRouteContract(t),
		MCPToolLifecycleContract:  loadMCPToolLifecycleContract(t),
		DurableTempDir:            t.TempDir(),
		LoopContract:              loop,
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundary := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/loop/boundary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, boundary, "minimalAgentLoopPrototype", true)
	assertBoolField(t, boundary, "fixtureBackedLoopOnly", true)
	assertBoolField(t, boundary, "usesFixtureModelScript", true)
	assertBoolField(t, boundary, "tempDurableEventSessionStoreEnabled", true)
	assertBoolField(t, boundary, "publishBeforePersist", false)
	assertBoolField(t, boundary, "persistBeforePublish", true)
	assertBoolField(t, boundary, "providerLiveCallsAllowed", false)
	assertBoolField(t, boundary, "toolExecutionAllowed", false)
	assertBoolField(t, boundary, "approvalExecutionAllowed", false)
	assertBoolField(t, boundary, "mcpConnectionAllowed", false)
	assertBoolField(t, boundary, "realWorkspaceMutationAllowed", false)
	assertBoolField(t, boundary, "defaultGoBackendEnabled", false)

	run := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/loop/run", g1.RuntimeToken, mustJSON(t, map[string]string{
		"threadId": loop.ThreadID,
		"turnId":   loop.TurnID,
	}), http.StatusOK)
	if run["status"] != "completed" {
		t.Fatalf("loop run status mismatch: %#v", run)
	}
	if !reflect.DeepEqual(stringSliceField(t, run, "eventKinds"), loop.Expected.EventKinds) {
		t.Fatalf("loop event kinds mismatch: %#v", run["eventKinds"])
	}
	if !reflect.DeepEqual(stringSliceField(t, run, "itemKinds"), loop.Expected.ItemKinds) {
		t.Fatalf("loop item kinds mismatch: %#v", run["itemKinds"])
	}
	if jsonIntField(t, run, "highestSeq") != loop.Expected.HighestSeq {
		t.Fatalf("loop highest seq mismatch: %#v", run)
	}
	assertBoolField(t, run, "allPersistBeforePublish", true)
	assertZeroSideEffects(t, mapField(t, run, "sideEffects"))

	replay := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/loop/replay?thread_id="+loop.ThreadID+"&after_seq=8", g1.RuntimeToken, nil, http.StatusOK)
	if len(arrayField(t, replay, "events")) != loop.Expected.ReplayAfterSeqCount {
		t.Fatalf("loop replay count mismatch: %#v", replay)
	}
	frames := durableSSEFrames(t, server.URL, "/v1/conformance/loop/threads/"+loop.ThreadID+"/events?since_seq=0", g1.RuntimeToken, "")
	if len(frames) != loop.Expected.SSEFrameCount {
		t.Fatalf("loop SSE frame count mismatch: got %d want %d", len(frames), loop.Expected.SSEFrameCount)
	}
	caughtUpFrames := durableSSEFrames(t, server.URL, "/v1/conformance/loop/threads/"+loop.ThreadID+"/events?since_seq=17", g1.RuntimeToken, "")
	if len(caughtUpFrames) != loop.Expected.CaughtUpReplayFrameCount {
		t.Fatalf("loop caught-up SSE mismatch: %#v", caughtUpFrames)
	}

	state := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/loop/recovered-state?thread_id="+loop.ThreadID, g1.RuntimeToken, nil, http.StatusOK)
	if !reflect.DeepEqual(stringSliceField(t, state, "eventKinds"), loop.Expected.EventKinds) {
		t.Fatalf("recovered event kinds mismatch: %#v", state["eventKinds"])
	}
	usage := mapField(t, state, "usageCacheAccounting")
	if !reflect.DeepEqual(stringSliceField(t, usage, "providers"), loop.Expected.Providers) ||
		jsonIntField(t, usage, "totalCacheHitTokens") != loop.Expected.TotalCacheHitTokens ||
		jsonIntField(t, usage, "totalCacheMissTokens") != loop.Expected.TotalCacheMissTokens {
		t.Fatalf("recovered cache telemetry mismatch: %#v", usage)
	}
	if !reflect.DeepEqual(stringSliceField(t, mapField(t, state, "approvals"), "statuses"), loop.Expected.ApprovalStatuses) {
		t.Fatalf("approval replay mismatch: %#v", state)
	}
	if !reflect.DeepEqual(stringSliceField(t, mapField(t, state, "userInputs"), "statuses"), loop.Expected.UserInputStatuses) {
		t.Fatalf("user-input replay mismatch: %#v", state)
	}
	mcp := mapField(t, state, "mcp")
	if !reflect.DeepEqual(stringSliceField(t, mcp, "toolNames"), loop.Expected.MCPToolNames) ||
		mcp["latestFingerprint"] != loop.Expected.LatestMCPFingerprint {
		t.Fatalf("MCP catalog replay mismatch: %#v", mcp)
	}

	cancel := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/loop/cancel", g1.RuntimeToken, mustJSON(t, map[string]string{
		"threadId": loop.CancelThreadID,
	}), http.StatusOK)
	if cancel["status"] != "aborted" || cancel["resultCode"] != "tool_call_cancelled" {
		t.Fatalf("cancel boundary mismatch: %#v", cancel)
	}
	assertBoolField(t, cancel, "allPersistBeforePublish", true)
	assertZeroSideEffects(t, mapField(t, cancel, "sideEffects"))

	resume := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/loop/resume", g1.RuntimeToken, mustJSON(t, map[string]string{
		"sourceThreadId": loop.ThreadID,
	}), http.StatusOK)
	assertBoolField(t, resume, "sourceMatchesExpected", true)
	assertBoolField(t, resume, "allPersistBeforePublish", true)
	assertZeroSideEffects(t, mapField(t, resume, "sideEffects"))
}

func loadMinimalAgentLoopContract(t *testing.T) GoMinimalAgentLoopContract {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-minimal-agent-loop-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read minimal agent loop contract fixture: %v", err)
	}
	var fixture GoMinimalAgentLoopContract
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode minimal agent loop contract fixture: %v", err)
	}
	return fixture
}

func assertZeroSideEffects(t *testing.T, sideEffects map[string]any) {
	t.Helper()
	for key, value := range sideEffects {
		number, ok := value.(float64)
		if !ok || int(number) != 0 {
			t.Fatalf("side effect %s should be 0, got %#v", key, sideEffects)
		}
	}
}
