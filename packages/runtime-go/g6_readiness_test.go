//go:build !analytix_prod

package runtimego

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var defaultRuntimeGateBackendField = strings.Join([]string{"runtime", "Backend"}, "")

func TestRuntimeProviderMatrixScaffoldRunsFixtureAndSkipsCredentialedWithoutEnv(t *testing.T) {
	ctx, cancel := runtimeReadinessProbeContext()
	defer cancel()

	result, err := RunProviderReadinessMatrix(ctx, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("run provider matrix: %v", err)
	}
	if !result.ProviderMatrixScaffold || !result.FixtureMatrixRequired || !result.CredentialedMatrixEnvGated {
		t.Fatalf("provider matrix scaffold flags mismatch: %#v", result)
	}
	if len(result.FixtureProbes) != 4 {
		t.Fatalf("expected DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom endpoint fixture probes: %#v", result.FixtureProbes)
	}
	for _, probe := range result.FixtureProbes {
		if probe.Status != "passed" || probe.Skipped {
			t.Fatalf("fixture probe should pass and never skip: %#v", probe)
		}
	}
	if result.DeepSeekCacheBenchmarkFormat["prefixHash"] == "" ||
		result.DeepSeekCacheBenchmarkFormat["cacheHitTokens"] != 700 ||
		result.DeepSeekCacheBenchmarkFormat["recordedAtFieldName"] != "recordedAt" {
		t.Fatalf("DeepSeek cache benchmark record format mismatch: %#v", result.DeepSeekCacheBenchmarkFormat)
	}
	if len(result.CredentialedProbes) != 4 {
		t.Fatalf("credentialed probe matrix should include every provider family: %#v", result.CredentialedProbes)
	}
	for _, probe := range result.CredentialedProbes {
		if probe.Status != "skipped" || !probe.Skipped {
			t.Fatalf("credentialed probe without env must be skipped, not passed: %#v", probe)
		}
	}
}

func TestRuntimeMCPMatrixScaffoldRunsFixtureAndSkipsCredentialedWithoutEnv(t *testing.T) {
	result := RunMCPReadinessMatrix(map[string]string{})
	if !result.MCPMatrixScaffold || !result.FixtureMCPRequired || !result.CredentialedMCPEnvGated {
		t.Fatalf("MCP matrix scaffold flags mismatch: %#v", result)
	}
	if result.TopLevelMCPIndexerExposed || result.ReasonixPublicProtocolUsed {
		t.Fatalf("MCP matrix must not expose forbidden public surfaces: %#v", result)
	}
	for _, key := range []string{"connect", "search", "callRequiresApproval", "approvedCallExecutes", "reconnect", "credentialRedaction"} {
		if result.FixtureProbe[key] != true {
			t.Fatalf("fixture MCP probe missing %s evidence: %#v", key, result.FixtureProbe)
		}
	}
	if len(result.CredentialedProbes) != 1 ||
		result.CredentialedProbes[0].Status != "skipped" ||
		!result.CredentialedProbes[0].Skipped {
		t.Fatalf("credentialed MCP without env must be skipped, not passed: %#v", result.CredentialedProbes)
	}

	configured := RunMCPReadinessMatrix(map[string]string{
		RuntimeMCPCommandEnv: "node real-mcp-runner.js",
	})
	if len(configured.CredentialedProbes) != 1 ||
		configured.CredentialedProbes[0].Status != "skipped" ||
		!configured.CredentialedProbes[0].Skipped {
		t.Fatalf("configured MCP command alone must still be skipped until external execution evidence exists: %#v", configured.CredentialedProbes)
	}
}

