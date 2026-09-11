package executiongrant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	runtimeports "analytix.local/runtime-go/internal/ports"
	grantregistryport "analytix.local/runtime-go/internal/ports/grantregistry"
)

const DefaultTTL = 15 * time.Minute

type Expectation struct {
	ContextDigest   string
	TurnID          string
	Provider        string
	ServerIdentity  string
	ToolName        string
	ToolCallID      string
	ConnectionEpoch uint64
	ArgsHash        string
	SchemaHash      string
	ScopeHash       string
	ApprovalState   string
	ReadOnly        bool
	Now             time.Time
}

type ValidationError struct {
	Code string
}

type AuthorizationInput struct {
	OperationContext      context.Context
	SecurityAuthority     turnsecurityapp.WorkspaceSecurityAuthority
	Reader                turnsecurityapp.WorkspaceReader
	Workspace             string
	SourceDiagnostics     []any
	LiveMCPTools          map[string]bool
	MCPServerIdentities   map[string]string
	Context               domainsecurity.TurnSecurityContext
	Grant                 domainsecurity.ExecutionGrant
	Provider              string
	Call                  domainmodel.ToolCall
	ToolScope             []string
	ExpectedApprovalState string
	Now                   time.Time
}

func (err ValidationError) Error() string {
	return err.Code
}

// TerminalFailureCode exposes a typed, host-authored classification without
// requiring terminal code to parse an error string.
func (err ValidationError) TerminalFailureCode() string {
	return strings.TrimSpace(err.Code)
}

// IssueProvider admits a provider-originated call only after the provider
// ingress has replaced its untrusted identifier with a host-issued identity.
func IssueProvider(context domainsecurity.TurnSecurityContext, provider string, call domainmodel.ToolCall, schemas []domainmodel.ToolSchema, toolScope []string, readOnly bool, approvalState string, connectionEpoch uint64, serverIdentity string, now time.Time) (domainsecurity.ExecutionGrant, error) {
	if !domainmodel.IsHostToolCallIDV1(call.ID) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_identity_invalid"}
	}
	return issue(context, provider, call, schemas, toolScope, readOnly, approvalState, connectionEpoch, serverIdentity, now)
}

// IssueHost is the explicit path for host-synthesized calls. Host callers use
// the same canonical identity type; origin changes who creates the identity,
// never the validation applied to durable execution authority.
func IssueHost(context domainsecurity.TurnSecurityContext, provider string, call domainmodel.ToolCall, schemas []domainmodel.ToolSchema, toolScope []string, readOnly bool, approvalState string, connectionEpoch uint64, serverIdentity string, now time.Time) (domainsecurity.ExecutionGrant, error) {
	if !domainmodel.IsHostToolCallIDV1(call.ID) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_identity_invalid"}
	}
	return issue(context, provider, call, schemas, toolScope, readOnly, approvalState, connectionEpoch, serverIdentity, now)
}

