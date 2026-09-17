import { app } from 'electron'
import { execFile, spawn, type ChildProcess } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, lstatSync, readFileSync } from 'node:fs'
import { mkdir, readdir, readFile } from 'node:fs/promises'
import { createServer } from 'node:net'
import { homedir } from 'node:os'
import { basename, dirname, isAbsolute, join, relative, resolve } from 'node:path'
import { promisify } from 'node:util'
import {
  DEFAULT_DEEPSEEK_BASE_URL,
  DEFAULT_MODEL_PROVIDER_ID,
  getModelProviderSettings,
  getModelProviderPreset,
  isAnalytixRuntimeInsecure,
  modelProviderPresetProfile,
  modelCapabilityProbeKey,
  resolveModelProviderProxyUrl,
  resolveAnalytixRuntimeSettings,
  type ModelProviderModelProfileV1,
  type ModelProviderProfileV1,
  type AnalytixSubagentsSettingsV1,
  type AnalytixRuntimeSettingsV1,
  type AppSettingsV1
} from '../shared/app-settings'
import {
  buildAnalytixServeArgs,
  resolveAnalytixExecutable
} from './resolve-analytix-binary'
import {
  AnalytixConfigSchema,
  ModelProvidersConfigSchema,
  RuntimeTuningConfigSchema
} from '../../packages/runtime/src/config/analytix-config.js'
import {
  McpCapabilityConfig,
  McpAccountCredentialScope,
  McpServerIdV1,
  McpServerConfig,
  SkillsCapabilityConfig,
  SubagentsCapabilityConfig,
  VisionBridgeCapabilityConfig,
  WebCapabilityConfig
} from '../../packages/runtime/src/contracts/capabilities.js'
import {
  providerRegistryOAuthBindingSchemaV1,
  type ProviderRegistryOAuthBindingV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import {
  atomicWriteFile,
  ensureFileMode
} from '../../packages/runtime/src/adapters/file/atomic-write.js'
import {
  buildRuntimeHostScheduleMcpBindingV1,
  GUI_SCHEDULE_MCP_SERVER_NAME,
  resolveClawScheduleMcpCommand,
  resolveAnalytixMcpJsonPath,
  type ClawScheduleMcpLaunchConfig,
  type RuntimeHostScheduleMcpBindingV1
} from './claw-schedule-mcp-config'
import { defaultAnalytixDataDir } from './runtime/analytix-adapter'
import type { BundledFundsMaterializationBindingV1 } from './runtime/bundled-funds-materialization'
import { isAnalytixHealthResponseBody } from './analytix-health'
import {
  appendManagedChildLogRecord,
  normalizeManagedChildLogRecord,
  type ManagedChildLogRecord
} from './logger'
import {
  computePluginSourceTreeIdentity,
  readStablePluginSourceFile
} from './plugin-source-integrity'
import { verifyInstalledPluginBindingV1 } from './plugin-install-verifier'
import {
  comparableSkillRootPath,
  guiPluginSkillManagedComparablePrefixes,
  guiSkillManagedComparablePaths,
  guiSkillRootsForRuntime,
  isAnalytixComputerUseSkillRoot,
  isLegacyComputerUseSkillRoot,
  normalizeSkillRootPath
} from './services/skill-service'
import {
  parseStrictJsonObject,
  parseStrictJsonValue
} from './controlled-artifact/strict-json'
import { readPrivateRegularFile } from './controlled-artifact/private-regular-file'

let child: ChildProcess | null = null
let childLogCapture: AnalytixChildLogCapture | null = null
let mainPrivateMcpConfigPath: string | null = null
let lastResolvedBinary: string | null = null
let analytixStartPromise: Promise<void> | null = null
let childStderrTail = ''
/** Children killed on purpose (stop/quit/settings restart) — their exit is not a crash. */
const intentionalStops = new WeakSet<ChildProcess>()
/** Children that completed the ready handshake — only their exits count as runtime crashes. */
const readyChildren = new WeakSet<ChildProcess>()

export type AnalytixUnexpectedExitInfo = {
  code: number | null
  signal: NodeJS.Signals | null
  stderrBytes: number
  stderrSha256: string
}

let onUnexpectedAnalytixExit: ((info: AnalytixUnexpectedExitInfo) => void) | null = null

/**
 * Called when a READY analytix child exits without the GUI asking for it.
 * Startup failures are excluded: those are already reported to the
 * caller of startAnalytixChild via the thrown error.
 */
export function setAnalytixUnexpectedExitHandler(
  handler: ((info: AnalytixUnexpectedExitInfo) => void) | null
): void {
  onUnexpectedAnalytixExit = handler
}

export function configureAnalytixProcessMainPrivatePaths(
  paths: Readonly<{ mcpConfigPath: string }> | null
): void {
  if (paths === null) {
    mainPrivateMcpConfigPath = null
    return
  }
  if (!isAbsolute(paths.mcpConfigPath) || resolve(paths.mcpConfigPath) !== paths.mcpConfigPath) {
    throw new Error('Analytix main-private MCP config path is invalid.')
  }
  mainPrivateMcpConfigPath = paths.mcpConfigPath
}

function resolveMainPrivateMcpConfigPath(): string {
  return mainPrivateMcpConfigPath ?? resolveAnalytixMcpJsonPath()
}

const execFileAsync = promisify(execFile)
const ANALYTIX_READY_PREFIX = 'ANALYTIX_READY '
// Cold starts on slow disks (Windows + antivirus scans, sqlite rebuilds,
// MCP server connects) routinely exceed 15s; killing analytix that early left
// fresh installs permanently "unable to connect" (#188).
const ANALYTIX_STARTUP_TIMEOUT_MS = 45_000
const ANALYTIX_STARTUP_HEALTH_POLL_MS = 500
const ANALYTIX_STARTUP_HEALTH_REQUEST_TIMEOUT_MS = 1_000
const ANALYTIX_STOP_GRACE_MS = 5_000
const ANALYTIX_STOP_FORCE_MS = 1_000
const STDERR_TAIL_MAX_CHARS = 32_768
const GUI_PLUGIN_MCP_SERVER_IDS_PATH = '.cache/gui-plugin-mcp-server-ids.json'
const GUI_PLUGIN_MCP_SERVER_IDS_CONTRACT = 'analytix-plugin-managed-mcp-server-ids'
const GUI_PLUGIN_MCP_SERVER_IDS_VERSION = 1
const MAX_RUNTIME_CONFIG_JSON_BYTES = 4 * 1024 * 1024
const MAX_PLUGIN_MANIFEST_JSON_BYTES = 256 * 1024
const MAX_PLUGIN_SIDECAR_JSON_BYTES = 64 * 1024
const ANALYTIX_COMPUTER_USE_MCP_SERVER_ID = 'analytix-computer-use'
const ANALYTIX_COMPUTER_USE_MCP_COMMAND_ENV = 'ANALYTIX_COMPUTER_USE_MCP_COMMAND'
const ANALYTIX_COMPUTER_USE_MCP_TIMEOUT_MS = 30_000
const HUB_MANAGED_STANDARD_MCP_SERVER_IDS = new Set([
  'playwright',
  'github',
  'context7',
  'sequential-thinking',
  'memory',
  'brave-search'
])
const HUB_MANAGED_STANDARD_MCP_PLUGIN_NAMES = new Map<string, ReadonlySet<string>>([
  ['playwright', new Set(['playwright-mcp'])],
  ['github', new Set(['github'])],
  ['context7', new Set(['context7'])],
  ['sequential-thinking', new Set(['sequential-thinking'])],
  ['memory', new Set(['memory'])],
  ['brave-search', new Set(['brave-search'])]
])
const MAX_TCP_PORT = 65_535
type StrictJsonReadState<T> =
  | { status: 'missing' }
  | { status: 'invalid' }
  | { status: 'valid'; value: T; bytes: Buffer }

type McpServerCandidate = {
  serverId: string
  source: 'existing' | 'gui-import' | 'installed-plugin' | 'computer-use' | 'schedule'
  markerVerifiedPlugin: boolean
  pluginName?: string
  normalize: (forceDisabled: boolean) => Record<string, unknown> | null
}

type ResolvedMcpServers = {
  servers: Record<string, unknown>
  pluginServerIds: string[]
}

type PluginManagedMcpServerIdsState = {
  ids: Set<string>
}

type VerifiedInstalledPlugin = {
  pluginDir: string
  pluginName: string
  pluginVersion: string
  manifest: Record<string, unknown>
  manifestSha256: string
  rawServers: Record<string, unknown>
}

export type AnalytixChildLifecycleEvent =
  | { code: 'ANALYTIX_CHILD_SPAWNED'; port: number }
  | { code: 'ANALYTIX_CHILD_EXITED'; exitCode: number | null; signaled: boolean }
  | { code: 'ANALYTIX_CHILD_PROCESS_ERROR' }
  | { code: 'ANALYTIX_CHILD_STARTUP_FAILED' }
  | { code: 'ANALYTIX_CHILD_READY'; port: number }
  | { code: 'ANALYTIX_STALE_CHILD_TERMINATION'; port: number }

export type AnalytixChildLogCapture = {
  captureStdout: (chunk: Buffer | string) => void
  captureStderr: (chunk: Buffer | string) => void
  logLifecycle: (event: AnalytixChildLifecycleEvent) => void
  close: () => Promise<void>
}

export type AnalytixChildLogWriter = (record: ManagedChildLogRecord) => void | Promise<void>

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function appendTail(current: string, nextChunk: string, maxChars = STDERR_TAIL_MAX_CHARS): string {
  const combined = `${current}${nextChunk}`
  return combined.length > maxChars ? combined.slice(-maxChars) : combined
}

export function analytixStderrDiagnostic(stderr: string): Pick<AnalytixUnexpectedExitInfo, 'stderrBytes' | 'stderrSha256'> {
  const stderrBytes = Buffer.byteLength(stderr, 'utf8')
  return {
    stderrBytes,
    stderrSha256: createHash('sha256').update(stderr, 'utf8').digest('hex')
  }
}

function normalizeCapturedChunk(chunk: Buffer | string): string {
  return String(chunk).replace(/\r\n/g, '\n').replace(/\r/g, '\n')
}

export function createAnalytixChildLogCapture(
  _pid: number | undefined,
  writer: AnalytixChildLogWriter = appendManagedChildLogRecord
): AnalytixChildLogCapture {
  const stdoutHash = createHash('sha256')
  const stderrHash = createHash('sha256')
  let stdoutBytes = 0
  let stderrBytes = 0
  let closed = false
  let pending = Promise.resolve()

  const writeRecord = (record: ManagedChildLogRecord): void => {
    const normalized = normalizeManagedChildLogRecord(record)
    if (!normalized) return
    pending = pending
      .then(() => writer(normalized))
      .catch(() => undefined)
  }

  const captureChunk = (
    stream: 'stdout' | 'stderr',
    chunk: Buffer | string
  ): void => {
    if (closed) return
    const bytes = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk, 'utf8')
    if (stream === 'stdout') {
      stdoutBytes += bytes.byteLength
      stdoutHash.update(bytes)
    } else {
      stderrBytes += bytes.byteLength
      stderrHash.update(bytes)
    }
  }

  return {
    captureStdout(chunk) {
      captureChunk('stdout', chunk)
    },
    captureStderr(chunk) {
      captureChunk('stderr', chunk)
    },
    logLifecycle(event) {
      if (closed) return
      writeRecord(event)
    },
    async close() {
      if (closed) {
        await pending
        return
      }
      closed = true
      writeRecord({
        code: 'ANALYTIX_CHILD_STDOUT_CAPTURED',
        bytes: stdoutBytes,
        sha256: stdoutHash.digest('hex')
      })
      writeRecord({
        code: 'ANALYTIX_CHILD_STDERR_CAPTURED',
        bytes: stderrBytes,
        sha256: stderrHash.digest('hex')
      })
      await pending
    }
  }
}

