import { EventEmitter } from 'node:events'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('electron', () => ({ ipcMain: { handle: vi.fn() } }))
import { ipcMain, type BrowserWindow, type IpcMainInvokeEvent } from 'electron'
import { beginUserInputWaitTransitionV1, registerUserInputWaitIpcV1 } from './user-input-wait-ipc'
const requestId = 'input_' + 'a'.repeat(64)
const selector = { threadId: 'thread-wait', turnId: 'turn-source', itemId: 'item_' + requestId, requestId }
const flush = async () => { for (let index = 0; index < 8; index++) await Promise.resolve() }
class Contents extends EventEmitter {
  id = 501; mainFrame = {}; destroyed = false; send = vi.fn()
  isDestroyed() { return this.destroyed }
}
let contents: Contents, window: EventEmitter & { webContents: Contents; isDestroyed: () => boolean }
let releases: Array<() => void>
const invoke = (channel: string, event: unknown, payload: unknown) => {
  const entry = vi.mocked(ipcMain.handle).mock.calls.find(call => call[0] === channel)
  if (!entry) throw new Error('Missing handler')
  return entry[1](event as IpcMainInvokeEvent, payload)
}
const event = (frame: unknown = contents.mainFrame) => ({ sender: contents, senderFrame: frame })
beforeEach(() => { vi.mocked(ipcMain.handle).mockClear(); contents = new Contents(); window = Object.assign(new EventEmitter(), { webContents: contents, isDestroyed: () => false }); releases = [] })
afterEach(async () => { releases.forEach(release => release()); contents.emit('destroyed'); await flush(); vi.restoreAllMocks() })
function register() {
  const signals: AbortSignal[] = []
  const transport = vi.fn(async (_selector, signal: AbortSignal) => {
    signals.push(signal)
    return { body: new ReadableStream<Uint8Array>({ start(controller) {
      controller.enqueue(new TextEncoder().encode('data: {"remainingMs":6000}\n\n'))
      signal.addEventListener('abort', () => controller.error(new Error('Synthetic abort')), { once: true })
    } }), isCurrent: () => true }
  })
  registerUserInputWaitIpcV1(() => window as unknown as BrowserWindow, transport)
  return { transport, signals }
}
describe('typed user-input wait Main IPC owner', () => {
  it('admits only the current main document and strict complete selector', async () => {
    const { transport } = register()
    expect(invoke('user-input-wait:open', event({}), selector)).toEqual({ state: 'unavailable' })
    expect(invoke('user-input-wait:open', event(), { ...selector, raw: 'synthetic injection' })).toEqual({ state: 'unavailable' })
    expect(transport).not.toHaveBeenCalled()
    expect(invoke('user-input-wait:open', event(), selector)).toEqual({ state: 'connecting' }); await flush()
    expect(contents.send).toHaveBeenCalledWith('user-input-wait:changed', { selector, wait: { state: 'waiting', snapshot: { remainingMs: 6000 }, observedAt: expect.any(Number) } })
  })
  it('keeps named hide/minimize independent from the business claim', async () => {
    const { signals } = register(); invoke('user-input-wait:open', event(), selector); await flush()
    window.emit('hide'); window.emit('minimize'); await flush(); expect(signals[0].aborted).toBe(false)
    invoke('user-input-wait:close', event(), selector); await flush(); expect(signals[0].aborted).toBe(true)
  })
  it.each(['did-start-navigation', 'render-process-gone'])('retires the irreversible document lease on %s', async name => {
    const { signals } = register(); invoke('user-input-wait:open', event(), selector); await flush()
    if (name === 'did-start-navigation') contents.emit(name, {}, 'https://example.invalid', false, true)
    else contents.emit(name, {})
    await flush(); expect(signals[0].aborted).toBe(true)
  })
  it('invalidates an existing claim and fences new admission throughout a case/runtime transition', async () => {
    const { signals, transport } = register(); invoke('user-input-wait:open', event(), selector); await flush()
    const release = beginUserInputWaitTransitionV1(); releases.push(release); await flush()
    expect(signals[0].aborted).toBe(true)
    expect(contents.send).toHaveBeenCalledWith('user-input-wait:changed', { selector, wait: { state: 'unavailable' } })
    expect(invoke('user-input-wait:open', event(), selector)).toEqual({ state: 'unavailable' }); expect(transport).toHaveBeenCalledOnce()
    release(); expect(invoke('user-input-wait:open', event(), selector).state).toBe('connecting')
  })
})
