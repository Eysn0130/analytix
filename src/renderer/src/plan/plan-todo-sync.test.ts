import { describe, expect, it } from 'vitest'
import type { ThreadTodoList } from '../agent/types'
import {
  extractPlanTodos,
  mergePlanTodosForRenderer,
  threadTodoWriteItems
} from './plan-todo-sync'

describe('plan todo lifecycle synchronization', () => {
  it('retains failed audit state across plan refresh and write projection', () => {
    const now = '2026-07-22T00:00:00.000Z'
    const planItems = extractPlanTodos({
      markdown: '- [ ] Verify evidence',
      threadId: 'thr_1',
      planId: 'plan_1',
      relativePath: '.analytix/plan/verify.md',
      now
    })
    const existing: ThreadTodoList = {
      threadId: 'thr_1',
      updatedAt: now,
      items: [{
        ...planItems[0],
        id: 'todo_existing',
        status: 'failed',
        statusReasonCode: 'verification_failed',
        note: 'Evidence receipt did not match the claim.'
      }]
    }

    const merged = mergePlanTodosForRenderer({
      threadId: 'thr_1',
      existing,
      planItems,
      now: '2026-07-22T00:01:00.000Z'
    })

    expect(merged.items).toHaveLength(1)
    expect(merged.items[0]).toMatchObject({
      id: 'todo_existing',
      status: 'failed',
      statusReasonCode: 'verification_failed',
      note: 'Evidence receipt did not match the claim.'
    })
    expect(threadTodoWriteItems(merged)).toEqual([
      expect.objectContaining({
        id: 'todo_existing',
        status: 'failed',
        statusReasonCode: 'verification_failed',
        note: 'Evidence receipt did not match the claim.'
      })
    ])
  })
})
