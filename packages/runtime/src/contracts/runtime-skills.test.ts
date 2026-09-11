import { describe, expect, it } from 'vitest'
import {
  PublicRuntimeSkillV2,
  RuntimeSkillsResponseV2,
  publicRuntimeSkillDisplayNameV2
} from './runtime-skills.js'

describe('runtime skills public contract', () => {
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
      skillCount: 1,
      validationErrorCount: 0,
      skills: [base]
    }).success).toBe(false)
  })
})
