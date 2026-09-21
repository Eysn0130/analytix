import { describe, expect, it } from 'vitest'
import {
  PublicRuntimeSkillV2,
  RuntimeSkillsResponseV2,
  publicRuntimeSkillDisplayNameV2
} from './runtime-skills.js'

describe('runtime skills public contract', () => {
  it('accepts the Go hosted-only projection without configured skill roots', () => {
    const response = {
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 0,
      skillCount: 1,
      validationErrorCount: 0,
      skills: [{ id: 'analytix-documents', name: 'Analytix Documents', scope: 'global', legacy: false }]
    }
    // Matches Go WithOfficeSkills + SkillResponse: installed contributions
    // do not add filesystem paths to the configured roots inventory.
    expect(RuntimeSkillsResponseV2.parse(response)).toEqual(response)
    // The public record has no origin/authority field. Parsing cannot infer
    // a configured-root requirement from the spelling of a skill id.
    const installed = { ...response, skills: [{ id: 'installed-review', name: 'Installed Review', scope: 'global', legacy: false }] }
    expect(RuntimeSkillsResponseV2.parse(installed)).toEqual(installed)
    for (const invalid of [
      { ...response, skillCount: 2 },
      { ...response, enabled: false, available: false, reasonCode: 'disabled_by_config' },
      { ...response, available: false },
      { ...response, reasonCode: 'unavailable' },
      { ...response, skills: [response.skills[0], response.skills[0]], skillCount: 2 },
      { ...response, configuredRootCount: -1 },
      { ...response, roots: ['/private/installed'] },
      { ...response, skills: [{ ...response.skills[0], entryPath: '/private/installed/SKILL.md' }] }
    ]) {
      expect(RuntimeSkillsResponseV2.safeParse(invalid).success).toBe(false)
    }
  })

  it('accepts only the closed host projection', () => {
    const skill = {
      id: 'deep_review.v2',
      name: 'Deep Review V2',
      scope: 'project' as const,
      legacy: false
    }
    expect(PublicRuntimeSkillV2.parse(skill)).toEqual(skill)
    expect(publicRuntimeSkillDisplayNameV2(skill.id)).toBe(skill.name)
    expect(RuntimeSkillsResponseV2.parse({
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 2,
      skillCount: 1,
      validationErrorCount: 1,
      skills: [skill]
    }).skills).toEqual([skill])
  })

  it('rejects private catalog fields and inconsistent state', () => {
    const base = {
      id: 'review',
      name: 'Review',
      scope: 'global' as const,
      legacy: false
    }
    for (const privateField of ['root', 'path', 'entryPath', 'description', 'model', 'allowedTools']) {
      expect(PublicRuntimeSkillV2.safeParse({ ...base, [privateField]: '/Users/private/skill' }).success)
        .toBe(false)
    }
    expect(PublicRuntimeSkillV2.safeParse({ ...base, name: '/Users/private/skill' }).success).toBe(false)
    expect(RuntimeSkillsResponseV2.safeParse({
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 1,
      skillCount: 2,
      validationErrorCount: 0,
      skills: [base]
    }).success).toBe(false)
    expect(RuntimeSkillsResponseV2.safeParse({
      schemaVersion: 2,
      enabled: false,
      available: false,
      reasonCode: 'disabled_by_config',
      configuredRootCount: 1,
      skillCount: 1,
      validationErrorCount: 0,
      skills: [base]
    }).success).toBe(false)
    expect(RuntimeSkillsResponseV2.safeParse({
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 1,
      skillCount: 2,
      validationErrorCount: 0,
      skills: [base, base]
    }).success).toBe(false)
    expect(RuntimeSkillsResponseV2.safeParse({
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 0,
      skillCount: 0,
      validationErrorCount: 0,
      skills: []
    }).success).toBe(false)
  })
})
