import {
  containsPrivateAcceptedFinalAuthority,
  isPublicSseIpcPayload,
  PublicRuntimeEventFilter
} from './public-runtime-content'
import {
  AcceptedFinalDeliveryBatchV2Schema,
  GeneralTerminalDeliveryBatchV1Schema,
  PublicProjectionRevokedEvent as PublicProjectionRevokedEventSchema
} from '../../packages/runtime/src/contracts/events.js'
import type { PublicProjectionRevokedEvent } from '../../packages/runtime/src/contracts/events.js'
import {
  TaskContinuationSnapshotV1Schema,
  ToolCallTurnItem,
  ToolProgressTurnItem,
  ToolResultTurnItem
} from '../../packages/runtime/src/contracts/items.js'
import {
  containsInternalCaseEntityReference,
  containsProtectedCaseFactCandidate
} from './ordinary-log-pii-projection'

export type { PublicProjectionRevokedEvent } from '../../packages/runtime/src/contracts/events.js'

export const PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODES = Object.freeze([
  'accepted_final_authority_pin_origin_mismatch',
  'accepted_final_authority_pin_unavailable',
  'accepted_final_authority_unavailable',
  'accepted_final_batch_invalid',
  'accepted_final_authority_invalid',
  'general_terminal_batch_invalid',
  'general_terminal_batch_public_tree_invalid',
  'general_terminal_batch_private_reasoning',
  'general_terminal_batch_restricted_evidence',
  'general_terminal_batch_secret_material',
  'general_terminal_batch_ordinary_pii',
  'general_terminal_batch_json_invalid',
  'general_terminal_batch_schema_invalid',
  'general_terminal_batch_ordinary_result_invalid',
  'general_terminal_batch_event_binding_invalid',
  'general_terminal_batch_integrity_invalid',
  'general_terminal_batch_verification_failed',
  'malformed_frame',
  'missing_transport_binding',
  'event_kind_mismatch',
  'event_sequence_mismatch',
  'event_thread_mismatch',
  'unsupported_event_kind',
  'invalid_public_projection'
] as const)

export type PublicRuntimeSseRejectionReasonCode =
  typeof PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODES[number]

const PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODE_SET = new Set<string>(
  PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODES
)

export function isPublicRuntimeSseRejectionReasonCode(
  value: unknown
): value is PublicRuntimeSseRejectionReasonCode {
  return typeof value === 'string' && PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODE_SET.has(value)
}

export type PublicRuntimeSseDecision =
  | { status: 'emit'; event: Record<string, unknown>; seq: number }
  | { status: 'revoke'; event: PublicProjectionRevokedEvent }
  | { status: 'withheld'; seq: number; reason: 'restricted_content' }
  | {
      status: 'invalid'
      reason:
        | 'malformed_frame'
        | 'missing_transport_binding'
        | 'event_kind_mismatch'
        | 'event_sequence_mismatch'
        | 'event_thread_mismatch'
        | 'unsupported_event_kind'
        | 'invalid_public_projection'
    }

const BASE_EVENT_KEYS = [
  'kind',
  'seq',
  'timestamp',
  'threadId',
  'turnId',
  'itemId',
  'trace'
] as const

type PublicEventContract = Readonly<{
  keys: readonly string[]
  validate: (event: Record<string, unknown>) => boolean
}>

function eventContract(
  keys: readonly string[],
  validate: (event: Record<string, unknown>) => boolean
): PublicEventContract {
  return { keys, validate }
}

/**
 * Ordinary live SSE is a closed discriminated contract. Text-bearing fields
 * are absent unless a validator below proves a fixed host phrase or a typed
 * user-message item. Accepted finals use their separately sealed batch.
 */
const PUBLIC_EVENT_CONTRACTS: Readonly<Record<string, PublicEventContract>> = {
  accepted_final_batch: eventContract([
    'schemaVersion', 'purpose', 'batchId', 'firstSeq', 'lastSeq', 'publicationCommitId',
    'eventManifestDigest', 'publicationAuthority', 'events'
  ], () => false),
  general_terminal_batch: eventContract([
    'schemaVersion', 'purpose', 'batchDigest', 'firstSeq', 'lastSeq',
    'generalTerminalCommitId', 'generalTerminalAuthorityKind',
    'generalTerminalAuthorityDigest', 'eventManifestDigest', 'projectedEventsDigest',
    'transportAuthority', 'evidenceAuthority', 'citationAuthority', 'factAnswerAllowed',
    'events', 'eventManifest'
  ], () => false),
  thread_created: eventContract(['status'], validateThreadCreatedEvent),
  thread_updated: eventContract(['status'], validateThreadUpdatedEvent),
  thread_archived: eventContract(['status'], validateThreadArchivedEvent),
  thread_rewound: eventContract([
    'rewindTurnId', 'removedTurns', 'remainingTurns', 'removedTurnIds'
  ], validateThreadRewoundEvent),
  turn_started: eventContract([
    'status', 'clientUserMessageId', 'admittedSeq', 'terminalReason', 'discard', 'cancelled',
    'model', 'providerId', 'approvalPolicy', 'sandboxMode', 'mode', 'disableUserInput',
    'maxModelSteps', 'workspaceCheckpointId'
  ], validateTurnLifecycleEvent),
  turn_completed: eventContract([
    'status', 'clientUserMessageId', 'admittedSeq', 'terminalReason', 'discard', 'cancelled',
    'model', 'providerId', 'approvalPolicy', 'sandboxMode', 'mode', 'disableUserInput',
    'maxModelSteps', 'workspaceCheckpointId'
  ], validateTurnLifecycleEvent),
  turn_failed: eventContract([
    'status', 'clientUserMessageId', 'admittedSeq', 'terminalReason', 'discard', 'cancelled',
    'model', 'providerId', 'approvalPolicy', 'sandboxMode', 'mode', 'disableUserInput',
    'maxModelSteps', 'workspaceCheckpointId'
  ], validateTurnLifecycleEvent),
  turn_aborted: eventContract([
    'status', 'clientUserMessageId', 'admittedSeq', 'terminalReason', 'discard', 'cancelled',
    'model', 'providerId', 'approvalPolicy', 'sandboxMode', 'mode', 'disableUserInput',
    'maxModelSteps', 'workspaceCheckpointId'
  ], validateTurnLifecycleEvent),
  turn_steered: eventContract([
    'clientUserMessageId', 'admittedSeq', 'model', 'providerId', 'approvalPolicy',
    'sandboxMode', 'mode', 'disableUserInput', 'maxModelSteps', 'workspaceCheckpointId'
  ], validateTurnLifecycleEvent),
  item_created: eventContract([
    'item', 'stage', 'callId', 'toolName', 'status', 'message', 'child', 'details'
  ], validateItemLifecycleEvent),
  item_updated: eventContract([
    'item', 'stage', 'callId', 'toolName', 'status', 'message', 'child', 'details'
  ], validateItemLifecycleEvent),
  item_completed: eventContract([
    'item', 'stage', 'callId', 'toolName', 'status', 'message', 'child', 'details'
  ], validateItemLifecycleEvent),
  tool_call_started: eventContract(['item'], validateItemLifecycleEvent),
  tool_call_finished: eventContract(['item'], validateItemLifecycleEvent),
  tool_call_ready: eventContract(['toolName', 'callId', 'readyCount', 'partial'], validateToolCallReadyEvent),
  tool_progress: eventContract([
    'toolName', 'callId', 'status', 'message', 'stage', 'child', 'details'
  ], validateToolProgressEvent),
  tool_result_upload_wait: eventContract(['status', 'toolResultCount'], validateToolUploadWaitEvent),
  tool_storm_suppressed: eventContract(['toolName', 'callId', 'message'], validateToolStormEvent),
  tool_catalog_changed: eventContract([
    'fingerprint', 'toolCount', 'changeKind', 'toolNames', 'message'
  ], validateToolCatalogEvent),
  child_steer_queued: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'steerMessageId',
    'parentThreadId', 'sourceTurnId', 'sourceToolCallId', 'createdAt', 'admittedAt'
  ], validateChildSteerEvent),
  child_steer_admitted: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'steerMessageId',
    'parentThreadId', 'sourceTurnId', 'sourceToolCallId', 'createdAt', 'admittedAt'
  ], validateChildSteerEvent),
  child_steer_rejected: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'steerMessageId',
    'parentThreadId', 'sourceTurnId', 'sourceToolCallId', 'createdAt', 'admittedAt'
  ], validateChildSteerEvent),
  child_pause_requested: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'pauseRequestId',
    'parentThreadId', 'sourceTurnId', 'createdAt', 'pausedAt', 'resumedAt'
  ], validateChildPauseEvent),
  child_paused: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'pauseRequestId',
    'parentThreadId', 'sourceTurnId', 'createdAt', 'pausedAt', 'resumedAt'
  ], validateChildPauseEvent),
  child_resume_requested: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'pauseRequestId',
    'parentThreadId', 'sourceTurnId', 'createdAt', 'pausedAt', 'resumedAt'
  ], validateChildPauseEvent),
  child_resumed: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'pauseRequestId',
    'parentThreadId', 'sourceTurnId', 'createdAt', 'pausedAt', 'resumedAt'
  ], validateChildPauseEvent),
  child_pause_rejected: eventContract([
    'status', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'pauseRequestId',
    'parentThreadId', 'sourceTurnId', 'createdAt', 'pausedAt', 'resumedAt'
  ], validateChildPauseEvent),
  approval_requested: eventContract([
    'approvalId', 'toolName', 'status', 'approvalPolicy', 'sandboxMode'
  ], validateApprovalEvent),
  approval_resolved: eventContract([
    'approvalId', 'toolName', 'status', 'approvalPolicy', 'sandboxMode'
  ], validateApprovalEvent),
  user_input_requested: eventContract(['inputId', 'status'], validateUserInputEvent),
  user_input_resolved: eventContract(['inputId', 'status'], validateUserInputEvent),
  compaction_started: eventContract(['auto'], validateCompactionEvent),
  compaction_completed: eventContract([
    'summary', 'replacedTokens', 'auto', 'pinnedConstraints', 'sourceDigest', 'digestMarker', 'sourceItemIds',
    'schemaVersion', 'reasoningExcluded', 'reasoningExclusionProof'
  ], validateCompactionEvent),
  goal_updated: eventContract(['goal', 'cleared'], validateGoalEvent),
  goal_cleared: eventContract(['goal', 'cleared'], validateGoalEvent),
  goal_evidence_audit: eventContract([
    'schemaVersion', 'changeId', 'runtimeContract', 'upstreamSource', 'goalId', 'result',
    'recovered', 'missingProjectChecks', 'incompleteTodos', 'commandMismatchMissing',
    'latestWriterReceiptIndex', 'blockedStateKey', 'missingCheckIds',
    'usesReasonixPublicProtocol', 'usesReasonixConfigRoot', 'changesRendererContract',
    'changesProductIdentity'
  ], validateGoalEvidenceAuditEvent),
  autoresearch_state_audit: eventContract([
    'schemaVersion', 'changeId', 'runtimeContract', 'upstreamSource', 'goalMode',
    'fileCount', 'requirementCount', 'completedRequirementCount', 'staleRequirementCount',
    'staleDirectionCount', 'complete',
    'pivotRequired', 'result', 'unknownRequirementAccepted',
    'findingsWrittenForUnknownRequirement', 'writesReasonixFile', 'writesAgentsFile',
    'stablePrefixContainsState', 'toolSchemaContainsState', 'topLevelAutoResearchRouteExposed',
    'usesReasonixPublicProtocol', 'usesReasonixConfigRoot', 'changesRendererContract',
    'changesProductIdentity'
  ], validateAutoresearchAuditEvent),
  todos_updated: eventContract(['todos', 'cleared'], validateTodosEvent),
  todos_cleared: eventContract(['todos', 'cleared'], validateTodosEvent),
  checkpoint_captured: eventContract(['checkpoint'], validateCheckpointEvent),
  checkpoint_rewind_rescue_created: eventContract(['rescue'], validateCheckpointEvent),
  checkpoint_rewind_applied: eventContract(['apply'], validateCheckpointEvent),
  pipeline_stage: eventContract([
    'stage', 'label', 'message', 'attempt', 'maxAttempt', 'child', 'details'
  ], validatePipelineEvent),
  usage: eventContract([
    'model', 'providerId', 'endpointFormat', 'effort', 'usageSource', 'childRunId', 'usage',
    'cacheDiagnostics'
  ], validateUsageEvent),
  error: eventContract([
    'message', 'code', 'severity', 'terminal', 'fatal', 'recoverable', 'details'
  ], validateErrorEvent),
  heartbeat: eventContract([], validateHeartbeatEvent),
  cursor_advanced: eventContract(['reason'], validateCursorAdvancedEvent),
  snapshot_required: eventContract([
    'sinceSeq', 'highestSeq', 'replayEventCount', 'reason'
  ], validateSnapshotRequiredEvent)
}

