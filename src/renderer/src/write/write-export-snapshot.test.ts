import { afterEach, describe, expect, it, vi } from 'vitest'
import { useWriteWorkspaceStore as store } from './write-workspace-store'
import { captureWriteExport } from './write-export-snapshot'
import type { ObjectEditingRequest, ObjectEditingResponse } from '../../../../packages/runtime/src/contracts/object-editing'

const sessionId = 'a'.repeat(48), objectId = 'b'.repeat(64), revision = 'c'.repeat(64), version = 'd'.repeat(48)
const document = { sessionId, objectId, revision, path: '/tmp/export/note.md', content: 'original' }
let release: (() => void) | undefined
async function setup(override?: (input: ObjectEditingRequest) => Promise<ObjectEditingResponse | undefined>) {
  const request = vi.fn(async (input: ObjectEditingRequest): Promise<ObjectEditingResponse> => {
    const result = await override?.(input)
    if (result) return result
    if (input.action === 'open') return { ok: true, document }
    if (input.action === 'draft-read') return { ok: false, code: 'draft_stale', message: 'No draft' }
    if (input.action === 'draft-update') return { ok: true, draft: { objectId, baseRevision: input.baseRevision, version, content: input.content } }
    throw new Error('unexpected operation')
  })
  const write = vi.fn()
  vi.stubGlobal('window', { analytix: { objects: { request }, files: { write } } })
  store.setState({ workspaceRoot: '/tmp/export', rootDirectory: '/tmp/export' })
  await store.getState().openFile('/tmp/export', document.path)
  store.getState().setFileContent('unsaved draft')
  return { request, write }
}
afterEach(() => { release?.(); release = undefined; store.getState().resetWorkspace(); vi.unstubAllGlobals() })

describe('explicit export draft capture', () => {
  it('captures unsaved bytes without saving, allows new input, and releases the autosave hold', async () => {
    const { request, write } = await setup()
    const prepared = await captureWriteExport('main-thread', () => 'main-thread')
    release = prepared.release
    expect(prepared.binding).toEqual({ sessionId, objectId, threadId: 'main-thread', baseRevision: revision, draftVersion: version })
    store.getState().setFileContent('newer input remains local')
    expect(await store.getState().flushSave('/tmp/export')).toBe(false)
    expect(store.getState().fileContent).toBe('newer input remains local')
    expect(request.mock.calls.map(([r]) => r.action)).toEqual(['open', 'draft-read', 'draft-update'])
    expect(write).not.toHaveBeenCalled()
    prepared.release(); prepared.release()
    expect(store.getState().exportInProgress).toBe(false)
    expect(store.getState().saveStatus).toBe('dirty')
  })

  it('settles the existing save before capture without saving later input', async () => {
    let finish!: (response: ObjectEditingResponse) => void
    const nextRevision = 'e'.repeat(64)
    const { request } = await setup(async input => input.action === 'commit' ? new Promise(resolve => { finish = resolve }) : undefined)
    const saving = store.getState().flushSave('/tmp/export')
    store.getState().setFileContent('later local input')
    const preparing = captureWriteExport('main-thread', () => 'main-thread')
    await Promise.resolve()
    expect(request.mock.calls.map(([r]) => r.action)).toEqual(['open', 'commit'])
    const commit = request.mock.calls[1][0]
    if (commit.action !== 'commit') throw new Error('missing commit')
    finish({ ok: true, receipt: { operationId: commit.operationId, revision: nextRevision, status: 'committed', savedAt: '2026-09-15T00:00:00Z' } })
    expect(await saving).toBe(false)
    const prepared = await preparing
    release = prepared.release
    expect(prepared.binding.baseRevision).toBe(nextRevision)
    expect(request).toHaveBeenLastCalledWith({ action: 'draft-update', sessionId, baseRevision: nextRevision, expectedVersion: '', content: 'later local input' })
    expect(request.mock.calls.filter(([r]) => r.action === 'commit')).toHaveLength(1)
  })

  it.each(['thread', 'input', 'revision', 'cas', 'transport'])('releases and preserves input on %s failure', async failure => {
    let thread = 'main-thread'
    await setup(async input => {
      if (input.action !== 'draft-read') return
      if (failure === 'thread') thread = 'other-thread'
      if (failure === 'input') store.getState().setFileContent('new input')
      if (failure === 'revision') store.setState({ objectSession: { sessionId, objectId, revision: 'e'.repeat(64) } })
      if (failure === 'cas') return { ok: false, code: 'session_invalid', message: 'expired' }
      if (failure === 'transport') throw new Error('lost response')
    })
    await expect(captureWriteExport('main-thread', () => thread)).rejects.toThrow()
    expect(store.getState().exportInProgress).toBe(false)
    expect(store.getState().fileContent).toBe(failure === 'input' ? 'new input' : 'unsaved draft')
  })
  it('uses current draft CAS and retains local input after a lost capture acknowledgement', async () => {
    let coreVersion = version
    let lost = true
    const { request } = await setup(async input => {
      if (input.action === 'draft-read') return { ok: true, draft: { objectId, baseRevision: revision, version: coreVersion, content: 'earlier draft' } }
      if (input.action === 'draft-update') {
        expect(input.expectedVersion).toBe(coreVersion)
        coreVersion = 'f'.repeat(48)
        if (lost) { lost = false; throw new Error('lost acknowledgement') }
        return { ok: true, draft: { objectId, baseRevision: revision, version: coreVersion, content: input.content } }
      }
    })
    await expect(captureWriteExport('main-thread', () => 'main-thread')).rejects.toThrow()
    expect(store.getState()).toMatchObject({ exportInProgress: false, fileContent: 'unsaved draft' })
    const prepared = await captureWriteExport('main-thread', () => 'main-thread')
    release = prepared.release
    expect(prepared.binding.draftVersion).toBe(coreVersion)
    expect(request.mock.calls.some(([r]) => r.action === 'commit')).toBe(false)
  })

})
