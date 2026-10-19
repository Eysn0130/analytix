import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import maintenanceActionsSource from './chat-store-maintenance-actions.ts?raw'
import type { AgentProvider, ThreadUsageSnapshot, TurnUsageById, ChatBlock, NormalizedThread, ThreadEventSink, ThreadExecutionPolicy, ThreadGoal, ThreadGoalStatus, ThreadTodoList, ThreadTodoStatus } from '../agent/types'
import type { SseEventPayload, SseEndPayload, SseErrorPayload } from '@shared/analytix-api'
import { ThreadDetailResponseV1Schema } from '../../../../packages/runtime/src/contracts/thread-detail.js'
import type { ChatState, ChatStoreGet, ChatStoreSet, SendMessageOverrides, TurnForkSource, TurnForkSnapshot } from './chat-store-types'
import { readThreadForkRegistry } from '../lib/thread-fork-registry'
import i18n from '../i18n'
import { getThreadBinding } from './chat-store-thread-binding'
import { hasFreshThreadDetailCache, invalidateThreadDetailCache, loadThreadDetailWithCache } from '../lib/thread-detail-cache'
import { queuedMessagesForThread, queuedPauseReason } from './chat-store-queue'
import { composerDraftKey, emptyComposerDraft } from './composer-drafts'
import { clearActiveStream, getActiveStreamSnapshotFor, resetActiveStream } from '../thread/streaming/active-stream-store'
import { extractPlanTodos } from '../plan/plan-todo-sync'

const registryMock = vi.hoisted(() => ({
  getProvider: vi.fn()
}))

vi.mock('../agent/registry', () => ({
  getProvider: registryMock.getProvider
}))

import { createMaintenanceActions } from './chat-store-maintenance-actions'
import { createThreadActions } from './chat-store-thread-actions'
import { clearBusyWatchdog, stopTurnCompletionPoll } from './chat-store-schedulers'
import { buildThreadEventSink } from './chat-store-runtime'
import { AnalytixRuntimeProvider } from '../agent/analytix-runtime'
import { createOrdinaryHistoryReplay, isOrdinaryDisplayHistoryBlock } from '../agent/analytix-mapper'

const nativeRequestId = (hex: string) => 'input_' + hex.repeat(64)
const nativeQuestionSelector = (requestId: string, threadId: string | null) =>
  ({ threadId, turnId: 'turn-question-owner', itemId: 'item_' + requestId, requestId })

type GoalPatch = {
  objective?: string
  status?: ThreadGoalStatus
  tokenBudget?: number | null
  research?: { enabled: boolean; requirements?: string[] }
}

type Harness = {
  actions: ReturnType<typeof createMaintenanceActions>
  createThread: ReturnType<typeof vi.fn>
  drainQueuedMessages: ReturnType<typeof vi.fn>
  get: ChatStoreGet
  provider: {
    renameThread: ReturnType<typeof vi.fn>
    updateThreadExecutionPolicy: ReturnType<typeof vi.fn>
    archiveThread: ReturnType<typeof vi.fn>
    deleteThread: ReturnType<typeof vi.fn>
    forkThread: ReturnType<typeof vi.fn>
    resumeSession: ReturnType<typeof vi.fn>
    setThreadGoal: ReturnType<typeof vi.fn>
    clearThreadGoal: ReturnType<typeof vi.fn>
    interruptTurn: ReturnType<typeof vi.fn>
    getThreadDetail: ReturnType<typeof vi.fn>
    rewindThread: ReturnType<typeof vi.fn>
    getAttachmentMetadata: ReturnType<typeof vi.fn>
    setThreadTodos: ReturnType<typeof vi.fn>
    clearThreadTodos: ReturnType<typeof vi.fn>
  }
  recoverActiveTurn: ReturnType<typeof vi.fn>
  refreshThreads: ReturnType<typeof vi.fn>
  selectThread: ReturnType<typeof vi.fn>
  sendMessage: ReturnType<typeof vi.fn>
  sseAbortRef: { current: AbortController | null }
  state: ChatState
}

const footerUsage: ThreadUsageSnapshot = {
  inputTokens: 2, outputTokens: 1, reasoningTokens: 0, cachedTokens: 0, cacheMissTokens: 2,
  cacheHitRate: null, totalTokens: 3, costUsd: null, costCny: null, priceConfigured: false,
  tokenEconomySavingsTokens: 0, turns: 1
}
const footerLastUsage = { threadId: 'thr_existing', snapshot: footerUsage }
function footerEntries(turnIds: string[]): TurnUsageById {
  return Object.fromEntries(turnIds.map((turnId, index) => [turnId, {
    threadId: 'thr_existing', turnId, firstSeq: index * 3 + 1, lastSeq: index * 3 + 3, snapshot: footerUsage
  }]))
}
function thread(id: string, goal: ThreadGoal | null = null): NormalizedThread {
  return {
    id,
    title: id,
    updatedAt: '2026-06-04T00:00:00.000Z',
    model: 'deepseek-v4-pro',
    mode: 'agent',
    workspace: '/workspace/analytix',
    status: 'idle',
    goal
  }
}

function goal(
  threadId: string,
  objective = 'ship goal mode',
  status: ThreadGoalStatus = 'active'
): ThreadGoal {
  return {
    threadId,
    objective,
    status,
    tokenBudget: null,
    tokensUsed: 0,
    timeUsedSeconds: 0,
    createdAt: '2026-06-04T00:00:00.000Z',
    updatedAt: '2026-06-04T00:01:00.000Z'
  }
}

function todos(
  threadId: string,
  items: Array<[string, ThreadTodoStatus]>
): ThreadTodoList {
  return {
    threadId,
    turnId: 'turn_todo',
    updatedAt: '2026-06-04T00:02:00.000Z',
    items: items.map(([content, status], index) => ({
      id: `todo_${index + 1}`,
      content,
      status,
      createdAt: '2026-06-04T00:00:00.000Z',
      updatedAt: '2026-06-04T00:01:00.000Z'
    }))
  }
}

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

function buildHarness(options: {
  activeThreadId?: string | null
  createThreadSucceeds?: boolean
  initialGoal?: ThreadGoal | null
  initialTodos?: ThreadTodoList | null
} = {}): Harness {
  const activeThreadId = options.activeThreadId === undefined ? 'thr_existing' : options.activeThreadId
  const createThreadSucceeds = options.createThreadSucceeds ?? true
  const initialGoal = options.initialGoal ?? null
  const initialTodos = options.initialTodos ?? null
  let state: ChatState

  const provider = {
    renameThread: vi.fn(async (_threadId: string, _title: string) => undefined),
    updateThreadExecutionPolicy: vi.fn(async (threadId: string, policy: ThreadExecutionPolicy) => ({ ...thread(threadId), ...policy,
      executionPolicyVersion: 2 as const, updatedAt: '2026-10-20T00:01:00Z' })),
    archiveThread: vi.fn(async () => undefined),
    deleteThread: vi.fn(async () => undefined),
    forkThread: vi.fn(async (threadId: string, _options?: { turnId?: string; endSeq?: number }) => ({
      ...thread('thr_fork'),
      forkedFromThreadId: threadId,
      forkedFromTitle: 'Parent thread',
      forkedAt: '2026-06-04T00:02:00.000Z',
      forkedFromMessageCount: 2,
      forkedFromTurnCount: 1
    })),
    resumeSession: vi.fn(async (sessionId: string) => ({
      threadId: 'thr_resumed',
      sessionId
    })),
    setThreadGoal: vi.fn(async (threadId: string, patch: GoalPatch) =>
      goal(
        threadId,
        patch.objective ?? state.activeThreadGoal?.objective ?? initialGoal?.objective ?? 'ship goal mode',
        patch.status ?? state.activeThreadGoal?.status ?? initialGoal?.status ?? 'active'
      )
    ),
    clearThreadGoal: vi.fn(async () => true),
    interruptTurn: vi.fn(async () => undefined),
    getThreadDetail: vi.fn(async () => ({
      blocks: state.blocks,
      latestSeq: state.lastSeq,
      latestTurnId: state.currentTurnId ?? undefined,
      threadStatus: 'aborted'
    })),
    rewindThread: vi.fn(async () => undefined),
    getAttachmentMetadata: vi.fn(),
    setThreadTodos: vi.fn(async (threadId: string, items: ThreadTodoList['items']) => ({
      threadId,
      turnId: state.activeThreadTodos?.turnId,
      updatedAt: '2026-06-04T00:03:00.000Z',
      items: items.map((item) => ({
        id: item.id ?? `todo_${item.content}`,
        content: item.content,
        status: item.status,
        statusReasonCode: item.statusReasonCode,
        source: item.source,
        note: item.note,
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:03:00.000Z'
      }))
    })),
    clearThreadTodos: vi.fn(async () => true)
  }
  registryMock.getProvider.mockReturnValue(provider)

  const createThread = vi.fn(async () => {
    if (!createThreadSucceeds) return
    const created = thread('thr_created')
    state.activeThreadId = created.id
    state.threads = [created, ...state.threads]
  })
  const refreshThreads = vi.fn(async (_options?: { isCurrent?: () => boolean }) => undefined)
  const selectThread = vi.fn(async () => undefined)
  const drainQueuedMessages = vi.fn(async () => undefined)
  const recoverActiveTurn = vi.fn(async () => false)
  const sendMessage = vi.fn(async (
    _text: string,
    _mode?: string,
    _overrides?: SendMessageOverrides
  ) => true)

  state = {
    blocks: [],
    goalStateEpoch: 0,
    activeGoalActivation: null,
    activeThreadGoal: initialGoal,
    activeThreadTodos: initialTodos,
    activeThreadId,
    queuedMessages: [],
    queuedMessagePauses: {},
    createThread,
    error: null,
    drainQueuedMessages,
    recoverActiveTurn,
    refreshThreads,
    route: 'chat',
    runtimeConnection: 'ready',
    selectThread,
    sendMessage,
    settingsSection: 'general',
    threadSearchResults: null,
    threads: activeThreadId ? [thread(activeThreadId, initialGoal)] : []
  } as unknown as ChatState

  const set: ChatStoreSet = (partial) => {
    const update = typeof partial === 'function' ? partial(state) : partial
    Object.assign(state, update)
  }
  const get: ChatStoreGet = () => state
  const sseAbortRef = { current: null as AbortController | null }
  const actions = createMaintenanceActions({
    set,
    get,
    sseAbortRef
  })
  state.renameThread = actions.renameThread
  state.renameActiveThread = actions.renameActiveThread

  return { actions, createThread, drainQueuedMessages, get, provider, recoverActiveTurn, refreshThreads, selectThread, sendMessage, sseAbortRef, state }
}

function sourceForkFixture(): { source: TurnForkSource; snapshot: TurnForkSnapshot; retire: () => void } {
  let current = true
  const snapshot: TurnForkSnapshot = { busy: false, latestSeq: 13, title: 'Actual secondary source', blocks: [
    { kind: 'user', id: 'source-user', text: 'SOURCE_HISTORY_ONLY', meta: { turnId: 'source-turn' } },
    { kind: 'assistant', id: 'source-final', text: 'SOURCE_ANSWER_ONLY', meta: { turnId: 'source-turn' },
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64), threadId: 'source-thread',
        turnId: 'source-turn', itemId: 'source-final', firstSeq: 11, lastSeq: 13, status: 'completed' } }
  ], rewoundTurnIds: {} }
  return { snapshot, retire: () => { current = false }, source: {
    threadId: 'source-thread', turnId: 'source-turn', finalItemId: 'source-final', status: 'completed', endSeq: 13,
    proof: { kind: 'general', receipt: { ...(snapshot.blocks[1] as Extract<ChatBlock, { kind: 'assistant' }>).generalTerminalProjectionReceipt! } },
    isCurrent: () => current, getSnapshot: () => snapshot
  } }
}

describe('thread execution policy metadata mutation', () => {
  const readOnly: ThreadExecutionPolicy = { approvalPolicy: 'never', sandboxMode: 'read-only' }
  const full: ThreadExecutionPolicy = { approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }
  const observed = (id: string, policy = readOnly, executionPolicyVersion: 2 | null = 2): NormalizedThread => ({
    ...thread(id), ...policy, executionPolicyVersion: executionPolicyVersion ?? undefined, updatedAt: '2026-10-20T00:01:00Z'
  })
  beforeEach(() => { registryMock.getProvider.mockReset(); invalidateThreadDetailCache(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => { invalidateThreadDetailCache(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

  it('waits for exact metadata, invalidates cached detail and preserves a running turn', async () => {
    const h = buildHarness(), response = deferred<NormalizedThread>()
    h.state.threads = [observed('thr_existing', full)]
    h.provider.getThreadDetail.mockResolvedValueOnce({ blocks: [], latestSeq: 1, thread: h.state.threads[0] })
    await loadThreadDetailWithCache(h.provider as unknown as AgentProvider, 'thr_existing', h.state.threads[0])
    expect(hasFreshThreadDetailCache(h.state.threads[0]!)).toBe(true)
    h.state.busy = true; h.state.currentTurnId = 'frozen-turn'; h.state.currentTurnUserId = 'frozen-user'
    const blocks = h.state.blocks, controller = new AbortController(); h.sseAbortRef.current = controller
    const binding = getThreadBinding(h.sseAbortRef)
    const streamGeneration = binding.generation, submissionGeneration = binding.submissionGeneration
    h.provider.updateThreadExecutionPolicy.mockImplementationOnce(() => response.promise)
    const pending = h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)
    expect(h.state.threads[0]).toMatchObject(full)
    expect(binding.threadPolicyPending.has('thr_existing')).toBe(true)
    expect(hasFreshThreadDetailCache(h.state.threads[0]!)).toBe(false)
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', full)).toBe(false)
    response.resolve(observed('thr_existing'))
    expect(await pending).toBe(true)
    expect(h.provider.updateThreadExecutionPolicy).toHaveBeenCalledExactlyOnceWith('thr_existing', readOnly)
    expect(h.provider.getThreadDetail).toHaveBeenCalledOnce()
    expect(h.state.threads[0]).toMatchObject({ ...readOnly, executionPolicyVersion: 2, executionPolicyUncertain: false })
    expect(h.state.busy).toBe(true); expect(h.state.currentTurnId).toBe('frozen-turn'); expect(h.state.blocks).toBe(blocks)
    expect(controller.signal.aborted).toBe(false)
    expect(binding.generation).toBe(streamGeneration); expect(binding.submissionGeneration).toBe(submissionGeneration)
    expect(binding.threadPolicyPending.has('thr_existing')).toBe(false)
    expect(binding.threadMetadataGeneration).toBe(2); expect(binding.threadMetadataGenerationById.get('thr_existing')).toBe(2)
    expect(binding.threadMetadataGenerationById.has('thr_other')).toBe(false)
  })

  it.each([true, false])('reads the actual owner after an uncertain PATCH and confirms only the exact pair (matches=%s)', async matches => {
    const h = buildHarness()
    h.state.threads = [observed('thr_existing', full)]
    h.provider.updateThreadExecutionPolicy.mockRejectedValueOnce(new Error('Synthetic lost acknowledgement'))
    h.provider.getThreadDetail.mockResolvedValueOnce({ blocks: [], latestSeq: 1,
      thread: observed('thr_existing', matches ? readOnly : full) })
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)).toBe(matches)
    expect(h.provider.getThreadDetail).toHaveBeenCalledExactlyOnceWith('thr_existing')
    expect(h.state.threads[0]).toMatchObject({ ...(matches ? readOnly : full), executionPolicyUncertain: false })
    expect(h.state.error === null).toBe(matches)
  })

  it.each(['PATCH', 'GET'] as const)('does not confirm an unchanged legacy full-access pair from %s without the marker', async seam => {
    const h = buildHarness()
    h.state.threads = [observed('thr_existing', full, null)]
    if (seam === 'PATCH') h.provider.updateThreadExecutionPolicy.mockResolvedValueOnce(observed('thr_existing', full, null))
    else {
      h.provider.updateThreadExecutionPolicy.mockRejectedValueOnce(new Error('Synthetic lost acknowledgement'))
      h.provider.getThreadDetail.mockResolvedValueOnce({ blocks: [], latestSeq: 1, thread: observed('thr_existing', full, null) })
    }
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', full)).toBe(false)
    expect(h.state.threads[0]?.executionPolicyVersion).toBeUndefined()
    expect(h.state.error).not.toBeNull()
  })

  it('retains the displayed pair and blocks authority after both PATCH and readback fail, then permits a real retry', async () => {
    const h = buildHarness()
    h.state.threads = [observed('thr_existing', full)]
    h.provider.updateThreadExecutionPolicy.mockRejectedValueOnce(new Error('Synthetic PATCH outcome unavailable'))
    h.provider.getThreadDetail.mockRejectedValueOnce(new Error('Synthetic readback unavailable'))
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)).toBe(false)
    expect(h.state.threads[0]).toMatchObject({ ...full, executionPolicyUncertain: true })
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)).toBe(true)
    expect(h.state.threads[0]).toMatchObject({ ...readOnly, executionPolicyUncertain: false })
  })

  it('rejects the wrong readback owner rather than clearing an unknown policy', async () => {
    const h = buildHarness()
    h.provider.updateThreadExecutionPolicy.mockRejectedValueOnce(new Error('Synthetic PATCH outcome unavailable'))
    h.provider.getThreadDetail.mockResolvedValueOnce({ blocks: [], latestSeq: 1, thread: observed('thr_other') })
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)).toBe(false)
    expect(h.state.threads[0]?.executionPolicyUncertain).toBe(true)
    expect(h.state.threads).toHaveLength(1)
  })

  it('updates only A metadata when its acknowledgement arrives after selecting B', async () => {
    const h = buildHarness(), response = deferred<NormalizedThread>()
    h.state.threads = [observed('thr_existing', full), observed('thr_other', full)]
    h.provider.updateThreadExecutionPolicy.mockImplementationOnce(() => response.promise)
    const pending = h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)
    h.state.activeThreadId = 'thr_other'; h.state.error = 'B keeps this error'
    getThreadBinding(h.sseAbortRef).generation += 1
    response.resolve(observed('thr_existing'))
    expect(await pending).toBe(true)
    expect(h.state.threads[0]).toMatchObject(readOnly); expect(h.state.threads[1]).toMatchObject(full)
    expect(h.state.activeThreadId).toBe('thr_other'); expect(h.state.error).toBe('B keeps this error')
  })

  it('does not write B error when A policy is changed from the B view', async () => {
    const h = buildHarness()
    h.state.threads = [observed('thr_existing', full), observed('thr_other', full)]
    h.state.activeThreadId = 'thr_other'; h.state.error = 'B keeps this error'
    h.provider.updateThreadExecutionPolicy.mockRejectedValueOnce(new Error('Synthetic A PATCH error'))
    h.provider.getThreadDetail.mockRejectedValueOnce(new Error('Synthetic A readback error'))
    expect(await h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)).toBe(false)
    expect(h.state.error).toBe('B keeps this error')
    expect(h.state.threads[0]?.executionPolicyUncertain).toBe(true)
    expect(h.state.threads[1]?.executionPolicyUncertain).not.toBe(true)
    expect(getThreadBinding(h.sseAbortRef).threadPolicyPending.size).toBe(0)
  })

  it('does not overwrite a replacement A view error after A to B to A', async () => {
    const h = buildHarness(), response = deferred<NormalizedThread>()
    h.provider.updateThreadExecutionPolicy.mockImplementationOnce(() => response.promise)
    const pending = h.actions.updateThreadExecutionPolicy('thr_existing', readOnly)
    h.state.activeThreadId = 'thr_other'; getThreadBinding(h.sseAbortRef).generation += 1
    h.state.activeThreadId = 'thr_existing'; getThreadBinding(h.sseAbortRef).generation += 1
    h.state.error = 'Replacement A keeps this error'
    h.provider.getThreadDetail.mockRejectedValueOnce(new Error('Synthetic readback unavailable'))
    response.reject(new Error('Synthetic stale PATCH error'))
    expect(await pending).toBe(false)
    expect(h.state.error).toBe('Replacement A keeps this error')
    expect(h.state.threads[0]?.executionPolicyUncertain).toBe(true)
  })
})

