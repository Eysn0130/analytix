#!/usr/bin/env node
import { spawn, type ChildProcess } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'
import process from 'node:process'
import {
  parseServeOptionsSafe,
  validateServeOptions,
  SERVE_USAGE,
  ServeExitCode
} from './serve.js'
import type { ServeOptions } from './cli-options.js'

export const ANALYTIX_READY_PREFIX = 'ANALYTIX_READY '
const GO_RUNTIME_SERVER_READY_PREFIX = 'ANALYTIX_RUNTIME_SERVER_READY '

export type AnalytixServeReadyPublicV1 = Readonly<{
  service: 'analytix'
  mode: 'serve'
  port: number
}>

export function projectAnalytixServeReadyPublicV1(
  source: Readonly<Record<string, unknown>>
): AnalytixServeReadyPublicV1 {
  const port = source.port
  if (typeof port !== 'number' || !Number.isSafeInteger(port) || port < 1 || port > 65_535) {
    throw new Error('Analytix serve public readiness requires a valid bound port.')
  }
  return Object.freeze({ service: 'analytix', mode: 'serve', port })
}

type AnalytixServeHandle = {
  readonly child: ChildProcess
  readonly host: string
  readonly port: number
  readonly ready: GoRuntimeReadyPayload
  close: () => Promise<void>
}

type GoRuntimeReadyPayload = {
  url: string
  runtimePid: number
  runtimeTokenConfigured: boolean
  persistenceRootsConfigured: boolean
  productionRuntime: boolean
  controlledArtifactHostV2Configured: false
  controlledArtifactHostV2Ready: false
  controlledArtifactHostV2BackendGeneration: 0
  controlledArtifactHostV2LaunchBindingProof: ''
  finalPublicationAuthorityKeyId: string
  finalPublicationAuthorityPublicKey: string
  witnessedAuthorityV2Configured: false
  witnessedAuthorityInstallationId: ''
  witnessedAuthorityKeyId: ''
  witnessedAuthorityManifestDigest: ''
  datasetSnapshotSelectionV2Configured: false
  datasetSnapshotAdmissionV2State: 'absent'
  datasetSnapshotAdmissionV2InstallationId: ''
  datasetSnapshotAdmissionV2RuntimeLaunchNonce: ''
  datasetSnapshotAdmissionV2StagingBindingDigest: ''
  datasetSnapshotAdmissionV2SelectionDigest: ''
  datasetSnapshotAdmissionV2SnapshotId: ''
  datasetSnapshotAdmissionV2AuthorityRecordDigest: ''
  datasetSnapshotAdmissionV2AckHmacSha256: ''
}

type GoRuntimeLaunchTarget =
  | {
      kind: 'binary'
      command: string
      argsPrefix: string[]
      cwd?: string
    }
  | {
      kind: 'go-run'
      command: string
      argsPrefix: string[]
      cwd: string
    }

/**
 * Serve mode runs unattended under the GUI. An uncaught error must not
 * leave a half-dead process: report it on stderr (the GUI captures the
 * tail), attempt a bounded graceful close, then exit non-zero so the
 * GUI supervisor can restart us.
 */
function installServeCrashHandlers(getHandle: () => AnalytixServeHandle | null): void {
  let crashing = false
  const crash = (_kind: string, _error: unknown): void => {
    if (crashing) return
    crashing = true
    process.stderr.write('[analytix] event=ANALYTIX_SERVE_CRASHED\n')
    const finish = (): void => process.exit(ServeExitCode.runtime)
    const handle = getHandle()
    if (!handle) {
      finish()
      return
    }
    const deadline = setTimeout(finish, 3000)
    deadline.unref()
    void handle
      .close()
      .catch(() => undefined)
      .finally(finish)
  }
  process.on('uncaughtException', (error) => crash('uncaughtException', error))
  process.on('unhandledRejection', (reason) => crash('unhandledRejection', reason))
}

/**
 * Serve-mode command. Kept separate from the dispatcher so GUI startup
 * still has the exact same ANALYTIX_READY handshake behavior.
 */
