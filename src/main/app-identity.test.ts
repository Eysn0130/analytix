import { beforeEach, describe, expect, it, vi } from 'vitest'
import path from 'node:path'

const setName = vi.fn()
const setAppUserModelId = vi.fn()
const setPath = vi.fn()
const getPath = vi.fn()

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
    setName.mockReset()
    setAppUserModelId.mockReset()
    setPath.mockReset()
    getPath.mockReset()
    getPath.mockReturnValue(path.resolve('/tmp/analytix-test-app-data'))
    vi.resetModules()
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
      path.resolve('/tmp/analytix-test-app-data', 'analytix')
    )
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
    configureAppIdentity({ ANALYTIX_USER_DATA_DIR: '/tmp/analytix-isolated-user-data' } as NodeJS.ProcessEnv)
    expect(getPath).not.toHaveBeenCalled()
    expect(setPath).toHaveBeenCalledWith('userData', path.resolve('/tmp/analytix-isolated-user-data'))
  })
})
