import type { ReactElement } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  ArrowDownToLine,
  ArrowUpFromLine,
  ChevronDown,
  CircleDot,
  Database,
  FilePlus2,
  GitBranch,
  Laptop,
  Loader2,
  Plus,
  ReceiptText,
  UsersRound,
  X
} from 'lucide-react'
import type { GitBranchesResult } from '@shared/git-branches'
import type {
  CoreThreadSummaryResponseJson,
  CoreThreadSummarySubagentJson,
  CoreThreadSummaryTaskJson
} from '../../agent/analytix-contract'
import { getProvider } from '../../agent/registry'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import {
  getSharedActiveCaseContext,
  querySharedCaseOverview,
  readCachedSharedCaseOverview,
  subscribeSharedCaseOverview,
  subscribeSharedActiveCaseContext
} from '../../data-analysis/services/analysis/stats-case-overview-resource'
import type { StatsV2CaseOverviewDTO } from '../../data-analysis/services/analysis/stats-api'
import { extractUnifiedDiffText, sumDiffStats, type DiffStats } from '../../lib/diff-stats'
import { useChatStore } from '../../store/chat-store'
import { SummaryRow } from './SummaryRow'
import { SummarySection } from './SummarySection'
import { SubagentSummaryRows, subagentTaskId } from './SubagentSummaryRows'
import { TaskSummaryRows } from './TaskSummaryRows'
import { OutputSummaryRows } from './OutputSummaryRows'
import {
  RuntimeDiagnosticsRows,
  buildRuntimeDiagnosticsEntries,
  type RuntimeDiagnosticAction,
  type RuntimeDiagnosticActionPayload,
  type RuntimeDiagnosticEntry
} from './RuntimeDiagnosticsPanel'
import type { RuntimeDiagnosticsFocus } from '../../lib/runtime-diagnostics-focus'
import { isPublicProjectionRevoked } from '../../lib/public-projection-revocation'
import { SideChatSummaryRows, SourceSummaryRows } from './SourceSummaryRows'
import { querySharedThreadSummary, readCachedSharedThreadSummary } from './thread-summary-resource'

type SummaryState =
  | { status: 'idle' | 'loading'; data: null; error: null }
  | { status: 'ready'; data: CoreThreadSummaryResponseJson; error: null }
  | { status: 'error'; data: CoreThreadSummaryResponseJson | null; error: string }

type OutputState =
  | { status: 'closed' }
  | { status: 'loading'; title: string }
  | { status: 'error'; title: string; message: string }

type ThreadSummaryEnvironment = {
  workspaceRoot: string
  diffStats: DiffStats | null
  gitInfo: GitBranchesResult | null
}

type CaseOverviewState =
  | { status: 'idle'; caseId: ''; data: null; error: null }
  | { status: 'loading'; caseId: string; data: StatsV2CaseOverviewDTO | null; error: null }
  | { status: 'ready'; caseId: string; data: StatsV2CaseOverviewDTO; error: null }
  | { status: 'error'; caseId: string; data: StatsV2CaseOverviewDTO | null; error: string }

const THREAD_SUMMARY_LIVE_REFRESH_DELAY_MS = 250
const THREAD_SUMMARY_IDLE_REFRESH_MS = 2_000

export type ThreadSummarySeqRefreshMarker = {
  seq: number
  threadId: string | null
}

function nextRuntimeDiagnosticsClientRequestId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `diag_${Date.now().toString(36)}`
}

