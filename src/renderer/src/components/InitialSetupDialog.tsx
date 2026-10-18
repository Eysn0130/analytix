import { type ReactElement, useCallback, useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useModalFocus } from '../hooks/use-modal-focus'
import {
  DEFAULT_MODEL_PROVIDER_ID,
  normalizeAppSettings,
  type AppSettingsPatch,
  type AppSettingsV1,
  type ModelProviderPreset
} from '@shared/app-settings'
import {
  INITIAL_SETUP_PROVIDER_PRESETS,
  buildInitialSetupSettings,
  initialSetupAutoWirePlan,
  initialSetupDrafts,
  initialSetupProfileId,
  initialSetupSelection,
  persistInitialSetup,
  type InitialSetupDrafts,
  type InitialSetupSelection
} from './initial-setup-save'
import { rendererRuntimeClient } from '../agent/runtime-client'
import type { RuntimeConnectionStatus } from '../agent/types'
import { applyTheme } from '../lib/apply-theme'
import { emitRendererSettingsChanged } from '../lib/keyboard-shortcut-settings'
import { useChatStore } from '../store/chat-store'
import type { InitialSetupMode } from '../store/chat-store-types'
import {
  Eye,
  EyeOff,
  ExternalLink,
  ChevronDown,
  X
} from '../design/AnalytixUiIcons'

type ThemePref = AppSettingsV1['theme']
type SetupFormPatch = AppSettingsPatch
type InitialSetupCompletionState = {
  runtimeConnection: RuntimeConnectionStatus
  error: string | null
  runtimeErrorDetail?: string | null
}

function isGenericRuntimeStartFailure(message: string, fallbackRuntimeError: string): boolean {
  if (!message) return true
  if (message === fallbackRuntimeError) return true
  const lowered = message.toLowerCase()
  return lowered.includes('fetch failed') ||
    lowered.includes('failed to fetch') ||
    lowered.includes('the operation was aborted due to timeout') ||
    lowered.includes('did not become healthy') ||
    lowered.includes('runtime unhealthy') ||
    lowered.includes('analytix serve') ||
    lowered.includes('无法连接到 analytix')
}

function firstRunRuntimeError(input: {
  stateError: string | null
  stateDetail?: string | null
  fallbackRuntimeError: string
  restartRuntimeError: string
}): string {
  const message = input.stateError?.trim()
  if (isGenericRuntimeStartFailure(message ?? '', input.fallbackRuntimeError)) {
    const detail = input.stateDetail?.trim()
    return detail ? `${input.restartRuntimeError}\n\n${detail}` : input.restartRuntimeError
  }
  return message || input.restartRuntimeError
}

const themeOptions: { value: ThemePref; labelKey: string }[] = [
  { value: 'system', labelKey: 'themeSystem' },
  { value: 'light', labelKey: 'themeLight' },
  { value: 'dark', labelKey: 'themeDark' }
]
const DEEPSEEK_USAGE_URL = 'https://platform.deepseek.com/usage'

type SetupProviderCard = {
  presetId: string
  name: string
  capability: 'speech' | 'image' | null
  preset: ModelProviderPreset | null
}

const PROVIDER_CARDS: SetupProviderCard[] = [
  {
    presetId: DEFAULT_MODEL_PROVIDER_ID,
    name: 'DeepSeek',
    capability: null,
    preset: null
  },
  ...INITIAL_SETUP_PROVIDER_PRESETS.map((preset) => ({
    presetId: preset.id,
    name: preset.name,
    capability: preset.speech ? ('speech' as const) : preset.image ? ('image' as const) : null,
    preset
  }))
]

function keyHintKey(card: SetupProviderCard, mode: InitialSetupSelection['mode']): string {
  if (card.presetId === DEFAULT_MODEL_PROVIDER_ID) return 'firstRunBuyApiHint'
  const suffix = mode === 'token-plan' ? 'TokenPlan' : 'Api'
  return card.presetId === 'xiaomi' ? `firstRunKeyHintXiaomi${suffix}` : `firstRunKeyHintMinimax${suffix}`
}

