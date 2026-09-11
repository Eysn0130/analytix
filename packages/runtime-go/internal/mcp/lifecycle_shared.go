package mcp

import (
	mcpredaction "analytix.local/runtime-go/internal/adapters/outbound/mcp/redaction"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmcpprotocol "analytix.local/runtime-go/internal/domain/mcpprotocol"
)

const MCPProtocolVersion = domainmcpprotocol.PreferredVersion

func CanonicalToolName(serverID, rawName string) string {
	if name, err := CanonicalToolNameChecked(serverID, rawName); err == nil {
		return name
	}
	return "mcp__" + NormalizeName(serverID) + "__" + NormalizeName(rawName)
}

func CanonicalToolNameChecked(serverID, rawName string) (string, error) {
	return domainmcpname.Canonical(serverID, rawName)
}

func CanonicalToolPrefix(serverID string) string {
	if prefix, err := CanonicalToolPrefixChecked(serverID); err == nil {
		return prefix
	}
	return "mcp__" + NormalizeName(serverID) + "__"
}

func CanonicalToolPrefixChecked(serverID string) (string, error) {
	return domainmcpname.Namespace(serverID)
}

func NormalizeName(value string) string {
	return domainmcpname.Normalize(value)
}

func RedactedDiagnostic(headers, env map[string]string, rawURL string) (string, bool) {
	return mcpredaction.RedactedDiagnostic(headers, env, rawURL)
}

func redactAuthMap(input map[string]string) map[string]string {
	return mcpredaction.RedactAuthMap(input)
}

func redactAuthURL(raw string) string {
	return mcpredaction.RedactAuthURL(raw)
}

func isAuthish(key string) bool {
	return mcpredaction.IsAuthish(key)
}

func containsExplicitAuthMaterial(value string) bool {
	return mcpredaction.ContainsExplicitAuthMaterial(value)
}

func redactMCPAuthText(value string) string {
	return mcpredaction.RedactAuthText(value)
}

func explicitAuthValues(headers, env map[string]string, rawURL string) []string {
	return mcpredaction.ExplicitAuthValues(headers, env, rawURL)
}

func isAuthQueryKey(key string) bool {
	return mcpredaction.IsAuthQueryKey(key)
}

func containsAnyValue(text string, values []string) bool {
	return mcpredaction.ContainsAnyValue(text, values)
}