function appRoot(): string {
  return app.isPackaged
    ? app.getAppPath().replace(/app\.asar$/, 'app.asar.unpacked')
    : app.getAppPath()
}

function appResourcesPath(): string {
  const processWithResources = process as NodeJS.Process & { resourcesPath?: string }
  if (app.isPackaged && typeof processWithResources.resourcesPath === 'string') {
    return processWithResources.resourcesPath
  }
  const appPath = app.getAppPath()
  return appPath.endsWith('app.asar') || appPath.endsWith('app.asar.unpacked')
    ? dirname(appPath)
    : appPath
}

function resolveNodeScriptCommand(command: string): string {
  if (command !== process.execPath) return command
  if (process.platform !== 'darwin') return command
  return resolveClawScheduleMcpCommand({
    appPath: app.getAppPath(),
    execPath: command,
    isPackaged: app.isPackaged
  })
}

export function resolveDocumentCodecLaunch(
  launch = { appPath: app.getAppPath(), execPath: process.execPath, isPackaged: app.isPackaged },
  platform: NodeJS.Platform = process.platform
): { executable: string; entry: string } {
  const root = launch.isPackaged ? launch.appPath.replace(/app\.asar$/, 'app.asar.unpacked') : launch.appPath
  return {
    executable: resolveClawScheduleMcpCommand(launch, platform),
    entry: join(root, 'out', 'office-codec', 'office-generation-codec-entry.js')
  }
}

export function resolveAnalytixDataDir(runtime: { dataDir: string }): string {
  const trimmed = runtime.dataDir?.trim()
  if (trimmed) return expandHomePath(trimmed)
  return defaultAnalytixDataDir()
}

function expandHomePath(path: string): string {
  if (path === '~') return homedir()
  if (path.startsWith('~/') || path.startsWith('~\\')) {
    return join(homedir(), path.slice(2).replace(/\\/g, '/'))
  }
  return path
}

export function isAnalytixChildRunning(): boolean {
  return child !== null && child.exitCode === null && child.signalCode === null
}

export function startAnalytixChild(settings: AppSettingsV1): Promise<void> {
  void settings
  return Promise.reject(
    new Error('TypeScript Analytix child runtime is retired; Go runtime-server is the only production agent runtime.')
  )
}

async function startAnalytixChildOnce(
  settings: AppSettingsV1,
  runtime: AnalytixRuntimeSettingsV1
): Promise<void> {
  if (childLogCapture) {
    await childLogCapture.close()
    childLogCapture = null
  }
  const root = appRoot()
  const resolution = resolveAnalytixExecutable(root, runtime.binaryPath)
  if (resolution.command === process.execPath && !existsSync(resolution.args[0])) {
    throw new Error(
      `Analytix runtime build is missing at ${resolution.args[0]}. Run \`npm run build:runtime\` before starting the GUI.`
    )
  }
  const dataDir = resolveAnalytixDataDir(runtime)
  await syncGuiManagedAnalytixConfig(dataDir, runtime, {
    scheduleMcp: {
      settings,
      launch: {
        appPath: app.getAppPath(),
        execPath: process.execPath,
        isPackaged: app.isPackaged
      }
    }
  })
  lastResolvedBinary = resolution.command === process.execPath
    ? resolution.args.join(' ')
    : resolution.command
  const proxyUrl = resolveModelProviderProxyUrl(settings)
  const args = buildAnalytixServeArgs({
    resolution,
    host: '127.0.0.1',
    port: runtime.port,
    dataDir,
    baseUrl: runtime.baseUrl,
    modelProxyUrl: proxyUrl,
    mcpProxyUrl: proxyUrl,
    endpointFormat: runtime.endpointFormat,
    model: runtime.model,
    approvalPolicy: runtime.approvalPolicy,
    sandboxMode: runtime.sandboxMode,
    tokenEconomyMode: runtime.tokenEconomyMode,
    insecure: isAnalytixRuntimeInsecure(runtime)
  })
  const runAsElectron = process.platform === 'darwin' && runtime.computerUse?.enabled === true
  const command = runAsElectron ? resolution.command : resolveNodeScriptCommand(resolution.command)
  const documentCodec = resolveDocumentCodecLaunch()
  const childEnv: NodeJS.ProcessEnv = {
    ...process.env,
    ANALYTIX_RUNTIME_TOKEN: runtime.runtimeToken,
    ANALYTIX_MCP_CONFIG_PATH: resolveMainPrivateMcpConfigPath(),
    ANALYTIX_APP_ROOT: appRoot(),
    ANALYTIX_DOCUMENT_CODEC_EXECUTABLE: documentCodec.executable,
    ANALYTIX_DOCUMENT_CODEC_ENTRY: documentCodec.entry,
    ANALYTIX_RESOURCES_PATH: appResourcesPath()
  }
  for (const name of [
    'ANALYTIX_API_KEY',
    'ANALYTIX_MODEL_PROVIDERS',
    'ANALYTIX_HUB_TEST_GATEWAY_TOKEN',
    'ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN'
  ]) {
    delete childEnv[name]
  }
  if (runAsElectron) {
    delete childEnv.ELECTRON_RUN_AS_NODE
  } else {
    childEnv.ELECTRON_RUN_AS_NODE = '1'
  }
  child = spawn(command, args, {
    env: childEnv,
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: false
  })
  const startedChild = child
  const startedLogCapture = createAnalytixChildLogCapture(startedChild.pid)
  childLogCapture = startedLogCapture
  childStderrTail = ''
  startedLogCapture.logLifecycle({ code: 'ANALYTIX_CHILD_SPAWNED', port: runtime.port })
  startedChild.stdout?.on('data', startedLogCapture.captureStdout)
  startedChild.stderr?.on('data', (chunk: Buffer | string) => {
    childStderrTail = appendTail(childStderrTail, normalizeCapturedChunk(chunk))
    startedLogCapture.captureStderr(chunk)
  })
  child.on('exit', (code, signal) => {
    startedLogCapture.logLifecycle({
      code: 'ANALYTIX_CHILD_EXITED',
      exitCode: code,
      signaled: signal !== null
    })
    void startedLogCapture.close()
    if (child === startedChild) child = null
    if (readyChildren.has(startedChild) && !intentionalStops.has(startedChild)) {
      onUnexpectedAnalytixExit?.({
        code: code ?? null,
        signal: signal ?? null,
        ...analytixStderrDiagnostic(childStderrTail)
      })
    }
  })
  child.on('error', () => {
    startedLogCapture.logLifecycle({ code: 'ANALYTIX_CHILD_PROCESS_ERROR' })
  })
  try {
    await waitForAnalytixStartup(startedChild, runtime.port)
  } catch (error) {
    startedLogCapture.logLifecycle({ code: 'ANALYTIX_CHILD_STARTUP_FAILED' })
    if (child === startedChild) {
      await stopAnalytixChildAndWait()
    }
    throw error
  }
  readyChildren.add(startedChild)
  startedLogCapture.logLifecycle({ code: 'ANALYTIX_CHILD_READY', port: runtime.port })
}

export async function syncGuiManagedAnalytixConfig(
  dataDir: string,
  runtime: Pick<
    AnalytixRuntimeSettingsV1,
    | 'mcpSearch'
    | 'runtimeTuning'
    | 'computerUse'
    | 'visionBridge'
    | 'modelCapabilityProbes'
    | 'subagents'
  > & Partial<AnalytixRuntimeSettingsV1>,
  options?: {
    scheduleMcp?: {
      settings: AppSettingsV1
      launch: ClawScheduleMcpLaunchConfig
    }
    mcpConfigPath?: string
    bundledFundsMaterialization?: BundledFundsMaterializationBindingV1
  }
): Promise<{ hostScheduleMcpBindingV1: RuntimeHostScheduleMcpBindingV1 | null }> {
  let hostScheduleMcpBindingV1: RuntimeHostScheduleMcpBindingV1 | null = null
  if (options?.scheduleMcp) {
    try {
      hostScheduleMcpBindingV1 = buildRuntimeHostScheduleMcpBindingV1(
        options.scheduleMcp.settings,
        options.scheduleMcp.launch
      )
    } catch {
      // The schedule MCP is additive. Invalid host projection closes only
      // that server; it must not become a general Agent startup gate.
    }
  }
  const configPath = join(dataDir, 'config.json')
  const configState = await readStrictJsonObjectState(configPath, MAX_RUNTIME_CONFIG_JSON_BYTES)
  const rawExisting = configState.status === 'valid' ? configState.value : null
  const rawCapabilities = objectValue(rawExisting?.capabilities)
  const existingMcpExplicitlyDisabled = objectValue(rawCapabilities.mcp).enabled === false
  const rawWeb = objectValue(rawCapabilities.web)
  const existingWebExplicitlyDisabled = rawWeb.enabled === false
  const existingWebFetchExplicitlyDisabled = rawWeb.fetchEnabled === false
  const existing = sanitizeProductionAnalytixConfig(rawExisting)
  const existingRuntimeTuning = objectValue(existing?.runtime)
  const capabilities = objectValue(existing?.capabilities)
  const mcp = parseAnalytixConfigSection(McpCapabilityConfig, capabilities.mcp)
  const importedMcpCandidates = await readGuiManagedMcpServerCandidates(
    options?.mcpConfigPath ?? resolveMainPrivateMcpConfigPath()
  )
  const pluginMcpCandidates = await discoverInstalledPluginMcpServerCandidates(
    dataDir,
    options?.bundledFundsMaterialization
  )
  const previousPluginMcpServerIds = await readPluginManagedMcpServerIds(
    dataDir,
    configState.status === 'valid' ? configState.bytes : null
  )
  const existingMcpServers = Object.fromEntries(
    Object.entries(objectValue(objectValue(capabilities.mcp).servers))
      .filter(([serverId, server]) => (
        !previousPluginMcpServerIds.ids.has(serverId) &&
        !isPersistedInstalledPluginMcpServer(dataDir, serverId, server)
      ))
  )
  const builtInComputerUseCandidate = builtInComputerUseMcpServerCandidateForRuntime(runtime.computerUse)
  const candidates: McpServerCandidate[] = [
    ...mcpServerCandidatesFromRecord(existingMcpServers, 'existing'),
    ...importedMcpCandidates,
    ...pluginMcpCandidates,
    ...(builtInComputerUseCandidate ? [builtInComputerUseCandidate] : [])
  ]
  if (hostScheduleMcpBindingV1 && !existingMcpExplicitlyDisabled) {
    candidates.push({
      serverId: GUI_SCHEDULE_MCP_SERVER_NAME,
      source: 'schedule',
      markerVerifiedPlugin: false,
      normalize: () => buildGuiScheduleAnalytixMcpServer(hostScheduleMcpBindingV1)
    })
  }
  const resolvedMcp = resolveMcpServerCandidates(candidates, existingMcpExplicitlyDisabled)
  const hasEnabledMcpServer = Object.values(resolvedMcp.servers).some(
    (server) => objectValue(server).enabled !== false
  )

  const search = objectValue(mcp.search)
  const web = parseAnalytixConfigSection(WebCapabilityConfig, capabilities.web)
  const skills = parseAnalytixConfigSection(SkillsCapabilityConfig, capabilities.skills)
  const subagents = parseAnalytixConfigSection(SubagentsCapabilityConfig, capabilities.subagents)
  const mcpSearch = runtime.mcpSearch
  const skillCapability = await skillCapabilityConfigForRuntime(
    skills,
    options?.scheduleMcp?.settings,
    options?.bundledFundsMaterialization
  )
  const next = {
    runtime: runtimeTuningConfigForRuntime(runtime.runtimeTuning, existingRuntimeTuning),
    capabilities: {
      web: {
        ...web,
        enabled: existingWebExplicitlyDisabled ? false : true,
        fetchEnabled: existingWebFetchExplicitlyDisabled ? false : true
      },
      skills: skillCapability,
      subagents: subagentsConfigForRuntime(runtime.subagents, subagents),
      visionBridge: visionBridgeConfigForRuntime(runtime),
      mcp: {
        ...mcp,
        ...(hostScheduleMcpBindingV1 || mcpSearch.enabled || hasEnabledMcpServer
          ? { enabled: existingMcpExplicitlyDisabled ? false : true }
          : {}),
        servers: resolvedMcp.servers,
        search: {
          ...search,
          enabled: mcpSearch.enabled,
          mode: mcpSearch.mode,
          autoThresholdToolCount: mcpSearch.autoThresholdToolCount,
          topKDefault: mcpSearch.topKDefault,
          topKMax: mcpSearch.topKMax,
          minScore: mcpSearch.minScore
        }
      }
    }
  }
  const parsedNext = AnalytixConfigSchema.safeParse(next)
  if (!parsedNext.success) {
    throw new Error(
      `Refusing to write invalid GUI-managed Analytix config at ${configPath}: ${JSON.stringify(parsedNext.error.issues, null, 2)}`
    )
  }
  const nextText = `${JSON.stringify(next, null, 2)}\n`
  const currentText = configState.status === 'valid' ? configState.bytes.toString('utf8') : null
  if (nextText !== currentText) {
    await mkdir(dirname(configPath), { recursive: true })
    await atomicWriteFile(configPath, nextText, { mode: 0o600 })
  } else {
    await ensureFileMode(configPath, 0o600)
  }
  await writePluginManagedMcpServerIds(dataDir, resolvedMcp.pluginServerIds, nextText)
  const scheduleServerValue = resolvedMcp.servers[GUI_SCHEDULE_MCP_SERVER_NAME]
  const scheduleServer = objectValue(scheduleServerValue)
  return {
    hostScheduleMcpBindingV1: hostScheduleMcpBindingV1 && scheduleServerValue !== undefined &&
      scheduleServer.enabled !== false
      ? hostScheduleMcpBindingV1
      : null
  }
}

