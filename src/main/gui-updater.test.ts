import { EventEmitter } from 'node:events'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type MockUpdater = EventEmitter & {
  autoDownload: boolean
  autoInstallOnAppQuit: boolean
  allowPrerelease: boolean
  forceDevUpdateConfig: boolean
  logger: unknown
  setFeedURL: ReturnType<typeof vi.fn>
  checkForUpdates: ReturnType<typeof vi.fn>
  downloadUpdate: ReturnType<typeof vi.fn>
  quitAndInstall: ReturnType<typeof vi.fn>
}

let updater: MockUpdater
let nativeUpdater: EventEmitter
let originalEnv: NodeJS.ProcessEnv
let originalPlatform: PropertyDescriptor
let appVersion: string
let mockedFiles: Map<string, string>
let showMessageBox: ReturnType<typeof vi.fn>
let openExternal: ReturnType<typeof vi.fn>

function createUpdater(): MockUpdater {
  return Object.assign(new EventEmitter(), {
    autoDownload: true,
    autoInstallOnAppQuit: true,
    allowPrerelease: false,
    forceDevUpdateConfig: false,
    logger: null,
    setFeedURL: vi.fn(),
    checkForUpdates: vi.fn(),
    downloadUpdate: vi.fn(),
    quitAndInstall: vi.fn()
  })
}

beforeEach(() => {
  originalEnv = { ...process.env }
  originalPlatform = Object.getOwnPropertyDescriptor(process, 'platform')!
  vi.useFakeTimers()
  vi.resetModules()
  updater = createUpdater()
  nativeUpdater = new EventEmitter()
  appVersion = '0.1.0'
  mockedFiles = new Map()
  showMessageBox = vi.fn().mockResolvedValue({ response: 1 })
  openExternal = vi.fn().mockResolvedValue(undefined)
  vi.doMock('node:fs/promises', () => ({
    mkdir: vi.fn().mockResolvedValue(undefined),
    readFile: vi.fn(async (path: string) => {
      const value = mockedFiles.get(String(path))
      if (value === undefined) throw Object.assign(new Error('not found'), { code: 'ENOENT' })
      return value
    }),
    writeFile: vi.fn(async (path: string, value: string) => {
      mockedFiles.set(String(path), String(value))
    })
  }))
  vi.doMock('electron', () => ({
    app: {
      isPackaged: true,
      getAppPath: () => '/tmp/analytix-updater-test-app',
      getPath: () => '/tmp/analytix-updater-test-user-data',
      getVersion: () => appVersion,
      getLocale: () => 'en-US'
    },
    autoUpdater: nativeUpdater,
    BrowserWindow: class {},
    dialog: { showMessageBox },
    shell: { openExternal }
  }))
  vi.doMock('electron-updater', () => ({
    default: { autoUpdater: updater },
    autoUpdater: updater
  }))
})

afterEach(() => {
  process.env = originalEnv
  Object.defineProperty(process, 'platform', originalPlatform)
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.doUnmock('electron')
  vi.doUnmock('electron-updater')
  vi.doUnmock('node:fs/promises')
  vi.resetModules()
})

