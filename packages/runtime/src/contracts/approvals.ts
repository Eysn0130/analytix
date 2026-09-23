import { z } from 'zod'

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

export const UserInputResolutionResponse = z.discriminatedUnion('status', [
  z.object({
    inputId: z.string().min(1),
    status: z.literal('submitted'),
    answers: z.array(z.record(z.string(), z.string()))
  }).strict(),
  z.object({
    inputId: z.string().min(1),
    status: z.literal('cancelled')
  }).strict()
])
export type UserInputResolutionResponse = z.infer<typeof UserInputResolutionResponse>
