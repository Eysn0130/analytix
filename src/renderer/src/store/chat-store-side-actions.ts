import type {
  AcceptedFinalProjectionBatch,
  AgentProvider,
  ChatBlock,
  CompactionBlock,
  GeneralTerminalProjectionBatch,
  RuntimeChildMetadata,
  RuntimeDisclosureMetadata,
  RuntimeStatusEventPayload,
  ThreadDeltaEvent,
  ThreadEventSink,
  ToolBlock,
  ToolEventPayload
} from '../agent/types'
import {
  acceptedFinalProjectionBatchIsSelfConsistent,
  acceptedFinalProjectionHasCandidateReceipt,
  acceptedFinalProjectionIsAlreadyCommitted,
  acceptedFinalProjectionReceiptsEqual,
  acceptedFinalUsageSnapshotsEqual
} from '../agent/accepted-final-projection-receipt'
import { DEFAULT_ANALYTIX_MODEL, type ModelReasoningEffort } from '@shared/app-settings'
import { projectModelReasoningEffortV1 } from '@shared/model-reasoning-effort'
import { projectToolEventForRenderer } from '../agent/analytix-mapper'
import { redactSecretText } from '@shared/secret-redaction'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'
import { markPublicProjectionRevoked } from '../lib/public-projection-revocation'
import type { ChatState, SideConversation, SidePanelState } from './chat-store-types'
import { hydrateBlockModelLabels } from './chat-store-helpers'
import {
  reconcileOptimisticUserBlock,
  settlePendingRuntimeWorkAfterInterrupt,
  threadSnapshotLooksRunning,
  upsertUserBlock
} from './chat-store-runtime-helpers'
import { createStreamingDeltaScheduler } from '../thread/streaming/streaming-delta-scheduler'
import {
  appendActiveStreamDeltas,
  clearActiveStream,
  getActiveStreamSnapshotFor,
  isPendingActiveStreamTurnId,
  migrateActiveStreamTurn,
  pendingActiveStreamTurnId
} from '../thread/streaming/active-stream-store'

type SideContext = {
  set: (partial: Partial<ChatState> | ((state: ChatState) => Partial<ChatState>)) => void
  get: () => ChatState
  getProvider: () => AgentProvider
  /** i18n reference (kept loose; the host already imports the default). */
  t: (key: string) => string
  formatRuntimeError: (error: unknown) => string
  shouldOpenSettingsForError: (error: unknown) => boolean
}

type ActiveSideAbort = {
  sideId: string
  abort: AbortController
}

type SideEventCursor = {
  turnId?: string | null
  seq?: number | null
  meta?: RuntimeDisclosureMetadata | Record<string, unknown> | null
}

type ApprovalBlock = Extract<ChatBlock, { kind: 'approval' }>
type UserInputBlock = Extract<ChatBlock, { kind: 'user_input' }>

const sideAbortControllers = new Map<string, AbortController>()
const sideLatestSeqByThread = new Map<string, number>()
const SIDE_TERMINAL_SNAPSHOT_UNAVAILABLE_KEY = 'common:runtimeFinalSnapshotUnavailable'

function createSideClientUserMessageId(): string {
  return globalThis.crypto.randomUUID()
}

function bindSideUserBlockTurn(blocks: ChatBlock[], userBlockId: string, turnId: string): ChatBlock[] {
  return blocks.map((block) => {
    if (block.kind !== 'user' || block.id !== userBlockId) return block
    return {
      ...block,
      meta: {
        ...(block.meta ?? {}),
        turnId
      }
    }
  })
}

function rememberSideSeq(sideId: string, seq: number | null | undefined): void {
  if (typeof seq !== 'number' || !Number.isFinite(seq)) return
  sideLatestSeqByThread.set(sideId, Math.max(sideLatestSeqByThread.get(sideId) ?? 0, seq))
}

function latestSideSeq(sideId: string, fallback = 0): number {
  return Math.max(fallback, sideLatestSeqByThread.get(sideId) ?? 0)
}

function compactTitlePrefix(value: string): string {
  return Array.from(value.trim()).slice(0, 5).join('')
}

function defaultSideTitle(parentTitle: string, parentThreadId: string): string {
  const trimmed = parentTitle.trim()
  if (trimmed) return `${compactTitlePrefix(trimmed)} · side`
  return `${parentThreadId.slice(0, 8)} · side`
}

function defaultSideModel(state: ChatState, parentThreadId: string): string {
  const parent = state.threads.find((thread) => thread.id === parentThreadId)
  if (parent?.model) return parent.model
  if (state.composerModel) return state.composerModel
  return DEFAULT_ANALYTIX_MODEL
}

function sideReasoningEffortRequestValue(value: unknown): ModelReasoningEffort | undefined {
  return projectModelReasoningEffortV1(value)
}

function sideApprovalBlockId(approvalId: string): string {
  return `approval_${approvalId}`
}

function upsertSideApprovalBlock(blocks: ChatBlock[], block: ApprovalBlock): ChatBlock[] {
  const idx = blocks.findIndex(
    (candidate) =>
      candidate.kind === 'approval' && (candidate.approvalId === block.approvalId || candidate.id === block.id)
  )
  if (idx < 0) return [...blocks, block]
  const current = blocks[idx]
  if (current.kind !== 'approval') return blocks
  const next: ApprovalBlock = {
    ...current,
    ...block,
    createdAt: current.createdAt ?? block.createdAt,
    status: current.status === 'pending' ? block.status : current.status,
    errorMessage: block.errorMessage ?? current.errorMessage,
    meta: {
      ...(current.meta ?? {}),
      ...(block.meta ?? {})
    }
  }
  const updated = [...blocks]
  updated[idx] = next
  return updated
}

function upsertSideUserInputBlock(blocks: ChatBlock[], block: UserInputBlock): ChatBlock[] {
  const idx = blocks.findIndex(
    (candidate) =>
      candidate.kind === 'user_input' &&
      (candidate.id === block.id || candidate.requestId === block.requestId || candidate.requestId === block.id)
  )
  if (idx < 0) return [...blocks, block]
  const current = blocks[idx]
  if (current.kind !== 'user_input') return blocks
  const next: UserInputBlock = {
    ...current,
    ...block,
    createdAt: current.createdAt ?? block.createdAt,
    status: current.status === 'pending' ? block.status : current.status,
    answers: block.answers ?? current.answers,
    errorMessage: block.errorMessage ?? current.errorMessage,
    meta: {
      ...(current.meta ?? {}),
      ...(block.meta ?? {})
    }
  }
  const updated = [...blocks]
  updated[idx] = next
  return updated
}

function upsertSideCompactionBlock(blocks: ChatBlock[], block: CompactionBlock): ChatBlock[] {
  const idx = blocks.findIndex((candidate) => candidate.kind === 'compaction' && candidate.id === block.id)
  if (idx < 0) return [...blocks, block]
  const current = blocks[idx]
  if (current.kind !== 'compaction') return blocks
  const next: CompactionBlock = {
    ...current,
    ...block,
    createdAt: current.createdAt ?? block.createdAt,
    detail: block.detail ?? current.detail,
    meta: {
      ...(current.meta ?? {}),
      ...(block.meta ?? {})
    }
  }
  const updated = [...blocks]
  updated[idx] = next
  return updated
}

function patchSide(
  state: ChatState,
  sideId: string,
  patch: (side: SideConversation) => SideConversation
): Partial<ChatState> {
  const current = state.sideConversations[sideId]
  if (!current) return {}
  const next = patch(current)
  if (next === current) return {}
  return { sideConversations: { ...state.sideConversations, [sideId]: next } }
}

function setSidePanel(panel: SidePanelState, patch: Partial<SidePanelState>): SidePanelState {
  return { ...panel, ...patch }
}

function flushSideLiveBlocks(side: SideConversation): { side: SideConversation; blocks: ChatBlock[] } {
  let nextBlocks = side.blocks
  const activeTurnId = side.turnId?.trim() || null
	const activeStream = activeTurnId ? getActiveStreamSnapshotFor(side.threadId, activeTurnId) : null
  let nextLiveAssistant = side.liveAssistant + (activeStream?.liveAssistant ?? '')
  if (nextLiveAssistant) {
    const id = `live_assistant_${side.lastSeq || Date.now()}`
    nextBlocks = [
      ...nextBlocks,
      {
        kind: 'assistant',
        id,
        createdAt: new Date().toISOString(),
        text: nextLiveAssistant,
        ...(activeTurnId ? { meta: { turnId: activeTurnId } } : {})
      }
    ]
    nextLiveAssistant = ''
  }
  if (activeTurnId && activeStream?.threadId) {
    clearActiveStream(side.threadId, activeTurnId)
  }
  if (nextBlocks === side.blocks) return { side, blocks: nextBlocks }
  return {
    side: { ...side, blocks: nextBlocks, liveAssistant: nextLiveAssistant },
    blocks: nextBlocks
  }
}

