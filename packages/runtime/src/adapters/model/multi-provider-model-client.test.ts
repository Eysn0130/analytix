import { describe, expect, it } from 'vitest'
import type { ModelCapabilityMetadata } from '../../contracts/capabilities.js'
import type { ThreadRecord, ThreadSummary } from '../../contracts/threads.js'
import type { ModelRequest, ModelStreamChunk } from '../../ports/model-client.js'
import type { ThreadStore } from '../../ports/thread-store.js'
import { CompatModelClient } from '../../model-test-support/model/compat-model-client.js'
import { MultiProviderModelClient } from '../../model-test-support/model/multi-provider-model-client.js'

type CapturedCall = {
  url: string
  headers: Record<string, string>
  body: Record<string, unknown>
}

function fakeFetch(calls: CapturedCall[]): typeof fetch {
  return (async (url: string, init?: RequestInit) => {
    const target = String(url)
    const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>
    calls.push({
      url: target,
      headers: headersRecord(init?.headers),
      body
    })
    const json = target.includes('/responses')
      ? {
          id: 'resp_cache',
          status: 'completed',
          output_text: 'responses ok',
          usage: {
            input_tokens: 400,
            output_tokens: 20,
            total_tokens: 420,
            input_tokens_details: { cached_tokens: 300 },
            output_tokens_details: { reasoning_tokens: 11 }
          }
        }
      : target.includes('/messages')
        ? {
            content: [{ type: 'text', text: 'messages ok' }],
            stop_reason: 'end_turn',
            usage: {
              input_tokens: 50,
              output_tokens: 10,
              cache_read_input_tokens: 1000,
              cache_creation_input_tokens: 200
            }
          }
        : { choices: [{ index: 0, finish_reason: 'stop', message: { content: 'chat ok' } }] }
    return new Response(JSON.stringify(json), {
      status: 200,
      headers: { 'content-type': 'application/json' }
    })
  }) as unknown as typeof fetch
}

function headersRecord(headers: RequestInit['headers']): Record<string, string> {
  if (!headers) return {}
  if (headers instanceof Headers) return Object.fromEntries(headers.entries())
  if (Array.isArray(headers)) return Object.fromEntries(headers)
  return headers as Record<string, string>
}

function storeWithThreads(records: Record<string, Partial<ThreadRecord>>): ThreadStore {
  return {
    list: async () => [] satisfies ThreadSummary[],
    get: async (threadId) => records[threadId] as ThreadRecord | undefined ?? null,
    upsert: async (thread) => thread,
    delete: async () => false
  }
}

function request(threadId: string, model: string, providerId?: string): ModelRequest {
  return {
    threadId,
    turnId: `${threadId}-turn`,
    model,
    ...(providerId ? { providerId } : {}),
    systemPrompt: 'You are Analytix.',
    prefix: [],
    history: [],
    tools: [],
    stream: false,
    abortSignal: new AbortController().signal
  }
}

async function drain(iterable: AsyncIterable<ModelStreamChunk>): Promise<ModelStreamChunk[]> {
  const chunks: ModelStreamChunk[] = []
  for await (const chunk of iterable) chunks.push(chunk)
  return chunks
}

async function expectProviderNotFound(input: Promise<unknown>, providerId: string, source: 'request' | 'thread'): Promise<void> {
  await expect(input).rejects.toMatchObject({
    code: 'provider_not_found',
    providerId,
    source
  })
}

async function expectInvalidModel(input: Promise<unknown>, providerId: string, model: string): Promise<void> {
  await expect(input).rejects.toMatchObject({
    code: 'invalid_model',
    providerId,
    model
  })
}

function usageFrom(chunks: ModelStreamChunk[]) {
  const usage = chunks.find((chunk) => chunk.kind === 'usage')
  if (!usage || usage.kind !== 'usage') throw new Error('missing usage chunk')
  return usage.usage
}

function textCapability(model: string, endpointFormat?: ModelCapabilityMetadata['endpointFormat']): ModelCapabilityMetadata {
  return {
    id: model,
    inputModalities: ['text'],
    outputModalities: ['text'],
    supportsToolCalling: true,
    messageParts: ['text'],
    ...(endpointFormat ? { endpointFormat } : {})
  }
}

