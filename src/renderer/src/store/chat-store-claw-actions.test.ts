import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ClawImChannelV1 } from '@shared/app-settings'
import { CLAW_MANAGED_INSTRUCTIONS_HEADING } from '@shared/app-settings'
import type { NormalizedThread } from '../agent/types'
import { rendererRuntimeClient } from '../agent/runtime-client'
import {
  channelWithClawThreadMapping,
  clawThreadIdForProvider,
  createClawActions,
  findRecoverableClawThread,
  resolveClawThreadId
} from './chat-store-claw-actions'

function channel(overrides: Partial<ClawImChannelV1> = {}): ClawImChannelV1 {
  const now = '2026-06-01T00:00:00.000Z'
  return {
    id: 'channel-1',
    provider: 'feishu',
    label: 'Feishu Agent01',
    enabled: true,
    model: 'auto',
    threadId: 'thr-codewhale-channel',
    workspaceRoot: '/Users/zxy/.analytix/claw/agent01',
    agentProfile: {
      name: '',
      description: '',
      identity: '',
      personality: '',
      userContext: '',
      replyRules: ''
    },
    conversations: [
      {
        id: 'conversation-1',
        chatId: 'chat-1',
        remoteThreadId: '',
        latestMessageId: 'message-1',
        senderId: 'sender-1',
        senderName: 'Alex',
        localThreadId: 'thr-codewhale-conversation',
        workspaceRoot: '/Users/zxy/.analytix/claw/agent01/conversations/chat-1',
        createdAt: now,
        updatedAt: now
      }
    ],
    createdAt: now,
    updatedAt: now,
    ...overrides
  }
}

function thread(id: string, title: string, updatedAt = '2026-06-01T00:00:00.000Z'): NormalizedThread {
  return {
    id,
    title,
    updatedAt,
    model: 'reasonix',
    mode: 'agent',
    workspace: '/Users/zxy/.analytix/default_workspace'
  }
}

