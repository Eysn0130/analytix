import {
  containsPrivateAcceptedFinalAuthority,
  isClosedPipelineDetails,
  isClosedLogicalStepEvent,
  isPublicHostErrorCode,
  isPublicProviderDispatchState,
  isPublicProviderEndpointFormat,
  isPublicProviderFailureStage,
  isPublicSseIpcPayload,
  PublicRuntimeEventFilter
} from './public-runtime-content'
import {
  AcceptedFinalDeliveryBatchV2Schema,
  ApprovalEvent,
  CompactionEvent,
  GeneralTerminalDeliveryBatchV1Schema,
  GoalActivationChangedEventV1,
  PublicProjectionRevokedEvent as PublicProjectionRevokedEventSchema,
  UserInputEvent
} from '../../packages/runtime/src/contracts/events.js'
import type { PublicProjectionRevokedEvent } from '../../packages/runtime/src/contracts/events.js'
import { GoalRoundSourceSchemaV1 } from '../../packages/runtime/src/contracts/goal.js'
import {
  ApprovalTurnItem,
  CompactionTurnItem,
  TaskContinuationSnapshotV1Schema,
  ToolCallTurnItem,
  ToolProgressTurnItem,
  ToolResultTurnItem,
  UserInputTurnItem,
  UserFileReferenceSchema
} from '../../packages/runtime/src/contracts/items.js'
import {
  containsInternalCaseEntityReference,
  containsUnboundCaseRiskV1
} from './ordinary-log-pii-projection'

export { isClosedProviderRetryLifecycle, isClosedProviderOutputCapObservation } from './public-runtime-content'

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

// Diagnostic paths are schema names, never keys taken from an event. Unknown
// segments stop at the nearest listed container; array indexes are normalized.
const DIAGNOSTIC_FIELD_PATHS = [
  '$', 'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'trace',
  'stage', 'callId', 'message', 'child', 'details',
  'item', 'item.id', 'item.threadId', 'item.turnId', 'item.role', 'item.status',
  'item.createdAt', 'item.finishedAt', 'item.kind', 'item.toolName', 'item.callId',
  'item.toolKind', 'item.isError', 'item.arguments', 'item.arguments.schemaVersion',
  'item.arguments.projectionKind', 'item.arguments.disclosure', 'item.arguments.messageKey',
  'item.arguments.privatePayloadWithheld', 'item.arguments.factAnswerAllowed',
  'item.arguments.evidenceAuthority', 'item.arguments.runtimeStatus', 'item.arguments.stage',
  'item.arguments.status', 'item.arguments.diagnostics', 'item.output',
  'item.output.schemaVersion', 'item.output.projectionKind', 'item.output.disclosure',
  'item.output.status', 'item.output.code', 'item.output.messageKey',
  'item.output.privatePayloadWithheld', 'item.output.factAnswerAllowed',
  'item.output.evidenceAuthority', 'item.output.taskJob', 'item.output.taskJob.operation',
  'item.output.taskJob.result', 'item.approvalId', 'item.inputId', 'item.summary',
  'item.prompt', 'item.questions', 'item.questions.[]', 'item.questions.[].id',
  'item.questions.[].header', 'item.questions.[].question', 'item.questions.[].options',
  'item.timedWait', 'item.text', 'item.displayText', 'item.delivery', 'item.clientUserMessageId',
  'item.workspaceCheckpointId', 'item.attachmentIds', 'item.activeSkillIds', 'item.taskContinuation', 'item.taskContinuation.schemaVersion',
  'item.taskContinuation.goal', 'item.taskContinuation.todos', 'item.taskContinuation.todos.[]',
  'item.taskContinuation.todos.[].todoId', 'item.taskContinuation.evidenceReferences',
  'item.taskContinuation.evidenceReferences.[]',
  'item.taskContinuation.evidenceReferences.[].referenceDigest',
  'approvalId', 'toolName', 'inputId', 'status', 'prompt', 'questions', 'questions.[]',
  'questions.[].id', 'questions.[].header', 'questions.[].question', 'questions.[].options',
  'timedWait', 'schemaVersion', 'purpose', 'batchId', 'firstSeq', 'lastSeq',
  'publicationCommitId', 'eventManifestDigest', 'publicationAuthority', 'batchDigest',
  'generalTerminalCommitId', 'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest',
  'projectedEventsDigest', 'transportAuthority', 'evidenceAuthority', 'citationAuthority',
  'factAnswerAllowed', 'events', 'events.[]', 'events.[].kind', 'events.[].seq',
  'events.[].timestamp', 'events.[].threadId', 'events.[].turnId', 'events.[].itemId',
  'events.[].item', 'events.[].item.kind', 'events.[].item.status',
  'events.[].item.ordinaryResult', 'eventManifest', 'eventManifest.[]'
] as const

