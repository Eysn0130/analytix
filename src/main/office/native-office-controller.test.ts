import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import { createNativeOfficeController, type NativeOfficeEngineRequest, type NativeOfficeSurface } from './native-office-controller'
import { nativeOfficeRequestSchema, nativeOfficeResponseSchema, type NativeOfficeView, type NativeOfficeFormatChoice, type NativeOfficeFormatting } from '../../shared/native-office'
import type { PluginPackageHostRequest, PluginPackageHostResponse, PluginPackageView } from '../../../packages/runtime/src/contracts/plugin-package-host'

vi.mock('../logger', () => ({ logNativeOfficeFailure: vi.fn() }))
import { logNativeOfficeFailure } from '../logger'

const bytes = new Uint8Array([80, 75, 3, 4, 1, 2, 3])
const revision = createHash('sha256').update(bytes).digest('hex')
const objectId = 'd'.repeat(64), sessionId = 'e'.repeat(48)
const bounds = { x: 10, y: 20, width: 900, height: 600 }
const open = { action: 'open', workspace: '/workspace', path: 'report.docx', bounds }
const state = () => ({ documentId: objectId, version: revision, kind: 'docx', changeSequence: 0, acknowledgedSequence: 0, dirty: false })
const selection = () => ({ documentId: objectId, version: revision, changeSequence: 0, kind: 'text', scope: 'current-view-text-only; no verified structural offset or durable anchor', text: 'local selection' })