describe('checkGuiUpdate feed URL', () => {
  it('schedules no update traffic for exact desktop external-state isolation', async () => {
    process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES = '1'
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    const module = await import('./gui-updater')
    ;(module.initializeGuiUpdater as (...args: unknown[]) => void)(
      () => null,
      () => 'stable',
      undefined,
      undefined,
      true
    )
    await vi.runOnlyPendingTimersAsync()

    expect(fetchMock).not.toHaveBeenCalled()
    expect(updater.checkForUpdates).not.toHaveBeenCalled()
  })

  it.each([
    { platform: 'darwin', code: 'unsupported', automaticChecks: 0 },
    { platform: 'linux', code: 'not_configured', automaticChecks: 1 },
    { platform: 'win32', code: 'not_configured', automaticChecks: 1 }
  ])('projects manual metadata fetch failures without copying network exceptions ($platform)', async ({ platform, code, automaticChecks }) => {
    // Unsupported unsigned macOS and an unconfigured updater on other hosts
    // reach the same safe manual projection through different public states.
    Object.defineProperty(process, 'platform', { value: platform })
    delete process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES
    // Exercise the network-error projection, not an absent-feed early return.
    // A clean CI checkout has no Owner update configuration.
    process.env.ANALYTIX_RELEASE_BASE_URL = 'https://updates.example.invalid/analytix'
    const sentinel = 'https://updates.example.test/private/customer-pii-13900000036?token=secret'
    const fetchMock = vi.fn().mockRejectedValueOnce(new Error(sentinel))
    vi.stubGlobal('fetch', fetchMock)
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'stable')

    const result = await module.checkGuiUpdate('stable')

    expect(result).toMatchObject({
      ok: false,
      currentVersion: '0.1.0',
      code,
      channel: 'stable',
      message: expect.stringContaining('Could not read GUI update metadata for the stable channel.')
    })
    expect(JSON.stringify(result)).not.toContain(sentinel)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(updater.checkForUpdates).toHaveBeenCalledTimes(automaticChecks)
  })

  it('uses the configured analytix release base URL', async () => {
    process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES = '1'
    process.env.ANALYTIX_RELEASE_BASE_URL = 'https://updates.example.com/analytix'
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    updater.checkForUpdates.mockResolvedValue({
      updateInfo: { version: '0.2.0', releaseDate: '2026-06-06T00:00:00.000Z' },
      isUpdateAvailable: true
    })

    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'stable')

    await expect(module.checkGuiUpdate('stable')).resolves.toMatchObject({
      ok: true,
      latestVersion: '0.2.0',
      hasUpdate: true
    })
    expect(fetchMock).not.toHaveBeenCalled()
    expect(updater.setFeedURL).toHaveBeenLastCalledWith({
      provider: 'generic',
      url: 'https://updates.example.com/analytix/channels/stable/latest/'
    })
  })

  it('uses the official standard Windows update feed when none is configured', async () => {
    process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES = '1'
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    updater.checkForUpdates.mockResolvedValue({
      updateInfo: { version: '0.2.0', releaseDate: '2026-06-06T00:00:00.000Z' },
      isUpdateAvailable: true
    })

    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'stable')

    await expect(module.checkGuiUpdate('stable')).resolves.toMatchObject({
      ok: true,
      latestVersion: '0.2.0',
      hasUpdate: true
    })
    expect(fetchMock).not.toHaveBeenCalled()
    expect(updater.setFeedURL).toHaveBeenLastCalledWith({
      provider: 'generic',
      url: 'https://analytix.top/desktop/releases/standard-win/'
    })
  })

  it('uses a direct update URL before the release base URL', async () => {
    process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES = '1'
    process.env.ANALYTIX_UPDATE_URL_BETA = 'https://updates.example.com/direct/{channel}/'
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    updater.checkForUpdates.mockResolvedValue({
      updateInfo: { version: '0.2.0', releaseDate: '2026-06-06T00:00:00.000Z' },
      isUpdateAvailable: true
    })

    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'beta')

    await expect(module.checkGuiUpdate('beta')).resolves.toMatchObject({
      ok: true,
      latestVersion: '0.2.0',
      hasUpdate: true
    })
    expect(fetchMock).not.toHaveBeenCalled()
    expect(updater.setFeedURL).toHaveBeenLastCalledWith({
      provider: 'generic',
      url: 'https://updates.example.com/direct/beta/'
    })
  })

  it('projects unknown GUI updater errors in both pushed state and check results', async () => {
    process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES = '1'
    const eventSentinel = '/private/customer-pii-13900000033 updater-event-error'
    const checkSentinel = 'https://updates.example.test/private/customer-pii-13900000034?token=secret'
    const send = vi.fn()
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => ({
      isDestroyed: () => false,
      webContents: { isDestroyed: () => false, send }
    }) as never, () => 'stable')
    send.mockClear()

    updater.emit('error', new Error(eventSentinel))

    expect(module.getGuiUpdateState()).toMatchObject({
      status: 'error',
      message: 'Could not check the stable update feed. Open the download page instead.',
      code: 'unknown'
    })
    expect(JSON.stringify(send.mock.calls)).not.toContain(eventSentinel)

    updater.checkForUpdates.mockRejectedValueOnce(new Error(checkSentinel))
    const result = await module.checkGuiUpdate('stable')

    expect(result).toMatchObject({
      ok: false,
      code: 'unknown',
      channel: 'stable',
      message: 'Could not check the stable update feed. Open the download page instead.'
    })
    expect(JSON.stringify(result)).not.toContain(checkSentinel)
  })
})

