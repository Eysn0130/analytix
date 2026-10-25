import { z } from 'zod'

// Desktop intent/receipt only. RuntimeToken and typed-local authority stay in Main.
const identity = z.string().min(1).max(128).regex(/^[A-Za-z0-9_.:-]+$/)
export const childInputClientMessageIdSchemaV1 = z.string().regex(
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
)
const message = z.string().min(1).refine(value => value === value.trim() && [...value].length <= 4000)
const scope = { threadId: identity, jobId: identity }
export const childInputInspectRequestSchemaV1 = z.object({ mode: z.literal('inspect'), ...scope }).strict()
export const childInputSubmitRequestSchemaV1 = z.object({
  mode: z.literal('start_or_steer'), ...scope,
  expectedRunId: identity, expectedChildTurnId: identity, message,
  clientMessageId: childInputClientMessageIdSchemaV1
}).strict()
export const childInputRequestSchemaV1 = z.discriminatedUnion('mode', [
  childInputInspectRequestSchemaV1, childInputSubmitRequestSchemaV1
])
export type ChildInputRequestV1 = z.infer<typeof childInputRequestSchemaV1>
export type ChildInputSubmitRequestV1 = z.infer<typeof childInputSubmitRequestSchemaV1>

const capabilities = z.object({
  send: z.enum(['steer', 'start', 'disabled']), stop: z.boolean(),
  runId: identity.optional(), childThreadId: identity.optional(), childTurnId: identity.optional(),
  reasonCode: z.enum(['authority_unavailable', 'run_conflict', 'pending_start', 'paused',
    'turn_terminalizing', 'thread_transition', 'new_turn_required', 'case_risk_raise',
    'context_changed', 'child_input_unknown', 'run_stopped', 'run_unavailable',
    'runtime_shutting_down', 'input_cancelled']).optional()
}).strict().refine(value => {
  const ids = [value.runId, value.childThreadId, value.childTurnId]
  return (ids.every(Boolean) || ids.every(id => id === undefined) && value.send === 'disabled' && !value.stop) &&
    !(value.send === 'steer' && !value.stop || value.send === 'start' && value.stop)
})
const receipt = z.object({
  action: z.enum(['start', 'steer']), admission: z.enum(['queued', 'admitted']),
  clientMessageId: childInputClientMessageIdSchemaV1, jobId: identity,
  childThreadId: identity, childTurnId: identity, messageId: identity
}).strict().refine(value => value.action !== 'start' || value.admission === 'admitted')
export const childInputResponseSchemaV1 = z.object({
  schemaVersion: z.literal(1), capabilities,
  receipt: receipt.optional(),
  code: z.enum(['conflict', 'new_turn_required', 'forbidden', 'not_found', 'validation_error',
    'child_input_unknown', 'runtime_shutting_down', 'input_cancelled']).optional()
}).strict().refine(value => !value.receipt || !value.code &&
  (!value.capabilities.childThreadId || value.receipt.childThreadId === value.capabilities.childThreadId))
export type ChildInputResponseV1 = z.infer<typeof childInputResponseSchemaV1>
export type ChildInputReceiptV1 = z.infer<typeof receipt>

export function childInputReceiptMatchesV1(
  receipt: ChildInputReceiptV1, request: ChildInputSubmitRequestV1, childThreadId: string
): boolean {
  return receipt.clientMessageId === request.clientMessageId && receipt.childThreadId === childThreadId &&
    (receipt.action === 'steer'
      ? receipt.jobId === request.expectedRunId && receipt.childTurnId === request.expectedChildTurnId
      : receipt.jobId !== request.expectedRunId && receipt.childTurnId !== request.expectedChildTurnId)
}
