export const THREAD_TRACE_EVENT_NAMES = [
  'thread.event.batch_received',
  'thread.delta.buffered',
  'thread.delta.flushed',
  'thread.active_stream.updated',
  'thread.projection.reduced',
  'thread.rows.updated',
  'thread.virtualizer.measured',
  'thread.scroll.anchor_corrected',
  'thread.react.commit_sample',
  'thread.markdown.finalized',
  'thread.terminal.verified',
  'thread.terminal.ipc_sent',
  'thread.terminal.renderer_committed',
  'thread.terminal.next_frame'
] as const

export type ThreadTraceEventName = (typeof THREAD_TRACE_EVENT_NAMES)[number]
export type ThreadTraceDataValue = number | boolean | null

export type ThreadTraceEventPayload = {
  name: ThreadTraceEventName
  timestamp: number
  threadId?: string
  data?: Record<string, ThreadTraceDataValue>
}

export function sanitizeThreadTraceEvent(input: {
  name: ThreadTraceEventName
  timestamp: number
  threadId?: string
  data?: Record<string, unknown>
}): ThreadTraceEventPayload {
  const data: Record<string, ThreadTraceDataValue> = {}
  for (const [key, value] of Object.entries(input.data ?? {})) {
    if (!/^[a-zA-Z0-9_.:-]{1,64}$/.test(key)) continue
    if (typeof value === 'number' && Number.isFinite(value)) {
      data[key] = value
    } else if (typeof value === 'boolean' || value === null) {
      data[key] = value
    }
  }
  return {
    name: input.name,
    timestamp: input.timestamp,
    ...(input.threadId ? { threadId: input.threadId.slice(0, 256) } : {}),
    ...(Object.keys(data).length > 0 ? { data } : {})
  }
}
