import { describe, expect, it, vi } from 'vitest'
import { createCanvasController } from './canvas-controller'
import type { PluginPackageHostRequest, PluginPackageHostResponse, PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

const id = 'a'.repeat(48), hash = 'b'.repeat(64)
function fixture() {
  let current = true
  let pkg: PluginPackageView = { packageId: 'analytix-canvas', packageVersion: '0.1.0', displayName: 'Canvas', origin: 'development-source', publishable: false, materialized: true,
    generationId: hash, activationState: 'recorded', desiredState: 'enabled', activationRevision: 1, activationId: 'c'.repeat(64), available: true,
    operations: ['open-object', 'read-object', 'close-object', 'proposal-apply', 'object-recovery'] }
  const document = { sessionId: id, objectId: hash, threadId: 'thread-main', kind: 'canvas' as const, path: '/workspace/diagram.canvas', revision: hash, content: 'e30=' }
  let respond: (request: PluginPackageHostRequest) => Promise<PluginPackageHostResponse> = async request => request.action === 'list'
    ? { ok: true, packages: [pkg] } : { ok: true, output: request.action === 'invoke' && request.operation === 'close-object' ? { ok: true, closed: true } : { ok: true, document } }
  const packageHost = vi.fn((request: PluginPackageHostRequest) => respond(request))
  const controller = createCanvasController({ packageHost, current: () => current })
  const open = { operation: 'open-object' as const, threadId: 'thread-main', kind: 'canvas' as const, object: { workspace: '/workspace', path: 'diagram.canvas' } }
  return { controller, packageHost, document, open, setCurrent: (v: boolean) => { current = v }, changePackage: (patch: Partial<PluginPackageView>) => { pkg = { ...pkg, ...patch } }, respond: (fn: typeof respond) => { respond = fn }, getPackage: () => pkg }
}

describe('Canvas Main ownership boundary', () => {
  it('rejects malformed activation metadata before invoking the adapter', async () => {
    const invalidPackages: Array<Partial<PluginPackageView> | { activationState: string }> = [
      { activationState: 'enabled' },
      { activationId: undefined },
      { activationRevision: 0 },
      { generationId: '' },
      { desiredState: 'disabled' },
      { materialized: false }
    ]
    for (const patch of invalidPackages) {
      const f = fixture()
      f.respond(async () => ({ ok: true, packages: [{ ...f.getPackage(), ...patch }] }) as PluginPackageHostResponse)
      expect(await f.controller.request(f.open)).toEqual({ ok: false, code: 'unavailable' })
      expect(f.packageHost.mock.calls.some(([request]) => request.action === 'invoke')).toBe(false)
    }
  })
  it('rejects ambiguous duplicate package identities before invoking the adapter', async () => {
    const f = fixture()
    f.respond(async () => ({ ok: true, packages: [f.getPackage(), f.getPackage()] }))
    expect(await f.controller.request(f.open)).toEqual({ ok: false, code: 'unavailable' })
    expect(f.packageHost.mock.calls.some(([request]) => request.action === 'invoke')).toBe(false)
  })
  it('rejects a malformed host envelope even when its inner document looks valid', async () => {
    const f = fixture()
    f.respond(async request => request.action === 'list'
      ? { ok: true, packages: [f.getPackage()] }
      : { ok: true, output: { ok: true, document: f.document }, unexpected: 'untrusted' })
    expect(await f.controller.request(f.open)).toEqual({ ok: false, code: 'unavailable' })
  })
  it('opens through the finite Host and rejects unowned or wrong-thread sessions', async () => {
    const f = fixture()
    expect(await f.controller.request(f.open)).toEqual({ ok: true, document: f.document })
    const invokes = () => f.packageHost.mock.calls.filter(([r]) => r.action === 'invoke').length
    expect(invokes()).toBe(1)
    for (const request of [ { operation: 'read-object', sessionId: 'd'.repeat(48), threadId: 'thread-main' }, { operation: 'read-object', sessionId: id, threadId: 'another-thread' } ]) {
      expect(await f.controller.request(request)).toEqual({ ok: false, code: 'unavailable' })
    }
    expect(invokes()).toBe(1)
    expect(await f.controller.request({ operation: 'read-object', sessionId: id, threadId: 'thread-main', path: '/private' })).toEqual({ ok: false, code: 'invalid_request' })
  })
  it('withdraws sessions after activation changes and does not send an apply', async () => {
    const f = fixture(); await f.controller.request(f.open)
    f.changePackage({ activationRevision: 3 })
    expect(await f.controller.request({ operation: 'proposal-apply', sessionId: id, threadId: 'thread-main', proposalId: 'c'.repeat(48) })).toEqual({ ok: false, code: 'unavailable' })
    expect(f.packageHost.mock.calls.some(([r]) => r.action === 'invoke' && r.operation === 'proposal-apply')).toBe(false)
  })
  it('does not deliver document bytes to a replacement frame and releases the abandoned handle', async () => {
    const f = fixture()
    f.respond(async request => {
      if (request.action === 'list') return { ok: true, packages: [f.getPackage()] }
      if (request.action === 'invoke' && request.operation === 'open-object') { f.setCurrent(false); return { ok: true, output: { ok: true, document: f.document } } }
      return { ok: true, output: { ok: true, closed: true } }
    })
    expect(await f.controller.request(f.open)).toEqual({ ok: false, code: 'unavailable' })
    expect(f.packageHost.mock.calls.some(([r]) => r.action === 'invoke' && r.operation === 'close-object')).toBe(true)
  })
  it('rejects malformed success or unrelated recovery and preserves explicit unknown results', async () => {
    const f = fixture(); await f.controller.request(f.open)
    f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] } : { ok: true, output: { ok: true, receipt: { operationId: 'native_save_' + hash, revision: hash, status: 'committed', savedAt: '' } } })
    const apply = { operation: 'proposal-apply', sessionId: id, threadId: 'thread-main', proposalId: 'c'.repeat(48) }
    expect(await f.controller.request(apply)).toEqual({ ok: false, code: 'unavailable' })
    const unknown = { ok: false, code: 'unavailable', receipt: { operationId: 'native_save_' + hash, revision: '', status: 'unknown', savedAt: '' } }
    f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] } : { ok: true, output: unknown })
    expect(await f.controller.request(apply)).toEqual(unknown)
  })
  it('binds reads to the opened object, not only its session and thread', async () => {
    for (const patch of [{ objectId: 'd'.repeat(64) }, { path: '/other/diagram.canvas' }, { kind: 'png' as const }]) {
      const f = fixture(); await f.controller.request(f.open)
      f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] }
        : { ok: true, output: { ok: true, document: { ...f.document, ...patch } } })
      expect(await f.controller.request({ operation: 'read-object', sessionId: id, threadId: 'thread-main' })).toEqual({ ok: false, code: 'unavailable' })
    }
  })
  it('permits a Core-validated new revision of the same object', async () => {
    const f = fixture(); await f.controller.request(f.open)
    const document = { ...f.document, revision: 'd'.repeat(64), content: 'e30K' }
    f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] }
      : { ok: true, output: { ok: true, document } })
    expect(await f.controller.request({ operation: 'read-object', sessionId: id, threadId: 'thread-main' })).toEqual({ ok: true, document })
  })
  it('cannot replace an existing owner with a colliding open response', async () => {
    const f = fixture(); await f.controller.request(f.open)
    f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] }
      : { ok: true, output: { ok: true, document: { ...f.document, threadId: 'thread-b' } } })
    expect(await f.controller.request({ ...f.open, threadId: 'thread-b' })).toEqual({ ok: false, code: 'unavailable' })
    const count = f.packageHost.mock.calls.length
    expect(await f.controller.request({ operation: 'read-object', sessionId: id, threadId: 'thread-b' })).toEqual({ ok: false, code: 'unavailable' })
    expect(f.packageHost.mock.calls).toHaveLength(count)
  })

  it('keeps captured selection binding exact and rejects model-only operations from Renderer', async () => {
    for (const patch of [{}, { sessionId: 'e'.repeat(48) }, { threadId: 'other-thread' }, { baseRevision: 'd'.repeat(64) }, { selectedIds: ['other'] }]) {
      const f = fixture(); await f.controller.request(f.open)
      f.changePackage({ operations: [...f.getPackage().operations, 'capture-selection', 'validate-selection'] })
      const selection = { scopeId: 'd'.repeat(48), sessionId: id, threadId: 'thread-main', baseRevision: hash, selectedIds: ['node-a'], editable: true, ...patch }
      f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] } : { ok: true, output: { ok: true, selection } })
      const result = await f.controller.request({ operation: 'capture-selection', sessionId: id, threadId: 'thread-main', baseRevision: hash, selectedIds: ['node-a'] })
      expect(result.ok).toBe(Object.keys(patch).length === 0)
      const calls = f.packageHost.mock.calls.length
      expect(await f.controller.request({ operation: 'model-selection-read', scopeId: 'd'.repeat(48), threadId: 'thread-main' })).toEqual({ ok: false, code: 'invalid_request' })
      expect(f.packageHost.mock.calls).toHaveLength(calls)
    }
  })
  it('validates a quoted scope through Core but does not use a mismatched returned handle', async () => {
    const f = fixture(); await f.controller.request(f.open)
    f.changePackage({ operations: [...f.getPackage().operations, 'validate-selection'] })
    const selection = { scopeId: 'd'.repeat(48), sessionId: id, threadId: 'thread-main', baseRevision: hash, selectedIds: ['node-a'], editable: true }
    f.respond(async request => request.action === 'list' ? { ok: true, packages: [f.getPackage()] } : { ok: true, output: { ok: true, selection } })
    const input = { operation: 'validate-selection', sessionId: id, threadId: 'thread-main', scopeId: selection.scopeId }
    expect(await f.controller.request(input)).toEqual({ ok: true, selection })
    expect(await f.controller.request({ ...input, scopeId: 'f'.repeat(48) })).toEqual({ ok: false, code: 'unavailable' })
  })
  it('permits only same-owner close after a generation change, not reading or applying', async () => {
    const f = fixture(); await f.controller.request(f.open)
    f.changePackage({ generationId: 'd'.repeat(64), activationRevision: 3 })
    expect(await f.controller.request({ operation: 'read-object', sessionId: id, threadId: 'thread-main' })).toEqual({ ok: false, code: 'unavailable' })
    expect(await f.controller.request({ operation: 'close-object', sessionId: id, threadId: 'foreign-thread' })).toEqual({ ok: false, code: 'unavailable' })
    expect(await f.controller.request({ operation: 'close-object', sessionId: id, threadId: 'thread-main' })).toEqual({ ok: true, closed: true })
    const count = f.packageHost.mock.calls.length
    expect(await f.controller.request({ operation: 'read-object', sessionId: id, threadId: 'thread-main' })).toEqual({ ok: false, code: 'unavailable' })
    expect(f.packageHost.mock.calls).toHaveLength(count)
  })

})
