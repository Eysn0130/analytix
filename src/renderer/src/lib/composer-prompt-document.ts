import { Schema, type DOMOutputSpec, type Node as ProseMirrorNode } from '@tiptap/pm/model'
import {
  extractComposerPluginMentions,
  extractComposerSkillMentions,
  formatComposerPluginMentionToken,
  formatComposerSkillMentionToken
} from './composer-mentions'

export type ComposerPromptPluginMentionMeta = {
  pluginId: string
  title: string
  description?: string
  iconUrl?: string
  brandColor?: string
}

export type ComposerPromptSkillMentionMeta = {
  skillId: string
  title: string
  description?: string
}

export type ComposerPromptMentionMetadata = {
  plugins?: ComposerPromptPluginMentionMeta[]
  skills?: ComposerPromptSkillMentionMeta[]
}

type ComposerPromptMentionAtom =
  | {
      kind: 'plugin'
      start: number
      end: number
      token: string
      label: string
      pluginId: string
    }
  | {
      kind: 'skill'
      start: number
      end: number
      token: string
      label: string
      skillId: string
    }

export type ComposerPromptProjectionSegment = {
  kind: 'text' | 'mention' | 'newline'
  rawStart: number
  rawEnd: number
  pmStart: number
  pmEnd: number
}

export type ComposerPromptProjection = {
  text: string
  segments: ComposerPromptProjectionSegment[]
}

function safeCssColor(value: string | undefined): string | undefined {
  const color = value?.trim()
  if (!color) return undefined
  if (/^#[0-9a-f]{3,8}$/iu.test(color)) return color
  if (/^rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}(?:\s*,\s*(?:0|1|0?\.\d+))?\s*\)$/iu.test(color)) {
    return color
  }
  return undefined
}

function mentionDomSpec(
  kind: 'plugin' | 'skill',
  attrs: {
    token?: string
    label?: string
    title?: string
    description?: string
    iconUrl?: string
    brandColor?: string
  }
): DOMOutputSpec {
  const title = attrs.title || attrs.label || attrs.token || ''
  const domAttrs: Record<string, string> = {
    class: `composer-prompt-mention composer-prompt-mention-${kind}`,
    'data-composer-mention-kind': kind,
    'data-composer-mention-token': attrs.token || '',
    contenteditable: 'false',
    title: attrs.description || title
  }
  const color = safeCssColor(attrs.brandColor)
  if (color) domAttrs.style = `color: ${color};`

  const icon: DOMOutputSpec = attrs.iconUrl
    ? [
        'span',
        { class: 'composer-prompt-mention-icon' },
        ['img', { src: attrs.iconUrl, alt: '', class: 'composer-prompt-mention-icon-image' }]
      ]
    : ['span', { class: 'composer-prompt-mention-icon composer-prompt-mention-icon-fallback', 'aria-hidden': 'true' }]

  return [
    'span',
    domAttrs,
    icon,
    ['span', { class: 'composer-prompt-mention-label' }, title]
  ]
}

export const composerPromptSchema = new Schema({
  nodes: {
    doc: {
      content: 'paragraph+'
    },
    paragraph: {
      content: 'inline*',
      group: 'block',
      parseDOM: [{ tag: 'p' }],
      toDOM: (): DOMOutputSpec => ['p', 0]
    },
    text: {
      group: 'inline'
    },
    pluginMention: {
      inline: true,
      group: 'inline',
      atom: true,
      selectable: false,
      draggable: false,
      attrs: {
        token: { default: '' },
        label: { default: '' },
        pluginId: { default: '' },
        title: { default: '' },
        description: { default: '' },
        iconUrl: { default: '' },
        brandColor: { default: '' }
      },
      parseDOM: [{
        tag: 'span[data-composer-mention-kind="plugin"]',
        getAttrs: (node) => {
          if (!(node instanceof HTMLElement)) return false
          return {
            token: node.dataset.composerMentionToken || '',
            label: node.textContent || '',
            pluginId: '',
            title: node.textContent || '',
            description: node.getAttribute('title') || ''
          }
        }
      }],
      toDOM: (node): DOMOutputSpec => mentionDomSpec('plugin', node.attrs)
    },
    skillMention: {
      inline: true,
      group: 'inline',
      atom: true,
      selectable: false,
      draggable: false,
      attrs: {
        token: { default: '' },
        label: { default: '' },
        skillId: { default: '' },
        title: { default: '' },
        description: { default: '' }
      },
      parseDOM: [{
        tag: 'span[data-composer-mention-kind="skill"]',
        getAttrs: (node) => {
          if (!(node instanceof HTMLElement)) return false
          return {
            token: node.dataset.composerMentionToken || '',
            label: node.textContent || '',
            skillId: '',
            title: node.textContent || '',
            description: node.getAttribute('title') || ''
          }
        }
      }],
      toDOM: (node): DOMOutputSpec => mentionDomSpec('skill', node.attrs)
    }
  },
  marks: {}
})

