import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
const generatedPreview = vi.hoisted(() => ({ open: vi.fn(async () => true) }))
vi.mock('../office/open-generated-artifact', () => ({ openGeneratedArtifact: generatedPreview.open }))
import type { AcceptedFinalProjectionBatch, ChatBlock, GeneralTerminalProjectionBatch } from '../agent/types'
import { dispatchAnalytixRuntimeEvent, dispatchAnalytixRuntimeEvents } from '../agent/analytix-mapper'
import { AnalytixRuntimeProvider } from '../agent/analytix-runtime'
import { rendererRuntimeClient } from '../agent/runtime-client'
import i18n from '../i18n'
import { formatRuntimeError } from '../lib/format-runtime-error'
import {
  isPublicProjectionRevoked,
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
  watchTurnCompletionNotification
} from './chat-store-runtime'
import {
  markThreadWorktree,
  readThreadWorktreeRegistry,
  saveThreadWorktreeRegistry
} from '../lib/thread-worktree-registry'
import { clearBusyWatchdog, resetBusyRecoveryAttempts } from './chat-store-schedulers'
import type { ChatState, ChatStoreSet } from './chat-store-types'
import {
  clearActiveStream,
  getActiveStreamSnapshot,
  pendingActiveStreamTurnId,
  resetActiveStream
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
      text: 'ordinary general guidance'
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
    expect(committed).toMatchObject({
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null,
      watchTurnCompletion: {},
      unreadThreadIds: {}
    })
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
      threads: [caseThread, derivedThread, safeThread],
      caseProjectThreadsById: { case_one: [caseThread, derivedThread, safeThread] },
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
    expect(getState().queuedMessagesPausedReason).toBe('failed')
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
    expect(getState().queuedMessagesPausedReason).toBe('failed')
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
        text: nextTurn.items[0].text, createdAt: timestamp, meta: { turnId: 'turn-next' }
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
      expect(reloaded.blocks).toEqual(getState().blocks)
      expect(reloaded.latestSeq).toBe(getState().lastSeq)
    } finally {
      request.mockRestore()
    }
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
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
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
            providerId: 'custom-openai sk-providerSecret1234567890',
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

  it('preserves provider retry attempts on runtime status rows', () => {
    const { getState, set, get } = makeSinkHarness({
      activeThreadId: 'thread-current',
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
      text: '模型服务正在重试',
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
      activeThreadId: 'thread-current',
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
      activeThreadId: 'thread-current',
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
      queuedMessages: [{ id: 'queued-1', text: 'next request' }]
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
    expect(getState().queuedMessagesPausedReason).toBe('failed')
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
