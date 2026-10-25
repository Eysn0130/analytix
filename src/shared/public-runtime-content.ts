import {
  PublicToolCallArgumentsProjectionV1,
  PublicToolResultProjectionV1,
  ToolCallTurnItem,
  ToolResultTurnItem
} from '../../packages/runtime/src/contracts/items.js'
import {
  AcceptedFinalDeliveryBatchV2Schema,
  GeneralTerminalDeliveryBatchV1Schema,
  GeneralTerminalErrorItemV1Schema,
  LogicalStepClosedDetailsV1Schema,
  LogicalStepStartedDetailsV1Schema,
  isToolNotAdvertisedDiagnosticDetailsV1
} from '../../packages/runtime/src/contracts/events.js'
import { PublicModelReasoningV2 } from '../../packages/runtime/src/contracts/runtime-info.js'
import { ReportedTokenCountV1Schema } from '../../packages/runtime/src/contracts/usage.js'
import { containsRestrictedEvidence } from './restricted-evidence-projection'
import {
  containsInternalCaseEntityReference,
  containsOrdinaryPublicPII,
  containsUnboundCaseRiskV1
} from './ordinary-log-pii-projection'
import { containsSecretMaterial } from './secret-redaction'

const PRIVATE_REASONING_KINDS = new Set([
  'assistant_reasoning',
  'assistant_reasoning_delta',
  'agent_reasoning'
])

const PUBLIC_REASONING_EFFORTS = new Set(['auto', 'off', 'low', 'medium', 'high', 'max'])

const PRIVATE_TERMINAL_CONTRACT_IDENTITIES = new Set([
  'general-terminal-publication.v1',
  'general-terminal-publication-archive.v1',
  'general-terminal-cas-binding.v1',
  'analytix.general-terminal-publication/v1',
  'analytix.general-terminal-publication-archive/v1',
  'analytix.general-terminal-cas-binding/v1'
])

const PRIVATE_REASONING_FIELD = /(?:["']?)(?:assistant_reasoning|assistantReasoning|reasoning_content|reasoningContent|thinking_content|thinkingContent|reasoning|thinking)(?:["']?)\s*[:=]/i

const PRIVATE_RUNTIME_DIAGNOSTIC_KEYS = new Set([
  'autocontinueerror',
  'completiondeliveryerror',
  'deliveryerror',
  'errorraw',
  'errortext',
  'evidenceledgererror',
  'outputpreview',
  'rawerror',
  'responsebody',
  'stack',
  'stacktrace',
  'stderr',
  'stdout',
  'traceback'
])

const PRIVATE_ACCEPTED_FINAL_STRONG_KEYS = new Set([
  'acceptedfinal',
  'factfinalwitnessadmission',
  'factfinalhostlocaladmission',
  'publicationsnapshotproof',
  'publicationsnapshotproofdigest'
])

const SAFE_ERROR_DETAIL_KEYS = new Set([
  'status',
  'providerId',
  'provider',
  'family',
  'model',
  'endpointFormat',
  'kind',
  'authStatus',
  'hasApiKey',
  'retryable',
  'attempt',
  'maxAttempt',
  'port',
  'backend',
  'requestedBackend',
  'autoStart',
  'phase'
])

const ENDPOINT_FORMATS = new Set(['chat_completions', 'responses', 'messages', 'custom_endpoint'])

const PROVIDER_FAILURE_STAGES = new Set([
  'request_validation', 'request_build', 'pre_send_body_audit', 'telemetry_begin',
  'transport_before_observed_send', 'transport_after_observed_send', 'local_admission_config',
  'callback_projection', 'unclassified'
])

const PROVIDER_DISPATCH_STATES = new Set(['not_sent', 'sent', 'indeterminate'])

const HOST_ERROR_CODES = new Set([
  'turn_failed', 'turn_cancelled', 'provider_error', 'provider_authentication_failed',
  'provider_rate_limited', 'provider_insufficient_balance', 'provider_endpoint_not_found',
  'provider_request_rejected', 'provider_request_failed', 'provider_request_error',
  'provider_unavailable', 'provider_network_unavailable', 'provider_timeout',
  'provider_stream_interrupted', 'provider_model_invalid', 'provider_not_configured',
  'provider_tool_arguments_invalid', 'provider_reasoning_markup_invalid', 'provider_empty_final',
  'attachment_text_fallback_too_large', 'publication_receipt_required', 'validation_error',
  'runtime_restarted', 'turn_recovery_boundary', 'approval_denied', 'input_cancelled',
  'turn_step_limit_exceeded', 'turn_security_authority_unavailable', 'turn_security_context_invalid',
  'turn_security_workspace_mismatch', 'turn_security_case_binding_mismatch',
  'turn_security_dataset_snapshot_mismatch', 'turn_security_risk_policy_mismatch',
  'tool_call_identity_invalid', 'tool_not_advertised', 'tool_schema_missing',
  'tool_schema_invalid', 'tool_private_arguments', 'tool_source_unavailable',
  'tool_invalid_arguments_storm', 'tool_failure_storm', 'execution_grant_rejected',
  'source_probe_unavailable', 'tool_pending_continuation_invalid', 'provider_stream_failed',
  'context_window_hard_limit', 'sse_setup_error'
])

const SAFE_PUBLIC_ID = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function hasOnlyKeys(record: Record<string, unknown>, allowed: Iterable<string>): boolean {
  const keys = new Set(allowed)
  return Object.keys(record).every((key) => keys.has(key))
}

function isSafeSequence(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
}

function isSafePublicId(value: unknown): value is string {
  return typeof value === 'string' && SAFE_PUBLIC_ID.test(value)
}

function isCanonicalTimestamp(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= 64 && value.trim() === value &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) &&
    !Number.isNaN(Date.parse(value))
}

function isOptionalBoolean(value: unknown): boolean {
  return value === undefined || typeof value === 'boolean'
}

function isOptionalSafeSequence(value: unknown): boolean {
  return value === undefined || isSafeSequence(value)
}

function isOptionalEnum(value: unknown, allowed: ReadonlySet<string>): boolean {
  return value === undefined || (typeof value === 'string' && allowed.has(value))
}

function isOptionalSafeIdentifier(value: unknown): boolean {
  return value === undefined || isSafePublicId(value)
}

function isClosedTrace(value: unknown): boolean {
  if (value === undefined) return true
  if (!isRecord(value) || !hasOnlyKeys(value, ['sse_sent_at', 'sse_live_emitted_at'])) return false
  return Object.keys(value).length === 2 &&
    typeof value.sse_sent_at === 'number' && Number.isFinite(value.sse_sent_at) && value.sse_sent_at >= 0 &&
    typeof value.sse_live_emitted_at === 'number' &&
    Number.isFinite(value.sse_live_emitted_at) && value.sse_live_emitted_at >= 0
}

export function isPublicHostErrorCode(value: unknown): value is string {
  return typeof value === 'string' && HOST_ERROR_CODES.has(value)
}

export function isPublicProviderEndpointFormat(value: unknown): value is string {
  return typeof value === 'string' && ENDPOINT_FORMATS.has(value)
}

export function isPublicProviderFailureStage(value: unknown): value is string {
  return typeof value === 'string' && PROVIDER_FAILURE_STAGES.has(value)
}

export function isPublicProviderDispatchState(value: unknown): value is string {
  return typeof value === 'string' && PROVIDER_DISPATCH_STATES.has(value)
}

/** Closed Host retry facts, reusable at renderer sink boundaries. No wall-clock admission. */
export function isClosedProviderRetryLifecycle(value: unknown): boolean {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'schemaVersion', 'source', 'phase', 'logicalSequence', 'outerAttempt', 'physicalAttempt',
    'delayMs', 'deadlineUnixMs', 'cancelReason'
  ]) || value.schemaVersion !== 1 ||
      ['logicalSequence', 'outerAttempt', 'physicalAttempt', 'delayMs', 'deadlineUnixMs'].some(key =>
        value[key] !== undefined && (!isSafeSequence(value[key]) || (value[key] as number) > 1e15)) ||
      !isSafeSequence(value.logicalSequence) || value.logicalSequence === 0 ||
      !isSafeSequence(value.outerAttempt) || value.outerAttempt === 0) return false
  if (value.source === 'transport_reconnect') {
    if (!isSafeSequence(value.physicalAttempt) || value.physicalAttempt === 0) return false
  } else if (value.source !== 'provider_retry' || value.physicalAttempt !== undefined) return false
  if (value.phase === 'scheduled') {
    return isSafeSequence(value.delayMs) && isSafeSequence(value.deadlineUnixMs) &&
      value.deadlineUnixMs > 0 && value.cancelReason === undefined
  }
  if (value.delayMs !== undefined || value.deadlineUnixMs !== undefined) return false
  if (value.phase === 'started') return value.cancelReason === undefined
  return value.phase === 'cancelled' && typeof value.cancelReason === 'string' && new Set([
    'context_cancelled', 'deadline_exceeded', 'attempt_rejected'
  ]).has(value.cancelReason)
}

