import { describe, expect, it } from 'vitest'
import { RuntimeEvent as RuntimeEventSchema, type RuntimeEvent } from '../src/contracts/events.js'
import {
  createAnalytixCheckpointMetadata,
  extractCheckpointChangedFilesFromItems,
  planConversationOnlyRewindFromEvents,
  type PrivateCheckpointToolResultInput
} from '../src/domain/checkpoint-rewind-contract.js'
import { makePublicToolResultWithheldProjection } from '../src/domain/item.js'

const timestamp = '2026-06-20T10:00:00.000Z'
const workspace = '/tmp/analytix-p4a-workspace'

function baseEvent(overrides: Partial<RuntimeEvent> & Pick<RuntimeEvent, 'kind'>): RuntimeEvent {
  return {
    seq: overrides.seq ?? 1,
    timestamp: overrides.timestamp ?? timestamp,
    threadId: overrides.threadId ?? 'thread-p4a',
    ...overrides
  } as RuntimeEvent
}

describe('P4A checkpoint rewind contract', () => {
  it('creates deterministic analytix-owned checkpoint metadata for file changes', () => {
    const metadata = createAnalytixCheckpointMetadata({
      workspace,
      threadId: 'thread-p4a',
      turnId: 'turn-2',
      createdAt: timestamp,
      changedFiles: [
        {
          path: '/tmp/analytix-p4a-workspace/src/app.ts',
          changeKind: 'modified',
          beforeHash: 'sha256:before',
          afterHash: 'sha256:after'
        },
        {
          relativePath: 'src/app.ts',
          changeKind: 'modified',
          beforeHash: 'sha256:ignored-first-touch-wins'
        },
        {
          relativePath: 'docs/new.md',
          changeKind: 'created'
        }
      ]
    })

    const same = createAnalytixCheckpointMetadata({
      workspace,
      threadId: 'thread-p4a',
      turnId: 'turn-2',
      createdAt: timestamp,
      changedFiles: [
        {
          path: '/tmp/analytix-p4a-workspace/src/app.ts',
          changeKind: 'modified',
          beforeHash: 'sha256:before',
          afterHash: 'sha256:after'
        },
        {
          relativePath: 'src/app.ts',
          changeKind: 'modified',
          beforeHash: 'sha256:ignored-first-touch-wins'
        },
        {
          relativePath: 'docs/new.md',
          changeKind: 'created'
        }
      ]
    })

    expect(metadata.checkpointId).toMatch(/^axcp_/)
    expect(metadata.checkpointId).toBe(same.checkpointId)
    expect(metadata).toMatchObject({
      schemaVersion: 1,
      threadId: 'thread-p4a',
      turnId: 'turn-2',
      workspace,
      createdAt: timestamp,
      status: 'captured'
    })
    expect(metadata.changedFiles).toEqual([
      {
        relativePath: 'src/app.ts',
        changeKind: 'modified',
        beforeHash: 'sha256:before',
        afterHash: 'sha256:after'
      },
      {
        relativePath: 'docs/new.md',
        changeKind: 'created'
      }
    ])
  })

  it('rejects checkpoint changed files that escape the workspace', () => {
    expect(() => createAnalytixCheckpointMetadata({
      workspace,
      threadId: 'thread-p4a',
      turnId: 'turn-2',
      createdAt: timestamp,
      changedFiles: [{ path: '../outside.txt' }]
    })).toThrow(/escapes workspace/)
  })

  it('accepts legal dot-dot-prefixed filenames inside the workspace', () => {
    const metadata = createAnalytixCheckpointMetadata({
      workspace,
      threadId: 'thread-p4a',
      turnId: 'turn-2',
      createdAt: timestamp,
      changedFiles: [{ relativePath: '..name', changeKind: 'modified' }]
    })

    expect(metadata.changedFiles).toEqual([
      {
        relativePath: '..name',
        changeKind: 'modified'
      }
    ])
  })

  it('extracts changed-file metadata from successful file_change tool results', () => {
    const items: PrivateCheckpointToolResultInput[] = [
      {
        kind: 'tool_result',
        toolKind: 'file_change',
        output: {
          path: '/tmp/analytix-p4a-workspace/src/app.ts',
          patch: 'diff --git a/src/app.ts b/src/app.ts\n'
        },
        isError: false
      },
      {
        kind: 'tool_result',
        toolKind: 'tool_call',
        output: { path: '/tmp/analytix-p4a-workspace/src/ignored.ts' },
        isError: false
      }
    ]

    expect(extractCheckpointChangedFilesFromItems(items, workspace)).toEqual([
      {
        relativePath: 'src/app.ts',
        changeKind: 'modified'
      }
    ])
  })

  it('plans a conversation-only rewind from analytix checkpoint events without rewriting the event log', () => {
    const checkpoint = createAnalytixCheckpointMetadata({
      workspace,
      threadId: 'thread-p4a',
      turnId: 'turn-2',
      createdAt: timestamp,
      changedFiles: [{ relativePath: 'src/app.ts', changeKind: 'modified' }]
    })
    const events: RuntimeEvent[] = [
      baseEvent({ kind: 'thread_created', seq: 1, title: 'P4A thread' }),
      baseEvent({ kind: 'turn_started', seq: 2, turnId: 'turn-1' }),
      baseEvent({
        kind: 'item_created',
        seq: 3,
        turnId: 'turn-1',
        itemId: 'item-user-1',
        item: {
          id: 'item-user-1',
          turnId: 'turn-1',
          threadId: 'thread-p4a',
          role: 'user',
          status: 'completed',
          createdAt: timestamp,
          finishedAt: timestamp,
          kind: 'user_message',
          text: 'first prompt'
        }
      }),
      baseEvent({
        kind: 'item_created',
        seq: 4,
        turnId: 'turn-1',
        itemId: 'item-assistant-1',
        item: {
          id: 'item-assistant-1',
          turnId: 'turn-1',
          threadId: 'thread-p4a',
          role: 'assistant',
          status: 'completed',
          createdAt: timestamp,
          finishedAt: timestamp,
          kind: 'assistant_text',
          text: 'first answer'
        }
      }),
      baseEvent({ kind: 'turn_completed', seq: 5, turnId: 'turn-1' }),
      baseEvent({ kind: 'checkpoint_captured', seq: 6, turnId: 'turn-2', checkpoint }),
      baseEvent({ kind: 'turn_started', seq: 7, turnId: 'turn-2' }),
      baseEvent({
        kind: 'item_created',
        seq: 8,
        turnId: 'turn-2',
        itemId: 'item-user-2',
        item: {
          id: 'item-user-2',
          turnId: 'turn-2',
          threadId: 'thread-p4a',
          role: 'user',
          status: 'completed',
          createdAt: timestamp,
          finishedAt: timestamp,
          kind: 'user_message',
          text: 'second prompt'
        }
      }),
      baseEvent({
        kind: 'item_created',
        seq: 9,
        turnId: 'turn-2',
        itemId: 'item-tool-2',
        item: {
          id: 'item-tool-2',
          turnId: 'turn-2',
          threadId: 'thread-p4a',
          role: 'tool',
          status: 'completed',
          createdAt: timestamp,
          finishedAt: timestamp,
          kind: 'tool_result',
          toolName: 'edit',
          callId: 'call-edit',
          toolKind: 'file_change',
          output: makePublicToolResultWithheldProjection({ status: 'completed' }),
          isError: false
        }
      }),
      baseEvent({ kind: 'turn_completed', seq: 10, turnId: 'turn-2' })
    ]

    const plan = planConversationOnlyRewindFromEvents(events, checkpoint.checkpointId)

    expect(plan.ok).toBe(true)
    if (!plan.ok) return
    expect(RuntimeEventSchema.parse(events[5])).toMatchObject({
      kind: 'checkpoint_captured',
      checkpoint: {
        schemaVersion: 1,
        checkpointId: checkpoint.checkpointId
      }
    })
    expect(events).toHaveLength(10)
    expect(plan.retainedEvents.map((event) => event.seq)).toEqual([1, 2, 3, 4, 5, 6])
    expect(plan.removedEvents.map((event) => event.seq)).toEqual([7, 8, 9, 10])
    expect(plan.removedTurnIds).toEqual(['turn-2'])
    expect(plan.projection.turns.map((turn) => turn.id)).toEqual(['turn-1'])
    expect(plan.projection.items.map((item) => item.id)).toEqual(['item-user-1', 'item-assistant-1'])
    expect(plan.projection.checkpoints).toEqual([checkpoint])
  })
})
