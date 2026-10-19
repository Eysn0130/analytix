import { afterEach, expect, test, vi } from 'vitest'
import { EventEmitter } from 'node:events'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fixture } from './test-fixture'
const mock = vi.hoisted(() => ({ privateLoad:vi.fn(), loadURL:vi.fn(), packaged: false, views: [] as any[], sessions: [] as any[], peers: [] as any[] }))
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
    setIgnoreMenuShortcuts = vi.fn(); executeJavaScriptInIsolatedWorld = vi.fn(async () => undefined)
    async loadURL(url: string) { await mock.loadURL(); this.url = url }
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
vi.mock('./office-assets',async(importOriginal)=>({...await importOriginal<object>(),loadPrivateOfficeBuffers:mock.privateLoad}))
vi.mock('../logger', () => ({ logNativeOfficeFailure: vi.fn() }))
import { logNativeOfficeFailure } from '../logger'
import { createOfficePrivateAdmissionProvider } from './office-private-admission'
import { loadOfficeBuffers } from './office-assets'
import { NativeOfficeSurface } from './native-office-surface'
import { isOfficeRequest, isOfficeResult, isOfficeEvent, OFFICE_MAX_BYTES } from './office-protocol'
const live: any[] = [], cleanup: (() => Promise<void>)[] = []
afterEach(async () => { for (const s of live.splice(0)) s.destroy(); for (const c of cleanup.splice(0)) await c(); mock.loadURL.mockReset(); vi.mocked(logNativeOfficeFailure).mockClear(); mock.packaged = false; mock.views.length = mock.sessions.length = mock.peers.length = 0; vi.useRealTimers() })
const bounds = { x: 0, y: 0, width: 800, height: 600 }
async function setup(protectedPresent?: boolean) {
  const f = await fixture(); cleanup.push(f.cleanup)
  const owner = Object.assign(new EventEmitter(), { isDestroyed: (): boolean => false, webContents: Object.assign(new EventEmitter(), { getZoomFactor: (): number => 1, send: vi.fn() }), contentView: { addChildView: vi.fn(), removeChildView: vi.fn() } })
  const events: any[] = [], surface = new NativeOfficeSurface({ ...f, owner: owner as any, protectedPresent, onEvent: e => events.push(e) }); live.push(surface)
  await surface.attach(bounds)
  return { surface, owner, events, view: mock.views.at(-1), session: mock.sessions.at(-1), peer: mock.peers.at(-1) }
}
const input = (command: string, operationId = command) => ({ command, operationId, documentId: 'document', version: 'version-1' }) as any
const state = (sequence = 0, acknowledged = 0, version = 'version-1') => ({ documentId: 'document', version, kind: 'docx', changeSequence: sequence, acknowledgedSequence: acknowledged, dirty: sequence !== acknowledged })
const selection = (sequence = 0, version = 'version-1') => ({ documentId: 'document', version, changeSequence: sequence, kind: 'text', scope: 'current-view-text-only', text: '' })
async function sent(peer: any) { await vi.waitFor(() => expect(peer.peer.messages.length).toBeGreaterThan(0)); return peer.peer.messages.at(-1) }
function cleanReply(peer: any, r: any, payload: any) { const { bytes, kind, status, persistedVersion, expectedChangeSequence, discard, action, zoom, ...base } = r; peer.postMessage({ ...base, type: 'result', ok: true, ...payload }) }
async function openSurface(s: any, kind: 'docx' | 'xlsx' | 'pptx' = 'docx') {
  const p = s.surface.request({ ...input('open'), kind, bytes: new Uint8Array([1, 2, 3]) })
  const initialSelection = kind === 'docx' ? selection() : {documentId:'document',version:'version-1',changeSequence:0,kind:'unavailable',scope:'current-view-unavailable'}
  const r = await sent(s.peer); cleanReply(s.peer, r, { state: {...state(),kind}, selection:initialSelection }); await p
  s.peer.peer.messages.length = 0
  return r
}
test.each(['docx','xlsx','pptx'] as const)('routes native %s right press and keyboard to one versioned menu intent without engine mutation', async kind => {
  const s=await setup(); await openSurface(s,kind)
  const preventDefault=vi.fn()
  const mouse=(type:string)=>s.view.webContents.emit('before-mouse-event',{preventDefault},{type,button:'right',x:55,y:66})
  mouse('mouseDown');mouse('mouseMove');mouse('contextMenu')
  s.view.webContents.emit('context-menu',{preventDefault});mouse('mouseUp')
  expect(preventDefault).toHaveBeenCalledTimes(4)
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(1)
  expect(s.owner.webContents.send).toHaveBeenLastCalledWith('office:menu-requested',{objectId:'document',revision:'version-1',expectedChangeSequence:0})
  mouse('mouseDown');mouse('mouseUp')
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(2)
  const count=s.owner.webContents.send.mock.calls.length
  for(const extra of [{isComposing:true},{isAutoRepeat:true},{control:true},{meta:true},{alt:true}])
    s.view.webContents.emit('before-input-event',{preventDefault},{type:'keyDown',key:'F10',shift:true,...extra})
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(count)
  s.view.webContents.emit('before-input-event',{preventDefault},{type:'keyDown',key:'F10',shift:true})
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(count+1)
  s.view.webContents.emit('before-input-event',{preventDefault},{type:'keyDown',key:'ContextMenu'})
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(count+2)
  s.view.webContents.emit('context-menu',{preventDefault});mouse('contextMenu')
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(count+2)
  expect(s.peer.peer.messages).toHaveLength(0)
})

test('opens the next right-press menu even if a previous release did not return to the surface',async()=>{
  const s=await setup();await openSurface(s)
  const preventDefault=vi.fn()
  for(const type of ['mouseDown','contextMenu','mouseDown','mouseUp'])
    s.view.webContents.emit('before-mouse-event',{preventDefault},{type,button:'right',x:0,y:0})
  expect(preventDefault).toHaveBeenCalledTimes(4)
  expect(s.owner.webContents.send).toHaveBeenCalledTimes(2)
  expect(s.peer.peer.messages).toHaveLength(0)
})

test('keeps plain left and middle input out of menu routing while suppressing secondary context events',async()=>{
  const s=await setup();await openSurface(s)
  for(const button of ['left','middle'])for(const type of ['mouseDown','mouseUp','mouseMove']){
    const preventDefault=vi.fn()
    s.view.webContents.emit('before-mouse-event',{preventDefault},{type,button,x:0,y:0})
    expect(preventDefault).not.toHaveBeenCalled()
  }
  const preventDefault=vi.fn()
  s.view.webContents.emit('before-mouse-event',{preventDefault},{type:'contextMenu',button:'right',x:0,y:0})
  s.view.webContents.emit('context-menu',{preventDefault})
  expect(preventDefault).toHaveBeenCalledTimes(2)
  expect(s.owner.webContents.send).not.toHaveBeenCalled();expect(s.peer.peer.messages).toHaveLength(0)
})

