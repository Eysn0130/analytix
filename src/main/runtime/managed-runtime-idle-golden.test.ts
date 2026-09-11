import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppSettingsV1 } from '../../shared/app-settings'

const transport = vi.hoisted(() => ({ base: vi.fn(), headers: vi.fn() }))
vi.mock('./analytix-adapter', () => ({
  getRuntimeBaseUrlForSettings: transport.base,
  runtimeAuthHeaders: transport.headers
}))

import { runtimeThreadsListHasActiveTurn, waitForRuntimeTurnsIdle } from './managed-runtime-idle'
import { classifyRuntimeActivity, observeRuntimeIdle } from './runtime-idle-observer'

const settings = Object.freeze({}) as AppSettingsV1
const list = (status: string) => ({ ok: true, status: 200, body: JSON.stringify({ threads: [{ status }] }) })
const active = ['queued', 'in_progress', 'started', 'running']
const inactive: unknown[] = ['idle', 'completed', 'failed', 'cancelled', 'RUNNING', '', null, 7, {}]

describe.each([
  { name: 'service', hasActive: runtimeThreadsListHasActiveTurn },
  { name: 'isolated', hasActive: (body: string) => classifyRuntimeActivity(body) === 'active' }
])('$name managed restart activity golden', ({ hasActive }) => {
  for (const status of [...active, ...inactive]) {
    it(`summary and direct turn status ${JSON.stringify(status)}`, () => {
      for (const thread of [{ status }, { status: 'idle', turns: [{ status }] }]) {
        expect(hasActive(JSON.stringify({ threads: [null, [], 7, thread] })))
          .toBe(active.includes(status as string))
      }
    })
  }
  it.each(['invalid-json', 'null', '[]', '{}', '{"threads":{}}'])('no active match for %s', (body) => {
    expect(hasActive(body)).toBe(false)
  })
})

describe('managed restart polling golden', () => {
  let now: number
  beforeEach(() => {
    now = 1_000
    vi.spyOn(Date, 'now').mockImplementation(() => now)
    transport.base.mockReturnValue('http://127.0.0.1:54321')
    transport.headers.mockReturnValue({ Authorization: 'Bearer SYNTHETIC_IDLE_TEST_ONLY' })
  })
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

  it.each([
    { statuses: ['idle'], timeout: 0, interval: 25, sleeps: [], result: 'idle' },
    { statuses: ['running'], timeout: 0, interval: 25, sleeps: [], result: 'timeout' },
    { statuses: ['queued'], timeout: -10, interval: 25, sleeps: [], result: 'timeout' },
    { statuses: ['running', 'idle'], timeout: 100, interval: 25, sleeps: [25], result: 'idle' },
    { statuses: ['running', 'running', 'running'], timeout: 30, interval: 25, sleeps: [25, 5], result: 'timeout' },
    { statuses: ['running', 'idle'], timeout: 10, interval: 100, sleeps: [10], result: 'idle' }
  ])('sequence $statuses with timeout $timeout', async ({ statuses, timeout, interval, sleeps, result }) => {
    const fetchThreads = vi.fn()
    for (const status of statuses) fetchThreads.mockResolvedValueOnce(list(status))
    const sleepMs = vi.fn(async (ms: number) => { now += ms })
    expect(await waitForRuntimeTurnsIdle({ settings, fetchThreads, sleepMs, timeoutMs: timeout, intervalMs: interval }))
      .toBe(result)
    expect(fetchThreads).toHaveBeenCalledTimes(statuses.length)
    expect(fetchThreads).toHaveBeenCalledWith(settings)
    expect(sleepMs.mock.calls.flat()).toEqual(sleeps)

    now = 1_000
    fetchThreads.mockClear()
    sleepMs.mockClear()
    for (const status of statuses) fetchThreads.mockResolvedValueOnce(list(status))
    expect(await observeRuntimeIdle({
      read: () => fetchThreads(settings), sleep: sleepMs, timeoutMs: timeout, intervalMs: interval
    })).toBe(result)
    expect(fetchThreads).toHaveBeenCalledTimes(statuses.length)
    expect(sleepMs.mock.calls.flat()).toEqual(sleeps)
  })

  it.each([401, 403, 500, 503])('HTTP %i is unavailable and cannot be treated as idle', async (status) => {
    const sleepMs = vi.fn()
    expect(await waitForRuntimeTurnsIdle({
      settings, sleepMs,
      fetchThreads: async () => ({ ok: false, status, body: 'SYNTHETIC_PRIVATE_ERROR' })
    })).toBe('unavailable')
    expect(sleepMs).not.toHaveBeenCalled()
  })

  it('fetch failure is projected, but a caller sleep failure is propagated', async () => {
    const failure = new Error('SYNTHETIC_POLL_FAILURE')
    expect(await waitForRuntimeTurnsIdle({ settings, fetchThreads: async () => { throw failure } })).toBe('unavailable')
    await expect(waitForRuntimeTurnsIdle({
      settings, fetchThreads: async () => list('running'), sleepMs: async () => { throw failure }
    })).rejects.toBe(failure)
  })

  it.each(['invalid-json', 'null', '[]', '{}', '{"threads":{}}'])('invalid payload %s cannot establish idle', async (body) => {
    const sleep = vi.fn()
    expect(await observeRuntimeIdle({
      read: async () => ({ ok: true, body }), sleep, timeoutMs: 100, intervalMs: 10
    })).toBe('unavailable')
    expect(await waitForRuntimeTurnsIdle({
      settings, fetchThreads: async () => ({ ok: true, status: 200, body }), sleepMs: sleep
    })).toBe('unavailable')
    expect(sleep).not.toHaveBeenCalled()
  })

  it('uses defaults, one in-flight request, and fresh state on retry', async () => {
    let calls = 0
    let inFlight = 0
    const sleepMs = vi.fn(async (ms: number) => { now += ms })
    const fetchThreads = async () => {
      expect(inFlight++).toBe(0)
      await Promise.resolve()
      inFlight--
      return list(++calls % 2 === 0 ? 'idle' : 'running')
    }
    for (let retry = 0; retry < 2; retry += 1) {
      expect(await waitForRuntimeTurnsIdle({ settings, fetchThreads, sleepMs })).toBe('idle')
    }
    expect(sleepMs.mock.calls).toEqual([[1000], [1000]])
    expect(inFlight).toBe(0)
  })

  it('uses the existing authenticated local transport and 5 second abort deadline', async () => {
    const timeout = vi.spyOn(AbortSignal, 'timeout')
    const request = vi.fn().mockResolvedValue(new Response(JSON.stringify({ threads: [] })))
    vi.stubGlobal('fetch', request)
    expect(await waitForRuntimeTurnsIdle({ settings })).toBe('idle')
    expect(transport.base).toHaveBeenCalledWith(settings)
    expect(transport.headers).toHaveBeenCalledWith(settings)
    expect(timeout).toHaveBeenCalledWith(5000)
    expect(request).toHaveBeenCalledExactlyOnceWith('http://127.0.0.1:54321/v1/threads?limit=500&include=side', {
      headers: { Authorization: 'Bearer SYNTHETIC_IDLE_TEST_ONLY' }, signal: expect.any(AbortSignal)
    })
  })
})
