import { z } from 'zod'
import { canvasChangeSchema, canvasImageOperationsSchema, canvasImageResultSchema, canvasOperationsSchema } from './canvas-editing'
import { nativeOfficeRecoverySchema } from './native-office-editing'
import { objectEditingReceiptSchema } from './object-editing'

// Protected-local plugin Host transport. These are not model tool arguments.
const sessionId = z.string().regex(/^[a-f0-9]{48}$/)
const revision = z.string().regex(/^[a-f0-9]{64}$/)
const id = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
const path = z.string().min(1).max(4096).refine(v => !/[\0\r\n]/.test(v) && new TextEncoder().encode(v).length <= 4096)
const binding = { sessionId, threadId: id }
const proposalBinding = { ...binding, proposalId: sessionId }
const recoveryBinding = { ...binding, changeId: revision, baseRevision: revision }
export const canvasHostRequestSchema = z.discriminatedUnion('operation', [
  z.object({ operation: z.literal('open-object'), threadId: id, object: z.object({ workspace: path, path }).strict(), kind: z.enum(['canvas', 'png']) }).strict(),
  z.object({ operation: z.literal('read-object'), ...binding }).strict(),
  z.object({ operation: z.literal('close-object'), ...binding }).strict(),
  z.object({ operation: z.literal('object-recovery'), ...binding }).strict(),
  z.object({ operation: z.literal('propose-scene'), ...binding, baseRevision: revision, selectedIds: z.array(id).min(1).max(1280).refine(v => new Set(v).size === v.length), operations: canvasOperationsSchema }).strict(),
  z.object({ operation: z.literal('propose-image'), ...binding, baseRevision: revision, operations: canvasImageOperationsSchema }).strict(),
  z.object({ operation: z.literal('proposal-read'), ...proposalBinding }).strict(),
  z.object({ operation: z.literal('proposal-apply'), ...proposalBinding }).strict(),
  z.object({ operation: z.literal('proposal-reject'), ...proposalBinding }).strict(),
  z.object({ operation: z.literal('undo-change'), ...recoveryBinding }).strict(),
  z.object({ operation: z.literal('resume-change'), ...recoveryBinding }).strict(),
  z.object({ operation: z.literal('cancel-change'), ...recoveryBinding }).strict()
])
export const canvasDocumentSchema = z.object({
  ...binding, objectId: revision, kind: z.enum(['canvas', 'png']), path, revision,
  content: z.string().min(4).max(Math.ceil((16 << 20) / 3) * 4).regex(/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/)
}).strict()
const proposalCommon = { proposalId: sessionId, baseRevision: revision, candidateDigest: revision, status: z.enum(['proposed', 'pending', 'applied', 'rejected']) }
export const canvasProposalSchema = z.discriminatedUnion('kind', [
  z.object({ ...proposalCommon, kind: z.literal('canvas'), factsDigest: revision, sceneDiff: z.array(canvasChangeSchema).min(1).max(128) }).strict(),
  z.object({ ...proposalCommon, kind: z.literal('png'), imageDiff: canvasImageResultSchema.shape.diff }).strict()
])
export const canvasHostResponseSchema = z.union([
  z.object({ ok: z.literal(true), document: canvasDocumentSchema }).strict(),
  z.object({ ok: z.literal(true), proposal: canvasProposalSchema }).strict(),
  z.object({ ok: z.literal(true), receipt: objectEditingReceiptSchema }).strict(),
  z.object({ ok: z.literal(true), recovery: nativeOfficeRecoverySchema }).strict(),
  z.object({ ok: z.literal(true), closed: z.literal(true) }).strict(),
  z.object({ ok: z.literal(true), rejected: z.literal(true) }).strict(),
  z.object({ ok: z.literal(false), code: z.enum(['unavailable', 'invalid_request', 'conflict', 'forbidden', 'too_large', 'persistence_failure', 'operation_mismatch', 'operation_not_found']), receipt: objectEditingReceiptSchema.optional() }).strict()
])
export type CanvasHostRequest = z.infer<typeof canvasHostRequestSchema>
export type CanvasHostResponse = z.infer<typeof canvasHostResponseSchema>
export type CanvasDocument = z.infer<typeof canvasDocumentSchema>
export type CanvasProposal = z.infer<typeof canvasProposalSchema>
