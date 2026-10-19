import { createElement } from 'react'
import { describe, expect, it } from 'vitest'
import { getComposerCommandRows, type SlashCommand, type SlashCommandId } from './floating-composer-commands'

function command(id: SlashCommandId, overrides: Partial<SlashCommand> = {}): SlashCommand {
  return { id, title: id, description: '', keywords: [], icon: createElement('svg'), ...overrides }
}

describe('composer command catalog presentation', () => {
  it('orders the real built-ins into Add and Commands with separate contribution groups', () => {
    const catalog = [
      command('plugin:browser'), command('export'), command('worktree'), command('plan'),
      command('new'), command('skill:report'), command('model'), command('feedback'),
      command('permission'), command('file'), command('compact'), command('goal'),
      command('workspace-files'), command('attachments')
    ]
    const rows = getComposerCommandRows(catalog, '')
    expect(rows.map(({ id, section }) => [id, section])).toEqual([
      ['file', 'add'], ['goal', 'add'], ['plan', 'add'], ['feedback', 'add'],
      ['compact', 'commands'], ['permission', 'commands'], ['model', 'commands'], ['export', 'commands'],
      ['new', 'extensions'], ['workspace-files', 'extensions'], ['attachments', 'extensions'],
      ['skill:report', 'skills'], ['plugin:browser', 'plugins'], ['worktree', 'worktree']
    ])
    expect(catalog[0].id).toBe('plugin:browser')
    expect(catalog.every(item => !('section' in item))).toBe(true)
  })

  it('does not manufacture absent capabilities or discard disabled explanations', () => {
    const catalog = [command('feedback', { disabled: true, disabledReason: 'Feedback is unavailable.' })]
    expect(getComposerCommandRows(catalog, '')).toEqual([{ ...catalog[0], section: 'add' }])
    expect(getComposerCommandRows([], '')).toEqual([])
  })

  it('keeps input actions out of inline discovery unless explicitly permitted', () => {
    const catalog = [
      command('file'), command('compact', { inputOnly: true }),
      command('skill:report', { inputOnly: false })
    ]
    expect(getComposerCommandRows(catalog, '', 'inline').map(item => item.id)).toEqual(['skill:report'])
    expect(getComposerCommandRows(catalog, 'file', 'inline')).toEqual([])
    expect(getComposerCommandRows(catalog, '').map(item => item.id)).toEqual(['file', 'compact', 'skill:report'])
  })

  it('ranks prefixes before subsequence matches and removes headings for a nonempty query', () => {
    const rows = getComposerCommandRows([
      command('new', { canonicalName: 'unplanned', title: 'unplanned' }),
      command('plan', { title: 'Plan' }),
      command('goal', { title: 'Goal' })
    ], 'PL')
    expect(rows.map(item => item.id)).toEqual(['plan', 'new'])
    expect(rows.every(item => item.section === undefined)).toBe(true)
  })

  it('preserves source order when candidate alignments tie', () => {
    const catalog = [
      command('skill:second', { canonicalName: 'alpha', title: 'alpha' }),
      command('skill:first', { canonicalName: 'alpha', title: 'alpha' })
    ]
    expect(getComposerCommandRows(catalog, 'al').map(item => item.id)).toEqual(['skill:second', 'skill:first'])
  })

  it('uses ordered fuzzy matches, localized titles and existing aliases without searching descriptions', () => {
    const catalog = [
      command('compact'), command('plan', { title: '计划' }),
      command('permission', { keywords: ['permissions', '权限'] }),
      command('new', { description: 'Compact the old conversation.' })
    ]
    expect(getComposerCommandRows(catalog, 'cpt').map(item => item.id)).toEqual(['compact'])
    expect(getComposerCommandRows(catalog, 'tpc')).toEqual([])
    expect(getComposerCommandRows(catalog, '计划').map(item => item.id)).toEqual(['plan'])
    expect(getComposerCommandRows(catalog, 'permissions').map(item => item.id)).toEqual(['permission'])
    expect(getComposerCommandRows(catalog, 'compact').map(item => item.id)).toEqual(['compact'])
  })
})
