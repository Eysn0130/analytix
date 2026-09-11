import { existsSync } from 'node:fs'
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../../shared/app-settings'
import {
  deleteGuiSkillPackage,
  guiSkillRootsForRuntime,
  listGuiSkillRoots,
  listGuiSkills,
  saveGuiSkillFile
} from './skill-service'
import type { BundledFundsMaterializationBindingV1 } from '../runtime/bundled-funds-materialization'

vi.mock('node:os', async (importOriginal) => {
  const actual = await importOriginal<typeof import('node:os')>()
  return {
    ...actual,
    homedir: () => process.env.ANALYTIX_SKILL_TEST_HOME || actual.homedir()
  }
})

describe('skill-service', () => {
  let tempRoot = ''

  beforeEach(async () => {
    tempRoot = await mkdtemp(join(tmpdir(), 'gui-skills-'))
    process.env.ANALYTIX_SKILL_TEST_HOME = tempRoot
  })

  afterEach(async () => {
    delete process.env.ANALYTIX_SKILL_TEST_HOME
    await rm(tempRoot, { recursive: true, force: true })
  })

  it('discovers project Codex skills from the active workspace', async () => {
    const workspaceRoot = join(tempRoot, 'workspace')
    const skillRoot = join(workspaceRoot, '.codex', 'skills', 'openspec-apply-change')
    await mkdir(skillRoot, { recursive: true })
    await writeFile(join(skillRoot, 'SKILL.md'), [
      '---',
      'name: openspec-apply-change',
      'description: Implement tasks from an OpenSpec change.',
      '---',
      '',
      'Implement tasks from an OpenSpec change.'
    ].join('\n'), 'utf8')

    const result = await listGuiSkills(createSettings(workspaceRoot), workspaceRoot)

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.skills).toContainEqual(expect.objectContaining({
      id: 'openspec-apply-change',
      name: 'Openspec Apply Change',
      description: 'Implement tasks from an OpenSpec change.',
      scope: 'project'
    }))
  })

  it('saves and deletes a GUI-managed skill package', async () => {
    const workspaceRoot = join(tempRoot, 'workspace-save')
    const skillRoot = join(workspaceRoot, '.codex', 'skills')
    const save = await saveGuiSkillFile(skillRoot, 'demo-skill', [
      '---',
      'name: demo-skill',
      'description: Demo skill.',
      '---',
      '',
      'Body.'
    ].join('\n'))

    expect(save.ok).toBe(true)
    if (!save.ok) return
    expect(existsSync(save.path)).toBe(true)

    const remove = await deleteGuiSkillPackage(skillRoot, 'demo-skill')

    expect(remove.ok).toBe(true)
    expect(existsSync(join(skillRoot, 'demo-skill'))).toBe(false)
  })

  it('deletes discovered skill package roots even when manifest id differs from folder name', async () => {
    const skillPackageRoot = join(tempRoot, 'custom-skills', 'folder-name')
    await mkdir(skillPackageRoot, { recursive: true })
    await writeFile(join(skillPackageRoot, 'skill.json'), JSON.stringify({
      id: 'manifest-id',
      name: 'Manifest Skill',
      entry: 'SKILL.md'
    }), 'utf8')
    await writeFile(join(skillPackageRoot, 'SKILL.md'), 'Body.', 'utf8')

    const remove = await deleteGuiSkillPackage(skillPackageRoot, 'manifest-id')

    expect(remove.ok).toBe(true)
    expect(existsSync(skillPackageRoot)).toBe(false)
  })

  it('keeps legacy SKILL.md entries with Chinese frontmatter names distinct', async () => {
    const workspaceRoot = join(tempRoot, 'workspace-cn')
    const skillRoot = join(workspaceRoot, '.agents', 'skills')
    const tddRoot = join(skillRoot, 'tdd')
    const reviewRoot = join(skillRoot, 'code-review')
    await mkdir(tddRoot, { recursive: true })
    await mkdir(reviewRoot, { recursive: true })
    await writeFile(join(tddRoot, 'SKILL.md'), [
      '---',
      'name: 测试驱动开发(TDD)',
      'description: 用测试先行推进实现。',
      '---',
      '',
      '# TDD',
      '',
      '先写失败测试，再实现。'
    ].join('\n'), 'utf8')
    await writeFile(join(reviewRoot, 'SKILL.md'), [
      '---',
      'name: 代码审查',
      'description: 检查回归风险。',
      '---',
      '',
      '# Review',
      '',
      '关注正确性和测试。'
    ].join('\n'), 'utf8')

    const result = await listGuiSkills(createSettings(workspaceRoot), workspaceRoot)

    expect(result.ok).toBe(true)
    if (!result.ok) return
    const projectSkills = result.skills.filter((skill) => skill.root.startsWith(skillRoot))
    expect(projectSkills).toHaveLength(2)
    expect(projectSkills).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'tdd',
        name: '测试驱动开发(TDD)',
        description: '用测试先行推进实现。'
      }),
      expect.objectContaining({
        id: 'code-review',
        name: '代码审查',
        description: '检查回归风险。'
      })
    ]))
    expect(projectSkills.map((skill) => skill.id)).not.toContain('skill')
  })

  it('detects workspace .claude/skills as a common directory and counts its skills', async () => {
    const workspaceRoot = join(tempRoot, 'ws-claude')
    const skillRoot = join(workspaceRoot, '.claude', 'skills', 'demo')
    await mkdir(skillRoot, { recursive: true })
    await writeFile(join(skillRoot, 'SKILL.md'), [
      '---', 'name: demo', 'description: Demo skill.', '---', '', 'Body.'
    ].join('\n'), 'utf8')

    const result = await listGuiSkillRoots(createSettings(workspaceRoot), workspaceRoot)

    expect(result.ok).toBe(true)
    if (!result.ok) return
    const claude = result.roots.find((root) => root.labelKey === 'pluginSkillRootWorkspaceClaude')
    expect(claude).toMatchObject({
      scope: 'project',
      source: 'common',
      exists: true,
      enabled: true,
      skillCount: 1
    })
    expect(comparable(claude?.path ?? '')).toBe(comparable(join(workspaceRoot, '.claude', 'skills')))
  })

  it('discovers and counts skills symlinked into .claude/skills (e.g. cc switch)', async (ctx) => {
    const workspaceRoot = join(tempRoot, 'ws-symlink')
    // cc switch stores the real skill files in its own config dir...
    const realSkill = join(tempRoot, 'cc-config', 'skills', 'linked-skill')
    await mkdir(realSkill, { recursive: true })
    await writeFile(join(realSkill, 'SKILL.md'), [
      '---', 'name: linked-skill', 'description: Linked via symlink.', '---', '', 'Body.'
    ].join('\n'), 'utf8')
    // ...and symlinks the per-skill directory into .claude/skills.
    const claudeSkills = join(workspaceRoot, '.claude', 'skills')
    await mkdir(claudeSkills, { recursive: true })
    try {
      await symlink(realSkill, join(claudeSkills, 'linked-skill'), 'dir')
    } catch {
      // Symlink creation can be unprivileged (e.g. Windows) — skip there.
      ctx.skip()
      return
    }

    const settings = createSettings(workspaceRoot)
    const result = await listGuiSkills(settings, workspaceRoot)
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.skills).toContainEqual(expect.objectContaining({
      id: 'linked-skill',
      name: 'Linked Skill',
      description: 'Linked via symlink.',
      scope: 'project'
    }))

    const roots = await listGuiSkillRoots(settings, workspaceRoot)
    expect(roots.ok).toBe(true)
    if (!roots.ok) return
    const claude = roots.roots.find((root) => root.labelKey === 'pluginSkillRootWorkspaceClaude')
    expect(claude?.skillCount).toBe(1)
  })

  it('omits a directory disabled via disabledDirs from runtime roots but still lists it', async () => {
    const workspaceRoot = join(tempRoot, 'ws-toggle')
    const claudeSkill = join(workspaceRoot, '.claude', 'skills', 'demo')
    const agentsSkill = join(workspaceRoot, '.agents', 'skills', 'demo2')
    await mkdir(claudeSkill, { recursive: true })
    await mkdir(agentsSkill, { recursive: true })
    await writeFile(join(claudeSkill, 'SKILL.md'), ['---', 'name: demo', '---', '', 'Body.'].join('\n'), 'utf8')
    await writeFile(join(agentsSkill, 'SKILL.md'), ['---', 'name: demo2', '---', '', 'Body.'].join('\n'), 'utf8')

    const settings = createSettings(workspaceRoot)
    settings.claw.skills.disabledDirs = ['workspace-claude']

    const runtimeRoots = (await guiSkillRootsForRuntime(settings, workspaceRoot)).map((root) =>
      comparable(root.path)
    )
    expect(runtimeRoots).not.toContain(comparable(join(workspaceRoot, '.claude', 'skills')))
    expect(runtimeRoots).toContain(comparable(join(workspaceRoot, '.agents', 'skills')))

    const list = await listGuiSkillRoots(settings, workspaceRoot)
    expect(list.ok).toBe(true)
    if (!list.ok) return
    const claude = list.roots.find((root) => root.labelKey === 'pluginSkillRootWorkspaceClaude')
    expect(claude?.enabled).toBe(false)
  })

  it('omits discovered Analytix-owned plugin roots disabled by path', async () => {
    const workspaceRoot = join(tempRoot, 'ws-plugin-toggle')
    const pluginRoot = join(tempRoot, '.analytix', 'plugins', 'cache', 'github', '1.0', 'skills')
    await mkdir(join(pluginRoot, 'review'), { recursive: true })
    await writeFile(join(pluginRoot, 'review', 'SKILL.md'), ['---', 'name: review', '---'].join('\n'), 'utf8')

    const settings = createSettings(workspaceRoot)
    expect((await guiSkillRootsForRuntime(settings, workspaceRoot)).map((root) => comparable(root.path)))
      .toContain(comparable(pluginRoot))

    settings.claw.skills.disabledDirs = [pluginRoot]
    expect((await guiSkillRootsForRuntime(settings, workspaceRoot)).map((root) => comparable(root.path)))
      .not.toContain(comparable(pluginRoot))
  })

  it('does not treat Hub projection copies as executable plugin skills', async () => {
    const workspaceRoot = join(tempRoot, 'ws-managed-projection')
    const projectedRoot = join(tempRoot, '.analytix', 'skills', 'quick-fact')
    const verifiedRoot = join(
      tempRoot,
      '.analytix',
      'plugins',
      'cache',
      'analytix-hub',
      'analytix-fund-analysis',
      '0.16.17',
      'skills',
      'quick-fact'
    )
    await mkdir(projectedRoot, { recursive: true })
    await mkdir(verifiedRoot, { recursive: true })
    await writeFile(join(projectedRoot, 'SKILL.md'), [
      '---', 'name: stale-quick-fact', 'description: stale projection', '---', '', 'stale'
    ].join('\n'), 'utf8')
    await writeFile(join(projectedRoot, '.analytix-hub-skill.json'), JSON.stringify({
      managedBy: 'analytix-hub',
      platform: 'mac-arm64',
      pluginName: 'analytix-fund-analysis',
      skillName: 'quick-fact',
      skillPath: 'skills/quick-fact/SKILL.md',
      sourceKind: 'plugin',
      version: '0.16.15'
    }), 'utf8')
    await writeFile(join(verifiedRoot, 'SKILL.md'), [
      '---', 'name: current-quick-fact', 'description: current plugin root', '---', '', 'current'
    ].join('\n'), 'utf8')

    const unbound = await listGuiSkills(createSettings(workspaceRoot), workspaceRoot, null)
    expect(unbound.ok).toBe(true)
    if (unbound.ok) {
      expect(unbound.skills).not.toContainEqual(expect.objectContaining({ id: 'quick-fact' }))
      expect(unbound.skills).not.toContainEqual(expect.objectContaining({ id: 'stale-quick-fact' }))
    }

    const result = await listGuiSkills(
      createSettings(workspaceRoot),
      workspaceRoot,
      fakeFundsMaterializationBinding(dirname(dirname(verifiedRoot)))
    )

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.skills).not.toContainEqual(expect.objectContaining({ id: 'stale-quick-fact' }))
    expect(result.skills).toContainEqual(expect.objectContaining({
      id: 'quick-fact',
      name: 'Current Quick Fact',
      description: 'current plugin root'
    }))
  })

  it('preserves valid managed projections owned by other plugins', async () => {
    const workspaceRoot = join(tempRoot, 'ws-other-managed-projection')
    const projectedRoot = join(tempRoot, '.analytix', 'skills', 'other-projection')
    await mkdir(projectedRoot, { recursive: true })
    await writeFile(join(projectedRoot, 'SKILL.md'), [
      '---', 'name: other-managed', 'description: another plugin projection', '---', '', 'other'
    ].join('\n'), 'utf8')
    await writeFile(join(projectedRoot, '.analytix-hub-skill.json'), JSON.stringify({
      managedBy: 'analytix-hub',
      platform: 'mac-arm64',
      pluginName: 'other-plugin',
      skillName: 'other-projection',
      skillPath: 'skills/other-projection/SKILL.md',
      sourceKind: 'plugin',
      version: '1.2.3'
    }), 'utf8')

    const result = await listGuiSkills(createSettings(workspaceRoot), workspaceRoot)

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.skills).toContainEqual(expect.objectContaining({
      id: 'other-projection',
      name: 'Other Managed',
      description: 'another plugin projection'
    }))
  })

  it('does not discover system Codex plugin cache roots by default', async () => {
    const workspaceRoot = join(tempRoot, 'ws-system-codex-plugin-cache')
    const codexPluginRoot = join(tempRoot, '.codex', 'plugins', 'cache', 'github', '1.0', 'skills')
    const analytixPluginRoot = join(tempRoot, '.analytix', 'plugins', 'cache', 'github', '1.0', 'skills')
    await mkdir(join(codexPluginRoot, 'system-review'), { recursive: true })
    await mkdir(join(analytixPluginRoot, 'owned-review'), { recursive: true })
    await writeFile(join(codexPluginRoot, 'system-review', 'SKILL.md'), ['---', 'name: system-review', '---'].join('\n'), 'utf8')
    await writeFile(join(analytixPluginRoot, 'owned-review', 'SKILL.md'), ['---', 'name: owned-review', '---'].join('\n'), 'utf8')

    const roots = (await guiSkillRootsForRuntime(createSettings(workspaceRoot), workspaceRoot)).map((root) =>
      comparable(root.path)
    )

    expect(roots).toContain(comparable(analytixPluginRoot))
    expect(roots).not.toContain(comparable(codexPluginRoot))
  })

  it('prefers Analytix Computer Use over the legacy Computer Use plugin skill root', async () => {
    const workspaceRoot = join(tempRoot, 'ws-computer-use-preference')
    const analytixRoot = join(tempRoot, '.analytix', 'plugins', 'cache', 'analytix-hub', 'analytix-computer-use', '0.2.3', 'skills')
    const legacyRoot = join(tempRoot, '.analytix', 'plugins', 'cache', 'analytix-hub', 'computer-use', '1.0.857', 'skills')
    await mkdir(join(analytixRoot, 'analytix-computer-use'), { recursive: true })
    await mkdir(join(legacyRoot, 'computer-use'), { recursive: true })
    await writeFile(join(analytixRoot, 'analytix-computer-use', 'SKILL.md'), [
      '---',
      'name: analytix-computer-use',
      '---'
    ].join('\n'), 'utf8')
    await writeFile(join(legacyRoot, 'computer-use', 'SKILL.md'), [
      '---',
      'name: computer-use',
      '---'
    ].join('\n'), 'utf8')

    const roots = (await guiSkillRootsForRuntime(createSettings(workspaceRoot), workspaceRoot)).map((root) =>
      comparable(root.path)
    )

    expect(roots).toContain(comparable(analytixRoot))
    expect(roots).not.toContain(comparable(legacyRoot))
  })

  function comparable(path: string): string {
    return path.replace(/\\/g, '/').replace(/\/+$/g, '').toLowerCase()
  }

  function fakeFundsMaterializationBinding(
    activePluginRoot: string,
    pluginVersion = '0.16.17'
  ): BundledFundsMaterializationBindingV1 {
    return {
      pluginName: 'analytix-fund-analysis',
      pluginVersion,
      activePluginRoot,
      publishable: false,
      factToolsEnabled: false
    } as unknown as BundledFundsMaterializationBindingV1
  }

  function createSettings(workspaceRoot: string): AppSettingsV1 {
    return {
      version: 1,
      locale: 'en',
      theme: 'system',
      uiFontScale: 'small',
      provider: defaultModelProviderSettings(),
      runtime: defaultAnalytixRuntimeSettings(),
      workspaceRoot,
      log: { enabled: false, retentionDays: 7 },
      notifications: { turnComplete: true },
      appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
      keyboardShortcuts: defaultKeyboardShortcuts(),
      write: defaultWriteSettings(),
      claw: defaultClawSettings(),
      schedule: defaultScheduleSettings(),
      guiUpdate: { channel: 'stable' },
      codePromptPrefix: '',
      disabledSkillIds: []
    }
  }
})
