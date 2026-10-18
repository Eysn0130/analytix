import { ordinarySettingsPatch } from '@shared/schedule-task-mutation'
import type { ReactElement } from 'react'
import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DEFAULT_WRITE_INLINE_COMPLETION_BASE_URL,
  DEFAULT_WRITE_WORKSPACE_ROOT,
  DEFAULT_APP_MOTION_PREFERENCE,
  DEFAULT_APP_ACCENT_COLOR,
  type AnalytixRuntimeSettingsPatchV1,
  type AppSettingsPatch,
  getAnalytixRuntimeSettings,
  getModelProviderSettings,
  isAnalytixRuntimeInsecure,
  resolveWriteInlineCompletionBaseUrl,
  resolveWriteInlineCompletionModel,
  type AppSettingsV1,
} from '@shared/app-settings'
import { rendererRuntimeClient } from '../agent/runtime-client'
import { getProvider } from '../agent/registry'
import type {
  CoreMemoryDiagnosticsJson,
  CoreMemoryRecordJson,
  CoreRuntimeInfoJson,
  CoreRuntimeToolDiagnosticsJson
} from '../agent/analytix-contract'
import type { WriteInlineCompletionDebugEntry } from '@shared/write-inline-completion'
import { applyCursorSpotlight, applyMotionPreference, applyTheme, applyUiFontScale, applyWriteTypography } from '../lib/apply-theme'
import { desktopQueryCache } from '../lib/desktop-query-cache'
import { formatWorkspacePickerError } from '../lib/format-workspace-picker-error'
import type { SkillRootListItem } from '@shared/analytix-api'
import { normalizeWorkspaceRoot } from '../lib/workspace-path'
import { useChatStore, type SettingsRouteSection } from '../store/chat-store'
import { AnalytixLoadingPage } from './brand/AnalytixLoadingPage'
import { SettingsSidebar, settingsTabForCategory } from './SettingsSidebar'
import { adoptPerformanceUsageHostReadback } from './chat/presentation/chat/TurnUsagePanel'
import { Dialog } from '../data-analysis/components/overlay/Dialog'
import type { ProviderSettingsActions } from './settings-section-providers'
import './settings-modal.css'
import { WriteDebugLogModal } from './settings-debug-log'
import { useSettingsGuiUpdate } from './use-settings-gui-update'
import {
  DEFAULT_WORKSPACE_ROOT,
  coerceRendererSettings,
  hasValidPort,
  listSettingsText,
  mergeSettings,
  splitSettingsList
} from './settings-utils'
import { loadAnalytixDiagnostics } from '../lib/load-analytix-diagnostics'
import { SETTINGS_CHANGED_EVENT, emitRendererSettingsChanged } from '../lib/keyboard-shortcut-settings'
import {
  AgentsSettingsSection,
  ArchivedThreadsSettingsSection,
  BrowserSettingsSection,
  ClawSettingsSection,
  ComputerUseSettingsSection,
  GeneralSettingsSection,
  ImageGenerationSettingsSection,
  KeyboardShortcutsSettingsSection,
  WorktreeSettingsSection,
  MemorySettingsSection,
  PermissionsSettingsSection,
  ProvidersSettingsSection,
  SpeechToTextSettingsSection,
  UpdatesSettingsSection,
  WriteSettingsSection
} from './settings-sections'

type SettingsCategory = 'account' | 'general' | 'providers' | 'write' | 'imageGeneration' | 'speechToText' | 'agents' | 'archives' | 'permissions' | 'browser' | 'computerUse' | 'worktree' | 'memory' | 'shortcuts' | 'claw' | 'updates'
function categoryForSettingsSection(section: SettingsRouteSection): SettingsCategory {
  if (section === 'skill' || section === 'mcp') return 'agents'
  if (section === 'debug' || section === 'mediaGeneration') return 'general'
  return section
}

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error'
type SettingsPatch = AppSettingsPatch
type MemoryProjectionOwner = { workspace: string; workspaceEpoch: number; entryEpoch: number }
type InlineNotice = {
  tone: 'success' | 'error' | 'info'
  message: string
}

const SETTINGS_SKILL_ROOTS_STALE_MS = 60_000
const RUNTIME_CONFIG_STALE_MS = 60_000

// Settings may refresh while a modal draft is open. Submit only the user's
// changes against its starting snapshot, never stale fields echoed by a read.
function ordinaryDraftPatch(baseline: AppSettingsV1, draft: AppSettingsV1): AppSettingsPatch {
  const difference = (before: unknown, after: unknown): unknown => {
    if (JSON.stringify(before) === JSON.stringify(after)) return undefined
    if (before && after && typeof before === 'object' && typeof after === 'object' &&
        !Array.isArray(before) && !Array.isArray(after)) {
      const patch: Record<string, unknown> = {}
      for (const [key, value] of Object.entries(after)) {
        const changed = difference((before as Record<string, unknown>)[key], value)
        if (changed !== undefined) patch[key] = changed
      }
      return Object.keys(patch).length ? patch : undefined
    }
    return after
  }
  const patch = (difference(ordinarySettingsPatch(baseline), ordinarySettingsPatch(draft)) ?? {}) as AppSettingsPatch
  // Model preferences have their own explicit Provider facade operation.
  if (patch.provider) {
    const { modelPreferences: _preferences, ...providerPatch } = patch.provider
    void _preferences
    if (Object.keys(providerPatch).length) patch.provider = providerPatch
    else delete patch.provider
  }
  return patch
}

const DeprecatedHubCompatibilitySection = lazy(async () => {
  const module = await import('./settings-section-account')
  return { default: module.AccountProfileSettingsSection }
})

