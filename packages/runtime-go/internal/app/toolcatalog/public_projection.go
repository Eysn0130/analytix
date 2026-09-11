package toolcatalog

import (
	"math"
	"strings"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// BuildPublicToolResultProjectionV1 is the only conversion from an in-attempt
// tool result into a value that may enter thread, event, SSE, or sidecar
// persistence. It deliberately preserves lifecycle metadata only. Raw text,
// binary data, remote authority claims, and arbitrary nested structures stay
// inside the current effect/provider attempt.
func BuildPublicToolResultProjectionV1(toolName string, output any, isError bool) domaintoolresult.PublicToolResultProjectionV1 {
	if duplicate, ok := output.(HostSideEffectDuplicateOutputV1); ok && isError && validHostSideEffectDuplicateOutputV1(duplicate) {
		return publicHostStatusProjection("blocked", "tool_blocked", duplicate.Code)
	}
	record, _ := output.(map[string]any)
	code := publicToolResultCode(toolName, record, isError)
	status, messageKey := publicToolResultStatus(code, isError)
	if MCPToolNeedsAnalytixCaseContext(toolName) {
		if projection, ok := caseSourcePublicProjectionV1(record, status); ok {
			return projection
		}
		return publicHostStatusProjection(status, messageKey, code)
	}
	if strings.TrimSpace(toolName) == ToolCreatePlanName {
		if projection, ok := planPublicProjectionV1(record, status); ok {
			return projection
		}
		return publicHostStatusProjection(status, messageKey, code)
	}
	if strings.TrimSpace(MCPToolServerID(toolName)) != "" {
		if projection, ok := mcpDiagnosticPublicProjectionV1(record, status); ok {
			return projection
		}
		return domaintoolresult.WithheldProjectionV1(status, "tool_output_private")
	}
	return publicHostStatusProjection(status, messageKey, code)
}

func publicHostStatusProjection(status string, messageKey string, code string) domaintoolresult.PublicToolResultProjectionV1 {
	return domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:          domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind:         domaintoolresult.ProjectionHostStatus,
		Disclosure:             domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:             messageKey,
		Status:                 status,
		Code:                   code,
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
	}
}

func caseSourcePublicProjectionV1(record map[string]any, status string) (domaintoolresult.PublicToolResultProjectionV1, bool) {
	if stringField(record, "code") != "case_source_result_private" {
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	executed, executedOK := record["executed"].(bool)
	isError, isErrorOK := record["isError"].(bool)
	if !executedOK || !isErrorOK {
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	messageKey := "case_source_private"
	if status != "completed" || isError {
		messageKey = "case_source_failed"
		status = "failed"
	} else if !executed {
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:          domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind:         domaintoolresult.ProjectionCaseSourceStatus,
		Disclosure:             domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:             messageKey,
		Status:                 status,
		Code:                   "case_source_result_private",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
	}
	return projection, domaintoolresult.ValidatePublicToolResultProjectionV1(projection) == nil
}

func planPublicProjectionV1(record map[string]any, status string) (domaintoolresult.PublicToolResultProjectionV1, bool) {
	if status != "completed" {
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	plan := domaintoolresult.PlanStatusV1{
		PlanID:       stringField(record, "plan_id"),
		RelativePath: stringField(record, "relative_path"),
		Operation:    stringField(record, "operation"),
		ContentHash:  stringField(record, "content_hash"),
		ByteSize:     int64Value(record["byte_size"]),
		SavedAt:      stringField(record, "saved_at"),
	}
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:          domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind:         domaintoolresult.ProjectionPlanStatus,
		Disclosure:             domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:             "plan_updated",
		Status:                 "completed",
		Code:                   "plan_updated",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
		Plan:                   &plan,
	}
	return projection, domaintoolresult.ValidatePublicToolResultProjectionV1(projection) == nil
}

func mcpDiagnosticPublicProjectionV1(record map[string]any, status string) (domaintoolresult.PublicToolResultProjectionV1, bool) {
	rpc, _ := record["rpcError"].(map[string]any)
	if rpc == nil {
		return domaintoolresult.PublicToolResultProjectionV1{}, false
	}
	diagnostic := domaintoolresult.MCPRPCErrorDiagnostic{
		Class:       stringField(rpc, "class"),
		DataPresent: boolFieldValue(rpc["dataPresent"]),
	}
	diagnostic.Code = canonicalRPCErrorCode(diagnostic.Class)
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:          domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind:         domaintoolresult.ProjectionMCPDiagnostic,
		Disclosure:             domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:             "mcp_request_rejected",
		Status:                 status,
		Code:                   "mcp_request_rejected",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
		RPCError:               &diagnostic,
	}
	return projection, domaintoolresult.ValidatePublicToolResultProjectionV1(projection) == nil
}

func publicToolResultStatus(code string, isError bool) (string, string) {
	switch code {
	case "tool_cancelled", "tool_timeout", "approval_cancelled", "user_input_cancelled":
		return "cancelled", "tool_cancelled"
	case "approval_policy_blocked", "publication_receipt_required", "case_report_publication_receipt_required", "side_effect_duplicate", "tool_not_advertised", "tool_source_unavailable":
		return "blocked", "tool_blocked"
	}
	if isError {
		return "failed", "tool_failed"
	}
	return "completed", "tool_completed"
}

func publicToolResultCode(toolName string, record map[string]any, isError bool) string {
	if strings.TrimSpace(MCPToolServerID(toolName)) != "" {
		return ""
	}
	code := strings.ToLower(strings.TrimSpace(stringField(record, "code")))
	if hostPublicToolResultCodeAllowed(code) {
		return code
	}
	if isError {
		return "tool_failed"
	}
	return "tool_completed"
}

func hostPublicToolResultCodeAllowed(code string) bool {
	switch code {
	case "approval_cancelled", "approval_denied", "approval_policy_blocked", "cancelled",
		"execution_grant_context_mismatch", "execution_grant_expired", "execution_grant_invalid",
		"loop_guard", "not_found", "publication_receipt_required", "case_report_publication_receipt_required",
		"runtime_recovered_job_interrupted", "sandbox_blocked", "side_effect_duplicate", "tool_blocked", "tool_cancelled",
		"tool_failed", "tool_not_advertised", "tool_source_unavailable", "tool_timeout",
		"user_input_cancelled", "validation_error", "workspace_escape":
		return true
	default:
		return false
	}
}

func canonicalRPCErrorCode(class string) int {
	switch strings.TrimSpace(class) {
	case "parse_error":
		return -32700
	case "invalid_request":
		return -32600
	case "method_not_found":
		return -32601
	case "invalid_params":
		return -32602
	case "internal_error":
		return -32603
	case "server_error":
		return -32000
	default:
		return 0
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case uint:
		if uint64(typed) <= uint64(^uint64(0)>>1) {
			return int64(typed)
		}
	case uint64:
		if typed <= uint64(^uint64(0)>>1) {
			return int64(typed)
		}
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) && typed >= -9223372036854775808 && typed < 9223372036854775808 && math.Trunc(typed) == typed {
			return int64(typed)
		}
	}
	return 0
}
