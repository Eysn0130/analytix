import type { ReactElement, ReactNode } from 'react'
import { useEffect, useMemo, useState } from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  AlertCircle,
  Bell,
  CheckCircle2,
  ChevronDown,
  CircleDot,
  ExternalLink,
  Filter,
  Loader2,
  Pause,
  Play,
  RotateCcw,
  SendHorizontal,
  Square
} from 'lucide-react'
import type {
  CoreThreadSummaryResponseJson,
  CoreThreadSummarySubagentJson,
  CoreThreadSummaryTaskJson
} from '../../agent/analytix-contract'
import type { ChatBlock, RuntimeJobDiagnosticsMetadata } from '../../agent/types'
import type {
  RuntimeDiagnosticsFocus,
  RuntimeDiagnosticsStatusFilter
} from '../../lib/runtime-diagnostics-focus'
import { subagentDisplayLabel } from './SubagentSummaryRows'

export type RuntimeDiagnosticEntryTone = 'default' | 'running' | 'success' | 'warning' | 'error'

export type RuntimeDiagnosticAction =
  | 'pause'
  | 'resume'
  | 'steer'
  | 'kill'
  | 'restart'
  | 'recover'

export type RuntimeDiagnosticActionPayload = {
  message?: string
}

export type RuntimeDiagnosticEntryFields = {
  jobId?: string
  deliveryId?: string
  childThreadId?: string
  childTurnId?: string
  parentThreadId?: string
  parentTurnId?: string
  sourceBlockId?: string
  statusKey?: RuntimeDiagnosticsStatusFilter
  runtimeStatus?: string
}

export type RuntimeDiagnosticEntry = {
  key: string
  title: string
  sourceLabel: string
  statusLabel: string
  tone: RuntimeDiagnosticEntryTone
  sortAt: string
  fields: RuntimeDiagnosticEntryFields
  diagnostics: RuntimeDiagnosticRecord
  detailRows: Array<{ label: string; value: string }>
}

type RuntimeDiagnosticRecord = RuntimeJobDiagnosticsMetadata & Record<string, unknown>

const RUNTIME_DIAGNOSTIC_STATUS_OPTIONS: Array<{ value: RuntimeDiagnosticsStatusFilter; label: string }> = [
  { value: 'all', label: '全部状态' },
  { value: 'running', label: 'running / 运行中' },
  { value: 'stale', label: 'stale / 可能停滞' },
  { value: 'lease_expired', label: 'lease_expired / 租约过期' },
  { value: 'orphaned', label: 'orphaned / 孤儿任务' },
  { value: 'recovering', label: 'recovering / 恢复中' },
  { value: 'recovered', label: 'recovered / 已恢复' },
  { value: 'dead_lettered', label: 'dead_lettered / 无法恢复' },
  { value: 'delivery_pending', label: 'delivery pending / 待投递' },
  { value: 'delivered', label: 'delivered / 已投递' },
  { value: 'skipped', label: 'skipped / 已跳过' },
  { value: 'retrying', label: 'retrying / 补投递' },
  { value: 'dead_letter', label: 'dead-letter / 投递死信' },
  { value: 'paused', label: 'paused / 已暂停' }
]

const DIAGNOSTIC_DETAIL_FIELDS: Array<[keyof RuntimeJobDiagnosticsMetadata | string, string]> = [
  ['notificationKind', '通知'],
  ['jobId', 'jobId'],
  ['childRunId', 'childRunId'],
  ['childThreadId', 'childThreadId'],
  ['childTurnId', 'childTurnId'],
  ['parentThreadId', 'parentThreadId'],
  ['parentTurnId', 'parentTurnId'],
  ['deliveryId', 'deliveryId'],
  ['deliveryStatus', '投递状态'],
  ['deliveryItemId', 'deliveryItemId'],
  ['autoContinueStatus', '自动续写'],
  ['autoContinueTurnId', '续写 turnId'],
  ['heartbeatStatus', '心跳状态'],
  ['heartbeatAgeMs', '心跳延迟'],
  ['staleAfterMs', '停滞阈值'],
  ['leaseExpired', '租约已过期'],
  ['orphaned', '孤儿任务'],
  ['stalled', '停滞'],
  ['recoveryStatus', '恢复状态'],
  ['recoveryAttempt', '恢复次数'],
  ['lateCompletionSuppressed', '旧结果抑制'],
  ['completionDeliveryAttempt', '投递次数'],
  ['warningCode', '警告代码']
]

