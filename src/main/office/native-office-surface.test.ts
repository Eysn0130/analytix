import { afterEach, expect, test, vi } from 'vitest'
import { EventEmitter } from 'node:events'
import { fixture } from './test-fixture'
const mock = vi.hoisted(() => ({ packaged: false, views: [] as any[], sessions: [] as any[], peers: [] as any[] }))
vi.mock('electron', async () => {
  const { EventEmitter } = await import('node:events')
  class Port extends EventEmitter {
    peer!: Port; closed = false; messages: any[] = []
    start() {}
    postMessage(data: any) { this.messages.push(data); queueMicrotask(() => { if (!this.closed) this.peer.emit('message', { data }) }) }
    close() { this.closed = true; this.emit('close') }
  }
  class Contents extends EventEmitter {
    dead = false; url = ''; options: any
    insertCSS = vi.fn(async (_css: string) => 'css-key'); removeInsertedCSS = vi.fn(async (_key: string) => {})
    setWindowOpenHandler = vi.fn(); setWebRTCIPHandlingPolicy = vi.fn()
    async loadURL(url: string) { this.url = url }
    isDestroyed() { return this.dead }
    close = vi.fn(() => { this.dead = true; this.emit('destroyed') })
    postMessage(_name: string, binding: any, [port]: Port[]) {
      mock.peers.push(port); port.postMessage({ type: 'ready', channel: binding.channel, protocolVersion: 1 })
    }
  }
  return {
    app: { get isPackaged() { return mock.packaged } },
    session: { fromPartition: vi.fn((partition: string, options: any) => {
      const s = Object.assign(new EventEmitter(), {
        partition, options, setPermissionCheckHandler: vi.fn(), setPermissionRequestHandler: vi.fn(), setDevicePermissionHandler: vi.fn(), setDisplayMediaRequestHandler: vi.fn(),
        webRequest: { onBeforeRequest: vi.fn() }, closeAllConnections: vi.fn(async () => {}), clearStorageData: vi.fn(async () => {})
      }); mock.sessions.push(s); return s
    }) },
    MessageChannelMain: class { port1 = new Port(); port2 = new Port(); constructor() { this.port1.peer = this.port2; this.port2.peer = this.port1 } },
    WebContentsView: class {
      webContents = new Contents(); setBounds = vi.fn(); setVisible = vi.fn()
      constructor(public options: any) { mock.views.push(this) }
    }
  }
})
import { NativeOfficeSurface } from './native-office-surface'
import { isOfficeRequest, isOfficeResult, isOfficeEvent, OFFICE_MAX_BYTES } from './office-protocol'
const live: any[] = [], cleanup: (() => Promise<void>)[] = []
afterEach(async () => { for (const s of live.splice(0)) s.destroy(); for (const c of cleanup.splice(0)) await c(); mock.packaged = false; mock.views.length = mock.sessions.length = mock.peers.length = 0; vi.useRealTimers() })
const bounds = { x: 0, y: 0, width: 800, height: 600 }
async function setup() {
  const f = await fixture(); cleanup.push(f.cleanup)
  const owner = Object.assign(new EventEmitter(), { isDestroyed: () => false, webContents: { getZoomFactor: (): number => 1, send: vi.fn() }, contentView: { addChildView: vi.fn(), removeChildView: vi.fn() } })
  const events: any[] = [], surface = new NativeOfficeSurface({ ...f, owner: owner as any, onEvent: e => events.push(e) }); live.push(surface)
  await surface.attach(bounds)
  return { surface, owner, events, view: mock.views.at(-1), session: mock.sessions.at(-1), peer: mock.peers.at(-1) }
}
const input = (command: string, operationId = command) => ({ command, operationId, documentId: 'document', version: 'version-1' }) as any
const state = (sequence = 0, acknowledged = 0, version = 'version-1') => ({ documentId: 'document', version, kind: 'docx', changeSequence: sequence, acknowledgedSequence: acknowledged, dirty: sequence !== acknowledged })
const selection = (sequence = 0, version = 'version-1') => ({ documentId: 'document', version, changeSequence: sequence, kind: 'text', scope: 'current-view-text-only', text: '' })
async function sent(peer: any) { await vi.waitFor(() => expect(peer.peer.messages.length).toBeGreaterThan(0)); return peer.peer.messages.at(-1) }
function cleanReply(peer: any, r: any, payload: any) { const { bytes, kind, status, persistedVersion, expectedChangeSequence, discard, ...base } = r; peer.postMessage({ ...base, type: 'result', ok: true, ...payload }) }
async function openSurface(s: any) {
  const p = s.surface.request({ ...input('open'), kind: 'docx', bytes: new Uint8Array([1, 2, 3]) })
  const r = await sent(s.peer); cleanReply(s.peer, r, { state: state(), selection: selection() }); await p
  s.peer.peer.messages.length = 0
  return r
}
test('source-only sandbox view rejects network, navigation, permissions, popup and download', async () => {
  const s = await setup(), prefs = s.view.options.webPreferences
  expect(prefs).toMatchObject({ sandbox: true, contextIsolation: true, nodeIntegration: false, nodeIntegrationInWorker: false, webSecurity: true, webviewTag: false, devTools: false })
  expect(s.session.partition).not.toMatch(/^persist:/); expect(s.session.options.cache).toBe(false)
  expect(s.session.setPermissionCheckHandler.mock.calls[0][0]()).toBe(false)
  const callback = vi.fn(); s.session.setPermissionRequestHandler.mock.calls[0][0](null, 'camera', callback); expect(callback).toHaveBeenCalledWith(false)
  const guard = s.session.webRequest.onBeforeRequest.mock.calls[0][0]
  for (const url of ['https://outside.invalid', 'file:///etc/passwd', 'http://127.0.0.1:1/assets/soffice.js']) { guard({ url, method: 'GET' }, callback); expect(callback).toHaveBeenLastCalledWith({ cancel: true }) }
  guard({ url: s.view.webContents.url, method: 'GET' }, callback); expect(callback).toHaveBeenLastCalledWith({ cancel: false })
  for (const name of ['will-navigate', 'will-frame-navigate', 'will-redirect', 'will-attach-webview']) { const preventDefault = vi.fn(); s.view.webContents.emit(name, { preventDefault }); expect(preventDefault).toHaveBeenCalledOnce() }
  expect(s.view.webContents.setWindowOpenHandler.mock.calls[0][0]()).toEqual({ action: 'deny' })
  const preventDefault = vi.fn(), cancel = vi.fn(); s.session.emit('will-download', { preventDefault }, { cancel }); expect(preventDefault).toHaveBeenCalledOnce(); expect(cancel).toHaveBeenCalledOnce()
  s.surface.hide(); expect(s.view.setVisible).toHaveBeenLastCalledWith(false)
  s.surface.destroy(); expect(s.owner.contentView.removeChildView).toHaveBeenCalledWith(s.view); expect(s.view.webContents.close).toHaveBeenCalledWith({ waitForBeforeUnload: false })
})
test('unmatched replies do not settle a request, and caller cannot choose channel', async () => {
  const s = await setup(); await openSurface(s)
  await expect(s.surface.request({ ...input('captureSelection'), channel: 'attacker' })).rejects.toThrow('office-invalid-request')
  let settled = false
  const p = s.surface.request(input('captureSelection')).then((v: any) => { settled = true; return v })
  const r = await sent(s.peer)
  for (const field of ['channel', 'operationId', 'documentId', 'version', 'command']) cleanReply(s.peer, { ...r, [field]: 'wrong' }, { state: state(), selection: selection() })
  await new Promise(r => setImmediate(r)); expect(settled).toBe(false)
  cleanReply(s.peer, r, { state: state(), selection: selection() }); await p
  await expect(s.surface.request(input('captureSelection'))).rejects.toThrow('office-operation-replayed')
})
test('all mutation commands are rejected before startup or port dispatch', async () => {
  const f = await fixture(); cleanup.push(f.cleanup)
  const owner = Object.assign(new EventEmitter(), { isDestroyed: () => false })
  const surface = new NativeOfficeSurface({ ...f, owner: owner as any, onEvent() {} }); live.push(surface)
  for (const command of ['export', 'ack', 'bold', 'undo', 'redo', 'local-ui']) {
    await expect(surface.request({ ...input(command), ...(command === 'ack' ? { status: 'committed', persistedVersion: 'version-2' } : {}) })).rejects.toThrow('office-invalid-request')
  }
  expect(mock.views).toHaveLength(0); expect(mock.sessions).toHaveLength(0); expect(mock.peers).toHaveLength(0)
})
test.each([[1, 0], [1, 1]])('read-only changed sequence %i ack %i fails closed', async (sequence, acknowledged) => {
  const s = await setup(), opened = await openSurface(s)
  s.peer.postMessage({ type: 'changed', channel: opened.channel, operationId: opened.operationId, documentId: 'document', version: 'version-1', state: state(sequence, acknowledged) })
  await new Promise(r => setImmediate(r))
  expect(s.events).toEqual([{ type: 'fatal', code: 'office-protocol-invalid' }]); expect(s.view.webContents.dead).toBe(true)
})
test('late generation events do not affect the active preview; zero-state notifications remain valid', async () => {
  const s = await setup(), opened = await openSurface(s)
  const changed = { type: 'changed', channel: opened.channel, operationId: opened.operationId, documentId: 'document', version: 'version-1', state: state(1) }
  s.peer.postMessage({ ...changed, operationId: 'older-open' })
  s.peer.postMessage({ ...changed, version: 'older-version', state: state(1, 0, 'older-version') })
  await new Promise(r => setImmediate(r)); expect(s.events).toEqual([]); expect(s.view.webContents.dead).toBe(false)
  s.peer.postMessage({ ...changed, state: state() })
  s.peer.postMessage({ type: 'selection', channel: opened.channel, operationId: opened.operationId, documentId: 'document', version: 'version-1', selection: selection() })
  await new Promise(r => setImmediate(r)); expect(s.events.map((e: any) => e.type)).toEqual(['changed', 'selection'])
})
test('capture result cannot assert a nonzero preview sequence', async () => {
  const s = await setup(); await openSurface(s)
  const p = s.surface.request(input('captureSelection')); const rejected = expect(p).rejects.toThrow('office-protocol-invalid'); const r = await sent(s.peer)
  cleanReply(s.peer, r, { state: state(1, 1), selection: selection(1) }); await rejected
  expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-protocol-invalid' })
})
test('operation timeout produces unknown and destroys the sink', async () => {
  const s = await setup(); await openSurface(s); vi.useFakeTimers()
  const p = s.surface.request(input('captureSelection')), rejected = expect(p).rejects.toThrow('office-operation-timeout-unknown')
  await vi.advanceTimersByTimeAsync(65001); await rejected
  expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-operation-timeout-unknown' }); expect(s.view.webContents.dead).toBe(true)
})
test('untrusted error text is replaced by a closed protocol code', async () => {
  const s = await setup(); await openSurface(s)
  const p = s.surface.request(input('captureSelection')), rejected = expect(p).rejects.toThrow('office-protocol-invalid'), r = await sent(s.peer)
  s.peer.postMessage({ ...r, type: 'result', ok: false, error: '/private/path: raw document content' }); await rejected
  expect(JSON.stringify(s.events)).not.toContain('/private/path'); expect(s.events.at(-1).code).toBe('office-protocol-invalid')
})
test('packaged app is denied before reading any resources', () => {
  mock.packaged = true
  expect(() => new NativeOfficeSurface({ owner: {} as any, assetRoot: '/no', sourceRoot: '/no', preloadPath: '/no', onEvent() {} })).toThrow('office-source-experiment-only')
})