describe('installGuiUpdate', () => {
  it('waits for managed runtime cleanup before asking the updater to quit and install', async () => {
    const module = await import('./gui-updater')
    let finishCleanup = (): void => {
      throw new Error('cleanup resolver was not set')
    }
    const beforeInstall = vi.fn(() => new Promise<void>((resolve) => {
      finishCleanup = resolve
    }))

    module.initializeGuiUpdater(() => null, () => 'stable', beforeInstall)
    updater.emit('update-downloaded', { version: '0.2.0', releaseDate: '2026-06-06T00:00:00.000Z' })

    const installing = module.installGuiUpdate()
    await Promise.resolve()

    expect(beforeInstall).toHaveBeenCalledTimes(1)
    expect(updater.quitAndInstall).not.toHaveBeenCalled()

    finishCleanup()
    await expect(installing).resolves.toEqual({ ok: true })
    expect(updater.quitAndInstall).toHaveBeenCalledWith(false, true)
  })

  it('reuses the same cleanup when the native updater emits before-quit-for-update', async () => {
    const module = await import('./gui-updater')
    let finishCleanup = (): void => {
      throw new Error('cleanup resolver was not set')
    }
    const beforeInstall = vi.fn(() => new Promise<void>((resolve) => {
      finishCleanup = resolve
    }))

    module.initializeGuiUpdater(() => null, () => 'stable', beforeInstall)
    updater.emit('update-downloaded', { version: '0.2.0', releaseDate: '2026-06-06T00:00:00.000Z' })

    nativeUpdater.emit('before-quit-for-update')
    const installing = module.installGuiUpdate()
    await Promise.resolve()

    expect(beforeInstall).toHaveBeenCalledTimes(1)
    expect(updater.quitAndInstall).not.toHaveBeenCalled()

    finishCleanup()
    await expect(installing).resolves.toEqual({ ok: true })
    expect(updater.quitAndInstall).toHaveBeenCalledWith(false, true)
  })

  it('projects managed-cleanup install failures in both pushed state and results', async () => {
    const sentinel = '/private/customer-pii-13900000054/updater-install-cleanup-error'
    const send = vi.fn()
    const beforeInstall = vi.fn().mockRejectedValueOnce(new Error(sentinel))
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => ({
      isDestroyed: () => false,
      webContents: { isDestroyed: () => false, send }
    }) as never, () => 'stable', beforeInstall)
    updater.emit('update-downloaded', { version: '0.2.0', releaseDate: '2026-08-27T00:00:00.000Z' })
    send.mockClear()

    const result = await module.installGuiUpdate()

    expect(result).toEqual({
      ok: false,
      currentVersion: '0.1.0',
      code: 'install_failed',
      message: 'GUI update installation failed.'
    })
    expect(module.getGuiUpdateState()).toMatchObject({
      status: 'error',
      code: 'install_failed',
      message: 'GUI update installation failed.'
    })
    expect(beforeInstall).toHaveBeenCalledOnce()
    expect(updater.quitAndInstall).not.toHaveBeenCalled()
    expect(JSON.stringify([result, module.getGuiUpdateState(), send.mock.calls])).not.toContain(sentinel)
  })

  it('projects native installer failures after managed cleanup completes', async () => {
    const sentinel = '/private/customer-pii-13900000055/updater-native-install-error'
    const send = vi.fn()
    const beforeInstall = vi.fn().mockResolvedValueOnce(undefined)
    updater.quitAndInstall.mockImplementationOnce(() => {
      throw new Error(sentinel)
    })
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => ({
      isDestroyed: () => false,
      webContents: { isDestroyed: () => false, send }
    }) as never, () => 'stable', beforeInstall)
    updater.emit('update-downloaded', { version: '0.2.0', releaseDate: '2026-08-27T00:00:00.000Z' })
    send.mockClear()

    const result = await module.installGuiUpdate()

    expect(result).toEqual({
      ok: false,
      currentVersion: '0.1.0',
      code: 'install_failed',
      message: 'GUI update installation failed.'
    })
    expect(module.getGuiUpdateState()).toMatchObject({
      status: 'error',
      code: 'install_failed',
      message: 'GUI update installation failed.'
    })
    expect(beforeInstall).toHaveBeenCalledOnce()
    expect(updater.quitAndInstall).toHaveBeenCalledOnce()
    expect(updater.quitAndInstall).toHaveBeenCalledWith(false, true)
    expect(JSON.stringify([result, module.getGuiUpdateState(), send.mock.calls])).not.toContain(sentinel)
  })
})

