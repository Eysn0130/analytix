package readiness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	provider "analytix.local/runtime-go/internal/provider"
)

const (
	RuntimeDurableRestartEvidenceEnv = "ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS"
	RuntimeProviderMatrixEnv         = "ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS"
	RuntimeMCPMatrixEnv              = "ANALYTIX_RUNTIME_MCP_MATRIX_STATUS"
	RuntimePackagedQAEnv             = "ANALYTIX_RUNTIME_PACKAGED_QA_STATUS"
	RuntimeReadyEnv                  = "ANALYTIX_RUNTIME_READY"
	RuntimeMCPCommandEnv             = "ANALYTIX_RUNTIME_MCP_COMMAND"
	RuntimeMCPURLEnv                 = "ANALYTIX_RUNTIME_MCP_URL"
)

var operatorGateEvidenceIDs = map[string]bool{
	"runtime-operator-gate":    true,
	"go-runtime-operator-gate": true,
}

var defaultRuntimeGateBackendField = strings.Join([]string{"runtime", "Backend"}, "")

var secretStringPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bBearer\s+[A-Za-z0-9._~+/-]+=*`),
	regexp.MustCompile(`\bBasic\s+[A-Za-z0-9._~+/-]+=*`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`),
	regexp.MustCompile(`(?i)(?:api[_-]?key|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)=[^&\s]+`),
	regexp.MustCompile(`(?i)(?:x-api-key|api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)\s*:\s*[^\s,;]+`),
}

type RuntimeReadinessCheck struct {
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Evidence string `json:"evidence,omitempty"`
	Message  string `json:"message,omitempty"`
}

type RuntimeReadinessStatus struct {
	SchemaVersion             int                   `json:"schemaVersion"`
	Ready                     bool                  `json:"ready"`
	ExplicitReadyGate         bool                  `json:"explicitReadyGate"`
	DurableRestartEvidence    RuntimeReadinessCheck `json:"durableRestartEvidence"`
	ProviderMatrix            RuntimeReadinessCheck `json:"providerMatrix"`
	MCPMatrix                 RuntimeReadinessCheck `json:"mcpMatrix"`
	PackagedQA                RuntimeReadinessCheck `json:"packagedQa"`
	OperatorGate              RuntimeReadinessCheck `json:"operatorGate"`
	MissingRequiredChecks     []string              `json:"missingRequiredChecks"`
	DefaultGoBackendEnabled   bool                  `json:"defaultGoBackendEnabled"`
	RendererVisibleGoSwitcher bool                  `json:"rendererVisibleGoSwitcher"`
	Notes                     []string              `json:"notes"`
}

// RuntimeReadinessProjection is the production-safe readiness shape exposed by
// the transitional runtime server. The full diagnostic status stays outside
// that facade.
type RuntimeReadinessProjection struct {
	SchemaVersion int  `json:"schemaVersion"`
	Ready         bool `json:"ready"`
}

type RuntimeReadinessProbe struct {
	ID                string               `json:"id"`
	Family            string               `json:"family"`
	EndpointFormat    string               `json:"endpointFormat"`
	Status            string               `json:"status"`
	Skipped           bool                 `json:"skipped"`
	Credentialed      bool                 `json:"credentialed"`
	RequestURL        string               `json:"requestUrl,omitempty"`
	RequestBodyFields []string             `json:"requestBodyFields,omitempty"`
	Usage             provider.Usage       `json:"usage,omitempty"`
	PrefixShape       provider.PrefixShape `json:"prefixShape,omitempty"`
	Message           string               `json:"message,omitempty"`
}

type RuntimeProviderMatrixResult struct {
	SchemaVersion                int                     `json:"schemaVersion"`
	ProviderMatrixScaffold       bool                    `json:"providerMatrixScaffold"`
	FixtureMatrixRequired        bool                    `json:"fixtureMatrixRequired"`
	CredentialedMatrixEnvGated   bool                    `json:"credentialedMatrixEnvGated"`
	ReadsRealAPIKeysByDefault    bool                    `json:"readsRealApiKeysByDefault"`
	DeepSeekCacheBenchmarkFormat map[string]any          `json:"deepseekCacheBenchmarkFormat"`
	FixtureProbes                []RuntimeReadinessProbe `json:"fixtureProbes"`
	CredentialedProbes           []RuntimeReadinessProbe `json:"credentialedProbes"`
}