function sideBlockBelongsToTurn(block: ChatBlock, turnId: string): boolean {
  if (!('meta' in block) || typeof block.meta?.turnId !== 'string') return false
  return block.meta.turnId.trim() === turnId
}

function sideToolEventCallId(ev: ToolEventPayload): string {
  return typeof ev.meta?.callId === 'string' ? ev.meta.callId.trim() : ''
}

function sideToolEventTurnId(ev: ToolEventPayload, currentTurnId: string | null | undefined): string {
  if (typeof ev.turnId === 'string' && ev.turnId.trim()) return ev.turnId.trim()
  if (typeof ev.meta?.turnId === 'string' && ev.meta.turnId.trim()) return ev.meta.turnId.trim()
  return currentTurnId?.trim() ?? ''
}

function sideToolBlockCallId(block: ToolBlock): string {
  return typeof block.meta?.callId === 'string' ? block.meta.callId.trim() : ''
}

function sideToolBlockTurnId(block: ToolBlock): string {
  return typeof block.meta?.turnId === 'string' ? block.meta.turnId.trim() : ''
}

function sideToolChildMetadata(value: unknown): RuntimeChildMetadata | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const child = value as RuntimeChildMetadata
  if (!child.childRunId && !child.childId) return undefined
  return child
}

function sideToolEventChild(ev: ToolEventPayload): RuntimeChildMetadata | undefined {
  return sideToolChildMetadata(ev.meta?.child)
}

function sideToolBlockChild(block: ToolBlock): RuntimeChildMetadata | undefined {
  return sideToolChildMetadata(block.meta?.child)
}

function sideChildIdentity(child: RuntimeChildMetadata | undefined): string {
  return child?.childRunId?.trim() || child?.childId?.trim() || ''
}

function sideChildTurnBoundary(child: RuntimeChildMetadata | undefined, fallbackTurnId: string): string {
  return child?.parentTurnId?.trim() || fallbackTurnId
}

function sideChildThreadBoundary(child: RuntimeChildMetadata | undefined): string {
  return child?.parentThreadId?.trim() || ''
}

function sideToolBlockMatchesChildEvent(
  block: ToolBlock,
  ev: ToolEventPayload,
  eventTurnId: string,
  blockTurnId: string
): boolean {
  const eventChild = sideToolEventChild(ev)
  const blockChild = sideToolBlockChild(block)
  const eventChildId = sideChildIdentity(eventChild)
  if (!eventChild || !blockChild || !eventChildId || sideChildIdentity(blockChild) !== eventChildId) return false
  const eventChildTurn = sideChildTurnBoundary(eventChild, eventTurnId)
  const blockChildTurn = sideChildTurnBoundary(blockChild, blockTurnId)
  if (eventChildTurn && blockChildTurn && eventChildTurn !== blockChildTurn) return false
  const eventChildThread = sideChildThreadBoundary(eventChild)
  const blockChildThread = sideChildThreadBoundary(blockChild)
  if (eventChildThread && blockChildThread && eventChildThread !== blockChildThread) return false
  return true
}

function mergeSideToolMeta(
  current: ToolBlock['meta'] | undefined,
  incoming: ToolEventPayload['meta'] | undefined,
  turnId: string
): ToolBlock['meta'] {
  const currentChild = sideToolChildMetadata(current?.child)
  const incomingChild = sideToolChildMetadata(incoming?.child)
  return {
    ...(current ?? {}),
    ...(incoming ?? {}),
    ...(currentChild || incomingChild ? { child: { ...(currentChild ?? {}), ...(incomingChild ?? {}) } } : {}),
    ...(turnId ? { turnId } : {})
  }
}

function sideToolBlockMatchesEvent(
  block: ToolBlock,
  ev: ToolEventPayload,
  currentTurnId: string | null | undefined
): boolean {
  const eventTurnId = sideToolEventTurnId(ev, currentTurnId)
  const blockTurnId = sideToolBlockTurnId(block)
  const eventChild = sideToolEventChild(ev)
  const blockChild = sideToolBlockChild(block)
  if (sideToolBlockMatchesChildEvent(block, ev, eventTurnId, blockTurnId)) return true
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
  const callId = sideToolEventCallId(ev)
  if (!callId || sideToolBlockCallId(block) !== callId) return false
  if (eventTurnId && blockTurnId) return eventTurnId === blockTurnId
  if (eventTurnId && !blockTurnId) return currentTurnId?.trim() === eventTurnId
  if (!eventTurnId && blockTurnId) return currentTurnId?.trim() === blockTurnId
  return false
}

function withoutProvisionalSideAssistantBlocks(
  blocks: ChatBlock[],
  turnId: string | null | undefined,
  userBlockId: string | null | undefined
): ChatBlock[] {
  const normalizedTurnId = turnId?.trim()
  const normalizedUserBlockId = userBlockId?.trim()
  let insideCurrentUserTurn = false
  return blocks.filter((block) => {
    if (block.kind === 'user') {
      insideCurrentUserTurn = !!normalizedUserBlockId && block.id === normalizedUserBlockId
      return true
    }
    if (block.kind !== 'assistant') return true
    if (block.id.startsWith('live_assistant_')) return false
    if (normalizedTurnId && sideBlockBelongsToTurn(block, normalizedTurnId)) return false
    return !insideCurrentUserTurn
  })
}

function turnIdFromSideCursor(cursor: SideEventCursor | null | undefined): string | null {
  const direct = cursor?.turnId?.trim()
  if (direct) return direct
  const metaTurnId = cursor?.meta && typeof cursor.meta.turnId === 'string' ? cursor.meta.turnId.trim() : ''
  return metaTurnId || null
}

function nextSideTurnId(currentTurnId: string | null | undefined, activeTurnId: string | null | undefined): string | null {
  const current = currentTurnId?.trim() || null
  const active = activeTurnId?.trim() || null
  if (!active) return current
  if (!current || isPendingActiveStreamTurnId(current)) return active
  return current
}

function sideMatchesExpectedTerminalTurn(side: SideConversation, expectedTurnId: string): boolean {
  const currentTurnId = side.turnId?.trim() ?? ''
  if (!expectedTurnId) return !currentTurnId
  if (!currentTurnId) return false
  return currentTurnId === expectedTurnId || isPendingActiveStreamTurnId(currentTurnId)
}

function assertSideTerminalSnapshotIdentity(
  detail: Awaited<ReturnType<AgentProvider['getThreadDetail']>>,
  expectedTurnId: string,
  expectedUserBlockId: string,
  expectedAcceptedFinalDigest?: string
): void {
  if (threadSnapshotLooksRunning(detail.blocks, detail.threadStatus)) {
    throw new Error('terminal side snapshot still reports a running turn')
  }
  if (expectedTurnId) {
    const latestTurnId = detail.latestTurnId?.trim() ?? ''
    if (latestTurnId) {
      if (latestTurnId !== expectedTurnId) throw new Error('terminal side snapshot turn identity mismatch')
    } else {
      const containsExpectedTurn = detail.blocks.some((block) => sideBlockBelongsToTurn(block, expectedTurnId))
      if (!containsExpectedTurn) throw new Error('terminal side snapshot does not identify the expected turn')
    }
  }
  const latestUserMessageId = detail.latestUserMessageId?.trim() ?? ''
  if (expectedUserBlockId && latestUserMessageId && latestUserMessageId !== expectedUserBlockId) {
    throw new Error('terminal side snapshot user identity mismatch')
  }
  if (detail.historyAuthority === 'case_boundary_only_v1') {
    if (
      !expectedTurnId ||
      detail.latestTurnId?.trim() !== expectedTurnId ||
      !detail.latestTurnAcceptedFinalDigest ||
      !detail.blocks.some((block) => block.kind === 'assistant' && sideBlockBelongsToTurn(block, expectedTurnId))
    ) {
      throw new Error('case terminal side snapshot has no accepted-final authority')
    }
  }
  if (
    expectedAcceptedFinalDigest &&
    detail.latestTurnAcceptedFinalDigest !== expectedAcceptedFinalDigest
  ) {
    throw new Error('terminal side event and snapshot accepted-final digests differ')
  }
}

