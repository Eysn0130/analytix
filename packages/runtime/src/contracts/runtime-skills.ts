import { z } from 'zod'

const Count = z.number().int().nonnegative().max(1_000_000_000)
const SkillId = z.string().regex(/^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/)

export function publicRuntimeSkillDisplayNameV2(id: string): string {
  return id
    .split(/[._-]+/)
    .filter(Boolean)
    .map((part) => `${part.slice(0, 1).toUpperCase()}${part.slice(1)}`)
    .join(' ')
}

export const PublicRuntimeSkillV2 = z
  .object({
    id: SkillId,
    name: z.string().min(1).max(256),
    scope: z.enum(['project', 'global']),
    legacy: z.boolean()
  })
  .strict()
  .superRefine((value, context) => {
    if (value.name !== publicRuntimeSkillDisplayNameV2(value.id)) {
      context.addIssue({ code: 'custom', message: 'public skill name must be derived from its id' })
    }
  })

export const RuntimeSkillsResponseV2 = z
  .object({
    schemaVersion: z.literal(2),
    enabled: z.boolean(),
    available: z.boolean(),
    reasonCode: z.enum(['available', 'disabled_by_config', 'unavailable']),
    configuredRootCount: Count,
    skillCount: Count,
    validationErrorCount: Count,
    skills: z.array(PublicRuntimeSkillV2).max(100_000)
  })
  .strict()
  .superRefine((value, context) => {
    if (value.skillCount !== value.skills.length) {
      context.addIssue({ code: 'custom', message: 'skill count does not match the public catalog' })
    }
    if (!value.enabled && value.skillCount !== 0) {
      context.addIssue({ code: 'custom', message: 'disabled skill catalog cannot publish skills' })
    }
    if (value.skillCount > 0 && value.configuredRootCount === 0) {
      context.addIssue({ code: 'custom', message: 'published skills require a configured root' })
    }
    if (new Set(value.skills.map((skill) => skill.id)).size !== value.skills.length) {
      context.addIssue({ code: 'custom', message: 'public skill ids must be unique' })
    }
    const expectedAvailable = value.enabled && value.skillCount > 0
    const expectedReason = expectedAvailable
      ? 'available'
      : value.enabled
        ? 'unavailable'
        : 'disabled_by_config'
    if (value.available !== expectedAvailable || value.reasonCode !== expectedReason) {
      context.addIssue({ code: 'custom', message: 'skill catalog state fields are inconsistent' })
    }
  })

export type PublicRuntimeSkillV2 = z.infer<typeof PublicRuntimeSkillV2>
export type RuntimeSkillsResponseV2 = z.infer<typeof RuntimeSkillsResponseV2>
