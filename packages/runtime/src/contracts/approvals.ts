import { z } from 'zod'

// Original Question owner, independent of a late reply's steering target.
export const UserInputActionSelectorSchemaV1 = z.object({
  threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..')),
  turnId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..')),
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/),
  requestId: z.string().regex(/^input_[a-f0-9]{64}$/)
}).strict().refine(value => value.itemId === 'item_' + value.requestId)
export type UserInputActionSelectorV1 = z.infer<typeof UserInputActionSelectorSchemaV1>

export const UserInputTimeoutSecondsSchema = z.number().int().refine(value => value === -1 || value >= 1 && value <= 2147483)
export const UserInputTimedWaitSchemaV1 = z.object({
  timeoutSeconds: UserInputTimeoutSecondsSchema,
  deadlineAt: z.string().datetime().nullable()
}).strict().refine(value => (value.timeoutSeconds === -1) === (value.deadlineAt === null))
export const UserInputWaitSnapshotSchemaV1 = z.object({ remainingMs: z.number().int().nonnegative().nullable() }).strict()
export const UserInputWaitContinueRequestSchemaV1 = z.object({ action: z.literal('continue'), selector: UserInputActionSelectorSchemaV1.optional() }).strict()
// This is a scheduling acknowledgement; observe the Go resolution for continued.
export const UserInputWaitScheduledResponseSchemaV1 = z.object({ inputId: z.string().regex(/^input_[a-f0-9]{64}$/), scheduled: z.literal(true) }).strict()
export type UserInputTimedWaitV1 = z.infer<typeof UserInputTimedWaitSchemaV1>
export type UserInputWaitSnapshotV1 = z.infer<typeof UserInputWaitSnapshotSchemaV1>
export type UserInputWaitContinueRequestV1 = z.infer<typeof UserInputWaitContinueRequestSchemaV1>
export type UserInputWaitScheduledResponseV1 = z.infer<typeof UserInputWaitScheduledResponseSchemaV1>

export const UserInputLateReplyRequestSchemaV1 = z.object({
  clientUserMessageId: z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/),
  expectedTurnId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
}).strict()
export const UserInputLateReplyDeliverySchemaV1 = z.object({
  clientUserMessageId: UserInputLateReplyRequestSchemaV1.shape.clientUserMessageId,
  turnId: UserInputLateReplyRequestSchemaV1.shape.expectedTurnId,
  state: z.enum(['queued', 'accepted', 'discarded'])
}).strict()
export const UserInputLateReplyResponseSchemaV1 = z.object({
  inputId: z.string().regex(/^input_[a-f0-9]{64}$/),
  status: z.literal('continued'),
  lateReply: UserInputLateReplyDeliverySchemaV1
}).strict()
export type UserInputLateReplyRequestV1 = z.infer<typeof UserInputLateReplyRequestSchemaV1>
export type UserInputLateReplyDeliveryV1 = z.infer<typeof UserInputLateReplyDeliverySchemaV1>
export type UserInputLateReplyResponseV1 = z.infer<typeof UserInputLateReplyResponseSchemaV1>

export const ApprovalDecisionRequest = z.object({
  decision: z.enum(['allow', 'deny']),
  /** Optional human-readable reason stored alongside the resolution. */
  reason: z.string().optional()
})
export type ApprovalDecisionRequest = z.infer<typeof ApprovalDecisionRequest>

export const ApprovalDecisionResponse = z.object({
  approvalId: z.string().min(1),
  decision: z.enum(['allow', 'deny']),
  status: z.enum(['allowed', 'denied', 'expired'])
})
export type ApprovalDecisionResponse = z.infer<typeof ApprovalDecisionResponse>

const LegacyUserInputAnswerSchema = z.object({
  id: z.string().min(1),
  label: z.string().optional(),
  value: z.string()
}).strict()

export const StructuredUserInputAnswerSchema = z.discriminatedUnion('kind', [
  z.object({
    id: z.string().min(1),
    kind: z.literal('answer'),
    optionIds: z.array(z.string().min(1)).max(3),
    customText: z.string().refine(value => value.trim().length > 0, 'custom text must not be blank').optional()
  }).strict(),
  z.object({ id: z.string().min(1), kind: z.literal('skip') }).strict()
]).refine(answer => answer.kind === 'skip' || answer.optionIds.length > 0 || Boolean(answer.customText?.trim()), {
  message: 'answer requires a selected option or custom text'
})

// Batch shape is checked here; the Go gate matches identities and selection
// cardinality against the signed pending request before claiming continuation.
export const UserInputResolutionRequest = z.union([
  z.object({
    selector: UserInputActionSelectorSchemaV1.optional(),
    answers: z.array(StructuredUserInputAnswerSchema).min(1).max(3),
    cancelled: z.literal(false).optional(),
    lateReply: UserInputLateReplyRequestSchemaV1.optional()
  }).strict(),
  z.object({
    selector: UserInputActionSelectorSchemaV1.optional(),
    answers: z.array(LegacyUserInputAnswerSchema).min(1).max(3),
    cancelled: z.literal(false).optional(),
    lateReply: UserInputLateReplyRequestSchemaV1.optional()
  }).strict(),
  z.object({ cancelled: z.literal(true), selector: UserInputActionSelectorSchemaV1.optional() }).strict()
]).superRefine((request, ctx) => {
  if ('answers' in request && new Set(request.answers.map(answer => answer.id)).size !== request.answers.length) {
    ctx.addIssue({ code: 'custom', path: ['answers'], message: 'question answers must be unique' })
  }
  if ('answers' in request) for (const [index, answer] of request.answers.entries()) {
    if ('optionIds' in answer && new Set(answer.optionIds).size !== answer.optionIds.length) {
      ctx.addIssue({ code: 'custom', path: ['answers', index, 'optionIds'], message: 'selected options must be unique' })
    }
  }
})

export const UserInputMetadataResolutionResponse = z.object({
  inputId: z.string().min(1),
  status: z.enum(['submitted', 'cancelled'])
}).strict()

export const UserInputResolutionResponse = z.discriminatedUnion('status', [
  z.object({
    inputId: z.string().min(1),
    status: z.literal('submitted'),
    // Retained only for legacy submissions; structured submissions return metadata.
    answers: z.array(z.record(z.string(), z.string())).optional()
  }).strict(),
  z.object({
    inputId: z.string().min(1),
    status: z.literal('cancelled')
  }).strict()
])
export type UserInputResolutionResponse = z.infer<typeof UserInputResolutionResponse>
