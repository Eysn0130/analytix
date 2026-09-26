import { chmodSync, cpSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, statSync, symlinkSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { createServer, type AddressInfo } from 'node:net'
import { homedir, tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { configureLogger } from './logger'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  getModelProviderPreset,
  modelProviderPresetProfile,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../shared/app-settings'
import { AnalytixConfigSchema } from '../../packages/runtime/src/config/analytix-config.js'

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getAppPath: () => '/tmp/analytix-test-app',
    getPath: () => '/tmp/analytix-test-user-data'
  }
}))

let tempRoot: string | null = null

function sealAuthorizedLocalRemountFixture(
  pluginDir: string,
  pluginName: string,
  version: string
): { sourcePath: string; markerPath: string } {
  if (!tempRoot) throw new Error('temp root not initialized')
  const packageSha256 = 'a'.repeat(64)
  const generation = `${version}-local-${packageSha256}`
  const sourcePath = join(
    tempRoot,
    '.cache',
    'analytix-hub-plugins',
    'marketplaces',
    'analytix-hub',
    'plugins',
    pluginName,
    generation
  )
  mkdirSync(sourcePath, { recursive: true })
  cpSync(pluginDir, sourcePath, { recursive: true })
  const markerPath = join(pluginDir, '.analytix-hub-installed-plugin.json')
  writeFileSync(markerPath, JSON.stringify({
    managedBy: 'analytix-hub',
    marketplaceName: 'analytix-hub',
    pluginName,
    version,
    packageSha256,
    sourcePath,
    source: { source: 'local', path: `./plugins/${pluginName}/${generation}` },
    transactionVersion: 'RuntimeCacheRemountTransactionV1',
    commitOrder: 'marketplace_pointer_last'
  }), 'utf8')
  chmodSync(markerPath, 0o600)
  return { sourcePath, markerPath }
}

function sealSelfAuthoredHubInstallFixture(
  pluginDir: string,
  pluginName: string,
  version: string
): void {
  if (!tempRoot) throw new Error('temp root not initialized')
  const sourcePath = join(
    tempRoot,
    '.cache',
    'analytix-hub-plugins',
    'marketplaces',
    'analytix-hub',
    'plugins',
    pluginName,
    version
  )
  mkdirSync(sourcePath, { recursive: true })
  cpSync(pluginDir, sourcePath, { recursive: true })
  const markerPath = join(pluginDir, '.analytix-hub-installed-plugin.json')
  writeFileSync(markerPath, JSON.stringify({
    managedBy: 'analytix-hub',
    marketplaceName: 'analytix-hub',
    pluginName,
    version,
    packageSha256: 'b'.repeat(64),
    sourcePath,
    sourceTreeSha256: 'c'.repeat(64),
    sourceTreeFileCount: 1,
    installType: 'user'
  }), 'utf8')
  chmodSync(markerPath, 0o600)
}

function writeBoundPluginMcpSidecar(dataDir: string, configPath: string, serverIds: string[]): void {
  const configBytes = readFileSync(configPath)
  const sidecarPath = join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json')
  mkdirSync(join(dataDir, '.cache'), { recursive: true })
  writeFileSync(sidecarPath, `${JSON.stringify({
    schemaVersion: 1,
    contract: 'analytix-plugin-managed-mcp-server-ids',
    configSha256: createHash('sha256').update(configBytes).digest('hex'),
    serverIds: [...serverIds].sort()
  }, null, 2)}\n`, 'utf8')
  chmodSync(sidecarPath, 0o600)
}

function createSettings(binaryPath: string): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: {
        ...defaultAnalytixRuntimeSettings(8899),
        binaryPath,
        autoStart: true
      },
    workspaceRoot: '/tmp/workspace',
    log: { enabled: false, retentionDays: 7 },
    notifications: { turnComplete: true },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    claw: defaultClawSettings(),
    schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: []
  }
}

describe('runtime stderr diagnostics', () => {
  it('exposes only byte count and digest, never stderr content', async () => {
    const module = await import('./analytix-process')
    const raw = '<think>PRIVATE_RUNTIME_REASONING</think>fatal'
    const diagnostic = module.analytixStderrDiagnostic(raw)
    expect(diagnostic.stderrBytes).toBe(Buffer.byteLength(raw, 'utf8'))
    expect(diagnostic.stderrSha256).toMatch(/^[a-f0-9]{64}$/)
    expect(JSON.stringify(diagnostic)).not.toMatch(/PRIVATE_RUNTIME_REASONING|<think>/)
  })
})

