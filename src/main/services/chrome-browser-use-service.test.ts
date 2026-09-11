import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

const electronMocks = vi.hoisted(() => ({
  openExternal: vi.fn()
}))

vi.mock('electron', () => ({
  shell: { openExternal: electronMocks.openExternal }
}))

import {
  ensureChromeBrowserUseNativeHostRegistration,
  getChromeBrowserUseStatus,
  inspectChromeExtensionStatus,
  inspectChromeBrowserUseStatus,
  inspectChromeNativeHostStatus,
  openChromeBrowserUseExtensionPage,
  readWindowsRegistryDefaultValueFromOutput
} from './chrome-browser-use-service'

const EXTENSION_ID = 'bccibejdpcjcdlcnbpempcpjgladapgk'
const NATIVE_HOST_NAME = 'top.analytix.codexextension'
const EXPECTED_ORIGIN = `chrome-extension://${EXTENSION_ID}/`

const tempRoots: string[] = []

function createTempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-chrome-use-'))
  tempRoots.push(root)
  return root
}

function writeJson(path: string, value: unknown): void {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`, 'utf8')
}

function createChromeProfile(root: string, options?: { disabled?: boolean }): string {
  const userDataDirectory = join(root, 'Chrome')
  const profilePath = join(userDataDirectory, 'Default')
  mkdirSync(profilePath, { recursive: true })
  mkdirSync(join(profilePath, 'Extensions', EXTENSION_ID, '1.0.0'), { recursive: true })
  writeJson(join(userDataDirectory, 'Local State'), {
    profile: {
      last_used: 'Default'
    }
  })
  writeJson(join(profilePath, 'Preferences'), {
    extensions: {
      settings: {
        [EXTENSION_ID]: {
          state: options?.disabled ? 0 : 1
        }
      }
    }
  })
  return userDataDirectory
}

function createNativeHostManifest(root: string, options?: { invalidOrigin?: boolean; missingHost?: boolean }): string {
  const manifestPath = join(root, `${NATIVE_HOST_NAME}.json`)
  const hostPath = join(root, 'extension-host')
  if (!options?.missingHost) {
    writeFileSync(hostPath, '', 'utf8')
  }
  writeJson(manifestPath, {
    name: NATIVE_HOST_NAME,
    description: 'Analytix Chrome Browser Use native messaging host',
    path: hostPath,
    type: 'stdio',
    allowed_origins: [
      options?.invalidOrigin ? `chrome-extension://wrong/` : EXPECTED_ORIGIN
    ]
  })
  return manifestPath
}

function createFakeBrowserClient(path: string, browserCount: number): void {
  writeFileSync(path, [
    'export async function setupBrowserRuntime({ globals }) {',
    '  globals.agent = {',
    '    browsers: {',
    `      list: async () => Array.from({ length: ${browserCount} }, (_, index) => ({ id: String(index + 1) }))`,
    '    }',
    '  }',
    '}',
    ''
  ].join('\n'), 'utf8')
}

afterEach(() => {
  electronMocks.openExternal.mockReset()
  for (const root of tempRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true })
  }
})

