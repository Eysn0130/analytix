// @vitest-environment jsdom
import { useDocxEditorStore,docxOpenKey } from '../../office/docx-editor-store'
import { useChatStore } from '../../store/chat-store'
import { act, createElement, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DocumentWorkspacePanel } from './DocumentWorkspacePanel'
import type { WorkspaceViewerControls } from './WorkspaceViewerHeader'
import { useNativeOfficeStore } from '../../office/native-office-store'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import { useWorkspaceTabsStore, type WorkspaceTab } from '../../store/workspace-tabs-store'
import type { NativeOfficeRequest, NativeOfficeResponse } from '@shared/native-office'
import type { WorkspaceFileSaveAsResult } from '@shared/workspace-file'
import type { DocxEditorView } from '@shared/docx-editor'
const leaves=vi.hoisted(()=>({docx:vi.fn(),native:vi.fn(),preview:vi.fn(),review:vi.fn(),beginReview:vi.fn(()=>true),beforeOpen:vi.fn((open:()=>void)=>open()),exportCurrent:vi.fn()}))
vi.mock('react-i18next',async importOriginal=>({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation:()=>({t:(key:string)=>key})
}))
vi.mock('../../office/DocxEditorPanel',()=>({DocxEditorPanel:(props:{visible:boolean;viewerControls?:WorkspaceViewerControls;fileActions?:(beforeOpen:(open:()=>void)=>void,exportCurrent:()=>void)=>ReactNode})=>{
  leaves.docx(props);return createElement('div',{'data-testid':'docx-panel','data-visible':String(props.visible)},props.fileActions?.(leaves.beforeOpen,leaves.exportCurrent))
}}))
vi.mock('../../office/NativeOfficePanel',()=>({NativeOfficePanel:(props:{fileActions:ReactNode;titleControl?:ReactNode;visible:boolean;viewerControls?:WorkspaceViewerControls;onPrepareDraft?:(value:string,references:never[])=>boolean})=>{
  leaves.native(props);return createElement('div',{'data-testid':'native-panel','data-visible':String(props.visible),role:'toolbar'},createElement('div',{'data-testid':'native-file-title'},props.titleControl),createElement('div',{'data-testid':'native-file-actions'},props.fileActions),createElement('button',{'data-testid':'request-modify',onClick:()=>props.onPrepareDraft?.('Suggest a change',[])},'Request Modify'))
}}))
vi.mock('../WorkspaceFilePreviewPanel',()=>({WorkspaceFilePreviewPanel:(props:unknown)=>{leaves.preview(props);return createElement('div',{'data-testid':'file-preview'})}}))
vi.mock('../write/WriteConflictReview',()=>({WriteConflictReview:()=>createElement('div',{'data-testid':'conflict-recovery'})}))
vi.mock('../write/WriteMarkdownEditor',()=>({WriteMarkdownEditor:(props:{handleRef?:{current:unknown};value:string})=>{leaves.review(props);if(props.handleRef)props.handleRef.current={beginDiffReview:leaves.beginReview};return createElement('div',{'data-testid':'review-editor'},props.value)}}))
let root:Root,container:HTMLDivElement
const onSubmitPrompt=vi.fn(),onPrepareDraft=vi.fn(()=>true),openEditorPath=vi.fn(async()=>({ok:true}))
const saveAs=vi.fn(async(_input:unknown):Promise<WorkspaceFileSaveAsResult>=>({ok:true,path:'/synthetic/saved.xlsx'}))
const path='/synthetic/report.xlsx'
class FixtureResizeObserver {
  observe():void {}
  unobserve():void {}
  disconnect():void {}
}
beforeEach(()=>{
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('ResizeObserver',FixtureResizeObserver);vi.clearAllMocks()
  useChatStore.setState({route:'chat',activeThreadId:'current-main'})
  useDocxEditorStore.setState(useDocxEditorStore.getInitialState(),true)
  saveAs.mockResolvedValue({ok:true,path:'/synthetic/saved.xlsx'});leaves.beginReview.mockReturnValue(true);leaves.beforeOpen.mockImplementation(open=>open())
  Object.defineProperty(window,'analytix',{configurable:true,value:{workspace:{openEditorPath},files:{saveAs},office:{request:vi.fn(async()=>({ok:true,view:null}))}}})
  useNativeOfficeStore.setState({...useNativeOfficeStore.getInitialState(),target:{workspace:'/synthetic',path}},true)
  useWriteWorkspaceStore.setState({...useWriteWorkspaceStore.getInitialState(),workspaceRoot:'/synthetic',activeFilePath:null,activeFileKind:null,saveStatus:'saved',pendingSave:null,reviewActive:false,pendingAgentReview:null,reviewRecovery:null},true)
  useWorkspaceTabsStore.setState({tabs:[],activeTabId:null,selectorOpen:false,open:true})
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
})
afterEach(async()=>{await act(async()=>root.unmount());container.remove();Reflect.deleteProperty(window,'analytix');vi.unstubAllGlobals();vi.restoreAllMocks()})
async function render(activeTab?:WorkspaceTab,visible=true,viewerControls?:WorkspaceViewerControls){await act(async()=>root.render(createElement(DocumentWorkspacePanel,{threadId:'current-main',activeTab,visible,onSubmitPrompt,onPrepareDraft,onFocusConversation:vi.fn(),viewerControls,fileBrowser:createElement('div',{'data-testid':'file-browser'},'Files')})))}
async function select(action:'system'|'file-manager'|'browse'|'download'){
  const labels={system:'用默认应用打开','file-manager':'在文件夹中显示',browse:'浏览工作区文件',download:'generatedFileDownload'}
  const trigger=container.querySelector<HTMLButtonElement>('button[aria-label="打开"]')!
  expect(trigger).toBeTruthy()
  await act(async()=>trigger.click())
  const item=[...document.querySelectorAll<HTMLButtonElement>('[role="menu"] button[role="menuitem"]')].find(button=>button.textContent===labels[action])!
  expect(item).toBeTruthy();expect(item.disabled).toBe(false)
  await act(async()=>item.click())
  expect(document.querySelector('[role="menu"]')).toBeNull()
}
function button(label:string){return [...container.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent===label)!}
const tab=(name:string):WorkspaceTab=>({id:'retained:'+name,kind:'document',mode:'documents',title:name,path:'/synthetic/'+name,workspaceRoot:'/synthetic'})
const wordView=(name='report.docx'):DocxEditorView=>({editorId:'00000000-0000-4000-8000-000000000001',workspace:'/synthetic',path:'/synthetic/'+name,threadId:'current-main',objectId:'a'.repeat(64),revision:'b'.repeat(64),version:0,phase:'ready',dirty:false,
  content:{encoding:'base64',kind:'docx',byteLength:4,sha256:'b'.repeat(64),data:'UEsDBA=='},proposals:[],reviews:[]})

