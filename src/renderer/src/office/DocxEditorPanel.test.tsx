// @vitest-environment jsdom
import { act,createElement,useEffect,useRef } from 'react'
import { createRoot,type Root } from 'react-dom/client'
import { Blob as NodeBlob,File as NodeFile } from 'node:buffer'
import { webcrypto } from 'node:crypto'
import { beforeEach,afterEach,it,expect,vi } from 'vitest'
import { DocxEditorPanel } from './DocxEditorPanel'
import { useDocxEditorStore } from './docx-editor-store'
import type { DocxEditorCaptureRequest,DocxEditorCapture } from '@shared/docx-editor'
import type { DocxEditorView } from '@shared/docx-editor'
const state=vi.hoisted(()=>({observe:undefined as ((snapshot:unknown)=>void)|undefined,observers:new Set<(snapshot:unknown)=>void>(),snapshot:{status:'ready',empty:false,quotedText:'Original'},selectionCapture:vi.fn(),viewport:undefined as (()=>void)|undefined,geometry:vi.fn(),viewportDetached:vi.fn(),capture:undefined as ((challenge:DocxEditorCaptureRequest)=>DocxEditorCapture|null)|undefined,replace:vi.fn(),execute:vi.fn(),export:vi.fn(),ready:vi.fn(),restore:vi.fn(),mode:vi.fn(),request:vi.fn(),saveAs:vi.fn()}))
const selected={status:'ready',empty:false,quotedText:'Original'}
const capture={quotedText:'Original',selectionTarget:{start:{kind:'text',blockId:'block',offset:0},end:{kind:'text',blockId:'block',offset:8}}}
const rectangle=(overrides:Partial<{left:number;right:number;top:number;bottom:number}>={})=>({pageIndex:0,left:40,right:140,top:100,bottom:120,width:100,height:20,...overrides})
const workerPaths={document:'superdoc-engine/assets/document-fixture.js',collaboration:'superdoc-engine/assets/collaboration-fixture.js',reviewIndex:'superdoc-engine/assets/review-fixture.js'}
const sd={ui:{viewport:{getRect:state.geometry,observe:(callback:()=>void)=>{state.viewport=callback;return()=>{state.viewport=undefined;state.viewportDetached()}}},selection:{observe:(callback:(snapshot:unknown)=>void)=>{state.observers.add(callback);state.observe=snapshot=>{state.snapshot=snapshot as typeof state.snapshot;for(const listener of [...state.observers])listener(snapshot)};return ()=>{state.observers.delete(callback);if(!state.observers.size)state.observe=undefined}},getSnapshot:()=>state.snapshot,capture:state.selectionCapture,
  restore:state.restore},commands:{get:()=>({getState:()=>({enabled:true})}),executeAsync:state.execute}},
  activeEditor:{doc:{replace:state.replace}},export:state.export,setDocumentMode:state.mode}
vi.mock('@superdoc/react',()=>({SuperDocEditor:(props:{onReady:(event:unknown)=>void})=>{
  state.ready(props);const onReady=useRef(props.onReady);useEffect(()=>{onReady.current({superdoc:sd})},[]);return createElement('div',{'data-testid':'upstream-editor'})
}}))
vi.mock('../components/write/WriteInlineAgent',()=>({WriteInlineAgent:(props:{action:{left:number;width:number;anchorTop:number;anchorBottom:number};value:string;textareaRef:{current:HTMLTextAreaElement|null};onValueChange:(value:string)=>void;onApplyEdit:(value:string)=>void;onQuoteSelection:()=>void;quickActions:{id:string;label:string;prompt:string;mode:string}[];onQuickAction:(action:{id:string;label:string;prompt:string;mode:string})=>void})=>createElement('div',{'data-floating-agent':'','data-left':props.action.left,'data-width':props.action.width,'data-top':props.action.anchorTop,'data-bottom':props.action.anchorBottom},
  createElement('textarea',{ref:props.textareaRef,value:props.value,onChange:()=>{},onInput:(event:{currentTarget:HTMLTextAreaElement})=>props.onValueChange(event.currentTarget.value)}),
  createElement('button',{onClick:()=>props.onApplyEdit(props.value||'Polish')},'Request suggestion'),createElement('button',{onClick:props.onQuoteSelection},'Quote'),
  ...props.quickActions.map(action=>createElement('button',{key:action.id,onClick:()=>props.onQuickAction(action)},action.label)))}))
const base:DocxEditorView={editorId:'00000000-0000-4000-8000-000000000001',workspace:'/workspace',path:'/workspace/report.docx',threadId:'thread',objectId:'a'.repeat(64),revision:'b'.repeat(64),
  version:0,phase:'ready',dirty:false,content:{encoding:'base64',kind:'docx',byteLength:4,sha256:'b'.repeat(64),data:'UEsDBA=='},proposals:[],reviews:[]}
