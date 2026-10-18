// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { WriteSidebar } from './WriteSidebar'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
const leaves=vi.hoisted(()=>({tree:vi.fn()}))
vi.mock('react-i18next',async importOriginal=>({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation:()=>({t:(key:string)=>key})
}))
vi.mock('./WriteFileTree',()=>({WriteFileTree:(props:{rootDirectory:string;onCreateFile:(root:string)=>void;onSelectFile:(path:string)=>void;onDeleteEntry:(entry:unknown)=>void;onRenameEntry:(entry:unknown)=>void})=>{
  leaves.tree(props);return createElement('div',{'data-browser-root':props.rootDirectory},
    createElement('button',{onClick:()=>props.onCreateFile(props.rootDirectory)},'Generic new file'),
    createElement('button',{onClick:()=>props.onSelectFile(props.rootDirectory+'/notes.md')},'Open Markdown'),
    createElement('button',{onClick:()=>props.onDeleteEntry({name:'draft.md',path:'/original/draft.md',ext:'.md',type:'file'})},'Delete original'),
    createElement('button',{onClick:()=>props.onRenameEntry({name:'original',path:'/original',ext:'',type:'directory'})},'Rename original parent'),
    createElement('button',{onClick:()=>props.onDeleteEntry({name:'draft.md',path:'/original/sub/draft.md',ext:'.md',type:'file'})},'Delete nested original'))
}}))
let root:Root,host:HTMLDivElement
const open=vi.fn(),load=vi.fn(async(owner:string)=>owner),create=vi.fn(async()=>'/browse/untitled.md')
const pick=vi.fn(async()=>({canceled:false,path:'/other'})),initialize=vi.fn(),settings=vi.fn(),add=vi.fn(),flush=vi.fn()
function currentDraft(){return {workspaceRoot:useWriteWorkspaceStore.getState().workspaceRoot,activeFilePath:useWriteWorkspaceStore.getState().activeFilePath,fileContent:useWriteWorkspaceStore.getState().fileContent,pendingSave:useWriteWorkspaceStore.getState().pendingSave}}
beforeEach(()=>{
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.clearAllMocks();create.mockResolvedValue('/browse/untitled.md');pick.mockResolvedValue({canceled:false,path:'/other'})
  useWriteWorkspaceStore.setState({...useWriteWorkspaceStore.getInitialState(),workspaceRoot:'/original',activeFilePath:'/original/draft.md',activeFileKind:'text',fileContent:'Retained original draft',saveStatus:'dirty',loadDirectory:load,createFile:create,initializeWorkspace:initialize,loadWriteSettings:settings,addWriteWorkspace:add,flushSave:flush},true)
  Object.defineProperty(window,'analytix',{configurable:true,value:{workspace:{pickDirectory:pick}}})
  host=document.createElement('div');document.body.append(host);root=createRoot(host)
})
afterEach(async()=>{await act(async()=>root.unmount());host.remove();Reflect.deleteProperty(window,'analytix');vi.restoreAllMocks();vi.unstubAllGlobals()})
async function render(path='/browse'){await act(async()=>root.render(createElement<NonNullable<Parameters<typeof WriteSidebar>[0]>>(WriteSidebar,{initialWorkspaceRoot:path,onOpenFile:open})))}
function button(label:string){return [...host.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.includes(label))!}
async function submit(){await act(async()=>document.body.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})))}
it('browses the requested files without initializing the old adapter, loading its catalog or saving a retained draft',async()=>{
  const original=currentDraft();await render()
  expect(leaves.tree.mock.calls.at(-1)?.[0]).toMatchObject({rootDirectory:'/browse'})
  expect(load).toHaveBeenCalledWith('/browse','/browse');expect(currentDraft()).toEqual(original)
  expect(initialize).not.toHaveBeenCalled();expect(settings).not.toHaveBeenCalled();expect(flush).not.toHaveBeenCalled()
  await act(async()=>button('Open Markdown').click());expect(open).toHaveBeenCalledExactlyOnceWith('/browse','/browse/notes.md')
})
it('keeps Add workspace as a temporary file browser root without changing writing settings or the original owner',async()=>{
  const original=currentDraft();await render();await act(async()=>button('writeAddWorkspace').click())
  expect(pick).toHaveBeenCalledExactlyOnceWith('/browse');expect(leaves.tree.mock.calls.at(-1)?.[0]).toMatchObject({rootDirectory:'/other'})
  expect(currentDraft()).toEqual(original);expect(add).not.toHaveBeenCalled();expect(settings).not.toHaveBeenCalled();expect(initialize).not.toHaveBeenCalled();expect(flush).not.toHaveBeenCalled()
})
it('allows a generic untitled.md file through the existing guarded action and opens only file preview',async()=>{
  const original=currentDraft();await render();await act(async()=>button('Generic new file').click())
  expect(document.body.querySelector<HTMLInputElement>('form input')?.value).toBe('untitled.md')
  await submit();expect(create).toHaveBeenCalledExactlyOnceWith('/browse','/browse/untitled.md','',{openAfterCreate:false})
  expect(open).toHaveBeenCalledExactlyOnceWith('/browse','/browse/untitled.md');expect(currentDraft()).toEqual(original);expect(flush).not.toHaveBeenCalled()
})
it('retains an old pending operation while generic browsing changes, and drops a late create navigation after root replacement',async()=>{
  const pending={operationId:'retained_op_001',objectId:'a'.repeat(64),baseRevision:'b'.repeat(64),content:'Frozen original'}
  useWriteWorkspaceStore.setState({pendingSave:pending,saveStatus:'unknown'})
  let finish!:(path:string)=>void;create.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
  await render();await act(async()=>button('Generic new file').click());await submit();await render('/replacement')
  await act(async()=>finish('/browse/untitled.md'))
  expect(open).not.toHaveBeenCalled();expect(useWriteWorkspaceStore.getState().pendingSave).toBe(pending)
  expect(useWriteWorkspaceStore.getState().fileContent).toBe('Retained original draft')
})