it('keeps one native toolbar, actual file menu actions, files and Escape focus',async()=>{
  await render();expect(container.querySelector('.document-workspace-header')).toBeNull();expect(container.querySelectorAll('[role="toolbar"]')).toHaveLength(1)
  await select('system');expect(openEditorPath).toHaveBeenLastCalledWith({workspaceRoot:'/synthetic',path,editorId:'system'})
  await select('file-manager');expect(openEditorPath).toHaveBeenLastCalledWith({workspaceRoot:'/synthetic',path,editorId:'file-manager'})
  await select('browse');expect(container.querySelector('[data-testid="file-browser"]')).not.toBeNull()
  await act(async()=>container.querySelector('section')!.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true})))
  expect(container.querySelector('[data-testid="file-browser"]')).toBeNull();expect(document.activeElement).toBe(container.querySelector('[aria-label="打开"]'))
})
it('downloads through the retained native action and passes Request Modify only to draft preparation',async()=>{
  await render();await select('download');expect(saveAs).toHaveBeenCalledExactlyOnceWith({sourcePath:path,workspaceRoot:'/synthetic'})
  expect(container.querySelector('[data-file-download-status]')?.textContent).toBe('generatedFileSaved')
  await act(async()=>container.querySelector<HTMLButtonElement>('[data-testid="request-modify"]')!.click())
  expect(onPrepareDraft).toHaveBeenCalledExactlyOnceWith('Suggest a change',[]);expect(onSubmitPrompt).not.toHaveBeenCalled()
})
it('rejects a late native selection after A to B without clearing the retained dirty B view',async()=>{
  const first=tab('first.xlsx'),second=tab('second.xlsx')
  const retained={objectId:'a'.repeat(64),path:second.path!,kind:'xlsx' as const,revision:'b'.repeat(64),dirty:true,status:'ready' as const}
  let finish!:(value:NativeOfficeResponse)=>void
  const request=vi.fn(async(_input:NativeOfficeRequest):Promise<NativeOfficeResponse>=>({ok:true,view:null}));request.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  Object.assign(window.analytix,{office:{request}})
  useNativeOfficeStore.setState({target:null,view:null,views:{[JSON.stringify(['/synthetic',second.path])]:retained}})
  await render(first);await render(second)
  const owner=useNativeOfficeStore.getState().target
  expect(owner).toEqual({workspace:'/synthetic',path:second.path});expect(useNativeOfficeStore.getState().view).toBe(retained)
  await act(async()=>finish({ok:true,view:null}))
  expect(useNativeOfficeStore.getState().target).toBe(owner);expect(useNativeOfficeStore.getState().view).toBe(retained)
  expect(container.querySelector('[data-testid="native-panel"]')?.getAttribute('data-visible')).toBe('true')
})
it.each(['hide','unmount'] as const)('drops a late native selection after %s',async transition=>{
  let finish!:(value:NativeOfficeResponse)=>void
  Object.assign(window.analytix,{office:{request:vi.fn(()=>new Promise<NativeOfficeResponse>(resolve=>{finish=resolve}))}})
  useNativeOfficeStore.setState({target:null,view:null});await render(tab('first.xlsx'))
  if(transition==='hide')await render(tab('first.xlsx'),false)
  else {await act(async()=>root.unmount());root=createRoot(container)}
  await act(async()=>finish({ok:true,view:null}));expect(useNativeOfficeStore.getState().target).toBeNull()
})
it('falls back an old path tab to file preview without opening, saving, recreating or changing its identity',async()=>{
  const old=tab('notes.md'),open=vi.spyOn(useWriteWorkspaceStore.getState(),'openFile'),initialize=vi.spyOn(useWriteWorkspaceStore.getState(),'initializeWorkspace'),flush=vi.spyOn(useWriteWorkspaceStore.getState(),'flushSave')
  useWorkspaceTabsStore.getState().openTab(old);await render(old)
  expect(container.querySelector('[data-testid="file-preview"]')).not.toBeNull();expect(container.querySelector('[data-testid="review-editor"]')).toBeNull()
  expect(leaves.preview.mock.calls.at(-1)?.[0]).toMatchObject({target:{path:old.path,workspaceRoot:old.workspaceRoot}})
  expect(useWorkspaceTabsStore.getState().tabs).toEqual([old]);expect(open).not.toHaveBeenCalled();expect(initialize).not.toHaveBeenCalled();expect(flush).not.toHaveBeenCalled()
})
it('returns a no-path legacy entry to the selector without creating a file or consuming another native target',async()=>{
  const old:WorkspaceTab={id:'tool:documents',kind:'tool',mode:'documents',title:'Legacy'}
  useWorkspaceTabsStore.getState().openTab(old);const create=vi.spyOn(useNativeOfficeStore.getState(),'createDocument')
  await render(old);expect(useWorkspaceTabsStore.getState()).toMatchObject({selectorOpen:true,tabs:[old]})
  expect(container.querySelector('[data-testid="native-panel"], [data-testid="file-preview"]')).toBeNull();expect(create).not.toHaveBeenCalled()
})
it('keeps a dirty original owner intact until an explicit successful save, then shows file preview',async()=>{
  const old=tab('notes.md'),flush=vi.fn(async()=>false),discard=vi.fn(async()=>false)
  useWriteWorkspaceStore.setState({activeFilePath:old.path!,activeFileKind:'text',fileContent:'Retained draft',saveStatus:'dirty',flushSave:flush,syncActiveFileFromDisk:discard})
  await render(old);expect(container.querySelector('[data-legacy-document-recovery]')).not.toBeNull();expect(flush).not.toHaveBeenCalled();expect(discard).not.toHaveBeenCalled()
  await act(async()=>button('save').click());expect(flush).toHaveBeenCalledExactlyOnceWith('/synthetic')
  expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Retained draft');expect(container.querySelector('[data-testid="file-preview"]')).toBeNull()
  flush.mockImplementationOnce(async()=>{useWriteWorkspaceStore.setState({saveStatus:'saved'});return true})
  await act(async()=>button('save').click());expect(container.querySelector('[data-testid="file-preview"]')).not.toBeNull()
})
it('queries a pending save through its original owner and forbids discard while its result is unknown',async()=>{
  const old=tab('notes.md'),pending={operationId:'original_op_001',objectId:'a'.repeat(64),baseRevision:'b'.repeat(64),content:'Frozen candidate'}
  const flush=vi.fn(async()=>false),discard=vi.fn(async()=>false)
  useWriteWorkspaceStore.setState({activeFilePath:old.path!,activeFileKind:'text',fileContent:pending.content,saveStatus:'unknown',pendingSave:pending,flushSave:flush,syncActiveFileFromDisk:discard})
  await render(old);expect(button('workspaceLegacyDraftDiscard').disabled).toBe(true)
  await act(async()=>button('workspaceLegacyDraftQuery').click());expect(flush).toHaveBeenCalledExactlyOnceWith('/synthetic')
  expect(useWriteWorkspaceStore.getState().pendingSave).toBe(pending);expect(discard).not.toHaveBeenCalled()
})
it('discards only the captured original draft after explicit confirmation and a successful reread',async()=>{
  const old=tab('notes.md'),discard=vi.fn(async()=>false)
  useWriteWorkspaceStore.setState({activeFilePath:old.path!,activeFileKind:'text',fileContent:'Captured draft',saveStatus:'dirty',syncActiveFileFromDisk:discard})
  await render(old);await act(async()=>button('workspaceLegacyDraftDiscard').click())
  expect(discard).toHaveBeenCalledExactlyOnceWith('/synthetic',{path:old.path,force:true,animate:false,expectedContent:'Captured draft'})
  expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Captured draft')
  discard.mockImplementationOnce(async()=>{useWriteWorkspaceStore.setState({fileContent:'Core current bytes',saveStatus:'saved'});return true})
  await act(async()=>button('workspaceLegacyDraftDiscard').click());expect(container.querySelector('[data-testid="file-preview"]')).not.toBeNull()
})
it('does not expose another tab’s retained draft and rejects an old owner action after switching tabs',async()=>{
  const old=tab('notes.md'),other=tab('other.md'),flush=vi.fn(async()=>false)
  useWriteWorkspaceStore.setState({activeFilePath:old.path!,activeFileKind:'text',fileContent:'Original private draft',saveStatus:'dirty',flushSave:flush})
  await render(old);const stale=button('save');await render(other)
  await act(async()=>stale.click());expect(flush).not.toHaveBeenCalled();expect(container.textContent).not.toContain('Original private draft')
  expect(useWriteWorkspaceStore.getState().fileContent).toBe('Original private draft')
})
it('restores only an already pending review through the shared Markdown editor without enabling completions or autosave',async()=>{
  const old=tab('notes.md'),flush=vi.fn(async()=>false)
  useWriteWorkspaceStore.setState({activeFilePath:old.path!,activeFileKind:'text',fileContent:'Original draft',saveStatus:'dirty',reviewActive:true,pendingAgentReview:{nextContent:'Proposed review'},flushSave:flush})
  await render(old);expect(leaves.beginReview).toHaveBeenCalledExactlyOnceWith({original:'Original draft',nextDoc:'Proposed review'})
  expect(leaves.review.mock.calls.at(-1)?.[0]).toMatchObject({completionEnabled:false,completionLongEnabled:false,workspaceRoot:'/synthetic',filePath:old.path})
  expect(flush).not.toHaveBeenCalled();expect(container.querySelector('[data-testid="file-preview"]')).toBeNull()
})


