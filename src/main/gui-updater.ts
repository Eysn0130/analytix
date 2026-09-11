import { app, autoUpdater as nativeAutoUpdater, BrowserWindow, dialog, shell } from 'electron'
import type { MessageBoxOptions } from 'electron'
import { existsSync, readFileSync } from 'node:fs'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import electronUpdater from 'electron-updater'
import type { ProgressInfo, UpdateDownloadedEvent, UpdateInfo } from 'electron-updater'
import type {
  GuiUpdateChannel,
  GuiUpdateDownloadResult,
  GuiUpdateFailureCode,
  GuiUpdateInfo,
  GuiUpdateInstallResult,
  GuiUpdateState
} from '../shared/gui-update'
import { nextGuiUpdateCheckDelay } from '../shared/gui-update-schedule'
import { DEFAULT_GUI_UPDATE_CHANNEL, normalizeGuiUpdateChannel } from '../shared/gui-update'
import { publicConsoleError, publicConsoleInfo, publicConsoleWarn } from './logger'

const DEFAULT_STABLE_UPDATE_FEED_URL = 'https://analytix.top/desktop/releases/standard-win/'
const DEFAULT_DOWNLOAD_PAGE_URL = 'https://analytix.top/downloads'
const UPDATE_FEED_PROBE_TIMEOUT_MS = 5_000
const { autoUpdater } = electronUpdater

let initialized = false
let desktopExternalStateIsolation = false
let getMainWindow: (() => BrowserWindow | null) | null = null
let lastInfo: Extract<GuiUpdateInfo, { ok: true }> | null = null
let lastState: GuiUpdateState = { status: 'idle' }
let downloaded = false
let downloadPromise: Promise<string[]> | null = null
let configuredChannel: GuiUpdateChannel = normalizeGuiUpdateChannel(
  process.env.ANALYTIX_UPDATE_CHANNEL?.trim() || undefined
)
let configuredFeedUrl = ''
let getSelectedChannel: (() => GuiUpdateChannel | Promise<GuiUpdateChannel>) | null = null
let getSelectedLocale: (() => 'en' | 'zh' | Promise<'en' | 'zh'>) | null = null
let beforeInstallUpdate: (() => void | Promise<void>) | null = null
let beforeInstallUpdatePromise: Promise<void> | null = null
let pendingVersionStateWrite: Promise<void> | null = null
let backgroundCheckTimer: NodeJS.Timeout | null = null
let backgroundCheckPromise: Promise<void> | null = null

const GUI_UPDATE_SCHEDULE_FILE = 'gui-update-schedule.json'
const GUI_VERSION_STATE_FILE = 'gui-version-state.json'

type GuiVersionState = {
  lastSeenVersion?: string
  pendingUpdate?: {
    version: string
    releaseNotes?: string
  }
}

function trimSlashes(value: string): string {
  return value.replace(/^\/+|\/+$/g, '')
}

function normalizeBaseUrl(raw: string): string {
  return raw.trim().replace(/\/+$/, '')
}

function joinUrl(base: string, ...parts: string[]): string {
  const cleanBase = normalizeBaseUrl(base)
  const cleanParts = parts.map((p) => trimSlashes(p)).filter(Boolean)
  return [cleanBase, ...cleanParts].join('/')
}

function envUpdateUrl(channel: GuiUpdateChannel): string {
  const channelSpecific = process.env[`ANALYTIX_UPDATE_URL_${channel.toUpperCase()}`]?.trim() ?? ''
  const direct = channelSpecific || process.env.ANALYTIX_UPDATE_URL?.trim() || ''
  return direct ? direct.replace(/\{channel\}/g, channel).replace(/\/?$/, '/') : ''
}

function uniqueStrings(values: string[]): string[] {
  return Array.from(new Set(values.filter(Boolean)))
}

