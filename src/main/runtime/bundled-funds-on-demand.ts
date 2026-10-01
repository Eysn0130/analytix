export class BundledFundsDispatchDeferredV1 extends Error {
  constructor() {
    super('Runtime dispatch is held for bundled Funds activation.')
    this.name = 'BundledFundsDispatchDeferredV1'
  }
}

export class BundledFundsDispatchAbortedV1 extends Error {
  constructor() {
    super('Runtime request was aborted while bundled Funds activation was pending.')
    this.name = 'BundledFundsDispatchAbortedV1'
  }
}

export type BundledFundsMaintenanceLeaseV1 = Readonly<{
  expiresAtUnixMs: number
  isCurrent: () => boolean
  commitStop: () => Promise<boolean>
  release: () => Promise<void>
}>

type ActivationDependenciesV1 = Readonly<{
  isReady: () => boolean
  isManaged: () => boolean
  isShuttingDown: () => boolean
  prepare: () => Promise<BundledFundsMaintenanceLeaseV1 | null>
  restartWithFunds: (lease: BundledFundsMaintenanceLeaseV1) => Promise<void>
}>

const DISPATCH_DRAIN_TIMEOUT_MS = 30_000
const DISPATCH_WAIT_TIMEOUT_MS = 180_000
const RELEASE_RECOVERY_RETRY_MS = 1_000

export function createBundledFundsOnDemandActivationV1(deps: ActivationDependenciesV1): {
  activate: (signal?: AbortSignal) => Promise<boolean>
  runDispatch: <T>(send: () => Promise<T>) => Promise<T>
  runAfterInFlight: <T>(transition: () => Promise<T>) => Promise<T>
  waitForInFlight: () => Promise<void>
  isInFlight: () => boolean
} {
  let inFlight: Promise<boolean> | null = null
  let closed = false
  let unresolvedLease: BundledFundsMaintenanceLeaseV1 | null = null
  let activeDispatches = 0
  const drainWaiters = new Set<() => void>()

  const waitForDispatches = async (): Promise<boolean> => {
    if (activeDispatches === 0) return true
    return new Promise<boolean>((resolve) => {
      let settled = false
      const finish = (idle: boolean): void => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        drainWaiters.delete(onDrained)
        resolve(idle)
      }
      const onDrained = (): void => finish(true)
      const timer = setTimeout(() => finish(false), DISPATCH_DRAIN_TIMEOUT_MS)
      drainWaiters.add(onDrained)
    })
  }

  const runDispatch = <T>(send: () => Promise<T>): Promise<T> => {
    if (closed) throw new BundledFundsDispatchDeferredV1()
    activeDispatches += 1
    const finish = (): void => {
      activeDispatches -= 1
      if (activeDispatches === 0) {
        for (const waiter of [...drainWaiters]) waiter()
      }
    }
    try {
      return Promise.resolve(send()).finally(finish)
    } catch (error) {
      finish()
      throw error
    }
  }

  const activate = (signal?: AbortSignal): Promise<boolean> => {
    if (signal?.aborted || deps.isShuttingDown()) return Promise.resolve(false)
    if (inFlight) return inFlight.then((ready) => ready && !signal?.aborted)
    if (unresolvedLease || closed) return Promise.resolve(false)
    if (deps.isReady()) return Promise.resolve(true)
    if (!deps.isManaged()) return Promise.resolve(false)
    closed = true
    const task = (async (): Promise<boolean> => {
      let lease: BundledFundsMaintenanceLeaseV1 | null = null
      try {
        if (!await waitForDispatches()) return false
        if (signal?.aborted || deps.isShuttingDown() || !deps.isManaged()) return false
        lease = await deps.prepare()
        if (!lease || signal?.aborted || deps.isShuttingDown() || !deps.isManaged() || !lease.isCurrent() ||
          Date.now() + 1_000 >= lease.expiresAtUnixMs) return false
        await deps.restartWithFunds(lease)
        return !signal?.aborted && !deps.isShuttingDown() && deps.isReady()
      } catch {
        return false
      } finally {
        if (lease) {
          try {
            await lease.release()
          } catch {
            if (lease.isCurrent()) unresolvedLease = lease
          }
        }
      }
    })()
    const tracked = task.then((ready) => ready && unresolvedLease === null).finally(() => {
      if (unresolvedLease) {
        const lease = unresolvedLease
        void (async () => {
          while (unresolvedLease === lease && !deps.isShuttingDown()) {
            await new Promise<void>((resolve) => setTimeout(resolve, RELEASE_RECOVERY_RETRY_MS))
            if (!lease.isCurrent()) break
            try {
              await lease.release()
              break
            } catch { /* Retain Main's dispatch fence and retry the exact lease. */ }
          }
          if (unresolvedLease === lease && (!lease.isCurrent() || !deps.isShuttingDown())) {
            unresolvedLease = null
            closed = false
          }
        })()
      } else {
        closed = false
      }
      if (inFlight === tracked) inFlight = null
    })
    inFlight = tracked
    return tracked
  }

  const runAfterInFlight = <T>(transition: () => Promise<T>): Promise<T> => {
    const pending = inFlight
    return pending
      ? pending.then(() => runAfterInFlight(transition), () => runAfterInFlight(transition))
      : transition()
  }

  return {
    activate,
    runDispatch,
    runAfterInFlight,
    waitForInFlight: async () => { await inFlight?.then(() => undefined, () => undefined) },
    isInFlight: () => inFlight !== null
  }
}

