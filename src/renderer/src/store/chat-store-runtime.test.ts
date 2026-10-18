import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
const generatedPreview = vi.hoisted(() => ({ open: vi.fn(async () => true) }))
vi.mock('../office/open-generated-artifact', () => ({ openGeneratedArtifact: generatedPreview.open }))
import type { AcceptedFinalProjectionBatch, ChatBlock, GeneralTerminalProjectionBatch, ThreadEventSink, ThreadGoal, ThreadGoalStateV1 } from '../agent/types'
import { dispatchAnalytixRuntimeEvent, dispatchAnalytixRuntimeEvents } from '../agent/analytix-mapper'
import { AnalytixRuntimeProvider } from '../agent/analytix-runtime'
import * as providerRegistry from '../agent/registry'
import * as sddTranscript from '../sdd/sdd-chat-transcript'
import { rendererRuntimeClient } from '../agent/runtime-client'
import type { ThreadTraceEventPayload } from '@shared/thread-trace'
import i18n from '../i18n'
import { isStrictPublicRuntimeSseIpcPayload, projectPublicRuntimeSseBlock } from '@shared/public-runtime-sse'
import { PublicRuntimeEventFilter } from '@shared/public-runtime-content'
import { formatRuntimeError } from '../lib/format-runtime-error'
import {
  isPublicProjectionRevoked,
  markPublicProjectionRevoked,
  resetPublicProjectionRevocationsForTests
} from '../lib/public-projection-revocation'
import {
  armBusyWatchdog,
  buildThreadEventSink,
  clearWatchedCompletionNotification,
  clearWatchedCompletionNotifications,
  clearPendingClawFeishuMirrors,
  completionNotificationDedupeKeyForWatchedThread,
  MAX_PENDING_CLAW_FEISHU_MIRRORS,
  MAX_WATCHED_COMPLETION_NOTIFICATIONS,
  rememberPendingClawFeishuMirror,
  takePendingClawFeishuMirror,
  syncTurnCompletionPoll,
  watchTurnCompletionNotification
} from './chat-store-runtime'
import {
  markThreadWorktree,
  readThreadWorktreeRegistry,
  saveThreadWorktreeRegistry
} from '../lib/thread-worktree-registry'
import { clearBusyWatchdog, resetBusyRecoveryAttempts, stopTurnCompletionPoll } from './chat-store-schedulers'
import { markDeletedUserInputDraftThread } from './chat-store-runtime-helpers'
import type { ChatState, ChatStoreSet, QueuedUserMessage } from './chat-store-types'
import { queuedPauseReason } from './chat-store-queue'
import {
  clearActiveStream,
  getActiveStreamSnapshot,
  getActiveStreamSnapshotFor,
  pendingActiveStreamTurnId,
  resetActiveStream,
  subscribeActiveStream
} from '../thread/streaming/active-stream-store'

function memoryStorage(): Storage {
  const store = new Map<string, string>()
  return {
    get length() {
      return store.size
    },
    clear: vi.fn(() => store.clear()),
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    key: vi.fn((index: number) => [...store.keys()][index] ?? null),
    removeItem: vi.fn((key: string) => {
      store.delete(key)
    }),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, value)
    })
  }
}

function makeSinkHarness(overrides: Partial<ChatState> = {}): {
  getState: () => ChatState
  set: ChatStoreSet
  get: () => ChatState
  setSnapshots: () => ChatState[]
} {
  let state = {
    activeThreadId: 'thread-current',
    blocks: [],
    liveAssistant: '',
    lastSeq: 0,
    usageRefreshKey: 0,
    busy: true,
    error: null,
    currentTurnId: 'turn-current',
    currentTurnUserId: 'user-current',
    turnStartedAtByUserId: { 'user-current': 1000 },
    turnDurationByUserId: {},
    watchTurnCompletion: {},
    unreadThreadIds: {},
    queuedMessages: [],
    queuedMessagePauses: {},
    threads: [],
    refreshThreads: vi.fn(async () => undefined),
    drainQueuedMessages: vi.fn(async () => undefined)
  } as unknown as ChatState
  state = { ...state, ...overrides }
  const snapshots: ChatState[] = []
  const get = (): ChatState => state
  const set: ChatStoreSet = (partial) => {
    const patch = typeof partial === 'function' ? partial(state) : partial
    state = { ...state, ...patch }
    snapshots.push(state)
  }
  return {
    getState: () => state,
    set,
    get,
    setSnapshots: () => snapshots
  }
}

function correlatedUserInputRequest(itemId: string, requestId: string): Parameters<ThreadEventSink['onUserInput']>[0] {
  const questionId = `${requestId}_1`
  return {
    itemId,
    requestId,
    questions: [{
      header: 'Mode',
      id: questionId,
      question: `Choose a mode for ${requestId}`,
      multiSelect: false,
      options: [{ id: `${questionId}_option_1`, label: 'Fast', description: 'Use the faster path' }]
    }]
  }
}

describe('actual conversation summary lifecycle', () => {
  it('preserves one completed summary and its facts when an older start is replayed', () => {
    const { set, get, getState } = makeSinkHarness({ busy: false })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    sink.onCompaction({ itemId: 'compaction', turnId: 'turn-compaction', summary: 'Compacting', status: 'running', auto: false })
    expect(getState().busy).toBe(true)
    sink.onCompaction({ itemId: 'compaction', turnId: 'turn-compaction', summary: 'Actual summary', summaryKind: 'conversation', detail: 'Actual summary', status: 'success', auto: false,
      meta: { shadowedItemCount: 7, replacedTokens: 234, tokenCountKind: 'estimated', tokenEstimatorVersion: 1 } })
    sink.onCompaction({ itemId: 'compaction', turnId: 'turn-compaction', summary: 'Compacting', status: 'running', auto: false })
    expect(getState().busy).toBe(false)
    const blocks = getState().blocks.filter(block => block.kind === 'compaction')
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({ id: 'compaction', status: 'success', detail: 'Actual summary', summaryKind: 'conversation', meta: { shadowedItemCount: 7, replacedTokens: 234 } })
  })
})

describe('live generated artifact preview', () => {
  const artifact = { artifactId: 'a'.repeat(64), kind: 'docx', contentHash: 'b'.repeat(64), byteSize: 1000, savedAt: '2026-09-15T01:00:00Z' }
  const event = { itemId: 'artifact-tool', turnId: 'turn-current', summary: 'generate_office_document', status: 'success' as const, meta: { toolName: 'generate_office_document', generatedArtifact: artifact } }

  it('opens a fresh live result once through its original main conversation', async () => {
    generatedPreview.open.mockClear()
    const { set, get } = makeSinkHarness({ workspaceRoot: '/workspace' })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    sink.onTool(event)
    await vi.dynamicImportSettled()
    expect(generatedPreview.open).toHaveBeenCalledWith(artifact.artifactId, { threadId: 'thread-current', workspace: '/workspace' })
    sink.onTool(event)
    await vi.dynamicImportSettled()
    expect(generatedPreview.open).toHaveBeenCalledOnce()
  })

  it('does not reopen an artifact already restored from conversation history', async () => {
    generatedPreview.open.mockClear()
    const { set, get } = makeSinkHarness({ workspaceRoot: '/workspace', blocks: [{ kind: 'tool', id: 'artifact-tool', summary: event.summary, status: 'success', meta: event.meta }] })
    buildThreadEventSink(set, get, { threadId: 'thread-current' }).onTool(event)
    await vi.dynamicImportSettled()
    expect(generatedPreview.open).not.toHaveBeenCalled()
  })
})

function acceptedFinalProjectionBatch(
  overrides: Partial<AcceptedFinalProjectionBatch> = {}
): AcceptedFinalProjectionBatch {
  const publicationCommitId = 'f'.repeat(64)
  return {
    batchId: '7'.repeat(64),
    threadId: 'thread-current',
    turnId: 'turn-current',
    publicationCommitId,
    firstSeq: 20,
    lastSeq: 22,
    receipt: {
      schemaVersion: 1,
      batchId: '7'.repeat(64),
      threadId: 'thread-current',
      turnId: 'turn-current',
      publicationCommitId,
      lastSeq: 22
    },
    assistant: {
      kind: 'assistant',
      id: 'item-final',
      createdAt: '2026-07-18T00:00:00Z',
      text: 'verified boundary answer',
      acceptedFinalView: {
        schemaVersion: 3,
        acceptedFinalDigest: publicationCommitId,
        publicationState: 'accepted',
        variant: 'GeneralGuidanceAnswer',
        terminalReason: 'success',
        blockerCode: '',
        coverageStatus: 'guidance_only',
        checkedScopeDigest: '',
        missingScopeCount: 0,
        claimCount: 0,
        claimTypes: [],
        receiptMetadata: {
          projection: 'masked_metadata_only', count: 0, setDigest: '8'.repeat(64), citations: []
        },
        noHitWording: '',
        acceptedAt: '2026-07-18T00:00:00Z'
      },
      meta: { turnId: 'turn-current' }
    },
    usage: {
      inputTokens: 10,
      outputTokens: 3,
      reasoningTokens: 0,
      cachedTokens: 5,
      cacheMissTokens: 5,
      cacheHitRate: 0.5,
      totalTokens: 13,
      costUsd: null,
      costCny: null,
      priceConfigured: false,
      tokenEconomySavingsTokens: 5,
      turns: 1
    },
    terminal: {
      status: 'completed',
      createdAt: '2026-07-18T00:00:00Z',
      acceptedFinalDigest: publicationCommitId,
      terminalReason: 'success'
    },
    ...overrides
  }
}

function generalTerminalProjectionBatch(
  overrides: Partial<GeneralTerminalProjectionBatch> = {}
): GeneralTerminalProjectionBatch {
  return {
    batchDigest: 'a'.repeat(64),
    threadId: 'thread-current',
    turnId: 'turn-current',
    firstSeq: 20,
    lastSeq: 22,
    terminalItem: {
      kind: 'assistant',
      id: 'item-general',
      createdAt: '2026-07-20T03:00:00Z',
      text: 'ordinary general guidance',
      meta: { turnId: 'turn-current' }
    },
    usage: {
      inputTokens: 2,
      outputTokens: 1,
      reasoningTokens: 0,
      cachedTokens: 0,
      cacheMissTokens: 0,
      cacheHitRate: null,
      totalTokens: 3,
      costUsd: 0,
      costCny: 0,
      priceConfigured: false,
      tokenEconomySavingsTokens: 0,
      turns: 1
    },
    terminal: {
      status: 'completed',
      createdAt: '2026-07-20T03:00:00Z',
      terminalReason: 'success'
    },
    ...overrides
  }
}

