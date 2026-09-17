import { describe, expect, it, vi } from 'vitest'
import JSZip from 'jszip'
import * as fs from 'node:fs/promises'
import * as path from 'node:path'
import { buildPresentationPptxBytes } from './presentation-generation-codec'
import { validateDocumentImageBytes, type DocumentDocxImage } from '../services/write-docx-service'

vi.mock('node:fs/promises', { spy: true })
vi.mock('node:path', { spy: true })

const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64')
const text = () => ({ id: 'text-one', kind: 'text', x: 0.5, y: 0.5, w: 12, h: 1, text: '中文<&报告 😀' })
const spec = (object: Record<string, unknown> = text()) => ({ slides: [{ id: 'slide-one', objects: [object] }] })
const picture = () => ({ id: 'image-one', kind: 'image', imageId: 'asset-one', x: 10, y: 1.5, w: 2, h: 2 })
const chart = (type = 'bar') => ({ id: `chart-${type}`, kind: 'chart', chartType: type, x: 1, y: 2, w: 8, h: 4, title: '季度<&对比', categories: ['一季度', '=1+1'], series: [{ name: '收入<&', values: [10, 20] }] })
const images = () => new Map<string, DocumentDocxImage>([['asset-one', { type: 'png', data: PNG }]])
const reject = async (input: unknown, options?: Parameters<typeof buildPresentationPptxBytes>[1]) => {
  await expect(buildPresentationPptxBytes(input, options).then(() => 'unexpected-success')).rejects.toThrow(/^presentation-generation-invalid-input$/)
}