export function RuntimeDiagnosticsRows({
  entries,
  focus,
  busyEntryKey,
  actionError,
  onAction,
  onOpenThread,
  onJumpToTimeline,
  onFocusHandled
}: {
  entries: RuntimeDiagnosticEntry[]
  focus?: RuntimeDiagnosticsFocus | null
  busyEntryKey?: string | null
  actionError?: string | null
  onAction?: (
    action: RuntimeDiagnosticAction,
    entry: RuntimeDiagnosticEntry,
    payload?: RuntimeDiagnosticActionPayload
  ) => void
  onOpenThread?: (
    threadId: string,
    options?: { title?: string; source?: 'side' | 'background-agent' }
  ) => void
  onJumpToTimeline?: (entry: RuntimeDiagnosticEntry) => void
  onFocusHandled?: () => void
}): ReactElement {
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<RuntimeDiagnosticsStatusFilter>('all')
  const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(() => new Set())

  useEffect(() => {
    if (!focus) return
    const focusQuery = runtimeDiagnosticsFocusQuery(focus)
    if (focusQuery) setQuery(focusQuery)
    if (focus.status) setStatusFilter(focus.status)
    const matchingKeys = entries
      .filter((entry) => entryMatchesFocus(entry, focus))
      .map((entry) => entry.key)
    if (matchingKeys.length > 0) {
      setExpandedKeys((current) => {
        const next = new Set(current)
        for (const key of matchingKeys) next.add(key)
        return next
      })
    }
    onFocusHandled?.()
  }, [entries, focus, onFocusHandled])

  const filteredEntries = useMemo(
    () => entries.filter((entry) => entryMatchesFilters(entry, query, statusFilter)),
    [entries, query, statusFilter]
  )

  if (entries.length === 0) {
    return <div className="py-1 text-base text-ds-faint">暂无运行时诊断</div>
  }
  const clearFilters = (): void => {
    setQuery('')
    setStatusFilter('all')
  }
  return (
    <div className="flex flex-col gap-0.5">
      <div className="mb-1 flex min-w-0 flex-col gap-1 rounded-md border border-ds-border-muted bg-ds-card/55 p-2">
        <div className="flex min-w-0 items-center gap-1.5">
          <Filter className="h-3.5 w-3.5 shrink-0 text-ds-faint" strokeWidth={2} />
          <input
            className="min-w-0 flex-1 rounded-md border border-ds-border/70 bg-ds-card px-2 py-1 text-[12px] leading-5 text-ds-muted outline-none transition placeholder:text-ds-faint focus:border-ds-muted/50"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            aria-label="筛选运行时诊断"
            placeholder="jobId / deliveryId / thread / turn"
          />
        </div>
        <div className="flex min-w-0 items-center gap-1.5">
          <select
            className="min-w-0 flex-1 rounded-md border border-ds-border/70 bg-ds-card px-2 py-1 text-[12px] leading-5 text-ds-muted outline-none transition focus:border-ds-muted/50"
            value={statusFilter}
            onChange={(event) => setStatusFilter(event.target.value as RuntimeDiagnosticsStatusFilter)}
            aria-label="筛选诊断状态"
          >
            {RUNTIME_DIAGNOSTIC_STATUS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
          <button
            type="button"
            className="shrink-0 rounded-md px-2 py-1 text-[12px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
            onClick={clearFilters}
          >
            清除
          </button>
        </div>
        <div className="text-[11px] leading-4 text-ds-faint">
          显示 {filteredEntries.length} / {entries.length}
        </div>
      </div>
      {actionError ? (
        <div className="rounded-md border border-red-500/20 bg-red-500/10 px-2 py-1.5 text-[12px] leading-5 text-red-600 dark:text-red-300">
          {actionError}
        </div>
      ) : null}
      {filteredEntries.length === 0 ? (
        <div className="py-1 text-base text-ds-faint">没有匹配的运行时诊断</div>
      ) : null}
      {filteredEntries.map((entry) => {
        const actions = runtimeDiagnosticAvailableActions(entry)
        const busy = busyEntryKey === entry.key
        const childThreadId = entry.fields.childThreadId
        const parentThreadId = entry.fields.parentThreadId
        const hasLinks = Boolean(
          (childThreadId && onOpenThread) ||
          (parentThreadId && onOpenThread) ||
          (entry.fields.sourceBlockId && onJumpToTimeline)
        )
        const open = expandedKeys.has(entry.key)
        return (
        <details
          key={entry.key}
          open={open}
          onToggle={(event) => {
            const isOpen = event.currentTarget.open
            setExpandedKeys((current) => {
              const next = new Set(current)
              if (isOpen) next.add(entry.key)
              else next.delete(entry.key)
              return next
            })
          }}
          className="group/runtime-diagnostic min-w-0 rounded-sm text-base text-ds-faint"
        >
          <summary className="flex min-h-7 min-w-0 cursor-pointer list-none items-center gap-2 rounded-sm py-1 text-left outline-none transition hover:bg-ds-hover hover:text-ds-ink focus-visible:bg-ds-hover [&::-webkit-details-marker]:hidden">
            <span className={`flex h-5 w-5 shrink-0 items-center justify-center ${diagnosticToneClass(entry.tone)}`}>
              <RuntimeDiagnosticIcon tone={entry.tone} statusLabel={entry.statusLabel} />
            </span>
            <span className="min-w-0 flex-1 truncate font-medium leading-5">
              {entry.title}
            </span>
            <span className="max-w-24 shrink-0 truncate text-[11px] font-medium leading-4 text-ds-faint">
              {entry.statusLabel}
            </span>
            <ChevronDown
              className="h-3.5 w-3.5 shrink-0 opacity-45 transition-transform group-open/runtime-diagnostic:rotate-180"
              strokeWidth={2}
            />
          </summary>
          <dl className="grid min-w-0 grid-cols-[auto,minmax(0,1fr)] gap-x-2 gap-y-1 pb-2 pl-7 pr-1 text-[11px] leading-4 text-ds-faint">
            <dt className="text-ds-muted">来源</dt>
            <dd className="min-w-0 truncate">{entry.sourceLabel}</dd>
            {hasLinks ? (
              <>
                <dt className="text-ds-muted">定位</dt>
                <dd className="flex min-w-0 flex-wrap gap-1">
                  {parentThreadId && onOpenThread ? (
                    <RuntimeDiagnosticMiniButton
                      label="打开父线程"
                      onClick={() => onOpenThread(parentThreadId, { source: 'side' })}
                    >
                      <ExternalLink className="h-3 w-3" strokeWidth={2} />
                      父线程
                    </RuntimeDiagnosticMiniButton>
                  ) : null}
                  {childThreadId && onOpenThread ? (
                    <RuntimeDiagnosticMiniButton
                      label="打开子线程"
                      onClick={() => onOpenThread(childThreadId, {
                        title: entry.title,
                        source: 'background-agent'
                      })}
                    >
                      <ExternalLink className="h-3 w-3" strokeWidth={2} />
                      子线程
                    </RuntimeDiagnosticMiniButton>
                  ) : null}
                  {entry.fields.sourceBlockId && onJumpToTimeline ? (
                    <RuntimeDiagnosticMiniButton
                      label="定位到消息时间线"
                      onClick={() => onJumpToTimeline(entry)}
                    >
                      <CircleDot className="h-3 w-3" strokeWidth={2} />
                      时间线
                    </RuntimeDiagnosticMiniButton>
                  ) : null}
                </dd>
              </>
            ) : null}
            {actions.length > 0 && onAction ? (
              <>
                <dt className="text-ds-muted">操作</dt>
                <dd className="flex min-w-0 flex-wrap gap-1">
                  {actions.map((action) => (
                    <RuntimeDiagnosticActionButton
                      key={`${entry.key}:${action}`}
                      action={action}
                      busy={busy}
                      onClick={(payload) => onAction(action, entry, payload)}
                    />
                  ))}
                </dd>
              </>
            ) : null}
            {entry.detailRows.map((row) => (
              <RuntimeDiagnosticDetailRow key={`${entry.key}:${row.label}:${row.value}`} label={row.label}>
                {row.value}
              </RuntimeDiagnosticDetailRow>
            ))}
          </dl>
        </details>
        )
      })}
    </div>
  )
}

export function buildRuntimeDiagnosticsEntries(
  summary: CoreThreadSummaryResponseJson | null | undefined,
  blocks: ChatBlock[] = []
): RuntimeDiagnosticEntry[] {
  const entries: RuntimeDiagnosticEntry[] = []

  if (summary) {
    for (const agent of summary.subagents) {
      const diagnostics = diagnosticsFromSubagent(agent)
      if (!hasRuntimeDiagnosticSignal(diagnostics, true)) continue
      entries.push(runtimeDiagnosticEntry({
        key: `subagent:${agent.key}`,
        title: subagentDisplayLabel(agent),
        sourceLabel: '子智能体',
        diagnostics,
        sortAt: agent.updatedAt
      }))
    }

    for (const task of summary.tasks) {
      const diagnostics = diagnosticsFromTask(task)
      if (!hasRuntimeDiagnosticSignal(diagnostics, task.kind !== 'command')) continue
      entries.push(runtimeDiagnosticEntry({
        key: `task:${task.id}`,
        title: task.kind === 'command' ? 'Command' : task.kind,
        sourceLabel: '后台任务',
        diagnostics,
        sortAt: summary.generatedAt
      }))
    }
  }

  for (const block of blocks) {
    const diagnostics = diagnosticsFromTimelineBlock(block)
    if (!diagnostics || !hasRuntimeDiagnosticSignal(diagnostics, false)) continue
      entries.push(runtimeDiagnosticEntry({
        key: timelineDiagnosticKey(block, diagnostics),
        title: timelineDiagnosticTitle(block, diagnostics),
        sourceLabel: '消息时间线',
        diagnostics,
        sortAt: block.createdAt ?? '',
        sourceBlockId: block.id
      }))
  }

  return dedupeDiagnostics(entries).sort((a, b) => b.sortAt.localeCompare(a.sortAt))
}

function RuntimeDiagnosticDetailRow({
  label,
  children
}: {
  label: string
  children: ReactNode
}): ReactElement {
  return (
    <>
      <dt className="whitespace-nowrap text-ds-muted">{label}</dt>
      <dd className="min-w-0 break-words font-mono text-[10.5px] text-ds-faint">{children}</dd>
    </>
  )
}

function RuntimeDiagnosticIcon({
  tone,
  statusLabel
}: {
  tone: RuntimeDiagnosticEntryTone
  statusLabel: string
}): ReactElement {
  if (tone === 'error') return <AlertCircle className="h-4 w-4" strokeWidth={2} />
  if (tone === 'warning') return <Bell className="h-4 w-4" strokeWidth={2} />
  if (tone === 'running') return <Loader2 className="h-4 w-4 animate-spin" strokeWidth={2} />
  if (tone === 'success') return <CheckCircle2 className="h-4 w-4" strokeWidth={2} />
  if (/retry|恢复|补投递/i.test(statusLabel)) return <RotateCcw className="h-4 w-4" strokeWidth={2} />
  return <CircleDot className="h-4 w-4" strokeWidth={2} />
}

function RuntimeDiagnosticMiniButton({
  label,
  onClick,
  children
}: {
  label: string
  onClick: () => void
  children: ReactNode
}): ReactElement {
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 rounded-md border border-ds-border-muted bg-ds-card/75 px-1.5 py-0.5 text-[11px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink"
      aria-label={label}
      title={label}
      onClick={(event) => {
        event.stopPropagation()
        onClick()
      }}
    >
      {children}
    </button>
  )
}

function RuntimeDiagnosticActionButton({
  action,
  busy,
  onClick
}: {
  action: RuntimeDiagnosticAction
  busy: boolean
  onClick: (payload?: RuntimeDiagnosticActionPayload) => void
}): ReactElement {
  const label = runtimeDiagnosticActionLabel(action)
  const Icon = runtimeDiagnosticActionIcon(action)
  return (
    <button
      type="button"
      disabled={busy}
      className="inline-flex items-center gap-1 rounded-md border border-ds-border-muted bg-ds-card/75 px-1.5 py-0.5 text-[11px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-50"
      aria-label={label}
      title={label}
      onClick={(event) => {
        event.stopPropagation()
        if (action === 'steer') {
          const message = typeof window !== 'undefined'
            ? window.prompt('追加给后台子智能体的任务')
            : ''
          if (!message?.trim()) return
          onClick({ message: message.trim() })
          return
        }
        onClick()
      }}
    >
      {busy ? (
        <Loader2 className="h-3 w-3 animate-spin" strokeWidth={2} />
      ) : (
        <Icon className="h-3 w-3" strokeWidth={2} />
      )}
      {label}
    </button>
  )
}

function runtimeDiagnosticActionLabel(action: RuntimeDiagnosticAction): string {
  switch (action) {
    case 'pause':
      return '暂停'
    case 'resume':
      return '恢复运行'
    case 'steer':
      return '追加任务'
    case 'kill':
      return '停止'
    case 'restart':
      return '重启'
    case 'recover':
      return '恢复'
    default:
      return action
  }
}

function runtimeDiagnosticActionIcon(action: RuntimeDiagnosticAction): LucideIcon {
  switch (action) {
    case 'pause':
      return Pause
    case 'resume':
      return Play
    case 'steer':
      return SendHorizontal
    case 'kill':
      return Square
    case 'restart':
    case 'recover':
      return RotateCcw
    default:
      return CircleDot
  }
}

function runtimeDiagnosticEntry(input: {
  key: string
  title: string
  sourceLabel: string
  diagnostics: RuntimeDiagnosticRecord
  sortAt: string
  sourceBlockId?: string
}): RuntimeDiagnosticEntry {
  const statusLabel = runtimeDiagnosticStatusLabel(input.diagnostics)
  return {
    key: input.key,
    title: input.title,
    sourceLabel: input.sourceLabel,
    statusLabel,
    tone: runtimeDiagnosticTone(input.diagnostics),
    sortAt: input.sortAt,
    fields: runtimeDiagnosticEntryFields(input.diagnostics, input.sourceBlockId),
    diagnostics: input.diagnostics,
    detailRows: runtimeDiagnosticDetailRows(input.diagnostics)
  }
}

function diagnosticsFromSubagent(agent: CoreThreadSummarySubagentJson): RuntimeDiagnosticRecord {
  return mergeDiagnostics(agent.diagnostics, {
    notificationKind: 'thread_summary_subagent',
    jobId: agent.taskJobId ?? agent.childRunId,
    childRunId: agent.childRunId,
    childThreadId: agent.childThreadId,
    childTurnId: agent.childTurnId,
    parentThreadId: agent.parentThreadId,
    parentTurnId: agent.parentTurnId,
    kind: agent.taskKind,
    status: agent.rawStatus,
    background: agent.background,
  })
}

function diagnosticsFromTask(task: CoreThreadSummaryTaskJson): RuntimeDiagnosticRecord {
  return mergeDiagnostics(undefined, {
    notificationKind: 'thread_summary_task',
    jobId: task.id.replace(/^taskjob:/, ''),
    kind: task.kind,
    status: task.status,
    background: task.background,
    terminal: task.terminal,
    active: task.active
  })
}

function diagnosticsFromTimelineBlock(block: ChatBlock): RuntimeDiagnosticRecord | null {
  if (!('meta' in block)) return null
  const meta = block.meta
  if (!meta || typeof meta !== 'object' || Array.isArray(meta)) return null
  const diagnostics = asRecord((meta as { diagnostics?: unknown }).diagnostics)
  if (!diagnostics) return null
  return mergeDiagnostics(diagnostics, {})
}

function runtimeDiagnosticEntryFields(
  diagnostics: RuntimeDiagnosticRecord,
  sourceBlockId?: string
): RuntimeDiagnosticEntryFields {
  return {
    jobId: readDiagnosticString(diagnostics, 'jobId') || readDiagnosticString(diagnostics, 'childRunId') || undefined,
    deliveryId: readDiagnosticString(diagnostics, 'deliveryId') || undefined,
    childThreadId: readDiagnosticString(diagnostics, 'childThreadId') || undefined,
    childTurnId: readDiagnosticString(diagnostics, 'childTurnId') || undefined,
    parentThreadId: readDiagnosticString(diagnostics, 'parentThreadId') || undefined,
    parentTurnId: readDiagnosticString(diagnostics, 'parentTurnId') || undefined,
    sourceBlockId,
    statusKey: runtimeDiagnosticStatusKey(diagnostics),
    runtimeStatus: readDiagnosticString(diagnostics, 'status') || undefined
  }
}

function runtimeDiagnosticStatusKey(
  diagnostics: RuntimeDiagnosticRecord
): RuntimeDiagnosticsStatusFilter | undefined {
  const notificationKind = readDiagnosticString(diagnostics, 'notificationKind')
  const autoStatus = readDiagnosticString(diagnostics, 'autoContinueStatus')
  if (notificationKind === 'background_job_auto_continue' && autoStatus) {
    if (autoStatus === 'skipped') return 'skipped'
    if (autoStatus === 'failed') return 'dead_lettered'
    if (autoStatus === 'started') return 'running'
  }

  const deliveryStatus = readDiagnosticString(diagnostics, 'deliveryStatus')
  if (deliveryStatus === 'pending') return 'delivery_pending'
  if (deliveryStatus === 'retry') return 'retrying'
  if (deliveryStatus === 'delivered') return 'delivered'
  if (deliveryStatus === 'skipped') return 'skipped'
  if (deliveryStatus === 'dead_letter' || deliveryStatus === 'dead_lettered') return 'dead_letter'

  if (autoStatus === 'skipped') return 'skipped'
  if (autoStatus === 'failed') return 'dead_lettered'
  if (autoStatus === 'started') return 'running'

  const recoveryStatus = readDiagnosticString(diagnostics, 'recoveryStatus')
  if (recoveryStatus === 'recovering') return 'recovering'
  if (recoveryStatus === 'recovered') return 'recovered'
  if (recoveryStatus === 'dead_lettered') return 'dead_lettered'

  const heartbeatStatus = readDiagnosticString(diagnostics, 'heartbeatStatus')
  if (heartbeatStatus === 'running') return 'running'
  if (heartbeatStatus === 'stale') return 'stale'
  if (heartbeatStatus === 'lease_expired') return 'lease_expired'
  if (heartbeatStatus === 'orphaned') return 'orphaned'
  if (heartbeatStatus === 'recovering') return 'recovering'
  if (heartbeatStatus === 'recovered') return 'recovered'
  if (heartbeatStatus === 'dead_lettered') return 'dead_lettered'
  if (heartbeatStatus === 'paused') return 'paused'

  if (diagnostics.leaseExpired === true) return 'lease_expired'
  if (diagnostics.orphaned === true) return 'orphaned'
  if (diagnostics.stalled === true) return 'stale'
  if (diagnostics.paused === true) return 'paused'

  const status = readDiagnosticString(diagnostics, 'status')
  if (status === 'running' || status === 'pending' || status === 'admitted' || status === 'requested') return 'running'
  if (status === 'stale') return 'stale'
  if (status === 'paused') return 'paused'
  if (status === 'lease_expired') return 'lease_expired'
  if (status === 'orphaned') return 'orphaned'
  if (status === 'recovering') return 'recovering'
  if (status === 'recovered') return 'recovered'
  if (status === 'dead_lettered' || status === 'failed') return 'dead_lettered'
  return undefined
}

function runtimeDiagnosticsFocusQuery(focus: RuntimeDiagnosticsFocus): string {
  return [
    focus.jobId,
    focus.deliveryId,
    focus.childThreadId,
    focus.childTurnId,
    focus.parentThreadId,
    focus.parentTurnId,
    focus.sourceBlockId
  ].find((value) => value?.trim())?.trim() ?? ''
}

function entryMatchesFocus(entry: RuntimeDiagnosticEntry, focus: RuntimeDiagnosticsFocus): boolean {
  const keys: Array<keyof RuntimeDiagnosticEntryFields> = [
    'jobId',
    'deliveryId',
    'childThreadId',
    'childTurnId',
    'parentThreadId',
    'parentTurnId',
    'sourceBlockId'
  ]
  return keys.some((key) => {
    const expected = focus[key as keyof RuntimeDiagnosticsFocus]
    if (!expected) return false
    return entry.fields[key] === expected
  })
}

function entryMatchesFilters(
  entry: RuntimeDiagnosticEntry,
  query: string,
  statusFilter: RuntimeDiagnosticsStatusFilter
): boolean {
  if (statusFilter !== 'all' && entry.fields.statusKey !== statusFilter) return false
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  const haystack = [
    entry.key,
    entry.title,
    entry.sourceLabel,
    entry.statusLabel,
    entry.fields.jobId,
    entry.fields.deliveryId,
    entry.fields.childThreadId,
    entry.fields.childTurnId,
    entry.fields.parentThreadId,
    entry.fields.parentTurnId,
    entry.fields.sourceBlockId,
    ...entry.detailRows.flatMap((row) => [row.label, row.value])
  ].filter(Boolean).join('\n').toLowerCase()
  return haystack.includes(normalized)
}

export function runtimeDiagnosticAvailableActions(
  entry: RuntimeDiagnosticEntry
): RuntimeDiagnosticAction[] {
  if (!entry.fields.jobId) return []
  const rawStatus = (entry.fields.runtimeStatus ?? '').toLowerCase()
  if (entry.fields.statusKey === 'dead_lettered' || entry.fields.statusKey === 'dead_letter') {
    return ['recover']
  }
  if (/completed|done|killed|interrupted|superseded|failed/.test(rawStatus)) {
    return []
  }
  switch (entry.fields.statusKey) {
    case 'paused':
      return ['resume', 'kill']
    case 'stale':
    case 'lease_expired':
    case 'orphaned':
      return ['recover', 'restart', 'kill']
    case 'running':
      return ['pause', 'steer', 'kill']
    case 'recovering':
    case 'delivery_pending':
    case 'retrying':
      return ['kill']
    default:
      return []
  }
}

function mergeDiagnostics(
  raw: Record<string, unknown> | undefined,
  base: Record<string, unknown>
): RuntimeDiagnosticRecord {
  const merged: RuntimeDiagnosticRecord = {}
  for (const candidate of [base, raw]) {
    if (!candidate) continue
    for (const key of ['jobId', 'childRunId', 'childThreadId', 'childTurnId', 'parentThreadId', 'parentTurnId', 'autoContinueTurnId', 'deliveryId', 'deliveryItemId', 'pauseRequestId'] as const) {
      const value = candidate[key]
      if (typeof value === 'string' && value.trim()) merged[key] = value.trim()
    }
    for (const [key, allowed] of [
      ['notificationKind', DIAGNOSTIC_NOTIFICATION_KINDS],
      ['kind', DIAGNOSTIC_JOB_KINDS],
      ['status', DIAGNOSTIC_JOB_STATUSES],
      ['heartbeatStatus', DIAGNOSTIC_HEARTBEAT_STATUSES],
      ['autoContinueStatus', DIAGNOSTIC_AUTO_CONTINUE_STATUSES],
      ['deliveryStatus', DIAGNOSTIC_DELIVERY_STATUSES],
      ['recoveryStatus', DIAGNOSTIC_RECOVERY_STATUSES],
      ['pauseStatus', DIAGNOSTIC_PAUSE_STATUSES],
      ['warningCode', DIAGNOSTIC_WARNING_CODES]
    ] as const) {
      const value = closedDiagnosticValue(candidate[key], allowed)
      if (value) (merged as Record<string, unknown>)[key] = value
    }
    for (const key of ['ageMs', 'idleMs', 'heartbeatAgeMs', 'staleAfterMs', 'stalledAfterMs', 'recoveryAttempt', 'completionDeliveryAttempt'] as const) {
      const value = candidate[key]
      if (typeof value === 'number' && Number.isFinite(value)) merged[key] = value
    }
    for (const key of ['terminal', 'background', 'canContinueParent', 'lateCompletionSuppressed', 'autoContinueParent', 'stalled', 'leaseExpired', 'orphaned', 'paused'] as const) {
      const value = candidate[key]
      if (typeof value === 'boolean') merged[key] = value
    }
  }
  return merged
}

const DIAGNOSTIC_NOTIFICATION_KINDS = ['background_job_completion', 'background_job_auto_continue', 'background_job_delivery', 'thread_summary_subagent', 'thread_summary_task'] as const
const DIAGNOSTIC_JOB_KINDS = ['task', 'parallel_task', 'planner', 'background-shell', 'bash', 'subagent', 'child-run', 'parallel-child-run', 'command', 'process', 'unknown'] as const
const DIAGNOSTIC_JOB_STATUSES = ['queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming', 'completed', 'failed', 'aborted', 'interrupted', 'killed', 'canceled', 'timeout', 'starting', 'pending', 'delivered', 'retry', 'recovering', 'recovered', 'dead_letter', 'dead_lettered', 'skipped', 'stopped', 'missing', 'archived', 'unknown'] as const
const DIAGNOSTIC_HEARTBEAT_STATUSES = ['running', 'healthy', 'stale', 'stalled', 'lease_expired', 'paused', 'completed', 'recovered', 'unknown'] as const
const DIAGNOSTIC_AUTO_CONTINUE_STATUSES = ['pending', 'started', 'skipped', 'failed', 'completed'] as const
const DIAGNOSTIC_DELIVERY_STATUSES = ['pending', 'retry', 'delivered', 'dead_letter', 'failed'] as const
const DIAGNOSTIC_RECOVERY_STATUSES = ['pending', 'recovering', 'recovered', 'failed', 'dead_lettered'] as const
const DIAGNOSTIC_PAUSE_STATUSES = ['pause_requested', 'paused', 'resume_requested', 'resuming', 'running', 'failed'] as const
const DIAGNOSTIC_WARNING_CODES = ['task_job_stalled', 'task_job_stale', 'slow-output'] as const

function closedDiagnosticValue<const T extends readonly string[]>(value: unknown, allowed: T): T[number] | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return allowed.includes(normalized as T[number]) ? normalized as T[number] : undefined
}

