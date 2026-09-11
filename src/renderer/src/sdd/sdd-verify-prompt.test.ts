import { describe, expect, it } from 'vitest'
import { SDD_VERIFY_INTRO, buildSddVerifyPrompt, isSddVerifyPrompt } from './sdd-verify-prompt'

describe('buildSddVerifyPrompt', () => {
  const prompt = buildSddVerifyPrompt({
    workspaceRoot: '/tmp/ws',
    draftRelativePath: '.analytixsdd/requirements/abc/requirement.md',
    planRelativePath: '.analytixsdd/plan/sdd-abc.md'
  })

  it('keeps the intro and core in-place verification instructions', () => {
    expect(prompt).toContain(SDD_VERIFY_INTRO)
    expect(prompt).toContain('Requirement file: .analytixsdd/requirements/abc/requirement.md')
    expect(prompt).toContain('change `- [ ]` to `- [x]`')
    expect(prompt).toContain('{verified}')
  })

  it('injects the intended-vs-implemented and test-scenarios framework guidance', () => {
    expect(prompt).toContain('## Intended-vs-implemented gap audit')
    expect(prompt).toContain('cite BOTH sides')
    expect(prompt).toContain('## Test scenarios per criterion')
  })

  it('detects its own intro', () => {
    expect(isSddVerifyPrompt(prompt)).toBe(true)
    expect(isSddVerifyPrompt('unrelated text')).toBe(false)
  })
})