test('conditional close refusal preserves the session for a subsequent authorized discard', async () => {
  const s = await setup(); await openSurface(s)
  await expect(s.surface.request(input('close'))).rejects.toThrow('office-invalid-request')
  const close = s.surface.request({ ...input('close'), expectedChangeSequence: 0, discard: false })
  const rejected = expect(close).rejects.toThrow('unsaved-changes'), r = await sent(s.peer)
  const { expectedChangeSequence, discard, ...base } = r
  s.peer.postMessage({ ...base, type: 'result', ok: false, error: 'unsaved-changes' }); await rejected
  expect(s.view.webContents.dead).toBe(false); expect(s.events).toEqual([])
  s.peer.peer.messages.length = 0
  const retry = s.surface.request({ ...input('close', 'discard-close'), expectedChangeSequence: 0, discard: true }), next = await sent(s.peer)
  cleanReply(s.peer, next, {}); expect((await retry).ok).toBe(true)
})
test('hide while startup is pending cannot be undone by a late attach', async () => {
  const f = await fixture(); cleanup.push(f.cleanup)
  const owner = Object.assign(new EventEmitter(), { isDestroyed: () => false, webContents: { getZoomFactor: (): number => 1, send: vi.fn() }, contentView: { addChildView: vi.fn(), removeChildView: vi.fn() } })
  const surface = new NativeOfficeSurface({ ...f, owner: owner as any, onEvent() {} }); live.push(surface)
  const attaching = surface.attach(bounds); surface.hide(); await attaching
  expect(owner.contentView.addChildView).not.toHaveBeenCalled()
  expect(mock.views.at(-1).setVisible).not.toHaveBeenCalledWith(true)
})
test('owner closure terminates a pending preview operation', async () => {
  const s = await setup(); await openSurface(s)
  const capturing = s.surface.request(input('captureSelection')), rejected = expect(capturing).rejects.toThrow('office-surface-destroyed')
  await sent(s.peer); s.owner.emit('closed'); await rejected
  expect(s.view.webContents.dead).toBe(true)
})

