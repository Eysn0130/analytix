import type i18next from 'i18next'
import { DEFAULT_ANALYTIX_MODEL, isComposerChatModelId, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import type { ChatState, ChatStoreGet, ChatStoreSet, InitialSetupMode, PluginHostRoute, SettingsRouteSection } from './chat-store-types'
import {
  canonicalComposerModelForSelection,
  composerModelSelectable,
  persistComposerProviderId,
  providerIdForComposerModel,
  providerIdMatchesComposerModel,
  readThreadComposerSelection,
  rememberThreadComposerSelection,
  readStoredComposerProviderId,
  readStoredComposerSelection
} from './chat-store-helpers'
import { clearedThreadSelection } from './chat-store-runtime-helpers'

type CreateAppActionsOptions = {
  set: ChatStoreSet
  get: ChatStoreGet
  i18n: typeof i18next
  persistComposerModel: (model: string) => void
  readStoredComposerModel: (allowedIds: readonly string[]) => string
  mergeComposerPickList: (upstreamOk: boolean, upstreamIds: string[]) => string[]
  fallbackComposerModel: (pickList: readonly string[], runtimeDefault: string) => string
  getComposerModelLoadPromise: () => Promise<void> | null
  setComposerModelLoadPromise: (promise: Promise<void> | null) => void
  applyTheme: (theme: AppSettingsV1['theme']) => void
  applyUiFontScale: (scale: AppSettingsV1['uiFontScale']) => void
  applyMotionPreference: (preference: AppSettingsV1['motionPreference']) => void
  applyCursorSpotlight: (enabled: boolean) => void
  applyWriteTypography: (typography: AppSettingsV1['write']['typography']) => void
  applyDocumentLocale: (locale: AppSettingsV1['locale']) => void
  workspaceLabelFromPath: (workspaceRoot: string) => string
  normalizeWorkspaceRoot: (workspaceRoot?: string | null) => string
}

let topNoticeSeq = 0

export function createAppActions(options: CreateAppActionsOptions): Pick<
  ChatState,
  | 'setError'
  | 'showTopNotice'
  | 'dismissTopNotice'
  | 'setComposerModel'
  | 'loadComposerModels'
  | 'setRoute'
  | 'openWrite'
  | 'openSettings'
  | 'openPlugins'
  | 'openClaw'
  | 'openSchedule'
  | 'openInitialSetup'
  | 'closeInitialSetup'
  | 'selectInspectorItem'
  | 'applyI18nFromSettings'
  | 'reloadUiSettings'
> {
  const {
    set,
    get,
    i18n,
    persistComposerModel,
    readStoredComposerModel,
    mergeComposerPickList,
    fallbackComposerModel,
    getComposerModelLoadPromise,
    setComposerModelLoadPromise,
    applyTheme,
    applyUiFontScale,
    applyMotionPreference,
    applyCursorSpotlight,
    applyWriteTypography,
    applyDocumentLocale,
    workspaceLabelFromPath,
    normalizeWorkspaceRoot
  } = options

  const showTopNotice: ChatState['showTopNotice'] = (notice) => {
    const message = notice.message.trim()
    if (!message) return
    topNoticeSeq += 1
    const prefix = notice.id?.trim() || 'top-notice'
    set({
      topNotice: {
        id: `${prefix}-${topNoticeSeq}`,
        tone: notice.tone ?? 'info',
        message,
        durationMs: notice.durationMs
      }
    })
  }

  const dismissTopNotice: ChatState['dismissTopNotice'] = (id) => {
    set((state) => {
      if (id && state.topNotice?.id !== id) return {}
      return { topNotice: null }
    })
  }

  return {
    setError: (message) => {
      set({ error: message })
      if (message) showTopNotice({ tone: 'error', message })
    },

    showTopNotice,

    dismissTopNotice,

    setComposerModel: (modelId, providerId) => {
      const groups = get().composerModelGroups
      const selectionGroups = providerId?.trim() ? groups.filter((group) => group.providerId === providerId.trim()) : groups
      const nextModelId = canonicalComposerModelForSelection(selectionGroups, modelId) || modelId.trim()
      const nextProviderId = providerId?.trim() || providerIdForComposerModel(get().composerModelGroups, nextModelId)
      const activeThreadId = get().activeThreadId
      if (activeThreadId) {
        rememberThreadComposerSelection(activeThreadId, nextModelId, nextProviderId)
      } else {
        persistComposerModel(nextModelId)
        persistComposerProviderId(nextProviderId)
      }
      set({ composerModel: nextModelId, composerProviderId: nextProviderId })
      const trimmed = nextModelId.trim()
      if (!activeThreadId && trimmed && trimmed.toLowerCase() !== 'auto' && typeof window.analytix !== 'undefined') {
        void window.analytix.settings.saveSettingsSilent({
          runtime: {
            model: trimmed,
            providerId: nextProviderId
          }
        })
      }
    },

    loadComposerModels: async () => {
      if (getComposerModelLoadPromise()) return getComposerModelLoadPromise()!
      if (typeof window.analytix === 'undefined') return
      const task = (async () => {
        const res = await window.analytix.runtime.fetchUpstreamModels()
        const pick = mergeComposerPickList(res.ok, res.ok ? res.modelIds : [])
        const groups = res.ok ? res.modelGroups ?? [] : []
        const runtimeDefault = res.ok ? res.defaultModelId?.trim() ?? '' : ''
        set((state) => {
          const isSelectable = (model: string): boolean => composerModelSelectable(pick, groups, model)
          const activeThread = state.activeThreadId
            ? state.threads.find((thread) => thread.id === state.activeThreadId) ?? null
            : null
          const threadSelection = activeThread ? readThreadComposerSelection(activeThread.id) : null
          const explicitSelection = activeThread
            ? threadSelection
            : state.composerProviderId.trim()
              ? { model: state.composerModel.trim(), providerId: state.composerProviderId.trim() }
              : readStoredComposerSelection()
          // A catalog refresh is not authorization to move an explicit selection
          // to another Provider. Keep the unavailable pair visible for correction.
          if (explicitSelection?.providerId && isComposerChatModelId(explicitSelection.model) &&
              (!res.ok || !providerIdMatchesComposerModel(groups, explicitSelection.providerId, explicitSelection.model))) {
            return {
              composerPickList: pick,
              composerModelGroups: groups,
              composerModel: explicitSelection.model,
              composerProviderId: explicitSelection.providerId
            }
          }
          const currentModel = state.composerModel.trim()
          const normalizedCurrentModel = currentModel.toLowerCase() === 'auto' ? '' : currentModel
          const storedModel = readStoredComposerModel(pick)
          const shouldUseStoredModel =
            !activeThread &&
            Boolean(storedModel) &&
            normalizedCurrentModel === DEFAULT_ANALYTIX_MODEL &&
            state.composerProviderId === ''
          let model = activeThread
            ? threadSelection?.model?.trim() || activeThread.model.trim()
            : shouldUseStoredModel
              ? storedModel
              : normalizedCurrentModel
          let shouldPersist = !activeThread && model !== state.composerModel
          if (model === '' || !isSelectable(model)) {
            model = activeThread ? '' : storedModel
            shouldPersist = false
          }
          if (model === '' || !isSelectable(model)) {
            model = fallbackComposerModel(pick, runtimeDefault)
            shouldPersist = false
          }
          const selectionGroups = explicitSelection?.providerId
            ? groups.filter((group) => group.providerId === explicitSelection.providerId)
            : groups
          const canonicalModel = canonicalComposerModelForSelection(selectionGroups, model)
          if (canonicalModel && canonicalModel !== model) {
            model = canonicalModel
            shouldPersist = !activeThread
          }
          const threadProviderId =
            threadSelection && providerIdMatchesComposerModel(groups, threadSelection.providerId, model)
              ? threadSelection.providerId
              : ''
          const storedProviderId = activeThread ? '' : readStoredComposerProviderId(groups, model)
          const currentProviderId = explicitSelection && providerIdMatchesComposerModel(groups, explicitSelection.providerId, model)
            ? explicitSelection.providerId : ''
          const providerId = currentProviderId || threadProviderId || storedProviderId || providerIdForComposerModel(groups, model)
          if (!activeThread && (shouldPersist || (res.ok && providerIdMatchesComposerModel(groups, providerId, model)))) {
            persistComposerModel(model)
          }
          if (!activeThread && providerId !== state.composerProviderId) persistComposerProviderId(providerId)
          if (
            activeThread &&
            (!threadSelection || threadSelection.model !== model || threadSelection.providerId !== providerId) &&
            composerModelSelectable(pick, groups, model)
          ) {
            rememberThreadComposerSelection(activeThread.id, model, providerId)
          }
          return {
            composerPickList: pick,
            composerModel: model,
            composerProviderId: providerId,
            composerModelGroups: groups
          }
        })
      })().finally(() => {
        setComposerModelLoadPromise(null)
      })
      setComposerModelLoadPromise(task)
      return task
    },

    setRoute: (route) => set({ route }),

    openWrite: async () => {
      set({ route: 'write' })
    },

    openSettings: (section: SettingsRouteSection = 'general') =>
      set((state) => ({
        route: 'settings',
        settingsSection: section,
        settingsReturnRoute: state.route === 'settings' ? state.settingsReturnRoute : state.route
      })),

    openPlugins: (host?: PluginHostRoute) =>
      set((state) => ({
        route: 'plugins',
        pluginHostRoute: host ?? (state.route === 'claw' ? 'claw' : 'chat')
      })),

    openClaw: () => {
      set((state) => ({
        ...(state.route === 'claw' ? {} : clearedThreadSelection()),
        route: 'claw'
      }))
      void get().refreshClawChannels()
    },

    openSchedule: () => {
      set({ route: 'schedule' })
    },

    openInitialSetup: (mode: InitialSetupMode = 'required') =>
      set({ initialSetupOpen: true, initialSetupMode: mode }),

    closeInitialSetup: () => set({ initialSetupOpen: false, initialSetupMode: 'required' }),

    selectInspectorItem: (id) => set({ inspectorSelectedId: id }),

    applyI18nFromSettings: async (locale) => {
      await i18n.changeLanguage(locale)
      applyDocumentLocale(locale)
    },

    reloadUiSettings: async () => {
      if (typeof window.analytix === 'undefined') return
      const settings = await rendererRuntimeClient.getSettings({ forceRefresh: true })
      const workspaceRoot = normalizeWorkspaceRoot(settings.workspaceRoot)
      applyTheme(settings.theme)
      applyUiFontScale(settings.uiFontScale)
      applyMotionPreference(settings.motionPreference)
      applyCursorSpotlight(settings.cursorSpotlight !== false)
      if (settings.write?.typography) applyWriteTypography(settings.write.typography)
      set({
        workspaceRoot,
        workspaceLabel: workspaceLabelFromPath(workspaceRoot),
        disabledSkillIds: settings.disabledSkillIds,
        clawChannels: settings.claw.channels,
        activeClawChannelId: settings.claw.channels.some(
          (channel) => channel.id === get().activeClawChannelId && channel.enabled
        )
          ? get().activeClawChannelId
          : settings.claw.channels.find((channel) => channel.enabled)?.id ?? ''
      })
      await get().applyI18nFromSettings(settings.locale)
      if (get().runtimeConnection === 'ready') {
        void get().refreshThreads()
      }
      void get().loadComposerModels()
    }
  }
}