let root:Root,container:HTMLDivElement
const submit=vi.fn(),prepare=vi.fn(()=>true)
async function render(view:DocxEditorView,visible=true,fileActions?:Parameters<typeof DocxEditorPanel>[0]['fileActions']){useDocxEditorStore.setState({views:{[view.editorId]:view}});await act(async()=>root.render(createElement(DocxEditorPanel,{view,visible,onSubmitPrompt:submit,onPrepareDraft:prepare,fileActions})))}
async function flush(){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,25))})}
function namedButton(label:string){return [...document.querySelectorAll<HTMLButtonElement>('button')].find(b=>b.getAttribute('aria-label')===label||b.textContent===label)}
async function click(label:string){
  let button=namedButton(label)
  if(!button&&['放弃工作副本','撤销已保存修改','完成已保存撤销'].includes(label)){
    await act(async()=>namedButton('更多 Word 操作')!.click());button=namedButton(label)
  }
  expect(button).toBeTruthy();await act(async()=>button!.click());await flush()
}
beforeEach(()=>{
  vi.stubGlobal('innerWidth',1280)
  vi.stubGlobal('__ANALYTIX_DOCX_WORKER_PATHS__',workerPaths)
  vi.spyOn(HTMLElement.prototype,'getBoundingClientRect').mockReturnValue({left:0,right:900,top:0,bottom:700,width:900,height:700,x:0,y:0,toJSON:()=>({})})
  state.geometry.mockReturnValue({found:true,rects:[rectangle()]})
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('Blob',NodeBlob);vi.stubGlobal('File',NodeFile);vi.stubGlobal('crypto',webcrypto)
  state.observers.clear();state.snapshot={...selected};state.selectionCapture.mockReset().mockImplementation(()=>state.snapshot.status==='ready'&&!state.snapshot.empty?capture:null)
  vi.clearAllMocks();state.restore.mockReset().mockReturnValue({ok:true});state.execute.mockResolvedValue({success:true});state.replace.mockResolvedValue({success:true})
  state.export.mockResolvedValue(new NodeBlob([Uint8Array.from([80,75,3,4,7])]))
  state.request.mockImplementation(async()=>({ok:true,view:base}));state.saveAs.mockResolvedValue({ok:true,path:'/saved.docx'})
  window.analytix={docxEditor:{request:state.request,onCapture:(handler:NonNullable<typeof state.capture>)=>{state.capture=handler;return()=>{state.capture=undefined}}},files:{saveAs:state.saveAs}} as unknown as typeof window.analytix
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
})
afterEach(async()=>{await act(async()=>root.unmount());container.remove();vi.restoreAllMocks();vi.unstubAllGlobals()})
it('exports the current Blob for Save As without sourcePath and keeps the upstream document stable across a saved revision',async()=>{
  await render(base);expect((state.ready.mock.calls.at(-1)![0] as {allowSelectionInViewMode:boolean}).allowSelectionInViewMode).toBe(true);expect((state.ready.mock.calls.at(-1)![0] as {telemetry:unknown}).telemetry).toEqual({enabled:false});const source=(state.ready.mock.calls.at(-1)![0] as {document:File}).document
  await click('另存为')
  expect(state.export).toHaveBeenCalledWith({exportType:['docx'],triggerDownload:false})
  expect(state.saveAs).toHaveBeenCalledWith(expect.objectContaining({dataBase64:'UEsDBAc='}))
  expect(state.saveAs.mock.calls[0][0]).not.toHaveProperty('sourcePath')
  await render({...base,revision:'c'.repeat(64)})
  expect((state.ready.mock.calls.at(-1)![0] as {document:File}).document).toBe(source)
})
it('uses the upstream format command and records one local version',async()=>{
  await render({...base,phase:'editing'})
  await act(async()=>state.observe?.(selected))
  const bold=container.querySelector<HTMLButtonElement>('[aria-label="加粗"]')!
  await act(async()=>bold.click())
  expect(state.restore).toHaveBeenCalledWith(capture)
  expect(state.execute).toHaveBeenCalledWith('bold',undefined)
  expect(state.request).toHaveBeenCalledWith({action:'changed',editorId:base.editorId,version:1})
})
it('restores the captured SDK range after a font control takes focus before formatting',async()=>{
  const view={...base,phase:'editing' as const}
  state.request.mockResolvedValue({ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected))
  const font=container.querySelector<HTMLSelectElement>('[aria-label="字体"]')!
  await act(async()=>{font.focus();state.observe?.({status:'ready',empty:true,quotedText:''})})
  state.restore.mockImplementation(()=>{setTimeout(()=>state.observe?.(selected),0);return {ok:true}})
  await act(async()=>{font.value='Arial';font.dispatchEvent(new Event('change',{bubbles:true}))});await flush()
  expect(state.execute).toHaveBeenCalledWith('font-family','Arial')
  expect(state.request).toHaveBeenCalledWith({action:'changed',editorId:base.editorId,version:1})
})
it('does not send a late capture to another document',async()=>{
  let finish!:(value:unknown)=>void
  state.request.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  await render(base);await act(async()=>state.observe?.(selected))
  const button=[...container.querySelectorAll('button')].find(b=>b.textContent==='Request suggestion')!
  await act(async()=>button.click())
  await render(base,false)
  await act(async()=>finish({ok:true,view:{...base,scope:{scopeId:'d'.repeat(48),selectionToken:'opaque_token',baseRevision:base.revision,changeSequence:0,threadId:'thread',editable:true,parts:[]}}}))
  expect(submit).not.toHaveBeenCalled();expect(prepare).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})

it.each([false,true])('routes Agent decision accept=%s through Core and applies only an accepted current replacement',async accept=>{
  const scope={scopeId:'d'.repeat(48),selectionToken:'opaque_token',baseRevision:base.revision,changeSequence:0,threadId:'thread',editable:true,parts:[]}
  const proposalId='e'.repeat(48)
  const view={...base,scope,proposals:[{proposalId,status:'proposed' as const,parts:[]}],reviews:[{proposalId,beforeText:'Original',afterText:'Replacement'}]}
  state.request.mockImplementation(async input=>input.action==='decide' && input.accept
    ? {ok:true,view:{...view,version:0,phase:'applying',dirty:true},replacement:'Replacement'}
    : input.action==='applied'?{ok:true,view:{...view,version:1,phase:'editing',dirty:true}}:{ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected))
  await click(accept?'应用并保存':'拒绝')
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'decide',accept,proposalId,scopeId:scope.scopeId}))
  if(accept){
    expect(state.replace).toHaveBeenCalledWith({target:capture.selectionTarget,text:'Replacement'},{changeMode:'direct'})
    expect(state.mode).toHaveBeenLastCalledWith('viewing')
    expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'save',version:1}))
  }else{
    expect(state.replace).not.toHaveBeenCalled()
    expect(state.request.mock.calls.some(([input])=>input.action==='save')).toBe(false)
  }
})

