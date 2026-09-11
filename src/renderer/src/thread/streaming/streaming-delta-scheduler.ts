import type { ThreadDeltaEvent } from '../../agent/types'
import { StreamingTextBuffer, type StreamingTextSnapshot } from './streaming-text-buffer'

type FrameScheduler = (callback: () => void) => number
type FrameCancel = (id: number) => void

export type StreamingDeltaSchedulerOptions = {
  onFlush: (snapshot: StreamingTextSnapshot) => void
  onBuffered?: (deltaCount: number) => void
  onFlushed?: (snapshot: StreamingTextSnapshot) => void
  scheduleFrame?: FrameScheduler
  cancelFrame?: FrameCancel
  forceSynchronous?: boolean
  flushFirstSynchronously?: boolean
}

function defaultScheduleFrame(callback: () => void): number {
  if (typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function') {
    return window.requestAnimationFrame(callback)
  }
  callback()
  return 0
}

function defaultCancelFrame(id: number): void {
  if (id && typeof window !== 'undefined' && typeof window.cancelAnimationFrame === 'function') {
    window.cancelAnimationFrame(id)
  }
}

export class StreamingDeltaScheduler {
  private readonly buffer = new StreamingTextBuffer()
  private readonly scheduleFrame: FrameScheduler
  private readonly cancelFrame: FrameCancel
  private frameId: number | null = null
  private firstFlushDone = false

  constructor(private readonly options: StreamingDeltaSchedulerOptions) {
    this.scheduleFrame = options.scheduleFrame ?? defaultScheduleFrame
    this.cancelFrame = options.cancelFrame ?? defaultCancelFrame
  }

  enqueue(deltas: ThreadDeltaEvent[]): void {
    if (deltas.length === 0) return
    this.buffer.append(deltas)
    this.options.onBuffered?.(deltas.length)
    if (this.options.forceSynchronous) {
      this.flushNow()
      return
    }
    if (this.options.flushFirstSynchronously && !this.firstFlushDone) {
      this.firstFlushDone = true
      this.flushNow()
      return
    }
    if (this.frameId !== null) return
    this.frameId = -1
    const scheduledId = this.scheduleFrame(() => this.flushNow())
    if (this.frameId !== null) this.frameId = scheduledId
  }

  flushNow(): void {
    const frameId = this.frameId
    if (frameId !== null) {
      this.frameId = null
      if (frameId > 0) this.cancelFrame(frameId)
    }
    const snapshots = this.buffer.drainAll()
    for (const snapshot of snapshots) {
      this.options.onFlushed?.(snapshot)
      this.options.onFlush(snapshot)
    }
  }

  discard(): void {
    const frameId = this.frameId
    if (frameId !== null) {
      this.frameId = null
      if (frameId > 0) this.cancelFrame(frameId)
    }
    this.buffer.clear()
  }

  dispose(): void {
    this.discard()
  }
}

export function createStreamingDeltaScheduler(options: StreamingDeltaSchedulerOptions): StreamingDeltaScheduler {
  return new StreamingDeltaScheduler(options)
}
