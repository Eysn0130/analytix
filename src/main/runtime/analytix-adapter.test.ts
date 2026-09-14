import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import { execFileSync, spawnSync, type ChildProcess } from 'node:child_process'
import { createHash } from 'node:crypto'
import { EventEmitter } from 'node:events'
import { mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import type { AddressInfo } from 'node:net'
import { tmpdir } from 'node:os'
import { delimiter, dirname, join } from 'node:path'
import { PassThrough } from 'node:stream'
import { afterEach, describe, expect, it } from 'vitest'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  resolveAnalytixRuntimeSettings,
  type AppSettingsV1
} from '../../shared/app-settings'
import { analytixThreadSteerPath } from '../../shared/analytix-endpoints'
import { sanitizePublicRuntimeValue } from '../../shared/public-runtime-content'
import { createProviderRegistryIpcHandler } from '../ipc/provider-registry-ipc'
import { RuntimeInfoResponse as RuntimeInfoResponseSchema } from '../../../packages/runtime/src/contracts/runtime-info.js'
import { RuntimeToolsResponse as RuntimeToolsResponseSchema } from '../../../packages/runtime/src/contracts/runtime-tools.js'
import {
  buildDevGoToolchainEnvV1,
  buildDesktopPrivateHistoryMigrationArgsV2,
  buildGoRuntimeStartupUserDataArgsV1,
  buildDesktopPrivateHistoryMigrationEnvV2,
  buildGoRuntimeProviderArgs,
  buildGoRuntimeSidecarEnv,
  DESKTOP_PRIVATE_HISTORY_MIGRATION_TIMEOUT_MS_V2,
  ensureGoDefaultBackend,
  getAnalytixRuntimeBackendStatus,
  GO_RUNTIME_SERVER_STARTUP_TIMEOUT_MS_V1,
  isExactDesktopPrivateHistoryMigrationReadyV2,
  type GoRuntimeG6ReadinessStatus,
  observeGoSidecarExit,
  probeGoConformanceRuntimeCanary,
  parseGoRuntimeServerReadyPayload,
  resolveBundledGoRuntimeServerPath,
  resolveGoRuntimeConfiguredDurableRoot,
  resolveGoRuntimeLaunchTarget,
  resolveGoRuntimeMCPConfigPath,
  resolveManagedGoRuntimeToken,
  resolveReportedActiveBackend,
  resolveGoRuntimeG6ReadinessStatus,
  resolveAnalytixRuntimeBackendGate,
  sanitizeRuntimeResponse,
  sanitizeRuntimeResponseBody,
  settleOptionalRuntimeCapability,
  retryWithoutOptionalCaseAuthorityV1,
  setGoRuntimeUnexpectedExitHandler,
  shouldRetryWithoutOptionalCaseAuthorityV1,
  takeMainOwnedRuntimeAuthorityForLaunchV1,
  takeExactGoRuntimeReadyLine,
  verifyWitnessedAuthorityStartupIdentity,
  waitForDesktopPrivateHistoryMigrationV2,
  runtimeRequestViaHost
} from './analytix-adapter'
import type {
  MainOwnedRuntimeAuthorityEnvelopeV1
} from './main-owned-authority-envelope-v1'

function mainOwnedAuthorityFixtureV1(): MainOwnedRuntimeAuthorityEnvelopeV1 {
  const publicKey = Buffer.from(Array.from({ length: 32 }, (_, index) => index + 1))
  const authorityKeyId = createHash('sha256').update(publicKey).digest('hex')
  const authorityPublicKey = publicKey.toString('base64url')
  publicKey.fill(0)
  return Object.freeze({
    schemaVersion: 1,
    purpose: 'analytix.runtime-main-owned-authority/v1',
    authorityAnchorV1: JSON.stringify({
      schemaVersion: 1,
      installationId: '1'.repeat(64),
      authorityKeyId,
      authorityPublicKey,
      currentManifestDigest: '2'.repeat(64)
    }),
    authorityManifestRoot: '/private/authority/manifest',
    authorityCredentialProfileRoot: '/private/authority/profile',
    authorityCredentialBundleRoot: '/private/authority/bundle'
  })
}

describe('desktop private history startup migration', () => {
  it('settles an unavailable optional capability without blocking the Agent base', async () => {
    const unavailable: string[] = []
    await expect(settleOptionalRuntimeCapability(
      'bundled_funds',
      async () => {
        throw new Error('private materialization detail')
      },
      (capability) => unavailable.push(capability)
    )).resolves.toBeNull()
    expect(unavailable).toEqual(['bundled_funds'])

    await expect(settleOptionalRuntimeCapability(
      'case_authority',
      async () => {
        throw new Error('private authority detail')
      },
      () => {
        throw new Error('diagnostic sink unavailable')
      }
    )).resolves.toBeNull()

    await expect(settleOptionalRuntimeCapability(
      'bundled_funds',
      async () => 'ready',
      (capability) => unavailable.push(capability)
    )).resolves.toBe('ready')
    expect(unavailable).toEqual(['bundled_funds'])
  })

  it('degrades an unavailable case authority without blocking the general launch', async () => {
    let reports = 0
    await expect(takeMainOwnedRuntimeAuthorityForLaunchV1(
      async () => {
        throw new Error('private authority detail')
      },
      () => { reports += 1 }
    )).resolves.toBeNull()
    expect(reports).toBe(1)

    const expected = mainOwnedAuthorityFixtureV1()
    await expect(takeMainOwnedRuntimeAuthorityForLaunchV1(
      async () => expected,
      () => { reports += 1 }
    )).resolves.toBe(expected)
    expect(reports).toBe(1)

    await expect(takeMainOwnedRuntimeAuthorityForLaunchV1(
      async () => {
        throw new Error('private authority detail')
      },
      () => {
        throw new Error('diagnostic sink unavailable')
      }
    )).resolves.toBeNull()
  })

  it('retries one pre-ready authority activation failure without authority', async () => {
    expect(shouldRetryWithoutOptionalCaseAuthorityV1({
      runtimeServer: true,
      authorityAttempted: true,
      readyPayloadReceived: false,
      fallbackAlreadyUsed: false
    })).toBe(true)
    expect(shouldRetryWithoutOptionalCaseAuthorityV1({
      runtimeServer: true,
      authorityAttempted: true,
      readyPayloadReceived: false,
      fallbackAlreadyUsed: true
    })).toBe(false)
    expect(shouldRetryWithoutOptionalCaseAuthorityV1({
      runtimeServer: true,
      authorityAttempted: true,
      readyPayloadReceived: true,
      fallbackAlreadyUsed: false
    })).toBe(false)
    expect(shouldRetryWithoutOptionalCaseAuthorityV1({
      runtimeServer: false,
      authorityAttempted: true,
      readyPayloadReceived: false,
      fallbackAlreadyUsed: false
    })).toBe(false)

    const attempts: string[] = []
    await expect(retryWithoutOptionalCaseAuthorityV1(
      true,
      new Error('private activation detail'),
      async () => {
        attempts.push('general')
        return 'ready'
      },
      () => attempts.push('reported')
    )).resolves.toBe('ready')
    expect(attempts).toEqual(['reported', 'general'])
    await expect(retryWithoutOptionalCaseAuthorityV1(
      true,
      new Error('private activation detail'),
      async () => 'ready-after-diagnostic-failure',
      () => {
        throw new Error('diagnostic sink unavailable')
      }
    )).resolves.toBe('ready-after-diagnostic-failure')

    await expect(retryWithoutOptionalCaseAuthorityV1(
      false,
      new Error('post-ready protocol failure'),
      async () => {
        attempts.push('must-not-run')
        return 'unexpected'
      }
    )).rejects.toThrow('post-ready protocol failure')
    expect(attempts).toEqual(['reported', 'general'])
  })

  it('owns the managed Go sidecar tree through startup, fallback cleanup, and stop', () => {
    const source = readFileSync(new URL('./analytix-adapter.ts', import.meta.url), 'utf8')
    const startOffset = source.indexOf('async function startGoConformanceSidecarOnce')
    const stopOffset = source.indexOf('async function stopGoConformanceSidecarOnce')
    const stopEndOffset = source.indexOf('async function fetchTextWithTimeout', stopOffset)
    expect(startOffset).toBeGreaterThanOrEqual(0)
    expect(stopOffset).toBeGreaterThan(startOffset)
    expect(stopEndOffset).toBeGreaterThan(stopOffset)

    const startBody = source.slice(startOffset, stopOffset)
    const spawnOffset = startBody.indexOf('const child = spawn(')
    const ownOffset = startBody.indexOf('const ownedProcess = ownSpawnedProcess(')
    const publishOffset = startBody.indexOf('goSidecarOwnedProcess = ownedProcess')
    const cleanupOffset = startBody.lastIndexOf('await stopGoConformanceSidecarAndWait()')
    const fallbackOffset = startBody.lastIndexOf('return retryWithoutOptionalCaseAuthorityV1(')
    expect(startBody).toContain("detached: process.platform !== 'win32'")
    expect(spawnOffset).toBeGreaterThanOrEqual(0)
    expect(ownOffset).toBeGreaterThan(spawnOffset)
    expect(publishOffset).toBeGreaterThan(ownOffset)
    expect(cleanupOffset).toBeGreaterThan(publishOffset)
    expect(fallbackOffset).toBeGreaterThan(cleanupOffset)

    const stopBody = source.slice(stopOffset, stopEndOffset)
    const terminateOffset = stopBody.indexOf('await stopOwnedProcess(current)')
    const releaseOffset = stopBody.indexOf('goSidecarOwnedProcess = null')
    expect(terminateOffset).toBeGreaterThanOrEqual(0)
    expect(releaseOffset).toBeGreaterThan(terminateOffset)
    expect(stopBody).not.toContain("current.kill('SIGTERM')")
    expect(stopBody).not.toContain("current.kill('SIGKILL')")
  })

  it('uses a dedicated bounded deadline for full desktop histories', () => {
    expect(DESKTOP_PRIVATE_HISTORY_MIGRATION_TIMEOUT_MS_V2).toBe(30 * 60_000)
    expect(GO_RUNTIME_SERVER_STARTUP_TIMEOUT_MS_V1).toBe(
      DESKTOP_PRIVATE_HISTORY_MIGRATION_TIMEOUT_MS_V2
    )
  })

  it('keeps admitted compiler caches out of the isolated application home', () => {
    const { DEVELOPMENT_CACHE_ROOT, DEVELOPMENT_CACHE_ENVIRONMENT } = require('../../../scripts/lib/development-cache-environment.cjs')
    const input = { HOME: '/synthetic/profile', PATH: '/safe-path', ANALYTIX_DEV_CACHE_ROOT: DEVELOPMENT_CACHE_ROOT,
      ...DEVELOPMENT_CACHE_ENVIRONMENT, OPENAI_API_KEY: 'synthetic-not-a-key' }
    const projected = buildDevGoToolchainEnvV1(input)
    for (const key of ['GOCACHE', 'GOMODCACHE', 'GOTMPDIR']) expect(projected[key]).toBe(DEVELOPMENT_CACHE_ENVIRONMENT[key])
    expect(projected.HOME).toBe('/synthetic/profile')
    expect(projected.OPENAI_API_KEY).toBeUndefined()
    expect(() => buildDevGoToolchainEnvV1({ ...input, GOMODCACHE: '/synthetic/other-cache' })).toThrow(/mismatch/)
    expect(() => buildDevGoToolchainEnvV1({ ...input, ANALYTIX_DEV_CACHE_ROOT: '/synthetic/other-root' })).toThrow(/not authoritative/)
  })

  it('gives Go discovery and development builds a credential-free fixed environment', () => {
    const env = buildDevGoToolchainEnvV1({
      HOME: '/safe-home',
      PATH: '/safe-path',
      GOROOT: '/private/untrusted-go-root',
      GOMODCACHE: '/private/untrusted-module-cache',
      ANALYTIX_API_KEY: 'provider-secret',
      ANALYTIX_RUNTIME_TOKEN: 'runtime-secret',
      ANALYTIX_MCP_CONFIG_PATH: '/private/mcp.json',
      OPENAI_API_KEY: 'ambient-secret',
      AWS_SECRET_ACCESS_KEY: 'cloud-secret',
      GIT_ASKPASS: '/private/credential-helper',
      SSH_AUTH_SOCK: '/private/ssh-agent',
      HTTP_PROXY: 'http://proxy.invalid',
      HTTPS_PROXY: 'http://proxy.invalid',
      GOFLAGS: '-toolexec=/private/untrusted-tool',
      GOPROXY: 'https://token@example.invalid',
      GOWORK: '/private/go.work'
    })
    expect(env).toMatchObject({
      HOME: '/safe-home',
      PATH: '/safe-path',
      CGO_ENABLED: '0',
      GOENV: 'off',
      GOPROXY: 'https://proxy.golang.org',
      GOSUMDB: 'sum.golang.org',
      GOTOOLCHAIN: 'local',
      GOVCS: 'public:git|hg,private:off',
      GOWORK: 'off'
    })
    for (const name of [
      'ANALYTIX_API_KEY',
      'ANALYTIX_RUNTIME_TOKEN',
      'ANALYTIX_MCP_CONFIG_PATH',
      'OPENAI_API_KEY',
      'AWS_SECRET_ACCESS_KEY',
      'GIT_ASKPASS',
      'SSH_AUTH_SOCK',
      'HTTP_PROXY',
      'HTTPS_PROXY',
      'GOFLAGS',
      'GOROOT',
      'GOMODCACHE'
    ]) {
      expect(env[name]).toBeUndefined()
    }
  })

  it('uses only the production Go runtime migration command and the ordinary durable root', () => {
    const dataDir = join(tmpdir(), 'analytix-migration-data')
    const userDataDir = join(tmpdir(), 'analytix-electron-user-data')
    expect(buildDesktopPrivateHistoryMigrationArgsV2(dataDir, userDataDir)).toEqual([
      'migration',
      'migrate-desktop-private-history-v2',
      '--data-dir',
      dataDir,
      '--durable-root',
      dataDir,
      '--user-data-dir',
      userDataDir
    ])
  })

  it('binds ordinary runtime startup to the same exact Electron userData root', () => {
    const userDataDir = join(tmpdir(), 'analytix-electron-user-data')
    expect(buildGoRuntimeStartupUserDataArgsV1(userDataDir)).toEqual([
      '--user-data-dir',
      userDataDir
    ])
  })

  it('does not forward provider, MCP, token, proxy, or credential environment', () => {
    const env = buildDesktopPrivateHistoryMigrationEnvV2({
      HOME: '/safe-home',
      PATH: '/safe-path',
      ANALYTIX_API_KEY: 'provider-secret',
      ANALYTIX_RUNTIME_TOKEN: 'runtime-secret',
      ANALYTIX_MCP_CONFIG_PATH: '/private/mcp.json',
      ANALYTIX_MODEL_PROVIDERS: '{"secret":true}',
      OPENAI_API_KEY: 'ambient-secret',
      HTTPS_PROXY: 'http://proxy.invalid'
    })
    expect(env.HOME).toBe('/safe-home')
    expect(env.PATH).toBe('/safe-path')
    expect(env.ANALYTIX_APP_ROOT).toBeTruthy()
    expect(env.ANALYTIX_RESOURCES_PATH).toBeTruthy()
    for (const name of [
      'ANALYTIX_API_KEY',
      'ANALYTIX_RUNTIME_TOKEN',
      'ANALYTIX_MCP_CONFIG_PATH',
      'ANALYTIX_MODEL_PROVIDERS',
      'OPENAI_API_KEY',
      'HTTPS_PROXY'
    ]) {
      expect(env[name]).toBeUndefined()
    }
  })

  it('accepts exactly one fixed stdout marker and no payload', () => {
    const marker = Buffer.from('ANALYTIX_DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2\n')
    expect(isExactDesktopPrivateHistoryMigrationReadyV2(marker)).toBe(true)
    expect(isExactDesktopPrivateHistoryMigrationReadyV2(Buffer.from(marker.subarray(0, -1)))).toBe(false)
    expect(isExactDesktopPrivateHistoryMigrationReadyV2(Buffer.concat([marker, Buffer.from('extra\n')]))).toBe(false)
    expect(isExactDesktopPrivateHistoryMigrationReadyV2(Buffer.from('noise\n'))).toBe(false)
  })

  it('waits for close and rejects bytes arriving after exit', async () => {
    const child = fakeMigrationChild()
    const outcome = waitForDesktopPrivateHistoryMigrationV2(child.process, 1_000).then(
      () => 'resolved',
      () => 'rejected'
    )
    child.stdout.write('ANALYTIX_DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2\n')
    child.process.emit('exit', 0, null)
    let settled = false
    outcome.then(() => { settled = true })
    await new Promise((resolve) => setImmediate(resolve))
    expect(settled).toBe(false)
    child.stdout.write('late-extra-byte')
    child.process.emit('close', 0, null)
    expect(await outcome).toBe('rejected')
  })

  it('accepts the exact marker only after stdio close', async () => {
    const child = fakeMigrationChild()
    const outcome = waitForDesktopPrivateHistoryMigrationV2(child.process, 1_000)
    child.stdout.write('ANALYTIX_DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2\n')
    child.process.emit('exit', 0, null)
    child.process.emit('close', 0, null)
    await expect(outcome).resolves.toBeUndefined()
    expect(child.killCalls).toBe(0)
  })

  it('keeps the barrier closed until close when termination cannot be confirmed', async () => {
    const child = fakeMigrationChild(false)
    const outcome = waitForDesktopPrivateHistoryMigrationV2(child.process, 1, 1_000).then(
      () => 'resolved',
      () => 'rejected'
    )
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(child.killCalls).toBe(1)
    let settled = false
    outcome.then(() => { settled = true })
    await new Promise((resolve) => setImmediate(resolve))
    expect(settled).toBe(false)
    child.process.emit('close', null, 'SIGKILL')
    expect(await outcome).toBe('rejected')
  })

  it('rejects after a bounded termination grace when close never arrives', async () => {
    const child = fakeMigrationChild(false)
    const outcome = waitForDesktopPrivateHistoryMigrationV2(child.process, 1, 5)
    await expect(outcome).rejects.toThrow('termination could not be confirmed')
    expect(child.killCalls).toBe(1)
  })
})

describe('Go runtime ready payload', () => {
  it('accepts only the closed PID-bound V2-aware shape', () => {
    const authorityPublicKey = Buffer.alloc(32, 0x42).toString('base64url')
    const disabled = {
      url: 'http://127.0.0.1:45123/',
      runtimePid: 4242,
      runtimeTokenConfigured: true,
      persistenceRootsConfigured: true,
      productionRuntime: true,
      controlledArtifactHostV2Configured: false,
      controlledArtifactHostV2Ready: false,
      controlledArtifactHostV2BackendGeneration: 0,
      controlledArtifactHostV2LaunchBindingProof: '',
      datasetSnapshotSelectionV2Configured: false,
      datasetSnapshotAdmissionV2State: 'absent',
      datasetSnapshotAdmissionV2InstallationId: '',
      datasetSnapshotAdmissionV2RuntimeLaunchNonce: '',
      datasetSnapshotAdmissionV2StagingBindingDigest: '',
      datasetSnapshotAdmissionV2SelectionDigest: '',
      datasetSnapshotAdmissionV2SnapshotId: '',
      datasetSnapshotAdmissionV2AuthorityRecordDigest: '',
      datasetSnapshotAdmissionV2AckHmacSha256: '',
      finalPublicationAuthorityKeyId: createHash('sha256').update(Buffer.from(authorityPublicKey, 'base64url')).digest('hex'),
      finalPublicationAuthorityPublicKey: authorityPublicKey,
      witnessedAuthorityV2Configured: false,
      witnessedAuthorityInstallationId: '',
      witnessedAuthorityKeyId: '',
      witnessedAuthorityManifestDigest: ''
    }
    expect(parseGoRuntimeServerReadyPayload(JSON.stringify(disabled))).toEqual(disabled)
    expect(parseGoRuntimeServerReadyPayload(JSON.stringify({
      ...disabled,
      controlledArtifactHostV2Configured: true,
      controlledArtifactHostV2Ready: true,
      controlledArtifactHostV2BackendGeneration: 17,
      controlledArtifactHostV2LaunchBindingProof: 'a'.repeat(64)
    }))).toMatchObject({
      controlledArtifactHostV2Configured: true,
      controlledArtifactHostV2BackendGeneration: 17
    })
    expect(parseGoRuntimeServerReadyPayload(JSON.stringify({
      ...disabled,
      witnessedAuthorityV2Configured: true,
      witnessedAuthorityInstallationId: '1'.repeat(64),
      witnessedAuthorityKeyId: '2'.repeat(64),
      witnessedAuthorityManifestDigest: '3'.repeat(64)
    }))).toMatchObject({
      witnessedAuthorityV2Configured: true,
      witnessedAuthorityInstallationId: '1'.repeat(64)
    })
    expect(() => parseGoRuntimeServerReadyPayload(JSON.stringify({
      url: disabled.url,
      runtimeTokenConfigured: true,
      persistenceRootsConfigured: true,
      productionRuntime: true
    }))).toThrow('invalid ready payload')
    expect(() => parseGoRuntimeServerReadyPayload(JSON.stringify({ ...disabled, extra: true })))
      .toThrow('invalid ready payload')
    expect(() => parseGoRuntimeServerReadyPayload(JSON.stringify({
      ...disabled,
      finalPublicationAuthorityKeyId: '0'.repeat(64)
    }))).toThrow('invalid ready payload')
    expect(() => parseGoRuntimeServerReadyPayload(JSON.stringify({
      ...disabled,
      witnessedAuthorityV2Configured: true
    }))).toThrow('invalid ready payload')
    expect(() => parseGoRuntimeServerReadyPayload(JSON.stringify({
      ...disabled,
      datasetSnapshotSelectionV2Configured: true
    }))).toThrow('invalid ready payload')
    expect(() => parseGoRuntimeServerReadyPayload(
      '{"url":"http://127.0.0.1:45123/","url":"http://127.0.0.1:45124/"}'
    )).toThrow()
  })

  it('requires the ready witness to match the exact authority sent by main', () => {
    const authority = mainOwnedAuthorityFixtureV1()
    const anchor = JSON.parse(authority.authorityAnchorV1) as {
      installationId: string
      authorityKeyId: string
      currentManifestDigest: string
    }
    const configured = {
      witnessedAuthorityV2Configured: true,
      witnessedAuthorityInstallationId: anchor.installationId,
      witnessedAuthorityKeyId: anchor.authorityKeyId,
      witnessedAuthorityManifestDigest: anchor.currentManifestDigest
    }
    expect(() => verifyWitnessedAuthorityStartupIdentity(configured, {
      ...anchor,
      authorityPublicKey: ''
    })).not.toThrow()
    expect(() => verifyWitnessedAuthorityStartupIdentity(configured, null))
      .toThrow('does not match the main-owned startup frame')
    expect(() => verifyWitnessedAuthorityStartupIdentity({
      ...configured,
      witnessedAuthorityManifestDigest: '9'.repeat(64)
    }, {
      ...anchor,
      authorityPublicKey: ''
    })).toThrow('does not match the main-owned startup frame')
    expect(() => verifyWitnessedAuthorityStartupIdentity({
      witnessedAuthorityV2Configured: false,
      witnessedAuthorityInstallationId: '',
      witnessedAuthorityKeyId: '',
      witnessedAuthorityManifestDigest: ''
    }, null)).not.toThrow()
  })

  it('accepts only one bounded first-line ready marker without stdout noise', () => {
    const prefix = 'ANALYTIX_RUNTIME_SERVER_READY '
    const line = `${prefix}{"url":"http://127.0.0.1:45123/"}`
    expect(takeExactGoRuntimeReadyLine(Buffer.from(line), prefix)).toBeNull()
    expect(takeExactGoRuntimeReadyLine(Buffer.from(`${line}\n`), prefix)).toBe(line)
    for (const invalid of [
      Buffer.from(`noise\n${line}\n`),
      Buffer.from(`${line}\nextra\n`),
      Buffer.from(`${line}\r\n`),
      Buffer.concat([Buffer.from(prefix), Buffer.from([0xff]), Buffer.from('\n')]),
      Buffer.alloc((64 << 10) + 1, 0x61)
    ]) {
      expect(() => takeExactGoRuntimeReadyLine(invalid, prefix)).toThrow('ready output is invalid')
    }
  })
})

