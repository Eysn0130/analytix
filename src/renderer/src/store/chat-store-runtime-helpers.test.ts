import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../agent/types'
import type { NormalizedThread } from '../agent/types'
import type { ChatState } from './chat-store-types'
import {
  clearedThreadSelection,
  findReusableEmptyThreadId,
  updateUserInputDraftInterruption,
  forgetUserInputDraftInterruptionsForThread,
  markDeletedUserInputDraftThread,
  findUserInputBlockIndex,
  findUserInputStatusBlockIndex,
  upsertUserInputBlock,
  hasPendingRuntimeWork,
  settlePendingRuntimeWorkAfterInterrupt,
  retireProviderRetryStatusesForTurn,
  threadHasPendingRuntimeWork,
  threadSnapshotLooksRunning,
  upsertUserBlock
} from './chat-store-runtime-helpers'

describe('terminal provider retry retirement', () => {
  const retry = { attempt: 2, maxAttempt: 3 }
  const row = (patch: Partial<Extract<ChatBlock, { kind: 'system' }>> = {}): Extract<ChatBlock, { kind: 'system' }> => ({
    kind: 'system', id: 'retry-current', text: 'Provider request is retrying',
    meta: { turnId: 'turn-current', providerRetry: retry }, ...patch
  })

  it('retires only an exact terminal turn and preserves adjacent or unowned object identities', () => {
    const adjacent = row({ id: 'adjacent', meta: { turnId: 'turn-other', providerRetry: retry } })
    const unowned = row({ id: 'unowned', meta: { providerRetry: retry } })
    const ordinary = row({ id: 'ordinary', text: 'Independent observation', meta: { turnId: 'turn-current' } })
    const blocks = [row(), row({ id: 'retry-zh', text: '模型服务正在重试' }), adjacent, unowned, ordinary]
    expect(retireProviderRetryStatusesForTurn(blocks, '')).toBe(blocks)
    expect(retireProviderRetryStatusesForTurn(blocks, 'turn-missing')).toBe(blocks)
    const retired = retireProviderRetryStatusesForTurn(blocks, 'turn-current')
    expect(retired).toEqual([adjacent, unowned, ordinary])
    retired.forEach((block, index) => expect(block).toBe([adjacent, unowned, ordinary][index]))
    expect(blocks[0].meta?.providerRetry).toBe(retry)
  })

  it.each([
    row({ severity: 'error', text: 'Actual provider failure', code: 'provider_unavailable', detail: 'HTTP 503' }),
    row({ severity: 'warning', text: 'Independent warning', detail: 'Preserve warning detail' }),
    row({ code: 'independent_code', text: 'Independent coded observation' }),
    row({ text: 'Independent text without a separate diagnostic field' }),
    row({ text: 'Independent metadata', meta: { turnId: 'turn-current', providerRetry: retry, displayText: 'Retained display text' } })
  ])('removes retry metadata without dropping independent error, code, detail or text: $text', original => {
    const [retired] = retireProviderRetryStatusesForTurn([original], 'turn-current')
    const meta = { ...original.meta }
    delete meta.providerRetry
    expect(retired).toEqual({ ...original, meta })
    expect(original.meta?.providerRetry).toBe(retry)
  })

  it.each([undefined, 'error'] as const)('preserves mixed provider errors without their stale retry caption (severity: %s)', severity => {
    const providerError = { status: 503, retryable: true, hasApiKey: false }
    const original = row({ severity, code: severity ? 'provider_unavailable' : undefined,
      meta: { turnId: 'turn-current', providerRetry: retry, providerError } })
    const [retired] = retireProviderRetryStatusesForTurn([original], 'turn-current')
    expect(retired).toEqual({ ...original, text: 'Model request details',
      meta: { turnId: 'turn-current', providerError } })
  })

  it.each(['providerRetryingStatus', 'common:providerRetryingStatus'])('retires only identified legacy retry captions and preserves mixed errors (%s)', text => {
    const providerError = { status: 503, retryable: true, hasApiKey: false }
    const mixed = row({ id: 'legacy-error', text, severity: 'error', code: 'provider_unavailable', detail: 'HTTP 503',
      meta: { turnId: 'turn-current', providerRetry: retry, providerError } })
    const adjacent = row({ id: 'adjacent', text, meta: { turnId: 'turn-other', providerRetry: retry } })
    const ordinary = row({ id: 'ordinary', text, meta: { turnId: 'turn-current' } })
    const user: ChatBlock = { kind: 'user', id: 'user', text, meta: { turnId: 'turn-current' } }
    const retired = retireProviderRetryStatusesForTurn([row({ text }), mixed, adjacent, ordinary, user], 'turn-current')
    expect(retired).toEqual([{ ...mixed, text: 'Model request details', meta: { turnId: 'turn-current', providerError } },
      adjacent, ordinary, user])
    expect(retired[1]).toBe(adjacent)
    expect(retired[2]).toBe(ordinary)
    expect(retired[3]).toBe(user)
    expect(mixed.meta?.providerRetry).toBe(retry)
  })
})

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
      'error',
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


