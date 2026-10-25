import { readFileSync } from 'node:fs'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { subscribeRuntimeThreadEvents } from '../claw-runtime-helpers'
import { startRuntimeThreadSubscription } from './runtime-thread-subscription'
import { isStrictPublicRuntimeSseIpcPayload } from '../../shared/public-runtime-sse'

const threadId = 'synthetic-subscription-thread'
const timestamp = '2026-01-01T00:00:00.000Z'
const heartbeat = (seq: number) => ({ kind: 'heartbeat', seq, timestamp, threadId })
const frame = (event: ReturnType<typeof heartbeat>, newline = '\n') =>
  [`id: ${event.seq}`, `event: ${event.kind}`, `data: ${JSON.stringify(event)}`, '', ''].join(newline)
const revoked = {
  schemaVersion: 1, kind: 'public_projection_revoked', threadId,
  historyAuthority: 'case_boundary_only_v1', code: 'case_public_authority_unavailable',
  action: 'purge_case_projection', terminal: true
}
const release: Array<() => void> = []
const handles: Array<{ close: () => void }> = []
let implementation = subscribeRuntimeThreadEvents

function openStream(parts: string[]) {
  let controller!: ReadableStreamDefaultController<Uint8Array>
  const cancel = vi.fn()
  const stream = new ReadableStream<Uint8Array>({
    start(value) {
      controller = value
      for (const part of parts) value.enqueue(new TextEncoder().encode(part))
    },
    cancel
  })
  release.push(() => { try { controller.close() } catch { /* Already cancelled. */ } })
  return { response: new Response(stream), cancel, controller }
}

async function flush() {
  for (let step = 0; step < 30; step += 1) await Promise.resolve()
}

async function subscribe(onEvent = vi.fn(), signal = new AbortController().signal, logError = vi.fn(), boundThreadId = threadId) {
  const handle = await implementation({
    baseUrl: 'http://127.0.0.1:54321/', threadId: boundThreadId,
    headers: { Authorization: 'Bearer SYNTHETIC_SUBSCRIPTION_ONLY' }, signal, onEvent, logError
  })
  handles.push(handle)
  await flush()
  return { handle, onEvent, logError }
}

