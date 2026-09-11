import { describe, expect, it, vi } from 'vitest'
import { buildEventStreamResponse } from '../src/server-test-support/routes/events.js'
import type { EventBus } from '../src/ports/event-bus.js'
import type { SessionStore } from '../src/ports/session-store.js'
import { readSseEvents } from './http-server-test-harness.js'

function eventData(frame: string): Record<string, unknown> {
  const line = frame.split('\n').find((item) => item.startsWith('data:'))
  if (!line) throw new Error(`missing SSE data line: ${frame}`)
  return JSON.parse(line.slice(5).trim()) as Record<string, unknown>
}

describe('runtime events route', () => {
  it('emits structured terminal setup errors with thread and seq context', async () => {
    const eventBus = {
      publish: vi.fn(),
      subscribe: vi.fn(() => () => {}),
      snapshotSince: vi.fn(() => []),
      highestSeq: vi.fn(() => 0),
      reset: vi.fn()
    } as unknown as EventBus
    const sessionStore = {
      highestSeq: vi.fn(async () => 5),
      loadEventsSince: vi.fn(async () => {
        throw new Error('events.jsonl is unreadable')
      })
    } as unknown as SessionStore

    const response = buildEventStreamResponse({
      request: new Request('http://localhost/v1/threads/thr_setup/events?since_seq=3'),
      threadId: 'thr_setup',
      eventBus,
      sessionStore
    })
    const frames = await readSseEvents(response)

    expect(frames).toHaveLength(1)
    expect(frames[0]).toContain('id: 3')
    expect(frames[0]).toContain('event: error')
    expect(eventData(frames[0])).toMatchObject({
      kind: 'error',
      seq: 3,
      threadId: 'thr_setup',
      code: 'sse_setup_error',
      message: 'events.jsonl is unreadable',
      severity: 'error',
      terminal: true,
      details: {
        phase: 'setup'
      }
    })
  })
})