describe('thread event sink binding', () => {
  afterEach(() => {
    clearActiveStream()
    resetPublicProjectionRevocationsForTests()
  })

  it.each(['disarmed', 'clear'] as const)('consumes %s before a pending Goal read and never restores old armed state after failure', async stop => {
    const goal: ThreadGoal = { threadId: 'thread-current', id: 'goal-current', revision: 2, objective: 'Synthetic objective', status: 'active', maxGoalRounds: 8, roundsStarted: 1, tokenBudget: null, tokensUsed: 0, timeUsedSeconds: 0, createdAt: '2026-10-11T00:00:00Z', updatedAt: '2026-10-11T00:00:00Z' }
    const armed = { id: 'goal-current', revision: 2, activation: 'armed' as const }
    let resolve!: (state: ThreadGoalStateV1) => void
    const read = vi.spyOn(AnalytixRuntimeProvider.prototype, 'getThreadGoalState')
      .mockImplementationOnce(() => new Promise(done => { resolve = done }))
      .mockRejectedValueOnce(new Error('Synthetic authority read unavailable'))
    const detail = vi.spyOn(AnalytixRuntimeProvider.prototype, 'getThreadDetail').mockRejectedValue(new Error('Unrelated detail unavailable'))
    try {
      const h = makeSinkHarness({ activeThreadGoal: goal, activeGoalActivation: armed, goalStateEpoch: 0 })
      const sink = buildThreadEventSink(h.set, h.get, { threadId: 'thread-current' })
      expect(sink.onGoalActivation?.({ threadId: 'thread-current', activation: armed })).toBeUndefined()
      expect(read).toHaveBeenCalledTimes(1)
      const stopped = stop === 'clear' ? null : { ...armed, activation: 'disarmed' as const }
      expect(sink.onGoalActivation?.({ threadId: 'thread-current', activation: stopped })).toBeUndefined()
      expect(read).toHaveBeenCalledTimes(2)
      expect(h.getState().activeGoalActivation).toEqual(stopped)
      resolve({ goal, activation: armed, historyBlocks: [] })
      for (let n = 0; n < 8; n++) await Promise.resolve()
      expect(h.getState().activeGoalActivation).toEqual(stopped)
      expect(h.getState().goalStateEpoch).toBe(2)
      expect(detail).not.toHaveBeenCalled()
    } finally { read.mockRestore(); detail.mockRestore() }
  })

  it('ignores assistant deltas from a stream bound to a different active thread', () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-new' })
    const controller = new AbortController()
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-old',
      signal: controller.signal
    })

    sink.onDeltas([{ kind: 'agent_message', text: 'old answer', seq: 7 }])

    expect('liveReasoning' in getState()).toBe(false)
    expect(getState().lastSeq).toBe(0)
  })

  it('ignores queued callbacks after a stream has been aborted', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
    })
    const controller = new AbortController()
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      signal: controller.signal
    })

    controller.abort()
    sink.onDeltas([{ kind: 'agent_message', text: 'late old answer', seq: 8 }])
    sink.onTurnComplete()

    expect('liveReasoning' in getState()).toBe(false)
    expect(getState().blocks).toEqual([])
    expect(getState().busy).toBe(true)
  })

  it.each([false, true])('records terminal commit before a separate next-frame sample (accepted final: %s)', async (acceptedFinal) => {
    const frames: FrameRequestCallback[] = []
    const traces: ThreadTraceEventPayload[] = []
    const storage = memoryStorage()
    storage.setItem('ANALYTIX_THREAD_TRACE', '1')
    vi.stubGlobal('window', {
      localStorage: storage,
      requestAnimationFrame: (callback: FrameRequestCallback) => { frames.push(callback); return frames.length },
      analytix: { diagnostics: { recordThreadTrace: async (event: ThreadTraceEventPayload) => { traces.push(event) } } }
    })
    try {
      const { set, get } = makeSinkHarness({ lastSeq: 19 })
      const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
      if (acceptedFinal) await sink.onAcceptedFinalBatch?.(acceptedFinalProjectionBatch())
      else await sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch())
      const terminalTraces = (): ThreadTraceEventPayload[] => traces.filter((event) => event.name.startsWith('thread.terminal.'))
      expect(terminalTraces().map((event) => event.name)).toEqual(['thread.terminal.renderer_committed'])
      for (const frame of frames) frame(123)
      expect(terminalTraces().map((event) => event.name)).toEqual([
        'thread.terminal.renderer_committed', 'thread.terminal.next_frame'
      ])
      expect(terminalTraces()[0]?.data).toEqual({
        lastSeq: 22, acceptedFinal,
        monotonicMs: expect.any(Number), timeOrigin: performance.timeOrigin
      })
      expect(terminalTraces()[0]?.data?.monotonicMs).toBeGreaterThanOrEqual(0)
      expect(JSON.stringify(terminalTraces())).not.toContain('ordinary general guidance')
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('applies accepted final, usage, terminal state, and cursor in one store commit', async () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-current', text: 'case question', meta: { turnId: 'turn-current' } },
      {
        kind: 'tool', id: 'tool-current', summary: 'running', status: 'running',
        toolKind: 'tool_call', meta: { turnId: 'turn-current' }
      },
      { kind: 'assistant', id: 'draft-current', text: 'untrusted draft', meta: { turnId: 'turn-current' } }
    ]
    const { getState, set, get, setSnapshots } = makeSinkHarness({
      blocks,
      liveAssistant: 'untrusted stream suffix',
      lastSeq: 19,
      watchTurnCompletion: { 'thread-current': true },
      unreadThreadIds: { 'thread-current': true }
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    const batch = acceptedFinalProjectionBatch()
    const receipt = await sink.onAcceptedFinalBatch?.(batch)

    expect(setSnapshots()).toHaveLength(1)
    expect(receipt).toEqual(batch.receipt)
    const committed = getState()
    expect(committed.blocks.map((block) => block.id)).toEqual([
      'user-current', 'tool-current', 'item-final'
    ])
    expect(committed.blocks.find((block) => block.id === 'item-final')).toMatchObject({
      acceptedFinalProjectionReceipt: batch.receipt
    })
    expect(committed.blocks.find((block) => block.id === 'tool-current')).toMatchObject({ status: 'error' })
    expect(JSON.stringify(committed.blocks)).not.toContain('untrusted draft')
    expect(committed.liveAssistant).toBe('')
    expect(committed.lastSeq).toBe(22)
    expect(committed.lastTurnUsage).toMatchObject({
      threadId: 'thread-current',
      snapshot: { totalTokens: 13 }
    })
    expect(committed.turnUsageById['turn-current']).toMatchObject({ threadId: batch.threadId, turnId: batch.turnId,
      firstSeq: batch.firstSeq, lastSeq: batch.lastSeq, snapshot: batch.usage })
    expect(committed.busy).toBe(false)
    expect(committed.currentTurnId).toBeNull()
    expect(committed.currentTurnUserId).toBeNull()
    expect(committed.watchTurnCompletion).toEqual({})
    expect(committed.unreadThreadIds).toEqual({})
  })

  it('applies a non-authoritative general final, usage, terminal state, and cursor in one store commit', async () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-current', text: 'general question', meta: { turnId: 'turn-current' } },
      {
        kind: 'tool', id: 'tool-current', summary: 'running', status: 'running',
        toolKind: 'tool_call', meta: { turnId: 'turn-current' }
      },
      { kind: 'assistant', id: 'draft-current', text: 'untrusted draft', meta: { turnId: 'turn-current' } }
    ]
    const { getState, set, get, setSnapshots } = makeSinkHarness({
      blocks,
      liveAssistant: 'untrusted stream suffix',
      lastSeq: 19,
      watchTurnCompletion: { 'thread-current': true },
      unreadThreadIds: { 'thread-current': true }
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const batch = generalTerminalProjectionBatch()

    await sink.onGeneralTerminalBatch?.(batch)

    expect(setSnapshots()).toHaveLength(1)
    const committed = getState()
    expect(committed.blocks.map((block) => block.id)).toEqual([
      'user-current', 'tool-current', 'item-general'
    ])
    expect(committed.blocks.find((block) => block.id === 'item-general')).toMatchObject({
      kind: 'assistant',
      text: 'ordinary general guidance',
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: batch.batchDigest,
        threadId: batch.threadId, turnId: batch.turnId, itemId: 'item-general', firstSeq: 20, lastSeq: 22, status: 'completed' }
    })
    expect(JSON.stringify(committed.blocks)).not.toContain('acceptedFinal')
    expect(JSON.stringify(committed.blocks)).not.toContain('untrusted draft')
    expect(committed.blocks.find((block) => block.id === 'tool-current')).toMatchObject({ status: 'error' })
    expect(committed.liveAssistant).toBe('')
    expect(committed.lastSeq).toBe(22)
    expect(committed.lastTurnUsage).toMatchObject({
      threadId: 'thread-current',
      snapshot: { totalTokens: 3 }
    })
    expect(committed.turnUsageById['turn-current']).toMatchObject({ threadId: batch.threadId, turnId: batch.turnId,
      firstSeq: batch.firstSeq, lastSeq: batch.lastSeq, snapshot: batch.usage })
    expect(committed).toMatchObject({
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null,
      watchTurnCompletion: {},
      unreadThreadIds: {}
    })
  })

  it('removes usage and retires a rewound active turn even without matching visible blocks', async () => {
    const batch = generalTerminalProjectionBatch(), entry = { threadId: batch.threadId, turnId: batch.turnId,
      firstSeq: batch.firstSeq, lastSeq: batch.lastSeq, snapshot: batch.usage }
    const kept = { ...entry, turnId: 'older', firstSeq: 1, lastSeq: 3 }
    const { getState, set, get } = makeSinkHarness({ blocks: [], lastSeq: 22,
      turnUsageById: { 'turn-current': entry, older: kept } })
    resetActiveStream(batch.threadId, { turnId: batch.turnId, liveAssistant: 'removed private buffer', lastSeq: 22 })
    const sink = buildThreadEventSink(set, get, { threadId: batch.threadId })
    sink.onThreadRewound?.({ threadId: batch.threadId, turnId: batch.turnId, removedTurnIds: [batch.turnId],
      removedTurns: 1, remainingTurns: 1, seq: 23 })
    expect(getState().turnUsageById).toEqual({ older: kept })
    expect(getState()).toMatchObject({ lastSeq: 23, busy: false, currentTurnId: null, rewoundTurnIds: { 'turn-current': true } })
    expect(getActiveStreamSnapshot().threadId).toBeNull()
    sink.onTurnStarted?.({ turnId: batch.turnId, seq: 24 })
    expect(getState().busy).toBe(false)
    await expect(sink.onGeneralTerminalBatch?.(batch)).rejects.toThrow(/rewound/)
    sink.onTurnStarted?.({ turnId: 'new-turn', seq: 24 })
    expect(getState().currentTurnId).toBe('new-turn')
  })

  it.each([false, true])('does not revive a committed terminal item from old starts or late deltas (accepted: %s)', async accepted => {
    const { getState, set, get, setSnapshots } = makeSinkHarness({ lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const batch = generalTerminalProjectionBatch()
    if (accepted) await sink.onAcceptedFinalBatch?.(acceptedFinalProjectionBatch())
    else await sink.onGeneralTerminalBatch?.(batch)
    const committed = getState().blocks, writes = setSnapshots().length
    for (const start of [{ turnId: batch.turnId, seq: 23 }, { turnId: 'old-different-turn', seq: 22 },
      { turnId: 'unbound-cursor' }, { turnId: '', seq: 24 }]) sink.onTurnStarted?.(start)
    sink.onDeltas([{ kind: 'agent_message', text: 'LATE_UNPUBLISHED_TEXT', seq: 23 }])
    await expect(sink.onGeneralTerminalBatch?.(batch)).rejects.toThrow('could not be committed atomically')
    expect(getState().blocks).toBe(committed)
    expect(setSnapshots()).toHaveLength(writes)
    expect(getState().blocks.filter(block => block.id === (accepted ? 'item-final' : 'item-general'))).toHaveLength(1)
    expect(getState().lastSeq).toBe(22)
    expect(getState().liveAssistant).toBe('')
    expect(JSON.stringify(getState().blocks)).not.toContain('LATE_UNPUBLISHED_TEXT')
    expect(getState().busy).toBe(false)
    sink.onTurnStarted?.({ turnId: 'next-real-turn', seq: 24 })
    expect(getState()).toMatchObject({ busy: true, currentTurnId: 'next-real-turn' })
  })

  it.each(['same', 'text', 'turn', 'cursor', 'time'] as const)(
    'preserves ordinary atomic proof only for a matching fresh snapshot (%s)', async difference => {
      const batch = generalTerminalProjectionBatch()
      const item = batch.terminalItem as Extract<ChatBlock, { kind: 'assistant' }>
      const incoming = { ...item,
        ...(difference === 'text' ? { text: 'CHANGED_UNAUTHORIZED_PUBLIC_ITEM' } : {}),
        ...(difference === 'turn' ? { meta: { turnId: 'other-turn' } } : {}),
        ...(difference === 'time' ? { createdAt: '2026-07-20T04:00:00Z' } : {}) }
      const getThreadDetail = vi.fn(async () => ({ blocks: [incoming], latestSeq: difference === 'cursor' ? 21 : 22,
        threadStatus: 'idle', latestTurnId: batch.turnId }))
      const { getState, set, get } = makeSinkHarness({ lastSeq: 19 })
      const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', getThreadDetail })
      await sink.onGeneralTerminalBatch?.(batch)
      const receipt = (getState().blocks.at(-1) as typeof item).generalTerminalProjectionReceipt
      incoming.generalTerminalProjectionReceipt = { ...receipt!, turnId: 'FORGED_HISTORY_CONTROL' }
      await sink.onSnapshotRequired?.({ threadId: batch.threadId, seq: 22, highestSeq: 22, replayEventCount: 1 })
      const refreshed = getState().blocks.at(-1) as typeof item
      expect(refreshed.text).toBe(difference === 'cursor' ? item.text : incoming.text)
      expect(refreshed.generalTerminalProjectionReceipt).toEqual(difference === 'same' || difference === 'cursor' ? receipt : undefined)
    })

  it('rejects a non-contiguous general terminal batch without exposing its assistant text', async () => {
    const { getState, set, get } = makeSinkHarness({
      lastSeq: 18,
      blocks: [
        { kind: 'assistant', id: 'draft-current', text: 'UNTRUSTED_GENERAL_DRAFT', meta: { turnId: 'turn-current' } }
      ]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await expect(sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch()))
      .rejects.toThrow('could not be committed atomically')
    expect(getState().lastSeq).toBe(18)
    expect(JSON.stringify(getState().blocks)).not.toContain('UNTRUSTED_GENERAL_DRAFT')
    expect(JSON.stringify(getState().blocks)).not.toContain('ordinary general guidance')
    expect(getState()).toMatchObject({
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null,
      error: i18n.t('common:runtimeFinalSnapshotUnavailable')
    })
  })

  it('replays only an exact atomically committed accepted-final receipt without another store write', async () => {
    const { getState, set, get, setSnapshots } = makeSinkHarness({ lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const batch = acceptedFinalProjectionBatch()

    const firstReceipt = await sink.onAcceptedFinalBatch?.(batch)
    const writesAfterCommit = setSnapshots().length
    resetActiveStream('thread-current', {
      turnId: pendingActiveStreamTurnId('thread-current', 'leftover'),
      liveAssistant: 'UNTRUSTED_LEFTOVER',
      lastSeq: batch.lastSeq
    })
    const replayReceipt = await sink.onAcceptedFinalBatch?.(batch)

    expect(firstReceipt).toEqual(batch.receipt)
    expect(replayReceipt).toEqual(batch.receipt)
    expect(setSnapshots()).toHaveLength(writesAfterCommit)
    expect(getState().blocks.filter((block) => block.id === batch.assistant.id)).toHaveLength(1)
    expect(getState().lastSeq).toBe(batch.lastSeq)
    expect(getActiveStreamSnapshot().threadId).toBeNull()
  })

  it('rejects a torn or mismatched accepted-final receipt instead of repairing it', async () => {
    const original = acceptedFinalProjectionBatch()
    const tornAssistant = {
      ...original.assistant,
      acceptedFinalProjectionReceipt: original.receipt
    }
    const { getState, set, get } = makeSinkHarness({
      blocks: [tornAssistant],
      lastSeq: original.firstSeq - 1
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await expect(sink.onAcceptedFinalBatch?.(original))
      .rejects.toThrow('could not be committed atomically')
    expect(getState().lastSeq).toBe(original.firstSeq - 1)
    expect(getState().blocks).toEqual([])
    expect(getState()).toMatchObject({
      busy: false,
      liveAssistant: '',
      currentTurnId: null,
      currentTurnUserId: null,
      error: i18n.t('common:runtimeFinalSnapshotUnavailable')
    })

    const mismatched = {
      ...original,
      batchId: '8'.repeat(64),
      receipt: { ...original.receipt, batchId: '8'.repeat(64) }
    }
    await expect(sink.onAcceptedFinalBatch?.(mismatched))
      .rejects.toThrow('could not be committed atomically')
    expect(getState().lastSeq).toBe(original.firstSeq - 1)
  })

  it('requires the exact turn id and contiguous predecessor before committing accepted-final', async () => {
    const pending = pendingActiveStreamTurnId('thread-current', 'user-current')
    const pendingHarness = makeSinkHarness({ currentTurnId: pending, lastSeq: 19 })
    const pendingSink = buildThreadEventSink(pendingHarness.set, pendingHarness.get, {
      threadId: 'thread-current'
    })
    await expect(pendingSink.onAcceptedFinalBatch?.(acceptedFinalProjectionBatch()))
      .rejects.toThrow('could not be committed atomically')
    expect(pendingHarness.getState().lastSeq).toBe(19)

    for (const cursor of [18, 20, 23]) {
      const harness = makeSinkHarness({ lastSeq: cursor })
      const sink = buildThreadEventSink(harness.set, harness.get, {
        threadId: 'thread-current'
      })
      await expect(sink.onAcceptedFinalBatch?.(acceptedFinalProjectionBatch()))
        .rejects.toThrow('could not be committed atomically')
      expect(harness.getState().lastSeq).toBe(cursor)
    }
  })

  it('quarantines same-turn drafts and active stream state when accepted-final continuity is invalid', async () => {
    const batch = acceptedFinalProjectionBatch()
    const { getState, set, get } = makeSinkHarness({
      lastSeq: batch.firstSeq,
      liveAssistant: 'UNTRUSTED_LIVE_CASE_FACT',
      blocks: [
        { kind: 'user', id: 'user-current', text: 'case question', meta: { turnId: batch.turnId } },
        { kind: 'assistant', id: 'draft-current', text: 'UNTRUSTED_DRAFT_CASE_FACT', meta: { turnId: batch.turnId } }
      ]
    })
    resetActiveStream(batch.threadId, {
      turnId: batch.turnId,
      liveAssistant: 'UNTRUSTED_ACTIVE_STREAM_CASE_FACT',
      lastSeq: batch.firstSeq
    })
    const sink = buildThreadEventSink(set, get, { threadId: batch.threadId })

    await expect(sink.onAcceptedFinalBatch?.(batch))
      .rejects.toThrow('could not be committed atomically')

    expect(getState().lastSeq).toBe(batch.firstSeq)
    expect(JSON.stringify(getState().blocks)).not.toContain('UNTRUSTED_DRAFT_CASE_FACT')
    expect(getState()).toMatchObject({
      liveAssistant: '',
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null
    })
    expect(getActiveStreamSnapshot().threadId).toBeNull()
  })

  it('rejects an accepted-final batch for another turn without advancing the cursor', async () => {
    const { getState, set, get } = makeSinkHarness({ lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const foreign = acceptedFinalProjectionBatch()
    foreign.turnId = 'turn-foreign'
    foreign.receipt = { ...foreign.receipt, turnId: 'turn-foreign' }
    foreign.assistant = {
      ...foreign.assistant,
      meta: { ...foreign.assistant.meta, turnId: 'turn-foreign' }
    }

    await expect(sink.onAcceptedFinalBatch?.(foreign))
      .rejects.toThrow('could not be committed atomically')
    expect(getState().lastSeq).toBe(19)
    expect(getState().busy).toBe(true)
    expect(getState().blocks).toEqual([])
  })

  it('purges a revoked case projection even after an ordinary terminal event', async () => {
    clearPendingClawFeishuMirrors()
    rememberPendingClawFeishuMirror('turn-revoked', {
      threadId: 'thread-current',
      userBlockId: 'user-current',
      userText: 'PRIVATE_CASE_PROMPT'
    })
    rememberPendingClawFeishuMirror('turn-safe', {
      threadId: 'thread-safe',
      userBlockId: 'user-safe',
      userText: 'safe prompt'
    })
    const caseThread = {
      id: 'thread-current',
      title: 'PRIVATE_CASE_TITLE',
      updatedAt: '2026-07-14T00:00:00.000Z',
      model: 'deepseek-chat',
      mode: 'agent',
      historyAuthority: 'case_boundary_only_v1' as const
    }
    const derivedThread = {
      ...caseThread,
      id: 'thread-derived',
      parentThreadId: 'thread-current'
    }
    const safeThread = {
      id: 'thread-safe',
      title: 'safe',
      updatedAt: '2026-07-14T00:00:00.000Z',
      model: 'deepseek-chat',
      mode: 'agent'
    }
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      activeThreadGoal: { objective: 'PRIVATE_GOAL' } as any,
      activeThreadTodos: { items: [{ content: 'PRIVATE_TODO' }] } as any,
      blocks: [{ kind: 'assistant', id: 'assistant-private', text: 'PRIVATE_CASE_FACT' } as any],
      liveAssistant: 'PRIVATE_CASE_DELTA',
      queuedMessages: [{ threadId: 'thread-current', id: 'retired-queue', text: 'PRIVATE_QUEUE', sending: true },
        { threadId: 'thread-derived', id: 'derived-queue', text: 'PRIVATE_DERIVED_QUEUE' },
        { threadId: 'thread-cache-derived', id: 'cache-queue', text: 'PRIVATE_CACHE_QUEUE' },
        { threadId: 'thread-safe', id: 'safe-queue', text: 'retain safe queue' }],
      queuedMessagePauses: { 'thread-current': 'failed', 'thread-derived': 'interrupted', 'thread-cache-derived': 'failed', 'thread-safe': 'failed' },
      threads: [caseThread, derivedThread, safeThread],
      caseProjectThreadsById: { case_one: [caseThread, derivedThread, { ...derivedThread, id: 'thread-cache-derived' }, safeThread] },
      sideConversations: {
        'thread-derived': {
          threadId: 'thread-derived',
          parentThreadId: 'thread-current',
          blocks: [{ kind: 'assistant', id: 'side-private', text: 'PRIVATE_SIDE_FACT' }]
        }
      } as any,
      sidePanel: { open: true, activeSideId: 'thread-derived' },
      threadHandoffOperations: [{
        id: 'handoff-private',
        sourceThreadId: 'thread-current',
        targetThreadId: 'thread-derived'
      }] as any,
      activeThreadHandoffOperationId: 'handoff-private',
      watchTurnCompletion: { 'thread-current': true, 'thread-safe': true },
      unreadThreadIds: { 'thread-current': true, 'thread-safe': true },
      lastTurnUsage: { threadId: 'thread-current', snapshot: {} as any },
      topNotice: { id: 'private', tone: 'info', message: 'PRIVATE_NOTICE' },
      runtimeErrorDetail: 'PRIVATE_RUNTIME_DETAIL'
    })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      getThreadDetail: vi.fn(async () => ({
        blocks: [],
        latestSeq: 1,
        latestTurnId: 'turn-current',
        threadStatus: 'completed'
      }))
    })

    await sink.onTurnComplete({ threadId: 'thread-current', turnId: 'turn-current', seq: 1 })
    await sink.onPublicProjectionRevoked?.({
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId: 'thread-current',
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected',
      action: 'purge_case_projection',
      terminal: true
    })

    expect(isPublicProjectionRevoked('thread-current')).toBe(true)
    expect(isPublicProjectionRevoked('thread-derived')).toBe(true)
    expect(getState().activeThreadId).toBeNull()
    expect(getState().blocks).toEqual([])
    expect(getState().liveAssistant).toBe('')
    expect(getState().threads).toEqual([safeThread])
    expect(getState().queuedMessages).toEqual([{ threadId: 'thread-safe', id: 'safe-queue', text: 'retain safe queue' }])
    expect(getState().queuedMessagePauses).toEqual({ 'thread-safe': 'failed' })
    expect(getState().caseProjectThreadsById.case_one).toEqual([safeThread])
    expect(getState().sideConversations).toEqual({})
    expect(getState().sidePanel.activeSideId).toBeNull()
    expect(getState().threadHandoffOperations).toEqual([])
    expect(getState().watchTurnCompletion).toEqual({ 'thread-safe': true })
    expect(getState().unreadThreadIds).toEqual({ 'thread-safe': true })
    expect(getState().lastTurnUsage).toBeNull()
    expect(getState().topNotice).toBeNull()
    expect(getState().runtimeErrorDetail).toBeNull()
    expect(getState().error).toBe(i18n.t('common:runtimeFinalSnapshotUnavailable'))
    expect(takePendingClawFeishuMirror('turn-revoked')).toBeUndefined()
    expect(takePendingClawFeishuMirror('turn-safe')).toMatchObject({ threadId: 'thread-safe' })
  })

  it('releases a thread-owned worktree only after trusted terminal reconciliation', async () => {
    const releaseWorktree = vi.fn(async () => undefined)
    const localStorage = memoryStorage()
    vi.stubGlobal('window', {
      localStorage,
      analytix: {
        workspace: { releaseWorktree }
      }
    })
    saveThreadWorktreeRegistry(
      markThreadWorktree('thread-current', {
        projectPath: '/workspace/analytix',
        poolIndex: 1,
        worktreePath: '/workspace/analytix-worktrees/pool-1',
        branch: 'analytix-pool-1'
      }),
      localStorage
    )
    const refreshThreads = vi.fn(async () => undefined)
    const drainQueuedMessages = vi.fn(async () => undefined)
    const { set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: true,
      refreshThreads,
      drainQueuedMessages
    })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      getThreadDetail: vi.fn(async () => ({
        blocks: [],
        latestSeq: 1,
        latestTurnId: 'turn-current',
        threadStatus: 'completed'
      }))
    })

    await sink.onTurnComplete()

    expect(releaseWorktree).toHaveBeenCalledWith({
      projectPath: '/workspace/analytix',
      poolIndex: 1
    })
    expect(readThreadWorktreeRegistry(localStorage).worktrees).toEqual({})
  })

	  it('advances the cursor without retaining assistant draft deltas', () => {
	    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
	    const controller = new AbortController()
	    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      signal: controller.signal
    })

    sink.onDeltas([{ kind: 'agent_message', text: 'fresh answer', seq: 9 }])

	    expect('liveReasoning' in getState()).toBe(false)
	    expect(getState().liveAssistant).toBe('')
	    expect(getState().lastSeq).toBe(9)
	  })

	  it('does not mark the parent composer busy for child-only runtime status after completion', () => {
	    const { getState, set, get } = makeSinkHarness({
	      activeThreadId: 'thread-current',
	      busy: false,
	      currentTurnId: 'turn-completed',
	      blocks: []
	    })
	    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

	    sink.onRuntimeStatus?.({
	      kind: 'pipeline_stage',
	      itemId: 'runtime_status_child_completed',
	      turnId: 'turn-parent',
	      message: '',
	      child: {
	        childRunId: 'job-1',
	        childThreadId: 'thread-child',
	        parentThreadId: 'thread-current',
	        childStatus: 'completed'
	      }
	    } as any)

	    expect(getState().busy).toBe(false)
	    expect(getState().currentTurnId).toBe('turn-completed')
	    expect(getState().blocks).toEqual([])
	  })

	  it('reconciles pending steering user blocks through turn_steered and item_created user messages', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [{
        kind: 'user',
        id: 'client_steer',
        text: 'add a constraint',
        meta: {
          turnId: 'turn-current',
          clientUserMessageId: 'client_steer',
          steeringStatus: 'pending'
        }
      }]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onTurnSteered?.({
      turnId: 'turn-current',
      clientUserMessageId: 'client_steer',
      admittedSeq: 7
    })

    expect(getState().blocks[0]).toMatchObject({
      id: 'client_steer',
      meta: {
        steeringStatus: 'admitted',
        admittedSeq: 7
      }
    })

    sink.onUserMessage({
      itemId: 'item_promoted_steer',
      turnId: 'turn-current',
      text: 'add a constraint',
      createdAt: '2026-07-01T00:00:00.000Z',
      meta: {
        clientUserMessageId: 'client_steer',
        steeringStatus: 'accepted'
      }
    })

    expect(getState().blocks).toEqual([
      expect.objectContaining({
        kind: 'user',
        id: 'item_promoted_steer',
        text: 'add a constraint',
        meta: expect.objectContaining({
          clientUserMessageId: 'client_steer',
          steeringStatus: 'accepted'
        })
      })
    ])
    expect(getState().queuedMessages).toEqual([])
  })

  describe('promoted user message identity', () => {
    // Reduced from the c47 native synthetic sequence: items 4/38/39, admissions 30/31.
    const threadId = 'thr_durable_5'
    const turnId = 'turn_5'
    const originalId = 'item_turn_5_user'
    const questionClient = '79908121-f23c-4297-bc51-b53946822cb5'
    const ordinaryClient = 'c93cd3bf-f2a3-413f-be12-dd42ab20d5e4'
    const questionItem = 'item_steer_a86cb36396aac3a0c939f07da0ee55c83cb89e10c5a85b73d3a5c62f8b3c5244'
    const ordinaryItem = 'item_steer_5fcd884c720e19cc01a7104146f33e5279510e6d7ba8b6a5ad0679500079b06e'
    const questionReply = JSON.stringify({
      kind: 'question_reply_content',
      questions: [{ header: 'BUSY', question: 'Choose Alpha and Beta, then answer later using the synthetic custom answer.',
        options: [{ label: 'Alpha', description: 'First synthetic branch.' }, { label: 'Beta', description: 'Second synthetic branch.' }], multiSelect: true }],
      answers: [{ selected: ['Alpha', 'Beta'], custom: 'QA_NATIVE_V2_BUSY_ACCEPT' }]
    })
    const ordinaryText = 'Keep the final synthetic response brief.'
    const questionTool: ChatBlock = {
      kind: 'tool', id: 'item_tool_host_v1_0ccba29a8b0cb166a38b305018a7fb98a076efa3b3e2bc32832c5bf43f7af34a', summary: 'request_user_input', status: 'success',
      meta: { turnId, toolName: 'request_user_input', toolCallIdentity: { threadId, turnId,
        callId: 'call_host_a6d6661f7f284d8ccbafc168e539b75bc0a6c16e409e6aca715d14be985517d3', itemId: 'item_tool_host_v1_0ccba29a8b0cb166a38b305018a7fb98a076efa3b3e2bc32832c5bf43f7af34a' } }
    }
    const questionCard: ChatBlock = { kind: 'user_input', id: 'item_input_be9075e1e243f5304e7617fe7d1e0e1c117968a28c7d49fb5c530fda42e17e25', requestId: 'input_be9075e1e243f5304e7617fe7d1e0e1c117968a28c7d49fb5c530fda42e17e25',
      questions: [{ id: 'input_be9075e1e243f5304e7617fe7d1e0e1c117968a28c7d49fb5c530fda42e17e25_1', header: 'BUSY', question: 'Choose Alpha and Beta.', options: [] }],
      status: 'continued', meta: { turnId } }
    const original = (): ChatBlock => ({ kind: 'user', id: originalId, text: 'QA_NATIVE_V2_BUSY', meta: { turnId, activeSkillIds: [] } })
    const pending = (which: 'question' | 'ordinary'): ChatBlock => ({
      kind: 'user', id: which === 'question' ? questionClient : ordinaryClient,
      text: which === 'question' ? questionReply : ordinaryText,
      meta: { turnId, clientUserMessageId: which === 'question' ? questionClient : ordinaryClient, steeringStatus: 'pending' }
    })
    const event = (which: 'original' | 'question' | 'ordinary', seq = which === 'original' ? 4 : which === 'question' ? 38 : 39): Parameters<typeof dispatchAnalytixRuntimeEvent>[0] => ({
      kind: 'item_created', threadId, turnId, seq,
      timestamp: '2026-10-10T19:35:49.224747Z',
      item: { id: which === 'original' ? originalId : which === 'question' ? questionItem : ordinaryItem,
        threadId, turnId, kind: 'user_message', role: 'user', status: 'completed',
        text: which === 'original' ? 'QA_NATIVE_V2_BUSY' : which === 'question' ? questionReply : ordinaryText,
        createdAt: '2026-10-10T19:34:47.584944Z',
        ...(which !== 'original' ? { delivery: 'steer', clientUserMessageId: which === 'question' ? questionClient : ordinaryClient } : {}) }
    })
    const deliver = (sink: ThreadEventSink, which: 'original' | 'question' | 'ordinary', seq?: number): Promise<void> =>
      dispatchAnalytixRuntimeEvent(event(which, seq), sink, async () => undefined)
    beforeEach(() => vi.useFakeTimers())
    afterEach(() => { clearBusyWatchdog(); clearActiveStream(); vi.useRealTimers() })

    it('preserves the native original question through both promotions, replay and late admission', async () => {
      const { getState, set, get } = makeSinkHarness({ activeThreadId: threadId, currentTurnId: turnId,
        currentTurnUserId: 'initial-client', blocks: [{ kind: 'user', id: 'initial-client', text: 'QA_NATIVE_V2_BUSY', meta: { turnId } }] })
      const sink = buildThreadEventSink(set, get, { threadId })
      const onUserMessage = vi.spyOn(sink, 'onUserMessage')
      await deliver(sink, 'original')
      expect(getState().blocks).toEqual([expect.objectContaining({ id: originalId, text: 'QA_NATIVE_V2_BUSY' })])
      set({ blocks: [...getState().blocks, questionTool, questionCard, pending('question'), pending('ordinary')] })
      sink.onTurnSteered?.({ turnId, clientUserMessageId: questionClient, admittedSeq: 30 })
      sink.onTurnSteered?.({ turnId, clientUserMessageId: ordinaryClient, admittedSeq: 31 })
      await deliver(sink, 'question')
      await deliver(sink, 'ordinary')
      expect(onUserMessage).toHaveBeenCalledWith(expect.objectContaining({ itemId: questionItem, seq: 38 }))
      const users = getState().blocks.filter(b => b.kind === 'user')
      expect(users.map(b => b.id)).toEqual([originalId, questionItem, ordinaryItem])
      expect(users.map(b => b.kind === 'user' && b.text)).toEqual(['QA_NATIVE_V2_BUSY', questionReply, ordinaryText])
      expect(users.slice(1)).toEqual([expect.objectContaining({ meta: expect.objectContaining({ steeringStatus: 'accepted' }) }),
        expect.objectContaining({ meta: expect.objectContaining({ steeringStatus: 'accepted' }) })])
      expect(getState().blocks.filter(b => b.kind !== 'user')).toEqual([questionTool, questionCard])
      // latest-user identity is also the existing terminal snapshot contract.
      expect(getState().currentTurnUserId).toBe(ordinaryItem)
      const times = getState().turnStartedAtByUserId
      await deliver(sink, 'original')
      await deliver(sink, 'question')
      await dispatchAnalytixRuntimeEvent({ ...event('ordinary'), kind: 'item_completed' }, sink, async () => undefined)
      sink.onTurnSteered?.({ turnId, clientUserMessageId: questionClient, admittedSeq: 30 })
      sink.onTurnSteered?.({ turnId, clientUserMessageId: ordinaryClient, admittedSeq: 31 })
      expect(getState().blocks.filter(b => b.kind === 'user')).toEqual(users)
      expect(getState()).toMatchObject({ busy: true, currentTurnId: turnId, currentTurnUserId: ordinaryItem,
        turnStartedAtByUserId: times })
    })

    it('appends an accepted Question reply without a public pending user row', async () => {
      const { getState, set, get } = makeSinkHarness({ activeThreadId: threadId, currentTurnId: turnId,
        currentTurnUserId: originalId, blocks: [original(), questionTool, questionCard, pending('ordinary')] })
      const sink = buildThreadEventSink(set, get, { threadId })
      await deliver(sink, 'question')
      await deliver(sink, 'ordinary')
      await deliver(sink, 'question')
      const users = getState().blocks.filter(b => b.kind === 'user')
      expect(users).toHaveLength(3)
      expect(users.find(b => b.id === originalId)).toMatchObject({ text: 'QA_NATIVE_V2_BUSY', meta: { turnId } })
      expect(users.find(b => b.id === questionItem)).toMatchObject({ text: questionReply,
        meta: { turnId, clientUserMessageId: questionClient, steeringStatus: 'accepted' } })
      expect(users.find(b => b.id === ordinaryItem)).toMatchObject({ text: ordinaryText,
        meta: { turnId, clientUserMessageId: ordinaryClient, steeringStatus: 'accepted' } })
      expect(users.some(b => b.id === questionClient || b.id === ordinaryClient)).toBe(false)
      expect(getState().blocks.filter(b => b.kind !== 'user')).toEqual([questionTool, questionCard])
      expect(getState().currentTurnUserId).toBe(ordinaryItem)
    })

    it('uses Host sequence rather than arrival order when the earlier promotion arrives late', async () => {
      const { getState, set, get } = makeSinkHarness({ activeThreadId: threadId, currentTurnId: turnId,
        currentTurnUserId: originalId, blocks: [original(), pending('question'), pending('ordinary')] })
      const sink = buildThreadEventSink(set, get, { threadId })
      await deliver(sink, 'ordinary')
      const times = getState().turnStartedAtByUserId
      await deliver(sink, 'question')
      expect(getState().blocks.filter(b => b.kind === 'user').map(b => b.id)).toEqual([originalId, questionItem, ordinaryItem])
      expect(getState().currentTurnUserId).toBe(ordinaryItem)
      expect(getState().turnStartedAtByUserId).toBe(times)
    })

    it.each(['next-turn', 'terminal'] as const)('updates only the matching historical row after %s', async (state) => {
      const next: ChatBlock = { kind: 'user', id: 'item_next_user', text: 'The next question.', meta: { turnId: 'turn_next' } }
      const { getState, set, get } = makeSinkHarness({ activeThreadId: threadId, busy: state === 'next-turn',
        currentTurnId: state === 'next-turn' ? 'turn_next' : null,
        currentTurnUserId: state === 'next-turn' ? next.id : null,
        liveAssistant: 'Current draft.', turnStartedAtByUserId: { [next.id]: 2000 },
        blocks: [original(), pending('ordinary'), next] })
      const sink = buildThreadEventSink(set, get, { threadId })
      await deliver(sink, 'ordinary')
      await deliver(sink, 'original')
      expect(getState()).toMatchObject({ busy: state === 'next-turn',
        currentTurnId: state === 'next-turn' ? 'turn_next' : null,
        currentTurnUserId: state === 'next-turn' ? next.id : null,
        liveAssistant: 'Current draft.', turnStartedAtByUserId: { [next.id]: 2000 } })
      expect(getState().blocks).toEqual([expect.objectContaining(original()), expect.objectContaining({ id: ordinaryItem, text: ordinaryText,
        meta: expect.objectContaining({ turnId, steeringStatus: 'accepted' }) }), next])
    })

    it('rejects a crossed turn or already accepted item identity without guessing a matching row', async () => {
      const { getState, set, get } = makeSinkHarness({ activeThreadId: threadId, currentTurnId: turnId,
        currentTurnUserId: originalId, blocks: [original(), pending('ordinary')] })
      const sink = buildThreadEventSink(set, get, { threadId })
      const before = getState().blocks
      const crossed = event('ordinary')
      await dispatchAnalytixRuntimeEvent({ ...crossed, turnId: 'other-turn', item: { ...crossed.item!, turnId: 'other-turn' } }, sink, async () => undefined)
      sink.onTurnSteered?.({ turnId: 'other-turn', clientUserMessageId: ordinaryClient, admittedSeq: 31 })
      expect(getState().blocks).toBe(before)
      await deliver(sink, 'ordinary')
      const accepted = getState().blocks
      await dispatchAnalytixRuntimeEvent({ ...crossed, seq: 40, item: { ...crossed.item!, id: 'different-item' } }, sink, async () => undefined)
      expect(getState().blocks).toBe(accepted)
      await dispatchAnalytixRuntimeEvent({ ...crossed, seq: 41, item: { ...crossed.item!, clientUserMessageId: questionClient } }, sink, async () => undefined)
      expect(getState().blocks).toBe(accepted)
      await dispatchAnalytixRuntimeEvent({ ...crossed, seq: 42, item: { ...crossed.item!, id: originalId } }, sink, async () => undefined)
      expect(getState().blocks).toBe(accepted)
    })

    it('keeps the legacy initial ACK path without a sequence or steering metadata', () => {
      const { getState, set, get } = makeSinkHarness({ blocks: [{ kind: 'user', id: 'user-current', text: 'Initial question.' }] })
      buildThreadEventSink(set, get, { threadId: 'thread-current' }).onUserMessage({
        itemId: 'item_original', turnId: 'turn-current', text: 'Initial question.' })
      expect(getState().blocks).toEqual([expect.objectContaining({ id: 'item_original', text: 'Initial question.' })])
      expect(getState().currentTurnUserId).toBe('item_original')
    })
  })

  it('recovers unaccepted steering text from the trusted snapshot into the explicit queue', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [{
        kind: 'user',
        id: 'client_unaccepted_steer',
        text: 'do not lose this',
        meta: {
          turnId: 'turn-current',
          clientUserMessageId: 'client_unaccepted_steer',
          steeringStatus: 'admitted'
        }
      }],
      queuedMessages: []
    })
    const getThreadDetail = vi.fn(async () => ({
      blocks: getState().blocks,
      latestSeq: 1,
      latestTurnId: 'turn-current',
      threadStatus: 'completed'
    }))
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', getThreadDetail })

    await sink.onTurnComplete()

    expect(getState().blocks).toEqual([])
    expect(getState().queuedMessages).toEqual([
      expect.objectContaining({
        id: 'client_unaccepted_steer',
        text: 'do not lose this'
      })
    ])
    expect(queuedPauseReason(getState())).toBe('failed')
  })

  it('replaces accumulated assistant deltas with the trusted terminal snapshot', async () => {
    clearActiveStream()
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('window', {
      requestAnimationFrame: vi.fn((callback: FrameRequestCallback) => {
        frames.push(callback)
        return frames.length
      }),
      cancelAnimationFrame: vi.fn()
    })
    try {
      const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
      const getThreadDetail = vi.fn(async () => ({
        blocks: [{
          kind: 'assistant' as const,
          id: 'assistant-trusted',
          text: 'trusted final',
          meta: { turnId: 'turn-current' }
        }],
        latestSeq: 4,
        latestTurnId: 'turn-current',
        threadStatus: 'completed'
      }))
      const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', getThreadDetail })

      sink.onDeltas([{ kind: 'agent_message', text: 'he', seq: 1 }])
      expect(getState().liveAssistant).toBe('')
      expect(getState().lastSeq).toBe(1)
      expect(frames).toHaveLength(0)

      sink.onDeltas([{ kind: 'agent_message', text: 'l', seq: 2 }])
      sink.onDeltas([{ kind: 'agent_message', text: 'lo', seq: 3 }])
      expect(getState().liveAssistant).toBe('')
      expect(getState().lastSeq).toBe(3)
      expect(frames).toHaveLength(0)

      expect(getActiveStreamSnapshot().threadId).toBeNull()
      expect(getState().liveAssistant).toBe('')
      expect(getState().lastSeq).toBe(3)

      await sink.onTurnComplete()
      expect(getThreadDetail).toHaveBeenCalledWith('thread-current')
      expect(getState().blocks).toEqual([
        expect.objectContaining({
          kind: 'assistant',
          text: 'trusted final',
          meta: { turnId: 'turn-current' }
        })
      ])
    } finally {
      clearActiveStream()
      vi.unstubAllGlobals()
    }
  })

  it('reconciles a missing completed assistant answer from thread detail', async () => {
    const getThreadDetail = vi.fn(async () => ({
      blocks: [
        {
          kind: 'user' as const,
          id: 'user-current',
          text: 'answer from persisted detail',
          meta: { turnId: 'turn-current' }
        },
        {
          kind: 'assistant' as const,
          id: 'assistant-final',
          text: 'final answer from detail',
          meta: { turnId: 'turn-current' }
        }
      ],
      latestSeq: 12,
      threadStatus: 'completed'
    }))
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [
        {
          kind: 'user',
          id: 'user-current',
          text: 'answer from persisted detail',
          meta: { turnId: 'turn-current' }
        }
      ],
      liveAssistant: '',
      lastSeq: 10
    })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      getThreadDetail
    })

    await sink.onTurnComplete()

    expect(getThreadDetail).toHaveBeenCalledWith('thread-current')
    expect(getState().blocks).toEqual([
      expect.objectContaining({
        kind: 'user',
        id: 'user-current'
      }),
      expect.objectContaining({
        kind: 'assistant',
        id: 'assistant-final',
        text: 'final answer from detail',
        meta: { turnId: 'turn-current' }
      })
    ])
    expect(getState().lastSeq).toBe(12)
    expect(getState().busy).toBe(false)
  })

  it('always reconciles thread detail even when live text looks complete', async () => {
    const getThreadDetail = vi.fn(async () => ({
      blocks: [
        { kind: 'user' as const, id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } },
        { kind: 'assistant' as const, id: 'assistant-detail', text: 'final answer', meta: { turnId: 'turn-current' } }
      ],
      latestSeq: 12,
      threadStatus: 'completed'
    }))
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [{ kind: 'user', id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } }],
      liveAssistant: 'final answer',
      lastSeq: 11
    })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      getThreadDetail
    })

    await sink.onTurnComplete()

    expect(getThreadDetail).toHaveBeenCalledTimes(1)
    expect(getState().blocks.filter((block) => block.kind === 'assistant')).toEqual([
      expect.objectContaining({
        kind: 'assistant',
        text: 'final answer',
        meta: { turnId: 'turn-current' }
      })
    ])
  })

  it('reconciles detail when live assistant only contains a think block', async () => {
    const getThreadDetail = vi.fn(async () => ({
      blocks: [
        { kind: 'user' as const, id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } },
        { kind: 'assistant' as const, id: 'assistant-detail', text: 'visible final answer', meta: { turnId: 'turn-current' } }
      ],
      latestSeq: 12,
      threadStatus: 'completed'
    }))
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [{ kind: 'user', id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } }],
      liveAssistant: '<think>still thinking</think>',
      lastSeq: 11
    })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      getThreadDetail
    })

    await sink.onTurnComplete()

    expect(getThreadDetail).toHaveBeenCalledTimes(1)
    expect(getState().blocks.filter((block) => block.kind === 'assistant')).toEqual([
      expect.objectContaining({ kind: 'assistant', text: 'visible final answer' })
    ])
  })

  it('deletes fabricated live text and blocks downstream effects when terminal GET fails', async () => {
    const refreshThreads = vi.fn(async () => undefined)
    const drainQueuedMessages = vi.fn(async () => undefined)
    const getThreadDetail = vi.fn(async () => {
      throw new Error('runtime unavailable')
    })
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [
        { kind: 'user', id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } },
        { kind: 'assistant', id: 'assistant-draft', text: 'fabricated fact', meta: { turnId: 'turn-current' } }
      ],
      liveAssistant: 'more fabricated facts',
      refreshThreads,
      drainQueuedMessages
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', getThreadDetail })

    await sink.onTurnComplete({ threadId: 'thread-current', turnId: 'turn-current', seq: 12 })

    expect(getState().blocks.filter((block) => block.kind === 'assistant')).toEqual([])
    expect(getState().liveAssistant).toBe('')
    expect(getState().busy).toBe(false)
    expect(getState().currentTurnId).toBe('turn-current')
    expect(getState().error).toBe(i18n.t('common:runtimeFinalSnapshotUnavailable'))
    expect(queuedPauseReason(getState())).toBe('failed')
    expect(refreshThreads).not.toHaveBeenCalled()
    expect(drainQueuedMessages).not.toHaveBeenCalled()
  })

  it('rejects a case terminal GET whose accepted-final digest differs from the terminal event', async () => {
    const getThreadDetail = vi.fn(async () => ({
      blocks: [
        { kind: 'user' as const, id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } },
        { kind: 'assistant' as const, id: 'assistant-final', text: 'host final', meta: { turnId: 'turn-current' } }
      ],
      latestSeq: 13,
      latestTurnId: 'turn-current',
      threadStatus: 'completed',
      historyAuthority: 'case_boundary_only_v1' as const,
      latestTurnAcceptedFinalDigest: 'a'.repeat(64)
    }))
    const { getState, set, get } = makeSinkHarness({
      blocks: [
        { kind: 'user', id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } },
        { kind: 'assistant', id: 'assistant-draft', text: 'fabricated fact', meta: { turnId: 'turn-current' } }
      ],
      liveAssistant: 'fabricated tail'
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', getThreadDetail })

    await sink.onTurnComplete({
      threadId: 'thread-current',
      turnId: 'turn-current',
      seq: 13,
      acceptedFinalDigest: 'b'.repeat(64),
      terminalReason: 'success'
    })

    expect(getState().blocks.filter((block) => block.kind === 'assistant')).toEqual([])
    expect(getState().error).toBe(i18n.t('common:runtimeFinalSnapshotUnavailable'))
    expect(getState().currentTurnId).toBe('turn-current')
  })

  it('does not create a pending active stream from an unaccepted delta', () => {
    clearActiveStream()
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      currentTurnId: null,
      currentTurnUserId: null
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onDeltas([{ kind: 'agent_message', text: 'hello', seq: 1 }])

    expect(getState().currentTurnId).toBeNull()
    expect(getState().lastSeq).toBe(1)
    expect(getActiveStreamSnapshot().threadId).toBeNull()
    clearActiveStream()
  })

  it('creates an active stream shell when a turn_started event arrives before text', () => {
    clearActiveStream()
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null,
      blocks: [{ kind: 'user', id: 'user-started', text: 'hello' }]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onTurnStarted?.({
      threadId: 'thread-current',
      turnId: 'turn-started',
      createdAt: '2026-07-04T00:00:00.000Z',
      seq: 6
    })

    expect(getState().busy).toBe(true)
    expect(getState().currentTurnId).toBe('turn-started')
    expect(getState().currentTurnUserId).toBe('user-started')
    expect(getState().blocks[0]).toMatchObject({ meta: { turnId: 'turn-started' } })
    expect(getActiveStreamSnapshot()).toMatchObject({
      threadId: 'thread-current',
      turnId: 'turn-started',
      liveAssistant: '',
      lastSeq: 6
    })
    clearActiveStream()
  })

  it('never mirrors assistant draft deltas into the row-local active stream store', () => {
    clearActiveStream()
    vi.stubGlobal('window', {
      analytix: {
        app: { showTurnCompleteNotification: vi.fn(async () => ({ ok: true, shown: false })) },
        workspace: { releaseWorktree: vi.fn(async () => undefined) }
      }
    })
    try {
      const { set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
      const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

      sink.onDeltas([{ kind: 'agent_message', text: 'local text', seq: 1 }])

      expect(getActiveStreamSnapshot().threadId).toBeNull()

      sink.onTurnComplete()

      expect(getActiveStreamSnapshot().threadId).toBeNull()
      expect(getActiveStreamSnapshot().liveAssistant).toBe('')
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('keeps process events while discarding surrounding assistant drafts', () => {
    clearActiveStream()
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onDeltas([
      { kind: 'agent_message', text: 'preface before tool', seq: 2 }
    ])

	 expect('liveReasoning' in getState()).toBe(false)
    expect(getState().liveAssistant).toBe('')
    expect(getActiveStreamSnapshot().threadId).toBeNull()

    sink.onTool({
      itemId: 'tool-1',
      summary: 'Run tool',
      status: 'running',
      toolKind: 'tool_call'
    })

    expect('liveReasoning' in getState()).toBe(false)
    expect(getState().liveAssistant).toBe('')
    expect(getActiveStreamSnapshot().threadId).toBeNull()
	 expect(getState().blocks).toEqual([
      expect.objectContaining({ kind: 'tool', id: 'tool-1' })
    ])

    sink.onDeltas([{ kind: 'agent_message', text: 'final answer', seq: 3 }])

    expect(getState().liveAssistant).toBe('')
    expect(getActiveStreamSnapshot().threadId).toBeNull()

    clearActiveStream()
  })

  it('updates an existing approval card when the live stream resolves it', () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onApproval({
      approvalId: 'approval-1',
      summary: 'Approve command',
      toolName: 'exec'
    })
    sink.onApprovalStatus?.({
      approvalId: 'approval-1',
      itemId: 'approval-approval-1',
      status: 'error',
      errorMessage: 'approval expired'
    })

    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({
      kind: 'approval',
      approvalId: 'approval-1',
      status: 'error',
      errorMessage: 'approval expired'
    })
  })

  it('updates an existing user-input card when the live stream resolves it by input id', () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onUserInput({
      itemId: 'item_input_1',
      requestId: 'input-1',
      questions: [
        {
          header: 'Mode',
          id: 'mode',
          question: 'Choose a mode',
          options: [{ label: 'Fast', description: 'Use the faster path' }]
        }
      ]
    })
    sink.onUserInputStatus({
      itemId: 'input-1',
      status: 'cancelled',
      errorMessage: 'user input cancelled'
    })

    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({
      kind: 'user_input',
      id: 'item_input_1',
      requestId: 'input-1',
      status: 'cancelled',
      errorMessage: 'user input cancelled'
    })
  })

  it('rejects crossed user-input status identities before resolving either pending card', () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', busy: false })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const first = correlatedUserInputRequest('item_input_a', 'input-a')
    const second = correlatedUserInputRequest('item_input_b', 'input-b')
    sink.onUserInput(first)
    sink.onUserInput(second)
    const pending = getState().blocks

    sink.onUserInputStatus({ itemId: first.itemId, requestId: second.requestId, status: 'cancelled' })
    expect(getState().blocks).toEqual(pending)

    sink.onUserInputStatus({ itemId: first.itemId, requestId: first.requestId, status: 'cancelled' })
    expect(getState().blocks.map(block => block.kind === 'user_input' ? block.status : null)).toEqual(['cancelled', 'pending'])
    sink.onUserInputStatus({ itemId: second.itemId, requestId: second.requestId, status: 'cancelled' })
    expect(getState().blocks.map(block => block.kind === 'user_input' ? block.status : null)).toEqual(['cancelled', 'cancelled'])
  })

  it.each(['item', 'request'] as const)('rejects a replayed user-input request with a conflicting %s identity', (identity) => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', busy: false })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const first = correlatedUserInputRequest('item_input_a', 'input-a')
    sink.onUserInput(first)
    const original = getState().blocks

    sink.onUserInput(correlatedUserInputRequest(
      identity === 'item' ? first.itemId : 'item_input_b',
      identity === 'request' ? first.requestId : 'input-b'
    ))

    expect(getState().blocks).toEqual(original)
  })

  it.each(['submitted', 'cancelled'] as const)('does not reopen or flip a %s user-input card after late events', (status) => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', busy: false })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const request = { ...correlatedUserInputRequest('item_input_a', 'input-a'), turnId: 'turn_input_finished' }
    sink.onUserInput(request)
    sink.onUserInputStatus({ itemId: request.itemId, requestId: request.requestId, status })
    set({ busy: false, currentTurnId: null })
    sink.onUserInput(request)

    expect(getState()).toMatchObject({ busy: false, currentTurnId: null })

    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({ id: request.itemId, requestId: request.requestId, status })

    sink.onUserInputStatus({ itemId: request.itemId, requestId: request.requestId, status: status === 'submitted' ? 'cancelled' : 'submitted' })
    expect(getState().blocks[0]).toMatchObject({ id: request.itemId, requestId: request.requestId, status })
  })

  it('ignores an ambiguous legacy user-input status in restored blocks', () => {
    const first = correlatedUserInputRequest('item_shared', 'input-a')
    const second = correlatedUserInputRequest('item_shared', 'input-b')
    const blocks: ChatBlock[] = [first, second].map(request => ({
      kind: 'user_input', id: request.itemId, requestId: request.requestId, questions: request.questions, status: 'pending'
    }))
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', blocks, busy: false })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onUserInputStatus({ itemId: 'item_shared', status: 'cancelled' })

    expect(getState().blocks).toEqual(blocks)
  })

  it('drops replayed deltas at or below the subscription floor', () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', lastSeq: 100 })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      sinceSeq: 100
    })

    sink.onDeltas([
      { kind: 'agent_message', text: 'replayed history', seq: 90 },
      { kind: 'agent_message', text: 'fresh answer', seq: 101 }
    ])

    expect(getState().liveAssistant).toBe('')
    expect(getState().lastSeq).toBe(101)
  })

	it('withholds an assistant delta at the next seq', () => {
    clearActiveStream()
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', lastSeq: 100 })
    const sink = buildThreadEventSink(set, get, {
      threadId: 'thread-current',
      sinceSeq: 100
    })

    sink.onDeltas([
      { kind: 'agent_message', text: 'same seq answer', seq: 101 }
    ])

    expect(getActiveStreamSnapshot().threadId).toBeNull()
    expect(getState().lastSeq).toBe(101)
    clearActiveStream()
  })

	  it('drops duplicate delta bytes while keeping the maximum cursor', () => {
	    clearActiveStream()
	    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current' })
	    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onDeltas([{ kind: 'agent_message', text: 'hello', seq: 11 }])
    sink.onDeltas([{ kind: 'agent_message', text: 'hello', seq: 11 }])
    sink.onDeltas([{ kind: 'agent_message', text: ' world', seq: 12 }])

	    expect(getActiveStreamSnapshot().threadId).toBeNull()
	    expect(getState().liveAssistant).toBe('')
	    expect(getState().lastSeq).toBe(12)
	    clearActiveStream()
	  })

  it('never rewinds lastSeq when a stale heartbeat seq arrives', () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', lastSeq: 500 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onSeq(3)

    expect(getState().lastSeq).toBe(500)
  })

  it('adopts a tool-first turn id before text arrives', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: false,
      currentTurnId: null,
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onTool({
      itemId: 'tool_call_1',
      summary: 'Search files',
      status: 'running',
      turnId: 'turn_tool_first',
      meta: { callId: 'call_1' }
    })

    expect(getState()).toMatchObject({
      busy: true,
      currentTurnId: 'turn_tool_first'
    })
    expect(getState().blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_1',
      meta: { callId: 'call_1', turnId: 'turn_tool_first' }
    })
  })

  it('ignores stale turn completion without clearing the active turn', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: true,
      currentTurnId: 'turn-current',
      lastSeq: 10,
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onTurnComplete({
      threadId: 'thread-current',
      turnId: 'turn-old',
      seq: 99
    })

    expect(getState()).toMatchObject({
      busy: true,
      currentTurnId: 'turn-current',
      lastSeq: 99,
      blocks: []
    })
  })

  it.each(['switch-thread', 'abort-binding', 'return-with-new-binding'] as const)(
    'ignores a delayed snapshot response after %s', async (transition) => {
      const controller = new AbortController()
      const harness = makeSinkHarness({
        activeThreadId: 'thread-a', lastSeq: 3, busy: true,
        blocks: [{ kind: 'user', id: 'old-a-user', text: 'old public turn' }]
      })
      type Detail = { blocks: ChatBlock[]; latestSeq: number; threadStatus: 'idle' }
      let resolveDetail!: (detail: Detail) => void
      const getThreadDetail = vi.fn(() => new Promise<Detail>((resolve) => { resolveDetail = resolve }))
      const sink = buildThreadEventSink(harness.set, harness.get, {
        threadId: 'thread-a', signal: controller.signal, getThreadDetail
      })
      const pending = sink.onSnapshotRequired?.({
        threadId: 'thread-a', seq: 99, highestSeq: 99, replayEventCount: 99
      })
      expect(getThreadDetail).toHaveBeenCalledExactlyOnceWith('thread-a')
      if (transition !== 'abort-binding') harness.set({ activeThreadId: 'thread-b' })
      if (transition !== 'switch-thread') controller.abort()
      // Returning to A has a new subscription generation; the old binding stays aborted.
      harness.set({
        activeThreadId: transition === 'switch-thread' ? 'thread-b' : 'thread-a',
        blocks: [{ kind: 'user', id: 'current-user', text: 'current public turn' }],
        lastSeq: 7, busy: true, currentTurnId: 'current-new-turn', currentTurnUserId: 'current-user'
      })
      const current = harness.getState()
      resolveDetail({
        blocks: [{ kind: 'assistant', id: 'obsolete-answer', text: 'obsolete public snapshot' }],
        latestSeq: 99, threadStatus: 'idle'
      })
      await pending
      expect(harness.getState()).toEqual(current)
    }
  )

  it.each(['complete', 'error'] as const)('revokes delayed %s reconciliation across aborted bindings and A-B-A', async terminal => {
    for (const returnToA of [false, true]) for (const rejectDetail of [false, true]) {
      const controller = new AbortController()
      const harness = makeSinkHarness({ activeThreadId: 'thread-a', lastSeq: 3 })
      let resolve!: (detail: { blocks: ChatBlock[]; latestSeq: number; threadStatus: string }) => void
      let reject!: (error: Error) => void
      const getThreadDetail = vi.fn(() => new Promise<{ blocks: ChatBlock[]; latestSeq: number; threadStatus: string }>((done, fail) => {
        resolve = done; reject = fail
      }))
      const sink = buildThreadEventSink(harness.set, harness.get, { threadId: 'thread-a', signal: controller.signal, getThreadDetail })
      const pending = terminal === 'complete'
        ? sink.onTurnComplete({ threadId: 'thread-a', turnId: 'turn-current', seq: 99 })
        : sink.onError(new Error('Synthetic terminal failure'), { terminal: true, threadId: 'thread-a', turnId: 'turn-current', seq: 99, status: 'failed' })
      expect(getThreadDetail).toHaveBeenCalledExactlyOnceWith('thread-a')
      if (returnToA) harness.set({ activeThreadId: 'thread-b' })
      controller.abort()
      harness.set({ activeThreadId: 'thread-a', lastSeq: 7, busy: true, currentTurnId: 'turn-current',
        blocks: [{ kind: 'user', id: 'current-user', text: 'current public turn' }] })
      const current = harness.getState()
      if (rejectDetail) reject(new Error('Synthetic retired read failure'))
      else resolve({ blocks: [{ kind: 'assistant', id: 'obsolete', text: 'obsolete public snapshot' }], latestSeq: 99, threadStatus: 'idle' })
      await pending
      expect(harness.getState()).toEqual(current)
    }
  })

  it('mirrors only primary blocks after an SDD terminal commit with restored display history', async () => {
    const primary: ChatBlock[] = [{ kind: 'user', id: 'primary-user', text: 'current primary' }]
    const getDetail = vi.spyOn(AnalytixRuntimeProvider.prototype, 'getThreadDetail').mockResolvedValue({ blocks: primary, latestSeq: 22, threadStatus: 'idle' })
    const mirror = vi.spyOn(sddTranscript, 'notifySddChatTranscriptMirror').mockImplementation(() => undefined)
    const harness = makeSinkHarness({ lastSeq: 19, threads: [{ id: 'thread-current', title: '.analytixsdd/requirements/draft',
      updatedAt: '', model: '', mode: 'agent', workspace: '/workspace' }], blocks: [
      { kind: 'user', id: 'old-display', text: 'compressed display history' },
      { kind: 'compaction', id: 'compaction-old', summary: '安全说明', status: 'success', meta: { turnId: 'old-turn' } }
    ] })
    try {
      await buildThreadEventSink(harness.set, harness.get, { threadId: 'thread-current' }).onGeneralTerminalBatch?.(generalTerminalProjectionBatch())
      expect(getDetail).toHaveBeenCalledExactlyOnceWith('thread-current')
      expect(mirror).toHaveBeenCalledOnce()
      expect(mirror.mock.calls[0][0]().blocks).toBe(primary)
      expect(harness.getState().blocks.some(block => block.id === 'old-display')).toBe(true)
    } finally { getDetail.mockRestore(); mirror.mockRestore() }
  })

  it('preserves live progress past H while a same-binding snapshot-required read is pending', async () => {
    const harness = makeSinkHarness({ activeThreadId: 'thread-a', lastSeq: 3, busy: true })
    type Detail = { blocks: ChatBlock[]; latestSeq: number; threadStatus: string }
    let resolve!: (detail: Detail) => void
    const sink = buildThreadEventSink(harness.set, harness.get, { threadId: 'thread-a',
      getThreadDetail: () => new Promise<Detail>(done => { resolve = done }) })
    const pending = sink.onSnapshotRequired?.({ threadId: 'thread-a', seq: 99, highestSeq: 99, replayEventCount: 99 })
    sink.onUserMessage({ itemId: 'live-user', text: 'live', turnId: 'live-turn' })
    sink.onTurnStarted?.({ turnId: 'live-turn', seq: 100 }); sink.onSeq(100)
    const current = harness.getState()
    resolve({ blocks: [], latestSeq: 99, threadStatus: 'idle' })
    await pending
    expect(harness.getState()).toEqual(current)
  })

  it('retries whole-log usage after an accepted same-cursor snapshot while rejecting older snapshots', async () => {
    const blocks: ChatBlock[] = [{ kind: 'user', id: 'user-current', text: 'unchanged', meta: { turnId: 'turn-current' } }]
    const harness = makeSinkHarness({ activeThreadId: 'thread-current', lastSeq: 12, blocks, busy: false,
      currentTurnId: null, currentTurnUserId: null, usageRefreshKey: 10 })
    const getThreadDetail = vi.fn().mockResolvedValueOnce({ blocks, latestSeq: 12, threadStatus: 'idle' })
      .mockResolvedValueOnce({ blocks: [], latestSeq: 11, threadStatus: 'idle' })
    const sink = buildThreadEventSink(harness.set, harness.get, { threadId: 'thread-current', getThreadDetail })
    await sink.onSnapshotRequired?.({ threadId: 'thread-current', seq: 12, highestSeq: 12 })
    expect(harness.getState()).toMatchObject({ lastSeq: 12, busy: false, currentTurnId: null, usageRefreshKey: 11, blocks })
    await sink.onSnapshotRequired?.({ threadId: 'thread-current', seq: 12, highestSeq: 12 })
    expect(harness.getState()).toMatchObject({ lastSeq: 12, usageRefreshKey: 11, blocks })
  })
  it('reconciles a snapshot-required replay gap from thread detail', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: true,
      currentTurnId: 'turn-current',
      currentTurnUserId: 'user-current',
      blocks: [{ kind: 'user', id: 'user-current', text: 'hello', meta: { turnId: 'turn-current' } }]
    })
    const getThreadDetail = vi.fn(async () => ({
      blocks: [
        { kind: 'user' as const, id: 'user-current', text: 'hello', meta: { turnId: 'turn-current' } },
        { kind: 'assistant' as const, id: 'assistant-current', text: 'complete answer', meta: { turnId: 'turn-current' } }
      ],
      latestSeq: 1034,
      threadStatus: 'idle',
      latestTurnId: 'turn-current',
      latestUserMessageId: 'user-current'
    }))
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', getThreadDetail })

    sink.onDeltas([{ kind: 'agent_message', text: 'partial', seq: 10, turnId: 'turn-current' }])
    await sink.onSnapshotRequired?.({
      threadId: 'thread-current',
      seq: 1034,
      highestSeq: 1034,
      replayEventCount: 1034
    })

    expect(getThreadDetail).toHaveBeenCalledWith('thread-current')
    expect(getState()).toMatchObject({
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null,
      liveAssistant: '',
      lastSeq: 1034
    })
    expect(getState().blocks).toEqual([
      { kind: 'user', id: 'user-current', text: 'hello', meta: { turnId: 'turn-current' } },
      { kind: 'assistant', id: 'assistant-current', text: 'complete answer', meta: { turnId: 'turn-current' } }
    ])
    expect(getActiveStreamSnapshot().threadId).toBeNull()
  })

  it('recovers mapped case ordinary and protected history from a snapshot gap and completes the next ordinary turn', async () => {
    const threadId = 'thread-current'
    const timestamp = '2026-07-18T00:00:00Z'
    const publicationCommitId = 'f'.repeat(64)
    const accepted = acceptedFinalProjectionBatch()
    const view = {
      ...accepted.assistant.acceptedFinalView!,
      variant: 'SourceUnavailableAnswer', terminalReason: 'source_unavailable',
      blockerCode: 'current_case_source_unavailable', coverageStatus: 'unavailable'
    }
    const refusal = {
      id: 'item-final', turnId: 'turn-protected', threadId, role: 'assistant',
      status: 'completed', kind: 'assistant_text', createdAt: timestamp, finishedAt: timestamp,
      text: 'Current case source is unavailable.', acceptedFinalView: view
    }
    const batchId = '7'.repeat(64)
    const eventManifestDigest = '8'.repeat(64)
    const binding = { threadId, turnId: refusal.turnId, timestamp, publicationCommitId }
    const events = [
      { ...binding, kind: 'item_completed', seq: 20, itemId: refusal.id, item: refusal,
        acceptedFinalDigest: publicationCommitId, publicationSlot: 'assistant-final',
        publicationEventId: '1'.repeat(64), publicationPayloadDigest: '2'.repeat(64) },
      { ...binding, kind: 'usage', seq: 21, model: 'test-model',
        usage: { promptTokens: 10, completionTokens: 3, totalTokens: 13, cacheHitRate: 0, turns: 1 },
        cacheDiagnostics: {}, usageFinalStatus: 'completed', acceptedFinalDigest: publicationCommitId,
        publicationSlot: 'usage', publicationEventId: '3'.repeat(64), publicationPayloadDigest: '4'.repeat(64) },
      { ...binding, kind: 'turn_completed', seq: 22, status: 'completed',
        terminalReason: 'source_unavailable', acceptedFinalDigest: publicationCommitId,
        publicationSlot: 'terminal', publicationEventId: '5'.repeat(64), publicationPayloadDigest: '6'.repeat(64) }
    ]
    // Main-verified IPC boundary fixture, passed through the real provider parser/mapper.
    const delivery = {
      ...binding, schemaVersion: 2, purpose: 'analytix.accepted-final-delivery-batch/v2',
      kind: 'accepted_final_batch', batchId, seq: 22, firstSeq: 20, lastSeq: 22,
      eventManifestDigest, events,
      publicationAuthority: {
        ...binding, schemaVersion: 'accepted-final-delivery-seal.v1',
        purpose: 'analytix.accepted-final-delivery-seal/v1', sealId: '9'.repeat(64),
        acceptedFinalDispositionDigest: 'a'.repeat(64), terminalDispositionId: 'b'.repeat(64),
        eventManifestDigest, sequencedEventsDigest: 'c'.repeat(64), batchId, firstSeq: 20, lastSeq: 22,
        authorityAlgorithm: 'Ed25519', authorityKeyId: 'a'.repeat(64),
        authorityPublicKey: 'p'.repeat(43), authoritySignature: 's'.repeat(86)
      }
    }
    const ordinaryTurn = async (turnId: string, text: string) => {
      const sha256 = async (value: string) => Array.from(new Uint8Array(
        await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
      ), (byte) => byte.toString(16).padStart(2, '0')).join('')
      const ordinaryResult = {
        schemaVersion: 1, purpose: 'analytix.ordinary-result/v1',
        projectionVersion: 'analytix.ordinary-output-projection/v1',
        logicalEffect: 'ordinary', ordinaryWork: true, candidateOrigin: 'provider_ordinary_only',
        evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false, text,
        textSha256: await sha256(text), resultDigest: ''
      }
      ordinaryResult.resultDigest = await sha256('analytix/ordinary-result/v1\0' + JSON.stringify(ordinaryResult))
      const base = { threadId, turnId, status: 'completed', createdAt: timestamp, finishedAt: timestamp }
      return {
        id: turnId, threadId, status: 'completed', createdAt: timestamp, finishedAt: timestamp,
        items: [
          { ...base, id: `user-${turnId}`, role: 'user', kind: 'user_message', text: 'Update the parser.' },
          { ...base, id: `assistant-${turnId}`, role: 'assistant', kind: 'assistant_text', text, ordinaryResult }
        ]
      }
    }
    const response = {
      id: threadId, historyAuthority: 'case_boundary_only_v1', status: 'idle', latestSeq: 30,
      acceptedFinalDeliveries: [delivery],
      turns: [
        { id: refusal.turnId, threadId, status: 'completed', createdAt: timestamp,
          finishedAt: timestamp, acceptedFinalView: view, items: [refusal] },
        await ordinaryTurn('turn-current', 'The parser update is complete.')
      ]
    }
    const request = vi.spyOn(rendererRuntimeClient, 'runtimeRequest').mockImplementation(async () => ({
      ok: true, status: 200, body: JSON.stringify(response)
    }))
    try {
      const provider = new AnalytixRuntimeProvider()
      const { getState, set, get } = makeSinkHarness({ currentTurnUserId: 'user-turn-current' })
      const sink = buildThreadEventSink(set, get, {
        threadId, getThreadDetail: provider.getThreadDetail.bind(provider)
      })
      sink.onDeltas([{ kind: 'agent_message', text: 'partial draft', seq: 10, turnId: 'turn-current' }])
      await sink.onSnapshotRequired?.({ threadId, seq: 30, highestSeq: 30, replayEventCount: 30 })
      expect(request).toHaveBeenCalledWith('/v1/threads/thread-current', 'GET')
      expect(getState()).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null,
        liveAssistant: '', lastSeq: 30 })
      expect(getState().blocks.map((block) => block.id)).toEqual([
        'item-final', 'user-turn-current', 'assistant-turn-current'
      ])
      expect(getState().blocks[0]).toMatchObject({ acceptedFinalView: { variant: 'SourceUnavailableAnswer' },
        acceptedFinalProjectionReceipt: { batchId }, meta: { turnId: 'turn-protected' } })
      expect(getState().blocks[2]).toMatchObject({ kind: 'assistant', text: 'The parser update is complete.',
        meta: { turnId: 'turn-current' } })
      expect(getState().blocks[2]).not.toHaveProperty('acceptedFinalView')
      expect(getActiveStreamSnapshot().threadId).toBeNull()

      const nextTurn = await ordinaryTurn('turn-next', 'The next parser update is complete.')
      response.turns.push(nextTurn)
      response.latestSeq = 34
      set({ busy: true, currentTurnId: 'turn-next', currentTurnUserId: 'user-turn-next' })
      const reconnected = buildThreadEventSink(set, get, {
        threadId, sinceSeq: 30, getThreadDetail: provider.getThreadDetail.bind(provider)
      })
      reconnected.onUserMessage({
        itemId: nextTurn.items[0].id, turnId: 'turn-next',
        text: nextTurn.items[0].text, createdAt: timestamp, meta: { turnId: 'turn-next', activeSkillIds: [] }
      })
      // The provider advances the ordinary SSE cursor after dispatching non-terminal events.
      reconnected.onSeq(31)
      const nextBinding = { threadId, turnId: 'turn-next', timestamp }
      await dispatchAnalytixRuntimeEvents([{
        ...nextBinding, schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1',
        kind: 'general_terminal_batch', batchDigest: 'a'.repeat(64), seq: 34, firstSeq: 32, lastSeq: 34,
        generalTerminalCommitId: 'b'.repeat(64), generalTerminalAuthorityKind: 'general_terminal_cas',
        generalTerminalAuthorityDigest: 'c'.repeat(64), eventManifestDigest: 'd'.repeat(64),
        projectedEventsDigest: 'e'.repeat(64), transportAuthority: 'host_batch_digest_v1',
        evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
        events: [
          { ...nextBinding, kind: 'item_completed', seq: 32, itemId: nextTurn.items[1].id,
            item: nextTurn.items[1] },
          { ...nextBinding, kind: 'usage', seq: 33, model: 'test-model', usage: {
            promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
            cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
            cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
            priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
            tokenEconomySavingsTokens: 0, turns: 1
          }, cacheDiagnostics: {}, usageFinalStatus: 'completed' },
          { ...nextBinding, kind: 'turn_completed', seq: 34, status: 'completed', terminalReason: 'success' }
        ],
        eventManifest: [
          { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
          { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
          { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
        ]
      }], reconnected, async () => undefined)
      expect(request).toHaveBeenCalledTimes(1)
      expect(getState()).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null,
        liveAssistant: '', lastSeq: 34 })
      expect(getState().blocks.map((block) => block.id)).toEqual([
        'item-final', 'user-turn-current', 'assistant-turn-current', 'user-turn-next', 'assistant-turn-next'
      ])
      expect(getState().blocks.at(-1)).toMatchObject({ text: 'The next parser update is complete.',
        meta: { turnId: 'turn-next' } })
      expect(getState().blocks.at(-1)).not.toHaveProperty('acceptedFinalProjectionReceipt')
      const reloaded = await provider.getThreadDetail(threadId)
      const historyProjection = getState().blocks.map(block => block.kind === 'assistant'
        ? Object.fromEntries(Object.entries(block).filter(([key]) => key !== 'generalTerminalProjectionReceipt')) : block)
      expect(reloaded.blocks).toEqual(historyProjection)
      expect(reloaded.latestSeq).toBe(getState().lastSeq)
    } finally {
      request.mockRestore()
    }
  })

  it('keeps admitted neutral request telemetry out of the transcript without exposing wire prose', async () => {
    const { getState, set, get } = makeSinkHarness({ activeThreadId: 'thread-current', currentTurnId: 'turn_1', blocks: [] })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    for (const [index, stage] of (['pre_send', 'post_send', 'response_received'] as const).entries()) {
      sink.onSeq(index + 1) // The existing transport advances the cursor before mapper dispatch.
      await dispatchAnalytixRuntimeEvents([{ kind: 'pipeline_stage', seq: index + 1,
        threadId: 'thread-current', turnId: 'turn_1', stage, label: 'UNTRUSTED_PROVIDER_PROGRESS_PROSE',
        timestamp: '2026-09-26T00:00:00Z' }], sink, async () => undefined)
      expect(getState().blocks).toEqual([])
      expect(getState().lastSeq).toBe(index + 1)
      expect(JSON.stringify(getState().blocks)).not.toContain('UNTRUSTED_PROVIDER_PROGRESS_PROSE')
    }
    expect(getState()).toMatchObject({ busy: true, liveAssistant: '', currentTurnId: 'turn_1' })
  })

  it('does not render blank runtime status rows for internal pipeline stages', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onRuntimeStatus?.({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_1_internal_stage_1',
      turnId: 'turn_1',
      stage: 'internal_stage'
    })

    expect(getState().blocks).toEqual([])
  })

  it('preserves safe provider diagnostics on runtime status rows', () => {
    // Deterministic synthetic redaction fixture; never use a real credential.
    const syntheticRedactionToken = 'sk-' + 'x'.repeat(24)
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current', currentTurnId: 'turn_1',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onRuntimeStatus?.({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_1_provider_error_2',
      turnId: 'turn_1',
      stage: 'provider_error',
      message: 'Provider stream failed',
      meta: {
        details: {
          providerError: {
            providerId: `custom-openai ${syntheticRedactionToken}`,
            endpointFormat: 'openai-chat',
            status: 401,
            authStatus: 'token=providerAuthSecret123456',
            hasApiKey: true,
            attempt: 1,
            failureStage: 'telemetry_begin',
            dispatchState: 'not_sent',
            requestUrl: 'https://example.test/v1/chat/completions?api_key=sk-secret'
          }
        }
      }
    })

    expect(getState().blocks[0]).toMatchObject({
      kind: 'system',
      id: 'runtime_status_turn_1_provider_error_2',
      text: '模型服务请求失败',
      meta: {
        providerError: {
          providerId: 'custom-openai <redacted>',
          endpointFormat: 'openai-chat',
          status: 401,
          authStatus: 'token=<redacted>',
          hasApiKey: true,
          attempt: 1,
          failureStage: 'telemetry_begin',
          dispatchState: 'not_sent'
        }
      }
    })
    const serialized = JSON.stringify(getState().blocks[0])
    expect((getState().blocks[0] as { meta?: { turnId?: string } }).meta?.turnId).toBe('turn_1')
    expect(serialized).not.toContain('requestUrl')
    expect(serialized).not.toContain('sk-secret')
    expect(serialized).not.toContain('providerSecret1234567890')
    expect(serialized).not.toContain('providerAuthSecret123456')
  })

  it('preserves provider retry attempts on runtime status rows', async () => {
    await i18n.changeLanguage('en')
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current', currentTurnId: 'turn_1',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onRuntimeStatus?.({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_1_provider_retrying_2',
      turnId: 'turn_1',
      stage: 'provider_retrying',
      message: 'Retrying provider stream',
      attempt: 2,
      maxAttempt: 3,
      meta: {
        details: {
          providerError: {
            providerId: 'custom-openai',
            status: 503,
            retryable: true
          }
        }
      }
    })

    expect(getState().blocks[0]).toMatchObject({
      kind: 'system',
      id: 'runtime_status_turn_1_provider_retrying_2',
      text: 'Provider request is retrying',
      meta: {
        providerError: {
          providerId: 'custom-openai',
          status: 503,
          retryable: true
        },
        providerRetry: {
          attempt: 2,
          maxAttempt: 3
        }
      }
    })
  })

  it('preserves provider recovery attempts on runtime status rows', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current', currentTurnId: 'turn_1',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onRuntimeStatus?.({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_1_empty_final_recovered_3',
      turnId: 'turn_1',
      stage: 'empty_final_recovered',
      message: 'Provider returned an empty final response',
      meta: {
        details: {
          visibleRecovery: true,
          recoveryKind: 'empty_final',
          recoveryAttempt: 1,
          maxRecoveryAttempts: 1,
          recoveryExhausted: true
        }
      }
    })

    expect(getState().blocks[0]).toMatchObject({
      kind: 'system',
      id: 'runtime_status_turn_1_empty_final_recovered_3',
      text: '模型响应为空，已切换到安全恢复输出',
      meta: {
        providerRecovery: {
          kind: 'empty_final',
          attempt: 1,
          maxAttempt: 1,
          recoveryExhausted: true
        }
      }
    })
  })

  it('drops marker-free private text from runtime status and job diagnostics', () => {
    const privateSentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current', currentTurnId: 'turn_1',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onRuntimeStatus?.({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_1_private_sentinel',
      turnId: 'turn_1',
      stage: 'provider_error',
      message: privateSentinel,
      meta: {
        details: {
          notificationKind: 'background_job_completion',
          jobId: 'job_safe',
          status: 'completed',
          outputPreview: privateSentinel,
          error: privateSentinel,
          reason: privateSentinel,
          autoContinueReason: privateSentinel,
          autoContinueError: privateSentinel,
          deliveryReason: privateSentinel,
          deliveryError: privateSentinel,
          recoveryReason: privateSentinel,
          deadLetterReason: privateSentinel,
          warning: privateSentinel,
          suggestion: privateSentinel,
          outputBytes: privateSentinel.length
        }
      }
    })

    expect(getState().blocks[0]).toMatchObject({
      kind: 'system',
      text: '模型服务请求失败',
      meta: {
        diagnostics: {
          notificationKind: 'background_job_completion',
          jobId: 'job_safe',
          status: 'completed'
        }
      }
    })
    expect(JSON.stringify(getState().blocks[0])).not.toContain(privateSentinel)
  })

  it('merges a partial tool dispatch with the completed tool result for the same call id', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_ready',
        seq: 21,
        callId: 'call_read',
        toolName: 'read',
        readyCount: 1
      },
      sink,
      async () => undefined
    )

    expect(getState().blocks).toEqual([
      expect.objectContaining({
        kind: 'tool',
        id: 'tool_call_read',
        status: 'running',
        meta: expect.objectContaining({
          callId: 'call_read',
          runtimeStatus: 'tool_call_ready'
        })
      })
    ])

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'item_completed',
        seq: 22,
        item: {
          id: 'item_result_read',
          turnId: 'turn-current',
          threadId: 'thread-current',
          role: 'tool',
          status: 'completed',
          createdAt: '2026-06-28T00:00:00.000Z',
          finishedAt: '2026-06-28T00:00:01.000Z',
          kind: 'tool_result',
          toolName: 'read',
          callId: 'call_read',
          output: {
            schemaVersion: 1,
            projectionKind: 'host_status',
            disclosure: 'metadata_only',
            status: 'completed',
            privatePayloadWithheld: true,
            factAnswerAllowed: false,
            evidenceAuthority: false,
            messageKey: 'tool_completed',
            code: 'tool_completed'
          }
        }
      },
      sink,
      async () => undefined
    )

    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_read',
      status: 'success',
      detail: 'tool_completed (tool_completed)',
      meta: expect.objectContaining({
        sourceItemId: 'item_result_read',
        callId: 'call_read',
        toolName: 'read'
      })
    })
    expect(JSON.stringify(getState().blocks[0])).not.toContain('/tmp/project/README.md')
    expect(JSON.stringify(getState().blocks[0])).not.toContain('hello')
  })

  it('updates the existing tool row when progress carries both itemId and callId', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_ready',
        seq: 31,
        itemId: 'item_call_task',
        callId: 'call_task',
        toolName: 'task',
        readyCount: 1
      },
      sink,
      async () => undefined
    )
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_progress',
        seq: 32,
        itemId: 'item_progress_task',
        callId: 'call_task',
        toolName: 'task',
        summary: 'Research child',
        status: 'running',
        message: 'subagent running'
      },
      sink,
      async () => undefined
    )

    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_task',
      summary: 'Tool activity',
      status: 'running',
      meta: expect.objectContaining({
        sourceItemId: 'item_progress_task',
        callId: 'call_task',
        runtimeStatus: 'tool_progress'
      })
    })
    expect(getState().blocks[0]).toHaveProperty('detail', undefined)
  })

  it('reprojects direct progress events before they enter renderer state', () => {
    const marker = 'SENTINEL_DIRECT_TOOL_EVENT_31C4'
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      currentTurnId: 'turn-current',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onTool({
      itemId: 'tool_call_task',
      turnId: 'turn-current',
      summary: marker,
      status: 'running',
      toolKind: 'subagent',
      detail: `${marker}:detail`,
      filePath: `/tmp/${marker}`,
      meta: {
        turnId: 'turn-current',
        callId: 'call_task',
        runtimeStatus: 'tool_progress',
        command: `printf ${marker}`,
        child: {
          parentThreadId: 'thread-current',
          parentTurnId: 'turn-current',
          childId: 'child_1',
          childRunId: 'job_1',
          childStatus: 'running',
          childLabel: marker,
          worktreePath: `/tmp/${marker}`
        }
      }
    })

    expect(getState().blocks[0]).toMatchObject({
      kind: 'tool',
      summary: 'Subagent activity',
      meta: expect.objectContaining({ runtimeStatus: 'tool_progress' })
    })
    expect(JSON.stringify(getState().blocks)).not.toContain(marker)
  })

  it('stores only closed cache diagnostics from mapped usage events', async () => {
    const marker = 'SENTINEL_CACHE_DIAGNOSTIC_6D10'
    const { getState, set, get } = makeSinkHarness()
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await dispatchAnalytixRuntimeEvent({
      kind: 'usage',
      seq: 99,
      cacheDiagnostics: {
        prefixHash: 'a'.repeat(64),
        route: marker,
        prefixChangeReasons: [marker],
        toolCount: 2
      },
      usage: {
        promptTokens: 4,
        completionTokens: 1,
        totalTokens: 5,
        cacheMissReasons: [marker],
        cacheSuggestions: [`${marker}:suggestion`],
        turns: 1
      }
    }, sink, async () => undefined)

    expect(getState().lastTurnUsage?.snapshot.cacheDiagnostics).toEqual({
      prefixHash: 'a'.repeat(64),
      toolCount: 2
    })
    expect(JSON.stringify(getState().lastTurnUsage)).not.toContain(marker)
  })

  it('does not merge tool progress into an earlier turn with the same callId', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      currentTurnId: 'turn-2',
      blocks: [
        {
          kind: 'tool',
          id: 'tool_call_task',
          summary: 'Old task',
          status: 'success',
          toolKind: 'subagent',
          meta: { callId: 'call_task', turnId: 'turn-1' }
        }
      ]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onTool({
      itemId: 'tool_call_task',
      summary: 'New task',
      status: 'running',
      toolKind: 'subagent',
      meta: { callId: 'call_task', turnId: 'turn-2' },
      turnId: 'turn-2'
    })

    expect(getState().blocks).toHaveLength(2)
    expect(getState().blocks[0]).toMatchObject({
      kind: 'tool',
      summary: 'Old task',
      meta: expect.objectContaining({ turnId: 'turn-1' })
    })
    expect(getState().blocks[1]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_task:turn-2',
      summary: 'New task',
      status: 'running',
      meta: expect.objectContaining({ callId: 'call_task', turnId: 'turn-2' })
    })
  })

  it('coalesces child pipeline and tool progress events by child run id', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      currentTurnId: 'turn-parent',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_progress',
        seq: 40,
        threadId: 'thread-current',
        turnId: 'turn-parent',
        itemId: 'item_progress_task',
        callId: 'call_task',
        toolName: 'task',
        summary: 'Research child',
        status: 'running',
        message: 'subagent running',
        child: {
          parentThreadId: 'thread-current',
          parentTurnId: 'turn-parent',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childLabel: 'Research child',
          childStatus: 'running',
          continueFrom: 'job-source'
        }
      },
      sink,
      async () => undefined
    )
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 41,
        threadId: 'thread-current',
        turnId: 'turn-parent',
        stage: 'subagent_completed',
        label: 'Research child',
        child: {
          parentThreadId: 'thread-current',
          parentTurnId: 'turn-parent',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childLabel: 'Research child',
          childStatus: 'completed',
          childThreadId: 'thread-child',
          ...({
            sourceRef: 'job-source',
            artifactPath: '/tmp/project/.analytix/jobs/job-1.log'
          } as Record<string, unknown>)
        }
      },
      sink,
      async () => undefined
    )

    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({
      kind: 'tool',
      status: 'success',
      toolKind: 'subagent',
      meta: expect.objectContaining({
        runtimeStatus: 'subagent_completed',
        child: expect.objectContaining({
          childRunId: 'job-1',
          childStatus: 'completed',
          childThreadId: 'thread-child'
        })
      })
    })
    const block = getState().blocks[0]
    const child = block.kind === 'tool' ? block.meta?.child : undefined
    expect(child).not.toHaveProperty('sourceRef')
    expect(child).not.toHaveProperty('artifactPath')
  })

  it('keeps parallel child progress events with the same parent callId as separate blocks', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      currentTurnId: 'turn-parent',
      blocks: []
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    for (const [index, label] of ['Noether', 'Nash', 'Lagrange'].entries()) {
      await dispatchAnalytixRuntimeEvent(
        {
          kind: 'tool_progress',
          seq: 50 + index,
          threadId: 'thread-current',
          turnId: 'turn-parent',
          itemId: `item_progress_parallel_${index + 1}`,
          callId: 'call_parallel_tasks',
          toolName: 'parallel_tasks',
          summary: label,
          status: 'running',
          message: 'subagent running',
          child: {
            parentThreadId: 'thread-current',
            parentTurnId: 'turn-parent',
            parentToolCallId: 'call_parallel_tasks',
            childId: `job-${index + 1}`,
            childRunId: `job-${index + 1}`,
            childLabel: label,
            childStatus: 'running',
            parallelGroupId: 'parallel-1',
            parallelIndex: index + 1
          }
        },
        sink,
        async () => undefined
      )
    }

    expect(getState().blocks).toHaveLength(3)
    expect(getState().blocks.map((block) => block.kind === 'tool' ? block.id : '')).toEqual([
      'subagent_job-1',
      'subagent_job-2',
      'subagent_job-3'
    ])
    expect(getState().blocks.map((block) => block.kind === 'tool' ? block.summary : '')).toEqual([
      'Subagent activity',
      'Subagent activity',
      'Subagent activity'
    ])
  })

  it('keeps Go tool lifecycle events visible before the live final answer', async () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      blocks: [{ kind: 'user', id: 'user-current', text: 'read the README' }]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_started',
        seq: 30,
        itemId: 'item_call_read',
        callId: 'call_read',
        toolName: 'read',
        summary: 'Read README'
      },
      sink,
      async () => undefined
    )
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'assistant_text_delta',
        seq: 31,
        text: 'The README says hello.'
      },
      sink,
      async () => undefined
    )

    expect(getState().blocks).toEqual([
      { kind: 'user', id: 'user-current', text: 'read the README' },
      expect.objectContaining({
        kind: 'tool',
        id: 'tool_call_read',
        status: 'running',
        summary: 'Read README'
      })
    ])
    expect(getState().liveAssistant).toBe('')

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_finished',
        seq: 32,
        itemId: 'item_result_read',
        callId: 'call_read',
        toolName: 'read',
        summary: 'Read README',
        status: 'completed',
        message: '/tmp/project/README.md'
      },
      sink,
      async () => undefined
    )

    expect(getState().blocks[1]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_read',
      status: 'success'
    })
    expect(getState().blocks[1]).toHaveProperty('detail', undefined)
    expect(getState().liveAssistant).toBe('')
  })
})

