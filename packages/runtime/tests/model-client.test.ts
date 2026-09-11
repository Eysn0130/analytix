import { describe, expect, it, vi } from 'vitest'
import { CompatModelClient } from '../src/model-test-support/model/compat-model-client.js'
import { MultiProviderModelClient } from '../src/model-test-support/model/multi-provider-model-client.js'
import { repairToolArguments } from '../src/model-test-support/model/compat-model-client.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import { createThreadRecord } from '../src/domain/thread.js'
import {
  makeAssistantReasoningItem,
  makeAssistantTextItem,
  makeCompactionItem,
  makePrivateToolCallItem,
  makePrivateToolResultItem,
  makePublicToolResultWithheldProjection,
  makeToolCallItem,
  makeToolResultItem,
  makeUserItem
} from '../src/domain/item.js'
import type { ModelRequest, ModelStreamChunk } from '../src/ports/model-client.js'

function buildRequest(abortSignal: AbortSignal): ModelRequest {
  return {
    threadId: 'thr_1',
    turnId: 'turn_1',
    model: 'deepseek-chat',
    systemPrompt: 'You are a helpful assistant.',
    prefix: [],
    history: [],
    tools: [
      {
        name: 'echo',
        description: 'Echo a string back to the model.',
        inputSchema: {
          type: 'object',
          properties: { text: { type: 'string' } },
          required: ['text']
        }
      }
    ],
    abortSignal
  }
}

function cacheSensitiveToolSchema(): Record<string, unknown> {
  return {
    required: ['b', 1, 'a', 'a', false],
    dependentRequired: { b: ['z', 'a', 'z'], bad: true, empty: [] },
    properties: {
      b: { type: 'string', enum: ['right', 'left'] },
      a: { type: 'number' }
    },
    type: ['object']
  }
}

function expectCanonicalToolSchema(schema: unknown): void {
  const parameters = schema as {
    dependentRequired?: Record<string, string[]>
    properties?: Record<string, unknown>
    required?: string[]
    type?: string[]
  }
  expect(Object.keys(parameters.properties ?? {})).toEqual(['a', 'b'])
  expect((parameters.properties?.b as { enum?: string[] } | undefined)?.enum).toEqual(['left', 'right'])
  expect(parameters.required).toEqual(['a', 'b'])
  expect(parameters.dependentRequired).toEqual({ b: ['a', 'z'] })
  expect(parameters.type).toEqual(['object'])
}

function collectKinds(chunks: ModelStreamChunk[]): string[] {
  return chunks.map((chunk) => chunk.kind)
}

function sseStream(payloads: Array<Record<string, unknown> | '[DONE]'>): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      for (const payload of payloads) {
        controller.enqueue(encoder.encode(`data: ${payload === '[DONE]' ? payload : JSON.stringify(payload)}\n\n`))
      }
      controller.close()
    }
  })
}

