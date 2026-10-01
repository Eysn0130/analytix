import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import { authorizeWriteRetrievalSource } from './write-retrieval-ipc'

const binding = 'a'.repeat(64)
const snapshot = { threadId: 'thread-1', workspace: '/workspace', binding }
const content = Buffer.from('SYNTHETIC_READ_CANARY')
const file = { path: 'reference.md', kind: 'text', revision: createHash('sha256').update(content).digest('hex'), content: content.toString('base64') }
function response(value: unknown) {
  return { ok: true, status: 200, body: JSON.stringify({ ok: true, snapshot: value }) }
}

describe('Core workspace read bridge', () => {
  it('rejects missing owner/thread without transport and a foreign root before scanning', async () => {
    const transport = vi.fn(async () => response(snapshot))
    expect(await authorizeWriteRetrievalSource(transport, undefined, '/workspace', () => true)).toBeNull()
    expect(await authorizeWriteRetrievalSource(transport, 'thread-1', '/workspace', () => false)).toBeNull()
    expect(transport).not.toHaveBeenCalled()
    expect(await authorizeWriteRetrievalSource(transport, 'thread-1', '/foreign', () => true)).toBeNull()
    expect(transport).toHaveBeenCalledExactlyOnceWith('/v1/local-display/workspace-read', JSON.stringify({ action: 'authorize', threadId: 'thread-1' }))
  })

  it('passes only thread/binding and consumes bounded revision-verified Core bytes', async () => {
    const transport = vi.fn(async (_path: string, body: string) => response(JSON.parse(body).action === 'scan' ? { ...snapshot, files: [file] } : snapshot))
    const source = await authorizeWriteRetrievalSource(transport, 'thread-1', '/workspace', () => true)
    expect(await source?.scan(false)).toEqual([file])
    expect(transport.mock.calls.map(([, body]) => JSON.parse(body))).toEqual([
      { action: 'authorize', threadId: 'thread-1' },
      { action: 'scan', threadId: 'thread-1', binding, includePdf: false },
      { action: 'validate', threadId: 'thread-1', binding }
    ])
  })

  it.each([
    { ...snapshot, threadId: 'other' },
    { ...snapshot, binding: 'b'.repeat(64) },
    { ...snapshot, files: [{ ...file, path: '../escape.md' }] },
    { ...snapshot, files: [{ ...file, revision: '0'.repeat(64) }] },
    { ...snapshot, files: [file, file] },
    { ...snapshot, files: [{ ...file, kind: 'pdf' }] }
  ])('rejects invalid scan snapshots', async invalid => {
    const transport = vi.fn().mockResolvedValueOnce(response(snapshot)).mockResolvedValue(response(invalid))
    const source = await authorizeWriteRetrievalSource(transport, 'thread-1', '/workspace', () => true)
    await expect(source?.scan(false)).rejects.toThrow()
  })

  it('drops a valid scan if renderer ownership or Core authority changes before return', async () => {
    for (const revoke of ['renderer', 'core']) {
      let current = true
      const transport = vi.fn(async (_path: string, body: string) => {
        const action = JSON.parse(body).action
        if (action === 'scan') {
          if (revoke === 'renderer') current = false
          return response({ ...snapshot, files: [file] })
        }
        return response(action === 'validate' ? { ...snapshot, binding: 'b'.repeat(64) } : snapshot)
      })
      const source = await authorizeWriteRetrievalSource(transport, 'thread-1', '/workspace', () => current)
      await expect(source?.scan(false)).rejects.toThrow()
    }
  })
})