func TestRuntimeG6ReadinessDefaultsFalseAndRequiresAllEvidence(t *testing.T) {
	defaultStatus := RuntimeReadinessStatusFromEnv(map[string]string{})
	if defaultStatus.Ready || !defaultStatus.DefaultGoBackendEnabled || defaultStatus.RendererVisibleGoSwitcher {
		t.Fatalf("G6 readiness must keep strict live readiness false while Go remains default and hidden from renderer switches: %#v", defaultStatus)
	}
	if len(defaultStatus.MissingRequiredChecks) != 5 {
		t.Fatalf("default readiness should report every hard gate missing: %#v", defaultStatus.MissingRequiredChecks)
	}

	ready := RuntimeReadinessStatusFromEnv(runtimeCredentialedReadinessEnvWithEvidence(t))
	if !ready.Ready || len(ready.MissingRequiredChecks) != 0 {
		t.Fatalf("explicitly passed G6 evidence should report ready: %#v", ready)
	}

	fixtureOnly := RuntimeReadinessStatusFromEnv(map[string]string{
		RuntimeReadyEnv:                  "1",
		RuntimeDurableRestartEvidenceEnv: "passed",
		RuntimeProviderMatrixEnv:         "passed",
		RuntimeMCPMatrixEnv:              "passed",
		RuntimePackagedQAEnv:             "passed",
	})
	if fixtureOnly.Ready ||
		fixtureOnly.ProviderMatrix.Status != "failed" ||
		fixtureOnly.MCPMatrix.Status != "failed" ||
		fixtureOnly.PackagedQA.Status != "failed" {
		t.Fatalf("fixture/local matrix pass must not count as credentialed default-backend readiness: %#v", fixtureOnly)
	}
}

func TestRuntimeG6ReadinessAcceptsUpgradedOperatorGateIDs(t *testing.T) {
	env := runtimeCredentialedReadinessEnvWithEvidence(t)
	operator := env[RuntimeReadyEnv+"_EVIDENCE"]
	body, err := os.ReadFile(operator)
	if err != nil {
		t.Fatalf("read operator evidence: %v", err)
	}
	upgraded := strings.Replace(
		string(body),
		`"id": "runtime-operator-gate"`,
		`"id": "go-runtime-operator-gate"`,
		1,
	)
	writeEvidenceFile(t, operator, upgraded)

	ready := RuntimeReadinessStatusFromEnv(env)
	if !ready.Ready || ready.OperatorGate.Status != "passed" || len(ready.MissingRequiredChecks) != 0 {
		t.Fatalf("current operator gate id should remain strict-G6 ready: %#v", ready)
	}
}

func TestRuntimeG6ReadinessRejectsThinPackagedEvidence(t *testing.T) {
	env := runtimeCredentialedReadinessEnvWithEvidence(t)
	packaged := env[RuntimePackagedQAEnv+"_EVIDENCE"]
	writeEvidenceFile(t, packaged, `{
		"schemaVersion": 1,
		"id": "runtime-packaged-qa",
		"passed": true,
		"checks": [
			{ "id": "packaged-app-startup", "status": "passed" },
			{ "id": "go-runtime-default-gate", "status": "passed" },
			{ "id": "health", "status": "passed" },
			{ "id": "thread-list", "status": "passed" },
			{ "id": "turn-create", "status": "passed" },
			{ "id": "sse-replay", "status": "passed" },
			{ "id": "typescript-retired-backend", "status": "passed" }
		]
	}`)

	ready := RuntimeReadinessStatusFromEnv(env)
	if ready.Ready || ready.PackagedQA.Status != "failed" || !strings.Contains(ready.PackagedQA.Message, "process/runtime/retired-backend evidence") {
		t.Fatalf("thin packaged evidence must fail strict G6 readiness: %#v", ready)
	}
}

func TestRuntimeG6ReadinessRejectsThinOperatorEvidence(t *testing.T) {
	env := runtimeCredentialedReadinessEnvWithEvidence(t)
	operator := env[RuntimeReadyEnv+"_EVIDENCE"]
	writeEvidenceFile(t, operator, `{
		"schemaVersion": 1,
		"id": "go-runtime-operator-gate",
		"status": "passed",
		"passed": true,
		"operatorGate": "`+RuntimeReadyEnv+`=1",
		"explicitEnvGate": true,
		"credentialedEvidenceReviewed": true,
		"goDefaultApproved": true,
		"typeScriptFallbackRetained": false,
		"goDefaultBackendEnabled": true
	}`)

	ready := RuntimeReadinessStatusFromEnv(env)
	if ready.Ready || ready.OperatorGate.Status != "failed" || !strings.Contains(ready.OperatorGate.Message, "digest bindings") {
		t.Fatalf("thin operator evidence must fail strict G6 readiness: %#v", ready)
	}
}

