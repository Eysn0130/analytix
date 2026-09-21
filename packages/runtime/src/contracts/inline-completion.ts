import { z } from 'zod'

export const inlineCompletionPath = '/v1/local-display/inline-completion'
export const inlineCompletionDocumentSchema = z.union([z.object({
  sessionId: z.string().regex(/^[a-f0-9]{48}$/),
  objectId: z.string().regex(/^[a-f0-9]{64}$/),
  baseRevision: z.string().regex(/^[a-f0-9]{64}$/)
}).strict(), z.object({
  path: z.string().min(1).max(4096).refine(value => !value.includes('\0'))
}).strict()])
const threadId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
export const inlineCompletionRequestSchema = z.object({
  threadId,
  requestId: z.string().uuid(),
  document: inlineCompletionDocumentSchema,
  prompt: z.string().min(1).max(65536),
  model: z.string().min(1).max(128)
}).strict()
export const inlineCompletionResponseSchema = z.union([
  z.object({
    ok: z.literal(true),
    result: z.object({
      threadId,
      requestId: z.string().uuid(),
      objectId: z.string().regex(/^[a-f0-9]{64}$/),
      baseRevision: z.string().regex(/^[a-f0-9]{64}$/),
      text: z.string().max(16384)
    }).strict()
  }).strict(),
  z.object({ ok: z.literal(false), code: z.enum(['invalid_request', 'busy', 'canceled', 'unavailable']) }).strict()
])
export type InlineCompletionDocument = z.infer<typeof inlineCompletionDocumentSchema>
