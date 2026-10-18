import subagentCss from './presentation/chat/toolviews/subagent-row.module.css'
import type { FormEvent, ReactElement } from 'react'
import { useEffect, useRef, useState } from 'react'
import type { ChildInputSubmitRequestV1 } from '@shared/child-input-contract'
import { useTranslation } from 'react-i18next'
import { Ban, Check, Diff, ExternalLink, ListTodo, Loader2, Pause, Play, RotateCcw, SendHorizontal, StopIcon as Square, Trash2 } from '../../design/AnalytixUiIcons'
import type { ChatBlock, RuntimeChildMetadata, RuntimeJobDiagnosticsMetadata } from '../../agent/types'
import { ChildInputRejectedError, rendererRuntimeClient } from '../../agent/runtime-client'
import { useChatStore } from '../../store/chat-store'
import { formatDuration, formatJobHeartbeatStatus } from './message-timeline-tools'
import { nativeSubagentCardModel, subagentPublicSummary } from './presentation/chat/models/subagent-card-model'
import { ToolDetails } from './presentation/chat/toolviews/ToolDetails'

type CardStatus = 'queued' | 'running' | 'paused' | 'done' | 'failed' | 'closed'

function blockMeta(block: ChatBlock): Record<string, unknown> | undefined {
  return block.kind === 'tool' || block.kind === 'approval' || block.kind === 'user' || block.kind === 'system'
    ? block.meta
    : undefined
}

export function childMetaFromBlock(block: ChatBlock): RuntimeChildMetadata | null {
  const child = blockMeta(block)?.child
  return child && typeof child === 'object' && !Array.isArray(child)
    ? child as RuntimeChildMetadata
    : null
}

function diagnosticsFromBlock(block: ChatBlock): RuntimeJobDiagnosticsMetadata | null {
  const diagnostics = blockMeta(block)?.diagnostics
  if (!diagnostics || typeof diagnostics !== 'object' || Array.isArray(diagnostics)) return null
  const raw = diagnostics as Record<string, unknown>
  const safe: Record<string, unknown> = {}
  for (const key of ['jobId', 'childRunId', 'childThreadId', 'childTurnId', 'parentThreadId', 'parentTurnId', 'autoContinueTurnId', 'deliveryId', 'deliveryItemId', 'pauseRequestId'] as const) {
    const value = raw[key]
    if (typeof value === 'string' && value.trim()) safe[key] = value.trim()
  }
  for (const [key, allowed] of [
    ['notificationKind', ['background_job_completion', 'background_job_auto_continue', 'background_job_delivery', 'thread_summary_subagent', 'thread_summary_task']],
    ['kind', ['task', 'parallel_task', 'planner', 'background-shell', 'bash', 'subagent', 'child-run', 'parallel-child-run', 'command', 'process', 'unknown']],
    ['status', ['queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming', 'completed', 'failed', 'aborted', 'interrupted', 'killed', 'canceled', 'timeout', 'starting', 'pending', 'delivered', 'retry', 'recovering', 'recovered', 'dead_letter', 'dead_lettered', 'skipped', 'stopped', 'missing', 'archived', 'unknown']],
    ['heartbeatStatus', ['running', 'healthy', 'stale', 'stalled', 'lease_expired', 'paused', 'completed', 'recovered', 'unknown']],
    ['autoContinueStatus', ['pending', 'started', 'skipped', 'failed', 'completed']],
    ['deliveryStatus', ['pending', 'retry', 'delivered', 'dead_letter', 'failed']],
    ['recoveryStatus', ['pending', 'recovering', 'recovered', 'failed', 'dead_lettered']],
    ['pauseStatus', ['pause_requested', 'paused', 'resume_requested', 'resuming', 'running', 'failed']],
    ['warningCode', ['task_job_stalled', 'task_job_stale', 'slow-output']]
  ] as const) {
    const value = raw[key]
    if (typeof value === 'string' && allowed.includes(value.trim() as never)) safe[key] = value.trim()
  }
  for (const key of ['ageMs', 'idleMs', 'heartbeatAgeMs', 'staleAfterMs', 'stalledAfterMs', 'recoveryAttempt', 'completionDeliveryAttempt'] as const) {
    const value = raw[key]
    if (typeof value === 'number' && Number.isFinite(value)) safe[key] = value
  }
  for (const key of ['terminal', 'background', 'canContinueParent', 'lateCompletionSuppressed', 'autoContinueParent', 'stalled', 'leaseExpired', 'orphaned', 'paused'] as const) {
    const value = raw[key]
    if (typeof value === 'boolean') safe[key] = value
  }
  return Object.keys(safe).length > 0 ? safe as RuntimeJobDiagnosticsMetadata : null
}

function toolNameFromBlock(block: ChatBlock): string {
  return block.kind === 'tool' && typeof block.meta?.toolName === 'string'
    ? block.meta.toolName.trim()
    : ''
}

function isAutoContinueProcessBlock(block: ChatBlock): boolean {
  return diagnosticsFromBlock(block)?.notificationKind === 'background_job_auto_continue'
}

export function isBackgroundShellProcessBlock(block: ChatBlock): boolean {
  const child = childMetaFromBlock(block)
  return Boolean(child && (child.kind === 'background-shell' || (child.background && child.childName === 'bash')))
}

export function isSubagentProcessBlock(block: ChatBlock): boolean {
  if (block.kind !== 'tool') return false
  if (isAutoContinueProcessBlock(block)) return false
  if (isBackgroundShellProcessBlock(block)) return false
  if (block.toolKind === 'subagent') return true
  const child = childMetaFromBlock(block)
  if (child) return true
  const toolName = toolNameFromBlock(block)
  return toolName === 'delegate_task' || toolName === 'task' || toolName === 'parallel_tasks'
}

function parentTurnId(block: ChatBlock): string {
  return childMetaFromBlock(block)?.parentTurnId ?? ''
}

export function shouldMergeProcessChildBlocks(left: ChatBlock, right: ChatBlock): boolean {
  return parentTurnId(left) === parentTurnId(right)
}

export function childProcessGroupKey(block: ChatBlock): string {
  const child = childMetaFromBlock(block)
  if (!child) return ''
  return [
    child.parentThreadId,
    child.parentTurnId,
    child.parallelGroupId ?? ''
  ].filter(Boolean).join('::')
}

export function childProcessIdentityKey(block: ChatBlock): string {
  const child = childMetaFromBlock(block)
  if (!child) return block.id
  return child.childRunId || child.childId || child.parentToolCallId || block.id
}

function resolveStatus(block: ChatBlock, child: RuntimeChildMetadata | null): CardStatus {
  const childStatus = child?.childStatus
  if (childStatus === 'queued') return 'queued'
  if (childStatus === 'running') return 'running'
  if (childStatus === 'paused') return 'paused'
  if (childStatus === 'pause_requested' || childStatus === 'resume_requested' || childStatus === 'resuming') return 'running'
  if (childStatus === 'completed') return 'done'
  if (childStatus === 'failed') {
    return 'failed'
  }
  if (childStatus === 'aborted' || childStatus === 'interrupted' || childStatus === 'killed') return 'closed'
  const blockStatus = 'status' in block && typeof block.status === 'string' ? block.status : undefined
  if (blockStatus === 'running') return 'running'
  if (blockStatus === 'error') return 'failed'
  if (blockStatus === 'success') return 'done'
  return 'running'
}

function statusLabel(status: CardStatus, t: Translator): string {
  switch (status) {
    case 'queued':
      return t('subagentStatusQueued', { defaultValue: 'Queued' })
    case 'running':
      return t('subagentStatusRunning', { defaultValue: 'Running' })
    case 'paused':
      return t('subagentStatusPaused', { defaultValue: 'Paused' })
    case 'done':
      return t('subagentStatusDone', { defaultValue: 'Done' })
    case 'failed':
      return t('subagentStatusFailed', { defaultValue: 'Failed' })
    case 'closed':
      return t('subagentStatusTerminal', { defaultValue: 'Ended' })
    default:
      return ''
  }
}

