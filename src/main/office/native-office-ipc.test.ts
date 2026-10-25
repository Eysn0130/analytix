import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { EventEmitter } from 'node:events'
const state = vi.hoisted(() => ({
  packaged: false, handlers: new Map<string, (...args: any[]) => any>(),
  lifecycle: new Map<string, (...args: any[]) => void>(),
  pointerActions:vi.fn(), freezeAnnotationInput: vi.fn(), hasPendingAnnotations: vi.fn(), flushAnnotations: vi.fn(), openPresent: vi.fn(), request: vi.fn(), getView: vi.fn(), destroy: vi.fn(), create: vi.fn(), dialog: vi.fn(), picker: vi.fn(), quit: vi.fn(), menu: vi.fn(), popup: vi.fn()
}))
vi.mock('electron', () => ({
  app: { get isPackaged() { return state.packaged }, getAppPath: () => '/source', getLocale: () => 'en',
    on: (name: string, handler: (...args: any[]) => void) => state.lifecycle.set(name, handler), quit: state.quit },
  dialog: { showMessageBox: state.dialog, showOpenDialog: state.picker }, ipcMain: { handle: (name: string, handler: (...args: any[]) => any) => state.handlers.set(name, handler) },
  Menu: { buildFromTemplate: state.menu }
}))
vi.mock('./native-office-controller', () => ({ createNativeOfficeController: (...args: unknown[]) => {
  state.create(...args)
  return { pointerActions:state.pointerActions, freezeAnnotationInput:state.freezeAnnotationInput, hasPendingAnnotations:state.hasPendingAnnotations, flushAnnotations:state.flushAnnotations, request: state.request, openPresent: state.openPresent, getView: state.getView, getViews: () => state.getView() ? [state.getView()] : [], saveAll: state.request, restoreVisibility: vi.fn(), destroy: state.destroy }
} }))
import { createWriteShutdownCoordinator } from '../write-shutdown'
import { createOfficePrivateAdmissionProvider, type OfficePrivateAdmission } from './office-private-admission'
import { registerNativeOfficeIpc, prepareNativeOfficeQuit, cancelNativeOfficeQuit, waitForNativeOfficeQuitTeardown } from './native-office-ipc'
vi.mock('../logger', () => ({ logNativeOfficeFailure: vi.fn() }))
import { logNativeOfficeFailure } from '../logger'
beforeEach(() => {
  vi.clearAllMocks(); state.packaged = false; state.handlers.clear(); state.lifecycle.clear()
  state.request.mockResolvedValue({ ok: true, view: null }); state.getView.mockReturnValue(null)
  state.destroy.mockResolvedValue(undefined);state.freezeAnnotationInput.mockResolvedValue(undefined)
  state.hasPendingAnnotations.mockReturnValue(false);state.flushAnnotations.mockResolvedValue({ok:true,view:null})
  state.menu.mockReturnValue({ popup: state.popup })
})
afterEach(()=>vi.useRealTimers())
function setup(privateAdmission?:()=>Promise<OfficePrivateAdmission>, resolvePresent?:Parameters<typeof registerNativeOfficeIpc>[3]) {
  const main = Object.assign(new EventEmitter(), { isDestroyed: () => false, close: vi.fn(),
    webContents: Object.assign(new EventEmitter(), { mainFrame: {}, send: vi.fn(), isDestroyed: () => false, isLoadingMainFrame: () => false, setWindowOpenHandler: vi.fn() }) })
  let current: typeof main | null = main
  registerNativeOfficeIpc(() => current as any, vi.fn(), privateAdmission,resolvePresent)
  const event = { sender: main.webContents, senderFrame: main.webContents.mainFrame }
  const shutdown = createWriteShutdownCoordinator({
    ipc: { handle: (name: string, handler: (...args: any[]) => any) => { state.handlers.set(name, handler) } } as any,
    prepareNative: prepareNativeOfficeQuit, cancelNative: cancelNativeOfficeQuit, notifyBlocked: async () => undefined
  })
  shutdown.trackWindow(main as any)
  main.webContents.send.mockImplementation((channel, request) => {
    if (channel === 'write:shutdown-request') state.handlers.get('write:shutdown-ack')!(event, { ...request, outcome: { result: 'ready' } })
  })
  return { main, event, invoke: (e: unknown = event, payload: unknown = { action: 'status' }) => state.handlers.get('office:request')!(e, payload),
    pick: (payload: unknown = { kind: 'docx', workspace: '/workspace' }, e: unknown = event) => state.handlers.get('office:pick-file')!(e, payload),
    replace: () => { current = null } }
}
it('opens only a current owner native action menu and rechecks its target when chosen', async () => {
  const h=setup(); await h.invoke()
  const view={objectId:'a'.repeat(64),revision:'b'.repeat(64),changeSequence:0}
  state.getView.mockReturnValue(view)
  const payload={objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,actions:[{id:'task:polish',label:'Polish',enabled:true}]}
  const menu=(event=h.event,request:any=payload)=>state.handlers.get('office:action-menu')!(event,request)
  expect(await menu({sender:{},senderFrame:{}} as any)).toEqual({actionId:null})
  expect(await menu(h.event,{...payload,revision:'c'.repeat(64)})).toEqual({actionId:null})
  expect(state.menu).not.toHaveBeenCalled()
  const first=menu(); state.menu.mock.calls.at(-1)![0][0].click()
  expect(await first).toEqual({actionId:'task:polish'})
  const second=menu(); state.getView.mockReturnValue({...view,changeSequence:1}); state.menu.mock.calls.at(-1)![0][0].click()
  expect(await second).toEqual({actionId:null})
})
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
  expect(await h.pick({kind:'xlsx',workspace:'/workspace'})).toEqual({ ok: false, error: 'unavailable' })
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
it.each(['navigation', 'render-process-gone', 'destroyed'])('revokes %s without destroying retained Office notes or delivering old views', async reason => {
  const h = setup()
  let finish!: (value: unknown) => void
  state.request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const pending = h.invoke()
  const onChange = state.create.mock.calls[0][0].onChange
  if (reason === 'navigation') h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  else h.main.webContents.emit(reason)
  onChange({ path: '/private/old.docx', dirty: true })
  finish({ ok: true, view: { path: '/private/old.docx', dirty: true } })
  expect(await pending).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(h.main.webContents.send).not.toHaveBeenCalledWith('office:changed', expect.anything())
  expect(state.destroy).not.toHaveBeenCalled()
  expect(await h.invoke()).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(state.request.mock.calls.filter(([request]) => request.action !== 'hide')).toHaveLength(1)
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(0)
  expect(h.main.webContents.listenerCount('render-process-gone')).toBe(0)
  expect(h.main.webContents.listenerCount('destroyed')).toBe(0)
})
it('reattaches retained Office state only after a current explicit open succeeds', async () => {
  const h = setup(); await h.invoke()
  const onChange = state.create.mock.calls[0][0].onChange
  h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  const open = { action: 'open', workspace: '/workspace', path: 'report.docx', bounds: { x: 0, y: 0, width: 800, height: 600 } }
  state.request.mockResolvedValueOnce({ ok: false, view: { path: '/private/old.docx' }, error: 'unknown' })
  expect(await h.invoke(h.event, open)).toEqual({ ok: false, view: null, error: 'unknown' })
  expect(await h.invoke()).toEqual({ ok: false, view: null, error: 'unavailable' })
  state.request.mockResolvedValueOnce({ ok: true, view: { path: '/workspace/report.docx' } })
  expect(await h.invoke(h.event, open)).toMatchObject({ ok: true })
  onChange({ path: '/workspace/report.docx' })
  expect(h.main.webContents.send).toHaveBeenCalledWith('office:changed', { path: '/workspace/report.docx' })
  expect(state.create).toHaveBeenCalledOnce()
  expect(state.destroy).not.toHaveBeenCalled()
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(1)
})
it('ignores iframe navigation and flushes Main notes without asking a replacement document for freeze ACK', async () => {
  const h = setup(); await h.invoke()
  h.main.webContents.emit('did-start-navigation', {}, 'about:blank', false, false)
  expect(await h.invoke()).toMatchObject({ ok: true })
  h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  const freezeInput = state.create.mock.calls[0][0].freezeAnnotationInput
  expect(await freezeInput(true)).toBe(true)
  expect(h.main.webContents.send.mock.calls.some(([channel]) => channel === 'office:annotation-input-freeze')).toBe(false)
  expect(await prepareNativeOfficeQuit(h.main as any)).toBe(true)
  expect(state.flushAnnotations).toHaveBeenCalledOnce()
  expect(state.destroy).not.toHaveBeenCalled()
})
it('rejects same-document picker and menu results after navigation and removes request listeners', async () => {
  const h = setup()
  state.picker.mockImplementationOnce(async () => {
    h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
    return { canceled: false, filePaths: ['/workspace/report.docx'] }
  })
  expect(await h.pick()).toEqual({ ok: false, error: 'unavailable' })
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(0)
  await h.invoke()
  const view = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), changeSequence: 0 }
  state.getView.mockReturnValue(view)
  const pending = state.handlers.get('office:action-menu')!(h.event, { objectId: view.objectId, revision: view.revision, expectedChangeSequence: 0,
    actions: [{ id: 'task:polish', label: 'Polish', enabled: true }] })
  expect(state.menu).toHaveBeenCalledOnce()
  h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  expect(await pending).toEqual({ actionId: null })
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(0)
})
it('rejects a same-frame freeze ACK after navigation and does not retain its listeners', async () => {
  const h = setup(); await h.invoke()
  const pending = state.create.mock.calls[0][0].freezeAnnotationInput(true)
  const [, request] = h.main.webContents.send.mock.calls.find(([channel]) => channel === 'office:annotation-input-freeze')!
  h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  expect(await pending).toBe(false)
  expect(await state.handlers.get('office:annotation-input-frozen')!(h.event, request)).toEqual({ ok: false })
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(0)
})
it('closing a readonly preview freezes and flushes before releasing without a save prompt', async () => {
  const h = setup(); await h.invoke()
  state.getView.mockReturnValue({ dirty: false, status: 'ready' })
  const event = { preventDefault: vi.fn() }
  h.main.emit('close', event)
  expect(event.preventDefault).toHaveBeenCalledOnce()
  await vi.waitFor(()=>expect(h.main.close).toHaveBeenCalledOnce())
  expect(state.freezeAnnotationInput).toHaveBeenCalledWith(true)
  expect(state.flushAnnotations).toHaveBeenCalledOnce()
  expect(state.dialog).not.toHaveBeenCalled()
  h.main.emit('closed')
  await vi.waitFor(()=>expect(state.destroy).toHaveBeenCalledOnce())
  expect(state.request.mock.calls.some(([request]) => request.action === 'save')).toBe(false)
})
it('keeps Core teardown pending after closed until Office destroy settles', async () => {
  const h=setup();await h.invoke()
  let finish!:()=>void
  state.destroy.mockImplementation(()=>new Promise<void>(resolve=>{finish=resolve}))
  h.main.emit('closed')
  let settled=false
  const waiting=waitForNativeOfficeQuitTeardown().then(()=>{settled=true})
  expect(settled).toBe(false)
  await vi.waitFor(()=>expect(state.destroy).toHaveBeenCalledOnce())
  expect(await prepareNativeOfficeQuit(h.main as any)).toBe(true)
  expect(settled).toBe(false)
  finish();await waiting
  expect(settled).toBe(true)
})
it('reports a failed cleanup once without poisoning a later teardown attempt', async () => {
  const h=setup();await h.invoke()
  state.destroy.mockRejectedValueOnce(Error('teardown failed'))
  h.main.emit('closed')
  await expect(waitForNativeOfficeQuitTeardown()).rejects.toThrow('native Office teardown did not settle cleanly')
  await expect(waitForNativeOfficeQuitTeardown()).resolves.toBeUndefined()
})