function updateFeedUrlCandidates(channel: GuiUpdateChannel): string[] {
  const direct = envUpdateUrl(channel)
  if (direct) return [direct]

  const releaseBaseUrl = process.env.ANALYTIX_RELEASE_BASE_URL?.trim() || ''
  if (releaseBaseUrl) {
    return uniqueStrings([`${joinUrl(releaseBaseUrl, 'channels', channel, 'latest')}/`])
  }
  return [DEFAULT_STABLE_UPDATE_FEED_URL]
}

function updateFeedUrl(channel: GuiUpdateChannel): string {
  return updateFeedUrlCandidates(channel)[0]
}

function updateFeedManifestUrl(feedUrl: string): string {
  return `${feedUrl}${platformManifestName()}`
}

async function isUpdateFeedAccessible(feedUrl: string): Promise<boolean> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), UPDATE_FEED_PROBE_TIMEOUT_MS)
  try {
    const res = await fetch(updateFeedManifestUrl(feedUrl), {
      method: 'HEAD',
      headers: {
        Accept: 'application/x-yaml,text/yaml,text/plain,*/*',
        'User-Agent': `analytix/${app.getVersion()}`
      },
      signal: controller.signal
    })
    return res.ok
  } catch {
    return false
  } finally {
    clearTimeout(timeout)
  }
}

async function resolveUpdateFeedUrl(channel: GuiUpdateChannel): Promise<string> {
  const candidates = updateFeedUrlCandidates(channel)
  if (candidates.length <= 1) return candidates[0]

  for (const candidate of candidates) {
    if (await isUpdateFeedAccessible(candidate)) return candidate
  }
  return candidates[candidates.length - 1]
}

function guiUpdateSchedulePath(): string {
  return join(app.getPath('userData'), GUI_UPDATE_SCHEDULE_FILE)
}

function guiVersionStatePath(): string {
  return join(app.getPath('userData'), GUI_VERSION_STATE_FILE)
}