describe('presentation-generation-codec', () => {
  it('creates widescreen OOXML with stable names, Chinese text, shapes, charts, workbooks and image bytes', async () => {
    vi.clearAllMocks()
    const bytes = await buildPresentationPptxBytes({ slides: [
      { id: 'slide-one', background: 'FAFAFA', objects: [text(), { id: 'shape-one', kind: 'shape', shape: 'rect', x: 0.5, y: 1.7, w: 2, h: 1, fill: '00AAFF', lineWidth: 0 }, { id: 'line-one', kind: 'shape', shape: 'line', x: 0.5, y: 7, w: 12, h: 0 }, chart(), picture()] },
      { id: 'slide-two', objects: [chart('line'), { id: 'shape-two', kind: 'shape', shape: 'ellipse', x: 10, y: 1, w: 2, h: 1 }] },
      { id: 'slide-three', objects: [chart('pie')] }
    ] }, { title: '汇报<&文档', images: images() })
    expect(Buffer.isBuffer(bytes)).toBe(true)
    expect(fs.open).not.toHaveBeenCalled()
    expect(fs.readFile).not.toHaveBeenCalled()
    expect(fs.realpath).not.toHaveBeenCalled()
    expect(path.resolve).not.toHaveBeenCalled()
    const zip = await JSZip.loadAsync(bytes)
    const slide = await zip.file('ppt/slides/slide1.xml')!.async('string')
    expect(slide).toContain('中文&lt;&amp;报告 😀')
    expect(slide).toContain('Noto Sans CJK SC')
    for (const [index, id] of ['slide-one', 'slide-two', 'slide-three'].entries()) {
      expect(await zip.file(`ppt/slides/slide${index + 1}.xml`)!.async('string')).toContain(`<p:cSld name="${id}">`)
    }
    for (const id of ['text-one', 'shape-one', 'line-one', 'chart-bar', 'image-one']) expect(slide).toMatch(new RegExp(`<p:cNvPr[^>]*name="${id}"`))
    expect(await zip.file('ppt/presentation.xml')!.async('string')).toMatch(/<p:sldSz cx="12192000" cy="6858000"/)
    expect(await zip.file('docProps/core.xml')!.async('string')).toContain('汇报&lt;&amp;文档')
    const charts = Object.keys(zip.files).filter((name) => /^ppt\/charts\/chart\d+\.xml$/.test(name))
    const workbooks = Object.keys(zip.files).filter((name) => /^ppt\/embeddings\/.*\.xlsx$/.test(name))
    expect(charts).toHaveLength(3)
    expect(workbooks).toHaveLength(3)
    for (const name of charts) expect(await zip.file(name)!.async('string')).toContain('季度&lt;&amp;对比')
    const workbook = await JSZip.loadAsync(await zip.file(workbooks[0]!)!.async('nodebuffer'))
    expect(await workbook.file('xl/sharedStrings.xml')!.async('string')).toContain('收入&lt;&amp;')
    expect(await workbook.file('xl/sharedStrings.xml')!.async('string')).toContain('=1+1')
    expect(await workbook.file('xl/worksheets/sheet1.xml')!.async('string')).not.toContain('<f>')
    const media = Object.keys(zip.files).filter((name) => /^ppt\/media\/.*\.png$/.test(name))
    expect(media).toHaveLength(1)
    expect(await zip.file(media[0]!)!.async('nodebuffer')).toEqual(PNG)
    for (const [name, part] of Object.entries(zip.files)) if (name.endsWith('.rels')) expect(await part.async('string')).not.toContain('TargetMode="External"')
  })

  it.each([
    null, {}, { slides: [] }, { ...spec(), url: 'https://invalid.example' },
    { slides: [{ id: 'slide-one', objects: [] }] },
    spec({ ...text(), url: 'https://invalid.example' }),
    spec({ ...text(), kind: 'media' }), spec({ ...text(), id: 'unsafe"name' }),
    spec({ ...text(), id: 'a'.repeat(65) }), spec({ ...text(), id: 'slide-one' }),
    spec({ ...text(), x: -1 }), spec({ ...text(), y: Infinity }), spec({ ...text(), w: 0 }),
    spec({ ...text(), w: NaN }), spec({ ...text(), x: 2 }), spec({ ...text(), h: 8 }),
    spec({ ...text(), fontSize: 9 }), spec({ ...text(), fontSize: 61 }), spec({ ...text(), fontSize: '20' }),
    spec({ ...text(), bold: 1 }), spec({ ...text(), color: '#ABCDEF' }), spec({ ...text(), align: 'justify' }),
    spec({ ...text(), text: 'x'.repeat(6001) }), spec({ ...text(), text: '\u0000private-sentinel' }), spec({ ...text(), text: '\ud800' }),
    spec({ ...text(), text: 'ok', imageId: 'wrong-kind' }),
    spec({ id: 'line-one', kind: 'shape', shape: 'line', x: 1, y: 1, w: 0, h: 0 }),
    spec({ id: 'shape-one', kind: 'shape', shape: 'rect', x: 1, y: 1, w: 1, h: 1, lineWidth: 21 }),
    spec({ ...chart(), chartType: 'scatter' }), spec({ ...chart(), categories: [] }),
    spec({ ...chart(), series: [{ name: 'Series', values: [1] }] }),
    spec({ ...chart(), series: [{ name: 'Series', values: [1, Infinity] }] }),
    spec({ ...chart(), series: [{ name: 'Series', values: [1, 2], path: '/private/file' }] }),
    spec({ ...chart(), categories: Array.from({ length: 101 }, () => 'x') }),
    spec({ ...chart(), series: Array.from({ length: 9 }, () => ({ name: 'S', values: [1, 2] })) }),
    spec({ ...chart(), title: 'x'.repeat(257) }),
    spec({ ...picture(), imageId: '../private.png' }), spec({ ...picture(), imageId: 'https://invalid.example/a.png' }),
    spec({ ...picture(), imageId: 'data:image/png;base64,AAA' }), spec(picture()),
    { slides: [{ id: 'slide-one', objects: [text(), text()] }] },
    { slides: Array.from({ length: 51 }, (_, index) => ({ id: `slide-${index}`, objects: [{ ...text(), id: `text-${index}` }] })) }
  ])('rejects invalid structured input without echo (%#)', async (input) => { await reject(input) })

  it('enforces page, deck and total text budgets including chart labels and metadata', async () => {
    await reject({ slides: [{ id: 'slide-one', objects: Array.from({ length: 101 }, (_, index) => ({ ...text(), id: `text-${index}` })) }] })
    await reject({ slides: Array.from({ length: 11 }, (_, slide) => ({ id: `slide-${slide}`, objects: Array.from({ length: 100 }, (_, index) => ({ ...text(), id: `text-${slide}-${index}`, text: '' })) })) })
    await reject({ slides: [{ id: 'slide-one', objects: Array.from({ length: 17 }, (_, index) => ({ ...text(), id: `text-${index}`, text: 'x'.repeat(6000) })) }] })
    await reject(spec({ ...chart(), categories: Array.from({ length: 20 }, () => 'x'.repeat(6000)), series: [{ name: 'S', values: Array.from({ length: 20 }, () => 1) }] }))
    await reject(spec(), { title: 'x'.repeat(257) })
  })

  it('rejects accessors, iterators, proxies, custom prototypes and sparse arrays without executing code', async () => {
    const getter = vi.fn(() => 'private-sentinel')
    const iterator = vi.fn(() => [text()][Symbol.iterator]())
    const trapped = vi.fn(() => Object.prototype)
    await reject(spec(Object.defineProperty(text(), 'text', { get: getter })))
    await reject({ slides: Object.defineProperty([spec().slides[0]], Symbol.iterator, { value: iterator }) })
    await reject({ slides: Object.defineProperty([spec().slides[0]], '0', { get: getter }) })
    await reject({ slides: new Array(1) })
    await reject(Object.create(spec()))
    await reject({ slides: Object.setPrototypeOf([spec().slides[0]], null) })
    await reject(new Proxy(spec(), { getPrototypeOf: trapped }))
    expect(getter).not.toHaveBeenCalled()
    expect(iterator).not.toHaveBeenCalled()
    expect(trapped).not.toHaveBeenCalled()
  })

  it('rejects invalid options and image container accessors without executing them', async () => {
    const getter = vi.fn(() => images())
    await reject(spec(), null as unknown as Parameters<typeof buildPresentationPptxBytes>[1])
    await reject(spec(), { title: undefined })
    await reject(spec(), { images: undefined })
    await reject(spec(), Object.defineProperty({}, 'images', { get: getter }))
    await reject(spec(), { images: Object.defineProperty(images(), Symbol.iterator, { value: getter }) })
    await reject(spec(picture()), { images: new Map([['asset-one', Object.defineProperty({ type: 'png' }, 'data', { get: getter })]]) as Map<string, DocumentDocxImage> })
    expect(getter).not.toHaveBeenCalled()
  })

  it('rejects untrusted image fields and real truncated bytes, and snapshots supplied bytes', async () => {
    const wrong = new Map([['asset-one', { type: 'png', data: PNG.subarray(0, 24) }]])
    await reject(spec(picture()), { images: wrong as Map<string, DocumentDocxImage> })
    await reject(spec(picture()), { images: new Map([['asset-one', { type: 'png', data: PNG, path: '/private/sentinel.png' }]]) as Map<string, DocumentDocxImage> })
    const original = Buffer.from(PNG)
    const pending = buildPresentationPptxBytes(spec(picture()), { images: new Map([['asset-one', { type: 'png', data: original }]]) })
    original.fill(0)
    const zip = await JSZip.loadAsync(await pending)
    const media = Object.keys(zip.files).find((name) => name.endsWith('.png'))!
    expect(await zip.file(media)!.async('nodebuffer')).toEqual(PNG)
  })

  it('bounds embedded bytes by image references as well as the supplied image map', async () => {
    const width = 1536, height = 1536
    const data = Buffer.alloc(54 + width * height * 3, 255)
    data.fill(0, 0, 54)
    data.write('BM')
    data.writeUInt32LE(data.length, 2)
    data.writeUInt32LE(54, 10)
    data.writeUInt32LE(40, 14)
    data.writeInt32LE(width, 18)
    data.writeInt32LE(height, 22)
    data.writeUInt16LE(1, 26)
    data.writeUInt16LE(24, 28)
    data.writeUInt32LE(width * height * 3, 34)
    const supplied = new Map<string, DocumentDocxImage>([['asset-one', { type: 'bmp', data }]])
    // This is one valid image below 8 MiB; four embeds exceed 24 MiB.
    expect((await validateDocumentImageBytes(supplied)).get('asset-one')).toMatchObject({ width, height })
    await reject({ slides: [{ id: 'slide-one', objects: Array.from({ length: 4 }, (_, index) => ({ ...picture(), id: `image-${index}` })) }] }, { images: supplied })
  })
})