it('dirty native close is cancelled by default and preserves the working copy', async () => {
  const h = setup(); await h.invoke()
  state.getView.mockReturnValue({dirty:true,status:'ready'})
  state.dialog.mockResolvedValue({response:2})
  const event = {preventDefault:vi.fn()}
  h.main.emit('close',event)
  await vi.waitFor(() => expect(state.dialog).toHaveBeenCalledOnce())
  expect(event.preventDefault).toHaveBeenCalledOnce()
  expect(h.main.close).not.toHaveBeenCalled()
  expect(state.destroy).not.toHaveBeenCalled()
})
it('closing an unsaved native tab requires an explicit save or discard decision', async () => {
  const h = setup()
  const view = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), changeSequence: 2, dirty: true, status: 'ready' }
  state.getView.mockReturnValue(view)
  state.request.mockImplementation(async request => request.action === 'close' && !request.discard
    ? {ok:false,view,error:'unsaved_changes'} : {ok:true,view})
  state.dialog.mockResolvedValue({response:2})
  expect(await h.invoke(h.event,{action:'close',objectId:view.objectId})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(state.request.mock.calls.some(([request]) => request.discard)).toBe(false)
  state.dialog.mockResolvedValue({response:1})
  expect(await h.invoke(h.event,{action:'close',objectId:view.objectId})).toMatchObject({ok:true})
  expect(state.request).toHaveBeenLastCalledWith({action:'close',objectId:view.objectId,discard:true})
})

