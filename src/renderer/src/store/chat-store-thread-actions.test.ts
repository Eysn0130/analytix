import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NormalizedThread, ThreadEventSink } from '../agent/types'
import type { ChatState, ChatStoreGet, ChatStoreSet, GuiPlanMessageContext, QueuedUserMessage } from './chat-store-types'
import type { ThreadHandoffOperation } from '@shared/thread-handoff'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { readThreadWorktreeRegistry } from '../lib/thread-worktree-registry'
import { formatRuntimeError } from '../lib/format-runtime-error'
import i18n from '../i18n'
import { useWriteWorkspaceStore } from '../write/write-workspace-store'
import { composeWritePrompt } from '../write/quoted-selection'
import { emptySelection } from '../write/write-workspace-store-helpers'
import { objectEditingRequestSchema, objectEditingResponseSchema, type ObjectEditingRequest } from '../../../../packages/runtime/src/contracts/object-editing'

vi.mock('electron', () => ({ clipboard: {} }))

const registryMock = vi.hoisted(() => ({
  getProvider: vi.fn()
}))

vi.mock('../agent/registry', () => ({
  getProvider: registryMock.getProvider
}))

import { createThreadActions } from './chat-store-thread-actions'
import {
  clearActiveStream,
  getActiveStreamSnapshotFor,
  isPendingActiveStreamTurnId,
  resetActiveStream
} from '../thread/streaming/active-stream-store'

