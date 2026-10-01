import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'
import {
  BundledFundsDispatchAbortedV1,
  BundledFundsDispatchDeferredV1,
  createBundledFundsOnDemandActivationV1,
  runBundledFundsDispatchWithRefreshV1,
  withCommittedBundledFundsStopV1
} from './bundled-funds-on-demand'

describe('bounded bundled Funds activation', () => {
  it('routes typed private Main requests through the pre-send dispatch and fresh pin seam', () => {
    const source = readFileSync(new URL('../index.ts', import.meta.url), 'utf8')
    const start = source.indexOf('async function localDisplayRuntimeRequest(')
    const body = source.slice(start, source.indexOf('if (runningClawScheduleMcpServer)', start))
    const dispatch = body.indexOf('runBundledFundsDispatchWithRefreshV1(')
    const refresh = body.indexOf('requestSettings = await store.load()', dispatch)
    const pin = body.indexOf('captureCurrentFinalPublicationAuthorityPin()', dispatch)
    const send = body.indexOf('const response = await fetch(', dispatch)
    expect(start).toBeGreaterThanOrEqual(0)
    expect(dispatch).toBeGreaterThanOrEqual(0)
    expect(refresh).toBeGreaterThan(dispatch)
    expect(pin).toBeGreaterThan(refresh)
    expect(send).toBeGreaterThan(pin)
  })

  it('closes Main dispatch before waiting for old requests and shares one Go lease and restart', async () => {
    let ready = false
    let releaseOldRequest: () => void = () => undefined
    const oldRequest = new Promise<void>((resolve) => { releaseOldRequest = resolve })
    const release = vi.fn(async () => {})
    const prepare = vi.fn(async () => ({ expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true, commitStop: async () => true, release }))
    const restart = vi.fn(async () => { ready = true })
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => ready,
      isManaged: () => true,
      isShuttingDown: () => false,
      prepare,
      restartWithFunds: restart
    })
    const oldDispatch = activation.runDispatch(() => oldRequest)
    const first = activation.activate()
    const second = activation.activate()
    expect(() => activation.runDispatch(async () => undefined)).toThrow(BundledFundsDispatchDeferredV1)
    expect(prepare).not.toHaveBeenCalled()
    releaseOldRequest()
    await oldDispatch
    await expect(first).resolves.toBe(true)
    await expect(second).resolves.toBe(true)
    expect(prepare).toHaveBeenCalledTimes(1)
    expect(restart).toHaveBeenCalledTimes(1)
    expect(release).toHaveBeenCalledTimes(1)
    await activation.runDispatch(async () => undefined)
  })

  it('never stops when lease is absent, request aborts, or quit begins', async () => {
    const restart = vi.fn(async () => {})
    const unavailable = createBundledFundsOnDemandActivationV1({
      isReady: () => false, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => null, restartWithFunds: restart
    })
    await expect(unavailable.activate()).resolves.toBe(false)
    const aborted = new AbortController()
    aborted.abort()
    await expect(unavailable.activate(aborted.signal)).resolves.toBe(false)
    let quitting = false
    const release = vi.fn(async () => {})
    const quittingAfterLease = createBundledFundsOnDemandActivationV1({
      isReady: () => false, isManaged: () => true, isShuttingDown: () => quitting,
      prepare: async () => { quitting = true; return { expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true, commitStop: async () => true, release } },
      restartWithFunds: restart
    })
    await expect(quittingAfterLease.activate()).resolves.toBe(false)
    expect(release).toHaveBeenCalledTimes(1)
    expect(restart).not.toHaveBeenCalled()
  })

  it('requires a current verified binding after restart and permits a later retry', async () => {
    let ready = false
    const restart = vi.fn(async () => { if (restart.mock.calls.length === 2) ready = true })
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => ready, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => ({ expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true, commitStop: async () => true, release: async () => {} }),
      restartWithFunds: restart
    })
    await expect(activation.activate()).resolves.toBe(false)
    await expect(activation.activate()).resolves.toBe(true)
    expect(restart).toHaveBeenCalledTimes(2)
  })

  it('stops only after the exact live lease is committed', async () => {
    const stop = vi.fn(async () => {})
    const commitStop = vi.fn(async () => true)
    const release = vi.fn(async () => {})
    const lease = { expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true, commitStop, release }
    await expect(withCommittedBundledFundsStopV1({ ...lease, expiresAtUnixMs: Date.now() }, stop)).resolves.toBe(false)
    await expect(withCommittedBundledFundsStopV1({ ...lease, isCurrent: () => false }, stop)).resolves.toBe(false)
    await expect(withCommittedBundledFundsStopV1({ ...lease, commitStop: async () => false }, stop)).resolves.toBe(false)
    expect(stop).not.toHaveBeenCalled()
    expect(commitStop).not.toHaveBeenCalled()
    await expect(withCommittedBundledFundsStopV1(lease, stop)).resolves.toBe(true)
    expect(commitStop).toHaveBeenCalledTimes(1)
    expect(stop).toHaveBeenCalledTimes(1)
  })

  it('runs a requested ordinary restart only after a concurrent preparation settles', async () => {
    const order: string[] = []
    let finishPrepare: () => void = () => undefined
    const prepareBarrier = new Promise<void>((resolve) => { finishPrepare = resolve })
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => false, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => {
        order.push('prepare')
        await prepareBarrier
        return { expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true,
          commitStop: async () => true, release: async () => {} }
      },
      restartWithFunds: async () => { order.push('activation restart') }
    })
    const activating = activation.activate()
    const manual = activation.runAfterInFlight(async () => { order.push('manual restart') })
    await Promise.resolve()
    expect(order).toEqual(['prepare'])
    finishPrepare()
    await activating
    await manual
    expect(order).toEqual(['prepare', 'activation restart', 'manual restart'])
  })

  it('keeps a second Funds request waiting even when Go binds before restart verification ends', async () => {
    let ready = false
    let finishVerification: () => void = () => undefined
    const verification = new Promise<void>((resolve) => { finishVerification = resolve })
    let restartEntered: () => void = () => undefined
    const entered = new Promise<void>((resolve) => { restartEntered = resolve })
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => ready, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => ({ expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true,
        commitStop: async () => true, release: async () => {} }),
      restartWithFunds: async () => { ready = true; restartEntered(); await verification }
    })
    const first = activation.activate()
    await entered
    let secondResolved = false
    const second = activation.activate().then((value) => { secondResolved = true; return value })
    await Promise.resolve()
    expect(secondResolved).toBe(false)
    finishVerification()
    await expect(first).resolves.toBe(true)
    await expect(second).resolves.toBe(true)
  })

  it('refreshes an ordinary private request after ensure and sends it once on the new Go pin', async () => {
    let finishRestart: () => void = () => undefined
    const restartBarrier = new Promise<void>((resolve) => { finishRestart = resolve })
    let ready = false
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => ready, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => ({ expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true,
        commitStop: async () => true, release: async () => {} }),
      restartWithFunds: async () => { await restartBarrier; ready = true }
    })
    let requestSettings = 'old-settings'
    let processPin = 'old-pin'
    const refresh = vi.fn(async () => { requestSettings = 'new-settings'; processPin = 'new-pin' })
    const send = vi.fn(async () => ({ requestSettings, processPin }))
    const activating = activation.activate()
    // The ordinary request completed ensure before activation closed dispatch.
    const request = runBundledFundsDispatchWithRefreshV1(activation, refresh, send)
    expect(send).not.toHaveBeenCalled()
    finishRestart()
    await activating
    await expect(request).resolves.toEqual({ requestSettings: 'new-settings', processPin: 'new-pin' })
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(send).toHaveBeenCalledTimes(1)
  })

  it('never retries an entered private mutation after transport ambiguity', async () => {
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => true, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => null, restartWithFunds: async () => {}
    })
    const refresh = vi.fn(async () => {})
    const send = vi.fn(async () => { throw new Error('transport ambiguous after send') })
    await expect(runBundledFundsDispatchWithRefreshV1(activation, refresh, send))
      .rejects.toThrow('transport ambiguous after send')
    expect(send).toHaveBeenCalledTimes(1)
    expect(refresh).not.toHaveBeenCalled()

    const deferredAfterEntry = vi.fn(async () => { throw new BundledFundsDispatchDeferredV1() })
    await expect(runBundledFundsDispatchWithRefreshV1(activation, refresh, deferredAfterEntry))
      .rejects.toBeInstanceOf(BundledFundsDispatchDeferredV1)
    expect(deferredAfterEntry).toHaveBeenCalledTimes(1)
    expect(refresh).not.toHaveBeenCalled()
  })

  it('holds Main dispatch after a lost commit ACK and release request until the exact lease is confirmed released', async () => {
    vi.useFakeTimers()
    try {
      let releaseAttempts = 0
      const release = vi.fn(async () => {
        releaseAttempts += 1
        if (releaseAttempts < 3) throw new Error('release transport lost')
      })
      const restart = vi.fn(async () => {})
      const activation = createBundledFundsOnDemandActivationV1({
        isReady: () => false, isManaged: () => true, isShuttingDown: () => false,
        prepare: async () => ({
          expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true,
          commitStop: async () => false, release
        }),
        restartWithFunds: async (lease) => { await withCommittedBundledFundsStopV1(lease, restart) }
      })
      await expect(activation.activate()).resolves.toBe(false)
      expect(() => activation.runDispatch(async () => undefined)).toThrow(BundledFundsDispatchDeferredV1)
      expect(restart).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(1_000)
      expect(() => activation.runDispatch(async () => undefined)).toThrow(BundledFundsDispatchDeferredV1)
      await vi.advanceTimersByTimeAsync(1_000)
      await expect(activation.runDispatch(async () => 'ordinary resumed')).resolves.toBe('ordinary resumed')
      expect(release).toHaveBeenCalledTimes(3)
      expect(restart).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('returns promptly when a private request aborts during activation', async () => {
    let finishRestart: () => void = () => undefined
    const restartBarrier = new Promise<void>((resolve) => { finishRestart = resolve })
    const activation = createBundledFundsOnDemandActivationV1({
      isReady: () => false, isManaged: () => true, isShuttingDown: () => false,
      prepare: async () => ({ expiresAtUnixMs: Date.now() + 30_000, isCurrent: () => true,
        commitStop: async () => true, release: async () => {} }),
      restartWithFunds: async () => { await restartBarrier }
    })
    const activating = activation.activate()
    const controller = new AbortController()
    const send = vi.fn(async () => undefined)
    const request = runBundledFundsDispatchWithRefreshV1(
      activation, async () => {}, send, controller.signal
    )
    controller.abort()
    await expect(request).rejects.toBeInstanceOf(BundledFundsDispatchAbortedV1)
    expect(send).not.toHaveBeenCalled()
    finishRestart()
    await activating
  })

  it('bounds a private request that has not entered dispatch while activation remains pending', async () => {
    vi.useFakeTimers()
    try {
      const send = vi.fn(async () => undefined)
      const pending = runBundledFundsDispatchWithRefreshV1(
        {
          runDispatch: () => { throw new BundledFundsDispatchDeferredV1() },
          waitForInFlight: () => new Promise<void>(() => undefined)
        },
        async () => undefined,
        send
      )
      const outcome = expect(pending).rejects.toBeInstanceOf(BundledFundsDispatchDeferredV1)
      await vi.advanceTimersByTimeAsync(180_000)
      await outcome
      expect(send).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })
})
