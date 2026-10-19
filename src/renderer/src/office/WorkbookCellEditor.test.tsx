// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeOfficeRequest, NativeOfficeResponse, NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import { WorkbookCellEditor, workbookAutoInputType, workbookDraftEdit, workbookSelectionSum, type WorkbookCellContextActions } from './WorkbookCellEditor'
import { nativeOfficeTargetKey, nativeWorkbookDraftKey, useNativeOfficeStore, type NativeWorkbookCellDraft } from './native-office-store'
import { createWriteShutdownHandler, isWriteShutdownFrozen } from '../write/write-shutdown'

const {beginShutdown,releaseShutdown}=vi.hoisted(()=>({beginShutdown:vi.fn(),releaseShutdown:vi.fn()}))
vi.mock('../write/write-workspace-store',()=>({useWriteWorkspaceStore:{getState:()=>({beginShutdown})}}))

type Cells = Extract<NativeOfficeSelection,{kind:'cells'}>
const editableCell:NativeWorkbookCellDraft['cell']={sheet:0,column:0,row:0,text:'12',value:12,formula:'12',valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}
const selection:Cells = {kind:'cells',documentId:'a'.repeat(64),version:'b'.repeat(64),changeSequence:0,token:'cell-selection-1',
  scope:'sheet-range-address-at-version-and-change-sequence',text:'12',capture:{capturedCharacters:2,totalCharacters:2,unit:'utf-16',complete:true,truncated:false},
  ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}],
  cells:[editableCell]}
const view:NativeOfficeView = {objectId:selection.documentId,path:'/synthetic/book.xlsx',kind:'xlsx',revision:selection.version,
  dirty:false,status:'ready',changeSequence:0,editing:true,workbookEditing:{editable:true},selection}
const workspace='/synthetic',threadId='thread',draftKey=nativeWorkbookDraftKey(workspace,view.path,threadId)
const targetKey=nativeOfficeTargetKey({workspace,path:view.path})
function receipt(type:'number'|'text'='text',value='001'):NativeOfficeView {
  return {...view,revision:'c'.repeat(64),selection:undefined,recovery:{pending:null,current:{version:3,origin:'manual',changeId:'d'.repeat(64),
    threadId:'thread',baseRevision:view.revision,revision:'c'.repeat(64),status:'committed',
    workbook:{before:{...selection.ranges[0],cells:[editableCell]},after:{...selection.ranges[0],cells:[{...editableCell,text:value,formula:value,valueType:type,value:type==='number'?Number(value):0}]},results:[value]},
    saveOperationId:'save-operation',undoOperationId:'undo-operation',canUndo:true,canCancel:false,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''}}}
}
let root:Root|null, container:HTMLDivElement, toolbarTarget:HTMLDivElement
let request:ReturnType<typeof vi.fn<(input:NativeOfficeRequest)=>Promise<NativeOfficeResponse | undefined>>>
async function render(next:NativeOfficeView = view,active = true,context:{workspace?:string;path?:string;threadId?:string;toolbar?:boolean|null;onContextActionsChange?:(value:WorkbookCellContextActions|null)=>void}={}) {
  await act(async()=>root!.render(createElement(WorkbookCellEditor,{view:next,threadId:context.threadId??threadId,
    workspace:context.workspace??workspace,path:context.path??next.path,active,busy:false,onRequest:request,toolbarTarget:context.toolbar === null ? null : context.toolbar === false ? undefined : toolbarTarget,onContextActionsChange:context.onContextActionsChange})))
}
const input = () => container.querySelector<HTMLInputElement>('[aria-label="单元格值或公式"]')!
const button = (label:string) => [...document.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.getAttribute('aria-label')===label || item.textContent===label)!
async function openOptions() {
  if (button('单元格编辑选项').getAttribute('aria-expanded') !== 'true') await act(async()=>button('单元格编辑选项').click())
}
async function clickButton(label:string) {
  if (!button(label)) await openOptions()
  await act(async()=>button(label).click())
}
async function fill(value:string) {
  await act(async()=>{
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input(),value)
    input().dispatchEvent(new Event('input',{bubbles:true}))
  })
}
async function openTypes() {
  if (!document.querySelector('[role="menu"][aria-label="单元格输入类型"]')) await act(async()=>input().dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true})))
}
async function choose(type:string) {
  await openTypes()
  await act(async()=>document.querySelector<HTMLButtonElement>(`[data-workbook-input-mode="${type}"]`)!.click())
}
async function shutdown(handler:ReturnType<typeof createWriteShutdownHandler>,requestId:string,phase:'prepare'|'cancel') {
  let result!:Awaited<ReturnType<typeof handler>>
  await act(async()=>{result=await handler({requestId,phase})})
  return result
}
function otherWorkbook():NativeOfficeView {
  const objectId='e'.repeat(64)
  return {...view,objectId,path:'/synthetic/other.xlsx',selection:{...selection,documentId:objectId,token:'other-cell-selection',text:'20',
    cells:[{...selection.cells![0],text:'20',formula:'20',value:20}]}}
}
beforeEach(()=>{
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
  request=vi.fn(async()=>({ok:true,view}))
  beginShutdown.mockReset();releaseShutdown.mockReset()
  beginShutdown.mockImplementation(()=>({release:releaseShutdown,save:async()=>({result:'ready' as const})}))
  useNativeOfficeStore.setState({target:{workspace,path:view.path},view,error:null,engineViews:{[targetKey]:view},views:{[targetKey]:view},
    workbookDrafts:{},workbookClosing:{},workbookDraftInputFrozen:false,workbookDraftComposing:false,receiveSequence:0,
    proposalInFlight:false,annotationInputFrozen:false,manualDiscardAttempts:{},manualOwners:{},creationAttempt:null})
  container=document.createElement('div');toolbarTarget=document.createElement('div');toolbarTarget.dataset.testViewerActions='';document.body.append(toolbarTarget,container);root=createRoot(container)
})
afterEach(async()=>{
  await act(async()=>root?.unmount());root=null;container.remove();toolbarTarget.remove()
  useNativeOfficeStore.setState({target:null,view:null,error:null,engineViews:{},views:{},workbookDrafts:{},workbookClosing:{},
    workbookDraftInputFrozen:false,workbookDraftComposing:false})
  vi.unstubAllGlobals()
})

