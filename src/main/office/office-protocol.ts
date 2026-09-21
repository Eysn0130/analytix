import type { NativePresentationSelection, NativePresentationReview, NativeWorkbookReview } from '../../../packages/runtime/src/contracts/native-office-editing'
// Browser-safe typed transport shared by Main and the sandboxed Office preload.
export const OFFICE_BIND_IPC = 'analytix-office-surface-bind-v1'
export const OFFICE_MAX_BYTES = 16 * 1024 * 1024
export const officeToken = (v: unknown): v is string => typeof v === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(v)
export type OfficeKind = 'docx' | 'xlsx' | 'pptx'
// Fixed operations only; no arbitrary UNO, URLs, paths or script dispatch.
export type OfficeCommand = 'open' | 'edit' | 'replace' | 'replaceCells' | 'replacePresentation' | 'export' | 'ack' | 'captureSelection' | 'close'
export type OfficeState = { documentId: string; version: string; kind: OfficeKind; changeSequence: number; acknowledgedSequence: number; dirty: boolean }
export type OfficeEnvelope = { command: OfficeCommand; operationId: string; documentId: string; version: string; channel: string }
export type OfficeRequest = OfficeEnvelope & { kind?: OfficeKind; bytes?: Uint8Array; status?: 'committed' | 'conflict' | 'failed'; persistedVersion?: string; expectedChangeSequence?: number; discard?: boolean; selectionToken?: string; text?: string; valueType?: 'text'; workbook?: NativeWorkbookReview; presentation?: NativePresentationReview; exportOperationId?: string; exportedSequence?: number }
export type OfficeRequestInput = Omit<OfficeRequest, 'channel'>
export type OfficeResult = OfficeEnvelope & { type: 'result'; ok: boolean; state?: OfficeState; selection?: Record<string, unknown>; bytes?: Uint8Array; exportedSequence?: number; error?: string }
export type OfficeEvent = { type: 'changed' | 'selection' | 'save-requested'; channel: string; operationId: string; documentId: string; version: string; state?: OfficeState; selection?: Record<string, unknown> }
export type OfficeReady = { type: 'ready'; channel: string; protocolVersion: 1 }
export type OfficeEngineFailure = { type: 'fatal'; channel: string; error: 'engine-load-failed' | 'engine-operation-failed' }
export type OfficeBinding = { type: 'bind'; channel: string }
export const officeCodes = new Set([
  'invalid-request', 'already-bound', 'engine-operation-failed', 'engine-load-failed',
  'document-already-open', 'open-failed', 'stale-document-version',
  'unsaved-changes', 'command-unavailable', 'unsupported-command', 'native-controls-hide-failed',
  'operation-in-progress', 'stale-selection', 'unsupported-selection', 'export-too-large', 'export-awaiting-ack', 'ack-mismatch', 'invalid-control-value', 'operation-replayed', 'session-operation-limit',
  'typed-mutation-failed', 'engine-timeout-state-unknown', 'surface-state-unknown-recreate-required'
])
const kinds = ['docx', 'xlsx', 'pptx']
export function officeRecord(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && Object.getPrototypeOf(v) === Object.prototype
}
const exact = (r: Record<string, unknown>, required: string[], optional: string[] = []) => required.every(k => Object.hasOwn(r, k)) && Object.keys(r).every(k => required.includes(k) || optional.includes(k))
const integer = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0
const text = (v: unknown, max = 4096): v is string => typeof v === 'string' && v.length <= max
const bytes = (v: unknown): v is Uint8Array => v instanceof Uint8Array && v.buffer instanceof ArrayBuffer && v.byteLength > 0 && v.byteLength <= OFFICE_MAX_BYTES
const base = ['command', 'operationId', 'documentId', 'version', 'channel']
const identity = (r: Record<string, unknown>) => ['operationId', 'documentId', 'version', 'channel'].every(k => officeToken(r[k]))
export function isOfficeRequest(v: unknown): v is OfficeRequest {
  if (!officeRecord(v) || !identity(v)) return false
  if (v.command === 'open') return exact(v, [...base, 'kind', 'bytes']) && kinds.includes(String(v.kind)) && bytes(v.bytes)
  if (v.command === 'close') return exact(v, [...base, 'expectedChangeSequence', 'discard']) && integer(v.expectedChangeSequence) && typeof v.discard === 'boolean'
  if (['captureSelection', 'edit', 'export'].includes(String(v.command))) return exact(v, base)
  if (v.command === 'replacePresentation') return exact(v, [...base,'selectionToken','expectedChangeSequence','presentation']) && officeToken(v.selectionToken) && integer(v.expectedChangeSequence) && isOfficePresentationReview(v.presentation)
  if (v.command === 'replaceCells') return exact(v, [...base,'selectionToken','expectedChangeSequence','workbook']) && officeToken(v.selectionToken) && integer(v.expectedChangeSequence) && isOfficeWorkbookReview(v.workbook)
  if (v.command === 'replace') return exact(v, [...base, 'selectionToken', 'expectedChangeSequence', 'text', 'valueType']) && officeToken(v.selectionToken) && integer(v.expectedChangeSequence) && text(v.text) && v.valueType === 'text'
  return v.command === 'ack' && exact(v, [...base, 'status', 'exportOperationId', 'exportedSequence'], ['persistedVersion']) && ['committed', 'conflict', 'failed'].includes(String(v.status)) && officeToken(v.exportOperationId) && integer(v.exportedSequence) && (v.status === 'committed' ? officeToken(v.persistedVersion) : v.persistedVersion === undefined)
}
export function isOfficeState(v: unknown): v is OfficeState {
  return officeRecord(v) && exact(v, ['documentId', 'version', 'kind', 'changeSequence', 'acknowledgedSequence', 'dirty']) && officeToken(v.documentId) && officeToken(v.version) && kinds.includes(String(v.kind)) && integer(v.changeSequence) && integer(v.acknowledgedSequence) && v.acknowledgedSequence <= v.changeSequence && v.dirty === (v.changeSequence !== v.acknowledgedSequence)
}
function selection(v: unknown): v is Record<string, unknown> {
  if (!officeRecord(v) || !officeToken(v.documentId) || !officeToken(v.version) || !integer(v.changeSequence) || !text(v.scope, 160)) return false
  const fields = ['documentId', 'version', 'changeSequence', 'kind', 'scope']
  if (v.token !== undefined) { if (!officeToken(v.token)) return false; fields.push('token') }
  if (v.capture !== undefined) {
    const c = v.capture
    if (!officeRecord(c) || !exact(c, ['capturedCharacters', 'totalCharacters', 'truncated', 'unit', 'complete']) || !integer(c.capturedCharacters) || !integer(c.totalCharacters) || c.capturedCharacters > c.totalCharacters || typeof c.truncated !== 'boolean' || typeof c.complete !== 'boolean' || c.unit !== 'utf-16') return false
    fields.push('capture')
  }
  if (v.presentation !== undefined) {
    if (v.kind !== 'shapes' || !isOfficePresentationSelection(v.presentation)) return false
    fields.push('presentation')
  }
  if (v.cells !== undefined) {
    if (v.kind !== 'cells' || !Array.isArray(v.cells) || v.cells.length > 256 || !v.cells.every(c => officeRecord(c) && exact(c, ['sheet','column','row','text','formula','value','valueType','numberFormat','rowVisible','columnVisible','merged']) && ['sheet','column','row','numberFormat'].every(k => integer(c[k])) && text(c.text) && text(c.formula) && Number.isFinite(c.value) && ['empty','text','number','formula'].includes(String(c.valueType)) && ['rowVisible','columnVisible','merged'].every(k => typeof c[k] === 'boolean'))) return false
    fields.push('cells')
  }
  if (v.kind === 'text') return exact(v, [...fields, 'text']) && text(v.text)
  if (v.kind === 'unavailable') return exact(v, fields)
  if (v.kind === 'cells') return exact(v, [...fields, 'ranges', 'text']) && text(v.text) && Array.isArray(v.ranges) && v.ranges.length <= 64 && v.ranges.every(a => officeRecord(a) && exact(a, ['sheet', 'sheetName', 'startColumn', 'startRow', 'endColumn', 'endRow']) && text(a.sheetName, 256) && ['sheet', 'startColumn', 'startRow', 'endColumn', 'endRow'].every(k => integer(a[k])) && Number(a.startColumn) <= Number(a.endColumn) && Number(a.startRow) <= Number(a.endRow))
  return v.kind === 'shapes' && exact(v, [...fields, 'shapes']) && Array.isArray(v.shapes) && v.shapes.length <= 64 && v.shapes.every(a => officeRecord(a) && exact(a, ['pageIndex', 'shapeIndex', 'name', 'type', 'text']) && integer(a.pageIndex) && integer(a.shapeIndex) && text(a.name) && text(a.type, 256) && text(a.text))
}
export function isOfficeEngineFailure(v: unknown): v is OfficeEngineFailure {
  return officeRecord(v) && exact(v, ['type', 'channel', 'error']) && v.type === 'fatal' && officeToken(v.channel) && ['engine-load-failed', 'engine-operation-failed'].includes(String(v.error))
}
export function isOfficeReady(v: unknown): v is OfficeReady {
  return officeRecord(v) && exact(v, ['type', 'channel', 'protocolVersion']) && v.type === 'ready' && officeToken(v.channel) && v.protocolVersion === 1
}
export function isOfficeResult(v: unknown): v is OfficeResult {
  if (!officeRecord(v) || v.type !== 'result' || !identity(v) || !['open', 'edit', 'replace', 'replaceCells', 'replacePresentation', 'export', 'ack', 'captureSelection', 'close'].includes(String(v.command))) return false
  const fields = [...base, 'type', 'ok']
  if (v.ok === false) return exact(v, [...fields, 'error']) && typeof v.error === 'string' && officeCodes.has(v.error)
  if (v.ok !== true) return false
  if (v.command === 'close') return exact(v, fields)
  if (!isOfficeState(v.state) || v.state.documentId !== v.documentId) return false
  if (v.command === 'export') return exact(v, [...fields, 'state', 'bytes', 'exportedSequence']) && bytes(v.bytes) && integer(v.exportedSequence) && v.exportedSequence <= v.state.changeSequence
  return exact(v, [...fields, 'state', 'selection']) && selection(v.selection) && v.selection.documentId === v.documentId && v.selection.version === v.state.version && v.selection.changeSequence === v.state.changeSequence
}
export function isOfficeEvent(v: unknown): v is OfficeEvent {
  if (!officeRecord(v) || !identity(v)) return false
  const fields = ['type', 'channel', 'operationId', 'documentId', 'version']
  if (v.type === 'save-requested') return exact(v, fields)
  if (v.type === 'changed') return exact(v, [...fields, 'state']) && isOfficeState(v.state) && v.state.documentId === v.documentId && v.state.version === v.version
  return v.type === 'selection' && exact(v, [...fields, 'selection']) && selection(v.selection) && v.selection.documentId === v.documentId && v.selection.version === v.version
}
export function sameOfficeEnvelope(a: OfficeEnvelope, b: OfficeEnvelope): boolean {
  return base.every(k => a[k as keyof OfficeEnvelope] === b[k as keyof OfficeEnvelope])
}

