import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import type { ThreadTodoList } from '../../agent/types'

const storeMock = vi.hoisted(() => ({
  state: {
    activeThreadTodos: null as ThreadTodoList | null,
    setActiveThreadTodoStatus: vi.fn(),
    clearActiveThreadTodos: vi.fn()
  }
}))

vi.mock('../../store/chat-store', () => ({
  useChatStore: (selector: (state: typeof storeMock.state) => unknown) => selector(storeMock.state)
}))

import { TodoPanel } from './TodoPanel'

function todoList(): ThreadTodoList {
  return {
    threadId: 'thr_todo',
    turnId: 'turn_todo',
    updatedAt: '2026-06-04T00:02:00.000Z',
    items: [
      {
        id: 'todo_1',
        content: 'Read current runtime contracts',
        status: 'completed',
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:01:00.000Z'
      },
      {
        id: 'todo_2',
        content: 'Close session todo UI evidence',
        status: 'in_progress',
        source: {
          kind: 'plan',
          planId: 'plan_1',
          relativePath: 'docs/plan.md',
          ordinal: 2,
          contentHash: 'hash_2'
        },
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:01:00.000Z'
      },
      {
        id: 'todo_3',
        content: 'Run regression',
        status: 'pending',
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:01:00.000Z'
      }
    ]
  }
}

describe('TodoPanel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
    storeMock.state.activeThreadTodos = null
    storeMock.state.setActiveThreadTodoStatus.mockReset()
    storeMock.state.clearActiveThreadTodos.mockReset()
  })

  it('renders active thread todos with status counts and plan source link', () => {
    storeMock.state.activeThreadTodos = todoList()

    const html = renderToStaticMarkup(createElement(TodoPanel, {
      onCollapse: () => undefined,
      onOpenPlan: () => undefined
    }))

    expect(html).toContain('Read current runtime contracts')
    expect(html).toContain('Close session todo UI evidence')
    expect(html).toContain('Run regression')
    expect(html).toContain('docs/plan.md')
    expect(html).toContain('Pending')
    expect(html).toContain('Active')
    expect(html).toContain('Done')
    expect(html).toContain('aria-label="Clear todos"')
  })

  it('does not render child todo projections as parent session todos', () => {
    storeMock.state.activeThreadTodos = {
      ...todoList(),
      childTodoProjections: [
        {
          id: 'projection_1',
          projectedItems: [
            { id: 'child_1', content: 'Child-only projected todo', status: 'completed' }
          ]
        }
      ]
    } as ThreadTodoList

    const html = renderToStaticMarkup(createElement(TodoPanel, {
      onCollapse: () => undefined,
      onOpenPlan: () => undefined
    }))

    expect(html).toContain('Read current runtime contracts')
    expect(html).not.toContain('Child-only projected todo')
  })

  it('renders retained terminal reasons and disables destructive clear', () => {
    const list = todoList()
    list.items.push(
      {
        id: 'todo_failed',
        content: 'Validate evidence',
        status: 'failed',
        statusReasonCode: 'verification_failed',
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:01:00.000Z'
      },
      {
        id: 'todo_canceled',
        content: 'Publish canceled artifact',
        status: 'canceled',
        statusReasonCode: 'user_canceled',
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:01:00.000Z'
      }
    )
    storeMock.state.activeThreadTodos = list

    const html = renderToStaticMarkup(createElement(TodoPanel, {
      onCollapse: () => undefined,
      onOpenPlan: () => undefined
    }))

    expect(html).toContain('Validate evidence')
    expect(html).toContain('verification_failed')
    expect(html).toContain('Publish canceled artifact')
    expect(html).toContain('user_canceled')
    expect(html).toContain('Failed')
    expect(html).toContain('Canceled')
    expect(html).toContain('title="Failed or canceled todos are retained for audit."')
    expect(html).toContain('disabled=""')
  })

  it('renders an empty session todo dock without destructive controls', () => {
    const html = renderToStaticMarkup(createElement(TodoPanel, {
      onCollapse: () => undefined,
      onOpenPlan: () => undefined
    }))

    expect(html).toContain('No todos yet')
    expect(html).not.toContain('aria-label="Clear todos"')
  })
})
