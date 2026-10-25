import { describe, expect, it, vi } from 'vitest'
import {
  chatBlockFromItem,
  projectToolEventForRenderer,
  createOrdinaryHistoryReplay,
  isOrdinaryDisplayHistoryBlock,
  dispatchAnalytixRuntimeEvent,
  dispatchAnalytixRuntimeEvents,
  generalTerminalProjectionBatchFromRuntime,
  mergeChatBlocks,
  mergeTurnUsageById,
  reconcileTurnUsageById,
  turnUsageEntryFromBatch,
  filterRewoundDisplayBlocks,
  RUNTIME_EVENT_KINDS_COVERED_BY_RENDERER,
  threadFromCore,
  usageFromCore
} from './analytix-mapper'
import { RuntimeEventKind } from '../../../../packages/runtime/src/contracts/events'
import { PublicToolResultProjectionV1, TurnItem } from '../../../../packages/runtime/src/contracts/items'
import { CORE_RUNTIME_EVENT_KINDS } from './analytix-contract'
import type { CoreRuntimeEventJson, CoreTurnItemJson, CoreChildRuntimeMetadataJson } from './analytix-contract'
import { taskJobSummaryV1 } from '../../../../packages/runtime/src/contracts/task-job-output'
import type { ChatBlock, CompactionEventPayload, ThreadErrorOptions, ThreadEventSink } from './types'

function makeSink(): ThreadEventSink {
  return {
    onSeq: () => undefined,
    onDeltas: () => undefined,
    onUserMessage: () => undefined,
    onTool: () => undefined,
    onCompaction: () => undefined,
    onApproval: () => undefined,
    onUserInput: () => undefined,
    onUserInputStatus: () => undefined,
    onGoal: () => undefined,
    onTodos: () => undefined,
    onTurnComplete: () => undefined,
    onError: () => undefined
  }
}

function generalTerminalBatch(): Record<string, unknown> {
  const timestamp = '2026-07-20T03:00:00Z'
  const events = [
    {
      kind: 'item_completed', seq: 11, timestamp, threadId: 'thr_general', turnId: 'turn_general',
      itemId: 'item_general',
      item: {
        id: 'item_general', turnId: 'turn_general', threadId: 'thr_general', role: 'assistant',
        status: 'completed', createdAt: timestamp, finishedAt: timestamp,
        kind: 'assistant_text',
        text: '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。'
      }
    },
    {
      kind: 'usage', seq: 12, timestamp, threadId: 'thr_general', turnId: 'turn_general',
      model: 'gpt-5', usage: {
        promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
        cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
        cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
        priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0, turns: 1
      }, cacheDiagnostics: {}, usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: 13, timestamp, threadId: 'thr_general', turnId: 'turn_general',
      status: 'completed', terminalReason: 'success'
    }
  ]
  return {
    schemaVersion: 1, purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch', batchDigest: 'a'.repeat(64),
    threadId: 'thr_general', turnId: 'turn_general', seq: 13, firstSeq: 11, lastSeq: 13,
    timestamp, generalTerminalCommitId: 'b'.repeat(64),
    generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: 'c'.repeat(64), eventManifestDigest: 'd'.repeat(64),
    projectedEventsDigest: 'e'.repeat(64), transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
    events,
    eventManifest: [
      { slot: 'terminal-item', eventId: 'f'.repeat(64), payloadDigest: '1'.repeat(64) },
      { slot: 'usage', eventId: '2'.repeat(64), payloadDigest: '3'.repeat(64) },
      { slot: 'terminal', eventId: '4'.repeat(64), payloadDigest: '5'.repeat(64) }
    ]
  }
}

function typedGeneralTerminalBatch(
  text = '已完成代码修改并通过相关测试。'
): Record<string, unknown> {
  const batch = generalTerminalBatch()
  const item = ((batch.events as Array<Record<string, unknown>>)[0].item as Record<string, unknown>)
  item.text = text
  item.ordinaryResult = {
    schemaVersion: 1,
    purpose: 'analytix.ordinary-result/v1',
    projectionVersion: 'analytix.ordinary-output-projection/v1',
    logicalEffect: 'ordinary',
    ordinaryWork: true,
    candidateOrigin: 'provider_ordinary_only',
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    text,
    textSha256: '6'.repeat(64),
    resultDigest: '7'.repeat(64)
  }
  return batch
}

function hostToolProjection(
  status: 'completed' | 'failed' | 'blocked' | 'cancelled' | 'unknown' = 'completed',
  code = status === 'completed' ? 'tool_completed' : 'tool_failed'
) {
  return {
    schemaVersion: 1 as const,
    projectionKind: 'host_status' as const,
    disclosure: 'metadata_only' as const,
    status,
    code,
    messageKey: status === 'completed' ? 'tool_completed' as const : 'tool_failed' as const,
    privatePayloadWithheld: true as const,
    factAnswerAllowed: false as const,
    evidenceAuthority: false as const
  }
}

function planToolProjection(operation: 'draft' | 'refine' = 'draft') {
  return {
    schemaVersion: 1 as const,
    projectionKind: 'plan_status' as const,
    disclosure: 'metadata_only' as const,
    status: 'completed' as const,
    code: 'plan_updated',
    messageKey: 'plan_updated' as const,
    privatePayloadWithheld: true as const,
    factAnswerAllowed: false as const,
    evidenceAuthority: false as const,
    plan: {
      planId: operation === 'draft' ? 'plan_login' : 'plan_x',
      relativePath: operation === 'draft' ? '.analytix/plan/login.md' : '.analytix/plan/x.md',
      operation,
      contentHash: 'd'.repeat(64),
      byteSize: 42,
      savedAt: '2024-01-01T00:00:01.000Z'
    }
  }
}

describe('runtime event wire coverage', () => {
  it('keeps renderer runtime event kinds aligned with the runtime schema', () => {
    expect([...CORE_RUNTIME_EVENT_KINDS].sort()).toEqual([...RuntimeEventKind.options].sort())
  })

  it('explicitly dispatches or ignores every runtime event kind', () => {
    const covered = new Set(RUNTIME_EVENT_KINDS_COVERED_BY_RENDERER)
    expect(covered.size).toBe(RUNTIME_EVENT_KINDS_COVERED_BY_RENDERER.length)
    expect([...covered].sort()).toEqual([...RuntimeEventKind.options].sort())
  })
})

describe('thread summary mapping', () => {
  it.each([undefined, 0, 1, 2, 3])('preserves only the real supported policy marker, source=%s', executionPolicyVersion => {
    const thread = threadFromCore({ id: 'thr_policy', title: 'Policy', workspace: '/workspace', model: '',
      mode: 'agent', status: 'idle', executionPolicyVersion, approvalPolicy: 'untrusted', sandboxMode: 'danger-full-access',
      createdAt: '2026-10-20T00:00:00Z', updatedAt: '2026-10-20T00:00:00Z' })
    expect(thread.executionPolicyVersion).toBe(executionPolicyVersion === 2 ? 2 : undefined)
    expect(thread.approvalPolicy).toBe('untrusted'); expect(thread.sandboxMode).toBe('danger-full-access')
    expect(thread.executionPolicyUncertain).toBe(false)
  })

  it('keeps analytix sidecar preview and counts backend-neutral', () => {
    const thread = threadFromCore({
      id: 'thr_summary',
      title: 'Sidecar summary',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      providerId: 'zai-coding-plan',
      mode: 'agent',
      status: 'idle',
      relation: 'primary',
      preview: 'cached preview line',
      turnCount: 3,
      messageCount: 7,
      latestTurnId: 'turn-3',
      createdAt: '2026-06-04T00:00:00.000Z',
      updatedAt: '2026-06-04T00:00:01.000Z'
    })

    expect(thread.preview).toBe('cached preview line')
    expect(thread.providerId).toBe('zai-coding-plan')
    expect(thread.turnCount).toBe(3)
    expect(thread.messageCount).toBe(7)
    expect(thread.latestTurnId).toBe('turn-3')
  })

  it('projects complete accounts from ordinary thread summary surfaces', () => {
    const account = '6222020202020202020'
    const thread = threadFromCore({
      id: 'thr_summary_account',
      title: `案件账号 ${account}`,
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      preview: `银行卡号 ${account}`,
      createdAt: '2026-06-04T00:00:00.000Z',
      updatedAt: '2026-06-04T00:00:01.000Z'
    })

    expect(thread.title).toContain('[ACCOUNT]')
    expect(thread.preview).toContain('[ACCOUNT]')
    expect(JSON.stringify(thread)).not.toContain(account)
  })

  it('normalizes pending auto-title sentinels away from thread summaries', () => {
    const thread = threadFromCore({
      id: 'thr_pending_title',
      title: '__analytix_pending_title__',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      createdAt: '2026-06-04T00:00:00.000Z',
      updatedAt: '2026-06-04T00:00:01.000Z'
    })

    expect(thread.title).not.toContain('__analytix')
    expect(thread.title.trim()).not.toBe('')
  })

  it('preserves goal blocked audit fields from thread summaries', () => {
    const thread = threadFromCore({
      id: 'thr_goal_audit',
      title: 'Goal audit',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      goal: {
        threadId: 'thr_goal_audit',
        objective: 'finish the governed task',
        status: 'active',
        tokensUsed: 0,
        timeUsedSeconds: 0,
        blockedReason: 'Need API access',
        blockedCount: 2,
        blockedTurnId: 'turn_2',
        strictCompletion: true,
        selfCheckRequired: true,
        selfCheckCompleted: true,
        selfCheckTurnId: 'turn_3',
        createdAt: '2026-06-04T00:00:00.000Z',
        updatedAt: '2026-06-04T00:00:01.000Z'
      },
      createdAt: '2026-06-04T00:00:00.000Z',
      updatedAt: '2026-06-04T00:00:01.000Z'
    })

    expect(thread.goal).toMatchObject({
      blockedReason: 'Need API access',
      blockedCount: 2,
      blockedTurnId: 'turn_2',
      strictCompletion: true,
      selfCheckRequired: true,
      selfCheckCompleted: true,
      selfCheckTurnId: 'turn_3'
    })
  })
})

describe('thread lifecycle event mapping', () => {
  it('surfaces turn_steered admission metadata through the event sink', async () => {
    const events: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTurnSteered: (event) => {
        events.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'turn_steered',
      threadId: 'thr_1',
      turnId: 'turn_1',
      seq: 6,
      timestamp: '2026-07-01T00:00:00.000Z',
      text: 'follow up',
      clientUserMessageId: 'client_steer'
    }, sink, async () => undefined)

    expect(events).toEqual([{
      turnId: 'turn_1',
      createdAt: '2026-07-01T00:00:00.000Z',
      text: 'follow up',
      clientUserMessageId: 'client_steer',
      admittedSeq: 6
    }])
  })

  it('surfaces child steer events as subagent metadata without transcript text', async () => {
    const tools: unknown[] = []
    const statuses: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        tools.push(event)
      },
      onRuntimeStatus: (event) => {
        statuses.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'child_steer_queued',
      threadId: 'thr_parent',
      turnId: 'turn_parent',
      seq: 7,
      timestamp: '2026-07-01T00:00:01.000Z',
      status: 'queued',
      jobId: 'job_1',
      childRunId: 'job_1',
      childThreadId: 'thr_child',
      childTurnId: 'turn_child',
      steerMessageId: 'steer_1',
      parentThreadId: 'thr_parent',
      child: {
        parentThreadId: 'thr_parent',
        parentTurnId: 'turn_parent',
        parentToolCallId: 'call_parent',
        childId: 'job_1',
        childRunId: 'job_1',
        jobId: 'job_1',
        childThreadId: 'thr_child',
        childTurnId: 'turn_child',
        childLabel: 'research',
        childStatus: 'running',
        background: true,
        canAcceptSteer: true,
        pendingSteers: 1,
        admittedSteers: 0,
        steerCount: 1,
        steerMessageId: 'steer_1',
        steerStatus: 'queued'
      }
    }, sink, async () => undefined)

    expect(statuses).toHaveLength(1)
    expect(tools).toHaveLength(1)
    expect(tools[0]).toMatchObject({
      itemId: 'subagent_job_1',
      status: 'running',
      toolKind: 'subagent',
      meta: {
        runtimeStatus: 'child_steer_queued',
        child: {
          jobId: 'job_1',
          childRunId: 'job_1',
          steerMessageId: 'steer_1',
          steerStatus: 'queued',
          pendingSteers: 1,
          canAcceptSteer: true
        }
      }
    })
    expect(JSON.stringify(tools[0])).not.toContain('narrow the scope')
  })

  it('surfaces child pause events as subagent metadata', async () => {
    const tools: unknown[] = []
    const statuses: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        tools.push(event)
      },
      onRuntimeStatus: (event) => {
        statuses.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'child_paused',
      threadId: 'thr_parent',
      turnId: 'turn_parent',
      seq: 8,
      timestamp: '2026-07-06T00:00:01.000Z',
      status: 'paused',
      jobId: 'job_1',
      childRunId: 'job_1',
      childThreadId: 'thr_child',
      childTurnId: 'turn_child',
      pauseRequestId: 'pause_1',
      parentThreadId: 'thr_parent',
      child: {
        parentThreadId: 'thr_parent',
        parentTurnId: 'turn_parent',
        parentToolCallId: 'call_parent',
        childId: 'job_1',
        childRunId: 'job_1',
        jobId: 'job_1',
        childThreadId: 'thr_child',
        childTurnId: 'turn_child',
        childLabel: 'research',
        childStatus: 'paused',
        background: true,
        canPause: false,
        canResume: true,
        paused: true,
        pauseRequestId: 'pause_1',
        pauseStatus: 'paused'
      }
    }, sink, async () => undefined)

    expect(statuses).toHaveLength(1)
    expect(tools).toHaveLength(1)
    expect(tools[0]).toMatchObject({
      itemId: 'subagent_job_1',
      status: 'running',
      toolKind: 'subagent',
      meta: {
        runtimeStatus: 'child_paused',
        child: {
          jobId: 'job_1',
          childRunId: 'job_1',
          pauseRequestId: 'pause_1',
          pauseStatus: 'paused',
          canResume: true,
          paused: true
        }
      }
    })
  })

  it('surfaces thread title updates through the event sink', async () => {
    const events: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onThreadLifecycle: (event) => {
        events.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'thread_updated',
      threadId: 'thr_1',
      seq: 5,
      timestamp: '2026-06-04T00:00:02.000Z',
      title: '排查标题逻辑',
      status: 'running'
    }, sink, async () => undefined)

    expect(events).toEqual([
      {
        threadId: 'thr_1',
        title: '排查标题逻辑',
        status: 'running',
        createdAt: '2026-06-04T00:00:02.000Z',
        seq: 5
      }
    ])
  })

  it('surfaces thread rewind events through the event sink', async () => {
    const events: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onThreadRewound: (event) => {
        events.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'thread_rewound',
      threadId: 'thr_1',
      rewindTurnId: 'turn_2',
      removedTurns: 2,
      remainingTurns: 1,
      removedTurnIds: ['turn_2', 'turn_3'],
      seq: 8,
      timestamp: '2026-06-04T00:00:03.000Z'
    }, sink, async () => undefined)

    expect(events).toEqual([
      {
        threadId: 'thr_1',
        turnId: 'turn_2',
        removedTurns: 2,
        remainingTurns: 1,
        removedTurnIds: ['turn_2', 'turn_3'],
        seq: 8
      }
    ])
  })
})

describe('assistant draft isolation', () => {
  it('preserves true completed assistant ownership only after a committed general terminal batch', () => {
    const batch = generalTerminalBatch()
    const projection = generalTerminalProjectionBatchFromRuntime(batch)
    expect(projection?.terminalItem).toMatchObject({
      id: 'item_general', assistantItemIdentity: { threadId: 'thr_general', turnId: 'turn_general', itemId: 'item_general' }
    })
    const partial = chatBlockFromItem({ id: 'live_assistant_partial', threadId: 'thr_general', turnId: 'turn_general',
      role: 'assistant', status: 'running', createdAt: '2026-10-11T00:00:00Z', kind: 'assistant_text', text: 'partial' })
    expect(partial?.kind === 'assistant' ? partial.assistantItemIdentity : undefined).toBeUndefined()
  })
  it('does not treat a standalone V3 public view as accepted-final authority', () => {
    expect(chatBlockFromItem({
      id: 'item_final',
      turnId: 'turn_case',
      threadId: 'thr_case',
      role: 'assistant',
      status: 'completed',
      createdAt: '2026-07-11T00:00:01Z',
      finishedAt: '2026-07-11T00:00:01Z',
      kind: 'assistant_text',
      text: 'verified final',
      acceptedFinalView: {
        schemaVersion: 3,
        acceptedFinalDigest: 'f'.repeat(64),
        publicationState: 'accepted',
        variant: 'SourceUnavailableAnswer',
        terminalReason: 'source_unavailable',
        blockerCode: 'current_case_source_unavailable',
        coverageStatus: 'unavailable',
        checkedScopeDigest: '',
        missingScopeCount: 0,
        claimCount: 0,
        claimTypes: [],
        receiptMetadata: {
          projection: 'masked_metadata_only',
          count: 0,
          setDigest: '8'.repeat(64),
          citations: []
        },
        noHitWording: '',
        acceptedAt: '2026-07-11T00:00:01Z'
      }
    })).toBeNull()
  })

  it('does not emit assistant drafts or duplicate completed snapshots', async () => {
    const deltas: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onDeltas: (events) => {
        deltas.push(...events)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'assistant_text_delta',
      seq: 1,
      item: {
        id: 'item_answer',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'assistant',
        status: 'running',
        createdAt: '2024-01-01T00:00:00.000Z',
        kind: 'assistant_text',
        text: 'he'
      }
    }, sink, async () => undefined)
    await dispatchAnalytixRuntimeEvent({
      kind: 'assistant_text_delta',
      seq: 2,
      item: {
        id: 'item_answer',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'assistant',
        status: 'running',
        createdAt: '2024-01-01T00:00:00.000Z',
        kind: 'assistant_text',
        text: 'llo'
      }
    }, sink, async () => undefined)
    await dispatchAnalytixRuntimeEvent({
      kind: 'item_created',
      seq: 3,
      item: {
        id: 'item_answer',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'assistant',
        status: 'completed',
        createdAt: '2024-01-01T00:00:00.000Z',
        kind: 'assistant_text',
        text: 'hello'
      }
    }, sink, async () => undefined)

    expect(deltas).toEqual([])
  })

  it('does not accept typed runtime text deltas without accepted snapshots', async () => {
    const deltas: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onDeltas: (events) => {
        deltas.push(...events)
      }
    }

    await dispatchAnalytixRuntimeEvents([
      {
        kind: 'assistant_text_delta',
        seq: 4,
        text: 'visible'
      }
    ], sink, async () => undefined)

    expect(deltas).toEqual([])
  })
})

describe('user input event mapping', () => {
  it('maps non-submitted user input resolution statuses to error', async () => {
    const statuses: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUserInputStatus: (event) => {
        statuses.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'user_input_resolved',
      seq: 7,
      itemId: 'input_item',
      inputId: 'input_request',
      status: 'error',
      message: 'user input failed'
    }, sink, async () => undefined)

    expect(statuses).toEqual([{
      itemId: 'input_item',
      requestId: 'input_request',
      status: 'error',
      errorMessage: 'user input failed'
    }])
  })
})

