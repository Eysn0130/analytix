import { access } from 'node:fs/promises'
import { basename, delimiter, dirname, isAbsolute, join, posix, win32 } from 'node:path'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js'
import type {
  HostAppState,
  HostControlAvailability,
  HostControlBackend,
  HostControlDoctorResult,
  HostControlTarget,
  HostScreenshot,
  MouseButton,
  ScrollDirection
} from './host-control.js'

type McpClientLike = {
  listTools(options?: { signal?: AbortSignal; timeout?: number }): Promise<{ tools: Array<{ name?: string }> }>
  callTool(
    input: { name: string; arguments: Record<string, unknown> },
    options?: { signal?: AbortSignal; timeout?: number }
  ): Promise<unknown>
  close(): Promise<void>
}

export type OpenComputerUseBackendOptions = {
  command?: string
  cwd?: string
  env?: NodeJS.ProcessEnv
  timeoutMs?: number
  maxImageDimension?: number
  maxImageBytes?: number
  clientFactory?: (command: string, options: OpenComputerUseBackendOptions) => Promise<McpClientLike>
}

type NormalizedMcpResult = {
  raw: unknown
  payload: Record<string, unknown>
  images: Array<{ mimeType: string; dataBase64: string; width?: number; height?: number }>
  isError: boolean
  text: string
}

const DEFAULT_TIMEOUT_MS = 15_000
const execFileAsync = promisify(execFile)

export class OpenComputerUseBackend implements HostControlBackend {
  readonly id = 'analytix-computer-use'
  readonly platform = process.platform
  private client: McpClientLike | null = null
  private connectPromise: Promise<HostControlAvailability> | null = null
  private lastReason?: string
  private lastApp?: string
  private toolNames = new Set<string>()

  constructor(private readonly options: OpenComputerUseBackendOptions = {}) {}

  async ensureReady(): Promise<HostControlAvailability> {
    if (!this.connectPromise) {
      this.connectPromise = this.connect().catch((error) => {
        const reason = summarizeError(error)
        this.lastReason = reason
        return { available: false, reason }
      })
    }
    return this.connectPromise
  }

  async availability(): Promise<HostControlAvailability> {
    return this.ensureReady()
  }

  async doctor(): Promise<HostControlDoctorResult> {
    const command = await resolveOpenComputerUseCommand(this.options.command)
    const checks: HostControlDoctorResult['checks'] = []
    if (!command) {
      const reason = openComputerUseInstallHint()
      return {
        backendId: this.id,
        platform: this.platform,
        available: false,
        reason,
        checks: [{ id: 'analytix-computer-use-command', status: 'failed', message: reason }]
      }
    }
    checks.push({
      id: 'analytix-computer-use-command',
      status: 'passed',
      message: `Analytix Computer Use command resolved: ${command}`
    })
    try {
      await execFileAsync(command, ['doctor'], {
        cwd: this.options.cwd,
        env: backendEnv(this.options),
        timeout: Math.min(this.options.timeoutMs ?? DEFAULT_TIMEOUT_MS, 10_000)
      })
      checks.push({
        id: 'analytix-computer-use-doctor',
        status: 'passed',
        message: 'Analytix Computer Use doctor completed successfully.'
      })
    } catch (error) {
      checks.push({
        id: 'analytix-computer-use-doctor',
        status: 'warning',
        message: `Analytix Computer Use doctor did not pass: ${summarizeError(error)}`
      })
    }
    const ready = await this.ensureReady()
    checks.push({
      id: 'analytix-computer-use-mcp',
      status: ready.available ? 'passed' : 'failed',
      message: ready.available
        ? `MCP backend connected with ${this.toolNames.size} tool(s).`
        : ready.reason ?? 'MCP backend is unavailable.'
    })
    return {
      backendId: this.id,
      platform: this.platform,
      available: ready.available,
      ...(ready.reason ? { reason: ready.reason } : {}),
      checks
    }
  }

  async listApps(): Promise<unknown[]> {
    const result = await this.call('list_apps', {})
    const apps = result.payload.apps ?? result.payload.applications ?? result.payload.result
    if (Array.isArray(apps)) return apps
    return parseOpenComputerUseAppList(result.text)
  }

