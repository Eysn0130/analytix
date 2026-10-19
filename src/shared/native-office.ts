import { nativePresentationSelectionSchema, nativeWorkbookSelectionSchema, nativeWorkbookManualEditSchema, nativeOfficeSelectionScopeSchema, nativeOfficeProposalSchema, nativeOfficeLocalReviewSchema, nativeOfficeRecoverySchema, nativeOfficeAnnotationSchema, nativeOfficeAnnotationNoteSchema, nativeOfficeCreateInputSchema, nativeOfficeCreationSchema, nativeOfficeManualAuthoritySchema } from '../../packages/runtime/src/contracts/native-office-editing'
import { z } from 'zod'
import { toolResultLocalPresentFileRequestSchemaV1 } from '../../packages/runtime/src/contracts/tool-result-local-display'

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
/** Worker-owned native endpoints, invalidated by text/selection/generation changes. */
export const nativeOfficeRangeLeaseSchema = z.object({ generation: index, sequence: index, token: selectionToken }).strict()
const nativeFontFamily = z.string().min(1).max(128).refine(value => value.trim() === value && !/\p{Cc}/u.test(value))
const nativeFontSize = z.number().finite().min(6).max(96).multipleOf(0.5)
/** This closed union is the complete formatting authority; no UNO/property dispatch. */
export const nativeOfficeFormatSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('font-family'), value: nativeFontFamily }).strict(),
  z.object({ kind: z.literal('font-size'), value: nativeFontSize }).strict(),
  z.object({ kind: z.literal('bold'), value: z.boolean() }).strict(),
  z.object({ kind: z.literal('italic'), value: z.boolean() }).strict(),
  z.object({ kind: z.literal('underline'), value: z.boolean() }).strict()
])
// Null means mixed or unavailable native properties, never a guessed default.
export const nativeOfficeFormatStateSchema = z.object({
  fontFamily: nativeFontFamily.nullable(), fontSize: nativeFontSize.nullable(),
  bold: z.boolean().nullable(), italic: z.boolean().nullable(), underline: z.boolean().nullable()
}).strict()
export const nativeOfficeFreezeAckSchema = z.object({
  changeId: digest, operationId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/), generation: index, sequence: index
}).strict()
export const nativeOfficeManualStateSchema = nativeOfficeManualAuthoritySchema.pick({
  version: true, origin: true, changeId: true, baseRevision: true
}).extend({ generation: index, frozen: z.boolean() }).strict()
export type NativeOfficeRangeLease = z.infer<typeof nativeOfficeRangeLeaseSchema>
export type NativeOfficeFormat = z.infer<typeof nativeOfficeFormatSchema>
export type NativeOfficeFormatState = z.infer<typeof nativeOfficeFormatStateSchema>
export type NativeOfficeFreezeAck = z.infer<typeof nativeOfficeFreezeAckSchema>
export type NativeOfficeManualState = z.infer<typeof nativeOfficeManualStateSchema>

export const nativeOfficePreviewViewSchema = z.object({
  zoom: z.number().finite().positive().max(1000), fit: z.boolean(),
  page: z.number().int().min(1).max(512).nullable(), pages: z.number().int().min(1).max(512).nullable()
}).strict().refine(v => (v.page === null) === (v.pages === null) && (v.page === null || v.page <= v.pages!))
export type NativeOfficePreviewView = z.infer<typeof nativeOfficePreviewViewSchema>
export const nativeOfficePlaybackSchema = z.object({
  generation: index, sequence: index, phase: z.enum(['idle','running','paused','unsupported','error']),
  slideIndex: z.number().int().min(-1).max(511).nullable(), slideCount: z.number().int().min(1).max(512).nullable()
}).strict().refine(v => v.phase === 'running' || v.phase === 'paused'
  ? v.slideIndex !== null && v.slideCount !== null && v.slideIndex < v.slideCount
  : v.slideIndex === null && v.slideCount === null)
export type NativeOfficePlayback = z.infer<typeof nativeOfficePlaybackSchema>
const viewControlSchema = z.object({ action: z.literal('viewControl'), ...editingTarget,
  control: z.enum(['read','fit','zoom-in','zoom-out','previous-page','next-page','set-zoom',
    'play-read','play-start','play-current','play-next-effect','play-previous-effect','play-next-slide','play-previous-slide','play-pause','play-resume','play-end']),
  zoom: z.number().int().min(20).max(400).optional(), playbackGeneration: index.optional()
}).strict().refine(v => (v.control === 'set-zoom') === Object.hasOwn(v, 'zoom') &&
  (v.control !== 'set-zoom' || v.zoom !== undefined) &&
  (v.control.startsWith('play-') && v.control !== 'play-read') === Object.hasOwn(v, 'playbackGeneration') &&
  (!Object.hasOwn(v, 'playbackGeneration') || v.playbackGeneration !== undefined))
