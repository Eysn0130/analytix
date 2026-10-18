import { z } from 'zod'

export const GoalRefSchemaV1 = z.object({
  id: z.string().trim().min(1), revision: z.number().int().positive().safe()
}).strict()
export type GoalRefV1 = z.infer<typeof GoalRefSchemaV1>
export const GoalRoundSourceSchemaV1 = z.object({
  kind: z.literal('goal'), goalId: z.string().trim().min(1),
  revision: z.number().int().positive().safe(), round: z.number().int().positive().safe()
}).strict()
export type GoalRoundSourceV1 = z.infer<typeof GoalRoundSourceSchemaV1>
export const GoalActivationSchemaV1 = GoalRefSchemaV1.extend({
  activation: z.enum(['armed', 'disarmed'])
}).strict()
export type GoalActivationV1 = z.infer<typeof GoalActivationSchemaV1>
