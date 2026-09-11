import { describe, expect, it } from 'vitest'
import type { CheckpointMetadata } from '../src/contracts/checkpoints.js'
import type { RuntimeEvent } from '../src/contracts/events.js'
import {
  buildAuditableCheckpointRewindPlan,
  createAnalytixCheckpointMetadata
} from '../src/domain/checkpoint-rewind-contract.js'
import { makePublicToolResultWithheldProjection } from '../src/domain/item.js'

const timestamp = '2026-06-20T10:00:00.000Z'
const workspace = '/tmp/analytix-p4b-workspace'

function baseEvent(overrides: Partial<RuntimeEvent> & Pick<RuntimeEvent, 'kind'>): RuntimeEvent {
  return {
    seq: overrides.seq ?? 1,
    timestamp: overrides.timestamp ?? timestamp,
    threadId: overrides.threadId ?? 'thread-p4b',
    ...overrides
  } as RuntimeEvent
}

function checkpointWithFiles(changedFiles: CheckpointMetadata['changedFiles']): CheckpointMetadata {
  return createAnalytixCheckpointMetadata({
    workspace,
    threadId: 'thread-p4b',
    turnId: 'turn-2',
    createdAt: timestamp,
    changedFiles
  })
}

function eventsForCheckpoint(
  checkpoint: CheckpointMetadata
): RuntimeEvent[] {
  return [
    baseEvent({ kind: 'thread_created', seq: 1, title: 'P4B thread' }),
    baseEvent({ kind: 'turn_started', seq: 2, turnId: 'turn-1' }),
    baseEvent({
      kind: 'item_created',
      seq: 3,
      turnId: 'turn-1',
      itemId: 'item-user-1',
      item: {
        id: 'item-user-1',
        turnId: 'turn-1',
        threadId: 'thread-p4b',
        role: 'user',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'user_message',
        text: 'first prompt'
      }
    }),
    baseEvent({ kind: 'turn_completed', seq: 4, turnId: 'turn-1' }),
    baseEvent({ kind: 'checkpoint_captured', seq: 5, turnId: 'turn-2', checkpoint }),
    baseEvent({ kind: 'turn_started', seq: 6, turnId: 'turn-2' }),
    baseEvent({
      kind: 'item_created',
      seq: 7,
      turnId: 'turn-2',
      itemId: 'item-user-2',
      item: {
        id: 'item-user-2',
        turnId: 'turn-2',
        threadId: 'thread-p4b',
        role: 'user',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'user_message',
        text: 'second prompt with raw secret SUPER_SECRET_PROMPT'
      }
    }),
    baseEvent({
      kind: 'item_created',
      seq: 8,
      turnId: 'turn-2',
      itemId: 'item-tool-2',
      item: {
        id: 'item-tool-2',
        turnId: 'turn-2',
        threadId: 'thread-p4b',
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
    baseEvent({ kind: 'turn_completed', seq: 9, turnId: 'turn-2' })
  ]
}

describe('P4B auditable checkpoint rewind plan', () => {
  it('creates a code-only restore plan from checkpoint changed files', () => {
    const checkpoint = checkpointWithFiles([
      {
        relativePath: 'src/app.ts',
        changeKind: 'modified',
        beforeHash: 'sha256:before-app',
        afterHash: 'sha256:after-app'
      },
      {
        relativePath: 'docs/new.md',
        changeKind: 'created',
        afterHash: 'sha256:new-doc'
      },
      {
        relativePath: 'src/removed.ts',
        changeKind: 'deleted',
        beforeHash: 'sha256:removed-before'
      }
    ])

    const result = buildAuditableCheckpointRewindPlan({
      events: eventsForCheckpoint(checkpoint),
      checkpointId: checkpoint.checkpointId,
      scope: 'code',
      createdAt: timestamp,
      pathRisks: new Map([
        ['src/app.ts', { exists: true }],
        ['docs/new.md', { exists: true }],
        ['src/removed.ts', { exists: false }]
      ])
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.plan.planId).toMatch(/^axrp_/)
    expect(result.plan.planDigest).toMatch(/^[a-f0-9]{64}$/)
    expect(result.plan.applyMode).toBe('plan_only')
    expect(result.plan.destructive).toBe(false)
    expect(result.plan.conversation).toBeUndefined()
    expect(result.plan.files.map((file) => [file.relativePath, file.action, file.status])).toEqual([
      ['src/app.ts', 'restore_previous_version', 'ready'],
      ['docs/new.md', 'delete_created_file', 'ready'],
      ['src/removed.ts', 'restore_deleted_file', 'ready']
    ])

    const same = buildAuditableCheckpointRewindPlan({
      events: eventsForCheckpoint(checkpoint),
      checkpointId: checkpoint.checkpointId,
      scope: 'code',
      createdAt: timestamp,
      pathRisks: new Map([
        ['src/app.ts', { exists: true }],
        ['docs/new.md', { exists: true }],
        ['src/removed.ts', { exists: false }]
      ])
    })
    expect(same.ok).toBe(true)
    if (same.ok) expect(same.plan.planDigest).toBe(result.plan.planDigest)
  })

  it('keeps conversation-only rewind based on event projection', () => {
    const checkpoint = checkpointWithFiles([
      { relativePath: 'src/app.ts', changeKind: 'modified', beforeHash: 'sha256:before-app' }
    ])

    const result = buildAuditableCheckpointRewindPlan({
      events: eventsForCheckpoint(checkpoint),
      checkpointId: checkpoint.checkpointId,
      scope: 'conversation',
      createdAt: timestamp
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.plan.files).toEqual([])
    expect(result.plan.conversation).toMatchObject({
      status: 'ready',
      boundaryTurnId: 'turn-2',
      retainedEventCount: 5,
      removedEventCount: 4,
      removedTurnIds: ['turn-2'],
      projection: {
        latestSeq: 5,
        turnCount: 1,
        itemCount: 1,
        checkpointCount: 1
      }
    })
  })

  it('creates a combined plan without mutating files or event arrays', () => {
    const checkpoint = checkpointWithFiles([
      {
        relativePath: 'src/app.ts',
        changeKind: 'modified',
        beforeHash: 'sha256:before-app',
        afterHash: 'sha256:after-app'
      }
    ])
    const events = eventsForCheckpoint(checkpoint)
    const beforeEvents = JSON.stringify(events)

    const result = buildAuditableCheckpointRewindPlan({
      events,
      checkpointId: checkpoint.checkpointId,
      scope: 'combined',
      createdAt: timestamp,
      pathRisks: new Map([['src/app.ts', { exists: true }]])
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(JSON.stringify(events)).toBe(beforeEvents)
    expect(result.plan.scope).toBe('combined')
    expect(result.plan.applyMode).toBe('plan_only')
    expect(result.plan.destructive).toBe(false)
    expect(result.plan.files).toHaveLength(1)
    expect(result.plan.conversation?.removedTurnIds).toEqual(['turn-2'])
  })

  it('handles path escape, absolute path, and symlink risks with safety-first statuses', () => {
    const checkpoint: CheckpointMetadata = {
      schemaVersion: 1,
      checkpointId: 'axcp_manual_risk',
      threadId: 'thread-p4b',
      turnId: 'turn-2',
      workspace,
      createdAt: timestamp,
      status: 'captured',
      changedFiles: [
        { relativePath: '../outside.ts', changeKind: 'modified', beforeHash: 'sha256:outside' },
        { relativePath: '/tmp/outside.ts', changeKind: 'modified', beforeHash: 'sha256:absolute' },
        { relativePath: 'src/link.ts', changeKind: 'modified', beforeHash: 'sha256:link' },
        { relativePath: '..name', changeKind: 'modified', beforeHash: 'sha256:dot-name' }
      ]
    }

    const result = buildAuditableCheckpointRewindPlan({
      events: eventsForCheckpoint(checkpoint),
      checkpointId: checkpoint.checkpointId,
      scope: 'code',
      createdAt: timestamp,
      pathRisks: new Map([['src/link.ts', { exists: true, isSymlink: true }]])
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.plan.files.map((file) => [file.relativePath, file.action, file.status, file.risk])).toEqual([
      ['../outside.ts', 'blocked', 'blocked', 'high'],
      ['/tmp/outside.ts', 'blocked', 'blocked', 'high'],
      ['src/link.ts', 'blocked', 'blocked', 'high'],
      ['..name', 'restore_previous_version', 'ready', 'medium']
    ])
  })

  it('does not include raw prompts, secrets, or full file content in restore plans', () => {
    const checkpoint = checkpointWithFiles([
      {
        relativePath: 'src/secret.ts',
        changeKind: 'modified',
        beforeHash: 'sha256:before-secret',
        afterHash: 'sha256:after-secret'
      }
    ])
    const result = buildAuditableCheckpointRewindPlan({
      events: eventsForCheckpoint(checkpoint),
      checkpointId: checkpoint.checkpointId,
      scope: 'combined',
      createdAt: timestamp,
      pathRisks: new Map([['src/secret.ts', { exists: true }]])
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    const serialized = JSON.stringify(result.plan)
    expect(result.plan.summary).toMatchObject({
      containsRawPrompt: false,
      containsFullFileContent: false,
      containsSecretValue: false
    })
    expect(serialized).not.toContain('SUPER_SECRET_PROMPT')
    expect(serialized).not.toContain('SUPER_SECRET_FILE_CONTENT')
    expect(serialized).not.toContain('export const token')
  })
})
