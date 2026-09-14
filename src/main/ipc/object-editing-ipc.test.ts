import { describe, expect, it, vi } from 'vitest'
import { createObjectEditingHandler } from './object-editing-ipc'
import { objectEditingPath, objectEditingReceiptSchema } from '../../../packages/runtime/src/contracts/object-editing'

const commit = { action: 'commit', sessionId: 'a'.repeat(48), operationId: 'save_1234', baseRevision: 'b'.repeat(64), content: 'local draft' }

describe('protected object IPC', () => {
  it('rejects caller authority and extra paths before transport', async () => {
    const transport = vi.fn()
    expect(await createObjectEditingHandler(transport)({ ...commit, path: '/other', principal: 'forged' })).toMatchObject({ ok: false, code: 'invalid_request' })
    expect(transport).not.toHaveBeenCalled()
  })

  it('uses only the protected lane and returns a confirmed receipt', async () => {
    const receipt = { operationId: commit.operationId, revision: 'c'.repeat(64), status: 'committed', savedAt: '2026-09-14T04:00:00Z' }
    const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify({ ok: true, receipt }) }))
    expect(await createObjectEditingHandler(transport)(commit)).toEqual({ ok: true, receipt })
    expect(transport).toHaveBeenCalledExactlyOnceWith(objectEditingPath, JSON.stringify(commit))
  })

  it.each(['timeout', 'malformed', 'empty receipt'])('retains operation identity after %s, without returning unsafe errors', async (kind) => {
    const transport = vi.fn(async () => {
      if (kind === 'timeout') throw new Error('private path or server body')
      return { ok: true, status: 200, body: kind === 'malformed' ? 'private path or server body' : JSON.stringify({ ok: true, receipt: { operationId: commit.operationId, revision: '', status: 'committed', savedAt: '' } }) }
    })
    const result = await createObjectEditingHandler(transport)(commit)
    expect(result).toMatchObject({ ok: false, code: 'persistence_failure', receipt: { operationId: commit.operationId, status: 'unknown' } })
    expect(JSON.stringify(result)).not.toContain('private path')
  })

  it('rejects a saved claim without durable revision and time', () => {
    expect(objectEditingReceiptSchema.safeParse({ operationId: 'save_1234', revision: '', status: 'committed', savedAt: '' }).success).toBe(false)
  })
})