function multiProviderClient(
  calls: CapturedCall[],
  threadStore: ThreadStore,
  modelCapabilities?: (model: string) => ModelCapabilityMetadata
): MultiProviderModelClient {
  const fetchImpl = fakeFetch(calls)
  const defaultClient = new CompatModelClient({
    baseUrl: 'https://default.example/v1',
    apiKey: 'sk-default',
    model: 'default-model',
    endpointFormat: 'chat_completions',
    nonStreaming: true,
    fetchImpl
  })

  return new MultiProviderModelClient({
    defaultClient,
    threadStore,
    providers: [
      {
        id: 'custom-messages',
        name: 'Custom Messages',
        apiKey: 'sk-custom',
        baseUrl: 'https://gateway.example/custom/messages?tenant=west',
        endpointFormat: 'custom_endpoint',
        models: ['custom-messages-model']
      },
      {
        id: 'mixed-provider',
        name: 'Mixed Provider',
        apiKey: 'sk-mixed',
        baseUrl: 'https://mixed.example/v1',
        endpointFormat: 'chat_completions',
        models: ['glm-5.1', 'minimax-m3']
      },
      {
        id: 'responses-provider',
        name: 'Responses Provider',
        apiKey: 'sk-responses',
        baseUrl: 'https://responses.example/v1',
        endpointFormat: 'responses',
        models: ['gpt-5-mini']
      },
      {
        id: 'messages-provider',
        name: 'Messages Provider',
        apiKey: 'sk-messages',
        baseUrl: 'https://messages.example/anthropic',
        endpointFormat: 'messages',
        models: ['MiniMax-M2.5']
      }
    ],
    defaultApiKey: 'sk-default',
    defaultModel: 'default-model',
    defaultEndpointFormat: 'chat_completions',
    ...(modelCapabilities ? { modelCapabilities } : {}),
    fetchImpl
  })
}