type Translator = (key: string, opts?: Record<string, unknown>) => string
type SteerSubmitStatus = 'idle' | 'sending' | 'queued' | 'admitted' | 'rejected' | 'error'
type PauseControlStatus = 'idle' | 'sending' | 'requested' | 'resumed' | 'killed' | 'rejected' | 'error'
type JobControlStatus = 'idle' | 'sending' | 'waiting' | 'restarted' | 'killed' | 'rejected' | 'error'
type IsolationReviewStatus = 'idle' | 'sending' | 'reviewed' | 'rejected' | 'error'
type IsolationDecisionStatus = 'idle' | 'sending' | 'accepted' | 'rejected' | 'cleaned' | 'error'
type IsolationRepairStatus = 'idle' | 'sending' | 'reported' | 'checked' | 'accepted' | 'rejected' | 'error'
type ChildTodoProjectionStatus = 'idle' | 'sending' | 'accepted' | 'rejected' | 'error'

const MAX_STEER_MESSAGE_CHARS = 4000

function compactNumber(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`
  return String(value)
}

function childLineMeta(child: RuntimeChildMetadata | null, t: Translator): string {
  const toolPolicy = child?.childToolPolicy
  const toolInvocations = child?.toolInvocations
  const durationMs = child?.durationMs
  const queuedMs = child?.queuedMs
  const tokens = child?.totalTokens
  const evidence = child?.evidenceBundleStatus
    ? `evidence ${child.evidenceBundleStatus}${child.evidenceCount !== undefined ? ` (${child.evidenceCount})` : ''}`
    : ''
  const execution = [
    child?.childProviderId,
    child?.childModel,
    child?.childModelVariant,
    child?.childEndpointFormat,
    child?.childModelSource
  ].filter(Boolean).join(' / ')
  const cacheHitRate = child?.cacheHitRate
  const changedFileCount = child?.changedFileCount ?? child?.changedFiles?.length
  const heartbeatStatus = formatJobHeartbeatStatus(child?.heartbeatStatus, t)
  const meta = [
    child?.jobId ? `job ${child.jobId}` : '',
    heartbeatStatus ? `${t('toolJobHeartbeatStatus', { defaultValue: 'status' })} ${heartbeatStatus}` : '',
    child?.childSeq !== undefined ? `#${child.childSeq}` : '',
    child?.parallelIndex !== undefined ? `${t('toolChildParallel')} ${child.parallelIndex}` : '',
    child?.childProfileMode ? `mode ${child.childProfileMode}` : '',
    toolPolicy === 'readOnly'
      ? t('subagentPolicyReadOnly', { defaultValue: 'read only' })
      : toolPolicy
        ? t('subagentPolicyFull', { defaultValue: 'full tools' })
        : '',
    child?.returnFormat ? `return ${child.returnFormat}` : '',
    toolInvocations !== undefined ? t('subagentSteps', { count: toolInvocations, defaultValue: '{{count}} tools' }) : '',
    durationMs !== undefined && durationMs > 0 ? formatDuration(durationMs) : '',
    queuedMs !== undefined && queuedMs > 0 ? `${t('toolChildQueued')} ${formatDuration(queuedMs)}` : '',
    tokens !== undefined && tokens > 0 ? t('subagentTokensChip', { count: compactNumber(tokens), defaultValue: '{{count}} tokens' }) : '',
    child?.tokenBudget !== undefined ? `token budget ${compactNumber(child.tokenBudget)}` : '',
    child?.timeBudgetMs !== undefined ? `time budget ${formatDuration(child.timeBudgetMs)}` : '',
    child?.budgetExceeded ? 'budget exceeded' : '',
    evidence,
    typeof cacheHitRate === 'number'
      ? `${t('toolChildCache')} ${Math.round((cacheHitRate <= 1 ? cacheHitRate * 100 : cacheHitRate))}%`
      : '',
    child?.background ? t('toolChildBackground') : '',
    child?.heartbeatAgeMs !== undefined ? `${t('toolJobHeartbeat', { defaultValue: 'heartbeat' })} ${formatDuration(child.heartbeatAgeMs)}` : '',
    child?.recoveryStatus ? `${t('toolJobRecoveryStatus', { defaultValue: 'recovery' })} ${child.recoveryStatus}` : '',
    child?.pendingSteers !== undefined && child.pendingSteers > 0
      ? t('subagentSteerPending', { count: child.pendingSteers, defaultValue: '{{count}} steer queued' })
      : '',
    child?.admittedSteers !== undefined && child.admittedSteers > 0
      ? t('subagentSteerAdmitted', { count: child.admittedSteers, defaultValue: '{{count}} steer admitted' })
      : '',
    child?.steerStatus ? `steer ${child.steerStatus}` : '',
    child?.pauseStatus ? `pause ${child.pauseStatus}` : '',
    child?.paused ? t('subagentPausedChip', { defaultValue: 'paused' }) : '',
    child?.isolationMode === 'worktree' ? t('subagentWorktreeIsolated', { defaultValue: 'isolated worktree' }) : '',
    changedFileCount !== undefined && changedFileCount > 0
      ? t('subagentWorktreeChangedFiles', { count: changedFileCount, defaultValue: '{{count}} changed files' })
      : '',
    child?.childTodoCount !== undefined
      ? t('subagentChildTodoCountChip', { count: child.childTodoCount, defaultValue: '{{count}} child todos' })
      : '',
    child?.childTodoProjectionStatus
      ? t('subagentChildTodoProjectionChip', { status: child.childTodoProjectionStatus, defaultValue: 'todo projection {{status}}' })
      : '',
    execution
  ].filter(Boolean)
  return meta.join(' · ')
}

function jobDiagnosticsMeta(child: RuntimeChildMetadata | null, diagnostics: RuntimeJobDiagnosticsMetadata | null, t: Translator): string {
  const heartbeatStatus = formatJobHeartbeatStatus(diagnostics?.heartbeatStatus || child?.heartbeatStatus, t)
  const heartbeatAgeMs = diagnostics?.heartbeatAgeMs ?? child?.heartbeatAgeMs
  const staleAfterMs = diagnostics?.staleAfterMs ?? diagnostics?.stalledAfterMs ?? child?.staleAfterMs
  const rows = [
    heartbeatStatus ? `${t('toolJobHeartbeatStatus', { defaultValue: 'status' })} ${heartbeatStatus}` : '',
    diagnostics?.jobId || child?.jobId ? `job ${diagnostics?.jobId || child?.jobId}` : '',
    diagnostics?.childThreadId || child?.childThreadId ? `thread ${diagnostics?.childThreadId || child?.childThreadId}` : '',
    diagnostics?.childTurnId || child?.childTurnId ? `turn ${diagnostics?.childTurnId || child?.childTurnId}` : '',
    heartbeatAgeMs !== undefined ? `${t('toolJobHeartbeat', { defaultValue: 'heartbeat' })} ${formatDuration(heartbeatAgeMs)}` : '',
    staleAfterMs !== undefined ? `${t('toolJobStaleAfter', { defaultValue: 'stale after' })} ${formatDuration(staleAfterMs)}` : '',
    diagnostics?.recoveryAttempt !== undefined || child?.recoveryAttempt !== undefined
      ? `${t('toolJobRecoveryAttempt', { defaultValue: 'recovery attempt' })} ${diagnostics?.recoveryAttempt ?? child?.recoveryAttempt}`
      : ''
  ].filter(Boolean)
  return rows.join(' · ')
}

function childTodoMeta(child: RuntimeChildMetadata | null, t: Translator): string {
  if (!child || (child.childTodoCount === undefined && !child.childTodoProjectionId)) return ''
  return [
    child.childTodoCount !== undefined
      ? t('subagentChildTodoSummary', {
          count: child.childTodoCount,
          completed: child.childTodoCompletedCount ?? 0,
          inProgress: child.childTodoInProgressCount ?? 0,
          pending: child.childTodoPendingCount ?? 0,
          blocked: child.childTodoBlockedCount ?? 0,
          canceled: child.childTodoCanceledCount ?? 0,
          defaultValue: '{{count}} child todos: {{completed}} done, {{inProgress}} active, {{pending}} pending, {{blocked}} blocked, {{canceled}} canceled'
        })
      : '',
    child.childTodoProjectionStatus
      ? t('subagentChildTodoProjectionStatus', {
          status: child.childTodoProjectionStatus,
          defaultValue: 'projection {{status}}'
        })
      : '',
    child.childTodoProjectionItemCount !== undefined
      ? t('subagentChildTodoProjectionItems', {
          count: child.childTodoProjectionItemCount,
          defaultValue: '{{count}} projected items'
        })
      : '',
    child.childTodoProjectionEvidenceCount !== undefined
      ? t('subagentChildTodoProjectionEvidence', {
          count: child.childTodoProjectionEvidenceCount,
          defaultValue: '{{count}} evidence ids'
        })
      : '',
    child.childTodoProjectionMappedCount !== undefined
      ? t('subagentChildTodoProjectionMapped', {
          count: child.childTodoProjectionMappedCount,
          completed: child.childTodoProjectionCompletedMappedCount ?? 0,
          defaultValue: '{{completed}} completed mapped to parent todos'
        })
      : '',
    child.childTodoProjectionDecisionId
      ? t('subagentChildTodoProjectionDecision', {
          decision: child.childTodoProjectionDecision || child.childTodoProjectionStatus || 'accepted',
          accepted: child.childTodoProjectionAcceptedItemCount ?? 0,
          skipped: child.childTodoProjectionSkippedItemCount ?? 0,
          defaultValue: 'decision {{decision}}: {{accepted}} accepted, {{skipped}} skipped'
        })
      : '',
    child.childTodoProjectionSummary || ''
  ].filter(Boolean).join(' · ')
}

