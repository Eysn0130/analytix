// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/xlsx-archive.ts
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** XLSX preview copies without DrawingML parts; worksheet XML and source bytes remain untouched. */
import { unzipSync, zipSync, type Unzipped } from 'fflate'
import { XMLParser } from 'fast-xml-parser'
import type { ExcelUnsupportedFeature } from './model'
import { ExcelPreviewError } from './error'

/** Owns the unpacked preview copy and its detected unsupported content. */
export class XlsxPreviewArchive {
  /** Detected workbook content that the preview does not display. */
  readonly unsupportedFeatures = new Set<ExcelUnsupportedFeature>()
  private readonly files: Unzipped
  private readonly parts = new Map<string, Uint8Array>()

  /**
   * Read an archive without modifying the borrowed source buffer.
   * @param bytes - Complete XLSX source bytes.
   * @throws When ZIP entries have ASCII case-equivalent names.
   */
  constructor(private readonly bytes: Uint8Array<ArrayBuffer>, maximumUncompressedBytes: number) {
    const expected = archiveSizes(bytes, maximumUncompressedBytes)
    this.files = unzipSync(bytes)
    let total = 0
    if (Object.keys(this.files).length !== expected.size) throw new ExcelPreviewError('invalid')
    for (const [path, part] of Object.entries(this.files)) {
      if (expected.get(path) !== part.byteLength) throw new ExcelPreviewError('invalid')
      total += part.byteLength
      if (total > maximumUncompressedBytes) throw new ExcelPreviewError('tooLarge')
    }
    for (const [path, bytes] of Object.entries(this.files)) {
      if (path.endsWith('/')) continue
      const key = partKey(path)
      if (this.parts.has(key)) throw new Error(`Ambiguous XLSX part: ${path}`)
      this.parts.set(key, bytes)
    }
  }

