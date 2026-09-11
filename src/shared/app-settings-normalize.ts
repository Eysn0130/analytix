import {
  DEFAULT_GUI_UPDATE_CHANNEL,
  DEFAULT_APP_MOTION_PREFERENCE,
  normalizeAppMotionPreference,
  normalizeGuiUpdateChannel,
  type AppBehaviorConfigV1,
  type AppSettingsV1,
  type ClawSettingsPatchV1,
  type GuiUpdateConfigV1,
  type NotificationConfigV1,
  type ScheduleSettingsPatchV1,
  WINDOW_CLOSE_ACTIONS,
  type WindowCloseAction,
  type WriteSettingsPatchV1
} from './app-settings-types'
import { normalizeKeyboardShortcuts, type KeyboardShortcutsConfigV1 } from './keyboard-shortcuts'
import {
  defaultAnalytixRuntimeSettings,
  getAnalytixRuntimeSettings,
  mergeAnalytixRuntimeSettings,
  migrateLegacyAppSettings
} from './app-settings-runtime'
import {
  defaultMiniMaxMediaGenerationAnalytixPatch,
  normalizeModelProviderSettings,
  resolveModelProviderRequestSelection
} from './app-settings-provider'
import { normalizeDeepseekBaseUrl } from './app-settings-normalizers'
import { normalizeClawSettings } from './app-settings-claw'
import { normalizeScheduleSettings } from './app-settings-schedule'
import { normalizeWriteSettings } from './app-settings-write'

export function normalizeAppSettings(settings: AppSettingsV1): AppSettingsV1 {
  const migrated = shouldMigrateLegacySettings(settings)
    ? migrateLegacyAppSettings(settings as Parameters<typeof migrateLegacyAppSettings>[0])
    : settings
  const sanitized = stripRejectedAppFields(migrated)
  const maybeSettings = sanitized as AppSettingsV1 & {
    appBehavior?: Partial<AppBehaviorConfigV1>
    keyboardShortcuts?: Partial<KeyboardShortcutsConfigV1>
    notifications?: Partial<NotificationConfigV1>
    provider?: Parameters<typeof normalizeModelProviderSettings>[0]
    write?: WriteSettingsPatchV1
    claw?: ClawSettingsPatchV1
    schedule?: ScheduleSettingsPatchV1
    guiUpdate?: Partial<GuiUpdateConfigV1>
  }
  const providerSettings = normalizeModelProviderSettings(maybeSettings.provider)
  const runtime = getAnalytixRuntimeSettings(maybeSettings)
  const rawAnalytix = maybeSettings.runtime
  const rawMediaPatch: Parameters<typeof defaultMiniMaxMediaGenerationAnalytixPatch>[0]['analytixPatch'] = {
    ...(rawAnalytix?.textToSpeech !== undefined ? { textToSpeech: rawAnalytix.textToSpeech } : {}),
    ...(rawAnalytix?.musicGeneration !== undefined ? { musicGeneration: rawAnalytix.musicGeneration } : {}),
    ...(rawAnalytix?.videoGeneration !== undefined ? { videoGeneration: rawAnalytix.videoGeneration } : {})
  }
  const miniMaxMediaDefaults = defaultMiniMaxMediaGenerationAnalytixPatch({
    providers: providerSettings.providers,
    currentAnalytix: runtime,
    analytixPatch: rawMediaPatch
  })
  const mergedRuntime = mergeAnalytixRuntimeSettings(defaultAnalytixRuntimeSettings(), {
    ...runtime,
    baseUrl: runtime.baseUrl.trim() ? normalizeDeepseekBaseUrl(runtime.baseUrl) : '',
    ...(miniMaxMediaDefaults ?? {})
  })
  const runtimeSelection = resolveModelProviderRequestSelection({
    ...sanitized,
    provider: providerSettings,
    runtime: mergedRuntime
  } as AppSettingsV1, {
    model: mergedRuntime.model,
    providerId: mergedRuntime.providerId || undefined,
    runtimeProviderId: mergedRuntime.providerId || undefined
  })
  const runtimeProviderId = mergedRuntime.providerId.trim()
    ? runtimeSelection.providerId ?? mergedRuntime.providerId
    : mergedRuntime.providerId
  return {
    version: 1,
    locale: maybeSettings.locale === 'en' ? 'en' : 'zh',
    theme:
      maybeSettings.theme === 'light' || maybeSettings.theme === 'dark' || maybeSettings.theme === 'system'
        ? maybeSettings.theme
        : 'light',
    uiFontScale:
      maybeSettings.uiFontScale === 'small' ||
      maybeSettings.uiFontScale === 'medium' ||
      maybeSettings.uiFontScale === 'large'
        ? maybeSettings.uiFontScale
        : 'small',
    motionPreference: normalizeAppMotionPreference(maybeSettings.motionPreference ?? DEFAULT_APP_MOTION_PREFERENCE),
    cursorSpotlight: maybeSettings.cursorSpotlight !== false,
    provider: providerSettings,
    runtime: {
      ...mergedRuntime,
      providerId: runtimeProviderId,
      model: runtimeSelection.model || mergedRuntime.model
    },
    workspaceRoot: typeof maybeSettings.workspaceRoot === 'string' ? maybeSettings.workspaceRoot : '',
    log: {
      enabled: maybeSettings.log?.enabled !== false,
      retentionDays: typeof maybeSettings.log?.retentionDays === 'number' ? maybeSettings.log.retentionDays : 2
    },
    notifications: {
      turnComplete: maybeSettings.notifications?.turnComplete !== false
    },
    appBehavior: normalizeAppBehaviorSettings(maybeSettings.appBehavior),
    keyboardShortcuts: normalizeKeyboardShortcuts(maybeSettings.keyboardShortcuts),
    write: normalizeWriteSettings(maybeSettings.write),
    claw: normalizeClawSettings(maybeSettings.claw),
    schedule: normalizeScheduleSettings(maybeSettings.schedule),
    guiUpdate: {
      channel: normalizeGuiUpdateChannel(
        maybeSettings.guiUpdate?.channel ?? DEFAULT_GUI_UPDATE_CHANNEL
      )
    },
    codePromptPrefix: typeof maybeSettings.codePromptPrefix === 'string' ? maybeSettings.codePromptPrefix : '',
    disabledSkillIds: normalizeDisabledSkillIds(maybeSettings.disabledSkillIds)
  }
}