async function serveMain(argv: readonly string[]): Promise<number> {
  if (argv.length === 0 || argv.includes('--help') || argv.includes('-h')) {
    process.stdout.write(SERVE_USAGE)
    return ServeExitCode.ok
  }
  const parsed = parseServeOptionsSafe(argv, process.env)
  if (!parsed.ok) {
    process.stderr.write(`analytix serve: ${parsed.message}\n`)
    if (parsed.issues) {
      process.stderr.write(`${JSON.stringify(parsed.issues, null, 2)}\n`)
    }
    return parsed.exitCode
  }
  let handle: AnalytixServeHandle | null = null
  installServeCrashHandlers(() => handle)
  const server = await startGoRuntimeServe(parsed.options, process.env)
  handle = server
  const publicReady = projectAnalytixServeReadyPublicV1({
    port: server.port,
  })
  process.stdout.write(`${ANALYTIX_READY_PREFIX}${JSON.stringify(publicReady)}\n`)
  process.stdout.write(JSON.stringify(publicReady, null, 2) + '\n')
  if (parsed.options.insecure) {
    process.stderr.write('analytix serve: warning: runtime authentication is disabled (--insecure).\n')
  }
  await new Promise<void>((resolve) => {
    const stop = () => {
      void server.close().finally(resolve)
    }
    process.once('SIGTERM', stop)
    process.once('SIGINT', stop)
  })
  return ServeExitCode.ok
}

export async function startGoRuntimeServe(
  options: ServeOptions,
  env: NodeJS.ProcessEnv
): Promise<AnalytixServeHandle> {
  const ambientModelProviders = parseAmbientModelProviders(env.ANALYTIX_MODEL_PROVIDERS)
  if (ambientModelProviders !== undefined) {
    validateServeOptions({ ...options, apiKey: '', modelProviders: ambientModelProviders })
  }
  const launchOptions = validateServeOptions({
    ...options,
    apiKey: firstNonEmpty(options.apiKey, env.ANALYTIX_API_KEY),
    modelProviders: options.modelProviders ?? ambientModelProviders
  })
  if (!launchOptions.insecure && !(launchOptions.runtimeToken || env.ANALYTIX_RUNTIME_TOKEN)) {
    throw new Error('A runtime token is required unless --insecure is explicitly enabled.')
  }
  const target = resolveGoRuntimeLaunchTarget(env)
  const childEnv: NodeJS.ProcessEnv = {
    ...env
  }
  const reservedNames = new Set([
    'ANALYTIX_API_KEY',
    'ANALYTIX_MODEL_PROVIDERS',
    'ANALYTIX_HUB_TEST_GATEWAY_TOKEN',
    'ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN',
    'ANALYTIX_RUNTIME_TOKEN'
  ])
  // Apply the existing child boundary before Windows collapses case aliases;
  // only the requested runtime token may survive that collapse.
  for (const name of Object.keys(childEnv)) {
    if (reservedNames.has(name.toUpperCase())) delete childEnv[name]
  }
  childEnv.ANALYTIX_RUNTIME_TOKEN = launchOptions.runtimeToken || env.ANALYTIX_RUNTIME_TOKEN || ''
  const child = spawn(target.command, [
    ...target.argsPrefix,
    ...goRuntimeServeArgs(launchOptions)
  ], {
    cwd: target.cwd,
    env: childEnv,
    stdio: ['ignore', 'pipe', 'pipe']
  })

  let ready: GoRuntimeReadyPayload
  try {
    ready = await waitForGoRuntimeReady(child)
  } catch (error) {
    await stopGoRuntimeChild(child)
    throw error
  }
  const expectedTokenConfigured = Boolean(launchOptions.runtimeToken || env.ANALYTIX_RUNTIME_TOKEN) || !launchOptions.insecure
  if (ready.runtimeTokenConfigured !== expectedTokenConfigured) {
    await stopGoRuntimeChild(child)
    throw new Error('Go runtime-server token configuration does not match the requested mode.')
  }
  if (!ready.persistenceRootsConfigured || !ready.productionRuntime) {
    await stopGoRuntimeChild(child)
    throw new Error('Go runtime-server did not prove production persistence readiness.')
  }
  if (!Number.isSafeInteger(child.pid) || Number(child.pid) <= 0 || ready.runtimePid !== child.pid) {
    await stopGoRuntimeChild(child)
    throw new Error('Go runtime-server ready process identity does not match the spawned runtime.')
  }
  if (launchOptions.port > 0 && portFromReadyUrl(ready.url) !== launchOptions.port) {
    await stopGoRuntimeChild(child)
    throw new Error('Go runtime-server ready endpoint does not match the requested port.')
  }
  return {
    child,
    host: launchOptions.host,
    port: portFromReadyUrl(ready.url) ?? launchOptions.port,
    ready,
    close: () => stopGoRuntimeChild(child)
  }
}