describe('typed workbook cell drafts',()=>{
  it('auto input preserves leading zeros, long identifiers, whitespace and numeric underflow as text',()=>{
    for (const value of ['001','-001','00.5','1234567890123456','0.1234567890123456',' 12 ','12 ',' ','1e999','1e-999','0x10','12 apples','2026-10-20'])
      expect(workbookAutoInputType(value),value).toBe('text')
    for (const value of ['0','-12.5','.5','1e3','0e-999']) expect(workbookAutoInputType(value),value).toBe('number')
    expect(workbookAutoInputType('')).toBe('empty')
    expect(workbookAutoInputType('=SUM(A1:A2)')).toBe('formula')
    expect(workbookAutoInputType('=')).toBe('formula')
    expect(workbookDraftEdit('formula','=')).toBeNull()
  })
  it.each(['empty','text','number','formula'] as const)('auto input chooses a number independently of the original %s cell type',async valueType=>{
    const cell={...editableCell,valueType,text:valueType === 'text' ? 'before' : valueType === 'empty' ? '' : '12',
      formula:valueType === 'formula' ? '=6*2' : valueType === 'empty' ? '' : '12',value:valueType === 'empty' || valueType === 'text' ? 0 : 12}
    await render({...view,selection:{...selection,cells:[cell]}});await fill('37')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'auto',type:'number',value:'37'})
    expect(request).not.toHaveBeenCalled()
    await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({action:'editWorkbookCell',edit:{type:'number',value:37}}))
  })
  it('auto input sends formula and text intent while keeping invalid formula input local',async()=>{
    await render();await fill('=')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'auto',type:'formula',value:'='})
    expect(button('保存').disabled).toBe(true);expect(request).not.toHaveBeenCalled()
    await fill('=SUM(A1:A2)')
    expect(button('保存').disabled).toBe(false)
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'auto',type:'formula'})
    await fill('hello');await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({edit:{type:'text',text:'hello'}}))
  })
  it('auto input clears a cell without disabling further input before an explicit save',async()=>{
    await render();await fill('')
    expect(input().disabled).toBe(false);expect(input().readOnly).toBe(false)
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'auto',type:'empty'})
    await fill('38')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'auto',type:'number',value:'38'})
    await fill('');await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({edit:{type:'empty'}}))
  })
  it('type context menu supports keyboard entry, Escape and focus-out without saving or stealing focus',async()=>{
    await render();await fill('37')
    await act(async()=>input().focus())
    const event=new KeyboardEvent('keydown',{key:'F10',shiftKey:true,bubbles:true,cancelable:true})
    await act(async()=>input().dispatchEvent(event))
    expect(event.defaultPrevented).toBe(true)
    expect(document.activeElement?.getAttribute('data-workbook-input-mode')).toBe('auto')
    await act(async()=>document.activeElement!.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true,cancelable:true})))
    expect(document.activeElement?.getAttribute('data-workbook-input-mode')).toBe('text')
    await act(async()=>document.activeElement!.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true})))
    expect(document.querySelector('[role="menu"][aria-label="单元格输入类型"]')).toBeNull()
    expect(document.activeElement).toBe(input())
    await openTypes()
    const outside=document.createElement('button');document.body.append(outside)
    try {
      await act(async()=>outside.focus())
      expect(document.querySelector('[role="menu"][aria-label="单元格输入类型"]')).toBeNull()
      expect(document.activeElement).toBe(outside)
    } finally {outside.remove()}
    expect(request).not.toHaveBeenCalled()
  })
  it('type context menu retains explicit override across remount and can restore auto from explicit empty',async()=>{
    await render();await choose('text');await fill('123')
    await act(async()=>root!.unmount());root=createRoot(container);await render()
    await fill('124')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'text',type:'text',value:'124'})
    await choose('empty')
    expect(input().disabled).toBe(false);expect(input().readOnly).toBe(true)
    await choose('auto')
    expect(input().readOnly).toBe(false)
    await fill('125')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'auto',type:'number',value:'125'})
    expect(request).not.toHaveBeenCalled()
  })
  it('input mode preserves legacy retained draft type when no mode field exists',async()=>{
    useNativeOfficeStore.getState().setWorkbookDraft(draftKey,{id:'legacy',workspace,path:view.path,threadId,objectId:view.objectId,
      revision:view.revision,sheetName:'Sheet1',cell:structuredClone(editableCell),type:'text',value:'123',changed:true})
    await render();await fill('124')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({id:'legacy',inputMode:'text',type:'text',value:'124'})
    expect(request).not.toHaveBeenCalled()
  })
  it('type context menu retires on selection or visibility changes and respects IME composition',async()=>{
    await render();await fill('37');await openTypes()
    await render({...view,selection:{...selection,token:'new-selection'}})
    expect(document.querySelector('[role="menu"][aria-label="单元格输入类型"]')).toBeNull()
    await openTypes();await render(view,false)
    expect(document.querySelector('[role="menu"][aria-label="单元格输入类型"]')).toBeNull()
    await render()
    await act(async()=>input().dispatchEvent(new CompositionEvent('compositionstart',{bubbles:true,data:'中'})))
    const event=new MouseEvent('contextmenu',{bubbles:true,cancelable:true})
    await act(async()=>input().dispatchEvent(event))
    expect(event.defaultPrevented).toBe(false)
    expect(document.querySelector('[role="menu"][aria-label="单元格输入类型"]')).toBeNull()
    await act(async()=>input().dispatchEvent(new CompositionEvent('compositionend',{bubbles:true,data:'中'})))
    expect(input().value).toBe('37');expect(request).not.toHaveBeenCalled()
  })
  it('context type actions fence live store selection, input state and lifecycle before applying local intent',async()=>{
    const offered:{current:WorkbookCellContextActions|null}={current:null}
    const publish=(next:WorkbookCellContextActions|null)=>{offered.current=next}
    await render(view,true,{onContextActionsChange:publish});await fill('37')
    const context=offered.current!,draft=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    const text=context.actions.find(action=>action.id === 'workbook-input-text')!
    expect(context.selectionToken).toBe(selection.token)
    const moved={...selection,token:'moved',ranges:[{...selection.ranges[0],startColumn:1,endColumn:1}],cells:[{...editableCell,column:1}]}
    const invalidStates=[{view:{...view,selection:{...selection,token:'renewed'}}},{view:{...view,selection:moved}},
      {view:{...view,revision:'e'.repeat(64)}},{view:{...view,changeSequence:1}},
      {workbookDraftInputFrozen:true},{workbookDraftComposing:true},{workbookClosing:{[targetKey]:true as const}}]
    for (const invalid of invalidStates) {
      await act(async()=>{useNativeOfficeStore.setState(invalid);text.run()})
      expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBe(draft)
      await act(async()=>useNativeOfficeStore.setState({view,workbookDraftInputFrozen:false,workbookDraftComposing:false,workbookClosing:{}}))
    }
    const newer={...draft,value:'38'}
    await act(async()=>{useNativeOfficeStore.getState().setWorkbookDraft(draftKey,newer,draft.id);text.run()})
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBe(newer)
    await act(async()=>useNativeOfficeStore.getState().setWorkbookDraft(draftKey,draft,draft.id))
    await act(async()=>text.run())
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({inputMode:'text',type:'text',value:'37'})
    const current=offered.current!,retained=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    await render({...view,selection:moved},true,{onContextActionsChange:publish})
    expect(offered.current).toBeNull()
    await act(async()=>current.actions[0].run())
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBe(retained)
    await render(view,false,{onContextActionsChange:publish});expect(offered.current).toBeNull()
    await act(async()=>root!.unmount());root=null
    await act(async()=>current.actions[0].run())
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBe(retained)
    expect(request).not.toHaveBeenCalled()
  })
  it('portals actions into the existing viewer row and keeps the formula row for content only',async()=>{
    await render()
    expect(toolbarTarget.contains(button('保存'))).toBe(true)
    expect(toolbarTarget.contains(button('单元格编辑选项'))).toBe(true)
    expect(container.querySelectorAll('button,select')).toHaveLength(0)
    expect(container.querySelector('[aria-label="工作簿选区"]')?.textContent).toBe('Sheet1!A1')
    expect(button('保存').disabled).toBe(true)
    expect(button('保存').getAttribute('data-tone')).toBeNull()
    await fill('13')
    expect(button('保存').disabled).toBe(false)
    expect(button('保存').getAttribute('data-tone')).toBe('commit')
    await fill('=')
    expect(button('保存').disabled).toBe(true)
    expect(button('保存').getAttribute('data-tone')).toBeNull()
    expect(document.querySelector('[aria-label="单元格输入类型"]')).toBeNull()
    await openOptions()
    expect(document.querySelector('[role="menu"][aria-label="单元格输入类型"]')).toBeNull()
    expect(document.querySelector('[data-workbook-input-mode]')).toBeNull()
    expect(request).not.toHaveBeenCalled()
  })
  it('uses the same content row for reachable standalone fallback actions',async()=>{
    await render(view,true,{toolbar:false})
    expect(container.contains(button('保存'))).toBe(true)
    expect(container.contains(button('单元格编辑选项'))).toBe(true)
    expect(container.querySelector('[data-workbook-cell-editor]')?.children).toHaveLength(1)
    await choose('text');await fill('literal')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({type:'text',value:'literal'})
    await clickButton('放弃草稿')
    expect(input().value).toBe('12')
  })
  it('waits for the parent toolbar slot without inserting actions into the formula row',async()=>{
    await render(view,true,{toolbar:null})
    expect(container.querySelectorAll('button,select')).toHaveLength(0)
    expect(button('保存')).toBeUndefined()
    await fill('13')
    await act(async()=>{await useNativeOfficeStore.getState().close(workspace,view.path)})
    expect(document.querySelector('[role="dialog"][aria-label="单元格编辑状态"]')).toBeNull()
    await render()
    expect(toolbarTarget.contains(button('保存'))).toBe(true)
    expect(container.querySelectorAll('button,select')).toHaveLength(0)
    expect(input().value).toBe('13')
    const feedback=document.querySelector<HTMLElement>('[role="dialog"][aria-label="单元格编辑状态"]')
    expect(feedback?.textContent).toContain('保留会取消关闭')
    expect(feedback?.style.visibility).not.toBe('hidden')
    expect(request).not.toHaveBeenCalled()
  })
  it('keeps text literal and only parses a finite number after explicit type choice',()=>{
    expect(workbookDraftEdit('text','001')).toEqual({type:'text',text:'001'})
    expect(workbookDraftEdit('text','')).toBeNull()
    expect(workbookDraftEdit('text',' ')).toEqual({type:'text',text:' '})
    expect(workbookDraftEdit('text','=SUM(A1:A2)')).toEqual({type:'text',text:'=SUM(A1:A2)'})
    expect(workbookDraftEdit('number','001')).toEqual({type:'number',value:1})
    expect(workbookDraftEdit('empty','12')).toEqual({type:'empty'})
    for(const value of ['', ' ', '12 apples','0x10','Infinity','1e999']) expect(workbookDraftEdit('number',value)).toBeNull()
    expect(workbookDraftEdit('formula','SUM(A1:A2)')).toBeNull()
    expect(workbookDraftEdit('formula','=SUM(A1:A2)')).toEqual({type:'formula',formula:'=SUM(A1:A2)'})
  })
  it('keeps changes local until Save and only explicit discard removes the retained draft',async()=>{
    await render();await choose('text');await fill('001')
    expect(request).not.toHaveBeenCalled();expect(button('保存').disabled).toBe(false)
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({type:'text',value:'001',changed:true})
    expect(useNativeOfficeStore.getState().views[targetKey].dirty).toBe(true)
    await clickButton('保留草稿')
    expect(input().value).toBe('001');expect(button('保存').disabled).toBe(false)
    await clickButton('放弃草稿')
    expect(input().value).toBe('12');expect(button('保存').disabled).toBe(true);expect(request).not.toHaveBeenCalled()
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
    expect(useNativeOfficeStore.getState().views[targetKey].dirty).toBe(false)
  })
  it('does not submit a typed edit that leaves the stored number unchanged',async()=>{
    await render();await fill('12.0')
    expect(button('保存').disabled).toBe(true);expect(request).not.toHaveBeenCalled()
  })
  it('requires the explicit clear type for an empty text input',async()=>{
    await render();await choose('text');await fill('')
    expect(button('保存').disabled).toBe(true)
    expect(document.body.textContent).toContain('空文本请明确选择清空类型。')
    expect(request).not.toHaveBeenCalled()
    await choose('empty');await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({action:'editWorkbookCell',edit:{type:'empty'}}))
  })
  it('sends an explicit typed manual request and only claims saved after a committed receipt',async()=>{
    request.mockResolvedValue({ok:true,view:receipt()})
    await render();await choose('text');await fill('001');await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith({action:'editWorkbookCell',objectId:view.objectId,revision:view.revision,
      expectedChangeSequence:0,threadId:'thread',selectionToken:selection.token,edit:{type:'text',text:'001'}})
    expect(document.body.textContent).toContain('已保存')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
  })
  it('does not call an unreceipted success saved',async()=>{
    await render();await fill('13');await act(async()=>button('保存').click())
    expect(document.body.textContent).not.toContain('已保存')
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('保存结果尚未确认')
    expect(document.querySelector('[role="dialog"][aria-label="单元格编辑状态"]')?.textContent).toContain('保存结果尚未确认')
    expect(button('保存').disabled).toBe(true)
    expect(button('保存').getAttribute('data-tone')).toBeNull()
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({value:'13',unconfirmed:true,pending:false})
  })
  it('keeps pending state reachable in the viewer row without claiming a receipt',async()=>{
    let finish!:(response:NativeOfficeResponse)=>void
    request.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
    await render();await fill('13');await act(async()=>button('保存').click())
    expect(button('保存中…').disabled).toBe(true)
    expect(button('保存中…').getAttribute('data-tone')).toBeNull()
    expect(toolbarTarget.querySelector('[role="status"]')?.textContent).toContain('等待文件回执')
    expect(document.body.textContent).not.toContain('已保存')
    await act(async()=>finish({ok:true,view:receipt('number','13')}))
    expect(toolbarTarget.querySelector('[role="status"]')?.textContent).toContain('Sheet1!A1 已保存')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
  })
  it('renews the native token for the same cell without erasing input and saves with the latest token',async()=>{
    request.mockResolvedValue({ok:true,view:receipt('number','13')})
    await render();await fill('13')
    const id=useNativeOfficeStore.getState().workbookDrafts[draftKey].id
    await render({...view,selection:{...selection,token:'cell-selection-2'}})
    expect(input().value).toBe('13');expect(button('保存').disabled).toBe(false)
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey].id).toBe(id)
    await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({selectionToken:'cell-selection-2',edit:{type:'number',value:13}}))
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
  })
  it.each(['selection','revision'] as const)('retains the original draft and disables Save when %s changes',async kind=>{
    await render();await fill('13')
    const original=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    const next = kind === 'selection' ? {...view,selection:{...selection,token:'cell-selection-2',
      ranges:[{...selection.ranges[0],startColumn:1,endColumn:1}],cells:[{...selection.cells![0],column:1}]}}
      : {...view,revision:'c'.repeat(64),selection:{...selection,version:'c'.repeat(64)}}
    await render(next)
    expect(input().value).toBe('13');expect(button('保存').disabled).toBe(true)
    expect(document.body.textContent).toContain('当前会话、选区或版本已变化，文字已保留')
    expect(container.querySelector('[aria-label="工作簿选区"]')?.textContent).toBe('Sheet1!A1')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toEqual(original)
    expect(request).not.toHaveBeenCalled()
    await render({...view,selection:{...selection,token:'cell-selection-returned'}})
    expect(input().value).toBe('13');expect(button('保存').disabled).toBe(false)
  })
  it('clears the submitted original-cell draft on its valid late receipt after selection changes',async()=>{
    let finish!:(response:NativeOfficeResponse)=>void
    request.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
    await render();await fill('13');await act(async()=>button('保存').click())
    await render({...view,selection:{...selection,token:'cell-selection-2',
      ranges:[{...selection.ranges[0],startColumn:1,endColumn:1}],cells:[{...selection.cells![0],column:1}]}})
    expect(input().value).toBe('13');expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]?.pending).toBe(true)
    await act(async()=>finish({ok:true,view:receipt('number','13')}))
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
    expect(document.body.textContent).toContain('Sheet1!A1 已保存')
    expect(input().value).toBe('12')
  })
  it('retains the exact draft across unmount and remount',async()=>{
    await render();await choose('text');await fill('retained words')
    const original=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    await act(async()=>root!.unmount());root=null
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toEqual(original)
    root=createRoot(container);await render()
    expect(input().value).toBe('retained words')
    await openTypes()
    expect(document.querySelector('[data-workbook-input-mode="text"]')?.getAttribute('aria-checked')).toBe('true')
    expect(button('保存').disabled).toBe(false)
    expect(request).not.toHaveBeenCalled()
  })
  it('keeps separate document drafts while switching away and back',async()=>{
    const other=otherWorkbook(),otherKey=nativeWorkbookDraftKey(workspace,other.path,threadId)
    await render();await fill('13')
    await render(other);expect(input().value).toBe('20');await fill('21')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey].value).toBe('13')
    expect(useNativeOfficeStore.getState().workbookDrafts[otherKey].value).toBe('21')
    await render();expect(input().value).toBe('13')
    await render(other);expect(input().value).toBe('21')
    expect(request).not.toHaveBeenCalled()
  })
  it('keeps another conversation draft visible without transferring its save authority',async()=>{
    await render();await fill('13')
    const original=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    const otherKey=nativeWorkbookDraftKey(workspace,view.path,'other-thread')
    await render(view,true,{threadId:'other-thread'})
    expect(input().value).toBe('13');expect(button('保存').disabled).toBe(true)
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toEqual(original)
    expect(useNativeOfficeStore.getState().workbookDrafts[otherKey]).toBeUndefined()
    await clickButton('保留草稿')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({id:original.id,threadId,value:'13'})
    await clickButton('放弃草稿')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
    expect(input().value).toBe('12')
    await fill('14')
    expect(useNativeOfficeStore.getState().workbookDrafts[otherKey]).toMatchObject({threadId:'other-thread',value:'14'})
    expect(request).not.toHaveBeenCalled()
  })
  it('a late receipt for another document clears only the submitted draft',async()=>{
    const other=otherWorkbook(),otherKey=nativeWorkbookDraftKey(workspace,other.path,threadId)
    await render(other);await fill('21')
    await render();await fill('13')
    let finish!:(response:NativeOfficeResponse)=>void
    request.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
    await act(async()=>button('保存').click())
    await render(other);expect(input().value).toBe('21')
    await act(async()=>finish({ok:true,view:receipt('number','13')}))
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
    expect(useNativeOfficeStore.getState().workbookDrafts[otherKey]).toMatchObject({value:'21',changed:true})
    expect(input().value).toBe('21');expect(document.body.textContent).not.toContain('已保存')
  })
  it('offers explicit keep or discard after a close request without silently clearing input',async()=>{
    await render();await fill('13')
    let closed=true
    await act(async()=>{closed=await useNativeOfficeStore.getState().close(workspace,view.path)})
    expect(closed).toBe(false)
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('保留会取消关闭')
    await clickButton('保留草稿')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({value:'13',closeRequested:false})
    expect(input().value).toBe('13');expect(request).not.toHaveBeenCalled()
    await act(async()=>{closed=await useNativeOfficeStore.getState().close(workspace,view.path)})
    expect(closed).toBe(false)
    expect(document.querySelector('[role="alert"]')?.textContent).toContain('保留会取消关闭')
    await clickButton('放弃草稿')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
    expect(input().value).toBe('12');expect(request).not.toHaveBeenCalled()
  })
  it('Escape preserves the draft and cancels its close prompt',async()=>{
    await render();await fill('13')
    await act(async()=>{await useNativeOfficeStore.getState().close(workspace,view.path)})
    await act(async()=>input().dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true})))
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({value:'13',closeRequested:false})
    expect(input().value).toBe('13');expect(button('保存').disabled).toBe(false)
    expect(request).not.toHaveBeenCalled()
  })
  it('focuses only the bounded input on native F2 or double-click intent',async()=>{
    await render({...view,selection:{...selection,editIntent:{kind:'f2',id:'edit-intent-1'}}})
    expect(document.activeElement).toBe(input());expect(request).not.toHaveBeenCalled()
    await render({...view,workbookEditing:{editable:false,reason:'unsupported_workbook'}})
    expect(input().disabled).toBe(true);expect(document.body.textContent).toContain('只读预览')
  })
})

