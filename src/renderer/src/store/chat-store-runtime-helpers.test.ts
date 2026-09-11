import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../agent/types'
import type { NormalizedThread } from '../agent/types'
import type { ChatState } from './chat-store-types'
import {
  findReusableEmptyThreadId,
  hasPendingRuntimeWork,
  settlePendingRuntimeWorkAfterInterrupt,
  threadHasPendingRuntimeWork,
  threadSnapshotLooksRunning,
  upsertUserBlock
} from './chat-store-runtime-helpers'

describe('chat-store-runtime-helpers compaction state', () => {
  it('only marks reconciled user blocks as steering accepted when they belong to a steer admission', () => {
    const normal = upsertUserBlock(
      [{ kind: 'user', id: 'item_user_1', createdAt: '2026-07-02T00:00:00.000Z', text: 'hello' }],
      {
        itemId: 'item_user_1',
        createdAt: '2026-07-02T00:00:00.000Z',
        text: 'hello'
      }
    )

    const normalBlock = normal[0]
    expect(normalBlock?.kind).toBe('user')
    expect(normalBlock?.kind === 'user' ? normalBlock.meta?.steeringStatus : undefined).toBeUndefined()

    const steering = upsertUserBlock(
      [{
        kind: 'user',
        id: 'client_steer',
        createdAt: '2026-07-02T00:00:00.000Z',
        text: 'follow up',
        meta: {
          clientUserMessageId: 'client_steer',
          steeringStatus: 'admitted'
        }
      }],
      {
        itemId: 'item_promoted_steer',
        createdAt: '2026-07-02T00:00:01.000Z',
        text: 'follow up',
        meta: {
          clientUserMessageId: 'client_steer'
        }
      }
    )

    expect(steering).toHaveLength(1)
    const steeringBlock = steering[0]
    expect(steeringBlock?.id).toBe('item_promoted_steer')
    expect(steeringBlock?.kind === 'user' ? steeringBlock.meta?.steeringStatus : undefined).toBe('accepted')
  })

  it('keeps the thread busy while a compaction item is running', () => {
    const runningCompaction: ChatBlock = {
      kind: 'compaction',
      id: 'compact-running',
      summary: 'Compacting context',
      status: 'running'
    }
    const completedCompaction: ChatBlock = {
      kind: 'compaction',
      id: 'compact-completed',
      summary: 'Compacted context',
      status: 'success'
    }

    expect(hasPendingRuntimeWork(runningCompaction)).toBe(true)
    expect(hasPendingRuntimeWork(completedCompaction)).toBe(false)
    expect(threadSnapshotLooksRunning([runningCompaction])).toBe(true)
    expect(threadSnapshotLooksRunning([completedCompaction])).toBe(false)
  })

  it('trusts an explicit idle thread status over stale pending blocks', () => {
    const staleTool: ChatBlock = {
      kind: 'tool',
      id: 'tool-stale',
      summary: 'Old tool',
      status: 'running',
      toolKind: 'tool_call'
    }

    expect(threadSnapshotLooksRunning([staleTool], 'idle')).toBe(false)
    expect(threadSnapshotLooksRunning([staleTool], 'aborted')).toBe(false)
    expect(threadSnapshotLooksRunning([staleTool], 'running')).toBe(true)
    expect(threadSnapshotLooksRunning([staleTool])).toBe(true)
  })

  it('ignores stale pending work once the same turn has visible assistant content', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-1', text: 'Run the task' },
      {
        kind: 'tool',
        id: 'tool-stale',
        summary: 'Old tool',
        status: 'running',
        toolKind: 'tool_call'
      },
      { kind: 'assistant', id: 'answer-1', text: 'The task is complete.' }
    ]

    expect(threadHasPendingRuntimeWork(blocks)).toBe(false)
    expect(threadSnapshotLooksRunning(blocks)).toBe(false)
  })

  it('keeps the thread busy when pending work has no later assistant answer', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-1', text: 'Run the task' },
      { kind: 'assistant', id: 'partial-1', text: 'I will check that.' },
      {
        kind: 'tool',
        id: 'tool-running',
        summary: 'Still running',
        status: 'running',
        toolKind: 'tool_call'
      }
    ]

    expect(threadHasPendingRuntimeWork(blocks)).toBe(true)
    expect(threadSnapshotLooksRunning(blocks)).toBe(true)
  })

  it('does not let stale pending work from an older turn block new input', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-1', text: 'First task' },
      {
        kind: 'tool',
        id: 'tool-stale',
        summary: 'Old tool',
        status: 'running',
        toolKind: 'tool_call'
      },
      { kind: 'user', id: 'user-2', text: 'Second task' },
      { kind: 'assistant', id: 'answer-2', text: 'Second answer.' }
    ]

    expect(threadHasPendingRuntimeWork(blocks)).toBe(false)
    expect(threadSnapshotLooksRunning(blocks)).toBe(false)
  })

  it('settles local pending work after a successful interrupt', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'tool',
        id: 'tool-running',
        summary: 'Running tool',
        status: 'running',
        toolKind: 'tool_call'
      },
      {
        kind: 'approval',
        id: 'approval-pending',
        approvalId: 'approval-1',
        summary: 'Needs approval',
        status: 'pending'
      },
      {
        kind: 'user_input',
        id: 'input-pending',
        requestId: 'input-1',
        questions: [],
        status: 'pending'
      },
      {
        kind: 'tool',
        id: 'tool-success',
        summary: 'Done',
        status: 'success',
        toolKind: 'tool_call'
      }
    ]

    const settled = settlePendingRuntimeWorkAfterInterrupt(blocks)

    expect(settled.map((block) => ('status' in block ? block.status : ''))).toEqual([
      'error',
      'error',
      'cancelled',
      'success'
    ])
    expect(settled.some(hasPendingRuntimeWork)).toBe(false)
  })
})

describe('findReusableEmptyThreadId', () => {
  const workspace = '/work/project'
  const makeThread = (overrides: Partial<NormalizedThread>): NormalizedThread => ({
    id: 'thread',
    title: '新会话',
    updatedAt: '2026-06-14T00:00:00.000Z',
    model: 'deepseek',
    mode: 'agent',
    workspace,
    ...overrides
  })
  const stateWith = (threads: NormalizedThread[]): ChatState =>
    ({ activeThreadId: null, threads, blocks: [] } as unknown as ChatState)
  const emptyProvider = { getThreadDetail: async () => ({ blocks: [] }) }

  it('reuses an empty thread that still carries the default placeholder title', async () => {
    const state = stateWith([makeThread({ id: 'blank', title: '新会话' })])
    const reused = await findReusableEmptyThreadId(state, emptyProvider, workspace)
    expect(reused).toBe('blank')
  })

  it('does not reuse an empty thread that carries a meaningful title (e.g. a released requirement)', async () => {
    // Regression: a freshly released SDD requirement thread is empty but keeps
    // its requirement title. Reusing it would make the next "new conversation"
    // inherit "旅游旅行社区网页" instead of starting fresh.
    const state = stateWith([makeThread({ id: 'requirement', title: '旅游旅行社区网页' })])
    const reused = await findReusableEmptyThreadId(state, emptyProvider, workspace)
    expect(reused).toBeNull()
  })

  it('does not reuse the active thread when it carries a meaningful title', async () => {
    const titled = makeThread({ id: 'requirement', title: '旅游旅行社区网页' })
    const state = {
      activeThreadId: 'requirement',
      threads: [titled],
      blocks: []
    } as unknown as ChatState
    const reused = await findReusableEmptyThreadId(state, emptyProvider, workspace)
    expect(reused).toBeNull()
  })
})
