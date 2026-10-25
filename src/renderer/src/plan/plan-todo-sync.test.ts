import { describe, expect, it } from 'vitest'
import type { ThreadTodoList } from '../agent/types'
import {
  extractPlanTodos,
  mergePlanTodosForRenderer,
  threadTodoWriteItems
} from './plan-todo-sync'

describe('plan todo lifecycle synchronization', () => {
  it.each([
    ['same plan', 'plan_1', '.analytix/plan/verify.md'],
    ['same path', 'plan_2', '.analytix/plan/verify.md'],
    ['content only', 'plan_2', '.analytix/plan/moved.md']
  ] as const)('retains both active todos through %s matching and normalized content', (_match, planId, relativePath) => {
    const now = '2026-07-22T00:00:00.000Z'
    const input = { threadId: 'thr_1', planId: 'plan_1', relativePath: '.analytix/plan/verify.md', now }
    const existing: ThreadTodoList = {
      threadId: input.threadId,
      updatedAt: now,
      items: extractPlanTodos({ ...input, markdown: '# Plan\n- [ ] Read context\n- [ ] Implement closure' })
        .map((item, index) => ({ ...item, id: `todo_existing_${index}`, status: 'in_progress' }))
    }
    const planItems = extractPlanTodos({
      ...input,
      planId,
      relativePath,
      markdown: '# Renamed plan\n- [ ] READ   context\n- [ ] implement closure'
    })

    const merged = mergePlanTodosForRenderer({ ...input, existing, planItems, now: '2026-07-22T00:01:00.000Z' })

    expect(threadTodoWriteItems(merged)).toEqual([
      expect.objectContaining({ id: 'todo_existing_0', content: 'READ context', status: 'in_progress' }),
      expect.objectContaining({ id: 'todo_existing_1', content: 'implement closure', status: 'in_progress' })
    ])
    expect(merged.items.map((item) => item.createdAt)).toEqual([now, now])
    expect(existing.items.map((item) => item.status)).toEqual(['in_progress', 'in_progress'])
  })

  it('completes the checked todo while retaining the other active todo', () => {
    const now = '2026-07-22T00:00:00.000Z'
    const input = { threadId: 'thr_1', planId: 'plan_1', relativePath: '.analytix/plan/verify.md', now }
    const existing: ThreadTodoList = {
      threadId: input.threadId,
      updatedAt: now,
      items: extractPlanTodos({ ...input, markdown: '- [ ] Read context\n- [ ] Implement closure' })
        .map((item) => ({ ...item, status: 'in_progress' }))
    }
    const planItems = extractPlanTodos({ ...input, markdown: '- [X] Read context\n- [ ] Implement closure' })

    const merged = mergePlanTodosForRenderer({ ...input, existing, planItems, now: '2026-07-22T00:01:00.000Z' })

    expect(threadTodoWriteItems(merged)).toEqual([
      expect.objectContaining({ id: existing.items[0].id, status: 'completed' }),
      expect.objectContaining({ id: existing.items[1].id, status: 'in_progress' })
    ])
    expect(merged.items[0].updatedAt).toBe('2026-07-22T00:01:00.000Z')
    expect(merged.items[1].updatedAt).toBe(now)
  })

  it('does not inherit active status from ordinal fallback when the content changes', () => {
    const now = '2026-07-22T00:00:00.000Z'
    const input = { threadId: 'thr_1', planId: 'plan_1', relativePath: '.analytix/plan/verify.md', now }
    const existing: ThreadTodoList = {
      threadId: input.threadId,
      updatedAt: now,
      items: extractPlanTodos({ ...input, markdown: '- [ ] Read context' })
        .map((item) => ({ ...item, id: 'todo_existing', status: 'in_progress' }))
    }
    const planItems = extractPlanTodos({ ...input, markdown: '- [ ] Implement replacement' })

    const merged = mergePlanTodosForRenderer({ ...input, existing, planItems, now: '2026-07-22T00:01:00.000Z' })

    expect(threadTodoWriteItems(merged)).toEqual([
      expect.objectContaining({ id: 'todo_existing', content: 'Implement replacement', status: 'pending' })
    ])
    expect(merged.items[0].source).toEqual(planItems[0].source)
    expect(merged.items[0].updatedAt).toBe('2026-07-22T00:01:00.000Z')
  })

  it.each([
    ['completed', ' '],
    ['completed', 'x'],
    ['failed', ' '],
    ['failed', 'x'],
    ['canceled', ' '],
    ['canceled', 'x']
  ] as const)('retains %s audit state with checkbox [%s] across refresh and write projection', (status, marker) => {
    const now = '2026-07-22T00:00:00.000Z'
    const planItems = extractPlanTodos({
      markdown: `- [${marker}] Verify evidence`,
      threadId: 'thr_1',
      planId: 'plan_1',
      relativePath: '.analytix/plan/verify.md',
      now
    })
    const audit = {
      ...(status === 'failed' ? { statusReasonCode: 'verification_failed' as const } : {}),
      ...(status === 'canceled' ? { statusReasonCode: 'user_canceled' as const } : {}),
      note: 'Evidence receipt did not match the claim.'
    }
    const existing: ThreadTodoList = {
      threadId: 'thr_1',
      updatedAt: now,
      items: [{
        ...planItems[0],
        id: 'todo_existing',
        status,
        ...audit
      }]
    }

    const merged = mergePlanTodosForRenderer({
      threadId: 'thr_1',
      existing,
      planItems,
      now: '2026-07-22T00:01:00.000Z'
    })

    expect(merged.items).toHaveLength(1)
    expect(merged.items[0]).toMatchObject({ id: 'todo_existing', status, ...audit, updatedAt: now })
    expect(threadTodoWriteItems(merged)).toEqual([
      expect.objectContaining({ id: 'todo_existing', status, ...audit })
    ])
  })
})
