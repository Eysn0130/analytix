import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPermissionEnrollment } from './computer-use-permission-access'
import { readHostPermissionSnapshot } from './computer-use-permission-status'

const effects = vi.hoisted(() => ({
  live: vi.fn(), screen: vi.fn(), configured: vi.fn(),
  accessibility: vi.fn(), capture: vi.fn(), sources: vi.fn(), open: vi.fn(), load: vi.fn()
}))
vi.mock('electron', () => ({
  systemPreferences: { isTrustedAccessibilityClient: effects.live, getMediaAccessStatus: effects.screen },
  desktopCapturer: { getSources: effects.sources },
  shell: { openExternal: effects.open }
}))
vi.mock('../../../packages/runtime/src/adapters/computer-use/backend-factory.js', () => ({
  selectHostControlBackend: vi.fn()
}))
vi.mock('./chrome-browser-use-service', () => ({ getChromeBrowserUseStatus: vi.fn() }))

const originalPlatform = Object.getOwnPropertyDescriptor(process, 'platform')!
const deniedSnapshot = {
  platform: 'darwin', supported: true, needsPermission: true,
  accessibility: 'denied', screenRecording: 'denied', accessibilityNeedsRestart: false
}
const settingsURL = 'x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture'
const syntheticError = new Error('SYNTHETIC_PERMISSION_ERROR_DO_NOT_PROJECT')

async function service(available: boolean, methods: boolean, implementation: string) {
  const importNative = () => {
    effects.load()
    if (!available) throw syntheticError
    return { default: {
      getAuthStatus: effects.configured,
      ...(methods ? {
        askForAccessibilityAccess: effects.accessibility,
        askForScreenCaptureAccess: effects.capture
      } : {})
    } }
  }
  if (implementation === 'candidate') {
    const access = createPermissionEnrollment({
      importNative: async () => importNative(),
      promptAccessibility: () => effects.live(true),
      promptScreenCapture: () => effects.sources({ types: ['screen'], thumbnailSize: { width: 1, height: 1 } }),
      openScreenSettings: () => effects.open(settingsURL)
    })
    return {
      async requestComputerUsePermission(kind: 'accessibility' | 'screenRecording') {
        await access.request(kind, process.platform)
        return readHostPermissionSnapshot({
          platform: process.platform,
          liveAccessibility: () => effects.live(false),
          configuredAccessibility: access.configuredAccessibility,
          screenCapture: () => effects.screen('screen')
        })
      }
    }
  }
  vi.doMock('@computer-use/node-mac-permissions', importNative)
  return import('./computer-use-permissions')
}