export function SettingsView(): ReactElement {
  const { t } = useTranslation('settings')
  const { t: tCommon } = useTranslation('common')
  const setRoute = useChatStore((s) => s.setRoute)
  const settingsReturnRoute = useChatStore((s) => s.settingsReturnRoute)
  const settingsSection = useChatStore((s) => s.settingsSection)
  const openCode = useChatStore((s) => s.openCode)
  const openPlugins = useChatStore((s) => s.openPlugins)
  const applyI18n = useChatStore((s) => s.applyI18nFromSettings)
  const reloadUiSettings = useChatStore((s) => s.reloadUiSettings)
  const showTopNotice = useChatStore((s) => s.showTopNotice)
  const probeRuntime = useChatStore((s) => s.probeRuntime)
  const runtimeConnection = useChatStore((s) => s.runtimeConnection)
  const selectThread = useChatStore((s) => s.selectThread)
  const archiveThread = useChatStore((s) => s.archiveThread)
  const deleteThread = useChatStore((s) => s.deleteThread)
  const [category, setCategory] = useState<SettingsCategory>(() => categoryForSettingsSection(settingsSection))
  const previousSettingsSection = useRef<SettingsRouteSection>(settingsSection)
  const providerDraftDirty = useRef(false)
  const [providerDirty, setProviderDirty] = useState(false)
  const providerActionsRef = useRef<ProviderSettingsActions | null>(null)
  const [providerActions, setProviderActions] = useState<ProviderSettingsActions | null>(null)
  const onProviderActionsChange = useCallback((actions: ProviderSettingsActions | null): void => {
    providerActionsRef.current = actions
    setProviderActions(actions)
  }, [])
  const onProviderDraftDirtyChange = useCallback((dirty: boolean): void => {
    providerDraftDirty.current = dirty
    setProviderDirty(dirty)
  }, [])
  const acceptSettingsSection = useCallback((section: SettingsRouteSection): boolean => {
    const selected = categoryForSettingsSection(section)
    if (category === 'providers' && selected !== category && providerDraftDirty.current) {
      showTopNotice({ tone: 'info', message: t('modelProviderUnsavedChanges') })
      useChatStore.getState().openSettings(previousSettingsSection.current)
      return false
    }
    previousSettingsSection.current = section
    setCategory(selected)
    if (useChatStore.getState().settingsSection !== section) useChatStore.getState().openSettings(section)
    return true
  }, [category, showTopNotice, t])
  const requestCategoryChange: typeof setCategory = (next) => {
    acceptSettingsSection(typeof next === 'function' ? next(category) : next)
  }
  const [form, setForm] = useState<AppSettingsV1 | null>(null)
  const settingsContentRef = useRef<HTMLDivElement | null>(null)
  const settingsLoaded = form !== null
  useLayoutEffect(() => {
    if (settingsContentRef.current) settingsContentRef.current.scrollTop = 0
  }, [category, settingsLoaded])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [workspacePickerError, setWorkspacePickerError] = useState<string | null>(null)
  const [writeWorkspacePickerError, setWriteWorkspacePickerError] = useState<string | null>(null)
  const [clawWorkspacePickerError, setClawWorkspacePickerError] = useState<string | null>(null)
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saveActionPending, setSaveActionPending] = useState(false)
  const saveActionInFlight = useRef(false)
  const settingsWritesPending = useRef(0)
  const [confirmDiscard, setConfirmDiscard] = useState(false)
  const keepEditingRef = useRef<HTMLButtonElement | null>(null)
  const discardPromptOpener = useRef<HTMLElement | null>(null)
  useLayoutEffect(() => {
    if (confirmDiscard) keepEditingRef.current?.focus()
    else if (discardPromptOpener.current?.isConnected) {
      discardPromptOpener.current.focus({ preventScroll: true })
      discardPromptOpener.current = null
    }
  }, [confirmDiscard])
  const [showRuntimeToken, setShowRuntimeToken] = useState(false)
  const [logPath, setLogPath] = useState('')
  const [logDirOpenError, setLogDirOpenError] = useState<string | null>(null)
  const [skillRoots, setSkillRoots] = useState<SkillRootListItem[]>([])
  const [skillRootsLoading, setSkillRootsLoading] = useState(false)
  const [skillNotice, setSkillNoticeState] = useState<InlineNotice | null>(null)
  const [mcpConfigPath, setMcpConfigPath] = useState('~/.analytix/mcp.json')
  const [mcpConfigText, setMcpConfigText] = useState('')
  const [mcpConfigExists, setMcpConfigExists] = useState(false)
  const [mcpLoading, setMcpLoading] = useState(false)
  const [mcpLoaded, setMcpLoaded] = useState(false)
  const [mcpBusy, setMcpBusy] = useState(false)
  const [mcpNotice, setMcpNoticeState] = useState<InlineNotice | null>(null)
  const [runtimeInfo, setRuntimeInfo] = useState<CoreRuntimeInfoJson | null>(null)
  const [toolDiagnostics, setToolDiagnostics] = useState<CoreRuntimeToolDiagnosticsJson | null>(null)
  const [memoryRecords, setMemoryRecords] = useState<CoreMemoryRecordJson[]>([])
  const [memoryDiagnostics, setMemoryDiagnostics] = useState<CoreMemoryDiagnosticsJson | null>(null)
  const [runtimeDiagnosticsBusy, setRuntimeDiagnosticsBusy] = useState(false)
  const [runtimeDiagnosticsNotice, setRuntimeDiagnosticsNoticeState] = useState<InlineNotice | null>(null)
  const [writeDebugModalOpen, setWriteDebugModalOpen] = useState(false)
  const [writeCompletionDebugEntries, setWriteCompletionDebugEntries] = useState<WriteInlineCompletionDebugEntry[]>([])
  const [writeCompletionDebugSelectedId, setWriteCompletionDebugSelectedId] = useState<string | null>(null)
  const [writeDebugLoading, setWriteDebugLoading] = useState(false)
  const [writeDebugError, setWriteDebugError] = useState<string | null>(null)
  const statusTimer = useRef<ReturnType<Window['setTimeout']> | null>(null)
  const draftVersion = useRef(0)
  const acceptedDraftVersion = useRef(0)
  const runtimeRejectedDraftVersion = useRef<number | null>(null)
  const settingsHostReadbackVersion = useRef(0)
  const settingsSaveQueue = useRef<Promise<unknown>>(Promise.resolve())
  const settingsWriteRequest = useRef(0)
  const settingsReadRequest = useRef(0)
  const pendingSettingsWrite = useRef<{ request: number; version: number } | null>(null)
  const profileDeletionIntents = useRef(new Map<string, number>())
  const profileIntentRevision = useRef(0)
  const settingsEntryActive = useRef(true)
  const latestForm = useRef<AppSettingsV1 | null>(null)
  const acceptedSettings = useRef<AppSettingsV1 | null>(null)
  const settingsFormBaseline = useRef<AppSettingsV1 | null>(null)
  const lastSubmittedDraft = useRef<{
    version: number; form: AppSettingsV1; baseline: AppSettingsV1; accepted: AppSettingsV1
  } | null>(null)
  const hostRecoveryId = useRef(0)
  const hostRecoveryPending = useRef(false)
  const [recoveringHostSettings, setRecoveringHostSettings] = useState(false)
  const memoryWorkspaceEpoch = useRef(0)
  const memoryEntryEpoch = useRef(0)
  const memoryListRequest = useRef(0)
  const modelPreferencesReadback = useRef<AppSettingsV1['provider']['modelPreferences']>(undefined)
  const applyForm = useCallback((next: AppSettingsV1): void => {
    if (normalizeWorkspaceRoot(next.workspaceRoot) !== normalizeWorkspaceRoot(latestForm.current?.workspaceRoot)) {
      memoryWorkspaceEpoch.current += 1
      memoryListRequest.current += 1
      setMemoryRecords([])
    }
    latestForm.current = next
    setForm(next)
  }, [])
  const agentsSectionRef = useRef<HTMLDivElement | null>(null)
  const skillSectionRef = useRef<HTMLDivElement | null>(null)
  const mcpSectionRef = useRef<HTMLDivElement | null>(null)
  const permissionsSectionRef = useRef<HTMLDivElement | null>(null)
  const logsSectionRef = useRef<HTMLDivElement | null>(null)
  const setSkillNotice = useCallback((notice: InlineNotice | null): void => {
    setSkillNoticeState(notice)
    if (notice) showTopNotice({ tone: notice.tone, message: notice.message })
  }, [showTopNotice])
  const setMcpNotice = useCallback((notice: InlineNotice | null): void => {
    setMcpNoticeState(notice)
    if (notice) showTopNotice({ tone: notice.tone, message: notice.message })
  }, [showTopNotice])
  const setRuntimeDiagnosticsNotice = useCallback((notice: InlineNotice | null): void => {
    setRuntimeDiagnosticsNoticeState(notice)
    if (notice) showTopNotice({ tone: notice.tone, message: notice.message })
  }, [showTopNotice])
  const formTheme = form?.theme
  const formAccentColor = form?.accentColor
  const formUiFontScale = form?.uiFontScale
  const formMotionPreference = form?.motionPreference
  const formCursorSpotlight = form?.cursorSpotlight
  const writeTypography = form?.write?.typography
  const formWorkspaceRoot = form?.workspaceRoot
  useLayoutEffect(() => {
    memoryEntryEpoch.current += 1
    return () => { memoryEntryEpoch.current += 1; memoryListRequest.current += 1 }
  }, [category])
  const captureMemoryOwner = useCallback((): MemoryProjectionOwner => ({
    workspace: normalizeWorkspaceRoot(formWorkspaceRoot),
    workspaceEpoch: memoryWorkspaceEpoch.current,
    entryEpoch: memoryEntryEpoch.current
  }), [formWorkspaceRoot])
  const isCurrentMemoryOwner = useCallback((owner: MemoryProjectionOwner): boolean =>
    settingsEntryActive.current && owner.workspaceEpoch === memoryWorkspaceEpoch.current &&
    owner.entryEpoch === memoryEntryEpoch.current &&
    owner.workspace === normalizeWorkspaceRoot(latestForm.current?.workspaceRoot), [])
  const formAnalytix = form ? getAnalytixRuntimeSettings(form) : null
  const formPort = formAnalytix?.port
  const formGuiUpdateChannel = form?.guiUpdate?.channel
  const {
    checkingGuiUpdate,
    checkGuiUpdate,
    downloadingGuiUpdate,
    downloadGuiUpdate,
    guiUpdateDownloaded,
    guiUpdateError,
    guiUpdateInfo,
    guiUpdateProgress,
    installingGuiUpdate,
    installGuiUpdate,
    resetGuiUpdateState
  } = useSettingsGuiUpdate({
    category,
    channel: formGuiUpdateChannel,
    form,
    t
  })

  useEffect(() => {
    if (saveStatus !== 'error' || !saveError) return
    showTopNotice({ tone: 'error', message: saveError })
  }, [saveError, saveStatus, showTopNotice])

  const readSettingsForm = useCallback((uncached = false): void => {
    const request = ++settingsReadRequest.current
    const loadVersion = draftVersion.current
    const loadWrite = settingsWriteRequest.current
    const loadHostVersion = settingsHostReadbackVersion.current
    const loadForm = latestForm.current
    const isCurrent = (): boolean => settingsEntryActive.current && request === settingsReadRequest.current &&
      loadVersion === draftVersion.current && loadWrite === settingsWriteRequest.current &&
      loadHostVersion === settingsHostReadbackVersion.current && latestForm.current === loadForm
    setLoadError(null)
    if (typeof window.analytix === 'undefined') {
      setLoadError('PRELOAD_BRIDGE')
      return
    }
    void (uncached ? rendererRuntimeClient.readSettingsFromHost()
      : rendererRuntimeClient.getSettings({ forceRefresh: true })).then(s => {
      if (!isCurrent()) return
      const next = coerceRendererSettings(s)
      acceptedSettings.current = next
      settingsFormBaseline.current = next
      acceptedDraftVersion.current = loadVersion
      modelPreferencesReadback.current = next.provider.modelPreferences
      applyForm(next)
      adoptPerformanceUsageHostReadback(next)
    }).catch(() => { if (isCurrent()) setLoadError('SETTINGS_UNAVAILABLE') })
  }, [applyForm])

  useEffect(() => {
    readSettingsForm()
    return () => { settingsReadRequest.current += 1 }
  }, [readSettingsForm])

  useEffect(() => {
    if (!formTheme || !formUiFontScale) return
    applyTheme(formTheme, formAccentColor)
    applyUiFontScale(formUiFontScale)
    applyMotionPreference(formMotionPreference)
    applyCursorSpotlight(formCursorSpotlight !== false)
  }, [formTheme, formAccentColor, formUiFontScale, formMotionPreference, formCursorSpotlight])

  // Live-preview the Write editor typography as the form changes, mirroring the
  // theme/scale preview above. Keyed on the scalar fields so it only re-applies
  // on real changes.
  useEffect(() => {
    if (writeTypography) applyWriteTypography(writeTypography)
  }, [
    writeTypography,
    writeTypography?.fontPreset,
    writeTypography?.customFontFamily,
    writeTypography?.fontSizePx,
    writeTypography?.lineHeight
  ])

  useEffect(() => {
    const subscribe = window.analytix?.runtime?.onRuntimeStatus
    if (typeof subscribe !== 'function') return
    return subscribe(status => {
      if (status.source !== 'settings-apply-rollback') return
      if (!status.rolledBack && status.code !== 'settings_apply_failed') return
      const version = draftVersion.current
      const write = settingsWriteRequest.current
      const visibleForm = latestForm.current
      const pending = pendingSettingsWrite.current
      if (!visibleForm || (acceptedDraftVersion.current !== version &&
          !(pending?.request === write && pending.version === version))) return
      runtimeRejectedDraftVersion.current = version
      const hostVersion = ++settingsHostReadbackVersion.current
      if (statusTimer.current) window.clearTimeout(statusTimer.current)
      statusTimer.current = null
      setSaveStatus('error')
      setSaveError(t(status.rolledBack && status.code !== 'settings_rollback_failed' ? 'settingsRuntimeRolledBack' : 'settingsRuntimeRollbackFailed'))
      const submitted = lastSubmittedDraft.current?.version === version ? lastSubmittedDraft.current : null
      const failedForm = submitted?.form ?? visibleForm
      if (submitted) {
        // Keep the original field ownership: Host refreshes were never local
        // edits, even if the committed readback briefly appeared before rollback.
        settingsFormBaseline.current = submitted.baseline
        applyForm(failedForm)
      }
      if (!status.rolledBack) return
      if (submitted) acceptedSettings.current = submitted.accepted
      const recovery = ++hostRecoveryId.current
      hostRecoveryPending.current = true
      setRecoveringHostSettings(true)
      void rendererRuntimeClient.readSettingsFromHost().then(s => {
        if (!settingsEntryActive.current || version !== draftVersion.current || write !== settingsWriteRequest.current ||
            hostVersion !== settingsHostReadbackVersion.current || latestForm.current !== failedForm) return
        const accepted = coerceRendererSettings(s)
        rendererRuntimeClient.invalidateSettings()
        modelPreferencesReadback.current = accepted.provider.modelPreferences
        acceptedSettings.current = accepted
        adoptPerformanceUsageHostReadback(accepted)
      }).catch(() => undefined).finally(() => {
        if (recovery !== hostRecoveryId.current) return
        hostRecoveryPending.current = false
        if (settingsEntryActive.current) setRecoveringHostSettings(false)
      })
    })
  }, [applyForm, t])

  useEffect(() => {
    const onSettingsChanged = (event: Event): void => {
      const next = (event as CustomEvent<AppSettingsV1>).detail
      if (next) {
        settingsHostReadbackVersion.current += 1
        const readback = coerceRendererSettings(next)
        acceptedSettings.current = readback
        modelPreferencesReadback.current = readback.provider.modelPreferences
        // Host refreshes must not replace a newer local draft waiting for its own save.
        if (acceptedDraftVersion.current === draftVersion.current && runtimeRejectedDraftVersion.current !== draftVersion.current) {
          settingsFormBaseline.current = readback
          applyForm(readback)
        }
      }
    }
    window.addEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)
    return () => window.removeEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)
  }, [applyForm])

  useEffect(() => {
    if (typeof window.analytix?.logs?.getPath !== 'function') return
    void window.analytix.logs.getPath().then((p) => setLogPath(p)).catch(() => undefined)
  }, [category])

  const loadWriteDebugEntries = useCallback(async (): Promise<void> => {
    setWriteDebugLoading(true)
    setWriteDebugError(null)
    try {
      const completionEntries = typeof window.analytix?.write?.listWriteInlineCompletionDebugEntries === 'function'
        ? await window.analytix.write.listWriteInlineCompletionDebugEntries()
        : []
      setWriteCompletionDebugEntries(completionEntries)
      setWriteCompletionDebugSelectedId((current) =>
        current && completionEntries.some((entry) => entry.id === current)
          ? current
          : completionEntries[0]?.id ?? null
      )
    } catch (error) {
      setWriteDebugError(error instanceof Error ? error.message : String(error))
    } finally {
      setWriteDebugLoading(false)
    }
  }, [])

  useEffect(() => {
    if (category !== 'write') return
    void loadWriteDebugEntries()
  }, [category, loadWriteDebugEntries])

  useEffect(() => {
    if (previousSettingsSection.current !== settingsSection) acceptSettingsSection(settingsSection)
  }, [acceptSettingsSection, settingsSection])

  useEffect(() => {
    if (!settingsLoaded) return
    if (settingsSection === 'debug') {
      if (category !== 'general') return
      const target = logsSectionRef.current
      if (!target) return
      const disclosure = target.closest('details')
      if (disclosure) disclosure.open = true
      const frame = window.requestAnimationFrame(() => {
        target.scrollIntoView({ behavior: 'auto', block: 'start' })
      })
      return () => window.cancelAnimationFrame(frame)
    }
    if (
      settingsSection === 'account' ||
      settingsSection === 'general' ||
      settingsSection === 'providers' ||
      settingsSection === 'write' ||
      settingsSection === 'imageGeneration' ||
      settingsSection === 'mediaGeneration' ||
      settingsSection === 'speechToText' ||
      settingsSection === 'archives' ||
      settingsSection === 'permissions' ||
      settingsSection === 'browser' ||
      settingsSection === 'computerUse' ||
      settingsSection === 'worktree' ||
      settingsSection === 'memory' ||
      settingsSection === 'claw' ||
      settingsSection === 'shortcuts' ||
      settingsSection === 'updates' ||
      category !== 'agents'
    ) {
      return
    }
    const refs: Record<
      Extract<SettingsRouteSection, 'agents' | 'skill' | 'mcp'>,
      HTMLDivElement | null
    > = {
      agents: agentsSectionRef.current,
      skill: skillSectionRef.current,
      mcp: mcpSectionRef.current
    }
    const target = refs[settingsSection]
    if (!target) return
    const frame = window.requestAnimationFrame(() => {
      target.scrollIntoView({ behavior: 'auto', block: 'start' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [category, settingsLoaded, settingsSection])

  useEffect(() => {
    settingsEntryActive.current = true
    return () => {
      settingsEntryActive.current = false
      draftVersion.current += 1
      if (statusTimer.current) window.clearTimeout(statusTimer.current)
    }
  }, [])

  const portError = useMemo(() => {
    if (!form || typeof formPort !== 'number') return null
    if (!hasValidPort(form)) return t('portInvalid')
    return null
  }, [form, formPort, t])

  const refreshSkillRoots = useCallback(async (): Promise<void> => {
    const listSkillRoots = window.analytix?.app?.listSkillRoots
    if (typeof listSkillRoots !== 'function') return
    setSkillRootsLoading(true)
    try {
      const workspaceRoot = normalizeWorkspaceRoot(formWorkspaceRoot)
      const result = await desktopQueryCache.fetchQuery({
        queryKey: ['skills', 'roots', workspaceRoot],
        staleTimeMs: SETTINGS_SKILL_ROOTS_STALE_MS,
        fetcher: () => listSkillRoots(workspaceRoot || undefined)
      })
      if (result.ok) setSkillRoots(result.roots)
    } catch {
      /* listing skill roots is best-effort; keep the last known list */
    } finally {
      setSkillRootsLoading(false)
    }
  }, [formWorkspaceRoot])

  useEffect(() => {
    if (category !== 'agents') return
    void refreshSkillRoots()
  }, [category, refreshSkillRoots])

  const loadMcpConfig = useCallback(async (): Promise<void> => {
    const getAnalytixConfigFile = window.analytix?.runtime?.getAnalytixConfigFile
    if (typeof getAnalytixConfigFile !== 'function') return
    setMcpLoading(true)
    setMcpNotice(null)
    try {
      const config = await desktopQueryCache.fetchQuery({
        queryKey: ['runtimeConfig', 'mcp'],
        staleTimeMs: RUNTIME_CONFIG_STALE_MS,
        fetcher: () => getAnalytixConfigFile()
      })
      setMcpConfigPath(config.path)
      setMcpConfigText(config.content)
      setMcpConfigExists(config.exists)
      setMcpLoaded(true)
    } catch (e) {
      setMcpNotice({
        tone: 'error',
        message: e instanceof Error ? e.message : String(e)
      })
    } finally {
      setMcpLoading(false)
    }
  }, [setMcpNotice])

  useEffect(() => {
    if (category !== 'agents' || mcpLoaded || mcpLoading) return
    void loadMcpConfig()
  }, [category, loadMcpConfig, mcpLoaded, mcpLoading])

  const openSkillRoot = async (path: string): Promise<void> => {
    if (!path) {
      setSkillNotice({ tone: 'error', message: t('skillsRootUnavailable') })
      return
    }
    if (typeof window.analytix?.app?.openSkillRoot !== 'function') return
    setSkillNotice(null)
    const result = await window.analytix.app.openSkillRoot(path)
    if (!result.ok) {
      setSkillNotice({ tone: 'error', message: result.message ?? t('applyFailed') })
    }
  }

  const toggleSkillRoot = (root: SkillRootListItem, enabled: boolean): void => {
    const current = form?.claw.skills.disabledDirs ?? []
    const keys = new Set([root.disableKey, root.id])
    const nextDisabled = enabled
      ? current.filter((entry) => !keys.has(entry))
      : [...new Set([...current, root.disableKey])]
    update({ claw: { skills: { disabledDirs: nextDisabled } } })
    // Optimistically reflect the toggle so the row responds before the
    // debounced save round-trips; skill counts are unaffected by toggling.
    setSkillRoots((roots) =>
      roots.map((item) =>
        item.id === root.id && item.path === root.path ? { ...item, enabled } : item
      )
    )
  }

  const saveMcpConfig = async (): Promise<void> => {
    if (typeof window.analytix?.runtime?.setAnalytixConfigFile !== 'function') return
    setMcpBusy(true)
    setMcpNotice(null)
    try {
      const result = await window.analytix.runtime.setAnalytixConfigFile(mcpConfigText)
      desktopQueryCache.invalidateQueries({ queryKey: ['runtimeConfig'], broadcast: true })
      desktopQueryCache.invalidateQueries({ queryKey: ['plugins'], broadcast: true })
      desktopQueryCache.invalidateQueries({ queryKey: ['skills'], broadcast: true })
      setMcpConfigPath(result.path)
      setMcpConfigExists(true)
      setMcpNotice({
        tone: 'success',
        message: t('mcpSaved', { path: result.path })
      })
    } catch (e) {
      setMcpNotice({
        tone: 'error',
        message: e instanceof Error ? e.message : String(e)
      })
    } finally {
      setMcpBusy(false)
    }
  }

  const openMcpConfigDir = async (): Promise<void> => {
    if (typeof window.analytix?.runtime?.openAnalytixConfigDir !== 'function') return
    const result = await window.analytix.runtime.openAnalytixConfigDir()
    if (!result.ok) {
      setMcpNotice({ tone: 'error', message: result.message ?? t('applyFailed') })
    }
  }

  const refreshAnalytixDiagnostics = useCallback(async (): Promise<void> => {
    const provider = getProvider()
    const owner = captureMemoryOwner()
    const request = ++memoryListRequest.current
    setRuntimeDiagnosticsBusy(true)
    setRuntimeDiagnosticsNotice(null)
    try {
      const loaded = await loadAnalytixDiagnostics(provider, {
        workspace: owner.workspace
      })
      setRuntimeInfo(loaded.runtimeInfo)
      setToolDiagnostics(loaded.toolDiagnostics)
      const memoryCurrent = isCurrentMemoryOwner(owner) && request === memoryListRequest.current
      if (memoryCurrent) setMemoryRecords(loaded.memoryRecords)
      const errors = memoryCurrent ? loaded.errors : loaded.errors.filter((error) => !error.startsWith('Memory:'))
      if (errors.length > 0) {
        setRuntimeDiagnosticsNotice({
          tone: 'error',
          message: errors.join(' | ')
        })
      }
    } catch (error) {
      setRuntimeInfo(null)
      setToolDiagnostics(null)
      if (isCurrentMemoryOwner(owner) && request === memoryListRequest.current) setMemoryRecords([])
      setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
    } finally {
      setRuntimeDiagnosticsBusy(false)
    }
  }, [captureMemoryOwner, isCurrentMemoryOwner, setRuntimeDiagnosticsNotice])

  useEffect(() => {
    if (category !== 'agents' && category !== 'browser' && category !== 'memory') return
    void refreshAnalytixDiagnostics()
  }, [category, refreshAnalytixDiagnostics])

  const refreshMemoryDiagnostics = async (): Promise<void> => {
    const provider = getProvider()
    if (typeof provider.getMemoryDiagnostics !== 'function') return
    try {
      const diagnostics = await provider.getMemoryDiagnostics()
      setMemoryDiagnostics(diagnostics)
    } catch {
      // best-effort; surfaced via runtimeDiagnosticsNotice elsewhere
    }
  }

  useEffect(() => {
    if (category !== 'memory') return
    void refreshMemoryDiagnostics()
  }, [category, memoryRecords])

  const createMemoryRecord = async (input: {
    content: string
    scope?: 'user' | 'workspace' | 'project'
    tags?: string[]
    confidence?: number
  }): Promise<boolean> => {
    const provider = getProvider()
    if (typeof provider.createMemory !== 'function') return false
    const owner = captureMemoryOwner()
    if (!isCurrentMemoryOwner(owner)) return false
    try {
      const memory = await provider.createMemory({ ...input, workspace: owner.workspace })
      if (isCurrentMemoryOwner(owner)) {
        memoryListRequest.current += 1
        setMemoryRecords((records) => [memory, ...records])
      }
      return true
    } catch (error) {
      if (isCurrentMemoryOwner(owner)) setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
      return false
    }
  }

  const updateMemoryRecord = async (
    memoryId: string,
    patch: { content?: string; tags?: string[]; confidence?: number; disabled?: boolean }
  ): Promise<boolean> => {
    const provider = getProvider()
    if (typeof provider.updateMemory !== 'function') return false
    const owner = captureMemoryOwner()
    try {
      const memory = await provider.updateMemory(memoryId, patch)
      if (isCurrentMemoryOwner(owner)) {
        memoryListRequest.current += 1
        setMemoryRecords((records) => records.map((record) => (record.id === memoryId ? memory : record)))
      }
      return true
    } catch (error) {
      if (isCurrentMemoryOwner(owner)) setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
      return false
    }
  }

  const disableMemoryRecord = async (memoryId: string): Promise<void> => {
    const provider = getProvider()
    if (typeof provider.updateMemory !== 'function') return
    const owner = captureMemoryOwner()
    try {
      const memory = await provider.updateMemory(memoryId, { disabled: true })
      if (isCurrentMemoryOwner(owner)) {
        memoryListRequest.current += 1
        setMemoryRecords((records) => records.map((record) => record.id === memoryId ? memory : record))
      }
    } catch (error) {
      if (isCurrentMemoryOwner(owner)) setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
    }
  }

  const deleteMemoryRecord = async (memoryId: string): Promise<void> => {
    const provider = getProvider()
    if (typeof provider.deleteMemory !== 'function') return
    const owner = captureMemoryOwner()
    try {
      await provider.deleteMemory(memoryId)
      if (isCurrentMemoryOwner(owner)) {
        memoryListRequest.current += 1
        setMemoryRecords((records) => records.filter((record) => record.id !== memoryId))
      }
    } catch (error) {
      if (isCurrentMemoryOwner(owner)) setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
    }
  }

  const scrollToAgentSection = (target: 'agents' | 'skill' | 'mcp'): void => {
    const refs = {
      agents: agentsSectionRef.current,
      skill: skillSectionRef.current,
      mcp: mcpSectionRef.current
    }
    refs[target]?.scrollIntoView({ behavior: 'auto', block: 'start' })
  }

  const finishSettingsWrite = (): void => {
    settingsWritesPending.current -= 1
    if (settingsEntryActive.current && settingsWritesPending.current === 0) {
      setSaveStatus(current => current === 'saving' ? 'idle' : current)
    }
  }

  const persistSettings = (
    snapshot: AppSettingsV1,
    version: number
  ): Promise<AppSettingsV1> => {
    const savedVersion = version
    settingsWritesPending.current += 1
    let saveRequest = 0
    let saveCompletionHostVersion: number | null = null
    const pending = settingsSaveQueue.current.then(async () => {
      if (!settingsEntryActive.current) throw new Error('Settings entry is no longer current.')
      if (version !== draftVersion.current) return latestForm.current ?? snapshot
      const current = snapshot
      if (!hasValidPort(current)) throw new Error('Invalid runtime port.')
      saveRequest = ++settingsWriteRequest.current
      setSaveStatus('saving')
      setSaveError(null)
      pendingSettingsWrite.current = { request: saveRequest, version: savedVersion }
      const saveHostVersion = settingsHostReadbackVersion.current
      saveCompletionHostVersion = saveHostVersion
      const baseline = settingsFormBaseline.current ?? current
      const patch = ordinaryDraftPatch(baseline, current)
      lastSubmittedDraft.current = { version: savedVersion, form: current, baseline,
        accepted: acceptedSettings.current ?? baseline }
      const profiles = getAnalytixRuntimeSettings(current).subagents.profiles
      const deletionIntents = new Map(profileDeletionIntents.current)
      const removedProfiles = Object.fromEntries([...deletionIntents.keys()]
        .filter(name => !Object.hasOwn(profiles, name)).map(name => [name, null]))
      if (Object.keys(removedProfiles).length) patch.runtime = { ...patch.runtime, subagents: { ...patch.runtime?.subagents,
        profiles: { ...patch.runtime?.subagents?.profiles, ...removedProfiles } } }
      const next = coerceRendererSettings(await rendererRuntimeClient.setSettings(patch))
      if (!settingsEntryActive.current || saveHostVersion !== settingsHostReadbackVersion.current) return next
      for (const [name, intent] of deletionIntents) {
        if (!Object.hasOwn(getAnalytixRuntimeSettings(next).subagents.profiles, name) &&
            profileDeletionIntents.current.get(name) === intent) profileDeletionIntents.current.delete(name)
      }
      modelPreferencesReadback.current = next.provider.modelPreferences
      acceptedSettings.current = next
      settingsFormBaseline.current = next
      adoptPerformanceUsageHostReadback(next)
      if (savedVersion !== draftVersion.current) return next
      applyForm(next)
      emitRendererSettingsChanged(next)
      const acceptedHostVersion = settingsHostReadbackVersion.current
      saveCompletionHostVersion = acceptedHostVersion
      await applyI18n(next.locale)
      if (!settingsEntryActive.current || savedVersion !== draftVersion.current ||
          acceptedHostVersion !== settingsHostReadbackVersion.current) return next
      acceptedDraftVersion.current = savedVersion
      void reloadUiSettings()
      void probeRuntime('background')
      setSaveStatus('saved')
      if (statusTimer.current) window.clearTimeout(statusTimer.current)
      statusTimer.current = window.setTimeout(() => {
        if (settingsEntryActive.current && savedVersion === draftVersion.current) setSaveStatus('idle')
        statusTimer.current = null
      }, 1500)
      return next
    }).catch((error: unknown) => {
      if (settingsEntryActive.current && savedVersion === draftVersion.current &&
          (saveCompletionHostVersion === null || saveCompletionHostVersion === settingsHostReadbackVersion.current)) {
        setSaveError(t('settingsSaveFailed'))
        setSaveStatus('error')
        void window.analytix?.logs?.error?.('settings', 'Failed to apply settings', { code: 'SETTINGS_APPLY_FAILED' }).catch(() => undefined)
        const failedForm = latestForm.current
        const failedHostReadbackVersion = settingsHostReadbackVersion.current
        // Read the Host directly: recovery must neither hold the save queue nor overwrite its query cache.
        if (saveRequest !== 0) {
          const recovery = ++hostRecoveryId.current
          hostRecoveryPending.current = true
          setRecoveringHostSettings(true)
          void (async () => {
            try {
              const accepted = coerceRendererSettings(await rendererRuntimeClient.readSettingsFromHost())
              if (settingsEntryActive.current && saveRequest === settingsWriteRequest.current &&
                  savedVersion === draftVersion.current &&
                  failedHostReadbackVersion === settingsHostReadbackVersion.current &&
                  failedForm && latestForm.current === failedForm) {
                // Failed IPC may follow a Host commit: retire stale cached settings only for this current recovery.
                rendererRuntimeClient.invalidateSettings()
                // Recover accepted preview values while retaining the failed form and its error.
                acceptedSettings.current = accepted
                adoptPerformanceUsageHostReadback(accepted)
              }
            } catch { /* Keep the last accepted preview and the failed local draft. */ }
          })().finally(() => {
            if (recovery !== hostRecoveryId.current) return
            hostRecoveryPending.current = false
            if (settingsEntryActive.current) setRecoveringHostSettings(false)
          })
        }
      }
      throw error
    })
    // A failed write is drained; it cannot poison the next normal save.
    const retireWrite = (): void => {
      finishSettingsWrite()
      if (pendingSettingsWrite.current?.request === saveRequest) pendingSettingsWrite.current = null
    }
    settingsSaveQueue.current = pending.then(retireWrite, retireWrite)
    return pending
  }

  // Provider model preferences are an explicit, independent facade action. Save
  // only that patch so a Registry readback cannot commit unrelated form drafts.
  const saveModelPreferences = (partial: SettingsPatch): Promise<AppSettingsV1> => {
    if (!latestForm.current || !settingsEntryActive.current) {
      return Promise.reject(new Error('Settings entry is unavailable.'))
    }
    settingsWritesPending.current += 1
    setSaveStatus('saving')
    const pending = settingsSaveQueue.current.then(async () => {
      if (!settingsEntryActive.current) throw new Error('Settings entry is no longer current.')
      const hostVersion = settingsHostReadbackVersion.current
      const next = coerceRendererSettings(await rendererRuntimeClient.setSettings(ordinarySettingsPatch(partial)))
      if (!settingsEntryActive.current || hostVersion !== settingsHostReadbackVersion.current) return next
      acceptedSettings.current = next
      modelPreferencesReadback.current = next.provider.modelPreferences
      if (acceptedDraftVersion.current === draftVersion.current) {
        settingsFormBaseline.current = next
        applyForm(next)
      }
      else if (latestForm.current) applyForm(mergeSettings(latestForm.current, {
        provider: { modelPreferences: next.provider.modelPreferences }
      }))
      emitRendererSettingsChanged(next)
      setSaveStatus('idle')
      return next
    }).catch((error: unknown) => {
      if (settingsEntryActive.current) {
        setSaveStatus('error')
        setSaveError(t('settingsSaveFailed'))
      }
      throw error
    })
    settingsSaveQueue.current = pending.then(finishSettingsWrite, finishSettingsWrite)
    return pending
  }

  const markDraftChanged = (): void => {
    draftVersion.current += 1
    if (statusTimer.current) window.clearTimeout(statusTimer.current)
    statusTimer.current = null
    setSaveStatus('idle')
    setSaveError(null)
  }

  const restorePreview = (settings: AppSettingsV1): void => {
    applyTheme(settings.theme, settings.accentColor)
    applyUiFontScale(settings.uiFontScale)
    applyMotionPreference(settings.motionPreference)
    applyCursorSpotlight(settings.cursorSpotlight !== false)
    applyWriteTypography(settings.write.typography)
    adoptPerformanceUsageHostReadback(settings)
    void applyI18n(settings.locale)
  }

  const saving = saveActionPending || recoveringHostSettings || saveStatus === 'saving' || providerActions?.busy === true
  const ordinaryDraftDirty = acceptedDraftVersion.current !== draftVersion.current || runtimeRejectedDraftVersion.current === draftVersion.current
  const hasUnsavedChanges = ordinaryDraftDirty || providerDirty || providerActions?.dirty === true

  const discardAndClose = (): void => {
    // A dispatched write cannot be cancelled. Keep the modal open until it
    // settles, and fence every queued/read callback before leaving.
    if (saveActionInFlight.current || settingsWritesPending.current > 0 || hostRecoveryPending.current || providerActionsRef.current?.busy) return
    providerActionsRef.current?.discard()
    settingsEntryActive.current = false
    draftVersion.current += 1
    settingsReadRequest.current += 1
    profileDeletionIntents.current.clear()
    if (statusTimer.current) window.clearTimeout(statusTimer.current)
    statusTimer.current = null
    if (acceptedSettings.current) restorePreview(acceptedSettings.current)
    setRoute(settingsReturnRoute)
  }

  const goBack = (): void => {
    if (saving || saveActionInFlight.current || settingsWritesPending.current > 0 || hostRecoveryPending.current || providerActionsRef.current?.busy) return
    if (confirmDiscard) { setConfirmDiscard(false); return }
    if (hasUnsavedChanges) {
      discardPromptOpener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      setConfirmDiscard(true)
      return
    }
    discardAndClose()
  }

  const savePreferences = async (): Promise<void> => {
    if (saveActionInFlight.current || settingsWritesPending.current > 0 || hostRecoveryPending.current || providerActionsRef.current?.busy) return
    const current = latestForm.current
    if (!current || !hasValidPort(current)) return
    saveActionInFlight.current = true
    setSaveActionPending(true)
    setSaveError(null)
    const retryRejectedDraft = runtimeRejectedDraftVersion.current === draftVersion.current
    runtimeRejectedDraftVersion.current = null
    let submittedVersion: number | null = null
    try {
      // Registry/SecretStore remains the sole credential authority. Its facade
      // owns key preservation and readback; ordinary settings never receive keys.
      const actions = providerActionsRef.current
      if (actions?.dirty && !await actions.save()) {
        // A nested provider editor may still need Apply. The provider owns its
        // precise error or pending message; false never means discard or close.
        setSaveStatus('idle')
        return
      }
      const snapshot = latestForm.current
      if (snapshot && (acceptedDraftVersion.current !== draftVersion.current || retryRejectedDraft)) {
        const version = draftVersion.current
        submittedVersion = version
        await persistSettings(snapshot, version)
        if (!settingsEntryActive.current || version !== draftVersion.current ||
            acceptedDraftVersion.current !== version || runtimeRejectedDraftVersion.current === version) return
      }
      if (settingsEntryActive.current) setSaveStatus('saved')
    } catch {
      // The owning persistence seam keeps the failed draft and error visible.
      if (settingsEntryActive.current && submittedVersion === null) {
        setSaveStatus('error')
        setSaveError(t('settingsSaveFailed'))
      }
    } finally {
      saveActionInFlight.current = false
      if (settingsEntryActive.current) setSaveActionPending(false)
    }
  }

  const openProviderSettings = (): void => { acceptSettingsSection('providers') }

  if (loadError) {
    const msg =
      loadError === 'PRELOAD_BRIDGE' ? t('preloadBridgeError') : t('settingsReadFailed')
    return (
      <Dialog open title={t('title')} width={678} className="ax-settings-modal" maskClassName="ax-settings-modal-mask"
        closeLabel="×" closeAriaLabel={t('settingsClose')} onClose={goBack}>
        <div className="ax-settings-content flex flex-col items-center justify-center gap-4 p-6 text-center">
          <p role="alert">{msg}</p>
          <button type="button" className="ax-settings-save" onClick={() => readSettingsForm(true)}>{t('settingsReadRetry')}</button>
          <button type="button" className="ax-settings-cancel" onClick={discardAndClose}>{t('settingsCancel')}</button>
        </div>
      </Dialog>
    )
  }

  if (!form) {
    return <Dialog open title={t('title')} width={678} className="ax-settings-modal" maskClassName="ax-settings-modal-mask"
      closeLabel="×" closeAriaLabel={t('settingsClose')} onClose={goBack}>
      <AnalytixLoadingPage fillParent label={t('loading')} />
    </Dialog>
  }

  const analytix = getAnalytixRuntimeSettings(form)
  const provider = getModelProviderSettings(form)

  const update = (partial: SettingsPatch): void => {
    for (const [name, profile] of Object.entries(partial.runtime?.subagents?.profiles ?? {})) {
      if (profile === null) profileDeletionIntents.current.set(name, ++profileIntentRevision.current)
      else profileDeletionIntents.current.delete(name)
    }
    const next = mergeSettings(latestForm.current ?? form, partial)
    applyForm(next)
    if (partial.locale) void applyI18n(partial.locale)
    if (partial.guiUpdate?.channel && partial.guiUpdate.channel !== form.guiUpdate.channel) {
      resetGuiUpdateState()
    }
    markDraftChanged()
  }

  const sharedBaseUrl = provider.baseUrl
  const writeInlineBaseUrlInherited =
    !form.write.inlineCompletion.baseUrl.trim() ||
    form.write.inlineCompletion.baseUrl.trim() === DEFAULT_WRITE_INLINE_COMPLETION_BASE_URL
  const writeInlineModelInherited = form.write.inlineCompletion.inheritModel !== false
  const effectiveWriteInlineBaseUrl = resolveWriteInlineCompletionBaseUrl(form)
  const effectiveWriteInlineModel = resolveWriteInlineCompletionModel(form)
  const updateSharedProvider = (patch: { baseUrl?: string }): void => {
    update({ provider: patch })
  }

  const updateAnalytix = (patch: AnalytixRuntimeSettingsPatchV1): void => {
    update({ runtime: patch })
  }

  const pickWorkspace = async (): Promise<void> => {
    try {
      setWorkspacePickerError(null)
      if (typeof window.analytix?.workspace?.pickDirectory !== 'function') {
        throw new Error('workspace:pick-directory unavailable')
      }
      const picked = await window.analytix.workspace.pickDirectory(form.workspaceRoot || undefined)
      if (!picked.canceled && picked.path) {
        update({ workspaceRoot: picked.path })
      }
    } catch (e) {
      setWorkspacePickerError(formatWorkspacePickerError(e))
    }
  }

  const resetWorkspaceToDefault = (): void => {
    setWorkspacePickerError(null)
    update({ workspaceRoot: DEFAULT_WORKSPACE_ROOT })
  }

  const pickWriteWorkspace = async (): Promise<void> => {
    try {
      setWriteWorkspacePickerError(null)
      if (typeof window.analytix?.workspace?.pickDirectory !== 'function') {
        throw new Error('workspace:pick-directory unavailable')
      }
      const picked = await window.analytix.workspace.pickDirectory(
        form.write.defaultWorkspaceRoot || DEFAULT_WRITE_WORKSPACE_ROOT
      )
      if (!picked.canceled && picked.path) {
        const workspaces = [
          picked.path,
          form.write.activeWorkspaceRoot,
          ...form.write.workspaces
        ].filter((value, index, list) => value.trim() && list.indexOf(value) === index)
        update({
          write: {
            defaultWorkspaceRoot: picked.path,
            activeWorkspaceRoot: picked.path,
            workspaces
          }
        })
      }
    } catch (e) {
      setWriteWorkspacePickerError(formatWorkspacePickerError(e))
    }
  }

  const resetWriteWorkspaceToDefault = (): void => {
    setWriteWorkspacePickerError(null)
    update({
      write: {
        defaultWorkspaceRoot: DEFAULT_WRITE_WORKSPACE_ROOT,
        activeWorkspaceRoot: DEFAULT_WRITE_WORKSPACE_ROOT,
        workspaces: [DEFAULT_WRITE_WORKSPACE_ROOT, ...form.write.workspaces]
      }
    })
  }

  const pickClawWorkspace = async (): Promise<void> => {
    try {
      setClawWorkspacePickerError(null)
      if (typeof window.analytix?.workspace?.pickDirectory !== 'function') {
        throw new Error('workspace:pick-directory unavailable')
      }
      const picked = await window.analytix.workspace.pickDirectory(
        form.claw.im.workspaceRoot || form.workspaceRoot || undefined
      )
      if (!picked.canceled && picked.path) {
        update({ claw: { im: { workspaceRoot: picked.path } } })
      }
    } catch (e) {
      setClawWorkspacePickerError(formatWorkspacePickerError(e))
    }
  }

  const resetClawWorkspaceToDefault = (): void => {
    setClawWorkspacePickerError(null)
    update({ claw: { im: { workspaceRoot: '' } } })
  }

  const clearWriteDebugEntries = async (): Promise<void> => {
    setWriteDebugLoading(true)
    setWriteDebugError(null)
    try {
      if (typeof window.analytix?.write?.clearWriteInlineCompletionDebugEntries === 'function') {
        await window.analytix.write.clearWriteInlineCompletionDebugEntries()
      }
      setWriteCompletionDebugEntries([])
      setWriteCompletionDebugSelectedId(null)
    } catch (error) {
      setWriteDebugError(error instanceof Error ? error.message : String(error))
    } finally {
      setWriteDebugLoading(false)
    }
  }

  const selectControlClass =
    'w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30'

  const settingsSectionContext = {
    t,
    tCommon,
    form,
    provider,
    analytix,
    update,
    saveModelPreferences,
    modelPreferencesReadback: modelPreferencesReadback.current,
    updateAnalytix,
    updateSharedProvider,
    sharedBaseUrl,
    showRuntimeToken,
    setShowRuntimeToken,
    portError,
    selectControlClass,
    openProviderSettings,
    pickWorkspace,
    resetWorkspaceToDefault,
    workspacePickerError,
    guiUpdateInfo,
    checkingGuiUpdate,
    downloadingGuiUpdate,
    installingGuiUpdate,
    guiUpdateDownloaded,
    guiUpdateProgress,
    guiUpdateError,
    checkGuiUpdate,
    downloadGuiUpdate,
    installGuiUpdate,
    logPath,
    logsSectionRef,
    logDirOpenError,
    setLogDirOpenError,
    pickWriteWorkspace,
    resetWriteWorkspaceToDefault,
    writeWorkspacePickerError,
    writeInlineBaseUrlInherited,
    effectiveWriteInlineBaseUrl,
    writeInlineModelInherited,
    effectiveWriteInlineModel,
    setWriteDebugModalOpen,
    loadWriteDebugEntries,
    scrollToAgentSection,
    agentsSectionRef,
    skillSectionRef,
    mcpSectionRef,
    permissionsSectionRef,
    skillRoots,
    skillRootsLoading,
    toggleSkillRoot,
    skillNotice,
    openSkillRoot,
    openPlugins,
    openSettingsSection: requestCategoryChange,
    onProviderDraftDirtyChange,
    onProviderActionsChange,
    mcpConfigPath,
    mcpConfigExists,
    mcpConfigText,
    setMcpConfigText,
    mcpLoading,
    mcpBusy,
    mcpNotice,
    saveMcpConfig,
    loadMcpConfig,
    openMcpConfigDir,
    runtimeInfo,
    toolDiagnostics,
    memoryRecords,
    memoryDiagnostics,
    runtimeDiagnosticsBusy,
    runtimeDiagnosticsNotice,
    refreshAnalytixDiagnostics,
    createMemoryRecord,
    updateMemoryRecord,
    disableMemoryRecord,
    deleteMemoryRecord,
    pickClawWorkspace,
    resetClawWorkspaceToDefault,
    clawWorkspacePickerError,
    splitSettingsList,
    listSettingsText,
    showTopNotice,
    runtimeReady: runtimeConnection === 'ready',
    locale: form.locale,
    openCode,
    selectThread,
    archiveThread,
    deleteThread
  }

  const footer = confirmDiscard ? (
    <div className="ax-settings-footer">
      <div className="ax-settings-footer-actions">
        <button ref={keepEditingRef} type="button" className="ax-settings-cancel" onClick={() => setConfirmDiscard(false)}>
          {t('settingsKeepEditing')}
        </button>
        <button type="button" className="ax-settings-save" onClick={discardAndClose}>
          {t('settingsDiscard')}
        </button>
      </div>
    </div>
  ) : (
    <div className="ax-settings-footer">
      {category === 'general' ? <button type="button" className="ax-settings-reset" disabled={saving}
        title={t('settingsResetAppearanceDescription')}
        onClick={() => update({ theme: 'light', accentColor: DEFAULT_APP_ACCENT_COLOR, uiFontScale: 'small',
          motionPreference: DEFAULT_APP_MOTION_PREFERENCE, cursorSpotlight: true })}>
        <span aria-hidden="true">↶</span> {t('settingsResetAppearance')}
      </button> : <span />}
      <span className="ax-settings-save-status" role="status" aria-live="polite" aria-atomic="true"
        data-tone={saveStatus === 'error' || portError ? 'error' : undefined}>
        {portError || (saveStatus === 'error' ? saveError || t('settingsSaveFailed')
          : saving ? t('settingsSaving') : hasUnsavedChanges ? t('settingsUnsavedChanges')
            : saveStatus === 'saved' ? t('settingsSaved') : '')}
      </span>
      <div className="ax-settings-footer-actions">
        <button type="button" className="ax-settings-cancel" disabled={saving} onClick={discardAndClose}>
          {t('settingsCancel')}
        </button>
        <button type="button" className="ax-settings-save" disabled={saving || !!portError || !hasUnsavedChanges}
          onClick={() => { void savePreferences() }}>
          {saving ? t('settingsSaving') : t('settingsSavePreferences')}
        </button>
      </div>
    </div>
  )

  return (
    <Dialog open title={t('title')} width={678} className="ax-settings-modal" maskClassName="ax-settings-modal-mask"
      closeLabel="×" closeAriaLabel={t('settingsClose')} onClose={goBack} footer={footer}>
      {confirmDiscard ? <div className="ax-settings-discard-prompt" role="alert">
        <h2>{t('settingsDiscardTitle')}</h2>
        <p>{t('settingsDiscardDescription')}</p>
      </div> : null}
      <div className="ax-settings-panel" hidden={confirmDiscard}>
        <fieldset disabled={saving} className="ax-settings-fields">
          <SettingsSidebar category={category} setCategory={requestCategoryChange} goBack={goBack} t={t} />
          <div ref={settingsContentRef} id="ax-settings-tabpanel" role="tabpanel"
            aria-labelledby={`ax-settings-tab-${settingsTabForCategory(category).id}`} className="ax-settings-content">
          {category === 'account' ? (
            <Suspense fallback={<AnalytixLoadingPage />}>
              <DeprecatedHubCompatibilitySection ctx={settingsSectionContext} />
            </Suspense>
          ) : null}
          {category === 'general' ? <GeneralSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'providers' ? <>
            <p className="mb-4 text-[13px] text-ds-muted">{t('modelProviderManualSaveHint')}</p>
            <ProvidersSettingsSection ctx={settingsSectionContext} />
          </> : null}
          {category === 'write' ? <WriteSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'imageGeneration' ? <ImageGenerationSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'speechToText' ? <SpeechToTextSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'agents' ? <AgentsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'permissions' ? <PermissionsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'browser' ? <BrowserSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'computerUse' ? <ComputerUseSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'archives' ? <ArchivedThreadsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'worktree' ? <WorktreeSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'memory' ? <MemorySettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'shortcuts' ? <KeyboardShortcutsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'claw' ? <ClawSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'updates' ? <UpdatesSettingsSection ctx={settingsSectionContext} /> : null}
          </div>
        </fieldset>
      </div>
      {writeDebugModalOpen ? (
        <WriteDebugLogModal
          completionEntries={writeCompletionDebugEntries}
          completionSelectedId={writeCompletionDebugSelectedId}
          loading={writeDebugLoading}
          error={writeDebugError}
          onSelectCompletion={setWriteCompletionDebugSelectedId}
          onRefresh={() => void loadWriteDebugEntries()}
          onClear={() => void clearWriteDebugEntries()}
          onClose={() => setWriteDebugModalOpen(false)}
          t={t}
        />
      ) : null}
    </Dialog>
  )
}