export function isClosedProviderOutputCapObservation(value: unknown): boolean {
  return isRecord(value) && hasOnlyKeys(value, ['stopReason', 'streamCompleted', 'logicalSequence', 'toolCallCount']) &&
    value.stopReason === 'length' && typeof value.streamCompleted === 'boolean' &&
    isSafeSequence(value.logicalSequence) && value.logicalSequence > 0 && value.logicalSequence <= 1e15 &&
    (value.toolCallCount === undefined || (isSafeSequence(value.toolCallCount) && value.toolCallCount <= 1e15))
}

export function isClosedPipelineDetails(value: unknown, stage: unknown): boolean {
  if (!isRecord(value)) return false
  if (stage === 'logical_step_started' || stage === 'logical_step_assembled') return LogicalStepStartedDetailsV1Schema.safeParse(value).success
  if (stage === 'logical_step_closed') return LogicalStepClosedDetailsV1Schema.safeParse(value).success
  const allowed = [
    'visibleRecovery', 'recoveryKind', 'recoveryAttempt', 'maxRecoveryAttempt',
    'maxRecoveryAttempts', 'recoveryExhausted', 'partialToolStarted', 'maxModelSteps', 'toolName',
    'guardKind', 'stormCount', 'providerError', 'reasonCode', 'firstReasoningLatencyMs',
    'retry', 'stopReason', 'streamCompleted', 'logicalSequence', 'toolCallCount', 'logicalStepMetricsVersion'
  ]
  if (!hasOnlyKeys(value, allowed) || !isOptionalBoolean(value.visibleRecovery) ||
      !isOptionalEnum(value.recoveryKind, new Set([
        'interrupted_stream', 'step_limit_final_answer', 'empty_final'
      ])) || !isOptionalSafeSequence(value.recoveryAttempt) ||
      !isOptionalSafeSequence(value.maxRecoveryAttempt) ||
      !isOptionalSafeSequence(value.maxRecoveryAttempts) || !isOptionalBoolean(value.recoveryExhausted) ||
      !isOptionalBoolean(value.partialToolStarted) || !isOptionalSafeSequence(value.maxModelSteps) ||
      !isOptionalSafeSequence(value.stormCount) || !isOptionalSafeSequence(value.firstReasoningLatencyMs) ||
      !isOptionalSafeIdentifier(value.toolName) ||
      !isOptionalEnum(value.guardKind, new Set([
        'tool_failure', 'invalid_tool_arguments'
      ])) ||
      (value.reasonCode !== undefined &&
        (typeof value.reasonCode !== 'string' || !HOST_ERROR_CODES.has(value.reasonCode)))) return false
  if (value.logicalStepMetricsVersion !== undefined &&
      (stage !== 'post_start' || value.logicalStepMetricsVersion !== 1)) return false
  if (value.retry !== undefined && (stage !== 'provider_retrying' || value.visibleRecovery === true ||
      !isClosedProviderRetryLifecycle(value.retry))) return false
  if (['stopReason', 'streamCompleted', 'logicalSequence', 'toolCallCount'].some(key => value[key] !== undefined) &&
      (stage !== 'response_received' || !isClosedProviderOutputCapObservation({
        stopReason: value.stopReason, streamCompleted: value.streamCompleted, logicalSequence: value.logicalSequence,
        ...(value.toolCallCount !== undefined ? { toolCallCount: value.toolCallCount } : {})
      }))) return false
  if (value.providerError === undefined) return true
  if (!isRecord(value.providerError) || !hasOnlyKeys(value.providerError, [
    'kind', 'code', 'providerId', 'family', 'endpointFormat', 'model', 'authStatus', 'hasApiKey',
    'retryable', 'status', 'retryAfterMs', 'attempt', 'failureStage', 'dispatchState'
  ])) return false
  const diagnostic = value.providerError
  const kindAttributed = typeof diagnostic.kind === 'string' && new Set([
    'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
    'invalid_model', 'unknown'
  ]).has(diagnostic.kind)
  const stageAttributed = typeof diagnostic.failureStage === 'string' &&
    PROVIDER_FAILURE_STAGES.has(diagnostic.failureStage) &&
    typeof diagnostic.dispatchState === 'string' && PROVIDER_DISPATCH_STATES.has(diagnostic.dispatchState)
  return (kindAttributed || stageAttributed) &&
    (diagnostic.code === undefined ||
      (typeof diagnostic.code === 'string' && HOST_ERROR_CODES.has(diagnostic.code))) &&
    isOptionalSafeIdentifier(diagnostic.providerId) && isOptionalSafeIdentifier(diagnostic.family) &&
    isOptionalSafeIdentifier(diagnostic.model) && isOptionalEnum(diagnostic.endpointFormat, ENDPOINT_FORMATS) &&
    isOptionalEnum(diagnostic.authStatus, new Set(['none', 'possible', 'required', 'unknown'])) &&
    isOptionalBoolean(diagnostic.hasApiKey) && isOptionalBoolean(diagnostic.retryable) &&
    (diagnostic.attempt === undefined ||
      (isSafeSequence(diagnostic.attempt) && diagnostic.attempt <= 1_000_000)) &&
    isOptionalEnum(diagnostic.failureStage, PROVIDER_FAILURE_STAGES) &&
    isOptionalEnum(diagnostic.dispatchState, PROVIDER_DISPATCH_STATES) &&
    (diagnostic.status === undefined ||
      (isSafeSequence(diagnostic.status) && diagnostic.status <= 599)) &&
    (diagnostic.retryAfterMs === undefined ||
      (isSafeSequence(diagnostic.retryAfterMs) && diagnostic.retryAfterMs <= 900_000))
}

