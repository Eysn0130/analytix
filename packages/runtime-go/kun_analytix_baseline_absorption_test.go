//go:build !analytix_prod

package runtimego

import (
	"context"
	"strings"
	"testing"

	upstreamaudit "analytix.local/runtime-go/internal/upstreamaudit"
)

func TestKunAnalytixBaselineGuardIsFullFunctionAndImmutable(t *testing.T) {
	guard := upstreamaudit.BuildKunAnalytixBaselineGuard()
	if guard.SchemaVersion != 1 || guard.ChangeID != upstreamaudit.KunAnalytixBaselineGuardChangeID || !guard.FullFunctionBaseline {
		t.Fatalf("baseline guard identity mismatch: %#v", guard)
	}
	if len(guard.KunSourceSnapshots) != 2 || guard.AnalytixSourceRoot != "/Users/sun/Projects/analytix" {
		t.Fatalf("baseline guard must record Kun/Analytix sources: %#v", guard)
	}
	if !strings.Contains(guard.KunSourceSnapshots[0], "github.com/KunAgent/Kun.git") ||
		!strings.Contains(guard.KunSourceSnapshots[0], "v0.2.13") ||
		!strings.Contains(guard.KunSourceSnapshots[1], "v0.2.14") {
		t.Fatalf("baseline guard must use KunAgent/Kun tag refs as source of truth: %#v", guard.KunSourceSnapshots)
	}
	if guard.ProtectedBaselineCount != len(guard.Rows) || guard.ProtectedBaselineCount < 7 {
		t.Fatalf("baseline guard must protect the full function surface: %#v", guard)
	}
	if guard.ForbiddenEntrypointExposed ||
		guard.DeprecatedBridgeAliasAllowed ||
		guard.LegacySettingsWriteAllowed ||
		guard.DeepSeekOnlyRuntimeAllowed ||
		!guard.ReadyForReasonixAbsorption {
		t.Fatalf("baseline guard allows forbidden product drift: %#v", guard)
	}
	rows := map[string]upstreamaudit.KunAnalytixBaselineRow{}
	for _, row := range guard.Rows {
		if row.ID == "" ||
			row.EntryPoint == "" ||
			row.TriggerPath == "" ||
			row.RuntimeContract == "" ||
			row.SettingsSchema == "" ||
			row.UISurface == "" ||
			len(row.ProtectionTests) == 0 ||
			!row.ImmutableProductBaseline ||
			row.Status != "protected" {
			t.Fatalf("baseline row is incomplete: %#v", row)
		}
		rows[row.ID] = row
	}
	for _, id := range []string{
		"product-identity-release",
		"bridge-settings-schema",
		"desktop-entry-ui",
		"chat-thread-session-sse",
		"provider-model-multimodel",
		"tools-approval-user-input-mcp-goal",
		"attachments-workspace-write-sdd-connect-phone",
	} {
		if _, ok := rows[id]; !ok {
			t.Fatalf("missing full-function baseline row %s", id)
		}
	}
	if !strings.Contains(rows["provider-model-multimodel"].ReasonixEnhancementAllowed, "DeepSeek cache/prefix enhancements only") {
		t.Fatalf("provider baseline must constrain DeepSeek absorption: %#v", rows["provider-model-multimodel"])
	}
}

func TestReasonixAbsorptionMatrixProtectsKunAnalytixBaseline(t *testing.T) {
	matrix := upstreamaudit.BuildReasonixAbsorptionMatrix()
	if matrix.SchemaVersion != 1 ||
		matrix.ChangeID != upstreamaudit.ReasonixAbsorptionMatrixChangeID ||
		matrix.ReasonixSourcePath != upstreamaudit.ReasonixSourcePath ||
		matrix.ReasonixSourceCommit != upstreamaudit.ReasonixSourceCommit {
		t.Fatalf("absorption matrix identity mismatch: %#v", matrix)
	}
	if matrix.GreenCount != 6 ||
		matrix.DeferredCount != 0 ||
		matrix.RejectedCount != 1 ||
		matrix.RedCount != 0 ||
		matrix.ForbiddenProductSurfaceCount != 1 ||
		matrix.MultiModelNonRegressionGreen != true ||
		matrix.DeepSeekEnhancementScopedOnly != true ||
		matrix.ReadyForG6 != true ||
		matrix.AbsorptionMatrixGreen != true ||
		matrix.DefaultBackendReady != false {
		t.Fatalf("absorption matrix status mismatch: %#v", matrix)
	}
	if len(matrix.ReadinessSemantics) == 0 {
		t.Fatalf("absorption matrix must document G6/default-backend semantics: %#v", matrix)
	}
	if len(matrix.G6Blockers) != 0 {
		t.Fatalf("Reasonix absorption matrix should have no red/deferred G6 blockers after AutoResearch is implemented: %#v", matrix.G6Blockers)
	}
	if !sameStringSet(matrix.ProviderFamilies, []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom_endpoint"}) {
		t.Fatalf("absorption matrix must keep all provider families: %#v", matrix.ProviderFamilies)
	}
	rows := map[string]upstreamaudit.ReasonixAbsorptionRow{}
	for _, row := range matrix.Rows {
		if row.ID == "" || row.AnalytixLanding == "" || len(row.MachineChecks) == 0 || len(row.KunBaselineProtection) == 0 {
			t.Fatalf("absorption row is incomplete: %#v", row)
		}
		if row.Status == upstreamaudit.AbsorptionStatusGreen && !row.DoesNotNarrowProviders {
			t.Fatalf("green absorption rows must not narrow providers: %#v", row)
		}
		rows[row.ID] = row
	}
	if rows["reasonix-public-protocol-product-entry"].Status != upstreamaudit.AbsorptionStatusRejected ||
		!rows["reasonix-public-protocol-product-entry"].ForbiddenProductSurface {
		t.Fatalf("Reasonix product surfaces must remain rejected: %#v", rows["reasonix-public-protocol-product-entry"])
	}
	if rows["autoresearch-project-state"].Status != upstreamaudit.AbsorptionStatusGreen ||
		rows["autoresearch-project-state"].AbsorptionClass != upstreamaudit.AbsorptionContractReimplement {
		t.Fatalf("AutoResearch project state must be green under /goal --research without product entry drift: %#v", rows["autoresearch-project-state"])
	}
}

