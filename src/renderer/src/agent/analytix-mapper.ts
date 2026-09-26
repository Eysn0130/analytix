import type {
  ChatBlock,
  AcceptedFinalProjectionBatch,
  AcceptedFinalProjectionReceiptV1,
  CompactionEventPayload,
  NormalizedCaseProject,
  NormalizedThread,
  ReviewBlock,
  ReviewEventPayload,
  ReviewOutput,
  ReviewTarget,
  RuntimeChildMetadata,
  RuntimeCacheDiagnosticsMetadata,
  RuntimeDisclosureMetadata,
  RuntimeErrorEventPayload,
  SnapshotRequiredEventPayload,
  RuntimeStatusEventPayload,
  ThreadGoal,
  ThreadTodoList,
  UserInputRequestPayload,
  UserMessageEventPayload,
  ThreadEventSink,
  RuntimeJobDiagnosticsMetadata,
  RuntimeProviderErrorDiagnosticsMetadata,
  GeneralTerminalProjectionBatch,
  ThreadUsageSnapshot,
  ToolBlock,
  ToolEventPayload,
  UserInputQuestion
} from './types'
import {
  acceptedFinalProjectionReceiptFromBatch,
  acceptedFinalProjectionReceiptsEqual
} from './accepted-final-projection-receipt'
import { redactSecrets, redactSecretText } from '@shared/secret-redaction'
import {
  containsInternalCaseEntityReference,
  containsProtectedCaseFactCandidate,
  projectOrdinaryPublicText
} from '@shared/ordinary-log-pii-projection'
import { sanitizePublicSerializedText } from '@shared/public-runtime-content'
import {
  AcceptedFinalPublicViewV3Schema,
  PublicToolCallArgumentsProjectionV1,
  PublicToolResultProjectionV1
} from '../../../../packages/runtime/src/contracts/items.js'
import {
  AcceptedFinalDeliveryBatchV2Schema,
  type AcceptedFinalDeliveryBatchV2,
  GeneralTerminalDeliveryBatchV1Schema
} from '../../../../packages/runtime/src/contracts/events.js'
import type {
  CoreChildRuntimeMetadataJson,
  CoreRuntimeEventJson,
  CoreRuntimeEventKind,
  CoreCaseProjectJson,
  CoreCacheDiagnosticsJson,
  CoreThreadGoalJson,
  CoreThreadTodoListJson,
  CoreThreadSummaryJson,
  CoreTurnItemJson,
  CoreReviewOutputJson,
  CoreReviewTargetJson,
  CoreUsageSnapshotJson
} from './analytix-contract'
import {
  getDefaultThreadTitle,
  isInternalPlaceholderThreadTitle
} from '../lib/thread-title'

const RUNTIME_EVENT_KINDS_DISPATCHED_BY_RENDERER = [
  'thread_created',
  'thread_updated',
  'thread_rewound',
  'turn_started',
  'turn_completed',
  'turn_failed',
  'turn_aborted',
  'turn_steered',
  'item_created',
  'item_updated',
  'item_completed',
  'tool_call_ready',
  'tool_progress',
  'tool_result_upload_wait',
  'tool_storm_suppressed',
  'tool_catalog_changed',
  'child_steer_queued',
  'child_steer_admitted',
  'child_steer_rejected',
  'child_pause_requested',
  'child_paused',
  'child_resume_requested',
  'child_resumed',
  'child_pause_rejected',
  'tool_call_started',
  'tool_call_finished',
  'approval_requested',
  'approval_resolved',
  'user_input_requested',
  'user_input_resolved',
  'compaction_started',
  'compaction_completed',
  'goal_updated',
  'goal_cleared',
  'todos_updated',
  'todos_cleared',
  'pipeline_stage',
  'usage',
  'error',
  'snapshot_required'
] as const satisfies readonly CoreRuntimeEventKind[]

const RUNTIME_EVENT_KINDS_IGNORED_BY_RENDERER = [
  'assistant_text_delta',
  'mcp_lifecycle_audit',
  'autoresearch_state_audit',
  'goal_evidence_audit',
  'checkpoint_captured',
  'checkpoint_rewind_rescue_created',
  'checkpoint_rewind_applied',
  'heartbeat'
] as const satisfies readonly CoreRuntimeEventKind[]

export const RUNTIME_EVENT_KINDS_COVERED_BY_RENDERER = [
  ...RUNTIME_EVENT_KINDS_DISPATCHED_BY_RENDERER,
  ...RUNTIME_EVENT_KINDS_IGNORED_BY_RENDERER
] as const satisfies readonly CoreRuntimeEventKind[]

export function buildQuery(options: Record<string, string | number | boolean | undefined>): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(options)) {
    if (value == null) continue
    if (typeof value === 'string' && !value.trim()) continue
    params.set(key, String(value))
  }
  const query = params.toString()
  return query ? `?${query}` : ''
}

function normalizeThreadTitleFromCore(title: string | undefined, fallbackId: string): string {
  const rawTitle = title?.trim() ?? ''
  return isInternalPlaceholderThreadTitle(rawTitle)
    ? getDefaultThreadTitle()
    : projectOrdinaryPublicText(rawTitle) || fallbackId.slice(0, 8)
}

export function threadFromCore(thread: CoreThreadSummaryJson): NormalizedThread {
  return {
    id: thread.id,
    title: normalizeThreadTitleFromCore(thread.title, thread.id),
    updatedAt: thread.updatedAt,
    model: thread.model,
    providerId: thread.providerId,
    mode: thread.mode,
    workspace: thread.workspace,
    status: thread.status,
    approvalPolicy: normalizeApprovalPolicy(thread.approvalPolicy),
    sandboxMode: normalizeSandboxMode(thread.sandboxMode),
    archived: thread.status === 'archived',
    preview: thread.preview ? projectOrdinaryPublicText(thread.preview) : thread.preview,
    messageCount: thread.messageCount,
    turnCount: thread.turnCount,
    latestTurnId: thread.latestTurnId,
    historyAuthority: thread.historyAuthority,
    relation: thread.relation,
    parentThreadId: thread.parentThreadId,
    forkedFromThreadId: thread.forkedFromThreadId,
    forkedFromTitle: thread.forkedFromTitle,
    forkedAt: thread.forkedAt,
    forkedFromMessageCount: thread.forkedFromMessageCount,
    forkedFromTurnCount: thread.forkedFromTurnCount,
    goal: thread.goal ? goalFromCore(thread.goal) : null,
    todos: thread.todos ? todosFromCore(thread.todos) : null
	  }
	}

export function caseProjectFromCore(project: CoreCaseProjectJson): NormalizedCaseProject {
  return {
    id: project.id,
    name: project.name?.trim() || project.rootPath || project.id,
    rootPath: project.rootPath,
    updatedAt: project.updatedAt,
    threadCount: Number(project.threadCount ?? 0),
    runningCount: Number(project.runningCount ?? 0),
    archivedCount: Number(project.archivedCount ?? 0),
    lastThreadId: project.lastThreadId,
    lastPreview: project.lastPreview,
    dataSizeEstimate: Number(project.dataSizeEstimate ?? 0),
    status: project.status
  }
}

function normalizeApprovalPolicy(value: string | undefined): NormalizedThread['approvalPolicy'] {
  switch (value) {
    case 'always':
    case 'auto':
    case 'on-request':
    case 'untrusted':
    case 'suggest':
    case 'never':
      return value
    default:
      return undefined
  }
}

function normalizeSandboxMode(value: string | undefined): NormalizedThread['sandboxMode'] {
  switch (value) {
    case 'read-only':
    case 'workspace-write':
    case 'danger-full-access':
    case 'external-sandbox':
      return value
    default:
      return undefined
  }
}

export function goalFromCore(goal: CoreThreadGoalJson): ThreadGoal {
  const evidenceLedger = goal.evidenceLedger?.map((entry) => ({
    id: entry.id,
    ...(entry.turnId ? { turnId: entry.turnId } : {}),
    ...(entry.toolCallId ? { toolCallId: entry.toolCallId } : {}),
    ...(entry.requirementId ? { requirementId: entry.requirementId } : {}),
    step: entry.step,
    evidence: entry.evidence ?? [],
    ...(entry.summary ? { summary: entry.summary } : {}),
    createdAt: entry.createdAt
  }))
  return {
    threadId: goal.threadId,
    objective: goal.objective,
    status: goal.status,
    tokenBudget: goal.tokenBudget ?? null,
    tokensUsed: goal.tokensUsed ?? 0,
    timeUsedSeconds: goal.timeUsedSeconds ?? 0,
    ...(evidenceLedger ? { evidenceLedger } : {}),
    ...(goal.research ? { research: goal.research } : {}),
    ...(goal.blockedReason ? { blockedReason: goal.blockedReason } : {}),
    ...(goal.blockedCount ? { blockedCount: goal.blockedCount } : {}),
    ...(goal.blockedTurnId ? { blockedTurnId: goal.blockedTurnId } : {}),
    ...(goal.strictCompletion ? { strictCompletion: goal.strictCompletion } : {}),
    ...(goal.selfCheckRequired ? { selfCheckRequired: goal.selfCheckRequired } : {}),
    ...(goal.selfCheckCompleted ? { selfCheckCompleted: goal.selfCheckCompleted } : {}),
    ...(goal.selfCheckTurnId ? { selfCheckTurnId: goal.selfCheckTurnId } : {}),
    createdAt: goal.createdAt,
    updatedAt: goal.updatedAt
  }
}

export function todosFromCore(todos: CoreThreadTodoListJson): ThreadTodoList {
  return {
    threadId: todos.threadId,
    ...(todos.turnId ? { turnId: todos.turnId } : {}),
    items: (todos.items ?? []).map((item) => ({
      id: item.id,
      content: item.content,
      status: item.status,
      ...(item.statusReasonCode ? { statusReasonCode: item.statusReasonCode } : {}),
      ...(item.source ? { source: { ...item.source } } : {}),
      ...(item.note ? { note: item.note } : {}),
      createdAt: item.createdAt,
      updatedAt: item.updatedAt
    })),
    updatedAt: todos.updatedAt
  }
}

function itemCreatedAt(item: CoreTurnItemJson): string | undefined {
  return item.createdAt || item.finishedAt
}

function toolStatus(item: CoreTurnItemJson): ToolBlock['status'] {
  if (item.isError || item.status === 'failed' || item.status === 'aborted') return 'error'
  if (item.status === 'pending' || item.status === 'running') return 'running'
  return 'success'
}

function outputText(value: unknown): string {
  if (typeof value === 'string') return value
  if (value == null) return ''
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function publicToolResultDetail(output: unknown): string {
  const parsed = PublicToolResultProjectionV1.safeParse(output)
  if (!parsed.success) return ''
  return parsed.data.code
    ? `${parsed.data.messageKey} (${parsed.data.code})`
    : parsed.data.messageKey
}

function childProgressItemId(child: Pick<RuntimeChildMetadata, 'kind' | 'childRunId' | 'childId'> | undefined): string | undefined {
  const childId = child?.childRunId?.trim() || child?.childId?.trim()
  if (!childId) return undefined
  return child?.kind === 'background-shell'
    ? `background_shell_${childId}`
    : `subagent_${childId}`
}

function toolBlockId(item: CoreTurnItemJson, child?: Pick<RuntimeChildMetadata, 'kind' | 'childRunId' | 'childId'>): string {
  if (item.kind === 'tool_progress') {
    const childItemId = childProgressItemId(child)
    if (childItemId) return childItemId
  }
  return item.callId?.trim() ? `tool_${item.callId}` : item.id
}

function stringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const strings = value.filter((entry): entry is string => typeof entry === 'string' && entry.trim().length > 0)
  return strings.length > 0 ? strings : undefined
}

function readStructuredString(record: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return undefined
}

function readStructuredNumber(record: Record<string, unknown> | undefined, ...keys: string[]): number | undefined {
  if (!record) return undefined
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'number' && Number.isFinite(value)) return value
  }
  return undefined
}

function readStructuredBoolean(record: Record<string, unknown> | undefined, ...keys: string[]): boolean | undefined {
  if (!record) return undefined
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'boolean') return value
  }
  return undefined
}

function readRuntimeEventString(event: CoreRuntimeEventJson, ...keys: string[]): string | undefined {
  return readStructuredString(event as Record<string, unknown>, ...keys)
}

function readRuntimeEventNumber(event: CoreRuntimeEventJson, ...keys: string[]): number | undefined {
  return readStructuredNumber(event as Record<string, unknown>, ...keys)
}

function readRuntimeEventBoolean(event: CoreRuntimeEventJson, ...keys: string[]): boolean | undefined {
  return readStructuredBoolean(event as Record<string, unknown>, ...keys)
}

function readUsageNumber(usage: CoreUsageSnapshotJson, ...keys: string[]): number | undefined {
  return readStructuredNumber(usage as Record<string, unknown>, ...keys)
}

function readUsageBoolean(usage: CoreUsageSnapshotJson, ...keys: string[]): boolean | undefined {
  return readStructuredBoolean(usage as Record<string, unknown>, ...keys)
}

const FILE_PATH_KEYS = [
  'absolute_path',
  'path',
  'file_path',
  'file',
  'relative_path',
  'target_path',
  'destination_path'
] as const

const COMMAND_KEYS = ['command', 'cmd', 'script'] as const
const COMMAND_RESULT_META_KEYS = [
  'exit_code',
  'session_id',
  'status',
  'pid',
  'shell',
  'cwd',
  'started_at',
  'finished_at',
  'partial',
  'stop_sent'
] as const

const TOOL_KIND_BY_NAME: ReadonlyMap<string, ToolBlock['toolKind']> = new Map([
  ['shell', 'command_execution'],
  ['bash', 'command_execution'],
  ['terminal', 'command_execution'],
  ['run_command', 'command_execution'],
  ['exec', 'command_execution'],
  ['read', 'tool_call'],
  ['write', 'file_change'],
  ['edit', 'file_change'],
  ['grep', 'tool_call'],
  ['find', 'tool_call'],
  ['ls', 'tool_call'],
  ['write_file', 'file_change'],
  ['read_file', 'file_change'],
  ['edit_file', 'file_change'],
  ['multi_edit', 'file_change'],
  ['apply_patch', 'file_change'],
  ['create_file', 'file_change'],
  ['create_plan', 'file_change'],
  ['delegate_task', 'subagent'],
  ['task', 'subagent'],
  ['parallel_tasks', 'subagent']
])

function toolKindForRuntimeTool(
  toolName: string | undefined,
  child?: Pick<RuntimeChildMetadata, 'kind'>
): NonNullable<ToolBlock['toolKind']> {
  if (child?.kind === 'background-shell') return 'command_execution'
  if (child) return 'subagent'
  const normalizedName = toolName?.trim()
  if (!normalizedName) return 'tool_call'
  return TOOL_KIND_BY_NAME.get(normalizedName) ?? 'tool_call'
}

