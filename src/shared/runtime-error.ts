import {
  PUBLIC_RUNTIME_HTTP_ERROR_CODES,
  PublicTurnFailureReasonCode,
  type PublicRuntimeHTTPErrorCode,
  type PublicTurnFailureReasonCode as PublicTurnFailureReasonCodeType
} from '../../packages/runtime/src/contracts/errors.js'

export type AnalytixErrorCode =
  | PublicRuntimeHTTPErrorCode
  | 'turn_in_progress'
  | 'turn_not_running'
  | 'turn_failed'
  | 'approval_not_pending'
  | 'capability_unavailable'
  | 'provider_unavailable'
  | 'policy_blocked'
  | 'attachment_validation_failed'
  | 'not_implemented'
  | 'aborted'

export type LegacyMainGuardCode =
  | 'runtime_auth_required'
  | 'runtime_request_failed'
  | 'runtime_unavailable'
  | 'fetch_failed'
  | 'runtime_offline'
  | 'runtime_port_conflict'
  | 'runtime_unhealthy'
  | 'runtime_request_user_input_unsupported'
  | 'runtime_response_schema_invalid'
  | 'runtime_response_not_public'
  | 'missing_api_key'

export type RuntimeErrorCode = AnalytixErrorCode | LegacyMainGuardCode | 'unknown'

export type RuntimeError = {
  code: RuntimeErrorCode
  message: string
  reasonCode?: PublicTurnFailureReasonCodeType
  /** @deprecated Public HTTP errors never carry arbitrary diagnostics. */
  details?: never
  /** @deprecated Public HTTP errors never carry provider payloads. */
  providerError?: never
}

const PUBLIC_HTTP_ERROR_MESSAGES: Readonly<Record<PublicRuntimeHTTPErrorCode, string>> = {
  validation_error: 'The request did not satisfy the runtime contract.',
  unauthorized: 'Runtime authentication is required.',
  forbidden: 'The request is not authorized.',
  not_found: 'The requested resource was not found.',
  conflict: 'The request conflicts with the current runtime state.',
  rate_limited: 'The runtime request rate limit was reached.',
  method_not_allowed: 'The HTTP method is not allowed for this endpoint.',
  invalid_checkpoint_scope: 'The request did not satisfy the runtime contract.',
  case_history_restricted: 'Case history is restricted to host-verified projections.',
  case_compaction_archive_required: 'Case compaction requires a verified publication archive.',
  thread_running: 'The request conflicts with the current runtime state.',
  turn_execution_conflict: 'Another terminal or security transition still owns this thread.',
  gate_continuation_unavailable: 'The pending gate continuation is unavailable.',
  gate_continuation_terminal_turn: 'The pending gate continuation is unavailable because the turn is terminal.',
  task_job_output_schema_invalid: 'The runtime request could not be completed safely.',
  worktree_isolation_authority_required: 'Worktree isolation controls require host-issued durable authority.',
  attachment_authority_unavailable: 'The runtime request could not be completed safely.',
  attachment_upload_unavailable: 'The runtime request could not be completed safely.',
  public_projection_pending: 'The thread public projection is finalizing.',
  model_modality_unsupported: 'The selected model does not support the requested modality.',
  internal_error: 'The runtime request could not be completed safely.'
}

const LEGACY_GUARD_MESSAGES: Readonly<Record<LegacyMainGuardCode, string>> = {
  runtime_auth_required: 'Runtime authentication is required.',
  runtime_request_failed: 'The runtime request could not be completed safely.',
  runtime_unavailable: 'The Analytix runtime is unavailable.',
  fetch_failed: 'The Analytix runtime is unavailable.',
  runtime_offline: 'The Analytix runtime is offline.',
  runtime_port_conflict: 'The Analytix runtime port is unavailable.',
  runtime_unhealthy: 'The Analytix runtime did not pass its health check.',
  runtime_request_user_input_unsupported: 'The runtime cannot accept user input for this request.',
  runtime_response_schema_invalid: 'Runtime response failed schema validation.',
  runtime_response_not_public: 'Runtime response was blocked at the public boundary.',
  missing_api_key: 'The selected provider credential is not configured.'
}

const ADDITIONAL_ERROR_MESSAGES: Readonly<Record<Exclude<AnalytixErrorCode, PublicRuntimeHTTPErrorCode | 'turn_failed'>, string>> = {
  turn_in_progress: 'The thread already has a turn in progress.',
  turn_not_running: 'The requested turn is not running.',
  approval_not_pending: 'The requested approval is no longer pending.',
  capability_unavailable: 'The requested runtime capability is unavailable.',
  provider_unavailable: 'The model provider is temporarily unavailable.',
  policy_blocked: 'The current host policy blocked the request.',
  attachment_validation_failed: 'The attachment did not satisfy the runtime contract.',
  not_implemented: 'The requested runtime capability is not implemented.',
  aborted: 'The runtime request was aborted.'
}

