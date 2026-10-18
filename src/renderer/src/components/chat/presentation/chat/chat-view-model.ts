/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { WorkDetailsMode } from '../../../../lib/chat-presentation-preferences'
export type ChatPresentationPolicy = {
  readonly mode: WorkDetailsMode
  readonly foldCompletedTurns: boolean
  readonly stepGrouping: 'collapsed' | 'history' | 'none'
  readonly liveProcessDetail: boolean
  readonly settledReasoningPreview: boolean
}
const policies: Readonly<Record<WorkDetailsMode, ChatPresentationPolicy>> = {
  compact: { mode: 'compact', foldCompletedTurns: true, stepGrouping: 'collapsed', liveProcessDetail: false, settledReasoningPreview: false },
  standard: { mode: 'standard', foldCompletedTurns: true, stepGrouping: 'collapsed', liveProcessDetail: true, settledReasoningPreview: true },
  detailed: { mode: 'detailed', foldCompletedTurns: true, stepGrouping: 'history', liveProcessDetail: true, settledReasoningPreview: true },
  verbose: { mode: 'verbose', foldCompletedTurns: false, stepGrouping: 'none', liveProcessDetail: false, settledReasoningPreview: true }
}
export const presentationPolicyFor = (mode: WorkDetailsMode): ChatPresentationPolicy => policies[mode]