function hasRuntimeDiagnosticSignal(
  diagnostics: RuntimeDiagnosticRecord,
  includePlainLifecycle: boolean
): boolean {
  const diagnosticKeys = [
    'heartbeatStatus',
    'heartbeatAgeMs',
    'leaseExpired',
    'staleAfterMs',
    'stalled',
    'orphaned',
    'recoveryStatus',
    'recoveryAttempt',
    'deliveryId',
    'deliveryStatus',
    'autoContinueStatus',
    'completionDeliveryAttempt',
    'lateCompletionSuppressed',
    'warningCode'
  ]
  if (diagnosticKeys.some((key) => diagnostics[key] !== undefined && diagnostics[key] !== '')) return true
  return includePlainLifecycle && Boolean(readDiagnosticString(diagnostics, 'jobId') || readDiagnosticString(diagnostics, 'childThreadId'))
}

function runtimeDiagnosticDetailRows(
  diagnostics: RuntimeDiagnosticRecord
): Array<{ label: string; value: string }> {
  const rows: Array<{ label: string; value: string }> = []
  for (const [key, label] of DIAGNOSTIC_DETAIL_FIELDS) {
    const value = diagnostics[key]
    const formatted = formatDiagnosticValue(key, value)
    if (!formatted) continue
    rows.push({ label, value: formatted })
  }
  return rows
}