describe('chat-store Claw actions helpers', () => {
  afterEach(() => {
    rendererRuntimeClient.invalidateSettings()
    vi.unstubAllGlobals()
  })

  it('uses the channel threadId when the latest conversation has none', () => {
    const item = channel({ threadId: 'analytix-channel-thread' })
    const conversation = { ...item.conversations[0], localThreadId: '' }
    expect(clawThreadIdForProvider(item, conversation)).toBe('analytix-channel-thread')
  })

  it('recovers an unmapped Claw managed Analytix session before creating a new empty one', () => {
    const item = channel()
    const recovered = findRecoverableClawThread(
      [
        thread('empty-claw-thread', '[Claw:Feishu Agent01]', '2026-06-01T00:02:00.000Z'),
        thread('old-content-thread', `${CLAW_MANAGED_INSTRUCTIONS_HEADING} legacy scheduled-task tools`, '2026-06-01T00:01:00.000Z')
      ],
      [item],
      item
    )

    expect(recovered?.id).toBe('old-content-thread')
  })

  it('writes recovered provider thread ids back to both channel and conversation', () => {
    const now = '2026-06-01T00:03:00.000Z'
    const next = channelWithClawThreadMapping(channel(), 'analytix-thread', now, 'conversation-1')

    expect(next.threadId).toBe('analytix-thread')
    expect(next.conversations[0]?.localThreadId).toBe('analytix-thread')
  })

  it('drops stale configured thread ids and falls back to a recovered thread', () => {
    expect(
      resolveClawThreadId({
        configuredThreadId: 'thr_missing',
        recoveredThreadId: 'thr_recovered',
        configuredThreadExists: false,
        configuredThreadHasUserMessages: false
      })
    ).toBe('thr_recovered')
  })

  it('keeps the configured thread when it exists and already has conversation history', () => {
    expect(
      resolveClawThreadId({
        configuredThreadId: 'thr_live',
        recoveredThreadId: 'thr_recovered',
        configuredThreadExists: true,
        configuredThreadHasUserMessages: true
      })
    ).toBe('thr_live')
  })

  it('keeps an empty IM channel on the Claw route instead of selecting a stale missing thread', async () => {
    rendererRuntimeClient.invalidateSettings()
    let settings = {
      workspaceRoot: '/Users/zxy/project',
      claw: {
        enabled: true,
        im: {
          enabled: true,
          provider: 'feishu',
          workspaceRoot: '/Users/zxy/project'
        },
        channels: [channel({ threadId: 'thr_missing', conversations: [] })]
      }
    }
    const settingsApi = {
      getSettings: vi.fn(async () => settings),
      setSettings: vi.fn(async (patch: { claw?: { channels?: ClawImChannelV1[] } }) => {
        settings = {
          ...settings,
          claw: {
            ...settings.claw,
            ...(patch.claw ?? {}),
            channels: patch.claw?.channels ?? settings.claw.channels
          }
        }
        return settings
      })
    }
    const analytix = { settings: settingsApi }
    vi.stubGlobal('window', { analytix })

    const provider = {
      createThread: vi.fn(),
      getThreadDetail: vi.fn(async () => {
        throw new Error('thread not found: thr_missing')
      }),
      deleteThread: vi.fn()
    }
    let state: Record<string, unknown> = {
      runtimeConnection: 'ready',
      route: 'chat',
      clawChannels: settings.claw.channels,
      activeClawChannelId: '',
      threads: [],
      activeThreadId: 'thr_previous',
      blocks: [{ kind: 'user', id: 'u1', text: 'hello' }],
      liveAssistant: '',
      busy: false,
      lastSeq: 0,
      currentTurnId: null,
      currentTurnUserId: null,
      inspectorSelectedId: null,
      composerModel: 'auto',
      error: 'previous error'
    }
    const set = vi.fn((partial: Record<string, unknown> | ((current: typeof state) => Record<string, unknown>)) => {
      const patch = typeof partial === 'function' ? partial(state) : partial
      state = { ...state, ...patch }
    })
    const actions = createClawActions({
      set: set as never,
      get: (() => state) as never,
      i18n: { t: (key: string) => key },
      getProvider: () => provider,
      newClawChannel: vi.fn() as never,
      normalizeClawComposerModel: (raw: string) => raw as never,
      activeClawChannel: vi.fn() as never,
      normalizeWorkspaceRoot: (workspaceRoot?: string | null) => workspaceRoot?.trim() ?? '',
      formatRuntimeError: (error: unknown) => error instanceof Error ? error.message : String(error),
      shouldOpenSettingsForError: () => false,
      clearedThreadSelection: () => ({
        activeThreadId: null,
        blocks: [],
        liveAssistant: '',
        busy: false,
        lastSeq: 0,
        currentTurnId: null,
        currentTurnUserId: null,
        inspectorSelectedId: null
      }),
      sseAbortRef: { current: null },
      clearBusyWatchdog: vi.fn()
    })

    await actions.selectClawChannel('channel-1')

    expect(provider.createThread).not.toHaveBeenCalled()
    expect(state.route).toBe('claw')
    expect(state.activeClawChannelId).toBe('channel-1')
    expect(state.activeThreadId).toBeNull()
    expect(state.error).toBeNull()
    expect(settingsApi.setSettings).toHaveBeenCalledWith({
      claw: {
        channels: [expect.objectContaining({ id: 'channel-1', threadId: '' })]
      }
    })
  })

  it('uses Connect Phone copy for generated mapped conversation placeholders', async () => {
    rendererRuntimeClient.invalidateSettings()
    let settings = {
      workspaceRoot: '/Users/zxy/project',
      claw: {
        enabled: true,
        im: {
          enabled: true,
          provider: 'feishu',
          workspaceRoot: '/Users/zxy/project'
        },
        channels: [channel()]
      }
    }
    const settingsApi = {
      getSettings: vi.fn(async () => settings),
      setSettings: vi.fn(async (patch: { claw?: { channels?: ClawImChannelV1[] } }) => {
        settings = {
          ...settings,
          claw: {
            ...settings.claw,
            ...(patch.claw ?? {}),
            channels: patch.claw?.channels ?? settings.claw.channels
          }
        }
        return settings
      })
    }
    const analytix = { settings: settingsApi }
    vi.stubGlobal('window', { analytix })

    const provider = {
      createThread: vi.fn(),
      getThreadDetail: vi.fn(async () => ({ blocks: [] })),
      deleteThread: vi.fn()
    }
    let state: Record<string, unknown>
    const selectThread = vi.fn(async (threadId: string) => {
      state.activeThreadId = threadId
    })
    state = {
      runtimeConnection: 'ready',
      route: 'chat',
      clawChannels: settings.claw.channels,
      activeClawChannelId: '',
      threads: [],
      selectThread,
      activeThreadId: null,
      blocks: [],
      liveAssistant: '',
      busy: false,
      lastSeq: 0,
      currentTurnId: null,
      currentTurnUserId: null,
      inspectorSelectedId: null,
      composerModel: 'auto',
      error: null
    }
    const set = vi.fn((partial: Record<string, unknown> | ((current: typeof state) => Record<string, unknown>)) => {
      const patch = typeof partial === 'function' ? partial(state) : partial
      state = { ...state, ...patch }
    })
    const actions = createClawActions({
      set: set as never,
      get: (() => state) as never,
      i18n: { t: (key: string) => key },
      getProvider: () => provider,
      newClawChannel: vi.fn() as never,
      normalizeClawComposerModel: (raw: string) => raw as never,
      activeClawChannel: vi.fn() as never,
      normalizeWorkspaceRoot: (workspaceRoot?: string | null) => workspaceRoot?.trim() ?? '',
      formatRuntimeError: (error: unknown) => error instanceof Error ? error.message : String(error),
      shouldOpenSettingsForError: () => false,
      clearedThreadSelection: () => ({
        activeThreadId: null,
        blocks: [],
        liveAssistant: '',
        busy: false,
        lastSeq: 0,
        currentTurnId: null,
        currentTurnUserId: null,
        inspectorSelectedId: null
      }),
      sseAbortRef: { current: null },
      clearBusyWatchdog: vi.fn()
    })

    await actions.selectClawConversation('channel-1', 'thr-codewhale-conversation')

    expect(provider.createThread).not.toHaveBeenCalled()
    expect((state.threads as NormalizedThread[])[0]).toMatchObject({
      id: 'thr-codewhale-conversation',
      title: '[Connect Phone:Feishu Agent01]'
    })
    expect((state.threads as NormalizedThread[])[0]?.title).not.toContain('[Claw:')
    expect(selectThread).toHaveBeenCalledWith('thr-codewhale-conversation')
  })
})

