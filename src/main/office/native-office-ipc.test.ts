import { beforeEach, expect, it, vi } from 'vitest'
import { EventEmitter } from 'node:events'
const state = vi.hoisted(() => ({
  packaged: false, handlers: new Map<string, (...args: any[]) => any>(),
  lifecycle: new Map<string, (...args: any[]) => void>(),
  request: vi.fn(), getView: vi.fn(), destroy: vi.fn(), create: vi.fn(), dialog: vi.fn(), picker: vi.fn(), quit: vi.fn()
}))
vi.mock('electron', () => ({
  app: { get isPackaged() { return state.packaged }, getAppPath: () => '/source', getLocale: () => 'en',
    on: (name: string, handler: (...args: any[]) => void) => state.lifecycle.set(name, handler), quit: state.quit },
  dialog: { showMessageBox: state.dialog, showOpenDialog: state.picker }, ipcMain: { handle: (name: string, handler: (...args: any[]) => any) => state.handlers.set(name, handler) }
}))
vi.mock('./native-office-controller', () => ({ createNativeOfficeController: (...args: unknown[]) => {
  state.create(...args)
  return { request: state.request, getView: state.getView, destroy: state.destroy }
} }))
import { registerNativeOfficeIpc } from './native-office-ipc'
beforeEach(() => {
  vi.clearAllMocks(); state.packaged = false; state.handlers.clear(); state.lifecycle.clear()
  state.request.mockResolvedValue({ ok: true, view: null }); state.getView.mockReturnValue(null)
  state.destroy.mockResolvedValue(undefined)
})
function setup() {
  const main = Object.assign(new EventEmitter(), { isDestroyed: () => false, close: vi.fn(),
    webContents: { mainFrame: {}, send: vi.fn() } })
  let current: typeof main | null = main
  registerNativeOfficeIpc(() => current as any, vi.fn())
  const event = { sender: main.webContents, senderFrame: main.webContents.mainFrame }
  return { main, event, invoke: (e: unknown = event, payload: unknown = { action: 'status' }) => state.handlers.get('office:request')!(e, payload),
    pick: (payload: unknown = { kind: 'docx', workspace: '/workspace' }, e: unknown = event) => state.handlers.get('office:pick-file')!(e, payload),
    replace: () => { current = null } }
}
it('uses an Office-only single-file picker and preserves cancellation without opening an object', async () => {
  const h = setup()
  state.picker.mockResolvedValue({ canceled: false, filePaths: ['/workspace/example.docx'] })
  expect(await h.pick()).toEqual({ ok: true, path: '/workspace/example.docx' })
  expect(state.picker).toHaveBeenCalledWith(h.main, expect.objectContaining({
    defaultPath: '/workspace', filters: [{ name: 'DOCX', extensions: ['docx'] }], properties: ['openFile', 'dontAddToRecent']
  }))
  state.picker.mockResolvedValue({ canceled: true, filePaths: [] })
  expect(await h.pick()).toEqual({ ok: true, path: null })
  expect(state.create).not.toHaveBeenCalled()
})
it('rejects invalid picker intent, foreign frames, wrong file kinds and late private paths', async () => {
  const h = setup()
  expect(await h.pick({ kind: 'docx', workspace: 'relative' })).toEqual({ ok: false, error: 'invalid_request' })
  expect(await h.pick(undefined, { sender: {}, senderFrame: {} })).toEqual({ ok: false, error: 'unavailable' })
  state.packaged = true
  expect(await h.pick()).toEqual({ ok: false, error: 'unavailable' })
  expect(state.picker).not.toHaveBeenCalled()
  state.packaged = false
  for (const paths of [['/workspace/example.xlsx'], ['/workspace/a.docx', '/workspace/b.docx'], ['relative.docx']]) {
    state.picker.mockResolvedValue({ canceled: false, filePaths: paths })
    expect(await h.pick()).toEqual({ ok: false, error: 'unavailable' })
  }
  let finish!: (value: unknown) => void
  state.picker.mockImplementation(() => new Promise(resolve => { finish = resolve }))
  const pending = h.pick()
  h.main.webContents.mainFrame = {}
  finish({ canceled: false, filePaths: ['/private/example.docx'] })
  expect(await pending).toEqual({ ok: false, error: 'unavailable' })
})
it('refuses non-owner frames and packaged execution before creating a surface controller', async () => {
  const h = setup()
  for (const event of [{ sender: {}, senderFrame: h.event.senderFrame }, { sender: h.event.sender, senderFrame: {} }]) {
    expect(await h.invoke(event)).toEqual({ ok: false, view: null, error: 'unavailable' })
  }
  state.packaged = true
  expect(await h.invoke()).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(state.create).not.toHaveBeenCalled(); expect(state.request).not.toHaveBeenCalled()
})
it('drops a protected local reply when the owner frame changes during the request', async () => {
  const h = setup()
  let finish!: (value: unknown) => void
  state.request.mockImplementation(() => new Promise(resolve => { finish = resolve }))
  const pending = h.invoke()
  h.main.webContents.mainFrame = {}
  finish({ ok: true, view: { path: '/private/example.docx' } })
  expect(await pending).toEqual({ ok: false, view: null, error: 'unavailable' })
})
it('closing a readonly preview releases its controller without a save prompt', async () => {
  const h = setup(); await h.invoke()
  state.getView.mockReturnValue({ dirty: false, status: 'ready' })
  const event = { preventDefault: vi.fn() }
  h.main.emit('close', event)
  expect(event.preventDefault).not.toHaveBeenCalled()
  expect(state.dialog).not.toHaveBeenCalled()
  h.main.emit('closed')
  expect(state.destroy).toHaveBeenCalledOnce()
  expect(state.request.mock.calls.some(([request]) => request.action === 'save')).toBe(false)
})
