import { open, realpath } from 'node:fs/promises'
import { dirname, extname, isAbsolute, relative, resolve, sep } from 'node:path'

import {
  AlignmentType,
  BorderStyle,
  Document,
  ExternalHyperlink,
  HeadingLevel,
  ImageRun,
  LevelFormat,
  LineRuleType,
  PageBreak,
  Packer,
  Paragraph,
  Table,
  TableCell,
  TableLayoutType,
  TableRow,
  TextRun,
  VerticalAlignTable,
  WidthType,
  type IFontAttributesProperties,
  type ParagraphChild
} from 'docx'
import { unified } from 'unified'
import remarkGfm from 'remark-gfm'
import remarkParse from 'remark-parse'
import JSZip from 'jszip'

import {
  normalizeWriteTypography,
  writeFontStackFor
} from '../../shared/app-settings-write'
import type { WriteTypographySettingsV1 } from '../../shared/app-settings-types'
import { isWriteOfficialDocumentTypography } from '../../shared/write-official-document'
import { containsPrivateReasoningContent } from '../../shared/public-runtime-content'
import { applyWriteMarkdownAlignmentDirectivesToTree } from '../../shared/write-text-align'
import { projectWriteExportMarkdownTreeV1 } from './write-export-content'

/** The only user-authored marker which is interpreted as a Word page break. */
export const WRITE_DOCX_PAGE_BREAK_MARKER = '<!-- analytix:page-break -->'

const MAX_IMAGE_BYTES = 8 * 1024 * 1024
const MAX_TOTAL_IMAGE_BYTES = 24 * 1024 * 1024
const MAX_IMAGE_COUNT = 24
const MAX_IMAGE_DIMENSION = 10_000
const MAX_IMAGE_PIXELS = 40_000_000
const DOCX_CONTENT_WIDTH_TWIPS = 9_744
const DOCX_MAX_IMAGE_WIDTH_PX = 650
const DOCX_MAX_IMAGE_HEIGHT_PX = 950

type MarkdownNode = {
  type?: string
  value?: unknown
  url?: unknown
  alt?: unknown
  depth?: number
  ordered?: boolean | null
  start?: number | null
  align?: Array<'left' | 'center' | 'right' | null> | null
  checked?: boolean | null
  children?: MarkdownNode[]
  data?: {
    hProperties?: Record<string, unknown>
    [key: string]: unknown
  }
}

export type BuildWriteDocxDocumentOptions = {
  /** The source document is used only for extension and local-image resolution. */
  sourcePath: string
  /** The canonical workspace boundary for local image reads. */
  workspaceRoot: string
  /** Caller-owned projection. Image targets retain the format's private read gate. */
  publicContent: string
  /** Trusted caller's prose policy, applied after parsing without rewriting image targets. */
  projectProse?: (content: string) => string
  title?: string
  typography?: Partial<WriteTypographySettingsV1>
}

export type WriteDocxPackageInspectionV1 = {
  parts: string[]
  mediaParts: string[]
  externalRelationships: Array<{ type: string; target: string }>
  documentXml: string
  stylesXml: string
  relationshipsXml: string
}

type ImageInfo = {
  type: 'png' | 'jpg' | 'gif' | 'bmp'
  width: number
  height: number
  data: Buffer
}

type RenderContext = {
  workspaceRoot: string
  sourcePath: string
  font: IFontAttributesProperties
  fontSize: number
  lineHeight: number
  defaultAlignment?: (typeof AlignmentType)[keyof typeof AlignmentType]
  officialTypography: boolean
  imageCount: number
  imageBytes: number
}

