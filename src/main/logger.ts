import { appendFile, mkdir, readdir, stat, unlink } from 'node:fs/promises'
import { join } from 'node:path'

export type LogLevel = 'error' | 'warn' | 'info'

type LoggerConfig = {
  /** Directory where log files are stored. */
  dir: string
  /** Whether logging is enabled. */
  enabled: boolean
  /** Delete log files older than this many days. */
  retentionDays: number
}

export type ManagedChildLogRecord =
  | { code: 'ANALYTIX_CHILD_SPAWNED'; port: number }
  | { code: 'ANALYTIX_CHILD_EXITED'; exitCode: number | null; signaled: boolean }
  | { code: 'ANALYTIX_CHILD_PROCESS_ERROR' }
  | { code: 'ANALYTIX_CHILD_STARTUP_FAILED' }
  | { code: 'ANALYTIX_CHILD_READY'; port: number }
  | { code: 'ANALYTIX_STALE_CHILD_TERMINATION'; port: number }
  | { code: 'ANALYTIX_CHILD_STDOUT_CAPTURED'; bytes: number; sha256: string }
  | { code: 'ANALYTIX_CHILD_STDERR_CAPTURED'; bytes: number; sha256: string }

type FixedLogCategory = 'child' | 'git' | 'logger' | 'main' | 'renderer'
type FixedLogDetail = Record<string, number | boolean | string>
const RENDERER_LOG_EVENT_CODES = [
  'renderer_approval_failed',
  'renderer_create_thread_failed',
  'renderer_diagnostic_failed',
  'renderer_git_checkpoint_failed',
  'renderer_gui_update_failed',
  'renderer_interrupt_failed',
  'renderer_notification_failed',
  'renderer_send_message_failed',
  'renderer_uncaught_error',
  'renderer_user_input_failed'
] as const
type RendererLogEventCode = typeof RENDERER_LOG_EVENT_CODES[number]
type MainLogEventCode = 'main_error' | 'main_info' | 'main_warn'
type FixedLogEventCode =
  | ManagedChildLogRecord['code']
  | RendererLogEventCode
  | MainLogEventCode
  | 'git_checkpoint'
  | 'managed_log_maintenance'
type FixedLogProjection = {
  category: FixedLogCategory
  eventCode: FixedLogEventCode
  detail?: FixedLogDetail
}

let cfg: LoggerConfig = { dir: '', enabled: true, retentionDays: 2 }
const SHA256_PATTERN = /^[a-f0-9]{64}$/u
const RENDERER_LOG_EVENT_CODE_SET: ReadonlySet<string> = new Set(RENDERER_LOG_EVENT_CODES)
const MAIN_LOG_EVENT_CODE_BY_LEVEL: Readonly<Record<LogLevel, MainLogEventCode>> = {
  error: 'main_error',
  info: 'main_info',
  warn: 'main_warn'
}
const SAFE_NUMBER_DETAIL_KEYS = new Set([
  'attempt',
  'attempts',
  'bytes',
  'count',
  'durationMs',
  'elapsedMs',
  'exitCode',
  'port',
  'previousPort',
  'responseBytes',
  'resultCount',
  'retentionDays',
  'schemaVersion',
  'statusCode',
  'stderrBytes',
  'stdoutBytes',
  'untrackedBytes',
  'untrackedCount'
])
const SAFE_BOOLEAN_DETAIL_KEYS = new Set([
  'autoStart',
  'hasApiKey',
  'isFatal',
  'ok',
  'retryable',
  'rolledBack',
  'signaled'
])
const SAFE_SHA256_DETAIL_KEYS = new Set([
  'contentSha256',
  'sha256',
  'stderrSha256',
  'stdoutSha256'
])

export function configureLogger(config: Partial<LoggerConfig>): void {
  cfg = { ...cfg, ...config }
}

function logFileName(timestamp: Date): string {
  const pad = (n: number): string => String(n).padStart(2, '0')
  return `analytix-${timestamp.getFullYear()}-${pad(timestamp.getMonth() + 1)}-${pad(timestamp.getDate())}.log`
}