describe('compaction mapping', () => {
  it('keeps one actual summary identity across live completion, failure and history item', async () => {
    const compactions: unknown[] = []
    const sink = { ...makeSink(), onCompaction: (event: unknown) => compactions.push(event) }
    await dispatchAnalytixRuntimeEvent({ kind: 'compaction_completed', threadId: 'thread', turnId: 'turn', itemId: 'compaction', compactionId: 'compaction',
      summaryKind: 'conversation', summary: 'Actual projected summary', shadowedItemCount: 7, replacedTokens: 234,
      tokenCountKind: 'estimated', tokenEstimatorVersion: 1, auto: false }, sink, async () => undefined)
    expect(compactions[0]).toMatchObject({ itemId: 'compaction', detail: 'Actual projected summary', summaryKind: 'conversation',
      meta: { shadowedItemCount: 7, replacedTokens: 234, tokenCountKind: 'estimated', tokenEstimatorVersion: 1 } })
    await dispatchAnalytixRuntimeEvent({ kind: 'compaction_failed', threadId: 'thread', turnId: 'turn', itemId: 'compaction', compactionId: 'compaction', auto: false }, sink, async () => undefined)
    expect(compactions[1]).toMatchObject({ itemId: 'compaction', status: 'error' })
    expect(compactions[1]).not.toHaveProperty('detail')
    expect(chatBlockFromItem({ id: 'compaction', threadId: 'thread', turnId: 'turn', kind: 'compaction', role: 'system', status: 'completed',
      createdAt: '2026-10-10T00:00:00Z', summaryKind: 'conversation', summary: 'Actual projected summary', shadowedItemCount: 7,
      replacedTokens: 234, tokenCountKind: 'estimated', tokenEstimatorVersion: 1 })).toMatchObject({ id: 'compaction', detail: 'Actual projected summary', meta: { shadowedItemCount: 7 } })
  })
  it('preserves manual compaction markers from runtime events and items', async () => {
    const compactions: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onCompaction: (event) => {
        compactions.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'compaction_completed',
      seq: 9,
      itemId: 'cmp_1',
      summary: 'Manual compact',
      auto: false
    }, sink, async () => undefined)

    expect(compactions).toEqual([expect.objectContaining({
      itemId: 'cmp_1',
      status: 'success',
      summary: 'Manual compact',
      auto: false
    })])

    expect(chatBlockFromItem({
      id: 'cmp_item',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'system',
      status: 'completed',
      createdAt: '2026-06-26T00:00:00.000Z',
      kind: 'compaction',
      summary: 'Manual compact item',
      replacedTokens: 120,
      pinnedConstraints: [],
      auto: false
    })).toMatchObject({
      kind: 'compaction',
      summary: 'Manual compact item',
      auto: false
    })

    await dispatchAnalytixRuntimeEvent({
      kind: 'item_created',
      seq: 10,
      item: {
        id: 'cmp_live_item',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'system',
        status: 'running',
        createdAt: '2026-06-26T00:00:01.000Z',
        kind: 'compaction',
        summary: 'Manual live compact item',
        replacedTokens: 64,
        pinnedConstraints: [],
        auto: false
      }
    }, sink, async () => undefined)

    expect(compactions[1]).toMatchObject({
      itemId: 'cmp_live_item',
      status: 'running',
      summary: 'Manual live compact item',
      auto: false
    })

    await dispatchAnalytixRuntimeEvent({
      kind: 'compaction_completed',
      seq: 11,
      itemId: 'cmp_legacy_missing_auto',
      summary: 'Missing provenance marker'
    }, sink, async () => undefined)
    expect(compactions[2]).toMatchObject({
      itemId: 'cmp_legacy_missing_auto',
      auto: false
    })

    expect(chatBlockFromItem({
      id: 'cmp_missing_auto_item',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'system',
      status: 'completed',
      createdAt: '2026-06-26T00:00:02.000Z',
      kind: 'compaction',
      summary: 'Missing provenance marker',
      replacedTokens: 1,
      pinnedConstraints: []
    })).toMatchObject({ kind: 'compaction', auto: false })

    await dispatchAnalytixRuntimeEvent({
      kind: 'compaction_started',
      seq: 12,
      threadId: 'thr_case_compaction',
      turnId: 'turn_case_compaction'
    }, sink, async () => undefined)
    await dispatchAnalytixRuntimeEvent({
      kind: 'compaction_completed',
      seq: 13,
      threadId: 'thr_case_compaction',
      turnId: 'turn_case_compaction',
      summary: 'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.',
      auto: false,
      pinnedConstraints: ['user: preserve recent turns'],
      sourceDigest: 'a'.repeat(64),
      digestMarker: `sha256:${'a'.repeat(12)}`,
      sourceItemIds: [],
      schemaVersion: 2,
      reasoningExcluded: true,
      reasoningExclusionProof: `sha256:${'b'.repeat(64)}`
    }, sink, async () => undefined)
    expect(compactions[3]).toMatchObject({
      itemId: 'compaction_turn_case_compaction',
      status: 'running'
    })
    expect(compactions[4]).toMatchObject({
      itemId: 'compaction_turn_case_compaction',
      status: 'success'
    })

    expect(chatBlockFromItem({
      id: 'cmp_case_item',
      turnId: 'turn_case_compaction',
      threadId: 'thr_case_compaction',
      role: 'system',
      status: 'completed',
      createdAt: '2026-08-02T00:00:00.000Z',
      finishedAt: '2026-08-02T00:00:00.000Z',
      kind: 'compaction',
      summary: 'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.',
      auto: false,
      pinnedConstraints: ['user: preserve recent turns'],
      sourceDigest: 'a'.repeat(64),
      digestMarker: `sha256:${'a'.repeat(12)}`,
      sourceItemIds: [],
      schemaVersion: 3,
      reasoningExcluded: true,
      reasoningExclusionProof: `sha256:${'b'.repeat(64)}`,
      assistantProseExcluded: true,
      toolPayloadsExcluded: true,
      caseFactsExcluded: true,
      caseHistoryProjectionVersion: 2
    })).toMatchObject({
      kind: 'compaction',
      auto: false
    })
  })
})

describe('todo event mapping', () => {
  it('surfaces thread todo updates through the event sink', async () => {
    const events: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTodos: (event) => {
        events.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'todos_updated',
      seq: 4,
      timestamp: '2026-06-04T00:00:00.000Z',
      threadId: 'thr_1',
      todos: {
        threadId: 'thr_1',
        turnId: 'turn_todo',
        updatedAt: '2026-06-04T00:00:00.000Z',
        items: [{
          id: 'todo_1',
          content: 'Wire todo panel',
          status: 'failed',
          statusReasonCode: 'tool_failed',
          source: { kind: 'manual' },
          note: 'verified by operator',
          createdAt: '2026-06-04T00:00:00.000Z',
          updatedAt: '2026-06-04T00:00:00.000Z'
        }]
      }
    }, sink, async () => undefined)

    expect(events).toEqual([{
      threadId: 'thr_1',
      createdAt: '2026-06-04T00:00:00.000Z',
      todos: {
        threadId: 'thr_1',
        turnId: 'turn_todo',
        updatedAt: '2026-06-04T00:00:00.000Z',
        items: [expect.objectContaining({
          content: 'Wire todo panel',
          status: 'failed',
          statusReasonCode: 'tool_failed',
          source: { kind: 'manual' },
          note: 'verified by operator'
        })]
      }
    }])
  })
})

describe('review mapping', () => {
  const reviewItem: CoreTurnItemJson = {
    id: 'item_review_1',
    turnId: 'turn_1',
    threadId: 'thr_1',
    role: 'assistant',
    status: 'completed',
    createdAt: '2026-06-04T00:00:00.000Z',
    kind: 'review',
    title: 'Review current changes',
    target: { kind: 'uncommittedChanges' },
    reviewText: 'No review findings.',
    output: {
      findings: [],
      overallCorrectness: 'patch is correct',
      overallExplanation: 'No blocking issues found.',
      overallConfidenceScore: 0.75
    }
  }

  it('maps persisted review items to review blocks', () => {
    const block = chatBlockFromItem(reviewItem)
    expect(block).toMatchObject({
      kind: 'review',
      id: 'item_review_1',
      title: 'Review current changes',
      status: 'success',
      output: {
        overallCorrectness: 'patch is correct'
      }
    })
  })

  it('surfaces review item updates through the event sink', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onReview: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'item_updated',
      seq: 7,
      item: reviewItem
    }, sink, async () => undefined)

    expect(captured).toMatchObject({
      itemId: 'item_review_1',
      status: 'success',
      reviewText: 'No review findings.'
    })
  })
})

describe('generated Office artifact mapping', () => {
  const artifact = { artifactId: 'a'.repeat(64), kind: 'docx' as const, contentHash: 'b'.repeat(64), byteSize: 1000, savedAt: '2026-09-15T01:00:00Z' }
  const item: CoreTurnItemJson = {
    id: 'generated-1', turnId: 'turn-1', threadId: 'thread-1', role: 'tool', status: 'completed',
    createdAt: artifact.savedAt, kind: 'tool_result', toolName: 'generate_office_document', callId: 'call-1', isError: false,
    output: { ...hostToolProjection(), projectionKind: 'artifact_status', messageKey: 'artifact_created', code: 'artifact_created', artifact }
  }
  it('maps an opaque host receipt without inventing file paths', () => {
    const block = chatBlockFromItem(item)
    expect(block?.kind).toBe('tool')
    if (block?.kind !== 'tool') throw new Error('expected generated object')
    expect(block.meta?.generatedArtifact).toEqual(artifact)
    expect(block.filePath).toBeUndefined()
  })
  it('does not promote arbitrary output or another tool into generated object authority', () => {
    for (const value of [ { ...item, output: { artifact } }, { ...item, toolName: 'remote_generator' }, { ...item, isError: true } ]) {
      const block = chatBlockFromItem(value)
      if (block?.kind !== 'tool') throw new Error('expected tool status')
      expect(block.meta?.generatedArtifact).toBeUndefined()
    }
  })
})

describe('create_plan tool mapping', () => {
  it('surfaces turn failure messages from Analytix lifecycle events', async () => {
    let capturedError: string | null = null
    let capturedErrorOptions: ThreadErrorOptions | null = null
    let capturedRuntimeError: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeError: (event) => {
        capturedRuntimeError = event
      },
      onError: (error, options) => {
        capturedError = error.message
        capturedErrorOptions = options ?? null
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'turn_failed',
      seq: 8,
      timestamp: '2024-01-01T00:00:00.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      message: 'model stream exploded'
    }, sink, async () => undefined)

    expect(capturedRuntimeError).toMatchObject({
      itemId: 'runtime_error_turn_1',
      message: 'model stream exploded',
      severity: 'error'
    })
    expect(JSON.parse(capturedError ?? '{}')).toMatchObject({
      message: 'model stream exploded',
      severity: 'error'
    })
    expect(capturedErrorOptions).toEqual({
      terminal: true,
      threadId: 'thr_1',
      turnId: 'turn_1',
      seq: 8,
      status: 'failed'
    })
  })

  it('treats terminal runtime error events as fatal turn errors', async () => {
    let capturedError: string | null = null
    let capturedErrorOptions: ThreadErrorOptions | null = null
    let capturedRuntimeError: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeError: (event) => {
        capturedRuntimeError = event
      },
      onError: (error, options) => {
        capturedError = error.message
        capturedErrorOptions = options ?? null
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'error',
      seq: 9,
      timestamp: '2024-01-01T00:00:00.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      message: 'model step budget exhausted',
      code: 'turn_step_limit_exceeded',
      terminal: true,
      details: { maxModelSteps: 1 },
      severity: 'error'
    }, sink, async () => undefined)

    expect(capturedRuntimeError).toMatchObject({
      itemId: 'runtime_error_turn_1',
      message: 'model step budget exhausted',
      code: 'turn_step_limit_exceeded',
      details: { maxModelSteps: 1 },
      severity: 'error'
    })
    expect(JSON.parse(capturedError ?? '{}')).toMatchObject({
      code: 'turn_step_limit_exceeded',
      message: 'model step budget exhausted',
      details: { maxModelSteps: 1 },
      severity: 'error'
    })
    expect(capturedErrorOptions).toEqual({
      terminal: true,
      threadId: 'thr_1',
      turnId: 'turn_1',
      seq: 9,
      status: 'failed'
    })
  })

  it('settles turn_aborted without routing it through normal completion', async () => {
    let completed = false
    let capturedError: string | null = null
    let capturedErrorOptions: ThreadErrorOptions | null = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTurnComplete: () => {
        completed = true
      },
      onError: (error, options) => {
        capturedError = error.message
        capturedErrorOptions = options ?? null
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'turn_aborted',
      seq: 10,
      timestamp: '2024-01-01T00:00:00.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1'
    }, sink, async () => undefined)

    expect(completed).toBe(false)
    expect(JSON.parse(capturedError ?? '{}')).toMatchObject({
      code: 'aborted',
      message: 'Turn aborted.',
      severity: 'info'
    })
    expect(capturedErrorOptions).toEqual({
      terminal: true,
      threadId: 'thr_1',
      turnId: 'turn_1',
      seq: 10,
      status: 'aborted'
    })
  })

  it('routes live error items to runtime error timeline events without fatal stream errors', async () => {
    let fatalCalled = false
    let capturedRuntimeError: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeError: (event) => {
        capturedRuntimeError = event
      },
      onError: () => {
        fatalCalled = true
      }
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'item_created',
      seq: 9,
      timestamp: '2024-01-01T00:00:00.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      item: {
        id: 'item_error_1',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'system',
        status: 'failed',
        createdAt: '2024-01-01T00:00:00.000Z',
        kind: 'error',
        message: 'Authorization: Bearer secret-token failed',
        code: 'stream_read_error',
        details: { token: 'secret-token' }
      }
    }, sink, async () => undefined)

    expect(fatalCalled).toBe(false)
    expect(capturedRuntimeError).toMatchObject({
      itemId: 'item_error_1',
      message: 'Authorization=<redacted> failed',
      code: 'stream_read_error',
      details: { token: 'secret-token' }
    })
  })

  it('maps a successful create_plan result to a tool block with plan metadata', () => {
    const item: CoreTurnItemJson = {
      id: 'item_plan_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      finishedAt: '2024-01-01T00:00:01.000Z',
      kind: 'tool_result',
      toolName: 'create_plan',
      callId: 'call_plan_1',
      output: planToolProjection()
    }
    const block = chatBlockFromItem(item)
    expect(block).not.toBeNull()
    if (block && block.kind === 'tool') {
      expect(block.status).toBe('success')
      expect(block.meta?.toolName).toBe('create_plan')
      expect(block.meta?.plan).toMatchObject({
        plan_id: 'plan_login',
        relative_path: '.analytix/plan/login.md',
        operation: 'draft',
        byte_size: 42
      })
      expect(JSON.stringify(block)).not.toContain('/tmp/ws')
    }
  })

  it('does not render or trust raw tool-call arguments as plan metadata', () => {
    const sentinel = 'PRIVATE_TOOL_ARGUMENT_SENTINEL'
    const block = chatBlockFromItem({
      id: 'item_plan_call_legacy',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'pending',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_call',
      toolName: 'create_plan',
      callId: 'call_plan_legacy',
      arguments: {
        plan_id: 'untrusted-plan',
        absolute_path: `/tmp/${sentinel}.md`,
        title: sentinel
      }
    })

    expect(block).toMatchObject({
      kind: 'tool',
      detail: 'tool arguments unavailable'
    })
    if (block?.kind === 'tool') expect(block.meta?.plan).toBeUndefined()
    expect(JSON.stringify(block)).not.toContain(sentinel)
  })

  it('maps a failed create_plan result without exposing its private error payload', () => {
    const item: CoreTurnItemJson = {
      id: 'item_plan_err',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'failed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'create_plan',
      callId: 'call_plan_err',
      isError: true,
      output: hostToolProjection('failed', 'validation_error')
    }
    const block = chatBlockFromItem(item)
    if (block && block.kind === 'tool') {
      expect(block.status).toBe('error')
      expect(block.meta?.plan).toBeUndefined()
      expect(block.detail).toBe('tool_failed (validation_error)')
    } else {
      throw new Error('expected tool block')
    }
  })

  it('does not lift untrusted tool-result attachments, paths, or inline media into tool metadata', () => {
    const sentinel = 'PRIVATE_TOOL_MEDIA_SENTINEL'
    const item: CoreTurnItemJson = {
      id: 'item_img_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'generate_image',
      callId: 'call_img_1',
      output: {
        files: [{
          relativePath: '.analytix-images/img-1.png',
          localFilePath: '/tmp/project/.analytix-images/img-1.png'
        }],
        attachments: [
          {
            id: 'att_abc',
            name: 'img-1.png',
            mimeType: 'image/png',
            width: 1024,
            height: 576,
            localFilePath: `/tmp/${sentinel}.png`,
            dataUrl: `data:image/png;base64,${sentinel}`,
            previewUrl: `https://attacker.invalid/${sentinel}`
          },
          {
            id: 'att_snake',
            name: 'img-snake.png',
            mimeType: 'image/png',
            local_file_path: '/tmp/picked/img-snake.png'
          },
          { id: '   ' },
          'not-an-object',
          { name: 'missing-id.png' }
        ],
        endpoint: 'generations'
      }
    }
    const block = chatBlockFromItem(item)
    expect(block).not.toBeNull()
    if (block && block.kind === 'tool') {
      expect(block.meta?.attachments).toBeUndefined()
      expect(block.meta?.generatedFiles).toBeUndefined()
      expect(block.detail).toBeUndefined()
      expect(JSON.stringify(block)).not.toContain(sentinel)
    } else {
      throw new Error('expected tool block')
    }
  })

  it('does not lift legacy generated-file paths from tool-result output', () => {
    const item: CoreTurnItemJson = {
      id: 'item_speech_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'generate_speech',
      callId: 'call_speech_1',
      output: {
        files: [{
          relativePath: '.analytix-audio/speech.mp3',
          absolutePath: '/tmp/project/.analytix-audio/speech.mp3',
          mimeType: 'audio/mpeg',
          byteSize: 128
        }]
      }
    }
    const block = chatBlockFromItem(item)
    expect(block).not.toBeNull()
    if (block && block.kind === 'tool') {
      expect(block.meta?.generatedFiles).toBeUndefined()
      expect(JSON.stringify(block)).not.toContain('/tmp/project/.analytix-audio/speech.mp3')
    } else {
      throw new Error('expected tool block')
    }
  })

  it('omits meta attachments when tool_result output has none worth showing', () => {
    const item: CoreTurnItemJson = {
      id: 'item_img_2',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'generate_image',
      callId: 'call_img_2',
      output: { attachments: 'nope' }
    }
    const block = chatBlockFromItem(item)
    if (block && block.kind === 'tool') {
      expect(block.meta?.attachments).toBeUndefined()
    } else {
      throw new Error('expected tool block')
    }
  })

  it('surfaces create_plan tool events through the event sink', () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }
    const event: CoreRuntimeEventJson = {
      kind: 'item_completed',
      seq: 5,
      item: {
        id: 'item_plan_sink',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'tool',
        status: 'completed',
        createdAt: '2024-01-01T00:00:00.000Z',
          kind: 'tool_result',
          toolName: 'create_plan',
          callId: 'call_plan_sink',
          output: planToolProjection('refine')
      }
    }
    void dispatchAnalytixRuntimeEvent(event, sink, async () => undefined)
    const capturedTool = captured as { meta?: { plan?: { plan_id?: string; operation?: string } } } | null
    expect(capturedTool).not.toBeNull()
    expect(capturedTool?.meta?.plan?.plan_id).toBe('plan_x')
    expect(capturedTool?.meta?.plan?.operation).toBe('refine')
  })
})

describe('user input mapping', () => {
  it('maps structured user-input items without inventing submit-only options', () => {
    const item: CoreTurnItemJson = {
      id: 'item_input_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'pending',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'user_input',
      inputId: 'input_1',
      prompt: 'Pick one',
      questions: [
        {
          header: 'Decision',
          id: 'choice',
          question: 'Pick one',
          options: [
            { label: 'Yes', description: 'Continue' },
            { label: 'No', description: 'Stop' }
          ]
        }
      ]
    }
    const block = chatBlockFromItem(item)
    expect(block).toMatchObject({
      kind: 'user_input',
      questions: [
        {
          header: 'Decision',
          id: 'choice',
          question: 'Pick one',
          options: [
            { label: 'Yes', description: 'Continue' },
            { label: 'No', description: 'Stop' }
          ]
        }
      ]
    })
  })

  it('preserves submitted and cancelled user-input states when hydrating history', () => {
    const submitted = chatBlockFromItem({
      id: 'item_input_submitted',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'submitted',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'user_input',
      inputId: 'input_submitted',
      prompt: 'Pick one'
    })
    const cancelled = chatBlockFromItem({
      id: 'item_input_cancelled',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'cancelled',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'user_input',
      inputId: 'input_cancelled',
      prompt: 'Pick one'
    })

    expect(submitted).toMatchObject({ kind: 'user_input', status: 'submitted' })
    expect(cancelled).toMatchObject({ kind: 'user_input', status: 'cancelled' })
    expect(submitted).toMatchObject({ meta: { turnId: 'turn_1' } })
    expect(cancelled).toMatchObject({ meta: { turnId: 'turn_1' } })
  })

  it('surfaces structured user-input requests from runtime events', async () => {
    let request: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUserInput: (payload) => {
        request = payload
      }
    }
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'user_input_requested',
        seq: 7,
        itemId: 'item_input_2',
        inputId: 'input_2',
        prompt: 'Choose',
        questions: [
          {
            header: 'Mode',
            id: 'mode',
            question: 'Choose',
            options: [{ label: 'Fast', description: 'Use the faster path' }]
          }
        ]
      },
      sink,
      async () => undefined
    )
    expect(request).toMatchObject({
      itemId: 'item_input_2',
      requestId: 'input_2',
      questions: [
        {
          header: 'Mode',
          id: 'mode',
          question: 'Choose',
          options: [{ label: 'Fast', description: 'Use the faster path' }]
        }
      ]
    })
  })

  it('surfaces snake_case user-input request and resolution ids from runtime events', async () => {
    let request: unknown = null
    let statusUpdate: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUserInput: (payload) => {
        request = payload
      },
      onUserInputStatus: (payload) => {
        statusUpdate = payload
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'user_input_requested',
        seq: 12,
        item_id: 'item_input_snake',
        input_id: 'input_snake',
        turn_id: 'turn_snake',
        prompt: 'Choose',
        questions: [
          {
            header: 'Mode',
            id: 'mode',
            question: 'Choose',
            options: [{ label: 'Fast', description: 'Use the faster path' }]
          }
        ]
      },
      sink,
      async () => undefined
    )
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'user_input_resolved',
        seq: 13,
        item_id: 'item_input_snake',
        input_id: 'input_snake',
        status: 'cancelled'
      },
      sink,
      async () => undefined
    )

    expect(request).toMatchObject({
      itemId: 'item_input_snake',
      requestId: 'input_snake',
      turnId: 'turn_snake'
    })
    expect(statusUpdate).toEqual({
      itemId: 'item_input_snake',
      requestId: 'input_snake',
      status: 'cancelled'
    })
  })

  it('does not emit duplicate user-input cards from generic item events', async () => {
    let called = false
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUserInput: () => {
        called = true
      }
    }
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'item_created',
        seq: 8,
        item: {
          id: 'item_input_dup',
          turnId: 'turn_1',
          threadId: 'thr_1',
          role: 'tool',
          status: 'pending',
          createdAt: '2024-01-01T00:00:00.000Z',
          kind: 'user_input',
          inputId: 'input_dup',
          prompt: 'Choose'
        }
      },
      sink,
      async () => undefined
    )
    expect(called).toBe(false)
  })
})