  /**
   * Omit DrawingML parts before ExcelJS parses them; callers must also ignore worksheet drawing references.
   * @returns Source bytes when no parts were omitted, otherwise an uncompressed temporary ZIP.
   */
  withoutDrawings(): Uint8Array<ArrayBuffer> {
    const entries = Object.entries(this.files)
    const omitted = new Set<string>()
    for (const [path, bytes] of entries) {
      // ExcelJS also discovers unreferenced drawings in its conventional directory.
      if (/^xl\/drawings\/(?:[^/]+\.xml|_rels\/[^/]+\.xml\.rels)$/i.test(path)) omitted.add(partKey(path))
      const match = /^(.*\/)?_rels\/([^/]+)\.rels$/i.exec(path)
      if (match === null) continue
      const owner = `${match[1] ?? ''}${match[2]}`
      new XMLParser({
        removeNSPrefix: true, ignoreAttributes: false, attributeNamePrefix: '', parseAttributeValue: false,
        updateTag: (tag: string, _path, attributes: Record<string, unknown>) => {
          const type = typeof attributes.Type === 'string'
            ? attributes.Type.replace(/^http:\/\/purl\.oclc\.org\/ooxml\/officeDocument\/relationships\//, 'http://schemas.openxmlformats.org/officeDocument/2006/relationships/') : undefined
          if (tag !== 'Relationship' || attributes.TargetMode === 'External') return tag
          const drawing = type === 'http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing'
          if (!drawing && type !== 'http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet') return tag
          if (typeof attributes.Target !== 'string') throw new Error('Missing XLSX relationship target')
          const target = resolvePart(owner, attributes.Target)
          const content = this.parts.get(partKey(target))
          if (content === undefined) throw new Error(`Missing XLSX part: ${target}`)
          this.inspectContent(content)
          if (drawing) {
            omitted.add(partKey(target))
            const split = target.lastIndexOf('/')
            omitted.add(partKey(`${target.slice(0, split + 1)}_rels/${target.slice(split + 1)}.rels`))
          }
          return tag
        },
      }).parse(decodeXml(bytes))
    }
    const retained = entries.filter(([path]) => !omitted.has(partKey(path)))
    return retained.length === entries.length ? this.bytes : new Uint8Array(zipSync(Object.fromEntries(retained), { level: 0 }))
  }

  private inspectContent(bytes: Uint8Array): void {
    new XMLParser({
      removeNSPrefix: true,
      processEntities: false,
      parseTagValue: false,
      stopNodes: ['*.sheetData'],
      updateTag: (tag: string) => {
        if (tag === 'chart') this.unsupportedFeatures.add('charts')
        if (tag === 'pic' || tag === 'picture') this.unsupportedFeatures.add('images')
        if (tag === 'sp' || tag === 'grpSp' || tag === 'cxnSp') this.unsupportedFeatures.add('shapes')
        if (tag === 'conditionalFormatting') this.unsupportedFeatures.add('conditionalFormatting')
        if (tag === 'sheetData') return false
        return tag
      },
    }).parse(decodeXml(bytes))
  }
}

/** OPC compares part names using ASCII case equivalence, leaving ZIP names intact. */
function partKey(path: string): string {
  return path.replace(/[A-Z]/g, character => character.toLowerCase())
}

/** Decode the mandatory XML encodings without rewriting the archived bytes. */
function decodeXml(bytes: Uint8Array): string {
  const encoding = (bytes[0] === 0xff && bytes[1] === 0xfe) || (bytes[0] === 0x3c && bytes[1] === 0)
    ? 'utf-16le'
    : (bytes[0] === 0xfe && bytes[1] === 0xff) || (bytes[0] === 0 && bytes[1] === 0x3c)
      ? 'utf-16be' : 'utf-8'
  return new TextDecoder(encoding, { fatal: true }).decode(bytes)
}

/** Resolve internal relationship targets without accepting traversal above the package root. */
function resolvePart(owner: string, target: string): string {
  const parts = target.startsWith('/') ? [] : owner.split('/').slice(0, -1)
  for (const segment of target.split('/')) {
    if (segment === '' || segment === '.') continue
    if (segment === '..') {
      if (parts.length === 0) throw new Error('XLSX relationship escapes package')
      parts.pop()
    } else parts.push(segment)
  }
  return parts.join('/')
}

/** Admit one bounded, single-disk ZIP before any inflation. ZIP64 is unnecessary within these limits. */
function archiveSizes(bytes: Uint8Array, maximum: number): Map<string, number> {
  if (!Number.isSafeInteger(maximum) || maximum <= 0 || bytes.length < 22) throw new ExcelPreviewError('invalid')
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  let end = bytes.length - 22
  const earliest = Math.max(0, end - 65535)
  for (; end >= earliest; end--) {
    if (view.getUint32(end, true) === 0x06054b50 && end + 22 + view.getUint16(end + 20, true) === bytes.length) break
  }
  if (end < earliest || view.getUint16(end + 4, true) !== 0 || view.getUint16(end + 6, true) !== 0) throw new ExcelPreviewError('invalid')
  const count = view.getUint16(end + 10, true), size = view.getUint32(end + 12, true), offset = view.getUint32(end + 16, true)
  if (count === 0xffff || size === 0xffffffff || offset === 0xffffffff || view.getUint16(end + 8, true) !== count || offset + size !== end) throw new ExcelPreviewError('invalid')
  const parts = new Map<string, number>(), keys = new Set<string>()
  let cursor = offset, total = 0
  for (let index = 0; index < count; index++) {
    if (cursor + 46 > end || view.getUint32(cursor, true) !== 0x02014b50) throw new ExcelPreviewError('invalid')
    const flags = view.getUint16(cursor + 8, true), method = view.getUint16(cursor + 10, true)
    const compressed = view.getUint32(cursor + 20, true), expanded = view.getUint32(cursor + 24, true)
    const nameSize = view.getUint16(cursor + 28, true), extraSize = view.getUint16(cursor + 30, true), commentSize = view.getUint16(cursor + 32, true)
    const local = view.getUint32(cursor + 42, true), next = cursor + 46 + nameSize + extraSize + commentSize
    if (next > end || (flags & 1) !== 0 || ![0, 8].includes(method) || view.getUint16(cursor + 34, true) !== 0 ||
        compressed === 0xffffffff || expanded === 0xffffffff || local === 0xffffffff || local + 30 > offset ||
        view.getUint32(local, true) !== 0x04034b50) throw new ExcelPreviewError('invalid')
    const dataStart = local + 30 + view.getUint16(local + 26, true) + view.getUint16(local + 28, true)
    if (dataStart + compressed > offset) throw new ExcelPreviewError('invalid')
    total += expanded
    if (!Number.isSafeInteger(total) || total > maximum) throw new ExcelPreviewError('tooLarge')
    let name: string
    try { name = new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(cursor + 46, cursor + 46 + nameSize)) }
    catch { throw new ExcelPreviewError('invalid') }
    const key = partKey(name)
    if (!name || name.includes('\0') || name.startsWith('/') || name.split('/').includes('..') || keys.has(key)) throw new ExcelPreviewError('invalid')
    keys.add(key); parts.set(name, expanded); cursor = next
  }
  if (cursor !== end) throw new ExcelPreviewError('invalid')
  return parts
}