function runtimeDiagnosticStatusLabel(diagnostics: RuntimeDiagnosticRecord): string {
  const notificationKind = readDiagnosticString(diagnostics, 'notificationKind')
  const autoStatus = readDiagnosticString(diagnostics, 'autoContinueStatus')
  if (notificationKind === 'background_job_auto_continue' && autoStatus) {
    return `自动续写 ${formatAutoContinueStatus(autoStatus)}`
  }
  const deliveryStatus = readDiagnosticString(diagnostics, 'deliveryStatus')
  if (deliveryStatus) return `投递 ${formatDeliveryStatus(deliveryStatus)}`
  if (autoStatus) {
    return `自动续写 ${formatAutoContinueStatus(autoStatus)}`
  }
  const recoveryStatus = readDiagnosticString(diagnostics, 'recoveryStatus')
  if (recoveryStatus) return formatRecoveryStatus(recoveryStatus)
  const heartbeatStatus = readDiagnosticString(diagnostics, 'heartbeatStatus')
  if (heartbeatStatus) return formatHeartbeatStatus(heartbeatStatus)
  const status = readDiagnosticString(diagnostics, 'status')
  if (status) return formatRuntimeStatus(status)
  return '诊断'
}

function runtimeDiagnosticTone(diagnostics: RuntimeDiagnosticRecord): RuntimeDiagnosticEntryTone {
  const statusValues = [
    readDiagnosticString(diagnostics, 'deliveryStatus'),
    readDiagnosticString(diagnostics, 'autoContinueStatus'),
    readDiagnosticString(diagnostics, 'recoveryStatus'),
    readDiagnosticString(diagnostics, 'heartbeatStatus'),
    readDiagnosticString(diagnostics, 'status')
  ].filter(Boolean).join(' ').toLowerCase()
  if (/dead_letter|dead-letter|failed|error/.test(statusValues)) return 'error'
  if (
    diagnostics.lateCompletionSuppressed === true ||
    diagnostics.leaseExpired === true ||
    diagnostics.orphaned === true ||
    diagnostics.stalled === true ||
    /skipped|retry|stale|lease_expired|orphaned|interrupted|killed|superseded/.test(statusValues)
  ) return 'warning'
  if (/running|pending|recovering|requested|admitted/.test(statusValues)) return 'running'
  if (/delivered|completed|recovered|done/.test(statusValues)) return 'success'
  return 'default'
}