const PRIVATE_EVENT_KINDS = new Set([
  'assistant_text_delta',
  'assistant_reasoning',
  'assistant_reasoning_delta',
  'agent_reasoning'
])

const CHECKPOINT_STATUS_KEYS = new Set([
  'schemaVersion', 'projectionKind', 'disclosure', 'status', 'checkpointRefDigest',
  'privatePayloadWithheld', 'factAnswerAllowed', 'evidenceAuthority', 'changedFileCount',
  'snapshotStorage', 'fileCount', 'scope', 'destructive', 'conversationStatus', 'summary'
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function hasOnlyKeys(record: Record<string, unknown>, allowed: Iterable<string>): boolean {
  const keys = new Set(allowed)
  return Object.keys(record).every((key) => keys.has(key))
}

function isExactNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.trim() === value
}

function isSafeSequence(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
}

const SAFE_PUBLIC_ID = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/
const SAFE_TOOL_NAME = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/
const SHA256_HEX = /^[a-f0-9]{64}$/
const SHA256_MARKER = /^sha256:[a-f0-9]{12,64}$/

const APPROVAL_POLICIES = new Set(['always', 'on-request', 'untrusted', 'never', 'auto', 'suggest'])
const SANDBOX_MODES = new Set(['read-only', 'workspace-write', 'danger-full-access', 'external-sandbox'])
const ENDPOINT_FORMATS = new Set(['chat_completions', 'responses', 'messages', 'custom_endpoint'])
const PROVIDER_FAILURE_STAGES = new Set([
  'request_validation', 'request_build', 'pre_send_body_audit', 'telemetry_begin',
  'transport_before_observed_send', 'transport_after_observed_send', 'local_admission_config',
  'callback_projection', 'unclassified'
])
const PROVIDER_DISPATCH_STATES = new Set(['not_sent', 'sent', 'indeterminate'])
const TERMINAL_REASONS = new Set([
  'success', 'source_unavailable', 'semantic_failure', 'provider_failure', 'cancel', 'timeout',
  'stream_abort', 'recovery', 'approval', 'user_input', 'resume', 'restart', 'report_fallback',
  'step_limit', 'background_completion', 'tool_failure', 'approval_denied', 'input_cancelled'
])
const JOB_STATUSES = new Set([
  'queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming',
  'completed', 'failed', 'aborted', 'killed', 'interrupted', 'canceled', 'timeout', 'unknown'
])
const DELIVERY_STATUSES = new Set(['pending', 'retry', 'delivered', 'skipped', 'dead_letter', 'unknown'])
const AUTO_CONTINUE_STATUSES = new Set(['starting', 'started', 'skipped', 'failed', 'unknown'])
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

const PIPELINE_LABEL_BY_STAGE: Readonly<Record<string, string>> = {
  setup: 'Setup',
  pre_start: 'Pre-Start',
  post_start: 'Post-Start',
  input_received: 'Input Received',
  input_cached: 'Input Cached',
  input_routed: 'Input Routed',
  input_compressed: 'Input Compressed',
  input_remembered: 'Input Remembered',
  pre_send: 'Pre-Send',
  post_send: 'Post-Send',
  response_received: 'Response Received',
  provider_retrying: 'Retrying provider stream',
  step_limit_finalizing: 'Model step budget reached',
  provider_error: 'Provider stream failed',
  empty_final_recovered: 'Provider returned an empty final response',
  loop_guard: 'Loop guard',
  unknown: 'Pipeline stage unavailable'
}

const GENERAL_COMPACTION_SUMMARY =
  'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.'
const CASE_COMPACTION_SUMMARY =
  'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.'

function isSafePublicId(value: unknown): value is string {
  return typeof value === 'string' && SAFE_PUBLIC_ID.test(value)
}

function isSafeToolName(value: unknown): value is string {
  return typeof value === 'string' && SAFE_TOOL_NAME.test(value)
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

function isOptionalSafePublicId(value: unknown): boolean {
  return value === undefined || isSafePublicId(value)
}

function isOptionalEnum(value: unknown, allowed: ReadonlySet<string>): boolean {
  return value === undefined || (typeof value === 'string' && allowed.has(value))
}

function isOptionalSafeIdentifier(value: unknown): boolean {
  return value === undefined || isSafePublicId(value)
}

function isSafeIdList(value: unknown): boolean {
  return Array.isArray(value) && value.length <= 512 && value.every(isSafePublicId) &&
    new Set(value).size === value.length
}

function eventHasTurn(event: Record<string, unknown>): boolean {
  return isSafePublicId(event.turnId) && event.itemId === undefined
}

function eventHasTurnWithOptionalItem(event: Record<string, unknown>): boolean {
  return isSafePublicId(event.turnId) &&
    (event.itemId === undefined || isSafePublicId(event.itemId))
}

function eventHasNoTurnOrItem(event: Record<string, unknown>): boolean {
  return event.turnId === undefined && event.itemId === undefined
}

function validateThreadCreatedEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && event.status === 'idle'
}

function validateThreadUpdatedEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && typeof event.status === 'string' &&
    new Set(['idle', 'running', 'archived', 'deleted']).has(event.status)
}

function validateThreadArchivedEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && event.status === 'archived'
}

function validateThreadRewoundEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && isSafePublicId(event.rewindTurnId) &&
    isSafeSequence(event.removedTurns) && isSafeSequence(event.remainingTurns) &&
    isSafeIdList(event.removedTurnIds)
}

function validateTurnLifecycleEvent(event: Record<string, unknown>): boolean {
  if (!eventHasTurn(event) || !isOptionalSafePublicId(event.clientUserMessageId) ||
      !isOptionalSafeSequence(event.admittedSeq) || !isOptionalSafeIdentifier(event.model) ||
      !isOptionalSafeIdentifier(event.providerId) || !isOptionalEnum(event.approvalPolicy, APPROVAL_POLICIES) ||
      !isOptionalEnum(event.sandboxMode, SANDBOX_MODES) ||
      !isOptionalEnum(event.mode, new Set(['agent', 'plan'])) || !isOptionalBoolean(event.disableUserInput) ||
      !isOptionalSafeSequence(event.maxModelSteps) || !isOptionalSafePublicId(event.workspaceCheckpointId) ||
      !isOptionalEnum(event.terminalReason, TERMINAL_REASONS) || !isOptionalBoolean(event.discard) ||
      !isOptionalBoolean(event.cancelled)) return false
  // Completed/failed/aborted terminal records are delivered only inside an
  // accepted-final or general-terminal atomic batch. A detached lifecycle
  // suffix must never advance the renderer past a missing item/usage prefix.
  if (event.kind === 'turn_completed' || event.kind === 'turn_failed' || event.kind === 'turn_aborted') {
    return false
  }
  const expectedStatus: Readonly<Record<string, string | undefined>> = {
    turn_started: 'running', turn_completed: 'completed', turn_failed: 'failed',
    turn_aborted: 'aborted', turn_steered: undefined
  }
  return event.status === expectedStatus[event.kind as string]
}

const ITEM_BASE_KEYS = [
  'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt', 'kind'
] as const

