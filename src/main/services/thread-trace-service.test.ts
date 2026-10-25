import { afterEach, describe, expect, it } from 'vitest'
import { mkdtemp, readFile, readdir, stat, truncate, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { THREAD_TRACE_EVENT_NAMES } from '../../shared/thread-trace'
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

  it('persists closed provider loop failures beside terminal timing in the owned trace file', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData()
    const ref = `ref-${'a'.repeat(64)}`
    const turnRef = `ref-${'b'.repeat(64)}`
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 3)
    const causes = [
      'provider_privacy_authority_unavailable', 'uninspectable_provider_part',
      'provider_schema_contains_pii', 'ordinary_provider_contains_case_effect',
      'provider_logical_effect_invalid', 'context_canceled', 'context_deadline_exceeded',
      'turn_security_authority_unavailable', 'turn_security_identity_mismatch',
      'turn_security_workspace_mismatch', 'turn_security_case_binding_mismatch',
      'turn_security_dataset_snapshot_mismatch', 'turn_security_risk_policy_mismatch',
      'turn_security_context_invalid', 'source_probe_unavailable', 'tool_source_unavailable',
      'unknown'
    ]
    const labels = [
      { stage: 'acquire', cause: 'context_canceled' },
      { stage: 'privacy_projection', cause: 'uninspectable_provider_part' },
      { stage: 'unknown', cause: 'turn_security_context_invalid' },
      ...causes.map((cause) => ({ stage: 'prepare', cause }))
    ]
    for (const data of labels) {
      const line = `[analytix-thread-trace] ${JSON.stringify({ name: 'thread.provider.loop_failure', threadId: ref, turnId: turnRef, data: { pid: 42, ...data } })}\n`
      receiver.write(line.slice(0, 45)); receiver.write(line.slice(45))
    }
    receiver.write(`[analytix-thread-trace] ${JSON.stringify({ name: 'thread.terminal.core_committed', threadId: ref, turnId: turnRef,
      data: { pid: 42, timeOrigin: 1000, publicationCommittedMs: 30 } })}\n`)
    await flushThreadTraceEventsForTests(userData)
    expect(await readdir(join(userData, 'traces'))).toEqual([`thread-${ref}.jsonl`])
    const rows = (await readFile(join(userData, 'traces', `thread-${ref}.jsonl`), 'utf8')).trim().split('\n').map((row) => JSON.parse(row))
    expect(rows).toHaveLength(labels.length + 1)
    expect(rows.slice(0, labels.length)).toEqual(labels.map((data) => ({
      name: 'thread.provider.loop_failure', timestamp: expect.any(Number), threadId: ref, turnId: turnRef,
      data: { pid: 42, ...data, runtimeGeneration: 3, mainReceivedMonotonicMs: expect.any(Number), mainTimeOrigin: expect.any(Number) }
    })))
    expect(rows.at(-1)).toMatchObject({ name: 'thread.terminal.core_committed', threadId: ref, turnId: turnRef,
      data: { pid: 42, timeOrigin: 1000, publicationCommittedMs: 30, runtimeGeneration: 3 } })
    expect(THREAD_TRACE_EVENT_NAMES).not.toContain('thread.provider.loop_failure')
    receiver.close()
  })

  it('rejects unowned or nonclosed provider failure records without retaining private fields', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData()
    const event = { name: 'thread.provider.loop_failure', threadId: `ref-${'a'.repeat(64)}`, turnId: `ref-${'b'.repeat(64)}`,
      data: { pid: 42, stage: 'prepare', cause: 'unknown' } }
    const secret = 'SYNTHETIC_PRIVATE_ERROR_REQUEST_CONFIG_PATH_SENTINEL'
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 3)
    const rejected = [
      { ...event, error: secret }, { ...event, threadId: secret },
      { ...event, turnId: `ref-${'A'.repeat(64)}` }, { ...event, turnId: `ref-${'b'.repeat(63)}` },
      { ...event, data: { ...event.data, pid: 43 } },
      { ...event, data: { stage: 'prepare', cause: 'unknown' } },
      { ...event, data: { pid: 42, cause: 'unknown' } },
      { ...event, data: { pid: 42, stage: 'prepare' } },
      { ...event, data: { ...event.data, stage: secret } },
      { ...event, data: { ...event.data, stage: 'prepare ' } },
      { ...event, data: { ...event.data, cause: secret } },
      { ...event, data: { ...event.data, cause: 'source_probe_unavailable ' } },
      { ...event, data: { ...event.data, stage: true } },
      { ...event, data: { ...event.data, cause: { error: secret } } },
      { ...event, data: { pid: 42, stage: 'privacy_projection', cause: 'unknown' } },
      { ...event, data: { pid: 42, stage: 'unknown', cause: 'provider_schema_contains_pii' } },
      { ...event, data: null }, { ...event, data: [] },
      ...['error', 'rawError', 'Details', 'request', 'path', 'provider', 'config', '__proto__'].map((key) => ({ ...event, data: { ...event.data, [key]: secret } }))
    ]
    for (const value of rejected) receiver.write(`[analytix-thread-trace] ${JSON.stringify(value)}\n`)
    for (const generation of [0, -1, 1.5, Infinity]) {
      const unowned = createRuntimePublicationTraceReceiver(userData, 42, generation)
      unowned.write(`[analytix-thread-trace] ${JSON.stringify(event)}\n`)
      unowned.close()
    }
    receiver.write(`[analytix-thread-trace] {"name":"thread.provider.loop_failure",${secret}\n`)
    await flushThreadTraceEventsForTests(userData)
    await expect(readdir(join(userData, 'traces'))).rejects.toThrow()
    receiver.close()
  })

  it('keeps provider failure capture opt in and recovers after oversized or incomplete lines', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '0'
    const userData = await tempUserData()
    const event = { name: 'thread.provider.loop_failure', threadId: `ref-${'a'.repeat(64)}`, turnId: `ref-${'b'.repeat(64)}`,
      data: { pid: 42, stage: 'prepare', cause: 'unknown' } }
    const line = `[analytix-thread-trace] ${JSON.stringify(event)}\n`
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 3)
    receiver.write(line)
    await flushThreadTraceEventsForTests(userData)
    await expect(readdir(join(userData, 'traces'))).rejects.toThrow()
    process.env.ANALYTIX_THREAD_TRACE = '1'
    receiver.write('SYNTHETIC_PRIVATE_STDERR'.repeat(500)); receiver.write('\n')
    receiver.write(line.slice(0, 55)); receiver.close(); receiver.write(line.slice(55))
    await flushThreadTraceEventsForTests(userData)
    await expect(readdir(join(userData, 'traces'))).rejects.toThrow()
    receiver.write(line)
    await flushThreadTraceEventsForTests(userData)
    const body = await readFile(join(userData, 'traces', `thread-${event.threadId}.jsonl`), 'utf8')
    expect(body).not.toContain('SYNTHETIC_PRIVATE_STDERR')
    expect(body.trim().split('\n')).toHaveLength(1)
    expect(JSON.parse(body).data).toMatchObject(event.data)
    receiver.close()
  })

  it('shares the existing bounded queue for provider failure records', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData()
    const ref = `ref-${'a'.repeat(64)}`
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 3)
    const line = `[analytix-thread-trace] ${JSON.stringify({ name: 'thread.provider.loop_failure', threadId: ref, turnId: `ref-${'b'.repeat(64)}`,
      data: { pid: 42, stage: 'prepare', cause: 'unknown' } })}\n`
    for (let index = 0; index < 257; index++) receiver.write(line)
    await flushThreadTraceEventsForTests(userData)
    const rows = (await readFile(join(userData, 'traces', `thread-${ref}.jsonl`), 'utf8')).trim().split('\n')
    expect(rows).toHaveLength(256)
    receiver.close()
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