function diagnosticToneClass(tone: RuntimeDiagnosticEntryTone): string {
  switch (tone) {
    case 'error':
      return 'text-red-600 dark:text-red-300'
    case 'warning':
      return 'text-orange-600 dark:text-orange-300'
    case 'running':
      return 'text-ds-accent'
    case 'success':
      return 'text-emerald-600 dark:text-emerald-300'
    default:
      return 'text-ds-muted'
  }
}

function timelineDiagnosticKey(block: ChatBlock, diagnostics: RuntimeDiagnosticRecord): string {
  const deliveryId = readDiagnosticString(diagnostics, 'deliveryId')
  const deliveryStatus = readDiagnosticString(diagnostics, 'deliveryStatus')
  if (deliveryId) return `timeline:delivery:${deliveryId}:${deliveryStatus}`
  const autoStatus = readDiagnosticString(diagnostics, 'autoContinueStatus')
  const jobId = readDiagnosticString(diagnostics, 'jobId')
  const autoTurnId = readDiagnosticString(diagnostics, 'autoContinueTurnId')
  if (autoStatus && jobId) return `timeline:auto:${jobId}:${autoStatus}:${autoTurnId}`
  const notificationKind = readDiagnosticString(diagnostics, 'notificationKind')
  if (notificationKind && jobId) return `timeline:${notificationKind}:${jobId}:${readDiagnosticString(diagnostics, 'status')}`
  return `timeline:${block.id}`
}

