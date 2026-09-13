import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import JSZip from 'jszip'

vi.mock('electron', () => ({
  BrowserWindow: vi.fn(class BrowserWindow {}),
  clipboard: { write: vi.fn() },
  dialog: { showSaveDialog: vi.fn() }
}))

import { BrowserWindow, clipboard, dialog } from 'electron'
import { copyWriteDocumentAsRichText, exportWriteDocument } from './write-export-service'

// Valid one-pixel PNG with a CRC-bound tEXt chunk containing only synthetic
// identifiers. Text/XML-only scanning misses these bytes inside media payloads;
// this fixture does not claim to exercise OCR or pixel-content classification.
const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAA8dEVYdERlc2NyaXB0aW9uAFNZTlRIRVRJQ19JTUFHRV9QUklWQUNZX0NBTkFSWSBwaG9uZToxMzgwMDEzODAwMH+tRIoAAAAASUVORK5CYII=', 'base64')
const unavailable = 'Write export cannot include images because image content privacy checks are unavailable.'
const syntaxes = {
  local: '![Cover](./cover.png)',
  reference: '![Cover][cover]\n\n[cover]: ./cover.png',
  remote: '![Cover](https://example.test/cover.png)',
  data: `![Cover](data:image/png;base64,${image.toString('base64')})`
}

describe('ordinary Write image publication boundary', () => {
  let workspaceRoot = ''
  let sourcePath = ''

  beforeEach(async () => {
    workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-image-publication-'))
    sourcePath = join(workspaceRoot, 'draft.md')
    await writeFile(sourcePath, '# Draft')
    await writeFile(join(workspaceRoot, 'cover.png'), image)
    vi.mocked(BrowserWindow).mockClear()
    vi.mocked(clipboard.write).mockReset()
    vi.mocked(dialog.showSaveDialog).mockReset()
  })

  afterEach(async () => {
    if (workspaceRoot) await rm(workspaceRoot, { recursive: true, force: true })
  })

  for (const format of ['html', 'doc', 'docx', 'pdf'] as const) {
    it.each(Object.entries(syntaxes))(`refuses %s images before ${format} publication`, async (_, content) => {
      const targetPath = join(workspaceRoot, `existing.${format}`)
      const original = Buffer.from('EXISTING_EXPORT_MUST_REMAIN_UNCHANGED')
      await writeFile(targetPath, original)
      vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

      const result = await exportWriteDocument({ path: sourcePath, format, content }, { workspaceRoot })
      const bytes = await readFile(targetPath)
      let imageCanaryEscaped = bytes.includes(image.toString('base64'))
      if (result.ok && format === 'docx') {
        const zip = await JSZip.loadAsync(bytes)
        for (const entry of Object.values(zip.files)) {
          if (entry.name.startsWith('word/media/') && !entry.dir && (await entry.async('nodebuffer')).equals(image)) imageCanaryEscaped = true
        }
      }
      // Bounded booleans expose a failed media boundary without dumping the
      // generated document, fonts, paths or binary payload into test logs.
      expect({ imageCanaryEscaped, targetUnchanged: bytes.equals(original), pdfWindowCreated: vi.mocked(BrowserWindow).mock.calls.length > 0 })
        .toEqual({ imageCanaryEscaped: false, targetUnchanged: true, pdfWindowCreated: false })
      expect(result).toEqual({ ok: false, canceled: false, message: unavailable })
      expect(clipboard.write).not.toHaveBeenCalled()
    })
  }

  it.each(Object.entries(syntaxes))('refuses %s images before clipboard publication', async (_, content) => {
    const result = await copyWriteDocumentAsRichText({ path: sourcePath, content }, { workspaceRoot })
    expect({ copied: vi.mocked(clipboard.write).mock.calls.length > 0 }).toEqual({ copied: false })
    expect(result).toEqual({ ok: false, message: unavailable })
    expect(BrowserWindow).not.toHaveBeenCalled()
  })

  it('keeps image syntax inside code as masked text', async () => {
    const content = '# Example\n\n```markdown\n![Example](./cover.png)\n```\n\n金额 123.45 元\n\n电话：13800138000'
    const result = await copyWriteDocumentAsRichText({ path: sourcePath, content }, { workspaceRoot })
    expect(result.ok).toBe(true)
    const publication = vi.mocked(clipboard.write).mock.calls[0][0]
    expect(publication.html).toContain('![Example](./cover.png)')
    expect(publication.html).toContain('123.45')
    expect(publication.html).not.toContain('<img')
    expect(JSON.stringify(publication)).not.toContain('13800138000')
  })
})
