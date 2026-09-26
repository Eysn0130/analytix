import type {
  AcceptedFinalProjectionBatch,
  ChatBlock,
  CompactionBlock,
  GeneralTerminalProjectionBatch,
  NormalizedThread,
  ReviewBlock,
  ReviewEventPayload,
  RuntimeChildMetadata,
  RuntimeDisclosureMetadata,
  RuntimeProviderErrorDiagnosticsMetadata,
  RuntimeProviderRecoveryDiagnosticsMetadata,
  RuntimeProviderRetryDiagnosticsMetadata,
  SnapshotRequiredEventPayload,
  RuntimeStatusEventPayload,
  TurnCompletedEventPayload,
  ThreadEventSink,
  ToolBlock,
  ToolEventPayload,
  AgentProvider
} from '../agent/types'
import {
  acceptedFinalProjectionBatchIsSelfConsistent,
  acceptedFinalProjectionHasCandidateReceipt,
  acceptedFinalProjectionIsAlreadyCommitted,
  acceptedFinalProjectionReceiptsEqual,
  acceptedFinalUsageSnapshotsEqual
} from '../agent/accepted-final-projection-receipt'
import { getProvider } from '../agent/registry'
import { projectToolEventForRenderer } from '../agent/analytix-mapper'
import { generatedArtifactMetadataSchema } from '../../../../packages/runtime/src/contracts/generated-artifact'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { describeRuntimeError, formatRuntimeError, getRuntimeErrorCode } from '../lib/format-runtime-error'
import { invalidateThreadDetailCache } from '../lib/thread-detail-cache'
import {
  markPublicProjectionRevoked,
  threadReferencesRevokedProjection
} from '../lib/public-projection-revocation'
import { isClawWorkspacePath, isInternalTemporaryWorkspace, normalizeWorkspaceRoot } from '../lib/workspace-path'
import type { ClawImChannelV1 } from '@shared/app-settings'
import { redactSecretText } from '@shared/secret-redaction'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import type { ChatState } from './chat-store-types'
import { isClawThread } from './chat-store-helpers'
import {
  clearedThreadSelection,
  collectAssistantTextForTurn,
  findLatestUserBlockId,
  reconcileOptimisticUserBlock,
  settlePendingRuntimeWorkAfterInterrupt,
  threadSnapshotLooksRunning,
  upsertUserBlock
} from './chat-store-runtime-helpers'
import {
  isWriteThreadId,
  type WriteThreadRegistry
} from '../write/write-thread-registry'
import {
  forgetThreadWorktree,
  readThreadWorktreeRegistry,
  saveThreadWorktreeRegistry
} from '../lib/thread-worktree-registry'
import { isSddAssistantThread } from '../sdd/sdd-thread-registry'
import {
  notifySddChatTranscriptMirror,
  purgeSddChatTranscriptForThread
} from '../sdd/sdd-chat-transcript'
import { invalidateSharedThreadSummary } from '../components/summary/thread-summary-resource'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import {
  armBusyWatchdog as armBusyWatchdogImpl,
  clearBusyWatchdog,
  resetBusyRecoveryAttempts,
  syncTurnCompletionPoll as syncTurnCompletionPollImpl
} from './chat-store-schedulers'
import { createStreamingDeltaScheduler } from '../thread/streaming/streaming-delta-scheduler'
import {
  appendActiveStreamDeltas,
  clearActiveStream,
  getActiveStreamSnapshotFor,
  isPendingActiveStreamTurnId,
  migrateActiveStreamTurn,
  pendingActiveStreamTurnId
} from '../thread/streaming/active-stream-store'
import {
  createThreadTraceEvent,
  isThreadTraceEnabled,
  PersistedThreadTraceSink
} from '../thread/tracing/thread-performance-trace'

const BUSY_WATCHDOG_MS = 180_000
const MAX_BUSY_RECOVERY_ATTEMPTS = 3
const MAX_RUNTIME_EVENT_TIMER_AGE_MS = 30 * 60_000
const CLOCK_SKEW_TOLERANCE_MS = 5_000
const RUNTIME_STREAM_RECOVERING_KEY = 'common:runtimeStreamRecovering'
const LEGACY_RUNTIME_STREAM_RECOVERING_VALUE = 'runtimeStreamRecovering'
const COMPLETION_NOTIFICATION_DEDUPE_LIMIT = 200
const LEGACY_LIVE_STORE_UPDATE_MIN_MS = 250
export const MAX_WATCHED_COMPLETION_NOTIFICATIONS = 200
export const MAX_PENDING_CLAW_FEISHU_MIRRORS = 50
const completionNotificationKeys: string[] = []
const completionNotificationKeySet = new Set<string>()
const watchCompletionNotificationKeys = new Map<string, string>()

export type PendingClawFeishuMirror = {
  threadId: string
  userBlockId: string
  userText: string
}

const pendingClawFeishuMirrors = new Map<string, PendingClawFeishuMirror>()

function trimBlocksForThreadRewind(blocks: ChatBlock[], removedTurnIds: string[]): {
  blocks: ChatBlock[]
  droppedUserIds: string[]
} {
  const removed = new Set(removedTurnIds.map((item) => item.trim()).filter(Boolean))
  if (removed.size === 0) return { blocks, droppedUserIds: [] }
  const index = blocks.findIndex((block) => (
    block.kind === 'user' &&
    typeof block.meta?.turnId === 'string' &&
    removed.has(block.meta.turnId)
  ))
  if (index < 0) return { blocks, droppedUserIds: [] }
  const droppedUserIds = blocks
    .slice(index)
    .filter((block): block is Extract<ChatBlock, { kind: 'user' }> => block.kind === 'user')
    .map((block) => block.id)
  return {
    blocks: blocks.slice(0, index),
    droppedUserIds
  }
}

export function watchTurnCompletionNotification(threadId: string, now = Date.now()): void {
  const normalizedThreadId = threadId.trim()
  if (!normalizedThreadId) return
  watchCompletionNotificationKeys.delete(normalizedThreadId)
  watchCompletionNotificationKeys.set(normalizedThreadId, `watch:${normalizedThreadId}:${now}`)
  while (watchCompletionNotificationKeys.size > MAX_WATCHED_COMPLETION_NOTIFICATIONS) {
    const oldestThreadId = watchCompletionNotificationKeys.keys().next().value
    if (!oldestThreadId) break
    watchCompletionNotificationKeys.delete(oldestThreadId)
  }
}

export function completionNotificationDedupeKeyForWatchedThread(
  threadId: string | null | undefined,
  now = Date.now()
): string {
  const normalizedThreadId = threadId?.trim()
  if (!normalizedThreadId) return `watch:unknown:${now}`
  return watchCompletionNotificationKeys.get(normalizedThreadId) ?? `watch:${normalizedThreadId}:${now}`
}

export function clearWatchedCompletionNotifications(): void {
  watchCompletionNotificationKeys.clear()
}

export function rememberPendingClawFeishuMirror(
  turnId: string,
  mirror: PendingClawFeishuMirror
): void {
  const normalizedTurnId = turnId.trim()
  const normalizedMirror = {
    threadId: mirror.threadId.trim(),
    userBlockId: mirror.userBlockId.trim(),
    userText: projectOrdinaryPublicText(mirror.userText.trim())
  }
  if (
    !normalizedTurnId ||
    !normalizedMirror.threadId ||
    !normalizedMirror.userBlockId ||
    !normalizedMirror.userText
  ) {
    return
  }
  pendingClawFeishuMirrors.delete(normalizedTurnId)
  pendingClawFeishuMirrors.set(normalizedTurnId, normalizedMirror)
  while (pendingClawFeishuMirrors.size > MAX_PENDING_CLAW_FEISHU_MIRRORS) {
    const oldestTurnId = pendingClawFeishuMirrors.keys().next().value
    if (!oldestTurnId) break
    pendingClawFeishuMirrors.delete(oldestTurnId)
  }
}

export function takePendingClawFeishuMirror(
  turnId: string | null | undefined
): PendingClawFeishuMirror | undefined {
  const normalizedTurnId = turnId?.trim()
  if (!normalizedTurnId) return undefined
  const mirror = pendingClawFeishuMirrors.get(normalizedTurnId)
  pendingClawFeishuMirrors.delete(normalizedTurnId)
  return mirror
}

export function clearPendingClawFeishuMirrors(): void {
  pendingClawFeishuMirrors.clear()
}

export function clearPendingClawFeishuMirrorsForThread(threadId: string): void {
  const normalizedThreadId = threadId.trim()
  if (!normalizedThreadId) return
  for (const [turnId, mirror] of pendingClawFeishuMirrors.entries()) {
    if (mirror.threadId === normalizedThreadId) pendingClawFeishuMirrors.delete(turnId)
  }
}

function isUserInputInterruptError(message: string | undefined): boolean {
  const lowered = message?.toLowerCase() ?? ''
  return lowered.includes('cancel') && lowered.includes('awaiting user input')
}

function isInterruptSettledError(error: unknown, message: string): boolean {
  const code = getRuntimeErrorCode(error)
  if (code === 'aborted') return true
  if (isUserInputInterruptError(message)) return true
  const raw = error instanceof Error ? error.message : String(error ?? '')
  const lowered = `${message}\n${raw}`.toLowerCase()
  return lowered.includes('interrupted') ||
    lowered.includes('aborted') ||
    lowered.includes('cancelled') ||
    lowered.includes('canceled')
}

export async function readWriteWorkspaceRoots(): Promise<string[]> {
  try {
    const settings = await rendererRuntimeClient.getSettings()
    const roots = [
      settings.write.defaultWorkspaceRoot,
      settings.write.activeWorkspaceRoot,
      ...settings.write.workspaces
    ]
      .map((workspaceRoot) => normalizeWorkspaceRoot(workspaceRoot))
      .filter(Boolean)
    return [...new Set(roots)]
  } catch {
    return []
  }
}

export function runtimeErrorDetail(error: unknown): string {
  const view = describeRuntimeError(error)
  if (view.detail) return view.detail
  const raw = error instanceof Error ? error.message : String(error ?? '')
  return raw === view.summary ? '' : raw
}

export function runtimeStreamRecoveringMessage(): string {
  return i18n.t(RUNTIME_STREAM_RECOVERING_KEY)
}

function isRuntimeStreamRecoveringError(error: string | null | undefined): boolean {
  return (
    error === runtimeStreamRecoveringMessage() ||
    error === LEGACY_RUNTIME_STREAM_RECOVERING_VALUE ||
    error === RUNTIME_STREAM_RECOVERING_KEY
  )
}

function clearRuntimeStreamRecoveringError(error: string | null): string | null {
  return isRuntimeStreamRecoveringError(error) ? null : error
}

function runtimeEventStartedAt(createdAt: string | undefined, now = Date.now()): number {
  if (!createdAt) return now
  const parsed = Date.parse(createdAt)
  if (!Number.isFinite(parsed)) return now
  if (parsed > now + CLOCK_SKEW_TOLERANCE_MS) return now
  if (now - parsed > MAX_RUNTIME_EVENT_TIMER_AGE_MS) return now
  return parsed
}

export function forkedMessageCount(blocks: ChatBlock[]): number {
  return blocks.filter((block) => block.kind === 'user' || block.kind === 'assistant').length
}

export function forkedTurnCount(blocks: ChatBlock[]): number {
  return blocks.filter((block) => block.kind === 'user').length
}

function rememberCompletionNotificationKey(key: string): boolean {
  if (!key) return true
  if (completionNotificationKeySet.has(key)) return false
  completionNotificationKeySet.add(key)
  completionNotificationKeys.push(key)
  while (completionNotificationKeys.length > COMPLETION_NOTIFICATION_DEDUPE_LIMIT) {
    const stale = completionNotificationKeys.shift()
    if (stale) completionNotificationKeySet.delete(stale)
  }
  return true
}

export function clearWatchedCompletionNotification(threadId: string): void {
  const normalizedThreadId = threadId.trim()
  if (!normalizedThreadId) return
  watchCompletionNotificationKeys.delete(normalizedThreadId)
}

function notifyTurnComplete(threadId: string | null, state: ChatState, dedupeKey: string): void {
  if (
    !threadId ||
    typeof window === 'undefined' ||
    typeof window.analytix?.app?.showTurnCompleteNotification !== 'function'
  ) return
  if (!rememberCompletionNotificationKey(dedupeKey)) return

  const threadTitle =
    state.threads.find((thread) => thread.id === threadId)?.title?.trim() ||
    i18n.t('common:untitledThread')

  void window.analytix
    .app.showTurnCompleteNotification({
      threadId,
      title: i18n.t('common:turnCompleteNotificationTitle'),
      body: i18n.t('common:turnCompleteNotificationBody', { title: threadTitle })
    })
    .then((result) => {
      if (result.ok || typeof window.analytix?.logs?.error !== 'function') return
      void window.analytix.logs.error('notification', 'Turn completion notification failed', {
        message: result.message,
        threadId
      }).catch(() => undefined)
    })
    .catch((error: unknown) => {
      if (typeof window.analytix?.logs?.error !== 'function') return
      void window.analytix.logs.error('notification', 'Turn completion notification failed', {
        message: error instanceof Error ? error.message : String(error),
        threadId
      }).catch(() => undefined)
    })
}

