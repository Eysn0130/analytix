import { describe, expect, it } from 'vitest'
import type { ModelProviderModelGroup } from '@shared/analytix-api'
import { buildContextCapacity, resolveComposerContextWindow, type ContextCapacityInput } from './context-capacity'

const input: ContextCapacityInput = {
  windowTokens: 1000, model: 'selected-model', providerId: 'selected-provider', threadId: 'thread',
  lastTurnUsage: { threadId: 'thread', snapshot: {
    inputTokens: 138389, model: 'sample-model', providerId: 'sample-provider'
  } }
}

describe('historical context samples and selected windows', () => {
  it('keeps an input sample above the selected window intact without inventing occupancy or free space', () => {
    expect(buildContextCapacity(input)).toEqual({
      windowTokens: 1000, model: 'selected-model', providerId: 'selected-provider',
      lastRequest: { inputTokens: 138389, model: 'sample-model', providerId: 'sample-provider', toolDefinitionTokens: null }
    })
  })

  it.each([
    { model: 'sample-model', providerId: 'sample-provider', windowTokens: 4096 },
    { model: 'another-model', providerId: 'sample-provider', windowTokens: 8192 },
    { model: 'sample-model', providerId: 'another-provider', windowTokens: 16384 }
  ])('does not reinterpret the historical sample when selecting $providerId / $model', selection => {
    const result = buildContextCapacity({ ...input, ...selection })
    expect(result.windowTokens).toBe(selection.windowTokens)
    expect(result.lastRequest).toEqual(buildContextCapacity(input).lastRequest)
    expect(result).not.toHaveProperty('usedRatio')
    expect(result).not.toHaveProperty('freeTokens')
    expect(result).not.toHaveProperty('categories')
  })

  it('does not borrow another thread sample or its tool estimate', () => {
    expect(buildContextCapacity({ ...input, threadId: 'another-thread', lastTurnUsage: {
      threadId: 'thread', snapshot: { ...input.lastTurnUsage!.snapshot,
        cacheDiagnostics: { toolSchemaEstimator: 'utf8_bytes_div4', toolSchemaTokens: 25 } }
    } }).lastRequest).toBeNull()
    expect(buildContextCapacity({ ...input, threadId: null }).lastRequest).toBeNull()
  })

  it('keeps missing sample identities unknown instead of filling them from the current selection', () => {
    const result = buildContextCapacity({ ...input, lastTurnUsage: { threadId: 'thread', snapshot: { inputTokens: 42 } } })
    expect(result.lastRequest).toEqual({ inputTokens: 42, model: undefined, providerId: undefined, toolDefinitionTokens: null })
  })

  it('has no local fallback without a request sample', () => {
    expect(buildContextCapacity({ ...input, windowTokens: undefined, lastTurnUsage: null })).toEqual({
      windowTokens: null, model: 'selected-model', providerId: 'selected-provider', lastRequest: null
    })
  })

  it.each([undefined, 0, -1, NaN, Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1])(
    'treats unavailable or invalid window %s as unknown', windowTokens => {
      const result = buildContextCapacity({ ...input, windowTokens })
      expect(result.windowTokens).toBeNull()
      expect(result.lastRequest?.inputTokens).toBe(138389)
    }
  )

  it.each([0, -1, NaN, Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1])(
    'does not turn defaulted zero or invalid input %s into a measured sample', inputTokens => {
      expect(buildContextCapacity({ ...input, lastTurnUsage: { threadId: 'thread', snapshot: { inputTokens } } })
        .lastRequest?.inputTokens).toBeNull()
    }
  )

  it('retains the explicit tool estimator as an independent historical diagnostic even above the input sample', () => {
    const result = buildContextCapacity({ ...input, lastTurnUsage: { threadId: 'thread', snapshot: {
      inputTokens: 10, cacheDiagnostics: { toolSchemaEstimator: 'utf8_bytes_div4', toolSchemaTokens: 1234 }
    } } })
    expect(result.lastRequest).toMatchObject({ inputTokens: 10, toolDefinitionTokens: 1234 })
    expect(result).not.toHaveProperty('categories')
  })

  it('does not use tool counts or a diagnostic without its estimator as token composition', () => {
    expect(buildContextCapacity({ ...input, lastTurnUsage: { threadId: 'thread', snapshot: {
      inputTokens: 10, cacheDiagnostics: { toolSchemaTokens: 1234, toolCount: 90 }
    } } }).lastRequest?.toolDefinitionTokens).toBeNull()
  })

  it.each([undefined, -1, NaN, Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1])(
    'hides invalid tool estimates %s', toolSchemaTokens => {
      expect(buildContextCapacity({ ...input, lastTurnUsage: { threadId: 'thread', snapshot: {
        inputTokens: 10, cacheDiagnostics: { toolSchemaEstimator: 'utf8_bytes_div4', toolSchemaTokens }
      } } }).lastRequest?.toolDefinitionTokens).toBeNull()
    }
  )

  it('preserves an explicit zero tool estimate without treating a defaulted input zero as measured', () => {
    expect(buildContextCapacity({ ...input, lastTurnUsage: { threadId: 'thread', snapshot: {
      inputTokens: 0, cacheDiagnostics: { toolSchemaEstimator: 'utf8_bytes_div4', toolSchemaTokens: 0 }
    } } }).lastRequest).toMatchObject({ inputTokens: null, toolDefinitionTokens: 0 })
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