function isClosedItemBase(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  return isSafePublicId(event.turnId) && isSafePublicId(event.itemId) &&
    item.threadId === event.threadId && item.turnId === event.turnId && item.id === event.itemId &&
    isCanonicalTimestamp(item.createdAt) &&
    (item.finishedAt === undefined || isCanonicalTimestamp(item.finishedAt))
}

function isClosedUserMessageItem(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  const allowed = [
    ...ITEM_BASE_KEYS, 'text', 'displayText', 'delivery', 'clientUserMessageId', 'attachmentIds',
    'workspaceCheckpointId'
  ]
  return hasOnlyKeys(item, allowed) && isClosedItemBase(event, item) && item.role === 'user' &&
    item.status === 'completed' && typeof item.text === 'string' && item.text.length <= 1_000_000 &&
    (item.displayText === undefined || typeof item.displayText === 'string') &&
    (item.delivery === undefined || item.delivery === 'steer') &&
    isOptionalSafePublicId(item.clientUserMessageId) && isOptionalSafePublicId(item.workspaceCheckpointId) &&
    (item.attachmentIds === undefined || isSafeIdList(item.attachmentIds))
}

function isClosedErrorItem(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  const allowed = [...ITEM_BASE_KEYS, 'message', 'code', 'severity']
  return hasOnlyKeys(item, allowed) && isClosedItemBase(event, item) && item.role === 'system' &&
    new Set(['completed', 'failed', 'aborted']).has(String(item.status)) &&
    isClosedHostError(item.code, item.message) && isOptionalEnum(item.severity, new Set(['info', 'warning', 'error']))
}

function isClosedToolProgressItem(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  return isClosedItemBase(event, item) && ToolProgressTurnItem.safeParse(item).success
}

function isClosedApprovalItem(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  return hasOnlyKeys(item, [...ITEM_BASE_KEYS, 'approvalId', 'toolName']) &&
    isClosedItemBase(event, item) && item.role === 'tool' && isSafePublicId(item.approvalId) &&
    isSafeToolName(item.toolName) && new Set(['pending', 'allowed', 'denied', 'expired']).has(String(item.status))
}

function isClosedUserInputItem(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  return hasOnlyKeys(item, [...ITEM_BASE_KEYS, 'inputId']) && isClosedItemBase(event, item) &&
    item.role === 'system' && isSafePublicId(item.inputId) &&
    new Set(['pending', 'submitted', 'cancelled']).has(String(item.status))
}

function isClosedCompactionItem(event: Record<string, unknown>, item: Record<string, unknown>): boolean {
  const allowed = [
    ...ITEM_BASE_KEYS, 'summary', 'replacedTokens', 'auto', 'sourceDigest', 'digestMarker',
    'sourceItemIds', 'pinnedConstraints', 'schemaVersion', 'reasoningExcluded', 'reasoningExclusionProof',
    'assistantProseExcluded', 'toolPayloadsExcluded', 'caseFactsExcluded',
    'providerHistoryProjectionVersion', 'caseHistoryProjectionVersion', 'taskContinuation'
  ]
  const continuationV4 = item.schemaVersion === 4
  const publicCaseV2 = item.schemaVersion === 3 && item.caseHistoryProjectionVersion === 2
  return hasOnlyKeys(item, allowed) && isClosedItemBase(event, item) && item.role === 'system' &&
    item.status === 'completed' &&
    item.summary === (publicCaseV2 ? CASE_COMPACTION_SUMMARY : GENERAL_COMPACTION_SUMMARY) &&
    (publicCaseV2 ? item.replacedTokens === undefined : isSafeSequence(item.replacedTokens)) &&
    isOptionalBoolean(item.auto) &&
    Array.isArray(item.pinnedConstraints) && item.pinnedConstraints.length === 1 &&
    item.pinnedConstraints[0] === 'user: preserve recent turns' &&
    (item.sourceDigest === undefined || (typeof item.sourceDigest === 'string' && SHA256_HEX.test(item.sourceDigest))) &&
    (item.digestMarker === undefined || (typeof item.digestMarker === 'string' && SHA256_MARKER.test(item.digestMarker))) &&
    (item.sourceItemIds === undefined || isSafeIdList(item.sourceItemIds)) &&
    (item.schemaVersion === undefined || item.schemaVersion === 3 || continuationV4) &&
    (item.reasoningExcluded === undefined || item.reasoningExcluded === true) &&
    (item.reasoningExclusionProof === undefined ||
      (typeof item.reasoningExclusionProof === 'string' && /^sha256:[a-f0-9]{64}$/.test(item.reasoningExclusionProof))) &&
    (item.assistantProseExcluded === undefined || item.assistantProseExcluded === true) &&
    (item.toolPayloadsExcluded === undefined || item.toolPayloadsExcluded === true) &&
    (item.caseFactsExcluded === undefined || item.caseFactsExcluded === true) &&
    (item.providerHistoryProjectionVersion === undefined || item.providerHistoryProjectionVersion === 1 ||
      (continuationV4 && item.providerHistoryProjectionVersion === 2)) &&
    (item.caseHistoryProjectionVersion === undefined || item.caseHistoryProjectionVersion === 1 || publicCaseV2) &&
    (!publicCaseV2 || (typeof item.auto === 'boolean' &&
      item.finishedAt === item.createdAt &&
      item.providerHistoryProjectionVersion === undefined && typeof item.sourceDigest === 'string' &&
      SHA256_HEX.test(item.sourceDigest) && item.digestMarker === `sha256:${item.sourceDigest.slice(0, 12)}` &&
      item.reasoningExcluded === true && typeof item.reasoningExclusionProof === 'string' &&
      /^sha256:[a-f0-9]{64}$/.test(item.reasoningExclusionProof) && Array.isArray(item.sourceItemIds) &&
      item.sourceItemIds.length === 0 && item.assistantProseExcluded === true &&
      item.toolPayloadsExcluded === true && item.caseFactsExcluded === true)) &&
    (continuationV4
      ? item.auto === true && TaskContinuationSnapshotV1Schema.safeParse(item.taskContinuation).success
      : item.taskContinuation === undefined)
}

function isClosedItem(event: Record<string, unknown>): boolean {
  if (!isRecord(event.item)) return false
  const item = event.item
  switch (item.kind) {
    case 'user_message':
      return isClosedUserMessageItem(event, item)
    case 'tool_call':
      return isClosedItemBase(event, item) && ToolCallTurnItem.safeParse(item).success
    case 'tool_result': {
      if (!isClosedItemBase(event, item)) return false
      return ToolResultTurnItem.safeParse(item).success
    }
    case 'approval':
      return isClosedApprovalItem(event, item)
    case 'user_input':
      return isClosedUserInputItem(event, item)
    case 'compaction':
      return isClosedCompactionItem(event, item)
    case 'error':
      return isClosedErrorItem(event, item)
    case 'tool_progress':
      return isClosedToolProgressItem(event, item)
    default:
      // Assistant/review prose is never an ordinary live item. Accepted
      // assistant output is carried only by a sealed accepted-final batch.
      return false
  }
}

function validateItemLifecycleEvent(event: Record<string, unknown>): boolean {
  if (!isSafePublicId(event.turnId)) return false
  if (event.item !== undefined) {
    if (event.stage !== undefined || event.callId !== undefined || event.toolName !== undefined ||
        event.status !== undefined || event.message !== undefined || event.child !== undefined ||
        event.details !== undefined || !isClosedItem(event)) return false
    const item = event.item as Record<string, unknown>
    // A completed host error is part of an accepted-final or general-terminal
    // suffix. Exposing the same item as a detached ordinary frame would let a
    // spoofed/legacy stream advance the desktop cursor without the matching
    // usage and terminal disposition.
    if (item.kind === 'error') return false
    if (event.kind === 'tool_call_started') return item.kind === 'tool_call' && item.status === 'running'
    if (event.kind === 'tool_call_finished') {
      return item.kind === 'tool_result' && new Set(['completed', 'failed']).has(String(item.status))
    }
    return new Set(['item_created', 'item_updated', 'item_completed']).has(String(event.kind))
  }
  if (!new Set(['item_created', 'item_updated', 'item_completed']).has(String(event.kind)) ||
      event.message !== 'child output withheld' || !isOptionalSafePublicId(event.callId) ||
      !isOptionalSafeIdentifier(event.toolName) || !isOptionalEnum(event.status, JOB_STATUSES) ||
      (event.stage !== undefined && !isClosedChildStage(event.stage)) ||
      (event.itemId !== undefined && !isSafePublicId(event.itemId))) return false
  const child = event.child !== undefined && isClosedChildMetadata(event.child)
  const details = event.details !== undefined && isClosedChildMetadata(event.details)
  return Boolean(child || details)
}

function validateToolCallReadyEvent(event: Record<string, unknown>): boolean {
  return eventHasTurnWithOptionalItem(event) &&
    isSafeToolName(event.toolName) && isSafePublicId(event.callId) &&
    typeof event.readyCount === 'number' && Number.isSafeInteger(event.readyCount) && event.readyCount > 0 &&
    isOptionalBoolean(event.partial)
}

function validateToolProgressEvent(event: Record<string, unknown>): boolean {
  if (!eventHasTurnWithOptionalItem(event) ||
      !isSafeToolName(event.toolName) || !isSafePublicId(event.callId) ||
      !new Set(['running', 'success', 'error', 'unknown']).has(String(event.status))) return false
  const hasChild = event.child !== undefined || event.details !== undefined
  if (!hasChild) return event.message === undefined && event.stage === undefined
  if (event.message !== 'child output withheld' || (event.stage !== undefined && !isClosedChildStage(event.stage))) return false
  return (event.child === undefined || isClosedChildMetadata(event.child)) &&
    (event.details === undefined || isClosedChildMetadata(event.details))
}

function validateToolUploadWaitEvent(event: Record<string, unknown>): boolean {
  return eventHasTurn(event) && event.status === 'waiting' && isSafeSequence(event.toolResultCount)
}

function validateToolStormEvent(event: Record<string, unknown>): boolean {
  return eventHasTurn(event) && isSafeToolName(event.toolName) && isSafePublicId(event.callId) &&
    (event.message === undefined || event.message === 'Repeated tool activity was suppressed.')
}