  async getAppState(app?: string): Promise<HostAppState> {
    if (app?.trim()) this.lastApp = app.trim()
    const result = await this.call('get_app_state', appArg(this.lastApp))
    const screenshot = screenshotFromResult(result)
    const tree = result.payload.accessibilityTree ??
      result.payload.accessibility_tree ??
      result.payload.tree ??
      result.payload.axTree ??
      result.payload.ax_tree
    const elements = result.payload.elements ?? result.payload.interactive_elements
    const parsedTextState = parseOpenComputerUseStateText(result.text)
    const accessibilityTree = tree ?? parsedTextState.accessibilityTree
    const interactiveElements = Array.isArray(elements) ? elements : parsedTextState.elements
    return {
      backendId: this.id,
      screenshot,
      ...(result.payload.app !== undefined ? { app: result.payload.app } : parsedTextState.app ? { app: parsedTextState.app } : {}),
      ...(result.payload.window !== undefined ? { window: result.payload.window } : parsedTextState.window ? { window: parsedTextState.window } : {}),
      accessibilityTree,
      elements: interactiveElements,
      warnings: stringArray(result.payload.warnings)
    }
  }

  async capture(app?: string): Promise<HostScreenshot> {
    return (await this.getAppState(app)).screenshot
  }

  async click(
    target: HostControlTarget = {},
    button: MouseButton = 'left',
    count: 1 | 2 = 1,
    modifiers: string[] = []
  ): Promise<void> {
    if (modifiers.length > 0) {
      throw new Error('Analytix Computer Use backend does not support click modifiers')
    }
    await this.call('click', targetArgs(target, {
      mouse_button: button,
      click_count: count
    }, this.lastApp))
  }

  async performSecondaryAction(target: HostControlTarget, action: string): Promise<void> {
    await this.call('perform_secondary_action', targetArgs(target, { action }, this.lastApp))
  }

  async drag(start: HostControlTarget, end: HostControlTarget): Promise<void> {
    await this.call('drag', {
      ...appArg(start.app ?? end.app ?? this.lastApp),
      ...coordinateArgs(start, 'from'),
      ...coordinateArgs(end, 'to')
    })
  }

  async scroll(target: HostControlTarget | undefined, direction: ScrollDirection, amount = 3): Promise<void> {
    if (!target || !Number.isInteger(target.elementIndex)) {
      throw new Error('Analytix Computer Use scroll requires element_index from get_app_state')
    }
    await this.call('scroll', targetArgs(target, { direction, pages: amount }, this.lastApp))
  }

  async typeText(text: string, target?: HostControlTarget): Promise<void> {
    if (!text) return
    await this.call('type_text', appArg(target?.app ?? this.lastApp, { text }))
  }

  async pressKey(key: string, target?: HostControlTarget): Promise<void> {
    await this.call('press_key', targetArgs(target ?? {}, { key }, this.lastApp))
  }

  async setValue(value: string, target?: HostControlTarget): Promise<void> {
    await this.call('set_value', targetArgs(target ?? {}, { value }, this.lastApp))
  }

  async cursorPosition(): Promise<{ x: number; y: number }> {
    const state = await this.getAppState(this.lastApp)
    const cursor = isRecord(state.window) ? (state.window.cursor ?? state.window.cursor_position) : undefined
    if (Array.isArray(cursor) && cursor.length >= 2) {
      const x = Number(cursor[0])
      const y = Number(cursor[1])
      if (Number.isFinite(x) && Number.isFinite(y)) return { x: Math.round(x), y: Math.round(y) }
    }
    return { x: 0, y: 0 }
  }

  async screenSize(): Promise<{ width: number; height: number }> {
    const shot = await this.capture(this.lastApp)
    return { width: shot.width, height: shot.height }
  }

  async moveTo(x: number, y: number): Promise<void> {
    await this.call('move', { x, y, coordinate: [x, y] })
  }

  async wait(ms: number, signal?: AbortSignal): Promise<void> {
    const clamped = Math.max(0, Math.min(ms, 60_000))
    if (clamped === 0 || signal?.aborted) return
    await new Promise<void>((resolve) => {
      const cleanup = (): void => {
        clearTimeout(timer)
        signal?.removeEventListener('abort', onAbort)
      }
      const onAbort = (): void => {
        cleanup()
        resolve()
      }
      const timer = setTimeout(() => {
        cleanup()
        resolve()
      }, clamped)
      signal?.addEventListener('abort', onAbort, { once: true })
    })
  }

