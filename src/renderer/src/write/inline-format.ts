export type WriteInlineFormatKind = 'bold' | 'italic' | 'strikethrough' | 'code'

type FormatMarker = {
  marker: string
  /**
   * Patterns matching a core string already wrapped by this format, tried in
   * order; the replacement removes exactly this format's markers.
   */
  unwrapPatterns: Array<{ pattern: RegExp; replacement: string }>
}

const MARKERS: Record<WriteInlineFormatKind, FormatMarker> = {
  bold: {
    marker: '**',
    unwrapPatterns: [{ pattern: /^\*\*([\s\S]+)\*\*$/, replacement: '$1' }]
  },
  italic: {
    marker: '*',
    unwrapPatterns: [
      // Bold+italic combo: peel off only the italic star pair.
      { pattern: /^\*\*\*([\s\S]+)\*\*\*$/, replacement: '**$1**' },
      // Lookahead keeps `**bold**` from being mistaken for italic.
      { pattern: /^\*(?!\*)([\s\S]*[^*])\*$/, replacement: '$1' }
    ]
  },
  strikethrough: {
    marker: '~~',
    unwrapPatterns: [{ pattern: /^~~([\s\S]+)~~$/, replacement: '$1' }]
  },
  code: {
    marker: '`',
    unwrapPatterns: [
      { pattern: /^``\s?([\s\S]+?)\s?``$/, replacement: '$1' },
      { pattern: /^`([^`]+)`$/, replacement: '$1' }
    ]
  }
}

type Segment = {
  text: string
  isGap: boolean
}

/**
 * Split a selection into formattable segments: inline code cannot span line
 * breaks, the other marks cannot span blank lines (paragraph breaks). Each
 * non-gap segment is toggled independently.
 */
function splitSegments(text: string, kind: WriteInlineFormatKind): Segment[] {
  const separator = kind === 'code' ? /(\n+)/ : /(\n[ \t]*\n+)/
  return text
    .split(separator)
    .filter((part) => part.length > 0)
    .map((part) => ({ text: part, isGap: /^\n/.test(part) }))
}

function trimCore(text: string): { lead: string; core: string; trail: string } {
  const lead = text.match(/^\s*/)?.[0] ?? ''
  const trail = text.slice(lead.length).match(/\s*$/)?.[0] ?? ''
  const core = text.slice(lead.length, text.length - trail.length)
  return { lead, core, trail }
}

function isWrapped(core: string, kind: WriteInlineFormatKind): boolean {
  return MARKERS[kind].unwrapPatterns.some(({ pattern }) => pattern.test(core))
}

function unwrap(core: string, kind: WriteInlineFormatKind): string {
  for (const { pattern, replacement } of MARKERS[kind].unwrapPatterns) {
    if (pattern.test(core)) return core.replace(pattern, replacement)
  }
  return core
}

function wrap(core: string, kind: WriteInlineFormatKind): string {
  const { marker } = MARKERS[kind]
  if (kind === 'code' && core.includes('`')) return `\`\` ${core} \`\``
  return `${marker}${core}${marker}`
}

/**
 * Toggle an inline markdown format on the selected text. Returns the
 * replacement string, or null when the selection cannot be formatted (only
 * whitespace, or nothing would change).
 *
 * When the selection mixes wrapped and unwrapped segments the unwrapped ones
 * are wrapped (matching the common "apply to everything" editor behavior);
 * only a fully wrapped selection toggles the format off.
 */
export function toggleWriteInlineFormat(
  text: string,
  kind: WriteInlineFormatKind
): string | null {
  if (!text.trim()) return null
  const segments = splitSegments(text, kind)
  const formattable = segments.filter((segment) => !segment.isGap && segment.text.trim())
  if (formattable.length === 0) return null
  const allWrapped = formattable.every((segment) => isWrapped(trimCore(segment.text).core, kind))

  const next = segments
    .map((segment) => {
      if (segment.isGap || !segment.text.trim()) return segment.text
      const { lead, core, trail } = trimCore(segment.text)
      const replacement = allWrapped
        ? unwrap(core, kind)
        : isWrapped(core, kind)
          ? core
          : wrap(core, kind)
      return `${lead}${replacement}${trail}`
    })
    .join('')

  return next === text ? null : next
}