test('preview bounds follow the main window zoom without scaling native input coordinates', async () => {
  const s = await setup()
  s.owner.webContents.getZoomFactor = () => 1.25
  await s.surface.attach({ x: 100, y: 40, width: 400, height: 600 })
  expect(s.view.setBounds).toHaveBeenLastCalledWith({ x: 125, y: 50, width: 500, height: 750 })
})

test('typed native protocol retains exact fields and bounded selection and input bytes', () => {
  const base = { ...input('captureSelection'), channel: 'channel' }
  expect(isOfficeRequest(base)).toBe(true)
  expect(isOfficeRequest({ ...base, value: 'unexpected' })).toBe(false)
  for (const bytes of [new Uint8Array(), new Uint8Array(OFFICE_MAX_BYTES + 1), new Uint8Array(new SharedArrayBuffer(1))]) expect(isOfficeRequest({ ...base, command: 'open', kind: 'docx', bytes })).toBe(false)
  expect(isOfficeRequest({ ...base, command: 'open', kind: 'xlsx', bytes: new Uint8Array([1]) })).toBe(true)
  for (const command of ['ack', 'bold', 'format', 'replace', 'undo', 'redo']) expect(isOfficeRequest({ ...base, command })).toBe(false)
  for (const command of ['edit','export']) {
    expect(isOfficeRequest({ ...base, command })).toBe(true)
    expect(isOfficeRequest({ ...base, command, script:'unsafe' })).toBe(false)
    expect(isOfficeResult({ ...base, command, type:'result', ok:false, error:'unsupported-command' })).toBe(true)
  }
  expect(isOfficeEvent({ channel: 'channel', operationId: 'save', documentId: 'document', version: 'version-1', type: 'save-requested' })).toBe(true)
  const reply = { ...base, type: 'result', ok: true, state: state(), selection: selection() }
  expect(isOfficeResult(reply)).toBe(true)
  expect(isOfficeResult({ ...reply, bytes: new Uint8Array([1]) })).toBe(false)
  expect(isOfficeResult({ ...reply, selection: { ...selection(), text: 'x'.repeat(4097) } })).toBe(false)
  expect(isOfficeResult({ ...reply, selection: { ...selection(), version: 'wrong' } })).toBe(false)
})