function keyPageUrl(card: SetupProviderCard, mode: InitialSetupSelection['mode']): string {
  if (!card.preset) return DEEPSEEK_USAGE_URL
  if (mode === 'token-plan' && card.preset.tokenPlan) return card.preset.tokenPlan.apiKeyUrl
  return card.preset.apiKeyUrl
}

function keyPlaceholder(card: SetupProviderCard, mode: InitialSetupSelection['mode']): string {
  if (mode === 'token-plan') {
    const prefix = card.preset?.tokenPlan?.keyPrefix
    return prefix ? `${prefix}...` : 'API Key'
  }
  return card.presetId === 'minimax' ? 'API Key' : 'sk-...'
}

export function canCloseInitialSetup(mode: InitialSetupMode): boolean {
  return mode === 'preview'
}

export async function completeInitialSetupAfterSave(input: {
  mode: InitialSetupMode
  reloadUiSettings: () => Promise<void>
  probeRuntime: (mode?: 'user' | 'background', options?: { restart?: boolean }) => Promise<void>
  openCode: () => Promise<void>
  selectSavedModel: () => void
  closeInitialSetup: () => void
  getState: () => InitialSetupCompletionState
  setDialogError: (message: string) => void
  fallbackRuntimeError: string
  restartRuntimeError: string
  isCurrent?: () => boolean
}): Promise<boolean> {
  const isCurrent = input.isCurrent ?? (() => true)
  if (!isCurrent()) return false
  await input.reloadUiSettings()
  if (!isCurrent()) return false
  if (input.mode === 'preview') {
    void input.probeRuntime('background')
    if (!isCurrent()) return false
    input.closeInitialSetup()
    return true
  }

  // Saving runtime settings already queues the managed restart when needed.
  // The committed Provider selection is read by Go from the Registry.
  await input.probeRuntime('user')
  if (!isCurrent()) return false
  const state = input.getState()
  if (state.runtimeConnection !== 'ready') {
    input.setDialogError(firstRunRuntimeError({
      stateError: state.error,
      stateDetail: state.runtimeErrorDetail,
      fallbackRuntimeError: input.fallbackRuntimeError,
      restartRuntimeError: input.restartRuntimeError
    }))
    return false
  }
  await input.openCode()
  if (!isCurrent()) return false
  input.selectSavedModel()
  input.closeInitialSetup()
  return true
}

