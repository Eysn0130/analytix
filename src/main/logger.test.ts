import { mkdtemp, readFile, readdir, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import loggerSource from './logger.ts?raw'
import {
  appendManagedChildLogRecord,
  configureLogger,
  logError,
  logInfo,
  logWarn,
  publicConsoleError,
  publicConsoleInfo,
  publicConsoleWarn
} from './logger'

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
