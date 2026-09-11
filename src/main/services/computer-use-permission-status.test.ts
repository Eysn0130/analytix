import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const probes = vi.hoisted(() => ({
  live: vi.fn(),
  configured: vi.fn(),
  capture: vi.fn(),
  requestAccessibility: vi.fn(),
  requestCapture: vi.fn(),
  sources: vi.fn(),
  open: vi.fn(),
  trace: [] as string[]
}))

vi.mock('electron', () => ({
  systemPreferences: {
    isTrustedAccessibilityClient: probes.live,
    getMediaAccessStatus: probes.capture
  },
  desktopCapturer: { getSources: probes.sources },
  shell: { openExternal: probes.open }
}))
vi.mock('@computer-use/node-mac-permissions', () => ({
  default: {
    getAuthStatus: probes.configured,
    askForAccessibilityAccess: probes.requestAccessibility,
    askForScreenCaptureAccess: probes.requestCapture
  }
}))
vi.mock('../../../packages/runtime/src/adapters/computer-use/backend-factory.js', () => ({
  selectHostControlBackend: vi.fn()
}))
vi.mock('./chrome-browser-use-service', () => ({ getChromeBrowserUseStatus: vi.fn() }))

import { getComputerUsePermissions } from './computer-use-permissions'
import { readHostPermissionSnapshot } from './computer-use-permission-status'

const originalPlatform = Object.getOwnPropertyDescriptor(process, 'platform')!
const failure = Symbol('probe failure')
const liveStates = [true, false, failure] as const
const configuredStates = ['authorized', 'granted', 'denied', undefined, failure] as const
const captureStates = [
  ['authorized', 'granted'],
  ['granted', 'granted'],
  ['not determined', 'unknown'],
  ['not-determined', 'unknown'],
  [undefined, 'unknown'],
  ['denied', 'denied'],
  ['restricted', 'denied'],
  ['', 'denied'],
  ['unexpected', 'denied'],
  [failure, 'unknown']
] as const

function platform(value: NodeJS.Platform): void {
  Object.defineProperty(process, 'platform', { configurable: true, value })
}

function reply(name: string, value: unknown): unknown {
  probes.trace.push(name)
  if (value === failure) throw new Error('SYNTHETIC_PRIVATE_PROBE_ERROR')
  return value
}

describe('OS permission snapshot behavior golden', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    probes.trace.length = 0
    platform('darwin')
  })
  afterEach(() => Object.defineProperty(process, 'platform', originalPlatform))

  for (const live of liveStates) {
    for (const configured of configuredStates) {
      for (const [capture, expectedCapture] of captureStates) {
        it(`live=${String(live)} configured=${String(configured)} capture=${String(capture)}`, async () => {
          probes.live.mockImplementation(() => reply('live', live))
          probes.configured.mockImplementation(() => reply('configured', configured))
          probes.capture.mockImplementation(() => reply('capture', capture))
          const expected = {
            platform: 'darwin',
            supported: true,
            needsPermission: true,
            accessibility: live === true ? 'granted' : 'denied',
            screenRecording: expectedCapture,
            accessibilityNeedsRestart: live !== true && configured === 'authorized'
          }
          expect(await getComputerUsePermissions()).toEqual(expected)
          expect(probes.trace).toEqual(['live', 'configured', 'capture'])
          expect(probes.live).toHaveBeenCalledExactlyOnceWith(false)
          expect(probes.configured).toHaveBeenCalledExactlyOnceWith('accessibility')
          expect(probes.capture).toHaveBeenCalledExactlyOnceWith('screen')
          expect(probes.requestAccessibility).not.toHaveBeenCalled()
          expect(probes.requestCapture).not.toHaveBeenCalled()
          expect(probes.sources).not.toHaveBeenCalled()
          expect(probes.open).not.toHaveBeenCalled()

          // The same golden binds the service adapter and the isolated evaluator.
          // This also compares the old service with its candidate before switching.
          probes.trace.length = 0
          expect(await readHostPermissionSnapshot({
            platform: 'darwin',
            liveAccessibility: () => probes.live(false),
            configuredAccessibility: async () => probes.configured('accessibility'),
            screenCapture: () => probes.capture('screen')
          })).toEqual(expected)
          expect(probes.trace).toEqual(['live', 'configured', 'capture'])
        })
      }
    }
  }

  it.each(['win32', 'linux', 'freebsd'] as const)('%s does not query or prompt the OS', async (value) => {
    platform(value)
    expect(await getComputerUsePermissions()).toEqual({
      platform: value,
      supported: true,
      needsPermission: false,
      accessibility: 'granted',
      screenRecording: 'granted',
      accessibilityNeedsRestart: false
    })
    for (const probe of Object.values(probes)) {
      if (typeof probe === 'function') expect(probe).not.toHaveBeenCalled()
    }
  })

  it('reads changed live permissions after a denied observation without persisting a grant', async () => {
    probes.live.mockReturnValueOnce(false).mockReturnValueOnce(true).mockReturnValueOnce(false)
    probes.configured.mockReturnValue('authorized')
    probes.capture.mockReturnValueOnce('denied').mockReturnValueOnce('granted').mockReturnValueOnce('denied')
    const snapshots = []
    for (let attempt = 0; attempt < 3; attempt += 1) snapshots.push(await getComputerUsePermissions())
    expect(snapshots.map(({ accessibility, screenRecording, accessibilityNeedsRestart }) => [
      accessibility, screenRecording, accessibilityNeedsRestart
    ])).toEqual([
      ['denied', 'denied', true],
      ['granted', 'granted', false],
      ['denied', 'denied', true]
    ])
  })
})
