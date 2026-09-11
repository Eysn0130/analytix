import { describe, expect, it } from 'vitest'
import { ANALYTIX_SYSTEM_PROMPT } from '../src/prompt/analytix-system-prompt.js'

describe('analytix system prompt', () => {
  it('keeps model-visible product identity under Analytix ownership', () => {
    expect(ANALYTIX_SYSTEM_PROMPT).toContain('You are Analytix')
    expect(ANALYTIX_SYSTEM_PROMPT).toContain('Connect Phone')
    expect(ANALYTIX_SYSTEM_PROMPT).not.toMatch(/\b(?:Claw|Kun|Reasonix)\b/)
  })
})
