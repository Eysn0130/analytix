import { describe, expect, it } from 'vitest'
import { CompactionTurnItem, TaskContinuationSnapshotV1Schema } from './items.js'

const digest = (value: string): string => value.repeat(64)

function continuation(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    schemaVersion: 1,
    goal: {
      goalId: 'goal-1',
      objective: 'Preserve current objective',
      status: 'active',
      stateDigest: digest('a')
    },
    todos: [{
      todoId: 'todo-1',
      content: 'Preserve unfinished work',
      status: 'pending',
      stateDigest: digest('b')
    }],
    latestUserConstraints: ['Do not upgrade unknown evidence'],
    evidenceReferences: [{
      referenceDigest: digest('c'),
      supportStatus: 'unverified_for_case_facts'
    }],
    evidenceAuthority: 'unverified_for_case_facts',
    stateDigest: digest('d'),
    ...overrides
  }
}

function compaction(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'item-compaction',
    turnId: 'turn-compaction',
    threadId: 'thread-compaction',
    role: 'system',
    status: 'completed',
    createdAt: '2026-07-22T00:00:00Z',
    finishedAt: '2026-07-22T00:00:00Z',
    kind: 'compaction',
    summary: 'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.',
    replacedTokens: 100,
    auto: true,
    pinnedConstraints: ['user: preserve recent turns'],
    sourceDigest: digest('e'),
    digestMarker: `sha256:${digest('e').slice(0, 12)}`,
    sourceItemIds: ['source-1'],
    schemaVersion: 4,
    reasoningExcluded: true,
    reasoningExclusionProof: `sha256:${digest('f')}`,
    assistantProseExcluded: true,
    toolPayloadsExcluded: true,
    caseFactsExcluded: true,
    providerHistoryProjectionVersion: 2,
    taskContinuation: continuation(),
    ...overrides
  }
}

function publicCaseCompaction(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return compaction({
    summary: 'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.',
    replacedTokens: undefined,
    sourceItemIds: [],
    schemaVersion: 3,
    providerHistoryProjectionVersion: undefined,
    caseHistoryProjectionVersion: 2,
    taskContinuation: undefined,
    ...overrides
  })
}

describe('automatic compaction continuation contract', () => {
  it('accepts the closed V4 snapshot without upgrading evidence authority', () => {
    const parsed = CompactionTurnItem.parse(compaction())
    expect(parsed.schemaVersion).toBe(4)
    expect(parsed.taskContinuation?.evidenceAuthority).toBe('unverified_for_case_facts')
    expect(parsed.taskContinuation?.evidenceReferences[0]?.supportStatus).toBe('unverified_for_case_facts')
  })

  it('rejects V4 without auto, continuation, or provider projection V2', () => {
    for (const invalid of [
      compaction({ auto: false }),
      compaction({ taskContinuation: undefined }),
      compaction({ providerHistoryProjectionVersion: 1 })
    ]) {
      expect(CompactionTurnItem.safeParse(invalid).success).toBe(false)
    }
  })

  it('rejects evidence upgrades, duplicate identities, unknown fields, and invalid todo reasons', () => {
    for (const invalid of [
      continuation({ evidenceAuthority: 'verified' }),
      continuation({ evidenceReferences: [
        { referenceDigest: digest('c'), supportStatus: 'unverified_for_case_facts' },
        { referenceDigest: digest('c'), supportStatus: 'unverified_for_case_facts' }
      ] }),
      continuation({ todos: [
        { todoId: 'todo-1', content: 'one', status: 'pending', stateDigest: digest('b') },
        { todoId: 'todo-1', content: 'two', status: 'pending', stateDigest: digest('c') }
      ] }),
      continuation({ todos: [{
        todoId: 'todo-1', content: 'failed without reason', status: 'failed', stateDigest: digest('b')
      }] }),
      continuation({ unknown: true })
    ]) {
      expect(TaskContinuationSnapshotV1Schema.safeParse(invalid).success).toBe(false)
    }
  })

  it('rejects continuation data on legacy and manual compactions', () => {
    expect(CompactionTurnItem.safeParse(compaction({ schemaVersion: 3, auto: false })).success).toBe(false)
    expect(CompactionTurnItem.safeParse(compaction({ schemaVersion: undefined, auto: false })).success).toBe(false)
    expect(CompactionTurnItem.safeParse(compaction({ unknown: true })).success).toBe(false)
  })

  it('accepts only the closed public case-compaction V2 marker', () => {
    expect(CompactionTurnItem.safeParse(publicCaseCompaction()).success).toBe(true)

    for (const invalid of [
      publicCaseCompaction({ summary: 'case history compacted' }),
      publicCaseCompaction({ replacedTokens: 1 }),
      publicCaseCompaction({ caseHistoryProjectionVersion: undefined, replacedTokens: 1 }),
      publicCaseCompaction({ caseHistoryProjectionVersion: 1, replacedTokens: 1 }),
      publicCaseCompaction({ auto: undefined }),
      publicCaseCompaction({ finishedAt: '2026-07-22T00:00:01Z' }),
      publicCaseCompaction({ pinnedConstraints: [] }),
      publicCaseCompaction({ sourceItemIds: ['private-item'] }),
      publicCaseCompaction({ providerHistoryProjectionVersion: 1 }),
      publicCaseCompaction({ taskContinuation: continuation() }),
      publicCaseCompaction({ sourceContextDigest: digest('a') }),
      publicCaseCompaction({ caseCompactionBinding: { schemaVersion: 1 } }),
      publicCaseCompaction({ assistantProseExcluded: undefined })
    ]) {
      expect(CompactionTurnItem.safeParse(invalid).success).toBe(false)
    }
  })
})
