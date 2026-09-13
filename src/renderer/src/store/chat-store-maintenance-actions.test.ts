import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import maintenanceActionsSource from './chat-store-maintenance-actions.ts?raw'
import type { ChatBlock, NormalizedThread, ThreadGoal, ThreadGoalStatus, ThreadTodoList, ThreadTodoStatus } from '../agent/types'
import type { ChatState, ChatStoreGet, ChatStoreSet, SendMessageOverrides } from './chat-store-types'
import { readThreadForkRegistry } from '../lib/thread-fork-registry'
import i18n from '../i18n'
import { getActiveStreamSnapshotFor, resetActiveStream } from '../thread/streaming/active-stream-store'

const registryMock = vi.hoisted(() => ({
  getProvider: vi.fn()
}))

vi.mock('../agent/registry', () => ({
  getProvider: registryMock.getProvider
}))

import { createMaintenanceActions } from './chat-store-maintenance-actions'
import { buildThreadEventSink } from './chat-store-runtime'

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
    archiveThread: ReturnType<typeof vi.fn>
    deleteThread: ReturnType<typeof vi.fn>
    forkThread: ReturnType<typeof vi.fn>
    resumeSession: ReturnType<typeof vi.fn>
    setThreadGoal: ReturnType<typeof vi.fn>
    clearThreadGoal: ReturnType<typeof vi.fn>
    interruptTurn: ReturnType<typeof vi.fn>
    getThreadDetail: ReturnType<typeof vi.fn>
    rewindThread: ReturnType<typeof vi.fn>
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
    archiveThread: vi.fn(async () => undefined),
    deleteThread: vi.fn(async () => undefined),
    forkThread: vi.fn(async (threadId: string, _options?: { turnId?: string }) => ({
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
  const refreshThreads = vi.fn(async () => undefined)
  const selectThread = vi.fn(async () => undefined)
  const drainQueuedMessages = vi.fn(async () => undefined)
  const recoverActiveTurn = vi.fn(async () => false)
  const sendMessage = vi.fn(async (
    _text: string,
    _mode?: string,
    _overrides?: SendMessageOverrides
  ) => true)

  state = {
    activeThreadGoal: initialGoal,
    activeThreadTodos: initialTodos,
    activeThreadId,
    createThread,
    error: null,
    drainQueuedMessages,
    recoverActiveTurn,
    refreshThreads,
    runtimeConnection: 'ready',
    selectThread,
    sendMessage,
    settingsSection: 'general',
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

  return { actions, createThread, drainQueuedMessages, get, provider, recoverActiveTurn, refreshThreads, selectThread, sendMessage, sseAbortRef, state }
}

describe('chat-store-maintenance-actions goal actions', () => {
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
      activeThreadTodos: { threadId: 'thr_existing', items: [] },
      blocks: [{ kind: 'assistant', id: 'assistant-1', text: 'old answer' }],
      busy: true,
      currentTurnId: 'turn-1',
      currentTurnUserId: 'user-1',
      liveAssistant: 'partial',
      lastSeq: 42,
      queuedMessages: [{ id: 'q-1', text: 'queued' }],
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
    expect(state.queuedMessages).toEqual([])
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
    expect(refreshThreads).toHaveBeenCalledTimes(1)
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
    expect(refreshThreads).toHaveBeenCalledTimes(1)
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

  it('sets a goal on the active thread, syncs snapshots, and starts the goal turn', async () => {
    const { actions, provider, refreshThreads, sendMessage, state } = buildHarness()

    const result = await actions.setActiveThreadGoal('  ship goal mode  ')

    expect(result).toBe(true)
    expect(provider.setThreadGoal).toHaveBeenCalledWith('thr_existing', {
      objective: 'ship goal mode',
      status: 'active'
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
    expect(sendMessage).toHaveBeenCalledWith(
      'ship goal mode',
      'agent',
      expect.objectContaining({
        displayText: expect.stringContaining('ship goal mode')
      })
    )
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
      status: 'active',
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
      status: 'active'
    })
    expect(createThread.mock.invocationCallOrder[0]).toBeLessThan(
      provider.setThreadGoal.mock.invocationCallOrder[0]
    )
    expect(state.activeThreadId).toBe('thr_created')
    expect(state.activeThreadGoal?.threadId).toBe('thr_created')
    expect(state.threads[0]?.goal?.objective).toBe('ship goal mode')
    expect(sendMessage).toHaveBeenCalledWith(
      'ship goal mode',
      'agent',
      expect.objectContaining({
        displayText: expect.stringContaining('ship goal mode')
      })
    )
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

  it('updates active todo status and keeps only one item in progress', async () => {
    const initialTodos = todos('thr_existing', [
      ['Read context', 'in_progress'],
      ['Implement closure', 'pending'],
      ['Verify', 'pending']
    ])
    const { actions, provider, state } = buildHarness({ initialTodos })

    const result = await actions.setActiveThreadTodoStatus('todo_2', 'in_progress')

    expect(result).toBe(true)
    expect(provider.setThreadTodos).toHaveBeenCalledWith('thr_existing', [
      expect.objectContaining({ id: 'todo_1', content: 'Read context', status: 'pending' }),
      expect.objectContaining({ id: 'todo_2', content: 'Implement closure', status: 'in_progress' }),
      expect.objectContaining({ id: 'todo_3', content: 'Verify', status: 'pending' })
    ])
    expect(state.activeThreadTodos?.items.map((item) => [item.content, item.status])).toEqual([
      ['Read context', 'pending'],
      ['Implement closure', 'in_progress'],
      ['Verify', 'pending']
    ])
    expect(state.threads[0]?.todos?.items[1]?.status).toBe('in_progress')
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
    provider.interruptTurn.mockImplementation(async () => {
      busyWhenBackendCalled = state.busy
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
        kind: 'user_input',
        id: 'input-account',
        requestId: 'request-account',
        questions: [],
        status: 'pending'
      }]
    })
    const answers = [{ id: 'account', label: `银行卡号 ${account}`, value: account }]

    await actions.resolveUserInput('input-account', { kind: 'submit', answers })

    expect(submitUserInputResponse).toHaveBeenCalledWith('request-account', answers)
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
        kind: 'user_input',
        id: 'input-unsupported',
        requestId: 'request-unsupported',
        questions: [],
        status: 'pending'
      }]
    })

    await actions.resolveUserInput('input-unsupported', {
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
    expect(sendMessage).toHaveBeenCalledWith('new text')
  })

  it('resends unchanged text exactly once after the rewind event precedes the successful HTTP response', async () => {
    const { actions, get, provider, sendMessage, state } = buildHarness()
    const originalText = 'retry this unchanged request'
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
    expect(sendMessage).not.toHaveBeenCalled()
    completeHTTP()
    await pending

    expect(provider.rewindThread).toHaveBeenCalledTimes(1)
    expect(sendMessage).toHaveBeenCalledTimes(1)
    expect(sendMessage).toHaveBeenCalledWith(originalText)
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
