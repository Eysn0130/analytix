import { z } from 'zod'

export const workspaceReadPath = '/v1/local-display/workspace-read'
const binding = z.string().regex(/^[a-f0-9]{64}$/)
const threadId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/)
export const workspaceReadRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('authorize'), threadId }).strict(),
  z.object({ action: z.literal('validate'), threadId, binding }).strict(),
  z.object({ action: z.literal('scan'), threadId, binding, includePdf: z.boolean() }).strict()
])
const file = z.object({
  path: z.string().min(1).max(4096).refine(value => !value.startsWith('/') && !value.includes('\\') &&
    !value.includes('\0') && !/[\r\n]/.test(value) &&
    value.split('/').every(part => part !== '' && part !== '.' && part !== '..')),
  kind: z.enum(['text', 'pdf']), revision: binding,
  content: z.string().max(24 * 1024 * 1024).regex(/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/)
}).strict()
export const workspaceReadSnapshotSchema = z.object({
  threadId, workspace: z.string().min(1).max(4096), binding,
  files: z.array(file).max(160).optional()
}).strict()
export const workspaceReadResponseSchema = z.union([
  z.object({ ok: z.literal(true), snapshot: workspaceReadSnapshotSchema }).strict(),
  z.object({ ok: z.literal(false), code: z.enum(['invalid_request', 'unavailable']) }).strict()
])
export type WorkspaceReadFile = z.infer<typeof file>