async function reconcileTerminalSideFromThreadDetail(input: {
  sideId: string
  turnId: string | null | undefined
  userBlockId: string | null | undefined
  provider: AgentProvider
  ctx: SideContext
  terminalError?: string | null
  acceptedFinalDigest?: string
}): Promise<void> {
  const sideId = input.sideId.trim()
  if (!sideId) return
  const expectedTurnId = input.turnId?.trim() ?? ''
  const expectedUserBlockId = input.userBlockId?.trim() ?? ''
  const quarantine = (side: SideConversation): SideConversation => ({
    ...side,
    blocks: withoutProvisionalSideAssistantBlocks(side.blocks, expectedTurnId, expectedUserBlockId),
    liveAssistant: ''
  })

  clearActiveStream(sideId)
  input.ctx.set((state) =>
    patchSide(state, sideId, (side) => {
      if (!sideMatchesExpectedTerminalTurn(side, expectedTurnId)) return side
      return quarantine(side)
    })
  )

  try {
    const detail = await input.provider.getThreadDetail(sideId)
    assertSideTerminalSnapshotIdentity(
      detail,
      expectedTurnId,
      expectedUserBlockId,
      input.acceptedFinalDigest
    )
    const loadedBlocks = hydrateBlockModelLabels(sideId, detail.blocks)
    const current = input.ctx.get().sideConversations[sideId]
    if (!current || current.threadId.trim() !== sideId || !sideMatchesExpectedTerminalTurn(current, expectedTurnId)) {
      throw new Error('terminal side state identity changed before reconciliation')
    }
    rememberSideSeq(sideId, detail.latestSeq)

    input.ctx.set((state) =>
      patchSide(state, sideId, (side) => {
        if (side.threadId.trim() !== sideId || !sideMatchesExpectedTerminalTurn(side, expectedTurnId)) return side
        return {
          ...side,
          blocks: loadedBlocks,
          liveAssistant: '',
          lastSeq: Math.max(side.lastSeq, detail.latestSeq),
          busy: false,
          turnId: null,
          userItemId: null,
          error: input.terminalError ?? null
        }
      })
    )
  } catch (error) {
    input.ctx.set((state) =>
      patchSide(state, sideId, (side) => {
        if (!sideMatchesExpectedTerminalTurn(side, expectedTurnId)) return side
        return {
          ...quarantine(side),
          busy: false,
          error: input.ctx.t(SIDE_TERMINAL_SNAPSHOT_UNAVAILABLE_KEY)
        }
      })
    )
    if (typeof window === 'undefined') return
    const message = redactSecretText(error instanceof Error ? error.message : String(error))
    void window.analytix?.logs?.error?.('side-terminal-reconcile', 'Failed to reconcile terminal side turn', {
      message,
      sideId,
      turnId: expectedTurnId || undefined,
      userBlockId: expectedUserBlockId || undefined
    }).catch(() => undefined)
  }
}

async function recoverSideSubscriptionAfterEnd(input: {
  sideId: string
  recoverySeq: number
  ctx: SideContext
  signal: AbortSignal
}): Promise<void> {
  const { sideId, ctx, signal } = input
  if (signal.aborted) return
  const side = ctx.get().sideConversations[sideId]
  if (!side?.busy) return
  const provider = ctx.getProvider()
  ctx.set((state) =>
    patchSide(state, sideId, (cur) => ({
      ...cur,
      error: ctx.t('common:runtimeStreamRecovering')
    }))
  )
  try {
    const detail = await provider.getThreadDetail(sideId)
    if (signal.aborted) return
    const blocks = hydrateBlockModelLabels(sideId, detail.blocks)
    const busy = threadSnapshotLooksRunning(blocks, detail.threadStatus)
    rememberSideSeq(sideId, detail.latestSeq)
    ctx.set((state) =>
      patchSide(state, sideId, (cur) => {
        const currentSeq = latestSideSeq(sideId, cur.lastSeq)
        const hasNewerLiveState = currentSeq > input.recoverySeq
        if (hasNewerLiveState) {
          return {
            ...cur,
            lastSeq: Math.max(cur.lastSeq, detail.latestSeq)
          }
        }
        return {
          ...cur,
          blocks,
          liveAssistant: '',
          lastSeq: Math.max(cur.lastSeq, detail.latestSeq),
          busy,
          turnId: busy ? detail.latestTurnId ?? cur.turnId : null,
          userItemId: detail.latestUserMessageId ?? cur.userItemId,
          error: busy ? ctx.t('common:runtimeStreamRecovering') : null
        }
      })
    )
    const recovered = ctx.get().sideConversations[sideId]
    if (!signal.aborted && recovered?.busy) {
      startSideSubscription(sideId, latestSideSeq(sideId, recovered.lastSeq), ctx)
    }
  } catch {
    if (signal.aborted) return
    clearActiveStream(sideId)
    ctx.set((state) =>
      patchSide(state, sideId, (cur) => {
        const turnId = cur.turnId?.trim() ?? ''
        return {
          ...cur,
          blocks: withoutProvisionalSideAssistantBlocks(cur.blocks, turnId, cur.userItemId),
          liveAssistant: '',
          busy: false,
          error: ctx.t(SIDE_TERMINAL_SNAPSHOT_UNAVAILABLE_KEY)
        }
      })
    )
  }
}

async function reconcileSideSnapshotFromThreadDetail(input: {
  sideId: string
  cursorSeq: number
  ctx: SideContext
}): Promise<void> {
  const sideId = input.sideId.trim()
  if (!sideId) return
  const provider = input.ctx.getProvider()
  try {
    const detail = await provider.getThreadDetail(sideId)
    const blocks = hydrateBlockModelLabels(sideId, detail.blocks)
    const busy = threadSnapshotLooksRunning(blocks, detail.threadStatus)
    rememberSideSeq(sideId, Math.max(detail.latestSeq, input.cursorSeq))
    input.ctx.set((state) =>
      patchSide(state, sideId, (side) => {
        clearActiveStream(sideId)
        return {
          ...side,
          blocks: busy ? blocks : settlePendingRuntimeWorkAfterInterrupt(blocks),
          liveAssistant: '',
          lastSeq: Math.max(side.lastSeq, detail.latestSeq, input.cursorSeq),
          busy,
          turnId: busy ? detail.latestTurnId ?? side.turnId : null,
          userItemId: detail.latestUserMessageId ?? side.userItemId,
          error: null
        }
      })
    )
  } catch (error) {
    if (typeof window === 'undefined') return
    void window.analytix?.logs?.error?.('side-snapshot-reconcile', 'Failed to reconcile side snapshot', {
      message: error instanceof Error ? error.message : String(error),
      threadId: sideId
    }).catch(() => undefined)
  }
}

function sideRuntimeStatusText(ev: RuntimeStatusEventPayload, t: SideContext['t']): string {
  const fixed = (key: string, fallback: string): string => {
    const translated = t(key)
    return translated && translated !== key ? translated : fallback
  }
  if (ev.child) return ''
  if (ev.kind === 'tool_result_upload_wait') return fixed('common:toolUploadWaitStatus', 'Tool results are being uploaded')
  if (ev.kind === 'tool_catalog_changed') return fixed('common:toolCatalogChangedStatus', 'Tool catalog changed')
  if (ev.kind === 'tool_storm_suppressed') return fixed('common:toolStormSuppressedStatus', 'Repeated tool activity was suppressed')
  if (ev.kind === 'compaction_summary_fallback') return fixed('common:compactionSummaryFallbackStatus', 'Context compaction summary is unavailable')
  if (ev.kind !== 'pipeline_stage') return ''
  switch (ev.stage) {
    case 'pre_send':
      return fixed('common:providerRequestPreparingStatus', 'Preparing model request')
    case 'post_send':
      return fixed('common:providerRequestStartedStatus', 'Model request started')
    case 'response_received':
      return fixed('common:providerResponseReceivedStatus', 'Response received')
    case 'provider_retrying':
      return fixed('common:providerRetryingStatus', 'Provider request is retrying')
    case 'provider_error':
      return fixed('common:providerErrorStatus', 'Provider request failed')
    case 'empty_final_recovered':
      return fixed('common:emptyFinalRecoveredStatus', 'Empty provider response was replaced safely')
    default:
      return ''
  }
}

function sideRuntimeStatusMeta(ev: RuntimeStatusEventPayload): RuntimeDisclosureMetadata | undefined {
  const meta: RuntimeDisclosureMetadata = {
    ...(ev.turnId ? { turnId: ev.turnId } : {}),
    ...(ev.child ? { child: ev.child } : {}),
    ...(ev.attempt !== undefined || ev.maxAttempt !== undefined
      ? { providerRetry: { ...(ev.attempt !== undefined ? { attempt: ev.attempt } : {}), ...(ev.maxAttempt !== undefined ? { maxAttempt: ev.maxAttempt } : {}) } }
      : {})
  }
  return Object.keys(meta).length ? meta : undefined
}