type DiagnosticFieldPath = typeof DIAGNOSTIC_FIELD_PATHS[number]
type DiagnosticStage = 'frame' | 'transport_binding' | 'event_contract' | 'event_schema' |
  'item_contract' | 'item_binding' | 'item_schema' | 'public_content' |
  'projection_filter' | 'projected_binding'
type DiagnosticReason = 'malformed_frame' | 'missing_transport_binding' | 'binding_mismatch' |
  'unsupported_event_kind' | 'unsupported_item_kind' | 'detached_terminal_item' |
  'unknown_fields' | 'invalid_field' | 'schema_invalid_type' | 'schema_invalid_value' |
  'schema_invalid_format' | 'schema_constraint' | 'schema_refinement' | 'schema_union' |
  'schema_other' | 'restricted_content' | 'public_content_rejected' | 'projection_rejected'

export type PublicRuntimeSseDecodeDiagnostic = Readonly<{
  validation: 'input' | 'projected'
  stage: DiagnosticStage
  reason: DiagnosticReason
  fieldPaths: readonly DiagnosticFieldPath[]
}>

type DiagnosticRecorder = (
  stage: DiagnosticStage,
  reason: DiagnosticReason,
  fieldPaths: readonly DiagnosticFieldPath[]
) => void

const DIAGNOSTIC_FIELD_PATH_SET = new Set<string>(DIAGNOSTIC_FIELD_PATHS)

function diagnosticFieldPath(prefix: DiagnosticFieldPath, path: readonly PropertyKey[]): DiagnosticFieldPath {
  let known = prefix
  for (const segment of path) {
    const name = typeof segment === 'number' ? '[]' : typeof segment === 'string' ? segment : ''
    // Dots cannot turn one unknown key into a sequence of known schema names.
    if (!name || name.includes('.')) break
    const candidate = known === '$' ? name : `${known}.${name}`
    if (!DIAGNOSTIC_FIELD_PATH_SET.has(candidate)) break
    known = candidate as DiagnosticFieldPath
  }
  return known
}

function invalidValidation(
  diagnostic: DiagnosticRecorder | undefined,
  stage: DiagnosticStage,
  reason: DiagnosticReason,
  fieldPaths: readonly DiagnosticFieldPath[]
): false {
  diagnostic?.(stage, reason, fieldPaths)
  return false
}

function schemaDiagnosticReason(code: string): DiagnosticReason {
  switch (code) {
    case 'unrecognized_keys': return 'unknown_fields'
    case 'invalid_type': return 'schema_invalid_type'
    case 'invalid_value': return 'schema_invalid_value'
    case 'invalid_format': return 'schema_invalid_format'
    case 'too_small':
    case 'too_big': return 'schema_constraint'
    case 'custom': return 'schema_refinement'
    case 'invalid_union': return 'schema_union'
    default: return 'schema_other'
  }
}

function schemaIsValid(
  result: { success: true } | { success: false; error: { issues: readonly {
    code: string; path: readonly PropertyKey[]
  }[] } },
  diagnostic: DiagnosticRecorder | undefined,
  stage: DiagnosticStage,
  prefix: DiagnosticFieldPath
): boolean {
  if (result.success || !diagnostic) return result.success
  const issues = result.error.issues.slice(0, 12)
  // In particular, unrecognized_keys.keys and union messages/errors contain
  // untrusted names or values. Only the first parse's code and path are read.
  const paths = [...new Set(issues.map((issue) => diagnosticFieldPath(prefix, issue.path)))]
  return invalidValidation(diagnostic, stage,
    schemaDiagnosticReason(issues[0]?.code ?? ''), paths.length ? paths : [prefix])
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
  validate: (event: Record<string, unknown>, diagnostic?: DiagnosticRecorder) => boolean
}>

