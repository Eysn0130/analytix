//go:build analytix_prod

package readiness

import "strings"

func RunMCPReadinessMatrix(env map[string]string) RuntimeMCPMatrixResult {
	if env == nil {
		env = environMap()
	}
	credentialed := []RuntimeReadinessProbe{{
		ID:             "credentialed-mcp",
		Family:         "mcp",
		EndpointFormat: "stdio-or-http",
		Status:         "skipped",
		Skipped:        true,
		Credentialed:   true,
		Message:        "production runtime requires real configured MCP stdio/http evidence; conformance fixture scaffold is excluded from analytix_prod builds",
	}}
	if strings.TrimSpace(env[RuntimeMCPCommandEnv]) != "" || strings.TrimSpace(env[RuntimeMCPURLEnv]) != "" {
		credentialed[0].Message = "credentialed MCP env is configured; production readiness still requires external connect/search/call/reconnect/redaction evidence"
	}
	return RuntimeMCPMatrixResult{
		SchemaVersion:              1,
		MCPMatrixScaffold:          false,
		FixtureMCPRequired:         false,
		CredentialedMCPEnvGated:    true,
		TopLevelMCPIndexerExposed:  false,
		ReasonixPublicProtocolUsed: false,
		FixtureProbe: map[string]any{
			"status":                         "excluded-from-production-build",
			"contractReplayMCPTransportUsed": false,
		},
		CredentialedProbes: credentialed,
	}
}
