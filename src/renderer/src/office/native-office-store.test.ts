// @vitest-environment jsdom
import {beforeEach,afterEach,expect,it,vi} from 'vitest'
import {useNativeOfficeStore,nativeOfficeTargetKey,nativeOfficeManualPhase,nativeOfficeHasUnresolvedChange,readNativeOfficeCreateReadiness,nativeOfficeCreateErrorKey,nativeWorkbookDraftKey,type NativeWorkbookCellDraft} from './native-office-store'
import type {NativeOfficeView,NativeOfficeRequest} from '@shared/native-office'
const selector={threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
const view:NativeOfficeView={objectId:'d'.repeat(64),path:'/workspace/report.docx',kind:'docx',revision:'e'.repeat(64),dirty:false,status:'ready'}
let request:ReturnType<typeof vi.fn>
beforeEach(()=>{request=vi.fn(async()=>({ok:true,view:null}));Object.defineProperty(window,'analytix',{configurable:true,value:{office:{request}}});useNativeOfficeStore.setState({target:null,view:null,engineViews:{},views:{},workbookDrafts:{},workbookClosing:{},workbookDraftInputFrozen:false,workbookDraftComposing:false,error:null,creationAttempt:null,manualOwners:{}})})
afterEach(()=>{Reflect.deleteProperty(window,'analytix');vi.restoreAllMocks()})
it('keeps same-file ordinary and Present views isolated by complete source selector',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',view.path);store.receive(view)
  await store.selectPresent('/workspace','display.docx',selector);store.receive(view,'unavailable')
  expect(useNativeOfficeStore.getState().view).toBeNull()
  const privateView={...view,objectId:'f'.repeat(64),protectedPresent:selector};store.receive(privateView)
  expect(useNativeOfficeStore.getState().view).toEqual(privateView)
  store.receive({...privateView,protectedPresent:{...selector,snapshotDigest:'a'.repeat(64)}})
  expect(useNativeOfficeStore.getState().view).toEqual(privateView)
  await store.select('/workspace',view.path);expect(useNativeOfficeStore.getState().view).toEqual(view)
})
it('a pending original close cannot clear or close a fresh same-selector selection',async()=>{
  const store=useNativeOfficeStore.getState();await store.selectPresent('/workspace','display.docx',selector)
  store.receive({...view,protectedPresent:selector})
  let finish!:(value:{ok:boolean;view:null})=>void
  request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  const close=store.closePresent(selector);await store.selectPresent('/workspace','display.docx',{...selector})
  const owner=useNativeOfficeStore.getState().target!;finish({ok:true,view:null});await close
  expect(useNativeOfficeStore.getState().target).toBe(owner);expect(nativeOfficeTargetKey(owner)).toContain('present')
})

const manual:NonNullable<NativeOfficeView['manual']>={version:2,origin:'manual',changeId:'f'.repeat(64),baseRevision:view.revision,generation:3,frozen:false}
it('keeps annotation mode distinct from actual native manual authority',()=>{
  expect(nativeOfficeManualPhase({...view,editing:true})).toBe('preview')
  expect(nativeOfficeManualPhase({...view,editing:true,manual})).toBe('active')
  expect(nativeOfficeManualPhase({...view,editing:true,manual:{...manual,frozen:true}})).toBe('frozen')
  expect(nativeOfficeManualPhase({...view,editing:true,manual,saving:true})).toBe('frozen')
  for(const next of [null,{...view,status:'loading' as const},{...view,kind:'xlsx' as const,editing:true,manual},
    {...view,editing:false,manual},{...view,editing:true,manual:{...manual,baseRevision:'a'.repeat(64)}},
    {...view,protectedPresent:selector,editing:true,manual}]) expect(nativeOfficeManualPhase(next)).toBe('unavailable')
})
it('retains manual recovery metadata only under the matching ordinary owner',async()=>{
  const recovery:NonNullable<NativeOfficeView['recovery']>={current:null,pending:{version:2,origin:'manual',changeId:manual.changeId,threadId:'thread-a',
    baseRevision:view.revision,revision:'',status:'prepared',saveOperationId:'save_manual_001',undoOperationId:'undo_manual_001',
    canUndo:false,canCancel:true,canRetryUndo:false,canResume:false,createdAt:'2026-10-15T00:00:00Z',savedAt:''}}
  const ordinary={...view,editing:true,dirty:true,manual,recovery}
  const store=useNativeOfficeStore.getState();await store.select('/workspace',view.path);store.receive(ordinary)
  expect(useNativeOfficeStore.getState().view).toEqual(ordinary)
  await store.selectPresent('/workspace','display.docx',selector)
  const presented={...view,objectId:'a'.repeat(64),protectedPresent:selector};store.receive(presented);store.receive(ordinary)
  expect(useNativeOfficeStore.getState().view).toEqual(presented)
  await store.select('/workspace',view.path)
  expect(useNativeOfficeStore.getState().view?.recovery?.pending).toEqual(recovery.pending)
})