describe('chat-store-maintenance-actions goal actions', () => {
  it('keeps the captured E when the source thread cursor advances', async () => {
    const { actions, provider } = buildHarness()
    const { source, snapshot } = sourceForkFixture()
    snapshot.latestSeq = 18
    await actions.forkThreadFromTurn('source-turn', source)
    expect(provider.forkThread).toHaveBeenCalledWith('source-thread', { turnId: 'source-turn', endSeq: 13 })
  })

  it('uses the explicit secondary source and its full history for fallback lineage', async () => {
    const { actions, provider, state, selectThread } = buildHarness()
    const { source } = sourceForkFixture()
    Object.assign(state, { busy: false, blocks: [
      { kind: 'user', id: 'main-a', text: 'WRONG_MAIN_A', meta: { turnId: 'main-a' } },
      { kind: 'user', id: 'main-b', text: 'WRONG_MAIN_B', meta: { turnId: 'main-b' } },
      { kind: 'user', id: 'main-c', text: 'WRONG_MAIN_C', meta: { turnId: 'main-c' } }
    ] satisfies ChatBlock[] })
    provider.forkThread.mockImplementationOnce(async (sourceId: string) => ({ ...thread('secondary-fork'),
      forkedFromThreadId: sourceId, forkedAt: '2026-06-04T00:03:00.000Z' }))
    await actions.forkThreadFromTurn('source-turn', source)
    expect(provider.forkThread).toHaveBeenCalledWith('source-thread', { turnId: 'source-turn', endSeq: 13 })
    expect(selectThread).toHaveBeenCalledWith('secondary-fork')
    expect(readThreadForkRegistry().forks['secondary-fork']).toMatchObject({
      parentThreadId: 'source-thread', parentTitle: 'Actual secondary source', forkedFromMessageCount: 2, forkedFromTurnCount: 1
    })
  })
  it.each(['busy', 'rewound', 'missing-final', 'later-transcript', 'stale-cursor', 'mismatched-receipt', 'same-cursor-new-digest', 'retired'] as const)(
    'does not send an explicit source fork for %s', async reason => {
      const { actions, provider } = buildHarness()
      const { source, snapshot, retire } = sourceForkFixture()
      if (reason === 'busy') snapshot.busy = true
      if (reason === 'rewound') snapshot.rewoundTurnIds = { 'source-turn': true }
      if (reason === 'missing-final') snapshot.blocks = snapshot.blocks.slice(0, 1)
      if (reason === 'later-transcript') snapshot.blocks.push({ kind: 'system', id: 'later', text: 'Later tool result', meta: { turnId: 'source-turn' } })
      if (reason === 'stale-cursor') snapshot.latestSeq = 12
      if (reason === 'mismatched-receipt') {
        const final = snapshot.blocks[1] as Extract<ChatBlock, { kind: 'assistant' }>
        final.generalTerminalProjectionReceipt = { ...final.generalTerminalProjectionReceipt!, threadId: 'wrong-thread' }
      }
      if (reason === 'same-cursor-new-digest') {
        const final = snapshot.blocks[1] as Extract<ChatBlock, { kind: 'assistant' }>
        final.generalTerminalProjectionReceipt = { ...final.generalTerminalProjectionReceipt!, batchDigest: 'b'.repeat(64) }
      }
      if (reason === 'retired') retire()
      await actions.forkThreadFromTurn('source-turn', source)
      expect(provider.forkThread).not.toHaveBeenCalled()
    })
  it.each(['source-retired', 'rewind', 'same-cursor-new-digest', 'main-binding-ABA', 'new-submission'] as const)(
    'keeps a created fork from navigating after %s during POST', async reason => {
      const { actions, provider, state, sseAbortRef, refreshThreads, selectThread } = buildHarness()
      const { source, snapshot, retire } = sourceForkFixture()
      const response = deferred<NormalizedThread>()
      provider.forkThread.mockImplementationOnce(() => response.promise)
      const pending = actions.forkThreadFromTurn('source-turn', source)
      expect(provider.forkThread).toHaveBeenCalledOnce()
      if (reason === 'source-retired') retire()
      if (reason === 'rewind') snapshot.rewoundTurnIds = { 'source-turn': true }
      if (reason === 'same-cursor-new-digest') {
        const final = snapshot.blocks[1] as Extract<ChatBlock, { kind: 'assistant' }>
        final.generalTerminalProjectionReceipt = { ...final.generalTerminalProjectionReceipt!, batchDigest: 'b'.repeat(64) }
      }
      if (reason === 'main-binding-ABA') {
        const binding = getThreadBinding(sseAbortRef)
        state.activeThreadId = 'main-other'; binding.generation += 1
        state.activeThreadId = 'thr_existing'; binding.generation += 1
      }
      if (reason === 'new-submission') getThreadBinding(sseAbortRef).submissionGeneration += 1
      response.resolve(thread('created-before-retirement'))
      await pending
      expect(provider.forkThread).toHaveBeenCalledOnce()
      expect(refreshThreads).not.toHaveBeenCalled(); expect(selectThread).not.toHaveBeenCalled()
      expect(readThreadForkRegistry().forks['created-before-retirement']).toBeUndefined()
      expect(state.error).toBeNull()
    })
  it('does not select a fork after the source retires during the existing refresh', async () => {
    const { actions, refreshThreads, selectThread } = buildHarness()
    const { source, retire } = sourceForkFixture()
    const refreshed = deferred<void>()
    refreshThreads.mockImplementationOnce(() => refreshed.promise)
    const pending = actions.forkThreadFromTurn('source-turn', source)
    await Promise.resolve()
    expect(refreshThreads).toHaveBeenCalledOnce()
    retire(); refreshed.resolve(undefined)
    await pending
    expect(selectThread).not.toHaveBeenCalled()
  })
  it('does not write a stale source error when an already sent fork rejects', async () => {
    const { actions, provider, state, selectThread } = buildHarness()
    const { source, retire } = sourceForkFixture()
    const response = deferred<NormalizedThread>()
    provider.forkThread.mockImplementationOnce(() => response.promise)
    const pending = actions.forkThreadFromTurn('source-turn', source)
    retire(); response.reject(new Error('Synthetic rejected fork'))
    await pending
    expect(state.error).toBeNull(); expect(selectThread).not.toHaveBeenCalled()
  })
  beforeEach(() => {
    registryMock.getProvider.mockReset()
    vi.stubGlobal('localStorage', memoryStorage())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('archives the active thread through the runtime route and clears the selected timeline', async () => {
    const { actions, provider, refreshThreads, sseAbortRef, state } = buildHarness()
    const abort = vi.fn()
    Object.assign(state, {
      activeThreadId: 'thr_existing',
      activeThreadGoal: goal('thr_existing'),
      turnUsageById: footerEntries(['turn-1']), rewoundTurnIds: { older_removed: true }, lastTurnUsage: footerLastUsage,
      activeThreadTodos: { threadId: 'thr_existing', items: [] },
      blocks: [{ kind: 'assistant', id: 'assistant-1', text: 'old answer' }],
      busy: true,
      currentTurnId: 'turn-1',
      currentTurnUserId: 'user-1',
      liveAssistant: 'partial',
      lastSeq: 42,
      queuedMessages: [{ threadId: 'thr_existing', id: 'q-1', text: 'queued' }],
      watchTurnCompletion: { thr_existing: true, thr_other: true },
      unreadThreadIds: { thr_existing: true, thr_other: true },
      threads: [
        thread('thr_existing', goal('thr_existing')),
        thread('thr_other')
      ]
    })
    sseAbortRef.current = { abort } as unknown as AbortController

    await actions.archiveThread('thr_existing', true)

    expect(provider.archiveThread).toHaveBeenCalledWith('thr_existing', true)
    expect(provider.deleteThread).not.toHaveBeenCalled()
    expect(abort).toHaveBeenCalledTimes(1)
    expect(sseAbortRef.current).toBeNull()
    expect(state.turnUsageById).toEqual({}); expect(state.rewoundTurnIds).toEqual({}); expect(state.lastTurnUsage).toBeNull()
    expect(state.activeThreadId).toBeNull()
    expect(state.activeThreadGoal).toBeNull()
    expect(state.activeThreadTodos).toBeNull()
    expect(state.blocks).toEqual([])
    expect(state.busy).toBe(false)
    expect(state.currentTurnId).toBeNull()
    expect(state.currentTurnUserId).toBeNull()
    expect(state.liveAssistant).toBe('')
    expect('liveReasoning' in state).toBe(false)
    expect(state.lastSeq).toBe(0)
    expect(state.queuedMessages).toEqual([{ threadId: 'thr_existing', id: 'q-1', text: 'queued' }])
    expect(queuedMessagesForThread(state)).toEqual([])
    expect(state.threads.find((item) => item.id === 'thr_existing')?.archived).toBe(true)
    expect(state.watchTurnCompletion).toEqual({ thr_other: true })
    expect(state.unreadThreadIds).toEqual({ thr_other: true })
    expect(refreshThreads).toHaveBeenCalledTimes(1)
  })

  it('forks the active thread through the runtime route, records lineage, and selects the fork', async () => {
    const { actions, provider, refreshThreads, selectThread, state } = buildHarness()
    Object.assign(state, {
      activeThreadId: 'thr_existing',
      blocks: [
        { kind: 'user', id: 'user-1', text: 'original ask', meta: { turnId: 'turn-1' } },
        { kind: 'assistant', id: 'assistant-1', text: 'original answer' }
      ] satisfies ChatBlock[],
      busy: false,
      threads: [
        { ...thread('thr_existing'), title: 'Parent thread' }
      ]
    })

    await actions.forkActiveThread()

    expect(provider.forkThread).toHaveBeenCalledWith('thr_existing')
    expect(refreshThreads).toHaveBeenCalledExactlyOnceWith()
    expect(selectThread).toHaveBeenCalledWith('thr_fork')
    expect(refreshThreads.mock.invocationCallOrder[0]).toBeLessThan(
      selectThread.mock.invocationCallOrder[0]
    )
    expect(readThreadForkRegistry().forks.thr_fork).toEqual({
      parentThreadId: 'thr_existing',
      parentTitle: 'Parent thread',
      createdAt: '2026-06-04T00:02:00.000Z',
      forkedFromMessageCount: 2,
      forkedFromTurnCount: 1
    })
  })

  it('forks the active thread from a specific turn and records truncated lineage', async () => {
    const { actions, provider, refreshThreads, selectThread, state } = buildHarness()
    provider.forkThread.mockImplementationOnce(async (threadId: string) => ({
      ...thread('thr_fork_turn'),
      forkedFromThreadId: threadId,
      forkedFromTitle: 'Parent thread',
      forkedAt: '2026-06-04T00:03:00.000Z'
    }))
    Object.assign(state, {
      activeThreadId: 'thr_existing',
      blocks: [
        { kind: 'user', id: 'user-1', text: 'first ask', meta: { turnId: 'turn-1' } },
        { kind: 'assistant', id: 'assistant-1', text: 'first answer', meta: { turnId: 'turn-1' } },
        { kind: 'user', id: 'user-2', text: 'second ask', meta: { turnId: 'turn-2' } },
        { kind: 'assistant', id: 'assistant-2', text: 'second answer', meta: { turnId: 'turn-2' } }
      ] satisfies ChatBlock[],
      busy: false,
      threads: [
        { ...thread('thr_existing'), title: 'Parent thread' }
      ]
    })

    await actions.forkThreadFromTurn(' turn-1 ')

    expect(provider.forkThread).toHaveBeenCalledWith('thr_existing', { turnId: 'turn-1' })
    expect(refreshThreads).toHaveBeenCalledExactlyOnceWith()
    expect(selectThread).toHaveBeenCalledWith('thr_fork_turn')
    expect(readThreadForkRegistry().forks.thr_fork_turn).toEqual({
      parentThreadId: 'thr_existing',
      parentTitle: 'Parent thread',
      createdAt: '2026-06-04T00:03:00.000Z',
      forkedFromMessageCount: 2,
      forkedFromTurnCount: 1
    })
  })

  it('resumes a session through the runtime route, refreshes threads, and selects the resumed thread', async () => {
    const { actions, provider, refreshThreads, selectThread } = buildHarness()
    const options = {
      mode: 'plan' as const,
      model: 'glm-5',
      providerId: 'zai-coding-plan'
    }

    const result = await actions.resumeSessionIntoThread('  sess_1  ', options)

    expect(result).toBe('thr_resumed')
    expect(provider.resumeSession).toHaveBeenCalledWith('sess_1', options)
    expect(refreshThreads).toHaveBeenCalledTimes(1)
    expect(selectThread).toHaveBeenCalledWith('thr_resumed')
    expect(refreshThreads.mock.invocationCallOrder[0]).toBeLessThan(
      selectThread.mock.invocationCallOrder[0]
    )
  })

  it('creates a Goal through lifecycle admission without a second ordinary message', async () => {
    const { actions, provider, refreshThreads, sendMessage, state } = buildHarness()

    const result = await actions.setActiveThreadGoal('  ship goal mode  ')

    expect(result).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenCalledWith('thr_existing', {
      objective: 'ship goal mode',
      operation: 'create'
    })
    expect(state.activeThreadGoal).toMatchObject({
      threadId: 'thr_existing',
      objective: 'ship goal mode',
      status: 'active'
    })
    expect(state.threads[0]?.goal).toMatchObject({
      threadId: 'thr_existing',
      objective: 'ship goal mode',
      status: 'active'
    })
    expect(refreshThreads).toHaveBeenCalledTimes(1)
    expect(sendMessage).not.toHaveBeenCalled()
  })

  it('starts a research goal through the existing goal action and runtime patch', async () => {
    const { actions, provider, sendMessage, state } = buildHarness()

    const result = await actions.setActiveThreadGoal('  map cache behavior  ', {
      research: true,
      requirements: ['Compare providers', 'Audit unsupported fallback']
    })

    expect(result).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenCalledWith('thr_existing', {
      objective: 'map cache behavior',
      research: {
        enabled: true,
        requirements: ['Compare providers', 'Audit unsupported fallback']
      }
    })
    expect(state.activeThreadGoal).toMatchObject({
      threadId: 'thr_existing',
      objective: 'map cache behavior',
      status: 'active'
    })
    expect(sendMessage).toHaveBeenCalledWith(
      'map cache behavior',
      'agent',
      expect.objectContaining({
        displayText: expect.stringContaining('map cache behavior')
      })
    )
  })

  it('creates a thread before setting the first goal when no thread is active', async () => {
    const { actions, createThread, provider, sendMessage, state } = buildHarness({
      activeThreadId: null
    })

    const result = await actions.setActiveThreadGoal('ship goal mode')

    expect(result).toBe(true)
    expect(createThread).toHaveBeenCalledTimes(1)
    expect(provider.setThreadGoal).toHaveBeenCalledWith('thr_created', {
      objective: 'ship goal mode',
      operation: 'create'
    })
    expect(createThread.mock.invocationCallOrder[0]).toBeLessThan(
      provider.setThreadGoal.mock.invocationCallOrder[0]
    )
    expect(state.activeThreadId).toBe('thr_created')
    expect(state.activeThreadGoal?.threadId).toBe('thr_created')
    expect(state.threads[0]?.goal?.objective).toBe('ship goal mode')
    expect(sendMessage).not.toHaveBeenCalled()
  })

  it('does not call goal APIs when a new thread cannot be created', async () => {
    const { actions, createThread, provider, sendMessage, state } = buildHarness({
      activeThreadId: null,
      createThreadSucceeds: false
    })

    const result = await actions.setActiveThreadGoal('ship goal mode')

    expect(result).toBe(false)
    expect(createThread).toHaveBeenCalledTimes(1)
    expect(provider.setThreadGoal).not.toHaveBeenCalled()
    expect(sendMessage).not.toHaveBeenCalled()
    expect(state.activeThreadGoal).toBeNull()
  })

  it('updates active goal status and keeps the thread snapshot in sync', async () => {
    const initialGoal = goal('thr_existing', 'finish testing', 'active')
    const { actions, provider, refreshThreads, state } = buildHarness({ initialGoal })

    const result = await actions.setActiveThreadGoalStatus('paused')

    expect(result).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenCalledWith('thr_existing', { status: 'paused' })
    expect(state.activeThreadGoal).toMatchObject({
      threadId: 'thr_existing',
      objective: 'finish testing',
      status: 'paused'
    })
    expect(state.threads[0]?.goal).toMatchObject({
      threadId: 'thr_existing',
      objective: 'finish testing',
      status: 'paused'
    })
    expect(refreshThreads).toHaveBeenCalledTimes(1)
  })

  it('uses counted Goal CAS, preserves paused edits and never sends a legacy resume message', async () => {
    const initialGoal={...goal('thr_existing','finish testing','paused'),id:'goal-exact',revision:7,maxGoalRounds:8,roundsStarted:3}
    const {actions,provider,state,sendMessage}=buildHarness({initialGoal})
    expect(await actions.setActiveThreadGoal('Changed objective',{maxGoalRounds:10})).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenCalledWith('thr_existing',{operation:'edit',ref:{id:'goal-exact',revision:7},objective:'Changed objective',maxGoalRounds:10})
    expect(state.activeThreadGoal?.status).toBe('paused')
    Object.assign(state,{activeThreadGoal:initialGoal})
    expect(await actions.setActiveThreadGoalStatus('active')).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenLastCalledWith('thr_existing',{operation:'resume',ref:{id:'goal-exact',revision:7}})
    Object.assign(state,{activeThreadGoal:goal('thr_existing','Legacy objective','paused')})
    expect(await actions.setActiveThreadGoalStatus('active')).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenLastCalledWith('thr_existing',{status:'active'})
    expect(sendMessage).not.toHaveBeenCalled()
  })

  it('clears a counted Goal using its exact ref and preserves an explicit null fallback', async () => {
    const initialGoal={...goal('thr_existing'),id:'goal-exact',revision:7,maxGoalRounds:8,roundsStarted:3}
    const {actions,provider,state}=buildHarness({initialGoal})
    expect(await actions.clearActiveThreadGoal()).toBe(true)
    expect(provider.clearThreadGoal).toHaveBeenCalledWith('thr_existing',{id:'goal-exact',revision:7})
    expect(state.activeThreadGoal).toBeNull()
    expect(state.activeGoalActivation).toBeNull()
  })

  it('clears the active goal and removes it from the thread snapshot', async () => {
    const initialGoal = goal('thr_existing', 'finish testing', 'active')
    const { actions, provider, refreshThreads, state } = buildHarness({ initialGoal })

    const result = await actions.clearActiveThreadGoal()

    expect(result).toBe(true)
    expect(provider.clearThreadGoal).toHaveBeenCalledWith('thr_existing')
    expect(state.activeThreadGoal).toBeNull()
    expect(state.threads[0]?.goal).toBeNull()
    expect(refreshThreads).toHaveBeenCalledTimes(1)
  })

  it.each(['other-thread', 'same-thread-new-binding'] as const)('does not let a late Goal mutation advance the current epoch after %s', async navigation => {
    const initialGoal = { ...goal('thr_existing'), id: 'goal-exact', revision: 1, maxGoalRounds: 8, roundsStarted: 1 }
    const { actions, provider, state, sseAbortRef } = buildHarness({ initialGoal })
    const committed = deferred<ThreadGoal>()
    provider.setThreadGoal.mockImplementationOnce(() => committed.promise)
    const pending = actions.setActiveThreadGoalStatus('paused')
    expect(provider.setThreadGoal).toHaveBeenCalledOnce()
    const binding = getThreadBinding(sseAbortRef)
    state.activeThreadId = 'thread-b'; binding.generation += 1
    state.goalStateEpoch = 41
    state.blocks = [{ kind: 'user', id: 'b-user', text: 'Current B history' }]
    state.activeThreadGoal = goal('thread-b', 'Current B objective')
    if (navigation === 'same-thread-new-binding') { state.activeThreadId = 'thr_existing'; binding.generation += 1 }
    const currentBlocks = state.blocks, currentGoal = state.activeThreadGoal
    committed.resolve({ ...initialGoal, revision: 2, status: 'paused' })
    expect(await pending).toBe(true)
    expect(state.goalStateEpoch).toBe(41)
    expect(state.blocks).toBe(currentBlocks)
    expect(state.activeThreadGoal).toBe(currentGoal)
    expect(provider.getThreadDetail).not.toHaveBeenCalled()
  })

  it('uses the one GoalState authority without coupling a committed mutation to unrelated detail failure', async () => {
    const { actions, provider, state } = buildHarness()
    const recorded = { ...goal('thr_existing'), id: 'goal-exact', revision: 1, maxGoalRounds: 8, roundsStarted: 0 }
    const readState = vi.fn(async () => ({ goal: recorded, activation: { id: 'goal-exact', revision: 1, activation: 'armed' as const }, historyBlocks: [] }))
    Object.assign(provider, { getThreadGoalState: readState })
    provider.getThreadDetail.mockRejectedValue(new Error('Unrelated detail unavailable'))
    expect(await actions.setActiveThreadGoal('ship goal mode')).toBe(true)
    expect(readState).toHaveBeenCalledExactlyOnceWith('thr_existing')
    expect(provider.getThreadDetail).not.toHaveBeenCalled()
    expect(state.activeThreadGoal).toEqual(recorded)
    expect(state.activeGoalActivation?.activation).toBe('armed')
  })

  it('starts a pending todo without demoting parallel active items', async () => {
    const initialTodos = todos('thr_existing', [
      ['Read context', 'in_progress'],
      ['Implement closure', 'in_progress'],
      ['Verify', 'pending']
    ])
    const { actions, provider, state } = buildHarness({ initialTodos })

    const result = await actions.setActiveThreadTodoStatus('todo_3', 'in_progress')

    expect(result).toBe(true)
    expect(provider.setThreadTodos).toHaveBeenCalledExactlyOnceWith('thr_existing', [
      expect.objectContaining({ id: 'todo_1', content: 'Read context', status: 'in_progress' }),
      expect.objectContaining({ id: 'todo_2', content: 'Implement closure', status: 'in_progress' }),
      expect.objectContaining({ id: 'todo_3', content: 'Verify', status: 'in_progress' })
    ])
    expect(state.activeThreadTodos?.items.map((item) => [item.id, item.status])).toEqual([
      ['todo_1', 'in_progress'],
      ['todo_2', 'in_progress'],
      ['todo_3', 'in_progress']
    ])
    expect(state.threads[0]?.todos).toBe(state.activeThreadTodos)
    expect(initialTodos.items.map((item) => item.status)).toEqual([
      'in_progress', 'in_progress', 'pending'
    ])
  })

  it('posts both active statuses when a title-only Plan edit attaches Plan sources', async () => {
    const initialTodos = todos('thr_existing', [
      ['Read context', 'in_progress'],
      ['Implement closure', 'in_progress']
    ])
    const { actions, state } = buildHarness({ initialTodos })
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      const payload = JSON.parse(body ?? '{}') as { todos: ThreadTodoList['items'] }
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({ todos: {
          ...initialTodos,
          items: payload.todos.map((item) => ({
            ...item,
            createdAt: initialTodos.items[0].createdAt,
            updatedAt: '2026-06-04T00:03:00.000Z'
          }))
        } })
      }
    })
    vi.stubGlobal('window', { analytix: { runtime: { runtimeRequest } } })
    registryMock.getProvider.mockReturnValue(new AnalytixRuntimeProvider())
    const plan = { id: 'plan_1', relativePath: '.analytix/plan/verify.md' }

    await expect(actions.syncPlanTodosFromMarkdown(
      plan, '# Revised plan\n- [ ] Read context\n- [ ] Implement closure'
    )).resolves.toBe(true)

    expect(runtimeRequest).toHaveBeenCalledExactlyOnceWith(
      '/v1/threads/thr_existing/todos', 'POST', expect.any(String)
    )
    const payload = JSON.parse(runtimeRequest.mock.calls[0][2] ?? '{}') as { todos: ThreadTodoList['items'] }
    expect(payload.todos).toEqual([
      expect.objectContaining({ id: 'todo_1', content: 'Read context', status: 'in_progress', source: expect.objectContaining({ kind: 'plan', planId: plan.id }) }),
      expect.objectContaining({ id: 'todo_2', content: 'Implement closure', status: 'in_progress', source: expect.objectContaining({ kind: 'plan', planId: plan.id }) })
    ])
    expect(state.activeThreadTodos?.items.map((item) => item.status)).toEqual(['in_progress', 'in_progress'])
    expect(state.threads[0]?.todos).toBe(state.activeThreadTodos)
    expect(initialTodos.items.map((item) => item.status)).toEqual(['in_progress', 'in_progress'])

    await expect(actions.syncPlanTodosFromMarkdown(
      plan, '# Revised plan\n- [x] Read context\n- [ ] Implement closure'
    )).resolves.toBe(true)

    expect(runtimeRequest).toHaveBeenCalledTimes(2)
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      2, '/v1/threads/thr_existing/todos', 'POST', expect.any(String)
    )
    const completedPayload = JSON.parse(runtimeRequest.mock.calls[1][2] ?? '{}') as { todos: ThreadTodoList['items'] }
    expect(completedPayload.todos).toEqual([
      expect.objectContaining({ id: 'todo_1', content: 'Read context', status: 'completed' }),
      expect.objectContaining({ id: 'todo_2', content: 'Implement closure', status: 'in_progress' })
    ])
    expect(state.activeThreadTodos?.items.map((item) => item.status)).toEqual(['completed', 'in_progress'])
  })

  it('skips a title-only Plan write when both active todos already have the same Plan source', async () => {
    const plan = { id: 'plan_1', relativePath: '.analytix/plan/verify.md' }
    const initialTodos: ThreadTodoList = {
      threadId: 'thr_existing',
      updatedAt: '2026-06-04T00:02:00.000Z',
      items: extractPlanTodos({
        threadId: 'thr_existing',
        planId: plan.id,
        relativePath: plan.relativePath,
        markdown: '# Original plan\n- [ ] Read context\n- [ ] Implement closure',
        now: '2026-06-04T00:00:00.000Z'
      }).map((item) => ({ ...item, status: 'in_progress' }))
    }
    const { actions, provider, state } = buildHarness({ initialTodos })

    await expect(actions.syncPlanTodosFromMarkdown(
      plan, '# Revised plan\n- [ ] Read context\n- [ ] Implement closure'
    )).resolves.toBe(true)

    expect(provider.setThreadTodos).not.toHaveBeenCalled()
    expect(state.activeThreadTodos).toBe(initialTodos)
    expect(state.activeThreadTodos?.items.map((item) => item.status)).toEqual(['in_progress', 'in_progress'])
  })

  it('retains parallel todo snapshots when a status update is rejected', async () => {
    const initialTodos = todos('thr_existing', [
      ['Read context', 'in_progress'],
      ['Implement closure', 'in_progress'],
      ['Verify', 'pending']
    ])
    const { actions, provider, state } = buildHarness({ initialTodos })
    state.threads[0].todos = initialTodos
    const response = deferred<ThreadTodoList>()
    provider.setThreadTodos.mockReturnValueOnce(response.promise)

    const pending = actions.setActiveThreadTodoStatus('todo_3', 'in_progress')

    expect(provider.setThreadTodos).toHaveBeenCalledExactlyOnceWith('thr_existing', [
      expect.objectContaining({ id: 'todo_1', content: 'Read context', status: 'in_progress' }),
      expect.objectContaining({ id: 'todo_2', content: 'Implement closure', status: 'in_progress' }),
      expect.objectContaining({ id: 'todo_3', content: 'Verify', status: 'in_progress' })
    ])
    expect(state.activeThreadTodos).toBe(initialTodos)
    expect(state.threads[0].todos).toBe(initialTodos)
    expect(state.error).toBeNull()

    response.reject(new Error('Synthetic Todo update rejected'))

    expect(await pending).toBe(false)
    expect(state.activeThreadTodos).toBe(initialTodos)
    expect(state.threads[0].todos).toBe(initialTodos)
    expect(state.activeThreadTodos?.items.map((item) => [item.id, item.status])).toEqual([
      ['todo_1', 'in_progress'],
      ['todo_2', 'in_progress'],
      ['todo_3', 'pending']
    ])
    expect(state.error).toBe(i18n.t('common:runtimeRequestFailed'))
  })

  it('clears active todos and syncs the thread snapshot', async () => {
    const initialTodos = todos('thr_existing', [
      ['Read context', 'completed'],
      ['Verify', 'in_progress']
    ])
    const { actions, provider, state } = buildHarness({ initialTodos })

    const result = await actions.clearActiveThreadTodos()

    expect(result).toBe(true)
    expect(provider.clearThreadTodos).toHaveBeenCalledWith('thr_existing')
    expect(state.activeThreadTodos).toBeNull()
    expect(state.threads[0]?.todos).toBeNull()
  })

  it('requires explicit audited operations for terminal transitions and retry', async () => {
    const initialTodos = todos('thr_existing', [['Run verification', 'failed']])
    initialTodos.items[0].statusReasonCode = 'verification_failed'
    const { actions, provider, state } = buildHarness({ initialTodos })

    await expect(actions.setActiveThreadTodoStatus('todo_1', 'in_progress')).resolves.toBe(false)

    expect(provider.setThreadTodos).not.toHaveBeenCalled()
    expect(state.activeThreadTodos?.items[0]).toMatchObject({
      status: 'failed',
      statusReasonCode: 'verification_failed'
    })
    expect(state.error).toBe(i18n.t('common:todoExplicitTransitionRequired'))
  })

  it('retains failed and canceled todos instead of clearing their audit record', async () => {
    const initialTodos = todos('thr_existing', [['Run verification', 'canceled']])
    initialTodos.items[0].statusReasonCode = 'user_canceled'
    const { actions, provider, state } = buildHarness({ initialTodos })

    await expect(actions.clearActiveThreadTodos()).resolves.toBe(false)

    expect(provider.clearThreadTodos).not.toHaveBeenCalled()
    expect(state.activeThreadTodos?.items[0]).toMatchObject({
      status: 'canceled',
      statusReasonCode: 'user_canceled'
    })
    expect(state.error).toBe(i18n.t('common:todoTerminalAuditRetained'))
  })

  it('settles local runtime work before the backend interrupt resolves', async () => {
    const { actions, provider, recoverActiveTurn, refreshThreads, state } = buildHarness()
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-1', text: 'run command' },
      {
        kind: 'tool',
        id: 'tool-1',
        summary: 'Running command',
        status: 'running',
        toolKind: 'command_execution'
      },
      {
        kind: 'approval',
        id: 'approval-1',
        approvalId: 'approval-1',
        summary: 'Approve command',
        status: 'pending'
      },
      {
        kind: 'user_input',
        id: 'input-1',
        requestId: 'input-1',
        questions: [],
        status: 'pending'
      }
    ]
    Object.assign(state, {
      blocks,
      busy: true,
      currentTurnId: 'turn-1',
      currentTurnUserId: 'user-1',
      liveAssistant: 'partial answer',
      queuedMessages: [],
      turnStartedAtByUserId: { 'user-1': Date.now() - 1000 },
      turnDurationByUserId: {},
    })
    resetActiveStream('thr_existing', {
      turnId: 'turn-1',
      liveAssistant: 'partial answer',
      lastSeq: 12
    })
    let busyWhenBackendCalled: boolean | null = null
    let inputStatusWhenBackendCalled: string | undefined
    provider.interruptTurn.mockImplementation(async () => {
      busyWhenBackendCalled = state.busy
      inputStatusWhenBackendCalled = state.blocks.flatMap(block => block.kind === 'user_input' ? [block.status] : [])[0]
    })
    provider.getThreadDetail.mockResolvedValueOnce({
      blocks: [
        { kind: 'user', id: 'user-1', text: 'run command', meta: { turnId: 'turn-1' } },
        {
          kind: 'tool',
          id: 'tool-1',
          summary: 'Running command',
          status: 'error',
          toolKind: 'command_execution',
          meta: { turnId: 'turn-1' }
        },
        {
          kind: 'approval',
          id: 'approval-1',
          approvalId: 'approval-1',
          summary: 'Approve command',
          status: 'error',
          meta: { turnId: 'turn-1' }
        },
        {
          kind: 'user_input',
          id: 'input-1',
          requestId: 'input-1',
          questions: [],
          status: 'cancelled',
          meta: { turnId: 'turn-1' }
        }
      ],
      latestSeq: 13,
      latestTurnId: 'turn-1',
      threadStatus: 'aborted'
    })

    await actions.interrupt()

    expect(provider.interruptTurn).toHaveBeenCalledWith('thr_existing', 'turn-1', { discard: false })
    expect(busyWhenBackendCalled).toBe(false)
    expect(inputStatusWhenBackendCalled).toBe('error')
    expect(state.busy).toBe(false)
    expect(state.currentTurnId).toBeNull()
    expect(state.currentTurnUserId).toBeNull()
    expect(state.liveAssistant).toBe('')
    expect(state.blocks.map((block) => ('status' in block ? block.status : block.kind))).toEqual([
      'user',
      'error',
      'error',
      'cancelled'
    ])
    expect(getActiveStreamSnapshotFor('thr_existing', 'turn-1').liveAssistant).toBe('')
    expect(refreshThreads).toHaveBeenCalledTimes(1)
    expect(recoverActiveTurn).not.toHaveBeenCalled()
  })

  it('preserves display-only history when interrupt replaces the current primary', async () => {
    const { actions, state } = buildHarness()
    const provider = new AnalytixRuntimeProvider()
    registryMock.getProvider.mockReturnValue(provider)
    vi.spyOn(provider, 'interruptTurn').mockResolvedValue(undefined)
    const primary = { blocks: [
      { kind: 'user' as const, id: 'current-user', text: 'Current request', meta: { turnId: 'current-turn' } },
      { kind: 'compaction' as const, id: 'compact-item', status: 'success' as const, summary: 'Context compacted', meta: { turnId: 'compact-turn' } }
    ], latestSeq: 4, latestTurnId: 'current-turn', latestUserMessageId: 'current-user', threadStatus: 'aborted' }
    const replay = createOrdinaryHistoryReplay('thr_existing', 4)
    const timestamp = '2026-10-08T00:00:00Z'
    replay.accept([{ kind: 'item_created', seq: 1, timestamp, threadId: 'thr_existing', turnId: 'compressed-turn',
      itemId: 'compressed-user', item: { id: 'compressed-user', threadId: 'thr_existing', turnId: 'compressed-turn',
        kind: 'user_message', role: 'user', status: 'completed', createdAt: timestamp, text: 'Authorized old display' } },
      { kind: 'compaction_completed', seq: 2, timestamp, threadId: 'thr_existing', turnId: 'compact-turn', summary: 'Context compacted' },
      { kind: 'cursor_advanced', seq: 4, timestamp, threadId: 'thr_existing' }])
    const displayed = replay.mergeSnapshot(primary.blocks)
    vi.spyOn(provider, 'getThreadDetail').mockResolvedValue(primary)
    const restore = vi.spyOn(provider, 'restoreThreadDisplayHistory').mockResolvedValue({ ...primary, blocks: displayed })
    Object.assign(state, { blocks: displayed, lastSeq: 4, busy: true, currentTurnId: 'current-turn',
      currentTurnUserId: 'current-user', liveAssistant: 'provisional', queuedMessages: [],
      turnStartedAtByUserId: {}, turnDurationByUserId: {} })
    await actions.interrupt()
    expect(restore).toHaveBeenCalledWith('thr_existing', primary, undefined)
    expect(state.blocks.find(block => block.id === 'compressed-user')).toMatchObject({ text: 'Authorized old display' })
    expect(state.blocks.filter(isOrdinaryDisplayHistoryBlock).map(block => block.id)).toEqual(['compressed-user'])
    expect(state.lastSeq).toBe(4); expect(state.currentTurnId).toBeNull(); expect(state.busy).toBe(false)
    expect((await provider.getThreadDetail('thr_existing')).blocks.some(isOrdinaryDisplayHistoryBlock)).toBe(false)
  })

  it('keeps a verified primary quarantined after required replay failure until the current owner restores it', async () => {
    const threadId = 'thr_interrupt_replay_failure', turnId = 'turn_27', userId = 'current-user'
    const timestamp = '2026-10-08T00:00:00Z'
    const { actions, state, get, sseAbortRef, refreshThreads, drainQueuedMessages } = buildHarness({ activeThreadId: threadId })
    Object.assign(state, { composerPickList: [], composerModelGroups: [], watchTurnCompletion: {}, unreadThreadIds: {},
      lastSeq: 0, busy: false, currentTurnId: null, currentTurnUserId: null, liveAssistant: '',
      turnStartedAtByUserId: {}, turnDurationByUserId: {}, turnUsageById: {}, rewoundTurnIds: {} })
    const set: ChatStoreSet = partial => Object.assign(state, typeof partial === 'function' ? partial(state) : partial)
    const threadActions = createThreadActions({ set, get, sseAbortRef })
    const provider = new AnalytixRuntimeProvider()
    registryMock.getProvider.mockReturnValue(provider)
    const primaryFields = { id: threadId, title: 'Synthetic interrupt replay', mode: 'agent',
      model: 'deepseek-chat', workspace: '/workspace/analytix', status: 'idle', createdAt: timestamp, updatedAt: timestamp,
      pendingApprovalIds: [], pendingUserInputIds: [] }
    let primary = ThreadDetailResponseV1Schema.parse({ ...primaryFields, latestSeq: 0, messageCount: 0, turnCount: 0, turns: [] })
    const runtimeRequest = vi.fn(async (path: string, method?: string) => {
      if (path === `/v1/threads/${threadId}` && method === 'GET') {
        return { ok: true, status: 200, body: JSON.stringify(ThreadDetailResponseV1Schema.parse(primary)) }
      }
      if (path === `/v1/threads/${threadId}/turns/${turnId}/interrupt` && method === 'POST') {
        return { ok: true, status: 200, body: '{}' }
      }
      throw new Error('Unexpected synthetic interrupt fixture route.')
    })
    const data = new Set<(payload: SseEventPayload) => void>()
    const ends = new Set<(payload: SseEndPayload) => void>()
    const errors = new Set<(payload: SseErrorPayload) => void>()
    type Replay = { streamId: string; data: (payload: SseEventPayload) => void;
      end: (payload: SseEndPayload) => void; error: (payload: SseErrorPayload) => void }
    const starts = [deferred<Replay>(), deferred<Replay>()]
    const replayProbes: Replay[] = []
    const ownedActions: Promise<unknown>[] = []
    let started = 0
    const firstAck = deferred<void>()
    const ackSseEvent = vi.fn(async (_id: string, seq: number) => { if (seq === 1) firstAck.resolve(); return true })
    const stopSse = vi.fn(async () => true)
    const logError = vi.fn(async () => undefined)
    const startSse = vi.fn(async (_id: string, _since: number, streamId: string | undefined, replay: unknown) => {
      expect(replay).toEqual({ replayThroughSeq: 4 })
      const listeners = { streamId: streamId!, data: [...data].at(-1)!, end: [...ends].at(-1)!, error: [...errors].at(-1)! }
      replayProbes.push(listeners)
      starts[started++].resolve(listeners)
      return { streamId: streamId! }
    })
    vi.stubGlobal('window', { analytix: { logs: { error: logError }, runtime: {
      runtimeRequest, startSse, ackSseEvent, stopSse,
      onSseEvent: (fn: (payload: SseEventPayload) => void) => { data.add(fn); return () => { data.delete(fn) } },
      onSseEnd: (fn: (payload: SseEndPayload) => void) => { ends.add(fn); return () => { ends.delete(fn) } },
      onSseError: (fn: (payload: SseErrorPayload) => void) => { errors.add(fn); return () => { errors.delete(fn) } }
    } } })
    const getDetail = vi.spyOn(provider, 'getThreadDetail')
    const oldStreamFinished = deferred<void>(), newStreamFinished = deferred<void>()
    let oldSink: ThreadEventSink | undefined
    vi.spyOn(provider, 'subscribeThreadEvents')
      .mockImplementationOnce(async (_id, _since, sink) => { oldSink = sink; return oldStreamFinished.promise })
      .mockImplementation(async () => newStreamFinished.promise)
    const replayEvents: SseEventPayload['events'] = [
      { kind: 'item_created', seq: 1, timestamp, threadId, turnId: 'compressed-turn', itemId: 'compressed-user',
        item: { id: 'compressed-user', threadId, turnId: 'compressed-turn', kind: 'user_message', role: 'user',
          status: 'completed', createdAt: timestamp, text: 'Authorized old display' } },
      { kind: 'cursor_advanced', seq: 4, timestamp, threadId, reason: 'restricted_content_removed' }
    ]
    const witness = () => JSON.stringify({ blocks: state.blocks, busy: state.busy, turn: state.currentTurnId,
      user: state.currentTurnUserId, lastSeq: state.lastSeq, live: state.liveAssistant, error: state.error })
    try {
      // Use the production subscription wrapper, but hold the old live provider
      // promise so its actual finally can run after a replacement binds.
      await threadActions.selectThread(threadId)
      const oldController = sseAbortRef.current!
      expect(oldSink).toBeDefined()
      const oldFinally = deferred<void>()
      const abort = oldController.abort.bind(oldController)
      let abortCalls = 0
      vi.spyOn(oldController, 'abort').mockImplementation(() => {
        abort(); if (++abortCalls === 2) oldFinally.resolve()
      })
      const user: ChatBlock = { kind: 'user', id: userId, text: 'Current request', meta: { turnId } }
      const summary = 'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.'
      const marker: ChatBlock = { kind: 'compaction', id: 'compact-item', status: 'success', summary, meta: { turnId: 'compact-turn' } }
      Object.assign(state, { blocks: [marker, user, { kind: 'assistant', id: 'unverified-draft', text: 'Unverified draft', meta: { turnId } }],
        lastSeq: 3, busy: true, currentTurnId: turnId, currentTurnUserId: userId, liveAssistant: 'Unverified live draft',
        queuedMessages: [], turnStartedAtByUserId: {}, turnDurationByUserId: {} })
      resetActiveStream(threadId, { turnId, liveAssistant: state.liveAssistant, lastSeq: 3 })
      primary = ThreadDetailResponseV1Schema.parse({ ...primaryFields, latestSeq: 4, latestTurnId: turnId, messageCount: 1, turnCount: 2, turns: [
        { id: 'compact-turn', threadId, status: 'completed', prompt: '/compact', createdAt: timestamp, finishedAt: timestamp,
          items: [{ id: marker.id, threadId, turnId: 'compact-turn', kind: 'compaction', role: 'system', status: 'completed',
            createdAt: timestamp, finishedAt: timestamp, summary, replacedTokens: 100, pinnedConstraints: ['user: preserve recent turns'] }] },
        { id: turnId, threadId, status: 'aborted', prompt: 'Current request', createdAt: timestamp, finishedAt: timestamp,
          items: [{ id: userId, threadId, turnId, kind: 'user_message',
          role: 'user', status: 'completed', createdAt: timestamp, text: 'Current request' }] }
      ] })
      const interrupted = actions.interrupt()
      ownedActions.push(interrupted)
      const failedReplay = await starts[0].promise
      expect(oldController.signal.aborted).toBe(true)
      const freshRead = getDetail.mock.results.at(-1)!
      if (freshRead.type !== 'return') throw new Error('Synthetic primary GET did not return.')
      const fresh = await freshRead.value
      expect(fresh).toMatchObject({ latestSeq: 4, latestTurnId: turnId, latestUserMessageId: userId, threadStatus: 'idle' })
      expect(fresh.authoritativeTurnStatuses?.[turnId]).toBe('aborted')
      expect(fresh.blocks.some(block => block.kind === 'compaction')).toBe(true)
      failedReplay.data({ streamId: failedReplay.streamId, events: [replayEvents[0]] })
      await firstAck.promise
      // This is the same public failure channel used by Main's start timeout;
      // it is a deterministic transport negative, not a timed native/UI test.
      failedReplay.error({ streamId: failedReplay.streamId, code: 'sse_stream_error' })
      await interrupted
      expect(state).toMatchObject({ busy: false, currentTurnId: turnId, currentTurnUserId: userId,
        error: i18n.t('common:runtimeFinalSnapshotUnavailable'), lastSeq: 3, liveAssistant: '' })
      expect(state.blocks.map(block => block.id)).toEqual([marker.id, userId])
      expect(getActiveStreamSnapshotFor(threadId, turnId).liveAssistant).toBe('')
      expect(refreshThreads).not.toHaveBeenCalled()
      expect(drainQueuedMessages).not.toHaveBeenCalled()
      expect(logError).toHaveBeenCalledWith('turn-terminal-reconcile', 'Failed to load trusted terminal snapshot',
        { message: 'Public history replay is unavailable.', threadId, turnId })
      expect([data.size, ends.size, errors.size]).toEqual([0, 0, 0])
      expect(stopSse).toHaveBeenCalledWith(failedReplay.streamId)
      const quarantined = witness()
      failedReplay.data({ streamId: failedReplay.streamId, events: replayEvents })
      failedReplay.end({ streamId: failedReplay.streamId })
      oldSink!.onDeltas([{ kind: 'agent_message', text: 'Late old draft', seq: 99, turnId }])
      await oldSink!.onTurnComplete({ threadId, turnId, seq: 99 })
      expect(witness()).toBe(quarantined)

      const recovering = threadActions.recoverActiveTurn()
      ownedActions.push(recovering)
      const replacement = sseAbortRef.current!
      const recoveredReplay = await starts[1].promise
      const recoveryGeneration = getThreadBinding(sseAbortRef).generation
      const pendingRecovery = witness()
      oldStreamFinished.resolve()
      await oldFinally.promise
      expect(sseAbortRef.current).toBe(replacement)
      expect(replacement.signal.aborted).toBe(false)
      expect(getThreadBinding(sseAbortRef).generation).toBe(recoveryGeneration)
      failedReplay.error({ streamId: failedReplay.streamId, code: 'sse_stream_error' })
      failedReplay.data({ streamId: failedReplay.streamId, events: replayEvents })
      failedReplay.end({ streamId: failedReplay.streamId })
      expect(witness()).toBe(pendingRecovery)
      expect(state.currentTurnId).toBe(turnId)
      recoveredReplay.data({ streamId: recoveredReplay.streamId, events: replayEvents })
      recoveredReplay.end({ streamId: recoveredReplay.streamId })
      await recovering
      expect(state).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null, error: null, lastSeq: 4, liveAssistant: '' })
      expect(state.blocks.filter(block => block.id === 'compressed-user')).toHaveLength(1)
      expect(isOrdinaryDisplayHistoryBlock(state.blocks.find(block => block.id === 'compressed-user')!)).toBe(true)
      expect(state.blocks.some(block => block.id === 'unverified-draft')).toBe(false)
      expect(getDetail).toHaveBeenCalledTimes(3)
      expect(startSse).toHaveBeenCalledTimes(2)
      expect([data.size, ends.size, errors.size]).toEqual([0, 0, 0])
    } finally {
      sseAbortRef.current?.abort()
      for (const replay of replayProbes) replay.error({ streamId: replay.streamId, code: 'sse_stream_error' })
      oldStreamFinished.resolve(); newStreamFinished.resolve()
      clearBusyWatchdog(); stopTurnCompletionPoll(); clearActiveStream(threadId)
      await Promise.allSettled(ownedActions)
      await vi.waitFor(() => expect(sseAbortRef.current).toBeNull())
    }
  })

  it('keeps the draft quarantined and blocks when interrupt reconciliation fails', async () => {
    ;(globalThis as { window?: unknown }).window = {
      analytix: {
        logs: { error: vi.fn(async () => undefined) }
      }
    }
    try {
      const { actions, provider, recoverActiveTurn, state } = buildHarness()
      Object.assign(state, {
        blocks: [{ kind: 'user', id: 'user-1', text: 'run command' }],
        busy: true,
        currentTurnId: 'turn-1',
        currentTurnUserId: 'user-1',
        liveAssistant: '',
        queuedMessages: [],
        turnStartedAtByUserId: {},
        turnDurationByUserId: {},
      })
      provider.interruptTurn.mockRejectedValueOnce(new Error('runtime timeout'))
      provider.getThreadDetail.mockRejectedValueOnce(new Error('runtime unavailable'))

      await actions.interrupt()

      expect(state.busy).toBe(false)
      expect(state.currentTurnId).toBe('turn-1')
      expect(state.error).toBe(i18n.t('common:runtimeFinalSnapshotUnavailable'))
      expect(recoverActiveTurn).not.toHaveBeenCalled()
    } finally {
      delete (globalThis as { window?: unknown }).window
    }
  })

  it('serializes user-input submit and cancel while a host response is pending', async () => {
    vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } })
    const { actions, provider, state } = buildHarness()
    let complete!: () => void
    const submit = vi.fn(() => new Promise<void>(resolve => { complete = resolve }))
    const cancel = vi.fn(async () => undefined)
    Object.assign(provider, { submitUserInputResponse: submit, cancelUserInput: cancel })
    Object.assign(state, { busy: false, blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), questions: [], status: 'pending' }] })
    const first = actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: state.activeThreadId }, { kind: 'submit', answers: [] })
    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: state.activeThreadId }, { kind: 'cancel' })
    expect(submit).toHaveBeenCalledOnce()
    expect(cancel).not.toHaveBeenCalled()
    complete()
    await first
    expect(state.blocks[0]).toMatchObject({ status: 'submitted' })
  })

  it('keeps a resolved user-input state when its older HTTP response fails', async () => {
    vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } })
    const { actions, provider, state } = buildHarness()
    let fail!: (error: Error) => void
    Object.assign(provider, { submitUserInputResponse: vi.fn(() => new Promise<void>((_resolve, reject) => { fail = reject })) })
    Object.assign(state, { busy: false, blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), questions: [], status: 'pending' }] })
    const pending = actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: state.activeThreadId }, { kind: 'submit', answers: [] })
    state.blocks = state.blocks.map(block => ({ ...block, status: 'submitted' })) as ChatBlock[]
    fail(new Error('host already resolved'))
    await pending
    expect(state.blocks[0]).toMatchObject({ status: 'submitted' })
    expect(state.error).toBeNull()
  })

  it('does not report an old user-input HTTP failure on a different thread', async () => {
    vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } })
    const { actions, provider, state } = buildHarness()
    let fail!: (error: Error) => void
    Object.assign(provider, { submitUserInputResponse: vi.fn(() => new Promise<void>((_resolve, reject) => { fail = reject })) })
    Object.assign(state, { busy: false, blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), questions: [], status: 'pending' }] })
    const pending = actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: state.activeThreadId }, { kind: 'submit', answers: [] })
    Object.assign(state, { activeThreadId: 'thr_other', blocks: [], error: null })
    fail(new Error('old thread HTTP failure'))
    await pending
    expect(state.error).toBeNull()
    expect(state.blocks).toEqual([])
  })

  it('serializes approval decisions and never overwrites a resolved approval with a late error', async () => {
    vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } })
    const { actions, provider, state } = buildHarness()
    let fail!: (error: Error) => void
    const submit = vi.fn(() => new Promise<void>((_resolve, reject) => { fail = reject }))
    Object.assign(provider, { submitApprovalDecision: submit })
    Object.assign(state, { blocks: [{ kind: 'approval', id: 'approval-1', approvalId: 'gate-1', status: 'pending', summary: 'Synthetic operation' }] })
    const first = actions.resolveApproval('approval-1', 'allow')
    const duplicate = actions.resolveApproval('approval-1', 'deny')
    expect(submit).toHaveBeenCalledOnce()
    state.blocks = state.blocks.map(block => ({ ...block, status: 'allowed' })) as ChatBlock[]
    fail(new Error('host already resolved'))
    await Promise.all([first, duplicate])
    expect(state.blocks[0]).toMatchObject({ status: 'allowed' })
    expect(state.error).toBeNull()
  })

  it('submits exact user-input answers to the host but persists only ordinary projections', async () => {
    const account = '6222020202020202020'
    vi.stubGlobal('window', {
      analytix: { logs: { error: vi.fn(async () => undefined) } }
    })
    const { actions, provider, state } = buildHarness()
    const submitUserInputResponse = vi.fn(async () => undefined)
    Object.assign(provider, { submitUserInputResponse })
    Object.assign(state, {
      busy: true,
      blocks: [{
        kind: 'user_input', meta: { turnId: 'turn-question-owner' },
        id: 'item_' + nativeRequestId('f'),
        requestId: nativeRequestId('f'),
        questions: [],
        status: 'pending'
      }]
    })
    const answers = [{ id: 'account', label: `银行卡号 ${account}`, value: account }]

    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('f'), requestId: nativeRequestId('f'), threadId: state.activeThreadId }, { kind: 'submit', answers })

    expect(submitUserInputResponse).toHaveBeenCalledWith(nativeRequestId('f'), answers, nativeQuestionSelector(nativeRequestId('f'), state.activeThreadId))
    expect(state.blocks).toEqual([
      expect.objectContaining({
        kind: 'user_input',
        status: 'submitted',
        answers: [{ id: 'account', label: '银行卡号 [ACCOUNT]', value: '[ACCOUNT]' }]
      })
    ])
    expect(JSON.stringify(state.blocks)).not.toContain(account)
  })

  it('never converts an unsupported user-input submission into a queued model prompt', async () => {
    const account = '6222020202020202020'
    const logError = vi.fn(async () => undefined)
    vi.stubGlobal('window', { analytix: { logs: { error: logError } } })
    const { actions, state } = buildHarness()
    Object.assign(state, {
      queuedMessages: [],
      blocks: [{
        kind: 'user_input', meta: { turnId: 'turn-question-owner' },
        id: 'item_' + nativeRequestId('b'),
        requestId: nativeRequestId('b'),
        questions: [],
        status: 'pending'
      }]
    })

    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('b'), requestId: nativeRequestId('b'), threadId: state.activeThreadId }, {
      kind: 'submit',
      answers: [{ id: 'account', label: account, value: account }]
    })

    expect(state.queuedMessages).toEqual([])
    expect(JSON.stringify(state)).not.toContain(account)
    expect(state.blocks).toEqual([
      expect.objectContaining({ kind: 'user_input', status: 'error' })
    ])
    expect(logError).toHaveBeenCalled()
  })

  it('restores a git checkpoint before rewinding and resending a turn', async () => {
    const restoreGitCheckpoint = vi.fn(async () => ({
      ok: true,
      checkpointId: 'gcp_1',
      repositoryRoot: '/workspace/analytix',
      head: 'abc',
      currentBranch: 'main',
      rescueCheckpointId: null
    }))
    vi.stubGlobal('window', {
      analytix: {
        workspace: { restoreGitCheckpoint },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, provider, sendMessage, state } = buildHarness()
    Object.assign(state, {
      busy: false,
      activeThreadId: 'thr_existing',
      blocks: [
        {
          kind: 'user',
          id: 'user_1',
          text: 'old text',
          meta: { turnId: 'turn_1', workspaceCheckpointId: 'gcp_1' }
        },
        { kind: 'assistant', id: 'assistant_1', text: 'old answer' }
      ],
      turnStartedAtByUserId: {},
      turnDurationByUserId: {},
      queuedMessages: []
    })

    await actions.rewindAndResend('user_1', 'new text')

    expect(restoreGitCheckpoint).toHaveBeenCalledWith({ checkpointId: 'gcp_1' })
    expect(provider.rewindThread).toHaveBeenCalledWith('thr_existing', 'turn_1')
    expect(restoreGitCheckpoint.mock.invocationCallOrder[0]).toBeLessThan(
      provider.rewindThread.mock.invocationCallOrder[0]
    )
    expect(sendMessage).toHaveBeenCalledWith('new text', undefined, expect.objectContaining({ attachmentIds: [], attachments: [], fileReferences: [] }))
  })

  it('resends unchanged text exactly once after the rewind event precedes the successful HTTP response', async () => {
    const { actions, get, provider, sendMessage, state } = buildHarness()
    const originalText = 'retry this unchanged request'
    state.turnUsageById = footerEntries(['turn_retained', 'turn_failed'])
    state.rewoundTurnIds = { previously_removed: true }; state.lastTurnUsage = footerLastUsage
    const retained: ChatBlock = { kind: 'user', id: 'user_retained', text: 'earlier request', meta: { turnId: 'turn_retained' } }
    Object.assign(state, {
      busy: false,
      blocks: [
        retained,
        { kind: 'user', id: 'user_failed', text: originalText, meta: { turnId: 'turn_failed' } },
        { kind: 'tool', id: 'error_failed', summary: 'Provider unavailable', status: 'error', meta: { turnId: 'turn_failed' } }
      ],
      liveAssistant: '',
      lastSeq: 10,
      currentTurnId: null,
      currentTurnUserId: null,
      turnStartedAtByUserId: { user_retained: 1, user_failed: 2 },
      turnDurationByUserId: { user_failed: 3 },
      queuedMessages: []
    })
    const set: ChatStoreSet = (partial) => {
      Object.assign(state, typeof partial === 'function' ? partial(state) : partial)
    }
    const sink = buildThreadEventSink(set, get, { threadId: 'thr_existing' })
    let completeHTTP!: () => void
    provider.rewindThread.mockImplementationOnce(async () => {
      sink.onThreadRewound?.({
        threadId: 'thr_existing', turnId: 'turn_failed', removedTurns: 1,
        remainingTurns: 1, removedTurnIds: ['turn_failed'], seq: 11
      })
      await new Promise<void>((resolve) => { completeHTTP = resolve })
    })

    const pending = actions.rewindAndResend('user_failed', originalText)
    expect(provider.rewindThread).toHaveBeenCalledWith('thr_existing', 'turn_failed')
    expect(state.blocks).toEqual([retained])
    expect(Object.keys(state.turnUsageById)).toEqual(['turn_retained'])
    expect(state.rewoundTurnIds).toMatchObject({ previously_removed: true, turn_failed: true })
    expect(sendMessage).not.toHaveBeenCalled()
    completeHTTP()
    await pending

    expect(provider.rewindThread).toHaveBeenCalledTimes(1)
    expect(Object.keys(state.turnUsageById)).toEqual(['turn_retained'])
    expect(state.rewoundTurnIds).toMatchObject({ previously_removed: true, turn_failed: true })
    expect(state.lastTurnUsage).toBeNull()
    expect(sendMessage).toHaveBeenCalledTimes(1)
    expect(sendMessage).toHaveBeenCalledWith(originalText, undefined, expect.objectContaining({ attachmentIds: [], attachments: [], fileReferences: [] }))
    expect(state.blocks).toEqual([retained])
    expect(state.turnStartedAtByUserId).toEqual({ user_retained: 1 })
    expect(state.turnDurationByUserId).toEqual({})
    expect(state.error).toBeNull()
  })

  it('blocks case-boundary rewind before restoring a git checkpoint', async () => {
    const restoreGitCheckpoint = vi.fn(async () => ({ ok: true }))
    vi.stubGlobal('window', {
      analytix: {
        workspace: { restoreGitCheckpoint },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, provider, sendMessage, state } = buildHarness()
    Object.assign(state, {
      busy: false,
      activeThreadId: 'thr_existing',
      threads: [{
        ...thread('thr_existing'),
        historyAuthority: 'case_boundary_only_v1' as const
      }],
      blocks: [{
        kind: 'user',
        id: 'user_case',
        text: 'old case prompt',
        meta: { turnId: 'turn_case', workspaceCheckpointId: 'gcp_case' }
      }],
      turnStartedAtByUserId: {},
      turnDurationByUserId: {},
      queuedMessages: []
    })

    await actions.rewindAndResend('user_case', 'new case prompt')

    expect(restoreGitCheckpoint).not.toHaveBeenCalled()
    expect(provider.rewindThread).not.toHaveBeenCalled()
    expect(sendMessage).not.toHaveBeenCalled()
    expect(state.error).toBeTruthy()
  })

  it('confirms and restores a git checkpoint for standalone workspace rollback', async () => {
    const canaries = {
      rescueId: 'ROLLBACK_RESCUE_CANARY_7F3C',
      workspaceRoot: '/private/rollback/workspace-canary',
      activeThreadId: 'ROLLBACK_THREAD_CANARY_7F3C'
    }
    const restoreGitCheckpoint = vi.fn(async () => ({
      ok: true,
      checkpointId: 'gcp_1',
      repositoryRoot: canaries.workspaceRoot,
      head: 'abc',
      currentBranch: 'main',
      rescueCheckpointId: canaries.rescueId
    }))
    const confirmDialog = vi.fn(async () => true)
    const infoSpy = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    vi.stubGlobal('window', {
      analytix: {
        app: { confirmDialog },
        workspace: { restoreGitCheckpoint },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    try {
      const { actions, state } = buildHarness()
      state.busy = false
      state.workspaceRoot = canaries.workspaceRoot
      state.activeThreadId = canaries.activeThreadId

      await actions.rollbackWorkspaceToCheckpoint(' gcp_1 ')

      expect(confirmDialog).toHaveBeenCalled()
      expect(restoreGitCheckpoint).toHaveBeenCalledWith({ checkpointId: 'gcp_1' })
      expect(state.error).toBeNull()
      expect(infoSpy).not.toHaveBeenCalled()
      expect(maintenanceActionsSource).not.toContain('console.info(')
      const consoleOutput = JSON.stringify(infoSpy.mock.calls)
      for (const canary of Object.values(canaries)) expect(consoleOutput).not.toContain(canary)
    } finally {
      infoSpy.mockRestore()
    }
  })
})


const attachmentId = `att_${'a'.repeat(24)}`
const secondAttachmentId = `att_${'b'.repeat(24)}`
function publicAttachment(id = attachmentId) {
  return { id, name: 'fixture.png', kind: 'image' as const, mimeType: 'image/png', byteSize: 2,
    scope: 'thread' as const, createdAt: '2026-10-05', updatedAt: '2026-10-05' }
}
function resendHarness(meta: Record<string, unknown> = {}) {
  const harness = buildHarness()
  Object.assign(harness.state, { busy: false, workspaceRoot: '/workspace/analytix',
    blocks: [{ kind: 'user', id: 'user_edit', text: 'old', meta: { turnId: 'turn_edit', ...meta } }],
    liveAssistant: '', currentTurnId: null, currentTurnUserId: null,
    turnStartedAtByUserId: {}, turnDurationByUserId: {}, queuedMessages: [], composerDrafts: {} })
  harness.state.updateComposerDraft = (key, update) => {
    const current = harness.state.composerDrafts[key] ?? emptyComposerDraft
    harness.state.composerDrafts = { ...harness.state.composerDrafts, [key]: update(current) }
  }
  harness.provider.getAttachmentMetadata.mockImplementation(async (id: string) => publicAttachment(id))
  return harness
}
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
describe('edit resend attachment continuity and ownership', () => {
  beforeEach(() => {
    registryMock.getProvider.mockReset()
    vi.stubGlobal('localStorage', memoryStorage())
  })
  afterEach(() => vi.unstubAllGlobals())

  it('captures mixed/ids-only attachments and legal file metadata without copying private payload or old receipts', async () => {
    const h = resendHarness({ attachmentIds: [` ${attachmentId} `, attachmentId, secondAttachmentId],
      attachments: [{ id: attachmentId, kind: 'image', name: 'fixture.png', mimeType: 'image/png', byteSize: 2,
        documentText: 'PRIVATE_TEXT', previewUrl: 'PRIVATE_URL', localFilePath: 'PRIVATE_PATH', receipt: 'OLD_RECEIPT' }],
      fileReferences: [{ path: '/workspace/analytix/fixture.txt', relativePath: 'fixture.txt', name: 'fixture.txt', kind: 'file', contents: 'PRIVATE_FILE' }],
      guiPlan: { planId: 'OLD_PLAN' }, useReceipts: ['OLD_RECEIPT'] })
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.provider.getAttachmentMetadata.mock.calls).toEqual([
      [attachmentId, { threadId: 'thr_existing', workspace: '/workspace/analytix' }],
      [secondAttachmentId, { threadId: 'thr_existing', workspace: '/workspace/analytix' }]
    ])
    const [text, mode, overrides] = h.sendMessage.mock.calls[0]
    expect(text).toBe('edited'); expect(mode).toBeUndefined()
    expect(overrides.attachmentIds).toEqual([attachmentId, secondAttachmentId])
    expect(overrides.attachments).toHaveLength(2)
    expect(overrides.attachments.map((a: { id: string }) => a.id)).toEqual([attachmentId, secondAttachmentId])
    expect(overrides.fileReferences).toEqual([{ path: '/workspace/analytix/fixture.txt', relativePath: 'fixture.txt', name: 'fixture.txt', kind: 'file' }])
    for (const field of ['PRIVATE_TEXT', 'PRIVATE_URL', 'PRIVATE_PATH', 'PRIVATE_FILE', 'OLD_RECEIPT', 'OLD_PLAN']) expect(JSON.stringify(overrides)).not.toContain(field)
    expect(overrides.submissionGuard.isCurrent()).toBe(true)
  })

  it.each(['missing', 'denied', 'wrong-id', 'malformed'])('fails before checkpoint or rewind when attachment metadata is %s', async reason => {
    const restoreGitCheckpoint = vi.fn(async () => ({ ok: true }))
    vi.stubGlobal('window', { analytix: { workspace: { restoreGitCheckpoint } } })
    const h = resendHarness({ attachmentIds: [attachmentId], workspaceCheckpointId: 'gcp_fixture' })
    if (reason === 'wrong-id') h.provider.getAttachmentMetadata.mockResolvedValue(publicAttachment(secondAttachmentId))
    else if (reason === 'malformed') h.provider.getAttachmentMetadata.mockResolvedValue({ ...publicAttachment(), documentText: 'PRIVATE_BODY' })
    else h.provider.getAttachmentMetadata.mockRejectedValue(new Error(reason === 'denied' ? '403 PRIVATE_BODY' : '404 PRIVATE_BODY'))
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(restoreGitCheckpoint).not.toHaveBeenCalled(); expect(h.provider.rewindThread).not.toHaveBeenCalled()
    expect(h.sendMessage).not.toHaveBeenCalled(); expect(h.state.blocks).toHaveLength(1)
    expect(h.state.error).toBeTruthy(); expect(h.state.error).not.toContain('PRIVATE_BODY')
    const draft = h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')]
    expect(draft.input).toBe('edited'); expect(draft.attachments).toEqual([{ id: attachmentId }])
  })

  it.each([{ attachmentIds: [attachmentId], attachments: [{ id: secondAttachmentId }] }, { attachmentIds: [123] }, { attachmentIds: [attachmentId], attachments: [{ id: attachmentId, kind: '' }] }, { fileReferences: [{ path: '', name: 'bad' }] }])('rejects inconsistent or malformed captured payload instead of text-only resend', async meta => {
    const h = resendHarness(meta)
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.provider.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
    expect(h.state.error).toBeTruthy(); expect(h.state.composerDrafts).toEqual({})
  })

  it.each(['metadata', 'restore', 'rewind'].flatMap(seam => ['A-B', 'A-B-A'].map(navigation => ({ seam, navigation }))))('revokes $navigation completion at the $seam await and preserves the new selection', async ({ seam, navigation }) => {
    const h = resendHarness(seam === 'metadata' ? { attachmentIds: [attachmentId] } : seam === 'restore' ? { workspaceCheckpointId: 'gcp_fixture' } : {})
    const pending = deferred<unknown>()
    if (seam === 'metadata') h.provider.getAttachmentMetadata.mockReturnValue(pending.promise)
    if (seam === 'restore') vi.stubGlobal('window', { analytix: { workspace: { restoreGitCheckpoint: vi.fn(() => pending.promise) } } })
    if (seam === 'rewind') h.provider.rewindThread.mockReturnValue(pending.promise)
    const operation = h.actions.rewindAndResend('user_edit', 'edited')
    h.state.activeThreadId = 'thr_other'; getThreadBinding(h.sseAbortRef).generation++
    if (navigation === 'A-B-A') { h.state.activeThreadId = 'thr_existing'; getThreadBinding(h.sseAbortRef).generation++ }
    h.state.blocks = [{ kind: 'assistant', id: 'new-selection', text: 'keep' }]
    h.state.queuedMessages = [{ threadId: h.state.activeThreadId!, id: 'new-queue', text: 'keep' }]
    h.state.error = 'new error'
    pending.resolve(seam === 'metadata' ? publicAttachment() : { ok: true })
    await operation
    expect(h.sendMessage).not.toHaveBeenCalled(); expect(h.state.blocks[0].id).toBe('new-selection')
    expect(h.state.queuedMessages[0].id).toBe('new-queue'); expect(h.state.error).toBe('new error')
    if (seam !== 'rewind') expect(h.provider.rewindThread).not.toHaveBeenCalled()
    const draft = h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')]
    expect(draft.input).toBe('edited')
    if (seam === 'metadata') expect(draft.attachments).toEqual([{ id: attachmentId }])
  })

  it.each(['missing-capability', 'missing-workspace', 'target-removed', 'case-authority-changed'])('blocks destructive work for %s', async seam => {
    const h = resendHarness({ attachmentIds: [attachmentId] })
    if (seam === 'missing-capability') Object.assign(h.provider, { getAttachmentMetadata: undefined })
    if (seam === 'missing-workspace') { h.state.workspaceRoot = ''; h.state.threads[0].workspace = '' }
    if (seam === 'target-removed' || seam === 'case-authority-changed') {
      h.provider.getAttachmentMetadata.mockImplementation(async () => {
        if (seam === 'target-removed') h.state.blocks = []
        else h.state.threads[0].historyAuthority = 'case_boundary_only_v1'
        return publicAttachment()
      })
    }
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.provider.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
  })

  it('admits only one destructive edit while validation is pending and fails closed on disconnect', async () => {
    const h = resendHarness({ attachmentIds: [attachmentId] }), pending = deferred<unknown>()
    h.provider.getAttachmentMetadata.mockReturnValue(pending.promise)
    const operation = h.actions.rewindAndResend('user_edit', 'first')
    await h.actions.rewindAndResend('user_edit', 'second')
    expect(h.provider.getAttachmentMetadata).toHaveBeenCalledTimes(1)
    h.state.runtimeConnection = 'offline'; pending.resolve(publicAttachment()); await operation
    expect(h.provider.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
  })


  it.each(['/new-review','plain edited request'])('does not infer a new load from edited history without direct selection: %s', async text => {
    const h=resendHarness({activeSkillIds:['old-review']});
    await h.actions.rewindAndResend('user_edit',text);
    expect(h.sendMessage.mock.calls[0][2]?.requestedSkillIds ?? []).toEqual([]);
  });

  it.each(['false', 'throw'])('restores editable payload after rewind when sending returns %s', async outcome => {
    const h = resendHarness({ attachmentIds: [attachmentId], fileReferences: [{ path: '/workspace/analytix/a.txt', relativePath: 'a.txt', name: 'a.txt', kind: 'file' }] })
    if (outcome === 'false') h.sendMessage.mockResolvedValue(false)
    else h.sendMessage.mockRejectedValue(new Error('PRIVATE_SEND_ERROR'))
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.provider.rewindThread).toHaveBeenCalledTimes(1); expect(h.state.blocks).toEqual([])
    const draft = h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')]
    expect(draft.input).toBe('edited'); expect(draft.attachments[0].id).toBe(attachmentId)
    expect(draft.fileReferences[0].type).toBe('file')
    expect(h.state.error).toBeTruthy(); expect(h.state.error).not.toContain('PRIVATE_SEND_ERROR')
  })

  it.each(['input', 'attachment', 'file'])('does not replace a newer %s draft after failed resend', async changed => {
    const h = resendHarness(), key = composerDraftKey('/workspace/analytix', 'thr_existing')
    h.sendMessage.mockImplementation(async () => {
      h.state.updateComposerDraft(key, draft => ({ ...draft, input: changed === 'input' ? 'new draft' : draft.input, inputRevision: draft.inputRevision + 1,
        attachments: changed === 'attachment' ? [{ id: secondAttachmentId }] : draft.attachments,
        fileReferences: changed === 'file' ? [{ path: '/workspace/new.txt', relativePath: 'new.txt', name: 'new.txt' }] : draft.fileReferences }))
      return false
    })
    await h.actions.rewindAndResend('user_edit', 'edited')
    const draft = h.state.composerDrafts[key]
    expect(draft.input).toBe(changed === 'input' ? 'new draft' : '')
    if (changed === 'attachment') expect(draft.attachments).toEqual([{ id: secondAttachmentId }])
    if (changed === 'file') expect(draft.fileReferences[0].name).toBe('new.txt')
  })

  it.each(['runtime_request_failed', 'runtime_response_schema_invalid', 'internal_error', 'uncoded', 'conflict'])('keeps the closed edit draft after real rewind SSE precedes rejected %s HTTP', async code => {
    const h = resendHarness({ attachmentIds: [attachmentId], fileReferences: [{ path: '/workspace/analytix/a.txt', relativePath: 'a.txt', name: 'a.txt', kind: 'file' }] })
    h.state.lastSeq = 10
    const seq = h.state.lastSeq + 1
    const sink = buildThreadEventSink(partial => Object.assign(h.state, typeof partial === 'function' ? partial(h.state) : partial), h.get, { threadId: 'thr_existing' })
    h.provider.rewindThread.mockImplementationOnce(async () => {
      sink.onThreadRewound?.({ threadId: 'thr_existing', turnId: 'turn_edit', removedTurns: 1, remainingTurns: 0, removedTurnIds: ['turn_edit'], seq })
      throw new Error(code === 'uncoded' ? 'PRIVATE_REWIND_RESPONSE' : JSON.stringify({ code, message: 'PRIVATE_REWIND_RESPONSE' }))
    })
    await h.actions.rewindAndResend('user_edit', 'edited after uncertain rewind')
    expect(h.state.blocks).toEqual([]); expect(h.state.lastSeq).toBe(seq)
    expect(h.sendMessage).not.toHaveBeenCalled()
    const draft = h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')]
    expect(draft?.input).toBe('edited after uncertain rewind')
    expect(draft.attachments[0].id).toBe(attachmentId); expect(draft.fileReferences[0].type).toBe('file')
    expect(h.state.error).toBe(i18n.t('common:rewindResendOutcomeUnknown'))
    expect(h.state.error).not.toContain('PRIVATE_REWIND_RESPONSE')
  })

  it('keeps an uncertain rewind draft without fabricating a local history cut when no SSE arrives', async () => {
    const h = resendHarness()
    h.provider.rewindThread.mockRejectedValue(new Error(JSON.stringify({ code: 'runtime_request_failed', message: 'synthetic transport failure' })))
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.state.blocks[0].id).toBe('user_edit'); expect(h.sendMessage).not.toHaveBeenCalled()
    expect(h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')]?.input).toBe('edited')
    expect(h.state.error).toBe(i18n.t('common:rewindResendOutcomeUnknown'))
  })

  it.each(['retained', 'rewound'])('keeps a failed rewind unknown with the exact target %s', async targetState => {
    const h = resendHarness()
    h.state.lastSeq = 10
    const sink = buildThreadEventSink(partial => Object.assign(h.state, typeof partial === 'function' ? partial(h.state) : partial), h.get, { threadId: 'thr_existing' })
    h.provider.rewindThread.mockImplementationOnce(async () => {
      if (targetState === 'rewound') sink.onThreadRewound?.({ threadId: 'thr_existing', turnId: 'turn_edit', removedTurns: 1, remainingTurns: 0, removedTurnIds: ['turn_edit'], seq: 11 })
      throw new Error(JSON.stringify({ code: 'not_found', message: 'The requested resource was not found.' }))
    })
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.sendMessage).not.toHaveBeenCalled()
    expect(h.state.error).toBe(i18n.t('common:rewindResendOutcomeUnknown'))
    const draft = h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')]
    expect(draft.input).toBe('edited')
    if (targetState === 'retained') expect(h.state.blocks[0].id).toBe('user_edit')
    else expect(h.state.blocks).toEqual([])
  })

  it.each(['input', 'attachment', 'file', 'deleted', 'migrated', 'case', 'navigation'])('respects %s ownership after a real rewind SSE and an uncertain HTTP receipt', async change => {
    const h = resendHarness({ attachmentIds: [attachmentId], fileReferences: [{ path: '/workspace/analytix/a.txt', relativePath: 'a.txt', name: 'a.txt', kind: 'file' }] })
    const key = composerDraftKey('/workspace/analytix', 'thr_existing')
    h.state.lastSeq = 10
    const sink = buildThreadEventSink(partial => Object.assign(h.state, typeof partial === 'function' ? partial(h.state) : partial), h.get, { threadId: 'thr_existing' })
    h.provider.rewindThread.mockImplementationOnce(async () => {
      sink.onThreadRewound?.({ threadId: 'thr_existing', turnId: 'turn_edit', removedTurns: 1, remainingTurns: 0, removedTurnIds: ['turn_edit'], seq: 11 })
      if (change === 'input' || change === 'attachment' || change === 'file') h.state.updateComposerDraft(key, draft => ({ ...draft,
        input: change === 'input' ? 'newer text' : draft.input, inputRevision: draft.inputRevision + 1,
        attachments: change === 'attachment' ? [{ id: secondAttachmentId }] : draft.attachments,
        fileReferences: change === 'file' ? [{ path: '/workspace/new.txt', relativePath: 'new.txt', name: 'new.txt' }] : draft.fileReferences }))
      if (change === 'deleted') h.state.threads = h.state.threads.filter(thread => thread.id !== 'thr_existing')
      if (change === 'migrated') h.state.threads[0].workspace = '/workspace/moved'
      if (change === 'case') h.state.threads[0].historyAuthority = 'case_boundary_only_v1'
      if (change === 'navigation') {
        h.state.activeThreadId = 'thr_other'; getThreadBinding(h.sseAbortRef).generation++
        h.state.blocks = [{ kind: 'assistant', id: 'new-view', text: 'keep' }]; h.state.error = 'new view error'
      }
      throw new Error(JSON.stringify({ code: 'runtime_request_failed', message: 'synthetic lost response' }))
    })
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.sendMessage).not.toHaveBeenCalled()
    const draft = h.state.composerDrafts[key]
    if (change === 'deleted' || change === 'migrated' || change === 'case') expect(draft).toBeUndefined()
    else if (change === 'navigation') { expect(draft.input).toBe('edited'); expect(h.state.blocks[0].id).toBe('new-view'); expect(h.state.error).toBe('new view error') }
    else {
      expect(draft.input).toBe(change === 'input' ? 'newer text' : '')
      if (change === 'attachment') expect(draft.attachments).toEqual([{ id: secondAttachmentId }])
      if (change === 'file') expect(draft.fileReferences[0].name).toBe('new.txt')
    }
  })

  it.each(['input', 'attachment', 'file', 'deleted', 'migrated', 'case', 'navigation'])('respects %s ownership when metadata preflight fails before rewind', async change => {
    const h = resendHarness({ attachmentIds: [attachmentId] })
    const key = composerDraftKey('/workspace/analytix', 'thr_existing')
    const pending = deferred<unknown>()
    h.provider.getAttachmentMetadata.mockReturnValue(pending.promise)
    const operation = h.actions.rewindAndResend('user_edit', 'edited')
    if (change === 'input' || change === 'attachment' || change === 'file') h.state.updateComposerDraft(key, draft => ({ ...draft,
      input: change === 'input' ? 'newer text' : draft.input, inputRevision: draft.inputRevision + 1,
      attachments: change === 'attachment' ? [{ id: secondAttachmentId }] : draft.attachments,
      fileReferences: change === 'file' ? [{ path: '/workspace/new.txt', relativePath: 'new.txt', name: 'new.txt' }] : draft.fileReferences }))
    if (change === 'deleted') h.state.threads = []
    if (change === 'migrated') h.state.threads[0].workspace = '/workspace/moved'
    if (change === 'case') h.state.threads[0].historyAuthority = 'case_boundary_only_v1'
    if (change === 'navigation') {
      h.state.activeThreadId = 'thr_other'; getThreadBinding(h.sseAbortRef).generation++
      h.state.blocks = [{ kind: 'assistant', id: 'new-view', text: 'keep' }]; h.state.error = 'new view error'
    }
    pending.reject(new Error('PRIVATE_METADATA_BODY')); await operation
    expect(h.provider.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
    const draft = h.state.composerDrafts[key]
    if (change === 'deleted' || change === 'migrated' || change === 'case') expect(draft).toBeUndefined()
    else if (change === 'navigation') {
      expect(draft.input).toBe('edited'); expect(draft.attachments).toEqual([{ id: attachmentId }])
      expect(h.state.blocks[0].id).toBe('new-view'); expect(h.state.error).toBe('new view error')
    } else {
      expect(draft.input).toBe(change === 'input' ? 'newer text' : '')
      if (change === 'attachment') expect(draft.attachments).toEqual([{ id: secondAttachmentId }])
      if (change === 'file') expect(draft.fileReferences[0].name).toBe('new.txt')
    }
  })

  it('does not accept a forged raw JSON conflict as proof of a pre-commit rejection', async () => {
    const h = resendHarness()
    h.provider.rewindThread.mockRejectedValue(new Error(JSON.stringify({ code: 'conflict', message: 'The request conflicts with the current runtime state.' })))
    await h.actions.rewindAndResend('user_edit', 'edited')
    expect(h.state.composerDrafts[composerDraftKey('/workspace/analytix', 'thr_existing')].input).toBe('edited')
    expect(h.state.error).toBe(i18n.t('common:rewindResendOutcomeUnknown')); expect(h.state.blocks[0].id).toBe('user_edit')
    expect(h.sendMessage).not.toHaveBeenCalled()
  })

  it('keeps file-context displayText turns uneditable and stops on restore or rewind failure', async () => {
    const blocked = resendHarness({ displayText: 'public visible text', fileReferences: [{ path: '/workspace/a.txt', relativePath: 'a.txt', name: 'a.txt' }] })
    await blocked.actions.rewindAndResend('user_edit', 'edited')
    expect(blocked.provider.rewindThread).not.toHaveBeenCalled(); expect(blocked.sendMessage).not.toHaveBeenCalled()
    const restoreGitCheckpoint = vi.fn(async () => ({ ok: false, message: 'PRIVATE_RESTORE' }))
    vi.stubGlobal('window', { analytix: { workspace: { restoreGitCheckpoint } } })
    const restored = resendHarness({ workspaceCheckpointId: 'gcp_fixture' })
    await restored.actions.rewindAndResend('user_edit', 'edited')
    expect(restored.provider.rewindThread).not.toHaveBeenCalled(); expect(restored.sendMessage).not.toHaveBeenCalled()
    const rewound = resendHarness(); rewound.provider.rewindThread.mockRejectedValue(new Error('PRIVATE_REWIND'))
    await rewound.actions.rewindAndResend('user_edit', 'edited')
    expect(rewound.sendMessage).not.toHaveBeenCalled(); expect(rewound.state.blocks).toHaveLength(1)
  })
})

describe('typed user-input display privacy', () => {
  beforeEach(() => {
    registryMock.getProvider.mockReset()
    vi.stubGlobal('localStorage', memoryStorage())
  })
  afterEach(() => vi.unstubAllGlobals())
  it('keeps private custom text only in the host submission and displays fixed status', async () => {
    const sentinel = 'PRIVATE_NON_PII_CUSTOM_SENTINEL'
    vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } })
    const { actions, provider, state } = buildHarness()
    const submit = vi.fn(async () => undefined)
    Object.assign(provider, { submitUserInputResponse: submit })
    Object.assign(state, { blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('e'), requestId: nativeRequestId('e'),
      status: 'pending', questions: [
        { id: nativeRequestId('e') + '_1', header: 'Path', question: 'Choose', multiSelect: true,
          options: [{ id: nativeRequestId('e') + '_1_option_1', label: 'Fast', description: '' }] },
        { id: nativeRequestId('e') + '_2', header: 'Detail', question: 'Detail', multiSelect: false, options: [] }
      ] }] })
    const answers = [
      { id: nativeRequestId('e') + '_1', kind: 'answer' as const, optionIds: [nativeRequestId('e') + '_1_option_1'], customText: sentinel },
      { id: nativeRequestId('e') + '_2', kind: 'skip' as const }
    ]
    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('e'), requestId: nativeRequestId('e'), threadId: state.activeThreadId }, { kind: 'submit', answers })
    expect(submit).toHaveBeenCalledWith(nativeRequestId('e'), answers, nativeQuestionSelector(nativeRequestId('e'), state.activeThreadId))
    expect(state.blocks[0]).toMatchObject({ status: 'submitted', answers: [
      { id: nativeRequestId('e') + '_1', label: 'Fast', value: `Fast\n${i18n.t('common:userInputSubmitted')}` },
      { id: nativeRequestId('e') + '_2', label: i18n.t('common:userInputSkipped'), value: '' }
    ] })
    expect(JSON.stringify(state.blocks)).not.toContain(sentinel)
    expect(JSON.stringify(state.blocks)).not.toContain('customText')
    expect(JSON.stringify(state.blocks)).not.toContain('optionIds')
  })
})