export type ManagedMcpAccountCredentialBindingV1 = {
  owner: 'mcp' | 'extension'
  provider: string
  accountId: string
  channelId: string
  purpose: 'mcp-oauth-access-token' | 'extension-provider-account-token'
  oauthBinding?: ProviderRegistryOAuthBindingV1
  ownerFingerprint: string
}

function sealMcpAccountCredentialBinding(server: Record<string, unknown>): Record<string, unknown> | null {
  const parsed = McpServerConfig.safeParse(server)
  if (!parsed.success) return null
  const account = parsed.data.accountCredential
  if (!account) return objectValue(parsed.data)
  const { bindingFingerprint: _ignored, ...authority } = account
  const canonical = { ...parsed.data, accountCredential: authority }
  const bindingFingerprint = createHash('sha256').update(JSON.stringify(canonical)).digest('hex')
  return { ...canonical, accountCredential: { ...authority, bindingFingerprint } }
}

/**
 * Reads only the already-normalized Main-owned runtime config. Installed
 * extension bindings must still match the verified plugin generation identity;
 * GUI MCP bindings are confined to their own server id.
 */
export async function listManagedMcpAccountCredentialBindings(
  dataDir: string
): Promise<ManagedMcpAccountCredentialBindingV1[]> {
  const state = await readStrictJsonObjectState(join(dataDir, 'config.json'), MAX_RUNTIME_CONFIG_JSON_BYTES)
  if (state.status !== 'valid') return []
  const servers = objectValue(objectValue(objectValue(state.value.capabilities).mcp).servers)
  const bindings: ManagedMcpAccountCredentialBindingV1[] = []
  for (const [serverId, rawServer] of Object.entries(servers)) {
    const parsed = McpServerConfig.safeParse(rawServer)
    if (!parsed.success || parsed.data.enabled === false || !parsed.data.accountCredential) continue
    const account = parsed.data.accountCredential
    const sealed = sealMcpAccountCredentialBinding(objectValue(parsed.data))
    const sealedParsed = McpServerConfig.safeParse(sealed)
    if (!sealedParsed.success || !sealedParsed.data.accountCredential ||
      sealedParsed.data.accountCredential.bindingFingerprint !== account.bindingFingerprint) continue
    const ownerFingerprint = account.bindingFingerprint
    if (!ownerFingerprint) continue
    if (account.owner === 'mcp') {
      if (account.provider !== serverId || parsed.data.identitySource === 'installed-plugin-manifest') continue
      bindings.push({
        ...account, channelId: serverId,
        ...(parsed.data.oauthBinding ? { oauthBinding: parsed.data.oauthBinding } : {}),
        ownerFingerprint
      })
      continue
    }
    if (parsed.data.identitySource !== 'installed-plugin-manifest' || !parsed.data.pluginRootPath) continue
    const pluginRoot = resolve(parsed.data.pluginRootPath)
    const pluginProvider = basename(dirname(pluginRoot))
    if (!pluginProvider || pluginProvider === '.' || account.provider !== pluginProvider) continue
    const binding = {
      ...account, channelId: serverId,
      ...(parsed.data.oauthBinding ? { oauthBinding: parsed.data.oauthBinding } : {}),
      ownerFingerprint
    }
    const verified = await verifyInstalledPlugin(pluginProvider, pluginRoot, dataDir)
    const verifiedRawServer = verified?.pluginName === pluginProvider
      ? verified.rawServers[serverId]
      : undefined
    const verifiedServer = verifiedRawServer === undefined || !verified
      ? null
      : normalizeInstalledPluginMcpServer(pluginRoot, serverId, verifiedRawServer, {
          manifestSha256: verified.manifestSha256,
          extensionProvider: verified.pluginName,
          expectedServerVersion: verified.pluginVersion
        })
    const verifiedParsed = McpServerConfig.safeParse(verifiedServer)
    if (!verifiedParsed.success || JSON.stringify(verifiedParsed.data.accountCredential) !== JSON.stringify(account)) {
      continue
    }
    bindings.push(binding)
  }
  return bindings.sort((left, right) => (
    `${left.owner}\0${left.provider}\0${left.channelId}\0${left.accountId}`
      .localeCompare(`${right.owner}\0${right.provider}\0${right.channelId}\0${right.accountId}`)
  ))
}

function builtInComputerUseMcpServerCandidateForRuntime(
  computerUse: Pick<AnalytixRuntimeSettingsV1, 'computerUse'>['computerUse']
): McpServerCandidate | null {
  if (computerUse.enabled !== true) return null
  return {
    serverId: ANALYTIX_COMPUTER_USE_MCP_SERVER_ID,
    source: 'computer-use',
    markerVerifiedPlugin: false,
    normalize: (forceDisabled) => {
      if (forceDisabled) return null
      const command = resolveBuiltInComputerUseMcpCommand()
      if (!command) return null
      return {
        enabled: true,
        transport: 'stdio',
        command,
      args: ['mcp'],
      env: {
        ANALYTIX_COMPUTER_USE_MANAGED_BY: 'analytix'
      },
      trustScope: 'user',
        backgroundStart: false,
        timeoutMs: ANALYTIX_COMPUTER_USE_MCP_TIMEOUT_MS
      }
    }
  }
}

export function resolveBuiltInComputerUseMcpCommand(options: {
  env?: NodeJS.ProcessEnv
  platform?: NodeJS.Platform
  arch?: NodeJS.Architecture
  root?: string
} = {}): string | null {
  const env = options.env ?? process.env
  const explicit = env[ANALYTIX_COMPUTER_USE_MCP_COMMAND_ENV]?.trim()
  if (explicit) {
    return isAbsolute(explicit) && !existsSync(explicit) ? null : explicit
  }
  const root = options.root ?? appRoot()
  const platform = options.platform ?? process.platform
  const arch = options.arch ?? process.arch
  const packageRoot = join(root, 'packages', 'runtime', 'node_modules', 'analytix-computer-use')
  const archDir = arch === 'arm64' ? 'arm64' : 'amd64'
  const nativeCandidates = platform === 'win32'
    ? [join(packageRoot, 'dist', 'windows', archDir, 'analytix-computer-use.exe')]
    : platform === 'darwin'
      ? [join(packageRoot, 'dist', 'Analytix Computer Use.app', 'Contents', 'MacOS', 'OpenComputerUse')]
      : [join(packageRoot, 'dist', 'linux', archDir, 'analytix-computer-use')]
  const shimName = platform === 'win32' ? 'analytix-computer-use.cmd' : 'analytix-computer-use'
  const candidates = [
    ...nativeCandidates,
    join(root, 'packages', 'runtime', 'node_modules', '.bin', shimName),
    join(root, 'node_modules', '.bin', shimName)
  ]
  return candidates.find((candidate) => existsSync(candidate)) ?? null
}

function buildGuiScheduleAnalytixMcpServer(
  binding: RuntimeHostScheduleMcpBindingV1
): Record<string, unknown> {
  return {
    enabled: true,
    transport: 'stdio',
    command: binding.command,
    args: [...binding.args],
    env: { ...binding.env },
    trustScope: binding.trustScope,
    timeoutMs: binding.timeoutMs
  }
}

async function skillCapabilityConfigForRuntime(
  existing: Record<string, unknown>,
  settings?: AppSettingsV1,
  bundledFundsMaterialization?: BundledFundsMaterializationBindingV1
): Promise<Record<string, unknown>> {
  // Carry over only the roots a user added by hand to the Analytix config file.
  // Drop previously-persisted GUI-managed roots so disabling a directory in
  // settings actually removes it — otherwise a toggled-off root would stick
  // around forever via `existing.roots`.
  const managed = guiSkillManagedComparablePaths(settings)
  const managedPrefixes = guiPluginSkillManagedComparablePrefixes()
  const runtimeSkillRoots = await guiSkillRootsForRuntime(
    settings,
    undefined,
    bundledFundsMaterialization ?? null
  )
  const hasAnalytixComputerUseRoot = runtimeSkillRoots.some((root) => isAnalytixComputerUseSkillRoot(root.path))
  const manualExisting = stringArrayValue(existing.roots)
    .map(normalizeSkillRootPath)
    .filter((path) => path.length > 0 && !isManagedSkillRoot(path, managed, managedPrefixes))
    .filter((path) => !(hasAnalytixComputerUseRoot && isLegacyComputerUseSkillRoot(path)))
  const roots = uniqueStrings([
    ...manualExisting,
    ...runtimeSkillRoots.map((root) => root.path)
  ])
  return {
    ...existing,
    // Auto-enable once we discover skill roots. There is no user-facing skills
    // enable toggle, so a persisted `enabled: false` is only ever the schema
    // default leaking onto disk — it must not permanently suppress discovered
    // skills. An explicit `true` still forces on even with no roots.
    enabled: roots.length > 0 || existing.enabled === true,
    roots,
    legacySkillMd: existing.legacySkillMd === false ? false : true
  }
}

