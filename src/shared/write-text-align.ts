import type { WriteTextAlign } from './app-settings-types'

export const WRITE_TEXT_ALIGN_DIRECTIVE_REGEX =
  /^\{:\s*align=(left|center|right|justify)\s*\}$/i

export function normalizeWriteTextAlign(value: unknown): WriteTextAlign | null {
  const normalized = typeof value === 'string' ? value.toLowerCase() : value
  return normalized === 'left' || normalized === 'center' || normalized === 'right' || normalized === 'justify'
    ? normalized
    : null
}

export function parseWriteTextAlignDirective(value: string): WriteTextAlign | null {
  const match = value.trim().match(WRITE_TEXT_ALIGN_DIRECTIVE_REGEX)
  return normalizeWriteTextAlign(match?.[1]?.toLowerCase())
}

export function writeTextAlignDirective(align: WriteTextAlign): string {
  return `{: align=${align}}`
}

export function writeTextAlignHtmlAttributes(align: WriteTextAlign | null | undefined): Record<string, string> {
  const normalized = normalizeWriteTextAlign(align)
  return normalized && normalized !== 'left'
    ? {
        className: `write-align-${normalized}`,
        'data-write-align': normalized,
        style: `text-align: ${normalized};`
      }
    : {}
}

export function stripWriteTextAlignDirectiveLines(lines: string[]): string[] {
  return lines.filter((line) => !parseWriteTextAlignDirective(line))
}

export function stripWriteTextAlignDirectivesFromMarkdown(content: string): string {
  return stripWriteTextAlignDirectiveLines(content.replace(/\r\n/g, '\n').split('\n')).join('\n')
}

export function markdownLineCanCarryAlignment(line: string): boolean {
  const trimmed = line.trim()
  if (!trimmed) return false
  if (/^(```|~~~)/.test(trimmed)) return false
  if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) return false
  if (/^([-+*]|\d+\.)\s+/.test(trimmed)) return false
  if (/^>\s?/.test(trimmed)) return false
  if (/^\|.*\|$/.test(trimmed)) return false
  return true
}

function isMarkdownFenceLine(line: string): boolean {
  return /^(```|~~~)/.test(line.trim())
}

export function applyWriteTextAlignToMarkdownLines(lines: string[], align: WriteTextAlign): string[] {
  const cleanLines = stripWriteTextAlignDirectiveLines(lines)
  if (align === 'left') return cleanLines

  const next: string[] = []
  let inFence = false
  for (const line of cleanLines) {
    next.push(line)
    if (isMarkdownFenceLine(line)) {
      inFence = !inFence
      continue
    }
    if (!inFence && markdownLineCanCarryAlignment(line)) {
      next.push(writeTextAlignDirective(align))
    }
  }
  return next
}

type MarkdownTreeNode = {
  type?: string
  value?: unknown
  children?: MarkdownTreeNode[]
  data?: {
    hProperties?: Record<string, unknown>
    [key: string]: unknown
  }
}

function directiveFromNode(node: MarkdownTreeNode | undefined): WriteTextAlign | null {
  if (!node || node.type !== 'paragraph' || !Array.isArray(node.children) || node.children.length !== 1) {
    return null
  }
  const child = node.children[0]
  return child?.type === 'text' && typeof child.value === 'string'
    ? parseWriteTextAlignDirective(child.value)
    : null
}

function applyAlignToNode(node: MarkdownTreeNode, align: WriteTextAlign): void {
  if (align === 'left') return
  node.data = {
    ...node.data,
    hProperties: {
      ...node.data?.hProperties,
      ...writeTextAlignHtmlAttributes(align)
    }
  }
}

export function applyWriteMarkdownAlignmentDirectivesToTree(tree: MarkdownTreeNode): void {
  const visit = (node: MarkdownTreeNode): void => {
    const children = node.children
    if (!Array.isArray(children)) return

    for (let index = 0; index < children.length; index += 1) {
      const align = directiveFromNode(children[index + 1])
      if (align) {
        applyAlignToNode(children[index], align)
        children.splice(index + 1, 1)
      }
      visit(children[index])
    }
  }

  visit(tree)
}