function childIsolationMeta(child: RuntimeChildMetadata | null, t: Translator): string {
  if (child?.isolationMode !== 'worktree') return ''
  const changedFileCount = child.changedFileCount ?? child.changedFiles?.length
  const shortCommit = (value: string | undefined): string => value ? value.slice(0, 12) : ''
  return [
    t('subagentWorktreeIsolated', { defaultValue: 'isolated worktree' }),
    child.worktreeBranch ? `branch ${child.worktreeBranch}` : '',
    child.worktreePath ? `path ${child.worktreePath}` : '',
    child.baseCommit ? `base ${shortCommit(child.baseCommit)}` : '',
    child.currentCommit ? `current ${shortCommit(child.currentCommit)}` : '',
    changedFileCount !== undefined
      ? t('subagentWorktreeChangedFiles', { count: changedFileCount, defaultValue: '{{count}} changed files' })
      : '',
    child.mergeStatus ? `merge ${child.mergeStatus}` : '',
    child.acceptDecisionId ? `accepted ${child.acceptDecisionId}` : '',
    child.appliedPatchDigest ? `patch ${child.appliedPatchDigest.slice(0, 19)}` : '',
    child.conflictReportId ? `conflict ${child.conflictReportId}` : '',
    child.conflictFileCount !== undefined ? `conflicts ${child.conflictFileCount}` : '',
    child.repairDryRunStatus ? `repair ${child.repairDryRunStatus}` : '',
    child.repairDecisionId ? `repair accepted ${child.repairDecisionId}` : '',
    child.cleanupReceiptId ? `cleanup ${child.cleanupReceiptId}` : ''
  ].filter(Boolean).join(' · ')
}

function childCanInspectInput(child: RuntimeChildMetadata | null): boolean {
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(child?.parentThreadId && child.childThreadId && jobId)
}

function childCanPause(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.background &&
    child.canPause === true &&
    child.parentThreadId &&
    jobId &&
    !childHasUnsafeRuntimeLease(child) &&
    (status === 'running' || status === 'queued')
  )
}

function childCanResume(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.background &&
    child.canResume === true &&
    child.parentThreadId &&
    jobId &&
    !childHasUnsafeRuntimeLease(child) &&
    status === 'paused'
  )
}

function childRuntimeHealthStatus(child: RuntimeChildMetadata | null): string {
  return child?.heartbeatStatus?.trim() ?? ''
}

function childHasUnsafeRuntimeLease(child: RuntimeChildMetadata | null): boolean {
  switch (childRuntimeHealthStatus(child)) {
    case 'stale':
    case 'lease_expired':
    case 'orphaned':
    case 'recovering':
    case 'recovered':
    case 'dead_lettered':
      return true
    default:
      return false
  }
}

function childCanUseRecoveryControls(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  const jobId = child?.jobId || child?.childRunId || child?.childId
  const health = childRuntimeHealthStatus(child)
  const retryDeadLetter = health === 'dead_lettered'
  return Boolean(
    child?.background &&
    child.parentThreadId &&
    jobId &&
    (
      retryDeadLetter ||
      ((status === 'running' || status === 'queued' || status === 'paused') &&
        (health === 'stale' || health === 'lease_expired' || health === 'orphaned'))
    )
  )
}

// This is deliberately not a user setting. Worktree mutation controls remain
// absent until the host can issue and durably reconcile same-epoch authority.
const WORKTREE_ISOLATION_CONTROLS_AVAILABLE = false

function childCanReviewIsolation(child: RuntimeChildMetadata | null): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(child?.isolationMode === 'worktree' && child.parentThreadId && jobId)
}

function childIsTerminalForIsolationDecision(status: CardStatus): boolean {
  return status === 'done' || status === 'failed' || status === 'closed'
}

function childReviewedMergeStatus(child: RuntimeChildMetadata | null): boolean {
  return child?.mergeStatus === 'review_requested' || child?.mergeStatus === 'conflicted'
}

function childCanRejectIsolation(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.isolationMode === 'worktree' &&
    child.parentThreadId &&
    jobId &&
    childIsTerminalForIsolationDecision(status) &&
    childReviewedMergeStatus(child)
  )
}

function childCanAcceptIsolation(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.isolationMode === 'worktree' &&
    child.parentThreadId &&
    jobId &&
    childIsTerminalForIsolationDecision(status) &&
    (child.mergeStatus === 'review_requested' || child.mergeStatus === 'clean')
  )
}

function childCanCleanupIsolation(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.isolationMode === 'worktree' &&
    child.parentThreadId &&
    jobId &&
    childIsTerminalForIsolationDecision(status) &&
    (child.mergeStatus === 'review_requested' || child.mergeStatus === 'conflicted' || child.mergeStatus === 'rejected' || child.mergeStatus === 'accepted')
  )
}

function childCanReportConflict(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.isolationMode === 'worktree' &&
    child.parentThreadId &&
    jobId &&
    childIsTerminalForIsolationDecision(status) &&
    child.mergeStatus === 'conflicted'
  )
}

function childCanCheckRepair(child: RuntimeChildMetadata | null, status: CardStatus, reportId?: string): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.isolationMode === 'worktree' &&
    child.parentThreadId &&
    jobId &&
    childIsTerminalForIsolationDecision(status) &&
    (child.mergeStatus === 'conflicted' || child.mergeStatus === 'repair_rejected') &&
    (reportId || child.conflictReportId)
  )
}

function childCanAcceptRepair(child: RuntimeChildMetadata | null, status: CardStatus, reviewId?: string, dryRunStatus?: string): boolean {
  if (!WORKTREE_ISOLATION_CONTROLS_AVAILABLE) return false
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.isolationMode === 'worktree' &&
    child.parentThreadId &&
    jobId &&
    childIsTerminalForIsolationDecision(status) &&
    child.mergeStatus === 'repair_checked' &&
    (dryRunStatus || child.repairDryRunStatus) === 'clean' &&
    (reviewId || child.repairReviewId)
  )
}

function childCanRejectTodoProjection(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.parentThreadId &&
    jobId &&
    child.childTodoProjectionId &&
    child.childTodoProjectionStatus === 'proposed' &&
    childIsTerminalForIsolationDecision(status)
  )
}

function childCanAcceptTodoProjection(child: RuntimeChildMetadata | null, status: CardStatus): boolean {
  const jobId = child?.jobId || child?.childRunId || child?.childId
  return Boolean(
    child?.parentThreadId &&
    jobId &&
    child.childTodoProjectionId &&
    child.childTodoProjectionStatus === 'proposed' &&
    (child.childTodoProjectionCompletedMappedCount ?? 0) > 0 &&
    childIsTerminalForIsolationDecision(status)
  )
}

const SUBAGENT_ACTION_STATUS_CODES = [
  'queued', 'admitted', 'rejected', 'requested', 'paused', 'resumed', 'killed',
  'recovered', 'restarted', 'reviewed', 'accepted', 'cleaned', 'conflicted',
  'repair_checked', 'clean', 'proposed', 'completed', 'success'
] as const
const SUBAGENT_ACTION_MERGE_STATUS_CODES = [
  'not_requested', 'review_requested', 'clean', 'conflicted', 'rejected',
  'accepted', 'cleaned', 'repair_checked', 'repair_rejected'
] as const
const SUBAGENT_ACTION_DRY_RUN_STATUS_CODES = ['clean', 'conflicted', 'failed'] as const
const SUBAGENT_ACTION_REASON_CODES = [
  'forbidden',
  'not_found',
  'validation_error',
  'conflict',
  'worktree_isolation_authority_required',
  'subagents_disabled'
] as const
const SUBAGENT_ACTION_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._:@-]{0,255}$/u