type RenderOptions = {
  listLevel?: number
  blockquoteLevel?: number
  tableCell?: boolean
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function isWithin(root: string, target: string): boolean {
  const rel = relative(root, target)
  return rel === '' || (rel !== '..' && !rel.startsWith(`..${sep}`) && !isAbsolute(rel))
}

function firstFontFamily(fontStack: string): string {
  for (const candidate of fontStack.split(',')) {
    const clean = candidate.trim().replace(/^['"]|['"]$/g, '')
    if (!clean || clean.startsWith('-') || /^(serif|sans-serif|monospace|cursive|fantasy)$/i.test(clean)) continue
    return clean
  }
  return 'PingFang SC'
}

function docxFont(typography: Partial<WriteTypographySettingsV1> | undefined): IFontAttributesProperties {
  const normalized = normalizeWriteTypography(typography)
  let family = firstFontFamily(writeFontStackFor(normalized.fontPreset, normalized.customFontFamily))
  if (family === 'Arial' || family.startsWith('-')) family = 'PingFang SC'
  return { ascii: family, hAnsi: family, eastAsia: family, cs: family }
}

function docxFontSize(typography: Partial<WriteTypographySettingsV1> | undefined): number {
  const normalized = normalizeWriteTypography(typography)
  return Math.max(2, Math.round(normalized.fontSizePx * 1.5))
}

function normalizeLineHeight(typography: Partial<WriteTypographySettingsV1> | undefined): number {
  return normalizeWriteTypography(typography).lineHeight
}

function alignmentForNode(node: MarkdownNode): (typeof AlignmentType)[keyof typeof AlignmentType] | undefined {
  const value = node.data?.hProperties?.['data-write-align']
  switch (value) {
    case 'center': return AlignmentType.CENTER
    case 'right': return AlignmentType.RIGHT
    case 'justify': return AlignmentType.JUSTIFIED
    case 'left': return AlignmentType.LEFT
    default: return undefined
  }
}

function runOptions(context: RenderContext, options: { bold?: boolean; italics?: boolean; strike?: boolean; code?: boolean } = {}) {
  return {
    font: options.code
      ? { ascii: 'Menlo', hAnsi: 'Menlo', eastAsia: context.font.eastAsia, cs: context.font.cs }
      : context.font,
    size: context.fontSize,
    bold: options.bold,
    italics: options.italics,
    strike: options.strike
  }
}

function textRuns(value: string, context: RenderContext, options: { bold?: boolean; italics?: boolean; strike?: boolean; code?: boolean } = {}): TextRun[] {
  const parts = value.replace(/\r\n/g, '\n').split('\n')
  const runs: TextRun[] = []
  for (const [index, part] of parts.entries()) {
    if (part) runs.push(new TextRun({ text: part, ...runOptions(context, options) }))
    if (index < parts.length - 1) runs.push(new TextRun({ break: 1, ...runOptions(context, options) }))
  }
  return runs
}

function plainInlineText(node: MarkdownNode): string {
  if (node.type === 'image') return asText(node.alt)
  if (node.type === 'break') return '\n'
  if (node.type === 'text' || node.type === 'inlineCode' || node.type === 'html' || node.type === 'code') return asText(node.value)
  return (node.children ?? []).map(plainInlineText).join('')
}

function allowedExternalLink(value: string): string | null {
  if (!value || Array.from(value).some((character) => {
    const code = character.charCodeAt(0)
    return code <= 0x1f || code === 0x7f
  })) return null
  try {
    const url = new URL(value)
    if (url.protocol !== 'https:' && url.protocol !== 'http:' && url.protocol !== 'mailto:') return null
    return url.href
  } catch {
    return null
  }
}

function decodeXmlAttribute(value: string): string {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
}

function imageExtensionType(source: string): ImageInfo['type'] | null {
  switch (extname(source).toLowerCase()) {
    case '.png': return 'png'
    case '.jpg':
    case '.jpeg': return 'jpg'
    case '.gif': return 'gif'
    case '.bmp': return 'bmp'
    default: return null
  }
}

function validateDimensions(width: number, height: number): void {
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) {
    throw new Error('DOCX image dimensions are invalid.')
  }
  if (width > MAX_IMAGE_DIMENSION || height > MAX_IMAGE_DIMENSION || width * height > MAX_IMAGE_PIXELS) {
    throw new Error('DOCX image dimensions exceed the safe limit.')
  }
}

function parsePng(buffer: Buffer): { width: number; height: number } | null {
  if (buffer.length < 24 || !buffer.subarray(0, 8).equals(Buffer.from('89504e470d0a1a0a', 'hex'))) return null
  return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) }
}