describe('busy watchdog re-arming on live ticks (#goal-recovering-banner)', () => {
  const BUSY_WATCHDOG_MS = 180_000

  beforeEach(() => {
    vi.useFakeTimers()
    resetBusyRecoveryAttempts()
  })
  afterEach(() => {
    clearBusyWatchdog()
    vi.useRealTimers()
  })

  it('keeps a long, quiet-but-healthy turn alive: heartbeats (onSeq) postpone recovery', () => {
    const recoverActiveTurn = vi.fn().mockResolvedValue(true)
    const { set, get } = makeSinkHarness({ busy: true, recoverActiveTurn })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    // Turn starts → watchdog armed (mirrors onUserMessage).
    armBusyWatchdog(set, get)

    // 10 minutes of nothing but the runtime's 15s heartbeat — e.g. one long
    // tool call producing no output. Each heartbeat ticks onSeq.
    for (let elapsed = 0; elapsed < 600_000; elapsed += 15_000) {
      vi.advanceTimersByTime(15_000)
      sink.onSeq(1)
    }

    // Stream is healthy the whole time, so the "正在恢复…" recovery never fires.
    expect(recoverActiveTurn).not.toHaveBeenCalled()
  })

  it('still recovers when the stream genuinely stalls (no ticks for the full window)', () => {
    const recoverActiveTurn = vi.fn().mockResolvedValue(true)
    const { set, get } = makeSinkHarness({ busy: true, recoverActiveTurn })
    buildThreadEventSink(set, get, { threadId: 'thread-current' })

    armBusyWatchdog(set, get)
    vi.advanceTimersByTime(BUSY_WATCHDOG_MS)

    expect(recoverActiveTurn).toHaveBeenCalledTimes(1)
  })

  it('quarantines provisional assistant text and pauses the queue after recovery is exhausted', () => {
    const recoverActiveTurn = vi.fn().mockResolvedValue(false)
    const drainQueuedMessages = vi.fn(async () => undefined)
    const { getState, set, get } = makeSinkHarness({
      busy: true,
      recoverActiveTurn,
      drainQueuedMessages,
      blocks: [
        { kind: 'user', id: 'user-current', text: 'prompt', meta: { turnId: 'turn-current' } },
        { kind: 'assistant', id: 'assistant-draft', text: 'fabricated draft', meta: { turnId: 'turn-current' } }
      ],
      liveAssistant: 'fabricated live tail',
      queuedMessages: [{ threadId: 'thread-current', id: 'queued-1', text: 'next request' }]
    })

    for (let attempt = 0; attempt < 4; attempt += 1) {
      armBusyWatchdog(set, get)
      vi.advanceTimersByTime(BUSY_WATCHDOG_MS)
    }

    expect(recoverActiveTurn).toHaveBeenCalledTimes(3)
    expect(getState().busy).toBe(false)
    expect(getState().currentTurnId).toBe('turn-current')
    expect(getState().liveAssistant).toBe('')
    expect(getState().blocks.filter((block) => block.kind === 'assistant')).toEqual([])
    expect(queuedPauseReason(getState())).toBe('failed')
    expect(drainQueuedMessages).not.toHaveBeenCalled()
  })

  it('does not keep a watchdog alive for an idle (non-busy) thread on heartbeats', () => {
    const recoverActiveTurn = vi.fn().mockResolvedValue(true)
    const { set, get } = makeSinkHarness({ busy: false, recoverActiveTurn })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    armBusyWatchdog(set, get)
    sink.onSeq(1) // heartbeat on an idle thread must not re-arm

    vi.advanceTimersByTime(BUSY_WATCHDOG_MS)
    // Watchdog fires once, sees busy=false, and bails without recovery.
    expect(recoverActiveTurn).not.toHaveBeenCalled()
  })
})