function isManagedSkillRoot(path: string, managed: Set<string>, managedPrefixes: string[]): boolean {
  const comparable = comparableSkillRootPath(path)
  return managed.has(comparable) ||
    managedPrefixes.some((prefix) => comparable === prefix || comparable.startsWith(`${prefix}/`))
}

function stringArrayValue(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
    : []
}

function uniqueStrings(values: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const value of values) {
    if (!value || seen.has(value)) continue
    seen.add(value)
    out.push(value)
  }
  return out
}

function mcpServerCandidatesFromRecord(
  servers: Record<string, unknown>,
  source: 'existing' | 'computer-use'
): McpServerCandidate[] {
  return Object.entries(servers).map(([serverId, server]) => ({
    serverId,
    source,
    markerVerifiedPlugin: false,
    normalize: (forceDisabled) => projectMcpServerActivation(
      normalizeGuiManagedMcpServer(serverId, server),
      forceDisabled
    )
  }))
}

async function readGuiManagedMcpServerCandidates(path: string): Promise<McpServerCandidate[]> {
  const state = await readStrictJsonObjectState(path, MAX_RUNTIME_CONFIG_JSON_BYTES)
  if (state.status !== 'valid') return []
  const rawServers = mcpServersFromStrictConfig(state.value)
  if (!rawServers) return []
  return Object.entries(rawServers).map(([serverId, server]) => ({
    serverId,
    source: 'gui-import' as const,
    markerVerifiedPlugin: false,
    normalize: (forceDisabled: boolean) => projectMcpServerActivation(
      normalizeGuiManagedMcpServer(serverId, server),
      forceDisabled
    )
  }))
}

async function discoverInstalledPluginMcpServerCandidates(
  dataDir: string,
  hostMaterialization?: BundledFundsMaterializationBindingV1
): Promise<McpServerCandidate[]> {
  const groups = await discoverInstalledPluginGenerationGroups(dataDir)
  const candidates: McpServerCandidate[] = []
  for (const pluginName of [...groups.keys()].sort()) {
    const group = groups.get(pluginName)
    if (!group || group.invalid || group.pluginDirs.length !== 1) continue
    const verified = await verifyInstalledPlugin(
      pluginName,
      group.pluginDirs[0],
      dataDir,
      hostMaterialization?.pluginName === pluginName ? hostMaterialization : undefined
    )
    if (!verified) continue
    for (const [serverId, server] of Object.entries(verified.rawServers)) {
      candidates.push({
        serverId,
        source: 'installed-plugin',
        markerVerifiedPlugin: true,
        pluginName: verified.pluginName,
        normalize: (forceDisabled) => normalizeInstalledPluginMcpServer(
          verified.pluginDir,
          serverId,
          server,
          {
            forceDisabled,
            manifestSha256: verified.manifestSha256,
            extensionProvider: verified.pluginName,
            expectedServerVersion: verified.pluginVersion
          }
        )
      })
    }
  }
  return candidates
}

async function discoverInstalledPluginGenerationGroups(
  dataDir: string
): Promise<Map<string, { pluginDirs: string[]; invalid: boolean }>> {
  const groups = new Map<string, { pluginDirs: string[]; invalid: boolean }>()
  for (const cacheRoot of installedPluginMcpCacheRoots(dataDir)) {
    const marketplaceRoot = join(cacheRoot, 'analytix-hub')
    const pluginEntries = await readdir(marketplaceRoot, { withFileTypes: true }).catch(() => [])
    for (const pluginEntry of pluginEntries) {
      if (pluginEntry.name.startsWith('.')) continue
      const canonicalName = pluginEntry.name.toLowerCase()
      const group = groups.get(canonicalName) ?? { pluginDirs: [], invalid: false }
      groups.set(canonicalName, group)
      if (!pluginEntry.isDirectory() || canonicalName !== pluginEntry.name) {
        group.invalid = true
        continue
      }
      const pluginRoot = join(marketplaceRoot, pluginEntry.name)
      const generations = await readdir(pluginRoot, { withFileTypes: true }).catch(() => [])
      if (generations.length === 0) group.invalid = true
      for (const generation of generations) {
        if (generation.name.startsWith('.')) continue
        if (!generation.isDirectory()) {
          group.invalid = true
          continue
        }
        group.pluginDirs.push(resolve(pluginRoot, generation.name))
      }
    }
  }
  return groups
}

function installedPluginMcpCacheRoots(dataDir: string): string[] {
  const candidates = [join(dataDir, 'plugins', 'cache')]
  if (basename(dataDir) === 'data') {
    candidates.push(join(dirname(dataDir), 'plugins', 'cache'))
  }
  return uniqueStrings(candidates.map((candidate) => resolve(candidate)))
}

async function verifyInstalledPlugin(
  directoryPluginName: string,
  pluginDir: string,
  dataDir: string,
  hostMaterialization?: BundledFundsMaterializationBindingV1
): Promise<VerifiedInstalledPlugin | null> {
  const runtimeHome = basename(dataDir) === 'data' ? dirname(dataDir) : dataDir
  const binding = verifyInstalledPluginBindingV1({
    runtimeHome,
    pluginDir,
    installCacheRoots: installedPluginMcpCacheRoots(dataDir),
    expectedPluginName: directoryPluginName,
    hostMaterialization
  })
  if (!binding) return null
  const { manifest, pluginName } = binding
  const rawServers = await readInstalledPluginMcpServers(pluginDir, manifest)
  if (!rawServers) return null
  for (const [serverId, server] of Object.entries(rawServers)) {
    if (
      !McpServerIdV1.safeParse(serverId).success ||
      !validInstalledPluginMcpServerInput(server) ||
      !normalizeInstalledPluginMcpServer(pluginDir, serverId, server, {
        forceDisabled: true,
        extensionProvider: pluginName,
        expectedServerVersion: binding.version
      })
    ) return null
  }
  return {
    pluginDir,
    pluginName,
    pluginVersion: binding.version,
    manifest,
    manifestSha256: createHash('sha256').update(binding.manifestBytes).digest('hex'),
    rawServers
  }
}

function exactString(value: unknown): string | null {
  return typeof value === 'string' && value === value.trim() ? value : null
}

async function readInstalledPluginMcpServers(
  pluginDir: string,
  manifest: Record<string, unknown>
): Promise<Record<string, unknown> | null> {
  const serverEntries = new Map<string, { serverId: string; server: unknown }>()
  let invalid = false
  const mergeServers = (servers: Record<string, unknown>): void => {
    for (const [serverId, server] of Object.entries(servers)) {
      const collisionKey = serverId.trim().toLowerCase()
      if (!collisionKey || !McpServerIdV1.safeParse(serverId).success || serverEntries.has(collisionKey)) {
        invalid = true
        continue
      }
      serverEntries.set(collisionKey, { serverId, server })
    }
  }
  if ('mcpServers' in manifest && 'mcp' in manifest) return null
  const rawManifestMcp = manifest.mcpServers ?? manifest.mcp
  if (rawManifestMcp !== undefined && typeof rawManifestMcp !== 'string') {
    const inline = strictObjectValue(rawManifestMcp)
    if (!inline) return null
    mergeServers(inline)
  }

  const fileCandidates = new Set<string>()
  if (typeof rawManifestMcp === 'string') {
    const filePath = resolvePluginPath(pluginDir, pluginDir, rawManifestMcp)
    if (!filePath) return null
    fileCandidates.add(filePath)
  }
  const defaultMcpPath = join(pluginDir, '.mcp.json')
  if (rawManifestMcp === undefined && existsSync(defaultMcpPath)) fileCandidates.add(defaultMcpPath)

  for (const filePath of fileCandidates) {
    const state = await readStrictJsonObjectState(filePath, MAX_PLUGIN_MANIFEST_JSON_BYTES)
    if (state.status !== 'valid') return null
    const servers = mcpServersFromStrictConfig(state.value)
    if (!servers) return null
    mergeServers(servers)
  }
  if (invalid) return null
  return Object.fromEntries([...serverEntries.values()].map((entry) => [entry.serverId, entry.server]))
}

function validInstalledPluginMcpServerInput(value: unknown): boolean {
  const server = strictObjectValue(value)
  if (!server) return false
  const allowedKeys = new Set([
    'enabled', 'disabled', 'transport', 'type', 'command', 'args', 'cwd', 'url',
    'headers', 'accountCredential', 'oauthBinding', 'env', 'env_vars', 'envVars', 'trustScope', 'trustedWorkspaceRoots',
    'lowPriority', 'backgroundStart', 'timeoutMs', 'startup_timeout_sec',
    'tool_timeout_sec', 'connect_timeout', 'execute_timeout', 'read_timeout',
    'required', 'enabled_tools', 'disabled_tools', 'default_tools_approval_mode', 'tools'
  ])
  if (Object.keys(server).some((key) => !allowedKeys.has(key))) return false
  for (const key of ['enabled', 'disabled', 'lowPriority', 'backgroundStart', 'required']) {
    if (key in server && typeof server[key] !== 'boolean') return false
  }
  for (const key of ['command', 'cwd', 'url', 'default_tools_approval_mode']) {
    if (key in server && !exactNonEmptyString(server[key])) return false
  }
  if ('transport' in server && !['stdio', 'streamable-http', 'sse'].includes(String(server.transport))) return false
  if ('type' in server && !['stdio', 'http', 'streamable-http', 'sse'].includes(String(server.type))) return false
  for (const key of ['args', 'trustedWorkspaceRoots', 'enabled_tools', 'disabled_tools']) {
    if (key in server && (!Array.isArray(server[key]) || !(server[key] as unknown[]).every((item) => typeof item === 'string'))) {
      return false
    }
  }
  for (const key of ['env_vars', 'envVars']) {
    if (key in server && (
      !Array.isArray(server[key]) ||
      !(server[key] as unknown[]).every((item) => typeof item === 'string' && /^[A-Za-z_][A-Za-z0-9_]*$/.test(item))
    )) return false
  }
  for (const key of ['headers', 'env']) {
    if (key in server && !strictStringRecord(server[key])) return false
  }
  if ('accountCredential' in server && !McpAccountCredentialScope.safeParse(server.accountCredential).success) {
    return false
  }
  if ('oauthBinding' in server && !providerRegistryOAuthBindingSchemaV1.safeParse(server.oauthBinding).success) {
    return false
  }
  for (const key of ['timeoutMs', 'startup_timeout_sec', 'tool_timeout_sec']) {
    if (key in server && !positiveIntegerValue(server[key])) return false
  }
  for (const key of ['connect_timeout', 'execute_timeout', 'read_timeout']) {
    if (key in server && server[key] !== null && !positiveIntegerValue(server[key])) return false
  }
  if ('trustScope' in server && server.trustScope !== 'user' && server.trustScope !== 'workspace') return false
  if ('tools' in server && !validPluginToolApprovalMap(server.tools)) return false
  return true
}