describe.each(['current', 'candidate'])('%s OS permission enrollment behavior golden', (implementation) => {
  const subject = (available: boolean, methods = true) => service(available, methods, implementation)
  beforeEach(() => {
    vi.resetModules()
    vi.resetAllMocks()
    Object.defineProperty(process, 'platform', { configurable: true, value: 'darwin' })
    effects.live.mockReturnValue(false)
    effects.configured.mockReturnValue('denied')
    effects.screen.mockReturnValue('denied')
    effects.sources.mockResolvedValue([])
    effects.open.mockResolvedValue(undefined)
  })
  afterEach(() => Object.defineProperty(process, 'platform', originalPlatform))

  for (const kind of ['accessibility', 'screenRecording'] as const) {
    for (const available of [true, false]) {
      it(`${kind}: ${available ? 'native' : 'fallback'} enrollment preserves denied state`, async () => {
        const api = await subject(available)
        expect(await api.requestComputerUsePermission(kind)).toEqual(deniedSnapshot)
        expect(effects.load).toHaveBeenCalledOnce()
        expect(effects.configured).toHaveBeenCalledTimes(available ? 1 : 0)
        expect(effects.screen).toHaveBeenCalledExactlyOnceWith('screen')
        if (kind === 'accessibility') {
          expect(effects.accessibility).toHaveBeenCalledTimes(available ? 1 : 0)
          expect(effects.live.mock.calls).toEqual(available ? [[false]] : [[true], [false]])
          expect(effects.sources).not.toHaveBeenCalled()
          expect(effects.open).not.toHaveBeenCalled()
        } else {
          expect(effects.capture.mock.calls).toEqual(available ? [[true]] : [])
          expect(effects.sources.mock.calls).toEqual(available ? [] : [[{
            types: ['screen'], thumbnailSize: { width: 1, height: 1 }
          }]])
          expect(effects.open).toHaveBeenCalledExactlyOnceWith(settingsURL)
          expect(effects.live).toHaveBeenCalledExactlyOnceWith(false)
        }
        expect(await api.requestComputerUsePermission(kind)).toEqual(deniedSnapshot)
        expect(effects.load).toHaveBeenCalledOnce()
      })

      it(`${kind}: enrollment errors do not escape or grant permission (${available})`, async () => {
        effects.accessibility.mockImplementation(() => { throw syntheticError })
        effects.capture.mockImplementation(() => { throw syntheticError })
        effects.sources.mockRejectedValue(syntheticError)
        effects.live.mockImplementation((prompt) => {
          if (prompt) throw syntheticError
          return false
        })
        const api = await subject(available)
        expect(await api.requestComputerUsePermission(kind)).toEqual(deniedSnapshot)
        expect(effects.open).toHaveBeenCalledTimes(kind === 'screenRecording' && !available ? 1 : 0)
      })
    }

    it(`${kind}: missing native method uses the Electron fallback`, async () => {
      const api = await subject(true, false)
      expect(await api.requestComputerUsePermission(kind)).toEqual(deniedSnapshot)
      expect(effects.accessibility).not.toHaveBeenCalled()
      expect(effects.capture).not.toHaveBeenCalled()
      if (kind === 'accessibility') expect(effects.live.mock.calls).toEqual([[true], [false]])
      else expect(effects.sources).toHaveBeenCalledOnce()
    })

    it(`${kind}: non-macOS performs no enrollment or native loading`, async () => {
      Object.defineProperty(process, 'platform', { configurable: true, value: 'win32' })
      const api = await subject(false)
      expect(await api.requestComputerUsePermission(kind)).toEqual({
        ...deniedSnapshot, platform: 'win32', needsPermission: false,
        accessibility: 'granted', screenRecording: 'granted'
      })
      for (const effect of Object.values(effects)) expect(effect).not.toHaveBeenCalled()
    })
  }

  it('awaits isolated fallback capture, then opens settings, then observes the current state', async () => {
    let finish!: (value: unknown[]) => void
    effects.sources.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    const api = await subject(false)
    const pending = api.requestComputerUsePermission('screenRecording')
    await vi.waitFor(() => expect(effects.sources).toHaveBeenCalledOnce())
    expect(effects.open).not.toHaveBeenCalled()
    expect(effects.screen).not.toHaveBeenCalled()
    finish([])
    expect(await pending).toEqual(deniedSnapshot)
    expect(effects.open.mock.invocationCallOrder[0]).toBeLessThan(effects.screen.mock.invocationCallOrder[0])
  })

  it('returns current status if System Settings cannot be opened', async () => {
    effects.open.mockRejectedValue(syntheticError)
    const api = await subject(true)
    expect(await api.requestComputerUsePermission('screenRecording')).toEqual(deniedSnapshot)
  })
})

describe('permission native-helper session invariants', () => {
  it('shares in-flight import and retries only after a new session', async () => {
    let finish!: (value: unknown) => void
    const load = vi.fn(() => new Promise((resolve) => { finish = resolve }))
    const ports = {
      importNative: load, promptAccessibility: vi.fn(), promptScreenCapture: vi.fn(), openScreenSettings: vi.fn()
    }
    const session = createPermissionEnrollment(ports)
    const pending = [session.configuredAccessibility(), session.configuredAccessibility()]
    await Promise.resolve()
    expect(load).toHaveBeenCalledOnce()
    finish({ default: { getAuthStatus: () => 'authorized' } })
    expect(await Promise.all(pending)).toEqual(['authorized', 'authorized'])
    expect(await session.configuredAccessibility()).toBe('authorized')
    expect(load).toHaveBeenCalledOnce()
    load.mockResolvedValue({ getAuthStatus: () => 'denied' })
    expect(await createPermissionEnrollment(ports).configuredAccessibility()).toBe('denied')
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('consumes asynchronous native rejection without waiting for a user decision', async () => {
    let reject!: (reason: unknown) => void
    const decision = new Promise((_resolve, rejectDecision) => { reject = rejectDecision })
    const session = createPermissionEnrollment({
      importNative: async () => ({ askForAccessibilityAccess: () => decision }),
      promptAccessibility: vi.fn(), promptScreenCapture: vi.fn(), openScreenSettings: vi.fn()
    })
    await session.request('accessibility', 'darwin')
    reject(syntheticError)
    await Promise.resolve()
  })
})
