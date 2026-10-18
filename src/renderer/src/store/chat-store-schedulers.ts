import type { ChatBlock } from '../agent/types'
import type { ChatState, ChatStoreGet, ChatStoreSet } from './chat-store-types'

let startupRuntimeProbeTimer: ReturnType<typeof setTimeout> | null = null
let busyWatchdogTimer: ReturnType<typeof setTimeout> | null = null
let busyRecoveryAttempts = 0
let turnCompletionPollTimer: ReturnType<typeof setInterval> | null = null
let turnCompletionPollGeneration = 0

type BusyWatchdogOptions = {
  timeoutMs: number
  maxAttempts: number
  finalizeBusyState: (state: ChatState) => Partial<ChatState>
  flushLiveBlocks: (state: ChatState, base: Partial<ChatState>) => Partial<ChatState>
  busyTimeoutMessage: () => string
  onFinalTimeout?: (state: ChatState) => void
}

type TurnCompletionPollOptions = {
  captureThreadOwner: (state: ChatState, threadId: string) => (() => boolean) | null
  loadThreadState: (
    state: ChatState,
    threadId: string
  ) => Promise<{ blocks: ChatBlock[]; threadStatus?: string }>
  threadLooksRunning: (blocks: ChatBlock[], threadStatus?: string) => boolean
  onCompletedThreads: (
    doneIds: string[],
    state: ChatState,
    set: ChatStoreSet,
    get: ChatStoreGet,
    isOwnerCurrent: (threadId: string) => boolean
  ) => void | Promise<void>
}

export function scheduleStartupRuntimeProbe(get: ChatStoreGet): void {
  if (startupRuntimeProbeTimer) {
    clearTimeout(startupRuntimeProbeTimer)
  }
	  startupRuntimeProbeTimer = setTimeout(() => {
	    startupRuntimeProbeTimer = null
	    void get().probeRuntime('user')
	  }, 120)
	}

export function clearBusyWatchdog(): void {
  if (busyWatchdogTimer) {
    clearTimeout(busyWatchdogTimer)
    busyWatchdogTimer = null
  }
}

export function resetBusyRecoveryAttempts(): void {
  busyRecoveryAttempts = 0
}

export function armBusyWatchdog(
  set: ChatStoreSet,
  get: ChatStoreGet,
  options: BusyWatchdogOptions
): void {
  clearBusyWatchdog()
  busyWatchdogTimer = setTimeout(() => {
    const state = get()
    if (!state.busy) return
    busyRecoveryAttempts += 1
    if (busyRecoveryAttempts <= options.maxAttempts && state.activeThreadId) {
      void state.recoverActiveTurn()
      return
    }
    options.onFinalTimeout?.(state)
    set((snapshot) => {
      const base: Partial<ChatState> = {
        ...options.finalizeBusyState(snapshot),
        busy: false,
        currentTurnId: null,
        error: options.busyTimeoutMessage()
      }
      return options.flushLiveBlocks(snapshot, base)
    })
  }, options.timeoutMs)
}

function clearTurnCompletionPollTimer(): void {
  // An idle timer does not retire an already claimed completion's guarded refresh.
  if (turnCompletionPollTimer) {
    clearInterval(turnCompletionPollTimer)
    turnCompletionPollTimer = null
  }
}

export function stopTurnCompletionPoll(): void {
  turnCompletionPollGeneration += 1
  clearTurnCompletionPollTimer()
}

export function syncTurnCompletionPoll(
  set: ChatStoreSet,
  get: ChatStoreGet,
  options: TurnCompletionPollOptions
): void {
  if (get().runtimeConnection !== 'ready') {
    stopTurnCompletionPoll()
    return
  }
  const ids = Object.keys(get().watchTurnCompletion).filter((id) => get().watchTurnCompletion[id])
  if (ids.length === 0) {
    clearTurnCompletionPollTimer()
    return
  }
  if (turnCompletionPollTimer != null) return

  const generation = ++turnCompletionPollGeneration
  const tick = (): void => {
    void pollTurnCompletionWatch(set, get, options, generation)
  }

  turnCompletionPollTimer = setInterval(tick, 2500)
  void tick()
}

async function pollTurnCompletionWatch(
  set: ChatStoreSet,
  get: ChatStoreGet,
  options: TurnCompletionPollOptions,
  generation: number
): Promise<void> {
  const pollCurrent = (): boolean => (
    generation === turnCompletionPollGeneration && get().runtimeConnection === 'ready'
  )
  if (generation !== turnCompletionPollGeneration) return
  const state = get()
  if (state.runtimeConnection !== 'ready') {
    stopTurnCompletionPoll()
    return
  }

  const ids = Object.keys(state.watchTurnCompletion).filter((id) => state.watchTurnCompletion[id])
  if (ids.length === 0) {
    clearTurnCompletionPollTimer()
    return
  }

  const doneIds: string[] = []
  const owners = new Map<string, () => boolean>()
  const isOwnerCurrent = (threadId: string): boolean => (
    pollCurrent() && owners.get(threadId)?.() === true
  )
  for (const threadId of ids) {
    if (!pollCurrent()) return
    try {
      const owner = options.captureThreadOwner(state, threadId)
      if (!owner) continue
      owners.set(threadId, owner)
      if (!isOwnerCurrent(threadId) || !get().watchTurnCompletion[threadId]) continue
      const { blocks, threadStatus } = await options.loadThreadState(state, threadId)
      if (
        isOwnerCurrent(threadId) && get().watchTurnCompletion[threadId] &&
        !options.threadLooksRunning(blocks, threadStatus)
      ) {
        doneIds.push(threadId)
      }
    } catch {
      /* ignore */
    }
  }

  const currentDoneIds = doneIds.filter((threadId) => (
    isOwnerCurrent(threadId) && get().watchTurnCompletion[threadId]
  ))
  if (currentDoneIds.length > 0) {
    await options.onCompletedThreads(currentDoneIds, state, set, get, isOwnerCurrent)
  }

  if (
    pollCurrent() &&
    Object.keys(get().watchTurnCompletion).filter((id) => get().watchTurnCompletion[id]).length === 0
  ) {
    clearTurnCompletionPollTimer()
  }
}
