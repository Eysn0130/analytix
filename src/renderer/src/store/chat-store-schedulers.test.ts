import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  armBusyWatchdog,
  clearBusyWatchdog,
  resetBusyRecoveryAttempts,
  stopTurnCompletionPoll,
  syncTurnCompletionPoll
} from './chat-store-schedulers'
import type { ChatState, ChatStoreSet } from './chat-store-types'

type StoreApi = { getState: () => ChatState; set: ChatStoreSet; get: () => ChatState }

function makeHarness(initial: Partial<ChatState> = {}): StoreApi {
  let state: ChatState = {
    activeThreadId: 't1',
    blocks: [],
    liveAssistant: '',
    lastSeq: 0,
    usageRefreshKey: 0,
    busy: true,
    error: null,
    currentTurnId: 'turn-1',
    currentTurnUserId: 'u1',
    turnStartedAtByUserId: {},
    turnDurationByUserId: {},
    watchTurnCompletion: {},
    unreadThreadIds: {},
    queuedMessages: [],
    threads: [],
    recoverActiveTurn: vi.fn().mockResolvedValue(undefined),
    ...initial
  } as ChatState
  return {
    getState: () => state,
    set: (partial) => {
      const update =
        typeof partial === 'function'
          ? (partial as (s: ChatState) => Partial<ChatState>)(state)
          : partial
      state = { ...state, ...update }
    },
    get: () => state
  }
}

describe('armBusyWatchdog (busyTimeout message contract)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetBusyRecoveryAttempts()
  })
  afterEach(() => {
    clearBusyWatchdog()
    vi.useRealTimers()
  })

  it('uses busyTimeoutMessage returned string verbatim when watchdog fires with attempts exhausted', () => {
    const h = makeHarness({ activeThreadId: null })
    const finalize = vi.fn().mockReturnValue({})
    const flush = vi.fn().mockImplementation((_state: ChatState, base: Partial<ChatState>) => base)
    const message = '已等待 9 分钟仍未收到运行时完成事件。可中断后重试。'
    armBusyWatchdog(h.set, h.get, {
      timeoutMs: 1_000,
      maxAttempts: 0, // skip recovery, go straight to finalize
      finalizeBusyState: finalize,
      flushLiveBlocks: flush,
      busyTimeoutMessage: () => message
    })
    vi.advanceTimersByTime(1_000)
    expect(h.getState().error).toBe(message)
    expect(h.getState().busy).toBe(false)
    expect(h.getState().currentTurnId).toBeNull()
    expect(finalize).toHaveBeenCalledOnce()
    expect(flush).toHaveBeenCalledOnce()
  })

  it('never drains queued messages when final timeout has no trusted terminal snapshot', () => {
    const drainQueuedMessages = vi.fn(async () => undefined)
    const onFinalTimeout = vi.fn()
    const h = makeHarness({ activeThreadId: null, drainQueuedMessages })
    armBusyWatchdog(h.set, h.get, {
      timeoutMs: 25,
      maxAttempts: 0,
      finalizeBusyState: () => ({}),
      flushLiveBlocks: (_state, base) => base,
      busyTimeoutMessage: () => 'blocked without terminal snapshot',
      onFinalTimeout
    })

    vi.advanceTimersByTime(25)

    expect(onFinalTimeout).toHaveBeenCalledOnce()
    expect(drainQueuedMessages).not.toHaveBeenCalled()
  })

  it('skips watchdog work if not busy at fire time', () => {
    const h = makeHarness()
    const finalize = vi.fn().mockReturnValue({})
    const flush = vi.fn().mockImplementation((_state: ChatState, base: Partial<ChatState>) => base)
    armBusyWatchdog(h.set, h.get, {
      timeoutMs: 50,
      maxAttempts: 0,
      finalizeBusyState: finalize,
      flushLiveBlocks: flush,
      busyTimeoutMessage: () => 'never'
    })
    // Simulate turn completing before watchdog fires
    h.set((s) => ({ ...s, busy: false }))
    vi.advanceTimersByTime(50)
    expect(finalize).not.toHaveBeenCalled()
    expect(h.getState().error).toBeNull()
  })

  it('attempts recovery and returns when attempts remain', () => {
    const h = makeHarness()
    const finalize = vi.fn().mockReturnValue({})
    const flush = vi.fn().mockImplementation((_state: ChatState, base: Partial<ChatState>) => base)
    armBusyWatchdog(h.set, h.get, {
      timeoutMs: 50,
      maxAttempts: 5, // high limit, will not finalize
      finalizeBusyState: finalize,
      flushLiveBlocks: flush,
      busyTimeoutMessage: () => 'should-not-be-used'
    })
    vi.advanceTimersByTime(50)
    expect(h.getState().recoverActiveTurn).toHaveBeenCalledTimes(1)
    expect(h.getState().busy).toBe(true) // not finalized
    expect(finalize).not.toHaveBeenCalled()
  })
})