beforeEach(() => vi.useFakeTimers())
afterEach(async () => {
  for (const handle of handles.splice(0)) handle.close()
  for (const close of release.splice(0)) close()
  await flush()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe.each([
  { name: 'service', subscribe: subscribeRuntimeThreadEvents },
  { name: 'candidate', subscribe: startRuntimeThreadSubscription }
])('$name public SSE subscription behavior golden', ({ subscribe: selected }) => {
  beforeEach(() => { implementation = selected })
  it('frames fragmented LF/CRLF streams and ignores transport comments', async () => {
    const data = `: connected\n\n${frame(heartbeat(1))}${frame(heartbeat(2), '\r\n')}`
    const source = openStream([data.slice(0, 11), data.slice(11, 71), data.slice(71)])
    const request = vi.fn().mockResolvedValue(source.response)
    vi.stubGlobal('fetch', request)
    const { onEvent } = await subscribe()
    expect(onEvent.mock.calls.map(([event]) => event)).toEqual([heartbeat(1), heartbeat(2)])
    expect(String(request.mock.calls[0][0])).toBe(`http://127.0.0.1:54321/v1/threads/${threadId}/events?since_seq=0`)
    expect(request.mock.calls[0][1]).toEqual({
      signal: expect.any(AbortSignal),
      headers: { Authorization: 'Bearer SYNTHETIC_SUBSCRIPTION_ONLY', Accept: 'text/event-stream' }
    })
  })

  it.each([400, 401, 403, 404, 410])('HTTP %i revokes once without exposing an error body', async (status) => {
    const request = vi.fn().mockResolvedValue(new Response('SYNTHETIC_PRIVATE_HTTP_BODY', { status }))
    vi.stubGlobal('fetch', request)
    const { onEvent, logError } = await subscribe()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(request).toHaveBeenCalledOnce()
    expect(onEvent).toHaveBeenCalledExactlyOnceWith(revoked)
    expect(logError).toHaveBeenCalledExactlyOnceWith('sse', 'SSE connection refused', { statusCode: status })
    expect(JSON.stringify([onEvent.mock.calls, logError.mock.calls])).not.toContain('SYNTHETIC_PRIVATE_HTTP_BODY')
  })

  it.each([408, 429, 500, 503])('HTTP %i retries after the existing delay', async (status) => {
    const request = vi.fn().mockResolvedValueOnce(new Response('', { status }))
      .mockResolvedValueOnce(openStream([frame(heartbeat(7))]).response)
    vi.stubGlobal('fetch', request)
    const { onEvent } = await subscribe()
    await vi.advanceTimersByTimeAsync(749)
    expect(request).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(2)
    expect(onEvent).toHaveBeenCalledExactlyOnceWith(heartbeat(7))
  })

  it('projects a refused frame as a purge event before any private payload reaches the consumer', async () => {
    const bad = { ...heartbeat(2), threadId: 'another-thread', privatePayload: 'SYNTHETIC_PRIVATE_EVENT' }
    const source = openStream([frame(heartbeat(1)), frame(bad)])
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(source.response))
    const { onEvent, logError } = await subscribe()
    expect(onEvent.mock.calls.map(([event]) => event)).toEqual([heartbeat(1), revoked])
    expect(logError).toHaveBeenCalledWith('sse', 'SSE event rejected', {
      code: 'sse_event_rejected', reasonCode: 'event_thread_mismatch'
    })
    expect(JSON.stringify([onEvent.mock.calls, logError.mock.calls])).not.toContain('SYNTHETIC_PRIVATE_EVENT')
  })

  it('keeps transient failure diagnostics bounded', async () => {
    const request = vi.fn().mockRejectedValueOnce(new Error('SYNTHETIC_PRIVATE_NETWORK_ERROR'))
      .mockResolvedValueOnce(openStream([]).response)
    vi.stubGlobal('fetch', request)
    const { logError } = await subscribe()
    expect(logError).toHaveBeenCalledExactlyOnceWith('sse', 'SSE stream error', { code: 'stream_error' })
    await vi.advanceTimersByTimeAsync(750)
    expect(request).toHaveBeenCalledTimes(2)
  })
})

describe('Host retry lifecycle subscription', () => {
  beforeEach(() => { implementation = startRuntimeThreadSubscription })
  it('passes retry frames through the production main subscription decoder and actual strict IPC predicate', async () => {
    const events = ['scheduled', 'started', 'cancelled'].map((phase, index) => ({
      kind: 'pipeline_stage', seq: 41 + index, threadId, turnId: 'turn-retry',
      timestamp: '2026-10-07T00:00:00.123456789Z', stage: 'provider_retrying',
      label: 'Retrying provider stream', attempt: 2, maxAttempt: 3, details: { retry: {
        schemaVersion: 1, source: 'provider_retry', logicalSequence: 7, outerAttempt: 2, phase,
        ...(phase === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1 } : {}),
        ...(phase === 'cancelled' ? { cancelReason: 'context_cancelled' } : {})
      } }
    }))
    const source = openStream(events.map(event => frame(event)))
    const fetchMock = vi.fn().mockResolvedValue(source.response)
    vi.stubGlobal('fetch', fetchMock)
    const { onEvent } = await subscribe()
    const received = onEvent.mock.calls.map(([event]) => event)
    expect(received).toEqual(events)
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'actual-decoder', events: received })).toBe(true)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('subscription cancellation regressions — known baseline bugs', () => {
  beforeEach(() => { implementation = startRuntimeThreadSubscription })
  it('does not start a fetch for an already-aborted caller', async () => {
    const controller = new AbortController()
    controller.abort()
    const request = vi.fn().mockResolvedValue(openStream([]).response)
    vi.stubGlobal('fetch', request)
    await subscribe(vi.fn(), controller.signal)
    expect(request).not.toHaveBeenCalled()
  })

  it('releases the current reader on close and produces no later event', async () => {
    const source = openStream([])
    const request = vi.fn().mockResolvedValue(source.response)
    vi.stubGlobal('fetch', request)
    const { handle, onEvent } = await subscribe()
    handle.close()
    handle.close()
    await flush()
    expect(source.cancel).toHaveBeenCalledOnce()
    expect(onEvent).not.toHaveBeenCalled()
    expect(request).toHaveBeenCalledOnce()
  })

  it('removes pending retry work immediately when closed', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 503 })))
    const { handle } = await subscribe()
    expect(vi.getTimerCount()).toBe(1)
    handle.close()
    await flush()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('reconnects after EOF with the delivered cursor without a hot loop', async () => {
    const request = vi.fn().mockResolvedValueOnce(new Response(frame(heartbeat(11))))
      .mockResolvedValueOnce(openStream([frame(heartbeat(12))]).response)
    vi.stubGlobal('fetch', request)
    const { onEvent } = await subscribe()
    expect(request).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(749)
    expect(request).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(2)
    expect(String(request.mock.calls[1][0])).toContain('since_seq=11')
    expect(onEvent.mock.calls.map(([event]) => event)).toEqual([heartbeat(11), heartbeat(12)])
  })

  it('bounds exponential retry and cancels it on parent abort', async () => {
    const controller = new AbortController()
    const request = vi.fn().mockImplementation(async () => new Response('', { status: 503 }))
    vi.stubGlobal('fetch', request)
    await subscribe(vi.fn(), controller.signal)
    for (const delay of [750, 1500, 3000, 5000, 5000]) {
      const count = request.mock.calls.length
      await vi.advanceTimersByTimeAsync(delay - 1)
      expect(request).toHaveBeenCalledTimes(count)
      await vi.advanceTimersByTimeAsync(1)
      expect(request).toHaveBeenCalledTimes(count + 1)
    }
    controller.abort()
    await flush()
    expect(vi.getTimerCount()).toBe(0)
  })
})

