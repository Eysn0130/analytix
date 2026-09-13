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
const QUOTED_AT_MENTION_BOUNDARY = /(^|[\s([{，。；：、])@"([^"\n\r]*)$/u
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
    const query = quoted[2] ?? ''
    const start = boundedCursor - query.length - 2
    return { start, end: boundedCursor, query, quoted: true }
  }

  const plain = AT_MENTION_BOUNDARY.exec(beforeCursor)
  if (!plain) return null
  const query = plain[2] ?? ''
  const start = boundedCursor - query.length - 1
  return { start, end: boundedCursor, query, quoted: false }
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