function harness(fault?: 'create' | 'attach' | 'open', editable = false, kind: 'docx' | 'pptx' = 'docx', manual = false) {
  let listener: (event: unknown) => void = () => {}
  let counter = 0, created = 0, destroyed = 0, hidden = 0, attached = 0, openOperationId = ''
  let openFailure = fault === 'open'
  const calls: NativeOfficeEngineRequest[] = [], hostCalls: PluginPackageHostRequest[] = []
  const views: Array<NativeOfficeView | null> = [], order: string[] = []
  const pkg: PluginPackageView = { packageId: kind === 'pptx' ? 'analytix-presentations' : 'analytix-documents', packageVersion: '1.0.0', displayName: 'Documents',
    origin: 'development-source', publishable: false, materialized: true, generationId: 'a'.repeat(64),
    activationState: 'recorded', desiredState: 'enabled', activationRevision: 1, activationId: 'b'.repeat(64),
    available: true, operations: ['open-object', 'close-object'] }
  if (editable) pkg.operations.push('commit-object', 'object-status')
  if (manual) pkg.operations.push('begin-manual-edit')
  const manualAuthority = {version:2 as const,origin:'manual' as const,changeId:'8'.repeat(64),baseRevision:revision,saveOperationId:'save_manual_pointer',undoOperationId:'undo_manual_pointer'}
  let manualStarted=false,manualFrozen=false
  const manualState = () => ({version:2 as const,origin:'manual' as const,changeId:manualAuthority.changeId,baseRevision:revision,generation:1,frozen:manualFrozen})
  const formatSample = {fontFamily:'Noto Sans CJK SC',fontSize:12,bold:false,italic:false,underline:false}
  const rangeLease = {generation:1,sequence:0,token:'format_selection'}
  let documentOutput: unknown = { ok: true, document: { sessionId, objectId, path: '/workspace/report.' + kind, revision, content: Buffer.from(bytes).toString('base64') } }
  let transform: (request: NativeOfficeEngineRequest, response: Record<string, unknown>) => unknown = (_request, reply) => reply
  let closeOutput: unknown = { ok: true, closed: true }
  let creationOutput: unknown = { ok:true, creation:{operationId:'create_operation',workspace:'/workspace',path:'/workspace/report.docx',revision,kind:'docx',byteSize:bytes.byteLength,savedAt:'2026-10-15T00:00:00Z'} }
  let pointerCurrent=false
  const presentPointer=vi.fn(async(_target:any):Promise<string|null|NativeOfficeFormatChoice>=>'quote'),dismissPointer=vi.fn(()=>{pointerCurrent=false})
  const createSurface = (): NativeOfficeSurface => {
    let surfaceDestroyed = false
    let playbackGeneration = 0, playbackSequence = 0, running = false, paused = false
    return {
      isPointerCurrent:()=>pointerCurrent, presentPointerActions:presentPointer, dismissPointerActions:dismissPointer,
      async attach() { attached++; if (fault === 'attach') throw Error('/private/office/body-sentinel') }, hide() { hidden++ },
      destroy() { surfaceDestroyed = true; destroyed++; order.push('destroy') },
      async request(request) {
        if (surfaceDestroyed) throw new Error('destroyed fixture surface')
        calls.push(request); order.push(`engine:${request.command}`)
        if (openFailure && request.command === 'open') throw Error('/private/office/body-sentinel')
        if (!['local-view', 'open', 'captureSelection', 'validateSelection', 'close'].includes(request.command) && !(editable && request.command === 'edit') && !(manual && request.command === 'manual-edit')) throw new Error('mutation sent to read-only engine')
        const reply: Record<string, unknown> = { type: 'result', channel: 'private-surface-channel', command: request.command,
          operationId: request.operationId, documentId: request.documentId, version: request.version, ok: true }
        if (request.command === 'open') openOperationId = request.operationId
        if (request.command === 'manual-edit') manualStarted=true
        if (request.command !== 'close') reply.state = { ...state(), kind, ...(manualStarted ? {manual:manualState()} : {}) }
        if (request.command === 'local-view') {
          if (request.action?.startsWith('play-')) {
            if (request.action !== 'play-read' && request.playbackGeneration !== playbackGeneration) throw Error('stale-selection')
            if (request.action === 'play-start' || request.action === 'play-current') { playbackGeneration++; running = true; paused = false }
            if (request.action === 'play-end') { playbackGeneration++; running = false; paused = false }
            if (request.action === 'play-pause') paused = true
            if (request.action === 'play-resume') paused = false
          }
          reply.view = {zoom:request.zoom ?? 100,zoomType:3,fit:request.action === 'fit',page:kind === 'pptx' ? 2 : null,pages:kind === 'pptx' ? 4 : null,readOnly:true,modified:false,
            ...(kind === 'pptx' ? {playback:{generation:playbackGeneration,sequence:++playbackSequence,phase:running ? paused ? 'paused' : 'running' : 'idle',slideIndex:running ? 0 : null,slideCount:running ? 3 : null}} : {})}
        } else if (!['close','validateSelection'].includes(request.command)) reply.selection = kind === 'pptx' ? {documentId:objectId,version:revision,changeSequence:0,kind:'unavailable',scope:'engine selection interface unavailable in this view'} : {...selection(),...(manualStarted ? {token:rangeLease.token,rangeLease:{...rangeLease},formatState:{...formatSample}} : {})}
        return transform(request, reply)
      }
    }
  }
  const controller = createNativeOfficeController({
    async packageHost(request): Promise<PluginPackageHostResponse> {
      hostCalls.push(request)
      if (request.action === 'list') return { ok: true, packages: [structuredClone(pkg)] }
      if (request.action !== 'invoke') throw new Error('must never enable a plugin')
      order.push(`core:${request.operation}`)
      if (request.operation === 'open-object') {
        const output = await documentOutput
        if (output instanceof Error) throw output
        return { ok: true, output }
      }
      if (request.operation === 'create-object') {
        const output = await creationOutput
        if (output instanceof Error) throw output
        return {ok:true,output}
      }
      if (request.operation === 'begin-manual-edit' && manual) return {ok:true,output:{ok:true,manual:manualAuthority}}
      if (request.operation === 'close-object') {
        if (closeOutput instanceof Error) throw closeOutput
        return { ok: true, output: closeOutput }
      }
      throw new Error('save protocol invoked by preview')
    },
    createSurface(onEvent) { if (fault === 'create') throw Error('/private/office/body-sentinel'); created++; listener = onEvent; return createSurface() },
    onChange(view) { views.push(view) }, newOperationId: () => `operation_${++counter}`
  })
  const emit = (value: unknown) => listener(value)
  return {
    controller, calls, hostCalls, views, pkg, order, emit, presentPointer,dismissPointer,manualAuthority,rangeLease,formatSample,
    freezeManual:()=>{manualFrozen=true;emit({type:'changed',documentId:objectId,version:revision,operationId:openOperationId,state:{...state(),manual:manualState()}})},setPointerCurrent:(value:boolean)=>{pointerCurrent=value},
    changed: () => emit({ type: 'changed', documentId: objectId, version: revision, operationId: openOperationId,
      state: { ...state(), changeSequence: 1, dirty: true } }),
    getOpenOperation: () => openOperationId,
    setCreation: (value: unknown) => { creationOutput = value },
    setOpenFailure: (value: boolean) => { openFailure = value },
    setDocument: (value: unknown) => { documentOutput = value }, setClose: (value: unknown) => { closeOutput = value },
    setTransform: (value: typeof transform) => { transform = value },
    getCreated: () => created, getDestroyed: () => destroyed, getHidden: () => hidden, getAttached: () => attached
  }
}
function expectReadOnly(h: ReturnType<typeof harness>) {
  expect(h.calls.every(call => ['local-view', 'open', 'captureSelection', 'close'].includes(call.command))).toBe(true)
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
    expect(h.calls.at(-1)).toMatchObject({ command: 'close', discard: false })
    expectReadOnly(h)
  })

  it.each([
    { fault: 'Core refusal', output: { ok: false, code: 'unavailable', message: 'private Core close body' }, error: 'unavailable' },
    { fault: 'invalid Core acknowledgement', output: { ok: true, closed: false }, error: 'invalid_response' },
    { fault: 'Core transport failure', output: new Error('private Core close body'), error: 'unavailable' }
  ])('retains the same cleanup owner after $fault until Core confirms close', async ({ output, error }) => {
    const h = harness(); await h.controller.request(open); h.setClose(output)
    for (const request of [{ action: 'close' }, { action: 'close', objectId }]) {
      expect(await h.controller.request(request)).toMatchObject({ ok: false, error, view: { objectId, revision, dirty: false, status: 'error', error } })
      expect(h.controller.getViews()).toHaveLength(1)
    }
    expect(h.calls.filter(call => call.command === 'close')).toHaveLength(1)
    expect(h.getDestroyed()).toBe(1)
    h.setClose({ ok: true, closed: true })
    expect(await h.controller.request({ action: 'close', objectId })).toEqual({ ok: true, view: null })
    const closes = h.hostCalls.filter(call => call.action === 'invoke' && call.operation === 'close-object')
    expect(closes).toHaveLength(3)
    for (const close of closes) expect(close).toMatchObject({ generationId: 'a'.repeat(64), input: { sessionId } })
    expect(h.controller.getViews()).toEqual([])
    expect(h.calls.filter(call => call.command === 'close')).toHaveLength(1)
    expect(h.getDestroyed()).toBe(1)
    expect(JSON.stringify(h.views)).not.toContain('private Core close body')
    expectReadOnly(h)
  })

  it('isolates a pending Core close from new annotations, keyed operations and late engine events', async () => {
    const h = harness(); await h.controller.request({ ...open, threadId: 'thread-one' })
    h.setClose(new Error('private Core close body'))
    expect(await h.controller.request({ action: 'close' })).toMatchObject({ ok: false, error: 'unavailable' })
    const hostCount = h.hostCalls.length, engineCount = h.calls.length
    for (const request of [
      { action: 'annotationSave', objectId, threadId: 'thread-one', note: 'new note', sourceRevision: revision },
      { action: 'annotationRetry', objectId, threadId: 'thread-one' },
      { action: 'saveStatus', objectId },
      { action: 'proposals', objectId, scopeId: 'f'.repeat(48) },
      { action: 'rejectProposal', objectId, scopeId: 'f'.repeat(48), proposalId: 'c'.repeat(48) }
    ]) expect(await h.controller.request(request)).toMatchObject({ ok: false, error: 'unavailable' })
    h.changed()
    h.emit({ type: 'save-requested', operationId: h.getOpenOperation(), documentId: objectId, version: revision })
    expect(h.controller.getView()).toMatchObject({ objectId, status: 'error', error: 'unavailable', dirty: false })
    expect(h.controller.hasPendingAnnotations()).toBe(false)
    expect(h.hostCalls).toHaveLength(hostCount); expect(h.calls).toHaveLength(engineCount)
    h.setClose({ ok: true, closed: true })
    expect(await h.controller.request({ action: 'close' })).toEqual({ ok: true, view: null })
    expectReadOnly(h)
  })

  it('preserves the dirty veto and retries an approved discard without closing the engine again', async () => {
    const h = harness(undefined, true); await h.controller.request(open)
    expect(await h.controller.request({ action: 'annotate', objectId, revision, expectedChangeSequence: 0 })).toMatchObject({ ok: true })
    h.changed()
    expect(await h.controller.request({ action: 'close' })).toMatchObject({ ok: false, error: 'unsaved_changes', view: { dirty: true } })
    expect(h.calls.filter(call => call.command === 'close')).toHaveLength(0)
    expect(h.hostCalls.some(call => call.action === 'invoke' && call.operation === 'close-object')).toBe(false)
    expect(h.getDestroyed()).toBe(0)
    h.setClose(new Error('private Core close body'))
    expect(await h.controller.request({ action: 'close', discard: true })).toMatchObject({ ok: false, error: 'unavailable', view: { objectId, revision, dirty: true } })
    expect(h.calls.at(-1)).toMatchObject({ command: 'close', discard: true, expectedChangeSequence: 1 })
    const engineCount = h.calls.length, hostCount = h.hostCalls.length
    expect(await h.controller.saveAll()).toMatchObject({ ok: false, error: 'unavailable' })
    expect(h.calls).toHaveLength(engineCount); expect(h.hostCalls).toHaveLength(hostCount)
    h.setClose({ ok: true, closed: true })
    expect(await h.controller.request({ action: 'close', objectId })).toEqual({ ok: true, view: null })
    expect(h.calls.filter(call => call.command === 'close')).toHaveLength(1)
    expect(h.getDestroyed()).toBe(1)
    const closes = h.hostCalls.filter(call => call.action === 'invoke' && call.operation === 'close-object')
    expect(closes).toHaveLength(2)
    for (const close of closes) expect(close).toMatchObject({ input: { sessionId } })
  })

  it('finishes a discarded owner Core close before reopening the same document', async () => {
    const h = harness(undefined, true); await h.controller.request(open)
    await h.controller.request({ action: 'annotate', objectId, revision, expectedChangeSequence: 0 }); h.changed()
    h.setClose(new Error('private Core close body'))
    expect((await h.controller.request({ action: 'close', discard: true })).ok).toBe(false)
    expect(await h.controller.request(open)).toMatchObject({ ok: false, error: 'unavailable', view: { objectId, dirty: true } })
    expect(h.hostCalls.filter(call => call.action === 'invoke' && call.operation === 'open-object')).toHaveLength(1)
    expect(h.controller.getViews()).toHaveLength(1)
    h.setClose({ ok: true, closed: true })
    const start = h.order.length
    expect(await h.controller.request(open)).toMatchObject({ ok: true, view: { objectId, dirty: false, status: 'ready' } })
    expect(h.order.slice(start)).toEqual(['core:close-object', 'core:open-object', 'engine:open'])
    expect(h.calls.filter(call => call.command === 'close')).toHaveLength(1)
    expect(h.getCreated()).toBe(2)
    expect(h.getDestroyed()).toBe(1)
    expect(h.controller.getViews()).toHaveLength(1)
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

it('accepts only closed appearance metadata at the renderer boundary', () => {
  expect(nativeOfficeRequestSchema.safeParse({ ...open, appearance: { theme: 'dark', reducedMotion: true } }).success).toBe(true)
  for (const appearance of [{ theme: 'url(https://outside.invalid)', reducedMotion: false }, { theme: 'light', reducedMotion: 'false' }, { theme: 'light', reducedMotion: false, css: 'body{}' }]) {
    expect(nativeOfficeRequestSchema.safeParse({ ...open, appearance }).success).toBe(false)
  }
})


const viewControl=(control='read',extra={})=>({action:'viewControl',objectId,revision,expectedChangeSequence:0,control,...extra})
it('exposes actual bounded view state through readonly controls without Core edits',async()=>{
  const h=harness();await h.controller.request(open)
  for(const zoom of [25,50,100,200]) {
    const result=await h.controller.request(viewControl('set-zoom',{zoom}))
    expect(nativeOfficeResponseSchema.parse(result)).toMatchObject({ok:true,view:{previewView:{zoom,fit:false,page:null,pages:null},dirty:false}})
  }
  expectReadOnly(h)
  expect(h.hostCalls.some(call=>call.action==='invoke'&&!['open-object','close-object'].includes(call.operation))).toBe(false)
  for(const extra of [{zoom:19},{zoom:401},{zoom:25.5},{path:'/private'},{control:['fit']}]) expect((await h.controller.request(viewControl('set-zoom',{zoom:25,...extra}))).ok).toBe(false)
})
it('rejects old-object controls without marking the current object as failed',async()=>{
  const h=harness();await h.controller.request(open)
  expect(await h.controller.request(viewControl('fit',{objectId:'f'.repeat(64)}))).toMatchObject({ok:false,error:'stale_selection',view:{status:'ready'}})
  expect(h.controller.getViews()[0].error).toBeUndefined()
  expect(h.calls.some(call=>call.command==='local-view')).toBe(false)
})
it('drops view events and results after hide before writing state',async()=>{
  const h=harness();await h.controller.request(open)
  let finish!:(value:unknown)=>void
  h.setTransform((request,reply)=>request.command==='local-view'?new Promise(resolve=>{finish=()=>resolve(reply)}):reply)
  const pending=h.controller.request(viewControl('set-zoom',{zoom:25}))
  await vi.waitFor(()=>expect(h.calls.at(-1)?.command).toBe('local-view'))
  await h.controller.request({action:'hide'})
  h.emit({type:'view',operationId:h.getOpenOperation(),documentId:objectId,version:revision,view:{zoom:25,zoomType:3,fit:false,page:null,pages:null,readOnly:true,modified:false}})
  expect(h.controller.getViews()[0].previewView).toBeUndefined()
  finish(undefined);expect(await pending).toMatchObject({ok:false,error:'stale_selection'})
  expect(h.controller.getViews()[0]).toMatchObject({status:'ready',dirty:false})
  expect(h.controller.getViews()[0].previewView).toBeUndefined()
})
it('unavailable actual view state clears controls while retaining the native preview',async()=>{
  const h=harness();await h.controller.request(open)
  await h.controller.request(viewControl('set-zoom',{zoom:25}))
  h.setTransform(request=>{if(request.command==='local-view')throw Error('view-unavailable');return {}})
  expect(await h.controller.request(viewControl('read'))).toMatchObject({ok:false,error:'unavailable',view:{status:'ready'}})
  expect(h.controller.getViews()[0].previewView).toBeUndefined()
  expect(h.getDestroyed()).toBe(0)
})

const presentationOpen = { ...open, path: 'report.pptx' }
const playbackControl = (control: string, generation = 0) => viewControl(control,
  control === 'play-read' ? {} : {playbackGeneration:generation})
it('projects the real show sequence independently of preview pages and preserves readonly authority', async () => {
  const h = harness(undefined, false, 'pptx')
  await h.controller.request(presentationOpen)
  expect(await h.controller.request(playbackControl('play-read'))).toMatchObject({ok:true,view:{playback:{phase:'idle',generation:0}}})
  expect(await h.controller.request(playbackControl('play-start'))).toMatchObject({ok:true,view:{
    previewView:{page:2,pages:4},playback:{generation:1,phase:'running',slideIndex:0,slideCount:3},dirty:false}})
  for (const control of ['play-next-effect','play-next-slide','play-previous-effect','play-previous-slide']) {
    expect((await h.controller.request(playbackControl(control, 1))).ok).toBe(true)
  }
  expect(h.calls.filter(call => call.command === 'local-view').map(call => call.action)).toEqual([
    'play-read','play-start','play-next-effect','play-next-slide','play-previous-effect','play-previous-slide'])
  expect(await h.controller.request(playbackControl('play-pause', 1))).toMatchObject({view:{playback:{phase:'paused'}}})
  expect(await h.controller.request(playbackControl('play-resume', 1))).toMatchObject({view:{playback:{phase:'running'}}})
  const before = h.calls.length
  expect(await h.controller.request(viewControl('fit'))).toMatchObject({ok:false,error:'unavailable'})
  expect(await h.controller.request({action:'captureSelection'})).toMatchObject({ok:false,error:'unavailable'})
  expect(h.calls).toHaveLength(before)
  expectReadOnly(h)
})
it('rejects obsolete playback generations and older samples without changing the current show', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  await h.controller.request(playbackControl('play-start'))
  const before = h.calls.length
  expect(await h.controller.request(playbackControl('play-next-effect', 0))).toMatchObject({ok:false,error:'stale_selection'})
  expect(h.calls).toHaveLength(before)
  for (const playback of [
    {generation:0,sequence:100,phase:'idle',slideIndex:null,slideCount:null},
    {generation:1,sequence:0,phase:'paused',slideIndex:0,slideCount:3}
  ]) h.emit({type:'view',operationId:h.getOpenOperation(),documentId:objectId,version:revision,
    view:{zoom:25,zoomType:3,fit:false,page:4,pages:4,readOnly:true,modified:false,playback}})
  expect(h.controller.getViews()[0]).toMatchObject({previewView:{zoom:100,page:2},playback:{generation:1,phase:'running'}})
  expect(h.getDestroyed()).toBe(0)
})
it('ends a hidden in-flight native start even when its visible receipt was rejected', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  let finish!: () => void
  h.setTransform((request, reply) => request.action === 'play-start'
    ? new Promise(resolve => { finish = () => resolve(reply) }) : reply)
  const pending = h.controller.request(playbackControl('play-start'))
  await vi.waitFor(() => expect(h.calls.at(-1)?.action).toBe('play-start'))
  await h.controller.request({action:'hide'})
  expect(h.getHidden()).toBe(1)
  finish(); expect(await pending).toMatchObject({ok:false,error:'stale_selection'})
  await h.controller.request({action:'status'})
  expect(h.calls.filter(call => call.command === 'local-view').map(call => call.action)).toEqual(['play-start','play-read','play-end'])
  expect(h.calls.at(-1)).toMatchObject({action:'play-end',playbackGeneration:1})
  expect(h.controller.getViews()[0].playback).toMatchObject({generation:2,phase:'idle'})
  await h.controller.request({action:'bounds',bounds})
  h.emit({type:'view',operationId:h.getOpenOperation(),documentId:objectId,version:revision,
    view:{zoom:100,zoomType:3,fit:false,page:2,pages:4,readOnly:true,modified:false,
      playback:{generation:1,sequence:999,phase:'running',slideIndex:0,slideCount:3}}})
  expect(h.controller.getViews()[0].playback?.phase).toBe('idle')
})
it('ends playback before normal native and Core close without generating or saving an artifact', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  await h.controller.request(playbackControl('play-start'))
  const start = h.order.length
  expect(await h.controller.request({action:'close',objectId})).toMatchObject({ok:true,view:null})
  expect(h.order.slice(start)).toEqual(['engine:local-view','engine:local-view','engine:close','core:close-object','destroy'])
  expect(h.calls.at(-2)).toMatchObject({action:'play-end',playbackGeneration:1})
  expectReadOnly(h)
})
it('retains a working readonly preview when this engine has no presentation capability', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  h.setTransform((request, reply) => { if (request.action?.startsWith('play-')) throw Error('presentation-unsupported'); return reply })
  expect(await h.controller.request(playbackControl('play-read'))).toMatchObject({ok:false,error:'presentation_unsupported',view:{status:'ready'}})
  await h.controller.request({action:'hide'}); await h.controller.request({action:'status'})
  expect(h.getDestroyed()).toBe(0)
})
it('retires a clean surface when native playback cleanup fails rather than claiming it stopped', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  await h.controller.request(playbackControl('play-start'))
  h.setTransform((request, reply) => { if (request.action === 'play-end') throw Error('presentation-failed'); return reply })
  await h.controller.request({action:'hide'}); await h.controller.request({action:'status'})
  expect(h.getDestroyed()).toBe(1)
  expect(h.controller.getViews()[0]).toMatchObject({status:'error',error:'presentation_failed',dirty:false,playback:{phase:'error',slideIndex:null,slideCount:null}})
  expect(h.hostCalls.some(call => call.action === 'invoke' && call.operation === 'commit-object')).toBe(false)
})
it('keeps ordinary dirty editing state and refuses a new show before reaching the engine', async () => {
  const h = harness(undefined, true, 'pptx'); await h.controller.request(presentationOpen)
  expect((await h.controller.request({action:'annotate',objectId,revision,expectedChangeSequence:0})).ok).toBe(true)
  h.emit({type:'changed',operationId:h.getOpenOperation(),documentId:objectId,version:revision,
    state:{...state(),kind:'pptx',changeSequence:1,dirty:true}})
  const before = h.calls.length
  expect(await h.controller.request({...playbackControl('play-start'),expectedChangeSequence:1})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(h.calls).toHaveLength(before)
  expect(h.controller.getViews()[0]).toMatchObject({dirty:true,editing:true,changeSequence:1})
})
it('rejects a generic preview-only receipt for a dedicated playback state read', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  h.setTransform((request, reply) => {
    if (request.action !== 'play-read') return reply
    const view = {...reply.view as Record<string,unknown>}; delete view.playback
    return {...reply,view}
  })
  expect(await h.controller.request(playbackControl('play-read'))).toMatchObject({ok:false,error:'invalid_response'})
  expect(h.getDestroyed()).toBe(1)
})
it('treats a canvas-owned startup busy refusal as transient rather than retiring a healthy surface', async () => {
  const h = harness(undefined, true, 'pptx'); await h.controller.request(presentationOpen)
  h.setTransform((request, reply) => { if (request.command === 'edit' || request.command === 'captureSelection') throw Error('operation-in-progress'); return reply })
  expect(await h.controller.request({action:'annotate',objectId,revision,expectedChangeSequence:0})).toMatchObject({ok:false,error:'unavailable',view:{status:'ready'}})
  expect(await h.controller.request({action:'captureSelection'})).toMatchObject({ok:false,error:'unavailable',view:{status:'ready'}})
  expect(h.getDestroyed()).toBe(0)
  expect(h.controller.getViews()[0].error).toBeUndefined()
  expect((await h.controller.request(playbackControl('play-read'))).ok).toBe(true)
})
it('destroys an unconfirmed startup lifetime before a delayed native running notification can revive it', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  h.setTransform((request, reply) => { if (request.action === 'play-start') throw Error('presentation-start-timeout'); return reply })
  expect(await h.controller.request(playbackControl('play-start'))).toMatchObject({ok:false,error:'presentation_start_timeout'})
  expect(h.getDestroyed()).toBe(1)
  h.emit({type:'view',operationId:h.getOpenOperation(),documentId:objectId,version:revision,
    view:{zoom:100,zoomType:3,fit:false,page:2,pages:4,readOnly:true,modified:false,
      playback:{generation:1,sequence:999,phase:'running',slideIndex:0,slideCount:3}}})
  expect(h.controller.getViews()[0]).toMatchObject({status:'error',error:'presentation_start_timeout',dirty:false})
  expect(h.controller.getViews()[0].playback?.phase).not.toBe('running')
})
it('does not issue another native end for an already confirmed idle show on repeated hides', async () => {
  const h = harness(undefined, false, 'pptx'); await h.controller.request(presentationOpen)
  await h.controller.request(playbackControl('play-start'))
  await h.controller.request(playbackControl('play-end', 1))
  for (let i = 0; i < 2; i++) {
    await h.controller.request({action:'hide'}); await h.controller.request({action:'status'})
    await h.controller.request({action:'bounds',bounds})
  }
  expect(h.calls.filter(call => call.action === 'play-end')).toHaveLength(1)
  expect(h.controller.getViews()[0].playback).toMatchObject({generation:2,phase:'idle'})
})
it('keeps a dirty unconfirmed show blocked across repeated opens until explicit discard retires its surface', async () => {
  const h = harness(undefined, true, 'pptx'); await h.controller.request(presentationOpen)
  await h.controller.request({action:'annotate',objectId,revision,expectedChangeSequence:0})
  await h.controller.request(playbackControl('play-start'))
  h.emit({type:'changed',operationId:h.getOpenOperation(),documentId:objectId,version:revision,
    state:{...state(),kind:'pptx',changeSequence:1,dirty:true}})
  h.setTransform((request, reply) => {
    if (request.action === 'play-end') throw Error('presentation-failed')
    if (request.action === 'play-read') return {...reply,state:{...state(),kind:'pptx',changeSequence:1,dirty:true},
      view:{...reply.view as Record<string,unknown>,modified:true}}
    return reply
  })
  await h.controller.request({action:'hide'}); await h.controller.request({action:'status'})
  const before = h.hostCalls.length
  expect(await h.controller.request(presentationOpen)).toMatchObject({ok:false,error:'unknown'})
  expect(h.hostCalls).toHaveLength(before)
  expect(h.getCreated()).toBe(1); expect(h.getDestroyed()).toBe(0)
  expect(h.controller.getViews()[0]).toMatchObject({dirty:true,editing:true,changeSequence:1})
  expect(await h.controller.request({action:'close',objectId})).toMatchObject({ok:false,error:'unsaved_changes'})
  expect(await h.controller.request({action:'close',objectId,discard:true})).toMatchObject({ok:true,view:null})
  expect(h.getDestroyed()).toBe(1)
})
it('allows only bounded playback on a current protected Present owner and retires it on close', async () => {
  const h = harness(undefined, false, 'pptx'), owner = new AbortController()
  const selector = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_' + 'a'.repeat(64),
    resultItemId:'item_result_' + 'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  h.setTransform((request, reply) => ({...reply,
    ...(reply.state ? {state:{...reply.state as Record<string,unknown>,documentId:request.documentId,version:request.version}} : {}),
    ...(reply.selection ? {selection:{...reply.selection as Record<string,unknown>,documentId:request.documentId,version:request.version}} : {})}))
  const opened = await h.controller.openPresent({action:'openPresent',protectedPresent:selector,bounds}, {
    path:'/workspace/report.pptx',workspaceRoot:'/workspace',kind:'pptx',selector,bytes:Buffer.from(bytes),digest:revision,
    size:bytes.length,mtimeMs:1,truncated:false,signal:owner.signal,isCurrent:() => !owner.signal.aborted,guard:async () => !owner.signal.aborted
  })
  expect(opened.ok).toBe(true)
  const target = {objectId:opened.view!.objectId,revision,expectedChangeSequence:0}
  expect(await h.controller.request({action:'viewControl',...target,control:'play-start',playbackGeneration:0})).toMatchObject({ok:true,view:{
    protectedPresent:selector,playback:{phase:'running'},dirty:false}})
  expect(await h.controller.request({action:'annotate',...target})).toMatchObject({ok:false,error:'invalid_request'})
  expect(await h.controller.request({action:'close',objectId:target.objectId})).toMatchObject({ok:true,view:null})
  expect(h.calls.at(-1)).toMatchObject({action:'play-end',playbackGeneration:1})
  expect(h.hostCalls.every(call => call.action === 'list')).toBe(true)
  expect(h.getDestroyed()).toBe(1)
})

