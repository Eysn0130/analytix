import { describe, expect, it } from 'vitest'
import { createThreadTraceEvent, InMemoryThreadTraceSink } from './thread-performance-trace'

describe('thread performance trace', () => {
  it('records analytix-owned thread trace event names', () => {
    const sink = new InMemoryThreadTraceSink()
    sink.record(
      createThreadTraceEvent('thread.projection.reduced', {
        timestamp: 123,
        threadId: 'thread-1',
        data: { rows: 3 }
      })
    )

    expect(sink.events).toEqual([
      {
        name: 'thread.projection.reduced',
        timestamp: 123,
        threadId: 'thread-1',
        data: { rows: 3 }
      }
    ])
  })

  it('drops string trace metadata before it can be persisted', () => {
    const sink = new InMemoryThreadTraceSink()
    sink.record(
      createThreadTraceEvent('thread.markdown.finalized', {
        timestamp: 124,
        threadId: 'thread-1',
        data: { text: 'do not store', codeBlocks: 2, streamed: false }
      })
    )

    expect(sink.events[0]?.data).toEqual({ codeBlocks: 2, streamed: false })
  })
})