const KNOWN_HTTP_CODES = new Set<string>(PUBLIC_RUNTIME_HTTP_ERROR_CODES)
const KNOWN_ADDITIONAL_CODES = new Set<string>(Object.keys(ADDITIONAL_ERROR_MESSAGES))
const KNOWN_LEGACY_CODES = new Set<string>(Object.keys(LEGACY_GUARD_MESSAGES))
const DEFAULT_PUBLIC_RUNTIME_ERROR_MESSAGE = 'The runtime request could not be completed safely.'

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function normalizedString(value: unknown): string {
  return typeof value === 'string' ? value.trim().toLowerCase() : ''
}

function normalizeCode(value: unknown): RuntimeErrorCode {
  const code = normalizedString(value)
  if (code === 'turn_failed' || KNOWN_HTTP_CODES.has(code) || KNOWN_ADDITIONAL_CODES.has(code)) {
    return code as AnalytixErrorCode
  }
  if (KNOWN_LEGACY_CODES.has(code)) return code as LegacyMainGuardCode
  return 'unknown'
}

function normalizeTurnFailureReason(value: unknown): PublicTurnFailureReasonCodeType {
  const parsed = PublicTurnFailureReasonCode.safeParse(normalizedString(value))
  return parsed.success ? parsed.data : 'turn_failed'
}

export function publicTurnFailureMessage(code: PublicTurnFailureReasonCodeType): string {
  switch (code) {
    case 'turn_cancelled':
      return 'The turn was cancelled before a verified response was available.'
    case 'provider_authentication_failed':
      return 'Provider authentication failed. Check the configured credential.'
    case 'provider_rate_limited':
      return 'The provider rate limit was reached. Retry after the bounded delay.'
    case 'provider_insufficient_balance':
      return 'The provider account balance or credit is insufficient.'
    case 'provider_endpoint_not_found':
      return 'The model endpoint was not found. Check Provider Base URL and Endpoint format.'
    case 'provider_request_rejected':
      return 'The provider rejected the model request. Response content was withheld.'
    case 'provider_unavailable':
      return 'The model provider is temporarily unavailable.'
    case 'provider_network_unavailable':
      return 'The model provider could not be reached.'
    case 'provider_timeout':
      return 'The provider request timed out before a verified response was available.'
    case 'provider_stream_interrupted':
    case 'provider_stream_failed':
      return 'The provider stream was interrupted before a verified response was available.'
    case 'provider_model_invalid':
      return 'The selected model is not configured for this provider.'
    case 'provider_not_configured':
      return 'The selected provider is not configured for this runtime.'
    case 'provider_tool_arguments_invalid':
      return 'The provider supplied incomplete or invalid tool arguments; execution was blocked.'
    case 'provider_reasoning_markup_invalid':
      return 'The provider returned invalid private-reasoning markup; the response was blocked.'
    case 'provider_empty_final':
      return 'The provider returned no final response after the bounded recovery attempt.'
    case 'attachment_text_fallback_too_large':
      return 'The attachment cannot be represented within the bounded text fallback.'
    case 'runtime_restarted':
      return 'The runtime restarted before this turn completed. The turn was marked aborted so the thread can continue.'
    case 'turn_recovery_boundary':
      return 'The bounded recovery path ended without a verified final response.'
    case 'approval_denied':
      return 'The requested operation was denied; no unverified final response was published.'
    case 'input_cancelled':
      return 'The requested input was cancelled; no unverified final response was published.'
    case 'turn_step_limit_exceeded':
      return 'The turn reached its configured model-step limit.'
    case 'tool_not_advertised':
      return 'The provider requested a tool that was not advertised for this turn.'
    case 'tool_call_identity_invalid':
      return 'The provider supplied an invalid or duplicate tool-call identity.'
    case 'tool_private_arguments':
      return 'The provider supplied private reasoning in tool arguments; execution was blocked.'
    case 'tool_source_unavailable':
    case 'source_probe_unavailable':
      return 'The required source is not available for the current run.'
    case 'publication_receipt_required':
      return 'Formal artifact publication requires current host publication authority.'
    case 'tool_invalid_arguments_storm':
      return 'Repeated invalid tool arguments caused the host to stop the turn.'
    case 'tool_failure_storm':
      return 'Repeated tool failures caused the host to stop the turn.'
    case 'validation_error':
      return 'The request did not satisfy the current host contract.'
    case 'context_window_hard_limit':
      return 'The bounded request still exceeds the model context window after safe compaction; no provider request was sent.'
    default:
      if (code.startsWith('turn_security_') || code.startsWith('execution_grant_')) {
        return 'Current host security authority rejected the operation.'
      }
      if (code.startsWith('tool_schema_')) {
        return 'The tool schema is unavailable or invalid for the current turn.'
      }
      if (code === 'tool_pending_continuation_invalid') {
        return 'Pending tool continuation failed current host revalidation.'
      }
      if (code.startsWith('tool_')) {
        return 'The host rejected or could not complete the tool operation.'
      }
      return 'The turn failed before a verified response was available.'
  }
}