describe('user-input identity compatibility boundary', () => {
  const input = (id: string, requestId: string): Extract<ChatBlock, { kind: 'user_input' }> => ({
    kind: 'user_input', id, requestId, status: 'pending', questions: []
  })
  it('admits exact pairs and the observed request alias when itemId is absent upstream', () => {
    const blocks = [input('item-a', 'input-a'), input('item-b', 'input-b')]
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'item-b', requestId: 'input-b', status: 'cancelled' })).toBe(1)
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'input-b', requestId: 'input-b', status: 'cancelled' })).toBe(1)
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'item-b', status: 'cancelled' })).toBe(1)
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'input-b', status: 'cancelled' })).toBe(1)
    expect(findUserInputBlockIndex(blocks, 'item-b', 'input-b')).toBe(1)
  })
  it('rejects crossed, unknown, blank and ambiguous restored identities', () => {
    const blocks = [input('item-a', 'input-a'), input('item-b', 'input-b')]
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'item-a', requestId: 'input-b', status: 'cancelled' })).toBe(-1)
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'unknown', requestId: 'input-b', status: 'cancelled' })).toBe(-1)
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'item-b', requestId: '', status: 'cancelled' })).toBe(-1)
    const restored = [input('shared', 'input-a'), input('shared', 'input-b')]
    expect(findUserInputStatusBlockIndex(restored, { itemId: 'shared', status: 'cancelled' })).toBe(-1)
    expect(findUserInputBlockIndex(restored, 'shared', 'input-b')).toBe(-1)
    expect(findUserInputStatusBlockIndex([...blocks, { kind: 'user', id: 'item-b', text: 'Synthetic duplicate' }],
      { itemId: 'input-b', requestId: 'input-b', status: 'cancelled' })).toBe(-1)
  })
  it('rejects collisions across item/request namespaces and preserves the admitted pair', () => {
    const blocks = [input('item-a', 'input-a'), input('item-b', 'input-b')]
    expect(upsertUserInputBlock(blocks, input('item-a', 'input-c'))).toBe(blocks)
    expect(upsertUserInputBlock(blocks, input('item-c', 'input-a'))).toBe(blocks)
    expect(upsertUserInputBlock(blocks, input('input-a', 'input-c'))).toBe(blocks)
    expect(upsertUserInputBlock(blocks, input('item-c', 'item-a'))).toBe(blocks)
    expect(upsertUserInputBlock(blocks, input('item-c', 'input-c'))).toHaveLength(3)
  })
  it.each(['submitted', 'cancelled', 'continued'] as const)('does not reopen or flip an admitted %s request', status => {
    const block = { ...input('item-a', 'input-a'), status }
    const blocks = [block]
    expect(upsertUserInputBlock(blocks, input('item-a', 'input-a'))).toBe(blocks)
    expect(upsertUserInputBlock(blocks, input('item-a', 'input-a'))).toMatchObject([{ status }])
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'item-a', requestId: 'input-a', status })).toBe(0)
    expect(findUserInputStatusBlockIndex(blocks, { itemId: 'item-a', requestId: 'input-a', status: status === 'submitted' ? 'cancelled' : 'submitted' })).toBe(-1)
  })
})

describe('bounded Question interruption metadata', () => {
  const requestId = 'input_' + 'a'.repeat(64)
  const selector = { threadId: 'thread-a', turnId: 'turn-a', itemId: 'item_' + requestId, requestId }
  const revision = '11111111-1111-4111-8111-111111111111'
  // Keys follow the exact normalized schema field order used by the leaf.
  const normalizedKey = JSON.stringify(selector)
  it('projects only revision and disposition and rejects malformed owner/revision metadata', () => {
    const result = updateUserInputDraftInterruption({}, normalizedKey, { revision, unsent: true, raw: 'PRIVATE_RAW_SENTINEL' } as { revision: string; unsent: boolean })
    expect(Object.values(result)).toEqual([{ revision, unsent: true }]); expect(JSON.stringify(result)).not.toContain('PRIVATE_')
    expect(updateUserInputDraftInterruption(result, '{}', { revision, unsent: true })).toBe(result)
    expect(updateUserInputDraftInterruption(result, normalizedKey, { revision: 'invalid', unsent: true })).toBe(result)
    expect(updateUserInputDraftInterruption(result, normalizedKey, null)).toEqual({})
  })
  it('rejects a former leaf write after confirmed deletion but keeps a different owner', () => {
    const deleted = markDeletedUserInputDraftThread(undefined, 'thread-a')
    expect(updateUserInputDraftInterruption({}, normalizedKey, { revision, unsent: false }, deleted)).toEqual({})
    const other = JSON.stringify({ ...selector, threadId: 'thread-b' })
    expect(updateUserInputDraftInterruption({}, other, { revision: null, unsent: true }, deleted)).toEqual({ [other]: { revision: null, unsent: true } })
  })
  it('forgets the deleted thread while retaining another owner', () => {
    const other = JSON.stringify({ ...selector, threadId: 'thread-b', turnId: 'turn-b' })
    const records = { ...updateUserInputDraftInterruption({}, normalizedKey, { revision, unsent: false }),
      ...updateUserInputDraftInterruption({}, other, { revision: null, unsent: true }) }
    expect(forgetUserInputDraftInterruptionsForThread(records, 'thread-a')).toEqual({ [other]: { revision: null, unsent: true } })
  })
})


describe('thread selection usage owner cleanup', () => {
  it('returns an empty historical owner and no stale latest usage for all shared selection-clear callers', () => {
    const cleared = clearedThreadSelection()
    expect(cleared).toMatchObject({ activeThreadId: null, blocks: [], turnUsageById: {}, rewoundTurnIds: {}, lastTurnUsage: null })
  })
})


describe('Queue selection preservation', () => {
  it('clears the selected timeline without retiring any thread queue or pause', () => {
    const queuedMessages = [{ threadId: 'thread-a', id: 'a', text: 'A' }, { threadId: 'thread-b', id: 'b', text: 'B' }]
    const queuedMessagePauses = { 'thread-a': 'interrupted', 'thread-b': 'failed' }
    const state = { activeThreadId: 'thread-a', queuedMessages, queuedMessagePauses }
    Object.assign(state, clearedThreadSelection())
    expect(state.activeThreadId).toBeNull()
    expect(state.queuedMessages).toBe(queuedMessages)
    expect(state.queuedMessagePauses).toBe(queuedMessagePauses)
  })
})