function stripRejectedAppFields<T extends object>(settings: T): T {
  const {
    agent: _agent,
    autoPlan: _autoPlan,
    auto_plan: _autoPlanSnake,
    agentProvider: _agentProvider,
    agents: _agents,
    deepseek: _deepseek,
    reasonix: _reasonix,
    ...rest
  } = settings as Record<string, unknown>
  void _agent
  void _autoPlan
  void _autoPlanSnake
  void _agentProvider
  void _agents
  void _deepseek
  void _reasonix
  return rest as T
}

function normalizeDisabledSkillIds(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return [...new Set(value
    .filter((id): id is string => typeof id === 'string')
    .map((id) => id.trim().replace(/^\/?skill:/i, '').trim())
    .filter(Boolean))]
}

export function normalizeAppBehaviorSettings(
  settings?: Partial<AppBehaviorConfigV1>
): AppBehaviorConfigV1 {
  const openAtLogin = settings?.openAtLogin === true
  const closeAction = normalizeWindowCloseAction(settings?.closeAction)
    ?? (settings?.closeToTray === true ? 'tray' : 'ask')
  return {
    openAtLogin,
    startMinimized: openAtLogin && settings?.startMinimized === true,
    closeAction,
    closeToTray: closeAction === 'tray'
  }
}

export function normalizeWindowCloseAction(value: unknown): WindowCloseAction | null {
  return typeof value === 'string' && WINDOW_CLOSE_ACTIONS.includes(value as WindowCloseAction)
    ? value as WindowCloseAction
    : null
}

export function mergeAppBehaviorSettings(
  current: AppBehaviorConfigV1,
  patch?: Partial<AppBehaviorConfigV1>
): AppBehaviorConfigV1 {
  const translatedPatch: Partial<AppBehaviorConfigV1> | undefined =
    patch && patch.closeAction === undefined && patch.closeToTray !== undefined
      ? {
          ...patch,
          closeAction: patch.closeToTray ? 'tray' : 'quit'
        }
      : patch
  return normalizeAppBehaviorSettings({
    ...current,
    ...(translatedPatch ?? {})
  })
}

function shouldMigrateLegacySettings(settings: AppSettingsV1): boolean {
  const raw = settings as AppSettingsV1 & {
    agentProvider?: unknown
    deepseek?: unknown
    runtime?: Partial<ReturnType<typeof defaultAnalytixRuntimeSettings>>
  }
  if (!raw.runtime) return true
  const dataDir = typeof raw.runtime.dataDir === 'string'
    ? raw.runtime.dataDir.replace(/\\/g, '/').toLowerCase()
    : ''
  return dataDir === '~/.analytix/coreagent' || dataDir.endsWith('/.analytix/coreagent')
}
