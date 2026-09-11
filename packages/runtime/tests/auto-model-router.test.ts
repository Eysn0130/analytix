import { describe, expect, it } from 'vitest'
import {
  autoModelHeuristic,
  AUTO_MODEL_ROUTER_FINGERPRINT,
  AUTO_MODEL_ROUTER_MODEL,
  AUTO_MODEL_ROUTER_SYSTEM_PROMPT,
  AUTO_MODEL_ROUTER_TIMEOUT_MS,
  buildAutoModelRouterFingerprint,
  parseAutoRouteRecommendation,
  recentAutoRouterContext,
  resolveAutoModelRoute
} from '../src/shared/auto-model-router.js'
import { makeAssistantTextItem, makeToolResultItem, makeUserItem } from '../src/domain/item.js'
import type { ModelClient, ModelRequest, ModelStreamChunk } from '../src/ports/model-client.js'

describe('auto model router', () => {
  it('fingerprints the classifier contract for route-cache rebuilds', () => {
    expect(buildAutoModelRouterFingerprint()).toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ routerModel: 'deepseek-v4-flash' }))
      .toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ routerModel: 'deepseek-v4-pro' }))
      .not.toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ systemPrompt: 'different classifier prompt' }))
      .not.toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ timeoutMs: 8_000 }))
      .not.toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ responseFormat: undefined }))
      .toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ maxTokens: 128 }))
      .not.toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ temperature: 0.1 }))
      .not.toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
    expect(buildAutoModelRouterFingerprint({ reasoningEffort: 'high' }))
      .not.toBe(AUTO_MODEL_ROUTER_FINGERPRINT)
  })

  it('parses trusted router model recommendations', () => {
    expect(parseAutoRouteRecommendation('{"model":"pro","thinking":"max"}')).toEqual({
      model: 'deepseek-v4-pro',
      reasoningEffort: 'max'
    })
    expect(parseAutoRouteRecommendation('noise {"model":"v4-flash"} tail')).toEqual({
      model: 'deepseek-v4-flash'
    })
    expect(parseAutoRouteRecommendation('{"model":"auto"}')).toBeNull()
    expect(parseAutoRouteRecommendation('not json')).toBeNull()
  })

  it('falls back to the DeepSeek TUI heuristic shape', () => {
    expect(autoModelHeuristic('hello')).toBe('deepseek-v4-flash')
    expect(autoModelHeuristic('please debug this failing migration')).toBe('deepseek-v4-pro')
    expect(autoModelHeuristic('x'.repeat(501))).toBe('deepseek-v4-pro')
    expect(autoModelHeuristic('x'.repeat(200))).toBe('deepseek-v4-flash')
  })

  it('builds recent context without the active turn', () => {
    const items = [
      makeUserItem({ id: 'u1', threadId: 'thr_1', turnId: 'turn_1', text: 'hello' }),
      makeAssistantTextItem({ id: 'a1', threadId: 'thr_1', turnId: 'turn_1', text: 'hi', status: 'completed' }),
      makeToolResultItem({
        id: 'r1',
        threadId: 'thr_1',
        turnId: 'turn_2',
        callId: 'call_1',
        toolName: 'read',
        output: 'file content'
      }),
      makeUserItem({ id: 'u2', threadId: 'thr_1', turnId: 'turn_3', text: 'latest' })
    ]

    expect(recentAutoRouterContext(items, 'turn_3')).toContain('user: hello')
    expect(recentAutoRouterContext(items, 'turn_3')).toContain('assistant: hi')
    expect(recentAutoRouterContext(items, 'turn_3'))
      .toContain('tool: [tool result: read] status=completed disclosure=metadata_only')
    expect(recentAutoRouterContext(items, 'turn_3')).not.toContain('file content')
    expect(recentAutoRouterContext(items, 'turn_3')).not.toContain('latest')
  })

  it('uses an isolated short JSON classifier request without prompt/tool carryover', async () => {
    const seenRequests: ModelRequest[] = []
    const modelClient: ModelClient = {
      provider: 'fake',
      model: 'fake',
      async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
        seenRequests.push(request)
        yield { kind: 'assistant_text_delta', text: '{"model":"deepseek-v4-flash","thinking":"off"}' }
        yield { kind: 'completed', stopReason: 'stop' }
      }
    }

    await resolveAutoModelRoute({
      modelClient,
      threadId: 'thr_1',
      turnId: 'turn_1',
      latestRequest: 'hello',
      recentContext: 'user: previous context',
      selectedModelMode: 'auto',
      abortSignal: new AbortController().signal
    })

    const capturedRequest = seenRequests[0]
    expect(capturedRequest).toBeDefined()
    if (!capturedRequest) throw new Error('classifier request was not captured')
    expect(capturedRequest).toMatchObject({
      threadId: 'thr_1',
      turnId: 'turn_1_auto_router',
      model: AUTO_MODEL_ROUTER_MODEL,
      systemPrompt: AUTO_MODEL_ROUTER_SYSTEM_PROMPT,
      prefix: [],
      tools: [],
      stream: false,
      maxTokens: 96,
      temperature: 0,
      responseFormat: 'json_object',
      reasoningEffort: 'off'
    })
    expect(capturedRequest.history).toHaveLength(1)
    const historyItem = capturedRequest.history[0]
    expect(historyItem).toMatchObject({
      kind: 'user_message',
      id: 'item_turn_1_auto_router_user',
      threadId: 'thr_1',
      turnId: 'turn_1_auto_router'
    })
    if (historyItem?.kind !== 'user_message') throw new Error('classifier history item was not a user message')
    expect(historyItem.text).toContain('Selected model mode: auto')
    expect(historyItem.text).toContain('Recent context:\nuser: previous context')
    expect(historyItem.text).toContain('Latest user request:\nhello')
    expect(capturedRequest.contextInstructions).toBeUndefined()
  })

  it('falls back to the heuristic when the classifier request times out', async () => {
    const seenRequests: ModelRequest[] = []
    const modelClient: ModelClient = {
      provider: 'slow-router',
      model: 'fake',
      stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
        seenRequests.push(request)
        return {
          [Symbol.asyncIterator]() {
            return {
              async next(): Promise<IteratorResult<ModelStreamChunk>> {
                await new Promise((_resolve, reject) => {
                  request.abortSignal.addEventListener('abort', () => reject(new Error('router timed out')), { once: true })
                })
                return { done: true, value: undefined }
              }
            }
          }
        }
      }
    }

    const route = await resolveAutoModelRoute({
      modelClient,
      threadId: 'thr_1',
      turnId: 'turn_2',
      latestRequest: 'please debug the failing release',
      recentContext: 'No prior context.',
      selectedModelMode: 'auto',
      abortSignal: new AbortController().signal,
      timeoutMs: 1
    })

    expect(route).toEqual({
      model: 'deepseek-v4-pro',
      reasoningEffort: 'max',
      source: 'heuristic'
    })
    const capturedRequest = seenRequests[0]
    expect(capturedRequest?.turnId).toBe('turn_2_auto_router')
    expect(capturedRequest?.abortSignal.aborted).toBe(true)
    expect(buildAutoModelRouterFingerprint({ timeoutMs: 1 }))
      .not.toBe(buildAutoModelRouterFingerprint({ timeoutMs: AUTO_MODEL_ROUTER_TIMEOUT_MS }))
  })
})