function guardSideSink(sink: ThreadEventSink, signal: AbortSignal): ThreadEventSink {
  const isActive = (): boolean => !signal.aborted
  return {
    onSeq: (seq) => {
      if (isActive()) sink.onSeq(seq)
    },
    onTurnStarted: (ev) => {
      if (isActive()) sink.onTurnStarted?.(ev)
    },
    onTurnSteered: (ev) => {
      if (isActive()) sink.onTurnSteered?.(ev)
    },
    onDeltas: (deltas) => {
      if (isActive()) sink.onDeltas(deltas)
    },
    onUserMessage: (ev) => {
      if (isActive()) sink.onUserMessage(ev)
    },
    onTool: (ev) => {
      if (isActive()) sink.onTool(ev)
    },
    onCompaction: (ev) => {
      if (isActive()) sink.onCompaction(ev)
    },
    onReview: (ev) => {
      if (isActive()) sink.onReview?.(ev)
    },
    onRuntimeStatus: (ev) => {
      if (isActive()) sink.onRuntimeStatus?.(ev)
    },
    onRuntimeError: (ev) => {
      if (isActive()) sink.onRuntimeError?.(ev)
    },
    onThreadLifecycle: (ev) => {
      if (isActive()) sink.onThreadLifecycle?.(ev)
    },
    onThreadRewound: (ev) => {
      if (isActive()) sink.onThreadRewound?.(ev)
    },
    onApproval: (req) => {
      if (isActive()) sink.onApproval(req)
    },
    onApprovalStatus: (ev) => {
      if (isActive()) sink.onApprovalStatus?.(ev)
    },
    onUserInput: (req) => {
      if (isActive()) sink.onUserInput(req)
    },
    onUserInputStatus: (ev) => {
      if (isActive()) sink.onUserInputStatus(ev)
    },
    onGoal: (ev) => {
      if (isActive()) sink.onGoal(ev)
    },
    onTodos: (ev) => {
      if (isActive()) sink.onTodos?.(ev)
    },
    onSnapshotRequired: (ev) => {
      if (!isActive()) return undefined
      return sink.onSnapshotRequired?.(ev)
    },
    onPublicProjectionRevoked: async (ev) => {
      if (!isActive() || !sink.onPublicProjectionRevoked) return
      await sink.onPublicProjectionRevoked(ev)
    },
    onAcceptedFinalBatch: async (batch) => {
      if (!isActive() || !sink.onAcceptedFinalBatch) {
        throw new Error('accepted-final delivery belongs to an inactive side stream')
      }
      return await sink.onAcceptedFinalBatch(batch)
    },
    onGeneralTerminalBatch: async (batch) => {
      if (!isActive() || !sink.onGeneralTerminalBatch) {
        throw new Error('general terminal delivery belongs to an inactive side stream')
      }
      await sink.onGeneralTerminalBatch(batch)
    },
    onTurnComplete: async (ev) => {
      if (isActive()) await sink.onTurnComplete(ev)
    },
    onError: async (err, options) => {
      if (isActive()) await sink.onError(err, options)
    },
    onUsage: (usage) => {
      if (isActive()) sink.onUsage?.(usage)
    }
  }
}