import type { ChatBlock, ToolBlock } from '../../../../agent/types'
import { PublicToolResultProjectionV1 } from '../../../../../../../packages/runtime/src/contracts/items'
import { toolResultLocalSelectorSchemaV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { childProcessGroupKey, childProcessIdentityKey, isBackgroundShellProcessBlock,
  isSubagentProcessBlock, shouldMergeProcessChildBlocks } from '../../SubagentCallCard'

// Preserve Analytix's current child identity merge; presentation owns no session state.
export type ProcessSection = {
  id: string
  kind: 'input' | 'execution' | 'output' | 'subagent' | 'background_shell' | 'error' | 'retry'
  blocks: ChatBlock[]
  /** A later public input/reply/status boundary ended this presentation segment, not its tools. */
  presentationClosed?: true
}

export function groupProcessSections(blocks: ChatBlock[], replyBoundaryAfterIds: readonly string[] = []): ProcessSection[] {
  const sections: ProcessSection[] = []
  const subagentSections = new Map<string, ProcessSection>()
  const replyBoundaries = new Set(replyBoundaryAfterIds)
  let presentationSegmentStart = 0
  const closePresentationSegment = () => {
    for (let index = presentationSegmentStart; index < sections.length; index++) sections[index].presentationClosed = true
    presentationSegmentStart = sections.length
    subagentSections.clear()
  }
  let previousBlockId: string | undefined

  for (const block of blocks) {
    if (previousBlockId !== undefined && replyBoundaries.has(previousBlockId)) closePresentationSegment()
    previousBlockId = block.id
    if (block.kind === 'user') {
      closePresentationSegment()
      // Steering is an independent ordered input, never part of a folded group.
      sections.push({ id: `input-${block.id}`, kind: 'input', blocks: [block] })
      continue
    }
    if (block.kind === 'system' && (block.severity === 'error' || block.meta?.providerRetry)) {
      closePresentationSegment()
      const kind = block.severity === 'error' ? 'error' : 'retry'
      sections.push({ id: `${kind}-${block.id}`, kind, blocks: [block] })
      continue
    }
    if (block.kind === 'assistant' && block.text.trim()) closePresentationSegment()
    if (isBackgroundShellProcessBlock(block)) {
      const last = sections[sections.length - 1]
      const first = last?.blocks[0]
      if (
        last &&
        last.kind === 'background_shell' && !last.presentationClosed &&
        first &&
        shouldMergeProcessChildBlocks(first, block)
      ) {
        last.blocks.push(block)
        continue
      }
      sections.push({ id: `background-shell-${block.id}`, kind: 'background_shell', blocks: [block] })
      continue
    }
    if (isSubagentProcessBlock(block)) {
      const groupKey = childProcessGroupKey(block)
      const section = groupKey ? subagentSections.get(groupKey) : undefined
      if (section) {
        upsertProcessChildBlock(section, block)
        continue
      }
      const nextSection: ProcessSection = { id: `subagent-${block.id}`, kind: 'subagent', blocks: [block] }
      sections.push(nextSection)
      if (groupKey) subagentSections.set(groupKey, nextSection)
      continue
    }
    if (isAutoContinueProcessBlock(block)) {
      sections.push({ id: `execution-${block.id}`, kind: 'execution', blocks: [block] })
      continue
    }
    const kind = block.kind === 'assistant' ? 'output' : 'execution'
    const last = sections[sections.length - 1]
    if (last && last.kind === kind && !last.presentationClosed) {
      last.blocks.push(block)
      continue
    }
    sections.push({
      id: `${kind}-${block.id}`,
      kind,
      blocks: [block]
    })
  }

  if (previousBlockId !== undefined && replyBoundaries.has(previousBlockId)) closePresentationSegment()
  return sections
}

function upsertProcessChildBlock(section: ProcessSection, block: ChatBlock): void {
  const key = childProcessIdentityKey(block)
  const index = section.blocks.findIndex((candidate) => childProcessIdentityKey(candidate) === key)
  if (index < 0) {
    section.blocks.push(block)
    return
  }
  section.blocks[index] = mergeProcessChildBlock(section.blocks[index], block)
}

function mergeProcessChildBlock(previous: ChatBlock, next: ChatBlock): ChatBlock {
  if (previous.kind !== 'tool' || next.kind !== 'tool') return next
  const recordedSkill = [next, ...(next.meta?.runtimeStatus === 'tool_progress' ? [previous] : [])].find(block => {
    if (block.meta?.toolName !== 'run_skill' || block.toolKind !== 'tool_call') return false
    const selector = toolResultLocalSelectorSchemaV1.omit({ threadId: true }).safeParse(block.meta.localResult)
    const result = PublicToolResultProjectionV1.safeParse(block.meta.publicResult)
    return selector.success && result.success && result.data.projectionKind === 'host_status' &&
      block.meta.sourceItemId === selector.data.resultItemId && block.meta.callId === selector.data.callId && block.meta.turnId === selector.data.turnId
  })
  return {
    ...previous,
    ...next,
    summary: next.summary || previous.summary,
    detail: next.detail || previous.detail,
    ...(recordedSkill ? { id: recordedSkill.id, toolKind: recordedSkill.toolKind, status: recordedSkill.status,
      summary: recordedSkill.summary, detail: recordedSkill.detail } : {}),
    meta: {
      ...previous.meta,
      ...next.meta,
      // The call observation and the ongoing child lifecycle are independent.
      ...(recordedSkill ? { toolName: 'run_skill', sourceItemId: recordedSkill.meta?.sourceItemId,
        callId: recordedSkill.meta?.callId, turnId: recordedSkill.meta?.turnId,
        localResult: recordedSkill.meta?.localResult, publicResult: recordedSkill.meta?.publicResult } : {}),
      child: {
        ...(
          previous.meta?.child && typeof previous.meta.child === 'object' && !Array.isArray(previous.meta.child)
            ? previous.meta.child
            : {}
        ),
        ...(
          next.meta?.child && typeof next.meta.child === 'object' && !Array.isArray(next.meta.child)
            ? next.meta.child
            : {}
        )
      }
    }
  }
}


export function hasProcessError(block: ChatBlock): boolean {
  return ('status' in block && block.status === 'error') || (block.kind === 'system' && block.severity === 'error')
}

import type { LucideIcon } from '../../../../design/AnalytixUiIcons'
import { Ban, Bot, BookOpen, FolderOpen, ListTodo, MessageSquareQuote, Minimize2, PencilLine, Search, Terminal, Wrench } from '../../../../design/AnalytixUiIcons'
import { sanitizePublicSerializedText } from '@shared/public-runtime-content'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import { redactSecretText } from '@shared/secret-redaction'
import { extractUnifiedDiffText, countDiffStats } from '../../../../lib/diff-stats'
import type { RuntimeDiagnosticsStatusFilter } from '../../../../lib/runtime-diagnostics-focus'
import { splitThink } from '../../../../thread/projection/thread-turns'
import { formatBytes, formatDuration, formatToolTitle, formatJobHeartbeatStatus } from '../../message-timeline-tools'

// Bounded public summaries and metadata are business projection, independent of JSX.
function processBlockHasRuntimeMetaDetail(block: ChatBlock): boolean {
  return (block.kind === 'tool' || block.kind === 'system') && runtimeMetaDetailSections(block, undefined).length > 0
}

export function hasRuntimeDiagnosticsSignal(diagnostics: Record<string, unknown>): boolean {
  return Boolean(
    readRuntimeString(diagnostics, 'jobId') ||
    readRuntimeString(diagnostics, 'childRunId') ||
    readRuntimeString(diagnostics, 'deliveryId') ||
    readRuntimeString(diagnostics, 'childThreadId') ||
    readRuntimeString(diagnostics, 'parentThreadId') ||
    readRuntimeString(diagnostics, 'heartbeatStatus') ||
    readRuntimeString(diagnostics, 'recoveryStatus') ||
    readRuntimeString(diagnostics, 'autoContinueStatus') ||
    readRuntimeString(diagnostics, 'deliveryStatus')
  )
}

export function runtimeDiagnosticsStatusForFocus(
  diagnostics: Record<string, unknown>
): RuntimeDiagnosticsStatusFilter | undefined {
  const notificationKind = readRuntimeString(diagnostics, 'notificationKind')
  const autoStatus = readRuntimeString(diagnostics, 'autoContinueStatus')
  if (notificationKind === 'background_job_auto_continue' && autoStatus) {
    if (autoStatus === 'started') return 'running'
    if (autoStatus === 'skipped') return 'skipped'
    if (autoStatus === 'failed') return 'dead_lettered'
  }
  const deliveryStatus = readRuntimeString(diagnostics, 'deliveryStatus')
  if (deliveryStatus === 'pending') return 'delivery_pending'
  if (deliveryStatus === 'retry') return 'retrying'
  if (deliveryStatus === 'delivered') return 'delivered'
  if (deliveryStatus === 'skipped') return 'skipped'
  if (deliveryStatus === 'dead_letter' || deliveryStatus === 'dead_lettered') return 'dead_letter'
  const heartbeatStatus = readRuntimeString(diagnostics, 'heartbeatStatus')
  if (
    heartbeatStatus === 'running' ||
    heartbeatStatus === 'stale' ||
    heartbeatStatus === 'lease_expired' ||
    heartbeatStatus === 'orphaned' ||
    heartbeatStatus === 'recovering' ||
    heartbeatStatus === 'recovered' ||
    heartbeatStatus === 'dead_lettered' ||
    heartbeatStatus === 'paused'
  ) {
    return heartbeatStatus
  }
  const recoveryStatus = readRuntimeString(diagnostics, 'recoveryStatus')
  if (recoveryStatus === 'recovering' || recoveryStatus === 'recovered' || recoveryStatus === 'dead_lettered') {
    return recoveryStatus
  }
  if (readRuntimeBoolean(diagnostics, 'leaseExpired')) return 'lease_expired'
  if (readRuntimeBoolean(diagnostics, 'orphaned')) return 'orphaned'
  if (readRuntimeBoolean(diagnostics, 'stalled')) return 'stale'
  return undefined
}

function executionToolName(block: ToolBlock): string {
  const rawSummary = block.summary?.trim() ?? ''
  return (readMetaString(block.meta, 'toolName') || extractToolName(rawSummary) || '').toLowerCase()
}

export function processBlockIcon(block: ChatBlock): LucideIcon | null {
  if (block.kind === 'assistant') return MessageSquareQuote
  if (block.kind === 'compaction') return Minimize2
  if (block.kind === 'approval') return Wrench
  if (block.kind === 'user_input') return MessageSquareQuote
  if (isBackgroundShellProcessBlock(block)) return Terminal
  if (block.kind !== 'tool') return null
  if (isAutoContinueProcessBlock(block)) {
    const diagnostics = jobDiagnosticsFromMeta(block.meta)
    const status = readRuntimeString(diagnostics, 'autoContinueStatus') || readRuntimeString(diagnostics, 'status')
    return status === 'skipped' || status === 'failed' ? Ban : Bot
  }
  if (isDeliveryLedgerProcessBlock(block)) {
    const diagnostics = jobDiagnosticsFromMeta(block.meta)
    const status = readRuntimeString(diagnostics, 'deliveryStatus') || readRuntimeString(diagnostics, 'status')
    return status === 'skipped' || status === 'dead_letter' || status === 'dead_lettered' ? Ban : Bot
  }
  return toolBlockIcon(block)
}

function toolBlockIcon(block: ToolBlock): LucideIcon {
  const toolName = executionToolName(block).toLowerCase()
  switch (toolName) {
    case 'bash':
    case 'shell':
    case 'terminal':
    case 'run_command':
    case 'exec':
      return Terminal
    case 'read':
    case 'read_file':
      return BookOpen
    case 'write':
    case 'write_file':
    case 'edit':
    case 'edit_file':
    case 'apply_patch':
    case 'create_file':
      return PencilLine
    case 'grep':
    case 'grep_files':
    case 'search':
    case 'search_files':
    case 'find':
      return Search
    case 'ls':
    case 'list':
    case 'list_dir':
      return FolderOpen
    case 'create_plan':
    case 'update_plan':
      return ListTodo
    default:
      break
  }

  if (block.toolKind === 'command_execution') return Terminal
  if (block.toolKind === 'file_change') return PencilLine
  return Wrench
}

export function toolFilePath(block: ToolBlock): string | undefined {
  const sourceText = [block.summary, block.detail ?? ''].filter(Boolean).join('\n')
  return (
    block.filePath ||
    extractQuotedField(sourceText, 'path') ||
    extractQuotedField(sourceText, 'file_path') ||
    extractQuotedField(sourceText, 'file')
  )
}

export type ProcessDetail =
  | { kind: 'none' }
  | { kind: 'assistant'; text: string }
  | { kind: 'tool'; text: string; isPatch: boolean; isError: boolean; filePath?: string }
  | { kind: 'runtime_meta' }
  | { kind: 'approval' }
  | { kind: 'user_input' }
  | { kind: 'text'; text: string }

type RuntimeMetaDetailSection = {
  id: string
  title: string
  rows: Array<{ label: string; value: string }>
}

function summarizeProcessText(text: string, max = 96): string {
  const oneLine = text.replace(/\s+/g, ' ').trim()
  if (!oneLine) return ''
  if (oneLine.length <= max) return oneLine
  return `${oneLine.slice(0, max - 1).trimEnd()}…`
}

function humanizeToolName(name: string): string {
  const trimmed = name.trim().replace(/[_-]+/g, ' ')
  if (!trimmed) return ''
  return trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
}

function builtInToolLabel(
  toolName: string,
  t: (key: string, opts?: Record<string, unknown>) => string
): string | undefined {
  switch (toolName) {
    case 'read':
    case 'read_file':
      return t('toolBuiltinRead')
    case 'write':
    case 'write_file':
      return t('toolBuiltinWrite')
    case 'edit':
    case 'edit_file':
    case 'multi_edit':
      return t('toolBuiltinEdit')
    case 'grep':
    case 'grep_files':
    case 'search_files':
      return t('toolBuiltinGrep')
    case 'find':
      return t('toolBuiltinFind')
    case 'ls':
      return t('toolBuiltinLs')
    case 'bash':
    case 'shell':
    case 'terminal':
    case 'run_command':
    case 'exec':
      // Logical bash tools may execute zsh/sh, PowerShell or cmd. No public
      // execution identity currently proves a Bash interpreter.
      return t('dshToolFamily.shell')
    case 'powershell':
    case 'pwsh':
      return 'PowerShell'
    case 'apply_patch':
      return t('toolBuiltinPatch')
    default:
      return undefined
  }
}

export function publicToolOperationTitle(block: ToolBlock, t: (key: string) => string): string | undefined {
  return builtInToolLabel(executionToolName(block), t)
}

function extractToolName(summary: string): string {
  const match = summary.trim().match(/^([a-z0-9_-]+)\s*:/i)
  return match?.[1] ?? ''
}

function extractQuotedField(text: string, field: string): string | undefined {
  const escaped = field.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const attr = new RegExp(`${escaped}="([^"]+)"`, 'i').exec(text)
  if (attr?.[1]) return attr[1]
  const json = new RegExp(`"${escaped}"\\s*:\\s*"([^"]+)"`, 'i').exec(text)
  if (json?.[1]) return json[1]
  return undefined
}

function readMetaString(meta: Record<string, unknown> | undefined, key: string): string | undefined {
  if (!meta) return undefined
  const value = meta[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

export function readMetaStringArray(meta: Record<string, unknown> | undefined, key: string): string[] {
  const value = meta?.[key]
  if (!Array.isArray(value)) return []
  return value.filter((entry): entry is string => typeof entry === 'string' && entry.trim().length > 0)
}

export function readMetaSources(meta: Record<string, unknown> | undefined): Array<{ title?: string; url?: string }> {
  const value = meta?.sources
  if (!Array.isArray(value)) return []
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') return null
      const raw = entry as Record<string, unknown>
      const title = typeof raw.title === 'string' && raw.title.trim() ? raw.title.trim() : undefined
      const url = publicSourceHref(typeof raw.url === 'string' ? raw.url.trim() : undefined)
      return title || url ? { ...(title ? { title } : {}), ...(url ? { url } : {}) } : null
    })
    .filter((entry): entry is { title?: string; url?: string } => entry !== null)
}

function readRuntimeRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

export function readRuntimeString(record: Record<string, unknown> | null, key: string): string {
  const value = record?.[key]
  return typeof value === 'string' && value.trim()
    ? redactSecretText(value.trim())
    : ''
}

function readRuntimeNumber(record: Record<string, unknown> | null, key: string): number | undefined {
  const value = record?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function readRuntimeBoolean(record: Record<string, unknown> | null, key: string): boolean {
  return record?.[key] === true
}

export function jobDiagnosticsFromMeta(meta: Record<string, unknown> | undefined): Record<string, unknown> | null {
  return readRuntimeRecord(meta?.diagnostics)
}

function jobNotificationKind(block: ChatBlock): string {
  if (block.kind !== 'tool' && block.kind !== 'system') return ''
  return readRuntimeString(jobDiagnosticsFromMeta(block.meta), 'notificationKind')
}

function isAutoContinueProcessBlock(block: ChatBlock): boolean {
  return jobNotificationKind(block) === 'background_job_auto_continue'
}

function isDeliveryLedgerProcessBlock(block: ChatBlock): boolean {
  return jobNotificationKind(block) === 'background_job_delivery'
}

function formatRuntimePercent(value: number | undefined): string {
  if (value === undefined) return ''
  const normalized = value <= 1 ? value * 100 : value
  return `${Math.max(0, Math.min(100, Math.round(normalized)))}%`
}

export function runtimeMetaDetailSections(
  block: ChatBlock,
  t: ((key: string, opts?: Record<string, unknown>) => string) | undefined
): RuntimeMetaDetailSection[] {
  if (block.kind !== 'tool' && block.kind !== 'system') return []
  const translate = (key: string): string => t?.(key) ?? key
  const meta = block.meta
  const child = readRuntimeRecord(meta?.child)
  const diagnostics = readRuntimeRecord(meta?.diagnostics)
  const sections: RuntimeMetaDetailSection[] = []

  if (child) {
    const durationMs = readRuntimeNumber(child, 'durationMs')
    const queuedMs = readRuntimeNumber(child, 'queuedMs')
    const tokenBudget = readRuntimeNumber(child, 'tokenBudget')
    const timeBudgetMs = readRuntimeNumber(child, 'timeBudgetMs')
    const evidenceStatus = readRuntimeString(child, 'evidenceBundleStatus')
    const evidenceCount = readRuntimeNumber(child, 'evidenceCount')
    const changedFileCount = readRuntimeNumber(child, 'changedFileCount')
    const rows = [
      { label: translate('toolChildLabel'), value: readRuntimeString(child, 'childLabel') },
      { label: translate('toolChildStatus'), value: readRuntimeString(child, 'childStatus') },
      { label: translate('toolChildId'), value: readRuntimeString(child, 'childId') },
      { label: translate('toolChildJob'), value: readRuntimeString(child, 'jobId') },
      { label: translate('toolChildRun'), value: readRuntimeString(child, 'childRunId') },
      { label: translate('toolChildThread'), value: readRuntimeString(child, 'childThreadId') },
      { label: translate('toolChildTurn'), value: readRuntimeString(child, 'childTurnId') },
      {
        label: translate('toolChildParent'),
        value: [
          readRuntimeString(child, 'parentThreadId'),
          readRuntimeString(child, 'parentTurnId'),
          readRuntimeString(child, 'parentToolCallId')
        ].filter(Boolean).join(' · ')
      },
      { label: translate('toolChildProfile'), value: readRuntimeString(child, 'childProfile') },
      { label: translate('toolChildProfileMode'), value: readRuntimeString(child, 'childProfileMode') },
      { label: translate('toolChildProfileDescription'), value: readRuntimeString(child, 'childProfileDescription') },
      {
        label: translate('toolChildEffort'),
        value: readRuntimeString(child, 'childEffort') || readRuntimeString(child, 'effort')
      },
      { label: translate('toolChildToolPolicy'), value: readRuntimeString(child, 'childToolPolicy') },
      { label: translate('toolChildReturnFormat'), value: readRuntimeString(child, 'returnFormat') },
      {
        label: translate('toolChildTokenBudget'),
        value: tokenBudget !== undefined ? String(tokenBudget) : ''
      },
      {
        label: translate('toolChildTimeBudget'),
        value: timeBudgetMs !== undefined ? formatDuration(timeBudgetMs) : ''
      },
      {
        label: translate('toolChildBudgetExceeded'),
        value: readRuntimeBoolean(child, 'budgetExceeded') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolChildEvidence'),
        value: evidenceStatus
          ? `${evidenceStatus}${evidenceCount !== undefined ? ` (${evidenceCount})` : ''}`
          : ''
      },
      {
        label: translate('toolChildParallel'),
        value: [
          readRuntimeNumber(child, 'parallelIndex') !== undefined
            ? String(readRuntimeNumber(child, 'parallelIndex'))
            : '',
          readRuntimeString(child, 'parallelGroupId')
        ].filter(Boolean).join(' · ')
      },
      {
        label: translate('toolChildSeq'),
        value: readRuntimeNumber(child, 'childSeq') !== undefined
          ? `#${readRuntimeNumber(child, 'childSeq')}`
          : ''
      },
      {
        label: translate('toolChildTools'),
        value: readRuntimeNumber(child, 'toolInvocations') !== undefined
          ? String(readRuntimeNumber(child, 'toolInvocations'))
          : ''
      },
      {
        label: translate('toolChildDuration'),
        value: durationMs !== undefined && durationMs > 0 ? formatDuration(durationMs) : ''
      },
      {
        label: translate('toolChildQueued'),
        value: queuedMs !== undefined && queuedMs > 0 ? formatDuration(queuedMs) : ''
      },
      {
        label: translate('toolChildTokens'),
        value: readRuntimeNumber(child, 'totalTokens') !== undefined
          ? String(readRuntimeNumber(child, 'totalTokens'))
          : ''
      },
      { label: translate('toolChildCache'), value: formatRuntimePercent(readRuntimeNumber(child, 'cacheHitRate')) },
      {
        label: translate('toolChildBackground'),
        value: readRuntimeBoolean(child, 'background') ? translate('toolMetaYes') : ''
      },
      { label: translate('toolChildWorktree'), value: readRuntimeString(child, 'worktreeBranch') },
      { label: translate('toolChildWorktreePath'), value: readRuntimeString(child, 'worktreePath') },
      { label: translate('toolChildBaseCommit'), value: readRuntimeString(child, 'baseCommit') },
      { label: translate('toolChildCurrentCommit'), value: readRuntimeString(child, 'currentCommit') },
      {
        label: translate('toolChildChangedFiles'),
        value: changedFileCount !== undefined ? String(changedFileCount) : ''
      },
      { label: translate('toolChildMergeStatus'), value: readRuntimeString(child, 'mergeStatus') }
    ].filter((row) => row.value)

    if (rows.length > 0) {
      sections.push({ id: 'child', title: translate('toolChildAgent'), rows })
    }
  }

  if (diagnostics) {
    const idleMs = readRuntimeNumber(diagnostics, 'idleMs')
    const heartbeatAgeMs = readRuntimeNumber(diagnostics, 'heartbeatAgeMs')
    const staleAfterMs = readRuntimeNumber(diagnostics, 'staleAfterMs') ?? readRuntimeNumber(diagnostics, 'stalledAfterMs')
    const outputBytes = readRuntimeNumber(diagnostics, 'outputBytes')
    const nextOffset = readRuntimeNumber(diagnostics, 'nextOffset')
    const rows = [
      { label: translate('toolJobNotification'), value: readRuntimeString(diagnostics, 'notificationKind') },
      { label: translate('toolChildJob'), value: readRuntimeString(diagnostics, 'jobId') },
      { label: translate('toolChildRun'), value: readRuntimeString(diagnostics, 'childRunId') },
      { label: translate('toolChildThread'), value: readRuntimeString(diagnostics, 'childThreadId') },
      { label: translate('toolChildTurn'), value: readRuntimeString(diagnostics, 'childTurnId') },
      { label: translate('toolParentThread'), value: readRuntimeString(diagnostics, 'parentThreadId') },
      { label: translate('toolParentTurn'), value: readRuntimeString(diagnostics, 'parentTurnId') },
      { label: translate('toolJobStatus'), value: readRuntimeString(diagnostics, 'status') },
      { label: translate('toolJobHeartbeatStatus'), value: formatJobHeartbeatStatus(readRuntimeString(diagnostics, 'heartbeatStatus'), translate) },
      { label: translate('toolAutoContinueStatus'), value: readRuntimeString(diagnostics, 'autoContinueStatus') },
      { label: translate('toolAutoContinueTurn'), value: readRuntimeString(diagnostics, 'autoContinueTurnId') },
      { label: translate('toolDeliveryId'), value: readRuntimeString(diagnostics, 'deliveryId') },
      { label: translate('toolDeliveryStatus'), value: readRuntimeString(diagnostics, 'deliveryStatus') },
      { label: translate('toolDeliveryItem'), value: readRuntimeString(diagnostics, 'deliveryItemId') },
      {
        label: translate('toolDeliveryAttempt'),
        value: readRuntimeNumber(diagnostics, 'completionDeliveryAttempt') !== undefined
          ? String(readRuntimeNumber(diagnostics, 'completionDeliveryAttempt'))
          : ''
      },
      { label: translate('toolDeliveryAt'), value: readRuntimeString(diagnostics, 'completionDeliveryAt') },
      { label: translate('toolDeliveryDeadLetterAt'), value: readRuntimeString(diagnostics, 'completionDeadLetterAt') },
      { label: translate('toolJobWarningCode'), value: readRuntimeString(diagnostics, 'warningCode') },
      { label: translate('toolJobLastHeartbeat'), value: readRuntimeString(diagnostics, 'lastHeartbeatAt') },
      { label: translate('toolJobLeaseOwner'), value: readRuntimeString(diagnostics, 'leaseOwner') },
      { label: translate('toolJobLeaseExpires'), value: readRuntimeString(diagnostics, 'leaseExpiresAt') },
      {
        label: translate('toolJobStaleAfter'),
        value: staleAfterMs !== undefined ? formatDuration(staleAfterMs) : ''
      },
      { label: translate('toolJobRecoveryStatus'), value: readRuntimeString(diagnostics, 'recoveryStatus') },
      {
        label: translate('toolJobRecoveryAttempt'),
        value: readRuntimeNumber(diagnostics, 'recoveryAttempt') !== undefined
          ? String(readRuntimeNumber(diagnostics, 'recoveryAttempt'))
          : ''
      },
      { label: translate('toolJobRecoveryUpdated'), value: readRuntimeString(diagnostics, 'recoveryUpdatedAt') },
      {
        label: translate('toolJobIdle'),
        value: idleMs !== undefined ? formatDuration(idleMs) : ''
      },
      {
        label: translate('toolJobHeartbeat'),
        value: [
          readRuntimeString(diagnostics, 'heartbeatAt'),
          heartbeatAgeMs !== undefined ? formatDuration(heartbeatAgeMs) : ''
        ].filter(Boolean).join(' · ')
      },
      {
        label: translate('toolJobOutput'),
        value: outputBytes !== undefined ? formatBytes(outputBytes) : ''
      },
      {
        label: translate('toolJobNextOffset'),
        value: nextOffset !== undefined ? formatBytes(nextOffset) : ''
      },
      {
        label: translate('toolJobStalled'),
        value: readRuntimeBoolean(diagnostics, 'stalled') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolJobLeaseExpired'),
        value: readRuntimeBoolean(diagnostics, 'leaseExpired') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolJobOrphaned'),
        value: readRuntimeBoolean(diagnostics, 'orphaned') ? translate('toolMetaYes') : ''
      },
      {
        label: translate('toolChildBackground'),
        value: readRuntimeBoolean(diagnostics, 'background') ? translate('toolMetaYes') : ''
      },
    ].filter((row) => row.value)

    if (rows.length > 0) {
      sections.push({ id: 'job', title: translate('toolJobDiagnostics'), rows })
    }
  }

  return sections
}

export function summarizeToolBlock(
  block: ToolBlock,
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  const diagnostics = jobDiagnosticsFromMeta(block.meta)
  if (readRuntimeString(diagnostics, 'notificationKind') === 'background_job_auto_continue') {
    const status = readRuntimeString(diagnostics, 'autoContinueStatus') || readRuntimeString(diagnostics, 'status')
    if (status === 'started') {
      const turnId = readRuntimeString(diagnostics, 'autoContinueTurnId')
      return turnId
        ? t('backgroundAutoContinueStartedWithTurn', { turnId })
        : t('backgroundAutoContinueStarted')
    }
    if (status === 'skipped') {
      return t('backgroundAutoContinueSkipped')
    }
    if (status === 'failed') {
      return t('backgroundAutoContinueFailed')
    }
    return t('backgroundAutoContinueStatus', { status: status || 'updated' })
  }
  if (readRuntimeString(diagnostics, 'notificationKind') === 'background_job_delivery') {
    const status = readRuntimeString(diagnostics, 'deliveryStatus') || readRuntimeString(diagnostics, 'status')
    switch (status) {
      case 'pending':
        return t('backgroundDeliveryPending')
      case 'retry':
        return t('backgroundDeliveryRetry')
      case 'delivered':
        return t('backgroundDeliveryDelivered')
      case 'skipped':
        return t('backgroundDeliverySkipped')
      case 'dead_letter':
      case 'dead_lettered':
        return t('backgroundDeliveryDeadLetter')
      default:
        return t('backgroundDeliveryStatus', { status: status || 'updated' })
    }
  }
  const publicText = (text: string) => projectOrdinaryPublicText(sanitizePublicSerializedText(text))
  // Child detail has its own bounded error projection; raw child diagnostics
  // are not an ordinary public failure line.
  const isError = block.status === 'error' && !isSubagentProcessBlock(block)
  // Only already-published closed detail is eligible. Project the full text
  // before selecting the literal first line; never request protected output here.
  const detailText = isError ? publicText(block.detail ?? '') : block.detail ?? ''
  if (isError && detailText !== '') {
    const newline = detailText.indexOf('\n')
    return newline === -1 ? detailText : detailText.slice(0, newline)
  }
  const rawSummary = (isError ? publicText(block.summary ?? '') : block.summary ?? '').trim()
  const toolName = executionToolName({ ...block, summary: rawSummary })
  const label = block.toolKind === 'subagent'
    ? formatToolTitle(block, t)
    : builtInToolLabel(toolName, t) || humanizeToolName(toolName) || formatToolTitle(block, t)
  const sourceText = [rawSummary, detailText].filter(Boolean).join('\n')
  const rawPath = toolFilePath({ ...block, summary: rawSummary, detail: detailText })
  const filePath = isError && rawPath ? publicText(rawPath) : rawPath
  const pattern =
    extractQuotedField(sourceText, 'pattern') ||
    extractQuotedField(sourceText, 'query') ||
    readMetaString(block.meta, 'pattern')
  const command = readMetaString(block.meta, 'command')

  if ((toolName === 'read_file' || toolName === 'read') && filePath) {
    return `${label} ${filePath}`
  }
  if ((toolName === 'write' || toolName === 'edit' || toolName === 'write_file' || toolName === 'edit_file' || toolName === 'multi_edit') && filePath) {
    return `${label} ${filePath}`
  }
  if ((toolName === 'grep_files' || toolName === 'search_files' || toolName === 'grep' || toolName === 'find') && pattern) {
    return filePath ? `${label} ${pattern} · ${filePath}` : `${label} ${pattern}`
  }
  if (toolName === 'ls' && filePath) {
    return `${label} ${filePath}`
  }
  if (command && block.toolKind === 'command_execution') {
    return `${formatToolTitle(block, t)} ${summarizeProcessText(command, 72)}`
  }
  if (filePath) {
    return `${label} ${filePath}`
  }
  if (pattern) {
    return `${label} ${pattern}`
  }
  if (rawSummary) {
    const compact = toolName ? rawSummary.replace(/^([a-z0-9_-]+)\s*:\s*/i, '') : rawSummary
    const summary = summarizeProcessText(compact, 72)
    return summary ? `${label} ${summary}` : label
  }
  return label
}

function normalizeProcessText(text: string): string {
  return text.replace(/\s+/g, ' ').trim().toLowerCase()
}

export function getProcessDetail(block: ChatBlock, summaryText?: string): ProcessDetail {
  if (block.kind === 'assistant') {
    const split = splitThink(block.text)
    const text = split.content
    return text.trim() ? { kind: 'assistant', text } : { kind: 'none' }
  }
  if (block.kind === 'tool') {
    const detailText = block.detail?.trim() ?? ''
    if (!detailText) {
      return processBlockHasRuntimeMetaDetail(block) ? { kind: 'runtime_meta' } : { kind: 'none' }
    }
    if (summaryText && normalizeProcessText(detailText) === normalizeProcessText(summaryText)) {
      return processBlockHasRuntimeMetaDetail(block) ? { kind: 'runtime_meta' } : { kind: 'none' }
    }
    const isError = block.status === 'error'
    const patchText =
      block.toolKind === 'file_change' && !isError
        ? extractUnifiedDiffText(detailText)
        : undefined
    return {
      kind: 'tool',
      text: patchText ?? block.detail!,
      isPatch: patchText !== undefined,
      isError,
      filePath: block.filePath
    }
  }
  if (block.kind === 'compaction') {
    const detailText = block.detail?.trim() ?? ''
    if (!detailText) return { kind: 'none' }
    if (summaryText && normalizeProcessText(detailText) === normalizeProcessText(summaryText)) {
      return { kind: 'none' }
    }
    return { kind: 'text', text: detailText }
  }
  if (block.kind === 'approval') return { kind: 'approval' }
  if (block.kind === 'user_input') return { kind: 'user_input' }
  if (block.kind === 'system' && block.text.trim()) {
    if (processBlockHasRuntimeMetaDetail(block)) return { kind: 'runtime_meta' }
    if (block.detail?.trim()) return { kind: 'text', text: block.detail }
    // Short system messages already fit in the summary line — skip the
    // expand affordance so we don't duplicate the same string.
    if (block.text.length <= 140) return { kind: 'none' }
    return { kind: 'text', text: block.text }
  }
  return { kind: 'none' }
}

export function describeProcessBlock(
  block: ChatBlock,
  t: (key: string, opts?: Record<string, unknown>) => string
): string {
  if (block.kind === 'assistant') {
    return t('processTextLabel')
  }
  if (block.kind === 'tool') {
    return summarizeToolBlock(block, t)
  }
  if (block.kind === 'compaction') {
    if (block.status === 'running') return t('compactionRunning')
    if (block.status === 'error') return block.summary || t('compactionFailed')
    if (typeof block.messagesBefore === 'number' && typeof block.messagesAfter === 'number') {
      return t('compactionCompletedWithCounts', {
        before: block.messagesBefore,
        after: block.messagesAfter
      })
    }
    return block.auto === true ? t('compactionAutoCompleted') : t('compactionManualCompleted')
  }
  if (block.kind === 'approval') {
    return block.summary || t('approvalTitle')
  }
  if (block.kind === 'user_input') {
    return t('userInputTitle')
  }
  if (block.kind === 'system') {
    return block.text
  }
  return 'text' in block ? block.text : t('processed')
}

// Count only complete unified hunks. Normalized markers let the shared counter
// distinguish content such as `+++literal` from actual file headers.
export function countPublicPatchStats(patch: string) {
  const changes: string[] = []
  let oldRemaining = 0, newRemaining = 0
  for (const line of patch.split('\n')) {
    const hunk = /^@@ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@/.exec(line)
    if (hunk) {
      if (oldRemaining || newRemaining) return undefined
      oldRemaining = Number(hunk[1] ?? 1); newRemaining = Number(hunk[2] ?? 1)
      if (!Number.isSafeInteger(oldRemaining) || !Number.isSafeInteger(newRemaining)) return undefined
      continue
    }
    if (!oldRemaining && !newRemaining) continue
    if (line.startsWith('\\ No newline at end of file')) continue
    if (line.startsWith('+') && newRemaining > 0) { newRemaining--; changes.push('+x') }
    else if (line.startsWith('-') && oldRemaining > 0) { oldRemaining--; changes.push('-x') }
    else if (line.startsWith(' ') && oldRemaining > 0 && newRemaining > 0) { oldRemaining--; newRemaining-- }
    else return undefined
  }
  if (oldRemaining || newRemaining) return undefined
  return countDiffStats(changes.join('\n')) ?? undefined
}


function publicSourceHref(value: string | undefined): string | undefined {
  if (!value || !URL.canParse(value)) return undefined
  return ['http:', 'https:'].includes(new URL(value).protocol) ? value : undefined
}

// Pure distinct-call ranking and title composition adapted from the pinned
// process-activity.ts / step-process.ts. This never associates private payloads.
export type ProcessActivity = 'read' | 'readImage' | 'search' | 'write' | 'edit' | 'commands' | 'code'
  | 'webSearch' | 'webFetch' | 'subagents' | 'plan' | 'questions' | 'tools'
export type ProcessActivityCount = { kind: ProcessActivity; count: number }
export function rankedProcessActivities(blocks: readonly ChatBlock[], classify: (block: ToolBlock) => ProcessActivity): ProcessActivityCount[] {
  const counts = new Map<ProcessActivity, number>(), seenCalls = new Set<string>(), seenRows = new Set<string>()
  for (const block of blocks) {
    if (block.kind !== 'tool') continue
    const callId = block.meta?.callId
    if (typeof callId === 'string' && callId !== '') {
      if (seenCalls.has(callId)) continue
      seenCalls.add(callId)
    } else {
      if (seenRows.has(block.id)) continue
      seenRows.add(block.id)
    }
    const kind = classify(block)
    counts.set(kind, (counts.get(kind) ?? 0) + 1)
  }
  // Stable sorting preserves the actual first category occurrence for ties.
  return [...counts].map(([kind, count]) => ({ kind, count })).sort((a, b) => b.count - a.count)
}
export function processTitle(counts: readonly ProcessActivityCount[], t: (key: string, options?: Record<string, unknown>) => string): string {
  const labels = counts.slice(0, 3).map(({ kind }) => t('dshStepProcess.done.' + kind)), first = labels[0]
  if (first === undefined) return t('dshStepProcess.done.thinking')
  const continuation = (label: string) => label.charAt(0).toLowerCase() + label.slice(1)
  const second = labels[1]
  if (second === undefined) return first
  if (labels.length === 2) {
    const prefix = t('dshStepProcess.sharedPrefix')
    const shared = prefix !== '' && first.startsWith(prefix) && second.startsWith(prefix)
    return t('dshStepProcess.joinTwo', { first, second: continuation(shared ? second.slice(prefix.length) : second) })
  }
  const title = [first, ...labels.slice(1).map(continuation)].join(t('dshStepProcess.comma'))
  return counts.length > 3 ? t('dshStepProcess.more', { title }) : title
}
