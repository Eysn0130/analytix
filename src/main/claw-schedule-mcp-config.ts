import { mkdir, readFile } from 'node:fs/promises'
import { homedir } from 'node:os'
import { basename, dirname, isAbsolute, join, normalize, posix, sep } from 'node:path'
import type { AppSettingsV1 } from '../shared/app-settings'
import {
  atomicWriteFile,
  ensureFileMode
} from '../../packages/runtime/src/adapters/file/atomic-write.js'

const CLAW_SCHEDULE_MCP_MARKER_START = '# analytix plugin:mcp:claw-schedule START'
const CLAW_SCHEDULE_MCP_MARKER_END = '# analytix plugin:mcp:claw-schedule END'
export const GUI_SCHEDULE_MCP_SERVER_NAME = 'gui_schedule'
export const GUI_SCHEDULE_MCP_TIMEOUT_MS = 5_000
export const HOST_SCHEDULE_MCP_BINDING_PURPOSE_V1 = 'analytix.runtime-host-schedule-mcp-binding/v1'
export const RUNTIME_STARTUP_PRIVATE_FRAME_PURPOSE_V1 = 'analytix.runtime-startup-private-frame/v1'
export const MAX_RUNTIME_STARTUP_PRIVATE_FRAME_BYTES_V1 = 512 << 10
const LEGACY_CLAW_SCHEDULE_MCP_SERVER_NAME = 'claw_schedule'
const GUI_SCHEDULE_MCP_NODE_ENTRY = 'out/main/claw-schedule-mcp-node-entry.js'
const ELECTRON_RUN_AS_NODE_ENV = Object.freeze({ ELECTRON_RUN_AS_NODE: '1' } as const)

type JsonRecord = Record<string, unknown>

export type ClawScheduleMcpLaunchConfig = {
  appPath: string
  execPath: string
  isPackaged: boolean
}

export type RuntimeHostScheduleMcpBindingV1 = Readonly<{
  schemaVersion: 1
  purpose: typeof HOST_SCHEDULE_MCP_BINDING_PURPOSE_V1
  serverId: typeof GUI_SCHEDULE_MCP_SERVER_NAME
  command: string
  args: readonly string[]
  env: Readonly<{ ELECTRON_RUN_AS_NODE: '1' }>
  trustScope: 'user'
  timeoutMs: typeof GUI_SCHEDULE_MCP_TIMEOUT_MS
}>

type ClawScheduleMcpConfigPaths = {
  configTomlPath?: string
  mcpJsonPath?: string
}

export function resolveAnalytixConfigPath(homeRoot: string = homedir()): string {
  return join(homeRoot, '.analytix', 'config.toml')
}

export function resolveAnalytixMcpJsonPath(homeRoot: string = homedir()): string {
  return join(homeRoot, '.analytix', 'mcp.json')
}

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isErrnoException(error: unknown): error is NodeJS.ErrnoException {
  return typeof error === 'object' && error !== null
}

export function buildClawScheduleMcpArgs(
  settings: AppSettingsV1,
  launch: ClawScheduleMcpLaunchConfig
): string[] {
  const args: string[] = [
    resolveClawScheduleMcpNodeEntryPath(launch),
    '--gui-schedule-mcp-server',
    '--base-url',
    `http://127.0.0.1:${settings.schedule.internal.port}`
  ]
  const secret = settings.schedule.internal.secret.trim()
  if (secret) {
    args.push('--secret', secret)
  }
  return args
}

export function resolveClawScheduleMcpNodeEntryPath(launch: ClawScheduleMcpLaunchConfig): string {
  if (launch.appPath.includes('/') && !launch.appPath.includes('\\')) {
    return posix.join(launch.appPath, GUI_SCHEDULE_MCP_NODE_ENTRY)
  }
  return join(launch.appPath, GUI_SCHEDULE_MCP_NODE_ENTRY)
}