it('an unknown update cannot be discarded by the window close dialog', async () => {
  const h = setup(); await h.invoke()
  state.getView.mockReturnValue({dirty:false,saving:true,status:'error'})
  state.dialog.mockResolvedValue({response:1})
  h.main.emit('close',{preventDefault:vi.fn()})
  await vi.waitFor(() => expect(state.dialog).toHaveBeenCalledOnce())
  expect(state.dialog).toHaveBeenCalledWith(h.main,expect.objectContaining({buttons:['查询后退出','取消'],cancelId:1}))
  expect(h.main.close).not.toHaveBeenCalled()
  expect(state.destroy).not.toHaveBeenCalled()
})


it('a readonly window with pending annotations waits for a single flush before closing',async()=>{
  const h=setup();await h.invoke();state.getView.mockReturnValue({dirty:false,status:'ready'})
  state.hasPendingAnnotations.mockReturnValue(true)
  let finish!:(value:unknown)=>void
  state.flushAnnotations.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  const event={preventDefault:vi.fn()};h.main.emit('close',event);h.main.emit('close',event)
  expect(event.preventDefault).toHaveBeenCalledTimes(2);await vi.waitFor(()=>expect(state.flushAnnotations).toHaveBeenCalledOnce())
  expect(h.main.close).not.toHaveBeenCalled();expect(state.dialog).not.toHaveBeenCalled()
  state.hasPendingAnnotations.mockReturnValue(false);finish({ok:true,view:null})
  await vi.waitFor(()=>expect(h.main.close).toHaveBeenCalledOnce())
  expect(state.request.mock.calls.some(([input])=>input.action==='save')).toBe(false)
})
it('a failed annotation flush blocks window close without offering discard or losing the controller',async()=>{
  const h=setup();await h.invoke();state.getView.mockReturnValue({dirty:false,status:'ready'})
  state.hasPendingAnnotations.mockReturnValue(true);state.flushAnnotations.mockResolvedValue({ok:false,error:'unavailable'})
  state.dialog.mockResolvedValue({response:0})
  h.main.emit('close',{preventDefault:vi.fn()})
  await vi.waitFor(()=>expect(state.dialog).toHaveBeenCalledWith(h.main,expect.objectContaining({message:'标注备注尚未保存',buttons:['返回文档']})))
  expect(h.main.close).not.toHaveBeenCalled();expect(state.destroy).not.toHaveBeenCalled()
  expect(state.hasPendingAnnotations()).toBe(true)
})