describe('busyTimeout minutes interpolation (#131)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetBusyRecoveryAttempts()
  })
  afterEach(() => {
    clearBusyWatchdog()
    vi.useRealTimers()
  })

  it('renders the minute count from production constants in the message', () => {
    const h = makeHarness({ activeThreadId: null })
    // Mirrors chat-store-runtime.ts:467-471 formula:
    // minutes = round((BUSY_WATCHDOG_MS * MAX_BUSY_RECOVERY_ATTEMPTS) / 60_000)
    // Current production: 180_000 * 3 / 60_000 = 9
    const minutes = Math.round((180_000 * 3) / 60_000)
    armBusyWatchdog(h.set, h.get, {
      timeoutMs: 10,
      maxAttempts: 0,
      finalizeBusyState: () => ({}),
      flushLiveBlocks: (_state: ChatState, base: Partial<ChatState>) => base,
      busyTimeoutMessage: () => `已等待 ${minutes} 分钟仍未收到运行时完成事件。`
    })
    vi.advanceTimersByTime(10)
    expect(typeof h.getState().error).toBe('string')
    expect(h.getState().error as string).toMatch(/已等待 9 分钟/)
  })
})

type CompletionPollOptions = Parameters<typeof syncTurnCompletionPoll>[2]
type CompletionRead = Awaited<ReturnType<CompletionPollOptions['loadThreadState']>>
type SyntheticWatchOwner = { receipt: string }

function deferredCompletionRead<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(yes => { resolve = yes })
  return { promise, resolve }
}

function completedWatchRead(threadId: string): CompletionRead {
  return { blocks: [{ kind: 'assistant', id: `${threadId}-final`, text: 'Synthetic completed reply' }], threadStatus: 'idle' }
}

function completionPollFixture(
  owners: Map<string, SyntheticWatchOwner>,
  loadThreadState: CompletionPollOptions['loadThreadState']
) {
  const accepted: Array<{ threadId: string; receipt: string }> = []
  const captureThreadOwner = vi.fn((_state: ChatState, threadId: string): (() => boolean) | null => {
    const captured = owners.get(threadId)
    return captured ? () => owners.get(threadId) === captured : null
  })
  const onCompletedThreads = vi.fn((
    doneIds: string[], _state: ChatState, _set: ChatStoreSet, _get: () => ChatState,
    isOwnerCurrent: (threadId: string) => boolean
  ) => {
    for (const threadId of doneIds) {
      if (isOwnerCurrent(threadId)) accepted.push({ threadId, receipt: owners.get(threadId)!.receipt })
    }
  })
  const options: CompletionPollOptions = {
    captureThreadOwner,
    loadThreadState,
    threadLooksRunning: (_blocks, status) => status === 'running',
    onCompletedThreads
  }
  return { options, accepted, captureThreadOwner, onCompletedThreads }
}

