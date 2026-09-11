import { describe, expect, it } from 'vitest'
import { CompatModelClient } from '../../model-test-support/model/compat-model-client.js'
import type { ModelCapabilityMetadata } from '../../contracts/capabilities.js'
import type { ModelEndpointFormat } from '../../contracts/model-endpoint-format.js'
import type { ModelHistoryItem, ModelRequest, ModelStreamChunk } from '../../ports/model-client.js'

const SHOT = 'SCREENSHOTBASE64DATA'

type CapturedCall = { url: string; body: Record<string, unknown> }

function caps(vision: boolean, endpointFormat?: ModelEndpointFormat): (model: string) => ModelCapabilityMetadata {
  const imagePart = endpointFormat === 'responses' ? 'input_image' : 'image_url'
  return (model) => ({
    id: model,
    inputModalities: vision ? ['text', 'image'] : ['text'],
    outputModalities: ['text'],
    supportsToolCalling: true,
    messageParts: vision ? ['text', imagePart] : ['text'],
    ...(endpointFormat ? { endpointFormat } : {})
  })
}

function fakeFetch(calls: CapturedCall[]): typeof fetch {
  return (async (url: string, init: { body: string }) => {
    const target = String(url)
    calls.push({ url: target, body: JSON.parse(init.body) as Record<string, unknown> })
    let json: Record<string, unknown>
    if (target.endsWith('/messages')) {
      json = { content: [{ type: 'text', text: 'ok' }], stop_reason: 'end_turn' }
    } else if (target.endsWith('/responses')) {
      json = { output_text: 'ok' }
    } else {
      json = { choices: [{ index: 0, finish_reason: 'stop', message: { content: 'ok' } }] }
    }
    return new Response(JSON.stringify(json), { status: 200, headers: { 'content-type': 'application/json' } })
  }) as unknown as typeof fetch
}

function screenshotHistory(): ModelHistoryItem[] {
  const base = {
    turnId: 'turn_1',
    threadId: 'thread_1',
    status: 'completed' as const,
    createdAt: '2026-01-01T00:00:00.000Z'
  }
  const toolCall: ModelHistoryItem = {
    ...base,
    id: 'item_call',
    role: 'assistant',
    kind: 'tool_call',
    toolName: 'computer_use',
    callId: 'call_1',
    toolKind: 'command_execution',
    arguments: { action: 'screenshot' }
  }
  const toolResult: ModelHistoryItem = {
    ...base,
    id: 'item_result',
    role: 'tool',
    kind: 'tool_result',
    toolName: 'computer_use',
    callId: 'call_1',
    toolKind: 'command_execution',
    isError: false,
    output: {
      kind: 'computer_screenshot',
      action: 'screenshot',
      screen: { width: 1280, height: 800 },
      images: [{ mime_type: 'image/png', data_base64: SHOT, width: 1280, height: 800 }]
    }
  }
  return [toolCall, toolResult]
}

function request(model: string): ModelRequest {
  return {
    threadId: 'thread_1',
    turnId: 'turn_1',
    model,
    systemPrompt: 'sys',
    prefix: [],
    history: screenshotHistory(),
    tools: [],
    abortSignal: new AbortController().signal
  }
}

async function drain(iterable: AsyncIterable<ModelStreamChunk>): Promise<void> {
  for await (const _ of iterable) void _
}

async function collect(iterable: AsyncIterable<ModelStreamChunk>): Promise<ModelStreamChunk[]> {
  const chunks: ModelStreamChunk[] = []
  for await (const chunk of iterable) chunks.push(chunk)
  return chunks
}

function jsonString(value: unknown): string {
  return JSON.stringify(value)
}

