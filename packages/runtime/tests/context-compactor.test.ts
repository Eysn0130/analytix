import { describe, expect, it } from 'vitest'
import { createImmutablePrefix } from '../src/cache/immutable-prefix.js'
import { makeToolCallItem, makeToolResultItem, makeUserItem } from '../src/domain/item.js'
import { ContextCompactor } from '../src/shared/context-compactor.js'

describe('ContextCompactor tail repair', () => {
  it('keeps recent tool-result tails paired with their tool-call items during compaction', () => {
    const compactor = new ContextCompactor({ softThreshold: 1, hardThreshold: 2 })
    const prefix = createImmutablePrefix({ systemPrompt: 'system' })
    const result = compactor.compact({
      threadId: 'thr_1',
      turnId: 'turn_1',
      prefix,
      keepRecent: 1,
      history: [
        makeUserItem({ id: 'u1', turnId: 'turn_1', threadId: 'thr_1', text: 'first request' }),
        makeToolCallItem({
          id: 'call_keep',
          turnId: 'turn_1',
          threadId: 'thr_1',
          callId: 'call_keep',
          toolName: 'read',
          arguments: { path: 'a.txt' }
        }),
        makeToolResultItem({
          id: 'result_keep',
          turnId: 'turn_1',
          threadId: 'thr_1',
          callId: 'call_keep',
          toolName: 'read',
          output: 'contents'
        })
      ]
    })

    expect(result.next.map((item) => item.id)).toEqual([
      result.summaryItem.id,
      'call_keep',
      'result_keep'
    ])
    expect(result.summaryItem.kind === 'compaction' ? result.summaryItem.sourceItemIds : [])
      .toEqual(['u1'])
  })
})
