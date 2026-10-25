import type { ScheduledTaskV1 } from '@shared/app-settings'
import type { ScheduleTaskListResult } from '@shared/schedule-task-mutation'
import { desktopQueryCache } from '../../lib/desktop-query-cache'
import { useEffect, useState, useSyncExternalStore } from 'react'

export type ScheduleTaskCatalogSnapshot = {
  tasks: ScheduledTaskV1[]
  phase: 'idle' | 'loading' | 'ready' | 'error' | 'unavailable'
  readRequest: number
  readSettled: number
}

type CatalogDeps = {
  read: () => Promise<ScheduleTaskListResult>
  listen: (invalidate: () => void) => () => void
}

/** One Host read shared by all mounted consumers; ordinals express causality, not wall time. */
export class ScheduleTaskCatalog {
  private snapshot: ScheduleTaskCatalogSnapshot = { tasks: [], phase: 'idle', readRequest: 0, readSettled: 0 }
  private listeners = new Set<() => void>()
  private removeInvalidation: (() => void) | null = null
  private epoch = 0
  private queued = false
  private inFlight: Promise<void> | null = null

  constructor(private readonly deps: CatalogDeps) {}

  getSnapshot = (): ScheduleTaskCatalogSnapshot => this.snapshot

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    if (!this.removeInvalidation) this.removeInvalidation = this.deps.listen(() => this.invalidate())
    // Coalesce mounts in a commit. Even an existing read can predate a new consumer.
    this.invalidate()
    return () => {
      this.listeners.delete(listener)
      if (this.listeners.size === 0) {
        this.removeInvalidation?.()
        this.removeInvalidation = null
        this.epoch += 1
        this.inFlight = null
        this.snapshot = { ...this.snapshot, phase: 'idle' }
      }
    }
  }

  private publish(snapshot: ScheduleTaskCatalogSnapshot): void {
    this.snapshot = snapshot
    for (const listener of this.listeners) listener()
  }

  invalidate(): void {
    this.epoch += 1
    this.inFlight = null
    this.publish({ ...this.snapshot, phase: 'idle' })
    if (this.listeners.size === 0 || this.queued) return
    this.queued = true
    queueMicrotask(() => {
      this.queued = false
      if (this.listeners.size > 0) void this.refresh()
    })
  }

  refresh = (): Promise<void> => {
    if (this.inFlight) return this.inFlight
    const epoch = this.epoch
    const request = this.snapshot.readRequest + 1
    this.publish({ ...this.snapshot, phase: 'loading', readRequest: request })
    const promise = Promise.resolve().then(() => this.deps.read()).then((result) => {
      if (epoch !== this.epoch) return
      if (result.kind === 'success') {
        this.publish({ tasks: result.tasks, phase: 'ready', readRequest: request, readSettled: request })
      } else {
        this.publish({ ...this.snapshot, phase: result.kind === 'unavailable' ? 'unavailable' : 'error' })
      }
    }).catch(() => {
      if (epoch === this.epoch) this.publish({ ...this.snapshot, phase: 'error' })
    }).finally(() => {
      if (this.inFlight === promise) this.inFlight = null
    })
    this.inFlight = promise
    return promise
  }
}

export function selectScheduleTask(
  snapshot: ScheduleTaskCatalogSnapshot,
  taskId: string,
  mountReadRequest: number
): { kind: 'found'; task: ScheduledTaskV1 } | { kind: 'missing' | 'pending' } {
  if (snapshot.phase !== 'ready' || snapshot.readSettled <= mountReadRequest) return { kind: 'pending' }
  const task = snapshot.tasks.find((item) => item.id === taskId)
  if (task) return { kind: 'found', task }
  return { kind: 'missing' }
}

/** A fresh answer is retained across transient refreshes, including confirmed deletion. */
export function useCurrentScheduleTask(taskId: string): {
  task: ScheduledTaskV1 | null | undefined
  snapshot: ScheduleTaskCatalogSnapshot
} {
  const snapshot = useSyncExternalStore(scheduleTaskCatalog.subscribe, scheduleTaskCatalog.getSnapshot)
  const [scope, setScope] = useState<{ id: string; baseline: number; confirmed: ScheduledTaskV1 | null | undefined }>(
    () => ({ id: taskId, baseline: snapshot.readRequest, confirmed: undefined })
  )
  const current = scope.id === taskId ? scope : { id: taskId, baseline: snapshot.readRequest, confirmed: undefined }
  if (current !== scope) setScope(current)
  const selected = selectScheduleTask(snapshot, taskId, current.baseline)
  const task = selected.kind === 'found' ? selected.task : selected.kind === 'missing' ? null : current.confirmed
  useEffect(() => { scheduleTaskCatalog.invalidate() }, [taskId])
  useEffect(() => {
    if (selected.kind === 'pending') return
    setScope(previous => previous.id === taskId && previous.confirmed !== task
      ? { ...previous, confirmed: task } : previous)
  }, [taskId, selected.kind, task])
  return { task, snapshot }
}

export const scheduleTaskCatalog = new ScheduleTaskCatalog({
  read: async () => {
    const listTasks = window.analytix?.schedule?.listTasks
    return typeof listTasks === 'function'
      ? listTasks()
      : { kind: 'unavailable', message: 'Schedule task catalog is unavailable.' }
  },
  listen: (invalidate) => desktopQueryCache.subscribe(({ queryKey }) => {
    if (queryKey.length === 0 || (queryKey[0] === 'schedule' && (queryKey.length === 1 || queryKey[1] === 'tasks'))) invalidate()
  })
})