it('freeze ACK requires the exact owner frame, request id and frozen value',async()=>{
  const h=setup();await h.invoke()
  const freeze=state.create.mock.calls.at(-1)![0].freezeAnnotationInput
  let completed=false
  const pending=freeze(true).then((ok:boolean)=>{completed=true;return ok})
  const payload=h.main.webContents.send.mock.calls.at(-1)![1]
  const acknowledge=(event=h.event,body:unknown=payload)=>state.handlers.get('office:annotation-input-frozen')!(event,body)
  expect(h.main.webContents.send).toHaveBeenLastCalledWith('office:annotation-input-freeze',expect.objectContaining({frozen:true}))
  for(const [event,body] of [[{sender:{},senderFrame:h.event.senderFrame},payload],[{sender:h.event.sender,senderFrame:{}},payload],[h.event,{...payload,requestId:'00000000-0000-4000-8000-000000000000'}],[h.event,{...payload,frozen:false}],[h.event,{...payload,extra:true}]]){
    expect(await acknowledge(event as typeof h.event,body)).toEqual({ok:false});expect(completed).toBe(false)
  }
  expect(await acknowledge()).toEqual({ok:true});expect(await pending).toBe(true)
  expect(await acknowledge()).toEqual({ok:false})
})
it.each(['foreign-frame','navigated-frame','timeout'] as const)('%s cannot confirm a safe close or trigger flush',async(reason)=>{
  vi.useFakeTimers();const h=setup();await h.invoke();state.getView.mockReturnValue({dirty:false,status:'ready'})
  const freeze=state.create.mock.calls.at(-1)![0].freezeAnnotationInput
  state.freezeAnnotationInput.mockImplementation(async(frozen:boolean)=>{if(frozen&&!await freeze(true))throw Error('freeze failed')})
  h.main.emit('close',{preventDefault:vi.fn()})
  await vi.waitFor(() => expect(h.main.webContents.send).toHaveBeenCalledWith('office:annotation-input-freeze',expect.anything()))
  const payload=h.main.webContents.send.mock.calls.at(-1)![1]
  if(reason!=='timeout'){
    if(reason==='navigated-frame')h.main.webContents.mainFrame={}
    expect(await state.handlers.get('office:annotation-input-frozen')!(reason==='foreign-frame'?{sender:h.event.sender,senderFrame:{}}:h.event,payload)).toEqual({ok:false})
  }
  await vi.advanceTimersByTimeAsync(5000)
  expect(state.flushAnnotations).not.toHaveBeenCalled();expect(h.main.close).not.toHaveBeenCalled();expect(state.destroy).not.toHaveBeenCalled()
  expect(state.freezeAnnotationInput).toHaveBeenLastCalledWith(false)
})
it('quit preparation shares the active flush and stays frozen until completion or explicit cancellation',async()=>{
  const h=setup();await h.invoke();state.hasPendingAnnotations.mockReturnValue(true)
  let complete!:(value:unknown)=>void
  state.flushAnnotations.mockImplementation(()=>new Promise(resolve=>{complete=resolve}))
  const first=prepareNativeOfficeQuit(),second=prepareNativeOfficeQuit()
  await vi.waitFor(()=>expect(state.flushAnnotations).toHaveBeenCalledOnce())
  expect(state.freezeAnnotationInput).toHaveBeenCalledOnce();expect(state.freezeAnnotationInput).toHaveBeenCalledWith(true)
  state.hasPendingAnnotations.mockReturnValue(false);complete({ok:true,view:null})
  expect(await first).toBe(true);expect(await second).toBe(true)
  expect(state.freezeAnnotationInput).toHaveBeenCalledOnce();expect(state.destroy).not.toHaveBeenCalled()
  await cancelNativeOfficeQuit();expect(state.freezeAnnotationInput).toHaveBeenLastCalledWith(false)
  expect(h.main.close).not.toHaveBeenCalled()
})
it('an earlier window close policy cancellation does not initiate a freeze, flush or close',async()=>{
  const h=setup();await h.invoke();state.getView.mockReturnValue({dirty:false,status:'ready'})
  const event={defaultPrevented:true,preventDefault:vi.fn()}
  h.main.emit('close',event)
  await Promise.resolve()
  expect(state.freezeAnnotationInput).not.toHaveBeenCalled();expect(state.flushAnnotations).not.toHaveBeenCalled()
  expect(h.main.close).not.toHaveBeenCalled();expect(state.destroy).not.toHaveBeenCalled()
})
it.each(['flush-failure','user-cancel'] as const)('quit %s releases the freeze and preserves the controller',async(reason)=>{
  const h=setup();await h.invoke()
  if(reason==='flush-failure'){
    state.hasPendingAnnotations.mockReturnValue(true)
    state.flushAnnotations.mockResolvedValue({ok:false,error:'unavailable'})
    state.dialog.mockResolvedValue({response:0})
  }else{
    state.getView.mockReturnValue({dirty:true,status:'ready'})
    state.dialog.mockResolvedValue({response:2})
  }
  expect(await prepareNativeOfficeQuit()).toBe(false)
  expect(state.freezeAnnotationInput.mock.calls).toEqual([[true],[false]])
  expect(state.destroy).not.toHaveBeenCalled();expect(h.main.close).not.toHaveBeenCalled();expect(state.quit).not.toHaveBeenCalled()
})


