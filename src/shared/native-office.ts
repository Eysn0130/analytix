import { nativeOfficeSelectionScopeSchema, nativeOfficeProposalSchema } from '../../packages/runtime/src/contracts/native-office-editing'
import { z } from 'zod'

const index = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const digest = z.string().regex(/^[a-f0-9]{64}$/)
export const nativeWorkspaceCommandSchema = z.object({objectId:digest, command:z.enum(['toggle-workspace','toggle-terminal','close-tab'])}).strict()
export type NativeWorkspaceCommand = z.infer<typeof nativeWorkspaceCommandSchema>
export const workspaceShortcutLabels = { workspace:'Ctrl/⌘+Shift+B', terminal:'Ctrl/⌘+`' } as const
export function nativeWorkspaceCommandFromInput(input: {key:string; control?:boolean; meta?:boolean; shift?:boolean; alt?:boolean; isComposing?:boolean}): NativeWorkspaceCommand['command'] | undefined {
  if (input.isComposing || input.alt || !(input.control || input.meta)) return
  const key = input.key.toLowerCase()
  if (key === 'b' && input.shift) return 'toggle-workspace'
  if (key === '`' && !input.shift) return 'toggle-terminal'
  if (key === 'w' && !input.shift) return 'close-tab'
}
export const nativeOfficeKindSchema = z.enum(['docx', 'xlsx', 'pptx'])
export const nativeOfficeBoundsSchema = z.object({
  x: z.number().int().min(0).max(16384), y: z.number().int().min(0).max(16384),
  width: z.number().int().min(1).max(16384), height: z.number().int().min(1).max(16384)
}).strict()
export const nativeOfficeAppearanceSchema = z.object({ theme: z.enum(['light', 'dark']), reducedMotion: z.boolean() }).strict()
const localPath = z.string().min(1).max(4096).refine(value => value.trim() === value && !['\0', '\r', '\n'].some(char => value.includes(char)) && !/^[a-z][a-z0-9+.-]*:\/\//i.test(value))
export const nativeOfficePickerRequestSchema = z.object({ kind: nativeOfficeKindSchema, workspace: localPath }).strict()
export const nativeOfficePickerResponseSchema = z.union([
  z.object({ ok: z.literal(true), path: localPath.nullable() }).strict(),
  z.object({ ok: z.literal(false), error: z.enum(['invalid_request', 'unavailable']) }).strict()
])

const selectionToken = z.string().regex(/^[A-Za-z0-9_-]{8,128}$/)
const editingTarget = { objectId: digest, revision: digest, expectedChangeSequence: index }
export const nativeOfficeMenuTargetSchema = z.object(editingTarget).strict()
const actionId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
export const nativeOfficeActionMenuSchema = z.object({
  ...editingTarget,
  actions: z.array(z.object({id:actionId,label:z.string().min(1).max(120).regex(/^[^\x00-\x1f]+$/),enabled:z.boolean()}).strict()).min(1).max(32)
}).strict().refine(value => new Set(value.actions.map(action => action.id)).size === value.actions.length)
export const nativeOfficeActionChoiceSchema = z.object({actionId:actionId.nullable()}).strict()
export type NativeOfficeMenuTarget = z.infer<typeof nativeOfficeMenuTargetSchema>
export type NativeOfficeActionMenu = z.infer<typeof nativeOfficeActionMenuSchema>

/** Renderer commands contain no native bytes, URLs, engine envelopes or UNO. */
export const nativeOfficeRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('open'), workspace: localPath, path: localPath, bounds: nativeOfficeBoundsSchema, appearance: nativeOfficeAppearanceSchema.optional() }).strict(),
  z.object({ action: z.literal('bounds'), bounds: nativeOfficeBoundsSchema, appearance: nativeOfficeAppearanceSchema.optional() }).strict(),
  z.object({ action: z.literal('hide') }).strict(),
  z.object({ action: z.literal('status') }).strict(),
  z.object({ action: z.literal('captureSelection') }).strict(),
  z.object({ action: z.literal('close'), objectId: digest.optional(), discard: z.boolean().optional() }).strict(),
  z.object({ action: z.literal('annotate'), ...editingTarget }).strict(),
  z.object({ action: z.literal('undoChange'), ...editingTarget }).strict(),
  z.object({ action: z.literal('save'), ...editingTarget }).strict(),
  z.object({ action: z.literal('saveStatus'), objectId: digest }).strict(),
  z.object({ action: z.literal('reference'), ...editingTarget, selectionToken, threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/), editable: z.boolean() }).strict(),
  z.object({ action: z.literal('proposals'), objectId: digest, scopeId:z.string().regex(/^[a-f0-9]{48}$/) }).strict(),
  z.object({ action: z.literal('acceptProposal'), ...editingTarget, scopeId:z.string().regex(/^[a-f0-9]{48}$/), proposalId:z.string().regex(/^[a-f0-9]{48}$/) }).strict(),
  z.object({ action: z.literal('rejectProposal'), objectId: digest, scopeId:z.string().regex(/^[a-f0-9]{48}$/), proposalId:z.string().regex(/^[a-f0-9]{48}$/) }).strict()
])
const capture = z.object({ capturedCharacters: index, totalCharacters: index, truncated: z.boolean(), unit: z.literal('utf-16'), complete: z.boolean() }).strict()
const selectionBase = { documentId: digest, version: digest, changeSequence: index, token: selectionToken.optional(), capture: capture.optional() }
export const nativeOfficeSelectionSchema = z.discriminatedUnion('kind', [
  z.object({ ...selectionBase, kind: z.literal('text'), scope: z.enum(['current-view-text-only; no verified structural offset or durable anchor', 'session-text-range-at-version-and-change-sequence']), text: z.string().max(4096) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('cells'), scope: z.literal('sheet-range-address-at-version-and-change-sequence'), text: z.string().max(4096), cells: z.array(z.object({ sheet: index, column: index, row: index, text: z.string().max(4096), formula: z.string().max(4096), value: z.number().finite(), valueType: z.enum(['empty', 'text', 'number', 'formula']), numberFormat: index, rowVisible: z.boolean(), columnVisible: z.boolean(), merged: z.boolean() }).strict()).max(256).optional(), ranges: z.array(z.object({
    sheet: index, sheetName: z.string().max(4096), startColumn: index, startRow: index, endColumn: index, endRow: index
  }).strict().refine(value => value.endColumn >= value.startColumn && value.endRow >= value.startRow)).max(64) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('shapes'), scope: z.literal('page-and-shape-index-at-version-and-change-sequence'), shapes: z.array(z.object({
    pageIndex: index, shapeIndex: index, name: z.string().max(4096), type: z.string().max(256), text: z.string().max(4096)
  }).strict()).max(64) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('unavailable'), scope: z.literal('engine selection interface unavailable in this view') }).strict()
])
export const nativeOfficeErrorSchema = z.enum([
  'invalid_request', 'unavailable', 'package_unavailable', 'package_changed', 'invalid_response',
  'engine_unavailable', 'unsaved_changes', 'conflict', 'unknown', 'save_failed', 'stale_selection', 'unsupported_selection', 'capacity'
])
export const nativeOfficeViewSchema = z.object({
  objectId: digest, path: localPath, kind: nativeOfficeKindSchema, revision: digest,
  dirty: z.boolean(), status: z.enum(['ready', 'error', 'loading']),
  scope: nativeOfficeSelectionScopeSchema.omit({sessionId:true}).optional(), proposals:z.array(nativeOfficeProposalSchema).max(16).optional(), appliedProposals:z.array(z.string()).max(16).optional(),
  editing: z.boolean().optional(), canUndo: z.boolean().optional(), changeSequence: index.optional(), acknowledgedSequence: index.optional(), saving: z.boolean().optional(),
  selection: nativeOfficeSelectionSchema.optional(), error: nativeOfficeErrorSchema.optional()
}).strict()
export const nativeOfficeResponseSchema = z.object({
  ok: z.boolean(), view: nativeOfficeViewSchema.nullable(), error: nativeOfficeErrorSchema.optional()
}).strict()
export type NativeOfficeRequest = z.infer<typeof nativeOfficeRequestSchema>
export type NativeOfficeBounds = z.infer<typeof nativeOfficeBoundsSchema>
export type NativeOfficeView = z.infer<typeof nativeOfficeViewSchema>
export type NativeOfficeSelection = z.infer<typeof nativeOfficeSelectionSchema>
export type NativeOfficeResponse = z.infer<typeof nativeOfficeResponseSchema>
export type NativeOfficeError = z.infer<typeof nativeOfficeErrorSchema>
export type NativeOfficeKind = z.infer<typeof nativeOfficeKindSchema>

export type NativeOfficeAppearance = z.infer<typeof nativeOfficeAppearanceSchema>
