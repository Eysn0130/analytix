package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// HostScheduleSideEffectProjectionV1 prepares only an exact current native
// Host request. This is a private semantic projection, not transport authority:
// actual dispatch still validates its durable grant against the live manager.
func (m *ProductionManager) HostScheduleSideEffectProjectionV1(ctx context.Context, envelope domainmcp.HostContextEnvelope, call domainmodel.ToolCall) (map[string]any, bool) {
	securityContext, grant, err := envelope.Authority()
	operation := domaintoolresult.ProtectedScheduleOperationForToolV1(call.Name)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	arguments, argumentsErr := domainsecurity.DecodeCanonicalJSONValue(call.Arguments)
	request, object := arguments.(map[string]any)
	if m == nil || ctx == nil || ctx.Err() != nil || err != nil || operation == "" ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		!domainmodel.IsHostToolCallIDV1(call.ID) || grant.ToolCallID != call.ID || grant.ToolName != call.Name ||
		grant.ReadOnly || (grant.ApprovalState != "approved" && grant.ApprovalState != "not_required") ||
		grant.ConnectionEpoch == 0 || expiresErr != nil || !time.Now().UTC().Before(expiresAt) ||
		argumentsErr != nil || !object || domainmcp.ProviderArgumentsContainHostAuthority(request) ||
		grant.ArgsHash != domainsecurity.CanonicalJSONHash(call.Arguments) {
		return nil, false
	}
	m.sourceExecutionMu.RLock()
	defer m.sourceExecutionMu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, toolOK := m.tools[call.Name]
	spec, specOK := m.specByIDNoLock(hostScheduleMCPServerIDV1)
	canonicalName, bindingErr := CanonicalToolNameChecked(tool.ServerID, tool.RawName)
	taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(tool.Tool.TaskSupport)
	if ctx.Err() != nil || !toolOK || tool.Placeholder || tool.ServerID != hostScheduleMCPServerIDV1 ||
		bindingErr != nil || canonicalName != call.Name || !m.managedToolCapabilityCurrentNoLock(call.Name, tool) ||
		!specOK || hostScheduleLoopbackPortV1(spec) == 0 || !mcpServerSpecAuthorizedForSecurityContext(spec, &securityContext) ||
		m.clients[tool.ServerID] == nil || m.connectionEpochs[tool.ServerID] != grant.ConnectionEpoch ||
		m.serverIdentities[tool.ServerID] != grant.ServerIdentity || managedToolSchemaHash(call.Name, tool) != grant.SchemaHash ||
		spec.ReadOnlyToolNames[tool.RawName] || !taskSupportOK || taskSupport == domainmcp.ToolTaskSupportRequired {
		return nil, false
	}
	// The trusted native owner supplies its closed input contract. Preserve exact
	// JSON values and optional-field presence; Host defaults remain Host-owned.
	if toolcatalogapp.ValidateProviderVisibleToolSchemas([]domainmodel.ToolSchema{{Name: call.Name, Parameters: tool.Tool.InputSchema}}) != nil ||
		toolcatalogapp.ValidateJSONSchemaRawValue(call.Arguments, tool.Tool.InputSchema, "arguments") != nil {
		return nil, false
	}
	return map[string]any{"operation": operation, "request": request}, true
}

type hostScheduleReceiptContextKeyV1 struct{}

// This envelope never enters the transport or provider arguments. The same
// source execution lease protects the binding checks and receipt creation.
func withHostScheduleReceiptContextV1(ctx context.Context, envelope domainmcp.HostContextEnvelope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, hostScheduleReceiptContextKeyV1{}, envelope)
}

type hostScheduleReceiptCarrierV1 struct {
	mu        sync.Mutex
	owner     *ProductionManager
	active    bool
	envelope  domainmcp.HostContextEnvelope
	rawSHA256 string
	receipt   *domaintoolresult.ProtectedScheduleReceiptV1
}

func (*hostScheduleReceiptCarrierV1) String() string               { return "[host-private schedule receipt]" }
func (c *hostScheduleReceiptCarrierV1) GoString() string           { return c.String() }
func (*hostScheduleReceiptCarrierV1) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

func (c *hostScheduleReceiptCarrierV1) discard() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
	c.envelope = domainmcp.HostContextEnvelope{}
	c.rawSHA256 = ""
	c.receipt = nil
}

