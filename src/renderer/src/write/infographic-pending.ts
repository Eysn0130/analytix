/**
 * Async infographic placeholders.
 *
 * Clicking 生成信息图 inserts a markdown image whose src uses the
 * `analytix-pending-infographic:` scheme, so the document itself carries the
 * placeholder position while generation runs in the background. All three
 * render surfaces (rich editor, live preview, split preview) recognize the
 * scheme and draw an animated "painting" canvas; when the IPC call resolves
 * the token is swapped for the real image path (or removed on failure).
 */

export const PENDING_INFOGRAPHIC_PROTOCOL = 'analytix-pending-infographic:'

const PENDING_SRC_PATTERN = /^analytix-pending-infographic:\/\/([A-Za-z0-9-]+)$/

export type PendingInfographicKind = 'infographic' | 'design' | 'prototype'

/** Generations still running in this renderer, keyed to their image kind.
 * Tokens left over from a crash or restored by undo after completion are not
 * in here and render as stale. */
const activeGenerations = new Map<string, PendingInfographicKind>()

function randomPendingId(): string {
  const generator = globalThis.crypto
  if (generator && typeof generator.randomUUID === 'function') return generator.randomUUID()
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

export function beginPendingInfographic(
  kind: PendingInfographicKind = 'infographic'
): { id: string; src: string } {
  const id = randomPendingId()
  activeGenerations.set(id, kind)
  return { id, src: `${PENDING_INFOGRAPHIC_PROTOCOL}//${id}` }
}

export function finishPendingInfographic(id: string): void {
  activeGenerations.delete(id)
}

export function isPendingInfographicActive(id: string): boolean {
  return activeGenerations.has(id)
}

/** Kind of an active generation; null for stale/unknown tokens. */
export function pendingInfographicKind(id: string): PendingInfographicKind | null {
  return activeGenerations.get(id) ?? null
}

/** Offset of the end of the line containing `offset` (insertion point for
 * placeholders: below the selection, never splitting its paragraph). */
export function lineEndAfter(content: string, offset: number): number {
  const nextBreak = content.indexOf('\n', offset)
  return nextBreak < 0 ? content.length : nextBreak
}

/** Id encoded in a pending src, or null when src is not a pending token. */
export function parsePendingInfographicId(src: string | undefined): string | null {
  if (!src) return null
  const match = PENDING_SRC_PATTERN.exec(src.trim())
  return match ? match[1] : null
}

export function buildPendingInfographicMarkdown(alt: string, src: string): string {
  return `![${alt.replace(/[[\]]/g, '')}](${src})`
}

/** Parse `![alt](analytix-pending-infographic://id)` image markdown. */
export function parsePendingInfographicImage(
  source: string
): { alt: string; id: string; src: string } | null {
  const match = /^!\[([^\]]*)\]\(\s*(?:<([^>]*)>|([^)\s]+))\s*\)$/.exec(source.trim())
  if (!match) return null
  const src = (match[2] ?? match[3] ?? '').trim()
  const id = parsePendingInfographicId(src)
  if (!id) return null
  return { alt: match[1] ?? '', id, src }
}

/**
 * Replace the placeholder image markdown with its final form inside plain
 * markdown text. `replacement` of null removes the placeholder, collapsing
 * the blank lines the insertion added so no empty gap is left behind.
 * Returns null when the token is not present (the user deleted it).
 */
export function replacePendingInfographicInText(
  content: string,
  pendingMarkdown: string,
  replacement: string | null
): string | null {
  const index = content.indexOf(pendingMarkdown)
  if (index < 0) return null
  const end = index + pendingMarkdown.length
  if (replacement !== null) {
    return content.slice(0, index) + replacement + content.slice(end)
  }
  let from = index
  while (from > 0 && content[from - 1] === '\n') from -= 1
  let to = end
  while (to < content.length && content[to] === '\n') to += 1
  const before = content.slice(0, from)
  const after = content.slice(to)
  const removedBreaks = (index - from) + (to - end)
  const keptBreaks = before.length === 0 || after.length === 0
    ? Math.min(1, removedBreaks)
    : Math.min(2, removedBreaks)
  return before + '\n'.repeat(keptBreaks) + after
}
