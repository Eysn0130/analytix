import { describe, expect, it } from 'vitest'
import type { ToolResultLocalTodoDataV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { todoCardModel } from './todo-card-model'
type Todo = ToolResultLocalTodoDataV1['todos']['items'][number]
const timestamp = '2026-10-09T00:00:00Z'
const todo = (id: string, content = id, status: Todo['status'] = 'pending', statusReasonCode?: Todo['statusReasonCode']): Todo =>
  ({ id, content, status, createdAt: timestamp, updatedAt: timestamp, ...(statusReasonCode ? { statusReasonCode } : {}) })
const list = (items: Todo[]) => ({ threadId: 'thread', items, updatedAt: timestamp })
const data = (items: Todo[], previous: Todo[] | null | undefined): ToolResultLocalTodoDataV1 => ({ todos: list(items),
  baseline: previous === undefined ? { availability: 'unavailable' } : { availability: 'known', todos: previous === null ? null : list(previous) } })
const t = (key: string, options?: Record<string, unknown>) => key + (options ? JSON.stringify(options) : '')
describe('committed Todo history presentation', () => {
  it('distinguishes a known initial list, initial empty list and unavailable predecessor', () => {
    const initial = todoCardModel(data([todo('a'), todo('b')], null), t)
    expect(initial.items.map(item => item.change?.value)).toEqual(['added', 'added'])
    expect(initial.caption).toBe('dshRecordedTool.todo.initial')
    expect(todoCardModel(data([], null), t).empty).toBe('dshRecordedTool.todo.empty')
    expect(todoCardModel(data([], []), t).caption).toBe('dshRecordedTool.todo.compare')
    const unavailable = todoCardModel(data([todo('a')], undefined), t)
    expect(unavailable.caption).toBe('dshRecordedTool.todo.unavailable')
    expect(unavailable.items[0].change).toBeUndefined(); expect(unavailable.unchanged).toBeUndefined()
    expect(unavailable.expandedSummary).toBeUndefined()
  })
  it('uses IDs for duplicate content and preserves a same-ID rename as one update', () => {
    const model = todoCardModel(data([todo('a', 'New'), todo('b', 'Same'), todo('c', 'Same')], [todo('a', 'Old'), todo('b', 'Same')]), t)
    expect(model.items.map(item => [item.id, item.change?.value])).toEqual([['a', 'updated'], ['c', 'added']])
    expect(model.items[0].fields).toEqual([{ label: 'dshRecordedTool.todo.previous', value: 'Old' }])
    expect(model.unchanged?.items.map(item => item.id)).toEqual(['b'])
    const replacement = todoCardModel(data([todo('b', 'Same')], [todo('a', 'Same')]), t)
    expect(replacement.items.map(item => item.change?.value)).toEqual(['added', 'removed'])
  })
  it('ignores inserted and removed offsets, but detects retained relative reorder', () => {
    const previous = [todo('a'), todo('b'), todo('c')]
    const offsets = todoCardModel(data([todo('x'), todo('a'), todo('c')], previous), t)
    expect(offsets.items.map(item => [item.id, item.change?.value])).toEqual([['x', 'added'], ['b', 'removed']])
    expect(offsets.unchanged?.items.map(item => item.id)).toEqual(['a', 'c'])
    const moved = todoCardModel(data([todo('b'), todo('a', 'Renamed', 'completed'), todo('c')], previous), t)
    expect(moved.items.map(item => [item.id, item.change?.value])).toEqual([['b', 'updated'], ['a', 'updated']])
    expect(moved.items[0].change?.label).toBe('dshRecordedTool.todo.reordered')
    expect(moved.items[1].change?.label).toBe('dshRecordedTool.todo.updated')
    expect(moved.items[1].previousStatus).toBe('dshRecordedTool.todoStatus.pending')
    expect(moved.unchanged?.items[0].id).toBe('c')
  })
  it('keeps five native states, actual reasons and completed-only progress', () => {
    const items = [todo('p'), todo('i', 'Actual active', 'in_progress'), todo('d', 'Done', 'completed'),
      todo('f', 'Failed', 'failed', 'tool_failed'), todo('c', 'Canceled', 'canceled', 'user_canceled')]
    const model = todoCardModel(data(items, null), t)
    expect(model.items.map(item => item.status?.value)).toEqual(['pending', 'in_progress', 'completed', 'failed', 'canceled'])
    expect(model.summary).toContain('{"done":1,"total":5}'); expect(model.summary).toContain('Actual active')
    expect(model.items[3].subtitle).toBe('dshRecordedTool.todoReason.tool_failed')
    const reason = todoCardModel(data([todo('f', 'Failed', 'failed', 'timeout')], [items[3]]), t)
    expect(reason.items).toHaveLength(1); expect(reason.items[0].change?.value).toBe('updated')
    expect(reason.items[0].subtitle).toBe('dshRecordedTool.todoReason.timeout')
  })
  it('does not confuse timestamp changes with semantic history changes', () => {
    const previous = todo('a'), current = { ...previous, updatedAt: '2026-10-10T00:00:00Z' }
    const model = todoCardModel(data([current], [previous]), t)
    expect(model.items).toHaveLength(0); expect(model.unchanged?.items).toHaveLength(1)
    expect(model.expandedSummary).toBe('dshRecordedTool.todo.noChanges')
  })
})

describe('parallel committed Todo summary', () => {
  it('uses the first committed active item and counts the additional active items', () => {
    const items = [todo('done', 'Done', 'completed'), todo('queued'), todo('first', 'First active', 'in_progress'),
      todo('second', 'Second active', 'in_progress'), todo('third', 'Third active', 'in_progress')]
    for (const previous of [null, undefined, [todo('old', 'Stale active', 'in_progress')]]) {
      const model = todoCardModel(data(items, previous), t)
      expect(model.summary).toBe('dshRecordedTool.todo.completed{"done":1,"total":5} · First active +2')
      expect(model.summary).not.toContain('Stale active')
    }
    expect(todoCardModel(data([items[2]], null), t).summary).toBe('dshRecordedTool.todo.completed{"done":0,"total":1} · First active')
    expect(todoCardModel(data([items[0], items[1]], null), t).summary).toBe('dshRecordedTool.todo.completed{"done":1,"total":2}')
  })
})