function releaseThreadWorktreeIfNeeded(threadId: string | null): void {
  if (!threadId || typeof window === 'undefined') return
  if (typeof window.analytix?.workspace?.releaseWorktree !== 'function') return
  const record = readThreadWorktreeRegistry().worktrees[threadId]
  if (!record) return
  void window.analytix.workspace
    .releaseWorktree({
      projectPath: record.projectPath,
      poolIndex: record.poolIndex
    })
    .catch(() => undefined)
  saveThreadWorktreeRegistry(forgetThreadWorktree(threadId))
}

/**
 * Compute the patch that finalizes timing for the current in-progress turn.
 * No-op if there is no current turn or its start time was not recorded.
 */
export function finalizeTurnTiming(state: ChatState): Partial<ChatState> {
  const userId = state.currentTurnUserId
  if (!userId) return {}
  const startedAt = state.turnStartedAtByUserId[userId]
  if (typeof startedAt !== 'number') {
    return { currentTurnUserId: null }
  }
  return {
    currentTurnUserId: null,
    turnDurationByUserId: {
      ...state.turnDurationByUserId,
      [userId]: Math.max(0, Date.now() - startedAt)
    }
  }
}

export function flushLiveBlocks(state: ChatState, base: Partial<ChatState> = {}): Partial<ChatState> {
  const nextBlocks = [...state.blocks]
  const now = Date.now()
  const createdAt = new Date(now).toISOString()
  const meta = state.currentTurnId ? { turnId: state.currentTurnId } : undefined
  if (state.liveAssistant.trim()) {
    nextBlocks.push({
      kind: 'assistant',
      id: `a-${now}`,
      createdAt,
      text: state.liveAssistant,
      ...(meta ? { meta } : {})
    })
  }
  if (nextBlocks.length === state.blocks.length) {
    return base
  }
  return {
    ...base,
    blocks: nextBlocks,
    liveAssistant: ''
  }
}

function goalStatusText(status: string): string {
  switch (status) {
    case 'active':
      return i18n.t('common:goalStatusActive')
    case 'paused':
      return i18n.t('common:goalStatusPaused')
    case 'blocked':
      return i18n.t('common:goalStatusBlocked')
    case 'usageLimited':
      return i18n.t('common:goalStatusUsageLimited')
    case 'budgetLimited':
      return i18n.t('common:goalStatusBudgetLimited')
    case 'complete':
      return i18n.t('common:goalStatusComplete')
    default:
      return status
  }
}

function goalTimelineText(goal: NonNullable<ChatState['activeThreadGoal']> | null, cleared?: boolean): string {
  if (!goal || cleared) return i18n.t('common:goalClearedTimeline')
  return i18n.t('common:goalUpdatedTimeline', {
    status: goalStatusText(goal.status),
    objective: goal.objective
  })
}

export function shouldOpenSettingsForError(error: unknown): boolean {
  return describeRuntimeError(error).settingsAction === 'agents'
}

export function looksLikeActiveTurnError(error: unknown): boolean {
  if (getRuntimeErrorCode(error) === 'turn_execution_conflict') return true
  const raw = error instanceof Error ? error.message : String(error ?? '')
  return raw.toLowerCase().includes('active turn')
}

export function isCodeThread(
  thread: NormalizedThread,
  clawChannels: ClawImChannelV1[] = [],
  _writeRegistry?: WriteThreadRegistry
): boolean {
  const workspace = normalizeWorkspaceRoot(thread.workspace)
  return Boolean(workspace) &&
    thread.archived !== true &&
    !isInternalTemporaryWorkspace(thread.workspace) &&
    !isClawWorkspacePath(thread.workspace) &&
    !isClawThread(thread, clawChannels) &&
    !isSddAssistantThread(thread)
}

export function latestThread(threads: NormalizedThread[]): NormalizedThread | null {
  return [...threads].sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))[0] ?? null
}

function normalizeFilePathForMatch(path?: string | null): string {
  return path?.trim().replace(/\\/g, '/').replace(/\/+$/, '') ?? ''
}

function isAbsoluteFilePath(path: string): boolean {
  return path.startsWith('/') || /^[A-Za-z]:\//.test(path)
}

function resolveWriteToolFilePath(filePath: string | undefined, workspaceRoot: string): string {
  const raw = normalizeFilePathForMatch(filePath)
  if (!raw) return ''
  if (isAbsoluteFilePath(raw)) return raw
  return `${normalizeFilePathForMatch(workspaceRoot)}/${raw.replace(/^\.?\//, '')}`
}

function notifyWriteWorkspaceFileRefresh(
  get: () => ChatState,
  event?: Pick<ToolEventPayload, 'filePath' | 'status' | 'toolKind'>
): void {
  if (get().route !== 'write' && get().route !== 'chat') return
  if (event && (event.toolKind !== 'file_change' || event.status !== 'success')) return

  const writeState = useWriteWorkspaceStore.getState()
  const workspaceRoot = normalizeFilePathForMatch(writeState.workspaceRoot)
  const activeFilePath = normalizeFilePathForMatch(writeState.activeFilePath)
  if (!workspaceRoot || !activeFilePath) return

  const candidatePath = resolveWriteToolFilePath(event?.filePath, workspaceRoot)
  const hasCandidate = candidatePath.length > 0
  const candidateInWorkspace = hasCandidate
    ? candidatePath === workspaceRoot || candidatePath.startsWith(`${workspaceRoot}/`)
    : true
  if (!candidateInWorkspace) return

  void useWriteWorkspaceStore.getState().refreshWorkspace(workspaceRoot)

  if (hasCandidate && candidatePath !== activeFilePath) return
  void useWriteWorkspaceStore.getState().syncActiveFileFromDisk(workspaceRoot, {
    path: activeFilePath,
    animate: true,
    force: true,
    reviewAsDiff: true
  })
}

function runtimeStatusText(event: RuntimeStatusEventPayload): string {
  if (event.child) return ''
  if (event.kind === 'tool_result_upload_wait') {
    return i18n.t('common:toolUploadWaitStatus', { count: event.toolResultCount ?? 0 })
  }
  if (event.kind === 'tool_catalog_changed') {
    return i18n.t('common:toolCatalogChangedStatus')
  }
  if (event.kind === 'tool_storm_suppressed') {
    return i18n.t('common:toolStormSuppressedStatus', { tool: 'tool' })
  }
  if (event.kind === 'compaction_summary_fallback') {
    return i18n.t('common:compactionSummaryFallbackStatus')
  }
  if (event.kind === 'pipeline_stage') {
    if (event.stage === 'provider_retrying') {
      return i18n.t('common:providerRetryingStatus', { defaultValue: '模型服务正在重试' })
    }
    if (event.stage === 'provider_error') {
      return i18n.t('common:providerErrorStatus', { defaultValue: '模型服务请求失败' })
    }
    if (event.stage === 'empty_final_recovered') {
      return i18n.t('common:emptyFinalRecoveredStatus', { defaultValue: '模型响应为空，已切换到安全恢复输出' })
    }
  }
  return ''
}

function readRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}

function readStringField(record: Record<string, unknown>, key: string): string | undefined {
  const value = record[key]
  return typeof value === 'string' && value.trim() ? redactSecretText(value.trim()) : undefined
}

