//go:build !analytix_prod

package runtimego

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	upstreamaudit "analytix.local/runtime-go/internal/upstreamaudit"
)

func TestReasonixSuperiorityMatrixClosesCodeStageOnly(t *testing.T) {
	matrix := upstreamaudit.BuildReasonixSuperiorityMatrix(RuntimeReadinessStatusFromEnv(map[string]string{}))
	if matrix.SchemaVersion != 1 ||
		matrix.ChangeID != upstreamaudit.ReasonixSuperiorityMatrixChangeID ||
		matrix.Stage != "reasonix-superiority-code-stage" ||
		matrix.ReasonixSourcePath != upstreamaudit.ReasonixSourcePath ||
		matrix.ReasonixSourceCommit != upstreamaudit.ReasonixSourceCommit {
		t.Fatalf("matrix identity mismatch: %#v", matrix)
	}
	if !matrix.CodeStageClosed ||
		matrix.StrictG6DefaultCutoverReady ||
		!matrix.GoDefaultCutoverCandidate ||
		matrix.LiveCutoverStatus != "post-cutover-live-validation-pending" ||
		matrix.ReasonixAbsorptionStatus != "code-stage-closed" ||
		matrix.GoRuntimeCoreStatus != "deterministic-core-green" ||
		matrix.GoDefaultLiveGateStatus != "post-cutover-live-validation-pending" ||
		len(matrix.LiveEvidenceBlockers) != 5 {
		t.Fatalf("Reasonix superiority code stage must close while live validation stays pending: %#v", matrix)
	}
	if matrix.RowCount != 13 ||
		matrix.AbsorbCount != 1 ||
		matrix.AdaptCount != 5 ||
		matrix.KeepKunCount != 5 ||
		matrix.RejectCount != 1 ||
		matrix.DeferCount != 1 ||
		matrix.CodeLevelAbsorbedCount != 6 ||
		matrix.LocalDeterministicGreenCount != matrix.RowCount-matrix.DeferCount {
		t.Fatalf("matrix decision counts drifted: %#v", matrix)
	}
	if matrix.LLMAnswerQualityEvidenceUsed ||
		!matrix.DeterministicEvidenceOnly ||
		!matrix.DeepSeekEnhancementScopedOnly ||
		!matrix.KunAnalytixProductLayerPreserved ||
		!matrix.ReasonixEngineRuntimeOnly ||
		matrix.TypeScriptFallbackRetained {
		t.Fatalf("matrix must preserve Reasonix superiority evidence and product boundaries: %#v", matrix)
	}
	if matrix.ProductImpactChangedCount != 0 ||
		matrix.TopLevelEntrypointAddedCount != 0 ||
		matrix.ReasonixPublicProtocolAddedCount != 0 ||
		matrix.ProviderMultiModelChangedCount != 0 ||
		matrix.StablePrefixDynamicStateRowCount != 0 {
		t.Fatalf("matrix must not change UI/settings/bridge/provider or stable prefix boundaries: %#v", matrix)
	}

	rows := map[string]upstreamaudit.ReasonixSuperiorityEvidenceRow{}
	for _, row := range matrix.Rows {
		if row.ID == "" ||
			row.Capability == "" ||
			row.ReasonixSourcePath != upstreamaudit.ReasonixSourcePath ||
			row.ReasonixSourceCommit != upstreamaudit.ReasonixSourceCommit ||
			len(row.ReasonixSourceSymbols) == 0 ||
			len(row.KunAnalytixBaselinePaths) == 0 ||
			len(row.DeterministicEvidence) == 0 ||
			len(row.AnalytixTargetPaths) == 0 ||
			len(row.RegressionTestPaths) == 0 ||
			row.Decision == "" ||
			row.TypeScriptFallbackRetained {
			t.Fatalf("superiority row is incomplete: %#v", row)
		}
		if row.ID == "go-default-live-cutover-evidence" {
			if row.LocalDeterministicGreen {
				t.Fatalf("live cutover strict-blocked row must not count as local deterministic green: %#v", row)
			}
		} else if !row.LocalDeterministicGreen {
			t.Fatalf("code-stage row must remain local deterministic green: %#v", row)
		}
		for _, check := range row.DeterministicEvidence {
			if check.ID == "" || check.Kind == "" || check.Status == "" || check.Evidence == "" {
				t.Fatalf("deterministic evidence check must be complete: row=%s check=%#v", row.ID, check)
			}
		}
		if row.ProductImpact.UISurfaceChanged ||
			row.ProductImpact.SettingsSchemaChanged ||
			row.ProductImpact.BridgeChanged ||
			row.ProductImpact.ProviderMultiModelChanged ||
			row.ProductImpact.TopLevelEntrypointAdded ||
			row.ProductImpact.ReasonixPublicProtocolAdded ||
			row.ProductImpact.StablePrefixUsesDynamicState {
			t.Fatalf("row violates product boundary: %#v", row)
		}
		switch row.Decision {
		case upstreamaudit.SuperiorityDecisionAbsorb, upstreamaudit.SuperiorityDecisionAdapt:
			if len(row.ReasonixStrongerEvidence) == 0 {
				t.Fatalf("absorb/adapt row must include Reasonix stronger evidence: %#v", row)
			}
			if row.RequiresLiveEvidence || row.LiveEvidenceStatus != "not-required" {
				t.Fatalf("absorb/adapt row must not require live provider evidence for code-stage absorption: %#v", row)
			}
		case upstreamaudit.SuperiorityDecisionKeepKun:
			if row.ReasonixStrongerEvidence == nil || len(row.ReasonixStrongerEvidence) != 0 {
				t.Fatalf("keep-kun row must expose empty Reasonix stronger evidence: %#v", row)
			}
		case upstreamaudit.SuperiorityDecisionReject:
			if row.ReasonixStrongerEvidence == nil ||
				len(row.ReasonixStrongerEvidence) != 0 ||
				len(row.DecisionEvidence) == 0 ||
				row.RejectedReason == "" {
				t.Fatalf("reject row must expose empty Reasonix stronger evidence and use decision evidence: %#v", row)
			}
		case upstreamaudit.SuperiorityDecisionDefer:
			if row.ReasonixStrongerEvidence == nil ||
				len(row.ReasonixStrongerEvidence) != 0 ||
				len(row.DecisionEvidence) == 0 ||
				row.DeferredReason == "" {
				t.Fatalf("defer row must expose empty Reasonix stronger evidence and use decision evidence: %#v", row)
			}
		default:
			t.Fatalf("unknown Reasonix superiority decision: %#v", row)
		}
		rows[row.ID] = row
	}

	if rows["mcp-lifecycle-lazy-schema-reconnect-redaction"].Decision != upstreamaudit.SuperiorityDecisionAbsorb {
		t.Fatalf("MCP lifecycle row should be absorbed: %#v", rows["mcp-lifecycle-lazy-schema-reconnect-redaction"])
	}
	if rows["deepseek-prefix-cache-stability"].Decision != upstreamaudit.SuperiorityDecisionAdapt ||
		!rows["deepseek-prefix-cache-stability"].ProductImpact.ProviderScoped ||
		!rows["deepseek-prefix-cache-stability"].ProductImpact.DoesNotNarrowProviders {
		t.Fatalf("DeepSeek cache row must be provider scoped without narrowing providers: %#v", rows["deepseek-prefix-cache-stability"])
	}
	if rows["kun-analytix-product-layer-baseline"].Decision != upstreamaudit.SuperiorityDecisionKeepKun ||
		rows["kun-analytix-product-layer-baseline"].KeepKunReason == "" {
		t.Fatalf("product layer must keep Kun/Analytix baseline: %#v", rows["kun-analytix-product-layer-baseline"])
	}
	for _, id := range []string{
		"prefix-shape-contract-baseline",
		"provider-endpoint-family-request-shape",
		"mcp-search-meta-tool-boundary",
		"runtime-event-sse-session-durability",
	} {
		if rows[id].Decision != upstreamaudit.SuperiorityDecisionKeepKun ||
			rows[id].CodeLevelAbsorbed ||
			rows[id].KeepKunReason == "" ||
			len(rows[id].KunAnalytixStrongerEvidence) == 0 ||
			rows[id].BaselineRetainedReason == "" {
			t.Fatalf("Analytix/Kun stronger baseline row must stay keep-kun: id=%s row=%#v", id, rows[id])
		}
	}
	if len(rows["kun-analytix-product-layer-baseline"].KunAnalytixStrongerEvidence) == 0 ||
		rows["kun-analytix-product-layer-baseline"].BaselineRetainedReason == "" {
		t.Fatalf("product layer keep-kun row must include machine-readable baseline-retained evidence: %#v", rows["kun-analytix-product-layer-baseline"])
	}
	if rows["reasonix-public-protocol-and-top-level-ui"].Decision != upstreamaudit.SuperiorityDecisionReject ||
		rows["reasonix-public-protocol-and-top-level-ui"].RejectedReason == "" {
		t.Fatalf("Reasonix public protocol must be rejected: %#v", rows["reasonix-public-protocol-and-top-level-ui"])
	}
	if rows["go-default-live-cutover-evidence"].Decision != upstreamaudit.SuperiorityDecisionDefer ||
		!rows["go-default-live-cutover-evidence"].RequiresLiveEvidence ||
		rows["go-default-live-cutover-evidence"].LiveEvidenceStatus != "post-cutover-live-validation-pending" {
		t.Fatalf("Go default live validation must be post-cutover pending: %#v", rows["go-default-live-cutover-evidence"])
	}
	var defaultReadyPreflight upstreamaudit.RuntimeMachineCheck
	for _, check := range rows["go-default-live-cutover-evidence"].DeterministicEvidence {
		if check.ID == "runtime-go-preflight-live-gate" {
			defaultReadyPreflight = check
		}
	}
	if defaultReadyPreflight.Status != "post-cutover-live-validation-pending" ||
		defaultReadyPreflight.ExpectedBlocked == nil ||
		*defaultReadyPreflight.ExpectedBlocked ||
		defaultReadyPreflight.AcceptedAsCodePhasePass == nil ||
		!*defaultReadyPreflight.AcceptedAsCodePhasePass ||
		defaultReadyPreflight.DefaultBackendReady == nil ||
		!*defaultReadyPreflight.DefaultBackendReady ||
		len(defaultReadyPreflight.BlockerIDs) != 0 {
		t.Fatalf("runtime-go preflight live-gate evidence must stay post-cutover-only while keeping default backend ready: %#v", defaultReadyPreflight)
	}
	for _, path := range rows["go-default-live-cutover-evidence"].RegressionTestPaths {
		if strings.Contains(path, " ") {
			t.Fatalf("regressionTestPaths must contain paths, not commands: %q", path)
		}
	}
	if len(rows["go-default-live-cutover-evidence"].RegressionCommands) == 0 {
		t.Fatalf("deferred live cutover row must keep command evidence separately")
	}
}