// Qualify the original complete event before treating closed Host retry
// numbers as metadata. This guard is structural and never calls a privacy
// gate, so the SSE and IPC gates can reuse it without recursion.
function isClosedProviderRetryEvent(value: unknown): value is Record<string, unknown> {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'trace',
    'stage', 'label', 'message', 'attempt', 'maxAttempt', 'child', 'details'
  ]) || value.kind !== 'pipeline_stage' || value.stage !== 'provider_retrying' ||
      value.label !== 'Retrying provider stream' || value.message !== undefined ||
      value.child !== undefined || value.itemId !== undefined ||
      !isSafeSequence(value.seq) || !isCanonicalTimestamp(value.timestamp) ||
      !isSafePublicId(value.threadId) || !isSafePublicId(value.turnId) ||
      !isClosedTrace(value.trace) || !isClosedPipelineDetails(value.details, value.stage) ||
      !isRecord(value.details) || !isRecord(value.details.retry) ||
      !hasOnlyKeys(value.details, ['retry', 'visibleRecovery', 'providerError']) ||
      !isSafeSequence(value.attempt) || value.attempt === 0 || value.attempt > 1e15 ||
      !isSafeSequence(value.maxAttempt) || value.maxAttempt < value.attempt || value.maxAttempt > 1e15) {
    return false
  }
  return value.attempt === (value.details.retry.source === 'transport_reconnect'
    ? value.details.retry.physicalAttempt : value.details.retry.outerAttempt)
}

function providerRetryPiiInspectionView(value: unknown, original: unknown = value): unknown {
  if (!isClosedProviderRetryEvent(original) || !isClosedProviderRetryEvent(value)) return value
  const details = value.details as Record<string, unknown>
  const retry = { ...(details.retry as Record<string, unknown>) }
  // Only the classifier sees zeros. Keep optional absence and original wire
  // values; arbitrary detail text and numeric fields still undergo PII checks.
  for (const key of ['logicalSequence', 'outerAttempt', 'physicalAttempt', 'delayMs', 'deadlineUnixMs']) {
    if (Object.prototype.hasOwnProperty.call(retry, key)) retry[key] = 0
  }
  return { ...value, details: { ...details, retry } }
}