function publicHTTPErrorCode(status: number, requestedCode: unknown): PublicRuntimeHTTPErrorCode | 'turn_failed' {
  const requested = normalizedString(requestedCode)
  if (status >= 500 && requested === 'turn_failed') return 'turn_failed'
  if (KNOWN_HTTP_CODES.has(requested) && publicHTTPSpecialCodeMatchesStatus(requested, status)) {
    return requested as PublicRuntimeHTTPErrorCode
  }
  if ([400, 405, 411, 413, 415, 422].includes(status)) return 'validation_error'
  if (status === 401) return 'unauthorized'
  if (status === 403) return 'forbidden'
  if (status === 404) return 'not_found'
  if (status === 409) return 'conflict'
  if (status === 429) return 'rate_limited'
  return status >= 500 ? 'internal_error' : 'conflict'
}

function publicHTTPSpecialCodeMatchesStatus(code: string, status: number): boolean {
  switch (code) {
    case 'method_not_allowed':
      return status === 405
    case 'invalid_checkpoint_scope':
      return status === 400
    case 'case_history_restricted':
      return status === 403
    case 'case_compaction_archive_required':
    case 'thread_running':
    case 'turn_execution_conflict':
    case 'gate_continuation_unavailable':
    case 'gate_continuation_terminal_turn':
    case 'worktree_isolation_authority_required':
      return status === 409
    case 'task_job_output_schema_invalid':
      return status >= 500
    case 'attachment_authority_unavailable':
    case 'attachment_upload_unavailable':
      return status === 503
    case 'public_projection_pending':
      return status === 503
    default:
      return false
  }
}

export function projectPublicRuntimeHTTPError(status: number, value: unknown): RuntimeError {
  const record = recordValue(value)
  const code = publicHTTPErrorCode(status, record.code ?? record.error)
  if (code === 'turn_failed') {
    const reasonCode = normalizeTurnFailureReason(record.reasonCode)
    return { code, reasonCode, message: publicTurnFailureMessage(reasonCode) }
  }
  return { code, message: PUBLIC_HTTP_ERROR_MESSAGES[code] }
}

function canonicalRuntimeError(code: RuntimeErrorCode, reasonCode?: unknown): RuntimeError {
  if (code === 'turn_failed') {
    const normalizedReason = normalizeTurnFailureReason(reasonCode)
    return { code, reasonCode: normalizedReason, message: publicTurnFailureMessage(normalizedReason) }
  }
  if (KNOWN_HTTP_CODES.has(code)) {
    return { code, message: PUBLIC_HTTP_ERROR_MESSAGES[code as PublicRuntimeHTTPErrorCode] }
  }
  if (KNOWN_ADDITIONAL_CODES.has(code)) {
    return { code, message: ADDITIONAL_ERROR_MESSAGES[code as keyof typeof ADDITIONAL_ERROR_MESSAGES] }
  }
  if (KNOWN_LEGACY_CODES.has(code)) {
    return { code, message: LEGACY_GUARD_MESSAGES[code as LegacyMainGuardCode] }
  }
  return { code: 'unknown', message: DEFAULT_PUBLIC_RUNTIME_ERROR_MESSAGE }
}

/** Parse only the closed public identity of a runtime failure. */
export function parseRuntimeErrorBody(body: string, _fallback: string): RuntimeError {
  let parsed: unknown
  try {
    parsed = JSON.parse(body)
  } catch {
    return canonicalRuntimeError('unknown')
  }
  const record = recordValue(parsed)
  const code = normalizeCode(record.code ?? record.error)
  return canonicalRuntimeError(code, record.reasonCode)
}

/** Serialize only a closed code/message pair; arbitrary diagnostics are dropped. */
export function runtimeErrorToError(error: RuntimeError): Error {
  const canonical = canonicalRuntimeError(normalizeCode(error.code), error.reasonCode)
  return canonical.code === 'unknown'
    ? new Error(canonical.message)
    : new Error(JSON.stringify(canonical))
}

export function isKnownAnalytixErrorCode(value: unknown): value is AnalytixErrorCode {
  const code = normalizeCode(value)
  return code !== 'unknown' && !KNOWN_LEGACY_CODES.has(code)
}

export function isLegacyMainGuardCode(value: unknown): value is LegacyMainGuardCode {
  return KNOWN_LEGACY_CODES.has(normalizedString(value))
}