function buildSideSink(sideId: string, ctx: SideContext, sinceSeq = 0, signal?: AbortSignal): ThreadEventSink {
  const isAborted = (): boolean => signal?.aborted ?? false
  // Replayed or re-delivered deltas duplicate text already on screen;
  // drop anything at or below the subscription's replay floor.
  let appliedDeltaSeqFloor = sinceSeq
  let streamTurnId = ctx.get().sideConversations[sideId]?.turnId?.trim() || null
  const adoptStreamTurnId = (turnId?: string | null): string => {
    const normalizedTurnId = turnId?.trim() || null
    if (normalizedTurnId) {
      if (streamTurnId && streamTurnId !== normalizedTurnId && isPendingActiveStreamTurnId(streamTurnId)) {
        migrateActiveStreamTurn(sideId, streamTurnId, normalizedTurnId)
      }
      streamTurnId = normalizedTurnId
      return streamTurnId
    }
    if (!streamTurnId) {
      streamTurnId = pendingActiveStreamTurnId(sideId, 'side')
    }
    return streamTurnId
  }
  const adoptEventCursor = (cursor?: SideEventCursor | null): string | null => {
    rememberSideSeq(sideId, cursor?.seq)
    const turnId = turnIdFromSideCursor(cursor)
    return turnId ? adoptStreamTurnId(turnId) : streamTurnId
  }
  const sideCursorPatch = (side: SideConversation, activeTurnId: string | null): Pick<SideConversation, 'lastSeq' | 'turnId'> => ({
    lastSeq: latestSideSeq(sideId, side.lastSeq),
    turnId: nextSideTurnId(side.turnId, activeTurnId)
  })
  const flushSideDeltas = ({ assistant, lastSeq, turnId }: { assistant: string; lastSeq: number; turnId: string | null }): void => {
    if (isAborted()) return
    const activeTurnId = adoptStreamTurnId(turnId)
    rememberSideSeq(sideId, lastSeq)
    appendActiveStreamDeltas({
      threadId: sideId,
      turnId: activeTurnId,
      assistant,
      lastSeq
    })
    ctx.set((s) =>
      patchSide(s, sideId, (side) => {
        const nextLastSeq = side.turnId || side.busy ? side.lastSeq : latestSideSeq(sideId, side.lastSeq)
        const nextTurnId = nextSideTurnId(side.turnId, activeTurnId)
        if (side.busy && side.turnId === nextTurnId && side.lastSeq === nextLastSeq) return side
        return {
          ...side,
          lastSeq: nextLastSeq,
          turnId: nextTurnId,
          busy: true,
          error: null
        }
      })
    )
  }
  const streamingDeltaScheduler = createStreamingDeltaScheduler({
    flushFirstSynchronously: true,
    onFlush: flushSideDeltas
  })
  const flushPendingDeltas = (): void => {
    if (isAborted()) return
    streamingDeltaScheduler.flushNow()
  }
  const sink: ThreadEventSink = {
    onSeq: (seq) => {
      rememberSideSeq(sideId, seq)
    },
    onTurnStarted: (ev) => {
      flushPendingDeltas()
      const activeTurnId = adoptStreamTurnId(ev.turnId)
      rememberSideSeq(sideId, (ev as { seq?: number }).seq)
      ctx.set((s) =>
        patchSide(s, sideId, (side) => ({
          ...side,
          busy: true,
          turnId: activeTurnId,
          lastSeq: latestSideSeq(sideId, side.lastSeq),
          error: null
        }))
      )
    },
    onUserMessage: (ev) => {
      flushPendingDeltas()
      const activeTurnId = adoptStreamTurnId(ev.turnId)
      rememberSideSeq(sideId, (ev as { seq?: number }).seq)
      ctx.set((s) =>
        patchSide(s, sideId, (side) => {
          const flushed = flushSideLiveBlocks(side)
          const optimisticUserId = side.userItemId
          const reconciledBlocks =
            optimisticUserId &&
            optimisticUserId !== ev.itemId &&
            flushed.blocks.some((block) => block.kind === 'user' && block.id === optimisticUserId)
              ? reconcileOptimisticUserBlock(flushed.blocks, optimisticUserId, ev.itemId, ev.text)
              : flushed.blocks
          const blocks = upsertUserBlock(reconciledBlocks, ev)
          return {
            ...flushed.side,
            blocks,
            busy: true,
            turnId: activeTurnId,
            lastSeq: latestSideSeq(sideId, side.lastSeq),
            userItemId: ev.itemId,
            error: null
          }
        })
      )
    },
    onDeltas: (rawDeltas) => {
      const deltas: ThreadDeltaEvent[] = []
      let batchMaxSeq = appliedDeltaSeqFloor
      for (const delta of rawDeltas) {
		if (typeof delta.seq === 'number') {
          if (delta.seq <= appliedDeltaSeqFloor) continue
          batchMaxSeq = Math.max(batchMaxSeq, delta.seq)
		  rememberSideSeq(sideId, delta.seq)
		}
		if (delta.turnId) adoptStreamTurnId(delta.turnId)
        deltas.push(delta)
      }
      appliedDeltaSeqFloor = batchMaxSeq
      if (deltas.length === 0) return
      streamingDeltaScheduler.enqueue(deltas)
    },
    onTool: (ev: ToolEventPayload) => {
      const projectedEvent = projectToolEventForRenderer(ev)
      if (!projectedEvent) return
      ev = projectedEvent
      flushPendingDeltas()
      const activeTurnId = adoptEventCursor(ev)
      ctx.set((s) =>
        patchSide(s, sideId, (side) => {
          const flushed = flushSideLiveBlocks(side)
          const eventTurnId = sideToolEventTurnId(ev, activeTurnId ?? side.turnId)
          const idx = flushed.blocks.findIndex((b) =>
            b.kind === 'tool' && sideToolBlockMatchesEvent(b, ev, activeTurnId ?? side.turnId)
          )
          let blocks: ChatBlock[]
          if (idx >= 0) {
            const cur = flushed.blocks[idx]
            if (cur.kind !== 'tool') return flushed.side
            const next: ToolBlock = {
              ...cur,
              summary: ev.summary || cur.summary,
              status: ev.status,
              toolKind: ev.toolKind ?? cur.toolKind,
              detail: ev.detail ?? cur.detail,
              filePath: ev.filePath ?? cur.filePath,
              meta: mergeSideToolMeta(cur.meta, ev.meta, eventTurnId)
            }
            blocks = [...flushed.blocks]
            blocks[idx] = next
          } else {
            const blockId = flushed.blocks.some((candidate) => candidate.id === ev.itemId)
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
              meta: mergeSideToolMeta(undefined, ev.meta, eventTurnId)
            }
            blocks = [...flushed.blocks, block]
          }
          return { ...flushed.side, ...sideCursorPatch(flushed.side, activeTurnId), blocks, busy: true, error: null }
        })
      )
    },
    onCompaction: (ev) => {
      flushPendingDeltas()
      const activeTurnId = adoptEventCursor(ev as SideEventCursor)
      ctx.set((s) =>
        patchSide(s, sideId, (side) => {
          const flushed = flushSideLiveBlocks(side)
          const block: CompactionBlock = {
            kind: 'compaction',
            id: ev.itemId,
            createdAt: ev.createdAt ?? new Date().toISOString(),
            summary: ev.summary,
            status: ev.status,
            detail: ev.detail,
            auto: ev.auto,
            meta: {
              ...(ev.meta ?? {}),
              ...(ev.turnId ? { turnId: ev.turnId } : {})
            }
          }
          return {
            ...flushed.side,
            ...sideCursorPatch(flushed.side, activeTurnId),
            blocks: upsertSideCompactionBlock(flushed.blocks, block)
          }
        })
      )
    },
    onRuntimeStatus: (ev) => {
      flushPendingDeltas()
      const activeTurnId = adoptEventCursor(ev as SideEventCursor)
      const text = sideRuntimeStatusText(ev, ctx.t)
      if (!text) return
      ctx.set((s) =>
        patchSide(s, sideId, (side) => {
          const flushed = flushSideLiveBlocks(side)
          const meta = sideRuntimeStatusMeta(ev)
          const block: Extract<ChatBlock, { kind: 'system' }> = {
            kind: 'system',
            id: ev.itemId,
            createdAt: ev.createdAt ?? new Date().toISOString(),
            text,
            ...(meta ? { meta } : {})
          }
          const index = flushed.blocks.findIndex((candidate) => candidate.kind === 'system' && candidate.id === ev.itemId)
          if (index >= 0) {
            const blocks = [...flushed.blocks]
            blocks[index] = block
            return { ...flushed.side, ...sideCursorPatch(flushed.side, activeTurnId), blocks, busy: true, error: null }
          }
          return { ...flushed.side, ...sideCursorPatch(flushed.side, activeTurnId), blocks: [...flushed.blocks, block], busy: true, error: null }
        })
      )
    },
    onApproval: (req) => {
      flushPendingDeltas()
      const activeTurnId = adoptEventCursor(req as SideEventCursor)
      ctx.set((s) =>
        patchSide(s, sideId, (side) => {
          const flushed = flushSideLiveBlocks(side)
          const block: ApprovalBlock = {
            kind: 'approval',
            id: sideApprovalBlockId(req.approvalId),
            createdAt: new Date().toISOString(),
            approvalId: req.approvalId,
            summary: req.summary,
            toolName: req.toolName,
            status: 'pending',
            meta: {
              ...(req.meta ?? {}),
              ...(req.turnId ? { turnId: req.turnId } : {})
            }
          }
          return {
            ...flushed.side,
            blocks: upsertSideApprovalBlock(flushed.blocks, block),
            ...sideCursorPatch(flushed.side, activeTurnId),
            ...(activeTurnId ? { busy: true, error: null } : {})
          }
        })
      )
    },
    onApprovalStatus: (ev) => {
      flushPendingDeltas()
      ctx.set((s) =>
        patchSide(s, sideId, (side) => ({
          ...side,
          blocks: side.blocks.map((block) =>
            block.kind === 'approval' && (block.approvalId === ev.approvalId || block.id === ev.itemId)
              ? { ...block, status: ev.status, errorMessage: ev.errorMessage ?? block.errorMessage }
              : block
          )
        }))
      )
    },
    onUserInput: (req) => {
      flushPendingDeltas()
      const activeTurnId = adoptEventCursor(req as SideEventCursor)
      ctx.set((s) =>
        patchSide(s, sideId, (side) => {
          const flushed = flushSideLiveBlocks(side)
          const block: UserInputBlock = {
            kind: 'user_input',
            id: req.itemId,
            createdAt: new Date().toISOString(),
            requestId: req.requestId,
            questions: req.questions,
            status: 'pending',
            meta: {
              ...(req.meta ?? {}),
              ...(req.turnId ? { turnId: req.turnId } : {})
            }
          }
          return {
            ...flushed.side,
            blocks: upsertSideUserInputBlock(flushed.blocks, block),
            ...sideCursorPatch(flushed.side, activeTurnId),
            ...(activeTurnId ? { busy: true, error: null } : {})
          }
        })
      )
    },
    onUserInputStatus: (ev) => {
      flushPendingDeltas()
      ctx.set((s) =>
        patchSide(s, sideId, (side) => ({
          ...side,
          blocks: side.blocks.map((block) =>
            block.kind === 'user_input' &&
            (block.id === ev.itemId || block.requestId === ev.itemId || block.requestId === ev.requestId)
              ? {
                  ...block,
                  status: ev.status,
                  answers: ev.answers ?? block.answers,
                  errorMessage: ev.errorMessage ?? block.errorMessage
                }
              : block
          )
        }))
      )
    },
    onGoal: () => {
      // Side conversations do not render goal chips yet.
    },
    onTodos: () => {
      // Side conversations do not render runtime todo chips yet.
    },
    onSnapshotRequired: async (ev) => {
      flushPendingDeltas()
      const cursorSeq = Math.max(ev.seq ?? 0, ev.highestSeq ?? 0)
      rememberSideSeq(sideId, cursorSeq)
      if (cursorSeq > appliedDeltaSeqFloor) appliedDeltaSeqFloor = cursorSeq
      await reconcileSideSnapshotFromThreadDetail({
        sideId,
        cursorSeq,
        ctx
      })
    },
    onPublicProjectionRevoked: async (ev) => {
      if (isAborted() || ev.threadId !== sideId) return
      markPublicProjectionRevoked(ev.threadId)
      streamingDeltaScheduler.discard()
      teardownSideSubscription(sideId)
      clearActiveStream(sideId)
      sideLatestSeqByThread.delete(sideId)
      ctx.set((state) => {
        const next = { ...state.sideConversations }
        const revoked = next[sideId]
        delete next[sideId]
        const nextActiveId = state.sidePanel.activeSideId === sideId && revoked
          ? Object.values(next).find((candidate) =>
              candidate.parentThreadId === revoked.parentThreadId
            )?.threadId ?? null
          : state.sidePanel.activeSideId
        return {
          sideConversations: next,
          sidePanel: {
            open: Boolean(nextActiveId) && state.sidePanel.open,
            activeSideId: nextActiveId
          }
        }
      })
    },
    onGeneralTerminalBatch: async (batch: GeneralTerminalProjectionBatch) => {
      if (isAborted() || batch.threadId !== sideId) {
        throw new Error('general terminal side delivery binding is invalid')
      }
      const before = ctx.get().sideConversations[sideId]
      const beforeCursor = before ? latestSideSeq(sideId, before.lastSeq) : -1
      if (!before || !/^[a-f0-9]{64}$/.test(batch.batchDigest) ||
          beforeCursor !== batch.firstSeq - 1 || before.turnId?.trim() !== batch.turnId ||
          (batch.terminalItem?.kind === 'assistant' && batch.terminal.status !== 'completed') ||
          (batch.terminalItem?.kind === 'system' && batch.terminal.status === 'completed')) {
        streamingDeltaScheduler.discard()
        clearActiveStream(sideId)
        streamTurnId = null
        ctx.set((state) => patchSide(state, sideId, (side) => ({
          ...side,
          blocks: settlePendingRuntimeWorkAfterInterrupt(side.blocks.filter((block) =>
            block.kind !== 'assistant' || block.meta?.turnId !== batch.turnId
          )),
          liveAssistant: '',
          busy: false,
          turnId: side.turnId?.trim() === batch.turnId ? null : side.turnId,
          userItemId: side.turnId?.trim() === batch.turnId ? null : side.userItemId,
          error: ctx.t(SIDE_TERMINAL_SNAPSHOT_UNAVAILABLE_KEY)
        })))
        throw new Error('general terminal side delivery could not be committed atomically')
      }

      streamingDeltaScheduler.discard()
      let committed = false
      ctx.set((state) => patchSide(state, sideId, (side) => {
        if (latestSideSeq(sideId, side.lastSeq) !== batch.firstSeq - 1 ||
            side.turnId?.trim() !== batch.turnId) return side
        const userIndex = side.userItemId
          ? side.blocks.findIndex((block) => block.kind === 'user' && block.id === side.userItemId)
          : -1
        let nextUserIndex = side.blocks.length
        if (userIndex >= 0) {
          for (let index = userIndex + 1; index < side.blocks.length; index += 1) {
            if (side.blocks[index]?.kind === 'user') {
              nextUserIndex = index
              break
            }
          }
        }
        const withoutProvisional = side.blocks.filter((block, index) => {
          if (block.kind !== 'assistant') return true
          if (block.meta?.turnId === batch.turnId) return false
          return !(userIndex >= 0 && index > userIndex && index < nextUserIndex)
        }).filter((block) => block.id !== batch.terminalItem?.id)
        committed = true
        return {
          ...side,
          blocks: [
            ...settlePendingRuntimeWorkAfterInterrupt(withoutProvisional),
            ...(batch.terminalItem ? [batch.terminalItem] : [])
          ],
          liveAssistant: '',
          lastSeq: batch.lastSeq,
          busy: false,
          turnId: null,
          userItemId: null,
          error: null,
          lastTurnUsage: batch.usage
        }
      }))
      if (!committed) {
        throw new Error('general terminal side delivery could not be committed atomically')
      }
      rememberSideSeq(sideId, batch.lastSeq)
      clearActiveStream(sideId)
      streamTurnId = null
    },
    onAcceptedFinalBatch: async (batch: AcceptedFinalProjectionBatch) => {
      if (isAborted() || batch.threadId !== sideId) {
        throw new Error('accepted-final side delivery binding is invalid')
      }
      const before = ctx.get().sideConversations[sideId]
      if (!before) {
        throw new Error('accepted-final side delivery could not be committed atomically')
      }
      const quarantineRejectedProjection = (): void => {
        const currentTurnMatches = before.turnId?.trim() === batch.turnId
        const hasCandidate = acceptedFinalProjectionHasCandidateReceipt(before.blocks, batch)
        if (!currentTurnMatches && !hasCandidate) return
        streamingDeltaScheduler.discard()
        clearActiveStream(sideId)
        streamTurnId = null
        ctx.set((state) => patchSide(state, sideId, (side) => {
          const sideCurrentTurnMatches = side.turnId?.trim() === batch.turnId
          const sideHasCandidate = acceptedFinalProjectionHasCandidateReceipt(side.blocks, batch)
          if (!sideCurrentTurnMatches && !sideHasCandidate) return side
          const blocks = side.blocks.filter((block) =>
            block.id !== batch.assistant.id && block.id !== batch.terminalError?.id &&
            !(block.kind === 'assistant' &&
              (block.meta?.turnId === batch.turnId ||
                acceptedFinalProjectionReceiptsEqual(
                  block.acceptedFinalProjectionReceipt,
                  batch.receipt
                )))
          )
          return {
            ...side,
            blocks: settlePendingRuntimeWorkAfterInterrupt(blocks),
            liveAssistant: '',
            busy: false,
            turnId: sideCurrentTurnMatches ? null : side.turnId,
            userItemId: sideCurrentTurnMatches ? null : side.userItemId,
            lastTurnUsage: sideHasCandidate ? null : side.lastTurnUsage,
            error: ctx.t(SIDE_TERMINAL_SNAPSHOT_UNAVAILABLE_KEY)
          }
        }))
      }
      const reject = (message: string): never => {
        quarantineRejectedProjection()
        throw new Error(message)
      }
      if (!acceptedFinalProjectionBatchIsSelfConsistent(batch)) {
        return reject('accepted-final side delivery binding is invalid')
      }
      const beforeCursor = latestSideSeq(sideId, before.lastSeq)
      if (acceptedFinalProjectionIsAlreadyCommitted(before.blocks, beforeCursor, batch)) {
        const terminalStateComplete = before.busy === false && before.liveAssistant === '' &&
          before.turnId === null && before.userItemId === null && before.error === null &&
          acceptedFinalUsageSnapshotsEqual(before.lastTurnUsage, batch.usage)
        if (!terminalStateComplete) {
          return reject('accepted-final side delivery could not be committed atomically')
        }
        streamingDeltaScheduler.discard()
        rememberSideSeq(sideId, batch.lastSeq)
        clearActiveStream(sideId)
        streamTurnId = null
        return batch.receipt
      }
      if (acceptedFinalProjectionHasCandidateReceipt(before.blocks, batch) ||
          beforeCursor !== batch.firstSeq - 1 || before.turnId?.trim() !== batch.turnId) {
        return reject('accepted-final side delivery could not be committed atomically')
      }

      streamingDeltaScheduler.discard()
      let committed = false
      ctx.set((state) => patchSide(state, sideId, (side) => {
        const sideCursor = latestSideSeq(sideId, side.lastSeq)
        if (sideCursor !== batch.firstSeq - 1 || side.turnId?.trim() !== batch.turnId ||
            acceptedFinalProjectionIsAlreadyCommitted(side.blocks, sideCursor, batch) ||
            acceptedFinalProjectionHasCandidateReceipt(side.blocks, batch)) return side
        const userIndex = side.userItemId
          ? side.blocks.findIndex((block) => block.kind === 'user' && block.id === side.userItemId)
          : -1
        let nextUserIndex = side.blocks.length
        if (userIndex >= 0) {
          for (let index = userIndex + 1; index < side.blocks.length; index += 1) {
            if (side.blocks[index]?.kind === 'user') {
              nextUserIndex = index
              break
            }
          }
        }
        const withoutProvisional = side.blocks.filter((block, index) => {
          if (block.kind !== 'assistant') return true
          if (block.meta?.turnId === batch.turnId) return false
          return !(userIndex >= 0 && index > userIndex && index < nextUserIndex)
        }).filter((block) => block.id !== batch.assistant.id && block.id !== batch.terminalError?.id)
        const assistant = {
          ...batch.assistant,
          acceptedFinalProjectionReceipt: batch.receipt,
          acceptedFinalProjectionTerminal: batch.terminal
        }
        committed = true
        return {
          ...side,
          blocks: [
            ...settlePendingRuntimeWorkAfterInterrupt(withoutProvisional),
            assistant,
            ...(batch.terminalError ? [batch.terminalError] : [])
          ],
          liveAssistant: '',
          lastSeq: batch.lastSeq,
          busy: false,
          turnId: null,
          userItemId: null,
          error: null,
          lastTurnUsage: batch.usage
        }
      }))
      if (!committed) {
        return reject('accepted-final side delivery could not be committed atomically')
      }
      rememberSideSeq(sideId, batch.lastSeq)
      clearActiveStream(sideId)
      streamTurnId = null
      return batch.receipt
    },
    onTurnComplete: async (ev) => {
      rememberSideSeq(sideId, ev?.seq)
      const eventTurnId = turnIdFromSideCursor(ev as SideEventCursor)
      if (eventTurnId) adoptStreamTurnId(eventTurnId)
      const completedSide = ctx.get().sideConversations[sideId] ?? null
      const activeSideTurnId = completedSide?.turnId?.trim() || null
      if (
        eventTurnId &&
        activeSideTurnId &&
        !isPendingActiveStreamTurnId(activeSideTurnId) &&
        activeSideTurnId !== eventTurnId
      ) {
        ctx.set((s) =>
          patchSide(s, sideId, (side) => ({
            ...side,
            lastSeq: latestSideSeq(sideId, side.lastSeq)
          }))
        )
        return
      }
      teardownSideSubscription(sideId)
      streamingDeltaScheduler.dispose()
      const completedTurnId = eventTurnId ?? streamTurnId ?? activeSideTurnId
      const completedUserBlockId = completedSide?.userItemId ?? null
      const provider = ctx.getProvider()
      await reconcileTerminalSideFromThreadDetail({
        sideId,
        turnId: completedTurnId,
        userBlockId: completedUserBlockId,
        provider,
        ctx,
        acceptedFinalDigest: ev?.acceptedFinalDigest
      })
      streamTurnId = null
    },
    onError: async (err, options) => {
      if (!options?.terminal) {
        ctx.set((s) =>
          patchSide(s, sideId, (side) => ({
            ...side,
            error: ctx.formatRuntimeError(err)
          }))
        )
        return
      }
      teardownSideSubscription(sideId)
      streamingDeltaScheduler.dispose()
      const side = ctx.get().sideConversations[sideId]
      if (options.threadId?.trim() && options.threadId.trim() !== sideId) return
      await reconcileTerminalSideFromThreadDetail({
        sideId,
        turnId: options.turnId?.trim() || streamTurnId || side?.turnId || null,
        userBlockId: side?.userItemId ?? null,
        provider: ctx.getProvider(),
        ctx,
        terminalError: ctx.formatRuntimeError(err),
        acceptedFinalDigest: options.acceptedFinalDigest
      })
      streamTurnId = null
    },
    onUsage: (usage) => {
      // Side usage is reported only to keep lastSeq cursors consistent;
      // a per-thread usage counter can be wired here in the future.
      void usage
    }
  }
  return signal ? guardSideSink(sink, signal) : sink
}

