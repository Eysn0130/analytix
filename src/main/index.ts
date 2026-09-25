import { registerBrowserGuest } from './browser/browser-selection'
import { clearWriteRetrievalCache } from './services/write-retrieval-service'
import {
  app,
  BrowserWindow,
  dialog,
  ipcMain,
  Menu,
  nativeImage,
  Notification,
  powerSaveBlocker,
  shell,
  Tray,
  type IpcMainInvokeEvent
} from 'electron'
import { existsSync } from 'node:fs'
import { createWriteShutdownCoordinator } from './write-shutdown'
import { prepareNativeOfficeQuit, cancelNativeOfficeQuit } from './office/native-office-ipc'
import { createHash } from 'node:crypto'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  JsonSettingsStore,
  devServerHintUrl
} from './settings-store'
import { emitHubActivityObservation } from './hub-activity-observation'
import analytixAppIconPng from '../asset/brand/analytix-app-icon-512.png?url'
import analytixMacIconPng from '../asset/brand/analytix-app-icon-1024.png?url'
import { appIdentityIconSource, createAppIcon, prepareTrayIcon } from './app-icon'
import { configureLinuxWaylandImeSwitches } from './app-command-line'
import { APP_DISPLAY_NAME, configureAppIdentity, configureMacChromiumSessionData } from './app-identity'
import {
  inspectLegacyMigrationRequirement,
  issueElectronLegacySingletonAuthority,
  LegacyMigrationBlockedError,
  runLegacyAnalytixDataMigration,
  type LegacyMigrationQuiescenceAuthorityV2
} from './legacy-data-migration'
import {
  applyAnalytixRuntimePatch,
  getAnalytixRuntimeSettings,
  mergeAnalytixRuntimeSettings,
  mergeClawSettings,
  mergeAppBehaviorSettings,
  mergeModelProviderSettings,
  mergeScheduleSettings,
  mergeWriteSettings,
  normalizeAppSettings,
  normalizeAppBehaviorSettings,
  normalizeKeyboardShortcuts,
  analytixRuntimeSettingsEqual,
  buildAnalytixRuntimeSettingsKey,
  resolveAnalytixRuntimeSettings,
  type AppBehaviorConfigV1,
  type AppSettingsPatch,
  type AppSettingsV1,
  type WindowCloseAction
} from '../shared/app-settings'
import { parseRuntimeErrorBody, runtimeErrorToError, type RuntimeErrorCode } from '../shared/runtime-error'
import {
  createRuntimeStatusPublicV1,
  type RuntimeStatusPublicSourceV1,
  type RuntimeStatusPublicV1,
  type RuntimeStatusPublicV1Input
} from '../shared/analytix-runtime-status'
import {
  configurePackagedMainOwnedRuntimeAuthorityBootstrapV1
} from './runtime/main-owned-authority-bootstrap-v1'
import type { GuiUpdateState } from '../shared/gui-update'
import { isAllowedDevPreviewUrl } from '../shared/dev-preview-url'
import { MACOS_TRAFFIC_LIGHT_POSITION } from '../shared/window-chrome'
import { WINDOW_STARTUP_SURFACE_READY_CHANNEL } from '../shared/window-startup'
import { isAuthorizedPrototypeFileUrl } from './services/prototype-embed-registry'
import { fetchUpstreamModelIds } from './upstream-models'
import {
  analytixRuntimeAdapter,
  captureCurrentFinalPublicationAuthorityPin,
  configureGoRuntimeDesktopExternalStateBoundary,
  getAnalytixRuntimeBackendStatus,
  getRuntimeBaseUrlForSettings,
  isCurrentFinalPublicationAuthorityPin,
  migrateDesktopPrivateHistoryBeforeStartV2,
  runtimeAuthHeaders,
  runtimeRequestViaHost,
  setGoRuntimeUnexpectedExitHandler
} from './runtime/analytix-adapter'
import { waitForRuntimeTurnsIdle } from './runtime/managed-runtime-idle'
import {
  listManagedMcpAccountCredentialBindings,
  configureAnalytixProcessMainPrivatePaths,
  resolveBuiltInComputerUseMcpCommand,
  resolveAnalytixDataDir,
  setAnalytixUnexpectedExitHandler,
  type AnalytixUnexpectedExitInfo
} from './analytix-process'
import {
  captureOpenComputerUseAgentBaselineV1,
  stopOwnedOpenComputerUseAppAgentV1
} from './open-computer-use-agent-lifecycle'
import { RestartBudget, restartAfterPendingEnsure } from './analytix-runtime-supervisor'
import {
  runtimeErrorPublicDiagnosticV1,
  type RuntimeErrorPublicDiagnosticV1
} from './runtime-public-diagnostic'
import {
  configureLogger,
  logError,
  logWarn,
  pruneOnStartup,
  publicConsoleError,
  publicConsoleInfo,
  publicConsoleWarn
} from './logger'
import { createClawRuntime, type ClawRuntime } from './claw-runtime'
import { createScheduleRuntime, type ScheduleRuntime } from './schedule-runtime'
import { runClawScheduleMcpServerFromArgv } from './claw-schedule-mcp-server'
import {
  clawScheduleMcpSettingsChanged,
  resolveAnalytixConfigPath,
  resolveAnalytixMcpJsonPath,
  syncClawScheduleMcpConfig,
  type ClawScheduleMcpLaunchConfig
} from './claw-schedule-mcp-config'
import { registerAppIpcHandlers } from './ipc/register-app-ipc-handlers'
import { createPrivateMediaRuntimeRequest } from './services/private-media-runtime-request'
import {
  accountCredentialExpectedState,
  createAccountCredentialIpcHandler,
  createMainAccountCredentialLister,
  createMainAccountCredentialResolver,
  createProviderRegistryIpcHandler
} from './ipc/provider-registry-ipc'
import {
  createProviderOAuthAuthorizationLifecycle,
  createProviderOAuthRefreshScheduler,
  oauthAuthorizationBindingsEqual,
  type OAuthAuthorizationBindingV1
} from './provider-oauth-lifecycle'
import {
  createMainOAuthAccountAuthority,
  providerRegistryInputWithOAuthBinding,
  providerOAuthOwnerFenceFromProjection,
  type OAuthBindingTargetV1
} from './provider-oauth-main-authority'
import { createNativeOAuthCallbackRouter, nativeOAuthCallbackFromArgv } from './native-oauth-callback'
import { migrateLegacyImAccountCredentials } from './im-account-lifecycle'
import { ImChannelAccountLifecycle } from './im-channel-lifecycle'
import { createInstalledExtensionAccountLifecycle } from './extension-account-lifecycle'
import { isLocalDisplayRuntimePathV1 } from './local-display-runtime-paths'
import { ensureChromeBrowserUseNativeHostRegistration } from './services/chrome-browser-use-service'
import {
  configureManagedWeixinBridgeUrlResolver,
  pollFeishuInstall,
  pollWeixinInstall,
  startFeishuInstallQrcode,
  startWeixinBridgeChannel,
  startWeixinInstallQrcode
} from './claw-platform-install'
import { registerRuntimeSseIpc } from './runtime-sse-ipc'
import {
  registerTerminalPtyIpc,
  type TerminalPtyIpcController
} from './terminal/terminal-pty-ipc'
import { defaultDarwinTerminalProtectedRoots } from './terminal/terminal-process-policy'
import { registerBackgroundTaskIpc } from './background-task-ipc'
import { DataAnalysisBackendManager } from './data-analysis/backend-manager'
import { registerDataAnalysisIpcHandlers } from './data-analysis/ipc'
import { bindDataAnalysisRendererPrincipal } from './data-analysis/renderer-principal'
import {
  configureWeixinBridgeRuntimeContextProvider,
  configureWeixinBridgeAccountCredentialResolver,
  configureWeixinBridgeContextTokenAuthority,
  ensureWeixinBridgeRpcUrl,
  getWeixinBridgeAccountUserId,
  sendWeixinBridgeMessage,
  stopWeixinBridgeRuntime
} from './weixin-bridge-runtime'
import { webhookUrl } from './claw-runtime-helpers'
import { createTelegramRuntime, type TelegramRuntime } from './telegram-runtime'
import { isAnalytixHealthResponseBody } from './analytix-health'
import {
  buildTrayMenuTemplate,
  currentTrayProviderObservation,
  parseTrayThreads,
  type TrayProviderObservation,
  type TrayThreadSummary
} from './tray-session-menu'
import {
  ANALYTIX_DEEPLINK_PROTOCOL,
  isValidAnalytixThreadId,
  parseAnalytixThreadDeepLink
} from './thread-deeplink'
import {
  installPackagedRendererProtocol,
  isPackagedRendererNavigationURL,
  packagedRendererURL,
  registerPackagedRendererScheme
} from './packaged-renderer-protocol'
import {
  applyDesktopLoginItemSettings,
  applyDesktopProtocolRegistrations,
  consumeDesktopExternalStateBoundary
} from './desktop-external-state-isolation'

const __dirname = dirname(fileURLToPath(import.meta.url))
// 必须和 electron-builder 的 appId 一致,否则 Windows 通知、任务栏分组
// 和托盘行为会拆成另一个应用身份。
const APP_USER_MODEL_ID = 'com.analytix.desktop'
const NATIVE_OAUTH_CALLBACK_PROTOCOL = 'com.analytix.desktop'
const HIDDEN_START_ARG = '--hidden'
const desktopExternalState = consumeDesktopExternalStateBoundary(process.env, undefined, app.isPackaged)
configureGoRuntimeDesktopExternalStateBoundary(desktopExternalState)
const desktopStateHomeRoot = desktopExternalState.isolated
  ? desktopExternalState.stateHomeRoot
  : homedir()
const clawScheduleMcpConfigPaths = desktopExternalState.isolated
  ? {
      configTomlPath: resolveAnalytixConfigPath(desktopStateHomeRoot),
      mcpJsonPath: resolveAnalytixMcpJsonPath(desktopStateHomeRoot)
    }
  : undefined
if (desktopExternalState.isolated) {
  configureAnalytixProcessMainPrivatePaths({
    mcpConfigPath: clawScheduleMcpConfigPaths!.mcpJsonPath
  })
}
const startupTraceEnabled =
  process.env.ANALYTIX_STARTUP_TRACE === '1'
const startupTraceStart = Date.now()
const computerUseAgentBaselinePromise = captureOpenComputerUseAgentBaselineV1()
const nativeOAuthCallbackRouter = createNativeOAuthCallbackRouter()

function traceStartup(label: string, detail?: unknown): void {
  if (!startupTraceEnabled) return
  const elapsed = String(Date.now() - startupTraceStart).padStart(6, ' ')
  publicConsoleInfo('startup', `[+${elapsed}ms] ${label}`, detail)
}

function isManagedGoRuntimeBackend(): boolean {
  const backend = getAnalytixRuntimeBackendStatus().gate.backend
  return backend === 'go-runtime-default'
}

function shouldStartWeixinBridgeRuntime(settings: AppSettingsV1): boolean {
  return settings.claw.enabled &&
    settings.claw.im.enabled &&
    settings.claw.channels.some((channel) => channel.enabled && channel.provider === 'weixin')
}

function syncWeixinBridgeRuntime(settings: AppSettingsV1): void {
  if (!shouldStartWeixinBridgeRuntime(settings)) return
  void ensureWeixinBridgeRpcUrl().then(async () => {
    const accountIds = settings.claw.channels
      .filter((channel) => channel.enabled && channel.platformAccount?.kind === 'weixin')
      .map((channel) => channel.platformAccount!.accountId)
    await Promise.all(accountIds.map((accountId) => startWeixinBridgeChannel(accountId)))
  }).catch((error) => {
    logWarn(
      'weixin-bridge',
      'Failed to start managed WeChat bridge.',
      runtimeErrorPublicDiagnosticV1(error)
    )
  })
}

const runningClawScheduleMcpServer =
  process.argv.includes('--gui-schedule-mcp-server') || process.argv.includes('--claw-schedule-mcp-server')

function resolveLogDirectory(): string {
  return join(app.getPath('userData'), 'logs')
}

function resolvePreloadPath(): string {
  const cjsPath = join(__dirname, '../preload/index.cjs')
  if (existsSync(cjsPath)) return cjsPath
  return join(__dirname, '../preload/index.mjs')
}

