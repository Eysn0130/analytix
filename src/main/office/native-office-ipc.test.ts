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
import { registerNativeOfficeIpc, prepareNativeOfficeQuit, cancelNativeOfficeQuit } from './native-office-ipc'
beforeEach(() => {
  vi.clearAllMocks(); state.packaged = false; state.handlers.clear(); state.lifecycle.clear()
  state.request.mockResolvedValue({ ok: true, view: null }); state.getView.mockReturnValue(null)
  state.destroy.mockResolvedValue(undefined);state.freezeAnnotationInput.mockResolvedValue(undefined)
  state.hasPendingAnnotations.mockReturnValue(false);state.flushAnnotations.mockResolvedValue({ok:true,view:null})
  state.menu.mockReturnValue({ popup: state.popup })
})
afterEach(()=>vi.useRealTimers())
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
