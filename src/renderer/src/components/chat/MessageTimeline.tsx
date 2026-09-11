import type { ProfilerOnRenderCallback, ReactElement, RefObject } from 'react'
import { Profiler, memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock, RuntimeConnectionStatus } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import { useTimelineStores } from './use-timeline-stores'
import { TIMELINE_AT_BOTTOM_PX, useTimelineScroll, type TimelineStreamingPhase } from './use-timeline-scroll'
import { deriveTurnSections, type TurnSections } from './derive-turn-sections'
import { MessageTimelineEmptyHero, ThreadForkBanner, ThreadForkPoint } from './message-timeline-empty'
import { GeneratedFilesPanel, MessageBubble } from './message-timeline-bubbles'
import { ReviewPlanCard, ReviewSummaryCard, TurnChangeSummary, WorkMetaRow } from './message-timeline-cards'
import { CompactionStatusRow, ProcessSectionRow, groupProcessSections } from './message-timeline-process'
import { ThreadUserMessageNavigationRail } from './ThreadUserMessageNavigationRail'
import { InjectedMemoryLookupProvider } from './injected-memory-lookup'
import {
  AnimatedWorkLogo,
  MASCOT_WORK_LOGO_VARIANT_LABEL_KEYS,
  WORK_LOGO_SWIM_MODE_LABEL_KEYS,
  useMascotWorkLogoVariant,
  useWorkLogoSwimMode
} from './AnimatedWorkLogo'
import type { UiPluginLabelKey } from '@shared/ui-plugin'
import { useUiPluginWorkLabel } from '../../store/ui-plugin-store'
import { extractPlanMetadataFromBlock } from '../../plan/plan-tool'
import { planDisplayNameFromRelativePath } from '../../plan/plan-path'
import { buildThreadProjection } from '../../thread/projection/thread-projection-store'
import {
  groupThreadTurns,
  sameThreadTurnContent as sameTurnContent,
  type ThreadTurn as Turn
} from '../../thread/projection/thread-turns'
import { computeVirtualThreadWindow } from '../../thread/virtualizer/analytix-thread-virtualizer'
import { createMeasuredRowCache } from '../../thread/virtualizer/measured-row-cache'
import { ResizeObserverBatcher } from '../../thread/virtualizer/resize-observer-batcher'
import type { ThreadUserMessageNavigationItem } from '../../thread/projection/thread-row-model'
import {
  createThreadTraceEvent,
  isThreadTraceEnabled,
  PersistedThreadTraceSink
} from '../../thread/tracing/thread-performance-trace'
import {
  useActiveStreamEstimateMetrics,
  useActiveStreamSnapshot
} from '../../thread/streaming/active-stream-store'

export { summarizeToolBlock } from './message-timeline-process'

const LIVE_PROCESS_TEXT_MAX_CHARS = 12000
const ESTIMATED_INITIAL_VIEWPORT_HEIGHT = 720

export function shouldShowReturnToBottomButton({
  bottomDistance,
  hasContent,
  responseSpacerHeightPx = 0,
  thresholdPx = TIMELINE_AT_BOTTOM_PX
}: {
  bottomDistance: number
  hasContent: boolean
  responseSpacerHeightPx?: number
  thresholdPx?: number
}): boolean {
  return hasContent && bottomDistance > Math.max(0, responseSpacerHeightPx) + thresholdPx
}

export type TimelineReturnToBottomState = {
  show: boolean
  hasLiveActivity: boolean
  onClick: () => void
}

export type TimelineRuntimeStateOverride = Partial<Pick<
  ReturnType<typeof useTimelineStores>,
  | 'busy'
  | 'currentTurnId'
  | 'currentTurnUserId'
  | 'turnStartedAtByUserId'
  | 'turnDurationByUserId'
>>

function blockRuntimeTurnId(block: ChatBlock | undefined): string | null {
  const meta = (block as { meta?: Record<string, unknown> } | undefined)?.meta
  const turnId = typeof meta?.turnId === 'string' ? meta.turnId.trim() : ''
  return turnId || null
}

function runtimeTurnId(turn: Turn): string | null {
  const userTurnId = blockRuntimeTurnId(turn.user)
  if (userTurnId) return userTurnId
  for (const block of turn.blocks) {
    const turnId = blockRuntimeTurnId(block)
    if (turnId) return turnId
  }
  return null
}

type Props = {
  blocks: ChatBlock[]
  live: string
  activeThreadId: string | null
  runtimeConnection: RuntimeConnectionStatus
  runtimeError?: string | null
  onRetryConnection: () => void
  onOpenSettings: () => void
  onSelectSuggestion?: (prompt: string) => void
  focusModeEnabled?: boolean
  devPreviewCard?: ReactElement | null
  /** Disables the inline Review Plan card's Build action while a turn runs. */
  planActionsBusy?: boolean
  /** Runs the active plan (Build button on the inline Review Plan card). */
  onBuildPlan?: () => void
  /** Opens/focuses the Plan panel (Open button on the inline card). */
  onOpenPlan?: () => void
  compactCards?: boolean
  edgeAlignedScroll?: boolean
  contentClassName?: string
  runtimeStateOverride?: TimelineRuntimeStateOverride
  onReturnToBottomStateChange?: (state: TimelineReturnToBottomState | null) => void
}

const TURN_PAGE_SIZE = 18
const AUTO_COLLAPSE_THRESHOLD = 24
const VIRTUALIZER_ROW_THRESHOLD = 48
const VIRTUALIZER_OVERSCAN_PX = 1600
const VIRTUALIZER_ROW_GAP_PX = 32
const MIN_LIVE_RESPONSE_SPACER_PX = 72
const MAX_LIVE_RESPONSE_SPACER_PX = 180

export function timelineResponseSpacerHeight(live: boolean): number | string {
  return live
    ? `clamp(${MIN_LIVE_RESPONSE_SPACER_PX}px, 16vh, ${MAX_LIVE_RESPONSE_SPACER_PX}px)`
    : 1
}

export function goalTimelinePaddingClass(route: 'chat' | 'claw', hasActiveGoal: boolean): string {
  return route === 'chat' && hasActiveGoal ? 'pb-32 md:pb-40' : 'pb-10'
}