it('does not turn a source-marked legacy Office tab into an ordinary native open',async()=>{
  const old={...tab('report.xlsx'),preview:'source' as const},selected=useNativeOfficeStore.getState().target
  const select=vi.spyOn(useNativeOfficeStore.getState(),'select')
  await render(old)
  expect(select).not.toHaveBeenCalled();expect(useNativeOfficeStore.getState().target).toBe(selected)
  expect(container.querySelector('[data-testid="native-panel"]')).toBeNull()
  expect(leaves.preview.mock.calls.at(-1)?.[0]).toMatchObject({target:{path:old.path,preview:'source'}})
})

it('routes ordinary DOCX through its dedicated owner without native selection or sourcePath export',async()=>{
  const selectDocx=vi.spyOn(useDocxEditorStore.getState(),'select').mockResolvedValue(true)
  const selectNative=vi.spyOn(useNativeOfficeStore.getState(),'select')
  await render(tab('report.docx'))
  expect(selectDocx).toHaveBeenCalledWith('/synthetic','/synthetic/report.docx',expect.anything(),expect.any(Function),expect.any(Function))
  expect(selectNative).not.toHaveBeenCalled()
  expect(container.querySelector('[data-testid="native-panel"]')).toBeNull()
  expect(saveAs).not.toHaveBeenCalled()
  selectDocx.mockRestore();selectNative.mockRestore()
})