describe('retained workbook draft shutdown guard',()=>{
  it.each(['hidden','unmounted'] as const)('vetoes shutdown for a %s draft until explicit discard',async state=>{
    const handler=createWriteShutdownHandler(),blockedId=`draft-${state}`,readyId=`discarded-${state}`
    try {
      await render();await fill('13')
      if (state === 'hidden') await render(view,false)
      else {await act(async()=>root!.unmount());root=null}
      expect(await shutdown(handler,blockedId,'prepare')).toEqual({result:'blocked',reason:'unavailable'})
      expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({value:'13',changed:true})
      expect(isWriteShutdownFrozen()).toBe(false);expect(beginShutdown).not.toHaveBeenCalled();expect(request).not.toHaveBeenCalled()
      if (!root) root=createRoot(container)
      await render();await clickButton('放弃草稿')
      expect(await shutdown(handler,readyId,'prepare')).toEqual({result:'ready'})
      expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
      expect(beginShutdown).toHaveBeenCalledOnce();expect(request).not.toHaveBeenCalled()
    } finally {
      await shutdown(handler,readyId,'cancel');await shutdown(handler,blockedId,'cancel')
    }
    expect(useNativeOfficeStore.getState().workbookDraftInputFrozen).toBe(false)
  })
  it('reports real IME composition separately without freezing or creating a draft',async()=>{
    const handler=createWriteShutdownHandler(),requestId='workbook-composing'
    try {
      await render()
      await act(async()=>input().dispatchEvent(new CompositionEvent('compositionstart',{bubbles:true,data:'中'})))
      expect(useNativeOfficeStore.getState().workbookDraftComposing).toBe(true)
      expect(await shutdown(handler,requestId,'prepare')).toEqual({result:'blocked',reason:'composing'})
      expect(isWriteShutdownFrozen()).toBe(false);expect(useNativeOfficeStore.getState().workbookDraftInputFrozen).toBe(false)
      expect(beginShutdown).not.toHaveBeenCalled();expect(useNativeOfficeStore.getState().workbookDrafts).toEqual({})
      expect(request).not.toHaveBeenCalled()
    } finally {
      await act(async()=>input().dispatchEvent(new CompositionEvent('compositionend',{bubbles:true,data:'中'})))
      await shutdown(handler,requestId,'cancel')
    }
    expect(useNativeOfficeStore.getState().workbookDraftComposing).toBe(false)
  })
  it('rejects first input and a new draft racing an already frozen clean shutdown',async()=>{
    const handler=createWriteShutdownHandler(),requestId='workbook-first-input-frozen'
    const candidate:NativeWorkbookCellDraft={id:'draft-after-freeze',workspace,path:view.path,threadId,objectId:view.objectId,
      revision:view.revision,sheetName:'Sheet1',cell:structuredClone(editableCell),type:'text',value:'forbidden',changed:true}
    try {
      await render()
      let result!:Awaited<ReturnType<typeof handler>>
      await act(async()=>{
        const preparing=handler({requestId,phase:'prepare'})
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input(),'13')
        input().dispatchEvent(new Event('input',{bubbles:true}))
        useNativeOfficeStore.getState().setWorkbookDraft(draftKey,candidate)
        result=await preparing
      })
      expect(result).toEqual({result:'ready'})
      expect(isWriteShutdownFrozen()).toBe(true);expect(useNativeOfficeStore.getState().workbookDraftInputFrozen).toBe(true)
      expect(useNativeOfficeStore.getState().workbookDrafts).toEqual({});expect(input().disabled).toBe(true)
      expect(button('保存').disabled).toBe(true);expect(request).not.toHaveBeenCalled()
    } finally {
      await shutdown(handler,requestId,'cancel')
    }
    expect(useNativeOfficeStore.getState().workbookDraftInputFrozen).toBe(false)
    await fill('14');expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]?.value).toBe('14')
  })
})