export function liveTurnProgressClass(hasActiveGoal: boolean): string {
  return hasActiveGoal
    ? 'flex w-fit max-w-full items-center gap-2 py-0.5 text-[14px] font-medium text-ds-muted mb-16 md:mb-20'
    : 'flex w-fit max-w-full items-center gap-2 py-0.5 text-[14px] font-medium text-ds-muted'
}

function blockScrollStamp(block: ChatBlock | undefined): string {
  if (!block) return ''
  switch (block.kind) {
    case 'user':
    case 'assistant':
    case 'system':
      return `${block.id}:${block.kind}:${block.text.length}`
    case 'tool':
      return `${block.id}:${block.kind}:${block.status}:${block.summary.length}:${block.detail?.length ?? 0}`
    case 'review':
      return `${block.id}:${block.kind}:${block.status}:${block.reviewText?.length ?? 0}`
    case 'approval':
    case 'user_input':
    case 'compaction':
      return `${block.id}:${block.kind}:${block.status}`
    default:
      return ''
  }
}

function sameProjectionBlock(left: ChatBlock, right: ChatBlock): boolean {
  if (left === right) return true
  if (left.kind !== right.kind || left.id !== right.id) return false
  switch (left.kind) {
    case 'assistant':
    case 'user':
    case 'system':
      return 'text' in right && left.text === right.text
    case 'tool':
      return right.kind === 'tool' &&
        left.status === right.status &&
        left.summary === right.summary &&
        left.detail === right.detail &&
        left.filePath === right.filePath
    case 'compaction':
      return right.kind === 'compaction' &&
        left.status === right.status &&
        left.summary === right.summary &&
        left.detail === right.detail
    case 'review':
      return right.kind === 'review' &&
        left.status === right.status &&
        left.title === right.title &&
        left.reviewText === right.reviewText
    case 'approval':
      return right.kind === 'approval' &&
        left.status === right.status &&
        left.summary === right.summary &&
        left.errorMessage === right.errorMessage
    case 'user_input':
      return right.kind === 'user_input' &&
        left.status === right.status &&
        left.questions === right.questions &&
        left.answers === right.answers
  }
  return false
}

function sameBlockList(left: readonly ChatBlock[], right: readonly ChatBlock[]): boolean {
  if (left === right) return true
  if (left.length !== right.length) return false
  for (let index = 0; index < left.length; index += 1) {
    const leftBlock = left[index]
    const rightBlock = right[index]
    if (!leftBlock || !rightBlock || !sameProjectionBlock(leftBlock, rightBlock)) return false
  }
  return true
}

function sameTurnSections(left: TurnSections, right: TurnSections): boolean {
  return sameBlockList(left.processBlocks, right.processBlocks) &&
    sameBlockList(left.assistantContentBlocks, right.assistantContentBlocks) &&
    sameBlockList(left.compactionBlocks, right.compactionBlocks) &&
    sameBlockList(left.generatedFileBlocks, right.generatedFileBlocks) &&
    sameBlockList(left.turnFileChanges, right.turnFileChanges)
}

function turnPreview(turn: Turn, fallback: string): string {
  const text = turn.user?.text.trim() ?? ''
  if (!text) return fallback
  const oneLine = text.replace(/\s+/g, ' ')
  return oneLine.length > 48 ? `${oneLine.slice(0, 47).trimEnd()}...` : oneLine
}

function processBlockHasError(block: ChatBlock): boolean {
  return (
    (block.kind === 'tool' && block.status === 'error') ||
    (block.kind === 'compaction' && block.status === 'error') ||
    (block.kind === 'review' && block.status === 'error') ||
    (block.kind === 'approval' && block.status === 'error') ||
    (block.kind === 'user_input' && block.status === 'error') ||
    (block.kind === 'system' && block.severity === 'error')
  )
}

function findContentSearchUnit(root: HTMLElement, itemId: string): HTMLElement | null {
  for (const node of root.querySelectorAll<HTMLElement>('[data-content-search-unit-key]')) {
    if (node.dataset.contentSearchUnitKey === itemId) return node
  }
  return null
}

function highlightNavigationTarget(target: HTMLElement): void {
  const highlightTarget =
    target.querySelector<HTMLElement>('[data-user-message-bubble], [data-composer-attachment-pill]') ?? target
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true
  const keyframes = [
    { backgroundColor: 'color-mix(in srgb, var(--ds-text) 14%, var(--ds-bubble-user))' },
    { backgroundColor: 'color-mix(in srgb, var(--ds-text) 14%, var(--ds-bubble-user))', offset: 0.35 },
    { backgroundColor: 'color-mix(in srgb, var(--ds-text) 5%, var(--ds-bubble-user))' }
  ]
  if (typeof highlightTarget.animate === 'function') {
    highlightTarget.animate(keyframes, {
      duration: reducedMotion ? 0 : 1400,
      easing: 'cubic-bezier(0.23, 1, 0.32, 1)'
    })
    return
  }
  highlightTarget.classList.remove('timeline-navigation-target-highlight')
  void highlightTarget.offsetWidth
  highlightTarget.classList.add('timeline-navigation-target-highlight')
  window.setTimeout(() => {
    highlightTarget.classList.remove('timeline-navigation-target-highlight')
  }, reducedMotion ? 0 : 1500)
}

function nextAnimationFrame(): Promise<void> {
  return new Promise((resolve) => {
    window.requestAnimationFrame(() => resolve())
  })
}