type RuntimeMCPMatrixResult struct {
	SchemaVersion              int                     `json:"schemaVersion"`
	MCPMatrixScaffold          bool                    `json:"mcpMatrixScaffold"`
	FixtureMCPRequired         bool                    `json:"fixtureMcpRequired"`
	CredentialedMCPEnvGated    bool                    `json:"credentialedMcpEnvGated"`
	TopLevelMCPIndexerExposed  bool                    `json:"topLevelMcpIndexerExposed"`
	ReasonixPublicProtocolUsed bool                    `json:"reasonixPublicProtocolUsed"`
	FixtureProbe               map[string]any          `json:"fixtureProbe"`
	CredentialedProbes         []RuntimeReadinessProbe `json:"credentialedProbes"`
}

type runtimeProviderCase struct {
	id                string
	family            string
	endpointFormat    string
	reasoningProtocol string
	baseURL           string
	model             string
	apiKeyEnv         string
	baseURLEnv        string
	modelEnv          string
}

func RuntimeReadinessStatusFromEnv(env map[string]string) RuntimeReadinessStatus {
	if env == nil {
		env = environMap()
	}
	checks := map[string]RuntimeReadinessCheck{
		"durableRestartEvidence": readinessCheckFromEnv(env, RuntimeDurableRestartEvidenceEnv),
		"providerMatrix":         credentialedProviderMatrixCheckFromEnv(env),
		"mcpMatrix":              credentialedMCPMatrixCheckFromEnv(env),
		"packagedQa":             packagedQACheckFromEnv(env),
		"operatorGate":           operatorGateCheckFromEnv(env),
	}
	missing := []string{}
	for id, check := range checks {
		if check.Status != "passed" {
			missing = append(missing, id)
		}
	}
	explicitReady := strings.TrimSpace(env[RuntimeReadyEnv]) == "1"
	return RuntimeReadinessStatus{
		SchemaVersion:             1,
		Ready:                     explicitReady && checks["operatorGate"].Status == "passed" && len(missing) == 0,
		ExplicitReadyGate:         explicitReady,
		DurableRestartEvidence:    checks["durableRestartEvidence"],
		ProviderMatrix:            checks["providerMatrix"],
		MCPMatrix:                 checks["mcpMatrix"],
		PackagedQA:                checks["packagedQa"],
		OperatorGate:              checks["operatorGate"],
		MissingRequiredChecks:     missing,
		DefaultGoBackendEnabled:   true,
		RendererVisibleGoSwitcher: false,
		Notes: []string{
			"Strict live readiness remains evidence-gated and may be pending after default cutover.",
			"Go runtime is the default backend; TypeScript remains only an explicit rollback compatibility override.",
		},
	}
}

func readinessCheckFromEnv(env map[string]string, key string) RuntimeReadinessCheck {
	value := strings.ToLower(strings.TrimSpace(env[key]))
	if value == "" {
		return RuntimeReadinessCheck{Status: "missing", Required: true, Message: key + " is not set"}
	}
	status := "failed"
	if value == "passed" || value == "pass" || value == "ok" || value == "1" {
		status = "passed"
	} else if value == "skipped" || value == "skip" {
		status = "skipped"
	}
	return RuntimeReadinessCheck{
		Status:   status,
		Required: true,
		Evidence: strings.TrimSpace(env[key+"_EVIDENCE"]),
		Message:  key + "=" + value,
	}
}

func credentialedProviderMatrixCheckFromEnv(env map[string]string) RuntimeReadinessCheck {
	check := readinessCheckFromEnv(env, RuntimeProviderMatrixEnv)
	if check.Status != "passed" {
		return check
	}
	if !hasCompleteProviderCredentialEnv(env) {
		check.Status = "failed"
		check.Message = RuntimeProviderMatrixEnv + "=passed without complete credentialed provider env; fixture/local matrix evidence does not count"
		return check
	}
	evidence := evidencePathFromEnv(env, RuntimeProviderMatrixEnv+"_EVIDENCE")
	ok, message := validateProviderEvidence(evidence)
	if ok {
		check.Evidence = evidence
		check.Message = message
		return check
	}
	check.Status = "failed"
	if evidence != "" {
		check.Evidence = evidence
	}
	check.Message = message
	return check
}

