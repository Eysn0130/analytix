import { EventEmitter } from 'node:events'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('electron', () => ({
  app: {
    isPackaged: false,
    getAppPath: vi.fn(() => '/tmp/analytix-app')
  }
}))

import {
  DATA_ANALYSIS_AUTH_HEADER,
  DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
  DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
  DataAnalysisBackendManager,
  createDataAnalysisLaunchProof,
  createDataAnalysisLaunchToken,
  injectDataAnalysisRendererAuthHeader,
  resolveDataAnalysisRendererAuthPolicy,
  shouldCancelDataAnalysisWebSocketRedirect,
  verifyDataAnalysisLaunchReadyProof,
  withDataAnalysisAuthHeader
} from './backend-manager'

type FakeFrame = { url: string }

type FakeRenderer = EventEmitter & {
  id: number
  mainFrame: FakeFrame
  getURL: () => string
  isDestroyed: () => boolean
  send: ReturnType<typeof vi.fn>
}

const originalRendererUrl = process.env.ELECTRON_RENDERER_URL
let nextRendererID = 100

function fakeRenderer(url = 'http://127.0.0.1:5173/app'): FakeRenderer {
  const frame = { url }
  return Object.assign(new EventEmitter(), {
    id: nextRendererID++,
    mainFrame: frame,
    getURL: () => frame.url,
    isDestroyed: () => false,
    send: vi.fn()
  }) as FakeRenderer
}

beforeEach(() => {
  nextRendererID = 100
  Reflect.set(process.env, 'ELECTRON_RENDERER_URL', 'http://127.0.0.1:5173/app')
})

afterEach(() => {
  vi.unstubAllGlobals()
  if (originalRendererUrl === undefined) Reflect.deleteProperty(process.env, 'ELECTRON_RENDERER_URL')
  else Reflect.set(process.env, 'ELECTRON_RENDERER_URL', originalRendererUrl)
})

describe('data analysis native authority quarantine', () => {
  it('returns one deterministic terminal boundary with no endpoint, process, or local runtime disclosure', async () => {
    const manager = new DataAnalysisBackendManager()
    const first = await manager.ensureBackend()
    const second = await manager.ensureBackend()

    expect(first).toEqual(second)
    expect(first).toMatchObject({
      phase: 'failed',
      generation: 0,
      blocker: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
      detail: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
      authority: 'unavailable',
      terminal: true,
      restartCount: 0
    })
    expect(first).not.toHaveProperty('apiBase')
    expect(first).not.toHaveProperty('wsBase')
    expect(first).not.toHaveProperty('pid')
    expect(first).not.toHaveProperty('port')
    expect(first).not.toHaveProperty('launchId')
    expect(manager.getRuntimeInfo().backend).toMatchObject({ managed: false, terminal: true })
    expect(manager.getRuntimeInfo().backend).not.toHaveProperty('pythonExec')
    await expect(manager.stopAndWait()).resolves.toBeUndefined()
  })

  it('cannot execute env-selected Python, child processes, ports, filesystem setup, or network fallbacks', async () => {
    const source = readFileSync(join(process.cwd(), 'src/main/data-analysis/backend-manager.ts'), 'utf8')
    for (const forbidden of [
      "node:child_process",
      'ANALYTIX_DATA_ANALYSIS_PYTHON',
      'ANALYTIX_DATA_ANALYSIS_BACKEND_DIR',
      'ANALYTIX_DATA_ANALYSIS_PORT',
      'spawn(',
      'execFile(',
      'mkdir(',
      'fetch('
    ]) {
      expect(source).not.toContain(forbidden)
    }

    const fetchMarker = vi.fn()
    vi.stubGlobal('fetch', fetchMarker)
    const manager = new DataAnalysisBackendManager()
    await expect(manager.request('/health')).rejects.toThrow(DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER)
    expect(fetchMarker).not.toHaveBeenCalled()
  })

  it('notifies listeners once with the same terminal boundary', () => {
    const manager = new DataAnalysisBackendManager()
    const listener = vi.fn()
    const unsubscribe = manager.onState(listener)
    expect(listener).toHaveBeenCalledTimes(1)
    expect(listener.mock.calls[0]?.[0]).toMatchObject({
      blocker: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
      terminal: true
    })
    unsubscribe()
  })
})