function validateToolCatalogEvent(event: Record<string, unknown>): boolean {
  if (event.turnId !== undefined || event.itemId !== undefined || !isSafePublicId(event.fingerprint) ||
      !isSafeSequence(event.toolCount) || !isOptionalEnum(event.changeKind, new Set(['additive', 'breaking'])) ||
      (event.message !== undefined && event.message !== 'MCP tool catalog changed after refresh.')) return false
  if (event.toolNames === undefined) return true
  if (!Array.isArray(event.toolNames)) return false
  const toolNames = event.toolNames
  return toolNames.length === event.toolCount && toolNames.every(isSafeToolName) &&
    new Set(toolNames).size === toolNames.length &&
    toolNames.every((name, index) => index === 0 || String(toolNames[index - 1]).localeCompare(name) < 0)
}

function validateChildSteerEvent(event: Record<string, unknown>): boolean {
  const statusByKind: Readonly<Record<string, string>> = {
    child_steer_queued: 'queued', child_steer_admitted: 'admitted', child_steer_rejected: 'rejected'
  }
  return eventHasTurn(event) && event.status === statusByKind[event.kind as string] &&
    isSafePublicId(event.jobId) && isSafePublicId(event.childRunId) &&
    isSafePublicId(event.steerMessageId) && isSafePublicId(event.parentThreadId) &&
    isOptionalSafePublicId(event.childThreadId) && isOptionalSafePublicId(event.childTurnId) &&
    isOptionalSafePublicId(event.sourceTurnId) && isOptionalSafePublicId(event.sourceToolCallId) &&
    (event.createdAt === undefined || isCanonicalTimestamp(event.createdAt)) &&
    (event.admittedAt === undefined || isCanonicalTimestamp(event.admittedAt))
}

function validateChildPauseEvent(event: Record<string, unknown>): boolean {
  const statuses: Readonly<Record<string, ReadonlySet<string>>> = {
    child_pause_requested: new Set(['requested']), child_paused: new Set(['paused']),
    child_resume_requested: new Set(['resume_requested']), child_resumed: new Set(['resumed']),
    child_pause_rejected: new Set(['rejected', 'expired'])
  }
  return eventHasTurn(event) && typeof event.status === 'string' &&
    Boolean(statuses[event.kind as string]?.has(event.status)) && isSafePublicId(event.jobId) &&
    isSafePublicId(event.childRunId) && isSafePublicId(event.pauseRequestId) &&
    isSafePublicId(event.parentThreadId) && isOptionalSafePublicId(event.childThreadId) &&
    isOptionalSafePublicId(event.childTurnId) && isOptionalSafePublicId(event.sourceTurnId) &&
    (event.createdAt === undefined || isCanonicalTimestamp(event.createdAt)) &&
    (event.pausedAt === undefined || isCanonicalTimestamp(event.pausedAt)) &&
    (event.resumedAt === undefined || isCanonicalTimestamp(event.resumedAt))
}

function validateApprovalEvent(event: Record<string, unknown>): boolean {
  const allowed = event.kind === 'approval_requested'
    ? new Set(['pending'])
    : new Set(['allowed', 'denied', 'expired'])
  return eventHasTurn(event) && isSafePublicId(event.approvalId) && isSafeToolName(event.toolName) &&
    typeof event.status === 'string' && allowed.has(event.status) &&
    isOptionalEnum(event.approvalPolicy, APPROVAL_POLICIES) && isOptionalEnum(event.sandboxMode, SANDBOX_MODES)
}

function validateUserInputEvent(event: Record<string, unknown>): boolean {
  const allowed = event.kind === 'user_input_requested'
    ? new Set(['pending'])
    : new Set(['submitted', 'cancelled'])
  return eventHasTurn(event) && isSafePublicId(event.inputId) &&
    typeof event.status === 'string' && allowed.has(event.status)
}

function validateCompactionEvent(event: Record<string, unknown>): boolean {
  if (!eventHasTurn(event)) return false
  if (event.kind === 'compaction_started') return event.auto === undefined || typeof event.auto === 'boolean'
  const caseCompaction = event.summary === CASE_COMPACTION_SUMMARY
  return (event.summary === GENERAL_COMPACTION_SUMMARY || caseCompaction) &&
    (caseCompaction ? event.replacedTokens === undefined : isSafeSequence(event.replacedTokens)) &&
    isOptionalBoolean(event.auto) && typeof event.sourceDigest === 'string' && SHA256_HEX.test(event.sourceDigest) &&
    Array.isArray(event.pinnedConstraints) && event.pinnedConstraints.length === 1 &&
    event.pinnedConstraints[0] === 'user: preserve recent turns' &&
    event.digestMarker === `sha256:${event.sourceDigest.slice(0, 12)}` &&
    (event.sourceItemIds === undefined || isSafeIdList(event.sourceItemIds)) &&
    (!caseCompaction || (Array.isArray(event.sourceItemIds) && event.sourceItemIds.length === 0 &&
      typeof event.auto === 'boolean')) && event.schemaVersion === 2 &&
    event.reasoningExcluded === true && typeof event.reasoningExclusionProof === 'string' &&
    /^sha256:[a-f0-9]{64}$/.test(event.reasoningExclusionProof)
}

function validateGoalEvent(event: Record<string, unknown>): boolean {
  // Goal prose may be provider/tool authored and has no live origin proof yet.
  // Clear notifications are metadata-only; updates remain available through
  // the separately validated thread snapshot until a typed origin is added.
  return eventHasNoTurnOrItem(event) && event.kind === 'goal_cleared' &&
    event.cleared === true && (event.goal === undefined || event.goal === null)
}

function validateTodosEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && event.kind === 'todos_cleared' &&
    event.cleared === true && (event.todos === undefined || event.todos === null)
}

function validateGoalEvidenceAuditEvent(event: Record<string, unknown>): boolean {
  const booleans = [
    'recovered', 'usesReasonixPublicProtocol', 'usesReasonixConfigRoot',
    'changesRendererContract', 'changesProductIdentity'
  ]
  return eventHasTurn(event) && event.schemaVersion === 1 &&
    event.changeId === 'goal-evidence-audit' && event.runtimeContract === 'analytix-go-runtime' &&
    event.upstreamSource === 'reasonix-absorbed' && new Set(['allowed', 'blocked', 'errored']).has(String(event.result)) &&
    booleans.every((key) => typeof event[key] === 'boolean') &&
    ['missingProjectChecks', 'incompleteTodos', 'commandMismatchMissing', 'latestWriterReceiptIndex']
      .every((key) => isSafeSequence(event[key])) && isOptionalSafePublicId(event.goalId) &&
    isOptionalSafePublicId(event.blockedStateKey) &&
    (event.missingCheckIds === undefined || isSafeIdList(event.missingCheckIds))
}

function validateAutoresearchAuditEvent(event: Record<string, unknown>): boolean {
  const booleanKeys = [
    'complete', 'pivotRequired', 'unknownRequirementAccepted', 'findingsWrittenForUnknownRequirement',
    'writesReasonixFile', 'writesAgentsFile', 'stablePrefixContainsState', 'toolSchemaContainsState',
    'topLevelAutoResearchRouteExposed', 'usesReasonixPublicProtocol', 'usesReasonixConfigRoot',
    'changesRendererContract', 'changesProductIdentity'
  ]
  if (!(eventHasTurn(event) && event.schemaVersion === 1 &&
    event.changeId === 'autoresearch-state-audit' && event.runtimeContract === 'analytix-go-runtime' &&
    event.upstreamSource === 'reasonix-absorbed' && event.goalMode === 'research' &&
    event.fileCount === 5 && ['requirementCount', 'completedRequirementCount', 'staleRequirementCount', 'staleDirectionCount']
      .every((key) => isSafeSequence(event[key])) && booleanKeys.every((key) => typeof event[key] === 'boolean') &&
    new Set(['created', 'resumed', 'pivot_required', 'complete', 'errored']).has(String(event.result)) &&
    event.unknownRequirementAccepted === false && event.findingsWrittenForUnknownRequirement === false &&
    event.writesReasonixFile === false && event.writesAgentsFile === false &&
    event.stablePrefixContainsState === false && event.toolSchemaContainsState === false &&
    event.topLevelAutoResearchRouteExposed === false && event.usesReasonixPublicProtocol === false &&
    event.usesReasonixConfigRoot === false && event.changesRendererContract === false &&
    event.changesProductIdentity === false)) return false
  const requirementCount = event.requirementCount as number
  const completedCount = event.completedRequirementCount as number
  const staleCount = event.staleRequirementCount as number
  const staleDirectionCount = event.staleDirectionCount as number
  const result = String(event.result)
  if (completedCount + staleCount !== requirementCount ||
      event.complete !== (result === 'complete') ||
      event.pivotRequired !== (result === 'pivot_required')) return false
  if (result === 'complete') {
    return requirementCount > 0 && completedCount === requirementCount && staleCount === 0
  }
  if (result === 'pivot_required') return staleCount > 0 && staleDirectionCount > 0
  return result === 'created' || result === 'resumed' || result === 'errored'
}

function validateCheckpointEvent(event: Record<string, unknown>): boolean {
  if (event.kind === 'checkpoint_captured') {
    return eventHasTurn(event) && isClosedCheckpointStatus(event.checkpoint)
  }
  if (!eventHasNoTurnOrItem(event)) return false
  return event.kind === 'checkpoint_rewind_rescue_created'
    ? isClosedCheckpointStatus(event.rescue)
    : isClosedCheckpointStatus(event.apply)
}

function pipelineLabel(stage: unknown): string | null {
  if (typeof stage !== 'string') return null
  if (PIPELINE_LABEL_BY_STAGE[stage]) return PIPELINE_LABEL_BY_STAGE[stage]
  if (stage.startsWith('subagent_') && JOB_STATUSES.has(stage.slice('subagent_'.length))) {
    return 'Subagent lifecycle update'
  }
  if (stage.startsWith('background_job_delivery_') && DELIVERY_STATUSES.has(stage.slice('background_job_delivery_'.length))) {
    return 'Background job delivery update'
  }
  if (stage.startsWith('background_job_auto_continue_') && AUTO_CONTINUE_STATUSES.has(stage.slice('background_job_auto_continue_'.length))) {
    return 'Background job continuation update'
  }
  if (stage.startsWith('background_job_') && JOB_STATUSES.has(stage.slice('background_job_'.length))) {
    return 'Background job lifecycle update'
  }
  return null
}

