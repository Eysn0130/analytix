import { describe, expect, it, vi } from 'vitest'
import type { AnalytixRuntimeApi, AnalytixRuntimeStatusPayload, RuntimeRequestResult } from '@shared/analytix-api'
import { MessageFeedbackController } from './message-feedback-client'

const version = '11111111-1111-4111-8111-111111111111'
const item = { messageId: 'message', rating: 'positive', version, createdAt: 1, updatedAt: 1 }
const result = (body: unknown, status = 200): RuntimeRequestResult => ({ ok: status < 400, status, body: JSON.stringify(body) })
function fixture(handler: AnalytixRuntimeApi['runtimeRequest']) {
  let status: ((value: AnalytixRuntimeStatusPayload) => void) | null = null
  const request = vi.fn(handler)
  const controller = new MessageFeedbackController('thread', { runtimeRequest: request,
    onRuntimeStatus: listener => { status = listener; return () => { status = null } } })
  const release = controller.subscribe(() => undefined)
  return { controller, request, release, running: () => status?.({ state: 'running' } as AnalytixRuntimeStatusPayload) }
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done }); return { promise, resolve } }

describe('per-thread message feedback controller', () => {
  it.each([409, 503])('keeps a retired DELETE failure (%i) out of a remounted owner while retaining conflict current', async status => {
    const write = deferred<RuntimeRequestResult>()
    const f = fixture(async (_path, method) => method === 'GET' ? result({ items: [item] }) : write.promise)
    const owner = Symbol('retired-row'), keeper = Symbol('other-row'), next = Symbol('remounted-row')
    const releaseOwner = f.controller.subscribe(() => undefined, owner)
    const releaseKeeper = f.controller.subscribe(() => undefined, keeper)
    f.release(); await Promise.resolve()
    const deleting = f.controller.rate('message', 'positive', null, owner)
    await f.controller.ensure(); await Promise.resolve()
    expect(f.request.mock.calls.map(call => call[1])).toEqual(['GET', 'DELETE'])
    releaseOwner(); await Promise.resolve()
    const releaseNext = f.controller.subscribe(() => undefined, next)
    const newer = { ...item, rating: 'negative', version: '22222222-2222-4222-8222-222222222222', updatedAt: 2 }
    write.resolve(status === 409
      ? result({ code: 'message_feedback_version_conflict', message: 'Message feedback changed. Review the current rating and submit again.', current: newer }, 409)
      : result({ code: 'unavailable' }, 503))
    await deleting
    expect(f.controller.getSnapshot().pending).toBe(false)
    expect(f.controller.getSnapshot().error).toBeNull()
    expect(f.controller.getSnapshot().errorOwner).toBeNull()
    expect(f.controller.getSnapshot().items.message).toEqual(status === 409 ? newer : item)
    releaseNext(); releaseKeeper()
  })
  it.each([409, 503])('preserves useful DELETE errors (%i) for an active owner', async status => {
    const newer = { ...item, version: '22222222-2222-4222-8222-222222222222', updatedAt: 2 }
    const f = fixture(async (_path, method) => method === 'GET' ? result({ items: [item] }) : status === 409
      ? result({ code: 'message_feedback_version_conflict', message: 'Message feedback changed. Review the current rating and submit again.', current: newer }, 409)
      : result({ code: 'unavailable' }, 503))
    const owner = Symbol('active-row')
    const releaseOwner = f.controller.subscribe(() => undefined, owner)
    await f.controller.rate('message', 'positive', null, owner)
    expect(f.controller.getSnapshot()).toMatchObject({ pending: false, error: status === 409 ? 'conflict' : 'save', errorMessageId: 'message', errorOwner: owner })
    expect(f.controller.getSnapshot().items.message).toEqual(status === 409 ? newer : item)
    releaseOwner(); await Promise.resolve()
    expect(f.controller.getSnapshot().error).toBeNull()
    f.release()
  })
  it('does not lose a reconnect arriving during the first lazy read', async () => {
    const first = deferred<RuntimeRequestResult>()
    let reads = 0
    const f = fixture(async () => ++reads === 1 ? first.promise : result({ items: [item] }))
    const pending = f.controller.ensure(); await Promise.resolve(); f.running(); f.running()
    expect(reads).toBe(1)
    first.resolve(result({ items: [] })); await pending; await f.controller.ensure()
    expect(reads).toBe(2); expect(f.controller.getSnapshot().items.message).toEqual(item)
    f.release()
  })
  it('retires an unmounted modal owner and keeps late mutation out of a new surface', async () => {
    const write = deferred<RuntimeRequestResult>()
    const f = fixture(async (_path, method) => method === 'GET' ? result({ items: [] }) : write.promise)
    const owner = Symbol('first-row'), other = Symbol('other-row')
    const releaseOwner = f.controller.subscribe(() => undefined, owner)
    const releaseOther = f.controller.subscribe(() => undefined, other)
    await f.controller.rate('message', 'positive', null, owner)
    const submitting = f.controller.submit(); await Promise.resolve()
    releaseOwner(); await Promise.resolve()
    expect(f.controller.getSnapshot().dialog).toBeNull()
    write.resolve(result(item)); await submitting
    expect(f.controller.getSnapshot().items.message).toEqual(item)
    expect(f.controller.getSnapshot().acknowledgement).toBeNull()
    await f.controller.rate('message', 'negative', null, other)
    expect(f.controller.getSnapshot().dialog?.owner).toBe(other)
    releaseOther(); f.release()
  })
  it('reads lazily once, retracts the same rating and opens an empty form for another rating', async () => {
    const f = fixture(async (_path, method) => method === 'GET' ? result({ items: [item] }) : result({ absent: true }))
    expect(f.request).not.toHaveBeenCalled()
    await Promise.all([f.controller.ensure(), f.controller.ensure()])
    expect(f.request).toHaveBeenCalledTimes(1)
    await f.controller.rate('message', 'negative', null)
    expect(f.controller.getSnapshot().dialog).toMatchObject({ rating: 'negative', note: '' })
    expect(f.request).toHaveBeenCalledTimes(1)
    f.controller.close()
    await f.controller.rate('message', 'positive', null)
    expect(f.request.mock.calls[1]).toEqual(['/v1/threads/thread/message-feedback/message', 'DELETE', JSON.stringify({ ifVersion: version })])
    expect(f.controller.getSnapshot().items.message).toBeUndefined()
    f.release()
  })
  it('freezes submit, preserves the failed draft and does not duplicate a pending write', async () => {
    const write = deferred<RuntimeRequestResult>()
    const f = fixture(async (_path, method) => method === 'GET' ? result({ items: [] }) : write.promise)
    await f.controller.rate('message', 'positive', null)
    f.controller.edit({ note: '  optional details  ', category: 'task-result' })
    const first = f.controller.submit()
    await Promise.resolve()
    await f.controller.submit(); f.controller.edit({ note: 'overwritten' }); f.controller.close()
    expect(f.request).toHaveBeenCalledTimes(2)
    expect(f.controller.getSnapshot().dialog?.note).toBe('  optional details  ')
    write.resolve(result({ code: 'unavailable' }, 503)); await first
    expect(f.controller.getSnapshot()).toMatchObject({ pending: false, error: 'save', dialog: { note: '  optional details  ', category: 'task-result' } })
    f.release()
  })
  it('applies conflict current and preserves the draft for explicit retry', async () => {
    const newer = { ...item, rating: 'negative', version: '22222222-2222-4222-8222-222222222222', updatedAt: 2 }
    let writes = 0
    const f = fixture(async (_path, method, body) => {
      if (method === 'GET') return result({ items: [] })
      if (++writes === 1) return result({ code: 'message_feedback_version_conflict', message: 'Message feedback changed. Review the current rating and submit again.', current: newer }, 409)
      expect(JSON.parse(body!)).toMatchObject({ ifVersion: newer.version, note: 'kept draft' })
      return result({ ...newer, rating: 'positive', note: 'kept draft' })
    })
    await f.controller.rate('message', 'positive', null); f.controller.edit({ note: 'kept draft' })
    await f.controller.submit()
    expect(writes).toBe(1)
    expect(f.controller.getSnapshot()).toMatchObject({ error: 'conflict', items: { message: newer }, dialog: { note: 'kept draft' } })
    await f.controller.submit(); expect(writes).toBe(2); expect(f.controller.getSnapshot().dialog).toBeNull()
    f.release()
  })
  it('serializes reconnect reconciliation after a mutation and ignores completion after unmount', async () => {
    const write = deferred<RuntimeRequestResult>()
    const f = fixture(async (_path, method) => method === 'GET' ? result({ items: [] }) : write.promise)
    f.running(); expect(f.request).not.toHaveBeenCalled()
    await f.controller.rate('message', 'positive', null)
    const submission = f.controller.submit(); await Promise.resolve(); f.running()
    expect(f.request).toHaveBeenCalledTimes(2)
    write.resolve(result(item)); await submission; await f.controller.ensure()
    expect(f.request.mock.calls.map(call => call[1])).toEqual(['GET', 'PUT', 'GET'])
    expect(f.controller.getSnapshot().items).toEqual({})
    f.release(); await Promise.resolve(); f.running()
    expect(f.request).toHaveBeenCalledTimes(3)
  })
  it('rejects a byte oversized draft without submitting and never reports a failed read as empty', async () => {
    const f = fixture(async () => result({}, 503))
    await f.controller.rate('message', 'positive', null)
    expect(f.controller.getSnapshot()).toMatchObject({ ensured: false, dialog: { rating: 'positive', note: '' } })
    f.controller.close()
    f.request.mockImplementation(async () => result({ items: [] }))
    await f.controller.rate('message', 'positive', null); f.controller.edit({ note: '界'.repeat(2731) })
    const count = f.request.mock.calls.length; await f.controller.submit()
    expect(f.controller.getSnapshot().error).toBe('note-too-large'); expect(f.request).toHaveBeenCalledTimes(count)
    f.release()
  })
})