export const RENDERER_TOOL_PROGRESS_SUMMARY = 'Tool activity'
export const RENDERER_SUBAGENT_PROGRESS_SUMMARY = 'Subagent activity'
export const RENDERER_BACKGROUND_PROGRESS_SUMMARY = 'Background task activity'

const RENDERER_IDENTIFIER_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._:@-]{0,255}$/u
const RENDERER_SHA256_PATTERN = /^[a-f0-9]{64}$/iu

const CHILD_STATUS_CODES = [
  'queued',
  'running',
  'pause_requested',
  'paused',
  'resume_requested',
  'resuming',
  'completed',
  'failed',
  'aborted',
  'interrupted',
  'killed'
] as const

const CHILD_STEER_STATUS_CODES = ['queued', 'admitted', 'rejected', 'expired'] as const
const CHILD_PAUSE_STATUS_CODES = [
  'requested',
  'pause_requested',
  'paused',
  'rejected',
  'expired',
  'resumed',
  'resume_requested',
  'resuming',
  'running',
  'failed'
] as const
const CHILD_MERGE_STATUS_CODES = [
  'not_requested',
  'review_requested',
  'clean',
  'conflicted',
  'rejected',
  'accepted',
  'cleaned',
  'repair_checked',
  'repair_rejected'
] as const
const CHILD_REPAIR_DRY_RUN_STATUS_CODES = ['clean', 'conflicted', 'failed'] as const
const CHILD_TODO_SCOPE_CODES = ['child', 'projection'] as const
const CHILD_TODO_PROJECTION_STATUS_CODES = ['proposed', 'accepted', 'rejected', 'superseded'] as const
const CHILD_DECISION_CODES = ['accepted', 'rejected'] as const
const CHILD_EVIDENCE_STATUS_CODES = ['parsed', 'not_found'] as const
const CHILD_TOOL_POLICY_CODES = ['readOnly', 'inherit'] as const
const CHILD_RETURN_FORMAT_CODES = ['summary', 'evidence', 'transcriptRef'] as const
const MODEL_REASONING_EFFORT_CODES = ['auto', 'off', 'low', 'medium', 'high', 'max'] as const

function rendererIdentifier(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return RENDERER_IDENTIFIER_PATTERN.test(normalized) ? normalized : undefined
}

function rendererSha256(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return RENDERER_SHA256_PATTERN.test(normalized) ? normalized.toLowerCase() : undefined
}

function rendererNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : undefined
}

function rendererBoolean(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}

function fixedProgressSummary(child: RuntimeChildMetadata | undefined): string {
  if (child?.kind === 'background-shell') return RENDERER_BACKGROUND_PROGRESS_SUMMARY
  if (child) return RENDERER_SUBAGENT_PROGRESS_SUMMARY
  return RENDERER_TOOL_PROGRESS_SUMMARY
}

function payloadFor(item: CoreTurnItemJson): Record<string, unknown> {
  if (item.kind === 'tool_result') {
    const parsed = PublicToolResultProjectionV1.safeParse(item.output)
    return parsed.success ? parsed.data as Record<string, unknown> : {}
  }
  if (item.kind === 'tool_call') {
    const parsed = PublicToolCallArgumentsProjectionV1.safeParse(item.arguments)
    return parsed.success ? parsed.data as Record<string, unknown> : {}
  }
  if (item.kind === 'tool_progress' && item.arguments && typeof item.arguments === 'object' && !Array.isArray(item.arguments)) {
    const raw = item.arguments as Record<string, unknown>
    return {
      ...(raw.child && typeof raw.child === 'object' && !Array.isArray(raw.child) ? { child: raw.child } : {}),
      ...(raw.diagnostics && typeof raw.diagnostics === 'object' && !Array.isArray(raw.diagnostics) ? { diagnostics: raw.diagnostics } : {})
    }
  }
  // Progress/history arguments have no renderer authority. In particular,
  // command/path-shaped values must not become presentation metadata merely
  // because a stale or non-conforming HTTP snapshot supplied them.
  return {}
}

export function normalizeChildMetadataForRenderer(
  child: CoreChildRuntimeMetadataJson | undefined
): RuntimeChildMetadata | undefined {
  if (!child) return undefined
  const parentThreadId = rendererIdentifier(child.parentThreadId)
  const parentTurnId = rendererIdentifier(child.parentTurnId)
  const childId = rendererIdentifier(child.childId)
  const childStatus = closedDiagnosticCode(child.childStatus, CHILD_STATUS_CODES)
  if (!parentThreadId || !parentTurnId || !childId || !childStatus) return undefined
  const kind = closedDiagnosticCode(child.kind, RUNTIME_JOB_KIND_CODES)
  return {
    ...(kind ? { kind } : {}),
    parentThreadId,
    parentTurnId,
    ...(rendererIdentifier(child.parentToolCallId) ? { parentToolCallId: rendererIdentifier(child.parentToolCallId) } : {}),
    childId,
    ...(rendererIdentifier(child.childRunId) ? { childRunId: rendererIdentifier(child.childRunId) } : {}),
    ...(rendererIdentifier(child.childThreadId) ? { childThreadId: rendererIdentifier(child.childThreadId) } : {}),
    ...(rendererIdentifier(child.childTurnId) ? { childTurnId: rendererIdentifier(child.childTurnId) } : {}),
    childStatus,
    ...(rendererNumber(child.childSeq) !== undefined ? { childSeq: rendererNumber(child.childSeq) } : {}),
    ...(exactClosedDiagnosticCode(child.childEffort, MODEL_REASONING_EFFORT_CODES) ? { childEffort: exactClosedDiagnosticCode(child.childEffort, MODEL_REASONING_EFFORT_CODES) } : {}),
    ...(exactClosedDiagnosticCode(child.effort, MODEL_REASONING_EFFORT_CODES) ? { effort: exactClosedDiagnosticCode(child.effort, MODEL_REASONING_EFFORT_CODES) } : {}),
    ...(closedDiagnosticCode(child.childToolPolicy, CHILD_TOOL_POLICY_CODES) ? { childToolPolicy: closedDiagnosticCode(child.childToolPolicy, CHILD_TOOL_POLICY_CODES) } : {}),
    ...(rendererIdentifier(child.jobId) ? { jobId: rendererIdentifier(child.jobId) } : {}),
    ...(closedDiagnosticCode(child.returnFormat, CHILD_RETURN_FORMAT_CODES) ? { returnFormat: closedDiagnosticCode(child.returnFormat, CHILD_RETURN_FORMAT_CODES) } : {}),
    ...(rendererNumber(child.tokenBudget) !== undefined ? { tokenBudget: rendererNumber(child.tokenBudget) } : {}),
    ...(rendererNumber(child.timeBudgetMs) !== undefined ? { timeBudgetMs: rendererNumber(child.timeBudgetMs) } : {}),
    ...(rendererBoolean(child.budgetExceeded) !== undefined ? { budgetExceeded: rendererBoolean(child.budgetExceeded) } : {}),
    ...(closedDiagnosticCode(child.evidenceBundleStatus, CHILD_EVIDENCE_STATUS_CODES) ? { evidenceBundleStatus: closedDiagnosticCode(child.evidenceBundleStatus, CHILD_EVIDENCE_STATUS_CODES) } : {}),
    ...(rendererNumber(child.evidenceCount) !== undefined ? { evidenceCount: rendererNumber(child.evidenceCount) } : {}),
    ...(rendererBoolean(child.prefixReused) !== undefined ? { prefixReused: rendererBoolean(child.prefixReused) } : {}),
    ...(rendererNumber(child.inheritedHistoryItems) !== undefined ? { inheritedHistoryItems: rendererNumber(child.inheritedHistoryItems) } : {}),
    ...(rendererNumber(child.toolInvocations) !== undefined ? { toolInvocations: rendererNumber(child.toolInvocations) } : {}),
    ...(rendererBoolean(child.evidenceLedgered) !== undefined ? { evidenceLedgered: rendererBoolean(child.evidenceLedgered) } : {}),
    ...(rendererNumber(child.durationMs) !== undefined ? { durationMs: rendererNumber(child.durationMs) } : {}),
    ...(rendererNumber(child.queuedMs) !== undefined ? { queuedMs: rendererNumber(child.queuedMs) } : {}),
    ...(rendererNumber(child.totalTokens) !== undefined ? { totalTokens: rendererNumber(child.totalTokens) } : {}),
    ...(rendererNumber(child.cachedTokens) !== undefined ? { cachedTokens: rendererNumber(child.cachedTokens) } : {}),
    ...(rendererNumber(child.cacheHitTokens) !== undefined ? { cacheHitTokens: rendererNumber(child.cacheHitTokens) } : {}),
    ...(rendererNumber(child.cacheMissTokens) !== undefined ? { cacheMissTokens: rendererNumber(child.cacheMissTokens) } : {}),
    ...(rendererNumber(child.cacheHitRate) !== undefined ? { cacheHitRate: rendererNumber(child.cacheHitRate) } : {}),
    ...(rendererNumber(child.cacheableTokenHitRate) !== undefined ? { cacheableTokenHitRate: rendererNumber(child.cacheableTokenHitRate) } : {}),
    ...(rendererNumber(child.totalInputTokenHitRate) !== undefined ? { totalInputTokenHitRate: rendererNumber(child.totalInputTokenHitRate) } : {}),
    ...(rendererNumber(child.costUsd) !== undefined ? { costUsd: rendererNumber(child.costUsd) } : {}),
    ...(rendererNumber(child.costCny) !== undefined ? { costCny: rendererNumber(child.costCny) } : {}),
    ...(rendererBoolean(child.priceConfigured) !== undefined ? { priceConfigured: rendererBoolean(child.priceConfigured) } : {}),
    ...(rendererNumber(child.cacheSavingsUsd) !== undefined ? { cacheSavingsUsd: rendererNumber(child.cacheSavingsUsd) } : {}),
    ...(rendererNumber(child.cacheSavingsCny) !== undefined ? { cacheSavingsCny: rendererNumber(child.cacheSavingsCny) } : {}),
    ...(rendererNumber(child.tokenEconomySavingsTokens) !== undefined ? { tokenEconomySavingsTokens: rendererNumber(child.tokenEconomySavingsTokens) } : {}),
    ...(rendererNumber(child.tokenEconomySavingsUsd) !== undefined ? { tokenEconomySavingsUsd: rendererNumber(child.tokenEconomySavingsUsd) } : {}),
    ...(rendererNumber(child.tokenEconomySavingsCny) !== undefined ? { tokenEconomySavingsCny: rendererNumber(child.tokenEconomySavingsCny) } : {}),
    ...(rendererBoolean(child.background) !== undefined ? { background: rendererBoolean(child.background) } : {}),
    ...(closedDiagnosticCode(child.heartbeatStatus, RUNTIME_HEARTBEAT_STATUS_CODES)
      ? { heartbeatStatus: closedDiagnosticCode(child.heartbeatStatus, RUNTIME_HEARTBEAT_STATUS_CODES) }
      : {}),
    ...(rendererNumber(child.heartbeatAgeMs) !== undefined ? { heartbeatAgeMs: rendererNumber(child.heartbeatAgeMs) } : {}),
    ...(rendererBoolean(child.leaseExpired) !== undefined ? { leaseExpired: rendererBoolean(child.leaseExpired) } : {}),
    ...(rendererNumber(child.staleAfterMs) !== undefined ? { staleAfterMs: rendererNumber(child.staleAfterMs) } : {}),
    ...(rendererBoolean(child.stalled) !== undefined ? { stalled: rendererBoolean(child.stalled) } : {}),
    ...(rendererBoolean(child.orphaned) !== undefined ? { orphaned: rendererBoolean(child.orphaned) } : {}),
    ...(closedDiagnosticCode(child.recoveryStatus, RUNTIME_RECOVERY_STATUS_CODES)
      ? { recoveryStatus: closedDiagnosticCode(child.recoveryStatus, RUNTIME_RECOVERY_STATUS_CODES) }
      : {}),
    ...(rendererNumber(child.recoveryAttempt) !== undefined ? { recoveryAttempt: rendererNumber(child.recoveryAttempt) } : {}),
    ...(rendererIdentifier(child.deliveryId) ? { deliveryId: rendererIdentifier(child.deliveryId) } : {}),
    ...(closedDiagnosticCode(child.deliveryStatus, RUNTIME_DELIVERY_STATUS_CODES)
      ? { deliveryStatus: closedDiagnosticCode(child.deliveryStatus, RUNTIME_DELIVERY_STATUS_CODES) }
      : {}),
    ...(rendererIdentifier(child.deliveryItemId) ? { deliveryItemId: rendererIdentifier(child.deliveryItemId) } : {}),
    ...(rendererNumber(child.completionDeliveryAttempt) !== undefined ? { completionDeliveryAttempt: rendererNumber(child.completionDeliveryAttempt) } : {}),
    ...(rendererIdentifier(child.parallelGroupId) ? { parallelGroupId: rendererIdentifier(child.parallelGroupId) } : {}),
    ...(rendererNumber(child.parallelIndex) !== undefined ? { parallelIndex: rendererNumber(child.parallelIndex) } : {}),
    ...(rendererIdentifier(child.steerMessageId) ? { steerMessageId: rendererIdentifier(child.steerMessageId) } : {}),
    ...(closedDiagnosticCode(child.steerStatus, CHILD_STEER_STATUS_CODES) ? { steerStatus: closedDiagnosticCode(child.steerStatus, CHILD_STEER_STATUS_CODES) } : {}),
    ...(rendererNumber(child.pendingSteers) !== undefined ? { pendingSteers: rendererNumber(child.pendingSteers) } : {}),
    ...(rendererNumber(child.admittedSteers) !== undefined ? { admittedSteers: rendererNumber(child.admittedSteers) } : {}),
    ...(rendererNumber(child.steerCount) !== undefined ? { steerCount: rendererNumber(child.steerCount) } : {}),
    ...(rendererBoolean(child.canAcceptSteer) !== undefined ? { canAcceptSteer: rendererBoolean(child.canAcceptSteer) } : {}),
    ...(rendererIdentifier(child.pauseRequestId) ? { pauseRequestId: rendererIdentifier(child.pauseRequestId) } : {}),
    ...(closedDiagnosticCode(child.pauseStatus, CHILD_PAUSE_STATUS_CODES) ? { pauseStatus: closedDiagnosticCode(child.pauseStatus, CHILD_PAUSE_STATUS_CODES) } : {}),
    ...(rendererBoolean(child.canPause) !== undefined ? { canPause: rendererBoolean(child.canPause) } : {}),
    ...(rendererBoolean(child.canResume) !== undefined ? { canResume: rendererBoolean(child.canResume) } : {}),
    ...(rendererBoolean(child.paused) !== undefined ? { paused: rendererBoolean(child.paused) } : {}),
    ...(rendererNumber(child.pauseCount) !== undefined ? { pauseCount: rendererNumber(child.pauseCount) } : {}),
    ...(child.isolationMode === 'worktree' ? { isolationMode: 'worktree' as const } : {}),
    ...(closedDiagnosticCode(child.mergeStatus, CHILD_MERGE_STATUS_CODES) ? { mergeStatus: closedDiagnosticCode(child.mergeStatus, CHILD_MERGE_STATUS_CODES) } : {}),
    ...(rendererIdentifier(child.mergeDecisionId) ? { mergeDecisionId: rendererIdentifier(child.mergeDecisionId) } : {}),
    ...(closedDiagnosticCode(child.mergeDecision, CHILD_DECISION_CODES) ? { mergeDecision: closedDiagnosticCode(child.mergeDecision, CHILD_DECISION_CODES) } : {}),
    ...(rendererIdentifier(child.cleanupReceiptId) ? { cleanupReceiptId: rendererIdentifier(child.cleanupReceiptId) } : {}),
    ...(rendererIdentifier(child.cleanupAcceptDecisionId) ? { cleanupAcceptDecisionId: rendererIdentifier(child.cleanupAcceptDecisionId) } : {}),
    ...(rendererBoolean(child.cleanupRemoved) !== undefined ? { cleanupRemoved: rendererBoolean(child.cleanupRemoved) } : {}),
    ...(rendererIdentifier(child.acceptDecisionId) ? { acceptDecisionId: rendererIdentifier(child.acceptDecisionId) } : {}),
    ...(rendererIdentifier(child.acceptApprovalId) ? { acceptApprovalId: rendererIdentifier(child.acceptApprovalId) } : {}),
    ...(rendererSha256(child.appliedPatchDigest) ? { appliedPatchDigest: rendererSha256(child.appliedPatchDigest) } : {}),
    ...(rendererIdentifier(child.conflictReportId) ? { conflictReportId: rendererIdentifier(child.conflictReportId) } : {}),
    ...(rendererSha256(child.parentHeadAtConflict) ? { parentHeadAtConflict: rendererSha256(child.parentHeadAtConflict) } : {}),
    ...(rendererSha256(child.childHeadAtConflict) ? { childHeadAtConflict: rendererSha256(child.childHeadAtConflict) } : {}),
    ...(rendererSha256(child.sourcePatchDigest) ? { sourcePatchDigest: rendererSha256(child.sourcePatchDigest) } : {}),
    ...(rendererNumber(child.conflictFileCount) !== undefined ? { conflictFileCount: rendererNumber(child.conflictFileCount) } : {}),
    ...(rendererIdentifier(child.repairReviewId) ? { repairReviewId: rendererIdentifier(child.repairReviewId) } : {}),
    ...(rendererIdentifier(child.repairConflictReportId) ? { repairConflictReportId: rendererIdentifier(child.repairConflictReportId) } : {}),
    ...(rendererSha256(child.repairPatchDigest) ? { repairPatchDigest: rendererSha256(child.repairPatchDigest) } : {}),
    ...(rendererSha256(child.repairExpectedParentHead) ? { repairExpectedParentHead: rendererSha256(child.repairExpectedParentHead) } : {}),
    ...(closedDiagnosticCode(child.repairDryRunStatus, CHILD_REPAIR_DRY_RUN_STATUS_CODES) ? { repairDryRunStatus: closedDiagnosticCode(child.repairDryRunStatus, CHILD_REPAIR_DRY_RUN_STATUS_CODES) } : {}),
    ...(rendererNumber(child.repairChangedFileCount) !== undefined ? { repairChangedFileCount: rendererNumber(child.repairChangedFileCount) } : {}),
    ...(rendererIdentifier(child.repairDecisionId) ? { repairDecisionId: rendererIdentifier(child.repairDecisionId) } : {}),
    ...(rendererIdentifier(child.repairApprovalId) ? { repairApprovalId: rendererIdentifier(child.repairApprovalId) } : {}),
    ...(rendererSha256(child.repairAppliedPatchDigest) ? { repairAppliedPatchDigest: rendererSha256(child.repairAppliedPatchDigest) } : {}),
    ...(rendererIdentifier(child.childTodoListId) ? { childTodoListId: rendererIdentifier(child.childTodoListId) } : {}),
    ...(closedDiagnosticCode(child.childTodoScope, CHILD_TODO_SCOPE_CODES) ? { childTodoScope: closedDiagnosticCode(child.childTodoScope, CHILD_TODO_SCOPE_CODES) } : {}),
    ...(rendererNumber(child.childTodoCount) !== undefined ? { childTodoCount: rendererNumber(child.childTodoCount) } : {}),
    ...(rendererNumber(child.childTodoCompletedCount) !== undefined ? { childTodoCompletedCount: rendererNumber(child.childTodoCompletedCount) } : {}),
    ...(rendererNumber(child.childTodoInProgressCount) !== undefined ? { childTodoInProgressCount: rendererNumber(child.childTodoInProgressCount) } : {}),
    ...(rendererNumber(child.childTodoPendingCount) !== undefined ? { childTodoPendingCount: rendererNumber(child.childTodoPendingCount) } : {}),
    ...(rendererNumber(child.childTodoFailedCount) !== undefined ? { childTodoFailedCount: rendererNumber(child.childTodoFailedCount) } : {}),
    ...(rendererNumber(child.childTodoBlockedCount) !== undefined ? { childTodoBlockedCount: rendererNumber(child.childTodoBlockedCount) } : {}),
    ...(rendererNumber(child.childTodoCanceledCount) !== undefined ? { childTodoCanceledCount: rendererNumber(child.childTodoCanceledCount) } : {}),
    ...(rendererIdentifier(child.childTodoProjectionId) ? { childTodoProjectionId: rendererIdentifier(child.childTodoProjectionId) } : {}),
    ...(closedDiagnosticCode(child.childTodoProjectionStatus, CHILD_TODO_PROJECTION_STATUS_CODES) ? { childTodoProjectionStatus: closedDiagnosticCode(child.childTodoProjectionStatus, CHILD_TODO_PROJECTION_STATUS_CODES) } : {}),
    ...(rendererNumber(child.childTodoProjectionItemCount) !== undefined ? { childTodoProjectionItemCount: rendererNumber(child.childTodoProjectionItemCount) } : {}),
    ...(rendererNumber(child.childTodoProjectionEvidenceCount) !== undefined ? { childTodoProjectionEvidenceCount: rendererNumber(child.childTodoProjectionEvidenceCount) } : {}),
    ...(rendererNumber(child.childTodoProjectionMappedCount) !== undefined ? { childTodoProjectionMappedCount: rendererNumber(child.childTodoProjectionMappedCount) } : {}),
    ...(rendererNumber(child.childTodoProjectionCompletedMappedCount) !== undefined ? { childTodoProjectionCompletedMappedCount: rendererNumber(child.childTodoProjectionCompletedMappedCount) } : {}),
    ...(rendererIdentifier(child.childTodoProjectionDecisionId) ? { childTodoProjectionDecisionId: rendererIdentifier(child.childTodoProjectionDecisionId) } : {}),
    ...(closedDiagnosticCode(child.childTodoProjectionDecision, CHILD_DECISION_CODES) ? { childTodoProjectionDecision: closedDiagnosticCode(child.childTodoProjectionDecision, CHILD_DECISION_CODES) } : {}),
    ...(rendererIdentifier(child.childTodoProjectionApprovalId) ? { childTodoProjectionApprovalId: rendererIdentifier(child.childTodoProjectionApprovalId) } : {}),
    ...(rendererNumber(child.childTodoProjectionAcceptedItemCount) !== undefined ? { childTodoProjectionAcceptedItemCount: rendererNumber(child.childTodoProjectionAcceptedItemCount) } : {}),
    ...(rendererNumber(child.childTodoProjectionSkippedItemCount) !== undefined ? { childTodoProjectionSkippedItemCount: rendererNumber(child.childTodoProjectionSkippedItemCount) } : {}),
    ...(rendererNumber(child.changedFileCount) !== undefined ? { changedFileCount: rendererNumber(child.changedFileCount) } : {})
  }
}

