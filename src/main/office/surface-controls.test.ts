// @vitest-environment jsdom
import html from './surface/office-surface.html?raw'
import source from './surface/office-surface.js?raw'
import { expect, test, vi } from 'vitest'

function trackWindowListeners() {
  const listeners: Array<{target:EventTarget; type:string; listener:EventListenerOrEventListenerObject; options?:boolean|AddEventListenerOptions}> = []
  const add = window.addEventListener.bind(window)
  const spy = vi.spyOn(window,'addEventListener').mockImplementation((type,listener,options) => {
    listeners.push({target:window,type,listener,options}); add(type,listener,options)
  })
  return Object.assign(() => { spy.mockRestore(); for (const entry of listeners) window.removeEventListener(entry.type,entry.listener,entry.options) },{listeners})
}

test('surface forwards actual view state through its existing typed bridge and retires duplicate footer controls', async () => {
  document.body.innerHTML = new DOMParser().parseFromString(html,'text/html').body.innerHTML
  expect(document.querySelector('#viewControls')).toBeNull()
  expect(document.querySelector('[data-view]')).toBeNull()
  vi.useFakeTimers()
  const releaseListeners=trackWindowListeners()
  try {
    let receive!: (request: unknown) => Promise<void>
    const send=vi.fn()
    vi.stubGlobal('analytixOfficeSurface',{connect:(handler:typeof receive) => {receive=handler},send})
    const Engine=new Function(source.replaceAll('export ','')+'\nreturn OfficeEngineSurface;')()
    Engine.prototype.start=async function(channel:string){this.channel=channel;this.state={kind:'pptx',documentId:'document',version:'version',changeSequence:0,acknowledgedSequence:0,dirty:false};this.openOperationId='opened'}
    let zoom=100
    const worker=vi.fn(async (request:any) => {
      if(request.action==='set-zoom') zoom=request.zoom
      return {view:{zoom,zoomType:3,fit:false,page:2,pages:3,readOnly:true,modified:false}}
    })
    Engine.prototype.worker=worker
    await receive({type:'bind',channel:'channel'})
    await receive({command:'local-view',action:'set-zoom',zoom:25,channel:'channel',operationId:'preset',documentId:'document',version:'version'})
    expect(worker).toHaveBeenCalledWith(expect.objectContaining({command:'local-view',action:'set-zoom',zoom:25}))
    expect(send).toHaveBeenCalledWith(expect.objectContaining({type:'view',operationId:'opened',documentId:'document',version:'version',view:expect.objectContaining({zoom:25,page:2,pages:3})}))
    expect(send).toHaveBeenCalledWith(expect.objectContaining({type:'result',command:'local-view',operationId:'preset',ok:true,view:expect.objectContaining({zoom:25})}))
    document.getElementById('qtcanvas')!.dispatchEvent(new Event('wheel'))
    await vi.advanceTimersByTimeAsync(180)
    expect(worker).toHaveBeenLastCalledWith(expect.objectContaining({action:'read'}))
    worker.mockRejectedValueOnce(new Error('view-unavailable'))
    await receive({command:'local-view',action:'read',channel:'channel',operationId:'missing',documentId:'document',version:'version'})
    expect(send).toHaveBeenCalledWith(expect.objectContaining({type:'view',operationId:'opened',view:null}))
    expect(send).toHaveBeenCalledWith(expect.objectContaining({type:'result',operationId:'missing',ok:false,error:'view-unavailable'}))
    expect(document.getElementById('footer')!.dataset.error).toBe('true')
    // Local wheel/resize reads have no renderer result announcement. Their
    // native error must remain visible after the regular footer is retired.
    await receive({command:'local-view',action:'read',channel:'channel',operationId:'restored',documentId:'document',version:'version'})
    expect(document.getElementById('footer')!.dataset.error).toBe('false')
    worker.mockRejectedValueOnce(new Error('view-unavailable'))
    document.getElementById('qtcanvas')!.dispatchEvent(new Event('wheel'))
    await vi.advanceTimersByTimeAsync(180)
    expect(document.getElementById('footer')!.dataset.error).toBe('true')
    expect(document.getElementById('status')!.textContent).toContain('视图调整不可用')
  } finally {releaseListeners();vi.clearAllTimers();vi.useRealTimers();vi.unstubAllGlobals();document.body.innerHTML=''}
})

test.each(['offered','format-choice'] as const)('the installed menu bridge waits for its pending DOCX read during %s validation',async phase=>{
  document.body.innerHTML=new DOMParser().parseFromString(html,'text/html').body.innerHTML
  vi.useFakeTimers()
  const release=trackWindowListeners(),captured=captureDomListeners(),send=vi.fn()
  try {
    let receive!:(request:any)=>Promise<void>,engine:any,finishRead!:(reply:any)=>void
    vi.stubGlobal('analytixOfficeSurface',{connect:(handler:typeof receive)=>{receive=handler},send})
    const Engine=new Function(source.replaceAll('export ','')+'\nreturn OfficeEngineSurface;')()
    const state={kind:'docx',documentId:'document',version:manualRevision,changeSequence:0,acknowledgedSequence:0,dirty:false,
      manual:{version:2,origin:'manual',changeId:manualChange,baseRevision:manualRevision,generation:1,frozen:false}}
    const view={zoom:100,zoomType:3,fit:false,page:null,pages:null,readOnly:false,modified:false}
    Engine.prototype.start=async function(channel:string){
      engine=this;Object.assign(this,{channel,state,view,openOperationId:'opened',editing:true,nativeWritable:true,inputGate:false,
        manualInput:{changeId:manualChange,generation:1},manualAvailable:()=>true})
    }
    const worker=vi.fn((request:any)=>request.command==='local-view'
      ? new Promise(resolve=>{finishRead=resolve})
      : Promise.resolve({...request,type:'result',ok:true,state:{...state,manual:{...state.manual}}}))
    Engine.prototype.worker=worker
    await receive({type:'bind',channel:'channel'})
    const canvas=document.getElementById('qtcanvas')!,selected={kind:'text',documentId:'document',version:manualRevision,changeSequence:0,
      token:'menu_selection_01',text:'Synthetic Word text',scope:'body',rangeLease:{generation:1,sequence:0,token:'menu_selection_01'},formatState:{...pointerFormatState}}
    captured.fire(canvas,'pointerdown',{button:0,isPrimary:true,pointerId:1})
    engine.onEvent({type:'selection',channel:'channel',operationId:'opened',documentId:'document',version:manualRevision,selection:selected})
    engine.onSelectionIdentity({eligible:true,changed:true,selectionToken:selected.token})
    captured.fire(canvas,'pointerup',{button:0,isPrimary:true,pointerId:1,clientX:30,clientY:40})
    const lease=send.mock.calls.find(([message])=>message.phase==='request')![0]
    if(phase==='format-choice'){
      await receive({...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],
        formatting:{changeId:manualChange,rangeLease:{...selected.rangeLease},formatState:{...pointerFormatState},labels:{...pointerFormatLabels}}})
      captured.fire(document.querySelector('[data-format-control="bold"]')!,'click')
      expect(send.mock.calls.some(([message])=>message.phase==='format')).toBe(true)
    }
    const reading=engine.localView('read')
    let settled=false
    const request={command:'validateSelection',channel:'channel',operationId:'menu_validate_01',documentId:'document',version:manualRevision,
      selectionToken:selected.token,expectedChangeSequence:0}
    const validating=receive(request).then(()=>{settled=true})
    for(let i=0;i<4;i++)await Promise.resolve()
    expect(settled).toBe(false)
    expect(worker).toHaveBeenCalledTimes(1)
    expect(send.mock.calls.some(([message])=>message.phase==='cancel')).toBe(false)
    finishRead({view:{...view}});await reading;await validating
    expect(worker.mock.calls.map(([message])=>message.command)).toEqual(['local-view','validateSelection'])
    expect(worker).toHaveBeenLastCalledWith(request)
    expect(send).toHaveBeenCalledWith(expect.objectContaining({command:'validateSelection',operationId:'menu_validate_01',ok:true}))
    expect(send.mock.calls.some(([message])=>message.phase==='cancel')).toBe(false)
    expect(engine.state).toMatchObject({changeSequence:0,dirty:false,manual:{generation:1,frozen:false}})
  } finally {captured.restore();release();vi.clearAllTimers();vi.useRealTimers();vi.unstubAllGlobals();document.body.innerHTML=''}
})

const editorView={zoom:175,zoomType:3,fit:true,page:5,pages:5,readOnly:true,modified:false}
function surfaceFixture() {
  document.body.innerHTML='<canvas id="canvas" tabindex="0"></canvas>'
  const release=trackWindowListeners(), canvas=document.getElementById('canvas')!
  const {Engine,validRequest,validPreviewView}=new Function(source.replaceAll('export ','')+'\nreturn {Engine:OfficeEngineSurface,validRequest,validPreviewView};')()
  const onEvent=vi.fn(),engine=new Engine(canvas,onEvent)
  engine.channel='channel';engine.openOperationId='opened'
  engine.state={kind:'pptx',documentId:'document',version:'version',changeSequence:0,acknowledgedSequence:0,dirty:false}
  engine.view={...editorView,playback:{generation:1,sequence:1,phase:'running',slideIndex:-1,slideCount:2}}
  let sequence=1
  const worker=vi.fn(async (request:any)=>({view:{...editorView,playback:{...engine.view.playback,sequence:++sequence,
    ...(request.action==='play-end'?{generation:engine.view.playback.generation+1,phase:'idle',slideIndex:null,slideCount:null}:{})}}}))
  engine.worker=worker
  const event=(view:any,extra:any={})=>engine.receiveView({type:'view',channel:'channel',documentId:'document',version:'version',operationId:'opened',view,...extra})
  return {engine,worker,onEvent,event,canvas,validRequest,validPreviewView,close:()=>{release();document.body.innerHTML=''}}
}

test('XLSX F2 and double-click request a bounded cell editor while native body input stays closed',async()=>{
  const f=surfaceFixture()
  try {
    f.engine.state={...f.engine.state,kind:'xlsx'};f.engine.view=null
    let sequence=0
    const selection=()=>({documentId:'document',version:'version',changeSequence:0,kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',
      token:`selection_cell_${++sequence}`,ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:2,endRow:2}],
      cells:[{sheet:0,column:0,row:2,text:'3',formula:'3',value:3,valueType:'number',numberFormat:7,rowVisible:true,columnVisible:true,merged:false}],
      text:'3',capture:{capturedCharacters:1,totalCharacters:1,truncated:false,unit:'utf-16',complete:true}})
    f.worker.mockImplementation(async(request:any):Promise<any>=>({...request,type:'result',ok:true,state:{...f.engine.state},selection:selection()}))
    const key=new KeyboardEvent('keydown',{key:'F2',bubbles:true,cancelable:true})
    f.canvas.dispatchEvent(key)
    await vi.waitFor(()=>expect(f.onEvent).toHaveBeenCalledWith(expect.objectContaining({type:'cell-edit-requested',intent:'f2',selection:expect.objectContaining({token:'selection_cell_1'})})))
    expect(key.defaultPrevented).toBe(true)
    const doubleClick=new MouseEvent('dblclick',{button:0,bubbles:true,cancelable:true})
    f.canvas.dispatchEvent(doubleClick)
    await vi.waitFor(()=>expect(f.onEvent).toHaveBeenCalledWith(expect.objectContaining({type:'cell-edit-requested',intent:'double-click',selection:expect.objectContaining({token:'selection_cell_2'})})))
    expect(doubleClick.defaultPrevented).toBe(true)
    expect(f.worker.mock.calls.map(([request])=>request.command)).toEqual(['captureSelection','captureSelection'])
    expect(f.engine.editing).toBe(false);expect(f.engine.nativeWritable).toBe(false);expect(f.engine.manualInput).toBeNull()
    const typing=new KeyboardEvent('keydown',{key:'a',bubbles:true,cancelable:true})
    f.canvas.dispatchEvent(typing);expect(typing.defaultPrevented).toBe(true)
  }finally{f.close()}
})

test('cell editor intent ignores multiple cells and a result that races the base revision',async()=>{
  const f=surfaceFixture()
  try {
    f.engine.state={...f.engine.state,kind:'xlsx'};f.engine.view=null
    const selected={kind:'cells',token:'selection_cell_01',ranges:[{startColumn:0,endColumn:0,startRow:0,endRow:1}],cells:[{},{}],capture:{complete:true}}
    f.worker.mockImplementation(async(request:any):Promise<any>=>({...request,state:{...f.engine.state},selection:selected}))
    await f.engine.requestCellEdit('f2');expect(f.onEvent).not.toHaveBeenCalled()
    f.worker.mockImplementation(async(request:any):Promise<any>=>{f.engine.state={...f.engine.state,version:'newer'};return {...request,state:{...f.engine.state},selection:{...selected,ranges:[{startColumn:0,endColumn:0,startRow:0,endRow:0}],cells:[{}]}}})
    await f.engine.requestCellEdit('double-click');expect(f.onEvent).not.toHaveBeenCalled()
  }finally{f.close()}
})