function eventContract(
  keys: readonly string[],
  validate: PublicEventContract['validate']
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
    'endReason', 'status', 'clientUserMessageId', 'admittedSeq', 'terminalReason', 'discard', 'cancelled',
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
    'approvalId', 'toolName', 'status', 'approvalPolicy', 'sandboxMode', 'summary'
  ], validateApprovalEvent),
  approval_resolved: eventContract([
    'approvalId', 'toolName', 'status', 'approvalPolicy', 'sandboxMode', 'summary'
  ], validateApprovalEvent),
  user_input_requested: eventContract(['inputId', 'status', 'prompt', 'questions', 'timedWait'], validateUserInputEvent),
  user_input_resolved: eventContract(['inputId', 'status', 'prompt', 'questions', 'timedWait'], validateUserInputEvent),
  compaction_started: eventContract(['auto', 'compactionId', 'schemaVersion', 'startedAt'], validateCompactionEvent),
  compaction_completed: eventContract([
    'summary', 'replacedTokens', 'auto', 'pinnedConstraints', 'sourceDigest', 'digestMarker', 'sourceItemIds',
    'schemaVersion', 'reasoningExcluded', 'reasoningExclusionProof', 'compactionId', 'summaryKind', 'summaryDigest',
    'checkpointDigest', 'shadowedItemCount', 'tokenCountKind', 'tokenEstimatorVersion', 'createdAt', 'finishedAt', 'turn'
  ], validateCompactionEvent),
  compaction_failed: eventContract(['auto', 'compactionId', 'schemaVersion', 'reasonCode', 'finishedAt'], validateCompactionEvent),
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

