import { describe, expect, it } from 'vitest'
import { CompactionEvent, RuntimeEvent } from './events.js'
import { CompactionTurnItem } from './items.js'
import { CompactResponse } from './turns.js'

function conversationCompactionFixture() {
  const createdAt = '2026-10-10T00:00:00Z', finishedAt = '2026-10-10T00:00:02Z'
  const item = { id: 'compaction_1', threadId: 'thread_1', turnId: 'turn_1', kind: 'compaction' as const,
    role: 'system' as const, status: 'completed' as const, createdAt, finishedAt,
    summary: 'Continue the accepted task using its recorded decisions.', replacedTokens: 1234,
    auto: false, pinnedConstraints: ['user: preserve recent turns'], sourceDigest: 'a'.repeat(64),
    digestMarker: `sha256:${'a'.repeat(12)}`, sourceItemIds: ['source_1'], schemaVersion: 5 as const,
    reasoningExcluded: true as const, reasoningExclusionProof: `sha256:${'b'.repeat(64)}`,
    toolPayloadsExcluded: true as const, caseFactsExcluded: true as const, providerHistoryProjectionVersion: 3 as const,
    compactionId: 'compaction_1', summaryKind: 'conversation' as const, summaryDigest: 'c'.repeat(64), checkpointDigest: 'd'.repeat(64),
    shadowedItemCount: 3, tokenCountKind: 'estimated' as const, tokenEstimatorVersion: 1 as const }
  const turn = { id: item.turnId, threadId: item.threadId, status: 'completed', prompt: '/compact', createdAt, startedAt: createdAt, finishedAt,
    items: [item], steering: [], attachmentIds: [], activeSkillIds: [], injectedMemoryIds: [] }
  const { id, role, status, kind, schemaVersion, ...facts } = item
  const event = { ...facts, seq: 7, timestamp: finishedAt, kind: 'compaction_completed' as const, schemaVersion: 3 as const, itemId: id, turn }
  // Item-only projection metadata does not appear on the event/response.
  const { toolPayloadsExcluded, caseFactsExcluded, providerHistoryProjectionVersion, ...closedEvent } = event
  return { item, event: closedEvent }
}

describe('compaction event contract', () => {
  it('binds the V5 summary, inventory count and estimated tokens across event and HTTP response', () => {
    const { item, event } = conversationCompactionFixture()
    expect(CompactionTurnItem.safeParse(item).success).toBe(true)
    expect(CompactionEvent.safeParse(event).success).toBe(true)
    expect(RuntimeEvent.safeParse(event).success).toBe(true)
    const { kind, seq, timestamp, ...response } = event
    expect(CompactResponse.safeParse({ ...response, ok: true, eventSeq: seq }).success).toBe(true)
    expect(CompactionEvent.safeParse({ ...event, shadowedItemCount: 99 }).success).toBe(false)
    expect(CompactionEvent.safeParse({ ...event, summary: 'different summary' }).success).toBe(false)
    expect(CompactionTurnItem.safeParse({ ...item, summaryKind: undefined }).success).toBe(false)
    expect(CompactionTurnItem.safeParse({ ...item, schemaVersion: 3 }).success).toBe(false)
  })

  it('keeps started and failed operations free of summary facts', () => {
    const { event } = conversationCompactionFixture()
    const started = { kind: 'compaction_started', schemaVersion: 3, seq: 5, timestamp: event.createdAt,
      threadId: event.threadId, turnId: event.turnId, itemId: event.itemId, compactionId: event.compactionId, auto: false, startedAt: event.createdAt }
    expect(CompactionEvent.safeParse(started).success).toBe(true)
    expect(RuntimeEvent.safeParse(started).success).toBe(true)
    expect(CompactionEvent.safeParse({ ...started, summary: 'imagined summary' }).success).toBe(false)
    const failed = { ...started, kind: 'compaction_failed', startedAt: undefined,
      reasonCode: 'summary_unavailable', finishedAt: event.finishedAt }
    expect(CompactionEvent.safeParse(failed).success).toBe(true)
    expect(RuntimeEvent.safeParse(failed).success).toBe(true)
  })
  it('preserves the public reasoning-exclusion fields on a completed event', () => {
    const event = {
      seq: 9,
      timestamp: '2026-08-02T00:00:00Z',
      threadId: 'thread-compaction',
      turnId: 'turn-compaction',
      kind: 'compaction_completed' as const,
      summary: 'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.',
      replacedTokens: 512,
      auto: false,
      pinnedConstraints: ['user: preserve recent turns'],
      sourceDigest: 'a'.repeat(64),
      digestMarker: `sha256:${'a'.repeat(12)}`,
      sourceItemIds: ['source-item'],
      schemaVersion: 2 as const,
      reasoningExcluded: true as const,
      reasoningExclusionProof: `sha256:${'b'.repeat(64)}`
    }

    expect(CompactionEvent.parse(event)).toEqual(event)
  })

  it('rejects malformed public reasoning-exclusion fields', () => {
    const base = {
      seq: 9,
      timestamp: '2026-08-02T00:00:00Z',
      threadId: 'thread-compaction',
      turnId: 'turn-compaction',
      kind: 'compaction_completed' as const
    }
    expect(CompactionEvent.safeParse({ ...base, schemaVersion: 3 }).success).toBe(false)
    expect(CompactionEvent.safeParse({ ...base, reasoningExcluded: false }).success).toBe(false)
    expect(CompactionEvent.safeParse({ ...base, reasoningExclusionProof: 'sha256:invalid' }).success).toBe(false)
  })
})