let server: Server | null = null
const TEST_COMMIT = 'a'.repeat(40)
const RUNTIME_GO_OPERATOR_ENV_KEYS = [
  'ANALYTIX_RUNTIME_READY',
  'ANALYTIX_GO_RUNTIME_G6_READY',
  'ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT',
  'ANALYTIX_RUNTIME_READY_EVIDENCE',
  'ANALYTIX_GO_RUNTIME_G6_READY_EVIDENCE',
  'ANALYTIX_RUNTIME_GO_OPERATOR_GATE_EVIDENCE'
]

function envWithoutRuntimeGoOperatorGate(): NodeJS.ProcessEnv {
  const env = { ...process.env }
  for (const key of RUNTIME_GO_OPERATOR_ENV_KEYS) {
    delete env[key]
  }
  return env
}

function firstHeader(value: string | string[] | undefined): string {
  return Array.isArray(value) ? value[0] ?? '' : value ?? ''
}

function writeExecutable(path: string, source: string): void {
  writeFileSync(path, source, { encoding: 'utf8', mode: 0o755 })
}

function settingsForPort(port: number): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: {
        ...defaultAnalytixRuntimeSettings(port),
        runtimeToken: 'usage-token'
      },
    workspaceRoot: '/tmp',
    log: { enabled: true, retentionDays: 7 },
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

type RequestHandler = (req: IncomingMessage, res: ServerResponse) => void

function listen(handler: RequestHandler): Promise<number> {
  server = createServer(handler)
  return new Promise((resolve, reject) => {
    server?.once('error', reject)
    server?.listen(0, '127.0.0.1', () => {
      const address = server?.address() as AddressInfo
      resolve(address.port)
    })
  })
}

function fakeChildProcess(): ChildProcess {
  return Object.assign(new EventEmitter(), {
    exitCode: null,
    signalCode: null
  }) as ChildProcess
}

function fakeMigrationChild(killResult = true): {
  process: ChildProcess
  stdout: PassThrough
  stderr: PassThrough
  readonly killCalls: number
} {
  const stdout = new PassThrough()
  const stderr = new PassThrough()
  let killCalls = 0
  const process = Object.assign(new EventEmitter(), {
    exitCode: null,
    signalCode: null,
    stdout,
    stderr,
    kill: () => {
      killCalls += 1
      return killResult
    }
  }) as unknown as ChildProcess
  return {
    process,
    stdout,
    stderr,
    get killCalls() { return killCalls }
  }
}

afterEach(async () => {
  setGoRuntimeUnexpectedExitHandler(null)
  const current = server
  server = null
  if (!current) return
  await new Promise<void>((resolve, reject) => {
    current.close((error) => {
      if (error) reject(error)
      else resolve()
    })
  })
})

function passedG6Readiness(): GoRuntimeG6ReadinessStatus {
  return {
    schemaVersion: 1,
    ready: true,
    explicitReadyGate: true,
    durableRestartEvidence: { status: 'passed', required: true, evidence: 'runtime-restart-drill' },
    providerMatrix: { status: 'passed', required: true, evidence: 'runtime-go-provider-matrix' },
    mcpMatrix: { status: 'passed', required: true, evidence: 'runtime-go-mcp-execution' },
    packagedQa: { status: 'passed', required: true, evidence: 'runtime-go-packaged-qa' },
    operatorGate: { status: 'passed', required: true, evidence: 'runtime-go-operator-gate' },
    missingRequiredChecks: [],
    defaultGoBackendEnabled: true,
    rendererVisibleGoSwitcher: false
  }
}

function publicCapabilityState(enabled: boolean, available: boolean) {
  return available
    ? { status: 'available', enabled: true, available: true, reasonCode: 'available' }
    : enabled
      ? { status: 'unavailable', enabled: true, available: false, reasonCode: 'unavailable' }
      : { status: 'disabled', enabled: false, available: false, reasonCode: 'disabled_by_config' }
}

function publicRuntimeInfoFixture(port: number): Record<string, unknown> {
  const available = publicCapabilityState(true, true)
  const disabled = publicCapabilityState(false, false)
  const mcpSearch = {
    enabled: false,
    mode: 'auto',
    active: false,
    available: false,
    reasonCode: 'disabled_by_config',
    indexedToolCount: 0,
    advertisedToolCount: 0,
    autoThresholdToolCount: 24,
    topKDefault: 8,
    topKMax: 24,
    minScore: 0,
    catalogDrift: false
  }
  return {
    schemaVersion: 2,
    status: 'ready',
    listenerScope: 'loopback',
    port,
    startedAt: '2026-06-23T00:00:00.000Z',
    insecure: false,
    storage: { configured: true, available: true },
    executionPolicy: { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' },
    provider: {
      id: 'deepseek',
      model: 'deepseek-chat',
      family: 'deepseek',
      endpointFormat: 'chat_completions',
      available: true,
      apiKeyConfigured: true,
      baseUrlConfigured: true,
      cacheTelemetrySupported: true,
      supportsImageInput: false,
      reasoningEffort: 'high'
    },
    networkProxy: { mode: 'off', configured: false, source: 'unknown', valid: true, credentialsMasked: true },
    capabilities: {
      contractVersion: 1,
      model: {
        id: 'deepseek-chat',
        providerId: 'deepseek',
        family: 'deepseek',
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        supportsImageInput: false,
        messageParts: ['text'],
        reasoning: {
          supportedEfforts: ['off', 'high', 'max'],
          defaultEffort: 'high',
          requestProtocol: 'deepseek-chat-completions'
        },
        endpointFormat: 'chat_completions'
      },
      cli: { serve: available, run: available, chat: available, exec: available },
      mcp: {
        ...disabled,
        configuredServers: 0,
        connectedServers: 0,
        toolCount: 0,
        promptCount: 0,
        resourceCount: 0,
        catalog: {
          status: 'disabled',
          reasonCode: 'disabled_by_config',
          toolCount: 0,
          advertisedToolCount: 0,
          promptCount: 0,
          resourceCount: 0,
          catalogDrift: false
        },
        search: mcpSearch
      },
      web: { ...disabled, fetch: disabled, search: disabled, provider: 'none', fetchEnabled: false, searchEnabled: false },
      skills: { ...available, configuredRoots: 1, discoveredSkills: 1 },
      subagents: {
        ...disabled,
        maxParallel: 0,
        maxChildRuns: 0,
        defaultToolPolicy: 'readOnly',
        profileCount: 0,
        internalLineageAvailable: false,
        profilesAvailable: false,
        durableChildRunStore: false,
        parallelExecutionAvailable: false,
        taskToolAvailable: false,
        parallelTasksToolAvailable: false,
        backgroundTaskJobsAvailable: false,
        backgroundShellAvailable: false,
        backgroundSubagentJobsAvailable: false,
        modelJobToolsAvailable: false,
        taskJobThreadScopeSupported: false
      },
      attachments: {
        ...available,
        maxImageBytes: 1,
        maxImageDimension: 1,
        allowedMimeTypes: ['image/png'],
        allowedDocumentMimeTypes: ['text/plain'],
        maxDocumentBytes: 1,
        maxDocumentTextChars: 1,
        textFallbackMaxBase64Bytes: 1,
        textFallbackMaxImageDimension: 1,
        textFallbackPreferredMimeType: 'image/png'
      },
      memory: { ...available, mode: 'manual', storeOnly: true, modelInjection: false, automaticCapture: false, scopes: [], maxInjectedRecords: 0 },
      imageGen: disabled,
      speechGen: disabled,
      musicGen: disabled,
      videoGen: disabled,
      computerUse: { ...disabled, mode: 'off' },
      visionBridge: {
        ...disabled,
        mode: 'off',
        maxScreenshotsPerTurn: 0,
        maxImagesPerTurn: 0,
        maxImageBytes: 0,
        maxImageDimension: 0,
        semanticProbeStatus: 'unknown'
      }
    }
  }
}

function publicRuntimeToolsFixture(): Record<string, unknown> {
  const disabled = publicCapabilityState(false, false)
  return {
    schemaVersion: 2,
    providerCount: 1,
    toolContracts: { count: 0, catalogHash: 'a'.repeat(64) },
    mcpServers: [],
    mcpSearch: {
      enabled: false,
      mode: 'auto',
      active: false,
      available: false,
      reasonCode: 'disabled_by_config',
      indexedToolCount: 0,
      advertisedToolCount: 0,
      autoThresholdToolCount: 24,
      topKDefault: 8,
      topKMax: 24,
      minScore: 0,
      catalogDrift: false
    },
    mcpPromptCount: 0,
    mcpResourceCount: 0,
    commands: [],
    networkProxy: { mode: 'off', configured: false, source: 'unknown', valid: true, credentialsMasked: true },
    webProviderCount: 0,
    skills: { enabled: true, available: true, reasonCode: 'available', configuredRootCount: 1, skillCount: 1, validationErrorCount: 0 },
    attachments: {
      enabled: true,
      count: 0,
      totalBytes: 0,
      maxImageBytes: 1,
      maxImageDimension: 1,
      allowedMimeTypes: ['image/png'],
      allowedDocumentMimeTypes: ['text/plain'],
      maxDocumentBytes: 1,
      maxDocumentTextChars: 1
    },
    memory: { enabled: true, activeCount: 0, tombstoneCount: 0 },
    subagents: {
      ...disabled,
      active: 0,
      queued: 0,
      profileCount: 0,
      maxParallel: 0,
      maxChildRuns: 0,
      defaultToolPolicy: 'readOnly',
      internalLineageAvailable: false,
      parallelExecutionAvailable: false,
      taskToolAvailable: false,
      parallelTasksToolAvailable: false,
      backgroundTaskJobsAvailable: false,
      backgroundShellAvailable: false,
      backgroundSubagentJobsAvailable: false
    }
  }
}

function credentialedG6Env(evidence?: {
  provider: string
  mcp: string
  packaged: string
  operator: string
}): NodeJS.ProcessEnv {
  return {
    ANALYTIX_RUNTIME_READY: '1',
    ANALYTIX_RUNTIME_GO_CURRENT_COMMIT: TEST_COMMIT,
    ANALYTIX_TEST_ALLOW_CURRENT_COMMIT_OVERRIDE: '1',
    NODE_ENV: 'test',
    ...(evidence ? { ANALYTIX_RUNTIME_READY_EVIDENCE: evidence.operator } : {}),
    ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
    ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
    ...(evidence ? { ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE: evidence.provider } : {}),
    ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
    ...(evidence ? { ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE: evidence.mcp } : {}),
    ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
    ...(evidence ? { ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: evidence.packaged } : {}),
    ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'key',
    ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.invalid/v1',
    ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
    ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'key',
    ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.invalid/v1',
    ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible',
    ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY: 'key',
    ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL: 'https://anthropic.invalid',
    ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL: 'claude-compatible',
    ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY: 'key',
    ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL: 'https://custom.invalid/full/path',
    ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'custom-compatible',
    ANALYTIX_RUNTIME_MCP_COMMAND: 'node scripted-mcp.js'
  }
}

function canonicalJSONString(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map((item) => canonicalJSONString(item)).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSONString(item)}`).join(',')}}`
  }
  return JSON.stringify(value)
}

function sha256(value: unknown): string {
  return createHash('sha256').update(canonicalJSONString(value)).digest('hex')
}

function sha256Text(value: string): string {
  return createHash('sha256').update(value).digest('hex')
}

function canaryAcceptedFinalBatch(
  threadId: string,
  turnId: string,
  firstSeq: number,
  timestamp: string,
  terminalTurnId = turnId
): Record<string, unknown> {
  const text = 'Go runtime canary completed.'
  const publicView = {
    schemaVersion: 2,
    publicationState: 'accepted',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 1,
    datasetSnapshotId: 'snapshot-canary',
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: '8'.repeat(64),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: timestamp,
    acceptedAt: timestamp
  }
  const record = {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: 'a'.repeat(64),
    authorityPublicKey: 'A'.repeat(43),
    threadId,
    turnId,
    envelopeDigest: publicView.envelopeDigest,
    contextDigest: publicView.contextDigest,
    contextEpoch: publicView.contextEpoch,
    datasetSnapshotId: publicView.datasetSnapshotId,
    variant: publicView.variant,
    terminalReason: publicView.terminalReason,
    renderedTextSha256: sha256Text(text),
    registrySequence: 0,
    registryStateDigest: 'd'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest: 'e'.repeat(64),
    privateRecordDigest: 'f'.repeat(64),
    acceptedAt: timestamp,
    authoritySignature: 'A'.repeat(86),
    recordDigest: '9'.repeat(64)
  }
  const view = {
    schemaVersion: 3,
    acceptedFinalDigest: record.recordDigest,
    publicationState: 'accepted',
    variant: publicView.variant,
    terminalReason: publicView.terminalReason,
    blockerCode: publicView.blockerCode,
    coverageStatus: publicView.coverageStatus,
    checkedScopeDigest: publicView.checkedScopeDigest,
    missingScopeCount: publicView.missingScopeCount,
    claimCount: publicView.claimCount,
    claimTypes: publicView.claimTypes,
    receiptMetadata: publicView.receiptMetadata,
    noHitWording: publicView.noHitWording,
    acceptedAt: publicView.acceptedAt
  }
  const lastSeq = firstSeq + 2
  const item = {
    id: `item_${turnId}_assistant`,
    turnId,
    threadId,
    role: 'assistant',
    status: 'completed',
    createdAt: timestamp,
    finishedAt: timestamp,
    kind: 'assistant_text',
    text,
    acceptedFinalView: view
  }
  const events = [
    {
      kind: 'item_completed', seq: firstSeq, timestamp, threadId, turnId, itemId: item.id, item,
      acceptedFinalDigest: record.recordDigest, publicationCommitId: record.recordDigest,
      publicationEventId: '1'.repeat(64), publicationSlot: 'assistant-final',
      publicationPayloadDigest: '2'.repeat(64)
    },
    {
      kind: 'usage', seq: firstSeq + 1, timestamp, threadId, turnId,
      model: 'gpt-canary', providerId: 'openai-canary', endpointFormat: 'chat_completions',
      usage: {
        promptTokens: 1_000, completionTokens: 20, reasoningTokens: 0, totalTokens: 1_020,
        cachedTokens: 700, cacheHitTokens: 700, cacheMissTokens: 300, cacheHitRate: 0.7, turns: 1
      },
      cacheDiagnostics: {
        cacheTelemetrySupported: true, cacheTelemetryPresent: true,
        cacheTelemetrySource: 'provider_usage', cacheHitTokens: 700, cacheMissTokens: 300,
        cacheHitRate: 0.7
      },
      usageFinalStatus: 'completed', acceptedFinalDigest: record.recordDigest,
      publicationCommitId: record.recordDigest, publicationEventId: '3'.repeat(64),
      publicationSlot: 'usage', publicationPayloadDigest: '4'.repeat(64)
    },
    {
      kind: 'turn_completed', seq: lastSeq, timestamp, threadId, turnId: terminalTurnId,
      status: 'completed', terminalReason: record.terminalReason,
      acceptedFinalDigest: record.recordDigest, publicationCommitId: record.recordDigest,
      publicationEventId: '5'.repeat(64), publicationSlot: 'terminal',
      publicationPayloadDigest: '6'.repeat(64)
    }
  ]
  const batchId = '7'.repeat(64)
  const eventManifestDigest = '8'.repeat(64)
  return {
    schemaVersion: 2,
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    kind: 'accepted_final_batch',
    batchId,
    threadId,
    turnId,
    seq: lastSeq,
    firstSeq,
    lastSeq,
    timestamp,
    publicationCommitId: record.recordDigest,
    eventManifestDigest,
    publicationAuthority: {
      schemaVersion: 'accepted-final-delivery-seal.v1',
      purpose: 'analytix.accepted-final-delivery-seal/v1',
      sealId: '0'.repeat(64),
      threadId,
      turnId,
      publicationCommitId: record.recordDigest,
      acceptedFinalDispositionDigest: 'a'.repeat(64),
      terminalDispositionId: 'b'.repeat(64),
      eventManifestDigest,
      sequencedEventsDigest: 'c'.repeat(64),
      batchId,
      firstSeq,
      lastSeq,
      timestamp,
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: record.authorityKeyId,
      authorityPublicKey: record.authorityPublicKey,
      authoritySignature: record.authoritySignature
    },
    events
  }
}

