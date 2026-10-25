import type { ConversationPresentationPhase } from './ConversationWidthControls'
import type { ProcessPaneRoute } from './presentation/chat/ProtectedProcessPane'
import { ConversationRecords, ConversationRecordDetails, ConversationRecordsToolbar, type ConversationViewMode } from './presentation/chat/ConversationRecords'
import { TrajectoryPanel } from '../trajectory/TrajectoryPanel'
import type { TrajectoryToolIdentity } from '../trajectory/trajectory-adapter'
import { toolResultLocalSelectorSchemaV1 } from '../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { conversationCallRecords, conversationRecordTurns, locateConversationCall, type ConversationCallRecord, type ConversationInspectRequest } from './presentation/chat/models/conversation-record-model'
import { sharedActiveCaseContextTransitionPending, subscribeBeforeSharedActiveCaseContextChange } from '../../data-analysis/services/analysis/stats-case-overview-resource'
import { ChatView } from './presentation/chat/ChatView'
import { usePerformanceUsageMode, type PerformanceUsageMode } from './presentation/chat/TurnUsagePanel'
import { ProtectedToolInspectorSelection } from './presentation/ProtectedToolResult'
import { useTurnTiming } from './presentation/chat/use-turn-timing'
import { GateResponseContext } from './message-timeline-bubbles'
import type { ProfilerOnRenderCallback, ReactElement, RefObject } from 'react'
import { Profiler, memo, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { ChatBlock, RuntimeConnectionStatus, ThreadUsageSnapshot } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import type { TurnForkOwner } from '../../store/chat-store-types'
import { useThreadUsageState } from '../../hooks/use-thread-usage'
import type { ConversationUsageV1, ProviderBillingV1 } from '../../../../../packages/runtime/src/contracts/usage'
import { useTimelineStores } from './use-timeline-stores'
import { TIMELINE_AT_BOTTOM_PX, useTimelineScroll, type TimelineStreamingPhase } from './use-timeline-scroll'
import type { TurnSections } from '../../thread/projection/derive-turn-sections'
import { MessageTimelineEmptyHero, ThreadForkBanner, ThreadForkPoint } from './message-timeline-empty'
import { MessageBubble } from './message-timeline-bubbles'
import { ReviewSummaryCard, TurnChangeSummary } from './message-timeline-cards'
import { ThreadUserMessageNavigationRail } from './ThreadUserMessageNavigationRail'
import { InjectedMemoryLookupProvider } from './injected-memory-lookup'
import { SubmittedPlanCards } from './presentation/chat/SubmittedPlanCards'
import { buildThreadProjection } from '../../thread/projection/thread-projection-store'
import {
  groupThreadTurns, runtimeTurnId, stableThreadTurnKey,
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
  isPendingActiveStreamTurnId,
  useActiveStreamMetrics,
  useActiveStreamSnapshot
} from '../../thread/streaming/active-stream-store'

export { summarizeToolBlock } from './presentation/chat/chat-view-model'

const ESTIMATED_INITIAL_VIEWPORT_HEIGHT = 720
const EMPTY_LOCAL_TIMING_MAP: Record<string, number> = {}

type TimelineInspectRequest = ConversationInspectRequest & {
  view: 'result' | 'inspect'
  trajectory?: { generation: number; recordKey: string; sourceScope: string }
}

// Bind one explicit click to the existing physical identity and disclosure scope.
// This contains only the public selector, never a protected payload.
function trajectoryInspectorSource(record: ConversationCallRecord): string {
  const { block } = record
  const selector = toolResultLocalSelectorSchemaV1.safeParse({ ...(block.meta?.localResult ?? {}), threadId: record.threadId })
  return JSON.stringify([record.key, Boolean(block.meta?.localResult), selector.success ? selector.data : null,
    block.status, block.meta?.toolName ?? null, block.toolKind ?? null])
}

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
  | 'authoritativeTurnTimings'
  | 'authoritativeTurnStatuses'
  | 'lastTurnUsage'
  | 'turnUsageById'
>>

type Props = {
  blocks: ChatBlock[]
  live: string
  activeThreadId: string | null
  runtimeConnection: RuntimeConnectionStatus
  runtimeError?: string | null
  conversationPhase?: ConversationPresentationPhase
  onRetryConnection: () => void
  onOpenSettings: () => void
  onSelectSuggestion?: (prompt: string) => void
  devPreviewCard?: ReactElement | null
  compactCards?: boolean
  edgeAlignedScroll?: boolean
  contentClassName?: string
  processPaneRoute?: ProcessPaneRoute
  runtimeStateOverride?: TimelineRuntimeStateOverride
  captureForkOwner?: () => TurnForkOwner | null
  conversationUsage?: ConversationUsageV1 | null
  focusRequestId?: string | null
  onRequestFocused?: () => void
  onReturnToBottomStateChange?: (state: TimelineReturnToBottomState | null) => void
  /** Undefined retains the local toolbar; null waits for its resident header seat. */
  conversationTabsTarget?: HTMLElement | null
}

const TURN_PAGE_SIZE = 18
const AUTO_COLLAPSE_THRESHOLD = 24
const VIRTUALIZER_ROW_THRESHOLD = 48
const VIRTUALIZER_OVERSCAN_PX = 1600
const CHAT_TURN_GAP_PX = 16
const ASSISTANT_PANEL_ROW_GAP_PX = 32
const MIN_LIVE_RESPONSE_SPACER_PX = 72
const MAX_LIVE_RESPONSE_SPACER_PX = 180

export function timelineResponseSpacerHeight(live: boolean): number | string {
  return live
    ? `clamp(${MIN_LIVE_RESPONSE_SPACER_PX}px, 16vh, ${MAX_LIVE_RESPONSE_SPACER_PX}px)`
    : 1
}

// The goal lives in the measured composer surface; it does not overlay the timeline.
export function goalTimelinePaddingClass(_route: 'chat' | 'claw', _hasActiveGoal: boolean): string {
  return 'pb-10'
}

export function liveTurnProgressClass(_hasActiveGoal: boolean): string {
  return 'flex w-fit max-w-full items-center gap-2 py-0.5 text-[14px] font-medium text-ds-muted'
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

function turnPreview(turn: Turn, fallback: string): string {
  const text = turn.user?.text.trim() ?? ''
  if (!text) return fallback
  const oneLine = text.replace(/\s+/g, ' ')
  return oneLine.length > 48 ? `${oneLine.slice(0, 47).trimEnd()}...` : oneLine
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
  conversationPhase,
  onRetryConnection,
  onOpenSettings,
  onSelectSuggestion,
  devPreviewCard,
  compactCards = false,
  edgeAlignedScroll = false,
  contentClassName,
  runtimeStateOverride,
  captureForkOwner,
  conversationUsage: suppliedConversationUsage,
  processPaneRoute,
  onReturnToBottomStateChange,
  conversationTabsTarget,
  focusRequestId,
  onRequestFocused
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const rowGapPx = edgeAlignedScroll || processPaneRoute !== undefined ? CHAT_TURN_GAP_PX : ASSISTANT_PANEL_ROW_GAP_PX
  const timelineStores = useTimelineStores(activeThreadId)
  const performanceUsageMode = usePerformanceUsageMode()
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
    activeThread,
    turnUsageById
  } = {
    ...timelineStores,
    ...(runtimeStateOverride ? { turnStartedAtByUserId: EMPTY_LOCAL_TIMING_MAP, turnDurationByUserId: EMPTY_LOCAL_TIMING_MAP, turnUsageById: {}, ...runtimeStateOverride } : {})
  }

  const [processPaneOwnerKey] = useState(() => crypto.randomUUID())
  const mainThreadId = useChatStore(state => state.activeThreadId)
  const secondaryProcessContext = useContext(GateResponseContext)
  const usageRefreshKey = useChatStore(state => state.usageRefreshKey)
  const rewoundTurnIds = useChatStore(state => state.rewoundTurnIds)
  const wholeLog = useThreadUsageState(activeThreadId,
    suppliedConversationUsage === undefined && runtimeConnection === 'ready' && !secondaryProcessContext && activeThreadId === mainThreadId,
    `${usageRefreshKey}:${busy ? 'running' : 'settled'}:${currentTurnId ?? ''}`,
    `${runtimeConnection}:${JSON.stringify(Object.keys(rewoundTurnIds ?? {}).sort())}`)
  const conversationUsage = suppliedConversationUsage === undefined ? wholeLog.conversationUsage : suppliedConversationUsage
  const turnBilling = (turnId: string | null): ProviderBillingV1 | undefined => {
    if (!turnId || !conversationUsage) return undefined
    const matches = conversationUsage.turns.filter(turn => turn.turnId === turnId)
    return matches.length === 1 ? matches[0].billing : undefined
  }
  const resolvedProcessPaneRoute = useMemo<ProcessPaneRoute | undefined>(() => {
    if (processPaneRoute !== undefined) return processPaneRoute
    // This is the current main transcript's mounting signature, not authority.
    if (!edgeAlignedScroll || secondaryProcessContext || !activeThreadId || activeThreadId !== mainThreadId) return undefined
    return { surface: 'main', ownerKey: processPaneOwnerKey, selectionKey: JSON.stringify([route, activeThreadId]), visible: true }
  }, [processPaneRoute, edgeAlignedScroll, secondaryProcessContext, activeThreadId, mainThreadId, processPaneOwnerKey, route])

  // Local committed selection token, independent of tool/Question authority.
  const forkSelectionKey = JSON.stringify([activeThreadId, route, resolvedProcessPaneRoute?.selectionKey,
    resolvedProcessPaneRoute?.visible, Boolean(secondaryProcessContext)])
  const forkToken = useMemo(() => ({ selectionKey: forkSelectionKey }), [forkSelectionKey])
  const committedForkToken = useRef<object | null>(null)
  useLayoutEffect(() => {
    committedForkToken.current = forkToken
    return () => { if (committedForkToken.current === forkToken) committedForkToken.current = null }
  }, [forkToken])
  const captureTimelineForkOwner = useCallback((): TurnForkOwner | null => {
    if (!activeThreadId || committedForkToken.current !== forkToken || resolvedProcessPaneRoute?.visible === false) return null
    const external = captureForkOwner?.()
    if (!external && (secondaryProcessContext || activeThreadId !== useChatStore.getState().activeThreadId)) return null
    return {
      isCurrent: () => committedForkToken.current === forkToken && (external?.isCurrent() ?? useChatStore.getState().activeThreadId === activeThreadId),
      getSnapshot: () => {
        if (committedForkToken.current !== forkToken) return null
        if (external) return external.getSnapshot()
        const state = useChatStore.getState()
        return state.activeThreadId === activeThreadId ? { blocks: state.blocks, busy: state.busy,
          latestSeq: state.lastSeq, rewoundTurnIds: state.rewoundTurnIds } : null
      }
    }
  }, [activeThreadId, forkToken, captureForkOwner, resolvedProcessPaneRoute?.visible, secondaryProcessContext])

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

  const recordScope = JSON.stringify([activeThreadId, workspaceRoot, route, runtimeConnection, resolvedProcessPaneRoute?.ownerKey,
    resolvedProcessPaneRoute?.selectionKey, resolvedProcessPaneRoute?.surface, resolvedProcessPaneRoute?.visible])
  const [recordsState, setRecordsState] = useState<{ scope: string; mode: ConversationViewMode; request: TimelineInspectRequest | null; applied: number | null } | null>(null)
  const recordVersion = useRef(0)
  const recordsCurrent = recordsState?.scope === recordScope ? recordsState : null
  const viewMode = recordsCurrent?.mode ?? 'conversation'
  const recordsMode = viewMode === 'records'
  const trajectoryMode = viewMode === 'trajectory'
  const currentViewMode = useRef(viewMode)
  currentViewMode.current = viewMode
  const viewSeat = useRef({ scope: recordScope, mode: viewMode, generation: 0 })
  if (viewSeat.current.scope !== recordScope || viewSeat.current.mode !== viewMode) {
    viewSeat.current = { scope: recordScope, mode: viewMode, generation: viewSeat.current.generation + 1 }
  }
  const viewGeneration = viewSeat.current.generation
  const turns = useMemo(() => recordsMode ? conversationRecordTurns(activeThreadId, blocks) : groupThreadTurns(blocks), [recordsMode, activeThreadId, blocks])
  const recordRequest = recordsMode ? recordsCurrent?.request ?? null : null
  const trajectoryRequest = trajectoryMode ? recordsCurrent?.request ?? null : null
  const records = useMemo(() => conversationCallRecords(activeThreadId, blocks), [activeThreadId, blocks])
  const currentRecords = useRef(records)
  currentRecords.current = records
  const trajectoryLocated = trajectoryRequest ? locateConversationCall(records, trajectoryRequest) : null
  const trajectoryRecord = trajectoryLocated?.kind === 'found' && trajectoryRequest?.trajectory?.generation === viewGeneration &&
    trajectoryLocated.record.key === trajectoryRequest.trajectory.recordKey &&
    trajectoryInspectorSource(trajectoryLocated.record) === trajectoryRequest.trajectory.sourceScope ? trajectoryLocated.record : null
  const locatedRecord = recordRequest ? locateConversationCall(records, recordRequest) : null
  const foundRecord = locatedRecord?.kind === 'found' ? locatedRecord.record : null
  const selectedRecord = recordsCurrent?.applied === recordRequest?.version ? foundRecord : null
  const currentRecordScope = useRef(recordScope)
  currentRecordScope.current = recordScope
  const currentRecordRequest = useRef(recordRequest ?? trajectoryRequest)
  currentRecordRequest.current = recordRequest ?? trajectoryRequest
  const trajectoryClose = useRef<{ scope: string; generation: number; version: number; close: () => void } | null>(null)
  const pauseRecordScroll = useRef<() => void>(() => undefined)
  const pauseViewScroll = useRef<() => void>(() => undefined)
  const viewScroll = useRef<{ scope: string; positions: Partial<Record<ConversationViewMode, number>> }>({ scope: recordScope, positions: {} })
  const captureViewScroll = useCallback(() => {
    if (currentRecordScope.current !== recordScope || currentViewMode.current !== viewMode) return
    if (viewScroll.current.scope !== recordScope) viewScroll.current = { scope: recordScope, positions: {} }
    const el = containerRef.current
    if (el) viewScroll.current.positions[viewMode] = el.scrollTop
    pauseViewScroll.current()
  }, [recordScope, viewMode])
  const recordFocusedRow = useRef<HTMLElement | null>(null)
  const recordReturnFocus = useRef<HTMLElement | null>(null)
  const visibleRecordsRoot = useCallback(() => {
    if (document.hidden || sharedActiveCaseContextTransitionPending() || resolvedProcessPaneRoute?.visible === false) return false
    const root = containerRef.current
    if (!root?.isConnected) return false
    for (let node: HTMLElement | null = root; node; node = node.parentElement) {
      const style = getComputedStyle(node)
      if (node.hidden || node.inert || node.hasAttribute('inert') || node.getAttribute('aria-hidden') === 'true' || style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return false
    }
    return true
  }, [resolvedProcessPaneRoute?.visible])
  const retireRecordRequest = useCallback(() => {
    recordVersion.current += 1
    if (currentRecordRequest.current) pauseRecordScroll.current()
    const request = currentRecordRequest.current
    const inline = trajectoryClose.current
    currentRecordRequest.current = null
    trajectoryClose.current = null
    if (inline && request && inline.version === request.version && inline.scope === currentRecordScope.current && inline.generation === viewSeat.current.generation) inline.close()
    setRecordsState(current => current ? { ...current, request: null, applied: null } : null)
  }, [])
  useLayoutEffect(() => {
    retireRecordRequest()
    setRecordsState(current => current?.scope === recordScope ? current : null)
    const api = window.analytix?.runtime
    const off = api?.onToolResultLocalDisplayInvalidated?.(id => { if (id === null) retireRecordRequest() })
    const runtimeOff = api?.onRuntimeStatus?.(status => { if (status.state !== 'running') retireRecordRequest() })
    const caseOff = subscribeBeforeSharedActiveCaseContextChange(retireRecordRequest)
    const hidden = () => { if (document.hidden) retireRecordRequest() }
    const observer = new MutationObserver(() => { if (!visibleRecordsRoot()) retireRecordRequest() })
    for (let node: HTMLElement | null = containerRef.current; node; node = node.parentElement) observer.observe(node, { attributes: true, attributeFilter: ['hidden', 'inert', 'aria-hidden', 'style', 'class'] })
    document.addEventListener('visibilitychange', hidden)
    return () => { currentRecordRequest.current = null; recordVersion.current += 1; off?.(); runtimeOff?.(); caseOff(); observer.disconnect(); document.removeEventListener('visibilitychange', hidden) }
  }, [recordScope, retireRecordRequest, visibleRecordsRoot])
  const inspectConversationCall = useCallback((turnId: string, callId: string, physical?: ConversationCallRecord, view: 'result' | 'inspect' = 'inspect') => {
    if (!activeThreadId || currentRecordScope.current !== recordScope || currentViewMode.current !== viewMode || viewSeat.current.generation !== viewGeneration || !visibleRecordsRoot()) return
    captureViewScroll()
    recordReturnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const matches = records.filter(record => record.threadId === activeThreadId && record.turnId === turnId && record.callId === callId)
    const source = physical ?? (matches.length === 1 ? matches[0] : undefined)
    const request: TimelineInspectRequest = { threadId: activeThreadId, turnId, callId, view, version: ++recordVersion.current,
      ...(source?.toolCallItemId ? { toolCallItemId: source.toolCallItemId } : {}), ...(source?.toolResultItemId ? { toolResultItemId: source.toolResultItemId } : {}) }
    currentRecordRequest.current = request
    setRecordsState({ scope: recordScope, mode: 'records', request, applied: null })
  }, [activeThreadId, recordScope, records, visibleRecordsRoot, viewMode, captureViewScroll, viewGeneration])
  const changeConversationView = useCallback((mode: ConversationViewMode) => {
    if (!activeThreadId || mode === viewMode || currentRecordScope.current !== recordScope || currentViewMode.current !== viewMode || viewSeat.current.generation !== viewGeneration || !visibleRecordsRoot()) return
    captureViewScroll()
    retireRecordRequest()
    setRecordsState({ scope: recordScope, mode, request: null, applied: null })
  }, [activeThreadId, viewMode, recordScope, visibleRecordsRoot, captureViewScroll, retireRecordRequest, viewGeneration])
  const mainConversation = route === 'chat' && !secondaryProcessContext && activeThreadId === mainThreadId
  const hideBlankMainTabs = mainConversation && (conversationPhase === 'hero' || conversationPhase === 'settling')
  const viewToolbar = activeThreadId && !hideBlankMainTabs ? <ConversationRecordsToolbar key={recordScope} mode={viewMode}
    pending={recordRequest && !selectedRecord ? locatedRecord?.kind === 'ambiguous' ? 'ambiguous' : 'missing' : undefined}
    onModeChange={changeConversationView} /> : null
  const residentTabsTarget = mainConversation ? conversationTabsTarget : undefined
  const inspectTrajectoryCall = useCallback((identity: TrajectoryToolIdentity) => {
    if (currentViewMode.current !== 'trajectory' || viewSeat.current.generation !== viewGeneration || currentRecordScope.current !== recordScope || identity.threadId !== activeThreadId || !identity.toolResultItemId || !visibleRecordsRoot()) return
    const found = locateConversationCall(currentRecords.current, identity)
    if (found.kind !== 'found') return
    recordReturnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const source = found.record
    const request: TimelineInspectRequest = { threadId: source.threadId, turnId: source.turnId, callId: source.callId,
      ...(source.toolCallItemId ? { toolCallItemId: source.toolCallItemId } : {}), toolResultItemId: source.toolResultItemId,
      view: 'inspect', version: ++recordVersion.current,
      trajectory: { generation: viewGeneration, recordKey: source.key, sourceScope: trajectoryInspectorSource(source) } }
    currentRecordRequest.current = request
    setRecordsState({ scope: recordScope, mode: 'trajectory', request, applied: null })
  }, [activeThreadId, recordScope, visibleRecordsRoot, viewGeneration])
  useLayoutEffect(() => {
    if (trajectoryRequest && (!trajectoryRecord || !visibleRecordsRoot())) retireRecordRequest()
  }, [trajectoryRequest, trajectoryRecord, visibleRecordsRoot, retireRecordRequest])
  const renderTrajectoryInspector = useCallback((identity: TrajectoryToolIdentity, close: () => void) => {
    const request = trajectoryRequest
    const expected = request?.trajectory
    const current = () => {
      const active = currentRecordRequest.current
      return Boolean(request && expected && active && currentRecordScope.current === recordScope &&
        currentViewMode.current === 'trajectory' && viewSeat.current.generation === expected.generation &&
        active.version === request.version && active.trajectory?.recordKey === expected.recordKey &&
        active.trajectory.sourceScope === expected.sourceScope)
    }
    if (!current() || !request || !expected || !trajectoryRecord || identity.threadId !== activeThreadId ||
      !identity.toolResultItemId || !visibleRecordsRoot()) return null
    const found = locateConversationCall(currentRecords.current, identity)
    if (found.kind !== 'found' || found.record.key !== expected.recordKey ||
      trajectoryInspectorSource(found.record) !== expected.sourceScope) return null
    trajectoryClose.current = { scope: recordScope, generation: expected.generation, version: request.version, close }
    const retire = () => { if (current()) retireRecordRequest() }
    return <ConversationRecordDetails key={expected.recordKey + ':' + request.version} record={found.record}
      requestVersion={request.version} initialView="inspect" presentation="embedded" onRetire={retire} onClose={() => {
        if (!current()) return
        retireRecordRequest()
        const target = recordReturnFocus.current
        if (target?.isConnected && visibleRecordsRoot()) target.focus({ preventScroll: true })
      }} />
  }, [trajectoryRequest, trajectoryRecord, recordScope, activeThreadId, visibleRecordsRoot, retireRecordRequest])
  const closeRecordDetails = useCallback(() => {
    retireRecordRequest()
    const target = recordReturnFocus.current?.isConnected ? recordReturnFocus.current : recordFocusedRow.current
    if (target?.isConnected && visibleRecordsRoot()) target.focus({ preventScroll: true })
  }, [retireRecordRequest, visibleRecordsRoot])
  const activeStreamForScroll = useActiveStreamEstimateMetrics(activeThreadId, currentTurnId)
  const currentRuntimeStream = useActiveStreamMetrics(activeThreadId, currentTurnId)
  const runtimeRunning = Boolean(currentTurnId && !isPendingActiveStreamTurnId(currentTurnId) &&
    currentRuntimeStream.threadId === activeThreadId && currentRuntimeStream.turnId === currentTurnId && currentRuntimeStream.runtimeRunning)
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
    pauseFollowForNavigation,
    pauseForViewChange,
    snapToBottomIfNearLive
  } = useTimelineScroll({
    containerRef,
    activeThreadId,
    pageSize: TURN_PAGE_SIZE,
    autoCollapseThreshold: AUTO_COLLAPSE_THRESHOLD,
    totalTurns: turns.length,
    busy,
    suspended: trajectoryMode,
    scrollDeps: {
      contentKey: scrollContentKey,
      streaming: hasLegacyLiveStream || hasActiveStreamShell,
      userTurnKey: currentTurnUserId ?? '',
      phase: latestTurnPhase
    },
    onAnchorCorrected: recordAnchorCorrection
  })
  pauseRecordScroll.current = pauseFollowForNavigation
  pauseViewScroll.current = pauseForViewChange
  useLayoutEffect(() => {
    if (viewScroll.current.scope !== recordScope) {
      viewScroll.current = { scope: recordScope, positions: {} }
      return
    }
    const saved = viewScroll.current.positions[viewMode]
    const el = containerRef.current
    if (!el) return
    if (saved !== undefined || trajectoryMode) {
      if (!trajectoryMode) pauseFollowForNavigation()
      el.scrollTop = saved ?? 0
    }
  }, [recordScope, viewMode, trajectoryMode, pauseFollowForNavigation])
  const hasLiveActivity = hasLegacyLiveStream || hasActiveStreamShell
  const effectiveResponseSpacerHeightPx = hasLiveResponseSpacer
    ? responseSpacerHeightPx || MIN_LIVE_RESPONSE_SPACER_PX
    : 0
  const handleReturnToBottomClick = useCallback((): void => {
    scrollToBottom({ behavior: 'smooth' })
  }, [scrollToBottom])

  useLayoutEffect(() => {
    if (trajectoryMode) return
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
  }, [hasLiveResponseSpacer, liveResponseSpacerHeight, trajectoryMode])

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

  const timingSettlementKey = `${busy ? 'running' : 'settled'}:${currentTurnId ?? ''}:${turns.length}`
  const { timings: authoritativeTimings, statuses: authoritativeStatuses } = useTurnTiming(activeThreadId, timingSettlementKey,
    runtimeStateOverride?.authoritativeTurnTimings, runtimeStateOverride?.authoritativeTurnStatuses)

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
        authoritativeTurnTimings: authoritativeTimings,
        authoritativeTurnStatuses: authoritativeStatuses,
        runtimeRunning,
        hiddenTurnCount: turns.length > AUTO_COLLAPSE_THRESHOLD ? hiddenTurnCount : 0,
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
      authoritativeTimings,
      authoritativeStatuses,
      runtimeRunning,
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
    if (trajectoryMode) { onReturnToBottomStateChange(null); return }
    onReturnToBottomStateChange({
      show: showReturnToBottom,
      hasLiveActivity,
      onClick: handleReturnToBottomClick
    })
  }, [
    handleReturnToBottomClick,
    hasLiveActivity,
    onReturnToBottomStateChange,
    showReturnToBottom,
    trajectoryMode
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
  useLayoutEffect(() => {
    rowCacheRef.current.prune([])
    setMeasurementRevision(value => value + 1)
  }, [viewMode])
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
    if (trajectoryMode) return
    const el = containerRef.current
    if (!el) return
    let frame: number | null = null
    let retired = false
    const update = (): void => {
      frame = null
      if (retired || currentViewMode.current === 'trajectory') return
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
      retired = true
      el.removeEventListener('scroll', scheduleUpdate)
      observer?.disconnect()
      if (frame !== null) window.cancelAnimationFrame(frame)
    }
  }, [containerRef, trajectoryMode])

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
        rowGapPx
      })
    },
    [measurementRevision, projection.rows, viewportMetrics.scrollTop, viewportMetrics.viewportHeight, rowGapPx]
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
    if (currentViewMode.current === 'trajectory') return false
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
      offsetTop += height + (index < projection.rows.length - 1 ? rowGapPx : 0)
    }

    const target = turnRefMap.current.get(turnKey)
    if (!target) return false
    target.scrollIntoView({ behavior, block: 'center' })
    return true
  }, [projection.rows, rowGapPx])

  const waitForNavigationTarget = useCallback(async (
    item: ThreadUserMessageNavigationItem,
    isCurrent: () => boolean,
    timeoutMs = 2000
  ): Promise<HTMLElement | null> => {
    const startedAt = performance.now()
    while (isCurrent() && performance.now() - startedAt <= timeoutMs) {
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
      const isCurrent = () => currentRecordScope.current === recordScope && currentViewMode.current === 'conversation' && viewSeat.current.generation === viewGeneration && visibleRecordsRoot()
      if (!isCurrent()) return
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
      const renderedTarget = await waitForNavigationTarget(item, isCurrent)
      if (!isCurrent()) return
      if (renderedTarget) {
        renderedTarget.scrollIntoView({ behavior, block: 'center' })
        highlightNavigationTarget(renderedTarget)
        return
      }

      const turnTarget = turnRefMap.current.get(item.turnKey)
      turnTarget?.scrollIntoView({ behavior, block: 'center' })
    })()
  }, [findNavigationTarget, scrollToEstimatedTurn, waitForNavigationTarget, recordScope, viewGeneration, visibleRecordsRoot])

  useLayoutEffect(() => {
    if (!recordRequest || recordsCurrent?.applied === recordRequest.version || !foundRecord) return
    const target = foundRecord
    const matches = turns.map((turn, index) => ({ turn, index })).filter(({ turn }) => runtimeTurnId(turn) === target.turnId && turn.blocks.includes(target.block))
    if (matches.length !== 1 || !visibleRecordsRoot()) return
    pauseFollowForNavigation()
    const { turn, index } = matches[0]
    if (turns.length - index > visibleTurnCount) {
      loadEarlierTurns({ userInitiated: true, preserveScroll: false })
      return
    }
    const current = () => currentRecordScope.current === recordScope && currentRecordRequest.current?.version === recordRequest.version &&
      currentRecordRequest.current?.threadId === activeThreadId && visibleRecordsRoot()
    if (!current()) return
    scrollToEstimatedTurn(stableThreadTurnKey(turn, index), 'auto')
    let cancelled = false
    void (async () => {
      const started = performance.now()
      while (!cancelled && current() && performance.now() - started < 2000) {
        const nodes = Array.from(containerRef.current?.querySelectorAll<HTMLElement>('[data-conversation-call-key]') ?? [])
          .filter(node => node.dataset.conversationCallKey === target.key && node.closest<HTMLElement>('[data-conversation-record-turn]')?.dataset.conversationRecordTurn === target.turnId)
        if (nodes.length === 1 && current()) {
          nodes[0].scrollIntoView({ behavior: 'auto', block: 'center' })
          recordFocusedRow.current = nodes[0]
          nodes[0].focus({ preventScroll: true })
          setRecordsState(value => value?.scope === recordScope && value.request?.version === recordRequest.version ? { ...value, applied: recordRequest.version } : value)
          return
        }
        await nextAnimationFrame()
      }
    })()
    return () => { cancelled = true }
  }, [recordRequest, foundRecord, recordsCurrent?.applied,
    turns, visibleTurnCount, loadEarlierTurns, pauseFollowForNavigation, scrollToEstimatedTurn, recordScope, activeThreadId, visibleRecordsRoot])

  useEffect(() => {
    if (trajectoryMode || !focusRequestId) return undefined
    const turnIndex = turns.findIndex(turn => turn.blocks.some(block =>
      (block.kind === 'user_input' && block.requestId === focusRequestId) ||
      (block.kind === 'approval' && block.approvalId === focusRequestId)))
    if (turnIndex < 0) return undefined
    if (turns.length - turnIndex > visibleTurnCount) {
      loadEarlierTurns()
      return undefined
    }
    const row = projection.rows.find(row => row.kind === 'turn' && row.turn.key === stableThreadTurnKey(turns[turnIndex], turnIndex))
    if (row?.kind === 'turn') scrollToEstimatedTurn(row.turn.key, 'auto')
    let cancelled = false
    const isCurrent = () => !cancelled && currentRecordScope.current === recordScope && currentViewMode.current === viewMode && viewSeat.current.generation === viewGeneration && visibleRecordsRoot()
    void (async () => {
      // The existing virtualizer renders the requested row after scrolling.
      const started = performance.now()
      while (isCurrent() && performance.now() - started < 2000) {
        const gate = Array.from(document.querySelectorAll<HTMLElement>('[data-analytix-gate-composer] [data-gate-request-id]'))
          .find(node => node.dataset.gateRequestId === focusRequestId && node.closest<HTMLElement>('[data-analytix-gate-composer]')?.dataset.gateThreadId === activeThreadId) ?? Array.from(containerRef.current?.querySelectorAll<HTMLElement>('[data-gate-request-id]') ?? [])
          .find(node => node.dataset.gateRequestId === focusRequestId)
        if (gate && isCurrent()) {
          gate.scrollIntoView({ behavior: 'auto', block: 'center' })
          const control = gate.querySelector<HTMLElement>('button:not(:disabled), textarea:not(:disabled)')
          ;(control ?? gate).focus({ preventScroll: true })
          onRequestFocused?.()
          return
        }
        await nextAnimationFrame()
      }
    })()
    return () => { cancelled = true }
  }, [activeThreadId, focusRequestId, turns, visibleTurnCount, loadEarlierTurns, projection.rows, scrollToEstimatedTurn, onRequestFocused, trajectoryMode, recordScope, viewMode, viewGeneration, visibleRecordsRoot])

  return (
    <Profiler id="message-timeline" onRender={recordReactCommit}>
      <InjectedMemoryLookupProvider workspaceRoot={workspaceRoot}>
      <ProtectedToolInspectorSelection scopeKey={recordScope + ':' + viewMode}>
      {residentTabsTarget ? createPortal(viewToolbar, residentTabsTarget, recordScope) : null}
      <div
        ref={containerRef}
        data-analytix-message-timeline-scroller
        data-conversation-composer-overlay={viewMode !== 'conversation' ? '' : undefined}
        data-conversation-view={viewMode}
        className={`ds-no-drag relative flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden ${
          edgeAlignedScroll ? 'ds-stage-scroll-edge' : ''
        }`}
        style={{ overflowAnchor: 'none' }}
      >
      {viewMode === 'conversation' ? <ThreadUserMessageNavigationRail
        items={projection.userMessageNavigationItems}
        containerRef={containerRef}
        railLabel={t('timelineJumpRailLabel')}
        noContentLabel={t('timelineUserMessageNavigationNoContent')}
        itemAriaLabel={(item) => t('timelineJumpTurn', { index: item.position })}
        onSelectItem={revealNavigationItem}
      /> : null}
      <div
        data-analytix-message-timeline-content
        className={`ds-message-timeline-content ds-chat-column-inset mx-auto flex w-full min-w-0 ${trajectoryMode ? 'max-w-none min-h-0 flex-1' : 'max-w-4xl'} flex-col ${route === 'chat' && edgeAlignedScroll && viewMode === 'conversation' ? 'pt-4' : 'pt-8'} ${contentClassName ?? ''} ${
        goalTimelinePaddingClass(heroRoute, Boolean(activeThreadGoal))
      }`}
      >
        {residentTabsTarget === undefined ? viewToolbar : null}
        {trajectoryMode && activeThreadId ? <TrajectoryPanel threadId={activeThreadId} onInspectTool={inspectTrajectoryCall} renderToolInspector={renderTrajectoryInspector} /> : <>
        {virtualizerEnabled && virtualWindow.beforeHeight > 0 ? (
          <div aria-hidden style={{ height: virtualWindow.beforeHeight }} className="w-full shrink-0" />
        ) : null}
        {renderedItems.map(({ row, index }) => {
          let content: ReactElement | null = null
          switch (row.kind) {
            case 'emptyHero':
              content = (
                <MessageTimelineEmptyHero
                  phase={conversationPhase}
                  route={heroRoute}
                  ready={runtimeConnection === 'ready'}
                  hasWorkspace={!!workspaceRoot}
                  hasComposerWorkspacePicker={route === 'chat' && edgeAlignedScroll && !secondaryProcessContext}
                  runtimeError={runtimeError}
                  activeClawChannel={activeClawChannel}
                  onPickWorkspace={() => void chooseWorkspace()}
                  onRetry={onRetryConnection}
                  onOpenSettings={onOpenSettings}
                  onSelectSuggestion={onSelectSuggestion}
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
              content = recordsMode && activeThreadId && runtimeTurnId(row.turn.turn) ? <ConversationRecords threadId={activeThreadId} turnId={runtimeTurnId(row.turn.turn)!}
                blocks={[...(row.turn.turn.user ? [row.turn.turn.user] : []), ...row.turn.turn.blocks]} request={recordRequest} selected={selectedRecord?.key ?? null}
                onInspect={record => inspectConversationCall(record.turnId, record.callId, record, 'result')} /> : (
                <MemoMessageTurn
                  processPaneRoute={resolvedProcessPaneRoute}
                  runtimeAvailable={runtimeConnection === 'ready'}
                  ownerBusy={busy}
                  captureForkOwner={captureForkOwner || !secondaryProcessContext && activeThreadId === mainThreadId ? captureTimelineForkOwner : undefined}
                  activeThreadId={activeThreadId}
                  isLatest={row.turn.isLatest || row.turn.isLiveTurn}
                  isLiveTurn={row.turn.isLiveTurn}
                  activeStreamTurnId={
                    row.turn.isLiveTurn ? currentTurnId ?? runtimeTurnId(row.turn.turn) : runtimeTurnId(row.turn.turn)
                  }
                  workspaceRoot={workspaceRoot}
                  turn={row.turn.turn}
                  isProcessing={row.turn.isProcessing}
                  liveContent={row.turn.liveContent}
                  durationMs={row.turn.timing.durationMs}
                  runningStartedAt={row.turn.timing.runningStartedAt}
                  turnStatus={row.turn.timing.status}
                  billing={turnBilling(runtimeTurnId(row.turn.turn))}
                  performanceUsageMode={performanceUsageMode}
                  usage={turnUsageById?.[runtimeTurnId(row.turn.turn) ?? '']?.threadId === activeThreadId
                    ? turnUsageById[runtimeTurnId(row.turn.turn) ?? ''].snapshot : undefined}
                  userNavigationItemId={row.turn.userNavigationItemId}
                  devPreviewCard={row.turn.devPreviewCard}
                  viewportRef={containerRef}
                  compactCards={compactCards}
                  sections={row.turn.sections}
                  reviewBlocks={row.turn.reviewBlocks}
                  onInspect={inspectConversationCall}
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
                  processPaneRoute={resolvedProcessPaneRoute}
                  runtimeAvailable={runtimeConnection === 'ready'}
                  ownerBusy={busy}
                  captureForkOwner={captureForkOwner || !secondaryProcessContext && activeThreadId === mainThreadId ? captureTimelineForkOwner : undefined}
                  activeThreadId={activeThreadId}
                  isLatest
                  isLiveTurn
                  activeStreamTurnId={row.turnId ?? null}
                  performanceUsageMode={performanceUsageMode}
                  workspaceRoot={workspaceRoot}
                  turn={{ blocks: [] }}
                  isProcessing={row.isProcessing}
                  liveContent={row.liveContent}
                  devPreviewCard={row.devPreviewCard}
                  viewportRef={containerRef}
                  compactCards={compactCards}
                  durationMs={row.timing.durationMs}
                  runningStartedAt={row.timing.runningStartedAt}
                  turnStatus={row.timing.status}
                  billing={turnBilling(row.turnId ?? null)}
                  sections={row.sections}
                  reviewBlocks={row.reviewBlocks}
                  onInspect={inspectConversationCall}
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
              style={index < projection.rows.length - 1 ? { marginBottom: rowGapPx } : undefined}
            >
              {content}
            </div>
          )
        })}
        {virtualizerEnabled && virtualWindow.afterHeight > 0 ? (
          <div aria-hidden style={{ height: virtualWindow.afterHeight }} className="w-full shrink-0" />
        ) : null}
        </>}
      </div>
      </div>
      {selectedRecord && recordRequest ? <ConversationRecordDetails key={selectedRecord.key + ':' + recordRequest.version}
        record={selectedRecord} requestVersion={recordRequest.version} initialView={recordRequest.view} onViewChange={view => {
          const request = currentRecordRequest.current
          if (request?.version !== recordRequest.version) return
          currentRecordRequest.current = { ...request, view }
          setRecordsState(current => current?.request?.version === recordRequest.version
            ? { ...current, request: { ...current.request, view } } : current)
        }} onRetire={() => {
          if (currentRecordRequest.current?.version === recordRequest.version) retireRecordRequest()
        }} onClose={() => {
          if (currentRecordRequest.current?.version === recordRequest.version) closeRecordDetails()
        }} /> : null}
      </ProtectedToolInspectorSelection>
      </InjectedMemoryLookupProvider>
    </Profiler>
  )
}

function MessageTurn({ activeThreadId, isLatest, isLiveTurn, activeStreamTurnId, turn, isProcessing, liveContent,
  durationMs, turnStatus, usage, billing, performanceUsageMode, userNavigationItemId, devPreviewCard,
  viewportRef, compactCards = false, sections, reviewBlocks,
  onAssistantFinalized, processPaneRoute, runtimeAvailable, ownerBusy, captureForkOwner, onInspect }: {
  activeThreadId: string | null; isLatest: boolean; isLiveTurn: boolean; activeStreamTurnId?: string | null
  workspaceRoot: string; turn: Turn; isProcessing: boolean; liveContent: string
  durationMs?: number; runningStartedAt?: number; turnStatus?: import('../../agent/types').AuthoritativeTurnStatus; usage?: ThreadUsageSnapshot
  billing?: ProviderBillingV1
  performanceUsageMode: PerformanceUsageMode
  userNavigationItemId?: string; devPreviewCard?: ReactElement | null
  viewportRef: RefObject<HTMLDivElement | null>; compactCards?: boolean; sections: TurnSections
  reviewBlocks: Extract<ChatBlock, { kind: 'review' }>[]; onAssistantFinalized?: () => void
  processPaneRoute?: ProcessPaneRoute; runtimeAvailable: boolean; ownerBusy: boolean; captureForkOwner?: () => TurnForkOwner | null
  onInspect?: (turnId: string, callId: string) => void
}): ReactElement {
  const mainThreadId = useChatStore(state => state.activeThreadId)
  const secondary = useContext(GateResponseContext)
  const forkThreadFromTurn = useChatStore(state => state.forkThreadFromTurn)
  const rollbackWorkspaceToCheckpoint = useChatStore(state => state.rollbackWorkspaceToCheckpoint)
  const [forking, setForking] = useState(false)
  const [rollingBack, setRollingBack] = useState(false)
  const turnId = activeStreamTurnId ?? runtimeTurnId(turn)
  const streamTiming = useActiveStreamMetrics(activeThreadId, turnId)
  const localStartedAt = isProcessing && streamTiming.runtimeRunning && streamTiming.threadId === activeThreadId && streamTiming.turnId === turnId
    && streamTiming.startedAt !== null && Number.isFinite(streamTiming.startedAt) ? streamTiming.startedAt : undefined
  const owned = Boolean(activeThreadId && activeThreadId === mainThreadId && !secondary)
  const checkpoint = turn.user?.meta?.workspaceCheckpointId?.trim()
  return <ChatView onInspect={onInspect} runtimeAvailable={runtimeAvailable} processPaneRoute={processPaneRoute} processPaneAutoAcquire={isLatest} threadId={activeThreadId} turnId={turnId} turn={turn} sections={sections}
    processing={isProcessing} running={Boolean(isLiveTurn && isProcessing && turnId && !isPendingActiveStreamTurnId(turnId) &&
      (turnStatus === 'running' || streamTiming.threadId === activeThreadId && streamTiming.turnId === turnId && streamTiming.runtimeRunning))}
    turnStatus={turnStatus} completedDurationMs={durationMs} runningStartedAt={localStartedAt}
    usage={usage} billing={billing} performanceUsageMode={performanceUsageMode} tailReveal={isLatest ? 'always' : 'hover'} userNavigationItemId={userNavigationItemId} onAssistantFinalized={onAssistantFinalized}
    liveAnswer={isLiveTurn ? <LiveAssistantBubble activeThreadId={activeThreadId} turnId={turnId} fallbackText={liveContent} /> : null}
    fork={!isProcessing && activeThreadId && turnId && captureForkOwner ? { busy: forking, unavailable: !runtimeAvailable || ownerBusy, run: boundary => {
      if (forking || boundary.threadId !== activeThreadId || boundary.turnId !== turnId) return
      const sourceOwner = captureForkOwner()
      if (!sourceOwner || !sourceOwner.isCurrent()) return
      setForking(true); void forkThreadFromTurn(turnId, { ...boundary, ...sourceOwner }).finally(() => setForking(false))
    }} : undefined}
    rollback={!isProcessing && owned && checkpoint ? { busy: rollingBack, run: () => {
      if (rollingBack) return
      setRollingBack(true); void rollbackWorkspaceToCheckpoint(checkpoint).finally(() => setRollingBack(false))
    }} : undefined}
    deliveries={<>
      {reviewBlocks.map(review => <ReviewSummaryCard key={review.id} review={review} />)}
      {!isProcessing ? devPreviewCard : null}
      {!isProcessing ? <SubmittedPlanCards threadId={activeThreadId} turnId={turnId} blocks={turn.blocks}
        presented={runtimeAvailable && processPaneRoute?.visible !== false} /> : null}
      {!isProcessing && sections.turnFileChanges.length ? <TurnChangeSummary changes={sections.turnFileChanges}
        viewportRef={viewportRef} compact={compactCards} /> : null}
    </>} />
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
      <MessageBubble block={{ kind: 'assistant', id: 'live-assistant', text }} threadId={activeThreadId} />
    </div>
  )
})

const MemoMessageTurn = memo(MessageTurn)