describe('readonly complete selection statistics',()=>{
  it('sums canonical complete bounded numeric cells and verified formula results',()=>{
    const selected:Cells = {...selection,ranges:[{...selection.ranges[0],endRow:2}],cells:[selection.cells![0],
      {...selection.cells![0],row:1,valueType:'formula',formula:'=A1*2',value:24,valueError:false,formulaResultType:'number'},
      {...selection.cells![0],row:2,valueType:'text',text:'100',value:0}]}
    expect(workbookSelectionSum(selected)).toBe(36)
    expect(workbookSelectionSum({...selected,cells:[selected.cells![0],{...selected.cells![1],valueError:true},selected.cells![2]]})).toBeNull()
    expect(workbookSelectionSum({...selected,cells:[selected.cells![0],{...selected.cells![1],valueError:undefined},selected.cells![2]]})).toBeNull()
    expect(workbookSelectionSum({...selected,cells:[selected.cells![0],{...selected.cells![1],formulaResultType:undefined},selected.cells![2]]})).toBeNull()
    expect(workbookSelectionSum({...selected,cells:[selected.cells![0],{...selected.cells![1],formulaResultType:'text',value:0,text:'example'},selected.cells![2]]})).toBe(12)
  })
  it('does not report an entire row or column sum from a partial or incomplete capture',()=>{
    expect(workbookSelectionSum({...selection,ranges:[{...selection.ranges[0],endRow:1048575}],capture:{...selection.capture!,complete:false,truncated:true}})).toBeNull()
    expect(workbookSelectionSum({...selection,ranges:[{...selection.ranges[0],endRow:1}]})).toBeNull()
    expect(workbookSelectionSum({...selection,cells:[{...selection.cells![0],row:1}]})).toBeNull()
    expect(workbookSelectionSum({...selection,ranges:[selection.ranges[0],selection.ranges[0]]})).toBeNull()
  })
})
