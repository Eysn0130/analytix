export const RUNTIME_STATUS_PUBLIC_V1_STATES = [
  'starting',
  'running',
  'restarting',
  'crashed',
  'failed',
  'stopped'
] as const

export type RuntimeStatusPublicStateV1 = typeof RUNTIME_STATUS_PUBLIC_V1_STATES[number]

export const RUNTIME_STATUS_PUBLIC_V1_SOURCES = [
  'ensure',
  'restart',
  'supervisor',
  'watchdog',
  'settings-apply',
  'settings-apply-rollback',
  'mcp-config',
  'hub-account',
  'browser-preview'
] as const

export type RuntimeStatusPublicSourceV1 = typeof RUNTIME_STATUS_PUBLIC_V1_SOURCES[number]

type RuntimeStatusPublicCodeSpecV1 = {
  state: RuntimeStatusPublicStateV1
  message: string
  sources: readonly RuntimeStatusPublicSourceV1[]
}

const RUNTIME_STATUS_PUBLIC_V1_CODE_SPECS = {
  runtime_starting: {
    state: 'starting',
    message: 'Analytix runtime is starting.',
    sources: ['ensure', 'restart']
  },
  runtime_ready: {
    state: 'running',
    message: 'Analytix runtime is ready.',
    sources: ['ensure', 'restart', 'supervisor', 'watchdog', 'settings-apply', 'mcp-config']
  },
  supervisor_unexpected_exit: {
    state: 'crashed',
    message: 'Analytix runtime exited unexpectedly.',
    sources: ['supervisor']
  },
  supervisor_restart_scheduled: {
    state: 'restarting',
    message: 'Analytix runtime is restarting automatically.',
    sources: ['supervisor']
  },
  supervisor_restart_unavailable: {
    state: 'stopped',
    message: 'Analytix runtime stopped because automatic restart is unavailable.',
    sources: ['supervisor']
  },
  supervisor_restart_exhausted: {
    state: 'failed',
    message: 'Analytix runtime stopped after the automatic restart limit was reached.',
    sources: ['supervisor']
  },
  watchdog_restart_scheduled: {
    state: 'restarting',
    message: 'Analytix runtime stopped responding and is restarting.',
    sources: ['watchdog']
  },
  watchdog_restart_failed: {
    state: 'failed',
    message: 'Analytix runtime stopped responding and could not be restarted.',
    sources: ['watchdog']
  },
  settings_apply_stopped: {
    state: 'stopped',
    message: 'Analytix runtime stopped after a settings change disabled startup.',
    sources: ['settings-apply']
  },
  settings_apply_restart_scheduled: {
    state: 'restarting',
    message: 'Analytix runtime is restarting to apply settings.',
    sources: ['settings-apply']
  },
  settings_rollback_stopped: {
    state: 'stopped',
    message: 'Previous runtime settings were restored, but automatic startup is unavailable.',
    sources: ['settings-apply-rollback']
  },
  settings_rollback_ready: {
    state: 'running',
    message: 'Analytix runtime is using the restored settings.',
    sources: ['settings-apply-rollback']
  },
  settings_rollback_failed: {
    state: 'failed',
    message: 'Previous runtime settings were restored, but the runtime could not be started.',
    sources: ['settings-apply-rollback']
  },
  mcp_config_restart_scheduled: {
    state: 'restarting',
    message: 'Analytix runtime is restarting to apply the MCP configuration.',
    sources: ['mcp-config']
  },
  mcp_config_restart_failed: {
    state: 'failed',
    message: 'Analytix runtime could not restart after the MCP configuration changed.',
    sources: ['mcp-config']
  },
  hub_account_signed_out: {
    state: 'stopped',
    message: 'Analytix runtime stopped because the Hub account is signed out.',
    sources: ['hub-account']
  },
  browser_preview_ready: {
    state: 'running',
    message: 'Analytix browser preview is connected to the local runtime proxy.',
    sources: ['browser-preview']
  }
} as const satisfies Record<string, RuntimeStatusPublicCodeSpecV1>

export type RuntimeStatusPublicCodeV1 = keyof typeof RUNTIME_STATUS_PUBLIC_V1_CODE_SPECS

export const RUNTIME_STATUS_PUBLIC_V1_CODES = Object.freeze(
  Object.keys(RUNTIME_STATUS_PUBLIC_V1_CODE_SPECS) as RuntimeStatusPublicCodeV1[]
)

export type RuntimeStatusPublicV1 = {
  state: RuntimeStatusPublicStateV1
  source: RuntimeStatusPublicSourceV1
  code: RuntimeStatusPublicCodeV1
  message: string
  attempt?: number
  maxAttempts?: number
  stderrBytes?: number
  stderrSha256?: string
  errorBytes?: number
  errorSha256?: string
  rolledBack?: boolean
  at: string
}

export type RuntimeStatusPublicV1Input = {
  source: RuntimeStatusPublicSourceV1
  code: RuntimeStatusPublicCodeV1
  attempt?: number
  maxAttempts?: number
  stderrBytes?: number
  stderrSha256?: string
  errorBytes?: number
  errorSha256?: string
  rolledBack?: boolean
  at?: string | Date
}

const PUBLIC_KEYS = new Set([
  'state',
  'source',
  'code',
  'message',
  'attempt',
  'maxAttempts',
  'stderrBytes',
  'stderrSha256',
  'errorBytes',
  'errorSha256',
  'rolledBack',
  'at'
])