it.each([
  ['create', 'controller_surface_create', 'unexpected'],
  ['attach', 'controller_surface_attach', 'unexpected'],
  ['open', 'controller_surface_open', 'engine_unavailable']
] as const)('observes %s failure while preserving open cleanup and public errors', async (fault, stage, code) => {
  vi.mocked(logNativeOfficeFailure).mockClear()
  const h = harness(fault)
  const result = await h.controller.request(open)
  expect(result).toMatchObject({ ok: false, view: null, error: fault === 'open' ? 'engine_unavailable' : 'unavailable' })
  expect(logNativeOfficeFailure).toHaveBeenCalledWith(stage, code, false)
  expect(h.hostCalls.at(-1)).toMatchObject({ action: 'invoke', operation: 'close-object' })
  expect(h.getDestroyed()).toBe(fault === 'create' ? 0 : 1)
  expect(JSON.stringify(vi.mocked(logNativeOfficeFailure).mock.calls)).not.toContain('/private/office/body-sentinel')
})
it('observes the package gate before a surface exists', async () => {
  vi.mocked(logNativeOfficeFailure).mockClear()
  const h = harness(); h.pkg.available = false
  expect(await h.controller.request(open)).toMatchObject({ ok: false, error: 'package_unavailable' })
  expect(logNativeOfficeFailure).toHaveBeenCalledWith('controller_package', 'package_unavailable', false)
  expect(h.calls).toHaveLength(0); expect(h.getDestroyed()).toBe(0)
})

