import { describe, expect, it } from 'vitest'
import type { NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import { isNativeSelectionEditable, nativeActionReferencesCurrent, nativeReferencesPrompt, useNativeReferenceStore, type ImageRegionNativeReference, type NativeReference } from './native-reference-store'

const view: NativeOfficeView = { objectId: 'a'.repeat(64), revision: 'b'.repeat(64), path: '/synthetic/report.docx', kind: 'docx', status: 'ready', dirty: false, editing: true, changeSequence: 2 }
const selection: NativeOfficeSelection = { kind: 'text', documentId: view.objectId, version: view.revision, changeSequence: 2, token: 'selection-token', scope: 'session-text-range-at-version-and-change-sequence', text: 'Synthetic text', capture: { capturedCharacters: 14, totalCharacters: 14, unit: 'utf-16', truncated: false, complete: true } }
const reference: NativeReference = { id: 'ref', threadId: 'thread', workspace: '/synthetic', path: view.path, objectId: view.objectId, revision: view.revision, selection, label: 'report.docx · 选中文本', text: selection.text, note: '请澄清这句话' }

describe('native annotation references', () => {
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
    const shape = {pageIndex:0,shapeIndex:0,name:'Text',type:'text',text:'Value'}
    const shapes: NativeOfficeSelection = { ...selection, kind:'shapes',scope:'page-and-shape-index-at-version-and-change-sequence',shapes:[shape] }
    expect(isNativeSelectionEditable(view, shapes)).toBe(true)
    expect(isNativeSelectionEditable(view, {...shapes,shapes:[shape,{...shape,shapeIndex:1}]})).toBe(false)
  })
  it('keeps note and version with editable references without duplicating private selection text', () => {
    const prompt = nativeReferencesPrompt([{ ...reference, editable:true, scopeId:'s'.repeat(48) }])
    expect(prompt).toContain(reference.note)
    expect(prompt).toContain(view.revision)
    expect(prompt).toContain('修改序列: 2')
    expect(prompt).toContain('native_selection_propose')
    expect(prompt).not.toContain(selection.text)
  })
  it('marks unsupported references as discussion-only even if a noneditable scope exists', () => {
    const prompt = nativeReferencesPrompt([{ ...reference, editable:false, scopeId:'s'.repeat(48) }])
    expect(prompt).toContain('此选区不支持直接应用修改')
    expect(prompt).toContain(reference.note)
    expect(prompt).not.toContain('native_selection_propose')
  })
})

it('rechecks the native task revision, sequence and scoped thread after asynchronous message preparation', () => {
  const scoped = {...reference,editable:true,scopeId:'a'.repeat(48)}
  const current = {...view,scope:{scopeId:scoped.scopeId,threadId:reference.threadId} as NativeOfficeView['scope']}
  expect(nativeActionReferencesCurrent([scoped],[current])).toBe(true)
  expect(nativeActionReferencesCurrent([scoped],[{...current,revision:'c'.repeat(64)}])).toBe(false)
  expect(nativeActionReferencesCurrent([scoped],[{...current,changeSequence:3}])).toBe(false)
  expect(nativeActionReferencesCurrent([scoped],[{...current,scope:undefined}])).toBe(false)
  expect(nativeActionReferencesCurrent([{...scoped,threadId:'another'}],[current])).toBe(false)
  expect(nativeActionReferencesCurrent([scoped],[])).toBe(false)
})


it('downgrades revoked prior scopes for the same object and thread without changing their frozen annotation', () => {
  const prior = {...reference,scopeId:'a'.repeat(48),editable:true}
  const otherThread = {...prior,id:'other-thread',threadId:'other-thread'}
  const otherObject = {...prior,id:'other-object',objectId:'c'.repeat(64)}
  useNativeReferenceStore.setState({references:[prior,otherThread,otherObject]})
  useNativeReferenceStore.getState().add({...reference,scopeId:'b'.repeat(48),editable:true,note:'新标注'})
  const references = useNativeReferenceStore.getState().references
  expect(references[0]).toMatchObject({editable:false,text:prior.text,note:prior.note,selection:prior.selection})
  expect(references[0].scopeId).toBeUndefined()
  expect(references[1]).toEqual(otherThread)
  expect(references[2]).toEqual(otherObject)
  expect(references[3]).toMatchObject({editable:true,scopeId:'b'.repeat(48),note:'新标注'})
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