  async close(): Promise<void> {
    await this.client?.close().catch(() => undefined)
    this.client = null
    this.connectPromise = null
  }

  private async connect(): Promise<HostControlAvailability> {
    const command = await resolveOpenComputerUseCommand(this.options.command)
    if (!command) {
      const reason = openComputerUseInstallHint()
      this.lastReason = reason
      return { available: false, reason }
    }
    const client = await (this.options.clientFactory ?? createOpenComputerUseMcpClient)(command, this.options)
    const tools = await client.listTools({ timeout: this.options.timeoutMs ?? DEFAULT_TIMEOUT_MS })
    this.toolNames = new Set(tools.tools.map((tool) => tool.name).filter((name): name is string => Boolean(name)))
    if (!this.toolNames.has('get_app_state') || !this.toolNames.has('list_apps')) {
      await client.close().catch(() => undefined)
      const reason = `Analytix Computer Use MCP is missing required tools: ${['list_apps', 'get_app_state'].filter((name) => !this.toolNames.has(name)).join(', ')}`
      this.lastReason = reason
      return { available: false, reason }
    }
    this.client = client
    this.lastReason = undefined
    return { available: true }
  }

  private async call(name: string, args: Record<string, unknown>): Promise<NormalizedMcpResult> {
    const ready = await this.ensureReady()
    if (!ready.available || !this.client) {
      throw new Error(ready.reason ?? this.lastReason ?? 'Analytix Computer Use MCP backend is unavailable')
    }
    const result = normalizeMcpResult(await this.client.callTool(
      { name, arguments: pruneUndefined(args) },
      { timeout: this.options.timeoutMs ?? DEFAULT_TIMEOUT_MS }
    ))
    if (result.isError) {
      throw new Error(result.text || `Analytix Computer Use tool ${name} returned an error`)
    }
    return result
  }
}

async function createOpenComputerUseMcpClient(
  command: string,
  options: OpenComputerUseBackendOptions
): Promise<McpClientLike> {
  const client = new Client({ name: 'analytix-computer-use', version: '0.1.0' })
  const transport = new StdioClientTransport({
    command,
    args: ['mcp'],
    cwd: options.cwd,
    env: backendEnv(options),
    stderr: 'pipe'
  })
  await client.connect(transport, { timeout: options.timeoutMs ?? DEFAULT_TIMEOUT_MS })
  return {
    listTools: (callOptions) => client.listTools(undefined, callOptions),
    callTool: (input, callOptions) => client.callTool(input, undefined, callOptions),
    close: () => client.close()
  }
}

export async function resolveOpenComputerUseCommand(configured?: string): Promise<string | null> {
  const candidates = openComputerUseCommandCandidates(configured)
  for (const candidate of candidates) {
    if (!candidate) continue
    if (!isAbsolute(candidate) && !candidate.includes('/') && !candidate.includes('\\')) {
      return candidate
    }
    try {
      await access(candidate)
      return candidate
    } catch {
      // Try the next candidate.
    }
  }
  return null
}

type OpenComputerUseCommandCandidateOptions = {
  platform?: NodeJS.Platform
  arch?: string
  env?: NodeJS.ProcessEnv
  cwd?: string
  resourcesPath?: string
  execPath?: string
}