function goRuntimeServeArgs(options: ServeOptions): string[] {
  return [
    '--host',
    options.host,
    '--port',
    String(options.port),
    '--data-dir',
    options.dataDir,
    ...(options.configPath ? ['--mcp-config-path', options.configPath] : []),
    '--approval-policy',
    options.approvalPolicy,
    '--sandbox-mode',
    options.sandboxMode,
    '--token-economy-mode',
    options.tokenEconomyMode ? 'true' : 'false',
    ...(options.insecure ? ['--insecure'] : [])
  ]
}

export function resolveGoRuntimeLaunchTarget(env: NodeJS.ProcessEnv): GoRuntimeLaunchTarget {
  const binaryOverride = firstNonEmpty(
    env.ANALYTIX_RUNTIME_SERVER_BINARY,
    env.ANALYTIX_RUNTIME_GO_SERVER
  )
  if (binaryOverride) {
    return {
      kind: 'binary',
      command: binaryOverride,
      argsPrefix: parseArgsJSON(env.ANALYTIX_RUNTIME_SERVER_BINARY_ARGS_JSON)
    }
  }

  const binaryName = process.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server'
  for (const candidate of runtimeServerBinaryCandidates(env, binaryName)) {
    if (existsSync(candidate)) return { kind: 'binary', command: candidate, argsPrefix: [] }
  }

  const runtimeGoDir = resolveRuntimeGoDir(env)
  if (runtimeGoDir) {
    return {
      kind: 'go-run',
      command: env.GO_BINARY || 'go',
      argsPrefix: ['run', '-tags', 'analytix_prod', './cmd/runtime-server'],
      cwd: runtimeGoDir
    }
  }

  throw new Error('Go runtime-server binary is missing and packages/runtime-go source was not found.')
}

function runtimeServerBinaryCandidates(env: NodeJS.ProcessEnv, binaryName: string): string[] {
  const here = dirname(fileURLToPath(import.meta.url))
  const roots = [
    env.ANALYTIX_APP_ROOT,
    process.cwd(),
    resolve(here, '..', '..', '..'),
    resolve(here, '..', '..', '..', '..')
  ].filter((item): item is string => Boolean(item && item.trim()))
  const candidates: string[] = []
  for (const root of roots) {
    candidates.push(join(root, 'runtime-go', 'bin', binaryName))
    candidates.push(join(root, 'packages', 'runtime-go', 'bin', binaryName))
  }
  return candidates
}

function resolveRuntimeGoDir(env: NodeJS.ProcessEnv): string | null {
  const here = dirname(fileURLToPath(import.meta.url))
  const candidates = [
    env.ANALYTIX_RUNTIME_GO_DIR,
    join(process.cwd(), 'packages', 'runtime-go'),
    resolve(here, '..', '..', '..', 'runtime-go'),
    resolve(here, '..', '..', '..', '..', 'packages', 'runtime-go')
  ].filter((item): item is string => Boolean(item && item.trim()))
  return candidates.find((candidate) => existsSync(join(candidate, 'go.mod'))) ?? null
}

