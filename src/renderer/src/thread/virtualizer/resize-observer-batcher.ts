export type RowMeasurement = {
  rowId: string
  height: number
}

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

export type ResizeObserverBatcherOptions = {
  onMeasurements: (measurements: RowMeasurement[]) => void
  scheduleFrame?: FrameScheduler
  cancelFrame?: FrameCancel
}

export class ResizeObserverBatcher {
  private readonly pending = new Map<string, number>()
  private readonly scheduleFrame: FrameScheduler
  private readonly cancelFrame: FrameCancel
  private frameId: number | null = null

  constructor(private readonly options: ResizeObserverBatcherOptions) {
    this.scheduleFrame = options.scheduleFrame ?? defaultScheduleFrame
    this.cancelFrame = options.cancelFrame ?? defaultCancelFrame
  }

  record(rowId: string, height: number): void {
    if (!rowId || !Number.isFinite(height) || height <= 0) return
    this.pending.set(rowId, Math.ceil(height))
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
    if (this.pending.size === 0) return
    const measurements = Array.from(this.pending, ([rowId, height]) => ({ rowId, height }))
    this.pending.clear()
    this.options.onMeasurements(measurements)
  }

  dispose(): void {
    const frameId = this.frameId
    if (frameId !== null) {
      this.frameId = null
      if (frameId > 0) this.cancelFrame(frameId)
    }
    this.pending.clear()
  }
}
