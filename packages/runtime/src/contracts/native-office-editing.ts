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
const selectionCapture = z.object({
  sessionId: selectionSession, threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/),
  selectionToken: nativeToken, changeSequence: nativeSequence, baseRevision: nativeRevision,
  text: selectionText, editable: z.boolean()
}).strict()
export const nativeOfficeSelectionCaptureInputSchema = selectionCapture.refine(value => !value.editable || value.text.length <= MaxNativeOfficeSelectionUTF16)
export const nativeOfficeScopeInputSchema = z.object({ sessionId: selectionSession, scopeId }).strict()
export const nativeOfficeProposalRejectInputSchema = nativeOfficeScopeInputSchema.extend({ proposalId: scopeId, operationId: nativeOperation })
export const nativeOfficeProposalDecisionInputSchema = nativeOfficeProposalRejectInputSchema.extend({
  selectionToken: nativeToken, changeSequence: nativeSequence, baseRevision: nativeRevision
})
export const nativeOfficeSelectionScopeSchema = selectionCapture.omit({ text: true }).extend({ scopeId, parts: selectionParts })
const replacementParts = selectionParts.refine(parts => parts.reduce((units, part) => units + (part.kind === 'literal' ? part.text.length : 0), 0) <= MaxNativeOfficeSelectionUTF16)
export const nativeOfficeProposalSchema = z.object({ proposalId: scopeId, status: z.enum(['proposed', 'approved', 'rejected']), parts: replacementParts }).strict()
export const nativeOfficeReplacementSchema = z.object({
  proposalId: scopeId, operationId: nativeOperation, text: selectionText.refine(value => value.length <= MaxNativeOfficeSelectionUTF16),
  changeId: nativeRevision, saveOperationId: nativeOperation,
  selectionToken: nativeToken, changeSequence: nativeSequence, baseRevision: nativeRevision
}).strict()
// Raw review text is protected-local display data. Never include this in a
// model tool result, reference card, or persisted conversation message.
export const nativeOfficeLocalReviewSchema = z.object({ proposalId: scopeId, beforeText: selectionText, afterText: selectionText }).strict()
export const nativeOfficeChangeSchema = z.object({
  changeId: nativeRevision, threadId: selectionCapture.shape.threadId, proposalId: scopeId,
  baseRevision: nativeRevision, revision: z.union([nativeRevision, z.literal('')]),
  status: z.enum(['prepared', 'unknown', 'committed', 'conflict', 'undone', 'cancelled', 'superseded']),
  beforeText: selectionText, afterText: selectionText,
  saveOperationId: nativeOperation, undoOperationId: nativeOperation, canUndo: z.boolean(),
  canCancel: z.boolean(), canRetryUndo: z.boolean(), canResume: z.boolean(),
  createdAt: z.string().max(64), savedAt: z.string().max(64)
}).strict()
export const nativeOfficeRecoverySchema = z.object({current:nativeOfficeChangeSchema.nullable(),pending:nativeOfficeChangeSchema.nullable()}).strict()
export const nativeOfficeSelectionResponseSchema = z.union([
  objectEditingResponseSchema,
  z.object({ ok: z.literal(true), scope: nativeOfficeSelectionScopeSchema }).strict(),
  z.object({ ok: z.literal(true), proposals: z.array(nativeOfficeProposalSchema).max(16), localReviews: z.array(nativeOfficeLocalReviewSchema).max(16) }).strict(),
  z.object({ ok: z.literal(true), recovery: nativeOfficeRecoverySchema }).strict(),
  z.object({ ok: z.literal(true), proposal: nativeOfficeProposalSchema }).strict(),
  z.object({ ok: z.literal(true), replacement: nativeOfficeReplacementSchema }).strict()
])
export type NativeOfficeSelectionScope = z.infer<typeof nativeOfficeSelectionScopeSchema>
export type NativeOfficeProposal = z.infer<typeof nativeOfficeProposalSchema>
export type NativeOfficeReplacement = z.infer<typeof nativeOfficeReplacementSchema>
export type NativeOfficeSelectionResponse = z.infer<typeof nativeOfficeSelectionResponseSchema>