func TestRuntimeG6ReadinessRejectsHeaderStyleSecrets(t *testing.T) {
	env := runtimeCredentialedReadinessEnvWithEvidence(t)
	provider := env[RuntimeProviderMatrixEnv+"_EVIDENCE"]
	body, err := os.ReadFile(provider)
	if err != nil {
		t.Fatalf("read provider evidence: %v", err)
	}
	leaky := strings.Replace(
		string(body),
		`"schemaVersion": 1,`,
		`"schemaVersion": 1, "leakedHeader": "authorization: Bearer sk-live-secret-123456789012345",`,
		1,
	)
	writeEvidenceFile(t, provider, leaky)

	ready := RuntimeReadinessStatusFromEnv(env)
	if ready.Ready || ready.ProviderMatrix.Status != "failed" || !strings.Contains(ready.ProviderMatrix.Message, "secret material") {
		t.Fatalf("header-style secret evidence must fail strict G6 readiness: %#v", ready)
	}
}

func runtimeCredentialedReadinessEnvWithEvidence(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	provider := filepath.Join(dir, "provider-matrix.json")
	mcp := filepath.Join(dir, "mcp-matrix.json")
	packaged := filepath.Join(dir, "packaged-qa.json")
	operator := filepath.Join(dir, "operator-gate.json")
	writeEvidenceFile(t, provider, `{
		"schemaVersion": 1,
		"credentialedProbes": [
			{ "id": "deepseek", "status": "passed", "skipped": false, "credentialed": true, "requestUrl": "https://deepseek.example/v1/chat/completions" },
			{ "id": "openai-compatible", "status": "passed", "skipped": false, "credentialed": true, "requestUrl": "https://openai.example/v1/chat/completions" },
			{ "id": "anthropic-compatible", "status": "passed", "skipped": false, "credentialed": true, "requestUrl": "https://anthropic.example/v1/messages" },
			{ "id": "custom-endpoint", "status": "passed", "skipped": false, "credentialed": true, "requestUrl": "https://custom.example/messages" }
		]
	}`)
	writeEvidenceFile(t, mcp, `{
		"schemaVersion": 1,
		"topLevelMcpIndexerExposed": false,
		"reasonixPublicProtocolUsed": false,
		"credentialedExecution": true,
		"connect": true,
		"toolDiscoverySearch": true,
		"toolCall": true,
		"approvalUserInput": true,
		"reconnect": true,
		"redaction": true,
		"credentialedProbes": [
			{
				"id": "credentialed-mcp",
				"status": "passed",
				"skipped": false,
				"credentialed": true,
				"credentialedExecution": true,
				"connect": true,
				"search": true,
				"toolCall": true,
				"approvalUserInput": true,
				"reconnect": true,
				"credentialRedaction": true
			}
		]
	}`)
	writeEvidenceFile(t, packaged, `{
		"schemaVersion": 1,
		"id": "runtime-packaged-qa",
		"changeId": "runtime-go-default",
		"stage": "packaged-desktop-qa",
		"upgradesChangeId": "runtime-go-default",
		"status": "passed",
		"passed": true,
		"goDefaultBackendEnabled": true,
		"rendererVisibleGoSwitcher": false,
		"typeScriptFallbackRetained": false,
		"requiredCheckIds": [
			"packaged-app-startup",
			"go-runtime-default-gate",
			"health",
			"runtime-info",
			"thread-list",
			"turn-create",
			"sse-replay",
			"typescript-retired-backend"
		],
		"checks": [
			{ "id": "packaged-app-startup", "status": "passed" },
			{ "id": "go-runtime-default-gate", "status": "passed" },
			{ "id": "health", "status": "passed" },
			{ "id": "runtime-info", "status": "passed" },
			{ "id": "thread-list", "status": "passed" },
			{ "id": "turn-create", "status": "passed" },
			{ "id": "sse-replay", "status": "passed" },
			{ "id": "typescript-retired-backend", "status": "passed" }
		],
		"startup": {
			"appPathExists": true,
			"launchCommandConfigured": true,
			"launchCommandReferencesAppPath": true,
			"launchCommandHash": "0000000000000000000000000000000000000000000000000000000000000000",
			"exitCode": 0
		},
		"runtime": {
			"runtimeUrl": "http://127.0.0.1:12345",
			"runtimeTokenConfigured": true,
			"healthOK": true,
			"threadIdHash": "1111111111111111111111111111111111111111111111111111111111111111",
			"turnIdHash": "2222222222222222222222222222222222222222222222222222222222222222"
		},
		"defaultRuntimeGate": {
			"`+defaultRuntimeGateBackendField+`": "go-runtime-default",
			"explicitRuntimeBackendOverrideEnvSet": false,
			"goDefaultBackendEnabled": true
		},
			"retiredBackendEvidence": {
				"requestedBackend": "typescript",
				"code": "retired_backend",
				"activeBackendAfterRequest": "go-runtime-default",
				"defaultRuntimeStopped": false,
				"tsStarted": false,
				"runtimeHealthAfterRequest": true,
				"defaultRuntimeLaunchCommandHash": "0000000000000000000000000000000000000000000000000000000000000000",
				"preRequestRuntimeUrl": "http://127.0.0.1:12345",
				"postRequestRuntimeUrl": "http://127.0.0.1:12345",
				"verifiedAt": "2026-06-24T00:00:00.000Z",
				"credentialSecretsRecorded": false,
				"redaction": { "status": "passed", "secretMaterialFound": false }
			}
		}`)
	writeEvidenceJSON(t, operator, map[string]any{
		"schemaVersion":                1,
		"id":                           "runtime-operator-gate",
		"status":                       "passed",
		"passed":                       true,
		"operatorGate":                 RuntimeReadyEnv + "=1",
		"explicitEnvGate":              true,
		"credentialedEvidenceReviewed": true,
		"defaultRuntimeApproved":       true,
		"goDefaultApproved":            true,
		"typeScriptFallbackRetained":   false,
		"goDefaultBackendEnabled":      true,
		"reasonixEngineRuntimeOnly":    true,
		"evidenceTargetCommit":         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"evidenceDigestAlgorithm":      "sha256:canonical-json-v1",
		"evidenceDigests": map[string]any{
			"provider": evidenceDigestFromFile(t, provider),
			"mcp":      evidenceDigestFromFile(t, mcp),
			"packaged": evidenceDigestFromFile(t, packaged),
		},
		"reportPaths": map[string]any{
			"provider": provider,
			"mcp":      mcp,
			"packaged": packaged,
		},
		"evidenceReviewed": []map[string]any{
			{"id": "provider-matrix-credentialed", "status": "passed", "path": provider},
			{"id": "mcp-matrix-credentialed", "status": "passed", "path": mcp},
			{"id": "packaged-qa", "status": "passed", "path": packaged},
		},
	})
	return map[string]string{
		RuntimeReadyEnv:                              "1",
		RuntimeReadyEnv + "_EVIDENCE":                operator,
		RuntimeDurableRestartEvidenceEnv:             "passed",
		RuntimeProviderMatrixEnv:                     "passed",
		RuntimeProviderMatrixEnv + "_EVIDENCE":       provider,
		RuntimeMCPMatrixEnv:                          "passed",
		RuntimeMCPMatrixEnv + "_EVIDENCE":            mcp,
		RuntimePackagedQAEnv:                         "passed",
		RuntimePackagedQAEnv + "_EVIDENCE":           packaged,
		"ANALYTIX_RUNTIME_DEEPSEEK_API_KEY":          "key",
		"ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL":         "https://deepseek.invalid/v1",
		"ANALYTIX_RUNTIME_DEEPSEEK_MODEL":            "deepseek-chat",
		"ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY":     "key",
		"ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL":    "https://openai.invalid/v1",
		"ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL":       "gpt-compatible",
		"ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY":  "key",
		"ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL": "https://anthropic.invalid",
		"ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL":    "claude-compatible",
		"ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY":   "key",
		"ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL":       "https://custom.invalid/full/path",
		"ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL":     "custom-compatible",
		RuntimeMCPCommandEnv:                         "node fixture-mcp.js",
	}
}

func writeEvidenceFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write evidence file %s: %v", path, err)
	}
}

func writeEvidenceJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal evidence file %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write evidence file %s: %v", path, err)
	}
}

func evidenceDigestFromFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evidence file %s: %v", path, err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("parse evidence file %s: %v", path, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("canonicalize evidence file %s: %v", path, err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