function isClosedChildStage(value: unknown): boolean {
  return value === 'child_lifecycle_unknown' || pipelineLabel(value) !== null
}

function validatePipelineEvent(event: Record<string, unknown>): boolean {
  if (!eventHasTurn(event) || !isOptionalSafeSequence(event.attempt) || !isOptionalSafeSequence(event.maxAttempt)) return false
  const hasChild = event.child !== undefined || (event.details !== undefined && isClosedChildMetadata(event.details))
  if (hasChild) {
    return event.message === 'child output withheld' && isClosedChildStage(event.stage) &&
      event.label === undefined && (event.child === undefined || isClosedChildMetadata(event.child)) &&
      (event.details === undefined || isClosedChildMetadata(event.details))
  }
  const label = pipelineLabel(event.stage)
  return label !== null && event.label === label && event.message === undefined &&
    (event.details === undefined || isClosedPipelineDetails(event.details))
}

function isClosedUsageSnapshot(value: unknown): boolean {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'promptTokens', 'completionTokens', 'reasoningTokens', 'totalTokens', 'cachedTokens',
    'cacheHitTokens', 'cacheMissTokens', 'cacheHitRate', 'cacheableTokenHitRate',
    'totalInputTokenHitRate', 'turns', 'priceConfigured', 'costUsd', 'costCny',
    'cacheSavingsUsd', 'cacheSavingsCny', 'tokenEconomySavingsTokens',
    'tokenEconomySavingsUsd', 'tokenEconomySavingsCny', 'hasError'
  ])) return false
  for (const key of [
    'promptTokens', 'completionTokens', 'totalTokens', 'turns', 'reasoningTokens', 'cachedTokens',
    'cacheHitTokens', 'cacheMissTokens', 'tokenEconomySavingsTokens'
  ]) {
    if (key === 'promptTokens' || key === 'completionTokens' || key === 'totalTokens' || key === 'turns') {
      if (!isSafeSequence(value[key])) return false
    } else if (!isOptionalSafeSequence(value[key])) return false
  }
  for (const key of ['cacheHitRate', 'cacheableTokenHitRate', 'totalInputTokenHitRate']) {
    const rate = value[key]
    if (key === 'cacheHitRate' && rate === undefined) return false
    if (rate !== undefined && rate !== null && (typeof rate !== 'number' || !Number.isFinite(rate) || rate < 0 || rate > 1)) return false
  }
  for (const key of ['costUsd', 'costCny', 'cacheSavingsUsd', 'cacheSavingsCny', 'tokenEconomySavingsUsd', 'tokenEconomySavingsCny']) {
    const amount = value[key]
    if (amount !== undefined && (typeof amount !== 'number' || !Number.isFinite(amount) || amount < 0)) return false
  }
  return isOptionalBoolean(value.priceConfigured) && isOptionalBoolean(value.hasError)
}

function isClosedCacheDiagnostics(value: unknown): boolean {
  if (!isRecord(value)) return false
  const allowed = [
    'prefixHash', 'prefixChanged', 'prefixChangeReasons', 'toolSourceChanged', 'toolSourceChangeReasons',
    'systemHash', 'modeHash', 'prefixItemsHash', 'toolsHash', 'toolSchemaTokens', 'toolCount',
    'toolSourcesHash', 'toolSourceIds', 'route', 'cacheTelemetrySupported', 'firstTokenLatencyMs',
    'durationMs', 'cacheHitTokens', 'cacheMissTokens', 'terminalCacheDiagnosticsSchema',
    'terminalCacheDiagnosticsValid', 'terminalCacheDiagnosticsDisposition'
  ]
  if (!hasOnlyKeys(value, allowed)) return false
  for (const key of ['prefixHash', 'systemHash', 'modeHash', 'prefixItemsHash', 'toolsHash', 'toolSourcesHash']) {
    const digest = value[key]
    if (digest !== undefined && (typeof digest !== 'string' || !SHA256_HEX.test(digest))) return false
  }
  for (const key of ['prefixChanged', 'toolSourceChanged', 'cacheTelemetrySupported', 'terminalCacheDiagnosticsValid']) {
    if (!isOptionalBoolean(value[key])) return false
  }
  for (const key of ['toolSchemaTokens', 'toolCount', 'firstTokenLatencyMs', 'durationMs', 'cacheHitTokens', 'cacheMissTokens']) {
    if (!isOptionalSafeSequence(value[key])) return false
  }
  if (value.prefixChangeReasons !== undefined && (!Array.isArray(value.prefixChangeReasons) || value.prefixChangeReasons.length !== 0)) return false
  if (value.toolSourceChangeReasons !== undefined && (!Array.isArray(value.toolSourceChangeReasons) || value.toolSourceChangeReasons.length !== 0)) return false
  if (value.toolSourceIds !== undefined && (!Array.isArray(value.toolSourceIds) || !value.toolSourceIds.every(isSafePublicId))) return false
  if (!isOptionalEnum(value.route, new Set(['direct_answer', 'light_agent', 'tool_agent', 'subagent_agent'])) ||
      (value.terminalCacheDiagnosticsSchema !== undefined && value.terminalCacheDiagnosticsSchema !== 'terminal-cache-diagnostics.v1') ||
      (value.terminalCacheDiagnosticsDisposition !== undefined && value.terminalCacheDiagnosticsDisposition !== 'rejected')) return false
  return true
}

function validateUsageEvent(event: Record<string, unknown>): boolean {
  if (!eventHasTurn(event) || !isOptionalSafeIdentifier(event.model) ||
      !isOptionalSafeIdentifier(event.providerId) || !isOptionalEnum(event.endpointFormat, ENDPOINT_FORMATS) ||
      !isOptionalEnum(event.effort, new Set(['auto', 'off', 'low', 'medium', 'high', 'max'])) ||
      !isOptionalSafeIdentifier(event.usageSource) || !isOptionalSafePublicId(event.childRunId) ||
      !isClosedUsageSnapshot(event.usage) ||
      (event.cacheDiagnostics !== undefined && !isClosedCacheDiagnostics(event.cacheDiagnostics))) return false
  // Production usage is terminal telemetry. It is public only inside the
  // accepted-final or general-terminal atomic batch that binds the exact turn
  // disposition. A detached legacy/spoofed usage frame is never authoritative.
  return false
}

function fixedHostErrorMessage(code: string): string {
  const messages: Readonly<Record<string, string>> = {
    turn_cancelled: 'The turn was cancelled before a verified response was available.',
    provider_authentication_failed: 'Provider authentication failed. Check the configured credential.',
    provider_rate_limited: 'The provider rate limit was reached. Retry after the bounded delay.',
    provider_insufficient_balance: 'The provider account balance or credit is insufficient.',
    provider_endpoint_not_found: 'The model endpoint was not found. Check Provider Base URL and Endpoint format.',
    provider_request_rejected: 'The provider rejected the model request. Response content was withheld.',
    provider_unavailable: 'The model provider is temporarily unavailable.',
    provider_network_unavailable: 'The model provider could not be reached.',
    provider_timeout: 'The provider request timed out before a verified response was available.',
    provider_stream_interrupted: 'The provider stream was interrupted before a verified response was available.',
    provider_model_invalid: 'The selected model is not configured for this provider.',
    provider_not_configured: 'The selected provider is not configured for this runtime.',
    provider_tool_arguments_invalid: 'The provider supplied incomplete or invalid tool arguments; execution was blocked.',
    provider_reasoning_markup_invalid: 'The provider returned invalid private-reasoning markup; the response was blocked.',
    provider_empty_final: 'The provider returned no final response after the bounded recovery attempt.',
    attachment_text_fallback_too_large: 'The attachment cannot be represented within the bounded text fallback.',
    runtime_restarted: 'The runtime restarted before this turn completed. The turn was marked aborted so the thread can continue.',
    turn_recovery_boundary: 'The bounded recovery path ended without a verified final response.',
    approval_denied: 'The requested operation was denied; no unverified final response was published.',
    input_cancelled: 'The requested input was cancelled; no unverified final response was published.',
    turn_step_limit_exceeded: 'The turn reached its configured model-step limit.',
    tool_not_advertised: 'The provider requested a tool that was not advertised for this turn.',
    tool_call_identity_invalid: 'The provider supplied an invalid or duplicate tool-call identity.',
    tool_private_arguments: 'The provider supplied private reasoning in tool arguments; execution was blocked.',
    tool_source_unavailable: 'The required source is not available for the current run.',
    source_probe_unavailable: 'The required source is not available for the current run.',
    publication_receipt_required: 'Formal artifact publication requires current host publication authority.',
    tool_invalid_arguments_storm: 'Repeated invalid tool arguments caused the host to stop the turn.',
    tool_failure_storm: 'Repeated tool failures caused the host to stop the turn.',
    validation_error: 'The request did not satisfy the current host contract.',
    context_window_hard_limit: 'The bounded request still exceeds the model context window after safe compaction; no provider request was sent.',
    sse_setup_error: 'SSE setup failed'
  }
  if (messages[code]) return messages[code]
  if (code.startsWith('turn_security_') || code.startsWith('execution_grant_')) {
    return 'Current host security authority rejected the operation.'
  }
  if (code.startsWith('tool_schema_')) return 'The tool schema is unavailable or invalid for the current turn.'
  if (code === 'tool_pending_continuation_invalid') return 'Pending tool continuation failed current host revalidation.'
  if (code.startsWith('tool_')) return 'The host rejected or could not complete the tool operation.'
  if (code.startsWith('approval_')) return 'The operation did not receive current host approval.'
  if (code.startsWith('source_')) return 'The required source is unavailable or no longer current.'
  return 'The turn failed before a verified response was available.'
}

