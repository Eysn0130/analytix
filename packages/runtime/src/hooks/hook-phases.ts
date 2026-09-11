/**
 * Hook phase literals shared by config schemas and the runtime hook engine.
 * Keep this file dependency-free so package builds can include hook config
 * without pulling the TypeScript tool execution runtime into production dist.
 */
export const HOOK_PHASES = [
  'PreToolUse',
  'PostToolUse',
  'UserPromptSubmit',
  'TurnStart',
  'TurnEnd',
  'PreCompact'
] as const

export type HookPhase = (typeof HOOK_PHASES)[number]
