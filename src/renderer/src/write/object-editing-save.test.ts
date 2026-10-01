import { afterEach, describe, expect, it, vi } from 'vitest'
import { useWriteWorkspaceStore as store } from './write-workspace-store'
import type { ObjectEditingRequest, ObjectEditingResponse } from '../../../../packages/runtime/src/contracts/object-editing'

const sessionId = 'a'.repeat(48)
const objectId = 'b'.repeat(64)
const revision = 'c'.repeat(64)
const nextRevision = 'd'.repeat(64)
const opened = { ok: true as const, document: { sessionId, objectId, revision, path: '/tmp/edit/draft.md', content: 'original' } }
const committed = (operationId: string): ObjectEditingResponse => ({ ok: true, receipt: { operationId, revision: nextRevision, status: 'committed', savedAt: '2026-09-14T04:00:00Z' } })

async function setup(handler: (request: ObjectEditingRequest) => Promise<ObjectEditingResponse>, read = () => opened) {
  const request = vi.fn(async (input: ObjectEditingRequest) => input.action === 'open' ? read() : handler(input))
  const write = vi.fn()
  vi.stubGlobal('window', { analytix: { objects: { request }, files: { write } } })
  store.setState({ workspaceRoot: '/tmp/edit', rootDirectory: '/tmp/edit' })
  await store.getState().openFile('/tmp/edit', opened.document.path)
  expect(store.getState().objectSession?.revision).toBe(revision)
  return { request, write }
}

afterEach(() => { store.getState().resetWorkspace(); vi.unstubAllGlobals() })

describe('versioned object saving', () => {
  it('requires an unchanged comparison and saves the draft with the compared disk revision', async () => {
    let disk = opened
    let conflict = true
    const { request } = await setup(async (input) => {
      if (input.action !== 'commit') throw new Error('unexpected operation')
      if (conflict) return { ok: false, code: 'conflict', message: 'external write' }
      return committed(input.operationId)
    }, () => disk)
    store.getState().setFileContent('my retained edit')
    expect(await store.getState().flushSave('/tmp/edit')).toBe(false)
    disk = { ok: true, document: { ...opened.document, revision: nextRevision, content: 'external version' } }
    const comparison = { sessionId, objectId, revision: nextRevision, workspaceRoot: '/tmp/edit', path: opened.document.path, diskContent: 'external version', localContent: 'my retained edit' }
    conflict = false
    expect(await store.getState().resolveFileConflict(comparison, 'keep-draft')).toBe(true)
    expect(request).toHaveBeenLastCalledWith(expect.objectContaining({ action: 'commit', content: 'my retained edit', baseRevision: nextRevision }))
    const commits = request.mock.calls.map(([r]) => r).filter((r) => r.action === 'commit')
    expect(commits[1].operationId).not.toBe(commits[0].operationId)
  })

  it('rejects an obsolete disk comparison without discarding the draft', async () => {
    const { request } = await setup(async () => ({ ok: false, code: 'conflict', message: 'external write' }))
    store.getState().setFileContent('my retained edit')
    expect(await store.getState().flushSave('/tmp/edit')).toBe(false)
    const comparison = { sessionId, objectId, revision: nextRevision, workspaceRoot: '/tmp/edit', path: opened.document.path, diskContent: 'old comparison', localContent: 'my retained edit' }
    expect(await store.getState().resolveFileConflict(comparison, 'use-disk')).toBe(false)
    expect(store.getState()).toMatchObject({ fileContent: 'my retained edit', saveStatus: 'conflict' })
    expect(request.mock.calls.filter(([r]) => r.action === 'commit')).toHaveLength(1)
  })

  it('preserves newer input while acknowledging only the frozen saved revision', async () => {
    let resolve!: (response: ObjectEditingResponse) => void
    let operationId = ''
    const { request, write } = await setup(async (input) => {
      if (input.action !== 'commit') throw new Error('unexpected operation')
      operationId = input.operationId
      return new Promise((done) => { resolve = done })
    })
    store.getState().setFileContent('first edit')
    const save = store.getState().flushSave('/tmp/edit')
    store.getState().setFileContent('newer edit')
    resolve(committed(operationId))
    expect(await save).toBe(false)
    expect(store.getState()).toMatchObject({ fileContent: 'newer edit', saveStatus: 'dirty', pendingSave: null, objectSession: { revision: nextRevision } })
    expect(request).toHaveBeenLastCalledWith(expect.objectContaining({ action: 'commit', content: 'first edit', baseRevision: revision }))
    expect(write).not.toHaveBeenCalled()
  })

  it('queries a lost receipt before retrying the exact operation, including after newer edits', async () => {
    let first = true
    const { request } = await setup(async (input) => {
      if (input.action === 'status') return { ok: false, code: 'operation_not_found', message: 'missing' }
      if (input.action !== 'commit') throw new Error('unexpected operation')
      if (first) { first = false; throw new Error('transport lost') }
      return committed(input.operationId)
    })
    store.getState().setFileContent('frozen')
    expect(await store.getState().flushSave('/tmp/edit')).toBe(false)
    expect(store.getState().saveStatus).toBe('unknown')
    store.getState().setFileContent('new input')
    expect(await store.getState().flushSave('/tmp/edit')).toBe(false)
    const commits = request.mock.calls.map(([r]) => r).filter((r) => r.action === 'commit')
    expect(commits).toHaveLength(2)
    expect(commits[1]).toEqual(commits[0])
    expect(request.mock.calls.map(([r]) => r.action)).toEqual(['open', 'commit', 'status', 'commit'])
    expect(store.getState()).toMatchObject({ fileContent: 'new input', saveStatus: 'dirty' })
  })

  it('retains the draft and frozen operation on a conflict; no file-write fallback', async () => {
    const { write } = await setup(async (input) => ({ ok: false, code: 'conflict', message: 'conflict', ...('operationId' in input ? { receipt: { operationId: input.operationId, revision, status: 'conflict' as const, savedAt: '' } } : {}) }))
    store.getState().setFileContent('retained')
    expect(await store.getState().flushSave('/tmp/edit')).toBe(false)
    expect(store.getState()).toMatchObject({ saveStatus: 'conflict', fileContent: 'retained', pendingSave: { content: 'retained' } })
    expect(await store.getState().openWorkspaceHome('/tmp/edit')).toBe(false)
    expect(await store.getState().syncActiveFileFromDisk('/tmp/edit', { force: true })).toBe(false)
    expect(write).not.toHaveBeenCalled()
  })

  it('rebinds the same object after runtime restart without replacing the working copy', async () => {
    let first = true
    const { request } = await setup(async (input) => {
      if (first) { first = false; return { ok: false, code: 'session_invalid', message: 'restart' } }
      if (input.action === 'status') return committed(input.operationId)
      throw new Error('unexpected operation')
    })
    store.getState().setFileContent('saved before lost response')
    expect(await store.getState().flushSave('/tmp/edit')).toBe(true)
    expect(request.mock.calls.map(([r]) => r.action)).toEqual(['open', 'commit', 'open', 'status'])
    expect(store.getState()).toMatchObject({ fileContent: 'saved before lost response', saveStatus: 'saved' })
  })
})