export function isOfficeWorkbookReview(v: unknown): v is NativeWorkbookReview {
  if (!officeRecord(v) || !exact(v,['before','after','results']) || !Array.isArray(v.results) || v.results.length > 256 || !v.results.every(r => text(r,65536))) return false
  const snapshot = (s:unknown): boolean => {
    if (!officeRecord(s) || !exact(s,['sheet','sheetName','startColumn','startRow','endColumn','endRow','cells']) || !['sheet','startColumn','startRow','endColumn','endRow'].every(k => integer(s[k])) || !text(s.sheetName,31) || !Array.isArray(s.cells)) return false
    const a=s as unknown as NativeWorkbookReview['before'], width=a.endColumn-a.startColumn+1, height=a.endRow-a.startRow+1
    return a.sheet < 20 && a.sheetName.length > 0 && a.endColumn < 1024 && a.endRow < 100000 && width > 0 && height > 0 && width*height === a.cells.length && a.cells.length <= 256 && a.cells.every((c,i) => officeRecord(c) && exact(c,['sheet','column','row','text','formula','value','valueType','numberFormat','rowVisible','columnVisible','merged']) && c.sheet === a.sheet && c.column === a.startColumn+i%width && c.row === a.startRow+Math.floor(i/width) && text(c.text) && text(c.formula) && Number.isFinite(c.value) && integer(c.numberFormat) && c.rowVisible === true && c.columnVisible === true && c.merged === false && ['empty','text','number','formula'].includes(c.valueType))
  }
  if (!snapshot(v.before) || !snapshot(v.after)) return false
  const r=v as unknown as NativeWorkbookReview
  return ['sheet','sheetName','startColumn','startRow','endColumn','endRow'].every(k => r.before[k as keyof typeof r.before] === r.after[k as keyof typeof r.after]) && r.results.length === r.after.cells.length
}