type SubagentActionReasonCode = typeof SUBAGENT_ACTION_REASON_CODES[number] | 'unknown'
type SubagentActionPublicResponse = {
  status?: typeof SUBAGENT_ACTION_STATUS_CODES[number]
  reasonCode?: SubagentActionReasonCode
  jobId?: string
  mergeStatus?: typeof SUBAGENT_ACTION_MERGE_STATUS_CODES[number]
  changedFileCount?: number
  conflictReportId?: string
  repairReviewId?: string
  dryRunStatus?: typeof SUBAGENT_ACTION_DRY_RUN_STATUS_CODES[number]
  projectionId?: string
  decisionId?: string
}

function closedActionCode<const T extends readonly string[]>(value: unknown, allowed: T): T[number] | undefined {
  return typeof value === 'string' && allowed.includes(value as T[number])
    ? value as T[number]
    : undefined
}

function actionIdentifier(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return SUBAGENT_ACTION_ID_PATTERN.test(normalized) ? normalized : undefined
}

/** Never returns response `reason`, `error`, or another free-text field. */
export function projectSubagentActionResponse(body: string): SubagentActionPublicResponse {
  if (!body.trim()) return {}
  try {
    const parsed = JSON.parse(body) as Record<string, unknown>
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const job = parsed.job && typeof parsed.job === 'object' && !Array.isArray(parsed.job)
      ? parsed.job as Record<string, unknown>
      : undefined
    const rawReasonCode = parsed.reasonCode ?? parsed.code
    const reasonCode = closedActionCode(rawReasonCode, SUBAGENT_ACTION_REASON_CODES) ?? (
      rawReasonCode !== undefined || typeof parsed.reason === 'string' || typeof parsed.error === 'string'
        ? 'unknown'
        : undefined
    )
    const changedFileCount = typeof parsed.changedFileCount === 'number' &&
      Number.isSafeInteger(parsed.changedFileCount) && parsed.changedFileCount >= 0
      ? parsed.changedFileCount
      : undefined
    const status = closedActionCode(parsed.status, SUBAGENT_ACTION_STATUS_CODES)
    const jobId = actionIdentifier(parsed.jobId) ?? actionIdentifier(job?.id)
    const mergeStatus = closedActionCode(parsed.mergeStatus, SUBAGENT_ACTION_MERGE_STATUS_CODES)
    const conflictReportId = actionIdentifier(parsed.conflictReportId)
    const repairReviewId = actionIdentifier(parsed.repairReviewId)
    const dryRunStatus = closedActionCode(parsed.dryRunStatus, SUBAGENT_ACTION_DRY_RUN_STATUS_CODES)
    const projectionId = actionIdentifier(parsed.projectionId)
    const decisionId = actionIdentifier(parsed.decisionId)
    return {
      ...(status ? { status } : {}),
      ...(reasonCode ? { reasonCode } : {}),
      ...(jobId ? { jobId } : {}),
      ...(mergeStatus ? { mergeStatus } : {}),
      ...(changedFileCount !== undefined ? { changedFileCount } : {}),
      ...(conflictReportId ? { conflictReportId } : {}),
      ...(repairReviewId ? { repairReviewId } : {}),
      ...(dryRunStatus ? { dryRunStatus } : {}),
      ...(projectionId ? { projectionId } : {}),
      ...(decisionId ? { decisionId } : {})
    }
  } catch {
    return {}
  }
}

function actionReasonText(code: SubagentActionReasonCode | undefined, t: Translator, fallbackKey: string, fallback: string): string {
  switch (code) {
    case 'forbidden':
      return t('subagentActionForbidden', { defaultValue: 'This child action is not allowed' })
    case 'not_found':
      return t('subagentActionNotFound', { defaultValue: 'The child task is no longer available' })
    case 'validation_error':
      return t('subagentActionInvalid', { defaultValue: 'The child action request is invalid' })
    case 'conflict':
      return t('subagentActionConflict', { defaultValue: 'The child task state changed; refresh and try again' })
    case 'worktree_isolation_authority_required':
      return t('subagentActionApprovalRequired', { defaultValue: 'Worktree approval is required' })
    case 'subagents_disabled':
      return t('subagentActionDisabled', { defaultValue: 'Subagent actions are disabled' })
    case 'unknown':
    default:
      return t(fallbackKey, { defaultValue: fallback })
  }
}

const parseJobControlResponse = projectSubagentActionResponse
const parseIsolationReviewResponse = projectSubagentActionResponse
const parseIsolationDecisionResponse = projectSubagentActionResponse
const parseIsolationRepairResponse = projectSubagentActionResponse
const parseChildTodoProjectionResponse = projectSubagentActionResponse

function nextClientMessageId(): string {
  const cryptoValue = globalThis.crypto
  if (cryptoValue && typeof cryptoValue.randomUUID === 'function') {
    return cryptoValue.randomUUID()
  }
  return `steer_${Date.now().toString(36)}`
}

function SubagentPauseControls({
  child,
  status
}: {
  child: RuntimeChildMetadata | null
  status: CardStatus
}): ReactElement | null {
  const { t } = useTranslation('common')
  const [controlStatus, setControlStatus] = useState<PauseControlStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const jobId = child?.jobId || child?.childRunId || child?.childId
  const canPause = childCanPause(child, status)
  const canResume = childCanResume(child, status)
  if (!child || !jobId || (!canPause && !canResume)) return null
  const postControl = async (action: 'pause' | 'resume' | 'kill'): Promise<void> => {
    if (controlStatus === 'sending') return
    setControlStatus('sending')
    setResponseText('')
    try {
      const body = {
        threadId: child.parentThreadId,
        jobId,
        clientRequestId: nextClientMessageId(),
        ...(child.parentTurnId ? { sourceTurnId: child.parentTurnId } : {}),
        ...(action === 'kill' ? { reason: 'killed by parent control' } : {})
      }
      const response = action === 'pause'
        ? await rendererRuntimeClient.pauseTaskJob(body)
        : action === 'resume'
          ? await rendererRuntimeClient.resumeTaskJob(body)
          : await rendererRuntimeClient.killTaskJob(body)
      if (!response.ok) {
        setControlStatus('rejected')
        setResponseText(t('subagentPauseRejected', { defaultValue: 'Request was rejected' }))
        return
      }
      if (action === 'pause') {
        setControlStatus('requested')
        setResponseText(t('subagentPauseRequested', { defaultValue: 'Pause requested for the next safe child boundary' }))
      } else if (action === 'resume') {
        setControlStatus('resumed')
        setResponseText(t('subagentResumeRequested', { defaultValue: 'Resume requested' }))
      } else {
        setControlStatus('killed')
        setResponseText(t('subagentKillRequested', { defaultValue: 'Kill requested' }))
      }
    } catch {
      setControlStatus('error')
      setResponseText(t('subagentPauseFailed', { defaultValue: 'Request failed' }))
    }
  }
  const statusText = responseText || (
    canPause
      ? t('subagentPauseHint', { defaultValue: 'Pauses at the next safe child boundary' })
      : t('subagentResumeHint', { defaultValue: 'Resume the paused child run' })
  )
  return (
    <div className="flex min-w-0 items-center gap-1.5" onClick={(event) => event.stopPropagation()}>
      {canPause ? (
        <>
          <button
            type="button"
            disabled={controlStatus === 'sending'}
            onClick={() => void postControl('pause')}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentPauseAction', { defaultValue: 'Pause at safe boundary' })}
            title={t('subagentPauseAction', { defaultValue: 'Pause at safe boundary' })}
          >
            {controlStatus === 'sending' ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
            ) : (
              <Pause className="h-3.5 w-3.5" strokeWidth={1.9} />
            )}
          </button>
          <button
            type="button"
            disabled={controlStatus === 'sending'}
            onClick={() => void postControl('kill')}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentKillAction', { defaultValue: 'Kill child run' })}
            title={t('subagentKillAction', { defaultValue: 'Kill child run' })}
          >
            <Square className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
        </>
      ) : null}
      {canResume ? (
        <>
          <button
            type="button"
            disabled={controlStatus === 'sending'}
            onClick={() => void postControl('resume')}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentResumeAction', { defaultValue: 'Resume child run' })}
            title={t('subagentResumeAction', { defaultValue: 'Resume child run' })}
          >
            {controlStatus === 'sending' ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
            ) : (
              <Play className="h-3.5 w-3.5" strokeWidth={1.9} />
            )}
          </button>
          <button
            type="button"
            disabled={controlStatus === 'sending'}
            onClick={() => void postControl('kill')}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentKillAction', { defaultValue: 'Kill paused child run' })}
            title={t('subagentKillAction', { defaultValue: 'Kill paused child run' })}
          >
            <Square className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
        </>
      ) : null}
      <span className="max-w-[18rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>
        {statusText}
      </span>
    </div>
  )
}