function readNumberField(record: Record<string, unknown>, key: string): number | undefined {
  const value = record[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function normalizeProviderErrorDiagnostic(value: unknown): RuntimeProviderErrorDiagnosticsMetadata | undefined {
  const record = readRecord(value)
  if (!record) return undefined
  const status = readNumberField(record, 'status')
  const attempt = readNumberField(record, 'attempt')
  const failureStage = readStringField(record, 'failureStage')
  const dispatchState = readStringField(record, 'dispatchState')
  const closedFailureStage = failureStage && new Set([
    'request_validation', 'request_build', 'pre_send_body_audit', 'telemetry_begin',
    'transport_before_observed_send', 'transport_after_observed_send', 'local_admission_config',
    'callback_projection', 'unclassified'
  ]).has(failureStage) ? failureStage as RuntimeProviderErrorDiagnosticsMetadata['failureStage'] : undefined
  const closedDispatchState = dispatchState && new Set([
    'not_sent', 'sent', 'indeterminate'
  ]).has(dispatchState) ? dispatchState as RuntimeProviderErrorDiagnosticsMetadata['dispatchState'] : undefined
  const diagnostic: RuntimeProviderErrorDiagnosticsMetadata = {
    ...(readStringField(record, 'providerId') ? { providerId: readStringField(record, 'providerId') } : {}),
    ...(readStringField(record, 'family') ? { family: readStringField(record, 'family') } : {}),
    ...(readStringField(record, 'endpointFormat') ? { endpointFormat: readStringField(record, 'endpointFormat') } : {}),
    ...(status !== undefined ? { status } : {}),
    ...(readStringField(record, 'kind') ? { kind: readStringField(record, 'kind') } : {}),
    ...(typeof record.hasApiKey === 'boolean' ? { hasApiKey: record.hasApiKey } : {}),
    ...(readStringField(record, 'authStatus') ? { authStatus: readStringField(record, 'authStatus') } : {}),
    ...(typeof record.retryable === 'boolean' ? { retryable: record.retryable } : {}),
    ...(attempt !== undefined && Number.isSafeInteger(attempt) && attempt >= 0 && attempt <= 1_000_000
      ? { attempt }
      : {}),
    ...(closedFailureStage ? { failureStage: closedFailureStage } : {}),
    ...(closedDispatchState ? { dispatchState: closedDispatchState } : {})
  }
  return Object.keys(diagnostic).length > 0 ? diagnostic : undefined
}

function normalizeProviderRetryDiagnostic(event: RuntimeStatusEventPayload): RuntimeProviderRetryDiagnosticsMetadata | undefined {
  const meta = readRecord(event.meta)
  const details = readRecord(meta?.details)
  const attempt = event.attempt ?? readNumberField(meta ?? {}, 'attempt') ?? readNumberField(details ?? {}, 'attempt')
  const maxAttempt = event.maxAttempt ?? readNumberField(meta ?? {}, 'maxAttempt') ?? readNumberField(details ?? {}, 'maxAttempt')
  const retry: RuntimeProviderRetryDiagnosticsMetadata = {
    ...(attempt !== undefined ? { attempt } : {}),
    ...(maxAttempt !== undefined ? { maxAttempt } : {})
  }
  return Object.keys(retry).length > 0 ? retry : undefined
}

function normalizeProviderRecoveryDiagnostic(event: RuntimeStatusEventPayload): RuntimeProviderRecoveryDiagnosticsMetadata | undefined {
  const meta = readRecord(event.meta)
  const details = readRecord(meta?.details)
  const attempt = readNumberField(meta ?? {}, 'recoveryAttempt') ?? readNumberField(details ?? {}, 'recoveryAttempt')
  const maxAttempt = readNumberField(meta ?? {}, 'maxRecoveryAttempts') ?? readNumberField(details ?? {}, 'maxRecoveryAttempts')
  const kind = readStringField(meta ?? {}, 'recoveryKind') ?? readStringField(details ?? {}, 'recoveryKind')
  const recoveryExhausted = meta?.recoveryExhausted === true || details?.recoveryExhausted === true
  const recovery: RuntimeProviderRecoveryDiagnosticsMetadata = {
    ...(kind ? { kind } : {}),
    ...(attempt !== undefined ? { attempt } : {}),
    ...(maxAttempt !== undefined ? { maxAttempt } : {}),
    ...(recoveryExhausted ? { recoveryExhausted: true } : {})
  }
  return Object.keys(recovery).length > 0 ? recovery : undefined
}

function normalizeRuntimeStatusJobDiagnostics(value: unknown): RuntimeDisclosureMetadata['diagnostics'] | undefined {
  const record = readRecord(value)
  if (!record) return undefined
  const notificationKind = readStringField(record, 'notificationKind')
  if (
    notificationKind !== 'background_job_completion' &&
    notificationKind !== 'background_job_auto_continue' &&
    notificationKind !== 'background_job_delivery'
  ) {
    return undefined
  }
  const out: NonNullable<RuntimeDisclosureMetadata['diagnostics']> = {
    notificationKind
  }
  for (const key of ['jobId', 'childRunId', 'childThreadId', 'childTurnId', 'parentThreadId', 'parentTurnId', 'autoContinueTurnId', 'deliveryId', 'deliveryItemId'] as const) {
    const value = readStringField(record, key)
    if (value) out[key] = value
  }
  const kind = runtimeClosedCode(record.kind, RUNTIME_JOB_KIND_CODES)
  if (kind) out.kind = kind
  const status = runtimeClosedCode(record.status, RUNTIME_JOB_STATUS_CODES)
  if (status) out.status = status
  const heartbeatStatus = runtimeClosedCode(record.heartbeatStatus, RUNTIME_HEARTBEAT_STATUS_CODES)
  if (heartbeatStatus) out.heartbeatStatus = heartbeatStatus
  const autoContinueStatus = runtimeClosedCode(record.autoContinueStatus, RUNTIME_AUTO_CONTINUE_STATUS_CODES)
  if (autoContinueStatus) out.autoContinueStatus = autoContinueStatus
  const deliveryStatus = runtimeClosedCode(record.deliveryStatus, RUNTIME_DELIVERY_STATUS_CODES)
  if (deliveryStatus) out.deliveryStatus = deliveryStatus
  const recoveryStatus = runtimeClosedCode(record.recoveryStatus, RUNTIME_RECOVERY_STATUS_CODES)
  if (recoveryStatus) out.recoveryStatus = recoveryStatus
  const warningCode = runtimeClosedCode(record.warningCode, RUNTIME_WARNING_CODES)
  if (warningCode) out.warningCode = warningCode
  for (const key of ['heartbeatAgeMs', 'staleAfterMs', 'stalledAfterMs', 'recoveryAttempt', 'completionDeliveryAttempt'] as const) {
    const value = record[key]
    if (typeof value === 'number' && Number.isFinite(value)) out[key] = value
  }
  for (const key of ['terminal', 'background', 'canContinueParent', 'lateCompletionSuppressed', 'autoContinueParent', 'stalled', 'leaseExpired', 'orphaned'] as const) {
    if (typeof record[key] === 'boolean') out[key] = record[key]
  }
  return out
}

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
const RUNTIME_WARNING_CODES = ['task_job_stalled', 'task_job_stale', 'slow-output'] as const

function runtimeClosedCode<const T extends readonly string[]>(value: unknown, allowed: T): T[number] | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return allowed.includes(normalized as T[number]) ? normalized as T[number] : undefined
}

function runtimeStatusMeta(event: RuntimeStatusEventPayload): RuntimeDisclosureMetadata | undefined {
  const meta = readRecord(event.meta)
  const details = readRecord(meta?.details)
  const jobDiagnostics = normalizeRuntimeStatusJobDiagnostics(details)
  const providerError =
    normalizeProviderErrorDiagnostic(meta?.providerError) ??
    normalizeProviderErrorDiagnostic(details?.providerError)
  const providerRetry = normalizeProviderRetryDiagnostic(event)
  const providerRecovery = normalizeProviderRecoveryDiagnostic(event)
  const disclosure: RuntimeDisclosureMetadata = {
    ...(event.turnId ? { turnId: event.turnId } : {}),
    ...(event.child ? { child: event.child } : {}),
    ...(jobDiagnostics ? { diagnostics: jobDiagnostics } : {}),
    ...(providerError ? { providerError } : {}),
    ...(providerRetry ? { providerRetry } : {}),
    ...(providerRecovery ? { providerRecovery } : {})
  }
  return Object.keys(disclosure).length > 0 ? disclosure : undefined
}

function runtimeErrorPayloadToError(event: {
  message: string
  code?: string
  details?: unknown
  severity?: string
}): Error {
  return new Error(JSON.stringify({
    ...(event.code ? { code: event.code } : {}),
    message: event.message,
    ...(event.details !== undefined ? { details: event.details } : {}),
    ...(event.severity ? { severity: event.severity } : {})
  }))
}

function recoverPendingSteeringBlocks(
  state: ChatState,
  reason: ChatState['queuedMessagesPausedReason']
): Partial<ChatState> {
  const pending = state.blocks.filter((block): block is Extract<ChatBlock, { kind: 'user' }> =>
    block.kind === 'user' &&
    (block.meta?.steeringStatus === 'pending' || block.meta?.steeringStatus === 'admitted')
  )
  if (pending.length === 0) return {}
  return {
    blocks: state.blocks.filter((block) =>
      !(block.kind === 'user' &&
        (block.meta?.steeringStatus === 'pending' || block.meta?.steeringStatus === 'admitted'))
    ),
    queuedMessages: [
      ...state.queuedMessages,
      ...pending.map((block, index) => ({
        id: block.meta?.clientUserMessageId || `q-steer-${Date.now()}-${index}`,
        text: block.text,
        ...(block.meta?.displayText ? { displayText: block.meta.displayText } : {}),
        ...(block.modelLabel ? { modelLabel: block.modelLabel } : {}),
        ...(block.meta?.attachmentIds?.length ? { attachmentIds: block.meta.attachmentIds } : {}),
        ...(block.meta?.attachments?.length ? { attachments: block.meta.attachments } : {}),
        ...(block.meta?.fileReferences?.length ? { fileReferences: block.meta.fileReferences } : {})
      }))
    ],
    queuedMessagesPausedReason: reason
  }
}

function normalizeRuntimeErrorText(value: string | undefined): string {
  return (value ?? '').replace(/\s+/g, ' ').trim()
}

function sameRuntimeErrorContent(
  left: Extract<ChatBlock, { kind: 'system' }>,
  right: Extract<ChatBlock, { kind: 'system' }>
): boolean {
  return (
    left.severity === right.severity &&
    left.code === right.code &&
    normalizeRuntimeErrorText(left.text) === normalizeRuntimeErrorText(right.text) &&
    normalizeRuntimeErrorText(left.detail) === normalizeRuntimeErrorText(right.detail)
  )
}

function findSameTurnRuntimeErrorIndex(
  blocks: ChatBlock[],
  block: Extract<ChatBlock, { kind: 'system' }>
): number {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const candidate = blocks[index]
    if (candidate.kind === 'user') break
    if (candidate.kind === 'system' && sameRuntimeErrorContent(candidate, block)) return index
  }
  return -1
}

function upsertRuntimeErrorBlock(blocks: ChatBlock[], block: Extract<ChatBlock, { kind: 'system' }>): ChatBlock[] {
  const index = blocks.findIndex((candidate) => candidate.kind === 'system' && candidate.id === block.id)
  if (index < 0) {
    const duplicateIndex = findSameTurnRuntimeErrorIndex(blocks, block)
    if (duplicateIndex < 0) return [...blocks, block]
    const next = [...blocks]
    const existing = next[duplicateIndex]
    next[duplicateIndex] = {
      ...block,
      createdAt: existing?.createdAt ?? block.createdAt
    }
    return next
  }
  const next = [...blocks]
  next[index] = block
  return next
}

export function armBusyWatchdog(
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void,
  get: () => ChatState
): void {
  armBusyWatchdogImpl(set, get, {
    timeoutMs: BUSY_WATCHDOG_MS,
    maxAttempts: MAX_BUSY_RECOVERY_ATTEMPTS,
    finalizeBusyState: finalizeTurnTiming,
    flushLiveBlocks: (state, base) => {
      const blocks = settlePendingRuntimeWorkAfterInterrupt(
        stripProvisionalAssistantForTurn(state.blocks, state.currentTurnId, state.currentTurnUserId)
      )
      return {
        ...base,
        blocks,
        liveAssistant: '',
        currentTurnId: state.currentTurnId,
        currentTurnUserId: state.currentTurnUserId,
        queuedMessagesPausedReason: 'failed'
      }
    },
    busyTimeoutMessage: () => i18n.t('common:busyTimeout', { minutes: Math.round((BUSY_WATCHDOG_MS * MAX_BUSY_RECOVERY_ATTEMPTS) / 60_000) }),
    onFinalTimeout: (state) => clearActiveStream(state.activeThreadId, state.currentTurnId)
  })
}

export function syncTurnCompletionPoll(
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void,
  get: () => ChatState
): void {
  syncTurnCompletionPollImpl(set, get, {
    loadThreadState: async (state, threadId) => {
      const provider = getProvider()
      return provider.getThreadDetail(threadId)
    },
    threadLooksRunning: threadSnapshotLooksRunning,
    onCompletedThreads: async (doneIds, state, setState, getState) => {
      for (const id of doneIds) {
        clearActiveStream(id)
        notifyTurnComplete(
          id,
          state,
          completionNotificationDedupeKeyForWatchedThread(id)
        )
        clearWatchedCompletionNotification(id)
      }
      setState((snapshot) => {
        const watchTurnCompletion = { ...snapshot.watchTurnCompletion }
        const unreadThreadIds = { ...snapshot.unreadThreadIds }
        for (const id of doneIds) {
          delete watchTurnCompletion[id]
          unreadThreadIds[id] = true
        }
        return { watchTurnCompletion, unreadThreadIds }
      })
      void getState().refreshThreads()
    }
  })
}

export type ThreadEventSinkBinding = {
  threadId?: string
  signal?: AbortSignal
  /**
   * Seq the subscription replays from. Deltas at or below this floor are
   * duplicates of text already in the timeline (a replayed backlog or a
   * re-delivered live event) and are dropped instead of appended, so a
   * stale cursor can no longer re-stream the whole conversation into the
   * live bubble.
   */
  sinceSeq?: number
  getThreadDetail?: AgentProvider['getThreadDetail']
}

function createLiveTextBuffer(initial = ''): {
  append(text: string): void
  hasText(): boolean
  text(): string
  reset(): void
} {
  let chunks = initial ? [initial] : []
  let cached = initial

  return {
    append(text) {
      if (!text) return
      chunks.push(text)
      cached = ''
    },
    hasText() {
      return chunks.length > 0 && this.text().trim().length > 0
    },
    text() {
      if (!cached && chunks.length > 0) cached = chunks.join('')
      return cached
    },
    reset() {
      chunks = []
      cached = ''
    }
  }
}

function blockBelongsToTurn(block: ChatBlock, turnId: string): boolean {
  if (!('meta' in block) || typeof block.meta?.turnId !== 'string') return false
  return block.meta.turnId.trim() === turnId
}

function toolEventCallId(ev: ToolEventPayload): string {
  return typeof ev.meta?.callId === 'string' ? ev.meta.callId.trim() : ''
}

function toolEventTurnId(ev: ToolEventPayload, currentTurnId: string | null | undefined): string {
  if (typeof ev.turnId === 'string' && ev.turnId.trim()) return ev.turnId.trim()
  if (typeof ev.meta?.turnId === 'string' && ev.meta.turnId.trim()) return ev.meta.turnId.trim()
  return currentTurnId?.trim() ?? ''
}

function toolBlockCallId(block: ToolBlock): string {
  return typeof block.meta?.callId === 'string' ? block.meta.callId.trim() : ''
}

function toolBlockTurnId(block: ToolBlock): string {
  return typeof block.meta?.turnId === 'string' ? block.meta.turnId.trim() : ''
}

function toolChildMetadata(value: unknown): RuntimeChildMetadata | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const child = value as RuntimeChildMetadata
  if (!child.childRunId && !child.childId) return undefined
  return child
}

function toolEventChild(ev: ToolEventPayload): RuntimeChildMetadata | undefined {
  return toolChildMetadata(ev.meta?.child)
}

function toolBlockChild(block: ToolBlock): RuntimeChildMetadata | undefined {
  return toolChildMetadata(block.meta?.child)
}

function childIdentity(child: RuntimeChildMetadata | undefined): string {
  return child?.childRunId?.trim() || child?.childId?.trim() || ''
}

function childTurnBoundary(child: RuntimeChildMetadata | undefined, fallbackTurnId: string): string {
  return child?.parentTurnId?.trim() || fallbackTurnId
}

function childThreadBoundary(child: RuntimeChildMetadata | undefined): string {
  return child?.parentThreadId?.trim() || ''
}

function toolBlockMatchesChildEvent(
  block: ToolBlock,
  ev: ToolEventPayload,
  eventTurnId: string,
  blockTurnId: string
): boolean {
  const eventChild = toolEventChild(ev)
  const blockChild = toolBlockChild(block)
  const eventChildId = childIdentity(eventChild)
  if (!eventChild || !blockChild || !eventChildId || childIdentity(blockChild) !== eventChildId) return false
  const eventChildTurn = childTurnBoundary(eventChild, eventTurnId)
  const blockChildTurn = childTurnBoundary(blockChild, blockTurnId)
  if (eventChildTurn && blockChildTurn && eventChildTurn !== blockChildTurn) return false
  const eventChildThread = childThreadBoundary(eventChild)
  const blockChildThread = childThreadBoundary(blockChild)
  if (eventChildThread && blockChildThread && eventChildThread !== blockChildThread) return false
  return true
}