it('shares Word default-app, folder and file browser actions while exporting its current working copy',async()=>{
  vi.spyOn(useDocxEditorStore.getState(),'select').mockResolvedValue(true)
  const view=wordView();useDocxEditorStore.setState({views:{[view.editorId]:view}})
  await render(tab('report.docx'))
  expect(container.querySelectorAll('[aria-label="打开"]')).toHaveLength(1)
  await select('system');expect(openEditorPath).toHaveBeenLastCalledWith({workspaceRoot:view.workspace,path:view.path,editorId:'system'})
  await select('file-manager');expect(openEditorPath).toHaveBeenLastCalledWith({workspaceRoot:view.workspace,path:view.path,editorId:'file-manager'})
  expect(leaves.beforeOpen).toHaveBeenCalledTimes(2)
  await select('download');await act(async()=>container.querySelector<HTMLButtonElement>('[data-native-office-download]')!.click())
  expect(leaves.exportCurrent).toHaveBeenCalledTimes(2);expect(saveAs).not.toHaveBeenCalled()
  await select('browse');expect(container.querySelector('[data-testid="file-browser"]')).not.toBeNull()
  await act(async()=>container.querySelector('section')!.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true})))
  expect(container.querySelector('[data-testid="file-browser"]')).toBeNull();expect(document.activeElement).toBe(container.querySelector('[aria-label="打开"]'))
  await render(tab('report.xlsx'))
  expect(container.querySelectorAll('[aria-label="打开"]')).toHaveLength(1)
  expect(container.querySelector('[data-testid="docx-panel"]')?.getAttribute('data-visible')).toBe('false')
})

