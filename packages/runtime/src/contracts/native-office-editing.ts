import { z } from 'zod'
import { objectEditingPatchPartSchema, objectEditingResponseSchema } from './object-editing'

/** Protected-local package Host input only. Never a model tool/history payload. */
export const MaxNativeOfficeBytes = 16 * 1024 * 1024
export const nativeOfficeContentSchema = z.object({
  encoding: z.literal('base64'),
  kind: z.enum(['docx', 'xlsx', 'pptx']),
  byteLength: z.number().int().positive().max(MaxNativeOfficeBytes),
  sha256: z.string().regex(/^[a-f0-9]{64}$/),
  data: z.string().max(Math.ceil(MaxNativeOfficeBytes / 3) * 4)
}).strict().superRefine((content, ctx) => {
  const { data, byteLength } = content
  const padding = byteLength % 3 === 0 ? 0 : 3 - byteLength % 3
  const end = data.length - padding
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  if (data.length !== Math.ceil(byteLength / 3) * 4 ||
      !/^[A-Za-z0-9+/]*$/.test(data.slice(0, end)) ||
      data.slice(end) !== '='.repeat(padding) ||
      (padding > 0 && (alphabet.indexOf(data[end - 1]) & (padding === 2 ? 15 : 3)) !== 0)) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Invalid canonical base64 or byte length.' })
  }
})

// Core verifies SHA-256 against decoded bytes and inspects the OOXML package
// kind/security profile before persistence; schema validation is not admission.
export const nativeOfficeCommitInputSchema = z.object({
  sessionId: z.string().regex(/^[a-f0-9]{48}$/),
  operationId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/),
  threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/),
  changeId: z.string().regex(/^[a-f0-9]{64}$/),
  baseRevision: z.string().regex(/^[a-f0-9]{64}$/),
  content: nativeOfficeContentSchema
}).strict()
export const nativeOfficeStatusInputSchema = nativeOfficeCommitInputSchema.pick({ sessionId: true, operationId: true })
export type NativeOfficeCommitInput = z.infer<typeof nativeOfficeCommitInputSchema>
export type NativeOfficeStatusInput = z.infer<typeof nativeOfficeStatusInputSchema>

// Native selections use engine-owned opaque targets. They are not UTF-16 ranges
// into an OOXML/base64 draft. Main alone may capture, approve or revoke them.
const selectionSession = z.string().regex(/^[a-f0-9]{48}$/)
const scopeId = selectionSession
const nativeRevision = z.string().regex(/^[a-f0-9]{64}$/)
const nativeOperation = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/)
const nativeToken = z.string().regex(/^[A-Za-z0-9_-]{8,128}$/)
const nativeSequence = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const nativeThread = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
const nativeLocalPath = z.string().min(1).max(32768).refine(value => !value.includes('\0'))