function parseGif(buffer: Buffer): { width: number; height: number } | null {
  if (buffer.length < 10 || (buffer.subarray(0, 6).toString('ascii') !== 'GIF87a' && buffer.subarray(0, 6).toString('ascii') !== 'GIF89a')) return null
  return { width: buffer.readUInt16LE(6), height: buffer.readUInt16LE(8) }
}

function parseBmp(buffer: Buffer): { width: number; height: number } | null {
  if (buffer.length < 26 || buffer.subarray(0, 2).toString('ascii') !== 'BM') return null
  const dibSize = buffer.readUInt32LE(14)
  if (dibSize < 12 || buffer.length < 14 + dibSize) return null
  if (dibSize === 12) return { width: buffer.readUInt16LE(18), height: buffer.readUInt16LE(20) }
  return { width: Math.abs(buffer.readInt32LE(18)), height: Math.abs(buffer.readInt32LE(22)) }
}

function parseJpeg(buffer: Buffer): { width: number; height: number } | null {
  if (buffer.length < 4 || buffer[0] !== 0xff || buffer[1] !== 0xd8) return null
  let offset = 2
  while (offset + 3 < buffer.length) {
    if (buffer[offset] !== 0xff) {
      offset += 1
      continue
    }
    while (offset < buffer.length && buffer[offset] === 0xff) offset += 1
    const marker = buffer[offset++]
    if (marker === 0xd9 || marker === 0xda) break
    if (marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) continue
    if (offset + 1 >= buffer.length) break
    const segmentLength = buffer.readUInt16BE(offset)
    if (segmentLength < 2 || offset + segmentLength > buffer.length) break
    const isSof = (marker >= 0xc0 && marker <= 0xc3) || (marker >= 0xc5 && marker <= 0xc7) || (marker >= 0xc9 && marker <= 0xcb) || (marker >= 0xcd && marker <= 0xcf)
    if (isSof && segmentLength >= 7) {
      return { height: buffer.readUInt16BE(offset + 3), width: buffer.readUInt16BE(offset + 5) }
    }
    offset += segmentLength
  }
  return null
}

function parseImageDimensions(buffer: Buffer, type: ImageInfo['type']): { width: number; height: number } | null {
  if (type === 'png') return parsePng(buffer)
  if (type === 'jpg') return parseJpeg(buffer)
  if (type === 'gif') return parseGif(buffer)
  return parseBmp(buffer)
}

