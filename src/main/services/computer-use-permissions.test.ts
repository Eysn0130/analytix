import { beforeEach, describe, expect, it, vi } from 'vitest'

const testMocks = vi.hoisted(() => ({
  getChromeBrowserUseStatus: vi.fn(),
  selectHostControlBackend: vi.fn()
}))

vi.mock('electron', () => ({
  desktopCapturer: { getSources: vi.fn() },
  shell: { openExternal: vi.fn() },
  systemPreferences: {
    getMediaAccessStatus: vi.fn(() => 'denied'),
    isTrustedAccessibilityClient: vi.fn(() => false)
  }
}))

vi.mock('../../../packages/runtime/src/adapters/computer-use/backend-factory.js', () => ({
  selectHostControlBackend: testMocks.selectHostControlBackend
}))

vi.mock('./chrome-browser-use-service', () => ({
  getChromeBrowserUseStatus: testMocks.getChromeBrowserUseStatus
}))

vi.mock('@computer-use/nut-js', () => ({
  get screen(): never {
    throw new Error('/private/customer-pii-13900000040/nut-js-import-error')
  }
}))

import { getComputerUseDoctor } from './computer-use-permissions'

describe('getComputerUseDoctor public diagnostics', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('projects backend and adapter failures without raw diagnostic reasons', async () => {
    const readinessSentinel = '/private/customer-pii-13900000038/backend-readiness-error'
    const fallbackSentinel = '/private/customer-pii-13900000039/backend-fallback-error'
    const chromeSentinel = '/private/customer-pii-13900000041/chrome-diagnostics-error'
    testMocks.selectHostControlBackend.mockResolvedValue({
      backend: {},
      readiness: { available: false, reason: readinessSentinel },
      preferredBackendId: 'analytix-computer-use',
      selectedBackendId: 'nut-js',
      fallbackReason: fallbackSentinel
    })
    testMocks.getChromeBrowserUseStatus.mockRejectedValue(new Error(chromeSentinel))

    const result = await getComputerUseDoctor()

    expect(result.backend).toMatchObject({
      preferred: 'analytix-computer-use',
      selected: 'nut-js',
      available: false,
      reason: 'No computer-use backend is available.'
    })
    expect(result.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'backend-analytix-computer-use',
        status: 'warning',
        message: 'Analytix Computer Use MCP backend 未被选中；runtime 会在可行时降级 fallback。'
      }),
      expect.objectContaining({
        id: 'backend-nut-js',
        status: 'failed',
        message: 'nut-js fallback modules are unavailable.'
      }),
      expect.objectContaining({
        id: 'chrome-browser-use-diagnostics',
        status: 'warning',
        message: 'Chrome Browser Use diagnostics are unavailable.'
      })
    ]))
    expect(testMocks.selectHostControlBackend).toHaveBeenCalledOnce()
    expect(testMocks.getChromeBrowserUseStatus).toHaveBeenCalledOnce()
    const serialized = JSON.stringify(result)
    expect([
      readinessSentinel,
      fallbackSentinel,
      '/private/customer-pii-13900000040/nut-js-import-error',
      chromeSentinel
    ].filter((sentinel) => serialized.includes(sentinel))).toEqual([])
  })
})
