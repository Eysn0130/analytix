package mcp

import (
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const HostContextEnvelopeVersion = 1

var providerAuthorityArgumentKeys = []string{"_analytix", "__analytix", "analytix_runtime_context"}

func ProviderAuthorityArgumentKeys() []string {
	return append([]string(nil), providerAuthorityArgumentKeys...)
}

func ProviderArgumentsContainHostAuthority(arguments map[string]any) bool {
	for _, key := range providerAuthorityArgumentKeys {
		if _, found := arguments[key]; found {
			return true
		}
	}
	return false
}

// HostContextEnvelope is an in-process authority carrier. Its fields are
// intentionally private so provider JSON cannot construct or mutate it.
type HostContextEnvelope struct {
	version int
	context domainsecurity.TurnSecurityContext
	grant   domainsecurity.ExecutionGrant
}

func NewHostContextEnvelope(securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) (HostContextEnvelope, error) {
	envelope := HostContextEnvelope{version: HostContextEnvelopeVersion, context: securityContext, grant: grant}
	if err := ValidateHostContextEnvelope(envelope); err != nil {
		return HostContextEnvelope{}, err
	}
	return envelope, nil
}

func ValidateHostContextEnvelope(envelope HostContextEnvelope) error {
	if envelope.version != HostContextEnvelopeVersion ||
		domainsecurity.ValidateExecutionGrantForContext(envelope.grant, envelope.context) != nil ||
		envelope.grant.TurnID != envelope.context.TurnID ||
		envelope.grant.ContextDigest != envelope.context.ContextDigest {
		return errors.New("MCP host context envelope authority is invalid")
	}
	return nil
}

func (envelope HostContextEnvelope) Authority() (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant, error) {
	if err := ValidateHostContextEnvelope(envelope); err != nil {
		return domainsecurity.TurnSecurityContext{}, domainsecurity.ExecutionGrant{}, err
	}
	return envelope.context, envelope.grant, nil
}

func (envelope HostContextEnvelope) RuntimeContextRecord() (map[string]any, error) {
	securityContext, grant, err := envelope.Authority()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"version":            HostContextEnvelopeVersion,
		"threadId":           securityContext.ThreadID,
		"turnId":             securityContext.TurnID,
		"workspaceRealPath":  securityContext.WorkspaceRealPath,
		"tenantId":           securityContext.TenantID,
		"userId":             securityContext.UserID,
		"caseId":             securityContext.CaseID,
		"caseBindingHash":    securityContext.CaseBindingHash,
		"datasetSnapshotId":  securityContext.DatasetSnapshotID,
		"sourceManifestHash": securityContext.SourceManifestHash,
		"contextEpoch":       securityContext.ContextEpoch,
		"issuedAt":           securityContext.IssuedAt,
		"contextDigest":      securityContext.ContextDigest,
		"grantId":            grant.GrantID,
		"provider":           grant.Provider,
		"serverIdentity":     grant.ServerIdentity,
		"toolName":           grant.ToolName,
		"toolCallId":         grant.ToolCallID,
		"connectionEpoch":    grant.ConnectionEpoch,
		"argsHash":           grant.ArgsHash,
		"schemaHash":         grant.SchemaHash,
		"scopeHash":          grant.ScopeHash,
		"readOnly":           grant.ReadOnly,
		"approvalState":      grant.ApprovalState,
		"grantIssuedAt":      grant.IssuedAt,
		"expiresAt":          grant.ExpiresAt,
	}, nil
}