describe('user-input identity action boundary', () => {
  it('cancels each of two independently displayed pending requests by its captured pair', async () => {
    const { actions, provider, state } = buildHarness({ activeThreadId: 'thr_existing' })
    const cancel = vi.fn(async () => undefined)
    Object.assign(provider, { cancelUserInput: cancel })
    Object.assign(state, { busy: false, blocks: [
      { kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'pending', questions: [] },
      { kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('b'), requestId: nativeRequestId('b'), status: 'pending', questions: [] }
    ] })
    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('b'), requestId: nativeRequestId('b'), threadId: 'thr_existing' }, { kind: 'cancel' })
    expect(cancel).toHaveBeenNthCalledWith(1, nativeRequestId('b'), nativeQuestionSelector(nativeRequestId('b'), 'thr_existing'))
    expect(state.blocks).toMatchObject([{ status: 'pending' }, { status: 'cancelled' }])
    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: 'thr_existing' }, { kind: 'cancel' })
    expect(cancel).toHaveBeenNthCalledWith(2, nativeRequestId('a'), nativeQuestionSelector(nativeRequestId('a'), 'thr_existing'))
    expect(state.blocks).toMatchObject([{ status: 'cancelled' }, { status: 'cancelled' }])
  })

  it.each(['submit', 'cancel'] as const)('rejects a displayed %s after the same item changes to a different request', async kind => {
    const { actions, provider, state } = buildHarness({ activeThreadId: 'thr_existing' })
    const submit = vi.fn(async () => undefined), cancel = vi.fn(async () => undefined)
    Object.assign(provider, { submitUserInputResponse: submit, cancelUserInput: cancel })
    Object.assign(state, { blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('c'), status: 'pending', questions: [] }] })
    await expect(actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('d'), threadId: 'thr_existing' },
      kind === 'submit' ? { kind, answers: [{ id: 'q1', label: '', value: 'Synthetic old answer' }] } : { kind })).rejects.toThrow(i18n.t('common:ownedThreadUnavailable'))
    expect(submit).not.toHaveBeenCalled(); expect(cancel).not.toHaveBeenCalled()
    expect(state.blocks).toMatchObject([{ requestId: nativeRequestId('c'), status: 'pending' }])
  })

  it.each(['submit', 'cancel'] as const)('rejects a captured %s on a different active thread', async kind => {
    const { actions, provider, state } = buildHarness({ activeThreadId: 'thr_other' })
    const submit = vi.fn(async () => undefined), cancel = vi.fn(async () => undefined)
    Object.assign(provider, { submitUserInputResponse: submit, cancelUserInput: cancel })
    Object.assign(state, { blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'pending', questions: [] }] })
    await actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: 'thr_existing' },
      kind === 'submit' ? { kind, answers: [{ id: 'q1', label: '', value: 'Synthetic old answer' }] } : { kind })
    expect(submit).not.toHaveBeenCalled(); expect(cancel).not.toHaveBeenCalled()
    expect(state.blocks).toMatchObject([{ status: 'pending' }])
  })

  it.each([
    { itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('b'), threadId: 'thr_existing', turnId: 'turn-question-owner' },
    { itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: null, turnId: 'turn-question-owner' },
    { itemId: 'unknown', requestId: nativeRequestId('a'), threadId: 'thr_existing', turnId: 'turn-question-owner' },
    { itemId: 'item_' + nativeRequestId('a'), requestId: '', threadId: 'thr_existing', turnId: 'turn-question-owner' }
  ])('rejects an unknown or conflicting displayed target %j', async target => {
    const { actions, provider, state } = buildHarness({ activeThreadId: 'thr_existing' })
    const cancel = vi.fn(async () => undefined)
    Object.assign(provider, { cancelUserInput: cancel })
    Object.assign(state, { blocks: [
      { kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'pending', questions: [] },
      { kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('b'), requestId: nativeRequestId('b'), status: 'pending', questions: [] }
    ] })
    if (target.threadId === null) await actions.resolveUserInput(target, { kind: 'cancel' })
    else await expect(actions.resolveUserInput(target, { kind: 'cancel' })).rejects.toThrow(i18n.t('common:ownedThreadUnavailable'))
    expect(cancel).not.toHaveBeenCalled()
    expect(state.blocks).toMatchObject([{ status: 'pending' }, { status: 'pending' }])
  })

  it('does not send a late captured cancel after the exact request has already submitted', async () => {
    const { actions, provider, state } = buildHarness({ activeThreadId: 'thr_existing' })
    const cancel = vi.fn(async () => undefined)
    Object.assign(provider, { cancelUserInput: cancel })
    Object.assign(state, { blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'submitted', questions: [] }] })
    await expect(actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: 'thr_existing' }, { kind: 'cancel' })).rejects.toThrow(i18n.t('common:pendingRequestNotResolved'))
    expect(cancel).not.toHaveBeenCalled()
    expect(state.blocks).toMatchObject([{ status: 'submitted' }])
  })

  beforeEach(() => {
    registryMock.getProvider.mockReset()
    vi.stubGlobal('localStorage', memoryStorage())
    vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('does not cancel the first pending request when two displayed requests share an item identity', async () => {
    const { actions, provider, state } = buildHarness()
    const cancel = vi.fn(async () => undefined)
    Object.assign(provider, { cancelUserInput: cancel })
    Object.assign(state, { busy: false, blocks: [
      { kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'pending', questions: [] },
      { kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('b'), status: 'pending', questions: [] }
    ] })
    await expect(actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('b'), threadId: state.activeThreadId }, { kind: 'cancel' })).rejects.toThrow(i18n.t('common:ownedThreadUnavailable'))
    expect(cancel).not.toHaveBeenCalled()
    expect(state.blocks.map(block => block.kind === 'user_input' ? block.status : undefined)).toEqual(['pending', 'pending'])
  })
})


