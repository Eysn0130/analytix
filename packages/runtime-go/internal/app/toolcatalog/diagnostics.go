package toolcatalog

import (
	"context"
	"errors"
	"sort"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const ToolCreatePlanName = "create_plan"

func ToolContractDiagnostics(tools []domainmodel.ToolSchema) []any {
	entries := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		providerID, providerKind := ToolContractProvider(tool)
		inputSchema := CanonicalToolParameters(tool.Parameters)
		if inputSchema == nil {
			inputSchema = map[string]any{}
		}
		entry := map[string]any{
			"name":              strings.TrimSpace(tool.Name),
			"description":       strings.TrimSpace(tool.Description),
			"inputSchema":       inputSchema,
			"toolKind":          ToolContractKind(tool.Name),
			"providerId":        providerID,
			"providerKind":      providerKind,
			"toolPolicy":        ToolContractPolicy(tool.Name),
			"providerEnabled":   true,
			"providerAvailable": true,
		}
		if outputSchema := CanonicalToolParameters(tool.OutputSchema); outputSchema != nil {
			entry["outputSchema"] = outputSchema
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		leftName, rightName := stringField(entries[i], "name"), stringField(entries[j], "name")
		if leftName != rightName {
			return leftName < rightName
		}
		return stringField(entries[i], "providerId") < stringField(entries[j], "providerId")
	})
	out := make([]any, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out
}

func ToolContractProvider(tool domainmodel.ToolSchema) (string, string) {
	switch strings.TrimSpace(tool.Source) {
	case "mcp":
		return "mcp", "mcp"
	case "plan", "user_input":
		return "gui", "gui"
	case "skill":
		return "skill", "skill"
	case "subagent", "job":
		return "delegation", "delegation"
	}
	if tool.Name == "web_fetch" {
		return "web", "web"
	}
	return "builtin", "built-in"
}

func ToolContractKind(name string) string {
	kind := ToolKind(name)
	if kind == "skill" {
		return "tool_call"
	}
	return kind
}

func ToolContractPolicy(name string) string {
	if IsUserInputTool(name) || name == ToolCreatePlanName || IsJobTool(name) {
		return "auto"
	}
	if IsMutatingTool(name) {
		return "on-request"
	}
	return "auto"
}

func ToolKind(name string) string {
	if name == "run_skill" {
		return "skill"
	}
	if name == "task" || name == "delegate_task" || name == "parallel_tasks" {
		return "subagent"
	}
	if name == ToolCreatePlanName || name == "write" || name == "write_file" || name == "edit" || name == "edit_file" || name == "multi_edit" || name == "move_file" || name == "notebook_edit" || name == "delete_range" || name == "delete_symbol" {
		return "file_change"
	}
	if name == "bash" || IsJobTool(name) {
		return "command_execution"
	}
	return "tool_call"
}

func IsUserInputTool(name string) bool {
	return name == "user_input" || name == "request_user_input"
}

// IsCompatibilityToolAlias classifies host-supported compatibility names for
// closed rejection diagnostics only. It does not advertise or authorize a
// tool and callers must separately prove that the active runtime catalog knew
// the name.
func IsCompatibilityToolAlias(name string) bool {
	switch strings.TrimSpace(name) {
	case "read_file", "write_file", "edit_file", "delegate_task", "request_user_input", "todo_patch":
		return true
	default:
		return false
	}
}

func IsJobTool(name string) bool {
	switch name {
	case "wait", "bash_output", "kill_shell", "restart_job", "list_jobs":
		return true
	default:
		return false
	}
}

func IsSubagentTool(toolName string) bool {
	switch toolName {
	case "delegate_task", "task", "parallel_tasks", "run_skill":
		return true
	default:
		return false
	}
}

func IsGoalTool(name string) bool {
	switch name {
	case "get_goal", "create_goal", "complete_step", "update_goal":
		return true
	default:
		return false
	}
}

func IsTodoTool(name string) bool {
	switch name {
	case "todo_list", "todo_write", "todo_ops", "todo_patch":
		return true
	default:
		return false
	}
}

func IsThreadStateTool(name string) bool {
	return IsGoalTool(name) || IsTodoTool(name) || name == ToolCreatePlanName || IsUserInputTool(name)
}

func IsMutatingTool(toolName string) bool {
	return toolName == ReportDeliveryToolName ||
		toolName == "write" ||
		toolName == "write_file" ||
		toolName == "edit" ||
		toolName == "edit_file" ||
		toolName == "multi_edit" ||
		toolName == "move_file" ||
		toolName == "notebook_edit" ||
		toolName == "delete_range" ||
		toolName == "delete_symbol" ||
		toolName == "bash" ||
		toolName == "delegate_task" ||
		toolName == "task" ||
		toolName == "parallel_tasks" ||
		toolName == "run_skill" ||
		strings.HasPrefix(toolName, "mcp__")
}

func RequiresApproval(toolName string, approvalPolicy string, sandboxMode string) bool {
	if IsUserInputTool(toolName) || toolName == ToolCreatePlanName {
		return false
	}
	if sandboxMode == "read-only" || sandboxMode == "external-sandbox" {
		return IsFileMutationTool(toolName)
	}
	if approvalPolicy == "never" {
		return false
	}
	if approvalPolicy == "always" {
		return true
	}
	if IsMutatingTool(toolName) {
		return approvalPolicy != "auto"
	}
	return false
}

func RequiresApprovalWithHostPolicy(toolName string, approvalPolicy string, sandboxMode string, hostReadOnly bool) bool {
	if strings.TrimSpace(toolName) == ReportDeliveryToolName {
		return strings.TrimSpace(approvalPolicy) != "never"
	}
	if approvalPolicy == "always" {
		return true
	}
	if hostReadOnly {
		return false
	}
	return RequiresApproval(toolName, approvalPolicy, sandboxMode)
}

func BlockedByApprovalNever(toolName string, mcpAvailable bool, mcpToolReadOnly bool) bool {
	if strings.TrimSpace(toolName) == ReportDeliveryToolName {
		return true
	}
	if strings.HasPrefix(toolName, "mcp__") {
		if MCPToolServerID(toolName) == "" {
			return true
		}
		// The caller must pass only host-owned read-only authority here. Remote
		// readOnlyHint metadata never reaches this policy boundary.
		return !mcpAvailable || !mcpToolReadOnly
	}
	return IsMutatingTool(toolName)
}

func ShouldRecordGenericToolProgress(toolName string) bool {
	return strings.TrimSpace(toolName) != "" && !IsUserInputTool(toolName) && !IsSubagentTool(toolName)
}

func ToolSchemaNameSet(tools []domainmodel.ToolSchema) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tool := range tools {
		out[tool.Name] = true
	}
	return out
}

func ToolNameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			set[strings.TrimSpace(name)] = true
		}
	}
	return set
}

func LiveToolNames(source any) []string {
	live, ok := source.(interface{ LiveTools() []string })
	if !ok || live == nil {
		return nil
	}
	return live.LiveTools()
}

// LiveToolNamesForSecurityContext resolves the executable tool set for one
// frozen turn. Sources without the context-aware contract may still expose
// ordinary MCP tools, but high-risk case tools are removed fail-closed.
func LiveToolNamesForSecurityContext(source any, context domainsecurity.TurnSecurityContext) []string {
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return nil
	}
	if contextual, ok := source.(interface {
		LiveToolsForSecurityContext(domainsecurity.TurnSecurityContext) []string
	}); ok && contextual != nil {
		names := contextual.LiveToolsForSecurityContext(context)
		out := make([]string, 0, len(names))
		for _, name := range names {
			if !MCPToolMayBeAdvertisedToProvider(name) {
				continue
			}
			caseTool := MCPToolNeedsAnalytixCaseContext(name)
			if MCPToolRequiresHostArtifactAuthority(name) ||
				(caseTool && (!domainsecurity.TurnSecurityContextAllowsCaseEvidence(context) ||
					!MCPToolHostReadOnly(source, name))) {
				continue
			}
			out = append(out, name)
		}
		return out
	}
	names := LiveToolNames(source)
	out := make([]string, 0, len(names))
	for _, name := range names {
		if !MCPToolMayBeAdvertisedToProvider(name) {
			continue
		}
		caseTool := MCPToolNeedsAnalytixCaseContext(name)
		if MCPToolRequiresHostArtifactAuthority(name) ||
			(caseTool && (!domainsecurity.TurnSecurityContextAllowsCaseEvidence(context) ||
				!MCPToolHostReadOnly(source, name))) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// MCPToolHostReadOnly reads the host-cached, current catalog policy used to
// issue ExecutionGrant.ReadOnly. Missing metadata is mutating fail-closed.
func MCPToolHostReadOnly(source any, toolName string) bool {
	provider, ok := source.(interface{ ToolReadOnlyHint(string) bool })
	return ok && provider != nil && provider.ToolReadOnlyHint(strings.TrimSpace(toolName))
}

func MCPConnectionEpoch(source any, toolName string) (uint64, bool) {
	provider, ok := source.(interface {
		ToolConnectionEpoch(string) (uint64, bool)
	})
	if !ok || provider == nil {
		return 0, false
	}
	return provider.ToolConnectionEpoch(strings.TrimSpace(toolName))
}

func MCPConnectionEpochs(source any, toolNames []string) map[string]uint64 {
	out := map[string]uint64{}
	for _, toolName := range toolNames {
		if epoch, ok := MCPConnectionEpoch(source, toolName); ok && epoch > 0 {
			out[strings.TrimSpace(toolName)] = epoch
		}
	}
	return out
}

func MCPServerIdentity(source any, toolName string, context domainsecurity.TurnSecurityContext) (string, bool) {
	provider, ok := source.(interface {
		ToolServerIdentity(string, domainsecurity.TurnSecurityContext) (string, bool)
	})
	if !ok || provider == nil {
		return "", false
	}
	identity, valid := provider.ToolServerIdentity(strings.TrimSpace(toolName), context)
	return strings.TrimSpace(identity), valid && strings.TrimSpace(identity) != ""
}

func MCPServerIdentities(source any, toolNames []string, context domainsecurity.TurnSecurityContext) map[string]string {
	out := map[string]string{}
	for _, toolName := range toolNames {
		if identity, ok := MCPServerIdentity(source, toolName, context); ok {
			out[strings.TrimSpace(toolName)] = identity
		}
	}
	return out
}

func ToolNotAdvertisedOutput(toolName string) map[string]any {
	return map[string]any{
		"code":  "tool_not_advertised",
		"error": toolName + " is not advertised by active tool policy",
	}
}

func ToolProgressStatus(status string) string {
	switch status {
	case "completed":
		return "success"
	case "failed", "killed", "aborted", "interrupted":
		return "error"
	default:
		return "running"
	}
}

func PersistableToolOutput(toolName string, output any) any {
	return persistableToolOutput(toolName, output, nil)
}

func PersistableToolOutputForExecution(call domainmodel.ToolCall, securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, output any) any {
	authority := &caseToolProjectionAuthority{Call: call, Context: securityContext, Grant: grant}
	return persistableToolOutput(call.Name, output, authority)
}

type caseToolProjectionAuthority struct {
	Call    domainmodel.ToolCall
	Context domainsecurity.TurnSecurityContext
	Grant   domainsecurity.ExecutionGrant
}

func persistableToolOutput(toolName string, output any, authority *caseToolProjectionAuthority) any {
	if duplicate, ok := output.(HostSideEffectDuplicateOutputV1); ok {
		if validHostSideEffectDuplicateOutputV1(duplicate) {
			return duplicate
		}
		return invalidCaseToolPublicProjection()
	}
	if MCPToolNeedsAnalytixCaseContext(toolName) {
		record, ok := output.(map[string]any)
		if !ok {
			return invalidCaseToolPublicProjection()
		}
		sanitized := cloneMap(record)
		delete(sanitized, domainmcp.HostRawToolResultKey)
		delete(sanitized, domainmcp.HostEvidenceSettlementCarrierKey)
		return caseToolPublicProjection(toolName, sanitized, authority)
	}
	record, ok := output.(map[string]any)
	if !ok {
		return output
	}
	sanitized := cloneMap(record)
	delete(sanitized, domainmcp.HostRawToolResultKey)
	delete(sanitized, domainmcp.HostEvidenceSettlementCarrierKey)
	if MCPToolServerID(toolName) != "" {
		sanitized = sanitizeMCPPublicProjection(sanitized)
		if bounded, ok := domainmcp.BoundedJSONRPCDiagnostic(record["rpcError"]); ok {
			sanitized["rpcError"] = bounded
			sanitized["error"] = "MCP JSON-RPC request was rejected (" + bounded["class"].(string) + ")"
		} else {
			delete(sanitized, "rpcError")
		}
	}
	if !IsUserInputTool(toolName) {
		return sanitized
	}
	delete(sanitized, "answers")
	if _, ok := sanitized["answerCount"]; !ok {
		if answers, ok := record["answers"].([]map[string]string); ok {
			sanitized["answerCount"] = float64(len(answers))
		}
	}
	return sanitized
}

func sanitizeMCPPublicProjection(record map[string]any) map[string]any {
	out := make(map[string]any, len(record))
	for key, value := range record {
		if key == "toolOutcome" {
			continue
		}
		if mcpPrivateProjectionKey(key) {
			continue
		}
		out[key] = sanitizeMCPPublicValue(value)
	}
	return out
}

func sanitizeMCPPublicValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if mcpPrivateProjectionKey(key) {
				continue
			}
			out[key] = sanitizeMCPPublicValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, sanitizeMCPPublicValue(child))
		}
		return out
	default:
		return value
	}
}

func mcpPrivateProjectionKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "_meta", "evidencereceipts", "evidence_receipts", "candidateevidencereceipts", "candidate_evidence_receipts",
		"untrustedmeta", "untrusted_meta", "reportedsafetoanswer", "reported_safe_to_answer", "reportedsemanticstatus",
		"reported_semantic_status", "reportedcaseid", "reported_case_id", "reportedcontextepoch", "reported_context_epoch",
		"reporteddatasetsnapshotid", "reported_dataset_snapshot_id", "reportedserveridentity", "reported_server_identity",
		"sourceassertionsauthoritative", "source_assertions_authoritative", "safetoanswer", "safe_to_answer_current_task",
		"serveridentity", "server_identity", "executiongrantid", "execution_grant_id":
		return true
	default:
		return key == domainmcp.HostRawToolResultKey || key == domainmcp.HostEvidenceSettlementCarrierKey
	}
}

func caseToolPublicProjection(toolName string, output map[string]any, authority *caseToolProjectionAuthority) map[string]any {
	projection := map[string]any{
		"executed": boolFieldValue(output["executed"]), "isError": boolFieldValue(output["isError"]),
		"code": "case_source_result_private",
	}
	outcome, err := domainevidence.ParseToolOutcome(output["toolOutcome"])
	if err == nil && caseToolOutcomeMatchesAuthority(outcome, toolName, authority) {
		projection["executed"] = outcome.TransportStatus == domainevidence.TransportSuccess
		projection["isError"] = outcome.IsError
	} else if blocked, ok := caseHostBlockProjection(output, authority); ok {
		return blocked
	} else {
		projection["executed"] = false
		projection["isError"] = true
		projection["code"] = "case_tool_outcome_invalid"
	}
	return projection
}

