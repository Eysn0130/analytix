import { mkdtemp, readFile, readdir, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import loggerSource from './logger.ts?raw'
import { PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODES } from '../shared/public-runtime-sse'
import { createRuntimeStatusPublicV1 } from '../shared/analytix-runtime-status'
import {
  appendManagedChildLogRecord,
  configureLogger,
  GO_RUNTIME_STARTUP_FAILURE_CLASSES,
  logError,
  logInfo,
  logNativeOfficeFailure,
  logNativeOfficeManualBeginFailure,
  logNativeOfficeManualBeginPhaseV1,
  logNativeOfficeSaveFailure,
  type NativeOfficeManualBeginFailureState,
  type NativeOfficeSaveFailureState,
  logWarn,
  publicConsoleError,
  publicConsoleInfo,
  publicConsoleWarn
} from './logger'
import type { NativeManualBeginPhaseV1 } from './runtime/native-manual-begin-phase-v1'

let testDir = ''

const sha256 = 'a'.repeat(64)
const directCanaries = [
  'NEUTRAL_EXACT_CELL_CANARY_7F3C',
  'file:///private/case/source.csv',
  '/private/case/source.csv',
  'C:\\Cases\\source.csv',
  '\\\\server\\share\\case.csv',
  'SELECT 1',
  'UPDATE source_rows SET account = 1 WHERE row_id = 2',
  "tool='read' input='/private/case/source.csv'",
  'tool=read arguments={path:"/private/case/source.csv"}',
  '{"tool":"read","arguments":{"path":"/private/case/source.csv"}}',
  'ACCT:1',
  'acct:1',
  `cer1_${'b'.repeat(64)}`,
  '6222020202020202020',
  'victim@example.com',
  '13800138000',
  'AA:BB:CC:DD:EE:FF',
  '<think>PRIVATE_REASONING_CANARY</think>',
  'Authorization: Bearer PRIVATE_BEARER_CANARY',
  'api_key=PRIVATE_API_KEY_CANARY'
]

async function managedLogContent(): Promise<string> {
  const files = await readdir(testDir)
  return (await Promise.all(files.map((entry) => readFile(join(testDir, entry), 'utf8')))).join('\n')
}

afterEach(async () => {
  configureLogger({ dir: '', enabled: false })
  vi.restoreAllMocks()
  if (testDir) await rm(testDir, { recursive: true, force: true })
  testDir = ''
})

describe('managed logger closed projection', () => {
  it('persists fixed event codes and allowlisted scalar metadata, never caller free text', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-logger-closed-'))
    configureLogger({ dir: testDir, enabled: true, retentionDays: 2 })
    const callerText = directCanaries.join(' | ')

    logError('renderer-ipc', callerText, {
      code: 'renderer_send_message_failed',
      resultCount: 2,
      retryable: false,
      schemaVersion: 1,
      sha256,
      message: callerText,
      path: directCanaries[2],
      arbitrary: { sourceExactValue: directCanaries[0] }
    })
    logError('NEUTRAL_CALLER_CATEGORY_CANARY', callerText, { message: callerText })
    logWarn('NEUTRAL_WARN_CATEGORY_CANARY', callerText, { status: callerText })
    logInfo('NEUTRAL_INFO_CATEGORY_CANARY', callerText, { body: callerText })

    await vi.waitFor(async () => {
      const content = await managedLogContent()
      for (const canary of directCanaries) expect(content).not.toContain(canary)
      expect(content).not.toContain('NEUTRAL_CALLER_CATEGORY_CANARY')
      expect(content).not.toContain('NEUTRAL_WARN_CATEGORY_CANARY')
      expect(content).not.toContain('NEUTRAL_INFO_CATEGORY_CANARY')
      expect(content).not.toContain('sourceExactValue')
      expect(content).toContain('[renderer] event=renderer_send_message_failed')
      expect(content).toContain('[main] event=main_error')
      expect(content).toContain('[main] event=main_warn')
      expect(content).toContain('[main] event=main_info')
      expect(content).toContain('"resultCount":2')
      expect(content).toContain('"retryable":false')
      expect(content).toContain('"schemaVersion":1')
      expect(content).toContain(`"sha256":"${sha256}"`)
    })
  })

  it('uses the same closed projector for the public console', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const callerText = directCanaries.join(' | ')

    publicConsoleError('renderer-ipc', callerText, {
      code: 'renderer_send_message_failed',
      resultCount: 3,
      retryable: true,
      sha256,
      message: callerText,
      tool: directCanaries[8]
    })

    const output = error.mock.calls.flat().join(' ')
    for (const canary of directCanaries) expect(output).not.toContain(canary)
    expect(output).toContain('[renderer] event=renderer_send_message_failed')
    expect(output).toContain('"resultCount":3')
    expect(output).toContain('"retryable":true')
    expect(output).toContain(`"sha256":"${sha256}"`)
  })

  it('keeps contract-valid runtime rollback diagnostics in managed logs and public console', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-runtime-status-logger-'))
    configureLogger({ dir: testDir, enabled: true, retentionDays: 2 })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const { message, at, ...status } = createRuntimeStatusPublicV1({
      code: 'settings_rollback_ready', source: 'settings-apply-rollback', rolledBack: true,
      errorBytes: 27, errorSha256: sha256
    })
    const privateText = directCanaries.join(' | ')
    const detail = {
      ...status, error: privateText, message: privateText, body: privateText,
      path: privateText, token: privateText, arbitrary: privateText
    }
    logWarn('runtime-status', privateText, detail)
    publicConsoleWarn('runtime-status', privateText, detail)
    const expected = {
      code: 'settings_rollback_ready', state: 'running', source: 'settings-apply-rollback',
      rolledBack: true, errorBytes: 27, errorSha256: sha256
    }
    const [consoleLine] = warn.mock.calls[0]
    expect(JSON.parse(consoleLine.split(' detail=')[1])).toEqual(expected)
    expect(consoleLine).not.toContain(message)
    expect(consoleLine).not.toContain(at)
    for (const canary of directCanaries) expect(consoleLine).not.toContain(canary)
    await vi.waitFor(async () => {
      const line = (await managedLogContent()).split('\n').find(entry => entry.includes('"code":"settings_rollback_ready"'))
      expect(line).toBeDefined()
      expect(JSON.parse(line!.split(' detail=')[1])).toEqual(expected)
      for (const canary of directCanaries) expect(line).not.toContain(canary)
    })
  })

  it('keeps runtime status fields without inventing rollback or an error summary', () => {
    const info = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    publicConsoleInfo('runtime-status', '', { code: 'runtime_ready', state: 'running', source: 'ensure' })
    expect(JSON.parse(info.mock.calls[0][0].split(' detail=')[1])).toEqual({
      code: 'runtime_ready', state: 'running', source: 'ensure'
    })
    publicConsoleInfo('main', '', {
      code: 'runtime_ready', state: 'running', source: 'ensure', errorBytes: 27, errorSha256: sha256
    })
    expect(info.mock.calls[1][0]).toBe('[analytix] [main] event=main_info')
    publicConsoleInfo('runtime-status', '', {
      code: 'supervisor_restart_scheduled', state: 'restarting', source: 'supervisor',
      attempt: 1, maxAttempts: 3
    })
    expect(JSON.parse(info.mock.calls[2][0].split(' detail=')[1])).toEqual({
      code: 'supervisor_restart_scheduled', state: 'restarting', source: 'supervisor'
    })
  })

  it('rejects invalid runtime enum combinations, rollback flags and error-summary pairs as a whole', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-runtime-status-invalid-'))
    configureLogger({ dir: testDir, enabled: true, retentionDays: 2 })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const valid = {
      code: 'settings_rollback_ready', state: 'running', source: 'settings-apply-rollback',
      rolledBack: true, errorBytes: 27, errorSha256: sha256
    }
    const { rolledBack: _rolledBack, ...missingRollback } = valid
    const { errorSha256: _errorSha256, ...missingDigest } = valid
    const { errorBytes: _errorBytes, ...missingBytes } = valid
    const { state: _state, ...missingState } = valid
    const invalid = [
      { ...valid, code: directCanaries[0] }, { ...valid, state: directCanaries[0] },
      { ...valid, state: 'failed' }, { ...valid, source: directCanaries[0] },
      { ...valid, source: 'ensure' }, { ...valid, rolledBack: false }, missingRollback, missingState,
      missingDigest, missingBytes, { ...valid, errorBytes: -1 }, { ...valid, errorBytes: 1.5 },
      { ...valid, errorBytes: Number.MAX_SAFE_INTEGER + 1 }, { ...valid, errorBytes: NaN },
      { ...valid, errorSha256: directCanaries[0] }, { ...valid, errorSha256: 'A'.repeat(64) },
      { ...valid, errorSha256: 'a'.repeat(63) }, { ...valid, errorSha256: undefined },
      { ...valid, errorBytes: undefined, errorSha256: undefined },
      { ...valid, stderrBytes: 3 }, { ...valid, stderrSha256: sha256 },
      { ...valid, stderrBytes: undefined, stderrSha256: undefined },
      { ...valid, attempt: 1, maxAttempts: 3 },
      { ...valid, attempt: undefined, maxAttempts: undefined }, { ...valid, at: 'invalid' },
      { code: 'runtime_ready', state: 'running', source: 'ensure', rolledBack: true },
      { code: 'runtime_ready', state: 'running', source: 'ensure', rolledBack: undefined },
      { code: 'supervisor_restart_scheduled', state: 'restarting', source: 'supervisor' },
      Object.defineProperty({ ...valid }, 'state', { get() { throw new Error(directCanaries[0]) } })
    ]
    for (const detail of invalid) {
      expect(() => publicConsoleWarn('runtime-status', directCanaries[0], detail)).not.toThrow()
      logWarn('runtime-status', directCanaries[0], detail)
    }
    for (const [line] of warn.mock.calls) expect(line).toBe('[analytix] [main] event=main_warn')
    await vi.waitFor(async () => {
      const lines = (await managedLogContent()).split('\n').filter(line => line.includes('[main] event=main_warn'))
      expect(lines).toHaveLength(invalid.length)
      for (const line of lines) expect(line).not.toContain(' detail=')
    })
  })

  it('keeps an allowlisted startup stage and elapsed time without leaking caller text', () => {
    const info = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    publicConsoleInfo('startup', directCanaries.join(' | '), {
      stage: 'app.whenReady:start',
      elapsedMs: 42,
      path: directCanaries[2]
    })
    publicConsoleInfo('startup', 'untrusted stage', {
      stage: directCanaries[0],
      elapsedMs: 43
    })
    publicConsoleInfo('startup', 'safe runtime transition', {
      stage: 'runtime IPC restart:requested',
      elapsedMs: 45,
      path: directCanaries[2]
    })
    publicConsoleInfo('startup', 'safe owner phase', {
      stage: 'go_semantic_prepared',
      durationMs: 31000,
      path: directCanaries[2]
    })
    publicConsoleInfo('main', 'wrong category', {
      stage: 'app.whenReady:start',
      elapsedMs: 44
    })
    const [accepted, rejected, transition, owner, wrongCategory] = info.mock.calls.map((call) => call.join(' '))
    expect(accepted).toContain('"stage":"app.whenReady:start"')
    expect(accepted).toContain('"elapsedMs":42')
    expect(rejected).not.toContain('"stage"')
    expect(transition).toContain('"stage":"runtime IPC restart:requested"')
    expect(transition).not.toContain(directCanaries[2])
    expect(owner).toContain('"stage":"go_semantic_prepared"')
    expect(owner).toContain('"durationMs":31000')
    expect(owner).not.toContain(directCanaries[2])
    expect(wrongCategory).not.toContain('"stage"')
    for (const canary of directCanaries) {
      expect(accepted).not.toContain(canary)
      expect(rejected).not.toContain(canary)
    }
  })

  it('projects Go startup branches and actual exit fields without free text or stderr content', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-go-startup-logger-'))
    configureLogger({ dir: testDir, enabled: true, retentionDays: 2 })
    const info = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    const privateText = directCanaries.join(' | ')
    for (const failureClass of GO_RUNTIME_STARTUP_FAILURE_CLASSES) {
      const detail = {
        stage: 'go startup:failed', attempt: 7, failureClass, exitCode: null, signal: null,
        cleanupAfterStartupFailure: false, message: privateText, stack: privateText,
        stderr: privateText, headers: { authorization: privateText }
      }
      publicConsoleInfo('go-runtime-startup', privateText, detail)
      logInfo('go-runtime-startup', privateText, detail)
    }
    publicConsoleInfo('go-runtime-startup', privateText, {
      stage: 'go process:exited', attempt: 8, failureClass: 'ready_parse_failed',
      exitCode: -1, signal: 'SIGABRT', cleanupAfterStartupFailure: true
    })
    publicConsoleInfo('go-runtime-startup', privateText, {
      stage: 'go process:exited', attempt: 9, failureClass: null,
      exitCode: null, signal: privateText, cleanupAfterStartupFailure: false
    })
    const output = info.mock.calls.flat().join(' ')
    for (const canary of directCanaries) expect(output).not.toContain(canary)
    expect(output).not.toContain('stderr')
    expect(output).not.toContain('headers')
    expect(output).toContain('"exitCode":-1')
    expect(output).toContain('"signal":"SIGABRT"')
    expect(output).toContain('"signal":"OTHER"')
    expect(output).toContain('"failureClass":null')
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      for (const failureClass of GO_RUNTIME_STARTUP_FAILURE_CLASSES) {
        expect(content).toContain(`"failureClass":"${failureClass}"`)
      }
      for (const canary of directCanaries) expect(content).not.toContain(canary)
      expect(content).not.toContain('stderr')
      expect(content).not.toContain('headers')
      expect(content).toContain('"exitCode":null')
    })
  })

  it('rejects malformed or hostile Go startup metadata as a whole', () => {
    const info = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    const valid = {
      stage: 'go startup:failed', attempt: 1, failureClass: 'ready_parse_failed',
      exitCode: null, signal: null, cleanupAfterStartupFailure: false
    }
    for (const invalid of [
      { ...valid, stage: directCanaries[0] },
      { ...valid, attempt: 0 },
      { ...valid, failureClass: directCanaries[0] },
      { ...valid, failureClass: null },
      { ...valid, exitCode: 0.5 },
      { ...valid, exitCode: 2_147_483_648 },
      { ...valid, signal: { message: directCanaries[0] } },
      { ...valid, cleanupAfterStartupFailure: true },
      Object.defineProperty({ ...valid }, 'stage', { get() { throw new Error(directCanaries[0]) } })
    ]) {
      expect(() => publicConsoleInfo('go-runtime-startup', directCanaries[0], invalid)).not.toThrow()
    }
    for (const [line] of info.mock.calls) {
      expect(line).toBe('[analytix] [main] event=main_info')
    }
  })

  it('fails closed without throwing when hostile details reject enumeration or property reads', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-logger-hostile-detail-'))
    configureLogger({ dir: testDir, enabled: true, retentionDays: 2 })
    const hostileCanary = 'HOSTILE_DETAIL_CANARY_7F3C'
    const hostileOwnKeys = new Proxy({ hostileCanary }, {
      ownKeys() {
        throw new Error(hostileCanary)
      }
    })
    const hostileCodeGetter = Object.defineProperty({ resultCount: 99 }, 'code', {
      enumerable: false,
      get() {
        throw new Error(hostileCanary)
      }
    })

    expect(() => logError('renderer-ipc', hostileCanary, hostileCodeGetter)).not.toThrow()
    expect(() => logWarn('hostile-warn', hostileCanary, hostileOwnKeys)).not.toThrow()
    expect(() => logInfo('hostile-info', hostileCanary, hostileOwnKeys)).not.toThrow()

    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const info = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    expect(() => publicConsoleError('renderer-ipc', hostileCanary, hostileCodeGetter)).not.toThrow()
    expect(() => publicConsoleWarn('hostile-warn', hostileCanary, hostileOwnKeys)).not.toThrow()
    expect(() => publicConsoleInfo('hostile-info', hostileCanary, hostileOwnKeys)).not.toThrow()

    await vi.waitFor(async () => {
      const content = await managedLogContent()
      expect(content).not.toContain(hostileCanary)
      expect(content).not.toContain('"resultCount":99')
      expect(content).toContain('[renderer] event=renderer_diagnostic_failed')
      expect(content).toContain('[main] event=main_warn')
      expect(content).toContain('[main] event=main_info')
    })
    const consoleOutput = [...error.mock.calls, ...warn.mock.calls, ...info.mock.calls].flat().join(' ')
    expect(consoleOutput).not.toContain(hostileCanary)
    expect(consoleOutput).not.toContain('"resultCount":99')
    expect(consoleOutput).toContain('[renderer] event=renderer_diagnostic_failed')
    expect(consoleOutput).toContain('[main] event=main_warn')
    expect(consoleOutput).toContain('[main] event=main_info')
  })

  it('rejects raw or malformed child lines and accepts only the fixed lifecycle grammar', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-child-logger-closed-'))
    configureLogger({ dir: testDir, enabled: true, retentionDays: 2 })

    await appendManagedChildLogRecord(directCanaries[0] as never)
    await appendManagedChildLogRecord({
      code: 'ANALYTIX_CHILD_STDERR_CAPTURED',
      bytes: 99,
      sha256: directCanaries[0]
    } as never)
    await appendManagedChildLogRecord({ code: 'ANALYTIX_CHILD_READY', port: 8788 })
    await appendManagedChildLogRecord({
      code: 'ANALYTIX_CHILD_STDERR_CAPTURED',
      bytes: 99,
      sha256
    })

    const content = await managedLogContent()
    for (const canary of directCanaries) expect(content).not.toContain(canary)
    expect(content).toContain('[child] event=ANALYTIX_CHILD_READY detail={"port":8788}')
    expect(content).toContain(
      `[child] event=ANALYTIX_CHILD_STDERR_CAPTURED detail={"bytes":99,"sha256":"${sha256}"}`
    )
  })

  it('does not export a raw managed-log line writer or pattern-based free-text projector', () => {
    expect(loggerSource).not.toContain('appendManagedLogLine')
    expect(loggerSource).not.toContain('projectManagedLogText')
    expect(loggerSource).not.toContain('ABSOLUTE_PATH_PATTERN')
    expect(loggerSource).toContain('appendManagedChildLogRecord')
    expect(loggerSource).toContain('projectFixedDetail')
  })
})

