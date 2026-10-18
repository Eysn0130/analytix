import { afterEach, describe, expect, it, vi } from 'vitest'
import { EventEmitter } from 'node:events'
const ipc = vi.hoisted(() => ({ handlers: new Map<string, (event: any, payload: unknown) => any>() }))
vi.mock('electron', () => ({ ipcMain: { handle: (channel: string, handler: (event: any, payload: unknown) => any) => ipc.handlers.set(channel, handler) }, shell: { openExternal: vi.fn(async () => undefined) } }))
import {
  ProcessLocalDisplayHostV1, registerProcessLocalDisplayIpcV1, invalidateProcessLocalDisplaysV1,
  withProcessLocalDisplayTransitionV1
} from './process-local-display-ipc'
const scope = { threadId: 'thread-a', turnId: 'turn-a', contextDigest: 'b'.repeat(64), contextEpoch: '1', processIncarnation: 'incarnation-a' }
const request = { paneId: 'primary', threadId: scope.threadId, turnId: scope.turnId }
const source = { kind: 'tool' as const, unitId: 'unit-a', callId: 'call-a', toolCallItemId: 'call-item-a', toolResultItemId: 'result-item-a', startedEventSeq: '12' }
const unit = { unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', placement: 'anchored', calls: [{ callId: source.callId, toolCallItemId: source.toolCallItemId, toolResultItemId: source.toolResultItemId, startedEventSeq: source.startedEventSeq }], precedingCalls: [] }
const block = { blockId: 'block-a', source, kind: 'shell', state: 'completed', attempt: 0, durationMs: 3,
  exitCode: 0, truncated: false, classification: 'source_exact_local', captureDigest: 'a'.repeat(64),
  body: { text: 'synthetic private local body', before: '', after: '', error: '', addedLines: 0, removedLines: 0, approximate: false } }
const reply = (value: unknown) => ({ ok: true, status: 200, body: JSON.stringify(value) })
const snapshot = (withBody = false) => ({ status: 'snapshot', schemaVersion: 2, units: [unit], revision: '1', scope,
  blocks: [{ ...block, body: withBody ? block.body : undefined }] })
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done }); return { promise, resolve } }
function fixture() {
  let documentCurrent = true, runtimeCurrent = true
  const controller = new AbortController()
  const release = vi.fn(() => controller.abort())
  const owner = { ownerId: 1, documentNonce: '11111111-1111-4111-8111-111111111111', documentEpoch: '1', signal: controller.signal,
    isCurrent: () => documentCurrent, release }
  const transport = vi.fn(async (_path: string, raw: string, _signal?: AbortSignal) => {
    const input = JSON.parse(raw)
    if (input.action === 'authorize') return reply({ status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, revision: '0', scope })
    if (input.action === 'revoke') return reply({ status: 'revoked' })
    if (input.action === 'open_capture') return reply({ status: 'capture', schemaVersion: 2, unit, revision: '1', scope, block })
    return reply(snapshot(input.expandedBlockIds.length > 0))
  })
  const host = new ProcessLocalDisplayHostV1(transport, () => ({ isCurrent: () => runtimeCurrent }))
  const authorize = async () => {
    const result = await host.authorize(request, owner)
    expect(result.status).toBe('authorized')
    if (result.status !== 'authorized') throw new Error('synthetic authorization failed')
    return result
  }
  return { host, transport, owner, release, controller, authorize,
    revokeDocument: () => { documentCurrent = false; controller.abort() }, revokeRuntime: () => { runtimeCurrent = false } }
}
afterEach(() => { invalidateProcessLocalDisplaysV1(); ipc.handlers.clear() })
describe('protected process pane leases', () => {
  it.each(['tool_input', 'shell', 'web_fetch'])('fences complete running %s input through the existing expansion epoch', async kind => {
    const f = fixture(), authorized = await f.authorize(), local = { viewerId: authorized.viewerId }
    const inputSource = { ...source, toolResultItemId: undefined }
    const live = { blockId: 'raw-input', source: inputSource, kind, state: 'running', attempt: 0,
      durationMs: 0, truncated: false, classification: 'source_exact_local',
      input: { availability: 'available', inputJSON: ' {"number":9007199254740993} ' } }
    const inputAnchor = { callId: source.callId, toolCallItemId: source.toolCallItemId, startedEventSeq: source.startedEventSeq }
    const value = { ...snapshot(), units: [{ ...unit, calls: [inputAnchor] }], blocks: [live] }
    expect(f.host.setExpanded({ ...local, expandedBlockIds: [live.blockId] }, 1)).toEqual({ status: 'ok' })
    f.transport.mockResolvedValueOnce(reply(value))
    expect(await f.host.read(local, 1)).toEqual(value)
    const pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(() => pending.promise)
    const late = f.host.read(local, 1)
    f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)
    pending.resolve(reply(value))
    expect(await late).toEqual({ status: 'unavailable' })
    f.transport.mockResolvedValueOnce(reply({ ...value, blocks: [{ ...live, input: undefined }] }))
    expect(await f.host.read(local, 1)).toMatchObject({ status: 'snapshot', blocks: [{ blockId: live.blockId }] })
    f.transport.mockResolvedValueOnce(reply(value))
    expect(await f.host.read(local, 1)).toEqual({ status: 'unavailable' })
  })
  const pendingUnit = { ...unit, placement: 'pending', calls: [], precedingCalls: [] }
  const runningReasoning = (provider = false) => ({ blockId: 'live-think', kind: 'native_reasoning', state: 'running', attempt: 1,
    source: provider ? { kind: 'provider_reasoning', unitId: unit.unitId, producerId: unit.producerId,
      providerSequence: unit.providerSequence, attempt: 1, sourceOrdinal: '3' }
      : { kind: 'native', unitId: unit.unitId, producerId: unit.producerId,
        providerSequence: unit.providerSequence, attempt: 1, sourceOrdinal: '2', nativeIndex: 0 },
    ...(provider ? {} : { nativeIndex: 0 }), title: 'Think', preview: 'complete first line',
    durationMs: 0, truncated: false, classification: 'source_exact_local' })
  const runningShell = { blockId: 'live-shell', kind: 'shell', state: 'running', attempt: 0,
    source: { kind: source.kind, unitId: source.unitId, callId: source.callId,
      toolCallItemId: source.toolCallItemId, startedEventSeq: source.startedEventSeq }, title: 'printf synthetic', durationMs: 2,
    truncated: false, classification: 'source_exact_local' }
  const preparing = { blockId: 'preparing-call', kind: 'tool_preparation', state: 'preparing', attempt: 1,
    source: { kind: 'tool_preparation', unitId: unit.unitId, producerId: unit.producerId,
      providerSequence: unit.providerSequence, attempt: 1, callId: 'pending-call', sourceOrdinal: '2' },
    toolName: 'write', path: '/synthetic/partial.ts', inputUtf16CodeUnits: 3,
    durationMs: 0, truncated: false, classification: 'source_exact_local' }
  const liveCases = () => [
    { block: runningReasoning(), unit: pendingUnit }, { block: runningReasoning(true), unit: pendingUnit },
    { block: runningShell, unit: { ...unit, calls: [{ ...unit.calls[0], toolResultItemId: undefined }] } }
  ]
  const liveSnapshot = (live: { block: unknown; unit: unknown }, withBody = false) => ({ status: 'snapshot', schemaVersion: 2,
    revision: '1', scope, units: [live.unit], blocks: [{ ...(live.block as object), ...(withBody ? { body: block.body } : {}) }] })

  it('returns attributed running summaries and only remembered expanded text under the same Main lease', async () => {
    for (const live of liveCases()) {
      const f = fixture(), authorized = await f.authorize(), local = { viewerId: authorized.viewerId }
      f.transport.mockImplementation(async (_path, raw) => {
        const input = JSON.parse(raw)
        return input.action === 'revoke' ? reply({ status: 'revoked' })
          : reply(liveSnapshot(live, live.block.kind === 'native_reasoning' && input.expandedBlockIds.includes(live.block.blockId)))
      })
      const folded = await f.host.read(local, 1)
      expect(folded).toMatchObject({ status: 'snapshot', blocks: [live.block] })
      expect(JSON.stringify(folded)).not.toContain(block.body.text)
      expect(f.host.setExpanded({ ...local, expandedBlockIds: [live.block.blockId] }, 1)).toEqual({ status: 'ok' })
      const expanded = await f.host.read(local, 1)
      expect(expanded).toMatchObject({ status: 'snapshot', blocks: [live.block.kind === 'native_reasoning'
        ? { ...live.block, body: block.body } : live.block] })
      if (live.block.kind === 'shell') expect(JSON.stringify(expanded)).not.toContain(block.body.text)
      expect(f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)).toEqual({ status: 'ok' })
      expect(await f.host.read(local, 1)).toEqual(folded)
      expect(f.release).not.toHaveBeenCalled()
    }
    const f = fixture(), authorized = await f.authorize()
    f.transport.mockResolvedValueOnce(reply(liveSnapshot({ block: preparing, unit: pendingUnit })))
    expect(await f.host.read({ viewerId: authorized.viewerId }, 1)).toMatchObject({ status: 'snapshot', blocks: [preparing] })
  })
  it('rejects unexpanded reasoning text and all running shell or preparation payload', async () => {
    for (const live of [...liveCases(), { block: preparing, unit: pendingUnit }]) {
      const f = fixture(), authorized = await f.authorize()
      if (live.block.kind !== 'native_reasoning') {
        f.host.setExpanded({ viewerId: authorized.viewerId, expandedBlockIds: [live.block.blockId] }, 1)
      }
      f.transport.mockResolvedValueOnce(reply(liveSnapshot(live, true)))
      expect(await f.host.read({ viewerId: authorized.viewerId }, 1)).toEqual({ status: 'unavailable' })
      expect(f.release).toHaveBeenCalledOnce()
    }
  })
  it('drops a late provider reasoning body on collapse and a late running shell command on document retirement', async () => {
    for (const [index, live] of liveCases().slice(1).entries()) {
      const f = fixture(), authorized = await f.authorize(), local = { viewerId: authorized.viewerId }
      f.host.setExpanded({ ...local, expandedBlockIds: [live.block.blockId] }, 1)
      const pending = deferred<ReturnType<typeof reply>>()
      f.transport.mockImplementationOnce(async () => pending.promise)
      const reading = f.host.read(local, 1)
      if (index === 0) f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)
      else f.revokeDocument()
      pending.resolve(reply(liveSnapshot(live, live.block.kind === 'native_reasoning')))
      expect(await reading).toEqual({ status: 'unavailable' })
      expect(f.release).toHaveBeenCalledTimes(index === 0 ? 0 : 1)
    }
  })
  it('projects only real draft sources and never forwards the desktop source selector or expands a folded tool', async () => {
    const f = fixture(), authorized = await f.authorize(), local = { viewerId: authorized.viewerId }
    const draft = { blockId: 'draft-a', source: { kind: 'assistant_draft', unitId: unit.unitId, producerId: unit.producerId, providerSequence: unit.providerSequence, attempt: 1 },
      kind: 'assistant_draft', state: 'running', attempt: 1, durationMs: 0, truncated: false, classification: 'source_exact_local', contentClassification: 'unclassified_inline' }
    f.transport.mockImplementation(async (_path, raw) => {
      const input = JSON.parse(raw)
      if (input.action === 'revoke') return reply({ status: 'revoked' })
      expect(input).not.toHaveProperty('draftsOnly')
      return reply({ ...snapshot(input.expandedBlockIds.includes(block.blockId)),
        blocks: [{ ...block, preview: 'FOLDED_TOOL_PRIVATE_PREVIEW', body: input.expandedBlockIds.includes(block.blockId) ? block.body : undefined },
          { ...draft, ...(input.expandedBlockIds.includes(draft.blockId) ? { draftSegments: [{ offsetBytes: '0', text: 'PRIVATE_DRAFT ' }], draftBytes: '14' } : {}) }] })
    })
    const discovered = await f.host.read({ ...local, draftsOnly: true }, 1)
    expect(discovered).toMatchObject({ status: 'snapshot', blocks: [draft] })
    expect(JSON.stringify(discovered)).not.toContain('FOLDED_TOOL_PRIVATE_PREVIEW')
    f.host.setExpanded({ ...local, expandedBlockIds: [block.blockId, draft.blockId] }, 1)
    const expanded = await f.host.read({ ...local, draftsOnly: true }, 1)
    expect(expanded).toMatchObject({ status: 'snapshot', blocks: [{ ...draft, draftSegments: [{ offsetBytes: '0', text: 'PRIVATE_DRAFT ' }], draftBytes: '14' }] })
    expect(JSON.parse(f.transport.mock.calls.at(-1)![1]).expandedBlockIds).toEqual([draft.blockId])
    expect(JSON.stringify(expanded)).not.toContain(block.body.text)
    f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)
    expect(await f.host.read({ ...local, draftsOnly: true }, 1)).toEqual(discovered)
  })
  it.each([false, true])('rejects unexpanded draft segments even for draftsOnly=%s and retires its native owner', async draftsOnly => {
    const f = fixture(), authorized = await f.authorize()
    const draft = { blockId: 'draft-a', source: { kind: 'assistant_draft', unitId: unit.unitId, producerId: unit.producerId, providerSequence: unit.providerSequence, attempt: 1 },
      kind: 'assistant_draft', state: 'running', attempt: 1, durationMs: 0, truncated: false, classification: 'source_exact_local', contentClassification: 'provider_text',
      draftSegments: [{ offsetBytes: '0', text: 'PRIVATE_DRAFT ' }], draftBytes: '14' }
    f.transport.mockResolvedValueOnce(reply({ ...snapshot(), blocks: [draft] }))
    expect(await f.host.read({ viewerId: authorized.viewerId, draftsOnly }, 1)).toEqual({ status: 'unavailable' })
    expect(f.release).toHaveBeenCalledOnce()
  })
  it('mints a private Go token while returning only a Main-local handle', async () => {
    const f = fixture(), authorized = await f.authorize()
    const wire = JSON.parse(f.transport.mock.calls[0][1])
    expect(wire).toMatchObject({ action: 'authorize', documentNonce: f.owner.documentNonce, threadId: scope.threadId, turnId: scope.turnId })
    expect(wire.viewerToken).not.toBe(authorized.viewerId)
    expect(authorized).not.toHaveProperty('viewerToken')
    expect(authorized).not.toHaveProperty('documentNonce')
    expect(await f.host.read({ viewerId: authorized.viewerId }, 2)).toEqual({ status: 'unavailable' })
    expect(f.transport).toHaveBeenCalledTimes(1)
  })
  it('reads only remembered expanded bodies and retains authorization across collapse', async () => {
    const f = fixture(), authorized = await f.authorize()
    const local = { viewerId: authorized.viewerId }
    expect(await f.host.read(local, 1)).toEqual(snapshot())
    expect(f.host.setExpanded({ ...local, expandedBlockIds: [block.blockId] }, 1)).toEqual({ status: 'ok' })
    expect(await f.host.read(local, 1)).toEqual(snapshot(true))
    expect(f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)).toEqual({ status: 'ok' })
    expect(await f.host.read(local, 1)).toEqual(snapshot())
    expect(f.release).not.toHaveBeenCalled()
    const inputs = f.transport.mock.calls.map(call => JSON.parse(call[1]))
    expect(inputs.filter(input => input.action === 'authorize')).toHaveLength(1)
    expect(inputs.filter(input => input.action === 'revoke')).toHaveLength(0)
    expect(inputs.filter(input => input.action === 'read').map(input => input.expandedBlockIds)).toEqual([[], [block.blockId], []])
  })
  it('allows only one poll and aborts late expanded body on collapse without revoking preview', async () => {
    const f = fixture(), authorized = await f.authorize(), local = { viewerId: authorized.viewerId }
    f.host.setExpanded({ ...local, expandedBlockIds: [block.blockId] }, 1)
    const pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(async () => pending.promise)
    const older = f.host.read({ ...local, afterRevision: '0', waitMs: 25000 }, 1)
    expect(await f.host.read(local, 1)).toEqual({ status: 'busy' })
    const signal = f.transport.mock.calls[1][2]!
    f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)
    expect(signal.aborted).toBe(true)
    expect(await f.host.read(local, 1)).toEqual(snapshot())
    pending.resolve(reply(snapshot(true)))
    expect(await older).toEqual({ status: 'unavailable' })
    expect(f.release).not.toHaveBeenCalled()
    expect(await f.host.read(local, 1)).toEqual(snapshot())
  })
  it('rejects unexpanded bodies, changed scope, backwards revisions and open response fields', async () => {
    for (const value of [snapshot(true), { ...snapshot(), scope: { ...scope, contextDigest: 'c'.repeat(64) } },
      { ...snapshot(), viewerToken: 'forged' }, { ...snapshot(), blocks: [{ ...block, signature: 'private' }] }]) {
      const f = fixture(), authorized = await f.authorize()
      f.transport.mockResolvedValueOnce(reply(value))
      expect(await f.host.read({ viewerId: authorized.viewerId }, 1)).toEqual({ status: 'unavailable' })
      expect(f.release).toHaveBeenCalledOnce()
    }
    const f = fixture(), authorized = await f.authorize()
    expect((await f.host.read({ viewerId: authorized.viewerId }, 1)).status).toBe('snapshot')
    f.transport.mockResolvedValueOnce(reply({ status: 'unchanged', schemaVersion: 2, revision: '0', scope }))
    expect(await f.host.read({ viewerId: authorized.viewerId }, 1)).toEqual({ status: 'unavailable' })
  })
  it.each(['document', 'runtime', 'close'] as const)('drops a late poll after %s retirement', async reason => {
    const f = fixture(), authorized = await f.authorize(), local = { viewerId: authorized.viewerId }
    const pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(async () => pending.promise)
    const result = f.host.read(local, 1)
    if (reason === 'document') f.revokeDocument()
    if (reason === 'runtime') f.revokeRuntime()
    if (reason === 'close') f.host.close(local, 1)
    pending.resolve(reply(snapshot()))
    expect(await result).toEqual({ status: 'unavailable' })
    expect(await f.host.read(local, 1)).toEqual({ status: 'unavailable' })
  })
  it.each(['primary', 'secondary'] as const)('does not let old authorization revive or retire successor pane %s', async paneId => {
    const f = fixture(), pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(async () => pending.promise)
    const older = f.host.authorize({ ...request, paneId }, f.owner)
    const newController = new AbortController(), successorRelease = vi.fn(() => newController.abort())
    const successor = await f.host.authorize({ ...request, paneId }, { ...f.owner, signal: newController.signal, release: successorRelease,
      documentNonce: '22222222-2222-4222-8222-222222222222' })
    expect(successor.status).toBe('authorized')
    pending.resolve(reply({ status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, revision: '0', scope }))
    expect(await older).toEqual({ status: 'unavailable' })
    if (successor.status !== 'authorized') throw new Error('missing successor')
    expect(await f.host.read({ viewerId: successor.viewerId }, 1)).toEqual(snapshot())
    expect(successorRelease).not.toHaveBeenCalled()
    expect(f.release).toHaveBeenCalledOnce()
  })
  it('keeps sibling pane bodies and closes only the selected lease', async () => {
    const f = fixture(), first = await f.authorize()
    const leases = [first]
    for (const paneId of ['side', 'inspector']) {
      const controller = new AbortController()
      const value = await f.host.authorize({ ...request, paneId }, { ...f.owner, signal: controller.signal,
        release: () => controller.abort(), documentNonce: crypto.randomUUID() })
      expect(value.status).toBe('authorized')
      if (value.status === 'authorized') leases.push(value)
    }
    for (const value of leases) expect(await f.host.read({ viewerId: value.viewerId }, 1)).toEqual(snapshot())
    expect(f.host.close({ viewerId: leases[1].viewerId }, 1)).toEqual({ status: 'ok' })
    expect(await f.host.read({ viewerId: leases[1].viewerId }, 1)).toEqual({ status: 'unavailable' })
    for (const value of [leases[0], leases[2]]) expect(await f.host.read({ viewerId: value.viewerId }, 1)).toEqual(snapshot())
    expect(f.release).not.toHaveBeenCalled()
  })
  it('counts authorizing and active siblings together against the existing global quota', async () => {
    const f = fixture(), first = await f.authorize(), pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementation(async (_path: string, raw: string) =>
      JSON.parse(raw).action === 'authorize' ? pending.promise : reply(snapshot()))
    const controllers = Array.from({ length: 8 }, () => new AbortController())
    const releases = controllers.map(controller => vi.fn(() => controller.abort()))
    const requests = controllers.slice(0, 7).map((controller, index) => f.host.authorize({ ...request, paneId: 'pending-' + index },
      { ...f.owner, signal: controller.signal, release: releases[index], documentNonce: crypto.randomUUID() }))
    expect(await f.host.authorize({ ...request, paneId: 'overflow' }, { ...f.owner, signal: controllers[7].signal,
      release: releases[7], documentNonce: crypto.randomUUID() })).toEqual({ status: 'unavailable' })
    expect(releases[7]).toHaveBeenCalledOnce()
    expect(f.release).not.toHaveBeenCalled()
    pending.resolve(reply({ status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, revision: '0', scope }))
    expect((await Promise.all(requests)).every(result => result.status === 'authorized')).toBe(true)
    expect(await f.host.read({ viewerId: first.viewerId }, 1)).toEqual(snapshot())
  })
  it('checks owner again in the fresh-to-return microtask', async () => {
    const f = fixture(), authorized = await f.authorize(), pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(async () => pending.promise)
    const result = f.host.read({ viewerId: authorized.viewerId }, 1)
    pending.resolve(reply(snapshot()))
    queueMicrotask(f.revokeRuntime)
    expect(await result).toEqual({ status: 'unavailable' })
  })
  it('only opens a frozen capture and validates identity/digest before delivery', async () => {
    const f = fixture(), authorized = await f.authorize()
    const target = { viewerId: authorized.viewerId, blockId: block.blockId, captureDigest: block.captureDigest }
    expect(await f.host.openCapture(target, 1)).toEqual({ status: 'unavailable' })
    expect(f.transport).toHaveBeenCalledTimes(1)
    f.host.setExpanded({ viewerId: authorized.viewerId, expandedBlockIds: [block.blockId] }, 1)
    expect(await f.host.openCapture(target, 1)).toEqual({ status: 'capture', schemaVersion: 2, unit, revision: '1', scope, block })
    const wire = JSON.parse(f.transport.mock.calls[1][1])
    expect(wire).toEqual({ schemaVersion: 2, action: 'open_capture', viewerToken: expect.any(String), blockId: block.blockId, captureDigest: block.captureDigest })
    expect(await f.host.openCapture({ ...target, path: '/private/file' }, 1)).toEqual({ status: 'unavailable' })
    f.transport.mockResolvedValueOnce(reply({ status: 'capture', schemaVersion: 2, unit, revision: '1', scope, block: { ...block, captureDigest: 'd'.repeat(64) } }))
    expect(await f.host.openCapture(target, 1)).toEqual({ status: 'unavailable' })
  })
  it('cancels a pending capture on collapse and rejects a late target response', async () => {
    const f = fixture(), authorized = await f.authorize(), pending = deferred<ReturnType<typeof reply>>()
    const local = { viewerId: authorized.viewerId }
    f.host.setExpanded({ ...local, expandedBlockIds: [block.blockId] }, 1)
    f.transport.mockImplementationOnce(async () => pending.promise)
    const result = f.host.openCapture({ ...local, blockId: block.blockId, captureDigest: block.captureDigest }, 1)
    const signal = f.transport.mock.calls[1][2]!
    f.host.setExpanded({ ...local, expandedBlockIds: [] }, 1)
    expect(signal.aborted).toBe(true)
    pending.resolve(reply({ status: 'capture', schemaVersion: 2, unit, revision: '1', scope, block }))
    expect(await result).toEqual({ status: 'unavailable' })
    expect(f.release).not.toHaveBeenCalled()
  })
})
describe('native process IPC ownership', () => {
  function ipcFixture() {
    const f = fixture(), frame = { send: vi.fn() }, sender = Object.assign(new EventEmitter(), { id: 42, mainFrame: frame, isDestroyed: () => false })
    let visible = true, minimized = false, trusted = true
    const window = Object.assign(new EventEmitter(), { webContents: sender, isDestroyed: () => false,
      isVisible: () => visible, isMinimized: () => minimized })
    registerProcessLocalDisplayIpcV1(() => window as never, f.transport, () => ({ isCurrent: () => true }), () => trusted)
    const authorizeEnvelope = ipc.handlers.get('process-local:authorize')!
    ipc.handlers.set('process-local:authorize', async (event, payload) => {
      const envelope = await authorizeEnvelope(event, payload)
      return envelope?.result ?? envelope
    })
    const event = { sender, senderFrame: frame }
    return { ...f, sender, event, window, frame, authorizeEnvelope, hide: () => { visible = false; window.emit('hide') },
      hideWithoutEvent: () => { visible = false },
      minimize: () => { minimized = true; window.emit('minimize') }, untrust: () => { trusted = false } }
  }
  it.each(['global', 'case', 'hide', 'minimize', 'guard-loss'] as const)('notifies the native document of queued-reply retirement on %s', async reason => {
    const f = ipcFixture()
    const authorized = await ipc.handlers.get('process-local:authorize')!(f.event, request)
    expect(authorized.status).toBe('authorized')
    const local = { viewerId: authorized.viewerId }
    expect(await ipc.handlers.get('process-local:expand')!(f.event, { ...local, expandedBlockIds: [block.blockId] })).toEqual({ status: 'ok' })
    // Main has returned this body. The bridge tests hold its IPC delivery until
    // after invalidation; this test verifies the real Main control notification.
    const queued = await ipc.handlers.get('process-local:read')!(f.event, local)
    expect(queued).toEqual(snapshot(true))
    expect(f.frame.send).not.toHaveBeenCalled()
    if (reason === 'global') invalidateProcessLocalDisplaysV1()
    if (reason === 'case') await withProcessLocalDisplayTransitionV1(async () => undefined)
    if (reason === 'hide') f.hide()
    if (reason === 'minimize') f.minimize()
    if (reason === 'guard-loss') {
      f.hideWithoutEvent()
      expect(await ipc.handlers.get('process-local:read')!(f.event, local)).toEqual({ status: 'unavailable' })
    }
    expect(f.frame.send).toHaveBeenCalledTimes(reason === 'case' ? 2 : 1)
    expect(f.frame.send).toHaveBeenCalledWith('process-local:invalidated', expect.objectContaining({ schemaVersion: 2, documentEpoch: '1', authorityEpoch: expect.any(String) }))
    expect(await ipc.handlers.get('process-local:read')!(f.event, local)).toEqual({ status: 'unavailable' })
  })
  it('does not let pane replacement or an old close notify and retire its successor', async () => {
    const f = ipcFixture(), authorize = ipc.handlers.get('process-local:authorize')!
    const older = await authorize(f.event, request)
    const successor = await authorize(f.event, { ...request, paneId: request.paneId })
    expect(successor.status).toBe('authorized')
    expect(await ipc.handlers.get('process-local:close')!(f.event, { viewerId: older.viewerId })).toEqual({ status: 'unavailable' })
    expect(f.frame.send).not.toHaveBeenCalled()
    expect(await ipc.handlers.get('process-local:read')!(f.event, { viewerId: successor.viewerId })).toEqual(snapshot())
  })
  it('never grants a foreign or never-authorized frame a document notification record', async () => {
    const f = ipcFixture()
    const authorized = await ipc.handlers.get('process-local:authorize')!(f.event, request)
    expect(await ipc.handlers.get('process-local:read')!({ ...f.event, senderFrame: {} }, { viewerId: authorized.viewerId })).toEqual({ status: 'unavailable' })
    expect(f.frame.send).not.toHaveBeenCalled()
    const untrusted = ipcFixture()
    untrusted.untrust()
    expect(await ipc.handlers.get('process-local:authorize')!(untrusted.event, request)).toEqual({ status: 'unavailable' })
    invalidateProcessLocalDisplaysV1()
    expect(untrusted.frame.send).not.toHaveBeenCalled()
    expect(f.frame.send).toHaveBeenCalledWith('process-local:invalidated', expect.objectContaining({ schemaVersion: 2, documentEpoch: '1', authorityEpoch: expect.any(String) }))
  })
  it('clears an earlier authorized same-document copy when its principal retires', async () => {
    const f = ipcFixture()
    const authorized = await ipc.handlers.get('process-local:authorize')!(f.event, request)
    const local = { viewerId: authorized.viewerId }
    await ipc.handlers.get('process-local:expand')!(f.event, { ...local, expandedBlockIds: [block.blockId] })
    expect(await ipc.handlers.get('process-local:read')!(f.event, local)).toEqual(snapshot(true))
    f.untrust()
    expect(await ipc.handlers.get('process-local:read')!(f.event, local)).toEqual({ status: 'unavailable' })
    expect(f.frame.send).toHaveBeenCalledOnce()
    expect(f.frame.send).toHaveBeenCalledWith('process-local:invalidated', expect.objectContaining({ schemaVersion: 2, documentEpoch: '1', authorityEpoch: expect.any(String) }))
  })
  it('forgets a navigated document before global invalidation can reach a new document', async () => {
    const f = ipcFixture()
    expect((await ipc.handlers.get('process-local:authorize')!(f.event, request)).status).toBe('authorized')
    f.sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
    invalidateProcessLocalDisplaysV1()
    expect(f.frame.send).not.toHaveBeenCalled()
  })
  it('denies foreign frames, hidden/minimized windows and untrusted principals before reads', async () => {
    const f = ipcFixture(), authorize = ipc.handlers.get('process-local:authorize')!
    expect(await authorize({ ...f.event, senderFrame: {} }, request)).toEqual({ status: 'unavailable' })
    f.hide(); expect(await authorize(f.event, request)).toEqual({ status: 'unavailable' })
    const minimized = ipcFixture(); minimized.minimize()
    expect(await ipc.handlers.get('process-local:authorize')!(minimized.event, request)).toEqual({ status: 'unavailable' })
    const untrusted = ipcFixture(); untrusted.untrust()
    expect(await ipc.handlers.get('process-local:authorize')!(untrusted.event, request)).toEqual({ status: 'unavailable' })
    expect(f.transport).not.toHaveBeenCalled(); expect(minimized.transport).not.toHaveBeenCalled(); expect(untrusted.transport).not.toHaveBeenCalled()
  })
  it.each(['navigation', 'hide', 'minimize', 'principal'] as const)('aborts late native replies on %s and releases document listeners', async reason => {
    const f = ipcFixture(), authorize = ipc.handlers.get('process-local:authorize')!, read = ipc.handlers.get('process-local:read')!
    const authorized = await authorize(f.event, request)
    const pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(async () => pending.promise)
    const result = read(f.event, { viewerId: authorized.viewerId })
    const signal = f.transport.mock.calls[1][2]!
    if (reason === 'navigation') f.sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
    if (reason === 'hide') f.hide()
    if (reason === 'minimize') f.minimize()
    if (reason === 'principal') f.untrust()
    pending.resolve(reply(snapshot()))
    expect(await result).toEqual({ status: 'unavailable' })
    expect(signal.aborted).toBe(true)
    expect(await read(f.event, { viewerId: authorized.viewerId })).toEqual({ status: 'unavailable' })
    expect(f.sender.eventNames()).toEqual(reason === 'navigation' ? [] : ['did-start-navigation', 'render-process-gone', 'destroyed'])
  })
  it('blocks new authorization throughout a case transition and retires prior leases', async () => {
    const f = ipcFixture(), authorize = ipc.handlers.get('process-local:authorize')!, read = ipc.handlers.get('process-local:read')!
    const authorized = await authorize(f.event, request)
    await withProcessLocalDisplayTransitionV1(async () => {
      expect(await authorize(f.event, request)).toEqual({ status: 'unavailable' })
      expect(await read(f.event, { viewerId: authorized.viewerId })).toEqual({ status: 'unavailable' })
    })
    expect(await read(f.event, { viewerId: authorized.viewerId })).toEqual({ status: 'unavailable' })
  })
  it('retains the independent document watch after the latest pane closes and fences native navigation', async () => {
    const f = ipcFixture(), authorize = ipc.handlers.get('process-local:authorize')!
    const first = await authorize(f.event, request)
    const latest = await authorize(f.event, { ...request, paneId: 'latest' })
    expect(await ipc.handlers.get('process-local:close')!(f.event, { viewerId: latest.viewerId })).toEqual({ status: 'ok' })
    expect(f.sender.listenerCount('did-start-navigation')).toBeGreaterThan(0)
    f.sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
    expect(await ipc.handlers.get('process-local:read')!(f.event, { viewerId: first.viewerId })).toEqual({ status: 'unavailable' })
    expect(f.sender.eventNames()).toEqual([])
    invalidateProcessLocalDisplaysV1()
    expect(f.frame.send).not.toHaveBeenCalled()
  })
  it('stamps authorization with its actual native document and pre-reply authority epoch', async () => {
    const f = ipcFixture(), pending = deferred<ReturnType<typeof reply>>()
    f.transport.mockImplementationOnce(async () => pending.promise)
    const old = f.authorizeEnvelope(f.event, request)
    invalidateProcessLocalDisplaysV1()
    const fresh = await f.authorizeEnvelope(f.event, { ...request, paneId: 'fresh' })
    expect(fresh.result.status).toBe('authorized')
    expect(fresh.control).toEqual({ schemaVersion: 2, documentEpoch: '1', authorityEpoch: '1' })
    pending.resolve(reply({ status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, revision: '0', scope }))
    expect(await old).toEqual({ control: { schemaVersion: 2, documentEpoch: '1', authorityEpoch: '0' }, result: { status: 'unavailable' } })
    expect(await ipc.handlers.get('process-local:read')!(f.event, { viewerId: fresh.result.viewerId })).toEqual(snapshot())
    f.sender.emit('did-start-navigation', {}, 'analytix://app', false, true)
    const successor = await f.authorizeEnvelope(f.event, request)
    expect(successor.control.documentEpoch).toBe('2')
    expect(BigInt(successor.control.authorityEpoch)).toBeGreaterThan(1n)
  })

  it('synchronously fences queued body copies on in-place native navigation before the document watch retires', async () => {
    const f = ipcFixture(), first = await f.authorizeEnvelope(f.event, request)
    expect(first.result.status).toBe('authorized')
    const local = { viewerId: first.result.viewerId }
    await ipc.handlers.get('process-local:expand')!(f.event, { ...local, expandedBlockIds: [block.blockId] })
    expect(await ipc.handlers.get('process-local:read')!(f.event, local)).toEqual(snapshot(true))
    f.sender.emit('did-start-navigation', {}, 'analytix://app#same-dom', true, true)
    expect(f.frame.send).toHaveBeenCalledOnce()
    expect(f.frame.send).toHaveBeenCalledWith('process-local:invalidated', { schemaVersion: 2, documentEpoch: '1', authorityEpoch: '1' })
    expect(await ipc.handlers.get('process-local:read')!(f.event, local)).toEqual({ status: 'unavailable' })
    const fresh = await f.authorizeEnvelope(f.event, request)
    expect(fresh.result.status).toBe('authorized')
    expect(fresh.control.documentEpoch).toBe('2')
    expect(BigInt(fresh.control.authorityEpoch)).toBeGreaterThan(1n)
    expect(f.frame.send).toHaveBeenCalledOnce()
  })

})