function openComputerUseCommandCandidates(
  configured?: string,
  options: OpenComputerUseCommandCandidateOptions = {}
): string[] {
  const env = options.env ?? process.env
  const platform = options.platform ?? process.platform
  const arch = options.arch ?? process.arch
  const cwd = options.cwd ?? process.cwd()
  const execPath = options.execPath ?? process.execPath
  const bin = platform === 'win32' ? 'analytix-computer-use.cmd' : 'analytix-computer-use'
  const legacyBin = platform === 'win32' ? 'open-computer-use.cmd' : 'open-computer-use'
  const resourcesPath = options.resourcesPath ?? processResourcesPath()
  const packagedMode = Boolean(resourcesPath)
  const envAppRoot = resolveAllowedEnvAppRoot(env.ANALYTIX_APP_ROOT, resourcesPath, platform)
  const devPackageRoots = packagedMode
    ? []
    : [
        join(cwd, 'packages', 'runtime', 'node_modules', 'analytix-computer-use'),
        join(cwd, 'node_modules', 'analytix-computer-use'),
        join(cwd, '..', 'node_modules', 'analytix-computer-use'),
        join(dirname(execPath), 'node_modules', 'analytix-computer-use')
      ]
  const packageRoots = [
    ...(envAppRoot ? [
      join(envAppRoot, 'packages', 'runtime', 'node_modules', 'analytix-computer-use'),
      join(envAppRoot, 'node_modules', 'analytix-computer-use')
    ] : []),
    ...(resourcesPath ? [
      join(resourcesPath, 'app.asar.unpacked', 'packages', 'runtime', 'node_modules', 'analytix-computer-use'),
      join(resourcesPath, 'app.asar.unpacked', 'node_modules', 'analytix-computer-use')
    ] : []),
    ...devPackageRoots
  ]
  const nativeCandidates = packageRoots.flatMap((root) =>
    openComputerUseNativeExecutableCandidates(root, platform, arch)
  )
  const configuredShimNativeCandidates = [
    ...nativeCandidatesForNpmShim(configured, platform, arch),
    ...nativeCandidatesForNpmShim(env.ANALYTIX_COMPUTER_USE_PATH, platform, arch),
    ...nativeCandidatesForNpmShim(env.ANALYTIX_OPEN_COMPUTER_USE_PATH, platform, arch)
  ]
  const raw = [
    ...configuredShimNativeCandidates,
    configured,
    env.ANALYTIX_COMPUTER_USE_PATH,
    env.ANALYTIX_OPEN_COMPUTER_USE_PATH,
    ...nativeCandidates,
    ...(envAppRoot ? [
      join(envAppRoot, 'packages', 'runtime', 'node_modules', '.bin', bin),
      join(envAppRoot, 'node_modules', '.bin', bin),
      join(envAppRoot, 'packages', 'runtime', 'node_modules', '.bin', legacyBin),
      join(envAppRoot, 'node_modules', '.bin', legacyBin)
    ] : []),
    ...(resourcesPath ? [
      join(resourcesPath, 'app.asar.unpacked', 'packages', 'runtime', 'node_modules', '.bin', bin),
      join(resourcesPath, 'app.asar.unpacked', 'node_modules', '.bin', bin)
    ] : []),
    ...(packagedMode ? [] : [
      join(cwd, 'node_modules', '.bin', bin),
      join(cwd, '..', 'node_modules', '.bin', bin),
      join(dirname(execPath), 'node_modules', '.bin', bin),
      join(cwd, 'node_modules', '.bin', legacyBin),
      join(cwd, '..', 'node_modules', '.bin', legacyBin),
      join(dirname(execPath), 'node_modules', '.bin', legacyBin)
    ])
  ]
  const fromPath = packagedMode ? [] : (env.PATH ?? '').split(delimiter).map((entry) => join(entry, bin))
  const legacyFromPath = packagedMode ? [] : (env.PATH ?? '').split(delimiter).map((entry) => join(entry, legacyBin))
  const bareFallbacks = packagedMode ? [] : ['analytix-computer-use', 'open-computer-use']
  return [...raw, ...fromPath, ...legacyFromPath, ...bareFallbacks]
    .filter((item): item is string => Boolean(item?.trim()))
    .map((item) => item.trim())
    .filter((item, index, all) => all.indexOf(item) === index)
}

function resolveAllowedEnvAppRoot(
  rawAppRoot: string | undefined,
  resourcesPath: string | undefined,
  platform: NodeJS.Platform
): string | undefined {
  const appRoot = rawAppRoot?.trim()
  if (!appRoot) return undefined
  if (!resourcesPath) return appRoot
  const normalizedAppRoot = normalizePathForPlatform(appRoot, platform)
  const normalizedResourcesPath = normalizePathForPlatform(resourcesPath, platform)
  const pathApi = platform === 'win32' ? win32 : posix
  const pathFromResources = pathApi.relative(normalizedResourcesPath, normalizedAppRoot)
  return pathFromResources === '' || (!pathFromResources.startsWith('..') && !pathApi.isAbsolute(pathFromResources))
    ? appRoot
    : undefined
}