function getClawScheduleMcpLaunchConfig(): ClawScheduleMcpLaunchConfig {
  return {
    appPath: app.getAppPath(),
    execPath: process.execPath,
    isPackaged: app.isPackaged
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function runtimeFailure(
  code: string,
  message: string,
  status = 0,
  details?: unknown,
  providerError?: unknown
) {
  return {
    ok: false as const,
    status,
    body: JSON.stringify({
      code,
      message,
      ...(details !== undefined ? { details } : {}),
      ...(providerError !== undefined ? { providerError } : {})
    })
  }
}

function runtimeJsonError(code: string, message: string): Error {
  return runtimeErrorToError({ code: code as RuntimeErrorCode, message })
}

traceStartup('main module evaluated')

if (runningClawScheduleMcpServer && process.platform === 'darwin') {
  app.dock?.hide()
}

// 在最早的阶段把 app 名称、AppUserModelId 都设好。
// Windows 任务栏 / 系统托盘 / 通知中心看到的应用名都来自这里;
// 设得太晚的话 BrowserWindow title、托盘、IPC 启动时拿到的还是旧的。
// 抽到 app-identity.ts 是为了让测试可以直接 import,不被 main 的
// whenReady 副作用污染。
configureAppIdentity()

// The product process first acquires the exact legacy Electron singleton.
// Migration then requires independent Go-persistence quiescence plus a native
// handle-bound filesystem transaction before its lease, journal, stage, or
// target writes. The current userData singleton is acquired only after the
// final durable barrier. Helper mode never enters this migration path.
let legacyMigrationReady = runningClawScheduleMcpServer
if (!runningClawScheduleMcpServer) {
  const intendedUserDataPath = app.getPath('userData')
  let legacySingletonHeld = false
  try {
    const requirement = inspectLegacyMigrationRequirement({
      userDataPath: intendedUserDataPath
    })
    let quiescenceAuthority: LegacyMigrationQuiescenceAuthorityV2 | undefined
    if (requirement.legacyUserDataPath) {
      // Acquire the exact legacy Electron singleton before snapshotting. The
      // singleton's root artifacts are explicitly excluded from the content
      // generation; all other links/junctions remain forbidden.
      app.setPath('userData', requirement.legacyUserDataPath)
      legacySingletonHeld = app.requestSingleInstanceLock()
      if (!legacySingletonHeld || !app.hasSingleInstanceLock()) {
        throw new LegacyMigrationBlockedError('quiescence_unavailable')
      }
      quiescenceAuthority = issueElectronLegacySingletonAuthority({
        legacyUserDataPath: requirement.legacyUserDataPath,
        baseline: requirement.singletonBaseline,
        assertHeld: () => legacySingletonHeld && app.hasSingleInstanceLock()
      })
    }
    const legacyMigration = runLegacyAnalytixDataMigration({
      userDataPath: intendedUserDataPath,
      homeDir: desktopStateHomeRoot,
      quiescenceAuthority,
      log: (message, detail) => publicConsoleWarn('legacy-migration', message, detail)
    })
    legacyMigrationReady = legacyMigration.barrier.ready
    traceStartup('legacy data migration barrier ready', {
      status: legacyMigration.barrier.status,
      migratedOwnerCount: legacyMigration.migratedOwnerIds.length,
      settingsRewritten: legacyMigration.settingsRewritten
    })
  } catch (error) {
    const code = error instanceof LegacyMigrationBlockedError ? error.code : 'io_error'
    publicConsoleError(
      'legacy-migration',
      'Startup stopped because legacy data migration could not establish a safe activation barrier.',
      { code }
    )
    app.exit(1)
  } finally {
    if (legacySingletonHeld) app.releaseSingleInstanceLock()
    app.setPath('userData', intendedUserDataPath)
  }
}

if (!runningClawScheduleMcpServer && legacyMigrationReady) configureMacChromiumSessionData()

configureLinuxWaylandImeSwitches()

if (!runningClawScheduleMcpServer && process.platform === 'win32') {
  app.setAppUserModelId(APP_USER_MODEL_ID)
}

let mainWindow: BrowserWindow | null = null
let store: JsonSettingsStore
let logDir = ''
let clawRuntime: ClawRuntime | null = null
let scheduleRuntime: ScheduleRuntime | null = null
let telegramRuntime: TelegramRuntime | null = null
let dataAnalysisBackendManager: DataAnalysisBackendManager | null = null
let terminalPtyController: TerminalPtyIpcController | null = null
const retainedTerminalRuntimeProtectedRoots = new Set<string>()
let managedRuntimesStoppedForQuit = false
let managedRuntimesStopPromise: Promise<void> | null = null
let appBehavior: AppBehaviorConfigV1 = normalizeAppBehaviorSettings()
let tray: Tray | null = null
let trayMenu: Menu | null = null
let trayMenuOpenPromise: Promise<void> | null = null
let loadTrayProviderObservation: (() => Promise<TrayProviderObservation | null>) | null = null
let isQuitting = false
let nativeOfficeQuitPending = false
const writeShutdown = createWriteShutdownCoordinator({
  ipc: ipcMain,
  prepareNative: prepareNativeOfficeQuit,
  cancelNative: cancelNativeOfficeQuit,
  notifyBlocked: async window => {
    await dialog.showMessageBox(window, { type: 'warning', message: '文档尚未保存，已取消关闭',
      detail: '草稿仍保留在当前窗口。请完成输入、导出或处理保存提示后重试。', buttons: ['返回文档'], noLink: true })
  }
})
let closeWindowPromptOpen = false
let pendingDeepLinkThreadId = ''
let desktopStartupBarrierComplete = false

type GuiUpdaterModule = typeof import('./gui-updater')

let guiUpdaterModulePromise: Promise<GuiUpdaterModule> | null = null
let guiUpdaterInitialized = false

function emitClawChannelActivity(payload: { channelId: string; threadId: string }): void {
  if (!mainWindow || mainWindow.isDestroyed()) return
  mainWindow.webContents.send('claw:channel-activity', payload)
}

async function stopManagedRuntimesForQuit(): Promise<void> {
  if (managedRuntimesStoppedForQuit) return
  await stopManagedRuntimes()
  managedRuntimesStoppedForQuit = true
}

async function stopManagedRuntimes(): Promise<void> {
  clearWriteRetrievalCache()
  if (!managedRuntimesStopPromise) {
    managedRuntimesStopPromise = (async () => {
      terminalPtyController?.disposeAll()
      scheduleRuntime?.stop()
      clawRuntime?.stop()
      telegramRuntime?.stop()
      const dataAnalysisStop = dataAnalysisBackendManager?.stopAndWait() ?? Promise.resolve()
      stopWeixinBridgeRuntime()
      const results = await Promise.allSettled([dataAnalysisStop, analytixRuntimeAdapter.stopAndWait()])
      const computerUseAgentStop = await stopOwnedOpenComputerUseAppAgentV1({
        command: resolveBuiltInComputerUseMcpCommand(),
        appStartedAtMs: startupTraceStart,
        baseline: await computerUseAgentBaselinePromise
      })
      if (!['terminated', 'not-present'].includes(computerUseAgentStop.status)) {
        publicConsoleWarn('computer-use', 'The current Analytix Computer Use app agent was not stopped.', {
          code: computerUseAgentStop.code
        })
      }
      const failure = results.find((result): result is PromiseRejectedResult => result.status === 'rejected')
      if (failure) throw failure.reason
    })().finally(() => {
      managedRuntimesStopPromise = null
    })
  }
  return managedRuntimesStopPromise
}

async function loadGuiUpdaterModule(): Promise<GuiUpdaterModule> {
  if (!guiUpdaterModulePromise) {
    guiUpdaterModulePromise = import('./gui-updater')
      .then((module) => {
        if (!guiUpdaterInitialized) {
          module.initializeGuiUpdater(
            () => mainWindow,
            async () => (await store.load()).guiUpdate.channel,
            stopManagedRuntimesForQuit,
            async () => (await store.load()).locale,
            desktopExternalState.isolated
          )
          guiUpdaterInitialized = true
        }
        return module
      })
      .catch((error) => {
        guiUpdaterModulePromise = null
        throw error
      })
  }
  return guiUpdaterModulePromise
}

async function readGuiUpdateState(): Promise<GuiUpdateState> {
  if (!guiUpdaterModulePromise) return { status: 'idle' }
  try {
    const module = await loadGuiUpdaterModule()
    return module.getGuiUpdateState()
  } catch (error) {
    return {
      status: 'error',
      message: 'Unable to read the update status.',
      code: 'unknown'
    }
  }
}


function installDevPreviewWebviewGuards(): void {
  app.on('web-contents-created', (_, contents) => {
    contents.on('did-attach-webview', (_event, guest) => registerBrowserGuest(contents, guest))
    contents.on('will-attach-webview', (event, webPreferences, params) => {
      const src = typeof params.src === 'string' ? params.src : ''
      // Prototype embeds are file:// pages the renderer authorized through
      // write:authorize-prototype right before attaching.
      if (!isAllowedDevPreviewUrl(src) && !isAuthorizedPrototypeFileUrl(src)) {
        event.preventDefault()
        return
      }

      delete webPreferences.preload
      delete (webPreferences as { preloadURL?: string }).preloadURL
      webPreferences.nodeIntegration = false
      webPreferences.contextIsolation = true
      webPreferences.sandbox = true
      webPreferences.webSecurity = true
      webPreferences.allowRunningInsecureContent = false
    })

    contents.on('will-navigate', (event, navigationUrl) => {
      if (contents.getType() !== 'webview') return
      if (!isAllowedDevPreviewUrl(navigationUrl)) event.preventDefault()
    })

    contents.setWindowOpenHandler(({ url }) => {
      if (contents.getType() !== 'webview') return { action: 'allow' }
      return isAllowedDevPreviewUrl(url) ? { action: 'allow' } : { action: 'deny' }
    })
  })
}

function isTrustedDesktopRenderer(event: IpcMainInvokeEvent): boolean {
  const frame = event.senderFrame
  if (
    !frame ||
    frame !== event.sender.mainFrame ||
    event.sender.isDestroyed()
  ) {
    return false
  }
  const owner = BrowserWindow.fromWebContents(event.sender)
  if (!owner || owner.isDestroyed() || owner.webContents !== event.sender) return false
  let senderUrl = ''
  try {
    senderUrl = event.sender.getURL()
  } catch {
    return false
  }
  if (app.isPackaged) {
    return isPackagedRendererNavigationURL(frame.url) &&
      isPackagedRendererNavigationURL(senderUrl)
  }
  const expected = devServerHintUrl(false)
  if (!expected) return false
  try {
    const expectedUrl = new URL(expected)
    const frameUrl = new URL(frame.url)
    const contentsUrl = new URL(senderUrl)
    return frameUrl.origin === expectedUrl.origin &&
      contentsUrl.origin === expectedUrl.origin &&
      frameUrl.pathname === expectedUrl.pathname &&
      contentsUrl.pathname === expectedUrl.pathname
  } catch {
    return false
  }
}

function retainTerminalRuntimeProtectedRoots(settings: AppSettingsV1): void {
  const dataDir = resolveAnalytixDataDir(getAnalytixRuntimeSettings(settings))
  retainedTerminalRuntimeProtectedRoots.add(join(dataDir, 'private'))
  retainedTerminalRuntimeProtectedRoots.add(join(dataDir, 'child-runs'))
}

function silentSettingsPatchCanPersistWithoutRuntimeApply(
  partial: AppSettingsPatch
): boolean {
  if (Object.keys(partial).some((key) => key !== 'runtime' && key !== 'write')) {
    return false
  }
  if (
    Object.keys(partial.runtime ?? {}).some((key) =>
      key !== 'model' &&
      key !== 'providerId' &&
      key !== 'modelCapabilityProbes'
    )
  ) {
    return false
  }
  return !Object.keys(partial.write ?? {}).some((key) => key !== 'typography')
}


const appIconSource = appIdentityIconSource(process.platform, analytixAppIconPng, analytixMacIconPng)
const appIcon = createAppIcon(appIconSource)
traceStartup('app icon loaded', { source: appIconSource.startsWith('data:') ? 'data-url' : 'path' })
const gotSingleInstanceLock =
  legacyMigrationReady && (runningClawScheduleMcpServer || app.requestSingleInstanceLock())
traceStartup('single instance lock checked', {
  gotSingleInstanceLock,
  skippedForClawScheduleMcpServer: runningClawScheduleMcpServer
})
if (!gotSingleInstanceLock) {
  app.quit()
}

pendingDeepLinkThreadId = threadIdFromArgv(process.argv)
const initialNativeOAuthCallback = nativeOAuthCallbackFromArgv(process.argv)
if (initialNativeOAuthCallback) nativeOAuthCallbackRouter.route(initialNativeOAuthCallback)
app.on('open-url', (event, url) => {
  event.preventDefault()
  if (nativeOAuthCallbackRouter.route(url)) return
  const threadId = parseAnalytixThreadDeepLink(url)
  if (threadId) openThreadDeepLink(threadId)
})

function trayTooltipLabel(_locale: AppSettingsV1['locale']): string {
  return APP_DISPLAY_NAME
}

function windowCloseLabels(locale: AppSettingsV1['locale']): {
  title: string
  message: string
  detail: string
  minimizeToTray: string
  quit: string
  cancel: string
  remember: string
} {
  if (locale === 'zh') {
    return {
      title: '关闭窗口',
      message: '关闭窗口时要怎么处理？',
      detail: '选择最小化到托盘时，Analytix 会继续在后台运行；选择退出应用会结束后台服务。',
      minimizeToTray: '最小化到托盘',
      quit: '退出应用',
      cancel: '取消',
      remember: '记住我的选择，不再询问'
    }
  }
  return {
    title: 'Close window',
    message: 'What should Analytix do when this window closes?',
    detail: 'Minimize to tray keeps Analytix running in the background. Quit app stops the background service.',
    minimizeToTray: 'Minimize to tray',
    quit: 'Quit app',
    cancel: 'Cancel',
    remember: 'Remember my choice and do not ask again'
  }
}

function shouldStartHidden(settings: AppSettingsV1): boolean {
  return (
    process.platform === 'win32' &&
    settings.appBehavior.openAtLogin &&
    settings.appBehavior.startMinimized &&
    process.argv.includes(HIDDEN_START_ARG)
  )
}

function syncLoginItemSettings(settings: AppSettingsV1): void {
  const behavior = settings.appBehavior
  try {
    applyDesktopLoginItemSettings(desktopExternalState, {
      platform: process.platform,
      openAtLogin: behavior.openAtLogin,
      startMinimized: behavior.startMinimized,
      hiddenStartArg: HIDDEN_START_ARG,
      setLoginItemSettings: (loginItemSettings) => app.setLoginItemSettings(loginItemSettings)
    })
  } catch (error) {
    const diagnostic = runtimeErrorPublicDiagnosticV1(error)
    publicConsoleWarn('desktop-behavior', 'Failed to update login item settings.', diagnostic)
    logWarn('desktop-behavior', 'Failed to update login item settings.', diagnostic)
  }
}

function revealMainWindow(): void {
  if (!mainWindow || mainWindow.isDestroyed()) {
    createWindow()
    return
  }
  if (mainWindow.isMinimized()) mainWindow.restore()
  mainWindow.show()
  mainWindow.focus()
}

function threadIdFromArgv(argv: readonly string[]): string {
  for (const arg of argv) {
    const threadId = parseAnalytixThreadDeepLink(arg)
    if (threadId) return threadId
  }
  return ''
}

function openThreadDeepLink(threadId: string): void {
  const normalizedThreadId = threadId.trim()
  if (!isValidAnalytixThreadId(normalizedThreadId)) return
  if (!app.isReady() || !gotSingleInstanceLock || !desktopStartupBarrierComplete) {
    pendingDeepLinkThreadId = normalizedThreadId
    return
  }
  openThreadInNewWindow(normalizedThreadId)
}

function quitFromTray(): void {
  isQuitting = true
  app.quit()
}

function createTrayMenu(
  settings: AppSettingsV1,
  threads: TrayThreadSummary[],
  providerObservation: TrayProviderObservation | null = null
): Menu {
  return Menu.buildFromTemplate(buildTrayMenuTemplate({
    locale: settings.locale,
    threads,
    providerObservation,
    actions: {
      openThread: openThreadInNewWindow,
      newChat: () => createWindow({ primary: false }),
      openApp: revealMainWindow,
      quit: quitFromTray
    }
  }))
}

async function loadTrayThreads(settings: AppSettingsV1): Promise<TrayThreadSummary[]> {
  try {
    const response = await fetch(`${getRuntimeBaseUrlForSettings(settings)}/v1/threads?limit=20`, {
      headers: runtimeAuthHeaders(settings),
      signal: AbortSignal.timeout(1_000)
    })
    return response.ok ? parseTrayThreads(await response.text()) : []
  } catch (error) {
    logWarn('tray', 'Failed to load tray sessions.', runtimeErrorPublicDiagnosticV1(error))
    return []
  }
}

async function loadCurrentTrayProviderObservation(): Promise<TrayProviderObservation | null> {
  try {
    return await loadTrayProviderObservation?.() ?? null
  } catch (error) {
    logWarn('tray', 'Failed to load Provider account observation.', runtimeErrorPublicDiagnosticV1(error))
    return null
  }
}

function showTrayMenu(): void {
  if (!tray || trayMenuOpenPromise) return
  const currentTray = tray
  trayMenuOpenPromise = (async () => {
    const settings = await store.load()
    const [threads, providerObservation] = await Promise.all([
      loadTrayThreads(settings),
      loadCurrentTrayProviderObservation()
    ])
    if (currentTray.isDestroyed()) return
    trayMenu = createTrayMenu(settings, threads, providerObservation)
    currentTray.popUpContextMenu(trayMenu)
  })().finally(() => {
    trayMenuOpenPromise = null
  })
}

function syncTray(settings: AppSettingsV1): void {
  appBehavior = settings.appBehavior
  if (appBehavior.closeAction === 'quit') {
    if (tray) {
      tray.destroy()
      tray = null
      trayMenu = null
    }
    return
  }

  if (!tray) {
    // Tray / macOS menu bar uses the same app identity icon as the packaged app,
    // resized to the platform tray target so runtime chrome stays visually unified.
    const traySource = prepareTrayIcon(appIcon)
    tray = new Tray(traySource.isEmpty() ? nativeImage.createEmpty() : traySource)
    tray.on('click', showTrayMenu)
    tray.on('double-click', revealMainWindow)
    tray.on('right-click', showTrayMenu)
  }

  tray.setToolTip(trayTooltipLabel(settings.locale))
  trayMenu = createTrayMenu(settings, [])
  tray.setContextMenu(null)
}

async function saveWindowCloseActionPreference(closeAction: WindowCloseAction): Promise<void> {
  const saved = await store.patch({ appBehavior: { closeAction } })
  syncLoginItemSettings(saved)
  syncTray(saved)
}

async function promptWindowCloseAction(window: BrowserWindow): Promise<void> {
  if (closeWindowPromptOpen || window.isDestroyed()) return
  closeWindowPromptOpen = true
  try {
    const settings = await store.load()
    const labels = windowCloseLabels(settings.locale)
    const result = await dialog.showMessageBox(window, {
      type: 'question',
      title: labels.title,
      message: labels.message,
      detail: labels.detail,
      buttons: [labels.minimizeToTray, labels.quit, labels.cancel],
      defaultId: 0,
      cancelId: 2,
      noLink: true,
      checkboxLabel: labels.remember,
      checkboxChecked: false
    })
    if (result.response === 0) {
      if (result.checkboxChecked) {
        await saveWindowCloseActionPreference('tray')
      }
      window.hide()
      return
    }
    if (result.response === 1) {
      if (result.checkboxChecked) {
        await saveWindowCloseActionPreference('quit')
      }
      isQuitting = true
      app.quit()
    }
  } catch (error) {
    const diagnostic = runtimeErrorPublicDiagnosticV1(error)
    publicConsoleWarn('desktop-behavior', 'Failed to handle close-window prompt.', diagnostic)
    logWarn('desktop-behavior', 'Failed to handle close-window prompt.', diagnostic)
  } finally {
    closeWindowPromptOpen = false
  }
}

function handleMainWindowClose(window: BrowserWindow, event: Electron.Event): void {
  if (isQuitting) return
  if (appBehavior.closeAction === 'quit') return

  event.preventDefault()
  if (appBehavior.closeAction === 'tray') {
    window.hide()
    return
  }
  void promptWindowCloseAction(window)
}

function normalizeNotificationText(raw: string | undefined, fallback: string, maxLength: number): string {
  const value = typeof raw === 'string' && raw.trim() ? raw.trim() : fallback
  return value.length > maxLength ? `${value.slice(0, maxLength - 1)}…` : value
}

type TurnCompleteNotificationPayload = {
  threadId?: string
  title?: string
  body?: string
}

async function showTurnCompleteNotification(
  payload: TurnCompleteNotificationPayload
): Promise<{ ok: true; shown: boolean; reason?: string } | { ok: false; message: string }> {
  const settings = await store.load()
  if (!settings.notifications.turnComplete) {
    return { ok: true, shown: false, reason: 'disabled' }
  }
  if (!Notification.isSupported()) {
    return { ok: true, shown: false, reason: 'unsupported' }
  }

  const title = normalizeNotificationText(payload.title, 'Analytix', 80)
  const body = normalizeNotificationText(payload.body, 'Conversation complete.', 180)

  try {
    const notification = new Notification({
      title,
      body,
      icon: appIcon.isEmpty() ? undefined : appIcon
    })
    notification.on('click', () => {
      revealMainWindow()
    })
    notification.show()
    return { ok: true, shown: true }
  } catch (e) {
    logError('notification', 'Failed to show turn completion notification', {
      ...runtimeErrorPublicDiagnosticV1(e)
    })
    return { ok: false, message: 'The desktop notification could not be shown.' }
  }
}

async function probeThreadApi(settings: AppSettingsV1): Promise<
  | { ok: true }
  | { ok: false; error: string; message: string }
> {
  const base = getRuntimeBaseUrlForSettings(settings)
  const headers = runtimeAuthHeaders(settings)
  headers.set('Accept', 'application/json')

  try {
    const res = await fetch(`${base}/v1/threads?limit=1`, {
      headers,
      signal: AbortSignal.timeout(2_000)
    })
    if (res.ok) return { ok: true }
    const info = parseRuntimeErrorBody(
      await res.text(),
      'The local runtime returned an unexpected error.'
    )
    if (res.status === 401 && /bearer token required/i.test(info.message)) {
      return {
        ok: false,
        error: 'runtime_auth_required',
        message: 'The local runtime requires a bearer token for thread APIs.'
      }
    }
    return {
      ok: false,
      error: info.code === 'unknown' ? 'runtime_request_failed' : info.code,
      message: info.message
    }
  } catch (e) {
    return {
      ok: false,
      error: 'fetch_failed',
      message: 'The Analytix runtime is unavailable.'
    }
  }
}

async function waitForAnalytixHealth(settings: AppSettingsV1, timeoutMs: number): Promise<boolean> {
  const base = getRuntimeBaseUrlForSettings(settings)
  const deadline = Date.now() + timeoutMs

  while (Date.now() <= deadline) {
    try {
      const remaining = Math.max(1, deadline - Date.now())
      const res = await fetch(`${base}/health`, {
        headers: runtimeAuthHeaders(settings),
        signal: AbortSignal.timeout(Math.max(250, Math.min(1_000, remaining)))
      })
      if (res.ok && isAnalytixHealthResponseBody(await res.text())) return true
    } catch {
      /* retry until the deadline */
    }
    await sleep(150)
  }

  return false
}

async function sleepWithAbort(ms: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted || ms <= 0) return
  await new Promise<void>((resolve) => {
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    const onAbort = (): void => {
      clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      resolve()
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

let runtimeEnsurePromise: Promise<AppSettingsV1> | null = null
let runtimeEnsureFingerprint: string | null = null
let runtimeRestartPromise: Promise<void> | null = null
let runtimeSettingsApplyPromise: Promise<void> | null = null
let lastAppliedSettings: AppSettingsV1 | null = null

const RUNTIME_WATCHDOG_INTERVAL_MS = 30_000
const RUNTIME_WATCHDOG_FAILURE_THRESHOLD = 3
const runtimeRestartBudget = new RestartBudget({ windowMs: 60_000, maxRestarts: 3 })
let lastRuntimeStatus: RuntimeStatusPublicV1 | null = null
let supervisedRestartInFlight = false
let runtimeWatchdogTimer: NodeJS.Timeout | null = null
let runtimeWatchdogFailures = 0
let runtimeWatchdogTickInFlight = false

function publishRuntimeStatus(input: RuntimeStatusPublicV1Input): void {
  const full = createRuntimeStatusPublicV1(input)
  lastRuntimeStatus = full
  const { message: _message, at: _at, ...publicDiagnostic } = full
  logWarn('runtime-status', full.code, publicDiagnostic)
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) win.webContents.send('runtime:status', full)
  }
}

/** Record a healthy runtime: reset the crash budget and watchdog, announce recovery. */
function noteRuntimeHealthy(
  source: Exclude<RuntimeStatusPublicSourceV1, 'hub-account' | 'browser-preview'>
): void {
  runtimeRestartBudget.reset()
  runtimeWatchdogFailures = 0
  startRuntimeWatchdog()
  if (source !== 'settings-apply-rollback' && lastRuntimeStatus?.state !== 'running') {
    publishRuntimeStatus({ code: 'runtime_ready', source })
  }
}

function handleUnexpectedAnalytixExit(info: AnalytixUnexpectedExitInfo): void {
  terminalPtyController?.disposeAll()
  void superviseAnalytixCrash(info).catch((error: unknown) => {
    logError(
      'analytix-supervisor',
      'supervised restart crashed',
      runtimeErrorPublicDiagnosticV1(error)
    )
  })
}

async function superviseAnalytixCrash(info: AnalytixUnexpectedExitInfo): Promise<void> {
  if (managedRuntimesStoppedForQuit || isQuitting) return
  publishRuntimeStatus({
    code: 'supervisor_unexpected_exit',
    source: 'supervisor',
    stderrBytes: info.stderrBytes,
    stderrSha256: info.stderrSha256
  })
  if (supervisedRestartInFlight) return
  supervisedRestartInFlight = true
  try {
    const settings = await store.load()
    const runtime = getAnalytixRuntimeSettings(settings)
    if (!runtime.autoStart) {
      publishRuntimeStatus({
        code: 'supervisor_restart_unavailable',
        source: 'supervisor'
      })
      return
    }
    let lastErrorDiagnostic: RuntimeErrorPublicDiagnosticV1 | undefined
    for (;;) {
      if (managedRuntimesStoppedForQuit || isQuitting) return
      const verdict = runtimeRestartBudget.note()
      if (!verdict.allowed) {
        publishRuntimeStatus({
          code: 'supervisor_restart_exhausted',
          source: 'supervisor',
          stderrBytes: info.stderrBytes,
          stderrSha256: info.stderrSha256,
          ...lastErrorDiagnostic
        })
        return
      }
      publishRuntimeStatus({
        code: 'supervisor_restart_scheduled',
        source: 'supervisor',
        attempt: verdict.attempt,
        maxAttempts: 3
      })
      await new Promise((resolve) => setTimeout(resolve, verdict.delayMs))
      try {
        await ensureRuntime(await store.load())
        noteRuntimeHealthy('supervisor')
        return
      } catch (error) {
        lastErrorDiagnostic = runtimeErrorPublicDiagnosticV1(error)
        logWarn('analytix-supervisor', 'automatic restart attempt failed', {
          attempt: verdict.attempt,
          maxAttempts: 3,
          ...lastErrorDiagnostic
        })
      }
    }
  } finally {
    supervisedRestartInFlight = false
  }
}

function startRuntimeWatchdog(): void {
  if (runtimeWatchdogTimer) return
  const timer = setInterval(() => {
    void runtimeWatchdogTick().catch((error: unknown) => {
      logWarn('analytix-watchdog', 'watchdog tick failed', runtimeErrorPublicDiagnosticV1(error))
    })
  }, RUNTIME_WATCHDOG_INTERVAL_MS)
  timer.unref()
  runtimeWatchdogTimer = timer
}

function stopRuntimeWatchdog(): void {
  if (runtimeWatchdogTimer) {
    clearInterval(runtimeWatchdogTimer)
    runtimeWatchdogTimer = null
  }
}

/**
 * Post-startup liveness check for the GUI-managed analytix child: the boot
 * probe only covers launch, so a runtime that hangs later (blocked
 * event loop, sqlite lock) would otherwise stay dead until the user
 * restarts the app.
 */
async function runtimeWatchdogTick(): Promise<void> {
  if (runtimeWatchdogTickInFlight) return
  if (managedRuntimesStoppedForQuit || isQuitting) return
  if (
    supervisedRestartInFlight ||
    runtimeRestartPromise ||
    runtimeSettingsApplyPromise ||
    runtimeEnsurePromise
  ) {
    return
  }
  if (!analytixRuntimeAdapter.isChildRunning()) return
  runtimeWatchdogTickInFlight = true
  try {
    const settings = await store.load()
    const healthy = await waitForAnalytixHealth(settings, 5_000)
    if (healthy) {
      runtimeWatchdogFailures = 0
      return
    }
    runtimeWatchdogFailures += 1
    logWarn(
      'analytix-watchdog',
      `health probe failed (${runtimeWatchdogFailures}/${RUNTIME_WATCHDOG_FAILURE_THRESHOLD})`
    )
    if (runtimeWatchdogFailures < RUNTIME_WATCHDOG_FAILURE_THRESHOLD) return
    runtimeWatchdogFailures = 0
    publishRuntimeStatus({
      code: 'watchdog_restart_scheduled',
      source: 'watchdog'
    })
    try {
      await restartRuntime(settings)
      noteRuntimeHealthy('watchdog')
    } catch (error) {
      publishRuntimeStatus({
        code: 'watchdog_restart_failed',
        source: 'watchdog',
        ...runtimeErrorPublicDiagnosticV1(error)
      })
    }
  } finally {
    runtimeWatchdogTickInFlight = false
  }
}

function queueRuntimeSettingsApply(
  prev: AppSettingsV1,
  next: AppSettingsV1
): Promise<void> | null {
  // Always update the prev/next anchor so a later task diffs against
  // the settings that were actually applied last, not against the
  // original `prev` captured when this call was queued.
  const anchor = lastAppliedSettings ?? prev
  lastAppliedSettings = next
  const startupConfigChanged = runtimeStartupConfigChanged(anchor, next)
  if (!startupConfigChanged) return runtimeSettingsApplyPromise

  const previousTask = runtimeSettingsApplyPromise ?? Promise.resolve()
  const task = previousTask
    .catch(() => undefined)
    .then(async () => {
      const current = lastAppliedSettings ?? next
      await restartManagedRuntimeForSettingsChange(anchor, current)
    })
    .catch((error: unknown) => {
      logWarn(
        'settings-apply',
        'Failed to apply Analytix runtime settings in background',
        runtimeErrorPublicDiagnosticV1(error)
      )
    })
    .finally(() => {
      if (runtimeSettingsApplyPromise === task) {
        runtimeSettingsApplyPromise = null
      }
    })

  runtimeSettingsApplyPromise = task
  return task
}

function queueRuntimeMcpConfigApply(settings: AppSettingsV1): void {
  lastAppliedSettings = settings

  const previousTask = runtimeSettingsApplyPromise ?? Promise.resolve()
  const task = previousTask
    .catch(() => undefined)
    .then(async () => {
      const current = lastAppliedSettings ?? settings
      await restartManagedRuntimeForMcpConfigChange(current)
    })
    .catch((error: unknown) => {
      logWarn(
        'mcp-config',
        'Failed to apply Analytix MCP config change in background',
        runtimeErrorPublicDiagnosticV1(error)
      )
    })
    .finally(() => {
      if (runtimeSettingsApplyPromise === task) {
        runtimeSettingsApplyPromise = null
      }
    })

  runtimeSettingsApplyPromise = task
}

async function waitForQueuedRuntimeSettingsApply(): Promise<void> {
  if (!runtimeSettingsApplyPromise) return
  await runtimeSettingsApplyPromise
}

/**
 * Build a stable fingerprint of the settings that affect the
 * Analytix runtime so that `ensureRuntime` can debounce on real
 * state instead of on a single in-flight promise. Without this,
 * a fresh call that arrives while a failing ensure is still pending
 * would re-throw the old error.
 */
function runtimeFingerprint(settings: AppSettingsV1): string {
  return buildAnalytixRuntimeSettingsKey(settings)
}

async function ensureRuntime(settings: AppSettingsV1): Promise<AppSettingsV1> {
  const restart = runtimeRestartPromise
  if (restart) {
    try {
      await restart
      return store.load()
    } catch {
      /* fall through to a normal ensure so callers see the latest state */
    }
  }
  const fingerprint = runtimeFingerprint(settings)
  const pending = runtimeEnsurePromise
  const pendingFingerprint = runtimeEnsureFingerprint
  if (pending) {
    // Wait for the in-flight ensure, then re-evaluate against the
    // fingerprint so callers don't inherit a stale result.
    try {
      const ensuredSettings = await pending
      if (pendingFingerprint === fingerprint) return ensuredSettings
    } catch {
      /* fall through to retry with the current settings */
    }
  }
  const task = ensureRuntimeOnce(settings)
  let trackedTask: Promise<AppSettingsV1>
  trackedTask = task.finally(() => {
    if (runtimeEnsurePromise === trackedTask) {
      runtimeEnsurePromise = null
      runtimeEnsureFingerprint = null
    }
  })
  runtimeEnsurePromise = trackedTask
  runtimeEnsureFingerprint = fingerprint
  try {
    return await trackedTask
  } finally {
    /* cleanup runs via the .finally above */
  }
}

async function ensureRuntimeOnce(settings: AppSettingsV1): Promise<AppSettingsV1> {
  await waitForQueuedRuntimeSettingsApply()
  return ensureAnalytixRuntime(settings)
}

async function resolveManagedAnalytixLaunchSettings(
  settings: AppSettingsV1,
  source: string
): Promise<AppSettingsV1> {
  const runtime = getAnalytixRuntimeSettings(settings)
  const resolved = await analytixRuntimeAdapter.resolveAvailablePort(runtime.port)
  if (!resolved.changed) return settings

  const next = await store.patch({ runtime: { port: resolved.port } })
  lastAppliedSettings = next
  logWarn(source, `Analytix port ${runtime.port} is unavailable; using ${resolved.port} for the managed runtime`, {
    previousPort: runtime.port,
    port: resolved.port
  })
  return next
}

async function ensureAnalytixRuntime(settings: AppSettingsV1): Promise<AppSettingsV1> {
  const runtime = getAnalytixRuntimeSettings(settings)
  const adapter = analytixRuntimeAdapter

  if (runtime.autoStart && isManagedGoRuntimeBackend() && !adapter.isChildRunning()) {
    await adapter.reclaimPort(runtime.port)
  }

  const healthy = await waitForAnalytixHealth(settings, 2_000)
  if (healthy) {
    const threadApi = await probeThreadApi(settings)
    if (threadApi.ok) {
      noteRuntimeHealthy('ensure')
      return settings
    }
    throw runtimeJsonError(threadApi.error, threadApi.message)
  }

  if (!runtime.autoStart) {
    throw runtimeJsonError(
      'runtime_offline',
      'Analytix is offline. Enable automatic startup in Settings, or start `analytix serve` manually.'
    )
  }

  const launchSettings = await resolveManagedAnalytixLaunchSettings(settings, 'runtime-start')
  publishRuntimeStatus({ code: 'runtime_starting', source: 'ensure' })
  try {
    await adapter.ensureRunning(launchSettings)
  } catch (e) {
    publicConsoleError(
      'runtime',
      'Failed to start Analytix.',
      runtimeErrorPublicDiagnosticV1(e)
    )
    throw e
  }
  const started = await waitForAnalytixHealth(launchSettings, 20_000)
  if (!started) {
    throw runtimeJsonError(
      'runtime_unhealthy',
      'Analytix did not become healthy after launch.'
    )
  }

  const threadApi = await probeThreadApi(launchSettings)
  if (!threadApi.ok) {
    throw runtimeJsonError(threadApi.error, threadApi.message)
  }
  noteRuntimeHealthy('ensure')
  return launchSettings
}

async function restartRuntime(settings: AppSettingsV1): Promise<void> {
  terminalPtyController?.disposeAll()
  if (runtimeRestartPromise) return runtimeRestartPromise
  const pendingEnsure = runtimeEnsurePromise
  runtimeEnsurePromise = null
  runtimeEnsureFingerprint = null
  const task = restartAfterPendingEnsure(pendingEnsure, () => restartRuntimeOnce(settings))
    .finally(() => {
      if (runtimeRestartPromise === task) {
        runtimeRestartPromise = null
      }
    })
  runtimeRestartPromise = task
  return task
}

async function restartRuntimeOnce(settings: AppSettingsV1): Promise<void> {
  await waitForQueuedRuntimeSettingsApply()
  const runtime = getAnalytixRuntimeSettings(settings)

  if (!runtime.autoStart) {
    throw runtimeJsonError(
      'runtime_offline',
      'Analytix is offline. Enable automatic startup in Settings, or start `analytix serve` manually.'
    )
  }

  const adapter = analytixRuntimeAdapter
  publishRuntimeStatus({ code: 'runtime_starting', source: 'restart' })
  await adapter.stopAndWait()
  const launchSettings = await resolveManagedAnalytixLaunchSettings(settings, 'runtime-restart')

  try {
    await adapter.ensureRunning(launchSettings)
  } catch (e) {
    publicConsoleError(
      'runtime',
      'Failed to restart Analytix.',
      runtimeErrorPublicDiagnosticV1(e)
    )
    throw e
  }

  const healthy = await waitForAnalytixHealth(launchSettings, 20_000)
  if (!healthy) {
    throw runtimeJsonError(
      'runtime_unhealthy',
      'Analytix did not become healthy after restart.'
    )
  }

  const threadApi = await probeThreadApi(launchSettings)
  if (!threadApi.ok) {
    throw runtimeJsonError(threadApi.error, threadApi.message)
  }
  noteRuntimeHealthy('restart')
}

function createWindow(options: { suppressInitialShow?: boolean; initialThreadId?: string; primary?: boolean } = {}): BrowserWindow | null {
  // Successful shutdown holds this admission closed while Core stops.
  if (!writeShutdown.canCreateWindow()) return null
  traceStartup('createWindow:start')
  const preloadPath = resolvePreloadPath()
  const usesDesktopTitleBar = process.platform === 'win32' || process.platform === 'linux'
  const appWindow = new BrowserWindow({
    width: 1280,
    height: 840,
    minWidth: 960,
    minHeight: 640,
    title: APP_DISPLAY_NAME,
    icon: appIcon.isEmpty() ? undefined : appIcon,
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : usesDesktopTitleBar ? 'hidden' : 'default',
    trafficLightPosition: process.platform === 'darwin' ? MACOS_TRAFFIC_LIGHT_POSITION : undefined,
    acceptFirstMouse: true,
    autoHideMenuBar: usesDesktopTitleBar,
    paintWhenInitiallyHidden: true,
    show: false,
    webPreferences: {
      preload: preloadPath,
      contextIsolation: true,
      sandbox: true,
      webviewTag: true
    }
  })
  const primary = options.primary !== false
  if (primary) {
    mainWindow = appWindow
  }
  if (usesDesktopTitleBar) {
    appWindow.setMenu(null)
    appWindow.setMenuBarVisibility(false)
  }
  appWindow.webContents.on('preload-error', (_event, _preloadPath, error) => {
    const diagnostic = runtimeErrorPublicDiagnosticV1(error)
    publicConsoleError('preload', 'Failed to load preload.', diagnostic)
    logError('preload', 'Failed to load preload script', diagnostic)
  })
  appWindow.webContents.on('will-navigate', (event, navigationUrl) => {
    if (app.isPackaged && !isPackagedRendererNavigationURL(navigationUrl)) {
      event.preventDefault()
    }
  })
  let initialShowFallbackTimer: ReturnType<typeof setTimeout> | null = null
  const showWindow = (): void => {
    if (initialShowFallbackTimer) {
      clearTimeout(initialShowFallbackTimer)
      initialShowFallbackTimer = null
    }
    if (options.suppressInitialShow) return
    if (appWindow.isDestroyed() || appWindow.isVisible()) return
    appWindow.show()
  }
  const handleStartupSurfaceReady = (event: Electron.IpcMainEvent): void => {
    if (appWindow.isDestroyed() || event.sender !== appWindow.webContents) return
    traceStartup('window:startup-surface-ready')
    showWindow()
  }
  const disposeDataAnalysisRendererPrincipal = dataAnalysisBackendManager
    ? bindDataAnalysisRendererPrincipal(dataAnalysisBackendManager, appWindow)
    : () => undefined
  ipcMain.on(WINDOW_STARTUP_SURFACE_READY_CHANNEL, handleStartupSurfaceReady)
  appWindow.on('close', (event) => {
    if (appWindow.isDestroyed() || !primary) return
    handleMainWindowClose(appWindow, event)
  })
  writeShutdown.trackWindow(appWindow)
  appWindow.on('closed', () => {
    if (initialShowFallbackTimer) {
      clearTimeout(initialShowFallbackTimer)
      initialShowFallbackTimer = null
    }
    ipcMain.off(WINDOW_STARTUP_SURFACE_READY_CHANNEL, handleStartupSurfaceReady)
    disposeDataAnalysisRendererPrincipal()
    if (primary && mainWindow === appWindow) {
      clearWriteRetrievalCache()
      mainWindow = null
    }
  })
  const devUrl = devServerHintUrl(app.isPackaged)
  traceStartup('createWindow:load', { devUrl: devUrl ?? 'file' })
  if (devUrl) {
    const url = new URL(devUrl)
    if (options.initialThreadId) url.searchParams.set('threadId', options.initialThreadId)
    appWindow.loadURL(url.toString())
  } else {
    void appWindow.loadURL(packagedRendererURL(options.initialThreadId))
  }
  appWindow.once('ready-to-show', () => {
    traceStartup('window:ready-to-show')
  })
  appWindow.webContents.once('did-finish-load', () => {
    traceStartup('window:did-finish-load')
    if (lastRuntimeStatus && !appWindow.isDestroyed()) {
      appWindow.webContents.send('runtime:status', lastRuntimeStatus)
    }
  })
  appWindow.webContents.once('did-fail-load', () => {
    traceStartup('window:did-fail-load')
    showWindow()
  })
  initialShowFallbackTimer = setTimeout(() => {
    traceStartup('window:fallback-show-timeout')
    showWindow()
  }, 3500)
  return appWindow
}

function openThreadInNewWindow(threadId: string): void {
  const normalizedThreadId = threadId.trim()
  if (!normalizedThreadId) return
  createWindow({ initialThreadId: normalizedThreadId, primary: false })
}

/**
 * Stable equality for the Analytix runtime settings. Most fields are flat,
 * but GUI-managed capability options can be nested, so compare values
 * structurally while still surviving future field additions.
 */
function analytixRuntimeConfigChanged(prev: AppSettingsV1, next: AppSettingsV1): boolean {
  return !analytixRuntimeSettingsEqual(prev, next)
}

function runtimeStartupConfigChanged(prev: AppSettingsV1, next: AppSettingsV1): boolean {
  return analytixRuntimeConfigChanged(prev, next) || clawScheduleMcpSettingsChanged(prev, next)
}

/**
 * Reject runtime-affecting values that would persist a config analytix can
 * never boot with. Runs before the settings patch is written to disk.
 */
function validateRuntimeSettingsForApply(next: AppSettingsV1): string | null {
  const runtime = resolveAnalytixRuntimeSettings(next)
  if (!Number.isInteger(runtime.port) || runtime.port < 1 || runtime.port > 65_535) {
    return `Analytix port must be an integer between 1 and 65535 (got ${String(runtime.port)})`
  }
  const baseUrl = (runtime.baseUrl ?? '').trim()
  if (baseUrl) {
    try {
      const parsed = new URL(baseUrl)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        return `model base URL must use http(s): ${baseUrl}`
      }
    } catch {
      return `model base URL is not a valid URL: ${baseUrl}`
    }
  }
  return null
}

async function restartManagedRuntimeForSettingsChange(
  prev: AppSettingsV1,
  next: AppSettingsV1
): Promise<void> {
  if (!runtimeStartupConfigChanged(prev, next)) return

  const runtime = resolveAnalytixRuntimeSettings(next)
  const adapter = analytixRuntimeAdapter
  const wasRunning = adapter.isChildRunning()

  if (!wasRunning) return

  await waitForManagedRuntimeReadyBeforeStop(prev, 'settings-apply')
  terminalPtyController?.disposeAll()
  await adapter.stopAndWait()
  if (!runtime.autoStart) {
    publishRuntimeStatus({
      code: 'settings_apply_stopped',
      source: 'settings-apply'
    })
    return
  }

  publishRuntimeStatus({ code: 'settings_apply_restart_scheduled', source: 'settings-apply' })
  try {
    const launchSettings = await resolveManagedAnalytixLaunchSettings(next, 'settings-apply')
    await adapter.ensureRunning(launchSettings)
    const healthy = await waitForAnalytixHealth(launchSettings, 20_000)
    if (!healthy) {
      throw new Error('Analytix did not become healthy after the settings change')
    }
    noteRuntimeHealthy('settings-apply')
  } catch (e) {
    const diagnostic = runtimeErrorPublicDiagnosticV1(e)
    logWarn('settings-apply', 'Analytix restart failed after settings change', diagnostic)
    await rollbackRuntimeSettingsAfterFailedApply(prev, diagnostic)
  }
}

/**
 * A settings change took the runtime down and the new config cannot
 * boot. Restore the previous runtime/provider settings on disk (so the
 * next app launch is not bricked either) and bring analytix back up on the
 * last-known-good configuration.
 */
async function rollbackRuntimeSettingsAfterFailedApply(
  prev: AppSettingsV1,
  failureDiagnostic: RuntimeErrorPublicDiagnosticV1
): Promise<void> {
  const adapter = analytixRuntimeAdapter
  let base: AppSettingsV1 = prev
  try {
    base = await store.patch({
      runtime: getAnalytixRuntimeSettings(prev),
      provider: prev.provider
    })
    lastAppliedSettings = base
  } catch (error) {
    logWarn(
      'settings-apply',
      'failed to restore previous runtime settings on disk',
      runtimeErrorPublicDiagnosticV1(error)
    )
  }
  if (!getAnalytixRuntimeSettings(base).autoStart) {
    publishRuntimeStatus({
      code: 'settings_rollback_stopped',
      source: 'settings-apply-rollback',
      rolledBack: true,
      ...failureDiagnostic
    })
    return
  }
  try {
    const launchSettings = await resolveManagedAnalytixLaunchSettings(base, 'settings-apply-rollback')
    await adapter.ensureRunning(launchSettings)
    const healthy = await waitForAnalytixHealth(launchSettings, 20_000)
    if (!healthy) {
      throw new Error('previous configuration did not become healthy')
    }
    noteRuntimeHealthy('settings-apply-rollback')
    publishRuntimeStatus({
      code: 'settings_rollback_ready',
      source: 'settings-apply-rollback',
      rolledBack: true,
      ...failureDiagnostic
    })
  } catch (error) {
    publishRuntimeStatus({
      code: 'settings_rollback_failed',
      source: 'settings-apply-rollback',
      rolledBack: true,
      ...runtimeErrorPublicDiagnosticV1(error)
    })
  }
}

async function restartManagedRuntimeForMcpConfigChange(settings: AppSettingsV1): Promise<void> {
  const runtime = resolveAnalytixRuntimeSettings(settings)
  const adapter = analytixRuntimeAdapter
  const wasRunning = adapter.isChildRunning()

  if (!wasRunning) return
  await waitForManagedRuntimeReadyBeforeStop(settings, 'mcp-config')
  terminalPtyController?.disposeAll()
  await adapter.stopAndWait()
  if (!runtime.autoStart) return

  publishRuntimeStatus({ code: 'mcp_config_restart_scheduled', source: 'mcp-config' })
  try {
    const launchSettings = await resolveManagedAnalytixLaunchSettings(settings, 'mcp-config')
    await adapter.ensureRunning(launchSettings)
    const healthy = await waitForAnalytixHealth(launchSettings, 20_000)
    if (!healthy) {
      throw new Error('Analytix did not become healthy after the MCP config change')
    }
    noteRuntimeHealthy('mcp-config')
  } catch (e) {
    const diagnostic = runtimeErrorPublicDiagnosticV1(e)
    logWarn('mcp-config', 'Analytix restart failed after MCP config change', diagnostic)
    publishRuntimeStatus({
      code: 'mcp_config_restart_failed',
      source: 'mcp-config',
      ...diagnostic
    })
  }
}

async function waitForManagedRuntimeReadyBeforeStop(
  settings: AppSettingsV1,
  source: string
): Promise<void> {
  const healthy = await waitForAnalytixHealth(settings, 20_000)
  if (!healthy) {
    logWarn(source, 'Analytix did not become healthy before a managed restart; stopping it anyway')
    return
  }
  const idle = await waitForRuntimeTurnsIdle({ settings })
  if (idle === 'timeout') {
    logWarn(source, 'Analytix still has running turns after waiting; stopping it anyway')
  } else if (idle === 'unavailable') {
    logWarn(source, 'Could not verify Analytix turn idleness before a managed restart; stopping it anyway')
  }
}

async function runtimeRequest(
  settings: AppSettingsV1,
  pathAndQuery: string,
  init: { method?: string; body?: string; headers?: Record<string, string> }
): Promise<{ ok: boolean; status: number; body: string }> {
  try {
    return await runtimeRequestViaHost(settings, pathAndQuery, init, ensureRuntime)
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e)
    const publicPath = pathAndQuery.split('?', 1)[0] || '/'
    logError(
      'runtime-request',
      `HTTP request to ${publicPath} failed`,
      runtimeErrorPublicDiagnosticV1(e)
    )
    const parsed = parseRuntimeErrorBody(message, 'The Analytix runtime is unavailable.')
    if (parsed.code !== 'unknown') {
      return runtimeFailure(parsed.code, parsed.message)
    }
    return runtimeFailure('fetch_failed', 'The Analytix runtime is unavailable.')
  }
}

/**
 * Typed local display is deliberately kept outside runtimeRequest: its
 * response may contain source-exact displayValue bytes and must not cross the
 * generic public sanitizer or generic runtime logging path.
 */
async function localDisplayRuntimeRequest(
  settings: AppSettingsV1,
  path: string,
  body: string,
  signal?: AbortSignal
): Promise<{ ok: boolean; status: number; body: string }> {
  if (!isLocalDisplayRuntimePathV1(path)) {
    return { ok: false, status: 400, body: '' }
  }
  try {
    if (signal?.aborted) return { ok: false, status: 499, body: '' }
    const ensuredSettings = await ensureRuntime(settings)
    if (signal?.aborted) return { ok: false, status: 499, body: '' }
    const requestSettings = ensuredSettings ?? settings
    const runtimeAuthority = captureCurrentFinalPublicationAuthorityPin()
    if (!runtimeAuthority) return { ok: false, status: 503, body: '' }
    const baseUrl = getRuntimeBaseUrlForSettings(requestSettings)
    const headers = runtimeAuthHeaders(requestSettings)
    headers.set('Accept', 'application/json')
    headers.set('Content-Type', 'application/json')
    headers.set('X-Analytix-Local-Display', 'typed-v1')
    const response = await fetch(`${baseUrl}${path}`, {
      method: 'POST',
      headers,
      body,
      signal: AbortSignal.any([
        ...(signal ? [signal] : []),
        AbortSignal.timeout(path === '/v1/local-display/funds-import/stage' ||
          path === '/v1/local-display/funds-import/confirm' ? 180_000 : 60_000)
      ])
    })
    const responseBody = await response.text()
    if (signal?.aborted || !isCurrentFinalPublicationAuthorityPin(runtimeAuthority)) {
      return { ok: false, status: 409, body: '' }
    }
    return {
      ok: response.ok,
      status: response.status,
      body: responseBody
    }
  } catch {
    return { ok: false, status: 503, body: '' }
  }
}

if (runningClawScheduleMcpServer) {
  void runClawScheduleMcpServerFromArgv(process.argv).catch((error) => {
    publicConsoleError('claw-schedule-mcp', 'Server failed.', error)
    process.exit(1)
  })
} else if (gotSingleInstanceLock) {
registerPackagedRendererScheme()
app.whenReady().then(async () => {
  traceStartup('app.whenReady:start')
  applyDesktopProtocolRegistrations(
    desktopExternalState,
    [ANALYTIX_DEEPLINK_PROTOCOL, NATIVE_OAUTH_CALLBACK_PROTOCOL],
    (protocol) => app.setAsDefaultProtocolClient(protocol)
  )
  installPackagedRendererProtocol()

  traceStartup('install webview guards:start')
  installDevPreviewWebviewGuards()
  traceStartup('install webview guards:done')

  if (process.platform === 'darwin') {
    const macDockIcon = createAppIcon(analytixMacIconPng)
    app.dock?.setIcon(macDockIcon.isEmpty() ? appIcon : macDockIcon)
  }

  store = new JsonSettingsStore(app.getPath('userData'), {
    homeRoot: desktopStateHomeRoot
  })
  traceStartup('settings load:start')
  const initial = await store.load()
  retainTerminalRuntimeProtectedRoots(initial)
  traceStartup('settings load:done')
  traceStartup('desktop private history migration:start')
  await migrateDesktopPrivateHistoryBeforeStartV2(initial, app.getPath('userData'))
  traceStartup('desktop private history migration:done')
  configurePackagedMainOwnedRuntimeAuthorityBootstrapV1({
    appIsPackaged: app.isPackaged,
    userDataDir: app.getPath('userData')
  })
  setAnalytixUnexpectedExitHandler(handleUnexpectedAnalytixExit)
  setGoRuntimeUnexpectedExitHandler(handleUnexpectedAnalytixExit)
  appBehavior = initial.appBehavior
  syncLoginItemSettings(initial)
  syncTray(initial)
  await syncClawScheduleMcpConfig(
    initial,
    getClawScheduleMcpLaunchConfig(),
    clawScheduleMcpConfigPaths
  ).catch((error) => {
    publicConsoleError('claw-schedule-mcp', 'Failed to sync config on startup.', error)
  })

  logDir = resolveLogDirectory()
  configureLogger({
    dir: logDir,
    enabled: initial.log.enabled,
    retentionDays: initial.log.retentionDays
  })
  traceStartup('logger configured')
  dataAnalysisBackendManager = new DataAnalysisBackendManager()
  const chromeNativeHostRegistration = ensureChromeBrowserUseNativeHostRegistration()
  if (!chromeNativeHostRegistration.ok) {
    logWarn('chrome-browser-use', 'failed to register Analytix Chrome native host', {
      reason: chromeNativeHostRegistration.reason
    })
  }
  scheduleRuntime = createScheduleRuntime({ store, runtimeRequest, logError, powerSaveBlocker })
  scheduleRuntime.sync(initial)
  const mainAccountRuntimeRequest = async (path: string, method?: string, body?: string) => {
    const settings = await store.load()
    return runtimeRequest(settings, path, { method, body })
  }
  const resolveMainAccountCredential = createMainAccountCredentialResolver(mainAccountRuntimeRequest)
  const manageMainAccountCredential = createAccountCredentialIpcHandler(mainAccountRuntimeRequest)
  const listMainAccountCredentials = createMainAccountCredentialLister(mainAccountRuntimeRequest)
  const mainProviderRegistry = createProviderRegistryIpcHandler(mainAccountRuntimeRequest)
  loadTrayProviderObservation = async () => {
    const snapshot = await mainProviderRegistry({ schemaVersion: 1, operation: 'list' })
    if ('error' in snapshot || !('providers' in snapshot) || !snapshot.selectedProviderId) return null
    const provider = snapshot.providers.find((candidate) => candidate.id === snapshot.selectedProviderId)
    if (!provider || provider.tombstone || !provider.credentialConfigured || !provider.credentialPurpose) return null
    const observation = await mainProviderRegistry({
      schemaVersion: 1,
      operation: 'observe-account',
      providerId: provider.id,
      expected: {
        registryRevision: snapshot.registryRevision,
        registryIncarnation: snapshot.registryIncarnation,
        providerRevision: provider.revision,
        providerGeneration: provider.generation,
        providerIncarnation: provider.incarnation,
        providerCredentialPurpose: provider.credentialPurpose
      }
    })
    if ('error' in observation || !('observedAt' in observation) || !('expiresAt' in observation)) return null
    const current = await mainProviderRegistry({ schemaVersion: 1, operation: 'list' })
    if ('error' in current || !('providers' in current)) return null
    return currentTrayProviderObservation({
      expectedSnapshot: snapshot,
      expectedProvider: provider,
      observation,
      currentSnapshot: current
    })
  }
  const managedMcpAccountBindings = async () => {
    const settings = await store.load()
    return listManagedMcpAccountCredentialBindings(
      resolveAnalytixDataDir(getAnalytixRuntimeSettings(settings))
    )
  }
  const installedExtensionAccountLifecycle = createInstalledExtensionAccountLifecycle({
    requestAccountCredential: manageMainAccountCredential,
    resolveAccountCredential: resolveMainAccountCredential,
    listAccountCredentialStates: listMainAccountCredentials,
    authorizeBinding: async (binding) => (await managedMcpAccountBindings()).some((candidate) =>
      candidate.owner === 'extension' && candidate.provider === binding.pluginId &&
      candidate.channelId === binding.serverId && candidate.accountId === binding.accountId &&
      candidate.purpose === 'extension-provider-account-token' &&
      candidate.ownerFingerprint === binding.ownerFingerprint
    )
  })
  const reconcileInstalledExtensionAccounts = async () => {
    const bindings = (await managedMcpAccountBindings())
      .filter((binding) => binding.owner === 'extension')
      .map((binding) => ({
        pluginId: binding.provider,
        serverId: binding.channelId,
        accountId: binding.accountId,
        ownerFingerprint: binding.ownerFingerprint
      }))
    return installedExtensionAccountLifecycle.reconcileManagedAccounts(bindings)
  }
  await reconcileInstalledExtensionAccounts().catch(() => {
    publicConsoleWarn('extension-account', 'Protected extension account reconciliation remains pending.')
  })
  await migrateLegacyImAccountCredentials({
    store,
    requestAccountCredential: manageMainAccountCredential,
    resolveAccountCredential: resolveMainAccountCredential
  }).catch(() => {
    publicConsoleWarn('claw-im', 'Protected legacy IM account migration remains pending.')
  })
  const imChannelAccountLifecycle = new ImChannelAccountLifecycle({
    dataDir: app.getPath('userData'),
    store,
    requestAccountCredential: manageMainAccountCredential,
    resolveAccountCredential: resolveMainAccountCredential,
    onCredentialInvalidated: async (scope) => {
      if (scope.owner === 'transport' && scope.provider === 'feishu') {
        await clawRuntime?.invalidateFeishuAccountCredential(scope)
      }
    }
  })
  await imChannelAccountLifecycle.recover().catch(() => {
    publicConsoleWarn('claw-im', 'Protected IM channel lifecycle recovery remains pending.')
  })
  const oauthProfileBinding = createHash('sha256')
    .update(`analytix-oauth-profile-v1\0${app.getPath('userData')}`)
    .digest('hex')
  const configFenceDecimal = (fingerprint: string) =>
    BigInt(`0x${fingerprint.slice(0, 16)}`).toString(10)
  const buildOwnedOAuthBinding = (input: {
    owner: 'provider' | 'mcp' | 'extension'
    provider: string
    accountId: string
    channelId?: string
    oauthBinding: {
      issuer: string
      authorizationEndpoint: string
      tokenEndpoint: string
      revocationEndpoint?: string
      clientId: string
      scopes: string[]
    }
    ownerFingerprint: string
    ownerRevision: string
    ownerGeneration: string
    ownerIncarnation: string
  }): OAuthAuthorizationBindingV1 => {
    const canonical = {
      issuer: new URL(input.oauthBinding.issuer).toString(),
      authorizationEndpoint: new URL(input.oauthBinding.authorizationEndpoint).toString(),
      tokenEndpoint: new URL(input.oauthBinding.tokenEndpoint).toString(),
      ...(input.oauthBinding.revocationEndpoint
        ? { revocationEndpoint: new URL(input.oauthBinding.revocationEndpoint).toString() }
        : {}),
      clientId: input.oauthBinding.clientId,
      scopes: [...input.oauthBinding.scopes]
    }
    const bindingKey = `oauthb_${createHash('sha256').update(JSON.stringify({
      owner: input.owner,
      provider: input.provider,
      accountId: input.accountId,
      channelId: input.channelId ?? '',
      ...canonical,
      ownerFingerprint: input.ownerFingerprint
    })).digest('base64url')}`
    return {
      owner: input.owner,
      ...canonical,
      provider: input.provider,
      accountId: input.accountId,
      ...(input.channelId ? { channelId: input.channelId } : {}),
      bindingKey,
      redirectUri: `${NATIVE_OAUTH_CALLBACK_PROTOCOL}:/oauth/callback/${bindingKey}`,
      ownerFingerprint: input.ownerFingerprint,
      ownerRevision: input.ownerRevision,
      ownerGeneration: input.ownerGeneration,
      ownerIncarnation: input.ownerIncarnation,
      profileBinding: oauthProfileBinding
    }
  }
  const resolveOwnedOAuthBinding = async (
    target: OAuthBindingTargetV1
  ): Promise<OAuthAuthorizationBindingV1 | null> => {
    if (target.owner === 'provider') {
      const result = await mainProviderRegistry({
        schemaVersion: 1, operation: 'get', providerId: target.providerId
      })
      if (!('provider' in result) || result.provider.tombstone) return null
      const provider = result.provider
      const oauthBinding = provider.oauthBinding
      if (!oauthBinding) return null
      const ownerFingerprint = createHash('sha256').update(JSON.stringify({
        providerId: provider.id,
        oauthBinding,
        incarnation: provider.incarnation
      })).digest('hex')
      return buildOwnedOAuthBinding({
        owner: 'provider', provider: provider.id, accountId: provider.id,
        oauthBinding,
        ownerFingerprint,
        ...providerOAuthOwnerFenceFromProjection(provider)
      })
    }
    const bindings = await managedMcpAccountBindings()
    const binding = bindings.find((candidate) => target.owner === 'mcp'
      ? candidate.owner === 'mcp' && candidate.provider === target.serverId &&
        candidate.channelId === target.serverId && candidate.accountId === target.accountId
      : candidate.owner === 'extension' && candidate.provider === target.pluginId &&
        candidate.channelId === target.serverId && candidate.accountId === target.accountId)
    const oauthBinding = binding?.oauthBinding
    if (!binding || !oauthBinding) return null
    const fence = configFenceDecimal(binding.ownerFingerprint)
    return buildOwnedOAuthBinding({
      owner: target.owner,
      provider: binding.provider,
      accountId: binding.accountId,
      channelId: binding.channelId,
      oauthBinding,
      ownerFingerprint: binding.ownerFingerprint,
      ownerRevision: fence,
      ownerGeneration: fence,
      ownerIncarnation: `cfg_${binding.ownerFingerprint}`
    })
  }
  let providerOAuthRefreshScheduler: ReturnType<typeof createProviderOAuthRefreshScheduler> | undefined
  const providerOAuthLifecycle = createProviderOAuthAuthorizationLifecycle({
    requestAccountCredential: manageMainAccountCredential,
    resolveAccountCredential: resolveMainAccountCredential,
    listAccountCredentialStates: listMainAccountCredentials,
    authorizeBinding: async (binding) => {
      const target: OAuthBindingTargetV1 = binding.owner === 'provider'
        ? { owner: 'provider', providerId: binding.provider }
        : binding.owner === 'mcp'
          ? { owner: 'mcp', serverId: binding.channelId ?? '', accountId: binding.accountId }
          : {
              owner: 'extension', pluginId: binding.provider,
              serverId: binding.channelId ?? '', accountId: binding.accountId
            }
      const current = await resolveOwnedOAuthBinding(target)
      return current !== null && oauthAuthorizationBindingsEqual(current, binding)
    },
    isInitiatingAuthorityCurrent: (webContentsId, profileBinding) =>
      profileBinding === oauthProfileBinding && Boolean(
        mainWindow && !mainWindow.isDestroyed() && !mainWindow.webContents.isDestroyed() &&
        mainWindow.webContents.id === webContentsId
      ),
    onRefreshInventoryChanged: () => { void providerOAuthRefreshScheduler?.rescan() }
  })
  const providerOAuthAccountManagement = createMainOAuthAccountAuthority({
    lifecycle: providerOAuthLifecycle,
    profileBinding: oauthProfileBinding,
    resolveBinding: resolveOwnedOAuthBinding,
    openExternal: async (authorizationUrl) => {
      await shell.openExternal(authorizationUrl)
    },
    configureProviderBinding: async ({ providerId, oauthBinding }) => {
      const current = await mainProviderRegistry({ schemaVersion: 1, operation: 'get', providerId })
      if (!('provider' in current) || current.provider.tombstone) {
        throw new Error('OAuth account binding is unavailable.')
      }
      const provider = current.provider
      if (JSON.stringify(provider.oauthBinding ?? null) === JSON.stringify(oauthBinding)) return
      const result = await mainProviderRegistry({
        schemaVersion: 1,
        operation: 'update',
        providerId,
        expected: {
          registryRevision: current.registryRevision,
          registryIncarnation: current.registryIncarnation,
          providerRevision: provider.revision,
          providerGeneration: provider.generation,
          providerIncarnation: provider.incarnation,
          providerCredentialPurpose: provider.credentialPurpose ?? ''
        },
        provider: providerRegistryInputWithOAuthBinding(provider, oauthBinding),
        credential: provider.credentialPurpose === 'provider-oauth-token-bundle'
          ? { kind: 'unset' }
          : { kind: 'keep' }
      })
      if (!('provider' in result) || JSON.stringify(result.provider.oauthBinding) !== JSON.stringify(oauthBinding)) {
        throw new Error('OAuth account binding is unavailable.')
      }
    }
  })
  await nativeOAuthCallbackRouter.activate(async (callbackUrl) => {
    await providerOAuthAccountManagement.handleNativeCallback(callbackUrl)
  })
  await providerOAuthLifecycle.sweepAuthorizationStates().catch(() => {
    publicConsoleWarn('provider-oauth', 'Protected OAuth authorization recovery remains pending.')
  })
  providerOAuthRefreshScheduler = createProviderOAuthRefreshScheduler({
    refreshInventory: (refreshWindowMs) =>
      providerOAuthLifecycle.refreshExpiringAccounts(refreshWindowMs),
    onError: () => {
      publicConsoleWarn('provider-oauth', 'Protected OAuth refresh remains pending.')
    }
  })
  await providerOAuthRefreshScheduler.start()
  configureWeixinBridgeAccountCredentialResolver(async (accountId) => {
    const settings = await store.load()
    const channels = settings.claw.channels.filter(
      (item) => item.enabled &&
        item.platformAccount?.kind === 'weixin' &&
        item.platformAccount.accountId === accountId
    )
    if (channels.length !== 1) return { status: 'unavailable', channelId: '', generation: '0', incarnation: '' }
    const channel = channels[0]
    const resolution = await resolveMainAccountCredential({
      owner: 'transport',
      provider: 'weixin',
      accountId,
      channelId: channel.id,
      purpose: 'transport-weixin-session-key'
    })
    if (!resolution.ok || resolution.credential.kind !== 'weixin') {
      return { status: 'unavailable', channelId: channel.id, generation: '0', incarnation: '' }
    }
    return {
      status: 'ready',
      token: resolution.credential.sessionKey,
      channelId: channel.id,
      generation: resolution.state.providerGeneration,
      incarnation: resolution.state.providerIncarnation
    }
  })
  const contextTokenScope = (input: { accountId: string; channelId: string; userId: string }) => ({
    owner: 'transport' as const,
    provider: 'weixin',
    accountId: input.accountId,
    channelId: `${input.channelId}:context:${createHash('sha256').update(input.userId, 'utf8').digest('hex').slice(0, 32)}`,
    purpose: 'transport-weixin-context-token' as const
  })
  configureWeixinBridgeContextTokenAuthority({
    put: async (input) => {
      const scope = contextTokenScope(input)
      const current = await manageMainAccountCredential({ schemaVersion: 1, operation: 'status', scope })
      if ('error' in current) throw new Error('WeChat context token authority is unavailable.')
      const committed = await manageMainAccountCredential({
        schemaVersion: 1,
        operation: 'put',
        scope,
        expected: accountCredentialExpectedState(current),
        credential: {
          kind: 'weixin-context',
          contextToken: input.token,
          parentGeneration: input.parentGeneration,
          parentIncarnation: input.parentIncarnation,
          expiresAtMs: input.expiresAtMs
        }
      })
      if ('error' in committed || committed.status !== 'ready') {
        throw new Error('WeChat context token authority is unavailable.')
      }
    },
    resolve: async (input) => {
      const scope = contextTokenScope(input)
      const resolution = await resolveMainAccountCredential(scope)
      if (!resolution.ok || resolution.credential.kind !== 'weixin-context') return undefined
      if (resolution.credential.parentGeneration === input.parentGeneration &&
        resolution.credential.parentIncarnation === input.parentIncarnation &&
        resolution.credential.expiresAtMs > Date.now()) {
        return resolution.credential.contextToken
      }
      await manageMainAccountCredential({
        schemaVersion: 1,
        operation: 'delete',
        scope,
        expected: accountCredentialExpectedState(resolution.state)
      }).catch(() => undefined)
      return undefined
    }
  })
  telegramRuntime = createTelegramRuntime({
    logError,
    onInbound: (payload) => clawRuntime?.handleTelegramUpdate(payload),
    resolveAccountCredential: async (request) => {
      const resolution = await resolveMainAccountCredential({
        owner: request.owner,
        provider: request.provider,
        accountId: request.accountId,
        channelId: request.channelId,
        purpose: request.purpose
      })
      if (!resolution.ok || resolution.credential.kind !== 'telegram') {
        return { status: 'unavailable', generation: '0', incarnation: '' }
      }
      return {
        status: 'ready',
        generation: resolution.state.providerGeneration,
        incarnation: resolution.state.providerIncarnation,
        botToken: resolution.credential.botToken,
        allowedChatIds: resolution.credential.allowedChatIds
      }
    }
  })
  clawRuntime = createClawRuntime({
    store,
    runtimeRequest,
    logError,
    notifyChannelActivity: emitClawChannelActivity,
    sendWeixinBridgeMessage,
    resolveWeixinAccountUserId: getWeixinBridgeAccountUserId,
    resolveAccountCredential: resolveMainAccountCredential,
    telegramRuntime,
    createScheduledTaskFromText: (text, options) =>
      scheduleRuntime?.createScheduledTaskFromText(text, options) ?? Promise.resolve({ kind: 'noop' })
  })
  clawRuntime.sync(initial)
  configureWeixinBridgeRuntimeContextProvider(async () => {
    const settings = await store.load()
    const channel = settings.claw.channels.find((item) => item.enabled && item.provider === 'weixin')
    return {
      webhookUrl: webhookUrl(settings),
      webhookSecret: settings.claw.im.secret,
      channelId: channel?.id ?? ''
    }
  })
  configureManagedWeixinBridgeUrlResolver(ensureWeixinBridgeRpcUrl)
  syncWeixinBridgeRuntime(initial)

  traceStartup('ipc registration:start')
  const applySettingsPatch = async (partial: AppSettingsPatch): Promise<AppSettingsV1> => {
    const prev = await store.load()
    const { runtime: runtimePatch, provider: providerPatch, ...restPatch } = partial
    const next = normalizeAppSettings({
      ...applyAnalytixRuntimePatch(prev, runtimePatch),
      ...restPatch,
      provider: mergeModelProviderSettings(prev.provider, providerPatch),
      log: { ...prev.log, ...(partial.log ?? {}) },
      notifications: { ...prev.notifications, ...(partial.notifications ?? {}) },
      appBehavior: mergeAppBehaviorSettings(prev.appBehavior, partial.appBehavior),
      keyboardShortcuts: normalizeKeyboardShortcuts({
        bindings: {
          ...prev.keyboardShortcuts.bindings,
          ...(partial.keyboardShortcuts?.bindings ?? {})
        }
      }),
      write: mergeWriteSettings(prev.write, partial.write),
      claw: mergeClawSettings(prev.claw, partial.claw),
      schedule: mergeScheduleSettings(prev.schedule, partial.schedule),
      guiUpdate: { ...prev.guiUpdate, ...(partial.guiUpdate ?? {}) }
    })
    if (prev.log.enabled !== next.log.enabled || prev.log.retentionDays !== next.log.retentionDays) {
      configureLogger({ enabled: next.log.enabled, retentionDays: next.log.retentionDays })
    }
    const runtimeValidationError = validateRuntimeSettingsForApply(next)
    if (runtimeValidationError) {
      throw new Error(`Invalid runtime settings: ${runtimeValidationError}`)
    }
    retainTerminalRuntimeProtectedRoots(prev)
    retainTerminalRuntimeProtectedRoots(next)
    const startupConfigChanged = runtimeStartupConfigChanged(prev, next)
    let releaseTerminalCreates = startupConfigChanged
      ? terminalPtyController?.suspendCreates() ?? null
      : null
    let saved: AppSettingsV1
    try {
      if (releaseTerminalCreates) {
        await new Promise<void>((resolveReady) => setImmediate(resolveReady))
      }
      saved = await store.patch(partial)
      if (startupConfigChanged || saved.write.inlineCompletion.enabled === false || saved.write.inlineCompletion.retrievalEnabled === false) {
        clearWriteRetrievalCache()
      } else if (prev.workspaceRoot && prev.workspaceRoot !== saved.workspaceRoot) {
        clearWriteRetrievalCache({ workspaceRoot: prev.workspaceRoot })
      }
      const runtimeApply = queueRuntimeSettingsApply(prev, saved)
      if (releaseTerminalCreates) {
        const release = releaseTerminalCreates
        releaseTerminalCreates = null
        if (runtimeApply) {
          void runtimeApply.then(release, release)
        } else {
          release()
        }
      }
    } catch (error) {
      releaseTerminalCreates?.()
      throw error
    }
    await syncClawScheduleMcpConfig(
      saved,
      getClawScheduleMcpLaunchConfig(),
      clawScheduleMcpConfigPaths
    ).catch((error) => {
      publicConsoleError('claw-schedule-mcp', 'Failed to sync config after settings change.', error)
    })
    await reconcileInstalledExtensionAccounts().catch(() => {
      publicConsoleWarn('extension-account', 'Protected extension account reconciliation remains pending.')
    })
    if (prev.guiUpdate.channel !== saved.guiUpdate.channel && guiUpdaterModulePromise) {
      void guiUpdaterModulePromise.then((module) => module.setGuiUpdateChannel(saved.guiUpdate.channel))
    }
    try {
      scheduleRuntime?.sync(saved)
      clawRuntime?.sync(saved)
    } catch (error) {
      logError(
        'settings-apply',
        'failed to sync schedule/claw runtimes after settings change',
        runtimeErrorPublicDiagnosticV1(error)
      )
    }
    syncWeixinBridgeRuntime(saved)
    syncLoginItemSettings(saved)
    syncTray(saved)
    return saved
  }

  type HubAccountService = import('./services/hub-account-service').HubAccountService
  let hubAccountServicePromise: Promise<HubAccountService> | null = null
  const loadHubAccountService = (): Promise<HubAccountService> => {
    if (!hubAccountServicePromise) {
      hubAccountServicePromise = import('./services/hub-account-service')
        .then(({ createHubAccountService }) => createHubAccountService({
          store,
          logError,
          restartRuntime: async () => {
            const settings = await store.load()
            if (getAnalytixRuntimeSettings(settings).autoStart) {
              await restartRuntime(settings)
              return
            }
            terminalPtyController?.disposeAll()
            await analytixRuntimeAdapter.stopAndWait()
            publishRuntimeStatus({
              code: 'hub_account_signed_out',
              source: 'hub-account'
            })
          }
        }))
        .catch((error) => {
          hubAccountServicePromise = null
          throw error
        })
    }
    return hubAccountServicePromise
  }

  const fetchModels = async () => {
    const registry = await mainProviderRegistry({ schemaVersion: 1, operation: 'list' })
    return fetchUpstreamModelIds(registry)
  }

  const saveSettingsPatch = async (partial: AppSettingsPatch): Promise<AppSettingsV1> => {
    if (!silentSettingsPatchCanPersistWithoutRuntimeApply(partial)) {
      return applySettingsPatch(partial)
    }
    return store.patch(partial)
  }

  registerAppIpcHandlers({
    store,
    loadHubAccountService,
    getMainWindow: () => mainWindow,
    canNavigateWindow: contents => {
      const owner = BrowserWindow.fromWebContents(contents)
      return !!owner && writeShutdown.canNavigateWindow(owner)
    },
    isTrustedProviderRegistrySender: isTrustedDesktopRenderer,
    applySettingsPatch,
    saveSettingsPatch,
    runtimeRequest: async (path, method, body) => {
      const settings = await store.load()
      const result = await runtimeRequest(settings, path, { method, body })
      const removedThread = method === 'DELETE' && result.ok ? /^\/v1\/threads\/([A-Za-z0-9._:-]+)$/.exec(path)?.[1] : undefined
      if (removedThread) clearWriteRetrievalCache({ threadId: removedThread })
      return result
    },
    protectedRuntimeRequest: mainAccountRuntimeRequest,
    getCurrentProfile: async () => {
      const settings = await store.load()
      const dataDirectory = resolveAnalytixDataDir(getAnalytixRuntimeSettings(settings))
      return { profileBinding: oauthProfileBinding, dataDirectory }
    },
    getCurrentPortableManifest: async () => {
      const result = await mainProviderRegistry({ schemaVersion: 1, operation: 'export-portable-manifest' })
      return 'manifestJson' in result ? result.manifestJson : null
    },
    getCurrentOwnerBindingInventory: async () => (await managedMcpAccountBindings()).map((binding) => ({
      owner: binding.owner,
      provider: binding.provider,
      accountId: binding.accountId,
      channelId: binding.channelId,
      purpose: binding.purpose,
      fingerprint: binding.ownerFingerprint
    })),
    privateMediaRequest: createPrivateMediaRuntimeRequest({
      loadSettings: () => store.load(),
      ensureRuntime,
      captureAuthorityPin: captureCurrentFinalPublicationAuthorityPin,
      isCurrentAuthorityPin: isCurrentFinalPublicationAuthorityPin,
      getBaseUrl: getRuntimeBaseUrlForSettings,
      getProfileBinding: (settings) => JSON.stringify({
        profileBinding: oauthProfileBinding,
        dataDirectory: resolveAnalytixDataDir(getAnalytixRuntimeSettings(settings))
      }),
      authHeaders: runtimeAuthHeaders
    }),
    localDisplayRequest: async (path, body, signal) => {
      const settings = await store.load()
      return localDisplayRuntimeRequest(settings, path, body, signal)
    },
    restartRuntime: async () => {
      const settings = await store.load()
      await restartRuntime(settings)
    },
    fetchUpstreamModels: fetchModels,
    getClawRuntime: () => clawRuntime,
    getScheduleRuntime: () => scheduleRuntime,
    startFeishuInstallQrcode,
    pollFeishuInstall,
    startWeixinInstallQrcode,
    pollWeixinInstall,
    imChannelAccountLifecycle,
    installedExtensionAccountLifecycle,
    providerOAuthAccountManagement,
    resolveAnalytixConfigPath: () => resolveAnalytixMcpJsonPath(desktopStateHomeRoot),
    onAnalytixMcpConfigWritten: async () => {
      const settings = await store.load()
      queueRuntimeMcpConfigApply(settings)
    },
    showTurnCompleteNotification,
    openThreadInNewWindow,
    getAppVersion: () => app.getVersion(),
    readGuiUpdateState,
    loadGuiUpdaterModule,
    resolveLogDirectory,
    logError
  })

  registerRuntimeSseIpc({ ipcMain, store, ensureRuntime, logError })
  terminalPtyController = registerTerminalPtyIpc({
    ipcMain,
    getMainWindow: () => mainWindow,
    getProtectedRoots: async () => {
      const settings = await store.load()
      retainTerminalRuntimeProtectedRoots(settings)
      return [
        ...defaultDarwinTerminalProtectedRoots(desktopStateHomeRoot),
        app.getPath('userData'),
        ...retainedTerminalRuntimeProtectedRoots
      ]
    },
    isTrustedSender: isTrustedDesktopRenderer,
    // The confirmed quit path disposes PTYs in stopManagedRuntimes. A raw
    // before-quit listener would destroy them even when Office cancels exit.
    logError
  })
  registerBackgroundTaskIpc({ ipcMain })
  if (!dataAnalysisBackendManager) throw new Error('data analysis renderer authority is unavailable')
  registerDataAnalysisIpcHandlers({
    manager: dataAnalysisBackendManager,
    getMainWindow: () => mainWindow,
    logError
  })
  traceStartup('ipc registration:done')

  desktopStartupBarrierComplete = true
  const initialDeepLinkThreadId = pendingDeepLinkThreadId
  pendingDeepLinkThreadId = ''
  createWindow({
    suppressInitialShow: initialDeepLinkThreadId ? false : shouldStartHidden(initial),
    ...(initialDeepLinkThreadId ? { initialThreadId: initialDeepLinkThreadId } : {})
  })
  traceStartup('createWindow:returned')
  emitHubActivityObservation('ordinary_startup_ready')

  void loadGuiUpdaterModule()
    .then((module) => module.showPostUpdateReleaseNotes())
    .catch((error) => {
      publicConsoleWarn('updater', 'Failed to show post-update release notes.', error)
    })

  void pruneOnStartup().catch((err) => {
    publicConsoleWarn('logger', 'Failed to prune logs.', err)
  })

  if (getAnalytixRuntimeSettings(initial).autoStart) {
    setTimeout(() => {
      void analytixRuntimeAdapter.resolveExecutable(initial).catch((err) => {
        publicConsoleWarn('runtime', 'Failed to prewarm Go runtime launcher.', err)
      })
    }, 1500)
  }

  app.on('second-instance', (_event, argv) => {
    const oauthCallback = nativeOAuthCallbackFromArgv(argv)
    if (oauthCallback) {
      nativeOAuthCallbackRouter.route(oauthCallback)
      return
    }
    const threadId = threadIdFromArgv(argv)
    if (threadId) {
      openThreadInNewWindow(threadId)
      return
    }
    revealMainWindow()
  })

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
    else revealMainWindow()
  })
}).catch((error) => {
  desktopStartupBarrierComplete = false
  const diagnostic = runtimeErrorPublicDiagnosticV1(error)
  publicConsoleError('startup', 'Startup failed.', { bytes: diagnostic.errorBytes, sha256: diagnostic.errorSha256 })
  dialog.showErrorBox('Analytix failed to start', 'The desktop application could not complete startup.')
  app.quit()
})
}

app.on('window-all-closed', () => {
  void stopManagedRuntimes().catch((error) => {
    publicConsoleWarn('runtime', 'Failed to stop Analytix runtime.', error)
  })
  if (process.platform !== 'darwin') {
    app.quit()
  }
})

app.on('before-quit', (event) => {
  isQuitting = true
  if (managedRuntimesStoppedForQuit) return
  event.preventDefault()
  if (nativeOfficeQuitPending) return
  nativeOfficeQuitPending = true
  void (async () => {
    if (!await writeShutdown.prepareQuit()) {
      isQuitting = false
      return
    }
    // Native drafts and unresolved file outcomes use the existing Core session.
    // Confirm and drain them before stopping that runtime, not at window close.
    stopRuntimeWatchdog()
    try { await stopManagedRuntimesForQuit() } catch (error) {
      publicConsoleWarn('runtime', 'Failed to stop Analytix runtime.', error)
      managedRuntimesStoppedForQuit = true
    }
    app.quit()
  })().catch(async () => {
    await writeShutdown.cancel().catch(() => undefined)
    isQuitting = false
    if (desktopStartupBarrierComplete) startRuntimeWatchdog()
  }).finally(() => { nativeOfficeQuitPending = false })
})
