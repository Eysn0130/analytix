import { afterEach, describe, expect, it, vi } from 'vitest'
import { openTextObject } from './object-editing-client'

afterEach(() => vi.unstubAllGlobals())

describe('object editor authority', () => {
  it.each(['unavailable', 'forbidden', 'persistence_failure'])('does not bypass Core on %s', async (code) => {
    const read = vi.fn()
    vi.stubGlobal('window', { analytix: { objects: { request: vi.fn(async () => ({ ok: false, code, message: 'failure' })) }, files: { read } } })
    await expect(openTextObject('/workspace', '/workspace/a.md')).rejects.toThrow()
    expect(read).not.toHaveBeenCalled()
  })

  it('labels the explicit unsupported-platform compatibility path', async () => {
    const read = vi.fn(async () => ({ ok: true, content: 'text', path: '/workspace/a.md', size: 4, truncated: false }))
    vi.stubGlobal('window', { analytix: { objects: { request: vi.fn(async () => ({ ok: false, code: 'unsupported_platform' })) }, files: { read } } })
    expect(await openTextObject('/workspace', '/workspace/a.md')).toMatchObject({ legacy: true, content: 'text', sessionId: '' })
    expect(read).toHaveBeenCalledOnce()
  })
})
