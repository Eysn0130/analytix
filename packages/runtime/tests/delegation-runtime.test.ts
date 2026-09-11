import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import { buildDelegationToolProviders } from '../src/tool-test-support/tool/delegation-tool-provider.js'
import { LocalToolHost } from '../src/tool-test-support/tool/local-tool-host.js'
import { AnalytixCapabilitiesConfig } from '../src/contracts/capabilities.js'
import { ChildRunRecord, DelegationRuntime, FileDelegationStore } from '../src/delegation-test-support/delegation-runtime.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'

describe('DelegationRuntime', () => {
  let dir = ''

  beforeEach(async () => {
    dir = await mkdtemp(join(tmpdir(), 'analytix-delegation-'))
  })

  afterEach(async () => {
    await rm(dir, { recursive: true, force: true })
  })

  it('creates child runs, persists records, and emits child event metadata', async () => {
    const sessionStore = new InMemorySessionStore()
    const externalUsage: unknown[] = []
    const externalUsageAttribution: unknown[] = []
    const runtime = createRuntime({
      sessionStore,
      recordExternalUsage: (_threadId, usage, attribution) => {
        externalUsage.push(usage)
        externalUsageAttribution.push(attribution)
        return { ...usage, totalTokens: 9 }
      }
    })
    const result = await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      label: 'research',
      prompt: 'Research A',
      workspace: '/tmp/ws',
      effort: 'high',
      signal: new AbortController().signal
    })

    expect(result).toMatchObject({ status: 'completed', summary: 'done: Research A' })
    expect((await runtime.diagnostics('thr_1')).childRuns).toHaveLength(1)
    const events = await sessionStore.loadEventsSince('thr_1', 0)
    expect(events.some((event) => event.child?.childId === result.id && event.child.childStatus === 'completed')).toBe(true)
    const usageEvent = events.find((event) => event.kind === 'usage')
    expect(usageEvent).toMatchObject({
      kind: 'usage',
      threadId: 'thr_1',
      turnId: 'turn_1',
      effort: 'high',
      usageSource: 'subagent',
      childRunId: result.id,
      usage: { totalTokens: 9 }
    })
    expect(externalUsage).toHaveLength(1)
    expect(externalUsage[0]).toMatchObject({ totalTokens: 3 })
    expect(externalUsageAttribution).toEqual([{ usageSource: 'subagent', childRunId: result.id }])
  })

  it('denies disabled delegation and exhausted child budgets', async () => {
    const disabled = createRuntime({ enabled: false })
    await expect(disabled.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'x',
      signal: new AbortController().signal
    })).rejects.toThrow(/disabled/)

    const budgeted = createRuntime({ maxChildRuns: 1 })
    await budgeted.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'first',
      signal: new AbortController().signal
    })
    await expect(budgeted.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'second',
      signal: new AbortController().signal
    })).rejects.toThrow(/budget/)
  })

  it('reconciles stale queued or running child runs to aborted on restart', async () => {
    const sessionStore = new InMemorySessionStore()
    const bus = new InMemoryEventBus()
    const recorder = new RuntimeEventRecorder({
      eventBus: bus,
      sessionStore,
      allocateSeq: (threadId) => bus.allocateSeq(threadId),
      nowIso: () => '2026-06-03T00:00:05.000Z'
    })
    const store = new FileDelegationStore(join(dir, 'children-reconcile'))
    await store.upsert(ChildRunRecord.parse({
      id: 'child_running',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      childThreadId: 'child_running',
      prompt: 'running child',
      status: 'running',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
      createdAt: '2026-06-03T00:00:00.000Z',
      startedAt: '2026-06-03T00:00:01.000Z',
      updatedAt: '2026-06-03T00:00:01.000Z'
    }))
    await store.upsert(ChildRunRecord.parse({
      id: 'child_queued',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      childThreadId: 'child_queued',
      prompt: 'queued child',
      status: 'queued',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
      createdAt: '2026-06-03T00:00:00.000Z',
      updatedAt: '2026-06-03T00:00:00.000Z'
    }))
    await store.upsert(ChildRunRecord.parse({
      id: 'child_done',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      childThreadId: 'child_done',
      prompt: 'done child',
      status: 'completed',
      summary: 'done',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
      createdAt: '2026-06-03T00:00:00.000Z',
      updatedAt: '2026-06-03T00:00:00.000Z'
    }))
    const config = AnalytixCapabilitiesConfig.parse({
      subagents: { enabled: true, maxParallel: 1, maxChildRuns: 3 }
    }).subagents
    const runtime = new DelegationRuntime({
      config,
      store,
      events: recorder,
      nowIso: () => '2026-06-03T00:00:05.000Z'
    })

    const reconciled = await runtime.reconcileRunningChildren('runtime restarted before child run finished')

    expect(reconciled).toHaveLength(2)
    expect(reconciled).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'child_queued',
        status: 'aborted',
        error: 'runtime restarted before child run finished',
        queuedMs: 5000
      }),
      expect.objectContaining({
        id: 'child_running',
        status: 'aborted',
        error: 'runtime restarted before child run finished',
        durationMs: 4000
      })
    ]))
    expect(await store.list('thr_parent')).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'child_queued', status: 'aborted' }),
      expect.objectContaining({ id: 'child_running', status: 'aborted' }),
      expect.objectContaining({ id: 'child_done', status: 'completed' })
    ]))
    const events = await sessionStore.loadEventsSince('thr_parent', 0)
    expect(events.filter((event) => event.child?.childStatus === 'aborted')).toHaveLength(2)
  })

  it('executes delegate_task through the normal tool host', async () => {
    const seenEfforts: Array<string | undefined> = []
    const seenParentToolCallIds: Array<string | undefined> = []
    const seenModelExecutions: unknown[] = []
    const sessionStore = new InMemorySessionStore()
    const runtime = createRuntime({
      executor: async (input) => {
        seenEfforts.push(input.effort)
        seenParentToolCallIds.push(input.parentToolCallId)
        seenModelExecutions.push(input.modelExecution)
        return {
          summary: `done: ${input.prompt}`,
          childThreadId: 'thr_child_delegate',
          childTurnId: 'turn_child_delegate',
          cacheDiagnostics: {
            prefixHash: 'prefix_delegate',
            prefixChanged: false,
            prefixChangeReasons: [],
            systemHash: 'system_delegate',
            prefixItemsHash: 'items_delegate',
            toolsHash: 'tools_delegate',
            toolSchemaTokens: 42,
            cacheTelemetrySupported: true,
            cacheHitTokens: 4,
            cacheMissTokens: 6,
            providerId: 'subagent-provider',
            model: 'subagent-model'
          },
          usage: {
            promptTokens: 10,
            completionTokens: 2,
            totalTokens: 12,
            cachedTokens: 6,
            cacheHitTokens: 4,
            cacheMissTokens: 6,
            cacheHitRate: 0.4,
            cacheableTokenHitRate: 0.5,
            totalInputTokenHitRate: 0.25,
            cacheMissReasons: ['cold_child_prefix'],
            cacheSuggestions: ['reuse child profile'],
            turns: 1,
            priceConfigured: true,
            costUsd: 0.001,
            costCny: 0.007,
            cacheSavingsUsd: 0.002,
            cacheSavingsCny: 0.014,
            tokenEconomySavingsTokens: 3,
            tokenEconomySavingsUsd: 0.003,
            tokenEconomySavingsCny: 0.021
          }
        }
      },
      sessionStore
    })
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildDelegationToolProviders(runtime))
    })
    expect((await host.listTools()).find((tool) => tool.name === 'delegate_task'))
      .toMatchObject({ toolKind: 'subagent' })
    const result = await host.execute({
      callId: 'call_1',
      toolName: 'delegate_task',
      arguments: { label: 'A', prompt: 'Investigate A', effort: 'low' }
    }, {
      threadId: 'thr_1',
      turnId: 'turn_1',
      workspace: '/tmp/ws',
      approvalPolicy: 'auto',
      abortSignal: new AbortController().signal,
      modelExecution: {
        providerId: 'anthropic-main',
        modelId: 'claude-3-5-sonnet',
        endpointFormat: 'messages',
        source: 'thread',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      },
      awaitApproval: async () => 'allow'
    })

    expect(result.item).toMatchObject({ kind: 'tool_result', isError: false })
    expect(seenEfforts).toEqual(['low'])
    expect(seenParentToolCallIds).toEqual(['call_1'])
    expect(seenModelExecutions).toEqual([
      expect.objectContaining({
        providerId: 'anthropic-main',
        modelId: 'claude-3-5-sonnet',
        endpointFormat: 'messages',
        source: 'thread'
      })
    ])
    if (result.item.kind === 'tool_result') {
      expect(result.item.output).toMatchObject({
        status: 'completed',
        summary: 'done: Investigate A',
        providerId: 'anthropic-main',
        endpointFormat: 'messages',
        modelSource: 'thread',
        effort: 'low',
        usage: { totalTokens: 12 }
      })
    }
    const events = await sessionStore.loadEventsSince('thr_1', 0)
    const completed = events.find((event) => event.child?.childStatus === 'completed')
    expect(completed?.child).toMatchObject({
      parentToolCallId: 'call_1',
      childRunId: expect.any(String),
      childThreadId: 'thr_child_delegate',
      childTurnId: 'turn_child_delegate',
      childModel: 'claude-3-5-sonnet',
      childProviderId: 'anthropic-main',
      childEndpointFormat: 'messages',
      childModelSource: 'thread',
      totalTokens: 12,
      cachedTokens: 6,
      cacheHitTokens: 4,
      cacheMissTokens: 6,
      cacheHitRate: 0.4,
      cacheableTokenHitRate: 0.5,
      totalInputTokenHitRate: 0.25,
      cacheMissReasons: ['cold_child_prefix'],
      cacheSuggestions: ['reuse child profile'],
      cacheDiagnostics: {
        prefixHash: 'prefix_delegate',
        prefixChanged: false,
        prefixChangeReasons: [],
        systemHash: 'system_delegate',
        prefixItemsHash: 'items_delegate',
        toolsHash: 'tools_delegate',
        toolSchemaTokens: 42,
        cacheTelemetrySupported: true,
        cacheHitTokens: 4,
        cacheMissTokens: 6,
        providerId: 'subagent-provider',
        model: 'subagent-model'
      },
      priceConfigured: true,
      costUsd: 0.001,
      costCny: 0.007,
      cacheSavingsUsd: 0.002,
      cacheSavingsCny: 0.014,
      tokenEconomySavingsTokens: 3,
      tokenEconomySavingsUsd: 0.003,
      tokenEconomySavingsCny: 0.021
    })
    const usageEvent = events.find((event) => event.kind === 'usage')
    expect(usageEvent).toMatchObject({
      cacheDiagnostics: { prefixHash: 'prefix_delegate', cacheTelemetrySupported: true },
      providerId: 'anthropic-main',
      endpointFormat: 'messages',
      usage: { totalTokens: 12, priceConfigured: true }
    })
  })

  it('inherits parent step limits for delegate_task children and accepts explicit max_steps', async () => {
    const seenMaxSteps: Array<number | undefined> = []
    const runtime = createRuntime({
      maxChildRuns: 4,
      executor: async (input) => {
        seenMaxSteps.push(input.maxModelSteps)
        return {
          summary: `done: ${input.prompt}`,
          usage: { promptTokens: 1, completionTokens: 2, totalTokens: 3 }
        }
      }
    })
    const host = new LocalToolHost({
      registry: new CapabilityRegistry(buildDelegationToolProviders(runtime))
    })
    const context = {
      threadId: 'thr_1',
      turnId: 'turn_1',
      workspace: '/tmp/ws',
      approvalPolicy: 'auto' as const,
      abortSignal: new AbortController().signal,
      runtimeStepLimits: { currentMaxModelSteps: 64 },
      awaitApproval: async () => 'allow' as const
    }

    const inherited = await host.execute({
      callId: 'call_inherit',
      toolName: 'delegate_task',
      arguments: { prompt: 'inherit budget' }
    }, context)
    const explicitUnlimited = await host.execute({
      callId: 'call_unlimited',
      toolName: 'delegate_task',
      arguments: { prompt: 'unlimited budget', max_steps: 0 }
    }, context)

    expect(seenMaxSteps).toEqual([32, 0])
    expect(inherited.item).toMatchObject({ kind: 'tool_result', isError: false })
    expect(explicitUnlimited.item).toMatchObject({ kind: 'tool_result', isError: false })
    if (inherited.item.kind === 'tool_result') {
      expect(inherited.item.output).toMatchObject({ maxModelSteps: 32 })
    }
    if (explicitUnlimited.item.kind === 'tool_result') {
      expect(explicitUnlimited.item.output).toMatchObject({ maxModelSteps: 0 })
    }
  })

  it('caps concurrency at maxParallel and queues the overflow instead of erroring', async () => {
    const gate = deferred<void>()
    let active = 0
    let maxObservedActive = 0
    const runtime = createRuntime({
      maxParallel: 2,
      maxChildRuns: 10,
      executor: async ({ prompt }) => {
        active += 1
        maxObservedActive = Math.max(maxObservedActive, active)
        await gate.promise
        active -= 1
        return { summary: `done: ${prompt}` }
      }
    })
    const signal = new AbortController().signal
    const runs = [0, 1, 2, 3].map((index) =>
      runtime.runChild({ parentThreadId: 'thr_1', parentTurnId: 'turn_1', prompt: `p${index}`, signal })
    )
    // Two children start; the other two wait on a parallel slot.
    await waitFor(() => maxObservedActive >= 2)
    expect(active).toBe(2)
    gate.resolve()
    const results = await Promise.all(runs)
    expect(results.every((record) => record.status === 'completed')).toBe(true)
    expect(maxObservedActive).toBe(2)
    expect((await runtime.diagnostics('thr_1')).childRuns).toHaveLength(4)
  })

  it('marks a child aborted while it is still queued', async () => {
    const gate = deferred<void>()
    const controller = new AbortController()
    const runtime = createRuntime({
      maxParallel: 1,
      executor: async () => {
        await gate.promise
        return { summary: 'blocking' }
      }
    })
    // Drive the only slot to a confirmed running state before enqueuing the
    // second child, so the abort target is deterministically the queued one.
    const blocking = runtime.runChild({ parentThreadId: 'thr_1', parentTurnId: 'turn_1', prompt: 'hold', signal: new AbortController().signal })
    await waitFor(async () => (await runtime.diagnostics('thr_1')).childRuns.some((run) => run.status === 'running'))
    const queued = runtime.runChild({ parentThreadId: 'thr_1', parentTurnId: 'turn_1', prompt: 'wait', signal: controller.signal })
    await waitFor(async () => (await runtime.diagnostics('thr_1')).childRuns.some((run) => run.status === 'queued'))
    controller.abort()
    await expect(queued).resolves.toMatchObject({ status: 'aborted' })
    gate.resolve()
    await expect(blocking).resolves.toMatchObject({ status: 'completed' })
  })

  it('resolves a profile to model, effort, preamble, tool policy, and tool scope', async () => {
    const seen: Array<{ model?: string; effort?: string; promptPreamble?: string; toolPolicy: string; toolScope?: string[] }> = []
    const runtime = createRuntime({
      defaultProfile: 'reviewer',
      profiles: {
        reviewer: {
          model: 'deepseek-v4-pro',
          effort: 'high',
          promptPreamble: 'Review for bugs.',
          toolPolicy: 'readOnly',
          tools: ['grep', 'read', 'grep']
        }
      },
      executor: async (input) => {
        seen.push({
          model: input.model,
          effort: input.effort,
          promptPreamble: input.promptPreamble,
          toolPolicy: input.toolPolicy,
          toolScope: input.toolScope
        })
        return { summary: 'reviewed', toolInvocations: 2, prefixReused: true, inheritedHistoryItems: 0 }
      }
    })
    const record = await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'check the diff',
      signal: new AbortController().signal
    })
    expect(seen[0]).toMatchObject({
      model: 'deepseek-v4-pro',
      effort: 'high',
      promptPreamble: 'Review for bugs.',
      toolPolicy: 'readOnly',
      toolScope: ['grep', 'read']
    })
    expect(record).toMatchObject({
      profile: 'reviewer',
      toolPolicy: 'readOnly',
      toolScope: ['grep', 'read'],
      model: 'deepseek-v4-pro',
      effort: 'high',
      toolInvocations: 2,
      prefixReused: true,
      inheritedHistoryItems: 0
    })
  })

  it('resolves profile-level provider, model, endpoint format, and variant overrides', async () => {
    const seen: Array<{ model?: string; modelExecution?: unknown }> = []
    const runtime = createRuntime({
      profiles: {
        reviewer: {
          providerId: 'openai-responses',
          model: 'gpt-5-mini',
          endpointFormat: 'responses',
          variant: '2026-06-30',
          toolPolicy: 'readOnly'
        }
      },
      executor: async (input) => {
        seen.push({ model: input.model, modelExecution: input.modelExecution })
        return { summary: 'profile override ok' }
      }
    })
    const record = await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'check provider override',
      profile: 'reviewer',
      modelExecution: {
        providerId: 'anthropic-main',
        modelId: 'claude-3-5-sonnet',
        endpointFormat: 'messages',
        variant: 'parent-variant',
        baseUrlFingerprint: 'parent-base-url',
        capabilityFingerprint: 'parent-capability',
        source: 'thread',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      },
      signal: new AbortController().signal
    })

    expect(seen[0]).toMatchObject({
      model: 'gpt-5-mini',
      modelExecution: {
        providerId: 'openai-responses',
        modelId: 'gpt-5-mini',
        endpointFormat: 'responses',
        variant: '2026-06-30',
        source: 'subagent-profile'
      }
    })
    expect(seen[0]?.modelExecution).not.toMatchObject({
      baseUrlFingerprint: 'parent-base-url',
      capabilityFingerprint: 'parent-capability'
    })
    expect(record).toMatchObject({
      providerId: 'openai-responses',
      model: 'gpt-5-mini',
      endpointFormat: 'responses',
      variant: '2026-06-30',
      modelSource: 'subagent-profile',
      modelExecution: expect.objectContaining({
        providerId: 'openai-responses',
        modelId: 'gpt-5-mini',
        endpointFormat: 'responses',
        variant: '2026-06-30',
        source: 'subagent-profile'
      })
    })
  })

  it('rejects child runs without an inherited provider before creating a record', async () => {
    const runtime = createRuntime({ defaultModelExecution: false })
    await expect(runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'must not fall back',
      modelExecution: {
        providerId: '',
        modelId: 'deepseek-v4-pro',
        source: 'thread',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      },
      signal: new AbortController().signal
    })).rejects.toThrow(/provider_not_found/)

    expect((await runtime.diagnostics('thr_1')).childRuns).toHaveLength(0)
  })

  it('rejects parent model execution without providerId as a child inheritance source', async () => {
    const runtime = createRuntime({ defaultModelExecution: false })
    await expect(runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'must not inherit an incomplete execution ref',
      modelExecution: {
        modelId: 'deepseek-v4-pro',
        source: 'runtime-default',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      } as never,
      signal: new AbortController().signal
    })).rejects.toThrow(/provider_not_found/)

    expect((await runtime.diagnostics('thr_1')).childRuns).toHaveLength(0)
  })

  it('rejects an unknown profile name', async () => {
    const runtime = createRuntime({ profiles: { reviewer: { toolPolicy: 'readOnly' } } })
    await expect(runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'x',
      profile: 'ghost',
      signal: new AbortController().signal
    })).rejects.toThrow(/unknown subagent profile/)
  })

  it('defaults the tool policy to read-only when no profile resolves', async () => {
    const seen: string[] = []
    const runtime = createRuntime({
      executor: async (input) => {
        seen.push(input.toolPolicy)
        return { summary: 'ok' }
      }
    })
    const record = await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'investigate',
      signal: new AbortController().signal
    })
    expect(seen[0]).toBe('readOnly')
    expect(record.toolPolicy).toBe('readOnly')
  })

  it('emits queued -> running -> completed events with observability metrics', async () => {
    const sessionStore = new InMemorySessionStore()
    const runtime = createRuntime({
      sessionStore,
      executor: async () => ({
        summary: 'ok',
        usage: { promptTokens: 1, completionTokens: 2, totalTokens: 3, cacheHitRate: 0.5, costUsd: 0.01 },
        toolInvocations: 4,
        prefixReused: true,
        inheritedHistoryItems: 0
      })
    })
    const record = await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'go',
      effort: 'high',
      signal: new AbortController().signal
    })
    const events = await sessionStore.loadEventsSince('thr_1', 0)
    const statuses = events
      .filter((event) => event.child?.childId === record.id)
      .map((event) => event.child?.childStatus)
    expect(statuses).toEqual(['queued', 'running', 'completed'])
    const runningSeq = events.find((event) =>
      event.child?.childId === record.id && event.child.childStatus === 'running'
    )?.seq
    expect(runningSeq).toBeGreaterThan(0)
    const replay = await sessionStore.loadEventsSince('thr_1', runningSeq ?? 0)
    expect(replay.map((event) => event.child?.childStatus).filter(Boolean)).toEqual(['completed'])
    const completed = events.find((event) => event.child?.childId === record.id && event.child.childStatus === 'completed')
    expect(completed?.child).toMatchObject({
      childRunId: record.id,
      childEffort: 'high',
      toolInvocations: 4,
      prefixReused: true,
      totalTokens: 3,
      cacheHitRate: 0.5,
      childToolPolicy: 'readOnly'
    })
  })

  it('records completed child runs into the parent goal evidence ledger when a goal is active', async () => {
    const sessionStore = new InMemorySessionStore()
    const bus = new InMemoryEventBus()
    const recorder = new RuntimeEventRecorder({
      eventBus: bus,
      sessionStore,
      allocateSeq: (threadId) => bus.allocateSeq(threadId),
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })
    const threads = new ThreadService({
      threadStore: new InMemoryThreadStore(),
      sessionStore,
      events: recorder,
      ids: new SequentialIdGenerator(),
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })
    await threads.create(
      { workspace: '/tmp/ws', model: 'deepseek-chat', mode: 'agent' },
      { id: 'thr_goal', title: 'Goal parent' }
    )
    await threads.setGoal('thr_goal', {
      objective: 'Investigate delegated tasks',
      status: 'active'
    })
    const runtime = createRuntime({
      sessionStore,
      recordParentEvidence: async (record) => {
        const goal = await threads.getGoal(record.parentThreadId)
        if (!goal || goal.status !== 'active') return false
        await threads.appendGoalEvidence(record.parentThreadId, {
          step: `Delegated task completed: ${record.label ?? record.id}`,
          evidence: [
            `delegate_task child ${record.id} completed`,
            record.summary ?? ''
          ],
          summary: 'Child run completed with evidence.',
          turnId: record.parentTurnId
        })
        return true
      },
      executor: async () => ({
        summary: 'child checked the cache path',
        toolInvocations: 2,
        prefixReused: true,
        inheritedHistoryItems: 0
      })
    })

    const record = await runtime.runChild({
      parentThreadId: 'thr_goal',
      parentTurnId: 'turn_parent',
      label: 'cache review',
      prompt: 'Check cache path',
      signal: new AbortController().signal
    })

    expect(record).toMatchObject({
      status: 'completed',
      evidenceLedgered: true,
      toolInvocations: 2,
      prefixReused: true
    })
    const goal = await threads.getGoal('thr_goal')
    expect(goal?.evidenceLedger).toEqual([
      expect.objectContaining({
        turnId: 'turn_parent',
        step: 'Delegated task completed: cache review',
        evidence: expect.arrayContaining([
          expect.stringContaining(`delegate_task child ${record.id} completed`),
          'child checked the cache path'
        ])
      })
    ])
    const events = await sessionStore.loadEventsSince('thr_goal', 0)
    expect(events.some((event) =>
      event.child?.childId === record.id && event.child.evidenceLedgered === true
    )).toBe(true)
  })

  it('aggregates child runs by label and model for dashboards', async () => {
    const runtime = createRuntime()
    await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      label: 'research',
      prompt: 'first',
      model: 'deepseek-v4-flash',
      signal: new AbortController().signal
    })
    await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      label: 'research',
      prompt: 'second',
      model: 'deepseek-v4-flash',
      signal: new AbortController().signal
    })

    const diagnostics = await runtime.diagnostics('thr_1')
    expect(diagnostics.aggregates[0]).toMatchObject({
      key: 'research:deepseek-v4-flash',
      runs: 2,
      completed: 2,
      totalTokens: 6,
      averageTotalTokens: 3
    })
  })

  it('keeps child run aggregates separate by effort', async () => {
    const runtime = createRuntime()
    await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      label: 'research',
      prompt: 'low effort',
      model: 'deepseek-v4-flash',
      effort: 'low',
      signal: new AbortController().signal
    })
    await runtime.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      label: 'research',
      prompt: 'high effort',
      model: 'deepseek-v4-flash',
      effort: 'high',
      signal: new AbortController().signal
    })

    const diagnostics = await runtime.diagnostics('thr_1')
    expect(diagnostics.aggregates).toEqual(expect.arrayContaining([
      expect.objectContaining({
        key: 'research:deepseek-v4-flash:low',
        effort: 'low',
        runs: 1
      }),
      expect.objectContaining({
        key: 'research:deepseek-v4-flash:high',
        effort: 'high',
        runs: 1
      })
    ]))
  })

  it('records child failure and parent interruption states', async () => {
    const failed = createRuntime({
      executor: async () => {
        throw new Error('child failed')
      }
    })
    await expect(failed.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'fail',
      signal: new AbortController().signal
    })).resolves.toMatchObject({ status: 'failed', error: 'child failed' })

    const controller = new AbortController()
    controller.abort()
    const aborted = createRuntime({
      executor: async ({ signal }) => {
        if (signal.aborted) throw new Error('aborted')
        return { summary: 'unreachable' }
      }
    })
    await expect(aborted.runChild({
      parentThreadId: 'thr_1',
      parentTurnId: 'turn_1',
      prompt: 'abort',
      signal: controller.signal
    })).resolves.toMatchObject({ status: 'aborted' })
  })

  function createRuntime(options: {
    enabled?: boolean
    maxParallel?: number
    maxChildRuns?: number
    defaultToolPolicy?: 'readOnly' | 'inherit'
    defaultProfile?: string
    profiles?: Record<string, {
      providerId?: string
      model?: string
      variant?: string
      endpointFormat?: 'chat_completions' | 'responses' | 'messages' | 'custom_endpoint'
      effort?: 'off' | 'low' | 'medium' | 'high' | 'max'
      promptPreamble?: string
      toolPolicy?: 'readOnly' | 'inherit'
      tools?: string[]
    }>
    sessionStore?: InMemorySessionStore
    executor?: ConstructorParameters<typeof DelegationRuntime>[0]['executor']
    recordExternalUsage?: ConstructorParameters<typeof DelegationRuntime>[0]['recordExternalUsage']
    recordParentEvidence?: ConstructorParameters<typeof DelegationRuntime>[0]['recordParentEvidence']
    defaultModelExecution?: boolean
  } = {}) {
    const sessionStore = options.sessionStore ?? new InMemorySessionStore()
    const bus = new InMemoryEventBus()
    const recorder = new RuntimeEventRecorder({
      eventBus: bus,
      sessionStore,
      allocateSeq: (threadId) => bus.allocateSeq(threadId),
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })
    const config = AnalytixCapabilitiesConfig.parse({
      subagents: {
        enabled: options.enabled ?? true,
        maxParallel: options.maxParallel ?? 1,
        maxChildRuns: options.maxChildRuns ?? 3,
        ...(options.defaultToolPolicy ? { defaultToolPolicy: options.defaultToolPolicy } : {}),
        ...(options.defaultProfile ? { defaultProfile: options.defaultProfile } : {}),
        ...(options.profiles ? { profiles: options.profiles } : {})
      }
    }).subagents
    let idSeq = 0
    const runtime = new DelegationRuntime({
      config,
      store: new FileDelegationStore(join(dir, 'children')),
      events: recorder,
      nowIso: () => '2026-06-03T00:00:00.000Z',
      idGenerator: () => `child_${++idSeq}_${Math.random().toString(36).slice(2, 6)}`,
      recordExternalUsage: options.recordExternalUsage,
      recordParentEvidence: options.recordParentEvidence,
      executor: options.executor ?? (async ({ prompt }) => ({
        summary: `done: ${prompt}`,
        usage: { promptTokens: 1, completionTokens: 2, totalTokens: 3 }
      }))
    })
    if (options.defaultModelExecution !== false) {
      const runChild = runtime.runChild.bind(runtime)
      runtime.runChild = ((input) => runChild({
        modelExecution: defaultParentModelExecution(),
        ...input
      })) as DelegationRuntime['runChild']
    }
    return runtime
  }
})

function defaultParentModelExecution() {
  return {
    providerId: 'anthropic-main',
    modelId: 'claude-3-5-sonnet',
    endpointFormat: 'messages' as const,
    source: 'thread' as const,
    resolvedAt: '2026-06-03T00:00:00.000Z'
  }
}

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void; reject: (error: unknown) => void } {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function waitFor(predicate: () => boolean | Promise<boolean>, timeoutMs = 2000): Promise<void> {
  const start = Date.now()
  for (;;) {
    if (await predicate()) return
    if (Date.now() - start > timeoutMs) throw new Error('waitFor timed out')
    await new Promise((resolve) => setTimeout(resolve, 5))
  }
}
