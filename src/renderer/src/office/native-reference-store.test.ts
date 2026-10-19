import { describe, expect, it } from 'vitest'
import type { NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import { isNativeSelectionEditable, nativeActionReferencesCurrent, nativeOfficeDraftReferencesCurrent, nativeOfficeSavedSelection, nativeReferencesPrompt, nativeSelectionLabel, nativeSelectionText, validateOfficeReferences, useNativeReferenceStore, type ImageRegionNativeReference, type NativeReference, type OfficeNativeReference } from './native-reference-store'

const view: NativeOfficeView = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), path: '/synthetic/report.docx', kind: 'docx', status: 'ready', dirty: false, editing: true, changeSequence: 2 }
const selection: NativeOfficeSelection = { kind: 'text', documentId: view.objectId, version: view.revision, changeSequence: 2, token: 'selection-token', scope: 'session-text-range-at-version-and-change-sequence', text: 'Synthetic text', capture: { capturedCharacters: 14, totalCharacters: 14, unit: 'utf-16', truncated: false, complete: true } }
const reference: OfficeNativeReference = { id: 'ref', threadId: 'thread', workspace: '/synthetic', path: view.path, objectId: view.objectId, revision: view.revision, selection, label: 'report.docx · 选中文本', text: selection.text, note: '请澄清这句话' }

describe('native annotation references', () => {
  it('keeps the true whole-column or row range and partial capture in discussion references', () => {
    const captured:NativeOfficeSelection = {...selection,kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',text:'1',
      ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:1048575}],
      capture:{capturedCharacters:1,totalCharacters:2048,unit:'utf-16',truncated:true,complete:false}}
    expect(nativeSelectionLabel({...view,path:'/synthetic/book.xlsx'},captured)).toBe('book.xlsx · Sheet1!A1:A1048576（部分捕获）')
    expect(nativeSelectionText(captured)).toContain('[选区仅部分捕获；范围 Sheet1!A1:A1048576]')
    const row = {...captured,ranges:[{...captured.ranges[0],endColumn:16383,endRow:0}]}
    expect(nativeSelectionLabel({...view,path:'/synthetic/book.xlsx'},row)).toContain('Sheet1!A1:XFD1（部分捕获）')
    expect(isNativeSelectionEditable({...view,kind:'xlsx'},captured)).toBe(false)
  })
  it('requires a complete current controlled single text target for direct proposals', () => {
    expect(isNativeSelectionEditable(view, selection)).toBe(true)
    expect(isNativeSelectionEditable({ ...view, editing: false }, selection)).toBe(false)
    expect(isNativeSelectionEditable(view, { ...selection, version: 'c'.repeat(64) })).toBe(false)
    expect(isNativeSelectionEditable(view, { ...selection, scope: 'current-view-text-only; no verified structural offset or durable anchor' })).toBe(false)
    expect(isNativeSelectionEditable(view, { ...selection, capture: { ...selection.capture!, complete: false, truncated: true } })).toBe(false)
    expect(isNativeSelectionEditable(view, { ...selection, text: '' })).toBe(false)
  })
  it('rejects incomplete, merged or multi-shape selections while typed numbers retain explicit edit scope', () => {
    const cells: NativeOfficeSelection = { ...selection, kind: 'cells', scope: 'sheet-range-address-at-version-and-change-sequence', text: 'Value', ranges: [{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}], cells:[{sheet:0,column:0,row:0,text:'Value',formula:'',value:0,valueType:'text',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}] }
    expect(isNativeSelectionEditable(view, cells)).toBe(true)
    expect(isNativeSelectionEditable(view, { ...cells, ranges:[{...cells.ranges[0],endRow:1}] })).toBe(false)
    expect(isNativeSelectionEditable(view, { ...cells, cells:[{...cells.cells![0],merged:true}] })).toBe(false)
    expect(isNativeSelectionEditable({...view,kind:'xlsx'}, {...cells,cells:[{...cells.cells![0],valueType:'number',value:1,text:'1',formula:'1'}]})).toBe(true)
    expect(isNativeSelectionEditable({...view,kind:'xlsx'}, {...cells,cells:[{...cells.cells![0],valueType:'formula',value:1,text:'1',formula:'=1'}]})).toBe(true)
    expect(isNativeSelectionEditable({...view,kind:'xlsx',workbookEditing:{editable:false,reason:'unsupported_workbook'}},cells)).toBe(false)
    const shape = {pageIndex:0,shapeIndex:0,name:'Text',type:'text',text:'Value'}
    const shapes: NativeOfficeSelection = { ...selection, kind:'shapes',scope:'page-and-shape-index-at-version-and-change-sequence',shapes:[shape] }
    expect(isNativeSelectionEditable(view, shapes)).toBe(true)
    expect(isNativeSelectionEditable(view, {...shapes,shapes:[shape,{...shape,shapeIndex:1}]})).toBe(false)
  })
  it.each([true,false])('keeps private local fields out of scoped prompts (editable=%s)', editable => {
    const prompt = nativeReferencesPrompt([{...reference,editable,scopeId:'a'.repeat(48)}])
    expect(prompt).toContain('Core scopeId: ' + 'a'.repeat(48))
    expect(prompt).toContain('native_selection_read')
    for (const privateField of [reference.label, reference.text, reference.note!, reference.path, reference.workspace, reference.revision, reference.selection.token!]) expect(prompt).not.toContain(privateField)
    expect(prompt).toContain(editable ? 'explicit user acceptance' : 'discussion-only')
  })
  it.each([undefined,'s'.repeat(48),'a'.repeat(47)])('rejects missing or malformed scope %s without raw fallback', scopeId => {
    expect(() => nativeReferencesPrompt([{...reference,editable:false,scopeId}])).toThrow('recaptured')
  })
})