export function MessageTimeline({
  blocks,
  live,
  activeThreadId,
  runtimeConnection,
  runtimeError,
  onRetryConnection,
  onOpenSettings,
  onSelectSuggestion,
  focusModeEnabled = false,
  devPreviewCard,
  planActionsBusy,
  onBuildPlan,
  onOpenPlan,
  compactCards = false,
  edgeAlignedScroll = false,
  contentClassName,
  runtimeStateOverride,
  onReturnToBottomStateChange
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const timelineStores = useTimelineStores(activeThreadId)
  const {
    route,
    workspaceRoot,
    chooseWorkspace,
    activeClawChannel,
    busy,
    currentTurnId,
    currentTurnUserId,
    turnStartedAtByUserId,
    turnDurationByUserId,
    activeThreadGoal,
    activeThread
  } = {
    ...timelineStores,
    ...(runtimeStateOverride ?? {})
  }

  const heroRoute: 'chat' | 'claw' = route === 'claw' ? 'claw' : 'chat'
  const endRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [responseSpacerHeightPx, setResponseSpacerHeightPx] = useState(0)
  const turnRefMap = useRef(new Map<string, HTMLDivElement>())
  const traceSinkRef = useRef<PersistedThreadTraceSink | null>(null)
  const activeThreadTraceIdRef = useRef<string | null>(activeThreadId)

  if (!traceSinkRef.current) {
    traceSinkRef.current = new PersistedThreadTraceSink()
  }

  useEffect(() => {
    activeThreadTraceIdRef.current = activeThreadId
  }, [activeThreadId])

  const recordAnchorCorrection = useCallback((data: Record<string, unknown>): void => {
    traceSinkRef.current?.record(
      createThreadTraceEvent('thread.scroll.anchor_corrected', {
        threadId: activeThreadTraceIdRef.current ?? undefined,
        data
      })
    )
  }, [])

  const turns = useMemo(() => groupThreadTurns(blocks), [blocks])
  const activeStreamForScroll = useActiveStreamEstimateMetrics(activeThreadId, currentTurnId)
  const latestBlock = blocks[blocks.length - 1]
  const scrollContentKey = [
    activeThreadId ?? '',
    turns.length,
    blocks.length,
    blockScrollStamp(latestBlock),
    activeStreamForScroll.version
  ].join(':')
  const hasLegacyLiveStream = Boolean(live.trim())
  const hasActiveStreamShell = Boolean(busy && currentTurnId)
  const hasLiveResponseSpacer = hasLegacyLiveStream || hasActiveStreamShell
  const liveResponseSpacerHeight = timelineResponseSpacerHeight(hasLiveResponseSpacer)
  const previousLiveResponseSpacerRef = useRef(hasLiveResponseSpacer)
  const latestTurnPhase: TimelineStreamingPhase = !busy
    ? 'idle'
    : live.trim() || activeStreamForScroll.contentLength > 0
      ? 'final_answer'
      : 'prework'
  const {
    visibleTurnCount,
    hiddenTurnCount,
    bottomDistance,
    loadEarlierTurns,
    collapseEarlierTurns,
    preserveBottomOnResize,
    scrollToBottom,
    snapToBottomIfNearLive
  } = useTimelineScroll({
    containerRef,
    activeThreadId,
    pageSize: TURN_PAGE_SIZE,
    autoCollapseThreshold: AUTO_COLLAPSE_THRESHOLD,
    totalTurns: turns.length,
    busy,
    scrollDeps: {
      contentKey: scrollContentKey,
      streaming: hasLegacyLiveStream || hasActiveStreamShell,
      userTurnKey: currentTurnUserId ?? '',
      phase: latestTurnPhase
    },
    onAnchorCorrected: recordAnchorCorrection
  })
  const hasLiveActivity = hasLegacyLiveStream || hasActiveStreamShell
  const effectiveResponseSpacerHeightPx = hasLiveResponseSpacer
    ? responseSpacerHeightPx || MIN_LIVE_RESPONSE_SPACER_PX
    : 0
  const handleReturnToBottomClick = useCallback((): void => {
    scrollToBottom({ behavior: 'smooth' })
  }, [scrollToBottom])

  useLayoutEffect(() => {
    if (!hasLiveResponseSpacer) {
      setResponseSpacerHeightPx(0)
      return
    }
    const node = endRef.current
    if (!node) return
    const updateSpacerHeight = (): void => {
      const nextHeight = Math.round(node.getBoundingClientRect().height)
      setResponseSpacerHeightPx((value) => value === nextHeight ? value : nextHeight)
    }
    updateSpacerHeight()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(updateSpacerHeight)
    observer.observe(node)
    return () => observer.disconnect()
  }, [hasLiveResponseSpacer, liveResponseSpacerHeight])

  useLayoutEffect(() => {
    const wasLive = previousLiveResponseSpacerRef.current
    previousLiveResponseSpacerRef.current = hasLiveResponseSpacer
    if (!wasLive || hasLiveResponseSpacer) return
    let frame = 0
    let frameId: number | null = null
    const settle = (): void => {
      snapToBottomIfNearLive()
      frame += 1
      if (frame < 60) {
        frameId = window.requestAnimationFrame(settle)
      }
    }
    snapToBottomIfNearLive()
    frameId = window.requestAnimationFrame(settle)
    return () => {
      if (frameId !== null) window.cancelAnimationFrame(frameId)
    }
  }, [hasLiveResponseSpacer, snapToBottomIfNearLive])

  // Tick a clock while a turn is running so the live "Worked for Xs" updates.
  const [tickNow, setTickNow] = useState(() => Date.now())
  useEffect(() => {
    if (!busy || !currentTurnUserId) return
    setTickNow(Date.now())
    const id = window.setInterval(() => setTickNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [busy, currentTurnUserId])

  const projection = useMemo(
    () =>
      buildThreadProjection({
        blocks,
        turns,
        live,
        activeThreadId,
        workspaceRoot,
        currentTurnId,
        currentTurnUserId,
        turnStartedAtByUserId,
        turnDurationByUserId,
        busy,
        tickNow,
        hiddenTurnCount,
        pageSize: TURN_PAGE_SIZE,
        autoCollapseThreshold: AUTO_COLLAPSE_THRESHOLD,
        activeThreadGoal,
        devPreviewCard,
        forkedFromThreadId: activeThread?.forkedFromThreadId,
        forkedFromTitle: activeThread?.forkedFromTitle,
        forkedFromTurnCount: activeThread?.forkedFromTurnCount,
        turnPreview,
        turnTitle: (index) => t('timelineJumpTurn', { index })
      }),
    [
      activeThread?.forkedFromThreadId,
      activeThread?.forkedFromTitle,
      activeThread?.forkedFromTurnCount,
      activeThreadGoal,
      activeThreadId,
      blocks,
      busy,
      currentTurnId,
      currentTurnUserId,
      devPreviewCard,
      hiddenTurnCount,
      live,
      t,
      tickNow,
      turnDurationByUserId,
      turnStartedAtByUserId,
      turns,
      workspaceRoot
    ]
  )
  const showReturnToBottom = shouldShowReturnToBottomButton({
    bottomDistance,
    hasContent: projection.hasContent,
    responseSpacerHeightPx: effectiveResponseSpacerHeightPx
  })

  useEffect(() => {
    if (!onReturnToBottomStateChange) return
    onReturnToBottomStateChange({
      show: showReturnToBottom,
      hasLiveActivity,
      onClick: handleReturnToBottomClick
    })
  }, [
    handleReturnToBottomClick,
    hasLiveActivity,
    onReturnToBottomStateChange,
    showReturnToBottom
  ])

  useEffect(() => {
    if (!onReturnToBottomStateChange) return
    return () => onReturnToBottomStateChange(null)
  }, [onReturnToBottomStateChange])

  useEffect(() => {
    traceSinkRef.current?.record(
      createThreadTraceEvent('thread.projection.reduced', {
        threadId: activeThreadId ?? undefined,
        data: {
          blocks: blocks.length,
          turns: turns.length,
          rows: projection.rows.length,
          live: Boolean(live.trim()),
        }
      })
    )
  }, [activeThreadId, blocks.length, live, projection.rows.length, turns.length])

  const rowCacheRef = useRef(createMeasuredRowCache())
  const rowObserverRef = useRef<ResizeObserver | null>(null)
  const rowNodeByIdRef = useRef(new Map<string, Element>())
  const rowIdByNodeRef = useRef(new WeakMap<Element, string>())
  const [measurementRevision, setMeasurementRevision] = useState(0)
  const [viewportMetrics, setViewportMetrics] = useState({ scrollTop: 0, viewportHeight: 0 })
  const resizeBatcherRef = useRef<ResizeObserverBatcher | null>(null)
  const projectedRowCountRef = useRef(0)
  projectedRowCountRef.current = projection.rows.length

  if (!resizeBatcherRef.current) {
    resizeBatcherRef.current = new ResizeObserverBatcher({
      onMeasurements: (measurements) => {
        let changed = false
        for (const measurement of measurements) {
          const previous = rowCacheRef.current.get(measurement.rowId)
          const next = Math.ceil(measurement.height)
          if (previous !== undefined && Math.abs(previous - next) <= 1) continue
          rowCacheRef.current.set(measurement.rowId, next)
          if (previous !== rowCacheRef.current.get(measurement.rowId)) changed = true
        }
        const virtualizerMayUseMeasurements = projectedRowCountRef.current > VIRTUALIZER_ROW_THRESHOLD
        if (changed && virtualizerMayUseMeasurements) setMeasurementRevision((value) => value + 1)
        if (changed) preserveBottomOnResize()
        if (changed && virtualizerMayUseMeasurements) {
          traceSinkRef.current?.record(
            createThreadTraceEvent('thread.virtualizer.measured', {
              threadId: activeThreadTraceIdRef.current ?? undefined,
              data: { measurements: measurements.length }
            })
          )
        }
      }
    })
  }

  useEffect(() => {
    rowCacheRef.current.prune(projection.rows.map((row) => row.id))
  }, [projection.rows])

  const activeStreamRowId = useMemo(() => {
    if (!activeStreamForScroll.threadId || activeStreamForScroll.threadId !== activeThreadId) return null
    for (let index = projection.rows.length - 1; index >= 0; index -= 1) {
      const row = projection.rows[index]
      if (row.kind === 'liveOnlyTurn') {
        if (!row.turnId || row.turnId === activeStreamForScroll.turnId) return row.id
        continue
      }
      if (row.kind === 'turn' && row.turn.isLiveTurn) return row.id
    }
    return null
  }, [activeStreamForScroll.threadId, activeStreamForScroll.turnId, activeThreadId, projection.rows])

  useEffect(() => {
    let changed = false
    if (activeStreamRowId) {
      changed = rowCacheRef.current.setLiveEstimate(activeStreamRowId, {
        processLength: activeStreamForScroll.processLength,
        contentLength: activeStreamForScroll.contentLength
      }) || changed
      changed = rowCacheRef.current.clearLiveEstimates(activeStreamRowId) || changed
    } else {
      changed = rowCacheRef.current.clearLiveEstimates() || changed
    }
    if (changed && projection.rows.length > VIRTUALIZER_ROW_THRESHOLD) {
      setMeasurementRevision((value) => value + 1)
    }
  }, [
    activeStreamForScroll.contentLength,
    activeStreamForScroll.processLength,
    activeStreamRowId,
    projection.rows.length
  ])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    let frame: number | null = null
    const update = (): void => {
      frame = null
      setViewportMetrics({
        scrollTop: el.scrollTop,
        viewportHeight: el.clientHeight
      })
    }
    const scheduleUpdate = (): void => {
      if (frame !== null) return
      frame = window.requestAnimationFrame(update)
    }
    update()
    el.addEventListener('scroll', scheduleUpdate, { passive: true })
    const observer =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver(() => scheduleUpdate())
    observer?.observe(el)
    return () => {
      el.removeEventListener('scroll', scheduleUpdate)
      observer?.disconnect()
      if (frame !== null) window.cancelAnimationFrame(frame)
    }
  }, [containerRef])

  useEffect(() => {
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const rowId = rowIdByNodeRef.current.get(entry.target)
        if (!rowId) continue
        resizeBatcherRef.current?.record(rowId, entry.contentRect.height)
      }
    })
    rowObserverRef.current = observer
    for (const node of rowNodeByIdRef.current.values()) observer.observe(node)
    return () => {
      observer.disconnect()
      rowObserverRef.current = null
      resizeBatcherRef.current?.dispose()
    }
  }, [])

  const registerThreadRow = useCallback((rowId: string, node: HTMLDivElement | null): void => {
    const observer = rowObserverRef.current
    const previous = rowNodeByIdRef.current.get(rowId)
    if (previous && previous !== node) {
      observer?.unobserve(previous)
      rowNodeByIdRef.current.delete(rowId)
    }
    if (!node) return
    rowNodeByIdRef.current.set(rowId, node)
    rowIdByNodeRef.current.set(node, rowId)
    observer?.observe(node)
    resizeBatcherRef.current?.record(rowId, node.getBoundingClientRect().height)
  }, [])

  const virtualWindow = useMemo(
    () => {
      void measurementRevision
      const viewportHeight =
        viewportMetrics.viewportHeight > 0
          ? viewportMetrics.viewportHeight
          : ESTIMATED_INITIAL_VIEWPORT_HEIGHT
      return computeVirtualThreadWindow({
        rows: projection.rows,
        cache: rowCacheRef.current,
        scrollTop: viewportMetrics.scrollTop,
        viewportHeight,
        overscanPx: VIRTUALIZER_OVERSCAN_PX,
        rowGapPx: VIRTUALIZER_ROW_GAP_PX
      })
    },
    [measurementRevision, projection.rows, viewportMetrics.scrollTop, viewportMetrics.viewportHeight]
  )
  const virtualizerEnabled =
    projection.rows.length > VIRTUALIZER_ROW_THRESHOLD
  const renderedItems = virtualizerEnabled
    ? virtualWindow.items.map((item) => ({ row: item.row, index: item.index }))
    : projection.rows.map((row, index) => ({ row, index }))

  useEffect(() => {
    traceSinkRef.current?.record(
      createThreadTraceEvent('thread.rows.updated', {
        threadId: activeThreadId ?? undefined,
        data: {
          rows: projection.rows.length,
          renderedRows: renderedItems.length,
          virtualized: virtualizerEnabled,
          totalHeight: Math.round(virtualWindow.totalHeight)
        }
      })
    )
  }, [
    activeThreadId,
    projection.rows.length,
    renderedItems.length,
    virtualWindow.totalHeight,
      virtualizerEnabled
  ])

  const commitMetricsRef = useRef({
    rows: 0,
    renderedRows: 0,
    virtualized: false
  })
  const lastReactCommitTraceAtRef = useRef(0)
  commitMetricsRef.current = {
    rows: projection.rows.length,
    renderedRows: renderedItems.length,
    virtualized: virtualizerEnabled
  }
  const recordReactCommit = useCallback<ProfilerOnRenderCallback>(
    (_id, phase, actualDuration, baseDuration, startTime, commitTime) => {
      if (!isThreadTraceEnabled()) return
      const profilingSession =
        typeof window !== 'undefined' &&
        window.localStorage?.getItem('analytix.threadTrace.profile') === '1'
      if (!busy && !profilingSession) return
      const now = Date.now()
      if (now - lastReactCommitTraceAtRef.current < 1000) return
      lastReactCommitTraceAtRef.current = now
      const metrics = commitMetricsRef.current
      traceSinkRef.current?.record(
        createThreadTraceEvent('thread.react.commit_sample', {
          threadId: activeThreadTraceIdRef.current ?? undefined,
          data: {
            actualDuration: Math.round(actualDuration),
            baseDuration: Math.round(baseDuration),
            commitLatency: Math.round(commitTime - startTime),
            mount: phase === 'mount',
            rows: metrics.rows,
            renderedRows: metrics.renderedRows,
            virtualized: metrics.virtualized
          }
        })
      )
    },
    [busy]
  )

  const findNavigationTarget = useCallback((itemId: string): HTMLElement | null => {
    const container = containerRef.current
    if (!container) return null
    return findContentSearchUnit(container, itemId)
  }, [])

  const scrollToEstimatedTurn = useCallback((turnKey: string, behavior: ScrollBehavior): boolean => {
    const container = containerRef.current
    if (!container) return false

    let offsetTop = 0
    for (let index = 0; index < projection.rows.length; index += 1) {
      const row = projection.rows[index]
      const height = rowCacheRef.current.estimate(row)
      if (row.kind === 'turn' && row.turn.key === turnKey) {
        const targetTop = Math.max(
          0,
          offsetTop - Math.max(0, (container.clientHeight - height) / 2)
        )
        container.scrollTo({ top: targetTop, behavior })
        return true
      }
      offsetTop += height + (index < projection.rows.length - 1 ? VIRTUALIZER_ROW_GAP_PX : 0)
    }

    const target = turnRefMap.current.get(turnKey)
    if (!target) return false
    target.scrollIntoView({ behavior, block: 'center' })
    return true
  }, [projection.rows])

  const waitForNavigationTarget = useCallback(async (
    item: ThreadUserMessageNavigationItem,
    timeoutMs = 2000
  ): Promise<HTMLElement | null> => {
    const startedAt = performance.now()
    while (performance.now() - startedAt <= timeoutMs) {
      const target = findNavigationTarget(item.id)
      if (target) return target
      await nextAnimationFrame()
    }
    return null
  }, [findNavigationTarget])

  const revealNavigationItem = useCallback((
    item: ThreadUserMessageNavigationItem,
    options?: { behavior?: ScrollBehavior }
  ): void => {
    void (async () => {
      const behavior = options?.behavior ?? 'smooth'
      const target = findNavigationTarget(item.id)
      if (target) {
        target.scrollIntoView({ behavior, block: 'center' })
        highlightNavigationTarget(target)
        return
      }

      if (!scrollToEstimatedTurn(item.turnKey, 'auto')) return
      await nextAnimationFrame()
      await nextAnimationFrame()
      const renderedTarget = await waitForNavigationTarget(item)
      if (renderedTarget) {
        renderedTarget.scrollIntoView({ behavior, block: 'center' })
        highlightNavigationTarget(renderedTarget)
        return
      }

      const turnTarget = turnRefMap.current.get(item.turnKey)
      turnTarget?.scrollIntoView({ behavior, block: 'center' })
    })()
  }, [findNavigationTarget, scrollToEstimatedTurn, waitForNavigationTarget])

  return (
    <Profiler id="message-timeline" onRender={recordReactCommit}>
      <InjectedMemoryLookupProvider workspaceRoot={workspaceRoot}>
      <div
        ref={containerRef}
        data-analytix-message-timeline-scroller
        className={`ds-no-drag relative flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden ${
          edgeAlignedScroll ? 'ds-stage-scroll-edge' : ''
        }`}
        style={{ overflowAnchor: 'none' }}
      >
      <ThreadUserMessageNavigationRail
        items={projection.userMessageNavigationItems}
        containerRef={containerRef}
        railLabel={t('timelineJumpRailLabel')}
        noContentLabel={t('timelineUserMessageNavigationNoContent')}
        itemAriaLabel={(item) => t('timelineJumpTurn', { index: item.position })}
        onSelectItem={revealNavigationItem}
      />
      <div
        data-analytix-message-timeline-content
        className={`ds-message-timeline-content ds-chat-column-inset mx-auto flex w-full min-w-0 max-w-4xl flex-col pt-8 ${contentClassName ?? ''} ${
        goalTimelinePaddingClass(heroRoute, Boolean(activeThreadGoal))
      }`}
      >
        {virtualizerEnabled && virtualWindow.beforeHeight > 0 ? (
          <div aria-hidden style={{ height: virtualWindow.beforeHeight }} className="w-full shrink-0" />
        ) : null}
        {renderedItems.map(({ row, index }) => {
          let content: ReactElement | null = null
          switch (row.kind) {
            case 'emptyHero':
              content = (
                <MessageTimelineEmptyHero
                  route={heroRoute}
                  ready={runtimeConnection === 'ready'}
                  hasWorkspace={!!workspaceRoot}
                  runtimeError={runtimeError}
                  activeClawChannel={activeClawChannel}
                  onPickWorkspace={() => void chooseWorkspace()}
                  onRetry={onRetryConnection}
                  onOpenSettings={onOpenSettings}
                  onSelectSuggestion={onSelectSuggestion}
                  focusModeEnabled={focusModeEnabled}
                />
              )
              break
            case 'forkBanner':
              content = <ThreadForkBanner parentTitle={row.parentTitle} />
              break
            case 'forkPoint':
              content = <ThreadForkPoint parentTitle={row.parentTitle} />
              break
            case 'loadEarlier':
              content = (
                <div className="flex items-center justify-center">
                  <button
                    type="button"
                    onClick={() => loadEarlierTurns({ userInitiated: true })}
                    className="ds-chip rounded-full px-4 py-2 text-[13px] font-medium text-ds-muted transition hover:text-ds-ink"
                  >
                    {t('timelineShowEarlierTurns', { count: Math.min(row.hiddenCount, row.pageSize) })}
                  </button>
                </div>
              )
              break
            case 'turn':
              content = (
                <MemoMessageTurn
                  activeThreadId={activeThreadId}
                  isLatest={row.turn.isLatest || row.turn.isLiveTurn}
                  activeStreamTurnId={
                    row.turn.isLiveTurn ? currentTurnId ?? runtimeTurnId(row.turn.turn) : runtimeTurnId(row.turn.turn)
                  }
                  workspaceRoot={workspaceRoot}
                  turn={row.turn.turn}
                  isProcessing={row.turn.isProcessing}
                  liveContent={row.turn.liveContent}
                  durationMs={row.turn.timing.durationMs}
                  userNavigationItemId={row.turn.userNavigationItemId}
                  devPreviewCard={row.turn.devPreviewCard}
                  planActionsBusy={planActionsBusy}
                  onBuildPlan={onBuildPlan}
                  onOpenPlan={onOpenPlan}
                  viewportRef={containerRef}
                  compactCards={compactCards}
                  sections={row.turn.sections}
                  reviewBlocks={row.turn.reviewBlocks}
                  onAssistantFinalized={snapToBottomIfNearLive}
                />
              )
              break
            case 'collapseEarlier':
              content = (
                <div className="flex items-center justify-center">
                  <button
                    type="button"
                    onClick={() => {
                      collapseEarlierTurns()
                    }}
                    className="rounded-full px-3 py-1.5 text-[12.5px] font-medium text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
                  >
                    {t('timelineCollapseEarlierTurns')}
                  </button>
                </div>
              )
              break
            case 'liveOnlyTurn':
              content = (
                <MemoMessageTurn
                  activeThreadId={activeThreadId}
                  isLatest
                  activeStreamTurnId={row.turnId ?? null}
                  workspaceRoot={workspaceRoot}
                  turn={{ blocks: [] }}
                  isProcessing={row.isProcessing}
                  liveContent={row.liveContent}
                  devPreviewCard={row.devPreviewCard}
                  viewportRef={containerRef}
                  compactCards={compactCards}
                  durationMs={row.timing.durationMs}
                  sections={row.sections}
                  reviewBlocks={row.reviewBlocks}
                  onAssistantFinalized={snapToBottomIfNearLive}
                />
              )
              break
            case 'bottomSpacer':
              content = (
                <div
                  ref={endRef}
                  aria-hidden
                  data-analytix-response-spacer={hasLiveResponseSpacer ? 'live' : 'idle'}
                  className="w-full shrink-0 transition-[height] duration-150 ease-out"
                  style={{ height: liveResponseSpacerHeight }}
                />
              )
              break
            default:
              content = null
          }

          return (
            <div
              key={row.id}
              ref={(node) => {
                registerThreadRow(row.id, node)
                if (row.kind === 'turn') {
                  if (node) turnRefMap.current.set(row.turn.key, node)
                  else turnRefMap.current.delete(row.turn.key)
                }
              }}
              className={row.kind === 'turn' ? 'scroll-mt-6' : undefined}
              data-analytix-timeline-row={row.kind}
              data-turn-key={row.kind === 'turn' ? row.turn.key : undefined}
              data-content-search-turn-key={row.kind === 'turn' ? row.turn.key : undefined}
              style={index < projection.rows.length - 1 ? { marginBottom: VIRTUALIZER_ROW_GAP_PX } : undefined}
            >
              {content}
            </div>
          )
        })}
        {virtualizerEnabled && virtualWindow.afterHeight > 0 ? (
          <div aria-hidden style={{ height: virtualWindow.afterHeight }} className="w-full shrink-0" />
        ) : null}
      </div>
      </div>
      </InjectedMemoryLookupProvider>
    </Profiler>
  )
}