function validPluginToolApprovalMap(value: unknown): boolean {
  const tools = strictObjectValue(value)
  if (!tools) return false
  for (const [toolName, raw] of Object.entries(tools)) {
    if (!/^[a-z0-9][a-z0-9_-]{0,127}$/.test(toolName)) return false
    const config = strictObjectValue(raw)
    if (!config || Object.keys(config).some((key) => key !== 'approval_mode')) return false
    if (!exactNonEmptyString(config.approval_mode)) return false
  }
  return true
}

function strictStringRecord(value: unknown): value is Record<string, string> {
  const record = strictObjectValue(value)
  return Boolean(record) && Object.entries(record!).every(
    ([key, item]) => key.length > 0 && key === key.trim() && typeof item === 'string'
  )
}

function resolveMcpServerCandidates(
  candidates: McpServerCandidate[],
  forceDisabled: boolean
): ResolvedMcpServers {
  const groups = new Map<string, McpServerCandidate[]>()
  for (const candidate of candidates) {
    const collisionKey = candidate.serverId.trim().toLowerCase()
    if (!collisionKey) continue
    const group = groups.get(collisionKey) ?? []
    group.push(candidate)
    groups.set(collisionKey, group)
  }
  const servers: Record<string, unknown> = {}
  const pluginServerIds: string[] = []
  for (const collisionKey of [...groups.keys()].sort()) {
    const group = groups.get(collisionKey) ?? []
    const winner = selectMcpServerCandidate(group, collisionKey)
    if (!winner || !McpServerIdV1.safeParse(winner.serverId).success) continue
    if (winner.source === 'installed-plugin' && forceDisabled) continue
    const normalized = winner.normalize(forceDisabled)
    const projected = projectMcpServerActivation(normalized, forceDisabled)
    const parsed = McpServerConfig.safeParse(projected)
    if (!parsed.success) continue
    if (winner.source === 'installed-plugin' && parsed.data.enabled === false) continue
    servers[winner.serverId] = parsed.data
    if (winner.source === 'installed-plugin') pluginServerIds.push(winner.serverId)
  }
  return { servers, pluginServerIds: [...new Set(pluginServerIds)].sort() }
}

function selectMcpServerCandidate(
  group: McpServerCandidate[],
  collisionKey: string
): McpServerCandidate | null {
  const schedule = group.filter((candidate) => candidate.source === 'schedule')
  if (collisionKey === GUI_SCHEDULE_MCP_SERVER_NAME) {
    return schedule.length === 1 ? schedule[0] : null
  }
  const computerUse = group.filter((candidate) => candidate.source === 'computer-use')
  if (collisionKey === ANALYTIX_COMPUTER_USE_MCP_SERVER_ID) {
    return computerUse.length === 1 ? computerUse[0] : null
  }
  if (group.length === 1) return group[0]
  const verifiedPlugins = group.filter((candidate) => candidate.markerVerifiedPlugin)
  if (
    verifiedPlugins.length === 1 &&
    HUB_MANAGED_STANDARD_MCP_SERVER_IDS.has(collisionKey) &&
    HUB_MANAGED_STANDARD_MCP_PLUGIN_NAMES.get(collisionKey)?.has(verifiedPlugins[0].pluginName ?? '') === true &&
    group.every((candidate) => candidate === verifiedPlugins[0] || candidate.source !== 'installed-plugin')
  ) return verifiedPlugins[0]

  if (group.length === 2) {
    const existing = group.find((candidate) => candidate.source === 'existing')
    const current = group.find((candidate) => candidate.source !== 'existing')
    if (existing && current && current.source !== 'installed-plugin' && existing.serverId === current.serverId) {
      const existingValue = existing.normalize(false)
      const currentValue = current.normalize(false)
      if (existingValue && currentValue && JSON.stringify(existingValue) === JSON.stringify(currentValue)) {
        return current
      }
    }
  }
  return null
}

function projectMcpServerActivation(
  server: Record<string, unknown> | null,
  forceDisabled: boolean
): Record<string, unknown> | null {
  if (!server) return null
  const parsed = McpServerConfig.safeParse(server)
  if (!parsed.success) return null
  if (!forceDisabled && parsed.data.enabled !== false) return objectValue(parsed.data)
  const disabled = McpServerConfig.safeParse({
    ...parsed.data,
    enabled: false,
    env: {},
    headers: {}
  })
  return disabled.success ? objectValue(disabled.data) : null
}

function mcpServersFromStrictConfig(config: Record<string, unknown>): Record<string, unknown> | null {
  const sources: Record<string, unknown>[] = []
  if (Object.prototype.hasOwnProperty.call(config, 'mcpServers')) {
    const servers = strictObjectValue(config.mcpServers)
    if (!servers) return null
    sources.push(servers)
  }
  if (Object.prototype.hasOwnProperty.call(config, 'servers')) {
    const servers = strictObjectValue(config.servers)
    if (!servers) return null
    sources.push(servers)
  }
  if (Object.prototype.hasOwnProperty.call(config, 'capabilities')) {
    const capabilities = strictObjectValue(config.capabilities)
    if (!capabilities) return null
    const mcp = strictObjectValue(capabilities.mcp)
    if (!mcp) return null
    const servers = strictObjectValue(mcp.servers)
    if (!servers) return null
    sources.push(servers)
  }
  if (sources.length > 1) return null
  return sources[0] ?? {}
}