const threadId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
export const nativeOfficeInputFreezeSchema = z.object({requestId:z.string().uuid(),frozen:z.boolean()}).strict()
const pointerLease = z.object({interactionId:selectionToken,selectionToken}).strict()
export const nativeOfficeMenuTargetSchema = z.object({...editingTarget,pointer:pointerLease.optional()}).strict()
const actionId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
const menuLabel = z.string().min(1).max(120).refine(value => !/\p{Cc}/u.test(value))
export const nativeOfficeFormattingSchema = z.object({
  changeId: digest, rangeLease: nativeOfficeRangeLeaseSchema, formatState: nativeOfficeFormatStateSchema,
  labels: z.object({ fontFamily: menuLabel, fontSize: menuLabel, bold: menuLabel, italic: menuLabel, underline: menuLabel, mixed: menuLabel }).strict()
}).strict()
export const nativeOfficeActionMenuSchema = z.object({
  ...editingTarget, pointer:pointerLease.optional(), formatting: nativeOfficeFormattingSchema.optional(),
  actions: z.array(z.object({id:actionId,label:menuLabel,enabled:z.boolean()}).strict()).min(1).max(32)
}).strict().refine(value => new Set(value.actions.map(action => action.id)).size === value.actions.length)
  .refine(value => !Object.hasOwn(value, 'formatting') || !!value.formatting && !!value.pointer &&
    value.formatting.rangeLease.token === value.pointer.selectionToken && value.formatting.rangeLease.sequence === value.expectedChangeSequence)
export const nativeOfficeFormatChoiceSchema = z.object({
  actionId: z.null(), changeId: digest, rangeLease: nativeOfficeRangeLeaseSchema, format: nativeOfficeFormatSchema
}).strict()
export const nativeOfficeActionChoiceSchema = z.union([
  z.object({actionId:actionId.nullable()}).strict(), nativeOfficeFormatChoiceSchema
])
export type NativeOfficeMenuTarget = z.infer<typeof nativeOfficeMenuTargetSchema>
export type NativeOfficeActionMenu = z.infer<typeof nativeOfficeActionMenuSchema>
export type NativeOfficeFormatting = z.infer<typeof nativeOfficeFormattingSchema>
export type NativeOfficeFormatChoice = z.infer<typeof nativeOfficeFormatChoiceSchema>
export type NativeOfficeActionChoice = z.infer<typeof nativeOfficeActionChoiceSchema>

