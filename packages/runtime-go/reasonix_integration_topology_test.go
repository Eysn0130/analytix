//go:build !analytix_prod

package runtimego

import (
	"net/http"
	"net/http/httptest"
	"testing"

	upstreamaudit "analytix.local/runtime-go/internal/upstreamaudit"
)

func TestReasonixIntegrationTopologyBindsEngineToAnalytixContracts(t *testing.T) {
	topology := upstreamaudit.BuildReasonixIntegrationTopology(RuntimeReadinessStatusFromEnv(map[string]string{}))
	if topology.SchemaVersion != 1 ||
		topology.ChangeID != upstreamaudit.ReasonixIntegrationTopologyChangeID ||
		topology.ReasonixSourcePath != upstreamaudit.ReasonixSourcePath ||
		topology.ReasonixSourceCommit != upstreamaudit.ReasonixSourceCommit ||
		topology.AnalytixSourceRoot != "/Users/sun/Projects/analytix" {
		t.Fatalf("topology identity mismatch: %#v", topology)
	}
	if topology.RowCount != 5 ||
		topology.BaselineSurfaceCount != 4 ||
		topology.ReasonixStrongerAbsorbedCount != topology.RowCount ||
		topology.ConflictPolicyCount != 4 ||
		topology.KunAnalytixRetainedSurfaceCount != topology.BaselineSurfaceCount ||
		topology.CodeLevelAbsorbedCount != topology.RowCount ||
		topology.ExistingEntryOnlyCount != topology.RowCount ||
		topology.LocalContractGreenCount != topology.RowCount {
		t.Fatalf("topology rows must all be code-level absorbed and bound to existing entries: %#v", topology)
	}
	if topology.TopLevelEntrypointAddedCount != 0 ||
		topology.UpstreamPublicProtocolAddedCount != 0 ||
		topology.RendererContractChangedCount != 0 ||
		topology.SettingsSchemaChangedCount != 0 ||
		topology.ProductIdentityChangedCount != 0 ||
		topology.StablePrefixDynamicStateRowCount != 0 {
		t.Fatalf("topology must preserve product/runtime boundaries: %#v", topology)
	}
	if !topology.DeepSeekEnhancementScopedOnly ||
		!topology.MultiModelNonRegressionProtected ||
		topology.TypeScriptFallbackRetained ||
		topology.StrictG6DefaultCutoverReady ||
		!topology.GoDefaultCutoverCandidate {
		t.Fatalf("topology must keep DeepSeek scoped, retire TS fallback, and mark live evidence as post-cutover: %#v", topology)
	}
	if len(topology.ExternalEvidenceBlockers) != 5 {
		t.Fatalf("topology should name the external G6 blockers without env evidence: %#v", topology.ExternalEvidenceBlockers)
	}

	rows := map[string]upstreamaudit.ReasonixIntegrationTopologyRow{}
	for _, row := range topology.Rows {
		if row.ID == "" ||
			row.Capability == "" ||
			row.ComparisonConclusion == "" ||
			row.Decision == "" ||
			row.ReasonixStrongerBecause == "" ||
			row.KunAnalytixRetainedBecause == "" ||
			row.ConflictPolicy == "" ||
			len(row.AdoptedEngineConstants) == 0 ||
			len(row.RetainedProductConstants) == 0 ||
			len(row.ReasonixEngineNodes) == 0 ||
			len(row.KunAnalytixBaselineAnchors) == 0 ||
			len(row.AnalytixEntryPoints) == 0 ||
			len(row.RuntimeContracts) == 0 ||
			len(row.GoRuntimeLanding) == 0 ||
			len(row.TypeScriptFallback) == 0 ||
			len(row.MachineChecks) == 0 ||
			!row.EvidenceState.CodeLevelAbsorbed ||
			!row.EvidenceState.LocalContractGreen ||
			row.EvidenceState.RequiresCredentialedG6 ||
			row.EvidenceState.CredentialedEvidence != "post-cutover-live-validation-pending" ||
			!row.EvidenceState.DefaultCutoverCandidate ||
			row.EvidenceState.TypeScriptFallbackRetain {
			t.Fatalf("topology row is incomplete: %#v", row)
		}
		if !row.ExistingEntryOnly ||
			row.TopLevelEntrypointAdded ||
			row.UpstreamPublicProtocolAdded ||
			row.RendererContractChanged ||
			row.SettingsSchemaChanged ||
			row.ProductIdentityChanged ||
			row.StablePrefixContainsDynamicState {
			t.Fatalf("topology row violates analytix boundaries: %#v", row)
		}
		rows[row.ID] = row
	}
	for _, id := range []string{
		"deepseek-cache-prefix-provider-adapter",
		"agent-loop-job-subagent-lineage",
		"mcp-lifecycle-search-call-reconnect-redaction",
		"autoresearch-project-state-goal-research",
		"workflow-create-loop-internal-planner",
	} {
		if _, ok := rows[id]; !ok {
			t.Fatalf("missing topology row %s", id)
		}
	}
	deepseek := rows["deepseek-cache-prefix-provider-adapter"]
	if !deepseek.ProviderSpecific ||
		deepseek.ComparisonConclusion != upstreamaudit.IntegrationDecisionConflictProductBaselineEngineAbs ||
		!deepseek.DoesNotNarrowProviders ||
		!sameStringSet(deepseek.ProviderFamilies, []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom_endpoint"}) {
		t.Fatalf("DeepSeek cache row must be provider-specific without narrowing providers: %#v", deepseek)
	}
	if rows["mcp-lifecycle-search-call-reconnect-redaction"].ComparisonConclusion != upstreamaudit.IntegrationDecisionReasonixEngineStrongerAbsorb {
		t.Fatalf("MCP row should be a pure Reasonix stronger engine absorption: %#v", rows["mcp-lifecycle-search-call-reconnect-redaction"])
	}
	for _, surface := range topology.BaselineSurfaces {
		if surface.Decision != upstreamaudit.IntegrationDecisionKunAnalytixBaselineRetained ||
			surface.ID == "" ||
			surface.TriggerPath == "" ||
			surface.RuntimeContract == "" ||
			surface.SettingsBridgePolicy == "" ||
			surface.ProviderPolicy == "" ||
			len(surface.ProtectionTests) == 0 {
			t.Fatalf("baseline surface should prove retained Kun/Analytix product contract: %#v", surface)
		}
	}
}