it('rechecks the native task revision, sequence and scoped thread after asynchronous message preparation', () => {
  const scoped = {...reference,editable:true,scopeId:'a'.repeat(48)}
  const current = {...view,scope:{scopeId:scoped.scopeId,threadId:'thread',selectionToken:selection.token!,baseRevision:view.revision,changeSequence:selection.changeSequence,editable:true,parts:[]}}
  expect(nativeActionReferencesCurrent([scoped],[current])).toBe(true)
  expect(nativeActionReferencesCurrent([scoped],[{...current,revision:'c'.repeat(64)}])).toBe(false)
  expect(nativeActionReferencesCurrent([scoped],[{...current,changeSequence:3}])).toBe(false)
  expect(nativeActionReferencesCurrent([scoped],[{...current,scope:undefined}])).toBe(false)
  expect(nativeActionReferencesCurrent([{...scoped,threadId:'another'}],[current])).toBe(false)
  expect(nativeActionReferencesCurrent([scoped],[])).toBe(false)
})

describe('Office draft reference admission', () => {
  function scopedDraft(editable: boolean, target: NativeOfficeSelection = selection, overrides: Partial<NativeOfficeView> = {}) {
    const scoped = {...reference,editable,scopeId:'a'.repeat(48),selection:target}
    const current: NativeOfficeView = {...view,...overrides,scope:{scopeId:scoped.scopeId,threadId:'thread',
      selectionToken:target.token!,baseRevision:view.revision,changeSequence:target.changeSequence,editable,parts:[]}}
    return {scoped,current}
  }

  it.each([true,false])('admits an explicitly scoped Office draft without changing editable=%s', editable => {
    const {scoped,current} = scopedDraft(editable)
    expect(nativeOfficeDraftReferencesCurrent([scoped],[current])).toBe(true)
    expect(scoped.editable).toBe(editable)
    expect(current.scope?.editable).toBe(editable)
    expect(nativeReferencesPrompt([scoped])).toContain(editable ? 'native_selection_propose' : 'discussion-only')
    expect(nativeOfficeDraftReferencesCurrent([{...scoped,kind:'office'}],[current])).toBe(true)
  })

  it('rejects implicit permission, empty drafts and non-Office references', () => {
    const {scoped,current} = scopedDraft(false)
    expect(nativeOfficeDraftReferencesCurrent([{...scoped,editable:undefined}],[current])).toBe(false)
    expect(nativeOfficeDraftReferencesCurrent([{...scoped,kind:''} as unknown as NativeReference],[current])).toBe(false)
    const editable = scopedDraft(true)
    expect(nativeOfficeDraftReferencesCurrent([{...editable.scoped,editable:'true'} as unknown as NativeReference],[editable.current])).toBe(false)
    expect(nativeOfficeDraftReferencesCurrent([], [current])).toBe(false)
    const canvas:NativeReference={kind:'canvas',id:'canvas',threadId:'thread',workspace:'/synthetic',path:'/synthetic/scene.json',
      objectId:view.objectId,revision:view.revision,label:'scene',text:'',sessionId:'b'.repeat(48),scopeId:scoped.scopeId,editable:true,selectedIds:['shape']}
    expect(nativeOfficeDraftReferencesCurrent([canvas],[current])).toBe(false)
    expect(nativeOfficeDraftReferencesCurrent([scoped,canvas],[current])).toBe(false)
  })

  it.each([true,false])('rejects stale document, token, thread, sequence and scope identity (editable=%s)', editable => {
    const {scoped,current} = scopedDraft(editable)
    for (const next of [
      {...scoped,revision:'c'.repeat(64)}, {...scoped,objectId:'c'.repeat(64)}, {...scoped,threadId:'another'},
      {...scoped,selection:{...selection,token:'another-token'}},
      {...scoped,selection:{...selection,version:'c'.repeat(64)}},
      {...scoped,selection:{...selection,documentId:'c'.repeat(64)}},
      {...scoped,selection:{...selection,changeSequence:3}}, {...scoped,scopeId:'b'.repeat(48)}, {...scoped,editable:!editable}
    ]) expect(nativeOfficeDraftReferencesCurrent([next],[current])).toBe(false)
    for (const field of [{selectionToken:'another-token'}, {baseRevision:'c'.repeat(64)}, {threadId:'another'}, {changeSequence:3}]) {
      expect(nativeOfficeDraftReferencesCurrent([scoped],[{...current,scope:{...current.scope!,...field}}])).toBe(false)
    }
    expect(nativeOfficeDraftReferencesCurrent([scoped],[])).toBe(false)
  })

  it.each([true,false])('rejects protected Present even with matching copied scope metadata (editable=%s)', editable => {
    const {scoped,current} = scopedDraft(editable)
    const protectedPresent={threadId:'thread',turnId:'turn',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),
      viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
    expect(nativeOfficeDraftReferencesCurrent([scoped],[{...current,protectedPresent}])).toBe(false)
  })

  it('requires a ready admitted editor and complete current capture for editable drafts', () => {
    const {scoped,current} = scopedDraft(true)
    for (const state of [{status:'loading' as const}, {status:'error' as const}, {saving:true}, {editing:false}]) {
      expect(nativeOfficeDraftReferencesCurrent([scoped],[{...current,...state}])).toBe(false)
    }
    for (const target of [
      {...selection,capture:undefined}, {...selection,capture:{...selection.capture!,complete:false}},
      {...selection,capture:{...selection.capture!,truncated:true}}, {...selection,version:'c'.repeat(64)},
      {...selection,documentId:'c'.repeat(64)},
      {...selection,scope:'current-view-text-only; no verified structural offset or durable anchor' as const}
    ]) {
      const draft = scopedDraft(true,target)
      expect(nativeOfficeDraftReferencesCurrent([draft.scoped],[draft.current])).toBe(false)
    }
  })
  it('rejects expired edit admission in the Core reply even when the copied scope identity still matches', async () => {
    const {scoped,current} = scopedDraft(true)
    expect(await validateOfficeReferences([scoped],async () => ({ok:true,view:current}),() => true)).toBe(true)
    for (const expired of [{editing:false}, {status:'loading' as const}, {saving:true}]) {
      expect(await validateOfficeReferences([scoped],async () => ({ok:true,view:{...current,...expired}}),() => true)).toBe(false)
    }
  })

  it('retains explicitly readonly discussion captures without imposing edit admission or upgrading them', () => {
    for (const target of [{...selection,capture:undefined}, {...selection,capture:{...selection.capture!,complete:false,truncated:true}}]) {
      const {scoped,current} = scopedDraft(false,target,{editing:false,status:'ready',saving:false})
      expect(nativeOfficeDraftReferencesCurrent([scoped],[current])).toBe(true)
      expect(scoped.editable).toBe(false)
      expect(nativeReferencesPrompt([scoped])).toContain('does not authorize native_selection_propose')
    }
  })

  const cells:NativeOfficeSelection={...selection,kind:'cells',scope:'sheet-range-address-at-version-and-change-sequence',text:'1',
    capture:{capturedCharacters:1,totalCharacters:1,unit:'utf-16',truncated:false,complete:true},
    ranges:[{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}],
    cells:[{sheet:0,column:0,row:0,text:'1',formula:'=1',value:1,valueType:'formula',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}]}
  const shape={pageIndex:0,shapeIndex:0,name:'Rectangle',type:'rectangle',text:''}
  const shapes:NativeOfficeSelection={kind:'shapes',documentId:view.objectId,version:view.revision,changeSequence:selection.changeSequence,
    token:selection.token,scope:'page-and-shape-index-at-version-and-change-sequence',shapes:[shape],
    capture:{capturedCharacters:0,totalCharacters:0,unit:'utf-16',truncated:false,complete:true},
    presentation:{pageIndex:0,targetShapeIndex:0,pageWidth100thMm:100,pageHeight100thMm:100,
      shapes:[{shapeIndex:0,kind:'rectangle',name:'Rectangle',text:'',x100thMm:1,y100thMm:1,width100thMm:10,height100thMm:10,fillRGB:'#123456'}]}}
  it.each([['xlsx',cells],['pptx',shapes]] as const)('preserves existing bounded %s edit admission', (kind,target) => {
    const {scoped,current} = scopedDraft(true,target,{kind})
    expect(nativeOfficeDraftReferencesCurrent([scoped],[current])).toBe(true)
  })

  it('keeps unsupported text, merged cells and multi-shape selections outside editable draft admission', () => {
    const targets:NativeOfficeSelection[]=[
      {...selection,scope:'current-view-text-only; no verified structural offset or durable anchor'},
      {...cells,cells:[{...cells.cells![0],merged:true}]},
      {...shapes,shapes:[shape,{...shape,shapeIndex:1}]}
    ]
    for (const target of targets) {
      const {scoped,current} = scopedDraft(true,target)
      expect(nativeOfficeDraftReferencesCurrent([scoped],[current])).toBe(false)
    }
  })

  it.each(['running','paused'] as const)('rejects all Office drafts during %s playback without changing scope authority', phase => {
    for (const editable of [true,false]) {
      const {scoped,current} = scopedDraft(editable,shapes,{kind:'pptx',playback:{generation:1,sequence:2,phase,slideIndex:0,slideCount:1}})
      expect(nativeOfficeDraftReferencesCurrent([scoped],[current])).toBe(false)
      expect(scoped.editable).toBe(editable)
    }
  })
})


