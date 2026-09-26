import { mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
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
})
