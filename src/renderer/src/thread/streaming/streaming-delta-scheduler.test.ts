import { describe, expect, it } from 'vitest'
import { StreamingDeltaScheduler } from './streaming-delta-scheduler'
import {
  MarkdownFinalizationQueue,
  MarkdownFinalizationScheduler
} from './markdown-finalization-queue'

describe('StreamingDeltaScheduler', () => {
  it('flushes multiple delta batches at most once per frame', () => {
    const frames: Array<() => void> = []
    const flushed: unknown[] = []
    const scheduler = new StreamingDeltaScheduler({
      scheduleFrame: (callback) => {
        frames.push(callback)
        return frames.length
      },
      cancelFrame: () => undefined,
      onFlush: (snapshot) => flushed.push(snapshot)
    })

    scheduler.enqueue([{ kind: 'agent_message', text: 'he', seq: 1 }])
    scheduler.enqueue([{ kind: 'agent_message', text: 'llo', seq: 2 }])
    scheduler.enqueue([{ kind: 'agent_message', text: '', seq: 3 }])

    expect(frames).toHaveLength(1)
    expect(flushed).toHaveLength(0)
    frames[0]?.()
    expect(flushed).toEqual([{ assistant: 'hello', lastSeq: 3, turnId: null }])
  })

  it('can flush the first delta synchronously and batch later deltas by frame', () => {
    const frames: Array<() => void> = []
    const flushed: unknown[] = []
    const scheduler = new StreamingDeltaScheduler({
      scheduleFrame: (callback) => {
        frames.push(callback)
        return frames.length
      },
      cancelFrame: () => undefined,
      flushFirstSynchronously: true,
      onFlush: (snapshot) => flushed.push(snapshot)
    })

    scheduler.enqueue([{ kind: 'agent_message', text: 'he', seq: 1 }])
    expect(frames).toHaveLength(0)
    expect(flushed).toEqual([{ assistant: 'he', lastSeq: 1, turnId: null }])

    scheduler.enqueue([{ kind: 'agent_message', text: 'l', seq: 2 }])
    scheduler.enqueue([{ kind: 'agent_message', text: 'lo', seq: 3 }])
    expect(frames).toHaveLength(1)
    expect(flushed).toHaveLength(1)

    frames[0]?.()
    expect(flushed).toEqual([
      { assistant: 'he', lastSeq: 1, turnId: null },
      { assistant: 'llo', lastSeq: 3, turnId: null }
    ])
  })

  it('keeps multiple turn streams isolated within the same frame', () => {
    const frames: Array<() => void> = []
    const flushed: unknown[] = []
    const scheduler = new StreamingDeltaScheduler({
      scheduleFrame: (callback) => {
        frames.push(callback)
        return frames.length
      },
      cancelFrame: () => undefined,
      onFlush: (snapshot) => flushed.push(snapshot)
    })

    scheduler.enqueue([
      { kind: 'agent_message', text: 'A1', seq: 1, turnId: 'turn-a' },
      { kind: 'agent_message', text: '', seq: 2, turnId: 'turn-b' }
    ])
    scheduler.enqueue([
      { kind: 'agent_message', text: 'A2', seq: 3, turnId: 'turn-a' },
      { kind: 'agent_message', text: 'B1', seq: 4, turnId: 'turn-b' }
    ])

    expect(frames).toHaveLength(1)
    frames[0]?.()
    expect(flushed).toEqual([
      { assistant: 'A1A2', lastSeq: 3, turnId: 'turn-a' },
      { assistant: 'B1', lastSeq: 4, turnId: 'turn-b' }
    ])
  })

  it('deduplicates markdown finalization jobs by row id', () => {
    const queue = new MarkdownFinalizationQueue()
    queue.enqueue({ rowId: 'row-a', text: 'first' })
    queue.enqueue({ rowId: 'row-a', text: 'final' })
    queue.enqueue({ rowId: 'row-b', text: 'second' })

    expect(queue.drain()).toEqual([
      { rowId: 'row-a', text: 'final' },
      { rowId: 'row-b', text: 'second' }
    ])
    expect(queue.size).toBe(0)
  })

  it('runs markdown finalization through a shared frame-budgeted scheduler', () => {
    const frames: Array<() => void> = []
    const finalized: string[] = []
    const scheduler = new MarkdownFinalizationScheduler({
      maxJobsPerFrame: 1,
      schedule: (callback) => {
        frames.push(callback)
        return frames.length
      },
      cancel: () => undefined
    })

    scheduler.enqueue({
      rowId: 'row-a',
      text: 'first',
      onFinalize: (job) => finalized.push(job.text)
    })
    scheduler.enqueue({
      rowId: 'row-b',
      text: 'second',
      onFinalize: (job) => finalized.push(job.text)
    })

    expect(frames).toHaveLength(1)
    frames[0]?.()
    expect(finalized).toEqual(['first'])
    expect(frames).toHaveLength(2)
    frames[1]?.()
    expect(finalized).toEqual(['first', 'second'])
  })
})