it('keeps unknown and conflict state distinct from a retryable failed save',()=>{
  expect(nativeOfficeHasUnresolvedChange(view,null)).toBe(false)
  expect(nativeOfficeHasUnresolvedChange(view,'save_failed')).toBe(false)
  for(const error of ['unknown','conflict']) {
    expect(nativeOfficeHasUnresolvedChange(view,error)).toBe(true)
    expect(nativeOfficeHasUnresolvedChange({...view,error:error as 'unknown'|'conflict'},null)).toBe(true)
  }
})

const createInput={threadId:'thread-a',fileName:'new.docx',bounds:{x:0,y:0,width:100,height:100},appearance:{theme:'light' as const,reducedMotion:false}}
function creation(input:Extract<NativeOfficeRequest,{action:'create'}>){return {operationId:input.operationId,workspace:'/workspace',path:'/workspace/'+input.fileName,
  revision:'a'.repeat(64),kind:'docx' as const,byteSize:128,savedAt:'2026-10-15T00:00:00Z'}}
it.each(['../bad.docx','.docx','wrong.txt',' padded.docx','bad/name.docx'])('rejects invalid create basename %s before IPC',async fileName=>{
  const result=await useNativeOfficeStore.getState().createDocument({...createInput,fileName})
  expect(result).toMatchObject({ok:false,error:'invalid_request'})
  expect(request).not.toHaveBeenCalled();expect(useNativeOfficeStore.getState().target).toBeNull()
})
it('sends one create for duplicate pending clicks and does not invent an Office view or object ID',async()=>{
  let finish!:(value:unknown)=>void
  request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  const store=useNativeOfficeStore.getState(),pending=store.createDocument(createInput)
  expect(await store.createDocument(createInput)).toBeUndefined()
  expect(request).toHaveBeenCalledOnce()
  const input=request.mock.calls[0][0] as Extract<NativeOfficeRequest,{action:'create'}>
  expect(input).toMatchObject({action:'create',threadId:'thread-a',fileName:'new.docx',bounds:createInput.bounds})
  expect(input.operationId).toBeTruthy();expect(input).not.toHaveProperty('workspace');expect(input).not.toHaveProperty('path')
  finish({ok:true,view:null,creation:creation(input)});await pending
  expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({phase:'created',creation:creation(input)})
  expect(useNativeOfficeStore.getState().view).toBeNull();expect(useNativeOfficeStore.getState().target).toBeNull()
})
it('retries an unknown create with the original operation and blocks replacement payloads',async()=>{
  request.mockRejectedValueOnce(new Error('synthetic lost reply'))
  const store=useNativeOfficeStore.getState();await store.createDocument(createInput)
  const original=request.mock.calls[0][0] as Extract<NativeOfficeRequest,{action:'create'}>
  expect(useNativeOfficeStore.getState().creationAttempt?.phase).toBe('unknown')
  expect(await store.createDocument({...createInput,threadId:'thread-b'})).toBeUndefined()
  expect(await store.createDocument({...createInput,fileName:'other.docx'})).toBeUndefined()
  request.mockImplementationOnce(async(input:Extract<NativeOfficeRequest,{action:'create'}>)=>({ok:true,view:null,creation:creation(input)}))
  await store.createDocument({...createInput,bounds:{...createInput.bounds,width:200}})
  expect(request).toHaveBeenCalledTimes(2)
  expect(request.mock.calls[1][0]).toMatchObject({operationId:original.operationId,threadId:original.threadId,fileName:original.fileName})
})
it.each(['missing','operation','basename'] as const)('retains an unresolved create without accepting a %s receipt',async failure=>{
  request.mockImplementationOnce(async(input:Extract<NativeOfficeRequest,{action:'create'}>)=>({ok:true,view:null,
    ...(failure==='missing'?{}:{creation:{...creation(input),...(failure==='operation'?{operationId:'other_operation_001'}:{path:'/workspace/other.docx'})}})}))
  const result=await useNativeOfficeStore.getState().createDocument(createInput)
  expect(result).toMatchObject({ok:false,error:'invalid_response'});expect(result?.creation).toBeUndefined()
  expect(useNativeOfficeStore.getState().creationAttempt?.phase).toBe('unknown')
  expect(useNativeOfficeStore.getState().target).toBeNull()
})
it('retains a committed creation receipt when its native preview cannot open',async()=>{
  request.mockImplementationOnce(async(input:Extract<NativeOfficeRequest,{action:'create'}>)=>({ok:false,error:'engine_unavailable',view:null,creation:creation(input)}))
  const result=await useNativeOfficeStore.getState().createDocument(createInput)
  expect(result).toMatchObject({ok:false,error:'engine_unavailable',creation:{path:'/workspace/new.docx'}})
  expect(useNativeOfficeStore.getState().creationAttempt?.phase).toBe('created')
  expect(request).toHaveBeenCalledOnce();expect(useNativeOfficeStore.getState().view).toBeNull()
})
it('preserves a Core permission refusal without opening or replacing the current document',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',view.path);store.receive(view);request.mockClear()
  request.mockResolvedValueOnce({ok:false,error:'forbidden',view:null})
  expect(await store.createDocument(createInput)).toMatchObject({ok:false,error:'forbidden'})
  expect(useNativeOfficeStore.getState().creationAttempt?.phase).toBe('failed')
  expect(useNativeOfficeStore.getState().view).toEqual(view);expect(useNativeOfficeStore.getState().target?.path).toBe(view.path)
  expect(request).toHaveBeenCalledOnce()
})


