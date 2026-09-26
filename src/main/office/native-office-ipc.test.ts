import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { EventEmitter } from 'node:events'
const state = vi.hoisted(() => ({
  packaged: false, handlers: new Map<string, (...args: any[]) => any>(),
  lifecycle: new Map<string, (...args: any[]) => void>(),
  freezeAnnotationInput: vi.fn(), hasPendingAnnotations: vi.fn(), flushAnnotations: vi.fn(), request: vi.fn(), getView: vi.fn(), destroy: vi.fn(), create: vi.fn(), dialog: vi.fn(), picker: vi.fn(), quit: vi.fn(), menu: vi.fn(), popup: vi.fn()
}))
vi.mock('electron', () => ({
  app: { get isPackaged() { return state.packaged }, getAppPath: () => '/source', getLocale: () => 'en',
    on: (name: string, handler: (...args: any[]) => void) => state.lifecycle.set(name, handler), quit: state.quit },
  dialog: { showMessageBox: state.dialog, showOpenDialog: state.picker }, ipcMain: { handle: (name: string, handler: (...args: any[]) => any) => state.handlers.set(name, handler) },
  Menu: { buildFromTemplate: state.menu }
}))
vi.mock('./native-office-controller', () => ({ createNativeOfficeController: (...args: unknown[]) => {
  state.create(...args)
  return { freezeAnnotationInput:state.freezeAnnotationInput, hasPendingAnnotations:state.hasPendingAnnotations, flushAnnotations:state.flushAnnotations, request: state.request, getView: state.getView, getViews: () => state.getView() ? [state.getView()] : [], saveAll: state.request, restoreVisibility: vi.fn(), destroy: state.destroy }
} }))
import { createWriteShutdownCoordinator } from '../write-shutdown'
import { createOfficePrivateAdmissionProvider, type OfficePrivateAdmission } from './office-private-admission'
import { registerNativeOfficeIpc, prepareNativeOfficeQuit, cancelNativeOfficeQuit } from './native-office-ipc'
beforeEach(() => {
  vi.clearAllMocks(); state.packaged = false; state.handlers.clear(); state.lifecycle.clear()
  state.request.mockResolvedValue({ ok: true, view: null }); state.getView.mockReturnValue(null)
  state.destroy.mockResolvedValue(undefined);state.freezeAnnotationInput.mockResolvedValue(undefined)
  state.hasPendingAnnotations.mockReturnValue(false);state.flushAnnotations.mockResolvedValue({ok:true,view:null})
  state.menu.mockReturnValue({ popup: state.popup })
})
afterEach(()=>vi.useRealTimers())
function setup(privateAdmission?:()=>Promise<OfficePrivateAdmission>) {
  const main = Object.assign(new EventEmitter(), { isDestroyed: () => false, close: vi.fn(),
    webContents: Object.assign(new EventEmitter(), { mainFrame: {}, send: vi.fn(), isDestroyed: () => false, isLoadingMainFrame: () => false, setWindowOpenHandler: vi.fn() }) })
  let current: typeof main | null = main
  registerNativeOfficeIpc(() => current as any, vi.fn(), privateAdmission)
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
  expect(state.destroy).toHaveBeenCalledOnce()
  expect(state.request.mock.calls.some(([request]) => request.action === 'save')).toBe(false)
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
  expect(await h.pick()).toMatchObject({ok:false,error:'unavailable'})
  expect(state.request).toHaveBeenCalledOnce();expect(state.picker).toHaveBeenCalledOnce()
})
it('structurally forged tokens cannot bypass the packaged guard',async()=>{
  const h=setup(async()=>({root:'/private',qualificationDigest:'a'.repeat(64)} as unknown as OfficePrivateAdmission));state.packaged=true
  expect(await h.invoke()).toMatchObject({ok:false,error:'unavailable'})
  expect(await h.pick()).toMatchObject({ok:false,error:'unavailable'})
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
