import type { ReactElement } from 'react'
import type { ReviewTarget } from '../../agent/types'

export type BuiltinSlashCommandId = 'new' | 'plan' | 'goal' | 'research' | 'review' | 'compact' | 'fork' | 'archive' | 'restore' | 'btw'
  | 'file' | 'workspace-files' | 'attachments' | 'permission' | 'model' | 'feedback' | 'export' | 'worktree'
export type SkillSlashCommandId = `skill:${string}`
export type PluginSlashCommandId = `plugin:${string}`
export type SlashCommandId = BuiltinSlashCommandId | SkillSlashCommandId | PluginSlashCommandId
export type ComposerCommandGroup = 'add' | 'commands' | 'extensions' | 'skills' | 'plugins' | 'worktree'
export type ComposerCommandPosition = 'leading' | 'inline'

export type SlashCommand = {
  id: SlashCommandId
  kind?: 'builtin' | 'skill' | 'plugin'
  title: string
  description: string
  keywords: string[]
  icon: ReactElement
  canonicalName?: string
  group?: ComposerCommandGroup
  /** Input actions stay at the leading trigger unless a contribution explicitly permits inline use. */
  inputOnly?: boolean
  badge?: string
  scopeLabel?: string
  skillPrompt?: string
  disabled?: boolean
  disabledReason?: string
  checked?: boolean
}

export type ComposerCommandRow = SlashCommand & { section?: ComposerCommandGroup }

/* Empty-query grouping and ranked discovery adapted from DeepSeek Harness
 * 687f282e8625c6b8d11ff58211a90524be2e4317, Copyright (c) 2026 DeepSeek, MIT.
 * See THIRD_PARTY_NOTICES.md and docs/analytix/upstreams/code-reuse-provenance.md.
 * Analytix supplies its existing command catalog and owns every action. */
const ADD_COMMANDS: readonly SlashCommandId[] = ['file', 'goal', 'plan', 'feedback']
const MAIN_COMMANDS: readonly SlashCommandId[] = ['compact', 'permission', 'model', 'export']
const COMMAND_GROUPS: readonly ComposerCommandGroup[] = ['add', 'commands', 'extensions', 'skills', 'plugins', 'worktree']

function commandGroup(command: SlashCommand): ComposerCommandGroup {
  if (ADD_COMMANDS.includes(command.id)) return 'add'
  if (MAIN_COMMANDS.includes(command.id)) return 'commands'
  if (command.kind === 'skill' || command.id.startsWith('skill:')) return 'skills'
  if (command.kind === 'plugin' || command.id.startsWith('plugin:')) return 'plugins'
  if (command.id === 'worktree') return 'worktree'
  return command.group ?? 'extensions'
}

function commandOrder(command: SlashCommand, group: ComposerCommandGroup): number {
  const order = group === 'add' ? ADD_COMMANDS : group === 'commands' ? MAIN_COMMANDS : []
  const index = order.indexOf(command.id)
  return index < 0 ? order.length : index
}

function boundaryBonus(name: string, index: number): number {
  return index === 0 || name.charAt(index - 1) === '-' || name.charAt(index - 1) === '_' ? 8 : 0
}

/** Strongest ordered-subsequence alignment, with adjacent and word-boundary bonuses. */
function alignmentScore(name: string, query: string): number | undefined {
  if (query.length > name.length) return undefined
  const noMatch = Number.NEGATIVE_INFINITY
  let previous = Array<number>(name.length).fill(noMatch)
  for (let index = 0; index < name.length; index++) {
    if (name.charAt(index) === query.charAt(0)) previous[index] = 1 + boundaryBonus(name, index) - index
  }
  for (let queryIndex = 1; queryIndex < query.length; queryIndex++) {
    const current = Array<number>(name.length).fill(noMatch)
    let left = noMatch
    let leftLeft = noMatch
    let bestGapped = noMatch
    for (const [index, prior] of previous.entries()) {
      if (leftLeft !== noMatch) bestGapped = Math.max(bestGapped, leftLeft + index - 2)
      if (name.charAt(index) === query.charAt(queryIndex)) {
        const bonus = 1 + boundaryBonus(name, index)
        let score = noMatch
        if (left !== noMatch) score = left + bonus + 4
        if (bestGapped !== noMatch) score = Math.max(score, bestGapped + bonus + 1 - index)
        current[index] = score
      }
      leftLeft = left
      left = prior
    }
    previous = current
  }
  let best = noMatch
  for (const score of previous) best = Math.max(best, score)
  return best === noMatch ? undefined : best
}

/** Present the real action catalog without adding executable or synthetic commands. */
export function getComposerCommandRows(
  commands: readonly SlashCommand[],
  rawQuery: string,
  position: ComposerCommandPosition = 'leading'
): ComposerCommandRow[] {
  const visible = commands.filter(command => position === 'leading' || command.inputOnly === false)
  const query = rawQuery.toLowerCase()
  if (query === '') {
    return COMMAND_GROUPS.flatMap(section => visible
      .filter(command => commandGroup(command) === section)
      .map((command, index) => ({ command, index }))
      .sort((left, right) => commandOrder(left.command, section) - commandOrder(right.command, section) || left.index - right.index)
      .map(({ command }) => ({ ...command, section })))
  }
  return visible.flatMap((command, index) => {
    const keys = [command.canonicalName ?? command.id, command.title, ...command.keywords]
    let prefix = false
    let score: number | undefined
    for (const key of keys) {
      const lower = key.toLowerCase()
      const keyScore = alignmentScore(lower, query)
      if (keyScore === undefined) continue
      prefix ||= lower.startsWith(query)
      score = score === undefined ? keyScore : Math.max(score, keyScore)
    }
    return score === undefined ? [] : [{ command, index, prefix, score }]
  })
    .sort((left, right) => Number(right.prefix) - Number(left.prefix) || right.score - left.score || left.index - right.index)
    .map(({ command }) => ({ ...command, section: undefined }))
}