describe('thread event sink runtime errors', () => {
  it('adds runtime error events to the timeline with details', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: true,
      blocks: [{ kind: 'user', id: 'user-current', text: 'hello' }]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })

    sink.onRuntimeError?.({
      itemId: 'error-1',
      createdAt: '2026-06-08T00:00:00.000Z',
      message: 'Authorization: Bearer secret-token failed',
      code: 'provider_unavailable',
      details: { token: 'secret-token' },
      severity: 'error'
    })
    sink.onRuntimeError?.({
      itemId: 'error-1',
      createdAt: '2026-06-08T00:00:00.000Z',
      message: 'Authorization: Bearer secret-token failed again',
      code: 'provider_unavailable',
      severity: 'error'
    })

    const systemBlocks = getState().blocks.filter((block) => block.kind === 'system')
    expect(systemBlocks).toHaveLength(1)
    expect(systemBlocks[0]).toMatchObject({
      kind: 'system',
      id: 'error-1',
      code: 'provider_unavailable',
      severity: 'error'
    })
    expect(systemBlocks[0].text).toBe(i18n.t('common:runtimeRequestFailed'))
    expect(`${systemBlocks[0].text}\n${systemBlocks[0].detail}`).not.toMatch(/secret-token|Authorization|Bearer/)
  })

  it('deduplicates matching runtime error and turn failure events inside one turn', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
      busy: true,
      blocks: [{ kind: 'user', id: 'user-current', text: 'draw a poster' }]
    })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    const message = `model request failed with status 400: ${JSON.stringify({
      error: {
        code: '400',
        message: `Not supported model ${'mimo-v2.5-pro-ultraspeed'.repeat(10)}`
      }
    })}`

    sink.onRuntimeError?.({
      itemId: 'runtime_error_turn-current',
      createdAt: '2026-06-08T00:00:00.000Z',
      message,
      code: 'http_400',
      severity: 'error'
    })
    sink.onRuntimeError?.({
      itemId: 'item_turn-current_error',
      createdAt: '2026-06-08T00:00:01.000Z',
      message,
      code: 'http_400',
      severity: 'error'
    })

    const systemBlocks = getState().blocks.filter((block) => block.kind === 'system')
    expect(systemBlocks).toHaveLength(1)
    expect(systemBlocks[0]).toMatchObject({
      id: 'item_turn-current_error',
      code: 'http_400',
      severity: 'error'
    })
    expect(systemBlocks[0].detail).toContain('Code: http_400')
    expect(systemBlocks[0].detail).toContain(i18n.t('common:runtimeProviderConfigurationHint'))
    expect(systemBlocks[0].detail).not.toContain(message)
  })

  it('does not treat a transport error message as a terminal turn', async () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-1', text: 'run command' },
      {
        kind: 'tool',
        id: 'tool-1',
        summary: 'Running command',
        status: 'running',
        toolKind: 'command_execution'
      }
    ]
    const state = {
      activeThreadId: 'thr-1',
      blocks,
      busy: true,
      currentTurnId: 'turn-1',
      currentTurnUserId: 'user-1',
      error: null,
      liveAssistant: 'untrusted partial answer',
      turnStartedAtByUserId: { 'user-1': Date.now() - 1000 },
      turnDurationByUserId: {},
    } as unknown as ChatState
    const set = (partial: Partial<ChatState> | ((value: ChatState) => Partial<ChatState>)): void => {
      Object.assign(state, typeof partial === 'function' ? partial(state) : partial)
    }

    await buildThreadEventSink(set, () => state).onError(new Error('sse stream interrupted'), {
      terminal: false
    })

    expect(state.busy).toBe(true)
    expect(state.currentTurnId).toBe('turn-1')
    expect(state.currentTurnUserId).toBe('user-1')
    expect(state.liveAssistant).toBe('untrusted partial answer')
    expect(state.error).toBe(formatRuntimeError(new Error('sse stream interrupted')))
    expect(state.blocks.map((block) => ('status' in block ? block.status : block.kind))).toEqual([
      'user',
      'running'
    ])
  })

  it('settles terminal turn failures instead of keeping the composer busy', async () => {
    await i18n.changeLanguage('en')
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-1', text: 'work toward goal' },
      {
        kind: 'tool',
        id: 'tool-1',
        summary: 'Running command',
        status: 'running',
        toolKind: 'command_execution'
      }
    ]
    const state = {
      activeThreadId: 'thr-1',
      blocks,
      busy: true,
      currentTurnId: 'turn-1',
      currentTurnUserId: 'user-1',
      error: null,
      runtimeErrorDetail: null,
      liveAssistant: '',
      turnStartedAtByUserId: { 'user-1': Date.now() - 1000 },
      turnDurationByUserId: {},
      watchTurnCompletion: { 'thr-1': true },
      unreadThreadIds: { 'thr-1': true },
      queuedMessages: []
    } as unknown as ChatState
    const set = (partial: Partial<ChatState> | ((value: ChatState) => Partial<ChatState>)): void => {
      Object.assign(state, typeof partial === 'function' ? partial(state) : partial)
    }

    const getThreadDetail = vi.fn(async () => ({
      blocks: [
        { kind: 'user' as const, id: 'user-1', text: 'work toward goal', meta: { turnId: 'turn-1' } },
        {
          kind: 'tool' as const,
          id: 'tool-1',
          summary: 'Running command',
          status: 'error' as const,
          toolKind: 'command_execution' as const,
          meta: { turnId: 'turn-1' }
        }
      ],
      latestSeq: 4,
      latestTurnId: 'turn-1',
      threadStatus: 'failed'
    }))
    await buildThreadEventSink(set, () => state, {
      threadId: 'thr-1',
      getThreadDetail
    }).onError(
      new Error(JSON.stringify({
        code: 'http_400',
        message: 'model stream exploded',
        severity: 'error'
      })),
      { terminal: true, threadId: 'thr-1', turnId: 'turn-1', seq: 4, status: 'failed' }
    )

    expect(state.busy).toBe(false)
    expect(state.currentTurnId).toBeNull()
    expect(state.currentTurnUserId).toBeNull()
    expect(state.error).toBe(i18n.t('common:runtimeProviderConfigurationError'))
    expect(state.runtimeErrorDetail).toContain('Code: http_400')
    expect(state.runtimeErrorDetail).not.toContain('model stream exploded')
    expect(state.watchTurnCompletion).toEqual({})
    expect(state.unreadThreadIds).toEqual({})
    expect(state.blocks.map((block) => ('status' in block ? block.status : block.kind))).toEqual([
      'user',
      'error'
    ])
  })
})

