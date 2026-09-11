package toolcatalog

import (
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
)

func MCPToolNeedsAnalytixCaseContext(toolName string) bool {
	serverID, _, ok := domainmcpname.Parse(toolName)
	return ok && (serverID == "analytix_funds" || serverID == "analytix-fund-analysis")
}

func MCPToolIsInternalPlumbingCanary(toolName string) bool {
	serverID, operation, ok := domainmcpname.Parse(toolName)
	return ok && operation == "count_case_rows" &&
		(serverID == "analytix_funds" || serverID == "analytix-fund-analysis")
}

// MCPToolMayBeAdvertisedToProvider keeps protocol canaries out of the model
// catalog. The funds server may retain count_case_rows for host-side plumbing
// checks, but the first valuable product capability is the fixed account-flow
// operation only.
func MCPToolMayBeAdvertisedToProvider(toolName string) bool {
	serverID, operation, ok := domainmcpname.Parse(toolName)
	if !ok {
		return false
	}
	if serverID == "analytix_funds" || serverID == "analytix-fund-analysis" {
		return operation == "analyze_account_flows"
	}
	return true
}

// MCPToolRequiresHostArtifactAuthority identifies legacy provider-facing tools
// whose implementation can create or publish artifacts. They remain
// quarantined before transport execution until the host-only artifact and
// PublicationReceipt pipeline exists; arguments and remote annotations cannot
// downgrade this classification.
func MCPToolRequiresHostArtifactAuthority(toolName string) bool {
	_, operation, ok := domainmcpname.Parse(toolName)
	if !ok {
		return false
	}
	switch operation {
	case "run_full_case_analysis", "create_case_notebook", "export_cleaned_case_data":
		return true
	default:
		return false
	}
}

func MCPProviderArgumentsContainHostAuthority(arguments map[string]any) bool {
	return domainmcp.ProviderArgumentsContainHostAuthority(arguments)
}

// MCPProviderArguments returns a detached argument object with every reserved
// host-authority key removed. It is defense in depth for internal/non-provider
// callers; provider-originated calls are rejected before execution when any
// reserved key is present.
func MCPProviderArguments(arguments map[string]any) map[string]any {
	next := cloneMap(arguments)
	if next == nil {
		next = map[string]any{}
	}
	for _, key := range domainmcp.ProviderAuthorityArgumentKeys() {
		delete(next, key)
	}
	return next
}