test('read-only dependency captures admit only bounded rectangles and preserve separate write-target validation',()=>{
  const f=surfaceFixture()
  try {
    const request={command:'captureCells',channel:'channel',operationId:'capture_01',documentId:'document',version:'version',selectionToken:'selection_cell_01',expectedChangeSequence:0,
      bounds:{sheet:0,startColumn:0,endColumn:0,startRow:0,endRow:2}}
    expect(f.validRequest(request)).toBe(true)
    expect(f.validRequest({...request,bounds:{...request.bounds,endRow:256}})).toBe(false)
    expect(f.validRequest({...request,bounds:{...request.bounds,path:'/tmp/other'}})).toBe(false)
  }finally{f.close()}
})

const manualRevision='a'.repeat(64), manualChange='b'.repeat(64)
const manualAuthority={version:2,origin:'manual',changeId:manualChange,baseRevision:manualRevision,saveOperationId:'save_operation_01',undoOperationId:'undo_operation_01'}
function manualSurfaceFixture(registerRuntime = true) {
  document.body.innerHTML='<canvas id="canvas" contenteditable="false" tabindex="0"></canvas>'
  const release=trackWindowListeners(),canvas=document.getElementById('canvas')!
  const {Engine,validRequest,validPreviewView,assetVersion}=new Function(source.replaceAll('export ','')+'\nreturn {Engine:OfficeEngineSurface,validRequest,validPreviewView,assetVersion:INPUT_ASSET_VERSION};')()
  const onEvent=vi.fn(),engine=new Engine(canvas,onEvent)
  const registrations=[['registerKeyEventCallback','keydown',2],['registerKeyEventCallback','keyup',3],['registerMouseEventCallback','mousedown',5],['registerMouseEventCallback','mouseup',6],['registerMouseEventCallback','mousemove',8]] as const
  const originalKey=vi.fn(()=>0),originalMouse=vi.fn(()=>0)
  vi.stubGlobal('registerKeyEventCallback',originalKey);vi.stubGlobal('registerMouseEventCallback',originalMouse)
  vi.stubGlobal('findEventTarget',(target:unknown)=>target==='#canvas'?canvas:null)
  vi.stubGlobal('JSEvents',{getTargetThreadForEventCallback:(threadId:number)=>threadId===2?417:threadId})
  engine.observeInputRuntime()
  const register=(threadId=2,entries:ReadonlyArray<readonly [string,string,number]>=registrations,callback=101)=>{
    for(const [name,type,id] of entries) (globalThis as any)[name]('#canvas',0,1,callback,id,type,threadId)
  }
  if(registerRuntime) register()
  engine.channel='channel';engine.openOperationId='opened'
  engine.state={kind:'docx',documentId:'document',version:manualRevision,changeSequence:0,acknowledgedSequence:0,dirty:false}
  const result=(request:any,state=engine.state)=>({command:request.command,channel:request.channel,operationId:request.operationId,documentId:request.documentId,version:request.version,
    type:'result',ok:true,state,selection:{kind:'text',documentId:'document',version:state.version,changeSequence:state.changeSequence,text:'',scope:'body'}})
  const bound=(request:any)=>({...result(request),inputRuntime:{assetVersion:request.assetVersion,threadMatches:true}})
  const manual=(request:any,state={...engine.state,manual:{version:2,origin:'manual',changeId:request.manual.changeId,baseRevision:request.manual.baseRevision,generation:engine.lastManualGeneration+1,frozen:false}})=>
    ({...result(request,state),inputRuntime:{assetVersion,threadMatches:true,generation:state.manual.generation,changeId:state.manual.changeId}})
  const frozen=(request:any,extra:any={})=>{
    const {selection:_,...reply}=result(request,{...engine.state,manual:{...engine.state.manual,frozen:true}})
    return {...reply,freezeAck:{changeId:manualChange,operationId:request.operationId,generation:1,sequence:engine.state.changeSequence,...extra}}
  }
  const worker=vi.fn(async(request:any)=>{
    if(request.command==='bind-input-runtime') return bound(request)
    if(request.command==='manual-edit') return manual(request)
    if(request.command==='insert-text') return result(request,{...engine.state,changeSequence:engine.state.changeSequence+1,dirty:true})
    if(request.command==='freeze') return frozen(request)
    if(request.command==='export') return {...result(request),exportedSequence:engine.state.changeSequence}
    return result(request)
  })
  engine.worker=worker
  const request=(command:string,extra:any={})=>engine.request({command,channel:'channel',operationId:command+'_operation_01',documentId:'document',version:engine.state.version,...extra})
  const key=(key:string,extra:KeyboardEventInit={})=>{const event=new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true,...extra});canvas.dispatchEvent(event);return event}
  const composition=(type:string,data='')=>{const event=new CompositionEvent(type,{data,bubbles:true,cancelable:true});canvas.dispatchEvent(event);return event}
  const paste=(text:string)=>{const getData=vi.fn((kind:string)=>kind==='text/plain'?text:'<b>untrusted</b>');const event=new Event('paste',{bubbles:true,cancelable:true});Object.defineProperty(event,'clipboardData',{value:{getData}});canvas.dispatchEvent(event);return {event,getData}}
  return {engine,worker,onEvent,result,bound,manual,frozen,register,registrations,originalKey,originalMouse,request,key,composition,paste,canvas,validRequest,validPreviewView,windowListeners:release.listeners,admit:()=>request('manual-edit',{manual:manualAuthority}),close:()=>{release();vi.unstubAllGlobals();document.body.innerHTML=''}}
}

test('only the exact Core manual authority and matching worker acknowledgement open native body input',async()=>{
  const h=manualSurfaceFixture()
  try {
    expect(h.key('a').defaultPrevented).toBe(true)
    h.engine.editing=true
    expect(h.key('Backspace').defaultPrevented).toBe(true)
    let finish!:()=>void
    let reached!:()=>void
    const ready=new Promise<void>(resolve=>{reached=resolve})
    h.worker.mockImplementation(async(request:any)=>request.command==='bind-input-runtime'?h.bound(request):new Promise(resolve=>{
      finish=()=>resolve(h.manual(request));reached()
    }))
    const admission=h.admit()
    await ready
    expect(h.key('中').defaultPrevented).toBe(true)
    expect(h.canvas.contentEditable).not.toBe('true')
    finish();await admission
    expect(h.engine.nativeWritable).toBe(true);expect(h.canvas.contentEditable).toBe('true')
    for(const key of ['a','中','Backspace','Delete','Enter','ArrowLeft']) expect(h.key(key).defaultPrevented).toBe(false)
    for(const key of ['z','y','b','i','u','o','p','n']) expect(h.key(key,{ctrlKey:true}).defaultPrevented).toBe(true)
    for(const key of ['a','c','v']) expect(h.key(key,{metaKey:true}).defaultPrevented).toBe(false)
    expect(h.key('F2').defaultPrevented).toBe(true)
    expect(h.key('Delete',{shiftKey:true}).defaultPrevented).toBe(true)
    const view={zoom:100,zoomType:3,fit:false,page:null,pages:null,readOnly:false,modified:false}
    expect(h.validPreviewView(view,'docx',true)).toBe(false)
    expect(h.validPreviewView(view,'docx',true,h.engine.nativeWritable)).toBe(true)
    expect(h.validPreviewView(view,'xlsx',true,true)).toBe(false)
    h.engine.closeInput();h.engine.editing=true
    expect(h.key('a').defaultPrevented).toBe(true)
  } finally {h.close()}
})

test('malformed manual, format, freeze and private text envelopes never widen the finite host seam',async()=>{
  const h=manualSurfaceFixture(),base={channel:'channel',documentId:'document',version:manualRevision,operationId:'operation_01'}
  try {
    const manual={...base,command:'manual-edit',manual:manualAuthority}
    expect(h.validRequest(manual)).toBe(true)
    for(const bad of [{...manual,manual:{...manualAuthority,origin:'agent'}},{...manual,manual:{...manualAuthority,baseRevision:'c'.repeat(64)}},{...manual,manual:{...manualAuthority,extra:true}},{...manual,uno:'.uno:EditDoc'}]) expect(h.validRequest(bad)).toBe(false)
    const rangeLease={generation:1,sequence:0,token:'selection_token'}
    for(const format of [{kind:'font-family',value:'Noto Sans CJK SC'},{kind:'font-size',value:12.5},{kind:'bold',value:true},{kind:'italic',value:false},{kind:'underline',value:true}]) expect(h.validRequest({...base,command:'format',changeId:manualChange,rangeLease,format})).toBe(true)
    for(const format of [{kind:'font-size',value:12.25},{kind:'font-family',value:'x\n'},{kind:'bold',value:'true'},{kind:'dispatch',value:'.uno:Save'}]) expect(h.validRequest({...base,command:'format',changeId:manualChange,rangeLease,format})).toBe(false)
    expect(h.validRequest({...base,command:'freeze',changeId:manualChange})).toBe(true)
    expect(h.validRequest({...base,command:'freeze',changeId:manualChange,freezeAck:{}})).toBe(false)
    await h.admit()
    await expect(h.request('insert-text',{changeId:manualChange,text:'private'})).rejects.toThrow('unsupported-command')
    h.worker.mockClear()
    await h.request('format',{changeId:manualChange,rangeLease,format:{kind:'font-size',value:12.5}})
    expect(h.worker).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({command:'format',rangeLease,format:{kind:'font-size',value:12.5}}))
  } finally {h.close()}
})

test('composition is browser owned until one complete native text commit, including final beforeinput and cancellation',async()=>{
  const h=manualSurfaceFixture()
  try {
    await h.admit();h.worker.mockClear()
    expect(h.composition('compositionstart').defaultPrevented).toBe(false)
    const qt=vi.fn();h.canvas.addEventListener('keydown',qt)
    expect(h.key('Process',{isComposing:true,code:'KeyA'}).defaultPrevented).toBe(false)
    expect(qt).not.toHaveBeenCalled()
    h.composition('compositionupdate','合成中文')
    await expect(h.engine.localView('read')).rejects.toThrow('operation-in-progress')
    const preedit=new InputEvent('beforeinput',{inputType:'insertCompositionText',data:'合成中文',isComposing:true,bubbles:true,cancelable:true})
    h.canvas.dispatchEvent(preedit);expect(preedit.defaultPrevented).toBe(true)
    expect(h.worker).not.toHaveBeenCalled()
    h.composition('compositionend','合成中文')
    const final=new InputEvent('beforeinput',{inputType:'insertText',data:'合成中文',bubbles:true,cancelable:true})
    h.canvas.dispatchEvent(final);expect(final.defaultPrevented).toBe(true)
    await h.engine.inputTail
    h.composition('compositionend','合成中文');await h.engine.inputTail
    expect(h.worker).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({command:'insert-text',changeId:manualChange,text:'合成中文'}))
    expect(h.canvas.textContent).toBe('')
    h.composition('compositionstart');h.composition('compositionend','');await h.engine.inputTail
    expect(h.worker).toHaveBeenCalledOnce()
    h.composition('compositionstart');h.engine.closeInput();h.composition('compositionend','retired')
    await h.engine.inputTail;expect(h.worker).toHaveBeenCalledOnce()
  } finally {h.close()}
})

test('paste consumes only wellformed bounded UTF8 plain text and never mirrors clipboard DOM',async()=>{
  const h=manualSurfaceFixture()
  try {
    await h.admit();h.worker.mockClear();h.engine.onInputError=vi.fn()
    const qtPaste=vi.fn();h.canvas.addEventListener('keydown',qtPaste)
    expect(h.key('v',{ctrlKey:true}).defaultPrevented).toBe(false)
    expect(qtPaste).not.toHaveBeenCalled()
    const {event,getData}=h.paste('合成\nplain')
    expect(event.defaultPrevented).toBe(true);expect(getData).toHaveBeenCalledExactlyOnceWith('text/plain')
    const duplicate=new InputEvent('beforeinput',{inputType:'insertFromPaste',data:'合成\nplain',bubbles:true,cancelable:true})
    h.canvas.dispatchEvent(duplicate);await h.engine.inputTail
    expect(h.worker).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({command:'insert-text',text:'合成\nplain'}))
    for(const invalid of ['\0','\ud800','中'.repeat(21846),'\f','\u0001']) {h.paste(invalid);await h.engine.inputTail}
    expect(h.worker).toHaveBeenCalledOnce();expect(h.engine.onInputError).toHaveBeenCalledTimes(5)
    for(const type of ['cut','drop']) {const event=new Event(type,{bubbles:true,cancelable:true});h.canvas.dispatchEvent(event);expect(event.defaultPrevented).toBe(true)}
    h.canvas.textContent='unwanted browser fallback';h.canvas.dispatchEvent(new Event('input',{bubbles:true}));expect(h.canvas.textContent).toBe('')
  } finally {h.close()}
})