describe('pending Claw Feishu mirrors', () => {
  afterEach(() => {
    clearPendingClawFeishuMirrors()
  })

  it('normalizes pending mirror fields before storing', () => {
    rememberPendingClawFeishuMirror(' turn-1 ', {
      threadId: ' thread-1 ',
      userBlockId: ' user-1 ',
      userText: ' hello '
    })

    expect(takePendingClawFeishuMirror('turn-1')).toEqual({
      threadId: 'thread-1',
      userBlockId: 'user-1',
      userText: 'hello'
    })
  })

  it('ignores invalid pending mirrors', () => {
    rememberPendingClawFeishuMirror('', {
      threadId: 'thread-1',
      userBlockId: 'user-1',
      userText: 'hello'
    })
    rememberPendingClawFeishuMirror('turn-2', {
      threadId: ' ',
      userBlockId: 'user-2',
      userText: 'hello'
    })
    rememberPendingClawFeishuMirror('turn-3', {
      threadId: 'thread-3',
      userBlockId: 'user-3',
      userText: ' '
    })

    expect(takePendingClawFeishuMirror('')).toBeUndefined()
    expect(takePendingClawFeishuMirror('turn-2')).toBeUndefined()
    expect(takePendingClawFeishuMirror('turn-3')).toBeUndefined()
  })

  it('caps pending mirrors and keeps the latest turns', () => {
    for (let index = 0; index < MAX_PENDING_CLAW_FEISHU_MIRRORS + 5; index += 1) {
      rememberPendingClawFeishuMirror(`turn-${index}`, {
        threadId: `thread-${index}`,
        userBlockId: `user-${index}`,
        userText: `hello-${index}`
      })
    }

    expect(takePendingClawFeishuMirror('turn-0')).toBeUndefined()
    expect(takePendingClawFeishuMirror('turn-4')).toBeUndefined()
    expect(takePendingClawFeishuMirror('turn-5')).toEqual({
      threadId: 'thread-5',
      userBlockId: 'user-5',
      userText: 'hello-5'
    })
    expect(takePendingClawFeishuMirror(`turn-${MAX_PENDING_CLAW_FEISHU_MIRRORS + 4}`)).toEqual({
      threadId: `thread-${MAX_PENDING_CLAW_FEISHU_MIRRORS + 4}`,
      userBlockId: `user-${MAX_PENDING_CLAW_FEISHU_MIRRORS + 4}`,
      userText: `hello-${MAX_PENDING_CLAW_FEISHU_MIRRORS + 4}`
    })
  })

  it('removes a pending mirror when taking it', () => {
    rememberPendingClawFeishuMirror('turn-1', {
      threadId: 'thread-1',
      userBlockId: 'user-1',
      userText: 'hello'
    })

    expect(takePendingClawFeishuMirror(' turn-1 ')).toEqual({
      threadId: 'thread-1',
      userBlockId: 'user-1',
      userText: 'hello'
    })
    expect(takePendingClawFeishuMirror('turn-1')).toBeUndefined()
  })
})