describe('timed user-input action receipts', () => {
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => vi.unstubAllGlobals())
  const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
  it('keeps captured late UUID and target when current runtime turn changes and never publishes raw answers', async () => {
    const h = buildHarness(), submit = vi.fn(async () => undefined)
    Object.assign(h.provider, { submitUserInputLateReply: submit })
    Object.assign(h.state, { busy: true, currentTurnId: 'turn-new', blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'continued', timedWait: timing, questions: [] }] })
    const answers = [{ id: nativeRequestId('a') + '_1', kind: 'answer' as const, optionIds: [], customText: 'PRIVATE_SYNTHETIC_LATE_ANSWER' }]
    const action = { kind: 'late' as const, answers, clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', expectedTurnId: 'turn-captured' }
    await h.actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: h.state.activeThreadId }, action)
    expect(submit).toHaveBeenCalledWith(nativeRequestId('a'), answers, { clientUserMessageId: action.clientUserMessageId, expectedTurnId: 'turn-captured' }, nativeQuestionSelector(nativeRequestId('a'), h.state.activeThreadId))
    expect(h.state.blocks).toMatchObject([{ status: 'continued' }])
    expect(h.state.blocks).toHaveLength(1); expect(JSON.stringify(h.state.blocks)).not.toContain('PRIVATE_SYNTHETIC_LATE_ANSWER')
  })
  it.each(['same', 'switched', 'already-streaming'] as const)('recovers actual idle wake through existing snapshot/SSE only for %s owner', async owner => {
    const h = buildHarness(); let finish!: () => void
    const submit = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    Object.assign(h.provider, { submitUserInputLateReply: submit })
    Object.assign(h.state, { busy: false, currentTurnId: null, route: 'chat', blocks: [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status: 'continued', questions: [] }] })
    const action = { kind: 'late' as const, answers: [], clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', expectedTurnId: 'turn-host-terminal' }
    const response = h.actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: h.state.activeThreadId }, action)
    expect(h.state.busy).toBe(false); expect(h.state.currentTurnId).toBeNull()
    if (owner === 'switched') h.state.activeThreadId = 'other-thread'
    if (owner === 'already-streaming') { h.state.busy = true; h.state.currentTurnId = 'actual-sse-new-turn' }
    finish(); await response
    expect(h.recoverActiveTurn).toHaveBeenCalledTimes(owner === 'same' ? 1 : 0)
    expect(h.state.blocks).toMatchObject([{ status: 'continued' }]); expect(h.state.blocks).toHaveLength(1)
  })
  it.each(['submitted', 'cancelled', 'continued'] as const)('rejects a stale foreground cancel on %s without changing the real status', async status => {
    const h = buildHarness(), cancel = vi.fn(async () => undefined)
    Object.assign(h.provider, { cancelUserInput: cancel })
    h.state.blocks = [{ kind: 'user_input', meta: { turnId: 'turn-question-owner' }, id: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), status, timedWait: timing, questions: [] }]
    await expect(h.actions.resolveUserInput({ turnId: 'turn-question-owner', itemId: 'item_' + nativeRequestId('a'), requestId: nativeRequestId('a'), threadId: h.state.activeThreadId }, { kind: 'cancel' })).rejects.toThrow()
    expect(cancel).not.toHaveBeenCalled(); expect(h.state.blocks).toMatchObject([{ status }])
  })
})