/** Closed, trace-free logical metadata; this structural guard never invokes a privacy gate. */
export function isClosedLogicalStepEvent(value: unknown): value is Record<string, unknown> {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'stage', 'label', 'details'
  ]) || value.kind !== 'pipeline_stage' || !isSafeSequence(value.seq) ||
      !isCanonicalTimestamp(value.timestamp) || !isSafePublicId(value.threadId) ||
      !isSafePublicId(value.turnId)) return false
  return ((value.stage === 'logical_step_started' && value.label === 'Logical step started') ||
          (value.stage === 'logical_step_assembled' && value.label === 'Logical step assembled') ||
          (value.stage === 'logical_step_closed' && value.label === 'Logical step closed')) &&
    isClosedPipelineDetails(value.details, value.stage)
}

function logicalStepPiiInspectionView(value: unknown, original: unknown = value): unknown {
  if (!isClosedLogicalStepEvent(original) || !isClosedLogicalStepEvent(value)) return value
  const details: Record<string, unknown> = { ...(value.details as Record<string, unknown>),
    logicalStepId: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa' }
  for (const key of ['llmMs', 'ttftMs', 'decodeMs', 'decodeTokens']) {
    if (Object.prototype.hasOwnProperty.call(details, key)) details[key] = 0
  }
  return { ...value, details }
}

function closedPipelinePiiInspectionView(value: unknown, original: unknown = value): unknown {
  const retry = providerRetryPiiInspectionView(value, original)
  return retry !== value ? retry : logicalStepPiiInspectionView(value, original)
}

function isClosedSseEnvelope(value: unknown): value is {
  streamId: string
  events: Array<Record<string, unknown>>
} {
  return isRecord(value) && hasOnlyKeys(value, ['streamId', 'events']) &&
    typeof value.streamId === 'string' && value.streamId.trim() === value.streamId &&
    value.streamId.length > 0 && value.streamId.length <= 256 &&
    Array.isArray(value.events) && value.events.every(isRecord)
}

function hasUnadmittedClosedPipelineEvent(value: unknown): boolean {
  const admitted = new Set<unknown>()
  if (isClosedProviderRetryEvent(value) || isClosedLogicalStepEvent(value)) admitted.add(value)
  else if (isClosedSseEnvelope(value)) {
    for (const event of value.events) {
      if (isClosedProviderRetryEvent(event) || isClosedLogicalStepEvent(event)) admitted.add(event)
    }
  }
  const active = new WeakSet<object>()
  let remaining = 100_000
  const visit = (entry: unknown, depth: number, carrierAllowed = false, directEvents = false): boolean => {
    if (depth > 32 || remaining-- <= 0) return true
    if (!entry || typeof entry !== 'object') return false
    if (active.has(entry)) return true
    if (isRecord(entry) && entry.kind === 'pipeline_stage') {
      const logical = entry.stage === 'logical_step_started' || entry.stage === 'logical_step_assembled' ||
        entry.stage === 'logical_step_closed'
      const retry = isRecord(entry.details) && Object.prototype.hasOwnProperty.call(entry.details, 'retry') &&
        entry.details.retry !== undefined
      if (logical || retry) return !carrierAllowed || !admitted.has(entry)
    }
    active.add(entry)
    try {
      if (directEvents && Array.isArray(entry)) {
        return entry.some(child => visit(child, depth + 1, true))
      }
      return Object.entries(entry).some(([key, child]) => visit(child, depth + 1, false,
        depth === 0 && key === 'events' && isClosedSseEnvelope(value)))
    } finally { active.delete(entry) }
  }
  // Only original direct events or direct events of the closed IPC envelope
  // may carry retry or logical-step facts. Validate the complete original
  // carrier before sanitization; removing fields cannot grant admission.
  // Admission is positional too: aliasing a direct event inside a wrapper
  // must not authorize its nested occurrence. Ordinary data stays unchanged.
  return visit(value, 0, true)
}

function publicRuntimePiiInspectionView(value: unknown, original: unknown = value): unknown {
  const eventView = closedPipelinePiiInspectionView(value, original)
  if (eventView !== value) return eventView
  // Only direct events in the actual IPC envelope get this treatment, never
  // arbitrary arrays, wrappers or objects discovered by recursive traversal.
  if (!isClosedSseEnvelope(original) || !isClosedSseEnvelope(value) ||
      value.events.length !== original.events.length) return value
  return { ...value, events: value.events.map((event, index) =>
    closedPipelinePiiInspectionView(event, original.events[index])) }
}


function normalizedKey(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]/g, '')
}

function isReasoningLikeKey(key: string): boolean {
  const normalized = normalizedKey(key)
  return normalized.includes('reasoning') || normalized.includes('thinking')
}

function publicReasoningMetadataState(
  key: string,
  value: unknown
): 'unrecognized' | 'legacy-empty' | 'valid' | 'invalid' {
  switch (normalizedKey(key)) {
    case 'reasoning':
      return PublicModelReasoningV2.safeParse(value).success ? 'valid' : 'invalid'
    case 'reasoningtokens':
      return (typeof value === 'number' && Number.isSafeInteger(value) && value >= 0) ||
        ReportedTokenCountV1Schema.safeParse(value).success ? 'valid' : 'invalid'
    case 'firstreasoninglatencyms':
      return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? 'valid' : 'invalid'
    case 'reasoningeffort':
      if (value === '') return 'legacy-empty'
      return typeof value === 'string' && value === value.trim() && PUBLIC_REASONING_EFFORTS.has(value)
        ? 'valid'
        : 'invalid'
    case 'reasoningexcluded':
      return value === true ? 'valid' : 'invalid'
    case 'reasoningexclusionproof':
      return typeof value === 'string' && /^sha256:[a-f0-9]{64}$/.test(value) ? 'valid' : 'invalid'
    case 'reasoningtoken':
    case 'reasoningdurationms':
    case 'reasoningstartedat':
    case 'reasoningfinishedat':
      return 'invalid'
    default:
      return 'unrecognized'
  }
}