describe('renderer principal and legacy launch-proof helpers', () => {
  it('pins packaged renderer authority to the fixed custom origin and entry', () => {
    const policy = resolveDataAnalysisRendererAuthPolicy(
      'http://127.0.0.1:5173/attacker-controlled-entry',
      true
    )

    expect(policy).toEqual({
      corsOrigins: ['analytix-app://renderer'],
      rendererOrigin: 'analytix-app://renderer',
      rendererEntryUrl: 'analytix-app://renderer/index.html'
    })
  })

  it('injects native auth only for the exact packaged custom-origin principal', () => {
    const token = createDataAnalysisLaunchToken()
    const policy = resolveDataAnalysisRendererAuthPolicy(undefined, true)
    const renderer = fakeRenderer('analytix-app://renderer/index.html?threadId=thr_1')
    const authority = {
      contents: renderer,
      frame: renderer.mainFrame,
      rendererEntryUrl: policy.rendererEntryUrl,
      generation: 1
    }
    const request = {
      url: 'http://127.0.0.1:18731/api',
      requestHeaders: { Origin: 'analytix-app://renderer' },
      resourceType: 'xhr',
      webContentsId: renderer.id,
      webContents: renderer as never,
      frame: renderer.mainFrame as never
    }

    expect(injectDataAnalysisRendererAuthHeader(request, {
      apiBase: 'http://127.0.0.1:18731',
      token,
      policy,
      authority: authority as never
    })[DATA_ANALYSIS_AUTH_HEADER]).toBe(token)
    expect(injectDataAnalysisRendererAuthHeader({
      ...request,
      requestHeaders: { Origin: 'null' }
    }, {
      apiBase: 'http://127.0.0.1:18731',
      token,
      policy,
      authority: authority as never
    })[DATA_ANALYSIS_AUTH_HEADER]).toBeUndefined()
    expect(injectDataAnalysisRendererAuthHeader({
      ...request,
      webContents: fakeRenderer('analytix-app://attacker/index.html') as never
    }, {
      apiBase: 'http://127.0.0.1:18731',
      token,
      policy,
      authority: authority as never
    })[DATA_ANALYSIS_AUTH_HEADER]).toBeUndefined()
  })

  it('registers only the exact renderer principal and revokes it on navigation', () => {
    const manager = new DataAnalysisBackendManager()
    const renderer = fakeRenderer()
    expect(manager.registerRenderer(renderer as never, { url: renderer.mainFrame.url } as never)).toBe(false)
    expect(manager.registerRenderer(renderer as never, renderer.mainFrame as never)).toBe(true)
    expect(manager.validateRenderer(renderer as never, renderer.mainFrame as never)).toBe(true)
    renderer.emit('did-start-navigation', { isMainFrame: true, isSameDocument: false })
    expect(manager.validateRenderer(renderer as never, renderer.mainFrame as never)).toBe(false)
    expect(manager.getAuthorizedRenderers()).toEqual([])
  })

  it('issues a generation-bound cancellable lease and aborts it before navigation can publish', async () => {
    const manager = new DataAnalysisBackendManager()
    const renderer = fakeRenderer()
    expect(manager.registerRenderer(renderer as never, renderer.mainFrame as never)).toBe(true)
    const generation = manager.getRendererAuthorityGeneration(renderer as never, renderer.mainFrame as never)
    expect(generation).toBeTypeOf('number')
    if (generation === undefined) throw new Error('expected renderer authority')
    expect(manager.acquireRendererAuthorityLease(renderer.id, generation + 1)).toBeNull()
    const lease = manager.acquireRendererAuthorityLease(renderer.id, generation)
    expect(lease).not.toBeNull()
    if (!lease) throw new Error('expected renderer lease')
    expect(lease.signal.aborted).toBe(false)
    expect(lease.isCurrent()).toBe(true)

    renderer.emit('did-start-navigation', { isMainFrame: true, isSameDocument: false })
    expect(lease.signal.aborted).toBe(true)
    expect(lease.isCurrent()).toBe(false)
    lease.release()
    lease.release()

    expect(manager.registerRenderer(renderer as never, renderer.mainFrame as never)).toBe(true)
    const nextGeneration = manager.getRendererAuthorityGeneration(renderer as never, renderer.mainFrame as never)
    expect(nextGeneration).toBeGreaterThan(generation)
    const nextLease = manager.acquireRendererAuthorityLease(renderer.id, nextGeneration!)
    expect(nextLease?.isCurrent()).toBe(true)
    await manager.stopAndWait()
    expect(nextLease?.signal.aborted).toBe(true)
    expect(nextLease?.isCurrent()).toBe(false)
  })

  it('never forwards a caller-controlled bearer header to an unbound renderer', () => {
    const token = createDataAnalysisLaunchToken()
    const policy = resolveDataAnalysisRendererAuthPolicy()
    const renderer = fakeRenderer()
    const headers = injectDataAnalysisRendererAuthHeader(
      {
        url: 'http://127.0.0.1:18731/api',
        requestHeaders: { Origin: policy.rendererOrigin ?? '', [DATA_ANALYSIS_AUTH_HEADER]: 'spoofed' },
        resourceType: 'xhr',
        webContentsId: renderer.id,
        webContents: renderer as never,
        frame: renderer.mainFrame as never
      },
      { apiBase: 'http://127.0.0.1:18731', token, policy }
    )
    expect(headers[DATA_ANALYSIS_AUTH_HEADER]).toBeUndefined()
    const request = withDataAnalysisAuthHeader({ headers: { [DATA_ANALYSIS_AUTH_HEADER]: 'spoofed' } }, token)
    expect(new Headers(request.headers).get(DATA_ANALYSIS_AUTH_HEADER)).toBe(token)
  })

  it('accepts only an exact HMAC-bound launch proof shape', () => {
    const token = createDataAnalysisLaunchToken()
    const launchId = createDataAnalysisLaunchToken()
    const challenge = createDataAnalysisLaunchToken()
    const pid = 4242
    const proof = {
      protocol: DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
      service: 'analytix-data-analysis',
      status: 'ok',
      launchId,
      challenge,
      pid,
      proof: createDataAnalysisLaunchProof({
        token,
        launchId,
        challenge,
        service: 'analytix-data-analysis',
        status: 'ok',
        pid
      })
    }
    expect(verifyDataAnalysisLaunchReadyProof(proof, { token, launchId, challenge, pid })).toBe(true)
    expect(verifyDataAnalysisLaunchReadyProof({ ...proof, extra: true }, { token, launchId, challenge, pid })).toBe(false)
    expect(verifyDataAnalysisLaunchReadyProof({ ...proof, pid: pid + 1 }, { token, launchId, challenge, pid })).toBe(false)
  })

  it('classifies loopback websocket redirects without following them', () => {
    expect(
      shouldCancelDataAnalysisWebSocketRedirect(
        { url: 'ws://127.0.0.1:18731/ws/events', statusCode: 302, resourceType: 'webSocket' },
        'http://127.0.0.1:18731'
      )
    ).toBe(true)
  })
})