function isClosedHostError(codeValue: unknown, messageValue: unknown): boolean {
  if (typeof codeValue !== 'string' || !HOST_ERROR_CODES.has(codeValue) || typeof messageValue !== 'string') return false
  return messageValue === fixedHostErrorMessage(codeValue) ||
    messageValue === `Runtime request failed (${codeValue}).`
}

function validateErrorEvent(event: Record<string, unknown>): boolean {
  return event.itemId === undefined && isClosedHostError(event.code, event.message) &&
    isOptionalEnum(event.severity, new Set(['info', 'warning', 'error'])) &&
    isOptionalBoolean(event.terminal) && isOptionalBoolean(event.fatal) && isOptionalBoolean(event.recoverable) &&
    (event.turnId === undefined || isSafePublicId(event.turnId)) &&
    (event.details === undefined || isClosedErrorDetails(event.details))
}

function validateHeartbeatEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event)
}

function validateCursorAdvancedEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && event.reason === 'restricted_content_removed'
}

function validateSnapshotRequiredEvent(event: Record<string, unknown>): boolean {
  return eventHasNoTurnOrItem(event) && isSafeSequence(event.sinceSeq) && isSafeSequence(event.highestSeq) &&
    isSafeSequence(event.replayEventCount) && event.reason === 'live_replay_backlog_exceeded' &&
    (event.highestSeq as number) >= (event.sinceSeq as number)
}

function isClosedTrace(value: unknown): boolean {
  if (value === undefined) return true
  if (!isRecord(value) || !hasOnlyKeys(value, ['sse_sent_at', 'sse_live_emitted_at'])) return false
  return Object.keys(value).length === 2 &&
    typeof value.sse_sent_at === 'number' && Number.isFinite(value.sse_sent_at) && value.sse_sent_at >= 0 &&
    typeof value.sse_live_emitted_at === 'number' &&
    Number.isFinite(value.sse_live_emitted_at) && value.sse_live_emitted_at >= 0
}

export function isPublicProjectionRevokedEvent(value: unknown): value is PublicProjectionRevokedEvent {
  return PublicProjectionRevokedEventSchema.safeParse(value).success
}

function isClosedCheckpointStatus(value: unknown): boolean {
  if (!isRecord(value) || !hasOnlyKeys(value, CHECKPOINT_STATUS_KEYS)) return false
  if (value.schemaVersion !== 1 || value.projectionKind !== 'checkpoint_status' ||
      value.disclosure !== 'metadata_only' || value.privatePayloadWithheld !== true ||
      value.factAnswerAllowed !== false || value.evidenceAuthority !== false ||
      typeof value.checkpointRefDigest !== 'string' || !SHA256_HEX.test(value.checkpointRefDigest)) return false
  if (!new Set(['captured', 'created', 'applied', 'blocked', 'failed', 'already_applied'])
    .has(String(value.status)) || !isOptionalSafeSequence(value.changedFileCount) ||
    !isOptionalSafeSequence(value.fileCount) ||
    (value.snapshotStorage !== undefined && value.snapshotStorage !== 'runtime_private_cas') ||
    !isOptionalEnum(value.scope, new Set(['code', 'conversation', 'combined'])) ||
    (value.destructive !== undefined && value.destructive !== true) ||
    !isOptionalEnum(value.conversationStatus, new Set([
      'not_requested', 'audit_recorded', 'blocked', 'already_applied'
    ]))) return false
  if (value.summary === undefined) return true
  if (!isRecord(value.summary) || !hasOnlyKeys(value.summary, [
    'fileAppliedCount', 'fileNoopCount', 'fileManualReviewCount', 'fileBlockedCount', 'fileFailedCount'
  ])) return false
  const summaryKeys = [
    'fileAppliedCount', 'fileNoopCount', 'fileManualReviewCount', 'fileBlockedCount', 'fileFailedCount'
  ]
  return summaryKeys.every((key) => isSafeSequence((value.summary as Record<string, unknown>)[key])) &&
    summaryKeys.reduce((sum, key) => sum + Number((value.summary as Record<string, unknown>)[key]), 0) ===
      value.fileCount
}

function isClosedChildMetadata(value: unknown): boolean {
  if (!isRecord(value)) return false
  const allowed = [
    'notificationKind', 'jobId', 'childId', 'childRunId', 'childThreadId', 'childTurnId',
    'parentThreadId', 'parentTurnId', 'parentToolCallId', 'status', 'childStatus', 'background',
    'terminal', 'autoContinueStatus', 'lateCompletionSuppressed', 'deliveryStatus', 'kind', 'id',
    'outputWithheld', 'outputTrustStatus', 'factAnswerAllowed', 'evidenceAuthority', 'canReadOutput',
    'canContinueParent'
  ]
  const childReferences = [value.jobId, value.childId, value.childRunId]
    .filter((entry): entry is string => entry !== undefined)
  if (!hasOnlyKeys(value, allowed) || value.kind !== 'subagent_task' ||
      value.outputWithheld !== true || value.outputTrustStatus !== 'untrusted_child_output' ||
      value.factAnswerAllowed !== false || value.evidenceAuthority !== false ||
      value.canReadOutput !== false || value.canContinueParent !== false ||
      !isSafePublicId(value.id) || childReferences.length === 0 ||
      !childReferences.every(isSafePublicId) || !childReferences.includes(value.id) ||
      !isOptionalSafePublicId(value.childThreadId) || !isOptionalSafePublicId(value.childTurnId) ||
      !isOptionalSafePublicId(value.parentThreadId) || !isOptionalSafePublicId(value.parentTurnId) ||
      !isOptionalSafePublicId(value.parentToolCallId) ||
      typeof value.status !== 'string' || !JOB_STATUSES.has(value.status) ||
      value.childStatus !== value.status || typeof value.terminal !== 'boolean' ||
      !isOptionalBoolean(value.background) ||
      !isOptionalBoolean(value.lateCompletionSuppressed) ||
      !isOptionalEnum(value.deliveryStatus, DELIVERY_STATUSES) ||
      !isOptionalEnum(value.autoContinueStatus, AUTO_CONTINUE_STATUSES) ||
      (value.notificationKind !== undefined && !new Set([
        'background_job_completion', 'background_job_auto_continue', 'background_job_delivery',
        'thread_summary_subagent', 'thread_summary_task'
      ]).has(String(value.notificationKind)))) return false
  return true
}

function isClosedPipelineDetails(value: unknown): boolean {
  if (!isRecord(value)) return false
  const allowed = [
    'visibleRecovery', 'recoveryKind', 'recoveryAttempt', 'maxRecoveryAttempt',
    'maxRecoveryAttempts', 'recoveryExhausted', 'partialToolStarted', 'maxModelSteps', 'toolName',
    'guardKind', 'stormCount', 'providerError', 'reasonCode'
  ]
  if (!hasOnlyKeys(value, allowed) || !isOptionalBoolean(value.visibleRecovery) ||
      !isOptionalEnum(value.recoveryKind, new Set([
        'interrupted_stream', 'step_limit_final_answer', 'empty_final'
      ])) || !isOptionalSafeSequence(value.recoveryAttempt) ||
      !isOptionalSafeSequence(value.maxRecoveryAttempt) ||
      !isOptionalSafeSequence(value.maxRecoveryAttempts) || !isOptionalBoolean(value.recoveryExhausted) ||
      !isOptionalBoolean(value.partialToolStarted) || !isOptionalSafeSequence(value.maxModelSteps) ||
      !isOptionalSafeSequence(value.stormCount) || !isOptionalSafeIdentifier(value.toolName) ||
      !isOptionalEnum(value.guardKind, new Set([
        'tool_failure', 'invalid_tool_arguments'
      ])) ||
      (value.reasonCode !== undefined &&
        (typeof value.reasonCode !== 'string' || !HOST_ERROR_CODES.has(value.reasonCode)))) return false
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

function isClosedGeneralTerminalErrorDetails(value: unknown): boolean {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'status', 'retryAfterMs', 'attempt', 'maxAttempt', 'recoveryAttempt', 'maxRecoveryAttempt',
    'maxRecoveryAttempts', 'maxModelSteps', 'stormCount', 'endpointFormat', 'kind', 'authStatus',
    'hasApiKey', 'retryable', 'executed', 'partialToolStarted', 'visibleRecovery',
    'recoveryExhausted', 'loopStep', 'advertisedToolCount',
    'failureStage', 'dispatchState',
    'rejectedToolNormalizedNameSha256', 'advertisedToolManifestHash',
    'advertisedNameSetSortedHash', 'providerRequestToolManifestHash', 'runToolStepManifestHash',
    'rejectedToolCategory', 'promptRoute', 'providerRequestRunToolStepManifestSame'
  ])) return false
  return (value.status === undefined || (isSafeSequence(value.status) && value.status <= 599)) &&
    (value.retryAfterMs === undefined || (isSafeSequence(value.retryAfterMs) && value.retryAfterMs <= 900_000)) &&
    ['attempt', 'maxAttempt', 'recoveryAttempt', 'maxRecoveryAttempt', 'maxRecoveryAttempts',
      'maxModelSteps', 'stormCount', 'loopStep', 'advertisedToolCount'].every((key) => value[key] === undefined ||
        (isSafeSequence(value[key]) && (value[key] as number) <= 1_000_000)) &&
    ['rejectedToolNormalizedNameSha256', 'advertisedToolManifestHash',
      'advertisedNameSetSortedHash', 'providerRequestToolManifestHash', 'runToolStepManifestHash']
      .every((key) => value[key] === undefined ||
        (typeof value[key] === 'string' && SHA256_HEX.test(value[key] as string))) &&
    isOptionalEnum(value.rejectedToolCategory, new Set([
      'known_builtin_not_advertised', 'known_mcp_not_advertised',
      'known_alias_not_advertised', 'unknown_provider_name'
    ])) && isOptionalEnum(value.promptRoute, new Set([
      'direct_answer', 'light_agent', 'tool_agent', 'subagent_agent'
    ])) &&
    isOptionalEnum(value.endpointFormat, ENDPOINT_FORMATS) &&
    isOptionalEnum(value.kind, new Set([
      'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
      'invalid_model', 'unknown'
    ])) && isOptionalEnum(value.authStatus, new Set(['none', 'required'])) &&
    isOptionalEnum(value.failureStage, PROVIDER_FAILURE_STAGES) &&
    isOptionalEnum(value.dispatchState, PROVIDER_DISPATCH_STATES) &&
    isOptionalBoolean(value.hasApiKey) && isOptionalBoolean(value.retryable) &&
    isOptionalBoolean(value.executed) && isOptionalBoolean(value.partialToolStarted) &&
    isOptionalBoolean(value.visibleRecovery) && isOptionalBoolean(value.recoveryExhausted)
    && isOptionalBoolean(value.providerRequestRunToolStepManifestSame)
}