function hasInvalidPublicReasoningMetadata(value: unknown): boolean {
  if (Array.isArray(value)) return value.some(hasInvalidPublicReasoningMetadata)
  if (!value || typeof value !== 'object') return false
  return Object.entries(value as Record<string, unknown>).some(([key, entry]) =>
    publicReasoningMetadataState(key, entry) === 'invalid' || hasInvalidPublicReasoningMetadata(entry)
  )
}

function isPrivateTerminalAuthorityKey(key: string): boolean {
  return normalizedKey(key).startsWith('generalterminal')
}

function isPrivateTerminalAuthorityObject(record: Record<string, unknown>): boolean {
  return [record.schemaVersion, record.purpose]
    .some((value) => typeof value === 'string' && PRIVATE_TERMINAL_CONTRACT_IDENTITIES.has(value.trim()))
}

function containsPotentialReasoningMarkup(value: string): boolean {
  const lower = value.toLowerCase()
  for (let index = lower.indexOf('<'); index >= 0; index = lower.indexOf('<', index + 1)) {
    const remaining = lower.slice(index)
    for (const name of ['think', 'analysis', 'reasoning']) {
      for (const marker of [`<${name}`, `</${name}`]) {
        if (remaining.length > 1 && marker.startsWith(remaining)) return true
        if (!remaining.startsWith(marker)) continue
        if (remaining.length === marker.length) return true
        const boundary = remaining[marker.length]
        if (boundary === '>' || boundary === '/' || /[\t\n\f\r ]/.test(boundary ?? '')) return true
      }
    }
  }
  return false
}

/**
 * The Go runtime is the only reasoning-markup decoder. Desktop boundaries do
 * not try to repair or incrementally reveal provider text: a complete value
 * containing reasoning markup, restricted evidence, or a closed internal
 * reference is withheld in full, while clean bytes are preserved exactly.
 */
export function sanitizePublicAssistantText(text: string): string {
  return containsPotentialReasoningMarkup(text) || containsRestrictedEvidence(text) ||
    containsInternalCaseEntityReference(text)
    ? ''
    : text
}

export function sanitizePublicSerializedText(value: string): string {
  if (containsPotentialReasoningMarkup(value) || PRIVATE_REASONING_FIELD.test(value) ||
      containsRestrictedEvidence(value) || containsInternalCaseEntityReference(value)) return ''
  return value
}

function isAssistantTextRecord(record: Record<string, unknown>): boolean {
  const kind = typeof record.kind === 'string' ? record.kind : ''
  const role = typeof record.role === 'string' ? record.role : ''
  return role === 'assistant' || kind === 'assistant_text' || kind === 'assistant_text_delta' || kind === 'agent_message'
}

function isUserAuthoredTextRecord(record: Record<string, unknown>): boolean {
  const kind = typeof record.kind === 'string' ? record.kind : ''
  const role = typeof record.role === 'string' ? record.role : ''
  return role === 'user' || kind === 'user_message'
}

function safeErrorCode(value: unknown): string {
  if (typeof value !== 'string') return 'runtime_error'
  const code = value.trim().slice(0, 128)
  return /^[A-Za-z0-9_.-]+$/.test(code) ? code : 'runtime_error'
}

export function stripASCIIControlCharacters(value: string): string {
  let sanitized = ''
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0
    if (codePoint >= 0x20 && codePoint !== 0x7f) sanitized += character
  }
  return sanitized
}

function safeDiagnosticValue(value: unknown): string | number | boolean | undefined {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value !== 'string') return undefined
  const sanitized = stripASCIIControlCharacters(sanitizePublicAssistantText(value)).trim()
  return sanitized ? sanitized.slice(0, 256) : undefined
}

function looksLikeRuntimeError(record: Record<string, unknown>): boolean {
  return record.kind === 'error' || (
    !Object.prototype.hasOwnProperty.call(record, 'kind') &&
    typeof record.code === 'string' &&
    ('message' in record || 'details' in record || 'severity' in record)
  )
}

function acceptedFinalProjectionState(record: Record<string, unknown>): 'absent' | 'valid' | 'invalid' {
  const hasRecord = Object.prototype.hasOwnProperty.call(record, 'acceptedFinal')
  const hasView = Object.prototype.hasOwnProperty.call(record, 'acceptedFinalView')
  if (!hasRecord && !hasView) return 'absent'
  // A V3 object is a closed shape, not display authority. Generic values that
  // carry either field remain invalid; only the Batch V2 fast paths below can
  // admit an accepted-final projection without rewriting its sealed bytes.
  return 'invalid'
}

function closedPublicToolResultProjection(value: unknown): Record<string, unknown> {
  const parsed = PublicToolResultProjectionV1.safeParse(value)
  if (parsed.success) return parsed.data as Record<string, unknown>
  return {
    schemaVersion: 1,
    projectionKind: 'withheld',
    disclosure: 'metadata_only',
    status: 'unknown',
    code: 'legacy_output_withheld',
    messageKey: 'legacy_output_withheld',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }
}