describe('runtime model provider snapshot', () => {
  it('clears ambient Provider credential authority for the child runtime', async () => {
    const source = readFileSync(new URL('./analytix-process.ts', import.meta.url), 'utf8')
    expect(source).toContain("'ANALYTIX_API_KEY'")
    expect(source).toContain("'ANALYTIX_MODEL_PROVIDERS'")
    expect(source).toContain("'ANALYTIX_HUB_TEST_GATEWAY_TOKEN'")
    expect(source).toContain("'ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN'")
    expect(source).toContain('delete childEnv[name]')
  })

  it('keeps the Go runtime boot configuration valid when provider credentials are absent', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')

    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    expect(env).toBeTruthy()
    const parsed = JSON.parse(env ?? '{}') as {
      defaultProviderId?: string
      providers?: Array<{ id: string; apiKey?: string; baseUrl: string }>
    }

    expect(parsed.defaultProviderId).toBe('deepseek')
    expect(parsed.providers?.length).toBeGreaterThan(0)
    expect(parsed.providers?.every((provider) => provider.apiKey === undefined)).toBe(true)
  })

  it('serializes key-free provider metadata for per-thread runtime routing', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')
    const preset = getModelProviderPreset('zai-coding-plan')
    expect(preset).not.toBeNull()
    const canary = ['closure', 'b', 'runtime-routing'].join('-')
    const provider = modelProviderPresetProfile(preset!)
    ;(provider as unknown as Record<string, unknown>).apiKey = canary
    settings.provider.providers = [
      ...defaultModelProviderSettings().providers,
      provider
    ]
    settings.runtime.providerId = provider.id

    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    expect(env).toBeTruthy()
    const parsed = JSON.parse(env ?? '{}') as {
      defaultProviderId?: string
      providers?: Array<{
        id: string
        apiKey?: string
        baseUrl: string
        endpointFormat?: string
        models?: string[]
        modelProfiles?: Record<string, { reasoning?: { requestProtocol?: string } }>
      }>
    }
    const routed = parsed.providers?.find((item) => item.id === 'zai-coding-plan')

    expect(parsed.defaultProviderId).toBe('zai-coding-plan')
    expect(routed).toMatchObject({
      id: 'zai-coding-plan',
      baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
      endpointFormat: 'custom_endpoint',
      models: expect.arrayContaining(['glm-5'])
    })
    expect(routed && 'apiKey' in routed).toBe(false)
    expect(JSON.stringify(parsed).includes(canary)).toBe(false)
    expect(routed?.modelProfiles?.['glm-5']?.reasoning?.requestProtocol)
      .toBe('glm-chat-completions')
  })

  it('uses provider.activeProviderId as the managed runtime default provider when runtime providerId is blank', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')
    const provider = {
      id: 'deepseek-live',
      name: 'DeepSeek Live',
      baseUrl: 'https://api.deepseek.com',
      endpointFormat: 'chat_completions' as const,
      models: ['deepseek-v4-pro'],
      modelProfiles: {}
    }
    settings.provider = {
      ...defaultModelProviderSettings(),
      activeProviderId: provider.id,
      providers: [
        ...defaultModelProviderSettings().providers,
        provider
      ]
    }
    settings.runtime.providerId = ''

    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    const parsed = JSON.parse(env ?? '{}') as {
      defaultProviderId?: string
      providers?: Array<{ id: string; apiKey?: string; baseUrl: string; endpointFormat?: string }>
    }
    const active = parsed.providers?.find((item) => item.id === provider.id)

    expect(parsed.defaultProviderId).toBe(provider.id)
    expect(active).toMatchObject({
      id: provider.id,
      baseUrl: 'https://api.deepseek.com',
      endpointFormat: 'chat_completions'
    })
  })

  it('does not inject the in-memory Hub gateway token into Provider metadata', async () => {
    const module = await import('./analytix-process')
    const { setHubGatewayRuntimeToken, clearHubGatewayRuntimeToken } = await import('./services/hub-gateway-runtime-secret')
    const settings = createSettings('')
    const provider: AppSettingsV1['provider']['providers'][number] = {
      id: 'analytix-hub',
      name: 'Analytix Hub',
      baseUrl: 'https://analytix.top/v1',
      endpointFormat: 'chat_completions',
      models: ['qwen3.6-plus'],
      modelProfiles: {}
    }
    settings.provider = {
      ...defaultModelProviderSettings(),
      activeProviderId: provider.id,
      providers: [
        ...defaultModelProviderSettings().providers,
        provider
      ]
    }
    settings.runtime.providerId = provider.id

    setHubGatewayRuntimeToken('gateway-test-token')
    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    clearHubGatewayRuntimeToken()
    const parsed = JSON.parse(env ?? '{}') as {
      providers?: Array<{ id: string; apiKey?: string; baseUrl: string }>
    }
    const active = parsed.providers?.find((item) => item.id === provider.id)

    expect(Object.hasOwn(settings.provider.providers.find((item) => item.id === provider.id) ?? {}, 'apiKey')).toBe(false)
    expect(active).toMatchObject({
      id: provider.id,
      baseUrl: 'https://analytix.top/v1'
    })
    expect(active && 'apiKey' in active).toBe(false)
  })

  it('keeps the Hub and official DeepSeek context windows provider-specific in the Go runtime snapshot', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')
    const hubProvider: AppSettingsV1['provider']['providers'][number] = {
      id: 'analytix-hub',
      name: 'Analytix Hub',
      baseUrl: 'https://analytix.top/v1',
      endpointFormat: 'chat_completions',
      models: ['deepseek-v4-flash'],
      modelProfiles: {
        'deepseek-v4-flash': {
          contextWindowTokens: 131_072,
          inputModalities: ['text'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text'],
          reasoning: {
            supportedEfforts: ['off', 'high', 'max'],
            defaultEffort: 'high',
            requestProtocol: 'deepseek-chat-completions'
          }
        }
      }
    }
    settings.provider = {
      ...defaultModelProviderSettings(),
      activeProviderId: hubProvider.id,
      providers: [
        ...defaultModelProviderSettings().providers,
        hubProvider
      ]
    }
    settings.runtime.providerId = hubProvider.id
    settings.runtime.model = 'deepseek-v4-flash'

    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    const parsed = JSON.parse(env ?? '{}') as {
      defaultProviderId?: string
      providers?: Array<{
        id: string
        modelProfiles?: Record<string, { contextWindowTokens?: number }>
      }>
    }
    const hub = parsed.providers?.find((item) => item.id === hubProvider.id)
    const official = parsed.providers?.find((item) => item.id === 'deepseek')

    expect(parsed.defaultProviderId).toBe(hubProvider.id)
    expect(hub?.modelProfiles?.['deepseek-v4-flash']?.contextWindowTokens).toBe(131_072)
    expect(official?.modelProfiles?.['deepseek-v4-flash']?.contextWindowTokens).toBe(1_000_000)
  })

  it('keeps preset providers in the Go runtime snapshot when their baseUrl is left blank', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')
    const defaults = defaultModelProviderSettings()
    settings.provider = {
      ...defaults,
      activeProviderId: 'deepseek',
      baseUrl: '',
      providers: defaults.providers.map((provider) =>
        provider.id === 'deepseek'
          ? { ...provider, baseUrl: '' }
          : provider
      )
    }
    settings.runtime.providerId = 'deepseek'

    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    const parsed = JSON.parse(env ?? '{}') as {
      defaultProviderId?: string
      providers?: Array<{ id: string; apiKey?: string; baseUrl: string }>
    }
    const active = parsed.providers?.find((item) => item.id === 'deepseek')

    expect(parsed.defaultProviderId).toBe('deepseek')
    expect(active).toMatchObject({
      id: 'deepseek',
      baseUrl: 'https://api.deepseek.com'
    })
  })

  it('refreshes provider endpoint and model-profile currentness in the managed runtime env snapshot', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')
    const provider: AppSettingsV1['provider']['providers'][number] = {
      id: 'current-provider',
      name: 'Current Provider',
      baseUrl: 'https://current.example/v1',
      endpointFormat: 'chat_completions' as const,
      models: ['current-model'],
      modelProfiles: {
        'current-model': {
          contextWindowTokens: 128_000,
          inputModalities: ['text'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text']
        }
      }
    }
    settings.provider.providers = [
      ...defaultModelProviderSettings().providers,
      provider
    ]
    settings.runtime.providerId = provider.id

    const first = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    settings.provider.providers = settings.provider.providers.map((item) =>
      item.id === provider.id
        ? {
            ...item,
            baseUrl: 'https://current.example/responses',
            endpointFormat: 'responses' as const,
            modelProfiles: {
              'current-model': {
                ...item.modelProfiles['current-model'],
                contextWindowTokens: 256_000,
                reasoning: {
                  supportedEfforts: ['off', 'high'],
                  defaultEffort: 'high',
                  requestProtocol: 'openai-responses'
                }
              }
            }
          }
        : item
    )
    const next = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)

    expect(next).not.toBe(first)
    const parsed = JSON.parse(next ?? '{}') as {
      providers?: Array<{
        id: string
        baseUrl: string
        endpointFormat?: string
        modelProfiles?: Record<string, {
          contextWindowTokens?: number
          reasoning?: { requestProtocol?: string }
        }>
      }>
    }
    const routed = parsed.providers?.find((item) => item.id === provider.id)
    expect(routed).toMatchObject({
      baseUrl: 'https://current.example/responses',
      endpointFormat: 'responses'
    })
    expect(routed?.modelProfiles?.['current-model']).toMatchObject({
      contextWindowTokens: 256_000,
      reasoning: { requestProtocol: 'openai-responses' }
    })
  })

  it('serializes provider pricing for Go runtime usage cost', async () => {
    const module = await import('./analytix-process')
    const settings = createSettings('')
    const provider: AppSettingsV1['provider']['providers'][number] = {
      id: 'priced-provider',
      name: 'Priced Provider',
      baseUrl: 'https://priced.example/v1',
      endpointFormat: 'chat_completions' as const,
      models: ['priced-default', 'priced-special'],
      price: { input: 3, output: 9, currency: 'USD' },
      prices: {
        'priced-special': { cacheHit: 1, input: 4, output: 12, currency: 'USD' }
      },
      modelProfiles: {
        'priced-special': {
          inputModalities: ['text'],
          outputModalities: ['text'],
          supportsToolCalling: true,
          messageParts: ['text'],
          price: { cacheHit: 0.5, input: 2, output: 8, currency: 'CNY' }
        }
      }
    }
    settings.provider.providers = [
      ...defaultModelProviderSettings().providers,
      provider
    ]
    settings.runtime.providerId = provider.id

    const env = module.buildRuntimeModelProvidersEnv(settings, settings.runtime)
    const parsed = JSON.parse(env ?? '{}') as {
      providers?: Array<{
        id: string
        price?: unknown
        prices?: Record<string, unknown>
        modelProfiles?: Record<string, { price?: unknown }>
      }>
    }
    const routed = parsed.providers?.find((item) => item.id === provider.id)

    expect(routed?.price).toEqual({ input: 3, output: 9, currency: 'USD' })
    expect(routed?.prices?.['priced-special']).toEqual({ cacheHit: 1, input: 4, output: 12, currency: 'USD' })
    expect(routed?.modelProfiles?.['priced-special']?.price).toEqual({ cacheHit: 0.5, input: 2, output: 8, currency: 'CNY' })
  })
})

function canBindTestPort(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const server = createServer()
    let settled = false
    const settle = (available: boolean): void => {
      if (settled) return
      settled = true
      server.removeAllListeners('error')
      resolve(available)
    }
    server.unref()
    server.once('error', () => settle(false))
    server.listen(port, '127.0.0.1', () => {
      server.close(() => settle(true))
    })
  })
}

beforeEach(() => {
  tempRoot = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-process-')))
  configureLogger({ dir: tempRoot, enabled: true, retentionDays: 7 })
})

afterEach(async () => {
  const module = await import('./analytix-process')
  await module.stopAnalytixChildAndWait()
  module.configureAnalytixProcessMainPrivatePaths(null)
  configureLogger({ dir: '', enabled: true, retentionDays: 2 })
  if (tempRoot) {
    rmSync(tempRoot, { recursive: true, force: true })
    tempRoot = null
  }
})

