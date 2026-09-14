import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent, ReactElement } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AlertCircle, Loader2, MessageCirclePlus, Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { buildCodeRuntimePrompt } from '@shared/app-settings'
import type { ModelProviderModelGroup } from '@shared/analytix-api'
import type { CoreThreadSummarySubagentJson } from '../../agent/analytix-contract'
import type {
  ChatBlock,
  CompactionEventPayload,
  RuntimeChildMetadata,
  RuntimeConnectionStatus,
  RuntimeDisclosureMetadata,
  RuntimeStatusEventPayload,
  ThreadDeltaEvent,
  ThreadEventSink,
  ToolBlock,
  ToolEventPayload
} from '../../agent/types'
import { getProvider } from '../../agent/registry'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { formatRuntimeError } from '../../lib/format-runtime-error'
import { hydrateBlockModelLabels } from '../../store/chat-store-helpers'
import {
  settlePendingRuntimeWorkAfterInterrupt,
  threadSnapshotLooksRunning,
  upsertUserBlock
} from '../../store/chat-store-runtime-helpers'
import type { QueuedUserMessage } from '../../store/chat-store-types'
import { createStreamingDeltaScheduler } from '../../thread/streaming/streaming-delta-scheduler'
import {
  appendActiveStreamDeltas,
  clearActiveStream,
  getActiveStreamSnapshotFor,
  migrateActiveStreamTurn,
  pendingActiveStreamTurnId
} from '../../thread/streaming/active-stream-store'
import { FloatingComposer } from '../chat/FloatingComposer'
import type { ComposerReasoningEffort } from '../chat/FloatingComposerModelPicker'
import type { TimelineRuntimeStateOverride } from '../chat/MessageTimeline'
import { MessageTimeline } from '../chat/MessageTimeline'
import { PanelCollapseButton } from '../workbench/PanelCollapseButton'
import { SubagentGlyph, subagentDisplayLabel, subagentLifecycleLabel } from './SubagentSummaryRows'

type ThreadDetailState =
  | {
      status: 'idle' | 'loading'
      threadId: string | null
      blocks: ChatBlock[]
      error: null
      latestSeq: number
      latestTurnId: string | null
      latestUserMessageId: string | null
      threadStatus?: string
      turnDurationByUserId?: Record<string, number>
    }
  | {
      status: 'ready'
      threadId: string
      blocks: ChatBlock[]
      error: null
      latestSeq: number
      latestTurnId: string | null
      latestUserMessageId: string | null
      threadStatus?: string
      turnDurationByUserId?: Record<string, number>
    }
  | {
      status: 'error'
      threadId: string
      blocks: ChatBlock[]
      error: string
      latestSeq: number
      latestTurnId: string | null
      latestUserMessageId: string | null
      threadStatus?: string
      turnDurationByUserId?: Record<string, number>
    }