test('freeze preserves composition, drains bridge ACKs, and accepts the exact worker native callback proof',async()=>{
  const h=manualSurfaceFixture()
  try {
    await h.admit();h.worker.mockClear()
    h.composition('compositionstart')
    expect(h.key('s',{ctrlKey:true}).defaultPrevented).toBe(true)
    expect(h.onEvent).toHaveBeenCalledWith(expect.objectContaining({type:'save-requested'}))
    await expect(h.request('freeze',{changeId:manualChange})).rejects.toThrow('native-composition-pending')
    expect(h.engine.composition).not.toBeNull();expect(h.engine.inputGate).toBe(false)
    let finish!:()=>void
    h.worker.mockImplementationOnce(request=>new Promise(resolve=>{finish=()=>resolve(h.result(request,{...h.engine.state,changeSequence:1,dirty:true}))}))
    h.composition('compositionend','synthetic')
    await Promise.resolve()
    const freezing=h.request('freeze',{changeId:manualChange})
    expect(h.engine.inputGate).toBe(true);expect(h.key('a').defaultPrevented).toBe(true)
    finish();await expect(freezing).resolves.toMatchObject({freezeAck:{changeId:manualChange,operationId:'freeze_operation_01',generation:1,sequence:1}})
    expect(h.worker.mock.calls.map(([request])=>request.command)).toEqual(['insert-text','freeze'])
    expect(h.engine.inputGate).toBe(true);expect(h.engine.state.manual.frozen).toBe(true)
    await expect(h.request('export')).rejects.toThrow('native-input-unavailable')
  } finally {h.close()}
})

test('canvas mouse guards close native context menus and bridge gates before actual Qt mouse listeners',async()=>{
  const h=manualSurfaceFixture(),native=vi.fn()
  const mouse=(type:string,button=0,ctrlKey=false)=>{const event=new MouseEvent(type,{button,ctrlKey,bubbles:true,cancelable:true});h.canvas.dispatchEvent(event);return event}
  try {
    for(const type of ['mousedown','mouseup','contextmenu']) h.canvas.addEventListener(type,native)
    for(const type of ['pointerdown','pointerup','pointermove','mousedown','mouseup','mousemove','contextmenu']) expect(mouse(type,2).defaultPrevented).toBe(true)
    expect(native).not.toHaveBeenCalled()
    await h.admit()
    expect(mouse('mousedown').defaultPrevented).toBe(false);expect(native).toHaveBeenCalledOnce()
    expect(mouse('mousedown',0,true).defaultPrevented).toBe(true)
    for(const field of ['inputGate','busy','inputPending']) {
      h.engine[field]=field==='inputPending'?1:true
      for(const type of ['pointerdown','pointerup','pointermove','mousedown','mouseup','mousemove']) expect(mouse(type).defaultPrevented).toBe(true)
      h.engine[field]=field==='inputPending'?0:false
    }
    h.composition('compositionstart')
    expect(mouse('mousedown').defaultPrevented).toBe(true)
    expect(native).toHaveBeenCalledOnce()
  } finally {h.close()}
})

test.each(['committed','conflict','failed'])('only a matching committed ACK updates the current virtual baseline: %s',async status=>{
  const h=manualSurfaceFixture(),bytes=new Uint8Array([11,22,33]),writeFile=vi.fn()
  vi.stubGlobal('FS',{stat:()=>({size:bytes.length}),readFile:()=>bytes,unlink:vi.fn(),writeFile})
  try {
    h.engine.editing=true;h.engine.state={...h.engine.state,changeSequence:1,dirty:true}
    h.worker.mockImplementation(async(request:any)=>request.command==='export'?{...h.result(request),exportedSequence:1}:h.result(request,status==='committed'?{...h.engine.state,version:'c'.repeat(64),acknowledgedSequence:1,dirty:false}:h.engine.state))
    const exported=await h.request('export');exported.bytes.fill(99)
    await expect(h.request('ack',{exportOperationId:'different',exportedSequence:1,status:'failed'})).rejects.toThrow('ack-mismatch')
    await h.request('ack',{exportOperationId:'export_operation_01',exportedSequence:1,status,...(status==='committed'?{persistedVersion:'c'.repeat(64)}:{})})
    if(status==='committed') expect(writeFile).toHaveBeenCalledExactlyOnceWith('/tmp/analytix-office/input.docx',new Uint8Array([11,22,33]))
    else expect(writeFile).not.toHaveBeenCalled()
    expect(h.engine.exportArtifact).toBeNull()
  } finally {h.close();vi.unstubAllGlobals()}
})

test('an input baseline sync failure retires the sink and never returns a successful ACK',async()=>{
  const h=manualSurfaceFixture()
  vi.stubGlobal('FS',{stat:()=>({size:1}),readFile:()=>new Uint8Array([1]),unlink:vi.fn(),writeFile:()=>{throw Error('synthetic failure')}})
  try {
    h.engine.editing=true;h.engine.state={...h.engine.state,changeSequence:1,dirty:true}
    h.worker.mockImplementation(async(request:any)=>request.command==='export'?{...h.result(request),exportedSequence:1}:h.result(request,{...h.engine.state,version:'c'.repeat(64),acknowledgedSequence:1,dirty:false}))
    await h.request('export')
    await expect(h.request('ack',{exportOperationId:'export_operation_01',exportedSequence:1,status:'committed',persistedVersion:'c'.repeat(64)})).rejects.toThrow('surface-state-unknown-recreate-required')
    expect(h.engine.failed).toBe(true);expect(h.onEvent).toHaveBeenCalledWith(expect.objectContaining({type:'fatal'}))
  } finally {h.close();vi.unstubAllGlobals()}
})

test('a committed manual ACK closes body authority while later AI editing retains only the native model',async()=>{
  const h=manualSurfaceFixture(),nextRevision='c'.repeat(64),writeFile=vi.fn()
  vi.stubGlobal('FS',{writeFile})
  try {
    await h.admit()
    h.engine.state={...h.engine.state,changeSequence:1,dirty:true}
    h.engine.exportArtifact={operationId:'held_export_01',sequence:1,documentId:'document',version:manualRevision,kind:'docx',bytes:new Uint8Array([7])}
    h.worker.mockImplementation(async(request:any)=>{
      if(request.command==='ack') {const {manual:_,...state}=h.engine.state;return h.result(request,{...state,version:nextRevision,acknowledgedSequence:1,dirty:false})}
      return h.result(request)
    })
    await h.request('ack',{exportOperationId:'held_export_01',exportedSequence:1,status:'committed',persistedVersion:nextRevision})
    expect(writeFile).toHaveBeenCalledExactlyOnceWith('/tmp/analytix-office/input.docx',new Uint8Array([7]))
    expect(h.engine.nativeWritable).toBe(true);expect(h.engine.manualInput).toBeNull();expect(h.engine.editing).toBe(false)
    expect(h.canvas.contentEditable).toBe('false');expect(h.key('a').defaultPrevented).toBe(true)
    await h.request('edit')
    expect(h.engine.editing).toBe(true);expect(h.key('a').defaultPrevented).toBe(true)
    h.worker.mockImplementationOnce(async(request:any)=>h.manual(request))
    await h.request('manual-edit',{operationId:'manual_second_01',manual:{...manualAuthority,changeId:'d'.repeat(64),baseRevision:nextRevision,saveOperationId:'save_second_01',undoOperationId:'undo_second_01'}})
    expect(h.engine.lastManualGeneration).toBe(2);expect(h.engine.manualInput.generation).toBe(2)
    expect(h.worker.mock.calls.filter(([request])=>request.command==='bind-input-runtime')).toHaveLength(1)
  } finally {h.close();vi.unstubAllGlobals()}
})

test('a mismatched manual acknowledgement and a failed native insert retire input without admitting another mutation',async()=>{
  const mismatch=manualSurfaceFixture()
  try {
    mismatch.worker.mockImplementation(async(request:any)=>request.command==='bind-input-runtime'?mismatch.bound(request):mismatch.manual(request,{...mismatch.engine.state,manual:{version:2,origin:'manual',changeId:'c'.repeat(64),baseRevision:manualRevision,generation:1,frozen:false}}))
    await expect(mismatch.admit()).rejects.toThrow('surface-state-unknown-recreate-required')
    expect(mismatch.engine.nativeWritable).toBe(false);expect(mismatch.key('a').defaultPrevented).toBe(true)
  } finally {mismatch.close()}
  const failed=manualSurfaceFixture()
  try {
    await failed.admit();failed.worker.mockRejectedValueOnce(Error('typed-mutation-failed'))
    failed.paste('synthetic');await failed.engine.inputTail
    expect(failed.engine.failed).toBe(true);expect(failed.engine.manualInput).toBeNull();expect(failed.key('a').defaultPrevented).toBe(true)
    expect(failed.onEvent).toHaveBeenCalledWith(expect.objectContaining({type:'fatal'}))
  } finally {failed.close()}
})

test('runtime binding observes and forwards all five real registrations, with one capability handshake per engine generation',async()=>{
  const h=manualSurfaceFixture()
  try {
    expect(h.originalKey.mock.calls).toEqual([
      ['#canvas',0,1,101,2,'keydown',2],['#canvas',0,1,101,3,'keyup',2]
    ])
    expect(h.originalMouse.mock.calls).toEqual([
      ['#canvas',0,1,101,5,'mousedown',2],['#canvas',0,1,101,6,'mouseup',2],['#canvas',0,1,101,8,'mousemove',2]
    ])
    await h.admit()
    expect(h.worker.mock.calls[0][0]).toMatchObject({command:'bind-input-runtime',threadId:417,assetVersion:expect.stringContaining('efaf0670b4d055f838a2849becb10f08aa06a257:5143e535')})
    expect(h.worker.mock.calls[1][0]).toMatchObject({command:'manual-edit'})
    expect(JSON.stringify(h.onEvent.mock.calls)).not.toContain('threadId')
    const proof=await h.engine.bindInputRuntime()
    expect(proof.generation).toBe(5)
    expect(h.worker.mock.calls.filter(([request])=>request.command==='bind-input-runtime')).toHaveLength(1)
    await expect(h.request('bind-input-runtime',{threadId:417,assetVersion:h.worker.mock.calls[0][0].assetVersion})).rejects.toThrow('unsupported-command')
    h.register(417,[h.registrations[4]])
    expect(h.engine.failed).toBe(true);expect(h.engine.manualInput).toBeNull()
  } finally {h.close()}
})

test.each(['absent','missing-mouse','mismatched-key','mismatched-mouse'])('manual admission refuses incomplete or mismatched native registration: %s',async mode=>{
  const h=manualSurfaceFixture(false)
  try {
    if(mode==='missing-mouse') h.register(2,h.registrations.slice(0,2))
    if(mode==='mismatched-key') {h.register(2,[h.registrations[0]]);h.register(418,[h.registrations[1]])}
    if(mode==='mismatched-mouse') {h.register(2,h.registrations.slice(0,2));h.register(418,h.registrations.slice(2))}
    await expect(h.admit()).rejects.toThrow('native-input-unavailable')
    expect(h.worker).not.toHaveBeenCalled();expect(h.canvas.contentEditable).not.toBe('true')
  } finally {h.close()}
})

test('runtime handshake mismatch and late registration changes retire the input sink before manual admission',async()=>{
  const mismatch=manualSurfaceFixture()
  try {
    mismatch.worker.mockImplementationOnce(async(request:any)=>({...mismatch.bound(request),inputRuntime:{assetVersion:'wrong',threadMatches:true}}))
    await expect(mismatch.admit()).rejects.toThrow('native-input-unavailable')
    expect(mismatch.engine.failed).toBe(true);expect(mismatch.worker).toHaveBeenCalledOnce()
  } finally {mismatch.close()}
  const late=manualSurfaceFixture()
  try {
    let finish!:()=>void,reached!:()=>void
    const ready=new Promise<void>(resolve=>{reached=resolve})
    late.worker.mockImplementationOnce(request=>new Promise(resolve=>{finish=()=>resolve(late.bound(request));reached()}))
    const admission=late.admit();await ready
    late.register(417,[late.registrations[0]])
    finish();await expect(admission).rejects.toThrow('native-input-unavailable')
    expect(late.engine.failed).toBe(true);expect(late.worker).toHaveBeenCalledOnce()
  } finally {late.close()}
})