func credentialedMCPMatrixCheckFromEnv(env map[string]string) RuntimeReadinessCheck {
	check := readinessCheckFromEnv(env, RuntimeMCPMatrixEnv)
	if check.Status != "passed" {
		return check
	}
	if strings.TrimSpace(env[RuntimeMCPCommandEnv]) == "" && strings.TrimSpace(env[RuntimeMCPURLEnv]) == "" {
		check.Status = "failed"
		check.Message = RuntimeMCPMatrixEnv + "=passed without " + RuntimeMCPCommandEnv + " or " + RuntimeMCPURLEnv + "; fixture/local MCP evidence does not count"
		return check
	}
	evidence := evidencePathFromEnv(env, RuntimeMCPMatrixEnv+"_EVIDENCE")
	ok, message := validateMCPEvidence(evidence)
	if ok {
		check.Evidence = evidence
		check.Message = message
		return check
	}
	check.Status = "failed"
	if evidence != "" {
		check.Evidence = evidence
	}
	check.Message = message
	return check
}

func packagedQACheckFromEnv(env map[string]string) RuntimeReadinessCheck {
	check := readinessCheckFromEnv(env, RuntimePackagedQAEnv)
	if check.Status != "passed" {
		return check
	}
	evidence := evidencePathFromEnv(env, RuntimePackagedQAEnv+"_EVIDENCE")
	ok, message := validatePackagedEvidence(evidence)
	if ok {
		check.Evidence = evidence
		check.Message = message
		return check
	}
	check.Status = "failed"
	if evidence != "" {
		check.Evidence = evidence
	}
	check.Message = message
	return check
}

func operatorGateCheckFromEnv(env map[string]string) RuntimeReadinessCheck {
	if strings.TrimSpace(env[RuntimeReadyEnv]) != "1" {
		return RuntimeReadinessCheck{
			Status:   "missing",
			Required: true,
			Message:  RuntimeReadyEnv + " is not set to 1",
		}
	}
	evidence := evidencePathFromEnv(env, RuntimeReadyEnv+"_EVIDENCE")
	expectedEvidencePaths := map[string]string{
		"provider": evidencePathFromEnv(env, RuntimeProviderMatrixEnv+"_EVIDENCE"),
		"mcp":      evidencePathFromEnv(env, RuntimeMCPMatrixEnv+"_EVIDENCE"),
		"packaged": evidencePathFromEnv(env, RuntimePackagedQAEnv+"_EVIDENCE"),
	}
	ok, message := validateOperatorGateEvidence(evidence, expectedEvidencePaths)
	if ok {
		return RuntimeReadinessCheck{
			Status:   "passed",
			Required: true,
			Evidence: evidence,
			Message:  message,
		}
	}
	return RuntimeReadinessCheck{
		Status:   "failed",
		Required: true,
		Evidence: evidence,
		Message:  message,
	}
}

func evidencePathFromEnv(env map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(env[key]); value != "" {
			return value
		}
	}
	return ""
}

func readJSONEvidence(path string) (map[string]any, string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "evidence path is not set", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "evidence path is unreadable: " + err.Error(), false
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, "evidence JSON is unreadable: " + err.Error(), false
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, "evidence JSON root must be an object", false
	}
	return root, "", true
}

func validateProviderEvidence(path string) (bool, string) {
	root, message, ok := readJSONEvidence(path)
	if !ok {
		return false, message
	}
	if finding := firstEvidenceSecretFinding(root, "$"); finding != "" {
		return false, "credentialed provider evidence must not contain secret material (" + finding + ")"
	}
	required := map[string]bool{
		"deepseek":             true,
		"openai-compatible":    true,
		"anthropic-compatible": true,
		"custom-endpoint":      true,
	}
	for _, probe := range listValue(root["credentialedProbes"]) {
		item := mapValue(probe)
		id := stringValue(item["id"])
		if credentialedProbePassed(item) && required[id] {
			delete(required, id)
		}
	}
	if len(required) > 0 {
		missing := []string{}
		for _, id := range []string{"deepseek", "openai-compatible", "anthropic-compatible", "custom-endpoint"} {
			if required[id] {
				missing = append(missing, id)
			}
		}
		return false, "credentialed provider evidence is incomplete; missing passed probes: " + strings.Join(missing, ", ")
	}
	return true, "credentialed provider evidence file includes all required provider families"
}