export function resolveClawScheduleMcpCommand(
  launch: ClawScheduleMcpLaunchConfig,
  platform: NodeJS.Platform = process.platform
): string {
  if (platform !== 'darwin') return launch.execPath
  if (!launch.execPath.includes('/Contents/MacOS/')) return launch.execPath

  const appContentsDir = posix.dirname(posix.dirname(launch.execPath))
  const appName = posix.basename(launch.execPath)
  const helperName = `${appName} Helper`
  return posix.join(
    appContentsDir,
    'Frameworks',
    `${helperName}.app`,
    'Contents',
    'MacOS',
    helperName
  )
}

export function buildRuntimeHostScheduleMcpBindingV1(
  settings: AppSettingsV1,
  launch: ClawScheduleMcpLaunchConfig
): RuntimeHostScheduleMcpBindingV1 {
  return validateRuntimeHostScheduleMcpBindingV1({
    schemaVersion: 1,
    purpose: HOST_SCHEDULE_MCP_BINDING_PURPOSE_V1,
    serverId: GUI_SCHEDULE_MCP_SERVER_NAME,
    command: resolveClawScheduleMcpCommand(launch),
    args: buildClawScheduleMcpArgs(settings, launch),
    env: ELECTRON_RUN_AS_NODE_ENV,
    trustScope: 'user',
    timeoutMs: GUI_SCHEDULE_MCP_TIMEOUT_MS
  })
}

export function validateRuntimeHostScheduleMcpBindingV1(
  input: unknown
): RuntimeHostScheduleMcpBindingV1 {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    throw new Error('Host schedule MCP binding is invalid.')
  }
  const candidate = input as Record<string, unknown>
  const keys = Object.keys(candidate).sort()
  const expectedKeys = [
    'args', 'command', 'env', 'purpose', 'schemaVersion', 'serverId',
    'timeoutMs', 'trustScope'
  ]
  const args = Array.isArray(candidate.args) && candidate.args.every(
    (value) => typeof value === 'string'
  )
    ? [...candidate.args] as string[]
    : []
  const env = candidate.env && typeof candidate.env === 'object' &&
    !Array.isArray(candidate.env)
    ? candidate.env as Record<string, unknown>
    : null
  const command = typeof candidate.command === 'string' ? candidate.command : ''
  const entrypoint = args[0] ?? ''
  let canonicalPort = 0
  try {
    const parsed = new URL(args[3] ?? '')
    canonicalPort = Number(parsed.port)
    if (
      !Number.isSafeInteger(canonicalPort) || canonicalPort < 1 ||
      canonicalPort > 65_535 || args[3] !== `http://127.0.0.1:${canonicalPort}` ||
      parsed.username !== '' || parsed.password !== '' || parsed.pathname !== '/' ||
      parsed.search !== '' || parsed.hash !== ''
    ) {
      canonicalPort = 0
    }
  } catch {
    canonicalPort = 0
  }
  if (
    keys.length !== expectedKeys.length ||
    keys.some((key, index) => key !== expectedKeys[index]) ||
    candidate.schemaVersion !== 1 ||
    candidate.purpose !== HOST_SCHEDULE_MCP_BINDING_PURPOSE_V1 ||
    candidate.serverId !== GUI_SCHEDULE_MCP_SERVER_NAME ||
    candidate.trustScope !== 'user' ||
    candidate.timeoutMs !== GUI_SCHEDULE_MCP_TIMEOUT_MS ||
    !command || command !== command.trim() ||
    !isAbsolute(command) || normalize(command) !== command || command.endsWith(sep) ||
    !entrypoint || entrypoint !== entrypoint.trim() ||
    !isAbsolute(entrypoint) || normalize(entrypoint) !== entrypoint || entrypoint.endsWith(sep) ||
    (args.length !== 4 && args.length !== 6) ||
    args[1] !== '--gui-schedule-mcp-server' ||
    args[2] !== '--base-url' || canonicalPort === 0 ||
    (args.length === 6 && (
      args[4] !== '--secret' || !args[5] || args[5] !== args[5].trim()
    )) ||
    !env || Object.keys(env).length !== 1 || env.ELECTRON_RUN_AS_NODE !== '1' ||
    [command, ...args].some(
      (value) => value.includes('\0') || !hasWellFormedUTF16V1(value)
    )
  ) {
    throw new Error('Host schedule MCP binding is invalid.')
  }
  const normalized = Object.freeze({
    schemaVersion: 1,
    purpose: HOST_SCHEDULE_MCP_BINDING_PURPOSE_V1,
    serverId: GUI_SCHEDULE_MCP_SERVER_NAME,
    command,
    args: Object.freeze(args),
    env: ELECTRON_RUN_AS_NODE_ENV,
    trustScope: 'user',
    timeoutMs: GUI_SCHEDULE_MCP_TIMEOUT_MS
  })
  try {
    const scheduleOnlyBody = goCompatibleJSONStringifyV1({
      schemaVersion: 1,
      purpose: RUNTIME_STARTUP_PRIVATE_FRAME_PURPOSE_V1,
      hostScheduleMcpBindingV1: normalized
    })
    const scheduleOnlyBytes = Buffer.byteLength(scheduleOnlyBody, 'utf8')
    if (
      scheduleOnlyBytes === 0 ||
      scheduleOnlyBytes > MAX_RUNTIME_STARTUP_PRIVATE_FRAME_BYTES_V1
    ) {
      throw new Error('Host schedule MCP binding is invalid.')
    }
  } catch {
    throw new Error('Host schedule MCP binding is invalid.')
  }
  return normalized
}

