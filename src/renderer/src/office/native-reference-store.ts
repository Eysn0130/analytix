import { create } from 'zustand'
import type { NativeOfficeSelection, NativeOfficeView } from '@shared/native-office'

export type NativeReference = {
  id: string; threadId: string | null; workspace: string; path: string
  scopeId?: string; editable?: boolean; note?: string;
  objectId: string; revision: string; label: string; text: string; selection: NativeOfficeSelection
}
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
/** Only complete, version-bound single targets can request an editable Core scope. */
export function isNativeSelectionEditable(view: NativeOfficeView, selection: NativeOfficeSelection): boolean {
  if (!view.editing || !selection.token || !selection.capture?.complete || selection.capture.truncated ||
    selection.documentId !== view.objectId || selection.version !== view.revision ||
    selection.changeSequence !== (view.changeSequence ?? 0) || !nativeSelectionText(selection).trim()) return false
  if (selection.kind === 'text') return selection.scope === 'session-text-range-at-version-and-change-sequence'
  if (selection.kind === 'shapes') return selection.shapes.length === 1 && !!selection.shapes[0].text.trim()
  if (selection.kind !== 'cells' || selection.ranges.length !== 1 || selection.cells?.length !== 1) return false
  const range = selection.ranges[0], cell = selection.cells[0]
  return !cell.merged && range.startColumn === range.endColumn && range.startRow === range.endRow &&
    range.sheet === cell.sheet && range.startColumn === cell.column && range.startRow === cell.row && !!cell.text.trim()
}
/** Frozen local snapshots. A quote remains a discussion reference after switching tabs. */
export const useNativeReferenceStore = create<{
  references: NativeReference[]
  add: (reference: Omit<NativeReference, 'id'>) => void
  remove: (id: string) => void
}>((set) => ({
  references: [],
  add: reference => set(state => ({references:[...state.references.slice(-7).map(previous => {
    if (!reference.scopeId || !previous.scopeId || previous.objectId !== reference.objectId || previous.threadId !== reference.threadId) return previous
    const {scopeId: _scopeId, ...snapshot} = previous
    return {...snapshot, editable:false}
  }), {...reference, id:crypto.randomUUID()}]})),
  remove: id => set(state => ({references:state.references.filter(r => r.id !== id)}))
}))
export function nativeReferencesPrompt(references: NativeReference[]): string {
  return references.map(r => {
    const version = `版本: ${r.revision}；修改序列: ${r.selection.changeSequence}${r.selection.capture?.truncated ? '；部分捕获，不能视为完整选区' : ''}`
    const note = r.note?.trim() ? `\n[用户标注备注]\n${r.note.trim()}\n[/用户标注备注]` : ''
    if (r.scopeId && r.editable) return `[原生文档选区] ${r.label}\n${version}\nCore scopeId: ${r.scopeId}\n用户请求局部修改时，使用 native_selection_read 读取此 scope，再使用 native_selection_propose 提交受保护 parts 提案，等待用户接受。不能通过文件工具修改整个文件，不猜测选区，不将提案描述为已应用。${note}`
    return `[原生文档引用] ${r.label}\n${version}\n以下是引用快照，只作讨论，不授予文件写权限；此选区不支持直接应用修改。\n[引用原文]\n${r.text}\n[/引用原文]${note}`
  }).join('\n\n')
}