function normalizePathForPlatform(path: string, platform: NodeJS.Platform): string {
  const pathApi = platform === 'win32' ? win32 : posix
  const resolved = pathApi.resolve(path)
  return platform === 'win32' ? resolved.toLowerCase() : resolved
}

function openComputerUseNativeExecutableCandidates(
  packageRoot: string,
  platform: NodeJS.Platform,
  arch: string
): string[] {
  const relativePath = openComputerUseNativeRelativePath(platform, arch)
  return relativePath ? [join(packageRoot, ...relativePath)] : []
}

function openComputerUseNativeRelativePath(platform: NodeJS.Platform, arch: string): string[] | null {
  const normalizedArch = arch === 'arm64' ? 'arm64' : 'amd64'
  if (platform === 'win32') {
    return ['dist', 'windows', normalizedArch, 'analytix-computer-use.exe']
  }
  if (platform === 'darwin') {
    return ['dist', 'Analytix Computer Use.app', 'Contents', 'MacOS', 'OpenComputerUse']
  }
  if (platform === 'linux') {
    return ['dist', 'linux', normalizedArch, 'analytix-computer-use']
  }
  return null
}

function nativeCandidatesForNpmShim(
  command: string | undefined,
  platform: NodeJS.Platform,
  arch: string
): string[] {
  if (!command || !isAbsolute(command)) return []
  const lower = basename(command).toLowerCase()
  if (lower !== 'analytix-computer-use.cmd' && lower !== 'open-computer-use.cmd') return []
  if (basename(dirname(command)) !== '.bin') return []
  return openComputerUseNativeExecutableCandidates(
    join(dirname(dirname(command)), 'analytix-computer-use'),
    platform,
    arch
  )
}

function processResourcesPath(): string {
  const value = (process as NodeJS.Process & { resourcesPath?: unknown }).resourcesPath
  return typeof value === 'string' ? value.trim() : ''
}

export const _internals = {
  openComputerUseCommandCandidates
}

function openComputerUseInstallHint(): string {
  return 'Analytix Computer Use is not available. Bundle analytix-computer-use with analytix, install analytix-computer-use, or set ANALYTIX_COMPUTER_USE_PATH to the CLI path.'
}

function backendEnv(options: OpenComputerUseBackendOptions): Record<string, string> {
  const raw: NodeJS.ProcessEnv = {
    ...process.env,
    ...options.env,
    ...(options.maxImageDimension ? { OPEN_COMPUTER_USE_IMAGE_MAX_DIMENSION: String(options.maxImageDimension) } : {}),
    ...(options.maxImageBytes ? { OPEN_COMPUTER_USE_IMAGE_MAX_BYTES: String(options.maxImageBytes) } : {})
  }
  const env: Record<string, string> = {}
  for (const [key, value] of Object.entries(raw)) {
    if (typeof value === 'string') env[key] = value
  }
  return env
}

function targetArgs(
  target: HostControlTarget,
  extra: Record<string, unknown> = {},
  fallbackApp?: string
): Record<string, unknown> {
  return {
    ...appArg(target.app ?? fallbackApp),
    ...elementArg(target.elementIndex),
    ...coordinateArgs(target),
    ...extra
  }
}

function appArg(app?: string, extra: Record<string, unknown> = {}): Record<string, unknown> {
  const trimmed = app?.trim()
  return { ...(trimmed ? { app: trimmed } : {}), ...extra }
}

function elementArg(elementIndex: number | undefined, key = 'element_index'): Record<string, unknown> {
  return Number.isInteger(elementIndex) ? { [key]: String(elementIndex) } : {}
}

function coordinateArgs(target: HostControlTarget, prefix = ''): Record<string, unknown> {
  if (typeof target.x !== 'number' || typeof target.y !== 'number') return {}
  return {
    [prefix ? `${prefix}_x` : 'x']: target.x,
    [prefix ? `${prefix}_y` : 'y']: target.y
  }
}

