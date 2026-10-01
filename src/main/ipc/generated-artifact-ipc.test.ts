import { describe, expect, it, vi } from 'vitest'
import { createGeneratedArtifactHandler } from './generated-artifact-ipc'

const request = { threadId: 'thread-1', artifactId: 'a'.repeat(64) }
const artifact = { ...request, workspace: '/synthetic/workspace', path: '/synthetic/workspace/report.docx', kind: 'docx', revision: 'b'.repeat(64), byteSize: 1024, changed: false }

describe('generated artifact IPC', () => {
  it('resolves only the fixed private route with typed opaque identifiers', async () => {
    const transport = vi.fn().mockResolvedValue({ ok: true, status: 200, body: JSON.stringify({ ok: true, artifact }) })
    expect(await createGeneratedArtifactHandler(transport)(request)).toEqual({ ok: true, artifact })
    expect(transport).toHaveBeenCalledWith('/v1/local-display/generated-artifact', JSON.stringify(request))
  })
  it.each([
    { ...request, path: '/untrusted/report.docx' },
    { ...request, artifactId: '../report.docx' }
  ])('rejects path-bearing input before transport', async payload => {
    const transport = vi.fn()
    expect(await createGeneratedArtifactHandler(transport)(payload)).toEqual({ ok: false, code: 'invalid_request' })
    expect(transport).not.toHaveBeenCalled()
  })
  it.each([
    { ...artifact, threadId: 'other-thread' },
    { ...artifact, artifactId: 'c'.repeat(64) },
    { ...artifact, internalError: '/private/path' }
  ])('rejects mismatched or untyped replies', async reply => {
    const transport = vi.fn().mockResolvedValue({ ok: true, status: 200, body: JSON.stringify({ ok: true, artifact: reply }) })
    expect(await createGeneratedArtifactHandler(transport)(request)).toEqual({ ok: false, code: 'unavailable' })
  })
  it('does not disclose a transport error', async () => {
    const transport = vi.fn().mockRejectedValue(new Error('/private/token'))
    expect(await createGeneratedArtifactHandler(transport)(request)).toEqual({ ok: false, code: 'unavailable' })
  })
})