func issue(context domainsecurity.TurnSecurityContext, provider string, call domainmodel.ToolCall, schemas []domainmodel.ToolSchema, toolScope []string, readOnly bool, approvalState string, connectionEpoch uint64, serverIdentity string, now time.Time) (domainsecurity.ExecutionGrant, error) {
	if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context); err != nil {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_context_invalid"}
	}
	if callUsesCaseDataAuthority(call) && domainsecurity.TurnSecurityContextIsBoundaryOnly(context) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_context_invalid"}
	}
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_identity_invalid"}
	}
	_, _, isMCP, malformedMCP := mcpToolIdentity(call.Name)
	if malformedMCP {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_tool_identity_invalid"}
	}
	if err := toolcatalogapp.ValidateProviderVisibleToolSchemas(schemas); err != nil {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_schema_invalid"}
	}
	schema, ok := schemaByName(call.Name, schemas)
	if !ok || len(schema.Parameters) == 0 {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_schema_missing"}
	}
	if !exactToolScopeMatchesSchemas(toolScope, schemas) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_scope_mismatch"}
	}
	if _, blocked := toolcatalogapp.ValidateToolCallArguments(call, schemas); blocked {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_arguments_invalid"}
	}
	if caseDataCallRequiresCurrentAuthority(context, call, readOnly) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_case_authority_required"}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if approvalState != "not_required" && approvalState != "pending" {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_approval_invalid"}
	}
	if (isMCP && connectionEpoch == 0) || (!isMCP && connectionEpoch != 0) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_connection_epoch_invalid"}
	}
	boundIdentity := "host:builtin"
	if isMCP {
		serverID, _, _, _ := mcpToolIdentity(call.Name)
		if !validVerifiedMCPServerIdentity(serverIdentity, serverID, connectionEpoch) {
			return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_server_identity_invalid"}
		}
		boundIdentity = serverIdentity
	} else if strings.TrimSpace(serverIdentity) != "" && strings.TrimSpace(serverIdentity) != boundIdentity {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_server_identity_invalid"}
	}
	argsHash := ArgumentsHash(call.Arguments)
	if argsHash == "" {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_arguments_invalid"}
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context:         context,
		Provider:        provider,
		ServerIdentity:  boundIdentity,
		ToolName:        call.Name,
		ToolCallID:      call.ID,
		ConnectionEpoch: connectionEpoch,
		ArgsHash:        argsHash,
		SchemaHash:      toolcatalogapp.ToolSchemaHash([]domainmodel.ToolSchema{schema}),
		ScopeHash:       ScopeHash(toolScope),
		ReadOnly:        readOnly,
		ApprovalState:   approvalState,
		IssuedAt:        now,
		ExpiresAt:       now.Add(DefaultTTL),
	})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_integrity_invalid"}
	}
	return grant, nil
}

func Validate(grant domainsecurity.ExecutionGrant, expected Expectation) error {
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		return ValidationError{Code: "execution_grant_integrity_invalid"}
	}
	if _, _, _, malformed := mcpToolIdentity(grant.ToolName); malformed {
		return ValidationError{Code: "execution_grant_tool_identity_invalid"}
	}
	if _, _, _, malformed := mcpToolIdentity(expected.ToolName); malformed {
		return ValidationError{Code: "execution_grant_tool_identity_invalid"}
	}
	checks := []struct {
		actual string
		want   string
		code   string
	}{
		{grant.ContextDigest, expected.ContextDigest, "execution_grant_context_mismatch"},
		{grant.TurnID, expected.TurnID, "execution_grant_turn_mismatch"},
		{grant.Provider, expected.Provider, "execution_grant_provider_mismatch"},
		{grant.ServerIdentity, expected.ServerIdentity, "execution_grant_server_mismatch"},
		{grant.ToolName, expected.ToolName, "execution_grant_tool_mismatch"},
		{grant.ToolCallID, expected.ToolCallID, "execution_grant_call_mismatch"},
		{grant.ArgsHash, expected.ArgsHash, "execution_grant_arguments_mismatch"},
		{grant.SchemaHash, expected.SchemaHash, "execution_grant_schema_mismatch"},
		{grant.ScopeHash, expected.ScopeHash, "execution_grant_scope_mismatch"},
		{grant.ApprovalState, expected.ApprovalState, "execution_grant_approval_mismatch"},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.actual) == "" || check.actual != check.want {
			return ValidationError{Code: check.code}
		}
	}
	if grant.ReadOnly != expected.ReadOnly {
		return ValidationError{Code: "execution_grant_readonly_mismatch"}
	}
	if grant.ConnectionEpoch != expected.ConnectionEpoch {
		return ValidationError{Code: "execution_grant_connection_epoch_mismatch"}
	}
	now := expected.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || !expiresAt.After(issuedAt) {
		return ValidationError{Code: "execution_grant_time_invalid"}
	}
	if issuedAt.After(now.Add(5*time.Second)) || expiresAt.Sub(issuedAt) > DefaultTTL {
		return ValidationError{Code: "execution_grant_time_invalid"}
	}
	if !now.Before(expiresAt) {
		return ValidationError{Code: "execution_grant_expired"}
	}
	return nil
}