const INPUT_KEYS = new Set([
  'source',
  'code',
  'attempt',
  'maxAttempts',
  'stderrBytes',
  'stderrSha256',
  'errorBytes',
  'errorSha256',
  'rolledBack',
  'at'
])

const ATTEMPT_CODES = new Set<RuntimeStatusPublicCodeV1>([
  'supervisor_restart_scheduled'
])

const ROLLBACK_CODES = new Set<RuntimeStatusPublicCodeV1>([
  'settings_rollback_stopped',
  'settings_rollback_ready',
  'settings_rollback_failed'
])

const SHA256_PATTERN = /^[a-f0-9]{64}$/u

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function hasOnlyKeys(value: Record<string, unknown>, allowed: ReadonlySet<string>): boolean {
  return Object.keys(value).every((key) => allowed.has(key))
}

function hasOwn(value: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key)
}

function isSafeByteCount(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
}

function isCanonicalIsoTimestamp(value: unknown): value is string {
  if (typeof value !== 'string') return false
  try {
    return new Date(value).toISOString() === value
  } catch {
    return false
  }
}

function diagnosticPairIsValid(
  value: Record<string, unknown>,
  bytesKey: 'stderrBytes' | 'errorBytes',
  shaKey: 'stderrSha256' | 'errorSha256'
): boolean {
  const hasBytes = hasOwn(value, bytesKey)
  const hasSha = hasOwn(value, shaKey)
  if (hasBytes !== hasSha) return false
  if (!hasBytes) return true
  return isSafeByteCount(value[bytesKey]) &&
    typeof value[shaKey] === 'string' &&
    SHA256_PATTERN.test(value[shaKey])
}

function attemptPairIsValid(
  value: Record<string, unknown>,
  code: RuntimeStatusPublicCodeV1
): boolean {
  const hasAttempt = hasOwn(value, 'attempt')
  const hasMaxAttempts = hasOwn(value, 'maxAttempts')
  if (hasAttempt !== hasMaxAttempts) return false
  if (!ATTEMPT_CODES.has(code)) return !hasAttempt
  if (!hasAttempt) return false
  return isPositiveSafeInteger(value.attempt) &&
    isPositiveSafeInteger(value.maxAttempts) &&
    value.attempt <= value.maxAttempts
}

function rollbackFlagIsValid(
  value: Record<string, unknown>,
  code: RuntimeStatusPublicCodeV1
): boolean {
  const hasRolledBack = hasOwn(value, 'rolledBack')
  if (ROLLBACK_CODES.has(code)) return hasRolledBack && value.rolledBack === true
  return !hasRolledBack
}

function readCode(value: unknown): RuntimeStatusPublicCodeV1 | null {
  if (typeof value !== 'string') return null
  return Object.prototype.hasOwnProperty.call(RUNTIME_STATUS_PUBLIC_V1_CODE_SPECS, value)
    ? value as RuntimeStatusPublicCodeV1
    : null
}

/**
 * Parses the desktop runtime-status IPC payload as an exact public value.
 * Unknown keys, arbitrary messages, non-canonical timestamps, incomplete
 * diagnostics, and invalid code/state/source combinations fail closed.
 */
export function parseRuntimeStatusPublicV1(value: unknown): RuntimeStatusPublicV1 | null {
  if (!isRecord(value) || !hasOnlyKeys(value, PUBLIC_KEYS)) return null
  const code = readCode(value.code)
  if (!code) return null
  const spec = RUNTIME_STATUS_PUBLIC_V1_CODE_SPECS[code]
  if (value.state !== spec.state || value.message !== spec.message) return null
  if (typeof value.source !== 'string' || !spec.sources.some((source) => source === value.source)) return null
  if (!isCanonicalIsoTimestamp(value.at)) return null
  if (!diagnosticPairIsValid(value, 'stderrBytes', 'stderrSha256')) return null
  if (!diagnosticPairIsValid(value, 'errorBytes', 'errorSha256')) return null
  if (!attemptPairIsValid(value, code) || !rollbackFlagIsValid(value, code)) return null

  return Object.freeze({ ...value }) as RuntimeStatusPublicV1
}

export function isRuntimeStatusPublicV1(value: unknown): value is RuntimeStatusPublicV1 {
  return parseRuntimeStatusPublicV1(value) !== null
}

function canonicalizeAt(value: RuntimeStatusPublicV1Input['at']): string {
  const date = value === undefined ? new Date() : value instanceof Date ? value : new Date(value)
  return date.toISOString()
}

/** Creates the only permitted host copy for a public runtime status. */
export function createRuntimeStatusPublicV1(input: RuntimeStatusPublicV1Input): RuntimeStatusPublicV1 {
  if (!isRecord(input) || !hasOnlyKeys(input, INPUT_KEYS)) {
    throw new TypeError('invalid_runtime_status_public_v1_input')
  }
  const code = readCode(input.code)
  if (!code) throw new TypeError('invalid_runtime_status_public_v1_code')
  const spec = RUNTIME_STATUS_PUBLIC_V1_CODE_SPECS[code]
  const candidate: Record<string, unknown> = {
    state: spec.state,
    source: input.source,
    code,
    message: spec.message,
    at: canonicalizeAt(input.at)
  }
  for (const key of [
    'attempt',
    'maxAttempts',
    'stderrBytes',
    'stderrSha256',
    'errorBytes',
    'errorSha256',
    'rolledBack'
  ] as const) {
    if (input[key] !== undefined) candidate[key] = input[key]
  }
  const parsed = parseRuntimeStatusPublicV1(candidate)
  if (!parsed) throw new TypeError('invalid_runtime_status_public_v1_input')
  return parsed
}
