import { types } from 'node:util'
import PptxGenJS from 'pptxgenjs'
import JSZip from 'jszip'
import { validateDocumentImageBytes, DOCUMENT_DOCX_IMAGE_LIMITS, type DocumentDocxImage } from '../services/write-docx-service'
import type { PresentationObjectV1, PresentationSpecV1 } from './presentation-generation-types'

const WIDTH = 13.333333
const HEIGHT = 7.5
const FONT = 'Noto Sans CJK SC'
const ID = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/
const COLOR = /^[0-9A-Fa-f]{6}$/
const COMMON = ['id', 'kind', 'x', 'y', 'w', 'h']
const FIELDS = {
  text: ['text', 'fontSize', 'bold', 'color', 'align'],
  shape: ['shape', 'fill', 'lineColor', 'lineWidth'],
  chart: ['chartType', 'title', 'categories', 'series'],
  image: ['imageId']
} as const

export type BuildPresentationPptxOptions = { title?: string; images?: ReadonlyMap<string, DocumentDocxImage> }

function invalid(): Error { return new Error('presentation-generation-invalid-input') }

function record(input: unknown, allowed: readonly string[], required: readonly string[]): Record<string, unknown> {
  if (!input || typeof input !== 'object' || types.isProxy(input) || (Object.getPrototypeOf(input) !== Object.prototype && Object.getPrototypeOf(input) !== null)) throw invalid()
  const value: Record<string, unknown> = Object.create(null)
  for (const key of Reflect.ownKeys(input)) {
    if (typeof key !== 'string' || !allowed.includes(key)) throw invalid()
    const descriptor = Object.getOwnPropertyDescriptor(input, key)!
    if (!('value' in descriptor)) throw invalid()
    value[key] = descriptor.value
  }
  if (required.some((key) => !Object.hasOwn(value, key))) throw invalid()
  return value
}

function array(input: unknown, min: number, max: number): unknown[] {
  if (!Array.isArray(input) || types.isProxy(input) || Object.getPrototypeOf(input) !== Array.prototype) throw invalid()
  const length = Object.getOwnPropertyDescriptor(input, 'length')!.value as number
  if (length < min || length > max || Reflect.ownKeys(input).length !== length + 1) throw invalid()
  const result: unknown[] = []
  for (let index = 0; index < length; index += 1) {
    const descriptor = Object.getOwnPropertyDescriptor(input, String(index))
    if (!descriptor || !('value' in descriptor)) throw invalid()
    result.push(descriptor.value)
  }
  return result
}

function number(input: unknown, min: number, max: number): number {
  if (typeof input !== 'number' || !Number.isFinite(input) || input < min || input > max) throw invalid()
  return input
}

function identifier(input: unknown): string {
  if (typeof input !== 'string' || !ID.test(input)) throw invalid()
  return input
}

function color(input: unknown): string {
  if (typeof input !== 'string' || !COLOR.test(input)) throw invalid()
  return input.toUpperCase()
}

function choice<T extends string>(input: unknown, allowed: readonly T[]): T {
  if (typeof input !== 'string' || !allowed.includes(input as T)) throw invalid()
  return input as T
}

