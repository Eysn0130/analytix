import { describe, expect, it } from 'vitest'
import {
  RUNTIME_STATUS_PUBLIC_V1_CODES,
  createRuntimeStatusPublicV1,
  isRuntimeStatusPublicV1,
  parseRuntimeStatusPublicV1,
  type RuntimeStatusPublicCodeV1,
  type RuntimeStatusPublicSourceV1
} from './analytix-runtime-status'

const AT = '2026-07-20T06:30:00.000Z'
const SHA256 = 'a'.repeat(64)

const VALID_SOURCE_BY_CODE = {
  runtime_starting: 'ensure',
  runtime_ready: 'restart',
  supervisor_unexpected_exit: 'supervisor',
  supervisor_restart_scheduled: 'supervisor',
  supervisor_restart_unavailable: 'supervisor',
  supervisor_restart_exhausted: 'supervisor',
  watchdog_restart_scheduled: 'watchdog',
  watchdog_restart_failed: 'watchdog',
  settings_apply_stopped: 'settings-apply',
  settings_apply_restart_scheduled: 'settings-apply',
  settings_rollback_stopped: 'settings-apply-rollback',
  settings_rollback_ready: 'settings-apply-rollback',
  settings_rollback_failed: 'settings-apply-rollback',
  mcp_config_restart_scheduled: 'mcp-config',
  mcp_config_restart_failed: 'mcp-config',
  hub_account_signed_out: 'hub-account',
  browser_preview_ready: 'browser-preview'
} as const satisfies Record<RuntimeStatusPublicCodeV1, RuntimeStatusPublicSourceV1>

function inputFor(code: RuntimeStatusPublicCodeV1) {
  return {
    code,
    source: VALID_SOURCE_BY_CODE[code],
    at: AT,
    ...(code === 'supervisor_restart_scheduled'
      ? { attempt: 1, maxAttempts: 3 }
      : {}),
    ...(code.startsWith('settings_rollback_')
      ? { rolledBack: true as const }
      : {})
  }
}

describe('RuntimeStatusPublicV1', () => {
  it('creates every closed code with its host-owned state and fixed copy', () => {
    expect(Object.keys(VALID_SOURCE_BY_CODE).sort()).toEqual([...RUNTIME_STATUS_PUBLIC_V1_CODES].sort())
    for (const code of RUNTIME_STATUS_PUBLIC_V1_CODES) {
      const value = createRuntimeStatusPublicV1(inputFor(code))
      expect(value.code).toBe(code)
      expect(value.at).toBe(AT)
      expect(value.message).toMatch(/^Analytix|^Previous/u)
      expect(parseRuntimeStatusPublicV1(value)).toEqual(value)
      expect(isRuntimeStatusPublicV1(value)).toBe(true)
    }
  })

  it('canonicalizes creator timestamps and preserves only diagnostic hashes, not private text', () => {
    const value = createRuntimeStatusPublicV1({
      code: 'watchdog_restart_failed',
      source: 'watchdog',
      stderrBytes: 0,
      stderrSha256: SHA256,
      errorBytes: 117,
      errorSha256: 'b'.repeat(64),
      at: '2026-07-20T14:30:00+08:00'
    })

    expect(value.at).toBe(AT)
    expect(value).toEqual({
      state: 'failed',
      source: 'watchdog',
      code: 'watchdog_restart_failed',
      message: 'Analytix runtime stopped responding and could not be restarted.',
      stderrBytes: 0,
      stderrSha256: SHA256,
      errorBytes: 117,
      errorSha256: 'b'.repeat(64),
      at: AT
    })
  })

  it.each([
    ['arbitrary message', { message: 'private provider error: secret' }],
    ['additional field', { privateError: 'secret' }],
    ['unknown code', { code: 'runtime_unknown' }],
    ['mismatched state', { state: 'running' }],
    ['mismatched source', { source: 'hub-account' }],
    ['non-canonical ISO timestamp', { at: '2026-07-20T14:30:00+08:00' }],
    ['short stderr hash', { stderrBytes: 1, stderrSha256: 'abcd' }],
    ['uppercase error hash', { errorBytes: 1, errorSha256: 'A'.repeat(64) }],
    ['orphaned stderr byte count', { stderrBytes: 1 }],
    ['orphaned error hash', { errorSha256: SHA256 }],
    ['attempt metadata on another code', { attempt: 1, maxAttempts: 3 }],
    ['rollback flag on another code', { rolledBack: true }]
  ])('rejects %s', (_label, patch) => {
    const valid = createRuntimeStatusPublicV1({
      code: 'watchdog_restart_failed',
      source: 'watchdog',
      at: AT
    })
    expect(parseRuntimeStatusPublicV1({ ...valid, ...patch })).toBeNull()
  })

  it('requires bounded restart attempt metadata and the canonical rollback flag', () => {
    const restart = createRuntimeStatusPublicV1({
      code: 'supervisor_restart_scheduled',
      source: 'supervisor',
      attempt: 1,
      maxAttempts: 3,
      at: AT
    })
    expect(parseRuntimeStatusPublicV1({ ...restart, attempt: 4 })).toBeNull()
    expect(parseRuntimeStatusPublicV1({ ...restart, maxAttempts: undefined })).toBeNull()

    const rollback = createRuntimeStatusPublicV1({
      code: 'settings_rollback_ready',
      source: 'settings-apply-rollback',
      rolledBack: true,
      at: AT
    })
    expect(parseRuntimeStatusPublicV1({ ...rollback, rolledBack: false })).toBeNull()
    const { rolledBack: _removed, ...withoutRollback } = rollback
    expect(parseRuntimeStatusPublicV1(withoutRollback)).toBeNull()
  })

  it('rejects malformed creator input instead of silently dropping it', () => {
    expect(() => createRuntimeStatusPublicV1({
      ...inputFor('runtime_ready'),
      message: 'caller supplied copy'
    } as never)).toThrow('invalid_runtime_status_public_v1_input')
    expect(() => createRuntimeStatusPublicV1({
      code: 'runtime_ready',
      source: 'browser-preview',
      at: AT
    })).toThrow('invalid_runtime_status_public_v1_input')
  })
})
