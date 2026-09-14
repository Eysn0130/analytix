import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { createNativeOfficeController, type NativeOfficeEngineRequest, type NativeOfficeSurface } from './native-office-controller'
import { nativeOfficeRequestSchema, nativeOfficeResponseSchema, type NativeOfficeView } from '../../shared/native-office'
import type { PluginPackageHostRequest, PluginPackageHostResponse, PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

const bytes = new Uint8Array([80, 75, 3, 4, 1, 2, 3])
const revision = createHash('sha256').update(bytes).digest('hex')
const objectId = 'd'.repeat(64), sessionId = 'e'.repeat(48)
const bounds = { x: 10, y: 20, width: 900, height: 600 }
const open = { action: 'open', workspace: '/workspace', path: 'report.docx', bounds }
const state = () => ({ documentId: objectId, version: revision, kind: 'docx', changeSequence: 0, acknowledgedSequence: 0, dirty: false })
const selection = () => ({ documentId: objectId, version: revision, changeSequence: 0, kind: 'text', scope: 'current-view-text-only; no verified structural offset or durable anchor', text: 'local selection' })

function harness() {
  let listener: (event: unknown) => void = () => {}
  let counter = 0, destroyed = 0, hidden = 0, attached = 0, openOperationId = ''
  const calls: NativeOfficeEngineRequest[] = [], hostCalls: PluginPackageHostRequest[] = []
  const views: Array<NativeOfficeView | null> = [], order: string[] = []
  const pkg: PluginPackageView = { packageId: 'analytix-documents', packageVersion: '1.0.0', displayName: 'Documents',
    origin: 'development-source', publishable: false, materialized: true, generationId: 'a'.repeat(64),
    activationState: 'recorded', desiredState: 'enabled', activationRevision: 1, activationId: 'b'.repeat(64),
    available: true, operations: ['open-object', 'close-object'] }
  let documentOutput: unknown = { ok: true, document: { sessionId, objectId, path: '/workspace/report.docx', revision, content: Buffer.from(bytes).toString('base64') } }
  let transform: (request: NativeOfficeEngineRequest, response: Record<string, unknown>) => unknown = (_request, reply) => reply
  let closeOutput: unknown = { ok: true, closed: true }
  const surface: NativeOfficeSurface = {
    async attach() { attached++ }, hide() { hidden++ }, destroy() { destroyed++; order.push('destroy') },
    async request(request) {
      calls.push(request); order.push(`engine:${request.command}`)
      if (!['open', 'captureSelection', 'close'].includes(request.command)) throw new Error('mutation sent to read-only engine')
      const reply: Record<string, unknown> = { type: 'result', channel: 'private-surface-channel', command: request.command,
        operationId: request.operationId, documentId: request.documentId, version: request.version, ok: true }
      if (request.command === 'open') openOperationId = request.operationId
      if (request.command !== 'close') reply.state = state()
      if (request.command === 'captureSelection') reply.selection = selection()
      return transform(request, reply)
    }
  }
  const controller = createNativeOfficeController({
    async packageHost(request): Promise<PluginPackageHostResponse> {
      hostCalls.push(request)
      if (request.action === 'list') return { ok: true, packages: [structuredClone(pkg)] }
      if (request.action !== 'invoke') throw new Error('must never enable a plugin')
      order.push(`core:${request.operation}`)
      if (request.operation === 'open-object') return { ok: true, output: documentOutput }
      if (request.operation === 'close-object') return { ok: true, output: closeOutput }
      throw new Error('save protocol invoked by preview')
    },
    createSurface(onEvent) { listener = onEvent; return surface },
    onChange(view) { views.push(view) }, newOperationId: () => `operation_${++counter}`
  })
  const emit = (value: unknown) => listener(value)
  return {
    controller, calls, hostCalls, views, pkg, order, emit,
    changed: () => emit({ type: 'changed', documentId: objectId, version: revision, operationId: openOperationId,
      state: { ...state(), changeSequence: 1, dirty: true } }),
    getOpenOperation: () => openOperationId,
    setDocument: (value: unknown) => { documentOutput = value }, setClose: (value: unknown) => { closeOutput = value },
    setTransform: (value: typeof transform) => { transform = value },
    getDestroyed: () => destroyed, getHidden: () => hidden, getAttached: () => attached
  }
}
function expectReadOnly(h: ReturnType<typeof harness>) {
  expect(h.calls.every(call => ['open', 'captureSelection', 'close'].includes(call.command))).toBe(true)
  expect(h.hostCalls.every(call => call.action === 'list' || (call.action === 'invoke' && ['open-object', 'close-object'].includes(call.operation)))).toBe(true)
  expect(h.views.every(view => view === null || (!view.dirty && ['loading', 'ready', 'error'].includes(view.status)))).toBe(true)
}

describe('Main-owned read-only native Office preview', () => {
  it('opens through the pinned private package and exposes no native bytes/session', async () => {
    const h = harness()
    const result = await h.controller.request(open)
    expect(result).toMatchObject({ ok: true, view: { status: 'ready', dirty: false, revision } })
    expect(h.hostCalls.find(call => call.action === 'invoke')).toMatchObject({ packageId: 'analytix-documents', generationId: 'a'.repeat(64), expectedRevision: 1, operation: 'open-object', input: { object: { workspace: '/workspace', path: 'report.docx' } } })
    expect(h.calls).toHaveLength(1)
    expect(h.calls[0]).toMatchObject({ command: 'open', kind: 'docx', bytes })
    expect(JSON.stringify(h.views)).not.toContain(Buffer.from(bytes).toString('base64'))
    expect(JSON.stringify(h.views)).not.toContain(sessionId)
    expect(nativeOfficeResponseSchema.safeParse(result).success).toBe(true)
    expectReadOnly(h)
  })

  it.each(['save', 'bold', 'undo', 'redo'])('rejects %s before reaching either Core or engine', async action => {
    const h = harness()
    expect(await h.controller.request({ action })).toMatchObject({ ok: false, error: 'invalid_request' })
    expect(h.hostCalls).toHaveLength(0)
    await h.controller.request(open)
    const hostCount = h.hostCalls.length, engineCount = h.calls.length
    expect(await h.controller.request({ action })).toMatchObject({ ok: false, error: 'invalid_request', view: { status: 'ready', dirty: false } })
    expect(h.hostCalls).toHaveLength(hostCount); expect(h.calls).toHaveLength(engineCount)
    expectReadOnly(h)
  })

  it('ignores save-requested shortcuts and uses no Core operation status protocol', async () => {
    const h = harness(); await h.controller.request(open)
    h.emit({ type: 'save-requested', operationId: 'save-from-surface', documentId: objectId, version: revision })
    h.emit({ type: 'save-requested', operationId: 'save-from-surface', documentId: objectId, version: revision })
    expect(await h.controller.request({ action: 'status' })).toMatchObject({ ok: true, view: { status: 'ready' } })
    expect(h.calls.map(call => call.command)).toEqual(['open'])
    expectReadOnly(h)
  })

  it('captures only a bound read-only selection and keeps hide/bounds separate from close', async () => {
    const h = harness(); await h.controller.request(open)
    expect(await h.controller.request({ action: 'captureSelection' })).toMatchObject({ ok: true, view: { selection: selection() } })
    await h.controller.request({ action: 'hide' }); await h.controller.request({ action: 'bounds', bounds })
    expect(h.getHidden()).toBe(1); expect(h.getAttached()).toBe(2); expect(h.getDestroyed()).toBe(0)
    expect(h.calls.map(call => call.command)).toEqual(['open', 'captureSelection'])
    expectReadOnly(h)
  })

  it('fails closed on changed events and can close without a save/discard confirmation', async () => {
    const h = harness(); await h.controller.request(open); h.changed()
    expect(h.controller.getView()).toMatchObject({ status: 'error', dirty: false, error: 'invalid_response' })
    expect(await h.controller.request({ action: 'captureSelection' })).toMatchObject({ ok: false })
    expect(await h.controller.request({ action: 'close' })).toEqual({ ok: true, view: null })
    expect(h.getDestroyed()).toBe(1)
    expect(h.hostCalls.filter(call => call.action === 'invoke' && call.operation === 'close-object')).toHaveLength(1)
    expectReadOnly(h)
  })

  it.each(['open', 'captureSelection'])('rejects dirty or advanced sequence in %s result', async command => {
    for (const dirtyState of [{ ...state(), changeSequence: 1, dirty: true }, { ...state(), changeSequence: 1, acknowledgedSequence: 1 }]) {
      const h = harness()
      if (command !== 'open') await h.controller.request(open)
      h.setTransform((request, reply) => request.command === command ? { ...reply, state: dirtyState } : reply)
      expect(await h.controller.request(command === 'open' ? open : { action: command })).toMatchObject({ ok: false, error: 'invalid_response' })
      expect(await h.controller.request({ action: 'close' })).toEqual({ ok: true, view: null })
      expect(h.getDestroyed()).toBe(1)
      expectReadOnly(h)
    }
  })

  it('rejects changed events during loading even if a later open response claims clean', async () => {
    const h = harness()
    h.setTransform((request, reply) => { if (request.command === 'open') h.changed(); return reply })
    expect(await h.controller.request(open)).toMatchObject({ ok: false, view: null })
    expect(h.getDestroyed()).toBe(1)
    expectReadOnly(h)
  })

  it('ignores foreign/stale selection events but fails closed on invalid bound envelopes', async () => {
    const h = harness(); await h.controller.request(open)
    for (const changed of [{ documentId: 'f'.repeat(64) }, { version: 'f'.repeat(64) }]) {
      h.emit({ type: 'changed', operationId: h.getOpenOperation(), documentId: objectId, version: revision, state: state(), ...changed })
    }
    expect(h.controller.getView()).toMatchObject({ status: 'ready' })
    h.emit({ type: 'selection', operationId: h.getOpenOperation(), documentId: objectId, version: revision, selection: { ...selection(), documentId: 'f'.repeat(64) } })
    expect(h.controller.getView()).toMatchObject({ status: 'error', error: 'invalid_response' })
    await h.controller.request({ action: 'close' }); expectReadOnly(h)
  })

  it('revalidates availability and same generation without enabling the package', async () => {
    const h = harness(); await h.controller.request(open)
    h.pkg.available = false; h.pkg.desiredState = 'disabled'; h.pkg.activationRevision = 2
    expect(await h.controller.request({ action: 'captureSelection' })).toMatchObject({ ok: false, error: 'package_unavailable' })
    expect(h.calls).toHaveLength(1)
    h.pkg.available = true; h.pkg.desiredState = 'enabled'; h.pkg.activationRevision = 3; h.pkg.generationId = 'f'.repeat(64)
    expect(await h.controller.request({ action: 'status' })).toMatchObject({ ok: false, error: 'package_changed' })
    h.pkg.generationId = 'a'.repeat(64)
    expect(await h.controller.request({ action: 'captureSelection' })).toMatchObject({ ok: true, view: { status: 'ready' } })
    await h.controller.request({ action: 'close' })
    expect(h.hostCalls.find(call => call.action === 'invoke' && call.operation === 'close-object')).toMatchObject({ expectedRevision: 3, generationId: 'a'.repeat(64) })
    expectReadOnly(h)
  })

  it('serializes open/close and revokes Core only after closing the preview engine', async () => {
    const h = harness()
    const [opened, closed] = await Promise.all([h.controller.request(open), h.controller.request({ action: 'close' })])
    expect(opened.ok).toBe(true); expect(closed).toEqual({ ok: true, view: null })
    expect(h.order).toEqual(['core:open-object', 'engine:open', 'engine:close', 'core:close-object', 'destroy'])
    expect(h.calls.at(-1)).toMatchObject({ command: 'close', discard: true })
    expectReadOnly(h)
  })

  it('cleans failed/unavailable previews without a save confirmation or raw errors', async () => {
    const h = harness(); await h.controller.request(open)
    h.emit({ type: 'fatal', code: 'office-engine-closed-unknown' })
    expect(h.controller.getView()).toMatchObject({ status: 'error', error: 'engine_unavailable' })
    expect(await h.controller.request({ action: 'close' })).toEqual({ ok: true, view: null })
    expect(h.getDestroyed()).toBe(1)
    const failed = harness(); await failed.controller.request(open)
    failed.setTransform((request, reply) => { if (request.command === 'close') throw new Error('private native body'); return reply })
    expect(await failed.controller.request({ action: 'close' })).toEqual({ ok: true, view: null })
    expect(JSON.stringify(failed.views)).not.toContain('private native body')
    expectReadOnly(h); expectReadOnly(failed)
  })

  it('validates private document, base64 and engine correlation before accepting a preview', async () => {
    for (const change of [{ path: '/workspace/other.docx' }, { revision: 'f'.repeat(64) }, { content: Buffer.from(bytes).toString('base64') + '\n' }]) {
      const h = harness()
      h.setDocument({ ok: true, document: { objectId, sessionId, path: '/workspace/report.docx', revision, content: Buffer.from(bytes).toString('base64'), ...change } })
      expect(await h.controller.request(open)).toMatchObject({ ok: false, error: 'invalid_response', view: null })
      expect(h.calls).toHaveLength(0)
      expectReadOnly(h)
    }
    for (const change of [{ operationId: 'wrong-operation' }, { documentId: 'f'.repeat(64) }, { bytes }, { command: 'export' }]) {
      const h = harness(); h.setTransform((_request, reply) => ({ ...reply, ...change }))
      expect(await h.controller.request(open)).toMatchObject({ ok: false, error: 'invalid_response', view: null })
      expect(h.getDestroyed()).toBe(1); expectReadOnly(h)
    }
  })

  it('rejects Renderer bytes, URLs and arbitrary engine commands', async () => {
    const h = harness()
    for (const payload of [{ ...open, bytes }, { ...open, path: 'https://remote.invalid/report.docx' }, { ...open, url: 'https://remote.invalid' }, { action: 'captureSelection', uno: '.uno:Save' }]) {
      expect(nativeOfficeRequestSchema.safeParse(payload).success).toBe(false)
      expect(await h.controller.request(payload)).toMatchObject({ ok: false, error: 'invalid_request' })
    }
    expect(h.hostCalls).toHaveLength(0); expect(h.calls).toHaveLength(0)
  })

  it('reuses the same document without opening another Core session', async () => {
    const h = harness(); await h.controller.request(open)
    expect(await h.controller.request(open)).toMatchObject({ ok: true, view: { status: 'ready' } })
    expect(h.calls.map(call => call.command)).toEqual(['open'])
    expect(h.hostCalls.filter(call => call.action === 'invoke' && call.operation === 'open-object')).toHaveLength(1)
    expect(h.getAttached()).toBe(2)
    await h.controller.request({ action: 'close' }); expectReadOnly(h)
  })

  it('does not restore a ready preview from an in-flight result after owner disposal', async () => {
    const h = harness(); await h.controller.request(open)
    let complete: () => void = () => {}
    let entered: () => void = () => {}
    const started = new Promise<void>(resolve => { entered = resolve })
    h.setTransform((request, reply) => request.command === 'captureSelection' ? new Promise(resolve => {
      complete = () => resolve(reply); entered()
    }) : reply)
    const capture = h.controller.request({ action: 'captureSelection' })
    await started
    const start = h.views.length, shutdown = h.controller.destroy()
    complete()
    expect((await capture).ok).toBe(false)
    await shutdown
    expect(h.controller.getView()).toBeNull(); expect(h.getDestroyed()).toBe(1)
    expect(h.views.slice(start).some(view => view?.status === 'ready')).toBe(false)
    expectReadOnly(h)
  })

  it('destroys an owner once and refuses subsequent preview requests', async () => {
    const h = harness(); await h.controller.request(open)
    await h.controller.destroy(); await h.controller.destroy()
    expect(h.controller.getView()).toBeNull(); expect(h.getDestroyed()).toBe(1)
    expect(await h.controller.request(open)).toMatchObject({ ok: false, error: 'unavailable' })
    expectReadOnly(h)
  })
})