function canaryGeneralTerminalBatch(
  threadId: string,
  turnId: string,
  firstSeq: number,
  timestamp: string
): Record<string, unknown> {
  const lastSeq = firstSeq + 2
  const events = [
    {
      kind: 'item_completed', seq: firstSeq, timestamp, threadId, turnId,
      itemId: `item_${turnId}_assistant`, item: {
        id: `item_${turnId}_assistant`, turnId, threadId, role: 'assistant', status: 'completed',
        createdAt: timestamp, finishedAt: timestamp, kind: 'assistant_text',
        text: '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。'
      }
    },
    {
      kind: 'usage', seq: firstSeq + 1, timestamp, threadId, turnId, model: 'gpt-canary',
      usage: {
        promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
        cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
        cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
        priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0, turns: 1
      },
      cacheDiagnostics: {}, usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: lastSeq, timestamp, threadId, turnId,
      status: 'completed', terminalReason: 'success'
    }
  ]
  return {
    schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch', batchDigest: 'a'.repeat(64), threadId, turnId,
    seq: lastSeq, firstSeq, lastSeq, timestamp, generalTerminalCommitId: 'b'.repeat(64),
    generalTerminalAuthorityKind: 'general_terminal_cas', generalTerminalAuthorityDigest: 'c'.repeat(64),
    eventManifestDigest: 'd'.repeat(64), projectedEventsDigest: 'e'.repeat(64),
    transportAuthority: 'host_batch_digest_v1', evidenceAuthority: false,
    citationAuthority: false, factAnswerAllowed: false, events,
    eventManifest: [
      { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
    ]
  }
}

function writeRuntimeEvidenceFiles(dir: string): {
  provider: string
  mcp: string
  packaged: string
  packagedGui: string
  packagedSoak: string
  operator: string
} {
  const provider = join(dir, 'provider-matrix.json')
  const mcp = join(dir, 'mcp-matrix.json')
  const packaged = join(dir, 'packaged-qa.json')
  const packagedGui = join(dir, 'packaged-gui-bridge-smoke.json')
  const packagedSoak = join(dir, 'packaged-session-soak.json')
  const operator = join(dir, 'operator-gate.json')
  const providerProbe = (
    id: string,
    endpointFamily: string,
    endpointFormat: string,
    requestUrl: string,
    urlPolicy: string,
    providerScoped: boolean
  ): Record<string, unknown> => ({
    id,
    status: 'passed',
    skipped: false,
    credentialed: true,
    endpointFamily,
    endpointFormat,
    requestUrl,
    requestShape: {
      method: 'POST',
      urlPolicy,
      requestUrl,
      endpointFamily,
      endpointFormat,
      credentialConfigured: true,
      credentialValueRecorded: false,
      streamRequested: true,
      messageCount: 1,
      inputConfigured: false,
      maxTokensField: 'max_tokens',
      deepSeekSpecificRequestFieldsPresent: false
    },
    requestShapeValidation: { status: 'passed', missing: [] },
    streamParsing: { status: 'passed' },
    usageParsing: { status: 'passed' },
    cacheTelemetry: {
      status: 'passed',
      providerScoped,
      deepSeekSpecificRequestFieldsPresent: false
    },
    errorHandling: {
      status: 'passed',
      httpStatus: 400,
      errorProbePassed: true,
      requestShape: {
        method: 'POST',
        sameRequestUrl: true,
        intentionallyInvalidBody: true,
        credentialValueRecorded: false,
        requestBodyRecorded: false
      },
      responseSummary: 'intentional provider error',
      rawResponseRecorded: false,
      credentialValueRecorded: false
    },
    redaction: {
      status: 'passed',
      credentialValueRecorded: false
    }
  })
  const providerEvidence = {
    schemaVersion: 1,
    id: 'runtime-go-provider-matrix',
    status: 'passed',
    passed: true,
    credentialedMatrixEnvGated: true,
    readsRealApiKeysByDefault: false,
    credentialSecretsRecorded: false,
    redaction: { status: 'passed', secretMaterialFound: false },
    credentialedProbes: [
      providerProbe(
        'deepseek',
        'openai-chat-completions',
        'openai-chat-completions',
        'https://deepseek.example/v1/chat/completions',
        'append-chat-completions',
        true
      ),
      providerProbe(
        'openai-compatible',
        'openai-chat-completions',
        'openai-chat-completions',
        'https://openai.example/v1/chat/completions',
        'append-chat-completions',
        false
      ),
      providerProbe(
        'anthropic-compatible',
        'anthropic-messages',
        'anthropic-messages',
        'https://anthropic.example/v1/messages',
        'append-anthropic-messages',
        false
      ),
      providerProbe(
        'custom-endpoint',
        'custom-full-endpoint',
        'custom-full-endpoint',
        'https://custom.example/full/path',
        'custom-full-endpoint-no-append',
        false
      )
    ]
  }
  const approvalEvidencePath = join(dir, 'approval-evidence.json')
  const approvalEvidenceBody = `${JSON.stringify({
    schemaVersion: 1,
    id: 'runtime-go-mcp-approval-user-input-evidence',
    status: 'passed',
    passed: true,
    approvalUserInput: true,
    rawValueRecorded: false,
    credentialSecretsRecorded: false
  }, null, 2)}\n`
  writeFileSync(approvalEvidencePath, approvalEvidenceBody, 'utf8')
  const mcpEvidence = {
    schemaVersion: 1,
    id: 'runtime-go-mcp-execution',
    status: 'passed',
    passed: true,
    topLevelMcpIndexerExposed: false,
    reasonixPublicProtocolUsed: false,
    credentialedExecution: true,
    connect: true,
    toolDiscoverySearch: true,
    toolCall: true,
    approvalUserInput: true,
    reconnect: true,
    redaction: true,
    credentialedProbes: [
      {
        id: 'credentialed-mcp',
        status: 'passed',
        skipped: false,
        credentialed: true,
        credentialedExecution: true,
        connect: true,
        toolDiscoverySearch: true,
        search: true,
        toolCall: true,
        approvalUserInput: true,
        reconnect: true,
        credentialRedaction: true,
        commandConfigured: true,
        toolNameConfigured: true,
        calledToolNameHash: sha256Text('contract-mcp-tool'),
        toolCatalogDigest: sha256Text(canonicalJSONString(['contract-mcp-tool'])),
        reconnectToolNameHash: sha256Text('contract-mcp-tool'),
        reconnectToolCatalogDigest: sha256Text(canonicalJSONString(['contract-mcp-tool'])),
        reconnectToolCount: 1,
        approvalUserInputEvidence: {
          status: 'passed',
          evidencePath: approvalEvidencePath,
          evidenceSha256: sha256Text(approvalEvidenceBody),
          evidenceDigestAlgorithm: 'sha256:file-bytes-v1',
          rawValueRecorded: false,
          credentialSecretsRecorded: false
        }
      }
    ]
  }
  const packagedEvidence = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-qa',
    stage: 'packaged-desktop-qa',
    status: 'passed',
    passed: true,
    goDefaultBackendEnabled: true,
    rendererVisibleGoSwitcher: false,
    typeScriptFallbackRetained: false,
    credentialSecretsRecorded: false,
    redaction: { status: 'passed', secretMaterialFound: false },
    requiredCheckIds: [
      'packaged-app-startup',
      'go-runtime-default-gate',
      'health',
      'runtime-info',
      'thread-list',
      'turn-create',
      'sse-replay',
      'typescript-retired-backend'
    ],
    checks: [
      { id: 'packaged-app-startup', status: 'passed' },
      { id: 'go-runtime-default-gate', status: 'passed' },
      { id: 'health', status: 'passed' },
      { id: 'runtime-info', status: 'passed' },
      { id: 'thread-list', status: 'passed' },
      { id: 'turn-create', status: 'passed' },
      { id: 'sse-replay', status: 'passed' },
      { id: 'typescript-retired-backend', status: 'passed' }
    ],
    startup: {
      appPathExists: true,
      launchCommandConfigured: true,
      launchCommandReferencesAppPath: true,
      launchCommandHash: '0'.repeat(64),
      exitCode: 0
    },
    runtime: {
      runtimeUrl: 'http://127.0.0.1:12345',
      runtimeTokenConfigured: true,
      healthOK: true,
      runtimeInfo: {
        candidateContract: true,
        hostLocalhost: true,
        dataDirConfigured: true,
        dataDirHash: '3'.repeat(64),
        mcpAvailable: false,
        mcpDiagnosticHonest: true,
        subagentsAvailable: true,
        subagentsDiagnosticHonest: true,
        productRuntimeInfoHidesUpstreamEvidence: true,
        reasonixUpstreamAbsorptionPresent: false,
        rawValueRecorded: false,
        infoShapeDigest: '4'.repeat(64)
      },
      threadIdHash: '1'.repeat(64),
      turnIdHash: '2'.repeat(64)
    },
    defaultRuntimeGate: {
      runtimeBackend: 'go-runtime-default',
      explicitRuntimeBackendOverrideEnvSet: false,
      goDefaultBackendEnabled: true
    },
    retiredBackendEvidence: {
      requestedBackend: 'typescript',
      code: 'retired_backend',
      activeBackendAfterRequest: 'go-runtime-default',
      defaultRuntimeStopped: false,
      tsStarted: false,
      runtimeHealthAfterRequest: true,
      defaultRuntimeLaunchCommandHash: '0'.repeat(64),
      preRequestRuntimeUrl: 'http://127.0.0.1:12345',
      postRequestRuntimeUrl: 'http://127.0.0.1:12345',
      verifiedAt: '2026-06-24T00:00:00.000Z',
      credentialSecretsRecorded: false,
      redaction: { status: 'passed', secretMaterialFound: false }
    }
  }
  const packagedGuiEvidence = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-gui-smoke',
    stage: 'packaged-gui-bridge-smoke',
    status: 'passed',
    passed: true,
    checks: [
      'packaged-app',
      'renderer-bridge',
      'settings-bridge',
      'runtime-restart',
      'runtime-health',
      'runtime-info'
    ].map((id) => ({ id, status: 'passed' })),
    app: {
      executableExists: true,
      runtimeServerBinaryExists: true,
      appAsarExists: true,
      bundledRuntimeGoSourcePresent: false
    },
    renderer: {
      apiPresent: true,
      legacyKun: false,
      legacyReasonix: false,
      settingsOk: true,
      settingsRuntimeTopLevel: true,
      restartOk: true,
      healthStatus: 200,
      healthService: 'analytix',
      runtimeInfoStatus: 200,
      runtimeInfoProductClean: true,
      runtimeInfoHasLegacyMcpLocalMarker: false,
      runtimeInfoHasLegacyAbsorptionMarker: false
    },
    redaction: {
      status: 'passed',
      secretMaterialFound: false
    }
  }
  const packagedSoakEvidence = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-session-soak',
    stage: 'packaged-session-soak',
    status: 'passed',
    passed: true,
    runtimeServerBinaryExists: true,
    turnsRequested: 3,
    turnsCompleted: 3,
    providerRequestCount: 3,
    providerRequestDigests: ['1'.repeat(64), '2'.repeat(64), '3'.repeat(64)],
    providerHistoryPairing: [
      { turn: 2, previousPromptSeen: true, previousAssistantSeen: true },
      { turn: 3, previousPromptSeen: true, previousAssistantSeen: true }
    ],
    durableRestart: {
      preRestartRuntimeUrl: 'http://127.0.0.1:12345',
      postRestartRuntimeUrl: 'http://127.0.0.1:23456',
      sameThreadReadableAfterRestart: true,
      turnCountAfterRestart: 3,
      latestTurnIdHash: '4'.repeat(64)
    },
    eventReplay: {
      beforeRestartContainsLatestTurn: true,
      afterRestartContainsLatestTurn: true,
      turnCompletedSeen: true,
      usageSeen: true,
      eventReplayDigest: '5'.repeat(64)
    },
    usage: {
      totalTokens: 42,
      costUsd: 0.01,
      costUnknown: false
    },
    forbiddenFixtureMarkersAbsent: true,
    credentialSecretsRecorded: false,
    rawValueRecorded: false,
    redaction: {
      status: 'passed',
      secretMaterialFound: false
    },
    failureChecks: []
  }
  const operatorEvidence = {
    schemaVersion: 1,
    id: 'runtime-go-operator-gate',
    status: 'passed',
    passed: true,
    operatorGate: 'ANALYTIX_RUNTIME_READY=1',
    explicitEnvGate: true,
    credentialedEvidenceReviewed: true,
    defaultRuntimeApproved: true,
    goDefaultApproved: true,
    typeScriptFallbackRetained: false,
    goDefaultBackendEnabled: true,
    reasonixEngineRuntimeOnly: true,
    evidenceTargetCommit: TEST_COMMIT,
    evidenceDigestAlgorithm: 'sha256:canonical-json-v1',
    evidenceDigests: {
      provider: sha256(providerEvidence),
      mcp: sha256(mcpEvidence),
      packaged: sha256(packagedEvidence),
      packagedGui: sha256(packagedGuiEvidence),
      packagedSoak: sha256(packagedSoakEvidence)
    },
    reportPaths: {
      provider,
      mcp,
      packaged,
      packagedGui,
      packagedSoak
    },
    evidenceReviewed: [
      { id: 'provider-matrix-credentialed', status: 'passed', path: provider },
      { id: 'mcp-matrix-credentialed', status: 'passed', path: mcp },
      { id: 'packaged-qa', status: 'passed', path: packaged },
      { id: 'packaged-gui-bridge-smoke', status: 'passed', path: packagedGui },
      { id: 'packaged-session-soak', status: 'passed', path: packagedSoak }
    ]
  }
  writeFileSync(provider, JSON.stringify(providerEvidence), 'utf8')
  writeFileSync(mcp, JSON.stringify(mcpEvidence), 'utf8')
  writeFileSync(packaged, JSON.stringify(packagedEvidence), 'utf8')
  writeFileSync(packagedGui, JSON.stringify(packagedGuiEvidence), 'utf8')
  writeFileSync(packagedSoak, JSON.stringify(packagedSoakEvidence), 'utf8')
  writeFileSync(operator, JSON.stringify(operatorEvidence), 'utf8')
  return { provider, mcp, packaged, packagedGui, packagedSoak, operator }
}