it.each([
  ['forbidden', 'forbidden', false],
  ['unavailable', 'unavailable', false],
  ['conflict', 'unavailable', false],
  ['unknown_denial', 'invalid_response', false],
  ['forbidden', 'invalid_response', true]
] as const)('projects only a validated Core %s refusal as %s without creating a surface', async (code, error, extraField) => {
  vi.mocked(logNativeOfficeFailure).mockClear()
  const h = harness(), privateMessage = '/private/office/denied-body-sentinel'
  h.setDocument({ ok: false, code, message: privateMessage, ...(extraField ? { details: privateMessage } : {}) })
  const result = await h.controller.request(open)
  expect(result).toEqual({ ok: false, view: null, error })
  expect(nativeOfficeResponseSchema.safeParse(result).success).toBe(true)
  expect(logNativeOfficeFailure).toHaveBeenCalledWith('controller_open_object', error, false)
  expect(h.getCreated()).toBe(0); expect(h.calls).toHaveLength(0)
  expect(h.hostCalls.filter(call => call.action === 'invoke')).toHaveLength(1)
  expect(JSON.stringify({ result, views: h.views, logs: vi.mocked(logNativeOfficeFailure).mock.calls })).not.toContain(privateMessage)
})

it('keeps an open-object transport failure unavailable without exporting its message', async () => {
  vi.mocked(logNativeOfficeFailure).mockClear()
  const h = harness(), privateMessage = '/private/office/transport-body-sentinel'
  h.setDocument(new Error(privateMessage))
  const result = await h.controller.request(open)
  expect(result).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(logNativeOfficeFailure).toHaveBeenCalledWith('controller_open_object', 'unavailable', false)
  expect(h.getCreated()).toBe(0); expect(h.calls).toHaveLength(0)
  expect(JSON.stringify({ result, logs: vi.mocked(logNativeOfficeFailure).mock.calls })).not.toContain(privateMessage)
})