it('queries a lost applied ACK without applying the replacement twice',async()=>{
  const scope={scopeId:'d'.repeat(48),selectionToken:'opaque_token',baseRevision:base.revision,changeSequence:0,threadId:'thread',editable:true,parts:[]}
  const proposalId='e'.repeat(48)
  const view={...base,scope,proposals:[{proposalId,status:'proposed' as const,parts:[]}],reviews:[{proposalId,beforeText:'Original',afterText:'Replacement'}]}
  let acknowledged=false
  state.request.mockImplementation(async input=>{
    if(input.action==='decide')return {ok:true,view:{...view,phase:'applying',dirty:true},replacement:'Replacement'}
    if(input.action==='applied'){acknowledged=true;return {ok:false,error:'unknown',view:{...view,version:1,phase:'editing',dirty:true}}}
    if(input.action==='status'&&acknowledged)return {ok:true,view:{...view,version:1,phase:'editing',dirty:true}}
    return {ok:true,view}
  })
  await render(view);await act(async()=>state.observe?.(selected));await click('应用并保存')
  expect(state.replace).toHaveBeenCalledTimes(1)
  await render({...view,version:1,phase:'editing',dirty:true})
  await click('查询结果')
  expect(state.replace).toHaveBeenCalledTimes(1)
  expect(namedButton('保存')?.disabled).toBe(false)
})
it('invalidates an exported snapshot when a queued upstream mutation arrives during Save',async()=>{
  const view={...base,phase:'editing' as const,dirty:true}
  state.request.mockResolvedValue({ok:true,view})
  let finish!:(value:Blob)=>void
  state.export.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  await render(view);await click('保存')
  await act(async()=>{(state.ready.mock.calls.at(-1)![0] as {onEditorUpdate:()=>void}).onEditorUpdate()})
  await act(async()=>finish(new NodeBlob([Uint8Array.from([80,75,3,4,7])]) as unknown as Blob));await flush()
  expect(state.request.mock.calls.some(([input])=>input.action==='save')).toBe(false)
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'changed',version:1}))
  expect(container.querySelector('[role="alert"]')?.textContent).toContain('文档仍在更新')
})

it('uses renderer-defined worker paths without embedding upstream asset hashes',async()=>{
  await render(base)
  expect((state.ready.mock.calls.at(-1)![0] as {workerUrls:unknown}).workerUrls).toEqual(Object.fromEntries(Object.entries(workerPaths).map(([key,path])=>[key,new URL('./'+path,window.location.href).href])))
})
it('tracks viewport geometry, hides offscreen/unresolved ranges and restores the same capture without requests',async()=>{
  const scope={scopeId:'d'.repeat(48),selectionToken:'opaque_token',baseRevision:base.revision,changeSequence:0,threadId:'thread',editable:false,parts:[]}
  state.request.mockResolvedValue({ok:true,view:{...base,scope}})
  await render(base);await act(async()=>state.observe?.(selected));await click('Quote')
  const challenge={requestId:'00000000-0000-4000-8000-000000000009',editorId:base.editorId,version:0,revision:base.revision}
  const initial=state.capture!(challenge)!,requests=state.request.mock.calls.length
  expect(initial.captureId).toBeTruthy();expect(prepare).toHaveBeenCalledTimes(1)
  state.geometry.mockReturnValue({found:true,rects:[rectangle({left:600,right:700,top:180,bottom:200})]})
  await act(async()=>state.viewport?.())
  expect(container.querySelector('[data-floating-agent]')?.getAttribute('data-left')).toBe('480')
  expect(container.querySelector('[data-floating-agent]')?.getAttribute('data-top')).toBe('180')
  expect(state.geometry).toHaveBeenLastCalledWith({target:capture.selectionTarget})
  for(const result of [{found:true,rects:[rectangle({top:800,bottom:820})]},{found:false,rects:[]},{found:true,rects:[rectangle({left:NaN})]}]){
    state.geometry.mockReturnValue(result);await act(async()=>state.viewport?.())
    expect(container.querySelector('[data-floating-agent]')).toBeNull()
  }
  state.geometry.mockImplementationOnce(()=>{throw Error('unresolved geometry')})
  await act(async()=>state.viewport?.());expect(container.querySelector('[data-floating-agent]')).toBeNull()
  state.geometry.mockReturnValue({found:true,rects:[rectangle({top:-80,bottom:-60}),rectangle({top:60,bottom:80})]})
  await act(async()=>state.viewport?.())
  expect(container.querySelector('[data-floating-agent]')?.getAttribute('data-top')).toBe('60')
  expect(state.capture!(challenge)?.captureId).toBe(initial.captureId)
  expect(useDocxEditorStore.getState().views[base.editorId].scope?.selectionToken).toBe(scope.selectionToken)
  expect(state.request).toHaveBeenCalledTimes(requests);expect(prepare).toHaveBeenCalledTimes(1);expect(submit).not.toHaveBeenCalled()
})
it('clips geometry to the editor scroll container as well as the window',async()=>{
  await render(base);await act(async()=>state.observe?.(selected))
  const host=container.querySelector('[aria-busy]')!
  vi.spyOn(host,'getBoundingClientRect').mockReturnValue({left:20,right:250,top:20,bottom:180,width:230,height:160,x:20,y:20,toJSON:()=>({})})
  state.geometry.mockReturnValue({found:true,rects:[rectangle({top:200,bottom:220})]})
  await act(async()=>state.viewport?.());expect(container.querySelector('[data-floating-agent]')).toBeNull()
  state.geometry.mockReturnValue({found:true,rects:[rectangle({left:5,right:50,top:160,bottom:200})]})
  await act(async()=>state.viewport?.())
  const floating=container.querySelector('[data-floating-agent]')!
  expect(floating.getAttribute('data-left')).toBe('36');expect(floating.getAttribute('data-bottom')).toBe('180')
})
it.each(['version','revision'] as const)('hides stale %s geometry without recapturing',async field=>{
  await render(base);await act(async()=>state.observe?.(selected))
  await render({...base,...(field==='version'?{version:1}:{revision:'c'.repeat(64)})})
  state.geometry.mockClear();await act(async()=>state.viewport?.())
  expect(container.querySelector('[data-floating-agent]')).toBeNull();expect(state.geometry).not.toHaveBeenCalled();expect(state.request).not.toHaveBeenCalled()
})
it('cleans both subscriptions and ignores an already-queued viewport callback after unmount',async()=>{
  await render(base);await act(async()=>state.observe?.(selected))
  const queued=state.viewport!
  await act(async()=>root.unmount())
  state.geometry.mockClear();await act(async()=>queued())
  expect(state.viewportDetached).toHaveBeenCalledTimes(1);expect(state.observe).toBeUndefined();expect(state.geometry).not.toHaveBeenCalled()
  root=createRoot(container)
})

