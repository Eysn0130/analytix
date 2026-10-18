import { describe, expect, it, vi } from 'vitest'
import {
  MAX_RESTART_DELAY_MS,
  RestartBudget,
  restartAfterPendingEnsure,
  SettingsRuntimeStopHandoff
} from './analytix-runtime-supervisor'

function budgetAt(times: { value: number }): RestartBudget {
  return new RestartBudget({
    windowMs: 60_000,
    maxRestarts: 3,
    baseDelayMs: 1_000,
    delayFactor: 3,
    now: () => times.value
  })
}

describe('RestartBudget', () => {
  it('allows up to maxRestarts attempts with exponential backoff delays', () => {
    const clock = { value: 0 }
    const budget = budgetAt(clock)

    expect(budget.note()).toEqual({ allowed: true, attempt: 1, delayMs: 1_000 })
    clock.value += 1_000
    expect(budget.note()).toEqual({ allowed: true, attempt: 2, delayMs: 3_000 })
    clock.value += 1_000
    expect(budget.note()).toEqual({ allowed: true, attempt: 3, delayMs: 9_000 })
  })

  it('circuit-breaks once the window is saturated', () => {
    const clock = { value: 0 }
    const budget = budgetAt(clock)
    budget.note()
    budget.note()
    budget.note()

    const verdict = budget.note()
    expect(verdict.allowed).toBe(false)
    expect(verdict.delayMs).toBe(0)
  })

  it('frees attempts as they age out of the sliding window', () => {
    const clock = { value: 0 }
    const budget = budgetAt(clock)
    budget.note()
    budget.note()
    budget.note()
    expect(budget.note().allowed).toBe(false)

    clock.value = 60_001
    const verdict = budget.note()
    expect(verdict.allowed).toBe(true)
    expect(verdict.attempt).toBe(1)
    expect(verdict.delayMs).toBe(1_000)
  })

  it('reset() clears the window so the next crash starts fresh', () => {
    const clock = { value: 0 }
    const budget = budgetAt(clock)
    budget.note()
    budget.note()
    budget.reset()

    const verdict = budget.note()
    expect(verdict).toEqual({ allowed: true, attempt: 1, delayMs: 1_000 })
  })

  it('clamps restart delays to the maximum timer delay', () => {
    const budget = new RestartBudget({
      windowMs: 60_000,
      maxRestarts: 3,
      baseDelayMs: Number.MAX_SAFE_INTEGER,
      delayFactor: Number.MAX_SAFE_INTEGER,
      now: () => 0
    })

    expect(budget.note()).toEqual({
      allowed: true,
      attempt: 1,
      delayMs: MAX_RESTART_DELAY_MS
    })
  })

  it('falls back from non-finite numeric options', () => {
    const budget = new RestartBudget({
      windowMs: Number.NaN,
      maxRestarts: Number.NaN,
      baseDelayMs: Number.NaN,
      delayFactor: Number.NaN,
      now: () => 0
    })

    expect(budget.note()).toEqual({ allowed: true, attempt: 1, delayMs: 1_000 })
  })
})