function childMetadataFromPayload(item: CoreTurnItemJson): RuntimeChildMetadata | undefined {
  const payload = payloadFor(item)
  const child = payload.child
  if (!child || typeof child !== 'object' || Array.isArray(child)) return undefined
  return normalizeChildMetadataForRenderer(child as CoreChildRuntimeMetadataJson)
}

function normalizeJobDiagnostics(value: unknown): RuntimeJobDiagnosticsMetadata | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const raw = value as Record<string, unknown>
  const normalized: RuntimeJobDiagnosticsMetadata = {}
  for (const key of ['pauseRequestId', 'jobId', 'childRunId', 'childThreadId', 'childTurnId', 'parentThreadId', 'parentTurnId', 'autoContinueTurnId', 'deliveryId', 'deliveryItemId'] as const) {
    const entry = raw[key]
    if (typeof entry === 'string' && entry.trim()) normalized[key] = entry.trim()
  }
  const notificationKind = closedDiagnosticCode(raw.notificationKind, RUNTIME_JOB_NOTIFICATION_KINDS)
  if (notificationKind) normalized.notificationKind = notificationKind
  const kind = closedDiagnosticCode(raw.kind, RUNTIME_JOB_KIND_CODES) ??
    (typeof raw.kind === 'string' && raw.kind.trim() ? 'unknown' : undefined)
  if (kind) normalized.kind = kind
  const status = closedDiagnosticCode(raw.status, RUNTIME_JOB_STATUS_CODES) ??
    (typeof raw.status === 'string' && raw.status.trim() ? 'unknown' : undefined)
  if (status) normalized.status = status
  const heartbeatStatus = closedDiagnosticCode(raw.heartbeatStatus, RUNTIME_HEARTBEAT_STATUS_CODES)
  if (heartbeatStatus) normalized.heartbeatStatus = heartbeatStatus
  const autoContinueStatus = closedDiagnosticCode(raw.autoContinueStatus, RUNTIME_AUTO_CONTINUE_STATUS_CODES)
  if (autoContinueStatus) normalized.autoContinueStatus = autoContinueStatus
  const deliveryStatus = closedDiagnosticCode(raw.deliveryStatus, RUNTIME_DELIVERY_STATUS_CODES)
  if (deliveryStatus) normalized.deliveryStatus = deliveryStatus
  const recoveryStatus = closedDiagnosticCode(raw.recoveryStatus, RUNTIME_RECOVERY_STATUS_CODES)
  if (recoveryStatus) normalized.recoveryStatus = recoveryStatus
  const pauseStatus = closedDiagnosticCode(raw.pauseStatus, RUNTIME_PAUSE_STATUS_CODES)
  if (pauseStatus) normalized.pauseStatus = pauseStatus
  const warningCode = closedDiagnosticCode(raw.warningCode, RUNTIME_WARNING_CODES)
  if (warningCode) normalized.warningCode = warningCode
  for (const key of ['ageMs', 'idleMs', 'heartbeatAgeMs', 'staleAfterMs', 'stalledAfterMs', 'recoveryAttempt', 'completionDeliveryAttempt'] as const) {
    const entry = raw[key]
    if (typeof entry === 'number' && Number.isFinite(entry)) normalized[key] = entry
  }
  for (const key of ['terminal', 'background', 'stalled', 'paused', 'canContinueParent', 'lateCompletionSuppressed', 'autoContinueParent', 'leaseExpired', 'orphaned'] as const) {
    const entry = raw[key]
    if (typeof entry === 'boolean') normalized[key] = entry
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined
}

function normalizeProviderErrorDiagnostics(
  value: unknown
): RuntimeProviderErrorDiagnosticsMetadata | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const raw = value as Record<string, unknown>
  const status = rendererNumber(raw.status)
  const attempt = rendererNumber(raw.attempt)
  const kind = exactClosedDiagnosticCode(raw.kind, PROVIDER_ERROR_KIND_CODES)
  const endpointFormat = exactClosedDiagnosticCode(raw.endpointFormat, PROVIDER_ENDPOINT_FORMAT_CODES)
  const authStatus = exactClosedDiagnosticCode(raw.authStatus, PROVIDER_AUTH_STATUS_CODES)
  const failureStage = exactClosedDiagnosticCode(raw.failureStage, PROVIDER_FAILURE_STAGE_CODES)
  const dispatchState = exactClosedDiagnosticCode(raw.dispatchState, PROVIDER_DISPATCH_STATE_CODES)
  const normalized: RuntimeProviderErrorDiagnosticsMetadata = {
    ...(status !== undefined && Number.isSafeInteger(status) && status <= 599 ? { status } : {}),
    ...(attempt !== undefined && Number.isSafeInteger(attempt) && attempt <= 1_000_000 ? { attempt } : {}),
    ...(kind ? { kind } : {}),
    ...(endpointFormat ? { endpointFormat } : {}),
    ...(authStatus ? { authStatus } : {}),
    ...(failureStage ? { failureStage } : {}),
    ...(dispatchState ? { dispatchState } : {}),
    ...(typeof raw.hasApiKey === 'boolean' ? { hasApiKey: raw.hasApiKey } : {}),
    ...(typeof raw.retryable === 'boolean' ? { retryable: raw.retryable } : {})
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined
}

const RUNTIME_JOB_NOTIFICATION_KINDS = [
  'background_job_completion',
  'background_job_auto_continue',
  'background_job_delivery',
  'thread_summary_subagent',
  'thread_summary_task'
] as const

const RUNTIME_JOB_KIND_CODES = [
  'task',
  'parallel_task',
  'planner',
  'background-shell',
  'bash',
  'subagent',
  'child-run',
  'parallel-child-run',
  'command',
  'process',
  'unknown'
] as const

const RUNTIME_JOB_STATUS_CODES = [
  'queued',
  'running',
  'pause_requested',
  'paused',
  'resume_requested',
  'resuming',
  'completed',
  'failed',
  'aborted',
  'interrupted',
  'killed',
  'canceled',
  'timeout',
  'starting',
  'pending',
  'delivered',
  'retry',
  'recovering',
  'recovered',
  'dead_letter',
  'dead_lettered',
  'skipped',
  'stopped',
  'missing',
  'archived',
  'unknown'
] as const

const RUNTIME_HEARTBEAT_STATUS_CODES = [
  'running',
  'healthy',
  'stale',
  'stalled',
  'lease_expired',
  'paused',
  'completed',
  'recovered',
  'unknown'
] as const