it('packaged Office accepts current Main-only Core admission without reading the development asset environment',async()=>{
  const resources='/Applications/analytix.app/Contents/Resources'
  const transport=vi.fn(async(_path:string,body:string)=>({ok:true,status:200,body:JSON.stringify({ok:true,requestId:JSON.parse(body).requestId,root:resources+'/office-private',qualificationDigest:'a'.repeat(64)})}))
  const h=setup(createOfficePrivateAdmissionProvider(transport,resources));state.packaged=true
  expect(await h.invoke()).toEqual({ok:true,view:null});expect(state.create).toHaveBeenCalledOnce()
  state.picker.mockResolvedValue({canceled:true,filePaths:[]})
  expect(await h.pick()).toEqual({ok:true,path:null})
  transport.mockResolvedValue({ok:false,status:503,body:JSON.stringify({ok:false,code:'unavailable'})})
  expect(await h.invoke()).toMatchObject({ok:false,error:'unavailable'})
  expect(await h.pick({kind:'xlsx',workspace:'/workspace'})).toMatchObject({ok:false,error:'unavailable'})
  expect(state.request).toHaveBeenCalledOnce();expect(state.picker).toHaveBeenCalledOnce()
})
it('structurally forged tokens cannot bypass the packaged guard',async()=>{
  const h=setup(async()=>({root:'/private',qualificationDigest:'a'.repeat(64)} as unknown as OfficePrivateAdmission));state.packaged=true
  expect(await h.invoke()).toMatchObject({ok:false,error:'unavailable'})
  expect(await h.pick({kind:'xlsx',workspace:'/workspace'})).toMatchObject({ok:false,error:'unavailable'})
  expect(state.create).not.toHaveBeenCalled();expect(state.picker).not.toHaveBeenCalled()
})
it('an admission response arriving after the owner frame changes cannot create a controller',async()=>{
  let finish!:(value:{ok:boolean;status:number;body:string})=>void,requestId=''
  const resources='/Applications/analytix.app/Contents/Resources'
  const provider=createOfficePrivateAdmissionProvider(async(_path,body)=>{requestId=JSON.parse(body).requestId;return new Promise(resolve=>{finish=resolve})},resources)
  const h=setup(provider);state.packaged=true
  const pending=h.invoke();h.main.webContents.mainFrame={}
  finish({ok:true,status:200,body:JSON.stringify({ok:true,requestId,root:resources+'/office-private',qualificationDigest:'a'.repeat(64)})})
  expect(await pending).toMatchObject({ok:false,error:'unavailable'})
  expect(state.create).not.toHaveBeenCalled()
})
it('an admitted controller records the last annotation synchronously before freeze ACK without waiting on another admission query',async()=>{
  const resources='/Applications/analytix.app/Contents/Resources'
  const transport=vi.fn(async(_path:string,body:string)=>({ok:true,status:200,body:JSON.stringify({ok:true,requestId:JSON.parse(body).requestId,root:resources+'/office-private',qualificationDigest:'a'.repeat(64)})}))
  const h=setup(createOfficePrivateAdmissionProvider(transport,resources));state.packaged=true;await h.invoke()
  const order:string[]=[]
  state.request.mockImplementation(async request=>{if(request.action==='annotationSave'){order.push('note');state.hasPendingAnnotations.mockReturnValue(true)}return {ok:true,view:null}})
  state.freezeAnnotationInput.mockImplementation(async()=>{order.push('freeze')})
  state.flushAnnotations.mockImplementation(async()=>{state.hasPendingAnnotations.mockReturnValue(false);return {ok:true,view:null}})
  // A new admission query would never finish. Earlier note IPC must still be
  // delivered to Main before the independently delivered freeze ACK.
  transport.mockImplementation(()=>new Promise(()=>{}))
  const request={action:'annotationSave',objectId:'a'.repeat(64),threadId:'thread',note:'last key',sourceRevision:'b'.repeat(64)}
  const saving=h.invoke(h.event,request)
  expect(state.request).toHaveBeenLastCalledWith(request);expect(state.hasPendingAnnotations()).toBe(true)
  h.main.emit('close',{preventDefault:vi.fn()})
  await vi.waitFor(() => expect(order.slice(0,2)).toEqual(['note','freeze']));expect(transport).toHaveBeenCalledOnce()
  state.hasPendingAnnotations.mockReturnValue(false);await saving
  await vi.waitFor(()=>expect(h.main.close).toHaveBeenCalledOnce())
  h.replace();expect(await h.invoke(h.event,request)).toMatchObject({ok:false,error:'unavailable'})
  expect(state.request.mock.calls.filter(([input])=>input.action==='annotationSave')).toHaveLength(1)
})

it('resolves selector-only Present Office privately and clears its bytes after the controller opens', async () => {
  const selector = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  const bytes = Buffer.from([80,75,3,4,0]), cap = {bytes,path:'/sealed/actual.docx',workspaceRoot:'/sealed',kind:'docx',selector,
    digest:'d'.repeat(64),size:bytes.length,mtimeMs:1,truncated:false,signal:new AbortController().signal,isCurrent:() => true,guard:async () => true}
  const resolve = vi.fn(async () => cap as any), h = setup(undefined,resolve)
  const request = {action:'openPresent',protectedPresent:selector,bounds:{x:0,y:0,width:800,height:600}}
  state.openPresent.mockImplementationOnce(async (input,snapshot) => {expect(input).toEqual(request);expect(snapshot).toBe(cap);expect(bytes[0]).toBe(80);return {ok:true,view:null}})
  expect(await h.invoke(h.event,request)).toEqual({ok:true,view:null})
  expect(resolve).toHaveBeenCalledWith(h.event,selector);expect(state.request).not.toHaveBeenCalled()
  expect(bytes.every(value => value === 0)).toBe(true)
  await h.invoke(h.event,{...request,path:'/forged.docx'});expect(resolve).toHaveBeenCalledTimes(1)
  expect(await h.invoke({sender:{},senderFrame:{}} as any,request)).toMatchObject({ok:false});expect(resolve).toHaveBeenCalledTimes(1)
})
it('does not expose bytes when a Present cap completes after native owner replacement', async () => {
  const selector = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  let finish!: (value:any) => void
  const h = setup(undefined,() => new Promise(resolve => {finish = resolve}))
  const pending = h.invoke(h.event,{action:'openPresent',protectedPresent:selector,bounds:{x:0,y:0,width:800,height:600}})
  await vi.waitFor(() => expect(finish).toBeDefined());h.main.webContents.mainFrame = {}
  const bytes = Buffer.from([80,75,3,4]);finish({bytes})
  expect(await pending).toMatchObject({ok:false,view:null});expect(bytes.every(value => value === 0)).toBe(true)
  expect(state.openPresent).not.toHaveBeenCalled()
})

it('observes admission denial only for an open request', async () => {
  const h = setup(async () => { throw Error('/private/admission-sentinel') })
  state.packaged = true
  expect(await h.invoke(h.event, { action: 'open', workspace: '/workspace', path: 'report.docx', bounds: { x: 0, y: 0, width: 800, height: 600 } })).toMatchObject({ ok: false })
  expect(logNativeOfficeFailure).toHaveBeenCalledExactlyOnceWith('ipc_admission', 'unavailable', false)
  expect(state.create).not.toHaveBeenCalled()
  expect(JSON.stringify(vi.mocked(logNativeOfficeFailure).mock.calls)).not.toContain('/private/admission-sentinel')
})
it.each(['missing', 'throwing'] as const)('observes %s protected resolution without changing its result', async mode => {
  const secret = '/private/present-resolution-sentinel'
  const h = setup(undefined, async () => { if (mode === 'throwing') throw Error(secret); return null })
  const selector = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 0 }
  const pending = h.invoke(h.event, { action: 'openPresent', protectedPresent: selector, bounds: { x: 0, y: 0, width: 800, height: 600 } })
  if (mode === 'throwing') await expect(pending).rejects.toThrow(secret)
  else expect(await pending).toMatchObject({ ok: false, view: null, error: 'unavailable' })
  expect(logNativeOfficeFailure).toHaveBeenCalledExactlyOnceWith('ipc_resolve_present', mode === 'throwing' ? 'unexpected' : 'unavailable', true)
  expect(JSON.stringify(vi.mocked(logNativeOfficeFailure).mock.calls)).not.toContain(secret)
  expect(state.openPresent).not.toHaveBeenCalled()
})