function strictObjectValue(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function exactNonEmptyString(value: unknown): string | null {
  return typeof value === 'string' && value.length > 0 && value === value.trim()
    ? value
    : null
}

function normalizeInstalledPluginMcpServer(
  pluginDir: string,
  serverId: string,
  server: unknown,
  options: {
    forceDisabled?: boolean
    manifestSha256?: string
    extensionProvider?: string
    expectedServerVersion?: string
  } = {}
): Record<string, unknown> | null {
  const raw = objectValue(server)
  const rawCwd = scalarStringValue(raw.cwd)
  const cwd = rawCwd ? resolvePluginPath(pluginDir, pluginDir, rawCwd) : pluginDir
  if (rawCwd && !cwd) return null
  const commandValue = scalarStringValue(raw.command)
  const command = commandValue ? resolvePluginPath(pluginDir, cwd, commandValue) : undefined
  if (commandValue && !command) return null
  const url = scalarStringValue(raw.url)
  const args = stringArrayValue(raw.args).map((arg) => resolvePluginPath(pluginDir, cwd, arg))
  if (args.some((arg) => !arg)) return null
  const enabled = options.forceDisabled !== true && raw.enabled !== false && raw.disabled !== true
  const headers = enabled ? stringRecordValue(raw.headers) : {}
  const accountCredential = raw.accountCredential === undefined
    ? undefined
    : McpAccountCredentialScope.safeParse(raw.accountCredential)
  if (accountCredential && !accountCredential.success) return null
  if (accountCredential?.success && (
    accountCredential.data.owner !== 'extension' ||
    accountCredential.data.provider !== options.extensionProvider
  )) return null
  const oauthBinding = raw.oauthBinding
  const env = enabled
    ? {
        ...inheritedEnvRecord(stringArrayValue(raw.env_vars).concat(stringArrayValue(raw.envVars))),
        ...stringRecordValue(raw.env)
      }
    : {}
  const transport = normalizeMcpTransport(raw.transport, command, url)
  if (!transport) return null

  const trustedWorkspaceRoots = stringArrayValue(raw.trustedWorkspaceRoots)
  const trustScope = normalizeMcpTrustScope(raw.trustScope, trustedWorkspaceRoots)
  if (trustScope === 'workspace' && trustedWorkspaceRoots.length === 0) return null

  const timeoutMs = positiveIntegerValue(raw.timeoutMs)
  const parsed = McpServerConfig.safeParse({
    enabled,
    transport,
    ...(command ? { command } : {}),
    ...(args.length > 0 ? { args } : {}),
    ...(transport === 'stdio' ? { cwd } : {}),
    ...(url ? { url } : {}),
    ...(Object.keys(headers).length > 0 ? { headers } : {}),
    ...(enabled && accountCredential?.success ? { accountCredential: accountCredential.data } : {}),
    ...(enabled && accountCredential?.success && oauthBinding !== undefined ? { oauthBinding } : {}),
    ...(Object.keys(env).length > 0 ? { env } : {}),
    trustScope,
    ...(trustedWorkspaceRoots.length > 0 ? { trustedWorkspaceRoots } : {}),
    ...(raw.lowPriority === true ? { lowPriority: true } : {}),
    ...(raw.backgroundStart === true ? { backgroundStart: true } : {}),
    ...(timeoutMs ? { timeoutMs } : {})
  })

  if (!parsed.success) return null
  const normalized = objectValue(parsed.data)
  const expectedServerVersion = exactNonEmptyString(options.expectedServerVersion)
  const entrypointPath = [command, ...args].find((candidate): candidate is string =>
    typeof candidate === 'string' && candidate.length > 0 && isAbsolute(candidate) && pathInsideOrEqual(candidate, pluginDir) && existsSync(candidate)
  ) ?? ''
  const highRiskFundsServer = ['analytix_funds', 'analytix-fund-analysis'].includes(serverId.toLowerCase())
  if (!expectedServerVersion || (highRiskFundsServer && !entrypointPath)) return null
  let manifestSha256 = ''
  let entrypointSha256 = ''
  let sourceTree: ReturnType<typeof computePluginSourceTreeIdentity>
  try {
    manifestSha256 = options.manifestSha256 ?? createHash('sha256')
      .update(readStablePluginSourceFile(join(pluginDir, '.codex-plugin', 'plugin.json'), MAX_PLUGIN_MANIFEST_JSON_BYTES))
      .digest('hex')
    entrypointSha256 = entrypointPath
      ? createHash('sha256')
          .update(readStablePluginSourceFile(entrypointPath, 64 * 1024 * 1024))
          .digest('hex')
      : ''
    sourceTree = computePluginSourceTreeIdentity(pluginDir)
  } catch {
    return null
  }
  return sealMcpAccountCredentialBinding({
    ...normalized,
    expectedServerName: serverId,
    expectedServerVersion,
    identitySource: 'installed-plugin-manifest',
    manifestSha256,
    ...(entrypointPath ? { entrypointPath, entrypointSha256 } : {}),
    pluginRootPath: sourceTree.rootRealPath,
    sourceTreeSha256: sourceTree.treeSha256
  })
}

function resolvePluginPath(pluginDir: string, cwd: string, value: string): string {
  if (!value || isAbsolute(value)) return value
  if (value === '.' || value.startsWith('./') || value.startsWith('../')) {
    const target = resolve(cwd || pluginDir, value)
    return pathInsideOrEqual(target, pluginDir) ? target : ''
  }
  return value
}

function pathInsideOrEqual(targetPath: string, rootPath: string): boolean {
  const root = resolve(rootPath)
  const target = resolve(targetPath)
  const rel = relative(root, target)
  return rel === '' || (!!rel && !rel.startsWith('..') && !isAbsolute(rel))
}

function isPersistedInstalledPluginMcpServer(
  dataDir: string,
  serverId: string,
  value: unknown
): boolean {
  const server = strictObjectValue(value)
  if (!server || server.identitySource !== 'installed-plugin-manifest') return false
  const pluginRootPath = exactNonEmptyString(server.pluginRootPath)
  const expectedServerName = exactNonEmptyString(server.expectedServerName)
  const expectedServerVersion = exactNonEmptyString(server.expectedServerVersion)
  if (
    !pluginRootPath ||
    !isAbsolute(pluginRootPath) ||
    expectedServerName !== serverId ||
    !expectedServerVersion ||
    typeof server.manifestSha256 !== 'string' ||
    !/^[a-f0-9]{64}$/.test(server.manifestSha256) ||
    typeof server.sourceTreeSha256 !== 'string' ||
    !/^[a-f0-9]{64}$/.test(server.sourceTreeSha256)
  ) return false
  const trustedRoots = installedPluginMcpCacheRoots(dataDir).map((root) => join(root, 'analytix-hub'))
  if (!trustedRoots.some((root) => pathInsideOrEqual(pluginRootPath, root))) return false
  const entrypointPath = server.entrypointPath
  const entrypointSha256 = server.entrypointSha256
  if (entrypointPath === undefined && entrypointSha256 === undefined) return true
  return typeof entrypointPath === 'string' &&
    isAbsolute(entrypointPath) &&
    pathInsideOrEqual(entrypointPath, pluginRootPath) &&
    typeof entrypointSha256 === 'string' &&
    /^[a-f0-9]{64}$/.test(entrypointSha256)
}

async function readPluginManagedMcpServerIds(
  dataDir: string,
  configBytes: Buffer | null
): Promise<PluginManagedMcpServerIdsState> {
  const fileState = readPrivateRegularFile(
    join(dataDir, GUI_PLUGIN_MCP_SERVER_IDS_PATH),
    MAX_PLUGIN_SIDECAR_JSON_BYTES
  )
  if (fileState.status !== 'valid') return { ids: new Set() }
  let value: unknown
  try {
    value = parseStrictJsonValue(fileState.bytes, {
      maxBytes: MAX_PLUGIN_SIDECAR_JSON_BYTES,
      maxDepth: 8,
      maxTokens: 1024,
      maxStringBytes: 4096
    })
  } catch {
    return { ids: new Set() }
  }
  if (Array.isArray(value)) return { ids: new Set() }
  const sidecar = strictObjectValue(value)
  if (!sidecar || Object.keys(sidecar).some((key) => ![
    'schemaVersion', 'contract', 'configSha256', 'serverIds'
  ].includes(key))) return { ids: new Set() }
  const ids = exactMcpServerIdList(sidecar.serverIds, true)
  const configSha256 = configBytes
    ? createHash('sha256').update(configBytes).digest('hex')
    : ''
  if (
    sidecar.schemaVersion !== GUI_PLUGIN_MCP_SERVER_IDS_VERSION ||
    sidecar.contract !== GUI_PLUGIN_MCP_SERVER_IDS_CONTRACT ||
    sidecar.configSha256 !== configSha256 ||
    !ids
  ) return { ids: new Set() }
  return { ids: new Set(ids) }
}

function exactMcpServerIdList(value: unknown, requireSorted: boolean): string[] | null {
  if (!Array.isArray(value)) return null
  const ids: string[] = []
  const seen = new Set<string>()
  for (const item of value) {
    if (typeof item !== 'string' || !McpServerIdV1.safeParse(item).success || seen.has(item)) return null
    seen.add(item)
    ids.push(item)
  }
  if (requireSorted && ids.some((item, index) => index > 0 && ids[index - 1] > item)) return null
  return ids
}

async function writePluginManagedMcpServerIds(
  dataDir: string,
  ids: string[],
  configText: string
): Promise<void> {
  const targetPath = join(dataDir, GUI_PLUGIN_MCP_SERVER_IDS_PATH)
  await mkdir(dirname(targetPath), { recursive: true })
  const serverIds = [...new Set(ids)].sort()
  if (serverIds.length !== ids.length || serverIds.some((id) => !McpServerIdV1.safeParse(id).success)) {
    throw new Error('Refusing to persist invalid plugin-managed MCP server IDs')
  }
  const body = `${JSON.stringify({
    schemaVersion: GUI_PLUGIN_MCP_SERVER_IDS_VERSION,
    contract: GUI_PLUGIN_MCP_SERVER_IDS_CONTRACT,
    configSha256: createHash('sha256').update(configText, 'utf8').digest('hex'),
    serverIds
  }, null, 2)}\n`
  if (existsSync(targetPath)) {
    const info = lstatSync(targetPath)
    if (!info.isFile() || info.isSymbolicLink() || info.nlink !== 1) {
      throw new Error('Refusing to replace unsafe plugin-managed MCP sidecar')
    }
    const current = readPrivateRegularFile(targetPath, MAX_PLUGIN_SIDECAR_JSON_BYTES)
    if (current.status === 'valid' && current.bytes.toString('utf8') === body) return
  }
  await atomicWriteFile(targetPath, body, { mode: 0o600 })
  if (readPrivateRegularFile(targetPath, MAX_PLUGIN_SIDECAR_JSON_BYTES).status !== 'valid') {
    throw new Error('Plugin-managed MCP sidecar was not published as a private regular file')
  }
}

function normalizeGuiManagedMcpServer(serverId: string, server: unknown): Record<string, unknown> | null {
  const raw = objectValue(server)
  const command = scalarStringValue(raw.command)
  const url = scalarStringValue(raw.url)
  const args = stringArrayValue(raw.args)
  const cwd = scalarStringValue(raw.cwd)
  const enabled = raw.enabled !== false && raw.disabled !== true
  const headers = enabled ? stringRecordValue(raw.headers) : {}
  const accountCredential = raw.accountCredential === undefined
    ? undefined
    : McpAccountCredentialScope.safeParse(raw.accountCredential)
  if (accountCredential && !accountCredential.success) return null
  if (accountCredential?.success && (
    accountCredential.data.owner !== 'mcp' ||
    accountCredential.data.provider !== serverId
  )) return null
  const oauthBinding = raw.oauthBinding
  const env = enabled ? stringRecordValue(raw.env) : {}
  const transport = normalizeMcpTransport(raw.transport, command, url)
  if (!transport) return null

  const trustedWorkspaceRoots = stringArrayValue(raw.trustedWorkspaceRoots)
  const trustScope = normalizeMcpTrustScope(raw.trustScope, trustedWorkspaceRoots)
  if (trustScope === 'workspace' && trustedWorkspaceRoots.length === 0) return null

  const timeoutMs = positiveIntegerValue(raw.timeoutMs)
  const parsed = McpServerConfig.safeParse({
    enabled,
    transport,
    ...(command ? { command } : {}),
    ...(args.length > 0 ? { args } : {}),
    ...(transport === 'stdio' && cwd ? { cwd } : {}),
    ...(url ? { url } : {}),
    ...(Object.keys(headers).length > 0 ? { headers } : {}),
    ...(enabled && accountCredential?.success ? { accountCredential: accountCredential.data } : {}),
    ...(enabled && accountCredential?.success && oauthBinding !== undefined ? { oauthBinding } : {}),
    ...(Object.keys(env).length > 0 ? { env } : {}),
    trustScope,
    ...(trustedWorkspaceRoots.length > 0 ? { trustedWorkspaceRoots } : {}),
    ...(raw.lowPriority === true ? { lowPriority: true } : {}),
    ...(raw.backgroundStart === true ? { backgroundStart: true } : {}),
    ...(timeoutMs ? { timeoutMs } : {})
  })

  return parsed.success ? sealMcpAccountCredentialBinding(objectValue(parsed.data)) : null
}

function normalizeMcpTransport(
  value: unknown,
  command: string | undefined,
  url: string | undefined
): 'stdio' | 'streamable-http' | 'sse' | null {
  if (value === 'stdio' || value === 'streamable-http' || value === 'sse') return value
  if (command) return 'stdio'
  if (url) return 'streamable-http'
  return null
}

function normalizeMcpTrustScope(
  value: unknown,
  trustedWorkspaceRoots: string[]
): 'user' | 'workspace' {
  if (value === 'user' || value === 'workspace') return value
  return trustedWorkspaceRoots.length > 0 ? 'workspace' : 'user'
}

function scalarStringValue(value: unknown): string | undefined {
  return typeof value === 'string'
    ? value
    : typeof value === 'number' || typeof value === 'boolean'
      ? String(value)
      : undefined
}

function stringRecordValue(value: unknown): Record<string, string> {
  const record = objectValue(value)
  const next: Record<string, string> = {}
  for (const [key, item] of Object.entries(record)) {
    const normalized = scalarStringValue(item)
    if (normalized !== undefined) next[key] = normalized
  }
  return next
}

function inheritedEnvRecord(keys: string[]): Record<string, string> {
  const next: Record<string, string> = {}
  for (const key of keys) {
    const name = key.trim()
    if (!name || Object.prototype.hasOwnProperty.call(next, name)) continue
    const value = process.env[name]
    if (typeof value === 'string') next[name] = value
  }
  return next
}

function positiveIntegerValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : undefined
}

function modelConfigProfilesFromProviderProfiles(
  profiles: Record<string, ModelProviderModelProfileV1>
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [modelId, profile] of Object.entries(profiles)) {
    const trimmed = modelId.trim()
    if (!trimmed) continue
    out[trimmed] = {
      ...(profile.aliases?.length ? { aliases: profile.aliases } : {}),
      ...(profile.contextWindowTokens ? { contextWindowTokens: profile.contextWindowTokens } : {}),
      inputModalities: profile.inputModalities,
      outputModalities: profile.outputModalities,
      supportsToolCalling: profile.supportsToolCalling,
      messageParts: profile.messageParts,
      ...(profile.reasoning ? { reasoning: profile.reasoning } : {}),
      ...(profile.endpointFormat ? { endpointFormat: profile.endpointFormat } : {}),
      ...(profile.price ? { price: profile.price } : {})
    }
  }
  return out
}

function runtimeModelProviderBaseUrl(provider: ModelProviderProfileV1): string {
  const explicit = provider.baseUrl.trim()
  if (explicit) return explicit
  if (provider.id === DEFAULT_MODEL_PROVIDER_ID) return DEFAULT_DEEPSEEK_BASE_URL
  const preset = getModelProviderPreset(provider.id)
  return preset ? modelProviderPresetProfile(preset).baseUrl.trim() : ''
}

export function buildRuntimeModelProvidersEnv(
  settings: AppSettingsV1,
  runtime: Pick<AnalytixRuntimeSettingsV1, 'providerId'>
): string | undefined {
  const providerSettings = getModelProviderSettings(settings)
  const providers = providerSettings.providers
    .map((provider) => {
      const id = provider.id.trim()
      const baseUrl = runtimeModelProviderBaseUrl(provider)
      if (!id || !baseUrl) return null
      return {
        id,
        ...(provider.name.trim() ? { name: provider.name.trim() } : {}),
        baseUrl,
        endpointFormat: provider.endpointFormat,
        models: provider.models.map((model) => model.trim()).filter(Boolean),
        modelProfiles: modelConfigProfilesFromProviderProfiles(provider.modelProfiles),
        ...(provider.price ? { price: provider.price } : {}),
        ...(provider.prices ? { prices: provider.prices } : {})
      }
    })
    .filter((provider): provider is NonNullable<typeof provider> => provider !== null)
  if (providers.length === 0) return undefined
  const parsed = ModelProvidersConfigSchema.parse({
    defaultProviderId: runtime.providerId.trim() || providerSettings.activeProviderId || DEFAULT_MODEL_PROVIDER_ID,
    providers
  })
  return JSON.stringify(parsed)
}

