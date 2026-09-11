import { jsonResponse, type JsonResponse } from '../response.js'
import {
  PublicRuntimeSkillV2,
  RuntimeSkillsResponseV2,
  publicRuntimeSkillDisplayNameV2
} from '../../contracts/runtime-skills.js'
import type { ServerRuntime } from './server-runtime.js'

export async function listSkills(runtime: ServerRuntime): Promise<JsonResponse> {
  const diagnostics = runtime.skills
    ? await runtime.skills()
    : {
        enabled: false,
        roots: [],
        skills: [],
        validationErrors: [],
        lastActivations: []
      }
  const skills = (diagnostics.enabled && diagnostics.roots.length > 0 ? diagnostics.skills : []).flatMap((skill) => {
    const parsed = PublicRuntimeSkillV2.safeParse({
      id: skill.id,
      name: publicRuntimeSkillDisplayNameV2(skill.id),
      scope: skill.scope,
      legacy: skill.legacy
    })
    return parsed.success ? [parsed.data] : []
  })
  const available = diagnostics.enabled && skills.length > 0
  return jsonResponse(RuntimeSkillsResponseV2.parse({
    schemaVersion: 2,
    enabled: diagnostics.enabled,
    available,
    reasonCode: available ? 'available' : diagnostics.enabled ? 'unavailable' : 'disabled_by_config',
    configuredRootCount: diagnostics.roots.length,
    skillCount: skills.length,
    validationErrorCount: diagnostics.validationErrors.length,
    skills
  }))
}
