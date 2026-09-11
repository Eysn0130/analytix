import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const electronMocks = vi.hoisted(() => ({
  registerSchemesAsPrivileged: vi.fn(),
  handle: vi.fn(),
  fetch: vi.fn(),
  stat: vi.fn(),
  app: {
    isPackaged: true,
    getAppPath: vi.fn(() => '/Applications/Analytix.app/Contents/Resources/app.asar')
  }
}))

vi.mock('electron', () => ({
  app: electronMocks.app,
  net: { fetch: electronMocks.fetch },
  protocol: {
    registerSchemesAsPrivileged: electronMocks.registerSchemesAsPrivileged,
    handle: electronMocks.handle
  }
}))

vi.mock('node:fs/promises', () => ({
  stat: electronMocks.stat
}))

import {
  PACKAGED_RENDERER_ENTRY_URL,
  PACKAGED_RENDERER_ORIGIN,
  PACKAGED_RENDERER_SCHEME,
  installPackagedRendererProtocol,
  isPackagedRendererNavigationURL,
  packagedRendererURL,
  registerPackagedRendererScheme,
  resolvePackagedRendererAssetPath
} from './packaged-renderer-protocol'

beforeEach(() => {
  electronMocks.registerSchemesAsPrivileged.mockReset()
  electronMocks.handle.mockReset()
  electronMocks.fetch.mockReset()
  electronMocks.stat.mockReset()
  electronMocks.stat.mockResolvedValue({ isFile: () => true })
  electronMocks.app.isPackaged = true
})

