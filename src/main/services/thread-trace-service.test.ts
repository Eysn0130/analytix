import { afterEach, describe, expect, it } from 'vitest'
import { mkdtemp, readFile, readdir, stat, truncate, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import {
  appendThreadTraceEvent,
  createRuntimePublicationTraceReceiver,
  flushThreadTraceEventsForTests,
  pruneThreadTraces,
  resetThreadTraceWriterForTests
} from './thread-trace-service'

async function tempUserData(): Promise<string> {
  return mkdtemp(join(tmpdir(), 'analytix-trace-test-'))
}

describe('thread trace service', () => {
  const previousEnv = process.env.ANALYTIX_THREAD_TRACE

  afterEach(() => {
    resetThreadTraceWriterForTests()
    if (previousEnv === undefined) {
      delete process.env.ANALYTIX_THREAD_TRACE
    } else {
      process.env.ANALYTIX_THREAD_TRACE = previousEnv
    }
  })

  it('joins only bounded owned Core observations to the existing trace file', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData()
    const { createHash } = await import('node:crypto')
    const threadId = 'synthetic-thread'
    const ref = `ref-${createHash('sha256').update(threadId).digest('hex')}`
    const event = { name: 'thread.terminal.core_committed', threadId: ref, turnId: `ref-${'b'.repeat(64)}`,
      data: { pid: 42, timeOrigin: 1000, publicationCommittedMs: 30, publicationGapMs: 10, logicalCallHmac: 'c'.repeat(64), physicalAttempt: 1 } }
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 3)
    const line = `[analytix-thread-trace] ${JSON.stringify(event)}\n`
    receiver.write(line.slice(0, 35)); receiver.write(line.slice(35))
    receiver.write(`[analytix-thread-trace] ${JSON.stringify({ ...event, data: { ...event.data, body: 'PRIVATE_BODY' } })}\n`)
    receiver.write(`[analytix-thread-trace] ${JSON.stringify({ ...event, data: { ...event.data, pid: 43 } })}\n`)
    receiver.write('private stderr must not be forwarded\n')
    receiver.write('x'.repeat(9000)); receiver.write('\n')
    await appendThreadTraceEvent(userData, { name: 'thread.terminal.verified', timestamp: 1001, threadId, data: { lastSeq: 8 } })
    await flushThreadTraceEventsForTests(userData)
    const files = await readdir(join(userData, 'traces'))
    expect(files).toEqual([`thread-${ref}.jsonl`])
    const rows = (await readFile(join(userData, 'traces', files[0]), 'utf8')).trim().split('\n').map((row) => JSON.parse(row))
    expect(rows).toHaveLength(2)
    expect(rows[0]).toMatchObject({ ...event, data: { ...event.data, runtimeGeneration: 3 } })
    expect(rows[0].data.mainReceivedMonotonicMs).toBeGreaterThanOrEqual(0)
    expect(rows[1].name).toBe('thread.terminal.verified')
    receiver.close()
  })

  it('does not collect Core stderr when tracing is disabled or retain incomplete lines after close', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '0'
    const userData = await tempUserData()
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 1)
    receiver.write('[analytix-thread-trace] ')
    process.env.ANALYTIX_THREAD_TRACE = '1'
    receiver.write('{"name":"thread.terminal.core_committed"')
    receiver.close()
    receiver.write('}\n')
    await flushThreadTraceEventsForTests(userData)
    await expect(readdir(join(userData, 'traces'))).rejects.toThrow()
  })

  it('does not write traces by default', async () => {
    const userData = await tempUserData()

    const result = await appendThreadTraceEvent(userData, {
      name: 'thread.delta.buffered',
      timestamp: 1,
      threadId: 'thr_default',
      data: { deltas: 1 }
    })
    await flushThreadTraceEventsForTests(userData)

    expect(result.ok).toBe(false)
    await expect(readdir(join(userData, 'traces'))).rejects.toThrow()
  })

  it('projects renderer-controlled identifiers and data before the durable write', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData()
    const hostileThreadId =
      'HOSTILE_THREAD_TRACE_CANARY_/Users/private/synthetic/case-42_PII_SYNTHETIC-ACCOUNT-622202'
    const hostileDataKey =
      'private_path_Users_private_synthetic_case_42_PII_ACCOUNT_622202'

    const firstResult = await appendThreadTraceEvent(userData, {
      name: 'thread.delta.buffered',
      timestamp: 101,
      threadId: hostileThreadId,
      data: {
        deltas: 1,
        renderer_delta_buffered_at: 102,
        [hostileDataKey]: true
      }
    })
    const secondResult = await appendThreadTraceEvent(userData, {
      name: 'thread.projection.reduced',
      timestamp: 103,
      threadId: hostileThreadId,
      data: { blocks: null, rows: 2, live: false, deltas: 999 }
    })
    await flushThreadTraceEventsForTests(userData)

    expect(firstResult.ok).toBe(true)
    expect(secondResult.ok).toBe(true)
    if (!firstResult.ok || !secondResult.ok) throw new Error('thread trace write failed')
    expect(Object.keys(firstResult).sort()).toEqual(['ok', 'path'])
    expect(firstResult.path).toBe(secondResult.path)
    expect(firstResult.path).toMatch(/^traces\/thread-ref-[a-f0-9]{64}\.jsonl$/)
    expect(firstResult.path).not.toContain(userData)
    expect(firstResult.path).not.toContain(hostileThreadId)

    const traceDir = join(userData, 'traces')
    const traceFiles = await readdir(traceDir)
    expect(traceFiles).toHaveLength(1)
    expect(traceFiles[0]).toMatch(/^thread-ref-[a-f0-9]{64}\.jsonl$/)
    const body = await readFile(join(traceDir, traceFiles[0]), 'utf8')
    expect(body).not.toContain(hostileThreadId)
    expect(body).not.toContain(hostileDataKey)
    expect(body).not.toContain('SYNTHETIC-ACCOUNT-622202')

    const records = body.trim().split('\n').map((line) => JSON.parse(line) as Record<string, unknown>)
    expect(records).toHaveLength(2)
    expect(records[0]).toEqual({
      name: 'thread.delta.buffered',
      timestamp: 101,
      threadId: expect.stringMatching(/^ref-[a-f0-9]{64}$/),
      data: { deltas: 1, renderer_delta_buffered_at: 102 }
    })
    expect(records[1]).toEqual({
      name: 'thread.projection.reduced',
      timestamp: 103,
      threadId: records[0]?.threadId,
      data: { blocks: null, rows: 2, live: false }
    })
  })

  it('batches enabled traces and prunes oldest files over the global cap', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData()

    const result = await appendThreadTraceEvent(userData, {
      name: 'thread.delta.buffered',
      timestamp: 1,
      threadId: 'thr_enabled',
      data: { deltas: 1 }
    })
    await flushThreadTraceEventsForTests(userData)

    expect(result.ok).toBe(true)
    if (!result.ok) throw new Error('thread trace write failed')
    const traceDir = join(userData, 'traces')
    const tracePath = join(userData, result.path)
    expect((await stat(tracePath)).size).toBeGreaterThan(0)

    await truncate(tracePath, 20 * 1024 * 1024)
    await appendThreadTraceEvent(userData, {
      name: 'thread.delta.buffered',
      timestamp: 2,
      threadId: 'thr_enabled',
      data: { deltas: 2 }
    })
    await flushThreadTraceEventsForTests(userData)
    const rotatedTracePath = tracePath.replace(/\.jsonl$/, '.1.jsonl')
    expect((await stat(rotatedTracePath)).size).toBe(20 * 1024 * 1024)
    expect((await stat(tracePath)).size).toBeGreaterThan(0)

    const oldTrace = join(traceDir, 'old.jsonl')
    await writeFile(oldTrace, '')
    await truncate(oldTrace, 513 * 1024 * 1024)
    await pruneThreadTraces(userData)

    await expect(stat(oldTrace)).rejects.toThrow()
  })
})
