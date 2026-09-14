import { z } from 'zod'

/** Protected-local host transport only; never a model tool or history payload. */
export const objectEditingPath = '/v1/local-display/object-editing'
const sessionId = z.string().regex(/^[a-f0-9]{48}$/)
const revision = z.string().regex(/^[a-f0-9]{64}$/)
const operationId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/)

export const objectEditingRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('open'), workspace: z.string().min(1), path: z.string().min(1) }).strict(),
  z.object({ action: z.literal('commit'), sessionId, operationId, baseRevision: revision, content: z.string().max(1_572_864) }).strict(),
  z.object({ action: z.literal('status'), sessionId, operationId }).strict(),
  z.object({ action: z.literal('close'), sessionId }).strict()
])

export const objectEditingReceiptSchema = z.object({
  operationId,
  revision: z.union([revision, z.literal('')]),
  status: z.enum(['committed', 'conflict', 'unknown', 'pending']),
  savedAt: z.string()
}).strict().superRefine((receipt, ctx) => {
  if (receipt.status === 'committed' && (!revision.safeParse(receipt.revision).success || !z.string().datetime({ offset: true }).safeParse(receipt.savedAt).success)) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'A committed receipt requires a revision and confirmation time.' })
  }
})

export const objectEditingResponseSchema = z.union([
  z.object({ ok: z.literal(true), document: z.object({
    sessionId, objectId: revision, path: z.string().min(1), content: z.string(), revision
  }).strict() }).strict(),
  z.object({ ok: z.literal(true), receipt: objectEditingReceiptSchema }).strict(),
  z.object({ ok: z.literal(true), closed: z.literal(true) }).strict(),
  z.object({ ok: z.literal(false), code: z.enum([
    'unavailable', 'unsupported_platform', 'invalid_request', 'session_invalid', 'capacity', 'forbidden',
    'not_text', 'too_large', 'conflict', 'operation_mismatch', 'operation_not_found', 'persistence_failure'
  ]), message: z.string(), receipt: objectEditingReceiptSchema.optional() }).strict()
])

export type ObjectEditingRequest = z.infer<typeof objectEditingRequestSchema>
export type ObjectEditingResponse = z.infer<typeof objectEditingResponseSchema>