it('keeps the newer native format selection when an older same-session view arrives',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',view.path)
  const before:NativeOfficeView={...view,editing:true,manual,changeSequence:2}
  store.receive(before)
  const current:NativeOfficeView={...before,dirty:true,changeSequence:3,selection:{kind:'text',documentId:view.objectId,version:view.revision,changeSequence:3,token:'fresh_format_token',scope:'session-text-range-at-version-and-change-sequence',text:'Selected',rangeLease:{generation:manual.generation,sequence:3,token:'fresh_format_token'},formatState:{fontFamily:null,fontSize:12,bold:true,italic:null,underline:false}}}
  store.receive(current)
  const received=useNativeOfficeStore.getState().receiveSequence
  store.receive(before,'stale_selection')
  expect(useNativeOfficeStore.getState().view).toEqual(current)
  expect(useNativeOfficeStore.getState().error).toBeNull()
  expect(useNativeOfficeStore.getState().receiveSequence).toBe(received)
})
it('retires remembered manual ownership when Host changes the actual session',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',view.path)
  const owner={owner:nativeOfficeTargetKey({workspace:'/workspace',path:view.path}),objectId:view.objectId,threadId:'thread-a',changeId:manual.changeId,baseRevision:view.revision,generation:manual.generation}
  useNativeOfficeStore.setState({manualOwners:{[view.objectId]:owner}})
  store.receive({...view,editing:true,manual})
  expect(useNativeOfficeStore.getState().manualOwners[view.objectId]).toEqual(owner)
  store.receive({...view,editing:true,manual:{...manual,generation:manual.generation+1}})
  expect(useNativeOfficeStore.getState().manualOwners[view.objectId]).toBeUndefined()
})