describe('Question metadata lifecycle at confirmed thread deletion', () => {
  const requestId = 'input_' + 'a'.repeat(64)
  const key = (threadId: string) => JSON.stringify({ threadId, turnId: 'turn-synthetic', itemId: 'item_' + requestId, requestId })
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => vi.unstubAllGlobals())
  it.each([false, true])('clears only a confirmed deletion and preserves failure metadata, fails=%s', async fails => {
    const { actions, state, provider } = buildHarness({ activeThreadId: null })
    state.userInputDraftInterruptions = { [key('thr_existing')]: { revision: null, unsent: true }, [key('thr_other')]: { revision: null, unsent: true } }
    if (fails) provider.deleteThread.mockRejectedValueOnce(new Error('Synthetic deletion failure'))
    await actions.deleteThread('thr_existing')
    expect(state.deletedUserInputDraftThreadIds?.thr_existing).toBe(fails ? undefined : true)
    expect(state.userInputDraftInterruptions).toEqual(fails
      ? { [key('thr_existing')]: { revision: null, unsent: true }, [key('thr_other')]: { revision: null, unsent: true } }
      : { [key('thr_other')]: { revision: null, unsent: true } })
  })
  it('retains warnings on ordinary archive and clears them on the existing deletion fallback', async () => {
    const { actions, state, provider } = buildHarness({ activeThreadId: null })
    state.userInputDraftInterruptions = { [key('thr_existing')]: { revision: null, unsent: true } }
    await actions.archiveThread('thr_existing', true)
    expect(state.userInputDraftInterruptions).toEqual({ [key('thr_existing')]: { revision: null, unsent: true } })
    Reflect.deleteProperty(provider, 'archiveThread')
    await actions.archiveThread('thr_existing', true)
    expect(provider.deleteThread).toHaveBeenCalledWith('thr_existing')
    expect(state.deletedUserInputDraftThreadIds?.thr_existing).toBe(true)
    expect(state.userInputDraftInterruptions).toEqual({})
  })
})