describe('watched completion notifications', () => {
  afterEach(() => {
    clearWatchedCompletionNotifications()
  })

  it('normalizes watched thread ids before storing and clearing', () => {
    watchTurnCompletionNotification(' thread-1 ', 1000)

    expect(completionNotificationDedupeKeyForWatchedThread('thread-1', 2000)).toBe('watch:thread-1:1000')

    clearWatchedCompletionNotification(' thread-1 ')

    expect(completionNotificationDedupeKeyForWatchedThread('thread-1', 2000)).toBe('watch:thread-1:2000')
  })

  it('ignores empty watched thread ids', () => {
    watchTurnCompletionNotification(' ', 1000)

    expect(completionNotificationDedupeKeyForWatchedThread('', 2000)).toBe('watch:unknown:2000')
  })

  it('caps watched completion notifications and keeps the latest thread watches', () => {
    for (let index = 0; index < MAX_WATCHED_COMPLETION_NOTIFICATIONS + 5; index += 1) {
      watchTurnCompletionNotification(`thread-${index}`, index)
    }

    expect(completionNotificationDedupeKeyForWatchedThread('thread-0', 999)).toBe('watch:thread-0:999')
    expect(completionNotificationDedupeKeyForWatchedThread('thread-4', 999)).toBe('watch:thread-4:999')
    expect(completionNotificationDedupeKeyForWatchedThread('thread-5', 999)).toBe('watch:thread-5:5')
    expect(
      completionNotificationDedupeKeyForWatchedThread(`thread-${MAX_WATCHED_COMPLETION_NOTIFICATIONS + 4}`, 999)
    ).toBe(`watch:thread-${MAX_WATCHED_COMPLETION_NOTIFICATIONS + 4}:${MAX_WATCHED_COMPLETION_NOTIFICATIONS + 4}`)
  })

  it('refreshes existing watched threads as the most recent entry', () => {
    watchTurnCompletionNotification('thread-0', 0)
    for (let index = 1; index < MAX_WATCHED_COMPLETION_NOTIFICATIONS; index += 1) {
      watchTurnCompletionNotification(`thread-${index}`, index)
    }
    watchTurnCompletionNotification('thread-0', 1000)
    watchTurnCompletionNotification(`thread-${MAX_WATCHED_COMPLETION_NOTIFICATIONS}`, 2000)

    expect(completionNotificationDedupeKeyForWatchedThread('thread-1', 999)).toBe('watch:thread-1:999')
    expect(completionNotificationDedupeKeyForWatchedThread('thread-0', 999)).toBe('watch:thread-0:1000')
  })
})