describe('native Office diagnostic projection', () => {
  it('persists only closed stage, error code and preview kind', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-office-logger-'))
    configureLogger({ dir: testDir, enabled: true })
    logWarn('native-office', directCanaries.join(' | '), {
      stage: 'surface_load', errorCode: 'office-startup-failed', protectedPresent: true,
      path: directCanaries[2], error: directCanaries[0], stack: directCanaries[0]
    })
    await vi.waitFor(async () => {
      const body = await managedLogContent()
      expect(body).toContain('detail={"stage":"surface_load","errorCode":"office-startup-failed","protectedPresent":true}')
      for (const canary of directCanaries) expect(body).not.toContain(canary)
    })
  })

  it('preserves forbidden Office failures without exporting source details', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-office-forbidden-logger-'))
    configureLogger({ dir: testDir, enabled: true })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const safe = { stage: 'controller_open_object', errorCode: 'forbidden', protectedPresent: false }
    const hostile = { ...safe, path: directCanaries[2], error: directCanaries[0], stack: directCanaries.join(' | ') }
    logNativeOfficeFailure('controller_open_object', 'forbidden', false)
    logWarn('native-office', directCanaries.join(' | '), hostile)
    publicConsoleWarn('native-office', directCanaries.join(' | '), hostile)
    const detail = JSON.stringify(safe)
    expect(warn).toHaveBeenCalledWith(`[analytix] [main] event=main_warn detail=${detail}`)
    await vi.waitFor(async () => {
      const body = await managedLogContent()
      expect(body).toContain(`detail=${detail}`)
      for (const canary of directCanaries) expect(body).not.toContain(canary)
    })
    for (const call of warn.mock.calls) for (const canary of directCanaries) expect(call.join(' ')).not.toContain(canary)
  })

  it('retains only complete closed protected-open booleans', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-office-checkpoint-logger-'))
    configureLogger({ dir: testDir, enabled: true })
    const openState = {openDispatched:true,openResolved:true,guardPassed:true,owns:true,visible:false,epochMatch:false,snapshotCurrent:true,ownerAborted:false}
    logWarn('native-office', directCanaries[0], {stage:'controller_surface_open_current_final',errorCode:'unavailable',protectedPresent:true,
      openState:{...openState,path:directCanaries[2],body:directCanaries[0]}})
    await vi.waitFor(async () => {
      const body = await managedLogContent()
      expect(body).toContain('"openDispatched":true,"openResolved":true,"guardPassed":true,"owns":true,"visible":false,"epochMatch":false,"snapshotCurrent":true,"ownerAborted":false')
      for (const canary of directCanaries) expect(body).not.toContain(canary)
    })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    for (const invalidState of [{openDispatched:true},{...openState,epochMatch:directCanaries[0]}]) {
      publicConsoleWarn('native-office', directCanaries[0], {stage:'controller_surface_open_current_final',errorCode:'unavailable',protectedPresent:true,openState:invalidState})
    }
    for (const call of warn.mock.calls) expect(call.join(' ')).not.toContain('detail=')
  })

  it('rejects unknown and hostile Office details without retaining partial values', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const safe = { stage: 'controller_surface_open', errorCode: 'engine_unavailable', protectedPresent: false }
    for (const detail of [
      { ...safe, stage: directCanaries[0] }, { ...safe, errorCode: directCanaries[2] },
      { ...safe, protectedPresent: 'false' },
      Object.defineProperty({ ...safe }, 'stage', { get() { throw Error(directCanaries[0]) } })
    ]) expect(() => publicConsoleWarn('native-office', directCanaries[0], detail)).not.toThrow()
    for (const call of warn.mock.calls) {
      const text = call.join(' ')
      expect(text).not.toContain('detail=')
      for (const canary of directCanaries) expect(text).not.toContain(canary)
    }
  })
})