function teardownSideSubscription(sideId: string): void {
  const ac = sideAbortControllers.get(sideId)
  if (ac) {
    ac.abort()
    sideAbortControllers.delete(sideId)
  }
}

function startSideSubscription(sideId: string, sinceSeq: number, ctx: SideContext): void {
  teardownSideSubscription(sideId)
  const ac = new AbortController()
  sideAbortControllers.set(sideId, ac)
  const sink = buildSideSink(sideId, ctx, sinceSeq, ac.signal)
  const provider = ctx.getProvider()
  void provider.subscribeThreadEvents(sideId, sinceSeq, sink, ac.signal)
    .catch(() => undefined)
    .then(() => {
      if (ac.signal.aborted) return
      const side = ctx.get().sideConversations[sideId]
      if (!side?.busy) return
      void recoverSideSubscriptionAfterEnd({
        sideId,
        recoverySeq: latestSideSeq(sideId, side.lastSeq),
        ctx,
        signal: ac.signal
      })
    })
}

export function createSideActions(ctx: SideContext): Pick<
  ChatState,
  | 'spawnSideConversation'
  | 'openSideConversationDraft'
  | 'sendSideMessage'
  | 'interruptSide'
  | 'setSideInput'
  | 'setSideModel'
  | 'setSideReasoningEffort'
  | 'selectSideConversation'
  | 'openThreadInSidePanel'
  | 'setSidePanelOpen'
  | 'closeSideConversation'
  | 'discardSideConversation'
  | 'promoteSideConversation'
