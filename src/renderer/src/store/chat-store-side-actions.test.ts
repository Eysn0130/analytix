import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createSideActions,
  teardownAllSideSubscriptions
} from './chat-store-side-actions'
import { DEFAULT_ANALYTIX_MODEL } from '@shared/app-settings'
import type { ChatState } from './chat-store-types'
import type {
  AcceptedFinalProjectionBatch,
  AgentProvider,
  NormalizedThread,
  ThreadEventSink
} from '../agent/types'
import {
  clearActiveStream,
  getActiveStreamSnapshotFor,
  isPendingActiveStreamTurnId,
  resetActiveStream
} from '../thread/streaming/active-stream-store'
import { resetPublicProjectionRevocationsForTests } from '../lib/public-projection-revocation'

type Harness = {
  state: ChatState
  set: (partial: Partial<ChatState> | ((s: ChatState) => Partial<ChatState>)) => void
  get: () => ChatState
  provider: FakeProvider
  actions: ReturnType<typeof createSideActions>
}

type ThreadDetailResponse = Awaited<ReturnType<AgentProvider['getThreadDetail']>>
type SendUserMessageResponse = Awaited<ReturnType<AgentProvider['sendUserMessage']>>

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

class FakeProvider implements AgentProvider {
  readonly id = 'analytix' as const
  readonly displayName = 'Fake'
  detailResponse: ThreadDetailResponse | null = null
  forkMock = vi.fn()
  sendMock = vi.fn()
  deleteMock = vi.fn()
  detailMock = vi.fn()
  patchMock = vi.fn()
  relationMock = vi.fn()
  interruptMock = vi.fn()
  subscribeMock = vi.fn()
  refreshThreadsMock = vi.fn()
  closeSideMock = vi.fn()
  getCapabilities() {
    return { interrupt: true, stream: true, approvals: true, attachFiles: false }
  }
  async connect() {}
  async listThreads(): Promise<NormalizedThread[]> {
    return []
  }
  async createThread(): Promise<NormalizedThread> {
    throw new Error('not used')
  }
  async getThreadDetail(threadId: string) {
    this.detailMock(threadId)
    return this.detailResponse ?? {
      blocks: [
        {
          kind: 'assistant' as const,
          id: 'assistant_child_1',
          createdAt: '2026-06-02T00:00:00.000Z',
          text: 'child result'
        }
      ],
      latestSeq: 12,
      threadStatus: 'running',
      latestTurnId: 'turn_child_1',
      latestUserMessageId: 'user_child_1'
    }
  }
  async sendUserMessage(
    threadId: string,
    text: string,
    options?: { model?: string; reasoningEffort?: string }
  ) {
    const mocked = await this.sendMock(threadId, text, options)
    if (mocked && typeof mocked === 'object') {
      const response = mocked as Partial<SendUserMessageResponse>
      return {
        threadId: response.threadId ?? threadId,
        turnId: response.turnId ?? `turn_${threadId}_${Date.now()}`,
        ...(response.userMessageItemId ? { userMessageItemId: response.userMessageItemId } : {})
      }
    }
    return { threadId, turnId: `turn_${threadId}_${Date.now()}` }
  }
  async steerUserMessage(input: { threadId: string; turnId: string; clientUserMessageId?: string }) {
    return {
      threadId: input.threadId,
      turnId: input.turnId,
      ...(input.clientUserMessageId ? { clientUserMessageId: input.clientUserMessageId } : {})
    }
  }
  async interruptTurn(threadId: string, turnId: string) {
    this.interruptMock(threadId, turnId)
  }
  async renameThread() {}
  async updateThreadRelation(threadId: string, relation: NonNullable<NormalizedThread['relation']>) {
    this.relationMock(threadId, relation)
  }
  async archiveThread() {}
  async deleteThread(threadId: string) {
    this.deleteMock(threadId)
  }
  async compactThread() {}
  async forkThread(
    threadId: string,
    options?: { relation?: 'primary' | 'fork' | 'side'; title?: string; turnId?: string }
  ) {
    this.forkMock(threadId, options)
    return {
      id: `side_${threadId}`,
      title: options?.title ?? `${threadId} · side`,
      updatedAt: '2026-06-02T00:00:00.000Z',
      model: 'deepseek-chat',
      mode: 'agent',
      workspace: '/tmp',
      status: 'idle',
      relation: 'side' as const,
      parentThreadId: threadId,
      forkedFromThreadId: threadId,
      forkedFromTitle: 'Parent',
      forkedAt: '2026-06-02T00:00:00.000Z'
    }
  }
  async resumeSession() {
    return { threadId: 'resumed', sessionId: 'sid' }
  }
  async subscribeThreadEvents(
    threadId: string,
    sinceSeq: number,
    sink: ThreadEventSink,
    signal: AbortSignal
  ): Promise<void> {
    const mocked = this.subscribeMock(threadId, sinceSeq, sink, signal)
    sink.onSeq(0)
    if (mocked && typeof mocked === 'object' && typeof (mocked as Promise<void>).then === 'function') {
      return mocked as Promise<void>
    }
    signal.addEventListener('abort', () => {
      // simulate cleanup; the real implementation stops the SSE stream
    })
    return new Promise(() => undefined)
  }
  async submitApprovalDecision() {}
  async submitUserInputResponse() {}
  async cancelUserInput() {}
}

