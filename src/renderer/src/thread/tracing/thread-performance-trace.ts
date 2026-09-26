import type { ThreadTraceEventName, ThreadTraceEventPayload } from '@shared/thread-trace'
import { sanitizeThreadTraceEvent } from '@shared/thread-trace'

export type { ThreadTraceEventName }
export type ThreadTraceEvent = ThreadTraceEventPayload

export type ThreadTraceSink = {
  record(event: ThreadTraceEvent): void
}

function envThreadTraceEnabled(): boolean {
  const value = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process
    ?.env?.ANALYTIX_THREAD_TRACE
    ?.trim()
    .toLowerCase()
  return value === '1' || value === 'true' || value === 'yes'
}

export function isThreadTraceEnabled(): boolean {
  if (envThreadTraceEnabled()) return true
  if (typeof window === 'undefined') return false
  try {
    return (
      window.localStorage?.getItem('ANALYTIX_THREAD_TRACE') === '1' ||
      window.localStorage?.getItem('analytix.threadTrace') === '1'
    )
  } catch {
    return false
  }
}

export class InMemoryThreadTraceSink implements ThreadTraceSink {
  readonly events: ThreadTraceEvent[] = []

  record(event: ThreadTraceEvent): void {
    this.events.push(sanitizeThreadTraceEvent(event))
  }
}

export class PersistedThreadTraceSink implements ThreadTraceSink {
  record(event: ThreadTraceEvent): void {
    if (!isThreadTraceEnabled()) return
    const api = typeof window === 'undefined' ? undefined : window.analytix?.diagnostics
    if (typeof api?.recordThreadTrace !== 'function') return
    try {
      void api.recordThreadTrace(sanitizeThreadTraceEvent(event)).catch(() => undefined)
    } catch {
      // Optional diagnostics cannot fail a React commit or terminal ACK.
    }
  }
}

// Ephemeral correlation only. No body, prompt, or sealed event is changed, and
// neither ACK nor terminal settlement waits for a component or a visible frame.
const terminalDOMTraces = new Map<string, {
  threadId: string
  turnId?: string
  lastSeq: number
  acceptedFinal: boolean
  committedAt: number
  observed: boolean
}>()

export function registerTerminalDOMTrace(
  threadId: string, itemId: string, lastSeq: number, acceptedFinal: boolean, turnId?: string
): void {
  if (!isThreadTraceEnabled()) return
  const key = `${threadId}\u0000${itemId}`
  if (terminalDOMTraces.get(key)?.lastSeq === lastSeq) return
  terminalDOMTraces.delete(key)
  terminalDOMTraces.set(key, { threadId, turnId, lastSeq, acceptedFinal, committedAt: performance.now(), observed: false })
  while (terminalDOMTraces.size > 128) {
    const oldest = terminalDOMTraces.keys().next().value
    if (oldest === undefined) break
    terminalDOMTraces.delete(oldest)
  }
}

export function observeTerminalDOMCommit(threadId: string, itemId: string, element: HTMLElement): void {
  if (!isThreadTraceEnabled() || !element.isConnected) return
  const entry = terminalDOMTraces.get(`${threadId}\u0000${itemId}`)
  if (!entry) return
  // A synthetic QA observer can bind its bounded capture to this actual answer
  // component. This marker proves a DOM commit, not compositor presentation.
  element.dataset.terminalTraceSeq = String(entry.lastSeq)
  if (entry.observed) return
  entry.observed = true
  const now = performance.now()
  new PersistedThreadTraceSink().record(createThreadTraceEvent('thread.terminal.dom_committed', {
    threadId, turnId: entry.turnId,
    data: {
      lastSeq: entry.lastSeq, acceptedFinal: entry.acceptedFinal,
      monotonicMs: now, timeOrigin: performance.timeOrigin,
      storeToDomMs: Math.max(0, now - entry.committedAt),
      documentVisible: document.visibilityState === 'visible'
    }
  }))
}

export function createThreadTraceEvent(
  name: ThreadTraceEventName,
  input: {
    timestamp?: number
    threadId?: string
    turnId?: string
    data?: Record<string, unknown>
  } = {}
): ThreadTraceEvent {
  return sanitizeThreadTraceEvent({
    name,
    timestamp: input.timestamp ?? Date.now(),
    ...(input.threadId ? { threadId: input.threadId } : {}),
    ...(input.turnId ? { turnId: input.turnId } : {}),
    ...(input.data ? { data: input.data } : {})
  })
}
