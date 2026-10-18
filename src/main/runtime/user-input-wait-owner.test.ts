import { afterEach, describe, expect, it, vi } from 'vitest'
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'
import { createUserInputWaitTransportV1, UserInputWaitOwnerV1 } from './user-input-wait-owner'
import type { UserInputLocalSelectorV1 } from '../../../packages/runtime/src/contracts/user-input-local-display'
const selector: UserInputLocalSelectorV1 = { threadId: 'thread-wait', turnId: 'turn-wait', itemId: 'item_input_' + 'a'.repeat(64), requestId: 'input_' + 'a'.repeat(64) }
const encode = (text: string) => new TextEncoder().encode(text)
const frame = (remainingMs: number | null) => `data: ${JSON.stringify({ remainingMs })}\n\n`
const flush = async () => { for (let index = 0; index < 8; index++) await Promise.resolve() }
function documentOwner() { return { isCurrent: () => true, release: vi.fn() } }
let hosts: UserInputWaitOwnerV1[] = []
function host(transport: ConstructorParameters<typeof UserInputWaitOwnerV1>[0], notify = vi.fn()) {
  const owner = new UserInputWaitOwnerV1(transport, notify); hosts.push(owner); return { owner, notify }
}
afterEach(async () => { hosts.forEach(owner => owner.invalidate()); hosts = []; await flush(); vi.useRealTimers() })
describe('Main-owned user-input wait connection', () => {
  it('parses a fragmented real first frame and keeps the connection held without a business timer', async () => {
    vi.useFakeTimers()
    let stream!: ReadableStreamDefaultController<Uint8Array>
    const body = new ReadableStream<Uint8Array>({ start(controller) { stream = controller } })
    const transport = vi.fn(async () => ({ body, isCurrent: () => true })), document = documentOwner()
    const { owner, notify } = host(transport)
    expect(owner.open(selector, document)).toEqual({ state: 'connecting' }); await flush()
    stream.enqueue(encode('data: {"remaining')); stream.enqueue(encode('Ms":1200}\n\n')); await flush()
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'waiting', snapshot: { remainingMs: 1200 }, observedAt: Date.now() } })
    await vi.advanceTimersByTimeAsync(60_000)
    expect(document.release).not.toHaveBeenCalled(); expect(transport).toHaveBeenCalledOnce()
    owner.close(selector); expect(document.release).toHaveBeenCalledOnce()
  })
  it('deduplicates mounted leaves, but releases only after an explicit close or Host EOF', async () => {
    let stream!: ReadableStreamDefaultController<Uint8Array>
    const body = new ReadableStream<Uint8Array>({ start(controller) { stream = controller } })
    const first = documentOwner(), second = documentOwner(), transport = vi.fn(async () => ({ body, isCurrent: () => true }))
    const { owner, notify } = host(transport)
    owner.open(selector, first); await flush(); stream.enqueue(encode(frame(null))); await flush()
    expect(owner.open(selector, second).state).toBe('waiting'); expect(second.release).toHaveBeenCalledOnce()
    expect(first.release).not.toHaveBeenCalled(); expect(transport).toHaveBeenCalledOnce()
    stream.close(); await flush()
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'ended' } }); expect(first.release).toHaveBeenCalledOnce()
  })
  it.each(['', 'data: {"remainingMs":-1}\n\n', 'data: {"remainingMs":1,"raw":"synthetic"}\n\n', 'event: continued\ndata: {"remainingMs":1}\n\n'])('reports unavailable for malformed or empty transport %j', async text => {
    const body = new ReadableStream<Uint8Array>({ start(controller) { if (text) controller.enqueue(encode(text)); controller.close() } })
    const document = documentOwner(), { owner, notify } = host(async () => ({ body, isCurrent: () => true }))
    owner.open(selector, document); await flush()
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'unavailable' } }); expect(document.release).toHaveBeenCalledOnce()
  })
  it('does not mistake a truncated UTF-8 EOF after the first frame for settlement', async () => {
    const first = encode(frame(10)), bytes = new Uint8Array(first.length + 1); bytes.set(first); bytes[first.length] = 0xc2
    const body = new ReadableStream<Uint8Array>({ start(controller) { controller.enqueue(bytes); controller.close() } })
    const { owner, notify } = host(async () => ({ body, isCurrent: () => true })); owner.open(selector, documentOwner()); await flush()
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'unavailable' } })
  })
  it('cannot let a delayed old reader failure close a retried same-selector claim', async () => {
    const streams: ReadableStreamDefaultController<Uint8Array>[] = []
    const transport = vi.fn(async () => ({ body: new ReadableStream<Uint8Array>({ start(controller) { streams.push(controller) } }), isCurrent: () => true }))
    const first = documentOwner(), second = documentOwner(), { owner, notify } = host(transport)
    owner.open(selector, first); await flush(); owner.close(selector)
    owner.open(selector, second); await flush(); streams[1].enqueue(encode(frame(6000))); await flush()
    streams[0].error(new Error('Synthetic delayed abort')); await flush()
    expect(owner.open(selector, documentOwner()).state).toBe('waiting')
    expect(second.release).not.toHaveBeenCalled(); expect(transport).toHaveBeenCalledTimes(2)
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'waiting', snapshot: { remainingMs: 6000 }, observedAt: expect.any(Number) } })
  })
  it('cannot let a delayed old transport resolve close its successor', async () => {
    let resolveOld!: (stream: { body: ReadableStream<Uint8Array>; isCurrent: () => boolean }) => void
    const held = () => new ReadableStream<Uint8Array>({ start(controller) { controller.enqueue(encode(frame(6000))) } })
    const transport = vi.fn().mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve })).mockResolvedValue({ body: held(), isCurrent: () => true })
    const { owner } = host(transport), second = documentOwner()
    owner.open(selector, documentOwner()); owner.close(selector); owner.open(selector, second); await flush()
    resolveOld({ body: held(), isCurrent: () => true }); await flush()
    expect(owner.open(selector, documentOwner()).state).toBe('waiting'); expect(second.release).not.toHaveBeenCalled()
  })
  it('retires a document or runtime pin and never opens a reader after that pin changed', async () => {
    let resolve!: (stream: { body: ReadableStream<Uint8Array>; isCurrent: () => boolean }) => void
    const body = new ReadableStream<Uint8Array>(), getReader = vi.spyOn(body, 'getReader')
    const document = documentOwner(), { owner, notify } = host(() => new Promise(done => { resolve = done }))
    owner.open(selector, document); resolve({ body, isCurrent: () => false }); await flush()
    expect(getReader).not.toHaveBeenCalled(); expect(document.release).toHaveBeenCalledOnce()
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'unavailable' } })
  })
  it('times out an unavailable first frame without manufacturing continued', async () => {
    vi.useFakeTimers()
    const document = documentOwner(), { owner, notify } = host(async (_selector, signal) => ({ body: new ReadableStream<Uint8Array>({ start(controller) { signal.addEventListener('abort', () => controller.error(new Error('Synthetic abort')), { once: true }) } }), isCurrent: () => true }))
    owner.open(selector, document); await flush(); await vi.advanceTimersByTimeAsync(30_000); await flush()
    expect(notify).toHaveBeenLastCalledWith({ selector, wait: { state: 'unavailable' } }); expect(document.release).toHaveBeenCalledOnce()
  })
})