const RUNTIME_AUTO_CONTINUE_STATUS_CODES = ['pending', 'started', 'skipped', 'failed', 'completed'] as const
const RUNTIME_DELIVERY_STATUS_CODES = ['pending', 'retry', 'delivered', 'dead_letter', 'failed'] as const
const RUNTIME_RECOVERY_STATUS_CODES = ['pending', 'recovering', 'recovered', 'failed', 'dead_lettered'] as const
const RUNTIME_PAUSE_STATUS_CODES = ['pause_requested', 'paused', 'resume_requested', 'resuming', 'running', 'failed'] as const
const RUNTIME_WARNING_CODES = ['task_job_stalled', 'task_job_stale', 'slow-output'] as const
const PROVIDER_ERROR_KIND_CODES = [
  'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network', 'http',
  'invalid_model', 'unknown'
] as const
const PROVIDER_ENDPOINT_FORMAT_CODES = [
  'chat_completions', 'responses', 'messages', 'custom_endpoint'
] as const
const PROVIDER_AUTH_STATUS_CODES = ['none', 'possible', 'required', 'unknown'] as const
const PROVIDER_FAILURE_STAGE_CODES = [
  'request_validation', 'request_build', 'pre_send_body_audit', 'telemetry_begin',
  'transport_before_observed_send', 'transport_after_observed_send', 'local_admission_config',
  'callback_projection', 'unclassified'
] as const
const PROVIDER_DISPATCH_STATE_CODES = ['not_sent', 'sent', 'indeterminate'] as const

function closedDiagnosticCode<const T extends readonly string[]>(value: unknown, allowed: T): T[number] | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return allowed.includes(normalized as T[number]) ? normalized as T[number] : undefined
}

function exactClosedDiagnosticCode<const T extends readonly string[]>(value: unknown, allowed: T): T[number] | undefined {
  return typeof value === 'string' && allowed.includes(value as T[number]) ? value as T[number] : undefined
}

function normalizeUserFileReferences(value: unknown): RuntimeDisclosureMetadata['fileReferences'] {
  if (!Array.isArray(value)) return undefined
  const references = value
    .map((entry): NonNullable<RuntimeDisclosureMetadata['fileReferences']>[number] | null => {
      if (!entry || typeof entry !== 'object') return null
      const raw = entry as Record<string, unknown>
      const path = typeof raw.path === 'string' ? raw.path.trim() : ''
      const relativePath = typeof raw.relativePath === 'string' ? raw.relativePath.trim() : ''
      const name = typeof raw.name === 'string' ? raw.name.trim() : ''
      if (!path || !relativePath || !name) return null
      return {
        path,
        relativePath,
        name,
        ...(raw.kind === 'directory' ? { kind: 'directory' as const } : { kind: 'file' as const })
      }
    })
    .filter((entry): entry is NonNullable<RuntimeDisclosureMetadata['fileReferences']>[number] => entry !== null)
  return references.length > 0 ? references : undefined
}

function applyRuntimeDisclosureMeta(
  meta: Record<string, unknown>,
  item: CoreTurnItemJson,
  child?: CoreChildRuntimeMetadataJson | RuntimeChildMetadata
): void {
  const attachmentIds = stringArray(item.attachmentIds)
  const fileReferences = normalizeUserFileReferences(item.fileReferences)
  const activeSkillIds = stringArray(item.activeSkillIds)
  const injectedMemoryIds = stringArray(item.injectedMemoryIds)
  const normalizedChild = normalizeChildMetadataForRenderer(child as CoreChildRuntimeMetadataJson | undefined)
  const displayText = typeof item.displayText === 'string' ? item.displayText.trim() : ''
  const clientUserMessageId = typeof item.clientUserMessageId === 'string' ? item.clientUserMessageId.trim() : ''
  if (displayText && displayText !== item.text?.trim()) {
    meta.displayText = projectOrdinaryPublicText(displayText)
  }
  if (clientUserMessageId) {
    meta.clientUserMessageId = clientUserMessageId
    meta.steeringStatus = 'accepted'
  }
  if (item.delivery === 'steer') {
    meta.delivery = 'steer'
  }
  if (attachmentIds) meta.attachmentIds = attachmentIds
  if (fileReferences) meta.fileReferences = fileReferences
  if (typeof item.workspaceCheckpointId === 'string' && item.workspaceCheckpointId.trim()) {
    meta.workspaceCheckpointId = item.workspaceCheckpointId.trim()
  }
  if (activeSkillIds) meta.activeSkillIds = activeSkillIds
  if (injectedMemoryIds) meta.injectedMemoryIds = injectedMemoryIds
  if (typeof item.skillInjectionBytes === 'number') {
    meta.skillInjectionBytes = item.skillInjectionBytes
  }
  if (normalizedChild) meta.child = normalizedChild
}

function applyCommandResultMeta(meta: Record<string, unknown>, item: CoreTurnItemJson): void {
  const payload = payloadFor(item)
  for (const key of COMMAND_RESULT_META_KEYS) {
    const value = payload[key]
    if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
      meta[key] = value
    }
  }
}

function inferToolPresentation(item: CoreTurnItemJson): {
  toolKind: ToolBlock['toolKind']
  filePath?: string
  command?: string
} {
  const payload = payloadFor(item)
  const filePath = readStructuredString(payload, ...FILE_PATH_KEYS)
  const command = readStructuredString(payload, ...COMMAND_KEYS)

  if (
    item.toolKind === 'tool_call' ||
    item.toolKind === 'command_execution' ||
    item.toolKind === 'file_change' ||
    item.toolKind === 'subagent'
  ) {
    return {
      toolKind: item.toolKind,
      ...(filePath ? { filePath } : {}),
      ...(command ? { command } : {})
    }
  }

  const toolName = item.toolName?.trim() ?? ''
  const byName = TOOL_KIND_BY_NAME.get(toolName)
  if (byName) {
    return {
      toolKind: byName,
      ...(filePath ? { filePath } : {}),
      ...(command ? { command } : {})
    }
  }

  // Payload-only fallback. Prefer the kind whose field is present
  // on the payload; if both are present, the explicit command wins
  // (matches the previous heuristic and what the tests assert).
  if (command) {
    return { toolKind: 'command_execution', command }
  }
  if (filePath) {
    return { toolKind: 'file_change', filePath }
  }
  return { toolKind: 'tool_call' }
}

function isPlanItem(item: CoreTurnItemJson): boolean {
  if (item.toolName === 'create_plan') return true
  if (item.kind === 'tool_result' && isPlanOutput(item.output)) return true
  return false
}

function isPlanOutput(output: unknown): boolean {
  const projection = PublicToolResultProjectionV1.safeParse(output)
  return projection.success && projection.data.projectionKind === 'plan_status'
}

function extractPlanMetadata(item: CoreTurnItemJson): Record<string, unknown> | null {
  if (item.kind === 'tool_result') {
    const projection = PublicToolResultProjectionV1.safeParse(item.output)
    if (!projection.success || projection.data.projectionKind !== 'plan_status') return null
    return {
      plan_id: projection.data.plan.planId,
      relative_path: projection.data.plan.relativePath,
      operation: projection.data.plan.operation,
      saved_at: projection.data.plan.savedAt,
      content_hash: projection.data.plan.contentHash,
      byte_size: projection.data.plan.byteSize,
      ...(item.isError ? { error: projection.data.messageKey } : {})
    }
  }
  return null
}

function toolBlockFromItem(item: CoreTurnItemJson, child?: CoreChildRuntimeMetadataJson): ToolBlock {
  const publicPayload = payloadFor(item)
  const normalizedChild = normalizeChildMetadataForRenderer(child) ?? childMetadataFromPayload(item)
  const detail = item.kind === 'tool_result'
    ? publicToolResultDetail(item.output)
    : item.kind === 'tool_call'
      ? PublicToolCallArgumentsProjectionV1.safeParse(item.arguments).success
        ? 'tool arguments withheld'
        : 'tool arguments unavailable'
      : undefined
  const isPlan = isPlanItem(item)
  const summary = item.kind === 'tool_progress' || normalizedChild
    ? fixedProgressSummary(normalizedChild)
    : item.summary?.trim() ||
      (isPlan ? 'Create plan' : null) ||
      item.toolName?.trim() ||
      (item.kind === 'tool_result' ? 'tool result' : 'tool')
  const meta: Record<string, unknown> = {
    sourceItemId: item.id,
    ...(item.turnId ? { turnId: item.turnId } : {}),
    ...(item.callId ? { callId: item.callId } : {}),
    ...(item.kind !== 'tool_progress' && !normalizedChild && item.toolName ? { toolName: item.toolName } : {})
  }
  const payload = publicPayload
  applyRuntimeDisclosureMeta(meta, item, normalizedChild)
  const diagnostics = item.kind === 'tool_result' || item.kind === 'tool_progress'
    ? normalizeJobDiagnostics(payload.diagnostics)
    : undefined
  if (diagnostics) meta.diagnostics = diagnostics
  if (item.kind === 'tool_progress') meta.runtimeStatus = 'tool_progress'
  const presentation = inferToolPresentation(item)
  if (presentation.command) meta.command = presentation.command
  if (presentation.toolKind === 'command_execution') applyCommandResultMeta(meta, item)
  if (isPlan) {
    const plan = extractPlanMetadata(item)
    if (plan) meta.plan = plan
  }
  if (item.kind === 'tool_result' && item.toolName === 'generate_office_document' && !item.isError) {
    const projection = PublicToolResultProjectionV1.safeParse(item.output)
    if (projection.success && projection.data.projectionKind === 'artifact_status') meta.generatedArtifact = projection.data.artifact
  }
  return {
    kind: 'tool',
    id: toolBlockId(item, normalizedChild),
    createdAt: itemCreatedAt(item),
    summary,
    status: toolStatus(item),
    toolKind: presentation.toolKind,
    ...(presentation.filePath ? { filePath: presentation.filePath } : {}),
    ...(detail ? { detail } : {}),
    meta
  }
}

export function mergeChatBlocks(blocks: ChatBlock[]): ChatBlock[] {
  const merged: ChatBlock[] = []
  const toolIndexes = new Map<string, number>()
  for (const block of blocks) {
    if (block.kind !== 'tool') {
      merged.push(block)
      continue
    }
    const existingIndex = toolIndexes.get(block.id)
    if (existingIndex === undefined) {
      toolIndexes.set(block.id, merged.length)
      merged.push(block)
      continue
    }
    const existing = merged[existingIndex]
    if (!existing || existing.kind !== 'tool') {
      merged.push(block)
      continue
    }
    const preserveFinishedResultDetail =
      readStructuredString(block.meta ?? {}, 'runtimeStatus') === 'tool_progress' &&
      block.status !== 'running' &&
      existing.status !== 'running'
    merged[existingIndex] = {
      ...existing,
      ...block,
      createdAt: existing.createdAt ?? block.createdAt,
      summary:
        preserveFinishedResultDetail && existing.summary
          ? existing.summary
          : block.summary || existing.summary,
      detail:
        preserveFinishedResultDetail && existing.detail
          ? existing.detail
          : block.detail ?? existing.detail,
      filePath: block.filePath ?? existing.filePath,
      toolKind: block.toolKind ?? existing.toolKind,
      meta: { ...(existing.meta ?? {}), ...(block.meta ?? {}) }
    }
  }
  return merged
}

function userInputQuestionsFromItem(item: CoreTurnItemJson): UserInputQuestion[] {
  return questionsFromCore(item.questions, item.prompt, item.inputId ?? item.id)
}

function questionsFromCore(
  questions: CoreTurnItemJson['questions'] | CoreRuntimeEventJson['questions'] | undefined,
  prompt: string | undefined,
  fallbackId: string
): UserInputQuestion[] {
  if (Array.isArray(questions) && questions.length > 0) {
    return questions
      .map((question) => normalizeUserInputQuestion(question))
      .filter((question): question is UserInputQuestion => question !== null)
  }
  return [
    {
      header: 'Input',
      id: fallbackId,
      question: prompt?.trim() ? projectOrdinaryPublicText(prompt.trim()) : 'Input requested',
      options: []
    }
  ]
}

function normalizeUserInputQuestion(question: unknown): UserInputQuestion | null {
  if (!question || typeof question !== 'object') return null
  const raw = question as Record<string, unknown>
  const options = Array.isArray(raw.options)
    ? raw.options
        .map((option) => normalizeUserInputOption(option))
        .filter((option): option is UserInputQuestion['options'][number] => option !== null)
    : []
  return {
    header: typeof raw.header === 'string' && raw.header.trim()
      ? projectOrdinaryPublicText(raw.header.trim())
      : 'Input',
    id: typeof raw.id === 'string' && raw.id.trim() ? raw.id.trim() : 'input',
    question: typeof raw.question === 'string' && raw.question.trim()
      ? projectOrdinaryPublicText(raw.question.trim())
      : 'Input requested',
    options
  }
}

function normalizeUserInputOption(option: unknown): UserInputQuestion['options'][number] | null {
  if (!option || typeof option !== 'object') return null
  const raw = option as Record<string, unknown>
  const label = typeof raw.label === 'string' && raw.label.trim() ? raw.label.trim() : null
  if (!label) return null
  return {
    label: projectOrdinaryPublicText(label),
    description: typeof raw.description === 'string'
      ? projectOrdinaryPublicText(raw.description)
      : ''
  }
}

function normalizeCacheDiagnostics(value: CoreCacheDiagnosticsJson | undefined): RuntimeCacheDiagnosticsMetadata | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const normalized: RuntimeCacheDiagnosticsMetadata = {}
  for (const key of ['prefixHash', 'systemHash', 'modeHash', 'prefixItemsHash', 'toolsHash', 'toolSourcesHash'] as const) {
    const digest = rendererSha256(value[key])
    if (digest) normalized[key] = digest
  }
  for (const key of ['prefixChanged', 'toolSourceChanged', 'cacheTelemetrySupported', 'modelInputComparable', 'providerCostEstimateComplete'] as const) {
    const flag = rendererBoolean(value[key])
    if (flag !== undefined) normalized[key] = flag
  }
  for (const key of ['toolSchemaTokens', 'toolCount', 'firstTokenLatencyMs', 'durationMs', 'cacheHitTokens', 'cacheMissTokens', 'modelInputComparablePrefixBytes', 'providerAttemptCount', 'providerCostKnownAttemptCount', 'providerKnownCostUsdNanos', 'providerKnownCostCnyNanos'] as const) {
    const count = rendererNumber(value[key])
    if (count !== undefined) normalized[key] = count
  }
  if (value.dynamicStateCheck === 'not_checked') normalized.dynamicStateCheck = value.dynamicStateCheck
  if (value.toolSchemaEstimator === 'utf8_bytes_div4') normalized.toolSchemaEstimator = value.toolSchemaEstimator
  if (value.responseModelObservation === 'not_reported' || value.responseModelObservation === 'matches_resolved' || value.responseModelObservation === 'differs_resolved') {
    normalized.responseModelObservation = value.responseModelObservation
  }
  if (value.modelInputFirstDifference === 'unavailable' || value.modelInputFirstDifference === 'none' || value.modelInputFirstDifference === 'system' || value.modelInputFirstDifference === 'tools' || value.modelInputFirstDifference === 'history' || value.modelInputFirstDifference === 'current' || value.modelInputFirstDifference === 'ordering') {
    normalized.modelInputFirstDifference = value.modelInputFirstDifference
  }
  return Object.keys(normalized).length > 0 ? normalized : undefined
}