function hasWellFormedUTF16V1(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const codeUnit = value.charCodeAt(index)
    if (codeUnit >= 0xd800 && codeUnit <= 0xdbff) {
      if (index + 1 >= value.length) return false
      const next = value.charCodeAt(index + 1)
      if (next < 0xdc00 || next > 0xdfff) return false
      index += 1
    } else if (codeUnit >= 0xdc00 && codeUnit <= 0xdfff) {
      return false
    }
  }
  return true
}

export function goCompatibleJSONStringifyV1(value: unknown): string {
  const encoded = JSON.stringify(value, (_key, item: unknown) => {
    if (typeof item === 'string' && !hasWellFormedUTF16V1(item)) {
      throw new Error('Host schedule MCP binding is invalid.')
    }
    return item
  })
  if (typeof encoded !== 'string') {
    throw new Error('Host schedule MCP binding is invalid.')
  }
  return encoded
    .replace(/</gu, '\\u003c')
    .replace(/>/gu, '\\u003e')
    .replace(/&/gu, '\\u0026')
    .replace(/\u2028/gu, '\\u2028')
    .replace(/\u2029/gu, '\\u2029')
}

export function buildClawScheduleMcpServerConfig(
  settings: AppSettingsV1,
  launch: ClawScheduleMcpLaunchConfig
): JsonRecord {
  return {
    command: resolveClawScheduleMcpCommand(launch),
    args: buildClawScheduleMcpArgs(settings, launch),
    env: ELECTRON_RUN_AS_NODE_ENV,
    url: null,
    connect_timeout: null,
    execute_timeout: null,
    read_timeout: null,
    disabled: false,
    enabled: true,
    required: false,
    enabled_tools: [],
    disabled_tools: []
  }
}

export function buildSyncedClawScheduleMcpJson(
  existing: unknown,
  settings: AppSettingsV1,
  launch: ClawScheduleMcpLaunchConfig
): JsonRecord {
  const base = isRecord(existing) ? existing : {}
  const servers = isRecord(base.servers) ? base.servers : {}
  const { [LEGACY_CLAW_SCHEDULE_MCP_SERVER_NAME]: _legacyScheduleServer, ...userServers } = servers
  const timeouts = isRecord(base.timeouts)
    ? base.timeouts
    : {
        connect_timeout: 10,
        execute_timeout: 60,
        read_timeout: 120
      }

  return {
    ...base,
    timeouts,
    servers: {
      ...userServers,
      [GUI_SCHEDULE_MCP_SERVER_NAME]: buildClawScheduleMcpServerConfig(settings, launch)
    }
  }
}

