import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  AcceptedSlotDisplayRequest,
  DirectSourcePreviewRequest,
  LocalDisplayResult,
  SseEventPayload
} from '@shared/analytix-api'
import { installBrowserAnalytixBridge } from './browser-analytix-bridge'

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key: string) {
      return values.get(key) ?? null
    },
    key(index: number) {
      return [...values.keys()][index] ?? null
    },
    removeItem(key: string) {
      values.delete(key)
    },
    setItem(key: string, value: string) {
      values.set(key, value)
    }
  }
}

function stubBrowserWindow(overrides: Partial<Window> = {}): void {
  const storage = createMemoryStorage()
  vi.stubGlobal('window', {
    localStorage: storage,
    location: {
      protocol: 'http:',
      hostname: 'localhost'
    },
    confirm: vi.fn(() => true),
    open: vi.fn(),
    setTimeout,
    clearTimeout,
    ...overrides
  })
  vi.stubGlobal('document', {
    documentElement: {
      dataset: {}
    }
  })
  vi.stubGlobal('navigator', {
    platform: 'MacIntel'
  })
}

async function waitFor(predicate: () => boolean, timeoutMs = 500): Promise<void> {
  const startedAt = Date.now()
  while (!predicate()) {
    if (Date.now() - startedAt > timeoutMs) {
      throw new Error('Timed out waiting for browser bridge condition')
    }
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
}

function runtimeEvent(seq: number, threadId: string): Record<string, unknown> {
  return {
    seq,
    kind: 'tool_progress',
    timestamp: `2026-07-14T00:00:${String(seq).padStart(2, '0')}.000Z`,
    threadId,
    turnId: `turn-${threadId}`,
    toolName: 'test_tool',
    callId: `call-${seq}`,
    status: 'running'
  }
}

function eventFrame(seq: number, threadId: string): string {
  return `id: ${seq}\nevent: tool_progress\ndata: ${JSON.stringify(runtimeEvent(seq, threadId))}\n\n`
}

function publicProjectionRevokedFrame(threadId: string): string {
  return `event: public_projection_revoked\ndata: ${JSON.stringify({
    schemaVersion: 1,
    kind: 'public_projection_revoked',
    threadId,
    historyAuthority: 'case_boundary_only_v1',
    code: 'case_public_authority_rejected',
    action: 'purge_case_projection',
    terminal: true
  })}\n\n`
}

function payloadFrame(seq: number, event: string, payload: Record<string, unknown>): string {
  return `id: ${seq}\nevent: ${event}\ndata: ${JSON.stringify({
    seq,
    kind: event,
    timestamp: '2026-07-14T00:00:00.000Z',
    ...payload
  })}\n\n`
}

function acceptedFinalPair() {
  const acceptedFinal = {
    schemaVersion: 3,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: 'a'.repeat(64),
    authorityPublicKey: 'A'.repeat(43),
    threadId: 'thread-case',
    turnId: 'turn-case',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-case-3',
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    renderedTextSha256: 'd'.repeat(64),
    registrySequence: 0,
    registryStateDigest: 'e'.repeat(64),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v2',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    privateRecordDigest: 'f'.repeat(64),
    acceptedAt: '2026-07-11T01:02:03.000Z',
    authoritySignature: 'A'.repeat(86),
    recordDigest: '9'.repeat(64)
  }
  const acceptedFinalView = {
    schemaVersion: 1,
    publicationState: 'accepted',
    acceptedFinalDigest: acceptedFinal.recordDigest,
    envelopeDigest: acceptedFinal.envelopeDigest,
    contextDigest: acceptedFinal.contextDigest,
    contextEpoch: acceptedFinal.contextEpoch,
    datasetSnapshotId: acceptedFinal.datasetSnapshotId,
    variant: acceptedFinal.variant,
    terminalReason: acceptedFinal.terminalReason,
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: '8'.repeat(64),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: acceptedFinal.acceptedAt,
    acceptedAt: acceptedFinal.acceptedAt
  }
  return { acceptedFinal, acceptedFinalView }
}

function controlledStream(initialFrames: string[] = []): {
  stream: ReadableStream<Uint8Array>
  enqueue: (frame: string) => void
  close: () => void
} {
  const encoder = new TextEncoder()
  let controller: ReadableStreamDefaultController<Uint8Array> | null = null
  const stream = new ReadableStream<Uint8Array>({
    start(nextController) {
      controller = nextController
      for (const frame of initialFrames) {
        nextController.enqueue(encoder.encode(frame))
      }
    }
  })
  return {
    stream,
    enqueue(frame: string) {
      controller?.enqueue(encoder.encode(frame))
    },
    close() {
      controller?.close()
    }
  }
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('browser analytix bridge', () => {
  it('installs a development browser bridge before the renderer boots', async () => {
    const fetch = vi.fn(async () => new Response('{"ok":true}', { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)

    const installed = installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    expect(installed).toBe(true)
    expect(window.analytix.app.platform).toBe('darwin')
    expect('probeModelProvider' in window.analytix.runtime).toBe(false)
    expect(document.documentElement.dataset.bridge).toBe('browser-preview')

    const settings = await window.analytix.settings.getSettings()
    expect(settings.locale).toBe('zh')
    expect(settings.runtime.autoStart).toBe(false)

    await window.analytix.runtime.runtimeRequest('/health', 'GET')
    expect(fetch).toHaveBeenCalledWith('/runtime-proxy/health', expect.objectContaining({
      method: 'GET'
    }))
  })

  it('fails Provider Registry closed without fetch storage logging or secret echo', async () => {
    const fetch = vi.fn()
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const log = vi.spyOn(console, 'log').mockImplementation(() => undefined)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    stubBrowserWindow()
    const getItem = vi.spyOn(window.localStorage, 'getItem')
    const setItem = vi.spyOn(window.localStorage, 'setItem')
    const removeItem = vi.spyOn(window.localStorage, 'removeItem')
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    getItem.mockClear()
    setItem.mockClear()
    removeItem.mockClear()

    const syntheticSecretMarker = 'synthetic-browser-provider-secret'
    const result = await window.analytix.providerRegistry.request({
      schemaVersion: 1,
      operation: 'connect',
      expected: {
        registryRevision: '0',
        registryIncarnation: `inc_${'a'.repeat(43)}`,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      provider: {
        id: 'provider-alpha',
        kind: 'openai-compatible',
        endpoint: 'https://provider.invalid/v1',
        proxy: '',
        models: ['model-alpha'],
        mediaModels: [],
        selectedModel: 'model-alpha',
        selectedMediaModel: '',
        selectedRoutes: ['primary']
      },
      credential: {
        kind: 'set',
        purpose: 'provider-api-key',
        valueBase64: 'c3ludGhldGljLWJyb3dzZXItcHJvdmlkZXItc2VjcmV0'
      }
    })

    expect(result).toEqual({
      schemaVersion: 1,
      error: { code: 'runtime_unavailable', message: 'The provider registry is unavailable.' }
    })
    expect(JSON.stringify(result)).not.toContain(syntheticSecretMarker)
    expect(fetch).not.toHaveBeenCalled()
    expect(getItem).not.toHaveBeenCalled()
    expect(setItem).not.toHaveBeenCalled()
    expect(removeItem).not.toHaveBeenCalled()
    expect(error).not.toHaveBeenCalled()
    expect(log).not.toHaveBeenCalled()
    expect(warn).not.toHaveBeenCalled()
  })

  it('fails protected recovery closed without browser artifact or storage access', async () => {
    const fetch = vi.fn()
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    const getItem = vi.spyOn(window.localStorage, 'getItem')
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    getItem.mockClear()

    await expect(window.analytix.providerCredentialRecovery.createDestinationRequest())
      .resolves.toEqual({ ok: false, code: 'runtime_unavailable' })
    await expect(window.analytix.providerCredentialRecovery.createSourceBundle())
      .resolves.toEqual({ ok: false, code: 'runtime_unavailable' })
    expect(fetch).not.toHaveBeenCalled()
    expect(getItem).not.toHaveBeenCalled()
  })

  it('projects both browser-preview log APIs to one fixed renderer diagnostic', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const canaries = [
      'BROWSER_PREVIEW_LOG_CANARY_7F3C',
      '/private/case/source.csv',
      `cer1_${'b'.repeat(64)}`,
      '6222020202020202020'
    ]
    stubBrowserWindow()
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    await window.analytix.logs.error(canaries[0], canaries[1], {
      authorityRef: canaries[2],
      account: canaries[3]
    })
    await window.analytix.diagnostics.logError(canaries[1], canaries[0], {
      path: canaries[1],
      authorityRef: canaries[2]
    })

    expect(error).toHaveBeenCalledTimes(2)
    expect(error).toHaveBeenNthCalledWith(1, '[analytix] [renderer] event=renderer_diagnostic_failed')
    expect(error).toHaveBeenNthCalledWith(2, '[analytix] [renderer] event=renderer_diagnostic_failed')
    const output = JSON.stringify(error.mock.calls)
    for (const canary of canaries) expect(output).not.toContain(canary)
  })

  it('keeps typed local display unavailable outside the desktop host without a network request', async () => {
    const directRequest: DirectSourcePreviewRequest = {
      kind: 'direct_source_preview',
      view: 'transactions',
      fields: ['account', 'amountText'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full'
    }
    const acceptedRequest: AcceptedSlotDisplayRequest = {
      kind: 'accepted_slot_display',
      threadId: 'thread-local-display',
      turnId: 'turn-local-display',
      acceptedFinalDigest: 'b'.repeat(64),
      displayMode: 'masked'
    }
    const fetch = vi.fn()
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    await expect(window.analytix.runtime.directSourcePreview(directRequest)).resolves.toMatchObject({
      ok: false,
      status: 403,
      code: 'forbidden'
    } satisfies Partial<LocalDisplayResult>)
    await expect(window.analytix.runtime.acceptedSlotDisplay(acceptedRequest)).resolves.toMatchObject({
      ok: false,
      status: 403,
      code: 'forbidden'
    } satisfies Partial<LocalDisplayResult>)
    await expect(window.analytix.runtime.importMappingPreview({
      kind: 'import_mapping_preview',
      selector: `tlsel1_${'a'.repeat(64)}`,
      fields: ['sourceColumn'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'full'
    })).resolves.toMatchObject({ ok: false, status: 403, code: 'forbidden' })
    await expect(window.analytix.runtime.cleaningDiffPreview({
      kind: 'cleaning_diff_preview',
      selector: `tlsel1_${'b'.repeat(64)}`,
      fields: ['account'],
      rowOffset: 0,
      rowLimit: 25,
      displayMode: 'masked'
    })).resolves.toMatchObject({ ok: false, status: 403, code: 'forbidden' })
    await expect(window.analytix.runtime.stageFundsCSVSnapshot()).resolves.toEqual({
      ok: false,
      canceled: false,
      code: 'forbidden',
      message: 'Funds CSV snapshot staging is available only through the desktop host.'
    })
    const selector = `tlsel1_${'c'.repeat(64)}`
    await expect(window.analytix.runtime.confirmFundsCSVSnapshot(selector)).resolves.toMatchObject({
      ok: false, code: 'forbidden'
    })
    await expect(window.analytix.runtime.cancelFundsCSVImport(selector)).resolves.toMatchObject({
      ok: false, code: 'forbidden'
    })
    await expect(window.analytix.runtime.statusFundsCSVImport(selector)).resolves.toMatchObject({
      ok: false, code: 'forbidden'
    })
    await expect(window.analytix.runtime.runDeterministicFundsCleaning()).resolves.toMatchObject({
      ok: false, status: 'unavailable', code: 'runtime_unavailable'
    })
    await expect(window.analytix.runtime.revokeCleaningDiffPreview(selector)).resolves.toMatchObject({
      ok: false, code: 'runtime_unavailable'
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('rejects old or malformed typed diagnostics at the browser public boundary', async () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        host: '127.0.0.1',
        dataDir: privateSentinel,
        providers: [{ apiKey: privateSentinel }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(privateSentinel, { status: 200 }))
      .mockResolvedValueOnce(new Response('', { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    const oldInfo = await window.analytix.runtime.runtimeRequest('/v1/runtime/info', 'GET')
    const malformedTools = await window.analytix.runtime.runtimeRequest('/v1/runtime/tools', 'GET')
    const emptyMemory = await window.analytix.runtime.runtimeRequest('/v1/memory/diagnostics', 'GET')

    for (const response of [oldInfo, malformedTools, emptyMemory]) {
      expect(response).toEqual({
        ok: false,
        status: 502,
        body: expect.any(String)
      })
      expect(JSON.stringify(response)).not.toContain(privateSentinel)
    }
    expect(JSON.parse(oldInfo.body)).toEqual({
      code: 'runtime_response_schema_invalid',
      message: 'Runtime response failed schema validation.'
    })
    expect(JSON.parse(malformedTools.body)).toEqual({
      code: 'runtime_response_not_public',
      message: 'Runtime response was blocked at the public boundary.'
    })
  })

  it('uses the strict shared case-project schemas at the browser boundary', async () => {
    const project = {
      id: 'case_0123456789abcdef01234567',
      name: 'case-a',
      rootPath: '/cases/a',
      updatedAt: '2026-07-22T00:00:00Z',
      threadCount: 0,
      runningCount: 0,
      archivedCount: 0,
      lastThreadId: '',
      lastPreview: '',
      dataSizeEstimate: 0,
      status: 'ready'
    }
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        caseProjects: [project],
        indexStatus: 'ready'
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ threads: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ project, threads: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        caseProjects: [{ ...project, privateField: 'must-not-pass' }],
        indexStatus: 'ready'
      }), { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    const list = await window.analytix.runtime.runtimeRequest('/v1/case-projects', 'GET')
    const threads = await window.analytix.runtime.runtimeRequest(
      '/v1/case-projects/case_0123456789abcdef01234567/threads',
      'GET'
    )
    const detail = await window.analytix.runtime.runtimeRequest(
      '/v1/case-projects/case_0123456789abcdef01234567/detail',
      'GET'
    )
    expect(JSON.parse(list.body)).toEqual({ caseProjects: [project], indexStatus: 'ready' })
    expect(JSON.parse(threads.body)).toEqual({ threads: [] })
    expect(JSON.parse(detail.body)).toEqual({ project, threads: [] })

    const rejected = await window.analytix.runtime.runtimeRequest('/v1/case-projects', 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain('must-not-pass')
  })

  it('rejects marker-free arbitrary output from both public task-output routes', async () => {
    const privateOutputSentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    const taskOutput = {
      schemaVersion: 1,
      availability: 'available',
      jobId: 'job-private-output',
      status: 'completed',
      output: privateOutputSentinel,
      offset: 0,
      nextOffset: privateOutputSentinel.length,
      outputBytes: privateOutputSentinel.length,
      truncated: false
    }
    const summaryOutput = {
      schemaVersion: 1,
      availability: 'available',
      taskId: 'taskjob:job-private-output',
      status: 'completed',
      output: privateOutputSentinel,
      offset: 0,
      nextOffset: privateOutputSentinel.length,
      outputBytes: privateOutputSentinel.length,
      truncated: false
    }
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(taskOutput), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(summaryOutput), { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    const task = await window.analytix.runtime.runtimeRequest('/v1/runtime/task-jobs/output', 'GET')
    const summary = await window.analytix.runtime.runtimeRequest(
      '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-private-output/output',
      'GET'
    )
    for (const response of [task, summary]) {
      expect(response.ok).toBe(false)
      expect(response.status).toBe(502)
      expect(response.body).not.toContain(privateOutputSentinel)
    }
  })

  it('strictly validates and route-binds browser summary and mutation responses', async () => {
    const privateSentinel = '<think>BROWSER_PRIVATE_SUMMARY_7F3C</think>'
    const task = {
      schemaVersion: 1,
      id: 'taskjob:job-1',
      kind: 'task',
      status: 'running',
      background: false,
      terminal: false,
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false
    }
    const summary = {
      threadId: 'thread-1',
      generatedAt: '2026-07-20T00:00:00Z',
      latestSeq: 1,
      subagents: [],
      tasks: [task],
      outputs: [],
      sources: [],
      sideChats: [],
      backgroundProcesses: []
    }
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(summary), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...summary, threadId: 'thread-2' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        ...summary,
        sideChats: [{
          threadId: 'side-1', title: privateSentinel, status: 'idle', relation: 'side',
          parentThreadId: 'thread-1', createdAt: '2026-07-20T00:00:00Z', updatedAt: '2026-07-20T00:00:01Z'
        }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ task }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ task: { ...task, id: 'taskjob:job-2' } }), { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    const accepted = await window.analytix.runtime.runtimeRequest('/v1/threads/thread-1/summary', 'GET')
    expect(accepted.ok).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(summary)

    const foreign = await window.analytix.runtime.runtimeRequest('/v1/threads/thread-1/summary', 'GET')
    const poisoned = await window.analytix.runtime.runtimeRequest('/v1/threads/thread-1/summary', 'GET')
    const mutation = await window.analytix.runtime.runtimeRequest(
      '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/kill',
      'POST'
    )
    const foreignMutation = await window.analytix.runtime.runtimeRequest(
      '/v1/threads/thread-1/summary/tasks/taskjob%3Ajob-1/kill',
      'POST'
    )

    expect(mutation.ok).toBe(true)
    for (const response of [foreign, poisoned, foreignMutation]) {
      expect(response.ok).toBe(false)
      expect(response.status).toBe(502)
      expect(response.body).not.toContain(privateSentinel)
    }
  })

  it('projects browser proxy failures from HTTP status without returning raw bodies', async () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        code: 'attachment_upload_unavailable',
        message: privateSentinel,
        details: privateSentinel
      }), { status: 401 }))
      .mockResolvedValueOnce(new Response(privateSentinel, { status: 404 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    const unauthorized = await window.analytix.runtime.runtimeRequest('/v1/runtime/info', 'GET')
    const notFound = await window.analytix.runtime.runtimeRequest('/v1/private', 'GET')

    expect(JSON.parse(unauthorized.body)).toEqual({
      code: 'unauthorized',
      message: 'Runtime authentication is required.'
    })
    expect(JSON.parse(notFound.body)).toEqual({
      code: 'not_found',
      message: 'The requested resource was not found.'
    })
    expect(JSON.stringify([unauthorized, notFound])).not.toContain(privateSentinel)
  })

  it('does not replace the Electron preload bridge', () => {
    const existing = { app: { platform: 'darwin' } } as Window['analytix']
    stubBrowserWindow({ analytix: existing })

    const installed = installBrowserAnalytixBridge()

    expect(installed).toBe(false)
    expect(window.analytix).toBe(existing)
    expect(document.documentElement.dataset.bridge).toBeUndefined()
  })

  it('does not install outside local browser preview origins by default', () => {
    stubBrowserWindow({
      location: {
        protocol: 'https:',
        hostname: 'example.com'
      } as Location
    })

    const installed = installBrowserAnalytixBridge()

    expect(installed).toBe(false)
    expect(window.analytix).toBeUndefined()
    expect(document.documentElement.dataset.bridge).toBeUndefined()
  })

  it('does not expose deprecated Kun or Reasonix bridge aliases', () => {
    stubBrowserWindow()
    vi.stubGlobal('fetch', vi.fn())

    const installed = installBrowserAnalytixBridge()

    expect(installed).toBe(true)
    expect(window.analytix).toBeDefined()
    expect((window as typeof window & { kun?: unknown }).kun).toBeUndefined()
    expect((window as typeof window & { kunGui?: unknown }).kunGui).toBeUndefined()
    expect((window as typeof window & { reasonix?: unknown }).reasonix).toBeUndefined()
    expect((window as typeof window & { analytixGui?: unknown }).analytixGui).toBeUndefined()
    expect((window as typeof window & { deepseek?: unknown }).deepseek).toBeUndefined()
  })

  it('proxies browser preview SSE through the analytix runtime events path', async () => {
    const events: SseEventPayload[] = []
    let fetchCount = 0
    const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      fetchCount += 1
      if (fetchCount === 1) {
        return new Response(eventFrame(5, 'thread-1'), { status: 200 })
      }
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
      })
    })
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)

    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    const unsubscribe = window.analytix.runtime.onSseEvent((payload) => events.push(payload))

    await window.analytix.runtime.startSse('thread-1', 4, 'stream-1')
    await waitFor(() => events.length === 1)
    await expect(window.analytix.runtime.ackSseEvent('stream-1', 5, {
      batchId: '7'.repeat(64),
      threadId: 'thread-1',
      turnId: 'turn-1',
      publicationCommitId: '9'.repeat(64)
    })).resolves.toBe(false)
    await expect(window.analytix.runtime.ackSseEvent('stream-1', 5)).resolves.toBe(true)
    unsubscribe()
    await window.analytix.runtime.stopSse('stream-1')

    expect(fetch.mock.calls[0]?.[0]).toBe('/runtime-proxy/v1/threads/thread-1/events?since_seq=4&live=1')
    expect(fetch.mock.calls[0]?.[1]).toEqual(expect.objectContaining({
      headers: {
        Accept: 'text/event-stream',
        'Last-Event-ID': '4'
      }
    }))
    expect(events).toEqual([
      {
        streamId: 'stream-1',
        events: [runtimeEvent(5, 'thread-1')]
      }
    ])
    expect(JSON.stringify(events)).not.toContain('hello')
  })

  it('rejects arbitrary tool progress prose at the browser SSE boundary', async () => {
    const marker = 'SENTINEL_BROWSER_TOOL_PROGRESS_PROSE_8C42'
    const invalid = {
      ...runtimeEvent(5, 'thread-1'),
      summary: marker,
      message: `${marker}:message`
    }
    const frame = `id: 5\nevent: tool_progress\ndata: ${JSON.stringify(invalid)}\n\n`
    const events: SseEventPayload[] = []
    const errors: Array<{ streamId: string; code?: string; message?: string; reasonCode?: string }> = []
    const fetch = vi.fn(async () => new Response(frame, { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)

    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    window.analytix.runtime.onSseEvent((payload) => events.push(payload))
    window.analytix.runtime.onSseError((payload) => errors.push(payload))
    await window.analytix.runtime.startSse('thread-1', 4, 'stream-invalid-progress')
    await waitFor(() => errors.length === 1)

    expect(events).toEqual([])
    expect(errors).toEqual([{
      streamId: 'stream-invalid-progress',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'invalid_public_projection'
    }])
    expect(JSON.stringify({ events, errors })).not.toContain(marker)
    await window.analytix.runtime.stopSse('stream-invalid-progress')
  })

  it('fails closed on accepted-final HTTP and SSE projections without browser authority verification', async () => {
    const { acceptedFinal, acceptedFinalView } = acceptedFinalPair()
    const validItem = {
      id: 'item-case-final',
      turnId: acceptedFinal.turnId,
      threadId: acceptedFinal.threadId,
      role: 'assistant',
      status: 'completed',
      createdAt: acceptedFinal.acceptedAt,
      finishedAt: acceptedFinal.acceptedAt,
      kind: 'assistant_text',
      text: 'host boundary',
      acceptedFinal,
      acceptedFinalView
    }
    const invalidItem = {
      ...validItem,
      text: 'must not pass',
      acceptedFinalView: { ...acceptedFinalView, rawReceiptId: 'receipt-private' }
    }
    const eventMetadata = {
      timestamp: acceptedFinal.acceptedAt,
      threadId: acceptedFinal.threadId,
      turnId: acceptedFinal.turnId,
      itemId: validItem.id,
      acceptedFinalDigest: acceptedFinal.recordDigest,
      publicationCommitId: acceptedFinal.recordDigest,
      publicationEventId: '7'.repeat(64),
      publicationSlot: 'assistant-final',
      publicationPayloadDigest: '6'.repeat(64)
    }
    const controlled = controlledStream([
      payloadFrame(1, 'item_completed', {
        ...eventMetadata,
        item: validItem
      }),
      payloadFrame(2, 'assistant_reasoning_delta', {
        threadId: 'thread-case',
        text: 'PRIVATE_REASONING'
      }),
      payloadFrame(3, 'item_completed', { ...eventMetadata, item: invalidItem })
    ])
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        kind: 'thread_snapshot',
        items: [invalidItem]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(controlled.stream, {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    const events: SseEventPayload[] = []
    const errors: Array<{ streamId: string; code?: string; message?: string; reasonCode?: string }> = []

    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    const response = await window.analytix.runtime.runtimeRequest('/v1/threads/thread-case', 'GET')
    expect(response).toMatchObject({ ok: false, status: 502 })
    expect(JSON.parse(response.body)).toEqual({
      code: 'runtime_response_not_public',
      message: 'Runtime response was blocked at the public boundary.'
    })

    window.analytix.runtime.onSseEvent((payload) => events.push(payload))
    window.analytix.runtime.onSseError((payload) => errors.push(payload))
    await window.analytix.runtime.startSse('thread-case', 0, 'stream-case')
    await waitFor(() => errors.length === 1)
    const serialized = JSON.stringify({ events, errors })
    expect(events).toEqual([])
    expect(errors[0]).toMatchObject({ streamId: 'stream-case', code: 'sse_event_rejected' })
    expect(serialized).not.toContain('host boundary')
    expect(serialized).not.toContain('must not pass')
    expect(serialized).not.toContain('receipt-private')
    expect(serialized).not.toContain('PRIVATE_REASONING')

    await window.analytix.runtime.stopSse('stream-case')
    controlled.close()
  })

  it('rejects detached accepted-final strong markers at the browser HTTP fallback', async () => {
    const strongMarkers = [
      'acceptedFinal',
      'factFinalWitnessAdmission',
      'publicationSnapshotProof',
      'publicationSnapshotProofDigest'
    ]
    const marker = 'PRIVATE_BROWSER_ACCEPTED_FINAL_MARKER_7D21'
    const fetch = vi.fn()
    for (const key of strongMarkers) {
      fetch.mockResolvedValueOnce(new Response(JSON.stringify({
        kind: 'generic_envelope',
        nested: { [key]: marker }
      }), { status: 200 }))
    }
    fetch.mockResolvedValueOnce(new Response(JSON.stringify({
      envelope: 'generic-envelope',
      registryHead: 'generic-registry-head',
      publicationIntent: 'generic-publication-intent',
      storeDigest: 'generic-store-digest'
    }), { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    for (const [index, key] of strongMarkers.entries()) {
      const response = await window.analytix.runtime.runtimeRequest(`/v1/generic-private-${index}`, 'GET')
      expect(response).toMatchObject({ ok: false, status: 502 })
      expect(response.body).not.toContain(key)
      expect(response.body).not.toContain(marker)
    }

    const generic = await window.analytix.runtime.runtimeRequest('/v1/generic-public', 'GET')
    expect(generic).toEqual({
      ok: true,
      status: 200,
      body: JSON.stringify({
        envelope: 'generic-envelope',
        registryHead: 'generic-registry-head',
        publicationIntent: 'generic-publication-intent',
        storeDigest: 'generic-store-digest'
      })
    })
  })

  it('rejects accepted-final strong markers before a known browser response schema can transform them', async () => {
    const marker = 'PRIVATE_BROWSER_SCHEMA_MARKER_4A17'
    const fetch = vi.fn(async () => new Response(JSON.stringify({
      attachment: {
        id: 'att_0123456789abcdef01234567',
        name: 'public.txt',
        kind: 'document',
        mimeType: 'text/plain',
        byteSize: 6,
        scope: 'thread',
        createdAt: '2026-08-25T00:00:00Z',
        updatedAt: '2026-08-25T00:00:00Z',
        publicationSnapshotProofDigest: marker
      }
    }), { status: 200 }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)
    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })

    const response = await window.analytix.runtime.runtimeRequest(
      '/v1/attachments/att_0123456789abcdef01234567',
      'GET'
    )
    expect(response).toMatchObject({ ok: false, status: 502 })
    expect(JSON.parse(response.body)).toEqual({
      code: 'runtime_response_not_public',
      message: 'Runtime response was blocked at the public boundary.'
    })
    expect(response.body).not.toContain(marker)
    expect(response.body).not.toContain('publicationSnapshotProofDigest')
  })

  it('flushes the first browser preview runtime event immediately and batches later events per frame', async () => {
    vi.useFakeTimers()
    const controlled = controlledStream([eventFrame(1, 'thread-1')])
    const events: SseEventPayload[] = []
    const fetch = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)

    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    window.analytix.runtime.onSseEvent((payload) => events.push(payload))
    await window.analytix.runtime.startSse('thread-1', 0, 'stream-1')
    await vi.advanceTimersByTimeAsync(0)

    expect(events).toEqual([
      {
        streamId: 'stream-1',
        events: [runtimeEvent(1, 'thread-1')]
      }
    ])

    controlled.enqueue(eventFrame(2, 'thread-1'))
    await vi.advanceTimersByTimeAsync(0)
    expect(events).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(15)
    expect(events).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)

    expect(events).toEqual([
      {
        streamId: 'stream-1',
        events: [runtimeEvent(1, 'thread-1')]
      },
      {
        streamId: 'stream-1',
        events: [runtimeEvent(2, 'thread-1')]
      }
    ])

    await window.analytix.runtime.stopSse('stream-1')
    controlled.close()
  })

  it('delivers revocation control alone and discards a queued browser event', async () => {
    const controlled = controlledStream([eventFrame(1, 'thread-revoked')])
    const events: SseEventPayload[] = []
    const ended: string[] = []
    const fetch = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)

    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    window.analytix.runtime.onSseEvent((payload) => events.push(payload))
    window.analytix.runtime.onSseEnd(({ streamId }) => ended.push(streamId))
    await window.analytix.runtime.startSse('thread-revoked', 0, 'stream-revoked')
    await waitFor(() => events.length === 1)

    controlled.enqueue(eventFrame(2, 'thread-revoked'))
    controlled.enqueue(publicProjectionRevokedFrame('thread-revoked'))
    await waitFor(() => events.length === 2 && ended.length === 1)

    expect(events).toEqual([
      {
        streamId: 'stream-revoked',
        events: [runtimeEvent(1, 'thread-revoked')]
      },
      {
        streamId: 'stream-revoked',
        events: [{
          schemaVersion: 1,
          kind: 'public_projection_revoked',
          threadId: 'thread-revoked',
          historyAuthority: 'case_boundary_only_v1',
          code: 'case_public_authority_rejected',
          action: 'purge_case_projection',
          terminal: true
        }]
      }
    ])
    expect(JSON.stringify(events)).not.toContain('call-2')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('caps browser ACKs at delivered events and fails closed on a mismatched thread binding', async () => {
    const controlled = controlledStream([eventFrame(1, 'thread-browser-strict')])
    const events: SseEventPayload[] = []
    const errors: Array<{ streamId: string; code?: string; message?: string }> = []
    const fetch = vi.fn(async () => new Response(controlled.stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    stubBrowserWindow()
    vi.stubGlobal('fetch', fetch)

    installBrowserAnalytixBridge({ runtimeProxyPrefix: '/runtime-proxy' })
    window.analytix.runtime.onSseEvent((payload) => events.push(payload))
    window.analytix.runtime.onSseError((payload) => errors.push(payload))
    await window.analytix.runtime.startSse('thread-browser-strict', 0, 'stream-browser-strict')
    await waitFor(() => events.length === 1)

    await expect(window.analytix.runtime.ackSseEvent('stream-browser-strict', 2)).resolves.toBe(false)
    await expect(window.analytix.runtime.ackSseEvent('stream-browser-strict', 1)).resolves.toBe(true)
    controlled.enqueue(payloadFrame(9, 'heartbeat', {
      threadId: 'other-thread',
      privatePayload: 'BROWSER_PRIVATE_SENTINEL'
    }))
    await waitFor(() => errors.length === 1)

    expect(events).toEqual([{
      streamId: 'stream-browser-strict',
      events: [runtimeEvent(1, 'thread-browser-strict')]
    }])
    expect(errors).toEqual([{
      streamId: 'stream-browser-strict',
      code: 'sse_event_rejected',
      message: 'Runtime request failed (sse_event_rejected).',
      reasonCode: 'event_thread_mismatch'
    }])
    expect(JSON.stringify(events)).not.toContain('BROWSER_PRIVATE_SENTINEL')
  })

  it('drops legacy agent envelopes and Reasonix auto-plan fields from browser preview settings', async () => {
    stubBrowserWindow()
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 404 })))
    const canary = ['browser', 'preview', 'credential'].join('-')
    window.localStorage.setItem('analytix.browserPreview.settings', JSON.stringify({
      version: 1,
      agentProvider: 'kun',
      agents: {
        kun: {
          model: 'kun-model',
          apiKey: canary
        }
      },
      agent: {
        auto_plan: true
      },
      autoPlan: true,
      auto_plan: true,
      deepseek: {
        apiKey: canary
      },
      reasonix: {
        model: 'reasonix-model'
      },
      runtime: {
        model: 'deepseek-v4-pro',
        endpointFormat: 'responses',
        agentProvider: 'reasonix',
        agents: {
          reasonix: {
            model: 'reasonix-runtime-model'
          }
        },
        autoPlan: true,
        auto_plan: true,
        deepseek: {
          apiKey: canary
        },
        reasonix: {
          model: 'reasonix-runtime-model'
        }
      }
    }))

    installBrowserAnalytixBridge({ force: true })
    const loaded = await window.analytix.settings.getSettings()
    const loadedRecord = loaded as unknown as Record<string, unknown>
    const loadedRuntime = loaded.runtime as unknown as Record<string, unknown>

    expect(loaded.runtime.model).toBe('deepseek-v4-pro')
    expect(loaded.runtime.endpointFormat).toBe('responses')
    expect(JSON.stringify(loaded).includes(canary)).toBe(false)
    expect(JSON.stringify(loaded).includes('apiKey')).toBe(false)
    for (const key of ['agent', 'agentProvider', 'agents', 'autoPlan', 'auto_plan', 'deepseek', 'reasonix']) {
      expect(key in loadedRecord, `${key} must not survive as a browser preview app setting`).toBe(false)
      expect(key in loadedRuntime, `${key} must not survive as a browser preview runtime setting`).toBe(false)
    }

    await window.analytix.settings.saveSettingsSilent({ runtime: { model: 'deepseek-v4-flash' } })
    const persisted = JSON.parse(
      window.localStorage.getItem('analytix.browserPreview.settings') ?? '{}'
    ) as Record<string, unknown>
    const persistedRuntime = persisted.runtime as Record<string, unknown>

    expect(persistedRuntime.model).toBe('deepseek-v4-flash')
    expect(persistedRuntime.endpointFormat).toBe('responses')
    expect(JSON.stringify(persisted).includes(canary)).toBe(false)
    expect(JSON.stringify(persisted).includes('apiKey')).toBe(false)
    for (const key of ['agent', 'agentProvider', 'agents', 'autoPlan', 'auto_plan', 'deepseek', 'reasonix']) {
      expect(key in persisted, `${key} must not be persisted as a browser preview app setting`).toBe(false)
      expect(key in persistedRuntime, `${key} must not be persisted as a browser preview runtime setting`).toBe(false)
    }
  })

  it('persists browser preview settings in localStorage', async () => {
    stubBrowserWindow()
    vi.stubGlobal('fetch', vi.fn())

    installBrowserAnalytixBridge({ force: true })
    await window.analytix.settings.setSettings({ workspaceRoot: '/tmp/preview' })

    const next = await window.analytix.settings.getSettings()
    expect(next.workspaceRoot).toBe('/tmp/preview')
    expect(window.localStorage.getItem('analytix.browserPreview.settings')).toContain('/tmp/preview')
  })
})
