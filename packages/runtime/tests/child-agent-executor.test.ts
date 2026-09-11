import { describe, expect, it } from 'vitest'

import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import { LocalToolHost, buildDefaultLocalTools } from '../src/tool-test-support/tool/local-tool-host.js'
import { createImmutablePrefix } from '../src/cache/immutable-prefix.js'
import type { ModelExecutionRef } from '../src/contracts/model-execution-ref.js'
import { createChildAgentExecutor } from '../src/delegation-test-support/child-agent-executor.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import type { ModelClient, ModelRequest, ModelStreamChunk } from '../src/ports/model-client.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'

function model(chunks: ModelStreamChunk[], seen: ModelRequest[] = []): ModelClient {
  return {
    provider: 'child-test',
    model: 'child-test',
    async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
      seen.push(request)
      for (const chunk of chunks) yield chunk
    }
  }
}

function childModelExecution(modelId: string, providerId = 'child-provider'): ModelExecutionRef {
  return {
    providerId,
    modelId,
    endpointFormat: 'chat_completions',
    source: 'thread',
    resolvedAt: '2026-06-03T00:00:00.000Z'
  }
}

describe('child agent executor', () => {
  it('runs a real child AgentLoop and returns assistant summary plus usage', async () => {
    const seen: ModelRequest[] = []
    const executor = createChildAgentExecutor({
      model: model([
        { kind: 'assistant_text_delta', text: 'child ' },
        { kind: 'assistant_text_delta', text: 'answer' },
        {
          kind: 'usage',
          usage: {
            promptTokens: 11,
            completionTokens: 3,
            totalTokens: 14,
            cacheHitTokens: 5,
            cacheMissTokens: 6,
            cacheHitRate: 5 / 11,
            cachedTokens: 5,
            turns: 1,
            costUsd: 0.001,
            cacheSavingsUsd: 0.0002
          }
        },
        { kind: 'completed', stopReason: 'stop' }
      ], seen),
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-test',
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    const result = await executor({
      childId: 'child_1',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      label: 'research',
      prompt: 'Research the issue',
      workspace: '/tmp/project',
      modelExecution: {
        providerId: 'anthropic-main',
        modelId: 'child-test',
        endpointFormat: 'messages',
        source: 'thread',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      },
      effort: 'high',
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })

    expect(result.summary).toBe('child answer')
    expect(result).toMatchObject({ prefixReused: true, inheritedHistoryItems: 0 })
    expect(result.usage).toMatchObject({
      promptTokens: 11,
      completionTokens: 3,
      totalTokens: 14,
      cacheHitTokens: 5,
      cacheSavingsUsd: 0.0002,
      turns: 1
    })
    expect(seen).toHaveLength(1)
    expect(seen[0]).toMatchObject({
      threadId: 'child_1',
      model: 'child-test',
      providerId: 'anthropic-main',
      reasoningEffort: 'high',
      systemPrompt: 'child system',
      history: [
        expect.objectContaining({
          kind: 'user_message',
          text: 'Research the issue'
        })
      ]
    })
    expect(seen[0]?.tools).toEqual([])
  })

  it('persists durable child sessions as parent-owned side threads', async () => {
    const nowIso = () => '2026-06-03T00:00:00.000Z'
    const eventBus = new InMemoryEventBus()
    const sessionStore = new InMemorySessionStore()
    const threadStore = new InMemoryThreadStore()
    const ids = new SequentialIdGenerator()
    const events = new RuntimeEventRecorder({
      eventBus,
      sessionStore,
      allocateSeq: (threadId) => eventBus.allocateSeq(threadId),
      nowIso
    })
    const executor = createChildAgentExecutor({
      model: model([
        { kind: 'assistant_text_delta', text: 'durable result' },
        {
          kind: 'usage',
          usage: { promptTokens: 5, completionTokens: 2, totalTokens: 7, cacheHitRate: null, turns: 1 }
        },
        { kind: 'completed', stopReason: 'stop' }
      ]),
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-test',
      durableState: { threadStore, sessionStore, events, ids },
      nowIso
    })

    const result = await executor({
      childId: 'child_durable',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      label: 'durable child',
      prompt: 'Persist this child run',
      workspace: '/tmp/project',
      modelExecution: {
        providerId: 'openai-responses',
        modelId: 'child-test',
        endpointFormat: 'responses',
        source: 'thread',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      },
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })

    expect(result).toMatchObject({
      summary: 'durable result',
      childThreadId: 'child_durable',
      prefixReused: true,
      inheritedHistoryItems: 0
    })
    const thread = await threadStore.get('child_durable')
    expect(thread).toMatchObject({
      id: 'child_durable',
      title: 'Child agent: durable child',
      relation: 'side',
      parentThreadId: 'thr_parent',
      workspace: '/tmp/project',
      providerId: 'openai-responses',
      status: 'idle'
    })
    const items = await sessionStore.loadItems('child_durable')
    expect(items.map((item) => item.kind)).toEqual(['user_message', 'assistant_text'])
    expect(items[0]).toMatchObject({ text: 'Persist this child run', threadId: 'child_durable' })
    expect(items[1]).toMatchObject({ text: 'durable result', threadId: 'child_durable' })
    const eventsForChild = await sessionStore.loadEventsSince('child_durable', 0)
    expect(eventsForChild.map((event) => event.kind)).toEqual(expect.arrayContaining([
      'thread_created',
      'turn_started',
      'item_created',
      'usage',
      'turn_completed'
    ]))
    const threadService = new ThreadService({ threadStore, sessionStore, events, ids, nowIso })
    expect((await threadService.list()).some((candidate) => candidate.id === 'child_durable')).toBe(false)
    expect(await threadService.list({ includeSide: true })).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'child_durable',
        relation: 'side',
        parentThreadId: 'thr_parent',
        messageCount: 2,
        turnCount: 1
      })
    ]))
  })

  it('continues durable child sessions by appending to the existing child thread', async () => {
    const nowIso = () => '2026-06-03T00:00:00.000Z'
    const eventBus = new InMemoryEventBus()
    const sessionStore = new InMemorySessionStore()
    const threadStore = new InMemoryThreadStore()
    const ids = new SequentialIdGenerator()
    const events = new RuntimeEventRecorder({
      eventBus,
      sessionStore,
      allocateSeq: (threadId) => eventBus.allocateSeq(threadId),
      nowIso
    })
    const seen: ModelRequest[] = []
    let calls = 0
    const executor = createChildAgentExecutor({
      model: {
        provider: 'child-continuation',
        model: 'child-continuation',
        async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
          seen.push(request)
          calls += 1
          yield { kind: 'assistant_text_delta', text: calls === 1 ? 'first result' : 'second result' }
          yield { kind: 'completed', stopReason: 'stop' }
        }
      },
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-continuation',
      durableState: { threadStore, sessionStore, events, ids },
      nowIso
    })

    await executor({
      childId: 'child_continue',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'first prompt',
      workspace: '/tmp/project',
      modelExecution: childModelExecution('child-continuation'),
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })
    const continued = await executor({
      childId: 'child_continue',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent_2',
      prompt: 'second prompt',
      workspace: '/tmp/project',
      modelExecution: childModelExecution('child-continuation'),
      toolPolicy: 'inherit',
      transcript: {
        mode: 'continue',
        sourceId: 'child_continue',
        targetId: 'child_continue',
        identityHash: 'test'
      },
      signal: new AbortController().signal
    })

    const thread = await threadStore.get('child_continue')
    expect(thread?.turns).toHaveLength(2)
    expect((await sessionStore.loadItems('child_continue')).map((item) => item.kind)).toEqual([
      'user_message',
      'assistant_text',
      'user_message',
      'assistant_text'
    ])
    expect(continued.transcriptItems?.map((item) => item.kind)).toEqual([
      'user_message',
      'assistant_text',
      'user_message',
      'assistant_text'
    ])
    expect(seen).toHaveLength(2)
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('first prompt')
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('first result')
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('second prompt')
    expect((await sessionStore.loadEventsSince('child_continue', 0))
      .filter((event) => event.kind === 'thread_created')).toHaveLength(1)
  })

  it('forks durable child sessions by cloning the source child thread into a new side thread', async () => {
    const nowIso = () => '2026-06-03T00:00:00.000Z'
    const eventBus = new InMemoryEventBus()
    const sessionStore = new InMemorySessionStore()
    const threadStore = new InMemoryThreadStore()
    const ids = new SequentialIdGenerator()
    const events = new RuntimeEventRecorder({
      eventBus,
      sessionStore,
      allocateSeq: (threadId) => eventBus.allocateSeq(threadId),
      nowIso
    })
    const seen: ModelRequest[] = []
    let calls = 0
    const executor = createChildAgentExecutor({
      model: {
        provider: 'child-fork',
        model: 'child-fork',
        async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
          seen.push(request)
          calls += 1
          yield { kind: 'assistant_text_delta', text: calls === 1 ? 'source result' : 'fork result' }
          yield { kind: 'completed', stopReason: 'stop' }
        }
      },
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-fork',
      durableState: { threadStore, sessionStore, events, ids },
      nowIso
    })

    await executor({
      childId: 'child_source',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'source prompt',
      workspace: '/tmp/project',
      modelExecution: childModelExecution('child-fork'),
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })
    await executor({
      childId: 'child_fork',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent_2',
      prompt: 'fork prompt',
      workspace: '/tmp/project',
      modelExecution: childModelExecution('child-fork'),
      toolPolicy: 'inherit',
      transcript: {
        mode: 'fork',
        sourceId: 'child_source',
        targetId: 'child_fork',
        identityHash: 'test'
      },
      signal: new AbortController().signal
    })

    const fork = await threadStore.get('child_fork')
    expect(fork).toMatchObject({
      id: 'child_fork',
      relation: 'side',
      parentThreadId: 'thr_parent',
      forkedFromThreadId: 'child_source',
      forkedFromTitle: 'Child agent: child_source'
    })
    expect(fork?.turns).toHaveLength(2)
    const forkItems = await sessionStore.loadItems('child_fork')
    expect(forkItems.map((item) => item.kind)).toEqual([
      'user_message',
      'assistant_text',
      'user_message',
      'assistant_text'
    ])
    expect(forkItems[0]).toMatchObject({ text: 'source prompt', threadId: 'child_fork' })
    expect(forkItems[1]).toMatchObject({ text: 'source result', threadId: 'child_fork' })
    expect(forkItems[2]).toMatchObject({ text: 'fork prompt', threadId: 'child_fork' })
    expect(forkItems[3]).toMatchObject({ text: 'fork result', threadId: 'child_fork' })
    expect(seen).toHaveLength(2)
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('source prompt')
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('source result')
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('fork prompt')
    const threadService = new ThreadService({ threadStore, sessionStore, events, ids, nowIso })
    expect(await threadService.list({ includeSide: true })).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'child_fork',
        relation: 'side',
        parentThreadId: 'thr_parent',
        forkedFromThreadId: 'child_source',
        messageCount: 4,
        turnCount: 2
      })
    ]))
  })

  it('fails the child run when the child loop cannot produce a completed turn', async () => {
    const executor = createChildAgentExecutor({
      model: model([{ kind: 'error', message: 'model failed', code: 'bad_model' }]),
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-test',
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    await expect(executor({
      childId: 'child_fail',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'Fail',
      modelExecution: childModelExecution('child-test'),
      toolPolicy: 'readOnly',
      signal: new AbortController().signal
    })).rejects.toThrow(/child agent failed|model failed/i)
  })

  it('rejects direct child runs without an inherited provider before creating runtime state', async () => {
    const seen: ModelRequest[] = []
    const eventBus = new InMemoryEventBus()
    const sessionStore = new InMemorySessionStore()
    const threadStore = new InMemoryThreadStore()
    const ids = new SequentialIdGenerator()
    const events = new RuntimeEventRecorder({
      eventBus,
      sessionStore,
      allocateSeq: (threadId) => eventBus.allocateSeq(threadId),
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })
    const executor = createChildAgentExecutor({
      model: model([{ kind: 'assistant_text_delta', text: 'should not run' }], seen),
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'default-child',
      durableState: { threadStore, sessionStore, events, ids },
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    await expect(executor({
      childId: 'child_missing_provider',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'Do not fallback',
      model: 'explicit-child-model',
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })).rejects.toThrow(/provider_not_found/)

    expect(seen).toHaveLength(0)
    expect(await threadStore.get('child_missing_provider')).toBeNull()
  })

  it('rejects parent model execution refs without a provider at the direct executor boundary', async () => {
    const seen: ModelRequest[] = []
    const executor = createChildAgentExecutor({
      model: model([{ kind: 'assistant_text_delta', text: 'should not run' }], seen),
      toolHost: new LocalToolHost({ registry: new CapabilityRegistry([]) }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'default-child',
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    await expect(executor({
      childId: 'child_fallback_parent',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'Do not run without a provider',
      modelExecution: {
        modelId: 'default-model',
        source: 'runtime-default',
        resolvedAt: '2026-06-03T00:00:00.000Z'
      } as ModelExecutionRef,
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })).rejects.toThrow(/provider_not_found/)

    expect(seen).toHaveLength(0)
  })

  it('restricts a read-only child to investigation tools and a preamble prompt', async () => {
    const seen: ModelRequest[] = []
    const registry = new CapabilityRegistry([{
      id: 'builtin',
      kind: 'built-in',
      enabled: true,
      available: true,
      tools: buildDefaultLocalTools()
    }])
    const executor = createChildAgentExecutor({
      model: model([
        { kind: 'assistant_text_delta', text: 'done' },
        { kind: 'completed', stopReason: 'stop' }
      ], seen),
      toolHost: new LocalToolHost({ registry }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-test',
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    const result = await executor({
      childId: 'child_ro',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'Investigate the bug',
      promptPreamble: 'Read-only review.',
      modelExecution: childModelExecution('child-test'),
      toolPolicy: 'readOnly',
      signal: new AbortController().signal
    })

    const toolNames = (seen[0]?.tools ?? []).map((tool) => tool.name).sort()
    expect(toolNames).toEqual(['find', 'grep', 'ls', 'read'])
    expect(seen[0]?.history?.[0]).toMatchObject({
      kind: 'user_message',
      text: 'Read-only review.\n\nInvestigate the bug'
    })
    expect(result).toMatchObject({ prefixReused: true, inheritedHistoryItems: 0 })
  })

  it('filters recursive meta tools from inherited child tool catalogs', async () => {
    const seen: ModelRequest[] = []
    const registry = new CapabilityRegistry([{
      id: 'mixed-tools',
      kind: 'built-in',
      enabled: true,
      available: true,
      tools: [
        testTool('safe_probe'),
        testTool('delegate_task'),
        testTool('task'),
        testTool('parallel_tasks'),
        testTool('wait'),
        testTool('bash_output'),
        testTool('kill_shell'),
        testTool('user_input'),
        testTool('request_user_input'),
        testTool('create_goal'),
        testTool('update_goal')
      ]
    }])
    const executor = createChildAgentExecutor({
      model: model([
        { kind: 'assistant_text_delta', text: 'done' },
        { kind: 'completed', stopReason: 'stop' }
      ], seen),
      toolHost: new LocalToolHost({ registry }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-test',
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    await executor({
      childId: 'child_inherit',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'Use inherited tools safely',
      modelExecution: childModelExecution('child-test'),
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })

    expect((seen[0]?.tools ?? []).map((tool) => tool.name)).toEqual(['safe_probe'])
  })

  it('keeps headless subagents under the inherited approval policy', async () => {
    const seen: ModelRequest[] = []
    let calls = 0
    let executed = false
    const guardedTool = LocalToolHost.defineTool({
      name: 'guarded_write',
      description: 'would mutate from a child run',
      inputSchema: { type: 'object', properties: {} },
      policy: 'auto',
      execute: async () => {
        executed = true
        return { output: { ok: true } }
      }
    })
    const executor = createChildAgentExecutor({
      model: {
        provider: 'child-policy',
        model: 'child-policy',
        async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
          seen.push(request)
          calls += 1
          if (calls === 1) {
            yield {
              kind: 'tool_call_complete',
              callId: 'call_guarded',
              toolName: 'guarded_write',
              arguments: {}
            }
            yield { kind: 'completed', stopReason: 'tool_calls' }
            return
          }
          yield { kind: 'assistant_text_delta', text: 'child saw policy block' }
          yield { kind: 'completed', stopReason: 'stop' }
        }
      },
      toolHost: new LocalToolHost({ tools: [guardedTool] }),
      prefix: createImmutablePrefix({ systemPrompt: 'child system' }),
      defaultModel: 'child-policy',
      approvalPolicy: 'never',
      sandboxMode: 'danger-full-access',
      nowIso: () => '2026-06-03T00:00:00.000Z'
    })

    const result = await executor({
      childId: 'child_policy',
      parentThreadId: 'thr_parent',
      parentTurnId: 'turn_parent',
      prompt: 'Try the guarded tool',
      modelExecution: childModelExecution('child-policy'),
      toolPolicy: 'inherit',
      signal: new AbortController().signal
    })

    expect(executed).toBe(false)
    expect(result.summary).toBe('child saw policy block')
    expect(seen[0]?.tools.map((tool) => tool.name)).toContain('guarded_write')
    expect(JSON.stringify(seen[1]?.history ?? [])).toContain('approval_policy_blocked')
  })
})

function testTool(name: string) {
  return LocalToolHost.defineTool({
    name,
    description: `${name} test tool`,
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    policy: 'auto',
    execute: async () => ({ output: { ok: true } })
  })
}