describe('startAnalytixChild', () => {
  it('resolves the standalone development codec with the existing macOS Helper', async () => {
    const { resolveDocumentCodecLaunch } = await import('./analytix-process')
    expect(resolveDocumentCodecLaunch({ appPath: '/synthetic/source', execPath: '/synthetic/Electron.app/Contents/MacOS/Electron', isPackaged: false }, 'darwin')).toEqual({
      executable: '/synthetic/Electron.app/Contents/Frameworks/Electron Helper.app/Contents/MacOS/Electron Helper',
      entry: '/synthetic/source/out/office-codec/office-generation-codec-entry.js'
    })
  })

  it('resolves the packaged codec to real unpacked files without using ASAR paths for Go', async () => {
    const { resolveDocumentCodecLaunch } = await import('./analytix-process')
    expect(resolveDocumentCodecLaunch({ appPath: '/synthetic/Analytix.app/Contents/Resources/app.asar', execPath: '/synthetic/Analytix.app/Contents/MacOS/Analytix', isPackaged: true }, 'darwin')).toEqual({
      executable: '/synthetic/Analytix.app/Contents/Frameworks/Analytix Helper.app/Contents/MacOS/Analytix Helper',
      entry: '/synthetic/Analytix.app/Contents/Resources/app.asar.unpacked/out/office-codec/office-generation-codec-entry.js'
    })
    expect(resolveDocumentCodecLaunch({ appPath: '/synthetic/resources/app.asar', execPath: '/synthetic/analytix', isPackaged: true }, 'linux')).toEqual({
      executable: '/synthetic/analytix', entry: '/synthetic/resources/app.asar.unpacked/out/office-codec/office-generation-codec-entry.js'
    })
  })

  it('rejects the retired TypeScript child runtime path without spawning a process', async () => {
    const module = await import('./analytix-process')
    await expect(module.startAnalytixChild(createSettings('/tmp/retired-runtime.js'))).rejects.toThrow(
      /TypeScript Analytix child runtime is retired/
    )
    expect(module.isAnalytixChildRunning()).toBe(false)
  })
})

describe('reclaimAnalytixPort', () => {
  it('recognizes stale Go runtime-server commands without matching generic servers', async () => {
    const module = await import('./analytix-process')

    expect(module.commandLooksLikeStaleAnalytixRuntime(
      '/var/folders/go-build/exe/runtime-server --addr 127.0.0.1:8901 --fixtures-dir /Users/sun/Projects/analytix/packages/runtime/src/conformance/fixtures --runtime-token tok-1 --durable-root /Users/sun/.analytix/data/runtime-go'
    )).toBe(true)
    expect(module.commandLooksLikeStaleAnalytixRuntime(
      '/Users/sun/Projects/analytix/packages/runtime-go/bin/runtime-server --host 127.0.0.1 --port 8901 --data-dir /Users/sun/.analytix/data --model deepseek-chat --approval-policy on-request'
    )).toBe(true)
    expect(module.commandLooksLikeStaleAnalytixRuntime(
      '/Users/sun/Projects/analytix/packages/runtime-go/bin/runtime-server --addr 127.0.0.1:8901 --data-dir /Users/sun/.analytix/data --durable-root /Users/sun/.analytix/data --model deepseek-chat'
    )).toBe(true)
    expect(module.commandLooksLikeStaleAnalytixRuntime(
      '/usr/local/bin/runtime-server --addr 127.0.0.1:8901'
    )).toBe(false)
  })

  it('reports a port as unavailable when another listener owns it', async () => {
    const server = createServer()
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject)
      server.listen(0, '127.0.0.1', () => resolve())
    })
    try {
      const address = server.address() as AddressInfo
      const module = await import('./analytix-process')

      await expect(module.reclaimAnalytixPort(address.port)).resolves.toEqual({
        ok: false,
        message: `port ${address.port} is in use`
      })
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()))
    }
  })

  it('allows non-positive ports so Analytix can request an ephemeral port', async () => {
    const module = await import('./analytix-process')

    await expect(module.reclaimAnalytixPort(0)).resolves.toEqual({ ok: true })
  })

  it('resolves the next available fallback port when the preferred port is unavailable', async () => {
    let server: ReturnType<typeof createServer> | null = null
    let preferredPort = 0
    for (let attempt = 0; attempt < 20; attempt += 1) {
      const candidate = createServer()
      await new Promise<void>((resolve, reject) => {
        candidate.once('error', reject)
        candidate.listen(0, '127.0.0.1', () => resolve())
      })
      const address = candidate.address() as AddressInfo
      if (address.port < 65_535 && await canBindTestPort(address.port + 1)) {
        server = candidate
        preferredPort = address.port
        break
      }
      await new Promise<void>((resolve) => candidate.close(() => resolve()))
    }
    if (!server || preferredPort <= 0) {
      throw new Error('Could not find consecutive test ports')
    }
    try {
      const module = await import('./analytix-process')

      const resolved = await module.resolveAvailableAnalytixPort(preferredPort)

      expect(resolved).toMatchObject({
        changed: true,
        message: `port ${preferredPort} is in use`
      })
      expect(resolved.port).toBeGreaterThan(preferredPort)
      await expect(module.reclaimAnalytixPort(resolved.port)).resolves.toEqual({ ok: true })
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()))
    }
  })
})

describe('resolveAnalytixDataDir', () => {
  it('expands Windows-style home-relative data directories', async () => {
    const module = await import('./analytix-process')

    expect(module.resolveAnalytixDataDir({ dataDir: '~\\deepseek\\analytix' })).toBe(join(homedir(), 'deepseek', 'analytix'))
  })

  it('does not expand non-home tilde prefixes', async () => {
    const module = await import('./analytix-process')

    expect(module.resolveAnalytixDataDir({ dataDir: '~other\\analytix' })).toBe('~other\\analytix')
  })
})

describe('parseListeningPidsFromNetstat', () => {
  it('extracts the listening TCP PIDs for the port across IPv4/IPv6, ignoring everything else', async () => {
    const { parseListeningPidsFromNetstat } = await import('./analytix-process')
    const output = [
      '',
      'Active Connections',
      '',
      '  Proto  Local Address          Foreign Address        State           PID',
      '  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1010',
      '  TCP    127.0.0.1:8899         0.0.0.0:0              LISTENING       6789',
      '  TCP    [::1]:8899             [::]:0                 LISTENING       6789',
      '  TCP    127.0.0.1:8899         127.0.0.1:51000        ESTABLISHED     7000',
      '  TCP    127.0.0.1:18899        0.0.0.0:0              LISTENING       8000',
      '  UDP    0.0.0.0:8899           *:*                                    9000',
      `  TCP    127.0.0.1:8899         0.0.0.0:0              LISTENING       ${process.pid}`,
      ''
    ].join('\r\n')

    // Dedups IPv4+IPv6 rows for the same PID; excludes the :135 listener, the
    // ESTABLISHED row, the :18899 suffix collision, the UDP row, and our own PID.
    expect(parseListeningPidsFromNetstat(output, 8899)).toEqual([6789])
  })

  it('returns no PIDs when nothing listens on the port', async () => {
    const { parseListeningPidsFromNetstat } = await import('./analytix-process')
    const output = '  TCP    127.0.0.1:8899         0.0.0.0:0              LISTENING       6789'

    expect(parseListeningPidsFromNetstat(output, 9999)).toEqual([])
  })
})