describe('CompatModelClient', () => {
  it('uses request.model over client default model', async () => {
    const response = {
      id: 'r2',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: {
            role: 'assistant',
            content: 'done'
          }
        }
      ],
      usage: {
        prompt_tokens: 1,
        completion_tokens: 1,
        total_tokens: 2
      }
    }
    const sentBodies: Array<{ model?: string }> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'deepseek-v4-pro'
    for await (const _chunk of client.stream(request)) {
      // drain
    }
    expect(sentBodies[0]?.model).toBe('deepseek-v4-pro')
  })

  it('builds chat completions URLs for base URLs with and without version segments', async () => {
    const cases = [
      ['https://zenmux.ai/api', 'https://zenmux.ai/api/v1/chat/completions'],
      ['https://zenmux.ai/api/v1', 'https://zenmux.ai/api/v1/chat/completions'],
      ['https://zenmux.ai/api/v1/', 'https://zenmux.ai/api/v1/chat/completions'],
      ['https://zenmux.ai/api/v2', 'https://zenmux.ai/api/v2/chat/completions'],
      ['https://api.deepseek.com/beta', 'https://api.deepseek.com/v1/chat/completions'],
      ['https://api.deepseek.com', 'https://api.deepseek.com/v1/chat/completions']
    ]

    for (const [baseUrl, expectedUrl] of cases) {
      const sentUrls: string[] = []
      const fetchImpl: typeof fetch = async (url) => {
        sentUrls.push(String(url))
        return new Response(JSON.stringify({
          id: 'url',
          model: 'deepseek-chat',
          choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'done' } }]
        }), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        })
      }
      const client = new CompatModelClient({
        baseUrl,
        apiKey: 'k',
        model: 'deepseek-chat',
        fetchImpl,
        nonStreaming: true
      })

      for await (const _chunk of client.stream(buildRequest(new AbortController().signal))) {
        // drain
      }

      expect(sentUrls[0]).toBe(expectedUrl)
    }
  })

  it('routes a thread providerId to that provider full custom endpoint', async () => {
    const threadStore = new InMemoryThreadStore()
    await threadStore.upsert(createThreadRecord({
      id: 'thr_zai',
      title: 'Z.ai turn',
      workspace: '/tmp/project',
      model: 'glm-5',
      providerId: 'zai-coding-plan'
    }))
    const sent: Array<{ url: string; body: { model?: string }; authorization?: string }> = []
    const fetchImpl: typeof fetch = async (url, init) => {
      sent.push({
        url: String(url),
        body: JSON.parse(String(init?.body ?? '{}')),
        authorization: new Headers(init?.headers).get('authorization') ?? undefined
      })
      return new Response(sseStream([
        {
          id: 'chunk_1',
          model: 'glm-5',
          choices: [{ index: 0, delta: { role: 'assistant', content: 'ok' }, finish_reason: null }]
        },
        {
          id: 'chunk_2',
          model: 'glm-5',
          choices: [{ index: 0, delta: {}, finish_reason: 'stop' }]
        },
        '[DONE]'
      ]), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }
    const defaultClient = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com/beta',
      apiKey: 'sk-default',
      model: 'deepseek-v4-pro',
      fetchImpl
    })
    const client = new MultiProviderModelClient({
      defaultClient,
      threadStore,
      defaultApiKey: 'sk-default',
      defaultModel: 'deepseek-v4-pro',
      providers: [{
        id: 'zai-coding-plan',
        apiKey: 'sk-zai',
        baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
        endpointFormat: 'custom_endpoint',
        models: ['glm-5']
      }],
      fetchImpl
    })
    const request = buildRequest(new AbortController().signal)
    request.threadId = 'thr_zai'
    request.model = 'glm-5'

    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(request)) chunks.push(chunk)
    const diagnostics = await client.diagnosticsForRequest({ threadId: 'thr_zai', model: 'glm-5' })

    expect(sent[0]).toMatchObject({
      url: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
      body: { model: 'glm-5' },
      authorization: 'Bearer sk-zai'
    })
    expect(chunks.some((chunk) => chunk.kind === 'assistant_text_delta' && chunk.text === 'ok')).toBe(true)
    expect(diagnostics).toMatchObject({
      providerId: 'zai-coding-plan',
      providerBaseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
      endpointFormat: 'custom_endpoint',
      configuredModel: 'glm-5'
    })
  })

  it('uses the Responses API format when selected', async () => {
    const sentUrls: string[] = []
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (url, init) => {
      sentUrls.push(String(url))
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'resp_1',
        status: 'completed',
        output_text: 'done',
        usage: { input_tokens: 2, output_tokens: 3, total_tokens: 5 }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/api/v1',
      apiKey: 'k',
      model: 'gpt-5-mini',
      endpointFormat: 'responses',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.maxTokens = 128
    request.responseFormat = 'json_object'
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(request)) {
      chunks.push(chunk)
    }

    expect(sentUrls[0]).toBe('https://example.com/api/v1/responses')
    expect(sentBodies[0]).toMatchObject({
      model: 'deepseek-chat',
      max_output_tokens: 128,
      text: { format: { type: 'json_object' } }
    })
    expect(sentBodies[0]?.input).toEqual(expect.arrayContaining([
      expect.objectContaining({ role: 'system', content: 'You are a helpful assistant.' })
    ]))
    expect(sentBodies[0]?.tools).toEqual([
      expect.objectContaining({
        type: 'function',
        name: 'echo',
        parameters: expect.objectContaining({ type: 'object' })
      })
    ])
    expect(chunks).toEqual([
      { kind: 'assistant_text_delta', text: 'done' },
      expect.objectContaining({ kind: 'usage', usage: expect.objectContaining({ promptTokens: 2, completionTokens: 3 }) }),
      { kind: 'completed', stopReason: 'stop' }
    ])
  })

  it('maps Responses API cached input token details into cache telemetry', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify({
        id: 'resp_cache',
        status: 'completed',
        output_text: 'cached',
        usage: {
          input_tokens: 400,
          output_tokens: 20,
          total_tokens: 420,
          input_tokens_details: { cached_tokens: 300 }
        }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/api/v1',
      apiKey: 'k',
      model: 'gpt-5-mini',
      endpointFormat: 'responses',
      fetchImpl,
      nonStreaming: true
    })

    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const usageChunk = chunks.find((chunk) => chunk.kind === 'usage')
    const usage = usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage : null
    expect(usage).not.toBeNull()
    expect(usage).toMatchObject({
      promptTokens: 400,
      completionTokens: 20,
      totalTokens: 420,
      cachedTokens: 300,
      cacheHitTokens: 300,
      cacheMissTokens: 100
    })
    expect(usage?.cacheHitRate).toBeCloseTo(0.75)
  })

  it('treats cached_tokens zero as known cache telemetry rather than unknown', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify({
        id: 'chatcmpl_zero_cache',
        choices: [{ message: { role: 'assistant', content: 'cold prefix' }, finish_reason: 'stop' }],
        usage: {
          prompt_tokens: 50,
          completion_tokens: 4,
          total_tokens: 54,
          prompt_tokens_details: { cached_tokens: 0 }
        }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://openai-compatible.example/v1',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })

    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const usageChunk = chunks.find((chunk) => chunk.kind === 'usage')
    const usage = usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage : null
    expect(usage).toMatchObject({
      promptTokens: 50,
      completionTokens: 4,
      totalTokens: 54,
      cachedTokens: 0,
      cacheHitTokens: 0,
      cacheMissTokens: 50,
      cacheHitRate: 0
    })
  })

  it('maps provider reasoning token details into usage snapshots', async () => {
    const chatClient = new CompatModelClient({
      baseUrl: 'https://openai-compatible.example/v1',
      apiKey: 'k',
      model: 'reasoner-chat',
      fetchImpl: async () =>
        new Response(JSON.stringify({
          id: 'chat_reasoning',
          model: 'reasoner-chat',
          choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'done' } }],
          usage: {
            prompt_tokens: 20,
            completion_tokens: 15,
            total_tokens: 35,
            completion_tokens_details: { reasoning_tokens: 9 }
          }
        }), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        }),
      nonStreaming: true
    })
    const chatChunks: ModelStreamChunk[] = []
    for await (const chunk of chatClient.stream(buildRequest(new AbortController().signal))) {
      chatChunks.push(chunk)
    }
    const chatUsage = chatChunks.find((chunk) => chunk.kind === 'usage')
    expect(chatUsage).toMatchObject({
      kind: 'usage',
      usage: expect.objectContaining({ reasoningTokens: 9 })
    })

    const responsesClient = new CompatModelClient({
      baseUrl: 'https://openai-compatible.example/v1',
      apiKey: 'k',
      model: 'reasoner-responses',
      endpointFormat: 'responses',
      fetchImpl: async () =>
        new Response(JSON.stringify({
          id: 'resp_reasoning',
          status: 'completed',
          output_text: 'done',
          usage: {
            input_tokens: 30,
            output_tokens: 18,
            total_tokens: 48,
            output_tokens_details: { reasoning_tokens: 11 }
          }
        }), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        }),
      nonStreaming: true
    })
    const responsesChunks: ModelStreamChunk[] = []
    for await (const chunk of responsesClient.stream(buildRequest(new AbortController().signal))) {
      responsesChunks.push(chunk)
    }
    const responsesUsage = responsesChunks.find((chunk) => chunk.kind === 'usage')
    expect(responsesUsage).toMatchObject({
      kind: 'usage',
      usage: expect.objectContaining({ reasoningTokens: 11 })
    })
  })

  it('uses the Anthropic Messages API format when selected', async () => {
    const sentUrls: string[] = []
    const sentBodies: Array<Record<string, unknown>> = []
    const sentHeaders: Array<Record<string, string>> = []
    const fetchImpl: typeof fetch = async (url, init) => {
      sentUrls.push(String(url))
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      sentHeaders.push(init?.headers as Record<string, string>)
      return new Response(JSON.stringify({
        id: 'msg_1',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'hello' }],
        stop_reason: 'end_turn',
        usage: { input_tokens: 4, output_tokens: 2 }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://claude.example',
      apiKey: 'anthropic-key',
      model: 'claude-sonnet-4-5',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(sentUrls[0]).toBe('https://claude.example/v1/messages')
    expect(sentHeaders[0]).toMatchObject({
      Authorization: 'Bearer anthropic-key',
      'x-api-key': 'anthropic-key',
      'anthropic-version': '2023-06-01'
    })
    expect(sentBodies[0]).toMatchObject({
      model: 'deepseek-chat',
      max_tokens: 4096,
      system: [{
        type: 'text',
        text: 'You are a helpful assistant.',
        cache_control: { type: 'ephemeral' }
      }],
      messages: [],
      tools: [{
        name: 'echo',
        description: 'Echo a string back to the model.',
        input_schema: expect.objectContaining({ type: 'object' })
      }]
    })
    expect(chunks).toEqual([
      { kind: 'assistant_text_delta', text: 'hello' },
      expect.objectContaining({ kind: 'usage', usage: expect.objectContaining({ promptTokens: 4, completionTokens: 2 }) }),
      { kind: 'completed', stopReason: 'stop' }
    ])
  })

  it('keeps volatile context out of the Anthropic system block and marks cache breakpoints', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'msg_2',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'ok' }],
        stop_reason: 'end_turn',
        usage: { input_tokens: 4, output_tokens: 2 }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://api.minimaxi.com/anthropic',
      apiKey: 'k',
      model: 'MiniMax-M2.5',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.contextInstructions = ['Tokens used: 4321 — continue the goal.']
    request.history = [
      makeUserItem({ id: 'user_1', turnId: 'turn_1', threadId: 'thr_1', text: 'hello' }),
      makeAssistantTextItem({ id: 'asst_1', turnId: 'turn_1', threadId: 'thr_1', text: 'hi there' }),
      makeUserItem({ id: 'user_2', turnId: 'turn_2', threadId: 'thr_1', text: 'continue' })
    ]
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const body = sentBodies[0]
    // The volatile per-turn instruction must not invalidate the cached
    // system prefix: it trails the history inside the final user turn.
    expect(body.system).toEqual([{
      type: 'text',
      text: 'You are a helpful assistant.',
      cache_control: { type: 'ephemeral' }
    }])
    const messages = body.messages as Array<{ role: string; content: Array<Record<string, unknown>> }>
    const lastMessage = messages[messages.length - 1]
    expect(lastMessage.role).toBe('user')
    const lastBlocks = lastMessage.content
    expect(lastBlocks.some((block) => String(block.text ?? '').includes('Tokens used: 4321'))).toBe(true)
    // Explicit-cache providers (MiniMax) only cache content before
    // cache_control breakpoints: the last two messages carry one.
    expect(lastBlocks[lastBlocks.length - 1].cache_control).toEqual({ type: 'ephemeral' })
    const previousMessage = messages[messages.length - 2]
    const previousBlocks = previousMessage.content
    expect(previousBlocks[previousBlocks.length - 1].cache_control).toEqual({ type: 'ephemeral' })
  })

  it('enables MiniMax M3 adaptive thinking from a model reasoning profile', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'msg_m3',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'ok' }],
        stop_reason: 'end_turn'
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://api.minimaxi.com/anthropic',
      apiKey: 'k',
      model: 'MiniMax-M3',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true,
      modelCapabilities: (model) => ({
        id: model,
        inputModalities: ['text', 'image'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        contextWindowTokens: 1_000_000,
        messageParts: ['text', 'image_url'],
        reasoning: {
          supportedEfforts: ['auto', 'off'],
          defaultEffort: 'auto',
          requestProtocol: 'anthropic-thinking'
        }
      })
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'MiniMax-M3'
    request.reasoningEffort = 'max'
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    expect(sentBodies[0]?.thinking).toEqual({ type: 'adaptive' })
  })

  it('sends Anthropic Messages effort with adaptive thinking from a model reasoning profile', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'msg_effort',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'ok' }],
        stop_reason: 'end_turn'
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/anthropic',
      apiKey: 'k',
      model: 'deepseek-v4-pro',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true,
      modelCapabilities: (model) => ({
        id: model,
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        contextWindowTokens: 1_000_000,
        messageParts: ['text'],
        reasoning: {
          supportedEfforts: ['off', 'low', 'medium', 'high', 'max'],
          defaultEffort: 'max',
          requestProtocol: 'anthropic-thinking'
        }
      })
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'deepseek-v4-pro'
    request.reasoningEffort = 'max'
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    expect(sentBodies[0]?.thinking).toEqual({ type: 'adaptive' })
    expect(sentBodies[0]?.output_config).toEqual({ effort: 'max' })
  })

  it('round-trips Anthropic signed thinking before tool calls', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'msg_signed_thinking',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'ok' }],
        stop_reason: 'end_turn'
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/anthropic',
      apiKey: 'k',
      model: 'claude-sonnet-4-5',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true,
      modelCapabilities: (model) => ({
        id: model,
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        contextWindowTokens: 1_000_000,
        messageParts: ['text'],
        reasoning: {
          supportedEfforts: ['off', 'low', 'medium', 'high', 'max'],
          defaultEffort: 'high',
          requestProtocol: 'anthropic-thinking'
        }
      })
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'claude-sonnet-4-5'
    request.reasoningEffort = 'high'
    request.history = [
      makeAssistantReasoningItem({
        id: 'reasoning_1',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'I should call echo.',
        signature: 'sig_abc',
        status: 'completed'
      }),
      makePrivateToolCallItem({
        id: 'call_1',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'toolu_1',
        toolName: 'echo',
        arguments: { text: 'hello' }
      }),
      makePrivateToolResultItem({
        id: 'result_1',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'toolu_1',
        toolName: 'echo',
        output: 'hello'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages as Array<{ role?: string; content?: unknown }> | undefined
    const assistantMessage = messages?.find((message) => message.role === 'assistant')
    expect(assistantMessage?.content).toEqual([
      { type: 'thinking', thinking: 'I should call echo.', signature: 'sig_abc' },
      expect.objectContaining({ type: 'tool_use', id: 'toolu_1', name: 'echo', input: { text: 'hello' } })
    ])
  })

  it('does not send thinking controls for MiniMax M2.x built-in reasoning profiles', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'msg_m25',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'ok' }],
        stop_reason: 'end_turn'
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://api.minimaxi.com/anthropic',
      apiKey: 'k',
      model: 'MiniMax-M2.5',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true,
      modelCapabilities: (model) => ({
        id: model,
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        contextWindowTokens: 204_800,
        messageParts: ['text'],
        reasoning: {
          supportedEfforts: ['auto'],
          defaultEffort: 'auto',
          requestProtocol: 'none'
        }
      })
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'MiniMax-M2.5'
    request.reasoningEffort = 'off'
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    expect(sentBodies[0]).not.toHaveProperty('thinking')
  })

  it('maps GLM reasoning profiles to GLM thinking request controls', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify({
        id: 'glm_1',
        model: 'glm-5.2',
        choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }]
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4/chat/completions',
      apiKey: 'k',
      model: 'glm-5.2',
      endpointFormat: 'custom_endpoint',
      fetchImpl,
      nonStreaming: true,
      modelCapabilities: (model) => ({
        id: model,
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        contextWindowTokens: 1_000_000,
        messageParts: ['text'],
        reasoning: {
          supportedEfforts: ['off', 'high', 'max'],
          defaultEffort: 'max',
          requestProtocol: 'glm-chat-completions'
        }
      })
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'glm-5.2'
    request.reasoningEffort = 'max'
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    expect(sentBodies[0]?.thinking).toEqual({ type: 'enabled', clear_thinking: true })
    expect(sentBodies[0]).not.toHaveProperty('reasoning_effort')
  })

  it('maps Anthropic usage where input_tokens excludes cache reads and writes', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify({
        id: 'msg_3',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'ok' }],
        stop_reason: 'end_turn',
        usage: {
          input_tokens: 50,
          output_tokens: 10,
          cache_read_input_tokens: 1000,
          cache_creation_input_tokens: 200
        }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://api.minimaxi.com/anthropic',
      apiKey: 'k',
      model: 'MiniMax-M2.5',
      endpointFormat: 'messages',
      fetchImpl,
      nonStreaming: true
    })
    const chunks: ModelStreamChunk[] = []
    const request = buildRequest(new AbortController().signal)
    request.model = 'MiniMax-M2.5'
    for await (const chunk of client.stream(request)) {
      chunks.push(chunk)
    }
    const usageChunk = chunks.find((chunk) => chunk.kind === 'usage')
    const usage = usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage : null
    expect(usage).not.toBeNull()
    expect(usage!.promptTokens).toBe(1250)
    expect(usage!.cacheHitTokens).toBe(1000)
    expect(usage!.cacheMissTokens).toBe(250)
    expect(usage!.totalTokens).toBe(1260)
    expect(usage!.cacheHitRate).toBeCloseTo(0.8)
    expect(usage!.costCny).toBeCloseTo(0.000924)
    expect(usage!.costUsd).toBeUndefined()
  })

  it('streams Responses API text and function calls', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      { type: 'response.output_text.delta', delta: 'hi' },
      {
        type: 'response.output_item.added',
        output_index: 1,
        item: { type: 'function_call', call_id: 'call_echo', name: 'echo', arguments: '' }
      },
      { type: 'response.function_call_arguments.delta', output_index: 1, delta: '{"text":"ok"}' },
      {
        type: 'response.output_item.done',
        output_index: 1,
        item: { type: 'function_call', call_id: 'call_echo', name: 'echo', arguments: '{"text":"ok"}' }
      },
      {
        type: 'response.completed',
        response: {
          status: 'completed',
          output: [{ type: 'function_call', call_id: 'call_echo', name: 'echo', arguments: '{"text":"ok"}' }],
          usage: { input_tokens: 3, output_tokens: 4 }
        }
      }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com',
      apiKey: 'k',
      model: 'gpt-5-mini',
      endpointFormat: 'responses',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(collectKinds(chunks)).toEqual([
      'assistant_text_delta',
      'tool_call_delta',
      'tool_call_delta',
      'tool_call_complete',
      'usage',
      'completed'
    ])
    expect(chunks.filter((chunk) => chunk.kind === 'tool_call_delta')[0]).toMatchObject({
      callId: 'call_echo',
      toolName: 'echo'
    })
    expect(chunks.find((chunk) => chunk.kind === 'tool_call_complete')).toMatchObject({
      callId: 'call_echo',
      toolName: 'echo',
      arguments: { text: 'ok' }
    })
  })

  it('streams Anthropic Messages API text and tool calls', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      {
        type: 'message_start',
        message: { usage: { input_tokens: 5, output_tokens: 1 } }
      },
      {
        type: 'content_block_delta',
        index: 0,
        delta: { type: 'text_delta', text: 'hi' }
      },
      {
        type: 'content_block_start',
        index: 1,
        content_block: { type: 'tool_use', id: 'toolu_1', name: 'echo', input: {} }
      },
      {
        type: 'content_block_delta',
        index: 1,
        delta: { type: 'input_json_delta', partial_json: '{"text":"ok"}' }
      },
      { type: 'content_block_stop', index: 1 },
      { type: 'message_delta', delta: { stop_reason: 'tool_use' }, usage: { output_tokens: 8 } },
      { type: 'message_stop' }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://claude.example',
      apiKey: 'k',
      model: 'claude-sonnet-4-5',
      endpointFormat: 'messages',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(collectKinds(chunks)).toEqual([
      'assistant_text_delta',
      'tool_call_delta',
      'tool_call_delta',
      'tool_call_complete',
      'usage',
      'completed'
    ])
    expect(chunks.filter((chunk) => chunk.kind === 'tool_call_delta')[0]).toMatchObject({
      callId: 'toolu_1',
      toolName: 'echo'
    })
    expect(chunks.find((chunk) => chunk.kind === 'tool_call_complete')).toMatchObject({
      callId: 'toolu_1',
      toolName: 'echo',
      arguments: { text: 'ok' }
    })
    expect(chunks.find((chunk) => chunk.kind === 'usage')).toMatchObject({
      usage: expect.objectContaining({ promptTokens: 5, completionTokens: 8, totalTokens: 13 })
    })
  })

  it('streams Anthropic signed thinking signature deltas', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      {
        type: 'content_block_delta',
        index: 0,
        delta: { type: 'thinking_delta', thinking: 'private reasoning' }
      },
      {
        type: 'content_block_delta',
        index: 0,
        delta: { type: 'signature_delta', signature: 'sig_stream' }
      },
      { type: 'message_delta', delta: { stop_reason: 'end_turn' }, usage: { output_tokens: 3 } },
      { type: 'message_stop' }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://claude.example',
      apiKey: 'k',
      model: 'claude-sonnet-4-5',
      endpointFormat: 'messages',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks).toContainEqual({
      kind: 'assistant_reasoning_delta',
      text: 'private reasoning'
    })
    expect(chunks).toContainEqual({
      kind: 'assistant_reasoning_delta',
      text: '',
      signature: 'sig_stream'
    })
  })

  it('does not inject body.thinking on non-DeepSeek host (issue #26)', async () => {
    const response = {
      id: 'r3',
      model: 'deepseek-chat',
      choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://openrouter.ai/api/v1',   // NOT api.deepseek.com
      apiKey: 'k',
      model: 'deepseek-v4-pro',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'deepseek-v4-pro'
    for await (const _chunk of client.stream(request)) {
      // drain
    }
    // The DeepSeek-specific `thinking` protocol extension must not be sent
    // to third-party OpenAI-compat providers — they may reject it. See issue #26.
    expect(sentBodies[0]).not.toHaveProperty('thinking')
  })

  it('injects body.thinking on the official DeepSeek host (issue #26 regression guard)', async () => {
    const response = {
      id: 'r4',
      model: 'deepseek-chat',
      choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-v4-pro',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'deepseek-v4-pro'
    for await (const _chunk of client.stream(request)) {
      // drain
    }
    // On the official host, the `thinking` field must still be set for v4 models.
    expect(sentBodies[0]).toHaveProperty('thinking')
    expect((sentBodies[0] as { thinking: { type: string } }).thinking).toMatchObject({ type: 'enabled' })
  })

  it('sends per-request router controls when requested', async () => {
    const response = {
      id: 'router',
      model: 'deepseek-v4-flash',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: {
            role: 'assistant',
            content: '{"model":"deepseek-v4-pro","thinking":"max"}'
          }
        }
      ]
    }
    const sentBodies: Array<Record<string, unknown>> = []
    const sentAccept: string[] = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      sentAccept.push(String((init?.headers as Record<string, string>).Accept ?? ''))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'deepseek-v4-flash'
    request.tools = []
    request.stream = false
    request.maxTokens = 96
    request.temperature = 0
    request.responseFormat = 'json_object'
    request.reasoningEffort = 'off'
    for await (const _chunk of client.stream(request)) {
      // drain
    }
    expect(sentAccept[0]).toBe('application/json')
    expect(sentBodies[0]).toMatchObject({
      model: 'deepseek-v4-flash',
      stream: false,
      max_tokens: 96,
      temperature: 0,
      response_format: { type: 'json_object' },
      thinking: { type: 'disabled' }
    })
  })

  it('requests usage in streaming responses', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const sentHeaders: Array<Record<string, string>> = []
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}\n\n'))
        controller.enqueue(encoder.encode('data: [DONE]\n\n'))
        controller.close()
      }
    })
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      sentHeaders.push(init?.headers as Record<string, string>)
      return new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })

    for await (const _chunk of client.stream(buildRequest(new AbortController().signal))) {
      // drain
    }

    expect(sentBodies[0]).toMatchObject({
      stream: true,
      stream_options: { include_usage: true }
    })
    expect(sentHeaders[0]?.Accept).toBeUndefined()
  })

  it('keeps requiredToolName as loop metadata instead of sending provider tool_choice', async () => {
    const response = {
      id: 'required-tool-metadata',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ]
    }
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.requiredToolName = 'echo'
    for await (const _chunk of client.stream(request)) {
      // drain
    }
    expect(sentBodies[0]).toHaveProperty('tools')
    expect(sentBodies[0]).not.toHaveProperty('tool_choice')
  })

  it('passes the request abort signal to fetch', async () => {
    const controller = new AbortController()
    let seenSignal: AbortSignal | undefined
    const fetchImpl: typeof fetch = async (_url, init) => {
      seenSignal = init?.signal as AbortSignal | undefined
      return new Response(JSON.stringify({
        id: 'signal',
        model: 'deepseek-chat',
        choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'done' } }]
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    for await (const _chunk of client.stream(buildRequest(controller.signal))) {
      // drain
    }
    expect(seenSignal).toBe(controller.signal)
  })

  it('strips DeepSeek thinking payload for Azure OpenAI-compatible endpoints', async () => {
    const response = {
      id: 'azure',
      model: 'gpt-4.1',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ]
    }
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.openai.azure.com/openai/deployments/demo',
      apiKey: 'k',
      model: 'gpt-4.1',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'gpt-4.1'
    request.reasoningEffort = 'high'
    for await (const _chunk of client.stream(request)) {
      // drain
    }
    expect(sentBodies[0]?.reasoning_effort).toBe('high')
    expect(sentBodies[0]).not.toHaveProperty('thinking')
  })

  it('maps Xiaomi max reasoning to the highest supported Xiaomi effort from model profiles', async () => {
    const response = {
      id: 'xiaomi',
      model: 'mimo-v2.5-pro',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ]
    }
    const sentBodies: Array<Record<string, unknown>> = []
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://api.xiaomimimo.com/v1',
      apiKey: 'k',
      model: 'mimo-v2.5-pro',
      fetchImpl,
      nonStreaming: true,
      modelCapabilities: (model) => ({
        id: model,
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        contextWindowTokens: 1_000_000,
        messageParts: ['text'],
        reasoning: {
          supportedEfforts: ['off', 'low', 'medium', 'high'],
          defaultEffort: 'high',
          requestProtocol: 'mimo-chat-completions'
        }
      })
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'mimo-v2.5-pro'
    request.reasoningEffort = 'max'
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    expect(sentBodies[0]?.reasoning_effort).toBe('high')
    expect(sentBodies[0]?.thinking).toEqual({ type: 'enabled' })
  })

  it('parses a non-streaming JSON response into chunks', async () => {
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'tool_calls',
          message: {
            role: 'assistant',
            content: 'I will run the tool.',
            reasoning_content: 'I should call echo.',
            tool_calls: [
              {
                id: 'call_1',
                type: 'function',
                function: {
                  name: 'echo',
                  arguments: JSON.stringify({ text: 'hi' })
                }
              }
            ]
          }
        }
      ],
      usage: {
        prompt_tokens: 50,
        completion_tokens: 10,
        total_tokens: 60,
        prompt_tokens_details: { cached_tokens: 30 }
      }
    }
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }
    const textChunk = chunks.find((c) => c.kind === 'assistant_text_delta')
    const reasoningChunk = chunks.find((c) => c.kind === 'assistant_reasoning_delta')
    const callChunk = chunks.find((c) => c.kind === 'tool_call_complete')
    const usageChunk = chunks.find((c) => c.kind === 'usage')
    const completionChunk = chunks.find((c) => c.kind === 'completed')
    expect(textChunk && textChunk.kind === 'assistant_text_delta' ? textChunk.text : '').toBe(
      'I will run the tool.'
    )
    expect(
      reasoningChunk && reasoningChunk.kind === 'assistant_reasoning_delta' ? reasoningChunk.text : ''
    ).toBe('I should call echo.')
    expect(
      callChunk && callChunk.kind === 'tool_call_complete' ? callChunk.arguments : {}
    ).toEqual({ text: 'hi' })
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.cacheHitTokens : 0).toBe(30)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.cacheMissTokens : 0).toBe(20)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.costUsd : 0).toBeGreaterThan(0)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.costCny : 0).toBeGreaterThan(0)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.cacheSavingsUsd : undefined).toBeUndefined()
    expect(
      completionChunk && completionChunk.kind === 'completed' ? completionChunk.stopReason : ''
    ).toBe('tool_calls')
  })

  it('splits a leading non-streaming <think> block into reasoning', async () => {
    const response = {
      id: 'think_nonstream',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: {
            role: 'assistant',
            content: '\n\n  <think>private chain</think>\n\nvisible answer'
          }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const reasoning = chunks
      .filter((chunk) => chunk.kind === 'assistant_reasoning_delta')
      .map((chunk) => (chunk as { text: string }).text)
      .join('')
    const text = chunks
      .filter((chunk) => chunk.kind === 'assistant_text_delta')
      .map((chunk) => (chunk as { text: string }).text)
      .join('')

    expect(reasoning).toBe('private chain')
    expect(text).toBe('visible answer')
  })

  it('repairs fenced non-streaming tool arguments', async () => {
    const response = {
      id: 'repair',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'tool_calls',
          message: {
            role: 'assistant',
            content: null,
            tool_calls: [
              {
                id: 'call_repair',
                type: 'function',
                function: {
                  name: 'echo',
                  arguments: '```json\n{"text":"repaired"}\n```'
                }
              }
            ]
          }
        }
      ]
    }
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }
    const callChunk = chunks.find((c) => c.kind === 'tool_call_complete')
    expect(callChunk && callChunk.kind === 'tool_call_complete' ? callChunk.arguments : {})
      .toEqual({ text: 'repaired' })
  })

  it('closes truncated JSON tool arguments without swallowing non-json guidance', () => {
    expect(repairToolArguments('{"time": 2').arguments).toEqual({ time: 2 })
    expect(repairToolArguments('{"command": "ls -la').arguments).toEqual({ command: 'ls -la' })
    expect(repairToolArguments('{"a": 1,').arguments).toEqual({ a: 1 })
    expect(repairToolArguments('{"a":').arguments).toEqual({ a: null })
    expect(repairToolArguments('{"path": "C:\\\\tmp\\').arguments).toEqual({ path: 'C:\\tmp' })
    expect(repairToolArguments('{"items": [1, 2').arguments).toEqual({ items: [1, 2] })
    expect(repairToolArguments('[1, 2').arguments).toEqual({ value: [1, 2] })
    expect(repairToolArguments('total garbage').arguments).toEqual({ __raw: 'total garbage' })
  })

  it('repairs truncated non-streaming tool arguments', async () => {
    const response = {
      id: 'repair_truncated',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'tool_calls',
          message: {
            role: 'assistant',
            content: null,
            tool_calls: [
              {
                id: 'call_truncated_repair',
                type: 'function',
                function: {
                  name: 'echo',
                  arguments: '{"text":"repaired'
                }
              }
            ]
          }
        }
      ]
    }
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }
    const callChunk = chunks.find((c) => c.kind === 'tool_call_complete')
    expect(callChunk && callChunk.kind === 'tool_call_complete' ? callChunk.arguments : {})
      .toEqual({ text: 'repaired' })
  })

  it('prefers DeepSeek native prompt cache hit and miss counters', async () => {
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: {
        prompt_tokens: 1000,
        completion_tokens: 10,
        total_tokens: 1010,
        prompt_cache_hit_tokens: 930,
        prompt_cache_miss_tokens: 70,
        prompt_tokens_details: { cached_tokens: 123 }
      }
    }
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }
    const usageChunk = chunks.find((c) => c.kind === 'usage')
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.cacheHitTokens : 0).toBe(930)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.cacheMissTokens : 0).toBe(70)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.cacheHitRate : 0).toBeCloseTo(0.93)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.costUsd : 0).toBeCloseTo(0.000015204)
    expect(usageChunk && usageChunk.kind === 'usage' ? usageChunk.usage.costCny : 0).toBeCloseTo(0.0001086)
  })

  it('sends tools in a canonical order for a stable cache prefix', async () => {
    const sentBodies: Array<{ tools?: Array<{ function?: { name?: string; parameters?: unknown } }> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.tools = [
      {
        name: 'zeta',
        description: 'z',
        inputSchema: cacheSensitiveToolSchema(),
        toolKind: 'subagent',
        snipHint: { head: 80, tail: 8, headChars: 10_000, tailChars: 1_000 }
      },
      {
        name: 'alpha',
        description: 'a',
        inputSchema: { type: 'object', properties: { z: { type: 'string' }, a: { type: 'string' } } },
        toolKind: 'tool_call',
        snipHint: { head: 120, tail: 12, headChars: 12_000, tailChars: 2_000 }
      }
    ]
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const sentBody = sentBodies[0]
    expect(sentBody?.tools?.map((tool) => tool.function?.name)).toEqual(['alpha', 'zeta'])
    expectCanonicalToolSchema(sentBody?.tools?.[1]?.function?.parameters)
    expect(JSON.stringify(sentBody?.tools)).not.toMatch(/toolKind|snipHint|headChars|tailChars|subagent/)
  })

  it('canonicalizes tool schemas for OpenAI responses and Anthropic messages bodies', async () => {
    const responsesBodies: Array<{
      tools?: Array<{ name?: string; parameters?: unknown }>
    }> = []
    const responsesClient = new CompatModelClient({
      baseUrl: 'https://openai-compatible.example/v1',
      apiKey: 'k',
      model: 'deepseek-chat',
      endpointFormat: 'responses',
      fetchImpl: async (_url, init) => {
        responsesBodies.push(JSON.parse(String(init?.body ?? '{}')))
        return new Response(JSON.stringify({
          id: 'resp_tool_schema',
          status: 'completed',
          output_text: 'done'
        }), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        })
      },
      nonStreaming: true
    })
    const responsesRequest = buildRequest(new AbortController().signal)
    responsesRequest.tools = [{
      name: 'zeta',
      description: 'z',
      inputSchema: cacheSensitiveToolSchema(),
      toolKind: 'subagent',
      snipHint: { head: 80, tail: 8, headChars: 10_000, tailChars: 1_000 }
    }]
    for await (const _chunk of responsesClient.stream(responsesRequest)) {
      // drain
    }

    expect(responsesBodies[0]?.tools?.[0]?.name).toBe('zeta')
    expectCanonicalToolSchema(responsesBodies[0]?.tools?.[0]?.parameters)
    expect(JSON.stringify(responsesBodies[0]?.tools)).not.toMatch(/toolKind|snipHint|headChars|tailChars|subagent/)

    const messagesBodies: Array<{
      tools?: Array<{ name?: string; input_schema?: unknown }>
    }> = []
    const messagesClient = new CompatModelClient({
      baseUrl: 'https://claude.example',
      apiKey: 'anthropic-key',
      model: 'claude-sonnet-4-5',
      endpointFormat: 'messages',
      fetchImpl: async (_url, init) => {
        messagesBodies.push(JSON.parse(String(init?.body ?? '{}')))
        return new Response(JSON.stringify({
          id: 'msg_tool_schema',
          type: 'message',
          role: 'assistant',
          content: [{ type: 'text', text: 'done' }],
          stop_reason: 'end_turn'
        }), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        })
      },
      nonStreaming: true
    })
    const messagesRequest = buildRequest(new AbortController().signal)
    messagesRequest.tools = [{
      name: 'zeta',
      description: 'z',
      inputSchema: cacheSensitiveToolSchema(),
      toolKind: 'subagent',
      snipHint: { head: 80, tail: 8, headChars: 10_000, tailChars: 1_000 }
    }]
    for await (const _chunk of messagesClient.stream(messagesRequest)) {
      // drain
    }

    expect(messagesBodies[0]?.tools?.[0]?.name).toBe('zeta')
    expectCanonicalToolSchema(messagesBodies[0]?.tools?.[0]?.input_schema)
    expect(JSON.stringify(messagesBodies[0]?.tools)).not.toMatch(/toolKind|snipHint|headChars|tailChars|subagent/)
  })

  it('heals incomplete tool-call pairs before sending history upstream', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.history = [
      makeToolResultItem({
        id: 'orphan_result',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_orphan',
        toolName: 'echo',
        output: 'orphan'
      }),
      makeToolCallItem({
        id: 'missing_result_call',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_missing',
        toolName: 'echo',
        arguments: { text: 'missing' }
      }),
      makeUserItem({ id: 'user_after_missing', turnId: 'turn_1', threadId: 'thr_1', text: 'continue' }),
      makeToolCallItem({
        id: 'valid_call',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_ok',
        toolName: 'echo',
        arguments: { text: 'ok' }
      }),
      makeToolResultItem({
        id: 'valid_result',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_ok',
        toolName: 'echo',
        output: 'ok'
      })
    ]
    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    expect(messages.some((message) => message.tool_call_id === 'call_orphan')).toBe(false)
    const missingCallIndex = messages.findIndex((message) =>
      Array.isArray(message.tool_calls) &&
      message.tool_calls.some((call: { id?: string }) => call.id === 'call_missing')
    )
    expect(missingCallIndex).toBeGreaterThanOrEqual(0)
    expect(messages[missingCallIndex + 1]).toMatchObject({
      role: 'tool',
      tool_call_id: 'call_missing',
      content: JSON.stringify(makePublicToolResultWithheldProjection({ status: 'cancelled' }))
    })
    expect(messages.some((message) => message.role === 'user' && message.content === 'continue')).toBe(true)
    expect(
      messages.some((message) =>
        Array.isArray(message.tool_calls) &&
        message.tool_calls.some((call: { id?: string }) => call.id === 'call_ok')
      )
    ).toBe(true)
    expect(messages.some((message) => message.role === 'tool' && message.tool_call_id === 'call_ok')).toBe(true)
  })

  it('groups completed multi-tool blocks into one assistant tool_calls message', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.history = [
      makePrivateToolCallItem({
        id: 'call_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        arguments: { text: 'a' }
      }),
      makeToolCallItem({
        id: 'call_b',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_b',
        toolName: 'echo',
        arguments: { text: 'b' }
      }),
      makeAssistantTextItem({
        id: 'assistant_bridge',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'I will run both checks.',
        status: 'completed'
      }),
      makePrivateToolResultItem({
        id: 'result_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        output: 'a'
      }),
      makeToolResultItem({
        id: 'result_b',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_b',
        toolName: 'echo',
        output: 'b'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    const assistantToolMessage = messages.find((message) => Array.isArray(message.tool_calls))
    const toolMessages = messages.filter((message) => message.role === 'tool')

    expect(assistantToolMessage).toMatchObject({
      role: 'assistant',
      content: 'I will run both checks.'
    })
    expect((assistantToolMessage?.tool_calls as Array<{ id?: string }> | undefined)?.map((call) => call.id))
      .toEqual(['call_a', 'call_b'])
    expect(toolMessages.map((message) => message.tool_call_id)).toEqual(['call_a', 'call_b'])
  })

  it('preserves thinking reasoning_content for completed tool-call blocks', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.reasoningEffort = 'high'
    request.history = [
      makeToolCallItem({
        id: 'call_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        arguments: { text: 'a' }
      }),
      makeToolCallItem({
        id: 'call_b',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_b',
        toolName: 'echo',
        arguments: { text: 'b' }
      }),
      makeAssistantReasoningItem({
        id: 'assistant_reasoning_bridge',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'I need to inspect the current changes before writing the commit message.',
        status: 'completed'
      }),
      makeToolResultItem({
        id: 'result_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        output: 'a'
      }),
      makeToolResultItem({
        id: 'result_b',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_b',
        toolName: 'echo',
        output: 'b'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    const assistantToolMessage = messages.find((message) => Array.isArray(message.tool_calls))

    expect(assistantToolMessage?.reasoning_content).toBe(
      'I need to inspect the current changes before writing the commit message.'
    )
    expect(assistantToolMessage?.content).toBe('')
    expect((assistantToolMessage?.tool_calls as Array<{ id?: string }> | undefined)?.map((call) => call.id))
      .toEqual(['call_a', 'call_b'])
    expect(messages.filter((message) => message.role === 'tool').map((message) => message.tool_call_id))
      .toEqual(['call_a', 'call_b'])
  })

  it('does not re-upload ordinary assistant reasoning_content in thinking mode', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.reasoningEffort = 'high'
    request.history = [
      makeAssistantReasoningItem({
        id: 'assistant_reasoning',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'Private reasoning that should not perturb the next prefix.',
        status: 'completed'
      }),
      makeAssistantTextItem({
        id: 'assistant_text',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'Done.',
        status: 'completed'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const assistantTextMessage = sentBodies[0]?.messages?.find((message) => message.role === 'assistant')

    expect(assistantTextMessage?.content).toBe('Done.')
    expect(assistantTextMessage).not.toHaveProperty('reasoning_content')
  })

  it('uses a single space for empty tool-call reasoning_content only', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.reasoningEffort = 'high'
    request.history = [
      makeAssistantTextItem({
        id: 'assistant_text',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'Done.',
        status: 'completed'
      }),
      makeToolCallItem({
        id: 'call_a',
        turnId: 'turn_2',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        arguments: { text: 'a' }
      }),
      makeToolResultItem({
        id: 'result_a',
        turnId: 'turn_2',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        output: 'a'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    const assistantTextMessage = messages.find((message) => message.role === 'assistant' && message.content === 'Done.')
    const assistantToolMessage = messages.find((message) => Array.isArray(message.tool_calls))

    expect(assistantTextMessage).not.toHaveProperty('reasoning_content')
    expect(assistantToolMessage?.reasoning_content).toBe(' ')
    expect(assistantToolMessage?.content).toBe('')
  })

  it('treats fixed DeepSeek v4 models as thinking producers', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>>; thinking?: unknown; reasoning_effort?: unknown }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-v4-pro',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.model = 'deepseek-v4-pro'
    request.history = [
      makeAssistantTextItem({
        id: 'assistant_text',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'Done.',
        status: 'completed'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const body = sentBodies[0]
    const assistantMessage = body?.messages?.find((message) => message.role === 'assistant')

    expect(body?.thinking).toEqual({ type: 'enabled' })
    expect(body?.reasoning_effort).toBeUndefined()
    expect(assistantMessage).not.toHaveProperty('reasoning_content')
  })

  it('preserves thinking reasoning_content that appears before tool calls', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.reasoningEffort = 'high'
    request.history = [
      makeAssistantReasoningItem({
        id: 'assistant_reasoning_before_call',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'I should inspect git status before answering.',
        status: 'completed'
      }),
      makeAssistantTextItem({
        id: 'assistant_text_before_call',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'I will inspect the changes.',
        status: 'completed'
      }),
      makeToolCallItem({
        id: 'call_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        arguments: { text: 'a' }
      }),
      makeToolResultItem({
        id: 'result_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        output: 'a'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const assistantToolMessage = sentBodies[0]?.messages?.find((message) => Array.isArray(message.tool_calls))
    const assistantMessages = sentBodies[0]?.messages?.filter((message) => message.role === 'assistant') ?? []

    expect(assistantMessages).toHaveLength(1)
    expect(assistantToolMessage?.content).toBe('I will inspect the changes.')
    expect(assistantToolMessage?.reasoning_content).toBe('I should inspect git status before answering.')
  })

  it('serializes undefined tool outputs as empty string content', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.history = [
      makePrivateToolCallItem({
        id: 'call_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        arguments: { text: 'a' }
      }),
      makePrivateToolResultItem({
        id: 'result_a',
        turnId: 'turn_1',
        threadId: 'thr_1',
        callId: 'call_a',
        toolName: 'echo',
        output: undefined
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const toolMessage = sentBodies[0]?.messages?.find((message) => message.role === 'tool')

    expect(toolMessage?.content).toBe('')
  })

  it('sends compaction summaries as mutable system messages', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.history = [
      makeCompactionItem({
        id: 'compact_1',
        turnId: 'turn_1',
        threadId: 'thr_1',
        summary: 'User wants the login feature finished. Keep the auth files in scope.',
        replacedTokens: 123,
        pinnedConstraints: []
      }),
      makeUserItem({ id: 'user_after_compact', turnId: 'turn_2', threadId: 'thr_1', text: 'continue' })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    expect(messages[0]).toMatchObject({ role: 'system', content: 'You are a helpful assistant.' })
    expect(messages[1]).toMatchObject({
      role: 'system',
      content: expect.stringContaining('User wants the login feature finished')
    })
    expect(messages[2]).toMatchObject({ role: 'user', content: 'continue' })
  })

  it('sends volatile context instructions after the history for cache prefix stability', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.contextInstructions = ['Tokens used: 4321 — continue the goal.']
    request.history = [
      makeUserItem({ id: 'user_1', turnId: 'turn_1', threadId: 'thr_1', text: 'hello' })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    const instructionIndex = messages.findIndex(
      (message) => typeof message.content === 'string' && message.content.includes('Tokens used: 4321')
    )
    const userIndex = messages.findIndex((message) => message.role === 'user')
    expect(instructionIndex).toBeGreaterThan(userIndex)
    expect(messages[instructionIndex]).toMatchObject({ role: 'system' })
  })

  it('wraps steering user messages with current-turn guidance for provider history replay', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true
    })
    const request = buildRequest(new AbortController().signal)
    request.history = [
      makeUserItem({
        id: 'user_steer',
        turnId: 'turn_1',
        threadId: 'thr_1',
        text: 'prefer the CSV route',
        delivery: 'steer'
      })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    const user = messages.find((message) => message.role === 'user')
    expect(user?.content).toContain('Mid-turn user follow-up for the current task')
    expect(user?.content).toContain('prefer the CSV route')
  })

  it('preserves the latest compaction summary when applying history limits', async () => {
    const sentBodies: Array<{ messages?: Array<Record<string, unknown>> }> = []
    const response = {
      id: 'r1',
      model: 'deepseek-chat',
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'done' }
        }
      ],
      usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 }
    }
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')))
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      nonStreaming: true,
      historyLimit: 2
    })
    const request = buildRequest(new AbortController().signal)
    request.history = [
      makeCompactionItem({
        id: 'compact_1',
        turnId: 'turn_1',
        threadId: 'thr_1',
        summary: 'Keep original requirement beta.',
        replacedTokens: 50,
        pinnedConstraints: []
      }),
      makeUserItem({ id: 'old_1', turnId: 'turn_2', threadId: 'thr_1', text: 'old detail one' }),
      makeUserItem({ id: 'old_2', turnId: 'turn_3', threadId: 'thr_1', text: 'old detail two' }),
      makeUserItem({ id: 'latest', turnId: 'turn_4', threadId: 'thr_1', text: 'latest question' })
    ]

    for await (const _chunk of client.stream(request)) {
      // drain
    }

    const messages = sentBodies[0]?.messages ?? []
    expect(JSON.stringify(messages)).toContain('Keep original requirement beta')
    expect(JSON.stringify(messages)).not.toContain('old detail two')
    expect(messages.at(-1)).toMatchObject({ role: 'user', content: 'latest question' })
  })

  it('reports an error when the HTTP response is not OK', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const providerMessage = `Not supported model ${'mimo-v2.5-pro-ultraspeed'.repeat(40)}`
    const body = JSON.stringify({ error: { code: '400', message: providerMessage } })
    const fetchImpl: typeof fetch = async () =>
      new Response(body, { status: 400 })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }
    warn.mockRestore()
    expect(chunks[0].kind).toBe('error')
    expect(chunks[0]).toMatchObject({
      kind: 'error',
      message: `model request failed with status 400: ${body}`,
      code: 'http_400'
    })
    expect(JSON.stringify(chunks[0])).toContain(providerMessage)
  })

  it('adds a provider configuration hint while projecting the HTTP failure warning', async () => {
    const requestModelSentinel = 'model-/private/analytix-rc-k-s03/REQUEST_MODEL'
    const configuredModelSentinel = 'model-/private/analytix-rc-k-s03/CONFIGURED_MODEL'
    const baseUrlSentinel = 'https://api.example.com/private/analytix-rc-k-s03/BASE_URL/chat/completions?api_key=SECRET_SENTINEL'
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const fetchImpl: typeof fetch = async () => new Response('', { status: 404 })
    const client = new CompatModelClient({
      baseUrl: baseUrlSentinel,
      apiKey: 'k',
      model: configuredModelSentinel,
      endpointFormat: 'custom_endpoint',
      fetchImpl
    })
    const request = buildRequest(new AbortController().signal)
    request.model = requestModelSentinel
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(request)) {
      chunks.push(chunk)
    }

    expect(chunks[0]).toMatchObject({
      kind: 'error',
      message: 'model request failed with status 404: Check your model provider configuration, especially Base URL and Endpoint format.',
      code: 'http_404'
    })
    expect(warn).toHaveBeenCalledTimes(1)
    expect(warn).toHaveBeenCalledWith(
      '[analytix] event=ANALYTIX_MODEL_HTTP_REQUEST_FAILED'
    )
    const serializedWarnings = JSON.stringify(warn.mock.calls)
    expect(serializedWarnings).not.toContain(requestModelSentinel)
    expect(serializedWarnings).not.toContain(configuredModelSentinel)
    expect(serializedWarnings).not.toContain('/private/analytix-rc-k-s03/BASE_URL')
    expect(serializedWarnings).not.toContain('SECRET_SENTINEL')
    warn.mockRestore()
  })

  it('reports provider JSON error payloads returned with HTTP 200', async () => {
    const fetchImpl: typeof fetch = async () =>
      new Response(JSON.stringify({
        error: {
          message: 'model mimo-v2.5-pro-ultraspeed is not available for this account',
          code: 'model_not_available'
        }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    const client = new CompatModelClient({
      baseUrl: 'https://api.xiaomimimo.com/v1',
      apiKey: 'k',
      model: 'mimo-v2.5-pro-ultraspeed',
      fetchImpl,
      nonStreaming: true
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks).toEqual([
      {
        kind: 'error',
        message: 'model mimo-v2.5-pro-ultraspeed is not available for this account',
        code: 'model_not_available'
      }
    ])
  })

  it('reports streamed provider error payloads returned with HTTP 200', async () => {
    const body = sseStream([
      {
        error: {
          message: 'no permission to access model mimo-v2.5-pro-ultraspeed',
          type: 'permission_denied'
        }
      },
      '[DONE]'
    ])
    const fetchImpl: typeof fetch = async () =>
      new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    const client = new CompatModelClient({
      baseUrl: 'https://api.xiaomimimo.com/v1',
      apiKey: 'k',
      model: 'mimo-v2.5-pro-ultraspeed',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.find((chunk) => chunk.kind === 'error')).toMatchObject({
      kind: 'error',
      message: 'no permission to access model mimo-v2.5-pro-ultraspeed',
      code: 'permission_denied'
    })
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toMatchObject({
      kind: 'completed',
      stopReason: 'error'
    })
  })

  it('parses streamed SSE events with tool call deltas', async () => {
    const frames = [
      'data: {"choices":[{"delta":{"content":"Hello "}}]}\n\n',
      'data: {"choices":[{"delta":{"content":"world"}}]}\n\n',
      'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"echo","arguments":"{\\"text\\":"}}]}}]}\n\n',
      'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\"hi\\"}"}}]}}]}\n\n',
      'data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}\n\n',
      'data: [DONE]\n\n'
    ]
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const frame of frames) controller.enqueue(encoder.encode(frame))
        controller.close()
      }
    })
    const fetchImpl: typeof fetch = async () =>
      new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }
    const text = chunks
      .filter((c) => c.kind === 'assistant_text_delta')
      .map((c) => (c as { text: string }).text)
      .join('')
    expect(text).toBe('Hello world')
    const complete = chunks.find((c) => c.kind === 'tool_call_complete')
    expect(complete && complete.kind === 'tool_call_complete' ? complete.callId : '').toBe('call_1')
    expect(complete && complete.kind === 'tool_call_complete' ? complete.arguments : {}).toEqual({ text: 'hi' })
    expect(chunks.find((c) => c.kind === 'usage')).toBeDefined()
  })

  it('splits a leading streamed <think> block across chat deltas', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      { choices: [{ delta: { content: '\n\n  <th' } }] },
      { choices: [{ delta: { content: 'ink>private' } }] },
      { choices: [{ delta: { content: ' chain</thi' } }] },
      { choices: [{ delta: { content: 'nk>\n\nvisible' } }] },
      { choices: [{ delta: {}, finish_reason: 'stop' }] },
      '[DONE]'
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const reasoning = chunks
      .filter((chunk) => chunk.kind === 'assistant_reasoning_delta')
      .map((chunk) => (chunk as { text: string }).text)
      .join('')
    const text = chunks
      .filter((chunk) => chunk.kind === 'assistant_text_delta')
      .map((chunk) => (chunk as { text: string }).text)
      .join('')

    expect(reasoning).toBe('private chain')
    expect(text).toBe('visible')
  })

  it('keeps mid-answer <think> mentions as ordinary streamed text', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      { choices: [{ delta: { content: 'the model emits ' } }] },
      { choices: [{ delta: { content: '<think> tags around its reasoning' } }] },
      { choices: [{ delta: {}, finish_reason: 'stop' }] },
      '[DONE]'
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.some((chunk) => chunk.kind === 'assistant_reasoning_delta')).toBe(false)
    expect(chunks
      .filter((chunk) => chunk.kind === 'assistant_text_delta')
      .map((chunk) => (chunk as { text: string }).text)
      .join('')).toBe('the model emits <think> tags around its reasoning')
  })

  it('keeps reading streamed usage sent after finish_reason', async () => {
    const frames = [
      'data: {"choices":[{"delta":{"content":"done"}}]}\n\n',
      'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n\n',
      'data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}\n\n',
      'data: [DONE]\n\n'
    ]
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const frame of frames) controller.enqueue(encoder.encode(frame))
        controller.close()
      }
    })
    const fetchImpl: typeof fetch = async () =>
      new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const usage = chunks.find((c) => c.kind === 'usage')
    const completed = chunks.find((c) => c.kind === 'completed')
    expect(usage && usage.kind === 'usage' ? usage.usage.totalTokens : 0).toBe(10)
    expect(completed && completed.kind === 'completed' ? completed.stopReason : '').toBe('stop')
  })

  it('retries without stream usage options when a provider rejects them', async () => {
    const sentBodies: Array<Record<string, unknown>> = []
    const encoder = new TextEncoder()
    const retryBody = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('data: {"choices":[{"delta":{"content":"retried"}}]}\n\n'))
        controller.enqueue(
          encoder.encode(
            'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}\n\n'
          )
        )
        controller.enqueue(encoder.encode('data: [DONE]\n\n'))
        controller.close()
      }
    })
    const fetchImpl: typeof fetch = async (_url, init) => {
      sentBodies.push(JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>)
      if (sentBodies.length === 1) {
        return new Response('unknown field stream_options.include_usage', { status: 400 })
      }
      return new Response(retryBody, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    }
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const text = chunks
      .filter((c) => c.kind === 'assistant_text_delta')
      .map((c) => (c as { text: string }).text)
      .join('')
    const usage = chunks.find((c) => c.kind === 'usage')
    expect(sentBodies).toHaveLength(2)
    expect(sentBodies[0]).toHaveProperty('stream_options')
    expect(sentBodies[1]).not.toHaveProperty('stream_options')
    expect(chunks).toContainEqual(expect.objectContaining({
      kind: 'retrying',
      attempt: 1,
      maxAttempt: 1,
      status: 400,
      message: expect.stringContaining('stream_options.include_usage')
    }))
    expect(text).toBe('retried')
    expect(usage && usage.kind === 'usage' ? usage.usage.totalTokens : 0).toBe(7)
  })

  it('merges streamed tool-call deltas by index when the provider id arrives later', async () => {
    const frames = [
      'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"echo","arguments":"{\\"text\\":"}}]}}]}\n\n',
      'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_provider","function":{"arguments":"\\"late-id\\"}"}}]}}]}\n\n',
      'data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}\n\n',
      'data: [DONE]\n\n'
    ]
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const frame of frames) controller.enqueue(encoder.encode(frame))
        controller.close()
      }
    })
    const fetchImpl: typeof fetch = async () =>
      new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const complete = chunks.find((c) => c.kind === 'tool_call_complete')
    expect(complete && complete.kind === 'tool_call_complete' ? complete.callId : '').toBe('call_1')
    expect(complete && complete.kind === 'tool_call_complete' ? complete.arguments : {}).toEqual({ text: 'late-id' })
  })

  it('accepts a clean streamed EOF after a terminal finish reason', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      { choices: [{ index: 0, delta: { content: 'done' }, finish_reason: null }] },
      { choices: [{ index: 0, delta: {}, finish_reason: 'stop' }] }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.find((chunk) => chunk.kind === 'assistant_text_delta')).toMatchObject({ text: 'done' })
    expect(chunks.find((chunk) => chunk.kind === 'error')).toBeUndefined()
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toMatchObject({
      kind: 'completed',
      stopReason: 'stop'
    })
  })

  it('rejects a clean streamed EOF before chat tool calls complete', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      {
        choices: [{
          index: 0,
          delta: {
            tool_calls: [{
              index: 0,
              id: 'call_1',
              function: { name: 'echo', arguments: '{"text":' }
            }]
          }
        }]
      },
      {
        choices: [{
          index: 0,
          delta: { tool_calls: [{ index: 0, function: { arguments: '"partial"' } }] }
        }]
      }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    const deltas = chunks.filter((chunk) => chunk.kind === 'tool_call_delta')
    expect(deltas).toHaveLength(3)
    expect(deltas[0]).toMatchObject({
      kind: 'tool_call_delta',
      callId: 'call_1',
      toolName: 'echo'
    })
    expect(deltas[1]).toMatchObject({
      kind: 'tool_call_delta',
      callId: 'call_1',
      toolName: 'echo',
      argumentsDelta: '{"text":'
    })
    expect(deltas[2]).toMatchObject({
      kind: 'tool_call_delta',
      callId: 'call_1',
      toolName: 'echo',
      argumentsDelta: '"partial"'
    })
    expect(chunks.find((chunk) => chunk.kind === 'tool_call_complete')).toBeUndefined()
    expect(chunks.find((chunk) => chunk.kind === 'error')).toMatchObject({
      kind: 'error',
      code: 'stream_incomplete_tool_call'
    })
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toBeUndefined()
  })

  it('rejects a clean streamed EOF before Responses API tool calls complete', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      {
        type: 'response.output_item.added',
        output_index: 0,
        item: { type: 'function_call', call_id: 'call_echo', name: 'echo', arguments: '' }
      },
      { type: 'response.function_call_arguments.delta', output_index: 0, delta: '{"text":' }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com',
      apiKey: 'k',
      model: 'gpt-5-mini',
      endpointFormat: 'responses',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.find((chunk) => chunk.kind === 'tool_call_delta' && chunk.argumentsDelta)).toMatchObject({
      callId: 'call_echo',
      argumentsDelta: '{"text":'
    })
    expect(chunks.find((chunk) => chunk.kind === 'tool_call_complete')).toBeUndefined()
    expect(chunks.find((chunk) => chunk.kind === 'error')).toMatchObject({
      kind: 'error',
      code: 'stream_incomplete_tool_call'
    })
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toBeUndefined()
  })

  it('rejects a clean streamed EOF before Anthropic Messages tool calls complete', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      {
        type: 'content_block_start',
        index: 1,
        content_block: { type: 'tool_use', id: 'toolu_1', name: 'echo', input: {} }
      },
      {
        type: 'content_block_delta',
        index: 1,
        delta: { type: 'input_json_delta', partial_json: '{"text":' }
      }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://claude.example',
      apiKey: 'k',
      model: 'claude-sonnet-4-5',
      endpointFormat: 'messages',
      fetchImpl
    })
    const chunks: ModelStreamChunk[] = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.find((chunk) => chunk.kind === 'tool_call_delta' && chunk.argumentsDelta)).toMatchObject({
      callId: 'toolu_1',
      argumentsDelta: '{"text":'
    })
    expect(chunks.find((chunk) => chunk.kind === 'tool_call_complete')).toBeUndefined()
    expect(chunks.find((chunk) => chunk.kind === 'error')).toMatchObject({
      kind: 'error',
      code: 'stream_incomplete_tool_call'
    })
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toBeUndefined()
  })

  it('rejects a clean streamed EOF before text completion', async () => {
    const fetchImpl: typeof fetch = async () => new Response(sseStream([
      { choices: [{ index: 0, delta: { content: 'partial' } }] }
    ]), {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.find((chunk) => chunk.kind === 'assistant_text_delta')).toMatchObject({ text: 'partial' })
    expect(chunks.find((chunk) => chunk.kind === 'error')).toMatchObject({
      kind: 'error',
      code: 'stream_ended_before_completion'
    })
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toBeUndefined()
  })

  it('fails a streamed response that goes idle without DONE', async () => {
    const encoder = new TextEncoder()
    let cancelReason: unknown
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('data: {"choices":[{"delta":{"content":"partial"}}]}\n\n'))
      },
      cancel(reason) {
        cancelReason = reason
      }
    })
    const fetchImpl: typeof fetch = async () =>
      new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } })
    const client = new CompatModelClient({
      baseUrl: 'https://example.com/beta',
      apiKey: 'k',
      model: 'deepseek-chat',
      fetchImpl,
      streamIdleTimeoutMs: 5
    })
    const chunks = []
    for await (const chunk of client.stream(buildRequest(new AbortController().signal))) {
      chunks.push(chunk)
    }

    expect(chunks.find((chunk) => chunk.kind === 'assistant_text_delta')).toMatchObject({
      text: 'partial'
    })
    expect(chunks.find((chunk) => chunk.kind === 'error')).toMatchObject({
      code: 'stream_idle_timeout'
    })
    expect(cancelReason).toBe('model stream idle timeout')
    expect(chunks.find((chunk) => chunk.kind === 'completed')).toBeUndefined()
  })
})