function isManagedLogFile(entry: string): boolean {
  return entry.startsWith('analytix-') && entry.endsWith('.log')
}

/** Best-effort prune without a separate sweeper. */
async function pruneOldLogs(): Promise<void> {
  try {
    const entries = await readdir(cfg.dir)
    const cutoff = Date.now() - cfg.retentionDays * 24 * 60 * 60 * 1000
    for (const entry of entries) {
      if (!isManagedLogFile(entry)) continue
      try {
        const info = await stat(join(cfg.dir, entry))
        if (info.mtimeMs < cutoff) await unlink(join(cfg.dir, entry))
      } catch {
        /* skip unreadable files */
      }
    }
  } catch {
    /* directory may not exist yet */
  }
}

function projectFixedDetail(value: unknown): FixedLogDetail | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const projected: FixedLogDetail = {}
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    if (SAFE_NUMBER_DETAIL_KEYS.has(key)) {
      if (typeof entry === 'number' && Number.isFinite(entry) && entry >= 0) projected[key] = entry
      continue
    }
    if (SAFE_BOOLEAN_DETAIL_KEYS.has(key)) {
      if (typeof entry === 'boolean') projected[key] = entry
      continue
    }
    if (SAFE_SHA256_DETAIL_KEYS.has(key) && typeof entry === 'string' && SHA256_PATTERN.test(entry)) {
      projected[key] = entry
    }
  }
  return Object.keys(projected).length > 0 ? projected : undefined
}

function projectMainLog(
  level: LogLevel,
  category: string,
  detail: unknown
): FixedLogProjection {
  const fallback = projectMainLogFallback(level, category)
  try {
    const fixedDetail = projectFixedDetail(detail)
    if (category === 'renderer-ipc') {
      const candidate = detail && typeof detail === 'object' && !Array.isArray(detail)
        ? (detail as Record<string, unknown>).code
        : undefined
      return {
        ...fallback,
        eventCode: isRendererLogEventCode(candidate)
          ? candidate
          : 'renderer_diagnostic_failed',
        ...(fixedDetail ? { detail: fixedDetail } : {})
      }
    }
    return { ...fallback, ...(fixedDetail ? { detail: fixedDetail } : {}) }
  } catch {
    return fallback
  }
}

function projectMainLogFallback(level: LogLevel, category: string): FixedLogProjection {
  if (category === 'renderer-ipc') {
    return {
      category: 'renderer',
      eventCode: 'renderer_diagnostic_failed'
    }
  }
  if (category === 'git-checkpoint') {
    return { category: 'git', eventCode: 'git_checkpoint' }
  }
  if (category === 'logger') {
    return { category: 'logger', eventCode: 'managed_log_maintenance' }
  }
  return { category: 'main', eventCode: MAIN_LOG_EVENT_CODE_BY_LEVEL[level] }
}

function isRendererLogEventCode(value: unknown): value is RendererLogEventCode {
  return typeof value === 'string' && RENDERER_LOG_EVENT_CODE_SET.has(value)
}

async function appendFixedLogLine(level: LogLevel, projection: FixedLogProjection): Promise<void> {
  if (!cfg.enabled || !cfg.dir) return
  const stamp = new Date().toISOString()
  const detail = projection.detail ? ` detail=${JSON.stringify(projection.detail)}` : ''
  const line = `[${stamp}] [${level.toUpperCase()}] [${projection.category}] event=${projection.eventCode}${detail}\n`
  try {
    await mkdir(cfg.dir, { recursive: true })
    await appendFile(join(cfg.dir, logFileName(new Date())), line, 'utf8')
    await pruneOldLogs()
  } catch {
    /* never crash the app because of logging */
  }
}

export function logError(category: string, _message: string, detail?: unknown): void {
  void appendFixedLogLine('error', projectMainLog('error', category, detail))
}

export function logWarn(category: string, _message: string, detail?: unknown): void {
  void appendFixedLogLine('warn', projectMainLog('warn', category, detail))
}

export function logInfo(category: string, _message: string, detail?: unknown): void {
  void appendFixedLogLine('info', projectMainLog('info', category, detail))
}