test('synchronizes shell appearance once per change without changing document pixels', async () => {
  const s = await setup()
  await s.surface.attach(bounds, { theme: 'dark', reducedMotion: true })
  const css = s.view.webContents.insertCSS.mock.calls[0][0]
  expect(css).toContain('color-scheme:dark')
  expect(css).toContain('button{transition:none!important}')
  expect(css).not.toContain('canvas')
  await s.surface.attach({ ...bounds, x: 4 }, { theme: 'dark', reducedMotion: true })
  expect(s.view.webContents.insertCSS).toHaveBeenCalledTimes(1)
  await s.surface.attach(bounds, { theme: 'light', reducedMotion: false })
  expect(s.view.webContents.removeInsertedCSS).toHaveBeenCalledWith('css-key')
})

test('native focus routes only fixed workspace shortcuts and leaves content input and save keys to the isolated input guard', async () => {
  const s = await setup(); await openSurface(s)
  const preventDefault = vi.fn()
  for (const [key,shift,command] of [['w',false,'close-tab'],['b',true,'toggle-workspace'],['`',false,'toggle-terminal']] as const) {
    s.view.webContents.emit('before-input-event',{preventDefault},{type:'keyDown',key,meta:true,shift,isAutoRepeat:false,isComposing:false})
    expect(s.owner.webContents.send).toHaveBeenLastCalledWith('office:workspace-command',{objectId:'document',command})
  }
  expect(preventDefault).toHaveBeenCalledTimes(3)
  for (const input of [{key:'w',isComposing:true},{key:'w',isAutoRepeat:true},{key:'s'},{key:'z'},{key:'Escape'}]) {
    s.view.webContents.emit('before-input-event',{preventDefault},{type:'keyDown',meta:true,...input})
  }
  expect(preventDefault).toHaveBeenCalledTimes(3)
})
