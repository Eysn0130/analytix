// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1, type ScheduledTaskV1 } from '@shared/app-settings'
import type { ScheduleTaskListResult } from '@shared/schedule-task-mutation'
import { ScheduleTaskCatalog, scheduleTaskCatalog, selectScheduleTask, useCurrentScheduleTask } from './schedule-task-catalog'
import { ScheduleCreateCard } from './ScheduleCreateCard'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}
function task(id: string): ScheduledTaskV1 {
  return normalizeAppSettings({ schedule: { tasks: [{ id, title: id, prompt: 'Synthetic prompt' }] } } as AppSettingsV1).schedule.tasks[0]
}
async function flush() { for (let i = 0; i < 6; i++) await Promise.resolve() }
function fixture(read: () => Promise<ScheduleTaskListResult>) {
  let invalidate!: () => void
  const remove = vi.fn()
  const catalog = new ScheduleTaskCatalog({ read, listen: callback => { invalidate = callback; return remove } })
  return { catalog, invalidate: () => invalidate(), remove }
}

describe('causal shared Host task catalog', () => {
  it('does not treat an earlier positive catalog as a current answer', () => {
    const old = { tasks: [task('task')], phase: 'ready' as const, readRequest: 7, readSettled: 7 }
    expect(selectScheduleTask(old, 'task', 7).kind).toBe('pending')
    expect(selectScheduleTask({ ...old, phase: 'loading', readRequest: 8 }, 'task', 7).kind).toBe('pending')
    expect(selectScheduleTask({ ...old, readRequest: 8, readSettled: 8 }, 'task', 7).kind).toBe('found')
  })
  it('shares one read across a group of mounted consumers', async () => {
    const read = vi.fn(async (): Promise<ScheduleTaskListResult> => ({ kind: 'success', tasks: [task('host-task')] }))
    const { catalog, remove } = fixture(read)
    const a = catalog.subscribe(vi.fn()); const b = catalog.subscribe(vi.fn())
    await flush()
    expect(read).toHaveBeenCalledTimes(1)
    expect(catalog.getSnapshot()).toMatchObject({ phase: 'ready', readRequest: 1, readSettled: 1 })
    a(); b(); expect(remove).toHaveBeenCalledTimes(1)
  })

  it('cannot confirm creation-time absence from an earlier catalog or an earlier issued read', async () => {
    const old = deferred<ScheduleTaskListResult>(); const fresh = deferred<ScheduleTaskListResult>()
    const read = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const { catalog } = fixture(read)
    const a = catalog.subscribe(vi.fn()); await flush()
    const mount = catalog.getSnapshot().readRequest
    const b = catalog.subscribe(vi.fn()); await flush()
    old.resolve({ kind: 'success', tasks: [] }); await flush()
    expect(selectScheduleTask(catalog.getSnapshot(), 'new-task', mount).kind).toBe('pending')
    fresh.resolve({ kind: 'success', tasks: [task('new-task')] }); await flush()
    expect(selectScheduleTask(catalog.getSnapshot(), 'new-task', mount)).toMatchObject({ kind: 'found', task: { id: 'new-task' } })
    a(); b()
  })

  it('requires a successful post-mount read before declaring a task deleted', async () => {
    const read = vi.fn(async (): Promise<ScheduleTaskListResult> => ({ kind: 'success', tasks: [] }))
    const { catalog } = fixture(read)
    await catalog.refresh()
    const mount = catalog.getSnapshot().readRequest
    expect(selectScheduleTask(catalog.getSnapshot(), 'missing', mount).kind).toBe('pending')
    const unsubscribe = catalog.subscribe(vi.fn()); await flush()
    expect(selectScheduleTask(catalog.getSnapshot(), 'missing', mount).kind).toBe('missing')
    unsubscribe()
  })

  it('keeps records on failed/unavailable reads and retries against Host', async () => {
    const read = vi.fn<() => Promise<ScheduleTaskListResult>>()
      .mockResolvedValueOnce({ kind: 'success', tasks: [task('task')] })
      .mockResolvedValueOnce({ kind: 'unavailable', message: 'No Host' })
      .mockResolvedValueOnce({ kind: 'success', tasks: [] })
    const { catalog, invalidate } = fixture(read)
    const unsubscribe = catalog.subscribe(vi.fn()); await flush()
    const settled = catalog.getSnapshot().readSettled
    invalidate(); await flush()
    expect(catalog.getSnapshot()).toMatchObject({ phase: 'unavailable', readSettled: settled, tasks: [{ id: 'task' }] })
    expect(selectScheduleTask(catalog.getSnapshot(), 'other', 0).kind).toBe('pending')
    await catalog.refresh()
    expect(catalog.getSnapshot()).toMatchObject({ phase: 'ready', tasks: [], readSettled: 3 })
    unsubscribe()
  })

  it('ignores late responses after invalidation, deletion and A-to-B-to-A consumer lifecycles', async () => {
    const oldA = deferred<ScheduleTaskListResult>(); const oldB = deferred<ScheduleTaskListResult>(); const newA = deferred<ScheduleTaskListResult>()
    const read = vi.fn().mockReturnValueOnce(oldA.promise).mockReturnValueOnce(oldB.promise).mockReturnValueOnce(newA.promise)
    const { catalog } = fixture(read)
    const a = catalog.subscribe(vi.fn()); await flush(); a()
    const b = catalog.subscribe(vi.fn()); await flush(); b()
    const mount = catalog.getSnapshot().readRequest
    const a2 = catalog.subscribe(vi.fn()); await flush()
    newA.resolve({ kind: 'success', tasks: [] }); await flush()
    oldB.resolve({ kind: 'success', tasks: [task('foreign-thread-task')] }); oldA.resolve({ kind: 'success', tasks: [task('deleted-task')] }); await flush()
    expect(catalog.getSnapshot().tasks).toEqual([])
    expect(selectScheduleTask(catalog.getSnapshot(), 'deleted-task', mount).kind).toBe('missing')
    a2()
  })
})