it('defers the Word opener to its guard and rejects its captured action after switching files',async()=>{
  vi.spyOn(useDocxEditorStore.getState(),'select').mockResolvedValue(true)
  const view=wordView();useDocxEditorStore.setState({views:{[view.editorId]:view}})
  let deferred:(()=>void)|undefined
  leaves.beforeOpen.mockImplementation(open=>{deferred=open})
  await render(tab('report.docx'));await select('system')
  expect(deferred).toBeTypeOf('function');expect(openEditorPath).not.toHaveBeenCalled()
  await render(tab('report.xlsx'));await act(async()=>deferred?.())
  expect(openEditorPath).not.toHaveBeenCalled()
})

it('local Word shows loading, a target-specific error and retry without a thread or text preview',async()=>{
 useChatStore.setState({route:'chat',activeThreadId:null,workspaceRoot:'/synthetic'})
 useDocxEditorStore.setState(useDocxEditorStore.getInitialState(),true)
 const owner=tab('local-entry.docx');useWorkspaceTabsStore.getState().openTab(owner)
 let finish!:(value:boolean)=>void
 const select=vi.spyOn(useDocxEditorStore.getState(),'select').mockImplementation(()=>new Promise<boolean>(resolve=>{finish=resolve}))
 await render(owner)
 expect(select).toHaveBeenCalledWith('/synthetic',owner.path,undefined,expect.any(Function),expect.any(Function))
 expect(container.querySelector('[data-docx-opening]')).not.toBeNull();expect(container.querySelector('[data-testid="file-preview"]')).toBeNull()
 useDocxEditorStore.setState({openErrors:{[docxOpenKey('/synthetic',owner.path!)]: 'forbidden'}})
 await act(async()=>finish(false))
 expect(container.querySelector('[data-docx-open-error]')?.textContent).toContain('当前权限不允许')
 expect(container.querySelector('[data-testid="file-preview"]')).toBeNull()
 await act(async()=>button('重试').click());expect(select).toHaveBeenCalledTimes(2)
 await act(async()=>finish(false));select.mockRestore()
})

