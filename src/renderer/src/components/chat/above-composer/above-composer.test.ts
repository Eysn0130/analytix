import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it } from 'vitest'
import i18n from '../../../i18n'
import {
  threadHandoffStepsWithRollback,
  type ThreadHandoffOperation,
  type ThreadHandoffStepStatus
} from '@shared/thread-handoff'
import { AboveComposerPanelStack } from './AboveComposerPanel'
import { AboveComposerPanelRow } from './AboveComposerPanelRow'
import { ComposerGoalRow } from './ComposerGoalRow'
import { ComposerQueuedMessageList } from './ComposerQueuedMessageList'
import {
  buildTodoPlanPillState,
  ComposerInProgressStatusPill,
  todoPlanBelongsToCurrentTurn
} from './ComposerInProgressStatusPill'
import { ProgressStepRow } from './ProgressStepRow'
import { ThreadHandoffProgressModalContent } from './ThreadHandoffProgressModal'
import type { ThreadTodoList, ThreadTodoStatus } from '../../../agent/types'
import type { CurrentTurnDiffSummary } from '../../../lib/composer-change-summary'

function todoList(items: Array<[string, ThreadTodoStatus]>): ThreadTodoList {
  return {
    threadId: 'thr_todo',
    turnId: 'turn_1',
    updatedAt: '2026-06-29T00:00:00.000Z',
    items: items.map(([content, status], index) => ({
      id: `todo_${index}`,
      content,
      status,
      createdAt: '2026-06-29T00:00:00.000Z',
      updatedAt: '2026-06-29T00:00:00.000Z'
    }))
  }
}

function diffSummary(
  fileCount: number,
  added: number,
  removed: number
): CurrentTurnDiffSummary {
  return {
    fileCount,
    added,
    removed,
    files: Array.from({ length: fileCount }, (_, index) => ({
      path: `src/file-${index}.ts`,
      added: index === 0 ? added : 0,
      removed: index === 0 ? removed : 0
    }))
  }
}

describe('above composer panel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('does not render a placeholder when there are no rows', () => {
    const html = renderToStaticMarkup(createElement(AboveComposerPanelStack, null, null, false))

    expect(html).toBe('')
  })

  it('keeps row order stable', () => {
    const html = renderToStaticMarkup(
      createElement(
        AboveComposerPanelStack,
        null,
        createElement(AboveComposerPanelRow, null, 'queue row'),
        createElement(AboveComposerPanelRow, null, 'goal row'),
        createElement(AboveComposerPanelRow, null, 'handoff row')
      )
    )

    expect(html).toContain('data-above-composer-portal="true"')
    expect(html).toContain('w-[90%]')
    expect(html.indexOf('queue row')).toBeLessThan(html.indexOf('goal row'))
    expect(html.indexOf('goal row')).toBeLessThan(html.indexOf('handoff row'))
  })

  it('renders queued messages before the active goal when both rows exist', () => {
    const html = renderToStaticMarkup(
      createElement(
        AboveComposerPanelStack,
        null,
        createElement(ComposerQueuedMessageList, {
          messages: [{ id: 'q-1', text: 'queued follow-up', mode: 'agent' }],
          onRemove: () => undefined
        }),
        createElement(ComposerGoalRow, {
          goal: {
            threadId: 'thr_1',
            objective: 'Ship the top tray',
            status: 'active',
            tokensUsed: 0,
            timeUsedSeconds: 61,
            createdAt: '2026-06-29T00:00:00.000Z',
            updatedAt: '2026-06-29T00:01:00.000Z'
          },
          elapsedLabel: '1m 1s',
          onEdit: () => undefined,
          onToggleStatus: () => undefined,
          onClear: () => undefined
        })
      )
    )

    expect(html.indexOf('queued follow-up')).toBeGreaterThanOrEqual(0)
    expect(html.indexOf('Ship the top tray')).toBeGreaterThanOrEqual(0)
    expect(html.indexOf('queued follow-up')).toBeLessThan(html.indexOf('Ship the top tray'))
  })
})

describe('ProgressStepRow', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it.each([
    ['running', 'Running'],
    ['done', 'Done'],
    ['failed', 'Failed'],
    ['pending', 'Pending']
  ] as Array<[ThreadHandoffStepStatus, string]>)('renders %s with an sr-only status label', (status, label) => {
    const html = renderToStaticMarkup(
      createElement(ProgressStepRow, { status, children: 'Apply changes' })
    )

    expect(html).toContain('sr-only')
    expect(html).toContain(label)
    expect(html).toContain('Apply changes')
  })
})

