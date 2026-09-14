// Browser-safe typed transport shared by Main and the sandboxed Office preload.
export const OFFICE_BIND_IPC = 'analytix-office-surface-bind-v1'
export const OFFICE_MAX_BYTES = 16 * 1024 * 1024
export const officeToken = (v: unknown): v is string => typeof v === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(v)
export type OfficeKind = 'docx' | 'xlsx' | 'pptx'
// Legacy type spellings remain compile-compatible; runtime validators below are read-only.
export type OfficeCommand = 'open' | 'export' | 'ack' | 'captureSelection' | 'bold' | 'undo' | 'redo' | 'close'
export type OfficeState = { documentId: string; version: string; kind: OfficeKind; changeSequence: number; acknowledgedSequence: number; dirty: boolean }
export type OfficeEnvelope = { command: OfficeCommand; operationId: string; documentId: string; version: string; channel: string }
export type OfficeRequest = OfficeEnvelope & { kind?: OfficeKind; bytes?: Uint8Array; status?: 'committed' | 'conflict' | 'failed'; persistedVersion?: string; expectedChangeSequence?: number; discard?: boolean }
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
  'operation-in-progress', 'operation-replayed', 'session-operation-limit',
  'engine-timeout-state-unknown', 'surface-state-unknown-recreate-required'
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
  return v.command === 'captureSelection' && exact(v, base)
}
export function isOfficeState(v: unknown): v is OfficeState {
  return officeRecord(v) && exact(v, ['documentId', 'version', 'kind', 'changeSequence', 'acknowledgedSequence', 'dirty']) && officeToken(v.documentId) && officeToken(v.version) && kinds.includes(String(v.kind)) && integer(v.changeSequence) && integer(v.acknowledgedSequence) && v.acknowledgedSequence <= v.changeSequence && v.dirty === (v.changeSequence !== v.acknowledgedSequence)
}
function selection(v: unknown): v is Record<string, unknown> {
  if (!officeRecord(v) || !officeToken(v.documentId) || !officeToken(v.version) || !integer(v.changeSequence) || !text(v.scope, 160)) return false
  const fields = ['documentId', 'version', 'changeSequence', 'kind', 'scope']
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
  if (!officeRecord(v) || v.type !== 'result' || !identity(v) || !['open', 'captureSelection', 'close'].includes(String(v.command))) return false
  const fields = [...base, 'type', 'ok']
  if (v.ok === false) return exact(v, [...fields, 'error']) && typeof v.error === 'string' && officeCodes.has(v.error)
  if (v.ok !== true) return false
  if (v.command === 'close') return exact(v, fields)
  if (!isOfficeState(v.state) || v.state.documentId !== v.documentId) return false
  return exact(v, [...fields, 'state', 'selection']) && selection(v.selection) && v.selection.documentId === v.documentId && v.selection.version === v.state.version && v.selection.changeSequence === v.state.changeSequence
}
export function isOfficeEvent(v: unknown): v is OfficeEvent {
  if (!officeRecord(v) || !identity(v)) return false
  const fields = ['type', 'channel', 'operationId', 'documentId', 'version']
  if (v.type === 'changed') return exact(v, [...fields, 'state']) && isOfficeState(v.state) && v.state.documentId === v.documentId && v.state.version === v.version
  return v.type === 'selection' && exact(v, [...fields, 'selection']) && selection(v.selection) && v.selection.documentId === v.documentId && v.selection.version === v.version
}
export function sameOfficeEnvelope(a: OfficeEnvelope, b: OfficeEnvelope): boolean {
  return base.every(k => a[k as keyof OfficeEnvelope] === b[k as keyof OfficeEnvelope])
}
