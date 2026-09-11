import { z } from 'zod'
import { MODEL_ENDPOINT_FORMATS } from './model-endpoint-format.js'

export const ModelExecutionSourceSchema = z.enum([
  'thread',
  'subagent-profile',
  'explicit-input',
  'session',
  'runtime-default'
])
export type ModelExecutionSource = z.infer<typeof ModelExecutionSourceSchema>

export const ModelExecutionRefSchema = z.object({
  providerId: z.string().min(1),
  modelId: z.string().min(1),
  variant: z.string().min(1).optional(),
  endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional(),
  baseUrlFingerprint: z.string().min(1).optional(),
  customFullEndpointFingerprint: z.string().min(1).optional(),
  capabilityFingerprint: z.string().min(1).optional(),
  source: ModelExecutionSourceSchema,
  resolvedAt: z.string().min(1)
}).strict()
export type ModelExecutionRef = z.infer<typeof ModelExecutionRefSchema>