/** Renderer commands contain no native bytes, URLs, engine envelopes or UNO. */
export const nativeOfficeRequestSchema = z.discriminatedUnion('action', [
  viewControlSchema,
  z.object({ action: z.literal('create'), ...nativeOfficeCreateInputSchema.shape, bounds: nativeOfficeBoundsSchema, appearance: nativeOfficeAppearanceSchema.optional() }).strict(),
  z.object({ action: z.literal('beginManualEdit'), ...editingTarget, threadId }).strict(),
  z.object({ action: z.literal('editWorkbookCell'), ...editingTarget, threadId, selectionToken, edit:nativeWorkbookManualEditSchema }).strict(),
  z.object({ action: z.literal('format'), ...editingTarget, changeId: digest, rangeLease: nativeOfficeRangeLeaseSchema, format: nativeOfficeFormatSchema }).strict(),
  z.object({ action: z.literal('discardManualEdit'), ...editingTarget, threadId, changeId: digest }).strict(),

  z.object({ action: z.literal('openPresent'), protectedPresent: toolResultLocalPresentFileRequestSchemaV1, bounds: nativeOfficeBoundsSchema, appearance: nativeOfficeAppearanceSchema.optional() }).strict(),
  z.object({ action: z.literal('open'), workspace: localPath, path: localPath, bounds: nativeOfficeBoundsSchema, appearance: nativeOfficeAppearanceSchema.optional(), threadId: threadId.nullable().optional() }).strict(),
  z.object({ action: z.literal('bounds'), bounds: nativeOfficeBoundsSchema, appearance: nativeOfficeAppearanceSchema.optional() }).strict(),
  z.object({ action: z.literal('hide') }).strict(),
  z.object({ action: z.literal('status') }).strict(),
  z.object({ action: z.literal('captureSelection') }).strict(),
  z.object({ action: z.literal('close'), objectId: digest.optional(), discard: z.boolean().optional() }).strict(),
  z.object({ action: z.literal('annotate'), ...editingTarget }).strict(),
  z.object({ action: z.literal('undoChange'), ...editingTarget, threadId }).strict(),
  z.object({ action: z.literal('cancelChange'), ...editingTarget, threadId, changeId: digest }).strict(),
  z.object({ action: z.literal('resumeChange'), ...editingTarget, threadId, changeId: digest }).strict(),
  z.object({ action: z.literal('save'), ...editingTarget }).strict(),
  z.object({ action: z.literal('saveStatus'), objectId: digest }).strict(),
  z.object({ action: z.literal('annotationSave'), objectId: digest, threadId, note: nativeOfficeAnnotationNoteSchema, sourceRevision: digest }).strict(),
  z.object({ action: z.literal('annotationRetry'), objectId: digest, threadId }).strict(),
  z.object({ action: z.literal('reference'), ...editingTarget, selectionToken, threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/), editable: z.boolean(), saveFirst: z.literal(true).optional() }).strict(),
  z.object({ action: z.literal('proposals'), objectId: digest, scopeId:z.string().regex(/^[a-f0-9]{48}$/) }).strict(),
  z.object({ action: z.literal('acceptProposal'), ...editingTarget, scopeId:z.string().regex(/^[a-f0-9]{48}$/), proposalId:z.string().regex(/^[a-f0-9]{48}$/) }).strict(),
  z.object({ action: z.literal('rejectProposal'), objectId: digest, scopeId:z.string().regex(/^[a-f0-9]{48}$/), proposalId:z.string().regex(/^[a-f0-9]{48}$/) }).strict()
])
const capture = z.object({ capturedCharacters: index, totalCharacters: index, truncated: z.boolean(), unit: z.literal('utf-16'), complete: z.boolean() }).strict()
const selectionBase = { documentId: digest, version: digest, changeSequence: index, token: selectionToken.optional(), capture: capture.optional() }
export const nativeOfficeSelectionSchema = z.discriminatedUnion('kind', [
  z.object({ ...selectionBase, kind: z.literal('text'), rangeLease: nativeOfficeRangeLeaseSchema.optional(), formatState: nativeOfficeFormatStateSchema.optional(), scope: z.enum(['current-view-text-only; no verified structural offset or durable anchor', 'session-text-range-at-version-and-change-sequence']), text: z.string().max(4096) }).strict().refine(value => (!value.rangeLease && !value.formatState) || !!value.rangeLease && !!value.formatState && value.rangeLease.token === value.token && value.rangeLease.sequence === value.changeSequence),
  z.object({ ...selectionBase, kind: z.literal('cells'), editIntent:z.object({kind:z.enum(['f2','double-click']),id:selectionToken}).strict().optional(), scope: z.literal('sheet-range-address-at-version-and-change-sequence'), text: z.string().max(4096), cells: z.array(z.object({ sheet: index, column: index, row: index, text: z.string().max(4096), formula: z.string().max(4096), value: z.number().finite(), valueType: z.enum(['empty', 'text', 'number', 'formula']), valueError:z.boolean().optional(), formulaResultType:z.enum(['number','text']).optional(), numberFormat: index, rowVisible: z.boolean(), columnVisible: z.boolean(), merged: z.boolean() }).strict()).max(256).optional(), ranges: z.array(z.object({
    sheet: index, sheetName: z.string().max(4096), startColumn: index, startRow: index, endColumn: index, endRow: index
  }).strict().refine(value => value.endColumn >= value.startColumn && value.endRow >= value.startRow)).max(64) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('shapes'), presentation: nativePresentationSelectionSchema.optional(), scope: z.literal('page-and-shape-index-at-version-and-change-sequence'), shapes: z.array(z.object({
    pageIndex: index, shapeIndex: index, name: z.string().max(4096), type: z.string().max(256), text: z.string().max(4096)
  }).strict()).max(64) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('unavailable'), scope: z.literal('engine selection interface unavailable in this view') }).strict()
])
export const nativeOfficeErrorSchema = z.enum([
  'invalid_request', 'unavailable', 'forbidden', 'package_unavailable', 'package_changed', 'invalid_response',
  'engine_unavailable', 'unsaved_changes', 'conflict', 'unknown', 'save_failed', 'stale_selection', 'unsupported_selection', 'capacity',
  'presentation_unsupported', 'presentation_failed', 'presentation_start_timeout', 'native_input_unavailable', 'composition_pending'
])
export const nativeOfficeViewSchema = z.object({
  objectId: digest, path: localPath, kind: nativeOfficeKindSchema, revision: digest,
  dirty: z.boolean(), status: z.enum(['ready', 'error', 'loading']),
  protectedPresent: toolResultLocalPresentFileRequestSchemaV1.optional(),
  workbookEditing:z.object({editable:z.boolean(),reason:z.literal('unsupported_workbook').optional()}).strict().optional(),
  previewView: nativeOfficePreviewViewSchema.optional(),
  playback: nativeOfficePlaybackSchema.optional(),
  scope: nativeOfficeSelectionScopeSchema.omit({sessionId:true}).optional(), proposals:z.array(nativeOfficeProposalSchema).max(16).optional(), appliedProposals:z.array(z.string()).max(16).optional(),
  recovery: nativeOfficeRecoverySchema.optional(), localReviews: z.array(nativeOfficeLocalReviewSchema).max(16).optional(),
  annotation: nativeOfficeAnnotationSchema.optional(), annotationSaving: z.boolean().optional(), annotationError: z.boolean().optional(),
  manual: nativeOfficeManualStateSchema.optional(),
  editing: z.boolean().optional(), canUndo: z.boolean().optional(), changeSequence: index.optional(), acknowledgedSequence: index.optional(), saving: z.boolean().optional(),
  selection: nativeOfficeSelectionSchema.optional(), error: nativeOfficeErrorSchema.optional()
}).strict().refine(v => !v.previewView || (v.kind === 'pptx' ? v.previewView.page !== null : v.previewView.page === null))
  .refine(v => !Object.hasOwn(v, 'playback') || v.kind === 'pptx' && v.playback !== undefined)
  .refine(v => !v.manual || v.kind === 'docx' && v.manual.baseRevision === v.revision && v.editing === true)
  .refine(v => !v.protectedPresent || (['docx','pptx'].includes(v.kind) && !v.dirty && !v.editing && !v.saving &&
    !v.manual && !v.selection && !v.scope && !v.annotation && !v.annotationSaving && !v.annotationError && !v.recovery && !v.proposals && !v.localReviews && !v.appliedProposals && !v.canUndo &&
    (v.changeSequence ?? 0) === 0 && (v.acknowledgedSequence ?? 0) === 0))