function metadataMaps(metadata: ComposerPromptMentionMetadata | undefined): {
  plugins: Map<string, ComposerPromptPluginMentionMeta>
  skills: Map<string, ComposerPromptSkillMentionMeta>
} {
  return {
    plugins: new Map((metadata?.plugins ?? []).map((item) => [item.pluginId.trim().toLowerCase(), item])),
    skills: new Map((metadata?.skills ?? []).map((item) => [item.skillId.trim().toLowerCase(), item]))
  }
}

function collectMentionAtoms(input: string): ComposerPromptMentionAtom[] {
  const atoms = [
    ...extractComposerPluginMentions(input).map((mention): ComposerPromptMentionAtom => ({
      kind: 'plugin',
      start: mention.start,
      end: mention.end,
      token: mention.token,
      label: mention.label,
      pluginId: mention.pluginId
    })),
    ...extractComposerSkillMentions(input).map((mention): ComposerPromptMentionAtom => ({
      kind: 'skill',
      start: mention.start,
      end: mention.end,
      token: mention.token,
      label: mention.label,
      skillId: mention.skillId
    }))
  ].sort((left, right) => left.start - right.start || left.end - right.end)

  const nonOverlapping: ComposerPromptMentionAtom[] = []
  let cursor = 0
  for (const atom of atoms) {
    if (atom.start < cursor) continue
    nonOverlapping.push(atom)
    cursor = atom.end
  }
  return nonOverlapping
}

function textNode(value: string): ProseMirrorNode | null {
  return value ? composerPromptSchema.text(value) : null
}

export function parseComposerPromptDoc(
  input: string,
  metadata?: ComposerPromptMentionMetadata
): ProseMirrorNode {
  const atoms = collectMentionAtoms(input)
  const meta = metadataMaps(metadata)
  const lines = input.split('\n')
  const paragraphs: ProseMirrorNode[] = []
  let lineStart = 0

  for (const line of lines) {
    const lineEnd = lineStart + line.length
    const children: ProseMirrorNode[] = []
    let cursor = lineStart
    for (const atom of atoms) {
      if (atom.start < lineStart || atom.end > lineEnd) continue
      const before = textNode(input.slice(cursor, atom.start))
      if (before) children.push(before)
      if (atom.kind === 'plugin') {
        const plugin = meta.plugins.get(atom.pluginId.trim().toLowerCase())
        children.push(composerPromptSchema.nodes.pluginMention.create({
          token: atom.token,
          label: atom.label,
          pluginId: atom.pluginId,
          title: plugin?.title || atom.label || atom.pluginId,
          description: plugin?.description || atom.pluginId,
          iconUrl: plugin?.iconUrl || '',
          brandColor: plugin?.brandColor || ''
        }))
      } else {
        const skill = meta.skills.get(atom.skillId.trim().toLowerCase())
        children.push(composerPromptSchema.nodes.skillMention.create({
          token: atom.token,
          label: atom.label,
          skillId: atom.skillId,
          title: skill?.title || atom.label || atom.skillId,
          description: skill?.description || atom.skillId
        }))
      }
      cursor = atom.end
    }
    const after = textNode(input.slice(cursor, lineEnd))
    if (after) children.push(after)
    paragraphs.push(composerPromptSchema.nodes.paragraph.create(null, children))
    lineStart = lineEnd + 1
  }

  return composerPromptSchema.nodes.doc.create(null, paragraphs.length > 0
    ? paragraphs
    : [composerPromptSchema.nodes.paragraph.create()])
}