func caseHostBlockProjection(output map[string]any, authority *caseToolProjectionAuthority) (map[string]any, bool) {
	if !validCaseToolProjectionAuthority(authority) {
		return nil, false
	}
	code, _ := output["code"].(string)
	code = strings.TrimSpace(code)
	switch code {
	case "approval_policy_blocked", "publication_receipt_required", "case_report_publication_receipt_required", "tool_not_advertised", "tool_cancelled", "tool_timeout":
		return map[string]any{"executed": false, "isError": true, "code": code}, true
	default:
		return nil, false
	}
}

func caseToolOutcomeMatchesAuthority(outcome domainevidence.ToolOutcome, toolName string, authority *caseToolProjectionAuthority) bool {
	if !validCaseToolProjectionAuthority(authority) {
		return false
	}
	return outcome.ToolName == strings.TrimSpace(toolName) && outcome.ToolName == authority.Call.Name &&
		outcome.ToolCallID == authority.Call.ID && outcome.ToolCallID == authority.Grant.ToolCallID &&
		outcome.ContextDigest == authority.Context.ContextDigest && outcome.ContextEpoch == authority.Context.ContextEpoch &&
		outcome.DatasetSnapshotID == authority.Context.DatasetSnapshotID && outcome.CaseID == authority.Context.CaseID &&
		outcome.ExecutionGrantID == authority.Grant.GrantID && outcome.ServerIdentity == authority.Grant.ServerIdentity &&
		authority.Grant.ContextDigest == authority.Context.ContextDigest && authority.Grant.ToolName == authority.Call.Name &&
		authority.Grant.ArgsHash == domainsecurity.CanonicalJSONHash(authority.Call.Arguments)
}

func validCaseToolProjectionAuthority(authority *caseToolProjectionAuthority) bool {
	return authority != nil && domainsecurity.ValidateTurnSecurityContextForExecution(authority.Context) == nil &&
		domainsecurity.ValidateExecutionGrantForContext(authority.Grant, authority.Context) == nil &&
		authority.Context.TurnID == authority.Grant.TurnID
}

func invalidCaseToolPublicProjection() map[string]any {
	return map[string]any{"executed": false, "isError": true, "code": "case_tool_output_invalid"}
}

func boolFieldValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func CancelledOutput(toolName string, callID string, cause error) map[string]any {
	code := "tool_cancelled"
	message := "tool execution cancelled because the turn was interrupted"
	if errors.Is(cause, context.DeadlineExceeded) {
		code = "tool_timeout"
		message = "tool execution stopped because the turn context deadline was exceeded"
	}
	return map[string]any{
		"code":      code,
		"error":     message,
		"toolName":  toolName,
		"callId":    callID,
		"cancelled": true,
	}
}

func IsFileMutationTool(toolName string) bool {
	return toolName == "write" ||
		toolName == "write_file" ||
		toolName == "edit" ||
		toolName == "edit_file" ||
		toolName == "multi_edit" ||
		toolName == "move_file" ||
		toolName == "notebook_edit" ||
		toolName == "delete_range" ||
		toolName == "delete_symbol"
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func cloneMap(record map[string]any) map[string]any {
	out := make(map[string]any, len(record))
	for key, value := range record {
		out[key] = value
	}
	return out
}
