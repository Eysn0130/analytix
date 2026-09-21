import { nativeTypedPresentationSelection, nativeTypedWorkbookSelection } from '../../../shared/native-office'
import { create } from 'zustand'
import type { NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'
import type { ImageRegion } from '../../../../packages/runtime/src/contracts/object-editing'

export type OfficeNativeReference = {
  kind?: 'office'
  id: string; threadId: string | null; workspace: string; path: string
  scopeId?: string; editable?: boolean; note?: string;
  objectId: string; revision: string; label: string; text: string; selection: NativeOfficeSelection
}
export type CanvasNativeReference = {
  kind: 'canvas'; id: string; threadId: string; workspace: string; path: string
  objectId: string; revision: string; label: string; text: ''
  sessionId: string; scopeId?: string; editable?: boolean; selectedIds: string[]
}
export type ImageRegionNativeReference = {
  kind: 'image-region'; id: string; threadId: string; workspace: string; path: string
  objectId: string; revision: string; label: string; text: ''
  sessionId: string; scopeId?: string; editable: false
  annotationRevision: string; width: number; height: number; region: ImageRegion
}
export type NativeReference = OfficeNativeReference | CanvasNativeReference | ImageRegionNativeReference
export type NativeReferenceInput = Omit<OfficeNativeReference, 'id'> | Omit<CanvasNativeReference, 'id'> | Omit<ImageRegionNativeReference, 'id'>
const column = (n: number): string => { let label = ''; for (n++; n; n = Math.floor((n - 1) / 26)) label = String.fromCharCode(65 + (n - 1) % 26) + label; return label }
export function nativeSelectionLabel(view: NativeOfficeView, selection: NativeOfficeSelection): string {
  const name = view.path.split(/[\\/]/).at(-1) ?? view.kind
  if (selection.kind === 'cells') return `${name} · ${selection.ranges.map(r => `${r.sheetName}!${column(r.startColumn)}${r.startRow + 1}:${column(r.endColumn)}${r.endRow + 1}`).join('；')}`
  if (selection.kind === 'shapes') return `${name} · ${selection.shapes.map(s => `第${s.pageIndex + 1}页 · ${s.name || '文本框'}`).join('；')}`
  return `${name} · 选中文本`
}
export function nativeSelectionText(selection: NativeOfficeSelection): string {
  if (selection.kind === 'unavailable') return ''
  const text = selection.kind === 'shapes' ? selection.shapes.map(s => s.text.trim() ? s.text : `第${s.pageIndex + 1}页 · 形状${s.shapeIndex + 1} · ${s.name || s.type}（未捕获文本，仅对象标注）`).join('\n')
    : selection.kind === 'cells' && selection.cells ? selection.cells.map(c => `${column(c.column)}${c.row + 1}: ${c.text}${c.valueType === 'formula' ? ` [公式 ${c.formula}]` : ''}${!c.rowVisible || !c.columnVisible ? ' [隐藏行/列]' : ''}${c.merged ? ' [合并单元格]' : ''}`).join('\n')
    : selection.text
  const clipped = text.slice(0, 4096).replace(/[\uD800-\uDBFF]$/, '')
  return text.length > clipped.length ? `${clipped}\n[引用内容已截断]` : text
}
/** Only complete current text targets or bounded workbook rectangles can request a Core edit scope. */
export function isNativeSelectionEditable(view: NativeOfficeView, selection: NativeOfficeSelection): boolean {
  if (!view.editing || !selection.token || !selection.capture?.complete || selection.capture.truncated ||
    selection.documentId !== view.objectId || selection.version !== view.revision ||
    selection.changeSequence !== (view.changeSequence ?? 0)) return false
  if (nativeTypedPresentationSelection(selection) || nativeTypedWorkbookSelection(selection)) return true
  if (!nativeSelectionText(selection).trim()) return false
  if (selection.kind === 'text') return selection.scope === 'session-text-range-at-version-and-change-sequence'
  if (selection.kind === 'shapes') return selection.shapes.length === 1 && !!selection.shapes[0].text.trim()
  if (selection.kind !== 'cells' || selection.ranges.length !== 1 || selection.cells?.length !== 1) return false
  const range = selection.ranges[0], cell = selection.cells[0]
  return !cell.merged && (cell.valueType === 'text' || cell.valueType === 'empty') && range.startColumn === range.endColumn && range.startRow === range.endRow &&
    range.sheet === cell.sheet && range.startColumn === cell.column && range.startRow === cell.row && !!cell.text.trim()
}
/** Recheck an explicit task after asynchronous preparation; the Core still owns authority. */
export function nativeActionReferencesCurrent(references: readonly NativeReference[], views: readonly NativeOfficeView[]): boolean {
  return references.length > 0 && references.every(reference => {
    if (reference.kind === 'canvas' || reference.kind === 'image-region') return false // Send-time Core validation.
    const view = views.find(candidate => candidate.objectId === reference.objectId)
    return !!view && view.revision === reference.revision &&
      (view.changeSequence ?? 0) === reference.selection.changeSequence &&
      (!reference.editable || view.scope?.scopeId === reference.scopeId && view.scope?.threadId === reference.threadId)
  })
}
/** Frozen local snapshots. Office may retain discussion-only text; Canvas always
 * requires a current Core scope and never falls back to copied source content. */
export const useNativeReferenceStore = create<{
  references: NativeReference[]
  drafts: Record<string, { note: string; selection?: NativeOfficeSelection; dirty?: boolean; sourceRevision?: string }>
  setDraft: (key: string, draft: { note: string; selection?: NativeOfficeSelection; dirty?: boolean; sourceRevision?: string }) => void
  revokeScopes: (objectId: string) => void
  add: (reference: NativeReferenceInput) => NativeReference
  remove: (id: string) => void
}>((set) => ({
  references: [],
  drafts: {},
  setDraft: (key, draft) => set(state => ({drafts:{...state.drafts,[key]:draft}})),
  revokeScopes: objectId => set(state => ({references:state.references.map(previous => {
    if (!previous.scopeId || previous.objectId !== objectId) return previous
    const {scopeId: _scopeId, ...snapshot} = previous
    return {...snapshot,editable:false as const}
  })})),
  add: reference => {
    const snapshot: NativeReference = {...structuredClone(reference), id:crypto.randomUUID()}
    set(state => ({references:[...state.references.filter(previous => !((reference.kind === 'canvas' || reference.kind === 'image-region') && previous.kind === reference.kind && previous.objectId === reference.objectId && previous.threadId === reference.threadId)).slice(-7).map(previous => {
    if (!reference.scopeId || !previous.scopeId || previous.objectId !== reference.objectId || previous.threadId !== reference.threadId) return previous
    const {scopeId: _scopeId, ...snapshot} = previous
    return {...snapshot, editable:false as const}
  }), snapshot]}))
    return snapshot
  },
  remove: id => set(state => ({references:state.references.filter(r => r.id !== id)}))
}))
export function nativeReferencesPrompt(references: NativeReference[]): string {
  return references.map(r => {
    if (r.kind === 'image-region') {
      if (!r.scopeId || !/^[a-f0-9]{48}$/.test(r.scopeId)) throw new Error('Image region must be recaptured before sending.')
      // Geometry, notes and pixels are never trusted from the composer snapshot.
      return `[Image region reference]\nCore scopeId: ${r.scopeId}\nUse native_selection_read for the current region and projected note. This is discussion-only. No image pixels have been supplied; do not claim to see, redact or modify the image.`
    }
    if (r.kind === 'canvas') {
      if (!r.scopeId || !r.editable || !/^[a-f0-9]{48}$/.test(r.scopeId)) throw new Error('Canvas selection must be recaptured before sending.')
      // Never use the Office snapshot fallback for a Canvas quote. Raw Scene,
      // path, labels, stable IDs and copied notes stay on the local display lane.
      return `[Canvas selection]\nCore scopeId: ${r.scopeId}\nUse native_selection_read for this scope, then native_selection_propose with the canvas payload and selected opaque aliases. Preserve facts and unrelated objects. The proposal requires explicit user acceptance; it is not a saved edit.`
    }
    const version = `版本: ${r.revision}；修改序列: ${r.selection.changeSequence}${r.selection.capture?.truncated ? '；部分捕获，不能视为完整选区' : ''}`
    const note = r.note?.trim() ? `\n[用户标注备注]\n${r.note.trim()}\n[/用户标注备注]` : ''
    if (r.scopeId && r.editable) return `[原生文档选区] ${r.label}\n${version}\nCore scopeId: ${r.scopeId}\n用户请求局部修改时，使用 native_selection_read 读取此 scope，再使用 native_selection_propose 提交提案：typed workbook scope 使用显式 number/formula/range 与矩形内相对坐标，presentation 属性修改使用 shape-geometry 或 shape-fill，文本修改使用受保护 parts，等待用户接受。不能通过文件工具修改整个文件，不猜测选区，不将提案描述为已应用。${note}`
    return `[原生文档引用] ${r.label}\n${version}\n以下是引用快照，只作讨论，不授予文件写权限；此选区不支持直接应用修改。\n[引用原文]\n${r.text}\n[/引用原文]${note}`
  }).join('\n\n')
}