it('replaces an Office object/thread reference without duplicating chips or losing unrelated references', () => {
  const prior = {...reference,scopeId:'a'.repeat(48),editable:false}
  const otherThread = {...prior,id:'other-thread',threadId:'other-thread'}
  const otherObject = {...prior,id:'other-object',objectId:'c'.repeat(64)}
  useNativeReferenceStore.setState({references:[prior,otherThread,otherObject]})
  const replacement = useNativeReferenceStore.getState().add({...reference,scopeId:'b'.repeat(48),editable:false,note:'新标注'})
  expect(useNativeReferenceStore.getState().references).toEqual([otherThread,otherObject,replacement])
})

it('binds readonly scopes and fails after a Core currentness challenge refuses or drifts', async () => {
  const scoped = {...reference,editable:false,scopeId:'a'.repeat(48)}
  const current:NativeOfficeView = {...view,scope:{scopeId:scoped.scopeId,threadId:'thread',selectionToken:selection.token!,baseRevision:view.revision,changeSequence:2,editable:false,parts:[]}}
  expect(nativeActionReferencesCurrent([scoped],[current])).toBe(true)
  for (const field of [{scopeId:'b'.repeat(48)},{threadId:'other'},{selectionToken:'other-token'},{baseRevision:'c'.repeat(64)},{changeSequence:3},{editable:true}]) {
    expect(nativeActionReferencesCurrent([scoped],[{...current,scope:{...current.scope!,...field}}])).toBe(false)
  }
  expect(await validateOfficeReferences([scoped],async input => {
    expect(input).toEqual({action:'proposals',objectId:scoped.objectId,scopeId:scoped.scopeId})
    return {ok:true,view:current}
  },() => true)).toBe(true)
  expect(await validateOfficeReferences([scoped],async () => ({ok:false,view:current,error:'stale_selection'}),() => true)).toBe(false)
  let live=true
  expect(await validateOfficeReferences([scoped],async () => {live=false;return {ok:true,view:current}},() => live)).toBe(false)
})