describe('approval mapping', () => {
  it('maps expired approval items to error when hydrating history', () => {
    const block = chatBlockFromItem({
      id: 'item_approval_expired',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'expired',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'approval',
      approvalId: 'appr_expired',
      toolName: 'shell',
      summary: 'Approval required'
    })

    expect(block).toMatchObject({
      kind: 'approval',
      status: 'error',
      errorMessage: 'Approval expired because the turn was interrupted.',
      meta: { turnId: 'turn_1' }
    })
  })

  it('does not emit duplicate approval cards from generic item events', async () => {
    let called = false
    const sink: ThreadEventSink = {
      ...makeSink(),
      onApproval: () => {
        called = true
      }
    }
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'item_created',
        seq: 9,
        item: {
          id: 'item_approval_dup',
          turnId: 'turn_1',
          threadId: 'thr_1',
          role: 'tool',
          status: 'pending',
          createdAt: '2024-01-01T00:00:00.000Z',
          kind: 'approval',
          approvalId: 'appr_1',
          toolName: 'shell',
          summary: 'Approval required'
        }
      },
      sink,
      async () => undefined
    )
    expect(called).toBe(false)
  })

  it('maps approval resolved events to live approval status updates', async () => {
    const statuses: Array<Parameters<NonNullable<ThreadEventSink['onApprovalStatus']>>[0]> = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onApprovalStatus: (ev) => {
        statuses.push(ev)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'approval_resolved',
        seq: 10,
        threadId: 'thr_1',
        turnId: 'turn_1',
        approvalId: 'appr_1',
        toolName: 'shell',
        status: 'expired',
        summary: 'Approval required'
      },
      sink,
      async () => undefined
    )

    expect(statuses).toEqual([
      {
        itemId: 'appr_1',
        approvalId: 'appr_1',
        status: 'error',
        errorMessage: 'Approval expired because the turn was interrupted.'
      }
    ])
  })

  it('maps snake_case approval resolved events to live approval status updates', async () => {
    const statuses: Array<Parameters<NonNullable<ThreadEventSink['onApprovalStatus']>>[0]> = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onApprovalStatus: (ev) => {
        statuses.push(ev)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'approval_resolved',
        seq: 11,
        item_id: 'item_approval_snake',
        approval_id: 'appr_snake',
        status: 'denied'
      },
      sink,
      async () => undefined
    )

    expect(statuses).toEqual([
      {
        itemId: 'item_approval_snake',
        approvalId: 'appr_snake',
        status: 'denied'
      }
    ])
  })
})

describe('tool block merging', () => {
  it('coalesces tool_call and tool_result items for the same call id into one block', () => {
    const blocks = mergeChatBlocks([
      chatBlockFromItem({
        id: 'item_call',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'tool',
        status: 'pending',
        createdAt: '2024-01-01T00:00:00.000Z',
        kind: 'tool_call',
        toolName: 'echo',
        callId: 'call_1',
        arguments: { text: 'hi' }
      })!,
      chatBlockFromItem({
        id: 'item_result',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'tool',
        status: 'completed',
        createdAt: '2024-01-01T00:00:01.000Z',
        kind: 'tool_result',
        toolName: 'echo',
        callId: 'call_1',
        output: { echoed: 'hi' }
      })!
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_1',
      status: 'success'
    })
  })

  it('keeps finished tool result detail when terminal progress arrives later', () => {
    const blocks = mergeChatBlocks([
      chatBlockFromItem({
        id: 'item_result',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'tool',
        status: 'failed',
        createdAt: '2024-01-01T00:00:01.000Z',
        kind: 'tool_result',
        toolName: 'ls',
        callId: 'call_ls',
        output: hostToolProjection('failed', 'workspace_escape'),
        isError: true
      })!,
      {
        kind: 'tool',
        id: 'tool_call_ls',
        summary: 'tool failed',
        status: 'error',
        toolKind: 'tool_call',
        detail: 'tool failed',
        meta: {
          callId: 'call_ls',
          toolName: 'ls',
          runtimeStatus: 'tool_progress'
        }
      } satisfies ChatBlock
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      kind: 'tool',
      id: 'tool_call_ls',
      summary: 'ls',
      status: 'error'
    })
    expect(blocks[0]?.kind === 'tool' ? blocks[0].detail : '').toContain('workspace_escape')
    expect(blocks[0]?.kind === 'tool' ? blocks[0].detail : '').not.toContain('/Users/sun/Desktop')
    expect(blocks[0]?.kind === 'tool' ? blocks[0].detail : '').not.toBe('tool failed')
  })

  it('replays persisted background subagent completion progress with child diagnostics', () => {
    const block = chatBlockFromItem({
      id: 'item_progress_turn_1_background_job_completed_job_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-07T00:00:00.000Z',
      finishedAt: '2026-07-07T00:00:00.000Z',
      kind: 'tool_progress',
      toolName: 'task',
      toolKind: 'subagent',
      callId: 'call_task',
      summary: 'Background child',
      arguments: {
        runtimeStatus: 'tool_progress',
        stage: 'background_job_completed',
        summary: 'BG_WAKE_DONE',
        child: {
          parentThreadId: 'thr_1',
          parentTurnId: 'turn_1',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          childStatus: 'completed',
          background: true,
          heartbeatStatus: 'recovered',
          lastHeartbeatAt: '2026-07-07T00:00:00.000Z',
          heartbeatAgeMs: 250,
          leaseOwner: 'analytix-runtime',
          leaseExpiresAt: '2026-07-07T00:05:00.000Z',
          leaseExpired: false,
          staleAfterMs: 30000,
          orphaned: true,
          recoveryStatus: 'recovered',
          recoveryAttempt: 1,
          recoveryReason: 'runtime_restart_orphaned_running_job',
          recoveryUpdatedAt: '2026-07-07T00:00:01.000Z',
          deadLetterReason: 'previous_delivery_failed'
        },
        diagnostics: {
          notificationKind: 'background_job_completion',
          jobId: 'job-1',
          status: 'completed',
          heartbeatStatus: 'recovered',
          lastHeartbeatAt: '2026-07-07T00:00:00.000Z',
          heartbeatAgeMs: 250,
          leaseOwner: 'analytix-runtime',
          leaseExpiresAt: '2026-07-07T00:05:00.000Z',
          leaseExpired: false,
          staleAfterMs: 30000,
          orphaned: true,
          recoveryStatus: 'recovered',
          recoveryAttempt: 1,
          recoveryReason: 'runtime_restart_orphaned_running_job',
          recoveryUpdatedAt: '2026-07-07T00:00:01.000Z',
          deadLetterReason: 'previous_delivery_failed',
          terminal: true,
          background: true,
          canContinueParent: true,
          lateCompletionSuppressed: false,
          outputPreview: 'BG_WAKE_DONE'
        }
      }
    })

    expect(block).toMatchObject({
      kind: 'tool',
      id: 'subagent_job-1',
      summary: 'Subagent activity',
      status: 'success',
      toolKind: 'subagent',
      meta: {
        runtimeStatus: 'tool_progress',
        callId: 'call_task',
        child: {
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          childStatus: 'completed',
          background: true,
          heartbeatStatus: 'recovered',
          heartbeatAgeMs: 250,
          leaseExpired: false,
          staleAfterMs: 30000,
          orphaned: true,
          recoveryStatus: 'recovered',
          recoveryAttempt: 1
        },
        diagnostics: {
          notificationKind: 'background_job_completion',
          jobId: 'job-1',
          heartbeatStatus: 'recovered',
          heartbeatAgeMs: 250,
          leaseExpired: false,
          staleAfterMs: 30000,
          orphaned: true,
          recoveryStatus: 'recovered',
          recoveryAttempt: 1,
          canContinueParent: true,
          lateCompletionSuppressed: false
        }
      }
    })
    const serialized = JSON.stringify(block)
    expect(serialized).not.toContain('BG_WAKE_DONE')
    expect(serialized).not.toContain('analytix-runtime')
    expect(serialized).not.toContain('runtime_restart_orphaned_running_job')
    expect(serialized).not.toContain('previous_delivery_failed')
  })

  it('replays persisted background auto-continue progress diagnostics', () => {
    const block = chatBlockFromItem({
      id: 'item_progress_turn_1_background_job_auto_continue_started_job_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-07T00:00:01.000Z',
      finishedAt: '2026-07-07T00:00:01.000Z',
      kind: 'tool_progress',
      toolName: 'background_auto_continue',
      callId: 'background_auto_continue_started_job_1',
      summary: 'background job woke the parent thread',
      arguments: {
        runtimeStatus: 'tool_progress',
        stage: 'background_job_auto_continue_started',
        status: 'started',
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          autoContinueParent: true,
          autoContinueStatus: 'started',
          autoContinueTurnId: 'turn_auto',
          deliveryId: 'delivery_turn_1_job_1',
          deliveryStatus: 'delivered'
        }
      }
    })

    expect(block).toMatchObject({
      kind: 'tool',
      id: 'tool_background_auto_continue_started_job_1',
      summary: 'Tool activity',
      status: 'success',
      meta: {
        runtimeStatus: 'tool_progress',
        callId: 'background_auto_continue_started_job_1',
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job-1',
          childThreadId: 'thr_child',
          autoContinueStatus: 'started',
          autoContinueTurnId: 'turn_auto',
          deliveryStatus: 'delivered'
        }
      }
    })
  })

  it('replays persisted background delivery ledger diagnostics', () => {
    const block = chatBlockFromItem({
      id: 'item_progress_turn_1_background_job_delivery_dead_letter_delivery_turn_1_job_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'failed',
      createdAt: '2026-07-07T00:00:02.000Z',
      finishedAt: '2026-07-07T00:00:02.000Z',
      kind: 'tool_progress',
      toolName: 'background_delivery',
      callId: 'background_delivery_dead_letter_delivery_turn_1_job_1',
      summary: 'background job completion delivery dead-lettered',
      arguments: {
        runtimeStatus: 'tool_progress',
        stage: 'background_job_delivery_dead_letter',
        status: 'dead_letter',
        diagnostics: {
          notificationKind: 'background_job_delivery',
          jobId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          deliveryId: 'delivery_turn_1_job_1',
          deliveryStatus: 'dead_letter',
          deliveryItemId: 'item_progress_turn_1_background_job_completed_job_1',
          deliveryReason: 'parent_thread_missing',
          completionDeliveryAttempt: 3,
          completionDeadLetterAt: '2026-07-07T00:00:02.000Z',
          recoveryAttempt: 2,
          deadLetterReason: 'parent_thread_missing'
        }
      }
    })

    expect(block).toMatchObject({
      kind: 'tool',
      id: 'tool_background_delivery_dead_letter_delivery_turn_1_job_1',
      status: 'error',
      meta: {
        runtimeStatus: 'tool_progress',
        callId: 'background_delivery_dead_letter_delivery_turn_1_job_1',
        diagnostics: {
          notificationKind: 'background_job_delivery',
          jobId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          deliveryId: 'delivery_turn_1_job_1',
          deliveryStatus: 'dead_letter',
          deliveryItemId: 'item_progress_turn_1_background_job_completed_job_1',
          completionDeliveryAttempt: 3,
          recoveryAttempt: 2
        }
      }
    })
    expect(JSON.stringify(block)).not.toContain('parent_thread_missing')
  })
})