describe('syncGuiManagedAnalytixConfig', () => {
  it('uses the configured main-private MCP path when the caller does not pass one', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const dataDir = join(tempRoot, 'runtime-data')
    const isolatedMcpConfigPath = join(tempRoot, 'isolated-home', '.analytix', 'mcp.json')
    mkdirSync(dirname(isolatedMcpConfigPath), { recursive: true })
    writeFileSync(isolatedMcpConfigPath, JSON.stringify({
      servers: {
        isolated_manual: {
          command: 'isolated-manual-mcp',
          args: [],
          env: {},
          url: null,
          enabled: true,
          required: false
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')
    module.configureAnalytixProcessMainPrivatePaths({
      mcpConfigPath: isolatedMcpConfigPath
    })

    await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings())

    const parsed = JSON.parse(readFileSync(join(dataDir, 'config.json'), 'utf8')) as any
    expect(parsed.capabilities.mcp.servers.isolated_manual).toMatchObject({
      command: 'isolated-manual-mcp',
      trustScope: 'user'
    })
  })

  it('creates a production runtime config containing only active settings', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings())

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.serve).toBeUndefined()
    expect(parsed.contextCompaction).toBeUndefined()
    expect(parsed.models).toBeUndefined()
    expect(parsed.runtime.streamIdleTimeoutMs).toBe(45000)
    expect(parsed.runtime.toolStorm).toBeUndefined()
    expect(parsed.runtime.toolArgumentRepair).toBeUndefined()
    expect(parsed.runtime.stepLimits).toMatchObject({
      defaultMaxModelSteps: 64,
      userGlobalMaxModelSteps: 0,
      plannerMaxModelSteps: 0,
      headlessMaxModelSteps: 0
    })
    expect(parsed.quality).toBeUndefined()
    expect(parsed.capabilities.attachments).toBeUndefined()
    expect(parsed.capabilities.web).toMatchObject({ enabled: true, fetchEnabled: true })
    expect(parsed.capabilities.mcp.search).toMatchObject({ enabled: false, mode: 'auto' })
    expect(parsed.capabilities.subagents).toMatchObject({
      enabled: true,
      maxParallel: 8,
      maxChildRuns: 64,
      defaultToolPolicy: 'readOnly',
      profiles: {}
    })
    expect(parsed.capabilities.imageGen).toBeUndefined()
    expect(parsed.capabilities.speechGen).toBeUndefined()
    expect(parsed.capabilities.musicGen).toBeUndefined()
    expect(parsed.capabilities.videoGen).toBeUndefined()
  })

  it('writes GUI-managed subagent profiles into runtime capabilities config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, {
      ...defaultAnalytixRuntimeSettings(),
      subagents: {
        enabled: true,
        maxParallel: 4,
        maxChildRuns: 24,
        defaultToolPolicy: 'inherit',
        defaultProfile: 'reviewer',
        profiles: {
          reviewer: {
            prompt: 'Review for correctness only.',
            model: 'deepseek-v4-pro',
            effort: 'high',
            toolPolicy: 'readOnly',
            tools: ['grep', 'read']
          }
        }
      }
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)
    expect(parsed.capabilities.subagents).toMatchObject({
      enabled: true,
      maxParallel: 4,
      maxChildRuns: 24,
      defaultToolPolicy: 'inherit',
      defaultProfile: 'reviewer',
      profiles: {
        reviewer: {
          promptPreamble: 'Review for correctness only.',
          model: 'deepseek-v4-pro',
          effort: 'high',
          toolPolicy: 'readOnly',
          tools: ['grep', 'read']
        }
      }
    })
  })

  it('does not copy Electron-only image generation settings or secrets into runtime config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const runtime = {
      ...defaultAnalytixRuntimeSettings(),
      imageGeneration: {
        enabled: true,
        providerId: '',
        protocol: 'openai-images' as const,
        baseUrl: 'https://api.siliconflow.cn/v1',
        model: 'Kwai-Kolors/Kolors',
        defaultSize: '',
        timeoutMs: 240000
      }
    }

    await module.syncGuiManagedAnalytixConfig(tempRoot, runtime)

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.imageGen).toBeUndefined()
    expect(readFileSync(configPath, 'utf8')).not.toContain('sk-image-test')
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)

    await module.syncGuiManagedAnalytixConfig(tempRoot, runtime)
    const cleared = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(cleared.capabilities.imageGen).toBeUndefined()
  })

  it('keeps the config stable across repeated syncs with imageGen configured', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const runtime = {
      ...defaultAnalytixRuntimeSettings(),
      imageGeneration: {
        enabled: true,
        providerId: '',
        protocol: 'openai-images' as const,
        baseUrl: 'https://api.siliconflow.cn/v1',
        model: 'Kwai-Kolors/Kolors',
        defaultSize: '1024x1024',
        timeoutMs: 180000
      }
    }

    await module.syncGuiManagedAnalytixConfig(tempRoot, runtime)
    const firstText = readFileSync(configPath, 'utf8')
    const firstMtime = statSync(configPath).mtimeMs
    await new Promise((resolve) => setTimeout(resolve, 25))

    // If the capability sanitizer strips imageGen from the existing config,
    // every sync rewrites the file and restarts Analytix in a loop.
    await module.syncGuiManagedAnalytixConfig(tempRoot, runtime)
    expect(readFileSync(configPath, 'utf8')).toBe(firstText)
    expect(statSync(configPath).mtimeMs).toBe(firstMtime)
  })

  it('does not copy unsupported media generation settings or secrets into runtime config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const runtime = {
      ...defaultAnalytixRuntimeSettings(),
      textToSpeech: {
        enabled: true,
        providerId: '',
        protocol: 'minimax-t2a' as const,
        baseUrl: 'https://api.minimax.io',
        model: 'speech-2.8-hd',
        voice: 'male-qn-qingse',
        format: 'mp3',
        timeoutMs: 120000
      },
      musicGeneration: {
        enabled: true,
        providerId: '',
        protocol: 'minimax-music' as const,
        baseUrl: 'https://api.minimax.io',
        model: 'music-2.6',
        format: 'mp3',
        timeoutMs: 300000
      },
      videoGeneration: {
        enabled: true,
        providerId: '',
        protocol: 'minimax-video' as const,
        baseUrl: 'https://api.minimax.io',
        model: 'MiniMax-Hailuo-2.3',
        defaultDuration: 6,
        defaultResolution: '1080P',
        timeoutMs: 900000,
        pollIntervalMs: 10000
      }
    }

    await module.syncGuiManagedAnalytixConfig(tempRoot, runtime)

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.speechGen).toBeUndefined()
    expect(parsed.capabilities.musicGen).toBeUndefined()
    expect(parsed.capabilities.videoGen).toBeUndefined()
    expect(readFileSync(configPath, 'utf8')).not.toMatch(/sk-(tts|music|video)-test/)
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)

    await module.syncGuiManagedAnalytixConfig(tempRoot, {
      ...runtime,
      textToSpeech: { ...runtime.textToSpeech, voice: '' }
    })
    const cleared = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(cleared.capabilities.speechGen).toBeUndefined()
  })

  it('adds the built-in schedule MCP server to Analytix runtime capabilities', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const settings = createSettings('/tmp/fake-analytix-child.js')
    settings.schedule.internal.port = 9788
    settings.schedule.internal.secret = 'top-secret'

    const syncResult = await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      scheduleMcp: {
        settings,
        launch: {
          appPath: '/tmp/analytix-test-app',
          execPath: '/tmp/electron',
          isPackaged: false
        }
      }
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.enabled).toBe(true)
    expect(syncResult.hostScheduleMcpBindingV1).toEqual({
      schemaVersion: 1,
      purpose: 'analytix.runtime-host-schedule-mcp-binding/v1',
      serverId: 'gui_schedule',
      command: '/tmp/electron',
      args: [
        '/tmp/analytix-test-app/out/main/claw-schedule-mcp-node-entry.js',
        '--gui-schedule-mcp-server',
        '--base-url',
        'http://127.0.0.1:9788',
        '--secret',
        'top-secret'
      ],
      env: { ELECTRON_RUN_AS_NODE: '1' },
      trustScope: 'user',
      timeoutMs: 5_000
    })
    expect(parsed.capabilities.mcp.servers.gui_schedule).toMatchObject({
      enabled: true,
      transport: 'stdio',
      command: '/tmp/electron',
      args: [
        '/tmp/analytix-test-app/out/main/claw-schedule-mcp-node-entry.js',
        '--gui-schedule-mcp-server',
        '--base-url',
        'http://127.0.0.1:9788',
        '--secret',
        'top-secret'
      ],
      env: {
        ELECTRON_RUN_AS_NODE: '1'
      },
      trustScope: 'user'
    })
  })

  it('keeps the private schedule binding on the host candidate when user config collides', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          servers: {
            gui_schedule: {
              enabled: true,
              transport: 'stdio',
              command: '/tmp/untrusted-schedule',
              args: [],
              env: {},
              trustScope: 'user',
              timeoutMs: 5_000
            }
          }
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')
    const settings = createSettings('/tmp/fake-analytix-child.js')
    settings.schedule.internal.port = 9788

    const syncResult = await module.syncGuiManagedAnalytixConfig(
      tempRoot,
      defaultAnalytixRuntimeSettings(),
      {
        scheduleMcp: {
          settings,
          launch: {
            appPath: '/tmp/analytix-test-app',
            execPath: '/tmp/electron',
            isPackaged: false
          }
        }
      }
    )

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.servers.gui_schedule.command).toBe('/tmp/electron')
    expect(syncResult.hostScheduleMcpBindingV1?.command).toBe('/tmp/electron')
    expect(syncResult.hostScheduleMcpBindingV1?.args[3]).toBe('http://127.0.0.1:9788')
  })

  it('snapshots the persisted and private schedule projection before asynchronous config work', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const module = await import('./analytix-process')
    const settings = createSettings('/tmp/fake-analytix-child.js')
    settings.schedule.internal.port = 9788
    settings.schedule.internal.secret = 'initial-secret'

    const syncPromise = module.syncGuiManagedAnalytixConfig(
      tempRoot,
      defaultAnalytixRuntimeSettings(),
      {
        scheduleMcp: {
          settings,
          launch: {
            appPath: '/tmp/analytix-test-app',
            execPath: '/tmp/electron',
            isPackaged: false
          }
        }
      }
    )
    settings.schedule.internal.port = 9789
    settings.schedule.internal.secret = 'mutated-secret'

    const syncResult = await syncPromise
    const parsed = JSON.parse(
      readFileSync(join(tempRoot, 'config.json'), 'utf8')
    ) as any
    expect(syncResult.hostScheduleMcpBindingV1?.args.slice(3)).toEqual([
      'http://127.0.0.1:9788',
      '--secret',
      'initial-secret'
    ])
    expect(parsed.capabilities.mcp.servers.gui_schedule.args.slice(3)).toEqual(
      syncResult.hostScheduleMcpBindingV1?.args.slice(3)
    )
  })

  it('closes an invalid schedule projection without blocking ordinary runtime config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')

    const syncResult = await module.syncGuiManagedAnalytixConfig(
      tempRoot,
      defaultAnalytixRuntimeSettings(),
      {
        scheduleMcp: {
          settings: createSettings('/tmp/fake-analytix-child.js'),
          launch: {
            appPath: '/tmp/invalid-\ud800',
            execPath: '/tmp/electron',
            isPackaged: false
          }
        }
      }
    )

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(syncResult.hostScheduleMcpBindingV1).toBeNull()
    expect(parsed.capabilities.mcp.servers.gui_schedule).toBeUndefined()
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)
  })

  it('closes an escape-expanded oversized schedule frame without blocking ordinary runtime config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const settings = createSettings('/tmp/fake-analytix-child.js')
    settings.schedule.internal.port = 9788
    settings.schedule.internal.secret = '&'.repeat(90_000)

    const syncResult = await module.syncGuiManagedAnalytixConfig(
      tempRoot,
      defaultAnalytixRuntimeSettings(),
      {
        scheduleMcp: {
          settings,
          launch: {
            appPath: '/tmp/analytix-test-app',
            execPath: '/tmp/electron',
            isPackaged: false
          }
        }
      }
    )

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(syncResult.hostScheduleMcpBindingV1).toBeNull()
    expect(parsed.capabilities.mcp.servers.gui_schedule).toBeUndefined()
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)
  })

  it('auto-registers the bundled Analytix Computer Use MCP server when enabled', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const command = join(tempRoot, 'analytix-computer-use')
    const previousCommand = process.env.ANALYTIX_COMPUTER_USE_MCP_COMMAND
    writeFileSync(command, '', 'utf8')
    process.env.ANALYTIX_COMPUTER_USE_MCP_COMMAND = command

    try {
      await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings())
    } finally {
      if (previousCommand === undefined) delete process.env.ANALYTIX_COMPUTER_USE_MCP_COMMAND
      else process.env.ANALYTIX_COMPUTER_USE_MCP_COMMAND = previousCommand
    }

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.enabled).toBe(true)
    expect(parsed.capabilities.mcp.servers['analytix-computer-use']).toMatchObject({
      enabled: true,
      transport: 'stdio',
      command,
      args: ['mcp'],
      env: {
        ANALYTIX_COMPUTER_USE_MANAGED_BY: 'analytix'
      },
      trustScope: 'user',
      backgroundStart: false,
      timeoutMs: 30000
    })
  })

  it('resolves the packaged Windows Analytix Computer Use native executable before command shims', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const module = await import('./analytix-process')
    const root = join(tempRoot, 'app.asar.unpacked')
    const native = join(
      root,
      'packages',
      'runtime',
      'node_modules',
      'analytix-computer-use',
      'dist',
      'windows',
      'amd64',
      'analytix-computer-use.exe'
    )
    const shim = join(root, 'packages', 'runtime', 'node_modules', '.bin', 'analytix-computer-use.cmd')
    mkdirSync(join(native, '..'), { recursive: true })
    mkdirSync(join(shim, '..'), { recursive: true })
    writeFileSync(native, '', 'utf8')
    writeFileSync(shim, '', 'utf8')

    expect(module.resolveBuiltInComputerUseMcpCommand({
      root,
      platform: 'win32',
      arch: 'x64',
      env: {}
    })).toBe(native)
  })

  it('adds GUI project and configured global skill roots to Analytix runtime capabilities', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const module = await import('./analytix-process')
    const settings = createSettings('/tmp/fake-analytix-child.js')
    const workspaceRoot = join(tempRoot, 'workspace')
    const extraRoot = join(tempRoot, 'extra-skills')
    settings.workspaceRoot = workspaceRoot
    settings.claw.skills.extraDirs = [extraRoot]
    mkdirSync(join(workspaceRoot, '.codex', 'skills'), { recursive: true })

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      scheduleMcp: {
        settings,
        launch: {
          appPath: '/tmp/analytix-test-app',
          execPath: '/tmp/electron',
          isPackaged: false
        }
      }
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.skills.enabled).toBe(true)
    expect(parsed.capabilities.skills.legacySkillMd).toBe(true)
    expect(parsed.capabilities.skills.roots).toEqual(expect.arrayContaining([
      join(workspaceRoot, '.codex', 'skills'),
      extraRoot
    ]))
  })

  it('re-enables skills when roots are discovered despite a persisted enabled:false', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    // Simulate a config whose skills capability was persisted with the schema
    // default enabled:false (there is no user-facing disable toggle).
    writeFileSync(configPath, JSON.stringify({
      capabilities: { skills: { enabled: false, roots: [], legacySkillMd: true } }
    }), 'utf8')
    const module = await import('./analytix-process')
    const settings = createSettings('/tmp/fake-analytix-child.js')
    const workspaceRoot = join(tempRoot, 'workspace')
    settings.workspaceRoot = workspaceRoot
    mkdirSync(join(workspaceRoot, '.codex', 'skills'), { recursive: true })

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      scheduleMcp: {
        settings,
        launch: { appPath: '/tmp/analytix-test-app', execPath: '/tmp/electron', isPackaged: false }
      }
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.skills.enabled).toBe(true)
    expect(parsed.capabilities.skills.roots).toEqual(expect.arrayContaining([
      join(workspaceRoot, '.codex', 'skills')
    ]))
  })

  it('scrubs compatibility-only config while preserving active MCP settings and servers', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    writeFileSync(configPath, JSON.stringify({
      legacyTopLevelFlag: true,
      contextCompaction: {
        modelProfiles: {
          'custom-model': {
            contextWindowTokens: 128000
          }
        }
      },
      models: {
        profiles: {
          'user-model': {
            contextWindowTokens: 96000,
            contextCompaction: {
              softThreshold: 86000
            }
          },
          'deepseek-v4-pro': {
            contextCompaction: {
              softThreshold: 970000
            }
          }
        }
      },
      runtime: {
        customRuntimeFlag: true,
        toolStorm: {
          customStormFlag: 'keep'
        }
      },
      serve: {
        legacyServeFlag: true,
        tokenEconomy: {
          customTokenEconomyFlag: 'keep',
          historyHygiene: {
            customHistoryFlag: true
          }
        }
      },
      capabilities: {
        mcp: {
          enabled: true,
          servers: {
            'custom-github': {
              transport: 'stdio',
              command: 'github-mcp',
              trustScope: 'user'
            }
          }
        },
        web: {
          enabled: true,
          fetchEnabled: true
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, {
      ...defaultAnalytixRuntimeSettings(),
      storage: {
        backend: 'hybrid',
        sqlitePath: '/tmp/analytix-index.sqlite3'
      },
      contextCompaction: {
        defaultSoftThreshold: 32000,
        defaultHardThreshold: 64000,
        summaryMode: 'model',
        summaryTimeoutMs: 30000,
        summaryMaxTokens: 1600,
        summaryInputMaxBytes: 131072
      },
      runtimeTuning: {
        streamIdleTimeoutMs: 120000,
        stepLimits: {
          defaultMaxModelSteps: 96,
          userGlobalMaxModelSteps: 48,
          plannerMaxModelSteps: 12,
          headlessMaxModelSteps: 24
        },
        toolStorm: {
          enabled: false,
          windowSize: 12,
          threshold: 4
        },
        toolArgumentRepair: {
          maxStringBytes: 262144
        }
      },
      mcpSearch: {
        enabled: true,
        mode: 'search',
        autoThresholdToolCount: 12,
        topKDefault: 4,
        topKMax: 9,
        minScore: 0.2
      },
      tokenEconomy: {
        enabled: true,
        compressToolDescriptions: false,
        compressToolResults: true,
        conciseResponses: false,
        historyHygiene: {
          maxToolResultLines: 100,
          maxToolResultBytes: 16384,
          maxToolResultTokens: 4000,
          maxToolArgumentStringBytes: 4096,
          maxToolArgumentStringTokens: 1000,
          maxArrayItems: 40,
          maxCumulativeToolResultTokens: 0,
          keepRecentToolResults: 0
        }
      }
    }, {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)
    expect(parsed.legacyTopLevelFlag).toBeUndefined()
    expect(parsed.serve).toBeUndefined()
    expect(parsed.contextCompaction).toBeUndefined()
    expect(parsed.models).toBeUndefined()
    expect(parsed.runtime.toolStorm).toBeUndefined()
    expect(parsed.runtime.customRuntimeFlag).toBeUndefined()
    expect(parsed.runtime.toolArgumentRepair).toBeUndefined()
    expect(parsed.runtime.stepLimits).toMatchObject({
      defaultMaxModelSteps: 96,
      userGlobalMaxModelSteps: 48,
      plannerMaxModelSteps: 12,
      headlessMaxModelSteps: 24
    })
    expect(parsed.runtime.streamIdleTimeoutMs).toBe(120000)
    expect(parsed.capabilities.attachments).toBeUndefined()
    expect(parsed.capabilities.mcp.servers['custom-github'].command).toBe('github-mcp')
    expect(parsed.capabilities.web.fetchEnabled).toBe(true)
    expect(parsed.capabilities.mcp.search).toMatchObject({
      enabled: true,
      mode: 'search',
      autoThresholdToolCount: 12,
      topKDefault: 4,
      topKMax: 9,
      minScore: 0.2
    })
  })

  it('imports GUI-managed MCP servers into runtime capabilities', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const mcpConfigPath = join(tempRoot, 'mcp.json')
    writeFileSync(mcpConfigPath, JSON.stringify({
      servers: {
        'stata-mcp': {
          command: 'uvx',
          args: ['stata-mcp'],
          cwd: '/tmp/stata-workspace',
          env: {
            STATA_CLI: 'D:\\stata\\StataMP-64.exe'
          },
          lowPriority: true,
          backgroundStart: true,
          enabled: true,
          disabled: false
        },
        'docs-mcp': {
          url: 'https://mcp.example.test/mcp',
          headers: {
            Authorization: 'Bearer docs-token'
          }
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.enabled).toBe(true)
    expect(parsed.capabilities.mcp.servers['stata-mcp']).toMatchObject({
      enabled: true,
      transport: 'stdio',
      command: 'uvx',
      args: ['stata-mcp'],
      cwd: '/tmp/stata-workspace',
      env: {
        STATA_CLI: 'D:\\stata\\StataMP-64.exe'
      },
      lowPriority: true,
      backgroundStart: true,
      trustScope: 'user'
    })
    expect(parsed.capabilities.mcp.servers['docs-mcp']).toMatchObject({
      enabled: true,
      transport: 'streamable-http',
      url: 'https://mcp.example.test/mcp',
      headers: {
        Authorization: 'Bearer docs-token'
      },
      trustScope: 'user'
    })
  })

  it('uses Hub-managed standard MCP plugins instead of same-named local MCP config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const mcpConfigPath = join(tempRoot, 'mcp.json')
    writeFileSync(mcpConfigPath, JSON.stringify({
      servers: {
        memory: {
          command: 'legacy-memory-mcp',
          args: ['--stdio']
        },
        'docs-mcp': {
          command: 'docs-mcp',
          args: ['--stdio']
        }
      }
    }), 'utf8')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'memory', '2026.1.26')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'memory',
      version: '2026.1.26',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        memory: {
          command: 'npx',
          args: ['-y', '@modelcontextprotocol/server-memory@2026.1.26']
        }
      }
    }), 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'memory', '2026.1.26')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.servers.memory).toMatchObject({
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-memory@2026.1.26']
    })
    expect(parsed.capabilities.mcp.servers['docs-mcp']).toMatchObject({
      command: 'docs-mcp',
      args: ['--stdio']
    })
  })

  it('imports authorized local-remount MCP servers into runtime capabilities', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const previousDemoToken = process.env.DEMO_PLUGIN_TOKEN
    process.env.DEMO_PLUGIN_TOKEN = 'secret-token'
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'demo-plugin', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'demo-plugin',
      version: '1.0.0',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        'demo-plugin': {
          command: 'node',
          args: ['./mcp/server.cjs', '--stdio'],
          cwd: '.'
        },
        'demo-bin': {
          command: './bin/server',
          args: ['--stdio'],
          cwd: '.',
          lowPriority: true,
          backgroundStart: true,
          env_vars: ['DEMO_PLUGIN_TOKEN', 'MISSING_DEMO_PLUGIN_TOKEN'],
          env: {
            DEMO_STATIC_FLAG: '1'
          }
        }
      }
    }), 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'demo-plugin', '1.0.0')
    mkdirSync(join(dataDir, '.cache'), { recursive: true })
    writeFileSync(
      join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json'),
      JSON.stringify(['demo-bin', 'demo-plugin']),
      'utf8'
    )
    const module = await import('./analytix-process')
    let firstConfigText = ''
    let firstSidecarText = ''

    try {
      await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
        mcpConfigPath
      })
      firstConfigText = readFileSync(configPath, 'utf8')
      firstSidecarText = readFileSync(join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json'), 'utf8')
      await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
        mcpConfigPath
      })
    } finally {
      if (previousDemoToken === undefined) {
        delete process.env.DEMO_PLUGIN_TOKEN
      } else {
        process.env.DEMO_PLUGIN_TOKEN = previousDemoToken
      }
    }

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(readFileSync(configPath, 'utf8')).toBe(firstConfigText)
    expect(readFileSync(join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json'), 'utf8')).toBe(firstSidecarText)
    expect(JSON.parse(firstSidecarText)).toMatchObject({
      schemaVersion: 1,
      contract: 'analytix-plugin-managed-mcp-server-ids',
      configSha256: createHash('sha256').update(firstConfigText, 'utf8').digest('hex'),
      serverIds: ['demo-bin', 'demo-plugin']
    })
    expect(parsed.capabilities.mcp.enabled).toBe(true)
    expect(parsed.capabilities.mcp.servers['demo-plugin']).toMatchObject({
      enabled: true,
      transport: 'stdio',
      command: 'node',
      args: [join(pluginDir, 'mcp', 'server.cjs'), '--stdio'],
      cwd: pluginDir,
      trustScope: 'user'
    })
    expect(parsed.capabilities.mcp.servers['demo-bin']).toMatchObject({
      enabled: true,
      transport: 'stdio',
      command: join(pluginDir, 'bin', 'server'),
      args: ['--stdio'],
      cwd: pluginDir,
      lowPriority: true,
      backgroundStart: true,
      env: {
        DEMO_PLUGIN_TOKEN: 'secret-token',
        DEMO_STATIC_FLAG: '1'
      },
      trustScope: 'user'
    })
  })

  it('projects installed extension account metadata without credential bytes or references', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'extension-demo', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'extension-demo',
      version: '1.0.0',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        'extension-demo': {
          transport: 'streamable-http',
          url: 'https://extension.invalid/mcp',
          trustScope: 'user',
          accountCredential: {
            owner: 'extension',
            provider: 'extension-demo',
            accountId: 'account-a',
            purpose: 'extension-provider-account-token'
          },
          oauthBinding: {
            schemaVersion: 1,
            issuer: 'https://oauth.extension.invalid/',
            authorizationEndpoint: 'https://login.extension.invalid/authorize',
            tokenEndpoint: 'https://tokens.extension.invalid/token',
            revocationEndpoint: 'https://tokens.extension.invalid/revoke',
            clientId: 'extension-public-client',
            scopes: ['openid', 'profile'],
            redirectModeVersion: 1
          }
        }
      }
    }), 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'extension-demo', '1.0.0')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath: join(tempRoot, 'missing-mcp.json')
    })

    const configText = readFileSync(configPath, 'utf8')
    const parsed = JSON.parse(configText) as any
    expect(parsed.capabilities.mcp.servers['extension-demo'].accountCredential).toEqual({
      owner: 'extension',
      provider: 'extension-demo',
      accountId: 'account-a',
      purpose: 'extension-provider-account-token',
      bindingFingerprint: expect.stringMatching(/^[a-f0-9]{64}$/)
    })
    const bindings = await module.listManagedMcpAccountCredentialBindings(dataDir)
    expect(bindings).toHaveLength(1)
    expect(bindings[0]).toMatchObject({
      owner: 'extension',
      provider: 'extension-demo',
      accountId: 'account-a',
      channelId: 'extension-demo',
      purpose: 'extension-provider-account-token',
      oauthBinding: {
        issuer: 'https://oauth.extension.invalid/',
        clientId: 'extension-public-client'
      }
    })
    expect(bindings[0]?.ownerFingerprint).toMatch(/^[a-f0-9]{64}$/)
    const tampered = JSON.parse(configText) as any
    tampered.capabilities.mcp.servers['extension-demo'].accountCredential.accountId = 'forged-account'
    writeFileSync(configPath, `${JSON.stringify(tampered, null, 2)}\n`, 'utf8')
    expect(await module.listManagedMcpAccountCredentialBindings(dataDir)).toEqual([])
    expect(configText).not.toMatch(/credentialRef|valueBase64|synthetic-extension-secret/)
  })

  it('rejects self-authored hub-install markers before MCP discovery', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'forged-plugin', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'forged-plugin',
      version: '1.0.0',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        'forged-plugin': {
          command: 'node',
          args: ['./server.cjs'],
          env: { FORGED_PLUGIN_SECRET: 'must-not-enter-runtime-config' }
        }
      }
    }), 'utf8')
    sealSelfAuthoredHubInstallFixture(pluginDir, 'forged-plugin', '1.0.0')

    const module = await import('./analytix-process')
    await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath: join(tempRoot, 'missing-mcp.json')
    })

    const configText = readFileSync(configPath, 'utf8')
    const parsed = JSON.parse(configText) as any
    expect(parsed.capabilities.mcp.servers['forged-plugin']).toBeUndefined()
    expect(configText).not.toContain('must-not-enter-runtime-config')
    expect(JSON.parse(readFileSync(
      join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json'),
      'utf8'
    ))).toMatchObject({ serverIds: [] })
  })

  it('clears inherited and previously persisted secrets for disabled installed MCP plugins', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const previousToken = process.env.ANALYTIX_API_TOKEN
    const inheritedSecret = 'disabled-inherited-secret-sentinel'
    const staticSecret = 'disabled-static-secret-sentinel'
    const headerSecret = 'disabled-header-secret-sentinel'
    process.env.ANALYTIX_API_TOKEN = inheritedSecret
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'analytix-fund-analysis', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    mkdirSync(join(pluginDir, 'mcp'), { recursive: true })
    mkdirSync(dataDir, { recursive: true })
    writeFileSync(join(pluginDir, 'mcp', 'server.mjs'), 'export {}\n', 'utf8')
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'analytix-fund-analysis',
      version: '1.0.0',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        analytix_funds: {
          command: 'node',
          args: ['./mcp/server.mjs'],
          cwd: '.',
          disabled: true,
          env_vars: ['ANALYTIX_API_TOKEN'],
          env: { STATIC_SECRET: staticSecret },
          headers: { Authorization: `Bearer ${headerSecret}` }
        }
      }
    }), 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'analytix-fund-analysis', '1.0.0')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          servers: {
            analytix_funds: {
              enabled: true,
              transport: 'stdio',
              command: 'node',
              env: {
                ANALYTIX_API_TOKEN: inheritedSecret,
                STATIC_SECRET: staticSecret
              },
              headers: { Authorization: `Bearer ${headerSecret}` }
            }
          }
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    try {
      await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
        mcpConfigPath
      })
    } finally {
      if (previousToken === undefined) {
        delete process.env.ANALYTIX_API_TOKEN
      } else {
        process.env.ANALYTIX_API_TOKEN = previousToken
      }
    }

    const configText = readFileSync(configPath, 'utf8')
    expect(configText).not.toContain(inheritedSecret)
    expect(configText).not.toContain(staticSecret)
    expect(configText).not.toContain(headerSecret)
    const parsed = JSON.parse(configText) as any
    expect(parsed.capabilities.mcp.servers.analytix_funds).toBeUndefined()
  })

  it('does not materialize installed-plugin secrets while MCP is globally disabled', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const previousToken = process.env.DEMO_PLUGIN_TOKEN
    const inheritedSecret = 'global-disabled-inherited-secret-sentinel'
    const staticSecret = 'global-disabled-static-secret-sentinel'
    const headerSecret = 'global-disabled-header-secret-sentinel'
    process.env.DEMO_PLUGIN_TOKEN = inheritedSecret
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'demo-plugin', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'demo-plugin',
      version: '1.0.0',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        'demo-plugin': {
          command: 'node',
          args: ['./server.mjs'],
          cwd: '.',
          env_vars: ['DEMO_PLUGIN_TOKEN'],
          env: { STATIC_SECRET: staticSecret },
          headers: { Authorization: `Bearer ${headerSecret}` }
        }
      }
    }), 'utf8')
    writeFileSync(join(pluginDir, 'server.mjs'), 'export {}\n', 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'demo-plugin', '1.0.0')
    mkdirSync(dataDir, { recursive: true })
    writeFileSync(configPath, JSON.stringify({ capabilities: { mcp: { enabled: false } } }), 'utf8')
    const module = await import('./analytix-process')

    try {
      await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
        mcpConfigPath: join(tempRoot, 'missing-mcp.json')
      })
    } finally {
      if (previousToken === undefined) delete process.env.DEMO_PLUGIN_TOKEN
      else process.env.DEMO_PLUGIN_TOKEN = previousToken
    }

    const configText = readFileSync(configPath, 'utf8')
    expect(configText).not.toContain(inheritedSecret)
    expect(configText).not.toContain(staticSecret)
    expect(configText).not.toContain(headerSecret)
    const parsed = JSON.parse(configText) as any
    expect(parsed.capabilities.mcp.enabled).toBe(false)
    expect(parsed.capabilities.mcp.servers['demo-plugin']).toBeUndefined()
  })

  it('quarantines colliding installed plugin generations before reading env vars', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const previousToken = process.env.ANALYTIX_API_TOKEN
    const staleSecret = 'stale-generation-secret-sentinel'
    process.env.ANALYTIX_API_TOKEN = staleSecret
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    for (const [version, disabled] of [['0.9.0', false], ['0.16.0', true]] as const) {
      const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'analytix-fund-analysis', version)
      mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
      mkdirSync(join(pluginDir, 'mcp'), { recursive: true })
      writeFileSync(join(pluginDir, 'mcp', 'server.mjs'), 'export {}\n', 'utf8')
      writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
        name: 'analytix-fund-analysis',
        version,
        mcpServers: './.mcp.json'
      }), 'utf8')
      writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
        mcpServers: {
          analytix_funds: {
            command: 'node',
            args: ['./mcp/server.mjs'],
            cwd: '.',
            disabled,
            env_vars: ['ANALYTIX_API_TOKEN']
          }
        }
      }), 'utf8')
      sealAuthorizedLocalRemountFixture(pluginDir, 'analytix-fund-analysis', version)
    }
    mkdirSync(join(dataDir, '.cache'), { recursive: true })
    writeFileSync(join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json'), JSON.stringify(['analytix_funds']), 'utf8')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          servers: {
            analytix_funds: {
              enabled: true,
              transport: 'stdio',
              command: 'node',
              env: { ANALYTIX_API_TOKEN: staleSecret }
            }
          }
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    try {
      await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
        mcpConfigPath: join(tempRoot, 'missing-mcp.json')
      })
    } finally {
      if (previousToken === undefined) delete process.env.ANALYTIX_API_TOKEN
      else process.env.ANALYTIX_API_TOKEN = previousToken
    }

    const configText = readFileSync(configPath, 'utf8')
    expect(configText).not.toContain(staleSecret)
    const parsed = JSON.parse(configText) as any
    expect(parsed.capabilities.mcp.servers.analytix_funds).toBeUndefined()
  })

  it('drops installed plugin MCP servers whose relative paths escape the plugin directory', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'escape-plugin', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'escape-plugin',
      version: '1.0.0',
      mcpServers: './.mcp.json'
    }), 'utf8')
    writeFileSync(join(pluginDir, '.mcp.json'), JSON.stringify({
      mcpServers: {
        safe: {
          command: 'node',
          args: ['./mcp/server.cjs'],
          cwd: '.'
        },
        'escape-cwd': {
          command: './server.cjs',
          args: ['--stdio'],
          cwd: '..'
        },
        'escape-arg': {
          command: 'node',
          args: ['../outside.cjs'],
          cwd: '.'
        }
      }
    }), 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'escape-plugin', '1.0.0')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.servers).toEqual({})
  })

  it('does not register plugin apps or resources as runtime MCP servers', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const dataDir = join(tempRoot, 'data')
    const configPath = join(dataDir, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    const pluginDir = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'browser-app', '1.0.0')
    mkdirSync(join(pluginDir, '.codex-plugin'), { recursive: true })
    mkdirSync(join(dataDir, '.cache'), { recursive: true })
    writeFileSync(join(pluginDir, '.codex-plugin', 'plugin.json'), JSON.stringify({
      name: 'browser-app',
      version: '1.0.0',
      apps: {
        browser: {
          resources: ['browser://tabs'],
          connector: 'browser'
        }
      }
    }), 'utf8')
    sealAuthorizedLocalRemountFixture(pluginDir, 'browser-app', '1.0.0')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          enabled: true,
          servers: {
            'app-resource': {
              enabled: true,
              transport: 'stdio',
              command: '/tmp/stale-app-resource',
              trustScope: 'user'
            },
            manual: {
              enabled: true,
              transport: 'stdio',
              command: 'manual-mcp',
              trustScope: 'user'
            }
          }
        }
      }
    }), 'utf8')
    writeBoundPluginMcpSidecar(dataDir, configPath, ['app-resource'])
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(dataDir, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.servers['app-resource']).toBeUndefined()
    expect(parsed.capabilities.mcp.servers.browser).toBeUndefined()
    expect(parsed.capabilities.mcp.servers.manual).toMatchObject({
      transport: 'stdio',
      command: 'manual-mcp',
      trustScope: 'user'
    })
    expect(JSON.parse(readFileSync(join(dataDir, '.cache', 'gui-plugin-mcp-server-ids.json'), 'utf8'))).toMatchObject({
      schemaVersion: 1,
      contract: 'analytix-plugin-managed-mcp-server-ids',
      serverIds: []
    })
  })

  it('removes stale plugin-managed MCP servers after a plugin is uninstalled', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    mkdirSync(join(tempRoot, '.cache'), { recursive: true })
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          enabled: true,
          servers: {
            'demo-plugin': {
              enabled: true,
              transport: 'stdio',
              command: '/tmp/stale-plugin-server',
              trustScope: 'user'
            },
            manual: {
              enabled: true,
              transport: 'stdio',
              command: 'manual-mcp',
              trustScope: 'user'
            }
          }
        }
      }
    }), 'utf8')
    writeBoundPluginMcpSidecar(tempRoot, configPath, ['demo-plugin'])
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.servers['demo-plugin']).toBeUndefined()
    expect(parsed.capabilities.mcp.servers.manual).toMatchObject({
      transport: 'stdio',
      command: 'manual-mcp'
    })
  })

  it('does not let legacy or hash-mismatched sidecars delete manual MCP servers', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const sidecarPath = join(tempRoot, '.cache', 'gui-plugin-mcp-server-ids.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    mkdirSync(join(tempRoot, '.cache'), { recursive: true })
    const writeManualConfig = (): void => writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          servers: {
            manual: {
              enabled: true,
              transport: 'stdio',
              command: 'manual-mcp',
              trustScope: 'user'
            }
          }
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    writeManualConfig()
    writeFileSync(sidecarPath, JSON.stringify(['manual']), 'utf8')
    chmodSync(sidecarPath, 0o600)
    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), { mcpConfigPath })
    expect((JSON.parse(readFileSync(configPath, 'utf8')) as any).capabilities.mcp.servers.manual).toBeDefined()

    writeManualConfig()
    writeFileSync(sidecarPath, JSON.stringify({
      schemaVersion: 1,
      contract: 'analytix-plugin-managed-mcp-server-ids',
      configSha256: '0'.repeat(64),
      serverIds: ['manual']
    }), 'utf8')
    chmodSync(sidecarPath, 0o600)
    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), { mcpConfigPath })
    expect((JSON.parse(readFileSync(configPath, 'utf8')) as any).capabilities.mcp.servers.manual).toBeDefined()
    expect(JSON.parse(readFileSync(sidecarPath, 'utf8'))).toMatchObject({ serverIds: [] })
  })

  it('recovers a stale plugin config without trusting a corrupt sidecar or erasing manual config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const sidecarPath = join(tempRoot, '.cache', 'gui-plugin-mcp-server-ids.json')
    const pluginRootPath = join(tempRoot, 'plugins', 'cache', 'analytix-hub', 'stale-plugin', '1.0.0')
    mkdirSync(join(tempRoot, '.cache'), { recursive: true })
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          servers: {
            'stale-plugin': {
              enabled: true,
              transport: 'stdio',
              command: 'node',
              trustScope: 'user',
              expectedServerName: 'stale-plugin',
              expectedServerVersion: '1.0.0',
              identitySource: 'installed-plugin-manifest',
              manifestSha256: 'a'.repeat(64),
              pluginRootPath,
              sourceTreeSha256: 'b'.repeat(64)
            },
            manual: {
              enabled: true,
              transport: 'stdio',
              command: 'manual-mcp',
              trustScope: 'user'
            }
          }
        }
      }
    }), 'utf8')
    writeFileSync(sidecarPath, '{"schemaVersion":1,"schemaVersion":2}', 'utf8')
    chmodSync(sidecarPath, 0o600)
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath: join(tempRoot, 'missing-mcp.json')
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.servers['stale-plugin']).toBeUndefined()
    expect(parsed.capabilities.mcp.servers.manual).toMatchObject({ command: 'manual-mcp' })
    expect(JSON.parse(readFileSync(sidecarPath, 'utf8'))).toMatchObject({ serverIds: [] })
  })

  it('rejects a symlink sidecar without following or replacing it', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const cacheDir = join(tempRoot, '.cache')
    const sidecarPath = join(cacheDir, 'gui-plugin-mcp-server-ids.json')
    const externalPath = join(tempRoot, 'external-sidecar.json')
    mkdirSync(cacheDir, { recursive: true })
    writeFileSync(configPath, JSON.stringify({
      capabilities: { mcp: { servers: { manual: { transport: 'stdio', command: 'manual-mcp', trustScope: 'user' } } } }
    }), 'utf8')
    writeFileSync(externalPath, JSON.stringify(['manual']), 'utf8')
    symlinkSync(externalPath, sidecarPath)
    const module = await import('./analytix-process')

    await expect(module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath: join(tempRoot, 'missing-mcp.json')
    })).rejects.toThrow('Refusing to replace unsafe plugin-managed MCP sidecar')
    expect(readFileSync(externalPath, 'utf8')).toBe(JSON.stringify(['manual']))
    expect((JSON.parse(readFileSync(configPath, 'utf8')) as any).capabilities.mcp.servers.manual).toBeDefined()
  })

  it('removes stale plugin-managed skill roots after a Hub plugin is uninstalled', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    const mcpConfigPath = join(tempRoot, 'missing-mcp.json')
    const stalePluginSkillRoot = join(
      homedir(),
      '.analytix',
      'plugins',
      'cache',
      'analytix-hub',
      'demo-plugin',
      '1.0.0',
      'skills'
    )
    const manualSkillRoot = join(tempRoot, 'manual-skills')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        skills: {
          enabled: true,
          roots: [stalePluginSkillRoot, manualSkillRoot]
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      mcpConfigPath
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.skills.roots).not.toContain(stalePluginSkillRoot)
    expect(parsed.capabilities.skills.roots).toContain(manualSkillRoot)
  })

  it('replaces unparsable historical Analytix config with a valid GUI-managed config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    writeFileSync(configPath, '{ legacy config', 'utf8')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings())

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as unknown
    expect(AnalytixConfigSchema.safeParse(parsed).success).toBe(true)
  })

  it('does not enable MCP when the capability is explicitly disabled', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        mcp: {
          enabled: false
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    const syncResult = await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings(), {
      scheduleMcp: {
        settings: createSettings('/tmp/fake-analytix-child.js'),
        launch: {
          appPath: '/tmp/analytix-test-app',
          execPath: '/tmp/electron',
          isPackaged: false
        }
      }
    })

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.mcp.enabled).toBe(false)
    expect(parsed.capabilities.mcp.servers.gui_schedule).toBeUndefined()
    expect(syncResult.hostScheduleMcpBindingV1).toBeNull()
  })

  it('scrubs an obsolete attachment capability from production runtime config', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        attachments: {
          enabled: false,
          maxImageBytes: 1024
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings())

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.attachments).toBeUndefined()
  })

  it('does not override explicitly disabled web fetch capability', async () => {
    if (!tempRoot) throw new Error('temp root not initialized')
    const configPath = join(tempRoot, 'config.json')
    writeFileSync(configPath, JSON.stringify({
      capabilities: {
        web: {
          enabled: false,
          fetchEnabled: false,
          searchEnabled: true,
          provider: 'custom-search'
        }
      }
    }), 'utf8')
    const module = await import('./analytix-process')

    await module.syncGuiManagedAnalytixConfig(tempRoot, defaultAnalytixRuntimeSettings())

    const parsed = JSON.parse(readFileSync(configPath, 'utf8')) as any
    expect(parsed.capabilities.web).toMatchObject({
      enabled: false,
      fetchEnabled: false,
      searchEnabled: true,
      provider: 'custom-search'
    })
  })
})