describe('native Office manual-begin diagnostic projection', () => {
  it('projects only complete manual-begin scalars through the fixed helper and drops nested and outer canaries', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-office-manual-begin-logger-'))
    configureLogger({ dir: testDir, enabled: true })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const manualBegin: NativeOfficeManualBeginFailureState = {
      schemaAccepted: true, outerCode: 'conflict', innerCode: 'forbidden',
      pendingSave: true, pendingRecovery: false, pendingManualCancel: true
    }
    const expected = { stage: 'controller_manual_begin', errorCode: 'forbidden', protectedPresent: false, ...manualBegin }
    const hostile = {
      stage: 'controller_manual_begin', errorCode: 'forbidden', protectedPresent: false,
      manualBegin: { ...manualBegin, path: directCanaries[2], response: directCanaries.join(' | '), stack: directCanaries[0] },
      body: directCanaries.join(' | '), source: directCanaries[0]
    }
    logNativeOfficeManualBeginFailure('forbidden', manualBegin)
    logWarn('native-office', directCanaries.join(' | '), hostile)
    publicConsoleWarn('native-office', directCanaries.join(' | '), hostile)
    const detail = JSON.stringify(expected)
    expect(warn).toHaveBeenCalledExactlyOnceWith(`[analytix] [main] event=main_warn detail=${detail}`)
    await vi.waitFor(async () => {
      const body = await managedLogContent()
      expect(body.split('\n').filter(line => line.includes(`detail=${detail}`))).toHaveLength(2)
      expect(body).not.toContain('"manualBegin"')
      for (const canary of directCanaries) expect(body).not.toContain(canary)
    })
    for (const call of warn.mock.calls) for (const canary of directCanaries) expect(call.join(' ')).not.toContain(canary)
  })

  it('retains null parse evidence, exact booleans and only the current outer and inner schema error-code unions', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const base: NativeOfficeManualBeginFailureState = {
      schemaAccepted: null, outerCode: null, innerCode: null,
      pendingSave: false, pendingRecovery: false, pendingManualCancel: false
    }
    const outerCodes: Array<NonNullable<NativeOfficeManualBeginFailureState['outerCode']>> = [
      'unavailable', 'invalid_request', 'identity_invalid', 'package_not_found',
      'conflict', 'disabled', 'adapter_unavailable', 'persistence_failure'
    ]
    const innerCodes: Array<NonNullable<NativeOfficeManualBeginFailureState['innerCode']>> = [
      'unavailable', 'unsupported_platform', 'invalid_request', 'session_invalid', 'capacity', 'forbidden',
      'not_text', 'too_large', 'conflict', 'operation_mismatch', 'operation_not_found', 'persistence_failure',
      'draft_stale', 'scope_invalid', 'proposal_invalid', 'projection_unavailable', 'protected_span_invalid'
    ]
    const states: NativeOfficeManualBeginFailureState[] = [
      base,
      { ...base, schemaAccepted: false, pendingSave: true },
      { ...base, schemaAccepted: true, pendingRecovery: true, pendingManualCancel: true },
      ...outerCodes.map(outerCode => ({ ...base, schemaAccepted: true, outerCode })),
      ...innerCodes.map(innerCode => ({ ...base, schemaAccepted: true, innerCode }))
    ]
    for (const manualBegin of states) {
      const detail = { stage: 'controller_manual_begin', errorCode: 'unknown', protectedPresent: false, manualBegin }
      publicConsoleWarn('native-office', '', detail)
      expect(warn).toHaveBeenLastCalledWith(`[analytix] [main] event=main_warn detail=${JSON.stringify({ stage: detail.stage, errorCode: detail.errorCode, protectedPresent: false, ...manualBegin })}`)
    }
    expect(warn).toHaveBeenCalledTimes(states.length)
  })

  it('rejects partial, untyped, unknown-code, protected and cross-stage manual-begin evidence without a partial diagnostic', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const manualBegin: NativeOfficeManualBeginFailureState = {
      schemaAccepted: null, outerCode: null, innerCode: null,
      pendingSave: false, pendingRecovery: false, pendingManualCancel: false
    }
    const base = { stage: 'controller_manual_begin', errorCode: 'unknown', protectedPresent: false }
    const invalidStates: unknown[] = [
      undefined, null, [], {}, Object.create(manualBegin),
      ...Object.keys(manualBegin).map(missing => Object.fromEntries(Object.entries(manualBegin).filter(([key]) => key !== missing))),
      { ...manualBegin, schemaAccepted: 0 }, { ...manualBegin, schemaAccepted: 'false' },
      { ...manualBegin, outerCode: 'save_failed' }, { ...manualBegin, outerCode: 'constructor' },
      { ...manualBegin, outerCode: directCanaries[0] }, { ...manualBegin, outerCode: false },
      { ...manualBegin, innerCode: 'identity_invalid' }, { ...manualBegin, innerCode: 'constructor' },
      { ...manualBegin, innerCode: directCanaries[2] }, { ...manualBegin, innerCode: 1 },
      { ...manualBegin, pendingSave: 'false' }, { ...manualBegin, pendingRecovery: 0 }, { ...manualBegin, pendingManualCancel: null },
      Object.defineProperty({ ...manualBegin }, 'schemaAccepted', { get() { throw Error(directCanaries[0]) } })
    ]
    const invalidDetails = [
      ...invalidStates.map(state => ({ ...base, manualBegin: state })),
      base,
      { ...base, errorCode: directCanaries[0], manualBegin },
      { ...base, protectedPresent: true, manualBegin },
      { ...base, protectedPresent: 'false', manualBegin },
      { ...base, stage: 'surface_open', manualBegin },
      { ...base, stage: 'controller_recovery', manualBegin: undefined },
      { ...base, manualBegin, openState: undefined }
    ]
    for (const detail of invalidDetails) expect(() => publicConsoleWarn('native-office', directCanaries.join(' | '), detail)).not.toThrow()
    // These six fields remain outside the general category detail allowlist.
    publicConsoleWarn('main', directCanaries[0], manualBegin)
    expect(warn).toHaveBeenCalledTimes(invalidDetails.length + 1)
    for (const call of warn.mock.calls) {
      expect(call).toEqual(['[analytix] [main] event=main_warn'])
      for (const canary of directCanaries) expect(call.join(' ')).not.toContain(canary)
    }
  })

})