func Authorize(input AuthorizationInput) error {
	if !domainmodel.IsHostToolCallIDV1(input.Call.ID) ||
		!domainmodel.IsHostToolCallIDV1(input.Grant.ToolCallID) {
		return ValidationError{Code: "execution_grant_identity_invalid"}
	}
	if input.OperationContext == nil || input.SecurityAuthority.Observer == nil {
		return ValidationError{Code: "execution_grant_authority_unavailable"}
	}
	currentInput := turnsecurityapp.CurrentValidationInput{
		OperationContext: input.OperationContext, Identity: input.SecurityAuthority.Identity,
		Observer:      input.SecurityAuthority.Observer,
		RiskAuthority: input.SecurityAuthority.RiskAuthority, SnapshotAuthority: input.SecurityAuthority.SnapshotAuthority,
		SnapshotAuthorityV2: input.SecurityAuthority.SnapshotAuthorityV2,
		Reader:              input.Reader, Context: input.Context, Workspace: input.Workspace, SourceDiagnostics: input.SourceDiagnostics,
	}
	if currentErr := turnsecurityapp.ValidateCurrentForEffect(
		currentInput, callUsesCaseDataAuthority(input.Call),
	); currentErr != nil {
		return turnSecurityValidationError(currentErr)
	}
	if input.Context.TurnID != input.Grant.TurnID || input.Context.ContextDigest != input.Grant.ContextDigest {
		return ValidationError{Code: "execution_grant_context_mismatch"}
	}
	_, _, isMCP, malformedMCP := mcpToolIdentity(input.Call.Name)
	if malformedMCP {
		return ValidationError{Code: "execution_grant_tool_identity_invalid"}
	}
	if isMCP && !input.LiveMCPTools[input.Call.Name] {
		return ValidationError{Code: "execution_grant_source_unavailable"}
	}
	expectedServerIdentity := "host:builtin"
	if isMCP {
		expectedServerIdentity = strings.TrimSpace(input.MCPServerIdentities[input.Call.Name])
		serverID, _, _, _ := mcpToolIdentity(input.Call.Name)
		if !validVerifiedMCPServerIdentity(expectedServerIdentity, serverID, input.Grant.ConnectionEpoch) {
			return ValidationError{Code: "execution_grant_server_identity_invalid"}
		}
	}
	approvalState := strings.TrimSpace(input.ExpectedApprovalState)
	if approvalState == "" {
		approvalState = input.Grant.ApprovalState
		if approvalState != "not_required" && approvalState != "approved" {
			return ValidationError{Code: "execution_grant_approval_mismatch"}
		}
	}
	if err := Validate(input.Grant, Expectation{
		ContextDigest: input.Context.ContextDigest, TurnID: input.Context.TurnID, Provider: input.Provider,
		ServerIdentity: expectedServerIdentity, ToolName: input.Call.Name, ToolCallID: input.Call.ID,
		ConnectionEpoch: input.Grant.ConnectionEpoch, ArgsHash: ArgumentsHash(input.Call.Arguments), SchemaHash: input.Grant.SchemaHash, ScopeHash: ScopeHash(input.ToolScope),
		ApprovalState: approvalState, ReadOnly: input.Grant.ReadOnly, Now: input.Now,
	}); err != nil {
		return err
	}
	if caseDataCallRequiresCurrentAuthority(input.Context, input.Call, input.Grant.ReadOnly) {
		return ValidationError{Code: "execution_grant_case_authority_required"}
	}
	if strings.TrimSpace(input.ExpectedApprovalState) != "pending" && toolcatalogapp.IsCaseBoundSecurityContext(input.Context) &&
		input.Grant.ServerIdentity == "host:builtin" && toolcatalogapp.CaseArtifactCallRequiresAuthority(input.Call) {
		return ValidationError{Code: "publication_receipt_required"}
	}
	return nil
}

func caseDataCallRequiresCurrentAuthority(
	context domainsecurity.TurnSecurityContext,
	call domainmodel.ToolCall,
	readOnly bool,
) bool {
	name := strings.TrimSpace(call.Name)
	if name == toolcatalogapp.ReportDeliveryToolName {
		return domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil
	}
	if !toolcatalogapp.MCPToolNeedsAnalytixCaseContext(name) &&
		!toolcatalogapp.CaseArtifactCallRequiresAuthority(call) {
		return false
	}
	return !readOnly || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(context)
}

