import { BrowserWindow, clipboard, dialog } from 'electron'
import { createRequire } from 'node:module'
import { existsSync } from 'node:fs'
import { mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createElement, type ComponentPropsWithoutRef, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  normalizeWriteTypography,
  writeFontStackFor
} from '../../shared/app-settings-write'
import type { WriteTypographySettingsV1 } from '../../shared/app-settings-types'
import { isWriteOfficialDocumentTypography } from '../../shared/write-official-document'
import { assertOrdinaryWriteExportContentV1, projectOrdinaryWriteExportContentV1, projectWriteExportMarkdownTreeV1, WriteExportPublicError } from './write-export-content'
export { projectOrdinaryWriteExportContentV1 } from './write-export-content'
import type {
  WriteExportFormat,
  WriteExportPayload,
  WriteExportResult,
  WriteRichClipboardPayload,
  WriteRichClipboardResult
} from '../../shared/write-export'
import { resolveWriteMarkdownResource } from '../../shared/write-markdown-resource'
import { applyWriteMarkdownAlignmentDirectivesToTree } from '../../shared/write-text-align'
import { resolveWorkspaceFile } from './workspace-service'
import { buildWriteDocxDocument } from './write-docx-service'

const require = createRequire(import.meta.url)

function applyWriteMarkdownAlignmentDirectives() {
  return (tree: Parameters<typeof applyWriteMarkdownAlignmentDirectivesToTree>[0]): void => {
    applyWriteMarkdownAlignmentDirectivesToTree(tree)
  }
}

function applyWriteExportTextProjection() {
  return (tree: Parameters<typeof projectWriteExportMarkdownTreeV1>[0]) => projectWriteExportMarkdownTreeV1(tree)
}

function normalizeExportTypography(typography?: WriteExportPayload['typography']): WriteTypographySettingsV1 {
  return normalizeWriteTypography(typography as Partial<WriteTypographySettingsV1> | undefined)
}

async function officialExportFontFaceCss(embedFonts: boolean): Promise<string> {
  try {
    const regular = resolveOfficialFontFile('noto-serif-sc-chinese-simplified-400-normal.woff2')
    const bold = resolveOfficialFontFile('noto-serif-sc-chinese-simplified-700-normal.woff2')
    const regularSource = embedFonts
      ? `data:font/woff2;base64,${(await readFile(regular)).toString('base64')}`
      : pathToFileURL(regular).href
    const boldSource = embedFonts
      ? `data:font/woff2;base64,${(await readFile(bold)).toString('base64')}`
      : pathToFileURL(bold).href
    return `
  @font-face {
    font-family: "Noto Serif SC";
    font-style: normal;
    font-display: swap;
    font-weight: 400;
    src: url("${regularSource}") format("woff2");
  }

  @font-face {
    font-family: "Noto Serif SC";
    font-style: normal;
    font-display: swap;
    font-weight: 700;
    src: url("${boldSource}") format("woff2");
  }
`
  } catch {
    return ''
  }
}

function resolveOfficialFontFile(fileName: string): string {
  const resourcePath = process.resourcesPath
    ? join(process.resourcesPath, 'fonts', 'noto-serif-sc', fileName)
    : ''
  if (resourcePath && existsSync(resourcePath)) return resourcePath
  return require.resolve(`@fontsource/noto-serif-sc/files/${fileName}`)
}

async function buildWriteExportCss(
  typography?: WriteExportPayload['typography'],
  embedOfficialFonts = false
): Promise<string> {
  const normalized = normalizeExportTypography(typography)
  const fontFamily = writeFontStackFor(normalized.fontPreset, normalized.customFontFamily)
  return `${await officialExportFontFaceCss(embedOfficialFonts)}
  :root {
    --write-export-font-family: ${fontFamily};
    --write-export-font-size: ${normalized.fontSizePx}px;
    --write-export-line-height: ${normalized.lineHeight};
  }
${officialDocumentExportCss(normalized)}
${EXPORT_CSS}`
}

