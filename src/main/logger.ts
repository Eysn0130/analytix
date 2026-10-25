import { appendFile, mkdir, readdir, stat, unlink } from 'node:fs/promises'
import { join } from 'node:path'
import { createRuntimeStatusPublicV1, type RuntimeStatusPublicV1Input } from '../shared/analytix-runtime-status'
import { isPublicRuntimeSseRejectionReasonCode } from '../shared/public-runtime-sse'
import { startupOwnerPhasesV1 } from './runtime/startup-owner-trace-v1'
import { normalizeNativeManualBeginPhaseV1, type NativeManualBeginPhaseV1 } from './runtime/native-manual-begin-phase-v1'
import type { PluginPackageHostResponse } from '../../packages/runtime/src/contracts/plugin-package-host'
import type { ObjectEditingResponse } from '../../packages/runtime/src/contracts/object-editing'

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
type FixedLogDetail = Record<string, number | boolean | string | null>
export const GO_RUNTIME_STARTUP_FAILURE_CLASSES = [
  'private_frame_failed',
  'stdout_unavailable',
  'ready_output_limit',
  'ready_line_invalid',
  'ready_payload_invalid',
  'ready_parse_failed',
  'ready_child_exited',
  'ready_child_error',
  'ready_timeout',
  'ready_wait_failed',
  'ready_identity_failed',
  'post_ready_failed'
] as const
export type GoRuntimeStartupFailureClass = typeof GO_RUNTIME_STARTUP_FAILURE_CLASSES[number]
export const NATIVE_OFFICE_DIAGNOSTIC_STAGES = [
  'ipc_admission', 'ipc_resolve_present', 'ipc_controller_open',
  'controller_validation', 'controller_package', 'controller_open_object', 'controller_content',
  'controller_surface_create', 'controller_surface_attach', 'controller_surface_open', 'controller_recovery', 'controller_manual_begin', 'controller_save_failure',
  'controller_surface_open_guard_before', 'controller_surface_open_guard_after',
  'controller_surface_open_guard_final', 'controller_surface_open_current_final',
  'surface_admission', 'surface_preload', 'surface_assets', 'surface_asset_server',
  'surface_view', 'surface_load', 'surface_transfer_guard', 'surface_bind', 'surface_ready',
  'surface_open', 'surface_request'
] as const
export type NativeOfficeDiagnosticStage = typeof NATIVE_OFFICE_DIAGNOSTIC_STAGES[number]
const NATIVE_OFFICE_OPEN_FAILURE_BOOLEAN_KEYS = [
  'openDispatched', 'openResolved', 'guardPassed', 'owns', 'visible', 'epochMatch', 'snapshotCurrent', 'ownerAborted'
] as const
export type NativeOfficeOpenFailureState = Record<typeof NATIVE_OFFICE_OPEN_FAILURE_BOOLEAN_KEYS[number], boolean>
const NATIVE_OFFICE_MANUAL_BEGIN_FAILURE_KEYS = [
  'schemaAccepted', 'outerCode', 'innerCode', 'pendingSave', 'pendingRecovery', 'pendingManualCancel'
] as const
export type NativeOfficeManualBeginFailureState = {
  /** Last completed response schema parse in this begin attempt; null before any parse. */
  schemaAccepted: boolean | null
  outerCode: Extract<PluginPackageHostResponse, { ok: false }>['code'] | null
  innerCode: Extract<ObjectEditingResponse, { ok: false }>['code'] | null
  pendingSave: boolean
  pendingRecovery: boolean
  pendingManualCancel: boolean
}
const NATIVE_OFFICE_SAVE_PHASES = ['prepare', 'replace', 'export', 'commit', 'ack', 'query', 'recovery'] as const
const NATIVE_OFFICE_SAVE_RECEIPT_STATUSES = ['committed', 'conflict', 'unknown', 'pending'] as const
/** Closed failure facts only. Schema/receipt facts describe the last Core
 * response checked in this attempt; native operations do not invent one. */