describe('streaming runtime status events', () => {
  it('drops marker-free progress, pipeline, path, command, and child metadata text', async () => {
    const marker = 'SENTINEL_RENDERER_PRIVATE_TEXT_9B31'
    const historyBlock = chatBlockFromItem({
      id: 'item_progress_marker',
      threadId: 'thr_parent',
      turnId: 'turn_parent',
      role: 'tool',
      kind: 'tool_progress',
      status: 'running',
      summary: marker,
      callId: 'call_task',
      toolName: 'task',
      arguments: {
        command: `printf ${marker}`,
        path: `/tmp/${marker}`,
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_task',
          childId: 'child_1',
          childRunId: 'job_1',
          childStatus: 'running',
          childLabel: marker,
          childProfileDescription: `${marker}:profile`,
          worktreePath: `/tmp/${marker}`,
          conflictSummary: `${marker}:conflict`
        },
        diagnostics: {
          status: 'running',
          warning: marker,
          suggestion: `${marker}:suggestion`
        }
      }
    } as CoreTurnItemJson)
    const liveEvents: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => liveEvents.push(event),
      onRuntimeStatus: (event) => liveEvents.push(event)
    }

    await dispatchAnalytixRuntimeEvent({
      kind: 'tool_progress',
      seq: 1,
      threadId: 'thr_parent',
      turnId: 'turn_parent',
      itemId: 'item_progress_marker',
      callId: 'call_task',
      toolName: 'task',
      summary: marker,
      message: `${marker}:message`,
      status: 'running',
      child: {
        parentThreadId: 'thr_parent',
        parentTurnId: 'turn_parent',
        parentToolCallId: 'call_task',
        childId: 'child_1',
        childRunId: 'job_1',
        childStatus: 'running',
        childLabel: marker,
        childProfileDescription: `${marker}:profile`,
        worktreePath: `/tmp/${marker}`
      }
    }, sink, async () => undefined)
    await dispatchAnalytixRuntimeEvent({
      kind: 'pipeline_stage',
      seq: 2,
      threadId: 'thr_parent',
      turnId: 'turn_parent',
      stage: 'provider_retrying',
      label: marker,
      message: `${marker}:pipeline`,
      details: { message: `${marker}:detail`, reason: `${marker}:reason` }
    }, sink, async () => undefined)

    expect(historyBlock).toMatchObject({
      kind: 'tool',
      summary: 'Subagent activity',
      meta: expect.objectContaining({ runtimeStatus: 'tool_progress' })
    })
    expect(historyBlock).not.toHaveProperty('filePath')
    expect(historyBlock?.kind === 'tool' ? historyBlock.meta?.command : undefined).toBeUndefined()
    expect(JSON.stringify([historyBlock, liveEvents])).not.toContain(marker)
  })

  it('surfaces tool-call ready events as running tool cards', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_ready',
        seq: 20,
        itemId: 'item_tool_turn_1_call_read',
        callId: 'call_read',
        toolName: 'read',
        readyCount: 2,
        partial: true
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      itemId: 'tool_call_read',
      summary: 'read',
      status: 'running',
      toolKind: 'tool_call',
      meta: {
        sourceItemId: 'item_tool_turn_1_call_read',
        callId: 'call_read',
        toolName: 'read',
        readyCount: 2,
        partial: true,
        runtimeStatus: 'tool_call_ready'
      }
    })
  })

  it('surfaces typed tool started and finished events even without item snapshots', async () => {
    const captured: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvents([
      {
        kind: 'tool_call_started',
        seq: 24,
        itemId: 'item_call_read',
        callId: 'call_read',
        toolName: 'read',
        turnId: 'turn_live',
        summary: 'Read README'
      },
      {
        kind: 'tool_call_finished',
        seq: 25,
        itemId: 'item_result_read',
        callId: 'call_read',
        toolName: 'read',
        turnId: 'turn_live',
        summary: 'Read README',
        status: 'completed',
        message: '/tmp/project/README.md'
      }
    ], sink, async () => undefined)

    expect(captured).toEqual([
      expect.objectContaining({
        itemId: 'tool_call_read',
        summary: 'Read README',
        status: 'running',
        turnId: 'turn_live',
        meta: expect.objectContaining({
          sourceItemId: 'item_call_read',
          callId: 'call_read',
          toolName: 'read',
          turnId: 'turn_live',
          runtimeStatus: 'tool_call_started'
        })
      }),
      expect.objectContaining({
        itemId: 'tool_call_read',
        summary: 'Read README',
        status: 'success',
        turnId: 'turn_live',
        meta: expect.objectContaining({
          sourceItemId: 'item_result_read',
          callId: 'call_read',
          toolName: 'read',
          turnId: 'turn_live',
          runtimeStatus: 'tool_call_finished'
        })
      })
    ])
    expect(JSON.stringify(captured)).not.toContain('/tmp/project/README.md')
  })

  it('surfaces snake_case Go tool lifecycle and progress events', async () => {
    const captured: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvents([
      {
        kind: 'tool_call_started',
        seq: 34,
        item_id: 'item_call_grep',
        call_id: 'call_grep',
        tool_name: 'grep',
        turn_id: 'turn_snake',
        summary: 'Search source'
      },
      {
        kind: 'tool_progress',
        seq: 35,
        call_id: 'call_grep',
        tool_name: 'grep',
        turn_id: 'turn_snake',
        summary: 'Search source',
        status: 'running',
        message: 'scanning files'
      },
      {
        kind: 'tool_call_finished',
        seq: 36,
        item_id: 'item_result_grep',
        call_id: 'call_grep',
        tool_name: 'grep',
        turn_id: 'turn_snake',
        summary: 'Search source',
        status: 'completed',
        message: '2 matches'
      }
    ], sink, async () => undefined)

    expect(captured).toEqual([
      expect.objectContaining({
        itemId: 'tool_call_grep',
        summary: 'Search source',
        status: 'running',
        turnId: 'turn_snake',
        meta: expect.objectContaining({
          sourceItemId: 'item_call_grep',
          callId: 'call_grep',
          turnId: 'turn_snake',
          runtimeStatus: 'tool_call_started'
        })
      }),
      expect.objectContaining({
        itemId: 'tool_call_grep',
        summary: 'Tool activity',
        status: 'running',
        turnId: 'turn_snake',
        meta: expect.objectContaining({
          callId: 'call_grep',
          turnId: 'turn_snake',
          runtimeStatus: 'tool_progress'
        })
      }),
      expect.objectContaining({
        itemId: 'tool_call_grep',
        summary: 'Search source',
        status: 'success',
        turnId: 'turn_snake',
        meta: expect.objectContaining({
          sourceItemId: 'item_result_grep',
          callId: 'call_grep',
          toolName: 'grep',
          turnId: 'turn_snake',
          runtimeStatus: 'tool_call_finished'
        })
      })
    ])
    expect(JSON.stringify(captured)).not.toContain('scanning files')
    expect(JSON.stringify(captured)).not.toContain('2 matches')
  })

  it('surfaces early tool-call ready events before a durable item id exists', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_ready',
        seq: 21,
        callId: 'call_search',
        toolName: 'grep',
        readyCount: 1
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      itemId: 'tool_call_search',
      summary: 'grep',
      status: 'running',
      toolKind: 'tool_call',
      meta: {
        callId: 'call_search',
        toolName: 'grep',
        readyCount: 1,
        runtimeStatus: 'tool_call_ready'
      }
    })
  })

  it('classifies subagent tool-call ready events before a durable item id exists', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_call_ready',
        seq: 21,
        callId: 'call_task',
        toolName: 'task',
        readyCount: 1
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      itemId: 'tool_call_task',
      summary: 'task',
      status: 'running',
      toolKind: 'subagent',
      meta: {
        callId: 'call_task',
        toolName: 'task',
        readyCount: 1,
        runtimeStatus: 'tool_call_ready'
      }
    })
  })

  it('surfaces tool progress events as updates to the running tool card', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_progress',
        seq: 22,
        itemId: 'item_tool_1',
        callId: 'call_task',
        toolName: 'task',
        summary: 'Research child',
        status: 'running',
        message: 'subagent running',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'job-1',
          childStatus: 'running'
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      itemId: 'subagent_job-1',
      summary: 'Subagent activity',
      status: 'running',
      toolKind: 'subagent',
      meta: {
        sourceItemId: 'item_tool_1',
        runtimeStatus: 'tool_progress',
        callId: 'call_task',
        child: {
          childId: 'job-1',
          childStatus: 'running'
        }
      }
    })
    expect(captured).not.toHaveProperty('detail')
  })

  it('preserves background shell child kind on tool progress events', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_progress',
        seq: 23,
        itemId: 'item_tool_bash',
        callId: 'call_bash',
        toolName: 'bash',
        summary: 'npm test',
        status: 'running',
        message: 'background shell running',
        child: {
          kind: 'background-shell',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'job-1',
          childRunId: 'job-1',
          childName: 'bash',
          childLabel: 'npm test',
          childStatus: 'running',
          background: true
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      itemId: 'background_shell_job-1',
      summary: 'Background task activity',
      status: 'running',
      toolKind: 'command_execution',
      meta: {
        runtimeStatus: 'tool_progress',
        callId: 'call_bash',
        child: {
          kind: 'background-shell',
          childRunId: 'job-1',
          childStatus: 'running',
          background: true
        }
      }
    })
    expect(captured).not.toHaveProperty('detail')
  })

  it('surfaces tool-result upload waits as runtime status events', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_result_upload_wait',
        seq: 21,
        timestamp: '2026-06-03T10:00:00.000Z',
        threadId: 'thr_1',
        turnId: 'turn_1',
        status: 'waiting',
        toolResultCount: 3
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      kind: 'tool_result_upload_wait',
      itemId: 'runtime_status_turn_1_tool_upload_wait',
      turnId: 'turn_1',
      createdAt: '2026-06-03T10:00:00.000Z',
      toolResultCount: 3
    })
  })

  it('does not surface internal runtime pipeline stages as visible work rows', async () => {
    const statuses: unknown[] = []
    const tools: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        statuses.push(event)
      },
      onTool: (event) => {
        tools.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 23,
        timestamp: '2026-06-03T10:00:02.000Z',
        threadId: 'thr_1',
        turnId: 'turn_1',
        stage: 'setup',
        label: 'Setup'
      },
      sink,
      async () => undefined
    )

    expect(statuses).toEqual([])
    expect(tools).toEqual([])
  })

  it.each(['logical_step_started', 'logical_step_closed'] as const)('keeps %s statistics metadata out of visible work rows', async stage => {
    const statuses: unknown[] = [], tools: unknown[] = []
    const sink: ThreadEventSink = { ...makeSink(), onRuntimeStatus: event => { statuses.push(event) },
      onTool: event => { tools.push(event) } }
    await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: 23, timestamp: '2026-10-12T00:00:00Z',
      threadId: 'thr_1', turnId: 'turn_1', stage, label: 'UNTRUSTED_STATS_LABEL',
      details: { schemaVersion: 1, logicalStepId: '11111111-1111-4111-8111-111111111111',
        ...(stage === 'logical_step_closed' ? { assistantMessageAssembled: true, llmMs: 0, ttftMs: 0, decodeMs: 0, decodeTokens: 0 } : {}) }
    }, sink, async () => undefined)
    expect(statuses).toEqual([])
    expect(tools).toEqual([])
  })

  it('coalesces fixed provider progress by turn and rejects unscoped progress', async () => {
    const statuses: Array<{ itemId: string; label?: string }> = []
    const sink: ThreadEventSink = { ...makeSink(), onRuntimeStatus: event => { statuses.push(event) } }
    for (const [index, stage] of (['pre_send', 'post_send', 'response_received'] as const).entries()) {
      await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: index + 1,
        threadId: 'thr_progress', turnId: 'turn_progress', stage, label: 'UNTRUSTED_STAGE_TEXT' }, sink, async () => undefined)
    }
    expect(statuses.map(s => s.label)).toEqual(['Preparing model request', 'Model request started', 'Response received'])
    expect(new Set(statuses.map(s => s.itemId))).toEqual(new Set(['runtime_status_turn_progress_provider_progress']))
    await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: 4,
      threadId: 'thr_progress', stage: 'post_send', label: 'UNSCOPED' }, sink, async () => undefined)
    expect(statuses).toHaveLength(3)
  })

  it('surfaces subagent pipeline child linkage as a runtime status event', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 24,
        timestamp: '2026-06-03T10:00:03.000Z',
        threadId: 'thr_parent',
        turnId: 'turn_parent',
        stage: 'subagent_aborted',
        label: 'research child',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          childLabel: 'PRIVATE_CHILD_LABEL_71C0',
          childStatus: 'aborted',
          childModel: 'claude-sonnet-4',
          childProviderId: 'anthropic-main',
          childEndpointFormat: 'messages',
          childModelVariant: 'opus',
          childModelSource: 'subagent-profile',
          continueFrom: 'job-source',
          ...({
            sourceRef: 'job-source',
            artifactPath: '/tmp/project/.analytix/jobs/job-1.log'
          } as Record<string, unknown>),
          childModelExecution: {
            providerId: 'anthropic-main',
            modelId: 'claude-sonnet-4',
            variant: 'opus',
            endpointFormat: 'messages',
            baseUrlFingerprint: 'sha256:base-url',
            customFullEndpointFingerprint: 'sha256:full-endpoint',
            capabilityFingerprint: 'sha256:capabilities',
            source: 'subagent-profile',
            resolvedAt: '2026-06-03T10:00:03.000Z'
          },
          childEffort: 'auto',
          childToolPolicy: 'readOnly',
          background: true,
          totalTokens: 42,
          cachedTokens: 21,
          cacheHitTokens: 21,
          cacheMissTokens: 21,
          cacheHitRate: 0.5,
          cacheableTokenHitRate: 0.5,
          totalInputTokenHitRate: 0.42,
          cacheDiagnostics: {
            providerId: 'deepseek',
            prefixHash: 'prefix_1',
            cacheHitTokens: 21,
            cacheMissTokens: 21
          }
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_parent_subagent_aborted_job-1',
      turnId: 'turn_parent',
      createdAt: '2026-06-03T10:00:03.000Z',
      stage: 'subagent_aborted',
      label: 'Subagent lifecycle update',
      message: 'Subagent lifecycle update',
      child: {
        parentThreadId: 'thr_parent',
        parentTurnId: 'turn_parent',
        parentToolCallId: 'call_task',
        childId: 'job-1',
        childRunId: 'job-1',
        childThreadId: 'thr_child',
        childTurnId: 'turn_child',
        childStatus: 'aborted',
        childEffort: 'auto',
        childToolPolicy: 'readOnly',
        background: true,
        totalTokens: 42,
        cachedTokens: 21,
        cacheHitTokens: 21,
        cacheMissTokens: 21,
        cacheHitRate: 0.5,
        cacheableTokenHitRate: 0.5,
        totalInputTokenHitRate: 0.42
      }
    })
    const capturedChild = (captured as { child?: Record<string, unknown> } | null)?.child
    expect(capturedChild).not.toHaveProperty('sourceRef')
    expect(capturedChild).not.toHaveProperty('artifactPath')
    expect(capturedChild).not.toHaveProperty('cacheDiagnostics')
    expect(capturedChild).not.toHaveProperty('childLabel')
    expect(capturedChild).not.toHaveProperty('childModel')
    expect(capturedChild).not.toHaveProperty('continueFrom')
  })

  it('projects subagent pipeline stages into visible child-agent tool rows', async () => {
    const statuses: unknown[] = []
    const tools: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        statuses.push(event)
      },
      onTool: (event) => {
        tools.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 28,
        timestamp: '2026-06-03T10:00:08.000Z',
        threadId: 'thr_parent',
        turnId: 'turn_parent',
        stage: 'subagent_running',
        label: 'research child',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_task',
          childId: 'job-1',
          childRunId: 'job-1',
          childThreadId: 'thr_child',
          childLabel: 'research',
          childStatus: 'running',
          childModel: 'claude-sonnet-4',
          childProviderId: 'anthropic-main',
          childEndpointFormat: 'messages',
          childModelVariant: 'opus',
          childModelSource: 'subagent-profile',
          childModelExecution: {
            providerId: 'anthropic-main',
            modelId: 'claude-sonnet-4',
            variant: 'opus',
            endpointFormat: 'messages',
            capabilityFingerprint: 'sha256:capabilities',
            source: 'subagent-profile',
            resolvedAt: '2026-06-03T10:00:08.000Z'
          },
          childEffort: 'high',
          childProfile: 'researcher',
          childProfileMode: 'subagent',
          childProfileDescription: 'research profile',
          childToolPolicy: 'readOnly',
          jobId: 'job-1',
          returnFormat: 'evidence',
          tokenBudget: 2048,
          timeBudgetMs: 30000,
          budgetExceeded: true,
          evidenceBundleStatus: 'not_found',
          evidenceCount: 0,
          childTodoListId: 'child-todos-1',
          childTodoScope: 'child',
          childTodoCount: 3,
          childTodoCompletedCount: 1,
          childTodoInProgressCount: 1,
          childTodoPendingCount: 1,
          childTodoProjectionId: 'projection-1',
          childTodoProjectionStatus: 'proposed',
          childTodoProjectionSummary: '3 child todos: 1 completed, 1 in progress, 1 pending, 0 blocked, 0 canceled',
          childTodoProjectionItemCount: 3,
          childTodoProjectionEvidenceCount: 2,
          childTodoProjectionMappedCount: 2,
          childTodoProjectionCompletedMappedCount: 1,
          childTodoProjectionDecisionId: 'projection-decision-1',
          childTodoProjectionDecision: 'accepted',
          childTodoProjectionApprovalId: 'approval-1',
          childTodoProjectionAcceptedItemCount: 1,
          childTodoProjectionSkippedItemCount: 2,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job-1',
          worktreeBranch: 'codex/subagent/thr_parent/job-1',
          baseCommit: 'abc1234567890',
          currentCommit: 'def1234567890',
          mergeStatus: 'not_requested',
          mergeDecisionId: 'decision-1',
          mergeDecision: 'reject',
          mergeDecisionCreatedAt: '2026-06-03T10:00:12.000Z',
          cleanupReceiptId: 'cleanup-1',
          cleanupAcceptDecisionId: 'accept-1',
          cleanupRemoved: true,
          cleanupRetainedReason: 'already removed',
          acceptDecisionId: 'accept-1',
          acceptApprovalId: 'approval-1',
          appliedPatchDigest: 'sha256:abc',
          conflictReportId: 'conflict-1',
          parentHeadAtConflict: 'parent-head',
          childHeadAtConflict: 'child-head',
          sourcePatchDigest: 'sha256:source',
          conflictSummary: 'patch conflict',
          conflictFileCount: 1,
          conflictFiles: [{ path: 'src/conflict.ts', status: 'UU' }],
          repairReviewId: 'repair-review-1',
          repairConflictReportId: 'conflict-1',
          repairPatchDigest: 'sha256:repair',
          repairExpectedParentHead: 'parent-head',
          repairDryRunStatus: 'clean',
          repairChangedFileCount: 1,
          repairChangedFiles: [{ path: 'src/conflict.ts', status: 'M' }],
          repairDecisionId: 'repair-decision-1',
          repairApprovalId: 'approval-repair-1',
          repairAppliedPatchDigest: 'sha256:repair',
          diffSummary: '1 file changed',
          changedFileCount: 1,
          changedFiles: [{ path: 'src/app.ts', status: 'M' }],
          background: true,
          parallelIndex: 1,
          totalTokens: 12,
          cachedTokens: 3,
          cacheHitTokens: 3,
          cacheMissTokens: 9,
          cacheHitRate: 0.25,
          cacheDiagnostics: {
            providerId: 'deepseek',
            prefixHash: 'prefix_2',
            cacheHitTokens: 3,
            cacheMissTokens: 9
          }
        },
        details: {
          status: 'running',
          artifactPath: '/tmp/project/.analytix/jobs/job-1.log',
          stalled: true,
          heartbeatAt: '2026-06-03T10:00:10.000Z',
          heartbeatAgeMs: 15000,
          warningCode: 'task_job_stalled',
          warning: 'background job job-1 has been idle',
          suggestedTools: ['bash_output', 'wait', 'kill_shell'],
          suggestion: 'Use bash_output(jobId="job-1") to inspect recent output.'
        }
      },
      sink,
      async () => undefined
    )

    expect(statuses).toHaveLength(1)
    expect(tools).toEqual([
      expect.objectContaining({
        itemId: 'subagent_job-1',
        summary: 'Subagent activity',
        status: 'running',
        toolKind: 'subagent',
        meta: expect.objectContaining({
          runtimeStatus: 'subagent_running',
          stage: 'subagent_running',
          child: expect.objectContaining({
            parentThreadId: 'thr_parent',
            parentTurnId: 'turn_parent',
            childRunId: 'job-1',
            childStatus: 'running',
            childEffort: 'high',
            childToolPolicy: 'readOnly',
            jobId: 'job-1',
            returnFormat: 'evidence',
            tokenBudget: 2048,
            timeBudgetMs: 30000,
            budgetExceeded: true,
            evidenceBundleStatus: 'not_found',
            evidenceCount: 0,
            childTodoListId: 'child-todos-1',
            childTodoScope: 'child',
            childTodoCount: 3,
            childTodoCompletedCount: 1,
            childTodoInProgressCount: 1,
            childTodoPendingCount: 1,
            childTodoProjectionId: 'projection-1',
            childTodoProjectionStatus: 'proposed',
            childTodoProjectionItemCount: 3,
            childTodoProjectionEvidenceCount: 2,
            childTodoProjectionMappedCount: 2,
            childTodoProjectionCompletedMappedCount: 1,
            childTodoProjectionDecisionId: 'projection-decision-1',
            childTodoProjectionDecision: 'accepted',
            childTodoProjectionApprovalId: 'approval-1',
            childTodoProjectionAcceptedItemCount: 1,
            childTodoProjectionSkippedItemCount: 2,
            isolationMode: 'worktree',
            mergeStatus: 'not_requested',
            mergeDecisionId: 'decision-1',
            cleanupReceiptId: 'cleanup-1',
            cleanupAcceptDecisionId: 'accept-1',
            cleanupRemoved: true,
            acceptDecisionId: 'accept-1',
            acceptApprovalId: 'approval-1',
            conflictReportId: 'conflict-1',
            conflictFileCount: 1,
            repairReviewId: 'repair-review-1',
            repairConflictReportId: 'conflict-1',
            repairDryRunStatus: 'clean',
            repairChangedFileCount: 1,
            repairDecisionId: 'repair-decision-1',
            repairApprovalId: 'approval-repair-1',
            changedFileCount: 1,
            background: true,
            parallelIndex: 1,
            totalTokens: 12,
            cachedTokens: 3,
            cacheHitTokens: 3,
            cacheMissTokens: 9,
            cacheHitRate: 0.25
          }),
          diagnostics: {
            status: 'running',
            stalled: true,
            heartbeatAgeMs: 15000,
            warningCode: 'task_job_stalled'
          }
        })
      })
    ])
    const serializedTools = JSON.stringify(tools)
    expect(serializedTools).not.toContain('/tmp/project/.analytix/jobs/job-1.log')
    expect(serializedTools).not.toContain('background job job-1 has been idle')
    expect(serializedTools).not.toContain('Use bash_output')
    expect(serializedTools).not.toContain('prefix_2')
    expect(serializedTools).not.toContain('research child')
    expect(serializedTools).not.toContain('research profile')
    expect(serializedTools).not.toContain('claude-sonnet-4')
    expect(serializedTools).not.toContain('/tmp/analytix/subagent-worktrees')
    expect(serializedTools).not.toContain('patch conflict')
    expect(serializedTools).not.toContain('src/conflict.ts')
  })

  it('does not treat child turn lifecycle events as parent terminal events', async () => {
    let parentCompleted = 0
    let parentFailed = 0
    const statuses: unknown[] = []
    const tools: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTurnComplete: () => {
        parentCompleted += 1
      },
      onError: () => {
        parentFailed += 1
      },
      onRuntimeStatus: (event) => {
        statuses.push(event)
      },
      onTool: (event) => {
        tools.push(event)
      }
    }

    for (const [kind, childStatus, message] of [
      ['turn_started', 'running', undefined],
      ['turn_completed', 'completed', undefined],
      ['turn_aborted', 'aborted', undefined],
      ['turn_failed', 'failed', 'child failed']
    ] as const) {
      await dispatchAnalytixRuntimeEvent(
        {
          kind,
          seq: 40,
          timestamp: '2026-06-03T10:00:07.000Z',
          threadId: 'thr_parent',
          turnId: 'turn_parent',
          ...(message ? { message } : {}),
          child: {
            parentThreadId: 'thr_parent',
            parentTurnId: 'turn_parent',
            parentToolCallId: 'call_task',
            childId: 'job-1',
            childRunId: 'job-1',
            childThreadId: 'thr_child',
            childTurnId: 'turn_child',
            childLabel: 'research',
            childStatus
          }
        },
        sink,
        async () => undefined
      )
    }

    expect(parentCompleted).toBe(0)
    expect(parentFailed).toBe(0)
    expect(statuses).toHaveLength(4)
    expect(statuses.map((status) => (status as { stage?: string }).stage)).toEqual([
      'subagent_running',
      'subagent_completed',
      'subagent_aborted',
      'subagent_failed'
    ])
    expect(statuses.at(-1)).toMatchObject({
      kind: 'pipeline_stage',
      message: 'Subagent lifecycle update',
      child: {
        parentThreadId: 'thr_parent',
        parentTurnId: 'turn_parent',
        childRunId: 'job-1',
        childStatus: 'failed'
      }
    })
    expect(tools.map((tool) => (tool as { status?: string }).status)).toEqual([
      'running',
      'success',
      'error',
      'error'
    ])
    expect(tools.at(-1)).toMatchObject({
      itemId: 'subagent_job-1',
      toolKind: 'subagent',
      status: 'error',
      meta: {
        runtimeStatus: 'subagent_failed',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childRunId: 'job-1',
          childStatus: 'failed'
        }
      }
    })
  })

  it('surfaces provider retry pipeline stages as runtime status events', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 25,
        timestamp: '2026-06-03T10:00:04.000Z',
        threadId: 'thr_retry',
        turnId: 'turn_retry',
        stage: 'provider_retrying',
        label: 'Retrying provider stream',
        attempt: 2,
        maxAttempt: 3,
        details: {
          message: 'gateway timeout'
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_retry_provider_retrying_25',
      turnId: 'turn_retry',
      stage: 'provider_retrying',
      label: 'Provider request is retrying',
      message: 'Provider request is retrying',
      attempt: 2,
      maxAttempt: 3,
      meta: {
        stage: 'provider_retrying',
        attempt: 2,
        maxAttempt: 3
      }
    })
    expect(JSON.stringify(captured)).not.toContain('gateway timeout')
  })

  it('surfaces provider error pipeline stages as runtime status events', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 27,
        timestamp: '2026-06-03T10:00:06.000Z',
        threadId: 'thr_error',
        turnId: 'turn_error',
        stage: 'provider_error',
        label: 'Provider stream failed',
        details: {
          providerError: {
            kind: 'server',
            status: 503,
            retryable: false,
            failureStage: 'transport_after_observed_send',
            dispatchState: 'sent',
            attempt: 1,
            responseBody: 'PRIVATE_PROVIDER_RESPONSE'
          }
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_error_provider_error_27',
      turnId: 'turn_error',
      stage: 'provider_error',
      label: 'Provider request failed',
      message: 'Provider request failed',
      meta: {
        stage: 'provider_error',
        details: {
          providerError: {
            kind: 'server',
            status: 503,
            retryable: false,
            failureStage: 'transport_after_observed_send',
            dispatchState: 'sent',
            attempt: 1
          }
        }
      }
    })
    expect(JSON.stringify(captured)).not.toContain('PRIVATE_PROVIDER_RESPONSE')
    expect(JSON.stringify(captured)).not.toContain('responseBody')
  })

  it('surfaces empty-final recovery pipeline stages as runtime status events', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'pipeline_stage',
        seq: 26,
        timestamp: '2026-06-03T10:00:05.000Z',
        threadId: 'thr_empty',
        turnId: 'turn_empty',
        stage: 'empty_final_recovered',
        label: 'Provider returned an empty final response',
        details: {
          visibleRecovery: true,
          recoveryKind: 'empty_final',
          recoveryAttempt: 1,
          maxRecoveryAttempts: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      kind: 'pipeline_stage',
      itemId: 'runtime_status_turn_empty_empty_final_recovered_26',
      turnId: 'turn_empty',
      stage: 'empty_final_recovered',
      message: 'Empty provider response was replaced safely',
      meta: {
        stage: 'empty_final_recovered',
        details: {
          recoveryAttempt: 1
        }
      }
    })
  })

	  it('surfaces tool catalog drift as a runtime status event', async () => {
	    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        captured = event
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'tool_catalog_changed',
        seq: 22,
        timestamp: '2026-06-03T10:00:01.000Z',
        threadId: 'thr_1',
        turnId: 'turn_1',
        fingerprint: 'fp_next',
        toolCount: 12,
        message: 'Tool catalog changed'
      },
      sink,
      async () => undefined
    )

	    expect(captured).toMatchObject({
	      kind: 'tool_catalog_changed',
	      itemId: 'runtime_status_tool_catalog_fp_next',
	      turnId: 'turn_1',
	      createdAt: '2026-06-03T10:00:01.000Z',
	      message: 'Tool catalog changed.'
	    })
	  })

	  it('surfaces storm suppression as a runtime status event', async () => {
	    let captured: unknown = null
	    const sink: ThreadEventSink = {
	      ...makeSink(),
	      onRuntimeStatus: (event) => {
	        captured = event
	      }
	    }

	    await dispatchAnalytixRuntimeEvent(
	      {
	        kind: 'tool_storm_suppressed',
	        seq: 23,
	        timestamp: '2026-06-03T10:00:02.000Z',
	        threadId: 'thr_1',
	        turnId: 'turn_1',
	        itemId: 'item_call_read_storm',
	        callId: 'call_read',
	        toolName: 'read',
	        message: 'read repeated the same arguments'
	      },
	      sink,
	      async () => undefined
	    )

	    expect(captured).toMatchObject({
	      kind: 'tool_storm_suppressed',
	      itemId: 'item_call_read_storm',
	      turnId: 'turn_1',
	      createdAt: '2026-06-03T10:00:02.000Z',
	      callId: 'call_read',
	      toolName: 'read',
	      message: 'Repeated tool activity was suppressed.'
	    })
	  })

  it('surfaces snake_case runtime status fields without losing turn binding', async () => {
    const statuses: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onRuntimeStatus: (event) => {
        statuses.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvents([
      {
        kind: 'tool_result_upload_wait',
        seq: 31,
        timestamp: '2026-06-03T10:00:11.000Z',
        thread_id: 'thr_snake',
        turn_id: 'turn_snake',
        tool_result_count: 4
      },
      {
        kind: 'tool_storm_suppressed',
        seq: 32,
        timestamp: '2026-06-03T10:00:12.000Z',
        thread_id: 'thr_snake',
        turn_id: 'turn_snake',
        item_id: 'item_call_read_storm_snake',
        call_id: 'call_read_snake',
        tool_name: 'read_file',
        message: 'read_file repeated the same arguments'
      }
    ], sink, async () => undefined)

    expect(statuses).toEqual([
      expect.objectContaining({
        kind: 'tool_result_upload_wait',
        itemId: 'runtime_status_turn_snake_tool_upload_wait',
        turnId: 'turn_snake',
        toolResultCount: 4
      }),
      expect.objectContaining({
        kind: 'tool_storm_suppressed',
        itemId: 'item_call_read_storm_snake',
        turnId: 'turn_snake',
        callId: 'call_read_snake',
        toolName: 'read_file',
        message: 'Repeated tool activity was suppressed.'
      })
    ])
  })
	})

