import type { ReactElement } from 'react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DEFAULT_WRITE_INLINE_COMPLETION_BASE_URL,
  DEFAULT_WRITE_WORKSPACE_ROOT,
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
import { SettingsSidebar } from './SettingsSidebar'
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
  EasterEggSettingsSection,
  GeneralSettingsSection,
  ImageGenerationSettingsSection,
  KeyboardShortcutsSettingsSection,
  LlmDebugSettingsSection,
  WorktreeSettingsSection,
  MediaGenerationSettingsSection,
  MemorySettingsSection,
  PermissionsSettingsSection,
  ProvidersSettingsSection,
  SpeechToTextSettingsSection,
  UpdatesSettingsSection,
  WriteSettingsSection
} from './settings-sections'

type SettingsCategory = 'account' | 'general' | 'providers' | 'write' | 'imageGeneration' | 'mediaGeneration' | 'speechToText' | 'agents' | 'archives' | 'permissions' | 'browser' | 'computerUse' | 'worktree' | 'memory' | 'shortcuts' | 'easterEgg' | 'claw' | 'updates' | 'debug'
type SaveStatus = 'idle' | 'saving' | 'saved' | 'error'
type SettingsPatch = AppSettingsPatch
type InlineNotice = {
  tone: 'success' | 'error' | 'info'
  message: string
}

