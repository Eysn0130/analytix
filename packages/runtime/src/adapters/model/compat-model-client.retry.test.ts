import { describe, expect, it } from 'vitest'
import { CompatModelClient } from '../../model-test-support/model/compat-model-client.js'
import type { ModelRequest, ModelStreamChunk } from '../../ports/model-client.js'

function request(signal?: AbortSignal): ModelRequest {
  return {
    threadId: 't1',
    turnId: 'u1',
    model: 'glm-5.1',
    systemPrompt: 'You are a helpful assistant.',
    prefix: [],
    history: [],
    tools: [],
    abortSignal: signal ?? new AbortController().signal
  }
}

async function drain(iterable: AsyncIterable<ModelStreamChunk>): Promise<ModelStreamChunk[]> {
  const chunks: ModelStreamChunk[] = []
  for await (const chunk of iterable) chunks.push(chunk)
  return chunks
}

function okJson(): Response {
  return new Response(
    JSON.stringify({ choices: [{ index: 0, finish_reason: 'stop', message: { content: 'ok' } }] }),
    { status: 200, headers: { 'content-type': 'application/json' } }
  )
}

function gatewayError(status: number): Response {
  return new Response(
    `<html><head><title>${status}</title></head><body><center>${status}</center><center>alb</center></body></html>`,
    { status, headers: { 'content-type': 'text/html' } }
  )
}

function client(fetchImpl: typeof fetch): CompatModelClient {
  return new CompatModelClient({
    baseUrl: 'https://provider.example/v1',
    apiKey: 'sk-test',
    model: 'glm-5.1',
    endpointFormat: 'chat_completions',
    nonStreaming: true,
    fetchImpl
  })
}

describe('CompatModelClient transient gateway retry', () => {
  it('retries a 502 Bad Gateway and then succeeds', async () => {
    let calls = 0
    const fetchImpl = (async () => {
      calls += 1
      return calls === 1 ? gatewayError(502) : okJson()
    }) as unknown as typeof fetch

    const chunks = await drain(client(fetchImpl).stream(request()))

    expect(calls).toBe(2)
    expect(chunks.some((chunk) => chunk.kind === 'assistant_text_delta')).toBe(true)
    expect(chunks.at(-1)).toEqual({ kind: 'completed', stopReason: 'stop' })
    expect(chunks.some((chunk) => chunk.kind === 'error')).toBe(false)
  })

  it('does not retry a non-transient 500', async () => {
    let calls = 0
    const fetchImpl = (async () => {
      calls += 1
      return gatewayError(500)
    }) as unknown as typeof fetch

    const chunks = await drain(client(fetchImpl).stream(request()))

    expect(calls).toBe(1)
    expect(chunks.some((chunk) => chunk.kind === 'error')).toBe(true)
  })

  it('stops retrying when the request is aborted during backoff', async () => {
    const controller = new AbortController()
    let calls = 0
    const fetchImpl = (async () => {
      calls += 1
      controller.abort()
      return gatewayError(503)
    }) as unknown as typeof fetch

    const chunks = await drain(client(fetchImpl).stream(request(controller.signal)))

    expect(calls).toBe(1)
    expect(chunks.some((chunk) => chunk.kind === 'error')).toBe(true)
  })
})
