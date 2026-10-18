import { describe, expect, it } from 'vitest'
import {
  extractRequestedSkillIds,
  extractComposerPluginMentions,
  extractComposerSkillMentions,
  formatComposerPluginMentionToken,
  formatComposerSkillMentionToken,
  getComposerAtMentionAtCursor,
  getComposerMentionDisplayPlan,
  getUserMessageDisplayParts,
  removeComposerMentionToken,
  removeComposerMentionTokens,
  replaceComposerAtMentionInInput
} from './composer-mentions'

describe('composer mention helpers', () => {
  it('keeps long incomplete escaped mentions editable without backtracking', () => {
    const incomplete = '$[](' + String.raw`\!`.repeat(5000)
    expect(getComposerMentionDisplayPlan(incomplete)).toMatchObject({ hasMentions: false, editableValue: incomplete })
  })

  it('parses escaped URI punctuation only when the reference is actually closed', () => {
    expect(extractComposerPluginMentions(String.raw`@[Tool](plugin://report\)part)`)[0]).toMatchObject({ pluginId: 'report)part' })
    expect(extractComposerPluginMentions(String.raw`@[Tool](plugin://report\)`)).toEqual([])
  })

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


describe('sent user text reference projection', () => {
  it('decorates only message-loaded slash skills with whitespace boundaries', () => {
    const input = '/audit /other /audit.md /audit/path /audit。 mail@example.test\n/audit end'
    const parts = getUserMessageDisplayParts(input, ['audit'])
    expect(parts.filter(part => part.kind === 'loaded-skill').map(part => input.slice(part.start, part.end))).toEqual(['/audit', '/audit'])
    expect(parts.map(part => input.slice(part.start, part.end)).join('')).toBe(input)
    expect(getUserMessageDisplayParts('/audit', []).every(part => part.kind === 'text')).toBe(true)
    expect(parts.filter(part => part.kind === 'file')).toEqual([])
    expect(getComposerMentionDisplayPlan('@src/file.ts /audit')).toMatchObject({ hasMentions: false, editableValue: '@src/file.ts /audit' })
  })

  it('keeps wire plugin/skill precedence and labels while projecting adjacent files', () => {
    const plugin = formatComposerPluginMentionToken('Tool @nested', 'plugin')
    const skill = formatComposerSkillMentionToken('Skill @nested', 'audit')
    const parts = getUserMessageDisplayParts(`${plugin} ${skill} @src/index.ts,`, ['audit'])
    expect(parts.filter(part => part.kind !== 'text').map(part => part.kind)).toEqual(['plugin', 'skill', 'file'])
    expect(parts.find(part => part.kind === 'file')).toMatchObject({ path: 'src/index.ts', display: 'index.ts' })
    expect(parts.at(-1)).toMatchObject({ kind: 'text', text: ',' })
  })

  it('supports composer-escaped quoted files, directory decoration and long wrapping labels', () => {
    const long = 'long'.repeat(100)
    const input = String.raw`@"docs/my \"quoted\" file.txt" @src/ ` + `@${long}.txt！`
    const parts = getUserMessageDisplayParts(input)
    expect(parts.filter(part => part.kind === 'file')).toMatchObject([
      { path: 'docs/my "quoted" file.txt', display: 'my "quoted" file.txt', directory: false },
      { path: 'src/', display: 'src', directory: true },
      { path: `${long}.txt`, display: `${long}.txt`, directory: false }
    ])
    expect(parts.map(part => input.slice(part.start, part.end)).join('')).toBe(input)
    expect(getUserMessageDisplayParts('@"unfinished name').every(part => part.kind === 'text')).toBe(true)
  })
})

describe('explicit initial skill requests', () => {
  it('extracts existing selections and whitespace-bound slash names without rewriting input', () => {
    const text = '/review use $[Report](skill://report) /skill:audit /review';
    expect(extractRequestedSkillIds(text)).toEqual(['review', 'report', 'audit']);
    expect(text).toBe('/review use $[Report](skill://report) /skill:audit /review');
    expect(extractRequestedSkillIds('docs/review https://host/review (/review) /review, /Review /a_b')).toEqual([]);
    expect(extractRequestedSkillIds('$[ignored /fake](skill://report)')).toEqual(['report']);
    expect(extractRequestedSkillIds('@[Plugin /review label](plugin://plug)')).toEqual([]);
    expect(extractRequestedSkillIds('$[bad](skill://..%2Fsecret)')).toEqual([]);
  });
  it('decorates legacy slash selections only after successful item loading', () => {
    expect(getUserMessageDisplayParts('/skill:review', []).some(part => part.kind === 'loaded-skill')).toBe(false);
    expect(getUserMessageDisplayParts('/skill:review', ['review'])).toMatchObject([{kind:'loaded-skill',skillId:'review'}]);
  });
});