async function readWorkspaceImage(source: string, context: RenderContext): Promise<ImageInfo> {
  if (/^[a-z][a-z0-9+.-]*:/i.test(source)) throw new Error('DOCX images must be local workspace files.')
  if (source.trim() !== source || source.includes('\0')) throw new Error('DOCX image path is invalid.')
  const candidate = resolve(context.sourcePath ? dirname(context.sourcePath) : context.workspaceRoot, source)
  let canonical: string
  try {
    canonical = await realpath(candidate)
  } catch {
    throw new Error('DOCX image file was not found.')
  }
  if (!isWithin(context.workspaceRoot, canonical)) throw new Error('DOCX image escaped the canonical workspace.')
  const type = imageExtensionType(canonical)
  if (!type) throw new Error('DOCX image format is not supported.')
  const handle = await open(canonical, 'r')
  let data: Buffer
  let expectedSize = 0
  try {
    const metadata = await handle.stat()
    if (!metadata.isFile() || metadata.size > MAX_IMAGE_BYTES) throw new Error('DOCX image exceeds the safe size limit.')
    if (context.imageCount >= MAX_IMAGE_COUNT) throw new Error('DOCX image count exceeds the safe limit.')
    if (context.imageBytes + metadata.size > MAX_TOTAL_IMAGE_BYTES) throw new Error('DOCX image total size exceeds the safe limit.')
    expectedSize = metadata.size
    data = await handle.readFile()
  } finally {
    await handle.close()
  }
  let currentCanonical: string
  try {
    currentCanonical = await realpath(candidate)
  } catch {
    throw new Error('DOCX image authority changed while reading.')
  }
  if (currentCanonical !== canonical || data.length !== expectedSize) {
    throw new Error('DOCX image authority changed while reading.')
  }
  const dimensions = parseImageDimensions(data, type)
  if (!dimensions) throw new Error('DOCX image signature does not match its extension.')
  validateDimensions(dimensions.width, dimensions.height)
  context.imageCount += 1
  context.imageBytes += data.length
  return { ...dimensions, type, data }
}

function imageTransformation(image: ImageInfo): { width: number; height: number } {
  const scale = Math.min(1, DOCX_MAX_IMAGE_WIDTH_PX / image.width, DOCX_MAX_IMAGE_HEIGHT_PX / image.height)
  return {
    width: Math.max(1, Math.round(image.width * scale)),
    height: Math.max(1, Math.round(image.height * scale))
  }
}

async function inlineChildren(node: MarkdownNode, context: RenderContext, allowLinks = true): Promise<ParagraphChild[]> {
  const output: ParagraphChild[] = []
  switch (node.type) {
    case 'text': output.push(...textRuns(asText(node.value), context)); break
    case 'strong':
      for (const child of node.children ?? []) output.push(...await inlineChildrenWithRunOptions(child, context, { bold: true }, allowLinks))
      break
    case 'emphasis':
      for (const child of node.children ?? []) output.push(...await inlineChildrenWithRunOptions(child, context, { italics: true }, allowLinks))
      break
    case 'delete':
      for (const child of node.children ?? []) output.push(...await inlineChildrenWithRunOptions(child, context, { strike: true }, allowLinks))
      break
    case 'inlineCode': output.push(...textRuns(asText(node.value), context, { code: true })); break
    case 'break': output.push(new TextRun({ break: 1, ...runOptions(context) })); break
    case 'link': {
      const label = (node.children ?? []).map(plainInlineText).join('') || asText(node.url)
      const href = allowedExternalLink(asText((node as MarkdownNode & { url?: unknown }).url))
      if (!allowLinks || !href) output.push(...textRuns(label, context))
      else output.push(new ExternalHyperlink({ link: href, children: textRuns(label, context) }))
      break
    }
    case 'image': {
      const url = asText((node as MarkdownNode & { url?: unknown }).url)
      const image = await readWorkspaceImage(url, context)
      output.push(new ImageRun({
        type: image.type,
        data: image.data,
        transformation: imageTransformation(image),
        altText: { name: 'analytix-image', description: asText((node as MarkdownNode & { alt?: unknown }).alt) }
      }))
      break
    }
    case 'html': output.push(...textRuns(asText(node.value), context)); break
    default:
      if (typeof node.value === 'string') output.push(...textRuns(node.value, context))
      else for (const child of node.children ?? []) output.push(...await inlineChildren(child, context, allowLinks))
  }
  return output
}