function MessageTurn({
  activeThreadId,
  isLatest,
  activeStreamTurnId,
  workspaceRoot,
  turn,
  isProcessing,
  liveContent,
  durationMs,
  userNavigationItemId,
  devPreviewCard,
  planActionsBusy,
  onBuildPlan,
  onOpenPlan,
  viewportRef,
  compactCards = false,
  sections,
  reviewBlocks,
  onAssistantFinalized
}: {
  activeThreadId: string | null
  isLatest: boolean
  activeStreamTurnId?: string | null
  workspaceRoot: string
  turn: Turn
  isProcessing: boolean
  liveContent: string
  durationMs?: number
  userNavigationItemId?: string
  devPreviewCard?: ReactElement | null
  planActionsBusy?: boolean
  onBuildPlan?: () => void
  onOpenPlan?: () => void
  viewportRef: RefObject<HTMLDivElement | null>
  compactCards?: boolean
  sections: TurnSections
  reviewBlocks: Extract<ChatBlock, { kind: 'review' }>[]
  onAssistantFinalized?: () => void
}): ReactElement {
  const activeThreadGoal = useChatStore((s) => s.activeThreadGoal)
  const forkThreadFromTurn = useChatStore((s) => s.forkThreadFromTurn)
  const rollbackWorkspaceToCheckpoint = useChatStore((s) => s.rollbackWorkspaceToCheckpoint)
  const turnId = activeStreamTurnId ?? runtimeTurnId(turn)
  const activeStreamEstimate = useActiveStreamEstimateMetrics(isLatest ? activeThreadId : null, isLatest ? turnId : null)
  const [forking, setForking] = useState(false)
  const [rollingBackCheckpointId, setRollingBackCheckpointId] = useState<string | null>(null)
  // Inline Review Plan card: surfaced under a turn that produced a
  // successful `create_plan` result so the user can open/build the plan
  // without leaving the conversation.
  const planResult = useMemo(() => {
    if (isProcessing) return null
    for (let index = turn.blocks.length - 1; index >= 0; index -= 1) {
      const block = turn.blocks[index]
      if (block.kind !== 'tool' || block.status !== 'success') continue
      const meta = extractPlanMetadataFromBlock(block)
      if (meta) return meta
    }
    return null
  }, [turn.blocks, isProcessing])
  const [workExpandedOverride, setWorkExpandedOverride] = useState<boolean | null>(null)

  const {
    processBlocks,
    assistantContentBlocks,
    compactionBlocks,
    generatedFileBlocks,
    turnFileChanges
  } = sections
  const preferActiveLiveProcess = isLatest && activeStreamEstimate.processLength > 0
  const displayProcessBlocks = useMemo(
    () =>
      preferActiveLiveProcess
        ? processBlocks.filter((block) => block.id !== 'live-reasoning' && block.id !== 'live-assistant')
        : processBlocks,
    [preferActiveLiveProcess, processBlocks]
  )
  const liveProcessStepCount = preferActiveLiveProcess ? 1 : 0
  const hasProcessError = displayProcessBlocks.some(processBlockHasError)
  const sectionsHaveLiveProcess = displayProcessBlocks.some(
    (block) => block.id === 'live-reasoning' || block.id === 'live-assistant'
  )
  const hasPendingInteractiveProcess = displayProcessBlocks.some(
    (block) =>
      (block.kind === 'tool' && block.status === 'running') ||
      (block.kind === 'approval' && block.status === 'pending') ||
      (block.kind === 'user_input' && block.status === 'pending')
  )
  const effectiveProcessing = isProcessing || preferActiveLiveProcess
  const forceExpandForError = effectiveProcessing && hasProcessError
  const hasFinalAnswer = assistantContentBlocks.length > 0
  const workExpanded = forceExpandForError ||
    (workExpandedOverride ?? (effectiveProcessing || hasPendingInteractiveProcess || (hasProcessError && !hasFinalAnswer)))

  const processSections = useMemo(
    () => (workExpanded ? groupProcessSections(displayProcessBlocks) : []),
    [displayProcessBlocks, workExpanded]
  )
  // Live assistant text belongs in the normal answer flow while the turn is
  // still running; turn_completed only solidifies it into a persisted block.
  const forkTurnId = turnId?.trim() ?? ''
  const forkActionBlockId =
    !effectiveProcessing && forkTurnId
      ? assistantContentBlocks[assistantContentBlocks.length - 1]?.id
      : undefined
  const rollbackCheckpointId = turn.user?.meta?.workspaceCheckpointId?.trim() ?? ''
  const rollbackActionBlockId =
    !isProcessing && rollbackCheckpointId
      ? assistantContentBlocks[assistantContentBlocks.length - 1]?.id
      : undefined
  const forkFromTurn = async (): Promise<void> => {
    if (!forkTurnId || forking) return
    setForking(true)
    try {
      await forkThreadFromTurn(forkTurnId)
    } finally {
      setForking(false)
    }
  }
  const rollbackWorkspace = async (checkpointId: string): Promise<void> => {
    const targetCheckpointId = checkpointId.trim()
    if (!targetCheckpointId || rollingBackCheckpointId) return
    setRollingBackCheckpointId(targetCheckpointId)
    try {
      await rollbackWorkspaceToCheckpoint(targetCheckpointId)
    } finally {
      setRollingBackCheckpointId(null)
    }
  }

  // Keep pure completed reasoning tucked away, but surface concrete tool,
  // approval, user-input, and runtime process rows so turns never look final-only.

  const hasProcess = effectiveProcessing || displayProcessBlocks.length > 0

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {turn.user ? (
        <div data-content-search-unit-key={userNavigationItemId}>
          <MessageBubble block={turn.user} />
        </div>
      ) : null}

      {hasProcess ? (
        <div className="flex flex-col gap-1 pb-2">
          <WorkMetaRow
            processing={effectiveProcessing}
            stepCount={displayProcessBlocks.length + liveProcessStepCount}
            durationMs={durationMs}
            expanded={workExpanded}
            collapsible={!forceExpandForError}
            onToggle={() => setWorkExpandedOverride(!workExpanded)}
          />
          {workExpanded && processSections.length > 0 ? (
            <div className="flex flex-col gap-1">
              {processSections.map((section) => (
                <ProcessSectionRow
                  key={section.id}
                  section={section}
                  processing={effectiveProcessing}
                  viewportRef={viewportRef}
                />
              ))}
            </div>
          ) : null}
          {isLatest && workExpanded && !sectionsHaveLiveProcess ? (
            <LiveProcessRows
              activeThreadId={activeThreadId}
              turnId={turnId}
              workspaceRoot={workspaceRoot}
              isProcessing={isProcessing}
              durationMs={durationMs}
              viewportRef={viewportRef}
              showMeta={false}
            />
          ) : null}
        </div>
      ) : null}

      {!hasProcess && isLatest ? (
        <LiveProcessRows
          activeThreadId={activeThreadId}
          turnId={turnId}
          workspaceRoot={workspaceRoot}
          isProcessing={isProcessing}
          durationMs={durationMs}
          viewportRef={viewportRef}
          showMeta
        />
      ) : null}

      {assistantContentBlocks.map((block) => (
        <MessageBubble
          key={block.id}
          block={block}
          onAssistantFinalized={onAssistantFinalized}
          forkAction={
            block.id === forkActionBlockId
              ? {
                  busy: forking,
                  onFork: () => {
                    void forkFromTurn()
                  }
                }
              : undefined
          }
          rollbackAction={
            block.id === rollbackActionBlockId
              ? {
                  busy: rollingBackCheckpointId === rollbackCheckpointId,
                  onRollback: () => {
                    void rollbackWorkspace(rollbackCheckpointId)
                  }
                }
              : undefined
          }
        />
      ))}

      {compactionBlocks.map((block) => (
        <CompactionStatusRow key={block.id} block={block} />
      ))}

      {isLatest ? (
        <LiveAssistantBubble
          activeThreadId={activeThreadId}
          turnId={turnId}
          fallbackText={liveContent}
        />
      ) : liveContent.trim() ? (
        <MessageBubble block={{ kind: 'assistant', id: 'live-assistant', text: liveContent }} />
      ) : null}

      <GeneratedFilesPanel blocks={generatedFileBlocks} />

      {reviewBlocks.map((review) => (
        <ReviewSummaryCard key={review.id} review={review} />
      ))}

      {isProcessing ? <LiveTurnProgressRow hasActiveGoal={Boolean(activeThreadGoal)} /> : null}

      {!isProcessing && devPreviewCard ? devPreviewCard : null}

      {planResult ? (
        <ReviewPlanCard
          title={planResult.title?.trim() || planDisplayNameFromRelativePath(planResult.relativePath)}
          relativePath={planResult.relativePath}
          busy={planActionsBusy === true}
          onOpen={onOpenPlan}
          onBuild={onBuildPlan}
        />
      ) : null}

      {!isProcessing && turnFileChanges.length > 0 ? (
        <TurnChangeSummary changes={turnFileChanges} viewportRef={viewportRef} compact={compactCards} />
      ) : null}
    </div>
  )
}