describe('ComposerInProgressStatusPill', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('chooses the first in-progress step before pending steps', () => {
    const state = buildTodoPlanPillState(todoList([
      ['Read context', 'pending'],
      ['Implement pill', 'in_progress'],
      ['Verify behavior', 'pending']
    ]))

    expect(state?.stepNumber).toBe(2)
    expect(state?.stepCount).toBe(3)
    expect(state?.completedPercent).toBe(0)
  })

  it('chooses the first pending step when no step is in progress', () => {
    const state = buildTodoPlanPillState(todoList([
      ['Read context', 'completed'],
      ['Implement pill', 'pending'],
      ['Verify behavior', 'pending']
    ]))

    expect(state?.stepNumber).toBe(2)
    expect(state?.completedPercent).toBeCloseTo(100 / 3)
  })

  it('chooses the last step when all steps are completed', () => {
    const state = buildTodoPlanPillState(todoList([
      ['Read context', 'completed'],
      ['Implement pill', 'completed'],
      ['Verify behavior', 'completed']
    ]))

    expect(state?.stepNumber).toBe(3)
    expect(state?.completedPercent).toBe(100)
  })

  it('does not render while the thread is idle or missing a live turn', () => {
    const todos = todoList([
      ['Read context', 'in_progress']
    ])

    expect(renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos,
        diffSummary: null,
        busy: false,
        currentTurnId: 'turn_1'
      })
    )).toBe('')
    expect(renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos,
        diffSummary: null,
        busy: true,
        currentTurnId: null
      })
    )).toBe('')
  })

  it('does not render without active todo items', () => {
    expect(renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: null,
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_1'
      })
    )).toBe('')
    expect(renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: { threadId: 'thr_todo', items: [], updatedAt: '2026-06-29T00:00:00.000Z' },
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_1'
      })
    )).toBe('')
  })

  it('does not render ownerless todos even when they were updated recently', () => {
    const ownerlessTodos = {
      ...todoList([['Old plan item', 'in_progress']]),
      turnId: undefined,
      updatedAt: '2026-06-29T00:02:00.000Z'
    }

    expect(todoPlanBelongsToCurrentTurn(ownerlessTodos, 'turn_2')).toBe(false)
    expect(renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: ownerlessTodos,
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_2'
      })
    )).toBe('')
  })

  it('renders current turn todos by turnId without a local turn start timestamp', () => {
    const currentTodos = {
      ...todoList([['Restored running plan item', 'in_progress']]),
      turnId: 'turn_current',
      updatedAt: '2026-06-29T00:00:00.000Z'
    }

    expect(todoPlanBelongsToCurrentTurn(currentTodos, 'turn_current')).toBe(true)
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: currentTodos,
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_current'
      })
    )

    expect(html).toContain('Step 1 / 1')
    expect(html).toContain('Restored running plan item')
  })

  it('does not render todos from a different turn even when they are recent', () => {
    const otherTurnTodos = {
      ...todoList([['Other turn plan item', 'in_progress']]),
      turnId: 'turn_other',
      updatedAt: '2026-06-29T00:02:00.000Z'
    }

    expect(todoPlanBelongsToCurrentTurn(otherTurnTodos, 'turn_current')).toBe(false)
    expect(renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: otherTurnTodos,
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_current'
      })
    )).toBe('')
  })

  it('does not render while a blocking request is pending', () => {
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: todoList([['Read context', 'in_progress']]),
        diffSummary: diffSummary(5, 348, 0),
        busy: true,
        currentTurnId: 'turn_1',
        hasBlockingRequest: true
      })
    )

    expect(html).toBe('')
  })

  it('renders every step in the hover tooltip', () => {
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: todoList([
          ['Read Codex source', 'completed'],
          ['Map active todos', 'in_progress'],
          ['Verify tooltip content', 'pending']
        ]),
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_1'
      })
    )

    expect(html).toContain('Step 2 / 3')
    expect(html).toContain('Read Codex source')
    expect(html).toContain('Map active todos')
    expect(html).toContain('Verify tooltip content')
    const visibleText = html.replace(/<[^>]*>/g, '')
    expect(visibleText).not.toContain('1.')
    expect(html).toContain('w-max')
    expect(html).toContain('max-w-[min(20rem,calc(100vw-16px))]')
    expect(html).toContain("before:-bottom-2")
    expect(html).not.toContain('w-[min(24rem')
    expect(html).not.toContain('min-w-56')
  })

  it('renders todo-only Chinese progress text', async () => {
    await i18n.changeLanguage('zh')
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: todoList([
          ['阅读 Codex 源码', 'in_progress'],
          ['映射 todo 状态', 'pending'],
          ['实现 pill', 'pending'],
          ['验证视觉', 'pending']
        ]),
        diffSummary: null,
        busy: true,
        currentTurnId: 'turn_1'
      })
    )

    expect(html).toContain('第 1 / 4 步')
  })

  it('renders diff-only status without a leading separator', async () => {
    await i18n.changeLanguage('zh')
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: null,
        diffSummary: diffSummary(5, 348, 0),
        busy: true,
        currentTurnId: 'turn_1'
      })
    )

    expect(html).toContain('5 个文件已更改')
    expect(html).toContain('+348')
    expect(html).toContain('-0')
    expect(html).not.toContain('·')
  })

  it('can hide the inline diff while keeping the current todo plan visible', async () => {
    await i18n.changeLanguage('zh')
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: todoList([
          ['步骤一', 'completed'],
          ['步骤二', 'in_progress']
        ]),
        diffSummary: diffSummary(5, 348, 0),
        showDiffSummary: false,
        busy: true,
        currentTurnId: 'turn_1'
      })
    )

    expect(html).toContain('第 2 / 2 步')
    expect(html).toContain('步骤二')
    expect(html).not.toContain('5 个文件已更改')
    expect(html).not.toContain('+348')
    expect(html).not.toContain('·')
  })

  it('does not render a diff-only pill when the inline diff gate is disabled', async () => {
    await i18n.changeLanguage('zh')
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: null,
        diffSummary: diffSummary(5, 348, 0),
        showDiffSummary: false,
        busy: true,
        currentTurnId: 'turn_1'
      })
    )

    expect(html).toBe('')
  })

  it('renders todo and diff status in one shared capsule', async () => {
    await i18n.changeLanguage('zh')
    const html = renderToStaticMarkup(
      createElement(ComposerInProgressStatusPill, {
        todos: todoList([
          ['步骤一', 'completed'],
          ['步骤二', 'completed'],
          ['步骤三', 'completed'],
          ['步骤四', 'completed'],
          ['步骤五', 'in_progress']
        ]),
        diffSummary: diffSummary(5, 348, 0),
        busy: true,
        currentTurnId: 'turn_1'
      })
    )

    expect(html).toContain('第 5 / 5 步')
    expect(html).toContain('·')
    expect(html).toContain('5 个文件已更改')
    expect(html).toContain('+348')
    expect(html).toContain('-0')
    expect(html.indexOf('第 5 / 5 步')).toBeLessThan(html.indexOf('5 个文件已更改'))
  })
})