> {
  const actions: Pick<
    ChatState,
    | 'spawnSideConversation'
    | 'openSideConversationDraft'
    | 'sendSideMessage'
    | 'interruptSide'
    | 'setSideInput'
    | 'setSideModel'
    | 'setSideReasoningEffort'
    | 'selectSideConversation'
    | 'openThreadInSidePanel'
    | 'setSidePanelOpen'
    | 'closeSideConversation'
    | 'discardSideConversation'
    | 'promoteSideConversation'
  > = {
    spawnSideConversation: async (seedText) => {
      const state = ctx.get()
      const parentId = state.activeThreadId
      if (!parentId) {
        ctx.set({ error: ctx.t('common:sideConversationNeedsActiveThread') })
        return null
      }
      if (state.runtimeConnection !== 'ready') {
        ctx.set({ error: ctx.t('common:runtimeActionNeedsConnection') })
        return null
      }
      const provider = ctx.getProvider()
      if (typeof provider.forkThread !== 'function') {
        ctx.set({ error: ctx.t('common:runtimeFeatureUnsupported') })
        return null
      }
      const parentThread = state.threads.find((thread) => thread.id === parentId)
      const title = defaultSideTitle(parentThread?.title ?? '', parentId)
      let forked
      try {
        forked = await provider.forkThread(parentId, { relation: 'side', title })
      } catch (e) {
        ctx.set({
          error: ctx.formatRuntimeError(e),
          ...(ctx.shouldOpenSettingsForError(e)
            ? { route: 'settings' as const, settingsSection: 'agents' as const }
            : {})
        })
        return null
      }
      const now = new Date().toISOString()
      const inheritedAt = new Date().toISOString()
      const side: SideConversation = {
        threadId: forked.id,
        parentThreadId: parentId,
        title: forked.title ?? title,
        createdAt: now,
        inheritedAt,
        blocks: [],
        liveAssistant: '',
        lastSeq: 0,
        input: '',
        model: defaultSideModel(state, parentId),
        reasoningEffort: 'max',
        busy: false,
        turnId: null,
        userItemId: null,
        error: null,
        lastTurnUsage: null
      }
      ctx.set((s) => ({
        sideConversations: { ...s.sideConversations, [forked.id]: side },
        sidePanel: setSidePanel(s.sidePanel, { open: true, activeSideId: forked.id })
      }))
      // Start a dedicated SSE subscription for this side thread. The
      // main `activeThreadId` and main subscription are untouched.
      startSideSubscription(forked.id, 0, ctx)
      if (seedText && seedText.trim()) {
        // Call the side action directly through the closure we are
        // currently building so store-level `state.sendSideMessage`
        // shims (e.g. test harnesses) cannot swallow the seed send.
        const started = await actions.sendSideMessage(forked.id, seedText.trim())
        if (!started) return forked.id
      }
      return forked.id
    },

    openSideConversationDraft: () => {
      ctx.set((s) => ({
        sidePanel: setSidePanel(s.sidePanel, { open: true, activeSideId: null })
      }))
    },

    sendSideMessage: async (sideId, text) => {
      const state = ctx.get()
      const side = state.sideConversations[sideId]
      if (!side) return false
      if (side.busy) return false
      const trimmed = text.trim()
      if (!trimmed) return false
      const provider = ctx.getProvider()
      const reasoningEffort = sideReasoningEffortRequestValue(side.reasoningEffort)
      const optimisticUserId = createSideClientUserMessageId()
      const provisionalTurnId = pendingActiveStreamTurnId(sideId, optimisticUserId)
      const createdAt = new Date().toISOString()
      const publicText = projectOrdinaryPublicText(trimmed)
      ctx.set((s) =>
        patchSide(s, sideId, (cur) => ({
          ...cur,
          input: '',
          busy: true,
          turnId: provisionalTurnId,
          userItemId: optimisticUserId,
          blocks: [
            ...cur.blocks,
            {
              kind: 'user',
              id: optimisticUserId,
              createdAt,
              text: publicText,
              meta: { turnId: provisionalTurnId }
            }
          ],
          error: null
        }))
      )
      try {
        const { turnId, userMessageItemId } = await provider.sendUserMessage(sideId, trimmed, {
          model: side.model,
          ...(reasoningEffort ? { reasoningEffort } : {})
        })
        const currentTurnId = ctx.get().sideConversations[sideId]?.turnId
        if (currentTurnId && currentTurnId !== turnId) {
          migrateActiveStreamTurn(sideId, currentTurnId, turnId)
        }
        ctx.set((s) =>
          patchSide(s, sideId, (cur) => {
            const runtimeUserId = userMessageItemId ?? optimisticUserId
            const reconciledBlocks = userMessageItemId && userMessageItemId !== optimisticUserId
              ? reconcileOptimisticUserBlock(cur.blocks, optimisticUserId, userMessageItemId, publicText)
              : cur.blocks
            const blocks = bindSideUserBlockTurn(reconciledBlocks, runtimeUserId, turnId)
            return {
              ...cur,
              blocks,
              input: '',
              busy: true,
              turnId,
              userItemId: userMessageItemId ?? cur.userItemId,
              error: null
            }
          })
        )
        // Re-attach the subscription from the latest seen seq so we don't
        // miss items emitted between the previous reconnect and the new
        // turn creation. The side snapshot captured before send can be
        // stale when the old subscription already delivered live deltas.
        startSideSubscription(sideId, latestSideSeq(sideId, ctx.get().sideConversations[sideId]?.lastSeq ?? side.lastSeq), ctx)
        return true
      } catch (e) {
        clearActiveStream(sideId, provisionalTurnId)
        ctx.set((s) =>
          patchSide(s, sideId, (cur) => {
            const stillOnOptimisticTurn = cur.turnId === provisionalTurnId || cur.userItemId === optimisticUserId
            return {
              ...cur,
              input: stillOnOptimisticTurn && !cur.input.trim() && publicText === trimmed
                ? trimmed
                : cur.input,
              blocks: cur.blocks.filter((block) => !(block.kind === 'user' && block.id === optimisticUserId)),
              busy: stillOnOptimisticTurn ? false : cur.busy,
              turnId: cur.turnId === provisionalTurnId ? null : cur.turnId,
              userItemId: cur.userItemId === optimisticUserId ? null : cur.userItemId,
              error: ctx.formatRuntimeError(e)
            }
          })
        )
        return false
      }
    },

    interruptSide: async (sideId) => {
      const state = ctx.get()
      const side = state.sideConversations[sideId]
      if (!side || !side.turnId) return
      const provider = ctx.getProvider()
      const interruptedTurnId = side.turnId
      teardownSideSubscription(sideId)
      clearActiveStream(sideId)
      ctx.set((current) =>
        patchSide(current, sideId, (active) => {
          if (!sideMatchesExpectedTerminalTurn(active, interruptedTurnId)) return active
          return {
            ...active,
            blocks: withoutProvisionalSideAssistantBlocks(
              active.blocks,
              interruptedTurnId,
              side.userItemId
            ),
            liveAssistant: '',
            busy: false
          }
        })
      )
      let terminalError: string | null = null
      try {
        await provider.interruptTurn(sideId, interruptedTurnId)
      } catch (e) {
        terminalError = ctx.formatRuntimeError(e)
      }
      await reconcileTerminalSideFromThreadDetail({
        sideId,
        turnId: interruptedTurnId,
        userBlockId: side.userItemId,
        provider,
        ctx,
        terminalError
      })
    },

    setSideInput: (sideId, text) => {
      ctx.set((s) => patchSide(s, sideId, (cur) => ({ ...cur, input: text })))
    },

    setSideModel: (sideId, model) => {
      ctx.set((s) => patchSide(s, sideId, (cur) => ({ ...cur, model })))
    },

    setSideReasoningEffort: (sideId, effort) => {
      const projected = projectModelReasoningEffortV1(effort)
      if (!projected) return
      ctx.set((s) => patchSide(s, sideId, (cur) => ({ ...cur, reasoningEffort: projected })))
    },

    selectSideConversation: (sideId) => {
      ctx.set((s) => {
        if (!s.sideConversations[sideId]) return {}
        return { sidePanel: setSidePanel(s.sidePanel, { activeSideId: sideId, open: true }) }
      })
    },

    openThreadInSidePanel: async (threadId, options = {}) => {
      const targetId = threadId.trim()
      if (!targetId) return false
      const state = ctx.get()
      const parentId = options.parentThreadId ?? state.activeThreadId
      if (!parentId) {
        ctx.set({ error: ctx.t('common:sideConversationNeedsActiveThread') })
        return false
      }
      if (state.runtimeConnection !== 'ready') {
        ctx.set({ error: ctx.t('common:runtimeActionNeedsConnection') })
        return false
      }
      const existing = state.sideConversations[targetId]
      if (existing) {
        ctx.set((s) => ({
          sidePanel: setSidePanel(s.sidePanel, { activeSideId: targetId, open: true })
        }))
        startSideSubscription(targetId, latestSideSeq(targetId, existing.lastSeq), ctx)
        return true
      }

      const provider = ctx.getProvider()
      const targetThread = state.threads.find((thread) => thread.id === targetId) ?? null
      let detail: Awaited<ReturnType<AgentProvider['getThreadDetail']>>
      try {
        detail = await provider.getThreadDetail(targetId)
      } catch (e) {
        ctx.set({
          error: ctx.formatRuntimeError(e),
          ...(ctx.shouldOpenSettingsForError(e)
            ? { route: 'settings' as const, settingsSection: 'agents' as const }
            : {})
        })
        return false
      }

      const now = new Date().toISOString()
      const blocks = hydrateBlockModelLabels(targetId, detail.blocks)
      const side: SideConversation = {
        threadId: targetId,
        parentThreadId: parentId,
        title: options.title ?? targetThread?.title ?? targetId.slice(0, 8),
        source: options.source ?? 'background-agent',
        createdAt: now,
        inheritedAt: now,
        blocks,
        liveAssistant: '',
        lastSeq: detail.latestSeq,
        input: '',
        model: targetThread?.model ?? defaultSideModel(state, parentId),
        reasoningEffort: 'max',
        busy: threadSnapshotLooksRunning(blocks, detail.threadStatus),
        turnId: detail.latestTurnId ?? null,
        userItemId: detail.latestUserMessageId ?? null,
        error: null,
        lastTurnUsage: null
      }
      ctx.set((s) => ({
        sideConversations: { ...s.sideConversations, [targetId]: side },
        sidePanel: setSidePanel(s.sidePanel, { open: true, activeSideId: targetId })
      }))
      startSideSubscription(targetId, detail.latestSeq, ctx)
      return true
    },

    setSidePanelOpen: (open) => {
      ctx.set((s) => ({ sidePanel: setSidePanel(s.sidePanel, { open }) }))
    },

    closeSideConversation: async (sideId) => {
      const state = ctx.get()
      const closingSide = state.sideConversations[sideId] ?? null
      teardownSideSubscription(sideId)
      clearActiveStream(sideId)
      sideLatestSeqByThread.delete(sideId)
      ctx.set((s) => {
        const next = { ...s.sideConversations }
        delete next[sideId]
        const nextActiveId =
          s.sidePanel.activeSideId === sideId && closingSide
            ? Object.values(next).find((side) => side.parentThreadId === closingSide.parentThreadId)?.threadId ?? null
            : s.sidePanel.activeSideId
        const nextPanel: SidePanelState = {
          open: nextActiveId ? s.sidePanel.open : false,
          activeSideId: nextActiveId
        }
        return { sideConversations: next, sidePanel: nextPanel }
      })
    },

    discardSideConversation: async (sideId) => {
      const state = ctx.get()
      const side = state.sideConversations[sideId]
      teardownSideSubscription(sideId)
      clearActiveStream(sideId)
      sideLatestSeqByThread.delete(sideId)
      ctx.set((s) => {
        const next = { ...s.sideConversations }
        delete next[sideId]
        const nextActiveId =
          s.sidePanel.activeSideId === sideId && side
            ? Object.values(next).find((candidate) => candidate.parentThreadId === side.parentThreadId)?.threadId ?? null
            : s.sidePanel.activeSideId
        const nextPanel: SidePanelState = {
          open: nextActiveId ? s.sidePanel.open : false,
          activeSideId: nextActiveId
        }
        return { sideConversations: next, sidePanel: nextPanel }
      })
      if (side) {
        const provider = ctx.getProvider()
        try {
          await provider.deleteThread(sideId)
        } catch (e) {
          ctx.set({
            error: ctx.formatRuntimeError(e),
            ...(ctx.shouldOpenSettingsForError(e)
              ? { route: 'settings' as const, settingsSection: 'agents' as const }
              : {})
          })
        }
      }
    },

    promoteSideConversation: async (sideId) => {
      const state = ctx.get()
      const side = state.sideConversations[sideId]
      if (!side) return
      const provider = ctx.getProvider()
      if (typeof provider.updateThreadRelation !== 'function') {
        ctx.set({ error: ctx.formatRuntimeError(new Error('promote side conversation is not supported')) })
        return
      }
      try {
        await provider.updateThreadRelation(sideId, 'primary')
      } catch (e) {
        ctx.set({ error: ctx.formatRuntimeError(e) })
        return
      }
      await ctx.get().refreshThreads()
      // Closing is a structural teardown; call directly so a stubbed
      // `state.closeSideConversation` (e.g. in tests) cannot swallow it.
      await actions.closeSideConversation(sideId)
    }
  }
  return actions
}

/**
 * Internal helper: tear down all side subscriptions. Used by the
 * `boot`/`unmount` path to avoid dangling SSE streams on app shutdown.
 */
export function teardownAllSideSubscriptions(): void {
  for (const ac of sideAbortControllers.values()) ac.abort()
  sideAbortControllers.clear()
  sideLatestSeqByThread.clear()
}
