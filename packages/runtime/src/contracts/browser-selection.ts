import { z } from 'zod'

export const browserSelectionPath = '/v1/local-display/browser-selection'
const token = z.string().regex(/^[a-f0-9]{48}$/)
export const browserScopeSchema = z.object({
  scopeId: token, documentId: token, selectionId: token,
  threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/),
  workspace: z.string().min(1).max(4096)
}).strict()
export type BrowserScope = z.infer<typeof browserScopeSchema>
export const browserSelectionRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('capture'), guestId: z.number().int().positive(), threadId: browserScopeSchema.shape.threadId }).strict(),
  z.object({ action: z.literal('validate'), scope: browserScopeSchema }).strict(),
  z.object({ action: z.literal('revoke'), scope: browserScopeSchema }).strict()
])
export type BrowserSelectionRequest = z.infer<typeof browserSelectionRequestSchema>
export type BrowserSelectionResponse = { ok: true; scope?: BrowserScope } | { ok: false; error?: 'child-frame-unsupported' }