test.each(['native','owner'] as const)('observes macOS Control in %s contents even when mouse modifiers are absent, and consumes its matching release',async source=>{
  const s=await setup();await openSurface(s)
  const platform=Object.getOwnPropertyDescriptor(process,'platform')!
  try{
    Object.defineProperty(process,'platform',{value:'darwin'})
    const keyboard=source==='native'?s.view.webContents:s.owner.webContents
    keyboard.emit('before-input-event',{preventDefault:vi.fn()},{type:'keyDown',key:'Control',control:true})
    const down=vi.fn(),up=vi.fn()
    s.view.webContents.emit('before-mouse-event',{preventDefault:down},{type:'mouseDown',button:'left',x:0,y:0})
    keyboard.emit('before-input-event',{preventDefault:vi.fn()},{type:'keyUp',key:'Control',control:false})
    s.view.webContents.emit('before-mouse-event',{preventDefault:up},{type:'mouseUp',button:'left',x:0,y:0})
    expect(down).toHaveBeenCalledOnce();expect(up).toHaveBeenCalledOnce()
    expect(s.owner.webContents.send).toHaveBeenCalledExactlyOnceWith('office:menu-requested',{objectId:'document',revision:'version-1',expectedChangeSequence:0})
    const plain=vi.fn()
    s.view.webContents.emit('before-mouse-event',{preventDefault:plain},{type:'mouseDown',button:'left',x:0,y:0})
    expect(plain).not.toHaveBeenCalled();expect(s.owner.webContents.send).toHaveBeenCalledOnce()
    expect(s.peer.peer.messages).toHaveLength(0)
  }finally{Object.defineProperty(process,'platform',platform)}
})

test.each(['protected','pending'] as const)('consumes macOS Control-click while %s without opening a menu or mutating the engine',async stage=>{
  const s=await setup(stage==='protected');await openSurface(s)
  const platform=Object.getOwnPropertyDescriptor(process,'platform')!
  const closing=stage==='pending'?s.surface.request({...input('close'),expectedChangeSequence:0,discard:false}):undefined
  const request=closing?await sent(s.peer):undefined
  const before=s.peer.peer.messages.length
  try{
    Object.defineProperty(process,'platform',{value:'darwin'})
    s.owner.webContents.emit('before-input-event',{preventDefault:vi.fn()},{type:'keyDown',key:'Control',control:true})
    const preventDefault=vi.fn()
    for(const type of ['mouseDown','mouseUp'])s.view.webContents.emit('before-mouse-event',{preventDefault},{type,button:'left',x:0,y:0})
    expect(preventDefault).toHaveBeenCalledTimes(2)
    expect(s.owner.webContents.send).not.toHaveBeenCalled();expect(s.peer.peer.messages).toHaveLength(before)
  }finally{
    Object.defineProperty(process,'platform',platform)
    if(closing){cleanReply(s.peer,request,{});await closing}
  }
})

test('clears macOS Control on window blur or hide, ignores hidden keys, and removes owner observers on dispose',async()=>{
  const s=await setup();await openSurface(s)
  const platform=Object.getOwnPropertyDescriptor(process,'platform')!
  const control=()=>s.owner.webContents.emit('before-input-event',{preventDefault:vi.fn()},{type:'keyDown',key:'Control',control:true})
  const left=()=>s.view.webContents.emit('before-mouse-event',{preventDefault:vi.fn()},{type:'mouseDown',button:'left',x:0,y:0})
  try{
    Object.defineProperty(process,'platform',{value:'darwin'})
    control();s.owner.emit('blur');left();expect(s.owner.webContents.send).not.toHaveBeenCalled()
    control();s.surface.hide();control();await s.surface.attach(bounds);left()
    expect(s.owner.webContents.send).not.toHaveBeenCalled()
    expect(s.owner.webContents.listenerCount('before-input-event')).toBe(1)
    s.surface.destroy()
    expect(s.owner.webContents.listenerCount('before-input-event')).toBe(0)
    expect(s.owner.listenerCount('blur')).toBe(0)
    expect(s.peer.peer.messages).toHaveLength(0)
  }finally{Object.defineProperty(process,'platform',platform)}
})

test('does not treat Control-left as a menu on non-macOS platforms',async()=>{
  const s=await setup();await openSurface(s)
  const platform=Object.getOwnPropertyDescriptor(process,'platform')!
  try{
    Object.defineProperty(process,'platform',{value:'linux'})
    s.owner.webContents.emit('before-input-event',{preventDefault:vi.fn()},{type:'keyDown',key:'Control',control:true})
    const preventDefault=vi.fn()
    s.view.webContents.emit('before-mouse-event',{preventDefault},{type:'mouseDown',button:'left',x:0,y:0})
    expect(preventDefault).not.toHaveBeenCalled();expect(s.owner.webContents.send).not.toHaveBeenCalled()
  }finally{Object.defineProperty(process,'platform',platform)}
})

test('blocks mouse and keyboard menus without a current document, while hidden or closing, and after disposal',async()=>{
  const s=await setup()
  const route=()=>{
    const preventDefault=vi.fn()
    for(const type of ['mouseDown','mouseUp'])s.view.webContents.emit('before-mouse-event',{preventDefault},{type,button:'right',x:0,y:0})
    s.view.webContents.emit('before-input-event',{preventDefault},{type:'keyDown',key:'ContextMenu'})
  }
  route();expect(s.owner.webContents.send).not.toHaveBeenCalled()
  await openSurface(s);s.surface.hide();route();expect(s.owner.webContents.send).not.toHaveBeenCalled()
  await s.surface.attach(bounds)
  const close=s.surface.request({...input('close'),expectedChangeSequence:0,discard:false})
  const rejected=expect(close).rejects.toThrow('unsaved-changes'),r=await sent(s.peer)
  const before=s.peer.peer.messages.length;route()
  expect(s.owner.webContents.send).not.toHaveBeenCalled();expect(s.peer.peer.messages).toHaveLength(before)
  const {expectedChangeSequence,discard,...base}=r
  s.peer.postMessage({...base,type:'result',ok:false,error:'unsaved-changes'});await rejected
  route();expect(s.owner.webContents.send).toHaveBeenCalledTimes(2)
  s.owner.webContents.send.mockClear();s.owner.isDestroyed=()=>true;route()
  expect(s.owner.webContents.send).not.toHaveBeenCalled()
  s.owner.isDestroyed=()=>false;s.surface.destroy();route()
  expect(s.owner.webContents.send).not.toHaveBeenCalled()
})

test('does not route mouse or keyboard menus before the native document surface is ready',async()=>{
  const f=await fixture();cleanup.push(f.cleanup)
  const owner=Object.assign(new EventEmitter(),{isDestroyed:()=>false,webContents:Object.assign(new EventEmitter(),{getZoomFactor:()=>1,send:vi.fn()}),contentView:{addChildView:vi.fn(),removeChildView:vi.fn()}})
  let finishLoad!:()=>void
  mock.loadURL.mockImplementationOnce(()=>new Promise<void>(resolve=>{finishLoad=resolve}))
  const surface=new NativeOfficeSurface({...f,owner:owner as any,onEvent(){}});live.push(surface)
  const attaching=surface.attach(bounds)
  await vi.waitFor(()=>expect(finishLoad).toBeTypeOf('function'))
  const contents=mock.views.at(-1).webContents,preventDefault=vi.fn()
  for(const type of ['mouseDown','mouseUp'])contents.emit('before-mouse-event',{preventDefault},{type,button:'right',x:0,y:0})
  contents.emit('before-input-event',{preventDefault},{type:'keyDown',key:'ContextMenu'})
  expect(owner.webContents.send).not.toHaveBeenCalled()
  expect(preventDefault).toHaveBeenCalledTimes(2)
  finishLoad();await attaching
  expect(mock.peers.at(-1).peer.messages).toHaveLength(0)
})
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
  expect(logNativeOfficeFailure).toHaveBeenCalledWith('surface_request', 'office-operation-timeout-unknown', false)
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
  const owner = Object.assign(new EventEmitter(), { isDestroyed: () => false, webContents: Object.assign(new EventEmitter(), { getZoomFactor: (): number => 1, send: vi.fn() }), contentView: { addChildView: vi.fn(), removeChildView: vi.fn() } })
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