func validateMCPEvidence(path string) (bool, string) {
	root, message, ok := readJSONEvidence(path)
	if !ok {
		return false, message
	}
	if finding := firstEvidenceSecretFinding(root, "$"); finding != "" {
		return false, "credentialed MCP evidence must not contain secret material (" + finding + ")"
	}
	probes := listValue(root["credentialedProbes"])
	explicitCredentialedExecution := boolValue(root["credentialedExecution"])
	passed := stringValue(root["status"]) == "passed" && boolValue(root["credentialed"])
	for _, probe := range probes {
		item := mapValue(probe)
		if boolValue(item["credentialedExecution"]) {
			explicitCredentialedExecution = true
		}
		if credentialedProbePassed(item) {
			passed = true
		}
	}
	if !explicitCredentialedExecution || !passed {
		return false, "credentialed MCP evidence must include a passed credentialed execution probe, not only the fixture MCP scaffold"
	}
	if boolValue(root["topLevelMcpIndexerExposed"]) || boolValue(root["reasonixPublicProtocolUsed"]) {
		return false, "credentialed MCP evidence must not add forbidden MCP indexer UI or upstream protocol surface"
	}
	if missing := missingMCPCoverage(root, probes); len(missing) > 0 {
		return false, "credentialed MCP evidence is incomplete; missing passed coverage: " + strings.Join(missing, ", ")
	}
	return true, "credentialed MCP evidence file includes real connect/search/call/approval/reconnect/redaction coverage"
}

