/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolResultLocalTodoDataV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ToolDetailItem, ToolDetailsModel } from './details-card-model'
import type { RecordedToolTranslate } from './recorded-tool-card-model'
type Todo = ToolResultLocalTodoDataV1['todos']['items'][number]
/** Diff committed snapshots by stable native identity, never by duplicated content. */
export function todoCardModel(data: ToolResultLocalTodoDataV1, t: RecordedToolTranslate): ToolDetailsModel {
  const todos = data.todos.items
  const done = todos.filter(item => item.status === 'completed').length
  const active = todos.filter(item => item.status === 'in_progress')
  const head = t('dshRecordedTool.todo.completed', { done, total: todos.length })
  const summary = active.length ? head + ' · ' + active[0].content + (active.length > 1 ? ' +' + (active.length - 1) : '') : head
  const item = (todo: Todo): ToolDetailItem => ({ id: todo.id, title: todo.content, fields: [],
    status: { value: todo.status, label: t('dshRecordedTool.todoStatus.' + todo.status) },
    ...(todo.statusReasonCode ? { subtitle: t('dshRecordedTool.todoReason.' + todo.statusReasonCode) } : {}) })
  const empty = t('dshRecordedTool.todo.empty')
  if (data.baseline.availability === 'unavailable') return { summary, items: todos.map(item), empty, caption: t('dshRecordedTool.todo.unavailable') }
  const initial = data.baseline.todos === null
  const previous = data.baseline.todos?.items ?? []
  const before = new Map(previous.map(todo => [todo.id, todo]))
  const current = new Map(todos.map(todo => [todo.id, todo]))
  // Relative retained positions ignore simple insertion/removal offsets.
  const oldOrder = previous.filter(todo => current.has(todo.id)).map(todo => todo.id)
  const newOrder = todos.filter(todo => before.has(todo.id)).map(todo => todo.id)
  const unchanged: ToolDetailItem[] = [], changed: ToolDetailItem[] = []
  let added = 0, removed = 0, updated = 0
  for (const todo of todos) {
    const old = before.get(todo.id), detail = item(todo)
    if (!old) { added++; detail.change = { value: 'added', label: t('dshRecordedTool.todo.added') }; changed.push(detail) }
    else if (old.content !== todo.content || old.status !== todo.status || old.statusReasonCode !== todo.statusReasonCode ||
      oldOrder.indexOf(todo.id) !== newOrder.indexOf(todo.id)) {
      const semanticChange = old.content !== todo.content || old.status !== todo.status || old.statusReasonCode !== todo.statusReasonCode
      updated++; detail.change = { value: 'updated', label: t('dshRecordedTool.todo.' + (semanticChange ? 'updated' : 'reordered')) }
      if (old.status !== todo.status) detail.previousStatus = t('dshRecordedTool.todoStatus.' + old.status)
      if (old.content !== todo.content) detail.fields = [{ label: t('dshRecordedTool.todo.previous'), value: old.content }]
      changed.push(detail)
    } else unchanged.push(detail)
  }
  for (const todo of previous) if (!current.has(todo.id)) {
    removed++; changed.push({ ...item(todo), change: { value: 'removed', label: t('dshRecordedTool.todo.removed') } })
  }
  const changes = added + removed + updated
  return { summary, expandedSummary: changes ? t('dshRecordedTool.todo.changes', { added, removed, updated }) : t('dshRecordedTool.todo.noChanges'),
    items: changed, empty: initial ? empty : changes ? empty : t('dshRecordedTool.todo.noChanges'),
    caption: t('dshRecordedTool.todo.' + (initial ? 'initial' : 'compare')),
    ...(unchanged.length ? { unchanged: { label: t('dshRecordedTool.todo.unchanged', { count: unchanged.length }), items: unchanged } } : {}) }
}
