import { z } from 'zod'
import { nativeOfficeContentSchema, nativeOfficeSelectionScopeSchema, nativeOfficeProposalSchema, nativeOfficeLocalReviewSchema } from '../../packages/runtime/src/contracts/native-office-editing'

const id = z.string().uuid()
const revision = z.string().regex(/^[a-f0-9]{64}$/)
const path = z.string().min(1).max(4096).refine(value => !['\0','\r','\n'].some(character=>value.includes(character)))
const version = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const target = { editorId: id, version }
const content = nativeOfficeContentSchema.refine(value => value.kind === 'docx')
export const docxEditorRequestSchema = z.discriminatedUnion('action', [
  z.object({ action:z.literal('open'), workspace:path, path, threadId:z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/) }).strict(),
  z.object({ action:z.literal('begin'), ...target }).strict(),
  z.object({ action:z.literal('changed'), ...target }).strict(),
  z.object({ action:z.literal('save'), ...target, content }).strict(),
  z.object({ action:z.literal('status'), editorId:id }).strict(),
  z.object({ action:z.literal('close'), editorId:id, discard:z.boolean().default(false) }).strict(),
  // Public upstream captures stay in the bound renderer instance. Main issues a
  // new opaque token for this explicit DOCX producer; no native token is accepted.
  z.object({ action:z.literal('capture'), ...target, instanceId:id, captureId:id,
    editable:z.boolean() }).strict(),
  z.object({ action:z.literal('applied'), ...target, instanceId:id, captureId:id }).strict(),
  z.object({ action:z.literal('revoke'), editorId:id }).strict(),
  z.object({ action:z.literal('proposals'), editorId:id }).strict(),
  z.object({ action:z.literal('decide'), ...target, scopeId:z.string().regex(/^[a-f0-9]{48}$/),
    proposalId:z.string().regex(/^[a-f0-9]{48}$/), accept:z.boolean(), instanceId:id, captureId:id }).strict()
])
export const docxEditorViewSchema = z.object({
  editorId:id, workspace:path, path, threadId:z.string(), objectId:revision, revision,
  version, phase:z.enum(['ready','editing','applying','unknown','conflict']), dirty:z.boolean(),
  content, scope:nativeOfficeSelectionScopeSchema.omit({sessionId:true}).optional(),
  proposals:z.array(nativeOfficeProposalSchema).max(16).default([]),
  reviews:z.array(nativeOfficeLocalReviewSchema).max(16).default([])
}).strict()
export const docxEditorResponseSchema = z.object({
  ok:z.boolean(), view:docxEditorViewSchema.nullable(),
  error:z.enum(['invalid_request','unavailable','package_changed','forbidden','conflict','unknown','unsaved_changes','stale_selection']).optional(),
  replacement:z.string().max(4096).optional()
}).strict()
export type DocxEditorRequest = z.input<typeof docxEditorRequestSchema>
export type DocxEditorView = z.infer<typeof docxEditorViewSchema>
export type DocxEditorResponse = z.infer<typeof docxEditorResponseSchema>

export const docxEditorCaptureRequestSchema=z.object({requestId:id,editorId:id,version,revision}).strict()
export const docxEditorCaptureSchema=z.object({requestId:id,editorId:id,version,revision,instanceId:id,captureId:id,
  text:z.string().min(1).max(4096),target:z.record(z.string(),z.unknown()).refine(value=>JSON.stringify(value).length<=16384)
}).strict()
export type DocxEditorCaptureRequest=z.infer<typeof docxEditorCaptureRequestSchema>
export type DocxEditorCapture=z.infer<typeof docxEditorCaptureSchema>