/** User creation is resolved inside Core's current thread workspace; no caller path grants authority. */
export const nativeOfficeCreateInputSchema = z.object({
  threadId: nativeThread, operationId: nativeOperation,
  fileName: z.string().min(6).max(255).refine(value => value.trim() === value &&
    !/[\\/\p{Cc}:*?"<>|]/u.test(value) && /\.docx$/i.test(value) &&
    value.slice(0, -5).trim().length > 0 && new TextEncoder().encode(value).byteLength <= 255)
}).strict()
export const nativeOfficeCreationSchema = z.object({
  operationId: nativeOperation, workspace: nativeLocalPath, path: nativeLocalPath,
  revision: nativeRevision, kind: z.literal('docx'),
  byteSize: z.number().int().positive().max(MaxNativeOfficeBytes), savedAt: z.string().datetime({ offset: true })
}).strict()
/** Core persists this authority before Main permits the first native manual mutation. */
export const nativeOfficeManualBeginInputSchema = z.object({
  sessionId: selectionSession, threadId: nativeThread, operationId: nativeOperation, baseRevision: nativeRevision
}).strict()
export const nativeOfficeManualAuthoritySchema = z.object({
  version: z.literal(2), origin: z.literal('manual'), changeId: nativeRevision,
  baseRevision: nativeRevision, saveOperationId: nativeOperation, undoOperationId: nativeOperation
}).strict()
export type NativeOfficeCreateInput = z.infer<typeof nativeOfficeCreateInputSchema>
export type NativeOfficeCreation = z.infer<typeof nativeOfficeCreationSchema>
export type NativeOfficeManualBeginInput = z.infer<typeof nativeOfficeManualBeginInputSchema>
export type NativeOfficeManualAuthority = z.infer<typeof nativeOfficeManualAuthoritySchema>
export const MaxNativeOfficeSelectionUTF16 = 4096
const selectionText = z.string().max(65_536).refine(value => {
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index)
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++index)
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false
    } else if (code >= 0xdc00 && code <= 0xdfff) return false
  }
  return new TextEncoder().encode(value).byteLength <= 65_536
})
const selectionParts = z.array(objectEditingPatchPartSchema).max(256)
export const nativeWorkbookCellSchema = z.object({
  sheet: nativeSequence, column: nativeSequence.max(1023), row: nativeSequence.max(99999),
  text: z.string().max(4096), formula: z.string().max(4096), value: z.number().finite(),
  valueType: z.enum(['empty','text','number','formula']), numberFormat: nativeSequence,
  rowVisible: z.literal(true), columnVisible: z.literal(true), merged: z.literal(false)
}).strict()
export const nativeWorkbookSelectionSchema = z.object({
  sheet: nativeSequence.max(19), sheetName: z.string().min(1).max(31),
  startColumn: nativeSequence.max(1023), endColumn: nativeSequence.max(1023),
  startRow: nativeSequence.max(99999), endRow: nativeSequence.max(99999),
  cells: z.array(nativeWorkbookCellSchema).min(1).max(256)
}).strict().refine(s => s.endColumn >= s.startColumn && s.endRow >= s.startRow &&
  (s.endColumn-s.startColumn+1)*(s.endRow-s.startRow+1) === s.cells.length &&
  s.cells.every((c,i) => c.sheet === s.sheet && c.column === s.startColumn+i%(s.endColumn-s.startColumn+1) && c.row === s.startRow+Math.floor(i/(s.endColumn-s.startColumn+1))))