func TestMultiModelNonRegressionMatrix(t *testing.T) {
	result, err := upstreamaudit.RunMultiModelNonRegressionMatrix(context.Background())
	if err != nil {
		t.Fatalf("run runtime multi-model matrix: %v", err)
	}
	if !result.Green ||
		!result.DeepSeekProviderSpecific ||
		!result.NonDeepSeekCacheDiagnosticsOff ||
		!result.CustomEndpointUsesExactURL ||
		result.ReadsRealAPIKeysByDefault {
		t.Fatalf("multi-model matrix must be green without narrowing providers: %#v", result)
	}
	if !sameStringSet(result.ProviderFamilies, []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom_endpoint"}) {
		t.Fatalf("provider families mismatch: %#v", result.ProviderFamilies)
	}
	for _, probe := range result.Probes {
		if probe.Status != "passed" || probe.RequestURL == "" || len(probe.RequestBodyFields) == 0 {
			t.Fatalf("probe did not complete: %#v", probe)
		}
		switch probe.Family {
		case "deepseek":
			if !probe.CacheTelemetrySupported ||
				!containsString(probe.DeepSeekRequestOnlyFields, "thinking") ||
				!containsString(probe.DeepSeekRequestOnlyFields, "reasoning_effort") ||
				probe.Usage.CacheHitTokens != 700 ||
				probe.Usage.CacheMissTokens != 300 {
				t.Fatalf("DeepSeek probe must keep provider-specific cache/prefix fields: %#v", probe)
			}
		case "openai-compatible":
			if probe.CacheTelemetrySupported ||
				containsString(probe.RequestBodyFields, "thinking") ||
				!strings.HasSuffix(probe.RequestURL, "/openai/v1/chat/completions") ||
				probe.Usage.TotalTokens != 430 {
				t.Fatalf("OpenAI-compatible probe regressed: %#v", probe)
			}
		case "anthropic-compatible":
			if probe.CacheTelemetrySupported ||
				containsString(probe.RequestBodyFields, "thinking") ||
				!strings.HasSuffix(probe.RequestURL, "/anthropic/v1/messages") ||
				probe.Usage.TotalTokens != 1270 {
				t.Fatalf("Anthropic-compatible probe regressed: %#v", probe)
			}
		case "custom_endpoint":
			if probe.CacheTelemetrySupported ||
				containsString(probe.RequestBodyFields, "thinking") ||
				!strings.HasSuffix(probe.RequestURL, "/custom-endpoint") ||
				!sameStringSet(probe.RequestBodyFields, []string{"messages", "model", "stream", "stream_options", "tools"}) ||
				probe.Usage.TotalTokens != 325 {
				t.Fatalf("custom endpoint probe regressed: %#v", probe)
			}
		default:
			t.Fatalf("unexpected provider family: %#v", probe)
		}
	}
}

func TestProviderTurnConfigDoesNotDefaultToDeepSeekForOtherFamilies(t *testing.T) {
	base := "http://127.0.0.1:9999"
	cases := []struct {
		providerID         string
		model              string
		family             string
		endpointFormat     string
		cacheSupported     bool
		deepseekPrefixOnly bool
		baseSuffix         string
	}{
		{"deepseek-test-local", "", "deepseek", "chat_completions", true, true, "/deepseek/v1"},
		{"openai-compatible-test-local", "", "openai-compatible", "chat_completions", false, false, "/openai/v1"},
		{"anthropic-compatible-test-local", "", "anthropic-compatible", "messages", false, false, "/anthropic"},
		{"custom-endpoint-test-local", "", "custom_endpoint", "custom_endpoint", false, false, "/custom-endpoint"},
		{"zai-coding-plan", "glm-5", "custom_endpoint", "custom_endpoint", false, false, "/custom-endpoint"},
	}
	for _, tc := range cases {
		config := upstreamaudit.ProviderTurnConfig(tc.providerID, tc.model, base)
		if config.Family != tc.family ||
			config.EndpointFormat != tc.endpointFormat ||
			config.CacheTelemetrySupported != tc.cacheSupported ||
			config.DeepSeekPrefixEnhancement != tc.deepseekPrefixOnly ||
			!strings.HasSuffix(config.BaseURL, tc.baseSuffix) {
			t.Fatalf("provider config mismatch for %s: %#v", tc.providerID, config)
		}
		if config.Family != "deepseek" && config.ReasoningEffort != "" {
			t.Fatalf("non-DeepSeek config must not inherit DeepSeek default reasoning effort: %#v", config)
		}
	}
}
