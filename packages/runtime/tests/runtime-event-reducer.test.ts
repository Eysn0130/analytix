import { describe, expect, it } from 'vitest'

import type { RuntimeEvent } from '../src/contracts/events.js'
import { emptyUsageSnapshot } from '../src/contracts/usage.js'
import { replayRuntimeEvents } from '../src/domain/runtime-event-reducer.js'

const timestamp = '2026-06-03T10:00:00.000Z'

function baseEvent(overrides: Partial<RuntimeEvent> & Pick<RuntimeEvent, 'kind'>): RuntimeEvent {
  return {
    seq: overrides.seq ?? 1,
    timestamp: overrides.timestamp ?? timestamp,
    threadId: overrides.threadId ?? 'thread-1',
    ...overrides
  } as RuntimeEvent
}

describe('runtime event reducer', () => {
  it('replays thread, turn, and streaming text item state', () => {
    const events: RuntimeEvent[] = [
      baseEvent({
        kind: 'thread_created',
        seq: 1,
        title: 'Session'
      }),
      baseEvent({
        kind: 'turn_started',
        seq: 2,
        turnId: 'turn-1'
      }),
      baseEvent({
        kind: 'assistant_text_delta',
        seq: 3,
        turnId: 'turn-1',
        itemId: 'item-1',
        item: {
          id: 'item-1',
          turnId: 'turn-1',
          threadId: 'thread-1',
          role: 'assistant',
          status: 'running',
          createdAt: timestamp,
          kind: 'assistant_text',
          text: 'Hel'
        }
      }),
      baseEvent({
        kind: 'assistant_text_delta',
        seq: 4,
        turnId: 'turn-1',
        itemId: 'item-1',
        item: {
          id: 'item-1',
          turnId: 'turn-1',
          threadId: 'thread-1',
          role: 'assistant',
          status: 'completed',
          createdAt: timestamp,
          finishedAt: timestamp,
          kind: 'assistant_text',
          text: 'lo'
        }
      }),
      baseEvent({
        kind: 'turn_completed',
        seq: 5,
        turnId: 'turn-1'
      })
    ]

    const projection = replayRuntimeEvents(events)

    expect(projection.title).toBe('Session')
    expect(projection.lastSeq).toBe(5)
    expect(projection.turns).toEqual([
      expect.objectContaining({
        id: 'turn-1',
        status: 'completed',
        itemIds: ['item-1']
      })
    ])
    expect(projection.items).toEqual([
      expect.objectContaining({
        id: 'item-1',
        kind: 'assistant_text',
        text: 'Hello',
        status: 'completed'
      })
    ])
  })

  it('drops legacy reasoning deltas while advancing the cursor and replaying public text', () => {
    const projection = replayRuntimeEvents([
      baseEvent({
        kind: 'turn_started',
        seq: 1,
        turnId: 'turn-legacy'
      }),
      ({
        kind: 'assistant_reasoning_delta',
        seq: 2,
        timestamp,
        threadId: 'thread-1',
        turnId: 'turn-legacy',
        text: 'thinking '
      } as unknown as RuntimeEvent),
      baseEvent({
        kind: 'assistant_text_delta',
        seq: 3,
        turnId: 'turn-legacy',
        text: 'hello'
      } as Partial<RuntimeEvent> & Pick<RuntimeEvent, 'kind'>),
      baseEvent({
        kind: 'assistant_text_delta',
        seq: 4,
        turnId: 'turn-legacy',
        text: ' world'
      } as Partial<RuntimeEvent> & Pick<RuntimeEvent, 'kind'>)
    ])

    expect(projection.items).toEqual([
      expect.objectContaining({
        id: 'item_turn-legacy_assistant',
        kind: 'assistant_text',
        text: 'hello world'
      })
    ])
    expect(projection.turns).toEqual([
      expect.objectContaining({
        id: 'turn-legacy',
        itemIds: ['item_turn-legacy_assistant']
      })
    ])
    expect(projection.lastSeq).toBe(4)
    expect(JSON.stringify(projection)).not.toMatch(/thinking|assistant_reasoning/)
  })

  it('projects compactions and accumulates cache savings from usage events', () => {
    const usage = emptyUsageSnapshot()
    usage.promptTokens = 10
    usage.completionTokens = 2
    usage.totalTokens = 12
    usage.cacheSavingsUsd = 0.01
    usage.cacheSavingsCny = 0.07
    usage.tokenEconomySavingsTokens = 2048
    usage.tokenEconomySavingsUsd = 0.0009
    usage.tokenEconomySavingsCny = 0.0063

    const projection = replayRuntimeEvents([
      baseEvent({
        kind: 'usage',
        seq: 1,
        usage
      }),
      baseEvent({
        kind: 'compaction_completed',
        seq: 2,
        turnId: 'turn-1',
        itemId: 'compact-1',
        summary: 'Kept the core instructions.',
        replacedTokens: 200,
        pinnedConstraints: ['Active Skill: test']
      })
    ])

    expect(projection.usage.totalTokens).toBe(12)
    expect(projection.usage.cacheSavingsUsd).toBeCloseTo(0.01)
    expect(projection.usage.cacheSavingsCny).toBeCloseTo(0.07)
    expect(projection.usage.tokenEconomySavingsTokens).toBe(2048)
    expect(projection.usage.tokenEconomySavingsUsd).toBeCloseTo(0.0009)
    expect(projection.usage.tokenEconomySavingsCny).toBeCloseTo(0.0063)
    expect(projection.compactions).toEqual([
      expect.objectContaining({
        itemId: 'compact-1',
        replacedTokens: 200,
        pinnedConstraints: ['Active Skill: test']
      })
    ])
    expect(projection.items).toEqual([
      expect.objectContaining({
        id: 'compact-1',
        kind: 'compaction',
        summary: 'Kept the core instructions.',
        replacedTokens: 200
      })
    ])
  })

  it('keeps child run lifecycle separate from the parent turn', () => {
    const projection = replayRuntimeEvents([
      baseEvent({
        kind: 'turn_started',
        seq: 1,
        turnId: 'parent-turn'
      }),
      baseEvent({
        kind: 'turn_started',
        seq: 2,
        turnId: 'child-turn',
        child: {
          parentThreadId: 'thread-1',
          parentTurnId: 'parent-turn',
          parentToolCallId: 'call_task',
          childId: 'child-1',
          childRunId: 'child-1',
          childThreadId: 'thread-child',
          childTurnId: 'turn-child',
          childLabel: 'analysis',
          childStatus: 'running',
          childEffort: 'auto',
          background: true,
          parallelGroupId: 'parallel-1',
          parallelIndex: 1
        }
      }),
      baseEvent({
        kind: 'turn_completed',
        seq: 3,
        turnId: 'child-turn',
        child: {
          parentThreadId: 'thread-1',
          parentTurnId: 'parent-turn',
          parentToolCallId: 'call_task',
          childId: 'child-1',
          childRunId: 'child-1',
          childThreadId: 'thread-child',
          childTurnId: 'turn-child',
          childLabel: 'analysis',
          childStatus: 'completed',
          childSeq: 2,
          childEffort: 'auto',
          background: true,
          parallelGroupId: 'parallel-1',
          parallelIndex: 1,
          evidenceLedgered: true,
          toolInvocations: 3
        }
      })
    ])

    expect(projection.turns).toEqual([
      expect.objectContaining({
        id: 'parent-turn',
        status: 'running'
      })
    ])
    expect(projection.childRuns).toEqual([
      expect.objectContaining({
        childId: 'child-1',
        childRunId: 'child-1',
        parentToolCallId: 'call_task',
        childThreadId: 'thread-child',
        childTurnId: 'turn-child',
        label: 'analysis',
        status: 'completed',
        seq: 2,
        effort: 'auto',
        background: true,
        parallelGroupId: 'parallel-1',
        parallelIndex: 1,
        evidenceLedgered: true,
        toolInvocations: 3
      })
    ])
  })

  it('replays child lifecycle by stable run identity when transient child ids change', () => {
    const projection = replayRuntimeEvents([
      baseEvent({
        kind: 'turn_started',
        seq: 1,
        turnId: 'child-turn-start',
        child: {
          parentThreadId: 'thread-1',
          parentTurnId: 'parent-turn',
          parentToolCallId: 'call_task',
          childId: 'transient-child-start',
          childRunId: 'run-stable',
          childThreadId: 'thread-child',
          childStatus: 'running'
        }
      }),
      baseEvent({
        kind: 'turn_completed',
        seq: 2,
        turnId: 'child-turn-done',
        child: {
          parentThreadId: 'thread-1',
          parentTurnId: 'parent-turn',
          parentToolCallId: 'call_task',
          childId: 'transient-child-done',
          childRunId: 'run-stable',
          childThreadId: 'thread-child',
          childStatus: 'completed'
        }
      })
    ])

    expect(projection.childRuns).toEqual([
      expect.objectContaining({
        childId: 'transient-child-done',
        childRunId: 'run-stable',
        childThreadId: 'thread-child',
        status: 'completed'
      })
    ])
  })

  it('does not merge distinct child runs that share the same parent tool call', () => {
    const projection = replayRuntimeEvents([
      baseEvent({
        kind: 'turn_started',
        seq: 1,
        turnId: 'child-turn-a',
        child: {
          parentThreadId: 'thread-1',
          parentTurnId: 'parent-turn',
          parentToolCallId: 'call_parallel',
          childId: 'child-a',
          childRunId: 'run-a',
          childThreadId: 'thread-child-a',
          childStatus: 'running'
        }
      }),
      baseEvent({
        kind: 'turn_started',
        seq: 2,
        turnId: 'child-turn-b',
        child: {
          parentThreadId: 'thread-1',
          parentTurnId: 'parent-turn',
          parentToolCallId: 'call_parallel',
          childId: 'child-b',
          childRunId: 'run-b',
          childThreadId: 'thread-child-b',
          childStatus: 'running'
        }
      })
    ])

    expect(projection.childRuns.map((run) => run.childRunId)).toEqual(['run-a', 'run-b'])
  })

  it('keeps goal evidence audits as audit-only events', () => {
    const turnStarted = baseEvent({
      kind: 'turn_started',
      seq: 1,
      turnId: 'turn-goal'
    })
    const withoutAudit = replayRuntimeEvents([turnStarted])
    const withAudit = replayRuntimeEvents([
      turnStarted,
      baseEvent({
        kind: 'goal_evidence_audit',
        seq: 2,
        turnId: 'turn-goal',
        schemaVersion: 1,
        changeId: 'goal-evidence-audit',
        runtimeContract: 'analytix-go-runtime',
        upstreamSource: 'reasonix-absorbed',
        goalId: 'goal-thread-1',
        result: 'blocked',
        recovered: false,
        missingProjectChecks: 1,
        incompleteTodos: 0,
        commandMismatchMissing: 1,
        latestWriterReceiptIndex: 1,
        blockedStateKey: 'missingprojectchecks:runtime-go-unit:incompletetodos:none',
        missingCheckIds: ['runtime-go-unit'],
        missingCommands: ['go test ./...'],
        usesReasonixPublicProtocol: false,
        usesReasonixConfigRoot: false,
        changesRendererContract: false,
        changesProductIdentity: false
      })
    ])

    expect(withAudit.lastSeq).toBe(2)
    expect(withAudit.turns).toEqual(withoutAudit.turns)
    expect(withAudit.items).toEqual(withoutAudit.items)
    expect(withAudit.usage).toEqual(withoutAudit.usage)
    expect(withAudit.toolCatalog).toEqual(withoutAudit.toolCatalog)
    expect(withAudit.childRuns).toEqual(withoutAudit.childRuns)
  })

  it('keeps AutoResearch state audits as audit-only events', () => {
    const turnStarted = baseEvent({
      kind: 'turn_started',
      seq: 1,
      turnId: 'turn-research'
    })
    const withoutAudit = replayRuntimeEvents([turnStarted])
    const withAudit = replayRuntimeEvents([
      turnStarted,
      baseEvent({
        kind: 'autoresearch_state_audit',
        seq: 2,
        turnId: 'turn-research',
        schemaVersion: 1,
        changeId: 'autoresearch-state-audit',
        runtimeContract: 'analytix-go-runtime',
        upstreamSource: 'reasonix-absorbed',
        goalMode: 'research',
        stateRelativePath: '.analytix/autoresearch/thread-1',
        taskSpecPath: '.analytix/autoresearch/thread-1/task_spec.md',
        progressPath: '.analytix/autoresearch/thread-1/progress.json',
        findingsPath: '.analytix/autoresearch/thread-1/findings.jsonl',
        directionsTriedPath: '.analytix/autoresearch/thread-1/directions_tried.json',
        iterationLogPath: '.analytix/autoresearch/thread-1/iteration_log.jsonl',
        requiredFiles: [
          'task_spec.md',
          'progress.json',
          'findings.jsonl',
          'directions_tried.json',
          'iteration_log.jsonl'
        ],
        fileCount: 5,
        requirementCount: 1,
        completedRequirementCount: 0,
        staleRequirementCount: 1,
        staleDirectionCount: 1,
        complete: false,
        pivotRequired: true,
        result: 'pivot_required',
        unknownRequirementAccepted: false,
        findingsWrittenForUnknownRequirement: false,
        writesReasonixFile: false,
        writesAgentsFile: false,
        stablePrefixContainsState: false,
        toolSchemaContainsState: false,
        topLevelAutoResearchRouteExposed: false,
        usesReasonixPublicProtocol: false,
        usesReasonixConfigRoot: false,
        changesRendererContract: false,
        changesProductIdentity: false
      })
    ])

    expect(withAudit.lastSeq).toBe(2)
    expect(withAudit.turns).toEqual(withoutAudit.turns)
    expect(withAudit.items).toEqual(withoutAudit.items)
    expect(withAudit.usage).toEqual(withoutAudit.usage)
    expect(withAudit.toolCatalog).toEqual(withoutAudit.toolCatalog)
    expect(withAudit.childRuns).toEqual(withoutAudit.childRuns)
  })

  it('records tool catalog drift and error items', () => {
    const projection = replayRuntimeEvents([
      baseEvent({
        kind: 'tool_catalog_changed',
        seq: 1,
        fingerprint: 'fp-2',
        toolCount: 2,
        toolNames: ['read', 'edit'],
        message: 'Catalog changed'
      }),
      baseEvent({
        kind: 'error',
        seq: 2,
        turnId: 'turn-1',
        itemId: 'error-1',
        message: 'Budget limit reached',
        code: 'budget_limited',
        details: { spent: 2, budget: 1 },
        severity: 'error'
      })
    ])

    expect(projection.toolCatalog).toEqual({
      fingerprint: 'fp-2',
      toolCount: 2,
      toolNames: ['read', 'edit'],
      message: 'Catalog changed'
    })
    expect(projection.errors).toEqual([
      expect.objectContaining({
        seq: 2,
        turnId: 'turn-1',
        itemId: 'error-1',
        message: 'Budget limit reached',
        code: 'budget_limited',
        details: { spent: 2, budget: 1 },
        severity: 'error'
      })
    ])
    expect(projection.items).toEqual([
      expect.objectContaining({
        id: 'error-1',
        kind: 'error',
        status: 'failed',
        message: 'Budget limit reached',
        code: 'budget_limited',
        details: { spent: 2, budget: 1 },
        severity: 'error'
      })
    ])
  })
})