describe('Analytix extension metadata mapping', () => {
  it('maps turn disclosure metadata onto user messages', () => {
    const block = chatBlockFromItem({
      id: 'item_user_meta',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'user',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'user_message',
      text: 'look at this',
      displayText: 'Inspect attached image',
      attachmentIds: ['att_1'],
      fileReferences: [{
        path: '/workspace/analytix/src/App.tsx',
        relativePath: 'src/App.tsx',
        name: 'App.tsx',
        kind: 'file'
      }],
      activeSkillIds: ['skill_review'],
      injectedMemoryIds: ['mem_1'],
      skillInjectionBytes: 128
    })
    expect(block).toMatchObject({
      kind: 'user',
      meta: {
        turnId: 'turn_1',
        displayText: 'Inspect attached image',
        attachmentIds: ['att_1'],
        fileReferences: [{ relativePath: 'src/App.tsx', kind: 'file' }],
        activeSkillIds: ['skill_review'],
        injectedMemoryIds: ['mem_1'],
        skillInjectionBytes: 128
      }
    })
  })

  it('projects complete accounts from hydrated user text and display metadata', () => {
    const account = '6222020202020202020'
    const block = chatBlockFromItem({
      id: 'item_user_account',
      turnId: 'turn_account',
      threadId: 'thr_account',
      role: 'user',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'user_message',
      text: `account=${account}`,
      displayText: `查询银行卡号 ${account}`
    })

    expect(block).toMatchObject({
      kind: 'user',
      text: 'account=[ACCOUNT]',
      meta: { displayText: '查询银行卡号 [ACCOUNT]' }
    })
    expect(JSON.stringify(block)).not.toContain(account)
  })

  it('surfaces host child metadata but never promotes raw tool citations', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTool: (event) => {
        captured = event
      }
    }
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'item_completed',
        seq: 12,
        child: {
          parentThreadId: 'thr_1',
          parentTurnId: 'turn_1',
          childId: 'child_research',
          childLabel: 'research',
          childStatus: 'completed',
          childSeq: 2,
          evidenceLedgered: true,
          toolInvocations: 2
        },
        item: {
          id: 'item_web',
          turnId: 'turn_1',
          threadId: 'thr_1',
          role: 'tool',
          status: 'completed',
          createdAt: '2024-01-01T00:00:00.000Z',
          kind: 'tool_result',
          toolName: 'web_search',
          callId: 'call_web',
          output: {
            query: 'analytix mcp',
            sources: [
              {
                sourceId: 'src_1',
                title: 'Docs',
                url: 'https://example.com/docs',
                retrievedAt: '2024-01-01T00:00:00.000Z'
              }
            ]
          }
        }
      },
      sink,
      async () => undefined
    )
    expect(captured).toMatchObject({
      meta: {
        child: {
          childId: 'child_research',
          evidenceLedgered: true,
          toolInvocations: 2
        }
      }
    })
    expect((captured as { meta?: Record<string, unknown> } | null)?.meta?.sources).toBeUndefined()
    expect(JSON.stringify(captured)).not.toContain('https://example.com/docs')
    expect(JSON.stringify(captured)).not.toContain('PRIVATE_CHILD_LABEL_71C0')
  })

  it('passes parent turn completion identity to the sink', async () => {
    const completions: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onTurnComplete: (event) => {
        completions.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'turn_completed',
        seq: 19,
        timestamp: '2026-06-03T10:00:07.000Z',
        threadId: 'thr_parent',
        turnId: 'turn_parent',
        acceptedFinalDigest: 'a'.repeat(64),
        terminalReason: 'success'
      },
      sink,
      async () => undefined
    )

    expect(completions).toEqual([
      {
        threadId: 'thr_parent',
        turnId: 'turn_parent',
        createdAt: '2026-06-03T10:00:07.000Z',
        seq: 19,
        acceptedFinalDigest: 'a'.repeat(64),
        terminalReason: 'success'
      }
    ])
  })

  it('maps snapshot-required replay recovery events to the sink', async () => {
    const snapshots: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onSnapshotRequired: (event) => {
        snapshots.push(event)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'snapshot_required',
        seq: 1034,
        timestamp: '2026-06-03T10:00:08.000Z',
        threadId: 'thr_1',
        sinceSeq: 0,
        highestSeq: 1034,
        replayEventCount: 1034,
        reason: 'live_replay_backlog_exceeded'
      },
      sink,
      async () => undefined
    )

    expect(snapshots).toEqual([
      {
        threadId: 'thr_1',
        createdAt: '2026-06-03T10:00:08.000Z',
        seq: 1034,
        sinceSeq: 0,
        highestSeq: 1034,
        replayEventCount: 1034,
        reason: 'live_replay_backlog_exceeded'
      }
    ])
  })
})

describe('usage event mapping', () => {
  it('does not infer cache hit rate from cachedTokens-only usage events', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured = usage
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 12,
        usage: {
          promptTokens: 100,
          completionTokens: 5,
          totalTokens: 105,
          cachedTokens: 42,
          turns: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      inputTokens: 100,
      outputTokens: 5,
      totalTokens: 105,
      cachedTokens: 0,
      cacheMissTokens: 0,
      cacheHitRate: null,
      turns: 1
    })
  })

  it('derives cache hit rate only from explicit hit and miss usage counters', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured = usage
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 13,
        usage: {
          promptTokens: 100,
          completionTokens: 5,
          totalTokens: 105,
          cacheHitTokens: 80,
          cacheMissTokens: 20,
          cacheableTokenHitRate: 0.8,
          totalInputTokenHitRate: 0.8,
          cacheMissReasons: ['tool_catalog_changed'],
          cacheSuggestions: ['Keep tools stable.'],
          tokenEconomySavingsTokens: 4096,
          turns: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      inputTokens: 100,
      outputTokens: 5,
      reasoningTokens: 0,
      totalTokens: 105,
      cachedTokens: 80,
      cacheMissTokens: 20,
      cacheHitRate: 0.8,
      cacheableTokenHitRate: 0.8,
      totalInputTokenHitRate: 0.8,
      tokenEconomySavingsTokens: 4096,
      turns: 1
    })
    expect(JSON.stringify(captured)).not.toContain('tool_catalog_changed')
    expect(JSON.stringify(captured)).not.toContain('Keep tools stable.')
  })

  it('passes provider reasoning token usage through to the renderer', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured = usage
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 14,
        usage: {
          promptTokens: 100,
          completionTokens: 5,
          reasoningTokens: 7,
          totalTokens: 105,
          turns: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      inputTokens: 100,
      outputTokens: 5,
      reasoningTokens: 7,
      totalTokens: 105,
      turns: 1
    })
  })

  it('preserves live usage attribution and cache diagnostics metadata', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured = usage
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 17,
        model: 'deepseek-chat',
        providerId: 'deepseek',
        effort: 'auto',
        usageSource: 'subagent',
        childRunId: 'child_run_1',
        cacheDiagnostics: {
          prefixHash: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
          toolsHash: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
          route: 'direct_answer',
          toolCount: 0,
          firstTokenLatencyMs: 12,
          durationMs: 34,
          prefixChangeReasons: ['tools'],
          cacheHitTokens: 32,
          cacheMissTokens: 4
        },
        usage: {
          promptTokens: 100,
          completionTokens: 5,
          totalTokens: 105,
          cacheHitTokens: 32,
          cacheMissTokens: 4,
          turns: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      model: 'deepseek-chat',
      providerId: 'deepseek',
      effort: 'auto',
      usageSource: 'subagent',
      childRunId: 'child_run_1',
      cachedTokens: 32,
      cacheMissTokens: 4,
      cacheDiagnostics: {
        prefixHash: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        toolsHash: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
        toolCount: 0,
        firstTokenLatencyMs: 12,
        durationMs: 34
      }
    })
    expect(JSON.stringify(captured)).not.toContain('direct_answer')
    expect(JSON.stringify(captured)).not.toContain('prefixChangeReasons')
  })

  it('never normalizes invalid reasoning effort into live usage metadata', async () => {
    const captured: Array<Record<string, unknown>> = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured.push(usage as unknown as Record<string, unknown>)
      }
    }
    const sentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    for (const effort of [' high ', 'HIGH', sentinel]) {
      await dispatchAnalytixRuntimeEvent(
        {
          kind: 'usage',
          seq: 18 + captured.length,
          effort,
          usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2, turns: 1 }
        } as unknown as CoreRuntimeEventJson,
        sink,
        async () => undefined
      )
    }
    expect(captured).toHaveLength(3)
    expect(captured.every((usage) => usage.effort === undefined)).toBe(true)
    expect(JSON.stringify(captured)).not.toContain(sentinel)
  })

  it('preserves unknown versus configured zero usage cost', async () => {
    const captured: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured.push(usage)
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 15,
        usage: {
          promptTokens: 10,
          completionTokens: 2,
          totalTokens: 12,
          turns: 1
        }
      },
      sink,
      async () => undefined
    )
    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 16,
        usage: {
          promptTokens: 10,
          completionTokens: 2,
          totalTokens: 12,
          costUsd: 0,
          costCny: 0,
          priceConfigured: true,
          costEstimateStatus: 'complete',
          costKnownCurrencies: ['USD'],
          cacheSavingsUsd: 0.0001,
          turns: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured[0]).toMatchObject({
      costUsd: null,
      costCny: null,
      priceConfigured: false,
      costEstimateStatus: 'unknown'
    })
    expect(captured[1]).toMatchObject({
      costUsd: 0,
      costCny: null,
      priceConfigured: true,
      costEstimateStatus: 'complete',
      costKnownCurrencies: ['USD'],
      cacheSavingsUsd: 0.0001
    })
  })

  it('normalizes snake_case live usage fields without losing cost or cache attribution', async () => {
    let captured: unknown = null
    const sink: ThreadEventSink = {
      ...makeSink(),
      onUsage: (usage) => {
        captured = usage
      }
    }

    await dispatchAnalytixRuntimeEvent(
      {
        kind: 'usage',
        seq: 17,
        provider_id: 'xiaomi',
        usage_source: 'assistant',
        usage: {
          prompt_tokens: 10,
          completion_tokens: 3,
          reasoning_tokens: 1,
          total_tokens: 14,
          cache_hit_tokens: 6,
          cache_miss_tokens: 4,
          cache_hit_rate: 0.6,
          cost_usd: 0,
          cost_cny: 0,
          price_configured: true,
          cost_estimate_status: 'complete',
          cost_known_currencies: ['CNY'],
          cache_savings_usd: 0.0002,
          token_economy_savings_tokens: 5,
          cache_miss_reasons: ['tool schema changed'],
          cache_suggestions: ['keep tool order stable'],
          turns: 1
        }
      },
      sink,
      async () => undefined
    )

    expect(captured).toMatchObject({
      providerId: 'xiaomi',
      usageSource: 'assistant',
      inputTokens: 10,
      outputTokens: 3,
      reasoningTokens: 1,
      totalTokens: 14,
      cachedTokens: 6,
      cacheMissTokens: 4,
      cacheHitRate: 0.6,
      costUsd: null,
      costCny: 0,
      priceConfigured: true,
      costEstimateStatus: 'complete',
      costKnownCurrencies: ['CNY'],
      cacheSavingsUsd: 0.0002,
      tokenEconomySavingsTokens: 5
    })
    expect(JSON.stringify(captured)).not.toContain('tool schema changed')
    expect(JSON.stringify(captured)).not.toContain('keep tool order stable')
  })
})

describe('tool presentation inference', () => {
  it('prefers explicit toolKind from Analytix over local heuristics', () => {
    const block = chatBlockFromItem({
      id: 'item_explicit_kind',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'custom_tool',
      toolKind: 'command_execution',
      callId: 'call_explicit',
      output: { path: '/tmp/should-not-force-file-kind', command: 'echo hi' }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'command_execution'
    })
    expect(block?.kind === 'tool' ? block.meta?.command : undefined).toBeUndefined()
  })

  it('preserves explicit subagent toolKind from Go runtime items', () => {
    const block = chatBlockFromItem({
      id: 'item_subagent_kind',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'task',
      toolKind: 'subagent',
      callId: 'call_subagent',
      output: { childRunId: 'job-1', summary: 'done' }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'subagent'
    })
  })

  it('uses the explicit command_execution kind without exposing the private command string', () => {
    const block = chatBlockFromItem({
      id: 'item_shell',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_call',
      toolName: 'shell',
      toolKind: 'command_execution',
      callId: 'call_shell',
      arguments: {
        schemaVersion: 1,
        projectionKind: 'withheld',
        disclosure: 'metadata_only',
        messageKey: 'tool_arguments_withheld',
        privatePayloadWithheld: true,
        factAnswerAllowed: false,
        evidenceAuthority: false
      }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'command_execution',
      meta: { toolName: 'shell' }
    })
    expect(JSON.stringify(block)).not.toContain('npm test')
  })

  it('surfaces bash session metadata on command blocks', () => {
    const block = chatBlockFromItem({
      id: 'item_bash_session',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'bash',
      toolKind: 'command_execution',
      callId: 'call_bash',
      output: {
        command: 'npm run dev',
        session_id: 'bash_abc123',
        status: 'running',
        pid: 1234,
        shell: 'bash',
        cwd: '/tmp/app'
      }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'command_execution',
      meta: {
        toolName: 'bash'
      }
    })
    expect(JSON.stringify(block)).not.toContain('npm run dev')
  })

  it('uses the explicit file_change kind and surfaces the path', () => {
    const block = chatBlockFromItem({
      id: 'item_file',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'write_file',
      toolKind: 'file_change',
      callId: 'call_file',
      output: { path: '/tmp/demo.ts', bytes_written: 12 }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'file_change',
      meta: {
        turnId: 'turn_1'
      }
    })
    expect(block?.kind === 'tool' ? block.filePath : undefined).toBeUndefined()
  })

  it('classifies built-in write/edit tools as file_change by name when toolKind is omitted', () => {
    const block = chatBlockFromItem({
      id: 'item_write_builtin',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'write',
      callId: 'call_write',
      output: { path: '/tmp/demo.ts', bytes_written: 12 }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'file_change'
    })
    expect(block?.kind === 'tool' ? block.filePath : undefined).toBeUndefined()
  })

  it('classifies Reasonix-style multi_edit as file_change by name when toolKind is omitted', () => {
    const block = chatBlockFromItem({
      id: 'item_multi_edit_builtin',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'multi_edit',
      callId: 'call_multi_edit',
      output: { path: '/tmp/demo.ts', replacements: 3 }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'file_change'
    })
    expect(block?.kind === 'tool' ? block.filePath : undefined).toBeUndefined()
  })

  it('classifies built-in bash by name as command_execution when toolKind is omitted', () => {
    const block = chatBlockFromItem({
      id: 'item_bash_builtin',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'bash',
      callId: 'call_bash',
      output: { command: 'pwd', output: '/tmp' }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'command_execution',
      meta: { toolName: 'bash' }
    })
    expect(block?.kind === 'tool' ? block.meta?.command : undefined).toBeUndefined()
  })

  it('classifies subagent tools by name when toolKind is omitted', () => {
    const block = chatBlockFromItem({
      id: 'item_task_legacy',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'parallel_tasks',
      callId: 'call_parallel',
      output: { results: [] }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'subagent'
    })
  })

  it('does not infer presentation from legacy raw result payload shape', () => {
    const block = chatBlockFromItem({
      id: 'item_legacy',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'future_tool',
      callId: 'call_legacy',
      output: { command: 'npm test', path: '/tmp/demo.ts' }
    })
    expect(block).toMatchObject({
      kind: 'tool',
      toolKind: 'tool_call'
    })
    expect(JSON.stringify(block)).not.toContain('npm test')
  })

  it('rejects accepted-final batches mixed with ordinary or duplicate renderer events', async () => {
    const accepted = { kind: 'accepted_final_batch' }
    for (const events of [
      [accepted, { kind: 'heartbeat', seq: 1 }],
      [accepted, accepted]
    ]) {
      await expect(dispatchAnalytixRuntimeEvents(events, makeSink(), async () => undefined))
        .rejects.toThrow('cannot be mixed')
    }
  })

  it('projects a general terminal batch only through the atomic renderer sink', async () => {
    const projections: unknown[] = []
    const sink: ThreadEventSink = {
      ...makeSink(),
      onGeneralTerminalBatch: (batch) => {
        projections.push(batch)
      }
    }
    const batch = generalTerminalBatch()

    expect(generalTerminalProjectionBatchFromRuntime(batch)).toMatchObject({
      batchDigest: 'a'.repeat(64),
      threadId: 'thr_general',
      turnId: 'turn_general',
      firstSeq: 11,
      lastSeq: 13,
      terminalItem: {
        kind: 'assistant',
        id: 'item_general',
        text: '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。'
      },
      terminal: { status: 'completed', terminalReason: 'success' }
    })
    await dispatchAnalytixRuntimeEvents([batch], sink, async () => undefined)
    expect(projections).toHaveLength(1)
  })

  it('projects a typed ordinary terminal result without displaying its contract metadata', async () => {
    const batch = typedGeneralTerminalBatch()
    expect(generalTerminalProjectionBatchFromRuntime(batch)).toMatchObject({
      terminalItem: {
        kind: 'assistant',
        id: 'item_general',
        text: '已完成代码修改并通过相关测试。'
      }
    })
    expect(JSON.stringify(generalTerminalProjectionBatchFromRuntime(batch)))
      .not.toContain('analytix.ordinary-result/v1')
  })

  it.each([
    '日期：2026-07-01；数量：42；金额：1234.56 元；参考编号：REF-17。',
    '日期：2026-07-01；数量：42；金额：1234.56 元；参考编号：550e8400-e29b-41d4-a716-44665544a000。',
    '请解释 DOM 元素的父子关系。'
  ])('projects an ordinary answer without case fact authority: %s', async (answer) => {
    const batch = typedGeneralTerminalBatch(answer)
    expect(generalTerminalProjectionBatchFromRuntime(batch)).toMatchObject({
      terminalItem: { kind: 'assistant', text: answer }
    })
  })

  it.each([
    '张某与李某是父子。',
    '甲公司支付给乙公司2645.72元。',
    '甲公司支付乙公司2645.72元。',
    '甲公司支付乙公司2万元。',
    '张某与李某存在父子关系。',
    '甲公司取得2026年收益￥2万元。',
    '甲公司取得2026年收益2万元。',
    '甲公司支付\n2645.72 元'
  ])('withholds a typed case assertion from renderer projection: %s', (text) => {
    expect(generalTerminalProjectionBatchFromRuntime(typedGeneralTerminalBatch(text))).toBeNull()
  })

  it.each([
    ['fabricated amount', '该案涉案金额为 2,645,472 元。'],
    ['fabricated account', '资金已流入银行账号 6222020200001234567。']
  ])('rejects general terminal renderer projection containing %s', async (_label, text) => {
    const batch = generalTerminalBatch()
    const itemEvent = (batch.events as Array<Record<string, unknown>>)[0]
    ;(itemEvent.item as Record<string, unknown>).text = text
    const sink: ThreadEventSink = {
      ...makeSink(),
      onGeneralTerminalBatch: () => undefined
    }

    expect(generalTerminalProjectionBatchFromRuntime(batch)).toBeNull()
    await expect(dispatchAnalytixRuntimeEvents([batch], sink, async () => undefined))
      .rejects.toThrow('no atomic renderer sink')
  })

  it('rejects missing general terminal atomic sinks and mixed terminal batches', async () => {
    const batch = generalTerminalBatch()
    await expect(dispatchAnalytixRuntimeEvents([batch], makeSink(), async () => undefined))
      .rejects.toThrow('no atomic renderer sink')
    await expect(dispatchAnalytixRuntimeEvents(
      [batch, { kind: 'heartbeat', seq: 14 }],
      { ...makeSink(), onGeneralTerminalBatch: () => undefined },
      async () => undefined
    )).rejects.toThrow('cannot be mixed')
  })

  it('preserves terminal error turn identity for accepted-final projection matching', () => {
    const block = chatBlockFromItem({
      id: 'terminal_error_turn_1',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'system',
      status: 'failed',
      createdAt: '2026-07-18T00:00:00Z',
      finishedAt: '2026-07-18T00:00:00Z',
      kind: 'error',
      code: 'provider_failure',
      message: 'provider unavailable',
      severity: 'error'
    })

    expect(block).toMatchObject({
      kind: 'system',
      id: 'terminal_error_turn_1',
      meta: { turnId: 'turn_1' }
    })
  })
})