function officialDocumentExportCss(typography: WriteTypographySettingsV1): string {
  if (!isWriteOfficialDocumentTypography(typography)) return ''
  return `
  .markdown-body > h1:first-child,
  .markdown-body > h2:first-child {
    text-align: center;
    text-indent: 0;
  }

  .markdown-body > p {
    text-align: justify;
    text-align-last: auto;
    text-indent: 2em;
  }

  .markdown-body blockquote p,
  .markdown-body li p {
    text-align: inherit;
    text-indent: 0;
  }
`
}

const EXPORT_CSS = `
  :root {
    color-scheme: light;
  }

  * {
    box-sizing: border-box;
  }

  html {
    background: #ffffff;
  }

  body {
    margin: 0;
    background: #ffffff;
    color: #111827;
    font-family: var(--write-export-font-family);
    font-size: var(--write-export-font-size);
    line-height: var(--write-export-line-height);
  }

  a {
    color: #0f62fe;
    text-decoration: none;
  }

  a:hover {
    text-decoration: underline;
  }

  .document-shell {
    padding: 22mm 18mm;
  }

  .markdown-body {
    max-width: 100%;
  }

  .markdown-body > :first-child {
    margin-top: 0;
  }

  .markdown-body > :last-child {
    margin-bottom: 0;
  }

  .markdown-body [data-write-align="center"],
  .markdown-body .write-align-center {
    text-align: center;
  }

  .markdown-body [data-write-align="right"],
  .markdown-body .write-align-right {
    text-align: right;
  }

  .markdown-body [data-write-align="justify"],
  .markdown-body .write-align-justify {
    text-align: justify;
    text-align-last: auto;
  }

  .markdown-body p,
  .markdown-body ul,
  .markdown-body ol,
  .markdown-body blockquote,
  .markdown-body pre,
  .markdown-body table {
    margin: 0 0 1em;
  }

  .markdown-body h1,
  .markdown-body h2,
  .markdown-body h3,
  .markdown-body h4,
  .markdown-body h5,
  .markdown-body h6 {
    margin: 1.45em 0 0.65em;
    line-height: 1.24;
    color: #0f172a;
    font-weight: 700;
  }

  .markdown-body h1 {
    font-size: 2em;
    border-bottom: 1px solid #e5e7eb;
    padding-bottom: 0.3em;
  }

  .markdown-body h2 {
    font-size: 1.55em;
    border-bottom: 1px solid #edf2f7;
    padding-bottom: 0.24em;
  }

  .markdown-body h3 {
    font-size: 1.25em;
  }

  .markdown-body ul,
  .markdown-body ol {
    padding-left: 1.5em;
  }

  .markdown-body li + li {
    margin-top: 0.3em;
  }

  .markdown-body blockquote {
    padding: 0.3em 0 0.3em 1em;
    border-left: 4px solid #dbe4ff;
    color: #475569;
    background: #f8fbff;
  }

  .markdown-body code {
    font-family: "SFMono-Regular", "Menlo", "Consolas", "Liberation Mono", monospace;
    font-size: 0.92em;
  }

  .markdown-body p code,
  .markdown-body li code,
  .markdown-body td code {
    padding: 0.12em 0.38em;
    border-radius: 0.42em;
    background: #f1f5f9;
    color: #0f172a;
  }

  .markdown-body pre {
    overflow-x: auto;
    padding: 0.95em 1.05em;
    border-radius: 0.9em;
    background: #0f172a;
    color: #e2e8f0;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .markdown-body pre code {
    background: transparent;
    color: inherit;
    padding: 0;
  }

  .markdown-body hr {
    height: 1px;
    border: 0;
    background: #e5e7eb;
    margin: 1.6em 0;
  }

  .markdown-body table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.95em;
  }

  .markdown-body th,
  .markdown-body td {
    border: 1px solid #dbe3ee;
    padding: 0.5em 0.7em;
    vertical-align: top;
    text-align: left;
  }

  .markdown-body th {
    background: #f8fafc;
    font-weight: 700;
  }

  .markdown-body img {
    display: block;
    max-width: 100%;
    height: auto;
    margin: 1rem auto;
  }

  .plain-text {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: "SFMono-Regular", "Menlo", "Consolas", "Liberation Mono", monospace;
  }

  @page {
    size: A4;
    margin: 0;
  }
`

