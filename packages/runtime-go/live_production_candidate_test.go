//go:build !analytix_prod

package runtimego

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	providerscript "analytix.local/runtime-go/internal/testsupport/providerscript"
)

func TestRuntimeGoProviderContractStreamsScriptedHTTPAndParsesCacheUsage(t *testing.T) {
	result, err := providerscript.RunLocalProviderContract(context.Background())
	if err != nil {
		t.Fatalf("run provider contract: %v", err)
	}
	assertBoolField(t, result, "runtimeGoContractParitySlice", true)
	assertBoolField(t, result, "providerFamiliesCovered", true)
	assertBoolField(t, result, "contractReplayProviderServer", true)
	assertBoolField(t, result, "readsRealAPIKeys", false)
	assertBoolField(t, result, "externalNetworkUsed", false)
	assertBoolField(t, result, "deepseekCacheFieldsConsistent", true)
	assertBoolField(t, result, "stablePrefixEquivalent", true)
	assertBoolField(t, result, "dynamicStateInStablePrefix", false)
	if numberField(t, result, "requestShapeCount") != 3 {
		t.Fatalf("request shape count mismatch: %#v", result)
	}
	if numberField(t, result, "streamCompletedCount") != 3 {
		t.Fatalf("stream completion count mismatch: %#v", result)
	}
	if numberField(t, result, "deepseekCacheHitTokens") != 700 ||
		numberField(t, result, "deepseekCacheMissTokens") != 300 ||
		numberField(t, result, "openaiCompatibleCacheHitTokens") != 300 ||
		numberField(t, result, "anthropicCacheHitTokens") != 1000 {
		t.Fatalf("cache usage parsing mismatch: %#v", result)
	}
}

func TestRuntimeGoContractSidecarCoversDurableGateMCPJobAndChecklist(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundary := assertLiveJSON(t, server.URL, http.MethodGet, liveProductionCandidatePrefix+"/boundary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, boundary, "runtimeGoContractParitySlice", true)
	assertBoolField(t, boundary, "internalGateOnly", true)
	assertBoolField(t, boundary, "contractReplayOnly", false)
	assertBoolField(t, boundary, "usesContractReplayProviderServer", true)
	assertBoolField(t, boundary, "usesContractReplayMCPTransport", true)
	assertBoolField(t, boundary, "usesRealDurableEventSink", true)
	assertBoolField(t, boundary, "defaultGoBackendEnabled", false)
	assertBoolField(t, boundary, "rendererVisibleGoRoutesAllowed", false)
	assertBoolField(t, boundary, "reasonixPublicProtocolAllowed", false)

	durable := assertLiveJSON(t, server.URL, http.MethodPost, liveProductionCandidatePrefix+"/durable-replay", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, durable, "durableReplayFromEventSink", true)
	assertBoolField(t, durable, "usageCacheAccountingPersisted", true)
	assertBoolField(t, durable, "allPersistBeforePublish", true)
	if jsonIntField(t, durable, "eventCount") != 4 || jsonIntField(t, durable, "highestSeq") != 4 {
		t.Fatalf("durable production candidate replay mismatch: %#v", durable)
	}
	if eventKinds, ok := durable["eventKinds"].([]any); ok {
		for _, kind := range eventKinds {
			if kind == "assistant_reasoning_delta" || kind == "assistant_reasoning" {
				t.Fatalf("durable production candidate leaked reasoning event: %#v", durable)
			}
		}
	}

	gate := assertLiveJSON(t, server.URL, http.MethodPost, liveProductionCandidatePrefix+"/approval-user-input", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, gate, "deniedToolExecuted", false)
	assertBoolField(t, gate, "submitBodyEchoesAnswers", true)
	assertBoolField(t, gate, "submittedAnswersPersistedInEvents", false)
	assertBoolField(t, gate, "submittedAnswersPrivacyBoundary", true)
	assertBoolField(t, gate, "lateApprovalRejected", true)

	mcp := assertLiveJSON(t, server.URL, http.MethodPost, liveProductionCandidatePrefix+"/mcp-manager", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, mcp, "productionFallbackMCPTransportUsed", false)
	assertBoolField(t, mcp, "contractReplayMCPTransportUsed", true)
	assertBoolField(t, mcp, "connectedInitially", true)
	assertBoolField(t, mcp, "deniedMCPToolExecuted", false)
	assertBoolField(t, mcp, "approvedMCPToolExecuted", true)
	assertBoolField(t, mcp, "credentialRead", false)
	assertBoolField(t, mcp, "topLevelMCPIndexerRouteExposed", false)

	job := assertLiveJSON(t, server.URL, http.MethodPost, liveProductionCandidatePrefix+"/job-lineage", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, job, "parentGoalLineageRequired", true)
	assertBoolField(t, job, "sameParentLineage", true)
	assertBoolField(t, job, "topLevelRouteExposed", false)
	assertBoolField(t, job, "rendererVisibleRouteExposed", false)

	checklist := assertLiveJSON(t, server.URL, http.MethodGet, liveProductionCandidatePrefix+"/single-baseline-checklist", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, checklist, "machineTestable", true)
	assertBoolField(t, checklist, "tsRuntimeDefaultRetained", true)
	assertBoolField(t, checklist, "defaultGoBackendEnabled", false)
	assertBoolField(t, checklist, "rendererPreloadMainContractChange", false)
	assertBoolField(t, checklist, "runtimeReadinessDefaultReady", false)
	if jsonIntField(t, checklist, "presentForbiddenRedundancyCount") != 0 {
		t.Fatalf("forbidden redundancy should be absent: %#v", checklist)
	}
	if candidates, ok := checklist["postRuntimeReadinessDeleteCandidates"].([]any); !ok || len(candidates) < 5 {
		t.Fatalf("runtime readiness delete candidates should be machine-readable: %#v", checklist["postRuntimeReadinessDeleteCandidates"])
	}

	canary := assertLiveJSON(t, server.URL, http.MethodPost, liveProductionCandidatePrefix+"/canary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, canary, "ok", true)
	assertBoolField(t, canary, "runtimeGoContractParitySlice", true)
	assertBoolField(t, canary, "defaultGoBackendEnabled", false)
	assertBoolField(t, canary, "rendererVisibleGoRoutesAllowed", false)
	assertBoolField(t, canary, "reasonixPublicProtocolAllowed", false)
	checks := stringSliceField(t, canary, "checks")
	for _, expected := range []string{
		"contract-provider-server",
		"durable-replay",
		"approval-user-input-manager",
		"contract-mcp-manager",
		"job-lineage",
		"single-baseline-checklist",
	} {
		if !containsString(checks, expected) {
			t.Fatalf("canary missing check %q: %#v", expected, checks)
		}
	}

	forbidden := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/go", g1.RuntimeToken, nil, http.StatusNotFound)
	if forbidden["code"] != "not_found" {
		t.Fatalf("renderer-visible Go route should remain hidden: %#v", forbidden)
	}

	snapshot := harness.Snapshot()
	if snapshot.ProductionCandidate == nil || snapshot.ProductionCandidate["runtimeGoContractParitySlice"] != true {
		t.Fatalf("runtime contract snapshot missing: %#v", snapshot)
	}
}

func numberField(t *testing.T, record map[string]any, field string) int {
	t.Helper()
	switch value := record[field].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		t.Fatalf("field %q is not numeric: %#v", field, record[field])
		return 0
	}
}