func callUsesCaseDataAuthority(call domainmodel.ToolCall) bool {
	name := strings.TrimSpace(call.Name)
	return name == toolcatalogapp.ReportDeliveryToolName ||
		toolcatalogapp.MCPToolNeedsAnalytixCaseContext(name) ||
		toolcatalogapp.CaseArtifactCallRequiresAuthority(call)
}

// CallUsesCaseDataAuthority exposes the host-owned per-call classification to
// downstream execution boundaries. It deliberately delegates to the same
// classifier used for grant issuance and currentness validation.
func CallUsesCaseDataAuthority(call domainmodel.ToolCall) bool {
	return callUsesCaseDataAuthority(call)
}

// ValidateContextForCall keeps the ordinary/protected choice next to the
// canonical tool classifier. A structurally executable general context is not
// case-fact authority merely because it can run ordinary effects.
func ValidateContextForCall(context domainsecurity.TurnSecurityContext, call domainmodel.ToolCall) error {
	if callUsesCaseDataAuthority(call) {
		return domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context)
	}
	return domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context)
}

// ValidateExecutionGrantForCall binds a syntactically valid grant to the
// exact turn context under the same per-call ordinary/protected rule used at
// issuance and live authorization.
func ValidateExecutionGrantForCall(
	context domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	call domainmodel.ToolCall,
) error {
	if err := ValidateExecutionGrantForToolName(context, grant, call.Name); err != nil {
		return err
	}
	if grant.ToolCallID != strings.TrimSpace(call.ID) ||
		grant.ArgsHash != domainsecurity.CanonicalJSONHash(call.Arguments) {
		return ValidationError{Code: "execution_grant_call_mismatch"}
	}
	return nil
}

// ValidateExecutionGrantForToolName is the durable replay form used where the
// private argument body is intentionally absent. It still applies the
// canonical tool-name classifier and exact turn/context binding.
func ValidateExecutionGrantForToolName(
	context domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	toolName string,
) error {
	call := domainmodel.ToolCall{Name: strings.TrimSpace(toolName)}
	if err := ValidateContextForCall(context, call); err != nil {
		return err
	}
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		return err
	}
	if grant.TurnID != context.TurnID || grant.ContextDigest != context.ContextDigest ||
		grant.ToolName != call.Name {
		return ValidationError{Code: "execution_grant_context_mismatch"}
	}
	return nil
}

func turnSecurityValidationError(err error) error {
	if err == nil {
		return nil
	}
	switch strings.TrimSpace(err.Error()) {
	case "turn_security_workspace_mismatch", "turn_security_case_binding_mismatch", "turn_security_dataset_snapshot_mismatch", "turn_security_risk_policy_mismatch":
		return ValidationError{Code: strings.TrimSpace(err.Error())}
	default:
		return ValidationError{Code: "execution_grant_context_invalid"}
	}
}

func AuthorizePending(ctx context.Context, authority turnsecurityapp.WorkspaceSecurityAuthority, reader turnsecurityapp.WorkspaceReader, store grantregistryport.Reader, source runtimeports.MCPToolAdvertisementSource, pending appmodel.PendingToolCall, approvalState string, now time.Time) error {
	_, err := AuthorizePendingRegistry(ctx, authority, reader, store, source, pending, approvalState, now)
	return err
}

func ValidatePendingCatalog(pending appmodel.PendingToolCall, schemas []domainmodel.ToolSchema, currentReadOnly bool) error {
	if domainsecurity.ValidateExecutionGrant(pending.ExecutionGrant) != nil || pending.ExecutionGrant.ToolName != pending.Call.Name ||
		pending.ExecutionGrant.ToolCallID != pending.Call.ID || pending.ExecutionGrant.ReadOnly != currentReadOnly {
		return ValidationError{Code: "execution_grant_catalog_policy_mismatch"}
	}
	if err := toolcatalogapp.ValidateProviderVisibleToolSchemas(schemas); err != nil {
		return ValidationError{Code: "execution_grant_schema_unavailable"}
	}
	currentScope := make([]string, 0, len(schemas))
	seen := map[string]bool{}
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if name == "" || seen[name] {
			return ValidationError{Code: "execution_grant_schema_unavailable"}
		}
		seen[name] = true
		currentScope = append(currentScope, name)
	}
	if !seen[pending.Call.Name] || ScopeHash(currentScope) != pending.ExecutionGrant.ScopeHash ||
		ScopeHash(pending.ToolScope) != pending.ExecutionGrant.ScopeHash || SchemaHash(pending.Call.Name, schemas) != pending.ExecutionGrant.SchemaHash {
		return ValidationError{Code: "execution_grant_schema_mismatch"}
	}
	if _, blocked := toolcatalogapp.ValidateToolCallArguments(pending.Call, schemas); blocked {
		return ValidationError{Code: "execution_grant_arguments_invalid"}
	}
	return nil
}

