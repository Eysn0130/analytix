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
    void api.recordThreadTrace(sanitizeThreadTraceEvent(event)).catch(() => undefined)
  }
}

export function createThreadTraceEvent(
  name: ThreadTraceEventName,
  input: {
    timestamp?: number
    threadId?: string
    data?: Record<string, unknown>
  } = {}
): ThreadTraceEvent {
  return sanitizeThreadTraceEvent({
    name,
    timestamp: input.timestamp ?? Date.now(),
    ...(input.threadId ? { threadId: input.threadId } : {}),
    ...(input.data ? { data: input.data } : {})
  })
}