describe('HTTP-confirmed rewind usage ownership', () => {
  it('cuts only the captured suffix without SSE and preserves prior tombstones', async () => {
    const { actions, state, provider, sendMessage } = buildHarness()
    const retained: ChatBlock = { kind: 'user', id: 'retained-user', text: 'earlier', meta: { turnId: 'retained' } }
    Object.assign(state, { busy: false, blocks: [retained,
      { kind: 'user', id: 'target-user', text: 'old', meta: { turnId: 'target' } },
      { kind: 'user', id: 'later-user', text: 'later', meta: { turnId: 'later' } }],
      turnUsageById: footerEntries(['retained', 'target', 'later']), rewoundTurnIds: { previous: true }, lastTurnUsage: footerLastUsage })
    await actions.rewindAndResend('target-user', 'new')
    expect(provider.rewindThread).toHaveBeenCalledWith('thr_existing', 'target')
    expect(state.blocks).toEqual([retained]); expect(Object.keys(state.turnUsageById)).toEqual(['retained'])
    expect(state.rewoundTurnIds).toEqual({ previous: true, target: true, later: true })
    expect(state.lastTurnUsage).toBeNull(); expect(sendMessage).toHaveBeenCalledOnce()
  })
  it('does not manufacture a cut or discard usage when HTTP fails without a durable SSE receipt', async () => {
    const { actions, state, provider, sendMessage } = buildHarness()
    const blocks: ChatBlock[] = [{ kind: 'user', id: 'target-user', text: 'old', meta: { turnId: 'target' } }]
    const entries = footerEntries(['target']), tombstones = { previous: true as const }
    Object.assign(state, { busy: false, blocks, turnUsageById: entries, rewoundTurnIds: tombstones, lastTurnUsage: footerLastUsage })
    provider.rewindThread.mockRejectedValueOnce(new Error('Synthetic unavailable reply'))
    await actions.rewindAndResend('target-user', 'new')
    expect(state.blocks).toBe(blocks); expect(state.turnUsageById).toBe(entries)
    expect(state.rewoundTurnIds).toBe(tombstones); expect(state.lastTurnUsage).toBe(footerLastUsage)
    expect(sendMessage).not.toHaveBeenCalled()
  })
})