function tokenForMentionNode(node: ProseMirrorNode): string {
  if (node.type.name === 'pluginMention') {
    return node.attrs.token || formatComposerPluginMentionToken(
      node.attrs.label || node.attrs.title || node.attrs.pluginId,
      node.attrs.pluginId
    )
  }
  if (node.type.name === 'skillMention') {
    return node.attrs.token || formatComposerSkillMentionToken(
      node.attrs.label || node.attrs.title || node.attrs.skillId,
      node.attrs.skillId
    )
  }
  return ''
}

export function serializeComposerPromptDoc(doc: ProseMirrorNode): string {
  const paragraphs: string[] = []
  doc.forEach((paragraph) => {
    let text = ''
    paragraph.forEach((child) => {
      if (child.isText) {
        text += child.text ?? ''
      } else {
        text += tokenForMentionNode(child)
      }
    })
    paragraphs.push(text)
  })
  return paragraphs.join('\n')
}

export function buildComposerPromptProjection(doc: ProseMirrorNode): ComposerPromptProjection {
  const segments: ComposerPromptProjectionSegment[] = []
  let text = ''
  let rawCursor = 0
  doc.forEach((paragraph, paragraphOffset, paragraphIndex) => {
    let pmCursor = paragraphOffset + 1
    paragraph.forEach((child) => {
      if (child.isText) {
        const childText = child.text ?? ''
        const rawEnd = rawCursor + childText.length
        segments.push({
          kind: 'text',
          rawStart: rawCursor,
          rawEnd,
          pmStart: pmCursor,
          pmEnd: pmCursor + childText.length
        })
        text += childText
        rawCursor = rawEnd
        pmCursor += childText.length
        return
      }
      const token = tokenForMentionNode(child)
      const rawEnd = rawCursor + token.length
      segments.push({
        kind: 'mention',
        rawStart: rawCursor,
        rawEnd,
        pmStart: pmCursor,
        pmEnd: pmCursor + child.nodeSize
      })
      text += token
      rawCursor = rawEnd
      pmCursor += child.nodeSize
    })

    if (paragraphIndex < doc.childCount - 1) {
      const pmLineEnd = paragraphOffset + paragraph.nodeSize - 1
      const pmNextLineStart = paragraphOffset + paragraph.nodeSize + 1
      segments.push({
        kind: 'newline',
        rawStart: rawCursor,
        rawEnd: rawCursor + 1,
        pmStart: pmLineEnd,
        pmEnd: pmNextLineStart
      })
      text += '\n'
      rawCursor += 1
    }
  })
  return { text, segments }
}

function docEndSelectionPos(doc: ProseMirrorNode): number {
  return Math.max(1, doc.content.size - 1)
}

export function composerRawOffsetToPmPos(doc: ProseMirrorNode, rawOffset: number): number {
  const projection = buildComposerPromptProjection(doc)
  const boundedOffset = Math.max(0, Math.min(projection.text.length, rawOffset))
  for (const segment of projection.segments) {
    if (boundedOffset < segment.rawStart) return segment.pmStart
    if (segment.kind === 'text') {
      if (boundedOffset <= segment.rawEnd) {
        return segment.pmStart + boundedOffset - segment.rawStart
      }
      continue
    }
    if (boundedOffset <= segment.rawEnd) {
      return boundedOffset <= segment.rawStart ? segment.pmStart : segment.pmEnd
    }
  }
  return docEndSelectionPos(doc)
}

export function pmPosToComposerRawOffset(doc: ProseMirrorNode, pmPos: number): number {
  const projection = buildComposerPromptProjection(doc)
  const boundedPos = Math.max(0, Math.min(doc.content.size, pmPos))
  for (const segment of projection.segments) {
    if (boundedPos < segment.pmStart) return segment.rawStart
    if (segment.kind === 'text') {
      if (boundedPos <= segment.pmEnd) {
        return segment.rawStart + boundedPos - segment.pmStart
      }
      continue
    }
    if (boundedPos <= segment.pmEnd) {
      return boundedPos <= segment.pmStart ? segment.rawStart : segment.rawEnd
    }
  }
  return projection.text.length
}