function mergeToolMeta(
  current: ToolBlock['meta'] | undefined,
  incoming: ToolEventPayload['meta'] | undefined,
  turnId: string
): ToolBlock['meta'] {
  const currentChild = toolChildMetadata(current?.child)
  const incomingChild = toolChildMetadata(incoming?.child)
  return {
    ...(current ?? {}),
    ...(incoming ?? {}),
    ...(currentChild || incomingChild ? { child: { ...(currentChild ?? {}), ...(incomingChild ?? {}) } } : {}),
    ...(turnId ? { turnId } : {})
  }
}

function toolBlockMatchesEvent(
  block: ToolBlock,
  ev: ToolEventPayload,
  currentTurnId: string | null | undefined
): boolean {
  const eventTurnId = toolEventTurnId(ev, currentTurnId)
  const blockTurnId = toolBlockTurnId(block)
  const eventChild = toolEventChild(ev)
  const blockChild = toolBlockChild(block)
  if (toolBlockMatchesChildEvent(block, ev, eventTurnId, blockTurnId)) return true
  if (eventChild) {
    if (blockChild) return false
    return block.id === ev.itemId
  }
  if (block.id === ev.itemId) {
    if (eventTurnId && blockTurnId) return eventTurnId === blockTurnId
    if (eventTurnId && !blockTurnId) return false
    if (!eventTurnId && blockTurnId && currentTurnId?.trim()) return blockTurnId === currentTurnId.trim()
    return !currentTurnId?.trim()
  }
  const callId = toolEventCallId(ev)
  if (!callId || toolBlockCallId(block) !== callId) return false
  if (eventTurnId && blockTurnId) return eventTurnId === blockTurnId
  if (eventTurnId && !blockTurnId) return currentTurnId?.trim() === eventTurnId
  if (!eventTurnId && blockTurnId) return currentTurnId?.trim() === blockTurnId
  return false
}

function adoptTurnIdPatch(state: ChatState, turnId: string | null | undefined): Partial<ChatState> {
  const eventTurnId = turnId?.trim()
  if (!eventTurnId) return {}
  const currentTurnId = state.currentTurnId?.trim() ?? ''
  if (!currentTurnId || currentTurnId === eventTurnId || isPendingActiveStreamTurnId(currentTurnId)) {
    return { currentTurnId: eventTurnId }
  }
  return {}
}

const RUNTIME_FINAL_SNAPSHOT_UNAVAILABLE_KEY = 'common:runtimeFinalSnapshotUnavailable'

function stateTurnMatchesExpected(
  currentTurnId: string | null | undefined,
  expectedTurnId: string | null | undefined
): boolean {
  const current = currentTurnId?.trim()
  const expected = expectedTurnId?.trim()
  if (!expected) return !current
  if (!current) return false
  return current === expected || isPendingActiveStreamTurnId(current)
}

function stripProvisionalAssistantForTurn(
  blocks: ChatBlock[],
  turnId: string | null | undefined,
  userBlockId: string | null | undefined
): ChatBlock[] {
  const expectedTurnId = turnId?.trim()
  const expectedUserBlockId = userBlockId?.trim()
  const userIndex = expectedUserBlockId
    ? blocks.findIndex((block) => block.kind === 'user' && block.id === expectedUserBlockId)
    : -1
  let nextUserIndex = blocks.length
  if (userIndex >= 0) {
    for (let index = userIndex + 1; index < blocks.length; index += 1) {
      if (blocks[index]?.kind === 'user') {
        nextUserIndex = index
        break
      }
    }
  }
  return blocks.filter((block, index) => {
    if (block.kind !== 'assistant') return true
    if (expectedTurnId && blockBelongsToTurn(block, expectedTurnId)) return false
    return !(userIndex >= 0 && index > userIndex && index < nextUserIndex)
  })
}

export function quarantineTerminalTurnDraft(input: {
  threadId: string | null | undefined
  turnId: string | null | undefined
  userBlockId: string | null | undefined
  busy: boolean
  eventSeq?: number
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void
  get: () => ChatState
}): boolean {
  const threadId = input.threadId?.trim()
  if (!threadId) return false
  const expectedTurnId = input.turnId?.trim() || input.get().currentTurnId?.trim() || null
  const expectedUserBlockId = input.userBlockId?.trim() || input.get().currentTurnUserId?.trim() || null
  const initial = input.get()
  if (initial.activeThreadId !== threadId || !stateTurnMatchesExpected(initial.currentTurnId, expectedTurnId)) {
    return false
  }
  clearActiveStream(threadId)
  input.set((state) => {
    if (state.activeThreadId !== threadId || !stateTurnMatchesExpected(state.currentTurnId, expectedTurnId)) {
      return {}
    }
    return {
      blocks: stripProvisionalAssistantForTurn(state.blocks, expectedTurnId, expectedUserBlockId),
      liveAssistant: '',
      busy: input.busy,
      currentTurnId: expectedTurnId,
      currentTurnUserId: expectedUserBlockId,
      ...(input.eventSeq !== undefined ? { lastSeq: Math.max(state.lastSeq, input.eventSeq) } : {})
    }
  })
  return true
}

export async function reconcileTerminalTurnFromThreadDetail(input: {
  threadId: string | null | undefined
  turnId: string | null | undefined
  userBlockId: string | null | undefined
  eventSeq?: number
  terminalError?: string | null
  terminalErrorDetail?: string | null
  terminalStatus?: 'completed' | 'failed' | 'aborted'
  acceptedFinalDigest?: string
  loadThreadDetail: AgentProvider['getThreadDetail']
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void
  get: () => ChatState
}): Promise<boolean> {
  const threadId = input.threadId?.trim()
  if (!threadId) return false
  const expectedTurnId = input.turnId?.trim() || input.get().currentTurnId?.trim() || null
  const expectedUserBlockId = input.userBlockId?.trim() || input.get().currentTurnUserId?.trim() || null
  if (!quarantineTerminalTurnDraft({
    threadId,
    turnId: expectedTurnId,
    userBlockId: expectedUserBlockId,
    busy: true,
    eventSeq: input.eventSeq,
    set: input.set,
    get: input.get
  })) {
    return false
  }

  try {
    const detail = await input.loadThreadDetail(threadId)
    const detailTurnId = detail.latestTurnId?.trim()
    if (expectedTurnId && detailTurnId && detailTurnId !== expectedTurnId) {
      throw new Error('terminal thread snapshot belongs to a different turn')
    }
    const detailUserBlockId = detail.latestUserMessageId?.trim()
    if (expectedUserBlockId && detailUserBlockId && detailUserBlockId !== expectedUserBlockId) {
      throw new Error('terminal thread snapshot belongs to a different user message')
    }
    if (detail.historyAuthority === 'case_boundary_only_v1') {
      if (!expectedTurnId || detailTurnId !== expectedTurnId || !detail.latestTurnAcceptedFinalDigest) {
        throw new Error('case terminal thread snapshot has no accepted-final authority')
      }
      const acceptedAssistantExists = detail.blocks.some(
        (block) => block.kind === 'assistant' && blockBelongsToTurn(block, expectedTurnId)
      )
      if (!acceptedAssistantExists) {
        throw new Error('case terminal thread snapshot has no accepted assistant item')
      }
    }
    if (
      input.acceptedFinalDigest &&
      detail.latestTurnAcceptedFinalDigest !== input.acceptedFinalDigest
    ) {
      throw new Error('terminal event and thread snapshot accepted-final digests differ')
    }
    if (threadSnapshotLooksRunning(detail.blocks, detail.threadStatus)) {
      throw new Error('terminal thread snapshot is still running')
    }
    let applied = false
    input.set((state) => {
      if (state.activeThreadId !== threadId) return {}
      if (!stateTurnMatchesExpected(state.currentTurnId, expectedTurnId)) return {}
      applied = true
      const settledBlocks = settlePendingRuntimeWorkAfterInterrupt(detail.blocks)
      const steeringRecovery = recoverPendingSteeringBlocks(
        { ...state, blocks: settledBlocks },
        input.terminalStatus === 'aborted' ? 'interrupted' : 'failed'
      )
      const blocks = steeringRecovery.blocks ?? settledBlocks
      const timing = finalizeTurnTiming({
        ...state,
        currentTurnId: expectedTurnId,
        currentTurnUserId: expectedUserBlockId
      })
      const watchTurnCompletion = { ...state.watchTurnCompletion }
      delete watchTurnCompletion[threadId]
      const unreadThreadIds = { ...state.unreadThreadIds }
      delete unreadThreadIds[threadId]
      clearWatchedCompletionNotification(threadId)
      return {
        ...timing,
        blocks,
        lastSeq: Math.max(state.lastSeq, detail.latestSeq, input.eventSeq ?? 0),
        liveAssistant: '',
        busy: false,
        currentTurnId: null,
        currentTurnUserId: null,
        activeThreadGoal: detail.goal ?? state.activeThreadGoal,
        activeThreadTodos: detail.todos ?? state.activeThreadTodos,
        error: input.terminalStatus === 'failed' ? input.terminalError ?? null : null,
        runtimeErrorDetail: input.terminalStatus === 'failed' ? input.terminalErrorDetail ?? null : null,
        queuedMessagesPausedReason: null,
        watchTurnCompletion,
        unreadThreadIds,
        ...steeringRecovery
      }
    })
    if (!applied) return false
    clearActiveStream(threadId)
    return true
  } catch (error) {
    input.set((state) => {
      if (state.activeThreadId !== threadId || !stateTurnMatchesExpected(state.currentTurnId, expectedTurnId)) {
        return {}
      }
      return {
        blocks: stripProvisionalAssistantForTurn(state.blocks, expectedTurnId, expectedUserBlockId),
        liveAssistant: '',
        busy: false,
        currentTurnId: expectedTurnId,
        currentTurnUserId: expectedUserBlockId,
        error: i18n.t(RUNTIME_FINAL_SNAPSHOT_UNAVAILABLE_KEY),
        runtimeErrorDetail: null,
        queuedMessagesPausedReason: 'failed',
        ...(input.eventSeq !== undefined ? { lastSeq: Math.max(state.lastSeq, input.eventSeq) } : {})
      }
    })
    if (typeof window !== 'undefined') {
      void window.analytix?.logs?.error?.('turn-terminal-reconcile', 'Failed to load trusted terminal snapshot', {
        message: error instanceof Error ? error.message : String(error),
        threadId,
        turnId: expectedTurnId
      }).catch(() => undefined)
    }
    return false
  }
}

async function reconcileThreadSnapshotFromDetail(input: {
  event: SnapshotRequiredEventPayload
  fallbackThreadId: string | null | undefined
  loadThreadDetail: AgentProvider['getThreadDetail']
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void
  get: () => ChatState
}): Promise<void> {
  const threadId = input.event.threadId?.trim() || input.fallbackThreadId?.trim()
  if (!threadId) return
  const cursorSeq = Math.max(input.event.seq ?? 0, input.event.highestSeq ?? 0)

  try {
    const detail = await input.loadThreadDetail(threadId)
    input.set((state) => {
      if (state.activeThreadId !== threadId) {
        return cursorSeq ? { lastSeq: Math.max(state.lastSeq, cursorSeq) } : {}
      }
      const busy = threadSnapshotLooksRunning(detail.blocks, detail.threadStatus)
      const blocks = busy ? detail.blocks : settlePendingRuntimeWorkAfterInterrupt(detail.blocks)
      clearActiveStream(threadId)
      return {
        blocks,
        liveAssistant: '',
        lastSeq: Math.max(state.lastSeq, detail.latestSeq, cursorSeq),
        busy,
        currentTurnId: busy ? detail.latestTurnId ?? state.currentTurnId : null,
        currentTurnUserId: busy ? detail.latestUserMessageId ?? state.currentTurnUserId : null,
        activeThreadGoal: detail.goal ?? state.activeThreadGoal,
        activeThreadTodos: detail.todos ?? state.activeThreadTodos,
        error: clearRuntimeStreamRecoveringError(state.error)
      }
    })
  } catch (error) {
    if (typeof window === 'undefined') return
    void window.analytix?.logs?.error?.('thread-snapshot-reconcile', 'Failed to reconcile runtime snapshot', {
      message: error instanceof Error ? error.message : String(error),
      threadId
    }).catch(() => undefined)
  }
}

