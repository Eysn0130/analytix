import { z } from 'zod'

// Protected-local only. No producer, public event, history or model contract
// may embed this payload. Wire input carries identities, never body/path.
export const TOOL_RESULT_LOCAL_DISPLAY_PATH_V1 = '/v1/local-display/tool-result-snapshot'
export const TOOL_RESULT_LOCAL_DISPLAY_MAX_BYTES_V1 = 256 << 10
const recordId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const text = (bytes: number) => z.string().refine(value =>
  !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= bytes)
export const toolResultLocalSelectorSchemaV1 = z.object({
  threadId: recordId, turnId: recordId,
  callId: z.string().regex(/^call_host_[a-f0-9]{64}$/),
  resultItemId: z.string().regex(/^item_result_[a-f0-9]{64}$/)
}).strict()
export const toolResultLocalViewRequestSchemaV1 = toolResultLocalSelectorSchemaV1.extend({
  viewId: z.string().uuid()
}).strict()
export const toolResultLocalEffectRequestSchemaV1 = toolResultLocalViewRequestSchemaV1.extend({
  snapshotDigest: digest
}).strict()
export const toolResultLocalCaptureSchemaV1 = z.object({
  kind: z.enum(['shell', 'read']),
  status: z.enum(['completed', 'failed', 'timeout', 'canceled', 'unknown']),
  body: text(32 << 10), label: text(1024).refine(value => !value.includes('\0')),
  exitCode: z.number().int().nonnegative().optional(),
  durationMs: z.number().int().nonnegative(), truncated: z.boolean(),
  startLine: z.number().int().nonnegative(), endLine: z.number().int().nonnegative(),
  totalLines: z.number().int().nonnegative()
}).strict().superRefine((capture, context) => {
  if (capture.kind === 'shell'
    ? capture.startLine !== 0 || capture.endLine !== 0 || capture.totalLines !== 0
    : capture.exitCode !== undefined || capture.durationMs !== 0 || capture.status !== 'completed' ||
      capture.endLine < capture.startLine || capture.totalLines > 0 && capture.endLine > capture.totalLines && capture.body !== '') {
    context.addIssue({ code: 'custom', message: 'Invalid protected capture.' })
  }
})
export const toolResultLocalDisplaySchemaV1 = z.object({
  schemaVersion: z.literal(1), snapshotDigest: digest,
  resultItemId: z.string().regex(/^item_result_[a-f0-9]{64}$/),
  toolName: z.enum(['bash', 'read', 'read_file']), capture: toolResultLocalCaptureSchemaV1
}).strict().refine(value => (value.toolName === 'bash') === (value.capture.kind === 'shell'))
export const toolResultLocalFailureSchemaV1 = z.object({
  ok: z.literal(false), code: z.enum(['unavailable', 'unsupported', 'canceled'])
}).strict()
export const toolResultLocalViewResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), display: toolResultLocalDisplaySchemaV1 }).strict(),
  toolResultLocalFailureSchemaV1
])
export const toolResultLocalEffectResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true) }).strict(), toolResultLocalFailureSchemaV1
])
export type ToolResultLocalSelectorV1 = z.infer<typeof toolResultLocalSelectorSchemaV1>
export type ToolResultLocalViewRequestV1 = z.infer<typeof toolResultLocalViewRequestSchemaV1>
export type ToolResultLocalEffectRequestV1 = z.infer<typeof toolResultLocalEffectRequestSchemaV1>
export type ToolResultLocalDisplayV1 = z.infer<typeof toolResultLocalDisplaySchemaV1>
export type ToolResultLocalViewResultV1 = z.infer<typeof toolResultLocalViewResultSchemaV1>
export type ToolResultLocalEffectResultV1 = z.infer<typeof toolResultLocalEffectResultSchemaV1>
