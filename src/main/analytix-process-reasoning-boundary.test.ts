import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import analytixProcessSource from './analytix-process.ts?raw'
import type { ManagedChildLogRecord } from './logger'

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getAppPath: () => '/tmp/analytix-test-app',
    getPath: () => '/tmp/analytix-test-user-data'
  }
}))

describe('Analytix child process log quarantine', () => {
  it('persists only fixed stream diagnostics and closed lifecycle events', async () => {
    const { createAnalytixChildLogCapture } = await import('./analytix-process')
    const records: ManagedChildLogRecord[] = []
    const capture = createAnalytixChildLogCapture(4242, (record) => {
      records.push(record)
    })
    const stdoutParts = [
      Buffer.from('MARKER_FREE_PRIVATE_STDOUT_SENTINEL:'),
      Buffer.from('案件账号=6222020200000000000\n', 'utf8')
    ]
    const stderrParts = [
      'MARKER_FREE_PRIVATE_STDERR_SENTINEL:',
      'raw provider failure detail'
    ]
    const lifecycleError = 'MARKER_FREE_PRIVATE_ERROR_MESSAGE_SENTINEL'
    const privateDataDir = '/private/cases/case-a/runtime-data'

    for (const part of stdoutParts) capture.captureStdout(part)
    for (const part of stderrParts) capture.captureStderr(part)
    capture.logLifecycle({
      code: 'ANALYTIX_CHILD_SPAWNED',
      port: 8788,
      dataDir: privateDataDir
    } as Parameters<typeof capture.logLifecycle>[0])
    capture.logLifecycle({
      code: 'ANALYTIX_CHILD_PROCESS_ERROR',
      message: lifecycleError,
      stdout: stdoutParts.join(''),
      stderr: stderrParts.join('')
    } as Parameters<typeof capture.logLifecycle>[0])
    capture.logLifecycle({
      code: 'ANALYTIX_CHILD_STARTUP_FAILED',
      message: lifecycleError
    } as Parameters<typeof capture.logLifecycle>[0])
    capture.logLifecycle({
      code: 'ANALYTIX_CHILD_EXITED',
      exitCode: 1,
      signaled: false
    })

    await capture.close()
    await capture.close()

    const persisted = JSON.stringify(records)
    const stdoutBytes = Buffer.concat(stdoutParts)
    const stderrBytes = Buffer.from(stderrParts.join(''), 'utf8')
    const stdoutRecord = records.find((record) => record.code === 'ANALYTIX_CHILD_STDOUT_CAPTURED')
    const stderrRecord = records.find((record) => record.code === 'ANALYTIX_CHILD_STDERR_CAPTURED')
    const lifecycleRecords = records.filter((record) => !record.code.endsWith('_CAPTURED'))

    expect(stdoutRecord).toEqual({
      code: 'ANALYTIX_CHILD_STDOUT_CAPTURED',
      bytes: stdoutBytes.byteLength,
      sha256: createHash('sha256').update(stdoutBytes).digest('hex')
    })
    expect(stderrRecord).toEqual({
      code: 'ANALYTIX_CHILD_STDERR_CAPTURED',
      bytes: stderrBytes.byteLength,
      sha256: createHash('sha256').update(stderrBytes).digest('hex')
    })
    expect(records.filter((record) => record.code === 'ANALYTIX_CHILD_STDOUT_CAPTURED')).toHaveLength(1)
    expect(records.filter((record) => record.code === 'ANALYTIX_CHILD_STDERR_CAPTURED')).toHaveLength(1)
    expect(lifecycleRecords).toEqual([
      { code: 'ANALYTIX_CHILD_SPAWNED', port: 8788 },
      { code: 'ANALYTIX_CHILD_PROCESS_ERROR' },
      { code: 'ANALYTIX_CHILD_STARTUP_FAILED' },
      { code: 'ANALYTIX_CHILD_EXITED', exitCode: 1, signaled: false }
    ])
    expect(persisted).not.toContain('MARKER_FREE_PRIVATE')
    expect(persisted).not.toContain('6222020200000000000')
    expect(persisted).not.toContain('raw provider failure detail')
    expect(persisted).not.toContain(privateDataDir)
    expect(persisted).not.toContain(lifecycleError)
  })

  it('routes the production child call graph only through the typed managed-log owner', () => {
    expect(analytixProcessSource).not.toContain('appendManagedLogLine')
    expect(analytixProcessSource).not.toContain('formatAnalytixLogLine')
    expect(analytixProcessSource).toContain('appendManagedChildLogRecord')
    expect(analytixProcessSource).toContain('writer: AnalytixChildLogWriter = appendManagedChildLogRecord')
  })
})