const LOCAL_IMAGE_PATTERN = /(<img\b[^>]*?\bsrc=")([^"]+)(")/gi
const LOCAL_IMAGE_UNAVAILABLE_MESSAGE = 'Write export local image is unavailable.'

function isMarkdownFile(filePath: string): boolean {
  return /\.(md|markdown|mdx)$/i.test(filePath)
}

function basenameWithoutExtension(filePath: string): string {
  const name = basename(filePath)
  const extension = extname(name)
  return extension ? name.slice(0, -extension.length) : name
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function exportExtension(format: WriteExportFormat): string {
  if (format === 'html') return '.html'
  if (format === 'pdf') return '.pdf'
  if (format === 'doc') return '.doc'
  return '.docx'
}

function exportDialogFilter(format: WriteExportFormat): Electron.FileFilter {
  if (format === 'html') return { name: 'HTML', extensions: ['html'] }
  if (format === 'pdf') return { name: 'PDF', extensions: ['pdf'] }
  if (format === 'doc') return { name: 'DOC', extensions: ['doc'] }
  return { name: 'DOCX', extensions: ['docx'] }
}

function mimeTypeForPath(filePath: string): string | null {
  const extension = extname(filePath).toLowerCase()
  if (extension === '.png') return 'image/png'
  if (extension === '.jpg' || extension === '.jpeg') return 'image/jpeg'
  if (extension === '.gif') return 'image/gif'
  if (extension === '.webp') return 'image/webp'
  if (extension === '.bmp') return 'image/bmp'
  if (extension === '.svg') return 'image/svg+xml'
  return null
}

function ensureExportExtension(targetPath: string, format: WriteExportFormat): string {
  const extension = exportExtension(format)
  return extname(targetPath).trim() ? targetPath : `${targetPath}${extension}`
}

function defaultExportPath(sourcePath: string, format: WriteExportFormat): string {
  return join(dirname(sourcePath), `${basenameWithoutExtension(sourcePath)}${exportExtension(format)}`)
}

function isWithinWorkspace(workspaceRoot: string, targetPath: string): boolean {
  const relativePath = relative(workspaceRoot, targetPath)
  return relativePath === '' || (
    relativePath !== '..' &&
    !relativePath.startsWith(`..${sep}`) &&
    !isAbsolute(relativePath)
  )
}

async function localFileUrlToDataUri(value: string, workspaceRoot: string): Promise<string | null> {
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    return null
  }
  if (parsed.protocol !== 'file:') return null

  try {
    const canonicalFilePath = await realpath(fileURLToPath(parsed))
    if (!isWithinWorkspace(workspaceRoot, canonicalFilePath)) {
      throw new WriteExportPublicError(LOCAL_IMAGE_UNAVAILABLE_MESSAGE)
    }
    const mimeType = mimeTypeForPath(canonicalFilePath)
    if (!mimeType) throw new WriteExportPublicError(LOCAL_IMAGE_UNAVAILABLE_MESSAGE)
    const buffer = await readFile(canonicalFilePath)
    return `data:${mimeType};base64,${buffer.toString('base64')}`
  } catch {
    throw new WriteExportPublicError(LOCAL_IMAGE_UNAVAILABLE_MESSAGE)
  }
}

export async function inlineLocalImagesInHtml(html: string, trustedWorkspaceRoot: string): Promise<string> {
  const matches = [...html.matchAll(LOCAL_IMAGE_PATTERN)]
  if (matches.length === 0) return html

  let workspaceRoot: string
  try {
    if (!trustedWorkspaceRoot.trim()) throw new WriteExportPublicError(LOCAL_IMAGE_UNAVAILABLE_MESSAGE)
    workspaceRoot = await realpath(resolve(trustedWorkspaceRoot))
  } catch {
    throw new WriteExportPublicError(LOCAL_IMAGE_UNAVAILABLE_MESSAGE)
  }

  const replacements = new Map<string, string>()
  await Promise.all(
    matches.map(async (match) => {
      const rawSrc = match[2]
      if (!rawSrc || replacements.has(rawSrc)) return
      const dataUri = await localFileUrlToDataUri(rawSrc, workspaceRoot)
      if (dataUri) replacements.set(rawSrc, dataUri)
    })
  )

  if (replacements.size === 0) return html
  return html.replace(LOCAL_IMAGE_PATTERN, (fullMatch, prefix, rawSrc, suffix) => {
    return `${prefix}${replacements.get(rawSrc) ?? rawSrc}${suffix}`
  })
}

function renderPlainTextFragment(content: string): string {
  return renderToStaticMarkup(
    createElement(
      'pre',
      {
        className: 'plain-text'
      },
      content
    )
  )
}

function isRelativeMarkdownLink(href: string | undefined): boolean {
  if (!href?.trim()) return false
  const value = href.trim()
  return !value.startsWith('/') &&
    !value.startsWith('\\') &&
    !value.startsWith('#') &&
    !/^[a-zA-Z]:[\\/]/.test(value) &&
    !/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(value)
}

function isPosixOrUncAbsoluteMarkdownLink(href: string | undefined): boolean {
  if (!href?.trim()) return false
  const value = href.trim()
  return value.startsWith('/') || value.startsWith('\\') || /^%5c/iu.test(value)
}

function resolveWriteExportAnchorHref(
  href: string | undefined,
  sourcePath: string,
  resolveRelativeLinks: boolean | undefined
): string | undefined {
  if (href && projectOrdinaryWriteExportContentV1(href) !== href) return undefined
  if (resolveRelativeLinks === false) {
    if (isPosixOrUncAbsoluteMarkdownLink(href)) return undefined
    if (isRelativeMarkdownLink(href)) return href
  }
  return resolveWriteMarkdownResource(href, sourcePath) ?? href
}

function renderMarkdownFragment(
  content: string,
  sourcePath: string,
  options?: { resolveRelativeLinks?: boolean }
): string {
  return renderToStaticMarkup(
    createElement(
      ReactMarkdown,
      {
        remarkPlugins: [remarkGfm, applyWriteMarkdownAlignmentDirectives, applyWriteExportTextProjection],
        components: {
          a: ({
            href,
            children,
            ...props
          }: ComponentPropsWithoutRef<'a'> & { href?: string; children?: ReactNode }): ReactNode =>
            createElement(
              'a',
              {
                ...props,
                href: resolveWriteExportAnchorHref(
                  href,
                  sourcePath,
                  options?.resolveRelativeLinks
                )
              },
              children
            ),
          img: ({
            src,
            alt,
            ...props
          }: ComponentPropsWithoutRef<'img'> & { src?: string; alt?: string | null }): ReactNode =>
            createElement('img', {
              ...props,
              // React otherwise hoists a preload containing the original file
              // URL outside the image that our embedding gate replaces.
              fetchPriority: 'low',
              src: resolveWriteMarkdownResource(src, sourcePath),
              alt: alt ?? ''
            })
        }
      },
      content
    )
  )
}

async function buildWriteHtmlFragment(options: {
  sourcePath: string
  workspaceRoot: string
  content: string
  resolveRelativeLinks?: boolean
}): Promise<string> {
  assertOrdinaryWriteExportContentV1(options.content)
  const fragment = isMarkdownFile(options.sourcePath)
    ? renderMarkdownFragment(options.content, options.sourcePath, {
        resolveRelativeLinks: options.resolveRelativeLinks
      })
    : renderPlainTextFragment(projectOrdinaryWriteExportContentV1(options.content))
  const body = await inlineLocalImagesInHtml(fragment, options.workspaceRoot)
  return `<article class="markdown-body">${body}</article>`
}

export async function buildWriteClipboardHtmlFragment(options: {
  sourcePath: string
  workspaceRoot: string
  content: string
}): Promise<string> {
  return buildWriteHtmlFragment(options)
}

export function buildWriteExportFileName(sourcePath: string, format: WriteExportFormat): string {
  return `${basenameWithoutExtension(sourcePath)}${exportExtension(format)}`
}

export async function buildWriteExportHtmlDocument(options: {
  sourcePath: string
  workspaceRoot: string
  content: string
  title?: string
  typography?: WriteExportPayload['typography']
  wordCompatible?: boolean
  includeSourceBaseHref?: boolean
  resolveRelativeLinks?: boolean
  embedOfficialFonts?: boolean
}): Promise<string> {
  const title = options.title?.trim() || basenameWithoutExtension(options.sourcePath)
  const body = await buildWriteHtmlFragment({
    sourcePath: options.sourcePath,
    workspaceRoot: options.workspaceRoot,
    content: options.content,
    resolveRelativeLinks: options.resolveRelativeLinks
  })
  const baseElement = options.includeSourceBaseHref === false
    ? []
    : [`  <base href="${escapeHtml(pathToFileURL(`${dirname(options.sourcePath)}/`).href)}" />`]
  const namespaces = options.wordCompatible
    ? ' xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:w="urn:schemas-microsoft-com:office:word"'
    : ''

  return [
    '<!DOCTYPE html>',
    `<html lang="en"${namespaces}>`,
    '<head>',
    '  <meta charset="utf-8" />',
    '  <meta name="viewport" content="width=device-width, initial-scale=1" />',
    `  <title>${escapeHtml(title)}</title>`,
    ...baseElement,
    `  <style>${await buildWriteExportCss(options.typography, options.embedOfficialFonts)}</style>`,
    '</head>',
    '<body>',
    '  <main class="document-shell">',
    `    ${body}`,
    '  </main>',
    '</body>',
    '</html>'
  ].join('\n')
}

export async function copyWriteDocumentAsRichText(
  payload: WriteRichClipboardPayload,
  options: {
    workspaceRoot: string
    authorityCurrent?: () => boolean | Promise<boolean>
  }
): Promise<WriteRichClipboardResult> {
  try {
    const publicContent = projectOrdinaryWriteExportContentV1(payload.content)
    if (options.authorityCurrent && !(await options.authorityCurrent())) {
      return {
        ok: false,
        message: 'Write export authority is no longer current.'
      }
    }
    const resolved = await resolveWorkspaceFile({
      path: payload.path,
      workspaceRoot: options.workspaceRoot
    })
    if (!resolved.ok) {
      return {
        ok: false,
        message: resolved.message
      }
    }

    const html = await buildWriteClipboardHtmlFragment({
      sourcePath: resolved.path,
      workspaceRoot: options.workspaceRoot,
      content: payload.content
    })

    if (options.authorityCurrent && !(await options.authorityCurrent())) {
      return {
        ok: false,
        message: 'Write export authority is no longer current.'
      }
    }

    clipboard.write({
      html,
      text: publicContent
    })

    return {
      ok: true,
      copiedAt: new Date().toISOString()
    }
  } catch (error) {
    return {
      ok: false,
      message: error instanceof WriteExportPublicError ? error.message : 'Write rich clipboard copy failed.'
    }
  }
}

async function renderHtmlToPdf(html: string): Promise<Buffer> {
  const tempDir = await mkdtemp(join(tmpdir(), 'analytix-export-'))
  const tempHtmlPath = join(tempDir, 'document.html')
  await writeFile(tempHtmlPath, html, 'utf8')

  const hiddenWindow = new BrowserWindow({
    show: false,
    backgroundColor: '#ffffff',
    webPreferences: {
      sandbox: true
    }
  })

  try {
    await hiddenWindow.loadURL(pathToFileURL(tempHtmlPath).href)
    await hiddenWindow.webContents.executeJavaScript(`
      Promise.all([
        document.fonts?.ready ?? Promise.resolve(),
        Promise.all(
          Array.from(document.images).map((image) => {
            if (image.complete) return Promise.resolve()
            return new Promise((resolve) => {
              const done = () => resolve(undefined)
              image.addEventListener('load', done, { once: true })
              image.addEventListener('error', done, { once: true })
            })
          })
        )
      ]).then(() => undefined)
    `)
    await delay(120)
    const pdf = await hiddenWindow.webContents.printToPDF({
      printBackground: true,
      preferCSSPageSize: true
    })
    return Buffer.from(pdf)
  } finally {
    if (!hiddenWindow.isDestroyed()) hiddenWindow.destroy()
    await rm(tempDir, { recursive: true, force: true })
  }
}

async function showExportSaveDialog(
  sourcePath: string,
  format: WriteExportFormat,
  parentWindow?: BrowserWindow | null
): Promise<Electron.SaveDialogReturnValue> {
  const options: Electron.SaveDialogOptions = {
    title: 'Export document',
    defaultPath: defaultExportPath(sourcePath, format),
    filters: [exportDialogFilter(format)]
  }
  return parentWindow
    ? dialog.showSaveDialog(parentWindow, options)
    : dialog.showSaveDialog(options)
}

export async function exportWriteDocument(
  payload: WriteExportPayload,
  options: {
    workspaceRoot: string
    parentWindow?: BrowserWindow | null
    authorityCurrent?: () => boolean | Promise<boolean>
  }
): Promise<WriteExportResult> {
  try {
    assertOrdinaryWriteExportContentV1(payload.content)
    if (options.authorityCurrent && !(await options.authorityCurrent())) {
      return {
        ok: false,
        canceled: false,
        message: 'Write export authority is no longer current.'
      }
    }
    const resolved = await resolveWorkspaceFile({
      path: payload.path,
      workspaceRoot: options.workspaceRoot
    })
    if (!resolved.ok) {
      return {
        ok: false,
        canceled: false,
        message: resolved.message
      }
    }

    const sourcePath = resolved.path
    const exportDialogResult = await showExportSaveDialog(sourcePath, payload.format, options?.parentWindow)
    if (exportDialogResult.canceled || !exportDialogResult.filePath) {
      return {
        ok: false,
        canceled: true
      }
    }
    if (options.authorityCurrent && !(await options.authorityCurrent())) {
      return {
        ok: false,
        canceled: false,
        message: 'Write export authority is no longer current.'
      }
    }

    const targetPath = ensureExportExtension(exportDialogResult.filePath, payload.format)
    const title = basenameWithoutExtension(sourcePath)

    let output: Buffer | string
    if (payload.format === 'docx') {
      output = await buildWriteDocxDocument({
        sourcePath,
        workspaceRoot: options.workspaceRoot,
        publicContent: payload.content,
        projectProse: projectOrdinaryWriteExportContentV1,
        typography: normalizeExportTypography(payload.typography),
        title
      })
    } else {
      const html = await buildWriteExportHtmlDocument({
        sourcePath,
        workspaceRoot: options.workspaceRoot,
        content: payload.content,
        typography: payload.typography,
        title,
        wordCompatible: payload.format === 'doc',
        includeSourceBaseHref: payload.format !== 'html',
        resolveRelativeLinks: payload.format !== 'html',
        embedOfficialFonts: payload.format === 'html'
      })
      output = payload.format === 'pdf' ? await renderHtmlToPdf(html) : html
    }

    if (options.authorityCurrent && !(await options.authorityCurrent())) {
      return {
        ok: false,
        canceled: false,
        message: 'Write export authority is no longer current.'
      }
    }
    await writeFile(targetPath, output, typeof output === 'string' ? 'utf8' : undefined)

    return {
      ok: true,
      path: targetPath,
      format: payload.format,
      exportedAt: new Date().toISOString()
    }
  } catch (error) {
    return {
      ok: false,
      canceled: false,
      message: error instanceof WriteExportPublicError ? error.message : 'Write export failed.'
    }
  }
}
