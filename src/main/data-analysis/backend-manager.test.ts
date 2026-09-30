import { EventEmitter } from 'node:events'
import { readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { Server, Socket } from 'node:net'
import { preProcessFile } from 'typescript'
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
const fileEffects = ['mkdir', 'writeFile', 'appendFile', 'rm', 'rename', 'unlink', 'copyFile']
const moduleEffects: [string, string[]][] = [
  ['node:child_process', ['spawn', 'spawnSync', 'exec', 'execSync', 'execFile', 'execFileSync', 'fork']],
  ['node:fs/promises', fileEffects],
  ['node:fs', [...fileEffects, ...fileEffects.map((name) => `${name}Sync`), 'createWriteStream']],
  ['node:http', ['request', 'get']], ['node:https', ['request', 'get']]
]
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
  vi.restoreAllMocks()
  vi.unstubAllEnvs()
  vi.unstubAllGlobals()
  for (const module of ['electron', ...moduleEffects.map(([name]) => name)]) vi.doUnmock(module)
  if (originalRendererUrl === undefined) Reflect.deleteProperty(process.env, 'ELECTRON_RENDERER_URL')
  else Reflect.set(process.env, 'ELECTRON_RENDERER_URL', originalRendererUrl)
})

describe('data analysis native authority quarantine', () => {
  it('keeps legacy process and native execution authority outside its dependency boundary', () => {
    const source = readFileSync(join(process.cwd(), 'src/main/data-analysis/backend-manager.ts'), 'utf8')
    const ownerDirectory = join(process.cwd(), 'src/main/data-analysis')
    const dependencyIdentity = (name: string) => name.startsWith('.')
      ? resolve(ownerDirectory, name).replace(/\.[cm]?[jt]s$/u, '')
      : name
    const dependencies = preProcessFile(source, true, true).importedFiles.map((entry) => dependencyIdentity(entry.fileName))
    for (const forbidden of [
      'node:child_process', 'child_process',
      './native-runtime-integrity', './python-runtime-integrity', './native-runtime-paths'
    ]) expect(dependencies).not.toContain(dependencyIdentity(forbidden))
  })

  it.each([false, true])('has no I/O effects across the cold lifecycle with synthetic overrides (packaged=%s)', async (isPackaged) => {
    const effects = new Map<string, ReturnType<typeof vi.fn>>()
    const guard = (name: string) => {
      const marker = vi.fn(() => { throw new Error(`synthetic forbidden effect: ${name}`) })
      effects.set(name, marker)
      return marker
    }
    const protect = (module: string, actual: Record<string, unknown>, methods: string[]) => ({
      ...actual, ...Object.fromEntries(methods.map((name) => [name, guard(`${module}.${name}`)]))
    })
    for (const [module, methods] of moduleEffects) {
      vi.doMock(module, async () => {
        const actual = await vi.importActual<Record<string, unknown>>(module)
        const guarded = protect(module, actual, methods)
        if (module === 'node:fs') {
          guarded.promises = protect('node:fs.promises', actual.promises as Record<string, unknown>, fileEffects)
        }
        return { ...guarded, default: guarded }
      })
    }
    vi.spyOn(Server.prototype, 'listen').mockImplementation(guard('server.listen'))
    vi.spyOn(Socket.prototype, 'connect').mockImplementation(guard('socket.connect'))
    vi.stubGlobal('fetch', guard('fetch'))
    vi.stubGlobal('WebSocket', guard('WebSocket'))
    vi.doMock('electron', () => ({
      app: { isPackaged, getAppPath: () => '/synthetic/analytix-app' },
      net: { fetch: guard('electron.net.fetch'), request: guard('electron.net.request') },
      protocol: {}
    }))
    for (const name of [
      'ANALYTIX_DATA_ANALYSIS_PYTHON', 'ANALYTIX_DATA_ANALYSIS_BACKEND_DIR', 'ANALYTIX_DATA_ANALYSIS_PORT',
      'ANALYTIX_DATA_ENGINE_BIN', 'ANALYTIX_ANALYSIS_COMPUTE_BIN',
      'ANALYTIX_IMPORT_ACCELERATOR_BIN', 'ANALYTIX_CLEANING_OPS_BIN'
    ]) vi.stubEnv(name, name.endsWith('_PORT') ? '18731' : '/synthetic/untrusted-override')

    // Observe import-time effects too; never clear guards after the cold import.
    vi.resetModules()
    const { DataAnalysisBackendManager: ColdManager } = await import('./backend-manager')
    const manager = new ColdManager()
    const states: unknown[] = []
    const unsubscribe = manager.onState((state) => states.push(state))
    const expected = manager.getState()
    expect(expected).toMatchObject({ blocker: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER, terminal: true, phase: 'failed' })
    expect(await Promise.all([manager.ensureBackend(), manager.ensureBackend()])).toEqual([expected, expected])
    await expect(manager.request('/health')).rejects.toThrow(DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER)
    for (const path of ['/health', 'http://127.0.0.1:18731/health', 'https://synthetic.invalid/fallback']) {
      await expect(manager.request(path, { method: 'POST', body: 'synthetic' })).rejects.toThrow(DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER)
    }
    const renderer = fakeRenderer(isPackaged ? 'analytix-app://renderer/index.html' : undefined)
    expect(manager.registerRenderer(renderer as never)).toBe(true)
    const generation = manager.getRendererAuthorityGeneration(renderer as never)!
    const lease = manager.acquireRendererAuthorityLease(renderer.id, generation)
    expect(lease?.isCurrent()).toBe(true)
    await manager.stopAndWait()
    expect(lease?.signal.aborted).toBe(true)
    expect(lease?.isCurrent()).toBe(false)
    expect(await manager.ensureBackend()).toEqual(expected)
    expect(manager.getRuntimeInfo().backend).toMatchObject({ managed: false, terminal: true })
    expect(states).toEqual([expected])
    unsubscribe()
    expect([...effects].filter(([, marker]) => marker.mock.calls.length).map(([name]) => name)).toEqual([])
  })

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