describe('native Office save failure projection', () => {
  const state: NativeOfficeSaveFailureState = {
    phase: 'commit', outerSchemaAccepted: true, innerSchemaAccepted: true,
    outerCode: null, innerCode: 'invalid_request', receiptPresent: false, receiptStatus: null
  }

  it('persists only closed save facts and drops workbook, envelope, path, token and ID canaries', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-office-save-logger-'))
    configureLogger({ dir: testDir, enabled: true })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const privateFacts = {
      path: directCanaries[2], formula: '=PRIVATE_FORMULA_CANARY(A1:A2)', bytes: 'PRIVATE_BYTES_CANARY',
      envelope: { message: directCanaries.join(' | ') }, token: 'PRIVATE_SELECTION_TOKEN_CANARY',
      operationId: 'PRIVATE_OPERATION_ID_CANARY'
    }
    const expected = { stage: 'controller_save_failure', errorCode: 'save_failed', protectedPresent: false, ...state }
    const hostile = { ...expected, ...privateFacts, saveFailure: { ...state, ...privateFacts } }
    logNativeOfficeSaveFailure('save_failed', state)
    logWarn('native-office', directCanaries.join(' | '), hostile)
    publicConsoleWarn('native-office', directCanaries.join(' | '), hostile)
    const detail = JSON.stringify(expected)
    expect(warn).toHaveBeenCalledExactlyOnceWith(`[analytix] [main] event=main_warn detail=${detail}`)
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      expect(content.split('\n').filter(line => line.includes(`detail=${detail}`))).toHaveLength(2)
      expect(content).not.toContain('"saveFailure"')
      for (const canary of [...directCanaries, ...Object.values(privateFacts).filter(value => typeof value === 'string')]) expect(content).not.toContain(canary)
    })
  })

  it('retains native, parse, receipt and schema-code evidence without inventing response facts', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const states: NativeOfficeSaveFailureState[] = [
      { phase: 'replace', outerSchemaAccepted: null, innerSchemaAccepted: null, outerCode: null, innerCode: null, receiptPresent: null, receiptStatus: null },
      { ...state, phase: 'export', outerSchemaAccepted: false, innerSchemaAccepted: null, innerCode: null, receiptPresent: null },
      { ...state, outerCode: 'adapter_unavailable', innerSchemaAccepted: null, innerCode: null, receiptPresent: null },
      { ...state, innerSchemaAccepted: false, innerCode: null, receiptPresent: null },
      ...(['committed', 'conflict', 'unknown', 'pending'] as const).map(receiptStatus => ({ ...state, phase: 'ack' as const, innerCode: null, receiptPresent: true, receiptStatus })),
      ...(['invalid_request', 'not_text', 'too_large', 'persistence_failure'] as const).map(innerCode => ({ ...state, innerCode }))
    ]
    for (const saveFailure of states) {
      publicConsoleWarn('native-office', '', { stage: 'controller_save_failure', errorCode: 'unknown', protectedPresent: false, saveFailure })
      expect(warn).toHaveBeenLastCalledWith(`[analytix] [main] event=main_warn detail=${JSON.stringify({ stage: 'controller_save_failure', errorCode: 'unknown', protectedPresent: false, ...saveFailure })}`)
    }
  })

  it('rejects partial, hostile, unknown-code, protected and cross-stage save facts as a whole', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const base = { stage: 'controller_save_failure', errorCode: 'unknown', protectedPresent: false }
    const invalid = [
      undefined, null, [], {}, Object.create(state),
      ...Object.keys(state).map(missing => Object.fromEntries(Object.entries(state).filter(([key]) => key !== missing))),
      { ...state, phase: directCanaries[0] }, { ...state, outerSchemaAccepted: 'true' }, { ...state, innerSchemaAccepted: 0 },
      { ...state, outerCode: 'constructor' }, { ...state, innerCode: directCanaries[0] },
      { ...state, receiptPresent: 'false' }, { ...state, receiptStatus: 'saved' }, { ...state, receiptStatus: 'committed' },
      Object.defineProperty({ ...state }, 'innerCode', { get() { throw Error(directCanaries[0]) } })
    ]
    const details = [
      ...invalid.map(saveFailure => ({ ...base, saveFailure })), base,
      { ...base, saveFailure: state, protectedPresent: true },
      { ...base, saveFailure: state, stage: 'controller_recovery' },
      { ...base, saveFailure: state, manualBegin: undefined }, { ...base, saveFailure: state, openState: undefined }
    ]
    for (const detail of details) expect(() => publicConsoleWarn('native-office', directCanaries[0], detail)).not.toThrow()
    publicConsoleWarn('main', directCanaries[0], state)
    for (const call of warn.mock.calls) expect(call).toEqual(['[analytix] [main] event=main_warn'])
  })
})