async function inlineChildrenWithRunOptions(
  node: MarkdownNode,
  context: RenderContext,
  options: { bold?: boolean; italics?: boolean; strike?: boolean },
  allowLinks: boolean
): Promise<ParagraphChild[]> {
  if (node.type === 'text' || node.type === 'inlineCode' || node.type === 'html') {
    return textRuns(asText(node.value), context, { ...options, code: node.type === 'inlineCode' })
  }
  if (node.type === 'link') {
    const label = (node.children ?? []).map(plainInlineText).join('') || asText((node as MarkdownNode & { url?: unknown }).url)
    const href = allowedExternalLink(asText((node as MarkdownNode & { url?: unknown }).url))
    return href && allowLinks
      ? [new ExternalHyperlink({ link: href, children: textRuns(label, context, options) })]
      : textRuns(label, context, options)
  }
  if (node.type === 'strong' || node.type === 'emphasis' || node.type === 'delete') {
    const nestedOptions = {
      ...options,
      bold: options.bold || node.type === 'strong',
      italics: options.italics || node.type === 'emphasis',
      strike: options.strike || node.type === 'delete'
    }
    const result: ParagraphChild[] = []
    for (const child of node.children ?? []) result.push(...await inlineChildrenWithRunOptions(child, context, nestedOptions, allowLinks))
    return result
  }
  if (node.type === 'image' || node.type === 'break') return inlineChildren(node, context, allowLinks)
  const result: ParagraphChild[] = []
  for (const child of node.children ?? []) result.push(...await inlineChildrenWithRunOptions(child, context, options, allowLinks))
  return result
}

function paragraphProperties(context: RenderContext, node: MarkdownNode, options: RenderOptions = {}) {
  const alignment = alignmentForNode(node) ?? context.defaultAlignment
  const official = context.officialTypography
  const isHeading = node.type === 'heading'
  const shouldCenterOfficialHeading = official && isHeading && (node as MarkdownNode & { depth?: number }).depth !== undefined && ((node as MarkdownNode & { depth?: number }).depth === 1 || (node as MarkdownNode & { depth?: number }).depth === 2)
  return {
    alignment: shouldCenterOfficialHeading ? AlignmentType.CENTER : alignment,
    spacing: { after: isHeading ? 120 : 160, line: Math.round(context.lineHeight * 240), lineRule: LineRuleType.AUTO },
    indent: official && node.type === 'paragraph' && !options.tableCell && !options.listLevel && !options.blockquoteLevel
      ? { firstLineChars: 200 }
      : options.listLevel
        ? { left: 720 * options.listLevel, hanging: 360 }
        : undefined,
    border: options.blockquoteLevel
      ? { left: { style: BorderStyle.SINGLE, size: 6, color: 'A0AEC0', space: 6 } }
      : undefined,
    keepNext: isHeading ? true : undefined,
    widowControl: true,
    autoSpaceEastAsianText: true
  }
}

async function renderParagraph(node: MarkdownNode, context: RenderContext, options: RenderOptions = {}): Promise<Paragraph> {
  const children = await inlineChildrenForNode(node, context)
  const depth = (node as MarkdownNode & { depth?: number }).depth
  const headingByDepth: Record<number, (typeof HeadingLevel)[keyof typeof HeadingLevel]> = {
    1: HeadingLevel.HEADING_1,
    2: HeadingLevel.HEADING_2,
    3: HeadingLevel.HEADING_3,
    4: HeadingLevel.HEADING_4,
    5: HeadingLevel.HEADING_5,
    6: HeadingLevel.HEADING_6
  }
  const heading = typeof depth === 'number' ? headingByDepth[depth] : undefined
  return new Paragraph({
    children: children.length ? children : [new TextRun({ text: '', ...runOptions(context) })],
    heading,
    ...paragraphProperties(context, node, options)
  })
}

async function inlineChildrenForNode(node: MarkdownNode, context: RenderContext): Promise<ParagraphChild[]> {
  const output: ParagraphChild[] = []
  for (const child of node.children ?? []) output.push(...await inlineChildren(child, context))
  return output
}