it('keeps closed cache observation and cost coverage without treating legacy false as a check', () => {
  const usage = usageFromCore({ promptTokens: 2, completionTokens: 1, totalTokens: 3 }, {
    cacheDiagnostics: {
      dynamicStateCheck: 'not_checked', toolSchemaEstimator: 'utf8_bytes_div4',
      responseModelObservation: 'differs_resolved', modelInputComparable: true,
      modelInputFirstDifference: 'tools', modelInputComparablePrefixBytes: 12,
      providerAttemptCount: 3, providerCostKnownAttemptCount: 2,
      providerKnownCostUsdNanos: 12, providerKnownCostCnyNanos: 30,
      providerCostEstimateComplete: false
    }
  })
  expect(usage.cacheDiagnostics).toMatchObject({dynamicStateCheck: 'not_checked', responseModelObservation: 'differs_resolved', modelInputFirstDifference: 'tools', providerCostEstimateComplete: false, providerAttemptCount: 3})
  const legacy = usageFromCore({}, {cacheDiagnostics: {dynamicStateLeaked: false, dynamicStateCheck: 'checked', responseModelObservation: 'PRIVATE', modelInputFirstDifference: 'PRIVATE'} as never})
  expect(legacy.cacheDiagnostics).toBeUndefined()
})


describe('settled protected result metadata bridge', () => {
  it.each(['bash', 'read', 'read_file'])('carries %s identity through the real tool event port', async (toolName) => {
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    const item: CoreTurnItemJson = {
      id: 'item_result_' + 'b'.repeat(64), turnId: 'turn_own', threadId: 'thr_own', createdAt: '2026-10-06T00:00:00Z',
      kind: 'tool_result', role: 'tool', status: 'completed', toolName,
      callId: 'call_host_' + 'a'.repeat(64), toolKind: 'tool_call', isError: false,
      output: { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only',
        status: 'completed', messageKey: 'tool_completed', code: 'tool_completed',
        privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
    }
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: 'thr_own', turnId: 'turn_own', item }, sink, async () => undefined)
    expect(received).toHaveLength(1)
    expect(received[0].meta?.localResult).toEqual({
      turnId: 'turn_own', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64)
    })
    // A later lifecycle-only event must omit the key so the existing three
    // consumer metadata merges retain the settled identity.
    await dispatchAnalytixRuntimeEvent({ kind: 'tool_progress', threadId: 'thr_own', turnId: 'turn_own',
      callId: item.callId, toolName, status: 'completed' }, sink, async () => undefined)
    expect(Object.hasOwn(received[1].meta ?? {}, 'localResult')).toBe(false)
  })
  it('does not give unknown tools or call-only items a settled selector', () => {
    const base = { id: 'item_call', turnId: 'turn_own', threadId: 'thr_own', role: 'tool' as const, createdAt: '2026-10-06T00:00:00Z', callId: 'call_host_' + 'a'.repeat(64), status: 'completed' as const }
    const unknown = chatBlockFromItem({ ...base, kind: 'tool_result', toolName: 'ls', output: {} })
    const call = chatBlockFromItem({ ...base, kind: 'tool_call', toolName: 'bash' })
    expect(unknown?.kind === 'tool' ? unknown.meta?.localResult : undefined).toBeUndefined()
    expect(call?.kind === 'tool' ? call.meta?.localResult : undefined).toBeUndefined()
  })
})


describe('typed public tool projection and token-unit adapters', () => {
  const toolItem = (output: unknown): CoreTurnItemJson => ({ id: 'item-result', turnId: 'turn', threadId: 'thread', role: 'tool', status: 'completed',
    createdAt: '2026-10-07T00:00:00Z', kind: 'tool_result', toolName: 'create_plan', callId: 'call', toolKind: 'tool_call', isError: false, output })
  it('carries a strict public result through hydration and settled event and never carries an unvalidated extra field', async () => {
    const projection = planToolProjection(), item = toolItem(projection)
    expect(chatBlockFromItem(item)).toMatchObject({ kind: 'tool', meta: { publicResult: projection } })
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = [], sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', item, threadId: 'thread', turnId: 'turn' }, sink, async () => undefined)
    expect(received[0].meta?.publicResult).toEqual(projection)
    await dispatchAnalytixRuntimeEvent({ kind: 'tool_call_finished', callId: 'call', toolName: 'create_plan', status: 'completed' }, sink, async () => undefined)
    expect(received[1].meta).not.toHaveProperty('publicResult')
    const hostile = chatBlockFromItem(toolItem({ ...projection, rawBody: 'PRIVATE_UNPUBLISHED_BODY' }))
    expect(hostile?.kind === 'tool' ? hostile.meta?.publicResult : undefined).toBeUndefined()
    expect(JSON.stringify(hostile)).not.toContain('PRIVATE_UNPUBLISHED_BODY')
  })
  it.each([0, 120])('preserves %s replaced tokens in existing metadata through history, item events and compaction events', async count => {
    const item: CoreTurnItemJson = { id: 'compaction-a', turnId: 'turn', threadId: 'thread', role: 'system', status: 'completed', createdAt: '2026-10-07T00:00:00Z',
      kind: 'compaction', replacedTokens: count, pinnedConstraints: [] }
    const b = chatBlockFromItem(item)
    expect(b).toMatchObject({ kind: 'compaction', meta: { replacedTokens: count } }); expect(b).not.toHaveProperty('messagesBefore')
    const events: Array<Parameters<ThreadEventSink['onCompaction']>[0]> = [], sink = { ...makeSink(), onCompaction: (event: Parameters<ThreadEventSink['onCompaction']>[0]) => events.push(event) }
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', item }, sink, async () => undefined)
    await dispatchAnalytixRuntimeEvent({ kind: 'compaction_completed', turnId: 'turn', replacedTokens: count }, sink, async () => undefined)
    expect(events).toHaveLength(2)
    for (const event of events) { expect(event.meta?.replacedTokens).toBe(count); expect(event).not.toHaveProperty('messagesBefore') }
  })
  it.each([-1, 1.5, NaN])('does not invent a token count or message count for invalid %s input', count => {
    const b = chatBlockFromItem({ id: 'compaction-a', role: 'system', status: 'completed', createdAt: '2026-10-07T00:00:00Z', threadId: 'thread', turnId: 'turn',
      kind: 'compaction', replacedTokens: count, pinnedConstraints: [] })
    expect(b?.kind === 'compaction' ? b.meta?.replacedTokens : undefined).toBeUndefined(); expect(b).not.toHaveProperty('messagesBefore')
  })
})


describe('retry lifecycle status projection', () => {
  it('keeps stable scoped identities, Host units and event ordering while withholding prose', async () => {
    const statuses: Parameters<NonNullable<ThreadEventSink['onRuntimeStatus']>>[0][] = []
    const sink: ThreadEventSink = { ...makeSink(), onRuntimeStatus: event => statuses.push(event) }
    const base = { schemaVersion: 1, source: 'transport_reconnect', logicalSequence: 7, outerAttempt: 2, physicalAttempt: 3 }
    for (const [index, phase] of ['scheduled', 'started', 'cancelled'].entries()) {
      await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: 40 + index,
        threadId: 'retry-thread', turnId: 'retry-turn', stage: 'provider_retrying',
        timestamp: '2026-10-07T00:00:00.123456789Z', attempt: 3, maxAttempt: 4,
        label: 'PRIVATE_LABEL', message: 'PRIVATE_MESSAGE', details: {
          retry: { ...base, phase, ...(phase === 'scheduled' ? { delayMs: 0, deadlineUnixMs: 1 } : {}),
            ...(phase === 'cancelled' ? { cancelReason: 'deadline_exceeded' } : {}) },
          providerError: { kind: 'network', status: 503, retryable: true, responseBody: 'PRIVATE_BODY' }
        } }, sink, async () => undefined)
    }
    expect(new Set(statuses.map(event => event.itemId)).size).toBe(1)
    expect(statuses.map(event => event.seq)).toEqual([40, 41, 42])
    expect(statuses.map(event => event.retry?.phase)).toEqual(['scheduled', 'started', 'cancelled'])
    expect(statuses[0]).toMatchObject({ threadId: 'retry-thread', turnId: 'retry-turn',
      retry: { ...base, phase: 'scheduled', delayMs: 0, deadlineUnixMs: 1 },
      meta: { details: { providerError: { kind: 'network', status: 503, retryable: true } } } })
    expect(JSON.stringify(statuses)).not.toContain('PRIVATE_')
    for (const details of [
      { ...base, physicalAttempt: 4 }, { ...base, outerAttempt: 3 }, { ...base, logicalSequence: 8 },
      { schemaVersion: 1, source: 'provider_retry', logicalSequence: 7, outerAttempt: 2 }
    ]) await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: 50 + statuses.length,
      threadId: 'retry-thread', turnId: 'retry-turn', stage: 'provider_retrying',
      details: { retry: { ...details, phase: 'started' } } }, sink, async () => undefined)
    expect(new Set(statuses.map(event => event.itemId)).size).toBe(5)
  })
  it('keeps legacy and malformed/visibleRecovery lifecycle unknown and length nonterminal', async () => {
    const statuses: Parameters<NonNullable<ThreadEventSink['onRuntimeStatus']>>[0][] = []
    const sink: ThreadEventSink = { ...makeSink(), onRuntimeStatus: event => statuses.push(event),
      onTurnComplete: vi.fn(), onError: vi.fn(), onTool: vi.fn() }
    const retry = { schemaVersion: 1, source: 'provider_retry', phase: 'started', logicalSequence: 1, outerAttempt: 2 }
    for (const details of [{}, { retry: { ...retry, privateText: 'PRIVATE_REASON' } }, { retry, visibleRecovery: true }]) {
      await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: 1 + statuses.length,
        threadId: 'retry-thread', turnId: 'retry-turn', stage: 'provider_retrying', details }, sink, async () => undefined)
    }
    expect(statuses.every(status => status.retry === undefined)).toBe(true)
    await dispatchAnalytixRuntimeEvent({ kind: 'pipeline_stage', seq: 4, threadId: 'retry-thread', turnId: 'retry-turn',
      stage: 'response_received', details: { stopReason: 'length', streamCompleted: false, logicalSequence: 1 } }, sink, async () => undefined)
    expect(statuses[3].providerOutputCap).toEqual({ stopReason: 'length', streamCompleted: false, logicalSequence: 1 })
    expect(sink.onTurnComplete).not.toHaveBeenCalled(); expect(sink.onError).not.toHaveBeenCalled(); expect(sink.onTool).not.toHaveBeenCalled()
  })
})

describe('Core user-input identity preservation', () => {
  const item = { id: 'item_owned', turnId: 'turn_owned', threadId: 'thread_owned', role: 'system' as const,
    status: 'pending', createdAt: '2026-10-08T00:00:00Z', kind: 'user_input', inputId: 'input_owned',
    prompt: 'Choose', questions: [{ id: 'input_owned_1', header: 'Path', question: 'Choose', multiSelect: true,
      options: [{ id: 'input_owned_1_option_1', label: 'Fast', description: '' }] }] }
  it('carries Core option identity and mode through item and requested event paths', async () => {
    expect(chatBlockFromItem(item)).toMatchObject({ questions: item.questions })
    const onUserInput = vi.fn()
    await dispatchAnalytixRuntimeEvent({ kind: 'user_input_requested', seq: 1, inputId: item.inputId,
      itemId: item.id, questions: [...item.questions] }, { ...makeSink(), onUserInput }, async () => undefined)
    expect(onUserInput).toHaveBeenCalledWith(expect.objectContaining({ questions: item.questions }))
  })
  it('does not silently delete an invalid option from a modern batch', () => {
    expect(chatBlockFromItem({ ...item, questions: [{ ...item.questions[0], options: [
      ...item.questions[0].options, { id: 'input_owned_1_option_2', label: '', description: '' }
    ] }] })).toMatchObject({ questions: [] })
  })
  it.each([
    { questions: [{ id: 'foreign', header: 'Path', question: 'Choose', multiSelect: false, options: [] }] },
    { questions: [...item.questions, { id: 'legacy', header: 'Path', question: 'Choose', options: [] }] }
  ])('rejects detached or partially downgraded modern batches', ({ questions }) => {
    expect(chatBlockFromItem({ ...item, questions })).toMatchObject({ questions: [] })
  })
  it('does not invent a request handle for an incomplete requested event', async () => {
    const onUserInput = vi.fn()
    await dispatchAnalytixRuntimeEvent({ kind: 'user_input_requested', seq: 1, prompt: 'Choose',
      questions: [...item.questions] }, { ...makeSink(), onUserInput }, async () => undefined)
    expect(onUserInput).not.toHaveBeenCalled()
  })
})


describe('closed Web fetch metadata through mapper input', () => {
  const success = {
    schemaVersion: 1, projectionKind: 'web_fetch_status', disclosure: 'metadata_only',
    status: 'completed', messageKey: 'tool_completed', code: 'tool_completed',
    privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
    web: { outcome: 'success', statusCode: 201, byteCount: 12, truncated: false }
  }
  it('keeps the actual non-200 status, zero bytes, and unknown read truncation', () => {
    expect(PublicToolResultProjectionV1.parse(success)).toEqual(success)
    const empty = { ...success, web: { ...success.web, statusCode: 204, byteCount: 0 } }
    expect(PublicToolResultProjectionV1.parse(empty)).toEqual(empty)
    const readError = { ...success, status: 'failed', messageKey: 'tool_failed', code: 'tool_failed',
      web: { outcome: 'read_error', statusCode: 200, byteCount: 3 } }
    expect(PublicToolResultProjectionV1.parse(readError)).toEqual(readError)
    const noResponse = { ...readError, web: { outcome: 'transport_error' } }
    expect(PublicToolResultProjectionV1.parse(noResponse)).toEqual(noResponse)
  })
  it.each([
    { statusCode: undefined }, { statusCode: 200.1 }, { statusCode: 99 }, { statusCode: 600 }, { statusCode: 503 },
    { byteCount: undefined }, { byteCount: -1 }, { byteCount: Number.MAX_SAFE_INTEGER + 1 },
    { truncated: undefined }, { truncated: 'false' }, { outcome: 'unknown' }
  ])('rejects absent or contradictory success metadata %j', web => {
    expect(PublicToolResultProjectionV1.safeParse({ ...success, web: { ...success.web, ...web } }).success).toBe(false)
  })
  it.each(['rawBody', 'text', 'title', 'snippet', 'url', 'finalUrl', 'sources', 'arbitraryJSON'])('rejects unpublished %s at both object levels', key => {
    const privateValue = 'PRIVATE_WEB_SENTINEL'
    expect(PublicToolResultProjectionV1.safeParse({ ...success, [key]: privateValue }).success).toBe(false)
    expect(PublicToolResultProjectionV1.safeParse({ ...success, web: { ...success.web, [key]: privateValue } }).success).toBe(false)
  })
  it('binds the result to exact web_fetch and the matching error lifecycle', () => {
    const item = { id: 'result', turnId: 'turn', threadId: 'thread', role: 'tool', status: 'completed',
      createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result', toolName: 'web_fetch',
      callId: 'call_host_' + 'a'.repeat(64), toolKind: 'tool_call' as const, output: success, isError: false }
    expect(TurnItem.safeParse(item).success).toBe(true)
    expect(chatBlockFromItem({ ...item, role: 'tool' as const })).toMatchObject({ kind: 'tool', meta: { publicResult: success } })
    for (const toolName of ['web_search', 'fetch_url', 'browse', 'WEB_FETCH']) {
      expect(TurnItem.safeParse({ ...item, toolName }).success).toBe(false)
      const block = chatBlockFromItem({ ...item, role: 'tool' as const, toolName })
      if (block?.kind !== 'tool') throw new Error('fixture must hydrate a tool block')
      expect(block.meta).not.toHaveProperty('publicResult')
    }
    for (const toolKind of ['command_execution', 'file_change', 'subagent']) {
      expect(TurnItem.safeParse({ ...item, toolKind }).success).toBe(false)
    }
    expect(TurnItem.safeParse({ ...item, isError: true, status: 'failed' }).success).toBe(false)
  })
  it.each(['command_execution', 'file_change', 'subagent', undefined, 'skill'])('does not attach Web metadata with raw kind %s', toolKind => {
    const item = { id: 'result', turnId: 'turn', threadId: 'thread', role: 'tool', status: 'completed',
      createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result', toolName: 'web_fetch',
      callId: 'call_host_' + 'a'.repeat(64), ...(toolKind === undefined ? {} : { toolKind }), output: success, isError: false }
    const block = chatBlockFromItem(item as unknown as CoreTurnItemJson)
    if (block?.kind !== 'tool') throw new Error('fixture must hydrate a tool block')
    expect(block.meta).not.toHaveProperty('publicResult')
  })
})


