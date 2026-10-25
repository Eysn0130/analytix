import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NormalizedThread } from '../agent/types'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { createRuntimeStatusPublicV1 } from '@shared/analytix-runtime-status'
import { runtimeErrorToError } from '@shared/runtime-error'
import i18n from '../i18n'
import type { ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'
import { getThreadBinding } from './chat-store-thread-binding'
import { stopTurnCompletionPoll } from './chat-store-schedulers'
import * as navigationRuntime from './chat-store-runtime'
import { composerDraftKey, emptyComposerDraft } from './composer-drafts'
import { readThreadForkRegistry, saveThreadForkRegistry } from '../lib/thread-fork-registry'
import { readWriteThreadRegistry, saveWriteThreadRegistry } from '../write/write-thread-registry'

const registryMock = vi.hoisted(() => ({
  getProvider: vi.fn(),
  checkCredentialReadiness: vi.fn()
}))

vi.mock('../agent/registry', () => ({
  getProvider: registryMock.getProvider
}))

vi.mock('../account/local-provider-readiness', async (importOriginal) => ({
  ...await importOriginal<typeof import('../account/local-provider-readiness')>(),
  checkLocalProviderReadiness: registryMock.checkCredentialReadiness
}))

import { createNavigationActions } from './chat-store-navigation-actions'
import { createMaintenanceActions } from './chat-store-maintenance-actions'

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

function buildHarness(overrides: Partial<ChatState> = {}, sseAbortRef: { current: AbortController | null } = { current: null }): {
  actions: ReturnType<typeof createNavigationActions>
  state: ChatState
  createThread: ReturnType<typeof vi.fn>
  refreshThreads: ReturnType<typeof vi.fn>
  loadComposerModels: ReturnType<typeof vi.fn>
  selectThread: ReturnType<typeof vi.fn>
  setCalls: Partial<ChatState>[]
  sseAbortRef: { current: AbortController | null }
} {
  const createThread = vi.fn(async () => undefined)
  const refreshThreads = vi.fn(async () => undefined)
  const selectThread = vi.fn(async () => undefined)
  const loadComposerModels = vi.fn(async () => undefined)
  const setCalls: Partial<ChatState>[] = []
  let state = {
    activeThreadId: 'thr_default',
    queuedMessages: [],
    queuedMessagePauses: {},
    blocks: [],
    busy: false,
    clawChannels: [],
    caseProjects: [],
    caseProjectThreadsById: {},
    caseProjectLoadingById: {},
    caseProjectErrorsById: {},
    caseProjectExpandedById: {},
    composerDrafts: {},
    userInputDraftInterruptions: {},
    deletedUserInputDraftThreadIds: {},
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
    threadSearch: '',
    threadSearchResults: null,
    showArchivedThreads: false,
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
      sseAbortRef
    }),
    get state() {
      return state
    },
    createThread,
    refreshThreads,
    loadComposerModels,
    selectThread,
    setCalls,
    sseAbortRef
  }
}

