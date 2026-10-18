import { expect, test, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { isOfficeRequest } from './office-protocol'
const envelope = (command: string, extra = {}) => ({ command, channel: 'channel', documentId: 'document', version: 'version', operationId: command, ...extra })
function worker(readOnly = true, kind = 'docx', pageCount = 3) {
  const messages: any[] = [], listeners: any = {}, close = vi.fn(), remove = vi.fn()
  const properties: Record<string, number> = {ZoomValue:100, ZoomType:3}
  const setPropertyValue = vi.fn((name: string, value: any) => { properties[name] = value.value })
  const settings = {getPropertyValue:(name:string) => properties[name], setPropertyValue,
    getPropertySetInfo:() => ({getPropertyByName:() => ({Type:'short'})})}
  const pages = Array.from({length:pageCount}, () => ({})); let page = pages[0]
  const controller = {
    ...settings, getViewSettings:() => settings, getCurrentPage:() => page, setCurrentPage:(value:any) => {page=value},
    getFrame: () => ({ getContainerWindow: () => ({}), LayoutManager: { setVisible() {}, isVisible: () => false } }),
    getSelection: () => ({ getString: () => '' }), addSelectionChangeListener() {}, removeSelectionChangeListener: remove
  }
  const load = vi.fn()
  const model = { getDrawPages:() => ({getCount:() => pages.length, getByIndex:(i:number) => pages[i]}), isReadonly: vi.fn(() => readOnly), isModified: vi.fn(() => false), getCurrentController: () => controller, setModified: vi.fn(), close,
    addModifyListener: (l: any) => { listeners.modify = l }, removeModifyListener: remove, storeToURL() {} }
  const port: any = { postMessage: (m: any) => messages.push(m) }
  const css = { frame: { Desktop: { create: () => ({ loadComponentFromURL: (...args: unknown[]) => { load(...args); return model } }) } },
    beans: { PropertyValue: function(this: any, p: any) { Object.assign(this, p) } }, util: { XModifyListener: {} }, view: { XSelectionChangeListener: {} } }
  const zeta = { Any:function(this:any,_type:any,value:any){this.value=value}, sameUnoObject:(a:any,b:any) => a === b, uno: { com: { sun: { star: css } } }, getUnoComponentContext() {}, mainPort: port, unoObject: (_: any, o: any) => o }
  vm.runInNewContext(readFileSync(new URL('./surface/office-worker.js', import.meta.url), 'utf8'), { Module: { zetajs: { then: (fn: any) => fn(zeta) } } })
  const send = (command: string, extra = {}) => { port.onmessage({ data: envelope(command, extra) }); return messages.at(-1) }
  send('bind'); send('open', { kind })
  return { send, close, model, remove, load, properties, setPropertyValue, change: () => listeners.modify.modified() }
}
test('conditional close accepts clean exact sequence and performs one native close', () => {
  const w = worker(); expect(w.send('close', { expectedChangeSequence: 0, discard: false }).ok).toBe(true); expect(w.close).toHaveBeenCalledOnce()
})
test('dirty or stale sequence close is rejected without removing listeners or closing model', () => {
  const w = worker(); w.model.isModified.mockReturnValue(true); w.change()
  for (const sequence of [0, 1]) expect(w.send('close', { expectedChangeSequence: sequence, discard: false })).toMatchObject({ ok: false, error: 'unsaved-changes' })
  expect(w.close).not.toHaveBeenCalled(); expect(w.remove).not.toHaveBeenCalled()
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 1, acknowledgedSequence: 0, dirty: true })
  expect(w.send('close', { expectedChangeSequence: 1, discard: true }).ok).toBe(true); expect(w.close).toHaveBeenCalledOnce()
})
test('preview opens native files readonly and denies every mutation or export command', () => {
  const w = worker()
  const properties = w.load.mock.calls[0][3]
  expect(properties).toEqual(expect.arrayContaining([
    expect.objectContaining({ Name: 'ReadOnly', Value: true }),
    expect.objectContaining({ Name: 'LockEditDoc', Value: true }),
    expect.objectContaining({ Name: 'LockSave', Value: true }),
    expect.objectContaining({ Name: 'LockExport', Value: true })
  ]))
  for (const command of ['export', 'ack', 'bold', 'undo', 'redo', 'local-ui']) {
    expect(w.send(command)).toMatchObject({ ok: false, error: 'unsupported-command' })
  }
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 0, dirty: false })
})
test('native engine that does not honor readonly is closed before publishing a document', () => {
  const w = worker(false)
  expect(w.close).toHaveBeenCalledOnce()
  expect(w.send('captureSelection')).toMatchObject({ ok: false, error: 'stale-document-version' })
})
test('malformed close never reaches native close even with discard set', () => {
  const w = worker()
  for (const value of [{}, { expectedChangeSequence: -1, discard: true }, { expectedChangeSequence: Infinity, discard: true }, { expectedChangeSequence: 0, discard: 'yes' }]) {
    expect(isOfficeRequest(envelope('close', value))).toBe(false)
    expect(w.send('close', value).error).toBe('invalid-request')
  }
  expect(w.close).not.toHaveBeenCalled()
  expect(isOfficeRequest(envelope('close', { expectedChangeSequence: 0, discard: false }))).toBe(true)
})
test('Main facade starts engine and sends typed correlated replies without exposing a DOM port', async () => {
  let handler: any
  const bridge = { connect: (h: any) => { handler = h }, send: vi.fn() }
  const nodes: any = Object.fromEntries(['qtcanvas','status','selection','preview','footer','kind','viewControls'].map(k => [k, { addEventListener() {}, setAttribute() {}, appendChild() {}, replaceChildren() {}, contains:()=>false, dataset: {}, focus() {}, querySelectorAll: () => [] }]))
  const context: any = { setTimeout,clearTimeout,document: { createElement:()=>({addEventListener(){},setAttribute(){},replaceChildren(){}}), getElementById: (id: string) => nodes[id], querySelectorAll: () => [] }, analytixOfficeSurface: bridge, addEventListener: vi.fn(), window: { addEventListener() {} }, crypto: { randomUUID: () => 'operation' } }
  vm.createContext(context)
  const source = readFileSync(new URL('./surface/office-surface.js', import.meta.url), 'utf8').replaceAll('export ', '')
  vm.runInContext(source + '\nglobalThis.Engine = OfficeEngineSurface;', context)
  context.Engine.prototype.start = vi.fn(async function(this: any, channel: string) { this.channel = channel })
  context.Engine.prototype.request = vi.fn(async (r: any) => ({ type: 'result', ...r, ok: true }))
  await handler({ type: 'bind', channel: 'channel' })
  expect(bridge.send).toHaveBeenCalledWith({ type: 'ready', channel: 'channel', protocolVersion: 1 })
  // Create the request in the page realm to match the production contextBridge clone.
  context.deliver = handler
  await vm.runInContext("deliver({command:'close',channel:'channel',operationId:'close',documentId:'document',version:'version',expectedChangeSequence:0,discard:false})", context)
  expect(context.Engine.prototype.request).toHaveBeenCalledOnce()
  context.Engine.prototype.request.mockRejectedValueOnce(new Error('/private/document content'))
  await vm.runInContext("deliver({command:'bold',channel:'channel',operationId:'bold',documentId:'document',version:'version'})", context)
  expect(bridge.send.mock.calls.at(-1)?.[0]).toMatchObject({ type: 'result', ok: false, error: 'engine-operation-failed' })
  context.Engine.prototype.request.mockRejectedValueOnce(new Error('unsupported-format-fidelity'))
  await vm.runInContext("deliver({command:'edit',channel:'channel',operationId:'edit',documentId:'document',version:'version'})", context)
  expect(bridge.send.mock.calls.at(-1)?.[0]).toMatchObject({type:'result',ok:false,error:'unsupported-format-fidelity'})
  expect(nodes.status.textContent).toContain('可继续预览')
  expect(JSON.stringify(bridge.send.mock.calls)).not.toContain('/private/document')
})