it('keeps ordinary manual ownership isolated from a protected view of the same object',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',view.path)
  const owner={owner:nativeOfficeTargetKey({workspace:'/workspace',path:view.path}),objectId:view.objectId,threadId:'thread-a',changeId:manual.changeId,baseRevision:view.revision,generation:manual.generation}
  useNativeOfficeStore.setState({manualOwners:{[view.objectId]:owner}});store.receive({...view,editing:true,manual})
  await store.selectPresent('/workspace',view.path,selector);store.receive({...view,protectedPresent:selector})
  expect(useNativeOfficeStore.getState().manualOwners[view.objectId]).toEqual(owner)
})

const readyDocuments = () => ({packageId:'analytix-documents' as const,packageVersion:'1.0.0',displayName:'Documents',
  origin:'development-source' as const,publishable:false as const,materialized:true,generationId:'a'.repeat(64),
  activationState:'recorded' as const,desiredState:'enabled' as const,activationRevision:1,activationId:'b'.repeat(64),available:true,
  operations:['create-object','open-object','close-object']})

it.each(['ready','unset','disabled','missing','unmaterialized','operation','malformed','duplicates','failed','throw'] as const)(
  'reads actual public Documents readiness without enabling packages: %s',async kind=>{
    const pkg=readyDocuments()
    const unset={...pkg,activationState:'unset',activationRevision:0,available:false}
    Reflect.deleteProperty(unset,'desiredState');Reflect.deleteProperty(unset,'activationId')
    const packages=kind==='missing'?[]:kind==='unset'?[unset]:kind==='disabled'?[{...pkg,desiredState:'disabled',available:false}]
      :kind==='unmaterialized'?[{...pkg,materialized:false,generationId:'',available:false}]
      :kind==='operation'?[{...pkg,operations:['open-object','close-object']}]
      :kind==='malformed'?[{...pkg,available:'true'}]:kind==='duplicates'?[pkg,pkg]:[pkg]
    const list=vi.fn(async()=>{if(kind==='throw')throw new Error('synthetic list failure');return kind==='failed'?{ok:false,code:'unavailable',message:'Synthetic'}:{ok:true,packages}})
    Object.assign(window.analytix,{packageHost:{request:list}})
    const expected=kind==='ready'?'ready':['unset','disabled'].includes(kind)?'not-enabled'
      :['missing','unmaterialized','operation'].includes(kind)?'unavailable':'unknown'
    expect(await readNativeOfficeCreateReadiness()).toBe(expected)
    expect(list).toHaveBeenCalledExactlyOnceWith({action:'list'});expect(request).not.toHaveBeenCalled()
  })
it.each(['package_unavailable','package_changed','forbidden','conflict','invalid_request','capacity'] as const)(
  'keeps an unknown create unresolved after replay refusal %s until a matching receipt arrives',async error=>{
    request.mockRejectedValueOnce(new Error('synthetic lost reply'))
    const store=useNativeOfficeStore.getState();await store.createDocument(createInput)
    const original=useNativeOfficeStore.getState().creationAttempt!.input
    request.mockResolvedValueOnce({ok:false,error,view:null})
    await store.createDocument(createInput)
    expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({phase:'unknown',input:original,error})
    expect(await store.createDocument({...createInput,fileName:'other.docx'})).toBeUndefined()
    expect(request).toHaveBeenCalledTimes(2)
    request.mockImplementationOnce(async(input:Extract<NativeOfficeRequest,{action:'create'}>)=>({ok:true,view:null,creation:creation(input)}))
    await store.createDocument(createInput)
    expect(request.mock.calls[2][0]).toMatchObject(original)
    expect(useNativeOfficeStore.getState().creationAttempt).toMatchObject({phase:'created',input:original})
  })