function waitForGoRuntimeReady(child: ChildProcess): Promise<GoRuntimeReadyPayload> {
  return new Promise((resolveReady, rejectReady) => {
    if (!child.stdout) {
      rejectReady(new Error('Go runtime-server stdout is unavailable.'))
      return
    }
    child.stderr?.resume()
    let stdout = ''
    let settled = false
    const settle = (fn: () => void): void => {
      if (settled) return
      settled = true
      child.stdout?.off('data', onStdout)
      child.off('exit', onExit)
      fn()
    }
    const onStdout = (chunk: Buffer | string): void => {
      stdout += String(chunk)
      const lines = stdout.split('\n')
      stdout = lines.pop() ?? ''
      for (const line of lines) {
        if (line.startsWith(GO_RUNTIME_SERVER_READY_PREFIX)) {
          settle(() => {
            child.stdout?.resume()
            try {
              const parsed = JSON.parse(line.slice(GO_RUNTIME_SERVER_READY_PREFIX.length)) as unknown
              if (!isGoRuntimeReadyPayload(parsed)) {
                rejectReady(new Error('Go runtime-server emitted an invalid ready payload.'))
                return
              }
              resolveReady(parsed)
            } catch (error) {
              rejectReady(error)
            }
          })
          continue
        }
      }
    }
    const onExit = (code: number | null, signal: NodeJS.Signals | null): void => {
      settle(() => {
        rejectReady(new Error(`Go runtime-server exited before ready with code ${code ?? 'unknown'} signal ${signal ?? 'none'}.`))
      })
    }
    child.stdout.on('data', onStdout)
    child.once('exit', onExit)
  })
}

function isGoRuntimeReadyPayload(value: unknown): value is GoRuntimeReadyPayload {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  const allowed = new Set([
    'url',
    'runtimePid',
    'runtimeTokenConfigured',
    'persistenceRootsConfigured',
    'productionRuntime',
    'controlledArtifactHostV2Configured',
    'controlledArtifactHostV2Ready',
    'controlledArtifactHostV2BackendGeneration',
    'controlledArtifactHostV2LaunchBindingProof',
    'finalPublicationAuthorityKeyId',
    'finalPublicationAuthorityPublicKey',
    'witnessedAuthorityV2Configured',
    'witnessedAuthorityInstallationId',
    'witnessedAuthorityKeyId',
    'witnessedAuthorityManifestDigest',
    'datasetSnapshotSelectionV2Configured',
    'datasetSnapshotAdmissionV2State',
    'datasetSnapshotAdmissionV2InstallationId',
    'datasetSnapshotAdmissionV2RuntimeLaunchNonce',
    'datasetSnapshotAdmissionV2StagingBindingDigest',
    'datasetSnapshotAdmissionV2SelectionDigest',
    'datasetSnapshotAdmissionV2SnapshotId',
    'datasetSnapshotAdmissionV2AuthorityRecordDigest',
    'datasetSnapshotAdmissionV2AckHmacSha256'
  ])
  if (Object.keys(record).some((key) => !allowed.has(key)) || Object.keys(record).length !== allowed.size) {
    return false
  }
  const authorityPublicKey = typeof record.finalPublicationAuthorityPublicKey === 'string'
    ? Buffer.from(record.finalPublicationAuthorityPublicKey, 'base64url')
    : Buffer.alloc(0)
  const authorityValid = typeof record.finalPublicationAuthorityKeyId === 'string' &&
    /^[a-f0-9]{64}$/.test(record.finalPublicationAuthorityKeyId) && authorityPublicKey.length === 32 &&
    authorityPublicKey.toString('base64url') === record.finalPublicationAuthorityPublicKey &&
    createHash('sha256').update(authorityPublicKey).digest('hex') === record.finalPublicationAuthorityKeyId
  const protectedCapabilitiesAbsent =
    record.controlledArtifactHostV2Configured === false &&
    record.controlledArtifactHostV2Ready === false &&
    record.controlledArtifactHostV2BackendGeneration === 0 &&
    record.controlledArtifactHostV2LaunchBindingProof === '' &&
    record.witnessedAuthorityV2Configured === false &&
    record.witnessedAuthorityInstallationId === '' &&
    record.witnessedAuthorityKeyId === '' &&
    record.witnessedAuthorityManifestDigest === '' &&
    record.datasetSnapshotSelectionV2Configured === false &&
    record.datasetSnapshotAdmissionV2State === 'absent' &&
    record.datasetSnapshotAdmissionV2InstallationId === '' &&
    record.datasetSnapshotAdmissionV2RuntimeLaunchNonce === '' &&
    record.datasetSnapshotAdmissionV2StagingBindingDigest === '' &&
    record.datasetSnapshotAdmissionV2SelectionDigest === '' &&
    record.datasetSnapshotAdmissionV2SnapshotId === '' &&
    record.datasetSnapshotAdmissionV2AuthorityRecordDigest === '' &&
    record.datasetSnapshotAdmissionV2AckHmacSha256 === ''
  return (
    typeof record.url === 'string' &&
    isLoopbackRuntimeURL(record.url) &&
    Number.isSafeInteger(record.runtimePid) && Number(record.runtimePid) > 0 &&
    typeof record.runtimeTokenConfigured === 'boolean' &&
    typeof record.persistenceRootsConfigured === 'boolean' &&
    typeof record.productionRuntime === 'boolean' && protectedCapabilitiesAbsent && authorityValid
  )
}