function thread(id: string): NormalizedThread {
  return {
    id,
    title: id,
    updatedAt: '2026-06-09T00:00:00.000Z',
    model: 'deepseek-v4-pro',
    mode: 'agent',
    workspace: '/workspace/analytix',
    status: 'running'
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

function buildHarness(): {
  actions: ReturnType<typeof createThreadActions>
  sseAbortRef: { current: AbortController | null }
  state: ChatState
} {
  let state: ChatState
  state = {
    activeThreadId: 'thr_existing',
    blocks: [],
    busy: true,
    clawChannels: [],
    codeWorkspaceRoots: [],
    composerModel: '',
    composerProviderId: '',
    currentTurnId: null,
    currentTurnUserId: null,
    error: 'previous error',
    lastSeq: 0,
    loadComposerModels: vi.fn(async () => undefined),
    queuedMessages: [],
    drainQueuedMessages: vi.fn(async () => undefined),
    recoverActiveTurn: vi.fn(async () => true),
    refreshThreads: vi.fn(async () => undefined),
    route: 'chat',
    runtimeConnection: 'ready',
    turnDurationByUserId: {},
    turnStartedAtByUserId: {},
    threads: [thread('thr_existing')]
  } as unknown as ChatState

  const set: ChatStoreSet = (partial) => {
    const update = typeof partial === 'function' ? partial(state) : partial
    Object.assign(state, update)
  }
  const get: ChatStoreGet = () => state
  const sseAbortRef = { current: null as AbortController | null }
  const actions = createThreadActions({
    set,
    get,
    sseAbortRef
  })
  state.sendMessage = actions.sendMessage
  return { actions, sseAbortRef, state }
}

async function waitForAssertion(assertion: () => void): Promise<void> {
  let lastError: unknown
  for (let attempt = 0; attempt < 12; attempt += 1) {
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

describe('chat-store-thread-actions queued messages', () => {
  it('opens, edits, saves and reopens a real synthetic file, then sends its selection in the existing conversation', async () => {
    // Load host fixtures at runtime so Node/Electron ambient globals do not
    // change the renderer's browser type environment.
    const { mkdtemp, readFile, writeFile, rm } = await vi.importActual<{
      mkdtemp: (prefix: string) => Promise<string>
      readFile: (path: string, encoding: 'utf8') => Promise<string>
      writeFile: (path: string, content: string) => Promise<void>
      rm: (path: string, options: { recursive: boolean; force: boolean }) => Promise<void>
    }>('node:fs/promises')
    const { tmpdir } = await vi.importActual<{ tmpdir: () => string }>('node:os')
    const { createHash } = await vi.importActual<{ createHash: (algorithm: string) => { update: (value: string) => { digest: (encoding: 'hex') => string } } }>('node:crypto')
    const hash = (value: string) => createHash('sha256').update(value).digest('hex')
    const root = await mkdtemp(`${tmpdir()}/analytix-workbench-roundtrip-`)
    const filePath = `${root}/report.md`
    const sessionId = 'a'.repeat(48), objectId = hash(filePath)
    const receipts = new Map<string, ReturnType<typeof objectEditingResponseSchema.parse>>()
    const objectRequest = vi.fn(async (input: ObjectEditingRequest) => {
      const request = objectEditingRequestSchema.parse(input)
      if (request.action === 'open') {
        expect(request).toEqual({action:'open',workspace:root,path:filePath})
        const content = await readFile(filePath, 'utf8')
        return objectEditingResponseSchema.parse({ok:true,document:{sessionId,objectId,path:filePath,content,revision:hash(content)}})
      }
      if (request.action === 'close') {
        expect(request.sessionId).toBe(sessionId)
        return objectEditingResponseSchema.parse({ok:true,closed:true})
      }
      if (request.action !== 'commit') throw new Error('Unexpected synthetic object operation')
      expect(request.sessionId).toBe(sessionId)
      const replay = receipts.get(request.operationId)
      if (replay) return replay
      expect(request.baseRevision).toBe(hash(await readFile(filePath, 'utf8')))
      await writeFile(filePath, request.content)
      const result = objectEditingResponseSchema.parse({ok:true,receipt:{operationId:request.operationId,revision:hash(request.content),status:'committed',savedAt:'2026-09-14T00:00:00Z'}})
      receipts.set(request.operationId,result)
      return result
    })
    const legacyRead = vi.fn(), legacyWrite = vi.fn()
    const provider = {
      sendUserMessage: vi.fn(async (_threadId: string, _text: string, _options: unknown) => ({ threadId: 'thr_existing', turnId: 'turn_roundtrip', userMessageItemId: 'user_roundtrip' })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', { analytix: {
      objects:{request:objectRequest}, files: { read:legacyRead, write:legacyWrite },
      settings: { getSettings: vi.fn(async () => ({ runtime: { providerId: 'synthetic', model: 'synthetic' }, codePromptPrefix: '' })) },
      logs: { error: vi.fn(async () => undefined) }
    } })
    try {
      await writeFile(filePath, '# 合成报告\n\n原始段落\n')
      useWriteWorkspaceStore.getState().resetWorkspace()
      useWriteWorkspaceStore.setState({ workspaceRoot: root })
      await useWriteWorkspaceStore.getState().openFile(root, filePath)
      expect(useWriteWorkspaceStore.getState().fileContent).toContain('原始段落')
      const content = '# 合成报告\n\n已编辑的中文段落\n'
      useWriteWorkspaceStore.getState().setFileContent(content)
      expect(await useWriteWorkspaceStore.getState().flushSave(root)).toBe(true)
      expect(await readFile(filePath, 'utf8')).toBe(content)
      expect(await useWriteWorkspaceStore.getState().openWorkspaceHome(root)).toBe(true)
      await useWriteWorkspaceStore.getState().openFile(root, filePath)
      expect(useWriteWorkspaceStore.getState().fileContent).toBe(content)
      const selected = '已编辑的中文段落'
      const from = content.indexOf(selected)
      useWriteWorkspaceStore.getState().setSelection({ ...emptySelection(), text: selected, charCount: selected.length,
        ranges: [{ from, to: from + selected.length, text: selected, startLine: 3, endLine: 3, startColumn: 1, endColumn: selected.length + 1, charCount: selected.length }] })
      useWriteWorkspaceStore.getState().quoteCurrentSelection(root)
      const quotes = useWriteWorkspaceStore.getState().quotedSelections
      expect(quotes).toHaveLength(1)
      expect(quotes[0].snapshotContent).toBe(content)
      const prompt = composeWritePrompt('解释这个选区', quotes, { workspaceRoot: root, activeFilePath: filePath })
      const { actions, state } = buildHarness()
      state.busy = false
      state.threads[0].workspace = root
      expect(await actions.sendMessage(prompt, 'agent')).toBe(true)
      expect(state.activeThreadId).toBe('thr_existing')
      expect(provider.sendUserMessage).toHaveBeenCalledWith('thr_existing', expect.stringContaining(selected), expect.any(Object))
      expect(provider.sendUserMessage.mock.calls[0][1]).not.toContain('snapshotContent')
      expect(objectRequest.mock.calls.filter(([request]) => request.action === 'commit')).toHaveLength(1)
      expect(useWriteWorkspaceStore.getState().objectSession?.revision).toBe(hash(content))
      expect(legacyRead).not.toHaveBeenCalled()
      expect(legacyWrite).not.toHaveBeenCalled()
    } finally {
      useWriteWorkspaceStore.getState().resetWorkspace()
      await rm(root, { recursive: true, force: true })
    }
  })

  beforeEach(() => {
    rendererRuntimeClient.invalidateSettings()
    registryMock.getProvider.mockReset()
    registryMock.getProvider.mockReturnValue({})
  })

  afterEach(() => {
    clearActiveStream()
    rendererRuntimeClient.invalidateSettings()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('does not queue GUI plan messages while another turn is active', async () => {
    const { actions, state } = buildHarness()
    const guiPlan: GuiPlanMessageContext = {
      operation: 'draft',
      workspaceRoot: '/workspace/analytix',
      relativePath: '.analytixsdd/plan/feature.md',
      planId: 'plan-1',
      sourceRequest: 'feature'
    }

    await expect(actions.sendMessage('prompt one', 'plan', {
      displayText: 'Generate implementation plan',
      guiPlan
    })).resolves.toBe(false)

    expect(state.queuedMessages).toHaveLength(0)
    expect(state.error).toBeTruthy()
  })

  it('steers a running turn instead of queueing or interrupting when the composer sends text while busy', async () => {
    const abort = vi.fn()
    const provider = {
      steerUserMessage: vi.fn(async (input: { threadId: string; turnId: string; text: string; clientUserMessageId?: string }) => ({
        threadId: input.threadId,
        turnId: input.turnId,
        clientUserMessageId: input.clientUserMessageId,
        admittedSeq: 42
      })),
      interruptTurn: vi.fn(),
      sendUserMessage: vi.fn()
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, sseAbortRef, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.currentTurnId = 'turn_active'
    state.busy = true
    sseAbortRef.current = { abort } as unknown as AbortController

    await expect(actions.sendMessage('  add this constraint  ', 'agent')).resolves.toBe(true)

    expect(provider.steerUserMessage).toHaveBeenCalledWith(expect.objectContaining({
      threadId: 'thr_existing',
      turnId: 'turn_active',
      expectedTurnId: 'turn_active',
      text: 'add this constraint'
    }))
    expect(state.queuedMessages).toEqual([])
    expect(provider.sendUserMessage).not.toHaveBeenCalled()
    expect(provider.interruptTurn).not.toHaveBeenCalled()
    expect(abort).not.toHaveBeenCalled()
    expect(state.busy).toBe(true)
    expect(state.blocks).toEqual([
      expect.objectContaining({
        kind: 'user',
        text: 'add this constraint',
        meta: expect.objectContaining({
          turnId: 'turn_active',
          steeringStatus: 'admitted',
          admittedSeq: 42
        })
      })
    ])
  })

  it('sends sensitive steering input exactly once but stores only its ordinary projection', async () => {
    const account = '6222020202020202020'
    const provider = {
      steerUserMessage: vi.fn(async (input: { threadId: string; turnId: string; text: string }) => ({
        threadId: input.threadId,
        turnId: input.turnId,
        admittedSeq: 43
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.currentTurnId = 'turn_sensitive'
    state.busy = true

    await expect(actions.sendMessage(`查询银行卡号 ${account}`, 'agent')).resolves.toBe(true)

    expect(provider.steerUserMessage).toHaveBeenCalledWith(expect.objectContaining({
      text: `查询银行卡号 ${account}`
    }))
    expect(state.queuedMessages).toEqual([])
    expect(JSON.stringify(state.blocks)).not.toContain(account)
    expect(state.blocks).toEqual([
      expect.objectContaining({ kind: 'user', text: '查询银行卡号 [ACCOUNT]' })
    ])
  })

  it('keeps closed-ref user input exact for the private host while withholding public renderer state', async () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const exactInput = `inspect private binding ${authorityRef}`
    const provider = {
      steerUserMessage: vi.fn(async (input: { threadId: string; turnId: string; text: string }) => ({
        threadId: input.threadId,
        turnId: input.turnId,
        admittedSeq: 44
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.currentTurnId = 'turn_private_ref'
    state.busy = true

    await expect(actions.sendMessage(exactInput, 'agent')).resolves.toBe(true)

    expect(provider.steerUserMessage).toHaveBeenCalledWith(expect.objectContaining({ text: exactInput }))
    expect(state.queuedMessages).toEqual([])
    expect(JSON.stringify(state.blocks)).not.toContain(authorityRef)
    expect(state.blocks).toEqual([
      expect.objectContaining({ kind: 'user', text: '' })
    ])
  })

  it('fails closed instead of storing sensitive input in the renderer queue', async () => {
    const account = '6222020202020202020'
    const { actions, state } = buildHarness()
    state.currentTurnId = 'turn_no_steer'
    state.busy = true

    await expect(actions.sendMessage(`查询银行卡号 ${account}`, 'agent')).resolves.toBe(false)

    expect(state.queuedMessages).toEqual([])
    expect(JSON.stringify(state)).not.toContain(account)
    expect(state.error).toBe(i18n.t('common:queuedMessageSensitiveInputBlocked'))
  })

  it('quarantines a legacy sensitive queue entry without dispatching its projected preview', async () => {
    const account = '6222020202020202020'
    const { actions, state } = buildHarness()
    const sendMessage = vi.fn(async () => true)
    state.busy = false
    state.sendMessage = sendMessage as unknown as ChatState['sendMessage']
    state.queuedMessages = [{ id: 'q-sensitive', text: `account=${account}` }]

    await actions.drainQueuedMessages()

    expect(sendMessage).not.toHaveBeenCalled()
    expect(state.queuedMessages).toEqual([])
    expect(state.queuedMessagesPausedReason).toBe('failed')
    expect(state.error).toBe(i18n.t('common:queuedMessageSensitiveInputBlocked'))
  })

  it('keeps attachment text and preview bytes out of optimistic blocks and queues', async () => {
    const sentinel = '6222020202020202020'
    const { actions, state } = buildHarness()
    state.currentTurnId = null
    state.busy = true

    await expect(actions.sendMessage('inspect attachment', 'agent', {
      attachmentIds: ['att_0123456789abcdef01234567'],
      attachments: [{
        id: 'att_0123456789abcdef01234567',
        kind: 'document',
        name: 'statement.pdf',
        mimeType: 'application/pdf',
        documentText: sentinel,
        textPreview: sentinel,
        previewUrl: `data:application/pdf;base64,${sentinel}`
      }]
    })).resolves.toBe(true)

    expect(state.queuedMessages).toHaveLength(1)
    expect(JSON.stringify(state.queuedMessages)).not.toContain(sentinel)
    expect(state.queuedMessages[0]?.attachments?.[0]).toEqual({
      id: 'att_0123456789abcdef01234567',
      kind: 'document',
      name: 'statement.pdf',
      mimeType: 'application/pdf'
    })
  })

  it('removes stale queued GUI plan messages before draining normal queued messages', async () => {
    const { actions, state } = buildHarness()
    const sendMessage = vi.fn(async (_text, _mode, overrides) => {
      state.queuedMessages = state.queuedMessages.filter((message) => message.id !== overrides?.queued?.id)
      return true
    })
    state.busy = false
    state.sendMessage = sendMessage as unknown as ChatState['sendMessage']
    state.queuedMessages = [
      {
        id: 'q-plan',
        text: 'internal plan prompt',
        mode: 'plan',
        guiPlan: {
          operation: 'draft',
          workspaceRoot: '/workspace/analytix',
          relativePath: '.analytixsdd/plan/one.md',
          planId: 'plan-1'
        }
      },
      {
        id: 'q-user',
        text: 'normal follow-up',
        mode: 'agent'
      }
    ]

    await actions.drainQueuedMessages()

    expect(state.queuedMessages).toEqual([])
    expect(sendMessage).toHaveBeenCalledWith('normal follow-up', 'agent', {
      queued: expect.objectContaining({ id: 'q-user' })
    })
  })

  it('edits, removes, and reorders queued messages without dropping metadata', () => {
    const { actions, state } = buildHarness()
    state.queuedMessages = [
      {
        id: 'q-a',
        text: 'first',
        mode: 'agent',
        model: 'deepseek-v4-pro',
        providerId: 'deepseek',
        reasoningEffort: 'high',
        attachmentIds: ['att-1'],
        attachments: [{
          id: 'att-1',
          name: 'shot.png',
          mimeType: 'image/png',
          localFilePath: '/tmp/shot.png'
        }],
        fileReferences: [{
          path: '/workspace/analytix/src/App.tsx',
          relativePath: 'src/App.tsx',
          name: 'App.tsx',
          kind: 'file'
        }]
      },
      {
        id: 'q-b',
        text: 'second',
        mode: 'plan',
        model: 'gpt-5',
        providerId: 'openai'
      }
    ]
    state.queuedMessagesPausedReason = 'failed'

    actions.editQueuedMessage('q-a', ' revised first ')
    actions.reorderQueuedMessages(['q-b', 'q-a'])
    actions.removeQueuedMessage('q-b')

    expect(state.queuedMessages).toEqual([
      expect.objectContaining({
        id: 'q-a',
        text: 'revised first',
        mode: 'agent',
        model: 'deepseek-v4-pro',
        providerId: 'deepseek',
        reasoningEffort: 'high',
        attachmentIds: ['att-1'],
        attachments: [expect.objectContaining({ id: 'att-1' })],
        fileReferences: [expect.objectContaining({ relativePath: 'src/App.tsx' })]
      })
    ])
    expect(state.queuedMessagesPausedReason).toBe('failed')
  })

  it('sends a queued message immediately with its original metadata', async () => {
    const { actions, state } = buildHarness()
    const queued: QueuedUserMessage = {
      id: 'q-now',
      text: 'send now',
      mode: 'agent' as const,
      model: 'deepseek-v4-pro',
      providerId: 'deepseek',
      reasoningEffort: 'medium',
      fileReferences: [{
        path: '/workspace/analytix/src/App.tsx',
        relativePath: 'src/App.tsx',
        name: 'App.tsx',
        kind: 'file' as const
      }]
    }
    const sendMessage = vi.fn(async (_text, _mode, overrides) => {
      state.queuedMessages = state.queuedMessages.filter((message) => message.id !== overrides?.queued?.id)
      return true
    })
    state.busy = false
    state.queuedMessages = [queued]
    state.queuedMessagesPausedReason = 'interrupted'
    state.sendMessage = sendMessage as unknown as ChatState['sendMessage']

    await expect(actions.sendQueuedMessageNow('q-now')).resolves.toBe(true)

    expect(sendMessage).toHaveBeenCalledWith('send now', 'agent', {
      queued: expect.objectContaining({
        id: 'q-now',
        model: 'deepseek-v4-pro',
        providerId: 'deepseek',
        reasoningEffort: 'medium',
        fileReferences: [expect.objectContaining({ relativePath: 'src/App.tsx' })]
      })
    })
    expect(state.queuedMessages).toEqual([])
    expect(state.queuedMessagesPausedReason).toBeNull()
  })

  it('keeps an explicit queued message when immediate steer admission fails', async () => {
    const provider = {
      steerUserMessage: vi.fn(async () => {
        throw new Error('turn inactive')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    const queued = {
      id: 'q-steer-fail',
      text: 'retry this follow-up',
      mode: 'agent' as const,
      fileReferences: [{
        path: '/workspace/analytix/src/App.tsx',
        relativePath: 'src/App.tsx',
        name: 'App.tsx',
        kind: 'file' as const
      }]
    }
    state.busy = true
    state.activeThreadId = 'thr_existing'
    state.currentTurnId = 'turn_active'
    state.queuedMessages = [queued]

    await expect(actions.sendQueuedMessageNow('q-steer-fail')).resolves.toBe(false)

    expect(provider.steerUserMessage).toHaveBeenCalled()
    expect(state.blocks).toEqual([])
    expect(state.queuedMessages).toEqual([queued])
    expect(state.queuedMessagesPausedReason).toBe('failed')
    expect(state.error).toBe(formatRuntimeError(new Error('turn inactive')))
  })

  it('keeps and pauses a queued message when immediate steer is unsupported while busy', async () => {
    registryMock.getProvider.mockReturnValue({})
    const { actions, state } = buildHarness()
    const queued = { id: 'q-no-steer', text: 'retry later', mode: 'agent' as const }
    state.busy = true
    state.activeThreadId = 'thr_existing'
    state.currentTurnId = 'turn_active'
    state.queuedMessages = [queued]

    await expect(actions.sendQueuedMessageNow('q-no-steer')).resolves.toBe(false)

    expect(state.blocks).toEqual([])
    expect(state.queuedMessages).toEqual([queued])
    expect(state.queuedMessagesPausedReason).toBe('failed')
  })

  it('does not drain queued messages while the queue is paused', async () => {
    const { actions, state } = buildHarness()
    const sendMessage = vi.fn(async () => true)
    state.busy = false
    state.queuedMessagesPausedReason = 'interrupted'
    state.queuedMessages = [{ id: 'q-paused', text: 'later', mode: 'agent' }]
    state.sendMessage = sendMessage as unknown as ChatState['sendMessage']

    await actions.drainQueuedMessages()

    expect(sendMessage).not.toHaveBeenCalled()
    expect(state.queuedMessages).toEqual([{ id: 'q-paused', text: 'later', mode: 'agent' }])
  })

  it('blocks thread handoff while queued messages exist', async () => {
    const startThreadHandoff = vi.fn()
    vi.stubGlobal('window', {
      analytix: {
        workspace: { startThreadHandoff },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.queuedMessages = [{ id: 'q-blocker', text: 'finish me first', mode: 'agent' }]
    state.threadHandoffOperations = []

    await expect(actions.startThreadHandoff({
      direction: 'to-worktree',
      sourceThreadId: 'thr_existing',
      sourceWorkspace: '/workspace/analytix'
    })).resolves.toBe(false)

    expect(startThreadHandoff).not.toHaveBeenCalled()
    expect(state.error).toBeTruthy()
  })

  it('upserts thread handoff operations from real event statuses', () => {
    const { actions, state } = buildHarness()
    state.threadHandoffOperations = []
    state.activeThreadHandoffOperationId = null
    const operation = (status: ThreadHandoffOperation['status']): ThreadHandoffOperation => ({
      id: 'handoff-1',
      direction: 'to-worktree',
      status,
      sourceThreadId: 'thr_existing',
      targetThreadId: null,
      sourceWorkspace: '/workspace/analytix',
      targetWorkspace: '/workspace/analytix-worktrees/pool-0',
      sourceBranch: 'main',
      localBranch: 'main',
      worktreeBranch: 'analytix-pool-0',
      request: {
        direction: 'to-worktree',
        sourceThreadId: 'thr_existing',
        sourceWorkspace: '/workspace/analytix'
      },
      steps: [
        { id: 'create-new-worktree', status: status === 'running' ? 'running' : status === 'queued' ? 'pending' : 'done' },
        { id: 'switching-thread', status: status === 'success' ? 'done' : 'pending' }
      ],
      errorMessage: status === 'error' ? 'boom' : null,
      warningMessage: status === 'warning' ? 'check manually' : null,
      execOutput: status === 'error' ? { output: 'git failed' } : null,
      hasUnseenTerminalState: status === 'error' || status === 'warning' || status === 'success',
      worktree: null,
      createdAt: '2026-06-29T00:00:00.000Z',
      updatedAt: `2026-06-29T00:00:0${status.length}.000Z`
    })

    actions.handleThreadHandoffEvent(operation('queued'))
    expect(state.threadHandoffOperations[0].status).toBe('queued')
    actions.handleThreadHandoffEvent(operation('running'))
    expect(state.threadHandoffOperations[0].status).toBe('running')
    actions.handleThreadHandoffEvent(operation('warning'))
    expect(state.threadHandoffOperations[0].status).toBe('warning')
    actions.handleThreadHandoffEvent(operation('error'))
    expect(state.threadHandoffOperations[0]).toMatchObject({ status: 'error', errorMessage: 'boom' })
    actions.handleThreadHandoffEvent(operation('success'))
    expect(state.threadHandoffOperations[0].status).toBe('success')
    expect(state.activeThreadHandoffOperationId).toBe('handoff-1')
  })

  it('finalizes a worktree handoff switch and records the worktree registry only after renderer workspace update succeeds', async () => {
    const localStorage = memoryStorage()
    const updateThreadWorkspace = vi.fn(async () => undefined)
    registryMock.getProvider.mockReturnValue({ updateThreadWorkspace })
    let completedOperation: ThreadHandoffOperation | null = null
    const completeThreadHandoffSwitch = vi.fn(async (params: { operationId: string; targetThreadId?: string }) => {
      if (!completedOperation) throw new Error('missing operation')
      return {
        operation: {
          ...completedOperation,
          status: 'success',
          targetThreadId: params.targetThreadId ?? null,
          steps: completedOperation.steps.map((step) =>
            step.id === 'switching-thread' ? { ...step, status: 'done' as const } : step
          )
        }
      }
    })
    vi.stubGlobal('window', {
      localStorage,
      analytix: {
        workspace: {
          completeThreadHandoffSwitch,
          failThreadHandoffSwitch: vi.fn()
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.threads = [thread('thr_existing')]
    state.threadHandoffOperations = []
    const operation: ThreadHandoffOperation = {
      id: 'handoff-finalize',
      direction: 'to-worktree',
      status: 'running',
      sourceThreadId: 'thr_existing',
      targetThreadId: null,
      sourceWorkspace: '/workspace/analytix',
      targetWorkspace: '/workspace/analytix-worktrees/pool-1',
      sourceBranch: 'main',
      localBranch: 'main',
      worktreeBranch: 'analytix-pool-1',
      request: {
        direction: 'to-worktree',
        sourceThreadId: 'thr_existing',
        sourceWorkspace: '/workspace/analytix'
      },
      steps: [
        { id: 'create-new-worktree', status: 'done' },
        { id: 'switching-thread', status: 'running' }
      ],
      errorMessage: null,
      warningMessage: null,
      execOutput: null,
      hasUnseenTerminalState: false,
      worktree: {
        poolIndex: 1,
        path: '/workspace/analytix-worktrees/pool-1',
        branch: 'analytix-pool-1',
        inUse: true,
        taskId: 'handoff-finalize',
        baseCommit: 'abc',
        changesCount: 0
      },
      createdAt: '2026-06-29T00:00:00.000Z',
      updatedAt: '2026-06-29T00:00:01.000Z'
    }
    completedOperation = operation

    actions.handleThreadHandoffEvent(operation)
    await new Promise((resolve) => setTimeout(resolve, 0))
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(updateThreadWorkspace).toHaveBeenCalledWith('thr_existing', '/workspace/analytix-worktrees/pool-1')
    expect(completeThreadHandoffSwitch).toHaveBeenCalledWith({
      operationId: 'handoff-finalize',
      targetThreadId: 'thr_existing'
    })
    expect(state.threads[0].workspace).toBe('/workspace/analytix-worktrees/pool-1')
    expect(readThreadWorktreeRegistry(localStorage).worktrees.thr_existing).toMatchObject({
      projectPath: '/workspace/analytix',
      poolIndex: 1,
      worktreePath: '/workspace/analytix-worktrees/pool-1',
      branch: 'analytix-pool-1'
    })
    expect(state.threadHandoffOperations[0].status).toBe('success')
  })

  it('fails the handoff switch when renderer workspace update fails without recording the worktree', async () => {
    const localStorage = memoryStorage()
    const updateThreadWorkspace = vi.fn(async () => {
      throw new Error('workspace update failed')
    })
    registryMock.getProvider.mockReturnValue({ updateThreadWorkspace })
    const failThreadHandoffSwitch = vi.fn(async (params: { operationId: string; message: string }) => ({
      operation: {
        ...operation,
        status: 'error' as const,
        errorMessage: params.message,
        execOutput: { output: params.message },
        hasUnseenTerminalState: true,
        steps: operation.steps.map((step) =>
          step.id === 'switching-thread' ? { ...step, status: 'failed' as const } : step
        )
      }
    }))
    vi.stubGlobal('window', {
      localStorage,
      analytix: {
        workspace: {
          completeThreadHandoffSwitch: vi.fn(),
          failThreadHandoffSwitch
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.threads = [thread('thr_existing')]
    state.threadHandoffOperations = []
    const operation: ThreadHandoffOperation = {
      id: 'handoff-finalize-fail',
      direction: 'to-worktree',
      status: 'running',
      sourceThreadId: 'thr_existing',
      targetThreadId: null,
      sourceWorkspace: '/workspace/analytix',
      targetWorkspace: '/workspace/analytix-worktrees/pool-2',
      sourceBranch: 'main',
      localBranch: 'main',
      worktreeBranch: 'analytix-pool-2',
      request: {
        direction: 'to-worktree',
        sourceThreadId: 'thr_existing',
        sourceWorkspace: '/workspace/analytix'
      },
      steps: [
        { id: 'create-new-worktree', status: 'done' },
        { id: 'switching-thread', status: 'running' }
      ],
      errorMessage: null,
      warningMessage: null,
      execOutput: null,
      hasUnseenTerminalState: false,
      worktree: {
        poolIndex: 2,
        path: '/workspace/analytix-worktrees/pool-2',
        branch: 'analytix-pool-2',
        inUse: true,
        taskId: 'handoff-finalize-fail',
        baseCommit: 'def',
        changesCount: 0
      },
      createdAt: '2026-06-29T00:00:00.000Z',
      updatedAt: '2026-06-29T00:00:01.000Z'
    }

    actions.handleThreadHandoffEvent(operation)
    await new Promise((resolve) => setTimeout(resolve, 0))
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(updateThreadWorkspace).toHaveBeenCalledWith('thr_existing', '/workspace/analytix-worktrees/pool-2')
    expect(failThreadHandoffSwitch).toHaveBeenCalledWith({
      operationId: 'handoff-finalize-fail',
      message: formatRuntimeError(new Error('workspace update failed'))
    })
    expect(state.threads[0].workspace).toBe('/workspace/analytix')
    expect(readThreadWorktreeRegistry(localStorage).worktrees.thr_existing).toBeUndefined()
    expect(state.threadHandoffOperations[0]).toMatchObject({
      status: 'error',
      errorMessage: formatRuntimeError(new Error('workspace update failed')),
      hasUnseenTerminalState: true
    })
    expect(state.error).toBe(formatRuntimeError(new Error('workspace update failed')))
  })

  it('forwards the selected composer provider to the runtime turn without restarting', async () => {
    const provider = {
      connect: vi.fn(async () => undefined),
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(() => new Promise<void>(() => undefined))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const saveSettingsSilent = vi.fn(async () => ({
      runtime: { providerId: 'xiaomi-token-plan', model: 'mimo-v2.5' },
      codePromptPrefix: ''
    }))
    const restartRuntime = vi.fn(async () => undefined)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            runtime: { providerId: 'minimax-token-plan', model: 'MiniMax-M2' },
            codePromptPrefix: ''
          })),
          saveSettingsSilent
        },
        runtime: { restartRuntime },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, sseAbortRef, state } = buildHarness()
    state.busy = false
    state.composerModel = 'mimo-v2.5'
    state.composerProviderId = 'xiaomi-token-plan'

    await expect(actions.sendMessage('hello', 'agent')).resolves.toBe(true)

    expect(saveSettingsSilent).not.toHaveBeenCalled()
    expect(restartRuntime).not.toHaveBeenCalled()
    expect(provider.connect).not.toHaveBeenCalled()
    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_existing',
      'hello',
      expect.objectContaining({ model: 'mimo-v2.5', providerId: 'xiaomi-token-plan' })
    )
    expect(state.blocks).toEqual([
      expect.objectContaining({
        kind: 'user',
        id: 'user_1',
        text: 'hello',
        meta: expect.objectContaining({ turnId: 'turn_1' })
      })
    ])
    expect(state.busy).toBe(true)
    expect(state.currentTurnId).toBe('turn_1')
    expect(state.currentTurnUserId).toBe('user_1')
    expect(sseAbortRef.current).toBeInstanceOf(AbortController)
    expect(provider.subscribeThreadEvents).toHaveBeenCalledWith(
      'thr_existing',
      0,
      expect.any(Object),
      sseAbortRef.current?.signal
    )
    expect(provider.subscribeThreadEvents.mock.invocationCallOrder[0]).toBeLessThan(
      provider.sendUserMessage.mock.invocationCallOrder[0]
    )
    expect(state.refreshThreads).toHaveBeenCalledTimes(1)
  })

  it('does not infer case risk from workspace membership for an ordinary turn', async () => {
    const provider = {
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.busy = false
    state.caseProjectThreadsById = {
      case_workspace: [{ ...thread('thr_existing'), status: 'idle' }]
    }

    await expect(actions.sendMessage('update the structured Todo', 'agent')).resolves.toBe(true)

    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_existing',
      'update the structured Todo',
      expect.not.objectContaining({ riskIntent: 'case' })
    )
  })

  it('clears a terminal SSE ref so the next send resubscribes from the latest seq', async () => {
    const provider = {
      connect: vi.fn(async () => undefined),
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: `turn_${provider.sendUserMessage.mock.calls.length + 1}`,
        userMessageItemId: `user_${provider.sendUserMessage.mock.calls.length + 1}`
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            runtime: { providerId: 'deepseek', model: 'deepseek-chat' },
            codePromptPrefix: ''
          }))
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, sseAbortRef, state } = buildHarness()
    state.busy = false
    state.lastSeq = 7

    await expect(actions.sendMessage('first', 'agent')).resolves.toBe(true)
    await waitForAssertion(() => expect(sseAbortRef.current).toBeNull())

    state.busy = false
    state.currentTurnId = null
    state.currentTurnUserId = null
    state.lastSeq = 12

    await expect(actions.sendMessage('second', 'agent')).resolves.toBe(true)

    const subscribeCalls = provider.subscribeThreadEvents.mock.calls as unknown as Array<
      [string, number, ThreadEventSink, AbortSignal]
    >
    expect(subscribeCalls.map((call) => call[1])).toEqual([7, 7, 12, 12])
  })

  it('does not let a stale thread detail response overwrite live SSE events', async () => {
    const provider = {
      subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: {
        onDeltas(deltas: Array<{ kind: 'agent_message'; text: string; seq: number }>): void
      }) => {
        sink.onDeltas([{ kind: 'agent_message', text: 'live answer', seq: 1 }])
        return new Promise<void>(() => undefined)
      }),
      getThreadDetail: vi.fn(async () => ({
        blocks: [{ kind: 'user', id: 'user_stale', text: 'stale prompt' }],
        latestSeq: 0,
        threadStatus: 'running',
        latestTurnId: 'turn_stale',
        latestUserMessageId: 'user_stale',
        turnDurationByUserId: {}
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.blocks = []
    state.lastSeq = 0

    await actions.subscribeThreadEventsLive('thr_existing')

    expect(state.lastSeq).toBe(1)
    expect(state.liveAssistant ?? '').toBe('')
    expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_stale' })])
  })

  it('withholds live assistant drafts while getThreadDetail is pending and after stale detail returns', async () => {
    let resolveDetail: (value: {
      blocks: Array<{ kind: 'user'; id: string; text: string }>
      latestSeq: number
      threadStatus: string
      latestTurnId: string
      latestUserMessageId: string
      turnDurationByUserId: Record<string, number>
    }) => void
    const detailPromise = new Promise<Parameters<typeof resolveDetail>[0]>((resolve) => {
      resolveDetail = resolve
    })
    const provider = {
      subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: {
        onDeltas(deltas: Array<{ kind: 'agent_message'; text: string; seq: number }>): void
      }) => {
        sink.onDeltas([{ kind: 'agent_message', text: 'live answer', seq: 3 }])
        return new Promise<void>(() => undefined)
      }),
      getThreadDetail: vi.fn(() => detailPromise)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, sseAbortRef, state } = buildHarness()
    state.activeThreadId = 'thr_other'
    state.blocks = [{ kind: 'user', id: 'old_user', text: 'old' }]
    state.liveAssistant = 'old live'
    state.lastSeq = 9

    const pending = actions.subscribeThreadEventsLive('thr_existing')
    await Promise.resolve()

    expect(provider.subscribeThreadEvents).toHaveBeenCalledWith(
      'thr_existing',
      0,
      expect.any(Object),
      sseAbortRef.current?.signal
    )
    expect(state.blocks).toEqual([])
    expect(state.liveAssistant).toBe('')
    expect(state.lastSeq).toBe(3)
    expect(state.currentTurnId).toBeNull()

    resolveDetail!({
      blocks: [{ kind: 'user', id: 'user_stale', text: 'stale prompt' }],
      latestSeq: 2,
      threadStatus: 'running',
      latestTurnId: 'turn_stale',
      latestUserMessageId: 'user_stale',
      turnDurationByUserId: {}
    })
    await pending

    expect(state.liveAssistant).toBe('')
    expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_stale' })])
    expect(state.busy).toBe(true)
    expect(state.lastSeq).toBe(3)
    expect(state.currentTurnId).toBe('turn_stale')
    expect(getActiveStreamSnapshotFor('thr_existing', 'turn_stale').threadId).toBeNull()
  })

  it('never exposes live assistant text for a known case-boundary thread', async () => {
    let resolveDetail: (value: {
      blocks: Array<{ kind: 'user'; id: string; text: string }>
      latestSeq: number
      threadStatus: string
      latestTurnId: string
      latestUserMessageId: string
      turnDurationByUserId: Record<string, number>
      historyAuthority: 'case_boundary_only_v1'
    }) => void
    const detailPromise = new Promise<Parameters<typeof resolveDetail>[0]>((resolve) => {
      resolveDetail = resolve
    })
    const provider = {
      subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: {
        onDeltas(deltas: Array<{ kind: 'agent_message'; text: string; seq: number }>): void
      }) => {
        sink.onDeltas([{
          kind: 'agent_message',
          text: 'RESTRICTED_LIVE_FACT_SENTINEL',
          seq: 3
        }])
        return new Promise<void>(() => undefined)
      }),
      getThreadDetail: vi.fn(() => detailPromise)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_other'
    state.threads = [{
      ...thread('thr_existing'),
      historyAuthority: 'case_boundary_only_v1'
    }]

    const pending = actions.subscribeThreadEventsLive('thr_existing')
    await Promise.resolve()

    expect(state.liveAssistant).toBe('')
    expect(JSON.stringify(state.blocks)).not.toContain('RESTRICTED_LIVE_FACT_SENTINEL')

    resolveDetail!({
      blocks: [{ kind: 'user', id: 'user_safe', text: 'safe prompt' }],
      latestSeq: 2,
      threadStatus: 'running',
      latestTurnId: 'turn_case',
      latestUserMessageId: 'user_safe',
      turnDurationByUserId: {},
      historyAuthority: 'case_boundary_only_v1'
    })
    await pending

    expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_safe' })])
    expect(state.liveAssistant).toBe('')
    expect(JSON.stringify(state.blocks)).not.toContain('RESTRICTED_LIVE_FACT_SENTINEL')
    expect(getActiveStreamSnapshotFor('thr_existing', 'turn_case')).toMatchObject({
      liveAssistant: '',
      lastSeq: 3
    })
  })

  it('merges persisted history with live blocks when live events arrive before thread detail', async () => {
    let resolveDetail: (value: {
      blocks: Array<{ kind: 'user' | 'assistant'; id: string; text: string }>
      latestSeq: number
      threadStatus: string
      latestTurnId: string
      latestUserMessageId: string
      turnDurationByUserId: Record<string, number>
    }) => void
    const detailPromise = new Promise<Parameters<typeof resolveDetail>[0]>((resolve) => {
      resolveDetail = resolve
    })
    const provider = {
      subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: {
        onSeq(seq: number): void
        onUserMessage(ev: {
          itemId: string
          turnId: string
          text: string
          createdAt: string
        }): void
      }) => {
        sink.onSeq(3)
        sink.onUserMessage({
          itemId: 'user_live',
          turnId: 'turn_live',
          text: 'live prompt',
          createdAt: '2026-07-04T00:00:00.000Z'
        })
        return new Promise<void>(() => undefined)
      }),
      getThreadDetail: vi.fn(() => detailPromise)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_other'

    const pending = actions.subscribeThreadEventsLive('thr_existing')
    await Promise.resolve()

    expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_live' })])

    resolveDetail!({
      blocks: [
        { kind: 'user', id: 'user_history', text: 'history prompt' },
        { kind: 'assistant', id: 'assistant_history', text: 'history answer' }
      ],
      latestSeq: 2,
      threadStatus: 'running',
      latestTurnId: 'turn_history',
      latestUserMessageId: 'user_history',
      turnDurationByUserId: {}
    })
    await pending

    expect(state.blocks).toEqual([
      expect.objectContaining({ id: 'user_history' }),
      expect.objectContaining({ id: 'assistant_history' }),
      expect.objectContaining({ id: 'user_live' })
    ])
    expect(state.lastSeq).toBe(3)
    expect(state.busy).toBe(true)
  })

  it.each(['write', 'claw'] as const)(
    'quarantines live completion until thread detail wins the %s route race',
    async (route) => {
      let resolveDetail: (value: {
        blocks: Array<{ kind: 'user' | 'assistant'; id: string; text: string; meta?: { turnId: string } }>
        latestSeq: number
        threadStatus: string
        latestTurnId: string
        latestUserMessageId: string
        turnDurationByUserId: Record<string, number>
      }) => void
      const detailPromise = new Promise<Parameters<typeof resolveDetail>[0]>((resolve) => {
        resolveDetail = resolve
      })
      const provider = {
        subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: ThreadEventSink) => {
          sink.onSeq(4)
          sink.onUserMessage({
            itemId: 'user_live',
            turnId: 'turn_live',
            text: 'live prompt',
            createdAt: '2026-07-04T00:00:00.000Z'
          })
          sink.onDeltas([{ kind: 'agent_message', text: 'final answer', seq: 4, turnId: 'turn_live' }])
          sink.onTurnComplete()
          return new Promise<void>(() => undefined)
        }),
        getThreadDetail: vi.fn(() => detailPromise)
      }
      registryMock.getProvider.mockReturnValue(provider)
      const { actions, state } = buildHarness()
      state.activeThreadId = 'thr_other'
      state.route = route

      const pending = actions.subscribeThreadEventsLive('thr_existing')
      await Promise.resolve()

      expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_live' })])
      expect(state.blocks.some((block) => block.kind === 'assistant')).toBe(false)
      expect(state.liveAssistant).toBe('')

      resolveDetail!({
        blocks: [
          { kind: 'user', id: 'user_live', text: 'live prompt', meta: { turnId: 'turn_live' } },
          { kind: 'assistant', id: 'assistant_detail', text: 'final answer', meta: { turnId: 'turn_live' } }
        ],
        latestSeq: 2,
        threadStatus: 'idle',
        latestTurnId: 'turn_live',
        latestUserMessageId: 'user_live',
        turnDurationByUserId: {}
      })
      await pending

      const assistants = state.blocks.filter((block) => block.kind === 'assistant')
      expect(assistants).toHaveLength(1)
      expect(assistants[0]).toMatchObject({ text: 'final answer' })
    }
  )

  it('keeps identical final-answer text when it belongs to a different turn', async () => {
    let resolveDetail: (value: {
      blocks: Array<{ kind: 'user' | 'assistant'; id: string; text: string; meta?: { turnId: string } }>
      latestSeq: number
      threadStatus: string
      latestTurnId: string
      latestUserMessageId: string
      turnDurationByUserId: Record<string, number>
    }) => void
    const detailPromise = new Promise<Parameters<typeof resolveDetail>[0]>((resolve) => {
      resolveDetail = resolve
    })
    const provider = {
      subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: ThreadEventSink) => {
        sink.onSeq(4)
        sink.onUserMessage({
          itemId: 'user_live',
          turnId: 'turn_live',
          text: 'again',
          createdAt: '2026-07-04T00:00:00.000Z'
        })
        sink.onDeltas([{ kind: 'agent_message', text: '好的', seq: 4, turnId: 'turn_live' }])
        sink.onTurnComplete()
        return new Promise<void>(() => undefined)
      }),
      getThreadDetail: vi.fn(() => detailPromise)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_other'

    const pending = actions.subscribeThreadEventsLive('thr_existing')
    await Promise.resolve()

    resolveDetail!({
      blocks: [
        { kind: 'user', id: 'user_old', text: 'old prompt' },
        { kind: 'assistant', id: 'assistant_old', text: '好的' },
        { kind: 'user', id: 'user_live', text: 'again' },
        { kind: 'assistant', id: 'assistant_detail', text: '好的' }
      ],
      latestSeq: 2,
      threadStatus: 'idle',
      latestTurnId: 'turn_live',
      latestUserMessageId: 'user_live',
      turnDurationByUserId: {}
    })
    await pending

    const assistants = state.blocks.filter((block) => block.kind === 'assistant')
    expect(assistants).toHaveLength(2)
    expect(assistants.map((block) => block.text)).toEqual(['好的', '好的'])
    expect(state.blocks.map((block) => block.id)).toContain('user_live')
  })

  it('restores missing registered-thread metadata together with its history', async () => {
    const restored = { ...thread('thr_restored'), title: 'Restored SDD', workspace: '/Users/synthetic/plan-workspace', historyAuthority: 'case_boundary_only_v1' as const }
    const blocks = [{ kind: 'user' as const, id: 'restored-user', text: 'Synthetic request' }]
    const provider = {
      getThreadDetail: vi.fn(async () => ({ thread: restored, blocks, latestSeq: 7, threadStatus: 'idle' })),
      subscribeThreadEvents: vi.fn(() => new Promise<void>(() => undefined))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    const other = thread('thr_other')
    state.threads = [other]
    state.composerPickList = []
    state.composerModelGroups = []
    await actions.selectThread(restored.id)
    expect(state.error).toBeNull()
    expect(state.activeThreadId).toBe(restored.id)
    expect(state.threads).toEqual([other, restored])
    expect(state.blocks).toMatchObject(blocks)
    expect(state.lastSeq).toBe(7)
  })

  it('refuses mismatched detail metadata without adopting its history', async () => {
    registryMock.getProvider.mockReturnValue({ getThreadDetail: vi.fn(async () => ({
      thread: thread('wrong-thread'), blocks: [{ kind: 'user', id: 'wrong-user', text: 'Wrong' }], latestSeq: 7
    })) })
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_other'
    state.threads = [thread('thr_other')]
    await actions.selectThread('requested-thread')
    expect(state.activeThreadId).toBe('thr_other')
    expect(state.threads.map((entry) => entry.id)).toEqual(['thr_other'])
    expect(state.blocks.some((block) => block.id === 'wrong-user')).toBe(false)
  })

  it('clears stale active stream when selecting a completed thread detail', async () => {
    resetActiveStream('thr_existing', {
      turnId: 'turn-old',
      liveAssistant: 'stale answer',
      lastSeq: 1
    })
    const provider = {
      getThreadDetail: vi.fn(async () => ({
        blocks: [
          { kind: 'user' as const, id: 'user-old', text: 'prompt', meta: { turnId: 'turn-old' } },
          { kind: 'assistant' as const, id: 'assistant-old', text: 'final', meta: { turnId: 'turn-old' } }
        ],
        latestSeq: 5,
        threadStatus: 'idle',
        latestTurnId: 'turn-old',
        latestUserMessageId: 'user-old',
        turnDurationByUserId: {}
      })),
      subscribeThreadEvents: vi.fn(() => new Promise<void>(() => undefined))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_other'
    state.threads = [thread('thr_existing'), thread('thr_other')]

    await actions.selectThread('thr_existing')

    expect(getActiveStreamSnapshotFor('thr_existing', 'turn-old').liveAssistant).toBe('')
  })

  it('keeps SSE streaming when getThreadDetail fails', async () => {
    const provider = {
      subscribeThreadEvents: vi.fn((_threadId: string, _sinceSeq: number, sink: {
        onDeltas(deltas: Array<{ kind: 'agent_message'; text: string; seq: number }>): void
      }) => {
        sink.onDeltas([{ kind: 'agent_message', text: 'still streaming', seq: 1 }])
        return new Promise<void>(() => undefined)
      }),
      getThreadDetail: vi.fn(async () => {
        throw new Error('detail timeout')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_other'

    await actions.subscribeThreadEventsLive('thr_existing')

    expect(provider.subscribeThreadEvents).toHaveBeenCalledTimes(1)
    expect(state.liveAssistant).toBe('')
    expect(state.busy).toBe(true)
    expect(state.error).toBe(formatRuntimeError(new Error('detail timeout')))
  })

  it('does not clear same-thread blocks or live buffers while resubscribing', async () => {
    let resolveDetail: (value: {
      blocks: Array<{ kind: 'user'; id: string; text: string }>
      latestSeq: number
      threadStatus: string
      latestTurnId: string
      latestUserMessageId: string
      turnDurationByUserId: Record<string, number>
    }) => void
    const provider = {
      subscribeThreadEvents: vi.fn(() => new Promise<void>(() => undefined)),
      getThreadDetail: vi.fn(() => new Promise<Parameters<typeof resolveDetail>[0]>((resolve) => {
        resolveDetail = resolve
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.blocks = [{ kind: 'user', id: 'user_current', text: 'current prompt' }]
    state.liveAssistant = 'partial answer'
    state.lastSeq = 5

    const pending = actions.subscribeThreadEventsLive('thr_existing')
    await Promise.resolve()

    expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_current' })])
    expect(state.liveAssistant).toBe('partial answer')
    expect('liveReasoning' in state).toBe(false)

    resolveDetail!({
      blocks: [{ kind: 'user', id: 'user_stale', text: 'stale prompt' }],
      latestSeq: 4,
      threadStatus: 'running',
      latestTurnId: 'turn_stale',
      latestUserMessageId: 'user_stale',
      turnDurationByUserId: {}
    })
    await pending

    expect(state.blocks).toEqual([expect.objectContaining({ id: 'user_current' })])
    expect(state.liveAssistant).toBe('partial answer')
    expect('liveReasoning' in state).toBe(false)
  })

  it('opens SSE from zero and lets a newer thread detail absorb replayed idle deltas', async () => {
    const provider = {
      subscribeThreadEvents: vi.fn(async (_threadId: string, _sinceSeq: number, sink: {
        onDeltas(deltas: Array<{ kind: 'agent_message'; text: string; seq: number }>): void
      }) => {
        sink.onDeltas([{ kind: 'agent_message', text: 'old replay', seq: 4 }])
      }),
      getThreadDetail: vi.fn(async () => ({
        blocks: [
          { kind: 'user', id: 'user_done', text: 'done prompt' },
          { kind: 'assistant', id: 'assistant_done', text: 'done answer' }
        ],
        latestSeq: 5,
        threadStatus: 'idle',
        latestTurnId: 'turn_done',
        latestUserMessageId: 'user_done',
        turnDurationByUserId: {}
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, sseAbortRef, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.blocks = []
    state.lastSeq = 0
    state.busy = false

    await actions.subscribeThreadEventsLive('thr_existing')

    expect(provider.getThreadDetail).toHaveBeenCalledTimes(1)
    expect(provider.subscribeThreadEvents).toHaveBeenCalledWith(
      'thr_existing',
      0,
      expect.any(Object),
      sseAbortRef.current?.signal
    )
    expect(state.lastSeq).toBe(5)
    expect(state.liveAssistant).toBe('')
    expect(state.busy).toBe(false)
    expect(state.blocks).toEqual([
      expect.objectContaining({ id: 'user_done', text: 'done prompt' }),
      expect.objectContaining({ id: 'assistant_done', text: 'done answer' })
    ])
  })

  it('does not let recoverActiveTurn overwrite a newer live terminal error', async () => {
    const provider = {
      subscribeThreadEvents: vi.fn(async () => undefined),
      getThreadDetail: vi.fn(async () => {
        state.lastSeq = 7
        state.busy = false
        state.error = 'terminal runtime error'
        state.blocks = [
          { kind: 'user', id: 'user_live', text: 'live prompt' },
          {
            kind: 'system',
            id: 'error_live',
            text: 'provider failed',
            severity: 'error',
            code: 'http_400'
          }
        ]
        state.currentTurnId = null
        state.currentTurnUserId = null
        return {
          blocks: [
            { kind: 'user', id: 'user_stale', text: 'stale prompt' },
            {
              kind: 'tool',
              id: 'tool_stale',
              summary: 'stale running tool',
              status: 'running',
              toolKind: 'command_execution'
            }
          ],
          latestSeq: 6,
          threadStatus: 'running',
          latestTurnId: 'turn_stale',
          latestUserMessageId: 'user_stale',
          turnDurationByUserId: {}
        }
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, sseAbortRef, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.blocks = [{ kind: 'user', id: 'user_before', text: 'before' }]
    state.busy = true
    state.currentTurnId = 'turn_before'
    state.currentTurnUserId = 'user_before'
    state.lastSeq = 5

    await expect(actions.recoverActiveTurn()).resolves.toBe(false)

    expect(state.lastSeq).toBe(7)
    expect(state.busy).toBe(false)
    expect(state.error).toBe('terminal runtime error')
    expect(state.currentTurnId).toBeNull()
    expect(state.currentTurnUserId).toBeNull()
    expect(state.blocks).toEqual([
      expect.objectContaining({ id: 'user_live', text: 'live prompt' }),
      expect.objectContaining({ id: 'error_live', kind: 'system', code: 'http_400' })
    ])
    expect(state.blocks).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'tool_stale' })
    ]))
    expect(provider.subscribeThreadEvents).toHaveBeenCalledWith(
      'thr_existing',
      7,
      expect.any(Object),
      expect.any(AbortSignal)
    )
  })

  it('blocks case recovery when idle GET has no accepted-final authority', async () => {
    const provider = {
      subscribeThreadEvents: vi.fn(async () => undefined),
      getThreadDetail: vi.fn(async () => ({
        blocks: [
          { kind: 'user', id: 'user_case', text: 'case prompt', meta: { turnId: 'turn_case' } }
        ],
        latestSeq: 8,
        threadStatus: 'idle',
        latestTurnId: 'turn_case',
        latestUserMessageId: 'user_case',
        turnDurationByUserId: {},
        historyAuthority: 'case_boundary_only_v1' as const
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.threads = [{ ...thread('thr_existing'), historyAuthority: 'case_boundary_only_v1' }]
    state.blocks = [
      { kind: 'user', id: 'user_case', text: 'case prompt', meta: { turnId: 'turn_case' } },
      { kind: 'assistant', id: 'assistant_draft', text: 'fabricated case fact', meta: { turnId: 'turn_case' } }
    ]
    state.liveAssistant = 'fabricated tail'
    state.busy = true
    state.currentTurnId = 'turn_case'
    state.currentTurnUserId = 'user_case'
    state.lastSeq = 7
    state.queuedMessages = [{ id: 'queued_case', text: 'next request' }]

    await expect(actions.recoverActiveTurn()).resolves.toBe(false)

    expect(provider.subscribeThreadEvents).not.toHaveBeenCalled()
    expect(state.blocks.filter((block) => block.kind === 'assistant')).toEqual([])
    expect(state.liveAssistant).toBe('')
    expect(state.busy).toBe(false)
    expect(state.currentTurnId).toBe('turn_case')
    expect(state.currentTurnUserId).toBe('user_case')
    expect(state.error).toBe(i18n.t('common:runtimeFinalSnapshotUnavailable'))
    expect(state.queuedMessagesPausedReason).toBe('failed')
    expect(state.drainQueuedMessages).not.toHaveBeenCalled()
  })

  it('forwards structured file references through the send path', async () => {
    const provider = {
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.busy = false

    await expect(actions.sendMessage('inspect files', 'agent', {
      fileReferences: [{
        path: '/workspace/analytix/src/App.tsx',
        relativePath: 'src/App.tsx',
        name: 'App.tsx',
        kind: 'file'
      }]
    })).resolves.toBe(true)

    expect(state.blocks[0]).toMatchObject({
      kind: 'user',
      meta: {
        fileReferences: [{ relativePath: 'src/App.tsx', kind: 'file' }]
      }
    })
    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_existing',
      'inspect files',
      expect.objectContaining({
        fileReferences: expect.arrayContaining([
          expect.objectContaining({ relativePath: 'src/App.tsx', kind: 'file' })
        ])
      })
    )
  })

  it('creates a git checkpoint before sending and forwards its id to the runtime turn', async () => {
    const provider = {
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const createGitCheckpoint = vi.fn(async () => ({
      ok: true,
      checkpointId: 'gcp_1',
      repositoryRoot: '/workspace/analytix',
      head: 'abc',
      currentBranch: 'main'
    }))
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/workspace/analytix',
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        workspace: {
          createGitCheckpoint
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.busy = false

    await expect(actions.sendMessage('change files', 'agent')).resolves.toBe(true)

    expect(createGitCheckpoint).toHaveBeenCalledWith({
      workspaceRoot: '/workspace/analytix',
      threadId: 'thr_existing',
      timeoutMs: 650
    })
    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_existing',
      'change files',
      expect.objectContaining({ workspaceCheckpointId: 'gcp_1' })
    )
    expect(state.blocks[0]).toMatchObject({
      kind: 'user',
      id: 'user_1',
      meta: { workspaceCheckpointId: 'gcp_1' }
    })
  })

  it('continues sending without a checkpoint id when git checkpoint exceeds the send soft timeout', async () => {
    vi.useFakeTimers()
    const provider = {
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    type CheckpointOkResult = {
      ok: true
      checkpointId: string
      repositoryRoot: string
      head: string
      currentBranch: string
    }
    let resolveCheckpoint: ((value: CheckpointOkResult) => void) | undefined
    const createGitCheckpoint = vi.fn(() => new Promise<CheckpointOkResult>((resolve) => {
      resolveCheckpoint = resolve
    }))
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/workspace/analytix',
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        workspace: {
          createGitCheckpoint
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.busy = false

    const sendPromise = actions.sendMessage('change files', 'agent')
    await vi.advanceTimersByTimeAsync(700)

    await expect(sendPromise).resolves.toBe(true)

    expect(createGitCheckpoint).toHaveBeenCalledTimes(1)
    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_existing',
      'change files',
      expect.not.objectContaining({ workspaceCheckpointId: expect.any(String) })
    )
    if (resolveCheckpoint) resolveCheckpoint({
      ok: true,
      checkpointId: 'gcp_late',
      repositoryRoot: '/workspace/analytix',
      head: 'abc',
      currentBranch: 'main'
    })
  })

  it('caches git_unavailable checkpoint results per workspace', async () => {
    const provider = {
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const createGitCheckpoint = vi.fn(async () => ({
      ok: false as const,
      reason: 'git_unavailable' as const,
      message: 'git not found'
    }))
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/workspace/analytix',
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        workspace: {
          createGitCheckpoint
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })

    const first = buildHarness()
    first.state.busy = false
    await expect(first.actions.sendMessage('first turn', 'agent')).resolves.toBe(true)
    const second = buildHarness()
    second.state.busy = false
    await expect(second.actions.sendMessage('second turn', 'agent')).resolves.toBe(true)

    expect(createGitCheckpoint).toHaveBeenCalledTimes(1)
    expect(provider.sendUserMessage).toHaveBeenCalledTimes(2)
    const sendCalls = provider.sendUserMessage.mock.calls as unknown as Array<[string, string, Record<string, unknown>]>
    expect(sendCalls[1]?.[2]).not.toHaveProperty('workspaceCheckpointId')
  })

  it('creates worktree-backed threads through the analytix workspace bridge', async () => {
    const provider = {
      createThread: vi.fn(async (input: { workspace: string; title: string; mode: string }) => ({
        id: 'thr_worktree',
        title: input.title,
        updatedAt: '2026-06-09T00:00:00.000Z',
        model: 'deepseek-v4-pro',
        mode: input.mode,
        workspace: input.workspace,
        status: 'idle'
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const findAvailableWorktreePoolIndex = vi.fn(async () => 1)
    const acquireWorktree = vi.fn(async () => ({
      path: '/workspace/analytix-worktrees/pool-1',
      branch: 'analytix-pool-1'
    }))
    const localStorage = memoryStorage()
    const getSettings = vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({
      workspaceRoot: '/workspace/analytix'
    } as never)
    vi.stubGlobal('window', {
      localStorage,
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/workspace/analytix',
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        workspace: {
          findAvailableWorktreePoolIndex,
          acquireWorktree
        }
      }
    })
    const { actions, state } = buildHarness()
    state.selectThread = vi.fn(async () => undefined) as never

    await actions.createThread({
      workspaceRoot: '/workspace/analytix',
      useWorktreePool: true
    })

    expect(findAvailableWorktreePoolIndex).toHaveBeenCalledWith({
      projectPath: '/workspace/analytix'
    })
    expect(acquireWorktree).toHaveBeenCalledWith(expect.objectContaining({
      projectPath: '/workspace/analytix',
      poolIndex: 1,
      force: false
    }))
    expect(provider.createThread).toHaveBeenCalledWith(expect.objectContaining({
      workspace: '/workspace/analytix-worktrees/pool-1'
    }))
    expect(state.selectThread).toHaveBeenCalledWith('thr_worktree')
    expect(readThreadWorktreeRegistry(localStorage).worktrees.thr_worktree).toMatchObject({
      projectPath: '/workspace/analytix',
      poolIndex: 1,
      worktreePath: '/workspace/analytix-worktrees/pool-1',
      branch: 'analytix-pool-1'
    })
    getSettings.mockRestore()
  })

  it('starts a clean Code draft without creating a runtime thread', async () => {
    const provider = {
      createThread: vi.fn()
    }
    registryMock.getProvider.mockReturnValue(provider)
    const getSettings = vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({
      workspaceRoot: '/workspace/analytix'
    } as never)
    const { actions, state } = buildHarness()
    state.activeThreadId = 'thr_existing'
    state.busy = false
    state.workspaceRoot = '/workspace/analytix'
    state.workspaceLabel = 'analytix'

    await actions.createThread()

    expect(provider.createThread).not.toHaveBeenCalled()
    expect(state.activeThreadId).toBeNull()
    expect(state.blocks).toEqual([])
    expect(state.workspaceRoot).toBe('/workspace/analytix')
    getSettings.mockRestore()
  })

  it('opens a clean new session while keeping a busy previous thread watched', async () => {
    const provider = {
      createThread: vi.fn()
    }
    registryMock.getProvider.mockReturnValue(provider)
    const getSettings = vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({
      workspaceRoot: '/workspace/analytix'
    } as never)
    const { actions, sseAbortRef, state } = buildHarness()
    const abort = vi.fn()
    sseAbortRef.current = { abort } as unknown as AbortController
    Object.assign(state, {
      activeThreadId: 'thr_existing',
      activeThreadGoal: { threadId: 'thr_existing', objective: 'old goal', status: 'active' },
      activeThreadTodos: { threadId: 'thr_existing', items: [{ id: 'todo-1', text: 'old todo', status: 'pending' }] },
      blocks: [{ kind: 'assistant', id: 'assistant-1', text: 'partial old answer' }],
      busy: true,
      currentTurnId: 'turn-1',
      currentTurnUserId: 'user-1',
      liveAssistant: 'partial',
      lastSeq: 19,
      queuedMessages: [{ id: 'queued-1', text: 'follow-up' }],
      watchTurnCompletion: { thr_other: true },
      workspaceRoot: '/workspace/old'
    })

    await actions.createThread({ workspaceRoot: '/workspace/analytix' })

    expect(provider.createThread).not.toHaveBeenCalled()
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
    expect(state.watchTurnCompletion).toEqual({ thr_other: true, thr_existing: true })
    expect(state.workspaceRoot).toBe('/workspace/analytix')
    expect(state.route).toBe('chat')
    expect(state.codeWorkspaceRoots).toContain('/workspace/analytix')
    getSettings.mockRestore()
  })

  it('creates the first sent Code thread with runtime auto-title enabled', async () => {
    const provider = {
      createThread: vi.fn(async (input: { workspace: string; autoTitle?: boolean; title?: string; mode: string }) => ({
        id: 'thr_auto_title',
        title: '新会话',
        updatedAt: '2026-06-09T00:00:00.000Z',
        model: 'deepseek-v4-pro',
        mode: input.mode,
        workspace: input.workspace,
        status: 'idle'
      })),
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_auto_title',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/workspace/analytix',
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          }))
        },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.activeThreadId = null
    state.busy = false
    state.blocks = []
    state.threads = []
    state.workspaceRoot = '/workspace/analytix'

    await expect(actions.sendMessage('请分析这个 bug', 'agent')).resolves.toBe(true)

    expect(provider.createThread).toHaveBeenCalledWith(expect.objectContaining({
      workspace: '/workspace/analytix',
      autoTitle: true,
      mode: 'agent'
    }))
    expect(provider.createThread.mock.calls[0]?.[0]).not.toHaveProperty('title')
    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_auto_title',
      '请分析这个 bug',
      expect.objectContaining({ displayText: '请分析这个 bug' })
    )
  })

  it('forwards an override provider from the write route without restarting', async () => {
    const provider = {
      connect: vi.fn(async () => undefined),
      sendUserMessage: vi.fn(async () => ({
        threadId: 'thr_existing',
        turnId: 'turn_1',
        userMessageItemId: 'user_1'
      })),
      subscribeThreadEvents: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const saveSettingsSilent = vi.fn(async () => ({
      runtime: { providerId: 'minimax-token-plan', model: 'MiniMax-M3' },
      codePromptPrefix: ''
    }))
    const restartRuntime = vi.fn(async () => undefined)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            runtime: { providerId: 'deepseek', model: 'deepseek-v4-pro' },
            codePromptPrefix: ''
          })),
          saveSettingsSilent
        },
        runtime: { restartRuntime },
        logs: { error: vi.fn(async () => undefined) }
      }
    })
    const { actions, state } = buildHarness()
    state.route = 'write'
    state.busy = false

    await expect(actions.sendMessage('make a prototype', 'agent', {
      model: 'MiniMax-M3',
      providerId: 'minimax-token-plan'
    })).resolves.toBe(true)

    expect(saveSettingsSilent).not.toHaveBeenCalled()
    expect(restartRuntime).not.toHaveBeenCalled()
    expect(provider.connect).not.toHaveBeenCalled()
    expect(provider.sendUserMessage).toHaveBeenCalledWith(
      'thr_existing',
      'make a prototype',
      expect.objectContaining({ model: 'MiniMax-M3', providerId: 'minimax-token-plan' })
    )
  })
})

describe('send receipt acknowledgement and housekeeping', () => {
  beforeEach(() => {
    rendererRuntimeClient.invalidateSettings()
    registryMock.getProvider.mockReset()
  })
  afterEach(() => {
    clearActiveStream()
    rendererRuntimeClient.invalidateSettings()
    vi.unstubAllGlobals()
  })
  it.each(['before-ack', 'after-ack'])('reports the original send result when an error occurs %s', async phase => {
    const provider = {
      sendUserMessage: vi.fn(async () => {
        if (phase === 'before-ack') throw new Error('Synthetic send rejected')
        return { threadId: 'thr_existing', turnId: 'confirmed-turn', userMessageItemId: 'confirmed-user' }
      }),
      subscribeThreadEvents: vi.fn(() => new Promise<void>(() => undefined))
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', { analytix: {
      settings: { getSettings: vi.fn(async () => ({ runtime: { providerId: 'synthetic', model: 'synthetic' }, codePromptPrefix: '' })) },
      logs: { error: vi.fn(async () => undefined) }
    } })
    const { actions, state, sseAbortRef } = buildHarness()
    state.busy = false
    if (phase === 'after-ack') state.refreshThreads = vi.fn(async () => { throw new Error('Synthetic housekeeping failed') })
    expect(await actions.sendMessage('Original request', 'agent')).toBe(phase === 'after-ack')
    expect(provider.sendUserMessage).toHaveBeenCalledOnce()
    if (phase === 'after-ack') {
      expect(state).toMatchObject({ busy: true, currentTurnId: 'confirmed-turn', currentTurnUserId: 'confirmed-user', error: null })
    } else expect(state).toMatchObject({ busy: false, currentTurnId: null, currentTurnUserId: null })
    sseAbortRef.current?.abort()
  })
})
