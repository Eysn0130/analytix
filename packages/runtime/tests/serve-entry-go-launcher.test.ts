import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_SERVE_OPTIONS } from '../src/cli/cli-options.js'
import {
  projectAnalytixServeReadyPublicV1,
  startGoRuntimeServe
} from '../src/cli/serve-entry.js'

const tempRoots: string[] = []
const TEST_AUTHORITY_PUBLIC_KEY = Buffer.alloc(32, 0x51).toString('base64url')
const TEST_AUTHORITY_KEY_ID = createHash('sha256')
  .update(Buffer.from(TEST_AUTHORITY_PUBLIC_KEY, 'base64url'))
  .digest('hex')

function tempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-serve-go-'))
  tempRoots.push(root)
  return root
}

function fakeReadyPayloadSource(
  port: number,
  overrides: Record<string, unknown> = {}
): string {
  return `({
  url: 'http://127.0.0.1:${port}',
  runtimePid: process.pid,
  runtimeTokenConfigured: Boolean(process.env.ANALYTIX_RUNTIME_TOKEN),
  persistenceRootsConfigured: true,
  productionRuntime: true,
  controlledArtifactHostV2Configured: false,
  controlledArtifactHostV2Ready: false,
  controlledArtifactHostV2BackendGeneration: 0,
  controlledArtifactHostV2LaunchBindingProof: '',
  finalPublicationAuthorityKeyId: ${JSON.stringify(TEST_AUTHORITY_KEY_ID)},
  finalPublicationAuthorityPublicKey: ${JSON.stringify(TEST_AUTHORITY_PUBLIC_KEY)},
  witnessedAuthorityV2Configured: false,
  witnessedAuthorityInstallationId: '',
  witnessedAuthorityKeyId: '',
  witnessedAuthorityManifestDigest: '',
  datasetSnapshotSelectionV2Configured: false,
  datasetSnapshotAdmissionV2State: 'absent',
  datasetSnapshotAdmissionV2InstallationId: '',
  datasetSnapshotAdmissionV2RuntimeLaunchNonce: '',
  datasetSnapshotAdmissionV2StagingBindingDigest: '',
  datasetSnapshotAdmissionV2SelectionDigest: '',
  datasetSnapshotAdmissionV2SnapshotId: '',
  datasetSnapshotAdmissionV2AuthorityRecordDigest: '',
  datasetSnapshotAdmissionV2AckHmacSha256: '',
  ...${JSON.stringify(overrides)}
})`
}

function fakeReadyRuntimeSource(
  port: number,
  overrides: Record<string, unknown> = {},
  omittedKeys: readonly string[] = []
): string {
  return `
const payload = ${fakeReadyPayloadSource(port, overrides)}
for (const key of ${JSON.stringify(omittedKeys)}) delete payload[key]
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + JSON.stringify(payload) + '\\n')
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`
}

afterEach(() => {
  vi.restoreAllMocks()
  while (tempRoots.length > 0) {
    const root = tempRoots.pop()
    if (root) rmSync(root, { recursive: true, force: true })
  }
})