test.each([false,true])('input guard blocks manual mutation even during AI preparation: %s', async (prepared) => {
  const listeners = new Map<string, (event: any) => void>()
  const context: any = { document: { getElementById: () => null }, window: { addEventListener: (type: string, handler: any) => listeners.set(type, handler) } }
  vm.createContext(context)
  const source = readFileSync(new URL('./surface/office-surface.js', import.meta.url), 'utf8').replaceAll('export ', '')
  vm.runInContext(source + '\nglobalThis.engine = new OfficeEngineSurface({}, () => {});', context)
  context.engine.channel = 'channel'
  context.engine.worker = vi.fn()
  for (const command of ['export', 'ack', 'bold', 'undo', 'redo', 'local-ui']) {
    await expect(context.engine.request(envelope(command))).rejects.toThrow('unsupported-command')
  }
  expect(context.engine.worker).not.toHaveBeenCalled()
  context.engine.editing = prepared
  for (const key of ['a', '中', 'Backspace', 'Delete', 'Enter', 'F2']) {
    const e = { key, preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get('keydown')!(e)
    expect(e.preventDefault).toHaveBeenCalledOnce()
    expect(e.stopImmediatePropagation).toHaveBeenCalledOnce()
  }
  for (const key of ['s', 'b', 'i', 'u', 'z', 'y', 'v', 'x', 'o', 'p', 'n']) {
    const e = { key, ctrlKey: true, preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get('keydown')!(e)
    expect(e.preventDefault).toHaveBeenCalledOnce()
  }
  for (const type of ['beforeinput', 'paste', 'drop', 'cut', 'compositionstart', 'compositionupdate', 'compositionend']) {
    const e = { preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get(type)!(e)
    expect(e.stopImmediatePropagation).toHaveBeenCalledOnce()
  }
  for (const input of [{ key: 'ArrowDown' }, { key: 'PageDown' }, { key: 'c', metaKey: true }]) {
    const e = { ...input, preventDefault: vi.fn(), stopImmediatePropagation: vi.fn() }
    listeners.get('keydown')!(e)
    expect(e.preventDefault).not.toHaveBeenCalled()
  }
  expect(listeners.has('wheel')).toBe(false)
})


test('native view notifications preserve the clean sequence without clearing modified state', () => {
  const w = worker()
  w.change()
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 0, dirty: false })
  expect(w.model.setModified).not.toHaveBeenCalled()
  expect(w.send('close', { expectedChangeSequence: 0, discard: false }).ok).toBe(true)
  expect(w.model.setModified).not.toHaveBeenCalled()
})

test.each(['readonly-lost', 'readonly-failed', 'modified-failed'])('unknown native view state stays fail-closed: %s', (reason) => {
  const w = worker()
  if (reason === 'readonly-lost') w.model.isReadonly.mockReturnValue(false)
  else if (reason === 'readonly-failed') w.model.isReadonly.mockImplementation(() => { throw Error('unknown') })
  else w.model.isModified.mockImplementation(() => { throw Error('unknown') })
  w.change()
  expect(w.send('captureSelection').state).toMatchObject({ changeSequence: 1, dirty: true })
  expect(w.model.setModified).not.toHaveBeenCalled()
})

// LibreOffice MacroExecMode IDL: NEVER_EXECUTE=0; ALWAYS_EXECUTE_NO_WARN=4.
// Capture the real adapter call, independent of its policy constant.
test('every native open and reopen explicitly forbids macros and external updates', () => {
  const w = worker()
  for (const kind of ['docx', 'xlsx', 'pptx']) {
    expect(w.send('close', { expectedChangeSequence: 0, discard: false }).ok).toBe(true)
    expect(w.send('open', { kind }).ok).toBe(true)
    const properties = w.load.mock.calls.at(-1)![3] as { Name: string; Value: unknown }[]
    expect(properties.filter(p => p.Name === 'MacroExecutionMode')).toEqual([{ Name: 'MacroExecutionMode', Value: 0 }])
    expect(properties.filter(p => p.Name === 'UpdateDocMode')).toEqual([{ Name: 'UpdateDocMode', Value: 0 }])
  }
})

test('typed private protocol admits only exact AI text replacement and denies manual editing commands',()=>{
  for(const command of ['format','bold','undo','redo']) expect(isOfficeRequest(envelope(command))).toBe(false)
  const request=envelope('replace',{selectionToken:'selection',expectedChangeSequence:0,text:'AI replacement',valueType:'text'})
  expect(isOfficeRequest(request)).toBe(true)
  for(const valueType of ['number','formula']) expect(isOfficeRequest({...request,valueType})).toBe(false)
  expect(isOfficeRequest({...request,text:'x'.repeat(4097)})).toBe(false)
  expect(isOfficeRequest({...request,format:{operation:'bold',value:true}})).toBe(false)
})


test('actual surface relay forwards only strict typed rectangles after edit admission',async()=>{
 const context:any={document:{getElementById:()=>null},window:{addEventListener(){}}};vm.createContext(context)
 const source=readFileSync(new URL('./surface/office-surface.js',import.meta.url),'utf8').replaceAll('export ','')
 vm.runInContext(source+'\nglobalThis.engine=new OfficeEngineSurface({},()=>{});',context)
 const state={documentId:'document',version:'version',kind:'xlsx',changeSequence:0,acknowledgedSequence:0,dirty:false}
 context.engine.channel='channel';context.engine.state=state;context.engine.worker=vi.fn(async()=>({state}))
 const cell={sheet:0,column:0,row:0,text:'1',formula:'1',value:1,valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}
 const before={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0,cells:[cell]}
 const request={...envelope('replaceCells'),selectionToken:'selection',expectedChangeSequence:0,workbook:{before,after:{...before,cells:[{...cell,text:'2',formula:'2',value:2}]},results:['']}}
 const deliver=(value:unknown)=>{context.payload=JSON.stringify(value);return vm.runInContext('engine.request(JSON.parse(payload))',context)}
 expect(isOfficeRequest(request)).toBe(true)
 await expect(deliver(request)).rejects.toThrow('unsupported-command')
 context.engine.editing=true;await deliver(request);expect(context.engine.worker).toHaveBeenCalledOnce()
 expect(context.engine.worker.mock.calls[0][0].workbook).toEqual(request.workbook)
 for(const bad of [{...request,path:'/private'},{...request,workbook:{...request.workbook,after:{...before,endRow:1}}},{...request,workbook:{...request.workbook,before:{...before,cells:[{...cell,merged:true}]}}},{...request,workbook:{...request.workbook,before:{...before,cells:[{...cell,valueType:'command',command:'.uno:Shell'}]}}}]){
  expect(isOfficeRequest(bad)).toBe(false);await expect(deliver(bad)).rejects.toThrow('invalid-request')
 }
 expect(context.engine.worker).toHaveBeenCalledOnce()
})


test('real worker sets exact 25/50/100/200 view zoom without changing the document', () => {
  const w = worker()
  for (const zoom of [25,50,100,200]) {
    expect(w.send('local-view',{action:'set-zoom',zoom})).toMatchObject({ok:true,view:{zoom,zoomType:3,fit:false,readOnly:true,modified:false,page:null,pages:null}})
    expect(w.properties.ZoomValue).toBe(zoom)
    expect(w.send('captureSelection').state).toMatchObject({changeSequence:0,acknowledgedSequence:0,dirty:false})
  }
  expect(w.send('local-view',{action:'fit'})).toMatchObject({ok:true,view:{fit:true,zoomType:1}})
  w.send('local-view',{action:'set-zoom',zoom:20})
  expect(w.send('local-view',{action:'zoom-out'}).view.zoom).toBe(20)
  w.send('local-view',{action:'set-zoom',zoom:400})
  expect(w.send('local-view',{action:'zoom-in'}).view.zoom).toBe(400)
})
test('real worker rejects invalid presets and excess fields before setting a view property', () => {
  const w=worker(); w.setPropertyValue.mockClear()
  for(const zoom of [19,401,25.5,NaN,Infinity,'25',undefined]) expect(w.send('local-view',{action:'set-zoom',zoom}).ok).toBe(false)
  expect(w.send('local-view',{action:'fit',zoom:25})).toMatchObject({ok:false,error:'invalid-request'})
  expect(w.send('local-view',{action:'set-zoom',zoom:25,path:'/private'})).toMatchObject({ok:false,error:'invalid-request'})
  expect(w.setPropertyValue).not.toHaveBeenCalled()
  w.model.isReadonly.mockReturnValue(false)
  expect(w.send('local-view',{action:'set-zoom',zoom:25})).toMatchObject({ok:false,error:'view-unavailable'})
  expect(w.setPropertyValue).not.toHaveBeenCalled()
})
test('real worker reports actual PPTX pages and keeps unavailable page counts unavailable', () => {
  const w=worker(true,'pptx',3)
  expect(w.send('local-view',{action:'read'}).view).toMatchObject({page:1,pages:3})
  expect(w.send('local-view',{action:'next-page'}).view).toMatchObject({page:2,pages:3})
  expect(w.send('local-view',{action:'next-page'}).view).toMatchObject({page:3,pages:3})
  expect(w.send('local-view',{action:'next-page'}).view).toMatchObject({page:3,pages:3})
  expect(w.send('local-view',{action:'previous-page'}).view).toMatchObject({page:2,pages:3})
  expect(worker(true,'pptx',0).send('local-view',{action:'read'})).toMatchObject({ok:false,error:'view-unavailable'})
})

test('validateSelection crosses the existing finite request seam without edit, export or a new capture',async()=>{
  const context:any={document:{getElementById:()=>null},window:{addEventListener(){}}}
  vm.createContext(context)
  const source=readFileSync(new URL('./surface/office-surface.js',import.meta.url),'utf8').replaceAll('export ','')
  vm.runInContext(source+'\nglobalThis.engine=new OfficeEngineSurface({},()=>{});',context)
  const engine=context.engine;engine.channel='channel';engine.openOperationId='opened'
  engine.state={documentId:'document',version:'version',kind:'docx',changeSequence:0,acknowledgedSequence:0,dirty:false}
  engine.worker=vi.fn(async(r:any)=>({...r,type:'result',ok:true,state:engine.state}))
  context.request=envelope('validateSelection',{selectionToken:'selection_token',expectedChangeSequence:0})
  const call=vm.runInContext('({...request})',context)
  const reply=await engine.request(call)
  expect(reply.ok).toBe(true);expect(reply.selection).toBeUndefined()
  expect(engine.worker).toHaveBeenCalledExactlyOnceWith(call);expect(engine.editing).toBe(false)
  await expect(engine.request(vm.runInContext('({...request,script:"not-allowed"})',context))).rejects.toThrow('invalid-request')
})

function pendingReadOutcome(promise:Promise<any>) {
  const outcome:{status:'pending'|'fulfilled'|'rejected';value?:any;error?:Error;done:Promise<void>}={status:'pending',done:Promise.resolve()}
  outcome.done=promise.then(value=>{outcome.status='fulfilled';outcome.value=value},error=>{outcome.status='rejected';outcome.error=error})
  return outcome
}
function pendingDocxRead() {
  let viewOperation=0
  const context:any={document:{getElementById:()=>null},window:{addEventListener(){}},crypto:{randomUUID:()=>`view_operation_${++viewOperation}`}}
  vm.createContext(context)
  const source=readFileSync(new URL('./surface/office-surface.js',import.meta.url),'utf8').replaceAll('export ','')
  vm.runInContext(source+'\nglobalThis.engine=new OfficeEngineSurface({},()=>{});',context)
  const realm=(value:unknown)=>{context.payload=JSON.stringify(value);return vm.runInContext('JSON.parse(payload)',context)}
  const engine=context.engine,version='a'.repeat(64),changeId='b'.repeat(64)
  engine.channel='channel';engine.openOperationId='opened';engine.editing=true;engine.nativeWritable=true;engine.inputGate=false
  // Synthetic snapshots exercise identity checks only; this fixture never grants native input.
  engine.state=realm({documentId:'document',version,kind:'docx',changeSequence:0,acknowledgedSequence:0,dirty:false,
    manual:{version:2,origin:'manual',changeId,baseRevision:version,generation:1,frozen:false}})
  engine.manualInput=realm({documentId:'document',version,opened:'opened',changeId,generation:1,inputGeneration:1})
  let finish!:(reply:unknown)=>void,fail!:(error:Error)=>void,validationError:Error|undefined
  const viewReply=new Promise((resolve,reject)=>{finish=resolve;fail=reject})
  const worker=vi.fn((request:any)=>{
    if(request.command==='local-view')return viewReply
    if(request.command!=='validateSelection')throw Error('unexpected fixture command')
    if(validationError)return Promise.reject(validationError)
    const {command,channel,operationId,documentId,version}=request
    return Promise.resolve(realm({type:'result',command,channel,operationId,documentId,version,ok:true,state:engine.state}))
  })
  engine.worker=worker
  const call=realm(envelope('validateSelection',{version,selectionToken:'selection_token',expectedChangeSequence:0}))
  return {engine,worker,call,realm,
    start:(action='read')=>pendingReadOutcome(engine.localView(action)),
    validate:(current?:()=>boolean)=>pendingReadOutcome(engine.request(call,current)),
    finish:()=>finish(realm({view:{zoom:100,zoomType:3,fit:true,page:null,pages:null,readOnly:false,modified:false}})),
    fail:(error:Error)=>fail(error),setValidationError:(error:Error)=>{validationError=error}}
}

test('a menu validation waits for the current DOCX read and dispatches its unchanged native envelope once',async()=>{
  const h=pendingDocxRead(),reading=h.start(),validation=h.validate(()=>true)
  try {
    await Promise.resolve();await Promise.resolve()
    expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
    expect(validation.error?.message ?? validation.status).toBe('pending')
    h.finish();await Promise.all([reading.done,validation.done])
    expect(reading.status).toBe('fulfilled');expect(validation.status).toBe('fulfilled')
    expect(validation.value).toMatchObject({ok:true,state:{documentId:'document',changeSequence:0}})
    expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view','validateSelection'])
    expect(h.worker.mock.calls.filter(([request])=>request.command==='validateSelection').map(([request])=>request)).toEqual([h.call])
  } finally { h.finish();await Promise.all([reading.done,validation.done]) }
})
test('menu validation still rejects unrelated view work, unscoped validation and a format write while busy',async()=>{
  for(const scoped of [false,true]){
    const h=pendingDocxRead(),reading=h.start(scoped?'fit':'read'),validation=h.validate(scoped?()=>true:undefined)
    await validation.done
    expect(validation.status).toBe('rejected');expect(validation.error?.message).toBe('operation-in-progress')
    expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
    h.finish();await reading.done
  }
  const h=pendingDocxRead(),reading=h.start()
  const {channel,documentId,version}=h.call
  const formatting=pendingReadOutcome(h.engine.request(h.realm({command:'format',channel,operationId:'format_operation',documentId,version,
    changeId:h.engine.state.manual.changeId,rangeLease:{generation:1,sequence:0,token:h.call.selectionToken},format:{kind:'bold',value:true}}),()=>true))
  await formatting.done
  expect(formatting.status).toBe('rejected');expect(formatting.error?.message).toBe('operation-in-progress')
  expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
  h.finish();await reading.done
})
test('menu validation retains pending-input and composition refusals after the current read finishes',async()=>{
  for(const pending of ['input','composition'] as const){
    const h=pendingDocxRead(),reading=h.start(),validation=h.validate(()=>true)
    await Promise.resolve();await Promise.resolve()
    if(pending==='input')h.engine.inputPending=1
    else h.engine.composition={serial:1}
    h.finish();await Promise.all([reading.done,validation.done])
    expect(validation.status).toBe('rejected')
    expect(validation.error?.message).toBe(pending==='input'?'operation-in-progress':'native-composition-pending')
    expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
  }
})
test.each(['document','manual-generation','sequence'] as const)('a waiting menu validation cannot cross a changed %s authority',async changed=>{
  const h=pendingDocxRead(),reading=h.start(),validation=h.validate(()=>true)
  await Promise.resolve();await Promise.resolve()
  if(changed==='document')h.engine.state.documentId='another-document'
  else if(changed==='manual-generation')h.engine.state.manual.generation++
  else {h.engine.state.changeSequence++;h.engine.state.dirty=true}
  h.finish();await Promise.all([reading.done,validation.done])
  expect(validation.status).toBe('rejected')
  expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
})
test('cancelled input ownership or a replaced range predicate prevents waiting validation dispatch',async()=>{
  for(const cancelled of ['input-owner','range-token'] as const){
    const h=pendingDocxRead(),reading=h.start();let selectionToken=h.call.selectionToken
    const validation=h.validate(()=>selectionToken===h.call.selectionToken)
    await Promise.resolve();await Promise.resolve()
    if(cancelled==='input-owner')h.engine.closeInput()
    else selectionToken='different_selection_token'
    h.finish();await Promise.all([reading.done,validation.done])
    expect(validation.status).toBe('rejected')
    expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
  }
})
test('read and native validation operation-in-progress failures retain their error without retrying',async()=>{
  const readingCase=pendingDocxRead(),reading=readingCase.start(),waiting=readingCase.validate(()=>true)
  await Promise.resolve();await Promise.resolve()
  readingCase.fail(Error('operation-in-progress'));await Promise.all([reading.done,waiting.done])
  expect(reading.error?.message).toBe('operation-in-progress');expect(waiting.error?.message).toBe('operation-in-progress')
  expect(waiting.status).toBe('rejected')
  expect(readingCase.worker.mock.calls.map(([request])=>request.command)).toEqual(['local-view'])
  const nativeCase=pendingDocxRead();nativeCase.setValidationError(Error('operation-in-progress'))
  const validation=nativeCase.validate(()=>true);await validation.done
  expect(validation.status).toBe('rejected');expect(validation.error?.message).toBe('operation-in-progress')
  expect(nativeCase.worker.mock.calls.map(([request])=>request.command)).toEqual(['validateSelection'])
})
