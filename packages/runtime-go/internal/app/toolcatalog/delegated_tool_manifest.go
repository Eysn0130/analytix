package toolcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type canonicalDelegatedMCPAuthorityV1 struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	InputSchema     any    `json:"inputSchema"`
	OutputSchema    any    `json:"outputSchema"`
	TaskSupport     string `json:"taskSupport"`
	ReadOnly        bool   `json:"readOnly"`
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	ServerIdentity  string `json:"serverIdentity"`
}

// BuildDelegatedToolManifestV1 is the sole builder used before a child job is
// queued and before each child provider dispatch. Unrelated MCP catalog tools
// are excluded because only provider-visible MCP schemas inside the frozen
// child scope are delegated.
func BuildDelegatedToolManifestV1(
	toolScope []string,
	toolSchemas []domainmodel.ToolSchema,
	advertisements []MCPToolAdvertisementV1,
) (*domainjob.DelegatedToolManifestV1, error) {
	if _, err := domainjob.DelegatedToolScopeHashV1(toolScope); err != nil {
		return nil, err
	}
	advertisementByName := make(map[string]MCPToolAdvertisementV1, len(advertisements))
	for _, advertisement := range advertisements {
		name := strings.TrimSpace(advertisement.Name)
		if name == "" || advertisementByName[name].Name != "" {
			return nil, errors.New("delegated MCP advertisement set is invalid")
		}
		advertisementByName[name] = advertisement
	}
	canonicalMCP := make([]canonicalDelegatedMCPAuthorityV1, 0, len(advertisements))
	seenMCP := map[string]bool{}
	for _, schema := range toolSchemas {
		name := strings.TrimSpace(schema.Name)
		if name == "" {
			return nil, errors.New("delegated tool schema name is invalid")
		}
		if strings.TrimSpace(schema.Source) != "mcp" && !strings.HasPrefix(name, "mcp__") {
			continue
		}
		advertisement, ok := advertisementByName[name]
		serverID := MCPToolServerID(name)
		identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(strings.TrimSpace(advertisement.ServerIdentity))
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(advertisement.TaskSupport)
		if !ok || seenMCP[name] || serverID == "" || advertisement.ConnectionEpoch == 0 || identityErr != nil ||
			identity.ServerID != serverID || identity.ConnectionEpoch != advertisement.ConnectionEpoch || !taskSupportOK {
			return nil, errors.New("delegated MCP authority is invalid")
		}
		seenMCP[name] = true
		description := strings.TrimSpace(advertisement.Description)
		if description == "" {
			description = "Tool from a configured MCP server."
		}
		canonicalMCP = append(canonicalMCP, canonicalDelegatedMCPAuthorityV1{
			Name: name, Description: description,
			InputSchema: CanonicalToolParameters(advertisement.InputSchema), OutputSchema: CanonicalToolParameters(advertisement.OutputSchema),
			TaskSupport: string(taskSupport), ReadOnly: advertisement.ReadOnly,
			ConnectionEpoch: advertisement.ConnectionEpoch, ServerIdentity: strings.TrimSpace(advertisement.ServerIdentity),
		})
	}
	sort.SliceStable(canonicalMCP, func(i, j int) bool { return canonicalMCP[i].Name < canonicalMCP[j].Name })
	body, err := json.Marshal(canonicalMCP)
	if err != nil {
		return nil, err
	}
	return domainjob.NewDelegatedToolManifestV1(toolScope, ToolSchemaHash(toolSchemas), domainmodel.BytesHash(body))
}

func DelegatedToolManifestMismatchV1(expected *domainjob.DelegatedToolManifestV1, actual *domainjob.DelegatedToolManifestV1) string {
	if expected == nil || actual == nil || expected.Version != actual.Version || expected.ScopeHash != actual.ScopeHash ||
		expected.ToolSchemaHash != actual.ToolSchemaHash {
		return "subagent_tool_schema_mismatch"
	}
	if expected.MCPAuthorityHash != actual.MCPAuthorityHash {
		return "subagent_mcp_authority_mismatch"
	}
	if expected.ManifestHash != actual.ManifestHash {
		return "subagent_tool_manifest_invalid"
	}
	return ""
}

func ValidateDelegatedToolManifestForDispatchV1(
	expected *domainjob.DelegatedToolManifestV1,
	toolScope []string,
	toolSchemas []domainmodel.ToolSchema,
	advertisements []MCPToolAdvertisementV1,
) error {
	if expected == nil {
		return nil
	}
	actual, err := BuildDelegatedToolManifestV1(toolScope, toolSchemas, advertisements)
	if err != nil {
		return errors.New("subagent_tool_manifest_invalid")
	}
	if mismatch := DelegatedToolManifestMismatchV1(expected, actual); mismatch != "" {
		return errors.New(mismatch)
	}
	return nil
}
