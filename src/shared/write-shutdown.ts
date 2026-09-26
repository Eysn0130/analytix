import { z } from 'zod'

export const WRITE_SHUTDOWN_REQUEST = 'write:shutdown-request'
export const WRITE_SHUTDOWN_ACK = 'write:shutdown-ack'
export const writeShutdownRequestSchema = z.object({
  requestId: z.string().uuid(),
  phase: z.enum(['prepare', 'cancel'])
}).strict()
export const writeShutdownResultSchema = z.discriminatedUnion('result', [
  z.object({ result: z.literal('ready') }).strict(),
  z.object({ result: z.literal('blocked'), reason: z.enum(['composing', 'exporting', 'conflict', 'save_unconfirmed', 'unavailable']) }).strict()
])
export const writeShutdownAckSchema = z.object({
  ...writeShutdownRequestSchema.shape,
  outcome: writeShutdownResultSchema
}).strict()
export type WriteShutdownRequest = z.infer<typeof writeShutdownRequestSchema>
export type WriteShutdownResult = z.infer<typeof writeShutdownResultSchema>
export type WriteShutdownHandler = (request: WriteShutdownRequest) => Promise<WriteShutdownResult>
