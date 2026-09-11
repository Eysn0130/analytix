import { describe, expect, it } from 'vitest'
import { isPublicSseIpcPayload, PublicRuntimeEventFilter } from './public-runtime-content'
import { isClosedPublicRuntimeSseEvent, projectPublicRuntimeSseBlock } from './public-runtime-sse'

const digest = (value: string): string => value.repeat(64)

function automaticCompactionEvent(): Record<string, unknown> {
  const timestamp = '2026-07-22T00:00:00.000Z'
  return {
    seq: 41,
    kind: 'item_completed',
    timestamp,
    threadId: 'thread-auto-compaction',
    turnId: 'turn-auto-compaction',
    itemId: 'item-auto-compaction',
    item: {
      id: 'item-auto-compaction',
      turnId: 'turn-auto-compaction',
      threadId: 'thread-auto-compaction',
      role: 'system',
      status: 'completed',
      createdAt: timestamp,
      finishedAt: timestamp,
      kind: 'compaction',
      summary: 'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.',
      replacedTokens: 100,
      auto: true,
      pinnedConstraints: ['user: preserve recent turns'],
      sourceDigest: digest('a'),
      digestMarker: `sha256:${digest('a').slice(0, 12)}`,
      sourceItemIds: ['source-1'],
      schemaVersion: 4,
      reasoningExcluded: true,
      reasoningExclusionProof: `sha256:${digest('b')}`,
      assistantProseExcluded: true,
      toolPayloadsExcluded: true,
      caseFactsExcluded: true,
      providerHistoryProjectionVersion: 2,
      taskContinuation: {
        schemaVersion: 1,
        goal: {
          goalId: 'goal-1',
          objective: 'Preserve current objective',
          status: 'active',
          stateDigest: digest('c')
        },
        todos: [{
          todoId: 'todo-1',
          content: 'Preserve unfinished work',
          status: 'pending',
          stateDigest: digest('d')
        }],
        latestUserConstraints: ['Preserve the latest instruction'],
        evidenceReferences: [{
          referenceDigest: digest('e'),
          supportStatus: 'unverified_for_case_facts'
        }],
        evidenceAuthority: 'unverified_for_case_facts',
        stateDigest: digest('f')
      }
    }
  }
}

function caseCompactionItemEvent(): Record<string, unknown> {
  const timestamp = '2026-07-22T00:00:00.000Z'
  return {
    seq: 42,
    kind: 'item_completed',
    timestamp,
    threadId: 'thread-case-compaction',
    turnId: 'turn-case-compaction',
    itemId: 'item-case-compaction',
    item: {
      id: 'item-case-compaction',
      turnId: 'turn-case-compaction',
      threadId: 'thread-case-compaction',
      role: 'system',
      status: 'completed',
      createdAt: timestamp,
      finishedAt: timestamp,
      kind: 'compaction',
      summary: 'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.',
      auto: false,
      pinnedConstraints: ['user: preserve recent turns'],
      sourceDigest: digest('1'),
      digestMarker: `sha256:${digest('1').slice(0, 12)}`,
      sourceItemIds: [],
      schemaVersion: 3,
      reasoningExcluded: true,
      reasoningExclusionProof: `sha256:${digest('2')}`,
      assistantProseExcluded: true,
      toolPayloadsExcluded: true,
      caseFactsExcluded: true,
      caseHistoryProjectionVersion: 2
    }
  }
}

function caseCompactionCompletedEvent(): Record<string, unknown> {
  const itemEvent = caseCompactionItemEvent()
  const item = itemEvent.item as Record<string, unknown>
  return {
    seq: 43,
    kind: 'compaction_completed',
    timestamp: itemEvent.timestamp,
    threadId: itemEvent.threadId,
    turnId: itemEvent.turnId,
    summary: item.summary,
    auto: item.auto,
    pinnedConstraints: item.pinnedConstraints,
    sourceDigest: item.sourceDigest,
    digestMarker: item.digestMarker,
    sourceItemIds: item.sourceItemIds,
    schemaVersion: 2,
    reasoningExcluded: true,
    reasoningExclusionProof: item.reasoningExclusionProof
  }
}

