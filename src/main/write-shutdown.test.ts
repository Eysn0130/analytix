import { EventEmitter } from 'node:events'
import type { BrowserWindow, IpcMain } from 'electron'
import { afterEach, expect, test, vi } from 'vitest'
import { createWriteShutdownCoordinator } from './write-shutdown'

function setup() {
  const handlers = new Map<string, (...args: any[]) => any>()
  const native = vi.fn(async () => true), cancel = vi.fn(async () => undefined), notify = vi.fn(async () => undefined)
  const coordinator = createWriteShutdownCoordinator({ ipc: { handle: (name, handler) => handlers.set(name, handler) } as Pick<IpcMain,'handle'>,
    prepareNative: native, cancelNative: cancel, notifyBlocked: notify, timeoutMs: 50 })
  const window = () => {
    let destroyed = false
    const main = Object.assign(new EventEmitter(), { isDestroyed: () => destroyed, webContents: Object.assign(new EventEmitter(), { mainFrame: {}, send: vi.fn(), isLoadingMainFrame: vi.fn(() => false), setWindowOpenHandler: vi.fn() }), close: vi.fn() })
    const close = () => {
      const event = { defaultPrevented: false, preventDefault() { this.defaultPrevented = true } }
      main.emit('close', event)
      return event
    }
    main.close.mockImplementation(() => { if (!close().defaultPrevented) { destroyed = true; main.emit('closed') } })
    coordinator.trackWindow(main as unknown as BrowserWindow)
    const ack = (outcome: unknown = { result: 'ready' }, event?: unknown, body?: unknown) => {
      const request = main.webContents.send.mock.calls.at(-1)![1]
      return handlers.get('write:shutdown-ack')!(event ?? { sender: main.webContents, senderFrame: main.webContents.mainFrame }, body ?? { ...request, outcome })
    }
    const autoCancel = () => main.webContents.send.mockImplementation((_channel, request) => {
      if (request.phase === 'cancel') ack()
    })
    return { main, close, ack, autoCancel }
  }
  return { coordinator, native, cancel, notify, window }
}
afterEach(() => vi.useRealTimers())
test('one close owner waits for text ACK, then native, and replays close once', async () => {
  const h = setup(), w = h.window()
  expect(w.close().defaultPrevented).toBe(true); w.close()
  expect(w.main.webContents.send).toHaveBeenCalledOnce(); expect(h.native).not.toHaveBeenCalled()
  w.ack(); await vi.waitFor(() => expect(w.main.close).toHaveBeenCalledOnce())
  expect(h.native).toHaveBeenCalledExactlyOnceWith(w.main)
  expect(h.cancel).not.toHaveBeenCalled()
})
test.each(['sender','frame','id','phase','extra','navigation'] as const)('rejects %s ACK and cancels the unconfirmed freeze on timeout', async kind => {
  vi.useFakeTimers(); const h = setup(), w = h.window(); w.autoCancel()
  const waiting = h.coordinator.prepareQuit()
  const request = w.main.webContents.send.mock.calls.at(-1)![1]
  const event = { sender: w.main.webContents, senderFrame: w.main.webContents.mainFrame }
  const body = { ...request, outcome: { result: 'ready' } }
  if (kind === 'navigation') w.main.webContents.mainFrame = {}
  const result = w.ack(undefined, kind === 'sender' ? { ...event, sender: {} } : kind === 'frame' ? { ...event, senderFrame: {} } : event,
    kind === 'id' ? { ...body, requestId: '00000000-0000-4000-8000-000000000000' } : kind === 'phase' ? { ...body, phase: 'cancel' } : kind === 'extra' ? { ...body, content: 'must not be accepted' } : body)
  expect(result).toEqual({ ok: false })
  await vi.advanceTimersByTimeAsync(100)
  expect(await waiting).toBe(false); expect(h.native).not.toHaveBeenCalled(); expect(h.cancel).toHaveBeenCalledOnce()
  expect(w.main.close).not.toHaveBeenCalled()
})
test('native cancel thaws every prepared renderer and permits another attempt', async () => {
  const h = setup(), a = h.window(), b = h.window(); a.autoCancel(); b.autoCancel(); h.native.mockResolvedValue(false)
  const quit = h.coordinator.prepareQuit(); a.ack()
  await vi.waitFor(() => expect(b.main.webContents.send).toHaveBeenCalledOnce()); b.ack()
  expect(await quit).toBe(false)
  for (const w of [a,b]) expect(w.main.webContents.send).toHaveBeenLastCalledWith('write:shutdown-request', expect.objectContaining({phase:'cancel'}))
  h.native.mockResolvedValue(true)
  const retry = h.coordinator.prepareQuit(); a.ack(); await vi.waitFor(() => expect(b.main.webContents.send.mock.calls.at(-1)![1].phase).toBe('prepare')); b.ack()
  expect(await retry).toBe(true)
})
test('an earlier tray/ask cancellation does not start any preparation', () => {
  const h=setup(),w=h.window()
  const event={defaultPrevented:true,preventDefault:vi.fn()};w.main.emit('close',event)
  expect(event.preventDefault).not.toHaveBeenCalled();expect(w.main.webContents.send).not.toHaveBeenCalled();expect(h.native).not.toHaveBeenCalled()
})
test('quit joins in-flight window preparation without an early close or thaw',async()=>{
  const h=setup(),w=h.window();w.close();const quit=h.coordinator.prepareQuit();w.ack()
  expect(await quit).toBe(true);expect(w.main.close).not.toHaveBeenCalled();expect(h.cancel).not.toHaveBeenCalled()
  expect(w.main.webContents.send).toHaveBeenCalledOnce()
})
test('window creation during native preparation prevents quit from missing a renderer',async()=>{
  const h=setup(),w=h.window();w.autoCancel();let finish!:(ok:boolean)=>void
  h.native.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  const quit=h.coordinator.prepareQuit();w.ack();await vi.waitFor(()=>expect(h.native).toHaveBeenCalledOnce())
  h.window();finish(true);expect(await quit).toBe(false)
})
test('a lost cancel ACK must be resolved before the next prepare token is sent',async()=>{
  vi.useFakeTimers();const h=setup(),w=h.window()
  h.native.mockResolvedValue(false)
  const first=h.coordinator.prepareQuit();w.ack();await vi.advanceTimersByTimeAsync(100)
  expect(await first).toBe(false)
  const cancelled=w.main.webContents.send.mock.calls.at(-1)![1]
  expect(cancelled.phase).toBe('cancel')
  h.native.mockResolvedValue(true)
  const second=h.coordinator.prepareQuit()
  expect(w.main.webContents.send).toHaveBeenLastCalledWith('write:shutdown-request',cancelled)
  w.ack();await vi.advanceTimersByTimeAsync(0)
  expect(w.main.webContents.send.mock.calls.at(-1)![1]).toMatchObject({phase:'prepare'})
  expect(w.main.webContents.send.mock.calls.at(-1)![1].requestId).not.toBe(cancelled.requestId)
  w.ack();expect(await second).toBe(true)
})
test('a later close-policy veto releases a prepared document instead of leaving the editor frozen',async()=>{
  const h=setup(),w=h.window();w.autoCancel()
  w.main.on('close',event=>{if(h.native.mock.calls.length)event.preventDefault()})
  w.close();w.ack();await vi.waitFor(()=>expect(h.cancel).toHaveBeenCalledOnce())
  expect(w.main.isDestroyed()).toBe(false)
  expect(w.main.webContents.send).toHaveBeenLastCalledWith('write:shutdown-request',expect.objectContaining({phase:'cancel'}))
})
test('a late unprepared window and a replaced prepared frame cannot inherit quit close admission',async()=>{
  const h=setup(),a=h.window()
  const preparing=h.coordinator.prepareQuit();a.ack();expect(await preparing).toBe(true)
  const late=h.window()
  expect(late.main.webContents.send).not.toHaveBeenCalled()
  expect(late.close().defaultPrevented).toBe(true)
  a.main.webContents.mainFrame={}
  expect(a.close().defaultPrevented).toBe(true)
  expect(h.native).toHaveBeenCalledOnce()
})
test('quit preparation closes creation and main-frame navigation admission until explicit cancel',async()=>{
  const h=setup(),w=h.window();w.autoCancel()
  expect(h.coordinator.canCreateWindow()).toBe(true)
  const prepare=h.coordinator.prepareQuit()
  expect(h.coordinator.canCreateWindow()).toBe(false)
  w.ack();expect(await prepare).toBe(true)
  expect(h.coordinator.canCreateWindow()).toBe(false)
  for(const name of ['will-navigate','will-frame-navigate','will-redirect']){
    const event={isMainFrame:true,preventDefault:vi.fn()};w.main.webContents.emit(name,event)
    expect(event.preventDefault).toHaveBeenCalledOnce()
  }
  const subframe={isMainFrame:false,preventDefault:vi.fn()};w.main.webContents.emit('will-frame-navigate',subframe)
  expect(subframe.preventDefault).not.toHaveBeenCalled()
  const popup=w.main.webContents.setWindowOpenHandler.mock.calls[0][0]
  expect(popup()).toEqual({action:'deny'})
  expect(w.close().defaultPrevented).toBe(false)
  await h.coordinator.cancel()
  expect(h.coordinator.canCreateWindow()).toBe(true);expect(popup()).toEqual({action:'allow'})
  const navigation={preventDefault:vi.fn()};w.main.webContents.emit('will-navigate',navigation)
  expect(navigation.preventDefault).not.toHaveBeenCalled()
})
test('an already loading main frame cannot be admitted for Core shutdown',async()=>{
  const h=setup(),w=h.window();w.main.webContents.isLoadingMainFrame.mockReturnValue(true)
  expect(await h.coordinator.prepareQuit()).toBe(false)
  expect(w.main.webContents.send).not.toHaveBeenCalled();expect(h.native).not.toHaveBeenCalled()
  expect(h.coordinator.canCreateWindow()).toBe(true)
})