function normalizeMcpResult(raw: unknown): NormalizedMcpResult {
  const payload: Record<string, unknown> = {}
  const images: NormalizedMcpResult['images'] = []
  const texts: string[] = []
  const root = isRecord(raw) ? raw : {}
  mergePayload(payload, root.structuredContent)
  mergePayload(payload, root.structured_content)
  const content = Array.isArray(root.content) ? root.content : []
  for (const block of content) {
    if (!isRecord(block)) continue
    if (block.type === 'text' && typeof block.text === 'string') {
      texts.push(block.text)
      mergePayload(payload, parseJson(block.text))
    } else if (block.type === 'image') {
      const data = typeof block.data === 'string' ? block.data : ''
      if (data) {
        images.push({
          mimeType: typeof block.mimeType === 'string'
            ? block.mimeType
            : typeof block.mime_type === 'string'
              ? block.mime_type
              : 'image/png',
          dataBase64: stripDataUrl(data),
          width: numberField(block.width),
          height: numberField(block.height)
        })
      }
    }
  }
  mergePayload(payload, parseJson(texts.join('\n')))
  collectImages(payload, images)
  return {
    raw,
    payload,
    images,
    isError: root.isError === true || root.is_error === true,
    text: texts.join('\n').trim()
  }
}

function screenshotFromResult(result: NormalizedMcpResult): HostScreenshot {
  const image = result.images[0]
  if (!image) throw new Error('Analytix Computer Use result did not include a screenshot image')
  const screen = isRecord(result.payload.screen) ? result.payload.screen : {}
  const dimensions = imageDimensionsFromBase64(image.dataBase64, image.mimeType)
  return {
    mimeType: image.mimeType,
    dataBase64: image.dataBase64,
    width: image.width ?? numberField(screen.width) ?? numberField(result.payload.width) ?? dimensions?.width ?? 1,
    height: image.height ?? numberField(screen.height) ?? numberField(result.payload.height) ?? dimensions?.height ?? 1
  }
}

function imageDimensionsFromBase64(dataBase64: string, mimeType: string): { width: number; height: number } | undefined {
  const data = Buffer.from(stripDataUrl(dataBase64), 'base64')
  if (/png/i.test(mimeType) && data.length >= 24 && data.subarray(0, 8).equals(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]))) {
    const width = data.readUInt32BE(16)
    const height = data.readUInt32BE(20)
    return validDimensions(width, height)
  }
  if (/jpe?g/i.test(mimeType) && data.length >= 4 && data[0] === 0xff && data[1] === 0xd8) {
    let offset = 2
    while (offset + 9 < data.length) {
      if (data[offset] !== 0xff) {
        offset += 1
        continue
      }
      const marker = data[offset + 1]
      const length = data.readUInt16BE(offset + 2)
      if (length < 2) return undefined
      if (
        marker === 0xc0 ||
        marker === 0xc1 ||
        marker === 0xc2 ||
        marker === 0xc3 ||
        marker === 0xc5 ||
        marker === 0xc6 ||
        marker === 0xc7 ||
        marker === 0xc9 ||
        marker === 0xca ||
        marker === 0xcb ||
        marker === 0xcd ||
        marker === 0xce ||
        marker === 0xcf
      ) {
        const height = data.readUInt16BE(offset + 5)
        const width = data.readUInt16BE(offset + 7)
        return validDimensions(width, height)
      }
      offset += 2 + length
    }
  }
  return undefined
}

function validDimensions(width: number, height: number): { width: number; height: number } | undefined {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return undefined
  if (width <= 0 || height <= 0 || width > 100_000 || height > 100_000) return undefined
  return { width, height }
}

function collectImages(value: unknown, out: NormalizedMcpResult['images']): void {
  if (Array.isArray(value)) {
    for (const item of value) collectImages(item, out)
    return
  }
  if (!isRecord(value)) return
  const data = stringValue(value.data_base64) ?? stringValue(value.data) ?? stringValue(value.base64)
  if (data) {
    out.push({
      mimeType: stringValue(value.mime_type) ?? stringValue(value.mimeType) ?? 'image/png',
      dataBase64: stripDataUrl(data),
      width: numberField(value.width),
      height: numberField(value.height)
    })
  }
  for (const child of Object.values(value)) collectImages(child, out)
}

function mergePayload(target: Record<string, unknown>, value: unknown): void {
  if (!isRecord(value)) return
  for (const [key, item] of Object.entries(value)) target[key] = item
}