function parseSpec(input: unknown, title: unknown): { spec: PresentationSpecV1; title?: string } {
  let textLength = 0
  let objectCount = 0
  const ids = new Set<string>()
  const unique = (input: unknown): string => {
    const id = identifier(input)
    if (ids.has(id)) throw invalid()
    ids.add(id)
    return id
  }
  const text = (input: unknown, max = 6000): string => {
    if (typeof input !== 'string' || input.length > max) throw invalid()
    for (const character of input) {
      const code = character.codePointAt(0)!
      if ((code < 32 && code !== 9 && code !== 10 && code !== 13) || (code >= 0xd800 && code <= 0xdfff) || code === 0xfffe || code === 0xffff) throw invalid()
    }
    textLength += input.length
    if (textLength > 100000) throw invalid()
    return input
  }
  const parsedTitle = title === undefined ? undefined : text(title, 256)
  const deck = record(input, ['slides'], ['slides'])
  const slides = array(deck.slides, 1, 50).map((rawSlide) => {
    const slide = record(rawSlide, ['id', 'background', 'objects'], ['id', 'objects'])
    const id = unique(slide.id)
    const background = Object.hasOwn(slide, 'background') ? color(slide.background) : undefined
    const objects = array(slide.objects, 1, 100).map((rawObject): PresentationObjectV1 => {
      const object = record(rawObject, [...COMMON, ...Object.values(FIELDS).flat()], COMMON)
      const kind = choice(object.kind, ['text', 'shape', 'chart', 'image'] as const)
      if (Object.keys(object).some((key) => !COMMON.includes(key) && !(FIELDS[kind] as readonly string[]).includes(key))) throw invalid()
      if (++objectCount > 1000) throw invalid()
      const shape = kind === 'shape' ? choice(object.shape, ['rect', 'ellipse', 'line'] as const) : undefined
      const x = number(object.x, 0, WIDTH), y = number(object.y, 0, HEIGHT)
      const w = number(object.w, 0, WIDTH), h = number(object.h, 0, HEIGHT)
      if (x + w > WIDTH || y + h > HEIGHT || (shape === 'line' ? w === 0 && h === 0 : w === 0 || h === 0)) throw invalid()
      const position = { id: unique(object.id), x, y, w, h }
      if (kind === 'text') {
        if (Object.hasOwn(object, 'bold') && typeof object.bold !== 'boolean') throw invalid()
        return { ...position, kind, text: text(object.text),
          fontSize: Object.hasOwn(object, 'fontSize') ? number(object.fontSize, 10, 60) : undefined,
          bold: object.bold as boolean | undefined,
          color: Object.hasOwn(object, 'color') ? color(object.color) : undefined,
          align: Object.hasOwn(object, 'align') ? choice(object.align, ['left', 'center', 'right'] as const) : undefined }
      }
      if (kind === 'shape') return { ...position, kind, shape: shape!,
        fill: Object.hasOwn(object, 'fill') ? color(object.fill) : undefined,
        lineColor: Object.hasOwn(object, 'lineColor') ? color(object.lineColor) : undefined,
        lineWidth: Object.hasOwn(object, 'lineWidth') ? number(object.lineWidth, 0, 20) : undefined }
      if (kind === 'image') return { ...position, kind, imageId: identifier(object.imageId) }
      const chartType = choice(object.chartType, ['bar', 'line', 'pie'] as const)
      const categories = array(object.categories, 1, 100).map((label) => text(label))
      const series = array(object.series, 1, 8).map((rawSeries) => {
        const series = record(rawSeries, ['name', 'values'], ['name', 'values'])
        return { name: text(series.name), values: array(series.values, categories.length, categories.length).map((value) => number(value, -Number.MAX_VALUE, Number.MAX_VALUE)) }
      })
      return { ...position, kind, chartType, categories, series, title: Object.hasOwn(object, 'title') ? text(object.title, 256) : undefined }
    })
    return { id, background, objects }
  })
  return { spec: { slides }, title: parsedTitle }
}

function snapshotImages(input: unknown): ReadonlyMap<string, DocumentDocxImage> {
  const images = new Map<string, DocumentDocxImage>()
  if (input === undefined) return images
  if (!input || types.isProxy(input) || Object.getPrototypeOf(input) !== Map.prototype || Reflect.ownKeys(input).length !== 0) throw invalid()
  if (Object.getOwnPropertyDescriptor(Map.prototype, 'size')!.get!.call(input) > DOCUMENT_DOCX_IMAGE_LIMITS.maxCount) throw invalid()
  for (const [key, raw] of Map.prototype.entries.call(input) as Iterable<[unknown, unknown]>) {
    const id = identifier(key)
    const image = record(raw, ['type', 'data'], ['type', 'data'])
    if (types.isProxy(image.data) || !Buffer.isBuffer(image.data)) throw invalid()
    images.set(id, { type: choice(image.type, ['png', 'jpg', 'gif', 'bmp'] as const), data: image.data })
  }
  return images
}