it('keeps an otherwise successful open unavailable when visibility changes before its response', async () => {
  vi.mocked(logNativeOfficeFailure).mockClear()
  const h = harness()
  let complete: (value: unknown) => void = () => {}
  h.setDocument(new Promise(resolve => { complete = resolve }))
  const pending = h.controller.request(open)
  await vi.waitFor(() => expect(h.hostCalls.some(call => call.action === 'invoke' && call.operation === 'open-object')).toBe(true))
  await h.controller.request({ action: 'hide' })
  complete({ ok: true, document: { sessionId, objectId, path: '/workspace/report.docx', revision, content: Buffer.from(bytes).toString('base64') } })
  expect(await pending).toEqual({ ok: false, view: null, error: 'unavailable' })
  expect(logNativeOfficeFailure).toHaveBeenCalledWith('controller_open_object', 'unavailable', false)
  expect(h.getCreated()).toBe(0); expect(h.calls).toHaveLength(0)
  expect(h.hostCalls.at(-1)).toMatchObject({ action: 'invoke', operation: 'close-object', input: { sessionId } })
})

it('validates a DOCX range before presentation and again before a pointer action; no edit or Host reference is issued',async()=>{
  const h=harness();await h.controller.request(open)
  const selected={...selection(),token:'selection_token'}
  h.emit({type:'selection',documentId:objectId,version:revision,operationId:h.getOpenOperation(),selection:selected})
  const menu={objectId,revision,expectedChangeSequence:0,pointer:{interactionId:'interaction_id',selectionToken:selected.token},actions:[{id:'quote',label:'Quote',enabled:true}]}
  h.setPointerCurrent(true)
  expect(await h.controller.pointerActions(menu)).toBe('quote')
  expect(h.calls.filter(r=>r.command==='validateSelection')).toHaveLength(2)
  expect(h.calls.filter(r=>r.command==='validateSelection').every(r=>r.selectionToken===selected.token)).toBe(true)
  expect(h.hostCalls.some(r=>r.action==='invoke'&&r.operation==='capture-selection')).toBe(false)
  expect(h.controller.getView()?.editing).not.toBe(true)
  h.setPointerCurrent(true)
  h.setTransform((r,reply)=>{if(r.command==='validateSelection')throw Error('stale-selection');return reply})
  expect(await h.controller.pointerActions(menu)).toBeNull()
  expect(h.presentPointer).toHaveBeenCalledTimes(1)
})
it('a pointer lease cancelled or hidden while its action is pending never activates',async()=>{
  const h=harness();await h.controller.request(open)
  h.emit({type:'selection',documentId:objectId,version:revision,operationId:h.getOpenOperation(),selection:{...selection(),token:'selection_token'}})
  const menu={objectId,revision,expectedChangeSequence:0,pointer:{interactionId:'interaction_id',selectionToken:'selection_token'},actions:[{id:'quote',label:'Quote',enabled:true}]}
  let choose!:(id:string|null)=>void
  h.presentPointer.mockImplementation(()=>new Promise(resolve=>{choose=resolve}));h.setPointerCurrent(true)
  const pending=h.controller.pointerActions(menu)
  await vi.waitFor(()=>expect(h.presentPointer).toHaveBeenCalledOnce())
  await h.controller.request({action:'hide'});choose('quote')
  expect(await pending).toBeNull()
  expect(h.calls.filter(r=>r.command==='validateSelection')).toHaveLength(1)
})