function cssEscape(value: string): string {
  if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') return CSS.escape(value)
  return value.replace(/["\\]/g, '\\$&')
}

function runtimeSummaryStatusToDiagnosticsFilter(
  status: string | undefined
): RuntimeDiagnosticsFocus['status'] {
  const normalized = status?.trim()
  switch (normalized) {
    case 'running':
    case 'stale':
    case 'lease_expired':
    case 'orphaned':
    case 'recovering':
    case 'recovered':
    case 'dead_lettered':
    case 'paused':
      return normalized
    case 'pending':
    case 'active':
      return 'running'
    default:
      return undefined
  }
}

async function refreshAfterDiagnosticAction(
  activeThreadId: string | null,
  loadSummary: (mode?: 'initial' | 'refresh') => Promise<void>
): Promise<void> {
  await loadSummary('refresh')
  const store = useChatStore.getState()
  await Promise.allSettled([
    store.refreshThreads(),
    activeThreadId && store.activeThreadId === activeThreadId
      ? store.selectThread(activeThreadId)
      : Promise.resolve()
  ])
}

export function isThreadSummaryPublicationBlocked(threadId: string): boolean {
  const normalizedThreadId = threadId.trim()
  if (!normalizedThreadId || isPublicProjectionRevoked(normalizedThreadId)) return true
  return useChatStore.getState().threads.some((thread) => (
    thread.id === normalizedThreadId && thread.historyAuthority === 'case_boundary_only_v1'
  ))
}

function useCaseOverviewState(): CaseOverviewState {
  const [caseContext, setCaseContext] = useState(() => getSharedActiveCaseContext())
  const [state, setState] = useState<CaseOverviewState>(() => {
    const caseId = String(caseContext?.caseId || '').trim()
    if (!caseId) return { status: 'idle', caseId: '', data: null, error: null }
    const cached = readCachedSharedCaseOverview(caseId)
    return cached
      ? { status: 'ready', caseId, data: cached, error: null }
      : { status: 'loading', caseId, data: null, error: null }
  })
  const requestSeqRef = useRef(0)

  const loadCaseOverview = useCallback((caseId: string, force = false): void => {
    const normalizedCaseId = String(caseId || '').trim()
    if (!normalizedCaseId) {
      requestSeqRef.current += 1
      setState({ status: 'idle', caseId: '', data: null, error: null })
      return
    }
    const cached = force ? null : readCachedSharedCaseOverview(normalizedCaseId)
    if (cached) {
      setState({ status: 'ready', caseId: normalizedCaseId, data: cached, error: null })
      void querySharedCaseOverview(normalizedCaseId, { force }).catch(() => {})
      return
    }

    const requestSeq = requestSeqRef.current + 1
    requestSeqRef.current = requestSeq
    setState((current) => ({
      status: 'loading',
      caseId: normalizedCaseId,
      data: current.caseId === normalizedCaseId ? current.data : null,
      error: null
    }))
    void querySharedCaseOverview(normalizedCaseId, { force })
      .then((overview) => {
        if (requestSeqRef.current !== requestSeq) return
        setState({ status: 'ready', caseId: normalizedCaseId, data: overview, error: null })
      })
      .catch((error) => {
        if (requestSeqRef.current !== requestSeq) return
        const message = error instanceof Error ? error.message : String(error)
        setState((current) => ({
          status: 'error',
          caseId: normalizedCaseId,
          data: current.caseId === normalizedCaseId ? current.data : null,
          error: message
        }))
      })
  }, [])

  useEffect(() => subscribeSharedActiveCaseContext(setCaseContext, { emitCurrent: true }), [])

  useEffect(() => {
    loadCaseOverview(caseContext?.caseId ?? '')
  }, [caseContext?.caseId, loadCaseOverview])

  useEffect(() => {
    const caseId = String(caseContext?.caseId || '').trim()
    if (!caseId) return undefined
    return subscribeSharedCaseOverview(
      caseId,
      (overview) => {
        setState({ status: 'ready', caseId, data: overview, error: null })
      },
      { emitCurrent: true }
    )
  }, [caseContext?.caseId])

  return state
}

export function ThreadSummaryPanel({
  activeThreadId,
  className = '',
  onCollapse,
  onInspectChildAgent,
  onSubagentsChange,
  onOpenChanges,
  diagnosticsFocus,
  onDiagnosticsFocusHandled
}: {
  activeThreadId: string | null
  className?: string
  onCollapse: () => void
  onInspectChildAgent?: (subagents: CoreThreadSummarySubagentJson[], selectedKey: string) => void
  onSubagentsChange?: (threadId: string, subagents: CoreThreadSummarySubagentJson[]) => void
  onOpenChanges?: () => void
  diagnosticsFocus?: RuntimeDiagnosticsFocus | null
  onDiagnosticsFocusHandled?: () => void
}): ReactElement {
  const openThreadInSidePanel = useChatStore((s) => s.openThreadInSidePanel)
  const blocks = useChatStore((s) => s.blocks)
  const runtimeLastSeq = useChatStore((s) => s.lastSeq)
  const threadBusy = useChatStore((s) => s.busy)
  const workspaceRoot = useChatStore((s) => s.workspaceRoot)
  const [state, setState] = useState<SummaryState>({ status: 'idle', data: null, error: null })
  const [busyTaskId, setBusyTaskId] = useState<string | null>(null)
  const [busyDiagnosticKey, setBusyDiagnosticKey] = useState<string | null>(null)
  const [diagnosticActionError, setDiagnosticActionError] = useState<string | null>(null)
  const [localDiagnosticsFocus, setLocalDiagnosticsFocus] = useState<RuntimeDiagnosticsFocus | null>(null)
  const [outputState, setOutputState] = useState<OutputState>({ status: 'closed' })
  const [gitInfo, setGitInfo] = useState<GitBranchesResult | null>(null)
  const caseOverview = useCaseOverviewState()
  const liveRefreshMarkerRef = useRef<ThreadSummarySeqRefreshMarker>({ threadId: null, seq: 0 })
  const activeThreadIdRef = useRef(activeThreadId)
  const summaryLoadGenerationRef = useRef(0)
  activeThreadIdRef.current = activeThreadId
  const provider = useMemo(() => getProvider(), [])
  const diffStats = useMemo(() => {
    const patches = blocks.flatMap((block) =>
      block.kind === 'tool' && block.toolKind === 'file_change' && block.status === 'success'
        ? [extractUnifiedDiffText(block.detail)]
        : []
    )
    return sumDiffStats(patches)
  }, [blocks])
  const runtimeDiagnostics = useMemo(
    () => buildRuntimeDiagnosticsEntries(state.data, blocks),
    [blocks, state.data]
  )
  const effectiveDiagnosticsFocus = diagnosticsFocus ?? localDiagnosticsFocus

  useEffect(() => {
    const root = workspaceRoot.trim()
    if (!root || typeof window.analytix?.workspace?.getGitBranches !== 'function') {
      setGitInfo(null)
      return undefined
    }

    let cancelled = false
    void window.analytix.workspace.getGitBranches(root)
      .then((result) => {
        if (!cancelled) setGitInfo(result)
      })
      .catch(() => {
        if (!cancelled) setGitInfo(null)
      })
    return () => {
      cancelled = true
    }
  }, [workspaceRoot])

  const load = useCallback(async (mode: 'initial' | 'refresh' = 'refresh') => {
    if (!activeThreadId) {
      summaryLoadGenerationRef.current += 1
      setState({ status: 'ready', data: emptySummary(activeThreadId ?? ''), error: null })
      return
    }
    const requestThreadId = activeThreadId
    if (isThreadSummaryPublicationBlocked(requestThreadId)) {
      summaryLoadGenerationRef.current += 1
      const data = emptySummary(requestThreadId)
      setState({ status: 'ready', data, error: null })
      onSubagentsChange?.(requestThreadId, data.subagents)
      return
    }
    const requestGeneration = ++summaryLoadGenerationRef.current
    const isStale = (): boolean => (
      summaryLoadGenerationRef.current !== requestGeneration ||
      activeThreadIdRef.current !== requestThreadId ||
      isThreadSummaryPublicationBlocked(requestThreadId)
    )
    if (isStale()) return
    const cachedSummary = mode === 'initial' ? readCachedSharedThreadSummary(activeThreadId) : null
    setState((current) => mode === 'initial' || current.status === 'idle'
      ? cachedSummary
        ? { status: 'ready', data: cachedSummary, error: null }
        : { status: 'loading', data: null, error: null }
      : current)
    try {
      const runtimeSummary = await querySharedThreadSummary(activeThreadId, {
        force: mode !== 'initial',
        keepStale: mode !== 'initial'
      })
      if (isStale()) return
      setState({ status: 'ready', data: runtimeSummary, error: null })
      onSubagentsChange?.(activeThreadId, runtimeSummary.subagents)
    } catch (error) {
      if (isStale()) return
      const message = error instanceof Error ? error.message : String(error)
      setState((current) => ({
        status: 'error',
        data: current.data,
        error: message
      }))
    }
  }, [activeThreadId, onSubagentsChange, provider])

  useEffect(() => {
    void load('initial')
  }, [load])

  useEffect(() => {
    if (!shouldRefreshThreadSummaryForSeq(liveRefreshMarkerRef.current, activeThreadId, runtimeLastSeq)) return
    const timer = window.setTimeout(() => void load('refresh'), THREAD_SUMMARY_LIVE_REFRESH_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [activeThreadId, load, runtimeLastSeq])

  const active = state.data
    ? state.data.subagents.some((agent) => agent.status === 'active') ||
      state.data.tasks.some((task) => task.active === true)
    : false
  useEffect(() => {
    if (!active && !threadBusy) return
    void load('refresh')
    const timer = window.setInterval(() => void load('refresh'), 1_000)
    return () => window.clearInterval(timer)
  }, [active, load, threadBusy])

  useEffect(() => {
    if (!activeThreadId || active || threadBusy) return
    const timer = window.setInterval(() => void load('refresh'), THREAD_SUMMARY_IDLE_REFRESH_MS)
    return () => window.clearInterval(timer)
  }, [active, activeThreadId, load, threadBusy])

  const openThread = (
    threadId: string,
    options?: { title?: string; source?: 'side' | 'background-agent' }
  ): void => {
    void openThreadInSidePanel(threadId, options)
  }

  const jumpToTimeline = (entry: RuntimeDiagnosticEntry): void => {
    const blockId = entry.fields.sourceBlockId
    if (!blockId || typeof document === 'undefined') return
    const target = document.querySelector<HTMLElement>(
      `[data-analytix-process-block-id="${cssEscape(blockId)}"]`
    )
    target?.scrollIntoView({ block: 'center', behavior: 'smooth' })
    target?.focus?.({ preventScroll: true })
  }

  const runDiagnosticAction = async (
    action: RuntimeDiagnosticAction,
    entry: RuntimeDiagnosticEntry,
    payload?: RuntimeDiagnosticActionPayload
  ): Promise<void> => {
    const jobId = entry.fields.jobId
    const threadId = entry.fields.parentThreadId || activeThreadId
    if (!threadId || !jobId) return
    setBusyDiagnosticKey(entry.key)
    setDiagnosticActionError(null)
    try {
      const baseBody = {
        threadId,
        jobId,
        clientRequestId: nextRuntimeDiagnosticsClientRequestId(),
        ...(entry.fields.parentTurnId ? { sourceTurnId: entry.fields.parentTurnId } : {})
      }
      const response = action === 'pause'
        ? await rendererRuntimeClient.pauseTaskJob(baseBody)
        : action === 'resume'
          ? await rendererRuntimeClient.resumeTaskJob(baseBody)
          : action === 'steer'
            ? await rendererRuntimeClient.steerTaskJob({
              ...baseBody,
              message: payload?.message ?? '',
              clientMessageId: nextRuntimeDiagnosticsClientRequestId()
            })
            : action === 'restart'
                ? await rendererRuntimeClient.restartTaskJob(baseBody)
                : action === 'recover'
                  ? await rendererRuntimeClient.recoverTaskJob({
                    ...baseBody,
                    ...(entry.fields.deliveryId ? { deliveryId: entry.fields.deliveryId } : {}),
                    reason: 'recovered from runtime diagnostics'
                  })
                  : await rendererRuntimeClient.killTaskJob({
                    ...baseBody,
                    reason: 'killed from runtime diagnostics'
                  })
      if (!response.ok) {
        throw new Error(`runtime action failed (${response.status})`)
      }
      await refreshAfterDiagnosticAction(activeThreadId, load)
    } catch (error) {
      setDiagnosticActionError(error instanceof Error ? error.message : String(error))
    } finally {
      setBusyDiagnosticKey(null)
    }
  }

  const killSubagent = async (agent: CoreThreadSummarySubagentJson): Promise<void> => {
    const taskId = subagentTaskId(agent)
    if (!activeThreadId || !taskId || !agent.canKill) return
    setBusyTaskId(taskId)
    try {
      await provider.killThreadSummaryTask?.(activeThreadId, taskId)
      await load('refresh')
    } finally {
      setBusyTaskId(null)
    }
  }

  const focusSubagentDiagnostics = (agent: CoreThreadSummarySubagentJson): void => {
    setLocalDiagnosticsFocus({
      jobId: agent.taskJobId ?? agent.childRunId,
      childThreadId: agent.childThreadId,
      childTurnId: agent.childTurnId,
      parentThreadId: agent.parentThreadId,
      parentTurnId: agent.parentTurnId,
      status: runtimeSummaryStatusToDiagnosticsFilter(agent.diagnostics?.status || agent.rawStatus)
    })
  }

  const focusTaskDiagnostics = (task: CoreThreadSummaryTaskJson): void => {
    setLocalDiagnosticsFocus({
      jobId: task.id.replace(/^taskjob:/, ''),
      status: runtimeSummaryStatusToDiagnosticsFilter(task.status)
    })
  }

  const handleDiagnosticsFocusHandled = (): void => {
    setLocalDiagnosticsFocus(null)
    onDiagnosticsFocusHandled?.()
  }

  return (
    <ThreadSummaryPanelView
      state={state}
      busyTaskId={busyTaskId}
      outputState={outputState}
      busyDiagnosticKey={busyDiagnosticKey}
      diagnosticActionError={diagnosticActionError}
      environment={{ workspaceRoot, diffStats, gitInfo }}
      caseOverview={caseOverview}
      runtimeDiagnostics={runtimeDiagnostics}
      diagnosticsFocus={effectiveDiagnosticsFocus}
      className={className}
      onCollapse={onCollapse}
      onInspectChildAgent={onInspectChildAgent}
      onOpenChanges={onOpenChanges}
      onCloseOutput={() => setOutputState({ status: 'closed' })}
      onOpenThread={openThread}
      onOpenTaskDiagnostics={focusTaskDiagnostics}
      onKillSubagent={(agent) => void killSubagent(agent)}
      onOpenSubagentDiagnostics={focusSubagentDiagnostics}
      onRuntimeDiagnosticAction={(action, entry, payload) => void runDiagnosticAction(action, entry, payload)}
      onJumpRuntimeDiagnosticToTimeline={jumpToTimeline}
      onDiagnosticsFocusHandled={handleDiagnosticsFocusHandled}
    />
  )
}

export function shouldRefreshThreadSummaryForSeq(
  marker: ThreadSummarySeqRefreshMarker,
  activeThreadId: string | null,
  runtimeLastSeq: number
): boolean {
  if (!activeThreadId) {
    marker.threadId = null
    marker.seq = 0
    return false
  }
  if (marker.threadId !== activeThreadId) {
    marker.threadId = activeThreadId
    marker.seq = runtimeLastSeq
    return false
  }
  if (runtimeLastSeq <= marker.seq) return false
  marker.seq = runtimeLastSeq
  return true
}

export function ThreadSummaryPanelView({
  state,
  busyTaskId,
  busyDiagnosticKey,
  diagnosticActionError,
  outputState,
  environment,
  caseOverview = { status: 'idle', caseId: '', data: null, error: null },
  className = '',
  onCollapse,
  onCloseOutput,
  onOpenChanges,
  onInspectChildAgent,
  onOpenThread,
  onOpenTaskDiagnostics,
  onKillSubagent,
  onOpenSubagentDiagnostics,
  runtimeDiagnostics,
  diagnosticsFocus,
  onRuntimeDiagnosticAction,
  onJumpRuntimeDiagnosticToTimeline,
  onDiagnosticsFocusHandled
}: {
  state: SummaryState
  busyTaskId: string | null
  busyDiagnosticKey?: string | null
  diagnosticActionError?: string | null
  outputState: OutputState
  environment?: ThreadSummaryEnvironment
  caseOverview?: CaseOverviewState
  className?: string
  onCollapse: () => void
  onOpenChanges?: () => void
  onInspectChildAgent?: (subagents: CoreThreadSummarySubagentJson[], selectedKey: string) => void
  onCloseOutput: () => void
  onOpenThread: (threadId: string, options?: { title?: string; source?: 'side' | 'background-agent' }) => void
  onOpenTaskDiagnostics?: (task: CoreThreadSummaryTaskJson) => void
  onKillSubagent?: (agent: CoreThreadSummarySubagentJson) => void
  onOpenSubagentDiagnostics?: (agent: CoreThreadSummarySubagentJson) => void
  runtimeDiagnostics?: RuntimeDiagnosticEntry[]
  diagnosticsFocus?: RuntimeDiagnosticsFocus | null
  onRuntimeDiagnosticAction?: (
    action: RuntimeDiagnosticAction,
    entry: RuntimeDiagnosticEntry,
    payload?: RuntimeDiagnosticActionPayload
  ) => void
  onJumpRuntimeDiagnosticToTimeline?: (entry: RuntimeDiagnosticEntry) => void
  onDiagnosticsFocusHandled?: () => void
}): ReactElement {
  const data = state.data
  const hasCaseOverview = caseOverview.status !== 'idle'
  const hasEnvironment = Boolean(
    environment?.workspaceRoot ||
    environment?.diffStats ||
    (environment?.gitInfo?.ok ? environment.gitInfo.currentBranch : null)
  )
  const showBlockingLoading = state.status === 'loading' && !hasCaseOverview && !hasEnvironment
  return (
    <div className={`ds-thread-summary-card relative flex max-h-full min-h-0 w-[300px] flex-col overflow-hidden rounded-3xl border border-ds-border bg-ds-card-strong pt-2 shadow-md ${className}`}>
      <div className="flex h-10 shrink-0 items-center gap-2 px-4">
        <div className="min-w-0 flex-1 truncate text-base font-semibold leading-5 text-ds-ink">置顶摘要</div>
        <button
          type="button"
          aria-label="关闭摘要"
          className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/25"
          onClick={onCollapse}
        >
          <X className="h-4 w-4" strokeWidth={2} />
        </button>
      </div>
      <div className="flex h-fit max-h-full min-h-0 flex-col gap-3 overflow-y-auto pb-3">
        {showBlockingLoading ? (
          <div className="flex items-center gap-2 px-4 py-1 text-base text-ds-faint">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>正在加载摘要...</span>
          </div>
        ) : null}
        {state.status === 'error' ? (
          <div className="mx-4 flex items-start gap-2 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-[12px] text-red-600 dark:text-red-300">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <span className="min-w-0 break-words">{state.error}</span>
          </div>
        ) : null}
        {hasCaseOverview ? (
          <SummarySection title="案件数据概览" count={caseOverview.data ? 4 : 0}>
            <CaseOverviewSummaryRows state={caseOverview} />
          </SummarySection>
        ) : null}
        {hasEnvironment && environment ? (
          <SummarySection
            title="环境信息"
            count={0}
            after={<Plus className="h-5 w-5 text-ds-muted" strokeWidth={1.9} />}
          >
            <EnvironmentSummaryRows environment={environment} onOpenChanges={onOpenChanges} />
          </SummarySection>
        ) : null}
        {data ? (
          <>
            {data.subagents.length > 0 ? (
              <SummarySection title="子智能体" count={data.subagents.length} autoCollapse={data.subagents.every((item) => item.status !== 'active')}>
                <SubagentSummaryRows
                  subagents={data.subagents}
                  busyTaskId={busyTaskId}
                  onInspectChildAgent={onInspectChildAgent
                    ? (agent) => onInspectChildAgent(data.subagents, agent.key)
                    : undefined}
                  onOpenThread={(threadId, title) => onOpenThread(threadId, { title, source: 'background-agent' })}
                  onKill={onKillSubagent}
                />
              </SummarySection>
            ) : null}
            {data.tasks.length > 0 ? (
              <SummarySection title="后台任务" count={data.tasks.length} autoCollapse={data.tasks.every((item) => item.active !== true)}>
                <TaskSummaryRows
                  tasks={data.tasks}
                  busyTaskId={busyTaskId}
                  onOpenDiagnostics={onOpenTaskDiagnostics}
                />
              </SummarySection>
            ) : null}
            {data.outputs.length > 0 ? (
              <SummarySection title="输出" count={data.outputs.length}>
                <OutputSummaryRows outputs={data.outputs} />
              </SummarySection>
            ) : null}
            {data.sideChats.length > 0 ? (
              <SummarySection title="侧聊" count={data.sideChats.length}>
                <SideChatSummaryRows sideChats={data.sideChats} onOpenThread={onOpenThread} />
              </SummarySection>
            ) : null}
            <SummarySection title="来源" count={data.sources.length} defaultCollapsed={data.sources.length > 0}>
              <SourceSummaryRows sources={data.sources} />
            </SummarySection>
          </>
        ) : null}
        {outputState.status !== 'closed' ? (
          <div className="mx-4 border-t border-ds-border-muted pt-3">
            <div className="mb-2 flex items-center gap-2">
              <div className="min-w-0 flex-1 truncate text-[12px] font-semibold text-ds-ink">{outputState.title}</div>
              <button
                type="button"
                className="rounded px-2 py-1 text-[12px] text-ds-muted hover:bg-ds-hover hover:text-ds-ink"
                onClick={onCloseOutput}
              >
                关闭
              </button>
            </div>
            <pre className="max-h-56 overflow-auto whitespace-pre-wrap rounded-md bg-[var(--ds-code-bg)] px-3 py-2 text-[11px] leading-5 text-ds-ink">
              {outputState.status === 'loading'
                ? '正在读取输出...'
                : outputState.message}
            </pre>
          </div>
        ) : null}
      </div>
    </div>
  )
}

function EnvironmentSummaryRows({
  environment,
  onOpenChanges
}: {
  environment: ThreadSummaryEnvironment
  onOpenChanges?: () => void
}): ReactElement {
  const branch = environment.gitInfo?.ok ? environment.gitInfo.currentBranch : null
  const rootLabel = environment.gitInfo?.ok
    ? environment.gitInfo.repositoryRoot
    : environment.workspaceRoot

  return (
    <>
      <SummaryRow
        icon={<FilePlus2 className="h-5 w-5" strokeWidth={2.1} />}
        label="变更"
        trailing={environment.diffStats ? <DiffStatsInline stats={environment.diffStats} /> : null}
        trailingVisible
        interactive={Boolean(environment.diffStats && onOpenChanges)}
        onClick={environment.diffStats && onOpenChanges ? onOpenChanges : undefined}
      />
      {rootLabel ? (
        <SummaryRow
          icon={<Laptop className="h-5 w-5" strokeWidth={2.1} />}
          label="本地"
          title={rootLabel}
          trailing={<ChevronDown className="h-4 w-4 text-ds-muted" strokeWidth={2.2} />}
          trailingVisible
        />
      ) : null}
      {branch ? (
        <SummaryRow
          icon={<GitBranch className="h-5 w-5" strokeWidth={2.1} />}
          label={branch}
          trailing={<ChevronDown className="h-4 w-4 text-ds-muted" strokeWidth={2.2} />}
          trailingVisible
        />
      ) : null}
      {rootLabel ? (
        <SummaryRow
          icon={<CircleDot className="h-5 w-5" strokeWidth={2.1} />}
          label="提交或推送"
        />
      ) : null}
    </>
  )
}

function CaseOverviewSummaryRows({ state }: { state: CaseOverviewState }): ReactElement {
  if (state.status === 'loading' && !state.data) {
    return (
      <SummaryRow
        icon={<Loader2 className="h-5 w-5 animate-spin" strokeWidth={2} />}
        label="正在读取案件数据"
        labelClassName="text-ds-faint"
      />
    )
  }
  if (state.status === 'error' && !state.data) {
    return (
      <SummaryRow
        icon={<AlertCircle className="h-5 w-5" strokeWidth={2} />}
        label="案件数据暂不可用"
        labelClassName="text-ds-faint"
        detail={state.error}
        title={state.error}
      />
    )
  }

  const overview = state.data
  if (!overview) {
    return (
      <SummaryRow
        icon={<Database className="h-5 w-5" strokeWidth={2} />}
        label="暂无案件数据"
        labelClassName="text-ds-faint"
      />
    )
  }

  const publicationAllowed = overview.fact_answer_allowed === true
  const accountVerified = publicationAllowed && overview.account_status === 'verified'
  const transactionVerified = publicationAllowed && overview.transaction_status === 'verified'
  const amountVerified = publicationAllowed && overview.amount_status === 'verified'
  const amountPartial = publicationAllowed && overview.amount_status === 'partial'
  const accountTitle = accountVerified
    ? `账户数 ${formatCompactCount(overview.account_count, '个')}（涉及个人 ${formatCompactCount(overview.personal_account_count, '个')}，对公 ${formatCompactCount(overview.corporate_account_count, '个')}${(overview.unknown_account_count ?? 0) > 0 ? `，未识别 ${formatCompactCount(overview.unknown_account_count, '个')}` : ''}）`
    : `账户统计不可用：${formatCaseOverviewBlocker(overview.account_blocker)}`
  const transactionValue = transactionVerified
    ? formatCompactCount(overview.transaction_count, '条')
    : '不可用'
  const amountUnavailableValue = amountPartial ? '部分覆盖' : '不可用'
  const amountDetail = amountVerified
    ? ''
    : amountPartial
      ? `${formatAmountCoverage(overview)}；覆盖不完整，不发布全案金额`
      : `金额统计不可用：${formatCaseOverviewBlocker(overview.amount_blocker)}`
  return (
    <>
      <SummaryRow
        icon={<UsersRound className="h-5 w-5" strokeWidth={2} />}
        label={accountVerified ? <AccountOverviewLabel overview={overview} /> : '账户数'}
        labelClassName={accountVerified ? 'text-ds-ink' : 'text-ds-faint'}
        detail={accountTitle}
        title={accountTitle}
        trailing={<MetricValue>{accountVerified ? formatCompactCount(overview.account_count, '个') : '不可用'}</MetricValue>}
        trailingVisible
      />
      <SummaryRow
        icon={<ReceiptText className="h-5 w-5" strokeWidth={2} />}
        label="交易明细"
        labelClassName={transactionVerified ? 'text-ds-ink' : 'text-ds-faint'}
        detail={transactionVerified ? `主端交易明细 ${transactionValue}` : `交易统计不可用：${formatCaseOverviewBlocker(overview.transaction_blocker)}`}
        title={transactionVerified ? `主端交易明细 ${transactionValue}` : `交易统计不可用：${formatCaseOverviewBlocker(overview.transaction_blocker)}`}
        trailing={<MetricValue>{transactionValue}</MetricValue>}
        trailingVisible
      />
      <SummaryRow
        icon={<ArrowDownToLine className="h-5 w-5" strokeWidth={2} />}
        label="进账资金"
        labelClassName={amountVerified ? 'text-ds-ink' : 'text-ds-faint'}
        detail={amountVerified ? `主端进账资金 ${formatCompactMoney(overview.inflow_amount)}` : amountDetail}
        title={amountVerified ? `主端进账资金 ${formatCompactMoney(overview.inflow_amount)}` : amountDetail}
        trailing={<MetricValue tone="in">{amountVerified ? formatCompactMoney(overview.inflow_amount) : amountUnavailableValue}</MetricValue>}
        trailingVisible
      />
      <SummaryRow
        icon={<ArrowUpFromLine className="h-5 w-5" strokeWidth={2} />}
        label="出账资金"
        labelClassName={amountVerified ? 'text-ds-ink' : 'text-ds-faint'}
        detail={amountVerified ? `主端出账资金 ${formatCompactMoney(overview.outflow_amount)}` : amountDetail}
        title={amountVerified ? `主端出账资金 ${formatCompactMoney(overview.outflow_amount)}` : amountDetail}
        trailing={<MetricValue tone="out">{amountVerified ? formatCompactMoney(overview.outflow_amount) : amountUnavailableValue}</MetricValue>}
        trailingVisible
      />
    </>
  )
}

function AccountOverviewLabel({ overview }: { overview: StatsV2CaseOverviewDTO }): ReactElement {
  return (
    <span className="flex min-w-0 items-baseline gap-1.5">
      <span className="shrink-0">账户数</span>
      <span className="min-w-0 truncate text-[11px] font-medium leading-4 text-ds-faint">
        {[
          `个人 ${formatCompactCount(overview.personal_account_count, '个')}`,
          `对公 ${formatCompactCount(overview.corporate_account_count, '个')}`,
          (overview.unknown_account_count ?? 0) > 0 ? `未识别 ${formatCompactCount(overview.unknown_account_count, '个')}` : null
        ].filter(Boolean).join(' · ')}
      </span>
    </span>
  )
}

function MetricValue({
  children,
  tone = 'default'
}: {
  children: string
  tone?: 'default' | 'in' | 'out'
}): ReactElement {
  const color = tone === 'in'
    ? 'var(--ds-diff-added)'
    : tone === 'out'
      ? 'var(--ds-diff-removed)'
      : undefined
  return (
    <span className="font-semibold tabular-nums leading-5" style={color ? { color } : undefined}>
      {children}
    </span>
  )
}

function formatCompactCount(value: number | null | undefined, suffix: string): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '不可用'
  const normalized = Math.max(0, Math.round(value))
  if (normalized >= 100_000_000) {
    return `${formatUnitNumber(normalized / 100_000_000)}亿${suffix}`
  }
  if (normalized >= 10_000) {
    return `${formatUnitNumber(normalized / 10_000)}万${suffix}`
  }
  return `${normalized.toLocaleString('zh-CN')}${suffix}`
}

function formatCompactMoney(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '不可用'
  const normalized = Math.max(0, value)
  if (normalized >= 100_000_000) {
    return `${formatUnitNumber(normalized / 100_000_000)}亿元`
  }
  if (normalized >= 10_000) {
    return `${formatUnitNumber(normalized / 10_000)}万元`
  }
  return `${normalized.toLocaleString('zh-CN', {
    minimumFractionDigits: normalized % 1 === 0 ? 0 : 2,
    maximumFractionDigits: 2
  })}元`
}

function formatAmountCoverage(overview: StatsV2CaseOverviewDTO): string {
  const present = overview.amount_present_rows
  const total = overview.amount_total_rows
  const missing = overview.amount_missing_rows
  const parseFailed = overview.amount_parse_failed_rows
  if (present === null || total === null || missing === null || parseFailed === null ||
    !Number.isFinite(present) || !Number.isFinite(total) || !Number.isFinite(missing) || !Number.isFinite(parseFailed)) {
    return '金额覆盖范围未知'
  }
  return `金额可用 ${present.toLocaleString('zh-CN')} / ${total.toLocaleString('zh-CN')} 条，缺失 ${missing.toLocaleString('zh-CN')} 条，解析失败 ${parseFailed.toLocaleString('zh-CN')} 条`
}

function formatCaseOverviewBlocker(blocker: string): string {
  switch (String(blocker || '').trim()) {
    case 'materialization_unavailable': return '当前数据快照尚未就绪'
    case 'transaction_source_unavailable': return '交易数据源不可用'
    case 'transaction_query_failed': return '交易统计校验失败'
    case 'empty_scope_requires_host_receipt': return '已查范围为空，尚无宿主 no-hit 回执'
    case 'amount_column_unavailable': return '金额字段不可用'
    case 'account_source_unavailable': return '账户数据源不可用'
    case 'account_identity_unavailable': return '账户标识字段不可用'
    case 'account_query_failed': return '账户统计校验失败'
    case 'account_query_result_invalid': return '账户统计结果无效'
    case 'account_partition_invalid': return '账户分类校验失败'
    case 'case_unavailable': return '尚未绑定案件'
    case 'host_evidence_receipt_required': return '尚无同轮同快照的宿主证据回执'
    default: return '当前证据不足'
  }
}

function formatUnitNumber(value: number): string {
  return value.toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })
}

function DiffStatsInline({ stats }: { stats: DiffStats }): ReactElement {
  return (
    <span className="inline-flex items-center gap-1 font-medium tabular-nums leading-5">
      <span style={{ color: 'var(--ds-diff-added)' }}>+{stats.added.toLocaleString()}</span>
      <span style={{ color: 'var(--ds-diff-removed)' }}>-{stats.removed.toLocaleString()}</span>
    </span>
  )
}

function emptySummary(threadId: string): CoreThreadSummaryResponseJson {
  return {
    threadId,
    generatedAt: new Date().toISOString(),
    latestSeq: 0,
    subagents: [],
    tasks: [],
    outputs: [],
    sources: [],
    sideChats: [],
    backgroundProcesses: []
  }
}