describe('MultiProviderModelClient provider selection', () => {
  it('routes a thread provider to its custom full endpoint and Messages request shape', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ selected: { id: 'selected', providerId: 'custom-messages' } })
    )

    const chunks = await drain(client.stream(request('selected', 'custom-messages-model')))
    const diagnostics = await client.diagnosticsForRequest({ threadId: 'selected', model: 'custom-messages-model' })

    expect(chunks.some((chunk) => chunk.kind === 'assistant_text_delta')).toBe(true)
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('https://gateway.example/custom/messages?tenant=west')
    expect(calls[0].headers['x-api-key']).toBe('sk-custom')
    expect(calls[0].headers['anthropic-version']).toBe('2023-06-01')
    expect(calls[0].headers.Authorization).toBe('Bearer sk-custom')
    expect(calls[0].body.model).toBe('custom-messages-model')
    expect(calls[0].body.messages).toBeDefined()
    expect(calls[0].body.system).toBeDefined()
    expect(calls[0].body).not.toHaveProperty('input')
    expect(calls[0].body).not.toHaveProperty('stream_options')
    expect(diagnostics).toEqual({
      provider: 'compat',
      providerId: 'custom-messages',
      providerBaseUrl: 'https://gateway.example/custom/messages?tenant=west',
      endpointFormat: 'custom_endpoint',
      configuredModel: 'custom-messages-model'
    })
  })

  it('uses an explicit request providerId before thread or default provider selection', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ selected: { id: 'selected', providerId: 'removed-provider' } })
    )

    await drain(client.stream(request('selected', 'custom-messages-model', 'custom-messages')))
    const diagnostics = await client.diagnosticsForRequest({
      threadId: 'selected',
      model: 'custom-messages-model',
      providerId: 'custom-messages'
    })

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('https://gateway.example/custom/messages?tenant=west')
    expect(calls[0].headers['x-api-key']).toBe('sk-custom')
    expect(diagnostics.providerId).toBe('custom-messages')
  })

  it('throws provider_not_found instead of falling back when the thread provider is missing', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ missing: { id: 'missing', providerId: 'removed-provider' } })
    )

    await expectProviderNotFound(
      drain(client.stream(request('missing', 'runtime-selected-model'))),
      'removed-provider',
      'thread'
    )
    await expectProviderNotFound(
      client.diagnosticsForRequest({ threadId: 'missing', model: 'runtime-selected-model' }),
      'removed-provider',
      'thread'
    )

    expect(calls).toHaveLength(0)
  })

  it('throws provider_not_found instead of falling back when an explicit request provider is missing', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ selected: { id: 'selected', providerId: 'custom-messages' } })
    )

    await expectProviderNotFound(
      drain(client.stream(request('selected', 'runtime-selected-model', 'removed-request-provider'))),
      'removed-request-provider',
      'request'
    )
    await expectProviderNotFound(
      client.diagnosticsForRequest({
        threadId: 'selected',
        model: 'runtime-selected-model',
        providerId: 'removed-request-provider'
      }),
      'removed-request-provider',
      'request'
    )

    expect(calls).toHaveLength(0)
  })

  it('does not let child subagent requests fall back when the inherited parent provider is missing', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ child: { id: 'child' } })
    )

    await expectProviderNotFound(
      drain(client.stream(request('child', 'child-inherited-model', 'removed-parent-provider'))),
      'removed-parent-provider',
      'request'
    )

    expect(calls).toHaveLength(0)
  })

  it('rejects a model that is not in the selected bounded provider before fetch', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ selected: { id: 'selected', providerId: 'mixed-provider' } })
    )

    await expectInvalidModel(
      drain(client.stream(request('selected', 'mimo-v2-pro'))),
      'mixed-provider',
      'mimo-v2-pro'
    )
    await expectInvalidModel(
      client.diagnosticsForRequest({ threadId: 'selected', model: 'mimo-v2-pro' }),
      'mixed-provider',
      'mimo-v2-pro'
    )

    expect(calls).toHaveLength(0)
  })

  it('reports per-model endpointFormat overrides in diagnostics', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({ mixed: { id: 'mixed', providerId: 'mixed-provider' } }),
      (model) => textCapability(model, model === 'minimax-m3' ? 'messages' : undefined)
    )

    await drain(client.stream(request('mixed', 'minimax-m3')))
    const diagnostics = await client.diagnosticsForRequest({ threadId: 'mixed', model: 'minimax-m3' })

    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('https://mixed.example/v1/messages')
    expect(calls[0].headers['x-api-key']).toBe('sk-mixed')
    expect(calls[0].body).toMatchObject({
      model: 'minimax-m3',
      messages: expect.any(Array)
    })
    expect(calls[0].body).not.toHaveProperty('input')
    expect(diagnostics).toEqual({
      provider: 'compat',
      providerId: 'mixed-provider',
      providerBaseUrl: 'https://mixed.example/v1',
      endpointFormat: 'messages',
      configuredModel: 'minimax-m3'
    })
  })

  it('preserves provider-native cache and reasoning usage after provider selection', async () => {
    const calls: CapturedCall[] = []
    const client = multiProviderClient(
      calls,
      storeWithThreads({
        responses: { id: 'responses', providerId: 'responses-provider' },
        messages: { id: 'messages', providerId: 'messages-provider' }
      })
    )

    const responsesUsage = usageFrom(await drain(client.stream(request('responses', 'gpt-5-mini'))))
    const messagesUsage = usageFrom(await drain(client.stream(request('messages', 'MiniMax-M2.5'))))

    expect(calls.map((call) => call.url)).toEqual([
      'https://responses.example/v1/responses',
      'https://messages.example/anthropic/v1/messages'
    ])
    expect(calls[0].body).toMatchObject({
      model: 'gpt-5-mini',
      input: expect.any(Array)
    })
    expect(calls[0].body).not.toHaveProperty('messages')
    expect(responsesUsage).toMatchObject({
      promptTokens: 400,
      completionTokens: 20,
      reasoningTokens: 11,
      totalTokens: 420,
      cachedTokens: 300,
      cacheHitTokens: 300,
      cacheMissTokens: 100,
      cacheHitRate: 0.75
    })

    expect(calls[1].body).toMatchObject({
      model: 'MiniMax-M2.5',
      messages: expect.any(Array)
    })
    expect(calls[1].body).not.toHaveProperty('input')
    expect(messagesUsage).toMatchObject({
      promptTokens: 1250,
      completionTokens: 10,
      totalTokens: 1260,
      cachedTokens: 1000,
      cacheHitTokens: 1000,
      cacheMissTokens: 250,
      cacheHitRate: 0.8
    })
  })
})