describe('user Word creation and native manual admission', () => {
  const request = { action:'create',threadId:'thread-word',operationId:'create_operation',fileName:'report.docx',bounds }
  const creation = { operationId:request.operationId,workspace:'/workspace',path:'/workspace/report.docx',revision,kind:'docx',byteSize:bytes.byteLength,savedAt:'2026-10-15T00:00:00Z' }
  it('returns only the Core thread-workspace receipt without opening, attaching or publishing a native view', async () => {
    const h=harness();h.pkg.operations.push('create-object')
    const result=await h.controller.request(request)
    expect(result).toEqual({ok:true,view:null,creation})
    expect(h.hostCalls.find(call=>call.action==='invoke'&&call.operation==='create-object')).toMatchObject({input:{threadId:request.threadId,operationId:request.operationId,fileName:request.fileName}})
    expect(h.hostCalls.some(call=>call.action==='invoke'&&call.operation==='open-object')).toBe(false)
    expect(h.calls).toEqual([]);expect(h.views).toEqual([])
    expect(h.getCreated()).toBe(0);expect(h.getAttached()).toBe(0);expect(h.getHidden()).toBe(0)
    expect(h.controller.getView()).toBeNull();expect(h.controller.getViews()).toEqual([])
    expect(nativeOfficeResponseSchema.safeParse(result).success).toBe(true)
    expect(await h.controller.request({...request,workspace:'/other'})).toMatchObject({ok:false,error:'invalid_request'})
  })
  it('retries only explicit native open after its failure without repeating a confirmed Core creation', async () => {
    const h=harness('open');h.pkg.operations.push('create-object')
    const result=await h.controller.request(request)
    expect(result).toEqual({ok:true,view:null,creation})
    expect(h.getCreated()).toBe(0)
    const receipt=nativeOfficeResponseSchema.parse(result).creation!
    const openCreated={action:'open',threadId:request.threadId,workspace:receipt.workspace,path:receipt.path,bounds}
    expect(await h.controller.request(openCreated)).toMatchObject({ok:false,view:null})
    h.setOpenFailure(false)
    expect(await h.controller.request(openCreated)).toMatchObject({ok:true,view:{path:receipt.path,status:'ready',dirty:false}})
    expect(h.hostCalls.filter(call=>call.action==='invoke'&&call.operation==='create-object')).toHaveLength(1)
    expect(h.hostCalls.filter(call=>call.action==='invoke'&&call.operation==='open-object')).toHaveLength(2)
    expect(h.calls.map(call=>call.command)).toEqual(['open','open'])
    expect(h.hostCalls.some(call=>call.action==='invoke'&&['undo-change','cancel-change'].includes(call.operation))).toBe(false)
  })
  it('retains a deferred Core receipt after hide without creating a native owner', async () => {
    const h=harness();h.pkg.operations.push('create-object')
    let complete!:(value:unknown)=>void
    h.setCreation(new Promise(resolve=>{complete=resolve}))
    const pending=h.controller.request(request)
    await vi.waitFor(()=>expect(h.hostCalls.some(call=>call.action==='invoke'&&call.operation==='create-object')).toBe(true))
    await h.controller.request({action:'hide'})
    complete({ok:true,creation})
    expect(await pending).toEqual({ok:true,view:null,creation})
    expect(h.calls).toEqual([]);expect(h.views).toEqual([])
    expect(h.getCreated()).toBe(0);expect(h.getAttached()).toBe(0);expect(h.getHidden()).toBe(0)
    expect(h.controller.getView()).toBeNull()
  })
  it('does not reattach or replace the existing native owner when a deferred creation finishes after hide', async () => {
    const h=harness();h.pkg.operations.push('create-object');await h.controller.request(open)
    const before=structuredClone(h.controller.getView())
    let complete!:(value:unknown)=>void
    h.setCreation(new Promise(resolve=>{complete=resolve}))
    const pending=h.controller.request(request)
    await vi.waitFor(()=>expect(h.hostCalls.some(call=>call.action==='invoke'&&call.operation==='create-object')).toBe(true))
    await h.controller.request({action:'hide'})
    complete({ok:true,creation})
    expect(await pending).toEqual({ok:true,view:null,creation})
    expect(h.controller.getView()).toEqual(before)
    expect(h.getCreated()).toBe(1);expect(h.getAttached()).toBe(1);expect(h.getHidden()).toBe(1)
    expect(h.calls.map(call=>call.command)).toEqual(['open'])
    expect(h.hostCalls.filter(call=>call.action==='invoke'&&call.operation==='open-object')).toHaveLength(1)
  })
  it('does not alter a pending explicit native open visibility owner when creation is requested', async () => {
    const h=harness();h.pkg.operations.push('create-object')
    let complete!:(value:unknown)=>void
    h.setDocument(new Promise(resolve=>{complete=resolve}))
    const opening=h.controller.request(open)
    await vi.waitFor(()=>expect(h.hostCalls.some(call=>call.action==='invoke'&&call.operation==='open-object')).toBe(true))
    const creating=h.controller.request(request)
    complete({ok:true,document:{sessionId,objectId,path:'/workspace/report.docx',revision,content:Buffer.from(bytes).toString('base64')}})
    expect(await opening).toMatchObject({ok:true,view:{objectId,status:'ready'}})
    expect(await creating).toEqual({ok:true,view:null,creation})
    expect(h.controller.getView()).toMatchObject({objectId,status:'ready'})
    expect(h.getCreated()).toBe(1);expect(h.getAttached()).toBe(1);expect(h.getHidden()).toBe(0)
    expect(h.calls.map(call=>call.command)).toEqual(['open'])
    expect(h.hostCalls.filter(call=>call.action==='invoke'&&call.operation==='open-object')).toHaveLength(1)
  })
  it('preserves creation uncertainty and forbidden without inventing a path or opening a surface', async () => {
    const h=harness();h.pkg.operations.push('create-object');h.setCreation(Error('synthetic lost reply'))
    expect(await h.controller.request(request)).toMatchObject({ok:false,error:'unknown'})
    expect(h.getCreated()).toBe(0)
    h.setCreation({ok:false,code:'forbidden',message:'denied'})
    expect(await h.controller.request(request)).toMatchObject({ok:false,error:'forbidden'})
    expect(h.getCreated()).toBe(0)
  })
  it('refuses manual entry when the Core package has no explicit manual capability', async () => {
    const h=harness();await h.controller.request({...open,threadId:'thread-word'})
    const result=await h.controller.request({action:'beginManualEdit',objectId,revision,expectedChangeSequence:0,threadId:'thread-word'})
    expect(result).toMatchObject({ok:false,error:'package_unavailable',view:{dirty:false}})
    expect(h.hostCalls.some(call=>call.action==='invoke'&&call.operation==='begin-manual-edit')).toBe(false)
    expect(h.calls.map(call=>call.command)).toEqual(['open'])
  })
})