test('packaged surface accepts only an internal Core token and rechecks it before creating the isolated sink',async()=>{
  const f=await fixture();cleanup.push(f.cleanup)
  const buffers=await loadOfficeBuffers(f.sourceRoot,f.assetRoot)
  mock.privateLoad.mockResolvedValue({buffers,preloadPath:f.preloadPath})
  const resources='/Applications/analytix.app/Contents/Resources'
  const transport=vi.fn(async(_path:string,body:string)=>({ok:true,status:200,body:JSON.stringify({ok:true,requestId:JSON.parse(body).requestId,root:resources+'/office-private',qualificationDigest:'a'.repeat(64)})}))
  const token=await createOfficePrivateAdmissionProvider(transport,resources)()
  const owner=Object.assign(new EventEmitter(),{isDestroyed:()=>false,webContents:Object.assign(new EventEmitter(),{getZoomFactor:()=>1}),contentView:{addChildView:vi.fn(),removeChildView:vi.fn()}})
  mock.packaged=true
  expect(()=>new NativeOfficeSurface({owner:owner as any,privateAdmission:{...token} as any,onEvent(){}})).toThrow('office-source-experiment-only')
  const surface=new NativeOfficeSurface({owner:owner as any,privateAdmission:token,onEvent(){}});live.push(surface)
  await surface.attach(bounds)
  expect(transport).toHaveBeenCalledTimes(3)
  expect(mock.views.at(-1).options.webPreferences).toMatchObject({preload:f.preloadPath,sandbox:true,nodeIntegration:false,contextIsolation:true})
  expect(mock.views.at(-1).webContents.url).toMatch(/^http:\/\/127\.0\.0\.1:/)
})
test('a previously minted private token cannot create a view after Core admission disappears',async()=>{
  const resources='/Applications/analytix.app/Contents/Resources'
  const transport=vi.fn(async(_path:string,body:string)=>({ok:true,status:200,body:JSON.stringify({ok:true,requestId:JSON.parse(body).requestId,root:resources+'/office-private',qualificationDigest:'a'.repeat(64)})}))
  const token=await createOfficePrivateAdmissionProvider(transport,resources)()
  transport.mockResolvedValue({ok:false,status:503,body:JSON.stringify({ok:false,code:'unavailable'})})
  const owner=Object.assign(new EventEmitter(),{isDestroyed:()=>false})
  mock.packaged=true
  const surface=new NativeOfficeSurface({owner:owner as any,privateAdmission:token,onEvent(){}});live.push(surface)
  await expect(surface.attach(bounds)).rejects.toThrow('office-assets-unavailable')
  expect(mock.views).toHaveLength(0)
})