it('local Word invalidates an old pending open and starts the externally selected thread',async()=>{
 useChatStore.setState({route:'chat',activeThreadId:'thread-a',workspaceRoot:'/synthetic'})
 const owner=tab('local-switch.docx');useWorkspaceTabsStore.getState().openTab(owner)
 const finishes:Array<(value:boolean)=>void>=[],checks:Array<()=>boolean>=[]
 const select=vi.spyOn(useDocxEditorStore.getState(),'select').mockImplementation((_root,_path,_thread,current)=>{
  checks.push(current!);return new Promise<boolean>(resolve=>finishes.push(resolve))
 })
 await render(owner)
 await act(async()=>{useChatStore.setState({activeThreadId:'thread-b'});root.render(createElement(DocumentWorkspacePanel,{threadId:'thread-b',activeTab:owner,visible:true,onSubmitPrompt,onPrepareDraft,onFocusConversation:vi.fn(),fileBrowser:null}))})
 expect(checks[0]()).toBe(false);expect(select).toHaveBeenCalledTimes(2)
 expect(select.mock.calls[1][2]).toBe('thread-b')
 await act(async()=>finishes[0](false));expect(container.querySelector('[data-docx-open-error]')).toBeNull()
 await act(async()=>finishes[1](true));expect(container.querySelector('[data-docx-opening]')).toBeNull();select.mockRestore()
})


it.each(['docx','xlsx','md'] as const)('passes the workspace viewer controls unchanged to the %s leaf',async extension=>{
  const controls:WorkspaceViewerControls={focused:true,onToggleFocus:vi.fn(),onCollapse:vi.fn()}
  if(extension==='docx'){
    vi.spyOn(useDocxEditorStore.getState(),'select').mockResolvedValue(true)
    const view=wordView();useDocxEditorStore.setState({views:{[view.editorId]:view}})
  }
  await render(tab(`report.${extension}`),true,controls)
  const leaf=extension==='docx'?leaves.docx:extension==='xlsx'?leaves.native:leaves.preview
  const received=leaf.mock.calls.at(-1)?.[0].viewerControls as WorkspaceViewerControls
  expect(received).toBe(controls)
  expect(received.focused).toBe(true)
  await act(async()=>{received.onToggleFocus();received.onCollapse()})
  expect(controls.onToggleFocus).toHaveBeenCalledOnce();expect(controls.onCollapse).toHaveBeenCalledOnce()
  expect(onSubmitPrompt).not.toHaveBeenCalled();expect(onPrepareDraft).not.toHaveBeenCalled();expect(saveAs).not.toHaveBeenCalled()
})


it('keeps the Native file title separate from right-side download and its receipt',async()=>{
  await render()
  const title=container.querySelector('[data-testid="native-file-title"]')!
  const actions=container.querySelector('[data-testid="native-file-actions"]')!
  expect(title.querySelector('button[aria-label="打开"]')).not.toBeNull()
  expect(title.textContent).toBe('report.xlsx')
  expect(title.querySelector('[data-native-office-download]')).toBeNull()
  expect(actions.querySelector('[data-native-office-download]')).not.toBeNull()
  expect(actions.querySelector('button[aria-label="打开"]')).toBeNull()
  await act(async()=>actions.querySelector<HTMLButtonElement>('[data-native-office-download]')!.click())
  expect(saveAs).toHaveBeenCalledExactlyOnceWith({sourcePath:path,workspaceRoot:'/synthetic'})
  expect(actions.querySelector('[data-file-download-status]')?.textContent).toBe('generatedFileSaved')
  expect(title.querySelector('[data-file-download-status]')).toBeNull()
  expect(container.querySelectorAll('[role="toolbar"]')).toHaveLength(1)
})