async function renderBlocks(nodes: MarkdownNode[], context: RenderContext, options: RenderOptions = {}): Promise<Array<Paragraph | Table>> {
  const output: Array<Paragraph | Table> = []
  for (const node of nodes) {
    switch (node.type) {
      case 'heading':
      case 'paragraph':
      case 'code':
      case 'thematicBreak': {
        if (node.type === 'thematicBreak') {
          output.push(new Paragraph({ children: [new TextRun({ text: '────────────────', ...runOptions(context) })], ...paragraphProperties(context, node, options) }))
        } else {
          const paragraphNode = node.type === 'code' ? { ...node, children: [{ type: 'inlineCode', value: asText(node.value) }] } : node
          output.push(await renderParagraph(paragraphNode, context, options))
        }
        break
      }
      case 'blockquote':
        output.push(...await renderBlocks(node.children ?? [], context, { ...options, blockquoteLevel: (options.blockquoteLevel ?? 0) + 1 }))
        break
      case 'list':
        output.push(...await renderList(node, context, options.listLevel ?? 0))
        break
      case 'table':
        output.push(await renderTable(node, context))
        break
      case 'html':
        if (node.value === WRITE_DOCX_PAGE_BREAK_MARKER) output.push(new Paragraph({ children: [new PageBreak()] }))
        else output.push(new Paragraph({ children: textRuns(asText(node.value), context), ...paragraphProperties(context, node, options) }))
        break
      default:
        if (node.children?.length) output.push(...await renderBlocks(node.children, context, options))
        else if (typeof node.value === 'string') output.push(new Paragraph({ children: textRuns(node.value, context), ...paragraphProperties(context, node, options) }))
        break
    }
  }
  return output
}

async function renderList(node: MarkdownNode, context: RenderContext, level: number): Promise<Paragraph[]> {
  const ordered = Boolean(node.ordered)
  const reference = ordered ? 'analytix-numbered-list' : 'analytix-bullet-list'
  const paragraphs: Paragraph[] = []
  for (const item of node.children ?? []) {
    const children = item.children ?? []
    const first = children.find((child) => child.type === 'paragraph')
    if (first) {
      paragraphs.push(new Paragraph({
        children: await inlineChildrenForNode(first, context),
        numbering: { reference, level: Math.min(level, 8) },
        ...paragraphProperties(context, first, { listLevel: level + 1 })
      }))
    }
    for (const child of children) {
      if (child === first) continue
      if (child.type === 'list') paragraphs.push(...await renderList(child, context, level + 1))
      else if (child.type === 'paragraph') paragraphs.push(await renderParagraph(child, context, { listLevel: level + 1 }))
    }
  }
  return paragraphs
}

async function renderTable(node: MarkdownNode, context: RenderContext): Promise<Table> {
  const rows = node.children ?? []
  const columnCount = Math.max(1, ...rows.map((row) => row.children?.length ?? 0))
  const columnWidth = Math.max(720, Math.floor(DOCX_CONTENT_WIDTH_TWIPS / columnCount))
  const tableRows: TableRow[] = []
  for (const [rowIndex, row] of rows.entries()) {
    const cells: TableCell[] = []
    for (let index = 0; index < columnCount; index += 1) {
      const cell = row.children?.[index]
      const cellParagraph = new Paragraph({
        children: cell ? await inlineChildrenForNode(cell, context) : [new TextRun({ text: '', ...runOptions(context) })],
        ...paragraphProperties(context, cell ?? { type: 'paragraph' }, { tableCell: true })
      })
      cells.push(new TableCell({
        children: [cellParagraph],
        width: { size: columnWidth, type: WidthType.DXA },
        verticalAlign: VerticalAlignTable.CENTER,
        shading: rowIndex === 0 ? { fill: 'E5E7EB' } : undefined
      }))
    }
    tableRows.push(new TableRow({ children: cells }))
  }
  return new Table({
    rows: tableRows,
    width: { size: DOCX_CONTENT_WIDTH_TWIPS, type: WidthType.DXA },
    columnWidths: Array.from({ length: columnCount }, () => columnWidth),
    layout: TableLayoutType.FIXED,
    borders: {
      top: { style: BorderStyle.SINGLE, size: 4, color: 'B7C0CE' },
      bottom: { style: BorderStyle.SINGLE, size: 4, color: 'B7C0CE' },
      left: { style: BorderStyle.SINGLE, size: 4, color: 'B7C0CE' },
      right: { style: BorderStyle.SINGLE, size: 4, color: 'B7C0CE' },
      insideHorizontal: { style: BorderStyle.SINGLE, size: 4, color: 'D3D8E0' },
      insideVertical: { style: BorderStyle.SINGLE, size: 4, color: 'D3D8E0' }
    }
  })
}

