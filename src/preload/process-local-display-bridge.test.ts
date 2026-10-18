import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AnalytixApi } from '../shared/analytix-api'
import { createRequire } from 'node:module'
const { JSDOM } = createRequire(import.meta.url)('jsdom') as { JSDOM: new (html: string) => { window: { document: Document } } }
import { createRuntimeStatusPublicV1 } from '../shared/analytix-runtime-status'
import type { ProcessDisplayBlockV1 } from '../../packages/runtime/src/contracts/protected-process-local-display'

const electronMock = vi.hoisted(() => {
  const exposedApi = new Map<string, unknown>()
  return {
    exposedApi,
    exposeInMainWorld: vi.fn((name: string, api: unknown) => exposedApi.set(name, api)),
    invoke: vi.fn(),
    on: vi.fn(),
    removeListener: vi.fn(),
    getPathForFile: vi.fn()
  }
})

vi.mock('electron', () => ({
  contextBridge: { exposeInMainWorld: electronMock.exposeInMainWorld },
  ipcRenderer: {
    invoke: (channel: string, ...args: unknown[]) => {
      const control = controlFixtureV2()
      return Promise.resolve(electronMock.invoke(channel, ...args)).then(value =>
        channel === 'process-local:authorize' && !value?.control ? { control, result: value } : value)
    },
    on: electronMock.on,
    removeListener: electronMock.removeListener
  },
  webUtils: { getPathForFile: electronMock.getPathForFile }
}))