function isLoopbackRuntimeURL(value: string): boolean {
  try {
    const parsed = new URL(value)
    const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
    const port = Number(parsed.port)
    return (
      parsed.protocol === 'http:' &&
      parsed.username === '' &&
      parsed.password === '' &&
      parsed.pathname === '/' &&
      parsed.search === '' &&
      parsed.hash === '' &&
      (hostname === '127.0.0.1' || hostname === '::1' || hostname === 'localhost') &&
      Number.isInteger(port) &&
      port > 0 &&
      port <= 65535
    )
  } catch {
    return false
  }
}

function stopGoRuntimeChild(child: ChildProcess): Promise<void> {
  return new Promise((resolveStop) => {
    if (child.exitCode !== null || child.signalCode !== null) {
      resolveStop()
      return
    }
    const timer = setTimeout(() => {
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL')
    }, 3000)
    timer.unref()
    child.once('exit', () => {
      clearTimeout(timer)
      resolveStop()
    })
    child.kill('SIGTERM')
  })
}

function portFromReadyUrl(url: string): number | null {
  try {
    const parsed = new URL(url)
    const port = Number(parsed.port)
    return Number.isInteger(port) ? port : null
  } catch {
    return null
  }
}

function firstNonEmpty(...values: Array<string | undefined>): string {
  return values.map((value) => value?.trim() ?? '').find(Boolean) ?? ''
}

function parseAmbientModelProviders(value: string | undefined): unknown {
  const trimmed = value?.trim()
  if (!trimmed) return undefined
  try {
    return JSON.parse(trimmed)
  } catch {
    throw new Error('serve Provider metadata is invalid')
  }
}

function parseArgsJSON(value: string | undefined): string[] {
  const trimmed = value?.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed)
    if (Array.isArray(parsed) && parsed.every((item) => typeof item === 'string')) return parsed
  } catch {
    // Fall through to the safe empty default.
  }
  return []
}

export async function main(argv: readonly string[]): Promise<number> {
  const [command, ...rest] = argv
  if (command === undefined || command === '--help' || command === '-h') {
    process.stdout.write(SERVE_USAGE)
    return ServeExitCode.ok
  }
  if (command === 'serve') {
    return serveMain(rest)
  }
  if (command.startsWith('--')) {
    return serveMain(argv)
  }
  process.stderr.write(`analytix: unknown command '${command}'. Only 'analytix serve' is supported.\n`)
  process.stderr.write(SERVE_USAGE)
  return ServeExitCode.usage
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).then(
    (code) => {
      process.exit(code)
    },
    () => {
      process.stderr.write('[analytix] event=ANALYTIX_SERVE_STARTUP_FAILED\n')
      process.exit(ServeExitCode.runtime)
    }
  )
}