describe('runtimeRequestViaHost', () => {
  it('accepts the public rewind response and rejects its private authority turn extension', () => {
    const response = {
      threadId: 'thr_rewind',
      turnId: 'turn_removed',
      removedTurns: 1,
      remainingTurns: 2,
      removedTurnIds: ['turn_removed']
    }
    const path = '/v1/threads/thr_rewind/rewind'
    const accepted = sanitizeRuntimeResponse({
      ok: true, status: 200, body: JSON.stringify(response)
    }, path, null, 'POST')
    expect(accepted.ok).toBe(true)
    expect(accepted.status).toBe(200)
    expect(JSON.parse(accepted.body)).toEqual(response)

    const privateAuthorityTurn = 'turn_private_rewind_authority'
    const rejected = sanitizeRuntimeResponse({
      ok: true, status: 200,
      body: JSON.stringify({ ...response, authorityTurnId: privateAuthorityTurn })
    }, path, null, 'POST')
    expect(rejected).toEqual({
      ok: false, status: 502,
      body: JSON.stringify({
        code: 'runtime_response_schema_invalid',
        message: 'Runtime response failed schema validation.'
      })
    })
    expect(rejected.body).not.toContain(privateAuthorityTurn)
  })

  it('rejects a strict V3 generic accepted-final view without its verified delivery seal', () => {
    const text = 'public generic final'
    const threadId = 'thread-v3'
    const turnId = 'turn-v3'
    const acceptedAt = '2026-08-24T00:00:01Z'
    const view = {
      schemaVersion: 3,
      acceptedFinalDigest: 'a'.repeat(64),
      publicationState: 'accepted',
      variant: 'GeneralGuidanceAnswer',
      terminalReason: 'success',
      blockerCode: '',
      coverageStatus: 'guidance_only',
      checkedScopeDigest: '',
      missingScopeCount: 0,
      claimCount: 0,
      claimTypes: [],
      receiptMetadata: {
        projection: 'masked_metadata_only',
        count: 0,
        setDigest: 'c'.repeat(64),
        citations: []
      },
      noHitWording: '',
      acceptedAt
    }
    const item = {
      id: 'item-v3', turnId, threadId,
      role: 'assistant', status: 'completed', createdAt: '2026-08-24T00:00:00Z',
      finishedAt: view.acceptedAt, kind: 'assistant_text', text, acceptedFinalView: view
    }
    const body = sanitizeRuntimeResponseBody(JSON.stringify({ item }))

    expect(JSON.parse(body)).toEqual({})
    expect(body).not.toContain(view.acceptedFinalDigest)
    expect(body).not.toContain(view.receiptMetadata.setDigest)
    expect(body).not.toContain('"acceptedFinal":')
    expect(body).not.toContain('"contextDigest":')
    expect(body).not.toContain('"datasetSnapshotId":')

    const hostile = sanitizeRuntimeResponseBody(JSON.stringify({
      item: {
        ...item,
        acceptedFinalView: { ...view, contextDigest: 'd'.repeat(64) }
      }
    }))
    expect(JSON.parse(hostile)).toEqual({})
    const privateRecord = sanitizeRuntimeResponseBody(JSON.stringify({
      item: { ...item, acceptedFinal: { recordDigest: view.acceptedFinalDigest } }
    }))
    expect(JSON.parse(privateRecord)).toBeNull()
  })

  it('withholds detached accepted-final strong markers before HTTP fallback sanitization', () => {
    for (const marker of [
      'acceptedFinal',
      'factFinalWitnessAdmission',
      'publicationSnapshotProof',
      'publicationSnapshotProofDigest'
    ]) {
      const canary = `PRIVATE_${marker}_CANARY`
      const body = sanitizeRuntimeResponseBody(JSON.stringify({
        generic: { nested: { [marker]: canary } }
      }))
      expect(JSON.parse(body)).toBeNull()
      expect(body).not.toContain(canary)
      expect(body).not.toContain(marker)

      const debugBody = sanitizeRuntimeResponseBody(JSON.stringify({
        rounds: [{ id: 1, durationMs: 1, nested: { [marker]: canary } }]
      }), '/v1/debug/llm-rounds')
      expect(JSON.parse(debugBody)).toBeNull()
      expect(debugBody).not.toContain(canary)
      expect(debugBody).not.toContain(marker)

      const debugResponse = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify({
          rounds: [{ id: 1, durationMs: 1, nested: { [marker]: canary } }]
        })
      }, '/v1/debug/llm-rounds', null, 'GET')
      expect(debugResponse).toMatchObject({ ok: false, status: 502 })
      expect(debugResponse.body).not.toContain(canary)
      expect(debugResponse.body).not.toContain(marker)
    }

    const generic = {
      generic: {
        envelope: { state: 'closed' },
        registryHead: { state: 'healthy' },
        publicationIntent: { state: 'queued' },
        storeDigest: 'a'.repeat(64)
      }
    }
    expect(JSON.parse(sanitizeRuntimeResponseBody(JSON.stringify(generic)))).toEqual(generic)
  })

  it('withholds an entire assistant value containing private reasoning before Electron IPC', () => {
    const body = sanitizeRuntimeResponseBody(JSON.stringify({
      thread: {
        turns: [{
          items: [
            { kind: 'assistant_reasoning', text: 'PRIVATE_ITEM' },
            {
              role: 'assistant',
              kind: 'assistant_text',
              text: '公开<think>PRIVATE_TAG</think>结论',
              reasoning_content: 'PRIVATE_FIELD'
            },
            {
              role: 'user',
              kind: 'user_message',
              text: '字面量 <think>example</think>'
            }
          ]
        }]
      },
      usage: { reasoningTokens: 7 }
    }))

    expect(JSON.parse(body)).toEqual({
      thread: {
        turns: [{
          items: [
            {
              role: 'assistant',
              kind: 'assistant_text',
              text: ''
            },
            {
              role: 'user',
              kind: 'user_message',
              text: '字面量 <think>example</think>'
            }
          ]
        }]
      },
      usage: { reasoningTokens: 7 }
    })
    expect(body).not.toMatch(/PRIVATE_ITEM|PRIVATE_TAG|PRIVATE_FIELD|assistant_reasoning/)
  })

  it('withholds restricted evidence and source-exact bindings before Electron HTTP IPC', () => {
    const exactAccount = '0012-3456789012345678'
    const body = sanitizeRuntimeResponseBody(JSON.stringify({
      thread: {
        review: {
          output: {
            schemaVersion: 2,
            purpose: 'analytix.source-field-binding/v2',
            sourceExactValue: exactAccount,
            sourceExactValueSha256: 'a'.repeat(64),
            bindingDigest: 'b'.repeat(64)
          }
        }
      }
    }))

    expect(body).toBe('null')
    expect(body).not.toContain(exactAccount)
  })

  it('withholds raw tool-result payload and unknown root fields before HTTP IPC', () => {
    const sentinel = 'PRIVATE_HTTP_TOOL_RESULT_SENTINEL'
    const body = sanitizeRuntimeResponseBody(JSON.stringify({
      thread: {
        turns: [{
          items: [{
            id: 'item-tool-result',
            turnId: 'turn-tool-result',
            threadId: 'thread-tool-result',
            role: 'tool',
            status: 'completed',
            createdAt: '2026-07-14T00:00:00.000Z',
            finishedAt: '2026-07-14T00:00:01.000Z',
            kind: 'tool_result',
            toolName: 'mcp__hostile__query',
            callId: 'call-hostile',
            toolKind: 'tool_call',
            isError: false,
            summary: sentinel,
            dataUrl: `data:image/png;base64,${sentinel}`,
            output: {
              code: '6222020202020202020',
              content: sentinel,
              citations: [{ sourceId: sentinel }],
              attachments: [{ id: 'att-hostile', localFilePath: `/tmp/${sentinel}` }],
              previewUrl: `http://127.0.0.1:4173/${sentinel}`
            }
          }]
        }]
      }
    }))
    const parsed = JSON.parse(body) as { thread: { turns: Array<{ items: Array<Record<string, unknown>> }> } }
    const item = parsed.thread.turns[0]?.items[0]
    expect(item).toMatchObject({
      kind: 'tool_result',
      output: {
        projectionKind: 'withheld',
        messageKey: 'legacy_output_withheld',
        privatePayloadWithheld: true,
        factAnswerAllowed: false,
        evidenceAuthority: false
      }
    })
    expect(item).not.toHaveProperty('summary')
    expect(item).not.toHaveProperty('dataUrl')
    expect(body).not.toContain(sentinel)
    expect(body).not.toContain('6222020202020202020')
  })

  it('projects LLM debug responses to hashes, counts, timing, and numeric usage only', () => {
    const body = sanitizeRuntimeResponseBody(JSON.stringify({
      rounds: [{
        id: 3,
        threadId: 'thread-1',
        turnId: 'turn-1',
        provider: 'deepseek',
        model: 'deepseek-chat',
        url: 'https://example.invalid/v1/chat?token=PRIVATE_URL',
        startedAt: '2026-07-10T00:00:00.000Z',
        finishedAt: '2026-07-10T00:00:01.000Z',
        durationMs: 1000,
        requestBody: { messages: [{ content: 'PRIVATE_REQUEST' }] },
        output: {
          text: 'PRIVATE_OUTPUT',
          reasoning: 'PRIVATE_REASONING',
          toolCalls: [{ callId: 'call-1', arguments: { account: 'PRIVATE_ACCOUNT' } }],
          usage: { promptTokens: 10, completionTokens: 2, raw: 'PRIVATE_USAGE' },
          stopReason: 'stop',
          error: null
        }
      }]
    }), '/v1/debug/llm-rounds')
    const parsed = JSON.parse(body) as { rounds: Array<Record<string, unknown>> }

    expect(parsed.rounds[0]).toEqual(expect.objectContaining({
      id: 3,
      durationMs: 1000,
      status: 'completed',
      toolCallCount: 1,
      usage: { promptTokens: 10, completionTokens: 2 },
      stopReason: 'stop',
      request: expect.objectContaining({ sha256: expect.stringMatching(/^[a-f0-9]{64}$/), bytes: expect.any(Number) }),
      response: expect.objectContaining({ sha256: expect.stringMatching(/^[a-f0-9]{64}$/), bytes: expect.any(Number) })
    }))
    expect(body).not.toMatch(/PRIVATE_URL|PRIVATE_REQUEST|PRIVATE_OUTPUT|PRIVATE_REASONING|PRIVATE_ACCOUNT|PRIVATE_USAGE/)
    expect(parsed.rounds[0]).not.toHaveProperty('url')
    expect(parsed.rounds[0]).not.toHaveProperty('threadId')
    expect(parsed.rounds[0]).not.toHaveProperty('turnId')
    expect(parsed.rounds[0]).not.toHaveProperty('provider')
    expect(parsed.rounds[0]).not.toHaveProperty('model')
    expect(parsed.rounds[0]).not.toHaveProperty('startedAt')
    expect(parsed.rounds[0]).not.toHaveProperty('finishedAt')
    expect(parsed.rounds[0]).not.toHaveProperty('requestBody')
    expect(parsed.rounds[0]).not.toHaveProperty('output')
  })

  it('maps marker-free LLM debug strings to closed public codes only', () => {
    const sentinel = 'MARKER_FREE_PROVIDER_ERROR_7F3C'
    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({
        rounds: [{
          id: 7,
          threadId: sentinel,
          turnId: sentinel,
          provider: sentinel,
          model: sentinel,
          durationMs: 12,
          output: {
            stopReason: sentinel,
            error: { code: sentinel, message: sentinel }
          }
        }]
      })
    }, '/v1/debug/llm-rounds', null, 'GET')

    expect(response.ok).toBe(true)
    expect(JSON.parse(response.body)).toEqual({
      rounds: [{
        id: 7,
        durationMs: 12,
        status: 'failed',
        response: expect.objectContaining({ sha256: expect.stringMatching(/^[a-f0-9]{64}$/), bytes: expect.any(Number) }),
        errorCode: 'provider_request_failed'
      }]
    })
    expect(response.body).not.toContain(sentinel)
  })

  it('fails closed for non-JSON runtime responses without forwarding their body', () => {
    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: 'PRIVATE_UNSTRUCTURED_PROVIDER_OUTPUT'
    }, '/v1/threads/thread-1')
    expect(response).toEqual({
      ok: false,
      status: 502,
      body: JSON.stringify({
        code: 'runtime_response_not_public',
        message: 'Runtime response was blocked at the public boundary.'
      })
    })
    expect(response.body).not.toContain('PRIVATE_UNSTRUCTURED_PROVIDER_OUTPUT')
  })

  it('withholds restricted non-JSON response bodies in the raw body sanitizer', () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const canonicalEvidenceV3 = JSON.stringify({
      canonicalEvidence: {
        schemaVersion: 3,
        purpose: 'analytix.canonical-evidence/v3',
        facts: []
      }
    })

    expect(sanitizeRuntimeResponseBody(`prefix ${authorityRef} suffix`)).toBe('')
    expect(sanitizeRuntimeResponseBody(`prefix ${canonicalEvidenceV3} suffix`)).toBe('')
    expect(sanitizeRuntimeResponseBody('ordinary plain runtime status')).toBe('ordinary plain runtime status')
  })

  it('fails closed on marker-free raw failure diagnostics in thread HTTP responses', () => {
    const sentinel = 'SOL_RAW_JOB_ERROR_SENTINEL_7F3C'
    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({
        id: 'thread-1',
        turns: [],
        child: { evidenceLedgerError: sentinel },
        details: { deliveryError: sentinel, autoContinueError: sentinel }
      })
    }, '/v1/threads/thread-1', null)

    expect(response).toEqual({
      ok: false,
      status: 502,
      body: JSON.stringify({
        code: 'runtime_response_not_public',
        message: 'Runtime response was blocked at the public boundary.'
      })
    })
    expect(response.body).not.toContain(sentinel)
  })

  it('requires an exact registered schema for every successful runtime response', () => {
    const thread = {
      id: 'thread-1',
      title: 'User-owned title',
      autoTitle: true,
      workspace: '~',
      model: 'model-1',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: '2026-07-20T00:00:00Z',
      updatedAt: '2026-07-20T00:00:00Z',
      turns: []
    }
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads', null, 'POST')
    expect(accepted.ok).toBe(true)

    const sentinel = 'MARKER_FREE_PROVIDER_ERROR_7F3C'
    for (const body of [
      { ...thread, diagnostics: { message: sentinel, exception: sentinel } },
      {
        ...thread,
        turns: [{
          id: 'turn-1',
          threadId: 'thread-1',
          status: 'running',
          prompt: 'User prompt',
          createdAt: '2026-07-20T00:00:00Z',
          items: [{
            id: 'item-1',
            turnId: 'turn-1',
            threadId: 'thread-1',
            role: 'tool',
            status: 'running',
            createdAt: '2026-07-20T00:00:00Z',
            kind: 'tool_progress',
            summary: sentinel,
            arguments: { command: sentinel, path: `/tmp/${sentinel}` }
          }]
        }]
      }
    ]) {
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(body)
      }, '/v1/threads/thread-1', null, 'GET')
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain(sentinel)
    }

    const unregistered = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ ok: true, message: sentinel })
    }, '/v1/unregistered-success', null, 'GET')
    expect(unregistered.ok).toBe(false)
    expect(unregistered.status).toBe(502)
    expect(unregistered.body).not.toContain(sentinel)
  })

  it('strictly admits Todo, manual compaction, and manual memory responses', () => {
    const todos = {
      threadId: 'thread-1',
      items: [
        {
          id: 'todo-manual',
          content: 'Manual task',
          status: 'pending',
          source: { kind: 'manual' },
          note: 'bounded note',
          createdAt: '2026-07-22T00:00:00Z',
          updatedAt: '2026-07-22T00:00:00Z'
        },
        {
          id: 'todo-plan',
          content: 'Plan task',
          status: 'in_progress',
          source: { kind: 'plan', planId: 'plan-1', relativePath: 'plan.md', ordinal: 0, contentHash: 'hash-1' },
          createdAt: '2026-07-22T00:00:00Z',
          updatedAt: '2026-07-22T00:00:00Z'
        },
        {
          id: 'todo-child',
          content: 'Child task',
          status: 'completed',
          source: { kind: 'child', childRunId: 'run-1', projectionId: 'projection-1' },
          createdAt: '2026-07-22T00:00:00Z',
          updatedAt: '2026-07-22T00:00:00Z'
        }
      ],
      updatedAt: '2026-07-22T00:00:00Z'
    }
    const liveMemory = {
      id: 'mem_go_1',
      content: 'Operator-managed record',
      scope: 'workspace',
      tags: [],
      confidence: 1,
      provenance: 'manual-general',
      captureMode: 'manual',
      modelInjection: false,
      createdAt: '2026-07-22T00:00:00Z',
      updatedAt: '2026-07-22T00:00:00Z'
    }
    const tombstone = {
      id: 'mem_go_2',
      createdAt: '2026-07-22T00:00:00Z',
      updatedAt: '2026-07-22T00:01:00Z',
      deletedAt: '2026-07-22T00:01:00Z'
    }
    const cases: Array<[string, string, unknown]> = [
      ['/v1/threads/thread-1/todos', 'GET', { todos }],
      ['/v1/threads/thread-1/todos', 'DELETE', { cleared: true }],
      ['/v1/threads/thread-1/compact', 'POST', {
        threadId: 'thread-1',
        replacedTokens: 12,
        summary: 'Prior conversation compacted.',
        pinnedConstraints: ['keep safety gate']
      }],
      ['/v1/memory', 'GET', { memories: [liveMemory, tombstone] }],
      ['/v1/memory', 'POST', { memory: liveMemory }],
      ['/v1/memory/mem_go_1', 'PATCH', { memory: liveMemory }],
      ['/v1/memory/mem_go_2', 'DELETE', { memory: tombstone }]
    ]
    for (const [path, method, body] of cases) {
      const accepted = sanitizeRuntimeResponse({ ok: true, status: 200, body: JSON.stringify(body) }, path, null, method)
      expect(accepted.ok, `${method} ${path}: ${accepted.body}`).toBe(true)
      expect(JSON.parse(accepted.body)).toEqual(body)

      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify({ ...(body as Record<string, unknown>), privateField: 'must-not-pass' })
      }, path, null, method)
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain('must-not-pass')
    }
  })

  it('preserves canonical Registry failures through the public projection and typed IPC', async () => {
    const failure = { schemaVersion: 1, error: { code: 'persistence_failure', message: 'The provider registry is temporarily unavailable.' } }
    const handler = createProviderRegistryIpcHandler(async (path, method) => sanitizeRuntimeResponse({
      ok: false, status: 503, body: JSON.stringify(failure)
    }, path, null, method))
    expect(await handler({ schemaVersion: 1, operation: 'list' })).toEqual(failure)
  })

  it('rejects noncanonical Registry errors without exposing their content', async () => {
    for (const body of [
      { schemaVersion: 1, error: { code: 'persistence_failure', message: 'SYNTHETIC_PRIVATE_ERROR' } },
      { schemaVersion: 1, error: { code: 'persistence_failure', message: 'The provider registry is temporarily unavailable.' }, secret: 'SYNTHETIC_PRIVATE_ERROR' }
    ]) {
      const handler = createProviderRegistryIpcHandler(async (path, method) => sanitizeRuntimeResponse({
        ok: false, status: 503, body: JSON.stringify(body)
      }, path, null, method))
      const result = await handler({ schemaVersion: 1, operation: 'list' })
      expect(result).toMatchObject({ error: { code: 'invalid_response' } })
      expect(JSON.stringify(result)).not.toContain('SYNTHETIC_PRIVATE_ERROR')
    }
  })

  it('admits the key-free Provider Registry snapshot at the ordinary runtime boundary', () => {
    const snapshot = {
      schemaVersion: 1,
      registryRevision: '0',
      registryIncarnation: `inc_${'a'.repeat(43)}`,
      providers: []
    }
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(snapshot)
    }, '/v1/provider-registry', null, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(snapshot)

    const provider = {
      id: 'deepseek',
      kind: 'openai-compatible',
      endpoint: 'http://127.0.0.1:47231/v1',
      models: ['deepseek-v4-flash'],
      mediaModels: [],
      selectedModel: 'deepseek-v4-flash',
      selectedRoutes: ['primary'],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '1',
      generation: '1',
      incarnation: `inc_${'b'.repeat(43)}`,
      tombstone: false
    }
    const providerResponse = {
      schemaVersion: 1,
      registryRevision: '1',
      registryIncarnation: snapshot.registryIncarnation,
      provider
    }
    for (const [path, method] of [
      ['/v1/provider-registry', 'POST'],
      ['/v1/provider-registry/providers/deepseek', 'GET'],
      ['/v1/provider-registry/providers/deepseek', 'PATCH'],
      ['/v1/provider-registry/providers/deepseek/select', 'POST'],
      ['/v1/provider-registry/providers/deepseek/disconnect', 'POST'],
      ['/v1/provider-registry/providers/deepseek/credential', 'PUT'],
      ['/v1/provider-registry/providers/deepseek/discover-models', 'POST']
    ] as const) {
      const projected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(providerResponse)
      }, path, null, method)
      expect(projected.ok, `${method} ${path}: ${projected.body}`).toBe(true)
      expect(JSON.parse(projected.body)).toEqual(providerResponse)
    }

    const privateSentinel = 'cred_PRIVATE_PROVIDER_RUNTIME_SENTINEL'
    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({
        ...providerResponse,
        provider: { ...provider, credentialRef: privateSentinel }
      })
    }, '/v1/provider-registry/providers/deepseek', null, 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain(privateSentinel)

    const wrongMethod = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(providerResponse)
    }, '/v1/provider-registry/providers/deepseek/select', null, 'PUT')
    expect(wrongMethod.ok).toBe(false)
    expect(wrongMethod.status).toBe(502)
  })

  it('atomically validates public diagnostics v2 and rejects old raw shapes', () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    for (const [path, value] of [
      ['/v1/runtime/info', publicRuntimeInfoFixture(1234)],
      ['/v1/runtime/tools', publicRuntimeToolsFixture()]
    ] as const) {
      const schema = path === '/v1/runtime/info' ? RuntimeInfoResponseSchema : RuntimeToolsResponseSchema
      const parsed = schema.safeParse(value)
      expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)
      const projected = sanitizePublicRuntimeValue(value)
      expect(projected, `${path}: public projection rejected a closed schema`).toBeDefined()
      expect(canonicalJSONString(projected), `${path}: public projection changed a closed schema`)
        .toBe(canonicalJSONString(value))
      const accepted = sanitizeRuntimeResponse({ ok: true, status: 200, body: JSON.stringify(value) }, path)
      expect(accepted.ok, `${path}: ${accepted.body}`).toBe(true)
      expect(JSON.parse(accepted.body)).toEqual(value)

      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify({ ...value, dataDir: privateSentinel, providers: [{ apiKey: privateSentinel }] })
      }, path)
      expect(rejected).toEqual({
        ok: false,
        status: 502,
        body: JSON.stringify({
          code: 'runtime_response_schema_invalid',
          message: 'Runtime response failed schema validation.'
        })
      })
      expect(rejected.body).not.toContain(privateSentinel)
    }
  })

  it('accepts the closed runtime skills projection and rejects private catalog fields', () => {
    const response = {
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 2,
      skillCount: 1,
      validationErrorCount: 1,
      skills: [{
        id: 'deep-review',
        name: 'Deep Review',
        scope: 'project',
        legacy: false
      }]
    }
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(response)
    }, '/v1/skills')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(response)

    for (const privateField of ['roots', 'validationErrors', 'reason']) {
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify({ ...response, [privateField]: ['/Users/private/skill'] })
      }, '/v1/skills')
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain('/Users/private/skill')
    }
    const rejectedSkill = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({
        ...response,
        skills: [{ ...response.skills[0], entryPath: '/Users/private/skill/SKILL.md' }]
      })
    }, '/v1/skills')
    expect(rejectedSkill.ok).toBe(false)
    expect(rejectedSkill.status).toBe(502)
    expect(rejectedSkill.body).not.toContain('/Users/private/skill')

    expect(sanitizeRuntimeResponse({ ok: true, status: 200, body: '' }, '/v1/skills')).toEqual({
      ok: false,
      status: 502,
      body: JSON.stringify({
        code: 'runtime_response_schema_invalid',
        message: 'Runtime response failed schema validation.'
      })
    })
  })

  it('strictly validates every case-project response route before IPC', () => {
    const project = {
      id: 'case_0123456789abcdef01234567',
      name: 'case-a',
      rootPath: '/cases/a',
      updatedAt: '2026-07-22T00:00:00Z',
      threadCount: 1,
      runningCount: 0,
      archivedCount: 0,
      lastThreadId: 'thread-1',
      lastPreview: '',
      dataSizeEstimate: 0,
      status: 'ready'
    }
    const thread = {
      id: 'thread-1',
      title: 'Case A',
      workspace: '/cases/a',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: '2026-07-22T00:00:00Z',
      updatedAt: '2026-07-22T00:00:00Z',
      messageCount: 0,
      turnCount: 0,
      historyAuthority: 'case_boundary_only_v1'
    }
    for (const [path, body] of [
      ['/v1/case-projects', { caseProjects: [project], indexStatus: 'ready' }],
      ['/v1/case-projects/case_0123456789abcdef01234567/threads', { threads: [thread] }],
      ['/v1/case-projects/case_0123456789abcdef01234567/detail', { project, threads: [thread] }]
    ] as const) {
      const accepted = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(body)
      }, path, null, 'GET')
      expect(accepted.ok, `${path}: ${accepted.body}`).toBe(true)
      expect(JSON.parse(accepted.body)).toEqual(body)
    }

    const missingCount = { ...project } as Record<string, unknown>
    delete missingCount.threadCount
    for (const body of [
      { caseProjects: [missingCount], indexStatus: 'ready' },
      { caseProjects: [{ ...project, privateField: 'must-not-pass' }], indexStatus: 'ready' },
      { project: null, threads: [] },
      { threads: [{ ...thread, reasoning: 'must-not-pass' }] }
    ]) {
      const path = Object.prototype.hasOwnProperty.call(body, 'caseProjects')
        ? '/v1/case-projects'
        : Object.prototype.hasOwnProperty.call(body, 'project')
          ? '/v1/case-projects/case_0123456789abcdef01234567/detail'
          : '/v1/case-projects/case_0123456789abcdef01234567/threads'
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(body)
      }, path, null, 'GET')
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain('must-not-pass')
    }
  })

  it('accepts the exact case-bound thread list projection and rejects unknown fields', () => {
    const summary = {
      id: 'thread-1',
      title: 'Case A',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: '2026-07-22T00:00:00Z',
      updatedAt: '2026-07-22T00:00:01Z',
      messageCount: 1,
      turnCount: 1,
      latestTurnId: 'turn-1',
      historyAuthority: 'case_boundary_only_v1'
    }
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ threads: [summary] })
    }, '/v1/threads?limit=1', null, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual({ threads: [summary] })

    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ threads: [{ ...summary, privateField: 'must-not-pass' }] })
    }, '/v1/threads?limit=1', null, 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain('must-not-pass')
  })

  it('projects non-success runtime bodies solely from their HTTP status', () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const response = sanitizeRuntimeResponse({
      ok: false,
      status: 401,
      body: JSON.stringify({
        code: 'attachment_upload_unavailable',
        message: privateSentinel,
        details: privateSentinel,
        providerError: privateSentinel
      })
    }, '/v1/runtime/info')

    expect(JSON.parse(response.body)).toEqual({
      code: 'unauthorized',
      message: 'Runtime authentication is required.'
    })
    expect(response.body).not.toContain(privateSentinel)
  })

  it('fails closed on mixed, legacy, or unknown task-output response shapes before IPC', () => {
    const privateOutputSentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    const validWithheld = {
      schemaVersion: 1,
      availability: 'withheld',
      jobId: 'job-1',
      status: 'completed',
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(validWithheld)
    }, '/v1/runtime/task-jobs/output')
    expect(accepted.ok).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(validWithheld)

    const arbitraryAvailable = {
      schemaVersion: 1,
      availability: 'available',
      jobId: 'job-private-output',
      status: 'completed',
      output: privateOutputSentinel,
      offset: 0,
      nextOffset: privateOutputSentinel.length,
      outputBytes: privateOutputSentinel.length,
      truncated: false
    }
    const arbitraryRejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(arbitraryAvailable)
    }, '/v1/runtime/task-jobs/output')
    expect(arbitraryRejected.ok).toBe(false)
    expect(arbitraryRejected.status).toBe(502)
    expect(arbitraryRejected.body).not.toContain(privateOutputSentinel)

    for (const body of [
      { ...validWithheld, output: 'PRIVATE_CHILD_OUTPUT' },
      { ...validWithheld, unknownAuthority: true },
      { jobId: 'job-1', status: 'completed', output: 'legacy bypass' },
      { ...validWithheld, availability: 'model_claimed_safe' }
    ]) {
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(body)
      }, '/v1/runtime/task-jobs/output?offset=9')
      expect(rejected).toEqual({
        ok: false,
        status: 502,
        body: JSON.stringify({
          code: 'runtime_response_schema_invalid',
          message: 'Runtime response failed schema validation.'
        })
      })
      expect(rejected.body).not.toContain('PRIVATE_CHILD_OUTPUT')
    }
  })

  it('strictly validates metadata-only task-job list, wait, and kill responses before IPC', () => {
    const privateSentinel = 'ZXQ_PRIV_7F3C9A2D_41B6'
    const job = {
      schemaVersion: 1,
      id: 'job-1',
      kind: 'background-shell',
      status: 'completed',
      background: true,
      terminal: true,
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
    for (const [path, body] of [
      ['/v1/runtime/task-jobs/list', { jobs: [job], count: 1 }],
      ['/v1/runtime/task-jobs/wait', { jobs: [job] }],
      ['/v1/runtime/task-jobs/kill', { job }]
    ] as const) {
      const accepted = sanitizeRuntimeResponse({
        ok: true, status: 200, body: JSON.stringify(body)
      }, path)
      expect(accepted.ok).toBe(true)
      expect(JSON.parse(accepted.body)).toEqual(body)

      const poisonedJob = { ...job, output: privateSentinel, reasoning_content: privateSentinel }
      const poisonedBody = path.endsWith('/list')
        ? { jobs: [poisonedJob], count: 1 }
        : path.endsWith('/wait')
          ? { jobs: [poisonedJob] }
          : { job: poisonedJob }
      const rejected = sanitizeRuntimeResponse({
        ok: true, status: 200, body: JSON.stringify(poisonedBody)
      }, path)
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain(privateSentinel)
    }
  })

  it('blocks private attachment response fields before IPC', () => {
    const attachment = {
      id: 'att_0123456789abcdef01234567',
      name: 'statement.pdf',
      kind: 'document',
      mimeType: 'application/pdf',
      byteSize: 128,
      scope: 'thread',
      createdAt: '2026-07-14T00:00:00Z',
      updatedAt: '2026-07-14T00:00:00Z'
    }
    expect(sanitizeRuntimeResponse({
      ok: true,
      status: 201,
      body: JSON.stringify({ attachment })
    }, '/v1/attachments').ok).toBe(true)

    for (const body of [
      { attachment: { ...attachment, documentText: 'PRIVATE_ACCOUNT_6222020202020202020' } },
      { attachment: { ...attachment, localFilePath: '/tmp/private.pdf' } },
      { attachment: { ...attachment, hash: 'a'.repeat(64) } },
      { attachment: { ...attachment, textFallback: { dataBase64: 'PRIVATE_FALLBACK' } } }
    ]) {
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(body)
      }, '/v1/attachments/att_0123456789abcdef01234567?thread_id=thr_1&workspace=%2Fworkspace')
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain('PRIVATE_')
      expect(rejected.body).not.toContain('/tmp/private.pdf')
    }
  })

  it('accepts only closed summary-output metadata and drops arbitrary output before IPC', () => {
    const privateOutputSentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    const validChildWithheld = {
      schemaVersion: 1,
      availability: 'withheld',
      taskId: 'taskjob:job-1',
      status: 'completed',
      reasonCode: 'security_bound_child_output',
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
    const validToolWithheld = {
      ...validChildWithheld,
      taskId: 'command:call-1',
      reasonCode: 'tool_output_private',
      outputTrustStatus: 'private_tool_output'
    }
    expect(sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(validChildWithheld)
    }, '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/output').ok).toBe(true)
    expect(sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(validToolWithheld)
    }, '/v1/threads/thread-1/summary/tasks/command%3Acall-1/output').ok).toBe(true)

    const arbitraryRejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        availability: 'available',
        taskId: 'taskjob:job-1',
        status: 'completed',
        output: privateOutputSentinel,
        offset: 0,
        nextOffset: privateOutputSentinel.length,
        outputBytes: privateOutputSentinel.length,
        truncated: false
      })
    }, '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/output')
    expect(arbitraryRejected.ok).toBe(false)
    expect(arbitraryRejected.status).toBe(502)
    expect(arbitraryRejected.body).not.toContain(privateOutputSentinel)

    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ ...validChildWithheld, output: privateOutputSentinel })
    }, '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/output')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain(privateOutputSentinel)
  })

  it('binds strict thread-summary responses to route identity and the public sanitizer', () => {
    const privateSentinel = '<think>PRIVATE_SUMMARY_REASONING_7F3C</think>'
    const accountSentinel = '6222020202020202020'
    const task = {
      schemaVersion: 1,
      id: 'taskjob:job-1',
      kind: 'task',
      status: 'running',
      background: false,
      terminal: false,
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
    const summary = {
      threadId: 'thread-1',
      generatedAt: '2026-07-20T00:00:00Z',
      latestSeq: 1,
      subagents: [],
      tasks: [task],
      outputs: [],
      sources: [],
      sideChats: [],
      backgroundProcesses: []
    }
    const accepted = sanitizeRuntimeResponse({
      ok: true, status: 200, body: JSON.stringify(summary)
    }, '/v1/threads/thread-1/summary')
    expect(accepted.ok).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(summary)

    const foreign = sanitizeRuntimeResponse({
      ok: true, status: 200, body: JSON.stringify({ ...summary, threadId: 'thread-2' })
    }, '/v1/threads/thread-1/summary')
    expect(foreign.ok).toBe(false)

    const factBearing = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ ...summary, outputs: [{ id: 'private', kind: 'file', label: '/private/case.csv' }] })
    }, '/v1/threads/thread-1/summary')
    expect(factBearing.ok).toBe(false)

    for (const title of [privateSentinel, accountSentinel]) {
      const poisoned = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify({
          ...summary,
          sideChats: [{
            threadId: 'side-1', title, status: 'idle', relation: 'side', parentThreadId: 'thread-1',
            createdAt: '2026-07-20T00:00:00Z', updatedAt: '2026-07-20T00:00:01Z'
          }]
        })
      }, '/v1/threads/thread-1/summary')
      expect(poisoned.ok).toBe(false)
      expect(poisoned.body).not.toContain(title)
    }

    const mutation = sanitizeRuntimeResponse({
      ok: true, status: 200, body: JSON.stringify({ task })
    }, '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/kill')
    expect(mutation.ok).toBe(true)
    const foreignMutation = sanitizeRuntimeResponse({
      ok: true, status: 200, body: JSON.stringify({ task: { ...task, id: 'taskjob:job-2' } })
    }, '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/kill')
    expect(foreignMutation.ok).toBe(false)
  })

  it('forwards daily usage requests to the Analytix runtime with bearer auth', async () => {
    let seenUrl = ''
    let seenAuthorization = ''
    let ensured = false
    const port = await listen((req, res) => {
      seenUrl = req.url ?? ''
      seenAuthorization = req.headers.authorization ?? ''
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify({
        group_by: 'day',
        from: '2026-06-01',
        to: '2026-06-02',
        buckets: [],
        totals: {
          input_tokens: 0,
          output_tokens: 0,
          reasoning_tokens: 0,
          total_tokens: 0,
          turns: 0,
          cache_miss_tokens: 0,
          cached_tokens: 0,
          cost_usd: 0,
          cost_cny: 0,
          price_configured: false,
          cache_savings_usd: 0,
          cache_savings_cny: 0,
          token_economy_savings_tokens: 0,
          token_economy_savings_usd: 0,
          token_economy_savings_cny: 0,
          thread_count: 0,
          cache_hit_rate: null,
          days: 2,
          active_days: 0
        },
        timezone: 'Asia/Shanghai'
      }))
    })

    const response = await runtimeRequestViaHost(
      settingsForPort(port),
      '/v1/usage?group_by=day&from=2026-06-01&to=2026-06-02&timezone=Asia%2FShanghai',
      { method: 'GET' },
      async () => {
        ensured = true
      }
    )

    expect(ensured).toBe(true)
    expect(response.ok).toBe(true)
    expect(response.status).toBe(200)
    expect(JSON.parse(response.body)).toEqual(expect.objectContaining({ group_by: 'day' }))
    expect(seenUrl).toBe('/v1/usage?group_by=day&from=2026-06-01&to=2026-06-02&timezone=Asia%2FShanghai')
    expect(seenAuthorization).toBe('Bearer usage-token')
  })

  it('omits bearer auth when runtime token is empty for insecure local runtime mode', async () => {
    let seenAuthorization = ''
    const port = await listen((req, res) => {
      seenAuthorization = req.headers.authorization ?? ''
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify({ threads: [] }))
    })
    const settings = settingsForPort(port)

    const response = await runtimeRequestViaHost(
      {
        ...settings,
        runtime: {
          ...settings.runtime,
          runtimeToken: ''
        }
      },
      '/v1/threads?limit=1',
      { method: 'GET' },
      async () => undefined
    )

    expect(response.ok).toBe(true)
    expect(response.status).toBe(200)
    expect(seenAuthorization).toBe('')
  })

  it('uses settings returned by ensureRuntime when the managed port changes', async () => {
    let seenUrl = ''
    const port = await listen((req, res) => {
      seenUrl = req.url ?? ''
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify({ threads: [] }))
    })

    const response = await runtimeRequestViaHost(
      settingsForPort(1),
      '/v1/threads?limit=1',
      { method: 'GET' },
      async () => settingsForPort(port)
    )

    expect(response.ok).toBe(true)
    expect(response.status).toBe(200)
    expect(seenUrl).toBe('/v1/threads?limit=1')
  })

  it('preserves encoded shared endpoint paths when forwarding to the runtime host', async () => {
    let seenUrl = ''
    let seenMethod = ''
    let seenAuthorization = ''
    let seenContentType = ''
    let seenCustomHeader = ''
    let seenBody = ''
    const path = `${analytixThreadSteerPath(
      'thr/with space?x=1#frag',
      'turn/with space?x=1#frag'
    )}?dry_run=true&timezone=Asia%2FShanghai`
    const port = await listen((req, res) => {
      seenUrl = req.url ?? ''
      seenMethod = req.method ?? ''
      seenAuthorization = req.headers.authorization ?? ''
      seenContentType = firstHeader(req.headers['content-type'])
      seenCustomHeader = firstHeader(req.headers['x-analytix-evidence'])
      req.setEncoding('utf8')
      req.on('data', (chunk) => {
        seenBody += chunk
      })
      req.on('end', () => {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          ok: true,
          threadId: 'thr/with space?x=1#frag',
          turnId: 'turn/with space?x=1#frag'
        }))
      })
    })

    const response = await runtimeRequestViaHost(
      settingsForPort(port),
      path,
      {
        method: 'POST',
        body: '{"action":"step"}',
        headers: { 'X-Analytix-Evidence': 'runtime-host-path' }
      },
      async () => undefined
    )

    expect(response.ok).toBe(true)
    expect(seenMethod).toBe('POST')
    expect(seenUrl).toBe(path)
    expect(seenUrl).toContain('thr%2Fwith%20space%3Fx%3D1%23frag')
    expect(seenUrl).toContain('turn%2Fwith%20space%3Fx%3D1%23frag')
    expect(seenUrl).toContain('timezone=Asia%2FShanghai')
    expect(seenAuthorization).toBe('Bearer usage-token')
    expect(seenContentType).toBe('application/json')
    expect(seenCustomHeader).toBe('runtime-host-path')
    expect(seenBody).toBe('{"action":"step"}')
  })

  it('wraps runtime connection failures with sanitized diagnostics', async () => {
    const settings = settingsForPort(1)
    let thrown: Error | null = null
    try {
      await runtimeRequestViaHost(
        settings,
        '/v1/threads?limit=1&token=query-secret',
        { method: 'GET' },
        async () => undefined
      )
    } catch (error) {
      thrown = error instanceof Error ? error : new Error(String(error))
    }

    expect(thrown).not.toBeNull()
    const payload = JSON.parse(thrown?.message ?? '{}') as {
      code?: string
      message?: string
      details?: Record<string, unknown>
    }
    expect(payload.code).toBe('runtime_unavailable')
    expect(payload.message).toBe('The Analytix runtime is unavailable.')
    expect(payload.message).not.toContain('query-secret')
    expect(payload.details).toBeUndefined()
    expect(JSON.stringify(payload)).not.toContain('usage-token')
    expect(JSON.stringify(payload)).not.toContain('query-secret')
  })
})