test.each(['unchanged','known-reload','missing-event','cross-thread','duplicate-handler','return-code','wrong-generation','timeout-late','close-late'])('readonly to manual loading handoff is correlated and blocks actual native producers: %s',async mode=>{
  const h=manualSurfaceFixture(),nativeKey=vi.fn(),nativePointer=vi.fn()
  let timer!:(()=>void),rejectNative!:(error:Error)=>void
  if(mode==='timeout-late') {
    vi.stubGlobal('setTimeout',vi.fn((callback:()=>void)=>{timer=callback;return 1}))
    vi.stubGlobal('clearTimeout',vi.fn())
  }
  try {
    await h.engine.bindInputRuntime()
    let finish!:()=>void,reached!:()=>void
    const ready=new Promise<void>(resolve=>{reached=resolve})
    h.worker.mockImplementationOnce(request=>new Promise((resolve,reject)=>{
      finish=()=>{
        const result=h.manual(request)
        if(mode==='wrong-generation') {result.state.manual.generation=99;result.inputRuntime.generation=99}
        resolve(result)
      };rejectNative=reject;reached()
    }))
    if(mode==='timeout-late') {
      // Exercise the real pending-request timeout, then deliver its late reply.
      h.engine.workerPort={postMessage:()=>reached()}
      h.engine.worker=h.engine.constructor.prototype.worker
    }
    h.canvas.addEventListener('keydown',nativeKey)
    for(const type of ['pointerdown','pointerup','pointermove','mousedown','mouseup','mousemove']) h.canvas.addEventListener(type,nativePointer)
    const admission=h.admit();await ready
    const rejected=admission.catch((error:Error)=>error)
    expect(h.engine.manualLoading).toMatchObject({operationId:'manual-edit_operation_01',documentId:'document',version:manualRevision,opened:'opened',expectedGeneration:1,threadId:417})
    for(const key of ['a','ArrowLeft','Control']) expect(h.key(key).defaultPrevented).toBe(true)
    for(const type of ['pointerdown','pointerup','pointermove','mousedown','mouseup','mousemove']) {
      const event=new MouseEvent(type,{button:0,bubbles:true,cancelable:true});h.canvas.dispatchEvent(event);expect(event.defaultPrevented).toBe(true)
    }
    expect(nativeKey).not.toHaveBeenCalled();expect(nativePointer).not.toHaveBeenCalled()
    if(mode==='known-reload') {
      h.register(2,h.registrations,0);h.register(2)
      h.register(2,[h.registrations[0]],0);h.register(2,[h.registrations[0]])
    } else if(mode==='missing-event') h.register(2,[h.registrations[0]],0)
    else if(mode==='cross-thread') {h.register(2,[h.registrations[0]],0);h.register(418,[h.registrations[0]])}
    else if(mode==='duplicate-handler') h.register(2,[h.registrations[0]])
    else if(mode==='return-code') {h.originalMouse.mockReturnValueOnce(-4);h.register(2,[h.registrations[2]],0)}
    else if(mode==='close-late') h.engine.closeInput()
    else if(mode==='timeout-late') {
      const pending=[...h.engine.pending.values()][0] as any
      const delayedReply=h.manual(pending.request)
      timer()
      expect(h.engine.pending.size).toBe(0)
      // The deleted pending owner cannot consume a later result or reopen input.
      pending.resolve(delayedReply)
      expect(h.engine.manualLoading).toBeNull()
      expect(delayedReply.inputRuntime.generation).toBe(1)
    }
    if(mode!=='timeout-late') finish()
    const outcome=await rejected
    if(['unchanged','known-reload'].includes(mode)) {
      expect(outcome.ok).toBe(true);expect(outcome.inputRuntime).toBeUndefined();expect(h.engine.failed).not.toBe(true)
      expect(h.engine.manualInput.generation).toBe(1);expect(h.engine.lastManualGeneration).toBe(1)
      expect(h.engine.inputRuntime.bound.generation).toBe(mode==='known-reload'?17:5)
      expect(h.engine.inputRuntime.qualified.generation).toBe(h.engine.inputRuntime.bound.generation)
      expect(h.canvas.contentEditable).toBe('true')
    } else {
      expect(outcome).toBeInstanceOf(Error);expect(h.engine.failed).toBe(true);expect(h.engine.lastManualGeneration).toBe(0)
      expect(h.engine.manualInput).toBeNull();expect(h.engine.inputRuntime.bound).toBeNull();expect(h.engine.inputRuntime.qualified).toBeNull()
      expect(h.canvas.contentEditable).toBe('false')
      expect(h.key('ArrowLeft').defaultPrevented).toBe(true)
      const late=new MouseEvent('mousedown',{button:0,bubbles:true,cancelable:true});h.canvas.dispatchEvent(late)
      expect(late.defaultPrevented).toBe(true);expect(nativeKey).not.toHaveBeenCalled();expect(nativePointer).not.toHaveBeenCalled()
    }
    expect(h.engine.manualLoading).toBeNull()
  } finally {rejectNative?.(Error('fixture closed'));h.close()}
})

test('freeze holds every producer for a delayed native callback and only replays its same exact operation',async()=>{
  const h=manualSurfaceFixture()
  try {
    await h.admit();h.worker.mockClear()
    let finish!:()=>void,reached!:()=>void
    const ready=new Promise<void>(resolve=>{reached=resolve})
    h.worker.mockImplementationOnce(request=>new Promise(resolve=>{finish=()=>resolve(h.frozen(request));reached()}))
    const freezing=h.request('freeze',{changeId:manualChange});await ready
    expect(h.engine.inputGate).toBe(true);expect(h.key('Delete').defaultPrevented).toBe(true)
    const move=new MouseEvent('mousemove',{button:0,bubbles:true,cancelable:true});h.canvas.dispatchEvent(move);expect(move.defaultPrevented).toBe(true)
    await expect(h.request('freeze',{changeId:manualChange})).rejects.toThrow('operation-in-progress')
    finish();const frozen=await freezing
    await expect(h.request('freeze',{changeId:manualChange})).resolves.toMatchObject({freezeAck:frozen.freezeAck})
    await expect(h.request('freeze',{changeId:manualChange,operationId:'different_freeze_01'})).rejects.toThrow('operation-in-progress')
    expect(h.worker).toHaveBeenCalledOnce();expect(h.engine.inputGate).toBe(true)
    expect(JSON.stringify(frozen)).not.toContain('threadId')
  } finally {h.close()}
})

test.each(['changeId','operationId','generation','sequence'])('freeze rejects a native callback with a mismatched %s',async field=>{
  const h=manualSurfaceFixture()
  try {
    await h.admit()
    h.worker.mockImplementationOnce(async(request:any)=>h.frozen(request,{[field]:['generation','sequence'].includes(field)?99:'d'.repeat(64)}))
    await expect(h.request('freeze',{changeId:manualChange})).rejects.toThrow('surface-state-unknown-recreate-required')
    expect(h.engine.failed).toBe(true);expect(h.engine.freezeProof).toBeNull();expect(h.engine.inputGate).toBe(true)
  } finally {h.close()}
})

test('manual export accepts only the held native freeze proof and retains its exact bytes for committed ACK',async()=>{
  const h=manualSurfaceFixture(),bytes=new Uint8Array([4,5,6]),writeFile=vi.fn()
  vi.stubGlobal('FS',{stat:()=>({size:3}),readFile:()=>bytes,unlink:vi.fn(),writeFile})
  try {
    await h.admit();h.paste('synthetic');await h.engine.inputTail
    await expect(h.request('export')).rejects.toThrow('native-input-unavailable')
    const frozen=await h.request('freeze',{changeId:manualChange})
    const count=h.worker.mock.calls.length
    await expect(h.request('export',{changeId:manualChange,freezeAck:{...frozen.freezeAck,sequence:2}})).rejects.toThrow('native-input-unavailable')
    expect(h.worker.mock.calls).toHaveLength(count)
    const exported=await h.request('export',{changeId:manualChange,freezeAck:frozen.freezeAck})
    expect(exported.bytes).toEqual(bytes);expect(h.engine.inputGate).toBe(true)
    const {manual:_,...state}=h.engine.state
    h.worker.mockImplementationOnce(async(request:any)=>h.result(request,{...state,version:'c'.repeat(64),acknowledgedSequence:1,dirty:false}))
    await h.request('ack',{exportOperationId:'export_operation_01',exportedSequence:1,status:'committed',persistedVersion:'c'.repeat(64)})
    expect(writeFile).toHaveBeenCalledExactlyOnceWith('/tmp/analytix-office/input.docx',bytes)
    expect(h.engine.manualInput).toBeNull();expect(h.engine.freezeProof).toBeNull();expect(h.engine.nativeWritable).toBe(true)
  } finally {h.close()}
})