function frame(event: Record<string, unknown>): string {
  return `id: 41\nevent: item_completed\ndata: ${JSON.stringify(event)}`
}

describe('public automatic compaction projection', () => {
  it('emits only the closed V4 continuation contract', () => {
    const event = automaticCompactionEvent()
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    expect(isPublicSseIpcPayload({ streamId: 'test-compaction', events: [event] })).toBe(true)
    const decision = projectPublicRuntimeSseBlock(
      frame(event),
      'thread-auto-compaction',
      new PublicRuntimeEventFilter()
    )
    expect(decision?.status).toBe('emit')
  })

  it('rejects evidence upgrades and open continuation fields', () => {
    for (const mutate of [
      (continuation: Record<string, unknown>) => { continuation.evidenceAuthority = 'verified' },
      (continuation: Record<string, unknown>) => { continuation.privateReasoning = 'must-not-pass' }
    ]) {
      const event = automaticCompactionEvent()
      const item = event.item as Record<string, unknown>
      const continuation = item.taskContinuation as Record<string, unknown>
      mutate(continuation)
      expect(projectPublicRuntimeSseBlock(
        frame(event),
        'thread-auto-compaction',
        new PublicRuntimeEventFilter()
      )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    }
  })

  it('admits the closed public case marker and item-less completion event', () => {
    for (const event of [caseCompactionItemEvent(), caseCompactionCompletedEvent()]) {
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
      expect(projectPublicRuntimeSseBlock(
        `id: ${String(event.seq)}\nevent: ${String(event.kind)}\ndata: ${JSON.stringify(event)}`,
        'thread-case-compaction',
        new PublicRuntimeEventFilter()
      )).toEqual({ status: 'emit', seq: event.seq, event })
    }
  })

  it('rejects private or detached case-compaction projections', () => {
    const invalidItems: Array<(item: Record<string, unknown>) => void> = [
      (item) => { item.taskContinuation = { schemaVersion: 1 } },
      (item) => { item.sourceContextDigest = digest('3') },
      (item) => { item.caseCompactionBinding = { schemaVersion: 1 } },
      (item) => { item.replacedTokens = 100 },
      (item) => { item.finishedAt = '2026-07-22T00:00:01.000Z' },
      (item) => { item.pinnedConstraints = [] },
      (item) => { item.sourceItemIds = ['private-item'] },
      (item) => { item.reasoningExclusionProof = undefined },
      (item) => { item.summary = 'case history compacted' }
    ]
    for (const mutate of invalidItems) {
      const event = caseCompactionItemEvent()
      mutate(event.item as Record<string, unknown>)
      expect(isClosedPublicRuntimeSseEvent(event)).toBe(false)
    }

    const completion = caseCompactionCompletedEvent()
    completion.itemId = 'private-item'
    expect(isClosedPublicRuntimeSseEvent(completion)).toBe(false)

    for (const mutate of [
      (event: Record<string, unknown>) => { event.summary = 'case history compacted' },
      (event: Record<string, unknown>) => { event.replacedTokens = 100 },
      (event: Record<string, unknown>) => { event.sourceItemIds = ['private-item'] },
      (event: Record<string, unknown>) => { event.auto = undefined },
      (event: Record<string, unknown>) => { event.schemaVersion = 3 },
      (event: Record<string, unknown>) => { event.reasoningExcluded = false },
      (event: Record<string, unknown>) => { event.reasoningExclusionProof = 'sha256:invalid' },
      (event: Record<string, unknown>) => { event.digestMarker = 'sha256:invalid' }
    ]) {
      const invalid = caseCompactionCompletedEvent()
      mutate(invalid)
      expect(isClosedPublicRuntimeSseEvent(invalid)).toBe(false)
    }
  })
})