describe('Core native manual-begin phase projection', () => {
  const frame: NativeManualBeginPhaseV1 = {
    version: 1, phase: 'prepare', errorKind: 'unavailable',
    manualServicePresent: true, captureBound: true, captureEntered: true, captureReused: false,
    baselineEntered: true, baselineOpenOK: true, sessionMatch: true, revisionMatch: true,
    captureAcquired: true, prepareEntered: true, prepareOK: false
  }

  it('persists exactly the frozen Core phase fields through the managed logger', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-core-manual-phase-logger-'))
    configureLogger({ dir: testDir, enabled: true })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const expected = { stage: 'core_manual_begin_phase', ...frame }
    logNativeOfficeManualBeginPhaseV1(frame)
    publicConsoleWarn('native-office', directCanaries.join(' | '), { stage: expected.stage, phaseFrame: frame })
    expect(warn).toHaveBeenCalledExactlyOnceWith(`[analytix] [main] event=main_warn detail=${JSON.stringify(expected)}`)
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      expect(content).toContain(`detail=${JSON.stringify(expected)}`)
      expect(content).not.toContain('"phaseFrame"')
      for (const canary of directCanaries) expect(content).not.toContain(canary)
    })
  })

  it('rejects incomplete, extended or mistyped frames as a whole without leaking payload or parse details', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const invalidFrames: unknown[] = [
      undefined, null, [], {}, Object.create(frame),
      ...Object.keys(frame).map(missing => Object.fromEntries(Object.entries(frame).filter(([key]) => key !== missing))),
      { ...frame, version: '1' }, { ...frame, version: 2 },
      { ...frame, phase: directCanaries[2] }, { ...frame, phase: 'constructor' },
      { ...frame, errorKind: directCanaries[0] }, { ...frame, errorKind: 'constructor' },
      { ...frame, prepareEntered: 'true' }, { ...frame, prepareOK: 0 },
      { ...frame, body: directCanaries.join(' | ') }, { ...frame, path: directCanaries[2] },
      Object.defineProperty({ ...frame }, 'phase', { get() { throw Error(directCanaries[0]) } })
    ]
    const invalidDetails = [
      ...invalidFrames.map(phaseFrame => ({ stage: 'core_manual_begin_phase', phaseFrame })),
      { stage: 'core_manual_begin_phase' },
      { stage: 'core_manual_begin_phase', phaseFrame: frame, path: directCanaries[2] },
      { stage: 'core_manual_begin_phase', phaseFrame: frame, manualBegin: {} },
      { stage: 'controller_manual_begin', errorCode: 'unavailable', protectedPresent: false, phaseFrame: frame },
      { stage: directCanaries[0], phaseFrame: frame }
    ]
    for (const detail of invalidDetails) {
      expect(() => publicConsoleWarn('native-office', directCanaries.join(' | '), detail)).not.toThrow()
    }
    expect(warn).toHaveBeenCalledTimes(invalidDetails.length)
    for (const call of warn.mock.calls) expect(call).toEqual(['[analytix] [main] event=main_warn'])
  })
})