describe('SettingsRuntimeStopHandoff', () => {
  it('retains the exact stopped child through a retired port probe for the accepted successor', async () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const child = {}
    const receipt = handoff.begin(child, 1)
    expect(handoff.confirmStopped(receipt, null)).toBe(true)
    let currentGeneration = 1
    let releasePort!: (settings: string) => void
    const port = new Promise<string>(resolve => { releasePort = resolve })
    const start = vi.fn(async (_settings: string) => undefined)
    const retired = handoff.startAfterPort(receipt, () => port, () => currentGeneration === 1, start)
    currentGeneration = 2
    releasePort('old-settings')
    await expect(retired).resolves.toBeNull()
    expect(start).not.toHaveBeenCalled()
    expect(handoff.admit(2, currentGeneration)).toBe(receipt)
    await handoff.startAfterPort(receipt, async () => 'latest-settings', () => currentGeneration === 2, start)
    expect(start).toHaveBeenCalledExactlyOnceWith('latest-settings')
    expect(handoff.hasPending()).toBe(false)
    // Clearing the pending obligation does not retire the successful runner.
    expect(handoff.owns(receipt)).toBe(true)
  })

  it('uses commit identity through A→B→A instead of equal settings content', () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    handoff.confirmStopped(receipt, null)
    expect(handoff.admit(3, 3)).toBe(receipt)
    expect(handoff.admit(2, 3)).toBeNull()
    expect(handoff.admit(1, 3, receipt)).toBeNull()
  })

  it('only admits the exact issuer at the original generation', () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    handoff.confirmStopped(receipt, null)
    expect(handoff.admit(1, 1)).toBeNull()
    expect(handoff.admit(1, 1, { ...receipt })).toBeNull()
    expect(handoff.admit(1, 1, receipt)).toBe(receipt)
  })

  it.each(['autoStart:false→true', 'manual offline', 'quit', 'explicit restart'])('does not revive a receipt after %s cancels it', async () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    handoff.confirmStopped(receipt, null)
    let releasePort!: (settings: string) => void
    const port = new Promise<string>(resolve => { releasePort = resolve })
    const start = vi.fn(async (_settings: string) => undefined)
    const task = handoff.startAfterPort(receipt, () => port, () => true, start)
    handoff.cancel()
    releasePort('stale')
    await expect(task).resolves.toBeNull()
    expect(start).not.toHaveBeenCalled()
    expect(handoff.admit(3, 3)).toBeNull()
    expect(handoff.owns(receipt)).toBe(false)
  })

  it('never confirms a late stop after cancellation or a different child appears', () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    expect(handoff.confirmStopped(receipt, {})).toBe(false)
    handoff.cancel()
    expect(handoff.confirmStopped(receipt, null)).toBe(false)
    expect(handoff.hasPending()).toBe(false)
    const later = handoff.begin({}, 2)
    expect(handoff.confirmStopped(later, null)).toBe(true)
    handoff.complete(receipt)
    expect(handoff.admit(3, 3)).toBe(later)
  })

  it('does not give an initially offline child a restart obligation', async () => {
    const handoff = new SettingsRuntimeStopHandoff()
    expect(handoff.admit(2, 2)).toBeNull()
    const unconfirmed = handoff.begin({}, 2)
    const resolvePort = vi.fn(async () => 'settings')
    const start = vi.fn(async (_settings: string) => undefined)
    await expect(handoff.startAfterPort(unconfirmed, resolvePort, () => true, start)).resolves.toBeNull()
    expect(resolvePort).not.toHaveBeenCalled()
    expect(start).not.toHaveBeenCalled()
  })

  it('keeps the obligation after failed launch instead of authorizing an old completion to clear it', async () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    handoff.confirmStopped(receipt, null)
    await expect(handoff.startAfterPort(receipt, async () => 'settings', () => true,
      async () => { throw new Error('launch failed') })).rejects.toThrow('launch failed')
    expect(handoff.admit(2, 2)).toBe(receipt)
  })

  it('observes the current rollback status epoch without reviving a cancelled start receipt', () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    handoff.confirmStopped(receipt, null)
    handoff.cancel()
    const rollbackStatus = handoff.cancellationWitness()
    expect(rollbackStatus()).toBe(true)
    expect(handoff.owns(receipt)).toBe(false)
    expect(handoff.admit(2, 2)).toBeNull()
    handoff.cancel()
    expect(rollbackStatus()).toBe(false)
  })

  it('does not let an old start completion clear a replacement stop obligation', async () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const old = handoff.begin({}, 1)
    handoff.confirmStopped(old, null)
    let finish!: () => void
    const start = new Promise<void>(resolve => { finish = resolve })
    const pending = handoff.startAfterPort(old, async () => 'old', () => true, () => start)
    await Promise.resolve()
    handoff.cancel()
    const replacement = handoff.begin({}, 2)
    handoff.confirmStopped(replacement, null)
    finish()
    await pending
    expect(handoff.admit(3, 3)).toBe(replacement)
  })

  it('keeps the exact obligation when a retired in-flight start fails', async () => {
    const handoff = new SettingsRuntimeStopHandoff()
    const receipt = handoff.begin({}, 1)
    handoff.confirmStopped(receipt, null)
    let current = 1
    let fail!: (error: Error) => void
    const start = new Promise<void>((_, reject) => { fail = reject })
    const pending = handoff.startAfterPort(receipt, async () => 'old', () => current === 1, () => start)
    await Promise.resolve()
    current = 2
    fail(new Error('retired start failed'))
    await expect(pending).rejects.toThrow('retired start failed')
    expect(handoff.admit(2, current)).toBe(receipt)
  })
})

describe('restartAfterPendingEnsure', () => {
  it('waits for an in-flight ensure before starting the replacement runtime', async () => {
    let releaseEnsure!: () => void
    const order: string[] = []
    const pendingEnsure = new Promise<void>((resolve) => {
      releaseEnsure = () => {
        order.push('ensure-settled')
        resolve()
      }
    })
    const task = restartAfterPendingEnsure(pendingEnsure, async () => {
      order.push('restart')
    })

    await Promise.resolve()
    expect(order).toEqual([])
    releaseEnsure()
    await task
    expect(order).toEqual(['ensure-settled', 'restart'])
  })

  it('starts one replacement after a failed ensure releases its child', async () => {
    const restart = vi.fn(async () => undefined)
    await expect(restartAfterPendingEnsure(Promise.reject(new Error('not ready')), restart)).resolves.toBeUndefined()
    expect(restart).toHaveBeenCalledTimes(1)
  })
})
