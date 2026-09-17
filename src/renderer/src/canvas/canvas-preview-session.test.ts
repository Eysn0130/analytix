import { describe, it } from 'vitest'
import assert from 'node:assert/strict'
import { createCanvasPreviewSession, type CanvasPreviewTarget } from './canvas-preview-session'
import type { CanvasDocument, CanvasHostRequest, CanvasHostResponse } from '../../../../packages/runtime/src/contracts/canvas-host'
const target: CanvasPreviewTarget = { threadId: 'thread-a', workspace: '/project', path: '/project/a.canvas', kind: 'canvas' }
const document: CanvasDocument = { sessionId: 'a'.repeat(48), objectId: 'b'.repeat(64), threadId: target.threadId,
  path: target.path, kind: 'canvas', revision: 'c'.repeat(64), content: 'e30=' }
const opened: CanvasHostResponse = { ok: true, document }
const closed: CanvasHostResponse = { ok: true, closed: true }
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(accept => { resolve = accept }); return { promise, resolve } }
const tick = async () => { for (let i = 0; i < 8; i++) await Promise.resolve() }

describe('Canvas preview lease ordering (no model or write authority)', () => {
  it('opens through the exact finite request and closes once', async () => {
    const calls: CanvasHostRequest[] = []
    const session = createCanvasPreviewSession(async request => { calls.push(request); return request.operation === 'open-object' ? opened : closed })
    assert.deepEqual(await session.open(target), document)
    assert.equal(await session.close(), true)
    assert.equal(await session.close(), true)
    assert.deepEqual(calls, [
      { operation: 'open-object', threadId: target.threadId, kind: 'canvas', object: { workspace: target.workspace, path: target.path } },
      { operation: 'close-object', sessionId: document.sessionId, threadId: target.threadId }
    ])
  })
  it('freezes caller input before asynchronous dispatch', async () => {
    const calls: CanvasHostRequest[] = [], mutable = { ...target }
    const session = createCanvasPreviewSession(async request => { calls.push(request); return opened })
    const result = session.open(mutable)
    mutable.threadId = 'thread-b'; mutable.path = '/private'
    await result
    assert.equal(calls[0].threadId, 'thread-a')
    assert.equal(calls[0].operation === 'open-object' && calls[0].object.path, target.path)
  })
  it('discards pending A, releases it before opening B, and cannot close reused B later', async () => {
    const wait = deferred<CanvasHostResponse>(), calls: CanvasHostRequest[] = []
    const session = createCanvasPreviewSession(async request => {
      calls.push(request)
      if (request.operation === 'close-object') return closed
      return calls.length === 1 ? wait.promise : { ok: true, document: { ...document, threadId: 'thread-b' } }
    })
    const a = session.open(target); await tick()
    const b = session.open({ ...target, threadId: 'thread-b' })
    wait.resolve(opened)
    assert.equal(await a, null)
    assert.equal((await b)?.threadId, 'thread-b')
    assert.deepEqual(calls.map(call => `${call.operation}:${call.threadId}`), ['open-object:thread-a', 'close-object:thread-a', 'open-object:thread-b'])
  })
  it('does not open a request invalidated before dispatch', async () => {
    let calls = 0
    const session = createCanvasPreviewSession(async () => { calls++; return opened })
    const pending = session.open(target)
    const done = session.close()
    assert.equal(await pending, null); assert.equal(await done, true); assert.equal(calls, 0)
  })
  it('does not publish a late response after closure', async () => {
    const wait = deferred<CanvasHostResponse>(), calls: string[] = []
    const session = createCanvasPreviewSession(async request => { calls.push(request.operation); return request.operation === 'open-object' ? wait.promise : closed })
    const pending = session.open(target); await tick()
    const closing = session.close(); wait.resolve(opened)
    assert.equal(await pending, null); assert.equal(await closing, true)
    assert.deepEqual(calls, ['open-object', 'close-object'])
  })
  it('does not accumulate handles or replace an unreleased owner when release fails', async () => {
    let canClose = false, opens = 0
    const session = createCanvasPreviewSession(async request => {
      if (request.operation === 'open-object') { opens++; return opened }
      return canClose ? closed : { ok: false, code: 'unavailable' }
    })
    await session.open(target)
    assert.equal(await session.open({ ...target, path: '/project/b.canvas' }), null)
    assert.equal(opens, 1)
    canClose = true
    assert.equal(await session.close(), true)
    assert.deepEqual(await session.open(target), document)
  })
  it('recovers after a throwing or rejected transport without poisoning the queue', async () => {
    let attempts = 0
    const session = createCanvasPreviewSession(request => {
      if (request.operation === 'close-object') return Promise.resolve(closed)
      attempts++
      if (attempts === 1) throw new Error('transport')
      if (attempts === 2) return Promise.reject(new Error('transport'))
      return Promise.resolve(opened)
    })
    assert.equal(await session.open(target), null)
    assert.equal(await session.open(target), null)
    assert.deepEqual(await session.open(target), document)
  })
  it('never displays a different thread or kind, even if a transport violates its contract', async () => {
    for (const replacement of [{ threadId: 'thread-b' }, { kind: 'png' as const }]) {
      const session = createCanvasPreviewSession(async () => ({ ok: true, document: { ...document, ...replacement } }))
      assert.equal(await session.open(target), null)
    }
  })
})