it('routes pointer menus only through the DOCX controller range lease and retains owner/frame cancellation',async()=>{
  const h=setup();await h.invoke()
  const view={objectId:'a'.repeat(64),revision:'b'.repeat(64),changeSequence:0,kind:'docx'}
  const payload={objectId:view.objectId,revision:view.revision,expectedChangeSequence:0,pointer:{interactionId:'interaction_id',selectionToken:'selection_token'},actions:[{id:'quote',label:'Quote',enabled:true}]}
  state.getView.mockReturnValue(view);state.pointerActions.mockResolvedValue('quote')
  const menu=(request:any=payload)=>state.handlers.get('office:action-menu')!(h.event,request)
  expect(await menu()).toEqual({actionId:'quote'});expect(state.pointerActions).toHaveBeenCalledWith(payload);expect(state.menu).not.toHaveBeenCalled()
  for(const next of [{...view,kind:'xlsx'},{...view,protectedPresent:{}}]){state.getView.mockReturnValue(next);expect(await menu()).toEqual({actionId:null})}
  expect(state.pointerActions).toHaveBeenCalledTimes(1)
  state.getView.mockReturnValue(view)
  state.pointerActions.mockImplementation(async()=>{h.main.webContents.mainFrame={};return 'quote'})
  expect(await menu()).toEqual({actionId:null})
})


it('packaged Word creation reaches only the Core controller without admitting native assets', async () => {
  const admit = vi.fn(async (): Promise<OfficePrivateAdmission> => { throw Error('synthetic assets unavailable') })
  const h = setup(admit); state.packaged = true
  const request = { action: 'create', threadId: 'thread-word', operationId: 'create_operation', fileName: 'report.docx', bounds: { x: 0, y: 0, width: 800, height: 600 } }
  const receipt = { operationId: request.operationId, workspace: '/workspace', path: '/workspace/report.docx', revision: 'a'.repeat(64), kind: 'docx', byteSize: 7, savedAt: '2026-10-15T00:00:00Z' }
  state.request.mockResolvedValueOnce({ ok: true, view: null, creation: receipt })
  expect(await h.invoke(h.event, request)).toEqual({ ok: true, view: null, creation: receipt })
  expect(state.create).toHaveBeenCalledOnce(); expect(state.request).toHaveBeenCalledExactlyOnceWith(request)
  expect(admit).not.toHaveBeenCalled(); expect(state.openPresent).not.toHaveBeenCalled()
  expect(h.main.webContents.send).not.toHaveBeenCalledWith('office:changed', expect.anything())
})

it('a creation receipt after revoked delivery cannot reattach old views or restore status access', async () => {
  const h = setup(); await h.invoke()
  const onChange = state.create.mock.calls[0][0].onChange
  h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  const request = { action: 'create', threadId: 'thread-word', operationId: 'create_operation', fileName: 'report.docx', bounds: { x: 0, y: 0, width: 800, height: 600 } }
  const receipt = { operationId: request.operationId, workspace: '/workspace', path: '/workspace/report.docx', revision: 'a'.repeat(64), kind: 'docx', byteSize: 7, savedAt: '2026-10-15T00:00:00Z' }
  const oldView = { path: '/workspace/retained.docx', dirty: false }
  state.request.mockClear()
  state.request.mockImplementationOnce(async () => { onChange(oldView); return { ok: true, view: null, creation: receipt } })
  expect(await h.invoke(h.event, request)).toEqual({ ok: true, view: null, creation: receipt })
  onChange(oldView)
  expect(await h.invoke()).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(state.request).toHaveBeenCalledExactlyOnceWith(request)
  expect(state.create).toHaveBeenCalledOnce(); expect(state.destroy).not.toHaveBeenCalled()
  expect(h.main.webContents.send).not.toHaveBeenCalledWith('office:changed', expect.anything())
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(0)
  expect(h.main.webContents.listenerCount('render-process-gone')).toBe(0)
  expect(h.main.webContents.listenerCount('destroyed')).toBe(0)
})

it('a clean active manual transaction still requires the existing quit decision and cancellation retains its owner', async () => {
  const h = setup(); await h.invoke()
  const view = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), status: 'ready', dirty: false, editing: true, changeSequence: 0,
    manual: { version: 2, origin: 'manual', changeId: 'c'.repeat(64), baseRevision: 'b'.repeat(64), generation: 1, frozen: false } }
  state.getView.mockReturnValue(view); state.dialog.mockResolvedValue({ response: 2 })
  expect(await prepareNativeOfficeQuit(h.main as any)).toBe(false)
  expect(state.dialog).toHaveBeenCalledExactlyOnceWith(h.main, expect.objectContaining({
    buttons: ['重试后退出', '放弃工作副本并退出', '取消'], defaultId: 2, cancelId: 2
  }))
  expect(state.request).toHaveBeenLastCalledWith({ action: 'hide' })
  expect(state.request.mock.calls.some(([request]) => request.action === 'close')).toBe(false)
  expect(state.freezeAnnotationInput.mock.calls).toEqual([[true], [false]])
  expect(state.getView()).toBe(view); expect(state.destroy).not.toHaveBeenCalled()
  expect(h.main.close).not.toHaveBeenCalled(); expect(state.quit).not.toHaveBeenCalled()
})