const previewView = (zoom=25) => ({zoom,zoomType:3,fit:false,page:null,pages:null,readOnly:true,modified:false})
test('readonly view operations remain correlated and nullable view events clear only the bound generation',async()=>{
  const s=await setup(),opened=await openSurface(s)
  const pending=s.surface.request({...input('local-view','preset'),action:'set-zoom',zoom:25})
  const sentControl=await sent(s.peer)
  s.peer.postMessage({type:'view',channel:opened.channel,operationId:'retired-open',documentId:'document',version:'version-1',view:previewView(50)})
  await new Promise(resolve=>setImmediate(resolve));expect(s.events).toEqual([])
  s.peer.postMessage({type:'view',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',view:previewView()})
  cleanReply(s.peer,sentControl,{state:state(),view:previewView()})
  expect(await pending).toMatchObject({ok:true,view:{zoom:25}})
  expect(s.events.at(-1)).toMatchObject({type:'view',view:{zoom:25}})
  s.peer.postMessage({type:'view',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',view:null})
  await new Promise(resolve=>setImmediate(resolve));expect(s.events.at(-1)).toMatchObject({type:'view',view:null})
  expect(s.view.webContents.isDestroyed()).toBe(false)
})
test('view codecs represent writable DOCX views and reject arbitrary actions, wrong kind pages and bounds',()=>{
  const control={...input('local-view'),channel:'channel',action:'set-zoom',zoom:25}
  expect(isOfficeRequest(control)).toBe(true)
  for(const extra of [{zoom:19},{zoom:401},{zoom:NaN},{zoom:25.5},{action:['fit']},{path:'/private'}]) expect(isOfficeRequest({...control,...extra})).toBe(false)
  const result={...input('local-view'),channel:'channel',type:'result',ok:true,state:state(),view:previewView()}
  expect(isOfficeResult(result)).toBe(true)
  expect(isOfficeResult({...result,view:{...previewView(),readOnly:false}})).toBe(true)
  expect(isOfficeResult({...result,state:{...state(),kind:'xlsx'},view:{...previewView(),readOnly:false}})).toBe(false)
  for(const view of [{...previewView(),page:1,pages:2},{...previewView(),zoom:Infinity}]) expect(isOfficeResult({...result,view})).toBe(false)
})

test('installs protected transfers before any bytes and rejects selection or mutation dispatch', async () => {
  const s = await setup(true), contents = s.view.webContents
  expect(contents.setIgnoreMenuShortcuts).toHaveBeenCalledWith(true)
  expect(contents.executeJavaScriptInIsolatedWorld).toHaveBeenCalledOnce()
  expect(contents.executeJavaScriptInIsolatedWorld.mock.calls[0][0]).toBe(1004)
  expect(s.peer.peer.messages).toHaveLength(0)
  const preventDefault = vi.fn()
  for (const key of ['c','x']) contents.emit('before-input-event',{preventDefault},{type:'keyDown',key,meta:true,isAutoRepeat:true})
  expect(preventDefault).toHaveBeenCalledTimes(2)
  await openSurface(s)
  for(const type of ['mouseDown','mouseUp'])contents.emit('before-mouse-event',{preventDefault},{type,button:'right',x:0,y:0})
  contents.emit('context-menu',{preventDefault})
  contents.emit('before-input-event',{preventDefault},{type:'keyDown',key:'ContextMenu'})
  expect(preventDefault).toHaveBeenCalledTimes(6)
  expect(s.owner.webContents.send).not.toHaveBeenCalled()
  for (const command of ['captureSelection','edit','export']) await expect(s.surface.request(input(command,'protected-'+command))).rejects.toThrow('office-invalid-request')
  expect(s.peer.peer.messages).toHaveLength(0)
  const {runInNewContext} = await import('node:vm'), listeners = new Map<string, (event:any) => void>()
  const prevent = vi.fn(), stop = vi.fn(), clear = vi.fn()
  class FakeEvent {preventDefault() {prevent()};stopImmediatePropagation() {stop()}}
  class FakeTransfer {clearData() {clear()}}
  runInNewContext(contents.executeJavaScriptInIsolatedWorld.mock.calls[0][1][0].code,{Event:FakeEvent,DataTransfer:FakeTransfer,window:{addEventListener:(type:string,listener:any) => listeners.set(type,listener)}})
  for (const type of ['copy','cut','dragstart']) listeners.get(type)!({clipboardData:new FakeTransfer(),dataTransfer:new FakeTransfer()})
  expect(prevent).toHaveBeenCalledTimes(3);expect(stop).toHaveBeenCalledTimes(3);expect(clear).toHaveBeenCalledTimes(3)
})
test('ordinary Office does not install the Present transfer guard', async () => {
  const s = await setup();expect(s.view.webContents.setIgnoreMenuShortcuts).not.toHaveBeenCalled();expect(s.view.webContents.executeJavaScriptInIsolatedWorld).not.toHaveBeenCalled()
})

test('logs the load stage once without retaining the underlying exception', async () => {
  mock.loadURL.mockRejectedValueOnce(Error('/private/office/engine-body-sentinel'))
  await expect(setup(true)).rejects.toThrow('office-startup-failed')
  expect(logNativeOfficeFailure).toHaveBeenCalledExactlyOnceWith('surface_load', 'office-startup-failed', true)
  expect(JSON.stringify(vi.mocked(logNativeOfficeFailure).mock.calls)).not.toContain('/private/office/engine-body-sentinel')
  expect(mock.views[0].webContents.dead).toBe(true)
})
test('a settled nonfatal request returns diagnostics to the idle stage', async () => {
  const s = await setup(); await openSurface(s)
  const pending = s.surface.request(input('captureSelection')), rejected = expect(pending).rejects.toThrow('engine-operation-failed')
  const request = await sent(s.peer)
  s.peer.postMessage({ ...request, type: 'result', ok: false, error: 'engine-operation-failed' })
  await rejected
  s.view.webContents.emit('render-process-gone')
  expect(logNativeOfficeFailure).toHaveBeenCalledExactlyOnceWith('surface_ready', 'office-engine-closed-unknown', false)
})

test('preserves the closed engine failure class in diagnostics', async () => {
  const s = await setup(), opened = await openSurface(s)
  s.peer.postMessage({ type: 'fatal', channel: opened.channel, error: 'engine-operation-failed' })
  await new Promise(resolve => setImmediate(resolve))
  expect(logNativeOfficeFailure).toHaveBeenCalledExactlyOnceWith('surface_ready', 'engine-operation-failed', false)
  expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-engine-closed-unknown' })
})

test('pointer actions require a current visible DOCX native token and cancel on hide, replacement selection or lifecycle changes',async()=>{
  const s=await setup(),opened=await openSurface(s)
  const native={...selection(),token:'selection_token',text:'same text'}
  const lease={channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',expectedChangeSequence:0,interactionId:'interaction_id',selectionToken:native.token}
  const message=(phase:string,extra={})=>({...lease,type:'pointer-actions',phase,...extra})
  s.peer.postMessage({type:'selection',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',selection:native})
  await vi.waitFor(()=>expect(s.events.at(-1)?.type).toBe('selection'))
  s.peer.postMessage(message('request'));await vi.waitFor(()=>expect(s.owner.webContents.send).toHaveBeenCalledWith('office:menu-requested',expect.objectContaining({pointer:{interactionId:lease.interactionId,selectionToken:native.token}})))
  const target:any={objectId:'document',revision:'version-1',expectedChangeSequence:0,pointer:{interactionId:lease.interactionId,selectionToken:native.token},actions:[{id:'quote',label:'Quote',enabled:true},{id:'copy',label:'Copy',enabled:false}]}
  const shown=s.surface.presentPointerActions(target)
  expect(s.surface.isPointerCurrent(target.pointer)).toBe(true)
  s.peer.postMessage(message('choice',{actionId:'quote'}));expect(await shown).toBe('quote')
  expect(s.surface.isPointerCurrent(target.pointer)).toBe(true)
  s.surface.hide();expect(s.surface.isPointerCurrent(target.pointer)).toBe(false)
  s.peer.postMessage(message('request'));await new Promise(resolve=>setImmediate(resolve))
  expect(await s.surface.presentPointerActions(target)).toBeNull()
  await s.surface.attach(bounds)
  s.peer.postMessage(message('request'));await vi.waitFor(()=>expect(s.surface.isPointerCurrent(target.pointer)).toBe(true))
  const pending=s.surface.presentPointerActions(target)
  s.peer.postMessage(message('choice',{actionId:'copy'}));expect(await pending).toBeNull()
  s.peer.postMessage(message('request'));await vi.waitFor(()=>expect(s.surface.isPointerCurrent(target.pointer)).toBe(true))
  const replaced=s.surface.presentPointerActions(target)
  s.peer.postMessage({type:'selection',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',selection:{...native,token:'different_token'}})
  expect(await replaced).toBeNull();expect(s.surface.isPointerCurrent(target.pointer)).toBe(false)
  // The next request uses a matching public selection event, without private lease fields.
  s.peer.postMessage({type:'selection',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',selection:native})
  await vi.waitFor(()=>expect(s.events.at(-1)?.selection?.token).toBe(native.token))
  s.peer.postMessage(message('request'));await vi.waitFor(()=>expect(s.surface.isPointerCurrent(target.pointer)).toBe(true))
  const keyboard=s.surface.presentPointerActions(target)
  s.view.webContents.emit('before-input-event',{preventDefault:vi.fn()},{type:'keyDown',key:'F10',shift:true})
  expect(await keyboard).toBeNull();expect(s.owner.webContents.send).toHaveBeenLastCalledWith('office:menu-requested',{objectId:'document',revision:'version-1',expectedChangeSequence:0})
  s.surface.destroy()
})
test('protected surfaces reject pointer requests and validation before any worker dispatch',async()=>{
  const s=await setup(true),opened=await openSurface(s)
  s.peer.postMessage({type:'pointer-actions',phase:'request',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',expectedChangeSequence:0,interactionId:'interaction_id',selectionToken:'selection_token'})
  await new Promise(resolve=>setImmediate(resolve));expect(s.owner.webContents.send).not.toHaveBeenCalled()
  await expect(s.surface.request({...input('validateSelection'),selectionToken:'selection_token',expectedChangeSequence:0})).rejects.toThrow('office-invalid-request')
  expect(s.peer.peer.messages).toHaveLength(0)
})


const manualBaseRevision = 'a'.repeat(64), manualSavedRevision = 'c'.repeat(64)
const manualAuthority = { version: 2, origin: 'manual', changeId: 'b'.repeat(64), baseRevision: manualBaseRevision, saveOperationId: 'manual_save_0001', undoOperationId: 'manual_undo_0001' }
const manualState = (sequence = 0, frozen = false) => ({ ...state(sequence, 0, manualBaseRevision), manual: { version: 2, origin: 'manual', changeId: manualAuthority.changeId, baseRevision: manualBaseRevision, generation: 1, frozen } })
const manualInput = (command: string, operationId: string) => ({ ...input(command, operationId), version: manualBaseRevision })
// Build a response envelope explicitly: manual authority, leases and save IDs are request-only fields.
function manualReply(peer: any, request: any, payload: any) {
  const { command, operationId, documentId, version, channel } = request
  peer.postMessage({ command, operationId, documentId, version, channel, type: 'result', ok: true, ...payload })
}
async function manualSurface(protectedPresent?: boolean) {
  const s = await setup(protectedPresent)
  const pending = s.surface.request({ ...manualInput('open', 'manual_open_0001'), kind: 'docx', bytes: new Uint8Array([1, 2, 3]) })
  const opened = await sent(s.peer)
  manualReply(s.peer, opened, { state: state(0, 0, manualBaseRevision), selection: selection(0, manualBaseRevision) }); await pending
  s.peer.peer.messages.length = 0
  return { ...s, opened }
}
async function authorizeNativeManual(s: any) {
  const pending = s.surface.request({ ...manualInput('manual-edit', 'manual_begin_0001'), manual: manualAuthority })
  const request = await sent(s.peer)
  manualReply(s.peer, request, { state: manualState(), selection: selection(0, manualBaseRevision) }); await pending
  s.peer.peer.messages.length = 0
}
async function exportManualChange(s: any) {
  s.peer.postMessage({ type: 'changed', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, state: manualState(1) })
  await vi.waitFor(() => expect(s.events.at(-1)).toMatchObject({ type: 'changed', state: { changeSequence: 1 } }))
  const freezing = s.surface.request({ ...manualInput('freeze', 'manual_freeze_0001'), changeId: manualAuthority.changeId })
  const freezeRequest = await sent(s.peer), freezeAck = { changeId: manualAuthority.changeId, operationId: freezeRequest.operationId, generation: 1, sequence: 1 }
  manualReply(s.peer, freezeRequest, { state: manualState(1, true), freezeAck }); await freezing
  s.peer.peer.messages.length = 0
  const exporting = s.surface.request({ ...manualInput('export', manualAuthority.saveOperationId), changeId: manualAuthority.changeId, freezeAck })
  const exportRequest = await sent(s.peer)
  manualReply(s.peer, exportRequest, { state: manualState(1, true), bytes: new Uint8Array([4, 5, 6]), exportedSequence: 1 }); await exporting
  s.peer.peer.messages.length = 0
}

test.each([false, true])('writable native views are rejected before manual authority, protected=%s', async protectedPresent => {
  const s = await manualSurface(protectedPresent)
  if (protectedPresent) {
    await expect(s.surface.request({ ...manualInput('manual-edit', 'manual_begin_0001'), manual: manualAuthority })).rejects.toThrow('office-invalid-request')
    expect(s.peer.peer.messages).toHaveLength(0)
  }
  s.peer.postMessage({ type: 'view', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, view: { ...previewView(), readOnly: false } })
  await vi.waitFor(() => expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-protocol-invalid' }))
  expect(s.view.webContents.dead).toBe(true)
})

test('only a correlated matching manual acknowledgement authorizes native DOCX input', async () => {
  const s = await manualSurface()
  let settled = false
  const pending = s.surface.request({ ...manualInput('manual-edit', 'manual_begin_0001'), manual: manualAuthority }).then(result => { settled = true; return result })
  const request = await sent(s.peer), payload = { state: manualState(), selection: selection(0, manualBaseRevision) }
  manualReply(s.peer, { ...request, operationId: 'foreign_begin_0001' }, payload)
  await new Promise(resolve => setImmediate(resolve)); expect(settled).toBe(false); expect(s.events).toEqual([])
  manualReply(s.peer, request, payload); expect(await pending).toMatchObject({ state: { manual: { changeId: manualAuthority.changeId, generation: 1 } } })
  s.peer.postMessage({ type: 'view', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, view: { ...previewView(), readOnly: false } })
  await vi.waitFor(() => expect(s.events.at(-1)).toMatchObject({ type: 'view', view: { readOnly: false } }))
  expect(s.view.webContents.dead).toBe(false)
})

test('manual acknowledgement cannot substitute a different Core change authority', async () => {
  const s = await manualSurface()
  const pending = s.surface.request({ ...manualInput('manual-edit', 'manual_begin_0001'), manual: manualAuthority })
  const rejected = expect(pending).rejects.toThrow('office-protocol-invalid'), request = await sent(s.peer), next = manualState()
  manualReply(s.peer, request, { state: { ...next, manual: { ...next.manual, changeId: 'd'.repeat(64) } }, selection: selection(0, manualBaseRevision) })
  await rejected
  expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-protocol-invalid' }); expect(s.view.webContents.dead).toBe(true)
})

test('exact committed acknowledgement ends manual authority and rejects subsequent unapproved dirty input', async () => {
  const s = await manualSurface(); await authorizeNativeManual(s); await exportManualChange(s)
  const committing = s.surface.request({ ...manualInput('ack', 'manual_ack_0001'), status: 'committed', persistedVersion: manualSavedRevision, exportOperationId: manualAuthority.saveOperationId, exportedSequence: 1 })
  const request = await sent(s.peer)
  manualReply(s.peer, request, { state: state(1, 1, manualSavedRevision), selection: selection(1, manualSavedRevision) })
  expect(await committing).toMatchObject({ state: { changeSequence: 1, acknowledgedSequence: 1, dirty: false } })
  s.peer.peer.messages.length = 0; s.events.length = 0
  await expect(s.surface.request({ ...input('export', 'unapproved_export_0001'), version: manualSavedRevision })).rejects.toThrow('office-invalid-request')
  expect(s.peer.peer.messages).toHaveLength(0)
  // The native model can remain writable; the browser input guard requires a new Core transaction.
  s.peer.postMessage({ type: 'view', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualSavedRevision, view: { ...previewView(), readOnly: false } })
  await vi.waitFor(() => expect(s.events.at(-1)).toMatchObject({ type: 'view', view: { readOnly: false, modified: false } }))
  s.peer.postMessage({ type: 'changed', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualSavedRevision, state: state(2, 1, manualSavedRevision) })
  await vi.waitFor(() => expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-protocol-invalid' }))
  expect(s.view.webContents.dead).toBe(true)
})

test.each([[1, 0], [2, 2], [2, 1]])('committed manual ack rejects sequence %i acknowledged %i instead of the exact exported sequence', async (sequence, acknowledged) => {
  const s = await manualSurface(); await authorizeNativeManual(s); await exportManualChange(s)
  const committing = s.surface.request({ ...manualInput('ack', 'manual_ack_0001'), status: 'committed', persistedVersion: manualSavedRevision, exportOperationId: manualAuthority.saveOperationId, exportedSequence: 1 })
  const rejected = expect(committing).rejects.toThrow('office-protocol-invalid'), request = await sent(s.peer)
  manualReply(s.peer, request, { state: state(sequence, acknowledged, manualSavedRevision), selection: selection(sequence, manualSavedRevision) })
  await rejected
  expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-protocol-invalid' }); expect(s.view.webContents.dead).toBe(true)
})

const pointerFormatState = { fontFamily: 'Aptos', fontSize: 12, bold: null, italic: false, underline: false }
const pointerFormatLabels = { fontFamily: 'Font', fontSize: 'Size', bold: 'Bold', italic: 'Italic', underline: 'Underline', mixed: 'Mixed' }
const pointerFormats = [
  { kind: 'font-family', value: 'Noto Sans CJK SC' }, { kind: 'font-size', value: 16.5 },
  { kind: 'bold', value: true }, { kind: 'italic', value: false }, { kind: 'underline', value: true }
] as const
async function formattingPointerSurface(manual = true, protectedPresent = false, requestPointer = true) {
  const s = await manualSurface(protectedPresent)
  if (manual && !protectedPresent) await authorizeNativeManual(s)
  const rangeLease = { generation: 1, sequence: 0, token: 'selection_token' }
  const native = { ...selection(0, manualBaseRevision), token: rangeLease.token, text: 'selected text', rangeLease, formatState: pointerFormatState }
  const lease = { channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision,
    expectedChangeSequence: 0, interactionId: 'format_interaction', selectionToken: native.token }
  const message = (phase: string, extra = {}) => ({ ...lease, type: 'pointer-actions', phase, ...extra })
  if (!protectedPresent) {
    s.peer.postMessage({ type: 'selection', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, selection: native })
    await vi.waitFor(() => expect(s.events.at(-1)?.selection?.token).toBe(native.token))
  }
  if (requestPointer) s.peer.postMessage(message('request'))
  await new Promise(resolve => setImmediate(resolve))
  const target = { objectId: 'document', revision: manualBaseRevision, expectedChangeSequence: 0,
    pointer: { interactionId: lease.interactionId, selectionToken: native.token }, actions: [{ id: 'quote', label: 'Quote', enabled: true }],
    formatting: { changeId: manualAuthority.changeId, rangeLease, formatState: pointerFormatState, labels: pointerFormatLabels } }
  return { ...s, target, message, native }
}

const { JSDOM } = createRequire(import.meta.url)('jsdom')
async function formattingPointerGuest() {
  const s = await formattingPointerSurface(true, false, false)
  const dom = new JSDOM('<main id="pointerRoot"><canvas id="pointerCanvas" tabindex="0"></canvas></main>', { url: 'https://office.invalid/', runScripts: 'outside-only', pretendToBeVisual: true })
  cleanup.push(async () => dom.window.close())
  const source = readFileSync(new URL('./surface/office-surface.js', import.meta.url), 'utf8')
  // No qtcanvas exists: evaluate the real toolbar without installing or starting the engine.
  const Toolbar = new dom.window.Function(source.replaceAll('export ', '') + '\nreturn OfficePointerToolbar;')()
  const guestValue = (value: unknown) => dom.window.JSON.parse(JSON.stringify(value))
  let interaction = 0, operation = 0
  Object.defineProperty(dom.window.crypto, 'randomUUID', { value: () => `synthetic_interaction_${++interaction}` })
  const canvas = dom.window.document.getElementById('pointerCanvas'), root = dom.window.document.getElementById('pointerRoot')
  const engine: any = { channel: s.opened.channel, openOperationId: s.opened.operationId, state: manualState(), failed: false,
    nativeWritable: true, editing: true, inputGate: false, inputPending: 0, busy: false, composition: null, view: null,
    fontFamilies: ['Aptos', 'Noto Sans CJK SC'], manualInput: { changeId: manualAuthority.changeId, generation: 1 } }
  engine.manualAvailable = () => !engine.failed && engine.nativeWritable && engine.state.manual?.frozen === false &&
    engine.manualInput?.changeId === engine.state.manual?.changeId && engine.manualInput?.generation === engine.state.manual?.generation
  // The lightweight mock port has no structured clone; retain each real boundary's receiving realm.
  const toolbar = new Toolbar(canvas, root, (message: any) => s.peer.postMessage(JSON.parse(JSON.stringify(message))), () => engine)
  const delivered: any[] = []
  s.peer.on('message', ({ data }: any) => { if (data.type === 'pointer-actions') { delivered.push(data); toolbar.receive(guestValue(data)) } })
  s.view.webContents.on('destroyed', () => dom.window.dispatchEvent(new dom.window.Event('pagehide')))
  const settle = () => new Promise<void>(resolve => setImmediate(resolve))
  const sample = async (native: any = s.native) => {
    const event = guestValue({ type: 'selection', channel: engine.channel, operationId: engine.openOperationId,
      documentId: engine.state.documentId, version: engine.state.version, selection: native })
    // Match the guest's actual event callback: toolbar first, then its typed Main bridge.
    toolbar.selection(event); s.peer.postMessage(JSON.parse(JSON.stringify(event))); await settle()
  }
  const pointer = { isTrusted: true, isPrimary: true, button: 0, pointerId: 1, target: canvas, clientX: 120, clientY: 90 }
  toolbar.pointerDown(pointer); await sample()
  // This worker confirmation is guest-private; it must never be posted to Main.
  toolbar.confirm({ selectionToken: s.native.token, eligible: true, changed: true }); toolbar.pointerUp(pointer)
  const target = { ...s.target, pointer: { interactionId: toolbar.lease.interactionId, selectionToken: toolbar.lease.selectionToken } }
  await vi.waitFor(() => expect(s.surface.isPointerCurrent(target.pointer)).toBe(true))
  const begin = async (command: string, extra = {}) => {
    const operationId = `stable_sample_${++operation}`, start = s.peer.peer.messages.length
    const pending = s.surface.request({ ...manualInput(command, operationId), ...extra })
    let request: any
    await vi.waitFor(() => { request = s.peer.peer.messages.slice(start).find((message: any) => message.operationId === operationId && message.command === command); expect(request).toBeDefined() })
    return { pending, request }
  }
  const control = (kind: string) => toolbar.bar.querySelector(`[data-format-control="${kind}"]`)
  const present = async () => {
    const pending = s.surface.presentPointerActions(target)
    await vi.waitFor(() => expect(toolbar.bar.hidden).toBe(false))
    expect(control('font-family').value).toBe('Aptos')
    expect(control('font-size').value).toBe('12')
    expect(control('bold').getAttribute('aria-pressed')).toBe('mixed')
    expect(toolbar.bar.querySelector('[data-pointer-action="quote"]').disabled).toBe(false)
    return { pending }
  }
  return { ...s, dom, engine, toolbar, target, sample, begin, present, control, delivered, settle, guestValue }
}

test.each(['requested', 'visible'] as const)('stable native selection survives Main sampling and keeps the actual guest toolbar %s', async stage => {
  const s = await formattingPointerGuest()
  let offered: { pending: Promise<unknown> } | undefined
  if (stage === 'visible') offered = await s.present()
  const bold = s.control('bold'), lease = structuredClone(s.toolbar.lease)
  const retained = () => {
    expect(s.surface.isPointerCurrent(s.target.pointer)).toBe(true)
    expect(s.toolbar.lease).toEqual(lease)
    expect(s.toolbar.bar.hidden).toBe(stage !== 'visible')
    if (stage === 'visible') expect(s.control('bold')).toBe(bold)
    expect(s.delivered.filter(message => message.phase === 'dismiss')).toEqual([])
  }
  await s.sample(); retained()
  const capturing = await s.begin('captureSelection')
  manualReply(s.peer, capturing.request, { state: manualState(), selection: structuredClone(s.native) })
  await capturing.pending; await s.settle(); retained()
  const validating = await s.begin('validateSelection', { selectionToken: s.native.token, expectedChangeSequence: 0 })
  s.engine.busy = true; s.engine.inputGate = true
  await s.sample()
  s.toolbar.confirm({ selectionToken: s.native.token, eligible: true, changed: false }); retained()
  manualReply(s.peer, validating.request, { state: manualState() }); await validating.pending
  s.engine.busy = false; s.engine.inputGate = false
  if (!offered) offered = await s.present()
  expect(s.control('bold').getAttribute('aria-pressed')).toBe('mixed')
  s.toolbar.chooseFormat({ isTrusted: true }, s.guestValue({ kind: 'bold', value: true }))
  expect(s.toolbar.bar.getAttribute('aria-busy')).toBe('true')
  expect(s.control('bold').disabled).toBe(true)
  expect(await offered.pending).toEqual({ actionId: null, changeId: manualAuthority.changeId, rangeLease: s.target.formatting.rangeLease, format: { kind: 'bold', value: true } })
  expect(s.peer.messages.some((message: any) => message.type === 'selection-confirmed')).toBe(false)
  expect(s.peer.peer.messages.some((message: any) => message.command === 'format')).toBe(false)
})

const pointerRetirements = ['format-state', 'token', 'empty', 'generation', 'sequence', 'version', 'freeze', 'revoke'] as const
test.each(pointerRetirements.flatMap(reason => (['requested', 'visible'] as const).map(stage => ({ reason, stage }))))('Main and the actual guest retire $stage formatting on $reason', async ({ reason, stage }) => {
  const s = await formattingPointerGuest()
  const offered = stage === 'visible' ? await s.present() : undefined
  if (reason === 'format-state') await s.sample({ ...s.native, formatState: { ...pointerFormatState, bold: true } })
  else if (reason === 'token') await s.sample({ ...s.native, token: 'selection_next', rangeLease: { ...s.native.rangeLease, token: 'selection_next' } })
  else if (reason === 'empty') {
    const { token: _token, rangeLease: _rangeLease, formatState: _formatState, ...empty } = s.native
    await s.sample({ ...empty, text: '' } as any)
  } else if (reason === 'version') {
    const committing = await s.begin('ack', { status: 'committed', persistedVersion: manualSavedRevision, exportOperationId: manualAuthority.saveOperationId, exportedSequence: 0 })
    s.engine.state = state(0, 0, manualSavedRevision); s.engine.manualInput = null
    manualReply(s.peer, committing.request, { state: s.engine.state, selection: selection(0, manualSavedRevision) })
    await committing.pending; await s.settle()
  } else {
    const next: any = reason === 'revoke' ? state(0, 0, manualBaseRevision) : manualState(reason === 'sequence' ? 1 : 0, reason === 'freeze')
    if (reason === 'generation') next.manual.generation++
    s.engine.state = next
    s.toolbar.changed()
    s.peer.postMessage({ type: 'changed', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, state: next })
    await s.settle()
    if (reason === 'generation' || reason === 'revoke') {
      expect(s.events.at(-1)).toEqual({ type: 'fatal', code: 'office-protocol-invalid' })
      expect(s.view.webContents.dead).toBe(true)
    }
  }
  if (offered) expect(await offered.pending).toBeNull()
  expect(s.toolbar.bar.hidden).toBe(true)
  expect(s.toolbar.bar.querySelector('[data-format-control]')).toBeNull()
  expect(s.surface.isPointerCurrent(s.target.pointer)).toBe(false)
  expect(await s.surface.presentPointerActions(s.target)).toBeNull()
  await s.settle(); expect(s.toolbar.bar.hidden).toBe(true)
  expect(s.peer.peer.messages.some((message: any) => message.command === 'format')).toBe(false)
})

test.each((['event', 'capture-result'] as const).flatMap(route => (['format-state', 'generation', 'empty', 'owner-unavailable'] as const).map(reason => ({ route, reason }))))('Main independently dismisses $route pointer samples on $reason without a guest cancel', async ({ route, reason }) => {
  const s = await formattingPointerSurface(), offered = s.surface.presentPointerActions(s.target)
  const start = s.peer.peer.messages.length
  let native: any = structuredClone(s.native)
  if (reason === 'format-state') native.formatState.bold = true
  else if (reason === 'generation') native.rangeLease.generation++
  else if (reason === 'empty') native = selection(0, manualBaseRevision)
  else s.owner.isDestroyed = () => true
  if (route === 'event') {
    const event = { type: 'selection', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, selection: native }
    expect(isOfficeEvent(event)).toBe(true); s.peer.postMessage(event)
  } else {
    const pending = s.surface.request(manualInput('captureSelection', 'independent_sample_0001'))
    await vi.waitFor(() => expect(s.peer.peer.messages.slice(start).some((message: any) => message.command === 'captureSelection')).toBe(true))
    const request = s.peer.peer.messages.slice(start).find((message: any) => message.command === 'captureSelection')
    const result = { ...request, type: 'result', ok: true, state: manualState(), selection: native }
    expect(isOfficeResult(result)).toBe(true); s.peer.postMessage(result); await pending
  }
  expect(await offered).toBeNull()
  expect(s.surface.isPointerCurrent(s.target.pointer)).toBe(false)
  expect(s.peer.peer.messages.slice(start)).toContainEqual(s.message('dismiss'))
  expect(s.peer.messages.some((message: any) => message.type === 'pointer-actions' && message.phase === 'cancel')).toBe(false)
  expect(s.events.some((event: any) => event.type === 'fatal')).toBe(false)
})

test.each(pointerFormats)('returns the typed pointer $kind choice only for its offered native manual lease', async format => {
  const s = await formattingPointerSurface()
  const pending = s.surface.presentPointerActions(s.target)
  expect(s.peer.peer.messages.at(-1)).toEqual(s.message('present', { actions: s.target.actions, formatting: s.target.formatting }))
  s.peer.postMessage(s.message('format', { changeId: manualAuthority.changeId, rangeLease: s.target.formatting.rangeLease, format }))
  expect(await pending).toEqual({ actionId: null, changeId: manualAuthority.changeId, rangeLease: s.target.formatting.rangeLease, format })
  // The pointer relay returns intent; only the controller can issue a native format mutation.
  expect(s.peer.peer.messages.some((request: any) => request.command === 'format')).toBe(false)
})

test.each(['change', 'generation', 'sequence', 'token', 'state'] as const)('refuses a pointer format offer with mismatched native %s', async mismatch => {
  const s = await formattingPointerSurface(), target: any = structuredClone(s.target)
  if (mismatch === 'change') target.formatting.changeId = 'd'.repeat(64)
  if (mismatch === 'generation') target.formatting.rangeLease.generation++
  if (mismatch === 'sequence') target.formatting.rangeLease.sequence++
  if (mismatch === 'token') target.formatting.rangeLease.token = 'different_token'
  if (mismatch === 'state') target.formatting.formatState = { ...pointerFormatState, bold: true }
  expect(await s.surface.presentPointerActions(target)).toBeNull()
  expect(s.peer.peer.messages.some((request: any) => request.phase === 'present')).toBe(false)
})

test.each(['change', 'generation', 'sequence', 'token', 'open-command'] as const)('a forged guest format %s never becomes a native mutation intent', async mismatch => {
  const s = await formattingPointerSurface(), pending = s.surface.presentPointerActions(s.target)
  const choice: any = { changeId: manualAuthority.changeId, rangeLease: { ...s.target.formatting.rangeLease }, format: { kind: 'bold', value: true } }
  if (mismatch === 'change') choice.changeId = 'd'.repeat(64)
  if (mismatch === 'generation') choice.rangeLease.generation++
  if (mismatch === 'sequence') choice.rangeLease.sequence++
  if (mismatch === 'token') choice.rangeLease.token = 'different_token'
  if (mismatch === 'open-command') choice.format = { kind: 'uno', value: '.uno:Save' }
  s.peer.postMessage(s.message('format', choice))
  expect(await pending).toBeNull()
  expect(s.peer.peer.messages.some((request: any) => request.command === 'format')).toBe(false)
})

test('a readonly pointer keeps its legacy quote string result while refusing formatting authority', async () => {
  const s = await formattingPointerSurface(false)
  expect(await s.surface.presentPointerActions(s.target)).toBeNull()
  const { formatting: _formatting, ...quoteOnly } = s.target
  s.peer.postMessage(s.message('request'))
  await vi.waitFor(() => expect(s.surface.isPointerCurrent(quoteOnly.pointer)).toBe(true))
  const pending = s.surface.presentPointerActions(quoteOnly)
  s.peer.postMessage(s.message('choice', { actionId: 'quote' }))
  expect(await pending).toBe('quote')
  expect(s.peer.peer.messages.some((request: any) => request.formatting)).toBe(false)
})

test('freezing an offered pointer formatting transaction retires its choice before export', async () => {
  const s = await formattingPointerSurface(), pending = s.surface.presentPointerActions(s.target)
  s.peer.postMessage({ type: 'changed', channel: s.opened.channel, operationId: s.opened.operationId, documentId: 'document', version: manualBaseRevision, state: manualState(0, true) })
  expect(await pending).toBeNull()
  s.peer.postMessage(s.message('format', { changeId: manualAuthority.changeId, rangeLease: s.target.formatting.rangeLease, format: { kind: 'bold', value: true } }))
  await new Promise(resolve => setImmediate(resolve))
  expect(await s.surface.presentPointerActions(s.target)).toBeNull()
  expect(s.peer.peer.messages.some((request: any) => request.command === 'format')).toBe(false)
})

test('a protected native surface never presents or returns pointer formatting', async () => {
  const s = await formattingPointerSurface(false, true)
  expect(await s.surface.presentPointerActions(s.target)).toBeNull()
  expect(s.peer.peer.messages.some((request: any) => request.phase === 'present')).toBe(false)
  expect(s.owner.webContents.send).not.toHaveBeenCalledWith('office:menu-requested', expect.anything())
})

const workbookCell={sheet:0,column:0,row:0,text:'1',formula:'1',value:1,valueType:'number',numberFormat:7,rowVisible:true,columnVisible:true,merged:false}
const workbookSnapshot={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0,cells:[workbookCell]}
const workbookSelection={documentId:'document',version:'version-1',changeSequence:0,kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',token:'selection_cell_01',
  ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}],cells:[workbookCell],text:'1',capture:{capturedCharacters:1,totalCharacters:1,truncated:false,unit:'utf-16',complete:true}}

test('typed workbook dependency transport keeps read snapshots separate from selection and write authority',()=>{
  const base={...input('captureCells'),channel:'channel'},bounds={sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:0}
  expect(isOfficeRequest({...base,selectionToken:'selection_cell_01',expectedChangeSequence:0,bounds})).toBe(true)
  expect(isOfficeRequest({...base,selectionToken:'selection_cell_01',expectedChangeSequence:0,bounds:{...bounds,endRow:256}})).toBe(false)
  const result={...base,type:'result',ok:true,state:{...state(),kind:'xlsx'},workbook:workbookSnapshot}
  expect(isOfficeResult(result)).toBe(true)
  expect(isOfficeResult({...result,selection:workbookSelection})).toBe(false)
  expect(isOfficeResult({...result,workbook:{...workbookSnapshot,cells:[{...workbookCell,valueType:'formula',formula:'=1/0',valueError:true}]}})).toBe(false)
  const replace={...base,version:'a'.repeat(64),command:'replaceCells',selectionToken:'selection_cell_01',expectedChangeSequence:0,workbook:{before:workbookSnapshot,after:workbookSnapshot,results:['']},writeTarget:{sheet:0,column:0,row:0},candidate:{changeId:'b'.repeat(64),saveOperationId:'save_workbook_01',baseRevision:'a'.repeat(64)}}
  expect(isOfficeRequest(replace)).toBe(true)
  expect(isOfficeRequest({...replace,writeTarget:{...replace.writeTarget,path:'/tmp/other'}})).toBe(false)
  const {candidate:_,...unbound}=replace
  expect(isOfficeRequest(unbound)).toBe(false)
  expect(isOfficeRequest({...replace,candidate:{...replace.candidate,baseRevision:'c'.repeat(64)}})).toBe(false)
  expect(isOfficeRequest({...replace,candidate:{...replace.candidate,path:'/tmp/other'}})).toBe(false)
  for(const command of ['replaceCells','ack']) {
    const changed={...base,command,type:'result',ok:true,state:{...state(),kind:'xlsx'}}
    expect(isOfficeResult(changed)).toBe(true)
    expect(isOfficeResult({...changed,selection:workbookSelection})).toBe(false)
    expect(isOfficeResult({...changed,state:state()})).toBe(false)
  }
})

test('displayed workbook formula errors and bounded edit intent have explicit closed protocol fields',()=>{
  const event={type:'cell-edit-requested',intent:'f2',channel:'channel',operationId:'opened',documentId:'document',version:'version-1',selection:workbookSelection}
  expect(isOfficeEvent(event)).toBe(true)
  expect(isOfficeEvent({...event,intent:'paste'})).toBe(false)
  const errorSelection={...workbookSelection,cells:[{...workbookCell,valueType:'formula',formula:'=1/0',text:'#DIV/0!',value:0,valueError:true}]}
  expect(isOfficeEvent({...event,type:'selection',intent:undefined,selection:errorSelection})).toBe(false)
  const {intent:_,...selectionEvent}=event
  expect(isOfficeEvent({...selectionEvent,type:'selection',selection:errorSelection})).toBe(true)
  expect(isOfficeEvent({...selectionEvent,type:'selection',selection:{...errorSelection,cells:[{...errorSelection.cells[0],valueError:1}]}})).toBe(false)
  for(const formulaResultType of ['number','text'])expect(isOfficeEvent({...selectionEvent,type:'selection',selection:{...errorSelection,cells:[{...errorSelection.cells[0],valueError:false,formulaResultType}]}})).toBe(true)
  expect(isOfficeEvent({...selectionEvent,type:'selection',selection:{...errorSelection,cells:[{...errorSelection.cells[0],formulaResultType:'error'}]}})).toBe(false)
  expect(isOfficeEvent({...selectionEvent,type:'selection',selection:{...workbookSelection,cells:[{...workbookCell,formulaResultType:'number'}]}})).toBe(false)
})

test('readonly XLSX surfaces read dependency cells and forward fresh bounded cell edit requests without enabling input',async()=>{
  const s=await setup(),opening=s.surface.request({...input('open'),kind:'xlsx',bytes:new Uint8Array([1,2,3])})
  const opened=await sent(s.peer);cleanReply(s.peer,opened,{state:{...state(),kind:'xlsx'},selection:workbookSelection});await opening
  s.peer.peer.messages.length=0
  const reading=s.surface.request({...input('captureCells'),selectionToken:'selection_cell_01',expectedChangeSequence:0,bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:0}})
  const request=await sent(s.peer)
  s.peer.postMessage({command:request.command,channel:request.channel,operationId:request.operationId,documentId:request.documentId,version:request.version,type:'result',ok:true,state:{...state(),kind:'xlsx'},workbook:workbookSnapshot})
  expect(await reading).toMatchObject({workbook:workbookSnapshot})
  s.peer.postMessage({type:'cell-edit-requested',intent:'double-click',channel:opened.channel,operationId:opened.operationId,documentId:'document',version:'version-1',selection:workbookSelection})
  await vi.waitFor(()=>expect(s.events.at(-1)).toMatchObject({type:'cell-edit-requested',intent:'double-click'}))
  expect(s.view.webContents.dead).toBe(false)
  await expect(s.surface.request({...input('replaceCells'),selectionToken:'selection_cell_01',expectedChangeSequence:0,workbook:{before:workbookSnapshot,after:workbookSnapshot,results:['']}})).rejects.toThrow('office-invalid-request')
})
