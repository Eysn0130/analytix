export type WriteBlockType =
  | 'paragraph'
  | 'heading1'
  | 'heading2'
  | 'heading3'
  | 'quote'
  | 'bullet'
  | 'ordered'
  | 'code'

/** Display order for the block-type menu. */
export const WRITE_BLOCK_TYPES: WriteBlockType[] = [
  'paragraph',
  'heading1',
  'heading2',
  'heading3',
  'quote',
  'bullet',
  'ordered',
  'code'
]

function splitIndent(line: string): { indent: string; rest: string } {
  const match = /^([ \t]*)([\s\S]*)$/.exec(line)
  return { indent: match?.[1] ?? '', rest: match?.[2] ?? line }
}

function stripBlockMarker(rest: string): string {
  return rest
    .replace(/^#{1,6}[ \t]+/, '')
    .replace(/^>[ \t]?/, '')
    .replace(/^[-*+][ \t]+/, '')
    .replace(/^\d+[.)][ \t]+/, '')
}

/** Detect the markdown block type of a single source line. */
export function detectWriteBlockTypeFromLine(line: string): WriteBlockType {
  const { rest } = splitIndent(line)
  const heading = /^(#{1,6})[ \t]+/.exec(rest)
  if (heading) {
    const level = heading[1].length
    return level === 1 ? 'heading1' : level === 2 ? 'heading2' : 'heading3'
  }
  if (/^(```|~~~)/.test(rest)) return 'code'
  if (/^>[ \t]?/.test(rest)) return 'quote'
  if (/^[-*+][ \t]+/.test(rest)) return 'bullet'
  if (/^\d+[.)][ \t]+/.test(rest)) return 'ordered'
  return 'paragraph'
}

function markerFor(type: WriteBlockType, ordinal: number): string {
  switch (type) {
    case 'heading1':
      return '# '
    case 'heading2':
      return '## '
    case 'heading3':
      return '### '
    case 'quote':
      return '> '
    case 'bullet':
      return '- '
    case 'ordered':
      return `${ordinal}. `
    default:
      return ''
  }
}

/**
 * Rewrite the block markers of the given source lines to `type`. Existing
 * markers are stripped first so block types swap cleanly. Code blocks wrap the
 * stripped content in a fenced block; blank lines keep their emptiness so we
 * never emit a bare "# " marker.
 */
export function applyWriteBlockTypeToLines(lines: string[], type: WriteBlockType): string[] {
  const safeLines = lines.length > 0 ? lines : ['']
  if (type === 'code') {
    const body = safeLines.map((line) => {
      const { indent, rest } = splitIndent(line)
      return `${indent}${stripBlockMarker(rest)}`
    })
    return ['```', ...body, '```']
  }
  let ordinal = 0
  return safeLines.map((line) => {
    const { indent, rest } = splitIndent(line)
    const content = stripBlockMarker(rest)
    if (type === 'paragraph') return `${indent}${content}`
    if (!content.trim()) return line
    ordinal += 1
    return `${indent}${markerFor(type, ordinal)}${content}`
  })
}