function SubagentRecoveryControls({
  child,
  status
}: {
  child: RuntimeChildMetadata | null
  status: CardStatus
}): ReactElement | null {
  const { t } = useTranslation('common')
  const [controlStatus, setControlStatus] = useState<JobControlStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const jobId = child?.jobId || child?.childRunId || child?.childId
  if (!child || !jobId || !childCanUseRecoveryControls(child, status)) return null
  const health = childRuntimeHealthStatus(child)
  const deadLettered = health === 'dead_lettered'
  const postControl = async (action: 'recover' | 'restart' | 'kill'): Promise<void> => {
    if (controlStatus === 'sending') return
    setControlStatus('sending')
    setResponseText('')
    try {
      const body = {
        threadId: child.parentThreadId,
        jobId,
        ...(action === 'recover' ? { deliveryId: child.deliveryId, reason: 'recovered from subagent card' } : {}),
        ...(action === 'kill' ? { reason: 'killed after expired runtime lease' } : {})
      }
      const response = action === 'recover'
          ? await rendererRuntimeClient.recoverTaskJob(body)
          : action === 'restart'
            ? await rendererRuntimeClient.restartTaskJob(body)
            : await rendererRuntimeClient.killTaskJob(body)
      const parsed = parseJobControlResponse(response.body)
      if (!response.ok) {
        setControlStatus('rejected')
        setResponseText(t('subagentRecoveryActionRejected', { defaultValue: 'Request was rejected' }))
        return
      }
      if (action === 'recover') {
        setControlStatus('waiting')
        setResponseText(t('subagentRecoverRequested', { defaultValue: 'Recovery requested' }))
      } else if (action === 'restart') {
        setControlStatus('restarted')
        setResponseText(
          parsed.jobId
            ? t('subagentRestartRequestedWithJob', { jobId: parsed.jobId, defaultValue: 'Restarted as {{jobId}}' })
            : t('subagentRestartRequested', { defaultValue: 'Restart requested' })
        )
      } else {
        setControlStatus('killed')
        setResponseText(t('subagentKillRequested', { defaultValue: 'Kill requested' }))
      }
    } catch {
      setControlStatus('error')
      setResponseText(t('subagentRecoveryActionFailed', { defaultValue: 'Request failed' }))
    }
  }
  const statusText = responseText || t('subagentRecoveryHint', {
    defaultValue: 'Lease is stale; recover, restart, or stop the old run'
  })
  return (
    <div className="flex min-w-0 items-center gap-1.5" onClick={(event) => event.stopPropagation()}>
      <button
        type="button"
        disabled={controlStatus === 'sending'}
        onClick={() => void postControl('recover')}
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('subagentRecoverAction', { defaultValue: 'Recover child run' })}
        title={t('subagentRecoverAction', { defaultValue: 'Recover child run' })}
      >
        {controlStatus === 'sending' ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
        ) : (
          <RotateCcw className="h-3.5 w-3.5" strokeWidth={1.9} />
        )}
      </button>
      {!deadLettered ? (
        <>
          <button
            type="button"
            disabled={controlStatus === 'sending'}
            onClick={() => void postControl('restart')}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentRestartAction', { defaultValue: 'Restart child run' })}
            title={t('subagentRestartAction', { defaultValue: 'Restart child run' })}
          >
            <RotateCcw className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
          <button
            type="button"
            disabled={controlStatus === 'sending'}
            onClick={() => void postControl('kill')}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentRecoveryKillAction', { defaultValue: 'Kill expired child run' })}
            title={t('subagentRecoveryKillAction', { defaultValue: 'Kill expired child run' })}
          >
            <Square className="h-3.5 w-3.5" strokeWidth={1.9} />
          </button>
        </>
      ) : null}
      <span className="max-w-[20rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>
        {statusText}
      </span>
    </div>
  )
}

function SubagentSteerForm({ child }: { child: RuntimeChildMetadata }): ReactElement {
  const { t } = useTranslation('common')
  const [message, setMessage] = useState('')
  const [submitStatus, setSubmitStatus] = useState<SteerSubmitStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const jobId = child.jobId || child.childRunId || child.childId!
  const scope = JSON.stringify([child.parentThreadId, jobId, child.childThreadId])
  const activeThreadId = useChatStore(state => state.activeThreadId)
  const viewScope = JSON.stringify([scope, activeThreadId])
  const currentView = useRef({ scope: viewScope, epoch: 0 })
  if (currentView.current.scope !== viewScope) currentView.current = { scope: viewScope, epoch: currentView.current.epoch + 1 }
  const epoch = currentView.current.epoch
  const draft = useRef(message)
  draft.current = message
  const mounted = useRef(false)
  const sending = useRef(false)
  // An unknown result retains the entire envelope, including the inspected anchors.
  const attempts = useRef(new Map<string, { request: ChildInputSubmitRequestV1; uncertain: boolean }>())
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])
  useEffect(() => { setSubmitStatus('idle'); setResponseText('') }, [viewScope])
  const submit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault()
    event.stopPropagation()
    const text = message.trim()
    if ((!text && !attempts.current.has(scope)) || sending.current) return
    const isCurrent = (): boolean => mounted.current && currentView.current.epoch === epoch
    const submittedDraft = message
    sending.current = true
    setSubmitStatus('sending')
    setResponseText('')
    try {
      let attempt = attempts.current.get(scope)
      if (!attempt) {
        const caps = await rendererRuntimeClient.inspectChildInput(child.parentThreadId!, jobId, child.childThreadId)
        if (!isCurrent()) return
        if (caps.send === 'disabled' || !caps.runId || !caps.childTurnId) {
          setSubmitStatus('rejected')
          setResponseText(t('subagentMessageUnavailable', { defaultValue: 'Child cannot accept a message right now' }))
          return
        }
        attempt = { uncertain: false, request: {
          mode: 'start_or_steer', threadId: child.parentThreadId!, jobId,
          expectedRunId: caps.runId, expectedChildTurnId: caps.childTurnId,
          message: text, clientMessageId: globalThis.crypto.randomUUID()
        } }
        attempts.current.set(scope, attempt)
      }
      const receipt = await rendererRuntimeClient.submitChildInput(attempt.request, child.childThreadId!)
      if (!isCurrent()) { attempt.uncertain = true; return }
      attempts.current.delete(scope)
      setSubmitStatus(receipt.admission)
      setResponseText(receipt.action === 'start'
        ? t('subagentChildTurnStarted', { defaultValue: 'Started a new turn in this child session' })
        : receipt.admission === 'queued'
          ? t('subagentSteerQueued', { defaultValue: 'Queued for the next safe child turn boundary' })
          : t('subagentSteerAdmittedNotice', { defaultValue: 'Admitted into the child run' }))
      if (draft.current === submittedDraft && submittedDraft.trim() === attempt.request.message) setMessage('')
    } catch (error) {
      const attempt = attempts.current.get(scope)
      if (attempt && !(error instanceof ChildInputRejectedError)) attempt.uncertain = true
      if (!isCurrent()) return
      // A later authority failure cannot disprove a previously unknown admission.
      const rejected = error instanceof ChildInputRejectedError && (!attempt?.uncertain || error.code === 'input_cancelled')
      if (rejected) attempts.current.delete(scope)
      setSubmitStatus(rejected ? 'rejected' : 'error')
      setResponseText(attempts.current.has(scope)
        ? t('subagentMessageUnconfirmed', { defaultValue: 'Message unconfirmed; retry the same message to check admission' })
        : error instanceof ChildInputRejectedError
          ? t('subagentSteerRejected', { defaultValue: 'Child message was rejected' })
          : t('subagentSteerFailed', { defaultValue: 'Failed to send child message' }))
    } finally {
      sending.current = false
      if (isCurrent()) setSubmitStatus(current => current === 'sending' ? 'idle' : current)
    }
  }
  const pending = attempts.current.get(scope)
  const statusText = responseText || (pending
    ? t('subagentMessageUnconfirmed', { defaultValue: 'Message unconfirmed; retry the same message to check admission' })
    : t('subagentMessageHint', { defaultValue: 'Message this child session' }))
  return (
    <form className="flex min-w-0 items-center gap-1.5" onSubmit={submit} onClick={event => event.stopPropagation()}>
      <input
        className="min-w-0 flex-1 rounded-md border border-ds-border/70 bg-ds-subtle/50 px-2 py-1 text-[12.5px] leading-5 text-ds-muted outline-none transition placeholder:text-ds-faint focus:border-ds-muted/50"
        value={message}
        onChange={event => {
          setMessage([...event.target.value].slice(0, MAX_STEER_MESSAGE_CHARS).join(''))
          if (submitStatus !== 'sending') { setSubmitStatus('idle'); setResponseText('') }
        }}
        maxLength={MAX_STEER_MESSAGE_CHARS * 2}
        aria-label={t('subagentSteerInputLabel', { defaultValue: 'Message child agent' })}
        placeholder={t('subagentMessagePlaceholder', { defaultValue: 'Message child agent' })}
      />
      <button
        type="submit"
        disabled={(!message.trim() && !pending) || submitStatus === 'sending'}
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={pending ? t('subagentMessageRetry', { defaultValue: 'Retry unconfirmed child message' }) : t('subagentSteerSend', { defaultValue: 'Send child message' })}
        title={pending ? t('subagentMessageRetry', { defaultValue: 'Retry unconfirmed child message' }) : t('subagentSteerSend', { defaultValue: 'Send child message' })}
      >
        {submitStatus === 'sending' ? <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
          : <SendHorizontal className="h-3.5 w-3.5" strokeWidth={1.9} />}
      </button>
      <span role="status" className="max-w-[16rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>{statusText}</span>
    </form>
  )
}