describe('runtime backend gate', () => {
  it('reports unexpected ready Go runtime-server exits to the supervisor handler', () => {
    const child = fakeChildProcess()
    const exits: Array<{ code: number | null; signal: NodeJS.Signals | null }> = []
    setGoRuntimeUnexpectedExitHandler((info) => {
      exits.push({ code: info.code, signal: info.signal })
    })
    const observer = observeGoSidecarExit(child, { superviseUnexpectedExit: true })
    observer.markReady()

    child.emit('exit', 23, null)

    expect(exits).toEqual([{ code: 23, signal: null }])
  })

  it('does not report intentional or non-runtime-server Go sidecar exits as crashes', () => {
    const exits: Array<{ code: number | null; signal: NodeJS.Signals | null }> = []
    setGoRuntimeUnexpectedExitHandler((info) => {
      exits.push({ code: info.code, signal: info.signal })
    })

    const intentional = fakeChildProcess()
    const intentionalObserver = observeGoSidecarExit(intentional, { superviseUnexpectedExit: true })
    intentionalObserver.markReady()
    intentionalObserver.markIntentionalStop()
    intentional.emit('exit', 0, null)

    const conformance = fakeChildProcess()
    const conformanceObserver = observeGoSidecarExit(conformance, { superviseUnexpectedExit: false })
    conformanceObserver.markReady()
    conformance.emit('exit', 41, null)

    expect(exits).toEqual([])
  })

  it('uses a bundled runtime-server binary for packaged Go runtime startup', () => {
    const root = mkdtempSync(join(tmpdir(), 'analytix-packaged-go-runtime-'))
    try {
      const appPath = join(root, 'Analytix.app', 'Contents', 'Resources', 'app.asar')
      const binary = resolveBundledGoRuntimeServerPath(appPath, 'darwin')
      mkdirSync(dirname(binary), { recursive: true })
      writeFileSync(binary, '#!/bin/sh\n', 'utf8')

      const target = resolveGoRuntimeLaunchTarget({
        runtimeServer: true,
        appIsPackaged: true,
        appPath,
        platform: 'darwin',
        env: {}
      })

      expect(target).toEqual({
        command: binary,
        argsPrefix: [],
        mode: 'bundled-binary'
      })
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('fails packaged Go runtime startup when the bundled runtime-server binary is missing', () => {
    const root = mkdtempSync(join(tmpdir(), 'analytix-packaged-go-runtime-missing-'))
    try {
      const appPath = join(root, 'Analytix.app', 'Contents', 'Resources', 'app.asar')

      expect(() => resolveGoRuntimeLaunchTarget({
        runtimeServer: true,
        appIsPackaged: true,
        appPath,
        platform: 'darwin',
        env: {}
      })).toThrow(/Packaged Go runtime server binary is missing/)
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('allows explicit runtime-server binary overrides without requiring go run', () => {
    const root = mkdtempSync(join(tmpdir(), 'analytix-go-runtime-bin-override-'))
    try {
      const binary = join(root, 'runtime-server')
      writeFileSync(binary, '#!/bin/sh\n', 'utf8')

      expect(resolveGoRuntimeLaunchTarget({
        runtimeServer: true,
        appIsPackaged: false,
        env: {
          ANALYTIX_GO_RUNTIME_SERVER_BIN: binary
        }
      })).toEqual({
        command: binary,
        argsPrefix: [],
        mode: 'bundled-binary'
      })
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('PackagedRuntimeRejectsServerOverride', () => {
    const root = mkdtempSync(join(tmpdir(), 'analytix-packaged-go-runtime-override-'))
    try {
      const appPath = join(root, 'Analytix.app', 'Contents', 'Resources', 'app.asar')
      const bundled = resolveBundledGoRuntimeServerPath(appPath, 'darwin')
      const override = join(root, 'untrusted-runtime-server')
      mkdirSync(dirname(bundled), { recursive: true })
      writeFileSync(bundled, '#!/bin/sh\n', 'utf8')
      writeFileSync(override, '#!/bin/sh\n', 'utf8')

      expect(() => resolveGoRuntimeLaunchTarget({
        runtimeServer: true,
        appIsPackaged: true,
        appPath,
        platform: 'darwin',
        env: {
          ANALYTIX_GO_RUNTIME_SERVER_BIN: override
        }
      })).toThrow(/rejects ANALYTIX_GO_RUNTIME_SERVER_BIN/)
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  })

  it('keeps the default Go runtime durable root on the Analytix product data dir', () => {
    expect(resolveGoRuntimeConfiguredDurableRoot({
      backend: 'go-runtime-default',
      dataDir: '/Users/sun/.analytix/data'
    })).toEqual({
      durableRoot: '/Users/sun/.analytix/data',
      ownsDurableRoot: false
    })

    expect(resolveGoRuntimeConfiguredDurableRoot({
      backend: 'go-runtime-candidate',
      dataDir: '/Users/sun/.analytix/data',
      candidateDurableRoot: '/tmp/analytix-go-runtime-candidate-root'
    })).toEqual({
      durableRoot: '/tmp/analytix-go-runtime-candidate-root',
      ownsDurableRoot: false
    })

    expect(() => resolveGoRuntimeConfiguredDurableRoot({
      backend: 'go-runtime-candidate',
      dataDir: '/Users/sun/.analytix/data',
      candidateDurableRoot: '/tmp/wrong-root'
    })).toThrow('ANALYTIX_GO_RUNTIME_CANDIDATE_DURABLE_ROOT')
  })

			  it('builds and reuses a cached runtime-server binary for development startup', () => {
		    const root = mkdtempSync(join(tmpdir(), 'analytix-go-runtime-path-'))
		    try {
	      const goBin = join(root, 'go')
	      const goRoot = join(root, 'goroot')
	      const runtimeGoDir = join(root, 'runtime-go')
	      const cacheDir = join(root, 'cache')
	      mkdirSync(join(goRoot, 'src', 'context'), { recursive: true })
	      mkdirSync(join(runtimeGoDir, 'cmd', 'runtime-server'), { recursive: true })
	      writeFileSync(join(goRoot, 'src', 'context', 'context.go'), 'package context\n', 'utf8')
	      writeFileSync(join(runtimeGoDir, 'go.mod'), 'module analytix.local/runtime-go\n', 'utf8')
	      writeFileSync(join(runtimeGoDir, 'cmd', 'runtime-server', 'main.go'), 'package main\nfunc main() {}\n', 'utf8')
	      writeFileSync(goBin, `#!/bin/sh
if [ "$1" = "env" ]; then
  printf '%s\\n' '${goRoot}'
  exit 0
fi
if [ "$1" = "build" ]; then
  out=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then
      shift
      out="$1"
    fi
    shift
  done
  mkdir -p "$(dirname "$out")"
  printf '%s\\n' '#!/bin/sh' > "$out"
  chmod +x "$out"
  exit 0
fi
exit 1
`, { encoding: 'utf8', mode: 0o755 })

	      const target = resolveGoRuntimeLaunchTarget({
	        runtimeServer: true,
	        appIsPackaged: false,
	        runtimeGoDir,
	        goBinaryCandidates: [],
	        env: {
	          PATH: root,
	          ANALYTIX_GO_RUNTIME_SERVER_CACHE_DIR: cacheDir
	        }
	      })
	      expect(target.command.startsWith(realpathSync(cacheDir))).toBe(true)
	      expect(target).toEqual({
	        command: target.command,
	        argsPrefix: [],
	        mode: 'bundled-binary'
	      })
	      expect(readFileSync(target.command, 'utf8')).toContain('#!/bin/sh')

	      const reused = resolveGoRuntimeLaunchTarget({
	        runtimeServer: true,
	        appIsPackaged: false,
	        runtimeGoDir,
	        goBinaryCandidates: [],
	        env: {
	          PATH: root,
	          ANALYTIX_GO_RUNTIME_SERVER_CACHE_DIR: cacheDir
	        }
	      })
	      expect(reused).toEqual(target)

	      writeFileSync(target.command, '#!/bin/sh\nexit 99\n', 'utf8')
	      expect(() => resolveGoRuntimeLaunchTarget({
	        runtimeServer: true,
	        appIsPackaged: false,
	        runtimeGoDir,
	        goBinaryCandidates: [],
	        env: {
	          PATH: root,
	          ANALYTIX_GO_RUNTIME_SERVER_CACHE_DIR: cacheDir
	        }
	      })).toThrow(/failed closed identity verification/)
	    } finally {
	      rmSync(root, { recursive: true, force: true })
		    }
		  })

	  it('uses the formal contract sidecar for non-runtime-server source startup', () => {
	    const root = mkdtempSync(join(tmpdir(), 'analytix-go-contract-sidecar-path-'))
	    try {
	      const goBin = join(root, 'go')
	      const goRoot = join(root, 'goroot')
	      const runtimeGoDir = join(root, 'runtime-go')
	      mkdirSync(join(goRoot, 'src', 'context'), { recursive: true })
	      writeFileSync(join(goRoot, 'src', 'context', 'context.go'), 'package context\n', 'utf8')
	      writeFileSync(goBin, `#!/bin/sh\nprintf '%s\\n' '${goRoot}'\n`, { encoding: 'utf8', mode: 0o755 })

	      expect(resolveGoRuntimeLaunchTarget({
	        runtimeServer: false,
	        appIsPackaged: false,
	        runtimeGoDir,
	        goBinaryCandidates: [],
	        env: {
	          PATH: root
	        }
	      })).toEqual({
	        command: goBin,
	        argsPrefix: ['run', './cmd/contract-sidecar'],
	        cwd: runtimeGoDir,
	        mode: 'go-run-source'
	      })
	    } finally {
	      rmSync(root, { recursive: true, force: true })
	    }
	  })

  it('rejects the contract sidecar in packaged app startup paths', () => {
    expect(() => resolveGoRuntimeLaunchTarget({
      runtimeServer: false,
      appIsPackaged: true,
      env: {}
    })).toThrow(/contract sidecar is test\/conformance-only/i)
  })

		  it('uses Go runtime as the default core and rejects retired or unsupported backend overrides', () => {
    expect(resolveAnalytixRuntimeBackendGate({})).toEqual(expect.objectContaining({
      backend: 'go-runtime-default',
      reason: expect.stringContaining('default runtime core')
    }))
    expect(resolveGoRuntimeG6ReadinessStatus({})).toEqual(expect.objectContaining({
      ready: false,
      explicitReadyGate: false,
      defaultGoBackendEnabled: true,
      rendererVisibleGoSwitcher: false,
      missingRequiredChecks: expect.arrayContaining([
        'durableRestartEvidence',
        'providerMatrix',
        'mcpMatrix',
        'packagedQa'
      ])
    }))
    expect(resolveGoRuntimeG6ReadinessStatus({
      ANALYTIX_D0243_DURABLE_RESTART_PROOF: 'passed'
    })).toEqual(expect.objectContaining({
      durableRestartEvidence: expect.objectContaining({ status: 'missing' })
    }))
    expect(resolveGoRuntimeG6ReadinessStatus({
      ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed'
    })).toEqual(expect.objectContaining({
      durableRestartEvidence: expect.objectContaining({
        status: 'passed',
        message: 'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS=passed'
      })
    }))
    expect(resolveGoRuntimeG6ReadinessStatus({
      ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS: 'passed'
    })).toEqual(expect.objectContaining({
      durableRestartEvidence: expect.objectContaining({
        status: 'passed',
        message: 'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS=passed'
      })
    }))
    expect(resolveGoRuntimeG6ReadinessStatus({
      ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE: 'passed'
    })).toEqual(expect.objectContaining({
      durableRestartEvidence: expect.objectContaining({
        status: 'passed',
        message: 'ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE=passed'
      })
    }))
    const retiredTypeScriptGate = resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'typescript'
    })
    expect(retiredTypeScriptGate).toEqual(expect.objectContaining({
      backend: 'unsupported',
      reason: expect.stringContaining('TypeScript agent runtime backend is retired'),
      unsupportedBackend: expect.objectContaining({
        code: 'retired_backend',
        requestedBackend: 'typescript'
      })
    }))
    expect(resolveReportedActiveBackend(
      'go-runtime-default',
      retiredTypeScriptGate
    )).toBe('go-runtime-default')
    expect(resolveReportedActiveBackend(
      'go-runtime-default',
      resolveAnalytixRuntimeBackendGate({})
    )).toBe('go-runtime-default')
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go'
    }).backend).toBe('go-runtime-default')
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'typo-backend'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      requestedBackend: 'typo-backend',
      reason: expect.stringContaining('Unsupported ANALYTIX_RUNTIME_BACKEND=typo-backend'),
      unsupportedBackend: expect.objectContaining({
        code: 'unsupported_backend',
        requestedBackend: 'typo-backend'
      })
    }))
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go-conformance',
      ANALYTIX_GO_RUNTIME_CONFORMANCE: '1'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      conformanceGateEnabled: true,
      reason: expect.stringContaining('retired from Electron runtime selection'),
      unsupportedBackend: expect.objectContaining({
        code: 'retired_backend',
        requestedBackend: 'go-conformance'
      })
    }))
    for (const requestedBackend of ['go-conformance', 'go-production-candidate', 'go-runtime-candidate']) {
      expect(resolveAnalytixRuntimeBackendGate({
        ANALYTIX_RUNTIME_BACKEND: requestedBackend,
        ANALYTIX_GO_RUNTIME_CONFORMANCE: '1',
        ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE: '1',
        ANALYTIX_GO_RUNTIME_CANDIDATE: '1'
      }, { appIsPackaged: true })).toEqual(expect.objectContaining({
        backend: 'unsupported',
        requestedBackend,
        reason: expect.stringContaining('retired in packaged Analytix'),
        unsupportedBackend: expect.objectContaining({
          code: 'retired_backend',
          requestedBackend
        })
      }))
    }
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go-production-candidate'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      productionCandidateGateEnabled: false
    }))
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go-production-candidate',
      ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE: '1'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      productionCandidateGateEnabled: true,
      reason: expect.stringContaining('retired from Electron runtime selection')
    }))
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go-runtime-candidate'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      runtimeCandidateGateEnabled: false
    }))
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go-runtime-candidate',
      ANALYTIX_GO_RUNTIME_CANDIDATE: '1'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      runtimeCandidateGateEnabled: true,
      g6Readiness: expect.objectContaining({ ready: false })
    }))
    expect(resolveAnalytixRuntimeBackendGate({
      ANALYTIX_RUNTIME_BACKEND: 'go-runtime-candidate',
      ANALYTIX_GO_RUNTIME_CANDIDATE: '1',
      ANALYTIX_D0244_CAPABILITY_MATRIX_GREEN: '1',
      ANALYTIX_D0245_ABSORPTION_MATRIX_GREEN: '1',
      ANALYTIX_D0249_REASONIX_TOPOLOGY_CANDIDATE: '1'
    })).toEqual(expect.objectContaining({
      backend: 'unsupported',
      runtimeCandidateGateEnabled: true,
      g6Readiness: expect.objectContaining({ ready: false })
    }))
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-evidence-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const gate = resolveAnalytixRuntimeBackendGate({
        ANALYTIX_RUNTIME_BACKEND: 'go-runtime-candidate',
        ANALYTIX_GO_RUNTIME_CANDIDATE: '1',
        ...credentialedG6Env(evidence),
        ANALYTIX_D0252_GO_DEFAULT_CANDIDATE_REPORT: join(evidenceDir, 'retired-d0252-candidate.json')
      })
      expect(gate).toEqual(expect.objectContaining({
        backend: 'unsupported',
        runtimeCandidateGateEnabled: true,
        g6Readiness: expect.objectContaining({ ready: true }),
        reason: expect.stringContaining('retired from Electron runtime selection')
      }))
      expect(gate).not.toHaveProperty('d0252CandidateReport')
      expect(resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence)
      })).toEqual(expect.objectContaining({
        ready: true,
        missingRequiredChecks: []
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
    expect(resolveGoRuntimeG6ReadinessStatus({
      ANALYTIX_RUNTIME_READY: '1',
      ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
      ANALYTIX_D0243_PROVIDER_MATRIX_STATUS: 'passed',
      ANALYTIX_D0243_MCP_MATRIX_STATUS: 'passed',
      ANALYTIX_D0243_PACKAGED_QA_STATUS: 'passed'
    })).toEqual(expect.objectContaining({
      ready: false,
      missingRequiredChecks: expect.arrayContaining(['providerMatrix', 'mcpMatrix', 'packagedQa']),
      providerMatrix: expect.objectContaining({ status: 'missing' }),
      mcpMatrix: expect.objectContaining({ status: 'missing' }),
      packagedQa: expect.objectContaining({ status: 'missing' })
    }))
  })

  it('passes the generated runtime config path without Provider authority in the Go sidecar environment', () => {
    const settings = settingsForPort(8123)
    settings.provider = {
      ...defaultModelProviderSettings(),
      providers: [
        ...defaultModelProviderSettings().providers,
        {
          id: 'custom',
          name: 'Custom Provider',
          baseUrl: 'https://custom.example/v1',
          endpointFormat: 'messages',
          models: ['custom-model'],
          modelProfiles: {}
        }
      ]
    }
    settings.runtime.runtimeToken = 'runtime-token'
    settings.runtime.dataDir = '/tmp/analytix-go-sidecar-env'
    settings.runtime.providerId = 'custom'
    settings.runtime.model = 'custom-model'
    const runtime = resolveAnalytixRuntimeSettings(settings)
    const env = buildGoRuntimeSidecarEnv(settings, runtime, '/tmp/analytix-go-sidecar-env', {
      PATH: '/usr/bin'
    })

    expect(resolveGoRuntimeMCPConfigPath('/tmp/analytix-go-sidecar-env'))
      .toBe('/tmp/analytix-go-sidecar-env/config.json')
    expect(env.ANALYTIX_RUNTIME_TOKEN).toBe('runtime-token')
    expect(env.ANALYTIX_MCP_CONFIG_PATH).toBe('/tmp/analytix-go-sidecar-env/config.json')
    expect(env.ANALYTIX_APP_ROOT).toBe(process.cwd())
    expect(env.ANALYTIX_RESOURCES_PATH).toBe(process.cwd())
    expect(env.ANALYTIX_API_KEY).toBeUndefined()
    expect(env.ANALYTIX_MODEL_PROVIDERS).toBeUndefined()
  })

  it('keeps development and packaged launch configuration on the same data-directory-scoped authority path', () => {
    const settings = settingsForPort(8128)
    const runtime = resolveAnalytixRuntimeSettings(settings)
    const syntheticCredential = ['synthetic', 'ambient', 'authority'].join('-')
    const development = buildGoRuntimeSidecarEnv(
      settings,
      runtime,
      '/tmp/analytix-k4-shared-profile',
      {
        PATH: '/usr/bin',
        NODE_ENV: 'development',
        ANALYTIX_API_KEY: syntheticCredential,
        ANALYTIX_MODEL_PROVIDERS: JSON.stringify({ apiKey: syntheticCredential }),
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: syntheticCredential,
        ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_BASE_URL: 'https://ambient-provider.invalid/v1',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'ambient-model',
        ANALYTIX_HUB_TEST_GATEWAY_TOKEN: syntheticCredential,
        ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN: syntheticCredential
      },
      'development-runtime-token'
    )
    const packaged = buildGoRuntimeSidecarEnv(
      settings,
      runtime,
      '/tmp/analytix-k4-shared-profile',
      {
        PATH: '/usr/bin',
        NODE_ENV: 'production',
        ANALYTIX_API_KEY: syntheticCredential,
        ANALYTIX_MODEL_PROVIDERS: JSON.stringify({ apiKey: syntheticCredential }),
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: syntheticCredential,
        ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_BASE_URL: 'https://ambient-provider.invalid/v1',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'ambient-model',
        ANALYTIX_HUB_TEST_GATEWAY_TOKEN: syntheticCredential,
        ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN: syntheticCredential
      },
      'packaged-runtime-token'
    )

    expect(development.ANALYTIX_MCP_CONFIG_PATH).toBe('/tmp/analytix-k4-shared-profile/config.json')
    expect(packaged.ANALYTIX_MCP_CONFIG_PATH).toBe(development.ANALYTIX_MCP_CONFIG_PATH)
    for (const environment of [development, packaged]) {
      expect(environment.ANALYTIX_API_KEY).toBeUndefined()
      expect(environment.ANALYTIX_MODEL_PROVIDERS).toBeUndefined()
      expect(environment.ANALYTIX_RUNTIME_DEEPSEEK_API_KEY).toBeUndefined()
      expect(environment.ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_BASE_URL).toBeUndefined()
      expect(environment.ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL).toBeUndefined()
      expect(environment.ANALYTIX_HUB_TEST_GATEWAY_TOKEN).toBeUndefined()
      expect(environment.ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN).toBeUndefined()
      expect(JSON.stringify(environment)).not.toContain(syntheticCredential)
    }

    const isolated = buildGoRuntimeSidecarEnv(
      settings,
      runtime,
      '/tmp/analytix-k4-isolated-profile',
      { PATH: '/usr/bin' },
      'isolated-runtime-token'
    )
    expect(isolated.ANALYTIX_MCP_CONFIG_PATH).toBe('/tmp/analytix-k4-isolated-profile/config.json')
    expect(isolated.ANALYTIX_MCP_CONFIG_PATH).not.toBe(development.ANALYTIX_MCP_CONFIG_PATH)
  })

  it('uses an ephemeral token for cold managed startup and preserves explicit insecure mode', () => {
    const runtime = defaultAnalytixRuntimeSettings()
    expect(resolveManagedGoRuntimeToken(runtime, () => 'ephemeral-session-token'))
      .toBe('ephemeral-session-token')
    expect(resolveManagedGoRuntimeToken({ ...runtime, runtimeToken: 'configured-token' }, () => 'unused'))
      .toBe('configured-token')
    expect(resolveManagedGoRuntimeToken({ ...runtime, insecure: true }, () => 'unused'))
      .toBe('')
  })

  it('passes an ephemeral token through the child environment without requiring argv', () => {
    const settings = settingsForPort(8126)
    const runtime = resolveAnalytixRuntimeSettings(settings)
    const env = buildGoRuntimeSidecarEnv(
      settings,
      runtime,
      '/tmp/analytix-go-sidecar-ephemeral',
      { PATH: '/usr/bin' },
      'ephemeral-session-token'
    )
    expect(env.ANALYTIX_RUNTIME_TOKEN).toBe('ephemeral-session-token')
  })

  it('removes ambient startup and controlled-artifact authority from the child environment', () => {
    const settings = settingsForPort(8127)
    const runtime = resolveAnalytixRuntimeSettings(settings)
    const env = buildGoRuntimeSidecarEnv(
      settings,
      runtime,
      '/tmp/analytix-go-sidecar-controlled-artifact',
      {
        PATH: '/usr/bin',
        ANALYTIX_AUTHORITY_ANCHOR_V1: '{"ambient":"authority"}',
        ANALYTIX_AUTHORITY_MANIFEST_ROOT: '/ambient/authority/manifests',
        ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT: '/ambient/authority/profiles',
        ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT: '/ambient/authority/bundles',
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL: 'http://192.0.2.1:9999',
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN: 'ambient-attacker-token',
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL: 'https://127.0.0.1:49991',
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN: Buffer.alloc(32, 0x61).toString('base64url'),
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION: '9001',
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST: 'b'.repeat(64),
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER: 'attacker-root',
        ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256: 'a'.repeat(64)
      },
      'runtime-token'
    )
    expect(env.ANALYTIX_AUTHORITY_ANCHOR_V1).toBeUndefined()
    expect(env.ANALYTIX_AUTHORITY_MANIFEST_ROOT).toBeUndefined()
    expect(env.ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT).toBeUndefined()
    expect(env.ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER).toBeUndefined()
    expect(env.ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256).toBeUndefined()

  })

  it.each(['lowercase', 'mixed-case'])('removes %s ambient authority before Windows environment folding', (casing) => {
    const settings = settingsForPort(8127)
    const runtime = resolveAnalytixRuntimeSettings(settings)
    const reserved = [
      'ANALYTIX_API_KEY', 'ANALYTIX_MODEL_PROVIDERS',
      'ANALYTIX_HUB_TEST_GATEWAY_TOKEN', 'ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN',
      'ANALYTIX_RUNTIME_GO_DEEPSEEK_API_KEY', 'ANALYTIX_AUTHORITY_ANCHOR_V1',
      'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN', 'ANALYTIX_RUNTIME_TOKEN',
      'ANALYTIX_MCP_CONFIG_PATH', 'ANALYTIX_APP_ROOT', 'ANALYTIX_RESOURCES_PATH'
    ]
    const alias = (name: string): string => casing === 'lowercase'
      ? name.toLowerCase() : name.replace('ANALYTIX', 'Analytix')
    const ambient: NodeJS.ProcessEnv = {
      SystemRoot: 'C:\\Windows', Path: 'C:\\SyntheticTools',
      ...Object.fromEntries(reserved.map((name) => [alias(name), 'synthetic-stale-authority']))
    }
    const child = buildGoRuntimeSidecarEnv(settings, runtime, '/tmp/analytix-casefold', ambient, 'managed-token')
    expect(child.SystemRoot).toBe(ambient.SystemRoot)
    expect(child.Path).toBe(ambient.Path)
    expect(JSON.stringify(child)).not.toContain('synthetic-stale-authority')
    expect(ambient[alias('ANALYTIX_API_KEY')]).toBe('synthetic-stale-authority')
    for (const name of reserved) {
      const keys = Object.keys(child).filter((key) => key.toUpperCase() === name)
      expect(keys).toEqual(['ANALYTIX_RUNTIME_TOKEN', 'ANALYTIX_MCP_CONFIG_PATH',
        'ANALYTIX_APP_ROOT', 'ANALYTIX_RESOURCES_PATH'].includes(name) ? [name] : [])
    }
    expect(child.ANALYTIX_RUNTIME_TOKEN).toBe('managed-token')
  })

  it('hydrates the Go sidecar env from provider.activeProviderId when runtime providerId is blank', () => {
    const settings = settingsForPort(8124)
    settings.provider = {
      ...defaultModelProviderSettings(),
      activeProviderId: 'deepseek-live',
      providers: [
        ...defaultModelProviderSettings().providers,
        {
          id: 'deepseek-live',
          name: 'DeepSeek Live',
          baseUrl: 'https://api.deepseek.com',
          endpointFormat: 'chat_completions',
          models: ['deepseek-v4-pro'],
          modelProfiles: {}
        }
      ]
    }
    settings.runtime.providerId = ''
    settings.runtime.baseUrl = ''
    settings.runtime.model = 'deepseek-v4-pro'
    const legacySettingsCanary = ['legacy', 'settings', 'credential'].join('-')
    ;(settings.provider as unknown as Record<string, unknown>).apiKey = legacySettingsCanary
    const runtime = resolveAnalytixRuntimeSettings(settings)
    const privateProcessCredential = ['private', 'process', 'authority'].join('-')
    const env = buildGoRuntimeSidecarEnv(settings, runtime, '/tmp/analytix-go-sidecar-active-profile', {
      PATH: '/usr/bin',
      ANALYTIX_API_KEY: privateProcessCredential,
      ANALYTIX_MODEL_PROVIDERS: JSON.stringify({
        providers: [{ id: 'ambient', apiKey: privateProcessCredential }]
      })
    })

    expect(runtime.providerId).toBe('deepseek-live')
    expect(env.ANALYTIX_API_KEY).toBeUndefined()
    expect(env.ANALYTIX_MODEL_PROVIDERS).toBeUndefined()
    expect(JSON.stringify(env)).not.toContain(legacySettingsCanary)
    expect(JSON.stringify(env)).not.toContain(privateProcessCredential)
  })

  it('does not copy settings Provider metadata into Go runtime argv', () => {
    const settings = settingsForPort(8125)
    settings.provider = {
      ...defaultModelProviderSettings(),
      activeProviderId: 'deepseek-live',
      providers: [
        ...defaultModelProviderSettings().providers,
        {
          id: 'deepseek-live',
          name: 'DeepSeek Live',
          baseUrl: 'https://api.deepseek.com',
          endpointFormat: 'chat_completions',
          models: ['deepseek-v4-pro'],
          modelProfiles: {}
        }
      ]
    }
    settings.runtime.providerId = ''
    settings.runtime.baseUrl = ''
    settings.runtime.model = 'deepseek-v4-pro'
    const runtime = resolveAnalytixRuntimeSettings(settings)

    const args = buildGoRuntimeProviderArgs(settings, runtime)
    expect(args).toEqual([
      '--approval-policy',
      'on-request',
      '--sandbox-mode',
      'workspace-write'
    ])
    expect(JSON.stringify(args)).not.toMatch(/deepseek-live|api\.deepseek|deepseek-v4|model-providers|provider-id|base-url|endpoint-format/)
  })

  it('passes only the normalized execution policy to the production Go runtime argv', () => {
    const settings = settingsForPort(8126)
    settings.runtime.runtimeToken = 'runtime-token-must-stay-out-of-argv'
    settings.runtime.approvalPolicy = 'auto'
    settings.runtime.sandboxMode = 'danger-full-access'

    const runtime = resolveAnalytixRuntimeSettings(settings)
    const args = buildGoRuntimeProviderArgs(settings, runtime)

    const policyIndex = args.indexOf('--approval-policy')
    expect(args.slice(policyIndex, policyIndex + 4)).toEqual([
      '--approval-policy',
      'auto',
      '--sandbox-mode',
      'danger-full-access'
    ])
    expect(args).not.toContain('api-key-must-stay-out-of-argv')
    expect(args).not.toContain('runtime-token-must-stay-out-of-argv')

    settings.runtime.approvalPolicy = 'invalid' as typeof settings.runtime.approvalPolicy
    settings.runtime.sandboxMode = 'invalid' as typeof settings.runtime.sandboxMode
    const normalized = resolveAnalytixRuntimeSettings(settings)
    const normalizedArgs = buildGoRuntimeProviderArgs(settings, normalized)

    expect(normalized).toMatchObject({
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
    const normalizedPolicyIndex = normalizedArgs.indexOf('--approval-policy')
    expect(normalizedArgs.slice(normalizedPolicyIndex, normalizedPolicyIndex + 4)).toEqual([
      '--approval-policy',
      'on-request',
      '--sandbox-mode',
      'workspace-write'
    ])
    expect(normalizedArgs).not.toContain('invalid')
  })

  it('does not copy settings Provider proxy authority into Go runtime args', () => {
    const settings = settingsForPort(8126)
    settings.provider = {
      ...defaultModelProviderSettings(),
      proxy: { enabled: true, url: 'socks5://proxy-user:proxy-secret@127.0.0.1:7890' }
    }
    settings.runtime.providerId = 'deepseek'
    const runtime = resolveAnalytixRuntimeSettings(settings)

    const args = buildGoRuntimeProviderArgs(settings, runtime)
    expect(args).toEqual([
      '--approval-policy',
      'on-request',
      '--sandbox-mode',
      'workspace-write'
    ])
    expect(JSON.stringify(args)).not.toContain('proxy-user')
  })

  it('rejects credentialed evidence files that contain secret-like material', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-secret-evidence-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const providerEvidence = JSON.parse(readFileSync(evidence.provider, 'utf8')) as Record<string, unknown>
      writeFileSync(evidence.provider, JSON.stringify({
        ...providerEvidence,
        diagnostics: [
          'upstream error x-api-key: live-secret-value-1234567890',
          'redirect contained signature=live-signature-value',
          'proxy emitted auth: live-auth-value',
          'metadata included credential=live-credential-value',
          'jwt=live-jwt-value',
          'proxy auth Basic bGl2ZS1zZWNyZXQ='
        ]
      }), 'utf8')

      const readiness = resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence)
      })

      expect(readiness).toEqual(expect.objectContaining({
        ready: false,
        missingRequiredChecks: expect.arrayContaining(['providerMatrix'])
      }))
      expect(readiness.providerMatrix).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('secret material')
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('rejects provider evidence without the runtime-go provider matrix root contract', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-provider-evidence-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const providerEvidence = JSON.parse(readFileSync(evidence.provider, 'utf8')) as Record<string, unknown>
      writeFileSync(evidence.provider, JSON.stringify({
        ...providerEvidence,
        id: 'legacy-provider-matrix',
        credentialSecretsRecorded: false
      }), 'utf8')

      const readiness = resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence)
      })

      expect(readiness).toEqual(expect.objectContaining({
        ready: false,
        missingRequiredChecks: expect.arrayContaining(['providerMatrix'])
      }))
      expect(readiness.providerMatrix).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('runtime-go provider matrix')
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('rejects operator gate evidence that does not cover the current evidence commit', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-operator-evidence-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const operatorEvidence = JSON.parse(readFileSync(evidence.operator, 'utf8')) as Record<string, unknown>
      writeFileSync(evidence.operator, JSON.stringify({
        ...operatorEvidence,
        evidenceTargetCommit: 'old-commit'
      }), 'utf8')

      const readiness = resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence)
      })

      expect(readiness).toEqual(expect.objectContaining({
        ready: false,
        missingRequiredChecks: expect.arrayContaining(['operatorGate'])
      }))
      expect(readiness.operatorGate).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('does not cover')
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('rejects retired numbered operator gate evidence ids', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-operator-retired-id-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const operatorEvidence = JSON.parse(readFileSync(evidence.operator, 'utf8')) as Record<string, unknown>
      writeFileSync(evidence.operator, JSON.stringify({
        ...operatorEvidence,
        id: 'd0252-go-default-candidate-operator-gate'
      }), 'utf8')

      const readiness = resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence)
      })

      expect(readiness).toEqual(expect.objectContaining({
        ready: false,
        missingRequiredChecks: expect.arrayContaining(['operatorGate'])
      }))
      expect(readiness.operatorGate).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('id')
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('rejects permissive operator evidence with failed status, missing explicit gate, and non-SHA commit override', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-operator-permissive-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const operatorEvidence = JSON.parse(readFileSync(evidence.operator, 'utf8')) as Record<string, unknown>
      writeFileSync(evidence.operator, JSON.stringify({
        ...operatorEvidence,
        status: 'failed',
        passed: true,
        explicitEnvGate: false,
        evidenceTargetCommit: 'not-a-git-sha'
      }), 'utf8')

      const readiness = resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence),
        ANALYTIX_RUNTIME_GO_CURRENT_COMMIT: 'not-a-git-sha'
      })

      expect(readiness).toEqual(expect.objectContaining({
        ready: false,
        missingRequiredChecks: expect.arrayContaining(['operatorGate'])
      }))
      expect(readiness.operatorGate).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('explicitEnvGate')
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('rejects root-level MCP passed evidence without credentialed probe details', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-mcp-evidence-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      writeFileSync(evidence.mcp, JSON.stringify({
        id: 'runtime-go-mcp-execution',
        status: 'passed',
        passed: true,
        credentialedExecution: true,
        topLevelMcpIndexerExposed: false,
        reasonixPublicProtocolUsed: false,
        connect: true,
        toolDiscoverySearch: true,
        toolCall: true,
        approvalUserInput: true,
        reconnect: true,
        redaction: true,
        credentialedProbes: []
      }), 'utf8')

      const readiness = resolveGoRuntimeG6ReadinessStatus({
        ...credentialedG6Env(evidence)
      })

      expect(readiness).toEqual(expect.objectContaining({
        ready: false,
        missingRequiredChecks: expect.arrayContaining(['mcpMatrix'])
      }))
      expect(readiness.mcpMatrix).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('credentialed-mcp probe')
      }))
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('accepts the Go conformance backend only when the canary boundary is fixture-only and hidden', async () => {
    const port = await listen((req, res) => {
      if (req.url === '/health') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ threads: [] }))
        return
      }
      if (req.url === '/v1/conformance/loop/boundary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          testConformanceOnly: true,
          minimalAgentLoopPrototype: true,
          fixtureBackedLoopOnly: true,
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false,
          electronMainConnected: false
        }))
        return
      }
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ code: 'not_found' }))
    })

    const result = await probeGoConformanceRuntimeCanary(settingsForPort(port))

    expect(result).toEqual(expect.objectContaining({
      ok: true,
      checks: expect.arrayContaining([
        'health',
        'thread-api',
        'go-conformance-boundary',
        'renderer-visible-go-route-hidden'
      ])
    }))
  })

  it('rejects a Go canary that claims default-backend or renderer-visible route authority', async () => {
    const port = await listen((req, res) => {
      if (req.url === '/health') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ threads: [] }))
        return
      }
      if (req.url === '/v1/conformance/loop/boundary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          testConformanceOnly: true,
          minimalAgentLoopPrototype: true,
          fixtureBackedLoopOnly: true,
          defaultGoBackendEnabled: true,
          rendererVisibleGoRoutesAllowed: true,
          reasonixPublicProtocolAllowed: false,
          electronMainConnected: false
        }))
        return
      }
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ code: 'not_found' }))
    })

    const result = await probeGoConformanceRuntimeCanary(settingsForPort(port))

    expect(result).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-conformance-boundary'
    }))
  })

  it('accepts the Go production-candidate backend only after live provider, durable, gate, MCP, and job canary checks', async () => {
    const port = await listen((req, res) => {
      if (req.url === '/health') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ threads: [] }))
        return
      }
      if (req.url === '/v1/conformance/loop/boundary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          testConformanceOnly: true,
          minimalAgentLoopPrototype: true,
          fixtureBackedLoopOnly: true,
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false,
          electronMainConnected: false
        }))
        return
      }
      if (req.url === '/v1/internal/go-production-candidate/boundary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          runtimeGoContractParitySlice: true,
          internalGateOnly: true,
          usesContractReplayProviderServer: true,
          usesRealDurableEventSink: true,
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false
        }))
        return
      }
      if (req.url === '/v1/internal/go-production-candidate/canary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          ok: true,
          checks: [
            'contract-provider-server',
            'durable-replay',
            'approval-user-input-manager',
            'contract-mcp-manager',
            'job-lineage',
            'single-baseline-checklist'
          ],
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false
        }))
        return
      }
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ code: 'not_found' }))
    })

    const result = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-production-candidate'
    )

    expect(result).toEqual(expect.objectContaining({
      ok: true,
      checks: expect.arrayContaining([
        'go-production-candidate-boundary',
        'contract-provider-server',
        'durable-replay',
        'approval-user-input-manager',
        'contract-mcp-manager',
        'job-lineage',
        'single-baseline-checklist'
      ])
    }))
  })

  it('rejects a Go production-candidate canary that skips live manager evidence', async () => {
    const port = await listen((req, res) => {
      if (req.url === '/health') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ threads: [] }))
        return
      }
      if (req.url === '/v1/conformance/loop/boundary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          testConformanceOnly: true,
          minimalAgentLoopPrototype: true,
          fixtureBackedLoopOnly: true,
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false,
          electronMainConnected: false
        }))
        return
      }
      if (req.url === '/v1/internal/go-production-candidate/boundary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          runtimeGoContractParitySlice: true,
          internalGateOnly: true,
          usesContractReplayProviderServer: true,
          usesRealDurableEventSink: true,
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false
        }))
        return
      }
      if (req.url === '/v1/internal/go-production-candidate/canary') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({
          ok: true,
          checks: [
            'contract-provider-server',
            'durable-replay',
            'approval-user-input-manager'
          ],
          defaultGoBackendEnabled: false,
          rendererVisibleGoRoutesAllowed: false,
          reasonixPublicProtocolAllowed: false
        }))
        return
      }
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ code: 'not_found' }))
    })

    const result = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-production-candidate'
    )

    expect(result).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-production-candidate-canary'
    }))
  })

  it('redacts secrets from Go runtime canary response excerpts', async () => {
    const port = await listen((_req, res) => {
      res.writeHead(500, { 'Content-Type': 'text/plain' })
      res.end('startup failed Authorization: Bearer sk-liveSecretValue1234567890 token=runtime-token-value')
    })

    const result = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-default'
    )
    const message = result.ok ? '' : result.message

    expect(result).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'health'
    }))
    expect(message).toContain('Authorization=<redacted>')
    expect(message).toContain('token=<redacted>')
    expect(message).not.toContain('sk-liveSecretValue1234567890')
    expect(message).not.toContain('runtime-token-value')
  })

  it('rejects old raw runtime tools diagnostics during the current-run canary', async () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const port = await listen((req, res) => {
      res.setHeader('Content-Type', 'application/json')
      if (req.url === '/health') {
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401)
        res.end(JSON.stringify({ code: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.end(JSON.stringify({ threads: [] }))
        return
      }
      if (req.url === '/v1/runtime/info') {
        res.end(JSON.stringify(publicRuntimeInfoFixture(port)))
        return
      }
      if (req.url === '/v1/runtime/tools') {
        res.end(JSON.stringify({
          providers: [{ apiKey: privateSentinel }],
          toolContracts: [{ inputSchema: { description: privateSentinel } }],
          lastError: privateSentinel
        }))
        return
      }
      res.writeHead(404)
      res.end(JSON.stringify({ code: 'not_found' }))
    })

    const result = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      passedG6Readiness()
    )

    expect(result).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-runtime-candidate-tools'
    }))
    expect(JSON.stringify(result)).not.toContain(privateSentinel)
  })

  it('accepts the Go runtime-candidate backend only through real runtime contract endpoints', async () => {
    const canaryThreadCreateBodies: Array<Record<string, unknown>> = []
    const canaryTurnBodies: Array<Record<string, unknown>> = []
    let exposeAssistantDraft = false
    let mismatchTerminalTurn = false
    let gapBeforeGeneralTerminalBatch = false
    let truncateReplay = false
    const captureJSONBody = (
      req: IncomingMessage,
      onBody: (body: Record<string, unknown>) => void
    ): void => {
      let raw = ''
      req.setEncoding('utf8')
      req.on('data', (chunk) => {
        raw += chunk
      })
      req.on('end', () => {
        try {
          onBody(raw ? JSON.parse(raw) as Record<string, unknown> : {})
        } catch {
          onBody({})
        }
      })
    }
    const port = await listen((req, res) => {
      if (req.url === '/health') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'unauthorized', message: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ threads: [{ id: 'thr_old', title: 'Old durable thread' }] }))
        return
      }
      if (req.url === '/v1/threads' && req.method === 'POST') {
        captureJSONBody(req, (body) => {
          canaryThreadCreateBodies.push(body)
          res.writeHead(201, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify({ id: 'thr_canary', title: 'Go Runtime Candidate Canary', latestSeq: 0 }))
        })
        return
      }
      if (req.url === '/v1/runtime/info') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify(publicRuntimeInfoFixture(port)))
        return
      }
      if (req.url === '/v1/runtime/tools') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify(publicRuntimeToolsFixture()))
        return
      }
      if (req.url === '/v1/threads/thr_canary' && req.method === 'GET') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ id: 'thr_canary', latestSeq: 0 }))
        return
      }
      if (req.url === '/v1/threads/thr_canary/fork' && req.method === 'POST') {
        res.writeHead(201, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ id: 'thr_side', relation: 'side', parentThreadId: 'thr_canary' }))
        return
      }
      if (req.url === '/v1/sessions/thr_canary/resume-thread' && req.method === 'POST') {
        res.writeHead(201, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ thread_id: 'thr_resume', session_id: 'thr_canary', message_count: 1, summary: 'resumed' }))
        return
      }
      if (req.url === '/v1/threads/thr_canary' && req.method === 'PATCH') {
        captureJSONBody(req, (body) => {
          res.setHeader('Content-Type', 'application/json')
          res.end(JSON.stringify({ id: 'thr_canary', title: body.title, status: body.status }))
        })
        return
      }
      if (req.url === '/v1/threads/thr_canary/turns' && req.method === 'POST') {
        captureJSONBody(req, (body) => {
          canaryTurnBodies.push(body)
          res.writeHead(202, { 'Content-Type': 'application/json' })
          res.end(JSON.stringify({ threadId: 'thr_canary', turnId: 'turn_canary', userMessageItemId: 'item_user' }))
        })
        return
      }
      if (req.url === '/v1/threads/thr_canary/events?since_seq=0') {
        res.setHeader('Content-Type', 'text/event-stream; charset=utf-8')
        const frames: string[] = []
        const timestamp = '2026-06-23T00:00:01Z'
        let lastSeq = 0
        const pushFrame = (kind: string, payload: Record<string, unknown>): void => {
          const seq = lastSeq + 1
          lastSeq = seq
          const data = { kind, seq, timestamp, threadId: 'thr_canary', ...payload }
          frames.push(`id: ${seq}\nevent: ${kind}\ndata: ${JSON.stringify(data)}`)
        }
        pushFrame('turn_started', { turnId: 'turn_canary', status: 'running' })
        pushFrame('item_created', {
          turnId: 'turn_canary',
          itemId: 'item_user',
          item: {
            id: 'item_user', turnId: 'turn_canary', threadId: 'thr_canary', role: 'user',
            status: 'completed', createdAt: timestamp, finishedAt: timestamp,
            kind: 'user_message', text: 'Go runtime canary'
          }
        })
        pushFrame('approval_requested', {
          turnId: 'turn_canary', approvalId: 'appr_turn_canary', toolName: 'canary_tool', status: 'pending'
        })
        pushFrame('user_input_requested', {
          turnId: 'turn_canary', inputId: 'input_turn_canary', status: 'pending'
        })
        pushFrame('tool_catalog_changed', {
          fingerprint: 'a'.repeat(64),
          toolCount: 1,
          changeKind: 'additive',
          toolNames: ['canary_tool'],
          message: 'MCP tool catalog changed after refresh.'
        })
        if (exposeAssistantDraft) {
          pushFrame('assistant_text_delta', {
            turnId: 'turn_canary', itemId: 'item_turn_canary_assistant',
            item: {
              id: 'item_turn_canary_assistant', threadId: 'thr_canary', turnId: 'turn_canary',
              role: 'assistant', status: 'running', createdAt: timestamp, kind: 'assistant_text',
              text: 'DRAFT_SENTINEL'
            }
          })
        }
        pushFrame('pipeline_stage', {
          turnId: 'turn_canary',
          stage: 'subagent_completed',
          message: 'child output withheld',
          child: {
            kind: 'subagent_task', id: 'child_canary', childId: 'child_canary',
            parentThreadId: 'thr_canary', parentTurnId: 'turn_canary',
            status: 'completed', childStatus: 'completed', terminal: true,
            outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
            factAnswerAllowed: false, evidenceAuthority: false,
            canReadOutput: false, canContinueParent: false
          }
        })
        const generalTerminalBatch = canaryGeneralTerminalBatch(
          'thr_canary',
          'turn_general_canary',
          lastSeq + (gapBeforeGeneralTerminalBatch ? 2 : 1),
          timestamp
        )
        lastSeq = generalTerminalBatch.lastSeq as number
        frames.push(
          `id: ${lastSeq}\nevent: general_terminal_batch\ndata: ${JSON.stringify(generalTerminalBatch)}`
        )
        const acceptedFinalBatch = canaryAcceptedFinalBatch(
          'thr_canary',
          'turn_canary',
          lastSeq + 1,
          timestamp,
          mismatchTerminalTurn ? 'turn_stale' : 'turn_canary'
        )
        lastSeq = acceptedFinalBatch.lastSeq as number
        frames.push(
          `id: ${lastSeq}\nevent: accepted_final_batch\ndata: ${JSON.stringify(acceptedFinalBatch)}`
        )
        pushFrame('approval_resolved', {
          turnId: 'turn_canary', approvalId: 'appr_turn_canary', toolName: 'canary_tool', status: 'denied'
        })
        pushFrame('user_input_resolved', {
          turnId: 'turn_canary', inputId: 'input_turn_canary', status: 'submitted'
        })
        res.end(truncateReplay ? frames.join('\n\n') : `${frames.join('\n\n')}\n\n`)
        return
      }
      if (req.url === '/v1/approvals/appr_turn_canary' && req.method === 'POST') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ approvalId: 'appr_turn_canary', decision: 'deny', status: 'denied' }))
        return
      }
      if (req.url === '/v1/user-inputs/input_turn_canary' && req.method === 'POST') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ inputId: 'input_turn_canary', status: 'submitted', answers: [{ id: 'q1', label: 'Ship', value: 'yes' }] }))
        return
      }
      if (
        req.method === 'DELETE' &&
        (
          req.url === '/v1/threads/thr_canary' ||
          req.url === '/v1/threads/thr_side' ||
          req.url === '/v1/threads/thr_resume'
        )
      ) {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ deleted: true }))
        return
      }
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ code: 'not_found', message: 'route not found' }))
    })

    const defaultResult = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-default'
    )

    expect(defaultResult).toEqual(expect.objectContaining({
      ok: true,
      checks: expect.arrayContaining([
        'go-runtime-default-info',
        'go-runtime-default-tools',
        'go-runtime-default-product-readiness',
        'post-cutover-live-validation-pending',
        'go-runtime-default-thread-create',
        'go-runtime-default-thread-read',
        'go-runtime-default-fork',
        'go-runtime-default-resume',
        'go-runtime-default-patch',
        'go-runtime-default-thread-delete',
        'go-production-internal-route-hidden'
      ])
    }))
    expect(canaryTurnBodies).toHaveLength(0)

    const result = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      passedG6Readiness()
    )

    expect(result).toEqual(expect.objectContaining({
      ok: true,
      checks: expect.arrayContaining([
        'go-runtime-candidate-info',
        'go-runtime-candidate-tools',
        'go-runtime-candidate-thread-create',
        'go-runtime-candidate-thread-read',
        'go-runtime-candidate-fork',
        'go-runtime-candidate-resume',
        'go-runtime-candidate-patch',
        'go-runtime-candidate-turn',
        'go-runtime-candidate-sse-replay',
        'go-runtime-candidate-gates',
        'go-runtime-candidate-thread-delete',
        'go-runtime-candidate-g6-readiness',
        'durable-restart-evidence',
        'provider-matrix-status',
        'mcp-matrix-status',
        'packaged-qa-status',
        'go-production-internal-route-hidden'
      ])
    }))

    exposeAssistantDraft = true
    const draftResult = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      passedG6Readiness()
    )
    expect(draftResult).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-runtime-candidate-assistant-draft-exposed'
    }))
    exposeAssistantDraft = false

    mismatchTerminalTurn = true
    const mismatchedTerminalResult = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      passedG6Readiness()
    )
    expect(mismatchedTerminalResult).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-runtime-candidate-sse-replay'
    }))
    mismatchTerminalTurn = false

    gapBeforeGeneralTerminalBatch = true
    const generalGapResult = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      passedG6Readiness()
    )
    expect(generalGapResult).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-runtime-candidate-sse-replay'
    }))
    gapBeforeGeneralTerminalBatch = false

    truncateReplay = true
    const truncatedReplayResult = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      passedG6Readiness()
    )
    expect(truncatedReplayResult).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-runtime-candidate-sse-replay'
    }))
    truncateReplay = false

    const inheritedSettings = settingsForPort(port)
    inheritedSettings.provider = {
      baseUrl: 'https://api.deepseek.com',
      proxy: { enabled: false, url: '' },
      providers: [
        {
          id: 'deepseek',
          name: 'DeepSeek',
          baseUrl: 'https://api.deepseek.com',
          endpointFormat: 'chat_completions',
          models: ['deepseek-v4-pro'],
          modelProfiles: {}
        }
      ]
    }
    inheritedSettings.runtime.providerId = 'deepseek'
    inheritedSettings.runtime.baseUrl = ''
    inheritedSettings.runtime.model = 'deepseek-v4-pro'

    const turnCountBeforeInheritedDefault = canaryTurnBodies.length
    const inheritedResult = await probeGoConformanceRuntimeCanary(
      inheritedSettings,
      undefined,
      'go-runtime-default'
    )
    expect(inheritedResult).toEqual(expect.objectContaining({ ok: true }))
    const lastThreadBody = canaryThreadCreateBodies[canaryThreadCreateBodies.length - 1]
    expect(lastThreadBody).toMatchObject({ providerId: 'deepseek', model: 'deepseek-v4-pro' })
    expect(canaryTurnBodies).toHaveLength(turnCountBeforeInheritedDefault)
  })

  it('rejects the Go runtime-candidate backend when formal readiness evidence is missing even if absorption matrices are green', async () => {
    const port = await listen((req, res) => {
      if (req.url === '/health') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ status: 'ok', service: 'analytix', mode: 'serve' }))
        return
      }
      if (req.headers.authorization !== 'Bearer usage-token') {
        res.writeHead(401, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ code: 'unauthorized', message: 'unauthorized' }))
        return
      }
      if (req.url === '/v1/threads?limit=1') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify({ threads: [{ id: 'thr_canary', title: 'Canary' }] }))
        return
      }
      if (req.url === '/v1/runtime/info') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify(publicRuntimeInfoFixture(port)))
        return
      }
      if (req.url === '/v1/runtime/tools') {
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify(publicRuntimeToolsFixture()))
        return
      }
      res.writeHead(404, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ code: 'not_found', message: 'route not found' }))
    })

    const result = await probeGoConformanceRuntimeCanary(
      settingsForPort(port),
      undefined,
      'go-runtime-candidate',
      {
        ...passedG6Readiness(),
        ready: false,
        providerMatrix: { status: 'skipped', required: true, message: 'no credentials configured' },
        missingRequiredChecks: ['providerMatrix']
      }
    )

    expect(result).toEqual(expect.objectContaining({
      ok: false,
      failedCheck: 'go-runtime-candidate-g6-readiness'
    }))
  })

  it('fails closed instead of silently rolling back when the default Go runtime startup fails', async () => {
    await expect(ensureGoDefaultBackend(
      settingsForPort(1),
      async () => {
        throw new Error('default Go runtime unavailable')
      }
    )).rejects.toThrow(/TypeScript runtime fallback is retired/)

    expect(getAnalytixRuntimeBackendStatus()).toEqual(expect.objectContaining({
      activeBackend: 'go-runtime-default',
      fallbackReason: 'default Go runtime unavailable'
    }))
  })

  it('redacts secrets from Go runtime stderr before storing fallback status', async () => {
    await expect(ensureGoDefaultBackend(
      settingsForPort(1),
      async () => {
        throw new Error('startup failed Authorization: Bearer sk-liveSecretValue1234567890')
      }
    )).rejects.toThrow(/TypeScript runtime fallback is retired/)

    const status = getAnalytixRuntimeBackendStatus()
    expect(status.activeBackend).toBe('go-runtime-default')
    expect(status.fallbackReason).toContain('Authorization=<redacted>')
    expect(status.fallbackReason).not.toContain('sk-liveSecretValue1234567890')
    expect(status.fallbackReason).not.toContain('Bearer sk-')
  })

  it('ships the formal packaged QA command without reviving legacy evidence scripts', () => {
    const result = spawnSync(process.execPath, ['scripts/runtime-go-packaged-qa.mjs', '--dry-run', '--json'], {
      cwd: process.cwd(),
      encoding: 'utf8'
    })
    const report = JSON.parse(result.stdout) as {
      id: string
      status: string
      packaged: {
        actualPackagedAppEvidenceRequiredForFinalGate: boolean
        cutoverFinalAcceptanceReady: boolean
        missingFinalCoverage: string[]
      }
      checks: Array<{ id: string; status: string; command: string }>
    }

    expect(result.status).toBe(1)
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'skipped'
    }))
    expect(report.packaged.actualPackagedAppEvidenceRequiredForFinalGate).toBe(true)
    expect(report.packaged.cutoverFinalAcceptanceReady).toBe(false)
    expect(report.packaged.missingFinalCoverage).toEqual([
      'packaged-desktop-qa',
      'typescript-runtime-source-deleted',
      'durable-restart',
      'formal-release-publication-authority',
      'controlled-release-native-receipt'
    ])
    expect(report.checks[0]).toEqual(expect.objectContaining({
      id: 'cutover-report',
      status: 'skipped',
      command: 'npm run runtime:go:cutover-report -- --json'
    }))
  })

  it('ships the formal default readiness report that cannot authorize dry-run cutover', () => {
    const result = spawnSync(process.execPath, ['scripts/runtime-go-default-readiness-report.mjs', '--dry-run', '--json'], {
      cwd: process.cwd(),
      encoding: 'utf8'
    })
    const report = JSON.parse(result.stdout) as {
      id: string
      status: string
      goDefaultReady: boolean
      deterministicEvidenceOnly: boolean
      longRunningBenchmarkRequired: boolean
      checks: Array<{ id: string; status: string; command: string }>
    }

    expect(result.status).toBe(1)
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-default-readiness-report',
      status: 'skipped',
      goDefaultReady: false,
      deterministicEvidenceOnly: true,
      longRunningBenchmarkRequired: false
    }))
    expect(report.checks[0]).toEqual(expect.objectContaining({
      id: 'runtime-health-smoke',
      status: 'skipped',
      command: 'npm run runtime:go:health-smoke -- --json'
    }))
  })

  it('rejects packaged QA evidence when Go runtime subagents are not available', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-subagent-packaged-evidence-'))
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      const packaged = JSON.parse(readFileSync(evidence.packaged, 'utf8')) as Record<string, any>
      packaged.runtime.runtimeInfo.subagentsAvailable = false
      writeFileSync(evidence.packaged, JSON.stringify(packaged, null, 2), 'utf8')

      const status = resolveGoRuntimeG6ReadinessStatus(credentialedG6Env(evidence))

      expect(status.ready).toBe(false)
      expect(status.packagedQa).toEqual(expect.objectContaining({
        status: 'failed',
        message: expect.stringContaining('runtime.runtimeInfo.subagentsAvailable')
      }))
      expect(status.missingRequiredChecks).toContain('packagedQa')
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }
  })

  it('rejects retired candidate backend when QA is missing and exposes no renderer Go preflight switcher', () => {
    const evidenceDir = mkdtempSync(join(tmpdir(), 'analytix-runtime-evidence-'))
    let gate!: ReturnType<typeof resolveAnalytixRuntimeBackendGate>
    try {
      const evidence = writeRuntimeEvidenceFiles(evidenceDir)
      gate = resolveAnalytixRuntimeBackendGate({
        ANALYTIX_RUNTIME_BACKEND: 'go-runtime-candidate',
        ANALYTIX_GO_RUNTIME_CANDIDATE: '1',
        ...credentialedG6Env(evidence),
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: '',
        ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE: ''
      })
    } finally {
      rmSync(evidenceDir, { recursive: true, force: true })
    }

    expect(gate).toEqual(expect.objectContaining({
      backend: 'unsupported',
      runtimeCandidateGateEnabled: true,
      unsupportedBackend: expect.objectContaining({
        code: 'retired_backend',
        requestedBackend: 'go-runtime-candidate'
      }),
      g6Readiness: expect.objectContaining({
        ready: false,
        rendererVisibleGoSwitcher: false,
        missingRequiredChecks: expect.arrayContaining(['packagedQa'])
      })
    }))
  })

  it('ships the formal preflight as an audit command without authorizing dry-run cutover', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-evidence-'))
    const liveEvidence = join(dir, 'live-evidence-report.json')
    const operatorEvidence = join(dir, 'operator-gate.json')
    const operatorDependencySummary = {
      schemaVersion: 1,
      envGate: {
        runtimeReady: true,
        operatorApprovesDefault: true
      },
      evidence: {
        providerPassed: false,
        mcpPassed: true,
        packagedPassed: true,
        credentialedEvidenceReviewed: false
      },
      commitBinding: {
        required: false,
        relevantEvidenceClean: false,
        bound: true
      },
      blockers: [
        'provider matrix evidence is not passed'
      ]
    }
    writeFileSync(operatorEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-operator-gate',
      status: 'live_blocked',
      passed: false,
      dependencySummary: operatorDependencySummary
    }), 'utf8')
    writeFileSync(liveEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-live-evidence-report',
      status: 'live_blocked',
      passed: false,
      liveOrActualEvidenceUsed: true,
      deterministicEvidenceOnly: false,
      llmAnswerQualityEvidenceUsed: false,
      componentStatus: {
        provider: 'live_blocked',
        mcp: 'passed',
        packaged: 'passed',
        operator: 'live_blocked'
      },
      evidencePaths: {
        operator: operatorEvidence
      },
      nextEvidenceActions: {
        providerMatrix: {
          evidencePath: join(dir, 'provider-matrix.json'),
          settingsProfileCoverage: {
            status: 'loaded',
            source: 'default-app-settings',
            providerCount: 1,
            matchedProviderIds: ['deepseek'],
            usableProviderIds: ['deepseek'],
            usableNonDeepSeekProviderIds: [],
            deepseekSettingsProfileUsable: true,
            nonDeepSeekSettingsProfileUsable: false,
            satisfiesFinalProviderCoverageFromSettings: false,
            credentialValuesRecorded: false
          },
          completionOptions: {
            settingsProfilePath: 'provider.providers[]'
          },
          credentialedDeepSeekPassed: true,
          credentialedNonDeepSeekProviderIds: []
        }
      },
      missingExternalInputs: [
        {
          id: 'provider:at-least-one-non-deepseek-provider',
          reason: 'missing credential',
          candidateProviderIds: ['openai-compatible'],
          candidates: [
            {
              id: 'openai-compatible',
              status: 'skipped',
              missingEnv: [
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY',
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL',
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL'
              ],
              missingSettingsProfileFields: ['provider.providers[].matchingProfile']
            }
          ]
        },
        { id: 'operator:gate', reason: 'operator approval missing' }
      ]
    }), 'utf8')
    const preflight = spawnSync(
      process.execPath,
      ['scripts/runtime-go-preflight.mjs', '--dry-run', '--json', '--live-evidence-json', liveEvidence],
      { cwd: process.cwd(), encoding: 'utf8', env: envWithoutRuntimeGoOperatorGate() }
    )
    const report = JSON.parse(preflight.stdout) as {
      id: string
      status: string
      localChecksStatus: string
      gateMode: boolean
      dryRunCannotAuthorizeCutover: boolean
      expectedBlocked: boolean
      finalGateBlocked: boolean
      finalGateBlockers: string[]
      finalGateBlockerSummary: Record<string, unknown>
      finalGateNextActions: {
        credentialSecretsRecorded: boolean
        valuesRecorded: boolean
        actions: Array<{
          id: string
          status: string
          blockerIds?: string[]
          remediation?: {
            requiresOperatorReview: boolean
            destructiveCleanupSuggested: boolean
            guidance: string
          }
          command?: string
          evidencePath?: string
          dependencySummary?: Record<string, unknown>
          evidenceEnvGate?: Record<string, unknown>
          currentEnvGate?: Record<string, unknown>
          evidenceRefreshRequired?: boolean
          evidenceRefreshCommand?: string
          defaultReadinessCommand?: string
          candidateProviderIds?: string[]
          candidates?: Array<{
            id: string
            missingEnv: string[]
            missingSettingsProfileFields: string[]
          }>
          generatedArtifactDirtyFileCount?: number
          generatedArtifactDirtyFiles?: string[]
          generatedArtifactDirtyFilesTruncated?: boolean
          unclassifiedGeneratedArtifactDirtyFileCount?: number
          unclassifiedGeneratedArtifactDirtyFiles?: string[]
          unclassifiedGeneratedArtifactDirtyFilesTruncated?: boolean
          classifiedGeneratedArtifactDirtyFileCount?: number
          classifiedGeneratedArtifactDirtyFiles?: string[]
          classifiedGeneratedArtifactDirtyFilesTruncated?: boolean
          trackedGeneratedArtifactDirtyFileCount?: number
          trackedGeneratedArtifactDirtyFiles?: string[]
          trackedGeneratedArtifactDirtyFilesTruncated?: boolean
          unclassifiedTrackedGeneratedArtifactDirtyFileCount?: number
          unclassifiedTrackedGeneratedArtifactDirtyFiles?: string[]
          unclassifiedTrackedGeneratedArtifactDirtyFilesTruncated?: boolean
          classifiedTrackedGeneratedArtifactDirtyFileCount?: number
          classifiedTrackedGeneratedArtifactDirtyFiles?: string[]
          classifiedTrackedGeneratedArtifactDirtyFilesTruncated?: boolean
          untrackedGeneratedArtifactDirtyFileCount?: number
          untrackedGeneratedArtifactDirtyFiles?: string[]
          untrackedGeneratedArtifactDirtyFilesTruncated?: boolean
          nonGoalScopeDirtyFileCount?: number
          nonGoalScopeDirtyFiles?: string[]
          nonGoalScopeDirtyFilesTruncated?: boolean
          unclassifiedNonGoalScopeDirtyFileCount?: number
          unclassifiedNonGoalScopeDirtyFiles?: string[]
          unclassifiedNonGoalScopeDirtyFilesTruncated?: boolean
          classifiedNonGoalScopeDirtyFileCount?: number
          classifiedNonGoalScopeDirtyFiles?: string[]
          classifiedNonGoalScopeDirtyFilesTruncated?: boolean
          classifiedDirtyFileReasons?: Array<Record<string, unknown>>
          classifiedDirtyFileReasonsTruncated?: boolean
        }>
      }
      operatorDependencySummary: Record<string, unknown>
      operatorCurrentEnvGate: Record<string, unknown>
      liveEvidence: {
        status: string
        liveOrActualEvidenceUsed: boolean
        deterministicEvidenceOnly: boolean
        llmAnswerQualityEvidenceUsed: boolean
        missingExternalInputIds: string[]
        operatorDependencySummary: Record<string, unknown>
        nextEvidenceActions: {
          credentialSecretsRecorded: boolean
          valuesRecorded: boolean
          actions: Array<{
            id: string
            command?: string
            evidencePath?: string
            evidenceRefreshRequired?: boolean
            evidenceRefreshCommand?: string
            defaultReadinessCommand?: string
          }>
        }
      }
      worktree: {
        status: string
        dirtyFileCount: number
        trackedDirtyFileCount: number
        untrackedFileCount: number
        runtimeScopeDirtyFileCount: number
        repoHygieneDirtyFileCount: number
        goalScopeDirtyFileCount: number
        nonRuntimeScopeDirtyFileCount: number
        nonGoalScopeDirtyFileCount: number
        unclassifiedNonGoalScopeDirtyFileCount: number
        classifiedNonGoalScopeDirtyFileCount: number
        runtimeScopeDirtyFiles: string[]
        runtimeScopeDirtyFilesTruncated: boolean
        repoHygieneDirtyFiles: string[]
        repoHygieneDirtyFilesTruncated: boolean
        nonGoalScopeDirtyFiles: string[]
        nonGoalScopeDirtyFilesTruncated: boolean
        generatedArtifactDirtyFileCount: number
        unclassifiedGeneratedArtifactDirtyFileCount: number
        classifiedGeneratedArtifactDirtyFileCount: number
        generatedArtifactDirtyFiles: string[]
        generatedArtifactDirtyFilesTruncated: boolean
        trackedGeneratedArtifactDirtyFileCount: number
        unclassifiedTrackedGeneratedArtifactDirtyFileCount: number
        classifiedTrackedGeneratedArtifactDirtyFileCount: number
        trackedGeneratedArtifactDirtyFiles: string[]
        trackedGeneratedArtifactDirtyFilesTruncated: boolean
        untrackedGeneratedArtifactDirtyFileCount: number
        untrackedGeneratedArtifactDirtyFiles: string[]
        untrackedGeneratedArtifactDirtyFilesTruncated: boolean
        classifiedDirtyFileReasons: Array<Record<string, unknown>>
        classifiedDirtyFileReasonsTruncated: boolean
        finalAcceptanceRequiresCleanOrClassifiedTree: boolean
      }
      rendererVisibleGoSwitcher: boolean
      retiredGateIds: string[]
      credentialedGateIds: string[]
      checks: Array<{ id: string; status: string; command: string }>
    }

    try {
      expect(preflight.status).toBe(0)
      expect(report).toEqual(expect.objectContaining({
        id: 'runtime-go-preflight',
        status: 'skipped',
        localChecksStatus: 'skipped',
        gateMode: false,
        dryRunCannotAuthorizeCutover: true,
        expectedBlocked: true,
        finalGateBlocked: true,
        rendererVisibleGoSwitcher: false
      }))
      expect(report.finalGateBlockers).toEqual(expect.arrayContaining([
        'dry-run-cannot-authorize-cutover',
        'provider:at-least-one-non-deepseek-provider',
        'operator:gate'
      ]))
      expect(report.finalGateNextActions).toEqual(expect.objectContaining({
        credentialSecretsRecorded: false,
        valuesRecorded: false
      }))
      expect(report.finalGateNextActions.actions).toEqual(expect.arrayContaining([
        expect.objectContaining({
          id: 'dry-run-cannot-authorize-cutover',
          command: 'npm run runtime:go:preflight -- --json --gate'
        }),
        expect.objectContaining({
          id: 'provider:at-least-one-non-deepseek-provider',
          command: expect.stringContaining('runtime:go:live-validation'),
          candidates: expect.arrayContaining([
            expect.objectContaining({
              id: 'openai-compatible',
              missingEnv: expect.arrayContaining(['ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY'])
            })
          ])
        }),
        expect.objectContaining({
          id: 'operator:gate',
          command: expect.stringContaining('runtime:go:default-readiness-report'),
          evidenceRefreshRequired: false,
          evidenceRefreshCommand: expect.stringContaining('runtime:go:live-evidence'),
          defaultReadinessCommand: expect.stringContaining('runtime:go:default-readiness-report'),
          evidenceEnvGate: {
            runtimeReady: true,
            operatorApprovesDefault: true
          },
          dependencySummary: expect.objectContaining({
            envGate: {
              runtimeReady: true,
              operatorApprovesDefault: true
            },
            evidence: expect.objectContaining({
              providerPassed: false,
              mcpPassed: true,
              packagedPassed: true,
              credentialedEvidenceReviewed: false
            }),
            blockers: expect.arrayContaining([
              'provider matrix evidence is not passed'
            ])
          }),
          currentEnvGate: {
            runtimeReady: false,
            operatorApprovesDefault: false
          }
        })
      ]))
      expect(report.operatorCurrentEnvGate).toEqual({
        runtimeReady: false,
        operatorApprovesDefault: false
      })
      expect(report.operatorDependencySummary).toEqual(expect.objectContaining({
        envGate: {
          runtimeReady: true,
          operatorApprovesDefault: true
        },
        commitBinding: expect.objectContaining({
          required: false,
          bound: true
        })
      }))
      expect(report.finalGateBlockerSummary).toEqual(expect.objectContaining({
        requiresExternalInput: true,
        requiresNonDeepSeekProvider: true,
        providerGateDetail: expect.objectContaining({
          present: true,
          candidateProviderIds: ['openai-compatible'],
          credentialedDeepSeekPassed: true,
          credentialedNonDeepSeekProviderIds: [],
          requiresNonDeepSeekProvider: true,
          settingsProfileCoverage: expect.objectContaining({
            providerCount: 1,
            usableProviderIds: ['deepseek'],
            usableNonDeepSeekProviderIds: [],
            nonDeepSeekSettingsProfileUsable: false,
            satisfiesFinalProviderCoverageFromSettings: false,
            credentialValuesRecorded: false
          }),
          candidates: expect.arrayContaining([
            expect.objectContaining({
              id: 'openai-compatible',
              missingEnv: expect.arrayContaining([
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY',
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL',
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL'
              ]),
              missingSettingsProfileFields: ['provider.providers[].matchingProfile']
            })
          ])
        }),
        requiresOperatorApproval: false,
        requiresOperatorEvidenceRefresh: false,
        operatorBlockedByDependencies: true,
        operatorDependencyBlockers: ['provider matrix evidence is not passed'],
        operatorGateDetail: expect.objectContaining({
          present: true,
          blockedByDependencies: true,
          requiresOperatorApproval: false,
          evidenceEnvGate: {
            runtimeReady: true,
            operatorApprovesDefault: true
          },
          currentEnvGate: {
            runtimeReady: false,
            operatorApprovesDefault: false
          }
        })
      }))
      expect(report.liveEvidence).toEqual(expect.objectContaining({
        status: 'live_blocked',
        liveOrActualEvidenceUsed: true,
        deterministicEvidenceOnly: false,
        llmAnswerQualityEvidenceUsed: false,
        missingExternalInputIds: expect.arrayContaining([
          'provider:at-least-one-non-deepseek-provider',
          'operator:gate'
        ])
      }))
      expect(report.liveEvidence.operatorDependencySummary).toEqual(expect.objectContaining({
        envGate: {
          runtimeReady: true,
          operatorApprovesDefault: true
        }
      }))
      expect(report.liveEvidence.nextEvidenceActions).toEqual(expect.objectContaining({
        credentialSecretsRecorded: false,
        valuesRecorded: false,
        actions: expect.arrayContaining([
          expect.objectContaining({
            id: 'provider:at-least-one-non-deepseek-provider',
            command: expect.stringContaining('runtime:go:live-validation')
          }),
          expect.objectContaining({
            id: 'operator:gate',
            command: expect.stringContaining('runtime:go:default-readiness-report'),
            evidenceRefreshRequired: false,
            evidenceRefreshCommand: expect.stringContaining('runtime:go:live-evidence'),
            defaultReadinessCommand: expect.stringContaining('runtime:go:default-readiness-report'),
            dependencySummary: expect.objectContaining({
              evidence: expect.objectContaining({
                providerPassed: false,
                mcpPassed: true
              })
            })
          })
        ])
      }))
      expect(JSON.stringify(report.finalGateNextActions)).not.toMatch(/\bsk-[A-Za-z0-9_-]{8,}\b/)
      expect(report.worktree).toEqual(expect.objectContaining({
        status: expect.stringMatching(/^(clean|dirty)$/),
        finalAcceptanceRequiresCleanOrClassifiedTree: true
      }))
      expect(report.worktree.dirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.trackedDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.untrackedFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.runtimeScopeDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.repoHygieneDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.goalScopeDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.nonRuntimeScopeDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.nonGoalScopeDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.unclassifiedNonGoalScopeDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.classifiedNonGoalScopeDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.generatedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.unclassifiedGeneratedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.classifiedGeneratedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.trackedGeneratedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.unclassifiedTrackedGeneratedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.classifiedTrackedGeneratedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      expect(report.worktree.untrackedGeneratedArtifactDirtyFileCount).toBeGreaterThanOrEqual(0)
      const worktreeAction = report.finalGateNextActions.actions.find((item) =>
        item.id === 'worktree:clean-or-classify')
      if (report.worktree.status === 'dirty') {
        expect(worktreeAction).toEqual(expect.objectContaining({
          id: 'worktree:clean-or-classify',
          trackedGeneratedArtifactDirtyFileCount: report.worktree.trackedGeneratedArtifactDirtyFileCount,
          trackedGeneratedArtifactDirtyFiles: report.worktree.trackedGeneratedArtifactDirtyFiles,
          trackedGeneratedArtifactDirtyFilesTruncated: report.worktree.trackedGeneratedArtifactDirtyFilesTruncated,
          unclassifiedTrackedGeneratedArtifactDirtyFileCount: report.worktree.unclassifiedTrackedGeneratedArtifactDirtyFileCount,
          classifiedTrackedGeneratedArtifactDirtyFileCount: report.worktree.classifiedTrackedGeneratedArtifactDirtyFileCount,
          untrackedGeneratedArtifactDirtyFileCount: report.worktree.untrackedGeneratedArtifactDirtyFileCount,
          untrackedGeneratedArtifactDirtyFiles: report.worktree.untrackedGeneratedArtifactDirtyFiles,
          untrackedGeneratedArtifactDirtyFilesTruncated: report.worktree.untrackedGeneratedArtifactDirtyFilesTruncated,
          generatedArtifactDirtyFileCount: report.worktree.generatedArtifactDirtyFileCount,
          generatedArtifactDirtyFiles: report.worktree.generatedArtifactDirtyFiles,
          generatedArtifactDirtyFilesTruncated: report.worktree.generatedArtifactDirtyFilesTruncated,
          unclassifiedGeneratedArtifactDirtyFileCount: report.worktree.unclassifiedGeneratedArtifactDirtyFileCount,
          classifiedGeneratedArtifactDirtyFileCount: report.worktree.classifiedGeneratedArtifactDirtyFileCount,
          nonGoalScopeDirtyFileCount: report.worktree.nonGoalScopeDirtyFileCount,
          nonGoalScopeDirtyFiles: report.worktree.nonGoalScopeDirtyFiles,
          nonGoalScopeDirtyFilesTruncated: report.worktree.nonGoalScopeDirtyFilesTruncated,
          unclassifiedNonGoalScopeDirtyFileCount: report.worktree.unclassifiedNonGoalScopeDirtyFileCount,
          classifiedNonGoalScopeDirtyFileCount: report.worktree.classifiedNonGoalScopeDirtyFileCount,
          classifiedDirtyFileReasons: report.worktree.classifiedDirtyFileReasons,
          classifiedDirtyFileReasonsTruncated: report.worktree.classifiedDirtyFileReasonsTruncated,
          remediation: expect.objectContaining({
            destructiveCleanupSuggested: false
          })
        }))
      }
      if (report.worktree.unclassifiedNonGoalScopeDirtyFileCount > 0) {
        expect(report.finalGateBlockers).toContain('worktree:non-goal-dirty-files')
        expect(worktreeAction).toEqual(expect.objectContaining({
          status: 'required',
          blockerIds: expect.arrayContaining(['worktree:non-goal-dirty-files']),
          remediation: expect.objectContaining({
            requiresOperatorReview: true,
            guidance: expect.stringContaining('Classify')
          })
        }))
      }
      if (report.worktree.unclassifiedGeneratedArtifactDirtyFileCount > 0) {
        expect(report.finalGateBlockers).toContain('worktree:generated-artifacts')
        expect(worktreeAction).toEqual(expect.objectContaining({
          status: 'required',
          blockerIds: expect.arrayContaining(['worktree:generated-artifacts'])
        }))
      }
      if (report.worktree.unclassifiedTrackedGeneratedArtifactDirtyFileCount > 0) {
        expect(report.finalGateBlockers).toContain('worktree:tracked-generated-artifacts')
        expect(worktreeAction).toEqual(expect.objectContaining({
          status: 'required',
          blockerIds: expect.arrayContaining(['worktree:tracked-generated-artifacts'])
        }))
      }
      expect(report.worktree.runtimeScopeDirtyFiles.length).toBeLessThanOrEqual(20)
      expect(typeof report.worktree.runtimeScopeDirtyFilesTruncated).toBe('boolean')
      expect(report.worktree.repoHygieneDirtyFiles.length).toBeLessThanOrEqual(20)
      expect(typeof report.worktree.repoHygieneDirtyFilesTruncated).toBe('boolean')
      expect(report.worktree.nonGoalScopeDirtyFiles.length).toBeLessThanOrEqual(20)
      expect(typeof report.worktree.nonGoalScopeDirtyFilesTruncated).toBe('boolean')
      expect(report.worktree.generatedArtifactDirtyFiles.length).toBeLessThanOrEqual(20)
      expect(typeof report.worktree.generatedArtifactDirtyFilesTruncated).toBe('boolean')
      expect(report.worktree.trackedGeneratedArtifactDirtyFiles.length).toBeLessThanOrEqual(20)
      expect(typeof report.worktree.trackedGeneratedArtifactDirtyFilesTruncated).toBe('boolean')
      expect(report.worktree.untrackedGeneratedArtifactDirtyFiles.length).toBeLessThanOrEqual(20)
      expect(typeof report.worktree.untrackedGeneratedArtifactDirtyFilesTruncated).toBe('boolean')
      expect(report.retiredGateIds).toEqual(expect.arrayContaining([
        'runtime-go-product-regression',
        'runtime-go-speed-cache-gate',
        'runtime-go-preflight'
      ]))
      expect(report.credentialedGateIds).toEqual(expect.arrayContaining([
        'provider-matrix-credentialed',
        'mcp-matrix-credentialed'
      ]))
      expect(report.checks).toEqual(expect.arrayContaining([
        expect.objectContaining({ id: 'product-regression', status: 'skipped' }),
        expect.objectContaining({ id: 'speed-cache-gate', status: 'skipped' }),
        expect.objectContaining({ id: 'typecheck', status: 'skipped' }),
        expect.objectContaining({ id: 'build-runtime', status: 'skipped' }),
        expect.objectContaining({ id: 'diff-check', status: 'skipped' })
      ]))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('fails preflight gate mode while final live evidence is still blocked', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-gate-'))
    const liveEvidence = join(dir, 'live-evidence-report.json')
    writeFileSync(liveEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-live-evidence-report',
      status: 'live_blocked',
      passed: false,
      liveOrActualEvidenceUsed: true,
      deterministicEvidenceOnly: false,
      llmAnswerQualityEvidenceUsed: false,
      componentStatus: {
        provider: 'live_blocked',
        mcp: 'passed',
        packaged: 'passed',
        operator: 'live_blocked'
      },
      missingExternalInputs: [
        { id: 'provider:at-least-one-non-deepseek-provider', reason: 'missing credential' },
        { id: 'operator:gate', reason: 'operator approval missing' }
      ]
    }), 'utf8')
    try {
      const preflight = spawnSync(
        process.execPath,
        ['scripts/runtime-go-preflight.mjs', '--dry-run', '--json', '--gate', '--live-evidence-json', liveEvidence],
        { cwd: process.cwd(), encoding: 'utf8' }
      )
      const report = JSON.parse(preflight.stdout) as {
        status: string
        localChecksStatus: string
        gateMode: boolean
        finalGateBlocked: boolean
        finalGateBlockers: string[]
      }

      expect(preflight.status).toBe(1)
      expect(report).toEqual(expect.objectContaining({
        status: 'live_blocked',
        localChecksStatus: 'skipped',
        gateMode: true,
        finalGateBlocked: true
      }))
      expect(report.finalGateBlockers).toEqual(expect.arrayContaining([
        'dry-run-cannot-authorize-cutover',
        'provider:at-least-one-non-deepseek-provider',
        'operator:gate'
      ]))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('blocks preflight when live evidence component digests are stale', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-freshness-'))
    const providerEvidence = join(dir, 'provider-matrix.json')
    const liveEvidence = join(dir, 'live-evidence-report.json')
    writeFileSync(providerEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-provider-matrix',
      status: 'passed',
      passed: true
    }), 'utf8')
    writeFileSync(liveEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-live-evidence-report',
      status: 'passed',
      passed: true,
      liveOrActualEvidenceUsed: true,
      deterministicEvidenceOnly: false,
      llmAnswerQualityEvidenceUsed: false,
      componentStatus: {
        provider: 'passed',
        mcp: 'passed',
        packaged: 'passed',
        packagedGui: 'passed',
        packagedSoak: 'passed',
        operator: 'passed'
      },
      evidencePaths: {
        provider: providerEvidence
      },
      evidenceDigests: {
        provider: '0'.repeat(64)
      },
      missingExternalInputs: []
    }), 'utf8')

    try {
      const preflight = spawnSync(
        process.execPath,
        ['scripts/runtime-go-preflight.mjs', '--skip-commands', '--json', '--live-evidence-json', liveEvidence],
        { cwd: process.cwd(), encoding: 'utf8' }
      )
      const report = JSON.parse(preflight.stdout) as {
        finalGateBlocked: boolean
        finalGateBlockers: string[]
        liveEvidence: {
          freshness: {
            status: string
            digestStatus: string
            digestMismatches: string[]
          }
        }
      }

      expect(preflight.status).toBe(0)
      expect(report.finalGateBlocked).toBe(true)
      expect(report.finalGateBlockers).toEqual(expect.arrayContaining([
        'live-evidence-digest-mismatch'
      ]))
      expect(report.liveEvidence.freshness).toEqual(expect.objectContaining({
        status: 'failed',
        digestStatus: 'failed',
        digestMismatches: ['provider']
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('accepts live evidence bound to a source commit when HEAD only adds evidence files', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-evidence-only-'))
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    const providerEvidence = join(dir, 'provider-matrix.json')
    const liveEvidence = join(dir, 'live-evidence-report.json')
    const sourceCommit = '1111111111111111111111111111111111111111'
    const evidenceCommit = '2222222222222222222222222222222222222222'
    const providerReport = {
      schemaVersion: 1,
      id: 'runtime-go-provider-matrix',
      status: 'passed',
      passed: true
    }
    writeFileSync(providerEvidence, JSON.stringify(providerReport), 'utf8')
    writeFileSync(liveEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-live-evidence-report',
      status: 'passed',
      passed: true,
      sourceCommit,
      liveOrActualEvidenceUsed: true,
      deterministicEvidenceOnly: false,
      llmAnswerQualityEvidenceUsed: false,
      componentStatus: {
        provider: 'passed',
        mcp: 'passed',
        packaged: 'passed',
        packagedGui: 'passed',
        packagedSoak: 'passed',
        operator: 'passed'
      },
      evidencePaths: {
        provider: providerEvidence
      },
      evidenceDigests: {
        provider: sha256(providerReport)
      },
      missingExternalInputs: []
    }), 'utf8')
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'status' && args[1] === '--porcelain=v1') process.exit(0)
if (args[0] === 'rev-parse' && args[1] === 'HEAD') {
  console.log('${evidenceCommit}')
  process.exit(0)
}
if (args[0] === 'merge-base' && args[1] === '--is-ancestor' && args[2] === '${sourceCommit}' && args[3] === '${evidenceCommit}') {
  process.exit(0)
}
if (args[0] === 'diff' && args[1] === '--name-only' && args[2] === '${sourceCommit}..${evidenceCommit}') {
  console.log([
    'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json',
    'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-summary.md'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const preflight = spawnSync(
        process.execPath,
        ['scripts/runtime-go-preflight.mjs', '--skip-commands', '--json', '--live-evidence-json', liveEvidence],
        { cwd: process.cwd(), encoding: 'utf8' }
      )
      const report = JSON.parse(preflight.stdout) as {
        finalGateBlockers: string[]
        liveEvidence: {
          freshness: {
            status: string
            sourceCommitStatus: string
            sourceCommit: string
            currentCommit: string
          }
        }
      }

      expect(preflight.status).toBe(0)
      expect(report.finalGateBlockers).not.toContain('live-evidence-source-commit-stale')
      expect(report.liveEvidence.freshness).toEqual(expect.objectContaining({
        status: 'passed',
        sourceCommitStatus: 'evidence-only-commit',
        sourceCommit,
        currentCommit: evidenceCommit
      }))
    } finally {
      process.env.PATH = originalPath
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('blocks preflight when passed live evidence is structurally incomplete', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-incomplete-evidence-'))
    const liveEvidence = join(dir, 'live-evidence-report.json')
    writeFileSync(liveEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-live-evidence-report',
      status: 'passed',
      passed: true
    }), 'utf8')

    try {
      const preflight = spawnSync(
        process.execPath,
        ['scripts/runtime-go-preflight.mjs', '--skip-commands', '--json', '--live-evidence-json', liveEvidence],
        { cwd: process.cwd(), encoding: 'utf8' }
      )
      const report = JSON.parse(preflight.stdout) as {
        finalGateBlocked: boolean
        finalGateBlockers: string[]
        liveEvidence: {
          completeness: {
            status: string
            blockers: string[]
          }
        }
      }

      expect(preflight.status).toBe(0)
      expect(report.finalGateBlocked).toBe(true)
      expect(report.finalGateBlockers).toEqual(expect.arrayContaining([
        'live-evidence-report-incomplete'
      ]))
      expect(report.liveEvidence.completeness).toEqual(expect.objectContaining({
        status: 'failed',
        blockers: expect.arrayContaining([
          'sourceCommit',
          'evidenceDigests',
          'liveOrActualEvidenceUsed',
          'componentStatus.provider',
          'componentStatus.operator'
        ])
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('does not turn explicit live-blocked evidence into an incomplete-report blocker', () => {
    const dir = mkdtempSync(join(tmpdir(), 'analytix-preflight-live-blocked-evidence-'))
    const liveEvidence = join(dir, 'live-evidence-report.json')
    writeFileSync(liveEvidence, JSON.stringify({
      schemaVersion: 1,
      id: 'runtime-go-live-evidence-report',
      status: 'live_blocked',
      passed: false,
      liveOrActualEvidenceUsed: true,
      deterministicEvidenceOnly: false,
      missingExternalInputs: [
        { id: 'provider:at-least-one-non-deepseek-provider' },
        { id: 'operator:gate' }
      ],
      componentStatus: {
        provider: 'live_blocked',
        mcp: 'passed',
        packaged: 'passed',
        packagedGui: 'passed',
        packagedSoak: 'passed',
        operator: 'live_blocked'
      }
    }), 'utf8')

    try {
      const preflight = spawnSync(
        process.execPath,
        ['scripts/runtime-go-preflight.mjs', '--skip-commands', '--json', '--live-evidence-json', liveEvidence],
        { cwd: process.cwd(), encoding: 'utf8' }
      )
      const report = JSON.parse(preflight.stdout) as {
        finalGateBlocked: boolean
        finalGateBlockers: string[]
        liveEvidence: {
          completeness: {
            status: string
            blockers: string[]
          }
          finalGateBlockers: string[]
        }
      }

      expect(preflight.status).toBe(0)
      expect(report.finalGateBlocked).toBe(true)
      expect(report.finalGateBlockers).toEqual(expect.arrayContaining([
        'provider:at-least-one-non-deepseek-provider',
        'operator:gate'
      ]))
      expect(report.finalGateBlockers).not.toContain('live-evidence-report-incomplete')
      expect(report.liveEvidence.finalGateBlockers).not.toContain('live-evidence-report-incomplete')
      expect(report.liveEvidence.completeness).toEqual(expect.objectContaining({
        status: 'skipped',
        blockers: []
      }))
    } finally {
      rmSync(dir, { recursive: true, force: true })
    }
  })

  it('keeps the public runtime validation wrapper pointed at formal runtime-go scripts only', () => {
    const wrapper = readFileSync(join(process.cwd(), 'scripts/runtime-go-validation-command.mjs'), 'utf8')
    const packageJson = JSON.parse(readFileSync(join(process.cwd(), 'package.json'), 'utf8')) as {
      scripts: Record<string, string>
    }

    expect(wrapper).toContain("'product-regression': { script: 'runtime-go-product-regression.mjs' }")
    expect(wrapper).toContain("'speed-cache-gate': { script: 'runtime-go-speed-cache-gate.mjs' }")
    expect(wrapper).toContain("'packaged-qa': { script: 'runtime-go-packaged-qa.mjs' }")
    expect(wrapper).not.toMatch(/d0(?:24|25)[a-z0-9_-]*\.mjs/i)
    for (const [name, command] of Object.entries(packageJson.scripts)) {
      if (name === 'qa:runtime:packaged' || name.startsWith('runtime:go:')) {
        expect(command).toContain('scripts/runtime-go-validation-command.mjs')
        expect(`${name}: ${command}`).not.toMatch(/\b(?:d0(?:24|25)[a-z0-9_-]*|proof|fixture|fake|mcpLocalProof)\b/i)
      }
    }
  })
})