type NativeHookupEventWitness = {
  rawBuilder: Record<string, unknown>
  projected: Record<string, unknown>
  expected: Record<string, unknown>
  promotion: Record<string, unknown>
}

function nativeHookupEventWitness(): NativeHookupEventWitness {
  const witnessPath = process.env.ANALYTIX_NATIVE_HOOKUP_EVENT_WITNESS
  if (witnessPath === undefined) {
    const timestamp = '2026-10-09T00:00:00.123456789Z'
    const expected = {
      kind: 'turn_steered', seq: 41, timestamp, threadId: 'synthetic-native-hookup-thread',
      turnId: 'synthetic-native-hookup-turn', clientUserMessageId: 'synthetic-native-hookup-message',
      admittedSeq: 40, model: 'synthetic-model', providerId: 'synthetic-provider',
      approvalPolicy: 'on-request', sandboxMode: 'workspace-write', mode: 'agent',
      disableUserInput: false, maxModelSteps: 0
    }
    return {
      rawBuilder: { ...expected, text: 'SYNTHETIC_NATIVE_HOOKUP_RAW_BUILDER_BODY' },
      projected: { ...expected },
      expected,
      promotion: {
        kind: 'item_created', seq: 42, timestamp, threadId: expected.threadId,
        turnId: expected.turnId, itemId: 'synthetic-native-hookup-item', item: {
          id: 'synthetic-native-hookup-item', threadId: expected.threadId, turnId: expected.turnId,
          role: 'user', kind: 'user_message', status: 'completed', createdAt: timestamp,
          finishedAt: timestamp, delivery: 'steer', clientUserMessageId: expected.clientUserMessageId,
          text: 'Synthetic marker-free late reply'
        }
      }
    }
  }
  // A configured exporter is required to supply every original object. Never
  // repair a producer witness by filling metadata or removing its extra text.
  if (witnessPath.trim().length === 0) throw new Error('Native hookup witness path is empty')
  const value: unknown = JSON.parse(readFileSync(witnessPath, 'utf8'))
  const record = (candidate: unknown): candidate is Record<string, unknown> =>
    candidate !== null && typeof candidate === 'object' && !Array.isArray(candidate)
  if (!record(value) || !record(value.rawBuilder) || !record(value.projected) ||
      !record(value.expected) || !record(value.promotion)) {
    throw new Error('Native hookup witness requires rawBuilder/projected/expected/promotion objects')
  }
  for (const event of [value.projected, value.expected, value.promotion]) {
    if (!Number.isSafeInteger(event.seq) || typeof event.threadId !== 'string' ||
        typeof event.turnId !== 'string' || typeof event.timestamp !== 'string') {
      throw new Error('Native hookup witness is missing an original event binding')
    }
  }
  if (value.rawBuilder.kind !== 'turn_steered' || value.projected.kind !== 'turn_steered' ||
      value.expected.kind !== 'turn_steered' || 'text' in value.expected ||
      value.promotion.kind !== 'item_created' || !record(value.promotion.item) ||
      value.promotion.item.kind !== 'user_message' || value.promotion.item.status !== 'completed' ||
      value.promotion.threadId !== value.expected.threadId) {
    throw new Error('Native hookup witness has an unexpected event purpose or owner')
  }
  return {
    rawBuilder: value.rawBuilder, projected: value.projected,
    expected: value.expected, promotion: value.promotion
  }
}


