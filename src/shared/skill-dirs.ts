export type SkillDirScope = 'project' | 'global'

export type CommonSkillDir = {
  /** Stable identifier used for UI options and the `disabledDirs` toggle key. */
  id: string
  scope: SkillDirScope
  /**
   * Path relative to the workspace root (project scope) or the user home
   * directory (global scope). Written with POSIX separators; callers join it
   * against the appropriate base with their platform's path utilities.
   */
  relativePath: string
  /** i18n key (renderer `common` namespace) for the human-readable label. */
  labelKey: string
}

/**
 * Common per-workspace skill directory conventions, in precedence order:
 * when two roots expose the same skill id the earlier one wins. `.agents`
 * leads because it is the primary convention surfaced in onboarding/docs.
 */
export const COMMON_WORKSPACE_SKILL_DIRS: readonly CommonSkillDir[] = [
  { id: 'workspace-agents', scope: 'project', relativePath: '.agents/skills', labelKey: 'pluginSkillRootWorkspaceAgents' },
  { id: 'workspace-claude', scope: 'project', relativePath: '.claude/skills', labelKey: 'pluginSkillRootWorkspaceClaude' },
  { id: 'workspace-codex', scope: 'project', relativePath: '.codex/skills', labelKey: 'pluginSkillRootWorkspaceCodex' },
  { id: 'workspace-skills', scope: 'project', relativePath: 'skills', labelKey: 'pluginSkillRootWorkspaceSkills' }
]

/**
 * Common global skill directory conventions (relative to the user home dir),
 * in precedence order. Project roots always take precedence over these.
 */
export const COMMON_GLOBAL_SKILL_DIRS: readonly CommonSkillDir[] = [
  { id: 'global-agents', scope: 'global', relativePath: '.agents/skills', labelKey: 'pluginSkillRootGlobalAgents' },
  { id: 'global-claude', scope: 'global', relativePath: '.claude/skills', labelKey: 'pluginSkillRootGlobalClaude' },
  { id: 'global-codex', scope: 'global', relativePath: '.codex/skills', labelKey: 'pluginSkillRootGlobalCodex' },
  { id: 'global-deepseek', scope: 'global', relativePath: '.analytix/skills', labelKey: 'pluginSkillRootGlobalDeepseek' }
]

/**
 * All common skill directory conventions, project roots first. This is the
 * single source of truth shared by backend discovery (`skill-service`) and the
 * settings UI so the two never drift.
 */
export const COMMON_SKILL_DIRS: readonly CommonSkillDir[] = [
  ...COMMON_WORKSPACE_SKILL_DIRS,
  ...COMMON_GLOBAL_SKILL_DIRS
]