describe('CompatModelClient tool-result image forwarding', () => {
  it('forwards screenshots as image_url user messages for vision chat-completions models', async () => {
    const calls: CapturedCall[] = []
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'vision-model',
      endpointFormat: 'chat_completions',
      nonStreaming: true,
      fetchImpl: fakeFetch(calls),
      modelCapabilities: caps(true)
    })

    await drain(client.stream(request('vision-model')))

    const messages = calls[0].body.messages as Array<{ role: string; content: unknown }>
    const toolMessage = messages.find((message) => message.role === 'tool')
    expect(typeof toolMessage?.content).toBe('string')
    expect(jsonString(toolMessage?.content)).not.toContain(SHOT)
    const userImage = messages.find(
      (message) => message.role === 'user' &&
        Array.isArray(message.content) &&
        (message.content as Array<{ type: string }>).some((part) => part.type === 'image_url')
    )
    expect(userImage).toBeDefined()
    expect(jsonString(userImage?.content)).toContain(`data:image/png;base64,${SHOT}`)
  })

  it('forwards screenshots as input_image user content for vision Responses models', async () => {
    const calls: CapturedCall[] = []
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'responses-vision',
      endpointFormat: 'responses',
      nonStreaming: true,
      fetchImpl: fakeFetch(calls),
      modelCapabilities: caps(true, 'responses')
    })

    await drain(client.stream(request('responses-vision')))

    expect(calls[0].url).toMatch(/\/responses$/)
    const input = calls[0].body.input as Array<Record<string, unknown>>
    const toolOutput = input.find((item) => item.type === 'function_call_output')
    expect(typeof toolOutput?.output).toBe('string')
    expect(jsonString(toolOutput?.output)).not.toContain(SHOT)
    const userImage = input.find(
      (item) => item.role === 'user' &&
        Array.isArray(item.content) &&
        (item.content as Array<{ type: string }>).some((part) => part.type === 'input_image')
    )
    expect(userImage).toBeDefined()
    expect(jsonString(userImage?.content)).toContain(`data:image/png;base64,${SHOT}`)
    expect(jsonString(calls[0].body)).not.toContain('"type":"image_url"')
  })

  it('places screenshots as siblings of tool_result blocks for Anthropic messages', async () => {
    const calls: CapturedCall[] = []
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'claude-vision',
      endpointFormat: 'messages',
      nonStreaming: true,
      fetchImpl: fakeFetch(calls),
      modelCapabilities: caps(true, 'messages')
    })

    await drain(client.stream(request('claude-vision')))

    expect(calls[0].url).toMatch(/\/messages$/)
    const messages = calls[0].body.messages as Array<{ role: string; content: unknown }>
    const userMessage = messages.find(
      (message) => message.role === 'user' &&
        Array.isArray(message.content) &&
        (message.content as Array<{ type: string }>).some((block) => block.type === 'tool_result')
    )
    expect(userMessage).toBeDefined()
    const blocks = userMessage!.content as Array<{
      type: string
      content?: unknown
      source?: { data?: string; media_type?: string }
    }>
    const toolResult = blocks.find((block) => block.type === 'tool_result')
    expect(typeof toolResult?.content).toBe('string')
    expect(toolResult?.content).not.toContain(SHOT)
    const imageBlock = blocks.find((block) => block.type === 'image')
    expect(imageBlock?.source?.data).toBe(SHOT)
    expect(imageBlock?.source?.media_type).toBe('image/png')
  })

  it('does not send image parts to text-only models', async () => {
    const calls: CapturedCall[] = []
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'text-model',
      endpointFormat: 'chat_completions',
      nonStreaming: true,
      fetchImpl: fakeFetch(calls),
      modelCapabilities: caps(false)
    })

    await drain(client.stream(request('text-model')))

    const body = jsonString(calls[0].body)
    expect(body).not.toContain('image_url')
    expect(body).not.toContain(SHOT)
    expect(body).toContain('computer_screenshot')
  })

  it('treats unknown models without capability metadata as text-only', async () => {
    const calls: CapturedCall[] = []
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'unknown-custom-model',
      endpointFormat: 'chat_completions',
      nonStreaming: true,
      fetchImpl: fakeFetch(calls)
    })

    await drain(client.stream(request('unknown-custom-model')))

    const body = jsonString(calls[0].body)
    expect(body).not.toContain('image_url')
    expect(body).not.toContain(SHOT)
  })

  it('uses Vision Bridge observations for text-only screenshot tool results', async () => {
    const calls: CapturedCall[] = []
    const fetchImpl = (async (url: string, init: { body: string }) => {
      const body = JSON.parse(init.body) as Record<string, unknown>
      calls.push({ url: String(url), body })
      if (body.model === 'mimo-v2.5') {
        return new Response(JSON.stringify({
          choices: [{
            index: 0,
            finish_reason: 'stop',
            message: {
              content: JSON.stringify({
                screen_summary: 'Settings window',
                visible_text: ['OK'],
                interactive_elements: [{
                  label: 'OK',
                  kind: 'button',
                  element_index: 1,
                  bbox: [10, 10, 40, 24],
                  state: null,
                  confidence: 0.9
                }],
                spatial_notes: ['OK is near the top left'],
                recommended_next_action: 'click OK',
                uncertainties: [],
                confidence: 0.9
              })
            }
          }]
        }), { status: 200, headers: { 'content-type': 'application/json' } })
      }
      return new Response(JSON.stringify({
        choices: [{ index: 0, finish_reason: 'stop', message: { content: 'ok' } }]
      }), { status: 200, headers: { 'content-type': 'application/json' } })
    }) as unknown as typeof fetch
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'text-model',
      endpointFormat: 'chat_completions',
      nonStreaming: true,
      fetchImpl,
      modelCapabilities: caps(false),
      visionBridge: {
        enabled: true,
        mode: 'auto',
        providerId: 'xiaomi',
        baseUrl: 'https://api.xiaomimimo.com/v1',
        apiKey: 'sk-vision',
        endpointFormat: 'chat_completions',
        model: 'mimo-v2.5',
        maxImageDimension: 1280,
        maxImageBytes: 1500000,
        maxScreenshotsPerTurn: 4,
        observationCacheTtlMs: 120000,
        injectPolicy: 'observation_text',
        fallbackWhenPrimaryImageUnsupported: true,
        semanticProbeStatus: 'supported'
      }
    })

    await drain(client.stream(request('text-model')))

    expect(calls).toHaveLength(2)
    expect(calls[0].url).toMatch(/xiaomimimo/)
    const primaryBody = jsonString(calls[1].body)
    expect(primaryBody).not.toContain(SHOT)
    expect(primaryBody).not.toContain('image_url')
    expect(primaryBody).toContain('vision_bridge')
    expect(primaryBody).toContain('Settings window')
    expect(primaryBody).toContain('OK')
  })

  it('does not use Vision Bridge or send images when bridge probe is not supported', async () => {
    const calls: CapturedCall[] = []
    const client = new CompatModelClient({
      baseUrl: 'https://api.example.com/v1',
      apiKey: 'sk',
      model: 'text-model',
      endpointFormat: 'chat_completions',
      nonStreaming: true,
      fetchImpl: fakeFetch(calls),
      modelCapabilities: caps(false),
      visionBridge: {
        enabled: true,
        mode: 'auto',
        providerId: 'xiaomi',
        baseUrl: 'https://api.xiaomimimo.com/v1',
        apiKey: 'sk-vision',
        endpointFormat: 'chat_completions',
        model: 'mimo-v2.5',
        maxImageDimension: 1280,
        maxImageBytes: 1500000,
        maxScreenshotsPerTurn: 4,
        observationCacheTtlMs: 120000,
        injectPolicy: 'observation_text',
        fallbackWhenPrimaryImageUnsupported: true,
        semanticProbeStatus: 'semantic_failed'
      }
    })

    await drain(client.stream(request('text-model')))

    expect(calls).toHaveLength(1)
    const body = jsonString(calls[0].body)
    expect(body).not.toContain(SHOT)
    expect(body).not.toContain('image_url')
    expect(body).not.toContain('vision_bridge')
  })

  it('bridges screenshots to a DeepSeek text-only primary model without sending image bytes', async () => {
    const calls: CapturedCall[] = []
    const fetchImpl = (async (url: string, init: { body: string }) => {
      const body = JSON.parse(init.body) as Record<string, unknown>
      calls.push({ url: String(url), body })
      if (body.model === 'mimo-v2.5') {
        return new Response(JSON.stringify({
          choices: [{
            index: 0,
            finish_reason: 'stop',
            message: {
              content: JSON.stringify({
                screen_summary: 'Finder window with one OK button',
                visible_text: ['OK'],
                interactive_elements: [{
                  label: 'OK',
                  kind: 'button',
                  element_index: 2,
                  bbox: [10, 20, 80, 44],
                  state: null,
                  confidence: 0.95
                }],
                spatial_notes: ['OK is near the top-left corner'],
                recommended_next_action: 'click element_index 2',
                uncertainties: [],
                confidence: 0.92
              })
            }
          }]
        }), { status: 200, headers: { 'content-type': 'application/json' } })
      }
      return new Response(JSON.stringify({
        choices: [{
          index: 0,
          finish_reason: 'tool_calls',
          message: {
            content: null,
            tool_calls: [{
              id: 'call_next',
              type: 'function',
              function: {
                name: 'computer_use',
                arguments: JSON.stringify({ action: 'left_click', element_index: 2 })
              }
            }]
          }
        }]
      }), { status: 200, headers: { 'content-type': 'application/json' } })
    }) as unknown as typeof fetch
    const client = new CompatModelClient({
      baseUrl: 'https://api.deepseek.com/v1',
      apiKey: 'sk-deepseek',
      model: 'deepseek-v4-flash',
      endpointFormat: 'chat_completions',
      nonStreaming: true,
      fetchImpl,
      modelCapabilities: caps(false),
      visionBridge: {
        enabled: true,
        mode: 'auto',
        providerId: 'xiaomi',
        baseUrl: 'https://api.xiaomimimo.com/v1',
        apiKey: 'sk-vision',
        endpointFormat: 'chat_completions',
        model: 'mimo-v2.5',
        maxImageDimension: 1280,
        maxImageBytes: 1500000,
        maxScreenshotsPerTurn: 4,
        observationCacheTtlMs: 120000,
        injectPolicy: 'observation_text',
        fallbackWhenPrimaryImageUnsupported: true,
        semanticProbeStatus: 'supported'
      }
    })
    const chunks = await collect(client.stream({
      ...request('deepseek-v4-flash'),
      tools: [{
        name: 'computer_use',
        description: 'Control the computer',
        inputSchema: {
          type: 'object',
          properties: {
            action: { type: 'string' },
            element_index: { type: 'number' }
          },
          required: ['action']
        }
      }]
    }))

    expect(calls).toHaveLength(2)
    const primaryBody = jsonString(calls[1].body)
    expect(primaryBody).not.toContain(SHOT)
    expect(primaryBody).not.toContain('image_url')
    expect(primaryBody).toContain('vision_bridge')
    expect(primaryBody).toContain('Finder window')
    expect(chunks).toContainEqual({
      kind: 'tool_call_complete',
      callId: 'call_next',
      toolName: 'computer_use',
      arguments: { action: 'left_click', element_index: 2 }
    })
  })
})