function isClosedErrorDetails(value: unknown): boolean {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'status', 'providerId', 'provider', 'family', 'model', 'endpointFormat', 'kind', 'authStatus',
    'hasApiKey', 'retryable', 'attempt', 'maxAttempt', 'port', 'backend', 'requestedBackend',
    'autoStart', 'phase', 'failureStage', 'dispatchState'
  ])) return false
  return (value.status === undefined || (isSafeSequence(value.status) && value.status <= 599)) &&
    isOptionalSafeIdentifier(value.providerId) && isOptionalSafeIdentifier(value.provider) &&
    isOptionalSafeIdentifier(value.family) && isOptionalSafeIdentifier(value.model) &&
    isOptionalEnum(value.endpointFormat, ENDPOINT_FORMATS) &&
    isOptionalEnum(value.kind, new Set([
      'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
      'invalid_model', 'unknown'
    ])) && isOptionalEnum(value.authStatus, new Set(['none', 'possible', 'required', 'unknown'])) &&
    isOptionalBoolean(value.hasApiKey) && isOptionalBoolean(value.retryable) &&
    isOptionalEnum(value.failureStage, PROVIDER_FAILURE_STAGES) &&
    isOptionalEnum(value.dispatchState, PROVIDER_DISPATCH_STATES) &&
    isOptionalSafeSequence(value.attempt) && isOptionalSafeSequence(value.maxAttempt) &&
    (value.port === undefined || (isSafeSequence(value.port) && value.port <= 65_535)) &&
    isOptionalEnum(value.backend, new Set(['go', 'runtime-go', 'go-runtime-default'])) &&
    isOptionalEnum(value.requestedBackend, new Set(['', 'go', 'runtime-go', 'go-runtime-default'])) &&
    isOptionalBoolean(value.autoStart) &&
    isOptionalEnum(value.phase, new Set(['setup', 'highest_seq', 'replay', 'live']))
}

function isClosedAcceptedFinalDeliveryBatch(event: Record<string, unknown>): boolean {
  const allowed = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'schemaVersion', 'purpose', 'batchId',
    'firstSeq', 'lastSeq', 'publicationCommitId', 'eventManifestDigest', 'publicationAuthority',
    'events', 'trace'
  ]
  if (!AcceptedFinalDeliveryBatchV2Schema.safeParse(event).success ||
      !hasOnlyKeys(event, allowed) || event.schemaVersion !== 2 ||
      event.purpose !== 'analytix.accepted-final-delivery-batch/v2' ||
      event.kind !== 'accepted_final_batch' || !isExactNonEmptyString(event.threadId) ||
      !isExactNonEmptyString(event.turnId) || !isExactNonEmptyString(event.timestamp) ||
      !isSafeSequence(event.seq) || !isSafeSequence(event.firstSeq) || !isSafeSequence(event.lastSeq) ||
      event.seq !== event.lastSeq || (event.firstSeq as number) <= 0 ||
      (event.lastSeq as number) < (event.firstSeq as number) ||
      typeof event.batchId !== 'string' || !/^[a-f0-9]{64}$/.test(event.batchId) ||
      typeof event.publicationCommitId !== 'string' || !/^[a-f0-9]{64}$/.test(event.publicationCommitId) ||
      typeof event.eventManifestDigest !== 'string' || !/^[a-f0-9]{64}$/.test(event.eventManifestDigest) ||
      !isClosedAcceptedFinalDeliverySeal(event.publicationAuthority) ||
      !isClosedTrace(event.trace) ||
      !Array.isArray(event.events) || (event.events.length !== 3 && event.events.length !== 4) ||
      (event.lastSeq as number) - (event.firstSeq as number) + 1 !== event.events.length) {
    return false
  }
  const authority = event.publicationAuthority as Record<string, unknown>
  if (authority.threadId !== event.threadId || authority.turnId !== event.turnId ||
      authority.publicationCommitId !== event.publicationCommitId ||
      authority.eventManifestDigest !== event.eventManifestDigest ||
      authority.batchId !== event.batchId || authority.firstSeq !== event.firstSeq ||
      authority.lastSeq !== event.lastSeq || authority.timestamp !== event.timestamp) {
    return false
  }
  const expectedSlots = event.events.length === 3
    ? ['assistant-final', 'usage', 'terminal']
    : ['assistant-final', 'terminal-error-item', 'usage', 'terminal']
  return event.events.every((nested, index) => {
    if (!isRecord(nested) || nested.kind === 'accepted_final_batch' ||
        nested.threadId !== event.threadId || nested.turnId !== event.turnId ||
        nested.seq !== (event.firstSeq as number) + index ||
        nested.publicationCommitId !== event.publicationCommitId ||
        nested.acceptedFinalDigest !== event.publicationCommitId ||
        nested.publicationSlot !== expectedSlots[index]) {
      return false
    }
    // The strict batch schema above validates the purpose-specific nested
    // event union. Nested accepted-final fields are never valid as detached
    // ordinary live events, so there is intentionally no generic bypass.
    return true
  })
}

function isClosedAcceptedFinalDeliverySeal(value: unknown): boolean {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'schemaVersion', 'purpose', 'sealId', 'threadId', 'turnId', 'publicationCommitId',
    'acceptedFinalDispositionDigest', 'terminalDispositionId', 'eventManifestDigest',
    'sequencedEventsDigest', 'batchId', 'firstSeq', 'lastSeq', 'timestamp',
    'authorityAlgorithm', 'authorityKeyId', 'authorityPublicKey', 'authoritySignature'
  ])) return false
  return value.schemaVersion === 'accepted-final-delivery-seal.v1' &&
    value.purpose === 'analytix.accepted-final-delivery-seal/v1' && value.authorityAlgorithm === 'Ed25519' &&
    typeof value.sealId === 'string' && /^[a-f0-9]{64}$/.test(value.sealId) &&
    typeof value.acceptedFinalDispositionDigest === 'string' && /^[a-f0-9]{64}$/.test(value.acceptedFinalDispositionDigest) &&
    typeof value.terminalDispositionId === 'string' && /^[a-f0-9]{64}$/.test(value.terminalDispositionId) &&
    typeof value.eventManifestDigest === 'string' && /^[a-f0-9]{64}$/.test(value.eventManifestDigest) &&
    typeof value.sequencedEventsDigest === 'string' && /^[a-f0-9]{64}$/.test(value.sequencedEventsDigest) &&
    typeof value.batchId === 'string' && /^[a-f0-9]{64}$/.test(value.batchId) &&
    typeof value.publicationCommitId === 'string' && /^[a-f0-9]{64}$/.test(value.publicationCommitId) &&
    typeof value.authorityKeyId === 'string' && /^[a-f0-9]{64}$/.test(value.authorityKeyId) &&
    typeof value.authorityPublicKey === 'string' && /^[A-Za-z0-9_-]{43}$/.test(value.authorityPublicKey) &&
    typeof value.authoritySignature === 'string' && /^[A-Za-z0-9_-]{86}$/.test(value.authoritySignature) &&
    isExactNonEmptyString(value.threadId) && isExactNonEmptyString(value.turnId) &&
    isExactNonEmptyString(value.timestamp) && isSafeSequence(value.firstSeq) && isSafeSequence(value.lastSeq)
}

function isClosedGeneralTerminalDeliveryBatch(event: Record<string, unknown>): boolean {
  const allowed = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'schemaVersion', 'purpose',
    'batchDigest', 'firstSeq', 'lastSeq', 'generalTerminalCommitId',
    'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest', 'eventManifestDigest',
    'projectedEventsDigest', 'transportAuthority', 'evidenceAuthority', 'citationAuthority',
    'factAnswerAllowed', 'events', 'eventManifest'
  ]
  const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(event)
  if (!parsed.success || containsInternalCaseEntityReference(event) || !hasOnlyKeys(event, allowed) ||
      !isCanonicalTimestamp(event.timestamp) || !isSafePublicId(event.threadId) ||
      !isSafePublicId(event.turnId) || !isSafeSequence(event.seq) ||
      !isSafeSequence(event.firstSeq) || !isSafeSequence(event.lastSeq) ||
      event.seq !== event.lastSeq || event.evidenceAuthority !== false ||
      event.citationAuthority !== false || event.factAnswerAllowed !== false ||
      !Array.isArray(event.events) || !Array.isArray(event.eventManifest)) return false

  for (const nested of event.events) {
    if (!isRecord(nested) || nested.kind === 'general_terminal_batch' ||
        nested.kind === 'accepted_final_batch' || claimsAcceptedFinalDeliveryAuthority(nested)) return false
    if (nested.kind === 'item_completed') {
      const item = isRecord(nested.item) ? nested.item : null
      if (!item) return false
      if (item.kind === 'assistant_text' && isRecord(item.ordinaryResult) &&
          (typeof item.text !== 'string' || containsProtectedCaseFactCandidate(item.text))) return false
      if (item.kind === 'error' && (!isClosedHostError(item.code, item.message) ||
          (item.details !== undefined && !isClosedGeneralTerminalErrorDetails(item.details)))) return false
    }
    if (nested.kind === 'turn_failed' || nested.kind === 'turn_aborted' || nested.kind === 'turn_completed') {
      const hasMessage = nested.message !== undefined || nested.error !== undefined
      if (hasMessage && (!isClosedHostError(nested.code, nested.message) ||
          (nested.error !== undefined && nested.error !== nested.message))) return false
      if (nested.details !== undefined && !isClosedGeneralTerminalErrorDetails(nested.details)) return false
    }
  }
  return true
}