describe('analytix serve Go launcher', () => {
  it('does not import the retired TypeScript runtime factory', async () => {
    const source = await readFile(new URL('../src/cli/serve-entry.ts', import.meta.url), 'utf8')

    expect(source).not.toContain('../server/runtime-factory')
    expect(source).not.toContain('startAnalytixServe')
    expect(source).toContain('ANALYTIX_RUNTIME_SERVER_READY ')
  })

  it('projects ready and startup output from one positive public allowlist', async () => {
    const source = await readFile(new URL('../src/cli/serve-entry.ts', import.meta.url), 'utf8')
    const serveMainStart = source.indexOf('async function serveMain(')
    const serveMainEnd = source.indexOf('\nexport async function startGoRuntimeServe(', serveMainStart)
    const serveMain = source.slice(serveMainStart, serveMainEnd)

    expect(serveMain).toContain('projectAnalytixServeReadyPublicV1({')
    expect(serveMain).not.toMatch(
      /\b(?:host|url|configPath|dataDir|durableRoot|model|approvalPolicy|sandboxMode|insecure|startedAt|pid|runtime|productionRuntime|message)\s*:/u
    )

    const privatePath = '/Users/private/SENSITIVE_READY_PATH'
    const projected = projectAnalytixServeReadyPublicV1({
      port: 4567,
      host: '127.0.0.1',
      url: 'http://127.0.0.1:4567',
      configPath: `${privatePath}/config.json`,
      dataDir: `${privatePath}/data`,
      durableRoot: `${privatePath}/durable`,
      model: 'private-model',
      approvalPolicy: 'private-policy',
      sandboxMode: 'private-sandbox',
      pid: 12345,
      message: privatePath
    })
    expect(projected).toEqual({ service: 'analytix', mode: 'serve', port: 4567 })
    expect(JSON.stringify(projected)).not.toContain(privatePath)
    expect(Object.isFrozen(projected)).toBe(true)
    expect(() => projectAnalytixServeReadyPublicV1({ port: 0 })).toThrow(
      'requires a valid bound port'
    )
  })

  it('projects a top-level startup rejection to one fixed stderr event', async () => {
    const source = await readFile(new URL('../src/cli/serve-entry.ts', import.meta.url), 'utf8')
    const entrypointStart = source.lastIndexOf('if (process.argv[1]')
    const entrypoint = source.slice(entrypointStart)

    expect(entrypointStart).toBeGreaterThanOrEqual(0)
    expect(entrypoint).toContain("process.stderr.write('[analytix] event=ANALYTIX_SERVE_STARTUP_FAILED\\n')")
    expect(entrypoint).not.toContain('String(error)')
    expect(entrypoint).not.toContain('error.message')
    expect(entrypoint).not.toContain('error.stack')
    expect(entrypoint).toContain('process.exit(ServeExitCode.runtime)')
  })

  it('projects serve crash handlers to one fixed stderr event', async () => {
    const source = await readFile(new URL('../src/cli/serve-entry.ts', import.meta.url), 'utf8')
    const crashHandlerStart = source.indexOf('function installServeCrashHandlers(')
    const crashHandlerEnd = source.indexOf('\n/**\n * Serve-mode command.', crashHandlerStart)
    const crashHandler = source.slice(crashHandlerStart, crashHandlerEnd)

    expect(crashHandlerStart).toBeGreaterThanOrEqual(0)
    expect(crashHandlerEnd).toBeGreaterThan(crashHandlerStart)
    expect(crashHandler).toContain("process.stderr.write('[analytix] event=ANALYTIX_SERVE_CRASHED\\n')")
    expect(crashHandler).not.toContain('error.stack')
    expect(crashHandler).not.toContain('error.message')
    expect(crashHandler).not.toContain('String(error)')
    expect(crashHandler).toContain('.close()')
    expect(crashHandler).toContain('.finally(finish)')
  })

  it('requires an explicit token unless insecure mode is explicitly selected', async () => {
    await expect(startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(tempRoot(), 'data')
    }, {})).rejects.toThrow('runtime token is required')
  })

  it.each([
    {
      name: 'a preconstructed API key',
      options: { apiKey: 'synthetic-preconstructed-provider-credential' },
      env: {}
    },
    {
      name: 'preconstructed credential-bearing Provider metadata',
      options: {
        modelProviders: {
          providers: [{
            id: 'provider-preconstructed',
            baseUrl: 'https://provider.invalid/v1',
            apiKey: 'synthetic-preconstructed-provider-credential',
            models: []
          }]
        }
      },
      env: {}
    },
    {
      name: 'an ambient API key',
      options: {},
      env: { ANALYTIX_API_KEY: 'synthetic-ambient-provider-credential' }
    },
    {
      name: 'ambient credential-bearing Provider metadata',
      options: {},
      env: {
        ANALYTIX_MODEL_PROVIDERS: JSON.stringify({
          providers: [{
            id: 'provider-ambient',
            baseUrl: 'https://provider.invalid/v1',
            apiKey: 'synthetic-ambient-provider-credential'
          }]
        })
      }
    }
  ])('rejects $name before spawning Go', async ({ options, env }) => {
    await expect(startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      ...options,
      dataDir: join(tempRoot(), 'data'),
      runtimeToken: 'test-token'
    }, env)).rejects.toThrow(/Registry authority/)
  })

  it('keeps child environment credential-free and does not pass copied Provider authority in argv', async () => {
    const root = tempRoot()
    const capturePath = join(root, 'captured-provider-authority.json')
    const fakeRuntimeServer = join(root, 'fake-runtime-server.mjs')
    const hubGatewayCanary = 'synthetic-hub-gateway-bootstrap'
    const hubDesktopCanary = 'synthetic-hub-desktop-bootstrap'
    const systemRoot = process.env.SystemRoot || 'C:\\Windows'
    writeFileSync(fakeRuntimeServer, `
import { writeFileSync } from 'node:fs'
const valueAfter = (name) => {
  const index = process.argv.indexOf(name)
  return index >= 0 ? process.argv[index + 1] : null
}
writeFileSync(process.env.ANALYTIX_CAPTURE_PROVIDER_AUTHORITY, JSON.stringify({
  providerEnvironmentKeys: [
    'ANALYTIX_API_KEY',
    'ANALYTIX_MODEL_PROVIDERS',
    'ANALYTIX_HUB_TEST_GATEWAY_TOKEN',
    'ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN'
  ].flatMap((name) => Object.keys(process.env).filter((key) => key.toUpperCase() === name)),
  runtimeTokenKeys: Object.keys(process.env).filter((key) => key.toUpperCase() === 'ANALYTIX_RUNTIME_TOKEN'),
  runtimeToken: process.env.ANALYTIX_RUNTIME_TOKEN,
  systemRoot: process.env.SystemRoot,
  path: process.env.Path,
  modelProvidersJSON: valueAfter('--model-providers-json'),
  baseUrl: valueAfter('--base-url'),
  modelProxyUrl: valueAfter('--model-proxy-url'),
  endpointFormat: valueAfter('--endpoint-format'),
  model: valueAfter('--model'),
  argv: process.argv
}))
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + JSON.stringify(${fakeReadyPayloadSource(4577)}) + '\\n')
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`, 'utf8')

    const handle = await startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(root, 'data'),
      port: 4577,
      runtimeToken: 'test-token',
      modelProviders: {
        defaultProviderId: 'provider-key-free',
        providers: [{
          id: 'provider-key-free',
          baseUrl: 'https://provider.invalid/v1',
          models: ['model-key-free']
        }]
      }
    }, {
      ...Object.fromEntries(Object.entries(process.env).filter(([name]) =>
        !['PATH', 'SYSTEMROOT'].includes(name.toUpperCase())
      )),
      ANALYTIX_API_KEY: '',
      ANALYTIX_MODEL_PROVIDERS: '',
      ANALYTIX_HUB_TEST_GATEWAY_TOKEN: hubGatewayCanary,
      ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN: hubDesktopCanary,
      analytix_api_key: 'synthetic-casefold-credential',
      Analytix_Model_Providers: 'synthetic-casefold-credential',
      Analytix_Hub_Test_Gateway_Token: hubGatewayCanary,
      analytix_hub_test_desktop_auth_token: hubDesktopCanary,
      analytix_runtime_token: 'synthetic-stale-runtime-token',
      SystemRoot: systemRoot,
      Path: 'C:\\SyntheticTools',
      ANALYTIX_CAPTURE_PROVIDER_AUTHORITY: capturePath,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })

    try {
      const captured = JSON.parse(await readFile(capturePath, 'utf8')) as {
        providerEnvironmentKeys: string[]
        runtimeTokenKeys: string[]
        runtimeToken: string
        systemRoot: string
        path: string
        modelProvidersJSON: string | null
        baseUrl: string | null
        modelProxyUrl: string | null
        endpointFormat: string | null
        model: string | null
        argv: string[]
      }
      expect(captured.providerEnvironmentKeys).toEqual([])
      expect(captured.runtimeTokenKeys).toEqual(['ANALYTIX_RUNTIME_TOKEN'])
      expect(captured.runtimeToken).toBe('test-token')
      expect(captured.systemRoot).toBe(systemRoot)
      expect(captured.path).toBe('C:\\SyntheticTools')
      expect(captured.modelProvidersJSON).toBeNull()
      expect(captured.baseUrl).toBeNull()
      expect(captured.modelProxyUrl).toBeNull()
      expect(captured.endpointFormat).toBeNull()
      expect(captured.model).toBeNull()
      expect(JSON.stringify(captured)).not.toContain(hubGatewayCanary)
      expect(JSON.stringify(captured)).not.toContain(hubDesktopCanary)
    } finally {
      await handle.close()
    }
  })

  it('starts a Go-runtime-compatible process and waits for the runtime-server ready payload', async () => {
    const root = tempRoot()
    const dataDir = join(root, 'data')
    const fakeRuntimeServer = join(root, 'fake-runtime-server.mjs')
    writeFileSync(fakeRuntimeServer, fakeReadyRuntimeSource(4567), 'utf8')

    const handle = await startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir,
      port: 4567,
      runtimeToken: 'test-token',
      model: 'deepseek-chat'
    }, {
      ...process.env,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })

    try {
      expect(handle.ready).toEqual(expect.objectContaining({
        url: 'http://127.0.0.1:4567',
        runtimeTokenConfigured: true,
        productionRuntime: true,
        controlledArtifactHostV2Configured: false,
        witnessedAuthorityV2Configured: false,
        datasetSnapshotSelectionV2Configured: false,
        datasetSnapshotAdmissionV2State: 'absent'
      }))
      expect(handle.port).toBe(4567)
    } finally {
      await handle.close()
    }
  })

  it('suppresses raw child stdout before and after the internal ready marker', async () => {
    const root = tempRoot()
    const fakeRuntimeServer = join(root, 'fake-noisy-runtime-server.mjs')
    const beforeReady = 'HOSTILE_CHILD_STDOUT_BEFORE_READY /private/before-ready'
    const afterReady = 'HOSTILE_CHILD_STDOUT_AFTER_READY /private/after-ready'
    writeFileSync(fakeRuntimeServer, `
process.stdout.write(${JSON.stringify(`${beforeReady}\n`)})
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + JSON.stringify(${fakeReadyPayloadSource(4575)}) + '\\n')
setTimeout(() => process.stdout.write(${JSON.stringify(`${afterReady}\n`)}), 20)
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`, 'utf8')
    const stdoutWrite = vi.spyOn(process.stdout, 'write').mockImplementation(() => true)

    const handle = await startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(root, 'data'),
      port: 4575,
      runtimeToken: 'test-token'
    }, {
      ...process.env,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })

    try {
      await new Promise((resolve) => setTimeout(resolve, 75))
      const parentStdout = stdoutWrite.mock.calls.map(([chunk]) => String(chunk)).join('')
      expect(parentStdout).not.toContain(beforeReady)
      expect(parentStdout).not.toContain(afterReady)
      expect(parentStdout).not.toContain('ANALYTIX_RUNTIME_SERVER_READY')
    } finally {
      await handle.close()
    }
  })

  it('discards raw child stderr before and after the internal ready marker', async () => {
    const root = tempRoot()
    const fakeRuntimeServer = join(root, 'fake-noisy-stderr-runtime-server.mjs')
    const beforeReady =
      'HOSTILE_CHILD_STDERR_BEFORE_READY path=/private/before-ready pii=6222020202020202022'
    const afterReady =
      'HOSTILE_CHILD_STDERR_AFTER_READY model=private-model pid=987654 timestamp=2026-08-26T10:00:00Z'
    writeFileSync(fakeRuntimeServer, `
process.stderr.write(${JSON.stringify(`${beforeReady}\n`)})
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + JSON.stringify(${fakeReadyPayloadSource(4576)}) + '\\n')
setTimeout(() => process.stderr.write(${JSON.stringify(`${afterReady}\n`)}), 20)
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`, 'utf8')
    const stderrWrite = vi.spyOn(process.stderr, 'write').mockImplementation(() => true)

    const handle = await startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(root, 'data'),
      port: 4576,
      runtimeToken: 'test-token'
    }, {
      ...process.env,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })

    try {
      await new Promise((resolve) => setTimeout(resolve, 75))
      const parentStderr = stderrWrite.mock.calls.map(([chunk]) => String(chunk)).join('')
      expect(parentStderr).not.toContain(beforeReady)
      expect(parentStderr).not.toContain(afterReady)
      expect(parentStderr).not.toContain('/private/before-ready')
      expect(parentStderr).not.toContain('6222020202020202022')
      expect(parentStderr).not.toContain('private-model')
      expect(parentStderr).not.toContain('987654')
      expect(parentStderr).not.toContain('2026-08-26T10:00:00Z')
    } finally {
      await handle.close()
    }
  })

  it('omits raw child stderr from a pre-ready exit error', async () => {
    const root = tempRoot()
    const fakeRuntimeServer = join(root, 'fake-pre-ready-exit-runtime-server.mjs')
    const hostileStderr =
      'HOSTILE_CHILD_STDERR_EXIT path=/private/pre-ready-exit pii=110101199001011234'
    writeFileSync(fakeRuntimeServer, `
process.stderr.write(${JSON.stringify(`${hostileStderr}\n`)})
process.exit(23)
`, 'utf8')
    const stderrWrite = vi.spyOn(process.stderr, 'write').mockImplementation(() => true)

    let startupError: unknown
    try {
      await startGoRuntimeServe({
        ...DEFAULT_SERVE_OPTIONS,
        dataDir: join(root, 'data'),
        runtimeToken: 'test-token'
      }, {
        ...process.env,
        ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
        ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
      })
    } catch (error) {
      startupError = error
    }

    const startupMessage = String(startupError)
    const parentStderr = stderrWrite.mock.calls.map(([chunk]) => String(chunk)).join('')
    expect(startupMessage).toBe(
      'Error: Go runtime-server exited before ready with code 23 signal none.'
    )
    expect(startupMessage).not.toContain(hostileStderr)
    expect(startupMessage).not.toContain('/private/pre-ready-exit')
    expect(startupMessage).not.toContain('110101199001011234')
    expect(parentStderr).not.toContain(hostileStderr)
  })

  it('does not forward cold copied Provider defaults to the Go runtime', async () => {
    const root = tempRoot()
    const capturePath = join(root, 'captured-provider-route.json')
    const fakeRuntimeServer = join(root, 'fake-runtime-server.mjs')
    writeFileSync(fakeRuntimeServer, `
import { writeFileSync } from 'node:fs'
const valueAfter = (name) => {
  const index = process.argv.indexOf(name)
  return index >= 0 ? process.argv[index + 1] : null
}
writeFileSync(process.env.ANALYTIX_CAPTURE_PROVIDER_ROUTE, JSON.stringify({
  baseUrl: valueAfter('--base-url'),
  endpointFormat: valueAfter('--endpoint-format'),
  model: valueAfter('--model')
}))
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + JSON.stringify(${fakeReadyPayloadSource(4569)}) + '\\n')
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`, 'utf8')

    const handle = await startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(root, 'data'),
      port: 4569,
      runtimeToken: 'test-token'
    }, {
      ...process.env,
      ANALYTIX_CAPTURE_PROVIDER_ROUTE: capturePath,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })

    try {
      expect(JSON.parse(await readFile(capturePath, 'utf8'))).toEqual({
        baseUrl: null,
        endpointFormat: null,
        model: null
      })
    } finally {
      await handle.close()
    }
  })

  it.each([0, 45_000])(
    'forwards config containing streamIdleTimeoutMs=%s to the Go runtime',
    async (streamIdleTimeoutMs) => {
      const root = tempRoot()
      const dataDir = join(root, 'data')
      const configPath = join(root, 'config.json')
      const capturePath = join(root, 'captured-config.json')
      const fakeRuntimeServer = join(root, 'fake-runtime-server.mjs')
      writeFileSync(configPath, JSON.stringify({ runtime: { streamIdleTimeoutMs } }), 'utf8')
      writeFileSync(fakeRuntimeServer, `
import { readFileSync, writeFileSync } from 'node:fs'
const configPathIndex = process.argv.indexOf('--mcp-config-path')
const configPath = configPathIndex >= 0 ? process.argv[configPathIndex + 1] : ''
const config = configPath ? JSON.parse(readFileSync(configPath, 'utf8')) : null
writeFileSync(process.env.ANALYTIX_CAPTURE_CONFIG_PATH, JSON.stringify({
  configPath,
  config,
  runtimeTokenInArgv: process.argv.includes('--runtime-token')
}))
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + JSON.stringify(${fakeReadyPayloadSource(4568)}) + '\\n')
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`, 'utf8')

      const handle = await startGoRuntimeServe({
        ...DEFAULT_SERVE_OPTIONS,
        configPath,
        dataDir,
        port: 4568,
        runtimeToken: 'test-token'
      }, {
        ...process.env,
        ANALYTIX_CAPTURE_CONFIG_PATH: capturePath,
        ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
        ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
      })

      try {
        expect(JSON.parse(await readFile(capturePath, 'utf8'))).toEqual({
          configPath,
          config: { runtime: { streamIdleTimeoutMs } },
          runtimeTokenInArgv: false
        })
      } finally {
        await handle.close()
      }
    }
  )

  it.each([
    {
      name: 'missing persistence proof',
      payload: { url: 'http://127.0.0.1:4570', runtimeTokenConfigured: true, productionRuntime: true }
    },
    {
      name: 'non-production runtime',
      payload: {
        url: 'http://127.0.0.1:4571',
        runtimeTokenConfigured: true,
        persistenceRootsConfigured: true,
        productionRuntime: false
      }
    },
    {
      name: 'non-loopback URL',
      payload: {
        url: 'http://0.0.0.0:4572',
        runtimeTokenConfigured: true,
        persistenceRootsConfigured: true,
        productionRuntime: true
      }
    },
    {
      name: 'credential field',
      payload: {
        url: 'http://127.0.0.1:4573',
        runtimeTokenConfigured: true,
        persistenceRootsConfigured: true,
        productionRuntime: true,
        runtimeToken: 'leaked'
      }
    }
  ])('rejects a ready payload with $name', async ({ payload }) => {
    const root = tempRoot()
    const fakeRuntimeServer = join(root, 'fake-invalid-runtime-server.mjs')
    writeFileSync(fakeRuntimeServer, `
process.stdout.write('ANALYTIX_RUNTIME_SERVER_READY ' + ${JSON.stringify(JSON.stringify(payload))} + '\\n')
process.on('SIGTERM', () => process.exit(0))
setInterval(() => undefined, 1000)
`, 'utf8')

    await expect(startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(root, 'data'),
      runtimeToken: 'test-token'
    }, {
      ...process.env,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })).rejects.toThrow(/invalid ready payload|production persistence readiness/)
  })

  it.each([
    {
      name: 'an unknown key',
      overrides: { unexpectedCapability: true }
    },
    {
      name: 'a missing protected proof key',
      omittedKeys: ['datasetSnapshotAdmissionV2AckHmacSha256']
    },
    {
      name: 'a string boolean',
      overrides: { witnessedAuthorityV2Configured: 'false' }
    },
    {
      name: 'a fractional PID',
      overrides: { runtimePid: 1.5 }
    },
    {
      name: 'a malformed publication authority hash',
      overrides: { finalPublicationAuthorityKeyId: 'A'.repeat(64) }
    },
    {
      name: 'configured controlled-artifact authority',
      overrides: {
        controlledArtifactHostV2Configured: true,
        controlledArtifactHostV2Ready: true,
        controlledArtifactHostV2BackendGeneration: 1,
        controlledArtifactHostV2LaunchBindingProof: '1'.repeat(64)
      }
    },
    {
      name: 'configured witnessed authority',
      overrides: {
        witnessedAuthorityV2Configured: true,
        witnessedAuthorityInstallationId: '1'.repeat(64),
        witnessedAuthorityKeyId: '2'.repeat(64),
        witnessedAuthorityManifestDigest: '3'.repeat(64)
      }
    },
    {
      name: 'an admitted dataset snapshot',
      overrides: {
        datasetSnapshotSelectionV2Configured: true,
        datasetSnapshotAdmissionV2State: 'admitted',
        datasetSnapshotAdmissionV2InstallationId: '1'.repeat(64),
        datasetSnapshotAdmissionV2RuntimeLaunchNonce: '2'.repeat(64),
        datasetSnapshotAdmissionV2StagingBindingDigest: '3'.repeat(64),
        datasetSnapshotAdmissionV2SelectionDigest: '4'.repeat(64),
        datasetSnapshotAdmissionV2SnapshotId: `dsv2_${'5'.repeat(64)}`,
        datasetSnapshotAdmissionV2AuthorityRecordDigest: '6'.repeat(64),
        datasetSnapshotAdmissionV2AckHmacSha256: '7'.repeat(64)
      }
    }
  ])('rejects the full ready schema when it contains $name', async ({
    overrides = {},
    omittedKeys = []
  }) => {
    const root = tempRoot()
    const fakeRuntimeServer = join(root, 'fake-protected-runtime-server.mjs')
    writeFileSync(fakeRuntimeServer, fakeReadyRuntimeSource(4574, overrides, omittedKeys), 'utf8')

    await expect(startGoRuntimeServe({
      ...DEFAULT_SERVE_OPTIONS,
      dataDir: join(root, 'data'),
      port: 4574,
      runtimeToken: 'test-token'
    }, {
      ...process.env,
      ANALYTIX_RUNTIME_SERVER_BINARY: process.execPath,
      ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON: JSON.stringify([fakeRuntimeServer])
    })).rejects.toThrow('invalid ready payload')
  })
})