function clampNumber(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

function isSideConversationInspectorAgent(agent: CoreThreadSummarySubagentJson): boolean {
  return agent.key.startsWith('side-conversation:')
}

function sideConversationVisibleAfter(agent: CoreThreadSummarySubagentJson | null): string | null {
  const createdAt = agent?.createdAt?.trim()
  return createdAt || null
}

function blockCreatedAtMs(block: ChatBlock): number | null {
  const createdAt = block.createdAt?.trim()
  if (!createdAt) return null
  const value = Date.parse(createdAt)
  return Number.isFinite(value) ? value : null
}

function sideConversationVisibleBlocks(blocks: ChatBlock[], visibleAfter: string | null): ChatBlock[] {
  if (!visibleAfter) return blocks
  const boundary = Date.parse(visibleAfter)
  if (!Number.isFinite(boundary)) return blocks
  return blocks.filter((block) => {
    const createdAt = blockCreatedAtMs(block)
    return createdAt !== null && createdAt >= boundary
  })
}

function visibleSideConversationDetailState(
  state: ThreadDetailState,
  visibleAfter: string | null
): ThreadDetailState {
  const blocks = sideConversationVisibleBlocks(state.blocks, visibleAfter)
  return blocks === state.blocks ? state : { ...state, blocks }
}

function selectedSubagentMetaRows(agent: CoreThreadSummarySubagentJson): Array<{ label: string; value: string }> {
  const provider = agent.providerId
  const model = agent.model
  const endpointFormat = agent.endpointFormat
  const variant = agent.variant
  const modelSource = agent.modelSource
  const cache = typeof agent.cacheHitRate === 'number'
    ? `${Math.round(agent.cacheHitRate * 100)}%`
    : ''
  const tokens = typeof agent.totalTokens === 'number'
    ? String(agent.totalTokens)
    : ''
  return [
    { label: '状态', value: subagentLifecycleLabel(agent) },
    { label: '模型', value: model ?? '' },
    { label: 'Provider', value: provider ?? '' },
    { label: 'Endpoint', value: endpointFormat ?? '' },
    { label: 'Variant', value: variant ?? '' },
    { label: '来源', value: modelSource ?? '' },
    { label: 'Tokens', value: tokens },
    { label: '缓存', value: cache },
    { label: 'Profile', value: agent.profile ?? '' }
  ].filter((row) => row.value.trim().length > 0)
}

type SubagentTimelineRuntimeState = {
  busy: boolean
  currentTurnId: string | null
  currentTurnUserId: string | null
  turnStartedAtByUserId: Record<string, number>
}

function emptySubagentRuntimeState(): SubagentTimelineRuntimeState {
  return {
    busy: false,
    currentTurnId: null,
    currentTurnUserId: null,
    turnStartedAtByUserId: {}
  }
}

function runningStartedAtFromDetail(
  userId: string | null | undefined,
  turnDurationByUserId: Record<string, number> | undefined,
  fallback?: number
): number {
  const normalizedUserId = userId?.trim()
  const recordedDuration = normalizedUserId ? turnDurationByUserId?.[normalizedUserId] : undefined
  if (typeof recordedDuration === 'number' && Number.isFinite(recordedDuration) && recordedDuration >= 0) {
    return Date.now() - recordedDuration
  }
  return fallback ?? Date.now()
}

function eventSeq(value: unknown): number | undefined {
  const seq = (value as { seq?: unknown } | null)?.seq
  return typeof seq === 'number' && Number.isFinite(seq) ? seq : undefined
}

function eventCreatedAtMs(value: string | undefined): number {
  if (!value) return Date.now()
  const ms = Date.parse(value)
  return Number.isFinite(ms) ? ms : Date.now()
}

function eventTurnId(meta: { turnId?: string; meta?: Record<string, unknown> | RuntimeDisclosureMetadata } | undefined, fallback?: string | null): string {
  const direct = meta?.turnId?.trim()
  if (direct) return direct
  const metaTurnId = typeof meta?.meta?.turnId === 'string' ? meta.meta.turnId.trim() : ''
  if (metaTurnId) return metaTurnId
  return fallback?.trim() ?? ''
}

function toolEventCallId(ev: ToolEventPayload): string {
  return typeof ev.meta?.callId === 'string' ? ev.meta.callId.trim() : ''
}

function toolBlockCallId(block: ToolBlock): string {
  return typeof block.meta?.callId === 'string' ? block.meta.callId.trim() : ''
}

function toolBlockTurnId(block: ToolBlock): string {
  return typeof block.meta?.turnId === 'string' ? block.meta.turnId.trim() : ''
}

function childMeta(value: unknown): RuntimeChildMetadata | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const child = value as RuntimeChildMetadata
  if (!child.childRunId && !child.childId) return undefined
  return child
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

function toolBlockMatchesChildEvent(block: ToolBlock, ev: ToolEventPayload, eventTurnIdValue: string): boolean {
  const eventChild = childMeta(ev.meta?.child)
  const blockChild = childMeta(block.meta?.child)
  const eventChildId = childIdentity(eventChild)
  if (!eventChild || !blockChild || !eventChildId || childIdentity(blockChild) !== eventChildId) return false
  const blockTurnId = toolBlockTurnId(block)
  const eventChildTurn = childTurnBoundary(eventChild, eventTurnIdValue)
  const blockChildTurn = childTurnBoundary(blockChild, blockTurnId)
  if (eventChildTurn && blockChildTurn && eventChildTurn !== blockChildTurn) return false
  const eventChildThread = childThreadBoundary(eventChild)
  const blockChildThread = childThreadBoundary(blockChild)
  return !(eventChildThread && blockChildThread && eventChildThread !== blockChildThread)
}

function toolBlockMatchesEvent(block: ToolBlock, ev: ToolEventPayload, currentTurnId: string | null): boolean {
  const eventTurnIdValue = eventTurnId(ev, currentTurnId)
  if (toolBlockMatchesChildEvent(block, ev, eventTurnIdValue)) return true
  const blockTurnId = toolBlockTurnId(block)
  if (block.id === ev.itemId) {
    if (eventTurnIdValue && blockTurnId) return eventTurnIdValue === blockTurnId
    if (eventTurnIdValue && !blockTurnId) return false
    if (!eventTurnIdValue && blockTurnId && currentTurnId?.trim()) return blockTurnId === currentTurnId.trim()
    return !currentTurnId?.trim()
  }
  const callId = toolEventCallId(ev)
  if (!callId || toolBlockCallId(block) !== callId) return false
  if (eventTurnIdValue && blockTurnId) return eventTurnIdValue === blockTurnId
  if (eventTurnIdValue && !blockTurnId) return currentTurnId?.trim() === eventTurnIdValue
  if (!eventTurnIdValue && blockTurnId) return currentTurnId?.trim() === blockTurnId
  return false
}

function mergeToolMeta(
  current: ToolBlock['meta'] | undefined,
  incoming: ToolEventPayload['meta'] | undefined,
  turnId: string
): ToolBlock['meta'] {
  const currentChild = childMeta(current?.child)
  const incomingChild = childMeta(incoming?.child)
  return {
    ...(current ?? {}),
    ...(incoming ?? {}),
    ...(currentChild || incomingChild ? { child: { ...(currentChild ?? {}), ...(incomingChild ?? {}) } } : {}),
    ...(turnId ? { turnId } : {})
  }
}

function upsertToolEventBlock(blocks: ChatBlock[], ev: ToolEventPayload, currentTurnId: string | null): ChatBlock[] {
  const eventTurnIdValue = eventTurnId(ev, currentTurnId)
  const existingIndex = blocks.findIndex((block) =>
    block.kind === 'tool' && toolBlockMatchesEvent(block, ev, currentTurnId)
  )
  if (existingIndex >= 0) {
    const current = blocks[existingIndex]
    if (current.kind !== 'tool') return blocks
    const next: ToolBlock = {
      ...current,
      summary: ev.summary || current.summary,
      status: ev.status,
      toolKind: ev.toolKind ?? current.toolKind,
      detail: ev.detail ?? current.detail,
      filePath: ev.filePath ?? current.filePath,
      meta: mergeToolMeta(current.meta, ev.meta, eventTurnIdValue)
    }
    const updated = [...blocks]
    updated[existingIndex] = next
    return updated
  }
  const id = blocks.some((block) => block.id === ev.itemId)
    ? `${ev.itemId}:${eventTurnIdValue || Date.now()}`
    : ev.itemId
  return [
    ...blocks,
    {
      kind: 'tool',
      id,
      createdAt: new Date().toISOString(),
      summary: ev.summary,
      status: ev.status,
      toolKind: ev.toolKind,
      detail: ev.detail,
      filePath: ev.filePath,
      meta: mergeToolMeta(undefined, ev.meta, eventTurnIdValue)
    }
  ]
}

function blockMetaTurnId(block: ChatBlock): string {
  const meta = (block as { meta?: { turnId?: unknown } }).meta
  return typeof meta?.turnId === 'string' ? meta.turnId.trim() : ''
}

function hasTurnTextBlock(blocks: ChatBlock[], kind: 'assistant', turnId: string, text: string): boolean {
  const normalizedText = text.trim()
  if (!normalizedText) return true
  return blocks.some((block) =>
    block.kind === kind && blockMetaTurnId(block) === turnId && block.text.trim() === normalizedText
  )
}

function flushInspectorActiveStreamBlocks(blocks: ChatBlock[], threadId: string, turnId: string | null): ChatBlock[] {
  const normalizedTurnId = turnId?.trim() || null
  if (!normalizedTurnId) return blocks
  const snapshot = getActiveStreamSnapshotFor(threadId, normalizedTurnId)
  if (!snapshot.threadId) return blocks
  const createdAt = new Date().toISOString()
  let next = blocks
  if (snapshot.liveAssistant.trim() && !hasTurnTextBlock(next, 'assistant', normalizedTurnId, snapshot.liveAssistant)) {
    next = [
      ...next,
      {
        kind: 'assistant',
        id: `subagent_live_assistant_${snapshot.lastSeq || Date.now()}`,
        createdAt,
        text: snapshot.liveAssistant,
        meta: { turnId: normalizedTurnId }
      }
    ]
  }
  clearActiveStream(threadId, normalizedTurnId)
  return next
}

export function SubagentInspectorPanel({
  subagents,
  selectedKey,
  runtimeConnection,
  composerModel,
  composerProviderId,
  composerPickList,
  composerModelGroups = [],
  composerReasoningEffort,
  setComposerModel,
  setComposerReasoningEffort,
  onConfigureProviders,
  onSelectSubagent,
  onCloseSubagentTab,
  onCreateSideChat,
  createSideChatDisabled = false,
  tabbedWorkspace = false,
  onCollapse,
  onRetryConnection,
  onOpenSettings,
  className = ''
}: {
  subagents: CoreThreadSummarySubagentJson[]
  selectedKey: string | null
  runtimeConnection: RuntimeConnectionStatus
  composerModel: string
  composerProviderId?: string
  composerPickList: string[]
  composerModelGroups?: ModelProviderModelGroup[]
  composerReasoningEffort: ComposerReasoningEffort
  setComposerModel: (modelId: string, providerId?: string) => void
  setComposerReasoningEffort: (effort: ComposerReasoningEffort) => void
  onConfigureProviders?: () => void
  onSelectSubagent: (key: string) => void
  onCloseSubagentTab?: (key: string) => void | Promise<void>
  onCreateSideChat?: () => void | Promise<void>
  createSideChatDisabled?: boolean
  tabbedWorkspace?: boolean
  onCollapse: () => void
  onRetryConnection: () => void
  onOpenSettings: () => void
  className?: string
}): ReactElement {
  const { t } = useTranslation('common')
  const provider = useMemo(() => getProvider(), [])
  const selectedAgent = useMemo(
    () => subagents.find((agent) => agent.key === selectedKey) ?? subagents[0] ?? null,
    [selectedKey, subagents]
  )
  const selectedThreadId = selectedAgent?.childThreadId?.trim() || null
  const sideChatTabLabel = t('sidePanelTabLabel')
  const selectedLabel = selectedAgent
    ? isSideConversationInspectorAgent(selectedAgent)
      ? sideChatTabLabel
      : subagentDisplayLabel(selectedAgent)
    : onCreateSideChat
      ? sideChatTabLabel
      : '子智能体'
  const selectedIsSideChat = selectedAgent ? isSideConversationInspectorAgent(selectedAgent) : false
  const selectedThreadIdRef = useRef<string | null>(selectedThreadId)
  const detailSeqByThreadIdRef = useRef<Record<string, number>>({})
  const detailLoadInFlightRef = useRef<{
    threadId: string
    promise: Promise<{ threadId: string; running: boolean; latestSeq: number } | null>
  } | null>(null)
  const [detailState, setDetailState] = useState<ThreadDetailState>({
    status: 'idle',
    threadId: null,
    blocks: [],
    error: null,
    latestSeq: 0,
    latestTurnId: null,
    latestUserMessageId: null
  })
  const [inputBySubagentKey, setInputBySubagentKey] = useState<Record<string, string>>({})
  const [composerMode, setComposerMode] = useState<'agent' | 'plan'>('agent')
  const [runningTurnByThreadId, setRunningTurnByThreadId] = useState<Record<string, string>>({})
  const [timelineRuntimeByThreadId, setTimelineRuntimeByThreadId] = useState<Record<string, SubagentTimelineRuntimeState>>({})
  const timelineRuntimeByThreadIdRef = useRef(timelineRuntimeByThreadId)
  const [subscriptionRestartByThreadId, setSubscriptionRestartByThreadId] = useState<Record<string, number>>({})
  const [composerError, setComposerError] = useState<string | null>(null)
  const tabScrollRef = useRef<HTMLDivElement | null>(null)
  const tabScrollbarRef = useRef<HTMLDivElement | null>(null)
  const tabScrollbarDragRef = useRef<{
    pointerId: number
    startX: number
    startScrollLeft: number
    trackTravel: number
    maxScrollLeft: number
  } | null>(null)
  const [tabScrollbarDragging, setTabScrollbarDragging] = useState(false)
  const [tabScrollState, setTabScrollState] = useState({
    left: false,
    right: false,
    overflow: false,
    thumbLeft: 0,
    thumbWidth: 100
  })
  const selectedSideChatVisibleAfter = selectedIsSideChat ? sideConversationVisibleAfter(selectedAgent) : null
  const visibleDetailState = useMemo(
    () => selectedIsSideChat
      ? visibleSideConversationDetailState(detailState, selectedSideChatVisibleAfter)
      : detailState,
    [detailState, selectedIsSideChat, selectedSideChatVisibleAfter]
  )
  const selectedInput = selectedAgent ? inputBySubagentKey[selectedAgent.key] ?? '' : ''
  const selectedRunningTurnId = selectedThreadId ? runningTurnByThreadId[selectedThreadId] ?? null : null
  const selectedSubscriptionRestartToken = selectedThreadId ? subscriptionRestartByThreadId[selectedThreadId] ?? 0 : 0
  const selectedTimelineRuntime = selectedThreadId
    ? timelineRuntimeByThreadId[selectedThreadId] ?? emptySubagentRuntimeState()
    : emptySubagentRuntimeState()
  const detailLooksRunning =
    visibleDetailState.threadId === selectedThreadId
      ? threadSnapshotLooksRunning(visibleDetailState.blocks, visibleDetailState.threadStatus)
      : false
  const selectedBusy = Boolean(selectedRunningTurnId) || selectedTimelineRuntime.busy || detailLooksRunning
  const selectedTimelineStateOverride = useMemo<TimelineRuntimeStateOverride>(() => {
    const detailForSelected = visibleDetailState.threadId === selectedThreadId ? visibleDetailState : null
    const runningTurnId =
      selectedTimelineRuntime.currentTurnId
      ?? selectedRunningTurnId
      ?? (detailForSelected && detailLooksRunning ? detailForSelected.latestTurnId : null)
    return {
      busy: selectedBusy,
      currentTurnId: runningTurnId,
      currentTurnUserId:
        selectedTimelineRuntime.currentTurnUserId
        ?? (detailForSelected && detailLooksRunning ? detailForSelected.latestUserMessageId : null),
      turnStartedAtByUserId: selectedTimelineRuntime.turnStartedAtByUserId,
      turnDurationByUserId: detailForSelected?.turnDurationByUserId ?? {}
    }
  }, [
    detailLooksRunning,
    selectedBusy,
    selectedRunningTurnId,
    selectedThreadId,
    selectedTimelineRuntime,
    visibleDetailState
  ])

  useEffect(() => {
    selectedThreadIdRef.current = selectedThreadId
  }, [selectedThreadId])

  useEffect(() => {
    timelineRuntimeByThreadIdRef.current = timelineRuntimeByThreadId
  }, [timelineRuntimeByThreadId])

  useEffect(() => {
    const node = tabScrollRef.current
    if (!node) {
      setTabScrollState({
        left: false,
        right: false,
        overflow: false,
        thumbLeft: 0,
        thumbWidth: 100
      })
      return undefined
    }
    const update = (): void => {
      const maxScrollLeft = Math.max(0, node.scrollWidth - node.clientWidth)
      const overflow = maxScrollLeft > 1
      const thumbWidth = overflow
        ? Math.max(12, Math.min(100, (node.clientWidth / node.scrollWidth) * 100))
        : 100
      const thumbLeft = overflow
        ? Math.min(100 - thumbWidth, Math.max(0, (node.scrollLeft / maxScrollLeft) * (100 - thumbWidth)))
        : 0
      const next = {
        left: node.scrollLeft > 1,
        right: node.scrollLeft < maxScrollLeft - 1,
        overflow,
        thumbLeft,
        thumbWidth
      }
      setTabScrollState((current) =>
        current.left === next.left &&
        current.right === next.right &&
        current.overflow === next.overflow &&
        Math.abs(current.thumbLeft - next.thumbLeft) < 0.1 &&
        Math.abs(current.thumbWidth - next.thumbWidth) < 0.1
          ? current
          : next
      )
    }
    update()
    node.addEventListener('scroll', update, { passive: true })
    window.addEventListener('resize', update)
    const resizeObserver = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(update) : null
    resizeObserver?.observe(node)
    return () => {
      node.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
      resizeObserver?.disconnect()
    }
  }, [onCreateSideChat, subagents])

  const handleTabScrollbarPointerDown = useCallback((event: ReactPointerEvent<HTMLDivElement>): void => {
    const node = tabScrollRef.current
    const track = tabScrollbarRef.current
    if (!node || !track) return
    const maxScrollLeft = Math.max(0, node.scrollWidth - node.clientWidth)
    if (maxScrollLeft <= 1) return
    const rect = track.getBoundingClientRect()
    if (rect.width <= 0) return
    event.preventDefault()
    event.stopPropagation()
    const thumbWidthPx = Math.max(14, (rect.width * tabScrollState.thumbWidth) / 100)
    const trackTravel = Math.max(1, rect.width - thumbWidthPx)
    const thumbLeftPx = (node.scrollLeft / maxScrollLeft) * trackTravel
    const pointerX = event.clientX - rect.left
    const hitThumb = pointerX >= thumbLeftPx && pointerX <= thumbLeftPx + thumbWidthPx
    let startScrollLeft = node.scrollLeft
    if (!hitThumb) {
      const ratio = clampNumber((pointerX - thumbWidthPx / 2) / trackTravel, 0, 1)
      startScrollLeft = ratio * maxScrollLeft
      node.scrollLeft = startScrollLeft
    }
    tabScrollbarDragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startScrollLeft,
      trackTravel,
      maxScrollLeft
    }
    setTabScrollbarDragging(true)
    event.currentTarget.setPointerCapture(event.pointerId)
  }, [tabScrollState.thumbWidth])

  const handleTabScrollbarPointerMove = useCallback((event: ReactPointerEvent<HTMLDivElement>): void => {
    const drag = tabScrollbarDragRef.current
    const node = tabScrollRef.current
    if (!drag || !node || drag.pointerId !== event.pointerId) return
    event.preventDefault()
    const scrollDelta = ((event.clientX - drag.startX) / drag.trackTravel) * drag.maxScrollLeft
    node.scrollLeft = clampNumber(drag.startScrollLeft + scrollDelta, 0, drag.maxScrollLeft)
  }, [])

  const handleTabScrollbarPointerEnd = useCallback((event: ReactPointerEvent<HTMLDivElement>): void => {
    const drag = tabScrollbarDragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    tabScrollbarDragRef.current = null
    setTabScrollbarDragging(false)
  }, [])

  const handleSubagentTabKeyDown = useCallback((
    event: ReactKeyboardEvent<HTMLDivElement>,
    key: string
  ): void => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onSelectSubagent(key)
  }, [onSelectSubagent])

  useEffect(() => {
    if (subagents.length === 0) return
    if (selectedAgent) return
    onSelectSubagent(subagents[0].key)
  }, [onSelectSubagent, selectedAgent, subagents])

  const loadSelectedDetail = useCallback(async (mode: 'initial' | 'refresh'): Promise<{ threadId: string; running: boolean; latestSeq: number } | null> => {
    if (!selectedThreadId) {
      setDetailState({
        status: 'idle',
        threadId: null,
        blocks: [],
        error: null,
        latestSeq: 0,
        latestTurnId: null,
        latestUserMessageId: null
      })
      return null
    }

    const inFlight = detailLoadInFlightRef.current
    if (inFlight?.threadId === selectedThreadId) {
      return inFlight.promise
    }

    const loadingThreadId = selectedThreadId
    const promise = (async (): Promise<{ threadId: string; running: boolean; latestSeq: number } | null> => {
      setDetailState((current) => {
        if (mode === 'refresh' && current.threadId === loadingThreadId) return current
        if (current.threadId === loadingThreadId && current.status === 'ready') return current
        return {
          status: 'loading',
          threadId: loadingThreadId,
          blocks: current.threadId === loadingThreadId ? current.blocks : [],
          error: null,
          latestSeq: current.threadId === loadingThreadId ? current.latestSeq : 0,
          latestTurnId: current.threadId === loadingThreadId ? current.latestTurnId : null,
          latestUserMessageId: current.threadId === loadingThreadId ? current.latestUserMessageId : null,
          threadStatus: current.threadId === loadingThreadId ? current.threadStatus : undefined,
          turnDurationByUserId: current.threadId === loadingThreadId ? current.turnDurationByUserId : undefined
        }
      })
      try {
        const detail = await provider.getThreadDetail(loadingThreadId)
        if (selectedThreadIdRef.current !== loadingThreadId) return null
        const blocks = hydrateBlockModelLabels(loadingThreadId, detail.blocks)
        detailSeqByThreadIdRef.current = {
          ...detailSeqByThreadIdRef.current,
          [loadingThreadId]: Math.max(detailSeqByThreadIdRef.current[loadingThreadId] ?? 0, detail.latestSeq ?? 0)
        }
        setDetailState({
          status: 'ready',
          threadId: loadingThreadId,
          blocks,
          error: null,
          latestSeq: detail.latestSeq,
          latestTurnId: detail.latestTurnId ?? null,
          latestUserMessageId: detail.latestUserMessageId ?? null,
          threadStatus: detail.threadStatus,
          turnDurationByUserId: detail.turnDurationByUserId
        })
        const busy = threadSnapshotLooksRunning(blocks, detail.threadStatus)
        setTimelineRuntimeByThreadId((current) => ({
          ...current,
          [loadingThreadId]: (() => {
            const previous = current[loadingThreadId] ?? emptySubagentRuntimeState()
            const currentTurnUserId = busy ? detail.latestUserMessageId ?? previous.currentTurnUserId ?? null : null
            return {
              ...previous,
              busy,
              currentTurnId: busy ? detail.latestTurnId ?? previous.currentTurnId ?? null : null,
              currentTurnUserId,
              turnStartedAtByUserId: busy && currentTurnUserId
                ? {
                    ...previous.turnStartedAtByUserId,
                    [currentTurnUserId]: previous.turnStartedAtByUserId[currentTurnUserId] ??
                      runningStartedAtFromDetail(currentTurnUserId, detail.turnDurationByUserId)
                  }
                : previous.turnStartedAtByUserId
            }
          })()
        }))
        if (!busy) {
          setRunningTurnByThreadId((current) => {
            if (!current[loadingThreadId]) return current
            const next = { ...current }
            delete next[loadingThreadId]
            return next
          })
        }
        return { threadId: loadingThreadId, running: busy, latestSeq: detail.latestSeq ?? 0 }
      } catch (error) {
        if (selectedThreadIdRef.current !== loadingThreadId) return null
        setDetailState((current) => ({
          status: 'error',
          threadId: loadingThreadId,
          blocks: current.threadId === loadingThreadId ? current.blocks : [],
          error: error instanceof Error ? error.message : String(error),
          latestSeq: current.threadId === loadingThreadId ? current.latestSeq : 0,
          latestTurnId: current.threadId === loadingThreadId ? current.latestTurnId : null,
          latestUserMessageId: current.threadId === loadingThreadId ? current.latestUserMessageId : null,
          threadStatus: current.threadId === loadingThreadId ? current.threadStatus : undefined,
          turnDurationByUserId: current.threadId === loadingThreadId ? current.turnDurationByUserId : undefined
        }))
        return null
      }
    })()
    detailLoadInFlightRef.current = { threadId: loadingThreadId, promise }
    return promise.finally(() => {
      if (detailLoadInFlightRef.current?.threadId === loadingThreadId && detailLoadInFlightRef.current.promise === promise) {
        detailLoadInFlightRef.current = null
      }
    })
  }, [provider, selectedThreadId])

  useEffect(() => {
    if (!selectedThreadId) {
      void loadSelectedDetail('initial')
      return undefined
    }
    let cancelled = false
    void loadSelectedDetail('initial').then(() => {
      if (cancelled) return
    })
    const shouldPoll = selectedAgent?.status === 'active' || Boolean(selectedRunningTurnId) || detailLooksRunning
    if (!shouldPoll) {
      return () => {
        cancelled = true
      }
    }

    const timer = window.setInterval(() => void loadSelectedDetail('refresh'), 1_000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [detailLooksRunning, loadSelectedDetail, selectedAgent?.status, selectedRunningTurnId, selectedThreadId])

  useEffect(() => {
    if (!selectedThreadId || runtimeConnection !== 'ready') return undefined
    if (detailState.threadId !== selectedThreadId || detailState.status === 'loading') return undefined

    const threadId = selectedThreadId
    const abort = new AbortController()
    const sinceSeq = detailState.latestSeq
    const detailCoveredSeq = (): number => detailSeqByThreadIdRef.current[threadId] ?? 0
    let appliedDeltaSeqFloor = Math.max(sinceSeq, detailCoveredSeq())
    let streamTurnId = detailState.latestTurnId
    let subscriptionClosed = false
    const eventStale = (): boolean =>
      subscriptionClosed || abort.signal.aborted || selectedThreadIdRef.current !== threadId

    const patchRuntime = (patch: Partial<SubagentTimelineRuntimeState>): void => {
      setTimelineRuntimeByThreadId((current) => ({
        ...current,
        [threadId]: {
          ...(current[threadId] ?? emptySubagentRuntimeState()),
          ...patch
        }
      }))
    }

    const rememberSeq = (seq: number | undefined): void => {
      if (typeof seq !== 'number' || !Number.isFinite(seq)) return
      setDetailState((current) => {
        if (current.threadId !== threadId) return current
        return {
          ...current,
          latestSeq: Math.max(current.latestSeq, seq)
        }
      })
    }

    const adoptTurnId = (turnId?: string | null): string | null => {
      const normalized = turnId?.trim() || null
      if (!normalized) return streamTurnId
      if (streamTurnId && streamTurnId !== normalized && streamTurnId.startsWith('pending:')) {
        migrateActiveStreamTurn(threadId, streamTurnId, normalized)
      }
      streamTurnId = normalized
      patchRuntime({ busy: true, currentTurnId: normalized })
      return streamTurnId
    }

    const flushPendingLiveBlocks = (turnId?: string | null): void => {
      const targetTurnId = turnId?.trim() || streamTurnId
      if (!targetTurnId) return
      setDetailState((current) => {
        if (current.threadId !== threadId) return current
        const blocks = flushInspectorActiveStreamBlocks(current.blocks, threadId, targetTurnId)
        return blocks === current.blocks ? current : { ...current, blocks }
      })
    }

    const flushDeltas = ({ assistant, lastSeq, turnId }: { assistant: string; lastSeq: number; turnId: string | null }): void => {
      if (eventStale()) return
      const activeTurnId = adoptTurnId(turnId) ?? pendingActiveStreamTurnId(threadId, 'inspector')
      streamTurnId = activeTurnId
      rememberSeq(lastSeq)
      appendActiveStreamDeltas({
        threadId,
        turnId: activeTurnId,
        assistant,
        lastSeq
      })
      patchRuntime({
        busy: true,
        currentTurnId: activeTurnId
      })
    }

    const scheduler = createStreamingDeltaScheduler({
      flushFirstSynchronously: true,
      onFlush: flushDeltas
    })
    const flushScheduler = (): void => {
      if (!abort.signal.aborted) scheduler.flushNow()
    }

    const sink: ThreadEventSink = {
      onSeq: (seq) => {
        if (eventStale()) return
        rememberSeq(seq)
      },
      onTurnStarted: (ev) => {
        if (eventStale()) return
        flushScheduler()
        const activeTurnId = adoptTurnId(ev.turnId)
        rememberSeq(ev.seq)
        patchRuntime({
          busy: true,
          currentTurnId: activeTurnId,
          turnStartedAtByUserId: {
            ...(timelineRuntimeByThreadIdRef.current[threadId]?.turnStartedAtByUserId ?? {}),
            ...(detailState.latestUserMessageId
              ? { [detailState.latestUserMessageId]: eventCreatedAtMs(ev.createdAt) }
              : {})
          }
        })
      },
      onUserMessage: (ev) => {
        if (eventStale()) return
        flushScheduler()
        const activeTurnId = adoptTurnId(ev.turnId)
        rememberSeq(eventSeq(ev))
        const userId = ev.itemId
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const blocks = upsertUserBlock(current.blocks, {
            ...ev,
            meta: {
              ...(ev.meta ?? {}),
              ...(activeTurnId ? { turnId: activeTurnId } : {})
            }
          })
          return {
            ...current,
            blocks,
            latestTurnId: activeTurnId ?? current.latestTurnId,
            latestUserMessageId: userId,
            threadStatus: 'running'
          }
        })
        patchRuntime({
          busy: true,
          currentTurnId: activeTurnId,
          currentTurnUserId: userId,
          turnStartedAtByUserId: {
            ...(timelineRuntimeByThreadIdRef.current[threadId]?.turnStartedAtByUserId ?? {}),
            [userId]: eventCreatedAtMs(ev.createdAt)
          }
        })
      },
      onDeltas: (rawDeltas: ThreadDeltaEvent[]) => {
        if (eventStale()) return
        const deltas: ThreadDeltaEvent[] = []
        const seqFloor = Math.max(appliedDeltaSeqFloor, detailCoveredSeq())
        let batchMaxSeq = seqFloor
        for (const delta of rawDeltas) {
          if (typeof delta.seq === 'number') {
            if (delta.seq <= seqFloor) continue
            batchMaxSeq = Math.max(batchMaxSeq, delta.seq)
            rememberSeq(delta.seq)
          }
          if (delta.turnId) adoptTurnId(delta.turnId)
          deltas.push(delta)
        }
        appliedDeltaSeqFloor = batchMaxSeq
        if (deltas.length > 0) scheduler.enqueue(deltas)
      },
      onTool: (ev: ToolEventPayload) => {
        if (eventStale()) return
        flushScheduler()
        const activeTurnId = adoptTurnId(eventTurnId(ev, streamTurnId))
        rememberSeq(eventSeq(ev))
        flushPendingLiveBlocks(activeTurnId)
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const blocks = upsertToolEventBlock(current.blocks, ev, activeTurnId)
          return {
            ...current,
            blocks,
            latestTurnId: activeTurnId ?? current.latestTurnId,
            threadStatus: 'running'
          }
        })
        patchRuntime({ busy: true, currentTurnId: activeTurnId })
      },
      onCompaction: (ev: CompactionEventPayload) => {
        if (eventStale()) return
        flushScheduler()
        const activeTurnId = adoptTurnId(eventTurnId(ev, streamTurnId))
        rememberSeq(ev.seq)
        flushPendingLiveBlocks(activeTurnId)
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const block: ChatBlock = {
            kind: 'compaction',
            id: ev.itemId,
            createdAt: ev.createdAt ?? new Date().toISOString(),
            summary: ev.summary,
            status: ev.status,
            detail: ev.detail,
            auto: ev.auto,
            meta: {
              ...(ev.meta ?? {}),
              ...(activeTurnId ? { turnId: activeTurnId } : {})
            }
          }
          const index = current.blocks.findIndex((candidate) => candidate.kind === 'compaction' && candidate.id === ev.itemId)
          const blocks = index >= 0
            ? current.blocks.map((candidate, candidateIndex) => candidateIndex === index ? block : candidate)
            : [...current.blocks, block]
          return { ...current, blocks, latestTurnId: activeTurnId ?? current.latestTurnId }
        })
      },
      onReview: () => undefined,
      onApproval: (req) => {
        if (eventStale()) return
        flushScheduler()
        const activeTurnId = adoptTurnId(eventTurnId(req, streamTurnId))
        flushPendingLiveBlocks(activeTurnId)
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const block: ChatBlock = {
            kind: 'approval',
            id: `approval_${req.approvalId}`,
            createdAt: new Date().toISOString(),
            approvalId: req.approvalId,
            summary: req.summary,
            toolName: req.toolName,
            status: 'pending',
            meta: {
              ...(req.meta ?? {}),
              ...(activeTurnId ? { turnId: activeTurnId } : {})
            }
          }
          const index = current.blocks.findIndex((candidate) =>
            candidate.kind === 'approval' && (candidate.approvalId === req.approvalId || candidate.id === block.id)
          )
          const blocks = index >= 0
            ? current.blocks.map((candidate, candidateIndex) => candidateIndex === index ? block : candidate)
            : [...current.blocks, block]
          return { ...current, blocks, latestTurnId: activeTurnId ?? current.latestTurnId, threadStatus: 'running' }
        })
      },
      onApprovalStatus: (ev) => {
        if (eventStale()) return
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          return {
            ...current,
            blocks: current.blocks.map((block) =>
              block.kind === 'approval' && (block.approvalId === ev.approvalId || block.id === ev.itemId)
                ? { ...block, status: ev.status, errorMessage: ev.errorMessage ?? block.errorMessage }
                : block
            )
          }
        })
      },
      onUserInput: (req) => {
        if (eventStale()) return
        flushScheduler()
        const activeTurnId = adoptTurnId(eventTurnId(req, streamTurnId))
        flushPendingLiveBlocks(activeTurnId)
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const block: ChatBlock = {
            kind: 'user_input',
            id: req.itemId,
            createdAt: new Date().toISOString(),
            requestId: req.requestId,
            questions: req.questions,
            status: 'pending',
            meta: {
              ...(req.meta ?? {}),
              ...(activeTurnId ? { turnId: activeTurnId } : {})
            }
          }
          const index = current.blocks.findIndex((candidate) =>
            candidate.kind === 'user_input' && (candidate.id === req.itemId || candidate.requestId === req.requestId)
          )
          const blocks = index >= 0
            ? current.blocks.map((candidate, candidateIndex) => candidateIndex === index ? block : candidate)
            : [...current.blocks, block]
          return { ...current, blocks, latestTurnId: activeTurnId ?? current.latestTurnId, threadStatus: 'running' }
        })
      },
      onUserInputStatus: (ev) => {
        if (eventStale()) return
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          return {
            ...current,
            blocks: current.blocks.map((block) =>
              block.kind === 'user_input' && (block.id === ev.itemId || block.requestId === ev.itemId || block.requestId === ev.requestId)
                ? {
                    ...block,
                    status: ev.status,
                    answers: ev.answers ?? block.answers,
                    errorMessage: ev.errorMessage ?? block.errorMessage
                  }
                : block
            )
          }
        })
      },
      onRuntimeStatus: (_ev: RuntimeStatusEventPayload) => {
        // The persisted mapper already filters low-value runtime pipeline stages.
        // Keep the child inspector on that same path instead of reintroducing
        // generic Setup/Input Received style rows through live status events.
      },
      onRuntimeError: (ev) => {
        if (eventStale()) return
        flushScheduler()
        const createdAt = ev.createdAt ?? new Date().toISOString()
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const blocks = flushInspectorActiveStreamBlocks(current.blocks, threadId, streamTurnId)
          return {
            ...current,
            blocks: [
              ...blocks,
              {
                kind: 'system',
                id: ev.itemId,
                createdAt,
                text: ev.message,
                code: ev.code,
                severity: ev.severity ?? 'error'
              }
            ],
            threadStatus: 'error'
          }
        })
      },
      onThreadLifecycle: (ev) => {
        if (eventStale()) return
        rememberSeq(ev.seq)
      },
      onThreadRewound: (ev) => {
        if (eventStale()) return
        rememberSeq(ev.seq)
        void loadSelectedDetail('refresh')
      },
      onGoal: () => undefined,
      onTodos: () => undefined,
      onSnapshotRequired: async (ev) => {
        if (eventStale()) return
        flushScheduler()
        const snapshotSeq = Math.max(ev.seq ?? 0, ev.highestSeq ?? 0)
        appliedDeltaSeqFloor = Math.max(appliedDeltaSeqFloor, snapshotSeq)
        detailSeqByThreadIdRef.current = {
          ...detailSeqByThreadIdRef.current,
          [threadId]: Math.max(detailCoveredSeq(), snapshotSeq)
        }
        rememberSeq(snapshotSeq)
        if (streamTurnId) {
          flushPendingLiveBlocks(streamTurnId)
          clearActiveStream(threadId, streamTurnId)
        } else {
          clearActiveStream(threadId)
        }
        streamTurnId = null
        await loadSelectedDetail('refresh')
      },
      onTurnComplete: (ev) => {
        if (eventStale()) return
        flushScheduler()
        rememberSeq(ev?.seq)
        const completedTurnId = eventTurnId(ev, streamTurnId) || streamTurnId
        flushPendingLiveBlocks(completedTurnId)
        if (completedTurnId) clearActiveStream(threadId, completedTurnId)
        setRunningTurnByThreadId((current) => {
          if (!current[threadId]) return current
          const next = { ...current }
          delete next[threadId]
          return next
        })
        patchRuntime({ busy: false, currentTurnId: null, currentTurnUserId: null })
        streamTurnId = null
        void loadSelectedDetail('refresh')
      },
      onError: (error) => {
        if (eventStale()) return
        flushScheduler()
        const failedTurnId = streamTurnId
        setDetailState((current) => {
          if (current.threadId !== threadId) return current
          const blocks = settlePendingRuntimeWorkAfterInterrupt(
            flushInspectorActiveStreamBlocks(current.blocks, threadId, failedTurnId)
          )
          return {
            ...current,
            status: 'error',
            threadId,
            blocks,
            error: formatRuntimeError(error),
            threadStatus: 'error'
          }
        })
        if (failedTurnId) clearActiveStream(threadId, failedTurnId)
        patchRuntime({ busy: false, currentTurnId: null, currentTurnUserId: null })
      }
    }

    void provider.subscribeThreadEvents(threadId, sinceSeq, sink, abort.signal).then(() => {
      if (abort.signal.aborted) return
      void loadSelectedDetail('refresh').then((detail) => {
        if (abort.signal.aborted || selectedThreadIdRef.current !== threadId) return
        if (!detail?.running) return
        setSubscriptionRestartByThreadId((current) => ({
          ...current,
          [threadId]: (current[threadId] ?? 0) + 1
        }))
      })
    }).catch((error) => {
      if (abort.signal.aborted) return
      setDetailState((current) => {
        if (current.threadId !== threadId) return current
        return { ...current, status: 'error', threadId, error: formatRuntimeError(error) }
      })
    })

    return () => {
      subscriptionClosed = true
      scheduler.flushNow()
      if (streamTurnId) flushPendingLiveBlocks(streamTurnId)
      scheduler.dispose()
      abort.abort()
      if (streamTurnId) clearActiveStream(threadId, streamTurnId)
    }
  }, [
    detailState.status,
    detailState.threadId,
    loadSelectedDetail,
    provider,
    runtimeConnection,
    selectedSubscriptionRestartToken,
    selectedThreadId
  ])

  const setSelectedInput = useCallback((value: string): void => {
    const key = selectedAgent?.key
    if (!key) return
    setInputBySubagentKey((current) => ({ ...current, [key]: value }))
  }, [selectedAgent?.key])

  const sendSelectedSubagentMessage = useCallback((): void => {
    const threadId = selectedThreadId
    const agentKey = selectedAgent?.key
    const rawInput = selectedInput
    const text = rawInput.trim()
    if (!threadId || !agentKey || !text || runtimeConnection !== 'ready' || selectedBusy) return
    setComposerError(null)
    setInputBySubagentKey((current) => ({ ...current, [agentKey]: '' }))
    void (async () => {
      try {
        const settings = await rendererRuntimeClient.getSettings()
        const prompt = buildCodeRuntimePrompt(settings, text)
        const model = composerModel.trim()
        const providerId = composerProviderId?.trim()
        const reasoningEffort = composerReasoningEffort
        const provisionalTurnId = pendingActiveStreamTurnId(threadId, 'subagent-inspector')
        setRunningTurnByThreadId((current) => ({ ...current, [threadId]: current[threadId] ?? 'pending' }))
        setTimelineRuntimeByThreadId((current) => ({
          ...current,
          [threadId]: {
            ...(current[threadId] ?? emptySubagentRuntimeState()),
            busy: true,
            currentTurnId: current[threadId]?.currentTurnId ?? provisionalTurnId,
            currentTurnUserId: null
          }
        }))
        const started = await provider.sendUserMessage(threadId, prompt, {
          mode: composerMode,
          ...(model ? { model } : {}),
          ...(providerId ? { providerId } : {}),
          ...(reasoningEffort ? { reasoningEffort } : {}),
          displayText: text
        })
        const previousTurnId = timelineRuntimeByThreadIdRef.current[threadId]?.currentTurnId
        if (previousTurnId && previousTurnId !== started.turnId) {
          migrateActiveStreamTurn(threadId, previousTurnId, started.turnId)
        }
        setRunningTurnByThreadId((current) => ({ ...current, [threadId]: started.turnId }))
        setTimelineRuntimeByThreadId((current) => ({
          ...current,
          [threadId]: {
            ...(current[threadId] ?? emptySubagentRuntimeState()),
            busy: true,
            currentTurnId: started.turnId,
            currentTurnUserId: started.userMessageItemId ?? current[threadId]?.currentTurnUserId ?? null,
            turnStartedAtByUserId: {
              ...(current[threadId]?.turnStartedAtByUserId ?? {}),
              ...(started.userMessageItemId ? { [started.userMessageItemId]: Date.now() } : {})
            }
          }
        }))
        await loadSelectedDetail('refresh')
      } catch (error) {
        setInputBySubagentKey((current) => ({ ...current, [agentKey]: rawInput }))
        setComposerError(formatRuntimeError(error))
        clearActiveStream(threadId)
        setRunningTurnByThreadId((current) => {
          if (!current[threadId]) return current
          const next = { ...current }
          delete next[threadId]
          return next
        })
        setTimelineRuntimeByThreadId((current) => ({
          ...current,
          [threadId]: {
            ...(current[threadId] ?? emptySubagentRuntimeState()),
            busy: false,
            currentTurnId: null,
            currentTurnUserId: null
          }
        }))
      }
    })()
  }, [
    composerMode,
    composerModel,
    composerProviderId,
    composerReasoningEffort,
    loadSelectedDetail,
    provider,
    runtimeConnection,
    selectedAgent?.key,
    selectedBusy,
    selectedInput,
    selectedThreadId
  ])

  const interruptSelectedSubagent = useCallback((options?: { discard?: boolean }): void => {
    const threadId = selectedThreadId
    if (!threadId) return
    const turnId =
      selectedRunningTurnId
      || (detailState.threadId === threadId ? detailState.latestTurnId : null)
      || selectedAgent?.childTurnId
      || null
    if (!turnId || turnId === 'pending') return
    void provider.interruptTurn(threadId, turnId, options)
      .catch((error) => setComposerError(formatRuntimeError(error)))
      .finally(() => {
        setRunningTurnByThreadId((current) => {
          if (!current[threadId]) return current
          const next = { ...current }
          delete next[threadId]
          return next
        })
        void loadSelectedDetail('refresh')
      })
  }, [
    detailState.latestTurnId,
    detailState.threadId,
    loadSelectedDetail,
    provider,
    selectedAgent?.childTurnId,
    selectedRunningTurnId,
    selectedThreadId
  ])

  const emptyQueuedMessages = useMemo<QueuedUserMessage[]>(() => [], [])

  return (
    <aside
      className={`write-assistant-panel ds-no-drag flex min-h-0 flex-col border-l border-ds-border-muted bg-white backdrop-blur-xl dark:bg-ds-canvas ${className}`}
      data-subagent-inspector-panel
    >
      <div className="shrink-0 border-b border-ds-border-muted bg-white/92 dark:bg-ds-card">
        <div className="ds-right-panel-topbar">
          <div className="ds-right-panel-title-group">
            {selectedAgent ? (
              selectedIsSideChat ? (
                <MessageCirclePlus className="ds-toolbar-icon-svg shrink-0" strokeWidth={1.9} aria-hidden />
              ) : (
                <SubagentGlyph
                  seed={selectedAgent.childThreadId ?? selectedAgent.childRunId ?? selectedAgent.key}
                  active={selectedAgent.status === 'active'}
                />
              )
            ) : onCreateSideChat ? (
              <MessageCirclePlus className="ds-toolbar-icon-svg shrink-0" strokeWidth={1.9} aria-hidden />
            ) : null}
            <span className="ds-right-panel-title">{selectedLabel}</span>
            <span className="ds-right-panel-subtitle">
              {selectedAgent ? subagentLifecycleLabel(selectedAgent) : '未选择'}
            </span>
          </div>
          <PanelCollapseButton
            onClick={onCollapse}
            ariaLabel={t('rightPanelCollapse')}
            title={t('rightPanelCollapse')}
          />
        </div>
        {!tabbedWorkspace && (subagents.length > 0 || onCreateSideChat) ? (
          <div className="ds-subagent-inspector-tab-row flex min-w-0 gap-1 border-t border-ds-border-muted px-3 pb-1 pt-2">
            <div className="ds-subagent-inspector-tab-scroll-shell min-w-0 flex-1">
              <div
                ref={tabScrollRef}
                role="tablist"
                aria-label={onCreateSideChat ? t('sidePanelTitle') : '子智能体'}
                data-fade-left={tabScrollState.left ? 'true' : 'false'}
                data-fade-right={tabScrollState.right ? 'true' : 'false'}
                className="ds-subagent-inspector-tab-scroll flex min-w-0 gap-1 overflow-x-auto"
              >
                {subagents.map((agent) => {
                  const active = agent.key === selectedAgent?.key
                  const sideConversation = isSideConversationInspectorAgent(agent)
                  const label = sideConversation ? sideChatTabLabel : subagentDisplayLabel(agent)
                  return (
                    <div
                      key={agent.key}
                      role="tab"
                      tabIndex={0}
                      aria-selected={active}
                      title={sideConversation ? agent.title : undefined}
                      className={`ds-subagent-inspector-tab flex h-8 min-w-0 max-w-[180px] shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 text-left text-[12.5px] font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
                        active
                          ? 'is-active bg-ds-hover text-ds-ink shadow-[inset_0_0_0_1px_var(--ds-border-muted)]'
                          : 'text-ds-muted hover:text-ds-ink'
                      }`}
                      onClick={() => onSelectSubagent(agent.key)}
                      onKeyDown={(event) => handleSubagentTabKeyDown(event, agent.key)}
                    >
                      {sideConversation ? (
                        <MessageCirclePlus className="ds-toolbar-icon-svg shrink-0" strokeWidth={1.9} aria-hidden />
                      ) : (
                        <SubagentGlyph
                          seed={agent.childThreadId ?? agent.childRunId ?? agent.key}
                          active={agent.status === 'active'}
                        />
                      )}
                      <span className="min-w-0 truncate">{label}</span>
                      {active && onCloseSubagentTab ? (
                        <button
                          type="button"
                          aria-label={t('close')}
                          title={t('close')}
                          className="ds-subagent-inspector-tab-close"
                          onClick={(event) => {
                            event.stopPropagation()
                            void onCloseSubagentTab(agent.key)
                          }}
                          onKeyDown={(event) => event.stopPropagation()}
                          onPointerDown={(event) => event.stopPropagation()}
                        >
                          <X className="h-3 w-3" strokeWidth={2.5} aria-hidden />
                        </button>
                      ) : null}
                    </div>
                  )
                })}
              </div>
              {tabScrollState.overflow ? (
                <div
                  ref={tabScrollbarRef}
                  className="ds-subagent-inspector-tab-scrollbar"
                  data-dragging={tabScrollbarDragging ? 'true' : 'false'}
                  aria-hidden="true"
                  onPointerDown={handleTabScrollbarPointerDown}
                  onPointerMove={handleTabScrollbarPointerMove}
                  onPointerUp={handleTabScrollbarPointerEnd}
                  onPointerCancel={handleTabScrollbarPointerEnd}
                >
                  <div
                    className="ds-subagent-inspector-tab-scrollbar-thumb"
                    style={{
                      width: `${tabScrollState.thumbWidth}%`,
                      left: `${tabScrollState.thumbLeft}%`
                    }}
                  />
                </div>
              ) : null}
            </div>
            {onCreateSideChat ? (
              <button
                type="button"
                aria-label={t('sidePanelNew')}
                title={t('sidePanelNew')}
                disabled={createSideChatDisabled}
                className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-ds-muted transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/30 ${
                  createSideChatDisabled
                    ? 'cursor-not-allowed opacity-45'
                    : 'hover:bg-ds-hover hover:text-ds-ink'
                }`}
                onClick={() => void onCreateSideChat()}
              >
                <Plus className="h-4 w-4" strokeWidth={2} aria-hidden />
              </button>
            ) : null}
          </div>
        ) : null}
      </div>

      <div className="min-h-0 flex-1 bg-ds-main/45 dark:bg-transparent">
	        {selectedAgent ? (
	          <div className="flex h-full min-h-0 flex-col">
	            <SubagentMetadata agent={selectedAgent} />
	            {selectedThreadId ? (
	              <SubagentTimeline
	                state={visibleDetailState}
                runtimeStateOverride={selectedTimelineStateOverride}
                runtimeConnection={runtimeConnection}
                onRetryConnection={onRetryConnection}
                onOpenSettings={onOpenSettings}
                quietEmpty={selectedIsSideChat}
              />
            ) : (
              <SubagentEmptyState
                title="没有可打开的子线程"
                detail="此子智能体还没有返回可查看的线程详情。"
              />
            )}
          </div>
        ) : (
          onCreateSideChat ? (
            <SubagentBlankState />
          ) : (
            <SubagentEmptyState
              title="暂无子智能体"
              detail="置顶摘要发现子智能体后，会在这里显示它们的工作内容。"
            />
          )
        )}
      </div>
      <div className="shrink-0 border-t border-ds-border-muted bg-white/92 px-4 pb-4 pt-3 dark:bg-ds-card">
        {composerError ? (
          <div className="mb-3 flex items-start gap-2 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-[12px] leading-5 text-red-600 dark:text-red-300">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <span className="min-w-0 break-words">{composerError}</span>
          </div>
        ) : null}
        <FloatingComposer
          variant="compact"
          input={selectedInput}
          setInput={setSelectedInput}
          mode={composerMode}
          setMode={setComposerMode}
          busy={selectedBusy}
          runtimeReady={runtimeConnection === 'ready'}
          hasActiveThread={Boolean(selectedThreadId)}
          composerModel={composerModel}
          composerProviderId={composerProviderId}
          composerPickList={composerPickList}
          composerModelGroups={composerModelGroups}
          composerReasoningEffort={composerReasoningEffort}
          onComposerModelChange={setComposerModel}
          onComposerReasoningEffortChange={setComposerReasoningEffort}
          onConfigureProviders={onConfigureProviders}
          modelPickerMode="combobox"
          hideThreadContextPanels
          queuedMessages={emptyQueuedMessages}
          onRemoveQueuedMessage={() => undefined}
          attachmentUploadEnabled={false}
          fileReferenceEnabled={false}
          onSend={sendSelectedSubagentMessage}
          onInterrupt={interruptSelectedSubagent}
          hideBtwCommand
        />
      </div>
    </aside>
	  )
	}

function SubagentMetadata({ agent }: { agent: CoreThreadSummarySubagentJson }): ReactElement | null {
  const rows = selectedSubagentMetaRows(agent)
  if (rows.length === 0) return null
  return (
    <div className="shrink-0 border-b border-ds-border-muted px-4 py-3">
      <dl className="flex flex-wrap gap-x-3 gap-y-1.5 text-[11.5px] leading-5">
        {rows.map((row) => (
          <div key={row.label} className="flex min-w-0 max-w-full items-center gap-1.5">
            <dt className="shrink-0 font-medium text-ds-faint">{row.label}</dt>
            <dd className="min-w-0 truncate text-ds-muted" title={row.value}>{row.value}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function SubagentTimeline({
  state,
  runtimeStateOverride,
  runtimeConnection,
  onRetryConnection,
  onOpenSettings,
  quietEmpty = false,
  loadingLabel = '正在加载子智能体...',
  emptyTitle = '暂无工作内容',
  emptyDetail = '子智能体线程已经创建，但还没有可展示的消息。'
}: {
  state: ThreadDetailState
  runtimeStateOverride: TimelineRuntimeStateOverride
  runtimeConnection: RuntimeConnectionStatus
  onRetryConnection: () => void
  onOpenSettings: () => void
  quietEmpty?: boolean
  loadingLabel?: string
  emptyTitle?: string
  emptyDetail?: string
}): ReactElement {
  if (state.status === 'loading' && state.blocks.length === 0) {
    if (quietEmpty) return <SubagentBlankState />
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center px-5 text-[13px] font-medium text-ds-muted">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        {loadingLabel}
      </div>
    )
  }

  if (state.status === 'error' && state.blocks.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center px-5">
        <div className="flex max-w-sm items-start gap-2 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-[12px] leading-5 text-red-600 dark:text-red-300">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span className="min-w-0 break-words">{state.error}</span>
        </div>
      </div>
    )
  }

  if (state.blocks.length === 0) {
    if (quietEmpty) return <SubagentBlankState />
    return (
      <SubagentEmptyState
        title={emptyTitle}
        detail={emptyDetail}
      />
    )
  }

  return (
    <MessageTimeline
      blocks={state.blocks}
      live=""
      activeThreadId={state.threadId}
      runtimeConnection={runtimeConnection}
      onRetryConnection={onRetryConnection}
      onOpenSettings={onOpenSettings}
      compactCards
      contentClassName="max-w-none px-5"
      runtimeStateOverride={runtimeStateOverride}
    />
  )
}

function SubagentBlankState(): ReactElement {
  return <div className="h-full min-h-0 flex-1" aria-hidden="true" />
}

function SubagentEmptyState({
  title,
  detail
}: {
  title: string
  detail: string
}): ReactElement {
  return (
    <div className="flex h-full min-h-0 flex-1 items-center justify-center px-6 text-center">
      <div className="max-w-[260px]">
        <div className="text-[13px] font-semibold text-ds-ink">{title}</div>
        <p className="mt-1 text-[12px] leading-5 text-ds-muted">{detail}</p>
      </div>
    </div>
  )
}
