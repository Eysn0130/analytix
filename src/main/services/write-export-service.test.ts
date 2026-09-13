import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { existsSync } from 'node:fs'
import { mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

vi.mock('electron', () => ({
  BrowserWindow: vi.fn(class BrowserWindow {}),
  clipboard: {
    write: vi.fn()
  },
  dialog: {
    showSaveDialog: vi.fn()
  }
}))

import {
  buildWriteClipboardHtmlFragment,
  buildWriteExportFileName,
  buildWriteExportHtmlDocument,
  copyWriteDocumentAsRichText,
  exportWriteDocument,
  projectOrdinaryWriteExportContentV1
} from './write-export-service'
import { WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY } from '../../shared/write-official-document'
import { BrowserWindow, clipboard, dialog } from 'electron'
import JSZip from 'jszip'

describe('write-export-service helpers', () => {
  let workspaceRoot = ''

  beforeEach(async () => {
    workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-write-export-'))
    vi.mocked(BrowserWindow).mockClear()
    vi.mocked(clipboard.write).mockReset()
    vi.mocked(dialog.showSaveDialog).mockReset()
  })

  afterEach(async () => {
    vi.restoreAllMocks()
    if (workspaceRoot) await rm(workspaceRoot, { recursive: true, force: true })
  })

  it('builds export file names with the requested extension', () => {
    expect(buildWriteExportFileName('/tmp/draft.md', 'html')).toBe('draft.html')
    expect(buildWriteExportFileName('/tmp/draft.md', 'pdf')).toBe('draft.pdf')
    expect(buildWriteExportFileName('/tmp/draft.md', 'doc')).toBe('draft.doc')
    expect(buildWriteExportFileName('/tmp/draft.md', 'docx')).toBe('draft.docx')
  })

  it('omits the source directory from durable html export bytes', async () => {
    const sourceDirectorySentinel = 'SLICE37_SOURCE_DIRECTORY_SENTINEL'
    const sourceDirectory = join(workspaceRoot, sourceDirectorySentinel)
    const sourcePath = join(sourceDirectory, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = '# Public export\n\nOrdinary public content.'
    await mkdir(sourceDirectory)
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'html',
      content
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: true,
      path: targetPath,
      format: 'html',
      exportedAt: expect.any(String)
    })
    const artifact = await readFile(targetPath, 'utf8')
    expect(artifact).toContain('Ordinary public content.')
    expect(artifact).not.toContain('<base href=')
    expect(artifact).not.toContain(sourceDirectorySentinel)
  })

  it('preserves a safe relative link in durable html export bytes', async () => {
    const sourceDirectorySentinel = 'SLICE38_SOURCE_DIRECTORY_SENTINEL'
    const sourceDirectory = join(workspaceRoot, sourceDirectorySentinel)
    const sourcePath = join(sourceDirectory, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = '# Public export\n\n[Linked note](./linked-note.md)\n\nOrdinary public content.'
    await mkdir(sourceDirectory)
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'html',
      content
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: true,
      path: targetPath,
      format: 'html',
      exportedAt: expect.any(String)
    })
    const artifact = await readFile(targetPath, 'utf8')
    expect({
      ordinaryContentPresent: artifact.includes('Ordinary public content.'),
      linkTextPresent: artifact.includes('Linked note'),
      baseHrefPresent: artifact.includes('<base href='),
      fileHrefPresent: artifact.includes('href="file://'),
      sourceDirectorySentinelPresent: artifact.includes(sourceDirectorySentinel),
      safeRelativeHrefPresent: artifact.includes('href="./linked-note.md"')
    }).toEqual({
      ordinaryContentPresent: true,
      linkTextPresent: true,
      baseHrefPresent: false,
      fileHrefPresent: false,
      sourceDirectorySentinelPresent: false,
      safeRelativeHrefPresent: true
    })
  })

  it('omits an absolute local link while preserving an external link in durable html', async () => {
    const absolutePathSentinel = 'SLICE40_ABSOLUTE_ANCHOR_PATH_SENTINEL'
    const privateTarget = `/private/${absolutePathSentinel}/linked-note.md`
    const privateTargetWithFragment = `${privateTarget}#private-section`
    const externalTarget = 'https://example.com/public-note'
    const sourcePath = join(workspaceRoot, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = [
      '# Public export',
      '',
      `[Private note](${privateTarget})`,
      '',
      `[Private section](${privateTargetWithFragment})`,
      '',
      `[Public site](${externalTarget})`,
      '',
      'Ordinary public content.'
    ].join('\n')
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'html',
      content
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: true,
      path: targetPath,
      format: 'html',
      exportedAt: expect.any(String)
    })
    const artifact = await readFile(targetPath, 'utf8')
    expect({
      ordinaryContentPresent: artifact.includes('Ordinary public content.'),
      privateLinkTextPresent: artifact.includes('Private note'),
      privateSectionLinkTextPresent: artifact.includes('Private section'),
      publicLinkTextPresent: artifact.includes('Public site'),
      absoluteHrefPresent: artifact.includes(`href="${privateTarget}"`),
      absoluteFragmentHrefPresent: artifact.includes(`href="${privateTargetWithFragment}"`),
      fileHrefPresent: artifact.includes('href="file://'),
      privatePathSentinelPresent: artifact.includes(absolutePathSentinel),
      externalHrefPresent: artifact.includes(`href="${externalTarget}"`)
    }).toEqual({
      ordinaryContentPresent: true,
      privateLinkTextPresent: true,
      privateSectionLinkTextPresent: true,
      publicLinkTextPresent: true,
      absoluteHrefPresent: false,
      absoluteFragmentHrefPresent: false,
      fileHrefPresent: false,
      privatePathSentinelPresent: false,
      externalHrefPresent: true
    })
  })

  it('omits a direct file URL link from durable html', async () => {
    const privatePathSentinel = 'SLICE41_DIRECT_FILE_ANCHOR_SENTINEL'
    const privateTarget = `file:///private/${privatePathSentinel}/linked-note.md#private-section`
    const externalTarget = 'https://example.com/public-note'
    const sourcePath = join(workspaceRoot, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = [
      '# Public export',
      '',
      `[Private file](${privateTarget})`,
      '',
      `[Public site](${externalTarget})`,
      '',
      'Ordinary public content.'
    ].join('\n')
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'html',
      content
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: true,
      path: targetPath,
      format: 'html',
      exportedAt: expect.any(String)
    })
    const artifact = await readFile(targetPath, 'utf8')
    expect({
      ordinaryContentPresent: artifact.includes('Ordinary public content.'),
      privateLinkTextPresent: artifact.includes('Private file'),
      publicLinkTextPresent: artifact.includes('Public site'),
      fileHrefPresent: artifact.includes('href="file://'),
      privatePathSentinelPresent: artifact.includes(privatePathSentinel),
      externalHrefPresent: artifact.includes(`href="${externalTarget}"`)
    }).toEqual({
      ordinaryContentPresent: true,
      privateLinkTextPresent: true,
      publicLinkTextPresent: true,
      fileHrefPresent: false,
      privatePathSentinelPresent: false,
      externalHrefPresent: true
    })
  })

  it('omits Windows-drive and slash-authority links from durable html', async () => {
    const windowsPathSentinel = 'SLICE42_WINDOWS_ANCHOR_SENTINEL'
    const uncPathSentinel = 'SLICE42_UNC_ANCHOR_SENTINEL'
    const windowsTarget = `C:/Users/private/${windowsPathSentinel}/linked-note.md#private-section`
    const uncTarget = `//server/share/${uncPathSentinel}/linked-note.md#private-section`
    const schemeRelativeTarget = '//example.com/public-note'
    const externalTarget = 'https://example.com/public-note'
    const sourcePath = join(workspaceRoot, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = [
      '# Public export',
      '',
      `[Windows file](<${windowsTarget}>)`,
      '',
      `[UNC file](${uncTarget})`,
      '',
      `[Scheme-relative site](${schemeRelativeTarget})`,
      '',
      `[Public site](${externalTarget})`,
      '',
      'Ordinary public content.'
    ].join('\n')
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'html',
      content
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: true,
      path: targetPath,
      format: 'html',
      exportedAt: expect.any(String)
    })
    const artifact = await readFile(targetPath, 'utf8')
    expect({
      ordinaryContentPresent: artifact.includes('Ordinary public content.'),
      windowsLinkTextPresent: artifact.includes('Windows file'),
      uncLinkTextPresent: artifact.includes('UNC file'),
      schemeRelativeLinkTextPresent: artifact.includes('Scheme-relative site'),
      publicLinkTextPresent: artifact.includes('Public site'),
      fileHrefPresent: artifact.includes('href="file://'),
      windowsPathSentinelPresent: artifact.includes(windowsPathSentinel),
      uncPathSentinelPresent: artifact.includes(uncPathSentinel),
      schemeRelativeHrefPresent: artifact.includes(`href="${schemeRelativeTarget}"`),
      externalHrefPresent: artifact.includes(`href="${externalTarget}"`)
    }).toEqual({
      ordinaryContentPresent: true,
      windowsLinkTextPresent: true,
      uncLinkTextPresent: true,
      schemeRelativeLinkTextPresent: true,
      publicLinkTextPresent: true,
      fileHrefPresent: false,
      windowsPathSentinelPresent: false,
      uncPathSentinelPresent: false,
      schemeRelativeHrefPresent: false,
      externalHrefPresent: true
    })
  })

  it('omits a backslash UNC link from durable html after Markdown normalization', async () => {
    const uncPathSentinel = 'SLICE46_BACKSLASH_UNC_ANCHOR_SENTINEL'
    const uncTarget = ['\\\\server', 'share', uncPathSentinel, 'linked-note.md'].join('\\')
    const externalTarget = 'https://example.com/public-note'
    const sourceDirectory = join(workspaceRoot, 'private-source-directory')
    const sourcePath = join(sourceDirectory, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = [
      '# Public export',
      '',
      `[Backslash UNC file](${uncTarget})`,
      '',
      `[Public site](${externalTarget})`,
      '',
      'Ordinary public content.'
    ].join('\n')
    await mkdir(sourceDirectory)
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'html',
      content
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: true,
      path: targetPath,
      format: 'html',
      exportedAt: expect.any(String)
    })
    const artifact = await readFile(targetPath, 'utf8')
    const uncHref = artifact.match(
      new RegExp(`href="([^"]*${uncPathSentinel}[^"]*)"`)
    )?.[1] ?? ''
    expect({
      ordinaryContentPresent: artifact.includes('Ordinary public content.'),
      uncLinkTextPresent: artifact.includes('Backslash UNC file'),
      publicLinkTextPresent: artifact.includes('Public site'),
      fileHrefPresent: artifact.includes('href="file://'),
      uncPathSentinelPresent: artifact.includes(uncPathSentinel),
      normalizedPrivateHrefPresent: artifact.includes(
        `/server/share/${uncPathSentinel}/linked-note.md`
      ),
      uncHref,
      externalHrefPresent: artifact.includes(`href="${externalTarget}"`)
    }).toEqual({
      ordinaryContentPresent: true,
      uncLinkTextPresent: true,
      publicLinkTextPresent: true,
      fileHrefPresent: false,
      uncPathSentinelPresent: false,
      normalizedPrivateHrefPresent: false,
      uncHref: '',
      externalHrefPresent: true
    })
  })

  it('embeds official fonts without publishing the packaged resource path in durable html', async () => {
    const resourcePathSentinel = 'SLICE39_OFFICIAL_FONT_RESOURCE_PATH_SENTINEL'
    const resourcesPath = join(workspaceRoot, resourcePathSentinel)
    const fontDirectory = join(resourcesPath, 'fonts', 'noto-serif-sc')
    const sourcePath = join(workspaceRoot, 'draft.md')
    const targetPath = join(workspaceRoot, 'published.html')
    const content = '# Public export\n\nOrdinary public content.'
    const processWithResources = process as NodeJS.Process & { resourcesPath?: string }
    const originalResourcesPath = processWithResources.resourcesPath

    await mkdir(fontDirectory, { recursive: true })
    await writeFile(
      join(fontDirectory, 'noto-serif-sc-chinese-simplified-400-normal.woff2'),
      Buffer.from([0, 1, 2, 3])
    )
    await writeFile(
      join(fontDirectory, 'noto-serif-sc-chinese-simplified-700-normal.woff2'),
      Buffer.from([4, 5, 6, 7])
    )
    await writeFile(sourcePath, content, 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })
    Object.defineProperty(process, 'resourcesPath', {
      configurable: true,
      value: resourcesPath
    })

    try {
      const result = await exportWriteDocument({
        path: sourcePath,
        format: 'html',
        content
      }, { workspaceRoot })

      expect(result).toEqual({
        ok: true,
        path: targetPath,
        format: 'html',
        exportedAt: expect.any(String)
      })
      const artifact = await readFile(targetPath, 'utf8')
      expect({
        ordinaryContentPresent: artifact.includes('Ordinary public content.'),
        officialFontFacesPresent: (artifact.match(/@font-face/g) ?? []).length === 2,
        embeddedFontSourcesPresent:
          (artifact.match(/src: url\("data:font\/woff2;base64,/g) ?? []).length === 2,
        regularFontBytesPresent:
          artifact.includes('src: url("data:font/woff2;base64,AAECAw==") format("woff2")'),
        boldFontBytesPresent:
          artifact.includes('src: url("data:font/woff2;base64,BAUGBw==") format("woff2")'),
        fileFontUrlPresent: artifact.includes('src: url("file://'),
        resourcePathSentinelPresent: artifact.includes(resourcePathSentinel)
      }).toEqual({
        ordinaryContentPresent: true,
        officialFontFacesPresent: true,
        embeddedFontSourcesPresent: true,
        regularFontBytesPresent: true,
        boldFontBytesPresent: true,
        fileFontUrlPresent: false,
        resourcePathSentinelPresent: false
      })

      const defaultHelperHtml = await buildWriteExportHtmlDocument({
        sourcePath,
        workspaceRoot,
        content
      })
      expect({
        embeddedFontSourcesPresent:
          (defaultHelperHtml.match(/src: url\("data:font\/woff2;base64,/g) ?? []).length === 2,
        regularFileSourcePresent: defaultHelperHtml.includes(
          `src: url("${pathToFileURL(join(fontDirectory, 'noto-serif-sc-chinese-simplified-400-normal.woff2')).href}")`
        ),
        boldFileSourcePresent: defaultHelperHtml.includes(
          `src: url("${pathToFileURL(join(fontDirectory, 'noto-serif-sc-chinese-simplified-700-normal.woff2')).href}")`
        ),
        fileFontUrlPresent: defaultHelperHtml.includes('src: url("file://'),
        resourcePathSentinelPresent: defaultHelperHtml.includes(resourcePathSentinel)
      }).toEqual({
        embeddedFontSourcesPresent: false,
        regularFileSourcePresent: true,
        boldFileSourcePresent: true,
        fileFontUrlPresent: true,
        resourcePathSentinelPresent: true
      })
    } finally {
      if (originalResourcesPath === undefined) {
        Reflect.deleteProperty(process, 'resourcesPath')
      } else {
        Object.defineProperty(process, 'resourcesPath', {
          configurable: true,
          value: originalResourcesPath
        })
      }
    }
  })

  it('renders projected markdown typography and resolved links without media', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')

    const html = await buildWriteExportHtmlDocument({
      sourcePath,
      workspaceRoot,
      content: '# Heading\n{: align=center}\n\nBody\n{: align=justify}\n\n[Notes](./notes.md)',
      typography: {
        fontPreset: 'custom',
        customFontFamily: "'FangSong', serif",
        fontSizePx: 21,
        lineHeight: 2
      }
    })

    expect(html).toContain('--write-export-font-size: 21px')
    expect(html).toContain('data-write-align="center"')
    expect(html).toContain('data-write-align="justify"')
    expect(html).not.toContain('<img')
    expect(html).toContain(`href="${pathToFileURL(join(workspaceRoot, 'notes.md')).href}"`)
  })

  it('does not treat a dot-prefixed workspace image as privacy-authorized', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    const imagePath = join(workspaceRoot, '..cover.png')
    const imageBytes = Buffer.from('SLICE36_DOT_PREFIX_LOCAL_IMAGE', 'utf8')
    await writeFile(imagePath, imageBytes)

    await expect(buildWriteExportHtmlDocument({
      sourcePath,
      workspaceRoot,
      content: '# Heading\n\n![Cover](./..cover.png)'
    })).rejects.toThrow('Write export cannot include images because image content privacy checks are unavailable.')
  })

  it('renders clipboard html fragments for markdown content', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    const html = await buildWriteClipboardHtmlFragment({
      sourcePath,
      workspaceRoot,
      content: '# Heading\n\n**Bold**\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n[Notes](./notes.md)'
    })

    expect(html).toContain('<article class="markdown-body">')
    expect(html).toContain('<h1>Heading</h1>')
    expect(html).toContain('<strong>Bold</strong>')
    expect(html).toContain('<table>')
    expect(html).toContain(`href="${pathToFileURL(join(workspaceRoot, 'notes.md')).href}"`)
  })

  it('applies official document export typography without content directives', async () => {
    const sourcePath = join(workspaceRoot, 'notice.md')
    const html = await buildWriteExportHtmlDocument({
      sourcePath,
      workspaceRoot,
      content: '# 通知\n\n正文段落。',
      typography: WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY
    })

    expect(html).toContain('font-size: 21px')
    expect(html).toContain('text-align: center')
    expect(html).toContain('text-indent: 2em')
    expect(html).not.toContain('{: align=')
  })

  it('renders clipboard html fragments for plain text content', async () => {
    const sourcePath = join(workspaceRoot, 'draft.txt')
    const html = await buildWriteClipboardHtmlFragment({
      sourcePath,
      workspaceRoot,
      content: 'plain text\nline two'
    })

    expect(html).toContain('<article class="markdown-body">')
    expect(html).toContain('<pre class="plain-text">plain text\nline two</pre>')
  })

  it('writes html and plain text to the clipboard', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    await writeFile(sourcePath, '# Heading\n\n**Ordinary text**')

    const result = await copyWriteDocumentAsRichText({
      path: sourcePath,
      workspaceRoot: '/tmp/renderer-forged-workspace',
      content: '# Heading\n\n**Ordinary text**'
    }, { workspaceRoot })

    expect(result.ok).toBe(true)
    expect(clipboard.write).toHaveBeenCalledWith(
      expect.objectContaining({
        html: expect.stringContaining('<article class="markdown-body">'),
        text: '# Heading\n\n**Ordinary text**'
      })
    )
    expect(clipboard.write).toHaveBeenCalledWith(
      expect.objectContaining({
        html: expect.stringContaining('<strong>Ordinary text</strong>')
      })
    )
  })

  it('projects a hostile clipboard publication failure before returning it to the renderer', async () => {
    const clipboardErrorSentinel = 'customer-pii-13900000023'
    const sourcePath = join(workspaceRoot, 'draft.md')
    await writeFile(sourcePath, '# Public clipboard', 'utf8')
    vi.mocked(clipboard.write).mockImplementationOnce(() => {
      throw new Error(`/private/${clipboardErrorSentinel}/clipboard-provider`)
    })

    const result = await copyWriteDocumentAsRichText({
      path: sourcePath,
      content: '# Public clipboard'
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: false,
      message: 'Write rich clipboard copy failed.'
    })
    expect(JSON.stringify(result)).not.toContain(clipboardErrorSentinel)
    expect(clipboard.write).toHaveBeenCalledOnce()
  })

  it.each(['inline', 'reference'])('fails closed before HTML export when an absolute %s image is outside the trusted workspace', async (syntax) => {
    const outsideRoot = await mkdtemp(join(tmpdir(), 'analytix-write-export-private-'))
    try {
      const sourcePath = join(workspaceRoot, 'draft.md')
      const targetPath = join(workspaceRoot, 'draft.html')
      const outsideImagePath = join(outsideRoot, 'private.png')
      const sentinel = 'SLICE36_OUTSIDE_IMAGE_BYTES'
      await writeFile(sourcePath, '# Draft', 'utf8')
      await writeFile(outsideImagePath, sentinel, 'utf8')
      vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

      const result = await exportWriteDocument({
        path: sourcePath,
        format: 'html',
        content: syntax === 'inline' ? `# Draft\n\n![Private](${outsideImagePath})` : `# Draft\n\n![Private][cover]\n\n[cover]: ${outsideImagePath}`
      }, { workspaceRoot })

      expect(result).toEqual({
        ok: false,
        canceled: false,
        message: 'Write export cannot include images because image content privacy checks are unavailable.'
      })
      expect(existsSync(targetPath)).toBe(false)
      expect(JSON.stringify(result)).not.toContain(outsideRoot)
      expect(JSON.stringify(result)).not.toContain(sentinel)
    } finally {
      await rm(outsideRoot, { recursive: true, force: true })
    }
  })

  it('projects prose and public Markdown attributes through HTML and DOCX', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    await writeFile(sourcePath, '# Draft')
    const content = `# Draft\n\n/Users/synthetic/private-note.md\n\n13800138000\n\n\`\`\`/Users/synthetic/private-language\nsafe\n\`\`\`\n\n[Contact](mailto:alice@example.com)\n\n[](/Users/synthetic/private-note.md)\n\n[Safe](https://example.com/notice)`
    const html = await buildWriteClipboardHtmlFragment({ sourcePath, workspaceRoot, content })
    expect(html).not.toContain('<img')
    expect(html).toContain('https://example.com/notice')
    for (const privateText of [workspaceRoot, 'private-note.md', '13800138000', 'alice@example.com', 'private-language']) expect(html).not.toContain(privateText)

    const targetPath = join(workspaceRoot, 'draft.docx')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })
    const result = await exportWriteDocument({ path: sourcePath, format: 'docx', content }, { workspaceRoot })
    expect(result.ok).toBe(true)
    const zip = await JSZip.loadAsync(await readFile(targetPath))
    expect(Object.keys(zip.files).filter((path) => path.startsWith('word/media/') && !zip.files[path].dir)).toHaveLength(0)
    const xml = (await Promise.all(Object.values(zip.files).filter((file) => /\.xml$|\.rels$/.test(file.name)).map((file) => file.async('string')))).join('\n')
    expect(xml).toContain('https://example.com/notice')
    for (const privateText of [workspaceRoot, 'private-note.md', '13800138000', 'alice@example.com', 'private-language']) expect(xml).not.toContain(privateText)
  })

  it.each(['13912345678', '%31%33%39%31%32%33%34%35%36%37%38'])('rejects private remote image destinations before clipboard publication: %s', async (identifier) => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    await writeFile(sourcePath, '# Draft')
    const result = await copyWriteDocumentAsRichText({ path: sourcePath, content: `![Safe](https://example.test/${identifier}.png)` }, { workspaceRoot })
    expect(result).toEqual({ ok: false, message: 'Write export cannot include images because image content privacy checks are unavailable.' })
    expect(clipboard.write).not.toHaveBeenCalled()
  })

  it('fails closed before clipboard publication when a workspace image symlink escapes', async () => {
    const outsideRoot = await mkdtemp(join(tmpdir(), 'analytix-write-export-private-'))
    try {
      const sourcePath = join(workspaceRoot, 'draft.md')
      const outsideImagePath = join(outsideRoot, 'private.png')
      const linkedImagePath = join(workspaceRoot, 'linked.png')
      const sentinel = 'SLICE36_SYMLINK_IMAGE_BYTES'
      await writeFile(sourcePath, '# Draft', 'utf8')
      await writeFile(outsideImagePath, sentinel, 'utf8')
      await symlink(outsideImagePath, linkedImagePath)

      const result = await copyWriteDocumentAsRichText({
        path: sourcePath,
        workspaceRoot,
        content: '# Draft\n\n![Private](./linked.png)'
      }, { workspaceRoot })

      expect(result).toEqual({
        ok: false,
        message: 'Write export cannot include images because image content privacy checks are unavailable.'
      })
      expect(clipboard.write).not.toHaveBeenCalled()
      expect(JSON.stringify(result)).not.toContain(outsideRoot)
      expect(JSON.stringify(result)).not.toContain(sentinel)
    } finally {
      await rm(outsideRoot, { recursive: true, force: true })
    }
  })

  it('projects complete financial identifiers on ordinary clipboard and html paths', async () => {
    const sourcePath = join(workspaceRoot, 'case-note.md')
    await writeFile(sourcePath, '# Case note')
    const account = '0000622202020202020'

    const result = await copyWriteDocumentAsRichText({
      path: sourcePath,
      workspaceRoot,
      content: `# Case note\n\n账号：${account}`
    }, { workspaceRoot })

    expect(result.ok).toBe(true)
    expect(clipboard.write).toHaveBeenCalledWith(
      expect.objectContaining({
        text: '# Case note\n\n账号:[ACCOUNT]',
        html: expect.stringContaining('账号:[ACCOUNT]')
      })
    )
    expect(JSON.stringify(vi.mocked(clipboard.write).mock.calls)).not.toContain(account)
  })

  it('fails closed before clipboard publication when content contains private reasoning', async () => {
    const sourcePath = join(workspaceRoot, 'case-note.md')
    await writeFile(sourcePath, '# Case note')
    const result = await copyWriteDocumentAsRichText({
      path: sourcePath,
      workspaceRoot,
      content: 'public<think>PRIVATE_REASONING_SENTINEL</think>'
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: false,
      message: 'Write export contains private reasoning and was blocked.'
    })
    expect(clipboard.write).not.toHaveBeenCalled()
  })

  it('does not treat generic Write export as a controlled full-account surface', () => {
    expect(projectOrdinaryWriteExportContentV1('银行卡号 6222020202020202020'))
      .toBe('银行卡号 [ACCOUNT]')
  })

  it('rejects every structured detached private accepted-final strong marker but admits generic metadata', () => {
    for (const marker of [
      'acceptedFinal',
      'factFinalWitnessAdmission',
      'publicationSnapshotProof',
      'publicationSnapshotProofDigest'
    ]) {
      const content = JSON.stringify({ generic: { detached: { [marker]: 'PRIVATE_AUTHORITY_SENTINEL' } } })
      expect(() => projectOrdinaryWriteExportContentV1(content), marker)
        .toThrow('Write export contains private accepted-final authority and was blocked.')
    }

    const genericMetadata = JSON.stringify({
      envelope: { schemaVersion: 1 },
      registryHead: { sequence: 7 },
      publicationIntent: 'ordinary',
      storeDigest: 'generic-digest'
    })
    expect(projectOrdinaryWriteExportContentV1(genericMetadata)).toBe(genericMetadata)
  })

  it('blocks structured private accepted-final authority before workspace resolution or clipboard effects', async () => {
    const result = await copyWriteDocumentAsRichText({
      path: join(workspaceRoot, 'missing.md'),
      workspaceRoot,
      content: JSON.stringify({
        acceptedFinal: { digest: 'PRIVATE_ACCEPTED_FINAL_SENTINEL' },
        toolResult: {
          detail: 'PRIVATE_TOOL_DETAIL_SENTINEL',
          citation: 'SOURCE_EXACT_CITATION_SENTINEL',
          dataUrl: 'data:text/plain;base64,UkFXX1RPT0xfQllURVM=',
          path: '/Users/private/cases/tool-result.bin',
          person: '赵敏',
          privateDiagnostic: 'PRIVATE_DIAGNOSTIC_SENTINEL',
          rawBody: 'RAW_TOOL_BODY_SENTINEL'
        }
      })
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: false,
      message: 'Write export contains private accepted-final authority and was blocked.'
    })
    expect(clipboard.write).not.toHaveBeenCalled()
  })

  it('projects every file export format through the shared ordinary html boundary', async () => {
    const sourcePath = join(workspaceRoot, 'case-report.md')
    const account = '0000622202020202020'
    const html = await buildWriteExportHtmlDocument({
      sourcePath,
      workspaceRoot,
      content: `账号：${account}`
    })
    expect(html).toContain('账号:[ACCOUNT]')
    expect(html).not.toContain(account)

    await expect(buildWriteExportHtmlDocument({
      sourcePath,
      workspaceRoot,
      content: '<think>PRIVATE_FILE_REASONING</think>public'
    })).rejects.toThrow('Write export contains private reasoning and was blocked.')
  })

  it('writes DOCX through the document model after ordinary masking', async () => {
    const sourcePath = join(workspaceRoot, 'case-note.md')
    const targetPath = join(workspaceRoot, 'case-note.docx')
    const account = '0000622202020202020'
    await writeFile(sourcePath, '# Case note', 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })
    const fetchSpy = vi.spyOn(globalThis, 'fetch')

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'docx',
      content: `# 资金摘要\n\n账号：${account}\n\n- 已核验条目`,
      typography: WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY
    }, { workspaceRoot })

    expect(result).toMatchObject({ ok: true, path: targetPath, format: 'docx' })
    const archive = await JSZip.loadAsync(await readFile(targetPath))
    const documentXml = await archive.file('word/document.xml')?.async('text')
    expect(documentXml).toContain('资金摘要')
    expect(documentXml).toContain('[ACCOUNT]')
    expect(documentXml).not.toContain(account)
    expect(archive.file('word/_rels/document.xml.rels')).not.toBeNull()
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('blocks structured private accepted-final authority before dialog, file, network, or preview effects', async () => {
    const sourcePath = join(workspaceRoot, 'case-note.md')
    const targetPath = join(workspaceRoot, 'case-note.pdf')
    await writeFile(sourcePath, '# Case note', 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })
    const fetchSpy = vi.spyOn(globalThis, 'fetch')

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'pdf',
      content: JSON.stringify({
        publicationSnapshotProofDigest: 'PRIVATE_SNAPSHOT_PROOF_SENTINEL',
        toolResult: {
          detail: 'PRIVATE_TOOL_DETAIL_SENTINEL',
          citation: 'SOURCE_EXACT_CITATION_SENTINEL',
          dataUrl: 'data:text/plain;base64,UkFXX1RPT0xfQllURVM=',
          path: '/Users/private/cases/tool-result.bin',
          person: '赵敏',
          privateDiagnostic: 'PRIVATE_DIAGNOSTIC_SENTINEL',
          rawBody: 'RAW_TOOL_BODY_SENTINEL'
        }
      })
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: false,
      canceled: false,
      message: 'Write export contains private accepted-final authority and was blocked.'
    })
    expect(dialog.showSaveDialog).not.toHaveBeenCalled()
    expect(BrowserWindow).not.toHaveBeenCalled()
    expect(existsSync(targetPath)).toBe(false)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('projects a hostile final write failure before returning it to the renderer', async () => {
    const targetPathSentinel = 'customer-pii-13900000022'
    const sourcePath = join(workspaceRoot, 'draft.md')
    const targetPath = join(workspaceRoot, targetPathSentinel, 'published.docx')
    await writeFile(sourcePath, '# Public export', 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'docx',
      content: '# Public export'
    }, { workspaceRoot })

    expect(result).toEqual({
      ok: false,
      canceled: false,
      message: 'Write export failed.'
    })
    expect(JSON.stringify(result)).not.toContain(targetPathSentinel)
    expect(dialog.showSaveDialog).toHaveBeenCalledOnce()
    expect(existsSync(targetPath)).toBe(false)
  })

  it('rejects a source outside the Main-owned workspace before opening the save dialog', async () => {
    const outsideRoot = await mkdtemp(join(tmpdir(), 'analytix-write-export-outside-'))
    try {
      const sourcePath = join(outsideRoot, 'outside.md')
      await writeFile(sourcePath, '# Outside', 'utf8')

      const result = await exportWriteDocument({
        path: sourcePath,
        format: 'docx',
        content: '# Outside'
      }, { workspaceRoot })

      expect(result.ok).toBe(false)
      expect(dialog.showSaveDialog).not.toHaveBeenCalled()
    } finally {
      await rm(outsideRoot, { recursive: true, force: true })
    }
  })

  it('does not write after the Main-owned export authority changes', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    const targetPath = join(workspaceRoot, 'draft.docx')
    await writeFile(sourcePath, '# Draft', 'utf8')
    vi.mocked(dialog.showSaveDialog).mockResolvedValue({ canceled: false, filePath: targetPath })
    const authorityCurrent = vi.fn()
      .mockResolvedValueOnce(true)
      .mockResolvedValueOnce(false)

    const result = await exportWriteDocument({
      path: sourcePath,
      format: 'docx',
      content: '# Draft'
    }, { workspaceRoot, authorityCurrent })

    expect(result).toEqual({
      ok: false,
      canceled: false,
      message: 'Write export authority is no longer current.'
    })
    expect(authorityCurrent).toHaveBeenCalledTimes(2)
    expect(existsSync(targetPath)).toBe(false)
  })

  it('does not publish after the Main-owned clipboard authority changes', async () => {
    const sourcePath = join(workspaceRoot, 'draft.md')
    await writeFile(sourcePath, '# Draft', 'utf8')
    const authorityCurrent = vi.fn()
      .mockResolvedValueOnce(true)
      .mockResolvedValueOnce(false)

    const result = await copyWriteDocumentAsRichText({
      path: sourcePath,
      workspaceRoot: '/tmp/renderer-forged-workspace',
      content: '# Draft\n\n**Ordinary text**'
    }, { workspaceRoot, authorityCurrent })

    expect(result).toEqual({
      ok: false,
      message: 'Write export authority is no longer current.'
    })
    expect(authorityCurrent).toHaveBeenCalledTimes(2)
    expect(clipboard.write).not.toHaveBeenCalled()
  })
})