describe('multi-region image reference identity', () => {
  const image = (regionId: string): ImageRegionNativeReference => ({ kind: 'image-region', id: 'local', threadId: 'thread',
    workspace: '/synthetic', path: '/synthetic/image.png', objectId: 'a'.repeat(64), revision: 'b'.repeat(64),
    label: 'image.png', text: '', sessionId: 'c'.repeat(48), scopeId: 'd'.repeat(48), editable: false,
    annotationRevision: 'e'.repeat(64), regionId, width: 100, height: 80, region: { x: 1, y: 2, width: 3, height: 4 } })

  it('preserves other current regions and replaces only the same object/thread/region identity', () => {
    useNativeReferenceStore.setState({ references: [], drafts: {} })
    const first = useNativeReferenceStore.getState().add(image('1'.repeat(48)))
    const second = useNativeReferenceStore.getState().add({ ...image('2'.repeat(48)), scopeId: '2'.repeat(48) })
    const otherThread = useNativeReferenceStore.getState().add({ ...image('1'.repeat(48)), threadId: 'other-thread' })
    const otherObject = useNativeReferenceStore.getState().add({ ...image('1'.repeat(48)), objectId: 'f'.repeat(64) })
    expect(useNativeReferenceStore.getState().references).toEqual([first, second, otherThread, otherObject])
    const replacement = useNativeReferenceStore.getState().add({ ...image('1'.repeat(48)), scopeId: '3'.repeat(48) })
    expect(useNativeReferenceStore.getState().references).toEqual([second, otherThread, otherObject, replacement])
    expect(nativeReferencesPrompt([second, replacement])).toContain(second.scopeId)
    expect(nativeReferencesPrompt([second, replacement])).toContain(replacement.scopeId)
  })

  it.each(['annotationRevision', 'revision'] as const)('revokes prior region scopes on %s drift while retaining snapshots and unrelated threads', field => {
    useNativeReferenceStore.setState({ references: [], drafts: {} })
    const first = useNativeReferenceStore.getState().add(image('1'.repeat(48)))
    const second = useNativeReferenceStore.getState().add(image('2'.repeat(48)))
    const unrelated = useNativeReferenceStore.getState().add({ ...image('3'.repeat(48)), threadId: 'other-thread' })
    const latest = useNativeReferenceStore.getState().add({ ...image('1'.repeat(48)), [field]: '0'.repeat(64) })
    const references = useNativeReferenceStore.getState().references
    expect(references).toHaveLength(3)
    expect(references.some(reference => reference.id === first.id)).toBe(false)
    expect(references[0]).toEqual({ ...second, scopeId: undefined })
    expect(references[1]).toEqual(unrelated)
    expect(references[2]).toEqual(latest)
    expect(() => nativeReferencesPrompt([references[0]])).toThrow()
  })

  it('does not preserve an old revision scope merely because the new snapshot has no scope', () => {
    useNativeReferenceStore.setState({ references: [], drafts: {} })
    const first = useNativeReferenceStore.getState().add(image('1'.repeat(48)))
    useNativeReferenceStore.getState().add({ ...image('2'.repeat(48)), annotationRevision: '0'.repeat(64), scopeId: undefined })
    expect(useNativeReferenceStore.getState().references[0]).toEqual({ ...first, scopeId: undefined })
  })
})