function buildHarness(overrides: Partial<ChatState> = {}): Harness {
  const state: ChatState = {
    route: 'chat',
    settingsReturnRoute: 'chat',
    pluginHostRoute: 'chat',
    settingsSection: 'general',
    initialSetupOpen: false,
    initialSetupMode: 'required',
    workspaceRoot: '/tmp',
    workspaceLabel: '/tmp',
    runtimeConnection: 'ready',
    codeWorkspaceRoots: [],
    threads: [
      {
        id: 'thr_main',
        title: 'Parent',
        updatedAt: '2026-06-02T00:00:00.000Z',
        model: 'deepseek-chat',
        mode: 'agent',
        status: 'idle'
      }
    ],
    threadSearch: '',
    showArchivedThreads: false,
    activeThreadId: 'thr_main',
    blocks: [],
    liveAssistant: '',
    lastSeq: 0,
    usageRefreshKey: 0,
    busy: true,
    error: null,
    runtimeErrorDetail: null,
    currentTurnId: 'turn_main',
    currentTurnUserId: 'item_main',
    turnStartedAtByUserId: {},
    turnDurationByUserId: {},
    inspectorSelectedId: null,
    composerModel: 'deepseek-chat',
    composerPickList: ['deepseek-chat'],
    queuedMessages: [],
    watchTurnCompletion: {},
    unreadThreadIds: {},
    sideConversations: {},
    sidePanel: { open: false, activeSideId: null },
    clawChannels: [],
    activeClawChannelId: '',
    appendLocalClawTurn: () => undefined,
    setError: () => undefined,
    setComposerModel: () => undefined,
    loadComposerModels: async () => undefined,
    setRoute: () => undefined,
    openWrite: async () => undefined,
    openCode: async () => undefined,
    openSettings: () => undefined,
    openPlugins: () => undefined,
    openClaw: () => undefined,
    refreshClawChannels: async () => undefined,
    addClawChannel: async () => undefined,
    selectClawChannel: async () => undefined,
    selectClawConversation: async () => undefined,
    deleteClawChannel: async () => undefined,
    resetClawChannelSession: async () => undefined,
    setClawChannelModel: async () => undefined,
    openInitialSetup: () => undefined,
    closeInitialSetup: () => undefined,
    boot: async () => undefined,
    probeRuntime: async () => undefined,
    chooseWorkspace: async () => null,
    clearWorkspace: async () => undefined,
    deleteWorkspace: async () => undefined,
    refreshThreads: async () => {
      provider.refreshThreadsMock()
    },
    setThreadSearch: () => undefined,
    setShowArchivedThreads: () => undefined,
    createThread: async () => undefined,
    selectThread: async () => undefined,
    recoverActiveTurn: async () => false,
    sendMessage: async () => false,
    drainQueuedMessages: async () => undefined,
    removeQueuedMessage: () => undefined,
    rewindAndResend: async () => undefined,
    interrupt: async () => undefined,
    renameActiveThread: async () => undefined,
    renameThread: async () => undefined,
    archiveThread: async () => undefined,
    compactActiveThread: async () => undefined,
    forkActiveThread: async () => undefined,
    spawnSideConversation: async () => null,
    openSideConversationDraft: () => undefined,
    sendSideMessage: async () => false,
    interruptSide: async () => undefined,
    setSideInput: () => undefined,
    setSideModel: () => undefined,
    setSideReasoningEffort: () => undefined,
    selectSideConversation: () => undefined,
    openThreadInSidePanel: async () => false,
    setSidePanelOpen: () => undefined,
    closeSideConversation: async () => undefined,
    discardSideConversation: async () => undefined,
    promoteSideConversation: async () => undefined,
    resumeSessionIntoThread: async () => null,
    deleteThread: async () => undefined,
    resolveApproval: async () => undefined,
    resolveUserInput: async () => undefined,
    selectInspectorItem: () => undefined,
    applyI18nFromSettings: async () => undefined,
    reloadUiSettings: async () => undefined,
    ...overrides
  } as ChatState
  const set: Harness['set'] = (partial) => {
    const update = typeof partial === 'function' ? partial(state) : partial
    Object.assign(state, update)
  }
  const get: Harness['get'] = () => state
  const provider = new FakeProvider()
  const actions = createSideActions({
    set,
    get,
    getProvider: () => provider,
    t: (key) => key,
    formatRuntimeError: (e) => (e instanceof Error ? e.message : String(e ?? '')),
    shouldOpenSettingsForError: () => false
  })
  return { state, set, get, provider, actions }
}

function acceptedFinalSideProjectionBatch(
  threadId: string,
  turnId: string,
  firstSeq = 1
): AcceptedFinalProjectionBatch {
  const publicationCommitId = 'f'.repeat(64)
  const batchId = '7'.repeat(64)
  const lastSeq = firstSeq + 2
  return {
    batchId,
    threadId,
    turnId,
    publicationCommitId,
    firstSeq,
    lastSeq,
    receipt: {
      schemaVersion: 1,
      batchId,
      threadId,
      turnId,
      publicationCommitId,
      lastSeq
    },
    assistant: {
      kind: 'assistant',
      id: `assistant_${turnId}`,
      createdAt: '2026-07-18T00:00:00Z',
      text: 'verified side final',
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
      meta: { turnId }
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
    }
  }
}

async function waitForAssertion(assertion: () => void): Promise<void> {
  let lastError: unknown
  for (let attempt = 0; attempt < 25; attempt += 1) {
    try {
      assertion()
      return
    } catch (error) {
      lastError = error
      await new Promise((resolve) => setTimeout(resolve, 0))
    }
  }
  throw lastError
}

