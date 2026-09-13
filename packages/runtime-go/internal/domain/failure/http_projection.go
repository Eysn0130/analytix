package failure

import "strings"

const (
	CodeUnauthorized    = "unauthorized"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeConflict        = "conflict"
	CodeNewTurnRequired = "new_turn_required"
	CodeRateLimited     = "rate_limited"
	CodeInternalError   = "internal_error"
)

// ProjectHTTPFailure turns an untrusted handler body into a closed public
// error envelope. Only a bounded code, host-authored message, and the exact
// hash-and-enum-only tool_not_advertised diagnostic may survive; raw causes,
// paths, commands, SQL, MCP payloads, credentials, PII, and caller identifiers
// have no representation in the result.
func ProjectHTTPFailure(status int, body any) map[string]any {
	record, _ := body.(map[string]any)
	requestedCode := strings.TrimSpace(httpFailureString(record, "code"))
	if requestedCode == "" {
		requestedCode = strings.TrimSpace(httpFailureString(record, "error"))
	}
	code := publicHTTPCode(status, requestedCode)
	newTurnBlocker := ""
	if code == CodeNewTurnRequired {
		newTurnBlocker = publicNewTurnBlocker(httpFailureString(record, "blockerCode"))
		if newTurnBlocker == "" {
			code = CodeConflict
		}
	}
	message := publicHTTPMessage(status, code)
	out := map[string]any{"code": code, "message": message}
	if newTurnBlocker != "" {
		out["blockerCode"] = newTurnBlocker
	}
	if code == CodeTurnFailed {
		reasonCode := normalizeCode(httpFailureString(record, "reasonCode"))
		out["reasonCode"] = reasonCode
		out["message"] = New(reasonCode, nil).Message()
		if details, ok := record["details"].(map[string]any); reasonCode == "tool_not_advertised" && ok && ValidateToolNotAdvertisedDetails(details) {
			out["details"] = projectDetails(details)
		}
	}
	return out
}

func publicHTTPCode(status int, requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if status >= 500 && requested == CodeTurnFailed {
		return CodeTurnFailed
	}
	switch requested {
	case "method_not_allowed", "invalid_checkpoint_scope", "case_history_restricted",
		"case_compaction_archive_required", "thread_running", "gate_continuation_unavailable",
		"gate_continuation_terminal_turn", "task_job_output_schema_invalid", "worktree_isolation_authority_required",
		"attachment_authority_unavailable", "attachment_upload_unavailable",
		"turn_execution_conflict", CodeNewTurnRequired, "runtime_shutting_down",
		"accepted_final_hydration_unavailable", "public_projection_pending", "privacy_unavailable":
		if publicHTTPSpecialCodeMatchesStatus(requested, status) {
			return requested
		}
	}
	switch status {
	case 400, 405, 411, 413, 415, 422:
		return "validation_error"
	case 401:
		return CodeUnauthorized
	case 403:
		return CodeForbidden
	case 404:
		return CodeNotFound
	case 409:
		return CodeConflict
	case 429:
		return CodeRateLimited
	default:
		if status >= 500 {
			return CodeInternalError
		}
		return CodeConflict
	}
}

func publicHTTPSpecialCodeMatchesStatus(code string, status int) bool {
	switch code {
	case "method_not_allowed":
		return status == 405
	case "invalid_checkpoint_scope":
		return status == 400
	case "case_history_restricted":
		return status == 403
	case "case_compaction_archive_required", "thread_running", "gate_continuation_unavailable", "gate_continuation_terminal_turn", "worktree_isolation_authority_required":
		return status == 409
	case "turn_execution_conflict", CodeNewTurnRequired:
		return status == 409
	case "task_job_output_schema_invalid":
		return status >= 500
	case "attachment_authority_unavailable", "attachment_upload_unavailable", "runtime_shutting_down",
		"accepted_final_hydration_unavailable", "public_projection_pending", "privacy_unavailable":
		return status == 503
	default:
		return false
	}
}

func publicHTTPMessage(status int, code string) string {
	switch code {
	case "privacy_unavailable":
		return "Media content cannot be safely projected; this operation is unavailable."
	case "validation_error", "invalid_checkpoint_scope":
		return "The request did not satisfy the runtime contract."
	case CodeUnauthorized:
		return "Runtime authentication is required."
	case CodeForbidden:
		return "The request is not authorized."
	case CodeNotFound:
		return "The requested resource was not found."
	case CodeConflict, "thread_running":
		return "The request conflicts with the current runtime state."
	case CodeNewTurnRequired:
		return "This input requires a newly admitted turn under current host security authority."
	case "turn_execution_conflict":
		return "Another terminal or security transition still owns this thread."
	case "runtime_shutting_down":
		return "The runtime is shutting down and is not accepting new turns."
	case "accepted_final_hydration_unavailable":
		return "The accepted-final snapshot could not be verified under current host authority."
	case "public_projection_pending":
		return "The thread public projection is finalizing."
	case CodeRateLimited:
		return "The runtime request rate limit was reached."
	case "method_not_allowed":
		return "The HTTP method is not allowed for this endpoint."
	case "case_history_restricted":
		return "Case history is restricted to host-verified projections."
	case "case_compaction_archive_required":
		return "Case compaction requires a verified publication archive."
	case "gate_continuation_unavailable":
		return "The pending gate continuation is unavailable."
	case "gate_continuation_terminal_turn":
		return "The pending gate continuation is unavailable because the turn is terminal."
	case "worktree_isolation_authority_required":
		return "Worktree isolation controls require host-issued durable authority."
	default:
		if status >= 500 {
			return "The runtime request could not be completed safely."
		}
		return "The request was rejected by the runtime."
	}
}

func publicNewTurnBlocker(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "case_risk_raise", "context_changing_input",
		"turn_security_context_invalid", "turn_security_workspace_mismatch",
		"turn_security_case_binding_mismatch", "turn_security_dataset_snapshot_mismatch",
		"turn_security_risk_policy_mismatch":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func httpFailureString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
