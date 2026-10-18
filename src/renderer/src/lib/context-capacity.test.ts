import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../agent/types'
import type { ModelProviderModelGroup } from '@shared/analytix-api'
import {
  buildContextCapacity,
  resolveComposerContextWindow,
  estimateBlockTokens,
  estimateTokensFromText
} from './context-capacity'

describe('estimateTokensFromText', () => {
  it('returns 0 for empty input', () => {
    expect(estimateTokensFromText('')).toBe(0)
  })

  it('treats latin text as roughly 4 chars per token', () => {
    expect(estimateTokensFromText('a'.repeat(40))).toBe(10)
  })

  it('treats CJK characters as roughly one token each', () => {
    expect(estimateTokensFromText('上下文容量')).toBe(5)
  })

  it('counts astral-plane characters once, not twice', () => {
    // An emoji is a single surrogate pair; latin heuristic -> ceil(1/4) = 1.
    expect(estimateTokensFromText('😀')).toBe(1)
  })
})

describe('buildContextCapacity', () => {
  it('uses the measured total and keeps categories + free summing to the window', () => {
    const cap = buildContextCapacity({
      windowTokens: 200_000,
      lastTurnInputTokens: 138_389,
      messageTokens: 90_000,
      toolCount: 40,
      skillCount: 12
    })
    expect(cap.hasMeasuredTotal).toBe(true)
    expect(cap.usedTokens).toBe(138_389)
    expect(cap.freeTokens).toBe(200_000 - 138_389)
    const sum = cap.categories.reduce((acc, c) => acc + c.tokens, 0) + cap.freeTokens
    // Allow ±1 token of rounding drift across the five categories.
    expect(Math.abs(sum - cap.windowTokens)).toBeLessThanOrEqual(2)
    expect(cap.usedRatio).toBeCloseTo(138_389 / 200_000, 5)
  })

  it('clamps a plausible measured total that exceeds the window', () => {
    const cap = buildContextCapacity({
      windowTokens: 100_000,
      lastTurnInputTokens: 150_000,
      messageTokens: 30_000,
      toolCount: 10,
      skillCount: 0
    })
    expect(cap.usedTokens).toBe(100_000)
    expect(cap.freeTokens).toBe(0)
    expect(cap.usedRatio).toBe(1)
  })

  it('falls back when measured prompt tokens are implausibly inflated', () => {
    const cap = buildContextCapacity({
      windowTokens: 100_000,
      lastTurnInputTokens: 150_000,
      messageTokens: 0,
      toolCount: 10,
      skillCount: 0
    })
    expect(cap.hasMeasuredTotal).toBe(false)
    expect(cap.usedTokens).toBe(3980)
    expect(cap.freeTokens).toBe(96_020)
  })

  it('falls back to a pure estimate when there is no measured turn', () => {
    const cap = buildContextCapacity({
      windowTokens: 200_000,
      lastTurnInputTokens: null,
      messageTokens: 2,
      toolCount: 20,
      skillCount: 5
    })
    expect(cap.hasMeasuredTotal).toBe(false)
    const prefix = cap.categories
      .filter((c) => c.key !== 'messages')
      .reduce((acc, c) => acc + c.tokens, 0)
    expect(prefix).toBeGreaterThan(0)
    expect(cap.usedTokens).toBeGreaterThan(0)
    expect(cap.usedTokens).toBeLessThan(cap.windowTokens)
  })

  it('scales a pure estimate down so it never overflows the window', () => {
    const cap = buildContextCapacity({
      windowTokens: 1000,
      lastTurnInputTokens: null,
      messageTokens: 25_000,
      toolCount: 100,
      skillCount: 50
    })
    expect(cap.usedTokens).toBeLessThanOrEqual(cap.windowTokens)
    expect(cap.freeTokens).toBeGreaterThanOrEqual(0)
  })
})

describe('estimateBlockTokens', () => {
  it('estimates model-visible text per block kind', () => {
    expect(estimateBlockTokens({ kind: 'user', id: 'u1', text: 'hello world!' } as ChatBlock)).toBe(3)
    expect(
      estimateBlockTokens({
        kind: 'tool',
        id: 't1',
        name: 'read',
        status: 'done',
        detail: 'file contents here'
      } as unknown as ChatBlock)
    ).toBeGreaterThan(0)
  })

  it('discounts embedded base64 payloads in tool details', () => {
    const block: ChatBlock = {
      kind: 'tool',
      id: 'tool-1',
      summary: 'computer_use screenshot',
      status: 'success',
      detail: JSON.stringify({
        kind: 'computer_screenshot',
        images: [{ data_base64: 'A'.repeat(100_000), mime_type: 'image/png' }]
      })
    }
    expect(estimateBlockTokens(block)).toBeLessThan(2_000)
  })

  it('returns 0 for blocks with no model-visible text', () => {
    expect(
      estimateBlockTokens({
        kind: 'approval',
        id: 'p1',
        requestId: 'req',
        toolName: 'bash',
        createdAt: ''
      } as unknown as ChatBlock)
    ).toBe(0)
  })
})


describe('composer closure capacity identity', () => {
  const profile = (contextWindowTokens?: number, aliases?: string[]) => ({ contextWindowTokens, aliases,
    inputModalities: ['text' as const], outputModalities: ['text' as const], supportsToolCalling: true, messageParts: [] })
  const groups: ModelProviderModelGroup[] = [
    { providerId: 'one', label: 'One', modelIds: ['shared'], modelProfiles: { shared: profile(4096, ['alias']), unknown: profile() } },
    { providerId: 'two', label: 'Two', modelIds: ['shared'], modelProfiles: { shared: profile(8192) } }
  ]
  it('uses only the selected provider profile, including its aliases', () => {
    expect(resolveComposerContextWindow({ model: 'shared', providerId: 'one', groups })).toBe(4096)
    expect(resolveComposerContextWindow({ model: 'shared', providerId: 'two', groups })).toBe(8192)
    expect(resolveComposerContextWindow({ model: 'alias', providerId: 'one', groups })).toBe(4096)
    expect(resolveComposerContextWindow({ model: 'shared', providerId: 'missing', groups })).toBeUndefined()
  })
  it('never borrows a runtime denominator from a different provider or model', () => {
    const runtimeModel = { id: 'shared', providerId: 'two', contextWindowTokens: 32000 }
    expect(resolveComposerContextWindow({ model: 'unknown', providerId: 'one', groups, runtimeModel })).toBeUndefined()
    expect(resolveComposerContextWindow({ model: 'unknown', providerId: 'two', groups, runtimeModel })).toBeUndefined()
    expect(resolveComposerContextWindow({ model: 'auto', providerId: 'one', groups, runtimeModel })).toBeUndefined()
    expect(resolveComposerContextWindow({ model: 'auto', groups, runtimeModel })).toBe(32000)
  })
  it('accepts matched runtime aliases but hides missing or invalid capacity', () => {
    const noCapacity = [{ ...groups[0], modelProfiles: { shared: profile(undefined, ['alias']) } }]
    expect(resolveComposerContextWindow({ model: 'alias', providerId: 'one', groups: noCapacity,
      runtimeModel: { id: 'shared', providerId: 'one', contextWindowTokens: 16000 } })).toBe(16000)
    for (const value of [undefined, 0, -1, Infinity, NaN]) {
      expect(resolveComposerContextWindow({ model: 'alias', providerId: 'one', groups: noCapacity,
        runtimeModel: { id: 'shared', providerId: 'one', contextWindowTokens: value } })).toBeUndefined()
    }
  })
})