export type NativeOfficeSaveFailureState = Pick<NativeOfficeManualBeginFailureState, 'outerCode' | 'innerCode'> & {
  phase: typeof NATIVE_OFFICE_SAVE_PHASES[number]
  outerSchemaAccepted: boolean | null
  innerSchemaAccepted: boolean | null
  receiptPresent: boolean | null
  receiptStatus: typeof NATIVE_OFFICE_SAVE_RECEIPT_STATUSES[number] | null
}
const NATIVE_OFFICE_SAVE_FAILURE_KEYS = ['phase', 'outerSchemaAccepted', 'innerSchemaAccepted', 'outerCode', 'innerCode', 'receiptPresent', 'receiptStatus'] as const
const NATIVE_OFFICE_MANUAL_BEGIN_OUTER_CODES = {
  unavailable: true, invalid_request: true, identity_invalid: true, package_not_found: true,
  conflict: true, disabled: true, adapter_unavailable: true, persistence_failure: true
} as const satisfies Record<NonNullable<NativeOfficeManualBeginFailureState['outerCode']>, true>
const NATIVE_OFFICE_MANUAL_BEGIN_INNER_CODES = {
  unavailable: true, unsupported_platform: true, invalid_request: true, session_invalid: true,
  capacity: true, forbidden: true, not_text: true, too_large: true, conflict: true,
  operation_mismatch: true, operation_not_found: true, persistence_failure: true,
  draft_stale: true, scope_invalid: true, proposal_invalid: true,
  projection_unavailable: true, protected_span_invalid: true
} as const satisfies Record<NonNullable<NativeOfficeManualBeginFailureState['innerCode']>, true>
const NATIVE_OFFICE_DIAGNOSTIC_STAGE_SET: ReadonlySet<string> = new Set(NATIVE_OFFICE_DIAGNOSTIC_STAGES)
const USER_INPUT_LOCAL_READ_DIAGNOSTIC_STAGES = [
  'ipc_authority', 'request_admission', 'owner_admission', 'view_capacity', 'runtime_request',
  'response_owner', 'response_http', 'response_limit', 'response_json', 'response_schema', 'response_selector', 'display_owner', 'display_digest'
] as const
export type UserInputLocalReadDiagnosticStage = typeof USER_INPUT_LOCAL_READ_DIAGNOSTIC_STAGES[number]
const USER_INPUT_LOCAL_READ_DIAGNOSTIC_STAGE_SET: ReadonlySet<string> = new Set(USER_INPUT_LOCAL_READ_DIAGNOSTIC_STAGES)
const NATIVE_OFFICE_DIAGNOSTIC_ERROR_CODES = [
  'invalid_request', 'unavailable', 'forbidden', 'package_unavailable', 'package_changed', 'invalid_response',
  'engine_unavailable', 'unsaved_changes', 'conflict', 'unknown', 'save_failed', 'stale_selection',
  'unsupported_selection', 'capacity', 'office-startup-failed', 'office-assets-unavailable',
  'office-protocol-invalid', 'office-operation-timeout-unknown', 'office-engine-closed-unknown',
  'engine-load-failed', 'engine-operation-failed', 'unexpected'
] as const
export type NativeOfficeDiagnosticErrorCode = typeof NATIVE_OFFICE_DIAGNOSTIC_ERROR_CODES[number]
const NATIVE_OFFICE_DIAGNOSTIC_ERROR_CODE_SET: ReadonlySet<string> = new Set(NATIVE_OFFICE_DIAGNOSTIC_ERROR_CODES)
const GO_RUNTIME_STARTUP_FAILURE_CLASS_SET: ReadonlySet<string> = new Set(GO_RUNTIME_STARTUP_FAILURE_CLASSES)
const GO_RUNTIME_STARTUP_SIGNAL_SET: ReadonlySet<string> = new Set([
  'SIGABRT', 'SIGALRM', 'SIGBUS', 'SIGFPE', 'SIGHUP', 'SIGILL', 'SIGINT',
  'SIGKILL', 'SIGPIPE', 'SIGQUIT', 'SIGSEGV', 'SIGTERM', 'SIGTRAP', 'SIGUSR1', 'SIGUSR2'
])
const RENDERER_LOG_EVENT_CODES = [
  'renderer_approval_failed',
  'renderer_create_thread_failed',
  'renderer_diagnostic_failed',
  'renderer_sse_event_active',
  'renderer_sse_event_aborted',
  'renderer_sse_event_closed',
  'renderer_sse_ack_false_active',
  'renderer_sse_ack_false_aborted',
  'renderer_sse_ack_false_closed',
  'renderer_sse_ack_transport_active',
  'renderer_sse_ack_transport_aborted',
  'renderer_sse_ack_transport_closed',

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
const SSE_ACK_DIAGNOSTIC_REASONS: ReadonlySet<string> = new Set([
  'invalid_ack_request',
  'stream_unavailable',
  'owner_mismatch',
  'undelivered_sequence',
  'accepted_final_receipt_mismatch',
  'accepted_final_sequence_invalid',
  'unexpected_accepted_final_receipt',
])
const SAFE_NUMBER_DETAIL_KEYS = new Set([
  'attempt',
  'attempts',
  'bytes',
  'count',
  'durationMs',
  'elapsedMs',
  'maxConcurrentWaitForHealthProbes',
  'maxEventLoopLagMs',
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
export const STARTUP_TRACE_STAGES = [
  'main module evaluated',
  'legacy data migration barrier ready',
  'app icon loaded',
  'single instance lock checked',
  'createWindow:start',
  'window:startup-surface-ready',
  'createWindow:load',
  'window:ready-to-show',
  'window:did-finish-load',
  'window:did-fail-load',
  'window:fallback-show-timeout',
  'app.whenReady:start',
  'install webview guards:start',
  'install webview guards:done',
  'settings load:start',
  'settings load:done',
  'desktop private history migration:start',
  'desktop private history migration:done',
  'logger configured',
  'native host registration:done',
  'extension account reconciliation:scheduled',
  'extension account reconciliation:done',
  'legacy IM credential migration:done',
  'IM lifecycle recovery:done',
  'OAuth callback router:done',
  'OAuth authorization sweep:done',
  'OAuth refresh scheduler:done',
  'Claw and IM runtime composition:done',
  'ipc registration:start',
  'ipc registration:done',
  'createWindow:returned',
  'runtime ensure:begin',
  'runtime initial health:done',
  'runtime launch settings:done',
  'runtime adapter:begin',
  'runtime adapter:done',
  'runtime adapter:failed',
  'runtime health:done',
  'runtime thread probe:done',
  'runtime ensure:done',
  'runtime settings apply:queued',
  'runtime settings apply:begin',
  'runtime settings apply:restart begin',
  'runtime settings apply:restart done',
  'runtime settings apply:done',
  'runtime settings apply:failed',
  'runtime IPC restart:requested',
  'runtime IPC restart:done',
  'runtime restart:begin',
  'runtime restart:after settings apply',
  'runtime restart:done',
  'window close:requested',
  'window all closed',
  'app before quit:begin',
  'app before quit:prepared',
  'app before quit:closing',
  'app before quit:close allowed',
  'app before quit:close veto prior',
  'app before quit:close veto unprepared',
  'app before quit:close veto frame',
  'app before quit:close veto loading',
  'app before quit:close veto late',
  'app before quit:window closed',
  'app before quit:close unknown',
  'app before quit:windows closed',
  'app before quit:office teardown settled',
  'app before quit:office teardown failed',
  'app before quit:stop failed',
  'app before quit:blocked',
  'app before quit:runtime stopped',
  'app before quit:committed',
  'app before quit:cancelled',
  'go preflight:begin',
  'go capability materialization:done',
  'go authority:done',
  'go process:spawned',
  'go private frame:done',
  'go ready line:received',
  'go ready identity:verified',
  'go adapter:done',
  'go adapter:failed',
  ...startupOwnerPhasesV1
] as const
export type StartupTraceStage = typeof STARTUP_TRACE_STAGES[number]
const STARTUP_TRACE_STAGE_SET: ReadonlySet<string> = new Set(STARTUP_TRACE_STAGES)

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

function projectGoRuntimeStartupDetail(value: Record<string, unknown>): FixedLogDetail | undefined {
  const { stage, attempt, failureClass, exitCode, signal, cleanupAfterStartupFailure } = value
  if ((stage !== 'go startup:failed' && stage !== 'go process:exited') ||
      !Number.isSafeInteger(attempt) || (attempt as number) <= 0 ||
      (failureClass !== null && (typeof failureClass !== 'string' ||
        !GO_RUNTIME_STARTUP_FAILURE_CLASS_SET.has(failureClass))) ||
      (exitCode !== null && (!Number.isSafeInteger(exitCode) ||
        (exitCode as number) < -2_147_483_648 || (exitCode as number) > 2_147_483_647)) ||
      (signal !== null && typeof signal !== 'string') ||
      typeof cleanupAfterStartupFailure !== 'boolean' ||
      (stage === 'go startup:failed' && (failureClass === null || cleanupAfterStartupFailure)) ||
      (cleanupAfterStartupFailure && failureClass === null)) return undefined
  return {
    stage,
    attempt: attempt as number,
    failureClass: failureClass as GoRuntimeStartupFailureClass | null,
    exitCode: exitCode as number | null,
    signal: signal === null ? null : GO_RUNTIME_STARTUP_SIGNAL_SET.has(signal as string) ? signal as string : 'OTHER',
    cleanupAfterStartupFailure
  }
}

function projectRuntimeStatusDetail(value: Record<string, unknown>): FixedLogDetail | undefined {
  const input: Record<string, unknown> = {}
  for (const key of [
    'code', 'source', 'at', 'attempt', 'maxAttempts', 'stderrBytes', 'stderrSha256',
    'errorBytes', 'errorSha256', 'rolledBack'
  ]) {
    if (!Object.hasOwn(value, key)) continue
    const entry = value[key]
    if (entry === undefined) return undefined
    input[key] = entry
  }
  const status = createRuntimeStatusPublicV1(input as RuntimeStatusPublicV1Input)
  if (!Object.hasOwn(value, 'state') || value.state !== status.state) return undefined
  const projected: FixedLogDetail = { code: status.code, state: status.state, source: status.source }
  if (status.rolledBack !== undefined) projected.rolledBack = status.rolledBack
  if (status.errorBytes !== undefined && status.errorSha256 !== undefined) {
    projected.errorBytes = status.errorBytes
    projected.errorSha256 = status.errorSha256
  }
  return projected
}

function projectFixedDetail(value: unknown, category: string): FixedLogDetail | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  if (category === 'runtime-status') return projectRuntimeStatusDetail(value as Record<string, unknown>)
  if (category === 'sse') {
    const { code, reasonCode } = value as Record<string, unknown>
    if (code === 'sse_event_rejected') {
      return isPublicRuntimeSseRejectionReasonCode(reasonCode) ? { code, reasonCode } : undefined
    }
    if (code === 'sse_ack_rejected') {
      return typeof reasonCode === 'string' && SSE_ACK_DIAGNOSTIC_REASONS.has(reasonCode)
        ? { code, reasonCode } : undefined
    }
  }
  if (category === 'go-runtime-startup') return projectGoRuntimeStartupDetail(value as Record<string, unknown>)
  if (category === 'user-input-local-read') {
    const { stage, statusCode } = value as Record<string, unknown>
    if (typeof stage !== 'string' || !USER_INPUT_LOCAL_READ_DIAGNOSTIC_STAGE_SET.has(stage) ||
        statusCode !== null && (typeof statusCode !== 'number' || !Number.isInteger(statusCode) || statusCode < 100 || statusCode > 599)) return undefined
    return { stage, statusCode }
  }
  if (category === 'native-office') {
    const { stage, errorCode, protectedPresent } = value as Record<string, unknown>
    if (stage === 'core_manual_begin_phase' || Object.hasOwn(value, 'phaseFrame')) {
      if (stage !== 'core_manual_begin_phase' || Object.keys(value).length !== 2 ||
          !Object.hasOwn(value, 'stage') || !Object.hasOwn(value, 'phaseFrame')) return undefined
      const frame = normalizeNativeManualBeginPhaseV1((value as Record<string, unknown>).phaseFrame)
      return frame ? { stage, ...frame } : undefined
    }
    if (typeof stage !== 'string' || !NATIVE_OFFICE_DIAGNOSTIC_STAGE_SET.has(stage) ||
        typeof errorCode !== 'string' || !NATIVE_OFFICE_DIAGNOSTIC_ERROR_CODE_SET.has(errorCode) ||
        typeof protectedPresent !== 'boolean') return undefined
    if (stage === 'controller_save_failure' || Object.hasOwn(value, 'saveFailure')) {
      const state = (value as Record<string, unknown>).saveFailure
      if (stage !== 'controller_save_failure' || protectedPresent || Object.hasOwn(value, 'openState') || Object.hasOwn(value, 'manualBegin') ||
          !state || typeof state !== 'object' || Array.isArray(state) ||
          !NATIVE_OFFICE_SAVE_FAILURE_KEYS.every(key => Object.hasOwn(state, key))) return undefined
      const { phase, outerSchemaAccepted, innerSchemaAccepted, outerCode, innerCode, receiptPresent, receiptStatus } = state as Record<string, unknown>
      if (typeof phase !== 'string' || !NATIVE_OFFICE_SAVE_PHASES.some(allowed => allowed === phase) ||
          outerSchemaAccepted !== null && typeof outerSchemaAccepted !== 'boolean' ||
          innerSchemaAccepted !== null && typeof innerSchemaAccepted !== 'boolean' ||
          outerCode !== null && (typeof outerCode !== 'string' || !Object.hasOwn(NATIVE_OFFICE_MANUAL_BEGIN_OUTER_CODES, outerCode)) ||
          innerCode !== null && (typeof innerCode !== 'string' || !Object.hasOwn(NATIVE_OFFICE_MANUAL_BEGIN_INNER_CODES, innerCode)) ||
          receiptPresent !== null && typeof receiptPresent !== 'boolean' ||
          receiptStatus !== null && (typeof receiptStatus !== 'string' || !NATIVE_OFFICE_SAVE_RECEIPT_STATUSES.some(allowed => allowed === receiptStatus)) ||
          receiptStatus !== null && receiptPresent !== true) return undefined
      return { stage, errorCode, protectedPresent, phase, outerSchemaAccepted, innerSchemaAccepted, outerCode, innerCode, receiptPresent, receiptStatus }
    }
    if (stage === 'controller_manual_begin' || Object.hasOwn(value, 'manualBegin')) {
      const state = (value as Record<string, unknown>).manualBegin
      if (stage !== 'controller_manual_begin' || protectedPresent || Object.hasOwn(value, 'openState') ||
          !state || typeof state !== 'object' || Array.isArray(state) ||
          !NATIVE_OFFICE_MANUAL_BEGIN_FAILURE_KEYS.every(key => Object.hasOwn(state, key))) return undefined
      const { schemaAccepted, outerCode, innerCode, pendingSave, pendingRecovery, pendingManualCancel } = state as Record<string, unknown>
      if (schemaAccepted !== null && typeof schemaAccepted !== 'boolean' ||
          outerCode !== null && (typeof outerCode !== 'string' || !Object.hasOwn(NATIVE_OFFICE_MANUAL_BEGIN_OUTER_CODES, outerCode)) ||
          innerCode !== null && (typeof innerCode !== 'string' || !Object.hasOwn(NATIVE_OFFICE_MANUAL_BEGIN_INNER_CODES, innerCode)) ||
          typeof pendingSave !== 'boolean' || typeof pendingRecovery !== 'boolean' || typeof pendingManualCancel !== 'boolean') return undefined
      return { stage, errorCode, protectedPresent, schemaAccepted, outerCode, innerCode, pendingSave, pendingRecovery, pendingManualCancel }
    }
    const projected: FixedLogDetail = { stage, errorCode, protectedPresent }
    const state = (value as Record<string, unknown>).openState
    if (state !== undefined) {
      if (!protectedPresent || !state || typeof state !== 'object' || Array.isArray(state)) return undefined
      for (const key of NATIVE_OFFICE_OPEN_FAILURE_BOOLEAN_KEYS) {
        const entry = (state as Record<string, unknown>)[key]
        if (typeof entry !== 'boolean') return undefined
        projected[key] = entry
      }
    }
    return projected
  }
  const projected: FixedLogDetail = {}
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    if (key === 'stage' && category === 'startup' && typeof entry === 'string' && STARTUP_TRACE_STAGE_SET.has(entry)) {
      projected.stage = entry
      continue
    }
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
    const fixedDetail = projectFixedDetail(detail, category)
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

export function logNativeOfficeFailure(stage: NativeOfficeDiagnosticStage, errorCode: NativeOfficeDiagnosticErrorCode, protectedPresent: boolean, openState?: NativeOfficeOpenFailureState): void {
  logWarn('native-office', '', { stage, errorCode, protectedPresent, ...(openState ? { openState } : {}) })
}

export function logNativeOfficeManualBeginFailure(errorCode: NativeOfficeDiagnosticErrorCode, state: NativeOfficeManualBeginFailureState): void {
  logWarn('native-office', '', { stage: 'controller_manual_begin', errorCode, protectedPresent: false, manualBegin: state })
}

export function logNativeOfficeSaveFailure(errorCode: NativeOfficeDiagnosticErrorCode, state: NativeOfficeSaveFailureState): void {
  logWarn('native-office', '', { stage: 'controller_save_failure', errorCode, protectedPresent: false, saveFailure: state })
}

export function logNativeOfficeManualBeginPhaseV1(frame: NativeManualBeginPhaseV1): void {
  logWarn('native-office', '', { stage: 'core_manual_begin_phase', phaseFrame: frame })
}

export function logUserInputLocalReadFailure(stage: UserInputLocalReadDiagnosticStage, statusCode: number | null): void {
  logWarn('user-input-local-read', '', { stage, statusCode })
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
