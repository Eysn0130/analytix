import { describe, expect, it } from 'vitest'
import {
  projectModelReasoningEffortV1,
  validateOptionalModelReasoningEffortV1
} from './model-reasoning-effort'

describe('model reasoning effort authority', () => {
  it('preserves only the exact closed model effort set', () => {
    for (const effort of ['auto', 'off', 'low', 'medium', 'high', 'max']) {
      expect(projectModelReasoningEffortV1(effort)).toBe(effort)
      expect(validateOptionalModelReasoningEffortV1(effort)).toBe(effort)
    }
    expect(validateOptionalModelReasoningEffortV1(undefined)).toBeUndefined()
    expect(validateOptionalModelReasoningEffortV1('')).toBeUndefined()
  })

  it('rejects aliases without reflecting provider-originated bytes', () => {
    const sentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    for (const effort of [' high ', 'HIGH', 'adaptive', sentinel, 1, {}]) {
      expect(projectModelReasoningEffortV1(effort)).toBeUndefined()
      try {
        validateOptionalModelReasoningEffortV1(effort)
        throw new Error('expected invalid effort rejection')
      } catch (error) {
        expect(String(error)).not.toContain(sentinel)
        expect(String(error)).toContain('reasoning effort is invalid')
      }
    }
  })
})