func AuthorizePendingRegistry(ctx context.Context, authority turnsecurityapp.WorkspaceSecurityAuthority, reader turnsecurityapp.WorkspaceReader, store grantregistryport.Reader, source runtimeports.MCPToolAdvertisementSource, pending appmodel.PendingToolCall, approvalState string, now time.Time) (domainsecurity.ExecutionGrantRegistry, error) {
	if ctx == nil || authority.Observer == nil {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_authority_unavailable"}
	}
	workspace, thread, err := authoritativePendingWorkspace(reader, store, pending)
	if err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	registryStatus := domainsecurity.GrantRegistryActive
	if strings.TrimSpace(approvalState) == "pending" {
		registryStatus = domainsecurity.GrantRegistryPending
	}
	registry, err := RegistryFromThread(pending.ThreadID, thread, pending.TurnID)
	if err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(registry, pending.ThreadID, pending.TurnID, pending.ExecutionGrant, registryStatus); err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, ValidationError{Code: "execution_grant_registry_membership_invalid"}
	}
	if err := ValidatePriorSettledToolReferences(pending.ThreadID, thread, pending.SecurityContext, pending.ExecutionGrant, pending.PriorSettledToolRefs); err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	if err := authorizePendingAgainstCurrent(ctx, authority, reader, source, pending, approvalState, now, workspace); err != nil {
		return domainsecurity.ExecutionGrantRegistry{}, err
	}
	return registry, nil
}

// AuthorizeUnregisteredPending validates the exact live authority that may
// admit a freshly issued grant before its ready item is durable. It performs
// every current-context, source, schema, and prior-settlement check used by
// execution, but deliberately does not require registry membership that can
// only exist after this admission succeeds.
func AuthorizeUnregisteredPending(
	ctx context.Context,
	authority turnsecurityapp.WorkspaceSecurityAuthority,
	reader turnsecurityapp.WorkspaceReader,
	store grantregistryport.Reader,
	source runtimeports.MCPToolAdvertisementSource,
	pending appmodel.PendingToolCall,
	approvalState string,
	now time.Time,
) error {
	if ctx == nil || authority.Observer == nil {
		return ValidationError{Code: "execution_grant_authority_unavailable"}
	}
	workspace, thread, err := authoritativePendingWorkspace(reader, store, pending)
	if err != nil {
		return err
	}
	if err := ValidatePriorSettledToolReferencesBeforeAdmission(
		pending.ThreadID,
		thread,
		pending.SecurityContext,
		pending.ExecutionGrant,
		pending.PriorSettledToolRefs,
	); err != nil {
		return err
	}
	return authorizePendingAgainstCurrent(ctx, authority, reader, source, pending, approvalState, now, workspace)
}

