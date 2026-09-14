import { z } from 'zod'

/** Protected-local host transport only; never a model tool or history payload. */
export const objectEditingPath = '/v1/local-display/object-editing'
const sessionId = z.string().regex(/^[a-f0-9]{48}$/)
const revision = z.string().regex(/^[a-f0-9]{64}$/)
const operationId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$/)

// UTF-8 byte limits match Core; reject lone UTF-16 surrogates before JSON encoding.
const text = (maxBytes: number) => z.string().superRefine((value, ctx) => {
  let bytes = 0
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index)
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++index)
      if (!(next >= 0xdc00 && next <= 0xdfff)) { ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Invalid Unicode text.' }); return }
      bytes += 4
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Invalid Unicode text.' }); return
    } else bytes += code < 0x80 ? 1 : code < 0x800 ? 2 : 3
  }
  if (bytes > maxBytes) ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Text exceeds the byte limit.' })
})
const content = text(1_572_864)
const threadId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
const purpose = z.enum(['discuss', 'edit'])
export const objectEditingRangeSchema = z.object({ start: z.number().int().min(0).max(1_572_864), end: z.number().int().min(0).max(1_572_864) }).strict().refine(value => value.end >= value.start)
export const objectEditingPatchPartSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('literal'), text: text(65_536).refine(value => value.length > 0) }).strict(),
  z.object({ kind: z.literal('protected'), protectedRef: z.string().regex(/^protected_[a-f0-9]{48}$/) }).strict()
])
const parts = z.array(objectEditingPatchPartSchema).max(256).superRefine((value, ctx) => {
  const literal = value.map(part => part.kind === 'literal' ? part.text : '').join('')
  if (!text(65_536).safeParse(literal).success) ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Patch exceeds the byte limit.' })
})
const binding = { sessionId, scopeId: sessionId, threadId, purpose, draftVersion: sessionId }
export const objectEditingDraftSchema = z.object({ objectId: revision, baseRevision: revision, version: sessionId, content }).strict()
export const objectEditingScopeSchema = z.object({ scopeId: sessionId, objectId: revision, threadId, purpose, draftVersion: sessionId, baseRevision: revision, range: objectEditingRangeSchema, parts, current: z.boolean() }).strict()
export const objectEditingProposalSchema = z.object({ proposalId: sessionId, scopeId: sessionId, draftVersion: sessionId, parts, status: z.enum(['proposed', 'accepted', 'rejected']) }).strict()
export const objectEditingDecisionSchema = z.object({ proposalId: sessionId, status: z.enum(['accepted', 'rejected']), draftVersion: sessionId }).strict()

export const objectEditingRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('open'), workspace: text(1_572_864).refine(value => /^(?:\/|[A-Za-z]:[\\/]|\\\\)/.test(value) && !value.includes("\0")), path: text(1_572_864).refine(value => value.length > 0 && !value.includes("\0")) }).strict(),
  z.object({ action: z.literal('commit'), sessionId, operationId, baseRevision: revision, content }).strict(),
  z.object({ action: z.literal('status'), sessionId, operationId }).strict(),
  z.object({ action: z.literal('close'), sessionId }).strict(),
  z.object({ action: z.literal('draft-update'), sessionId, baseRevision: revision, expectedVersion: z.union([sessionId, z.literal('')]), content }).strict(),
  z.object({ action: z.literal('draft-read'), sessionId }).strict(),
  z.object({ action: z.literal('scope-capture'), sessionId, draftVersion: sessionId, threadId, purpose, range: objectEditingRangeSchema }).strict(),
  z.object({ action: z.literal('scope-read'), ...binding }).strict(),
  z.object({ action: z.literal('scope-revoke'), ...binding }).strict(),
  z.object({ action: z.literal('proposal-create'), ...binding, operationId, parts }).strict(),
  z.object({ action: z.literal('proposal-accept'), ...binding, operationId, proposalId: sessionId }).strict(),
  z.object({ action: z.literal('proposal-reject'), ...binding, operationId, proposalId: sessionId }).strict()
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

// These values are protected-local only, never ordinary turn/history events.
export const objectEditingResponseSchema = z.union([
  z.object({ ok: z.literal(true), draft: objectEditingDraftSchema }).strict(),
  z.object({ ok: z.literal(true), scope: objectEditingScopeSchema }).strict(),
  z.object({ ok: z.literal(true), revoked: z.literal(true) }).strict(),
  z.object({ ok: z.literal(true), proposal: objectEditingProposalSchema }).strict(),
  z.object({ ok: z.literal(true), decision: objectEditingDecisionSchema }).strict(),
  z.object({ ok: z.literal(true), document: z.object({
    sessionId, objectId: revision, path: z.string().min(1), content: z.string(), revision
  }).strict() }).strict(),
  z.object({ ok: z.literal(true), receipt: objectEditingReceiptSchema }).strict(),
  z.object({ ok: z.literal(true), closed: z.literal(true) }).strict(),
  z.object({ ok: z.literal(false), code: z.enum([
    'unavailable', 'unsupported_platform', 'invalid_request', 'session_invalid', 'capacity', 'forbidden',
    'not_text', 'too_large', 'conflict', 'operation_mismatch', 'operation_not_found', 'persistence_failure', 'draft_stale', 'scope_invalid', 'proposal_invalid', 'projection_unavailable', 'protected_span_invalid'
  ]), message: z.string(), receipt: objectEditingReceiptSchema.optional() }).strict()
])

export type ObjectEditingRequest = z.infer<typeof objectEditingRequestSchema>
export type ObjectEditingResponse = z.infer<typeof objectEditingResponseSchema>

// Main-only export lane. Deliberately excluded from ObjectEditingRequest so the
// Renderer cannot retrieve export snapshots through the generic object bridge.
export const objectExportBindingSchema = z.object({ sessionId, objectId: revision, threadId, baseRevision: revision, draftVersion: sessionId }).strict()
export const objectExportSnapshotRequestSchema = objectExportBindingSchema.extend({ action: z.literal('export-snapshot') }).strict()
export const objectExportSnapshotSchema = objectExportBindingSchema.extend({
  workspace: text(1_572_864).refine(value => /^(?:\/|[A-Za-z]:[\\/]|\\\\)/.test(value) && !value.includes('\0')),
  path: text(1_572_864).refine(value => value.length > 0 && !value.includes('\0')),
  content, contentDigest: revision
}).strict()
export const objectExportSnapshotResponseSchema = z.object({ ok: z.literal(true), snapshot: objectExportSnapshotSchema }).strict()
export type ObjectExportBinding = z.infer<typeof objectExportBindingSchema>
export type ObjectExportSnapshot = z.infer<typeof objectExportSnapshotSchema>
