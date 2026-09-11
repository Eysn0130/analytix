import { describe, expect, it } from 'vitest'
import { resolveWriteQuickActions } from './quick-actions'

// Stand-in translator: built-in keys resolve to a recognizable string so we can
// assert the localized fallback is used when label/prompt are empty.
const t = (key: string): string => `t:${key}`

describe('resolveWriteQuickActions', () => {
  it('fills empty built-in label/prompt from the localized defaults and keeps the built-in mode', () => {
    const resolved = resolveWriteQuickActions(
      [{ id: 'polish', label: '', prompt: '', mode: 'edit' }],
      t
    )
    expect(resolved).toEqual([
      { id: 'polish', label: 't:writeQuickActionPolish', prompt: 't:writeQuickActionPolishPrompt', mode: 'edit' }
    ])
  })

  it('prefers explicit label/prompt over the defaults', () => {
    const resolved = resolveWriteQuickActions(
      [{ id: 'polish', label: '提升写作', prompt: '改得更好', mode: 'edit' }],
      t
    )
    expect(resolved[0]).toMatchObject({ label: '提升写作', prompt: '改得更好', mode: 'edit' })
  })

  it('drops custom actions that have no resolvable prompt', () => {
    const resolved = resolveWriteQuickActions(
      [{ id: 'custom-1', label: 'Only label', prompt: '', mode: 'chat' }],
      t
    )
    expect(resolved).toEqual([])
  })

  it('keeps custom actions with a label and prompt', () => {
    const resolved = resolveWriteQuickActions(
      [{ id: 'custom-1', label: 'Translate', prompt: 'Translate to English', mode: 'chat' }],
      t
    )
    expect(resolved).toEqual([
      { id: 'custom-1', label: 'Translate', prompt: 'Translate to English', mode: 'chat' }
    ])
  })
})