describe('chat-store-side-actions', () => {
  beforeEach(() => {
    ;(globalThis as { window?: unknown }).window = {
      analytix: {
        runtime: {
          runtimeRequest: vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
        }
      }
    }
  })
  afterEach(() => {
    teardownAllSideSubscriptions()
    clearActiveStream()
    resetPublicProjectionRevocationsForTests()
    delete (globalThis as { window?: unknown }).window
  })

  it('spawnSideConversation does not change activeThreadId or main busy, even when main is running', async () => {
    const { actions, state, provider } = buildHarness()
    expect(state.activeThreadId).toBe('thr_main')
    expect(state.busy).toBe(true)

    const id = await actions.spawnSideConversation()

    expect(id).toBe('side_thr_main')
    expect(state.activeThreadId).toBe('thr_main')
    expect(state.busy).toBe(true)
    expect(state.sideConversations[id!]).toBeDefined()
    expect(state.sideConversations[id!].parentThreadId).toBe('thr_main')
    expect(state.sidePanel.open).toBe(true)
    expect(state.sidePanel.activeSideId).toBe(id)
    expect(provider.forkMock).toHaveBeenCalledWith('thr_main', { relation: 'side', title: 'Paren · side' })
    // A dedicated subscription was started for the side thread.
    expect(provider.subscribeMock).toHaveBeenCalledWith('side_thr_main', 0, expect.anything(), expect.anything())
  })

  it('openSideConversationDraft opens the side surface without forking a thread', () => {
    const { actions, state, provider } = buildHarness()

    actions.openSideConversationDraft()

    expect(state.sidePanel.open).toBe(true)
    expect(state.sidePanel.activeSideId).toBeNull()
    expect(state.sideConversations).toEqual({})
    expect(provider.forkMock).not.toHaveBeenCalled()
  })

  it('openThreadInSidePanel hydrates a child thread into the side panel without switching the main thread', async () => {
    const { actions, state, provider } = buildHarness()

    const opened = await actions.openThreadInSidePanel('thr_child', {
      title: 'Research child',
      source: 'background-agent'
    })

    expect(opened).toBe(true)
    expect(state.activeThreadId).toBe('thr_main')
    expect(state.sidePanel.open).toBe(true)
    expect(state.sidePanel.activeSideId).toBe('thr_child')
    expect(state.sideConversations.thr_child).toMatchObject({
      threadId: 'thr_child',
      parentThreadId: 'thr_main',
      title: 'Research child',
      source: 'background-agent',
      lastSeq: 12,
      busy: true,
      turnId: 'turn_child_1',
      userItemId: 'user_child_1'
    })
    expect(state.sideConversations.thr_child.blocks).toHaveLength(1)
    expect(provider.detailMock).toHaveBeenCalledWith('thr_child')
    expect(provider.subscribeMock).toHaveBeenCalledWith('thr_child', 12, expect.anything(), expect.anything())
  })

  it('reopens an existing child side thread from its own lastSeq cursor', async () => {
    const { actions, state, provider } = buildHarness({
      lastSeq: 99,
      sideConversations: {
        thr_child: {
          threadId: 'thr_child',
          parentThreadId: 'thr_main',
          title: 'Research child',
          source: 'background-agent',
          createdAt: '2026-06-02T00:00:00.000Z',
          inheritedAt: '2026-06-02T00:00:00.000Z',
          blocks: [],
          liveAssistant: '',
          lastSeq: 12,
          input: '',
          model: 'deepseek-chat',
          reasoningEffort: 'max',
          busy: false,
          turnId: null,
          userItemId: null,
          error: null
        }
      }
    })

    const opened = await actions.openThreadInSidePanel('thr_child', {
      title: 'Research child',
      source: 'background-agent'
    })

    expect(opened).toBe(true)
    expect(state.activeThreadId).toBe('thr_main')
    expect(provider.detailMock).not.toHaveBeenCalled()
    expect(provider.subscribeMock).toHaveBeenCalledWith('thr_child', 12, expect.anything(), expect.anything())
    expect(provider.subscribeMock).not.toHaveBeenCalledWith('thr_child', 0, expect.anything(), expect.anything())
    expect(provider.subscribeMock).not.toHaveBeenCalledWith('thr_child', 99, expect.anything(), expect.anything())
    expect(state.sidePanel).toEqual({ open: true, activeSideId: 'thr_child' })
  })

  it('spawnSideConversation with seedText immediately sends the first turn', async () => {
    const { actions, state, provider } = buildHarness()
    const id = await actions.spawnSideConversation('what is the dependency tree?')
    expect(id).toBe('side_thr_main')
    expect(provider.sendMock).toHaveBeenCalledWith(
      'side_thr_main',
      'what is the dependency tree?',
      expect.objectContaining({ model: 'deepseek-chat', reasoningEffort: 'max' })
    )
    const side = state.sideConversations[id!]
    expect(side.busy).toBe(true)
    expect(side.turnId).toMatch(/^turn_side_thr_main_/)
    expect(side.input).toBe('')
  })

  it('sends the selected side reasoning effort with side turns', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!

    actions.setSideReasoningEffort(id, 'low')
    const sent = await actions.sendSideMessage(id, 'use less reasoning')

    expect(sent).toBe(true)
    expect(state.sideConversations[id].reasoningEffort).toBe('low')
    expect(provider.sendMock).toHaveBeenLastCalledWith(
      id,
      'use less reasoning',
      expect.objectContaining({
        model: 'deepseek-chat',
        reasoningEffort: 'low'
      })
    )
  })

  it('does not normalize invalid side reasoning effort aliases', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!

    actions.setSideReasoningEffort(id, 'low')
    actions.setSideReasoningEffort(id, ' MAX ' as unknown as 'max')
    expect(state.sideConversations[id].reasoningEffort).toBe('low')
    await actions.sendSideMessage(id, 'keep exact effort')
    expect(provider.sendMock).toHaveBeenLastCalledWith(
      id,
      'keep exact effort',
      expect.objectContaining({ reasoningEffort: 'low' })
    )
  })

  it('renders a side user message and busy state before the provider creates the runtime turn', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const pendingSend = deferred<SendUserMessageResponse>()
    provider.sendMock.mockReturnValueOnce(pendingSend.promise)

    const sendPromise = actions.sendSideMessage(id, 'show immediately')

    const sideDuringSend = state.sideConversations[id]
    expect(sideDuringSend.busy).toBe(true)
    expect(sideDuringSend.input).toBe('')
    expect(state.busy).toBe(true)
    expect(sideDuringSend.blocks).toHaveLength(1)
    expect(sideDuringSend.blocks[0]).toMatchObject({
      kind: 'user',
      text: 'show immediately'
    })
    expect(isPendingActiveStreamTurnId(sideDuringSend.turnId)).toBe(true)
    const optimisticBlock = sideDuringSend.blocks[0]
    if (optimisticBlock?.kind !== 'user') throw new Error('expected optimistic side user block')
    expect(optimisticBlock.meta).toMatchObject({ turnId: sideDuringSend.turnId })

    pendingSend.resolve({
      threadId: id,
      turnId: 'turn_side_real',
      userMessageItemId: 'user_side_real'
    })

    await expect(sendPromise).resolves.toBe(true)
    const sideAfterSend = state.sideConversations[id]
    expect(sideAfterSend).toMatchObject({
      busy: true,
      turnId: 'turn_side_real',
      userItemId: 'user_side_real'
    })
    expect(sideAfterSend.blocks).toHaveLength(1)
    expect(sideAfterSend.blocks[0]).toMatchObject({
      kind: 'user',
      id: 'user_side_real',
      text: 'show immediately',
      meta: { turnId: 'turn_side_real' }
    })
  })

  it('sends an exact account to the host but keeps it out of side renderer state', async () => {
    const account = '6222020202020202020'
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const pendingSend = deferred<SendUserMessageResponse>()
    provider.sendMock.mockReturnValueOnce(pendingSend.promise)

    const sendPromise = actions.sendSideMessage(id, `查询银行卡号 ${account}`)

    expect(provider.sendMock).toHaveBeenLastCalledWith(
      id,
      `查询银行卡号 ${account}`,
      expect.anything()
    )
    expect(JSON.stringify(state.sideConversations[id])).not.toContain(account)
    expect(state.sideConversations[id].blocks).toEqual([
      expect.objectContaining({ kind: 'user', text: '查询银行卡号 [ACCOUNT]' })
    ])

    pendingSend.resolve({
      threadId: id,
      turnId: 'turn_side_account',
      userMessageItemId: 'user_side_account'
    })
    await expect(sendPromise).resolves.toBe(true)
    expect(JSON.stringify(state.sideConversations[id])).not.toContain(account)
  })

  it('commits side accepted-final answer, usage, cursor, and receipt atomically and replays exactly', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_final',
      userMessageItemId: 'user_side_final'
    })
    expect(await actions.sendSideMessage(id, 'case side question')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()
    const batch = acceptedFinalSideProjectionBatch(id, 'turn_side_final')

    const receipt = await sink!.onAcceptedFinalBatch?.(batch)
    const committedBlocks = state.sideConversations[id].blocks
    const committedUsage = state.sideConversations[id].lastTurnUsage

    expect(receipt).toEqual(batch.receipt)
    expect(state.sideConversations[id]).toMatchObject({
      lastSeq: batch.lastSeq,
      busy: false,
      turnId: null,
      userItemId: null,
      lastTurnUsage: { totalTokens: 13 }
    })
    expect(committedBlocks.find((block) => block.id === batch.assistant.id)).toMatchObject({
      acceptedFinalProjectionReceipt: batch.receipt
    })

    resetActiveStream(id, {
      turnId: `pending:${id}:leftover`,
      liveAssistant: 'UNTRUSTED_SIDE_LEFTOVER',
      lastSeq: batch.lastSeq
    })
    await expect(sink!.onAcceptedFinalBatch?.(batch)).resolves.toEqual(batch.receipt)
    expect(state.sideConversations[id].blocks).toBe(committedBlocks)
    expect(state.sideConversations[id].lastTurnUsage).toBe(committedUsage)
    expect(committedBlocks.filter((block) => block.id === batch.assistant.id)).toHaveLength(1)
    expect(getActiveStreamSnapshotFor(id, `pending:${id}:leftover`).threadId).toBeNull()
  })

  it('rejects accepted-final while the side turn still has an optimistic pending id', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const pendingSend = deferred<SendUserMessageResponse>()
    provider.sendMock.mockReturnValueOnce(pendingSend.promise)
    const sendPromise = actions.sendSideMessage(id, 'pending case side question')
    expect(isPendingActiveStreamTurnId(state.sideConversations[id].turnId)).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    await expect(sink!.onAcceptedFinalBatch?.(
      acceptedFinalSideProjectionBatch(id, 'turn_side_real')
    )).rejects.toThrow('could not be committed atomically')
    expect(state.sideConversations[id].lastSeq).toBe(0)
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)

    pendingSend.resolve({
      threadId: id,
      turnId: 'turn_side_real',
      userMessageItemId: 'user_side_real'
    })
    await expect(sendPromise).resolves.toBe(true)
  })

  it('purges a receipt-bearing side projection when its public authority is revoked', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_revoked',
      userMessageItemId: 'user_side_revoked'
    })
    expect(await actions.sendSideMessage(id, 'revoked case side question')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    const batch = acceptedFinalSideProjectionBatch(id, 'turn_side_revoked')
    await expect(sink?.onAcceptedFinalBatch?.(batch)).resolves.toEqual(batch.receipt)
    expect(JSON.stringify(state.sideConversations[id].blocks)).toContain(batch.batchId)

    await sink?.onPublicProjectionRevoked?.({
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId: id,
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected',
      action: 'purge_case_projection',
      terminal: true
    })

    expect(state.sideConversations[id]).toBeUndefined()
    expect(state.sidePanel.activeSideId).toBeNull()
  })

  it('quarantines same-turn side drafts when accepted-final continuity is invalid', async () => {
    const { actions, state, set, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_invalid_final',
      userMessageItemId: 'user_side_invalid_final'
    })
    expect(await actions.sendSideMessage(id, 'invalid continuity case')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    const side = state.sideConversations[id]
    set({
      sideConversations: {
        ...state.sideConversations,
        [id]: {
          ...side,
          lastSeq: 1,
          liveAssistant: 'UNTRUSTED_SIDE_LIVE_CASE_FACT',
          blocks: [
            ...side.blocks,
            {
              kind: 'assistant',
              id: 'draft_side_invalid',
              text: 'UNTRUSTED_SIDE_DRAFT_CASE_FACT',
              meta: { turnId: 'turn_side_invalid_final' }
            }
          ]
        }
      }
    })
    resetActiveStream(id, {
      turnId: 'turn_side_invalid_final',
      liveAssistant: 'UNTRUSTED_SIDE_ACTIVE_STREAM_FACT',
      lastSeq: 1
    })
    const batch = acceptedFinalSideProjectionBatch(id, 'turn_side_invalid_final', 3)

    await expect(sink?.onAcceptedFinalBatch?.(batch))
      .rejects.toThrow('could not be committed atomically')

    expect(state.sideConversations[id].lastSeq).toBe(1)
    expect(JSON.stringify(state.sideConversations[id].blocks))
      .not.toContain('UNTRUSTED_SIDE_DRAFT_CASE_FACT')
    expect(state.sideConversations[id]).toMatchObject({
      liveAssistant: '',
      busy: false,
      turnId: null,
      userItemId: null,
      error: 'common:runtimeFinalSnapshotUnavailable'
    })
    expect(getActiveStreamSnapshotFor(id, 'turn_side_invalid_final').threadId).toBeNull()
  })

  it('rolls back the optimistic side user message and restores input when provider send fails', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockRejectedValueOnce(new Error('send failed'))

    const sent = await actions.sendSideMessage(id, 'retry this side question')

    expect(sent).toBe(false)
    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: null,
      userItemId: null,
      input: 'retry this side question',
      error: 'send failed'
    })
    expect(state.sideConversations[id].blocks).toEqual([])
  })

  it('does not restore a complete account into side input after a failed send', async () => {
    const account = '6222020202020202020'
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockRejectedValueOnce(new Error('send failed'))

    await expect(actions.sendSideMessage(id, `查询银行卡号 ${account}`)).resolves.toBe(false)

    expect(state.sideConversations[id].input).toBe('')
    expect(JSON.stringify(state.sideConversations[id])).not.toContain(account)
  })

  it('reconciles a completed side turn from GET even when fabricated live text already exists', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const firstSubscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const firstSink = firstSubscription?.[2]
    expect(firstSink).toBeDefined()

    provider.sendMock.mockImplementationOnce(() => {
      firstSink!.onDeltas([
        { kind: 'agent_message', text: 'FABRICATED_SIDE_SENTINEL', seq: 13, turnId: 'turn_side_1' }
      ])
    })
    const sent = await actions.sendSideMessage(id, 'continue side')

    expect(sent).toBe(true)
    expect(state.sideConversations[id].liveAssistant).toBe('')
    const activeTurnId = state.sideConversations[id].turnId
    expect(activeTurnId).toBeTruthy()
    expect(getActiveStreamSnapshotFor(id, activeTurnId).liveAssistant).toBe('')
    expect(provider.subscribeMock.mock.calls.at(-1)?.[1]).toBe(13)
    const replaySubscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const replaySink = replaySubscription?.[2]
    const replaySignal = replaySubscription?.[3]
    replaySink!.onDeltas([
      { kind: 'agent_message', text: 'FABRICATED_SIDE_SENTINEL', seq: 13, turnId: 'turn_side_1' }
    ])
    const terminalDetail: ThreadDetailResponse = {
      blocks: [],
      latestSeq: 14,
      threadStatus: 'idle',
      latestTurnId: activeTurnId!,
      latestUserMessageId: state.sideConversations[id].userItemId ?? undefined
    }
    const pendingDetail = deferred<ThreadDetailResponse>()
    const getThreadDetail = vi.spyOn(provider, 'getThreadDetail').mockReturnValueOnce(pendingDetail.promise)
    const completion = replaySink!.onTurnComplete() as unknown as Promise<void>

    expect(getThreadDetail).toHaveBeenCalledWith(id)
    expect(state.sideConversations[id]).toMatchObject({ busy: true, turnId: activeTurnId, liveAssistant: '' })
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)
    expect(getActiveStreamSnapshotFor(id, activeTurnId).threadId).toBeNull()

    pendingDetail.resolve(terminalDetail)
    await completion

    const side = state.sideConversations[id]
    expect(side.liveAssistant).toBe('')
    expect(side.blocks).toEqual([])
    expect(side.busy).toBe(false)
    expect(side.turnId).toBeNull()
    expect(replaySignal?.aborted).toBe(true)
    expect(getActiveStreamSnapshotFor(id, activeTurnId).threadId).toBeNull()
  })

  it('ignores events from an aborted side subscription after resubscribe', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const staleSubscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const staleSink = staleSubscription?.[2]
    const staleSignal = staleSubscription?.[3]
    expect(staleSink).toBeDefined()
    expect(staleSignal?.aborted).toBe(false)

    await actions.sendSideMessage(id, 'start fresh side turn')

    const currentTurnId = state.sideConversations[id].turnId
    expect(currentTurnId).toBeTruthy()
    expect(staleSignal?.aborted).toBe(true)

    staleSink!.onDeltas([{ kind: 'agent_message', text: 'stale delta', seq: 88, turnId: 'turn_stale' }])
    staleSink!.onTool({
      itemId: 'tool_stale',
      turnId: 'turn_stale',
      summary: 'stale tool',
      status: 'running'
    })
    staleSink!.onTurnComplete({ turnId: 'turn_stale', seq: 89 })

    expect(state.sideConversations[id].turnId).toBe(currentTurnId)
    expect(state.sideConversations[id].busy).toBe(true)
    expect(state.sideConversations[id].blocks.some((block) => block.id === 'tool_stale')).toBe(false)
    expect(getActiveStreamSnapshotFor(id, 'turn_stale').threadId).toBeNull()
  })

	it('keeps the side cursor without retaining the answer draft', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onDeltas([
      { kind: 'agent_message', text: 'side same seq answer', seq: 13, turnId: 'turn_side_same_seq' }
    ])

    expect(state.sideConversations[id].lastSeq).toBe(13)
    expect(getActiveStreamSnapshotFor(id, 'turn_side_same_seq')).toMatchObject({
      liveAssistant: '',
      lastSeq: 13
    })
  })

  it('keeps long side streams out of Zustand until the turn settles', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onDeltas([{ kind: 'agent_message', text: 'seed', seq: 13, turnId: 'turn_side_long' }])
    const sideConversationsAfterShell = state.sideConversations
    const sideAfterShell = state.sideConversations[id]
    for (let index = 0; index < 120; index += 1) {
      sink!.onDeltas([{ kind: 'agent_message', text: `.${index}`, seq: 14 + index, turnId: 'turn_side_long' }])
    }

    expect(state.sideConversations).toBe(sideConversationsAfterShell)
    expect(state.sideConversations[id]).toBe(sideAfterShell)
    expect(state.sideConversations[id].liveAssistant).toBe('')
    expect(state.sideConversations[id].blocks).toHaveLength(0)
    expect(getActiveStreamSnapshotFor(id, 'turn_side_long').liveAssistant).toBe('')

    provider.detailResponse = {
      blocks: [
        {
          kind: 'assistant',
          id: 'assistant_side_long',
          text: 'persisted long result',
          meta: { turnId: 'turn_side_long' }
        }
      ],
      latestSeq: 134,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_long'
    }
    await sink!.onTurnComplete()

    expect(state.sideConversations[id].busy).toBe(false)
    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'assistant',
      text: 'persisted long result'
    })
    expect(state.sideConversations[id].blocks[0]).not.toMatchObject({ text: expect.stringContaining('seed.0.1') })
    expect(getActiveStreamSnapshotFor(id, 'turn_side_long').threadId).toBeNull()
  })

  it('adopts side tool turn ids before text arrives so interrupt targets the active turn', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    const signal = subscription?.[3]
    expect(sink).toBeDefined()

    sink!.onSeq(21)
    sink!.onTool({
      itemId: 'tool_call_1',
      turnId: 'turn_tool_first',
      summary: 'Search files',
      status: 'running',
      meta: { callId: 'call_1' }
    })
    sink!.onDeltas([{ kind: 'agent_message', text: 'partial side answer', seq: 22, turnId: 'turn_tool_first' }])

    expect(state.sideConversations[id]).toMatchObject({
      busy: true,
      turnId: 'turn_tool_first',
      lastSeq: 21
    })
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_1',
      meta: { turnId: 'turn_tool_first', callId: 'call_1' }
    })

    provider.detailResponse = {
      blocks: [
        {
          kind: 'tool',
          id: 'tool_call_1',
          summary: 'Search files',
          status: 'error',
          meta: { turnId: 'turn_tool_first', callId: 'call_1' }
        }
      ],
      latestSeq: 23,
      threadStatus: 'idle',
      latestTurnId: 'turn_tool_first'
    }
    await actions.interruptSide(id)
    expect(provider.interruptMock).toHaveBeenCalledWith(id, 'turn_tool_first')
    expect(signal?.aborted).toBe(true)
    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: null,
      userItemId: null,
      liveAssistant: ''
    })
    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_1',
      status: 'error'
    })
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)
    expect(getActiveStreamSnapshotFor(id, 'turn_tool_first').threadId).toBeNull()
  })

  it('merges side tool result events into the started tool row by call id and turn id', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onTool({
      itemId: 'tool_call_read',
      turnId: 'turn_side_tool',
      summary: 'Read file',
      status: 'running',
      toolKind: 'tool_call',
      meta: { callId: 'call_read' }
    })
    sink!.onTool({
      itemId: 'item_result_turn_side_tool_call_read',
      turnId: 'turn_side_tool',
      summary: 'Read file',
      status: 'success',
      toolKind: 'tool_call',
      detail: 'file contents',
      meta: { callId: 'call_read' }
    })

    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_read',
      status: 'success',
      detail: 'file contents',
      meta: { callId: 'call_read', turnId: 'turn_side_tool' }
    })
  })

  it('coalesces side child pipeline and progress tool rows by child run id', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onTool({
      itemId: 'tool_call_task',
      turnId: 'turn_side_tool',
      summary: 'Research child',
      status: 'running',
      toolKind: 'subagent',
      meta: {
        callId: 'call_task',
        child: {
          parentThreadId: id,
          parentTurnId: 'turn_side_tool',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childStatus: 'running',
          continueFrom: 'job-source'
        }
      }
    })
    sink!.onTool({
      itemId: 'subagent_job-1',
      turnId: 'turn_side_tool',
      summary: 'Research child',
      status: 'success',
      toolKind: 'subagent',
      meta: {
        child: {
          parentThreadId: id,
          parentTurnId: 'turn_side_tool',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thread-child',
          childStatus: 'completed',
          sourceRef: 'job-source'
        }
      }
    })

    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_task',
      status: 'success',
      meta: expect.objectContaining({
        child: expect.objectContaining({
          childRunId: 'job-1',
          childStatus: 'completed',
          childThreadId: 'thread-child'
        })
      })
    })
  })

  it('reprojects direct side progress events before they enter renderer state', async () => {
    const marker = 'SENTINEL_SIDE_TOOL_EVENT_17A8'
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onTool({
      itemId: 'tool_call_task',
      turnId: 'turn_side_tool',
      summary: marker,
      status: 'running',
      toolKind: 'subagent',
      detail: `${marker}:detail`,
      filePath: `/tmp/${marker}`,
      meta: {
        turnId: 'turn_side_tool',
        callId: 'call_task',
        runtimeStatus: 'tool_progress',
        command: `printf ${marker}`,
        child: {
          parentThreadId: id,
          parentTurnId: 'turn_side_tool',
          childId: 'child_1',
          childRunId: 'job_1',
          childStatus: 'running',
          childLabel: marker,
          childProfileDescription: `${marker}:profile`
        }
      }
    })

    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'tool',
      summary: 'Subagent activity',
      meta: expect.objectContaining({ runtimeStatus: 'tool_progress' })
    })
    expect(JSON.stringify(state.sideConversations[id].blocks)).not.toContain(marker)
  })

  it('reopens an existing busy side subscription from the latest seen seq', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onTurnStarted?.({ turnId: 'turn_side_seen', seq: 1 })
    sink!.onDeltas([{ kind: 'agent_message', text: 'already seen', seq: 42, turnId: 'turn_side_seen' }])
    expect(state.sideConversations[id].lastSeq).toBe(1)

    provider.subscribeMock.mockClear()
    await actions.openThreadInSidePanel(id, { parentThreadId: 'thr_main' })

    expect(provider.subscribeMock.mock.calls[0]?.[0]).toBe(id)
    expect(provider.subscribeMock.mock.calls[0]?.[1]).toBe(42)
  })

  it('reconciles side snapshot-required replay gaps from thread detail', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()
    provider.detailResponse = {
      blocks: [
        { kind: 'user', id: 'user_side_1', text: 'side question', meta: { turnId: 'turn_side_snapshot' } },
        { kind: 'assistant', id: 'assistant_side_1', text: 'side complete', meta: { turnId: 'turn_side_snapshot' } }
      ],
      latestSeq: 1034,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_snapshot',
      latestUserMessageId: 'user_side_1'
    }

    sink!.onDeltas([{ kind: 'agent_message', text: 'partial', seq: 12, turnId: 'turn_side_snapshot' }])
    await sink!.onSnapshotRequired?.({
      threadId: id,
      seq: 1034,
      highestSeq: 1034,
      replayEventCount: 1034
    })

    expect(provider.detailMock).toHaveBeenCalledWith(id)
    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: null,
      liveAssistant: '',
      lastSeq: 1034
    })
    expect(state.sideConversations[id].blocks).toEqual([
      { kind: 'user', id: 'user_side_1', text: 'side question', meta: { turnId: 'turn_side_snapshot' } },
      { kind: 'assistant', id: 'assistant_side_1', text: 'side complete', meta: { turnId: 'turn_side_snapshot' } }
    ])
    expect(getActiveStreamSnapshotFor(id, 'turn_side_snapshot').threadId).toBeNull()
  })

  it('recovers and resubscribes side streams when SSE ends while the side turn is still busy', async () => {
    const { actions, state, provider } = buildHarness()
    let subscribeCount = 0
    provider.subscribeMock.mockImplementation(() => {
      subscribeCount += 1
      return subscribeCount === 2 ? Promise.resolve() : undefined
    })
    provider.detailResponse = {
      blocks: [
        {
          kind: 'user',
          id: 'user_recovered',
          createdAt: '2026-06-02T00:00:00.000Z',
          text: 'recover me',
          meta: { turnId: 'turn_recovered' }
        }
      ],
      latestSeq: 42,
      threadStatus: 'running',
      latestTurnId: 'turn_recovered',
      latestUserMessageId: 'user_recovered'
    }

    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_recovered',
      userMessageItemId: 'user_recovered'
    })

    const sent = await actions.sendSideMessage(id, 'recover me')

    expect(sent).toBe(true)
    await waitForAssertion(() => {
      expect(provider.detailMock).toHaveBeenCalledWith(id)
      expect(provider.subscribeMock).toHaveBeenCalledWith(id, 42, expect.anything(), expect.anything())
    })
    expect(state.sideConversations[id]).toMatchObject({
      busy: true,
      turnId: 'turn_recovered',
      userItemId: 'user_recovered',
      lastSeq: 42
    })
  })

  it('reconciles a side completion from thread detail when the final assistant item was missed', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_final',
      userMessageItemId: 'user_side_final'
    })
    provider.detailResponse = {
      blocks: [
        {
          kind: 'user',
          id: 'user_side_final',
          createdAt: '2026-06-02T00:00:00.000Z',
          text: 'finish this',
          meta: { turnId: 'turn_side_final' }
        },
        {
          kind: 'assistant',
          id: 'assistant_side_final',
          createdAt: '2026-06-02T00:00:01.000Z',
          text: 'persisted final answer',
          meta: { turnId: 'turn_side_final' }
        }
      ],
      latestSeq: 33,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_final',
      latestUserMessageId: 'user_side_final'
    }

    const sent = await actions.sendSideMessage(id, 'finish this')
    expect(sent).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    await sink!.onTurnComplete()

    expect(state.sideConversations[id].blocks).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: 'assistant',
          text: 'persisted final answer'
        })
      ])
    )
    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: null,
      lastSeq: 33,
      userItemId: null
    })
  })

  it('blocks a completed side turn when the trusted GET fails and never restores provisional text', async () => {
    const logsError = vi.fn(async () => undefined)
    ;(globalThis as { window?: { analytix?: Record<string, unknown> } }).window = {
      analytix: {
        runtime: {
          runtimeRequest: vi.fn(async () => ({ ok: true, status: 200, body: '{}' }))
        },
        logs: { error: logsError }
      }
    }
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_final',
      userMessageItemId: 'user_side_final'
    })
    vi.spyOn(provider, 'getThreadDetail').mockRejectedValueOnce(
      new Error('provider failed with Authorization: Bearer sk-secret-value')
    )

    const sent = await actions.sendSideMessage(id, 'finish this')
    expect(sent).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onDeltas([
      {
        kind: 'agent_message',
        text: 'FABRICATED_SIDE_SENTINEL',
        seq: 34,
        turnId: 'turn_side_final'
      }
    ])
    await sink!.onTurnComplete()

    await waitForAssertion(() => {
      expect(logsError).toHaveBeenCalledWith(
        'side-terminal-reconcile',
        'Failed to reconcile terminal side turn',
        expect.objectContaining({
          message: expect.not.stringContaining('sk-secret-value'),
          sideId: id,
          turnId: 'turn_side_final',
          userBlockId: 'user_side_final'
        })
      )
    })
    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: 'turn_side_final',
      userItemId: 'user_side_final',
      liveAssistant: '',
      error: 'common:runtimeFinalSnapshotUnavailable'
    })
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)
    expect(getActiveStreamSnapshotFor(id, 'turn_side_final').threadId).toBeNull()
  })

  it('rejects a terminal side snapshot for a different turn identity', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_expected',
      userMessageItemId: 'user_side_expected'
    })
    expect(await actions.sendSideMessage(id, 'stay on this turn')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()
    sink!.onDeltas([
      {
        kind: 'agent_message',
        text: 'FABRICATED_EXPECTED_SENTINEL',
        seq: 35,
        turnId: 'turn_side_expected'
      }
    ])
    provider.detailResponse = {
      blocks: [
        {
          kind: 'assistant',
          id: 'assistant_other_turn',
          text: 'other turn output',
          meta: { turnId: 'turn_side_other' }
        }
      ],
      latestSeq: 36,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_other'
    }

    await sink!.onTurnComplete({ turnId: 'turn_side_expected', seq: 36 })

    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: 'turn_side_expected',
      liveAssistant: '',
      error: 'common:runtimeFinalSnapshotUnavailable'
    })
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)
    expect(JSON.stringify(state.sideConversations[id].blocks)).not.toContain('other turn output')
  })

  it('rejects a case side snapshot whose accepted-final digest differs from the terminal event', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_case',
      userMessageItemId: 'user_side_case'
    })
    expect(await actions.sendSideMessage(id, 'case question')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()
    sink!.onDeltas([{ kind: 'agent_message', text: 'FABRICATED_CASE_SIDE', seq: 37, turnId: 'turn_side_case' }])
    provider.detailResponse = {
      blocks: [
        { kind: 'user', id: 'user_side_case', text: 'case question', meta: { turnId: 'turn_side_case' } },
        { kind: 'assistant', id: 'assistant_side_case', text: 'host final', meta: { turnId: 'turn_side_case' } }
      ],
      latestSeq: 38,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_case',
      latestUserMessageId: 'user_side_case',
      historyAuthority: 'case_boundary_only_v1',
      latestTurnAcceptedFinalDigest: 'a'.repeat(64)
    }

    await sink!.onTurnComplete({
      turnId: 'turn_side_case',
      seq: 38,
      acceptedFinalDigest: 'b'.repeat(64),
      terminalReason: 'success'
    })

    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: 'turn_side_case',
      liveAssistant: '',
      error: 'common:runtimeFinalSnapshotUnavailable'
    })
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)
  })

  it('reconciles a failed terminal side event from GET before exposing any assistant text', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_failed',
      userMessageItemId: 'user_side_failed'
    })
    expect(await actions.sendSideMessage(id, 'fail safely')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()
    sink!.onDeltas([
      {
        kind: 'agent_message',
        text: 'FABRICATED_FAILED_SENTINEL',
        seq: 40,
        turnId: 'turn_side_failed'
      }
    ])
    provider.detailResponse = {
      blocks: [
        {
          kind: 'user',
          id: 'user_side_failed',
          text: 'fail safely',
          meta: { turnId: 'turn_side_failed' }
        },
        {
          kind: 'assistant',
          id: 'assistant_side_failed_boundary',
          text: 'Trusted boundary-only failure response',
          meta: { turnId: 'turn_side_failed' }
        }
      ],
      latestSeq: 41,
      threadStatus: 'failed',
      latestTurnId: 'turn_side_failed',
      latestUserMessageId: 'user_side_failed'
    }

    await sink!.onError(new Error('runtime terminal failure'), { terminal: true })

    expect(provider.detailMock).toHaveBeenCalledWith(id)
    expect(state.sideConversations[id]).toMatchObject({
      busy: false,
      turnId: null,
      liveAssistant: '',
      error: 'runtime terminal failure'
    })
    expect(state.sideConversations[id].blocks).toEqual(provider.detailResponse.blocks)
    expect(JSON.stringify(state.sideConversations[id].blocks)).not.toContain('FABRICATED_FAILED_SENTINEL')
    expect(getActiveStreamSnapshotFor(id, 'turn_side_failed').threadId).toBeNull()
  })

  it('keeps a non-terminal side transport error recoverable without GET or solidifying live text', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    provider.sendMock.mockReturnValueOnce({
      threadId: id,
      turnId: 'turn_side_transport',
      userMessageItemId: 'user_side_transport'
    })
    expect(await actions.sendSideMessage(id, 'keep recovering')).toBe(true)
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()
    sink!.onDeltas([
      {
        kind: 'agent_message',
        text: 'recoverable live text',
        seq: 50,
        turnId: 'turn_side_transport'
      }
    ])

    await sink!.onError(new Error('temporary transport failure'))

    expect(provider.detailMock).not.toHaveBeenCalled()
    expect(state.sideConversations[id]).toMatchObject({
      busy: true,
      turnId: 'turn_side_transport',
      liveAssistant: '',
      error: 'temporary transport failure'
    })
    expect(state.sideConversations[id].blocks.some((block) => block.kind === 'assistant')).toBe(false)
    expect(getActiveStreamSnapshotFor(id, 'turn_side_transport').liveAssistant).toBe('')
  })

  it('uses the Analytix default model when side creation has no parent or composer model to inherit', async () => {
    const { actions, state } = buildHarness({
      threads: [],
      activeThreadId: 'thr_missing',
      composerModel: '',
      composerPickList: []
    })

    const id = await actions.spawnSideConversation()

    expect(id).toBe('side_thr_missing')
    expect(state.sideConversations[id!].model).toBe(DEFAULT_ANALYTIX_MODEL)
  })

  it('a side turn updates only its own blocks/busy and tears down its subscription on close', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!

    // The main thread is still untouched.
    expect(state.blocks).toEqual([])
    expect(state.busy).toBe(true)

    // Send a side message; only the side slice's busy flips.
    const sent = await actions.sendSideMessage(id, 'hi from side')
    expect(sent).toBe(true)
    expect(state.sideConversations[id].busy).toBe(true)
    expect(state.busy).toBe(true)

    // Close tears the subscription (abort() called on the controller).
    const lastCall = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const signal = lastCall?.[3]
    expect(signal?.aborted).toBe(false)
    await actions.closeSideConversation(id)
    expect(state.sideConversations[id]).toBeUndefined()
    expect(signal?.aborted).toBe(true)
    expect(state.busy).toBe(true)
  })

  it('updates one provider progress row in its side conversation only', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const sink = (provider.subscribeMock.mock.calls.at(-1) as [string, number, ThreadEventSink, AbortSignal])[2]
    for (const stage of ['pre_send', 'post_send', 'response_received']) {
      sink.onRuntimeStatus?.({ kind: 'pipeline_stage', itemId: 'runtime_status_side_turn_provider_progress',
        turnId: 'side_turn', stage, label: 'UNTRUSTED_STAGE_TEXT' })
    }
    expect(state.blocks).toEqual([])
    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({ kind: 'system', text: 'Response received' })
  })

  it('side runtime status stays scoped after a cursor update', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onDeltas([{ kind: 'agent_message', text: 'answer first', seq: 13, turnId: 'turn_side_1' }])
    sink!.onSeq(14)
    sink!.onRuntimeStatus?.({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_side_1_provider_retrying_14',
      turnId: 'turn_side_1',
      createdAt: '2026-06-02T00:00:01.000Z',
      stage: 'provider_retrying',
      label: 'Retrying provider stream',
      attempt: 2,
      maxAttempt: 3,
      meta: { stage: 'provider_retrying', attempt: 2, maxAttempt: 3 }
    })

    expect(state.blocks).toEqual([])
    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'system',
      id: 'runtime_status_turn_side_1_provider_retrying_14',
      text: 'Provider request is retrying',
      meta: { turnId: 'turn_side_1', providerRetry: { attempt: 2, maxAttempt: 3 } }
    })
    expect(state.sideConversations[id]).toMatchObject({
      turnId: 'turn_side_1',
      lastSeq: 14
    })
  })

  it('side approval status updates stay scoped to the side conversation', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onApproval({
      approvalId: 'side-approval-1',
      summary: 'Approve side command',
      toolName: 'exec'
    })
    sink!.onApprovalStatus?.({
      approvalId: 'side-approval-1',
      itemId: 'approval-side-approval-1',
      status: 'denied'
    })

    expect(state.blocks).toEqual([])
    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'approval',
      approvalId: 'side-approval-1',
      status: 'denied'
    })
  })

  it('deduplicates replayed side approval requests by stable approval id', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onApproval({
      approvalId: 'side-approval-1',
      summary: 'Approve side command',
      toolName: 'exec',
      turnId: 'turn_side_gate'
    })
    sink!.onApproval({
      approvalId: 'side-approval-1',
      summary: 'Approve side command replayed',
      toolName: 'exec',
      turnId: 'turn_side_gate'
    })
    sink!.onApprovalStatus?.({
      approvalId: 'side-approval-1',
      itemId: 'approval-side-approval-1',
      status: 'allowed'
    })

    const approvalBlocks = state.sideConversations[id].blocks.filter((block) => block.kind === 'approval')
    expect(approvalBlocks).toHaveLength(1)
    expect(approvalBlocks[0]).toMatchObject({
      id: 'approval_side-approval-1',
      approvalId: 'side-approval-1',
      summary: 'Approve side command replayed',
      status: 'allowed',
      meta: { turnId: 'turn_side_gate' }
    })
  })

  it('replaces provisional side text before an approval with the trusted terminal snapshot', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onDeltas([
      { kind: 'agent_message', text: 'answer before approval', seq: 14, turnId: 'turn_side_1' }
    ])
    sink!.onApproval({
      approvalId: 'side-approval-1',
      summary: 'Approve side command',
      toolName: 'exec'
    })
    provider.detailResponse = {
      blocks: [
        {
          kind: 'assistant',
          id: 'assistant_side_approved',
          text: 'trusted persisted answer',
          meta: { turnId: 'turn_side_1' }
        },
        {
          kind: 'approval',
          id: 'approval_side-approval-1',
          approvalId: 'side-approval-1',
          summary: 'Approve side command',
          toolName: 'exec',
          status: 'denied',
          meta: { turnId: 'turn_side_1' }
        }
      ],
      latestSeq: 15,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_1'
    }
    await sink!.onTurnComplete()

    expect('liveReasoning' in state.sideConversations[id]).toBe(false)
    expect(state.sideConversations[id].liveAssistant).toBe('')
    expect(state.sideConversations[id].blocks.map((block) => block.kind)).toEqual(['assistant', 'approval'])
    expect(state.sideConversations[id].blocks[0]).toMatchObject({ text: 'trusted persisted answer' })
    expect(state.sideConversations[id].blocks[0]).not.toMatchObject({ text: 'answer before approval' })
  })

  it('side user-input status updates stay scoped to the side conversation', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onUserInput({
      itemId: 'item_side_input_1',
      requestId: 'side-input-1',
      questions: [
        {
          header: 'Mode',
          id: 'mode',
          question: 'Choose a mode',
          options: [{ label: 'Fast', description: 'Use the faster path' }]
        }
      ]
    })
    sink!.onUserInputStatus({
      itemId: 'item_side_input_1',
      status: 'cancelled',
      errorMessage: 'side input cancelled'
    })

    expect(state.blocks).toEqual([])
    expect(state.sideConversations[id].blocks).toHaveLength(1)
    expect(state.sideConversations[id].blocks[0]).toMatchObject({
      kind: 'user_input',
      id: 'item_side_input_1',
      requestId: 'side-input-1',
      status: 'cancelled',
      errorMessage: 'side input cancelled'
    })
  })

  it('deduplicates replayed side user-input requests and resolves by request id', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    const request = {
      itemId: 'item_side_input_1',
      requestId: 'side-input-1',
      questions: [
        {
          header: 'Mode',
          id: 'mode',
          question: 'Choose a mode',
          options: [{ label: 'Fast', description: 'Use the faster path' }]
        }
      ],
      turnId: 'turn_side_gate'
    }
    sink!.onUserInput(request)
    sink!.onUserInput(request)
    sink!.onUserInputStatus({
      itemId: 'item_side_input_resolved_event',
      requestId: 'side-input-1',
      status: 'submitted'
    })

    const inputBlocks = state.sideConversations[id].blocks.filter((block) => block.kind === 'user_input')
    expect(inputBlocks).toHaveLength(1)
    expect(inputBlocks[0]).toMatchObject({
      id: 'item_side_input_1',
      requestId: 'side-input-1',
      status: 'submitted',
      meta: { turnId: 'turn_side_gate' }
    })
  })

  it('upserts replayed side compaction events by item id', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onCompaction({
      itemId: 'item_side_compaction_1',
      turnId: 'turn_side_compact',
      summary: 'Compacting context',
      status: 'running'
    })
    sink!.onCompaction({
      itemId: 'item_side_compaction_1',
      turnId: 'turn_side_compact',
      summary: 'Context compacted',
      status: 'success',
      detail: 'Kept the important bits.'
    })

    const compactionBlocks = state.sideConversations[id].blocks.filter((block) => block.kind === 'compaction')
    expect(compactionBlocks).toHaveLength(1)
    expect(compactionBlocks[0]).toMatchObject({
      id: 'item_side_compaction_1',
      summary: 'Context compacted',
      status: 'success',
      detail: 'Kept the important bits.',
      meta: { turnId: 'turn_side_compact' }
    })
  })

  it('replaces provisional side text before user input with the trusted terminal snapshot', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const subscription = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const sink = subscription?.[2]
    expect(sink).toBeDefined()

    sink!.onDeltas([
      { kind: 'agent_message', text: 'answer before question', seq: 13, turnId: 'turn_side_1' }
    ])
    sink!.onUserInput({
      itemId: 'item_side_input_1',
      requestId: 'side-input-1',
      questions: [
        {
          header: 'Mode',
          id: 'mode',
          question: 'Choose a mode',
          options: [{ label: 'Fast', description: 'Use the faster path' }]
        }
      ]
    })
    provider.detailResponse = {
      blocks: [
        {
          kind: 'assistant',
          id: 'assistant_side_question',
          text: 'trusted answer before question',
          meta: { turnId: 'turn_side_1' }
        },
        {
          kind: 'user_input',
          id: 'item_side_input_1',
          requestId: 'side-input-1',
          questions: [
            {
              header: 'Mode',
              id: 'mode',
              question: 'Choose a mode',
              options: [{ label: 'Fast', description: 'Use the faster path' }]
            }
          ],
          status: 'cancelled',
          meta: { turnId: 'turn_side_1' }
        }
      ],
      latestSeq: 14,
      threadStatus: 'idle',
      latestTurnId: 'turn_side_1'
    }
    await sink!.onTurnComplete()

    expect(state.sideConversations[id].liveAssistant).toBe('')
    expect(state.sideConversations[id].blocks.map((block) => block.kind)).toEqual([
      'assistant',
      'user_input'
    ])
    expect(state.sideConversations[id].blocks[0]).toMatchObject({ text: 'trusted answer before question' })
    expect(state.sideConversations[id].blocks[0]).not.toMatchObject({ text: 'answer before question' })
  })

  it('promoteSideConversation clears the relation through the provider and refreshes the thread list', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!

    await actions.promoteSideConversation(id)

    expect(provider.relationMock).toHaveBeenCalledWith(id, 'primary')
    expect(provider.refreshThreadsMock).toHaveBeenCalledTimes(1)
    expect(state.sideConversations[id]).toBeUndefined()
  })

  it('discardSideConversation deletes the underlying thread and tears down the subscription', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    const lastCall = provider.subscribeMock.mock.calls.at(-1) as
      | [string, number, ThreadEventSink, AbortSignal]
      | undefined
    const signal = lastCall?.[3]

    await actions.discardSideConversation(id)
    expect(provider.deleteMock).toHaveBeenCalledWith(id)
    expect(state.sideConversations[id]).toBeUndefined()
    expect(signal?.aborted).toBe(true)
  })

  it('side state survives a main-thread switch: closing/discarding the side does not change activeThreadId', async () => {
    const { actions, state, provider } = buildHarness()
    const id = (await actions.spawnSideConversation())!
    // Simulate the user picking a different main thread mid-side.
    state.activeThreadId = 'thr_other'
    state.busy = false
    await actions.closeSideConversation(id)
    expect(state.activeThreadId).toBe('thr_other')
    expect(state.busy).toBe(false)
  })
})