it('manual discard uncertainty blocks quit and retains the owner until the same explicit close succeeds', async () => {
  const h = setup(); await h.invoke()
  const view = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), status: 'ready', dirty: false, editing: true, changeSequence: 0,
    manual: { version: 2, origin: 'manual', changeId: 'c'.repeat(64), baseRevision: 'b'.repeat(64), generation: 1, frozen: false } }
  state.getView.mockReturnValue(view); state.dialog.mockResolvedValue({ response: 1 })
  let closeAttempts = 0
  state.request.mockImplementation(async request => {
    if (request.action !== 'close') return { ok: true, view }
    if (++closeAttempts === 1) return { ok: false, view, error: 'unknown' }
    state.getView.mockReturnValue(null)
    return { ok: true, view: null }
  })
  const close = { action: 'close', objectId: view.objectId, discard: true }
  expect(await prepareNativeOfficeQuit(h.main as any)).toBe(false)
  expect(state.request).toHaveBeenLastCalledWith(close)
  expect(state.getView()).toBe(view); expect(state.create).toHaveBeenCalledOnce()
  expect(state.freezeAnnotationInput.mock.calls).toEqual([[true], [false]])
  expect(state.destroy).not.toHaveBeenCalled(); expect(h.main.close).not.toHaveBeenCalled(); expect(state.quit).not.toHaveBeenCalled()
  expect(await prepareNativeOfficeQuit(h.main as any)).toBe(true)
  expect(state.request.mock.calls.filter(([request]) => request.action === 'close').map(([request]) => request)).toEqual([close, close])
  expect(state.getView()).toBeNull(); expect(state.dialog).toHaveBeenCalledTimes(2)
  expect(state.freezeAnnotationInput.mock.calls).toEqual([[true], [false], [true]])
  expect(state.destroy).not.toHaveBeenCalled()
})

const ipcPointerFormats = [
  { kind: 'font-family', value: 'Noto Sans CJK SC' }, { kind: 'font-size', value: 16.5 },
  { kind: 'bold', value: true }, { kind: 'italic', value: false }, { kind: 'underline', value: true }
] as const
async function formattingPointerMenu() {
  const h = setup(); await h.invoke()
  const rangeLease = { generation: 1, sequence: 0, token: 'selection_token' }
  const formatState = { fontFamily: 'Aptos', fontSize: 12, bold: null, italic: false, underline: false }
  const view = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), changeSequence: 0, kind: 'docx', status: 'ready', dirty: false, editing: true,
    manual: { version: 2, origin: 'manual', changeId: 'c'.repeat(64), baseRevision: 'b'.repeat(64), generation: 1, frozen: false },
    selection: { documentId: 'a'.repeat(64), version: 'b'.repeat(64), changeSequence: 0, kind: 'text', text: 'selected text',
      scope: 'session-text-range-at-version-and-change-sequence', token: rangeLease.token, rangeLease, formatState } }
  const payload = { objectId: view.objectId, revision: view.revision, expectedChangeSequence: 0,
    pointer: { interactionId: 'format_interaction', selectionToken: rangeLease.token }, actions: [{ id: 'quote', label: 'Quote', enabled: true }],
    formatting: { changeId: view.manual.changeId, rangeLease,
      formatState,
      labels: { fontFamily: 'Font', fontSize: 'Size', bold: 'Bold', italic: 'Italic', underline: 'Underline', mixed: 'Mixed' } } }
  state.getView.mockReturnValue(view)
  const menu = (request: any = payload, event: any = h.event) => state.handlers.get('office:action-menu')!(event, request)
  return { ...h, view, payload, menu }
}

it.each(ipcPointerFormats)('relays the typed pointer $kind choice without nesting it inside actionId', async format => {
  const h = await formattingPointerMenu()
  const choice = { actionId: null, changeId: h.view.manual.changeId, rangeLease: h.payload.formatting.rangeLease, format }
  state.pointerActions.mockResolvedValue(choice)
  expect(await h.menu()).toEqual(choice)
  expect(state.pointerActions).toHaveBeenCalledExactlyOnceWith(h.payload)
  expect(state.menu).not.toHaveBeenCalled()
  expect(state.request).toHaveBeenCalledExactlyOnceWith({ action: 'status' })
})

it.each(['change', 'generation', 'sequence', 'token', 'action', 'open-command', 'extra-field'] as const)('rejects a controller pointer format result with forged %s', async mismatch => {
  const h = await formattingPointerMenu()
  const choice: any = { actionId: null, changeId: h.view.manual.changeId, rangeLease: { ...h.payload.formatting.rangeLease }, format: { kind: 'bold', value: true } }
  if (mismatch === 'change') choice.changeId = 'd'.repeat(64)
  if (mismatch === 'generation') choice.rangeLease.generation++
  if (mismatch === 'sequence') choice.rangeLease.sequence++
  if (mismatch === 'token') choice.rangeLease.token = 'different_token'
  if (mismatch === 'action') choice.actionId = 'quote'
  if (mismatch === 'open-command') choice.format = { kind: 'uno', value: '.uno:Save' }
  if (mismatch === 'extra-field') choice.dispatch = '.uno:Save'
  state.pointerActions.mockResolvedValue(choice)
  expect(await h.menu()).toEqual({ actionId: null })
  expect(state.pointerActions).toHaveBeenCalledExactlyOnceWith(h.payload)
  expect(state.menu).not.toHaveBeenCalled()
})

it.each(['selection', 'protected', 'spreadsheet', 'frozen', 'saving', 'readonly', 'token', 'sample'] as const)('refuses a %s pointer format target before invoking the controller', async reason => {
  const h = await formattingPointerMenu()
  const choice = { actionId: null, changeId: h.view.manual.changeId, rangeLease: h.payload.formatting.rangeLease, format: { kind: 'bold', value: true } }
  state.pointerActions.mockResolvedValue(choice)
  const view: any = structuredClone(h.view)
  if (reason === 'selection') view.changeSequence++
  if (reason === 'protected') view.protectedPresent = {}
  if (reason === 'spreadsheet') view.kind = 'xlsx'
  if (reason === 'frozen') view.manual.frozen = true
  if (reason === 'saving') view.saving = true
  if (reason === 'readonly') { view.editing = false; delete view.manual }
  if (reason === 'token') { view.selection.token = 'different_token'; view.selection.rangeLease.token = 'different_token' }
  if (reason === 'sample') view.selection.formatState.bold = true
  state.getView.mockReturnValue(view)
  expect(await h.menu()).toEqual({ actionId: null })
  expect(state.pointerActions).not.toHaveBeenCalled()
  expect(state.menu).not.toHaveBeenCalled()
})

