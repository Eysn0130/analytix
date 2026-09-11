import { describe, expect, it } from 'vitest'
import { CompactionEvent } from './events.js'

describe('compaction event contract', () => {
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
