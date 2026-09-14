import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest'
import type i18next from 'i18next'
import { DEFAULT_ANALYTIX_MODEL } from '@shared/app-settings'
import type { AppRoute, ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'
import {
  fallbackComposerModel,
  mergeComposerPickList,
  persistComposerModel,
  readStoredComposerModel
} from './chat-store-helpers'
import { createAppActions } from './chat-store-app-actions'

const COMPOSER_MODEL_STORAGE_KEY = 'analytix.composerModel'
const COMPOSER_PROVIDER_STORAGE_KEY = 'analytix.composerProviderId'
const THREAD_COMPOSER_SELECTION_STORAGE_KEY = 'analytix.threadComposerSelection.v1'

type ForbiddenTopLevelRoute =
  | 'workflow'
  | 'create-loop'
  | 'createLoop'
  | 'subagent'
  | 'subagents'
  | 'auto-research'
  | 'autoResearch'
  | 'mcp-indexer'
  | 'mcpIndexer'

const forbiddenTopLevelAppActions = [
  'openWorkflow',
  'openCreateLoop',
  'openSubagent',
  'openSubagents',
  'openAutoResearch',
  'openMCPIndexer',
  'openMcpIndexer'
]

function createMemoryStorage(): Storage {
  const items = new Map<string, string>()
  return {
    get length() {
      return items.size
    },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (index) => [...items.keys()][index] ?? null,
    removeItem: (key) => {
      items.delete(key)
    },
    setItem: (key, value) => {
      items.set(key, value)
    }
  }
}

type FetchModelsResult =
  | { ok: true; modelIds: string[]; defaultModelId?: string; modelGroups?: ChatState['composerModelGroups'] }
  | { ok: false; message: string }

function buildHarness(fetchModelsResult: FetchModelsResult): {
  actions: ReturnType<typeof createAppActions>
  state: ChatState
  refreshClawChannels: ReturnType<typeof vi.fn>
} {
  const refreshClawChannels = vi.fn(async () => undefined)
  let state = {
    activeThreadId: null,
    blocks: [],
    threads: [],
    composerModel: '',
    composerProviderId: '',
    composerPickList: mergeComposerPickList(false, []),
    composerModelGroups: [],
    refreshClawChannels,
    route: 'chat'
  } as unknown as ChatState
  let loadPromise: Promise<void> | null = null
  const set: ChatStoreSet = (partial) => {
    const update = typeof partial === 'function' ? partial(state) : partial
    Object.assign(state, update)
  }
  const get: ChatStoreGet = () => state

  vi.stubGlobal('window', {
    analytix: {
      runtime: {
        fetchUpstreamModels: vi.fn(async () => fetchModelsResult)
      },
      settings: {
        saveSettingsSilent: vi.fn(async () => state)
      }
    }
  })

  return {
    state,
    refreshClawChannels,
    actions: createAppActions({
      set,
      get,
      i18n: { t: (key: string) => key, changeLanguage: vi.fn(async () => undefined) } as unknown as typeof i18next,
      persistComposerModel,
      readStoredComposerModel,
      mergeComposerPickList,
      fallbackComposerModel,
      getComposerModelLoadPromise: () => loadPromise,
      setComposerModelLoadPromise: (promise) => {
        loadPromise = promise
      },
      applyTheme: () => undefined,
      applyUiFontScale: () => undefined,
      applyMotionPreference: () => undefined,
      applyCursorSpotlight: () => undefined,
      applyWriteTypography: () => undefined,
      applyDocumentLocale: () => undefined,
      workspaceLabelFromPath: (workspaceRoot) => workspaceRoot,
      normalizeWorkspaceRoot: (workspaceRoot) => workspaceRoot?.trim() ?? ''
    })
  }
}

describe('chat-store app actions composer model loading', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', createMemoryStorage())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it.each(['chat', 'write'] as const)('retains committed default model groups for a new %s composer', async (route) => {
    const modelGroups = [{
      providerId: 'deepseek',
      label: 'deepseek',
      modelIds: ['deepseek-v4-flash', 'deepseek-v4-pro']
    }]
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: modelGroups[0].modelIds,
      defaultModelId: 'deepseek-v4-pro',
      modelGroups
    })
    state.route = route

    await actions.loadComposerModels()

    expect(state.composerModelGroups).toEqual(modelGroups)
    expect(state.composerModel).toBe('deepseek-v4-pro')
    expect(state.composerProviderId).toBe('deepseek')
    expect(state.composerModelGroups[0].modelProfiles).toBeUndefined()
  })

  it('resolves an unbound shared model to the Registry selected group supplied first', async () => {
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['shared-model'],
      defaultModelId: 'shared-model',
      modelGroups: [
        { providerId: 'beta', label: 'beta', modelIds: ['shared-model'] },
        { providerId: 'alpha', label: 'alpha', modelIds: ['shared-model'] }
      ]
    })

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('shared-model')
    expect(state.composerProviderId).toBe('beta')
  })

  it.each(['current', 'stored', 'thread'] as const)('preserves an unavailable explicit %s Provider instead of switching to a namesake', async (source) => {
    const { actions, state } = buildHarness({
      ok: true, modelIds: ['shared-model'],
      modelGroups: [{ providerId: 'other', label: 'Other', modelIds: ['shared-model'] }]
    })
    if (source === 'current') {
      state.composerModel = 'shared-model'
      state.composerProviderId = 'missing'
    } else if (source === 'stored') {
      localStorage.setItem(COMPOSER_MODEL_STORAGE_KEY, 'shared-model')
      localStorage.setItem(COMPOSER_PROVIDER_STORAGE_KEY, 'missing')
    } else {
      state.activeThreadId = 'thread-a'
      state.threads = [{ id: 'thread-a', model: 'shared-model' } as ChatState['threads'][number]]
      localStorage.setItem(THREAD_COMPOSER_SELECTION_STORAGE_KEY,
        JSON.stringify({ 'thread-a': { model: 'shared-model', providerId: 'missing' } }))
    }
    await actions.loadComposerModels()
    expect(state.composerModel).toBe('shared-model')
    expect(state.composerProviderId).toBe('missing')
    expect(window.analytix.settings.saveSettingsSilent).not.toHaveBeenCalled()
  })

  it('keeps an explicit in-memory Provider when the catalog order changes', async () => {
    const { actions, state } = buildHarness({
      ok: true, modelIds: ['shared-model'], modelGroups: [
        { providerId: 'other', label: 'Other', modelIds: ['shared-model'] },
        { providerId: 'selected', label: 'Selected', modelIds: ['shared-model'] }
      ]
    })
    state.composerModel = 'shared-model'
    state.composerProviderId = 'selected'
    await actions.loadComposerModels()
    expect(state.composerProviderId).toBe('selected')
  })

  it('resolves an alias only within the explicitly selected Provider', () => {
    const { actions, state } = buildHarness({ ok: true, modelIds: [] })
    state.composerModelGroups = ['other', 'selected'].map((providerId) => ({
      providerId, label: providerId, modelIds: [providerId + '-canonical'],
      modelProfiles: { [providerId + '-canonical']: {
        inputModalities: ['text'], outputModalities: ['text'],
        messageParts: ['text'], supportsToolCalling: true, aliases: ['shared-alias']
      } }
    }))
    actions.setComposerModel('shared-alias', 'selected')
    expect(state.composerModel).toBe('selected-canonical')
    expect(state.composerProviderId).toBe('selected')
    expect(window.analytix.settings.saveSettingsSilent).toHaveBeenCalledWith({
      runtime: { model: 'selected-canonical', providerId: 'selected' }
    })
  })

  it('retains the selected Provider and model when Registry refresh fails', async () => {
    const { actions, state } = buildHarness({ ok: false, message: 'unavailable' })
    state.composerModel = 'my-custom-model'
    state.composerProviderId = 'local-profile'
    await actions.loadComposerModels()
    expect(state.composerModel).toBe('my-custom-model')
    expect(state.composerProviderId).toBe('local-profile')
  })

  it('restores the previously selected custom model after the full model list loads', async () => {
    localStorage.setItem(COMPOSER_MODEL_STORAGE_KEY, 'MiniMax-M2')
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['MiniMax-M2'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{
        providerId: 'minimax',
        label: 'MiniMax',
        modelIds: ['MiniMax-M2']
      }]
    })

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('MiniMax-M2')
    expect(state.composerProviderId).toBe('minimax')
    expect(localStorage.getItem(COMPOSER_MODEL_STORAGE_KEY)).toBe('MiniMax-M2')
    expect(localStorage.getItem(COMPOSER_PROVIDER_STORAGE_KEY)).toBe('minimax')
  })

  it('keeps a saved global model selection over the cold-start default', async () => {
    localStorage.setItem(COMPOSER_MODEL_STORAGE_KEY, 'deepseek-v4-flash')
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['deepseek-v4-pro', 'deepseek-v4-flash'],
      defaultModelId: DEFAULT_ANALYTIX_MODEL
    })
    state.composerModel = DEFAULT_ANALYTIX_MODEL

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('deepseek-v4-flash')
    expect(localStorage.getItem(COMPOSER_MODEL_STORAGE_KEY)).toBe('deepseek-v4-flash')
  })

  it('updates the composer provider when the picker supplies a provider id', () => {
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['MiniMax-M2'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{
        providerId: 'minimax',
        label: 'MiniMax',
        modelIds: ['MiniMax-M2']
      }]
    })
    state.composerModelGroups = [{
      providerId: 'minimax',
      label: 'MiniMax',
      modelIds: ['MiniMax-M2']
    }]

    actions.setComposerModel('MiniMax-M2', 'minimax')

    expect(state.composerModel).toBe('MiniMax-M2')
    expect(state.composerProviderId).toBe('minimax')
    expect(localStorage.getItem(COMPOSER_PROVIDER_STORAGE_KEY)).toBe('minimax')
    expect(window.analytix.settings.saveSettingsSilent).toHaveBeenCalledWith({
      runtime: { model: 'MiniMax-M2', providerId: 'minimax' }
    })
  })

  it('infers the Xiaomi provider when selecting a MiMo model without an explicit provider id', () => {
    const xiaomiGroup = {
      providerId: 'xiaomi-token-plan',
      label: 'Xiaomi Token Plan',
      modelIds: ['mimo-v2.5-pro', 'mimo-v2.5', 'mimo-v2-pro']
    }
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['mimo-v2.5-pro'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [xiaomiGroup]
    })
    state.composerModelGroups = [xiaomiGroup]

    actions.setComposerModel('mimo-v2.5-pro')

    expect(state.composerModel).toBe('mimo-v2.5-pro')
    expect(state.composerProviderId).toBe('xiaomi-token-plan')
    expect(localStorage.getItem(COMPOSER_PROVIDER_STORAGE_KEY)).toBe('xiaomi-token-plan')
    expect(window.analytix.settings.saveSettingsSilent).toHaveBeenCalledWith({
      runtime: { model: 'mimo-v2.5-pro', providerId: 'xiaomi-token-plan' }
    })
  })

  it('clears the persisted runtime provider when the selected model has no provider group', () => {
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['mimo-v2-pro'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: []
    })
    state.composerModelGroups = []

    actions.setComposerModel('mimo-v2-pro')

    expect(state.composerModel).toBe('mimo-v2-pro')
    expect(state.composerProviderId).toBe('')
    expect(localStorage.getItem(COMPOSER_PROVIDER_STORAGE_KEY)).toBe('')
    expect(window.analytix.settings.saveSettingsSilent).toHaveBeenCalledWith({
      runtime: { model: 'mimo-v2-pro', providerId: '' }
    })
  })

  it('canonicalizes deprecated MiMo model aliases before saving the runtime default', () => {
    const xiaomiGroup = {
      providerId: 'xiaomi-token-plan',
      label: 'Xiaomi Token Plan',
      modelIds: ['mimo-v2.5-pro'],
      modelProfiles: {
        'mimo-v2.5-pro': {
          inputModalities: ['text'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text'],
          aliases: ['mimo-v2.5-pro-ultraspeed']
        }
      }
    } satisfies ChatState['composerModelGroups'][number]
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['mimo-v2.5-pro'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [xiaomiGroup]
    })
    state.composerModelGroups = [xiaomiGroup]

    actions.setComposerModel('mimo-v2.5-pro-ultraspeed')

    expect(state.composerModel).toBe('mimo-v2.5-pro')
    expect(state.composerProviderId).toBe('xiaomi-token-plan')
    expect(localStorage.getItem(COMPOSER_MODEL_STORAGE_KEY)).toBe('mimo-v2.5-pro')
    expect(window.analytix.settings.saveSettingsSilent).toHaveBeenCalledWith({
      runtime: { model: 'mimo-v2.5-pro', providerId: 'xiaomi-token-plan' }
    })
  })

  it('keeps active-thread model changes out of the global Analytix default', () => {
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['MiniMax-M2'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{
        providerId: 'minimax',
        label: 'MiniMax',
        modelIds: ['MiniMax-M2']
      }]
    })
    state.activeThreadId = 'thread-a'
    state.threads = [{
      id: 'thread-a',
      title: 'Thread A',
      workspace: '/tmp/project',
      model: 'deepseek-v4-pro',
      status: 'idle',
      mode: 'agent',
      updatedAt: '2026-06-01T00:00:00.000Z'
    }]
    state.composerModelGroups = [{
      providerId: 'minimax',
      label: 'MiniMax',
      modelIds: ['MiniMax-M2']
    }]

    actions.setComposerModel('MiniMax-M2', 'minimax')

    expect(state.composerModel).toBe('MiniMax-M2')
    expect(state.composerProviderId).toBe('minimax')
    expect(localStorage.getItem(COMPOSER_MODEL_STORAGE_KEY)).toBeNull()
    expect(localStorage.getItem(COMPOSER_PROVIDER_STORAGE_KEY)).toBeNull()
    expect(JSON.parse(localStorage.getItem(THREAD_COMPOSER_SELECTION_STORAGE_KEY) ?? '{}')).toEqual({
      'thread-a': { model: 'MiniMax-M2', providerId: 'minimax' }
    })
    expect(window.analytix.settings.saveSettingsSilent).not.toHaveBeenCalled()
  })

  it('opens Connect Phone with a clean main-stage frame from another route', () => {
    const { actions, state, refreshClawChannels } = buildHarness({
      ok: true,
      modelIds: []
    })
    state.route = 'write' as AppRoute
    state.activeThreadId = 'write-thread'
    state.blocks = [{ kind: 'assistant', id: 'stale-write-message', text: 'stale write content' }]

    actions.openClaw()

    expect(state.route).toBe('claw')
    expect(state.activeThreadId).toBeNull()
    expect(state.blocks).toEqual([])
    expect(refreshClawChannels).toHaveBeenCalled()
  })

  it('does not clear the current Connect Phone thread when reopening the panel in-place', () => {
    const { actions, state, refreshClawChannels } = buildHarness({
      ok: true,
      modelIds: []
    })
    state.route = 'claw' as AppRoute
    state.activeThreadId = 'claw-thread'
    state.blocks = [{ kind: 'assistant', id: 'claw-message', text: 'existing phone thread' }]

    actions.openClaw()

    expect(state.route).toBe('claw')
    expect(state.activeThreadId).toBe('claw-thread')
    expect(state.blocks).toEqual([{ kind: 'assistant', id: 'claw-message', text: 'existing phone thread' }])
    expect(refreshClawChannels).toHaveBeenCalled()
  })

  it('restores a model selection from the active thread instead of the global picker', async () => {
    localStorage.setItem(COMPOSER_MODEL_STORAGE_KEY, 'deepseek-v4-flash')
    localStorage.setItem(
      THREAD_COMPOSER_SELECTION_STORAGE_KEY,
      JSON.stringify({ 'thread-a': { model: 'MiniMax-M2', providerId: 'minimax' } })
    )
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['MiniMax-M2'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{
        providerId: 'minimax',
        label: 'MiniMax',
        modelIds: ['MiniMax-M2']
      }]
    })
    state.activeThreadId = 'thread-a'
    state.threads = [{
      id: 'thread-a',
      title: 'Thread A',
      workspace: '/tmp/project',
      model: 'deepseek-v4-pro',
      status: 'idle',
      mode: 'agent',
      updatedAt: '2026-06-01T00:00:00.000Z'
    }]

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('MiniMax-M2')
    expect(state.composerProviderId).toBe('minimax')
    expect(localStorage.getItem(COMPOSER_MODEL_STORAGE_KEY)).toBe('deepseek-v4-flash')
  })

  it('canonicalizes a restored MiMo alias from the active thread selection', async () => {
    localStorage.setItem(
      THREAD_COMPOSER_SELECTION_STORAGE_KEY,
      JSON.stringify({ 'thread-a': { model: 'mimo-v2.5-pro-ultraspeed', providerId: 'xiaomi-token-plan' } })
    )
    const xiaomiGroup = {
      providerId: 'xiaomi-token-plan',
      label: 'Xiaomi Token Plan',
      modelIds: ['mimo-v2.5-pro'],
      modelProfiles: {
        'mimo-v2.5-pro': {
          inputModalities: ['text'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text'],
          aliases: ['mimo-v2.5-pro-ultraspeed']
        }
      }
    } satisfies ChatState['composerModelGroups'][number]
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['mimo-v2.5-pro'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [xiaomiGroup]
    })
    state.activeThreadId = 'thread-a'
    state.threads = [{
      id: 'thread-a',
      title: 'Thread A',
      workspace: '/tmp/project',
      model: 'deepseek-v4-pro',
      status: 'idle',
      mode: 'plan',
      updatedAt: '2026-06-01T00:00:00.000Z'
    }]

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('mimo-v2.5-pro')
    expect(state.composerProviderId).toBe('xiaomi-token-plan')
    expect(JSON.parse(localStorage.getItem(THREAD_COMPOSER_SELECTION_STORAGE_KEY) ?? '{}')).toEqual({
      'thread-a': { model: 'mimo-v2.5-pro', providerId: 'xiaomi-token-plan' }
    })
  })

  it('does not restore a per-thread selection filtered out of the composer menu', async () => {
    localStorage.setItem(
      THREAD_COMPOSER_SELECTION_STORAGE_KEY,
      JSON.stringify({ 'thread-a': { model: 'Kwai-Kolors/Kolors', providerId: 'minimax' } })
    )
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: ['Kwai-Kolors/Kolors'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{
        providerId: 'minimax',
        label: 'MiniMax',
        modelIds: ['Kwai-Kolors/Kolors'],
        modelProfiles: {
          'kwai-kolors/kolors': {
            inputModalities: ['text'],
            outputModalities: ['image'],
            supportsToolCalling: false,
            messageParts: ['text']
          }
        }
      }]
    })
    state.activeThreadId = 'thread-a'
    state.threads = [{
      id: 'thread-a',
      title: 'Thread A',
      workspace: '/tmp/project',
      model: 'deepseek-v4-pro',
      status: 'idle',
      mode: 'agent',
      updatedAt: '2026-06-01T00:00:00.000Z'
    }]

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('deepseek-v4-pro')
    expect(state.composerProviderId).toBe('')
  })

  it('does not overwrite a stored custom model when only fallback models are available', async () => {
    localStorage.setItem(COMPOSER_MODEL_STORAGE_KEY, 'MiniMax-M2')
    const { actions, state } = buildHarness({
      ok: false,
      message: 'upstream unavailable'
    })

    await actions.loadComposerModels()

    expect(state.composerModel).toBe('deepseek-v4-flash')
    expect(localStorage.getItem(COMPOSER_MODEL_STORAGE_KEY)).toBe('MiniMax-M2')
  })

  it('keeps Workflow/Create Loop out of the app route action surface', () => {
    const { actions, state } = buildHarness({
      ok: true,
      modelIds: [],
      defaultModelId: 'deepseek-v4-pro'
    })

    actions.openSchedule()

    expect(state.route).toBe('schedule')
    for (const action of forbiddenTopLevelAppActions) {
      expect(action in actions, `${action} must not be exposed as an app action`).toBe(false)
    }
    expectTypeOf<Extract<AppRoute, ForbiddenTopLevelRoute>>().toEqualTypeOf<never>()
  })
})