function removeMarkedTomlBlock(content: string, markerStart: string, markerEnd: string): string {
  const startIndex = content.indexOf(markerStart)
  const endIndex = content.indexOf(markerEnd)
  if (startIndex >= 0 && endIndex > startIndex) {
    const before = content.slice(0, startIndex).trimEnd()
    const after = content.slice(endIndex + markerEnd.length).trimStart()
    return `${before}${before && after ? '\n\n' : ''}${after}`.trim()
  }
  return content.trim()
}

function stripTomlTable(content: string, tableHeader: string): string {
  const lines = content.split('\n')
  const out: string[] = []
  let skipping = false
  for (const line of lines) {
    const trimmed = line.trim()
    if (!skipping && trimmed === tableHeader) {
      skipping = true
      continue
    }
    if (skipping) {
      if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
        skipping = false
        out.push(line)
      }
      continue
    }
    out.push(line)
  }
  return out.join('\n').trim()
}

export function removeLegacyClawScheduleTomlConfig(content: string): string {
  const hasLegacyConfig =
    content.includes(CLAW_SCHEDULE_MCP_MARKER_START) ||
    content.split('\n').some((line) => line.trim() === '[mcp_servers.claw_schedule]')
  if (!hasLegacyConfig) return content

  const withoutMarked = removeMarkedTomlBlock(
    content,
    CLAW_SCHEDULE_MCP_MARKER_START,
    CLAW_SCHEDULE_MCP_MARKER_END
  )
  const withoutLegacyTable = stripTomlTable(withoutMarked, '[mcp_servers.claw_schedule]')
  return withoutLegacyTable ? `${withoutLegacyTable}\n` : ''
}

async function readJsonFile(path: string): Promise<unknown | null> {
  let raw = ''
  try {
    raw = await readFile(path, 'utf8')
  } catch (error) {
    if (isErrnoException(error) && error.code === 'ENOENT') return null
    throw error
  }

  try {
    return JSON.parse(raw) as unknown
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`Failed to parse Analytix MCP config at ${path}: ${message}`, { cause: error })
  }
}

async function cleanupLegacyTomlConfig(path: string): Promise<void> {
  let current = ''
  try {
    current = await readFile(path, 'utf8')
  } catch (error) {
    if (isErrnoException(error) && error.code === 'ENOENT') return
    throw error
  }

  const next = removeLegacyClawScheduleTomlConfig(current)
  if (next === current) {
    await ensureFileMode(path, 0o600)
    return
  }
  await atomicWriteFile(path, next, { mode: 0o600 })
}

export function clawScheduleMcpSettingsChanged(prev: AppSettingsV1, next: AppSettingsV1): boolean {
  return (
    prev.schedule.internal.port !== next.schedule.internal.port ||
    prev.schedule.internal.secret.trim() !== next.schedule.internal.secret.trim()
  )
}

export async function syncClawScheduleMcpConfig(
  settings: AppSettingsV1,
  launch: ClawScheduleMcpLaunchConfig,
  paths: ClawScheduleMcpConfigPaths = {}
): Promise<void> {
  const configTomlPath = paths.configTomlPath ?? resolveAnalytixConfigPath()
  const mcpJsonPath = paths.mcpJsonPath ?? resolveAnalytixMcpJsonPath()

  await cleanupLegacyTomlConfig(configTomlPath)

  const current = await readJsonFile(mcpJsonPath)
  const next = buildSyncedClawScheduleMcpJson(current, settings, launch)
  const nextText = `${JSON.stringify(next, null, 2)}\n`
  const currentText = current === null ? '' : `${JSON.stringify(current, null, 2)}\n`
  if (nextText === currentText) {
    await ensureFileMode(mcpJsonPath, 0o600)
    return
  }

  await mkdir(dirname(mcpJsonPath), { recursive: true })
  await atomicWriteFile(mcpJsonPath, nextText, { mode: 0o600 })
}