function closedPublicToolCallArgumentsProjection(): Record<string, unknown> {
  return PublicToolCallArgumentsProjectionV1.parse({
    schemaVersion: 1,
    projectionKind: 'withheld',
    disclosure: 'metadata_only',
    messageKey: 'tool_arguments_withheld',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }) as Record<string, unknown>
}

const PUBLIC_TOOL_CALL_ITEM_KEYS = [
  'id',
  'turnId',
  'threadId',
  'role',
  'status',
  'createdAt',
  'finishedAt',
  'kind',
  'toolName',
  'callId',
  'toolKind',
  'contextDigest',
  'contextEpoch',
  'executionGrantId'
] as const

function closedPublicToolCallItem(record: Record<string, unknown>): Record<string, unknown> | undefined {
  const candidate: Record<string, unknown> = {}
  for (const key of PUBLIC_TOOL_CALL_ITEM_KEYS) {
    if (Object.prototype.hasOwnProperty.call(record, key)) candidate[key] = record[key]
  }
  candidate.arguments = closedPublicToolCallArgumentsProjection()
  const parsed = ToolCallTurnItem.safeParse(candidate)
  return parsed.success ? parsed.data as Record<string, unknown> : undefined
}

const PUBLIC_TOOL_RESULT_ITEM_KEYS = [
  'id',
  'turnId',
  'threadId',
  'role',
  'status',
  'createdAt',
  'finishedAt',
  'kind',
  'toolName',
  'callId',
  'toolKind',
  'isError',
  'contextDigest',
  'contextEpoch',
  'executionGrantId'
] as const

function closedPublicToolResultItem(record: Record<string, unknown>): Record<string, unknown> | undefined {
  const candidate: Record<string, unknown> = {}
  for (const key of PUBLIC_TOOL_RESULT_ITEM_KEYS) {
    if (Object.prototype.hasOwnProperty.call(record, key)) candidate[key] = record[key]
  }
  candidate.output = closedPublicToolResultProjection(record.output)
  const parsed = ToolResultTurnItem.safeParse(candidate)
  return parsed.success ? parsed.data as Record<string, unknown> : undefined
}

export function sanitizePublicRuntimeErrorEnvelope(value: unknown): Record<string, unknown> {
  if (containsRestrictedEvidence(value) || containsInternalCaseEntityReference(value)) {
    return { code: 'runtime_error', message: 'Runtime request failed.' }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return { code: 'runtime_error', message: 'Runtime request failed.' }
  }
  const record = value as Record<string, unknown>
  const closedFallback: Record<string, unknown> = {
    ...(record.kind === 'error' ? { kind: 'error' } : {}),
    code: 'runtime_error',
    message: 'Runtime request failed.'
  }
  if (GeneralTerminalErrorItemV1Schema.safeParse(record).success &&
      !containsRestrictedEvidence(record) && !containsPrivateReasoningContent(record) &&
      !containsPrivateRuntimeDiagnosticContent(record) && !containsSecretMaterial(record) &&
      !containsOrdinaryPublicPII(record) && !containsInternalCaseEntityReference(record)) {
    return record
  }
  const code = safeErrorCode(record.code)
  const fallback: Record<string, unknown> = {
    ...(record.kind === 'error' ? { kind: 'error' } : {}),
    code,
    message: `Runtime request failed (${code}).`
  }
  const sanitized: Record<string, unknown> = { ...fallback }
  for (const key of [
    'id', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'role', 'status', 'createdAt',
    'finishedAt', 'terminal', 'fatal', 'severity', 'retryable'
  ]) {
    const safe = safeDiagnosticValue(record[key])
    if (safe !== undefined) sanitized[key] = safe
  }
  const details = record.details && typeof record.details === 'object' && !Array.isArray(record.details)
    ? record.details as Record<string, unknown>
    : {}
  if (code === 'tool_not_advertised') {
    if (isToolNotAdvertisedDiagnosticDetailsV1(details)) {
      sanitized.details = { ...details }
    }
  } else {
    const safeDetails: Record<string, unknown> = {}
    for (const [key, entry] of Object.entries(details)) {
      if (!SAFE_ERROR_DETAIL_KEYS.has(key)) continue
      const safe = safeDiagnosticValue(entry)
      if (safe !== undefined) safeDetails[key] = safe
    }
    if (Object.keys(safeDetails).length > 0) sanitized.details = safeDetails
  }
  if (containsRestrictedEvidence(sanitized) || containsPrivateReasoningContent(sanitized) ||
      containsPrivateRuntimeDiagnosticContent(sanitized) || containsSecretMaterial(sanitized) ||
      containsOrdinaryPublicPII(sanitized) || containsInternalCaseEntityReference(sanitized)) return closedFallback
  return sanitized
}

/**
 * Removes content-bearing provider reasoning from runtime-shaped JSON. Numeric
 * usage counters and configuration such as reasoning effort remain intact.
 */