const PIPELINE_LABEL_BY_STAGE: Readonly<Record<string, string>> = {
  setup: 'Setup',
  pre_start: 'Pre-Start',
  post_start: 'Post-Start',
  logical_step_started: 'Logical step started',
  logical_step_assembled: 'Logical step assembled',
  logical_step_closed: 'Logical step closed',
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

const INITIAL_USER_SKILL_ID_PATTERN = /^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/

function isInitialUserSkillIdList(value: unknown): boolean {
  // Go omits the field when no skill was loaded; exposed selections are nonempty.
  if (!Array.isArray(value) || value.length === 0 || value.length > 16) return false
  const seen = new Set<string>()
  for (const id of value) {
    if (typeof id !== 'string' || INITIAL_USER_SKILL_ID_PATTERN.exec(id)?.[0] !== id || seen.has(id)) return false
    seen.add(id)
  }
  return true
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

function isClosedItemBase(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  if (!isSafePublicId(event.turnId)) return invalidValidation(diagnostic, 'item_binding', 'invalid_field', ['turnId'])
  if (!isSafePublicId(event.itemId)) return invalidValidation(diagnostic, 'item_binding', 'invalid_field', ['itemId'])
  if (item.threadId !== event.threadId) return invalidValidation(diagnostic, 'item_binding', 'binding_mismatch', ['item.threadId'])
  if (item.turnId !== event.turnId) return invalidValidation(diagnostic, 'item_binding', 'binding_mismatch', ['item.turnId'])
  if (item.id !== event.itemId) return invalidValidation(diagnostic, 'item_binding', 'binding_mismatch', ['item.id'])
  if (!isCanonicalTimestamp(item.createdAt)) return invalidValidation(diagnostic, 'item_binding', 'invalid_field', ['item.createdAt'])
  if (item.finishedAt !== undefined && !isCanonicalTimestamp(item.finishedAt)) {
    return invalidValidation(diagnostic, 'item_binding', 'invalid_field', ['item.finishedAt'])
  }
  return true
}

const CLOSED_USER_FILE_REFERENCES_SCHEMA = UserFileReferenceSchema.strict().array().max(4096)

function isClosedUserMessageItem(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  const allowed = [
    ...ITEM_BASE_KEYS, 'text', 'displayText', 'delivery', 'clientUserMessageId', 'attachmentIds',
    'workspaceCheckpointId', 'source', 'fileReferences', 'activeSkillIds'
  ]
  if (!hasOnlyKeys(item, allowed)) return invalidValidation(diagnostic, 'item_contract', 'unknown_fields', ['item'])
  if (item.source !== undefined && !GoalRoundSourceSchemaV1.safeParse(item.source).success) return false
  if (!isClosedItemBase(event, item, diagnostic)) return false
  if (item.role !== 'user') return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.role'])
  if (item.status !== 'completed') return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.status'])
  if (typeof item.text !== 'string' || item.text.length > 1_000_000) {
    return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.text'])
  }
  if (item.displayText !== undefined && typeof item.displayText !== 'string') {
    return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.displayText'])
  }
  if (item.fileReferences !== undefined && !schemaIsValid(
    CLOSED_USER_FILE_REFERENCES_SCHEMA.safeParse(item.fileReferences), diagnostic, 'item_schema', 'item'
  )) return false
  if (item.delivery !== undefined && item.delivery !== 'steer') {
    return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.delivery'])
  }
  if ('activeSkillIds' in item && (item.delivery === 'steer' || !isInitialUserSkillIdList(item.activeSkillIds))) {
    return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.activeSkillIds'])
  }
  if (!isOptionalSafePublicId(item.clientUserMessageId)) {
    return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.clientUserMessageId'])
  }
  if (!isOptionalSafePublicId(item.workspaceCheckpointId)) {
    return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.workspaceCheckpointId'])
  }
  return item.attachmentIds === undefined || isSafeIdList(item.attachmentIds) ||
    invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.attachmentIds'])
}

function isClosedErrorItem(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  const allowed = [...ITEM_BASE_KEYS, 'message', 'code', 'severity']
  return (hasOnlyKeys(item, allowed) || invalidValidation(diagnostic, 'item_contract', 'unknown_fields', ['item'])) &&
    isClosedItemBase(event, item, diagnostic) && item.role === 'system' &&
    new Set(['completed', 'failed', 'aborted']).has(String(item.status)) &&
    isClosedHostError(item.code, item.message) && isOptionalEnum(item.severity, new Set(['info', 'warning', 'error']))
}

function isClosedToolProgressItem(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  return isClosedItemBase(event, item, diagnostic) &&
    schemaIsValid(ToolProgressTurnItem.safeParse(item), diagnostic, 'item_schema', 'item')
}

function isClosedApprovalItem(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  return (hasOnlyKeys(item, [...ITEM_BASE_KEYS, 'approvalId', 'toolName', 'summary']) ||
      invalidValidation(diagnostic, 'item_contract', 'unknown_fields', ['item'])) &&
    isClosedItemBase(event, item, diagnostic) && item.role === 'tool' && isSafePublicId(item.approvalId) &&
    isSafeToolName(item.toolName) &&
    schemaIsValid(ApprovalTurnItem.safeParse(item), diagnostic, 'item_schema', 'item')
}

function isClosedUserInputItem(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  return (hasOnlyKeys(item, [...ITEM_BASE_KEYS, 'inputId', 'prompt', 'questions', 'timedWait']) ||
      invalidValidation(diagnostic, 'item_contract', 'unknown_fields', ['item'])) &&
    isClosedItemBase(event, item, diagnostic) &&
    item.role === 'system' && isSafePublicId(item.inputId) &&
    schemaIsValid(UserInputTurnItem.safeParse(item), diagnostic, 'item_schema', 'item')
}

function isClosedCompactionItem(
  event: Record<string, unknown>, item: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  if (item.schemaVersion === 5) {
    return isClosedItemBase(event, item, diagnostic) &&
      schemaIsValid(CompactionTurnItem.safeParse(item), diagnostic, 'item_schema', 'item')
  }
  const allowed = [
    ...ITEM_BASE_KEYS, 'summary', 'replacedTokens', 'auto', 'sourceDigest', 'digestMarker',
    'sourceItemIds', 'pinnedConstraints', 'schemaVersion', 'reasoningExcluded', 'reasoningExclusionProof',
    'assistantProseExcluded', 'toolPayloadsExcluded', 'caseFactsExcluded',
    'providerHistoryProjectionVersion', 'caseHistoryProjectionVersion', 'taskContinuation'
  ]
  const continuationV4 = item.schemaVersion === 4
  const publicCaseV2 = item.schemaVersion === 3 && item.caseHistoryProjectionVersion === 2
  return (hasOnlyKeys(item, allowed) || invalidValidation(diagnostic, 'item_contract', 'unknown_fields', ['item'])) &&
    isClosedItemBase(event, item, diagnostic) && item.role === 'system' &&
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
      ? item.auto === true && schemaIsValid(TaskContinuationSnapshotV1Schema.safeParse(item.taskContinuation),
        diagnostic, 'item_schema', 'item.taskContinuation')
      : item.taskContinuation === undefined)
}

function isClosedItem(event: Record<string, unknown>, diagnostic?: DiagnosticRecorder): boolean {
  if (!isRecord(event.item)) return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item'])
  const item = event.item
  switch (item.kind) {
    case 'user_message':
      return isClosedUserMessageItem(event, item, diagnostic)
    case 'tool_call':
      return isClosedItemBase(event, item, diagnostic) &&
        schemaIsValid(ToolCallTurnItem.safeParse(item), diagnostic, 'item_schema', 'item')
    case 'tool_result': {
      if (!isClosedItemBase(event, item, diagnostic)) return false
      return schemaIsValid(ToolResultTurnItem.safeParse(item), diagnostic, 'item_schema', 'item')
    }
    case 'approval':
      return isClosedApprovalItem(event, item, diagnostic)
    case 'user_input':
      return isClosedUserInputItem(event, item, diagnostic)
    case 'compaction':
      return isClosedCompactionItem(event, item, diagnostic)
    case 'error':
      return isClosedErrorItem(event, item, diagnostic)
    case 'tool_progress':
      return isClosedToolProgressItem(event, item, diagnostic)
    default:
      // Assistant/review prose is never an ordinary live item. Accepted
      // assistant output is carried only by a sealed accepted-final batch.
      return invalidValidation(diagnostic, 'item_contract', 'unsupported_item_kind', ['item.kind'])
  }
}

function validateItemLifecycleEvent(event: Record<string, unknown>, diagnostic?: DiagnosticRecorder): boolean {
  if (!isSafePublicId(event.turnId)) return invalidValidation(diagnostic, 'item_binding', 'invalid_field', ['turnId'])
  if (event.item !== undefined) {
    for (const key of ['stage', 'callId', 'toolName', 'status', 'message', 'child', 'details'] as const) {
      if (event[key] !== undefined) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', [key])
    }
    if (!isClosedItem(event, diagnostic)) return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item'])
    const item = event.item as Record<string, unknown>
    // A completed host error is part of an accepted-final or general-terminal
    // suffix. Exposing the same item as a detached ordinary frame would let a
    // spoofed/legacy stream advance the desktop cursor without the matching
    // usage and terminal disposition.
    if (item.kind === 'error') return invalidValidation(diagnostic, 'item_contract', 'detached_terminal_item', ['item.kind'])
    if (event.kind === 'tool_call_started') {
      if (item.kind !== 'tool_call') return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.kind'])
      return item.status === 'running' || invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.status'])
    }
    if (event.kind === 'tool_call_finished') {
      if (item.kind !== 'tool_result') return invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.kind'])
      return new Set(['completed', 'failed']).has(String(item.status)) ||
        invalidValidation(diagnostic, 'item_contract', 'invalid_field', ['item.status'])
    }
    return new Set(['item_created', 'item_updated', 'item_completed']).has(String(event.kind))
  }
  if (!new Set(['item_created', 'item_updated', 'item_completed']).has(String(event.kind))) {
    return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['kind'])
  }
  if (event.message !== 'child output withheld') return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['message'])
  if (!isOptionalSafePublicId(event.callId)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['callId'])
  if (!isOptionalSafeIdentifier(event.toolName)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['toolName'])
  if (!isOptionalEnum(event.status, JOB_STATUSES)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['status'])
  if (event.stage !== undefined && !isClosedChildStage(event.stage)) {
    return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['stage'])
  }
  if (event.itemId !== undefined && !isSafePublicId(event.itemId)) {
    return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['itemId'])
  }
  const child = event.child !== undefined && isClosedChildMetadata(event.child)
  const details = event.details !== undefined && isClosedChildMetadata(event.details)
  return Boolean(child || details) || invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['child', 'details'])
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