const pointerFormatLabels={fontFamily:'Font',fontSize:'Size',bold:'Bold',italic:'Italic',underline:'Underline',mixed:'Mixed'}
async function manualPointerMenu(){
  const h=harness(undefined,true,'docx',true)
  expect((await h.controller.request({...open,threadId:'thread-pointer'})).ok).toBe(true)
  expect((await h.controller.request({action:'beginManualEdit',threadId:'thread-pointer',objectId,revision,expectedChangeSequence:0})).ok).toBe(true)
  const formatting:NativeOfficeFormatting={changeId:h.manualAuthority.changeId,rangeLease:{...h.rangeLease},formatState:{...h.formatSample},labels:pointerFormatLabels}
  const menu={objectId,revision,expectedChangeSequence:0,pointer:{interactionId:'format_interaction',selectionToken:h.rangeLease.token},formatting,actions:[{id:'quote',label:'Quote',enabled:true}]}
  h.setPointerCurrent(true)
  return {h,menu,choice:{actionId:null,changeId:formatting.changeId,rangeLease:{...formatting.rangeLease},format:{kind:'bold',value:true}} as NativeOfficeFormatChoice}
}
it('relays a typed pointer format choice after two native range checks without performing a mutation or Core reference',async()=>{
  const {h,menu,choice}=await manualPointerMenu()
  h.presentPointer.mockResolvedValueOnce(choice)
  expect(await h.controller.pointerActions(menu)).toEqual(choice)
  expect(h.presentPointer).toHaveBeenCalledExactlyOnceWith(menu)
  expect(h.calls.filter(r=>r.command==='validateSelection')).toHaveLength(2)
  expect(h.calls.some(r=>r.command==='format'||r.command==='export'||r.command==='replace')).toBe(false)
  expect(h.hostCalls.some(r=>r.action==='invoke'&&['capture-selection','commit-object'].includes(r.operation))).toBe(false)
  expect(h.dismissPointer).toHaveBeenCalledOnce()
})
it('rejects a formatting offer unless it is the actual current manual range and format sample',async()=>{
  for(const mismatch of ['change','generation','sample'] as const){
    const {h,menu}=await manualPointerMenu()
    const formatting={...menu.formatting,
      ...(mismatch==='change'?{changeId:'9'.repeat(64)}:{}),
      ...(mismatch==='generation'?{rangeLease:{...menu.formatting.rangeLease,generation:2}}:{}),
      ...(mismatch==='sample'?{formatState:{...menu.formatting.formatState,fontSize:13}}:{})}
    expect(await h.controller.pointerActions({...menu,formatting})).toBeNull()
    expect(h.presentPointer).not.toHaveBeenCalled()
    expect(h.calls.some(r=>r.command==='validateSelection'||r.command==='format')).toBe(false)
  }
})
it('refuses forged typed format choices even when ordinary quote actions were offered',async()=>{
  for(const mismatch of ['change','generation','token','property'] as const){
    const {h,menu,choice}=await manualPointerMenu()
    const forged={...choice,
      ...(mismatch==='change'?{changeId:'9'.repeat(64)}:{}),
      ...(mismatch==='generation'?{rangeLease:{...choice.rangeLease,generation:2}}:{}),
      ...(mismatch==='token'?{rangeLease:{...choice.rangeLease,token:'different_token'}}:{}),
      ...(mismatch==='property'?{format:{kind:'uno',value:'.uno:Save'}}:{})}
    h.presentPointer.mockResolvedValueOnce(forged as NativeOfficeFormatChoice)
    expect(await h.controller.pointerActions(menu)).toBeNull()
    expect(h.calls.filter(r=>r.command==='validateSelection')).toHaveLength(1)
    expect(h.calls.some(r=>r.command==='format'||r.command==='export')).toBe(false)
  }
})
it('a readonly selection sample cannot create manual formatting authority',async()=>{
  const h=harness();await h.controller.request(open)
  const rangeLease={generation:1,sequence:0,token:'format_selection'},formatState={fontFamily:'Noto Sans CJK SC',fontSize:12,bold:false,italic:false,underline:false}
  h.emit({type:'selection',documentId:objectId,version:revision,operationId:h.getOpenOperation(),selection:{...selection(),token:rangeLease.token,rangeLease,formatState}})
  h.setPointerCurrent(true)
  expect(await h.controller.pointerActions({objectId,revision,expectedChangeSequence:0,pointer:{interactionId:'format_interaction',selectionToken:rangeLease.token},formatting:{changeId:'8'.repeat(64),rangeLease,formatState,labels:pointerFormatLabels},actions:[{id:'quote',label:'Quote',enabled:true}]})).toBeNull()
  expect(h.presentPointer).not.toHaveBeenCalled();expect(h.calls.some(r=>r.command==='validateSelection'||r.command==='format')).toBe(false)
  expectReadOnly(h)
})
it('format choices are refused if manual input freezes before presentation or while the guest choice is pending',async()=>{
  for(const timing of ['before','after'] as const){
    const {h,menu,choice}=await manualPointerMenu()
    if(timing==='before')h.freezeManual()
    else h.presentPointer.mockImplementationOnce(async()=>{h.freezeManual();return choice})
    expect(await h.controller.pointerActions(menu)).toBeNull()
    expect(h.presentPointer).toHaveBeenCalledTimes(timing==='before'?0:1)
    expect(h.calls.filter(r=>r.command==='validateSelection')).toHaveLength(timing==='before'?0:1)
    expect(h.calls.some(r=>r.command==='format'||r.command==='export')).toBe(false)
  }
})
