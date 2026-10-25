export type ComposerAtMention = {
  start: number
  end: number
  query: string
  quoted: boolean
}

export type ComposerPluginMentionToken = {
  token: string
  start: number
  end: number
  label: string
  pluginId: string
}

export type ComposerSkillMentionToken = {
  token: string
  start: number
  end: number
  label: string
  skillId: string
}

export type ComposerMentionDisplayPart =
  | {
      kind: 'text'
      key: string
      start: number
      end: number
      text: string
    }
  | {
      kind: 'plugin'
      key: string
      start: number
      end: number
      mention: ComposerPluginMentionToken
    }
  | {
      kind: 'skill'
      key: string
      start: number
      end: number
      mention: ComposerSkillMentionToken
    }

export type ComposerMentionDisplayPlan = {
  hasMentions: boolean
  editableStart: number
  editableValue: string
  parts: ComposerMentionDisplayPart[]
}

export const COMPOSER_PLUGIN_MENTION_PREFIX = 'plugin://'
export const COMPOSER_SKILL_MENTION_PREFIX = 'skill://'

const AT_MENTION_BOUNDARY = /(^|[\s([{，。；：、])@([^\s@"'\])]*)$/u
const QUOTED_AT_MENTION_BOUNDARY = /(^|[\s([{，。；：、])@"((?:\\[^\n\r]|[^"\\\n\r])*\\?)$/u
// Keep escape pairs disjoint from ordinary URI characters, including for
// incomplete user input that has no closing parenthesis.
const COMPOSER_LINK_MENTION = /([@$])\[((?:\\.|[^\]\\])*)\]\(((?:\\.|[^)\s\\])*)\)/gu

function escapeMentionLabel(value: string): string {
  return value.replaceAll('\\', '\\\\').replaceAll('[', '\\[').replaceAll(']', '\\]')
}

function unescapeMentionPart(value: string): string {
  return value.replace(/\\(.)/gu, '$1')
}

function encodeMentionId(value: string): string {
  return encodeURIComponent(value.trim())
}

function decodeMentionId(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

export function getComposerAtMentionAtCursor(input: string, cursor: number): ComposerAtMention | null {
  const boundedCursor = Math.max(0, Math.min(cursor, input.length))
  const beforeCursor = input.slice(0, boundedCursor)
  const quoted = QUOTED_AT_MENTION_BOUNDARY.exec(beforeCursor)
  if (quoted) {
    const rawQuery = quoted[2] ?? ''
    const start = boundedCursor - rawQuery.length - 2
    const query = rawQuery.replace(/\\(["\\])/gu, '$1')
    return { start, end: boundedCursor, query, quoted: true }
  }

  const plain = AT_MENTION_BOUNDARY.exec(beforeCursor)
  if (!plain) return null
  const query = plain[2] ?? ''
  const start = boundedCursor - query.length - 1
  return { start, end: boundedCursor, query, quoted: false }
}

/** An unfinished @ path for directory navigation; selecting a reference still closes the token. */
export function formatComposerDirectoryQuery(directory: string, quoted = false): string {
  const query = directory ? `${directory.replace(/\/+$/u, '')}/` : ''
  return quoted || /[\s@"')\]]/u.test(query)
    ? `@"${query.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}`
    : `@${query}`
}

export function formatComposerPluginMentionToken(displayName: string, pluginId: string): string {
  const trimmedId = pluginId.trim()
  const label = displayName.trim() || trimmedId
  return `@[${escapeMentionLabel(label)}](${COMPOSER_PLUGIN_MENTION_PREFIX}${encodeMentionId(trimmedId)})`
}

export function formatComposerSkillMentionToken(displayName: string, skillId: string): string {
  const trimmedId = skillId.trim()
  const label = displayName.trim() || trimmedId
  return `$[${escapeMentionLabel(label)}](${COMPOSER_SKILL_MENTION_PREFIX}${encodeMentionId(trimmedId)})`
}

export function replaceComposerAtMentionInInput(
  input: string,
  mention: ComposerAtMention,
  token: string
): { input: string; cursor: number } {
  const replacement = `${token}${input[mention.end] && /\s/u.test(input[mention.end] ?? '') ? '' : ' '}`
  const nextInput = `${input.slice(0, mention.start)}${replacement}${input.slice(mention.end)}`
  return {
    input: nextInput,
    cursor: mention.start + replacement.length
  }
}

function removeExactToken(input: string, token: string): string {
  const index = input.indexOf(token)
  if (index < 0) return input
  const before = input.slice(0, index).replace(/[ \t]+$/u, '')
  const after = input.slice(index + token.length).replace(/^[ \t]+/u, '')
  return `${before}${before && after ? ' ' : ''}${after}`
}

export function removeComposerMentionToken(input: string, token: string): string {
  return token ? removeExactToken(input, token) : input
}

export function removeComposerMentionTokens(input: string, tokens: string[]): string {
  return tokens.reduce((current, token) => removeComposerMentionToken(current, token), input)
}

export function extractComposerPluginMentions(input: string): ComposerPluginMentionToken[] {
  const mentions: ComposerPluginMentionToken[] = []
  for (const match of input.matchAll(COMPOSER_LINK_MENTION)) {
    const marker = match[1]
    const rawLabel = match[2] ?? ''
    const rawHref = unescapeMentionPart(match[3] ?? '')
    if (marker !== '@' || !rawHref.startsWith(COMPOSER_PLUGIN_MENTION_PREFIX)) continue
    const token = match[0]
    const start = match.index ?? 0
    const pluginId = decodeMentionId(rawHref.slice(COMPOSER_PLUGIN_MENTION_PREFIX.length))
    if (!pluginId.trim()) continue
    mentions.push({
      token,
      start,
      end: start + token.length,
      label: unescapeMentionPart(rawLabel),
      pluginId
    })
  }
  return mentions
}

export function extractComposerSkillMentions(input: string): ComposerSkillMentionToken[] {
  const mentions: ComposerSkillMentionToken[] = []
  for (const match of input.matchAll(COMPOSER_LINK_MENTION)) {
    const marker = match[1]
    const rawLabel = match[2] ?? ''
    const rawHref = unescapeMentionPart(match[3] ?? '')
    if (marker !== '$' || !rawHref.startsWith(COMPOSER_SKILL_MENTION_PREFIX)) continue
    const token = match[0]
    const start = match.index ?? 0
    const skillId = decodeMentionId(rawHref.slice(COMPOSER_SKILL_MENTION_PREFIX.length))
    if (!skillId.trim()) continue
    mentions.push({
      token,
      start,
      end: start + token.length,
      label: unescapeMentionPart(rawLabel),
      skillId
    })
  }
  return mentions
}

export function getComposerMentionDisplayPlan(input: string): ComposerMentionDisplayPlan {
  const atoms = [
    ...extractComposerPluginMentions(input).map((mention) => ({
      kind: 'plugin' as const,
      start: mention.start,
      end: mention.end,
      mention
    })),
    ...extractComposerSkillMentions(input).map((mention) => ({
      kind: 'skill' as const,
      start: mention.start,
      end: mention.end,
      mention
    }))
  ].sort((left, right) => left.start - right.start || left.end - right.end)

  if (atoms.length === 0) {
    return {
      hasMentions: false,
      editableStart: 0,
      editableValue: input,
      parts: []
    }
  }

  const parts: ComposerMentionDisplayPart[] = []
  let cursor = 0
  for (const atom of atoms) {
    if (atom.start < cursor) continue
    if (atom.start > cursor) {
      parts.push({
        kind: 'text',
        key: `text:${cursor}:${atom.start}`,
        start: cursor,
        end: atom.start,
        text: input.slice(cursor, atom.start)
      })
    }
    if (atom.kind === 'plugin') {
      parts.push({
        kind: 'plugin',
        key: `plugin:${atom.start}:${atom.end}`,
        start: atom.start,
        end: atom.end,
        mention: atom.mention
      })
    } else {
      parts.push({
        kind: 'skill',
        key: `skill:${atom.start}:${atom.end}`,
        start: atom.start,
        end: atom.end,
        mention: atom.mention
      })
    }
    cursor = atom.end
  }

  return {
    hasMentions: parts.some((part) => part.kind !== 'text'),
    editableStart: cursor,
    editableValue: input.slice(cursor),
    parts
  }
}

export type UserMessageDisplayPart = ComposerMentionDisplayPart
  | { kind: 'file'; key: string; start: number; end: number; label: string; path: string; display: string; directory: boolean }
  | { kind: 'loaded-skill'; key: string; start: number; end: number; skillId: string }

/** Sent-text projection only; composer editing keeps its existing wire-token plan.
 * Adapted from DeepSeek Harness user-text.tsx at 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger.
 * Decoration never grants file/skill navigation authority. */
export function getUserMessageDisplayParts(input: string, loadedSkillIds: readonly string[] = []): UserMessageDisplayPart[] {
  const wire = getComposerMentionDisplayPlan(input).parts.filter(part => part.kind !== 'text')
  const atoms: UserMessageDisplayPart[] = [...wire]
  const loaded = new Set(loadedSkillIds)
  let wireCursor = 0
  // Keep escaped quote/backslash pairs disjoint from ordinary quoted path text.
  const tokenPattern = /(^|\s)(\/(?:skill:)?[\w.-]+(?=\s|$)|@"(?:\\[^\r\n]|[^"\\\r\n])+"|@[^\s]+)/gu
  for (const match of input.matchAll(tokenPattern)) {
    const raw = match[2]!
    const start = match.index! + match[1]!.length
    const label = raw.startsWith('@"') ? raw : raw.replace(/[.,;:!?，。；：！？]+$/u, '')
    const end = start + label.length
    while (wireCursor < wire.length && wire[wireCursor]!.end <= start) wireCursor += 1
    const coveringWire = wire[wireCursor]
    if (label.length <= 1 || coveringWire && start < coveringWire.end && end > coveringWire.start) continue
    if (label.startsWith('/')) {
      const skillId = label.startsWith('/skill:') ? label.slice(7) : label.slice(1)
      if (loaded.has(skillId)) atoms.push({ kind: 'loaded-skill', key: `loaded-skill:${start}:${end}`, start, end, skillId })
      continue
    }
    const quoted = label.startsWith('@"')
    if (quoted && !label.endsWith('"')) continue
    const path = quoted ? label.slice(2, -1).replace(/\\(["\\])/gu, '$1') : label.slice(1)
    if (!path) continue
    atoms.push({ kind: 'file', key: `file:${start}:${end}`, start, end, label, path,
      display: path.split(/[\\/]/u).filter(Boolean).at(-1) ?? path, directory: path.endsWith('/') })
  }
  atoms.sort((left, right) => left.start - right.start || left.end - right.end)
  const parts: UserMessageDisplayPart[] = []
  let cursor = 0
  for (const atom of atoms) {
    if (atom.start < cursor) continue
    if (atom.start > cursor) parts.push({ kind: 'text', key: `text:${cursor}:${atom.start}`, start: cursor, end: atom.start, text: input.slice(cursor, atom.start) })
    parts.push(atom)
    cursor = atom.end
  }
  if (cursor < input.length) parts.push({ kind: 'text', key: `text:${cursor}:${input.length}`, start: cursor, end: input.length, text: input.slice(cursor) })
  return parts
}

/** Extract only from direct user input before document/file/native context is assembled.
 * Requests are hints; Core resolves and loads the current admitted snapshot. */
export function extractRequestedSkillIds(input: string): string[] {
  const wire = extractComposerSkillMentions(input)
  const occupied = [...wire, ...extractComposerPluginMentions(input)]
  const candidates = wire.map(mention => ({ start: mention.start, id: mention.skillId }))
  for (const match of input.matchAll(/(^|\s)\/(?:skill:([a-z0-9][a-z0-9._-]{0,127})|([a-z0-9]+(?:-[a-z0-9]+)*))(?=\s|$)/gu)) {
    const start = match.index! + match[1]!.length
    if (!occupied.some(mention => start >= mention.start && start < mention.end)) {
      candidates.push({ start, id: match[2] ?? match[3]! })
    }
  }
  return [...new Set(candidates.sort((a, b) => a.start - b.start).map(candidate => candidate.id)
    .filter(id => /^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/u.test(id)))].slice(0, 16)
}