func TestReasonixAbsorptionContractSeparatesCodeStageFromLiveCutover(t *testing.T) {
	matrix := upstreamaudit.BuildReasonixSuperiorityMatrix(RuntimeReadinessStatusFromEnv(map[string]string{}))
	rows := map[string]upstreamaudit.ReasonixSuperiorityEvidenceRow{}
	for _, row := range matrix.Rows {
		rows[row.ID] = row
	}
	absorbedIDs := []string{
		"deepseek-prefix-cache-stability",
		"provider-stream-usage-reasoning-guards",
		"agent-loop-job-subagent-lineage",
		"mcp-lifecycle-lazy-schema-reconnect-redaction",
		"approval-user-input-tool-result-history-repair",
		"autoresearch-workflow-create-loop-engine-discipline",
	}
	if !sameStringSet(matrix.DeterministicAbsorptionIDs, absorbedIDs) {
		t.Fatalf("deterministic absorption ids drifted: %#v", matrix.DeterministicAbsorptionIDs)
	}
	if !sameStringSet(matrix.PendingLiveValidationIDs, []string{"go-default-live-cutover-evidence"}) {
		t.Fatalf("only Go default cutover should require live validation: %#v", matrix.PendingLiveValidationIDs)
	}
	for _, id := range absorbedIDs {
		row := rows[id]
		if !row.CodeLevelAbsorbed ||
			!row.LocalDeterministicGreen ||
			row.RequiresLiveEvidence ||
			row.LiveEvidenceStatus != "not-required" ||
			len(row.DeterministicEvidence) == 0 ||
			len(row.ReasonixStrongerEvidence) == 0 {
			t.Fatalf("absorbed row must be deterministic-green without live credential dependency: id=%s row=%#v", id, row)
		}
	}
	liveGate := rows["go-default-live-cutover-evidence"]
	if liveGate.CodeLevelAbsorbed ||
		liveGate.LocalDeterministicGreen ||
		!liveGate.RequiresLiveEvidence ||
		liveGate.LiveEvidenceStatus != "post-cutover-live-validation-pending" ||
		liveGate.Decision != upstreamaudit.SuperiorityDecisionDefer {
		t.Fatalf("Go default live gate must stay separate from Reasonix absorption: %#v", liveGate)
	}
}