it.each([
  ['package_unavailable','workspaceWordDocumentsNotReady'],['package_changed','workspaceWordDocumentsChanged'],
  ['invalid_request','workspaceWordInvalidRequest'],['capacity','workspaceWordCapacity'],
  ['forbidden','nativeOfficeForbidden'],['conflict','workspaceWordNameConflict'],['engine_unavailable','workspaceWordCreateFailed']
])('maps creation error %s without blanket filename advice', (error,key)=>{expect(nativeOfficeCreateErrorKey(error)).toBe(key)})

const bookCell:NativeWorkbookCellDraft['cell']={sheet:0,column:0,row:0,text:'12',value:12,formula:'12',valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}
const book:NativeOfficeView={...view,path:'/workspace/book.xlsx',kind:'xlsx',workbookEditing:{editable:true}}
const bookDraftKey=nativeWorkbookDraftKey('/workspace',book.path,'thread-a')
function workbookDraft(overrides:Partial<NativeWorkbookCellDraft>={}):NativeWorkbookCellDraft {
  return {id:'local-draft',workspace:'/workspace',path:book.path,threadId:'thread-a',objectId:book.objectId,revision:book.revision,
    sheetName:'Sheet1',cell:bookCell,type:'text',value:'001',changed:true,...overrides}
}
it.each(['frozen','closing'] as const)('workbook input mode intent is retained and cannot change while %s',state=>{
  const store=useNativeOfficeStore.getState(),draft=workbookDraft({inputMode:'auto'})
  store.setWorkbookDraft(bookDraftKey,draft)
  useNativeOfficeStore.setState(state === 'frozen' ? {workbookDraftInputFrozen:true} : {workbookClosing:{[nativeOfficeTargetKey({workspace:'/workspace',path:book.path})]:true}})
  store.setWorkbookDraft(bookDraftKey,{...draft,inputMode:'text'},draft.id)
  expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]).toBe(draft)
  useNativeOfficeStore.setState({workbookDraftInputFrozen:false,workbookClosing:{}})
  store.setWorkbookDraft(bookDraftKey,{...draft,inputMode:'text'},draft.id)
  expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]).toMatchObject({inputMode:'text',type:'text',value:'001'})
})
type WorkbookChange=Extract<NonNullable<NonNullable<NativeOfficeView['recovery']>['current']>,{version:3}>
function workbookChange(overrides:Partial<WorkbookChange>={}):WorkbookChange {
  const bounds={sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}
  return {version:3,origin:'manual',changeId:'f'.repeat(64),threadId:'thread-a',baseRevision:book.revision,revision:'a'.repeat(64),status:'committed',
    workbook:{before:{...bounds,cells:[bookCell]},after:{...bounds,cells:[{...bookCell,valueType:'text',text:'001',formula:'001',value:0}]},results:['001']},
    saveOperationId:'save-operation',undoOperationId:'undo-operation',canUndo:true,canCancel:false,canRetryUndo:false,canResume:false,createdAt:'',savedAt:'',...overrides}
}
it('projects local workbook dirtiness into existing workspace views without changing native authority',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',book.path);store.receive(book)
  store.setWorkbookDraft(bookDraftKey,workbookDraft())
  const identity=nativeOfficeTargetKey({workspace:'/workspace',path:book.path})
  expect(useNativeOfficeStore.getState().views[identity].dirty).toBe(true)
  expect(useNativeOfficeStore.getState().engineViews[identity]).toBe(book)
  expect(useNativeOfficeStore.getState().view?.dirty).toBe(false)
  await store.select('/workspace',view.path);store.receive(view);await store.select('/workspace',book.path)
  expect(useNativeOfficeStore.getState().view).toBe(book)
  expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey].value).toBe('001')
  expect(useNativeOfficeStore.getState().views[identity].dirty).toBe(true)
})
it('vetoes tab close before IPC until the retained workbook draft is explicitly discarded or saved',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',book.path);store.receive(book)
  store.setWorkbookDraft(bookDraftKey,workbookDraft());request.mockClear()
  expect(await store.close('/workspace',book.path)).toBe(false)
  expect(request).not.toHaveBeenCalled()
  const retained=useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]
  expect(retained).toMatchObject({value:'001',closeRequested:true})
  store.setWorkbookDraft(bookDraftKey,{...retained,closeRequested:false},retained.id)
  expect(await store.close('/workspace',book.path)).toBe(false)
  expect(request).not.toHaveBeenCalled()
  store.setWorkbookDraft(bookDraftKey,null,retained.id)
  expect(await store.close('/workspace',book.path)).toBe(true)
  expect(request).toHaveBeenCalledExactlyOnceWith({action:'close',objectId:book.objectId})
  expect(useNativeOfficeStore.getState().target).toBeNull()
})
it.each([true,false])('fences first input during a pending native close and releases the fence after result %s',async ok=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',book.path);store.receive(book)
  let finish!:(result:{ok:boolean;view:null})=>void
  request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  const closing=store.close('/workspace',book.path)
  store.setWorkbookDraft(bookDraftKey,workbookDraft())
  expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]).toBeUndefined()
  expect(await store.close('/workspace',book.path)).toBe(false)
  finish({ok,view:null});expect(await closing).toBe(ok)
  expect(useNativeOfficeStore.getState().workbookClosing).toEqual({})
  if (!ok) {
    store.setWorkbookDraft(bookDraftKey,workbookDraft())
    expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey].value).toBe('001')
  }
})
it('keeps an unconfirmed draft until a query proves its exact target, base, thread, operation and typed value',async()=>{
  const store=useNativeOfficeStore.getState();await store.select('/workspace',book.path);store.receive(book)
  const draft=workbookDraft({unconfirmed:true,submitted:{edit:{type:'text',text:'001'},changeId:'f'.repeat(64),saveOperationId:'save-operation'}})
  store.setWorkbookDraft(bookDraftKey,draft)
  const expected=workbookChange()
  expected.workbook!.before.cells=[Object.fromEntries(Object.entries(bookCell).reverse()) as typeof bookCell]
  const wrongChanges=[workbookChange({threadId:'other-thread'}),workbookChange({baseRevision:'b'.repeat(64)}),
    workbookChange({changeId:'b'.repeat(64)}),workbookChange({saveOperationId:'other-operation'}),
    workbookChange({workbook:{...expected.workbook!,before:{...expected.workbook!.before,cells:[{...bookCell,column:1}]}}}),
    workbookChange({workbook:{...expected.workbook!,after:{...expected.workbook!.after,cells:[{...bookCell,valueType:'text',text:'other',formula:'other',value:0}]}}})]
  for (const change of wrongChanges) {
    store.receive({...book,revision:change.revision,recovery:{current:change,pending:null}})
    expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]).toEqual(draft)
  }
  for (const phase of [{status:'loading' as const,saving:true},{status:'loading' as const,saving:false},{status:'error' as const,saving:false}]) {
    store.receive({...book,...phase,revision:expected.revision,recovery:{current:expected,pending:null}})
    expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]).toEqual(draft)
  }
  store.receive({...book,revision:expected.revision,recovery:{current:expected,pending:null}})
  expect(useNativeOfficeStore.getState().workbookDrafts[bookDraftKey]).toBeUndefined()
  expect(useNativeOfficeStore.getState().views[nativeOfficeTargetKey({workspace:'/workspace',path:book.path})].dirty).toBe(false)
  expect(request.mock.calls.every(([input])=>input.action === 'hide')).toBe(true)
})