func authorizePendingAgainstCurrent(
	ctx context.Context,
	authority turnsecurityapp.WorkspaceSecurityAuthority,
	reader turnsecurityapp.WorkspaceReader,
	source runtimeports.MCPToolAdvertisementSource,
	pending appmodel.PendingToolCall,
	approvalState string,
	now time.Time,
	workspace string,
) error {
	_, _, isMCP, malformedMCP := mcpToolIdentity(pending.Call.Name)
	if malformedMCP {
		return ValidationError{Code: "execution_grant_tool_identity_invalid"}
	}
	liveMCPTools := map[string]bool{}
	mcpServerIdentities := map[string]string{}
	if isMCP {
		advertisements, snapshotOK := toolcatalogapp.MCPToolAdvertisementsForSecurityContextV1(source, pending.SecurityContext)
		if !snapshotOK {
			return ValidationError{Code: "execution_grant_source_unavailable"}
		}
		var current toolcatalogapp.MCPToolAdvertisementV1
		found := false
		for _, advertisement := range advertisements {
			if advertisement.Name == pending.Call.Name {
				current = advertisement
				found = true
				break
			}
		}
		if !found {
			return ValidationError{Code: "execution_grant_source_unavailable"}
		}
		if current.ConnectionEpoch != pending.ExecutionGrant.ConnectionEpoch {
			return ValidationError{Code: "execution_grant_connection_epoch_mismatch"}
		}
		if current.ServerIdentity != pending.ExecutionGrant.ServerIdentity {
			return ValidationError{Code: "execution_grant_server_mismatch"}
		}
		if current.ReadOnly != pending.ExecutionGrant.ReadOnly {
			return ValidationError{Code: "execution_grant_catalog_policy_mismatch"}
		}
		description := current.Description
		if strings.TrimSpace(description) == "" {
			description = "Tool from a configured MCP server."
		}
		currentSchema := []domainmodel.ToolSchema{{
			Name: pending.Call.Name, Description: description, Parameters: current.InputSchema, OutputSchema: current.OutputSchema,
			Source: "mcp", TaskSupport: string(current.TaskSupport),
		}}
		if SchemaHash(pending.Call.Name, currentSchema) != pending.ExecutionGrant.SchemaHash {
			return ValidationError{Code: "execution_grant_schema_mismatch"}
		}
		if _, blocked := toolcatalogapp.ValidateToolCallArguments(pending.Call, currentSchema); blocked {
			return ValidationError{Code: "execution_grant_arguments_invalid"}
		}
		liveMCPTools = toolcatalogapp.ToolNameSet(toolcatalogapp.MCPToolNamesFromAdvertisementsV1(advertisements))
		mcpServerIdentities = toolcatalogapp.MCPServerIdentitiesFromAdvertisementsV1(advertisements)
	}
	if err := Authorize(AuthorizationInput{
		OperationContext: ctx, SecurityAuthority: authority,
		Reader: reader, Workspace: workspace, SourceDiagnostics: turnsecurityapp.SourceDiagnosticsForContext(source, pending.SecurityContext),
		LiveMCPTools: liveMCPTools, MCPServerIdentities: mcpServerIdentities,
		Context: pending.SecurityContext, Grant: pending.ExecutionGrant,
		Provider: pending.ProviderID, Call: pending.Call, ToolScope: pending.ToolScope, ExpectedApprovalState: approvalState, Now: now,
	}); err != nil {
		return err
	}
	return nil
}

func AuthorizePendingApproval(ctx context.Context, authority turnsecurityapp.WorkspaceSecurityAuthority, reader turnsecurityapp.WorkspaceReader, store grantregistryport.Reader, source runtimeports.MCPToolAdvertisementSource, pending appmodel.PendingToolCall, decision string, now time.Time) (domainsecurity.ExecutionGrant, error) {
	if err := AuthorizePending(ctx, authority, reader, store, source, pending, "pending", now); err != nil {
		return domainsecurity.ExecutionGrant{}, err
	}
	if strings.TrimSpace(decision) != "allow" {
		return pending.ExecutionGrant, nil
	}
	return Approve(pending.ExecutionGrant, now)
}