test('presentation keyboard input consumes finite controls with the actual generation while readonly input stays blocked',async()=>{
  vi.useFakeTimers();const h=surfaceFixture()
  const press=(key:string,extra:KeyboardEventInit={})=>{
    const event=new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true,...extra});h.canvas.dispatchEvent(event);return event
  }
  try {
    expect(press(' ').defaultPrevented).toBe(true)
    await vi.advanceTimersByTimeAsync(0)
    expect(h.worker).toHaveBeenLastCalledWith(expect.objectContaining({action:'play-next-effect',playbackGeneration:1}))
    expect(h.engine.view).toMatchObject({page:5,pages:5,playback:{slideIndex:-1,slideCount:2}})
    press('PageDown');await vi.advanceTimersByTimeAsync(0)
    expect(h.worker).toHaveBeenLastCalledWith(expect.objectContaining({action:'play-next-slide',playbackGeneration:1}))
    const calls=h.worker.mock.calls.length
    expect(press('F5').defaultPrevented).toBe(true)
    expect(press('a').defaultPrevented).toBe(true)
    expect(press('s',{ctrlKey:true}).defaultPrevented).toBe(true)
    const input=new InputEvent('beforeinput',{bubbles:true,cancelable:true,inputType:'insertText',data:'x'})
    h.canvas.dispatchEvent(input);expect(input.defaultPrevented).toBe(true)
    expect(h.worker).toHaveBeenCalledTimes(calls)
    press('Escape');await vi.advanceTimersByTimeAsync(0)
    expect(h.worker).toHaveBeenLastCalledWith(expect.objectContaining({action:'play-end',playbackGeneration:1}))
    expect(h.engine.view.playback).toMatchObject({generation:2,phase:'idle'})
  } finally {h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('surface validation rejects invented playback state and nonfinite or unbound playback requests',()=>{
  const h=surfaceFixture()
  try {
    const request={command:'local-view',channel:'channel',operationId:'operation',documentId:'document',version:'version',action:'play-start',playbackGeneration:0}
    expect(h.validRequest(request)).toBe(true)
    for(const extra of [{playbackGeneration:undefined},{playbackGeneration:-1},{playbackGeneration:NaN},{zoom:100},{uno:'Presentation'}]) expect(h.validRequest({...request,...extra})).toBe(false)
    expect(h.validRequest({...request,action:'play-read'})).toBe(false)
    const {playbackGeneration:_,...read}=request
    expect(h.validRequest({...read,action:'play-read'})).toBe(true)
    const state={generation:0,sequence:0,phase:'running',slideIndex:-1,slideCount:2}
    expect(h.validPreviewView({...editorView,playback:state},'pptx',false)).toBe(true)
    for(const extra of [{slideIndex:-2},{slideIndex:2},{slideCount:0},{phase:'starting'},{phase:'idle'},{sequence:-1},{generation:NaN},{extra:'uno'}]) {
      expect(h.validPreviewView({...editorView,playback:{...state,...extra}},'pptx',false)).toBe(false)
    }
    expect(h.validPreviewView({...editorView,page:null,pages:null,playback:state},'docx',false)).toBe(false)
    expect(h.validPreviewView({...editorView,playback:undefined},'pptx',false)).toBe(false)
    expect(h.validPreviewView({...editorView,page:null,pages:null,playback:undefined},'docx',false)).toBe(false)
    expect(h.validPreviewView({...editorView,playback:{...state,phase:'error',slideIndex:null,slideCount:null}},'pptx',false)).toBe(true)
  } finally {h.close()}
})

test('late generation, old sample and old document events cannot replace the current show',()=>{
  const h=surfaceFixture()
  try {
    h.event({...editorView,playback:{generation:3,sequence:10,phase:'running',slideIndex:1,slideCount:2}})
    const retained=h.engine.view
    h.event({...editorView,playback:{generation:2,sequence:99,phase:'idle',slideIndex:null,slideCount:null}})
    h.event({...editorView,playback:{generation:3,sequence:9,phase:'paused',slideIndex:-1,slideCount:2}})
    h.event({...editorView,playback:{generation:4,sequence:20,phase:'idle',slideIndex:null,slideCount:null}},{documentId:'retired'})
    expect(h.engine.view).toBe(retained)
    expect(h.onEvent).toHaveBeenCalledOnce()
    h.event({...editorView,playback:{generation:3,sequence:11,phase:'paused',slideIndex:-1,slideCount:2}})
    expect(h.engine.view.playback).toMatchObject({phase:'paused',slideIndex:-1})
  } finally {h.close()}
})

test('a failed start retains the emitted actual failure view and rejected duplicate controls retain a running owner',async()=>{
  const h=surfaceFixture()
  try {
    const retained=h.engine.view
    h.worker.mockRejectedValueOnce(Error('operation-in-progress'))
    await expect(h.engine.localView('play-start',undefined,1)).rejects.toThrow('operation-in-progress')
    expect(h.engine.view).toBe(retained)
    h.worker.mockImplementationOnce(async()=>{
      h.event({...editorView,playback:{generation:2,sequence:3,phase:'unsupported',slideIndex:null,slideCount:null}})
      throw Error('presentation-unsupported')
    })
    await expect(h.engine.localView('play-start',undefined,1)).rejects.toThrow('presentation-unsupported')
    expect(h.engine.view).toMatchObject({zoom:175,page:5,pages:5,playback:{phase:'unsupported'}})
    expect(h.onEvent).not.toHaveBeenCalledWith(expect.objectContaining({view:null}))
  } finally {h.close()}
})

test('the installed bridge stops editor fit and delayed read scheduling during actual playback',async()=>{
  document.body.innerHTML=new DOMParser().parseFromString(html,'text/html').body.innerHTML
  vi.useFakeTimers();const release=trackWindowListeners()
  try {
    let receive!:(request:any)=>Promise<void>
    vi.stubGlobal('analytixOfficeSurface',{connect:(handler:typeof receive)=>{receive=handler},send:vi.fn()})
    const Engine=new Function(source.replaceAll('export ','')+'\nreturn OfficeEngineSurface;')()
    Engine.prototype.start=async function(channel:string){this.channel=channel;this.state={kind:'pptx',documentId:'document',version:'version',dirty:false};this.openOperationId='opened'}
    const worker=vi.fn(async()=>({view:{...editorView,playback:{generation:1,sequence:1,phase:'running',slideIndex:-1,slideCount:2}}}))
    Engine.prototype.worker=worker
    await receive({type:'bind',channel:'channel'})
    await receive({command:'local-view',action:'play-read',channel:'channel',operationId:'playback',documentId:'document',version:'version'})
    const calls=worker.mock.calls.length
    document.getElementById('qtcanvas')!.dispatchEvent(new Event('wheel'))
    window.dispatchEvent(new Event('resize'))
    await vi.advanceTimersByTimeAsync(500)
    expect(worker).toHaveBeenCalledTimes(calls)
    expect(document.getElementById('status')?.textContent).toBe('放映中 · — / 2')
  } finally {release();vi.clearAllTimers();vi.useRealTimers();vi.unstubAllGlobals();document.body.innerHTML=''}
})

test('canvas F5 startup timeout retires the surface and late playback samples cannot revive it',async()=>{
  vi.useFakeTimers();const h=surfaceFixture()
  try {
    h.engine.view={...editorView,playback:{generation:0,sequence:1,phase:'idle',slideIndex:null,slideCount:null}}
    h.worker.mockImplementationOnce(async()=>{
      h.event({...editorView,playback:{generation:1,sequence:2,phase:'error',slideIndex:null,slideCount:null}})
      throw Error('presentation-start-timeout')
    })
    const key=new KeyboardEvent('keydown',{key:'F5',bubbles:true,cancelable:true})
    h.canvas.dispatchEvent(key);await vi.advanceTimersByTimeAsync(0)
    expect(key.defaultPrevented).toBe(true)
    expect(h.worker).toHaveBeenCalledWith(expect.objectContaining({action:'play-start',playbackGeneration:0}))
    expect(h.onEvent).toHaveBeenCalledWith({type:'fatal',channel:'channel',error:'engine-operation-failed'})
    expect(h.engine.failed).toBe(true)
    const retained=h.engine.view,events=h.onEvent.mock.calls.length
    h.event({...editorView,playback:{generation:1,sequence:3,phase:'running',slideIndex:0,slideCount:2}})
    expect(h.engine.view).toBe(retained);expect(h.onEvent).toHaveBeenCalledTimes(events)
    await expect(h.engine.localView('play-read')).rejects.toThrow('operation-in-progress')
  } finally {h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('Main-owned startup timeout keeps its dedicated error for the host destruction path',async()=>{
  const h=surfaceFixture()
  try {
    h.engine.view={...editorView,playback:{generation:0,sequence:1,phase:'idle',slideIndex:null,slideCount:null}}
    h.worker.mockImplementationOnce(async()=>{
      h.event({...editorView,playback:{generation:1,sequence:2,phase:'error',slideIndex:null,slideCount:null}})
      throw Error('presentation-start-timeout')
    })
    await expect(h.engine.request({command:'local-view',channel:'channel',operationId:'host-start',documentId:'document',version:'version',action:'play-start',playbackGeneration:0})).rejects.toThrow('presentation-start-timeout')
    expect(h.onEvent).not.toHaveBeenCalledWith(expect.objectContaining({type:'fatal'}))
    expect(h.engine.view.playback.phase).toBe('error')
  } finally {h.close()}
})

function pointerFixture(kind='docx') {
  document.body.innerHTML='<main id="pointerRoot"><canvas id="pointerCanvas" tabindex="0"></canvas></main>'
  const release=trackWindowListeners()
  const Toolbar=new Function(source.replaceAll('export ','')+'\nreturn OfficePointerToolbar;')()
  const canvas=document.getElementById('pointerCanvas')!,root=document.getElementById('pointerRoot')!,send=vi.fn()
  const engine:any={channel:'channel',openOperationId:'opened',state:{kind,documentId:'document',version:'version',changeSequence:0},failed:false}
  const toolbar=new Toolbar(canvas,root,send,()=>engine)
  const selected={kind:'text',documentId:'document',version:'version',changeSequence:0,token:'selection_token',text:'same text'}
  // Pure listener fixtures; these are not Chrome/Qt native hit evidence.
  const pointer={isTrusted:true,isPrimary:true,button:0,pointerId:1,target:canvas,clientX:120,clientY:90}
  const confirm=(changed=true)=>{toolbar.selection({selection:selected});toolbar.confirm({selectionToken:selected.token,eligible:true,changed})}
  const start=()=>toolbar.pointerDown(pointer),end=()=>toolbar.pointerUp(pointer)
  return {toolbar,canvas,root,engine,send,selected,pointer,confirm,start,end,close:()=>{release();document.body.innerHTML=''}}
}
test('nearby DOCX actions require native nonempty endpoint change in the current gesture, never pointerup sampling',()=>{
  const h=pointerFixture()
  try {
    h.start();h.end();expect(h.send).not.toHaveBeenCalled()
    h.confirm(false);expect(h.send).not.toHaveBeenCalled()
    h.toolbar.cancel();h.start();h.confirm();h.end()
    const lease=h.send.mock.calls.at(-1)![0]
    expect(lease).toMatchObject({type:'pointer-actions',phase:'request',selectionToken:h.selected.token,expectedChangeSequence:0})
    expect(lease).not.toHaveProperty('x');expect(lease).not.toHaveProperty('y')
    expect(h.toolbar.bar.hidden).toBe(true)
    h.toolbar.receive({...lease,phase:'present',selectionToken:'old_token',actions:[{id:'quote',label:'Quote',enabled:true}]})
    expect(h.toolbar.bar.hidden).toBe(true)
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'<Quote>',enabled:true},{id:'edit',label:'Edit',enabled:false}]})
    expect(h.toolbar.bar.hidden).toBe(false);expect(h.toolbar.bar.querySelector('quote')).toBeNull()
    expect(h.toolbar.bar.querySelectorAll('button')[1].disabled).toBe(true)
    h.toolbar.bar.querySelector('button').click();expect(h.send.mock.calls.at(-1)![0].phase).toBe('request')
    h.toolbar.viewport({zoom:100});h.toolbar.viewport({zoom:100});expect(h.toolbar.bar.hidden).toBe(false)
    h.toolbar.viewport({zoom:125});expect(h.toolbar.bar.hidden).toBe(true)
    expect(h.send.mock.calls.at(-1)![0].phase).toBe('cancel')
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Late',enabled:true}]});expect(h.toolbar.bar.hidden).toBe(true)
  } finally {h.close()}
})
test.each(['wheel','scroll','resize','blur','pagehide','Escape','pointercancel','document-switch'])('nearby selection lease is retired by %s and ignores late results',reason=>{
  const h=pointerFixture()
  try {
    h.start();h.confirm();h.end();const lease=h.send.mock.calls.at(-1)![0]
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}]})
    if(reason==='Escape')window.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'}))
    else if(reason==='pointercancel')h.canvas.dispatchEvent(new Event('pointercancel'))
    else if(reason==='document-switch'){h.engine.state.documentId='other';h.toolbar.cancel()}
    else window.dispatchEvent(new Event(reason))
    expect(h.toolbar.bar.hidden).toBe(true);expect(h.send.mock.calls.at(-1)![0].phase).toBe('cancel')
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Late',enabled:true}]});expect(h.toolbar.bar.hidden).toBe(true)
  } finally {h.close()}
})
test.each(['docx','xlsx','pptx'])('synthetic pointer events, unavailable ranges and %s do not fabricate nearby authority',kind=>{
  const h=pointerFixture(kind)
  try {
    h.toolbar.pointerDown({...h.pointer,isTrusted:false});h.confirm();h.end();expect(h.send).not.toHaveBeenCalled()
    if(kind!=='docx'){h.start();h.confirm();h.end();expect(h.send).not.toHaveBeenCalled();return}
    h.start();h.toolbar.selection({selection:{...h.selected,text:''}});h.toolbar.confirm({selectionToken:h.selected.token,eligible:false,changed:true});h.end()
    expect(h.send).not.toHaveBeenCalled()
    // A drag can first collapse to a caret, then confirm one nonempty native range.
    h.start();h.toolbar.selection({selection:{...h.selected,text:''}});h.confirm();h.end();expect(h.send).toHaveBeenCalledOnce()
    h.toolbar.cancel();h.start();h.engine.state.version='new-version';h.confirm();h.end();expect(h.send.mock.calls.filter(([m])=>m.phase==='request')).toHaveLength(1)
  } finally {h.close()}
})

