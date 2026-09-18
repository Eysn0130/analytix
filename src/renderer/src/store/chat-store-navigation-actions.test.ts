import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NormalizedThread } from '../agent/types'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { createRuntimeStatusPublicV1 } from '@shared/analytix-runtime-status'
import { runtimeErrorToError } from '@shared/runtime-error'
import i18n from '../i18n'
import type { ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'

const registryMock = vi.hoisted(() => ({
  getProvider: vi.fn()
}))

vi.mock('../agent/registry', () => ({
  getProvider: registryMock.getProvider
}))

import { createNavigationActions } from './chat-store-navigation-actions'

function thread(overrides: Partial<NormalizedThread> & Pick<NormalizedThread, 'id' | 'workspace'>): NormalizedThread {
  return {
    id: overrides.id,
    title: overrides.title ?? overrides.id,
    updatedAt: overrides.updatedAt ?? '2026-06-12T00:00:00.000Z',
    model: overrides.model ?? 'deepseek-v4-pro',
    mode: overrides.mode ?? 'agent',
    workspace: overrides.workspace,
    ...(overrides.status ? { status: overrides.status } : {}),
    ...(overrides.archived !== undefined ? { archived: overrides.archived } : {}),
    ...(overrides.relation ? { relation: overrides.relation } : {}),
    ...(overrides.parentThreadId ? { parentThreadId: overrides.parentThreadId } : {})
  }
}

function buildHarness(overrides: Partial<ChatState> = {}): {
  actions: ReturnType<typeof createNavigationActions>
  state: ChatState
  createThread: ReturnType<typeof vi.fn>
  refreshThreads: ReturnType<typeof vi.fn>
  loadComposerModels: ReturnType<typeof vi.fn>
  selectThread: ReturnType<typeof vi.fn>
  setCalls: Partial<ChatState>[]
} {
  const createThread = vi.fn(async () => undefined)
  const refreshThreads = vi.fn(async () => undefined)
  const selectThread = vi.fn(async () => undefined)
  const loadComposerModels = vi.fn(async () => undefined)
  const setCalls: Partial<ChatState>[] = []
  let state = {
    activeThreadId: 'thr_default',
    blocks: [],
    busy: false,
    clawChannels: [],
    codeWorkspaceRoots: ['~/.analytix/default_workspace'],
    createThread,
    currentTurnId: null,
    currentTurnUserId: null,
    error: null,
    loadComposerModels,
    openWrite: vi.fn(async () => undefined),
    refreshThreads,
    route: 'chat',
    runtimeConnection: 'ready',
    selectThread,
    threads: [
      thread({
        id: 'thr_default',
        title: 'Only default thread',
        workspace: '~/.analytix/default_workspace'
      })
    ],
    unreadThreadIds: {},
    watchTurnCompletion: {},
    workspaceLabel: 'default_workspace',
    workspaceRoot: '~/.analytix/default_workspace',
    ...overrides
  } as unknown as ChatState

  const set: ChatStoreSet = (partial) => {
    const update = typeof partial === 'function' ? partial(state) : partial
    setCalls.push(update)
    state = { ...state, ...update }
  }
  const get: ChatStoreGet = () => state
  return {
    actions: createNavigationActions({
      set,
      get,
      sseAbortRef: { current: null }
    }),
    get state() {
      return state
    },
    createThread,
    refreshThreads,
    loadComposerModels,
    selectThread,
    setCalls
  }
}

describe('chat-store navigation workspace selection', () => {
  beforeEach(() => {
    rendererRuntimeClient.invalidateSettings()
    registryMock.getProvider.mockReset()
  })

  afterEach(() => {
    rendererRuntimeClient.invalidateSettings()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('restarts the managed runtime before probing after required first-run provider setup', async () => {
    const calls: string[] = []
    const provider = {
      connect: vi.fn(async () => {
        calls.push('connect')
      })
    }
    const restartRuntime = vi.fn(async () => {
      calls.push('restart')
    })
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '~/.analytix/default_workspace',
            runtime: { providerId: 'deepseek', model: 'deepseek-chat' }
          }))
        },
        runtime: { restartRuntime }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'idle' } as Partial<ChatState>)

    await expect(harness.actions.probeRuntime('user', { restart: true })).resolves.toBeUndefined()

    expect(restartRuntime).toHaveBeenCalledTimes(1)
    expect(provider.connect).toHaveBeenCalledTimes(1)
    expect(calls).toEqual(['restart', 'connect'])
    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.loadComposerModels).toHaveBeenCalledTimes(1)
    expect(harness.refreshThreads).toHaveBeenCalledTimes(1)
  })

  it('keeps projected runtime restart failures safe and actionable in renderer state', async () => {
    const hostileFields = [
      'HOSTILE_SETUP_ERROR',
      '/private/owner/case-42/runtime.json',
      'pii=110101199001011234',
      'model=private-model',
      'pid=424242',
      'timestamp=2026-08-26T12:34:56.789Z',
      'secret=restart-secret-value'
    ]
    const projected = runtimeErrorToError({
      code: 'runtime_unavailable',
      message: hostileFields.join(' ')
    })
    const ipcError = new Error(
      `Error invoking remote method 'runtime:restart': Error: ${projected.message}`
    )
    const restartRuntime = vi.fn(async () => {
      throw ipcError
    })
    const provider = { connect: vi.fn(async () => undefined) }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '~/.analytix/default_workspace',
            runtime: { providerId: 'deepseek', model: 'deepseek-chat' }
          }))
        },
        runtime: { restartRuntime }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'idle' } as Partial<ChatState>)

    await expect(harness.actions.probeRuntime('user', { restart: true })).resolves.toBeUndefined()

    expect(restartRuntime).toHaveBeenCalledTimes(1)
    expect(provider.connect).not.toHaveBeenCalled()
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBe(i18n.t('common:runtimeFetchFailed'))
    expect(harness.state.runtimeErrorDetail).toBe('Code: runtime_unavailable')
    expect(harness.state.route).toBe('settings')
    expect(harness.state.settingsSection).toBe('agents')
    const rendererState = JSON.stringify({
      error: harness.state.error,
      runtimeErrorDetail: harness.state.runtimeErrorDetail,
      route: harness.state.route,
      settingsSection: harness.state.settingsSection
    })
    for (const hostileField of hostileFields) {
      expect(rendererState).not.toContain(hostileField)
    }
  })

  it('waits for the runtime probe and first workbench preload before boot resolves', async () => {
    const calls: string[] = []
    const runtimeStatusSinkRef: { current: ((payload: unknown) => void) | null } = { current: null }
    const provider = {
      connect: vi.fn(async () => {
        calls.push('connect')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('document', {
      documentElement: {
        dataset: {},
        style: { setProperty: vi.fn() },
        setAttribute: vi.fn(),
        getAttribute: vi.fn(() => null)
      }
    })
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/Users/sun/Projects/analytix',
            theme: 'light',
            uiFontScale: 'medium',
            cursorSpotlight: true,
            locale: 'zh',
            disabledSkillIds: [],
            write: {
              defaultWorkspaceRoot: '',
              activeWorkspaceRoot: '',
              workspaces: []
            },
            claw: { channels: [] },
            runtime: { providerId: 'deepseek', model: 'deepseek-chat' }
          }))
        },
        providerRegistry: {
          request: vi.fn(async () => ({
            schemaVersion: 1,
            registryRevision: '1',
            registryIncarnation: `inc_${'a'.repeat(43)}`,
            selectedProviderId: 'deepseek',
            providers: [{
              id: 'deepseek',
              kind: 'deepseek',
              endpoint: 'https://api.deepseek.com',
              models: ['deepseek-chat'],
              mediaModels: [],
              selectedModel: 'deepseek-chat',
              selectedRoutes: [],
              credentialConfigured: true,
              credentialPurpose: 'provider-api-key',
              revision: '1',
              generation: '1',
              incarnation: `inc_${'b'.repeat(43)}`,
              tombstone: false
            }]
          }))
        },
        runtime: {
          onRuntimeStatus: vi.fn((handler: (payload: unknown) => void) => {
            runtimeStatusSinkRef.current = handler
            return () => undefined
          })
        },
        connectPhone: {
          onChannelActivity: vi.fn(() => () => undefined)
        }
      }
    })
    const harness = buildHarness({
      runtimeConnection: 'idle',
      applyI18nFromSettings: vi.fn(async () => undefined)
    } as unknown as Partial<ChatState>)
    harness.loadComposerModels.mockImplementation(async () => {
      calls.push('models')
    })
    harness.refreshThreads.mockImplementation(async () => {
      calls.push('threads')
    })
    harness.state.probeRuntime = harness.actions.probeRuntime

    await expect(harness.actions.boot()).resolves.toBeUndefined()

    expect(provider.connect).toHaveBeenCalledTimes(1)
    expect(harness.loadComposerModels).toHaveBeenCalledTimes(1)
    expect(harness.refreshThreads).toHaveBeenCalledTimes(1)
    expect(harness.state.runtimeConnection).toBe('ready')
    expect(calls[0]).toBe('connect')
    expect(calls.slice(1).sort()).toEqual(['models', 'threads'])

    const accepted = createRuntimeStatusPublicV1({
      code: 'supervisor_unexpected_exit',
      source: 'supervisor',
      stderrBytes: 0,
      stderrSha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
      at: '2026-07-20T06:30:00.000Z'
    })
    const runtimeStatusSink = runtimeStatusSinkRef.current
    expect(runtimeStatusSink).not.toBeNull()
    if (!runtimeStatusSink) throw new Error('runtime status handler was not registered')
    runtimeStatusSink({ ...accepted, message: 'private crash output' })
    expect(harness.state.runtimeStatus).toBeUndefined()
    runtimeStatusSink(accepted)
    expect(harness.state.runtimeStatus).toEqual(accepted)
  })

  it.each([
    {
      name: 'fresh profile',
      registry: {
        schemaVersion: 1,
        registryRevision: '1',
        registryIncarnation: `inc_${'a'.repeat(43)}`,
        providers: []
      },
      expected: {
        route: 'chat',
        initialSetupOpen: true,
        settingsSection: undefined,
        error: null
      }
    },
    {
      name: 'configured but unusable profile',
      registry: {
        schemaVersion: 1,
        registryRevision: '1',
        registryIncarnation: `inc_${'a'.repeat(43)}`,
        selectedProviderId: 'deepseek',
        providers: [{
          id: 'deepseek',
          kind: 'deepseek',
          endpoint: 'https://api.deepseek.com',
          models: ['deepseek-chat'],
          mediaModels: [],
          selectedModel: 'deepseek-chat',
          selectedRoutes: [],
          credentialConfigured: false,
          credentialPurpose: 'provider-api-key',
          revision: '1',
          generation: '1',
          incarnation: `inc_${'b'.repeat(43)}`,
          tombstone: false
        }]
      },
      expected: {
        route: 'settings',
        initialSetupOpen: false,
        settingsSection: 'providers',
        error: 'Local Provider recovery is required. Open Settings to review the selected Provider.'
      }
    }
  ])('routes a $name without probing the model runtime', async ({ registry, expected }) => {
    const provider = { connect: vi.fn(async () => undefined) }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('document', {
      documentElement: {
        dataset: {},
        style: { setProperty: vi.fn() },
        setAttribute: vi.fn(),
        getAttribute: vi.fn(() => null)
      }
    })
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/Users/sun/Projects/analytix',
            theme: 'light',
            uiFontScale: 'medium',
            cursorSpotlight: true,
            locale: 'zh',
            disabledSkillIds: [],
            write: {
              defaultWorkspaceRoot: '',
              activeWorkspaceRoot: '',
              workspaces: []
            },
            claw: { channels: [] },
            runtime: { providerId: 'deepseek', model: 'deepseek-chat' }
          }))
        },
        providerRegistry: { request: vi.fn(async () => registry) },
        runtime: { onRuntimeStatus: vi.fn(() => () => undefined) },
        connectPhone: { onChannelActivity: vi.fn(() => () => undefined) }
      }
    })
    const harness = buildHarness({
      runtimeConnection: 'idle',
      applyI18nFromSettings: vi.fn(async () => undefined)
    } as unknown as Partial<ChatState>)
    harness.state.probeRuntime = harness.actions.probeRuntime

    await expect(harness.actions.boot()).resolves.toBeUndefined()

    expect(harness.state.route).toBe(expected.route)
    expect(harness.state.initialSetupOpen).toBe(expected.initialSetupOpen)
    expect(harness.state.settingsSection).toBe(expected.settingsSection)
    expect(harness.state.error).toBe(expected.error)
    expect(harness.state.runtimeConnection).toBe('idle')
    expect(provider.connect).not.toHaveBeenCalled()
    expect(harness.loadComposerModels).not.toHaveBeenCalled()
    expect(harness.refreshThreads).not.toHaveBeenCalled()
    expect(JSON.stringify({
      error: harness.state.error,
      runtimeErrorDetail: harness.state.runtimeErrorDetail
    })).not.toContain('api.deepseek.com')
  })

  it('does not let a stale background probe failure overwrite a newer ready result', async () => {
    let rejectStaleProbe: (reason: Error) => void = () => undefined
    const staleProbe = new Promise<void>((_resolve, reject) => {
      rejectStaleProbe = reject
    })
    const provider = {
      connect: vi.fn()
        .mockImplementationOnce(() => staleProbe)
        .mockResolvedValueOnce(undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'ready' } as Partial<ChatState>)

    const first = harness.actions.probeRuntime('background')
    await vi.waitFor(() => expect(provider.connect).toHaveBeenCalledTimes(1))
    await harness.actions.probeRuntime('background')
    expect(harness.state.runtimeConnection).toBe('ready')

    rejectStaleProbe(new Error('stale probe failed'))
    await first

    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.state.error).toBeNull()
  })

  it('does not let a stale probe success overwrite a newer user probe failure', async () => {
    let resolveStaleProbe: () => void = () => undefined
    const staleProbe = new Promise<void>((resolve) => {
      resolveStaleProbe = resolve
    })
    const provider = {
      connect: vi.fn()
        .mockImplementationOnce(() => staleProbe)
        .mockRejectedValueOnce(new Error('user probe failed'))
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'ready' } as Partial<ChatState>)

    const first = harness.actions.probeRuntime('background')
    await vi.waitFor(() => expect(provider.connect).toHaveBeenCalledTimes(1))
    await harness.actions.probeRuntime('user')
    expect(harness.state.runtimeConnection).toBe('offline')

    resolveStaleProbe()
    await first

    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBeTruthy()
  })

  it('does not let a background probe failure interrupt an active user probe', async () => {
    let resolveUserProbe: () => void = () => undefined
    const userProbe = new Promise<void>((resolve) => {
      resolveUserProbe = resolve
    })
    const provider = {
      connect: vi.fn()
        .mockImplementationOnce(() => userProbe)
        .mockRejectedValueOnce(new Error('background probe failed'))
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'ready' } as Partial<ChatState>)

    const first = harness.actions.probeRuntime('user')
    await vi.waitFor(() => expect(provider.connect).toHaveBeenCalledTimes(1))
    await harness.actions.probeRuntime('background')

    expect(harness.state.runtimeConnection).toBe('checking')
    expect(harness.state.error).toBeNull()

    resolveUserProbe()
    await first

    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.state.error).toBeNull()
  })

  it('does not move the only default thread into a newly picked empty workspace', async () => {
    const provider = {
      updateThreadWorkspace: vi.fn(async () => undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const pickWorkspaceDirectory = vi.fn(async () => ({
      canceled: false,
      path: '/Users/zxy/new-project'
    }))
    const setSettings = vi.fn(async () => ({
      workspaceRoot: '/Users/zxy/new-project'
    }))
    vi.stubGlobal('window', {
      analytix: {
        workspace: { pickDirectory: pickWorkspaceDirectory },
        settings: { setSettings }
      }
    })
    const harness = buildHarness()

    await expect(harness.actions.chooseWorkspace()).resolves.toBe('/Users/zxy/new-project')

    expect(pickWorkspaceDirectory).toHaveBeenCalledWith('~/.analytix/default_workspace')
    expect(setSettings).toHaveBeenCalledWith({ workspaceRoot: '/Users/zxy/new-project' })
    expect(provider.updateThreadWorkspace).not.toHaveBeenCalled()
    expect(harness.state.threads.find((item) => item.id === 'thr_default')?.workspace)
      .toBe('~/.analytix/default_workspace')
    expect(harness.createThread).toHaveBeenCalledWith({ workspaceRoot: '/Users/zxy/new-project' })
    expect(harness.selectThread).not.toHaveBeenCalled()
  })

  it('selectWorkspaceRoot persists the directory and lands on a clean new conversation', async () => {
    const setSettings = vi.fn(async () => ({ workspaceRoot: '/Users/zxy/new-project' }))
    vi.stubGlobal('window', { analytix: { settings: { setSettings } } })
    const harness = buildHarness()

    await expect(harness.actions.selectWorkspaceRoot('/Users/zxy/new-project'))
      .resolves.toBe('/Users/zxy/new-project')

    expect(setSettings).toHaveBeenCalledWith({ workspaceRoot: '/Users/zxy/new-project' })
    expect(harness.state.workspaceRoot).toBe('/Users/zxy/new-project')
    expect(harness.state.workspaceLabel).toBe('new-project')
    // Clean empty-hero state so typing starts a fresh thread in the new directory.
    expect(harness.state.activeThreadId).toBeNull()
    expect(harness.state.blocks).toEqual([])
    expect(harness.state.codeWorkspaceRoots).toContain('/Users/zxy/new-project')
    expect(harness.refreshThreads).toHaveBeenCalled()
    // The default thread is preserved in the listing, just not active.
    expect(harness.selectThread).not.toHaveBeenCalled()
    expect(harness.createThread).not.toHaveBeenCalled()
  })

  it('selectWorkspaceRoot ignores an empty path', async () => {
    const setSettings = vi.fn(async () => ({ workspaceRoot: '' }))
    vi.stubGlobal('window', { analytix: { settings: { setSettings } } })
    const harness = buildHarness()

    await expect(harness.actions.selectWorkspaceRoot('   ')).resolves.toBeNull()
    expect(setSettings).not.toHaveBeenCalled()
    expect(harness.state.activeThreadId).toBe('thr_default')
  })

  it('opens the legacy document route without changing the active conversation or stream', async () => {
    const blocks = [{ kind: 'assistant', id: 'retained', text: 'retained conversation' }]
    const harness = buildHarness({ activeThreadId: 'current-thread', blocks, busy: true } as unknown as Partial<ChatState>)
    await harness.actions.openWrite()
    expect(harness.state.route).toBe('write')
    expect(harness.state.activeThreadId).toBe('current-thread')
    expect(harness.state.blocks).toEqual(blocks)
    expect(harness.state.busy).toBe(true)
    expect(harness.selectThread).not.toHaveBeenCalled()
    expect(harness.createThread).not.toHaveBeenCalled()
  })

  it('opens the Code home in one clean state when no code thread is available', async () => {
    const harness = buildHarness({
      activeThreadId: 'write-thread',
      blocks: [{ kind: 'assistant', id: 'stale-write-message', text: 'stale write content' }],
      route: 'write',
      threads: []
    } as unknown as Partial<ChatState>)

    await harness.actions.openCode()

    expect(harness.selectThread).not.toHaveBeenCalled()
    expect(harness.setCalls[0]).toMatchObject({
      route: 'chat',
      activeThreadId: null,
      blocks: []
    })
    expect(harness.state.route).toBe('chat')
    expect(harness.state.activeThreadId).toBeNull()
    expect(harness.state.blocks).toEqual([])
  })

  it('clears stale write content without automatically selecting an existing Code thread', async () => {
    const harness = buildHarness({
      activeThreadId: 'write-thread',
      blocks: [{ kind: 'assistant', id: 'stale-write-message', text: 'stale write content' }],
      route: 'write',
      threads: [
        thread({
          id: 'code-thread',
          title: 'Code thread',
          workspace: '~/.analytix/default_workspace'
        })
      ]
    } as unknown as Partial<ChatState>)

    await harness.actions.openCode()

    expect(harness.setCalls[0]).toMatchObject({
      route: 'chat',
      activeThreadId: null,
      blocks: []
    })
    expect(harness.selectThread).not.toHaveBeenCalled()
    expect(harness.state.activeThreadId).toBeNull()
  })

  it('does not preserve an active child side thread in the main sidebar list', async () => {
    const parentThread = thread({ id: 'thr_parent', title: 'Parent desktop task', workspace: '/cases/a' })
    const childThread = thread({
      id: 'thr_child',
      title: 'Child agent: Turing',
      workspace: '/cases/a',
      relation: 'side',
      parentThreadId: parentThread.id
    })
    const provider = {
      listThreads: vi.fn(async () => [parentThread]),
      getThreadDetail: vi.fn(async () => ({ blocks: [] }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({
      activeThreadId: childThread.id,
      threads: [childThread],
      unreadThreadIds: { [childThread.id]: true },
      watchTurnCompletion: { [childThread.id]: true },
      threadSearch: '',
      showArchivedThreads: false,
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()

    expect(provider.listThreads).toHaveBeenCalledWith({
      limit: 50,
      search: undefined,
      includeArchived: true
    })
    expect(harness.state.threads.map((item) => item.id)).toEqual([parentThread.id])
    expect(harness.state.activeThreadId).toBeNull()
    expect(harness.state.unreadThreadIds[childThread.id]).toBeUndefined()
    expect(harness.state.watchTurnCompletion[childThread.id]).toBeUndefined()
  })

  it('keeps active thread and watch state when refreshing expanded case-project summaries', async () => {
    const provider = {
      listThreads: vi.fn(async () => []),
      listCaseProjects: vi.fn(async () => ({
        indexStatus: 'ready' as const,
        caseProjects: [{
          id: 'case_a',
          name: 'case-a',
          rootPath: '/cases/a',
          updatedAt: '2026-06-12T00:00:00.000Z',
          threadCount: 2,
          runningCount: 0,
          archivedCount: 0
        }]
      })),
      listCaseProjectThreads: vi.fn(async () => [
        thread({ id: 'thr_case_loaded', workspace: '/cases/a', updatedAt: '2026-06-12T01:00:00.000Z' })
      ]),
      getThreadDetail: vi.fn(async (threadId: string) => ({
        blocks: [],
        threadStatus: threadId === 'thr_active' ? 'running' : 'idle'
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        app: { showTurnCompleteNotification: vi.fn(async () => ({ ok: true })) }
      }
    })
    const activeThread = thread({ id: 'thr_active', workspace: '/cases/a', status: 'running' })
    const unreadThread = thread({ id: 'thr_unread', workspace: '/cases/a' })
    const sideThread = thread({
      id: 'thr_child',
      title: 'Child agent: Turing',
      workspace: '/cases/a',
      relation: 'side',
      parentThreadId: activeThread.id
    })
    const legacyChildThread = thread({
      id: 'thr_legacy_child',
      title: 'Child agent: legacy',
      workspace: '/cases/a',
      parentThreadId: activeThread.id
    })
    const harness = buildHarness({
      activeThreadId: activeThread.id,
      caseProjects: [],
      caseProjectIndexStatus: 'ready',
      caseProjectThreadsById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectExpandedById: { case_a: true },
      threadSearch: '',
      showArchivedThreads: false,
      threads: [activeThread, unreadThread, sideThread, legacyChildThread],
      watchTurnCompletion: { [activeThread.id]: true },
      unreadThreadIds: { [unreadThread.id]: true, [sideThread.id]: true },
      workspaceRoot: '/cases/a',
      codeWorkspaceRoots: ['/cases/a']
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()

    expect(provider.listCaseProjects).toHaveBeenCalled()
    expect(provider.listThreads).toHaveBeenCalledWith({
      limit: 50,
      search: undefined,
      includeArchived: true
    })
    expect(provider.listCaseProjectThreads).toHaveBeenCalledWith('case_a', 200)
    expect(harness.state.activeThreadId).toBe(activeThread.id)
    expect(harness.state.watchTurnCompletion[activeThread.id]).toBe(true)
    expect(harness.state.unreadThreadIds[unreadThread.id]).toBe(true)
    expect(harness.state.threads.map((item) => item.id)).toEqual([
      'thr_case_loaded',
      'thr_active',
      'thr_unread'
    ])
    expect(harness.state.unreadThreadIds[sideThread.id]).toBeUndefined()
    expect(harness.state.caseProjectThreadsById.case_a.map((item) => item.id)).toEqual(['thr_case_loaded'])
  })

  it('runs case-project summary side effects without loading collapsed project threads', async () => {
    const provider = {
      listThreads: vi.fn(async () => []),
      listCaseProjects: vi.fn(async () => ({
        indexStatus: 'ready' as const,
        caseProjects: [{
          id: 'case_a',
          name: 'case-a',
          rootPath: '/cases/a',
          updatedAt: '2026-06-12T00:00:00.000Z',
          threadCount: 2,
          runningCount: 0,
          archivedCount: 0
        }]
      })),
      listCaseProjectThreads: vi.fn(async () => [
        thread({ id: 'thr_should_not_load', workspace: '/cases/a' })
      ]),
      getThreadDetail: vi.fn(async () => ({
        blocks: [],
        threadStatus: 'idle'
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const activeThread = thread({ id: 'thr_active', workspace: '/cases/a', status: 'running' })
    const unreadThread = thread({ id: 'thr_unread', workspace: '/cases/a' })
    const harness = buildHarness({
      activeThreadId: activeThread.id,
      caseProjects: [],
      caseProjectIndexStatus: 'ready',
      caseProjectThreadsById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectExpandedById: {},
      threadSearch: '',
      showArchivedThreads: false,
      threads: [activeThread, unreadThread],
      watchTurnCompletion: { [activeThread.id]: true },
      unreadThreadIds: { [unreadThread.id]: true },
      workspaceRoot: '/cases/a',
      codeWorkspaceRoots: ['/cases/a']
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()

    expect(provider.listCaseProjects).toHaveBeenCalled()
    expect(provider.listThreads).toHaveBeenCalledWith({
      limit: 50,
      search: undefined,
      includeArchived: true
    })
    expect(provider.listCaseProjectThreads).not.toHaveBeenCalled()
    expect(harness.state.activeThreadId).toBe(activeThread.id)
    expect(harness.state.watchTurnCompletion[activeThread.id]).toBe(true)
    expect(harness.state.unreadThreadIds[unreadThread.id]).toBe(true)
    expect(harness.state.threads.map((item) => item.id)).toEqual(['thr_active', 'thr_unread'])
  })

  it('keeps latest runtime threads visible when refreshing case-project summaries', async () => {
    const listedThread = thread({
      id: 'thr_runtime_latest',
      title: 'Runtime latest',
      workspace: '/cases/a',
      updatedAt: '2026-06-12T03:00:00.000Z'
    })
    const provider = {
      listThreads: vi.fn(async () => [listedThread]),
      listCaseProjects: vi.fn(async () => ({
        indexStatus: 'ready' as const,
        caseProjects: [{
          id: 'case_a',
          name: 'case-a',
          rootPath: '/cases/a',
          updatedAt: '2026-06-12T03:00:00.000Z',
          threadCount: 1,
          runningCount: 0,
          archivedCount: 0
        }]
      })),
      listCaseProjectThreads: vi.fn(async () => []),
      getThreadDetail: vi.fn(async () => ({
        blocks: [],
        threadStatus: 'idle'
      }))
    }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({
      activeThreadId: listedThread.id,
      caseProjects: [],
      caseProjectIndexStatus: 'ready',
      caseProjectThreadsById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectExpandedById: {},
      threadSearch: '',
      showArchivedThreads: false,
      threads: [],
      workspaceRoot: '/cases/a',
      codeWorkspaceRoots: ['/cases/a']
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()

    expect(provider.listThreads).toHaveBeenCalledWith({
      limit: 50,
      search: undefined,
      includeArchived: true
    })
    expect(harness.state.threads.map((item) => item.id)).toEqual(['thr_runtime_latest'])
    expect(harness.state.activeThreadId).toBe(listedThread.id)
  })

  it('keeps the runtime ready when a sidebar refresh fails but the runtime probe succeeds', async () => {
    const listedThread = thread({
      id: 'thr_runtime_healthy',
      title: 'Runtime remains healthy',
      workspace: '/cases/a'
    })
    const provider = {
      connect: vi.fn(async () => undefined),
      listThreads: vi.fn(async () => [listedThread]),
      listCaseProjects: vi.fn(async () => {
        throw new Error('case project index unavailable')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({
      activeThreadId: listedThread.id,
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      threads: [listedThread],
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()

    expect(provider.connect).toHaveBeenCalledTimes(1)
    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.state.error).toBeTruthy()
  })

  it('marks the runtime offline when both sidebar refresh and runtime probe fail', async () => {
    const listedThread = thread({
      id: 'thr_runtime_unhealthy',
      title: 'Runtime becomes unavailable',
      workspace: '/cases/a'
    })
    const provider = {
      connect: vi.fn(async () => {
        throw new Error('runtime unavailable')
      }),
      listThreads: vi.fn(async () => [listedThread]),
      listCaseProjects: vi.fn(async () => {
        throw new Error('case project index unavailable')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({
      activeThreadId: listedThread.id,
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      threads: [listedThread],
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()

    expect(provider.connect).toHaveBeenCalledTimes(1)
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBeTruthy()
  })

  it('recovers from a transient thread-list probe failure with a bounded background retry', async () => {
    vi.useFakeTimers()
    const listedThread = thread({
      id: 'thr_runtime_recovered',
      title: 'Runtime recovers',
      workspace: '/cases/a'
    })
    const provider = {
      connect: vi.fn()
        .mockRejectedValueOnce(new Error('runtime is still starting'))
        .mockResolvedValueOnce(undefined),
      listThreads: vi.fn()
        .mockRejectedValueOnce(new Error('thread list is not ready'))
        .mockRejectedValueOnce(new Error('thread list is not ready'))
        .mockResolvedValueOnce([listedThread])
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({
      activeThreadId: listedThread.id,
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      threads: [listedThread],
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    const refresh = harness.actions.refreshThreads()
    await vi.waitFor(() => expect(provider.connect).toHaveBeenCalledTimes(1))
    await refresh

    expect(harness.state.runtimeConnection).toBe('offline')
    expect(provider.connect).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1_000)
    expect(provider.connect).toHaveBeenCalledTimes(2)
    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.state.threads.map((item) => item.id)).toEqual([listedThread.id])

    await vi.advanceTimersByTimeAsync(10_000)
    expect(provider.connect).toHaveBeenCalledTimes(2)
  })

  it('recovers when a runtime-status background probe fails transiently', async () => {
    vi.useFakeTimers()
    const provider = {
      connect: vi.fn()
        .mockRejectedValueOnce(new Error('runtime status raced startup'))
        .mockResolvedValueOnce(undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'ready' } as Partial<ChatState>)
    harness.state.probeRuntime = harness.actions.probeRuntime

    await harness.actions.probeRuntime('background')
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(provider.connect).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(250)
    expect(provider.connect).toHaveBeenCalledTimes(2)
    expect(harness.state.runtimeConnection).toBe('ready')
  })

  it('retries a failed runtime-status probe that starts while already offline', async () => {
    vi.useFakeTimers()
    const provider = {
      connect: vi.fn()
        .mockRejectedValueOnce(new Error('runtime is not ready yet'))
        .mockResolvedValueOnce(undefined)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({ runtimeConnection: 'offline' } as Partial<ChatState>)
    harness.state.probeRuntime = harness.actions.probeRuntime

    await harness.actions.probeRuntime('background')
    expect(harness.state.runtimeConnection).toBe('offline')

    await vi.advanceTimersByTimeAsync(250)
    expect(provider.connect).toHaveBeenCalledTimes(2)
    expect(harness.state.runtimeConnection).toBe('ready')
  })

  it('keeps a persistently unavailable runtime offline after three background retries', async () => {
    vi.useFakeTimers()
    const restartRuntime = vi.fn(async () => undefined)
    const provider = {
      connect: vi.fn(async () => {
        throw new Error('runtime remains unavailable')
      }),
      listThreads: vi.fn(async () => {
        throw new Error('thread list remains unavailable')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        },
        runtime: { restartRuntime }
      }
    })
    const harness = buildHarness({
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()
    expect(provider.connect).toHaveBeenCalledTimes(1)
    expect(harness.state.runtimeConnection).toBe('offline')

    await vi.advanceTimersByTimeAsync(250)
    expect(provider.connect).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1_000)
    expect(provider.connect).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(3_000)
    expect(provider.connect).toHaveBeenCalledTimes(4)
    await vi.advanceTimersByTimeAsync(10_000)

    expect(provider.connect).toHaveBeenCalledTimes(4)
    expect(restartRuntime).not.toHaveBeenCalled()
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBeTruthy()
  })

  it('does not let a failed user probe restart background recovery retries', async () => {
    vi.useFakeTimers()
    let rejectUserProbe: (reason: Error) => void = () => undefined
    const pendingUserProbe = new Promise<void>((_resolve, reject) => {
      rejectUserProbe = reject
    })
    const provider = {
      connect: vi.fn()
        .mockRejectedValueOnce(new Error('runtime is still starting'))
        .mockImplementationOnce(() => pendingUserProbe)
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.refreshThreads()
    expect(provider.connect).toHaveBeenCalledTimes(1)
    expect(harness.state.runtimeConnection).toBe('offline')

    const userProbe = harness.actions.probeRuntime('user')
    await vi.waitFor(() => expect(provider.connect).toHaveBeenCalledTimes(2))
    expect(harness.state.runtimeConnection).toBe('checking')
    await vi.advanceTimersByTimeAsync(10_000)
    expect(provider.connect).toHaveBeenCalledTimes(2)

    rejectUserProbe(new Error('user probe failed'))
    await userProbe
    expect(harness.state.runtimeConnection).toBe('offline')
    await vi.advanceTimersByTimeAsync(10_000)
    expect(provider.connect).toHaveBeenCalledTimes(2)
  })

  it('recovers when a successful user probe preload falls offline transiently', async () => {
    vi.useFakeTimers()
    const listedThread = thread({
      id: 'thr_user_probe_preload_recovered',
      title: 'User probe preload recovers',
      workspace: '/cases/a'
    })
    const provider = {
      connect: vi.fn()
        .mockResolvedValueOnce(undefined)
        .mockRejectedValueOnce(new Error('preload thread probe raced startup'))
        .mockResolvedValueOnce(undefined),
      listThreads: vi.fn()
        .mockRejectedValueOnce(new Error('thread list is not ready'))
        .mockRejectedValueOnce(new Error('thread list is not ready'))
        .mockResolvedValueOnce([listedThread])
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({
      activeThreadId: listedThread.id,
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      threads: [listedThread],
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.probeRuntime('user')
    expect(provider.connect).toHaveBeenCalledTimes(2)
    expect(harness.state.runtimeConnection).toBe('offline')

    await vi.advanceTimersByTimeAsync(250)
    expect(provider.connect).toHaveBeenCalledTimes(3)
    expect(harness.state.runtimeConnection).toBe('ready')
  })

  it('keeps the retry budget bounded across alternating probe and preload failures', async () => {
    vi.useFakeTimers()
    let connectAttempt = 0
    const provider = {
      connect: vi.fn(async () => {
        connectAttempt += 1
        if (connectAttempt % 2 === 1) throw new Error('runtime probe unavailable')
      }),
      listThreads: vi.fn(async () => {
        throw new Error('thread list preload unavailable')
      })
    }
    registryMock.getProvider.mockReturnValue(provider)
    vi.stubGlobal('window', {
      analytix: {
        settings: {
          getSettings: vi.fn(async () => ({
            workspaceRoot: '/cases/a',
            runtime: { providerId: 'analytix-hub', model: 'deepseek-v4-flash' }
          }))
        }
      }
    })
    const harness = buildHarness({
      caseProjectExpandedById: {},
      caseProjectLoadingById: {},
      caseProjectErrorsById: {},
      caseProjectThreadsById: {},
      threadSearch: '',
      showArchivedThreads: false,
      workspaceRoot: '/cases/a',
      runtimeConnection: 'ready'
    } as unknown as Partial<ChatState>)
    Object.assign(harness.state, harness.actions)

    await harness.actions.probeRuntime('background')
    expect(harness.state.runtimeConnection).toBe('offline')

    await vi.advanceTimersByTimeAsync(100_000)
    expect(provider.connect).toHaveBeenCalledTimes(7)
    expect(harness.state.runtimeConnection).toBe('offline')
  })
})


function deferredReceipt<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
async function settleReceipt() { for (let n = 0; n < 20; n++) await Promise.resolve() }

describe('receipt-scoped navigation refresh', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    rendererRuntimeClient.invalidateSettings()
    registryMock.getProvider.mockReset()
  })
  afterEach(() => {
    rendererRuntimeClient.invalidateSettings()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })
  it.each([true, false])('checks ownership after write-workspace I/O (current=%s)', async currentAfterRead => {
    let current = true
    const settings = deferredReceipt<Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>>()
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockReturnValue(settings.promise)
    const provider = { listThreads: vi.fn(async () => [thread({ id: 'new', title: 'Fresh thread', workspace: '/synthetic/receipt' })]) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ threadSearch: '', showArchivedThreads: false })
    const before = harness.state
    const pending = harness.actions.refreshThreads({ isCurrent: () => current })
    await settleReceipt()
    expect(rendererRuntimeClient.getSettings).toHaveBeenCalledOnce()
    current = currentAfterRead
    settings.resolve({ write: { defaultWorkspaceRoot: '', activeWorkspaceRoot: '', workspaces: [] } } as unknown as Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>)
    await pending
    if (currentAfterRead) expect(harness.state.threads.some(value => value.id === 'new')).toBe(true)
    else {
      expect(harness.state).toEqual(before)
      expect(harness.setCalls).toEqual([])
    }
  })
  it.each(['resolved', 'rejected'])('does not change connection/error state after stale reconnect %s', async outcome => {
    let current = true
    const connection = deferredReceipt<void>()
    const provider = { listThreads: vi.fn(async () => { throw new Error('Synthetic listing failure') }), connect: vi.fn(() => connection.promise) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ threadSearch: '', showArchivedThreads: false })
    const before = harness.state
    const pending = harness.actions.refreshThreads({ isCurrent: () => current })
    await settleReceipt()
    expect(provider.connect).toHaveBeenCalledOnce()
    current = false
    if (outcome === 'resolved') connection.resolve()
    else connection.reject(new Error('Synthetic reconnect failure'))
    await pending
    expect(harness.state).toEqual(before)
    expect(harness.setCalls).toEqual([])
  })
  it.each([true, false])('checks ownership of the case-summary response (current=%s)', async currentAfterRead => {
    let current = true
    const listing = deferredReceipt<never[]>()
    registryMock.getProvider.mockReturnValue({ listCaseProjects: vi.fn(() => listing.promise) })
    const harness = buildHarness({ caseProjects: [], caseProjectThreadsById: {}, caseProjectLoadingById: {}, caseProjectErrorsById: {}, caseProjectExpandedById: {} })
    const pending = harness.actions.refreshCaseProjects({ isCurrent: () => current })
    current = currentAfterRead
    listing.resolve([])
    await pending
    expect(harness.setCalls).toHaveLength(currentAfterRead ? 1 : 0)
  })
  it.each(['resolved', 'rejected'])('releases only its loading marker after a stale case-thread %s response', async outcome => {
    let current = true
    const listing = deferredReceipt<NormalizedThread[]>()
    registryMock.getProvider.mockReturnValue({ listCaseProjectThreads: vi.fn(() => listing.promise) })
    const harness = buildHarness({ caseProjectThreadsById: {}, caseProjectLoadingById: {}, caseProjectErrorsById: {} })
    const pending = harness.actions.loadCaseProjectThreads('synthetic-case', { force: true, isCurrent: () => current })
    expect(harness.state.caseProjectLoadingById['synthetic-case']).toBe(true)
    const before = harness.state.threads
    current = false
    if (outcome === 'resolved') listing.resolve([])
    else listing.reject(new Error('Stale case-thread lookup'))
    await pending
    expect(harness.state.caseProjectLoadingById['synthetic-case']).toBe(false)
    expect(harness.state.caseProjectErrorsById['synthetic-case']).toBeNull()
    expect(harness.state.caseProjectThreadsById).toEqual({})
    expect(harness.state.threads).toEqual(before)
  })
})
