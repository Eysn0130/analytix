import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { buildSkillToolProviders } from '../src/tool-test-support/tool/skill-tool-provider.js'
import { AnalytixCapabilitiesConfig } from '../src/contracts/capabilities.js'
import { SkillRuntime } from '../src/skills/skill-runtime.js'

describe('buildSkillToolProviders', () => {
  let root = ''

  beforeEach(async () => {
    root = await mkdtemp(join(tmpdir(), 'analytix-skill-tool-'))
  })

  afterEach(async () => {
    await rm(root, { recursive: true, force: true })
  })

  async function runtimeWithSkill(): Promise<SkillRuntime> {
    const dir = join(root, 'demo')
    await mkdir(dir, { recursive: true })
    await writeFile(join(dir, 'skill.json'), JSON.stringify({ id: 'demo', name: 'Demo' }), 'utf8')
    await writeFile(join(dir, 'SKILL.md'), 'demo body instructions', 'utf8')
    const config = AnalytixCapabilitiesConfig.parse({
      skills: { enabled: true, roots: [root], legacySkillMd: true }
    })
    return SkillRuntime.create(config.skills)
  }

  it('returns no provider when no skills are loaded', async () => {
    const runtime = await SkillRuntime.create({ enabled: false, roots: [], legacySkillMd: true })
    expect(buildSkillToolProviders(runtime)).toEqual([])
    expect(buildSkillToolProviders(undefined)).toEqual([])
  })

  it('exposes a load_skill tool that returns the skill body', async () => {
    const runtime = await runtimeWithSkill()
    const [provider] = buildSkillToolProviders(runtime)
    expect(provider?.id).toBe('skill')
    const tool = provider?.tools.find((candidate) => candidate.name === 'load_skill')
    expect(tool).toBeDefined()

    const ok = await tool!.execute({ skill_id: '$demo' }, {} as never)
    expect(ok.isError).toBeFalsy()
    expect(ok.output).toMatchObject({ skillId: 'demo', name: 'Demo' })
    expect((ok.output as { instruction: string }).instruction).toContain('demo body instructions')

    const missing = await tool!.execute({ skill_id: 'nope' }, {} as never)
    expect(missing.isError).toBe(true)

    const blank = await tool!.execute({ skill_id: '   ' }, {} as never)
    expect(blank.isError).toBe(true)
  })
})