function showPointerActions(h:ReturnType<typeof pointerFixture>, actions=[{id:'quote',label:'Quote',enabled:true},{id:'disabled',label:'Disabled',enabled:false},{id:'edit',label:'Edit',enabled:true}]) {
  h.start();h.confirm();h.end()
  const lease=h.send.mock.calls.at(-1)![0]
  h.toolbar.receive({...lease,phase:'present',actions})
  return lease
}
function pointerKey(target:EventTarget,key:string,extra:Record<string,unknown>={}) {
  return {target,key,isTrusted:true,isComposing:false,ctrlKey:false,metaKey:false,altKey:false,preventDefault:vi.fn(),stopImmediatePropagation:vi.fn(),...extra}
}
test('only trusted outside pointers retire the nearby lease without intercepting Qt input or changing another editor focus',()=>{
  const h=pointerFixture()
  try {
    showPointerActions(h)
    const input=document.createElement('input');document.body.appendChild(input);input.focus()
    const outside={isTrusted:false,target:input,preventDefault:vi.fn(),stopImmediatePropagation:vi.fn()}
    h.toolbar.outsidePointer(outside);expect(h.toolbar.bar.hidden).toBe(false)
    h.toolbar.outsidePointer({...outside,isTrusted:true,target:h.toolbar.bar.firstChild});expect(h.toolbar.bar.hidden).toBe(false)
    h.toolbar.outsidePointer({...outside,isTrusted:true,target:h.canvas});expect(h.toolbar.bar.hidden).toBe(false)
    h.toolbar.outsidePointer({...outside,isTrusted:true})
    expect(h.toolbar.bar.hidden).toBe(true);expect(document.activeElement).toBe(input)
    expect(outside.preventDefault).not.toHaveBeenCalled();expect(outside.stopImmediatePropagation).not.toHaveBeenCalled()
    const qt=vi.fn();h.canvas.addEventListener('pointerdown',qt)
    const nativeInput=new Event('pointerdown',{bubbles:true,cancelable:true});h.canvas.dispatchEvent(nativeInput)
    expect(qt).toHaveBeenCalledOnce();expect(nativeInput.defaultPrevented).toBe(false)
  } finally {h.close()}
})
test('toolbar wheel and scroll remain usable while external scroll still retires the lease',()=>{
  const h=pointerFixture()
  try {
    const lease=showPointerActions(h)
    h.toolbar.bar.dispatchEvent(new Event('scroll'))
    h.toolbar.bar.firstChild.dispatchEvent(new Event('wheel',{bubbles:true}))
    expect(h.toolbar.bar.hidden).toBe(false);expect(h.send.mock.calls.at(-1)![0]).toEqual(lease)
    h.root.dispatchEvent(new Event('scroll'))
    expect(h.toolbar.bar.hidden).toBe(true);expect(h.send.mock.calls.at(-1)![0].phase).toBe('cancel')
  } finally {h.close()}
})
test('showing never steals focus, browser Tab may enter, and owned arrows/Home/End skip disabled actions and reveal focus',()=>{
  const h=pointerFixture()
  try {
    h.canvas.focus();showPointerActions(h)
    expect(document.activeElement).toBe(h.canvas)
    const tab=pointerKey(h.canvas,'Tab');h.toolbar.keyDown(tab)
    expect(tab.preventDefault).not.toHaveBeenCalled();expect(h.toolbar.bar.hidden).toBe(false)
    const buttons=[...(h.toolbar.bar as HTMLDivElement).querySelectorAll<HTMLButtonElement>('button')]
    // Simulate the browser's normal Tab focus transfer, not a product autofocus.
    buttons[0].focus();expect(document.activeElement).toBe(buttons[0])
    h.toolbar.bar.getBoundingClientRect=()=>({top:0,bottom:30,left:0,right:120})
    buttons[2].getBoundingClientRect=()=>({top:40,bottom:68,left:0,right:100} as DOMRect)
    const right=pointerKey(buttons[0],'ArrowRight');h.toolbar.keyDown(right)
    expect(document.activeElement).toBe(buttons[2]);expect(h.toolbar.bar.scrollTop).toBeGreaterThan(0)
    expect(right.preventDefault).toHaveBeenCalledOnce();expect(right.stopImmediatePropagation).toHaveBeenCalledOnce()
    h.toolbar.keyDown(pointerKey(buttons[2],'Home'));expect(document.activeElement).toBe(buttons[0])
    h.toolbar.keyDown(pointerKey(buttons[0],'End'));expect(document.activeElement).toBe(buttons[2])
    h.toolbar.keyDown(pointerKey(buttons[2],'ArrowDown'));expect(document.activeElement).toBe(buttons[0])
    h.toolbar.keyDown(pointerKey(buttons[0],'ArrowLeft'));expect(document.activeElement).toBe(buttons[2])
    h.toolbar.keyDown(pointerKey(buttons[2],'ArrowUp'));expect(document.activeElement).toBe(buttons[0])
    const insideTab=pointerKey(buttons[0],'Tab');h.toolbar.keyDown(insideTab)
    expect(insideTab.preventDefault).not.toHaveBeenCalled();expect(h.toolbar.bar.hidden).toBe(false)
  } finally {h.close()}
})
test('Escape returns only an owned focused toolbar to its still-current canvas and never steals another input focus',()=>{
  const h=pointerFixture(), focused=vi.spyOn(document,'hasFocus').mockReturnValue(true)
  try {
    h.canvas.focus();showPointerActions(h)
    const first=h.toolbar.bar.querySelector('button') as HTMLButtonElement;first.focus()
    const focus=vi.spyOn(h.canvas,'focus'),escape=pointerKey(first,'Escape')
    h.toolbar.keyDown(escape)
    expect(h.toolbar.bar.hidden).toBe(true);expect(document.activeElement).toBe(h.canvas)
    expect(focus).toHaveBeenCalledWith({preventScroll:true})
    showPointerActions(h)
    const input=document.createElement('input');document.body.appendChild(input);input.focus();focus.mockClear()
    const outside=pointerKey(input,'Escape');h.toolbar.keyDown(outside)
    expect(document.activeElement).toBe(input);expect(focus).not.toHaveBeenCalled();expect(outside.preventDefault).not.toHaveBeenCalled()
    showPointerActions(h);const last=h.toolbar.bar.querySelector('button') as HTMLButtonElement;last.focus();focus.mockClear()
    h.engine.state.version='retired';h.toolbar.keyDown(pointerKey(last,'Escape'))
    expect(focus).not.toHaveBeenCalled()
  } finally {focused.mockRestore();h.close()}
})
test('leaving toolbar focus retires it without pulling focus back, and untrusted or composing navigation cannot move it',()=>{
  const h=pointerFixture()
  try {
    showPointerActions(h);const first=h.toolbar.bar.querySelector('button') as HTMLButtonElement;first.focus()
    for(const extra of [{isTrusted:false},{isComposing:true},{ctrlKey:true},{altKey:true}]) {
      const event=pointerKey(first,'End',extra);h.toolbar.keyDown(event)
      expect(document.activeElement).toBe(first);expect(event.preventDefault).not.toHaveBeenCalled()
    }
    const input=document.createElement('textarea');document.body.appendChild(input);input.focus()
    expect(h.toolbar.bar.hidden).toBe(true);expect(document.activeElement).toBe(input)
    const event=pointerKey(input,'ArrowLeft');h.toolbar.keyDown(event)
    expect(document.activeElement).toBe(input);expect(event.preventDefault).not.toHaveBeenCalled()
  } finally {h.close()}
})
test('a narrow bounded toolbar keeps the full long action label and can reveal its focused button with internal scrolling',()=>{
  const h=pointerFixture()
  try {
    h.root.getBoundingClientRect=()=>({left:0,top:0,width:160,height:80} as DOMRect)
    Object.defineProperty(h.toolbar.bar,'offsetWidth',{value:144});Object.defineProperty(h.toolbar.bar,'offsetHeight',{value:64})
    const label='A'.repeat(120)
    showPointerActions(h,[{id:'long',label,enabled:true}])
    expect(h.toolbar.bar.style.left).toBe('8px');expect(h.toolbar.bar.style.top).toBe('8px')
    const button=h.toolbar.bar.querySelector('button') as HTMLButtonElement
    expect(button.textContent).toBe(label)
    h.toolbar.bar.getBoundingClientRect=()=>({top:8,bottom:72,left:8,right:152})
    button.getBoundingClientRect=()=>({top:70,bottom:98,left:8,right:152} as DOMRect)
    button.focus();expect(h.toolbar.bar.scrollTop).toBe(26)
    h.toolbar.bar.dispatchEvent(new Event('scroll'));expect(h.toolbar.bar.hidden).toBe(false)
  } finally {h.close()}
})

const pointerFormatLabels={fontFamily:'Font',fontSize:'Size',bold:'Bold',italic:'Italic',underline:'Underline',mixed:'Mixed'}
const pointerFormatState={fontFamily:'Noto Sans CJK SC',fontSize:12,bold:null,italic:true,underline:false}
function captureDomListeners() {
  const listeners:Array<{target:EventTarget;type:string;listener:EventListenerOrEventListenerObject}>=[]
  const add=EventTarget.prototype.addEventListener
  const spy=vi.spyOn(EventTarget.prototype,'addEventListener').mockImplementation(function(this:EventTarget,type,listener,options){
    if(listener) listeners.push({target:this,type,listener})
    add.call(this,type,listener,options)
  })
  const invoke=(entry:typeof listeners[number],event:unknown)=>typeof entry.listener==='function' ? entry.listener.call(entry.target,event as Event) : entry.listener.handleEvent(event as Event)
  return {listeners,invoke,restore:()=>spy.mockRestore(),fire:(target:EventTarget,type:string,extra:Record<string,unknown>={})=>{
    const entries=listeners.filter(entry=>entry.target===target && entry.type===type)
    expect(entries).toHaveLength(1)
    // Invoke the registered production listener with a declared trusted fixture;
    // dispatchEvent/click below still retain jsdom's actual untrusted semantics.
    const event={target,type,isTrusted:true,isComposing:false,preventDefault:vi.fn(),stopImmediatePropagation:vi.fn(),...extra}
    invoke(entries[0],event);return event
  }}
}
function formatPointerFixture() {
  const captured=captureDomListeners(),h=pointerFixture()
  h.engine.state={...h.engine.state,version:manualRevision,manual:{version:2,origin:'manual',changeId:manualChange,baseRevision:manualRevision,generation:1,frozen:false}}
  Object.assign(h.engine,{nativeWritable:true,editing:true,inputGate:false,inputPending:0,busy:false,composition:null,fontFamilies:['Noto Sans CJK SC','Synthetic Engine Serif'],manualInput:{changeId:manualChange,generation:1},manualAvailable:vi.fn(()=>true)})
  Object.assign(h.selected,{version:manualRevision,rangeLease:{generation:1,sequence:0,token:h.selected.token},formatState:{...pointerFormatState}})
  const formatting=(selected=h.toolbar.selected)=>({changeId:manualChange,rangeLease:{...selected.rangeLease},formatState:{...selected.formatState},labels:{...pointerFormatLabels}})
  const present=(extra:Record<string,unknown>={})=>{
    h.canvas.focus();h.start();h.confirm();h.end()
    const lease=h.send.mock.calls.at(-1)![0]
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],formatting:formatting(),...extra});return lease
  }
  const control=(kind:string)=>h.toolbar.bar.querySelector(`[data-format-control="${kind}"]`) as HTMLInputElement|HTMLSelectElement|HTMLButtonElement
  const request=(message=h.send.mock.calls.at(-1)![0],operationId='format_operation_01')=>({command:'format',channel:message.channel,operationId,documentId:message.documentId,version:message.version,changeId:message.changeId,rangeLease:{...message.rangeLease},format:{...message.format}})
  const result=(r:ReturnType<typeof request>,sequence=1,token='selection_next_01')=>{
    const state={...h.engine.state,changeSequence:sequence,dirty:true,manual:{...h.engine.state.manual}}
    const rangeLease={generation:state.manual.generation,sequence,token}
    const field=({'font-family':'fontFamily','font-size':'fontSize',bold:'bold',italic:'italic',underline:'underline'} as const)[r.format.kind as 'font-family'|'font-size'|'bold'|'italic'|'underline']
    const formatState={...h.toolbar.continuation.formatting.formatState,[field]:r.format.value}
    return {type:'result',command:r.command,channel:r.channel,operationId:r.operationId,documentId:r.documentId,version:r.version,ok:true,state,rangeLease,formatState,
      selection:{kind:'text',documentId:state.documentId,version:state.version,changeSequence:sequence,token,text:'same text',scope:'body',rangeLease:{...rangeLease},formatState:{...formatState}}}
  }
  return {...h,captured,formatting,present,control,request,result,close:()=>{h.toolbar.cancel();h.close();captured.restore()}}
}

test.each(['confirmed','pending','visible','format-choice'] as const)('duplicate native range samples preserve the %s format offer without re-presenting',phase=>{
  const h=formatPointerFixture()
  try {
    if(phase==='confirmed'){h.start();h.confirm()}else if(phase==='pending'){h.start();h.confirm();h.end()}else h.present()
    const lease=h.toolbar.lease,selected=structuredClone(h.toolbar.selected),size=h.control('font-size')
    if(phase==='visible'){size.focus();size.value='13.5'}
    if(phase==='format-choice')h.captured.fire(h.control('bold'),'click')
    const continuation=h.toolbar.continuation,calls=h.send.mock.calls.length
    // Native validation can emit duplicate notifications while input is gated.
    h.engine.busy=true;h.engine.inputGate=true
    h.toolbar.selection({selection:structuredClone(selected)})
    h.toolbar.confirm({selectionToken:selected.token,eligible:true,changed:false})
    expect(h.send.mock.calls).toHaveLength(calls)
    expect(h.toolbar.continuation).toBe(continuation)
    if(phase!=='format-choice')expect(h.toolbar.lease).toBe(lease)
    if(phase==='visible'){
      expect(h.toolbar.bar.hidden).toBe(false);expect(h.control('font-size')).toBe(size)
      expect(document.activeElement).toBe(size);expect(size.value).toBe('13.5')
    }
    h.engine.busy=false;h.engine.inputGate=false
    if(phase==='confirmed'){
      h.end();expect(h.send.mock.calls.filter(([message])=>message.phase==='request')).toHaveLength(1)
    }else if(phase==='pending'){
      h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],formatting:h.formatting()})
      expect(h.toolbar.bar.hidden).toBe(false)
    }else if(phase==='format-choice')expect(h.toolbar.beginFormat(h.request())).toBe(true)
  } finally {h.close()}
})