function validateApprovalEvent(event: Record<string, unknown>, diagnostic?: DiagnosticRecorder): boolean {
  const allowed = event.kind === 'approval_requested'
    ? new Set(['pending'])
    : new Set(['allowed', 'denied', 'expired'])
  return eventHasTurnWithOptionalItem(event) && isSafePublicId(event.approvalId) && isSafeToolName(event.toolName) &&
    typeof event.status === 'string' && allowed.has(event.status) &&
    isOptionalEnum(event.approvalPolicy, APPROVAL_POLICIES) && isOptionalEnum(event.sandboxMode, SANDBOX_MODES) &&
    schemaIsValid(ApprovalEvent.safeParse(event), diagnostic, 'event_schema', '$')
}

function validateUserInputEvent(event: Record<string, unknown>, diagnostic?: DiagnosticRecorder): boolean {
  const allowed = event.kind === 'user_input_requested'
    ? new Set(['pending'])
    : new Set(['submitted', 'cancelled', 'continued'])
  return eventHasTurnWithOptionalItem(event) && isSafePublicId(event.inputId) &&
    typeof event.status === 'string' && allowed.has(event.status) &&
    schemaIsValid(UserInputEvent.safeParse(event), diagnostic, 'event_schema', '$')
}

function validateCompactionEvent(event: Record<string, unknown>): boolean {
  if (event.schemaVersion === 3) return eventHasTurnWithOptionalItem(event) && CompactionEvent.safeParse(event).success
  if (!eventHasTurn(event)) return false
  if (event.kind === 'compaction_failed') return false
  if (['compactionId', 'summaryKind', 'summaryDigest', 'checkpointDigest', 'shadowedItemCount', 'tokenCountKind',
    'tokenEstimatorVersion', 'turn', 'createdAt', 'startedAt', 'finishedAt', 'reasonCode'].some(key => event[key] !== undefined)) return false
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
  if (event.stage === 'logical_step_started' || event.stage === 'logical_step_assembled' || event.stage === 'logical_step_closed') return isClosedLogicalStepEvent(event)
  if (!eventHasTurn(event) || !isOptionalSafeSequence(event.attempt) || !isOptionalSafeSequence(event.maxAttempt)) return false
  const hasChild = event.child !== undefined || (event.details !== undefined && isClosedChildMetadata(event.details))
  if (hasChild) {
    return event.message === 'child output withheld' && isClosedChildStage(event.stage) &&
      event.label === undefined && (event.child === undefined || isClosedChildMetadata(event.child)) &&
      (event.details === undefined || isClosedChildMetadata(event.details))
  }
  const label = pipelineLabel(event.stage)
  return label !== null && event.label === label && event.message === undefined &&
    (event.details === undefined || isClosedPipelineDetails(event.details, event.stage)) &&
    (!isRecord(event.details) || event.details.retry === undefined || (
      isSafeSequence(event.attempt) && event.attempt > 0 && event.attempt <= 1e15 &&
      isSafeSequence(event.maxAttempt) && event.maxAttempt >= event.attempt && event.maxAttempt <= 1e15 &&
      isRecord(event.details.retry) && event.attempt === (event.details.retry.source === 'transport_reconnect'
        ? event.details.retry.physicalAttempt : event.details.retry.outerAttempt)
    ))
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
      !isOptionalSafeIdentifier(event.providerId) || !(event.endpointFormat === undefined || isPublicProviderEndpointFormat(event.endpointFormat)) ||
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
  if (!isPublicHostErrorCode(codeValue) || typeof messageValue !== 'string') return false
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
    (value.endpointFormat === undefined || isPublicProviderEndpointFormat(value.endpointFormat)) &&
    isOptionalEnum(value.kind, new Set([
      'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
      'invalid_model', 'unknown'
    ])) && isOptionalEnum(value.authStatus, new Set(['none', 'required'])) &&
    (value.failureStage === undefined || isPublicProviderFailureStage(value.failureStage)) &&
    (value.dispatchState === undefined || isPublicProviderDispatchState(value.dispatchState)) &&
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
    (value.endpointFormat === undefined || isPublicProviderEndpointFormat(value.endpointFormat)) &&
    isOptionalEnum(value.kind, new Set([
      'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
      'invalid_model', 'unknown'
    ])) && isOptionalEnum(value.authStatus, new Set(['none', 'possible', 'required', 'unknown'])) &&
    isOptionalBoolean(value.hasApiKey) && isOptionalBoolean(value.retryable) &&
    (value.failureStage === undefined || isPublicProviderFailureStage(value.failureStage)) &&
    (value.dispatchState === undefined || isPublicProviderDispatchState(value.dispatchState)) &&
    isOptionalSafeSequence(value.attempt) && isOptionalSafeSequence(value.maxAttempt) &&
    (value.port === undefined || (isSafeSequence(value.port) && value.port <= 65_535)) &&
    isOptionalEnum(value.backend, new Set(['go', 'runtime-go', 'go-runtime-default'])) &&
    isOptionalEnum(value.requestedBackend, new Set(['', 'go', 'runtime-go', 'go-runtime-default'])) &&
    isOptionalBoolean(value.autoStart) &&
    isOptionalEnum(value.phase, new Set(['setup', 'highest_seq', 'replay', 'live']))
}