let controlAuthority = 0
function controlFixtureV2() { return { schemaVersion: 2, documentEpoch: '1', authorityEpoch: String(controlAuthority) } }
function invalidateControlV2() { controlAuthority += 1; emitIpc('process-local:invalidated', controlFixtureV2()) }
function nativeDocumentFixtureV2() {
  const doc = new JSDOM('<main><div data-process-pane-id="pane-a"></div><div data-process-pane-id="pane-old"></div><div data-process-pane-id="pane-new"></div></main>').window.document
  Object.defineProperty(doc, 'visibilityState', { value: 'visible', writable: true, configurable: true })
  return doc
}
const viewerId = 'f3b07ec5-e8ea-4d81-965f-a317fa986768'
const captureDigest = 'a'.repeat(64)
const scope = { threadId: 'thread-a', turnId: 'turn-a', contextDigest: 'b'.repeat(64), contextEpoch: '1', processIncarnation: 'incarnation-a' }
const privateMarker = 'synthetic-protected-process-value'
const source = { kind: 'tool' as const, unitId: 'unit-a', callId: 'call-a', toolCallItemId: 'call-item-a', toolResultItemId: 'result-item-a', startedEventSeq: '12' }
const unit = { unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', placement: 'anchored', calls: [{ callId: source.callId, toolCallItemId: source.toolCallItemId, toolResultItemId: source.toolResultItemId, startedEventSeq: source.startedEventSeq }], precedingCalls: [] }
const block: ProcessDisplayBlockV1 = {
  blockId: 'block-a', source, kind: 'shell', state: 'completed', attempt: 0,
  durationMs: 3, exitCode: 0, truncated: false,
  classification: 'source_exact_local', captureDigest,
  body: {
    text: privateMarker, before: '', after: '', error: '',
    addedLines: 0, removedLines: 0, approximate: false
  }
}
const authorized = { status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, viewerId, revision: '1', scope }
const snapshot = { status: 'snapshot', schemaVersion: 2, units: [unit], revision: '2', scope, blocks: [block] }
const capture = { status: 'capture', schemaVersion: 2, unit, revision: '2', scope, block }

type ProcessMethod =
  | 'authorizeProcessPane'
  | 'readProcessPane'
  | 'setProcessPaneExpanded'
  | 'closeProcessPane'
  | 'openProcessCapture'
const operations: Array<{ method: ProcessMethod; channel: string; request: unknown; result: unknown }> = [
  {
    method: 'authorizeProcessPane', channel: 'process-local:authorize',
    request: { paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' }, result: authorized
  },
  {
    method: 'setProcessPaneExpanded', channel: 'process-local:expand',
    request: { viewerId, expandedBlockIds: ['block-a'] }, result: { status: 'ok' }
  },
  {
    method: 'readProcessPane', channel: 'process-local:read',
    request: { viewerId, afterRevision: '1', waitMs: 25_000 }, result: snapshot
  },
  {
    method: 'openProcessCapture', channel: 'process-local:target',
    request: { viewerId, blockId: 'block-a', captureDigest }, result: capture
  },
  {
    method: 'closeProcessPane', channel: 'process-local:close',
    request: { viewerId }, result: { status: 'ok' }
  }
]

async function loadApi(): Promise<AnalytixApi> {
  vi.resetModules()
  await import('./index')
  const api = electronMock.exposedApi.get('analytix')
  expect(electronMock.exposeInMainWorld).toHaveBeenCalledWith('analytix', api)
  return api as AnalytixApi
}

function callProcess(api: AnalytixApi, method: ProcessMethod, request: unknown): Promise<unknown> {
  return (api.runtime[method] as (value: never) => Promise<unknown>)(request as never)
}

async function authorizeViewer(api: AnalytixApi): Promise<void> {
  electronMock.invoke.mockReset()
  electronMock.invoke.mockImplementation((channel: string) =>
    Promise.resolve(channel === 'process-local:authorize' ? authorized : { status: 'ok' }))
  await expect(api.runtime.authorizeProcessPane({
    paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a'
  })).resolves.toEqual(authorized)
  await expect(api.runtime.setProcessPaneExpanded({
    viewerId, expandedBlockIds: ['block-a']
  })).resolves.toEqual({ status: 'ok' })
  electronMock.invoke.mockClear()
}

async function prepareOperation(api: AnalytixApi, method: ProcessMethod): Promise<void> {
  await authorizeViewer(api)
  if (method === 'authorizeProcessPane') {
    await expect(api.runtime.closeProcessPane({ viewerId })).resolves.toEqual({ status: 'ok' })
    electronMock.invoke.mockClear()
  }
}

function deferredReply(): { promise: Promise<unknown>; resolve: (value: unknown) => void } {
  let resolve!: (value: unknown) => void
  const promise = new Promise<unknown>((finish) => { resolve = finish })
  return { promise, resolve }
}

function emitIpc(channel: string, value: unknown): void {
  const listeners = electronMock.on.mock.calls.filter(([registered]) => registered === channel)
  expect(listeners.length).toBeGreaterThan(0)
  for (const [, listener] of listeners) listener({}, value)
}

type ProcessInvalidation = { boundary: string; invalidate: (api: AnalytixApi) => unknown }
const invocationInvalidations: ProcessInvalidation[] = [
  { boundary: 'settings:set', invalidate: api => api.settings.setSettings({}) },
  { boundary: 'settings:save-silent', invalidate: api => api.settings.saveSettingsSilent({}) },
  { boundary: 'runtime:restart', invalidate: api => api.runtime.restartRuntime() },
  { boundary: 'data-analysis:ensure-workspace-case', invalidate: api =>
    api.dataAnalysis.ensureWorkspaceCase({ workspaceRoot: '/synthetic-process-workspace' }) }
]
const processInvalidations: ProcessInvalidation[] = [
  ...invocationInvalidations,
  { boundary: 'process-local:invalidated', invalidate: () => invalidateControlV2() },
  { boundary: 'runtime:status', invalidate: () => emitIpc('runtime:status',
    createRuntimeStatusPublicV1({ code: 'runtime_starting', source: 'restart' })) }
]
const queuedInvalidationCases = processInvalidations.flatMap(invalidation =>
  (['authorizeProcessPane', 'readProcessPane', 'openProcessCapture'] as const)
    .map(method => ({ ...invalidation, method })))

describe('protected process preload bridge', () => {
  const draft = { blockId: 'draft-a', source: { kind: 'assistant_draft', unitId: unit.unitId, producerId: unit.producerId, providerSequence: unit.providerSequence, attempt: 1 },
    kind: 'assistant_draft', state: 'running', attempt: 1, durationMs: 0, truncated: false, classification: 'source_exact_local', contentClassification: 'provider_text',
    draftSegments: [{ offsetBytes: '0', text: 'PRIVATE_DRAFT ' }], draftBytes: '14' }
  beforeEach(() => {
    controlAuthority = 0
    electronMock.exposedApi.clear()
    electronMock.exposeInMainWorld.mockClear()
    electronMock.invoke.mockReset()
    electronMock.on.mockReset()
    electronMock.removeListener.mockReset()
    electronMock.getPathForFile.mockReset()
  })
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
  it('bridges active Web link input identity only under the existing visible viewer lease', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi()
    await authorizeViewer(api)
    const request = { viewerId, blockId: 'web-input-a', inputDigest: 'd'.repeat(64), role: 'request' as const }
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    expect(await api.runtime.openProcessWebLink(request)).toEqual({ status: 'ok' })
    expect(electronMock.invoke).toHaveBeenCalledWith('process-local:web-link', request)
    electronMock.invoke.mockClear()
    expect(await api.runtime.openProcessWebLink({ ...request, url: 'https://forged.invalid' } as never)).toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok', targetUrl: 'https://private.invalid' })
    expect(await api.runtime.openProcessWebLink(request)).toEqual({ status: 'unavailable' })
    expect(await api.runtime.openProcessWebLink(request)).toEqual({ status: 'unavailable' })
  })
  it('drops late active-link acknowledgements after document authority is retired', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi()
    await authorizeViewer(api)
    const pending = deferredReply()
    electronMock.invoke.mockImplementationOnce(() => pending.promise)
    const result = api.runtime.openProcessWebLink({ viewerId, blockId: 'web-input-a', inputDigest: 'd'.repeat(64), role: 'request' })
    invalidateControlV2()
    pending.resolve({ status: 'ok' })
    expect(await result).toEqual({ status: 'unavailable' })
  })
  it.each(['tool_input', 'shell', 'web_fetch'] as const)('uses the existing expanded facade fence for complete running %s input', async kind => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(); await authorizeViewer(api)
    const inputSource = { ...source, toolResultItemId: undefined }
    const raw: ProcessDisplayBlockV1 = { blockId: 'raw-input', source: inputSource, kind, state: 'running', attempt: 0,
      durationMs: 0, truncated: false, classification: 'source_exact_local', input: { availability: 'available', inputJSON: ' {"number":9007199254740993} ' } }
    const inputAnchor = { callId: source.callId, toolCallItemId: source.toolCallItemId, startedEventSeq: source.startedEventSeq }
    const value = { ...snapshot, units: [{ ...unit, calls: [inputAnchor] }], blocks: [raw] }
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [raw.blockId] })
    electronMock.invoke.mockResolvedValueOnce(value)
    expect(await api.runtime.readProcessPane({ viewerId })).toEqual(value)
    const pending = deferredReply(); electronMock.invoke.mockImplementationOnce(() => pending.promise)
    const late = api.runtime.readProcessPane({ viewerId })
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [] })
    pending.resolve(value)
    expect(await late).toEqual({ status: 'unavailable' })
    electronMock.invoke.mockResolvedValueOnce(value)
    expect(await api.runtime.readProcessPane({ viewerId })).toEqual({ status: 'unavailable' })
  })
  it('keeps running draft bytes on the native expanded facade and rejects a full-source reply to draft-only discovery', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi()
    await authorizeViewer(api)
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [draft.blockId] })
    const value = { ...snapshot, blocks: [draft] }
    electronMock.invoke.mockResolvedValueOnce(value)
    expect(await api.runtime.readProcessPane({ viewerId, draftsOnly: true })).toEqual(value)
    expect(api).not.toHaveProperty('readProcessPane')
    electronMock.invoke.mockResolvedValueOnce({ ...snapshot, blocks: [{ ...block, body: undefined }] })
    expect(await api.runtime.readProcessPane({ viewerId, draftsOnly: true })).toEqual({ status: 'unavailable' })
    expect(await api.runtime.readProcessPane({ viewerId })).toEqual({ status: 'unavailable' })
  })
  it.each([false, true])('rejects unsolicited draft payload for draftsOnly=%s and retires the native facade handle', async draftsOnly => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi()
    await authorizeViewer(api)
    electronMock.invoke.mockResolvedValueOnce({ ...snapshot, blocks: [draft] })
    expect(await api.runtime.readProcessPane({ viewerId, draftsOnly })).toEqual({ status: 'unavailable' })
    const count = electronMock.invoke.mock.calls.length
    expect(await api.runtime.readProcessPane({ viewerId })).toEqual({ status: 'unavailable' })
    expect(electronMock.invoke.mock.calls).toHaveLength(count)
  })

  it('returns validated snapshots and frozen captures only through dedicated runtime methods', async () => {
    const api = await loadApi()
    for (const operation of operations) {
      electronMock.invoke.mockResolvedValueOnce(operation.result)
      await expect(callProcess(api, operation.method, operation.request)).resolves.toEqual(operation.result)
      expect(electronMock.invoke).toHaveBeenCalledWith(operation.channel, operation.request)
      expect(api).not.toHaveProperty(operation.method)
      expect(api.diagnostics).not.toHaveProperty(operation.method)
    }
    expect(electronMock.invoke.mock.calls.map(([channel]) => channel)).toEqual(operations.map(({ channel }) => channel))
    expect(electronMock.on.mock.calls.map(([channel]) => channel).filter(channel => String(channel).startsWith('process-local:')))
      .toEqual(['process-local:invalidated'])
    expect(api).not.toHaveProperty('onProcessPaneInvalidated')
    expect(api.diagnostics).not.toHaveProperty('onProcessPaneInvalidated')
    expect(electronMock.invoke).not.toHaveBeenCalledWith('runtime:request', expect.anything())
    expect(electronMock.invoke).not.toHaveBeenCalledWith('runtime:sse:start', expect.anything())
  })

  it('preserves unchanged and closed statuses without a private payload', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const unchanged = { status: 'unchanged', schemaVersion: 2, revision: '2', scope }
    electronMock.invoke.mockResolvedValueOnce(unchanged)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(unchanged)
    for (const status of ['unavailable', 'busy', 'revoked']) {
      for (const operation of operations) {
        await prepareOperation(api, operation.method)
        electronMock.invoke.mockResolvedValueOnce({ status })
        await expect(callProcess(api, operation.method, operation.request)).resolves.toEqual({ status })
      }
    }
  })

  it('rejects malformed selectors and out-of-bound polling or expansion before IPC', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const invalid: Array<[ProcessMethod, unknown]> = [
      ['authorizeProcessPane', { paneId: '', threadId: 'thread-a', turnId: 'turn-a' }],
      ['authorizeProcessPane', { paneId: 'pane-a', threadId: '../thread', turnId: 'turn-a' }],
      ['authorizeProcessPane', { paneId: 'pane-a', threadId: 'thread-a' }],
      ['readProcessPane', { viewerId: 'not-a-viewer' }],
      ['readProcessPane', { viewerId, afterRevision: '01' }],
      ['readProcessPane', { viewerId, afterRevision: '-1' }],
      ['readProcessPane', { viewerId, afterRevision: '1.0' }],
      ['readProcessPane', { viewerId, afterRevision: '18446744073709551616' }],
      ['readProcessPane', { viewerId, waitMs: -1 }],
      ['readProcessPane', { viewerId, waitMs: 25_001 }],
      ['readProcessPane', { viewerId, waitMs: 1.5 }],
      ['setProcessPaneExpanded', { viewerId, expandedBlockIds: ['block-a', 'block-a'] }],
      ['setProcessPaneExpanded', { viewerId, expandedBlockIds: Array.from({ length: 129 }, (_, i) => `block-${i}`) }],
      ['setProcessPaneExpanded', { viewerId, expandedBlockIds: ['../block'] }],
      ['closeProcessPane', { viewerId: '' }],
      ['openProcessCapture', { viewerId, blockId: 'block-a', captureDigest: 'invalid' }],
      ['openProcessCapture', { viewerId, blockId: '../block', captureDigest }]
    ]
    for (const [method, request] of invalid) {
      await expect(callProcess(api, method, request)).resolves.toEqual({ status: 'unavailable' })
    }
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('accepts the defined polling and expansion limits and collapse with no expanded blocks', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    electronMock.invoke.mockResolvedValueOnce({ status: 'unchanged', schemaVersion: 2, revision: '18446744073709551615', scope })
    await expect(api.runtime.readProcessPane({
      viewerId, afterRevision: '18446744073709551615', waitMs: 25_000
    })).resolves.toEqual({ status: 'unchanged', schemaVersion: 2, revision: '18446744073709551615', scope })
    const expandedBlockIds = Array.from({ length: 128 }, (_, i) => `block-${i}`)
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await expect(api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds })).resolves.toEqual({ status: 'ok' })
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await expect(api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [] })).resolves.toEqual({ status: 'ok' })
    expect(electronMock.invoke).toHaveBeenLastCalledWith('process-local:expand', { viewerId, expandedBlockIds: [] })
  })

  it('rejects unknown request fields, private Go identity and caller-provided paths or bodies', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    for (const operation of operations) {
      for (const field of ['unexpected', 'viewerToken', 'documentNonce', 'principal', 'path', 'body', 'authorized']) {
        const request = { ...(operation.request as Record<string, unknown>), [field]: privateMarker }
        await expect(callProcess(api, operation.method, request)).resolves.toEqual({ status: 'unavailable' })
      }
    }
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('discards invalid and unknown output fields instead of leaking private reply values', async () => {
    const api = await loadApi()
    const invalid: Array<[ProcessMethod, unknown]> = [
      ['authorizeProcessPane', { ...authorized, viewerToken: privateMarker }],
      ['authorizeProcessPane', { ...authorized, documentNonce: privateMarker }],
      ['authorizeProcessPane', { ...authorized, viewerId: 'invalid' }],
      ['authorizeProcessPane', { ...authorized, scope: { ...scope, principal: privateMarker } }],
      ['readProcessPane', { ...snapshot, body: privateMarker }],
      ['readProcessPane', { ...snapshot, viewerToken: privateMarker }],
      ['readProcessPane', { ...snapshot, revision: '01' }],
      ['readProcessPane', { ...snapshot, scope: { ...scope, documentNonce: privateMarker } }],
      ['readProcessPane', { ...snapshot, blocks: [block, block] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, chunk: privateMarker }] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, body: { ...block.body, viewerToken: privateMarker } }] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, body: { text: privateMarker } }] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, state: 'running' }] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, classification: 'withheld' }] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, body: { ...block.body, text: 'x'.repeat((32 << 10) + 1) } }] }],
      ['readProcessPane', { ...snapshot, blocks: [{ ...block, body: { ...block.body, text: '\u0000' } }] }],
      ['readProcessPane', { status: 'unchanged', schemaVersion: 2, revision: '2', scope, body: privateMarker }],
      ['readProcessPane', { status: 'streaming', delta: privateMarker }],
      ['openProcessCapture', { ...capture, viewerToken: privateMarker }],
      ['openProcessCapture', { ...capture, block: { ...block, path: privateMarker } }],
      ['openProcessCapture', { ...capture, block: { ...block, body: { ...block.body, unexpected: privateMarker } } }],
      ['openProcessCapture', { ...capture, block: { ...block, captureDigest: undefined } }],
      ['setProcessPaneExpanded', { status: 'ok', body: privateMarker }],
      ['closeProcessPane', { status: 'revoked', documentNonce: privateMarker }]
    ]
    for (const [method, response] of invalid) {
      await prepareOperation(api, method)
      const operation = operations.find((entry) => entry.method === method)!
      electronMock.invoke.mockResolvedValueOnce(response)
      const result = await callProcess(api, method, operation.request)
      expect(result).toEqual({ status: 'unavailable' })
      expect(JSON.stringify(result)).not.toContain(privateMarker)
      expect(electronMock.invoke).toHaveBeenCalledWith(operation.channel, operation.request)
    }
  })

  it('closes every method on rejected IPC without returning or logging raw errors', async () => {
    const api = await loadApi()
    const errorLog = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const warningLog = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    for (const operation of operations) {
      await prepareOperation(api, operation.method)
      electronMock.invoke.mockRejectedValueOnce(new Error(privateMarker))
      const result = await callProcess(api, operation.method, operation.request)
      expect(result).toEqual({ status: 'unavailable' })
      expect(JSON.stringify(result)).not.toContain(privateMarker)
      expect(electronMock.invoke).toHaveBeenCalledTimes(['readProcessPane', 'openProcessCapture', 'setProcessPaneExpanded'].includes(operation.method) ? 2 : 1)
      expect(electronMock.invoke).toHaveBeenCalledWith(operation.channel, operation.request)
    }
    expect(errorLog).not.toHaveBeenCalled()
    expect(warningLog).not.toHaveBeenCalled()
  })

  it('closes every method if IPC throws synchronously', async () => {
    const api = await loadApi()
    for (const operation of operations) {
      await prepareOperation(api, operation.method)
      electronMock.invoke.mockImplementation(() => { throw new Error(privateMarker) })
      await expect(callProcess(api, operation.method, operation.request)).resolves.toEqual({ status: 'unavailable' })
      expect(electronMock.invoke).toHaveBeenCalledTimes(['readProcessPane', 'openProcessCapture', 'setProcessPaneExpanded'].includes(operation.method) ? 2 : 1)
      expect(electronMock.invoke).toHaveBeenCalledWith(operation.channel, operation.request)
    }
  })

  it('also closes when request validation itself throws before IPC', async () => {
    const api = await loadApi()
    const request = Object.defineProperty({ threadId: 'thread-a', turnId: 'turn-a' }, 'paneId', {
      enumerable: true,
      get: () => { throw new Error(privateMarker) }
    })
    await expect(callProcess(api, 'authorizeProcessPane', request)).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('requires a currently authorized local viewer before any read, target or lifecycle IPC', async () => {
    const api = await loadApi()
    for (const operation of operations.filter(({ method }) => method !== 'authorizeProcessPane')) {
      await expect(callProcess(api, operation.method, operation.request)).resolves.toEqual({ status: 'unavailable' })
    }
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('discards queued read and target bodies after collapse while preserving the viewer', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const read = deferredReply()
    const target = deferredReply()
    const expand = deferredReply()
    electronMock.invoke.mockImplementation((channel: string) => {
      if (channel === 'process-local:read') return read.promise
      if (channel === 'process-local:target') return target.promise
      if (channel === 'process-local:expand') return expand.promise
      return Promise.resolve({ status: 'ok' })
    })
    const pendingRead = api.runtime.readProcessPane({ viewerId })
    const pendingTarget = api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })
    const pendingCollapse = api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [] })
    read.resolve(snapshot)
    target.resolve(capture)
    await expect(pendingRead).resolves.toEqual({ status: 'unavailable' })
    await expect(pendingTarget).resolves.toEqual({ status: 'unavailable' })
    expand.resolve({ status: 'ok' })
    await expect(pendingCollapse).resolves.toEqual({ status: 'ok' })
    expect(electronMock.invoke.mock.calls.some(([channel]) => channel === 'process-local:close')).toBe(false)
    const unchanged = { status: 'unchanged', schemaVersion: 2, revision: '2', scope }
    electronMock.invoke.mockResolvedValueOnce(unchanged)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(unchanged)
  })

  it('discards queued read and target bodies immediately when close starts', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const read = deferredReply()
    const target = deferredReply()
    const close = deferredReply()
    electronMock.invoke.mockImplementation((channel: string) => {
      if (channel === 'process-local:read') return read.promise
      if (channel === 'process-local:target') return target.promise
      return close.promise
    })
    const pendingRead = api.runtime.readProcessPane({ viewerId })
    const pendingTarget = api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })
    const pendingClose = api.runtime.closeProcessPane({ viewerId })
    read.resolve(snapshot)
    target.resolve(capture)
    await expect(pendingRead).resolves.toEqual({ status: 'unavailable' })
    await expect(pendingTarget).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    close.resolve({ status: 'ok' })
    await expect(pendingClose).resolves.toEqual({ status: 'ok' })
    expect(electronMock.invoke).toHaveBeenCalledTimes(3)
  })

  it('retires a replaced pane and closes its late authorization without reviving it', async () => {
    const api = await loadApi()
    const first = deferredReply()
    const secondId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    const secondAuthorized = { ...authorized, viewerId: secondId }
    electronMock.invoke
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(secondAuthorized)
      .mockResolvedValueOnce({ status: 'ok' })
    const pendingFirst = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })
    await expect(api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })).resolves.toEqual(secondAuthorized)
    first.resolve(authorized)
    await expect(pendingFirst).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).toHaveBeenLastCalledWith('process-local:close', { viewerId })
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
    electronMock.invoke.mockResolvedValueOnce({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
    await expect(api.runtime.readProcessPane({ viewerId: secondId })).resolves.toEqual({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
    expect(electronMock.invoke).toHaveBeenLastCalledWith('process-local:read', { viewerId: secondId })
  })

  it('retires active-viewer reads when the same pane authorization begins', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const read = deferredReply()
    const successor = deferredReply()
    const secondId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    electronMock.invoke.mockImplementation((channel: string) => {
      if (channel === 'process-local:read') return read.promise
      if (channel === 'process-local:authorize') return successor.promise
      return Promise.resolve({ status: 'ok' })
    })
    const pendingRead = api.runtime.readProcessPane({ viewerId })
    const pendingSuccessor = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })
    expect(electronMock.invoke).toHaveBeenCalledWith('process-local:close', { viewerId })
    read.resolve(snapshot)
    await expect(pendingRead).resolves.toEqual({ status: 'unavailable' })
    successor.resolve({ ...authorized, viewerId: secondId })
    await expect(pendingSuccessor).resolves.toEqual({ ...authorized, viewerId: secondId })
  })

  it('does not let an old authorization failure or close reply revoke its successor', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const oldClose = deferredReply()
    const oldAuth = deferredReply()
    const secondId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    const successorAuthorized = { ...authorized, viewerId: secondId }
    electronMock.invoke
      .mockReturnValueOnce(oldClose.promise)
      .mockReturnValueOnce(oldAuth.promise)
      .mockResolvedValueOnce(successorAuthorized)
    const pendingClose = api.runtime.closeProcessPane({ viewerId })
    const pendingAuth = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })
    await expect(api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })).resolves.toEqual(successorAuthorized)
    oldClose.resolve({ status: 'revoked' })
    oldAuth.resolve({ status: 'revoked' })
    await expect(pendingClose).resolves.toEqual({ status: 'revoked' })
    await expect(pendingAuth).resolves.toEqual({ status: 'unavailable' })
    electronMock.invoke.mockResolvedValueOnce({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
    await expect(api.runtime.readProcessPane({ viewerId: secondId })).resolves.toEqual({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
  })

  it('does not send a retired authorization close from a changed native document context', async () => {
    const oldDocument = nativeDocumentFixtureV2()
    vi.stubGlobal('document', oldDocument)
    const api = await loadApi()
    const first = deferredReply()
    electronMock.invoke.mockReturnValueOnce(first.promise)
    const pending = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })
    vi.stubGlobal('document', {})
    first.resolve(authorized)
    await expect(pending).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).toHaveBeenCalledTimes(1)
  })

  it('rejects bodies outside the local expansion set and does not invoke an unexpanded target', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    await expect(api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [] })).resolves.toEqual({ status: 'ok' })
    electronMock.invoke.mockClear()
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).toHaveBeenCalledTimes(2)
    expect(electronMock.invoke).toHaveBeenLastCalledWith('process-local:close', { viewerId })
    await authorizeViewer(api)
    await api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [] })
    const { body: _body, ...descriptor } = block
    const descriptors = { ...snapshot, blocks: [descriptor] }
    electronMock.invoke.mockResolvedValueOnce(descriptors)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(descriptors)
  })

  it('retires the local lease and clears expansion when collapse IPC rejects', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    electronMock.invoke.mockRejectedValueOnce(new Error(privateMarker))
    await expect(api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: [] })).resolves.toEqual({ status: 'unavailable' })
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('irreversibly fences queued bodies when the native document hides then becomes visible', async () => {
    const nativeDocument = nativeDocumentFixtureV2() as Document & { visibilityState: string }
    nativeDocument.visibilityState = 'visible'
    vi.stubGlobal('document', nativeDocument)
    const api = await loadApi()
    await authorizeViewer(api)
    const read = deferredReply()
    const target = deferredReply()
    electronMock.invoke.mockImplementation((channel: string) =>
      channel === 'process-local:read' ? read.promise : target.promise)
    const pendingRead = api.runtime.readProcessPane({ viewerId })
    const pendingTarget = api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })
    nativeDocument.visibilityState = 'hidden'
    nativeDocument.dispatchEvent(new nativeDocument.defaultView!.Event('visibilitychange'))
    nativeDocument.visibilityState = 'visible'
    nativeDocument.dispatchEvent(new nativeDocument.defaultView!.Event('visibilitychange'))
    read.resolve(snapshot)
    target.resolve(capture)
    await expect(pendingRead).resolves.toEqual({ status: 'unavailable' })
    await expect(pendingTarget).resolves.toEqual({ status: 'unavailable' })
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
    await authorizeViewer(api)
    const unchanged = { status: 'unchanged', schemaVersion: 2, revision: '2', scope }
    electronMock.invoke.mockResolvedValueOnce(unchanged)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(unchanged)
  })

  it('does not authorize or read from an already hidden native document', async () => {
    vi.stubGlobal('document', { visibilityState: 'hidden' })
    const api = await loadApi()
    await expect(api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it.each(queuedInvalidationCases)('fences queued $method replies across $boundary before invocation settles', async ({ method, boundary, invalidate }) => {
    const api = await loadApi()
    if (method !== 'authorizeProcessPane') await authorizeViewer(api)
    const operation = operations.find(entry => entry.method === method)!
    const queued = deferredReply(), transition = deferredReply()
    electronMock.invoke.mockImplementation((channel: string) => {
      if (channel === operation.channel) return queued.promise
      if (channel === boundary) return transition.promise
      return Promise.resolve({ status: 'ok' })
    })
    const pending = callProcess(api, method, operation.request)
    const changing = invalidate(api)
    queued.resolve(operation.result)
    await expect(pending).resolves.toEqual({ status: 'unavailable' })
    if (method === 'authorizeProcessPane') {
      expect(electronMock.invoke).toHaveBeenLastCalledWith('process-local:close', { viewerId })
    }
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
    transition.resolve(undefined)
    await changing
  })

  it.each(invocationInvalidations)('notifies protected leaves and retires the owner before $boundary invokes Main', async ({ boundary, invalidate }) => {
    const api = await loadApi()
    await authorizeViewer(api)
    const cleared = vi.fn(), changing = deferredReply()
    const unsubscribe = api.runtime.onProcessPaneInvalidated(cleared)
    electronMock.invoke.mockImplementation((channel: string) => {
      expect(channel).toBe(boundary)
      expect(cleared).toHaveBeenCalledTimes(1)
      expect(cleared.mock.calls[0]).toEqual([])
      return changing.promise
    })
    const pending = invalidate(api)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).toHaveBeenCalledTimes(2)
    expect(electronMock.invoke).toHaveBeenCalledWith('process-local:close', { viewerId })
    unsubscribe()
    changing.resolve(undefined)
    await pending
  })

  it('accepts only strict V2 control stamps and keeps its permanent fence after unsubscribe', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const cleared = vi.fn()
    const registered = electronMock.on.mock.calls.length
    const unsubscribe = api.runtime.onProcessPaneInvalidated(cleared)
    expect(electronMock.on.mock.calls).toHaveLength(registered)
    for (const value of [undefined, null, true, viewerId, {}, [], { viewerToken: privateMarker }, { body: privateMarker }]) {
      emitIpc('process-local:invalidated', value)
    }
    expect(cleared).not.toHaveBeenCalled()
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(snapshot)
    invalidateControlV2()
    expect(cleared.mock.calls).toEqual([[]])
    unsubscribe()
    await authorizeViewer(api)
    invalidateControlV2()
    expect(cleared.mock.calls).toEqual([[]])
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
    expect(electronMock.removeListener).not.toHaveBeenCalledWith('process-local:invalidated', expect.anything())
  })

  it('handles validated nonrunning runtime status without a renderer status subscription', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const cleared = vi.fn()
    api.runtime.onProcessPaneInvalidated(cleared)
    expect(electronMock.on.mock.calls.filter(([channel]) => channel === 'runtime:status')).toHaveLength(1)
    emitIpc('runtime:status', { state: 'starting', body: privateMarker })
    emitIpc('runtime:status', createRuntimeStatusPublicV1({ code: 'runtime_ready', source: 'restart' }))
    expect(cleared).not.toHaveBeenCalled()
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(snapshot)
    emitIpc('runtime:status', createRuntimeStatusPublicV1({ code: 'runtime_starting', source: 'restart' }))
    expect(cleared.mock.calls).toEqual([[]])
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('continues invalidation after a protected leaf throws and exposes no control payload or raw logs', async () => {
    const api = await loadApi()
    await authorizeViewer(api)
    const errorLog = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const warningLog = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const cleared = vi.fn()
    api.runtime.onProcessPaneInvalidated(() => { throw new Error(privateMarker) })
    api.runtime.onProcessPaneInvalidated(cleared)
    expect(() => invalidateControlV2()).not.toThrow()
    expect(cleared.mock.calls).toEqual([[]])
    expect(errorLog).not.toHaveBeenCalled()
    expect(warningLog).not.toHaveBeenCalled()
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it('keeps a successor active when an invalidated old authorization arrives and is closed', async () => {
    const api = await loadApi()
    const older = deferredReply(), secondId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    const successor = { ...authorized, viewerId: secondId }
    const cleared = vi.fn()
    api.runtime.onProcessPaneInvalidated(cleared)
    electronMock.invoke.mockReturnValueOnce(older.promise)
    const pending = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })
    invalidateControlV2()
    electronMock.invoke.mockResolvedValueOnce(successor)
    await expect(api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })).resolves.toEqual(successor)
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    older.resolve(authorized)
    await expect(pending).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).toHaveBeenLastCalledWith('process-local:close', { viewerId })
    expect(cleared.mock.calls).toEqual([[]])
    electronMock.invoke.mockResolvedValueOnce({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
    await expect(api.runtime.readProcessPane({ viewerId: secondId })).resolves.toEqual({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
  })
  it.each(['readProcessPane', 'openProcessCapture', 'setProcessPaneExpanded'] as const)('retires a current typed unavailable from %s and fences the other queued body', async method => {
    const api = await loadApi()
    await authorizeViewer(api)
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    const visible = await api.runtime.readProcessPane({ viewerId })
    expect(visible).toEqual(snapshot)
    let displayed: unknown = visible
    const cleared = vi.fn(() => { displayed = null })
    api.runtime.onProcessPaneInvalidated(cleared)
    const denied = deferredReply(), late = deferredReply()
    const operation = operations.find(entry => entry.method === method)!
    const otherMethod = method === 'openProcessCapture' ? 'readProcessPane' : 'openProcessCapture'
    const other = operations.find(entry => entry.method === otherMethod)!
    electronMock.invoke.mockImplementation((channel: string) => {
      if (channel === operation.channel) return denied.promise
      if (channel === other.channel) return late.promise
      return Promise.resolve({ status: 'ok' })
    })
    // For expand, first create its new epoch, then queue a body in that epoch.
    // Otherwise the old epoch fence would make this terminal test falsely pass.
    let failed: Promise<unknown>, queued: Promise<unknown>
    if (method === 'setProcessPaneExpanded') {
      failed = callProcess(api, method, operation.request)
      queued = callProcess(api, otherMethod, other.request)
    } else {
      queued = callProcess(api, otherMethod, other.request)
      failed = callProcess(api, method, operation.request)
    }
    denied.resolve({ status: 'unavailable' })
    await expect(failed).resolves.toEqual({ status: 'unavailable' })
    const notifiedBeforeLateBody = cleared.mock.calls.length
    late.resolve(other.result)
    const lateResult = await queued
    expect(notifiedBeforeLateBody).toBe(1)
    expect(displayed).toBeNull()
    expect(cleared.mock.calls).toEqual([[]])
    expect(lateResult).toEqual({ status: 'unavailable' })
    electronMock.invoke.mockClear()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual({ status: 'unavailable' })
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual({ status: 'unavailable' })
    expect(electronMock.invoke).not.toHaveBeenCalled()
  })

  it.each(['readProcessPane', 'openProcessCapture', 'setProcessPaneExpanded'] as const)('preserves the current owner and fresh body after busy from %s', async method => {
    const api = await loadApi()
    await authorizeViewer(api)
    const cleared = vi.fn()
    api.runtime.onProcessPaneInvalidated(cleared)
    const operation = operations.find(entry => entry.method === method)!
    electronMock.invoke.mockResolvedValueOnce({ status: 'busy' })
    await expect(callProcess(api, method, operation.request)).resolves.toEqual({ status: 'busy' })
    expect(cleared).not.toHaveBeenCalled()
    // Expand has advanced the epoch; these are fresh queries in that epoch.
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(snapshot)
    electronMock.invoke.mockResolvedValueOnce(capture)
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual(capture)
    expect(cleared).not.toHaveBeenCalled()
  })

  it.each(['readProcessPane', 'openProcessCapture', 'setProcessPaneExpanded'] as const)('does not retire successor B when old owner A returns unavailable from %s', async method => {
    const api = await loadApi()
    await authorizeViewer(api)
    const cleared = vi.fn(), older = deferredReply()
    api.runtime.onProcessPaneInvalidated(cleared)
    const operation = operations.find(entry => entry.method === method)!
    const secondId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    const successor = { ...authorized, viewerId: secondId }
    electronMock.invoke.mockImplementation((channel: string, payload?: { viewerId?: string }) => {
      if (channel === operation.channel && payload?.viewerId === viewerId) return older.promise
      if (channel === 'process-local:authorize') return Promise.resolve(successor)
      if (channel === 'process-local:read') return Promise.resolve(snapshot)
      if (channel === 'process-local:target') return Promise.resolve(capture)
      return Promise.resolve({ status: 'ok' })
    })
    const pending = callProcess(api, method, operation.request)
    await expect(api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: 'thread-a', turnId: 'turn-a' })).resolves.toEqual(successor)
    await expect(api.runtime.setProcessPaneExpanded({ viewerId: secondId, expandedBlockIds: ['block-a'] })).resolves.toEqual({ status: 'ok' })
    cleared.mockClear()
    older.resolve({ status: 'unavailable' })
    await expect(pending).resolves.toEqual({ status: 'unavailable' })
    expect(cleared).not.toHaveBeenCalled()
    await expect(api.runtime.readProcessPane({ viewerId: secondId })).resolves.toEqual(snapshot)
    await expect(api.runtime.openProcessCapture({ viewerId: secondId, blockId: 'block-a', captureDigest })).resolves.toEqual(capture)
    expect(cleared).not.toHaveBeenCalled()
  })

  it.each(['readProcessPane', 'setProcessPaneExpanded'] as const)('does not retire a newer epoch of the same owner after old %s returns unavailable', async method => {
    const api = await loadApi()
    await authorizeViewer(api)
    const cleared = vi.fn(), older = deferredReply()
    api.runtime.onProcessPaneInvalidated(cleared)
    const operation = operations.find(entry => entry.method === method)!
    electronMock.invoke.mockReturnValueOnce(older.promise)
    const pending = callProcess(api, method, operation.request)
    electronMock.invoke.mockImplementation((channel: string) => {
      if (channel === 'process-local:read') return Promise.resolve(snapshot)
      if (channel === 'process-local:target') return Promise.resolve(capture)
      return Promise.resolve({ status: 'ok' })
    })
    await expect(api.runtime.setProcessPaneExpanded({ viewerId, expandedBlockIds: ['block-a'] })).resolves.toEqual({ status: 'ok' })
    cleared.mockClear()
    older.resolve({ status: 'unavailable' })
    await expect(pending).resolves.toEqual({ status: 'unavailable' })
    expect(cleared).not.toHaveBeenCalled()
    await expect(api.runtime.readProcessPane({ viewerId })).resolves.toEqual(snapshot)
    await expect(api.runtime.openProcessCapture({ viewerId, blockId: 'block-a', captureDigest })).resolves.toEqual(capture)
    expect(cleared).not.toHaveBeenCalled()
  })
  it.each(['unavailable', 'busy'] as const)(
    'delivers initial %s before the retry continuation without invalidating an unleased pane', async status => {
      vi.stubGlobal('document', nativeDocumentFixtureV2())
      const api = await loadApi(), order: string[] = []
      let generation = 0
      api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, () => {
        generation += 1; order.push('invalidated')
      })
      const startedGeneration = generation
      electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { status } })
      const result = await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId }).then(reply => {
        order.push(reply.status)
        if (generation === startedGeneration) order.push('retry')
        return reply
      })
      expect(result).toEqual({ status })
      expect(order).toEqual([status, 'retry'])
      expect(electronMock.invoke).not.toHaveBeenCalledWith('process-local:close', expect.anything())
    })

  it('reaches authorization after the three bounded retry continuations on ordinary producer unavailability', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(), order: string[] = []
    let generation = 0
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, () => { generation += 1 })
    const statuses = ['unavailable', 'busy', 'unavailable'] as const
    electronMock.invoke.mockImplementation(() => Promise.resolve({ control: controlFixtureV2(),
      result: order.length < statuses.length ? { status: statuses[order.length] } : authorized }))
    for (let attempt = 0; attempt <= statuses.length; attempt++) {
      const startedGeneration = generation
      const result = await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })
      if (generation !== startedGeneration) break
      order.push(result.status)
      if (result.status === 'authorized') break
    }
    expect(order).toEqual([...statuses, 'authorized'])
    expect(electronMock.invoke).toHaveBeenCalledTimes(4)
  })

  it.each(['unavailable', 'busy', 'revoked', 'malformed'] as const)(
    'preserves a same-root pending successor without callbacks when the old authorization returns %s', async status => {
      vi.stubGlobal('document', nativeDocumentFixtureV2())
      const api = await loadApi(), clear = vi.fn(), old = deferredReply()
      api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, clear)
      electronMock.invoke.mockReturnValueOnce(old.promise)
      const pending = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })
      electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: authorized })
      expect(await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })).toEqual(authorized)
      expect(clear).not.toHaveBeenCalled()
      old.resolve({ control: status === 'malformed' ? { schemaVersion: 1 } : controlFixtureV2(),
        result: { status: status === 'malformed' ? 'unavailable' : status } })
      expect(await pending).toEqual({ status: 'unavailable' })
      expect(clear).not.toHaveBeenCalled()
      const unchanged = { status: 'unchanged', schemaVersion: 2, revision: '2', scope }
      electronMock.invoke.mockResolvedValueOnce(unchanged)
      expect(await api.runtime.readProcessPane({ viewerId })).toEqual(unchanged)
      expect(clear).not.toHaveBeenCalled()
    })

  it('cancels an unleased recovery on actual control advancement after the early unavailable Promise', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(), order: string[] = []
    let generation = 0
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, () => { generation += 1; order.push('invalidated') })
    const startingGeneration = generation
    electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { status: 'unavailable' } })
    const result = await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })
    order.push(result.status)
    expect(order).toEqual(['unavailable'])
    emitIpc('process-local:invalidated', controlFixtureV2())
    expect(generation).toBe(startingGeneration)
    invalidateControlV2()
    if (generation === startingGeneration) {
      order.push('retry')
      electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: authorized })
      await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })
    }
    expect(order).toEqual(['unavailable', 'invalidated'])
    expect(electronMock.invoke).toHaveBeenCalledTimes(1)
    emitIpc('process-local:invalidated', controlFixtureV2())
    emitIpc('process-local:invalidated', { ...controlFixtureV2(), authorityEpoch: '0' })
    expect(order).toEqual(['unavailable', 'invalidated'])
  })

  it('notifies unowned recovery subscriptions once while preserving the fresh authorized pane at the advancing fence', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(), oldClear = vi.fn(), newClear = vi.fn(), legacy = vi.fn()
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, oldClear)
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-new' }, newClear)
    api.runtime.onProcessPaneInvalidated(legacy)
    electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { status: 'busy' } })
    expect(await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })).toEqual({ status: 'busy' })
    expect(oldClear).not.toHaveBeenCalled()
    const control = { schemaVersion: 2, documentEpoch: '2', authorityEpoch: '1' }
    electronMock.invoke.mockResolvedValueOnce({ control, result: authorized })
    expect(await api.runtime.authorizeProcessPane({ paneId: 'pane-new', threadId: scope.threadId, turnId: scope.turnId })).toEqual(authorized)
    expect(oldClear.mock.calls).toEqual([[]])
    expect(newClear).not.toHaveBeenCalled()
    expect(legacy.mock.calls).toEqual([[]])
    emitIpc('process-local:invalidated', control)
    emitIpc('process-local:invalidated', { schemaVersion: 2, documentEpoch: '1', authorityEpoch: '99' })
    expect(oldClear).toHaveBeenCalledOnce()
    expect(newClear).not.toHaveBeenCalled()
    const unchanged = { status: 'unchanged', schemaVersion: 2, revision: '2', scope }
    electronMock.invoke.mockResolvedValueOnce(unchanged)
    expect(await api.runtime.readProcessPane({ viewerId })).toEqual(unchanged)
  })

  it.each(['runtime-status', 'document-hidden'] as const)(
    'cancels unleased recovery after ordinary unavailable on actual %s retirement', async reason => {
      const doc = nativeDocumentFixtureV2(); vi.stubGlobal('document', doc)
      const api = await loadApi(), order: string[] = []
      api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, () => order.push('invalidated'))
      electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { status: 'unavailable' } })
      const result = await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })
      order.push(result.status)
      expect(order).toEqual(['unavailable'])
      if (reason === 'runtime-status') emitIpc('runtime:status', createRuntimeStatusPublicV1({ code: 'runtime_starting', source: 'restart' }))
      else {
        Object.defineProperty(doc, 'visibilityState', { value: 'hidden', configurable: true })
        const event = doc.createEvent('Event'); event.initEvent('visibilitychange', false, false); doc.dispatchEvent(event)
      }
      expect(order).toEqual(['unavailable', 'invalidated'])
      expect(electronMock.invoke).toHaveBeenCalledTimes(1)
    })

  it('retires a control-less older pending pane on a newer document fence even without authority epoch change', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(), old = deferredReply(), oldClear = vi.fn(), newClear = vi.fn()
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-old' }, oldClear)
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-new' }, newClear)
    electronMock.invoke.mockReturnValueOnce(old.promise)
    const pending = api.runtime.authorizeProcessPane({ paneId: 'pane-old', threadId: scope.threadId, turnId: scope.turnId })
    electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { status: 'unavailable' } })
    expect(await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })).toEqual({ status: 'unavailable' })
    expect(oldClear).not.toHaveBeenCalled()
    const control = { schemaVersion: 2, documentEpoch: '2', authorityEpoch: '0' }
    electronMock.invoke.mockResolvedValueOnce({ control, result: authorized })
    expect(await api.runtime.authorizeProcessPane({ paneId: 'pane-new', threadId: scope.threadId, turnId: scope.turnId })).toEqual(authorized)
    expect(oldClear.mock.calls).toEqual([[]])
    expect(newClear).not.toHaveBeenCalled()
    const oldViewerId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    old.resolve({ control: controlFixtureV2(), result: { ...authorized, viewerId: oldViewerId } })
    expect(await pending).toEqual({ status: 'unavailable' })
    expect(oldClear).toHaveBeenCalledOnce()
    expect(electronMock.invoke).toHaveBeenCalledWith('process-local:close', { viewerId: oldViewerId })
    electronMock.invoke.mockResolvedValueOnce({ status: 'unchanged', schemaVersion: 2, revision: '2', scope })
    expect((await api.runtime.readProcessPane({ viewerId })).status).toBe('unchanged')
    expect(newClear).not.toHaveBeenCalled()
  })

  it.each(['unavailable', 'busy'] as const)(
    'releases all ordinary pending %s reservations without consuming global pane quota', async status => {
      const doc = nativeDocumentFixtureV2(); vi.stubGlobal('document', doc)
      const api = await loadApi(), clear = vi.fn()
      api.runtime.onProcessPaneInvalidated(clear)
      for (let index = 0; index < 9; index++) {
        const root = doc.createElement('div'), paneId = 'pending-retry-' + index
        root.setAttribute('data-process-pane-id', paneId); doc.body.appendChild(root)
        electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { status } })
        expect(await api.runtime.authorizeProcessPane({ paneId, threadId: scope.threadId, turnId: scope.turnId })).toEqual({ status })
      }
      expect(clear).not.toHaveBeenCalled()
      electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: authorized })
      expect(await api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId })).toEqual(authorized)
      expect(electronMock.invoke).toHaveBeenCalledTimes(10)
    })

  it.each(['unavailable', 'busy'] as const)(
    'notifies real authority loss from a failed pending %s and retires older active siblings', async status => {
      vi.stubGlobal('document', nativeDocumentFixtureV2())
      const api = await loadApi(); await authorizeViewer(api)
      const oldClear = vi.fn(), order: string[] = []
      api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, oldClear)
      api.runtime.onProcessPaneInvalidated({ paneId: 'pane-new' }, () => order.push('invalidated'))
      electronMock.invoke.mockResolvedValueOnce({ control: { ...controlFixtureV2(), authorityEpoch: '1' }, result: { status } })
      const result = await api.runtime.authorizeProcessPane({ paneId: 'pane-new', threadId: scope.threadId, turnId: scope.turnId }).then(reply => {
        order.push(reply.status); return reply
      })
      expect(result).toEqual({ status: 'unavailable' })
      expect(order).toEqual(['invalidated', 'unavailable'])
      expect(oldClear.mock.calls).toEqual([[]])
      expect(await api.runtime.readProcessPane({ viewerId })).toEqual({ status: 'unavailable' })
    })

  it('notifies a failed newer document without adopting its fence from an unavailable reply', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(); await authorizeViewer(api)
    const oldClear = vi.fn(), order: string[] = []
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, oldClear)
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-new' }, () => order.push('invalidated'))
    electronMock.invoke.mockResolvedValueOnce({ control: { schemaVersion: 2, documentEpoch: '2', authorityEpoch: '1' }, result: { status: 'busy' } })
    const result = await api.runtime.authorizeProcessPane({ paneId: 'pane-new', threadId: scope.threadId, turnId: scope.turnId }).then(reply => {
      order.push(reply.status); return reply
    })
    expect(result).toEqual({ status: 'unavailable' })
    expect(order).toEqual(['invalidated', 'unavailable'])
    expect(oldClear).not.toHaveBeenCalled()
    invalidateControlV2()
    expect(oldClear.mock.calls).toEqual([[]])
  })

  it.each(['revoked', 'hidden', 'replacement', 'navigation'] as const)(
    'keeps real pending %s invalidation ahead of the failure continuation', async reason => {
      const doc = nativeDocumentFixtureV2(); vi.stubGlobal('document', doc)
      const api = await loadApi(), order: string[] = [], queued = deferredReply()
      api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, () => order.push('invalidated'))
      electronMock.invoke.mockReturnValueOnce(queued.promise)
      const pending = api.runtime.authorizeProcessPane({ paneId: 'pane-a', threadId: scope.threadId, turnId: scope.turnId }).then(reply => {
        order.push(reply.status); return reply
      })
      const root = doc.querySelector('[data-process-pane-id="pane-a"]')!
      if (reason === 'hidden') root.parentElement!.setAttribute('hidden', '')
      if (reason === 'replacement') root.replaceWith(root.cloneNode())
      if (reason === 'navigation') invalidateControlV2()
      queued.resolve({ control: controlFixtureV2(), result: { status: reason === 'revoked' ? 'revoked' : 'busy' } })
      const expected = reason === 'revoked' ? 'revoked' : 'unavailable'
      expect(await pending).toEqual({ status: expected })
      expect(order).toEqual(['invalidated', expected])
    })

  it('keeps three mounted pane leases independent and scopes local invalidation callbacks', async () => {
    const doc = nativeDocumentFixtureV2()
    vi.stubGlobal('document', doc)
    const api = await loadApi()
    const ids = [viewerId, '10341b49-26c7-48f8-abcb-7b6ae6fa5c03', '20341b49-26c7-48f8-abcb-7b6ae6fa5c03']
    const panes = ['pane-a', 'pane-old', 'pane-new']
    const callbacks = panes.map(paneId => {
      const callback = vi.fn(); api.runtime.onProcessPaneInvalidated({ paneId }, callback); return callback
    })
    const legacy = vi.fn(); api.runtime.onProcessPaneInvalidated(legacy)
    for (let i = 0; i < panes.length; i++) {
      electronMock.invoke.mockResolvedValueOnce({ ...authorized, viewerId: ids[i] })
      expect((await api.runtime.authorizeProcessPane({ paneId: panes[i], threadId: scope.threadId, turnId: scope.turnId })).status).toBe('authorized')
      electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
      await api.runtime.setProcessPaneExpanded({ viewerId: ids[i], expandedBlockIds: ['block-a'] })
    }
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    expect(await api.runtime.closeProcessPane({ viewerId: ids[1] })).toEqual({ status: 'ok' })
    expect(callbacks.map(callback => callback.mock.calls.length)).toEqual([0, 1, 0])
    expect(legacy.mock.calls).toEqual([[]])
    for (const id of [ids[0], ids[2]]) {
      electronMock.invoke.mockResolvedValueOnce(snapshot)
      expect(await api.runtime.readProcessPane({ viewerId: id })).toEqual(snapshot)
    }
    const calls = electronMock.invoke.mock.calls.length
    expect(await api.runtime.readProcessPane({ viewerId: ids[1] })).toEqual({ status: 'unavailable' })
    expect(electronMock.invoke.mock.calls).toHaveLength(calls)
  })

  it('adopts a fresh authorization fence before an equal delayed control and only clears older panes', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi()
    await authorizeViewer(api)
    const oldBody = deferredReply(), oldClear = vi.fn(), newClear = vi.fn()
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, oldClear)
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-new' }, newClear)
    electronMock.invoke.mockReturnValueOnce(oldBody.promise)
    const queued = api.runtime.readProcessPane({ viewerId })
    const newId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    controlAuthority = 1
    electronMock.invoke.mockResolvedValueOnce({ control: controlFixtureV2(), result: { ...authorized, viewerId: newId } })
    expect((await api.runtime.authorizeProcessPane({ paneId: 'pane-new', threadId: scope.threadId, turnId: scope.turnId })).status).toBe('authorized')
    expect(oldClear.mock.calls).toEqual([[]])
    emitIpc('process-local:invalidated', controlFixtureV2())
    emitIpc('process-local:invalidated', { ...controlFixtureV2(), authorityEpoch: '0' })
    emitIpc('process-local:invalidated', { ...controlFixtureV2(), documentEpoch: '2', authorityEpoch: '9' })
    oldBody.resolve(snapshot)
    expect(await queued).toEqual({ status: 'unavailable' })
    expect(newClear).not.toHaveBeenCalled()
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await api.runtime.setProcessPaneExpanded({ viewerId: newId, expandedBlockIds: ['block-a'] })
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    expect(await api.runtime.readProcessPane({ viewerId: newId })).toEqual(snapshot)
    expect(oldClear).toHaveBeenCalledOnce()
  })

  it.each(['duplicate', 'replacement', 'hidden-ancestor', 'inert-ancestor', 'aria-hidden-ancestor', 'style-hidden'] as const)(
    'rejects queued body delivery after actual DOM %s and clears only the affected leaf', async reason => {
      const doc = nativeDocumentFixtureV2(); vi.stubGlobal('document', doc)
      const api = await loadApi(); await authorizeViewer(api)
      const cleared = vi.fn(); api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, cleared)
      const queued = deferredReply(); electronMock.invoke.mockReturnValueOnce(queued.promise)
      const read = api.runtime.readProcessPane({ viewerId })
      const root = doc.querySelector('[data-process-pane-id="pane-a"]')!
      if (reason === 'duplicate') root.parentElement!.appendChild(root.cloneNode())
      if (reason === 'replacement') root.replaceWith(root.cloneNode())
      if (reason === 'hidden-ancestor') root.parentElement!.setAttribute('hidden', '')
      if (reason === 'inert-ancestor') root.parentElement!.setAttribute('inert', '')
      if (reason === 'aria-hidden-ancestor') root.parentElement!.setAttribute('aria-hidden', 'true')
      if (reason === 'style-hidden') root.parentElement!.setAttribute('style', 'display: none')
      queued.resolve(snapshot)
      expect(await read).toEqual({ status: 'unavailable' })
      expect(cleared.mock.calls).toEqual([[]])
      expect(electronMock.invoke).toHaveBeenCalledWith('process-local:close', { viewerId })
    })

  it('rejects a same-name scope generation and preserves no stale protected body', async () => {
    const api = await loadApi(); await authorizeViewer(api)
    const cleared = vi.fn(); api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, cleared)
    electronMock.invoke.mockResolvedValueOnce({ ...snapshot, scope: { ...scope, contextEpoch: '2' } })
    expect(await api.runtime.readProcessPane({ viewerId })).toEqual({ status: 'unavailable' })
    expect(cleared.mock.calls).toEqual([[]])
  })

  it('accepts a newer native document only from fresh exact-root authorization and ignores old document controls', async () => {
    vi.stubGlobal('document', nativeDocumentFixtureV2())
    const api = await loadApi(); await authorizeViewer(api)
    const oldClear = vi.fn(), newClear = vi.fn()
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, oldClear)
    api.runtime.onProcessPaneInvalidated({ paneId: 'pane-new' }, newClear)
    const newId = '10341b49-26c7-48f8-abcb-7b6ae6fa5c03'
    electronMock.invoke.mockResolvedValueOnce({ control: { schemaVersion: 2, documentEpoch: '2', authorityEpoch: '1' }, result: { ...authorized, viewerId: newId } })
    expect((await api.runtime.authorizeProcessPane({ paneId: 'pane-new', threadId: scope.threadId, turnId: scope.turnId })).status).toBe('authorized')
    expect(oldClear.mock.calls).toEqual([[]])
    emitIpc('process-local:invalidated', { schemaVersion: 2, documentEpoch: '1', authorityEpoch: '99' })
    electronMock.invoke.mockResolvedValueOnce({ status: 'ok' })
    await api.runtime.setProcessPaneExpanded({ viewerId: newId, expandedBlockIds: ['block-a'] })
    electronMock.invoke.mockResolvedValueOnce(snapshot)
    expect(await api.runtime.readProcessPane({ viewerId: newId })).toEqual(snapshot)
    expect(newClear).not.toHaveBeenCalled()
  })

  it('releases rejected in-flight authorization reservations after roots unmount without consuming pane quota', async () => {
    const doc = nativeDocumentFixtureV2(); vi.stubGlobal('document', doc)
    const api = await loadApi()
    for (let index = 0; index < 9; index++) {
      const root = doc.createElement('div'), paneId = 'transient-' + index
      root.setAttribute('data-process-pane-id', paneId); doc.body.appendChild(root)
      const pending = deferredReply(); electronMock.invoke.mockReturnValueOnce(pending.promise)
      const authorization = api.runtime.authorizeProcessPane({ paneId, threadId: scope.threadId, turnId: scope.turnId })
      root.remove(); pending.resolve({ control: undefined, result: { status: 'unavailable' } })
      expect(await authorization).toEqual({ status: 'unavailable' })
    }
    await authorizeViewer(api)
  })

  it.each(['readProcessPane', 'openProcessCapture', 'setProcessPaneExpanded'] as const)(
    'retires the exact owner when %s rejects after its root hides', async method => {
      const doc = nativeDocumentFixtureV2(); vi.stubGlobal('document', doc)
      const api = await loadApi(); await authorizeViewer(api)
      const clear = vi.fn(); api.runtime.onProcessPaneInvalidated({ paneId: 'pane-a' }, clear)
      let reject!: (error: Error) => void
      const rejected = new Promise<unknown>((_resolve, fail) => { reject = fail })
      electronMock.invoke.mockReturnValueOnce(rejected)
      const operation = operations.find(value => value.method === method)!
      const queued = callProcess(api, method, operation.request)
      const root = doc.querySelector('[data-process-pane-id="pane-a"]')!
      root.setAttribute('hidden', ''); reject(new Error(privateMarker))
      expect(await queued).toEqual({ status: 'unavailable' })
      expect(clear.mock.calls).toEqual([[]])
      root.removeAttribute('hidden'); electronMock.invoke.mockClear()
      expect(await api.runtime.readProcessPane({ viewerId })).toEqual({ status: 'unavailable' })
      expect(electronMock.invoke).not.toHaveBeenCalled()
    })

})