func TestReasonixSuperiorityMatrixCanReportLiveReadyOnlyWithStrictEvidence(t *testing.T) {
	ready := RuntimeReadinessStatusFromEnv(runtimeCredentialedReadinessEnvWithEvidence(t))
	matrix := upstreamaudit.BuildReasonixSuperiorityMatrix(ready)
	if !matrix.StrictG6DefaultCutoverReady ||
		!matrix.GoDefaultCutoverCandidate ||
		matrix.LiveCutoverStatus != "ready" ||
		matrix.ReasonixAbsorptionStatus != "code-stage-closed" ||
		matrix.GoDefaultLiveGateStatus != "ready" ||
		len(matrix.LiveEvidenceBlockers) != 0 {
		t.Fatalf("matrix should report live candidate only after strict evidence: %#v", matrix)
	}
	for _, row := range matrix.Rows {
		if row.RequiresLiveEvidence && row.LiveEvidenceStatus != "ready" {
			t.Fatalf("live-evidence row should be ready when strict evidence passes: %#v", row)
		}
	}
}

func TestRuntimeInfoDoesNotExposeReasonixSuperiorityMatrixOrIndexerProtocol(t *testing.T) {
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
		t.Fatalf("public runtime info must not expose internal superiority claims: %#v", capabilities)
	}
	forbidden := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/mcp-indexer", g1.RuntimeToken, nil, http.StatusNotFound)
	if forbidden["code"] != "not_found" {
		t.Fatalf("Reasonix superiority matrix must not add MCP-indexer routes: %#v", forbidden)
	}
}