export function usageFromCore(
  usage: CoreUsageSnapshotJson,
  attribution?: Pick<ThreadUsageSnapshot, 'model' | 'providerId' | 'effort' | 'usageSource' | 'childRunId'> & {
    cacheDiagnostics?: CoreCacheDiagnosticsJson
  }
): ThreadUsageSnapshot {
  const inputTokens = readUsageNumber(usage, 'promptTokens', 'prompt_tokens') ?? 0
  const outputTokens = readUsageNumber(usage, 'completionTokens', 'completion_tokens') ?? 0
  const reasoningTokens = readUsageNumber(usage, 'reasoningTokens', 'reasoning_tokens') ?? 0
  const totalTokens = readUsageNumber(usage, 'totalTokens', 'total_tokens') ?? inputTokens + outputTokens
  const hitTokens = readUsageNumber(usage, 'cacheHitTokens', 'cache_hit_tokens')
  const missTokens = readUsageNumber(usage, 'cacheMissTokens', 'cache_miss_tokens')
  const hasHitTokens = hitTokens !== undefined
  const hasMissTokens = missTokens !== undefined
  const priceConfigured = readUsageBoolean(usage, 'priceConfigured', 'price_configured') === true
  const rawCostUsd = readUsageNumber(usage, 'costUsd', 'cost_usd')
  const rawCostCny = readUsageNumber(usage, 'costCny', 'cost_cny')
  const costUsd = rawCostUsd != null && (rawCostUsd > 0 || priceConfigured) ? rawCostUsd : null
  const costCny = rawCostCny != null && (rawCostCny > 0 || priceConfigured) ? rawCostCny : null
  const cachedTokens = hasHitTokens ? hitTokens ?? 0 : 0
  const cacheMissTokens = hasMissTokens ? missTokens ?? 0 : 0
  const cacheTotal = cachedTokens + cacheMissTokens
  const explicitCacheHitRate = readUsageNumber(usage, 'cacheHitRate', 'cache_hit_rate')
  const cacheHitRate = explicitCacheHitRate !== undefined
    ? explicitCacheHitRate
    : hasHitTokens && hasMissTokens && cacheTotal > 0
      ? cachedTokens / cacheTotal
      : null
  const cacheableTokenHitRate = readUsageNumber(usage, 'cacheableTokenHitRate', 'cacheable_token_hit_rate')
  const totalInputTokenHitRate = readUsageNumber(usage, 'totalInputTokenHitRate', 'total_input_token_hit_rate')
  const cacheSavingsUsd = readUsageNumber(usage, 'cacheSavingsUsd', 'cache_savings_usd')
  const cacheSavingsCny = readUsageNumber(usage, 'cacheSavingsCny', 'cache_savings_cny')
  const tokenEconomySavingsTokens = readUsageNumber(
    usage,
    'tokenEconomySavingsTokens',
    'token_economy_savings_tokens'
  ) ?? 0
  const turns = readUsageNumber(usage, 'turns') ?? 0
  return {
    ...(attribution?.model ? { model: attribution.model } : {}),
    ...(attribution?.providerId ? { providerId: attribution.providerId } : {}),
    ...(attribution?.effort ? { effort: attribution.effort } : {}),
    ...(attribution?.usageSource ? { usageSource: attribution.usageSource } : {}),
    ...(attribution?.childRunId ? { childRunId: attribution.childRunId } : {}),
    ...(normalizeCacheDiagnostics(attribution?.cacheDiagnostics)
      ? { cacheDiagnostics: normalizeCacheDiagnostics(attribution?.cacheDiagnostics) }
      : {}),
    inputTokens,
    outputTokens,
    reasoningTokens,
    cachedTokens,
    cacheMissTokens,
    cacheHitRate,
    cacheableTokenHitRate,
    totalInputTokenHitRate,
    totalTokens,
    costUsd,
    costCny,
    priceConfigured,
    ...(cacheSavingsUsd !== undefined ? { cacheSavingsUsd } : {}),
    ...(cacheSavingsCny !== undefined ? { cacheSavingsCny } : {}),
    tokenEconomySavingsTokens,
    turns
  }
}

function userMessageBlockFromItem(item: CoreTurnItemJson): ChatBlock | null {
  const meta: Record<string, unknown> = {}
  if (item.turnId) meta.turnId = item.turnId
  applyRuntimeDisclosureMeta(meta, item)
  return {
    kind: 'user',
    id: item.id,
    createdAt: itemCreatedAt(item),
    text: projectOrdinaryPublicText(item.text ?? ''),
    ...(Object.keys(meta).length > 0 ? { meta } : {})
  }
}

function userMessageEventFromItem(item: CoreTurnItemJson): UserMessageEventPayload {
  const meta: Record<string, unknown> = {}
  if (item.turnId) meta.turnId = item.turnId
  applyRuntimeDisclosureMeta(meta, item)
  return {
    itemId: item.id,
    turnId: item.turnId,
    createdAt: itemCreatedAt(item),
    text: projectOrdinaryPublicText(item.text ?? ''),
    ...(Object.keys(meta).length > 0 ? { meta } : {})
  }
}

function assistantTextBlockFromItem(item: CoreTurnItemJson): ChatBlock | null {
  // A standalone V3 view is only a projection shape. It is never delivery
  // authority; accepted-final assistants enter renderer state exclusively
  // through a main-verified Batch V2 + Seal V1 group below.
  if (item.acceptedFinal !== undefined || item.acceptedFinalView !== undefined) return null
  const text = sanitizePublicSerializedText(item.text ?? '')
  if (!text) return null
  return {
    kind: 'assistant',
    id: item.id,
    createdAt: itemCreatedAt(item),
    text,
    ...(item.turnId ? { meta: { turnId: item.turnId } } : {})
  }
}

function sealedAcceptedFinalAssistantBlockFromItem(item: CoreTurnItemJson): ChatBlock | null {
  if (item.acceptedFinal !== undefined) return null
  const acceptedFinalView = AcceptedFinalPublicViewV3Schema.safeParse(item.acceptedFinalView)
  if (!acceptedFinalView.success) return null
  const text = sanitizePublicSerializedText(item.text ?? '')
  if (!text) return null
  return {
    kind: 'assistant',
    id: item.id,
    createdAt: itemCreatedAt(item),
    text,
    acceptedFinalView: acceptedFinalView.data,
    ...(item.turnId ? { meta: { turnId: item.turnId } } : {})
  }
}

function approvalBlockFromItem(item: CoreTurnItemJson, child?: CoreChildRuntimeMetadataJson): ChatBlock {
  const meta: Record<string, unknown> = {}
  if (item.turnId) meta.turnId = item.turnId
  applyRuntimeDisclosureMeta(meta, item, child)
  return {
    kind: 'approval',
    id: item.id,
    createdAt: itemCreatedAt(item),
    approvalId: item.approvalId ?? item.id,
    summary: item.summary?.trim() || 'Approval required',
    toolName: item.toolName,
    status:
      item.status === 'allowed' || item.status === 'denied'
        ? item.status
        : item.status === 'failed' || item.status === 'expired'
          ? 'error'
          : 'pending',
    ...(item.status === 'expired'
      ? { errorMessage: 'Approval expired because the turn was interrupted.' }
      : {}),
    ...(Object.keys(meta).length > 0 ? { meta } : {})
  }
}

function userInputBlockFromItem(item: CoreTurnItemJson): ChatBlock {
  const meta: Record<string, unknown> = {}
  if (item.turnId) meta.turnId = item.turnId
  return {
    kind: 'user_input',
    id: item.id,
    createdAt: itemCreatedAt(item),
    requestId: item.inputId ?? item.id,
    questions: userInputQuestionsFromItem(item),
    status:
      item.status === 'failed'
        ? 'error'
        : item.status === 'completed' || item.status === 'submitted'
          ? 'submitted'
          : item.status === 'cancelled'
            ? 'cancelled'
          : 'pending',
    ...(Object.keys(meta).length > 0 ? { meta } : {})
  }
}

function userInputRequestFromCore(input: {
  itemId?: string
  inputId?: string
  turnId?: string
  prompt?: string
  questions?: CoreTurnItemJson['questions'] | CoreRuntimeEventJson['questions']
  seq?: number
}): UserInputRequestPayload {
  const fallbackId = input.inputId ?? input.itemId ?? `input_${input.seq ?? Date.now()}`
  return {
    itemId: input.itemId ?? fallbackId,
    requestId: input.inputId ?? fallbackId,
    questions: questionsFromCore(input.questions, input.prompt, input.inputId ?? fallbackId),
    ...(input.turnId ? { turnId: input.turnId, meta: { turnId: input.turnId } } : {})
  }
}

function compactionBlockFromItem(item: CoreTurnItemJson): ChatBlock {
  return {
    kind: 'compaction',
    id: item.id,
    createdAt: itemCreatedAt(item),
    summary: item.summary?.trim() || 'Context compacted',
    status: item.status === 'failed' ? 'error' : 'success',
    messagesBefore: item.replacedTokens,
    detail: item.pinnedConstraints?.join('\n'),
    auto: item.auto === true,
    ...(item.turnId ? { meta: { turnId: item.turnId } } : {})
  }
}

function reviewStatus(item: CoreTurnItemJson): ReviewEventPayload['status'] {
  if (item.status === 'pending' || item.status === 'running') return 'running'
  if (item.status === 'failed' || item.status === 'aborted') return 'error'
  return 'success'
}

function reviewTargetFromCore(target: CoreReviewTargetJson | undefined): ReviewTarget | undefined {
  if (!target || typeof target.kind !== 'string') return undefined
  switch (target.kind) {
    case 'uncommittedChanges':
      return { kind: 'uncommittedChanges' }
    case 'baseBranch':
      return target.branch?.trim() ? { kind: 'baseBranch', branch: target.branch } : undefined
    case 'commit':
      return target.sha?.trim() ? { kind: 'commit', sha: target.sha } : undefined
    case 'custom':
      return target.instructions?.trim()
        ? { kind: 'custom', instructions: target.instructions }
        : undefined
    default:
      return undefined
  }
}

function reviewOutputFromCore(output: unknown): ReviewOutput | undefined {
  if (!isCoreReviewOutput(output)) return undefined
  return {
    findings: (output.findings ?? []).map((finding) => ({
      title: finding.title,
      body: finding.body,
      confidenceScore: finding.confidenceScore,
      priority: finding.priority,
      codeLocation: {
        absoluteFilePath: finding.codeLocation.absoluteFilePath,
        lineRange: {
          start: finding.codeLocation.lineRange.start,
          end: finding.codeLocation.lineRange.end
        }
      }
    })),
    overallCorrectness: output.overallCorrectness,
    overallExplanation: output.overallExplanation,
    overallConfidenceScore: output.overallConfidenceScore
  }
}

function isCoreReviewOutput(value: unknown): value is CoreReviewOutputJson {
  if (!value || typeof value !== 'object') return false
  const raw = value as Partial<CoreReviewOutputJson>
  return (
    Array.isArray(raw.findings) &&
    (raw.overallCorrectness === 'patch is correct' || raw.overallCorrectness === 'patch is incorrect') &&
    typeof raw.overallExplanation === 'string' &&
    typeof raw.overallConfidenceScore === 'number'
  )
}

function reviewBlockFromItem(item: CoreTurnItemJson): ReviewBlock {
  return {
    kind: 'review',
    id: item.id,
    createdAt: itemCreatedAt(item),
    title: item.title?.trim() || 'Code review',
    status: reviewStatus(item),
    target: reviewTargetFromCore(item.target),
    reviewText: item.reviewText,
    output: reviewOutputFromCore(item.output)
  }
}

function errorSeverity(
  explicit: CoreTurnItemJson['severity'] | CoreRuntimeEventJson['severity'],
  code?: string
): 'info' | 'warning' | 'error' {
  if (explicit === 'info' || explicit === 'warning' || explicit === 'error') return explicit
  if (code === 'budget_warning' || code === 'compaction_summary_fallback') return 'warning'
  if (code === 'tool_catalog_changed' || code === 'tool_storm_suppressed') return 'info'
  return 'error'
}

function runtimeErrorDetail(message: string, code?: string, details?: unknown): string | undefined {
  const parts: string[] = []
  if (code) parts.push(`Code: ${code}`)
  if (message.trim()) parts.push(`Message:\n${redactSecretText(message)}`)
  if (details !== undefined) {
    try {
      parts.push(`Details:\n${JSON.stringify(redactSecrets(details), null, 2)}`)
    } catch {
      parts.push(`Details:\n${redactSecretText(String(details))}`)
    }
  }
  return parts.length > 0 ? parts.join('\n\n') : undefined
}

function systemErrorBlockFromItem(item: CoreTurnItemJson): ChatBlock {
  const message = item.message ?? 'Runtime error'
  const detail = runtimeErrorDetail(message, item.code, item.details)
  return {
    kind: 'system',
    id: item.id,
    createdAt: itemCreatedAt(item),
    text: redactSecretText(message),
    ...(item.code ? { code: item.code } : {}),
    ...(detail ? { detail } : {}),
    severity: errorSeverity(item.severity, item.code),
    ...(item.turnId ? { meta: { turnId: item.turnId } } : {})
  }
}

function runtimeErrorFromItem(item: CoreTurnItemJson): RuntimeErrorEventPayload {
  const message = item.message ?? 'Runtime error'
  return {
    itemId: item.id,
    createdAt: itemCreatedAt(item),
    message: redactSecretText(message),
    ...(item.code ? { code: item.code } : {}),
    ...(item.details !== undefined ? { details: item.details } : {}),
    severity: errorSeverity(item.severity, item.code)
  }
}