it.each([0.75,1,1.25])('bounds the viewing AI popover inside a 364px right panel at zoom %s',async zoom=>{
  vi.stubGlobal('innerWidth',1280)
  vi.spyOn(window,'getComputedStyle').mockReturnValue({zoom:String(zoom)} as CSSStyleDeclaration)
  await render(base)
  const host=container.querySelector('[aria-busy]')!
  vi.spyOn(host,'getBoundingClientRect').mockReturnValue({left:912,right:1276,top:131,bottom:836,width:364,height:705,x:912,y:131,toJSON:()=>({})})
  state.geometry.mockReturnValue({found:true,rects:[rectangle({left:1015.4,right:1115.4,top:281.8,bottom:301.8})]})
  await act(async()=>state.observe?.(selected))
  const floating=container.querySelector('[data-floating-agent]')!
  expect(floating).toBeTruthy()
  const left=Number(floating.getAttribute('data-left'))*zoom,width=Number(floating.getAttribute('data-width'))*zoom
  expect(width).toBeGreaterThan(0)
  expect(left).toBeGreaterThanOrEqual(912)
  expect(left+width).toBeLessThanOrEqual(1276)
  expect(left+width).toBeLessThan(1280)
  expect(Number(floating.getAttribute('data-top'))*zoom).toBeCloseTo(281.8)
  expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled()
})


