import { describe, expect, it } from 'vitest'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import {
  buildGoalLocalTools,
  COMPLETE_STEP_TOOL_NAME,
  CREATE_GOAL_TOOL_NAME,
  GET_GOAL_TOOL_NAME,
  RECORD_RESEARCH_DIRECTION_TOOL_NAME,
  UPDATE_GOAL_TOOL_NAME
} from '../src/tool-test-support/tool/goal-tools.js'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import { LocalToolHost } from '../src/tool-test-support/tool/local-tool-host.js'
import type { ToolHostContext } from '../src/ports/tool-host.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'

function buildService(): {
  service: ThreadService
  sessionStore: InMemorySessionStore
} {
  const bus = new InMemoryEventBus()
  const threadStore = new InMemoryThreadStore()
  const sessionStore = new InMemorySessionStore()
  const ids = new SequentialIdGenerator()
  let now = 1_700_000_000_000
  const nowIso = () => new Date((now += 1000)).toISOString()
  const events = new RuntimeEventRecorder({
    eventBus: bus,
    sessionStore,
    allocateSeq: (threadId) => bus.allocateSeq(threadId),
    nowIso
  })
  return {
    service: new ThreadService({ threadStore, sessionStore, events, ids, nowIso }),
    sessionStore
  }
}

function toolContext(threadId: string, turnId = 'turn_goal'): ToolHostContext {
  return {
    threadId,
    turnId,
    workspace: '/tmp',
    approvalPolicy: 'on-request',
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow'
  }
}

