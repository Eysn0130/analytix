import { describe, expect, it, vi } from 'vitest'
import { createCanvasController } from './canvas-controller'
import type { PluginPackageHostRequest, PluginPackageHostResponse, PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

const id = 'a'.repeat(48), hash = 'b'.repeat(64)
function fixture() {
  let current = true
  let pkg: PluginPackageView = { packageId: 'analytix-canvas', packageVersion: '0.1.0', displayName: 'Canvas', origin: 'development-source', publishable: false, materialized: true,
    generationId: hash, activationState: 'enabled', desiredState: 'enabled', activationRevision: 1, activationId: 'c'.repeat(64), available: true,
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
})
