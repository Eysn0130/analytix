import { copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { clearWritePdfTextCache, readWritePdfText, readWritePdfBytes } from './write-pdf-text-service'

vi.mock('node:fs/promises', async original => {
  const actual = await original<typeof import('node:fs/promises')>()
  return { ...actual, readFile: vi.fn(actual.readFile) }
})

function escapePdfText(text: string): string {
  return text.replaceAll('\\', '\\\\').replaceAll('(', '\\(').replaceAll(')', '\\)')
}

function createSimpleTextPdf(text: string): Buffer {
  const stream = `BT /F1 18 Tf 72 720 Td (${escapePdfText(text)}) Tj ET`
  const objects = [
    '1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n',
    '2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n',
    [
      '3 0 obj',
      '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]',
      '/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
      'endobj\n'
    ].join('\n'),
    '4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n',
    `5 0 obj\n<< /Length ${Buffer.byteLength(stream, 'ascii')} >>\nstream\n${stream}\nendstream\nendobj\n`
  ]
  let pdf = '%PDF-1.4\n'
  const offsets = [0]
  for (const object of objects) {
    offsets.push(Buffer.byteLength(pdf, 'ascii'))
    pdf += object
  }
  const xrefOffset = Buffer.byteLength(pdf, 'ascii')
  pdf += 'xref\n0 6\n'
  pdf += '0000000000 65535 f \n'
  for (const offset of offsets.slice(1)) {
    pdf += `${String(offset).padStart(10, '0')} 00000 n \n`
  }
  pdf += `trailer\n<< /Root 1 0 R /Size 6 >>\nstartxref\n${xrefOffset}\n%%EOF\n`
  return Buffer.from(pdf, 'ascii')
}

afterEach(() => {
  clearWritePdfTextCache()
})

describe('write PDF text service', () => {
  it('parses authorized bytes without reopening a file and refuses a revoked owner', async () => {
    vi.mocked(readFile).mockClear()
    const bytes = createSimpleTextPdf('Authorized PDF context')
    const result = await readWritePdfBytes(bytes, () => true)
    expect(result.ok).toBe(true)
    if (result.ok) expect(result.pages[0].text).toContain('Authorized PDF context')
    expect(await readWritePdfBytes(bytes, () => false)).toMatchObject({ ok: false })
    expect(readFile).not.toHaveBeenCalled()
  })

  it('extracts page text from a text-layer PDF fixture', async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-write-pdf-text-'))
    const pdfPath = join(workspaceRoot, 'papers', 'fixture.pdf')
    await mkdir(join(workspaceRoot, 'papers'), { recursive: true })
    await writeFile(pdfPath, createSimpleTextPdf('PDF BM25 keyword retrieval context'))

    const result = await readWritePdfText({
      workspaceRoot,
      path: 'papers/fixture.pdf'
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.pageCount).toBe(1)
    expect(result.hasText).toBe(true)
    expect(result.pages[0]).toMatchObject({
      page: 1,
      charStart: 0
    })
    expect(result.pages[0].text).toContain('PDF BM25 keyword retrieval context')
  })

  it('extracts real PDF text when the optional Node raster package cannot resolve', async () => {
    const root = await mkdtemp(join(tmpdir(), 'analytix-pdf-no-raster-'))
    try {
      const pdfPackage = join(root, 'node_modules', 'pdfjs-dist')
      await mkdir(join(pdfPackage, 'legacy', 'build'), { recursive: true })
      await copyFile(join(process.cwd(), 'node_modules/pdfjs-dist/package.json'),
        join(pdfPackage, 'package.json'))
      for (const name of ['pdf.mjs', 'pdf.worker.mjs']) {
        await copyFile(join(process.cwd(), 'node_modules/pdfjs-dist/legacy/build', name),
          join(pdfPackage, 'legacy/build', name))
      }
      await writeFile(join(root, 'fixture.pdf'), createSimpleTextPdf('PDF text without Node raster'))
      const script = join(root, 'read.mjs')
      await writeFile(script, `
      import { createRequire } from 'node:module'
      import { readFileSync } from 'node:fs'
      const require = createRequire(import.meta.url)
      let nativeResolvable = false
      try { require.resolve('@napi-rs/canvas'); nativeResolvable = true } catch {}
      globalThis.DOMMatrix = class DOMMatrix {}
      globalThis.ImageData = class ImageData {}
      globalThis.Path2D = class Path2D {}
      const pdfjs = await import('pdfjs-dist/legacy/build/pdf.mjs')
      const task = pdfjs.getDocument({ data: new Uint8Array(readFileSync(new URL('./fixture.pdf', import.meta.url))),
        disableFontFace: true, disableWorker: true, isEvalSupported: false, useSystemFonts: false })
      const document = await task.promise
      const page = await document.getPage(1)
      const text = (await page.getTextContent()).items.map(item => item.str || '').join(' ')
      await document.destroy()
      process.stdout.write(JSON.stringify({ nativeResolvable, text }))
      `)
      const child = spawnSync(process.execPath, [script], {
        cwd: root, env: { PATH: process.env.PATH || '' }, encoding: 'utf8',
        timeout: 20_000, maxBuffer: 4096
      })
      expect(child.status, child.stderr).toBe(0)
      expect(JSON.parse(child.stdout)).toEqual({
        nativeResolvable: false, text: 'PDF text without Node raster'
      })
    } finally {
      await rm(root, { recursive: true, force: true })
    }
  })
})