func authoritativePendingWorkspace(reader turnsecurityapp.WorkspaceReader, store grantregistryport.Reader, pending appmodel.PendingToolCall) (string, map[string]any, error) {
	if reader == nil || store == nil {
		return "", nil, ValidationError{Code: "execution_grant_authority_unavailable"}
	}
	thread, err := store.GetThread(pending.ThreadID)
	if err != nil || thread == nil {
		return "", nil, ValidationError{Code: "execution_grant_thread_unavailable"}
	}
	currentContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || currentContext != pending.SecurityContext {
		return "", nil, ValidationError{Code: "execution_grant_context_mismatch"}
	}
	epochState, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil || epochState.ThreadID != pending.ThreadID ||
		epochState.AcceptedSnapshot.Epoch != pending.SecurityContext.ContextEpoch {
		return "", nil, ValidationError{Code: "execution_grant_context_epoch_mismatch"}
	}
	turn, ok := appmodel.TurnByID(thread, pending.TurnID)
	if !ok || (stringMapField(turn, "status") != "running" && stringMapField(turn, "status") != "waiting") {
		return "", nil, ValidationError{Code: "execution_grant_turn_inactive"}
	}
	turnContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || turnContext != pending.SecurityContext {
		return "", nil, ValidationError{Code: "execution_grant_context_mismatch"}
	}
	workspace := stringMapField(thread, "workspace")
	realPath, err := reader.WorkspaceRealPath(workspace)
	if err != nil || realPath != pending.SecurityContext.WorkspaceRealPath {
		return "", nil, ValidationError{Code: "execution_grant_workspace_mismatch"}
	}
	return workspace, thread, nil
}

func stringMapField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func SettlementOutput(output any, isError bool, grant domainsecurity.ExecutionGrant, toolName string, expectedApprovalState string, authorize func() error) (any, bool) {
	if grant.ApprovalState == "pending" && expectedApprovalState != "pending" {
		return ErrorDetails(ValidationError{Code: "execution_grant_approval_pending"}, toolName), true
	}
	if authorize == nil {
		return ErrorDetails(ValidationError{Code: "execution_grant_authority_unavailable"}, toolName), true
	}
	if err := authorize(); err != nil {
		return ErrorDetails(err, toolName), true
	}
	return output, isError
}

func PendingBoundaryOutput(output any, isError bool) bool {
	if !isError {
		return false
	}
	record, ok := output.(map[string]any)
	if !ok {
		return false
	}
	code, _ := record["code"].(string)
	switch strings.TrimSpace(code) {
	case "approval_denied", "approval_policy_blocked", "loop_guard", "publication_receipt_required", "read_before_edit_required", "sandbox_blocked", "subagent_tool_filtered", "tool_cancelled", "tool_failed", "tool_timeout", "unsupported_file_type", "validation_error", "workspace_escape":
		return true
	default:
		return false
	}
}

func Approve(grant domainsecurity.ExecutionGrant, now time.Time) (domainsecurity.ExecutionGrant, error) {
	if grant.ApprovalState != "pending" {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_approval_mismatch"}
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || !now.UTC().Before(expiresAt) {
		return domainsecurity.ExecutionGrant{}, ValidationError{Code: "execution_grant_expired"}
	}
	issuedAt := now.UTC()
	approved := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: domainsecurity.TurnSecurityContext{
			TurnID:        grant.TurnID,
			ContextDigest: grant.ContextDigest,
		},
		Provider:        grant.Provider,
		ServerIdentity:  grant.ServerIdentity,
		ToolName:        grant.ToolName,
		ToolCallID:      grant.ToolCallID,
		ConnectionEpoch: grant.ConnectionEpoch,
		ArgsHash:        grant.ArgsHash,
		SchemaHash:      grant.SchemaHash,
		ScopeHash:       grant.ScopeHash,
		ReadOnly:        grant.ReadOnly,
		ApprovalState:   "approved",
		IssuedAt:        issuedAt,
		ExpiresAt:       expiresAt,
	})
	return approved, nil
}

func SchemaHash(toolName string, schemas []domainmodel.ToolSchema) string {
	schema, ok := schemaByName(toolName, schemas)
	if !ok {
		return ""
	}
	return toolcatalogapp.ToolSchemaHash([]domainmodel.ToolSchema{schema})
}

func ArgumentsHash(arguments json.RawMessage) string {
	return domainsecurity.CanonicalJSONHash(arguments)
}