function isClosedAcceptedFinalDeliveryBatch(event: Record<string, unknown>, diagnostic?: DiagnosticRecorder): boolean {
  const allowed = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'schemaVersion', 'purpose', 'batchId',
    'firstSeq', 'lastSeq', 'publicationCommitId', 'eventManifestDigest', 'publicationAuthority',
    'events', 'trace'
  ]
  if (!schemaIsValid(AcceptedFinalDeliveryBatchV2Schema.safeParse(event), diagnostic, 'event_schema', '$') ||
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

function isClosedGeneralTerminalDeliveryBatch(event: Record<string, unknown>, diagnostic?: DiagnosticRecorder): boolean {
  const allowed = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'schemaVersion', 'purpose',
    'batchDigest', 'firstSeq', 'lastSeq', 'generalTerminalCommitId',
    'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest', 'eventManifestDigest',
    'projectedEventsDigest', 'transportAuthority', 'evidenceAuthority', 'citationAuthority',
    'factAnswerAllowed', 'events', 'eventManifest'
  ]
  const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(event)
  if (!schemaIsValid(parsed, diagnostic, 'event_schema', '$') ||
      containsInternalCaseEntityReference(event) || !hasOnlyKeys(event, allowed) ||
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
          (typeof item.text !== 'string' || containsUnboundCaseRiskV1(item.text))) return false
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
  return validateClosedPublicRuntimeSseEvent(event)
}