describe('native steering producer Main subscription witness', () => {
  const witness = nativeHookupEventWitness()
  const owner = witness.expected.threadId as string
  const producerFrame = (event: Record<string, unknown>) =>
    [`id: ${String(event.seq)}`, `event: ${String(event.kind)}`, `data: ${JSON.stringify(event)}`, '', ''].join('\n')
  const ownerRevoked = { ...revoked, threadId: owner }
  beforeEach(() => { implementation = startRuntimeThreadSubscription })

  it('TARGET: delivers original projected metadata and actual item_created promotion through Main', async () => {
    const source = openStream([producerFrame(witness.projected), producerFrame(witness.promotion)])
    const request = vi.fn().mockResolvedValue(source.response)
    vi.stubGlobal('fetch', request)
    const { onEvent, logError } = await subscribe(vi.fn(), new AbortController().signal, vi.fn(), owner)
    const received = onEvent.mock.calls.map(([event]) => event)
    expect(received).toEqual([witness.expected, witness.promotion])
    expect(isStrictPublicRuntimeSseIpcPayload({ streamId: 'native-hookup-main', events: received })).toBe(true)
    expect(logError).not.toHaveBeenCalled()
    expect(request).toHaveBeenCalledOnce()
    expect(String(request.mock.calls[0][0])).toContain(`/v1/threads/${encodeURIComponent(owner)}/events?since_seq=0`)
  })

  it.each([
    ['text', { text: 'SYNTHETIC_NATIVE_HOOKUP_FORBIDDEN_BODY' }],
    ['unknown field', { unexpectedField: true }],
    ['illegal status', { status: 'queued' }]
  ] as const)('revokes once on steering %s and never delivers its body or the following promotion', async (_name, fields) => {
    const bad = { ...witness.expected, ...fields }
    const source = openStream([producerFrame(bad), producerFrame(witness.promotion)])
    const request = vi.fn().mockResolvedValue(source.response)
    vi.stubGlobal('fetch', request)
    const { onEvent, logError } = await subscribe(vi.fn(), new AbortController().signal, vi.fn(), owner)
    expect(onEvent).toHaveBeenCalledExactlyOnceWith(ownerRevoked)
    expect(logError).toHaveBeenCalledExactlyOnceWith('sse', 'SSE event rejected', {
      code: 'sse_event_rejected', reasonCode: 'invalid_public_projection'
    })
    expect(JSON.stringify([onEvent.mock.calls, logError.mock.calls])).not.toContain('SYNTHETIC_NATIVE_HOOKUP_FORBIDDEN_BODY')
    expect(source.cancel).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(request).toHaveBeenCalledOnce()
    expect(onEvent).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('preserves one valid steering event then purges on a foreign promotion item', async () => {
    const bad = { ...witness.promotion, item: {
      ...(witness.promotion.item as Record<string, unknown>), threadId: `${owner}-foreign`
    } }
    const source = openStream([producerFrame(witness.expected), producerFrame(bad), producerFrame(witness.promotion)])
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(source.response))
    const { onEvent, logError } = await subscribe(vi.fn(), new AbortController().signal, vi.fn(), owner)
    expect(onEvent.mock.calls.map(([event]) => event)).toEqual([witness.expected, ownerRevoked])
    expect(logError).toHaveBeenCalledExactlyOnceWith('sse', 'SSE event rejected', {
      code: 'sse_event_rejected', reasonCode: 'invalid_public_projection'
    })
    expect(source.cancel).toHaveBeenCalledOnce()
  })
})