function LiveTurnProgressRow({ hasActiveGoal }: { hasActiveGoal: boolean }): ReactElement {
  const { t, i18n } = useTranslation('common')
  const swimMode = useWorkLogoSwimMode(true)
  const mascotVariant = useMascotWorkLogoVariant(true)
  // mascot 模式是全局 html 属性;进行行每个回合重新挂载,挂载时读取即可
  const [mascotModeOn] = useState(
    () =>
      typeof document !== 'undefined' &&
      document.documentElement.getAttribute('data-mascot-mode') === 'on'
  )
  const swimLabelKey = WORK_LOGO_SWIM_MODE_LABEL_KEYS[swimMode]
  // UI 插件可声明自己的进行中文案(按泳姿键、按语言),未声明则用默认文案
  const pluginLabel = useUiPluginWorkLabel(
    swimLabelKey as UiPluginLabelKey,
    i18n.language ?? 'zh'
  )
  const label = mascotModeOn
    ? t(MASCOT_WORK_LOGO_VARIANT_LABEL_KEYS[mascotVariant])
    : pluginLabel ?? t(swimLabelKey)

  return (
    <div className={liveTurnProgressClass(hasActiveGoal)} data-analytix-live-progress>
      <span className="ds-work-logo-slot ds-work-logo-slot-sm mr-0.5">
        <AnimatedWorkLogo active mascotVariant={mascotVariant} mode={swimMode} phase="trail" size="sm" />
      </span>
      <span className="ds-shiny-text">{label}</span>
    </div>
  )
}