const nativeWorkbookEditSchema = z.discriminatedUnion('type', [
  z.object({rowOffset:nativeSequence.max(255),columnOffset:nativeSequence.max(255),type:z.literal('number'),value:z.number().finite()}).strict(),
  z.object({rowOffset:nativeSequence.max(255),columnOffset:nativeSequence.max(255),type:z.literal('formula'),formula:z.string().min(1).max(1024)}).strict(),
  z.object({rowOffset:nativeSequence.max(255),columnOffset:nativeSequence.max(255),type:z.literal('text'),text:z.string().max(4096)}).strict(),
  z.object({rowOffset:nativeSequence.max(255),columnOffset:nativeSequence.max(255),type:z.literal('empty')}).strict()
])
/** Explicit user intent. Text is never parsed as a number or a formula. */
export const nativeWorkbookManualEditSchema = z.discriminatedUnion('type', [
  z.object({type:z.literal('number'),value:z.number().finite()}).strict(),
  z.object({type:z.literal('formula'),formula:z.string().min(1).max(1024)}).strict(),
  z.object({type:z.literal('text'),text:z.string().min(1).max(4096)}).strict(),
  z.object({type:z.literal('empty')}).strict()
])
export const nativeWorkbookWriteTargetSchema = z.object({sheet:nativeSequence.max(19),sheetName:z.string().min(1).max(31),column:nativeSequence.max(1023),row:nativeSequence.max(99999)}).strict()
export const nativeWorkbookDependencyBoundsSchema = z.object({sheet:nativeSequence.max(19),startColumn:nativeSequence.max(1023),endColumn:nativeSequence.max(1023),startRow:nativeSequence.max(99999),endRow:nativeSequence.max(99999)}).strict().refine(v => v.endColumn >= v.startColumn && v.endRow >= v.startRow && (v.endColumn-v.startColumn+1)*(v.endRow-v.startRow+1) <= 256)
export const nativeWorkbookManualPlanInputSchema = z.object({sessionId:selectionSession,threadId:nativeThread,baseRevision:nativeRevision,writeTarget:nativeWorkbookWriteTargetSchema,edit:nativeWorkbookManualEditSchema}).strict()
export const nativeWorkbookManualPrepareInputSchema = nativeWorkbookManualPlanInputSchema.extend({operationId:nativeOperation,selectionToken:nativeToken,changeSequence:nativeSequence,workbook:nativeWorkbookSelectionSchema}).strict()
export type NativeWorkbookManualEdit = z.infer<typeof nativeWorkbookManualEditSchema>
export const nativeWorkbookPatchSchema = z.discriminatedUnion('kind',[
  z.object({kind:z.literal('number'),value:z.number().finite()}).strict(),
  z.object({kind:z.literal('formula'),formula:z.string().min(1).max(1024)}).strict(),
  z.object({kind:z.literal('range'),cells:z.array(nativeWorkbookEditSchema).min(1).max(256)}).strict()
])
export const nativeWorkbookReviewSchema = z.object({before:nativeWorkbookSelectionSchema,after:nativeWorkbookSelectionSchema,results:z.array(z.string().max(65536)).min(1).max(256)}).strict()
export type NativeWorkbookSelection = z.infer<typeof nativeWorkbookSelectionSchema>
export type NativeWorkbookReview = z.infer<typeof nativeWorkbookReviewSchema>
export const nativeWorkbookManualAuthoritySchema = z.object({
  version:z.literal(3),origin:z.literal('manual'),changeId:nativeRevision,baseRevision:nativeRevision,
  saveOperationId:nativeOperation,undoOperationId:nativeOperation,workbook:nativeWorkbookReviewSchema
}).strict()
const presentationDimension = z.number().int().min(0).max(1_000_000)
const presentationText = selectionText.refine(value => !value.includes('\0') && new TextEncoder().encode(value).byteLength <= 4096)
export const nativePresentationSelectionSchema = z.object({
  pageIndex: nativeSequence.max(511), targetShapeIndex: nativeSequence.max(63),
  pageWidth100thMm: presentationDimension.positive(), pageHeight100thMm: presentationDimension.positive(),
  shapes: z.array(z.object({
    shapeIndex: nativeSequence.max(63), kind: z.enum(['rectangle', 'ellipse', 'text']),
    name: presentationText, text: presentationText,
    x100thMm: presentationDimension, y100thMm: presentationDimension,
    width100thMm: presentationDimension.positive(), height100thMm: presentationDimension.positive(),
    fillRGB: z.string().regex(/^#[0-9a-f]{6}$/)
  }).strict()).min(1).max(64)
}).strict().refine(s => s.targetShapeIndex < s.shapes.length &&
  s.shapes.every((shape, i) => shape.shapeIndex === i && shape.x100thMm + shape.width100thMm <= s.pageWidth100thMm && shape.y100thMm + shape.height100thMm <= s.pageHeight100thMm) &&
  s.shapes.reduce((bytes, shape) => bytes + new TextEncoder().encode(shape.name + shape.text).byteLength, 0) <= 65536)
export const nativePresentationPatchSchema = z.discriminatedUnion('kind', [
  z.object({kind: z.literal('shape-geometry'), x100thMm: presentationDimension, y100thMm: presentationDimension, width100thMm: presentationDimension.positive(), height100thMm: presentationDimension.positive()}).strict(),
  z.object({kind: z.literal('shape-fill'), rgb: z.string().regex(/^#[0-9a-f]{6}$/)}).strict()
])
export const nativePresentationReviewSchema = z.object({before: nativePresentationSelectionSchema, after: nativePresentationSelectionSchema}).strict().refine(({before, after}) => {
  const a = before.shapes[before.targetShapeIndex], b = after.shapes[after.targetShapeIndex]
  if (!a || !b) return false
  const expected = structuredClone(before)
  expected.shapes[before.targetShapeIndex] = a.fillRGB !== b.fillRGB ? {...a, fillRGB: b.fillRGB} : {...a, x100thMm: b.x100thMm, y100thMm: b.y100thMm, width100thMm: b.width100thMm, height100thMm: b.height100thMm}
  return JSON.stringify(before) !== JSON.stringify(after) && JSON.stringify(expected) === JSON.stringify(after)
})
export type NativePresentationSelection = z.infer<typeof nativePresentationSelectionSchema>
export type NativePresentationReview = z.infer<typeof nativePresentationReviewSchema>
const selectionCapture = z.object({
  sessionId: selectionSession, threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/),
  selectionToken: nativeToken, changeSequence: nativeSequence, baseRevision: nativeRevision,
  text: selectionText, editable: z.boolean(), workbook:nativeWorkbookSelectionSchema.optional(), presentation:nativePresentationSelectionSchema.optional()
}).strict()
export const nativeOfficeSelectionCaptureInputSchema = selectionCapture.refine(value => !(value.workbook && value.presentation) && (!value.editable || !!value.workbook || !!value.presentation || value.text.length <= MaxNativeOfficeSelectionUTF16))
export const nativeOfficeScopeInputSchema = z.object({ sessionId: selectionSession, scopeId }).strict()
export const nativeOfficeProposalRejectInputSchema = nativeOfficeScopeInputSchema.extend({ proposalId: scopeId, operationId: nativeOperation })
export const nativeOfficeProposalDecisionInputSchema = nativeOfficeProposalRejectInputSchema.extend({
  selectionToken: nativeToken, changeSequence: nativeSequence, baseRevision: nativeRevision
})
export const nativeOfficeSelectionScopeSchema = selectionCapture.omit({ text: true }).extend({ scopeId, parts: selectionParts })
const replacementParts = selectionParts.refine(parts => parts.reduce((units, part) => units + (part.kind === 'literal' ? part.text.length : 0), 0) <= MaxNativeOfficeSelectionUTF16)
export const nativeOfficeProposalSchema = z.object({ proposalId: scopeId, status: z.enum(['proposed', 'approved', 'rejected']), parts: replacementParts, workbook:nativeWorkbookPatchSchema.optional(), presentation:nativePresentationPatchSchema.optional() }).strict()
export const nativeOfficeReplacementSchema = z.object({
  proposalId: scopeId, operationId: nativeOperation, text: selectionText, workbook:nativeWorkbookReviewSchema.optional(), presentation:nativePresentationReviewSchema.optional(),
  changeId: nativeRevision, saveOperationId: nativeOperation,
  selectionToken: nativeToken, changeSequence: nativeSequence, baseRevision: nativeRevision
}).strict().refine(value => !(value.workbook && value.presentation) && (!!value.workbook || !!value.presentation || value.text.length <= MaxNativeOfficeSelectionUTF16))
// Raw review text is protected-local display data. Never include this in a
// model tool result, reference card, or persisted conversation message.
export const nativeOfficeLocalReviewSchema = z.object({ proposalId: scopeId, beforeText: selectionText, afterText: selectionText, workbook:nativeWorkbookReviewSchema.optional(), presentation:nativePresentationReviewSchema.optional() }).strict()
const nativeOfficeProposalChangeSchema = z.object({
  changeId: nativeRevision, threadId: selectionCapture.shape.threadId, proposalId: scopeId,
  baseRevision: nativeRevision, revision: z.union([nativeRevision, z.literal('')]),
  status: z.enum(['prepared', 'unknown', 'committed', 'conflict', 'undone', 'cancelled', 'superseded']),
  beforeText: selectionText, afterText: selectionText, workbook:nativeWorkbookReviewSchema.optional(), presentation:nativePresentationReviewSchema.optional(),
  saveOperationId: nativeOperation, undoOperationId: nativeOperation, canUndo: z.boolean(),
  canCancel: z.boolean(), canRetryUndo: z.boolean(), canResume: z.boolean(),
  createdAt: z.string().max(64), savedAt: z.string().max(64)
}).strict()
// The legacy proposal arm is unchanged. Missing proposalId never implies manual authority.
export const nativeOfficeManualChangeSchema = nativeOfficeProposalChangeSchema.omit({
  proposalId: true, beforeText: true, afterText: true, workbook: true, presentation: true
}).extend({version: z.literal(2), origin: z.literal('manual')}).strict()
export const nativeWorkbookManualChangeSchema = nativeOfficeProposalChangeSchema.omit({proposalId:true,beforeText:true,afterText:true,presentation:true}).extend({version:z.literal(3),origin:z.literal('manual')}).strict().refine(v => v.status === 'unknown' || !!v.workbook)
export const nativeOfficeChangeSchema = z.union([nativeOfficeProposalChangeSchema, nativeOfficeManualChangeSchema, nativeWorkbookManualChangeSchema])
export const nativeOfficeRecoverySchema = z.object({current:nativeOfficeChangeSchema.nullable(),pending:nativeOfficeChangeSchema.nullable()}).strict()
// Annotation drafts are protected-local notes, never persisted selection authority.
export const nativeOfficeAnnotationNoteSchema = selectionText.refine(value => value.length <= 4096)
export const nativeOfficeAnnotationSchema = z.object({
  objectId: nativeRevision, threadId: selectionCapture.shape.threadId,
  draftRevision: z.union([nativeRevision, z.literal('')]), note: nativeOfficeAnnotationNoteSchema,
  sourceRevision: z.union([nativeRevision, z.literal('')]), updatedAt: z.string().max(64)
}).strict()
export type NativeOfficeAnnotation = z.infer<typeof nativeOfficeAnnotationSchema>
export const nativeOfficeSelectionResponseSchema = z.union([
  objectEditingResponseSchema,
  z.object({ ok: z.literal(true), creation: nativeOfficeCreationSchema }).strict(),
  z.object({ ok: z.literal(true), manual: nativeOfficeManualAuthoritySchema }).strict(),
  z.object({ ok: z.literal(true), dependencyBounds: nativeWorkbookDependencyBoundsSchema }).strict(),
  z.object({ ok: z.literal(true), workbookManual: nativeWorkbookManualAuthoritySchema }).strict(),
  z.object({ ok: z.literal(true), scope: nativeOfficeSelectionScopeSchema }).strict(),
  z.object({ ok: z.literal(true), proposals: z.array(nativeOfficeProposalSchema).max(16), localReviews: z.array(nativeOfficeLocalReviewSchema).max(16) }).strict(),
  z.object({ ok: z.literal(true), recovery: nativeOfficeRecoverySchema }).strict(),
  z.object({ ok: z.literal(true), annotation: nativeOfficeAnnotationSchema }).strict(),
  z.object({ ok: z.literal(true), proposal: nativeOfficeProposalSchema }).strict(),
  z.object({ ok: z.literal(true), replacement: nativeOfficeReplacementSchema }).strict()
])
export type NativeOfficeSelectionScope = z.infer<typeof nativeOfficeSelectionScopeSchema>
export type NativeOfficeProposal = z.infer<typeof nativeOfficeProposalSchema>
export type NativeOfficeReplacement = z.infer<typeof nativeOfficeReplacementSchema>
export type NativeOfficeSelectionResponse = z.infer<typeof nativeOfficeSelectionResponseSchema>
