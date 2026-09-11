//go:build !analytix_prod

package runtimego

import (
	"reflect"
	"testing"

	agent "analytix.local/runtime-go/internal/agent"
	conformance "analytix.local/runtime-go/internal/conformance"
	livelocal "analytix.local/runtime-go/internal/conformance/livelocal"
	contracts "analytix.local/runtime-go/internal/contracts"
	mcp "analytix.local/runtime-go/internal/mcp"
	provider "analytix.local/runtime-go/internal/provider"
	server "analytix.local/runtime-go/internal/server"
)

func TestRootCompatibilityShimsDelegateToInternalDomains(t *testing.T) {
	var _ livelocal.LiveLocalSidecarConfig = LiveLocalSidecarConfig{}
	var _ contracts.LiveLocalSidecarBoundary = LiveLocalSidecarBoundary{}
	var _ server.RuntimeServerContractConfig = RuntimeServerContractConfig{}
	var _ provider.G3ProviderConformanceContract = G3ProviderConformanceContract{}
	var _ mcp.G4ToolsConformanceContract = G4ToolsConformanceContract{}
	var _ conformance.G5FullLoopConformanceContract = G5FullLoopConformanceContract{}
	var _ agent.GoMinimalAgentLoopContract = GoMinimalAgentLoopContract{}

	if got, ok := NewLiveLocalSidecarHandler(LiveLocalSidecarConfig{}).(*livelocal.LiveLocalSidecarHarness); !ok || got == nil {
		t.Fatalf("NewLiveLocalSidecarHandler must return internal/conformance/livelocal harness, got %T", got)
	}
	if got := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{}); got == nil {
		t.Fatal("NewLiveLocalSidecarHarness returned nil")
	}
	if !reflect.DeepEqual(LiveLocalSidecarProductBoundary(), contracts.LiveLocalSidecarProductBoundary()) {
		t.Fatal("root live-local boundary diverged from internal/contracts")
	}
	if !reflect.DeepEqual(RuntimeServerContractProductBoundary(), contracts.RuntimeServerContractProductBoundary()) {
		t.Fatal("root runtime server boundary diverged from internal/contracts")
	}

	mutable := newLiveLocalG2MutableHandler("", nil)
	var _ *livelocal.LiveLocalG2MutableHandler = mutable
	store := mutable.Store()
	var _ *livelocal.LiveLocalG2Store = store
	var _ *provider.LiveLocalG3ProviderHandler = newLiveLocalG3ProviderHandler("", G3ProviderConformanceContract{}, store)
	var _ *mcp.LiveLocalG4ManagerHandler = newLiveLocalG4ManagerHandler(
		"",
		G4ToolsConformanceContract{},
		ApprovalUserInputRouteContract{},
		MCPToolLifecycleContract{},
		store,
	)

	durable, err := newLiveLocalDurableHandler("", t.TempDir(), nil)
	if err != nil {
		t.Fatalf("newLiveLocalDurableHandler: %v", err)
	}
	var _ *livelocal.LiveLocalDurableHandler = durable
	var _ *livelocal.LiveLocalLoopHandler = newLiveLocalLoopHandler("", GoMinimalAgentLoopContract{}, durable.Store())
	var _ *livelocal.LiveProductionCandidateHandler = newLiveProductionCandidateHandler("", durable.Store())

	handler := newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{DataDir: t.TempDir(), DurableTempDir: t.TempDir()})
	if handler == nil {
		t.Fatal("NewRuntimeServerContractHandler returned nil")
	}
}

func TestRootG5UsageHelpersDelegateToInternalServer(t *testing.T) {
	promptTokens := 3
	cacheHitRate := 0.5
	frames := []string{
		"event: usage\ndata: {\"usage\":{\"promptTokens\":3,\"cacheHitRate\":0.5}}\n\n",
	}
	usage := usagePayloadFromSSEFrames(frames)
	if got := intFromUsagePayload(usage, "promptTokens"); got != promptTokens {
		t.Fatalf("promptTokens = %d, want %d", got, promptTokens)
	}
	if got := floatFromUsagePayload(usage, "cacheHitRate"); got != cacheHitRate {
		t.Fatalf("cacheHitRate = %f, want %f", got, cacheHitRate)
	}
	if !usagePayloadMatchesExpected(usage, G3ProviderUsageSummary{
		PromptTokens: &promptTokens,
		CacheHitRate: &cacheHitRate,
	}) {
		t.Fatal("usage payload did not match expected summary")
	}
	if !usageSummaryMatchesExpected(
		G3ProviderUsageSummary{PromptTokens: &promptTokens},
		G3ProviderUsageSummary{PromptTokens: &promptTokens},
	) {
		t.Fatal("usage summaries did not match")
	}
	if got := sseEventNames(frames); !reflect.DeepEqual(got, []string{"usage"}) {
		t.Fatalf("sseEventNames = %v, want [usage]", got)
	}
}