function sanitizePublicRuntimeValueInternal(
  value: unknown,
  inheritedRuntimeText: boolean,
  inheritedUserText = false
): unknown {
  if (Array.isArray(value)) {
    return value
      .map((entry) => sanitizePublicRuntimeValueInternal(entry, inheritedRuntimeText, inheritedUserText))
      .filter((entry) => entry !== undefined)
  }
  if (typeof value === 'string') {
    return inheritedRuntimeText ? sanitizePublicSerializedText(value) : value
  }
  if (!value || typeof value !== 'object') return value

  const record = value as Record<string, unknown>
  if (isPrivateTerminalAuthorityObject(record)) return undefined
  const kind = typeof record.kind === 'string' ? record.kind : ''
  if (PRIVATE_REASONING_KINDS.has(kind)) return undefined
  if (kind === 'tool_call') return closedPublicToolCallItem(record)
  if (kind === 'tool_result') return closedPublicToolResultItem(record)
  if (looksLikeRuntimeError(record)) return sanitizePublicRuntimeErrorEnvelope(record)
  const acceptedFinalProjection = acceptedFinalProjectionState(record)
  if (acceptedFinalProjection === 'invalid' && kind === 'assistant_text') return undefined

  const userText = inheritedUserText || isUserAuthoredTextRecord(record)
  const runtimeText = userText
    ? false
    : inheritedRuntimeText || isAssistantTextRecord(record) || Object.keys(record).length > 0
  const sanitized: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(record)) {
    const reasoningMetadataState = publicReasoningMetadataState(key, entry)
    if (reasoningMetadataState === 'legacy-empty') continue
    if ((reasoningMetadataState === 'unrecognized' && isReasoningLikeKey(key)) ||
        isPrivateTerminalAuthorityKey(key)) continue
    if (acceptedFinalProjection === 'invalid' && (key === 'acceptedFinal' || key === 'acceptedFinalView')) continue
    const child = sanitizePublicRuntimeValueInternal(entry, runtimeText, userText)
    if (child !== undefined) sanitized[key] = child
  }
  return sanitized
}

export function sanitizePublicRuntimeValue(value: unknown): unknown {
  if (hasUnadmittedClosedPipelineEvent(value)) return undefined
  if (value && typeof value === 'object' && !Array.isArray(value) &&
      looksLikeRuntimeError(value as Record<string, unknown>)) {
    return sanitizePublicRuntimeErrorEnvelope(value)
  }
  if (containsPrivateAcceptedFinalAuthority(value) ||
      containsRestrictedEvidence(value) || containsInternalCaseEntityReference(value) ||
      hasInvalidPublicReasoningMetadata(value) ||
      containsPrivateRuntimeDiagnosticContent(value)) return undefined
  const sanitized = sanitizePublicRuntimeValueInternal(value, true)
  if (containsSecretMaterial(sanitized) || containsOrdinaryPublicPII(publicRuntimePiiInspectionView(sanitized, value)) ||
      containsInternalCaseEntityReference(sanitized)) return undefined
  return sanitized
}

function containsPrivateRuntimeDiagnosticContentInternal(value: unknown, depth: number): boolean {
  if (depth > 64) return true
  if (Array.isArray(value)) {
    return value.some((entry) => containsPrivateRuntimeDiagnosticContentInternal(entry, depth + 1))
  }
  if (!value || typeof value !== 'object') return false
  return Object.entries(value as Record<string, unknown>).some(([key, entry]) =>
    PRIVATE_RUNTIME_DIAGNOSTIC_KEYS.has(normalizedKey(key)) ||
    containsPrivateRuntimeDiagnosticContentInternal(entry, depth + 1)
  )
}

/**
 * Raw provider/tool/job diagnostics are private even when they do not contain
 * a recognizable reasoning marker. Public boundaries retain only closed
 * status/reason codes and numeric/hash diagnostics.
 */
export function containsPrivateRuntimeDiagnosticContent(value: unknown): boolean {
  return containsPrivateRuntimeDiagnosticContentInternal(value, 0)
}

function containsPrivateAcceptedFinalAuthorityInternal(
  value: unknown,
  depth: number,
  active: WeakSet<object>
): boolean {
  if (depth > 64) return true
  if (!value || typeof value !== 'object') return false
  if (active.has(value)) return true
  active.add(value)
  try {
    if (Array.isArray(value)) {
      return value.some((entry) => containsPrivateAcceptedFinalAuthorityInternal(entry, depth + 1, active))
    }
    return Object.entries(value as Record<string, unknown>).some(([key, entry]) =>
      PRIVATE_ACCEPTED_FINAL_STRONG_KEYS.has(normalizedKey(key)) ||
      containsPrivateAcceptedFinalAuthorityInternal(entry, depth + 1, active)
    )
  } finally {
    active.delete(value)
  }
}

/**
 * Detects private accepted-final authority even when a strong marker is
 * detached from its normal signed record. Generic metadata names such as
 * envelope, registryHead, publicationIntent, and storeDigest remain legal.
 */
export function containsPrivateAcceptedFinalAuthority(value: unknown): boolean {
  return containsPrivateAcceptedFinalAuthorityInternal(value, 0, new WeakSet<object>())
}

function acceptedFinalProjectionTreeIsValid(value: unknown, depth = 0): boolean {
  if (depth > 32) return false
  if (Array.isArray(value)) {
    return value.every((entry) => acceptedFinalProjectionTreeIsValid(entry, depth + 1))
  }
  if (!value || typeof value !== 'object') return true
  const record = value as Record<string, unknown>
  if (record.kind === 'tool_call' && !ToolCallTurnItem.safeParse(record).success) return false
  if (record.kind === 'tool_result' && !ToolResultTurnItem.safeParse(record).success) return false
  if (acceptedFinalProjectionState(record) === 'invalid') return false
  return Object.values(record).every((entry) => acceptedFinalProjectionTreeIsValid(entry, depth + 1))
}

function treeContainsAcceptedFinalProjection(value: unknown, depth = 0): boolean {
  if (depth > 32) return true
  if (Array.isArray(value)) return value.some((entry) => treeContainsAcceptedFinalProjection(entry, depth + 1))
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  if (Object.prototype.hasOwnProperty.call(record, 'acceptedFinal') ||
      Object.prototype.hasOwnProperty.call(record, 'acceptedFinalView')) return true
  return Object.values(record).some((entry) => treeContainsAcceptedFinalProjection(entry, depth + 1))
}