it.each(['error', 'unavailable'] as const)('keeps the mounted deleted card closed through loading and %s', async kind => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  const fresh = deferred<ScheduleTaskListResult>(), failed = deferred<ScheduleTaskListResult>()
  const read = vi.fn().mockResolvedValueOnce({ kind: 'success', tasks: [task('task')] })
    .mockReturnValueOnce(fresh.promise).mockReturnValueOnce(failed.promise)
  Object.assign(window, { analytix: { schedule: { listTasks: read } } })
  await scheduleTaskCatalog.refresh()
  function Card() {
    const { task: current } = useCurrentScheduleTask('task')
    return createElement(ScheduleCreateCard, {
      creation: { taskId: 'task', title: 'Creation', frequency: 'Created frequency' },
      currentTask: current == null ? current : { title: current.title, frequency: 'Current frequency' },
      labels: { open: 'Open', deleted: 'Deleted', openLabel: title => 'Open ' + title }, openTaskDetail: vi.fn()
    })
  }
  const host = document.createElement('div'), root = createRoot(host)
  try {
    await act(async () => { root.render(createElement(Card)); await flush() })
    expect(host.textContent).toContain('Creation')
    expect(host.textContent).not.toContain('Current frequency')
    await act(async () => { fresh.resolve({ kind: 'success', tasks: [] }); await flush() })
    expect(host.textContent).toContain('Deleted'); expect(host.querySelectorAll('button')).toHaveLength(0)
    await act(async () => { scheduleTaskCatalog.invalidate(); await flush() })
    expect(scheduleTaskCatalog.getSnapshot().phase).toBe('loading')
    expect(host.textContent).toContain('Deleted'); expect(host.querySelectorAll('button')).toHaveLength(0)
    await act(async () => { failed.resolve({ kind, message: 'Synthetic failure' }); await flush() })
    expect(scheduleTaskCatalog.getSnapshot().phase).toBe(kind)
    expect(host.textContent).toContain('Deleted'); expect(host.querySelectorAll('button')).toHaveLength(0)
  } finally { await act(async () => root.unmount()) }
})