function visionBridgeConfigForRuntime(
  runtime: Pick<AnalytixRuntimeSettingsV1, 'visionBridge' | 'modelCapabilityProbes'>
): Record<string, unknown> {
  const visionBridge = runtime.visionBridge
  const next: Record<string, unknown> = {
    enabled: visionBridge.enabled,
    mode: visionBridge.mode
  }
  const fields = {
    providerId: visionBridge.providerId,
    baseUrl: visionBridge.baseUrl,
    model: visionBridge.model
  }
  for (const [key, value] of Object.entries(fields)) {
    const trimmed = value.trim()
    if (trimmed) next[key] = trimmed
  }
  Object.assign(next, {
    endpointFormat: visionBridge.endpointFormat,
    maxImageDimension: visionBridge.maxImageDimension,
    maxImageBytes: visionBridge.maxImageBytes,
    maxScreenshotsPerTurn: visionBridge.maxScreenshotsPerTurn,
    observationCacheTtlMs: visionBridge.observationCacheTtlMs,
    injectPolicy: visionBridge.injectPolicy,
    fallbackWhenPrimaryImageUnsupported: visionBridge.fallbackWhenPrimaryImageUnsupported,
    semanticProbeStatus: visionBridgeSemanticProbeStatus(runtime)
  })
  return next
}

function visionBridgeSemanticProbeStatus(
  runtime: Pick<AnalytixRuntimeSettingsV1, 'visionBridge' | 'modelCapabilityProbes'>
): string {
  const bridge = runtime.visionBridge
  if (!bridge.enabled || bridge.mode === 'off') return 'unknown'
  if (!bridge.providerId.trim() || !bridge.model.trim() || !bridge.baseUrl.trim()) return 'unknown'
  const key = modelCapabilityProbeKey({
    providerId: bridge.providerId,
    model: bridge.model,
    baseUrl: bridge.baseUrl,
    endpointFormat: bridge.endpointFormat
  })
  const probe = runtime.modelCapabilityProbes[key]
  if (!probe) return 'unknown'
  if (Date.parse(probe.staleAfter) <= Date.now()) return 'stale'
  return probe.status
}

function runtimeTuningConfigForRuntime(
  runtimeTuning: Pick<AnalytixRuntimeSettingsV1, 'runtimeTuning'>['runtimeTuning'],
  existing: Record<string, unknown>
): Record<string, unknown> {
  const existingStepLimits = objectValue(existing.stepLimits)
  return {
    streamIdleTimeoutMs: runtimeTuning.streamIdleTimeoutMs,
    stepLimits: {
      ...existingStepLimits,
      defaultMaxModelSteps: runtimeTuning.stepLimits.defaultMaxModelSteps,
      userGlobalMaxModelSteps: runtimeTuning.stepLimits.userGlobalMaxModelSteps,
      plannerMaxModelSteps: runtimeTuning.stepLimits.plannerMaxModelSteps,
      headlessMaxModelSteps: runtimeTuning.stepLimits.headlessMaxModelSteps
    }
  }
}

async function readStrictJsonObjectState(
  path: string,
  maxBytes: number
): Promise<StrictJsonReadState<Record<string, unknown>>> {
  const state = await readStrictJsonValueState(path, maxBytes)
  if (state.status !== 'valid') return state
  try {
    return {
      status: 'valid',
      value: parseStrictJsonObject(state.bytes, {
        maxBytes,
        maxDepth: 32,
        maxTokens: 131_072,
        maxStringBytes: Math.min(maxBytes, 1 << 20)
      }),
      bytes: state.bytes
    }
  } catch {
    return { status: 'invalid' }
  }
}

async function readStrictJsonValueState(
  path: string,
  maxBytes: number
): Promise<StrictJsonReadState<unknown>> {
  let bytes: Buffer
  try {
    bytes = await readFile(path)
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return { status: 'missing' }
    throw error
  }
  try {
    return {
      status: 'valid',
      value: parseStrictJsonValue(bytes, {
        maxBytes,
        maxDepth: 32,
        maxTokens: 131_072,
        maxStringBytes: Math.min(maxBytes, 1 << 20)
      }),
      bytes
    }
  } catch {
    return { status: 'invalid' }
  }
}

type SafeParseSchema = {
  safeParse: (value: unknown) =>
    | { success: true; data: unknown }
    | { success: false }
}

function parseAnalytixConfigSection(
  schema: SafeParseSchema,
  value: unknown
): Record<string, unknown> {
  const parsed = schema.safeParse(objectValue(value))
  return parsed.success ? objectValue(parsed.data) : {}
}

function subagentsConfigForRuntime(
  subagents: AnalytixSubagentsSettingsV1,
  existing: Record<string, unknown>
): Record<string, unknown> {
  const profiles: Record<string, unknown> = { ...objectValue(existing.profiles) }
  for (const name of Object.keys(subagents.profiles).sort()) {
    const trimmedName = name.trim()
    if (!trimmedName) continue
    const profile = subagents.profiles[name]
    const nextProfile: Record<string, unknown> = {
      toolPolicy: profile.toolPolicy
    }
    if (profile.prompt.trim()) nextProfile.promptPreamble = profile.prompt.trim()
    if (profile.model.trim()) nextProfile.model = profile.model.trim()
    if (profile.effort) nextProfile.effort = profile.effort
    if (profile.tools.length > 0) nextProfile.tools = [...profile.tools]
    profiles[trimmedName] = nextProfile
  }
  const requestedDefaultProfile = subagents.defaultProfile.trim()
  const existingDefaultProfile = typeof existing.defaultProfile === 'string'
    ? existing.defaultProfile.trim()
    : ''
  const defaultProfile = requestedDefaultProfile || existingDefaultProfile
  const next: Record<string, unknown> = {
    ...existing,
    enabled: subagents.enabled,
    maxParallel: subagents.maxParallel,
    maxChildRuns: subagents.maxChildRuns,
    defaultToolPolicy: subagents.defaultToolPolicy,
    profiles
  }
  if (defaultProfile && Object.prototype.hasOwnProperty.call(profiles, defaultProfile)) {
    next.defaultProfile = defaultProfile
  } else {
    delete next.defaultProfile
  }
  return next
}

function sanitizeAnalytixCapabilitiesConfig(value: unknown): Record<string, unknown> {
  const raw = objectValue(value)
  const next: Record<string, unknown> = {}
  if ('mcp' in raw) next.mcp = parseAnalytixConfigSection(McpCapabilityConfig, raw.mcp)
  if ('web' in raw) next.web = parseAnalytixConfigSection(WebCapabilityConfig, raw.web)
  if ('skills' in raw) next.skills = parseAnalytixConfigSection(SkillsCapabilityConfig, raw.skills)
  if ('subagents' in raw) {
    next.subagents = parseAnalytixConfigSection(SubagentsCapabilityConfig, raw.subagents)
  }
  if ('visionBridge' in raw) {
    next.visionBridge = parseAnalytixConfigSection(VisionBridgeCapabilityConfig, raw.visionBridge)
  }
  return next
}

function sanitizeProductionAnalytixConfig(
  existing: Record<string, unknown> | null
): Record<string, unknown> | null {
  if (!existing) return null
  return {
    runtime: parseAnalytixConfigSection(RuntimeTuningConfigSchema, existing.runtime),
    capabilities: sanitizeAnalytixCapabilitiesConfig(existing.capabilities)
  }
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

export async function stopAnalytixChildAndWait(): Promise<void> {
  if (!child) {
    if (childLogCapture) {
      const capture = childLogCapture
      childLogCapture = null
      await capture.close()
    }
    return
  }
  const stoppingChild = child
  intentionalStops.add(stoppingChild)
  const pid = child.pid
  const capture = childLogCapture
  if (stoppingChild.exitCode === null && stoppingChild.signalCode === null) {
    try {
      stoppingChild.kill('SIGTERM')
    } catch {
      /* already gone */
    }
  }
  const exited = await waitForChildExit(stoppingChild, ANALYTIX_STOP_GRACE_MS)
  if (!exited) {
    try {
      if (pid) process.kill(pid, 'SIGKILL')
    } catch {
      /* already gone */
    }
    await waitForChildExit(stoppingChild, ANALYTIX_STOP_FORCE_MS)
  }
  if (child === stoppingChild) child = null
  if (capture) {
    childLogCapture = null
    await capture.close()
  }
}

function waitForChildExit(process: ChildProcess, timeoutMs: number): Promise<boolean> {
  if (process.exitCode !== null || process.signalCode !== null) return Promise.resolve(true)
  return new Promise((resolve) => {
    let settled = false
    const timer = setTimeout(() => settle(false), timeoutMs)
    const settle = (exited: boolean): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      process.removeListener('exit', onExit)
      process.removeListener('error', onError)
      resolve(exited)
    }
    const onExit = (): void => settle(true)
    const onError = (): void => settle(true)
    process.once('exit', onExit)
    process.once('error', onError)
  })
}

export async function reclaimAnalytixPort(
  port: number
): Promise<{ ok: true } | { ok: false; message: string }> {
  if (port <= 0) return { ok: true }
  if (await canBindTcpPort(port, '127.0.0.1')) return { ok: true }
  if (await killStaleAnalytixOnPort(port) && await canBindTcpPort(port, '127.0.0.1')) {
    return { ok: true }
  }
  return { ok: false, message: `port ${port} is in use` }
}

export async function resolveAvailableAnalytixPort(
  preferredPort: number
): Promise<{ port: number; changed: boolean; message?: string }> {
  if (preferredPort > 0) {
    if (await canBindTcpPort(preferredPort, '127.0.0.1')) {
      return { port: preferredPort, changed: false }
    }
    // Prefer reclaiming the configured port from a stale analytix left by a
    // crashed previous app run over silently moving to a new port.
    if (
      await killStaleAnalytixOnPort(preferredPort) &&
      await canBindTcpPort(preferredPort, '127.0.0.1')
    ) {
      return { port: preferredPort, changed: false }
    }
    for (let port = preferredPort + 1; port <= MAX_TCP_PORT; port += 1) {
      if (await canBindTcpPort(port, '127.0.0.1')) {
        return {
          port,
          changed: true,
          message: `port ${preferredPort} is in use`
        }
      }
    }
  }
  const port = await allocateTcpPort('127.0.0.1')
  return {
    port,
    changed: true,
    ...(preferredPort > 0 ? { message: `port ${preferredPort} is in use` } : {})
  }
}

/**
 * Kill a stale analytix serve process from a previous app run that is still
 * holding the configured port. Only processes whose command line looks
 * like our serve entry are touched; anything else keeps the port and we
 * fall back to allocating a different one.
 *
 * Safe by construction on every platform: any failure to positively
 * identify the holder as our own serve-entry leaves it untouched and the
 * caller allocates a different port instead.
 */