describe('image note composition protects Claw thread navigation', () => {
  async function setup(composing: boolean) {
    const { useWriteWorkspaceStore } = await import('../write/write-workspace-store')
    useWriteWorkspaceStore.setState({ imageRegionEditor: {
      workspace: '/synthetic', path: '/synthetic/image.png', threadId: 'image-owner', snapshot: null, annotation: null,
      region: null, note: '组合文本', dirty: true, pending: null, loading: false, revoked: false, stale: false,
      composing, status: 'dirty', error: null
    } })
    let state = { activeThreadId: 'image-owner', route: 'claw', runtimeConnection: 'ready', threads: [], clawChannels: [], activeClawChannelId: '' } as never
    const set = (patch: Record<string, unknown> | ((value: unknown) => Record<string, unknown>)) => {
      state = { ...state as object, ...(typeof patch === 'function' ? patch(state) : patch) } as never
    }
    const getSettings = vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({ claw: { channels: [] } } as never)
    const disconnect = vi.fn()
    vi.stubGlobal('window', { analytix: { connectPhone: { disconnectImChannel: disconnect } } })
    const provider = { createThread: vi.fn(), getThreadDetail: vi.fn(), deleteThread: vi.fn() }
    const stream = new AbortController()
    const actions = createClawActions({ set: set as never, get: () => state, i18n: { t: key => key }, getProvider: () => provider,
      newClawChannel: vi.fn(), normalizeClawComposerModel: value => value, activeClawChannel: () => null,
      normalizeWorkspaceRoot: value => value ?? '', formatRuntimeError: String, shouldOpenSettingsForError: () => false,
      clearedThreadSelection: () => ({ activeThreadId: null, blocks: [], liveAssistant: '', busy: false, lastSeq: 0,
        currentTurnId: null, currentTurnUserId: null, inspectorSelectedId: null }),
      sseAbortRef: { current: stream }, clearBusyWatchdog: vi.fn() })
    return { actions, get: () => state as { activeThreadId: string; clawChannels: unknown[] }, useWriteWorkspaceStore, getSettings, disconnect, stream }
  }
  it.each(['select', 'conversation', 'delete', 'reset', 'add'] as const)('rejects %s before thread/navigation side effects during composition', async name => {
    const h = await setup(true)
    try {
      if (name === 'select') await h.actions.selectClawChannel('channel')
      if (name === 'conversation') await h.actions.selectClawConversation('channel', 'thread')
      if (name === 'delete') await h.actions.deleteClawChannel('channel')
      if (name === 'reset') await h.actions.resetClawChannelSession('channel')
      if (name === 'add') await h.actions.addClawChannel('feishu')
      expect(h.get().activeThreadId).toBe('image-owner')
      expect(h.getSettings).not.toHaveBeenCalled(); expect(h.disconnect).not.toHaveBeenCalled()
      expect(h.stream.signal.aborted).toBe(false)
    } finally { h.useWriteWorkspaceStore.setState({ imageRegionEditor: null }); vi.restoreAllMocks(); vi.unstubAllGlobals() }
  })
  it('background channel refresh defers clearing the active image owner', async () => {
    const h = await setup(true)
    try {
      await h.actions.refreshClawChannels()
      expect(h.get().clawChannels).toEqual([])
      expect(h.get().activeThreadId).toBe('image-owner')
      expect(h.stream.signal.aborted).toBe(false)
    } finally { h.useWriteWorkspaceStore.setState({ imageRegionEditor: null }); vi.restoreAllMocks(); vi.unstubAllGlobals() }
  })
  it('composition starting during channel discovery prevents subsequent selection or clearing', async () => {
    const h = await setup(false)
    let resolve!: (value: never) => void
    h.getSettings.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    try {
      const navigation = h.actions.selectClawChannel('channel')
      h.useWriteWorkspaceStore.getState().setImageRegionComposing(true)
      h.useWriteWorkspaceStore.getState().setImageRegionComposing(false)
      resolve({ claw: { channels: [channel({ id: 'channel', threadId: '', conversations: [] })] } } as never)
      await navigation
      expect(h.get().activeThreadId).toBe('image-owner')
      expect(h.stream.signal.aborted).toBe(false)
    } finally { h.useWriteWorkspaceStore.setState({ imageRegionEditor: null }); vi.restoreAllMocks(); vi.unstubAllGlobals() }
  })
})