function publicSseEventAcceptedProjectionIsValid(value: Record<string, unknown>): boolean {
  if (value.kind === 'accepted_final_batch') {
    return AcceptedFinalDeliveryBatchV2Schema.safeParse(value).success
  }
  if (value.kind === 'general_terminal_batch') {
    return GeneralTerminalDeliveryBatchV1Schema.safeParse(value).success
  }
  if (!acceptedFinalProjectionTreeIsValid(value)) return false
  if (!treeContainsAcceptedFinalProjection(value)) return true
  return false
}

/** Validates the context-isolated main -> preload -> renderer SSE envelope. */
export function isPublicSseIpcPayload(value: unknown): value is {
  streamId: string
  events: Array<Record<string, unknown>>
} {
  if (hasUnadmittedClosedPipelineEvent(value) || containsPrivateAcceptedFinalAuthority(value) ||
      containsRestrictedEvidence(value) || containsPrivateReasoningContent(value) ||
      containsSecretMaterial(value) || containsOrdinaryPublicPII(publicRuntimePiiInspectionView(value)) ||
      containsInternalCaseEntityReference(value)) return false
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  if (Object.keys(record).some((key) => key !== 'streamId' && key !== 'events')) return false
  if (typeof record.streamId !== 'string' || record.streamId.trim() !== record.streamId ||
      record.streamId.length === 0 || record.streamId.length > 256) return false
  if (!Array.isArray(record.events) || record.events.some((event) => !event || typeof event !== 'object' || Array.isArray(event))) {
    return false
  }
  return record.events.every((event) => publicSseEventAcceptedProjectionIsValid(event as Record<string, unknown>))
}

/** Filters complete runtime events while retaining per-item delta state. */
export class PublicRuntimeEventFilter {
  push(value: unknown): Record<string, unknown> | null {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    const payload = value as Record<string, unknown>
    if (payload.kind === 'assistant_text_delta') return null

    // Accepted-final batches are already a sealed, byte-bound public
    // projection. Sanitizing a nested terminal error would rewrite signed
    // fields and make a valid four-slot batch unverifiable. Admit only the
    // closed current schema here and preserve it exactly for main-process
    // pin/signature/hash verification.
    if (payload.kind === 'accepted_final_batch') {
      if (!AcceptedFinalDeliveryBatchV2Schema.safeParse(payload).success ||
          containsPrivateAcceptedFinalAuthority(payload) ||
          containsPrivateReasoningContent(payload) || containsRestrictedEvidence(payload) ||
          containsSecretMaterial(payload) || containsOrdinaryPublicPII(payload) ||
          containsInternalCaseEntityReference(payload)) return null
      return payload
    }

    // Ordinary terminal text is admitted only as one host-bound atomic batch.
    // Preserve the exact bytes after closed-schema and privacy validation so
    // nested error normalization cannot invalidate the Go transport digests.
    // These hashes are transport integrity only; all authority flags remain
    // literal false in the schema.
    if (payload.kind === 'general_terminal_batch') {
      const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(payload)
      if (!parsed.success ||
          containsPrivateAcceptedFinalAuthority(payload) ||
          containsPrivateReasoningContent(payload) || containsRestrictedEvidence(payload) ||
          containsSecretMaterial(payload) || containsOrdinaryPublicPII(payload) ||
          containsInternalCaseEntityReference(payload)) return null
      const itemEvent = parsed.data.events.length === 3 ? parsed.data.events[0] : undefined
      if (itemEvent?.kind === 'item_completed' && itemEvent.item.kind === 'assistant_text' &&
          'ordinaryResult' in itemEvent.item && containsUnboundCaseRiskV1(itemEvent.item.text)) return null
      return payload
    }

    const sanitized = sanitizePublicRuntimeValue(payload)
    const result = sanitized && typeof sanitized === 'object' && !Array.isArray(sanitized)
      ? sanitized as Record<string, unknown>
      : null
    if (result && 'item' in payload && !('item' in result)) return null
    if (result && !publicSseEventAcceptedProjectionIsValid(result)) return null
    return result
  }

}

function containsPrivateReasoningContentInternal(
  value: unknown,
  inheritedRuntimeText: boolean,
  inheritedUserText = false
): boolean {
  if (Array.isArray(value)) {
    return value.some((entry) => containsPrivateReasoningContentInternal(entry, inheritedRuntimeText, inheritedUserText))
  }
  if (typeof value === 'string') {
    if (!inheritedRuntimeText) return false
    return containsPotentialReasoningMarkup(value) || PRIVATE_REASONING_FIELD.test(value)
  }
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  const kind = typeof record.kind === 'string' ? record.kind : ''
  if (PRIVATE_REASONING_KINDS.has(kind)) return true
  const userText = inheritedUserText || isUserAuthoredTextRecord(record)
  const runtimeText = userText
    ? false
    : inheritedRuntimeText || isAssistantTextRecord(record) || Object.keys(record).length > 0
  for (const [key, entry] of Object.entries(record)) {
    const reasoningMetadataState = publicReasoningMetadataState(key, entry)
    if (reasoningMetadataState === 'invalid' ||
        (reasoningMetadataState === 'unrecognized' && isReasoningLikeKey(key))) return true
    if (containsPrivateReasoningContentInternal(entry, runtimeText, userText)) return true
  }
  return false
}

export function containsPrivateReasoningContent(value: unknown): boolean {
  return containsPrivateReasoningContentInternal(value, true)
}