function parsePlainText(content: string): MarkdownNode {
  const children: MarkdownNode[] = []
  const lines = content.replace(/\r\n/g, '\n').split('\n')
  let paragraphLines: string[] = []
  const flush = (): void => {
    if (paragraphLines.length) {
      children.push({ type: 'paragraph', children: [{ type: 'text', value: paragraphLines.join('\n') }] })
      paragraphLines = []
    }
  }
  for (const line of lines) {
    if (line === WRITE_DOCX_PAGE_BREAK_MARKER) {
      flush()
      children.push({ type: 'html', value: WRITE_DOCX_PAGE_BREAK_MARKER })
    } else {
      paragraphLines.push(line)
    }
  }
  flush()
  if (!children.length) children.push({ type: 'paragraph', children: [{ type: 'text', value: '' }] })
  return { type: 'root', children }
}

function parseContent(sourcePath: string, content: string, projectProse?: (content: string) => string): MarkdownNode {
  const extension = extname(sourcePath).toLowerCase()
  const tree = extension === '.txt' || extension === '.text'
    ? parsePlainText(content)
    : unified().use(remarkParse).use(remarkGfm).parse(content) as unknown as MarkdownNode
  applyWriteMarkdownAlignmentDirectivesToTree(tree)
  if (projectProse) projectWriteExportMarkdownTreeV1(tree, projectProse)
  return tree
}

function numberingLevels(format: typeof LevelFormat.DECIMAL | typeof LevelFormat.BULLET): Array<{ level: number; format: typeof format; text: string; alignment: typeof AlignmentType.LEFT }> {
  return Array.from({ length: 9 }, (_, level) => ({
    level,
    format,
    text: format === LevelFormat.BULLET ? '•' : `%${level + 1}.`,
    alignment: AlignmentType.LEFT
  }))
}

/**
 * Performs the narrow package checks that matter at this local boundary. It is
 * intentionally structural: Word/WPS opening still belongs to packaged
 * evidence, while external image/file relationships are rejected here.
 */
export async function inspectWriteDocxPackageV1(buffer: Buffer): Promise<WriteDocxPackageInspectionV1> {
  const zip = await JSZip.loadAsync(buffer)
  const parts = Object.keys(zip.files)
  const required = ['[Content_Types].xml', '_rels/.rels', 'word/document.xml', 'word/styles.xml', 'word/_rels/document.xml.rels']
  for (const part of required) if (!zip.file(part)) throw new Error(`DOCX package is missing ${part}.`)
  const documentXml = await zip.file('word/document.xml')!.async('string')
  const stylesXml = await zip.file('word/styles.xml')!.async('string')
  const relationshipsXml = await zip.file('word/_rels/document.xml.rels')!.async('string')
  if (!/<w:document\b/.test(documentXml) || !/<w:body\b/.test(documentXml)) throw new Error('DOCX document XML is invalid.')
  if (!/<w:styles\b/.test(stylesXml)) throw new Error('DOCX styles XML is invalid.')
  const externalRelationships: Array<{ type: string; target: string }> = []
  const relationshipRegex = /<Relationship\b([^>]*?)\/?>(?:<\/Relationship>)?/g
  for (const match of relationshipsXml.matchAll(relationshipRegex)) {
    const attrs = match[1] ?? ''
    const type = decodeXmlAttribute(/\bType="([^"]+)"/.exec(attrs)?.[1] ?? '')
    const target = decodeXmlAttribute(/\bTarget="([^"]*)"/.exec(attrs)?.[1] ?? '')
    const targetMode = /\bTargetMode="([^"]+)"/.exec(attrs)?.[1] ?? ''
    if (targetMode === 'External') {
      if (type.endsWith('/image') || !type.endsWith('/hyperlink') || !allowedExternalLink(target)) {
        throw new Error('DOCX package contains an external file or image relationship.')
      }
      externalRelationships.push({ type, target })
    }
    if (type.endsWith('/image') && (target.startsWith('/') || target.includes('..'))) throw new Error('DOCX image relationship escaped the package.')
  }
  const mediaParts = parts.filter((part) => part.startsWith('word/media/') && !zip.files[part]?.dir)
  if (mediaParts.some((part) => part.includes('..') || part.endsWith('/'))) throw new Error('DOCX media part path is invalid.')
  return { parts, mediaParts, externalRelationships, documentXml, stylesXml, relationshipsXml }
}