async function finalizePackage(bytes: Buffer, slides: PresentationSpecV1['slides']): Promise<Buffer> {
  const zip = await JSZip.loadAsync(bytes)
  if (!zip.file('[Content_Types].xml') || !zip.file('ppt/presentation.xml')) throw invalid()
  for (let index = 0; index < slides.length; index += 1) {
    const name = `ppt/slides/slide${index + 1}.xml`
    const part = zip.file(name)
    if (!part) throw invalid()
    const xml = await part.async('string')
    // Only the pinned builder's default tag in our freshly generated slide may be changed.
    const original = `<p:cSld name="Slide ${index + 1}">`
    if (xml.match(/<p:cSld(?:\s[^>]*)?>/g)?.length !== 1 || !xml.includes(original)) throw invalid()
    zip.file(name, xml.replace(original, `<p:cSld name="${slides[index]!.id}">`))
  }
  for (const [name, file] of Object.entries(zip.files)) {
    if (name.startsWith('/') || name.split('/').includes('..')) throw invalid()
    if (name.endsWith('.rels') && /\bTargetMode\s*=\s*["']External["']/i.test(await file.async('string'))) throw invalid()
  }
  return zip.generateAsync({ type: 'nodebuffer', compression: 'DEFLATE' })
}

/** Pure data adapter: object IDs and validated bytes are the only non-text inputs passed to the builder. */
export async function buildPresentationPptxBytes(spec: unknown, options?: BuildPresentationPptxOptions): Promise<Buffer> {
  try {
    const settings = record(options === undefined ? {} : options, ['title', 'images'], [])
    if (Object.hasOwn(settings, 'title') && typeof settings.title !== 'string') throw invalid()
    if (Object.hasOwn(settings, 'images') && settings.images === undefined) throw invalid()
    const parsed = parseSpec(spec, settings.title)
    const images = await validateDocumentImageBytes(snapshotImages(settings.images))
    let embeddedBytes = 0
    for (const slide of parsed.spec.slides) for (const object of slide.objects) {
      if (object.kind !== 'image') continue
      const image = images.get(object.imageId)
      if (!image) throw invalid()
      embeddedBytes += image.data.length
      if (embeddedBytes > DOCUMENT_DOCX_IMAGE_LIMITS.maxTotalBytes) throw invalid()
    }
    const deck = new PptxGenJS()
    deck.layout = 'LAYOUT_WIDE'
    deck.author = 'Analytix'
    deck.subject = 'Analytix presentation'
    deck.title = parsed.title ?? ''
    deck.theme = { headFontFace: FONT, bodyFontFace: FONT }
    for (const source of parsed.spec.slides) {
      const slide = deck.addSlide()
      slide.background = { color: source.background ?? 'FFFFFF' }
      for (const object of source.objects) {
        const position = { x: object.x, y: object.y, w: object.w, h: object.h, objectName: object.id }
        if (object.kind === 'text') {
          slide.addText(object.text, {
            ...position, fontFace: FONT, fontSize: object.fontSize ?? 24,
            bold: object.bold ?? false, color: object.color ?? '172033', align: object.align ?? 'left',
            margin: 0.05, breakLine: false, valign: 'top', fit: 'shrink', lang: 'zh-CN'
          })
        } else if (object.kind === 'shape') {
          slide.addShape(deck.ShapeType[object.shape], {
            ...position,
            fill: { color: object.fill ?? 'E8EEF6', transparency: object.shape === 'line' ? 100 : 0 },
            line: { color: object.lineColor ?? '64748B', width: object.lineWidth ?? 1, transparency: object.lineWidth === 0 ? 100 : 0 }
          })
        } else if (object.kind === 'chart') {
          slide.addChart(deck.ChartType[object.chartType], object.series.map((series) => ({
            name: series.name, labels: [...object.categories], values: [...series.values]
          })), {
            ...position, title: object.title, showTitle: Boolean(object.title),
            showLegend: object.series.length > 1 || object.chartType === 'pie', showValue: false, legendPos: 'b',
            titleFontFace: FONT, titleFontSize: 18, catAxisLabelFontFace: FONT, catAxisLabelFontSize: 12,
            valAxisLabelFontFace: FONT, valAxisLabelFontSize: 12, legendFontFace: FONT, legendFontSize: 12,
            chartColors: ['2563EB', '0F766E', 'EA580C', '7C3AED', 'DC2626', '0891B2', 'A16207', '475569']
          })
        } else {
          const image = images.get(object.imageId)
          if (!image) throw invalid()
          const scale = Math.min(object.w / image.width, object.h / image.height)
          const w = image.width * scale, h = image.height * scale
          const mime = image.type === 'jpg' ? 'jpeg' : image.type
          slide.addImage({
            ...position, x: object.x + (object.w - w) / 2, y: object.y + (object.h - h) / 2,
            w, h, data: `data:image/${mime};base64,${image.data.toString('base64')}`
          })
        }
      }
    }
    const result = await deck.write({ outputType: 'nodebuffer', compression: true })
    if (!Buffer.isBuffer(result)) throw invalid()
    return await finalizePackage(result, parsed.spec.slides)
  } catch { throw invalid() }
}
