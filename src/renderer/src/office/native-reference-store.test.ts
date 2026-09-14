import { describe, expect, it } from 'vitest'
import type { NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import { isNativeSelectionEditable, nativeReferencesPrompt, useNativeReferenceStore, type NativeReference } from './native-reference-store'

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
  it('does not grant a scope for multi-cell, merged or multi-shape selections', () => {
    const cells: NativeOfficeSelection = { ...selection, kind: 'cells', scope: 'sheet-range-address-at-version-and-change-sequence', text: 'Value', ranges: [{sheet:0,sheetName:'Sheet1',startColumn:0,endColumn:0,startRow:0,endRow:0}], cells:[{sheet:0,column:0,row:0,text:'Value',formula:'',value:0,valueType:'text',numberFormat:0,rowVisible:true,columnVisible:true,merged:false}] }
    expect(isNativeSelectionEditable(view, cells)).toBe(true)
    expect(isNativeSelectionEditable(view, { ...cells, ranges:[{...cells.ranges[0],endRow:1}] })).toBe(false)
    expect(isNativeSelectionEditable(view, { ...cells, cells:[{...cells.cells![0],merged:true}] })).toBe(false)
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