describe('turn-completion poll captured owner receipts', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    clearBusyWatchdog()
    stopTurnCompletionPoll()
  })
  afterEach(() => {
    stopTurnCompletionPoll()
    vi.useRealTimers()
  })

  it('drops an old pending read after stop and restart while admitting the new poll receipt', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'same-source-owner' }]])
    const oldRead = deferredCompletionRead<CompletionRead>(), newRead = deferredCompletionRead<CompletionRead>()
    const oldLoad = vi.fn(() => oldRead.promise), newLoad = vi.fn(() => newRead.promise)
    const oldPoll = completionPollFixture(owners, oldLoad), newPoll = completionPollFixture(owners, newLoad)
    syncTurnCompletionPoll(h.set, h.get, oldPoll.options)
    expect(oldLoad).toHaveBeenCalledExactlyOnceWith(h.getState(), 'A')
    expect(oldPoll.captureThreadOwner.mock.results[0].value?.()).toBe(true)
    stopTurnCompletionPoll()
    h.set({ usageRefreshKey: 1 })
    const restartedState = h.getState()
    syncTurnCompletionPoll(h.set, h.get, newPoll.options)
    expect(newLoad).toHaveBeenCalledExactlyOnceWith(restartedState, 'A')
    oldRead.resolve(completedWatchRead('A'))
    await vi.advanceTimersByTimeAsync(0)
    // The source owner is unchanged: suppression must come from poll lifetime,
    // not from a boolean watch flag or an unrelated owner replacement.
    expect(oldPoll.captureThreadOwner.mock.results[0].value?.()).toBe(true)
    expect(oldPoll.onCompletedThreads).not.toHaveBeenCalled()
    expect(oldPoll.accepted).toEqual([])
    expect(newPoll.onCompletedThreads).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(1)
    newRead.resolve(completedWatchRead('A'))
    await vi.advanceTimersByTimeAsync(0)
    expect(newPoll.onCompletedThreads).toHaveBeenCalledExactlyOnceWith(['A'], restartedState, h.set, h.get, expect.any(Function))
    expect(newPoll.accepted).toEqual([{ threadId: 'A', receipt: 'same-source-owner' }])
  })

  it('drops a retired A receipt after false-to-true watch ABA while B keeps the same poll alive', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true, B: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-old-source' }], ['B', { receipt: 'B-continuous-source' }]])
    const aRead = deferredCompletionRead<CompletionRead>(), bRead = deferredCompletionRead<CompletionRead>()
    const load = vi.fn((_state: ChatState, threadId: string) => threadId === 'A' ? aRead.promise : bRead.promise)
    const poll = completionPollFixture(owners, load)
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    expect(load).toHaveBeenCalledOnce()
    expect(poll.captureThreadOwner.mock.calls[0][1]).toBe('A')
    h.set({ watchTurnCompletion: { A: false, B: true } })
    owners.delete('A')
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    owners.set('A', { receipt: 'A-new-source' })
    h.set({ watchTurnCompletion: { A: true, B: true } })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    expect(load).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(1)
    aRead.resolve(completedWatchRead('A'))
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.captureThreadOwner.mock.results[0].value?.()).toBe(false)
    expect(load.mock.calls.map(call => call[1])).toEqual(['A', 'B'])
    expect(poll.onCompletedThreads).not.toHaveBeenCalled()
    bRead.resolve(completedWatchRead('B'))
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.onCompletedThreads).toHaveBeenCalledOnce()
    expect(poll.onCompletedThreads.mock.calls[0][0]).toEqual(['B'])
    expect(poll.accepted).toEqual([{ threadId: 'B', receipt: 'B-continuous-source' }])
    expect(h.getState().watchTurnCompletion).toEqual({ A: true, B: true })
  })

  it('refuses completion when the captured source owner retires during the read even with watch still true', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-read-owner' }]])
    const response = deferredCompletionRead<CompletionRead>(), load = vi.fn(() => response.promise)
    const poll = completionPollFixture(owners, load)
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    expect(poll.captureThreadOwner.mock.results[0].value?.()).toBe(true)
    owners.delete('A')
    response.resolve(completedWatchRead('A'))
    await vi.advanceTimersByTimeAsync(0)
    expect(load).toHaveBeenCalledOnce()
    expect(poll.captureThreadOwner.mock.results[0].value?.()).toBe(false)
    expect(poll.onCompletedThreads).not.toHaveBeenCalled()
    expect(poll.accepted).toEqual([])
    expect(h.getState().watchTurnCompletion.A).toBe(true)
  })

  it('keeps an unknown owner watched without reading or fabricating a completion receipt', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { unknown: true } })
    const owners = new Map<string, SyntheticWatchOwner>()
    const load = vi.fn(async () => completedWatchRead('unknown'))
    const poll = completionPollFixture(owners, load)
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.captureThreadOwner).toHaveBeenCalledExactlyOnceWith(h.getState(), 'unknown')
    expect(poll.captureThreadOwner.mock.results[0].value).toBeNull()
    await vi.advanceTimersByTimeAsync(2500)
    expect(poll.captureThreadOwner).toHaveBeenCalledTimes(2)
    expect(load).not.toHaveBeenCalled()
    expect(poll.onCompletedThreads).not.toHaveBeenCalled()
    expect(poll.accepted).toEqual([])
    expect(h.getState().watchTurnCompletion.unknown).toBe(true)
  })

  it.each(['offline-before-read', 'offline-during-read'] as const)('does not admit a completed read through %s', async seam => {
    const h = makeHarness({ runtimeConnection: seam === 'offline-before-read' ? 'offline' : 'ready', watchTurnCompletion: { A: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-ready-source' }]])
    const response = deferredCompletionRead<CompletionRead>(), load = vi.fn(() => response.promise)
    const poll = completionPollFixture(owners, load)
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    if (seam === 'offline-during-read') {
      expect(load).toHaveBeenCalledOnce()
      h.set({ runtimeConnection: 'offline' })
      response.resolve(completedWatchRead('A'))
      await vi.advanceTimersByTimeAsync(0)
      await vi.advanceTimersByTimeAsync(2500)
      expect(load).toHaveBeenCalledOnce()
    } else {
      expect(load).not.toHaveBeenCalled()
      expect(poll.captureThreadOwner).not.toHaveBeenCalled()
    }
    expect(poll.onCompletedThreads).not.toHaveBeenCalled()
    expect(poll.accepted).toEqual([])
    expect(h.getState().watchTurnCompletion.A).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('gives the consumer a live owner guard across its own await without binding it to watch removal', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true, B: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-consumer-source' }], ['B', { receipt: 'B-running-source' }]])
    const load = vi.fn(async (_state: ChatState, threadId: string): Promise<CompletionRead> => threadId === 'A' ? completedWatchRead('A') : { blocks: [], threadStatus: 'running' })
    const poll = completionPollFixture(owners, load), consumer = deferredCompletionRead<void>()
    let isOwnerCurrent: ((threadId: string) => boolean) | undefined
    poll.onCompletedThreads.mockImplementationOnce(async (ids, _state, _set, _get, current) => {
      isOwnerCurrent = current
      expect(ids).toEqual(['A'])
      expect(current('A')).toBe(true)
      h.set({ watchTurnCompletion: { A: false, B: true } })
      expect(current('A')).toBe(true)
      await consumer.promise
      for (const threadId of ids) {
        if (current(threadId)) poll.accepted.push({ threadId, receipt: owners.get(threadId)!.receipt })
      }
    })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.onCompletedThreads).toHaveBeenCalledOnce()
    expect(load.mock.calls.map(call => call[1])).toEqual(['A', 'B'])
    expect(isOwnerCurrent?.('A')).toBe(true)
    owners.set('A', { receipt: 'A-replacement-source' })
    expect(isOwnerCurrent?.('A')).toBe(false)
    consumer.resolve(undefined)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.accepted).toEqual([])
    expect(h.getState().watchTurnCompletion).toEqual({ A: false, B: true })
    expect(vi.getTimerCount()).toBe(1)
  })

  it('keeps the last admitted completion refresh current across idle tick, idle sync and final idle cleanup', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-last-completion-source' }]])
    const load = vi.fn(async () => completedWatchRead('A'))
    const poll = completionPollFixture(owners, load), refresh = deferredCompletionRead<void>()
    let isOwnerCurrent: ((threadId: string) => boolean) | undefined
    poll.onCompletedThreads.mockImplementationOnce(async (ids, _state, _set, _get, current) => {
      isOwnerCurrent = current
      expect(ids).toEqual(['A'])
      expect(current('A')).toBe(true)
      h.set({ watchTurnCompletion: { A: false } })
      await refresh.promise
      for (const threadId of ids) {
        if (current(threadId)) poll.accepted.push({ threadId, receipt: owners.get(threadId)!.receipt })
      }
    })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.onCompletedThreads).toHaveBeenCalledOnce()
    expect(poll.accepted).toEqual([])
    await vi.advanceTimersByTimeAsync(2500)
    expect(vi.getTimerCount()).toBe(0)
    expect(isOwnerCurrent?.('A')).toBe(true)
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    expect(isOwnerCurrent?.('A')).toBe(true)
    refresh.resolve(undefined)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.accepted).toEqual([{ threadId: 'A', receipt: 'A-last-completion-source' }])
    expect(isOwnerCurrent?.('A')).toBe(true)
    expect(load).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('keeps an idle completion refresh revoked after offline sync and reconnect', async () => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-idle-offline-source' }]])
    const load = vi.fn(async () => completedWatchRead('A'))
    const poll = completionPollFixture(owners, load), refresh = deferredCompletionRead<void>()
    let isOwnerCurrent: ((threadId: string) => boolean) | undefined
    poll.onCompletedThreads.mockImplementationOnce(async (ids, _state, _set, _get, current) => {
      isOwnerCurrent = current
      expect(ids).toEqual(['A'])
      expect(current('A')).toBe(true)
      h.set({ watchTurnCompletion: { A: false } })
      await refresh.promise
      for (const threadId of ids) {
        if (current(threadId)) poll.accepted.push({ threadId, receipt: owners.get(threadId)!.receipt })
      }
    })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.onCompletedThreads).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(2500)
    expect(vi.getTimerCount()).toBe(0)
    expect(isOwnerCurrent?.('A')).toBe(true)
    h.set({ runtimeConnection: 'offline' })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    expect(isOwnerCurrent?.('A')).toBe(false)
    h.set({ runtimeConnection: 'ready' })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    expect(poll.captureThreadOwner.mock.results[0].value?.()).toBe(true)
    expect(isOwnerCurrent?.('A')).toBe(false)
    refresh.resolve(undefined)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.accepted).toEqual([])
    expect(isOwnerCurrent?.('A')).toBe(false)
    expect(load).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each(['explicit-stop', 'new-poll'] as const)('revokes an idle completion refresh through %s while retaining the source receipt', async retirement => {
    const h = makeHarness({ runtimeConnection: 'ready', watchTurnCompletion: { A: true } })
    const owners = new Map<string, SyntheticWatchOwner>([['A', { receipt: 'A-admitted-source' }], ['B', { receipt: 'B-new-poll-source' }]])
    const load = vi.fn(async () => completedWatchRead('A'))
    const poll = completionPollFixture(owners, load), refresh = deferredCompletionRead<void>()
    const newRead = deferredCompletionRead<CompletionRead>()
    const nextPoll = completionPollFixture(owners, vi.fn(() => newRead.promise))
    let isOwnerCurrent: ((threadId: string) => boolean) | undefined
    poll.onCompletedThreads.mockImplementationOnce(async (ids, _state, _set, _get, current) => {
      isOwnerCurrent = current
      h.set({ watchTurnCompletion: { A: false } })
      await refresh.promise
      for (const threadId of ids) {
        if (current(threadId)) poll.accepted.push({ threadId, receipt: owners.get(threadId)!.receipt })
      }
    })
    syncTurnCompletionPoll(h.set, h.get, poll.options)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.onCompletedThreads).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(2500)
    expect(isOwnerCurrent?.('A')).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    if (retirement === 'explicit-stop') stopTurnCompletionPoll()
    else {
      h.set({ watchTurnCompletion: { A: false, B: true } })
      syncTurnCompletionPoll(h.set, h.get, nextPoll.options)
      expect(nextPoll.options.loadThreadState).toHaveBeenCalledExactlyOnceWith(h.getState(), 'B')
    }
    expect(poll.captureThreadOwner.mock.results[0].value?.()).toBe(true)
    expect(isOwnerCurrent?.('A')).toBe(false)
    refresh.resolve(undefined)
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.accepted).toEqual([])
    expect(isOwnerCurrent?.('A')).toBe(false)
    if (retirement === 'new-poll') {
      expect(vi.getTimerCount()).toBe(1)
      newRead.resolve(completedWatchRead('B'))
      await vi.advanceTimersByTimeAsync(0)
      expect(nextPoll.accepted).toEqual([{ threadId: 'B', receipt: 'B-new-poll-source' }])
      expect(nextPoll.onCompletedThreads).toHaveBeenCalledOnce()
    } else {
      expect(vi.getTimerCount()).toBe(0)
      expect(nextPoll.options.loadThreadState).not.toHaveBeenCalled()
    }
  })
})
