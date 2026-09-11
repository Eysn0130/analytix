package toolcatalog

import (
	"encoding/json"
	"sort"
	"strings"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	runtimeports "analytix.local/runtime-go/internal/ports"
)

// MCPToolAdvertisementV1 keeps the app-layer name while the immutable value
// and side-effect port contract remain owned by domain/mcp and ports.
type MCPToolAdvertisementV1 = domainmcp.ToolAdvertisementV1

func MCPToolAdvertisementsForSecurityContextV1(source runtimeports.MCPToolAdvertisementSource, context domainsecurity.TurnSecurityContext) ([]MCPToolAdvertisementV1, bool) {
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return nil, false
	}
	if source == nil {
		return nil, false
	}
	raw := source.MCPToolAdvertisementSnapshotV1(context)
	type serverAuthority struct {
		identity string
		epoch    uint64
	}
	type serverPartition struct {
		authority    serverAuthority
		authoritySet bool
		invalid      bool
		seen         map[string]bool
		items        []MCPToolAdvertisementV1
	}
	partitions := map[string]*serverPartition{}
	for _, candidate := range raw {
		candidate.Name = strings.TrimSpace(candidate.Name)
		candidate.Description = strings.TrimSpace(candidate.Description)
		candidate.ServerIdentity = strings.TrimSpace(candidate.ServerIdentity)
		// count_case_rows is retained only for host-side MCP/authority checks.
		// Its readiness or schema cannot become a prerequisite for the
		// provider-facing valuable funds capability.
		if MCPToolIsInternalPlumbingCanary(candidate.Name) {
			continue
		}
		serverID := MCPToolServerID(candidate.Name)
		// A malformed name cannot identify an authority partition and cannot
		// produce an advertised tool. Ignore it without letting untrusted
		// identity text select an otherwise independent server to quarantine.
		if serverID == "" {
			continue
		}
		partition := partitions[serverID]
		if partition == nil {
			partition = &serverPartition{seen: map[string]bool{}}
			partitions[serverID] = partition
		}
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(candidate.TaskSupport)
		identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(candidate.ServerIdentity)
		if partition.seen[candidate.Name] || candidate.ConnectionEpoch == 0 || candidate.ServerIdentity == "" ||
			!taskSupportOK || !validMCPToolParameters(candidate.InputSchema) || !validMCPToolParameters(candidate.OutputSchema) {
			partition.invalid = true
			continue
		}
		if identityErr != nil || identity.ServerID != serverID || identity.ConnectionEpoch != candidate.ConnectionEpoch {
			partition.invalid = true
			continue
		}
		if partition.authoritySet &&
			(partition.authority.identity != candidate.ServerIdentity || partition.authority.epoch != candidate.ConnectionEpoch) {
			partition.invalid = true
			continue
		}
		partition.authority = serverAuthority{identity: candidate.ServerIdentity, epoch: candidate.ConnectionEpoch}
		partition.authoritySet = true
		partition.seen[candidate.Name] = true
		candidate.TaskSupport = taskSupport
		candidate.InputSchema = append(json.RawMessage(nil), candidate.InputSchema...)
		candidate.OutputSchema = append(json.RawMessage(nil), candidate.OutputSchema...)
		partition.items = append(partition.items, candidate)
	}
	out := make([]MCPToolAdvertisementV1, 0, len(raw))
	for _, partition := range partitions {
		if partition.invalid {
			continue
		}
		for _, candidate := range partition.items {
			if !MCPToolMayBeAdvertisedToProvider(candidate.Name) {
				continue
			}
			caseTool := MCPToolNeedsAnalytixCaseContext(candidate.Name)
			if candidate.TaskSupport == domainmcp.ToolTaskSupportRequired || MCPToolRequiresHostArtifactAuthority(candidate.Name) ||
				(caseTool && (!domainsecurity.TurnSecurityContextAllowsCaseEvidence(context) || !candidate.ReadOnly)) {
				continue
			}
			out = append(out, candidate)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, true
}

func ValidMCPToolAdvertisementsForSecurityContextV1(source runtimeports.MCPToolAdvertisementSource, context domainsecurity.TurnSecurityContext) []MCPToolAdvertisementV1 {
	advertisements, ok := MCPToolAdvertisementsForSecurityContextV1(source, context)
	if !ok {
		return nil
	}
	return advertisements
}

func MCPToolAdvertisementsByNameV1(advertisements []MCPToolAdvertisementV1, names []string) []MCPToolAdvertisementV1 {
	byName := make(map[string]MCPToolAdvertisementV1, len(advertisements))
	for _, advertisement := range advertisements {
		byName[advertisement.Name] = advertisement
	}
	out := make([]MCPToolAdvertisementV1, 0, len(names))
	for _, name := range names {
		if advertisement, ok := byName[name]; ok {
			out = append(out, advertisement)
		}
	}
	return out
}

func MCPToolSchemasFromAdvertisementsV1(advertisements []MCPToolAdvertisementV1) []MCPToolSchema {
	out := make([]MCPToolSchema, 0, len(advertisements))
	for _, advertisement := range advertisements {
		out = append(out, MCPToolSchema{
			Name: advertisement.Name, Description: advertisement.Description,
			Parameters:   append(json.RawMessage(nil), advertisement.InputSchema...),
			OutputSchema: append(json.RawMessage(nil), advertisement.OutputSchema...),
			TaskSupport:  advertisement.TaskSupport,
		})
	}
	return out
}

func MCPToolNamesFromAdvertisementsV1(advertisements []MCPToolAdvertisementV1) []string {
	out := make([]string, 0, len(advertisements))
	for _, advertisement := range advertisements {
		out = append(out, advertisement.Name)
	}
	return out
}

func MCPConnectionEpochsFromAdvertisementsV1(advertisements []MCPToolAdvertisementV1) map[string]uint64 {
	out := make(map[string]uint64, len(advertisements))
	for _, advertisement := range advertisements {
		out[advertisement.Name] = advertisement.ConnectionEpoch
	}
	return out
}

func MCPServerIdentitiesFromAdvertisementsV1(advertisements []MCPToolAdvertisementV1) map[string]string {
	out := make(map[string]string, len(advertisements))
	for _, advertisement := range advertisements {
		out[advertisement.Name] = advertisement.ServerIdentity
	}
	return out
}

func MCPReadOnlyPoliciesFromAdvertisementsV1(advertisements []MCPToolAdvertisementV1) map[string]bool {
	out := make(map[string]bool, len(advertisements))
	for _, advertisement := range advertisements {
		out[advertisement.Name] = advertisement.ReadOnly
	}
	return out
}