const empty={status:'ready',empty:true,quotedText:''}
const challenge={requestId:'00000000-0000-4000-8000-000000000009',editorId:base.editorId,version:0,revision:base.revision}
const scope={scopeId:'d'.repeat(48),selectionToken:'opaque_token',baseRevision:base.revision,changeSequence:0,threadId:'thread',editable:true,parts:[]}
async function focusInline(){
  const input=container.querySelector('textarea')!
  expect(input).toBeTruthy()
  await act(async()=>{
    input.dispatchEvent(new MouseEvent('pointerdown',{bubbles:true}))
    state.observe?.({...empty,status:'pending'})
    input.focus()
    state.observe?.(empty)
  })
  return input
}
it('keeps the real capture and typed prompt when focus enters its inline input, without restoring during typing',async()=>{
  await render(base);await act(async()=>state.observe?.(selected))
  const input=await focusInline()
  await act(async()=>{input.value='Polish this selection';input.dispatchEvent(new Event('input',{bubbles:true}));state.observe?.({...empty,status:'stale'})})
  expect(document.activeElement).toBe(input)
  expect(container.querySelector('textarea')).toBe(input)
  expect(input.value).toBe('Polish this selection')
  expect(state.capture!(challenge)).toBeNull()
  expect(state.restore).not.toHaveBeenCalled();expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled()
})
it('restores and verifies the SDK asynchronously before a focused prompt can obtain a Core scope',async()=>{
  await render(base);await act(async()=>state.observe?.(selected))
  const proof=state.capture!(challenge)!,input=await focusInline()
  await act(async()=>{input.value='Keep these facts';input.dispatchEvent(new Event('input',{bubbles:true}))})
  state.restore.mockImplementation(()=>{setTimeout(()=>state.observe?.(selected),0);return {ok:true}})
  state.request.mockImplementation(async request=>{
    if(request.action==='capture')expect(state.capture!(challenge)).toMatchObject({captureId:proof.captureId,text:'Original',target:capture.selectionTarget})
    return {ok:true,view:{...base,scope}}
  })
  await click('Request suggestion')
  expect(state.restore).toHaveBeenCalledWith(capture)
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'capture',captureId:proof.captureId,editable:true}))
  expect(submit).toHaveBeenCalledWith('Keep these facts',[expect.objectContaining({kind:'docx',scopeId:scope.scopeId,selectionToken:scope.selectionToken})])
  expect(state.replace).not.toHaveBeenCalled()
  expect(state.observers.size).toBe(1)
})
it.each(['refused','text','target'])('does not dispatch or replace when focus restoration is %s',async failure=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  state.restore.mockImplementation(()=>{
    if(failure==='refused')return {ok:false}
    state.selectionCapture.mockReturnValue(failure==='target'?{...capture,selectionTarget:{...capture.selectionTarget,end:{...capture.selectionTarget.end,offset:4}}}:{...capture,quotedText:'Different'})
    setTimeout(()=>state.observe?.({...selected,quotedText:failure==='text'?'Different':'Original'}),0)
    return {ok:true}
  })
  await click('Request suggestion')
  expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
  expect(state.capture!(challenge)).toBeNull();expect(state.observers.size).toBe(1)
})
it('does not preserve an empty SDK selection without an owned focus handoff',async()=>{
  await render(base);await act(async()=>state.observe?.(selected));await act(async()=>state.observe?.(empty))
  expect(container.querySelector('[data-floating-agent]')).toBeNull();expect(state.capture!(challenge)).toBeNull()
})
it('replaces the held capture when a genuinely different selection arrives and retires its scope',async()=>{
  await render(base);await act(async()=>state.observe?.(selected));const old=state.capture!(challenge)!
  await focusInline()
  const next={...capture,quotedText:'New text',selectionTarget:{...capture.selectionTarget,end:{...capture.selectionTarget.end,offset:4}}}
  state.selectionCapture.mockReturnValue(next)
  await act(async()=>state.observe?.({...selected,quotedText:'New text'}))
  const proof=state.capture!(challenge)!
  expect(proof.captureId).not.toBe(old.captureId);expect(proof.text).toBe('New text');expect(proof.target).toEqual(next.selectionTarget)
  expect(state.request).toHaveBeenCalledWith({action:'revoke',editorId:base.editorId})
  await act(async()=>state.observe?.(empty));expect(container.querySelector('[data-floating-agent]')).toBeNull()
})
it.each(['version','revision','path','workspace','threadId','objectId','editorId','hidden','scope'] as const)('retires a held capture after %s changes',async field=>{
  const initial=field==='scope'?{...base,scope}:base
  state.request.mockResolvedValue({ok:true,view:initial})
  await render(initial);await act(async()=>state.observe?.(selected));await focusInline()
  const next:DocxEditorView=field==='version'?{...initial,version:1}:field==='revision'?{...initial,revision:'c'.repeat(64)}:field==='scope'?{...initial,scope:undefined}:
    field==='hidden'?initial:{...initial,[field]:field==='editorId'?'00000000-0000-4000-8000-000000000002':'other'}
  await render(next,field!=='hidden')
  expect(container.querySelector('[data-floating-agent]')).toBeNull();expect(state.capture!(challenge)).toBeNull()
  expect(state.restore).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})
it('cancels an unsettled restore on close and ignores its late SDK result',async()=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  state.restore.mockReturnValue({ok:true})
  await click('Request suggestion')
  expect(state.observers.size).toBe(2);expect(state.request).not.toHaveBeenCalled()
  await render(base,false)
  await act(async()=>state.observe?.(selected))
  expect(state.observers.size).toBe(1);expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})
it.each([false,true])('keeps Core permission gates for review accept=%s after an inline focus handoff',async accept=>{
  const proposalId='e'.repeat(48),view={...base,scope,proposals:[{proposalId,status:'proposed' as const,parts:[]}],reviews:[{proposalId,beforeText:'Original',afterText:'Replacement'}]}
  state.request.mockImplementation(async input=>input.action==='decide'&&input.accept?{ok:true,view:{...view,phase:'applying',dirty:true},replacement:'Replacement'}:
    input.action==='applied'?{ok:true,view:{...view,version:1,phase:'editing',dirty:true}}:{ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected));await focusInline()
  state.restore.mockImplementation(()=>{setTimeout(()=>state.observe?.(selected),0);return {ok:true}})
  await click(accept?'应用并保存':'拒绝')
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'decide',accept,scopeId:scope.scopeId,proposalId}))
  if(accept){expect(state.replace).toHaveBeenCalledWith({target:capture.selectionTarget,text:'Replacement'},{changeMode:'direct'});expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'save'}))}
  else {expect(state.replace).not.toHaveBeenCalled();expect(state.request.mock.calls.some(([request])=>request.action==='save')).toBe(false)}
})

it('retires a bound capture when a recapture fails instead of dropping its revocation binding',async()=>{
  const view={...base,scope}
  state.request.mockResolvedValue({ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected));await focusInline()
  state.restore.mockImplementation(()=>{setTimeout(()=>state.observe?.(selected),0);return {ok:true}})
  state.request.mockImplementation(async input=>input.action==='capture'?{ok:false,error:'stale_selection',view:base}:{ok:true,view})
  await click('Request suggestion')
  expect(state.capture!(challenge)).toBeNull();expect(container.querySelector('[data-floating-agent]')).toBeNull()
  expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})
it('rejects a late successful recapture after the previously bound scope is revoked',async()=>{
  const view={...base,scope}
  state.request.mockResolvedValue({ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected));await focusInline()
  state.restore.mockImplementation(()=>{setTimeout(()=>state.observe?.(selected),0);return {ok:true}})
  let finish!:(result:unknown)=>void
  state.request.mockImplementation(input=>input.action==='capture'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:base}))
  await click('Request suggestion');expect(finish).toBeTruthy()
  await render(base)
  expect(state.capture!(challenge)).toBeNull()
  await act(async()=>finish({ok:true,view:{...base,scope:{...scope,scopeId:'f'.repeat(48)}}}))
  expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
  expect(state.request).toHaveBeenCalledWith({action:'revoke',editorId:base.editorId})
})