test.each(['range','empty','format','version','sequence','ack','generation','manual-change','opened'] as const)('a %s change retires the current format offer and ignores its old controls',reason=>{
  const h=formatPointerFixture()
  try {
    const lease=h.present(),formatting=h.formatting(),button=h.control('bold'),selected=structuredClone(h.toolbar.selected)
    if(reason==='range'){selected.token='different_range_token';selected.rangeLease.token=selected.token}
    else if(reason==='empty')selected.text=''
    else if(reason==='format')selected.formatState.bold=true
    else if(reason==='version')h.engine.state.version='changed-version'
    else if(reason==='sequence')h.engine.state.changeSequence++
    else if(reason==='ack'){h.engine.state.manual=undefined;h.engine.manualInput=null;h.engine.manualAvailable.mockReturnValue(false)}
    else if(reason==='generation')h.engine.state.manual.generation++
    else if(reason==='manual-change')h.engine.state.manual.changeId='d'.repeat(64)
    else h.engine.openOperationId='other-open'
    h.toolbar.selection({selection:selected})
    expect(h.toolbar.selected).toEqual(selected);expect(h.toolbar.lease).toBeNull();expect(h.toolbar.bar.hidden).toBe(true)
    expect(h.send.mock.calls.at(-1)![0].phase).toBe('cancel')
    h.captured.fire(button,'click')
    expect(h.send.mock.calls.filter(([message])=>message.phase==='format')).toHaveLength(0)
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Late',enabled:true}],formatting})
    expect(h.toolbar.bar.hidden).toBe(true)
  } finally {h.close()}
})

test.each(['generation','manual-change'] as const)('a pending offer cannot adopt a new manual owner after %s changes',reason=>{
  const h=formatPointerFixture()
  try {
    h.start();h.confirm();h.end();const lease=h.toolbar.lease,selected=structuredClone(h.toolbar.selected)
    if(reason==='generation')h.engine.state.manual.generation++
    else h.engine.state.manual.changeId='d'.repeat(64)
    h.toolbar.selection({selection:selected})
    expect(h.toolbar.lease).toBeNull();expect(h.toolbar.bar.hidden).toBe(true)
    expect(h.send.mock.calls.at(-1)![0].phase).toBe('cancel')
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Late',enabled:true}],formatting:h.formatting(selected)})
    expect(h.toolbar.bar.hidden).toBe(true)
  } finally {h.close()}
})

test('returning from native range B to A cannot revive the retired A format offer',()=>{
  const h=formatPointerFixture()
  try {
    const lease=h.present(),formatting=h.formatting(),a=structuredClone(h.toolbar.selected)
    const b={...structuredClone(a),token:'range_b_token',rangeLease:{...a.rangeLease,token:'range_b_token'}}
    h.toolbar.selection({selection:b});h.toolbar.selection({selection:a})
    h.toolbar.confirm({selectionToken:a.token,eligible:true,changed:true})
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Late A',enabled:true}],formatting})
    expect(h.toolbar.lease).toBeNull();expect(h.toolbar.bar.hidden).toBe(true)
    expect(h.send.mock.calls.filter(([message])=>message.phase==='request')).toHaveLength(1)
    expect(h.send.mock.calls.filter(([message])=>message.phase==='format')).toHaveLength(0)
  } finally {h.close()}
})

test.each([
  ['font-family','change','Synthetic Engine Serif',{kind:'font-family',value:'Synthetic Engine Serif'}],
  ['font-size','change','13.5',{kind:'font-size',value:13.5}],
  ['bold','click',null,{kind:'bold',value:true}],
  ['italic','click',null,{kind:'italic',value:false}],
  ['underline','click',null,{kind:'underline',value:true}]
] as const)('the actual %s control listener emits one closed format choice and consumes its lease', (kind,type,value,format)=>{
  const h=formatPointerFixture()
  try {
    const lease=h.present(),control=h.control(kind)
    expect(control).not.toBeNull();expect(control.getAttribute('aria-label')).toBe(pointerFormatLabels[kind==='font-family'?'fontFamily':kind==='font-size'?'fontSize':kind])
    control.focus();if(value!==null) control.value=value
    control.dispatchEvent(new Event(type,{bubbles:true,cancelable:true}))
    expect(h.send).toHaveBeenCalledOnce() // Synthetic dispatch never bypasses the production trust check.
    h.captured.fire(control,type)
    expect(h.send).toHaveBeenLastCalledWith({...lease,type:'pointer-actions',phase:'format',changeId:manualChange,rangeLease:{generation:1,sequence:0,token:lease.selectionToken},format})
    expect(h.toolbar.lease).toBeNull();expect(h.toolbar.bar.getAttribute('aria-busy')).toBe('true')
    for(const item of h.toolbar.bar.querySelectorAll('button,input,select')) expect(item.disabled).toBe(true)
    h.captured.fire(control,type)
    expect(h.send).toHaveBeenCalledTimes(2)
  } finally {h.close()}
})

test('mixed native values stay mixed and a single enumerated family is a truthful focusable output',()=>{
  const h=formatPointerFixture()
  try {
    h.engine.fontFamilies=['Noto Sans CJK SC'];Object.assign(h.selected,{formatState:{fontFamily:null,fontSize:null,bold:null,italic:null,underline:null}})
    h.present()
    expect(h.control('font-family').tagName).toBe('OUTPUT');expect(h.control('font-family').textContent).toBe('Mixed');expect(h.control('font-family').tabIndex).toBe(0)
    expect(h.toolbar.bar.querySelector('select')).toBeNull()
    const size=h.control('font-size') as HTMLInputElement
    expect(size.value).toBe('');expect(size.placeholder).toBe('Mixed');expect([size.min,size.max,size.step]).toEqual(['6','96','0.5'])
    for(const kind of ['bold','italic','underline']) expect(h.control(kind).getAttribute('aria-pressed')).toBe('mixed')
    size.focus();h.captured.fire(size,'change');expect(h.send).toHaveBeenCalledOnce()
    for(const value of ['5','96.5','12.25']) {size.value=value;h.captured.fire(size,'change')}
    expect(h.send).toHaveBeenCalledOnce()
    size.value='12.5';const enter=h.captured.fire(size,'keydown',{key:'Enter'})
    expect(enter.preventDefault).toHaveBeenCalledOnce();expect(h.send.mock.calls.at(-1)![0].format).toEqual({kind:'font-size',value:12.5})
  } finally {h.close()}
})

test.each([[],['Noto Sans CJK SC'],['Noto Sans CJK SC','Synthetic Engine Serif']].map(fonts=>({fonts})))('font choice exposes only the engine supplied list $fonts',({fonts})=>{
  const h=formatPointerFixture()
  try {
    h.engine.fontFamilies=fonts;h.present()
    if(fonts.length<2) {
      expect(h.control('font-family').tagName).toBe('OUTPUT');expect(h.control('font-family').textContent).toBe('Noto Sans CJK SC')
      expect(h.toolbar.bar.querySelector('select')).toBeNull()
    } else {
      const family=h.control('font-family') as HTMLSelectElement
      expect([...family.options].map(option=>option.value)).toEqual(fonts)
      family.value='not-an-engine-font';h.captured.fire(family,'change');expect(h.send).toHaveBeenCalledOnce()
    }
  } finally {h.close()}
})

test.each(['readonly','no-manual','no-input-owner','frozen','stale-sequence','old-token','other-document','other-generation','unqualified-runtime','saving','busy','pending-input','composition'])('format controls require current explicit manual range authority at render and apply: %s',reason=>{
  const h=formatPointerFixture()
  const invalidate=()=>{
    if(reason==='readonly') h.engine.nativeWritable=false
    else if(reason==='no-manual') delete h.engine.state.manual
    else if(reason==='no-input-owner') h.engine.manualInput=null
    else if(reason==='frozen') h.engine.state.manual.frozen=true
    else if(reason==='stale-sequence') h.engine.state.changeSequence++
    else if(reason==='old-token') h.toolbar.selected={...h.toolbar.selected,token:'different_token'}
    else if(reason==='other-document') h.engine.state.documentId='other'
    else if(reason==='other-generation') h.engine.state.manual.generation++
    else if(reason==='unqualified-runtime') h.engine.manualAvailable.mockReturnValue(false)
    else if(reason==='saving') h.engine.inputGate=true
    else if(reason==='busy') h.engine.busy=true
    else if(reason==='pending-input') h.engine.inputPending=1
    else h.engine.composition={serial:1}
  }
  try {
    h.present();const control=h.control('bold'),lease={...h.toolbar.lease},formatting=h.formatting()
    invalidate();h.captured.fire(control,'click')
    expect(h.send.mock.calls.filter(([message])=>message.phase==='format')).toHaveLength(0)
    h.toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],formatting})
    expect(h.toolbar.bar.hidden).toBe(true);expect(h.toolbar.bar.querySelector('[data-format-control]')).toBeNull();expect(h.toolbar.lease).toBeNull()
  } finally {h.close()}
})

test('a successful format requires its exact queued command, new native token and sequence, then starts a fresh interaction for a second format',()=>{
  const h=formatPointerFixture()
  try {
    const old=h.present();h.captured.fire(h.control('bold'),'click')
    const r=h.request()
    for(const extra of [{channel:'other'},{documentId:'other'},{version:'other'},{changeId:'c'.repeat(64)},{rangeLease:{...r.rangeLease,token:'other_token'}},{rangeLease:{...r.rangeLease,generation:2}},{rangeLease:{...r.rangeLease,sequence:1}},{format:{kind:'bold',value:false}},{command:'replace'}]) expect(h.toolbar.beginFormat({...r,...extra})).toBe(false)
    expect(h.toolbar.continuation.request).toBeNull();expect(h.toolbar.beginFormat(r)).toBe(true);expect(h.toolbar.beginFormat(r)).toBe(false)
    const result=h.result(r)
    h.engine.state=result.state;h.toolbar.selection({selection:result.selection});h.toolbar.changed();h.toolbar.completeFormat(r,result)
    const next=h.send.mock.calls.at(-1)![0]
    expect(next).toMatchObject({type:'pointer-actions',phase:'request',expectedChangeSequence:1,selectionToken:'selection_next_01'})
    expect(next.interactionId).not.toBe(old.interactionId);expect(h.toolbar.continuation).toBeNull()
    h.toolbar.receive({...old,phase:'present',actions:[{id:'quote',label:'Old',enabled:true}],formatting:{changeId:manualChange,rangeLease:r.rangeLease,formatState:pointerFormatState,labels:pointerFormatLabels}})
    expect(h.toolbar.bar.getAttribute('aria-busy')).toBe('true')
    h.toolbar.receive({...next,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],formatting:h.formatting()})
    expect(h.toolbar.bar.getAttribute('aria-busy')).toBeNull();expect(h.control('bold').getAttribute('aria-pressed')).toBe('true')
    h.captured.fire(h.control('underline'),'click');const second=h.request(undefined,'format_operation_02')
    expect(second.rangeLease).toEqual(result.rangeLease);expect(second.rangeLease.token).not.toBe(r.rangeLease.token)
    expect(h.toolbar.beginFormat(second)).toBe(true)
    const again=h.result(second,2,'selection_next_02');h.engine.state=again.state;h.toolbar.completeFormat(second,again)
    expect(h.send.mock.calls.filter(([message])=>message.phase==='format')).toHaveLength(2)
    expect(h.send.mock.calls.at(-1)![0]).toMatchObject({phase:'request',expectedChangeSequence:2,selectionToken:'selection_next_02'})
    expect(h.send.mock.calls.at(-1)![0].interactionId).not.toBe(next.interactionId)
  } finally {h.close()}
})

