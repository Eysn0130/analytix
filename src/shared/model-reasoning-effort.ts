import {
  MODEL_REASONING_EFFORTS,
  type ModelReasoningEffort
} from './app-settings-types'

const MODEL_REASONING_EFFORT_SET: ReadonlySet<string> = new Set(MODEL_REASONING_EFFORTS)

function isModelReasoningEffort(value: unknown): value is ModelReasoningEffort {
  return typeof value === 'string' && MODEL_REASONING_EFFORT_SET.has(value)
}

export function projectModelReasoningEffortV1(value: unknown): ModelReasoningEffort | undefined {
  return isModelReasoningEffort(value) ? value : undefined
}

export function validateOptionalModelReasoningEffortV1(value: unknown): ModelReasoningEffort | undefined {
  if (value === undefined || value === '') return undefined
  const effort = projectModelReasoningEffortV1(value)
  if (!effort) throw new Error('reasoning effort is invalid')
  return effort
}