it('keeps a same-batch scope revocation retired when a late recapture writes a nonempty scope',async()=>{
  const view={...base,scope}
  state.request.mockResolvedValue({ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected));await focusInline()
  state.restore.mockImplementation(()=>{setTimeout(()=>state.observe?.(selected),0);return {ok:true}})
  let finish!:(result:unknown)=>void
  state.request.mockImplementation(input=>input.action==='capture'?new Promise(resolve=>{finish=resolve}):Promise.resolve({ok:true,view:base}))
  await click('Request suggestion');expect(finish).toBeTruthy()
  await act(async()=>{
    useDocxEditorStore.setState({views:{[base.editorId]:base}})
    finish({ok:true,view:{...base,scope:{...scope,scopeId:'f'.repeat(48)}}})
  })
  expect(state.capture!(challenge)).toBeNull();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
  expect(state.request).toHaveBeenCalledWith({action:'revoke',editorId:base.editorId})
})

it.each(['focus','keyboard'])('retires the held source when the document receives a new %s interaction',async kind=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  const editor=container.querySelector<HTMLElement>('[data-testid="upstream-editor"]')!
  await act(async()=>{
    if(kind==='focus'){editor.tabIndex=0;editor.focus()}
    else editor.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true}))
  })
  expect(container.querySelector('[data-floating-agent]')).toBeNull();expect(state.capture!(challenge)).toBeNull()
  expect(state.restore).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})


it.each(['sync','async'])('keeps its capture when SDK restore produces %s editor focus before the ready snapshot',async timing=>{
  await render(base);await act(async()=>state.observe?.(selected));const proof=state.capture!(challenge)!
  await focusInline()
  const editor=container.querySelector<HTMLElement>('[data-testid="upstream-editor"]')!;editor.tabIndex=0
  const focusAndSelect=()=>{editor.focus();state.observe?.(selected)}
  state.restore.mockImplementation(()=>{if(timing==='sync')focusAndSelect();else setTimeout(focusAndSelect,0);return {ok:true}})
  state.request.mockResolvedValue({ok:true,view:{...base,scope}})
  await click('Request suggestion')
  expect(state.restore).toHaveBeenCalledTimes(1)
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'capture',captureId:proof.captureId}))
  expect(submit).toHaveBeenCalledTimes(1);expect(state.observers.size).toBe(1)
})
it('serializes two submits before React commits working state and owns only one restore waiter',async()=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  state.request.mockResolvedValue({ok:true,view:{...base,scope}})
  const button=[...container.querySelectorAll('button')].find(button=>button.textContent==='Request suggestion')!
  await act(async()=>{button.click();button.click()})
  expect(state.restore).toHaveBeenCalledTimes(1);expect(state.observers.size).toBe(2)
  await act(async()=>state.observe?.(selected))
  expect(state.request.mock.calls.filter(([request])=>request.action==='capture')).toHaveLength(1)
  expect(submit).toHaveBeenCalledTimes(1);expect(state.observers.size).toBe(1)
})
it('handles an owner removed before the restore observer runs without throwing or dispatching',async()=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  await click('Request suggestion')
  await act(async()=>{useDocxEditorStore.setState({views:{}});expect(()=>state.observe?.(selected)).not.toThrow()})
  expect(state.observers.size).toBe(1);expect(state.capture!(challenge)).toBeNull()
  expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled()
})
it.each(['pointer','keyboard'])('cancels pending restoration on a new document %s event before its late ready snapshot',async kind=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  state.request.mockResolvedValue({ok:true,view:{...base,scope}})
  const editor=container.querySelector<HTMLElement>('[data-testid="upstream-editor"]')!
  await click('Request suggestion')
  await act(async()=>{
    editor.dispatchEvent(kind==='pointer'?new MouseEvent('pointerdown',{bubbles:true}):new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true}))
    state.observe?.(selected)
  })
  expect(state.observers.size).toBe(1);expect(state.capture!(challenge)).toBeNull()
  expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})
it.each(['scope','version','revision','owner'] as const)('keeps a same-batch %s retirement during restoration before a late ready snapshot',async field=>{
  const view={...base,scope}
  state.request.mockResolvedValue({ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected));await focusInline()
  await click('Request suggestion')
  await act(async()=>{
    const revoked=field==='scope'?base:field==='version'?{...view,version:1}:field==='revision'?{...view,revision:'c'.repeat(64)}:{...view,threadId:'another-thread'}
    useDocxEditorStore.setState({views:{[base.editorId]:revoked}})
    useDocxEditorStore.setState({views:{[base.editorId]:view}})
    state.observe?.(selected)
  })
  expect(state.observers.size).toBe(1);expect(state.capture!(challenge)).toBeNull()
  expect(state.request).not.toHaveBeenCalledWith(expect.objectContaining({action:'capture'}));expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
})
it('detaches a pending restore on unmount and ignores the already queued ready callback',async()=>{
  await render(base);await act(async()=>state.observe?.(selected));await focusInline()
  await click('Request suggestion');const queued=state.observe!
  expect(state.observers.size).toBe(2)
  await act(async()=>root.unmount())
  expect(state.observers.size).toBe(0)
  await act(async()=>queued(selected))
  expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
  root=createRoot(container)
})
it('retires a timed-out restore and cannot dispatch from its late ready snapshot',async()=>{
  vi.useFakeTimers()
  try {
    await render(base);await act(async()=>state.observe?.(selected));await focusInline()
    const button=[...container.querySelectorAll('button')].find(button=>button.textContent==='Request suggestion')!
    await act(async()=>button.click())
    expect(state.observers.size).toBe(2)
    await act(async()=>vi.advanceTimersByTimeAsync(5000))
    expect(state.observers.size).toBe(1);expect(container.querySelector('[data-floating-agent]')).toBeNull()
    await act(async()=>state.observe?.(selected))
    expect(state.request).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();expect(state.replace).not.toHaveBeenCalled()
  } finally {vi.useRealTimers()}
})