const SETTINGS_SKILL_ROOTS_STALE_MS = 60_000
const RUNTIME_CONFIG_STALE_MS = 60_000

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
  const openWrite = useChatStore((s) => s.openWrite)
  const openClaw = useChatStore((s) => s.openClaw)
  const openSchedule = useChatStore((s) => s.openSchedule)
  const openInitialSetup = useChatStore((s) => s.openInitialSetup)
  const openPlugins = useChatStore((s) => s.openPlugins)
  const applyI18n = useChatStore((s) => s.applyI18nFromSettings)
  const reloadUiSettings = useChatStore((s) => s.reloadUiSettings)
  const showTopNotice = useChatStore((s) => s.showTopNotice)
  const probeRuntime = useChatStore((s) => s.probeRuntime)
  const threads = useChatStore((s) => s.threads)
  const runtimeConnection = useChatStore((s) => s.runtimeConnection)
  const refreshThreads = useChatStore((s) => s.refreshThreads)
  const selectThread = useChatStore((s) => s.selectThread)
  const archiveThread = useChatStore((s) => s.archiveThread)
  const deleteThread = useChatStore((s) => s.deleteThread)
  const [category, setCategory] = useState<SettingsCategory>('general')
  const [form, setForm] = useState<AppSettingsV1 | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [workspacePickerError, setWorkspacePickerError] = useState<string | null>(null)
  const [writeWorkspacePickerError, setWriteWorkspacePickerError] = useState<string | null>(null)
  const [clawWorkspacePickerError, setClawWorkspacePickerError] = useState<string | null>(null)
  const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)
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
  const saveTimer = useRef<ReturnType<typeof window.setTimeout> | null>(null)
  const statusTimer = useRef<ReturnType<typeof window.setTimeout> | null>(null)
  const draftVersion = useRef(0)
  const agentsSectionRef = useRef<HTMLDivElement | null>(null)
  const skillSectionRef = useRef<HTMLDivElement | null>(null)
  const mcpSectionRef = useRef<HTMLDivElement | null>(null)
  const permissionsSectionRef = useRef<HTMLDivElement | null>(null)
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
  const formUiFontScale = form?.uiFontScale
  const formMotionPreference = form?.motionPreference
  const formCursorSpotlight = form?.cursorSpotlight
  const writeTypography = form?.write?.typography
  const formWorkspaceRoot = form?.workspaceRoot
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

  useEffect(() => {
    let cancelled = false
    if (typeof window.analytix === 'undefined') {
      setLoadError('PRELOAD_BRIDGE')
      return
    }
    void rendererRuntimeClient
      .getSettings({ forceRefresh: true })
      .then((s) => {
        if (!cancelled) setForm(coerceRendererSettings(s))
      })
      .catch((e: unknown) => {
        if (!cancelled) setLoadError(e instanceof Error ? e.message : String(e))
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!formTheme || !formUiFontScale) return
    applyTheme(formTheme)
    applyUiFontScale(formUiFontScale)
    applyMotionPreference(formMotionPreference)
    applyCursorSpotlight(formCursorSpotlight !== false)
  }, [formTheme, formUiFontScale, formMotionPreference, formCursorSpotlight])

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
    const onSettingsChanged = (event: Event): void => {
      const next = (event as CustomEvent<AppSettingsV1>).detail
      if (next) setForm(coerceRendererSettings(next))
    }
    window.addEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)
    return () => window.removeEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)
  }, [])

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
    if (settingsSection === 'account') {
      setCategory('account')
      return
    }
    if (settingsSection === 'general') {
      setCategory('general')
      return
    }
    if (settingsSection === 'providers') {
      setCategory('providers')
      return
    }
    if (settingsSection === 'write') {
      setCategory('write')
      return
    }
    if (settingsSection === 'imageGeneration') {
      setCategory('imageGeneration')
      return
    }
    if (settingsSection === 'mediaGeneration') {
      setCategory('mediaGeneration')
      return
    }
    if (settingsSection === 'speechToText') {
      setCategory('speechToText')
      return
    }
    if (settingsSection === 'permissions') {
      setCategory('permissions')
      return
    }
    if (settingsSection === 'browser') {
      setCategory('browser')
      return
    }
    if (settingsSection === 'computerUse') {
      setCategory('computerUse')
      return
    }
    if (settingsSection === 'archives') {
      setCategory('archives')
      return
    }
    if (settingsSection === 'worktree') {
      setCategory('worktree')
      return
    }
    if (settingsSection === 'memory') {
      setCategory('memory')
      return
    }
    if (settingsSection === 'claw') {
      setCategory('claw')
      return
    }
    if (settingsSection === 'shortcuts') {
      setCategory('shortcuts')
      return
    }
    if (settingsSection === 'easterEgg') {
      setCategory('easterEgg')
      return
    }
    if (settingsSection === 'updates') {
      setCategory('updates')
      return
    }
    if (settingsSection === 'debug') {
      setCategory('debug')
      return
    }
    setCategory('agents')
  }, [settingsSection])

  useEffect(() => {
    if (!form) return
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
      settingsSection === 'easterEgg' ||
      settingsSection === 'updates' ||
      settingsSection === 'debug' ||
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
    window.requestAnimationFrame(() => {
      target.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }, [category, form, settingsSection])

  useEffect(() => {
    return () => {
      if (saveTimer.current) window.clearTimeout(saveTimer.current)
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
    setRuntimeDiagnosticsBusy(true)
    setRuntimeDiagnosticsNotice(null)
    try {
      const loaded = await loadAnalytixDiagnostics(provider, {
        workspace: normalizeWorkspaceRoot(formWorkspaceRoot)
      })
      setRuntimeInfo(loaded.runtimeInfo)
      setToolDiagnostics(loaded.toolDiagnostics)
      setMemoryRecords(loaded.memoryRecords)
      if (loaded.errors.length > 0) {
        setRuntimeDiagnosticsNotice({
          tone: 'error',
          message: loaded.errors.join(' | ')
        })
      }
    } catch (error) {
      setRuntimeInfo(null)
      setToolDiagnostics(null)
      setMemoryRecords([])
      setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
    } finally {
      setRuntimeDiagnosticsBusy(false)
    }
  }, [formWorkspaceRoot, setRuntimeDiagnosticsNotice])

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
    try {
      const memory = await provider.createMemory(input)
      setMemoryRecords((records) => [memory, ...records])
      return true
    } catch (error) {
      setRuntimeDiagnosticsNotice({
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
    try {
      const memory = await provider.updateMemory(memoryId, patch)
      setMemoryRecords((records) => records.map((record) => (record.id === memoryId ? memory : record)))
      return true
    } catch (error) {
      setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
      return false
    }
  }

  const disableMemoryRecord = async (memoryId: string): Promise<void> => {
    const provider = getProvider()
    if (typeof provider.updateMemory !== 'function') return
    try {
      const memory = await provider.updateMemory(memoryId, { disabled: true })
      setMemoryRecords((records) => records.map((record) => record.id === memoryId ? memory : record))
    } catch (error) {
      setRuntimeDiagnosticsNotice({
        tone: 'error',
        message: error instanceof Error ? error.message : String(error)
      })
    }
  }

  const deleteMemoryRecord = async (memoryId: string): Promise<void> => {
    const provider = getProvider()
    if (typeof provider.deleteMemory !== 'function') return
    try {
      await provider.deleteMemory(memoryId)
      setMemoryRecords((records) => records.filter((record) => record.id !== memoryId))
    } catch (error) {
      setRuntimeDiagnosticsNotice({
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
    refs[target]?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  const persistSettings = async (snapshot: AppSettingsV1, version: number): Promise<void> => {
    if (!hasValidPort(snapshot)) return
    setSaveStatus('saving')
    setSaveError(null)

    try {
      const next = coerceRendererSettings(await rendererRuntimeClient.setSettings(snapshot))
      if (version !== draftVersion.current) return

      setForm(next)
      emitRendererSettingsChanged(next)
      await applyI18n(next.locale)
      void reloadUiSettings()
      void probeRuntime('background')
      if (version !== draftVersion.current) return

      setSaveStatus('saved')
      if (statusTimer.current) window.clearTimeout(statusTimer.current)
      statusTimer.current = window.setTimeout(() => {
        if (version === draftVersion.current) setSaveStatus('idle')
        statusTimer.current = null
      }, 1500)
    } catch (e) {
      if (version !== draftVersion.current) return
      const message = e instanceof Error ? e.message : String(e)
      setSaveError(message)
      setSaveStatus('error')
      void window.analytix?.logs?.error?.('settings', 'Failed to apply settings', { message }).catch(() => undefined)
    }
  }

  const scheduleSave = (next: AppSettingsV1): void => {
    draftVersion.current += 1
    const version = draftVersion.current

    if (saveTimer.current) window.clearTimeout(saveTimer.current)
    if (statusTimer.current) window.clearTimeout(statusTimer.current)
    statusTimer.current = null
    setSaveError(null)

    if (!hasValidPort(next)) {
      setSaveStatus('idle')
      return
    }

    setSaveStatus('saving')
    saveTimer.current = window.setTimeout(() => {
      saveTimer.current = null
      void persistSettings(next, version)
    }, 450)
  }

  const flushPendingSave = async (): Promise<void> => {
    if (!form || !hasValidPort(form)) return
    draftVersion.current += 1
    const version = draftVersion.current

    if (saveTimer.current) {
      window.clearTimeout(saveTimer.current)
      saveTimer.current = null
    }
    if (statusTimer.current) {
      window.clearTimeout(statusTimer.current)
      statusTimer.current = null
    }

    await persistSettings(form, version)
  }

  const goBack = (): void => {
    void (async () => {
      await flushPendingSave()
      await reloadUiSettings()
      if (settingsReturnRoute === 'write') {
        await openWrite()
        return
      }
      if (settingsReturnRoute === 'claw') {
        openClaw()
        return
      }
      if (settingsReturnRoute === 'schedule') {
        openSchedule()
        return
      }
      if (settingsReturnRoute === 'plugins') {
        setRoute('plugins')
        return
      }
      await openCode()
    })()
  }

  const openOnboardingPreview = (): void => {
    void (async () => {
      await flushPendingSave()
      openInitialSetup('preview')
    })()
  }

  if (loadError) {
    const msg =
      loadError === 'PRELOAD_BRIDGE' ? t('preloadBridgeError') : t('loadFailed', { message: loadError })
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-ds-main p-6 text-center">
        <p className="max-w-md text-sm text-red-700 dark:text-red-300">{msg}</p>
        <button
          type="button"
          className="rounded-xl bg-ds-userbubble px-4 py-2 text-sm font-medium text-ds-userbubbleFg"
          onClick={goBack}
        >
          {t('back')}
        </button>
      </div>
    )
  }

  if (!form) {
    return <AnalytixLoadingPage fillParent label={t('loading')} />
  }

  const analytix = getAnalytixRuntimeSettings(form)
  const provider = getModelProviderSettings(form)

  const update = (partial: SettingsPatch): void => {
    const next = mergeSettings(form, partial)
    setForm(next)
    if (partial.locale) void applyI18n(partial.locale)
    if (partial.guiUpdate?.channel && partial.guiUpdate.channel !== form.guiUpdate.channel) {
      resetGuiUpdateState()
    }
    scheduleSave(next)
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
    updateAnalytix,
    updateSharedProvider,
    sharedBaseUrl,
    showRuntimeToken,
    setShowRuntimeToken,
    portError,
    selectControlClass,
    openOnboardingPreview,
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
    openSettingsSection: setCategory,
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
    threads,
    showTopNotice,
    runtimeReady: runtimeConnection === 'ready',
    locale: form.locale,
    refreshThreads,
    openCode,
    selectThread,
    archiveThread,
    deleteThread
  }

  return (
    <div className="ds-drag flex h-full min-h-0 w-full min-w-0 bg-ds-main">
      <SettingsSidebar category={category} setCategory={setCategory} goBack={goBack} t={t} />

      <div className="ds-page-scroll-edge ds-no-drag min-h-0 min-w-0 flex-1 overflow-y-auto px-10 py-10">
        <div className={`mx-auto ${category === 'account' ? 'max-w-[732px]' : 'max-w-3xl'}`}>
          {category !== 'account' && category !== 'browser' && category !== 'computerUse' ? (
            <div className="mb-8 flex items-start justify-between gap-4">
              <div>
                <h1 className="text-2xl font-semibold tracking-tight text-ds-ink">{t('title')}</h1>
                <p className="mt-1 text-[14px] text-ds-muted">{t('subtitle')}</p>
              </div>
              <span
                title={saveStatus === 'error' && saveError ? saveError : undefined}
                className={`shrink-0 rounded-full px-3 py-1 text-[12px] font-medium ${
                  portError
                    ? 'bg-amber-500/15 text-amber-700 dark:text-amber-200'
                    : saveStatus === 'saved'
                      ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-200'
                      : saveStatus === 'error'
                        ? 'bg-red-500/15 text-red-700 dark:text-red-200'
                        : 'bg-ds-subtle text-ds-muted'
                }`}
              >
                {portError
                  ? t('autoApplyBlocked')
                  : saveStatus === 'saving'
                    ? t('applying')
                    : saveStatus === 'saved'
                      ? t('applied')
                      : saveStatus === 'error'
                        ? t('applyFailed')
                        : t('autoApplyHint')}
              </span>
            </div>
          ) : null}

          {category === 'account' ? (
            <Suspense fallback={<AnalytixLoadingPage />}>
              <DeprecatedHubCompatibilitySection ctx={settingsSectionContext} />
            </Suspense>
          ) : null}
          {category === 'general' ? <GeneralSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'providers' ? <ProvidersSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'write' ? <WriteSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'imageGeneration' ? <ImageGenerationSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'mediaGeneration' ? <MediaGenerationSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'speechToText' ? <SpeechToTextSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'agents' ? <AgentsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'permissions' ? <PermissionsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'browser' ? <BrowserSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'computerUse' ? <ComputerUseSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'archives' ? <ArchivedThreadsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'worktree' ? <WorktreeSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'memory' ? <MemorySettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'shortcuts' ? <KeyboardShortcutsSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'easterEgg' ? <EasterEggSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'claw' ? <ClawSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'updates' ? <UpdatesSettingsSection ctx={settingsSectionContext} /> : null}
          {category === 'debug' ? <LlmDebugSettingsSection ctx={settingsSectionContext} /> : null}
        </div>
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
    </div>
  )
}