export function InitialSetupDialog(): ReactElement {
  const { t } = useTranslation('settings')
  const fieldId = useId()
  const initialSetupMode = useChatStore((s) => s.initialSetupMode)
  const initialSetupOpen = useChatStore((s) => s.initialSetupOpen)
  const closeInitialSetup = useChatStore((s) => s.closeInitialSetup)
  const applyI18n = useChatStore((s) => s.applyI18nFromSettings)
  const reloadUiSettings = useChatStore((s) => s.reloadUiSettings)
  const probeRuntime = useChatStore((s) => s.probeRuntime)
  const openCode = useChatStore((s) => s.openCode)

  const [form, setForm] = useState<AppSettingsV1 | null>(null)
  const [drafts, setDrafts] = useState<InitialSetupDrafts | null>(null)
  const [selection, setSelection] = useState<InitialSetupSelection>({
    presetId: DEFAULT_MODEL_PROVIDER_ID,
    mode: 'api'
  })
  const [showApiKey, setShowApiKey] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const formRef = useRef<AppSettingsV1 | null>(null)
  const mounted = useRef(false)
  const entryRevision = useRef(0)
  const editRevision = useRef(0)
  const saveSequence = useRef(0)
  const activeSave = useRef<number | null>(null)
  const isPreview = initialSetupMode === 'preview'
  const closeAllowed = canCloseInitialSetup(initialSetupMode)

  const setCurrentForm = (next: AppSettingsV1 | null): void => {
    formRef.current = next
    setForm(next)
  }

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      entryRevision.current += 1
      activeSave.current = null
    }
  }, [])

  useEffect(() => {
    entryRevision.current += 1
    activeSave.current = null
    setSaving(false)
  }, [initialSetupMode, initialSetupOpen])

  const settingsReadRequest = useRef(0)
  const [settingsReadFailed, setSettingsReadFailed] = useState(false)
  const readSetupSettings = useCallback((uncached = false): void => {
    const request = ++settingsReadRequest.current
    const entry = entryRevision.current
    const edit = editRevision.current
    const isCurrent = (): boolean => mounted.current && useChatStore.getState().initialSetupOpen &&
      request === settingsReadRequest.current && entry === entryRevision.current && edit === editRevision.current
    setSettingsReadFailed(false)
    void (uncached ? rendererRuntimeClient.readSettingsFromHost()
      : rendererRuntimeClient.getSettings({ forceRefresh: true }))
      .then((s) => {
        if (!isCurrent()) return
        formRef.current = s
        setForm(s)
        setDrafts(initialSetupDrafts(s))
        setSelection(initialSetupSelection(s))
      })
      .catch(() => { if (isCurrent()) setSettingsReadFailed(true) })
  }, [])

  useEffect(() => {
    if (initialSetupOpen && !formRef.current) readSetupSettings()
    return () => { settingsReadRequest.current += 1 }
  }, [initialSetupMode, initialSetupOpen, readSetupSettings])

  const updateForm = (patch: SetupFormPatch) => {
    const current = formRef.current
    if (!current) return
    editRevision.current += 1
    const next = normalizeAppSettings({
      ...current,
      ...patch
    } as AppSettingsV1)
    setCurrentForm(next)
  }

  const handleThemeChange = (theme: ThemePref) => {
    if (!formRef.current) return
    updateForm({ theme })
    applyTheme(theme, formRef.current.accentColor)
  }

  const handleClose = () => {
    if (!closeAllowed) return
    entryRevision.current += 1
    activeSave.current = null
    setSaving(false)
    setError(null)
    closeInitialSetup()
    void reloadUiSettings()
  }

  const dialogRef = useModalFocus(handleClose, closeAllowed)

  const handleOpenLocalData = () => {
    if (activeSave.current !== null) return
    entryRevision.current += 1
    setError(null)
    closeInitialSetup()
    void reloadUiSettings()
  }

  const handleOpenKeyPage = (url: string) => {
    if (typeof window.analytix?.app?.openExternal !== 'function') return
    void window.analytix.app.openExternal(url).catch(() => undefined)
  }

  const selectedCard = PROVIDER_CARDS.find((card) => card.presetId === selection.presetId) ?? PROVIDER_CARDS[0]
  const selectedProfileId = initialSetupProfileId(selection)
  const selectedDraft = drafts?.[selectedProfileId] ?? { apiKey: '', baseUrl: '' }

  const updateSelectedDraft = (patch: Partial<typeof selectedDraft>): void => {
    editRevision.current += 1
    setDrafts((current) => current
      ? { ...current, [selectedProfileId]: { ...current[selectedProfileId], ...patch } }
      : current)
  }

  const selectCard = (presetId: string): void => {
    if (selection.presetId !== presetId) editRevision.current += 1
    setError(null)
    setShowApiKey(false)
    setSelection((current) => (current.presetId === presetId ? current : { presetId, mode: 'api' }))
  }

  const selectMode = (mode: InitialSetupSelection['mode']): void => {
    if (selection.mode !== mode) editRevision.current += 1
    setError(null)
    setShowApiKey(false)
    setSelection((current) => ({ ...current, mode }))
  }

  const handleSave = async () => {
    const current = formRef.current
    if (!current || !drafts || activeSave.current !== null) return
    const operation = ++saveSequence.current
    const entry = entryRevision.current
    const edit = editRevision.current
    activeSave.current = operation
    const isCurrent = (): boolean => {
      const state = useChatStore.getState()
      return mounted.current && activeSave.current === operation &&
        entryRevision.current === entry && editRevision.current === edit &&
        state.initialSetupOpen && state.initialSetupMode === initialSetupMode
    }
    const assertCurrent = (): void => {
      if (!isCurrent()) throw new Error('Setup edit is no longer current.')
    }
    setSaving(true)
    setError(null)
    try {
      const next = await persistInitialSetup({
        settings: current,
        drafts,
        selection,
        requestProviderRegistry: async (request) => {
          assertCurrent()
          const result = await window.analytix.providerRegistry.request(request)
          assertCurrent()
          return result
        },
        saveSettings: async (settings) => {
          assertCurrent()
          const result = await rendererRuntimeClient.setSettings(settings)
          assertCurrent()
          return result
        }
      })
      if (!isCurrent()) return
      setCurrentForm(next)
      setDrafts(initialSetupDrafts(next))
      emitRendererSettingsChanged(next)
      await applyI18n(next.locale)
      if (!isCurrent()) return
      await completeInitialSetupAfterSave({
        mode: initialSetupMode,
        reloadUiSettings,
        probeRuntime,
        openCode,
        selectSavedModel: () => useChatStore.getState().setComposerModel(
          next.runtime.model, next.runtime.providerId
        ),
        closeInitialSetup,
        getState: useChatStore.getState,
        setDialogError: setError,
        fallbackRuntimeError: t('common:runtimeFetchFailed'),
        restartRuntimeError: t('common:firstRunRuntimeStartFailed'),
        isCurrent
      })
    } catch (e) {
      if (isCurrent()) setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (activeSave.current === operation) {
        activeSave.current = null
        if (mounted.current) setSaving(false)
      }
    }
  }

  if (!form || !drafts) {
    return (
      <div className="ds-no-drag fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
        <div ref={dialogRef} role="dialog" aria-modal="true" aria-label={t(settingsReadFailed ? 'settingsReadFailed' : 'loading')} tabIndex={-1} className="rounded-xl border border-ds-border bg-ds-card px-5 py-4 text-sm text-ds-muted shadow-panel">
          <p role={settingsReadFailed ? 'alert' : 'status'}>{t(settingsReadFailed ? 'settingsReadFailed' : 'loading')}</p>
          <div className="mt-3 flex flex-wrap gap-3">
            {settingsReadFailed ? <button type="button" onClick={() => readSetupSettings(true)}
              className="rounded-lg bg-accent px-3 py-2 font-semibold text-accent-foreground">{t('settingsReadRetry')}</button> : null}
            <button type="button" onClick={closeAllowed ? handleClose : handleOpenLocalData}
              className="px-2 py-2 text-ds-muted hover:text-ds-ink">{t(closeAllowed ? 'firstRunClose' : 'firstRunOpenLocalData')}</button>
          </div>
        </div>
      </div>
    )
  }

  const selectedTheme = form.theme
  const tokenPlan = selectedCard.preset?.tokenPlan ?? null
  const showTokenPlanMode = Boolean(tokenPlan)
  const regions = selection.mode === 'token-plan' ? tokenPlan?.regions ?? [] : []
  const wire = initialSetupAutoWirePlan(form, drafts)
  const wireNote = (() => {
    if (!selectedCard.capability) return null
    const wiredProfileId = selectedCard.capability === 'speech' ? wire.speechProviderId : wire.imageProviderId
    if (wiredProfileId && wiredProfileId === selectedProfileId) {
      return {
        tone: 'success' as const,
        text: t(selectedCard.capability === 'speech' ? 'firstRunAutoWireSpeech' : 'firstRunAutoWireImage')
      }
    }
    if (selection.mode === 'token-plan' && selectedDraft.apiKey.trim()) {
      const planServesCapability = selectedCard.capability === 'speech'
        ? Boolean(tokenPlan?.speech)
        : selectedCard.capability === 'image' && Boolean(tokenPlan?.image)
      if (!planServesCapability) {
        return {
          tone: 'warning' as const,
          text: t(selectedCard.capability === 'speech' ? 'firstRunTokenPlanNoSpeech' : 'firstRunTokenPlanNoImage')
        }
      }
    }
    return null
  })()

  const fieldClass = 'h-10 w-full rounded-lg border border-ds-border bg-ds-main px-3 text-sm text-ds-ink outline-none transition-colors focus:border-accent/50 focus:ring-2 focus:ring-accent/15'
  const labelClass = 'grid gap-1.5 text-[13px] font-medium text-ds-ink'
  const currentRegion = regions.some((region) => region.baseUrl === selectedDraft.baseUrl)
  return (
    <div className="ds-no-drag fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <section ref={dialogRef} tabIndex={-1} role="dialog" aria-modal="true"
        aria-labelledby={`${fieldId}-title`}
        className="flex max-h-[calc(100dvh-32px)] w-full max-w-[520px] flex-col overflow-hidden rounded-xl border border-ds-border bg-ds-card text-ds-ink shadow-panel">
        <header className="shrink-0 px-6 pb-4 pt-6">
          <div className="mb-4 flex items-center justify-between gap-3">
            <span className="text-sm font-semibold">Analytix</span>
            {closeAllowed ? <button type="button" onClick={handleClose}
              aria-label={t('firstRunClose')} title={t('firstRunClose')}
              className="flex h-8 w-8 items-center justify-center rounded-lg text-ds-muted hover:bg-ds-hover hover:text-ds-ink">
              <X className="h-4 w-4" />
            </button> : null}
          </div>
          <h1 id={`${fieldId}-title`} className="text-xl font-semibold">{t('firstRunTitle')}</h1>
          <p className="mt-2 text-sm leading-5 text-ds-muted">{t('firstRunSubtitle')}</p>
        </header>

        <div className="min-h-0 space-y-4 overflow-y-auto px-6 pb-5">
          <label className={labelClass} htmlFor={`${fieldId}-provider`}>
            {t('firstRunProviderLabel')}
            <select id={`${fieldId}-provider`} className={fieldClass} value={selection.presetId}
              onChange={(event) => selectCard(event.target.value)}>
              {PROVIDER_CARDS.map((card) => <option key={card.presetId} value={card.presetId}>{card.name}</option>)}
            </select>
          </label>
          {showTokenPlanMode ? <label className={labelClass} htmlFor={`${fieldId}-mode`}>
            {t('firstRunModeLabel')}
            <select id={`${fieldId}-mode`} className={fieldClass} value={selection.mode}
              onChange={(event) => selectMode(event.target.value as InitialSetupSelection['mode'])}>
              <option value="api">{t('firstRunModeApi')}</option>
              <option value="token-plan">{t('firstRunModeTokenPlan')}</option>
            </select>
          </label> : null}
          {regions.length > 0 ? <label className={labelClass} htmlFor={`${fieldId}-region`}>
            {t('firstRunRegionLabel')}
            <select id={`${fieldId}-region`} className={fieldClass} value={selectedDraft.baseUrl}
              onChange={(event) => updateSelectedDraft({ baseUrl: event.target.value })}>
              {!currentRegion ? <option value={selectedDraft.baseUrl}>{t('firstRunCustomEndpoint')}</option> : null}
              {regions.map((region) => <option key={region.id} value={region.baseUrl}>{t(`firstRunRegion_${region.id}`)}</option>)}
            </select>
          </label> : null}

          <div className="space-y-2">
            <label className={labelClass} htmlFor={`${fieldId}-api-key`}>
              {t('firstRunApiKeyLabel', { provider: selectedCard.name })}
            </label>
            <div className="relative">
              <input id={`${fieldId}-api-key`} aria-describedby={`${fieldId}-key-help`}
                type={showApiKey ? 'text' : 'password'} value={selectedDraft.apiKey}
                onChange={(event) => updateSelectedDraft({ apiKey: event.target.value })}
                placeholder={keyPlaceholder(selectedCard, selection.mode)}
                autoComplete="off" autoCorrect="off" autoCapitalize="off" spellCheck={false}
                className={`${fieldClass} pr-11 font-mono placeholder:font-sans`} />
              <button type="button" onClick={() => setShowApiKey((value) => !value)}
                aria-label={t(showApiKey ? 'hideSecret' : 'showSecret')} aria-pressed={showApiKey}
                className="absolute right-1 top-1 flex h-8 w-8 items-center justify-center rounded-md text-ds-muted hover:bg-ds-hover hover:text-ds-ink">
                {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            <p id={`${fieldId}-key-help`} className="text-xs leading-5 text-ds-muted">{t(keyHintKey(selectedCard, selection.mode))}</p>
            <button type="button" onClick={() => handleOpenKeyPage(keyPageUrl(selectedCard, selection.mode))}
              className="inline-flex items-center gap-1.5 text-xs font-medium text-accent hover:underline">
              {t('firstRunGetKeyAction')}<ExternalLink className="h-3 w-3" />
            </button>
            {wireNote ? <p role="status" className={`text-xs leading-5 ${wireNote.tone === 'success' ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300'}`}>{wireNote.text}</p> : null}
          </div>

          <div className="border-t border-ds-border-muted pt-3">
            <button type="button" aria-expanded={advancedOpen} aria-controls={`${fieldId}-advanced`}
              onClick={() => setAdvancedOpen((value) => !value)}
              className="inline-flex items-center gap-2 text-xs font-medium text-ds-muted hover:text-ds-ink">
              {t('firstRunAdvanced')}<ChevronDown className={`h-3.5 w-3.5 ${advancedOpen ? 'rotate-180' : ''}`} />
            </button>
            {advancedOpen ? <div id={`${fieldId}-advanced`} className="mt-4 grid gap-4">
              <label className={labelClass} htmlFor={`${fieldId}-base-url`}>
                {t('baseUrl')}
                <input id={`${fieldId}-base-url`} type="url" value={selectedDraft.baseUrl}
                  onChange={(event) => updateSelectedDraft({ baseUrl: event.target.value })}
                  placeholder="https://" spellCheck={false} className={fieldClass} />
              </label>
              <label className={labelClass} htmlFor="initial-setup-model">
                {t('firstRunModelLabel')}
                <input id="initial-setup-model" type="text" maxLength={256}
                  value={selection.model ?? buildInitialSetupSettings(form, drafts, selection).runtime.model}
                  onChange={(event) => {
                    editRevision.current += 1
                    setSelection((current) => ({ ...current, model: event.target.value }))
                  }} className={fieldClass} />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label className={labelClass} htmlFor={`${fieldId}-theme`}>
                  {t('theme')}
                  <select id={`${fieldId}-theme`} className={fieldClass} value={selectedTheme}
                    onChange={(event) => handleThemeChange(event.target.value as ThemePref)}>
                    {themeOptions.map(({ value, labelKey }) => <option key={value} value={value}>{t(labelKey)}</option>)}
                  </select>
                </label>
                <label className={labelClass} htmlFor={`${fieldId}-locale`}>
                  {t('language')}
                  <select id={`${fieldId}-locale`} className={fieldClass} value={form.locale}
                    onChange={(event) => {
                      const locale = event.target.value as AppSettingsV1['locale']
                      updateForm({ locale })
                      void applyI18n(locale)
                    }}>
                    <option value="en">English</option><option value="zh">简体中文</option>
                  </select>
                </label>
              </div>
            </div> : null}
          </div>
          <p className="text-xs leading-5 text-ds-muted">{t('firstRunSecureStorage')}</p>
        </div>

        <footer className="shrink-0 space-y-3 border-t border-ds-border-muted px-6 py-4">
          {error ? <p role="alert" className="whitespace-pre-wrap text-sm text-red-700 dark:text-red-300">{error}</p> : null}
          <div className="flex flex-wrap items-center justify-between gap-3">
            {closeAllowed ? <button type="button" onClick={handleClose} className="h-10 px-2 text-sm text-ds-muted hover:text-ds-ink">{t('firstRunClose')}</button>
              : <button type="button" disabled={saving} onClick={handleOpenLocalData}
                className="h-10 px-2 text-sm text-ds-muted hover:text-ds-ink disabled:opacity-50">{t('firstRunOpenLocalData')}</button>}
            <button type="button" disabled={saving} onClick={handleSave}
              className="min-h-10 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground hover:opacity-90 disabled:opacity-50">
              {saving ? t('firstRunSaving') : t('firstRunSave')}
            </button>
          </div>
          <p className="text-xs leading-5 text-ds-muted">{t(isPreview ? 'firstRunPreviewHint' : 'firstRunLocalDataHint')}</p>
        </footer>
      </section>
    </div>
  )
}