export function isOfficePresentationSelection(value: unknown): value is NativePresentationSelection {
  if (!officeRecord(value) || !exact(value,['pageIndex','targetShapeIndex','pageWidth100thMm','pageHeight100thMm','shapes']) || !Array.isArray(value.shapes)) return false
  const s = value as unknown as NativePresentationSelection
  const dimension = (n: unknown, min: number, max: number): boolean => integer(n) && n >= min && n <= max
  const label = (v: unknown): v is string => text(v) && !v.includes('\0') && !/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(v) && new TextEncoder().encode(v).length <= 4096
  if (!dimension(s.pageIndex,0,511) || !dimension(s.pageWidth100thMm,1,1000000) || !dimension(s.pageHeight100thMm,1,1000000) || !dimension(s.shapes.length,1,64) || !dimension(s.targetShapeIndex,0,s.shapes.length-1)) return false
  let total = 0
  return s.shapes.every((shape,i) => {
    if (!officeRecord(shape) || !exact(shape,['shapeIndex','kind','name','text','x100thMm','y100thMm','width100thMm','height100thMm','fillRGB']) || shape.shapeIndex !== i || !['rectangle','ellipse','text'].includes(shape.kind) || !label(shape.name) || !label(shape.text) || typeof shape.fillRGB !== 'string' || !/^#[0-9a-f]{6}$/.test(shape.fillRGB) || !dimension(shape.x100thMm,0,s.pageWidth100thMm) || !dimension(shape.y100thMm,0,s.pageHeight100thMm) || !dimension(shape.width100thMm,1,s.pageWidth100thMm-shape.x100thMm) || !dimension(shape.height100thMm,1,s.pageHeight100thMm-shape.y100thMm)) return false
    total += new TextEncoder().encode(shape.name + shape.text).length
    return total <= 65536
  })
}
export function isOfficePresentationReview(value: unknown): value is NativePresentationReview {
  if (!officeRecord(value) || !exact(value,['before','after']) || !isOfficePresentationSelection(value.before) || !isOfficePresentationSelection(value.after)) return false
  const a=value.before,b=value.after,expected=structuredClone(a),old=a.shapes[a.targetShapeIndex],next=b.shapes[b.targetShapeIndex]
  const target=expected.shapes[a.targetShapeIndex]
  if (old.fillRGB !== next.fillRGB) target.fillRGB=next.fillRGB
  else for (const key of ['x100thMm','y100thMm','width100thMm','height100thMm'] as const) target[key]=next[key]
  const equal=(x:NativePresentationSelection,y:NativePresentationSelection):boolean => x.pageIndex===y.pageIndex && x.targetShapeIndex===y.targetShapeIndex && x.pageWidth100thMm===y.pageWidth100thMm && x.pageHeight100thMm===y.pageHeight100thMm && x.shapes.length===y.shapes.length && x.shapes.every((shape,i)=>Object.keys(shape).every(k=>shape[k as keyof typeof shape]===y.shapes[i][k as keyof typeof shape]))
  return !equal(a,b) && equal(expected,b)
}