describe('downloadGuiUpdate', () => {
  it('projects updater download failures into fixed renderer state and results', async () => {
    process.env.ANALYTIX_ALLOW_UNSIGNED_UPDATES = '1'
    const sentinel = '/private/customer-pii-13900000035 updater-download-error'
    updater.checkForUpdates.mockResolvedValueOnce({
      updateInfo: { version: '0.2.0', releaseDate: '2026-08-26T00:00:00.000Z' },
      isUpdateAvailable: true
    })
    updater.downloadUpdate.mockRejectedValueOnce(new Error(sentinel))
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'stable')

    const result = await module.downloadGuiUpdate('stable')

    expect(result).toEqual({
      ok: false,
      currentVersion: '0.1.0',
      code: 'download_failed',
      message: 'GUI update download failed.'
    })
    expect(module.getGuiUpdateState()).toMatchObject({
      status: 'error',
      code: 'download_failed',
      message: 'GUI update download failed.'
    })
    expect(JSON.stringify([result, module.getGuiUpdateState()])).not.toContain(sentinel)
    expect(updater.checkForUpdates).toHaveBeenCalledOnce()
    expect(updater.downloadUpdate).toHaveBeenCalledOnce()
  })
})

describe('showPostUpdateReleaseNotes', () => {
  const versionStatePath = join(
    '/tmp/analytix-updater-test-user-data',
    'gui-version-state.json'
  )

  it('records the first launched version without showing a notice', async () => {
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'stable')

    await module.showPostUpdateReleaseNotes()

    expect(showMessageBox).not.toHaveBeenCalled()
    expect(JSON.parse(mockedFiles.get(versionStatePath) ?? '{}')).toEqual({
      lastSeenVersion: '0.1.0'
    })
  })

  it('shows downloaded release notes once after the version changes', async () => {
    appVersion = '0.2.0'
    process.env.ANALYTIX_CHANGELOG_URL = 'https://analytix.example.com/changelog'
    mockedFiles.set(
      versionStatePath,
      JSON.stringify({
        lastSeenVersion: '0.1.0',
        pendingUpdate: {
          version: '0.2.0',
          releaseNotes: '修复更新流程并改进启动体验。'
        }
      })
    )
    showMessageBox.mockResolvedValue({ response: 0 })
    const module = await import('./gui-updater')
    module.initializeGuiUpdater(() => null, () => 'stable', undefined, () => 'zh')

    await module.showPostUpdateReleaseNotes()
    await module.showPostUpdateReleaseNotes()

    expect(showMessageBox).toHaveBeenCalledTimes(1)
    expect(showMessageBox).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'analytix 已更新',
        message: '已更新到 analytix 0.2.0',
        detail: '修复更新流程并改进启动体验。',
        buttons: ['查看更新日志', '稍后']
      })
    )
    expect(openExternal).toHaveBeenCalledWith('https://analytix.example.com/changelog')
    expect(JSON.parse(mockedFiles.get(versionStatePath) ?? '{}')).toEqual({
      lastSeenVersion: '0.2.0'
    })
  })
})
