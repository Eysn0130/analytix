export type MarkdownFinalizationJob = {
  rowId: string
  text: string
}

type ScheduledMarkdownFinalizationJob = MarkdownFinalizationJob & {
  onFinalize: (job: MarkdownFinalizationJob) => void
}

type ScheduleFinalization = (callback: () => void) => number
type CancelFinalization = (handle: number) => void

export class MarkdownFinalizationQueue<Job extends MarkdownFinalizationJob = MarkdownFinalizationJob> {
  private readonly jobs = new Map<string, Job>()

  enqueue(job: Job): void {
    this.jobs.set(job.rowId, job)
  }

  delete(rowId: string): void {
    this.jobs.delete(rowId)
  }

  drain(limit = Number.POSITIVE_INFINITY): Job[] {
    const out: Job[] = []
    for (const [rowId, job] of this.jobs) {
      if (out.length >= limit) break
      out.push(job)
      this.jobs.delete(rowId)
    }
    return out
  }

  get size(): number {
    return this.jobs.size
  }
}

export type MarkdownFinalizationSchedulerOptions = {
  schedule?: ScheduleFinalization
  cancel?: CancelFinalization
  maxJobsPerFrame?: number
}

function defaultSchedule(callback: () => void): number {
  if (typeof window === 'undefined') {
    return globalThis.setTimeout(callback, 0) as unknown as number
  }
  const requestIdle = window.requestIdleCallback
  if (typeof requestIdle === 'function') {
    return requestIdle(() => callback(), { timeout: 120 })
  }
  if (typeof window.requestAnimationFrame === 'function') {
    return window.requestAnimationFrame(() => callback())
  }
  return window.setTimeout(callback, 0)
}

function defaultCancel(handle: number): void {
  if (typeof window === 'undefined') {
    globalThis.clearTimeout(handle)
    return
  }
  if (typeof window.cancelIdleCallback === 'function') {
    window.cancelIdleCallback(handle)
    return
  }
  if (typeof window.cancelAnimationFrame === 'function') {
    window.cancelAnimationFrame(handle)
    return
  }
  window.clearTimeout(handle)
}

export class MarkdownFinalizationScheduler {
  private readonly queue = new MarkdownFinalizationQueue<ScheduledMarkdownFinalizationJob>()
  private readonly schedule: ScheduleFinalization
  private readonly cancel: CancelFinalization
  private readonly maxJobsPerFrame: number
  private handle: number | null = null

  constructor(options: MarkdownFinalizationSchedulerOptions = {}) {
    this.schedule = options.schedule ?? defaultSchedule
    this.cancel = options.cancel ?? defaultCancel
    this.maxJobsPerFrame = Math.max(1, Math.floor(options.maxJobsPerFrame ?? 2))
  }

  enqueue(job: ScheduledMarkdownFinalizationJob): () => void {
    this.queue.enqueue(job)
    this.ensureScheduled()
    return () => this.queue.delete(job.rowId)
  }

  flushNow(limit = this.maxJobsPerFrame): number {
    const jobs = this.queue.drain(limit)
    for (const job of jobs) {
      job.onFinalize({ rowId: job.rowId, text: job.text })
    }
    return jobs.length
  }

  get size(): number {
    return this.queue.size
  }

  dispose(): void {
    if (this.handle !== null) {
      this.cancel(this.handle)
      this.handle = null
    }
    this.queue.drain()
  }

  private ensureScheduled(): void {
    if (this.handle !== null) return
    this.handle = this.schedule(() => {
      this.handle = null
      this.flushNow()
      if (this.queue.size > 0) this.ensureScheduled()
    })
  }
}

export const sharedMarkdownFinalizationScheduler = new MarkdownFinalizationScheduler()
