import { describe, expect, it } from 'vitest'

import { projectAttachmentReferencesForPublicSurfaces } from './attachment-public'

describe('attachment public projection', () => {
  it('excludes extracted text and preview bytes from public surfaces', () => {
    const sentinel = '6222020202020202020'
    const [attachment] = projectAttachmentReferencesForPublicSurfaces([{
      id: 'att_0123456789abcdef01234567',
      kind: 'document',
      name: 'statement.pdf',
      mimeType: 'application/pdf',
      byteSize: 128,
      pageCount: 2,
      documentText: sentinel,
      textPreview: sentinel,
      previewUrl: `data:application/pdf;base64,${sentinel}`,
      localFilePath: '/tmp/statement.pdf',
      FilePath: '/tmp/statement.pdf'
    }])

    expect(attachment).toEqual({
      id: 'att_0123456789abcdef01234567',
      kind: 'document',
      name: 'statement.pdf',
      mimeType: 'application/pdf',
      byteSize: 128,
      pageCount: 2
    })
    expect(JSON.stringify(attachment)).not.toContain(sentinel)
    expect(attachment).not.toHaveProperty('documentText')
    expect(attachment).not.toHaveProperty('textPreview')
    expect(attachment).not.toHaveProperty('previewUrl')
    expect(attachment).not.toHaveProperty('localFilePath')
    expect(attachment).not.toHaveProperty('FilePath')
  })
})