async function readGuiVersionState(): Promise<GuiVersionState> {
  try {
    const raw = await readFile(guiVersionStatePath(), 'utf8')
    const parsed = JSON.parse(raw) as GuiVersionState
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

async function writeGuiVersionState(state: GuiVersionState): Promise<void> {
  const path = guiVersionStatePath()
  await mkdir(dirname(path), { recursive: true })
  await writeFile(path, JSON.stringify(state, null, 2), 'utf8')
}

function changelogUrl(): string {
  return process.env.ANALYTIX_CHANGELOG_URL?.trim() || (resolveGithubReleaseUrl() ?? downloadPageUrl())
}

function normalizeReleaseNotes(value: unknown): string | undefined {
  if (typeof value === 'string') return value.trim() || undefined
  if (!Array.isArray(value)) return undefined
  const notes = value
    .map((entry) => {
      if (!entry || typeof entry !== 'object' || !('note' in entry)) return ''
      return typeof entry.note === 'string' ? entry.note.trim() : ''
    })
    .filter(Boolean)
  return notes.length > 0 ? notes.join('\n\n') : undefined
}

async function recordPendingUpdate(updateInfo: UpdateInfo): Promise<void> {
  const state = await readGuiVersionState()
  await writeGuiVersionState({
    ...state,
    pendingUpdate: {
      version: updateInfo.version.trim(),
      releaseNotes: normalizeReleaseNotes(updateInfo.releaseNotes)
    }
  })
}

async function selectedLocale(): Promise<'en' | 'zh'> {
  try {
    return (await getSelectedLocale?.()) === 'zh' ? 'zh' : 'en'
  } catch {
    return app.getLocale().toLowerCase().startsWith('zh') ? 'zh' : 'en'
  }
}

async function readLastScheduledCheckAt(): Promise<number | null> {
  try {
    const raw = await readFile(guiUpdateSchedulePath(), 'utf8')
    const parsed = JSON.parse(raw) as { lastCheckedAt?: unknown }
    const ms = typeof parsed.lastCheckedAt === 'string' ? Date.parse(parsed.lastCheckedAt) : Number.NaN
    return Number.isFinite(ms) ? ms : null
  } catch {
    return null
  }
}

async function writeLastScheduledCheckAt(nowMs: number): Promise<void> {
  const path = guiUpdateSchedulePath()
  await mkdir(dirname(path), { recursive: true })
  await writeFile(
    path,
    JSON.stringify({ lastCheckedAt: new Date(nowMs).toISOString() }, null, 2),
    'utf8'
  )
}

function normalizeGithubOwnerRepo(raw: string): string | null {
  let s = raw.trim()
  if (!s) return null
  if (s.startsWith('github:')) s = s.slice('github:'.length).trim()
  const ssh = s.match(/^git@github\.com:([\w.-]+\/[\w.-]+?)(?:\.git)?$/i)
  if (ssh?.[1]) return ssh[1].replace(/\.git$/i, '').replace(/^\/+|\/+$/g, '')
  const https = s.match(/github\.com\/([\w.-]+\/[\w.-]+?)(?:\.git)?(?:$|[#/])/i)
  if (https?.[1]) return https[1].replace(/\.git$/i, '').replace(/^\/+|\/+$/g, '')
  if (/^[\w.-]+\/[\w.-]+$/.test(s)) return s
  return null
}

function packageJsonPath(): string {
  return join(app.getAppPath(), 'package.json')
}

function readPackageJson(): Record<string, unknown> | null {
  try {
    const path = packageJsonPath()
    if (!existsSync(path)) return null
    return JSON.parse(readFileSync(path, 'utf8')) as Record<string, unknown>
  } catch {
    return null
  }
}

function resolveGithubReleaseUrl(): string | null {
  const envRepo = normalizeGithubOwnerRepo(process.env.ANALYTIX_GITHUB_REPO?.trim() ?? '')
  if (envRepo) return `https://github.com/${envRepo}/releases`

  const pkg = readPackageJson()
  const repository = pkg?.repository
  const raw =
    typeof repository === 'string'
      ? repository
      : repository && typeof repository === 'object' && 'url' in repository
        ? String((repository as { url?: unknown }).url ?? '')
        : ''
  const repo = normalizeGithubOwnerRepo(raw)
  return repo ? `https://github.com/${repo}/releases` : null
}

function downloadPageUrl(): string {
  const direct = process.env.ANALYTIX_DOWNLOAD_URL?.trim() || ''
  if (direct) return direct

  const pkg = readPackageJson()
  const homepage = typeof pkg?.homepage === 'string' ? pkg.homepage.trim() : ''
  if (homepage) return homepage

  return resolveGithubReleaseUrl() ?? DEFAULT_DOWNLOAD_PAGE_URL
}

function releaseUrlForVersion(version: string): string {
  const page = downloadPageUrl()
  if (/github\.com\/.+\/releases\/?$/i.test(page)) {
    return `${page.replace(/\/+$/, '')}/tag/v${version.replace(/^v/i, '')}`
  }
  return page
}

function parseVersionParts(v: string): number[] {
  const cleaned = v.trim().replace(/^v/i, '').replace(/-.*$/, '')
  return cleaned.split('.').map((part) => Number.parseInt(part, 10) || 0)
}

function isVersionGreater(latest: string, current: string): boolean {
  const a = parseVersionParts(latest)
  const b = parseVersionParts(current)
  const len = Math.max(a.length, b.length)
  for (let i = 0; i < len; i += 1) {
    const av = a[i] ?? 0
    const bv = b[i] ?? 0
    if (av > bv) return true
    if (av < bv) return false
  }
  return false
}

function platformManifestName(): string {
  if (process.platform === 'darwin') return 'latest-mac.yml'
  if (process.platform === 'linux') return 'latest-linux.yml'
  return 'latest.yml'
}

function parseYamlScalar(source: string, key: string): string {
  const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = source.match(new RegExp(`^${escaped}:\\s*['"]?([^'"\\n]+)['"]?\\s*$`, 'm'))
  return match?.[1]?.trim() ?? ''
}

function macAutoUpdateAllowed(): boolean {
  if (process.platform !== 'darwin') return true
  if (process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES === '1') return true

  const pkg = readPackageJson()
  const hints = pkg?.buildHints
  if (!hints || typeof hints !== 'object') return false
  const values = hints as { macSigningEnabled?: unknown; notarizationEnabled?: unknown }
  return values.macSigningEnabled === true && values.notarizationEnabled === true
}

function unsupportedMessage(): string {
  if (process.platform === 'darwin') {
    return 'Automatic updates require a signed and notarized macOS build. Use the download page for this build.'
  }
  return 'Automatic updates are not supported for this build. Use the download page instead.'
}

function extractHttpStatus(raw: string): number | null {
  const match = raw.match(/\b(\d{3})\b/)
  if (!match) return null
  const status = Number.parseInt(match[1], 10)
  return Number.isFinite(status) ? status : null
}

function sanitizeUpdaterError(raw: string, channel: GuiUpdateChannel): string {
  const message = raw.trim()
  if (!message) {
    return `Could not read GUI update metadata for the ${channel} channel. Open the download page instead.`
  }

  if (/Invalid release object path\./i.test(message)) {
    return `The ${channel} update feed is not published correctly yet. Open the download page instead.`
  }

  if (/Object not found\./i.test(message)) {
    return `The ${channel} update feed is missing release metadata right now. Open the download page instead.`
  }

  const status = extractHttpStatus(message)
  if (status === 400 || status === 404) {
    return `The ${channel} update feed is not available right now. Open the download page instead.`
  }
  if (status === 403) {
    return `The ${channel} update feed denied this request. Open the download page instead.`
  }
  if (status === 429) {
    return `The ${channel} update feed is rate limited right now. Please try again later.`
  }
  if (status && status >= 500) {
    return `The ${channel} update feed is temporarily unavailable. Please try again later.`
  }

  return `Could not check the ${channel} update feed. Open the download page instead.`
}

function toGuiInfo(updateInfo: UpdateInfo, hasUpdate: boolean, manualOnly = false): Extract<GuiUpdateInfo, { ok: true }> {
  const latestVersion = updateInfo.version.trim()
  return {
    ok: true,
    currentVersion: app.getVersion(),
    latestVersion,
    hasUpdate,
    releaseUrl: releaseUrlForVersion(latestVersion),
    releaseDate: updateInfo.releaseDate,
    channel: configuredChannel,
    manualOnly,
    downloaded
  }
}

function emitGuiUpdateState(state: GuiUpdateState): void {
  lastState = state
  const win = getMainWindow?.()
  if (!win || win.isDestroyed() || win.webContents.isDestroyed()) return
  win.webContents.send('gui:update-state', state)
}

function runBeforeInstallUpdate(): Promise<void> {
  if (!beforeInstallUpdate) return Promise.resolve()
  if (!beforeInstallUpdatePromise) {
    beforeInstallUpdatePromise = Promise.resolve()
      .then(() => beforeInstallUpdate?.())
      .then(() => undefined)
      .finally(() => {
        beforeInstallUpdatePromise = null
      })
  }
  return beforeInstallUpdatePromise
}

function clearBackgroundCheckTimer(): void {
  if (backgroundCheckTimer) {
    clearTimeout(backgroundCheckTimer)
    backgroundCheckTimer = null
  }
}

function shouldSkipScheduledCheck(): boolean {
  return (
    lastState.status === 'checking' ||
    lastState.status === 'downloading' ||
    lastState.status === 'downloaded' ||
    lastState.status === 'installing'
  )
}

async function scheduleNextBackgroundCheck(): Promise<void> {
  clearBackgroundCheckTimer()
  if (desktopExternalStateIsolation) return
  const lastCheckedAtMs = await readLastScheduledCheckAt()
  const delay = nextGuiUpdateCheckDelay(lastCheckedAtMs)
  backgroundCheckTimer = setTimeout(() => {
    void runScheduledGuiUpdateCheck()
  }, delay)
}

async function runScheduledGuiUpdateCheck(): Promise<void> {
  if (backgroundCheckPromise) return backgroundCheckPromise
  backgroundCheckPromise = (async () => {
    try {
      if (shouldSkipScheduledCheck()) return
      const nowMs = Date.now()
      await writeLastScheduledCheckAt(nowMs)
      await checkGuiUpdate()
    } catch (error) {
      publicConsoleWarn('updater', 'Scheduled GUI update check failed.', error)
    } finally {
      backgroundCheckPromise = null
      void scheduleNextBackgroundCheck()
    }
  })()
  return backgroundCheckPromise
}

async function resolveUpdateChannel(requested?: GuiUpdateChannel): Promise<GuiUpdateChannel> {
  if (requested) return normalizeGuiUpdateChannel(requested)
  if (getSelectedChannel) {
    return normalizeGuiUpdateChannel(await getSelectedChannel())
  }
  return DEFAULT_GUI_UPDATE_CHANNEL
}

function configureUpdaterChannel(channel: GuiUpdateChannel, feedUrl = updateFeedUrl(channel)): void {
  const normalized = normalizeGuiUpdateChannel(channel)
  const changed = normalized !== configuredChannel || feedUrl !== configuredFeedUrl
  configuredChannel = normalized
  configuredFeedUrl = feedUrl
  autoUpdater.allowPrerelease = normalized === 'beta'
  autoUpdater.setFeedURL({ provider: 'generic', url: feedUrl })
  if (!changed) return
  downloaded = false
  downloadPromise = null
  lastInfo = null
  emitGuiUpdateState({ status: 'idle' })
}

async function configureReachableUpdaterChannel(channel: GuiUpdateChannel): Promise<void> {
  configureUpdaterChannel(channel, await resolveUpdateFeedUrl(channel))
}

export function setGuiUpdateChannel(channel: GuiUpdateChannel): void {
  configureUpdaterChannel(channel)
}

async function checkManualUpdate(
  channel: GuiUpdateChannel,
  code: GuiUpdateFailureCode = 'unsupported'
): Promise<GuiUpdateInfo> {
  const currentVersion = app.getVersion()
  try {
    const feedUrl = configuredChannel === channel && configuredFeedUrl
      ? configuredFeedUrl
      : await resolveUpdateFeedUrl(channel)
    const url = updateFeedManifestUrl(feedUrl)
    const res = await fetch(url, {
      headers: {
        Accept: 'application/x-yaml,text/yaml,text/plain,*/*',
        'User-Agent': `analytix/${currentVersion}`
      }
    })
    if (!res.ok) {
      return {
        ok: false,
        currentVersion,
        code,
        message: `${unsupportedMessage()} Update metadata returned ${res.status}.`,
        releaseUrl: downloadPageUrl(),
        channel
      }
    }
    const text = await res.text()
    const latestVersion = parseYamlScalar(text, 'version')
    if (!latestVersion) {
      return {
        ok: false,
        currentVersion,
        code,
        message: `${unsupportedMessage()} Update metadata is missing a version.`,
        releaseUrl: downloadPageUrl(),
        channel
      }
    }
    const info: Extract<GuiUpdateInfo, { ok: true }> = {
      ok: true,
      currentVersion,
      latestVersion,
      hasUpdate: isVersionGreater(latestVersion, currentVersion),
      releaseUrl: releaseUrlForVersion(latestVersion),
      releaseDate: parseYamlScalar(text, 'releaseDate'),
      channel,
      manualOnly: true,
      downloaded: false
    }
    lastInfo = info
    emitGuiUpdateState(info.hasUpdate ? { status: 'available', info } : { status: 'not_available', info })
    return info
  } catch {
    return {
      ok: false,
      currentVersion,
      code,
      message: `${unsupportedMessage()} Could not read GUI update metadata for the ${channel} channel.`,
      releaseUrl: downloadPageUrl(),
      channel
    }
  }
}

export function initializeGuiUpdater(
  windowGetter: () => BrowserWindow | null,
  channelGetter?: () => GuiUpdateChannel | Promise<GuiUpdateChannel>,
  beforeInstall?: () => void | Promise<void>,
  localeGetter?: () => 'en' | 'zh' | Promise<'en' | 'zh'>,
  suppressBackgroundTrafficForDesktopExternalStateIsolation = false
): void {
  getMainWindow = windowGetter
  getSelectedChannel = channelGetter ?? null
  beforeInstallUpdate = beforeInstall ?? null
  getSelectedLocale = localeGetter ?? null
  desktopExternalStateIsolation = suppressBackgroundTrafficForDesktopExternalStateIsolation === true
  if (initialized) return
  initialized = true

  autoUpdater.autoDownload = false
  autoUpdater.autoInstallOnAppQuit = false
  configureUpdaterChannel(configuredChannel)
  if (!app.isPackaged) {
    autoUpdater.forceDevUpdateConfig = true
  }

  autoUpdater.logger = {
    info: (message?: unknown) => publicConsoleInfo('updater', 'Updater event.', message),
    warn: (message?: unknown) => publicConsoleWarn('updater', 'Updater warning.', message),
    error: (message?: unknown) => publicConsoleError('updater', 'Updater error.', message)
  }

  autoUpdater.on('checking-for-update', () => {
    emitGuiUpdateState({ status: 'checking', info: lastInfo ?? undefined })
  })

  autoUpdater.on('update-available', (updateInfo: UpdateInfo) => {
    downloaded = false
    const info = toGuiInfo(updateInfo, true)
    lastInfo = info
    emitGuiUpdateState({ status: 'available', info })
  })

  autoUpdater.on('update-not-available', (updateInfo: UpdateInfo) => {
    downloaded = false
    const info = toGuiInfo(updateInfo, false)
    lastInfo = info
    emitGuiUpdateState({ status: 'not_available', info })
  })

  autoUpdater.on('download-progress', (progress: ProgressInfo) => {
    emitGuiUpdateState({ status: 'downloading', info: lastInfo ?? undefined, progress })
  })

  autoUpdater.on('update-downloaded', (event: UpdateDownloadedEvent) => {
    downloaded = true
    const info = toGuiInfo(event, true)
    lastInfo = info
    pendingVersionStateWrite = recordPendingUpdate(event)
      .catch((error) => {
        publicConsoleWarn('updater', 'Failed to save release notes.', error)
      })
      .finally(() => {
        pendingVersionStateWrite = null
      })
    emitGuiUpdateState({ status: 'downloaded', info })
  })

  autoUpdater.on('error', (error) => {
    const message = sanitizeUpdaterError(
      error instanceof Error ? error.message : String(error),
      configuredChannel
    )
    emitGuiUpdateState({ status: 'error', info: lastInfo ?? undefined, message, code: 'unknown' })
  })

  nativeAutoUpdater?.on?.('before-quit-for-update', () => {
    void runBeforeInstallUpdate().catch((error) => {
      publicConsoleWarn('updater', 'Failed to stop runtimes before update quit.', error)
    })
  })

  void scheduleNextBackgroundCheck()
}

export async function showPostUpdateReleaseNotes(): Promise<void> {
  const currentVersion = app.getVersion().trim()
  const state = await readGuiVersionState()
  if (!state.lastSeenVersion) {
    await writeGuiVersionState({ ...state, lastSeenVersion: currentVersion })
    return
  }
  if (state.lastSeenVersion === currentVersion) return

  const pendingUpdate =
    state.pendingUpdate?.version === currentVersion ? state.pendingUpdate : undefined
  await writeGuiVersionState({ lastSeenVersion: currentVersion })

  const locale = await selectedLocale()
  const isZh = locale === 'zh'
  const options: MessageBoxOptions = {
    type: 'info',
    title: isZh ? 'analytix 已更新' : 'analytix updated',
    message: isZh
      ? `已更新到 analytix ${currentVersion}`
      : `analytix has been updated to ${currentVersion}`,
    detail:
      pendingUpdate?.releaseNotes ??
      (isZh
        ? '此版本的完整更新内容可在 analytix 更新日志中查看。'
        : 'See the analytix changelog for the complete release notes.'),
    buttons: isZh ? ['查看更新日志', '稍后'] : ['View changelog', 'Later'],
    defaultId: 0,
    cancelId: 1,
    noLink: true
  }
  const window = getMainWindow?.()
  const result =
    window && !window.isDestroyed()
      ? await dialog.showMessageBox(window, options)
      : await dialog.showMessageBox(options)
  if (result.response === 0) {
    await shell.openExternal(changelogUrl())
  }
}

export function getGuiUpdateState(): GuiUpdateState {
  return lastState
}

export async function checkGuiUpdate(channel?: GuiUpdateChannel): Promise<GuiUpdateInfo> {
  const selectedChannel = await resolveUpdateChannel(channel)
  await configureReachableUpdaterChannel(selectedChannel)

  if (!macAutoUpdateAllowed()) {
    return checkManualUpdate(selectedChannel, 'unsupported')
  }

  emitGuiUpdateState({ status: 'checking', info: lastInfo ?? undefined })
  try {
    const result = await autoUpdater.checkForUpdates()
    if (!result) {
      return checkManualUpdate(selectedChannel, 'not_configured')
    }
    const info = toGuiInfo(result.updateInfo, result.isUpdateAvailable)
    lastInfo = info
    emitGuiUpdateState(info.hasUpdate ? { status: 'available', info } : { status: 'not_available', info })
    return info
  } catch (e) {
    const message = sanitizeUpdaterError(e instanceof Error ? e.message : String(e), selectedChannel)
    const info: GuiUpdateInfo = {
      ok: false,
      currentVersion: app.getVersion(),
      message,
      code: 'unknown',
      releaseUrl: downloadPageUrl(),
      channel: selectedChannel
    }
    emitGuiUpdateState({ status: 'error', info, message, code: 'unknown' })
    return info
  }
}

export async function downloadGuiUpdate(channel?: GuiUpdateChannel): Promise<GuiUpdateDownloadResult> {
  const selectedChannel = await resolveUpdateChannel(channel)
  await configureReachableUpdaterChannel(selectedChannel)

  if (!macAutoUpdateAllowed()) {
    return {
      ok: false,
      currentVersion: app.getVersion(),
      code: 'unsupported',
      message: unsupportedMessage()
    }
  }

  try {
    if (!lastInfo?.hasUpdate || lastInfo.channel !== selectedChannel) {
      const checked = await checkGuiUpdate(selectedChannel)
      if (!checked.ok) return checked
      if (!checked.hasUpdate || checked.manualOnly) {
        return {
          ok: false,
          currentVersion: app.getVersion(),
          code: checked.manualOnly ? 'unsupported' : 'unknown',
          message: checked.manualOnly
            ? unsupportedMessage()
            : 'No downloadable GUI update is available.'
        }
      }
    }

    if (!downloadPromise) {
      downloadPromise = autoUpdater.downloadUpdate().finally(() => {
        downloadPromise = null
      })
    }
    const paths = await downloadPromise
    return { ok: true, paths }
  } catch {
    const message = 'GUI update download failed.'
    emitGuiUpdateState({ status: 'error', info: lastInfo ?? undefined, message, code: 'download_failed' })
    return {
      ok: false,
      currentVersion: app.getVersion(),
      code: 'download_failed',
      message
    }
  }
}

export async function installGuiUpdate(): Promise<GuiUpdateInstallResult> {
  try {
    if (!downloaded) {
      return {
        ok: false,
        currentVersion: app.getVersion(),
        code: 'install_failed',
        message: 'The update has not finished downloading yet.'
      }
    }
    emitGuiUpdateState({ status: 'installing', info: lastInfo ?? undefined })
    await Promise.all([pendingVersionStateWrite, runBeforeInstallUpdate()])
    autoUpdater.quitAndInstall(false, true)
    return { ok: true }
  } catch {
    const message = 'GUI update installation failed.'
    emitGuiUpdateState({ status: 'error', info: lastInfo ?? undefined, message, code: 'install_failed' })
    return {
      ok: false,
      currentVersion: app.getVersion(),
      code: 'install_failed',
      message
    }
  }
}