export function buildThreadEventSink(
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void,
  get: () => ChatState,
  binding: ThreadEventSinkBinding = {}
): ThreadEventSink {
  const boundThreadId = binding.threadId?.trim() ?? ''
  const loadThreadDetail = binding.getThreadDetail ?? ((threadId: string) => getProvider().getThreadDetail(threadId))
  const traceSink = new PersistedThreadTraceSink()
  const traceThreadId = (): string | undefined => boundThreadId || get().activeThreadId || undefined
  let appliedDeltaSeqFloor = binding.sinceSeq ?? 0
  const legacyLiveAssistant = createLiveTextBuffer(get().liveAssistant ?? '')
  let legacyLiveStoreLastPublishedAt = 0
  let terminalEventBarrier = false
  const isCurrentStream = (): boolean => {
    if (binding.signal?.aborted) return false
    return !boundThreadId || get().activeThreadId === boundThreadId
  }
  const invalidateBoundThreadDetail = (threadId?: string | null): void => {
    const targetThreadId = threadId?.trim() || traceThreadId()
    if (targetThreadId) invalidateThreadDetailCache(targetThreadId)
  }
  const streamingDeltaScheduler = createStreamingDeltaScheduler({
    onBuffered: (deltaCount) => {
      const bufferedAt = Date.now()
      traceSink.record(
        createThreadTraceEvent('thread.delta.buffered', {
          threadId: traceThreadId(),
          data: {
            deltas: deltaCount,
            renderer_delta_buffered_at: bufferedAt
          }
        })
      )
    },
    onFlushed: ({ assistant, lastSeq, turnId }) => {
      const flushedAt = Date.now()
      traceSink.record(
        createThreadTraceEvent('thread.delta.flushed', {
          threadId: traceThreadId(),
          data: {
            assistantChars: assistant.length,
            lastSeq,
            turnId,
            renderer_delta_flushed_at: flushedAt
          }
        })
      )
    },
    onFlush: ({ lastSeq, turnId }) => {
      if (!isCurrentStream()) return
      const assistant = ''
      const threadId = traceThreadId()
      const currentTurnUserId = get().currentTurnUserId
      const currentTurnId = get().currentTurnId
      const activeTurnId = turnId ?? currentTurnId ?? pendingActiveStreamTurnId(threadId, currentTurnUserId)
      if (turnId && currentTurnId && isPendingActiveStreamTurnId(currentTurnId) && currentTurnId !== turnId) {
        migrateActiveStreamTurn(threadId, currentTurnId, turnId)
      }
      if (assistant) legacyLiveAssistant.append(assistant)
      const now = Date.now()
      const shouldPublishLegacyLive =
        Boolean(assistant) &&
        (legacyLiveStoreLastPublishedAt === 0 ||
          now - legacyLiveStoreLastPublishedAt >= LEGACY_LIVE_STORE_UPDATE_MIN_MS)
      if (shouldPublishLegacyLive) {
        legacyLiveStoreLastPublishedAt = now
      }
      appendActiveStreamDeltas({
        threadId,
        turnId: activeTurnId,
        assistant,
        lastSeq,
        startedAt: currentTurnUserId
          ? get().turnStartedAtByUserId[currentTurnUserId] ?? null
          : null
      })
      if (typeof window !== 'undefined' && assistant && isThreadTraceEnabled()) {
        window.requestAnimationFrame(() => {
          traceSink.record(
            createThreadTraceEvent('thread.delta.flushed', {
              threadId: traceThreadId(),
              data: {
                lastSeq,
                turnId: activeTurnId,
                renderer_live_painted_at: Date.now()
              }
            })
          )
        })
      }
      set((s) => {
        resetBusyRecoveryAttempts()
        const nextError = clearRuntimeStreamRecoveringError(s.error)
        const nextLastSeq = Math.max(s.lastSeq, lastSeq)
        const nextLiveAssistant = shouldPublishLegacyLive ? legacyLiveAssistant.text() : null
        const base: Partial<ChatState> = {
          error: nextError,
          ...(nextLastSeq !== s.lastSeq ? { lastSeq: nextLastSeq } : {}),
          ...((!s.currentTurnId || isPendingActiveStreamTurnId(s.currentTurnId)) && activeTurnId
            ? { currentTurnId: activeTurnId }
            : {})
        }
        if (!s.busy) {
          base.busy = true
          armBusyWatchdog(set, get)
        }
        return {
          ...base,
          ...(nextLiveAssistant !== null && nextLiveAssistant !== s.liveAssistant
            ? { liveAssistant: nextLiveAssistant }
            : {})
        }
      })
    },
    flushFirstSynchronously: true
  })
  const flushLegacyLiveStore = (): void => {
    if (!isCurrentStream()) return
    const state = get()
    const nextLiveAssistant = legacyLiveAssistant.text()
    if (state.liveAssistant === nextLiveAssistant) return
    legacyLiveStoreLastPublishedAt = Date.now()
    set((s) => ({
      ...(nextLiveAssistant !== s.liveAssistant ? { liveAssistant: nextLiveAssistant } : {})
    }))
  }
  const flushLiveBlocksAndResetStream = (
    state: ChatState,
    base: Partial<ChatState> = {}
  ): Partial<ChatState> => {
    const activeTurnId = state.currentTurnId?.trim() || null
    const activeStream = activeTurnId
      ? getActiveStreamSnapshotFor(state.activeThreadId, activeTurnId)
      : null
    const activeStreamBelongsToTurn =
      Boolean(activeTurnId) &&
      activeStream?.turnId === activeTurnId &&
      activeStream.lastSeq <= state.lastSeq
    const activeAssistant = activeStreamBelongsToTurn ? activeStream?.liveAssistant ?? '' : ''
    const hadLiveSegment = Boolean(
      state.liveAssistant.trim() ||
        legacyLiveAssistant.hasText() ||
        activeAssistant.trim()
    )
    const flushState =
      !state.liveAssistant.trim() && activeAssistant.trim()
        ? {
            ...state,
            liveAssistant: activeAssistant
          }
        : state
    const flushed = flushLiveBlocks(flushState, base)
    if (hadLiveSegment) {
      legacyLiveAssistant.reset()
      legacyLiveStoreLastPublishedAt = 0
      clearActiveStream(state.activeThreadId, state.currentTurnId)
    }
    return flushed
  }
  const flushPendingStreamingDeltas = (): void => {
    streamingDeltaScheduler.flushNow()
    flushLegacyLiveStore()
  }
  const discardPendingStreamingDeltas = (): void => {
    streamingDeltaScheduler.discard()
    legacyLiveAssistant.reset()
    legacyLiveStoreLastPublishedAt = 0
  }

  return {
    onSeq: (seq) => {
      if (!isCurrentStream()) return
      traceSink.record(
        createThreadTraceEvent('thread.event.batch_received', {
          threadId: traceThreadId(),
          data: { seq }
        })
      )
      resetBusyRecoveryAttempts()
      // Re-arm the busy watchdog on every live tick so it behaves as an
      // *inactivity* timer rather than an absolute one. onSeq fires for
      // every SSE batch — both content events and the runtime's 15s
      // heartbeat (analytix events route) — so a healthy turn always keeps the
      // watchdog postponed, even a long-running tool call that produces no
      // output for minutes. Recovery ("正在恢复运行时事件流…") then only
      // triggers after the heartbeat genuinely stops for BUSY_WATCHDOG_MS
      // (a dead stream), instead of on any turn that simply runs past it.
      if (get().busy) armBusyWatchdog(set, get)
      // Monotonic: heartbeats and replays must never rewind the cursor —
      // a rewound lastSeq becomes the next subscription's since_seq and
      // replays history.
      set((s) => ({
        lastSeq: Math.max(s.lastSeq, seq),
        error: clearRuntimeStreamRecoveringError(s.error)
      }))
    },
    onTurnStarted: (ev) => {
      if (!isCurrentStream()) return
      terminalEventBarrier = false
      const turnId = ev.turnId?.trim()
      if (!turnId) return
      invalidateBoundThreadDetail(ev.threadId)
      flushPendingStreamingDeltas()
      const threadId = ev.threadId?.trim() || traceThreadId()
      const previousTurnId = get().currentTurnId
      if (threadId && previousTurnId && isPendingActiveStreamTurnId(previousTurnId) && previousTurnId !== turnId) {
        migrateActiveStreamTurn(threadId, previousTurnId, turnId)
      } else {
        appendActiveStreamDeltas({
          threadId,
          turnId,
          lastSeq: ev.seq,
          startedAt: runtimeEventStartedAt(ev.createdAt)
        })
      }
      set((s) => {
        resetBusyRecoveryAttempts()
        const currentUserId = s.currentTurnUserId ?? findLatestUserBlockId(s.blocks)
        const startedAt = runtimeEventStartedAt(ev.createdAt)
        const nextBlocks = currentUserId
          ? s.blocks.map((block) => {
              if (block.kind !== 'user' || block.id !== currentUserId) return block
              const existingTurnId = block.meta?.turnId?.trim()
              if (existingTurnId && !isPendingActiveStreamTurnId(existingTurnId) && existingTurnId !== turnId) return block
              return {
                ...block,
                meta: {
                  ...(block.meta ?? {}),
                  turnId
                }
              }
            })
          : s.blocks
        armBusyWatchdog(set, get)
        return {
          blocks: nextBlocks,
          busy: true,
          currentTurnId: turnId,
          ...(currentUserId ? { currentTurnUserId: currentUserId } : {}),
          ...(typeof ev.seq === 'number' ? { lastSeq: Math.max(s.lastSeq, ev.seq) } : {}),
          ...(currentUserId
            ? {
                turnStartedAtByUserId: {
                  ...s.turnStartedAtByUserId,
                  [currentUserId]: s.turnStartedAtByUserId[currentUserId] ?? startedAt
                }
              }
            : {}),
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onTurnSteered: (ev) => {
      if (!isCurrentStream()) return
      const clientUserMessageId = ev.clientUserMessageId?.trim()
      if (!clientUserMessageId) return
      set((s) => ({
        blocks: s.blocks.map((block) =>
          block.kind === 'user' && block.meta?.clientUserMessageId === clientUserMessageId
            ? {
                ...block,
                meta: {
                  ...(block.meta ?? {}),
                  steeringStatus: 'admitted' as const,
                  ...(ev.admittedSeq !== undefined ? { admittedSeq: ev.admittedSeq } : {})
                }
              }
            : block
        )
      }))
    },
    onUserMessage: (ev) => {
      if (!isCurrentStream()) return
      invalidateBoundThreadDetail()
      flushPendingStreamingDeltas()
      set((s) => {
        if (!isCurrentStream()) return {}
        resetBusyRecoveryAttempts()
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        const optimisticCurrentUserId = s.currentTurnUserId
        const reconciledBlocks =
          optimisticCurrentUserId &&
          optimisticCurrentUserId !== ev.itemId &&
          baseBlocks.some((block) => block.kind === 'user' && block.id === optimisticCurrentUserId)
            ? reconcileOptimisticUserBlock(
                baseBlocks,
                optimisticCurrentUserId,
                ev.itemId,
                ev.text,
                ev.modelLabel
              )
            : baseBlocks
        const nextBlocks = upsertUserBlock(reconciledBlocks, ev)
        const startedAt = runtimeEventStartedAt(ev.createdAt)
        const previousTurnId = s.currentTurnId
        if (ev.turnId && previousTurnId && isPendingActiveStreamTurnId(previousTurnId)) {
          migrateActiveStreamTurn(traceThreadId(), previousTurnId, ev.turnId)
        }
        armBusyWatchdog(set, get)
        return {
          ...flushed,
          blocks: nextBlocks,
          busy: true,
          currentTurnId: ev.turnId ?? s.currentTurnId,
          currentTurnUserId: ev.itemId,
          turnStartedAtByUserId: {
            ...s.turnStartedAtByUserId,
            [ev.itemId]: s.turnStartedAtByUserId[ev.itemId] ?? startedAt
          },
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onDeltas: (rawDeltas) => {
      if (!isCurrentStream()) return
      if (terminalEventBarrier) return
      invalidateBoundThreadDetail()
      let batchMaxSeq = appliedDeltaSeqFloor
      for (const delta of rawDeltas) {
        if (typeof delta.seq === 'number') batchMaxSeq = Math.max(batchMaxSeq, delta.seq)
      }
      appliedDeltaSeqFloor = batchMaxSeq
      if (batchMaxSeq > get().lastSeq) {
        set((state) => ({ lastSeq: Math.max(state.lastSeq, batchMaxSeq) }))
      }
    },
    onTool: (ev) => {
      if (!isCurrentStream()) return
      const projectedEvent = projectToolEventForRenderer(ev)
      if (!projectedEvent) return
      ev = projectedEvent
      const artifact = ev.status === 'success' && ev.meta?.toolName === 'generate_office_document'
        ? generatedArtifactMetadataSchema.safeParse(ev.meta.generatedArtifact)
        : null
      const artifactScope = { threadId: get().activeThreadId ?? '', workspace: get().workspaceRoot }
      const shouldOpenArtifact = artifact?.success && artifactScope.threadId && artifactScope.workspace &&
        !get().blocks.some(block => block.kind === 'tool' && block.status === 'success' &&
          generatedArtifactMetadataSchema.safeParse(block.meta?.generatedArtifact).data?.artifactId === artifact.data.artifactId)
      invalidateBoundThreadDetail()
      flushPendingStreamingDeltas()
      notifyWriteWorkspaceFileRefresh(get, ev)
      set((s) => {
        resetBusyRecoveryAttempts()
        const eventTurnId = toolEventTurnId(ev, s.currentTurnId)
        // Restore busy state on tool events (same reasoning as onDelta).
        const base: Partial<ChatState> = {
          ...adoptTurnIdPatch(s, eventTurnId)
        }
        if (!s.busy) {
          base.busy = true
          armBusyWatchdog(set, get)
        }
        const idx = s.blocks.findIndex((b) => (
          b.kind === 'tool' && toolBlockMatchesEvent(b, ev, s.currentTurnId)
        ))
        if (idx >= 0) {
          const cur = s.blocks[idx]
          if (cur.kind !== 'tool') return { ...base }
          const next: ToolBlock = {
            ...cur,
            summary: ev.summary || cur.summary,
            status: ev.status,
            toolKind: ev.toolKind ?? cur.toolKind,
            detail: ev.detail ?? cur.detail,
            filePath: ev.filePath ?? cur.filePath,
            meta: mergeToolMeta(cur.meta, ev.meta, eventTurnId)
          }
          const blocks = [...s.blocks]
          blocks[idx] = next
          return {
            ...base,
            blocks,
            error: clearRuntimeStreamRecoveringError(s.error)
          }
        }
        // New tool — flush pending live reasoning/assistant first so each
        // reasoning segment becomes its own timeline block in chronological
        // order, rather than collapsing into one giant trailing block.
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        const blockId = baseBlocks.some((candidate) => candidate.id === ev.itemId)
          ? `${ev.itemId}:${eventTurnId || Date.now()}`
          : ev.itemId
        const block: ToolBlock = {
          kind: 'tool',
          id: blockId,
          createdAt: new Date().toISOString(),
          summary: ev.summary,
          status: ev.status,
          ...(ev.toolKind ? { toolKind: ev.toolKind } : {}),
          ...(ev.detail ? { detail: ev.detail } : {}),
          ...(ev.filePath ? { filePath: ev.filePath } : {}),
          meta: mergeToolMeta(undefined, ev.meta, eventTurnId)
        }
        return {
          ...base,
          ...flushed,
          blocks: [...baseBlocks, block],
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
      if (shouldOpenArtifact && artifact?.success) {
        // History hydration already contains the receipt and does not reopen
        // objects. A newly completed live result opens through the same Core
        // resolver as its card, with the original conversation scope retained.
        void import('../office/open-generated-artifact').then(({ openGeneratedArtifact }) => {
          if (isCurrentStream()) void openGeneratedArtifact(artifact.data.artifactId, artifactScope)
        })
      }
    },
    onCompaction: (ev) => {
      if (!isCurrentStream()) return
      flushPendingStreamingDeltas()
      set((s) => {
        resetBusyRecoveryAttempts()
        const base: Partial<ChatState> = {
          ...adoptTurnIdPatch(s, ev.turnId)
        }
        if (!s.busy && ev.status === 'running') {
          base.busy = true
          armBusyWatchdog(set, get)
        }
        const idx = s.blocks.findIndex((b) => b.kind === 'compaction' && b.id === ev.itemId)
        if (idx >= 0) {
          const cur = s.blocks[idx]
          if (cur.kind !== 'compaction') return { ...base }
          const next: CompactionBlock = {
            ...cur,
            summary: ev.summary || cur.summary,
            status: ev.status,
            detail: ev.detail ?? cur.detail,
            auto: ev.auto ?? cur.auto,
            messagesBefore: ev.messagesBefore ?? cur.messagesBefore,
            messagesAfter: ev.messagesAfter ?? cur.messagesAfter,
            createdAt: cur.createdAt ?? ev.createdAt,
            meta: {
              ...(cur.meta ?? {}),
              ...(ev.meta ?? {}),
              ...(ev.turnId ? { turnId: ev.turnId } : {})
            }
          }
          const blocks = [...s.blocks]
          blocks[idx] = next
          return {
            ...base,
            blocks,
            error: clearRuntimeStreamRecoveringError(s.error)
          }
        }
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        const block: CompactionBlock = {
          kind: 'compaction',
          id: ev.itemId,
          createdAt: ev.createdAt ?? new Date().toISOString(),
          summary: ev.summary,
          status: ev.status,
          detail: ev.detail,
          auto: ev.auto,
          messagesBefore: ev.messagesBefore,
          messagesAfter: ev.messagesAfter,
          meta: {
            ...(ev.meta ?? {}),
            ...(ev.turnId ? { turnId: ev.turnId } : {})
          }
        }
        return {
          ...base,
          ...flushed,
          blocks: [...baseBlocks, block],
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onReview: (ev: ReviewEventPayload) => {
      if (!isCurrentStream()) return
      flushPendingStreamingDeltas()
      set((s) => {
        resetBusyRecoveryAttempts()
        const base: Partial<ChatState> = {}
        if (!s.busy && ev.status === 'running') {
          base.busy = true
          armBusyWatchdog(set, get)
        }
        const idx = s.blocks.findIndex((b) => b.kind === 'review' && b.id === ev.itemId)
        if (idx >= 0) {
          const cur = s.blocks[idx]
          if (cur.kind !== 'review') return { ...base }
          const next: ReviewBlock = {
            ...cur,
            title: ev.title || cur.title,
            status: ev.status,
            target: ev.target ?? cur.target,
            reviewText: ev.reviewText ?? cur.reviewText,
            output: ev.output ?? cur.output,
            createdAt: cur.createdAt ?? ev.createdAt
          }
          const blocks = [...s.blocks]
          blocks[idx] = next
          return {
            ...base,
            blocks,
            error: clearRuntimeStreamRecoveringError(s.error)
          }
        }
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        const block: ReviewBlock = {
          kind: 'review',
          id: ev.itemId,
          createdAt: ev.createdAt ?? new Date().toISOString(),
          title: ev.title,
          status: ev.status,
          target: ev.target,
          reviewText: ev.reviewText,
          output: ev.output
        }
        return {
          ...base,
          ...flushed,
          blocks: [...baseBlocks, block],
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onApproval: (req) => {
      if (!isCurrentStream()) return
      flushPendingStreamingDeltas()
      set((s) => {
        if (!isCurrentStream()) return {}
        resetBusyRecoveryAttempts()
        if (s.blocks.some((b) => b.kind === 'approval' && b.approvalId === req.approvalId)) {
          return {}
        }
        const base: Partial<ChatState> = {
          ...adoptTurnIdPatch(s, req.turnId)
        }
        if (!s.busy && req.turnId) {
          base.busy = true
          armBusyWatchdog(set, get)
        }
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        return {
          ...base,
          ...flushed,
          blocks: [
            ...baseBlocks,
            {
              kind: 'approval',
              id: `approval-${req.approvalId}`,
              createdAt: new Date().toISOString(),
              approvalId: req.approvalId,
              summary: req.summary,
              toolName: req.toolName,
              status: 'pending' as const,
              meta: {
                ...(req.meta ?? {}),
                ...(req.turnId ? { turnId: req.turnId } : {})
              }
            }
          ],
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onApprovalStatus: (ev) => {
      if (!isCurrentStream()) return
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      set((s) => ({
        error: clearRuntimeStreamRecoveringError(s.error),
        blocks: s.blocks.map((b) =>
          b.kind === 'approval' && (b.approvalId === ev.approvalId || b.id === ev.itemId)
            ? {
                ...b,
                status: ev.status,
                errorMessage: ev.errorMessage ?? b.errorMessage
              }
            : b
        )
      }))
    },
    onUserInput: (req) => {
      if (!isCurrentStream()) return
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()
      set((s) => {
        if (s.blocks.some((b) => b.kind === 'user_input' && b.requestId === req.requestId)) {
          return {}
        }
        const base: Partial<ChatState> = {
          ...adoptTurnIdPatch(s, req.turnId)
        }
        if (!s.busy && req.turnId) {
          base.busy = true
        }
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        return {
          ...base,
          ...flushed,
          blocks: [
            ...baseBlocks,
            {
              kind: 'user_input',
              id: req.itemId,
              createdAt: new Date().toISOString(),
              requestId: req.requestId,
              questions: req.questions,
              status: 'pending' as const,
              meta: {
                ...(req.meta ?? {}),
                ...(req.turnId ? { turnId: req.turnId } : {})
              }
            }
          ],
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onUserInputStatus: (ev) => {
      if (!isCurrentStream()) return
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      if (ev.status === 'submitted' && get().busy) {
        armBusyWatchdog(set, get)
      }
      set((s) => ({
        error: clearRuntimeStreamRecoveringError(s.error),
        blocks: s.blocks.map((b) =>
          b.kind === 'user_input' && (b.id === ev.itemId || b.requestId === ev.itemId || b.requestId === ev.requestId)
            ? b.status === 'submitted' && ev.status === 'error' && isUserInputInterruptError(ev.errorMessage)
              ? b
              : {
                  ...b,
                  status: ev.status,
                  answers: ev.answers ?? b.answers,
                  errorMessage: ev.errorMessage ?? b.errorMessage
                }
            : b
        )
      }))
    },
	    onRuntimeStatus: (ev) => {
	      if (!isCurrentStream()) return
	      flushPendingStreamingDeltas()
	      const text = runtimeStatusText(ev)
	      const childOnlyStatus = Boolean(ev.child) && !text
	      set((s) => {
	        if (childOnlyStatus) {
	          return {
	            error: clearRuntimeStreamRecoveringError(s.error)
	          }
	        }
	        resetBusyRecoveryAttempts()
	        const base: Partial<ChatState> = {
	          ...adoptTurnIdPatch(s, ev.turnId)
	        }
        if (!s.busy) {
          base.busy = true
          armBusyWatchdog(set, get)
	        }
	        const flushed = flushLiveBlocksAndResetStream(s)
	        const baseBlocks = flushed.blocks ?? s.blocks
	        if (!text) {
	          return {
	            ...base,
            ...flushed,
            error: clearRuntimeStreamRecoveringError(s.error)
          }
        }
        const meta = runtimeStatusMeta(ev)
        const block: ChatBlock = {
          kind: 'system',
          id: ev.itemId,
          createdAt: ev.createdAt ?? new Date().toISOString(),
          text,
          ...(meta ? { meta } : {})
        }
        const idx = baseBlocks.findIndex((candidate) => candidate.kind === 'system' && candidate.id === ev.itemId)
        const blocks = [...baseBlocks]
        if (idx >= 0) blocks[idx] = block
        else blocks.push(block)
        return {
          ...base,
          ...flushed,
          blocks,
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onRuntimeError: (ev) => {
      if (!isCurrentStream()) return
      invalidateBoundThreadDetail()
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      set((s) => {
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        const view = describeRuntimeError(runtimeErrorPayloadToError(ev))
        const block: Extract<ChatBlock, { kind: 'system' }> = {
          kind: 'system',
          id: ev.itemId,
          createdAt: ev.createdAt ?? new Date().toISOString(),
          text: view.summary,
          ...(view.code ? { code: view.code } : {}),
          ...(view.detail ? { detail: view.detail } : {}),
          severity: ev.severity ?? 'error'
        }
        return {
          ...flushed,
          blocks: upsertRuntimeErrorBlock(baseBlocks, block),
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onThreadLifecycle: (ev) => {
      if (!ev.threadId) return
      invalidateThreadDetailCache(ev.threadId)
      flushPendingStreamingDeltas()
      set((s) => {
        const nextLastSeq = ev.seq !== undefined ? Math.max(s.lastSeq, ev.seq) : s.lastSeq
        return {
          ...(nextLastSeq !== s.lastSeq ? { lastSeq: nextLastSeq } : {}),
          threads: s.threads.map((thread) =>
            thread.id === ev.threadId
              ? {
                  ...thread,
                  ...(ev.title !== undefined ? { title: ev.title } : {}),
                  ...(ev.status ? { status: ev.status, archived: ev.status === 'archived' } : {}),
                  updatedAt: ev.createdAt ?? thread.updatedAt
                }
              : thread
          )
        }
      })
    },
    onThreadRewound: (ev) => {
      if (!isCurrentStream()) return
      invalidateBoundThreadDetail(ev.threadId)
      flushPendingStreamingDeltas()
      clearBusyWatchdog()
      set((s) => {
        const removedTurnIds = ev.removedTurnIds.length ? ev.removedTurnIds : [ev.turnId]
        const trimmed = trimBlocksForThreadRewind(s.blocks, removedTurnIds)
        if (trimmed.blocks === s.blocks) {
          return {
            ...(ev.seq !== undefined ? { lastSeq: Math.max(s.lastSeq, ev.seq) } : {})
          }
        }
        const turnStartedAtByUserId = { ...s.turnStartedAtByUserId }
        const turnDurationByUserId = { ...s.turnDurationByUserId }
        for (const id of trimmed.droppedUserIds) {
          delete turnStartedAtByUserId[id]
          delete turnDurationByUserId[id]
        }
        const currentTurnRemoved = s.currentTurnId ? removedTurnIds.includes(s.currentTurnId) : false
        return {
          blocks: trimmed.blocks,
          liveAssistant: '',
          busy: currentTurnRemoved ? false : s.busy,
          currentTurnId: currentTurnRemoved ? null : s.currentTurnId,
          currentTurnUserId: currentTurnRemoved ? null : s.currentTurnUserId,
          turnStartedAtByUserId,
          turnDurationByUserId,
          queuedMessages: currentTurnRemoved ? [] : s.queuedMessages,
          ...(ev.seq !== undefined ? { lastSeq: Math.max(s.lastSeq, ev.seq) } : {}),
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onGoal: (ev) => {
      if (!isCurrentStream()) return
      if (!ev.threadId) return
      invalidateThreadDetailCache(ev.threadId)
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      set((s) => {
        const currentThread = s.activeThreadId === ev.threadId
        const updatedAt = ev.goal?.updatedAt ?? ev.createdAt ?? new Date().toISOString()
        const nextThreads = s.threads.map((thread) =>
          thread.id === ev.threadId
            ? {
                ...thread,
                goal: ev.goal,
                updatedAt
              }
            : thread
        )
        if (!currentThread) {
          return { threads: nextThreads }
        }
        const flushed = flushLiveBlocksAndResetStream(s)
        const baseBlocks = flushed.blocks ?? s.blocks
        const block: ChatBlock = {
          kind: 'system',
          id: `goal-${ev.threadId}-${updatedAt}-${ev.goal?.status ?? 'cleared'}`,
          createdAt: updatedAt,
          text: goalTimelineText(ev.goal, ev.cleared)
        }
        return {
          ...flushed,
          activeThreadGoal: ev.goal,
          threads: nextThreads,
          blocks: [...baseBlocks, block],
          error: clearRuntimeStreamRecoveringError(s.error)
        }
      })
    },
    onTodos: (ev) => {
      if (!isCurrentStream()) return
      if (!ev.threadId) return
      invalidateThreadDetailCache(ev.threadId)
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      set((s) => {
        const currentThread = s.activeThreadId === ev.threadId
        const todos = ev.cleared ? null : ev.todos
        const updatedAt = todos?.updatedAt ?? ev.createdAt ?? new Date().toISOString()
        const nextThreads = s.threads.map((thread) =>
          thread.id === ev.threadId
            ? {
                ...thread,
                todos,
                updatedAt
              }
            : thread
        )
        return currentThread
          ? {
              activeThreadTodos: todos,
              threads: nextThreads,
              error: clearRuntimeStreamRecoveringError(s.error)
            }
          : { threads: nextThreads }
      })
    },
    onSnapshotRequired: async (ev) => {
      if (!isCurrentStream()) return
      invalidateBoundThreadDetail(ev.threadId)
      flushPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      const cursorSeq = Math.max(ev.seq ?? 0, ev.highestSeq ?? 0)
      if (cursorSeq > appliedDeltaSeqFloor) appliedDeltaSeqFloor = cursorSeq
      await reconcileThreadSnapshotFromDetail({
        event: ev,
        fallbackThreadId: boundThreadId || get().activeThreadId,
        loadThreadDetail,
        set,
        get
      })
    },
    onPublicProjectionRevoked: async (ev) => {
      const revokedThreadId = ev.threadId
      if (boundThreadId && revokedThreadId !== boundThreadId) return
      markPublicProjectionRevoked(revokedThreadId)
      terminalEventBarrier = true
      discardPendingStreamingDeltas()
      legacyLiveAssistant.reset()
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()

      const currentState = get()
      const relatedThreadIds = new Set<string>([revokedThreadId])
      for (const thread of currentState.threads) {
        if (threadReferencesRevokedProjection(thread, revokedThreadId)) {
          relatedThreadIds.add(thread.id)
        }
      }
      for (const conversation of Object.values(currentState.sideConversations)) {
        if (
          conversation.threadId === revokedThreadId ||
          conversation.parentThreadId === revokedThreadId ||
          relatedThreadIds.has(conversation.parentThreadId)
        ) {
          relatedThreadIds.add(conversation.threadId)
        }
      }
      for (const threadId of relatedThreadIds) {
        markPublicProjectionRevoked(threadId)
        invalidateThreadDetailCache(threadId)
        invalidateSharedThreadSummary(threadId)
        clearActiveStream(threadId)
        clearWatchedCompletionNotification(threadId)
        clearPendingClawFeishuMirrorsForThread(threadId)
        void purgeSddChatTranscriptForThread(threadId).catch(() => undefined)
      }

      set((state) => {
        const activeRevoked = Boolean(
          state.activeThreadId && relatedThreadIds.has(state.activeThreadId)
        )
        const nextWatch = { ...state.watchTurnCompletion }
        const nextUnread = { ...state.unreadThreadIds }
        for (const threadId of relatedThreadIds) {
          delete nextWatch[threadId]
          delete nextUnread[threadId]
        }
        const nextCaseProjectThreadsById = Object.fromEntries(
          Object.entries(state.caseProjectThreadsById).map(([caseProjectId, threads]) => [
            caseProjectId,
            threads.filter((thread) => (
              !threadReferencesRevokedProjection(thread, revokedThreadId)
            ))
          ])
        )
        const nextSideConversations = Object.fromEntries(
          Object.entries(state.sideConversations).filter(([, conversation]) => (
            !relatedThreadIds.has(conversation.threadId) &&
            !relatedThreadIds.has(conversation.parentThreadId)
          ))
        )
        const nextHandoffs = state.threadHandoffOperations.filter((operation) => (
          !relatedThreadIds.has(operation.sourceThreadId) &&
          !(operation.targetThreadId && relatedThreadIds.has(operation.targetThreadId))
        ))
        const activeHandoffRetained = Boolean(
          state.activeThreadHandoffOperationId &&
          nextHandoffs.some((operation) => operation.id === state.activeThreadHandoffOperationId)
        )
        return {
          threads: state.threads.filter((thread) => (
            !threadReferencesRevokedProjection(thread, revokedThreadId)
          )),
          caseProjectThreadsById: nextCaseProjectThreadsById,
          sideConversations: nextSideConversations,
          sidePanel: state.sidePanel.activeSideId && relatedThreadIds.has(state.sidePanel.activeSideId)
            ? { ...state.sidePanel, activeSideId: null }
            : state.sidePanel,
          threadHandoffOperations: nextHandoffs,
          activeThreadHandoffOperationId: activeHandoffRetained
            ? state.activeThreadHandoffOperationId
            : null,
          watchTurnCompletion: nextWatch,
          unreadThreadIds: nextUnread,
          lastTurnUsage: state.lastTurnUsage && relatedThreadIds.has(state.lastTurnUsage.threadId)
            ? null
            : state.lastTurnUsage,
          ...(activeRevoked
            ? {
                ...clearedThreadSelection(),
                topNotice: null,
                runtimeErrorDetail: null,
                error: i18n.t(RUNTIME_FINAL_SNAPSHOT_UNAVAILABLE_KEY)
              }
            : {})
        }
      })
    },
    onGeneralTerminalBatch: async (batch: GeneralTerminalProjectionBatch) => {
      if (!isCurrentStream()) {
        throw new Error('general terminal delivery belongs to an inactive stream')
      }
      const reject = (message: string): never => {
        const before = get()
        if (before.activeThreadId === batch.threadId && before.currentTurnId?.trim() === batch.turnId) {
          terminalEventBarrier = true
          invalidateBoundThreadDetail(batch.threadId)
          discardPendingStreamingDeltas()
          resetBusyRecoveryAttempts()
          clearBusyWatchdog()
          clearActiveStream(batch.threadId)
          clearWatchedCompletionNotification(batch.threadId)
          takePendingClawFeishuMirror(batch.turnId)
          set((state) => {
            if (state.activeThreadId !== batch.threadId || state.currentTurnId?.trim() !== batch.turnId) return {}
            const watchTurnCompletion = { ...state.watchTurnCompletion }
            delete watchTurnCompletion[batch.threadId]
            return {
              blocks: settlePendingRuntimeWorkAfterInterrupt(stripProvisionalAssistantForTurn(
                state.blocks,
                batch.turnId,
                state.currentTurnUserId
              )),
              liveAssistant: '',
              busy: false,
              currentTurnId: null,
              currentTurnUserId: null,
              queuedMessagesPausedReason: 'failed',
              watchTurnCompletion,
              error: i18n.t(RUNTIME_FINAL_SNAPSHOT_UNAVAILABLE_KEY),
              runtimeErrorDetail: null
            }
          })
        }
        throw new Error(message)
      }
      const before = get()
      if (!/^[a-f0-9]{64}$/.test(batch.batchDigest) || batch.firstSeq <= 0 ||
          batch.lastSeq < batch.firstSeq || before.activeThreadId !== batch.threadId ||
          before.currentTurnId?.trim() !== batch.turnId || before.lastSeq !== batch.firstSeq - 1 ||
          (batch.terminalItem?.kind === 'assistant' && batch.terminal.status !== 'completed') ||
          (batch.terminalItem?.kind === 'system' && batch.terminal.status === 'completed')) {
        return reject('general terminal delivery could not be committed atomically')
      }

      terminalEventBarrier = true
      invalidateBoundThreadDetail(batch.threadId)
      discardPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()

      let committed = false
      set((state) => {
        if (state.activeThreadId !== batch.threadId || state.currentTurnId?.trim() !== batch.turnId ||
            state.lastSeq !== batch.firstSeq - 1) return {}
        const expectedUserBlockId = state.currentTurnUserId
        const withoutProvisional = stripProvisionalAssistantForTurn(
          state.blocks,
          batch.turnId,
          expectedUserBlockId
        )
        const withoutDuplicateTerminal = batch.terminalItem
          ? withoutProvisional.filter((block) => block.id !== batch.terminalItem?.id)
          : withoutProvisional
        const blocks: ChatBlock[] = [
          ...settlePendingRuntimeWorkAfterInterrupt(withoutDuplicateTerminal),
          ...(batch.terminalItem ? [batch.terminalItem] : [])
        ]
        const timing = finalizeTurnTiming({
          ...state,
          currentTurnId: batch.turnId,
          currentTurnUserId: expectedUserBlockId
        })
        const watchTurnCompletion = { ...state.watchTurnCompletion }
        delete watchTurnCompletion[batch.threadId]
        const unreadThreadIds = { ...state.unreadThreadIds }
        delete unreadThreadIds[batch.threadId]
        committed = true
        return {
          ...timing,
          blocks,
          liveAssistant: '',
          lastSeq: batch.lastSeq,
          usageRefreshKey: state.usageRefreshKey + 1,
          lastTurnUsage: { threadId: batch.threadId, snapshot: batch.usage },
          busy: false,
          currentTurnId: null,
          currentTurnUserId: null,
          error: null,
          runtimeErrorDetail: null,
          queuedMessagesPausedReason: batch.terminal.status === 'failed' ? 'failed' : null,
          watchTurnCompletion,
          unreadThreadIds
        }
      })
      if (!committed) return reject('general terminal delivery could not be committed atomically')

      clearActiveStream(batch.threadId)
      clearWatchedCompletionNotification(batch.threadId)
      const settledState = get()
      const pendingMirror = takePendingClawFeishuMirror(batch.turnId)
      const assistantMirrorText = pendingMirror
        ? collectAssistantTextForTurn(settledState.blocks, pendingMirror.userBlockId, '')
        : ''
      if (pendingMirror && assistantMirrorText &&
          typeof window.analytix?.connectPhone?.mirrorChannelMessage === 'function') {
        void window.analytix.connectPhone.mirrorChannelMessage(
          pendingMirror.threadId,
          assistantMirrorText,
          'assistant'
        ).catch(() => undefined)
      }
      notifyTurnComplete(
        batch.threadId,
        settledState,
        `general-terminal-batch:${batch.batchDigest}`
      )
      notifyWriteWorkspaceFileRefresh(get)
      notifySddChatTranscriptMirror(get)
      syncTurnCompletionPoll(set, get)
      void get().refreshThreads()
      if (get().queuedMessages.length === 0) releaseThreadWorktreeIfNeeded(batch.threadId)
      void get().drainQueuedMessages()
    },
    onAcceptedFinalBatch: async (batch: AcceptedFinalProjectionBatch) => {
      if (!isCurrentStream()) {
        throw new Error('accepted-final delivery belongs to an inactive stream')
      }
      const before = get()
      const quarantineRejectedProjection = (): void => {
        if (before.activeThreadId !== batch.threadId) return
        const currentTurnMatches = before.currentTurnId?.trim() === batch.turnId
        const hasCandidate = acceptedFinalProjectionHasCandidateReceipt(before.blocks, batch)
        if (!currentTurnMatches && !hasCandidate) return
        terminalEventBarrier = true
        invalidateBoundThreadDetail(batch.threadId)
        discardPendingStreamingDeltas()
        resetBusyRecoveryAttempts()
        clearBusyWatchdog()
        clearActiveStream(batch.threadId)
        clearWatchedCompletionNotification(batch.threadId)
        takePendingClawFeishuMirror(batch.turnId)
        set((state) => {
          if (state.activeThreadId !== batch.threadId) return {}
          const stateCurrentTurnMatches = state.currentTurnId?.trim() === batch.turnId
          const stateHasCandidate = acceptedFinalProjectionHasCandidateReceipt(state.blocks, batch)
          if (!stateCurrentTurnMatches && !stateHasCandidate) return {}
          const withoutProvisional = stripProvisionalAssistantForTurn(
            state.blocks,
            batch.turnId,
            state.currentTurnUserId
          ).filter((block) =>
            block.id !== batch.assistant.id && block.id !== batch.terminalError?.id &&
            !(block.kind === 'assistant' && acceptedFinalProjectionReceiptsEqual(
              block.acceptedFinalProjectionReceipt,
              batch.receipt
            ))
          )
          const watchTurnCompletion = { ...state.watchTurnCompletion }
          delete watchTurnCompletion[batch.threadId]
          return {
            blocks: settlePendingRuntimeWorkAfterInterrupt(withoutProvisional),
            liveAssistant: '',
            busy: false,
            currentTurnId: stateCurrentTurnMatches ? null : state.currentTurnId,
            currentTurnUserId: stateCurrentTurnMatches ? null : state.currentTurnUserId,
            lastTurnUsage: stateHasCandidate ? null : state.lastTurnUsage,
            queuedMessagesPausedReason: 'failed',
            watchTurnCompletion,
            error: i18n.t(RUNTIME_FINAL_SNAPSHOT_UNAVAILABLE_KEY),
            runtimeErrorDetail: null
          }
        })
      }
      const reject = (message: string): never => {
        quarantineRejectedProjection()
        throw new Error(message)
      }
      if (!acceptedFinalProjectionBatchIsSelfConsistent(batch)) {
        return reject('accepted-final delivery binding is invalid')
      }
      if (before.activeThreadId !== batch.threadId) {
        return reject('accepted-final delivery could not be committed atomically')
      }
      if (acceptedFinalProjectionIsAlreadyCommitted(before.blocks, before.lastSeq, batch)) {
        const terminalStateComplete = before.busy === false && before.liveAssistant === '' &&
          before.currentTurnId === null && before.currentTurnUserId === null &&
          before.lastTurnUsage?.threadId === batch.threadId &&
          acceptedFinalUsageSnapshotsEqual(before.lastTurnUsage.snapshot, batch.usage) &&
          before.queuedMessagesPausedReason ===
            (batch.terminal.status === 'failed' ? 'failed' : null)
        if (!terminalStateComplete) {
          return reject('accepted-final delivery could not be committed atomically')
        }
        terminalEventBarrier = true
        invalidateBoundThreadDetail(batch.threadId)
        discardPendingStreamingDeltas()
        resetBusyRecoveryAttempts()
        clearBusyWatchdog()
        clearActiveStream(batch.threadId)
        clearWatchedCompletionNotification(batch.threadId)
        return batch.receipt
      }
      if (acceptedFinalProjectionHasCandidateReceipt(before.blocks, batch) ||
          before.lastSeq !== batch.firstSeq - 1 ||
          before.currentTurnId?.trim() !== batch.turnId) {
        return reject('accepted-final delivery could not be committed atomically')
      }

      terminalEventBarrier = true
      invalidateBoundThreadDetail(batch.threadId)
      discardPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()

      let committed = false
      set((state) => {
        if (state.activeThreadId !== batch.threadId ||
            state.currentTurnId?.trim() !== batch.turnId ||
            state.lastSeq !== batch.firstSeq - 1 ||
            acceptedFinalProjectionIsAlreadyCommitted(state.blocks, state.lastSeq, batch) ||
            acceptedFinalProjectionHasCandidateReceipt(state.blocks, batch)) return {}
        const expectedUserBlockId = state.currentTurnUserId
        const withoutProvisional = stripProvisionalAssistantForTurn(
          state.blocks,
          batch.turnId,
          expectedUserBlockId
        )
        const withoutDuplicateTerminal = withoutProvisional.filter((block) =>
          block.id !== batch.assistant.id && block.id !== batch.terminalError?.id
        )
        const settledBlocks = settlePendingRuntimeWorkAfterInterrupt(withoutDuplicateTerminal)
        const assistant = {
          ...batch.assistant,
          acceptedFinalProjectionReceipt: batch.receipt,
          acceptedFinalProjectionTerminal: batch.terminal
        }
        const blocks: ChatBlock[] = [
          ...settledBlocks,
          assistant,
          ...(batch.terminalError ? [batch.terminalError] : [])
        ]
        const timing = finalizeTurnTiming({
          ...state,
          currentTurnId: batch.turnId,
          currentTurnUserId: expectedUserBlockId
        })
        const watchTurnCompletion = { ...state.watchTurnCompletion }
        delete watchTurnCompletion[batch.threadId]
        const unreadThreadIds = { ...state.unreadThreadIds }
        delete unreadThreadIds[batch.threadId]
        committed = true
        return {
          ...timing,
          blocks,
          liveAssistant: '',
          lastSeq: batch.lastSeq,
          usageRefreshKey: state.usageRefreshKey + 1,
          lastTurnUsage: { threadId: batch.threadId, snapshot: batch.usage },
          busy: false,
          currentTurnId: null,
          currentTurnUserId: null,
          error: null,
          runtimeErrorDetail: null,
          queuedMessagesPausedReason: batch.terminal.status === 'failed' ? 'failed' : null,
          watchTurnCompletion,
          unreadThreadIds
        }
      })
      if (!committed) {
        return reject('accepted-final delivery could not be committed atomically')
      }

      clearActiveStream(batch.threadId)
      clearWatchedCompletionNotification(batch.threadId)
      const settledState = get()
      const pendingMirror = takePendingClawFeishuMirror(batch.turnId)
      const assistantMirrorText = pendingMirror
        ? collectAssistantTextForTurn(settledState.blocks, pendingMirror.userBlockId, '')
        : ''
      if (pendingMirror && assistantMirrorText &&
          typeof window.analytix?.connectPhone?.mirrorChannelMessage === 'function') {
        void window.analytix.connectPhone.mirrorChannelMessage(
          pendingMirror.threadId,
          assistantMirrorText,
          'assistant'
        ).catch(() => undefined)
      }
      notifyTurnComplete(
        batch.threadId,
        settledState,
        `accepted-final-batch:${batch.batchId}`
      )
      notifyWriteWorkspaceFileRefresh(get)
      notifySddChatTranscriptMirror(get)
      syncTurnCompletionPoll(set, get)
      void get().refreshThreads()
      if (get().queuedMessages.length === 0) releaseThreadWorktreeIfNeeded(batch.threadId)
      void get().drainQueuedMessages()
      return batch.receipt
    },
    onTurnComplete: async (ev?: TurnCompletedEventPayload) => {
      if (!isCurrentStream()) return
      terminalEventBarrier = true
      invalidateBoundThreadDetail()
      discardPendingStreamingDeltas()
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()
      const eventThreadId = ev?.threadId?.trim() || null
      const eventTurnId = ev?.turnId?.trim() || null
      const eventSeq = typeof ev?.seq === 'number' ? ev.seq : undefined
      const currentState = get()
      if (eventThreadId && currentState.activeThreadId && eventThreadId !== currentState.activeThreadId) {
        if (eventSeq !== undefined) {
          set((s) => ({ lastSeq: Math.max(s.lastSeq, eventSeq) }))
        }
        terminalEventBarrier = false
        return
      }
      const currentTurnId = currentState.currentTurnId?.trim() || null
      if (
        eventTurnId &&
        currentTurnId &&
        !isPendingActiveStreamTurnId(currentTurnId) &&
        currentTurnId !== eventTurnId
      ) {
        if (eventSeq !== undefined) {
          set((s) => ({ lastSeq: Math.max(s.lastSeq, eventSeq) }))
        }
        terminalEventBarrier = false
        return
      }
      const completedThreadId = eventThreadId || currentState.activeThreadId
      const completedTurnId = eventTurnId || currentTurnId
      if (eventTurnId && currentTurnId && isPendingActiveStreamTurnId(currentTurnId)) {
        migrateActiveStreamTurn(completedThreadId, currentTurnId, eventTurnId)
      }
      const completedUserBlockId = currentState.currentTurnUserId
      const completedKey = completedTurnId
        ? `turn:${completedTurnId}`
        : `active:${completedThreadId ?? 'unknown'}:${currentState.lastSeq}`
      const reconciled = await reconcileTerminalTurnFromThreadDetail({
        threadId: completedThreadId,
        turnId: completedTurnId,
        userBlockId: completedUserBlockId,
        eventSeq,
        terminalStatus: 'completed',
        acceptedFinalDigest: ev?.acceptedFinalDigest,
        loadThreadDetail,
        set,
        get
      })
      if (!reconciled) return

      const settledState = get()
      const pendingMirror = takePendingClawFeishuMirror(completedTurnId)
      const assistantMirrorText =
        pendingMirror
          ? collectAssistantTextForTurn(
              settledState.blocks,
              pendingMirror.userBlockId,
              ''
            )
          : ''
      if (pendingMirror && assistantMirrorText && typeof window.analytix?.connectPhone?.mirrorChannelMessage === 'function') {
        void window.analytix.connectPhone.mirrorChannelMessage(
          pendingMirror.threadId,
          assistantMirrorText,
          'assistant'
        ).catch(() => undefined)
      }
      notifyTurnComplete(completedThreadId, settledState, completedKey)
      notifyWriteWorkspaceFileRefresh(get)
      notifySddChatTranscriptMirror(get)
      syncTurnCompletionPoll(set, get)
      void get().refreshThreads()
      if (get().queuedMessages.length === 0) {
        releaseThreadWorktreeIfNeeded(completedThreadId)
      }
      void get().drainQueuedMessages()
    },
    onError: async (err, options) => {
      if (!isCurrentStream()) return
      invalidateBoundThreadDetail()
      resetBusyRecoveryAttempts()
      clearBusyWatchdog()
      const state = get()
      const message = formatRuntimeError(err)
      const detail = runtimeErrorDetail(err)
      const terminal = options?.terminal === true
      const interrupted = isInterruptSettledError(err, message)
      if (!terminal) {
        set(() => ({
          error: message,
          runtimeErrorDetail: detail || null
        }))
        if (get().busy) armBusyWatchdog(set, get)
        return
      }

      terminalEventBarrier = true
      discardPendingStreamingDeltas()
      const eventThreadId = options?.threadId?.trim() || state.activeThreadId
      const eventTurnId = options?.turnId?.trim() || state.currentTurnId
      if (eventThreadId && state.activeThreadId && eventThreadId !== state.activeThreadId) {
        if (options?.seq !== undefined) {
          set((current) => ({ lastSeq: Math.max(current.lastSeq, options.seq ?? 0) }))
        }
        terminalEventBarrier = false
        return
      }
      if (!stateTurnMatchesExpected(state.currentTurnId, eventTurnId)) {
        if (options?.seq !== undefined) {
          set((current) => ({ lastSeq: Math.max(current.lastSeq, options.seq ?? 0) }))
        }
        terminalEventBarrier = false
        return
      }
      const reconciled = await reconcileTerminalTurnFromThreadDetail({
        threadId: eventThreadId,
        turnId: eventTurnId,
        userBlockId: state.currentTurnUserId,
        eventSeq: options?.seq,
        terminalStatus: options?.status ?? (interrupted ? 'aborted' : 'failed'),
        acceptedFinalDigest: options?.acceptedFinalDigest,
        terminalError: interrupted ? null : message,
        terminalErrorDetail: interrupted ? null : detail || null,
        loadThreadDetail,
        set,
        get
      })
      if (!reconciled) return
      takePendingClawFeishuMirror(eventTurnId)
      syncTurnCompletionPoll(set, get)
      void get().refreshThreads?.()
      if (get().queuedMessages.length === 0) {
        releaseThreadWorktreeIfNeeded(eventThreadId)
      }
      void get().drainQueuedMessages?.()
    },
    onUsage: (usage) => {
      if (!isCurrentStream()) return
      set((s) => ({
        usageRefreshKey: s.usageRefreshKey + 1,
        lastTurnUsage: { threadId: s.activeThreadId ?? '', snapshot: usage }
      }))
    }
  }
}