function SubagentIsolationReviewControl({
  child
}: {
  child: RuntimeChildMetadata | null
}): ReactElement | null {
  const { t } = useTranslation('common')
  const [reviewStatus, setReviewStatus] = useState<IsolationReviewStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const jobId = child?.jobId || child?.childRunId || child?.childId
  if (!child || !jobId || !childCanReviewIsolation(child)) return null
  const submit = async (): Promise<void> => {
    if (reviewStatus === 'sending') return
    setReviewStatus('sending')
    setResponseText('')
    try {
      const response = await rendererRuntimeClient.reviewTaskJobIsolation({
        threadId: child.parentThreadId,
        jobId,
        clientRequestId: nextClientMessageId()
      })
      const parsed = parseIsolationReviewResponse(response.body)
      if (!response.ok) {
        setReviewStatus('rejected')
        setResponseText(actionReasonText(parsed.reasonCode, t, 'subagentReviewRejected', 'Diff review was rejected'))
        return
      }
      setReviewStatus('reviewed')
      const count = parsed.changedFileCount ?? child.changedFileCount ?? child.changedFiles?.length ?? 0
      setResponseText(
        t('subagentReviewRequested', {
          count,
          status: parsed.mergeStatus || child.mergeStatus || 'review_requested',
          defaultValue: 'Diff review ready: {{count}} changed files'
        })
      )
    } catch {
      setReviewStatus('error')
      setResponseText(t('subagentReviewFailed', { defaultValue: 'Diff review failed' }))
    }
  }
  const statusText = responseText || t('subagentReviewInspectHint', { defaultValue: 'Inspect isolated worktree diff without merging' })
  return (
    <div className="flex min-w-0 items-center gap-1.5" onClick={(event) => event.stopPropagation()}>
      <button
        type="button"
        disabled={reviewStatus === 'sending'}
        onClick={() => void submit()}
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
        aria-label={t('subagentReviewInspectAction', { defaultValue: 'Inspect isolated diff' })}
        title={t('subagentReviewInspectAction', { defaultValue: 'Inspect isolated diff' })}
      >
        {reviewStatus === 'sending' ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
        ) : (
          <Diff className="h-3.5 w-3.5" strokeWidth={1.9} />
        )}
      </button>
      <span className="max-w-[18rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>
        {statusText}
      </span>
    </div>
  )
}

function SubagentIsolationDecisionControls({
  child,
  status
}: {
  child: RuntimeChildMetadata | null
  status: CardStatus
}): ReactElement | null {
  const { t } = useTranslation('common')
  const [decisionStatus, setDecisionStatus] = useState<IsolationDecisionStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const jobId = child?.jobId || child?.childRunId || child?.childId
  const canAccept = childCanAcceptIsolation(child, status)
  const canReject = childCanRejectIsolation(child, status)
  const canCleanup = childCanCleanupIsolation(child, status)
  if (!child || !jobId || (!canAccept && !canReject && !canCleanup)) return null
  const submit = async (action: 'accept' | 'reject' | 'cleanup'): Promise<void> => {
    if (decisionStatus === 'sending') return
    setDecisionStatus('sending')
    setResponseText('')
    try {
      const clientRequestId = nextClientMessageId()
      const body = {
        threadId: child.parentThreadId,
        jobId,
        clientRequestId
      }
      const response = action === 'accept'
        ? await rendererRuntimeClient.acceptTaskJobIsolation({
            ...body,
            mergeRequestId: child.mergeDecisionId || clientRequestId
          })
        : action === 'reject'
          ? await rendererRuntimeClient.rejectTaskJobIsolation({
            ...body,
            reason: 'rejected by parent'
          })
          : await rendererRuntimeClient.cleanupTaskJobIsolation(body)
      const parsed = parseIsolationDecisionResponse(response.body)
      if (!response.ok) {
        setDecisionStatus('error')
        const fallbackKey = action === 'accept'
          ? 'subagentAcceptFailed'
          : action === 'reject'
            ? 'subagentRejectFailed'
            : 'subagentCleanupFailed'
        const fallback = action === 'accept'
          ? 'Accept request failed'
          : action === 'reject'
            ? 'Reject request failed'
            : 'Cleanup request failed'
        setResponseText(actionReasonText(parsed.reasonCode, t, fallbackKey, fallback))
        return
      }
      if (action === 'accept') {
        setDecisionStatus('accepted')
        setResponseText(t('subagentAcceptRequested', { defaultValue: 'Clean diff accepted into parent workspace' }))
      } else if (action === 'reject') {
        setDecisionStatus('rejected')
        setResponseText(t('subagentRejectRequested', { defaultValue: 'Child result rejected; isolated files retained for audit' }))
      } else {
        setDecisionStatus('cleaned')
        setResponseText(t('subagentCleanupRequested', { defaultValue: 'Isolated worktree cleanup recorded' }))
      }
    } catch {
      setDecisionStatus('error')
      setResponseText(action === 'accept'
        ? t('subagentAcceptFailed', { defaultValue: 'Accept request failed' })
        : action === 'reject'
          ? t('subagentRejectFailed', { defaultValue: 'Reject request failed' })
          : t('subagentCleanupFailed', { defaultValue: 'Cleanup request failed' }))
    }
  }
  const statusText = responseText || t('subagentDecisionHint', { defaultValue: 'Parent decision only; no automatic cleanup' })
  return (
    <div className="flex min-w-0 items-center gap-1.5" onClick={(event) => event.stopPropagation()}>
      {canAccept ? (
        <button
          type="button"
          disabled={decisionStatus === 'sending'}
          onClick={() => void submit('accept')}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('subagentAcceptAction', { defaultValue: 'Accept clean isolated diff' })}
          title={t('subagentAcceptAction', { defaultValue: 'Accept clean isolated diff' })}
        >
          {decisionStatus === 'sending' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
          ) : (
            <Check className="h-3.5 w-3.5" strokeWidth={1.9} />
          )}
        </button>
      ) : null}
      {canReject ? (
        <button
          type="button"
          disabled={decisionStatus === 'sending'}
          onClick={() => void submit('reject')}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('subagentRejectAction', { defaultValue: 'Reject isolated result' })}
          title={t('subagentRejectAction', { defaultValue: 'Reject isolated result' })}
        >
          {decisionStatus === 'sending' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
          ) : (
            <Ban className="h-3.5 w-3.5" strokeWidth={1.9} />
          )}
        </button>
      ) : null}
      {canCleanup ? (
        <button
          type="button"
          disabled={decisionStatus === 'sending'}
          onClick={() => void submit('cleanup')}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('subagentCleanupAction', { defaultValue: 'Clean up isolated worktree' })}
          title={t('subagentCleanupAction', { defaultValue: 'Clean up isolated worktree' })}
        >
          {decisionStatus === 'sending' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
          ) : (
            <Trash2 className="h-3.5 w-3.5" strokeWidth={1.9} />
          )}
        </button>
      ) : null}
      <span className="max-w-[18rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>
        {statusText}
      </span>
    </div>
  )
}