export const nativeOfficeResponseSchema = z.object({
  ok: z.boolean(), view: nativeOfficeViewSchema.nullable(), error: nativeOfficeErrorSchema.optional(), creation: nativeOfficeCreationSchema.optional()
}).strict()
export type NativeOfficeRequest = z.infer<typeof nativeOfficeRequestSchema>
export type NativeOfficeBounds = z.infer<typeof nativeOfficeBoundsSchema>
export type NativeOfficeView = z.infer<typeof nativeOfficeViewSchema>
export type NativeOfficeSelection = z.infer<typeof nativeOfficeSelectionSchema>
export type NativeOfficeResponse = z.infer<typeof nativeOfficeResponseSchema>
export type NativeOfficeError = z.infer<typeof nativeOfficeErrorSchema>
export type NativeOfficeKind = z.infer<typeof nativeOfficeKindSchema>

export type NativeOfficeAppearance = z.infer<typeof nativeOfficeAppearanceSchema>

export function nativeTypedWorkbookSelection(selection: NativeOfficeSelection) {
  if (selection.kind !== 'cells' || selection.ranges.length !== 1 || !selection.capture?.complete || selection.capture.truncated || !selection.cells) return null
  if (selection.cells.some(c => c.valueError === true)) return null
  const parsed=nativeWorkbookSelectionSchema.safeParse({...selection.ranges[0],cells:selection.cells.map(({valueError: _valueError,formulaResultType: _formulaResultType,...cell}) => cell)})
  return parsed.success ? parsed.data : null
}

export function nativeTypedPresentationSelection(selection: NativeOfficeSelection) {
  if (selection.kind !== 'shapes' || selection.shapes.length !== 1 || !selection.presentation || !selection.token) return null
  const parsed = nativePresentationSelectionSchema.safeParse(selection.presentation)
  const target = selection.shapes[0]
  return parsed.success && parsed.data.pageIndex === target.pageIndex && parsed.data.targetShapeIndex === target.shapeIndex ? parsed.data : null
}