export async function buildWriteDocxDocument(options: BuildWriteDocxDocumentOptions): Promise<Buffer> {
  if (!options || typeof options.publicContent !== 'string') throw new Error('DOCX public content is required.')
  if (!options.sourcePath || !options.workspaceRoot) throw new Error('DOCX source and workspace paths are required.')
  if (containsPrivateReasoningContent(options.publicContent)) throw new Error('DOCX public content contains private reasoning.')
  const workspaceRoot = await realpath(resolve(options.workspaceRoot))
  const sourceCandidate = isAbsolute(options.sourcePath)
    ? resolve(options.sourcePath)
    : resolve(workspaceRoot, options.sourcePath)
  let sourcePath = sourceCandidate
  try { sourcePath = await realpath(sourceCandidate) } catch { /* unsaved source: lexical containment remains required */ }
  if (!isWithin(workspaceRoot, sourcePath)) throw new Error('DOCX source escaped the canonical workspace.')
  const normalized = normalizeWriteTypography(options.typography)
  const context: RenderContext = {
    workspaceRoot,
    sourcePath,
    font: docxFont(options.typography),
    fontSize: docxFontSize(options.typography),
    lineHeight: normalizeLineHeight(options.typography),
    defaultAlignment: normalizeWriteTypography(options.typography).textAlign === 'center'
      ? AlignmentType.CENTER
      : normalizeWriteTypography(options.typography).textAlign === 'right'
        ? AlignmentType.RIGHT
        : normalizeWriteTypography(options.typography).textAlign === 'justify'
          ? AlignmentType.JUSTIFIED
          : AlignmentType.LEFT,
    officialTypography: isWriteOfficialDocumentTypography(normalized),
    imageCount: 0,
    imageBytes: 0
  }
  const tree = parseContent(sourcePath, options.publicContent, options.projectProse)
  const children = await renderBlocks(tree.children ?? [], context)
  const document = new Document({
    title: options.title,
    creator: 'Analytix Desktop Write',
    description: 'Analytix host-owned document artifact',
    numbering: {
      config: [
        { reference: 'analytix-bullet-list', levels: numberingLevels(LevelFormat.BULLET) },
        { reference: 'analytix-numbered-list', levels: numberingLevels(LevelFormat.DECIMAL) }
      ]
    },
    sections: [{
      properties: {
        page: {
          size: { width: 11906, height: 16838 },
          margin: { top: 1080, right: 1080, bottom: 1080, left: 1080 }
        }
      },
      children: children.length ? children : [new Paragraph({ children: [new TextRun({ text: '', ...runOptions(context) })] })]
    }]
  })
  const buffer = await Packer.toBuffer(document)
  await inspectWriteDocxPackageV1(buffer)
  return buffer
}