test.each(['failed','wrong-operation','old-token','wrong-sequence','wrong-generation','mismatched-format-state','cancelled','frozen','saved','other-document','new-manual','saving','busy','retired-open','advanced-sequence','late'])('format completion cannot reopen a retired or mismatched selection: %s',reason=>{
  const h=formatPointerFixture()
  try {
    h.present();h.captured.fire(h.control('bold'),'click');const r=h.request();expect(h.toolbar.beginFormat(r)).toBe(true)
    const result=h.result(r);h.engine.state=result.state
    if(reason==='failed') result.ok=false
    else if(reason==='wrong-operation') result.operationId='unrelated_operation'
    else if(reason==='old-token') {result.selection.token=r.rangeLease.token;result.selection.rangeLease.token=r.rangeLease.token;result.rangeLease.token=r.rangeLease.token}
    else if(reason==='wrong-sequence') result.selection.changeSequence=0
    else if(reason==='wrong-generation') result.rangeLease.generation=2
    else if(reason==='mismatched-format-state') result.formatState={...result.formatState,bold:false}
    else if(reason==='cancelled' || reason==='late') h.toolbar.cancel()
    else if(reason==='frozen') {h.engine.state={...result.state,manual:{...result.state.manual,frozen:true}};h.engine.manualAvailable.mockReturnValue(false)}
    else if(reason==='saved') {h.engine.manualInput=null;h.engine.manualAvailable.mockReturnValue(false)}
    else if(reason==='other-document') h.engine.state={...result.state,documentId:'other'}
    else if(reason==='new-manual') h.engine.state={...result.state,manual:{...result.state.manual,changeId:'c'.repeat(64)}}
    else if(reason==='busy') h.engine.busy=true
    else if(reason==='retired-open') h.engine.openOperationId='other-open'
    else if(reason==='advanced-sequence') h.engine.state={...result.state,changeSequence:2}
    else h.engine.inputGate=true
    const calls=h.send.mock.calls.length;h.toolbar.completeFormat(r,result)
    expect(h.toolbar.lease).toBeNull();expect(h.toolbar.continuation).toBeNull();expect(h.toolbar.bar.hidden).toBe(true)
    expect(h.send.mock.calls.slice(calls).filter(([message])=>message.phase==='request')).toHaveLength(0)
    if(reason==='late') {h.toolbar.completeFormat(r,result);expect(h.send.mock.calls.slice(calls).filter(([message])=>message.phase==='request')).toHaveLength(0)}
  } finally {h.close()}
})

test('real Engine capture leaves focused format editing in the browser while keeping canvas admission closed behind the existing guards',async()=>{
  const captured=captureDomListeners(),h=manualSurfaceFixture()
  let toolbar:any
  try {
    h.worker.mockImplementation(async(request:any)=>request.command==='bind-input-runtime'?h.bound(request):{...h.manual(request),availableFonts:['Noto Sans CJK SC','Synthetic Engine Serif']})
    const admitted=await h.admit()
    expect(admitted.availableFonts).toBeUndefined();expect(h.engine.fontFamilies).toEqual(['Noto Sans CJK SC','Synthetic Engine Serif'])
    const root=document.createElement('main');document.body.appendChild(root);root.appendChild(h.canvas)
    const Toolbar=new Function(source.replaceAll('export ','')+'\nreturn OfficePointerToolbar;')(),send=vi.fn()
    toolbar=new Toolbar(h.canvas,root,send,()=>h.engine)
    const selected={kind:'text',documentId:'document',version:manualRevision,changeSequence:0,token:'selection_token',text:'same text',scope:'body',rangeLease:{generation:1,sequence:0,token:'selection_token'},formatState:{...pointerFormatState}}
    const pointer={isTrusted:true,isPrimary:true,button:0,pointerId:1,target:h.canvas,clientX:80,clientY:70}
    toolbar.pointerDown(pointer);toolbar.selection({selection:selected});toolbar.confirm({selectionToken:selected.token,eligible:true,changed:true});toolbar.pointerUp(pointer)
    const lease=send.mock.calls.at(-1)![0]
    toolbar.receive({...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],formatting:{changeId:manualChange,rangeLease:selected.rangeLease,formatState:selected.formatState,labels:pointerFormatLabels}})
    const size=toolbar.bar.querySelector('[data-format-control="font-size"]') as HTMLInputElement
    const family=toolbar.bar.querySelector('[data-format-control="font-family"]') as HTMLSelectElement
    const queue=vi.spyOn(h.engine,'queueText'),native=vi.fn(),clipboard=vi.fn(()=>'<html>untrusted</html>')
    for(const type of ['keydown','beforeinput','paste','compositionend']) h.canvas.addEventListener(type,native)
    h.worker.mockClear();h.canvas.focus()
    const keydowns=h.windowListeners.filter(entry=>entry.type==='keydown')
    expect(toolbar.bar.hidden).toBe(false);expect(toolbar.formatCurrent()).toBe(true);expect(keydowns).toHaveLength(3)
    const tab=pointerKey(h.canvas,'Tab')
    for(const entry of keydowns) captured.invoke(entry,tab)
    expect(tab.preventDefault).toHaveBeenCalledOnce();expect(tab.stopImmediatePropagation).toHaveBeenCalledOnce();expect(document.activeElement).toBe(family)
    size.focus();size.value='14'
    for(const type of ['beforeinput','paste','compositionstart','compositionend']) {
      const event=type==='beforeinput'?new InputEvent(type,{inputType:'insertText',data:'14',bubbles:true,cancelable:true}):new Event(type,{bubbles:true,cancelable:true})
      if(type==='paste') Object.defineProperty(event,'clipboardData',{value:{getData:clipboard}})
      size.dispatchEvent(event);expect(event.defaultPrevented).toBe(false)
    }
    for(const key of ['1','Backspace','ArrowDown','Tab']) {
      const event=new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true});size.dispatchEvent(event);expect(event.defaultPrevented).toBe(false)
    }
    // Enter/arrow admission through the actual window capture listeners is separate
    // from the size control's trusted Enter commit and from native Qt hit testing.
    for(const target of [size,family]) for(const key of ['Enter','ArrowDown']) {
      const event=pointerKey(target,key)
      for(const entry of keydowns) captured.invoke(entry,event)
      expect(event.preventDefault).not.toHaveBeenCalled()
    }
    family.focus();expect(document.activeElement).toBe(family);expect(toolbar.selected).toBe(selected)
    const bold=toolbar.bar.querySelector('[data-format-control="bold"]') as HTMLButtonElement
    const down=captured.fire(toolbar.bar,'pointerdown',{target:bold,button:0})
    expect(down.preventDefault).toHaveBeenCalledOnce();expect(document.activeElement).toBe(bold)
    expect(queue).not.toHaveBeenCalled();expect(h.worker).not.toHaveBeenCalled();expect(native).not.toHaveBeenCalled();expect(clipboard).not.toHaveBeenCalled()
    expect(h.engine.composition).toBeNull();expect(size.value).toBe('14')
    h.engine.inputGate=true
    expect(h.key('a').defaultPrevented).toBe(true)
    const before=new InputEvent('beforeinput',{inputType:'insertText',data:'canvas',bubbles:true,cancelable:true});h.canvas.dispatchEvent(before)
    expect(before.defaultPrevented).toBe(true);expect(native).not.toHaveBeenCalled();expect(queue).not.toHaveBeenCalled()
  } finally {toolbar?.cancel();h.close();captured.restore()}
})

test('the installed guest closes its format popup immediately on save from a focused size input and ignores a late menu',async()=>{
  document.body.innerHTML=new DOMParser().parseFromString(html,'text/html').body.innerHTML
  const release=trackWindowListeners(),captured=captureDomListeners(),send=vi.fn()
  let receive!:(request:unknown)=>Promise<void>,engine:any
  vi.stubGlobal('analytixOfficeSurface',{connect:(handler:typeof receive)=>{receive=handler},send})
  try {
    const Engine=new Function(source.replaceAll('export ','')+'\nreturn OfficeEngineSurface;')()
    Engine.prototype.start=async function(channel:string){
      engine=this;this.channel=channel;this.openOperationId='opened';this.editing=this.nativeWritable=true;this.inputGate=false
      this.state={kind:'docx',documentId:'document',version:manualRevision,changeSequence:0,acknowledgedSequence:0,dirty:false,manual:{version:2,origin:'manual',changeId:manualChange,baseRevision:manualRevision,generation:1,frozen:false}}
      this.manualInput={changeId:manualChange,generation:1};this.manualAvailable=()=>true;this.fontFamilies=['Noto Sans CJK SC']
    }
    await receive({type:'bind',channel:'channel'})
    const canvas=document.getElementById('qtcanvas')!,bar=document.getElementById('pointerActions')!
    const pointer={isPrimary:true,button:0,pointerId:1,clientX:90,clientY:60}
    captured.fire(canvas,'pointerdown',pointer)
    const selection={kind:'text',documentId:'document',version:manualRevision,changeSequence:0,token:'selection_token',text:'same text',scope:'body',rangeLease:{generation:1,sequence:0,token:'selection_token'},formatState:{...pointerFormatState}}
    engine.onEvent({type:'selection',selection});engine.onSelectionIdentity({selectionToken:selection.token,eligible:true,changed:true})
    captured.fire(canvas,'pointerup',pointer)
    const lease=send.mock.calls.filter(([message])=>message.type==='pointer-actions' && message.phase==='request').at(-1)![0]
    const menu={...lease,phase:'present',actions:[{id:'quote',label:'Quote',enabled:true}],formatting:{changeId:manualChange,rangeLease:selection.rangeLease,formatState:selection.formatState,labels:pointerFormatLabels}}
    await receive(menu);expect(bar.hidden).toBe(false)
    const size=bar.querySelector('[data-format-control="font-size"]') as HTMLInputElement;size.focus()
    const queue=vi.spyOn(engine,'queueText'),save=new KeyboardEvent('keydown',{key:'s',ctrlKey:true,bubbles:true,cancelable:true})
    size.dispatchEvent(save)
    expect(save.defaultPrevented).toBe(true);expect(bar.hidden).toBe(true);expect(bar.childElementCount).toBe(0)
    expect(send).toHaveBeenCalledWith(expect.objectContaining({type:'save-requested',documentId:'document',version:manualRevision}))
    expect(send).toHaveBeenCalledWith(expect.objectContaining({type:'pointer-actions',phase:'cancel',interactionId:lease.interactionId}))
    expect(queue).not.toHaveBeenCalled()
    await receive(menu);expect(bar.hidden).toBe(true);expect(bar.childElementCount).toBe(0)
  } finally {captured.restore();release();vi.unstubAllGlobals();document.body.innerHTML=''}
})


test('format continuation expiry cancels its consumed range and a late success cannot request it again',()=>{
  vi.useFakeTimers();const h=formatPointerFixture()
  try {
    h.present();h.captured.fire(h.control('bold'),'click');const r=h.request();expect(h.toolbar.beginFormat(r)).toBe(true)
    const result=h.result(r);h.engine.state=result.state;const calls=h.send.mock.calls.length
    vi.advanceTimersByTime(65000)
    expect(h.toolbar.continuation).toBeNull();expect(h.toolbar.bar.hidden).toBe(true)
    h.toolbar.completeFormat(r,result)
    expect(h.send.mock.calls.slice(calls).filter(([message])=>message.phase==='request')).toHaveLength(0)
  } finally {h.close();vi.clearAllTimers();vi.useRealTimers()}
})

test('manual admission strips private font/runtime evidence and caches an independent engine enumeration',async()=>{
  const h=manualSurfaceFixture(),fonts=['Noto Sans CJK SC','Synthetic Engine Serif']
  try {
    h.worker.mockImplementation(async(request:any)=>request.command==='bind-input-runtime'?h.bound(request):{...h.manual(request),availableFonts:fonts})
    const result=await h.admit()
    expect(result).not.toHaveProperty('availableFonts');expect(result).not.toHaveProperty('inputRuntime')
    expect(h.engine.fontFamilies).toEqual(['Noto Sans CJK SC','Synthetic Engine Serif']);expect(h.engine.fontFamilies).not.toBe(fonts)
    fonts[0]='Changed after receipt';fonts.push('Late family')
    expect(h.engine.fontFamilies).toEqual(['Noto Sans CJK SC','Synthetic Engine Serif'])
    expect(h.engine.manualInput).toMatchObject({changeId:manualChange,generation:1});expect(h.canvas.contentEditable).toBe('true')
  } finally {h.close()}
})

test.each([
  {reason:'non-array',fonts:'Noto Sans CJK SC'},
  {reason:'duplicate',fonts:['Noto Sans CJK SC','Noto Sans CJK SC']},
  {reason:'invalid-family',fonts:['Noto Sans CJK SC\n']},
  {reason:'over-bound',fonts:Array.from({length:257},(_,index)=>'Synthetic Engine Family '+index)}
])('invalid private font enumeration keeps the manual body closed: $reason',async({fonts})=>{
  const h=manualSurfaceFixture()
  try {
    h.worker.mockImplementation(async(request:any)=>request.command==='bind-input-runtime'?h.bound(request):{...h.manual(request),availableFonts:fonts})
    await expect(h.admit()).rejects.toThrow('surface-state-unknown-recreate-required')
    expect(h.engine.failed).toBe(true);expect(h.engine.manualInput).toBeNull();expect(h.engine.nativeWritable).toBe(false)
    expect(h.engine.fontFamilies).toEqual([]);expect(h.canvas.contentEditable).toBe('false');expect(h.key('a').defaultPrevented).toBe(true)
  } finally {h.close()}
})