export function normalizeManagedChildLogRecord(record: unknown): ManagedChildLogRecord | null {
  if (!record || typeof record !== 'object' || Array.isArray(record)) return null
  const candidate = record as Record<string, unknown>
  switch (candidate.code) {
    case 'ANALYTIX_CHILD_SPAWNED':
    case 'ANALYTIX_CHILD_READY':
    case 'ANALYTIX_STALE_CHILD_TERMINATION':
      if (!Number.isSafeInteger(candidate.port) || (candidate.port as number) <= 0 || (candidate.port as number) > 65_535) return null
      return { code: candidate.code, port: candidate.port as number }
    case 'ANALYTIX_CHILD_EXITED': {
      if (typeof candidate.signaled !== 'boolean') return null
      if (candidate.exitCode !== null &&
          (!Number.isSafeInteger(candidate.exitCode) || (candidate.exitCode as number) < 0)) return null
      return { code: candidate.code, exitCode: candidate.exitCode as number | null, signaled: candidate.signaled }
    }
    case 'ANALYTIX_CHILD_PROCESS_ERROR':
    case 'ANALYTIX_CHILD_STARTUP_FAILED':
      return { code: candidate.code }
    case 'ANALYTIX_CHILD_STDOUT_CAPTURED':
    case 'ANALYTIX_CHILD_STDERR_CAPTURED':
      if (!Number.isSafeInteger(candidate.bytes) || (candidate.bytes as number) < 0 ||
          typeof candidate.sha256 !== 'string' || !SHA256_PATTERN.test(candidate.sha256)) return null
      return { code: candidate.code, bytes: candidate.bytes as number, sha256: candidate.sha256 }
    default:
      return null
  }
}

function projectManagedChildLogRecord(record: unknown): FixedLogProjection | null {
  const normalized = normalizeManagedChildLogRecord(record)
  if (!normalized) return null
  switch (normalized.code) {
    case 'ANALYTIX_CHILD_SPAWNED':
    case 'ANALYTIX_CHILD_READY':
    case 'ANALYTIX_STALE_CHILD_TERMINATION':
      return { category: 'child', eventCode: normalized.code, detail: { port: normalized.port } }
    case 'ANALYTIX_CHILD_EXITED':
      return {
        category: 'child',
        eventCode: normalized.code,
        detail: {
          ...(normalized.exitCode === null ? {} : { exitCode: normalized.exitCode }),
          signaled: normalized.signaled
        }
      }
    case 'ANALYTIX_CHILD_PROCESS_ERROR':
    case 'ANALYTIX_CHILD_STARTUP_FAILED':
      return { category: 'child', eventCode: normalized.code }
    case 'ANALYTIX_CHILD_STDOUT_CAPTURED':
    case 'ANALYTIX_CHILD_STDERR_CAPTURED':
      return {
        category: 'child',
        eventCode: normalized.code,
        detail: { bytes: normalized.bytes, sha256: normalized.sha256 }
      }
  }
}

export async function appendManagedChildLogRecord(record: ManagedChildLogRecord): Promise<void> {
  const projection = projectManagedChildLogRecord(record)
  if (!projection) return
  await appendFixedLogLine('info', projection)
}

function writePublicConsole(
  level: LogLevel,
  category: string,
  detail?: unknown
): void {
  const projection = projectMainLog(level, category, detail)
  const projectedDetail = projection.detail ? ` detail=${JSON.stringify(projection.detail)}` : ''
  console[level](`[analytix] [${projection.category}] event=${projection.eventCode}${projectedDetail}`)
}

export function publicConsoleError(category: string, _message: string, detail?: unknown): void {
  writePublicConsole('error', category, detail)
}

export function publicConsoleWarn(category: string, _message: string, detail?: unknown): void {
  writePublicConsole('warn', category, detail)
}

export function publicConsoleInfo(category: string, _message: string, detail?: unknown): void {
  writePublicConsole('info', category, detail)
}

/** On startup, prune old logs immediately and record only closed metadata. */
export async function pruneOnStartup(): Promise<void> {
  await pruneOldLogs()
  logInfo('logger', 'managed log retention applied', { retentionDays: cfg.retentionDays })
}