describe('thread handoff step helpers', () => {
  it('shows a running rollback row after a failed step that has remaining work', () => {
    expect(threadHandoffStepsWithRollback([
      { id: 'stash-source-changes', status: 'done' },
      { id: 'apply-changes-to-worktree', status: 'failed' },
      { id: 'switching-thread', status: 'pending' }
    ])).toEqual([
      { id: 'stash-source-changes', status: 'done' },
      { id: 'apply-changes-to-worktree', status: 'failed' },
      { id: 'rolling-back-changes', status: 'running' }
    ])
  })
})

describe('ThreadHandoffProgressModal', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('renders terminal handoff errors with command output and recovery actions', () => {
    const operation: ThreadHandoffOperation = {
      id: 'handoff-error',
      direction: 'to-worktree',
      status: 'error',
      sourceThreadId: 'thr_1',
      targetThreadId: null,
      sourceWorkspace: '/tmp/project',
      targetWorkspace: '/tmp/project/.analytix-worktrees/0',
      sourceBranch: 'main',
      localBranch: 'main',
      worktreeBranch: 'analytix/handoff',
      request: {
        direction: 'to-worktree',
        sourceThreadId: 'thr_1',
        sourceWorkspace: '/tmp/project',
        localBranch: 'main'
      },
      steps: [
        { id: 'create-new-worktree', status: 'done' },
        { id: 'apply-changes-to-worktree', status: 'failed' },
        { id: 'switching-thread', status: 'pending' }
      ],
      errorMessage: 'renderer switch failed',
      warningMessage: null,
      execOutput: {
        command: 'thread-handoff',
        output: 'workspace update rejected'
      },
      hasUnseenTerminalState: true,
      worktree: null,
      createdAt: '2026-06-30T00:00:00.000Z',
      updatedAt: '2026-06-30T00:01:00.000Z'
    }

    const html = renderToStaticMarkup(createElement(ThreadHandoffProgressModalContent, {
      operation,
      onClose: () => undefined,
      onRetry: () => undefined
    }))

    expect(html).toContain('Thread move failed')
    expect(html).toContain('renderer switch failed')
    expect(html).toContain('$ thread-handoff')
    expect(html).toContain('workspace update rejected')
    expect(html).toContain('Close thread handoff status')
    expect(html).toContain('Retry handoff')
  })
})
