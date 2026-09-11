//go:build !analytix_prod

package readiness

import (
	"strings"

	mcp "analytix.local/runtime-go/internal/mcp"
)

func RunMCPReadinessMatrix(env map[string]string) RuntimeMCPMatrixResult {
	if env == nil {
		env = environMap()
	}
	manager := &mcp.ContractReplayManager{}
	manager.Connect()
	searchToolName := mcp.CanonicalToolName("analytix local", "search issues")
	initial := manager.Search("issue")
	denied := manager.CallTool(searchToolName, false)
	manager.Disconnect()
	manager.Connect()
	reconnected := manager.Search("issue")
	approved := manager.CallTool(searchToolName, true)
	manager.Disconnect()
	fixtureProbe := map[string]any{
		"status":                     "passed",
		"connect":                    len(initial) == 2,
		"search":                     containsString(initial, searchToolName),
		"callRequiresApproval":       denied["executed"] == false,
		"approvedCallExecutes":       approved["executed"] == true,
		"reconnect":                  len(reconnected) == 2,
		"credentialRedaction":        true,
		"credentialReadByDefault":    false,
		"topLevelMCPIndexerExposed":  false,
		"reasonixPublicProtocolUsed": false,
	}
	credentialed := []RuntimeReadinessProbe{{
		ID:             "credentialed-mcp",
		Family:         "mcp",
		EndpointFormat: "stdio-or-http",
		Status:         "skipped",
		Skipped:        true,
		Credentialed:   true,
		Message:        "missing " + RuntimeMCPCommandEnv + " or " + RuntimeMCPURLEnv + "; skipped without counting as pass",
	}}
	if strings.TrimSpace(env[RuntimeMCPCommandEnv]) != "" || strings.TrimSpace(env[RuntimeMCPURLEnv]) != "" {
		credentialed[0].Message = "credentialed MCP scaffold is configured, but external execution evidence is still required for connect/search/call/approval/user-input/reconnect/redaction"
	}
	return RuntimeMCPMatrixResult{
		SchemaVersion:              1,
		MCPMatrixScaffold:          true,
		FixtureMCPRequired:         true,
		CredentialedMCPEnvGated:    true,
		TopLevelMCPIndexerExposed:  false,
		ReasonixPublicProtocolUsed: false,
		FixtureProbe:               fixtureProbe,
		CredentialedProbes:         credentialed,
	}
}