function SubagentIsolationRepairControls({
  child,
  status
}: {
  child: RuntimeChildMetadata | null
  status: CardStatus
}): ReactElement | null {
  const { t } = useTranslation('common')
  const [repairStatus, setRepairStatus] = useState<IsolationRepairStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const [repairPatch, setRepairPatch] = useState('')
  const [localReportId, setLocalReportId] = useState(child?.conflictReportId ?? '')
  const [localReviewId, setLocalReviewId] = useState(child?.repairReviewId ?? '')
  const [localDryRunStatus, setLocalDryRunStatus] = useState(child?.repairDryRunStatus ?? '')
  const [localMergeStatus, setLocalMergeStatus] = useState(child?.mergeStatus ?? '')
  const jobId = child?.jobId || child?.childRunId || child?.childId
  const reportId = localReportId || child?.conflictReportId || ''
  const reviewId = localReviewId || child?.repairReviewId || ''
  const dryRunStatus = localDryRunStatus || child?.repairDryRunStatus || ''
  const mergeStatus = localMergeStatus || child?.mergeStatus || ''
  const hasRepairBase = Boolean(child?.isolationMode === 'worktree' && child.parentThreadId && jobId && childIsTerminalForIsolationDecision(status))
  const canReport = hasRepairBase && mergeStatus === 'conflicted'
  const canCheck = hasRepairBase && (mergeStatus === 'conflicted' || mergeStatus === 'repair_rejected') && Boolean(reportId)
  const canAccept = hasRepairBase && mergeStatus === 'repair_checked' && dryRunStatus === 'clean' && Boolean(reviewId)
  if (!child || !jobId || (!canReport && !canCheck && !canAccept)) return null

  const reportConflict = async (): Promise<void> => {
    if (repairStatus === 'sending') return
    setRepairStatus('sending')
    setResponseText('')
    try {
      const response = await rendererRuntimeClient.conflictReportTaskJobIsolation({
        threadId: child.parentThreadId,
        jobId,
        clientRequestId: nextClientMessageId()
      })
      const parsed = parseIsolationRepairResponse(response.body)
      if (!response.ok) {
        setRepairStatus('rejected')
        setResponseText(actionReasonText(parsed.reasonCode, t, 'subagentConflictReportFailed', 'Conflict report failed'))
        return
      }
      if (parsed.conflictReportId) {
        setLocalReportId(parsed.conflictReportId)
      }
      setLocalMergeStatus(parsed.mergeStatus || 'conflicted')
      setRepairStatus('reported')
      setResponseText(t('subagentConflictReportRequested', { defaultValue: 'Conflict report recorded without touching the parent workspace' }))
    } catch {
      setRepairStatus('error')
      setResponseText(t('subagentConflictReportFailed', { defaultValue: 'Conflict report failed' }))
    }
  }

  const checkRepair = async (): Promise<void> => {
    const patch = repairPatch.trim()
    if (!patch || !reportId || repairStatus === 'sending') return
    setRepairStatus('sending')
    setResponseText('')
    try {
      const response = await rendererRuntimeClient.repairCheckTaskJobIsolation({
        threadId: child.parentThreadId,
        jobId,
        conflictReportId: reportId,
        repairPatch: patch,
        clientRequestId: nextClientMessageId()
      })
      const parsed = parseIsolationRepairResponse(response.body)
      if (!response.ok) {
        setRepairStatus('rejected')
        setResponseText(actionReasonText(parsed.reasonCode, t, 'subagentRepairCheckFailed', 'Repair dry-run failed'))
        return
      }
      if (parsed.repairReviewId) {
        setLocalReviewId(parsed.repairReviewId)
      }
      if (parsed.dryRunStatus) {
        setLocalDryRunStatus(parsed.dryRunStatus)
      }
      if (parsed.mergeStatus) {
        setLocalMergeStatus(parsed.mergeStatus)
      }
      setRepairStatus('checked')
      setResponseText(
        parsed.dryRunStatus === 'clean'
          ? t('subagentRepairDryRunClean', { defaultValue: 'Repair patch dry-run is clean; nothing has been applied yet' })
          : t('subagentRepairDryRunConflicted', { defaultValue: 'Repair patch still conflicts; nothing was applied' })
      )
    } catch {
      setRepairStatus('error')
      setResponseText(t('subagentRepairCheckFailed', { defaultValue: 'Repair dry-run failed' }))
    }
  }

  const acceptRepair = async (): Promise<void> => {
    if (!reviewId || repairStatus === 'sending') return
    setRepairStatus('sending')
    setResponseText('')
    const clientRequestId = nextClientMessageId()
    try {
      const response = await rendererRuntimeClient.repairAcceptTaskJobIsolation({
        threadId: child.parentThreadId,
        jobId,
        repairReviewId: reviewId,
        clientRequestId
      })
      const parsed = parseIsolationRepairResponse(response.body)
      if (!response.ok) {
        setRepairStatus('rejected')
        setResponseText(actionReasonText(parsed.reasonCode, t, 'subagentRepairAcceptFailed', 'Repair accept failed'))
        return
      }
      if (parsed.mergeStatus) {
        setLocalMergeStatus(parsed.mergeStatus)
      }
      setRepairStatus('accepted')
      setResponseText(t('subagentRepairAccepted', { defaultValue: 'Approved repair patch applied; worktree was not cleaned up' }))
    } catch {
      setRepairStatus('error')
      setResponseText(t('subagentRepairAcceptFailed', { defaultValue: 'Repair accept failed' }))
    }
  }

  const statusText = responseText || t('subagentRepairHint', { defaultValue: 'Repair check is dry-run only; Accept repair requires parent approval' })
  return (
    <div className="space-y-1.5" onClick={(event) => event.stopPropagation()}>
      {canReport ? (
        <div className="flex min-w-0 items-center gap-1.5">
          <button
            type="button"
            disabled={repairStatus === 'sending'}
            onClick={() => void reportConflict()}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentConflictReportAction', { defaultValue: 'Record conflict report' })}
            title={t('subagentConflictReportAction', { defaultValue: 'Record conflict report' })}
          >
            {repairStatus === 'sending' ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
            ) : (
              <Diff className="h-3.5 w-3.5" strokeWidth={1.9} />
            )}
          </button>
          <span className="max-w-[18rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>{statusText}</span>
        </div>
      ) : null}
      {child.conflictSummary ? (
        <p className="truncate text-[12px] leading-5 text-ds-faint" title={child.conflictSummary}>{child.conflictSummary}</p>
      ) : null}
      {canCheck ? (
        <div className="flex min-w-0 items-start gap-1.5">
          <textarea
            className="min-h-14 min-w-0 flex-1 resize-y rounded-md border border-ds-border/70 bg-ds-subtle/50 px-2 py-1 text-[12px] leading-5 text-ds-muted outline-none transition placeholder:text-ds-faint focus:border-ds-muted/50"
            value={repairPatch}
            onChange={(event) => {
              setRepairPatch(event.target.value)
              if (repairStatus !== 'sending') {
                setRepairStatus('idle')
                setResponseText('')
              }
            }}
            aria-label={t('subagentRepairPatchInputLabel', { defaultValue: 'Repair patch' })}
            placeholder={t('subagentRepairPatchPlaceholder', { defaultValue: 'Paste a repair patch to dry-run against the parent workspace' })}
          />
          <button
            type="button"
            disabled={!repairPatch.trim() || repairStatus === 'sending'}
            onClick={() => void checkRepair()}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentRepairCheckAction', { defaultValue: 'Dry-run repair patch' })}
            title={t('subagentRepairCheckAction', { defaultValue: 'Dry-run repair patch' })}
          >
            {repairStatus === 'sending' ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
            ) : (
              <Diff className="h-3.5 w-3.5" strokeWidth={1.9} />
            )}
          </button>
        </div>
      ) : null}
      {canAccept ? (
        <div className="flex min-w-0 items-center gap-1.5">
          <button
            type="button"
            disabled={repairStatus === 'sending'}
            onClick={() => void acceptRepair()}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('subagentRepairAcceptAction', { defaultValue: 'Accept clean repair patch' })}
            title={t('subagentRepairAcceptAction', { defaultValue: 'Accept clean repair patch' })}
          >
            {repairStatus === 'sending' ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
            ) : (
              <Check className="h-3.5 w-3.5" strokeWidth={1.9} />
            )}
          </button>
          <span className="max-w-[18rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>{statusText}</span>
        </div>
      ) : null}
    </div>
  )
}

