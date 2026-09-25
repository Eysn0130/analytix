import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import path from 'node:path'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'

const setName = vi.fn()
const setAppUserModelId = vi.fn()
const setPath = vi.fn()
const getPath = vi.fn()
let appDataRoot: string

vi.mock('electron', () => ({
  app: {
    setName,
    setAppUserModelId,
    setPath,
    getPath
  }
}))

describe('app identity bootstrap', () => {
  beforeEach(() => {
    appDataRoot = mkdtempSync(path.join(tmpdir(), 'analytix-app-identity-'))
    setName.mockReset()
    setAppUserModelId.mockReset()
    setPath.mockReset()
    getPath.mockReset()
    getPath.mockReturnValue(appDataRoot)
    vi.resetModules()
  })

  afterEach(() => {
    rmSync(appDataRoot, { recursive: true, force: true })
  })

  it('uses the visible product name while pinning the lowercase userData directory', async () => {
    const {
      configureAppIdentity,
      APP_DISPLAY_NAME,
      APP_PRODUCT_NAME,
      APP_USER_DATA_DIRECTORY_NAME
    } = await import('./app-identity')
    configureAppIdentity()
    expect(setName).toHaveBeenCalledTimes(1)
    expect(setName).toHaveBeenCalledWith(APP_PRODUCT_NAME)
    expect(APP_PRODUCT_NAME).toBe('Analytix')
    expect(APP_DISPLAY_NAME).toBe('Analytix')
    expect(APP_USER_DATA_DIRECTORY_NAME).toBe('analytix')
    expect(getPath).toHaveBeenCalledWith('appData')
    expect(setPath).toHaveBeenCalledWith(
      'userData',
      path.resolve(appDataRoot, 'analytix')
    )
    expect(setPath).not.toHaveBeenCalledWith('sessionData', expect.anything())
  })

  it('does not call app.setAppUserModelId (caller responsibility on win32)', async () => {
    // setAppUserModelId 仍然由 main/index.ts 里的 win32 分支调用,
    // 这里只验证 configureAppIdentity 自己不重复设置。
    const { configureAppIdentity } = await import('./app-identity')
    configureAppIdentity()
    expect(setAppUserModelId).not.toHaveBeenCalled()
  })

  it('can isolate Electron userData through the internal override env', async () => {
    const { configureAppIdentity } = await import('./app-identity')
    const isolated = path.join(appDataRoot, 'isolated-user-data')
    configureAppIdentity({ ANALYTIX_USER_DATA_DIR: isolated } as NodeJS.ProcessEnv)
    expect(getPath).not.toHaveBeenCalled()
    expect(setPath).toHaveBeenCalledWith('userData', path.resolve(isolated))
  })

  it('starts a versioned macOS Chromium session only after userData migration', async () => {
    const { configureAppIdentity, configureMacChromiumSessionData } = await import('./app-identity')
    const userData = path.resolve(appDataRoot, 'analytix')
    getPath.mockImplementation((name: string) => name === 'userData' ? userData : appDataRoot)
    configureAppIdentity()
    expect(setPath).not.toHaveBeenCalledWith('sessionData', expect.anything())
    configureMacChromiumSessionData()
    if (process.platform === 'darwin') {
      expect(setPath).toHaveBeenCalledWith('sessionData', path.join(userData, 'chromium-session-no-keychain-v1'))
    } else {
      expect(setPath).not.toHaveBeenCalledWith('sessionData', expect.anything())
    }
  })
})
