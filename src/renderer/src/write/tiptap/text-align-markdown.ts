import {
  Extension,
  mergeAttributes,
  type MarkdownLexerConfiguration,
  type MarkdownParseHelpers,
  type MarkdownToken
} from '@tiptap/core'
import { Heading } from '@tiptap/extension-heading'
import { Paragraph } from '@tiptap/extension-paragraph'
import type { WriteTextAlign } from '@shared/app-settings'
import {
  normalizeWriteTextAlign,
  parseWriteTextAlignDirective,
  writeTextAlignDirective,
  writeTextAlignHtmlAttributes
} from '@shared/write-text-align'

type AlignedBlockToken = MarkdownToken & {
  type: 'writeAlignedBlock'
  raw: string
  blockType: 'heading' | 'paragraph'
  text: string
  depth?: number
  align: WriteTextAlign
  tokens?: MarkdownToken[]
}

const ALIGN_ATTR = {
  textAlign: {
    default: null,
    parseHTML: (element: HTMLElement): WriteTextAlign | null => {
      const dataAlign = element.getAttribute('data-write-align')
      const styleAlign = element.style.textAlign
      return normalizeWriteTextAlign(dataAlign) ?? normalizeWriteTextAlign(styleAlign)
    },
    renderHTML: (attrs: { textAlign?: WriteTextAlign | null }) =>
      writeTextAlignHtmlAttributes(attrs.textAlign)
  }
}

function withAlignDirective(markdown: string, align: unknown): string {
  const normalized = normalizeWriteTextAlign(align)
  return normalized && normalized !== 'left'
    ? `${markdown}\n${writeTextAlignDirective(normalized)}`
    : markdown
}

export const WriteAlignedParagraph = Paragraph.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      ...ALIGN_ATTR
    }
  },
  renderHTML({ HTMLAttributes }) {
    return ['p', mergeAttributes(this.options.HTMLAttributes, HTMLAttributes), 0]
  },
  renderMarkdown: (node, h, ctx) => {
    const content = Array.isArray(node.content) ? node.content : []
    if (content.length === 0) {
      const previousContent = Array.isArray(ctx?.previousNode?.content) ? ctx.previousNode.content : []
      const previousNodeIsEmptyParagraph =
        ctx?.previousNode?.type === 'paragraph' && previousContent.length === 0
      return previousNodeIsEmptyParagraph ? '&nbsp;' : ''
    }
    return withAlignDirective(h.renderChildren(content), node.attrs?.textAlign)
  }
})

export const WriteAlignedHeading = Heading.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      ...ALIGN_ATTR
    }
  },
  renderHTML({ node, HTMLAttributes }) {
    const hasLevel = this.options.levels.includes(node.attrs.level)
    const level = hasLevel ? node.attrs.level : this.options.levels[0]
    return [`h${level}`, mergeAttributes(this.options.HTMLAttributes, HTMLAttributes), 0]
  },
  renderMarkdown: (node, h) => {
    const level = node.attrs?.level ? parseInt(node.attrs.level, 10) : 1
    const headingChars = '#'.repeat(level)
    if (!node.content) return ''
    return withAlignDirective(`${headingChars} ${h.renderChildren(node.content)}`, node.attrs?.textAlign)
  }
})

export const WriteTextAlignMarkdown = Extension.create({
  name: 'writeTextAlignMarkdown',
  priority: 1200,
  markdownTokenName: 'writeAlignedBlock',
  markdownTokenizer: {
    name: 'writeAlignedBlock',
    level: 'block',
    start(src: string) {
      const heading = src.search(/^ {0,3}#{1,6}[ \t]+.+\n\{:\s*align=/m)
      const paragraph = src.search(/^[^\n#>*+\-\d`|][^\n]*\n\{:\s*align=/m)
      if (heading < 0) return paragraph
      if (paragraph < 0) return heading
      return Math.min(heading, paragraph)
    },
    tokenize(src: string, _tokens: MarkdownToken[], helpers: MarkdownLexerConfiguration) {
      const heading = src.match(
        /^( {0,3})(#{1,6})[ \t]+([^\n]+)\n\{:\s*align=(left|center|right|justify)\s*\}(?:\n+|$)/i
      )
      if (heading) {
        const align = parseWriteTextAlignDirective(`{: align=${heading[4]}}`)
        if (!align) return undefined
        const text = heading[3].trim()
        return {
          type: 'writeAlignedBlock',
          raw: heading[0],
          blockType: 'heading',
          depth: heading[2].length,
          text,
          align,
          tokens: helpers.inlineTokens(text)
        } satisfies AlignedBlockToken
      }

      const paragraph = src.match(
        /^((?! {0,3}(?:#{1,6}\s|[-+*]\s|\d+\.\s|>\s?|```|~~~|\|))(?:(?!\n\s*\n)[^\n])+(?:\n(?!\{:\s*align=|\s*$)[^\n]+)*)\n\{:\s*align=(left|center|right|justify)\s*\}(?:\n+|$)/i
      )
      if (!paragraph) return undefined
      const align = parseWriteTextAlignDirective(`{: align=${paragraph[2]}}`)
      if (!align) return undefined
      const text = paragraph[1].trimEnd()
      return {
        type: 'writeAlignedBlock',
        raw: paragraph[0],
        blockType: 'paragraph',
        text,
        align,
        tokens: helpers.inlineTokens(text)
      } satisfies AlignedBlockToken
    }
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    const alignedToken = token as AlignedBlockToken
    if (alignedToken.blockType === 'heading') {
      return helpers.createNode(
        'heading',
        { level: alignedToken.depth || 1, textAlign: alignedToken.align },
        helpers.parseInline(alignedToken.tokens || [])
      )
    }
    return helpers.createNode(
      'paragraph',
      { textAlign: alignedToken.align },
      helpers.parseInline(alignedToken.tokens || [])
    )
  }
})