describe('Question native turn action ownership', () => {
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()); vi.stubGlobal('window', { analytix: { logs: { error: vi.fn(async () => undefined) } } }) })
  afterEach(() => vi.unstubAllGlobals())
  const requestId = nativeRequestId('a')
  it.each(['submit', 'cancel', 'late'] as const)('rejects a captured %s when the same physical pair now has another native turn', async kind => {
    const { actions, provider, state } = buildHarness()
    const submit = vi.fn(), cancel = vi.fn(), late = vi.fn()
    Object.assign(provider, { submitUserInputResponse: submit, cancelUserInput: cancel, submitUserInputLateReply: late })
    state.blocks = [{ kind: 'user_input', id: 'item_' + requestId, requestId, status: kind === 'late' ? 'continued' : 'pending', questions: [], meta: { turnId: 'other-native-turn' } }]
    const action = kind === 'late' ? { kind, answers: [], clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', expectedTurnId: 'current-steering-turn' }
      : kind === 'submit' ? { kind, answers: [] } : { kind }
    await expect(actions.resolveUserInput(nativeQuestionSelector(requestId, state.activeThreadId), action)).rejects.toThrow(i18n.t('common:ownedThreadUnavailable'))
    expect(submit).not.toHaveBeenCalled(); expect(cancel).not.toHaveBeenCalled(); expect(late).not.toHaveBeenCalled()
  })
  it.each(['success', 'failure'] as const)('does not write a captured %s response into the same pair on a replacement native turn', async outcome => {
    const { actions, provider, state } = buildHarness()
    let finish!: () => void, fail!: (error: Error) => void
    Object.assign(provider, { submitUserInputResponse: vi.fn(() => new Promise<void>((resolve, reject) => { finish = resolve; fail = reject })) })
    const block: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: 'item_' + requestId, requestId, status: 'pending', questions: [], meta: { turnId: 'turn-question-owner' } }
    state.blocks = [block]
    const pending = actions.resolveUserInput(nativeQuestionSelector(requestId, state.activeThreadId), { kind: 'submit', answers: [] })
    const replacement = { ...block, meta: { turnId: 'replacement-native-turn' } }; state.blocks = [replacement]
    if (outcome === 'success') finish(); else fail(new Error('Synthetic old response'))
    await pending
    expect(state.blocks).toEqual([replacement]); expect(state.error).toBeNull()
    expect(provider.getThreadDetail).not.toHaveBeenCalled()
  })
  it.each([null, '', 'legacy-turn'] as const)('does not infer a missing or mismatched native selector from requestId, turn=%s', async turnId => {
    const { actions, provider, state } = buildHarness(), cancel = vi.fn()
    Object.assign(provider, { cancelUserInput: cancel })
    state.blocks = [{ kind: 'user_input', id: 'item_' + requestId, requestId, status: 'pending', questions: [], meta: { turnId: 'turn-question-owner' } }]
    await expect(actions.resolveUserInput({ ...nativeQuestionSelector(requestId, state.activeThreadId), turnId }, { kind: 'cancel' })).rejects.toThrow(i18n.t('common:ownedThreadUnavailable'))
    expect(cancel).not.toHaveBeenCalled()
  })
})


describe('Queue confirmed retirement and Stop owners', () => {
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => { clearActiveStream(); vi.unstubAllGlobals() })
  it.each(['delete', 'archive-delete-fallback'] as const)('retires only the confirmed thread through %s', async route => {
    const { actions, state, provider } = buildHarness()
    const other = { threadId: 'thr_other', id: 'same-id', text: 'retain', sending: true }
    Object.assign(state, { queuedMessages: [{ threadId: 'thr_existing', id: 'same-id', text: 'retire', sending: true }, other],
      queuedMessagePauses: { thr_existing: 'interrupted', thr_other: 'failed' }, watchTurnCompletion: {}, unreadThreadIds: {} })
    if (route === 'archive-delete-fallback') { Reflect.deleteProperty(provider, 'archiveThread'); await actions.archiveThread('thr_existing', true) }
    else await actions.deleteThread('thr_existing')
    expect(provider.deleteThread).toHaveBeenCalledExactlyOnceWith('thr_existing')
    expect(state.queuedMessages).toEqual([other])
    expect(state.queuedMessagePauses).toEqual({ thr_other: 'failed' })
  })

  it('retains Stop after trusted terminal reconciliation', async () => {
    const { actions, state, provider, drainQueuedMessages } = buildHarness()
    const pending = { threadId: 'thr_existing', id: 'after-stop', text: 'later' }
    Object.assign(state, { busy: true, currentTurnId: 'turn-stop', currentTurnUserId: 'user-stop',
      queuedMessages: [pending], blocks: [{ kind: 'user', id: 'user-stop', text: 'initial', meta: { turnId: 'turn-stop' } }],
      lastSeq: 3, turnStartedAtByUserId: {}, turnDurationByUserId: {} })
    provider.getThreadDetail.mockResolvedValueOnce({ blocks: state.blocks, latestSeq: 4,
      latestTurnId: 'turn-stop', threadStatus: 'aborted' })
    await actions.interrupt()
    expect(queuedPauseReason(state, pending.threadId)).toBe('interrupted')
    expect(state.queuedMessages).toEqual([pending])
    expect(drainQueuedMessages).toHaveBeenCalledWith(pending.threadId)
  })
})