function runtimeErrorFromEvent(
  event: CoreRuntimeEventJson,
  fallback: string
): RuntimeErrorEventPayload {
  const message = event.message ?? fallback
  const itemId = eventItemId(event) ?? `runtime_error_${eventTurnId(event) ?? eventThreadId(event) ?? event.seq ?? Date.now()}`
  return {
    itemId,
    createdAt: event.timestamp,
    message: redactSecretText(message),
    ...(event.code ? { code: event.code } : {}),
    ...(event.details !== undefined ? { details: event.details } : {}),
    severity: errorSeverity(event.severity, event.code)
  }
}

function errorForRuntimeEvent(payload: RuntimeErrorEventPayload): Error {
  return new Error(JSON.stringify({
    ...(payload.code ? { code: payload.code } : {}),
    message: payload.message,
    ...(payload.details !== undefined ? { details: payload.details } : {}),
    ...(payload.severity ? { severity: payload.severity } : {})
  }))
}

function runtimeEventBoolean(event: CoreRuntimeEventJson, key: 'terminal' | 'fatal' | 'recoverable'): boolean {
  return (event as Record<string, unknown>)[key] === true
}

function runtimeErrorEventIsTerminal(event: CoreRuntimeEventJson): boolean {
  if (runtimeEventBoolean(event, 'recoverable')) return false
  if (runtimeEventBoolean(event, 'terminal') || runtimeEventBoolean(event, 'fatal')) return true
  if (event.severity === 'info' || event.severity === 'warning') return false
  switch (event.code) {
    case 'turn_failed':
    case 'turn_step_limit_exceeded':
    case 'sse_stream_error':
    case 'sse_start_failed':
      return true
    default:
      return false
  }
}

/**
 * Build a `ChatBlock` from a turn item. Used both for replaying a
 * thread (load path) and as the canonical per-kind view that the
 * live event dispatcher maps onto sink callbacks.
 */
export function chatBlockFromItem(item: CoreTurnItemJson, child?: CoreChildRuntimeMetadataJson): ChatBlock | null {
  switch (item.kind) {
    case 'user_message':
      return userMessageBlockFromItem(item)
		case 'assistant_text':
			return assistantTextBlockFromItem(item)
		case 'assistant_reasoning':
			return null
    case 'tool_call':
    case 'tool_result':
    case 'tool_progress':
      return toolBlockFromItem(item, child)
    case 'approval':
      return approvalBlockFromItem(item, child)
    case 'user_input':
      return userInputBlockFromItem(item)
    case 'compaction':
      return compactionBlockFromItem(item)
    case 'review':
      return reviewBlockFromItem(item)
    case 'error':
      return systemErrorBlockFromItem(item)
    default:
      return null
  }
}

function toolEventFromItem(item: CoreTurnItemJson, child?: CoreChildRuntimeMetadataJson): ToolEventPayload {
  const block = toolBlockFromItem(item, child)
  return {
    itemId: block.id,
    turnId: item.turnId,
    summary: block.summary,
    status: block.status,
    toolKind: block.toolKind,
    detail: block.detail,
    filePath: block.filePath,
    meta: block.meta
  }
}

function compactionFromItem(item: CoreTurnItemJson): CompactionEventPayload {
  return {
    itemId: item.id,
    turnId: item.turnId,
    summary: item.summary?.trim() || 'Context compacted',
    status: item.status === 'failed' ? 'error' : item.status === 'running' ? 'running' : 'success',
    createdAt: itemCreatedAt(item),
    messagesBefore: item.replacedTokens,
    detail: item.pinnedConstraints?.length ? item.pinnedConstraints.join('\n') : undefined,
    auto: item.auto === true,
    ...(item.turnId ? { meta: { turnId: item.turnId } } : {})
  }
}

function reviewFromItem(item: CoreTurnItemJson): ReviewEventPayload {
  const block = reviewBlockFromItem(item)
  return {
    itemId: block.id,
    createdAt: block.createdAt,
    title: block.title,
    status: block.status,
    target: block.target,
    reviewText: block.reviewText,
    output: block.output
  }
}

/**
 * Dispatch a turn item to a live thread sink. The replay path uses
 * `chatBlockFromItem` directly; this function maps item snapshots onto
 * the `ThreadEventSink` callbacks that the chat store understands.
 */
function emitItem(
  item: CoreTurnItemJson,
  sink: ThreadEventSink,
  child?: CoreChildRuntimeMetadataJson
): void {
  switch (item.kind) {
    case 'user_message':
      sink.onUserMessage(userMessageEventFromItem(item))
      return
    case 'assistant_text':
    case 'assistant_reasoning':
      // Live text/reasoning arrives through *_delta events. Item events are
      // snapshots for replay/load paths and would duplicate streamed content.
      return
    case 'tool_call':
    case 'tool_result':
    case 'tool_progress':
      sink.onTool(toolEventFromItem(item, child))
      return
    // Approval and user_input have dedicated runtime events; the
    // generic item path would otherwise double-emit them.
    case 'approval':
    case 'user_input':
      return
    case 'compaction':
      sink.onCompaction(compactionFromItem(item))
      return
    case 'review':
      sink.onReview?.(reviewFromItem(item))
      return
    case 'error':
      sink.onRuntimeError?.(runtimeErrorFromItem(item))
      return
  }
}

function eventTurnId(event: CoreRuntimeEventJson): string | undefined {
  return readRuntimeEventString(event, 'turnId', 'turn_id') ?? event.item?.turnId
}

function eventThreadId(event: CoreRuntimeEventJson): string | undefined {
  return readRuntimeEventString(event, 'threadId', 'thread_id') ?? event.item?.threadId
}

function eventItemId(event: CoreRuntimeEventJson): string | undefined {
  return readRuntimeEventString(event, 'itemId', 'item_id')
}

function eventApprovalId(event: CoreRuntimeEventJson): string | undefined {
  return readRuntimeEventString(event, 'approvalId', 'approval_id')
}

function eventInputId(event: CoreRuntimeEventJson): string | undefined {
  return readRuntimeEventString(event, 'inputId', 'input_id')
}

function compactionFromEvent(
  event: CoreRuntimeEventJson,
  status: CompactionEventPayload['status']
): CompactionEventPayload {
  const turnId = eventTurnId(event)
  return {
    itemId: eventItemId(event) ?? (turnId ? `compaction_${turnId}` : `compaction_${event.seq ?? Date.now()}`),
    ...(turnId ? { turnId } : {}),
    ...(typeof event.seq === 'number' ? { seq: event.seq } : {}),
    summary: event.summary ?? 'Context compacted',
    status,
    ...(event.timestamp ? { createdAt: event.timestamp } : {}),
    messagesBefore: event.replacedTokens,
    detail: event.pinnedConstraints?.join('\n'),
    auto: event.auto === true,
    ...(turnId ? { meta: { turnId } } : {})
  }
}

function toolReadyFromEvent(event: CoreRuntimeEventJson): ToolEventPayload | null {
  const callId = readRuntimeEventString(event, 'callId', 'call_id') ?? ''
  const toolName = readRuntimeEventString(event, 'toolName', 'tool_name') ?? ''
  if (!callId || !toolName) return null
  const sourceItemId = readRuntimeEventString(event, 'itemId', 'item_id')
  const readyCount = readRuntimeEventNumber(event, 'readyCount', 'ready_count')
  const partial = readRuntimeEventBoolean(event, 'partial')
  const turnId = eventTurnId(event)
  return {
    itemId: `tool_${callId}`,
    turnId,
    summary: toolName,
    status: 'running',
    toolKind: toolKindForRuntimeTool(toolName),
    meta: {
      ...(turnId ? { turnId } : {}),
      ...(sourceItemId ? { sourceItemId } : {}),
      callId,
      toolName,
      ...(readyCount !== undefined ? { readyCount } : {}),
      ...(partial !== undefined ? { partial } : {}),
      runtimeStatus: 'tool_call_ready'
    }
  }
}

function turnStartedFromEvent(event: CoreRuntimeEventJson): { threadId?: string; turnId: string; createdAt?: string; seq?: number } | null {
  const turnId = readRuntimeEventString(event, 'turnId', 'turn_id')
  if (!turnId) return null
  return {
    ...(readRuntimeEventString(event, 'threadId', 'thread_id') ? { threadId: readRuntimeEventString(event, 'threadId', 'thread_id') } : {}),
    turnId,
    createdAt: event.timestamp,
    ...(typeof event.seq === 'number' ? { seq: event.seq } : {})
  }
}

function turnCompletedFromEvent(event: CoreRuntimeEventJson): {
  threadId?: string
  turnId?: string
  createdAt?: string
  seq?: number
  acceptedFinalDigest?: string
  terminalReason?: CoreRuntimeEventJson['terminalReason']
} {
  const threadId = readRuntimeEventString(event, 'threadId', 'thread_id')
  const turnId = readRuntimeEventString(event, 'turnId', 'turn_id')
  return {
    ...(threadId ? { threadId } : {}),
    ...(turnId ? { turnId } : {}),
    ...(event.timestamp ? { createdAt: event.timestamp } : {}),
    ...(typeof event.seq === 'number' ? { seq: event.seq } : {}),
    ...(event.acceptedFinalDigest ? { acceptedFinalDigest: event.acceptedFinalDigest } : {}),
    ...(event.terminalReason ? { terminalReason: event.terminalReason } : {})
  }
}

function snapshotRequiredFromEvent(event: CoreRuntimeEventJson): SnapshotRequiredEventPayload {
  const threadId = readRuntimeEventString(event, 'threadId', 'thread_id')
  const sinceSeq = readRuntimeEventNumber(event, 'sinceSeq', 'since_seq')
  const highestSeq = readRuntimeEventNumber(event, 'highestSeq', 'highest_seq')
  const replayEventCount = readRuntimeEventNumber(event, 'replayEventCount', 'replay_event_count')
  const reason = readRuntimeEventString(event, 'reason')
  return {
    ...(threadId ? { threadId } : {}),
    ...(event.timestamp ? { createdAt: event.timestamp } : {}),
    ...(typeof event.seq === 'number' ? { seq: event.seq } : {}),
    ...(sinceSeq !== undefined ? { sinceSeq } : {}),
    ...(highestSeq !== undefined ? { highestSeq } : {}),
    ...(replayEventCount !== undefined ? { replayEventCount } : {}),
    ...(reason ? { reason } : {})
  }
}

function toolLifecycleFromEvent(event: CoreRuntimeEventJson): ToolEventPayload | null {
  const callId = readRuntimeEventString(event, 'callId', 'call_id') ?? ''
  const toolName = readRuntimeEventString(event, 'toolName', 'tool_name') ?? ''
  const sourceItemId = readRuntimeEventString(event, 'itemId', 'item_id')
  const itemId = callId
    ? `tool_${callId}`
    : sourceItemId
      ? sourceItemId
      : ''
  if (!itemId || !toolName) return null
  const isFinished = event.kind === 'tool_call_finished'
  const status: ToolEventPayload['status'] = isFinished
    ? event.status === 'error' || event.status === 'failed' || event.status === 'aborted'
      ? 'error'
      : 'success'
    : 'running'
  const child = normalizeChildMetadataForRenderer(event.child)
  const turnId = eventTurnId(event)
  return {
    itemId,
    turnId,
    summary: child
      ? fixedProgressSummary(child)
      : typeof event.summary === 'string' && event.summary.trim()
        ? event.summary.trim()
        : toolName,
    status,
    toolKind: toolKindForRuntimeTool(toolName, child),
    meta: {
      ...(turnId ? { turnId } : {}),
      ...(sourceItemId ? { sourceItemId } : {}),
      callId,
      ...(child ? {} : { toolName }),
      runtimeStatus: event.kind,
      ...(child ? { child } : {})
    }
  }
}

function toolProgressFromEvent(event: CoreRuntimeEventJson): ToolEventPayload | null {
  const sourceItemId = readRuntimeEventString(event, 'itemId', 'item_id')
  const callId = readRuntimeEventString(event, 'callId', 'call_id')
  const child = normalizeChildMetadataForRenderer(event.child)
  const itemId = childProgressItemId(child) ??
    (callId
      ? `tool_${callId}`
      : sourceItemId
        ? sourceItemId
        : '')
  if (!itemId) return null
  const status = event.status === 'success' || event.status === 'error'
    ? event.status
    : 'running'
  const toolName = readRuntimeEventString(event, 'toolName', 'tool_name')
  const turnId = eventTurnId(event)
  return {
    itemId,
    turnId,
    summary: fixedProgressSummary(child),
    status,
    toolKind: toolKindForRuntimeTool(toolName, child),
    meta: {
      ...(turnId ? { turnId } : {}),
      ...(sourceItemId ? { sourceItemId } : {}),
      runtimeStatus: 'tool_progress',
      ...(callId ? { callId } : {}),
      ...(child ? { child } : {})
    }
  }
}

function childToolStatus(child: NonNullable<RuntimeStatusEventPayload['child']>): ToolEventPayload['status'] {
  switch (child.childStatus) {
    case 'completed':
      return 'success'
    case 'failed':
    case 'aborted':
    case 'interrupted':
    case 'killed':
      return 'error'
    default:
      return 'running'
  }
}

function childToolEventFromStatus(status: RuntimeStatusEventPayload): ToolEventPayload | null {
  const child = status.child
  if (!child) return null
  const childId = child.childRunId ?? child.childId
  if (!childId) return null
  const itemId = child.childRunId
    ? `subagent_${child.childRunId}`
    : child.parentToolCallId
      ? `tool_${child.parentToolCallId}`
      : `subagent_${child.childId}`
  const diagnostics = normalizeJobDiagnostics(status.meta?.details)
  return {
    itemId,
    turnId: status.turnId ?? child.parentTurnId,
    summary: fixedProgressSummary(child),
    status: childToolStatus(child),
    toolKind: child.kind === 'background-shell' ? 'command_execution' : 'subagent',
    meta: {
      ...((status.turnId ?? child.parentTurnId) ? { turnId: status.turnId ?? child.parentTurnId } : {}),
      runtimeStatus: status.stage ?? status.kind,
      ...(status.stage ? { stage: status.stage } : {}),
      ...(child.parentToolCallId ? { callId: child.parentToolCallId } : {}),
      child,
      ...(diagnostics ? { diagnostics } : {})
    }
  }
}

const VISIBLE_PROVIDER_PROGRESS_STAGES = new Set(['pre_send', 'post_send', 'response_received'])