describe('settled Web fetch protected selector through real mapper ports', () => {
  const completed = {
    schemaVersion: 1, projectionKind: 'web_fetch_status', disclosure: 'metadata_only',
    status: 'completed', messageKey: 'tool_completed', code: 'tool_completed',
    privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
    web: { outcome: 'success', statusCode: 201, byteCount: 12, truncated: false }
  }
  const item: CoreTurnItemJson = {
    id: 'item_result_' + 'b'.repeat(64), turnId: 'turn_own', threadId: 'thr_own',
    createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result', role: 'tool', status: 'completed',
    toolName: 'web_fetch', callId: 'call_host_' + 'a'.repeat(64), toolKind: 'tool_call',
    output: completed, isError: false
  }
  const selector = { turnId: item.turnId, callId: item.callId, resultItemId: item.id }

  it.each([false, true])('carries the settled Web identity through replay and live events with isError=%s', async isError => {
    const output = isError ? {
      ...completed, status: 'failed', messageKey: 'tool_failed', code: 'tool_failed',
      web: { outcome: 'http_error', statusCode: 503, byteCount: 0, truncated: false }
    } : completed
    const settled: CoreTurnItemJson = { ...item, output, isError, status: isError ? 'failed' : 'completed' }
    expect(TurnItem.safeParse(settled).success).toBe(true)
    const replay = chatBlockFromItem(settled)
    expect(replay).toMatchObject({ kind: 'tool', status: isError ? 'error' : 'success',
      toolKind: 'tool_call', meta: { localResult: selector, publicResult: output } })
    if (replay?.kind !== 'tool') throw new Error('fixture must hydrate a tool block')
    expect(Object.keys(replay.meta?.localResult as object).sort()).toEqual(['callId', 'resultItemId', 'turnId'])

    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId,
      item: settled }, sink, async () => undefined)
    expect(received).toHaveLength(1)
    expect(received[0].meta).toEqual(replay.meta)
    // Later lifecycle-only updates must not overwrite the settled binding.
    for (const kind of ['tool_progress', 'tool_call_finished'] as const) {
      await dispatchAnalytixRuntimeEvent({ kind, threadId: item.threadId, turnId: item.turnId,
        callId: item.callId, toolName: 'web_fetch', status: settled.status }, sink, async () => undefined)
      expect(Object.hasOwn(received.at(-1)?.meta ?? {}, 'localResult')).toBe(false)
    }
    const hostile = chatBlockFromItem({ ...settled,
      output: { ...output, rawBody: 'PRIVATE_UNPUBLISHED_WEB_BODY', requestUrl: 'PRIVATE_UNPUBLISHED_WEB_URL' } })
    if (hostile?.kind !== 'tool') throw new Error('fixture must hydrate a tool block')
    expect(hostile.meta).not.toHaveProperty('localResult')
    expect(hostile.meta).not.toHaveProperty('publicResult')
    expect(JSON.stringify(hostile)).not.toContain('PRIVATE_UNPUBLISHED_WEB_')
  })

  it.each(['web_search', 'fetch_url', 'browse', 'WEB_FETCH', ' web_fetch '])('does not grant the Web selector to alias %s', toolName => {
    const block = chatBlockFromItem({ ...item, toolName })
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each(['command_execution', 'file_change', 'subagent', undefined, 'skill'])('requires the raw host tool_call kind instead of %s', toolKind => {
    const block = chatBlockFromItem({ ...item, toolKind } as unknown as CoreTurnItemJson)
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each(['tool_call', 'tool_progress'] as const)('does not give a %s item a settled Web selector', kind => {
    const block = chatBlockFromItem({ ...item, kind, status: 'running', output: undefined })
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
    expect(block).toMatchObject({ kind: 'tool', status: 'running' })
  })
  it.each([
    ['id', ''], ['id', 'tool_call_not_a_result'],
    ['turnId', ''], ['turnId', '../foreign'],
    ['callId', ''], ['callId', 'call_host_bad'],
    ['threadId', ''], ['threadId', '../foreign']
  ])('requires the complete Core binding %s=%s', (field, value) => {
    const block = chatBlockFromItem({ ...item, [field]: value })
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each(['pending', 'running'] as const)('does not allocate a settled selector to a valid %s result', status => {
    const pending = { ...item, status, output: {
      schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'unknown',
      messageKey: 'tool_outcome_unknown', code: 'tool_outcome_unknown_after_restart',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false
    } }
    expect(TurnItem.safeParse(pending).success).toBe(true)
    const block = chatBlockFromItem(pending)
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each([
    { status: 'completed', isError: true }, { status: 'failed', isError: false },
    { status: 'aborted', isError: false }, { status: undefined, isError: false }
  ])('rejects contradictory or missing settlement lifecycle %j', lifecycle => {
    const malformed = { ...item, ...lifecycle } as unknown as CoreTurnItemJson
    expect(TurnItem.safeParse(malformed).success).toBe(false)
    const block = chatBlockFromItem(malformed)
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it('keeps two Core calls and result identities distinct through replay merging and live dispatch', async () => {
    const other: CoreTurnItemJson = { ...item, callId: 'call_host_' + 'c'.repeat(64), id: 'item_result_' + 'd'.repeat(64) }
    const calls = [item, other].map<CoreTurnItemJson>(value => ({ ...value, kind: 'tool_call', status: 'running', output: undefined }))
    const replay = mergeChatBlocks([...calls, item, other].map(value => chatBlockFromItem(value)!).filter(Boolean))
    expect(replay).toHaveLength(2)
    expect(replay.map(block => block.kind === 'tool' ? block.meta?.localResult : undefined)).toEqual([
      selector, { turnId: other.turnId, callId: other.callId, resultItemId: other.id }
    ])
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    for (const value of [item, other]) {
      await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: value.threadId, turnId: value.turnId,
        item: value }, sink, async () => undefined)
    }
    expect(received.map(event => event.meta?.localResult)).toEqual(replay.map(block => block.kind === 'tool' ? block.meta?.localResult : undefined))
  })
})


describe('settled Search protected selectors through real mapper ports', () => {
  const completed = {
    schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only',
    status: 'completed', messageKey: 'tool_completed', code: 'tool_completed',
    privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false
  }
  const item: CoreTurnItemJson = {
    id: 'item_result_' + 'e'.repeat(64), turnId: 'turn_search', threadId: 'thr_search',
    createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result', role: 'tool', status: 'completed',
    toolName: 'grep', callId: 'call_host_' + 'f'.repeat(64), toolKind: 'tool_call',
    output: completed, isError: false
  }
  const selector = { turnId: item.turnId, callId: item.callId, resultItemId: item.id }

  it.each([['grep', false], ['glob', false], ['grep', true], ['glob', true]] as const)(
    'carries exact %s settled identity through hydration and live items with isError=%s', async (toolName, isError) => {
      const output = isError ? { ...completed, status: 'failed', messageKey: 'tool_failed', code: 'tool_failed' } : completed
      const settled: CoreTurnItemJson = { ...item, toolName, output, isError, status: isError ? 'failed' : 'completed' }
      expect(TurnItem.safeParse(settled).success).toBe(true)
      const replay = chatBlockFromItem(settled)
      expect(replay).toMatchObject({ kind: 'tool', status: isError ? 'error' : 'success',
        toolKind: 'tool_call', meta: { localResult: selector, publicResult: output } })
      if (replay?.kind !== 'tool') throw new Error('fixture must hydrate a tool block')
      expect(Object.keys(replay.meta?.localResult as object).sort()).toEqual(['callId', 'resultItemId', 'turnId'])

      const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
      const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
      await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId,
        item: settled }, sink, async () => undefined)
      expect(received).toHaveLength(1)
      expect(received[0].meta?.localResult).toEqual(selector)
      for (const kind of ['tool_progress', 'tool_call_finished'] as const) {
        await dispatchAnalytixRuntimeEvent({ kind, threadId: item.threadId, turnId: item.turnId,
          callId: item.callId, toolName, status: settled.status }, sink, async () => undefined)
        expect(received.at(-1)?.meta).not.toHaveProperty('localResult')
      }
    }
  )

  it.each([' grep ', ' glob '])(
    'does not normalize the original producer name %s to grant a locator', toolName => {
      const block = chatBlockFromItem({ ...item, toolName })
      expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
    }
  )
  it.each(['GREP', 'GLOB', 'grep_files', 'search', 'search_files', 'find', 'ls', 'code_index', 'RUN_SKILL', 'goal', 'skill'])(
    'preserves ordinary producer %s for keyed Inspect without rewriting it as a specialized tool', toolName => {
      const block = chatBlockFromItem({ ...item, toolName })
      expect(block).toMatchObject({ kind: 'tool', toolKind: 'tool_call', meta: { toolName, localResult: selector, publicResult: completed } })
      expect(Object.keys(block?.kind === 'tool' ? block.meta?.localResult as object : {}).sort()).toEqual(['callId', 'resultItemId', 'turnId'])
    }
  )
  it.each(['command_execution', 'file_change', 'subagent', undefined, 'skill'])(
    'requires the raw host tool_call kind rather than %s', toolKind => {
      const block = chatBlockFromItem({ ...item, toolKind } as unknown as CoreTurnItemJson)
      expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
    }
  )
  it.each(['tool_call', 'tool_progress'] as const)('does not give a %s item a settled Search selector', kind => {
    const block = chatBlockFromItem({ ...item, kind, status: 'running', output: undefined })
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each([
    ['id', ''], ['id', 'not_a_result'], ['turnId', '../foreign'],
    ['callId', 'call_host_bad'], ['threadId', '']
  ])('requires a complete valid identity: %s=%s', (field, value) => {
    const block = chatBlockFromItem({ ...item, [field]: value })
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each(['pending', 'running'] as const)('does not grant a valid %s result a settled selector', status => {
    const unsettled = { ...item, status, output: { ...completed, status: 'unknown',
      messageKey: 'tool_outcome_unknown', code: 'tool_outcome_unknown_after_restart' } }
    expect(TurnItem.safeParse(unsettled).success).toBe(true)
    const block = chatBlockFromItem(unsettled)
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it.each([
    { status: 'completed', isError: true }, { status: 'failed', isError: false },
    { status: 'aborted', isError: false }, { status: undefined, isError: false }
  ])('rejects contradictory or missing settlement lifecycle %j', lifecycle => {
    const malformed = { ...item, ...lifecycle } as unknown as CoreTurnItemJson
    expect(TurnItem.safeParse(malformed).success).toBe(false)
    const block = chatBlockFromItem(malformed)
    expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
  })
  it('keeps raw search output out of selectors and renderer state', () => {
    const block = chatBlockFromItem({ ...item, output: { ...completed,
      pattern: 'PRIVATE_SEARCH_PATTERN', files: [{ path: 'PRIVATE_SEARCH_PATH',
        matches: [{ lineNumber: 1, line: 'PRIVATE_SEARCH_LINE' }] }] } })
    if (block?.kind !== 'tool') throw new Error('fixture must hydrate a tool block')
    expect(block.meta).not.toHaveProperty('localResult')
    expect(block.meta).not.toHaveProperty('publicResult')
    expect(JSON.stringify(block)).not.toContain('PRIVATE_SEARCH_')
  })
  it('keeps distinct settled call and result identities separate', () => {
    const other: CoreTurnItemJson = { ...item, toolName: 'glob',
      callId: 'call_host_' + 'a'.repeat(64), id: 'item_result_' + 'b'.repeat(64) }
    const replay = mergeChatBlocks([item, other].map(value => chatBlockFromItem(value)!))
    expect(replay).toHaveLength(2)
    expect(replay.map(block => block.kind === 'tool' ? block.meta?.localResult : undefined)).toEqual([
      selector, { turnId: other.turnId, callId: other.callId, resultItemId: other.id }
    ])
  })
})


describe('historical Core DTO isolation', () => {
  const host = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
    messageKey: 'tool_completed', code: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
  const rendererContext = { workspaceCheckpointId: 'checkpoint_turn', attachmentIds: ['att_turn'],
    activeSkillIds: ['skill_turn'], injectedMemoryIds: ['memory_turn'], skillInjectionBytes: 12 }
  const fixture = (toolName = 'web_fetch'): CoreTurnItemJson => ({
    id: 'item_result_' + '7'.repeat(64), callId: 'call_host_' + '8'.repeat(64), turnId: 'turn_history', threadId: 'thread_history',
    createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result', role: 'tool', toolKind: 'tool_call', status: 'completed', isError: false, toolName,
    output: toolName === 'web_fetch' ? { ...host, projectionKind: 'web_fetch_status', web: { outcome: 'success', statusCode: 201, byteCount: 7, truncated: false } } : host
  })
  it.each(['web_fetch', 'grep', 'glob'])('validates the original %s Core result and projects turn context separately', toolName => {
    const item = Object.freeze(fixture(toolName)), before = JSON.stringify(item)
    expect(TurnItem.safeParse(item).success).toBe(true)
    const block = chatBlockFromItem(item, undefined, rendererContext)
    expect(block).toMatchObject({ kind: 'tool', meta: { ...rendererContext, localResult: {
      turnId: item.turnId, callId: item.callId, resultItemId: item.id } } })
    expect(JSON.stringify(item)).toBe(before)
    expect(TurnItem.safeParse(item).success).toBe(true)
    expect(item).not.toHaveProperty('activeSkillIds')
  })
  it.each(['workspaceCheckpointId', 'attachmentIds', 'activeSkillIds', 'injectedMemoryIds', 'skillInjectionBytes', 'untrustedExtra'])(
    'does not erase the original unknown field %s to grant protected access', field => {
      const item = { ...fixture(), [field]: 'forged' }
      expect(TurnItem.safeParse(item).success).toBe(false)
      const block = chatBlockFromItem(item, undefined, rendererContext)
      expect(block?.kind === 'tool' ? block.meta?.localResult : undefined).toBeUndefined()
      expect(item).toHaveProperty(field, 'forged')
    })
})

describe('Footer receipt primary binding', () => {
  const scope = { threadId: 'thr_general', includeHistoricalBlocks: false }
  const primaryFrom = (batch: Record<string, unknown> = typedGeneralTerminalBatch()) => {
    const block = chatBlockFromItem((batch.events as Array<{ item?: CoreTurnItemJson }>)[0].item!)!
    if (block.kind !== 'assistant') throw new Error('Expected a real projected assistant fixture.')
    return block
  }
  const laterBatch = (itemId?: string): Record<string, unknown> => {
    const batch = typedGeneralTerminalBatch()
    const events = batch.events as Array<Record<string, unknown>>
    const manifest = batch.eventManifest as unknown[]
    if (itemId) {
      events[0].itemId = itemId
      const item = events[0].item as Record<string, unknown>
      item.id = itemId
    } else {
      events.shift(); manifest.shift()
    }
    batch.firstSeq = 15; batch.lastSeq = 14 + events.length; batch.seq = batch.lastSeq
    events.forEach((event, index) => { event.seq = 15 + index })
    expect(generalTerminalProjectionBatchFromRuntime(batch)).not.toBeNull()
    return batch
  }
  it('retains the verified receipt on an exact primary item without replacing its content or adding replay history', () => {
    const batch = typedGeneralTerminalBatch('Exact primary answer')
    const source = generalTerminalProjectionBatchFromRuntime(batch)!.terminalItem!
    const item = (batch.events as Array<{ item?: CoreTurnItemJson }>)[0].item!
    const primary = chatBlockFromItem(item)!
    expect(primary).not.toHaveProperty('generalTerminalProjectionReceipt')
    const replay = createOrdinaryHistoryReplay('thr_general', 13)
    replay.accept([{ kind: 'item_created', threadId: 'thr_general', turnId: 'old-turn', seq: 1,
      item: { id: 'replay-only-user', threadId: 'thr_general', turnId: 'old-turn', role: 'user', kind: 'user_message',
        status: 'completed', createdAt: item.createdAt, text: 'Replay-only history' } }])
    replay.accept([batch])
    const first: ChatBlock = { kind: 'user', id: 'primary-user', text: 'Primary request', meta: { turnId: 'turn_general' } }
    const extra: ChatBlock = { kind: 'system', id: 'primary-extra', text: 'Primary extra', meta: { turnId: 'turn_general' } }
    const snapshot = [first, primary, extra], before = JSON.stringify(snapshot)
    const merged = replay.mergeSnapshot(snapshot, { threadId: 'thr_general', includeHistoricalBlocks: false })
    expect(merged.find(block => block.id === primary.id)).toMatchObject({ generalTerminalProjectionReceipt: { threadId: 'thr_general', turnId: 'turn_general',
      itemId: source.id, firstSeq: 11, lastSeq: 13, batchDigest: 'a'.repeat(64), status: 'completed' } })
    expect(merged.map(block => block.id)).toEqual(snapshot.map(block => block.id))
    expect(merged[0]).toBe(first); expect(merged[2]).toBe(extra)
    expect(JSON.stringify(snapshot)).toBe(before)
    expect(merged[1]).toMatchObject(primary)
    expect(merged.some(isOrdinaryDisplayHistoryBlock)).toBe(false)
  })
  it.each(['turn', 'missing turn', 'item', 'text', 'time', 'missing time', 'illegal time'])(
    'does not bind a primary with mismatched %s', field => {
      const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
      switch (field) {
        case 'turn': primary.meta = { ...primary.meta, turnId: 'other-turn' }; break
        case 'missing turn': primary.meta = {}; break
        case 'item': primary.id = 'other-item'; break
        case 'text': primary.text += ' changed'; break
        case 'time': primary.createdAt = '2026-07-20T03:00:01Z'; break
        case 'missing time': delete primary.createdAt; break
        case 'illegal time': primary.createdAt = 'invalid'; break
      }
      const replay = createOrdinaryHistoryReplay('thr_general', 13); replay.accept([batch])
      const before = JSON.stringify(primary)
      expect(replay.mergeSnapshot([primary], scope)).toEqual([primary])
      expect(JSON.stringify(primary)).toBe(before)
      expect(primary).not.toHaveProperty('generalTerminalProjectionReceipt')
    })
  it('requires explicit matching primary scope and preserves the legacy call without grafting authority', () => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
    const replay = createOrdinaryHistoryReplay('thr_general', 13); replay.accept([batch])
    expect(replay.mergeSnapshot([primary])).toEqual([primary])
    expect(() => replay.mergeSnapshot([primary], { ...scope, threadId: 'other-thread' })).toThrow(/thread identity/)
    expect(primary).not.toHaveProperty('generalTerminalProjectionReceipt')
  })
  it.each([false, true])('rejects duplicate physical IDs before filtering content; conflicting text: %s', conflicting => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
    const duplicate = { ...primary, text: conflicting ? 'Changed duplicate' : primary.text }
    const replay = createOrdinaryHistoryReplay('thr_general', 13); replay.accept([batch])
    const snapshot = [primary, duplicate]
    expect(replay.mergeSnapshot(snapshot, scope)).toEqual(snapshot)
    expect(snapshot.every(block => !block.generalTerminalProjectionReceipt)).toBe(true)
  })
  it.each(['new-item', 'item_general', undefined])('refuses old or overwritten receipts after a newer same-turn batch: %s', itemId => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch), later = laterBatch(itemId)
    const replay = createOrdinaryHistoryReplay('thr_general', later.lastSeq as number)
    replay.accept([batch]); replay.accept([later])
    // For same-ID overwrite the exact latest item is still ambiguous; for a
    // new/no item batch the old candidate is outside the latest turn bounds.
    expect(replay.mergeSnapshot([primary], scope)).toEqual([primary])
    expect(primary).not.toHaveProperty('generalTerminalProjectionReceipt')
  })
  it('does not let a returned usage entry revive a stale terminal receipt', () => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch), later = laterBatch()
    const replay = createOrdinaryHistoryReplay('thr_general', later.lastSeq as number)
    replay.accept([batch]); replay.accept([later])
    const returned = replay.turnUsage()
    expect(returned.turn_general.firstSeq).toBe(15)
    returned.turn_general.firstSeq = 11; returned.turn_general.lastSeq = 13
    expect(replay.mergeSnapshot([primary], scope)).toEqual([primary])
    expect(primary).not.toHaveProperty('generalTerminalProjectionReceipt')
    expect(replay.turnUsage().turn_general.firstSeq).toBe(15)
  })
  it('ignores replay duplicates idempotently and does not expose a mutable verified receipt alias', () => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
    const replay = createOrdinaryHistoryReplay('thr_general', 13); replay.accept([batch]); replay.accept([batch])
    const bound = replay.mergeSnapshot([primary], scope)[0] as typeof primary
    expect(bound.generalTerminalProjectionReceipt?.lastSeq).toBe(13)
    expect(replay.mergeSnapshot([bound], scope)[0]).toBe(bound)
    bound.generalTerminalProjectionReceipt!.batchDigest = 'f'.repeat(64)
    expect((replay.mergeSnapshot([primary], scope)[0] as typeof primary).generalTerminalProjectionReceipt?.batchDigest).toBe('a'.repeat(64))
    const historical = replay.mergeSnapshot([])[0] as typeof primary
    historical.generalTerminalProjectionReceipt!.lastSeq = 99
    expect((replay.mergeSnapshot([primary], scope)[0] as typeof primary).generalTerminalProjectionReceipt?.lastSeq).toBe(13)
  })
  it.each(['newer', 'foreign', 'different digest'])('keeps an existing %s primary proof without replacement', variant => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
    const replay = createOrdinaryHistoryReplay('thr_general', 13); replay.accept([batch])
    const bound = replay.mergeSnapshot([primary], scope)[0] as typeof primary
    const receipt = { ...bound.generalTerminalProjectionReceipt! }
    if (variant === 'newer') { receipt.firstSeq = 20; receipt.lastSeq = 22 }
    if (variant === 'foreign') receipt.threadId = 'other-thread'
    if (variant === 'different digest') receipt.batchDigest = 'f'.repeat(64)
    const protectedPrimary = { ...primary, generalTerminalProjectionReceipt: receipt }
    expect(replay.mergeSnapshot([protectedPrimary], scope)[0]).toBe(protectedPrimary)
    expect(protectedPrimary.generalTerminalProjectionReceipt).toBe(receipt)
  })
  it('does not add ordinary proof to accepted-final ownership', () => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
    const replay = createOrdinaryHistoryReplay('thr_general', 13); replay.accept([batch])
    const protections: Array<Partial<typeof primary>> = [
      { acceptedFinalView: { schemaVersion: 3, acceptedFinalDigest: 'f'.repeat(64), publicationState: 'accepted',
        variant: 'SourceUnavailableAnswer', terminalReason: 'source_unavailable', blockerCode: 'current_case_source_unavailable',
        coverageStatus: 'unavailable', checkedScopeDigest: '', missingScopeCount: 0, claimCount: 0, claimTypes: [],
        receiptMetadata: { projection: 'masked_metadata_only', count: 0, setDigest: '8'.repeat(64), citations: [] },
        noHitWording: '', acceptedAt: primary.createdAt! } },
      { acceptedFinalProjectionReceipt: { schemaVersion: 1, batchId: 'b'.repeat(64), threadId: 'thr_general',
        turnId: 'turn_general', publicationCommitId: 'c'.repeat(64), lastSeq: 13 } },
      { acceptedFinalProjectionTerminal: { status: 'completed', createdAt: primary.createdAt!, acceptedFinalDigest: 'c'.repeat(64), terminalReason: 'success' } }
    ]
    for (const protection of protections) {
      const protectedPrimary = { ...primary, ...protection }
      expect(replay.mergeSnapshot([protectedPrimary], scope)[0]).toBe(protectedPrimary)
      expect(protectedPrimary).not.toHaveProperty('generalTerminalProjectionReceipt')
    }
  })
  it('does not reintroduce a primary or receipt removed by a verified rewind', () => {
    const batch = typedGeneralTerminalBatch(), primary = primaryFrom(batch)
    const replay = createOrdinaryHistoryReplay('thr_general', 14); replay.accept([batch])
    replay.accept([{ kind: 'thread_rewound', threadId: 'thr_general', seq: 14, removedTurnIds: ['turn_general'] }])
    expect(replay.mergeSnapshot([primary], scope)).toEqual([])
    expect(primary).not.toHaveProperty('generalTerminalProjectionReceipt')
  })

})

describe('ordinary display history replay', () => {
  const user = (id: string, turnId: string, seq: number): CoreRuntimeEventJson => ({
    kind: 'item_created', threadId: 'thr_general', turnId, seq,
    item: { id, threadId: 'thr_general', turnId, kind: 'user_message', role: 'user', status: 'completed', createdAt: '2026-10-08T00:00:00Z', text: id }
  })
  it('keeps replay chronology, primary turn contents and one current compaction marker', () => {
    const replay = createOrdinaryHistoryReplay('thr_general', 16)
    replay.accept([user('user-old', 'turn_general', 1)])
    replay.accept([typedGeneralTerminalBatch('authorized old output')])
    replay.accept([user('user-tail', 'tail-turn', 14), {
      kind: 'compaction_completed', threadId: 'thr_general', turnId: 'compact-turn', seq: 16,
      summary: 'Context compacted', pinnedConstraints: ['constraints are not a summary']
    }])
    const compact: ChatBlock = { kind: 'compaction', id: 'real-compact-item', status: 'success',
      summary: 'Context compacted', meta: { turnId: 'compact-turn' } }
    const tail: ChatBlock = { kind: 'user', id: 'user-tail', text: 'primary tail', meta: { turnId: 'tail-turn' } }
    const process: ChatBlock = { kind: 'system', id: 'tail-process', text: 'primary process', meta: { turnId: 'tail-turn' } }
    const merged = replay.mergeSnapshot([compact, tail, process])
    expect(merged.map(block => block.id)).toEqual(['user-old', 'item_general', 'user-tail', 'tail-process', 'real-compact-item'])
    expect(merged[1]).toMatchObject({ kind: 'assistant', text: 'authorized old output',
      generalTerminalProjectionReceipt: { schemaVersion: 1, batchDigest: 'a'.repeat(64),
        threadId: 'thr_general', turnId: 'turn_general', itemId: 'item_general',
        firstSeq: 11, lastSeq: 13, status: 'completed' } })
    expect(merged.slice(0, 2).every(isOrdinaryDisplayHistoryBlock)).toBe(true)
    expect(merged[1]).toHaveProperty('displayHistory', { source: 'host_public_replay', threadId: 'thr_general', throughSeq: 16 })
    expect(merged.slice(2).some(isOrdinaryDisplayHistoryBlock)).toBe(false)
    expect(merged[2]).toBe(tail)
    expect(merged.at(-1)).not.toHaveProperty('detail')
    expect(replay.mergeSnapshot([compact, tail, process])).toEqual(merged)
  })
  it('ignores drafts and recoverable diagnostics, applies rewind and omits orphan started markers', () => {
    const replay = createOrdinaryHistoryReplay('thr_general', 17)
    replay.accept([user('removed-user', 'turn_general', 1)])
    replay.accept([typedGeneralTerminalBatch('removed output')])
    replay.accept([
      { kind: 'error', threadId: 'thr_general', seq: 14, terminal: false, code: 'budget_warning' },
      { kind: 'compaction_started', threadId: 'thr_general', turnId: 'obsolete-compact', seq: 15 },
      { kind: 'thread_rewound', threadId: 'thr_general', seq: 16, removedTurnIds: ['turn_general'] },
      { kind: 'item_completed', threadId: 'thr_general', turnId: 'draft-turn', seq: 17,
        item: { id: 'draft', threadId: 'thr_general', turnId: 'draft-turn', role: 'assistant',
          kind: 'assistant_text', status: 'completed', createdAt: '2026-10-08T00:00:00Z', text: 'unverified draft' } }
    ])
    expect(replay.mergeSnapshot([{ kind: 'user', id: 'removed-user', text: 'old primary', meta: { turnId: 'turn_general' } }])).toEqual([])
  })
  it('binds historical usage to the original terminal turn and committed sequence', () => {
    const replay = createOrdinaryHistoryReplay('thr_general', 13)
    expect(() => replay.turnUsage()).toThrow(/boundary/)
    replay.accept([typedGeneralTerminalBatch('verified old output')])
    replay.accept([typedGeneralTerminalBatch('verified old output')])
    expect(Object.keys(replay.turnUsage())).toEqual(['turn_general'])
    expect(replay.turnUsage().turn_general).toMatchObject({ threadId: 'thr_general', turnId: 'turn_general',
      firstSeq: 11, lastSeq: 13, snapshot: { inputTokens: 2, outputTokens: 1, totalTokens: 3, cacheHitRate: null, costUsd: null } })
    expect(replay.turnUsage().turn_general.snapshot.cacheDiagnostics).toBeUndefined()
  })
  it('grants deletion only to full replay and fences old snapshots after a live rewind', () => {
    const batch = generalTerminalProjectionBatchFromRuntime(generalTerminalBatch())!
    const old = turnUsageEntryFromBatch(batch), future = { ...old, turnId: 'future', firstSeq: 20, lastSeq: 22 }
    const current = { turn_general: old, future }
    expect(reconcileTurnUsageById('thr_general', {}, current)).toEqual(current)
    expect(reconcileTurnUsageById('thr_general', {}, current, 13)).toEqual({ future })
    expect(reconcileTurnUsageById('thr_general', {}, current, NaN)).toEqual(current)
    expect(reconcileTurnUsageById('thr_general', current, { future }, undefined, { turn_general: true })).toEqual({ future })
    const constructorEntry = { ...old, turnId: 'constructor' }
    expect(mergeTurnUsageById('thr_general', { ['constructor']: constructorEntry })).toHaveProperty('constructor', constructorEntry)
    expect(mergeTurnUsageById('other', current)).toEqual({})
    expect(filterRewoundDisplayBlocks([{ kind: 'user', id: 'old', text: 'removed', meta: { turnId: 'turn_general' } },
      { kind: 'user', id: 'valid', text: 'history', meta: { turnId: 'future' } }], { turn_general: true })).toMatchObject([{ id: 'valid' }])
  })
  it('removes only the Host-named historical usage on replayed rewind', () => {
    const replay = createOrdinaryHistoryReplay('thr_general', 18)
    replay.accept([generalTerminalBatch()])
    // Preserve every identity binding in the second consumer-delivery fixture.
    const keptBatch = JSON.parse(JSON.stringify(generalTerminalBatch()).replaceAll('"turn_general"', '"kept-turn"')
      .replaceAll('"item_general"', '"kept-item"')) as Record<string, unknown>
    keptBatch.firstSeq = 15; keptBatch.lastSeq = 17; keptBatch.seq = 17
    for (const [index, event] of (keptBatch.events as Array<Record<string, unknown>>).entries()) event.seq = 15 + index
    replay.accept([keptBatch])
    replay.accept([{ kind: 'thread_rewound', threadId: 'thr_general', seq: 18, removedTurnIds: ['turn_general'] }])
    expect(Object.keys(replay.turnUsage())).toEqual(['kept-turn'])
    expect(replay.turnUsage()['kept-turn']).toMatchObject({ threadId: 'thr_general', turnId: 'kept-turn', firstSeq: 15, lastSeq: 17 })
  })

  it('rejects incomplete, cross-thread and mixed atomic history', () => {
    expect(() => createOrdinaryHistoryReplay('thr_general', 13).mergeSnapshot([])).toThrow(/boundary/)
    expect(() => createOrdinaryHistoryReplay('other', 13).accept([generalTerminalBatch()])).toThrow(/boundary/)
    expect(() => createOrdinaryHistoryReplay('thr_general', 12).accept([generalTerminalBatch()])).toThrow(/boundary/)
    expect(() => createOrdinaryHistoryReplay('thr_general', 13).accept([user('u', 'turn_general', 1), generalTerminalBatch()])).toThrow(/Mixed/)
  })
})

describe('specialized nine-key settled mapper seams', () => {
  const host = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
    code: 'tool_completed', messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
  const item: CoreTurnItemJson = { id: 'item_result_' + 'a'.repeat(64), turnId: 'turn-specialized', threadId: 'thread-specialized',
    createdAt: '2026-10-09T00:00:00Z', kind: 'tool_result', role: 'tool', status: 'completed', toolName: 'create_goal',
    callId: 'call_host_' + 'b'.repeat(64), toolKind: 'tool_call', output: host, isError: false }
  const selector = { turnId: item.turnId, callId: item.callId, resultItemId: item.id }
  const child: CoreChildRuntimeMetadataJson = { parentThreadId: 'thread-specialized', parentTurnId: 'turn-specialized',
    childId: 'child-a', childStatus: 'completed', kind: 'subagent', parentToolCallId: item.callId,
    childThreadId: 'child-thread', childTurnId: 'child-turn', background: true }
  it.each(['create_goal', 'get_goal', 'update_goal', 'todo_write', 'run_skill'])('preserves settled %s selector through hydration and live item dispatch', async toolName => {
    const value = { ...item, toolName }
    expect(TurnItem.safeParse(value).success).toBe(true)
    const replay = chatBlockFromItem(value)
    expect(replay).toMatchObject({ kind: 'tool', toolKind: 'tool_call', meta: { toolName, localResult: selector } })
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId, item: value }, sink, async () => undefined)
    expect(received).toHaveLength(1); expect(received[0].meta?.localResult).toEqual(selector)
    await dispatchAnalytixRuntimeEvent({ kind: 'tool_call_finished', threadId: item.threadId, turnId: item.turnId,
      callId: item.callId, toolName, status: 'completed' }, sink, async () => undefined)
    expect(received.at(-1)?.meta).not.toHaveProperty('localResult')
  })
  it('retains real run_skill identity through hydration, live dispatch and the native child projector', async () => {
    const value = { ...item, toolName: 'run_skill' }
    const result = chatBlockFromItem(value, child)
    expect(result).toMatchObject({ kind: 'tool', toolKind: 'tool_call', meta: { toolName: 'run_skill', localResult: selector,
      child: { childId: 'child-a', childThreadId: 'child-thread', parentToolCallId: item.callId } } })
    expect(result?.kind === 'tool' ? result.detail : '').not.toContain('childThreadId')
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId, item: value, child },
      { ...makeSink(), onTool: event => received.push(event) }, async () => undefined)
    const projected = projectToolEventForRenderer(received[0])
    expect(projected).toMatchObject({ toolKind: 'tool_call', status: 'success', meta: { toolName: 'run_skill', localResult: selector } })
    expect(projectToolEventForRenderer(projected!)).toEqual(projected)
  })
  it.each([
    { toolKind: 'subagent' }, { toolKind: 'command_execution' },
    { callId: 'foreign' }, { id: 'foreign' }, { threadId: '' }, { status: 'running' }, { isError: true },
    { output: { ...host, objective: 'PRIVATE_INPUT', todo: [{ content: 'PRIVATE_INPUT' }] } }
  ])('rejects an unadmitted specialized selector %j', patch => {
    const result = chatBlockFromItem({ ...item, ...patch } as CoreTurnItemJson)
    expect(result?.kind === 'tool' ? result.meta?.localResult : undefined).toBeUndefined()
    expect(JSON.stringify(result)).not.toContain('PRIVATE_INPUT')
  })
  it.each([['list_jobs', 'list'], ['bash_output', 'output'], ['kill_shell', 'kill'], ['wait', 'wait']] as const)(
    'carries %s recorded %s data through strict hydration and live items', async (toolName, operation) => {
      const job = taskJobSummaryV1({ id: 'job-specialized', kind: 'bash', status: 'failed', background: true })
      const result = operation === 'list' ? { jobs: [job], count: 1 } : operation === 'wait' ? { jobs: [job] } : operation === 'kill' ? { job } :
        { schemaVersion: 1, availability: 'withheld', jobId: job.id, status: 'failed', outputWithheld: true,
          reasonCode: 'security_bound_child_output', outputTrustStatus: 'untrusted_child_output', factAnswerAllowed: false,
          evidenceAuthority: false, canReadOutput: false, canContinueParent: false }
      const output = { ...host, projectionKind: 'task_job_result', taskJob: { operation, result } }
      const value: CoreTurnItemJson = { ...item, toolName, toolKind: 'command_execution', output }
      expect(TurnItem.safeParse(value).success).toBe(true)
      expect(chatBlockFromItem(value)).toMatchObject({ kind: 'tool', status: 'success', meta: { toolName, publicResult: output, localResult: selector } })
      const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
      await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId, item: value },
        { ...makeSink(), onTool: event => received.push(event) }, async () => undefined)
      expect(received[0].meta?.publicResult).toEqual(output); expect(received[0].meta?.localResult).toEqual(selector)
      for (const patch of [{ toolName: 'foreign' }, { toolKind: 'tool_call' }, { isError: true }, { callId: 'foreign' }]) {
        const rejected = chatBlockFromItem({ ...value, ...patch } as CoreTurnItemJson)
        expect(rejected?.kind === 'tool' ? rejected.meta?.publicResult : undefined).toBeUndefined()
        expect(rejected?.kind === 'tool' ? rejected.meta?.localResult : undefined).toBeUndefined()
      }
    })

  it.each(['ls', 'mcp__example__customTool'])('carries actual ordinary %s result identity through GET and live items without a tool-name allowlist', async toolName => {
    const value = { ...item, toolName }
    expect(TurnItem.safeParse(value).success).toBe(true)
    const replay = chatBlockFromItem(value)
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId, item: value }, sink, async () => undefined)
    expect(received[0].meta).toEqual(replay?.kind === 'tool' ? replay.meta : undefined)
    expect(received[0].meta).toMatchObject({ toolName, localResult: selector })
    for (const kind of ['tool_progress', 'tool_call_finished'] as const) {
      await dispatchAnalytixRuntimeEvent({ kind, threadId: item.threadId, turnId: item.turnId, callId: item.callId, toolName, status: 'completed' }, sink, async () => undefined)
      expect(received.at(-1)?.meta).not.toHaveProperty('localResult')
    }
  })
  it('keeps failed Job settlement readable without inventing a successful public Job result', () => {
    const value: CoreTurnItemJson = { ...item, toolName: 'kill_shell', toolKind: 'command_execution', status: 'failed', isError: true,
      output: { ...host, status: 'failed', code: 'tool_failed', messageKey: 'tool_failed' } }
    expect(TurnItem.safeParse(value).success).toBe(true)
    expect(chatBlockFromItem(value)).toMatchObject({ kind: 'tool', status: 'error', meta: { toolName: 'kill_shell', localResult: selector } })
    for (const toolName of ['kill_shell', 'restart_job']) {
      const wrongKind = chatBlockFromItem({ ...value, toolName, toolKind: 'tool_call' })
      expect(wrongKind?.kind === 'tool' ? wrongKind.meta?.localResult : undefined).toBeUndefined()
    }
  })
  it('keeps a settled child result on its own coordinates while the parent remains active', async () => {
    const value: CoreTurnItemJson = { ...item, toolName: 'ls', threadId: child.childThreadId!, turnId: child.childTurnId!,
      callId: 'call_host_' + 'c'.repeat(64), id: 'item_result_' + 'd'.repeat(64) }
    const activeChild: CoreChildRuntimeMetadataJson = { ...child, childStatus: 'running' }
    const ownSelector = { turnId: value.turnId, callId: value.callId, resultItemId: value.id }
    expect(chatBlockFromItem(value, activeChild)).toMatchObject({ kind: 'tool', meta: { localResult: ownSelector, recordThreadId: value.threadId } })
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: value.threadId, turnId: value.turnId, item: value, child: activeChild },
      { ...makeSink(), onTool: event => received.push(event) }, async () => undefined)
    expect(projectToolEventForRenderer(received[0])).toMatchObject({ toolKind: 'tool_call', status: 'success',
      meta: { toolName: 'ls', localResult: ownSelector, recordThreadId: value.threadId } })
  })
  it('rejects foreign, unsettled and payload-bearing child locators at the final live projector', async () => {
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: item.threadId, turnId: item.turnId,
      item: { ...item, toolName: 'run_skill' }, child }, { ...makeSink(), onTool: event => received.push(event) }, async () => undefined)
    const event = received[0], meta = event.meta!
    for (const patch of [
      { runtimeStatus: 'tool_progress' }, { sourceItemId: 'foreign-result' }, { recordThreadId: 'foreign-thread' },
      { localResult: { ...selector, turnId: 'foreign-turn' } }, { localResult: { ...selector, callId: 'call_host_' + 'c'.repeat(64) } },
      { localResult: { ...selector, resultItemId: 'item_result_' + 'd'.repeat(64) } },
      { localResult: { ...selector, inputJSON: 'PRIVATE_INPUT' } }, { child: { ...child, parentToolCallId: 'foreign-call' } }
    ]) {
      const projected = projectToolEventForRenderer({ ...event, meta: { ...meta, ...patch } })
      expect(projected?.meta).not.toHaveProperty('localResult')
      expect(projected?.meta).not.toHaveProperty('toolName')
      expect(JSON.stringify(projected)).not.toContain('PRIVATE_INPUT')
    }
    expect(projectToolEventForRenderer({ ...event, status: 'running' })?.meta).not.toHaveProperty('localResult')
    expect(projectToolEventForRenderer({ ...event, toolKind: 'subagent' })?.meta).not.toHaveProperty('localResult')
    const wrongChild = chatBlockFromItem({ ...item, toolName: 'run_skill' }, { ...child, parentToolCallId: 'foreign-call' })
    expect(wrongChild?.kind === 'tool' ? wrongChild.meta : undefined).not.toHaveProperty('localResult')
  })
})