func TestReasonixIntegrationTopologyCanReportStrictG6CandidateOnlyWithEvidence(t *testing.T) {
	ready := RuntimeReadinessStatusFromEnv(runtimeCredentialedReadinessEnvWithEvidence(t))
	topology := upstreamaudit.BuildReasonixIntegrationTopology(ready)
	if !topology.StrictG6DefaultCutoverReady ||
		!topology.GoDefaultCutoverCandidate ||
		len(topology.ExternalEvidenceBlockers) != 0 {
		t.Fatalf("topology should report cutover candidate only after strict runtime evidence: %#v", topology)
	}
	for _, row := range topology.Rows {
		if !row.EvidenceState.DefaultCutoverCandidate || len(row.EvidenceState.Blockers) != 0 {
			t.Fatalf("row should be candidate only when strict evidence is ready: %#v", row)
		}
	}
}

func TestRuntimeInfoDoesNotExposeReasonixIntegrationTopologyOrWorkflowProtocol(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", g1.RuntimeToken, nil, http.StatusOK)
	capabilities := mapField(t, info, "capabilities")
	if _, exposed := capabilities["upstreamAbsorption"]; exposed {
		t.Fatalf("public runtime info must not expose internal integration topology: %#v", capabilities)
	}
	forbidden := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/workflows", g1.RuntimeToken, nil, http.StatusNotFound)
	if forbidden["code"] != "not_found" {
		t.Fatalf("topology must not add workflow routes: %#v", forbidden)
	}
}