describe('SSE rejection diagnostic projection', () => {
  const ackReasons = [
    'invalid_ack_request', 'stream_unavailable', 'owner_mismatch', 'undelivered_sequence',
    'accepted_final_receipt_mismatch', 'accepted_final_sequence_invalid',
    'unexpected_accepted_final_receipt'
  ] as const
  const rendererCodes = [
    'renderer_sse_event_active', 'renderer_sse_event_aborted', 'renderer_sse_event_closed',
    'renderer_sse_ack_false_active', 'renderer_sse_ack_false_aborted', 'renderer_sse_ack_false_closed',
    'renderer_sse_ack_transport_active', 'renderer_sse_ack_transport_aborted', 'renderer_sse_ack_transport_closed'
  ] as const

  it('persists only every public SSE reason and all seven ACK reasons in disk and console logs', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-sse-reasons-'))
    configureLogger({ dir: testDir, enabled: true })
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const expected = [
      ...PUBLIC_RUNTIME_SSE_REJECTION_REASON_CODES.map(reasonCode => ({ code: 'sse_event_rejected', reasonCode })),
      ...ackReasons.map(reasonCode => ({ code: 'sse_ack_rejected', reasonCode }))
    ]
    for (const entry of expected) {
      const detail = { ...entry, diagnostic: { body: directCanaries[0] }, payload: directCanaries,
        streamId: directCanaries[0], threadId: directCanaries[1], path: directCanaries[2],
        locator: directCanaries[12], seq: 99, resultCount: 99, retryable: true, sha256 }
      logError('sse', directCanaries.join(' | '), detail)
      publicConsoleError('sse', directCanaries.join(' | '), detail)
    }
    const encoded = expected.map(entry => JSON.stringify(entry))
    expect(error.mock.calls.map(([line]) => line)).toEqual(encoded.map(detail =>
      `[analytix] [main] event=main_error detail=${detail}`))
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      const lines = content.trim().split('\n')
      expect(lines).toHaveLength(expected.length)
      const details = lines.map(line => {
        expect(line).toMatch(/\[ERROR\] \[main\] event=main_error detail=/)
        return line.slice(line.indexOf(' detail=') + ' detail='.length)
      })
      expect(details.sort()).toEqual([...encoded].sort())
      for (const canary of directCanaries) expect(content).not.toContain(canary)
    })
  })

  it('rejects unknown, non-string, cross-code and hostile reasons without partial disk or console details', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-sse-invalid-reasons-'))
    configureLogger({ dir: testDir, enabled: true })
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const stringifyReason = vi.fn(() => { throw new Error(directCanaries[0]) })
    const invalid: unknown[] = [
      { code: 'sse_event_rejected', reasonCode: directCanaries[0], resultCount: 99 },
      { code: 'sse_ack_rejected', reasonCode: directCanaries[2], resultCount: 99 },
      { code: 'sse_event_rejected', reasonCode: 'owner_mismatch', sha256 },
      { code: 'sse_ack_rejected', reasonCode: 'event_thread_mismatch', sha256 },
      { code: 'sse_event_rejected', reasonCode: 'EVENT_THREAD_MISMATCH', retryable: true },
      { code: 'sse_ack_rejected', reasonCode: 7 },
      { code: 'sse_event_rejected', reasonCode: null },
      { code: 'sse_ack_rejected' },
      { code: 'sse_event_rejected', reasonCode: { toString: stringifyReason } },
      Object.defineProperty({ reasonCode: 'event_thread_mismatch', resultCount: 99 }, 'code', {
        get() { throw new Error(directCanaries[0]) }
      }),
      Object.defineProperty({ code: 'sse_ack_rejected', resultCount: 99 }, 'reasonCode', {
        get() { throw new Error(directCanaries[2]) }
      })
    ]
    for (const detail of invalid) {
      expect(() => logError('sse', directCanaries[0], detail)).not.toThrow()
      expect(() => publicConsoleError('sse', directCanaries[0], detail)).not.toThrow()
    }
    expect(stringifyReason).not.toHaveBeenCalled()
    expect(error.mock.calls.map(([line]) => line)).toEqual(invalid.map(() =>
      '[analytix] [main] event=main_error'))
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      const lines = content.trim().split('\n')
      expect(lines).toHaveLength(invalid.length)
      for (const line of lines) expect(line).toMatch(/^\[[^\n]+\] \[ERROR\] \[main\] event=main_error$/)
      expect(content).not.toContain('detail=')
      for (const canary of directCanaries) expect(content).not.toContain(canary)
    })
  })

  it('does not read or enumerate hostile extra fields for a valid closed SSE reason', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-sse-hostile-extras-'))
    configureLogger({ dir: testDir, enabled: true })
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const readExtra = vi.fn(() => { throw new Error(directCanaries[0]) })
    const enumerate = vi.fn((): never => { throw new Error(directCanaries[2]) })
    const detail = { code: 'sse_event_rejected', reasonCode: 'event_thread_mismatch' }
    for (const key of ['diagnostic', 'payload', 'id', 'path', 'locator', 'seq', 'resultCount', 'sha256']) {
      Object.defineProperty(detail, key, { enumerable: true, get: readExtra })
    }
    const hostile = new Proxy(detail, { ownKeys: enumerate })
    logError('sse', directCanaries[0], hostile)
    publicConsoleError('sse', directCanaries[0], hostile)
    const expected = '{"code":"sse_event_rejected","reasonCode":"event_thread_mismatch"}'
    expect(error.mock.calls.map(([line]) => line)).toEqual([
      `[analytix] [main] event=main_error detail=${expected}`
    ])
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      expect(content.trim().split('\n')).toHaveLength(1)
      expect(content).toContain(`detail=${expected}`)
      for (const canary of directCanaries) expect(content).not.toContain(canary)
    })
    expect(readExtra).not.toHaveBeenCalled()
    expect(enumerate).not.toHaveBeenCalled()
  })

  it('retains all nine fixed renderer rejection event codes without caller text', async () => {
    testDir = await mkdtemp(join(tmpdir(), 'analytix-renderer-sse-codes-'))
    configureLogger({ dir: testDir, enabled: true })
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    for (const code of rendererCodes) {
      logError('renderer-ipc', directCanaries[0], { code })
      publicConsoleError('renderer-ipc', directCanaries[0], { code })
    }
    expect(error.mock.calls.map(([line]) => line)).toEqual(rendererCodes.map(code =>
      `[analytix] [renderer] event=${code}`))
    await vi.waitFor(async () => {
      const content = await managedLogContent()
      expect(content.trim().split('\n')).toHaveLength(rendererCodes.length)
      for (const code of rendererCodes) expect(content).toContain(`[renderer] event=${code}\n`)
      expect(content).not.toContain('detail=')
      for (const canary of directCanaries) expect(content).not.toContain(canary)
    })
  })
})