function validateClosedPublicRuntimeSseEvent(
  event: Record<string, unknown>, diagnostic?: DiagnosticRecorder
): boolean {
  const kind = typeof event.kind === 'string' ? event.kind : ''
  if (kind === 'goal_activation_changed') {
    return GoalActivationChangedEventV1.safeParse(event).success && isSafePublicId(event.threadId) &&
      isCanonicalTimestamp(event.timestamp) && isPublicSseIpcPayload({ streamId: 'goal-live-validation', events: [event] })
  }
  if (kind === 'public_projection_revoked') {
    return schemaIsValid(PublicProjectionRevokedEventSchema.safeParse(event), diagnostic, 'event_schema', '$')
  }
  if (kind === 'accepted_final_batch') return isClosedAcceptedFinalDeliveryBatch(event, diagnostic)
  if (kind === 'general_terminal_batch') return isClosedGeneralTerminalDeliveryBatch(event, diagnostic)
  const contract = PUBLIC_EVENT_CONTRACTS[kind]
  if (!contract) return invalidValidation(diagnostic, 'event_contract', 'unsupported_event_kind', ['kind'])
  if (!hasOnlyKeys(event, [...BASE_EVENT_KEYS, ...contract.keys])) {
    return invalidValidation(diagnostic, 'event_contract', 'unknown_fields', ['$'])
  }
  if (!isSafeSequence(event.seq)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['seq'])
  if (!isCanonicalTimestamp(event.timestamp)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['timestamp'])
  if (!isSafePublicId(event.threadId)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['threadId'])
  if (event.turnId !== undefined && !isSafePublicId(event.turnId)) {
    return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['turnId'])
  }
  if (event.itemId !== undefined && !isSafePublicId(event.itemId)) {
    return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['itemId'])
  }
  if (!isClosedTrace(event.trace)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['trace'])
  if (!contract.validate(event, diagnostic)) return invalidValidation(diagnostic, 'event_contract', 'invalid_field', ['$'])
  return isPublicSseIpcPayload({ streamId: 'strict-sse-validation', events: [event] }) ||
    invalidValidation(diagnostic, 'public_content', 'public_content_rejected', ['$'])
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
  filter: PublicRuntimeEventFilter,
  onDiagnostic?: (diagnostic: PublicRuntimeSseDecodeDiagnostic) => void
): PublicRuntimeSseDecision | null {
  let firstDiagnostic: PublicRuntimeSseDecodeDiagnostic | undefined
  const recordDiagnostic = (validation: PublicRuntimeSseDecodeDiagnostic['validation']): DiagnosticRecorder =>
    (stage, reason, fieldPaths) => {
      if (!onDiagnostic || firstDiagnostic) return
      firstDiagnostic = Object.freeze({
        validation, stage, reason, fieldPaths: Object.freeze([...fieldPaths].slice(0, 12))
      })
    }
  const inputDiagnostic = onDiagnostic ? recordDiagnostic('input') : undefined
  const projectedDiagnostic = onDiagnostic ? recordDiagnostic('projected') : undefined
  const reject = (
    decision: Extract<PublicRuntimeSseDecision, { status: 'invalid' | 'withheld' }>,
    stage: DiagnosticStage,
    reason: DiagnosticReason,
    fieldPaths: readonly DiagnosticFieldPath[],
    validation: PublicRuntimeSseDecodeDiagnostic['validation'] = 'input'
  ): PublicRuntimeSseDecision => {
    // A specific first-validation failure takes precedence over its caller's
    // generic fallback. Observing it must not change the stream decision.
    recordDiagnostic(validation)(stage, reason, fieldPaths)
    if (firstDiagnostic && onDiagnostic) {
      try {
        onDiagnostic(firstDiagnostic)
      } catch {
        // Observer errors do not change the decoder decision.
      }
    }
    return decision
  }
  const trimmed = block.trim()
  if (!trimmed || trimmed.split(/\r?\n/).every((line) => !line || line.startsWith(':'))) return null
  const parsed = parseStrictFrame(block)
  if (!parsed) return reject({ status: 'invalid', reason: 'malformed_frame' }, 'frame', 'malformed_frame', ['$'])
  if (parsed.event === 'public_projection_revoked') {
    if (parsed.id !== undefined || parsed.data.kind !== parsed.event) {
      return reject({ status: 'invalid', reason: 'malformed_frame' }, 'frame', 'malformed_frame', ['$'])
    }
    // Keep this parse at the original revocation gate; do not revalidate for
    // diagnosis after a boolean failure.
    const revocation = PublicProjectionRevokedEventSchema.safeParse(parsed.data)
    if (!schemaIsValid(revocation, inputDiagnostic, 'event_schema', '$')) {
      return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'event_schema', 'invalid_field', ['$'])
    }
    if (parsed.data.threadId !== expectedThreadId) {
      return reject({ status: 'invalid', reason: 'event_thread_mismatch' }, 'transport_binding', 'binding_mismatch', ['threadId'])
    }
    // The original guard narrows this value to the same schema type. The
    // parsed output is intentionally not substituted for the transport item.
    return { status: 'revoke', event: parsed.data as PublicProjectionRevokedEvent }
  }
  if (parsed.event === 'goal_activation_changed') {
    if (parsed.id !== undefined || parsed.data.kind !== parsed.event || parsed.data.threadId !== expectedThreadId ||
        !validateClosedPublicRuntimeSseEvent(parsed.data)) {
      return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'event_contract', 'invalid_field', ['$'])
    }
    const projected = filter.push(parsed.data)
    if (!projected || !validateClosedPublicRuntimeSseEvent(projected)) {
      return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'projection_filter', 'projection_rejected', ['$'])
    }
    // Zero is only the decoder's non-durable scheduling sentinel. It never
    // enters the payload, transport id, ACK cursor or persisted history.
    return { status: 'emit', event: projected, seq: 0 }
  }
  if (parsed.id === undefined) return reject({ status: 'invalid', reason: 'missing_transport_binding' },
    'transport_binding', 'missing_transport_binding', ['seq'])
  const seq = Number(parsed.id)
  if (!Number.isSafeInteger(seq)) return reject({ status: 'invalid', reason: 'missing_transport_binding' },
    'transport_binding', 'missing_transport_binding', ['seq'])
  if (parsed.data.kind !== parsed.event) return reject({ status: 'invalid', reason: 'event_kind_mismatch' },
    'transport_binding', 'binding_mismatch', ['kind'])
  if (parsed.data.seq !== seq || !isSafeSequence(parsed.data.seq)) {
    return reject({ status: 'invalid', reason: 'event_sequence_mismatch' }, 'transport_binding', 'binding_mismatch', ['seq'])
  }
  if (parsed.data.threadId !== expectedThreadId || !isExactNonEmptyString(parsed.data.threadId)) {
    return reject({ status: 'invalid', reason: 'event_thread_mismatch' }, 'transport_binding', 'binding_mismatch', ['threadId'])
  }
  if (PRIVATE_EVENT_KINDS.has(parsed.event)) {
    return reject({ status: 'withheld', seq, reason: 'restricted_content' }, 'public_content', 'restricted_content', ['kind'])
  }
  if (!PUBLIC_EVENT_CONTRACTS[parsed.event]) {
    return reject({ status: 'invalid', reason: 'unsupported_event_kind' }, 'event_contract', 'unsupported_event_kind', ['kind'])
  }
  // Validate the host-owned closed event before the generic public-content
  // filter. The filter may withhold private fields, but it must never turn an
  // open provider/tool/MCP payload into a new public contract.
  if (!validateClosedPublicRuntimeSseEvent(parsed.data, inputDiagnostic)) {
    return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'event_contract', 'invalid_field', ['$'])
  }

  const projected = filter.push(parsed.data)
  if (!projected) {
    const item = isRecord(parsed.data.item) ? parsed.data.item : undefined
    const privateKind = parsed.event === 'assistant_reasoning' ||
      parsed.event === 'assistant_reasoning_delta' ||
      parsed.event === 'agent_reasoning' ||
      item?.kind === 'assistant_reasoning'
    return privateKind || parsed.event === 'assistant_text_delta'
      ? reject({ status: 'withheld', seq, reason: 'restricted_content' },
        'projection_filter', 'restricted_content', ['$'], 'projected')
      : reject({ status: 'invalid', reason: 'invalid_public_projection' },
        'projection_filter', 'projection_rejected', ['$'], 'projected')
  }
  if (projected.kind !== parsed.event) {
    return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'projected_binding', 'binding_mismatch', ['kind'], 'projected')
  }
  if (projected.seq !== seq) {
    return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'projected_binding', 'binding_mismatch', ['seq'], 'projected')
  }
  if (projected.threadId !== expectedThreadId) {
    return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'projected_binding', 'binding_mismatch', ['threadId'], 'projected')
  }
  if (!validateClosedPublicRuntimeSseEvent(projected, projectedDiagnostic)) {
    return reject({ status: 'invalid', reason: 'invalid_public_projection' }, 'event_contract', 'invalid_field', ['$'], 'projected')
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