it('offers concise editing through the same captured Core scope and shows both review texts',async()=>{
  const proposalId='e'.repeat(48),view={...base,scope,proposals:[{proposalId,status:'proposed' as const,parts:[]}],reviews:[{proposalId,beforeText:'Original',afterText:'Shorter'}]}
  state.request.mockResolvedValue({ok:true,view})
  await render(view);await act(async()=>state.observe?.(selected));await click('精简')
  expect(submit).toHaveBeenCalledWith('精简选中内容，保留关键事实与原意。',[expect.objectContaining({kind:'docx',editable:true,scopeId:scope.scopeId})])
  expect([...container.querySelectorAll('dd')].map(value=>value.textContent)).toEqual(['Original','Shorter'])
  expect(container.querySelector('button[class*="accept"]')?.textContent).toBe('应用并保存')
  expect(state.replace).not.toHaveBeenCalled()
})

it.each(['unknown','forbidden','conflict'] as const)('keeps the applied draft after save %s and never repeats its replacement',async failure=>{
  const proposalId='e'.repeat(48),review={...base,scope,proposals:[{proposalId,status:'proposed' as const,parts:[]}],reviews:[{proposalId,beforeText:'Original',afterText:'Replacement'}]}
  const applied={...review,version:1,phase:'editing' as const,dirty:true}
  const unresolved={...applied,phase:failure==='unknown'?'unknown' as const:failure==='conflict'?'conflict' as const:'editing' as const}
  const committed={...base,version:1,revision:'c'.repeat(64),content:{...base.content,sha256:'c'.repeat(64),data:'UEsDBAc=',byteLength:5}}
  let saves=0
  state.request.mockImplementation(async input=>input.action==='decide'?{ok:true,view:{...review,phase:'applying',dirty:true},replacement:'Replacement'}:
    input.action==='applied'?{ok:true,view:applied}:input.action==='save'?++saves===1?{ok:false,error:failure,view:unresolved}:{ok:true,view:committed}:
    input.action==='status'?{ok:true,view:committed}:{ok:true,view:review})
  await render(review);await act(async()=>state.observe?.(selected));await click('应用并保存')
  await render(unresolved)
  expect(state.replace).toHaveBeenCalledTimes(1)
  expect(container.querySelector('[role="status"]')?.textContent).not.toBe('已保存')
  expect([...container.querySelectorAll('button')].find(b=>b.textContent==='应用并保存')?.disabled).toBe(true)
  if(failure==='unknown') {
    await click('查询结果')
    expect(saves).toBe(1)
    expect(state.request).toHaveBeenCalledWith({action:'status',editorId:base.editorId})
    await render(committed)
    expect(container.querySelector('[role="status"]')?.textContent).toBe('已保存')
  } else if(failure==='forbidden') {
    await click('保存');expect(saves).toBe(2)
  } else {
    await click('保存');expect(saves).toBe(1)
  }
  expect(state.replace).toHaveBeenCalledTimes(1)
})

