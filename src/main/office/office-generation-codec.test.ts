import { describe, expect, it, vi } from 'vitest'
import JSZip from 'jszip'
import { defaultFormats } from 'jimp'
import * as fs from 'node:fs/promises'
import { encodeOfficeGenerationV1 } from './office-generation-codec'
import { inspectWriteDocxPackageV1 } from '../services/write-docx-service'

vi.mock('node:fs/promises', { spy: true })

const PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='
const image = { id: 'image-1', type: 'png', dataBase64: PNG }
const input = { schemaVersion: 1, kind: 'docx', markdown: '# 中文标题\n\n| 项目 | 金额 |\n| --- | --- |\n| 收入 | 100 |\n\n![示意图](image-1)', title: '中文文档', images: [image] }

describe('office-generation-codec', () => {
  it('produces a real inspected package with Chinese, a table and the supplied image bytes', async () => {
    const buffer = await encodeOfficeGenerationV1(input)
    const inspection = await inspectWriteDocxPackageV1(buffer)
    expect(inspection.documentXml).toContain('中文标题')
    expect(inspection.documentXml).toContain('<w:tbl>')
    expect(inspection.documentXml).toContain('收入')
    expect(inspection.mediaParts).toHaveLength(1)
    expect(inspection.externalRelationships).toEqual([])
    const zip = await JSZip.loadAsync(buffer)
    expect(await zip.file('docProps/core.xml')!.async('string')).toContain('中文文档')
    expect(await zip.file(inspection.mediaParts[0]!)!.async('nodebuffer')).toEqual(Buffer.from(PNG, 'base64'))
    expect(fs.open).not.toHaveBeenCalled()
    expect(fs.realpath).not.toHaveBeenCalled()
  })

  it.each([['png', 'image/png'], ['jpg', 'image/jpeg'], ['gif', 'image/gif'], ['bmp', 'image/bmp']])('embeds a genuinely decodable %s image without changing its bytes', async (type, mime) => {
    const format = defaultFormats.map((factory) => factory()).find((candidate) => candidate.mime === mime)!
    const bytes = await format.encode({ width: 2, height: 1, data: Buffer.from([255, 0, 0, 255, 0, 255, 0, 255]) })
    const generated = await encodeOfficeGenerationV1({ ...input, images: [{ ...image, type, dataBase64: Buffer.from(bytes).toString('base64') }] })
    const inspection = await inspectWriteDocxPackageV1(generated)
    const zip = await JSZip.loadAsync(generated)
    expect(inspection.mediaParts).toHaveLength(1)
    expect(await zip.file(inspection.mediaParts[0]!)!.async('nodebuffer')).toEqual(Buffer.from(bytes))
  })

  it.each([
    null, [], {},
    { ...input, schemaVersion: 2 },
    { ...input, kind: 'pdf' },
    { ...input, markdown: 1 },
    { ...input, markdown: '中'.repeat(Math.floor(1024 * 1024 / 3) + 1) },
    { ...input, title: 'a'.repeat(257) },
    { ...input, title: null },
    { ...input, sourcePath: '/private/codec-sentinel.md' },
    { ...input, projectProse: 'arbitrary source' },
    { ...input, images: null },
    { ...input, images: Array.from({ length: 25 }, (_, index) => ({ ...image, id: `image-${index}` })) },
    { ...input, images: [image, image] },
    { ...input, images: [{ ...image, id: 'a'.repeat(65) }] },
    { ...input, images: [{ ...image, id: '../image-1' }] },
    { ...input, images: [{ ...image, path: '/private/codec-sentinel.png' }] },
    { ...input, images: [{ ...image, type: 'svg' }] },
    { ...input, images: [{ ...image, type: 'jpg' }] },
    { ...input, images: [{ ...image, dataBase64: '' }] },
    { ...input, images: [{ ...image, dataBase64: PNG + '\n' }] },
    { ...input, images: [{ ...image, dataBase64: PNG.replace(/=$/, '') }] },
    { ...input, images: [{ ...image, dataBase64: 'data:image/png;base64,' + PNG }] },
    { ...input, images: [{ ...image, dataBase64: 'A'.repeat(Math.ceil((8 * 1024 * 1024) / 3) * 4 + 4) }] },
    { ...input, markdown: '<think>PRIVATE_CODEC_SENTINEL</think>' }
  ])('rejects invalid data without echoing it (%#)', async (candidate) => {
    await expect(encodeOfficeGenerationV1(candidate)).rejects.toThrow(/^office-generation-invalid-input$/)
  })

  it.each(['../image.png', '/private/codec-sentinel.png', 'https://example.com/image.png', 'file:///private/image.png', 'data:image/png;base64,' + PNG, 'missing-image', 'image%2D1'])('rejects non-supplied image targets (%#)', async (target) => {
    await expect(encodeOfficeGenerationV1({ ...input, markdown: `![image](${target})` })).rejects.toThrow(/^office-generation-invalid-input$/)
  })

  it('validates images even inside links and rejects indirect image references', async () => {
    for (const markdown of ['[![image](../hidden.png)](https://example.com)', '![image][ref]\n\n[ref]: ../hidden.png']) {
      await expect(encodeOfficeGenerationV1({ ...input, markdown })).rejects.toThrow(/^office-generation-invalid-input$/)
    }
  })

  it('rejects accessors without evaluating supplied code', async () => {
    const getter = vi.fn(() => 'private-codec-sentinel')
    const candidate = Object.defineProperty({ ...input }, 'markdown', { get: getter })
    await expect(encodeOfficeGenerationV1(candidate)).rejects.toThrow(/^office-generation-invalid-input$/)
    expect(getter).not.toHaveBeenCalled()
  })

  it('rejects array accessors and iterators without executing them', async () => {
    const getter = vi.fn(() => image)
    const iterator = vi.fn(() => [image][Symbol.iterator]())
    const accessorArray = Object.defineProperty([image], '0', { get: getter })
    const iteratorArray = Object.defineProperty([image], Symbol.iterator, { value: iterator })
    for (const images of [accessorArray, iteratorArray, new Array(1), Object.setPrototypeOf([image], null)]) {
      await expect(encodeOfficeGenerationV1({ ...input, images }).then(() => 'unexpected-success')).rejects.toThrow(/^office-generation-invalid-input$/)
    }
    expect(getter).not.toHaveBeenCalled()
    expect(iterator).not.toHaveBeenCalled()
  })

  it('rejects truncated image headers that declare valid dimensions', async () => {
    const bmp = Buffer.alloc(26)
    bmp.write('BM')
    bmp.writeUInt32LE(12, 14)
    bmp.writeUInt16LE(1, 18)
    bmp.writeUInt16LE(1, 20)
    const truncated = [
      { type: 'png', data: Buffer.from(PNG, 'base64').subarray(0, 24) },
      { type: 'jpg', data: Buffer.from('ffd8ffc00008080001000100', 'hex') },
      { type: 'gif', data: Buffer.from('47494638396101000100', 'hex') },
      { type: 'bmp', data: bmp }
    ]
    for (const { type, data } of truncated) {
      await expect(encodeOfficeGenerationV1({ ...input, images: [{ ...image, type, dataBase64: data.toString('base64') }] }).then(() => 'unexpected-success')).rejects.toThrow(/^office-generation-invalid-input$/)
    }
  })

  it('rejects image dimension and aggregate byte limits', async () => {
    const oversized = Buffer.from(PNG, 'base64')
    oversized.writeUInt32BE(10001, 16)
    await expect(encodeOfficeGenerationV1({ ...input, images: [{ ...image, dataBase64: oversized.toString('base64') }] })).rejects.toThrow(/^office-generation-invalid-input$/)
    const large = Buffer.alloc(8 * 1024 * 1024)
    Buffer.from(PNG, 'base64').copy(large)
    const images = Array.from({ length: 4 }, (_, index) => ({ ...image, id: `image-${index}`, dataBase64: large.toString('base64') }))
    await expect(encodeOfficeGenerationV1({ ...input, images })).rejects.toThrow(/^office-generation-invalid-input$/)
  })
})