function SubagentChildTodoProjectionControls({
  child,
  status
}: {
  child: RuntimeChildMetadata | null
  status: CardStatus
}): ReactElement | null {
  const { t } = useTranslation('common')
  const activeThreadTodos = useChatStore((s) => s.activeThreadTodos)
  const [projectionStatus, setProjectionStatus] = useState<ChildTodoProjectionStatus>('idle')
  const [responseText, setResponseText] = useState('')
  const jobId = child?.jobId || child?.childRunId || child?.childId
  const canAccept = childCanAcceptTodoProjection(child, status)
  const canReject = childCanRejectTodoProjection(child, status)
  if (!child || !jobId || (!canAccept && !canReject)) return null

  const submitProjection = async (action: 'accept' | 'reject'): Promise<void> => {
    if (projectionStatus === 'sending') return
    setProjectionStatus('sending')
    setResponseText('')
    try {
      const clientRequestId = nextClientMessageId()
      const body = {
        threadId: child.parentThreadId,
        jobId,
        projectionId: child.childTodoProjectionId,
        clientRequestId
      }
      const response = action === 'accept'
        ? await rendererRuntimeClient.acceptChildTodoProjectionTaskJob({
            ...body,
            approvalId: `approval_${clientRequestId}`,
            expectedParentTodosUpdatedAt: activeThreadTodos?.updatedAt
          })
        : await rendererRuntimeClient.rejectChildTodoProjectionTaskJob({
            ...body,
            reason: 'rejected by parent'
          })
      const parsed = parseChildTodoProjectionResponse(response.body)
      if (!response.ok) {
        setProjectionStatus('error')
        setResponseText(actionReasonText(
          parsed.reasonCode,
          t,
          action === 'accept' ? 'subagentChildTodoAcceptFailed' : 'subagentChildTodoRejectFailed',
          action === 'accept' ? 'Child todo projection accept failed' : 'Child todo projection reject failed'
        ))
        return
      }
      if (action === 'accept') {
        setProjectionStatus('accepted')
        setResponseText(t('subagentChildTodoAccepted', {
          decisionId: parsed.decisionId || '',
          defaultValue: 'Child todo projection accepted by parent; parent goal is unchanged'
        }))
      } else {
        setProjectionStatus('rejected')
        setResponseText(t('subagentChildTodoRejected', { defaultValue: 'Child todo projection rejected; parent todos were unchanged' }))
      }
    } catch {
      setProjectionStatus('error')
      setResponseText(t('subagentChildTodoRejectFailed', { defaultValue: 'Child todo projection request failed' }))
    }
  }

  const statusText = responseText || t('subagentChildTodoProposalHint', { defaultValue: 'Child todo projection is a proposal; parent approval is required' })
  return (
    <div className="flex min-w-0 items-center gap-1.5" onClick={(event) => event.stopPropagation()}>
      {canAccept ? (
        <button
          type="button"
          disabled={projectionStatus === 'sending'}
          onClick={() => void submitProjection('accept')}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('subagentChildTodoAcceptAction', { defaultValue: 'Accept child todo projection' })}
          title={t('subagentChildTodoAcceptAction', { defaultValue: 'Accept child todo projection' })}
        >
          {projectionStatus === 'sending' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
          ) : (
            <Check className="h-3.5 w-3.5" strokeWidth={1.9} />
          )}
        </button>
      ) : null}
      {canReject ? (
        <button
          type="button"
          disabled={projectionStatus === 'sending'}
          onClick={() => void submitProjection('reject')}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('subagentChildTodoRejectAction', { defaultValue: 'Reject child todo projection' })}
          title={t('subagentChildTodoRejectAction', { defaultValue: 'Reject child todo projection' })}
        >
          {projectionStatus === 'sending' ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
          ) : (
            <ListTodo className="h-3.5 w-3.5" strokeWidth={1.9} />
          )}
        </button>
      ) : null}
      <span className="max-w-[18rem] truncate text-[12px] leading-5 text-ds-faint" title={statusText}>
        {statusText}
      </span>
    </div>
  )
}

export function subagentLineText(block: ChatBlock, t: Translator): { text: string; title: string } {
  const child = childMetaFromBlock(block)
  const text = subagentPublicSummary(block, child, t)
  const meta = childLineMeta(child, t)
  return { text, title: [text, meta].filter(Boolean).join(' · ') }
}

function ChildThreadButton({ childThreadId }: { childThreadId?: string }): ReactElement | null {
  const { t } = useTranslation('common')
  const selectThread = useChatStore((s) => s.selectThread)
  if (!childThreadId) return null
  return (
    <button
      type="button"
      onClick={(event) => {
        event.stopPropagation()
        void selectThread(childThreadId).catch(() => undefined)
      }}
      className="ml-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover/70 hover:text-ds-muted"
      aria-label={t('subagentOpenSession', { defaultValue: 'Open child session' })}
      title={t('subagentOpenSession', { defaultValue: 'Open child session' })}
    >
      <ExternalLink className="h-3 w-3" strokeWidth={1.9} />
    </button>
  )
}

/** Native control state is independent of the protected start-receipt lease. */
export function SubagentNativeControls({ block }: { block: ChatBlock }): ReactElement | null {
  const { t } = useTranslation('common')
  const child = childMetaFromBlock(block)
  const status = resolveStatus(block, child)
  const canSteer = childCanInspectInput(child)
  const canPauseOrResume = childCanPause(child, status) || childCanResume(child, status)
  const canUseRecoveryControls = childCanUseRecoveryControls(child, status)
  const canReviewIsolation = childCanReviewIsolation(child)
  const canDecideIsolation = childCanAcceptIsolation(child, status) || childCanRejectIsolation(child, status) || childCanCleanupIsolation(child, status)
  const canRepairIsolation = childCanReportConflict(child, status) || childCanCheckRepair(child, status, child?.conflictReportId) || childCanAcceptRepair(child, status, child?.repairReviewId, child?.repairDryRunStatus)
  const canActOnChildTodoProjection = childCanRejectTodoProjection(child, status) || childCanAcceptTodoProjection(child, status)

  if (!child) return null
  return <div data-subagent-native-controls aria-label={statusLabel(status, t)}>
    <div className={subagentCss.nativeActions}><ChildThreadButton childThreadId={child.childThreadId} /></div>
    <div className={subagentCss.nativeControls}>
      {canReviewIsolation ? <SubagentIsolationReviewControl child={child} /> : null}
      {canDecideIsolation ? <SubagentIsolationDecisionControls child={child} status={status} /> : null}
      {canRepairIsolation ? <SubagentIsolationRepairControls child={child} status={status} /> : null}
      {canActOnChildTodoProjection ? <SubagentChildTodoProjectionControls child={child} status={status} /> : null}
      {canUseRecoveryControls ? <SubagentRecoveryControls child={child} status={status} /> : null}
      {canPauseOrResume ? <SubagentPauseControls child={child} status={status} /> : null}
      {canSteer ? <SubagentSteerForm child={child} /> : null}
    </div>
  </div>
}

/** Recorded lifecycle and existing diagnostics use the compact DSH field layout. */
export function SubagentDetailBody({ block, showControls = true }: { block: ChatBlock; showControls?: boolean }): ReactElement | null {
  const { t } = useTranslation('common')
  const child = childMetaFromBlock(block)
  const model = nativeSubagentCardModel(child, t)
  const observations = [
    { label: t('toolChildAgent'), value: childLineMeta(child, t) },
    { label: t('toolJobHeartbeat', { defaultValue: 'Heartbeat' }), value: jobDiagnosticsMeta(child, diagnosticsFromBlock(block), t) },
    { label: t('subagentWorktreeIsolated', { defaultValue: 'Isolated worktree' }), value: childIsolationMeta(child, t) },
    { label: t('subagentChildTodoCountChip', { count: child?.childTodoCount ?? 0, defaultValue: '{{count}} child todos' }), value: childTodoMeta(child, t) }
  ].filter(field => field.value !== '')
  const items = model.items.map(item => ({ ...item, fields: [...item.fields, ...observations] }))
  return <section data-dsh-subagent-details className={subagentCss.nativeDetails}>
    <ToolDetails model={{ ...model, items }} />
    {showControls ? <SubagentNativeControls block={block} /> : null}
  </section>
}
