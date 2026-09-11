import { describe, expect, it } from 'vitest'
import {
  extractComposerPluginMentions,
  extractComposerSkillMentions,
  formatComposerPluginMentionToken,
  formatComposerSkillMentionToken,
  getComposerAtMentionAtCursor,
  getComposerMentionDisplayPlan,
  removeComposerMentionToken,
  removeComposerMentionTokens,
  replaceComposerAtMentionInInput
} from './composer-mentions'

describe('composer mention helpers', () => {
  it('detects plain and quoted @ mentions at the cursor', () => {
    expect(getComposerAtMentionAtCursor('ask @plug', 'ask @plug'.length)).toEqual({
      start: 4,
      end: 9,
      query: 'plug',
      quoted: false
    })
    expect(getComposerAtMentionAtCursor('ask @"fund plugin', 'ask @"fund plugin'.length)).toEqual({
      start: 4,
      end: 17,
      query: 'fund plugin',
      quoted: true
    })
  })

  it('formats and extracts plugin mentions', () => {
    const token = formatComposerPluginMentionToken('Analytix Fund', 'analytix-fund-analysis')
    expect(token).toBe('@[Analytix Fund](plugin://analytix-fund-analysis)')
    expect(extractComposerPluginMentions(`use ${token} now`)).toEqual([
      {
        token,
        start: 4,
        end: 4 + token.length,
        label: 'Analytix Fund',
        pluginId: 'analytix-fund-analysis'
      }
    ])
  })

  it('formats and extracts skill mentions with Codex skill marker', () => {
    const token = formatComposerSkillMentionToken('Audit Skill', 'audit/skill')
    expect(token).toBe('$[Audit Skill](skill://audit%2Fskill)')
    expect(extractComposerSkillMentions(`run ${token}`)[0]).toMatchObject({
      token,
      label: 'Audit Skill',
      skillId: 'audit/skill'
    })
  })

  it('replaces an active @ mention and removes exact mention tokens', () => {
    const input = 'ask @fund please'
    const mention = getComposerAtMentionAtCursor(input, 'ask @fund'.length)
    expect(mention).not.toBeNull()
    const token = formatComposerPluginMentionToken('Fund', 'fund')
    const next = replaceComposerAtMentionInInput(input, mention!, token)
    expect(next.input).toBe('ask @[Fund](plugin://fund) please')
    expect(removeComposerMentionToken(next.input, token)).toBe('ask please')
  })

  it('strips multiple mention tokens for display text', () => {
    const plugin = formatComposerPluginMentionToken('Docs', 'documents')
    const skill = formatComposerSkillMentionToken('Audit', 'audit')
    expect(removeComposerMentionTokens(`${plugin} ${skill} summarize this`, [plugin, skill])).toBe(
      'summarize this'
    )
  })

  it('preserves mention token order in the display plan', () => {
    const token = formatComposerPluginMentionToken('Computer Use', 'computer-use')
    const plan = getComposerMentionDisplayPlan(`使用 ${token} 插件`)
    expect(plan.hasMentions).toBe(true)
    expect(plan.editableValue).toBe(' 插件')
    expect(plan.parts).toHaveLength(2)
    expect(plan.parts[0]).toMatchObject({
      kind: 'text',
      text: '使用 '
    })
    expect(plan.parts[1]).toMatchObject({
      kind: 'plugin',
      start: '使用 '.length
    })
  })
})
