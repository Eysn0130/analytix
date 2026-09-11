import { z } from 'zod'

/**
 * Closed error codes that may cross the public HTTP boundary. A handler's raw
 * error text and diagnostics are private; only these codes and host-authored
 * messages have a public representation.
 */
export const PUBLIC_RUNTIME_HTTP_ERROR_CODES = [
  'validation_error',
  'unauthorized',
  'forbidden',
  'not_found',
  'conflict',
  'rate_limited',
  'method_not_allowed',
  'invalid_checkpoint_scope',
  'case_history_restricted',
  'case_compaction_archive_required',
  'thread_running',
  'gate_continuation_unavailable',
  'gate_continuation_terminal_turn',
  'task_job_output_schema_invalid',
  'worktree_isolation_authority_required',
  'attachment_authority_unavailable',
  'attachment_upload_unavailable',
  'model_modality_unsupported',
  'internal_error'
] as const

export const PublicRuntimeHTTPErrorCode = z.enum(PUBLIC_RUNTIME_HTTP_ERROR_CODES)
export type PublicRuntimeHTTPErrorCode = z.infer<typeof PublicRuntimeHTTPErrorCode>

/** Failure reasons are host-owned and never carry provider text or PII. */
export const PUBLIC_TURN_FAILURE_REASON_CODES = [
  'turn_failed',
  'turn_cancelled',
  'provider_error',
  'provider_authentication_failed',
  'provider_rate_limited',
  'provider_insufficient_balance',
  'provider_endpoint_not_found',
  'provider_request_rejected',
  'provider_unavailable',
  'provider_network_unavailable',
  'provider_timeout',
  'provider_stream_interrupted',
  'provider_stream_failed',
  'provider_model_invalid',
  'provider_not_configured',
  'provider_tool_arguments_invalid',
  'provider_reasoning_markup_invalid',
  'provider_empty_final',
  'attachment_text_fallback_too_large',
  'publication_receipt_required',
  'validation_error',
  'runtime_restarted',
  'turn_recovery_boundary',
  'approval_denied',
  'input_cancelled',
  'turn_step_limit_exceeded',
  'turn_security_authority_unavailable',
  'turn_security_context_invalid',
  'turn_security_workspace_mismatch',
  'turn_security_case_binding_mismatch',
  'turn_security_dataset_snapshot_mismatch',
  'turn_security_risk_policy_mismatch',
  'tool_call_identity_invalid',
  'tool_not_advertised',
  'tool_schema_missing',
  'tool_schema_invalid',
  'tool_private_arguments',
  'tool_source_unavailable',
  'tool_invalid_arguments_storm',
  'tool_failure_storm',
  'execution_grant_rejected',
  'source_probe_unavailable',
  'tool_pending_continuation_invalid',
  'context_window_hard_limit'
] as const

export const PublicTurnFailureReasonCode = z.enum(PUBLIC_TURN_FAILURE_REASON_CODES)
export type PublicTurnFailureReasonCode = z.infer<typeof PublicTurnFailureReasonCode>

const PublicRuntimeStandardErrorResponse = z.object({
  code: PublicRuntimeHTTPErrorCode,
  message: z.string().min(1).max(512)
}).strict()

const PublicRuntimeTurnFailureResponse = z.object({
  code: z.literal('turn_failed'),
  reasonCode: PublicTurnFailureReasonCode,
  message: z.string().min(1).max(512)
}).strict()

export const PublicRuntimeErrorResponse = z.union([
  PublicRuntimeStandardErrorResponse,
  PublicRuntimeTurnFailureResponse
])
export type PublicRuntimeErrorResponse = z.infer<typeof PublicRuntimeErrorResponse>

// Kept as the public contract name used by older consumers. It is now the
// same closed schema and no longer admits arbitrary details.
export const AnalytixErrorCode = PublicRuntimeHTTPErrorCode
export type AnalytixErrorCode = PublicRuntimeHTTPErrorCode

export const RuntimeErrorSeverity = z.enum(['info', 'warning', 'error'])
export type RuntimeErrorSeverity = z.infer<typeof RuntimeErrorSeverity>

export const AnalytixErrorBody = PublicRuntimeErrorResponse
export type AnalytixErrorBody = PublicRuntimeErrorResponse