describe('chrome-browser-use-service', () => {
  it('detects the configured Chrome extension in the selected profile', () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)

    const status = inspectChromeExtensionStatus({
      platform: 'darwin',
      homedir: root,
      env: { ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory }
    })

    expect(status.status).toBe('enabled')
    expect(status.selectedProfileDirectory).toBe('Default')
    expect(status.profiles[0]?.enabled).toBe(true)
  })

  it('distinguishes an installed-but-disabled extension from a missing extension', () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root, { disabled: true })

    const status = inspectChromeExtensionStatus({
      platform: 'darwin',
      homedir: root,
      env: { ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory }
    })

    expect(status.status).toBe('disabled')
    expect(status.profiles[0]?.installed).toBe(true)
    expect(status.profiles[0]?.enabled).toBe(false)
  })

  it('validates a native host manifest and expected extension origin', () => {
    const root = createTempRoot()
    const manifestPath = createNativeHostManifest(root)

    const status = inspectChromeNativeHostStatus({
      platform: 'win32',
      homedir: root,
      env: { ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath }
    })

    expect(status.status).toBe('configured')
    expect(status.expectedOrigin).toBe(EXPECTED_ORIGIN)
    expect(status.manifestPath).toBe(manifestPath)
    expect(status.hostPathExists).toBe(true)
  })

  it('rejects a native host manifest when the host executable path is missing', () => {
    const root = createTempRoot()
    const manifestPath = createNativeHostManifest(root, { missingHost: true })

    const status = inspectChromeNativeHostStatus({
      platform: 'darwin',
      homedir: root,
      env: { ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath }
    })

    expect(status.status).toBe('invalid')
    expect(status.hostPathExists).toBe(false)
    expect(status.problem).toContain('host executable path')
  })

  it('detects a system-level Windows native host registry entry', () => {
    const root = createTempRoot()
    const manifestPath = createNativeHostManifest(root)

    const status = inspectChromeNativeHostStatus({
      platform: 'win32',
      homedir: root,
      env: {},
      readWindowsRegistryDefaultValue: (registryKey) =>
        registryKey.startsWith('HKLM\\') ? manifestPath : null
    })

    expect(status.status).toBe('configured')
    expect(status.registryKey).toContain('HKLM')
    expect(status.registryManifestPath).toBe(manifestPath)
  })

  it('detects a 32-bit Windows native host registry entry before the 64-bit view', () => {
    const root = createTempRoot()
    const manifestPath = createNativeHostManifest(root)

    const status = inspectChromeNativeHostStatus({
      platform: 'win32',
      homedir: root,
      env: {},
      readWindowsRegistryDefaultValue: (registryKey) =>
        registryKey.includes('WOW6432Node') ? manifestPath : null
    })

    expect(status.status).toBe('configured')
    expect(status.registryKey).toContain('WOW6432Node')
    expect(status.registryManifestPath).toBe(manifestPath)
  })

  it('parses localized Windows default registry value names', () => {
    const output = [
      '',
      'HKEY_CURRENT_USER\\Software\\Google\\Chrome\\NativeMessagingHosts\\top.analytix.codexextension',
      '    (默认)    REG_SZ    "C:\\Users\\sun\\AppData\\Local\\Analytix\\extension\\top.analytix.codexextension.json"',
      ''
    ].join('\r\n')

    expect(readWindowsRegistryDefaultValueFromOutput(output)).toBe(
      'C:\\Users\\sun\\AppData\\Local\\Analytix\\extension\\top.analytix.codexextension.json'
    )
  })

  it('reports a missing Windows native host registry key instead of guessing a connection', () => {
    const root = createTempRoot()

    const status = inspectChromeNativeHostStatus({
      platform: 'win32',
      homedir: root,
      env: {},
      readWindowsRegistryDefaultValue: () => null
    })

    expect(status.status).toBe('missing')
    expect(status.registryKey).toContain(NATIVE_HOST_NAME)
  })

  it('registers the bundled Windows native host for the current user', () => {
    const root = createTempRoot()
    const resourcesPath = join(root, 'resources')
    const localAppData = join(root, 'LocalAppData')
    const hostPath = join(resourcesPath, 'managed-chrome', 'chrome', 'extension-host', 'windows', 'x64', 'extension-host.exe')
    mkdirSync(join(hostPath, '..'), { recursive: true })
    writeFileSync(hostPath, '', 'utf8')

    const regCalls: string[][] = []
    const result = ensureChromeBrowserUseNativeHostRegistration({
      platform: 'win32',
      arch: 'x64',
      homedir: root,
      resourcesPath,
      env: { LOCALAPPDATA: localAppData },
      execFileSync: (_command, args) => {
        regCalls.push(args.map(String))
        return Buffer.from('')
      }
    })

    const manifestPath = join(localAppData, 'Analytix', 'extension', `${NATIVE_HOST_NAME}.json`)
    expect(result).toMatchObject({
      ok: true,
      manifestPath,
      hostPath,
      registryKey: `HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\${NATIVE_HOST_NAME}`
    })
    expect(regCalls[0]).toEqual([
      'add',
      `HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\${NATIVE_HOST_NAME}`,
      '/ve',
      '/t',
      'REG_SZ',
      '/d',
      manifestPath,
      '/f'
    ])
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8')) as Record<string, unknown>
    expect(manifest).toMatchObject({
      name: NATIVE_HOST_NAME,
      type: 'stdio',
      path: hostPath,
      allowed_origins: [EXPECTED_ORIGIN]
    })
  })

  it('does not use the development managed-chrome directory when packaged resources are present', () => {
    const root = createTempRoot()
    const result = ensureChromeBrowserUseNativeHostRegistration({
      platform: 'win32',
      arch: 'x64',
      homedir: root,
      resourcesPath: join(root, 'resources'),
      env: { LOCALAPPDATA: join(root, 'LocalAppData') }
    })

    expect(result).toMatchObject({
      ok: false,
      reason: 'Bundled Analytix Chrome native host executable was not found.'
    })
  })

  it('keeps a configured extension and native host in pending-verification state without a handshake probe', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)
    const missingClientPath = join(root, 'missing-browser-client.mjs')

    const status = await inspectChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath,
        ANALYTIX_CHROME_BROWSER_CLIENT_PATH: missingClientPath
      }
    })

    expect(status.extension.status).toBe('enabled')
    expect(status.nativeHost.status).toBe('configured')
    expect(status.state).toBe('configuredNotVerified')
    expect(status.connected).toBe(false)
    expect(status.connectionProbe.status).toBe('unavailable')
  })

  it('marks Chrome Browser Use connected only when the browser-client handshake sees a browser backend', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)
    const browserClientPath = join(root, 'browser-client.mjs')
    createFakeBrowserClient(browserClientPath, 1)

    const status = await inspectChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath,
        ANALYTIX_CHROME_BROWSER_CLIENT_PATH: browserClientPath
      }
    })

    expect(status.state).toBe('connected')
    expect(status.connected).toBe(true)
    expect(status.connectionProbe).toMatchObject({
      status: 'passed',
      browserClientPath
    })
  })

  it('keeps configured Chrome Browser Use pending when browser-client finds no Chrome backend', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)
    const browserClientPath = join(root, 'browser-client.mjs')
    createFakeBrowserClient(browserClientPath, 0)

    const status = await inspectChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath,
        ANALYTIX_CHROME_BROWSER_CLIENT_PATH: browserClientPath
      }
    })

    expect(status.state).toBe('configuredNotVerified')
    expect(status.connected).toBe(false)
    expect(status.connectionProbe.status).toBe('unavailable')
    expect(status.connectionProbe.problem).toContain('no Chrome extension backend')
  })

  it('resolves browser-client next to the Windows native host plugin root', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const pluginRoot = join(root, 'analytix-bundled', 'chrome', 'latest')
    const browserClientPath = join(pluginRoot, 'scripts', 'browser-client.mjs')
    const hostPath = join(pluginRoot, 'extension-host', 'windows', 'x64', 'extension-host.exe')
    const manifestPath = join(root, `${NATIVE_HOST_NAME}.json`)
    mkdirSync(join(pluginRoot, 'scripts'), { recursive: true })
    mkdirSync(join(pluginRoot, 'extension-host', 'windows', 'x64'), { recursive: true })
    writeFileSync(hostPath, '', 'utf8')
    createFakeBrowserClient(browserClientPath, 1)
    writeJson(manifestPath, {
      name: NATIVE_HOST_NAME,
      path: hostPath,
      type: 'stdio',
      allowed_origins: [EXPECTED_ORIGIN]
    })

    const status = await inspectChromeBrowserUseStatus({
      platform: 'win32',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath
      }
    })

    expect(status.connected).toBe(true)
    expect(status.connectionProbe.browserClientPath).toBe(browserClientPath)
  })

  it('marks Chrome Browser Use connected only after a real probe passes', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)

    const status = await inspectChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath
      },
      probeConnection: async () => ({ status: 'passed' })
    })

    expect(status.state).toBe('connected')
    expect(status.connected).toBe(true)
    expect(status.connectionProbe.status).toBe('passed')
  })

  it('keeps configured Chrome Browser Use disconnected when the real probe fails', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)

    const status = await inspectChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath
      },
      probeConnection: async () => ({ status: 'failed', problem: 'native host pipe closed' })
    })

    expect(status.state).toBe('disconnected')
    expect(status.connected).toBe(false)
    expect(status.reason).toBe('native host pipe closed')
  })

  it('projects renderer diagnostics without private paths or raw probe errors', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)
    const errorSentinel = '/private/customer-pii-13900000037/native-host-pipe-error'
    let probeCalls = 0

    const status = await getChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath
      },
      probeConnection: async () => {
        probeCalls += 1
        throw new Error(errorSentinel)
      }
    })

    expect(status).toMatchObject({
      platform: 'darwin',
      connected: false,
      state: 'disconnected',
      extension: { status: 'enabled' },
      nativeHost: { status: 'configured' },
      connectionProbe: { status: 'failed' }
    })
    expect(probeCalls).toBe(1)
    expect(JSON.stringify(status)).not.toContain(root)
    expect(JSON.stringify(status)).not.toContain(errorSentinel)
  })

  it('reports a configured but missing browser-client probe path as unavailable', async () => {
    const root = createTempRoot()
    const userDataDirectory = createChromeProfile(root)
    const manifestPath = createNativeHostManifest(root)
    const missingClientPath = join(root, 'missing-browser-client.mjs')

    const status = await inspectChromeBrowserUseStatus({
      platform: 'darwin',
      homedir: root,
      env: {
        ANALYTIX_CHROME_USER_DATA_DIR: userDataDirectory,
        ANALYTIX_CHROME_NATIVE_HOST_MANIFEST_PATH: manifestPath,
        ANALYTIX_CHROME_BROWSER_CLIENT_PATH: missingClientPath
      }
    })

    expect(status.state).toBe('configuredNotVerified')
    expect(status.connected).toBe(false)
    expect(status.connectionProbe.status).toBe('unavailable')
    expect(status.connectionProbe.browserClientPath).toBe(missingClientPath)
    expect(status.connectionProbe.problem).toContain('does not exist')
  })

  it('projects Chrome extension-page action failures without shell errors', async () => {
    const errorSentinel = '/private/customer-pii-13900000042/open-external-error'
    electronMocks.openExternal.mockRejectedValueOnce(new Error(errorSentinel))

    await expect(openChromeBrowserUseExtensionPage('webstore')).resolves.toEqual({
      ok: false,
      message: 'Could not open the Chrome Browser Use extension page.'
    })
    expect(electronMocks.openExternal).toHaveBeenCalledOnce()
    expect(electronMocks.openExternal).toHaveBeenCalledWith(
      `https://chromewebstore.google.com/detail/codex/${EXTENSION_ID}`
    )
  })

  it('preserves the Chrome extension-page success result', async () => {
    electronMocks.openExternal.mockResolvedValueOnce(undefined)

    await expect(openChromeBrowserUseExtensionPage('webstore')).resolves.toEqual({ ok: true })
    expect(electronMocks.openExternal).toHaveBeenCalledOnce()
  })
})