describe('rename acknowledgement and selection ownership', () => {
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => vi.unstubAllGlobals())

  const ownerChanges = ['active', 'generation', 'submission', 'provider', 'route', 'ABA'] as const
  function seedSearchProjection(h: Harness): NormalizedThread {
    const other = { ...thread('thr_search_other'), title: 'Other search title' }
    h.state.threadSearch = 'Synthetic title filter'
    h.state.threadSearchResults = { query: h.state.threadSearch, threads: [thread('thr_existing'), other] }
    return other
  }
  function expectSearchProjection(h: Harness, title: string, other: NormalizedThread, query = 'Synthetic title filter'): void {
    expect(h.state.threadSearchResults?.query).toBe(query)
    expect(h.state.threadSearchResults?.threads[0].title).toBe(title)
    expect(h.state.threadSearchResults?.threads[1]).toBe(other)
    expect(h.state.threadSearchResults?.threads).toHaveLength(2)
  }
  function replaceOwner(h: Harness, change: typeof ownerChanges[number]): void {
    const binding = getThreadBinding(h.sseAbortRef)
    if (change === 'active') h.state.activeThreadId = 'thr_other'
    if (change === 'generation') binding.generation++
    if (change === 'submission') binding.submissionGeneration++
    if (change === 'provider') registryMock.getProvider.mockReturnValue({ ...h.provider, renameThread: vi.fn() })
    if (change === 'route') h.state.route = 'write'
    if (change === 'ABA') {
      h.state.activeThreadId = 'thr_other'; binding.generation++
      h.state.activeThreadId = 'thr_existing'; binding.generation++
    }
    h.state.error = 'New selection error'
  }

  it('rejects an offline rename after publishing its connection error without sending a request', async () => {
    const h = buildHarness()
    h.state.runtimeConnection = 'offline'
    await expect(h.actions.renameThread('thr_existing', 'New title')).rejects.toThrow(i18n.t('common:runtimeActionNeedsConnection'))
    expect(h.state.error).toBe(i18n.t('common:runtimeActionNeedsConnection'))
    expect(h.provider.renameThread).not.toHaveBeenCalled()
    expect(h.refreshThreads).not.toHaveBeenCalled()
    expect(h.state.threads[0].title).toBe('thr_existing')
  })

  it('propagates an active rename POST failure to its caller and retains the original title', async () => {
    const h = buildHarness()
    h.provider.renameThread.mockRejectedValueOnce(new Error('Synthetic rename rejected'))
    await expect(h.actions.renameActiveThread('New title')).rejects.toThrow()
    expect(h.provider.renameThread).toHaveBeenCalledExactlyOnceWith('thr_existing', 'New title')
    expect(h.state.error).toBe(i18n.t('common:runtimeRequestFailed'))
    expect(h.state.threads[0].title).toBe('thr_existing')
    expect(h.refreshThreads).not.toHaveBeenCalled()
  })

  it('does not change titles before HTTP acknowledgement and refreshes only after the committed rename', async () => {
    const h = buildHarness(), response = deferred<void>()
    const searchOther = seedSearchProjection(h)
    let refreshWasCurrent: boolean | undefined
    h.refreshThreads.mockImplementationOnce(async options => { refreshWasCurrent = options?.isCurrent?.() })
    h.state.threads = [thread('thr_existing'), thread('thr_other')]
    h.provider.renameThread.mockImplementationOnce(() => response.promise)
    const pending = h.actions.renameThread(' thr_existing ', ' New title ')
    expect(h.provider.renameThread).toHaveBeenCalledExactlyOnceWith('thr_existing', 'New title')
    expect(h.state.threads.map(value => value.title)).toEqual(['thr_existing', 'thr_other'])
    expectSearchProjection(h, 'thr_existing', searchOther)
    expect(h.refreshThreads).not.toHaveBeenCalled()
    response.resolve(undefined)
    await pending
    expect(h.state.threads.map(value => value.title)).toEqual(['New title', 'thr_other'])
    expectSearchProjection(h, 'New title', searchOther)
    expect(h.state.error).toBeNull()
    expect(h.refreshThreads).toHaveBeenCalledExactlyOnceWith({ isCurrent: expect.any(Function) })
    expect(refreshWasCurrent).toBe(true)
    expect(h.refreshThreads.mock.calls[0][0]?.isCurrent?.()).toBe(true)
  })

  it.each(['later-rename', 'replacement-binding'] as const)('keeps an acknowledged read guard alive until %s retires it', async retirement => {
    const h = buildHarness()
    h.state.threads = [thread('thr_existing'), thread('thr_other')]
    let readCurrent: (() => boolean) | undefined
    h.refreshThreads.mockImplementationOnce(async options => { readCurrent = options?.isCurrent })
    await h.actions.renameThread('thr_existing', 'Committed title')
    expect(readCurrent?.()).toBe(true)
    if (retirement === 'later-rename') {
      const later = deferred<void>()
      h.provider.renameThread.mockImplementationOnce(() => later.promise)
      const pending = h.actions.renameThread('thr_other', 'Later intent title')
      expect(h.provider.renameThread).toHaveBeenLastCalledWith('thr_other', 'Later intent title')
      expect(readCurrent?.()).toBe(false)
      expect(h.state.threads[0].title).toBe('Committed title')
      later.resolve(undefined)
      await pending
    } else {
      getThreadBinding(h.sseAbortRef).generation++
    }
    expect(readCurrent?.()).toBe(false)
    expect(h.state.activeThreadId).toBe('thr_existing')
  })

  it.each(ownerChanges.flatMap(change => ['ack', 'failure'].map(outcome => ({ change, outcome }))))(
    'keeps a late $outcome from replacing the new UI owner after $change', async ({ change, outcome }) => {
      const h = buildHarness(), response = deferred<void>()
      const searchOther = seedSearchProjection(h)
      h.provider.renameThread.mockImplementationOnce(() => response.promise)
      const completed = h.actions.renameThread('thr_existing', 'Acknowledged resource title')
        .then(() => 'resolved', () => 'rejected')
      replaceOwner(h, change)
      h.state.threadSearch = 'New synthetic filter'
      h.state.threadSearchResults = { query: h.state.threadSearch, threads: [...h.state.threadSearchResults!.threads] }
      const route = h.state.route
      if (outcome === 'ack') response.resolve(undefined)
      else response.reject(new Error(JSON.stringify({ code: 'missing_api_key', message: 'Synthetic old request failure' })))
      expect(await completed).toBe(outcome === 'ack' ? 'resolved' : 'rejected')
      expect(h.state.error).toBe('New selection error')
      expect(h.state.route).toBe(route)
      expect(h.state.settingsSection).toBe('general')
      expect(h.refreshThreads).not.toHaveBeenCalled()
      const expectedTitle = outcome === 'ack' && change !== 'provider' ? 'Acknowledged resource title' : 'thr_existing'
      expect(h.state.threads[0].title).toBe(expectedTitle)
      expectSearchProjection(h, expectedTitle, searchOther, 'New synthetic filter')
    }
  )

  it.each(['ack', 'failure'] as const)('keeps the newer same-target title when an older rename returns %s', async outcome => {
    const h = buildHarness(), older = deferred<void>(), newer = deferred<void>()
    const searchOther = seedSearchProjection(h)
    h.provider.renameThread.mockImplementationOnce(() => older.promise).mockImplementationOnce(() => newer.promise)
    const first = h.actions.renameThread('thr_existing', 'Older title').then(() => 'resolved', () => 'rejected')
    const second = h.actions.renameThread('thr_existing', 'Newer title')
    newer.resolve(undefined)
    await second
    expect(h.state.threads[0].title).toBe('Newer title')
    expectSearchProjection(h, 'Newer title', searchOther)
    if (outcome === 'ack') older.resolve(undefined)
    else older.reject(new Error('Synthetic superseded rename failure'))
    expect(await first).toBe(outcome === 'ack' ? 'resolved' : 'rejected')
    expect(h.state.threads[0].title).toBe('Newer title')
    expectSearchProjection(h, 'Newer title', searchOther)
    expect(h.state.error).toBeNull()
    expect(h.refreshThreads).toHaveBeenCalledTimes(1)
  })

  it('resolves an acknowledged rename when its subsequent refresh fails and records the refresh error', async () => {
    const h = buildHarness()
    h.refreshThreads.mockRejectedValueOnce(new Error('Synthetic refresh failure after committed rename'))
    await expect(h.actions.renameThread('thr_existing', 'Committed title')).resolves.toBeUndefined()
    expect(h.state.threads[0].title).toBe('Committed title')
    expect(h.state.error).toBe(i18n.t('common:runtimeRequestFailed'))
    expect(h.provider.renameThread).toHaveBeenCalledOnce()
  })

  it.each(['ack', 'failure'] as const)('revokes a pending refresh after an ABA selection before refresh %s', async outcome => {
    const h = buildHarness(), refreshed = deferred<void>()
    let refreshCurrent: (() => boolean) | undefined
    h.refreshThreads.mockImplementationOnce(async options => {
      refreshCurrent = options?.isCurrent
      await refreshed.promise
      if (options?.isCurrent && !options.isCurrent()) return
      h.state.threads = [{ ...thread('thr_existing'), title: 'Stale refresh title' }]
    })
    const pending = h.actions.renameThread('thr_existing', 'Committed title')
    await Promise.resolve()
    expect(h.state.threads[0].title).toBe('Committed title')
    expect(refreshCurrent?.()).toBe(true)
    replaceOwner(h, 'ABA')
    expect(refreshCurrent?.()).toBe(false)
    if (outcome === 'ack') refreshed.resolve(undefined)
    else refreshed.reject(new Error('Synthetic retired refresh failure'))
    await expect(pending).resolves.toBeUndefined()
    expect(h.state.threads[0].title).toBe('Committed title')
    expect(h.state.error).toBe('New selection error')
  })

  it.each(['post-ack', 'post-failure', 'refresh-failure'] as const)('preserves a newer error on the same binding through %s', async seam => {
    const h = buildHarness(), response = deferred<void>()
    let refreshCurrent: (() => boolean) | undefined
    if (seam === 'refresh-failure') h.refreshThreads.mockImplementationOnce(options => { refreshCurrent = options?.isCurrent; return response.promise })
    else {
      h.provider.renameThread.mockImplementationOnce(() => response.promise)
      if (seam === 'post-ack') h.refreshThreads.mockRejectedValueOnce(new Error('Synthetic refresh must not start after a newer error'))
    }
    const completed = h.actions.renameThread('thr_existing', 'Committed resource title').then(() => 'resolved', () => 'rejected')
    if (seam === 'refresh-failure') {
      await Promise.resolve()
      expect(h.state.threads[0].title).toBe('Committed resource title')
      expect(h.refreshThreads).toHaveBeenCalledOnce()
      expect(refreshCurrent?.()).toBe(true)
    }
    h.state.error = 'New same-binding error'
    if (seam === 'refresh-failure') expect(refreshCurrent?.()).toBe(false)
    if (seam === 'post-ack') response.resolve(undefined)
    else response.reject(new Error(JSON.stringify({ code: 'missing_api_key', message: 'Synthetic old operation failure' })))
    expect(await completed).toBe(seam === 'post-failure' ? 'rejected' : 'resolved')
    expect(h.state.error).toBe('New same-binding error')
    expect(h.state.route).toBe('chat')
    expect(h.state.settingsSection).toBe('general')
    expect(h.state.threads[0].title).toBe(seam === 'post-failure' ? 'thr_existing' : 'Committed resource title')
    if (seam !== 'refresh-failure') expect(h.refreshThreads).not.toHaveBeenCalled()
  })

  it('keeps a newer offline rename error after reconnecting and receiving an older same-target acknowledgement', async () => {
    const h = buildHarness(), older = deferred<void>()
    const offlineMessage = i18n.t('common:runtimeActionNeedsConnection')
    h.provider.renameThread.mockImplementationOnce(() => older.promise)
    const pending = h.actions.renameThread('thr_existing', 'Older pending title')
    h.state.runtimeConnection = 'offline'
    await expect(h.actions.renameThread('thr_existing', 'Newer offline title')).rejects.toThrow()
    expect(h.state.error).toBe(offlineMessage)
    h.state.runtimeConnection = 'ready'
    older.resolve(undefined)
    await pending
    expect(h.state.threads[0].title).toBe('thr_existing')
    expect(h.state.error).toBe(offlineMessage)
    expect(h.provider.renameThread).toHaveBeenCalledExactlyOnceWith('thr_existing', 'Older pending title')
    expect(h.refreshThreads).not.toHaveBeenCalled()
  })
})

describe('captured selection receipt for active-thread forks', () => {
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => vi.unstubAllGlobals())

  it.each(['different-thread', 'retired', 'no-active'] as const)('does not fall back to another active thread for %s receipt', async reason => {
    const h = buildHarness()
    h.state.error = 'Existing selection error'
    if (reason === 'no-active') h.state.activeThreadId = null
    await h.actions.forkActiveThread({ threadId: reason === 'different-thread' ? 'thr_failed_selection' : 'thr_existing', isCurrent: () => reason !== 'retired' })
    expect(h.provider.forkThread).not.toHaveBeenCalled()
    expect(h.refreshThreads).not.toHaveBeenCalled()
    expect(h.selectThread).not.toHaveBeenCalled()
    expect(readThreadForkRegistry().forks).toEqual({})
    expect(h.state.error).toBe('Existing selection error')
  })

  it('forks and records the exact active thread when its captured selection remains current', async () => {
    const h = buildHarness()
    h.state.threads = [{ ...thread('thr_existing'), title: 'Captured parent' }]
    await h.actions.forkActiveThread({ threadId: 'thr_existing', isCurrent: () => true })
    expect(h.provider.forkThread).toHaveBeenCalledExactlyOnceWith('thr_existing')
    expect(h.refreshThreads).toHaveBeenCalledExactlyOnceWith({ isCurrent: expect.any(Function) })
    expect(h.selectThread).toHaveBeenCalledExactlyOnceWith('thr_fork')
    expect(readThreadForkRegistry().forks.thr_fork).toMatchObject({ parentThreadId: 'thr_existing', parentTitle: 'Captured parent' })
  })

  it.each(['receipt-retired', 'ABA'].flatMap(change => ['ack', 'failure'].map(outcome => ({ change, outcome }))))(
    'ignores a created fork or failure after $change during POST, outcome=$outcome', async ({ change, outcome }) => {
      const h = buildHarness(), response = deferred<NormalizedThread>()
      let receiptCurrent = true
      h.provider.forkThread.mockImplementationOnce(() => response.promise)
      const pending = h.actions.forkActiveThread({ threadId: 'thr_existing', isCurrent: () => receiptCurrent })
      expect(h.provider.forkThread).toHaveBeenCalledExactlyOnceWith('thr_existing')
      if (change === 'receipt-retired') receiptCurrent = false
      else {
        const binding = getThreadBinding(h.sseAbortRef)
        h.state.activeThreadId = 'thr_other'; binding.generation++
        h.state.activeThreadId = 'thr_existing'; binding.generation++
      }
      h.state.error = 'Replacement selection error'
      if (outcome === 'ack') response.resolve(thread('created-by-retired-selection'))
      else response.reject(new Error(JSON.stringify({ code: 'missing_api_key', message: 'Synthetic retired fork failure' })))
      await pending
      expect(h.refreshThreads).not.toHaveBeenCalled()
      expect(h.selectThread).not.toHaveBeenCalled()
      expect(readThreadForkRegistry().forks['created-by-retired-selection']).toBeUndefined()
      expect(h.state.error).toBe('Replacement selection error')
      expect(h.state.route).toBe('chat')
      expect(h.provider.forkThread).toHaveBeenCalledOnce()
    }
  )

  it('does not select a created fork when its receipt retires during refresh', async () => {
    const h = buildHarness(), refreshed = deferred<void>()
    let receiptCurrent = true
    let refreshCurrent: (() => boolean) | undefined
    h.refreshThreads.mockImplementationOnce(async options => {
      refreshCurrent = options?.isCurrent
      await refreshed.promise
      if (options?.isCurrent && !options.isCurrent()) return
      h.state.threads = [thread('stale-refresh-result')]
    })
    const pending = h.actions.forkActiveThread({ threadId: 'thr_existing', isCurrent: () => receiptCurrent })
    await Promise.resolve()
    expect(h.refreshThreads).toHaveBeenCalledExactlyOnceWith({ isCurrent: expect.any(Function) })
    expect(refreshCurrent?.()).toBe(true)
    receiptCurrent = false
    const currentThreads = [thread('new-selection-view')]
    h.state.threads = currentThreads
    expect(refreshCurrent?.()).toBe(false)
    refreshed.resolve(undefined)
    await pending
    expect(h.state.threads).toBe(currentThreads)
    expect(h.selectThread).not.toHaveBeenCalled()
    expect(h.provider.forkThread).toHaveBeenCalledOnce()
  })
})

describe('HTTP-acknowledged lifecycle search projection', () => {
  beforeEach(() => { registryMock.getProvider.mockReset(); vi.stubGlobal('localStorage', memoryStorage()) })
  afterEach(() => vi.unstubAllGlobals())

  it('removes the deleted search hit after ACK even when the following refresh fails', async () => {
    const h = buildHarness({ activeThreadId: 'thr_other' }), response = deferred<void>()
    const target = thread('thr_existing'), other = { ...thread('thr_other'), title: 'Other search record' }
    const query = 'Synthetic lifecycle filter'
    Object.assign(h.state, { threads: [target, other], threadSearch: query,
      threadSearchResults: { query, threads: [target, other] }, watchTurnCompletion: {}, unreadThreadIds: {} })
    h.provider.deleteThread.mockImplementationOnce(() => response.promise)
    h.refreshThreads.mockRejectedValueOnce(new Error('Synthetic refresh failure after delete ACK'))
    const pending = h.actions.deleteThread(target.id)
    expect(h.provider.deleteThread).toHaveBeenCalledExactlyOnceWith(target.id)
    expect(h.state.threadSearchResults?.threads).toEqual([target, other])
    expect(h.refreshThreads).not.toHaveBeenCalled()
    response.resolve(undefined)
    await pending
    expect(h.refreshThreads).toHaveBeenCalledOnce()
    expect(h.state.threads).toEqual([other])
    expect(h.state.threadSearchResults?.query).toBe(query)
    expect(h.state.threadSearchResults?.threads).toEqual([other])
    expect(h.state.threadSearchResults?.threads[0]).toBe(other)
    expect(h.state.threadSearch).toBe(query)
  })

  it.each([
    { label: 'archive', initial: false, archived: true },
    { label: 'restore', initial: true, archived: false }
  ])('keeps the $label ACK in search results even when the following refresh fails', async ({ initial, archived }) => {
    const h = buildHarness({ activeThreadId: 'thr_other' }), response = deferred<void>()
    const target = { ...thread('thr_existing'), archived: initial }
    const other = { ...thread('thr_other'), title: 'Other search record', archived: false }
    const query = 'Synthetic lifecycle filter'
    Object.assign(h.state, { threads: [target, other], threadSearch: query,
      threadSearchResults: { query, threads: [target, other] }, watchTurnCompletion: {}, unreadThreadIds: {} })
    h.provider.archiveThread.mockImplementationOnce(() => response.promise)
    h.refreshThreads.mockRejectedValueOnce(new Error('Synthetic refresh failure after archive state ACK'))
    const pending = h.actions.archiveThread(target.id, archived)
    expect(h.provider.archiveThread).toHaveBeenCalledExactlyOnceWith(target.id, archived)
    expect(h.state.threadSearchResults?.threads[0].archived).toBe(initial)
    expect(h.refreshThreads).not.toHaveBeenCalled()
    response.resolve(undefined)
    await pending
    expect(h.refreshThreads).toHaveBeenCalledOnce()
    expect(h.provider.deleteThread).not.toHaveBeenCalled()
    expect(h.state.threads[0]).toMatchObject({ id: target.id, archived })
    expect(h.state.threadSearchResults?.query).toBe(query)
    expect(h.state.threadSearchResults?.threads[0]).toMatchObject({ id: target.id, archived })
    expect(h.state.threadSearchResults?.threads).toHaveLength(2)
    expect(h.state.threadSearchResults?.threads[1]).toBe(other)
    expect(h.state.threadSearch).toBe(query)
  })
})
