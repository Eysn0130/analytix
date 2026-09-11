import { mkdtemp, rm, symlink, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import { afterEach, describe, expect, it } from 'vitest'

import {
  buildWriteDocxDocument,
  inspectWriteDocxPackageV1,
  WRITE_DOCX_PAGE_BREAK_MARKER
} from './write-docx-service'
import { WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY } from '../../shared/write-official-document'

const ONE_BY_ONE_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  'base64'
)

async function fixtureWorkspace(): Promise<string> {
  const workspaceRoot = await mkdtemp(join(tmpdir(), 'analytix-write-docx-'))
  fixtureRoots.push(workspaceRoot)
  return workspaceRoot
}

const fixtureRoots: string[] = []

afterEach(async () => {
  const roots = fixtureRoots.splice(0)
  await Promise.all(roots.map((root) => rm(root, { recursive: true, force: true })))
})

describe('write-docx-service', () => {
  it('builds a real OOXML package directly from GFM AST', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const sourcePath = join(workspaceRoot, 'notice.md')
    await writeFile(join(workspaceRoot, 'seal.png'), ONE_BY_ONE_PNG)
    await writeFile(sourcePath, '# 通知')

    const buffer = await buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      title: '通知',
      typography: WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY,
      publicContent: [
        '# 通知标题',
        '{: align=center}',
        '',
        '正文 **加粗**、*斜体*、~~删除~~ 与 `代码`。',
        '',
        '- 第一项',
        '- 第二项',
        '',
        '1. 有序一',
        '2. 有序二',
        '',
        '> 公文引用',
        '',
        '| 项目 | 金额 |',
        '| --- | ---: |',
        '| 收入 | [ACCOUNT] |',
        '',
        '![印章](./seal.png)',
        '',
        '[安全链接](https://example.com/notice)',
        '',
        WRITE_DOCX_PAGE_BREAK_MARKER,
        '',
        '<script>alert("must remain text")</script>'
      ].join('\n')
    })

    const inspection = await inspectWriteDocxPackageV1(buffer)
    expect(buffer.byteLength).toBeGreaterThan(0)
    expect(inspection.mediaParts).toHaveLength(1)
    expect(inspection.mediaParts[0]).toMatch(/^word\/media\/[^/]+\.png$/)
    expect(inspection.externalRelationships).toHaveLength(1)
    expect(inspection.externalRelationships[0]?.target).toBe('https://example.com/notice')
    expect(inspection.documentXml).toContain('w:br w:type="page"')
    expect(inspection.documentXml).toContain('ACCOUNT')
    expect(inspection.documentXml).toContain('must remain text')
    expect(inspection.documentXml).toContain('eastAsia="FangSong"')
    expect(inspection.documentXml).toContain('w:firstLineChars="200"')
    expect(inspection.documentXml).toContain('w:pgSz w:w="11906" w:h="16838"')
    expect(inspection.documentXml).toContain('Heading1')
    expect(inspection.documentXml).toContain('<w:numPr>')
    expect(inspection.documentXml).toContain('w:tbl')
    expect(inspection.stylesXml).toContain('w:styles')
    expect(inspection.relationshipsXml).not.toContain('TargetMode="External" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"')
  })

  it('preserves plain text line breaks and the exact page-break marker', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const sourcePath = join(workspaceRoot, 'draft.txt')
    await writeFile(sourcePath, 'plain one\nplain two')
    const buffer = await buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: `plain one\nplain two\n${WRITE_DOCX_PAGE_BREAK_MARKER}\nplain three`
    })
    const inspection = await inspectWriteDocxPackageV1(buffer)
    expect(inspection.documentXml).toContain('plain one')
    expect(inspection.documentXml).toContain('plain two')
    expect(inspection.documentXml).toContain('plain three')
    expect(inspection.documentXml.match(/w:type="page"/g)).toHaveLength(1)
  })

  it('preserves an already-authorized full local projection without owning projection authority', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const sourcePath = join(workspaceRoot, 'authorized.md')
    const syntheticIdentifier = 'FULL-PROJECTION-SENTINEL-620202020202020202'
    await writeFile(sourcePath, '# Authorized')
    const buffer = await buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: `# Authorized\n\n${syntheticIdentifier}`
    })
    const inspection = await inspectWriteDocxPackageV1(buffer)
    expect(inspection.documentXml).toContain(syntheticIdentifier)
  })

  it('renders unsafe links as inert labels and never creates unsafe relationships', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const sourcePath = join(workspaceRoot, 'links.md')
    await writeFile(sourcePath, '# Links')
    const buffer = await buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: '[do not run](javascript:alert(1)) [file](file:///tmp/private) [mail](mailto:qa@example.com)'
    })
    const inspection = await inspectWriteDocxPackageV1(buffer)
    expect(inspection.externalRelationships).toHaveLength(1)
    expect(inspection.externalRelationships[0]?.target).toBe('mailto:qa@example.com')
    expect(inspection.documentXml).toContain('do not run')
    expect(inspection.documentXml).toContain('file')
    expect(inspection.documentXml).not.toContain('javascript:')
  })

  it('rejects remote, mismatched, symlink-escaped, and oversized image inputs', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const sourcePath = join(workspaceRoot, 'images.md')
    await writeFile(sourcePath, '# Images')
    await writeFile(join(workspaceRoot, 'wrong.jpg'), ONE_BY_ONE_PNG)
    await expect(buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: '![remote](https://example.com/a.png)'
    })).rejects.toThrow('local workspace files')
    await expect(buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: '![mismatch](./wrong.jpg)'
    })).rejects.toThrow('signature does not match')

    const outside = await fixtureWorkspace()
    await writeFile(join(outside, 'outside.png'), ONE_BY_ONE_PNG)
    await symlink(join(outside, 'outside.png'), join(workspaceRoot, 'escape.png'))
    await expect(buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: '![escape](./escape.png)'
    })).rejects.toThrow('escaped the canonical workspace')

    await writeFile(join(workspaceRoot, 'too-large.png'), Buffer.alloc(8 * 1024 * 1024 + 1))
    await expect(buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: '![oversized](./too-large.png)'
    })).rejects.toThrow('safe size limit')
  })

  it('rejects a source outside the canonical workspace', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const outside = await fixtureWorkspace()
    const sourcePath = join(outside, 'outside.md')
    await writeFile(sourcePath, '# Outside')
    await expect(buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: 'not allowed'
    })).rejects.toThrow('source escaped the canonical workspace')
  })

  it('fails closed when an unprojected private-reasoning marker reaches the host boundary', async () => {
    const workspaceRoot = await fixtureWorkspace()
    const sourcePath = join(workspaceRoot, 'private.md')
    await writeFile(sourcePath, '# Private')
    await expect(buildWriteDocxDocument({
      sourcePath,
      workspaceRoot,
      publicContent: '<think>PRIVATE_REASONING_SENTINEL</think>'
    })).rejects.toThrow('private reasoning')
  })
})