function timelineDiagnosticTitle(block: ChatBlock, diagnostics: RuntimeDiagnosticRecord): string {
  const notificationKind = readDiagnosticString(diagnostics, 'notificationKind')
  const jobId = readDiagnosticString(diagnostics, 'jobId')
  if (notificationKind === 'background_job_delivery') return `后台结果投递${jobId ? ` ${jobId}` : ''}`
  if (notificationKind === 'background_job_auto_continue') return `后台自动续写${jobId ? ` ${jobId}` : ''}`
  if (notificationKind === 'background_job_completion') return `后台任务完成${jobId ? ` ${jobId}` : ''}`
  if (block.kind === 'tool') return block.summary || '运行时诊断'
  if (block.kind === 'system') return block.text || '运行时诊断'
  return '运行时诊断'
}

function dedupeDiagnostics(entries: RuntimeDiagnosticEntry[]): RuntimeDiagnosticEntry[] {
  const byKey = new Map<string, RuntimeDiagnosticEntry>()
  for (const entry of entries) {
    const previous = byKey.get(entry.key)
    if (!previous || entry.sortAt.localeCompare(previous.sortAt) >= 0) {
      byKey.set(entry.key, entry)
    }
  }
  return [...byKey.values()]
}

function readDiagnosticString(
  diagnostics: RuntimeDiagnosticRecord,
  key: string
): string {
  const value = diagnostics[key]
  return typeof value === 'string' ? value.trim() : ''
}