// Called only after the manager's current native spec/client/epoch checks and
// the generic executor's raw/value/output-schema validation. Same-name MCP
// servers and unbound calls cannot reach this mint point.
func (m *ProductionManager) attachHostScheduleReceiptV1(ctx context.Context, toolName string, output map[string]any) {
	operation := domaintoolresult.ProtectedScheduleOperationForToolV1(toolName)
	envelope, bound := ctx.Value(hostScheduleReceiptContextKeyV1{}).(domainmcp.HostContextEnvelope)
	_, grant, err := envelope.Authority()
	lossless, ok := output[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if m == nil || ctx.Err() != nil || operation == "" || !bound || err != nil || grant.ToolName != toolName ||
		output["executed"] != true || output["transportStatus"] != "success" || output["code"] == "mcp_output_schema_invalid" ||
		!ok || !domainmcp.ValidLosslessToolResult(lossless) || lossless.HostPrivate != nil {
		return
	}
	recomputed := mcpprotocol.ExtractLosslessToolResult(lossless.RawResult)
	if !domainmcp.ValidLosslessToolResult(recomputed) || !reflect.DeepEqual(lossless.Value, recomputed.Value) {
		return
	}
	raw, err := domainjsonstrict.DecodeObject(lossless.RawResult, domainjsonstrict.Options{
		MaxBytes: domaintoolresult.ProtectedSnapshotEnvelopeLimitV1, MaxDepth: 16, MaxTokens: 2000, MaxStringBytes: domaintoolresult.ProtectedSnapshotEnvelopeLimitV1,
	})
	if err != nil {
		return
	}
	isError := false
	if value, present := raw["isError"]; present {
		var valid bool
		isError, valid = value.(bool)
		if !valid {
			return
		}
	}
	outputIsError := false
	if value, present := output["isError"]; present {
		var valid bool
		outputIsError, valid = value.(bool)
		if !valid {
			return
		}
	}
	if outputIsError != isError {
		return
	}
	structured, ok := raw["structuredContent"].(map[string]any)
	if !ok {
		return
	}
	body, err := json.Marshal(structured)
	if err != nil {
		return
	}
	receipt, err := domaintoolresult.ParseProtectedScheduleToolOutputV1(operation, body, isError)
	if err != nil {
		return
	}
	lossless.HostPrivate = &hostScheduleReceiptCarrierV1{owner: m, active: true, envelope: envelope, rawSHA256: lossless.RawSHA256, receipt: receipt}
	output[domainmcp.HostRawToolResultKey] = lossless
}

// Consuming burns the carrier even on a mismatched identity. Exact original
// context and grant distinguish concurrent same-argument calls in one turn.
func (m *ProductionManager) ConsumeHostScheduleReceiptV1(result domainmcp.LosslessToolResult, envelope domainmcp.HostContextEnvelope, outcome domainevidence.ToolOutcome) (domaintoolresult.ProtectedCaptureV1, bool) {
	carrier, ok := result.HostPrivate.(*hostScheduleReceiptCarrierV1)
	if !ok || carrier == nil {
		return domaintoolresult.ProtectedCaptureV1{}, false
	}
	carrier.mu.Lock()
	defer carrier.mu.Unlock()
	active, original, rawSHA, receipt := carrier.active, carrier.envelope, carrier.rawSHA256, carrier.receipt
	carrier.active = false
	carrier.envelope = domainmcp.HostContextEnvelope{}
	carrier.rawSHA256 = ""
	carrier.receipt = nil
	securityContext, grant, err := envelope.Authority()
	originalContext, originalGrant, originalErr := original.Authority()
	recomputed := mcpprotocol.ExtractLosslessToolResult(result.RawResult)
	if !active || m == nil || carrier.owner != m || err != nil || originalErr != nil ||
		!reflect.DeepEqual(securityContext, originalContext) || grant != originalGrant ||
		!domainmcp.ValidLosslessToolResult(result) || !domainmcp.ValidLosslessToolResult(recomputed) || rawSHA != result.RawSHA256 ||
		!reflect.DeepEqual(result.Value, recomputed.Value) || domaintoolresult.ValidateProtectedScheduleReceiptV1(receipt) != nil ||
		domainevidence.ValidateToolOutcome(outcome) != nil ||
		outcome.ToolName != grant.ToolName || outcome.ToolCallID != grant.ToolCallID || outcome.ContextDigest != securityContext.ContextDigest ||
		outcome.ExecutionGrantID != grant.GrantID || outcome.CaseID != securityContext.CaseID || outcome.ContextEpoch != securityContext.ContextEpoch ||
		outcome.DatasetSnapshotID != securityContext.DatasetSnapshotID || outcome.ServerIdentity != grant.ServerIdentity ||
		outcome.TransportStatus != "success" || string(outcome.SemanticStatus) != receipt.Status || outcome.IsError != (receipt.Status == "failure") ||
		domaintoolresult.ProtectedScheduleOperationForToolV1(grant.ToolName) != receipt.Operation {
		return domaintoolresult.ProtectedCaptureV1{}, false
	}
	// Decode the closed receipt again to detach every optional pointer from the
	// process carrier before returning it to the protected collector.
	body, err := json.Marshal(receipt)
	var copied domaintoolresult.ProtectedScheduleReceiptV1
	if err != nil || json.Unmarshal(body, &copied) != nil {
		return domaintoolresult.ProtectedCaptureV1{}, false
	}
	status := "completed"
	if outcome.IsError {
		status = "failed"
	}
	capture := domaintoolresult.ProtectedCaptureV1{Kind: "schedule", Status: status, Schedule: &copied}
	return capture, domaintoolresult.ValidateProtectedCaptureV1(capture) == nil
}

func (m *ProductionManager) DiscardHostScheduleReceiptV1(result domainmcp.LosslessToolResult) {
	if carrier, ok := result.HostPrivate.(*hostScheduleReceiptCarrierV1); ok {
		carrier.discard()
	}
}