describe('chat-store navigation workspace selection', () => {
  beforeEach(() => {
    rendererRuntimeClient.invalidateSettings()
    registryMock.getProvider.mockReset()
    registryMock.checkCredentialReadiness.mockReset().mockResolvedValue({ kind: 'ready', providerId: 'deepseek' })
  })

  afterEach(async () => {
    await i18n.changeLanguage('en')
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

  it('keeps initialization intact when secure storage becomes unavailable after restart', async () => {
    registryMock.getProvider.mockReturnValue({ connect: vi.fn(async () => undefined) })
    registryMock.checkCredentialReadiness.mockResolvedValue({ kind: 'recovery', reason: 'saved_connection_unavailable' })
    vi.stubGlobal('window', { analytix: {
      settings: { getSettings: vi.fn(async () => ({ workspaceRoot: '~/.analytix/default_workspace', runtime: { providerId: 'deepseek', model: 'deepseek-flash' } })) },
      runtime: { restartRuntime: vi.fn(async () => undefined) }
    } })
    const harness = buildHarness({ initialSetupOpen: false, runtimeConnection: 'idle' } as Partial<ChatState>)
    await harness.actions.probeRuntime('user', { restart: true })
    expect(harness.state.initialSetupOpen).toBe(false)
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(registryMock.checkCredentialReadiness).toHaveBeenCalledTimes(1)
    registryMock.checkCredentialReadiness.mockResolvedValue({ kind: 'ready', providerId: 'deepseek' })
    await harness.actions.probeRuntime('user')
    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.state.initialSetupOpen).toBe(false)
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
            accentColor: 'green',
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
      applyI18nFromSettings: vi.fn(async (locale: string) => { await i18n.changeLanguage(locale) })
    } as unknown as Partial<ChatState>)
    harness.loadComposerModels.mockImplementation(async () => {
      calls.push('models')
    })
    harness.refreshThreads.mockImplementation(async () => {
      calls.push('threads')
    })
    harness.state.probeRuntime = harness.actions.probeRuntime

    await expect(harness.actions.boot()).resolves.toBeUndefined()

    expect(document.documentElement.setAttribute).toHaveBeenCalledWith('data-accent', 'green')
    expect(document.documentElement.style.setProperty).toHaveBeenCalledWith('--ax-primary', '#168348')
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
        error: '已保存的连接暂时不可用。请重试，或打开设置检查供应商。原配置已保留。'
      }
    }
  ])('routes a $name and loads local history without probing the model runtime', async ({ registry, expected }) => {
    const localThread = thread({ id: 'thr_local_history', model: '', workspace: '/synthetic/local-history' })
    const provider = { connect: vi.fn(async () => undefined), listThreads: vi.fn(async () => [localThread]) }
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
      applyI18nFromSettings: vi.fn(async (locale: string) => { await i18n.changeLanguage(locale) })
    } as unknown as Partial<ChatState>)
    harness.state.probeRuntime = harness.actions.probeRuntime
    harness.state.refreshThreads = harness.actions.refreshThreads

    await expect(harness.actions.boot()).resolves.toBeUndefined()

    expect(harness.state.route).toBe(expected.route)
    expect(harness.state.initialSetupOpen).toBe(expected.initialSetupOpen)
    expect(harness.state.settingsSection).toBe(expected.settingsSection)
    expect(harness.state.error).toBe(expected.error)
    expect(harness.state.runtimeConnection).toBe('idle')
    expect(provider.connect).toHaveBeenCalledOnce()
    expect(provider.listThreads).toHaveBeenCalledOnce()
    expect(harness.state.threads).toContainEqual(localThread)
    expect(harness.createThread).not.toHaveBeenCalled()
    expect(harness.loadComposerModels).not.toHaveBeenCalled()
    expect(JSON.stringify({
      error: harness.state.error,
      runtimeErrorDetail: harness.state.runtimeErrorDetail
    })).not.toContain('api.deepseek.com')
  })

  it.each([
    { locale: 'zh', reason: 'saved_connection_unavailable', expected: '已保存的连接暂时不可用。请重试，或打开设置检查供应商。原配置已保留。' },
    { locale: 'en', reason: 'saved_connection_unavailable', expected: 'Your saved connection is temporarily unavailable. Retry or open Settings to review the Provider. Your configuration has been kept.' },
    { locale: 'zh', reason: 'credential_reentry_required', expected: '存储方式变更后，需要在设置中重新输入一次此供应商的凭据。原配置已保留。' },
    { locale: 'en', reason: 'credential_reentry_required', expected: 'This saved Provider credential needs to be re-entered once in Settings after the storage change. Your configuration has been kept.' }
  ])('stores localized $reason recovery guidance in $locale without generic/raw error details', async ({ locale, reason, expected }) => {
    await i18n.changeLanguage(locale)
    registryMock.getProvider.mockReturnValue({ connect: vi.fn(async () => undefined) })
    registryMock.checkCredentialReadiness.mockResolvedValue({ kind: 'recovery', reason })
    vi.stubGlobal('window', { analytix: {
      settings: { getSettings: vi.fn(async () => ({ workspaceRoot: '/workspace/fixture', runtime: {} })) },
      providerRegistry: { request: vi.fn() }
    } })
    const harness = buildHarness({ route: 'write', settingsSection: 'general', initialSetupOpen: false, initialSetupMode: 'required' })
    await harness.actions.probeRuntime('user')
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.route).toBe('settings')
    expect(harness.state.settingsSection).toBe('providers')
    expect(harness.state.initialSetupOpen).toBe(false)
    expect(harness.state.initialSetupMode).toBe('required')
    expect(harness.state.error).toBe(expected)
    expect(harness.state.runtimeErrorDetail).toBeNull()
    expect(registryMock.checkCredentialReadiness).toHaveBeenCalledTimes(1)
    expect(harness.loadComposerModels).not.toHaveBeenCalled()
  })

  it('keeps background credential recovery on the current page without reopening setup', async () => {
    vi.useFakeTimers()
    registryMock.getProvider.mockReturnValue({ connect: vi.fn(async () => undefined) })
    registryMock.checkCredentialReadiness.mockResolvedValue({ kind: 'recovery', reason: 'saved_connection_unavailable' })
    vi.stubGlobal('window', { analytix: {
      settings: { getSettings: vi.fn(async () => ({ workspaceRoot: '/workspace/fixture', runtime: {} })) },
      providerRegistry: { request: vi.fn() }
    } })
    const harness = buildHarness({ route: 'write', settingsSection: 'general', initialSetupOpen: false, initialSetupMode: 'required' })
    await harness.actions.probeRuntime('background')
    expect(registryMock.checkCredentialReadiness).toHaveBeenCalledTimes(1)
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBeTruthy()
    expect(harness.state.route).toBe('write')
    expect(harness.state.settingsSection).toBe('general')
    expect(harness.state.initialSetupOpen).toBe(false)
    expect(harness.state.initialSetupMode).toBe('required')
  })

  it('ignores an old user credential recovery after a newer user probe is ready', async () => {
    let finishOld!: (value: { kind: 'recovery'; reason: 'saved_connection_unavailable' }) => void
    const oldReadiness = new Promise<{ kind: 'recovery'; reason: 'saved_connection_unavailable' }>(resolve => { finishOld = resolve })
    registryMock.getProvider.mockReturnValue({ connect: vi.fn(async () => undefined) })
    registryMock.checkCredentialReadiness.mockImplementationOnce(() => oldReadiness)
      .mockResolvedValueOnce({ kind: 'ready', providerId: 'deepseek', model: 'deepseek-chat' })
    vi.stubGlobal('window', { analytix: {
      settings: { getSettings: vi.fn(async () => ({ workspaceRoot: '/workspace/fixture', runtime: {} })) },
      providerRegistry: { request: vi.fn() }
    } })
    const harness = buildHarness({ route: 'write', settingsSection: 'general', initialSetupOpen: false })
    const oldProbe = harness.actions.probeRuntime('user')
    await vi.waitFor(() => expect(registryMock.checkCredentialReadiness).toHaveBeenCalledTimes(1))
    await harness.actions.probeRuntime('user')
    finishOld({ kind: 'recovery', reason: 'saved_connection_unavailable' })
    await oldProbe
    expect(harness.state.runtimeConnection).toBe('ready')
    expect(harness.state.route).toBe('write')
    expect(harness.state.settingsSection).toBe('general')
    expect(harness.state.error).toBeNull()
    expect(harness.state.initialSetupOpen).toBe(false)
  })

  it('does not route a setup result through Provider recovery', async () => {
    registryMock.getProvider.mockReturnValue({ connect: vi.fn(async () => undefined) })
    registryMock.checkCredentialReadiness.mockResolvedValue({ kind: 'setup' })
    vi.stubGlobal('window', { analytix: {
      settings: { getSettings: vi.fn(async () => ({ workspaceRoot: '/workspace/fixture', runtime: {} })) },
      providerRegistry: { request: vi.fn() }
    } })
    const harness = buildHarness({ initialSetupOpen: true, initialSetupMode: 'required', settingsSection: 'general' })
    await harness.actions.probeRuntime('user')
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.route).toBe('chat')
    expect(harness.state.settingsSection).toBe('general')
    expect(harness.state.initialSetupOpen).toBe(true)
    expect(harness.state.initialSetupMode).toBe('required')
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

  it('keeps probing an unavailable local runtime at a bounded rate', async () => {
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
    expect(provider.connect).toHaveBeenCalledTimes(5)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(provider.connect).toHaveBeenCalledTimes(6)
    await vi.advanceTimersByTimeAsync(30_000)

    expect(provider.connect).toHaveBeenCalledTimes(7)
    expect(restartRuntime).not.toHaveBeenCalled()
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBeTruthy()
  })

  it('recovers when the local runtime becomes available after the initial retries', async () => {
    vi.useFakeTimers()
    const provider = {
      connect: vi.fn()
        .mockRejectedValueOnce(new Error('runtime still starting'))
        .mockRejectedValueOnce(new Error('runtime still starting'))
        .mockRejectedValueOnce(new Error('runtime still starting'))
        .mockRejectedValueOnce(new Error('runtime still starting'))
        .mockResolvedValue(undefined)
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
    await vi.advanceTimersByTimeAsync(250 + 1_000 + 3_000)
    expect(provider.connect).toHaveBeenCalledTimes(4)
    expect(harness.state.runtimeConnection).toBe('offline')

    await vi.advanceTimersByTimeAsync(10_000)
    expect(provider.connect).toHaveBeenCalledTimes(5)
    expect(harness.state.runtimeConnection).toBe('ready')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(provider.connect).toHaveBeenCalledTimes(5)
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

  it('caps the retry rate across alternating probe and preload failures', async () => {
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
    const attempts = provider.connect.mock.calls.length
    expect(attempts).toBeGreaterThan(7)
    expect(attempts).toBeLessThanOrEqual(13)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(provider.connect.mock.calls.length).toBeGreaterThan(attempts)
    expect(provider.connect.mock.calls.length).toBeLessThanOrEqual(attempts + 2)
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
  afterEach(async () => {
    await i18n.changeLanguage('en')
    rendererRuntimeClient.invalidateSettings()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })
  it.each(['before', 'during'] as const)('retires a list started %s A policy mutation without replacing its confirmed pair', async order => {
    const original: NormalizedThread = { ...thread({ id: 'policy-a', workspace: '/synthetic/policy' }),
      executionPolicyVersion: 2, approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }
    const updated: NormalizedThread = { ...original, approvalPolicy: 'never', sandboxMode: 'read-only' }
    const listing = deferredReceipt<NormalizedThread[]>(), patch = deferredReceipt<NormalizedThread>()
    const provider = { listThreads: vi.fn(() => listing.promise), updateThreadExecutionPolicy: vi.fn(() => patch.promise) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ activeThreadId: original.id, threads: [original] })
    const maintenance = createMaintenanceActions({ set: partial => {
      Object.assign(harness.state, typeof partial === 'function' ? partial(harness.state) : partial)
    }, get: () => harness.state, sseAbortRef: harness.sseAbortRef })
    const pendingRead = order === 'before' ? harness.actions.refreshThreads() : null
    const pendingMutation = maintenance.updateThreadExecutionPolicy(original.id, { approvalPolicy: 'never', sandboxMode: 'read-only' })
    const duringRead = order === 'during' ? harness.actions.refreshThreads() : null
    patch.resolve(updated); expect(await pendingMutation).toBe(true)
    listing.resolve([original]); await (pendingRead ?? duringRead)
    expect(harness.state.threads[0]).toMatchObject({ approvalPolicy: 'never', sandboxMode: 'read-only' })
    expect(harness.state.activeThreadId).toBe(original.id)
    expect(harness.state.error).toBeNull()
  })

  it('releases the case-project loading seat when A policy mutation retires its stale inventory', async () => {
    const original: NormalizedThread = { ...thread({ id: 'policy-a', workspace: '/synthetic/policy' }),
      executionPolicyVersion: 2, approvalPolicy: 'auto', sandboxMode: 'danger-full-access' }
    const listing = deferredReceipt<NormalizedThread[]>(), patch = deferredReceipt<NormalizedThread>()
    const provider = { listCaseProjectThreads: vi.fn(() => listing.promise), updateThreadExecutionPolicy: vi.fn(() => patch.promise) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ activeThreadId: original.id, threads: [original] })
    const maintenance = createMaintenanceActions({ set: partial => {
      Object.assign(harness.state, typeof partial === 'function' ? partial(harness.state) : partial)
    }, get: () => harness.state, sseAbortRef: harness.sseAbortRef })
    const pendingRead = harness.actions.loadCaseProjectThreads('policy-project', { force: true })
    expect(harness.state.caseProjectLoadingById['policy-project']).toBe(true)
    const pendingMutation = maintenance.updateThreadExecutionPolicy(original.id, { approvalPolicy: 'never', sandboxMode: 'read-only' })
    patch.resolve({ ...original, approvalPolicy: 'never', sandboxMode: 'read-only' })
    expect(await pendingMutation).toBe(true)
    listing.resolve([original]); await pendingRead
    expect(harness.state.caseProjectLoadingById['policy-project']).toBe(false)
    expect(harness.state.caseProjectErrorsById['policy-project']).toBeNull()
    expect(harness.state.caseProjectThreadsById['policy-project']).toBeUndefined()
    expect(harness.state.threads[0]).toMatchObject({ approvalPolicy: 'never', sandboxMode: 'read-only' })
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

describe('case-index refresh continuation ownership', () => {
  const result = (indexStatus: 'building' | 'ready') => ({ caseProjects: [], indexStatus })
  function indexHarness() {
    const harness = buildHarness({ caseProjects: [], caseProjectIndexStatus: 'ready',
      caseProjectThreadsById: {}, caseProjectLoadingById: {}, caseProjectErrorsById: {}, caseProjectExpandedById: {} })
    Object.assign(harness.state, harness.actions)
    return harness
  }
  beforeEach(() => {
    vi.useFakeTimers()
    registryMock.getProvider.mockReset()
  })
  afterEach(async () => {
    // Drain this fixture's scheduled callback even when a RED assertion exits early.
    // Resetting fake clocks alone would strand the baseline module's timer handle.
    try { await vi.runOnlyPendingTimersAsync() } finally { vi.useRealTimers() }
  })

  it('replaces an invalid receipt timer with one newer bounded continuation', async () => {
    let current = true
    const listCaseProjects = vi.fn().mockResolvedValueOnce(result('building'))
      .mockResolvedValueOnce(result('building')).mockResolvedValue(result('ready'))
    registryMock.getProvider.mockReturnValue({ listCaseProjects })
    const harness = indexHarness()
    await harness.actions.refreshCaseProjects({ isCurrent: () => current })
    expect(vi.getTimerCount()).toBe(1)
    current = false
    await harness.actions.refreshCaseProjects()
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(1500)
    expect(listCaseProjects).toHaveBeenCalledTimes(3)
    expect(harness.state.caseProjectIndexStatus).toBe('ready')
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(4500)
    expect(listCaseProjects).toHaveBeenCalledTimes(3)
  })

  it('cancels a prior building timer when the latest explicit refresh is ready', async () => {
    const listCaseProjects = vi.fn().mockResolvedValueOnce(result('building')).mockResolvedValue(result('ready'))
    registryMock.getProvider.mockReturnValue({ listCaseProjects })
    const harness = indexHarness()
    await harness.actions.refreshCaseProjects()
    expect(vi.getTimerCount()).toBe(1)
    await harness.actions.refreshCaseProjects()
    expect(harness.state.caseProjectIndexStatus).toBe('ready')
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(4500)
    expect(listCaseProjects).toHaveBeenCalledTimes(2)
  })

  it.each(['resolved', 'rejected'])('does not let an older %s I/O replace a newer building continuation', async outcome => {
    const old = deferredReceipt<ReturnType<typeof result>>()
    const listCaseProjects = vi.fn().mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce(result('building')).mockResolvedValue(result('ready'))
    registryMock.getProvider.mockReturnValue({ listCaseProjects })
    const harness = indexHarness()
    const pending = harness.actions.refreshCaseProjects()
    await harness.actions.refreshCaseProjects()
    expect(harness.state.caseProjectIndexStatus).toBe('building')
    const snapshot = harness.state
    if (outcome === 'resolved') old.resolve(result('ready'))
    else old.reject(new Error('Synthetic obsolete index response'))
    await expect(pending).resolves.toBeUndefined()
    expect(harness.state).toBe(snapshot)
    expect(harness.setCalls).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(1500)
    expect(listCaseProjects).toHaveBeenCalledTimes(3)
    expect(harness.state.caseProjectIndexStatus).toBe('ready')
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each(['response', 'timer'])('stops a pending index %s when the runtime disconnects', async boundary => {
    const listing = deferredReceipt<ReturnType<typeof result>>()
    const listCaseProjects = vi.fn().mockReturnValueOnce(listing.promise)
    registryMock.getProvider.mockReturnValue({ listCaseProjects })
    const harness = indexHarness()
    const pending = harness.actions.refreshCaseProjects()
    if (boundary === 'timer') {
      listing.resolve(result('building'))
      await pending
      expect(vi.getTimerCount()).toBe(1)
    }
    Object.assign(harness.state, { runtimeConnection: 'offline' })
    const snapshot = harness.state
    if (boundary === 'response') listing.resolve(result('building'))
    await pending
    await vi.advanceTimersByTimeAsync(1500)
    expect(harness.state).toBe(snapshot)
    expect(listCaseProjects).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not let an invalid caller cancel the current timer', async () => {
    const listCaseProjects = vi.fn().mockResolvedValueOnce(result('building')).mockResolvedValue(result('ready'))
    registryMock.getProvider.mockReturnValue({ listCaseProjects })
    const harness = indexHarness()
    await harness.actions.refreshCaseProjects()
    await harness.actions.refreshCaseProjects({ isCurrent: () => false })
    expect(listCaseProjects).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(1500)
    expect(listCaseProjects).toHaveBeenCalledTimes(2)
    expect(harness.state.caseProjectIndexStatus).toBe('ready')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('keeps a current listing failure observable without leaving a timer', async () => {
    const error = new Error('Synthetic current index failure')
    registryMock.getProvider.mockReturnValue({ listCaseProjects: vi.fn().mockRejectedValue(error) })
    const harness = indexHarness()
    await expect(harness.actions.refreshCaseProjects()).rejects.toBe(error)
    expect(harness.setCalls).toEqual([])
    expect(vi.getTimerCount()).toBe(0)
  })

  it('keeps the existing index timer local to its navigation action factory', async () => {
    const listCaseProjects = vi.fn().mockResolvedValueOnce(result('building'))
      .mockResolvedValueOnce(result('building')).mockResolvedValue(result('ready'))
    registryMock.getProvider.mockReturnValue({ listCaseProjects })
    const first = indexHarness()
    const second = indexHarness()
    await first.actions.refreshCaseProjects()
    await second.actions.refreshCaseProjects()
    expect(vi.getTimerCount()).toBe(2)
    await vi.advanceTimersByTimeAsync(1500)
    expect(listCaseProjects).toHaveBeenCalledTimes(4)
    expect(first.state.caseProjectIndexStatus).toBe('ready')
    expect(second.state.caseProjectIndexStatus).toBe('ready')
    expect(vi.getTimerCount()).toBe(0)
  })
})


describe('case-index continuation across actual runtime reconnection', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    registryMock.getProvider.mockReset()
    vi.stubGlobal('window', { analytix: {} })
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({
      write: { defaultWorkspaceRoot: '', activeWorkspaceRoot: '', workspaces: [] }
    } as unknown as Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>)
  })
  afterEach(async () => {
    try { await vi.runOnlyPendingTimersAsync() } finally {
      vi.restoreAllMocks()
      vi.unstubAllGlobals()
      vi.useRealTimers()
    }
  })
  it.each(['response', 'timer'])('does not revive a pre-disconnect index %s while reconnect preload is pending', async boundary => {
    const oldListing = deferredReceipt<{ caseProjects: never[]; indexStatus: 'building' }>()
    const newThreads = deferredReceipt<NormalizedThread[]>()
    const listCaseProjects = vi.fn()
      .mockImplementationOnce(() => boundary === 'response' ? oldListing.promise : Promise.resolve({ caseProjects: [], indexStatus: 'building' }))
      .mockResolvedValue({ caseProjects: [], indexStatus: 'ready' })
    const provider = {
      connect: vi.fn().mockRejectedValueOnce(new Error('Synthetic disconnect')).mockResolvedValue(undefined),
      listCaseProjects, listThreads: vi.fn(() => newThreads.promise)
    }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ threadSearch: '', showArchivedThreads: false,
      caseProjects: [], caseProjectIndexStatus: 'ready', caseProjectThreadsById: {},
      caseProjectLoadingById: {}, caseProjectErrorsById: {}, caseProjectExpandedById: {} })
    Object.assign(harness.state, harness.actions)
    const oldRefresh = harness.actions.refreshCaseProjects()
    if (boundary === 'timer') await oldRefresh
    await harness.actions.probeRuntime('user')
    expect(harness.state.runtimeConnection).toBe('offline')
    const reconnect = harness.actions.probeRuntime('user')
    try {
      await settleReceipt()
      expect(harness.state.runtimeConnection).toBe('ready')
      expect(provider.listThreads).toHaveBeenCalledOnce()
      const snapshot = harness.state
      oldListing.resolve({ caseProjects: [], indexStatus: 'building' })
      await oldRefresh
      await vi.advanceTimersByTimeAsync(1500)
      expect(listCaseProjects).toHaveBeenCalledOnce()
      expect(harness.state).toBe(snapshot)
    } finally {
      newThreads.resolve(harness.state.threads)
      await reconnect
    }
    expect(listCaseProjects).toHaveBeenCalledTimes(2)
    expect(harness.state.caseProjectIndexStatus).toBe('ready')
    await vi.advanceTimersByTimeAsync(1500)
    expect(listCaseProjects).toHaveBeenCalledTimes(2)
  })
})


function navigationMemoryStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
    removeItem: (key: string) => { values.delete(key) }
  }
}

describe('Queue workspace deletion receipts', () => {
  beforeEach(() => {
    registryMock.getProvider.mockReset()
    vi.stubGlobal('window', { analytix: {}, localStorage: navigationMemoryStorage() })
  })
  afterEach(() => { stopTurnCompletionPoll(); vi.unstubAllGlobals() })
  it.each(['all', 'second-rejected'] as const)('retires only confirmed deletion receipts when %s', async outcome => {
    const keep = { threadId: 'thr_default', id: 'same-id', text: 'retain default', sending: true }
    const harness = buildHarness({ threads: [thread({ id: 'thr_default', workspace: '/workspace/keep' }),
      thread({ id: 'delete-first', workspace: '/workspace/delete' }), thread({ id: 'delete-second', workspace: '/workspace/delete' })],
      queuedMessages: [keep, { threadId: 'delete-first', id: 'same-id', text: 'first', sending: true },
        { threadId: 'delete-second', id: 'same-id', text: 'second' }],
      queuedMessagePauses: { thr_default: 'interrupted', 'delete-first': 'failed', 'delete-second': 'failed' } })
    const deleteThread = vi.fn(async (id: string) => { if (outcome === 'second-rejected' && id === 'delete-second') throw new Error('Synthetic deletion rejected') })
    registryMock.getProvider.mockReturnValue({ deleteThread, listThreads: vi.fn(async () => [...harness.state.threads]) })
    await harness.actions.deleteWorkspace('/workspace/delete')
    expect(deleteThread.mock.calls).toEqual([['delete-first'], ['delete-second']])
    expect(harness.state.queuedMessages).toEqual(outcome === 'all' ? [keep] : [keep,
      { threadId: 'delete-second', id: 'same-id', text: 'second' }])
    expect(harness.state.queuedMessagePauses).toEqual(outcome === 'all' ? { thr_default: 'interrupted' }
      : { thr_default: 'interrupted', 'delete-second': 'failed' })
  })
})

describe('thread inventory projections and workspace deletion owners', () => {
  const root = '/synthetic/navigation-delete'
  const keepRoot = '/synthetic/navigation-keep'
  const namedThread = (id: string, workspace = root, overrides: Partial<NormalizedThread> = {}) =>
    thread({ id, title: 'Synthetic ' + id, workspace, ...overrides })
  const warningKey = (id: string) => {
    const requestId = 'input_' + 'a'.repeat(64)
    return JSON.stringify({ threadId: id, turnId: 'turn-' + id, itemId: 'item_' + requestId, requestId })
  }
  const fixtures: ReturnType<typeof buildHarness>[] = []
  function ownedHarness(overrides: Partial<ChatState> = {}, controller: AbortController | null = null) {
    const harness = buildHarness({ workspaceRoot: keepRoot, codeWorkspaceRoots: [keepRoot, root],
      activeThreadId: 'keep', threads: [namedThread('keep', keepRoot)], ...overrides }, { current: controller })
    fixtures.push(harness)
    return harness
  }
  function realRefresh(harness: ReturnType<typeof buildHarness>) {
    Object.assign(harness.state, harness.actions)
  }
  beforeEach(() => {
    vi.useFakeTimers()
    registryMock.getProvider.mockReset()
    vi.stubGlobal('window', { localStorage: navigationMemoryStorage(),
      analytix: { settings: { setSettings: vi.fn() } } })
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({
      write: { defaultWorkspaceRoot: '', activeWorkspaceRoot: '', workspaces: [] }
    } as unknown as Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>)
  })
  afterEach(async () => {
    for (const harness of fixtures.splice(0)) {
      Object.assign(harness.state, { runtimeConnection: 'offline' })
      harness.actions.setThreadSearch('')
    }
    stopTurnCompletionPoll()
    vi.clearAllTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('keeps search a pure projection and a limited inventory page preserves unloaded registries', async () => {
    const a = namedThread('keep', keepRoot), c = namedThread('unread', keepRoot)
    const match = namedThread('match', keepRoot), fork = namedThread('fork-match', keepRoot, { relation: 'fork', parentThreadId: a.id })
    saveThreadForkRegistry({ version: 1, forks: {
      'fork-match': { parentThreadId: a.id }, 'unloaded-fork': { parentThreadId: 'unloaded-parent' }
    } })
    saveWriteThreadRegistry({ version: 1, workspaces: {
      '/synthetic/unloaded-write': { activeThreadId: 'unloaded-write', threadIds: ['unloaded-write'] }
    } })
    const forks = readThreadForkRegistry(), writes = readWriteThreadRegistry()
    const inventory = [a, c]
    const harness = ownedHarness({ threads: inventory, threadSearch: 'needle',
      unreadThreadIds: { unread: true }, watchTurnCompletion: { unread: true }, busy: true })
    realRefresh(harness)
    const provider = { listThreads: vi.fn().mockResolvedValueOnce([
      match, fork, namedThread('side', keepRoot, { relation: 'side', parentThreadId: a.id }),
      namedThread('nonfork-child', keepRoot, { parentThreadId: a.id })
    ]).mockResolvedValue([match]) }
    registryMock.getProvider.mockReturnValue(provider)
    await harness.actions.refreshThreads()
    expect(harness.state.threadSearchResults?.query).toBe('needle')
    expect(harness.state.threadSearchResults?.threads.map(item => item.id)).toEqual([match.id, fork.id])
    expect(harness.state.threads).toBe(inventory)
    expect(harness.state.activeThreadId).toBe(a.id)
    expect(harness.state.unreadThreadIds).toEqual({ unread: true })
    expect(harness.state.watchTurnCompletion).toEqual({ unread: true })
    expect(readThreadForkRegistry()).toEqual(forks)
    expect(readWriteThreadRegistry()).toEqual(writes)
    harness.actions.setThreadSearch('')
    await vi.advanceTimersByTimeAsync(0)
    await settleReceipt()
    expect(provider.listThreads).toHaveBeenLastCalledWith({ limit: 50, search: undefined, includeArchived: true })
    expect(harness.state.threadSearchResults).toBeNull()
    expect(harness.state.threads.map(item => item.id)).toEqual(expect.arrayContaining([a.id, c.id, match.id]))
    expect(harness.state.unreadThreadIds.unread).toBe(true)
    expect(harness.state.watchTurnCompletion.unread).toBe(true)
    expect(readThreadForkRegistry().forks['unloaded-fork']).toEqual(forks.forks['unloaded-fork'])
    expect(readWriteThreadRegistry().workspaces['/synthetic/unloaded-write']).toEqual(writes.workspaces['/synthetic/unloaded-write'])
  })

  it.each(['resolved', 'rejected', 'ABA', 'clear', 'provider-swap', 'current-failure'] as const)(
    'owns debounced query receipts and never substitutes unfiltered results when %s', async mode => {
      const old = deferredReceipt<NormalizedThread[]>(), latest = deferredReceipt<NormalizedThread[]>()
      const provider = { listThreads: vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise),
        connect: vi.fn(async () => undefined) }
      const replacement = { listThreads: vi.fn(() => latest.promise), connect: vi.fn(async () => undefined) }
      registryMock.getProvider.mockReturnValue(provider)
      const harness = ownedHarness()
      realRefresh(harness)
      const inventory = harness.state.threads
      harness.actions.setThreadSearch('A')
      await vi.advanceTimersByTimeAsync(250)
      expect(provider.listThreads).toHaveBeenCalledOnce()
      if (mode === 'current-failure') {
        old.reject(new Error('Synthetic current search failure'))
        await settleReceipt()
        expect(provider.listThreads).toHaveBeenCalledOnce()
        expect(provider.connect).toHaveBeenCalledOnce()
        expect(harness.state.threadSearchResults).toBeNull()
        expect(harness.state.threads).toBe(inventory)
        expect(harness.state.error).toBeTruthy()
        return
      }
      if (mode === 'provider-swap') {
        registryMock.getProvider.mockReturnValue(replacement)
        void harness.actions.refreshThreads()
      } else if (mode === 'clear') {
        harness.actions.setThreadSearch('')
        await vi.advanceTimersByTimeAsync(0)
      } else {
        harness.actions.setThreadSearch('B')
        if (mode === 'ABA') harness.actions.setThreadSearch('A')
        await vi.advanceTimersByTimeAsync(250)
      }
      const fresh = namedThread('fresh-result', keepRoot)
      latest.resolve([fresh])
      await settleReceipt()
      const snapshot = harness.state, writes = harness.setCalls.length
      if (mode === 'clear') {
        expect(snapshot.threadSearchResults).toBeNull()
        expect(snapshot.threads.some(item => item.id === fresh.id)).toBe(true)
      } else {
        expect(snapshot.threadSearchResults).toEqual({ query: mode === 'ABA' || mode === 'provider-swap' ? 'A' : 'B', threads: [fresh] })
        expect(snapshot.threads).toBe(inventory)
      }
      if (mode === 'rejected') old.reject(new Error('Synthetic obsolete search failure'))
      else old.resolve([namedThread('obsolete-result', keepRoot)])
      await settleReceipt()
      expect(harness.state).toBe(snapshot)
      expect(harness.setCalls).toHaveLength(writes)
      expect(provider.connect).not.toHaveBeenCalled()
      expect(provider.listThreads).toHaveBeenCalledTimes(mode === 'provider-swap' ? 1 : 2)
      if (mode === 'provider-swap') expect(replacement.listThreads).toHaveBeenCalledOnce()
    })

  it.each(['inventory-first', 'query-first'] as const)('does not let refresh and query steal the latest receipt when %s', async order => {
    const old = deferredReceipt<NormalizedThread[]>(), latest = deferredReceipt<NormalizedThread[]>()
    const listThreads = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise)
    registryMock.getProvider.mockReturnValue({ listThreads })
    const harness = ownedHarness({ threadSearch: order === 'query-first' ? 'needle' : '' })
    realRefresh(harness)
    const pending = harness.actions.refreshThreads()
    harness.actions.setThreadSearch(order === 'query-first' ? '' : 'needle')
    await vi.advanceTimersByTimeAsync(order === 'query-first' ? 0 : 250)
    latest.resolve([namedThread('latest', keepRoot)])
    await settleReceipt()
    const snapshot = harness.state, writes = harness.setCalls.length
    old.resolve([namedThread('obsolete', keepRoot)])
    await pending
    expect(harness.state).toBe(snapshot)
    expect(harness.setCalls).toHaveLength(writes)
    expect(harness.state.threadSearchResults).toEqual(order === 'query-first' ? null
      : { query: 'needle', threads: [namedThread('latest', keepRoot)] })
    expect(harness.state.threads.some(item => item.id === 'obsolete')).toBe(false)
    expect(listThreads).toHaveBeenCalledTimes(2)
  })

  it.each(['all', 'first-rejected', 'second-rejected'] as const)(
    'uses complete workspace inventory and retires only confirmed owners when %s', async outcome => {
      const a = namedThread('delete-a'), b = namedThread('delete-b')
      const archived = namedThread('unloaded-archived', root, { status: 'archived', archived: true })
      const fork = namedThread('delete-fork', root, { relation: 'fork', parentThreadId: a.id })
      const side = namedThread('side', root, { relation: 'side', parentThreadId: a.id })
      const child = namedThread('nonfork-child', root, { parentThreadId: a.id })
      const keep = namedThread('keep', keepRoot), controller = new AbortController()
      const ids = [a.id, b.id, archived.id, fork.id]
      const draft = { ...emptyComposerDraft, input: 'Private synthetic draft', inputRevision: 1 }
      const harness = ownedHarness({ activeThreadId: a.id, busy: true, currentTurnId: 'turn-a',
        threads: [keep, a, b, fork, side, child],
        queuedMessages: [...ids, keep.id].map(threadId => ({ threadId, id: 'shared-intent', text: threadId, sending: true })),
        queuedMessagePauses: Object.fromEntries([...ids, keep.id].map(id => [id, 'failed' as const])),
        unreadThreadIds: Object.fromEntries([...ids, keep.id].map(id => [id, true])),
        watchTurnCompletion: Object.fromEntries([...ids, keep.id].map(id => [id, true])),
        composerDrafts: Object.fromEntries([...ids, keep.id].map(id => [composerDraftKey(id === keep.id ? keepRoot : root, id), draft])),
        userInputDraftInterruptions: Object.fromEntries([...ids, keep.id].map(id => [warningKey(id), { revision: null, unsent: true }])),
        userInputPanel: { threadId: a.id, requestId: null },
        threadSearchResults: { query: 'delete', threads: [a, b, archived, fork, keep] }
      }, controller)
      saveThreadForkRegistry({ version: 1, forks: { [fork.id]: { parentThreadId: a.id }, keep: { parentThreadId: 'keep-parent' } } })
      saveWriteThreadRegistry({ version: 1, workspaces: { [root]: { activeThreadId: a.id, threadIds: [a.id, b.id] },
        [keepRoot]: { activeThreadId: keep.id, threadIds: [keep.id] } } })
      const notification = vi.spyOn(navigationRuntime, 'clearWatchedCompletionNotification')
      const deleteThread = vi.fn(async (id: string) => {
        if (outcome === 'first-rejected' && id === a.id || outcome === 'second-rejected' && id === b.id) {
          throw new Error('Synthetic deletion rejected')
        }
      })
      const listThreads = vi.fn(async () => [keep, a, b, archived, fork, side, child])
      registryMock.getProvider.mockReturnValue({ listThreads, deleteThread })
      await harness.actions.deleteWorkspace(root)
      expect(listThreads).toHaveBeenCalledWith({ limit: 0, includeArchived: true })
      expect(deleteThread.mock.calls.map(([id]) => id)).toEqual(outcome === 'all' ? ids
        : outcome === 'first-rejected' ? [a.id] : [a.id, b.id])
      const deleted = outcome === 'all' ? ids : outcome === 'second-rejected' ? [a.id] : []
      for (const id of [...ids, keep.id]) {
        const retained = !deleted.includes(id)
        expect(harness.state.queuedMessages.some(item => item.threadId === id)).toBe(retained)
        expect(harness.state.queuedMessagePauses[id]).toBe(retained ? 'failed' : undefined)
        expect(harness.state.unreadThreadIds[id]).toBe(retained ? true : undefined)
        expect(harness.state.watchTurnCompletion[id]).toBe(retained ? true : undefined)
        expect(harness.state.composerDrafts[composerDraftKey(id === keep.id ? keepRoot : root, id)]).toBe(retained ? draft : undefined)
        expect(harness.state.userInputDraftInterruptions?.[warningKey(id)]).toEqual(retained ? { revision: null, unsent: true } : undefined)
        expect(harness.state.deletedUserInputDraftThreadIds?.[id]).toBe(retained ? undefined : true)
      }
      expect(notification.mock.calls.map(([id]) => id)).toEqual(deleted)
      expect(harness.state.threadSearchResults?.threads.map(item => item.id)).toEqual([a, b, archived, fork, keep].filter(item => !deleted.includes(item.id)).map(item => item.id))
      expect(harness.state.threads.map(item => item.id)).toEqual([keep, a, b, fork, side, child].filter(item => !deleted.includes(item.id)).map(item => item.id))
      expect(readThreadForkRegistry().forks[fork.id]).toEqual(outcome === 'all' ? undefined : { parentThreadId: a.id })
      expect(readThreadForkRegistry().forks.keep).toEqual({ parentThreadId: 'keep-parent' })
      expect(readWriteThreadRegistry().workspaces[keepRoot]).toEqual({ activeThreadId: keep.id, threadIds: [keep.id] })
      expect(readWriteThreadRegistry().workspaces[root]).toEqual(outcome === 'all' ? undefined
        : { activeThreadId: outcome === 'second-rejected' ? b.id : a.id,
          threadIds: outcome === 'second-rejected' ? [b.id] : [a.id, b.id] })
      expect(controller.signal.aborted).toBe(outcome !== 'first-rejected')
      expect(harness.state.activeThreadId).toBe(outcome === 'first-rejected' ? a.id : null)
      expect(harness.state.busy).toBe(outcome === 'first-rejected')
      if (outcome === 'first-rejected') {
        expect(harness.sseAbortRef.current).toBe(controller)
        expect(harness.state.userInputPanel?.threadId).toBe(a.id)
      } else expect(harness.state.userInputPanel).toBeUndefined()
      expect(harness.state.codeWorkspaceRoots.includes(root)).toBe(outcome !== 'all')
      if (outcome !== 'all') expect(harness.state.error).toBeTruthy()
    })

  it.each(['new-thread', 'ABA', 'later-dispatch', 'natural-EOF'] as const)('detaches only the owner captured at each DELETE dispatch when %s', async mode => {
    const first = deferredReceipt<void>(), second = deferredReceipt<void>()
    const a = namedThread('delete-a'), b = namedThread('delete-b'), original = new AbortController(), replacement = new AbortController()
    const harness = ownedHarness({ activeThreadId: mode === 'later-dispatch' ? 'keep' : a.id,
      busy: true, currentTurnId: 'original-turn', threads: [a, b, namedThread('keep', keepRoot)] }, original)
    const binding = getThreadBinding(harness.sseAbortRef)
    const deleteThread = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    registryMock.getProvider.mockReturnValue({ listThreads: vi.fn(async () => mode === 'later-dispatch' ? [a, b] : [a]), deleteThread })
    const pending = harness.actions.deleteWorkspace(root)
    await settleReceipt()
    expect(deleteThread).toHaveBeenCalledWith(a.id)
    if (mode === 'natural-EOF') {
      binding.naturallyRetired.add(original)
      harness.sseAbortRef.current = null
    } else {
      binding.generation += 1
      Object.assign(harness.state, { activeThreadId: b.id, currentTurnId: 'new-turn', busy: true })
      harness.sseAbortRef.current = replacement
      if (mode === 'ABA') {
        binding.generation += 1
        binding.submissionGeneration += 1
        Object.assign(harness.state, { activeThreadId: a.id, currentTurnId: 'replacement-a-turn' })
        harness.sseAbortRef.current = original
      }
    }
    first.resolve()
    await settleReceipt()
    expect(original.signal.aborted).toBe(mode === 'natural-EOF')
    expect(replacement.signal.aborted).toBe(false)
    if (mode === 'later-dispatch') {
      expect(deleteThread.mock.calls).toEqual([[a.id], [b.id]])
      expect(harness.state.activeThreadId).toBe(b.id)
      second.resolve()
      await pending
      expect(replacement.signal.aborted).toBe(true)
      expect(harness.sseAbortRef.current).toBeNull()
      expect(harness.state.activeThreadId).toBeNull()
      expect(harness.state.busy).toBe(false)
    } else if (mode === 'natural-EOF') {
      await pending
      expect(harness.state.activeThreadId).toBeNull()
      expect(harness.state.busy).toBe(false)
      expect(harness.sseAbortRef.current).toBeNull()
    } else {
      await pending
      expect(harness.state.activeThreadId).toBe(mode === 'ABA' ? a.id : b.id)
      expect(harness.state.busy).toBe(true)
      expect(harness.sseAbortRef.current).toBe(mode === 'ABA' ? original : replacement)
      expect(harness.state.codeWorkspaceRoots).toContain(root)
      expect(harness.refreshThreads).not.toHaveBeenCalled()
    }
  })

  it.each(['resolved', 'rejected'] as const)('keeps a newer same-root deletion error and root after an old DELETE is %s', async outcome => {
    const receipt = deferredReceipt<void>()
    const a = namedThread('delete-a')
    const harness = ownedHarness({ threads: [a, namedThread('keep', keepRoot)] })
    const listThreads = vi.fn().mockResolvedValueOnce([a]).mockRejectedValueOnce(new Error('Synthetic newer inventory failure'))
    const deleteThread = vi.fn(() => receipt.promise)
    registryMock.getProvider.mockReturnValue({ listThreads, deleteThread })
    const old = harness.actions.deleteWorkspace(root)
    await settleReceipt()
    await harness.actions.deleteWorkspace(root)
    const newerError = harness.state.error
    expect(newerError).toBeTruthy()
    if (outcome === 'resolved') receipt.resolve()
    else receipt.reject(new Error('Synthetic obsolete deletion failure'))
    await old
    expect(harness.state.error).toBe(newerError)
    expect(harness.state.codeWorkspaceRoots).toContain(root)
    expect(harness.refreshThreads).not.toHaveBeenCalled()
    expect(harness.state.threads.some(item => item.id === a.id)).toBe(outcome === 'rejected')
  })

  it.each(['ready', 'later-delete'] as const)('owns the post-delete building index lifecycle when %s', async mode => {
    const a = namedThread('delete-a'), keep = namedThread('keep', keepRoot)
    const laterListing = deferredReceipt<NormalizedThread[]>()
    let fullListings = 0
    const listThreads = vi.fn((options?: { limit?: number }) => {
      if (options?.limit === 0) {
        fullListings += 1
        return fullListings === 1 ? Promise.resolve([a]) : laterListing.promise
      }
      return Promise.resolve([keep])
    })
    const listCaseProjects = vi.fn()
      .mockResolvedValueOnce({ caseProjects: [], indexStatus: 'building' })
      .mockResolvedValue({ caseProjects: [], indexStatus: 'ready' })
    const deleteThread = vi.fn(async () => undefined)
    registryMock.getProvider.mockReturnValue({ listThreads, listCaseProjects, deleteThread })
    const harness = ownedHarness({ threads: [keep, a], caseProjectIndexStatus: 'ready' })
    realRefresh(harness)
    let laterIntent: Promise<void> | undefined
    try {
      await harness.actions.deleteWorkspace(root)
      expect(deleteThread.mock.calls).toEqual([[a.id]])
      expect(harness.state.codeWorkspaceRoots).not.toContain(root)
      expect(listCaseProjects).toHaveBeenCalledOnce()
      expect(harness.state.caseProjectIndexStatus).toBe('building')
      if (mode === 'later-delete') {
        laterIntent = harness.actions.deleteWorkspace(root)
        await settleReceipt()
        expect(fullListings).toBe(2)
        expect(listThreads).toHaveBeenLastCalledWith({ limit: 0, includeArchived: true })
      }
      const writes = harness.setCalls.length
      await vi.advanceTimersByTimeAsync(1500)
      await settleReceipt()
      expect(listCaseProjects).toHaveBeenCalledTimes(mode === 'ready' ? 2 : 1)
      expect(harness.state.caseProjectIndexStatus).toBe(mode === 'ready' ? 'ready' : 'building')
      if (mode === 'later-delete') expect(harness.setCalls).toHaveLength(writes)
    } finally {
      laterListing.resolve([])
      if (laterIntent) await laterIntent
    }
  })

  it.each(['rejected', 'late-workspace-switch'] as const)('preserves current workspace configuration while SETTINGS is %s', async outcome => {
    const settings = deferredReceipt<Awaited<ReturnType<typeof rendererRuntimeClient.setSettings>>>()
    const setSettings = vi.spyOn(rendererRuntimeClient, 'setSettings').mockReturnValue(settings.promise)
    const a = namedThread('delete-a')
    const harness = ownedHarness({ workspaceRoot: root, workspaceLabel: 'navigation-delete',
      threads: [a, namedThread('keep', keepRoot)] })
    registryMock.getProvider.mockReturnValue({ listThreads: vi.fn(async () => [a]), deleteThread: vi.fn(async () => undefined) })
    const pending = harness.actions.deleteWorkspace(root)
    await settleReceipt()
    expect(setSettings).toHaveBeenCalledWith({ workspaceRoot: '' })
    if (outcome === 'rejected') settings.reject(new Error('Synthetic settings failure'))
    else {
      getThreadBinding(harness.sseAbortRef).generation += 1
      Object.assign(harness.state, { workspaceRoot: '/synthetic/new-owner', workspaceLabel: 'new-owner',
        route: 'write', error: 'New workspace error', runtimeErrorDetail: 'New workspace detail' })
      settings.resolve({ workspaceRoot: '' } as Awaited<ReturnType<typeof rendererRuntimeClient.setSettings>>)
    }
    await pending
    expect(harness.state.threads.some(item => item.id === a.id)).toBe(false)
    expect(harness.state.codeWorkspaceRoots).toEqual([keepRoot, root])
    expect(harness.state.workspaceRoot).toBe(outcome === 'rejected' ? root : '/synthetic/new-owner')
    expect(harness.state.workspaceLabel).toBe(outcome === 'rejected' ? 'navigation-delete' : 'new-owner')
    expect(harness.state.error).toBeTruthy()
    if (outcome !== 'rejected') {
      expect(harness.state.error).toBe('New workspace error')
      expect(harness.state.runtimeErrorDetail).toBe('New workspace detail')
      expect(harness.state.route).toBe('write')
    }
    expect(harness.refreshThreads).not.toHaveBeenCalled()
  })

  it.each(['delete', 'settings', 'workspace-delete-reject'] as const)('does not overwrite a newer error owner after pending %s completes', async boundary => {
    const receipt = deferredReceipt<void>()
    const settings = deferredReceipt<Awaited<ReturnType<typeof rendererRuntimeClient.setSettings>>>()
    const setSettings = vi.spyOn(rendererRuntimeClient, 'setSettings').mockImplementation(patch => boundary === 'settings'
      ? settings.promise : Promise.resolve({ workspaceRoot: patch.workspaceRoot ?? '' } as Awaited<ReturnType<typeof rendererRuntimeClient.setSettings>>))
    const a = namedThread('delete-a')
    const harness = ownedHarness({ workspaceRoot: root, workspaceLabel: 'navigation-delete',
      activeThreadId: boundary === 'workspace-delete-reject' ? null : 'keep', threads: [a] })
    registryMock.getProvider.mockReturnValue({ listThreads: vi.fn(async () => [a]),
      deleteThread: vi.fn(() => boundary === 'settings' ? Promise.resolve() : receipt.promise) })
    const pending = harness.actions.deleteWorkspace(root)
    await settleReceipt()
    if (boundary === 'settings') expect(setSettings).toHaveBeenCalledOnce()
    if (boundary === 'workspace-delete-reject') {
      await harness.actions.selectWorkspaceRoot(keepRoot)
      expect(harness.state.activeThreadId).toBeNull()
      expect(harness.state.route).toBe('chat')
      expect(harness.state.workspaceRoot).toBe(keepRoot)
    }
    Object.assign(harness.state, { error: 'Newer error owner', runtimeErrorDetail: 'Newer detail owner' })
    const writes = harness.setCalls.length
    const refreshes = harness.refreshThreads.mock.calls.length
    if (boundary === 'workspace-delete-reject') receipt.reject(new Error('Synthetic obsolete workspace deletion failure'))
    else if (boundary === 'delete') receipt.resolve()
    else settings.resolve({ workspaceRoot: '' } as Awaited<ReturnType<typeof rendererRuntimeClient.setSettings>>)
    await pending
    expect(harness.state.threads.some(item => item.id === a.id)).toBe(boundary === 'workspace-delete-reject')
    expect(harness.state.error).toBe('Newer error owner')
    expect(harness.state.runtimeErrorDetail).toBe('Newer detail owner')
    expect(harness.state.workspaceRoot).toBe(boundary === 'workspace-delete-reject' ? keepRoot : '')
    expect(harness.state.codeWorkspaceRoots).toEqual(boundary === 'workspace-delete-reject' ? [keepRoot, root] : [keepRoot])
    expect(harness.refreshThreads).toHaveBeenCalledTimes(refreshes)
    expect(setSettings).toHaveBeenCalledOnce()
    if (boundary === 'workspace-delete-reject') expect(harness.setCalls).toHaveLength(writes)
  })
})


describe('local Core history without Provider readiness', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    registryMock.getProvider.mockReset()
    registryMock.checkCredentialReadiness.mockResolvedValue({ kind: 'setup' })
    vi.stubGlobal('window', { localStorage: navigationMemoryStorage(), analytix: {} })
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue({
      write: { defaultWorkspaceRoot: '', activeWorkspaceRoot: '', workspaces: [] }
    } as unknown as Awaited<ReturnType<typeof rendererRuntimeClient.getSettings>>)
  })
  afterEach(() => {
    stopTurnCompletionPoll()
    vi.clearAllTimers()
    rendererRuntimeClient.invalidateSettings()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it.each(['idle', 'offline'] as const)('loads, searches and shows archived local history while %s', async runtimeConnection => {
    const localThread = thread({ id: 'thr_local_inventory', model: '', workspace: '/synthetic/local-history' })
    const provider = { connect: vi.fn(async () => undefined), listThreads: vi.fn(async () => [localThread]) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ runtimeConnection, threads: [], activeThreadId: null })
    Object.assign(harness.state, harness.actions)
    await harness.actions.refreshThreads()
    expect(harness.state.threads).toEqual([localThread])
    harness.actions.setThreadSearch('local')
    await vi.advanceTimersByTimeAsync(250)
    expect(harness.state.threadSearchResults).toEqual({ query: 'local', threads: [localThread] })
    expect(provider.listThreads).toHaveBeenLastCalledWith({ limit: 50, search: 'local', includeArchived: true })
    harness.actions.setThreadSearch('')
    await vi.advanceTimersByTimeAsync(0)
    harness.actions.setShowArchivedThreads(true)
    await settleReceipt()
    expect(provider.listThreads).toHaveBeenLastCalledWith({ limit: 200, search: undefined, includeArchived: true })
    expect(harness.state.runtimeConnection).toBe(runtimeConnection)
    expect(harness.setCalls.some(call => call.runtimeConnection === 'ready')).toBe(false)
    expect(harness.createThread).not.toHaveBeenCalled()
    expect(harness.loadComposerModels).not.toHaveBeenCalled()
  })

  it('loads the existing case history owner without Provider readiness', async () => {
    const localThread = thread({ id: 'thr_local_case', model: '', workspace: '/synthetic/local-case' })
    const provider = { connect: vi.fn(async () => undefined), listThreads: vi.fn(async () => []),
      listCaseProjects: vi.fn(async () => ({ caseProjects: [], indexStatus: 'ready' })),
      listCaseProjectThreads: vi.fn(async () => [localThread]) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ runtimeConnection: 'offline', activeThreadId: null, threads: [] })
    Object.assign(harness.state, harness.actions)
    await harness.actions.refreshThreads()
    expect(provider.listCaseProjects).toHaveBeenCalledOnce()
    await harness.actions.loadCaseProjectThreads('local-case')
    expect(harness.state.caseProjectThreadsById['local-case']).toEqual([localThread])
    expect(harness.state.caseProjectLoadingById['local-case']).toBe(false)
    expect(harness.state.runtimeConnection).toBe('offline')
  })

  it('refreshes local history after a successful Core probe with missing Provider setup', async () => {
    const provider = { connect: vi.fn(async () => undefined) }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ runtimeConnection: 'idle' })
    await harness.actions.probeRuntime('user')
    expect(provider.connect).toHaveBeenCalledOnce()
    expect(harness.refreshThreads).toHaveBeenCalledExactlyOnceWith({ isCurrent: expect.any(Function) })
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.loadComposerModels).not.toHaveBeenCalled()
  })

  it.each(['revoked', 'provider-swap', 'disconnected'] as const)('ignores a local inventory connect after %s', async reason => {
    const connection = deferredReceipt<void>()
    const provider = { connect: vi.fn(() => connection.promise), listThreads: vi.fn() }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ runtimeConnection: 'idle' })
    let current = true
    const pending = harness.actions.refreshThreads({ isCurrent: () => current })
    if (reason === 'revoked') current = false
    if (reason === 'provider-swap') registryMock.getProvider.mockReturnValue({})
    if (reason === 'disconnected') Object.assign(harness.state, { runtimeConnection: 'offline' })
    const before = harness.state
    connection.resolve()
    await pending
    expect(provider.listThreads).not.toHaveBeenCalled()
    expect(harness.state).toBe(before)
    expect(harness.setCalls).toEqual([])
  })

  it('does not list local history when Core connection is unavailable', async () => {
    const provider = { connect: vi.fn(async () => { throw new Error('Synthetic Core unavailable') }), listThreads: vi.fn() }
    registryMock.getProvider.mockReturnValue(provider)
    const harness = buildHarness({ runtimeConnection: 'idle' })
    await harness.actions.refreshThreads()
    expect(provider.listThreads).not.toHaveBeenCalled()
    expect(harness.state.runtimeConnection).toBe('offline')
    expect(harness.state.error).toBeTruthy()
    expect(harness.createThread).not.toHaveBeenCalled()
  })
})