function claimsAcceptedFinalDeliveryAuthority(event: Record<string, unknown>): boolean {
  const item = isRecord(event.item) ? event.item : null
  return containsPrivateAcceptedFinalAuthority(event) ||
    ['acceptedFinalDigest', 'publicationCommitId', 'publicationEventId', 'publicationSlot',
    'publicationPayloadDigest'].some((key) => Object.prototype.hasOwnProperty.call(event, key)) ||
    Boolean(item && (Object.prototype.hasOwnProperty.call(item, 'acceptedFinal') ||
      Object.prototype.hasOwnProperty.call(item, 'acceptedFinalView')))
}

export function isClosedPublicRuntimeSseEvent(event: Record<string, unknown>): boolean {
  const kind = typeof event.kind === 'string' ? event.kind : ''
  if (kind === 'public_projection_revoked') return isPublicProjectionRevokedEvent(event)
  if (kind === 'accepted_final_batch') return isClosedAcceptedFinalDeliveryBatch(event)
  if (kind === 'general_terminal_batch') return isClosedGeneralTerminalDeliveryBatch(event)
  const contract = PUBLIC_EVENT_CONTRACTS[kind]
  if (!contract || !hasOnlyKeys(event, [...BASE_EVENT_KEYS, ...contract.keys])) return false
  if (!isSafeSequence(event.seq) || !isCanonicalTimestamp(event.timestamp) ||
      !isSafePublicId(event.threadId) ||
      (event.turnId !== undefined && !isSafePublicId(event.turnId)) ||
      (event.itemId !== undefined && !isSafePublicId(event.itemId)) ||
      !isClosedTrace(event.trace) || !contract.validate(event)) return false
  return isPublicSseIpcPayload({ streamId: 'strict-sse-validation', events: [event] })
}

export function isStrictPublicRuntimeSseIpcPayload(value: unknown): value is {
  streamId: string
  events: Array<Record<string, unknown>>
} {
  if (!isRecord(value) || !hasOnlyKeys(value, ['streamId', 'events'])) return false
  if (!isExactNonEmptyString(value.streamId) || value.streamId.length > 256 || !Array.isArray(value.events)) {
    return false
  }
  const controls = value.events.filter((event) => isPublicProjectionRevokedEvent(event))
  if (controls.length > 0) {
    return value.events.length === 1 && controls.length === 1
  }
  const acceptedFinalBatches = value.events.filter((event) =>
    isRecord(event) && event.kind === 'accepted_final_batch'
  )
  if (acceptedFinalBatches.length > 0) {
    return value.events.length === 1 && acceptedFinalBatches.length === 1 &&
      isClosedAcceptedFinalDeliveryBatch(acceptedFinalBatches[0] as Record<string, unknown>)
  }
  const generalTerminalBatches = value.events.filter((event) =>
    isRecord(event) && event.kind === 'general_terminal_batch'
  )
  if (generalTerminalBatches.length > 0) {
    return value.events.length === 1 && generalTerminalBatches.length === 1 &&
      isClosedGeneralTerminalDeliveryBatch(generalTerminalBatches[0] as Record<string, unknown>)
  }
  return value.events.every((event) => isRecord(event) && isClosedPublicRuntimeSseEvent(event) &&
    !claimsAcceptedFinalDeliveryAuthority(event))
}

export function isStrictPublicRuntimeSseEndPayload(value: unknown): value is { streamId: string } {
  return isRecord(value) && hasOnlyKeys(value, ['streamId']) &&
    isExactNonEmptyString(value.streamId) && value.streamId.length <= 256
}

export function isStrictPublicRuntimeSseErrorPayload(value: unknown): value is {
  streamId: string
  status?: number
  code?: string
  message?: string
  reasonCode?: PublicRuntimeSseRejectionReasonCode
} {
  if (!isRecord(value) || !hasOnlyKeys(value, [
    'streamId', 'status', 'code', 'message', 'reasonCode'
  ]) ||
      !isExactNonEmptyString(value.streamId) || value.streamId.length > 256) return false
  const hasStatus = value.status !== undefined
  const hasCode = value.code !== undefined
  if (!hasStatus && !hasCode) return false
  if (hasStatus && (!Number.isSafeInteger(value.status) || (value.status as number) < 100 ||
      (value.status as number) > 599)) return false
  if (hasCode) {
    if (typeof value.code !== 'string' || !/^[A-Za-z0-9_.-]{1,128}$/.test(value.code)) return false
    if (value.message !== `Runtime request failed (${value.code}).`) return false
  } else if (value.message !== undefined) {
    return false
  }
  if (value.reasonCode !== undefined && (
    value.code !== 'sse_event_rejected' ||
    !isPublicRuntimeSseRejectionReasonCode(value.reasonCode)
  )) return false
  return true
}

function readSseFieldValue(line: string, field: string): string | null {
  const prefix = `${field}:`
  if (!line.startsWith(prefix)) return null
  const raw = line.slice(prefix.length)
  const value = raw.startsWith(' ') ? raw.slice(1) : raw
  if (!value || value.trim() !== value) return null
  return value
}

function parseStrictFrame(block: string): {
  event: string
  id?: string
  data: Record<string, unknown>
} | null {
  const lines = block.split('\n').map((line) => line.endsWith('\r') ? line.slice(0, -1) : line)
  let eventName: string | undefined
  let eventId: string | undefined
  let dataText: string | undefined
  for (const line of lines) {
    if (!line || line.startsWith(':')) continue
    const eventValue = readSseFieldValue(line, 'event')
    if (eventValue !== null) {
      if (eventName !== undefined) return null
      eventName = eventValue
      continue
    }
    const idValue = readSseFieldValue(line, 'id')
    if (idValue !== null) {
      if (eventId !== undefined) return null
      eventId = idValue
      continue
    }
    const dataValue = readSseFieldValue(line, 'data')
    if (dataValue !== null) {
      if (dataText !== undefined) return null
      dataText = dataValue
      continue
    }
    return null
  }
  if (eventName === undefined || dataText === undefined) return null
  if (!/^[a-z][a-z0-9_]{0,95}$/.test(eventName) ||
      (eventId !== undefined && !/^(?:0|[1-9][0-9]*)$/.test(eventId))) return null
  try {
    const data = JSON.parse(dataText) as unknown
    if (!isRecord(data)) return null
    return eventId === undefined ? { event: eventName, data } : { event: eventName, id: eventId, data }
  } catch {
    return null
  }
}

export function projectPublicRuntimeSseBlock(
  block: string,
  expectedThreadId: string,
  filter: PublicRuntimeEventFilter
): PublicRuntimeSseDecision | null {
  const trimmed = block.trim()
  if (!trimmed || trimmed.split(/\r?\n/).every((line) => !line || line.startsWith(':'))) return null
  const parsed = parseStrictFrame(block)
  if (!parsed) return { status: 'invalid', reason: 'malformed_frame' }
  if (parsed.event === 'public_projection_revoked') {
    if (parsed.id !== undefined || parsed.data.kind !== parsed.event) {
      return { status: 'invalid', reason: 'malformed_frame' }
    }
    if (!isPublicProjectionRevokedEvent(parsed.data)) {
      return { status: 'invalid', reason: 'invalid_public_projection' }
    }
    if (parsed.data.threadId !== expectedThreadId) {
      return { status: 'invalid', reason: 'event_thread_mismatch' }
    }
    return { status: 'revoke', event: parsed.data }
  }
  if (parsed.id === undefined) return { status: 'invalid', reason: 'missing_transport_binding' }
  const seq = Number(parsed.id)
  if (!Number.isSafeInteger(seq)) return { status: 'invalid', reason: 'missing_transport_binding' }
  if (parsed.data.kind !== parsed.event) return { status: 'invalid', reason: 'event_kind_mismatch' }
  if (parsed.data.seq !== seq || !isSafeSequence(parsed.data.seq)) {
    return { status: 'invalid', reason: 'event_sequence_mismatch' }
  }
  if (parsed.data.threadId !== expectedThreadId || !isExactNonEmptyString(parsed.data.threadId)) {
    return { status: 'invalid', reason: 'event_thread_mismatch' }
  }
  if (PRIVATE_EVENT_KINDS.has(parsed.event)) {
    return { status: 'withheld', seq, reason: 'restricted_content' }
  }
  if (!PUBLIC_EVENT_CONTRACTS[parsed.event]) {
    return { status: 'invalid', reason: 'unsupported_event_kind' }
  }
  // Validate the host-owned closed event before the generic public-content
  // filter. The filter may withhold private fields, but it must never turn an
  // open provider/tool/MCP payload into a new public contract.
  if (!isClosedPublicRuntimeSseEvent(parsed.data)) {
    return { status: 'invalid', reason: 'invalid_public_projection' }
  }

  const projected = filter.push(parsed.data)
  if (!projected) {
    const item = isRecord(parsed.data.item) ? parsed.data.item : undefined
    const privateKind = parsed.event === 'assistant_reasoning' ||
      parsed.event === 'assistant_reasoning_delta' ||
      parsed.event === 'agent_reasoning' ||
      item?.kind === 'assistant_reasoning'
    return privateKind || parsed.event === 'assistant_text_delta'
      ? { status: 'withheld', seq, reason: 'restricted_content' }
      : { status: 'invalid', reason: 'invalid_public_projection' }
  }
  if (projected.kind !== parsed.event || projected.seq !== seq || projected.threadId !== expectedThreadId ||
      !isClosedPublicRuntimeSseEvent(projected)) {
    return { status: 'invalid', reason: 'invalid_public_projection' }
  }
  return { status: 'emit', event: projected, seq }
}

export function takePublicRuntimeSseBlock(buffer: string): { block: string; rest: string } | null {
  const lf = buffer.indexOf('\n\n')
  const crlf = buffer.indexOf('\r\n\r\n')
  if (lf === -1 && crlf === -1) return null
  if (crlf !== -1 && (lf === -1 || crlf < lf)) {
    return { block: buffer.slice(0, crlf), rest: buffer.slice(crlf + 4) }
  }
  return { block: buffer.slice(0, lf), rest: buffer.slice(lf + 2) }
}