const LiveAssistantBubble = memo(function LiveAssistantBubble({
  activeThreadId,
  turnId,
  fallbackText
}: {
  activeThreadId: string | null
  turnId: string | null
  fallbackText: string
}): ReactElement | null {
  const activeStream = useActiveStreamSnapshot(activeThreadId, turnId)
  const text = activeStream.liveAssistantContent.trim() ? activeStream.liveAssistantContent : fallbackText
  if (!text.trim()) return null
  return (
    <div data-analytix-live-assistant>
      <MessageBubble block={{ kind: 'assistant', id: 'live-assistant', text }} />
    </div>
  )
})

const LiveProcessRows = memo(function LiveProcessRows({
  activeThreadId,
  turnId,
  workspaceRoot,
  isProcessing,
  durationMs,
  viewportRef,
  showMeta
}: {
  activeThreadId: string | null
  turnId: string | null
  workspaceRoot: string
  isProcessing: boolean
  durationMs?: number
  viewportRef: RefObject<HTMLDivElement | null>
  showMeta: boolean
}): ReactElement | null {
  void activeThreadId
  void turnId
  void workspaceRoot
  void isProcessing
  void durationMs
  void viewportRef
  void showMeta
  return null
})

const MemoMessageTurn = memo(MessageTurn, (prev, next) => (
  prev.activeThreadId === next.activeThreadId &&
  prev.isLatest === next.isLatest &&
  prev.activeStreamTurnId === next.activeStreamTurnId &&
  prev.workspaceRoot === next.workspaceRoot &&
  sameTurnContent(prev.turn, next.turn) &&
  prev.isProcessing === next.isProcessing &&
  prev.liveContent === next.liveContent &&
  prev.durationMs === next.durationMs &&
  prev.userNavigationItemId === next.userNavigationItemId &&
  prev.devPreviewCard === next.devPreviewCard &&
  prev.planActionsBusy === next.planActionsBusy &&
  prev.onBuildPlan === next.onBuildPlan &&
  prev.onOpenPlan === next.onOpenPlan &&
  prev.compactCards === next.compactCards &&
  prev.viewportRef === next.viewportRef &&
  sameTurnSections(prev.sections, next.sections) &&
  sameBlockList(prev.reviewBlocks, next.reviewBlocks)
))