it.each(['frame', 'owner', 'selection', 'frozen', 'saving', 'readonly', 'token', 'sample'] as const)('drops a valid pointer format result after the %s changes while it is pending', async reason => {
  const h = await formattingPointerMenu()
  const choice = { actionId: null, changeId: h.view.manual.changeId, rangeLease: h.payload.formatting.rangeLease, format: { kind: 'bold', value: true } }
  state.pointerActions.mockImplementation(async () => {
    if (reason === 'frame') h.main.webContents.mainFrame = {}
    if (reason === 'owner') h.replace()
    const view: any = structuredClone(h.view)
    if (reason === 'selection') view.changeSequence++
    if (reason === 'frozen') view.manual.frozen = true
    if (reason === 'saving') view.saving = true
    if (reason === 'readonly') { view.editing = false; delete view.manual }
    if (reason === 'token') { view.selection.token = 'different_token'; view.selection.rangeLease.token = 'different_token' }
    if (reason === 'sample') view.selection.formatState.bold = true
    state.getView.mockReturnValue(view)
    return choice
  })
  expect(await h.menu()).toEqual({ actionId: null })
  expect(state.pointerActions).toHaveBeenCalledExactlyOnceWith(h.payload)
  expect(state.menu).not.toHaveBeenCalled()
  expect(state.request.mock.calls.some(([request]) => request.action === 'hide')).toBe(reason === 'frame' || reason === 'owner')
})

it.each(['navigation', 'render-process-gone', 'destroyed'] as const)('cancels a stale pointer range without hiding its document and still hides on %s', async reason => {
  const h = await formattingPointerMenu()
  const choice = { actionId: null, changeId: h.view.manual.changeId, rangeLease: h.payload.formatting.rangeLease, format: { kind: 'bold', value: true } }
  let finish!: (value: unknown) => void
  state.pointerActions.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const stale = h.menu()
  expect(state.pointerActions).toHaveBeenCalledExactlyOnceWith(h.payload)
  const nextLease = { ...h.payload.formatting.rangeLease, token: 'next_selection_token' }
  state.getView.mockReturnValue({ ...h.view, selection: { ...h.view.selection, token: nextLease.token, rangeLease: nextLease } })
  finish(choice)
  expect(await stale).toEqual({ actionId: null })
  expect(state.request).not.toHaveBeenCalledWith({ action: 'hide' })
  expect(await h.invoke()).toMatchObject({ ok: true })

  // The new native range needs its own current offer; the old choice stays cancelled.
  const nextPayload = { ...h.payload, pointer: { ...h.payload.pointer, selectionToken: nextLease.token },
    formatting: { ...h.payload.formatting, rangeLease: nextLease } }
  const nextChoice = { ...choice, rangeLease: nextLease }
  state.pointerActions.mockResolvedValueOnce(nextChoice)
  expect(await h.menu(nextPayload)).toEqual(nextChoice)
  expect(state.request).not.toHaveBeenCalledWith({ action: 'hide' })

  state.pointerActions.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const revoked = h.menu(nextPayload)
  if (reason === 'navigation') h.main.webContents.emit('did-start-navigation', {}, 'analytix://app', false, true)
  else h.main.webContents.emit(reason)
  expect(state.request).toHaveBeenCalledWith({ action: 'hide' })
  finish(nextChoice)
  expect(await revoked).toEqual({ actionId: null })
  expect(await h.invoke()).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(h.main.webContents.listenerCount('did-start-navigation')).toBe(0)
})

it('refuses unoffered formatting and formatting without a native pointer lease', async () => {
  const h = await formattingPointerMenu()
  const choice = { actionId: null, changeId: h.view.manual.changeId, rangeLease: h.payload.formatting.rangeLease, format: { kind: 'bold', value: true } }
  state.pointerActions.mockResolvedValue(choice)
  const { pointer: _pointer, ...withoutPointer } = h.payload
  expect(await h.menu(withoutPointer)).toEqual({ actionId: null })
  expect(state.pointerActions).not.toHaveBeenCalled(); expect(state.menu).not.toHaveBeenCalled()
  const { formatting: _formatting, ...quoteOnly } = h.payload
  expect(await h.menu(quoteOnly)).toEqual({ actionId: null })
  expect(state.pointerActions).toHaveBeenCalledExactlyOnceWith(quoteOnly)
  state.pointerActions.mockResolvedValue(null)
  expect(await h.menu(quoteOnly)).toEqual({ actionId: null })
})

it('rejects DOCX bridge requests from a foreign renderer or subframe before package access',async()=>{
  const h=setup(),request=state.handlers.get('docx-editor:request')!
  const payload={action:'open',workspace:'/workspace',path:'report.docx',threadId:'thread'}
  expect(await request({sender:{},senderFrame:{}},payload)).toMatchObject({ok:false,error:'unavailable'})
  expect(await request({...h.event,senderFrame:{}},payload)).toMatchObject({ok:false,error:'unavailable'})
  expect(state.create).not.toHaveBeenCalled()
})

it('ordinary DOCX picking does not require old native assets or create a native surface',async()=>{
  const h=setup();state.packaged=true;state.picker.mockResolvedValue({canceled:false,filePaths:['/workspace/report.docx']})
  expect(await h.pick()).toEqual({ok:true,path:'/workspace/report.docx'})
  expect(state.create).not.toHaveBeenCalled()
})
