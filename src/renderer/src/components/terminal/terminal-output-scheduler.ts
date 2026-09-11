type FrameScheduler = (callback: () => void) => number
type FrameCancel = (id: number) => void

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

export type TerminalOutputSchedulerOptions = {
  write: (data: string) => void
  scheduleFrame?: FrameScheduler
  cancelFrame?: FrameCancel
}

export class TerminalOutputScheduler {
  private readonly pending: string[] = []
  private readonly scheduleFrame: FrameScheduler
  private readonly cancelFrame: FrameCancel
  private frameId: number | null = null

  constructor(private readonly options: TerminalOutputSchedulerOptions) {
    this.scheduleFrame = options.scheduleFrame ?? defaultScheduleFrame
    this.cancelFrame = options.cancelFrame ?? defaultCancelFrame
  }

  enqueue(data: string): void {
    if (!data) return
    this.pending.push(data)
    if (this.frameId !== null) return
    this.frameId = -1
    const scheduledId = this.scheduleFrame(() => this.flush())
    if (this.frameId !== null) this.frameId = scheduledId
  }

  flush(): void {
    const frameId = this.frameId
    if (frameId !== null) {
      this.frameId = null
      if (frameId > 0) this.cancelFrame(frameId)
    }
    if (this.pending.length === 0) return
    const data = this.pending.join('')
    this.pending.length = 0
    this.options.write(data)
  }

  dispose(): void {
    const frameId = this.frameId
    if (frameId !== null) {
      this.frameId = null
      if (frameId > 0) this.cancelFrame(frameId)
    }
    this.pending.length = 0
  }
}
