import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import { createWriteExportSnapshotResolver } from './write-export-ipc'

const binding = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: 'main-thread', baseRevision: 'c'.repeat(64), draftVersion: 'd'.repeat(48) }
const snapshot = { ...binding, workspace: '/current-thread-workspace', path: '/current-thread-workspace/note.md', content: '未保存正文', contentDigest: createHash('sha256').update('未保存正文').digest('hex') }
const response = (value = snapshot) => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, snapshot: value }) })

describe('Core-owned export snapshot resolution', () => {
  it('uses identity-only requests and rechecks exact bytes and authority on every effect', async () => {
    const transport = vi.fn(async (_path: string, _body: string) => response())
    const resolved = await createWriteExportSnapshotResolver(transport)(binding, () => true)
    expect(resolved?.snapshot).toEqual(snapshot)
    expect(transport.mock.calls[0][0]).toBe('/v1/local-display/object-editing')
    expect(JSON.parse(transport.mock.calls[0][1])).toEqual({ action: 'export-snapshot', ...binding })
    expect(await resolved?.authorityCurrent()).toBe(true)
    transport.mockResolvedValueOnce({ ok: false, status: 409, body: '{"ok":false,"code":"conflict"}' })
    expect(await resolved?.authorityCurrent()).toBe(false)
    expect(transport).toHaveBeenCalledTimes(3)
  })

  it.each(['sessionId', 'objectId', 'threadId', 'baseRevision', 'draftVersion', 'contentDigest'] as const)('rejects a mismatched %s without returning private content', async key => {
    const value = { ...snapshot, [key]: key === 'threadId' ? 'foreign-thread' : 'f'.repeat(snapshot[key].length) }
    expect(await createWriteExportSnapshotResolver(vi.fn(async () => response(value)))(binding, () => true)).toBeNull()
  })

  it('rejects wrong status, extra fields, transport failure and replaced owner/frame', async () => {
    for (const value of [{ ...response(), status: 500 }, { ...response(), body: JSON.stringify({ ok: true, snapshot, extra: true }) }]) {
      expect(await createWriteExportSnapshotResolver(vi.fn(async () => value))(binding, () => true)).toBeNull()
    }
    expect(await createWriteExportSnapshotResolver(vi.fn(async () => { throw new Error('private path') }))(binding, () => true)).toBeNull()
    let current = true
    const transport = vi.fn(async () => { current = false; return response() })
    expect(await createWriteExportSnapshotResolver(transport)(binding, () => current)).toBeNull()
  })

  it('never silently switches the snapshot after the save dialog', async () => {
    const transport = vi.fn(async (_path: string, _body: string) => response())
    const resolved = await createWriteExportSnapshotResolver(transport)(binding, () => true)
    transport.mockResolvedValueOnce(response({ ...snapshot, content: 'other', contentDigest: createHash('sha256').update('other').digest('hex') }))
    expect(await resolved?.authorityCurrent()).toBe(false)
    transport.mockResolvedValueOnce(response({ ...snapshot, workspace: '/other' }))
    expect(await resolved?.authorityCurrent()).toBe(false)
  })
})
