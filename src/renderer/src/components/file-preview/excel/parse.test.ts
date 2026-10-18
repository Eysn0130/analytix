import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { parseExcel } from './parse'

const limits = { maxBytes: 16 << 20, maxUncompressedBytes: 64 << 20, maxCells: 250_000, timeoutMs: 15_000 }
const preview = { sheets: [{ id: '1', name: 'CSV', row: 1, column: 1, hide: 0, celldata: [] }], missingResults: 0, unsupportedFeatures: [] }
class ParserWorker {
  static instances: ParserWorker[] = []
  onmessage: ((event: MessageEvent<unknown>) => void) | null = null
  onerror: (() => void) | null = null
  onmessageerror: (() => void) | null = null
  postMessage = vi.fn()
  terminate = vi.fn()
  constructor(readonly url: URL, readonly options: WorkerOptions) { ParserWorker.instances.push(this) }
  reply(data: unknown) { this.onmessage?.({ data } as MessageEvent<unknown>) }
}
beforeEach(() => { ParserWorker.instances = []; vi.useFakeTimers(); vi.stubGlobal('Worker', ParserWorker) })
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })

describe('same-origin disposable spreadsheet parser', () => {
  it('uses a module worker and transfers a copy while preserving the retained current-file bytes', async () => {
    const bytes = new Uint8Array([65, 44, 66]), controller = new AbortController()
    const result = parseExcel(bytes, 'csv', limits, controller.signal)
    const worker = ParserWorker.instances[0]
    expect(worker.options).toEqual({ type: 'module', name: 'analytix-excel-preview' })
    expect(worker.url.pathname).toMatch(/\/worker\.ts$/)
    expect(worker.url.protocol).not.toBe('blob:')
    const [request, transfer] = worker.postMessage.mock.calls[0] as [{ bytes: Uint8Array; format: string; limits: typeof limits }, ArrayBuffer[]]
    expect(request).toMatchObject({ format: 'csv', limits })
    expect(request.bytes).toEqual(bytes); expect(request.bytes.buffer).not.toBe(bytes.buffer)
    expect(transfer).toEqual([request.bytes.buffer])
    worker.reply({ ok: true, value: preview })
    await expect(result).resolves.toEqual(preview)
    expect(bytes).toEqual(new Uint8Array([65, 44, 66]))
    expect(worker.terminate).toHaveBeenCalledTimes(1)
    expect(worker.onmessage).toBeNull(); expect(vi.getTimerCount()).toBe(0)
  })
  it('retires on abort before a late parser completion and never borrows another document lifetime', async () => {
    const first = new AbortController(), second = new AbortController(), bytes = new Uint8Array([65])
    const a = parseExcel(bytes, 'csv', limits, first.signal), workerA = ParserWorker.instances[0]
    const late = workerA.onmessage!
    const rejected = expect(a).rejects.toMatchObject({ name: 'AbortError' })
    const b = parseExcel(bytes, 'csv', limits, second.signal), workerB = ParserWorker.instances[1]
    first.abort(); await rejected
    late({ data: { ok: true, value: preview } } as MessageEvent<unknown>)
    workerB.reply({ ok: true, value: preview }); await expect(b).resolves.toEqual(preview)
    expect(workerA.terminate).toHaveBeenCalledTimes(1); expect(workerB.terminate).toHaveBeenCalledTimes(1)
  })
  it('fails before allocating a worker for expired or oversized input', async () => {
    const controller = new AbortController(); controller.abort()
    expect(() => parseExcel(new Uint8Array([65]), 'csv', limits, controller.signal)).toThrow()
    await expect(parseExcel(new Uint8Array([65, 66]), 'csv', { ...limits, maxBytes: 1 }, new AbortController().signal)).rejects.toThrow('tooLarge')
    expect(ParserWorker.instances).toHaveLength(0)
  })
  it('terminates once on timeout and drops late success', async () => {
    const result = parseExcel(new Uint8Array([65]), 'csv', limits, new AbortController().signal)
    const worker = ParserWorker.instances[0], late = worker.onmessage!
    const rejected = expect(result).rejects.toThrow('timeout')
    await vi.advanceTimersByTimeAsync(limits.timeoutMs); await rejected
    late({ data: { ok: true, value: preview } } as MessageEvent<unknown>)
    expect(worker.terminate).toHaveBeenCalledTimes(1); expect(vi.getTimerCount()).toBe(0)
  })
  it.each([null, {}, { ok: true, value: {} }, { ok: true, value: { ...preview, unsupportedFeatures: ['unknown'] } },
    { ok: true, value: { ...preview, sheets: [{ ...preview.sheets[0], row: 250001 }] } },
    { ok: false, code: 'untrusted' }])('retires malformed worker reply %j', async reply => {
    const result = parseExcel(new Uint8Array([65]), 'csv', limits, new AbortController().signal), worker = ParserWorker.instances[0]
    worker.reply(reply)
    await expect(result).rejects.toThrow('invalid')
    expect(worker.terminate).toHaveBeenCalledTimes(1)
  })
  it('preserves a known parser failure category and retires transport failure', async () => {
    const result = parseExcel(new Uint8Array([65]), 'csv', limits, new AbortController().signal)
    ParserWorker.instances[0].reply({ ok: false, code: 'encoding' }); await expect(result).rejects.toThrow('encoding')
    const transport = parseExcel(new Uint8Array([65]), 'csv', limits, new AbortController().signal)
    ParserWorker.instances[1].onmessageerror?.(); await expect(transport).rejects.toThrow('invalid')
    expect(ParserWorker.instances[1].terminate).toHaveBeenCalledTimes(1)
  })
})