it.each(['dirty','unknown'] as const)('requires the retained %s owner to settle before deleting it or renaming its parent',async status=>{
  const remove=vi.spyOn(useWriteWorkspaceStore.getState(),'deleteEntry'),rename=vi.spyOn(useWriteWorkspaceStore.getState(),'renameEntry')
  useWriteWorkspaceStore.setState({saveStatus:status,pendingSave:status==='unknown'?{operationId:'original_pending',objectId:'a'.repeat(64),baseRevision:'b'.repeat(64),content:'Retained original draft'}:null})
  const original=currentDraft();await render('/original');await act(async()=>button('Delete original').click());await submit()
  expect(remove).not.toHaveBeenCalled();expect(currentDraft()).toEqual(original)
  await act(async()=>button('Rename original parent').click());await submit()
  expect(rename).not.toHaveBeenCalled();expect(currentDraft()).toEqual(original)
  expect(host.textContent).toContain('workspaceLegacyDraftDescription')
})


it('protects a pending original file when the temporary browse root is an ancestor of its workspace',async()=>{
  const remove=vi.spyOn(useWriteWorkspaceStore.getState(),'deleteEntry')
  useWriteWorkspaceStore.setState({workspaceRoot:'/original/sub',activeFilePath:'/original/sub/draft.md',saveStatus:'unknown',pendingSave:{operationId:'overlap_pending',objectId:'a'.repeat(64),baseRevision:'b'.repeat(64),content:'Retained original draft'}})
  const original=currentDraft();await render('/original');await act(async()=>button('Delete nested original').click());await submit()
  expect(remove).not.toHaveBeenCalled();expect(currentDraft()).toEqual(original)
  expect(host.textContent).toContain('workspaceLegacyDraftDescription')
})
