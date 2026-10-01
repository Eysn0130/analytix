import { readFile, stat } from 'node:fs/promises'
import { extname } from 'node:path'
import type { WorkspaceFileTarget } from '../../shared/workspace-file'
import { resolveOpenTargetPath } from './workspace-paths'

const MAX_PDF_TEXT_BYTES = 64 * 1024 * 1024
const MAX_PDF_TEXT_PAGES = 300
const MAX_PDF_TEXT_CHARS = 1_000_000

export type WritePdfTextPage = {
  page: number
  text: string
  charStart: number
  charEnd: number
}

export type WritePdfTextResult =
  | {
      ok: true
      path: string
      size: number
      mtimeMs: number
      pageCount: number
      pages: WritePdfTextPage[]
      hasText: boolean
      truncated: boolean
    }
  | {
      ok: false
      message: string
    }

type PdfJsModule = typeof import('pdfjs-dist/legacy/build/pdf.mjs')

const pdfTextCache = new Map<string, Promise<WritePdfTextResult>>()
let pdfJsModulePromise: Promise<PdfJsModule> | null = null

function ensurePdfJsNodePolyfills(): void {
  const target = globalThis as unknown as Record<string, unknown>
  target.DOMMatrix ??= class DOMMatrix {}
  target.ImageData ??= class ImageData {}
  target.Path2D ??= class Path2D {}
}

async function loadPdfJs(): Promise<PdfJsModule> {
  if (!pdfJsModulePromise) {
    ensurePdfJsNodePolyfills()
    pdfJsModulePromise = import('pdfjs-dist/legacy/build/pdf.mjs')
  }
  return pdfJsModulePromise
}

function compactPdfText(text = ''): string {
  return String(text || '')
    .replace(/\r\n?/g, '\n')
    .replace(/[ \t\f\v]+/g, ' ')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

function textContentToPageText(content: { items?: unknown[] }): string {
  const parts: string[] = []
  for (const item of content.items ?? []) {
    if (!item || typeof item !== 'object') continue
    const value = (item as { str?: unknown }).str
    if (typeof value === 'string' && value.trim()) parts.push(value)
  }
  return compactPdfText(parts.join(' '))
}

async function extractPdfText(
  targetPath: string,
  size: number,
  mtimeMs: number,
  authorizedBytes?: Uint8Array,
  isCurrent: () => boolean = () => true
): Promise<WritePdfTextResult> {
  const pdfjs = await loadPdfJs()
  if (!isCurrent()) return { ok: false, message: 'PDF read is unavailable.' }
  const bytes = authorizedBytes ?? await readFile(targetPath)
  if (!isCurrent()) return { ok: false, message: 'PDF read is unavailable.' }
  const loadingTask = pdfjs.getDocument({
    data: new Uint8Array(bytes),
    disableFontFace: true,
    disableWorker: true,
    isEvalSupported: false,
    useSystemFonts: false
  } as unknown)
  const document = await loadingTask.promise
  const pageCount = document.numPages
  const pages: WritePdfTextPage[] = []
  let charOffset = 0
  let truncated = false

  try {
    const maxPages = Math.min(pageCount, MAX_PDF_TEXT_PAGES)
    for (let pageNumber = 1; pageNumber <= maxPages; pageNumber += 1) {
      if (!isCurrent()) return { ok: false, message: 'PDF read is unavailable.' }
      const page = await document.getPage(pageNumber)
      const content = await page.getTextContent()
      if (!isCurrent()) return { ok: false, message: 'PDF read is unavailable.' }
      const text = textContentToPageText(content)
      if (text) {
        const remaining = MAX_PDF_TEXT_CHARS - charOffset
        const pageText = text.length > remaining ? text.slice(0, Math.max(0, remaining)).trim() : text
        if (pageText) {
          pages.push({
            page: pageNumber,
            text: pageText,
            charStart: charOffset,
            charEnd: charOffset + pageText.length
          })
          charOffset += pageText.length + 1
        }
        if (text.length > remaining || charOffset >= MAX_PDF_TEXT_CHARS) {
          truncated = true
          break
        }
      }
      page.cleanup()
    }
    if (pageCount > MAX_PDF_TEXT_PAGES) truncated = true
  } finally {
    await document.destroy()
  }

  return {
    ok: true,
    path: targetPath,
    size,
    mtimeMs,
    pageCount,
    pages,
    hasText: pages.some((page) => page.text.trim().length > 0),
    truncated
  }
}

// Retrieval parses Core-authorized bytes; this path never reopens a filename.
export async function readWritePdfBytes(bytes: Uint8Array, isCurrent: () => boolean): Promise<WritePdfTextResult> {
  if (!isCurrent() || bytes.byteLength > 16 * 1024 * 1024) return { ok: false, message: 'PDF read is unavailable.' }
  try { return await extractPdfText('', bytes.byteLength, 0, bytes, isCurrent) }
  catch { return { ok: false, message: 'PDF read is unavailable.' } }
}

export async function readWritePdfText(payload: WorkspaceFileTarget): Promise<WritePdfTextResult> {
  try {
    return readLocalPdfTextByPath(
      await resolveOpenTargetPath(payload.path, payload.workspaceRoot),
      'This PDF is too large to parse in Write mode.'
    )
  } catch (error) {
    return {
      ok: false,
      message: error instanceof Error ? error.message : String(error)
    }
  }
}

export async function readLocalPdfText(payload: { path: string }): Promise<WritePdfTextResult> {
  try {
    return readLocalPdfTextByPath(
      await resolveOpenTargetPath(payload.path),
      'This PDF is too large to attach.'
    )
  } catch (error) {
    return {
      ok: false,
      message: error instanceof Error ? error.message : String(error)
    }
  }
}

async function readLocalPdfTextByPath(
  targetPath: string,
  tooLargeMessage: string
): Promise<WritePdfTextResult> {
  try {
    const fileInfo = await stat(targetPath)
    if (fileInfo.isDirectory()) return { ok: false, message: 'Cannot read text from a directory.' }
    if (fileInfo.size > MAX_PDF_TEXT_BYTES) {
      return { ok: false, message: tooLargeMessage }
    }
    if (extname(targetPath).toLowerCase() !== '.pdf') {
      return { ok: false, message: 'This file is not a PDF document.' }
    }

    const cacheKey = `${targetPath}:${fileInfo.size}:${fileInfo.mtimeMs}`
    const cached = pdfTextCache.get(cacheKey)
    if (cached) return cached

    const pending = extractPdfText(targetPath, fileInfo.size, fileInfo.mtimeMs).finally(() => {
      if (pdfTextCache.size > 32) {
        const oldest = pdfTextCache.keys().next().value
        if (oldest) pdfTextCache.delete(oldest)
      }
    })
    pdfTextCache.set(cacheKey, pending)
    return pending
  } catch (error) {
    return {
      ok: false,
      message: error instanceof Error ? error.message : String(error)
    }
  }
}

export function clearWritePdfTextCache(): void {
  pdfTextCache.clear()
}