const openExternal=vi.fn()
const fileActions:NonNullable<Parameters<typeof DocxEditorPanel>[0]['fileActions']>=beforeOpen=>createElement('button',{onClick:()=>beforeOpen(openExternal)},'Open external')
it.each(['取消','打开已保存版本'] as const)('requires an explicit dirty open choice and %s never writes',async choice=>{
  openExternal.mockClear()
  await render({...base,phase:'editing',dirty:true},true,fileActions);await click('Open external')
  expect(container.querySelector('[role="dialog"]')).toBeTruthy();expect(openExternal).not.toHaveBeenCalled()
  await click(choice)
  expect(openExternal).toHaveBeenCalledTimes(choice==='取消'?0:1)
  expect(state.request).not.toHaveBeenCalled();expect(state.export).not.toHaveBeenCalled()
})
it.each(['committed','unknown','conflict'] as const)('save and open waits for a confirmed %s receipt',async outcome=>{
  openExternal.mockClear()
  const dirty={...base,phase:'editing' as const,dirty:true}
  state.request.mockResolvedValue(outcome==='committed'?{ok:true,view:base}:{ok:false,error:outcome,view:{...dirty,phase:outcome}})
  await render(dirty,true,fileActions);await click('Open external');await click('保存并打开')
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'save'}))
  expect(openExternal).toHaveBeenCalledTimes(outcome==='committed'?1:0)
  if(outcome!=='committed')expect(container.querySelector('[role="dialog"] [role="alert"]')?.textContent).toBeTruthy()
})
it.each([true,false])('completes a deferred save and open after status confirmation only while intent is current=%s',async visible=>{
  openExternal.mockClear()
  const dirty={...base,phase:'editing' as const,dirty:true},pending={...dirty,phase:'unknown' as const}
  state.request.mockImplementation(async input=>input.action==='save'?{ok:false,error:'unknown',view:pending}:{ok:true,view:base})
  await render(dirty,true,fileActions);await click('Open external');await click('保存并打开')
  expect(openExternal).not.toHaveBeenCalled()
  await render(pending,true,fileActions)
  if(!visible)await render(pending,false,fileActions)
  if(visible)await click('查询结果')
  else {await render(pending,true,fileActions);await click('查询结果')}
  expect(openExternal).toHaveBeenCalledTimes(visible?1:0)
  expect(state.request.mock.calls.filter(([input])=>input.action==='save')).toHaveLength(1)
  expect(container.querySelector('[role="dialog"]')).toBeNull()
})
it('opens a clean document immediately and retires a deferred open after its owner is hidden',async()=>{
  openExternal.mockClear()
  await render(base,true,fileActions);await click('Open external')
  expect(openExternal).toHaveBeenCalledTimes(1);expect(container.querySelector('[role="dialog"]')).toBeNull()
  await render({...base,phase:'editing',dirty:true},true,fileActions);await click('Open external')
  await render({...base,phase:'editing',dirty:true},false,fileActions)
  expect(container.querySelector('[role="dialog"]')).toBeNull();expect(openExternal).toHaveBeenCalledTimes(1)
})
it('keeps SDK local undo separate from receipt-backed saved undo and reloads confirmed Core bytes',async()=>{
  const saved={...base,canUndo:true},restored={...base,version:1,revision:'c'.repeat(64),canUndo:false,content:{...base.content,sha256:'c'.repeat(64),data:'UEsDBAc=',byteLength:5}}
  state.request.mockResolvedValue({ok:true,view:restored})
  await render(saved)
  const source=(state.ready.mock.calls.at(-1)![0] as {document:File}).document
  expect(namedButton('撤销工作副本编辑')?.disabled).toBe(true)
  await click('撤销已保存修改')
  expect(state.request).toHaveBeenCalledWith({action:'undoChange',editorId:base.editorId,version:0})
  expect(state.execute).not.toHaveBeenCalled()
  const next=(state.ready.mock.calls.at(-1)![0] as {document:File}).document
  expect(next).not.toBe(source);expect([...new Uint8Array(await next.arrayBuffer())]).toEqual([80,75,3,4,7])
})
it('refreshes the retained SDK document after a confirmed saved undo finishes while hidden',async()=>{
  const saved={...base,canUndo:true},restored={...base,version:1,revision:'c'.repeat(64),canUndo:false,content:{...base.content,sha256:'c'.repeat(64),data:'UEsDBAc=',byteLength:5}}
  let finish!:(value:unknown)=>void
  state.request.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
  await render(saved);await click('撤销已保存修改')
  await render(saved,false)
  await act(async()=>finish({ok:true,view:restored}));await flush()
  await render(restored)
  const source=(state.ready.mock.calls.at(-1)![0] as {document:File}).document
  expect([...new Uint8Array(await source.arrayBuffer())]).toEqual([80,75,3,4,7])
  expect(state.request.mock.calls.filter(([input])=>input.action==='undoChange')).toHaveLength(1)
})
it('reloads a confirmed undo after status even when the pending view already carries the restored revision',async()=>{
  const saved={...base,canUndo:true},restored={...base,version:1,revision:'c'.repeat(64),canUndo:false,content:{...base.content,sha256:'c'.repeat(64),data:'UEsDBAc=',byteLength:5}}
  state.request.mockImplementation(async input=>input.action==='undoChange'?{ok:false,error:'unknown',view:{...restored,phase:'unknown'}}:{ok:true,view:restored})
  await render(saved);await click('撤销已保存修改')
  await render({...restored,phase:'unknown'});await click('查询结果')
  const source=(state.ready.mock.calls.at(-1)![0] as {document:File}).document
  expect([...new Uint8Array(await source.arrayBuffer())]).toEqual([80,75,3,4,7])
  expect(state.request.mock.calls.filter(([input])=>input.action==='undoChange')).toHaveLength(1)
})


it('uses one icon operation row and keeps destructive and saved undo actions in an identity-scoped menu',async()=>{
  await render({...base,canUndo:true})
  const header=container.querySelector('[data-workspace-viewer-header]')!
  expect(container.querySelectorAll('[data-workspace-viewer-header]')).toHaveLength(1)
  expect(header.textContent).toContain('report.docx')
  for(const button of header.querySelectorAll<HTMLButtonElement>('button')){
    expect(button.getAttribute('aria-label')).toBeTruthy()
    expect(button.querySelector('svg')).not.toBeNull()
    expect(button.textContent).toBe('')
  }
  expect(namedButton('保存')!.disabled).toBe(true)
  expect(namedButton('放弃工作副本')).toBeUndefined()
  await click('更多 Word 操作')
  expect(namedButton('放弃工作副本')).toBeDefined()
  expect(namedButton('撤销已保存修改')!.disabled).toBe(false)
  expect(state.request).not.toHaveBeenCalled()
  await render({...base,editorId:'00000000-0000-4000-8000-000000000002',path:'/workspace/other.docx'})
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(state.request).not.toHaveBeenCalled()
})

it('does not announce a saved receipt while editing a clean working copy or after a failed save',async()=>{
  const editing={...base,phase:'editing' as const}
  await render(editing)
  expect(container.querySelector('[data-workspace-viewer-header]')?.textContent).toContain('工作副本编辑中')
  state.request.mockResolvedValue({ok:false,error:'forbidden',view:editing})
  await click('保存')
  const header=container.querySelector('[data-workspace-viewer-header]')!
  expect(state.request).toHaveBeenCalledWith(expect.objectContaining({action:'save',editorId:base.editorId}))
  expect(header.querySelector('[role="alert"]')?.textContent).toBeTruthy()
  expect(header.textContent).not.toContain('已保存')
})