function parseJson(value: string): unknown {
  const trimmed = value.trim()
  if (!trimmed || !/^[[{]/.test(trimmed)) return null
  try {
    return JSON.parse(trimmed) as unknown
  } catch {
    return null
  }
}

function pruneUndefined(input: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(input).filter(([, value]) => value !== undefined))
}

function stripDataUrl(value: string): string {
  return value.replace(/^data:[^;]+;base64,/i, '')
}

function parseOpenComputerUseAppList(text: string): Array<Record<string, unknown>> {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const match = /^(.*?)\s+—\s+([^\s[]+)(?:\s+\[(.*?)])?$/.exec(line)
      if (!match) return { name: line }
      const flags = (match[3] ?? '')
        .split(',')
        .map((flag) => flag.trim())
        .filter(Boolean)
      return {
        name: match[1].trim(),
        bundleId: match[2].trim(),
        running: flags.includes('running') || flags.includes('frontmost'),
        frontmost: flags.includes('frontmost'),
        flags
      }
    })
}

function parseOpenComputerUseStateText(text: string): {
  app?: Record<string, unknown>
  window?: Record<string, unknown>
  accessibilityTree: Array<Record<string, unknown>>
  elements: Array<Record<string, unknown>>
} {
  const lines = text.split(/\r?\n/)
  const accessibilityTree: Array<Record<string, unknown>> = []
  const elements: Array<Record<string, unknown>> = []
  let app: Record<string, unknown> | undefined
  let window: Record<string, unknown> | undefined
  for (const line of lines) {
    const appMatch = /^App=([^\s]+)(?:\s+\(pid\s+(\d+)\))?/.exec(line.trim())
    if (appMatch) {
      app = {
        bundleId: appMatch[1],
        ...(appMatch[2] ? { pid: Number(appMatch[2]) } : {})
      }
      continue
    }
    const windowMatch = /^Window:\s+"([^"]*)",\s+App:\s+(.+?)\.$/.exec(line.trim())
    if (windowMatch) {
      window = { title: windowMatch[1], appName: windowMatch[2] }
      if (app && typeof app.name !== 'string') app = { ...app, name: windowMatch[2] }
      continue
    }
    const node = parseAccessibilityLine(line)
    if (!node) continue
    accessibilityTree.push(node)
    elements.push(node)
  }
  return { ...(app ? { app } : {}), ...(window ? { window } : {}), accessibilityTree, elements }
}

function parseAccessibilityLine(line: string): Record<string, unknown> | null {
  const match = /^(\s*)(\d+)\s+(.+)$/.exec(line)
  if (!match) return null
  const depth = Math.floor(match[1].replace(/[^\t]/g, '').length + match[1].replace(/\t/g, '').length / 2)
  const elementIndex = Number(match[2])
  const body = match[3].trim()
  const metadata: Record<string, unknown> = {}
  for (const [key, pattern] of Object.entries({
    id: /\bID:\s*([^,]+)(?:,|$)/,
    description: /\bDescription:\s*([^,]+)(?:,|$)/,
    value: /\bValue:\s*([^,]+)(?:,|$)/,
    help: /\bHelp:\s*([^,]+)(?:,|$)/
  })) {
    const value = pattern.exec(body)?.[1]?.trim()
    if (value) metadata[key] = value
  }
  const selected = /\(selected\)/.test(body)
  const disabled = /\(disabled\)/.test(body)
  const settable = /\(settable/.test(body)
  const primary = body
    .replace(/\bID:\s*.*$/, '')
    .replace(/\bDescription:\s*.*$/, '')
    .replace(/\bHelp:\s*.*$/, '')
    .replace(/\bValue:\s*.*$/, '')
    .replace(/\bSecondary Actions:\s*.*$/, '')
    .replace(/\((?:selected|disabled|settable[^)]*)\)/g, '')
    .trim()
  const [role = '', ...nameParts] = primary.split(/\s+/)
  return {
    element_index: elementIndex,
    depth,
    role,
    name: nameParts.join(' ').trim(),
    raw: line.trim(),
    ...(selected ? { selected: true } : {}),
    ...(disabled ? { disabled: true } : {}),
    ...(settable ? { settable: true } : {}),
    ...(Object.keys(metadata).length > 0 ? { metadata } : {})
  }
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

function numberField(value: unknown): number | undefined {
  const number = Number(value)
  return Number.isFinite(number) && number > 0 ? Math.round(number) : undefined
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function summarizeError(error: unknown): string {
  if (error instanceof Error) return error.message
  return String(error)
}