export type CompactCommand = {
  reason?: string
}

export type GoalCommand =
  | { action: 'menu' }
  | { action: 'set'; objective: string }
  | { action: 'research'; objective: string }
  | { action: 'pause' | 'resume' | 'clear' }

export const COMPACT_COMMAND_ALIASES = [
  'compact',
  'compress',
  'summarize',
  'summary',
  '压缩会话',
  '总结会话',
  '压缩',
  '总结'
]

export const NEW_COMMAND_ALIASES = [
  'new',
  'new-chat',
  'new-thread',
  '新会话',
  '新建会话'
]

export const REVIEW_COMMAND_ALIASES = [
  'review',
  'code-review',
  'codereview',
  '审查',
  '代码审查'
]

export const RESEARCH_COMMAND_ALIASES = [
  'research',
  'deepresearch',
  'deep-research',
  'deep_research'
]

export function getSlashQuery(input: string): string | null {
  const trimmed = input.trimStart()
  if (!trimmed.startsWith('/')) return null
  if (/\s/.test(trimmed)) return null
  return trimmed.slice(1).toLowerCase()
}

export function getGoalPanelDraftObjective(input: string, goalPanelOpen: boolean): string {
  const objective = input.trim()
  if (!goalPanelOpen || objective.length === 0 || objective.startsWith('/')) return ''
  return objective
}

export function parseCompactCommand(input: string): CompactCommand | null {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return null
  const body = trimmed.slice(1).trimStart()
  const lowerBody = body.toLowerCase()
  for (const alias of COMPACT_COMMAND_ALIASES) {
    const lowerAlias = alias.toLowerCase()
    if (lowerBody !== lowerAlias && !isSlashAliasMatch(lowerBody, lowerAlias)) continue
    const reason = body.slice(alias.length).trim()
    return reason ? { reason } : {}
  }
  return null
}

export function parseNewCommand(input: string): boolean {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return false
  const body = trimmed.slice(1).trimStart()
  const lowerBody = body.toLowerCase()
  return NEW_COMMAND_ALIASES.some((alias) => lowerBody === alias.toLowerCase())
}

export function parseGoalCommand(input: string): GoalCommand | false {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/goal')) return false
  const rest = trimmed.slice(5)
  if (rest.length > 0 && !/^\s/.test(rest)) return false
  const body = rest.trim()
  if (!body) return { action: 'menu' }
  const lowered = body.toLowerCase()
  if (lowered === 'pause') return { action: 'pause' }
  if (lowered === 'resume') return { action: 'resume' }
  if (lowered === 'clear') return { action: 'clear' }
  if (lowered === '--research' || lowered === 'research') return { action: 'menu' }
  for (const prefix of ['--research ', 'research ']) {
    if (lowered.startsWith(prefix)) {
      const objective = body.slice(prefix.length).trim()
      return objective ? { action: 'research', objective } : { action: 'menu' }
    }
  }
  return { action: 'set', objective: body }
}

export function parseReviewCommand(input: string): ReviewTarget | false {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return false
  const body = trimmed.slice(1).trimStart()
  const lowerBody = body.toLowerCase()
  let rest: string | null = null
  for (const alias of REVIEW_COMMAND_ALIASES) {
    const lowerAlias = alias.toLowerCase()
    if (lowerBody === lowerAlias) {
      rest = ''
      break
    }
    if (isSlashAliasMatch(lowerBody, lowerAlias)) {
      rest = body.slice(alias.length).trim()
      break
    }
  }
  if (rest == null) return false
  if (!rest) return { kind: 'uncommittedChanges' }
  const [verb = '', ...parts] = rest.split(/\s+/)
  const value = parts.join(' ').trim()
  switch (verb.toLowerCase()) {
    case 'base':
    case 'branch':
    case 'against':
      return value ? { kind: 'baseBranch', branch: value } : { kind: 'custom', instructions: rest }
    case 'commit':
      return value ? { kind: 'commit', sha: value } : { kind: 'custom', instructions: rest }
    default:
      return { kind: 'custom', instructions: rest }
  }
}

export function parseResearchCommand(input: string): string | null | false {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return false
  const body = trimmed.slice(1).trimStart()
  const lowerBody = body.toLowerCase()
  for (const alias of RESEARCH_COMMAND_ALIASES) {
    const lowerAlias = alias.toLowerCase()
    if (lowerBody === lowerAlias) return null
    if (isSlashAliasMatch(lowerBody, lowerAlias)) {
      const topic = body.slice(alias.length).trim()
      return topic || null
    }
  }
  return false
}

export function buildResearchPrompt(template: string, topic: string | null): string {
  const normalizedTopic = topic?.trim()
  return normalizedTopic ? template.replace('{{topic}}', normalizedTopic) : template
}

function isSlashAliasMatch(body: string, alias: string): boolean {
  if (!body.startsWith(alias)) return false
  const next = body.at(alias.length)
  return next != null && /\s/.test(next)
}

/**
 * Returns the seed text for a `/btw <question>` command, or `null`
 * if the input is not a `/btw` command. The renderer's slash menu
 * only matches single-word commands; this mirrors the existing
 * `parseGuiPlanCommand` interception that runs at send time so the
 * user can pass a question inline.
 */
export function parseBtwCommand(input: string): string | null | false {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/btw')) return false
  // The `/btw` token must be standalone (followed by whitespace or end).
  const rest = trimmed.slice(4)
  if (rest.length > 0 && !/^\s/.test(rest)) return false
  const question = rest.trim()
  return question.length > 0 ? question : null
}