const BASE_PIPELINE_STAGE_LABELS = new Map<string, string>([
  ['setup', 'Setup'],
  ['pre_start', 'Pre-start'],
  ['post_start', 'Post-start'],
  ['input_received', 'Input received'],
  ['input_cached', 'Input cached'],
  ['input_routed', 'Input routed'],
  ['input_compressed', 'Input compressed'],
  ['input_remembered', 'Input remembered'],
  ['pre_send', 'Preparing model request'],
  ['post_send', 'Model request started'],
  ['response_received', 'Response received'],
  ['provider_retrying', 'Provider request is retrying'],
  ['step_limit_finalizing', 'Model step budget reached'],
  ['provider_error', 'Provider request failed'],
  ['empty_final_recovered', 'Empty provider response was replaced safely'],
  ['loop_guard', 'Loop guard activated']
])

const BACKGROUND_TERMINAL_STATUS_CODES = ['completed', 'failed', 'aborted', 'interrupted', 'killed'] as const

function fixedPipelineStage(stage: unknown): { stage: string; label: string; visibleWithoutChild: boolean } | null {
  if (typeof stage !== 'string' || stage !== stage.trim()) return null
  const baseLabel = BASE_PIPELINE_STAGE_LABELS.get(stage)
  if (baseLabel) {
    return {
      stage,
      label: baseLabel,
      visibleWithoutChild: VISIBLE_PROVIDER_PROGRESS_STAGES.has(stage) || stage === 'provider_retrying' || stage === 'provider_error' || stage === 'empty_final_recovered'
    }
  }
  if (stage.startsWith('subagent_') && closedDiagnosticCode(stage.slice('subagent_'.length), CHILD_STATUS_CODES)) {
    return { stage, label: 'Subagent lifecycle update', visibleWithoutChild: false }
  }
  if (stage.startsWith('background_job_delivery_') && closedDiagnosticCode(stage.slice('background_job_delivery_'.length), RUNTIME_DELIVERY_STATUS_CODES)) {
    return { stage, label: 'Background task delivery update', visibleWithoutChild: false }
  }
  if (stage.startsWith('background_job_auto_continue_') && closedDiagnosticCode(stage.slice('background_job_auto_continue_'.length), RUNTIME_AUTO_CONTINUE_STATUS_CODES)) {
    return { stage, label: 'Background task continuation update', visibleWithoutChild: false }
  }
  if (stage.startsWith('background_job_') && closedDiagnosticCode(stage.slice('background_job_'.length), BACKGROUND_TERMINAL_STATUS_CODES)) {
    return { stage, label: 'Background task lifecycle update', visibleWithoutChild: false }
  }
  return null
}

/**
 * Final renderer-side projection for progress/child tool events. Stores call
 * this again even when the event came from this mapper so an alternate or
 * test provider cannot inject presentation text through a ThreadEventSink.
 */
export function projectToolEventForRenderer(event: ToolEventPayload): ToolEventPayload | null {
  const meta = event.meta && typeof event.meta === 'object' && !Array.isArray(event.meta)
    ? event.meta
    : {}
  const child = normalizeChildMetadataForRenderer(meta.child as CoreChildRuntimeMetadataJson | undefined)
  const runtimeStatus = typeof meta.runtimeStatus === 'string' ? meta.runtimeStatus.trim() : ''
  const restricted = runtimeStatus === 'tool_progress' || Boolean(child)
  if (!restricted) return event
  const itemId = rendererIdentifier(event.itemId)
  if (!itemId) return null
  const turnId = rendererIdentifier(event.turnId) ?? rendererIdentifier(meta.turnId)
  const sourceItemId = rendererIdentifier(meta.sourceItemId)
  const callId = rendererIdentifier(meta.callId)
  const diagnostics = normalizeJobDiagnostics(meta.diagnostics)
  const explicitStage = fixedPipelineStage(meta.stage ?? runtimeStatus)?.stage
  const childStage = child ? fixedPipelineStage(`subagent_${child.childStatus}`)?.stage : undefined
  const stage = explicitStage ?? childStage
  const safeMeta: Record<string, unknown> = {
    ...(turnId ? { turnId } : {}),
    ...(sourceItemId ? { sourceItemId } : {}),
    ...(callId ? { callId } : {}),
    runtimeStatus: runtimeStatus === 'tool_progress' ? 'tool_progress' : stage ?? 'child_lifecycle_unknown',
    ...(stage ? { stage } : {}),
    ...(child ? { child } : {}),
    ...(diagnostics ? { diagnostics } : {})
  }
  return {
    itemId,
    ...(turnId ? { turnId } : {}),
    summary: fixedProgressSummary(child),
    status: event.status === 'success' || event.status === 'error' ? event.status : 'running',
    toolKind: child?.kind === 'background-shell'
      ? 'command_execution'
      : child
        ? 'subagent'
        : event.toolKind === 'command_execution' || event.toolKind === 'file_change' || event.toolKind === 'subagent'
          ? event.toolKind
          : 'tool_call',
    meta: safeMeta
  }
}

function runtimeStatusFromEvent(event: CoreRuntimeEventJson): RuntimeStatusEventPayload | null {
  const turnId = eventTurnId(event)
  const threadId = eventThreadId(event)
  if (event.kind === 'error' && event.code === 'compaction_summary_fallback') {
    const key = turnId ?? threadId ?? event.seq ?? Date.now()
    return {
      kind: 'compaction_summary_fallback',
      itemId: `runtime_status_${key}_compaction_summary_fallback`,
      turnId,
      createdAt: event.timestamp,
      message: 'Context compaction summary is unavailable.'
    }
  }
  if (event.kind === 'tool_result_upload_wait') {
    const turnKey = turnId ?? threadId ?? event.seq ?? Date.now()
    const toolResultCount = readRuntimeEventNumber(event, 'toolResultCount', 'tool_result_count') ?? 0
    return {
      kind: 'tool_result_upload_wait',
      itemId: `runtime_status_${turnKey}_tool_upload_wait`,
      turnId,
      createdAt: event.timestamp,
      toolResultCount
    }
  }
  if (event.kind === 'tool_catalog_changed') {
    const key = event.fingerprint ?? event.seq ?? Date.now()
    return {
      kind: 'tool_catalog_changed',
      itemId: `runtime_status_tool_catalog_${key}`,
      turnId,
      createdAt: event.timestamp,
      ...(event.changeKind ? { changeKind: event.changeKind } : {}),
      message: 'Tool catalog changed.'
    }
  }
  if (event.kind === 'tool_storm_suppressed') {
    const callId = readRuntimeEventString(event, 'callId', 'call_id') ?? ''
    const toolName = readRuntimeEventString(event, 'toolName', 'tool_name') ?? ''
    if (!callId || !toolName) return null
    return {
      kind: 'tool_storm_suppressed',
      itemId: eventItemId(event) ?? `runtime_status_tool_storm_${callId}`,
      turnId,
      createdAt: event.timestamp,
      message: 'Repeated tool activity was suppressed.',
      toolName,
      callId
    }
  }
  if (event.kind === 'pipeline_stage') {
    const child = normalizeChildMetadataForRenderer(event.child)
    const key = child?.childRunId ?? child?.childId ?? event.seq ?? Date.now()
    const projection = fixedPipelineStage(event.stage)
    if (!projection || (!child && !projection.visibleWithoutChild)) return null
    const { stage, label } = projection
    const providerProgress = !child && VISIBLE_PROVIDER_PROGRESS_STAGES.has(stage)
    if (providerProgress && !turnId) return null
    const details = event.details && typeof event.details === 'object' && !Array.isArray(event.details)
      ? event.details as Record<string, unknown>
      : undefined
    const publicDetails = normalizeJobDiagnostics(details)
    const providerError = normalizeProviderErrorDiagnostics(details?.providerError)
    const projectedDetails = publicDetails || providerError
      ? { ...(publicDetails ?? {}), ...(providerError ? { providerError } : {}) }
      : undefined
    const attempt = readStructuredNumber(event as Record<string, unknown>, 'attempt') ?? readStructuredNumber(details, 'attempt')
    const maxAttempt = readStructuredNumber(event as Record<string, unknown>, 'maxAttempt') ?? readStructuredNumber(details, 'maxAttempt')
    return {
      kind: 'pipeline_stage',
      itemId: providerProgress
        ? `runtime_status_${turnId}_provider_progress`
        : `runtime_status_${turnId ?? threadId ?? 'thread'}_${stage}_${key}`,
      turnId,
      createdAt: event.timestamp,
      stage,
      label,
      message: label,
      ...(attempt !== undefined ? { attempt } : {}),
      ...(maxAttempt !== undefined ? { maxAttempt } : {}),
      ...(child ? { child } : {}),
      meta: {
        stage,
        ...(attempt !== undefined ? { attempt } : {}),
        ...(maxAttempt !== undefined ? { maxAttempt } : {}),
        ...(projectedDetails ? { details: projectedDetails } : {}),
        ...(child ? { child } : {})
      }
    }
  }
  return null
}

function childLifecycleStatusFromEvent(event: CoreRuntimeEventJson): RuntimeStatusEventPayload | null {
  const child = normalizeChildMetadataForRenderer(event.child)
  if (!child) return null
  const turnId = eventTurnId(event)
  const threadId = eventThreadId(event)
  const stage =
    event.kind === 'turn_started'
      ? 'subagent_running'
      : event.kind === 'turn_completed'
        ? 'subagent_completed'
        : event.kind === 'turn_aborted'
          ? 'subagent_aborted'
          : event.kind === 'turn_failed'
            ? 'subagent_failed'
            : ''
  if (!stage) return null
  const key = child.childRunId ?? child.childId
  const label = fixedPipelineStage(stage)?.label ?? 'Subagent lifecycle update'
  const publicDetails = normalizeJobDiagnostics(event.details)
  return {
    kind: 'pipeline_stage',
    itemId: `runtime_status_${turnId ?? threadId ?? child.parentTurnId}_${stage}_${key}`,
    turnId: turnId ?? child.parentTurnId,
    createdAt: event.timestamp,
    stage,
    label,
    message: label,
    child,
    meta: {
      stage,
      ...(publicDetails ? { details: publicDetails } : {}),
      child
    }
  }
}

function childSteerStatusFromEvent(event: CoreRuntimeEventJson): RuntimeStatusEventPayload | null {
  const child = normalizeChildMetadataForRenderer(event.child)
  if (!child) return null
  const turnId = eventTurnId(event) ?? child.parentTurnId
  const threadId = eventThreadId(event)
  const status = closedDiagnosticCode(
    readRuntimeEventString(event, 'status', 'steerStatus', 'steer_status')
    ?? child.steerStatus
    ?? String(event.kind ?? '').replace(/^child_steer_/, ''),
    CHILD_STEER_STATUS_CODES
  )
  if (!status) return null
  const steerMessageId = readRuntimeEventString(event, 'steerMessageId', 'steer_message_id') ?? child.steerMessageId
  const stage = typeof event.kind === 'string' ? event.kind : `child_steer_${status}`
  const label = 'Subagent control update'
  const message = 'Subagent steer state changed'
  return {
    kind: 'pipeline_stage',
    itemId: `runtime_status_${turnId ?? threadId ?? 'thread'}_${stage}_${steerMessageId ?? child.childRunId ?? child.childId}`,
    turnId,
    createdAt: readRuntimeEventString(event, 'createdAt') ?? event.timestamp,
    stage,
    label,
    message,
    child,
    meta: {
      stage,
      steerMessageId,
      steerStatus: status,
      child
    }
  }
}

function childPauseStatusFromEvent(event: CoreRuntimeEventJson): RuntimeStatusEventPayload | null {
  const child = normalizeChildMetadataForRenderer(event.child)
  if (!child) return null
  const turnId = eventTurnId(event) ?? child.parentTurnId
  const threadId = eventThreadId(event)
  const rawStatus = readRuntimeEventString(event, 'status', 'pauseStatus', 'pause_status')
    ?? child.pauseStatus
    ?? String(event.kind ?? '').replace(/^child_/, '').replace(/^pause_/, '')
  const normalizedStatus = rawStatus === 'pause_requested'
    ? 'requested'
    : rawStatus === 'resume_requested'
      ? 'resume_requested'
      : rawStatus
  const status = closedDiagnosticCode(normalizedStatus, CHILD_PAUSE_STATUS_CODES)
  if (!status) return null
  const pauseRequestId = readRuntimeEventString(event, 'pauseRequestId', 'pause_request_id') ?? child.pauseRequestId
  const stage = typeof event.kind === 'string' ? event.kind : `child_pause_${status}`
  const label = 'Subagent control update'
  const message = 'Subagent pause state changed'
  return {
    kind: 'pipeline_stage',
    itemId: `runtime_status_${turnId ?? threadId ?? 'thread'}_${stage}_${pauseRequestId ?? child.childRunId ?? child.childId}`,
    turnId,
    createdAt: readRuntimeEventString(event, 'pausedAt', 'resumedAt', 'createdAt') ?? event.timestamp,
    stage,
    label,
    message,
    child,
    meta: {
      stage,
      pauseRequestId,
      pauseStatus: status,
      child
    }
  }
}

	/** Coalesces consecutive public assistant deltas into one store update. */
export async function dispatchAnalytixRuntimeEvents(
  events: unknown[],
  sink: ThreadEventSink,
  handleApprovalRequest: (event: CoreRuntimeEventJson, sink: ThreadEventSink) => Promise<void>
): Promise<AcceptedFinalProjectionReceiptV1 | undefined> {
  const acceptedFinalCount = events.filter((event) =>
    event && typeof event === 'object' &&
    (event as { kind?: unknown }).kind === 'accepted_final_batch'
  ).length
  const generalTerminalCount = events.filter((event) =>
    event && typeof event === 'object' &&
    (event as { kind?: unknown }).kind === 'general_terminal_batch'
  ).length
  const atomicTerminalCount = acceptedFinalCount + generalTerminalCount
  if (atomicTerminalCount > 0 && (atomicTerminalCount !== 1 || events.length !== 1)) {
    throw new Error('atomic terminal delivery batch cannot be mixed with other renderer events')
  }
  let acceptedFinalReceipt: AcceptedFinalProjectionReceiptV1 | undefined
  for (const event of events) {
    if (event && typeof event === 'object' && (event as { kind?: unknown }).kind === 'accepted_final_batch') {
      const projection = acceptedFinalProjectionBatchFromRuntime(event)
      if (!projection || !sink.onAcceptedFinalBatch) {
        throw new Error('accepted-final delivery batch has no atomic renderer sink')
      }
      if (acceptedFinalReceipt) {
        throw new Error('multiple accepted-final delivery batches cannot share one renderer dispatch')
      }
      const committedReceipt = await sink.onAcceptedFinalBatch(projection)
      if (!acceptedFinalProjectionReceiptsEqual(committedReceipt, projection.receipt)) {
        throw new Error('accepted-final renderer commit receipt is mismatched')
      }
      acceptedFinalReceipt = committedReceipt
      continue
    }
    if (event && typeof event === 'object' && (event as { kind?: unknown }).kind === 'general_terminal_batch') {
      const projection = generalTerminalProjectionBatchFromRuntime(event)
      if (!projection || !sink.onGeneralTerminalBatch) {
        throw new Error('general terminal delivery batch has no atomic renderer sink')
      }
      await sink.onGeneralTerminalBatch(projection)
      continue
    }
    await dispatchAnalytixRuntimeEvent(event as CoreRuntimeEventJson, sink, handleApprovalRequest)
  }
  return acceptedFinalReceipt
}