func ScopeHash(scope []string) string {
	values := append([]string(nil), scope...)
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	sort.Strings(values)
	body, _ := json.Marshal(values)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func ServerIdentity(toolName string) string {
	return BoundServerIdentity(toolName, domainsecurity.TurnSecurityContext{}, 0)
}

func BoundServerIdentity(toolName string, context domainsecurity.TurnSecurityContext, connectionEpoch uint64) string {
	_, _, isMCP, malformed := mcpToolIdentity(toolName)
	if malformed {
		return ""
	}
	if !isMCP {
		return "host:builtin"
	}
	return ""
}

func validVerifiedMCPServerIdentity(identity string, serverID string, connectionEpoch uint64) bool {
	parsed, err := domainsecurity.ParseVerifiedMCPServerIdentity(strings.TrimSpace(identity))
	return err == nil && domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(parsed) && parsed.ServerID == serverID && parsed.ConnectionEpoch == connectionEpoch
}

func mcpToolIdentity(toolName string) (serverID, operation string, isMCP, malformed bool) {
	serverID, operation, ok := domainmcpname.Parse(toolName)
	if ok {
		return serverID, operation, true, false
	}
	trimmed := strings.TrimSpace(toolName)
	return "", "", false, strings.HasPrefix(strings.ToLower(trimmed), "mcp__")
}

func schemaByName(toolName string, schemas []domainmodel.ToolSchema) (domainmodel.ToolSchema, bool) {
	for _, schema := range schemas {
		if schema.Name == toolName {
			return schema, true
		}
	}
	return domainmodel.ToolSchema{}, false
}

func exactToolScopeMatchesSchemas(scope []string, schemas []domainmodel.ToolSchema) bool {
	if len(scope) == 0 || len(scope) != len(schemas) {
		return false
	}
	schemaNames := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if name == "" || name != schema.Name {
			return false
		}
		if _, duplicate := schemaNames[name]; duplicate {
			return false
		}
		schemaNames[name] = struct{}{}
	}
	scopeNames := make(map[string]struct{}, len(scope))
	for _, raw := range scope {
		name := strings.TrimSpace(raw)
		if name == "" || name != raw {
			return false
		}
		if _, duplicate := scopeNames[name]; duplicate {
			return false
		}
		if _, advertised := schemaNames[name]; !advertised {
			return false
		}
		scopeNames[name] = struct{}{}
	}
	return len(scopeNames) == len(schemaNames)
}

func grantID(grant domainsecurity.ExecutionGrant) string {
	grant.GrantID = ""
	body, _ := json.Marshal(grant)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func ErrorDetails(err error, toolName string) map[string]any {
	_ = toolName
	code := PublicValidationCode(err)
	return map[string]any{
		"code":  code,
		"error": "tool execution authorization was rejected by current host authority",
	}
}

// PublicValidationCode maps internal validation detail onto the closed set
// allowed to cross tool, control, event, and HTTP boundaries. Caller-provided
// tool names and arbitrary validation strings never become public codes.
func PublicValidationCode(err error) string {
	var typed ValidationError
	if !AsValidationError(err, &typed) {
		return "execution_grant_rejected"
	}
	code := strings.TrimSpace(typed.Code)
	switch code {
	case "publication_receipt_required":
		return code
	case "execution_grant_source_unavailable":
		return "execution_grant_source_unavailable"
	case "execution_grant_server_identity_invalid", "execution_grant_server_mismatch":
		return "execution_grant_server_identity_invalid"
	case "execution_grant_connection_epoch_invalid", "execution_grant_connection_epoch_mismatch":
		return "execution_grant_connection_epoch_mismatch"
	case "execution_grant_schema_missing", "execution_grant_schema_invalid", "execution_grant_schema_unavailable", "execution_grant_schema_mismatch", "execution_grant_catalog_policy_mismatch":
		return "execution_grant_schema_unavailable"
	case "execution_grant_expired", "execution_grant_time_invalid":
		return "execution_grant_expired"
	case "execution_grant_approval_invalid", "execution_grant_approval_mismatch", "execution_grant_approval_pending":
		return "execution_grant_approval_mismatch"
	case "execution_grant_context_invalid", "execution_grant_context_mismatch", "execution_grant_workspace_mismatch", "execution_grant_thread_unavailable", "execution_grant_turn_inactive", "execution_grant_identity_invalid":
		return "execution_grant_context_invalid"
	default:
		return "execution_grant_rejected"
	}
}

func AsValidationError(err error, target *ValidationError) bool {
	if err == nil || target == nil {
		return false
	}
	typed, ok := err.(ValidationError)
	if !ok {
		return false
	}
	*target = typed
	return true
}