describe('main Host retry wire consumption', () => {
  afterEach(() => { clearBusyWatchdog(); clearActiveStream(); resetPublicProjectionRevocationsForTests(); vi.useRealTimers() })
  const retry = { schemaVersion: 1, source: 'provider_retry', logicalSequence: 7, outerAttempt: 2 }
  async function deliver(sink: ThreadEventSink, seq: number, phase: string, overrides: Record<string, unknown> = {}) {
    const event = { kind: 'pipeline_stage', seq, threadId: 'thread-current', turnId: 'turn-current',
      stage: 'provider_retrying', label: 'Retrying provider stream', timestamp: '2026-10-07T00:00:00.123456789Z',
      attempt: 2, maxAttempt: 3, details: { retry: { ...retry, phase,
        ...(phase === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1 } : {}),
        ...(phase === 'cancelled' ? { cancelReason: 'context_cancelled' } : {}) },
        providerError: { kind: 'network', status: 503, retryable: true } }, ...overrides }
    const decision = projectPublicRuntimeSseBlock(`id: ${seq}\nevent: pipeline_stage\ndata: ${JSON.stringify(event)}`,
      'thread-current', new PublicRuntimeEventFilter())
    expect(decision?.status).toBe('emit')
    if (decision?.status !== 'emit') throw new Error('invalid retry fixture')
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'main-retry', events: [decision.event] })).toBe(true)
    await dispatchAnalytixRuntimeEvent(decision.event as Parameters<typeof dispatchAnalytixRuntimeEvent>[0], sink, async () => undefined)
  }
  it.each(['general', 'accepted', 'failed', 'aborted', 'waiting-aborted'] as const)('retires live retry rows in the terminal commit while preserving provider errors (%s)', async scenario => {
    const mode = scenario === 'waiting-aborted' ? 'aborted' : scenario
    const phase = scenario === 'waiting-aborted' ? 'scheduled' : 'started'
    const { getState, set, get, setSnapshots } = makeSinkHarness({ lastSeq: 16 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    await deliver(sink, 17, 'scheduled')
    await deliver(sink, 18, phase)
    await deliver(sink, 19, phase, { details: { retry: { ...retry, logicalSequence: 8, phase,
      ...(phase === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1 } : {}) } } })
    sink.onSeq(19)
    expect(getState().blocks.filter(block => block.kind === 'system' && block.meta?.providerRetry)).toHaveLength(2)
    expect(getState().busy).toBe(true)
    const commits = setSnapshots().length
    const batch = mode === 'accepted' ? acceptedFinalProjectionBatch() : generalTerminalProjectionBatch(
      mode === 'failed' || mode === 'aborted' ? {
        terminalItem: { kind: 'system', id: 'terminal-error', text: 'Actual terminal outcome',
          severity: mode === 'failed' ? 'error' : 'info', code: mode, meta: { turnId: 'turn-current' } },
        terminal: { status: mode, createdAt: '2026-07-20T03:00:00Z', terminalReason: mode === 'failed' ? 'provider_failure' : 'cancel' }
      } : {}
    )
    if (mode === 'accepted') await sink.onAcceptedFinalBatch?.(batch as AcceptedFinalProjectionBatch)
    else await sink.onGeneralTerminalBatch?.(batch as GeneralTerminalProjectionBatch)
    expect(setSnapshots()).toHaveLength(commits + 1)
    const terminal = getState()
    expect(terminal.blocks.some(block => block.kind === 'system' && block.meta?.providerRetry)).toBe(false)
    expect(terminal.blocks.find(block => block.kind === 'system' && block.meta?.providerError)).toMatchObject({
      meta: { turnId: 'turn-current', providerError: { status: 503, retryable: true } }
    })
    expect(terminal.blocks.filter(block => block.kind === 'system')).toHaveLength(mode === 'failed' || mode === 'aborted' ? 2 : 1)
    if (mode === 'failed' || mode === 'aborted') expect(terminal.blocks.at(-1)).toMatchObject({
      id: 'terminal-error', text: 'Actual terminal outcome', code: mode
    })
    expect(terminal.blocks.some(block => block.kind === 'system' && block.text.includes('正在重试'))).toBe(false)
    expect(terminal.turnUsageById['turn-current'].snapshot).toEqual(batch.usage)
    expect(terminal.busy).toBe(false)
    await deliver(sink, 23, 'scheduled')
    expect(getState()).toBe(terminal)
  })
  it('updates one row across phases and ignores duplicate/out-of-order seq without erasing providerError', async () => {
    const { getState, set, get } = makeSinkHarness()
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    await deliver(sink, 40, 'scheduled'); sink.onSeq(40)
    const id = getState().blocks[0].id
    await deliver(sink, 41, 'started'); sink.onSeq(41)
    await deliver(sink, 40, 'scheduled'); await deliver(sink, 41, 'started')
    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({ id, meta: { turnId: 'turn-current',
      providerRetry: { attempt: 2, maxAttempt: 3, lifecycle: { ...retry, phase: 'started' } },
      providerError: { kind: 'network', status: 503, retryable: true } } })
    await deliver(sink, 42, 'cancelled')
    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({ id, meta: { providerRetry: { lifecycle: { phase: 'cancelled', cancelReason: 'context_cancelled' } } } })
    await deliver(sink, 43, 'scheduled')
    expect(getState().blocks[0]).toMatchObject({ id, meta: { providerRetry: { lifecycle: { phase: 'scheduled' } } } })
    expect(getState().busy).toBe(true)
    expect(getState().blocks.some(block => block.kind === 'assistant' || block.kind === 'tool')).toBe(false)
  })
  it('never adopts an old turn, wrong thread, inactive stream or terminal late status; accepts the next started turn', async () => {
    vi.useFakeTimers()
    const { getState, set, get } = makeSinkHarness({ lastSeq: 19 })
    const controller = new AbortController()
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current', signal: controller.signal })
    await deliver(sink, 40, 'started', { turnId: 'old-turn' })
    expect(getState().blocks).toEqual([])
    sink.onRuntimeStatus?.({ kind: 'pipeline_stage', itemId: 'wrong-thread', threadId: 'other-thread', turnId: 'turn-current', stage: 'provider_retrying' })
    expect(getState().currentTurnId).toBe('turn-current')
    expect(vi.getTimerCount()).toBe(0)
    await sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch())
    const terminal = getState()
    await deliver(sink, 43, 'scheduled')
    expect(getState()).toBe(terminal); expect(getState().busy).toBe(false); expect(vi.getTimerCount()).toBe(0)
    sink.onTurnStarted?.({ threadId: 'thread-current', turnId: 'next-turn', seq: 44 })
    await deliver(sink, 45, 'started', { turnId: 'next-turn' })
    expect(getState()).toMatchObject({ busy: true, currentTurnId: 'next-turn' })
    expect(getState().blocks.at(-1)).toMatchObject({ meta: { turnId: 'next-turn', providerRetry: { lifecycle: { phase: 'started' } } } })
    await deliver(sink, 46, 'started', { turnId: 'turn-current' })
    expect(getState().blocks.at(-1)).toMatchObject({ meta: { turnId: 'next-turn' } })
    set({ activeThreadId: 'switched-thread' }); const switched = getState()
    await deliver(sink, 47, 'scheduled', { turnId: 'next-turn' }); expect(getState()).toBe(switched)
    set({ activeThreadId: 'thread-current' }); controller.abort(); const aborted = getState()
    await deliver(sink, 48, 'cancelled', { turnId: 'next-turn' }); expect(getState()).toBe(aborted)
    clearBusyWatchdog(); vi.useRealTimers()
  })
  it('revalidates alternate sink metadata and keeps length observations nonterminal', async () => {
    const { getState, set, get } = makeSinkHarness()
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    sink.onRuntimeStatus?.({ kind: 'pipeline_stage', itemId: 'unsafe', turnId: 'turn-current', stage: 'provider_retrying',
      retry: { ...retry, phase: 'started', privateText: 'PRIVATE_ABC' } as never, message: 'PRIVATE_ABC' })
    expect(JSON.stringify(getState().blocks)).not.toContain('PRIVATE_ABC')
    const unsafe = getState().blocks[0]
    expect(unsafe.kind === 'system' ? unsafe.meta?.providerRetry : undefined).toBeUndefined()
    await deliver(sink, 40, 'started', { stage: 'response_received', label: 'Response Received', details: {
      stopReason: 'length', streamCompleted: false, logicalSequence: 7
    } })
    expect(getState()).toMatchObject({ busy: true, currentTurnId: 'turn-current' })
    expect(getState().blocks.some(block => block.kind === 'system' && block.meta?.stage === 'response_received')).toBe(false)
    expect(getState().blocks.some(block => block.kind === 'assistant' && block.generalTerminalProjectionReceipt)).toBe(false)
  })
  it('preserves ordinary thread notices after completion without starting a turn or watchdog', async () => {
    vi.useFakeTimers()
    const { getState, set, get } = makeSinkHarness({ lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    await sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch())
    set({ error: 'retain existing error' })
    sink.onRuntimeStatus?.({ kind: 'tool_catalog_changed', itemId: 'catalog', threadId: 'thread-current', seq: 23,
      message: 'PRIVATE_TEXT' })
    expect(getState()).toMatchObject({ busy: false, currentTurnId: null, error: 'retain existing error' })
    expect(getState().blocks.at(-1)).toMatchObject({ id: 'catalog', kind: 'system' })
    expect(JSON.stringify(getState().blocks)).not.toContain('PRIVATE_TEXT')
    expect(vi.getTimerCount()).toBe(0)
  })

})