const privatePending = () => ({ schemaVersion: 1, ...selector, status: 'pending', callId: 'synthetic-call', answerState: 'pending',
  timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' },
  questions: [{ id: selector.requestId + '_1', header: 'Synthetic private header', question: 'Synthetic private question', multiSelect: false, options: [] }] })
async function localWaitServer(handle: (request: IncomingMessage, response: ServerResponse) => void) {
  const server = createServer(handle)
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve))
  const address = server.address(); if (!address || typeof address === 'string') throw new Error('Synthetic server unavailable')
  return { base: `http://127.0.0.1:${address.port}`, close: async () => {
    server.closeAllConnections(); await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  } }
}
type PrivatePreflight = (settings: object, path: string, body: string, signal: AbortSignal) => Promise<{ ok: boolean; status: number; body: string }>
function waitTransport(base: string, privateRequest = vi.fn<PrivatePreflight>(async () => ({ ok: true, status: 200, body: JSON.stringify(privatePending()) }))) {
  let current = true
  const pin = { runtimeUrl: base }, headers = new Headers({ Authorization: 'Bearer synthetic-test-only' })
  const transport = createUserInputWaitTransportV1({ loadSettings: async () => ({}), ensureRuntime: async () => undefined,
    captureAuthorityPin: () => pin, isCurrentAuthorityPin: value => value === pin && current,
    getBaseUrl: () => base, authHeaders: () => headers, privateDisplayRequest: privateRequest })
  return { transport, privateRequest, headers, retire: () => { current = false } }
}
describe('actual Main user-input wait GET transport', () => {
  it('preflights privately then opens the exact held GET and closes it only on explicit owner retirement', async () => {
    const seen: Array<{ url: string; method: string; accept?: string; authorization?: string; body: string }> = []
    let ended!: () => void
    const disconnected = new Promise<void>(resolve => { ended = resolve })
    const server = await localWaitServer((request, response) => {
      let body = ''; request.on('data', chunk => { body += String(chunk) }); request.on('end', () => {
        seen.push({ url: request.url!, method: request.method!, accept: request.headers.accept, authorization: request.headers.authorization, body })
        if (request.method === 'POST') { response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify(privatePending())); return }
        response.setHeader('Content-Type', 'text/event-stream; charset=utf-8'); response.write(frame(1200)); response.on('close', ended)
      })
    })
    const privateRequest = vi.fn(async (_settings: object, path: string, body: string, signal: AbortSignal) => {
      const response = await fetch(new URL(path, server.base), { method: 'POST', headers: { Authorization: 'Bearer synthetic-test-only', 'Content-Type': 'application/json' }, body, signal })
      return { ok: response.ok, status: response.status, body: await response.text() }
    })
    const fixture = waitTransport(server.base, privateRequest), document = documentOwner(), { owner, notify } = host(fixture.transport)
    try {
      owner.open(selector, document)
      await vi.waitFor(() => expect(notify).toHaveBeenCalledWith({ selector, wait: { state: 'waiting', snapshot: { remainingMs: 1200 }, observedAt: expect.any(Number) } }))
      expect(seen).toEqual([
        { url: '/v1/local-display/user-input', method: 'POST', accept: '*/*', authorization: 'Bearer synthetic-test-only', body: JSON.stringify(selector) },
        { url: `/v1/user-inputs/${selector.requestId}/wait`, method: 'GET', accept: 'text/event-stream', authorization: 'Bearer synthetic-test-only', body: '' }
      ])
      expect(document.release).not.toHaveBeenCalled(); expect(fixture.headers.has('Accept')).toBe(false)
      owner.close(selector); await disconnected; expect(document.release).toHaveBeenCalledOnce()
      expect(notify.mock.calls.some(([event]) => event.wait.state === 'continued')).toBe(false)
    } finally { owner.invalidate(); await server.close() }
  })
  it.each(['text/event-stream-invalid', 'application/json', ''])('rejects malformed SSE MIME %j over actual HTTP', async mime => {
    const server = await localWaitServer((_request, response) => { if (mime) response.setHeader('Content-Type', mime); response.end(frame(10)) })
    const fixture = waitTransport(server.base)
    try { await expect(fixture.transport(selector, new AbortController().signal)).rejects.toThrow(/^Question wait unavailable\.$/) }
    finally { await server.close() }
  })
  it('rejects redirects without following the alternate route or projecting private response data', async () => {
    let count = 0
    const server = await localWaitServer((_request, response) => { count++; response.writeHead(302, { Location: '/foreign' }); response.end('PRIVATE_RESPONSE') })
    try { await expect(waitTransport(server.base).transport(selector, new AbortController().signal)).rejects.toThrow(/^Question wait unavailable\.$/); expect(count).toBe(1) }
    finally { await server.close() }
  })
  it.each(['non200', 'foreign', 'continued', 'legacy', 'readOnly'] as const)('rejects a %s private preflight before any wait GET', async invalid => {
    let count = 0
    const server = await localWaitServer((_request, response) => { count++; response.end() })
    const original = privatePending(), display = invalid === 'foreign' ? { ...original, turnId: 'foreign' }
      : invalid === 'continued' ? { ...original, status: 'continued', answerState: 'unavailable' }
      : invalid === 'legacy' ? { ...original, timedWait: undefined }
      : invalid === 'readOnly' ? { ...original, readOnly: true, status: 'cancelled', answerState: 'cancelled', timedWait: null } : original
    const privateRequest = vi.fn<PrivatePreflight>(async () => ({ ok: true, status: invalid === 'non200' ? 202 : 200, body: JSON.stringify(display) }))
    try { await expect(waitTransport(server.base, privateRequest).transport(selector, new AbortController().signal)).rejects.toThrow('Question wait unavailable.'); expect(count).toBe(0) }
    finally { await server.close() }
  })
  it('checks the pin after private preflight and after actual HTTP response headers', async () => {
    let count = 0
    let retire: () => void = () => undefined
    const server = await localWaitServer((_request, response) => { count++; retire(); response.setHeader('Content-Type', 'text/event-stream'); response.end(frame(10)) })
    try {
      const first = waitTransport(server.base); first.privateRequest.mockImplementationOnce(async () => { first.retire(); return { ok: true, status: 200, body: JSON.stringify(privatePending()) } })
      await expect(first.transport(selector, new AbortController().signal)).rejects.toThrow('Question wait unavailable.'); expect(count).toBe(0)
      const second = waitTransport(server.base); retire = second.retire
      await expect(second.transport(selector, new AbortController().signal)).rejects.toThrow('Question wait unavailable.'); expect(count).toBe(1)
    } finally { await server.close() }
  })
})