describe('closed private question read failures',()=>{
  it('records only the fixed read stage and bounded HTTP status',async()=>{
    testDir=await mkdtemp(join(tmpdir(),'analytix-question-logger-'))
    configureLogger({dir:testDir,enabled:true})
    logWarn('user-input-local-read',directCanaries[0],{stage:'response_http',statusCode:403,selector:directCanaries[2],body:directCanaries[0]})
    await vi.waitFor(async()=>{
      const body=await managedLogContent()
      expect(body).toContain('detail={"stage":"response_http","statusCode":403}')
      for(const canary of directCanaries) expect(body).not.toContain(canary)
    })
  })
  it('rejects unknown stages, non-HTTP status and hostile fields without retaining partial values',()=>{
    const warn=vi.spyOn(console,'warn').mockImplementation(()=>undefined)
    for(const detail of [{stage:directCanaries[0],statusCode:403},{stage:'response_http',statusCode:directCanaries[2]},
      {stage:'response_http',statusCode:99},{stage:'response_http',statusCode:600},
      Object.defineProperty({statusCode:403},'stage',{get(){throw Error(directCanaries[0])}})]) {
      expect(()=>publicConsoleWarn('user-input-local-read',directCanaries[0],detail)).not.toThrow()
    }
    for(const call of warn.mock.calls) {
      const text=call.join(' ')
      expect(text).not.toContain('detail=')
      for(const canary of directCanaries) expect(text).not.toContain(canary)
    }
  })
})
