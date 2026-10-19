// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeOfficeRequest, NativeOfficeResponse, NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import { WorkbookCellEditor, workbookDraftEdit, workbookSelectionSum } from './WorkbookCellEditor'
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
let root:Root|null, container:HTMLDivElement
let request:ReturnType<typeof vi.fn<(input:NativeOfficeRequest)=>Promise<NativeOfficeResponse | undefined>>>
async function render(next:NativeOfficeView = view,active = true,context:{workspace?:string;path?:string;threadId?:string}={}) {
  await act(async()=>root!.render(createElement(WorkbookCellEditor,{view:next,threadId:context.threadId??threadId,
    workspace:context.workspace??workspace,path:context.path??next.path,active,busy:false,onRequest:request})))
}
const input = () => container.querySelector<HTMLInputElement>('[aria-label="单元格值或公式"]')!
const button = (label:string) => [...container.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent===label)!
async function fill(value:string) {
  await act(async()=>{
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input(),value)
    input().dispatchEvent(new Event('input',{bubbles:true}))
  })
}
async function choose(type:string) {
  await act(async()=>{
    const select = container.querySelector<HTMLSelectElement>('[aria-label="单元格输入类型"]')!
    select.value=type;select.dispatchEvent(new Event('change',{bubbles:true}))
  })
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
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
})
afterEach(async()=>{
  await act(async()=>root?.unmount());root=null;container.remove()
  useNativeOfficeStore.setState({target:null,view:null,error:null,engineViews:{},views:{},workbookDrafts:{},workbookClosing:{},
    workbookDraftInputFrozen:false,workbookDraftComposing:false})
  vi.unstubAllGlobals()
})

describe('typed workbook cell drafts',()=>{
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
    await act(async()=>button('保留草稿').click())
    expect(input().value).toBe('001');expect(button('保存').disabled).toBe(false)
    await act(async()=>button('放弃草稿').click())
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
    expect(container.textContent).toContain('空文本请明确选择清空类型。')
    expect(request).not.toHaveBeenCalled()
    await choose('empty');await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({action:'editWorkbookCell',edit:{type:'empty'}}))
  })
  it('sends an explicit typed manual request and only claims saved after a committed receipt',async()=>{
    request.mockResolvedValue({ok:true,view:receipt()})
    await render();await choose('text');await fill('001');await act(async()=>button('保存').click())
    expect(request).toHaveBeenCalledExactlyOnceWith({action:'editWorkbookCell',objectId:view.objectId,revision:view.revision,
      expectedChangeSequence:0,threadId:'thread',selectionToken:selection.token,edit:{type:'text',text:'001'}})
    expect(container.textContent).toContain('已保存')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toBeUndefined()
  })
  it('does not call an unreceipted success saved',async()=>{
    await render();await fill('13');await act(async()=>button('保存').click())
    expect(container.textContent).not.toContain('已保存')
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('保存结果尚未确认')
    expect(button('保存').disabled).toBe(true)
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({value:'13',unconfirmed:true,pending:false})
  })
  it.each(['saving','loading','unknown'] as const)('retains words and blocks input, resubmission and discard while %s',async phase=>{
    await render();await fill('13')
    const original=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    const next:NativeOfficeView={...view,...(phase==='saving'?{saving:true}:phase==='loading'?{status:'loading' as const}:{error:'unknown' as const})}
    await render(next)
    expect(input().value).toBe('13');expect(input().disabled).toBe(true)
    for(const label of ['保存','放弃草稿','保留草稿']) expect(button(label).disabled).toBe(true)
    await act(async()=>{button('保存').click();button('放弃草稿').click();button('保留草稿').click()})
    await fill('14')
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toEqual(original)
    expect(request).not.toHaveBeenCalled()
    await render()
    expect(input().value).toBe('13');expect(input().disabled).toBe(false)
    expect(button('保存').disabled).toBe(false);expect(button('放弃草稿').disabled).toBe(false)
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
    expect(container.textContent).toContain('当前会话、选区或版本已变化，文字已保留')
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
    expect(container.textContent).toContain('Sheet1!A1 已保存')
    expect(input().value).toBe('12')
  })
  it('retains the exact draft across unmount and remount',async()=>{
    await render();await choose('text');await fill('retained words')
    const original=useNativeOfficeStore.getState().workbookDrafts[draftKey]
    await act(async()=>root!.unmount());root=null
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toEqual(original)
    root=createRoot(container);await render()
    expect(input().value).toBe('retained words')
    expect(container.querySelector<HTMLSelectElement>('[aria-label="单元格输入类型"]')!.value).toBe('text')
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
    await act(async()=>button('保留草稿').click())
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({id:original.id,threadId,value:'13'})
    await act(async()=>button('放弃草稿').click())
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
    expect(input().value).toBe('21');expect(container.textContent).not.toContain('已保存')
  })
  it('offers explicit keep or discard after a close request without silently clearing input',async()=>{
    await render();await fill('13')
    let closed=true
    await act(async()=>{closed=await useNativeOfficeStore.getState().close(workspace,view.path)})
    expect(closed).toBe(false)
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('保留会取消关闭')
    await act(async()=>button('保留草稿').click())
    expect(useNativeOfficeStore.getState().workbookDrafts[draftKey]).toMatchObject({value:'13',closeRequested:false})
    expect(input().value).toBe('13');expect(request).not.toHaveBeenCalled()
    await act(async()=>{closed=await useNativeOfficeStore.getState().close(workspace,view.path)})
    expect(closed).toBe(false)
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('保留会取消关闭')
    await act(async()=>button('放弃草稿').click())
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
    expect(input().disabled).toBe(true);expect(container.textContent).toContain('只读预览')
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
      await render();await act(async()=>button('放弃草稿').click())
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