describe('active Web link under existing process lease', () => {
  const link = { status: 'web_link', schemaVersion: 2, revision: '1', scope, blockId: 'web-input-a',
    inputDigest: 'd'.repeat(64), role: 'request', targetUrl: 'https://synthetic.invalid/actual' }
  it('resolves only a sealed active input target twice and returns no URL', async () => {
    const f = fixture(), authorized = await f.authorize(), openExternal = vi.fn(async () => undefined)
    f.transport.mockResolvedValue(reply(link))
    const payload = { viewerId: authorized.viewerId, blockId: link.blockId, inputDigest: link.inputDigest, role: 'request' }
    expect(await f.host.openWebLink(payload, 1, openExternal)).toEqual({ status: 'ok' })
    expect(openExternal).toHaveBeenCalledWith(link.targetUrl)
    expect(f.transport).toHaveBeenCalledTimes(3)
    for (const [, raw] of f.transport.mock.calls.slice(1)) {
      expect(JSON.parse(raw)).toMatchObject({ schemaVersion: 2, action: 'open_web_link', blockId: link.blockId, inputDigest: link.inputDigest, role: 'request' })
      expect(JSON.parse(raw)).not.toHaveProperty('targetUrl')
    }
    expect(await f.host.openWebLink({ ...payload, url: 'https://forged.invalid' }, 1, openExternal)).toEqual({ status: 'unavailable' })
    expect(openExternal).toHaveBeenCalledTimes(1)
  })
  it.each(['url', 'digest', 'scope', 'runtime', 'document', 'scheme'] as const)('rejects changed %s before an OS effect', async reason => {
    const f = fixture(), authorized = await f.authorize(), openExternal = vi.fn(async () => undefined)
    f.transport.mockResolvedValueOnce(reply(link))
    f.transport.mockImplementationOnce(async () => {
      if (reason === 'runtime') f.revokeRuntime()
      if (reason === 'document') f.revokeDocument()
      return reply({ ...link,
        ...(reason === 'url' ? { targetUrl: 'https://replacement.invalid' } : {}),
        ...(reason === 'digest' ? { inputDigest: 'f'.repeat(64) } : {}),
        ...(reason === 'scope' ? { scope: { ...scope, contextEpoch: '2' } } : {}),
        ...(reason === 'scheme' ? { targetUrl: 'file:///synthetic/source' } : {}) })
    })
    expect(await f.host.openWebLink({ viewerId: authorized.viewerId, blockId: link.blockId, inputDigest: link.inputDigest, role: 'request' }, 1, openExternal)).toEqual({ status: 'unavailable' })
    expect(openExternal).not.toHaveBeenCalled()
  })
  it('retains the existing lease on a transient busy target with zero OS effect', async () => {
    const f = fixture(), authorized = await f.authorize(), openExternal = vi.fn(async () => undefined)
    const payload = { viewerId: authorized.viewerId, blockId: link.blockId, inputDigest: link.inputDigest, role: 'request' }
    f.transport.mockResolvedValueOnce(reply({ status: 'busy' }))
    expect(await f.host.openWebLink(payload, 1, openExternal)).toEqual({ status: 'busy' })
    expect(f.release).not.toHaveBeenCalled()
    f.transport.mockResolvedValue(reply(link))
    expect(await f.host.openWebLink(payload, 1, openExternal)).toEqual({ status: 'ok' })
    expect(openExternal).toHaveBeenCalledOnce()
  })
  it('drops a late target after process-pane expansion changes its epoch', async () => {
    const f = fixture(), authorized = await f.authorize(), pending = deferred<ReturnType<typeof reply>>(), openExternal = vi.fn(async () => undefined)
    f.transport.mockImplementationOnce(async () => pending.promise)
    const result = f.host.openWebLink({ viewerId: authorized.viewerId, blockId: link.blockId, inputDigest: link.inputDigest, role: 'request' }, 1, openExternal)
    f.host.setExpanded({ viewerId: authorized.viewerId, expandedBlockIds: [] }, 1)
    pending.resolve(reply(link))
    expect(await result).toEqual({ status: 'unavailable' })
    expect(openExternal).not.toHaveBeenCalled()
  })
})