describe('owned Core generation failure diagnostics', () => {
  const previousEnv = process.env.ANALYTIX_THREAD_TRACE
  afterEach(() => {
    resetThreadTraceWriterForTests()
    if (previousEnv === undefined) delete process.env.ANALYTIX_THREAD_TRACE
    else process.env.ANALYTIX_THREAD_TRACE = previousEnv
  })

  it('persists only valid stage/cause pairs and rejects foreign or private metadata', async () => {
    process.env.ANALYTIX_THREAD_TRACE = '1'
    const userData = await tempUserData(), ref = `ref-${'a'.repeat(64)}`
    const receiver = createRuntimePublicationTraceReceiver(userData, 42, 3)
    const pairs = [
      ['checkpoint_settlement', 'unavailable'], ['checkpoint_settlement', 'incomplete'],
      ['checkpoint_record', 'unavailable'],
      ...['host_preconsume', 'host_postconsume'].flatMap(stage =>
        ['context_canceled', 'context_deadline_exceeded', 'identity_invalid', 'snapshot_unavailable'].map(cause => [stage, cause])),
      ...['validation_invalid', 'target_unavailable', 'codec_invalid', 'checkpoint_unavailable', 'context_canceled', 'other_unavailable'].map(cause => ['generation_output', cause])
    ]
    const event = { name: 'thread.tool.generation_failure', threadId: ref, turnId: `ref-${'b'.repeat(64)}`,
      data: { pid: 42, stage: 'checkpoint_record', cause: 'unavailable' } }
    const send = (value: unknown) => receiver.write(`[analytix-thread-trace] ${JSON.stringify(value)}\n`)
    for (const [stage, cause] of pairs) send({ ...event, data: { pid: 42, stage, cause } })
    const secret = 'PRIVATE_GENERATION_ERROR_PATH_BODY_SENTINEL'
    for (const rejected of [
      { ...event, error: secret }, { ...event, threadId: secret },
      { ...event, data: { ...event.data, pid: 43 } },
      { ...event, data: { ...event.data, path: secret } },
      { ...event, data: { ...event.data, cause: 'incomplete' } },
      { ...event, data: { ...event.data, stage: '__proto__' } },
      { ...event, data: { ...event.data, stage: secret } },
      { ...event, data: { ...event.data, cause: secret } }
    ]) send(rejected)
    await flushThreadTraceEventsForTests(userData)
    const body = await readFile(join(userData, 'traces', `thread-${ref}.jsonl`), 'utf8')
    expect(body).not.toContain(secret)
    expect(body.trim().split('\n').map(row => JSON.parse(row).data)).toEqual(pairs.map(([stage, cause]) => ({
      pid: 42, stage, cause, runtimeGeneration: 3,
      mainReceivedMonotonicMs: expect.any(Number), mainTimeOrigin: expect.any(Number)
    })))
    expect(THREAD_TRACE_EVENT_NAMES).not.toContain('thread.tool.generation_failure')
    receiver.close()
  })
})