describe('committed output cap terminal metadata', () => {
  it('commits an empty cap atomically on the existing user without an assistant block', async () => {
    const user: ChatBlock = { kind: 'user', id: 'user-current', text: 'Review ordinary code.', createdAt: '2026-07-20T02:00:00Z', meta: { turnId: 'turn-current' } }
    const { getState, set, get } = makeSinkHarness({ blocks: [user], lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    await sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch({ terminalItem: undefined, lastSeq: 21,
      terminal: { status: 'completed', createdAt: '2026-07-20T03:00:00Z', terminalReason: 'success', endReason: { kind: 'max-tokens' } } }))
    expect(getState()).toMatchObject({ busy: false, lastSeq: 21, currentTurnId: null })
    expect(getState().blocks).toHaveLength(1)
    expect(getState().blocks[0]).toMatchObject({ kind: 'user', generalTerminalProjectionReceipt: {
      threadId: 'thread-current', turnId: 'turn-current', itemId: 'user-current', firstSeq: 20, lastSeq: 21, endReason: { kind: 'max-tokens' }
    } })
    clearBusyWatchdog()
  })
  it('keeps the normal final item associated with the same capped terminal fact', async () => {
    const { getState, set, get } = makeSinkHarness({ lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    await sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch({ terminal: {
      status: 'completed', createdAt: '2026-07-20T03:00:00Z', terminalReason: 'success', endReason: { kind: 'max-tokens' }
    } }))
    expect(getState().blocks.at(-1)).toMatchObject({ kind: 'assistant', generalTerminalProjectionReceipt: { endReason: { kind: 'max-tokens' } } })
    clearBusyWatchdog()
  })
  it('rejects cap metadata on an alternate failed terminal sink', async () => {
    const { set, get } = makeSinkHarness({ lastSeq: 19 })
    const sink = buildThreadEventSink(set, get, { threadId: 'thread-current' })
    await expect(sink.onGeneralTerminalBatch?.(generalTerminalProjectionBatch({ terminal: {
      status: 'failed', createdAt: '2026-07-20T03:00:00Z', terminalReason: 'provider_failure', endReason: { kind: 'max-tokens' }
    } }))).rejects.toThrow(/end reason/)
    clearBusyWatchdog()
  })
})


describe('Queue recovery stable intent ownership', () => {
  afterEach(() => { clearActiveStream(); clearBusyWatchdog() })
  it('deduplicates stable IDs, preserves edited drafts and all retry metadata, and retains an explicit Stop', async () => {
    const draft: QueuedUserMessage = { threadId: 'thread-current', id: 'stable-steer', text: 'same text',
      mode: 'agent', model: 'deepseek-v4-pro', providerId: 'deepseek', modelLabel: 'Synthetic model', reasoningEffort: 'high',
      displayText: 'visible draft', attachmentIds: ['att-1'], attachments: [{ id: 'att-1', name: 'fixture.png', mimeType: 'image/png' }],
      fileReferences: [{ path: '/workspace/fixture/a.ts', relativePath: 'a.ts', name: 'a.ts', kind: 'file' }] }
    const ticket = { ...draft, sending: true }
    const edited = { threadId: draft.threadId, id: 'edited-steer', text: 'edited after failure' }
    const other = { threadId: 'thread-other', id: draft.id, text: 'another owner' }
    const meta = { turnId: 'turn-current', clientUserMessageId: draft.id, steeringStatus: 'pending' as const, steeringQueue: draft }
    const { getState, set, get } = makeSinkHarness({ queuedMessages: [ticket, edited, other],
      queuedMessagePauses: { 'thread-current': 'interrupted', 'thread-other': 'failed' }, blocks: [
        { kind: 'user', id: draft.id, text: draft.text, meta },
        { kind: 'user', id: edited.id, text: 'same text', meta: { ...meta, clientUserMessageId: edited.id, steeringQueue: { ...draft, id: edited.id } } },
        { kind: 'user', id: 'same-text-distinct-id', text: draft.text, meta: { ...meta, clientUserMessageId: 'same-text-distinct-id', steeringQueue: { ...draft, id: 'same-text-distinct-id' } } }
      ] })
    const sink = buildThreadEventSink(set, get, { threadId: draft.threadId,
      getThreadDetail: async () => ({ blocks: getState().blocks, latestSeq: 20, latestTurnId: 'turn-current', threadStatus: 'completed' }) })
    await sink.onTurnComplete()
    expect(getState().queuedMessages).toEqual([expect.objectContaining({ ...draft, sending: false }), edited, other,
      expect.objectContaining({ ...draft, id: 'same-text-distinct-id', sending: false })])
    expect(getState().queuedMessages).not.toContain(ticket)
    expect(getState().queuedMessagePauses).toEqual({ 'thread-current': 'interrupted', 'thread-other': 'failed' })
    expect(getState().blocks).toEqual([])
    const recovered = getState().queuedMessages
    await sink.onTurnComplete()
    expect(getState().queuedMessages).toEqual(recovered)
    expect(queuedPauseReason(getState())).toBe('interrupted')
  })
})

describe('actual watched completion poll ownership', () => {
  type Detail = Awaited<ReturnType<AnalytixRuntimeProvider['getThreadDetail']>>
  type RefreshOptions = { isCurrent?: () => boolean }

  function deferredWatch<T>() {
    let resolve!: (value: T) => void
    const promise = new Promise<T>(yes => { resolve = yes })
    return { promise, resolve }
  }

  async function settleWatch() {
    for (let step = 0; step < 20; step++) await Promise.resolve()
  }

  function detail(status: 'idle' | 'running'): Detail {
    return { blocks: [], latestSeq: 1, threadStatus: status }
  }

  function harness(ids: string[], register = true) {
    const provider = new AnalytixRuntimeProvider()
    const read = vi.spyOn(provider, 'getThreadDetail').mockResolvedValue(detail('running'))
    const providerOwner = vi.spyOn(providerRegistry, 'getProvider').mockReturnValue(provider)
    const notification = vi.fn(async (_request: { threadId: string; title: string; body: string }) => ({ ok: true, shown: true }))
    vi.stubGlobal('window', { analytix: { app: { showTurnCompleteNotification: notification } } })
    const refresh = vi.fn(async (_options?: RefreshOptions): Promise<void> => undefined)
    const state = makeSinkHarness({
      activeThreadId: 'synthetic-current-unrelated', busy: false, runtimeConnection: 'ready',
      currentTurnId: null, currentTurnUserId: null, lastSeq: 10,
      threads: ids.map(id => ({ id, title: id, workspace: '/synthetic/watch-owner',
        updatedAt: '2026-10-01T00:00:00Z', model: 'synthetic', mode: 'agent' })),
      watchTurnCompletion: Object.fromEntries(ids.map(id => [id, true])),
      unreadThreadIds: {}, deletedUserInputDraftThreadIds: {}, refreshThreads: refresh
    })
    if (register) for (const id of ids) watchTurnCompletionNotification(id)
    return { ...state, provider, providerOwner, read, notification, refresh,
      start: () => syncTurnCompletionPoll(state.set, state.get) }
  }

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-14T00:00:00Z'))
    stopTurnCompletionPoll()
    clearWatchedCompletionNotifications()
    clearActiveStream()
    resetPublicProjectionRevocationsForTests()
  })
  afterEach(() => {
    stopTurnCompletionPoll()
    clearWatchedCompletionNotifications()
    clearActiveStream()
    resetPublicProjectionRevocationsForTests()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('drops a terminal read whose thread was confirmed deleted while the actual poll was waiting', async () => {
    const id = 'watch-deleted-ack', h = harness([id]), response = deferredWatch<Detail>()
    h.read.mockImplementationOnce(() => response.promise)
    resetActiveStream(id, { turnId: 'cached-deleted-turn', liveAssistant: 'Synthetic cached observation', lastSeq: 7 })
    const stream = getActiveStreamSnapshotFor(id)
    h.start()
    expect(h.read).toHaveBeenCalledExactlyOnceWith(id)

    clearWatchedCompletionNotification(id)
    h.set(state => ({
      threads: state.threads.filter(thread => thread.id !== id), watchTurnCompletion: {},
      deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(state.deletedUserInputDraftThreadIds, id),
      error: 'New state after confirmed deletion'
    }))
    const committed = h.getState()
    response.resolve(detail('idle'))
    await settleWatch()

    expect(h.getState()).toBe(committed)
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(h.notification).not.toHaveBeenCalled()
    expect(h.refresh).not.toHaveBeenCalled()
    expect(getActiveStreamSnapshotFor(id)).toBe(stream)
  })

  it('rejects an old A ticket re-created at the same timestamp while completing the unchanged B watch', async () => {
    const a = 'watch-same-time-a', b = 'watch-same-time-b', h = harness([a, b], false)
    const timestamp = Date.now(), oldA = deferredWatch<Detail>()
    watchTurnCompletionNotification(a, timestamp)
    watchTurnCompletionNotification(b, timestamp)
    const oldDedupe = completionNotificationDedupeKeyForWatchedThread(a)
    h.read.mockImplementation(async id => id === b ? detail('idle') : detail('running'))
      .mockImplementationOnce(() => oldA.promise)
    h.start()
    expect(h.read.mock.calls).toEqual([[a]])

    h.set(state => ({ watchTurnCompletion: { ...state.watchTurnCompletion, [a]: false } }))
    clearWatchedCompletionNotification(a)
    watchTurnCompletionNotification(a, timestamp)
    h.set(state => ({ watchTurnCompletion: { ...state.watchTurnCompletion, [a]: true } }))
    expect(completionNotificationDedupeKeyForWatchedThread(a)).toBe(oldDedupe)
    resetActiveStream(a, { turnId: 'new-a-ticket-turn', liveAssistant: 'New A stream remains owned', lastSeq: 20 })
    resetActiveStream(b, { turnId: 'b-completed-turn', liveAssistant: 'B finishes', lastSeq: 12 })
    const newAStream = getActiveStreamSnapshotFor(a)
    oldA.resolve(detail('idle'))
    await settleWatch()

    expect(h.read.mock.calls).toEqual([[a], [b]])
    expect(h.notification.mock.calls.map(([request]) => request.threadId)).toEqual([b])
    expect(h.getState().watchTurnCompletion).toEqual({ [a]: true })
    expect(h.getState().unreadThreadIds).toEqual({ [b]: true })
    expect(getActiveStreamSnapshotFor(a)).toBe(newAStream)
    expect(getActiveStreamSnapshotFor(b).threadId).toBeNull()
    expect(h.refresh).toHaveBeenCalledOnce()

    h.read.mockResolvedValueOnce(detail('idle'))
    await vi.advanceTimersByTimeAsync(2500)
    expect(h.notification.mock.calls.map(([request]) => request.threadId)).toEqual([b, a])
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(h.getState().unreadThreadIds).toEqual({ [a]: true, [b]: true })
    expect(getActiveStreamSnapshotFor(a).threadId).toBeNull()
    expect(h.refresh).toHaveBeenCalledTimes(2)
  })

  it('does not settle an old provider response into a replacement provider watch', async () => {
    const id = 'watch-provider-replaced', h = harness([id]), response = deferredWatch<Detail>()
    h.read.mockImplementationOnce(() => response.promise)
    resetActiveStream(id, { turnId: 'provider-owner-turn', liveAssistant: 'Replacement context remains', lastSeq: 8 })
    const stream = getActiveStreamSnapshotFor(id)
    h.start()
    expect(h.read).toHaveBeenCalledExactlyOnceWith(id)
    const replacement = new AnalytixRuntimeProvider()
    const replacementRead = vi.spyOn(replacement, 'getThreadDetail').mockResolvedValue(detail('running'))
    h.providerOwner.mockReturnValue(replacement)
    const before = h.getState()
    response.resolve(detail('idle'))
    await settleWatch()

    expect(h.getState()).toBe(before)
    expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(replacementRead).not.toHaveBeenCalled()
    expect(h.notification).not.toHaveBeenCalled()
    expect(h.refresh).not.toHaveBeenCalled()
    expect(getActiveStreamSnapshotFor(id)).toBe(stream)
  })

  it.each(['revoked', 'deleted-draft'] as const)('fails closed for %s before reading and after a pending read', async reason => {
    const id = `watch-late-${reason}`, h = harness([id]), response = deferredWatch<Detail>()
    h.read.mockImplementationOnce(() => response.promise)
    resetActiveStream(id, { turnId: 'protected-watch-turn', liveAssistant: 'Synthetic protected observation', lastSeq: 9 })
    const stream = getActiveStreamSnapshotFor(id)
    h.start()
    expect(h.read).toHaveBeenCalledExactlyOnceWith(id)
    if (reason === 'revoked') markPublicProjectionRevoked(id)
    else h.set(state => ({ deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(state.deletedUserInputDraftThreadIds, id) }))
    const before = h.getState()
    response.resolve(detail('idle'))
    await settleWatch()
    expect(h.getState()).toBe(before)
    expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
    expect(h.notification).not.toHaveBeenCalled()
    expect(h.refresh).not.toHaveBeenCalled()
    expect(getActiveStreamSnapshotFor(id)).toBe(stream)

    await vi.advanceTimersByTimeAsync(2500)
    expect(h.read).toHaveBeenCalledOnce()
    expect(h.getState()).toBe(before)
    expect(h.getState().unreadThreadIds).toEqual({})
  })

  it('admits a fresh missing ticket before dispatch and completes only from that actual GET response', async () => {
    const id = 'watch-unregistered-ticket', h = harness([id], false)
    const response = deferredWatch<Detail>(), timestamp = Date.now()
    h.read.mockImplementationOnce(() => response.promise)
    resetActiveStream(id, { turnId: 'unregistered-watch-turn', liveAssistant: 'Pending actual watch', lastSeq: 3 })
    const stream = getActiveStreamSnapshotFor(id), before = h.getState()
    expect(completionNotificationDedupeKeyForWatchedThread(id, timestamp + 1)).toBe(`watch:${id}:${timestamp + 1}`)
    h.start()
    await settleWatch()
    expect(h.read).toHaveBeenCalledExactlyOnceWith(id)
    expect(completionNotificationDedupeKeyForWatchedThread(id, timestamp + 1)).toBe(`watch:${id}:${timestamp}`)
    expect(h.getState()).toBe(before)
    expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(h.notification).not.toHaveBeenCalled()
    expect(h.refresh).not.toHaveBeenCalled()
    expect(getActiveStreamSnapshotFor(id)).toBe(stream)

    response.resolve(detail('idle'))
    await settleWatch()
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(h.getState().unreadThreadIds).toEqual({ [id]: true })
    expect(h.notification.mock.calls.map(([request]) => request.threadId)).toEqual([id])
    expect(h.refresh).toHaveBeenCalledOnce()
    expect(getActiveStreamSnapshotFor(id).threadId).toBeNull()
  })

  it('re-admits a live watch evicted by more than the bounded notification-ticket capacity before its fresh GET', async () => {
    const id = 'watch-cap-evicted-fresh', h = harness([id]), timestamp = Date.now()
    for (let index = 0; index < MAX_WATCHED_COMPLETION_NOTIFICATIONS; index++) {
      watchTurnCompletionNotification(`watch-cap-filler-${index}`, index)
    }
    expect(completionNotificationDedupeKeyForWatchedThread(id, timestamp + 1)).toBe(`watch:${id}:${timestamp + 1}`)
    h.read.mockResolvedValueOnce(detail('idle'))
    h.start()
    await settleWatch()

    expect(h.read).toHaveBeenCalledExactlyOnceWith(id)
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(h.getState().unreadThreadIds).toEqual({ [id]: true })
    expect(h.notification.mock.calls.map(([request]) => request.threadId)).toEqual([id])
    expect(h.refresh).toHaveBeenCalledOnce()
    expect(completionNotificationDedupeKeyForWatchedThread('watch-cap-filler-0', timestamp + 1)).toBe(`watch:watch-cap-filler-0:${timestamp + 1}`)
    expect(completionNotificationDedupeKeyForWatchedThread('watch-cap-filler-1', timestamp + 1)).toBe('watch:watch-cap-filler-1:1')
  })

  it.each(['cleared', 'cap-evicted'] as const)('never supplements a %s ticket on an already-dispatched terminal response', async retirement => {
    const id = `watch-old-out-${retirement}`, h = harness([id]), response = deferredWatch<Detail>(), timestamp = Date.now()
    h.read.mockImplementationOnce(() => response.promise)
    resetActiveStream(id, { turnId: 'old-out-live-turn', lastSeq: 17 })
    const stream = getActiveStreamSnapshotFor(id), before = h.getState()
    h.start()
    expect(h.read).toHaveBeenCalledExactlyOnceWith(id)
    if (retirement === 'cleared') clearWatchedCompletionNotification(id)
    else {
      for (let index = 0; index < MAX_WATCHED_COMPLETION_NOTIFICATIONS; index++) {
        watchTurnCompletionNotification(`watch-old-out-filler-${index}`, index)
      }
    }
    const missingTicket = `watch:${id}:${timestamp + 1}`
    expect(completionNotificationDedupeKeyForWatchedThread(id, timestamp + 1)).toBe(missingTicket)
    response.resolve(detail('idle'))
    await settleWatch()

    expect(completionNotificationDedupeKeyForWatchedThread(id, timestamp + 1)).toBe(missingTicket)
    expect(h.getState()).toBe(before)
    expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(getActiveStreamSnapshotFor(id)).toBe(stream)
    expect(h.notification).not.toHaveBeenCalled()
    expect(h.refresh).not.toHaveBeenCalled()

    h.read.mockResolvedValueOnce(detail('idle'))
    await vi.advanceTimersByTimeAsync(2500)
    expect(h.read.mock.calls).toEqual([[id], [id]])
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(h.getState().unreadThreadIds).toEqual({ [id]: true })
    expect(h.notification.mock.calls.map(([request]) => request.threadId)).toEqual([id])
    expect(h.refresh).toHaveBeenCalledOnce()
    expect(getActiveStreamSnapshotFor(id).threadId).toBeNull()
  })

  it.each(['offline', 'revoked', 'deleted-draft'] as const)('does not admit a missing ticket or GET for an ineligible %s watch', async reason => {
    const id = `watch-ineligible-fresh-${reason}`, h = harness([id], false), fallbackAt = Date.now() + 1
    if (reason === 'offline') h.set({ runtimeConnection: 'offline' })
    else if (reason === 'revoked') markPublicProjectionRevoked(id)
    else h.set(state => ({ deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(state.deletedUserInputDraftThreadIds, id) }))
    const before = h.getState()
    h.start()
    await settleWatch()
    expect(h.read).not.toHaveBeenCalled()
    expect(h.getState()).toBe(before)
    expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(h.notification).not.toHaveBeenCalled()
    expect(h.refresh).not.toHaveBeenCalled()
    expect(completionNotificationDedupeKeyForWatchedThread(id, fallbackAt)).toBe(`watch:${id}:${fallbackAt}`)
  })

  it('rechecks the ticket after an actual stream-clear listener replaces the watch at the same time', async () => {
    const id = 'watch-stream-listener-replaced', h = harness([id]), timestamp = Date.now()
    h.read.mockResolvedValueOnce(detail('idle'))
    resetActiveStream(id, { turnId: 'old-listener-turn', lastSeq: 5 })
    let replaced = false
    const unsubscribe = subscribeActiveStream(() => {
      if (replaced || getActiveStreamSnapshotFor(id).threadId !== null) return
      replaced = true
      clearWatchedCompletionNotification(id)
      watchTurnCompletionNotification(id, timestamp)
      h.set({ watchTurnCompletion: { [id]: true } })
      resetActiveStream(id, { turnId: 'new-listener-turn', lastSeq: 25 })
    })
    try {
      h.start()
      await settleWatch()
      expect(replaced).toBe(true)
      expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
      expect(h.getState().unreadThreadIds).toEqual({})
      expect(getActiveStreamSnapshotFor(id).turnId).toBe('new-listener-turn')
      expect(h.notification).not.toHaveBeenCalled()
      expect(h.refresh).not.toHaveBeenCalled()
    } finally {
      unsubscribe()
    }
  })

  it('does not retire a fresh watch registered at the same time by the actual notification dispatch', async () => {
    const id = 'watch-notification-replaced', h = harness([id]), timestamp = Date.now()
    h.read.mockResolvedValueOnce(detail('idle'))
    resetActiveStream(id, { turnId: 'old-notification-turn', lastSeq: 6 })
    h.notification.mockImplementationOnce(async () => {
      clearWatchedCompletionNotification(id)
      watchTurnCompletionNotification(id, timestamp)
      h.set({ watchTurnCompletion: { [id]: true } })
      resetActiveStream(id, { turnId: 'new-notification-turn', lastSeq: 26 })
      return { ok: true, shown: true }
    })
    h.start()
    await settleWatch()
    expect(h.notification).toHaveBeenCalledOnce()
    expect(h.getState().watchTurnCompletion).toEqual({ [id]: true })
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(getActiveStreamSnapshotFor(id).turnId).toBe('new-notification-turn')
    expect(h.refresh).not.toHaveBeenCalled()
    expect(completionNotificationDedupeKeyForWatchedThread(id)).toBe(`watch:${id}:${timestamp}`)
  })

  it('allows its own confirmed completion to retire the watch and still finish a guarded refresh', async () => {
    const id = 'watch-own-completion-refresh', h = harness([id]), refreshed = deferredWatch<void>()
    let refreshCurrent: (() => boolean) | undefined
    h.read.mockResolvedValueOnce(detail('idle'))
    h.refresh.mockImplementationOnce(async options => {
      refreshCurrent = options?.isCurrent
      await refreshed.promise
      if (!options?.isCurrent?.()) return
      h.set({ lastSeq: 41 })
      setTimeout(() => {
        if (options.isCurrent?.()) h.set({ lastSeq: 42 })
      }, 1500)
    })
    resetActiveStream(id, { turnId: 'actual-completed-turn', liveAssistant: 'Finishes normally', lastSeq: 11 })
    h.start()
    await settleWatch()

    expect(h.notification.mock.calls.map(([request]) => request.threadId)).toEqual([id])
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(h.getState().unreadThreadIds).toEqual({ [id]: true })
    expect(getActiveStreamSnapshotFor(id).threadId).toBeNull()
    expect(h.refresh).toHaveBeenCalledExactlyOnceWith({ isCurrent: expect.any(Function) })
    expect(refreshCurrent?.()).toBe(true)
    await vi.advanceTimersByTimeAsync(2500)
    expect(h.read).toHaveBeenCalledOnce()
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(refreshCurrent?.()).toBe(true)
    h.start()
    expect(refreshCurrent?.()).toBe(true)
    refreshed.resolve(undefined)
    await settleWatch()
    expect(h.getState().lastSeq).toBe(41)
    expect(h.notification).toHaveBeenCalledOnce()
    expect(refreshCurrent?.()).toBe(true)
    await vi.advanceTimersByTimeAsync(1500)
    expect(h.getState().lastSeq).toBe(42)
    expect(h.notification).toHaveBeenCalledOnce()
  })

  it.each(['same-time-rewatch', 'provider', 'new-poll', 'new-error'] as const)('prevents a late completion refresh from replacing new state after %s', async replacement => {
    const id = `watch-late-refresh-${replacement}`, h = harness([id]), refreshed = deferredWatch<void>()
    const timestamp = Date.now()
    let refreshCurrent: (() => boolean) | undefined
    h.read.mockResolvedValueOnce(detail('idle'))
    h.refresh.mockImplementationOnce(async options => {
      refreshCurrent = options?.isCurrent
      await refreshed.promise
      if (!options?.isCurrent?.()) return
      h.set({ lastSeq: 1, error: 'Stale completion refresh', unreadThreadIds: { [id]: true } })
    })
    h.start()
    await settleWatch()
    expect(refreshCurrent?.()).toBe(true)
    expect(h.getState().watchTurnCompletion).toEqual({})
    expect(h.notification).toHaveBeenCalledOnce()

    if (replacement === 'same-time-rewatch') {
      watchTurnCompletionNotification(id, timestamp)
      h.set({ watchTurnCompletion: { [id]: true } })
    } else if (replacement === 'provider') {
      h.providerOwner.mockReturnValue(new AnalytixRuntimeProvider())
    } else if (replacement === 'new-poll') {
      const other = 'watch-new-poll-owner'
      watchTurnCompletionNotification(other, timestamp)
      h.set({ watchTurnCompletion: { [other]: true } })
      stopTurnCompletionPoll()
      h.start()
      await settleWatch()
    }
    const newError = replacement === 'new-error' ? 'New current state' : null
    h.set({ lastSeq: 73, error: newError, unreadThreadIds: {} })
    resetActiveStream(id, { turnId: 'replacement-refresh-owner', liveAssistant: 'New stream survives late refresh', lastSeq: 73 })
    const stream = getActiveStreamSnapshotFor(id), before = h.getState()
    expect(refreshCurrent?.()).toBe(false)
    refreshed.resolve(undefined)
    await settleWatch()

    expect(h.getState()).toBe(before)
    expect(h.getState().lastSeq).toBe(73)
    expect(h.getState().error).toBe(newError)
    expect(h.getState().unreadThreadIds).toEqual({})
    expect(getActiveStreamSnapshotFor(id)).toBe(stream)
    expect(h.notification).toHaveBeenCalledOnce()
    expect(h.refresh).toHaveBeenCalledOnce()
  })

  it.each(['same-time-rewatch', 'deleted', 'revoked', 'explicit-stop', 'cap-evicted', 'provider', 'new-error', 'offline-sync'] as const)(
    'retires an admitted reader after the completion callback returned when %s replaces its owner', async retirement => {
      const id = `watch-returned-reader-${retirement}`, h = harness([id]), reader = deferredWatch<void>()
      const timestamp = Date.now()
      let readerCurrent: (() => boolean) | undefined
      h.read.mockResolvedValueOnce(detail('idle'))
      h.refresh.mockImplementationOnce(async options => {
        readerCurrent = options?.isCurrent
        // This read is admitted during refresh but its result arrives after
        // the completion callback has returned, like the existing index reader.
        void reader.promise.then(() => {
          if (options?.isCurrent?.()) h.set({ lastSeq: 1, error: 'Retired reader result', unreadThreadIds: { [id]: true } })
        })
      })
      h.start()
      await settleWatch()
      expect(h.getState().watchTurnCompletion).toEqual({})
      expect(h.getState().unreadThreadIds).toEqual({ [id]: true })
      expect(h.notification).toHaveBeenCalledOnce()
      expect(h.refresh).toHaveBeenCalledExactlyOnceWith({ isCurrent: expect.any(Function) })
      expect(readerCurrent?.()).toBe(true)
      expect(completionNotificationDedupeKeyForWatchedThread(id, timestamp + 1)).toBe(`watch:${id}:${timestamp}`)

      if (retirement === 'same-time-rewatch') {
        watchTurnCompletionNotification(id, timestamp)
        h.set({ watchTurnCompletion: { [id]: true } })
        expect(completionNotificationDedupeKeyForWatchedThread(id)).toBe(`watch:${id}:${timestamp}`)
      } else if (retirement === 'deleted') {
        clearWatchedCompletionNotification(id)
        h.set(state => ({ deletedUserInputDraftThreadIds: markDeletedUserInputDraftThread(state.deletedUserInputDraftThreadIds, id) }))
      } else if (retirement === 'revoked') markPublicProjectionRevoked(id)
      else if (retirement === 'explicit-stop') stopTurnCompletionPoll()
      else if (retirement === 'cap-evicted') {
        for (let index = 0; index < MAX_WATCHED_COMPLETION_NOTIFICATIONS; index++) {
          watchTurnCompletionNotification(`watch-returned-reader-filler-${index}`, index)
        }
      } else if (retirement === 'provider') h.providerOwner.mockReturnValue(new AnalytixRuntimeProvider())
      else if (retirement === 'offline-sync') {
        h.set({ runtimeConnection: 'offline' })
        h.start()
        h.set({ runtimeConnection: 'ready' })
      }
      const error = retirement === 'new-error' ? 'New reader owner error' : null
      h.set({ lastSeq: 92, error, unreadThreadIds: {} })
      const before = h.getState()
      expect(readerCurrent?.()).toBe(false)
      reader.resolve(undefined)
      await settleWatch()
      expect(h.getState()).toBe(before)
      expect(h.getState().lastSeq).toBe(92)
      expect(h.getState().error).toBe(error)
      expect(h.getState().unreadThreadIds).toEqual({})
      expect(h.read).toHaveBeenCalledOnce()
      expect(h.notification).toHaveBeenCalledOnce()
      expect(h.refresh).toHaveBeenCalledOnce()
    }
  )
})
