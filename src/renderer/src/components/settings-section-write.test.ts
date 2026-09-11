import { describe, expect, it } from 'vitest'
import { writeInlineCompletionModelOptions } from './settings-section-write'

describe('write inline completion model options', () => {
  it('keeps the writing model list scoped to the inherited provider', () => {
    const options = writeInlineCompletionModelOptions([
      'MiniMax-M2',
      'MiniMax-M3',
      'MiniMax-M2'
    ])

    expect(options).toEqual(['MiniMax-M2', 'MiniMax-M3'])
    expect(options).not.toContain('deepseek-v4-pro')
    expect(options).not.toContain('deepseek-v4-flash')
  })

  it('uses built-in defaults only when the provider has no models', () => {
    expect(writeInlineCompletionModelOptions([])).toEqual([
      'deepseek-v4-pro',
      'deepseek-v4-flash'
    ])
  })
})