describe('conversation record physical identity admission', () => {
  const base = { threadId: 'record-thread', turnId: 'record-turn', callId: 'record-call', toolName: 'get_goal', toolKind: 'tool_call', createdAt: '2026-10-09T00:00:00Z' } as const
  const args = { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
    privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
  const output = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed', code: 'tool_completed',
    messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
  it('retains strict call and result witnesses through actual live item dispatch and the final restricted projector', async () => {
    const received: Array<Parameters<ThreadEventSink['onTool']>[0]> = []
    const sink = { ...makeSink(), onTool: (event: Parameters<ThreadEventSink['onTool']>[0]) => received.push(event) }
    const call: CoreTurnItemJson = { ...base, id: 'physical-call', kind: 'tool_call', role: 'assistant', status: 'running', arguments: args }
    const result: CoreTurnItemJson = { ...base, id: 'physical-result', kind: 'tool_result', role: 'tool', status: 'completed', output, isError: false }
    for (const item of [call, result]) {
      await dispatchAnalytixRuntimeEvent({ kind: 'item_completed', threadId: base.threadId, turnId: base.turnId, item }, sink, async () => undefined)
    }
    expect(received).toHaveLength(2)
    expect(received[0].meta?.toolCallIdentity).toEqual({ threadId: base.threadId, turnId: base.turnId, callId: base.callId, itemId: call.id })
    expect(received[1].meta?.toolResultIdentity).toEqual({ threadId: base.threadId, turnId: base.turnId, callId: base.callId, itemId: result.id })
    const projected = projectToolEventForRenderer({ ...received[1], meta: { ...received[1].meta, runtimeStatus: 'tool_progress' } })
    expect(projected?.meta?.toolResultIdentity).toEqual(received[1].meta?.toolResultIdentity)
    expect(projected?.meta?.toolResultItemId).toBe(result.id)
    const mismatched = projectToolEventForRenderer({ ...received[1], meta: { ...received[1].meta, runtimeStatus: 'tool_progress', sourceItemId: 'unrelated-progress' } })
    expect(mismatched?.meta).not.toHaveProperty('toolResultIdentity')
    await dispatchAnalytixRuntimeEvent({ kind: 'tool_call_ready', threadId: base.threadId, turnId: base.turnId, callId: base.callId, toolName: base.toolName, itemId: call.id }, sink, async () => undefined)
    expect(received.at(-1)?.meta).not.toHaveProperty('toolCallIdentity')
  })
})


describe('output cap committed batch and history projection', () => {
  function cappedBatch(empty = false): Record<string, unknown> {
    const batch = typedGeneralTerminalBatch()
    const events = batch.events as Array<Record<string, unknown>>
    events[2].endReason = { kind: 'max-tokens' }
    if (empty) {
      batch.events = events.slice(1)
      batch.firstSeq = 12
      batch.eventManifest = (batch.eventManifest as unknown[]).slice(1)
    }
    return batch
  }
  it('maps a closed cap fact only from the validated terminal batch', () => {
    expect(generalTerminalProjectionBatchFromRuntime(cappedBatch())?.terminal.endReason).toEqual({ kind: 'max-tokens' })
    const batch = cappedBatch()
    ;(batch.events as Array<Record<string, unknown>>)[2].endReason = { kind: 'max-tokens', private: 'raw' }
    expect(generalTerminalProjectionBatchFromRuntime(batch)).toBeNull()
  })
  it('replays an empty cap as the same two-slot user association without inventing assistant text', () => {
    const item = { id: 'cap-user', kind: 'user_message', role: 'user', threadId: 'thr_general', turnId: 'turn_general', status: 'completed',
      createdAt: '2026-07-20T02:00:00Z', text: 'Review code.' }
    const replay = createOrdinaryHistoryReplay('thr_general', 13)
    replay.accept([{ kind: 'item_completed', seq: 1, timestamp: item.createdAt, threadId: item.threadId, turnId: item.turnId, item }])
    replay.accept([cappedBatch(true)])
    const snapshot = chatBlockFromItem(item as CoreTurnItemJson)!
    const blocks = replay.mergeSnapshot([snapshot], { threadId: 'thr_general' })
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({ kind: 'user', generalTerminalProjectionReceipt: {
      threadId: 'thr_general', turnId: 'turn_general', itemId: 'cap-user', firstSeq: 12, lastSeq: 13, endReason: { kind: 'max-tokens' }
    } })
    expect(replay.mergeSnapshot([{ ...snapshot, text: 'Changed identity text' } as ChatBlock], { threadId: 'thr_general' })[0]).not.toHaveProperty('generalTerminalProjectionReceipt')
  })
})


describe('ordinary compaction interrupted history', () => {
  it('keeps a start without committed evidence unknown while live starts still run', async () => {
    const event: CoreRuntimeEventJson = { kind: 'compaction_started', schemaVersion: 3, threadId: 'thr_unknown', turnId: 'turn_unknown', itemId: 'compaction_unknown', compactionId: 'compaction_unknown',
      auto: false, startedAt: '2026-10-09T00:00:00Z', timestamp: '2026-10-09T00:00:00Z', seq: 1 }
    const replay = createOrdinaryHistoryReplay('thr_unknown', 1)
    replay.accept([event])
    expect(replay.mergeSnapshot([], { threadId: 'thr_unknown' })).toMatchObject([{ kind: 'compaction', id: 'compaction_unknown', status: 'unknown', summary: '' }])
    const compactions: CompactionEventPayload[] = []
    await dispatchAnalytixRuntimeEvent(event, { ...makeSink(), onCompaction: ev => compactions.push(ev) }, async () => undefined)
    expect(compactions).toMatchObject([{ itemId: 'compaction_unknown', status: 'running' }])
  })
})

it('keeps initial loaded skills item-owned during hydration and clears absent facts', () => {
  const context = { activeSkillIds: ['turn-wide-claim'] };
  const user = { id:'user-one',createdAt:'2026-10-17T00:00:00Z',turnId:'turn-one',threadId:'thread-one',role:'user',status:'completed',kind:'user_message',text:'/review',activeSkillIds:['review'] } as const;
  const first = chatBlockFromItem({...user, activeSkillIds:['review']}, undefined, context);
  const second = chatBlockFromItem({...user, id:'user-two',activeSkillIds:undefined}, undefined, context);
  expect(first).toMatchObject({meta:{activeSkillIds:['review']}});
  expect(second).toMatchObject({meta:{activeSkillIds:[]}});
});