function formatDiagnosticValue(key: string, value: unknown): string {
  if (value === undefined || value === null || value === '') return ''
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'number') {
    if (key.endsWith('Ms')) return formatDuration(value)
    return Number.isFinite(value) ? String(value) : ''
  }
  if (Array.isArray(value)) return value.map((item) => String(item)).join(', ')
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms)) return ''
  if (ms < 1000) return `${Math.max(1, Math.round(ms))}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`
  const totalSeconds = Math.round(ms / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}m ${seconds}s`
}

function formatDeliveryStatus(status: string): string {
  switch (status) {
    case 'pending':
      return '待投递'
    case 'retry':
      return '补投递'
    case 'delivered':
      return '已投递'
    case 'skipped':
      return '已跳过'
    case 'dead_letter':
    case 'dead_lettered':
      return 'dead-letter'
    default:
      return status
  }
}

function formatAutoContinueStatus(status: string): string {
  switch (status) {
    case 'started':
      return '已唤醒'
    case 'skipped':
      return '已跳过'
    case 'failed':
      return '失败'
    default:
      return status
  }
}

function formatRecoveryStatus(status: string): string {
  switch (status) {
    case 'recovering':
      return '恢复中'
    case 'recovered':
      return '已恢复'
    case 'dead_lettered':
      return '无法恢复'
    default:
      return status
  }
}

function formatHeartbeatStatus(status: string): string {
  switch (status) {
    case 'running':
      return '运行中'
    case 'stale':
      return '可能停滞'
    case 'lease_expired':
      return '租约过期'
    case 'orphaned':
      return '孤儿任务'
    case 'recovering':
      return '恢复中'
    case 'recovered':
      return '已恢复'
    case 'dead_lettered':
      return '无法恢复'
    default:
      return status
  }
}

function formatRuntimeStatus(status: string): string {
  switch (status) {
    case 'running':
      return '运行中'
    case 'completed':
      return '完成'
    case 'failed':
      return '失败'
    case 'killed':
      return '已停止'
    case 'interrupted':
      return '已中断'
    case 'superseded':
      return '已被替代'
    default:
      return status
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}
