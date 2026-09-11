import { describe, expect, it } from 'vitest'
import { makeAssistantTextItem, makeCompactionItem, makeUserItem } from '../domain/item.js'
import {
  effectiveHistoryAfterLatestCompaction,
  insertCompactionIntoVisibleHistory,
  placeCompactionsAtTurnEnd
} from '../shared/compaction-history.js'

describe('compaction history projection', () => {
  it('keeps the visible transcript while model history starts at the latest compaction', () => {
    const threadId = 'thread_1'
    const turnId = 'turn_1'
    const headA = makeUserItem({ id: 'item_head_a', threadId, turnId, text: 'old user context' })
    const headB = makeAssistantTextItem({
      id: 'item_head_b',
      threadId,
      turnId,
      text: 'old assistant context',
      status: 'completed'
    })
    const previousSummary = makeCompactionItem({
      id: 'compaction_previous',
      threadId,
      turnId,
      summary: 'previous summary',
      replacedTokens: 100,
      pinnedConstraints: []
    })
    const tailA = makeUserItem({ id: 'item_tail_a', threadId, turnId, text: 'recent user context' })
    const tailB = makeAssistantTextItem({
      id: 'item_tail_b',
      threadId,
      turnId,
      text: 'recent assistant context',
      status: 'completed'
    })
    const nextSummary = makeCompactionItem({
      id: 'compaction_next',
      threadId,
      turnId,
      summary: 'next summary',
      replacedTokens: 200,
      pinnedConstraints: []
    })

    const visible = insertCompactionIntoVisibleHistory({
      visibleItems: [headA, headB, previousSummary, tailA, tailB],
      compactedItems: [nextSummary, tailA, tailB],
      summaryItem: nextSummary
    })

    expect(visible.map((item) => item.id)).toEqual([
      'item_head_a',
      'item_head_b',
      'compaction_previous',
      'compaction_next',
      'item_tail_a',
      'item_tail_b'
    ])
    expect(effectiveHistoryAfterLatestCompaction(visible).map((item) => item.id)).toEqual([
      'compaction_next',
      'item_tail_a',
      'item_tail_b'
    ])
  })

  it('moves compaction summaries to the end of a turn bucket for renderer grouping', () => {
    const threadId = 'thread_1'
    const turnId = 'turn_3'
    const userMessage = makeUserItem({ id: 'item_user_3', threadId, turnId, text: 'next request' })
    const summary = makeCompactionItem({
      id: 'compaction_for_turn_3',
      threadId,
      turnId,
      summary: 'fresh summary',
      replacedTokens: 200,
      pinnedConstraints: []
    })

    expect(placeCompactionsAtTurnEnd([summary, userMessage]).map((item) => item.id)).toEqual([
      'item_user_3',
      'compaction_for_turn_3'
    ])
  })
})