async function killStaleAnalytixOnPort(port: number): Promise<boolean> {
  const pids = await listListeningPidsOnPort(port)
  let reclaimed = false
  for (const pid of pids) {
    let command = ''
    try {
      command = await processCommandLine(pid)
    } catch {
      continue
    }
    if (!commandLooksLikeStaleAnalytixRuntime(command)) continue
    void appendManagedChildLogRecord({
      code: 'ANALYTIX_STALE_CHILD_TERMINATION',
      port
    })
    if (await terminateStalePid(pid)) reclaimed = true
  }
  return reclaimed
}

export function commandLooksLikeStaleAnalytixRuntime(command: string): boolean {
  const normalized = command.replace(/\\/g, '/').toLowerCase()
  if (normalized.includes('serve-entry')) return true

  const runtimeServerPattern = /(^|[\s/])runtime-server(?:\s|$)/
  const hasRuntimeServerBinary =
    runtimeServerPattern.test(normalized) ||
    normalized.includes('cmd/runtime-server')
  if (!hasRuntimeServerBinary) return false

  const hasSidecarRuntimeFlags =
    normalized.includes('--addr') &&
    normalized.includes('--runtime-token') &&
    (
      normalized.includes('--fixtures-dir') ||
      normalized.includes('--durable-root') ||
      normalized.includes('--candidate-durable-root') ||
      normalized.includes('--durable-temp-dir')
    )
  const hasServeRuntimeFlags =
    normalized.includes('--host') &&
    normalized.includes('--port') &&
    normalized.includes('--data-dir') &&
    (
      normalized.includes('--model') ||
      normalized.includes('--endpoint-format') ||
      normalized.includes('--approval-policy')
    )
  const hasNativeRuntimeFlags =
    normalized.includes('--addr') &&
    normalized.includes('--data-dir') &&
    (normalized.includes('--durable-root') || normalized.includes('--runtime-durable-root'))

  return (hasSidecarRuntimeFlags || hasServeRuntimeFlags || hasNativeRuntimeFlags) && normalized.includes('analytix')
}

/**
 * PIDs listening on `port`, excluding our own process. Uses `lsof` on
 * macOS/Linux and `netstat -ano` on Windows.
 */
async function listListeningPidsOnPort(port: number): Promise<number[]> {
  if (process.platform === 'win32') {
    try {
      const { stdout } = await execFileAsync('netstat', ['-ano'], {
        windowsHide: true,
        timeout: 5_000,
        maxBuffer: 8 * 1024 * 1024
      })
      return parseListeningPidsFromNetstat(stdout, port)
    } catch {
      return []
    }
  }
  try {
    const { stdout } = await execFileAsync('lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN'])
    return stdout
      .split('\n')
      .map((line) => Number(line.trim()))
      .filter((pid) => Number.isInteger(pid) && pid > 0 && pid !== process.pid)
  } catch {
    return []
  }
}

/**
 * Parse `netstat -ano` output into the PIDs holding a LISTENING TCP socket
 * on `port`. Columns are `Proto  Local  Foreign  State  PID`; UDP rows
 * (no State column) and non-matching ports are ignored. Matches both IPv4
 * (`127.0.0.1:<port>`) and IPv6 (`[::1]:<port>`) local addresses.
 */
export function parseListeningPidsFromNetstat(stdout: string, port: number): number[] {
  const pids = new Set<number>()
  for (const raw of stdout.split(/\r?\n/)) {
    const cols = raw.trim().split(/\s+/)
    if (cols.length < 5 || cols[0].toUpperCase() !== 'TCP') continue
    if (cols[3].toUpperCase() !== 'LISTENING') continue
    if (!cols[1].endsWith(`:${port}`)) continue
    const pid = Number(cols[cols.length - 1])
    if (Number.isInteger(pid) && pid > 0 && pid !== process.pid) pids.add(pid)
  }
  return [...pids]
}

/** Read a process's full command line (best effort, platform-specific). */
async function processCommandLine(pid: number): Promise<string> {
  if (process.platform === 'win32') {
    const { stdout } = await execFileAsync(
      'powershell',
      [
        '-NoProfile',
        '-NonInteractive',
        '-Command',
        `(Get-CimInstance Win32_Process -Filter 'ProcessId=${pid}').CommandLine`
      ],
      { windowsHide: true, timeout: 5_000 }
    )
    return stdout.trim()
  }
  const { stdout } = await execFileAsync('ps', ['-p', String(pid), '-o', 'command='])
  return stdout.trim()
}

/** Terminate a positively-identified stale analytix process. */
async function terminateStalePid(pid: number): Promise<boolean> {
  if (process.platform === 'win32') {
    try {
      await execFileAsync('taskkill', ['/PID', String(pid), '/T', '/F'], {
        windowsHide: true,
        timeout: 5_000
      })
      return true
    } catch {
      // taskkill exits non-zero when the PID is already gone — treat the
      // port as reclaimed only if the process really is no longer alive.
      return await waitForPidExit(pid, 0)
    }
  }
  try {
    process.kill(pid, 'SIGTERM')
  } catch {
    return false
  }
  if (!(await waitForPidExit(pid, 2_000))) {
    try {
      process.kill(pid, 'SIGKILL')
    } catch {
      /* already gone */
    }
    await waitForPidExit(pid, 1_000)
  }
  return true
}

async function waitForPidExit(pid: number, timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  for (;;) {
    try {
      process.kill(pid, 0)
    } catch {
      return true
    }
    if (Date.now() >= deadline) return false
    await sleep(100)
  }
}

function canBindTcpPort(port: number, host: string): Promise<boolean> {
  return new Promise((resolve) => {
    let settled = false
    const server = createServer()
    const settle = (available: boolean): void => {
      if (settled) return
      settled = true
      server.removeAllListeners('error')
      resolve(available)
    }
    server.unref()
    server.once('error', () => settle(false))
    server.listen({ port, host, exclusive: true }, () => {
      server.close(() => settle(true))
    })
  })
}

function allocateTcpPort(host: string): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer()
    const cleanup = (): void => {
      server.removeAllListeners('error')
      server.removeAllListeners('listening')
    }
    server.unref()
    server.once('error', (error) => {
      cleanup()
      reject(error)
    })
    server.listen({ port: 0, host, exclusive: true }, () => {
      const address = server.address()
      const port = typeof address === 'object' && address ? address.port : 0
      server.close((error) => {
        cleanup()
        if (error) reject(error)
        else if (port > 0) resolve(port)
        else reject(new Error('failed to allocate an available Analytix port'))
      })
    })
  })
}

async function waitForAnalytixStartup(startedChild: ChildProcess, port?: number): Promise<void> {
  if (startedChild.exitCode !== null) {
    throw new Error(describeAnalytixExit(startedChild.exitCode, null))
  }
  await new Promise<void>((resolve, reject) => {
    let settled = false
    let stdoutBuffer = ''
    let stderrTail = ''
    let healthProbeInFlight = false
    const timer = setTimeout(() => {
      if (settled) return
      settled = true
      cleanup()
      reject(new Error(describeAnalytixStartupTimeout(stderrTail)))
    }, ANALYTIX_STARTUP_TIMEOUT_MS)
    // The stdout ready marker can lag behind the actual server (pipe
    // buffering) or get lost in unusual spawn environments; the HTTP
    // health endpoint is the ground truth, so poll it in parallel.
    const healthTimer = port
      ? setInterval(() => {
          if (settled || healthProbeInFlight) return
          healthProbeInFlight = true
          void probeAnalytixHealth(port)
            .then((healthy) => {
              if (healthy) settleReady()
            })
            .finally(() => {
              healthProbeInFlight = false
            })
        }, ANALYTIX_STARTUP_HEALTH_POLL_MS)
      : null
    const cleanup = (): void => {
      clearTimeout(timer)
      if (healthTimer) clearInterval(healthTimer)
      startedChild.removeListener('exit', onExit)
      startedChild.removeListener('error', onError)
      startedChild.stdout?.removeListener('data', onStdout)
      startedChild.stderr?.removeListener('data', onStderr)
    }
    const tryParseReady = (): boolean => {
      const markerIndex = stdoutBuffer.indexOf(ANALYTIX_READY_PREFIX)
      if (markerIndex < 0) return false
      const afterPrefix = stdoutBuffer.slice(markerIndex + ANALYTIX_READY_PREFIX.length)
      const newlineIndex = afterPrefix.indexOf('\n')
      if (newlineIndex < 0) return false
      const jsonLine = afterPrefix.slice(0, newlineIndex).trim()
      if (!jsonLine) return false
      try {
        const parsed = JSON.parse(jsonLine) as { service?: string; mode?: string; port?: number }
        return parsed.service === 'analytix' && parsed.mode === 'serve' && typeof parsed.port === 'number'
      } catch {
        return false
      }
    }
    const settleReady = (): void => {
      if (settled) return
      settled = true
      cleanup()
      resolve()
    }
    const onStdout = (chunk: Buffer | string): void => {
      stdoutBuffer = appendTail(stdoutBuffer, String(chunk), STDERR_TAIL_MAX_CHARS * 2)
      if (tryParseReady()) settleReady()
    }
    const onStderr = (chunk: Buffer | string): void => {
      stderrTail = appendTail(stderrTail, String(chunk))
    }
    const onExit = (code: number | null, signal: NodeJS.Signals | null): void => {
      if (settled) return
      settled = true
      cleanup()
      reject(new Error(describeAnalytixExit(code, signal, stderrTail)))
    }
    const onError = (error: Error): void => {
      if (settled) return
      settled = true
      cleanup()
      reject(error)
    }
    startedChild.stdout?.on('data', onStdout)
    startedChild.stderr?.on('data', onStderr)
    startedChild.once('exit', onExit)
    startedChild.once('error', onError)
  })
}

function describeAnalytixExit(
  code: number | null,
  signal: NodeJS.Signals | null,
  stderrTail = ''
): string {
  const diagnostic = analytixStderrDiagnostic(stderrTail)
  const suffix = diagnostic.stderrBytes > 0
    ? ` (stderr_bytes=${diagnostic.stderrBytes}, stderr_sha256=${diagnostic.stderrSha256})`
    : ''
  if (signal) return `Analytix exited during startup with signal ${signal}${suffix}`
  if (typeof code === 'number') return `Analytix exited during startup with code ${code}${suffix}`
  return `Analytix exited during startup${suffix}`
}

function describeAnalytixStartupTimeout(stderrTail: string): string {
  const diagnostic = analytixStderrDiagnostic(stderrTail)
  const suffix = diagnostic.stderrBytes > 0
    ? ` (stderr_bytes=${diagnostic.stderrBytes}, stderr_sha256=${diagnostic.stderrSha256})`
    : ''
  return `Analytix did not report ready within ${ANALYTIX_STARTUP_TIMEOUT_MS}ms${suffix}`
}

async function probeAnalytixHealth(port: number): Promise<boolean> {
  try {
    const response = await fetch(`http://127.0.0.1:${port}/health`, {
      signal: AbortSignal.timeout(ANALYTIX_STARTUP_HEALTH_REQUEST_TIMEOUT_MS)
    })
    if (!response.ok) return false
    return isAnalytixHealthResponseBody(await response.text())
  } catch {
    return false
  }
}