// Deferred is thrown only before send enters runDispatch. Rebuild the request
// against the new Go process after that wait; never retry an entered send.
export async function runBundledFundsDispatchWithRefreshV1<T>(
  activation: Pick<ReturnType<typeof createBundledFundsOnDemandActivationV1>, 'runDispatch' | 'waitForInFlight'>,
  refresh: () => Promise<void>,
  send: () => Promise<T>,
  signal?: AbortSignal
): Promise<T> {
  const deadline = Date.now() + DISPATCH_WAIT_TIMEOUT_MS
  for (let attempt = 0; attempt < 3; attempt += 1) {
    if (signal?.aborted) throw new BundledFundsDispatchAbortedV1()
    let sendEntered = false
    try {
      return await activation.runDispatch(() => {
        sendEntered = true
        return send()
      })
    } catch (error) {
      if (!(error instanceof BundledFundsDispatchDeferredV1) || sendEntered || attempt === 2) throw error
      const remainingMs = deadline - Date.now()
      if (remainingMs <= 0) throw new BundledFundsDispatchDeferredV1()
      const inFlight = activation.waitForInFlight()
      let timer: ReturnType<typeof setTimeout> | null = null
      const timedOut = new Promise<void>((_, reject) => {
        timer = setTimeout(() => reject(new BundledFundsDispatchDeferredV1()), remainingMs)
      })
      if (signal) {
        let onAbort: () => void = () => undefined
        const aborted = new Promise<void>((resolve) => {
          onAbort = resolve
          if (signal.aborted) resolve()
          else signal.addEventListener('abort', onAbort, { once: true })
        })
        try {
          await Promise.race([inFlight, aborted, timedOut])
        } finally {
          if (timer) clearTimeout(timer)
          signal.removeEventListener('abort', onAbort)
        }
      } else {
        try { await Promise.race([inFlight, timedOut]) } finally { if (timer) clearTimeout(timer) }
      }
      if (signal?.aborted) throw new BundledFundsDispatchAbortedV1()
      await refresh()
    }
  }
  throw new BundledFundsDispatchDeferredV1()
}

// A timed preparation is never a stop permit. Go must atomically convert the
// same-process lease to a non-expiring fence immediately before Main stops it.
export async function withCommittedBundledFundsStopV1(
  lease: BundledFundsMaintenanceLeaseV1,
  stopAndRestart: () => Promise<void>
): Promise<boolean> {
  if (!lease.isCurrent() || Date.now() + 1_000 >= lease.expiresAtUnixMs) return false
  if (!await lease.commitStop() || !lease.isCurrent()) return false
  await stopAndRestart()
  return true
}