describe('goal local tools', () => {
  it('advertises get/create/complete/update goal tools', async () => {
    const { service } = buildService()
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const names = (await host.listTools(toolContext('thr_goal'))).map((tool) => tool.name)

    expect(names).toContain(GET_GOAL_TOOL_NAME)
    expect(names).toContain(CREATE_GOAL_TOOL_NAME)
    expect(names).toContain(COMPLETE_STEP_TOOL_NAME)
    expect(names).toContain(RECORD_RESEARCH_DIRECTION_TOOL_NAME)
    expect(names).toContain(UPDATE_GOAL_TOOL_NAME)
  })

  it('keeps read-only goal state visible in plan mode but blocks execution-state tools', async () => {
    const { service } = buildService()
    const host = new LocalToolHost({
      registry: new CapabilityRegistry([{
        id: 'goal',
        kind: 'gui',
        enabled: true,
        available: true,
        tools: buildGoalLocalTools(service)
      }])
    })
    const tools = await host.listTools({
      ...toolContext('thr_goal_plan'),
      threadMode: 'plan',
      allowedToolNames: [
        GET_GOAL_TOOL_NAME,
        COMPLETE_STEP_TOOL_NAME,
        UPDATE_GOAL_TOOL_NAME
      ]
    })
    const byName = new Map(tools.map((tool) => [tool.name, tool]))

    expect([...byName.keys()]).toEqual([GET_GOAL_TOOL_NAME])
    expect(byName.get(GET_GOAL_TOOL_NAME)?.inputSchema).toEqual({
      type: 'object',
      properties: {},
      additionalProperties: false
    })
    expect(byName.has(COMPLETE_STEP_TOOL_NAME)).toBe(false)
    expect(byName.has(UPDATE_GOAL_TOOL_NAME)).toBe(false)
    expect(byName.has(CREATE_GOAL_TOOL_NAME)).toBe(false)
    expect(byName.has(RECORD_RESEARCH_DIRECTION_TOOL_NAME)).toBe(false)
  })

  it('requires complete_step evidence before marking a goal complete', async () => {
    const { service, sessionStore } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_goal', title: 'Goal thread' }
    )
    await service.setGoal('thr_goal', {
      objective: 'check the memory pressure',
      status: 'active',
      tokenBudget: 100
    })
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const missingEvidence = await host.execute({
      callId: 'call_goal_complete_without_evidence',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_goal'))
    expect(missingEvidence.item.kind).toBe('tool_result')
    if (missingEvidence.item.kind !== 'tool_result') return
    expect(missingEvidence.item.isError).toBe(true)
    expect(missingEvidence.item.output).toMatchObject({
      error: expect.stringContaining(COMPLETE_STEP_TOOL_NAME)
    })
    expect((await service.getGoal('thr_goal'))?.status).toBe('active')

    const completedStep = await host.execute({
      callId: 'call_complete_step',
      toolName: COMPLETE_STEP_TOOL_NAME,
      arguments: {
        step: 'Validated memory pressure result',
        evidence: ['packages/runtime/tests/goal-tools.test.ts proves completion gating'],
        summary: 'The goal has evidence before completion.'
      }
    }, toolContext('thr_goal'))
    expect(completedStep.item.kind).toBe('tool_result')
    if (completedStep.item.kind !== 'tool_result') return
    expect(completedStep.item.isError).toBeFalsy()
    expect(completedStep.item.output).toMatchObject({
      entry: {
        step: 'Validated memory pressure result',
        evidence: ['packages/runtime/tests/goal-tools.test.ts proves completion gating'],
        toolCallId: 'call_complete_step',
        turnId: 'turn_goal'
      },
      goal: {
        evidenceLedger: [
          expect.objectContaining({
            step: 'Validated memory pressure result'
          })
        ]
      },
      evidenceLedgerCount: 1
    })

    const result = await host.execute({
      callId: 'call_goal_complete',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_goal'))

    expect(result.item.kind).toBe('tool_result')
    if (result.item.kind !== 'tool_result') return
    expect(result.item.isError).toBeFalsy()
    expect(result.item.output).toMatchObject({
      goal: {
        status: 'complete',
        objective: 'check the memory pressure',
        evidenceLedger: [
          expect.objectContaining({
            step: 'Validated memory pressure result'
          })
        ]
      },
      remainingTokens: 100,
      completionBudgetReport: expect.any(String)
    })
    expect((await service.getGoal('thr_goal'))?.status).toBe('complete')
    const events = await sessionStore.loadEventsSince('thr_goal', 0)
    expect(events.some((event) => event.kind === 'goal_updated')).toBe(true)
  })

  it('rejects complete_step without concrete evidence', async () => {
    const { service } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_goal', title: 'Goal thread' }
    )
    await service.setGoal('thr_goal', {
      objective: 'finish the audit',
      status: 'active'
    })
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const result = await host.execute({
      callId: 'call_empty_evidence',
      toolName: COMPLETE_STEP_TOOL_NAME,
      arguments: { step: 'Done', evidence: [] }
    }, toolContext('thr_goal'))

    expect(result.item.kind).toBe('tool_result')
    if (result.item.kind !== 'tool_result') return
    expect(result.item.isError).toBe(true)
    expect(result.item.output).toMatchObject({
      error: expect.stringContaining('evidence')
    })
    expect((await service.getGoal('thr_goal'))?.evidenceLedger ?? []).toHaveLength(0)
  })

  it('rejects goal completion while thread todos remain incomplete', async () => {
    const { service } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_goal', title: 'Goal thread' }
    )
    await service.setGoal('thr_goal', {
      objective: 'finish the audit',
      status: 'active'
    })
    await service.appendGoalEvidence('thr_goal', {
      step: 'Recorded completion evidence',
      evidence: ['goal evidence exists but todo state is still incomplete']
    })
    await service.setTodos('thr_goal', {
      todos: [
        { content: 'Review provider matrix', status: 'completed' },
        { content: 'Run endpoint regression', status: 'in_progress' }
      ]
    })
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const result = await host.execute({
      callId: 'call_goal_complete_with_pending_todo',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_goal'))

    expect(result.item.kind).toBe('tool_result')
    if (result.item.kind !== 'tool_result') return
    expect(result.item.isError).toBe(true)
    expect(result.item.output).toMatchObject({
      error: expect.stringContaining('incomplete todos')
    })
    expect(JSON.stringify(result.item.output)).toContain('Run endpoint regression')
    expect((await service.getGoal('thr_goal'))?.status).toBe('active')
  })

  it('requires strict completion self-check before update_goal completes a strict goal', async () => {
    const { service } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_strict_goal', title: 'Strict goal thread' }
    )
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const created = await host.execute({
      callId: 'call_create_strict_goal',
      toolName: CREATE_GOAL_TOOL_NAME,
      arguments: {
        objective: 'ship strict goal mode',
        strict_completion: true
      }
    }, toolContext('thr_strict_goal', 'turn_create_goal'))

    expect(created.item.kind).toBe('tool_result')
    if (created.item.kind !== 'tool_result') return
    expect(created.item.output).toMatchObject({
      goal: {
        objective: 'ship strict goal mode',
        status: 'active',
        strictCompletion: true
      }
    })

    await host.execute({
      callId: 'call_complete_strict_step',
      toolName: COMPLETE_STEP_TOOL_NAME,
      arguments: {
        step: 'Implemented strict goal mode',
        evidence: ['packages/runtime/src/tool-test-support/tool/goal-tools.ts updated']
      }
    }, toolContext('thr_strict_goal', 'turn_strict_evidence'))

    const intercepted = await host.execute({
      callId: 'call_complete_strict_goal_first',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_strict_goal', 'turn_strict_complete_1'))

    expect(intercepted.item.kind).toBe('tool_result')
    if (intercepted.item.kind !== 'tool_result') return
    expect(intercepted.item.isError).toBeFalsy()
    expect(intercepted.item.output).toMatchObject({
      goal: {
        status: 'active',
        strictCompletion: true,
        selfCheckRequired: true,
        selfCheckTurnId: 'turn_strict_complete_1'
      },
      completionAudit: {
        selfCheckRequired: true,
        instructions: expect.stringContaining('Strict goal completion self-check required')
      }
    })

    const repeatedWithoutSelfCheck = await host.execute({
      callId: 'call_complete_strict_goal_second',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_strict_goal', 'turn_strict_complete_2'))

    expect(repeatedWithoutSelfCheck.item.kind).toBe('tool_result')
    if (repeatedWithoutSelfCheck.item.kind !== 'tool_result') return
    expect(repeatedWithoutSelfCheck.item.isError).toBe(true)
    expect(repeatedWithoutSelfCheck.item.output).toMatchObject({
      error: expect.stringContaining('self-check')
    })

    const selfCheck = await host.execute({
      callId: 'call_strict_self_check',
      toolName: COMPLETE_STEP_TOOL_NAME,
      arguments: {
        step: 'Performed final strict self-check',
        evidence: ['npm run test -- packages/runtime/tests/goal-tools.test.ts'],
        self_check: true
      }
    }, toolContext('thr_strict_goal', 'turn_strict_self_check'))

    expect(selfCheck.item.kind).toBe('tool_result')
    if (selfCheck.item.kind !== 'tool_result') return
    expect(selfCheck.item.isError).toBeFalsy()
    expect(selfCheck.item.output).toMatchObject({
      goal: {
        status: 'active',
        strictCompletion: true,
        selfCheckRequired: true,
        selfCheckCompleted: true,
        selfCheckTurnId: 'turn_strict_self_check'
      },
      selfCheckCompleted: true
    })

    const completed = await host.execute({
      callId: 'call_complete_strict_goal_final',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_strict_goal', 'turn_strict_complete_3'))

    expect(completed.item.kind).toBe('tool_result')
    if (completed.item.kind !== 'tool_result') return
    expect(completed.item.isError).toBeFalsy()
    expect(completed.item.output).toMatchObject({
      goal: {
        status: 'complete',
        strictCompletion: true,
        selfCheckCompleted: true
      },
      completionBudgetReport: expect.any(String)
    })
  })

  it('requires repeated same-condition blocked audits before marking a goal blocked', async () => {
    const { service } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_goal', title: 'Goal thread' }
    )
    await service.setGoal('thr_goal', {
      objective: 'finish the audit',
      status: 'active'
    })
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const missingReason = await host.execute({
      callId: 'call_goal_blocked_missing_reason',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked' }
    }, toolContext('thr_goal', 'turn_block_0'))

    expect(missingReason.item.kind).toBe('tool_result')
    if (missingReason.item.kind !== 'tool_result') return
    expect(missingReason.item.isError).toBe(true)
    expect(missingReason.item.output).toMatchObject({
      error: expect.stringContaining('reason')
    })
    expect((await service.getGoal('thr_goal'))?.status).toBe('active')

    const first = await host.execute({
      callId: 'call_goal_blocked_1',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked', reason: 'Need API access.' }
    }, toolContext('thr_goal', 'turn_block_1'))

    expect(first.item.kind).toBe('tool_result')
    if (first.item.kind !== 'tool_result') return
    expect(first.item.isError).toBeFalsy()
    expect(first.item.output).toMatchObject({
      goal: {
        status: 'active',
        blockedReason: 'Need API access',
        blockedCount: 1,
        blockedTurnId: 'turn_block_1'
      },
      blockedAudit: {
        reason: 'Need API access',
        count: 1,
        requiredCount: 3,
        status: 'active'
      }
    })

    const duplicateSameTurn = await host.execute({
      callId: 'call_goal_blocked_same_turn',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked', reason: 'need api access!!!' }
    }, toolContext('thr_goal', 'turn_block_1'))

    expect(duplicateSameTurn.item.kind).toBe('tool_result')
    if (duplicateSameTurn.item.kind !== 'tool_result') return
    expect(duplicateSameTurn.item.output).toMatchObject({
      goal: {
        status: 'active',
        blockedCount: 1,
        blockedTurnId: 'turn_block_1'
      },
      blockedAudit: { count: 1, status: 'active' }
    })

    const second = await host.execute({
      callId: 'call_goal_blocked_2',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked', reason: 'need api access!!!' }
    }, toolContext('thr_goal', 'turn_block_2'))

    expect(second.item.kind).toBe('tool_result')
    if (second.item.kind !== 'tool_result') return
    expect(second.item.output).toMatchObject({
      goal: {
        status: 'active',
        blockedReason: 'need api access',
        blockedCount: 2,
        blockedTurnId: 'turn_block_2'
      },
      blockedAudit: { count: 2, status: 'active' }
    })

    const reset = await host.execute({
      callId: 'call_goal_blocked_reset',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked', reason: 'Waiting for staging credentials.' }
    }, toolContext('thr_goal', 'turn_block_3'))

    expect(reset.item.kind).toBe('tool_result')
    if (reset.item.kind !== 'tool_result') return
    expect(reset.item.output).toMatchObject({
      goal: {
        status: 'active',
        blockedReason: 'Waiting for staging credentials',
        blockedCount: 1,
        blockedTurnId: 'turn_block_3'
      },
      blockedAudit: { count: 1, status: 'active' }
    })

    await host.execute({
      callId: 'call_goal_blocked_4',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked', reason: 'waiting for staging credentials' }
    }, toolContext('thr_goal', 'turn_block_4'))
    const final = await host.execute({
      callId: 'call_goal_blocked_5',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'blocked', reason: 'waiting for staging credentials' }
    }, toolContext('thr_goal', 'turn_block_5'))

    expect(final.item.kind).toBe('tool_result')
    if (final.item.kind !== 'tool_result') return
    expect(final.item.isError).toBeFalsy()
    expect(final.item.output).toMatchObject({
      goal: {
        status: 'blocked',
        blockedReason: 'waiting for staging credentials',
        blockedCount: 3,
        blockedTurnId: 'turn_block_5'
      },
      blockedAudit: {
        count: 3,
        requiredCount: 3,
        status: 'blocked'
      }
    })
    expect(await service.getGoal('thr_goal')).toMatchObject({
      status: 'blocked',
      blockedReason: 'waiting for staging credentials',
      blockedCount: 3,
      blockedTurnId: 'turn_block_5'
    })
  })

  it('rejects research direction records without an active research goal', async () => {
    const { service } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_goal', title: 'Goal thread' }
    )
    await service.setGoal('thr_goal', {
      objective: 'finish the audit',
      status: 'active'
    })
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const result = await host.execute({
      callId: 'call_direction_without_research',
      toolName: RECORD_RESEARCH_DIRECTION_TOOL_NAME,
      arguments: {
        direction: 'Try old terminal protocol',
        outcome: 'dead_end'
      }
    }, toolContext('thr_goal'))

    expect(result.item).toMatchObject({
      kind: 'tool_result',
      isError: true,
      output: {
        error: expect.stringContaining('no active research goal')
      }
    })
  })

  it('requires every research requirement to have evidence before completion', async () => {
    const workspace = await mkdtemp(join(tmpdir(), 'analytix-research-goal-'))
    try {
      const { service } = buildService()
      await service.create(
        { workspace, model: 'deepseek-chat', mode: 'agent' },
        { id: 'thr_research', title: 'Research goal thread' }
      )
      await service.setGoal('thr_research', {
        objective: 'research cache behavior',
        status: 'active',
        research: {
          enabled: true,
          requirements: ['Compare providers', 'Audit unsupported fallback']
        }
      })
      const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

      const direction = await host.execute({
        callId: 'call_direction',
        toolName: RECORD_RESEARCH_DIRECTION_TOOL_NAME,
        arguments: {
          direction: 'Compare provider cache telemetry',
          outcome: 'promising',
          summary: 'Provider matrix shows usable cache attribution fields.'
        }
      }, toolContext('thr_research'))
      expect(direction.item).toMatchObject({
        kind: 'tool_result',
        isError: false,
        output: {
          direction: {
            direction: 'Compare provider cache telemetry',
            outcome: 'promising',
            summary: 'Provider matrix shows usable cache attribution fields.'
          }
        }
      })
      const directions = JSON.parse(
        await readFile(
          join(workspace, '.analytix/autoresearch/thr_research/directions_tried.json'),
          'utf8'
        )
      )
      expect(directions.directions).toEqual([
        expect.objectContaining({
          direction: 'Compare provider cache telemetry',
          outcome: 'promising'
        })
      ])
      const iterationLog = await readFile(
        join(workspace, '.analytix/autoresearch/thr_research/iteration_log.jsonl'),
        'utf8'
      )
      expect(iterationLog).toContain('"kind":"direction_recorded"')

      await host.execute({
        callId: 'call_req_1',
        toolName: COMPLETE_STEP_TOOL_NAME,
        arguments: {
          step: 'Compared providers',
          requirement_id: 'req_1',
          evidence: ['provider matrix fixture reviewed']
        }
      }, toolContext('thr_research'))
      const incomplete = await host.execute({
        callId: 'call_research_complete_early',
        toolName: UPDATE_GOAL_TOOL_NAME,
        arguments: { status: 'complete' }
      }, toolContext('thr_research'))
      expect(incomplete.item).toMatchObject({
        kind: 'tool_result',
        isError: true,
        output: {
          error: expect.stringContaining('every requirement')
        }
      })

      await host.execute({
        callId: 'call_req_2',
        toolName: COMPLETE_STEP_TOOL_NAME,
        arguments: {
          step: 'Audited unsupported fallback',
          requirement_id: 'req_2',
          evidence: ['unsupported providers remain unknown, not all miss']
        }
      }, toolContext('thr_research'))
      const completed = await host.execute({
        callId: 'call_research_complete',
        toolName: UPDATE_GOAL_TOOL_NAME,
        arguments: { status: 'complete' }
      }, toolContext('thr_research'))

      expect(completed.item).toMatchObject({ kind: 'tool_result', isError: false })
      expect((await service.getGoal('thr_research'))?.status).toBe('complete')
      const progress = JSON.parse(
        await readFile(join(workspace, '.analytix/autoresearch/thr_research/progress.json'), 'utf8')
      )
      expect(progress).toMatchObject({
        status: 'complete',
        requirements: [
          { id: 'req_1', status: 'completed', evidenceCount: 1 },
          { id: 'req_2', status: 'completed', evidenceCount: 1 }
        ]
      })
    } finally {
      await rm(workspace, { recursive: true, force: true })
    }
  })

  it('rejects unsupported status updates and missing goals without mutating state', async () => {
    const { service } = buildService()
    await service.create(
      { workspace: '/tmp', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_empty', title: 'Empty goal thread' }
    )
    const host = new LocalToolHost({ tools: buildGoalLocalTools(service) })

    const unsupported = await host.execute({
      callId: 'call_goal_pause',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'paused' }
    }, toolContext('thr_empty'))
    const missing = await host.execute({
      callId: 'call_goal_missing',
      toolName: UPDATE_GOAL_TOOL_NAME,
      arguments: { status: 'complete' }
    }, toolContext('thr_empty'))

    expect(unsupported.item.kind).toBe('tool_result')
    expect(missing.item.kind).toBe('tool_result')
    if (unsupported.item.kind !== 'tool_result' || missing.item.kind !== 'tool_result') return
    expect(unsupported.item.isError).toBe(true)
    expect(missing.item.isError).toBe(true)
    expect(await service.getGoal('thr_empty')).toBeNull()
  })
})