describe('packaged renderer protocol', () => {
  it('registers one standard secure origin without CSP or service-worker bypasses', () => {
    registerPackagedRendererScheme()

    expect(electronMocks.registerSchemesAsPrivileged).toHaveBeenCalledWith([{
      scheme: PACKAGED_RENDERER_SCHEME,
      privileges: {
        standard: true,
        secure: true,
        supportFetchAPI: true,
        corsEnabled: true
      }
    }])
    expect(electronMocks.registerSchemesAsPrivileged.mock.calls[0]?.[0]?.[0]?.privileges)
      .not.toHaveProperty('bypassCSP')
    expect(electronMocks.registerSchemesAsPrivileged.mock.calls[0]?.[0]?.[0]?.privileges)
      .not.toHaveProperty('allowServiceWorkers')
  })

  it('uses one fixed renderer origin and only adds an encoded thread query', () => {
    expect(PACKAGED_RENDERER_ORIGIN).toBe('analytix-app://renderer')
    expect(PACKAGED_RENDERER_ENTRY_URL).toBe('analytix-app://renderer/index.html')
    expect(packagedRendererURL()).toBe('analytix-app://renderer/index.html')
    expect(packagedRendererURL('thr one/二')).toBe(
      'analytix-app://renderer/index.html?threadId=thr+one%2F%E4%BA%8C'
    )
    expect(isPackagedRendererNavigationURL(PACKAGED_RENDERER_ENTRY_URL)).toBe(true)
    expect(isPackagedRendererNavigationURL(packagedRendererURL('thr_1'))).toBe(true)
    for (const hostile of [
      'analytix-app://renderer/assets/index.js',
      'analytix-app://renderer/index.html?extra=1',
      'analytix-app://renderer/index.html?threadId=',
      'analytix-app://renderer/index.html#fragment',
      'analytix-app://user@renderer/index.html',
      'analytix-app://attacker/index.html'
    ]) {
      expect(isPackagedRendererNavigationURL(hostile), hostile).toBe(false)
    }
  })

  it('maps only allowlisted decoded path segments below the fixed renderer root', () => {
    const root = '/Applications/Analytix.app/Contents/Resources/app.asar/out/renderer'
    expect(resolvePackagedRendererAssetPath(
      'analytix-app://renderer/assets/index-abc.js?cache=1',
      root
    )).toBe(`${root}/assets/index-abc.js`)
    expect(resolvePackagedRendererAssetPath(
      'analytix-app://renderer/%E6%96%87%E4%BB%B6.js',
      root
    )).toBeUndefined()
    expect(resolvePackagedRendererAssetPath(
      'analytix-app://renderer/flow-runtime/analysis_flow/index.html',
      root
    )).toBe(`${root}/flow-runtime/analysis_flow/index.html`)

    for (const hostile of [
      '',
      'analytix-app://renderer/',
      'analytix-app://renderer//index.html',
      'analytix-app://other/index.html',
      'analytix-app://user@renderer/index.html',
      'analytix-app://renderer:99/index.html',
      'analytix-app://renderer/%2e%2e/private',
      'analytix-app://renderer/%2Fprivate',
      'analytix-app://renderer/%5cprivate',
      'analytix-app://renderer/%00private',
      'analytix-app://renderer/assets/file%3Astream',
      'analytix-app://renderer/private/config.json',
      'file:///Applications/Analytix.app/Contents/Resources/app.asar/out/renderer/index.html'
    ]) {
      expect(resolvePackagedRendererAssetPath(hostile, root), hostile).toBeUndefined()
    }
  })

  it('installs a read-only ASAR-backed handler and preserves the file fuse boundary', async () => {
    electronMocks.fetch.mockResolvedValue(new Response('ok', {
      status: 200,
      headers: { 'Content-Type': 'application/javascript' }
    }))
    installPackagedRendererProtocol()

    expect(electronMocks.handle).toHaveBeenCalledTimes(1)
    expect(electronMocks.handle).toHaveBeenCalledWith(
      PACKAGED_RENDERER_SCHEME,
      expect.any(Function)
    )
    const handler = electronMocks.handle.mock.calls[0]?.[1] as
      ((request: Request) => Promise<Response>)
    const response = await handler(new Request(
      'analytix-app://renderer/assets/index.js',
      { method: 'GET' }
    ))
    expect(response.status).toBe(200)
    expect(response.headers.get('Cross-Origin-Resource-Policy')).toBe('same-origin')
    expect(response.headers.get('X-Content-Type-Options')).toBe('nosniff')
    expect(electronMocks.fetch).toHaveBeenCalledWith(
      'file:///Applications/Analytix.app/Contents/Resources/app.asar/out/renderer/assets/index.js'
    )

    const rejected = await handler(new Request(
      'analytix-app://renderer/index.html',
      { method: 'POST' }
    ))
    expect(rejected.status).toBe(405)
    expect(electronMocks.fetch).toHaveBeenCalledTimes(1)
  })

  it('does not install the packaged handler for a development renderer', () => {
    electronMocks.app.isPackaged = false
    registerPackagedRendererScheme()
    installPackagedRendererProtocol()
    expect(electronMocks.registerSchemesAsPrivileged).not.toHaveBeenCalled()
    expect(electronMocks.handle).not.toHaveBeenCalled()
  })

  it('wires the privileged scheme before ready and never loads the packaged renderer through file', () => {
    const source = readFileSync(resolve('src/main/index.ts'), 'utf8')
    const ownerBranchIndex = source.indexOf('} else if (gotSingleInstanceLock) {')
    const registerIndex = source.indexOf('registerPackagedRendererScheme()', ownerBranchIndex)
    const readyIndex = source.indexOf('app.whenReady().then')
    const installIndex = source.indexOf('installPackagedRendererProtocol()', readyIndex)
    const createIndex = source.indexOf('createWindow({', readyIndex)

    expect(registerIndex).toBeGreaterThan(ownerBranchIndex)
    expect(registerIndex).toBeLessThan(readyIndex)
    expect(installIndex).toBeGreaterThan(readyIndex)
    expect(installIndex).toBeLessThan(createIndex)
    expect(source).toContain('appWindow.loadURL(packagedRendererURL(options.initialThreadId))')
    expect(source).not.toContain("join(__dirname, '../renderer/index.html')")
    expect(source).not.toContain('appWindow.loadFile(')
    expect(source).toContain(
      'app.isPackaged && !isPackagedRendererNavigationURL(navigationUrl)'
    )
  })
})
