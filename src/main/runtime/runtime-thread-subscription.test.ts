import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { subscribeRuntimeThreadEvents } from '../claw-runtime-helpers'
import { startRuntimeThreadSubscription } from './runtime-thread-subscription'

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

async function subscribe(onEvent = vi.fn(), signal = new AbortController().signal, logError = vi.fn()) {
  const handle = await implementation({
    baseUrl: 'http://127.0.0.1:54321/', threadId,
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
