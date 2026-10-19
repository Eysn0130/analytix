// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeOfficeRequest, NativeOfficeResponse, NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import { WorkbookCellEditor, workbookDraftEdit, workbookSelectionSum } from './WorkbookCellEditor'

type Cells = Extract<NativeOfficeSelection,{kind:'cells'}>
const selection:Cells = {kind:'cells',documentId:'a'.repeat(64),version:'b'.repeat(64),changeSequence:0,token:'cell-selection-1',
  scope:'sheet-range-address-at-version-and-change-sequence',text:'12',capture:{capturedCharacters:2,totalCharacters:2,unit:'utf-16',complete:true,truncated:false},
  ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}],
  cells:[{sheet:0,column:0,row:0,text:'12',value:12,formula:'12',valueType:'number',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}]}
const view:NativeOfficeView = {objectId:selection.documentId,path:'/synthetic/book.xlsx',kind:'xlsx',revision:selection.version,
  dirty:false,status:'ready',changeSequence:0,editing:true,workbookEditing:{editable:true},selection}
function receipt():NativeOfficeView {
  return {...view,revision:'c'.repeat(64),selection:undefined,recovery:{pending:null,current:{version:3,origin:'manual',changeId:'d'.repeat(64),
    threadId:'thread',baseRevision:view.revision,revision:'c'.repeat(64),status:'committed',
    workbook:{before:{...selection.ranges[0],cells:selection.cells!},after:{...selection.ranges[0],cells:[{...selection.cells![0],text:'001',formula:'001',valueType:'text',value:0}]},results:['001']},
    saveOperationId:'save-operation',undoOperationId:'undo-operation',canUndo:true,canCancel:false,canRetryUndo:false,canResume:false,createdAt:'',savedAt:''}}}
}
let root:Root, container:HTMLDivElement
let request:ReturnType<typeof vi.fn<(input:NativeOfficeRequest)=>Promise<NativeOfficeResponse | undefined>>>
async function render(next:NativeOfficeView = view,active = true) {
  await act(async()=>root.render(createElement(WorkbookCellEditor,{view:next,threadId:'thread',active,busy:false,onRequest:request})))
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
beforeEach(()=>{
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
  request=vi.fn(async()=>({ok:true,view}))
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
})
afterEach(async()=>{await act(async()=>root.unmount());container.remove();vi.unstubAllGlobals()})

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
  it('keeps changes local until Save and Cancel restores the captured cell',async()=>{
    await render();await choose('text');await fill('001')
    expect(request).not.toHaveBeenCalled();expect(button('保存').disabled).toBe(false)
    await act(async()=>button('取消').click())
    expect(input().value).toBe('12');expect(button('保存').disabled).toBe(true);expect(request).not.toHaveBeenCalled()
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
  })
  it('does not call an unreceipted success saved',async()=>{
    await render();await fill('13');await act(async()=>button('保存').click())
    expect(container.textContent).not.toContain('已保存')
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('保存结果尚未确认')
    expect(button('保存').disabled).toBe(true)
  })
  it.each(['selection','revision'] as const)('invalidates an old draft when %s changes',async kind=>{
    await render();await fill('13')
    const next = kind === 'selection' ? {...view,selection:{...selection,token:'cell-selection-2'}}
      : {...view,revision:'c'.repeat(64),selection:{...selection,version:'c'.repeat(64)}}
    await render(next)
    expect(input().value).toBe('12');expect(button('保存').disabled).toBe(true)
  })
  it('ignores a late save receipt after the selected token changes',async()=>{
    let finish!:(response:NativeOfficeResponse)=>void
    request.mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
    await render();await fill('13');await act(async()=>button('保存').click())
    await render({...view,selection:{...selection,token:'cell-selection-2'}})
    await act(async()=>finish({ok:true,view:receipt()}))
    expect(input().value).toBe('12');expect(container.textContent).not.toContain('已保存')
  })
  it('focuses only the bounded input on native F2 or double-click intent',async()=>{
    await render({...view,selection:{...selection,editIntent:{kind:'f2',id:'edit-intent-1'}}})
    expect(document.activeElement).toBe(input());expect(request).not.toHaveBeenCalled()
    await render({...view,workbookEditing:{editable:false,reason:'unsupported_workbook'}})
    expect(input().disabled).toBe(true);expect(container.textContent).toContain('只读预览')
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