describe('saved native DOCX reference recapture',()=>{
  const dirty:NativeOfficeView={...view,dirty:true,selection,acknowledgedSequence:1}
  const fresh:NativeOfficeSelection={...selection,version:'c'.repeat(64),changeSequence:0,token:'fresh-native-token',text:'Saved native selection'}
  const saved:NativeOfficeView={...view,revision:fresh.version,dirty:false,changeSequence:0,acknowledgedSequence:0,selection:fresh,
    scope:{scopeId:'d'.repeat(48),threadId:'thread',selectionToken:fresh.token!,baseRevision:fresh.version,changeSequence:0,editable:false,parts:[]}}
  it('admits only the successful fresh selection and exact discussion scope',()=>{
    const admitted=nativeOfficeSavedSelection(dirty,selection,saved,'thread')
    expect(admitted).toEqual(fresh);expect(admitted).not.toBe(fresh)
    const next={...reference,scopeId:saved.scope!.scopeId,editable:false,revision:saved.revision,selection:admitted!,text:'Saved native selection'}
    expect(nativeOfficeDraftReferencesCurrent([next],[saved])).toBe(true)
    expect(nativeOfficeDraftReferencesCurrent([{...next,selection}],[saved])).toBe(false)
    expect(nativeOfficeDraftReferencesCurrent([{...next,revision:dirty.revision}],[saved])).toBe(false)
    const prompt=nativeReferencesPrompt([next])
    for(const privateValue of [next.text,next.label,next.note!,next.path,next.workspace,selection.token!,fresh.token!]) expect(prompt).not.toContain(privateValue)
  })
  it('does not use equal text with the original token as recapture evidence',()=>{
    const oldToken={...fresh,token:selection.token}
    expect(nativeOfficeSavedSelection(dirty,selection,{...saved,selection:oldToken,scope:{...saved.scope!,selectionToken:selection.token!}},'thread')).toBeNull()
    expect(nativeOfficeSavedSelection(dirty,selection,{...saved,selection:{...fresh,text:selection.kind==='text'?selection.text:''}},'thread')).not.toBeNull()
  })
  it('refuses incomplete save state, crossed authority or stale capture metadata',()=>{
    const variations:NativeOfficeView[]=[
      {...saved,dirty:true},{...saved,saving:true},{...saved,status:'error'},{...saved,acknowledgedSequence:1},
      {...saved,objectId:'e'.repeat(64)},{...saved,path:'/synthetic/other.docx'},{...saved,selection:undefined},
      {...saved,selection:{...fresh,version:dirty.revision}},{...saved,selection:{...fresh,changeSequence:3}},
      {...saved,selection:{...fresh,capture:undefined}},{...saved,selection:{...fresh,capture:{...fresh.capture!,complete:false}}},
      {...saved,selection:{...fresh,capture:{...fresh.capture!,truncated:true}}},
      {...saved,scope:undefined},{...saved,scope:{...saved.scope!,threadId:'other'}},{...saved,scope:{...saved.scope!,editable:true}},
      {...saved,scope:{...saved.scope!,selectionToken:selection.token!}},{...saved,scope:{...saved.scope!,baseRevision:dirty.revision}},
      {...saved,scope:{...saved.scope!,changeSequence:3}}
    ]
    for(const next of variations) expect(nativeOfficeSavedSelection(dirty,selection,next,'thread')).toBeNull()
    expect(nativeOfficeSavedSelection({...dirty,dirty:false},selection,saved,'thread')).toBeNull()
  })
  it.each([false,true])('does not admit a dirty discussion or editable draft (editable=%s)',editable=>{
    const scoped={...reference,editable,scopeId:'a'.repeat(48)}
    const current={...view,scope:{scopeId:scoped.scopeId,threadId:'thread',selectionToken:selection.token!,baseRevision:view.revision,changeSequence:2,editable,parts:[]}}
    expect(nativeOfficeDraftReferencesCurrent([scoped],[current])).toBe(true)
    expect(nativeOfficeDraftReferencesCurrent([scoped],[{...current,dirty:true}])).toBe(false)
    expect(nativeOfficeDraftReferencesCurrent([scoped],[{...current,saving:true}])).toBe(false)
    expect(nativeOfficeDraftReferencesCurrent([scoped],[{...current,status:'error'}])).toBe(false)
  })
})