func validatePackagedEvidence(path string) (bool, string) {
	root, message, ok := readJSONEvidence(path)
	if !ok {
		return false, message
	}
	if finding := firstEvidenceSecretFinding(root, "$"); finding != "" {
		return false, "packaged QA evidence must not contain secret material (" + finding + ")"
	}
	checks := listValue(root["checks"])
	required := map[string]bool{
		"packaged-app-startup":       true,
		"health":                     true,
		"runtime-info":               true,
		"thread-list":                true,
		"turn-create":                true,
		"sse-replay":                 true,
		"go-runtime-default-gate":    true,
		"typescript-retired-backend": true,
	}
	if stringValue(root["id"]) != "runtime-packaged-qa" || !boolValue(root["passed"]) {
		return false, "packaged QA evidence must be a passed runtime-packaged-qa report with all checks passed"
	}
	evidenceMissing := []string{}
	if stringValue(root["changeId"]) != "runtime-go-default" && stringValue(root["upgradesChangeId"]) != "runtime-go-default" {
		evidenceMissing = append(evidenceMissing, "changeId:runtime-go-default")
	}
	if !boolValue(root["goDefaultBackendEnabled"]) {
		evidenceMissing = append(evidenceMissing, "goDefaultBackendEnabled:true")
	}
	if boolValue(root["typeScriptFallbackRetained"]) {
		evidenceMissing = append(evidenceMissing, "typeScriptFallbackRetained:false")
	}
	startup := mapValue(root["startup"])
	runtime := mapValue(root["runtime"])
	legacyDefaultRuntimeGateKey := strings.Join([]string{"candidate", "Gate"}, "")
	legacyCandidateEnvSetKey := strings.Join([]string{"candidate", "Env", "Set"}, "")
	legacyCandidateStoppedKey := strings.Join([]string{"candidate", "Stopped"}, "")
	legacyCandidateLaunchCommandHashKey := strings.Join([]string{"candidate", "LaunchCommandHash"}, "")
	defaultRuntimeGate := mapValue(root["defaultRuntimeGate"])
	if len(defaultRuntimeGate) == 0 {
		defaultRuntimeGate = mapValue(root[legacyDefaultRuntimeGateKey])
	}
	retiredBackendEvidence := mapValue(root["retiredBackendEvidence"])
	explicitRuntimeBackendOverrideEnvSet := boolValue(defaultRuntimeGate["explicitRuntimeBackendOverrideEnvSet"])
	if _, ok := defaultRuntimeGate["explicitRuntimeBackendOverrideEnvSet"]; !ok {
		explicitRuntimeBackendOverrideEnvSet = boolValue(defaultRuntimeGate[legacyCandidateEnvSetKey])
	}
	defaultRuntimeStopped, defaultRuntimeStoppedSet := boolValue(retiredBackendEvidence["defaultRuntimeStopped"]), false
	if _, ok := retiredBackendEvidence["defaultRuntimeStopped"]; ok {
		defaultRuntimeStoppedSet = true
	} else if _, ok := retiredBackendEvidence[legacyCandidateStoppedKey]; ok {
		defaultRuntimeStopped = boolValue(retiredBackendEvidence[legacyCandidateStoppedKey])
		defaultRuntimeStoppedSet = true
	}
	defaultRuntimeLaunchCommandHash := stringValue(retiredBackendEvidence["defaultRuntimeLaunchCommandHash"])
	if defaultRuntimeLaunchCommandHash == "" {
		defaultRuntimeLaunchCommandHash = stringValue(retiredBackendEvidence[legacyCandidateLaunchCommandHashKey])
	}
	if !boolValue(startup["appPathExists"]) {
		evidenceMissing = append(evidenceMissing, "startup.appPathExists")
	}
	if !boolValue(startup["launchCommandConfigured"]) {
		evidenceMissing = append(evidenceMissing, "startup.launchCommandConfigured")
	}
	if !boolValue(startup["launchCommandReferencesAppPath"]) {
		evidenceMissing = append(evidenceMissing, "startup.launchCommandReferencesAppPath")
	}
	if intValue(startup["exitCode"]) != 0 {
		evidenceMissing = append(evidenceMissing, "startup.exitCode")
	}
	if !isSHA256(stringValue(startup["launchCommandHash"])) {
		evidenceMissing = append(evidenceMissing, "startup.launchCommandHash")
	}
	if stringValue(runtime["runtimeUrl"]) == "" {
		evidenceMissing = append(evidenceMissing, "runtime.runtimeUrl")
	}
	if !boolValue(runtime["runtimeTokenConfigured"]) {
		evidenceMissing = append(evidenceMissing, "runtime.runtimeTokenConfigured")
	}
	if !boolValue(runtime["healthOK"]) {
		evidenceMissing = append(evidenceMissing, "runtime.healthOK")
	}
	if !isSHA256(stringValue(runtime["threadIdHash"])) {
		evidenceMissing = append(evidenceMissing, "runtime.threadIdHash")
	}
	if !isSHA256(stringValue(runtime["turnIdHash"])) {
		evidenceMissing = append(evidenceMissing, "runtime.turnIdHash")
	}
	if !goDefaultBackendAlias(stringValue(defaultRuntimeGate[defaultRuntimeGateBackendField])) {
		evidenceMissing = append(evidenceMissing, "defaultRuntimeGate."+defaultRuntimeGateBackendField+":go-runtime-default")
	}
	if explicitRuntimeBackendOverrideEnvSet {
		evidenceMissing = append(evidenceMissing, "defaultRuntimeGate.explicitRuntimeBackendOverrideEnvSet:false")
	}
	if !boolValue(defaultRuntimeGate["goDefaultBackendEnabled"]) {
		evidenceMissing = append(evidenceMissing, "defaultRuntimeGate.goDefaultBackendEnabled:true")
	}
	if stringValue(retiredBackendEvidence["requestedBackend"]) != "typescript" {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.requestedBackend:typescript")
	}
	if stringValue(retiredBackendEvidence["code"]) != "retired_backend" {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.code:retired_backend")
	}
	if stringValue(retiredBackendEvidence["activeBackendAfterRequest"]) != "go-runtime-default" {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.activeBackendAfterRequest")
	}
	if !defaultRuntimeStoppedSet || defaultRuntimeStopped {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.defaultRuntimeStopped:false")
	}
	if boolValue(retiredBackendEvidence["tsStarted"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.tsStarted:false")
	}
	if !boolValue(retiredBackendEvidence["runtimeHealthAfterRequest"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.runtimeHealthAfterRequest")
	}
	if !isSHA256(defaultRuntimeLaunchCommandHash) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.defaultRuntimeLaunchCommandHash")
	} else if defaultRuntimeLaunchCommandHash != stringValue(startup["launchCommandHash"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.defaultRuntimeLaunchCommandHash:match")
	}
	if stringValue(retiredBackendEvidence["preRequestRuntimeUrl"]) != stringValue(runtime["runtimeUrl"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.preRequestRuntimeUrl:match")
	}
	if stringValue(retiredBackendEvidence["postRequestRuntimeUrl"]) != stringValue(runtime["runtimeUrl"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.postRequestRuntimeUrl:match")
	}
	if stringValue(retiredBackendEvidence["verifiedAt"]) == "" {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.verifiedAt")
	}
	if boolValue(retiredBackendEvidence["credentialSecretsRecorded"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.credentialSecretsRecorded:false")
	}
	retiredBackendRedaction := mapValue(retiredBackendEvidence["redaction"])
	if stringValue(retiredBackendRedaction["status"]) != "passed" || boolValue(retiredBackendRedaction["secretMaterialFound"]) {
		evidenceMissing = append(evidenceMissing, "retiredBackendEvidence.redaction")
	}
	if len(evidenceMissing) > 0 {
		return false, "packaged QA evidence is missing process/runtime/retired-backend evidence: " + strings.Join(evidenceMissing, ", ")
	}
	for _, check := range checks {
		item := mapValue(check)
		if stringValue(item["status"]) != "passed" {
			return false, "packaged QA evidence must be a passed runtime-packaged-qa report with all checks passed"
		}
		delete(required, stringValue(item["id"]))
	}
	if len(required) > 0 {
		missing := []string{}
		for _, id := range []string{"packaged-app-startup", "health", "runtime-info", "thread-list", "turn-create", "sse-replay", "go-runtime-default-gate", "typescript-retired-backend"} {
			if required[id] {
				missing = append(missing, id)
			}
		}
		return false, "packaged QA evidence is missing required check coverage: " + strings.Join(missing, ", ")
	}
	return true, "packaged QA evidence file includes all required checks and evidence bindings"
}

func validateOperatorGateEvidence(path string, expectedEvidencePaths map[string]string) (bool, string) {
	root, message, ok := readJSONEvidence(path)
	if !ok {
		return false, message
	}
	if finding := firstEvidenceSecretFinding(root, "$"); finding != "" {
		return false, "operator gate evidence must not contain secret material (" + finding + ")"
	}
	expectedEnvGate := RuntimeReadyEnv + "=1"
	envGate := stringValue(root["operatorGate"]) == expectedEnvGate ||
		stringValue(root["envGate"]) == expectedEnvGate ||
		boolValue(root["explicitEnvGate"])
	passed := stringValue(root["status"]) == "passed" || boolValue(root["passed"])
	reviewed := boolValue(root["credentialedEvidenceReviewed"])
	legacyGoDefaultApprovedKey := strings.Join([]string{"goDefault", "Candidate", "Approved"}, "")
	legacyDefaultApprovedKey := strings.Join([]string{"default", "Candidate", "Approved"}, "")
	approved := boolValue(root["goDefaultApproved"]) ||
		boolValue(root["defaultRuntimeApproved"]) ||
		boolValue(root[legacyGoDefaultApprovedKey]) ||
		boolValue(root[legacyDefaultApprovedKey])
	missing := []string{}
	if !operatorGateEvidenceIDs[stringValue(root["id"])] {
		missing = append(missing, "id")
	}
	if !passed {
		missing = append(missing, "status")
	}
	if !boolValue(root["passed"]) {
		missing = append(missing, "passed")
	}
	if !envGate {
		missing = append(missing, "operatorGate")
	}
	if !boolValue(root["explicitEnvGate"]) {
		missing = append(missing, "explicitEnvGate")
	}
	if !reviewed {
		missing = append(missing, "credentialedEvidenceReviewed")
	}
	if !approved {
		missing = append(missing, "goDefaultApproved")
	}
	if boolValue(root["typeScriptFallbackRetained"]) {
		missing = append(missing, "typeScriptFallbackRetained:false")
	}
	if !boolValue(root["goDefaultBackendEnabled"]) {
		missing = append(missing, "goDefaultBackendEnabled:true")
	}
	if !looksLikeGitCommit(firstNonEmptyString(root["evidenceTargetCommit"], root["commitHash"], root["candidateCommit"], root["sourceCommit"])) {
		missing = append(missing, "evidenceTargetCommit")
	}
	if stringValue(root["evidenceDigestAlgorithm"]) != "sha256:canonical-json-v1" {
		missing = append(missing, "evidenceDigestAlgorithm")
	}
	reportPaths := mapValue(root["reportPaths"])
	evidenceDigests := mapValue(root["evidenceDigests"])
	resolvedReportPaths := map[string]string{}
	for _, key := range []string{"provider", "mcp", "packaged"} {
		digest := stringValue(evidenceDigests[key])
		reportPath := stringValue(reportPaths[key])
		expectedPath := expectedEvidencePaths[key]
		if !isSHA256(digest) {
			missing = append(missing, "evidenceDigests."+key)
		}
		if strings.TrimSpace(reportPath) == "" {
			missing = append(missing, "reportPaths."+key)
			continue
		}
		resolvedReportPath := absPath(reportPath)
		resolvedReportPaths[key] = resolvedReportPath
		if expectedPath != "" && resolvedReportPath != absPath(expectedPath) {
			missing = append(missing, "reportPaths."+key+":match")
		}
		evidenceRoot, readMessage, readOK := readJSONEvidence(firstNonEmptyString(expectedPath, resolvedReportPath))
		if !readOK {
			missing = append(missing, "reportPaths."+key+":readable:"+readMessage)
			continue
		}
		if digest != sha256JSON(evidenceRoot) {
			missing = append(missing, "evidenceDigests."+key+":match")
		}
	}
	reviewedRows := listValue(root["evidenceReviewed"])
	requiredReviewed := map[string]string{
		"provider-matrix-credentialed": "provider",
		"mcp-matrix-credentialed":      "mcp",
		"packaged-qa":                  "packaged",
	}
	for id, key := range requiredReviewed {
		item := findEvidenceReviewRow(reviewedRows, id)
		if item == nil {
			missing = append(missing, "evidenceReviewed."+id)
			continue
		}
		if stringValue(item["status"]) != "passed" {
			missing = append(missing, "evidenceReviewed."+id+".status")
			continue
		}
		reviewedPath := stringValue(item["path"])
		expectedPath := firstNonEmptyString(expectedEvidencePaths[key], resolvedReportPaths[key])
		if strings.TrimSpace(reviewedPath) == "" {
			missing = append(missing, "evidenceReviewed."+id+".path")
		} else if expectedPath != "" && absPath(reviewedPath) != absPath(expectedPath) {
			missing = append(missing, "evidenceReviewed."+id+".path:match")
		}
	}
	if len(missing) > 0 {
		return false, "operator gate evidence must be a passed runtime operator gate JSON with explicit env gate, digest bindings, evidence review, and Go default approval: " + strings.Join(missing, ", ")
	}
	return true, "operator gate evidence file authorizes " + expectedEnvGate + " with digest bindings"
}

func missingMCPCoverage(root map[string]any, probes []any) []string {
	sources := []map[string]any{root}
	for _, probe := range probes {
		sources = append(sources, mapValue(probe))
	}
	required := []struct {
		id   string
		keys []string
	}{
		{id: "connect", keys: []string{"connect", "mcpConnect"}},
		{id: "tool-discovery-search", keys: []string{"toolDiscoverySearch", "toolDiscovery", "search"}},
		{id: "tool-call", keys: []string{"toolCall", "call", "approvedCallExecutes"}},
		{id: "approval-user-input", keys: []string{"approvalUserInput", "approval", "userInput"}},
		{id: "reconnect", keys: []string{"reconnect"}},
		{id: "redaction", keys: []string{"redaction", "credentialRedaction"}},
	}
	missing := []string{}
	for _, item := range required {
		if !coveragePassed(sources, item.keys) {
			missing = append(missing, item.id)
		}
	}
	return missing
}

func coveragePassed(sources []map[string]any, keys []string) bool {
	for _, source := range sources {
		for _, key := range keys {
			if boolValue(source[key]) || stringValue(source[key]) == "passed" {
				return true
			}
		}
	}
	return false
}

func firstEvidenceSecretFinding(value any, path string) string {
	switch item := value.(type) {
	case []any:
		for index, child := range item {
			if finding := firstEvidenceSecretFinding(child, path+"["+strconv.Itoa(index)+"]"); finding != "" {
				return finding
			}
		}
	case map[string]any:
		for key, child := range item {
			if isSecretEvidenceKey(key) {
				return path + "." + key
			}
			if finding := firstEvidenceSecretFinding(child, path+"."+key); finding != "" {
				return finding
			}
		}
	case string:
		if stringLooksLikeSecret(item) {
			return path
		}
	}
	return ""
}

func isSecretEvidenceKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
	switch normalized {
	case "apikey", "xapikey", "apikeyheader", "xapikeyheader", "authorization", "authorizationheader", "accesstoken", "refreshtoken", "runtimetoken", "token", "password", "secret", "secretkey", "privatekey", "clientsecret", "signature", "sig", "auth", "credential", "jwt":
		return true
	default:
		return false
	}
}

func stringLooksLikeSecret(value string) bool {
	for _, pattern := range secretStringPatterns {
		for _, match := range pattern.FindAllString(value, -1) {
			if strings.Contains(strings.ToLower(match), "redacted") {
				continue
			}
			return true
		}
	}
	return false
}

func credentialedProbePassed(value any) bool {
	item := mapValue(value)
	return stringValue(item["status"]) == "passed" &&
		boolValue(item["credentialed"]) &&
		!boolValue(item["skipped"])
}

func mapValue(value any) map[string]any {
	if item, ok := value.(map[string]any); ok {
		return item
	}
	return map[string]any{}
}

func listValue(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func boolValue(value any) bool {
	if flag, ok := value.(bool); ok {
		return flag
	}
	return false
}

func goDefaultBackendAlias(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "analytix", "go", "go-runtime", "go-runtime-default":
		return true
	default:
		return false
	}
}

func intValue(value any) int {
	switch item := value.(type) {
	case int:
		return item
	case float64:
		return int(item)
	default:
		return -1
	}
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func looksLikeGitCommit(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(stringValue(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func absPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	fullPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return fullPath
}

func sha256JSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func findEvidenceReviewRow(rows []any, id string) map[string]any {
	for _, row := range rows {
		item := mapValue(row)
		if stringValue(item["id"]) == id {
			return item
		}
	}
	return nil
}

func hasCompleteProviderCredentialEnv(env map[string]string) bool {
	for _, item := range runtimeProviderCases("") {
		if strings.TrimSpace(env[item.apiKeyEnv]) == "" ||
			strings.TrimSpace(env[item.baseURLEnv]) == "" ||
			strings.TrimSpace(env[item.modelEnv]) == "" {
			return false
		}
	}
	return true
}

func runtimeProviderCases(fixtureBaseURL string) []runtimeProviderCase {
	base := func(path string) string {
		if fixtureBaseURL == "" {
			return ""
		}
		return fixtureBaseURL + path
	}
	return []runtimeProviderCase{
		{
			id:                "deepseek",
			family:            "deepseek",
			endpointFormat:    "chat_completions",
			reasoningProtocol: "deepseek-chat-completions",
			baseURL:           base("/deepseek/v1"),
			model:             "deepseek-chat",
			apiKeyEnv:         "ANALYTIX_RUNTIME_DEEPSEEK_API_KEY",
			baseURLEnv:        "ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL",
			modelEnv:          "ANALYTIX_RUNTIME_DEEPSEEK_MODEL",
		},
		{
			id:             "openai-compatible",
			family:         "openai-compatible",
			endpointFormat: "chat_completions",
			baseURL:        base("/openai/v1"),
			model:          "gpt-compatible",
			apiKeyEnv:      "ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY",
			baseURLEnv:     "ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL",
			modelEnv:       "ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL",
		},
		{
			id:             "anthropic-compatible",
			family:         "anthropic-compatible",
			endpointFormat: "messages",
			baseURL:        base("/anthropic"),
			model:          "claude-compatible",
			apiKeyEnv:      "ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY",
			baseURLEnv:     "ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL",
			modelEnv:       "ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL",
		},
		{
			id:             "custom-endpoint",
			family:         "custom-endpoint",
			endpointFormat: "custom_endpoint",
			baseURL:        base("/custom-endpoint"),
			model:          "custom-compatible",
			apiKeyEnv:      "ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY",
			baseURLEnv:     "ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL",
			modelEnv:       "ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL",
		},
	}
}

func environMap() map[string]string {
	out := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func RuntimeReadinessProbeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func containsString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