export function generalTerminalProjectionBatchFromRuntime(
  value: unknown
): GeneralTerminalProjectionBatch | null {
  const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(value)
  if (!parsed.success || containsInternalCaseEntityReference(value)) return null
  const batch = parsed.data
  const usageEvent = batch.events[batch.events.length - 2]
  const terminalEvent = batch.events[batch.events.length - 1]
  if (usageEvent.kind !== 'usage' ||
      (terminalEvent.kind !== 'turn_completed' && terminalEvent.kind !== 'turn_failed' &&
        terminalEvent.kind !== 'turn_aborted')) return null
  let terminalItem: GeneralTerminalProjectionBatch['terminalItem']
  if (batch.events.length === 3) {
    const itemEvent = batch.events[0]
    if (itemEvent.kind !== 'item_completed') return null
    if (itemEvent.item.kind === 'assistant_text' && 'ordinaryResult' in itemEvent.item &&
        containsProtectedCaseFactCandidate(itemEvent.item.text)) return null
    const projected = chatBlockFromItem(itemEvent.item as CoreTurnItemJson)
    if (!projected || (projected.kind !== 'assistant' && projected.kind !== 'system')) return null
    terminalItem = projected
  }
  return {
    batchDigest: batch.batchDigest,
    threadId: batch.threadId,
    turnId: batch.turnId,
    firstSeq: batch.firstSeq,
    lastSeq: batch.lastSeq,
    ...(terminalItem ? { terminalItem } : {}),
    usage: usageFromCore(usageEvent.usage as CoreUsageSnapshotJson, {
      model: usageEvent.model,
      usageSource: usageEvent.usageSource,
      childRunId: usageEvent.childRunId,
      cacheDiagnostics: usageEvent.cacheDiagnostics
    }),
    terminal: {
      status: terminalEvent.status,
      createdAt: terminalEvent.timestamp,
      terminalReason: terminalEvent.terminalReason
    }
  }
}

export function acceptedFinalProjectionBatchFromRuntime(
  value: unknown
): AcceptedFinalProjectionBatch | null {
  const parsed = AcceptedFinalDeliveryBatchV2Schema.safeParse(value)
  if (!parsed.success) return null
  const batch: AcceptedFinalDeliveryBatchV2 = parsed.data
  const assistantEvent = batch.events[0]
  const usageEvent = batch.events[batch.events.length - 2]
  const terminalEvent = batch.events[batch.events.length - 1]
  if (assistantEvent.kind !== 'item_completed' || usageEvent.kind !== 'usage' ||
      (terminalEvent.kind !== 'turn_completed' && terminalEvent.kind !== 'turn_failed' &&
        terminalEvent.kind !== 'turn_aborted')) return null
  const assistant = sealedAcceptedFinalAssistantBlockFromItem(assistantEvent.item as CoreTurnItemJson)
  if (!assistant || assistant.kind !== 'assistant') return null
  let terminalError: Extract<ChatBlock, { kind: 'system' }> | undefined
  if (batch.events.length === 4) {
    const errorEvent = batch.events[1]
    if (errorEvent.kind !== 'item_completed' || errorEvent.item.kind !== 'error') return null
    const projected = chatBlockFromItem(errorEvent.item as CoreTurnItemJson)
    if (!projected || projected.kind !== 'system') return null
    terminalError = projected
  }
  const identity = {
    batchId: batch.batchId,
    threadId: batch.threadId,
    turnId: batch.turnId,
    publicationCommitId: batch.publicationCommitId,
    firstSeq: batch.firstSeq,
    lastSeq: batch.lastSeq
  }
  return {
    ...identity,
    receipt: acceptedFinalProjectionReceiptFromBatch(identity),
    assistant,
    ...(terminalError ? { terminalError } : {}),
    usage: usageFromCore(usageEvent.usage as CoreUsageSnapshotJson, {
      model: usageEvent.model,
      providerId: usageEvent.providerId,
      effort: usageEvent.effort,
      usageSource: usageEvent.usageSource,
      childRunId: usageEvent.childRunId,
      cacheDiagnostics: usageEvent.cacheDiagnostics
    }),
    terminal: {
      status: terminalEvent.status,
      createdAt: terminalEvent.timestamp,
      acceptedFinalDigest: terminalEvent.acceptedFinalDigest,
      terminalReason: terminalEvent.terminalReason
    }
  }
}

export async function dispatchAnalytixRuntimeEvent(
  event: CoreRuntimeEventJson,
  sink: ThreadEventSink,
  handleApprovalRequest: (event: CoreRuntimeEventJson, sink: ThreadEventSink) => Promise<void>
): Promise<void> {
  switch (event.kind) {
    case 'thread_created':
    case 'thread_updated': {
      const threadId = eventThreadId(event)
      if (threadId) {
        sink.onThreadLifecycle?.({
          threadId,
          ...(event.title !== undefined ? { title: normalizeThreadTitleFromCore(event.title, threadId) } : {}),
          ...(event.status ? { status: event.status } : {}),
          ...(event.timestamp ? { createdAt: event.timestamp } : {}),
          ...(event.seq !== undefined ? { seq: event.seq } : {})
        })
      }
      return
    }
    case 'thread_rewound': {
      const threadId = eventThreadId(event)
      const rewindTurnId = readRuntimeEventString(event, 'rewindTurnId', 'rewind_turn_id')
      if (threadId && rewindTurnId) {
        sink.onThreadRewound?.({
          threadId,
          turnId: rewindTurnId,
          removedTurns: readRuntimeEventNumber(event, 'removedTurns', 'removed_turns') ?? 0,
          remainingTurns: readRuntimeEventNumber(event, 'remainingTurns', 'remaining_turns') ?? 0,
          removedTurnIds: Array.isArray(event.removedTurnIds) ? event.removedTurnIds.filter((item) => typeof item === 'string') : [],
          ...(event.seq !== undefined ? { seq: event.seq } : {})
        })
      }
      return
    }
    case 'assistant_text_delta':
      return
    case 'item_created':
    case 'item_updated':
    case 'item_completed':
    case 'tool_call_started':
    case 'tool_call_finished':
      if (event.item) emitItem(event.item, sink, event.child)
      else {
        const tool = toolLifecycleFromEvent(event)
        if (tool) sink.onTool(tool)
      }
      return
    case 'tool_call_ready': {
      const tool = toolReadyFromEvent(event)
      if (tool) sink.onTool(tool)
      return
    }
    case 'tool_progress': {
      const tool = toolProgressFromEvent(event)
      if (tool) sink.onTool(tool)
      return
    }
    case 'tool_result_upload_wait': {
      const status = runtimeStatusFromEvent(event)
      if (status) sink.onRuntimeStatus?.(status)
      return
    }
    case 'pipeline_stage': {
      const status = runtimeStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
      }
      return
    }
    case 'tool_catalog_changed': {
      const status = runtimeStatusFromEvent(event)
      if (status) sink.onRuntimeStatus?.(status)
      return
    }
    case 'child_steer_queued':
    case 'child_steer_admitted':
    case 'child_steer_rejected': {
      const status = childSteerStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
      }
      return
    }
    case 'child_pause_requested':
    case 'child_paused':
    case 'child_resume_requested':
    case 'child_resumed':
    case 'child_pause_rejected': {
      const status = childPauseStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
      }
      return
    }
    case 'tool_storm_suppressed': {
      const status = runtimeStatusFromEvent(event)
      if (status) sink.onRuntimeStatus?.(status)
      return
    }
    case 'approval_requested':
      await handleApprovalRequest(event, sink)
      return
    case 'approval_resolved':
    {
      const itemId = eventItemId(event)
      const approvalId = eventApprovalId(event)
      sink.onApprovalStatus?.({
        itemId: itemId ?? approvalId ?? `approval_${event.seq ?? Date.now()}`,
        approvalId: approvalId ?? itemId ?? '',
        status:
          event.status === 'allowed'
            ? 'allowed'
            : event.status === 'denied'
              ? 'denied'
              : 'error',
        ...(event.status === 'expired' ? { errorMessage: 'Approval expired because the turn was interrupted.' } : {})
      })
      return
    }
    case 'user_input_requested':
      sink.onUserInput(
        userInputRequestFromCore({
          itemId: eventItemId(event),
          inputId: eventInputId(event),
          turnId: eventTurnId(event),
          prompt: event.prompt,
          questions: event.questions,
          seq: event.seq
        })
      )
      return
    case 'user_input_resolved': {
      const status = event.status === 'submitted'
        ? 'submitted'
        : event.status === 'cancelled'
          ? 'cancelled'
          : 'error'
      const itemId = eventItemId(event)
      const inputId = eventInputId(event)
      sink.onUserInputStatus({
        itemId: itemId ?? inputId ?? `input_${event.seq ?? Date.now()}`,
        ...(inputId ? { requestId: inputId } : {}),
        status,
        ...(status === 'error' ? { errorMessage: event.message ?? 'User input was not submitted.' } : {})
      })
      return
    }
    case 'compaction_started':
      sink.onCompaction(compactionFromEvent(event, 'running'))
      return
    case 'compaction_completed':
      sink.onCompaction(compactionFromEvent(event, 'success'))
      return
    case 'goal_updated':
    {
      const threadId = eventThreadId(event) ?? event.goal?.threadId ?? ''
      sink.onGoal({
        threadId,
        goal: event.goal ? goalFromCore(event.goal) : null,
        createdAt: event.timestamp
      })
      return
    }
    case 'goal_cleared':
      sink.onGoal({
        threadId: eventThreadId(event) ?? '',
        goal: null,
        cleared: true,
        createdAt: event.timestamp
      })
      return
    case 'todos_updated':
      sink.onTodos?.({
        threadId: eventThreadId(event) ?? event.todos?.threadId ?? '',
        todos: event.todos ? todosFromCore(event.todos) : null,
        createdAt: event.timestamp
      })
      return
    case 'todos_cleared':
      sink.onTodos?.({
        threadId: eventThreadId(event) ?? '',
        todos: null,
        cleared: true,
        createdAt: event.timestamp
      })
      return
    case 'usage':
      if (event.usage) {
        sink.onUsage?.(usageFromCore(event.usage, {
          model: readRuntimeEventString(event, 'model'),
          providerId: readRuntimeEventString(event, 'providerId', 'provider_id'),
          effort: exactClosedDiagnosticCode(event.effort, MODEL_REASONING_EFFORT_CODES),
          usageSource: readRuntimeEventString(event, 'usageSource', 'usage_source'),
          childRunId: readRuntimeEventString(event, 'childRunId', 'child_run_id'),
          cacheDiagnostics: event.cacheDiagnostics
        }))
      }
      return
    case 'turn_started': {
      const status = childLifecycleStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
        return
      }
      const started = turnStartedFromEvent(event)
      if (started) sink.onTurnStarted?.(started)
      return
    }
    case 'turn_completed': {
      const status = childLifecycleStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
        return
      }
      await sink.onTurnComplete(turnCompletedFromEvent(event))
      return
    }
    case 'turn_aborted': {
      const status = childLifecycleStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
        return
      }
      await sink.onError(errorForRuntimeEvent({
        itemId: eventItemId(event) ?? `runtime_error_${eventTurnId(event) ?? eventThreadId(event) ?? event.seq ?? Date.now()}`,
        createdAt: event.timestamp,
        message: event.message ?? 'Turn aborted.',
        code: 'aborted',
        severity: 'info'
      }), {
        terminal: true,
        threadId: eventThreadId(event),
        turnId: eventTurnId(event),
        seq: event.seq,
        status: 'aborted',
        ...(event.acceptedFinalDigest ? { acceptedFinalDigest: event.acceptedFinalDigest } : {}),
        ...(event.terminalReason ? { terminalReason: event.terminalReason } : {})
      })
      return
    }
    case 'turn_failed': {
      const status = childLifecycleStatusFromEvent(event)
      if (status) {
        sink.onRuntimeStatus?.(status)
        const childTool = childToolEventFromStatus(status)
        if (childTool) sink.onTool(childTool)
        return
      }
      const payload = runtimeErrorFromEvent(event, 'Analytix turn failed')
      sink.onRuntimeError?.(payload)
      await sink.onError(errorForRuntimeEvent(payload), {
        terminal: true,
        threadId: eventThreadId(event),
        turnId: eventTurnId(event),
        seq: event.seq,
        status: 'failed',
        ...(event.acceptedFinalDigest ? { acceptedFinalDigest: event.acceptedFinalDigest } : {}),
        ...(event.terminalReason ? { terminalReason: event.terminalReason } : {})
      })
      return
    }
    case 'turn_steered':
      sink.onTurnSteered?.({
        turnId: eventTurnId(event),
        createdAt: event.timestamp,
        text: typeof event.text === 'string' ? event.text : undefined,
        clientUserMessageId:
          readRuntimeEventString(event, 'clientUserMessageId', 'client_user_message_id'),
        admittedSeq: readRuntimeEventNumber(event, 'admittedSeq', 'admitted_seq') ?? event.seq
      })
      return
    case 'error':
      if (event.code === 'compaction_summary_fallback') {
        const status = runtimeStatusFromEvent(event)
        if (status) sink.onRuntimeStatus?.(status)
        return
      }
      {
        const payload = runtimeErrorFromEvent(event, 'Runtime error')
        sink.onRuntimeError?.(payload)
        if (runtimeErrorEventIsTerminal(event)) {
          await sink.onError(errorForRuntimeEvent(payload), {
            terminal: true,
            threadId: eventThreadId(event),
            turnId: eventTurnId(event),
            seq: event.seq,
            status: 'failed',
            ...(event.acceptedFinalDigest ? { acceptedFinalDigest: event.acceptedFinalDigest } : {}),
            ...(event.terminalReason ? { terminalReason: event.terminalReason } : {})
          })
        }
      }
      return
    case 'mcp_lifecycle_audit':
    case 'autoresearch_state_audit':
    case 'goal_evidence_audit':
    case 'checkpoint_captured':
    case 'checkpoint_rewind_rescue_created':
    case 'checkpoint_rewind_applied':
    case 'heartbeat':
      return
    case 'snapshot_required':
      await sink.onSnapshotRequired?.(snapshotRequiredFromEvent(event))
      return
    default:
      return
  }
}
