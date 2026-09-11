import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { CompatModelClient } from '../src/model-test-support/model/compat-model-client.js'
import {
  captureCachePrefixShape,
  compareCachePrefixShapes
} from '../src/cache/prefix-cache-diagnostics.js'
import { evaluateOfflineCacheCurveGuard } from '../src/cache/offline-cache-curve-guard.js'
import { makeUserItem } from '../src/domain/item.js'
import { ProviderCacheContract } from '../src/conformance/runtime-parity-fixtures.js'
import type { UsageSnapshot } from '../src/contracts/usage.js'
import type { ModelEndpointFormat } from '../src/contracts/model-endpoint-format.js'
import type { ModelRequest, ModelStreamChunk, ModelToolSpec } from '../src/ports/model-client.js'

function buildRequest(abortSignal: AbortSignal): ModelRequest {
  return {
    threadId: 'thr_cache_contract',
    turnId: 'turn_cache_contract',
    model: 'deepseek-v4-pro',
    systemPrompt: 'Stable cache contract system.',
    modeInstruction: 'Agent mode stays byte-stable.',
    prefix: [],
    history: [],
    tools: [],
    abortSignal
  }
}

async function collectUsage(
  client: CompatModelClient,
  configureRequest: (request: ModelRequest) => void = () => {}
): Promise<UsageSnapshot> {
  const chunks: ModelStreamChunk[] = []
  const request = buildRequest(new AbortController().signal)
  configureRequest(request)
  for await (const chunk of client.stream(request)) {
    chunks.push(chunk)
  }
  const usage = chunks.find((chunk) => chunk.kind === 'usage')
  if (!usage || usage.kind !== 'usage') throw new Error('fixture did not emit usage')
  return usage.usage
}

function jsonResponse(payload: Record<string, unknown>): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: { 'content-type': 'application/json' }
  })
}

function requestShapeResponse(testCase: ProviderCacheContract['requestShapeCases'][number]): Record<string, unknown> {
  if (testCase.expectedToolShape === 'responses-function') {
    return {
      id: `resp_${testCase.id}`,
      status: 'completed',
      output_text: 'ok',
      usage: { input_tokens: 4, output_tokens: 2, total_tokens: 6 }
    }
  }
  if (testCase.expectedToolShape === 'anthropic-input-schema') {
    return {
      id: `msg_${testCase.id}`,
      type: 'message',
      role: 'assistant',
      content: [{ type: 'text', text: 'ok' }],
      stop_reason: 'end_turn',
      usage: { input_tokens: 4, output_tokens: 2 }
    }
  }
  return {
    id: `chat_${testCase.id}`,
    model: testCase.model,
    choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }],
    usage: { prompt_tokens: 4, completion_tokens: 2, total_tokens: 6 }
  }
}

function expectToolShape(body: Record<string, unknown>, shape: ProviderCacheContract['requestShapeCases'][number]['expectedToolShape']): void {
  const tools = body.tools as Array<Record<string, unknown>> | undefined
  expect(Array.isArray(tools)).toBe(true)
  expect(tools?.[0]).toBeDefined()
  if (!tools?.[0]) return
  if (shape === 'openai-function') {
    expect(tools[0]).toMatchObject({
      type: 'function',
      function: {
        name: 'alpha',
        parameters: expect.objectContaining({ type: 'object' })
      }
    })
    return
  }
  if (shape === 'responses-function') {
    expect(tools[0]).toMatchObject({
      type: 'function',
      name: 'alpha',
      parameters: expect.objectContaining({ type: 'object' })
    })
    return
  }
  expect(tools[0]).toMatchObject({
    name: 'alpha',
    input_schema: expect.objectContaining({ type: 'object' })
  })
}

async function withLocalJsonProvider(
  handler: (request: {
    url: string
    method: string
    headers: Record<string, string | string[] | undefined>
    body: string
  }) => Record<string, unknown>
): Promise<{
  baseUrl: string
  close: () => Promise<void>
}> {
  const server = createServer(async (req, res) => {
    const chunks: Buffer[] = []
    for await (const chunk of req) chunks.push(Buffer.from(chunk))
    const body = Buffer.concat(chunks).toString('utf8')
    const payload = handler({
      url: req.url ?? '',
      method: req.method ?? '',
      headers: req.headers,
      body
    })
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify(payload))
  })
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      server.off('error', reject)
      resolve()
    })
  })
  const address = server.address() as AddressInfo
  return {
    baseUrl: `http://127.0.0.1:${address.port}`,
    close: () => new Promise<void>((resolve, reject) => {
      server.close((error) => error ? reject(error) : resolve())
    })
  }
}

const contractUrl = new URL('../src/conformance/fixtures/provider-cache-contract.json', import.meta.url)

function loadProviderCacheContract(): ProviderCacheContract {
  return ProviderCacheContract.parse(JSON.parse(readFileSync(contractUrl, 'utf8')))
}

function usageFromFixture(input: ProviderCacheContract['stablePrefix']['usage']): UsageSnapshot {
  const promptTokens = input.promptTokens ?? 0
  const completionTokens = input.completionTokens ?? 0
  return {
    promptTokens,
    completionTokens,
    totalTokens: input.totalTokens ?? promptTokens + completionTokens,
    cacheHitRate: input.cacheHitRate ?? null,
    ...(input.cacheHitTokens !== undefined ? { cacheHitTokens: input.cacheHitTokens } : {}),
    ...(input.cacheMissTokens !== undefined ? { cacheMissTokens: input.cacheMissTokens } : {}),
    turns: 1
  }
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function numberValue(record: Record<string, unknown>, field: string): number {
  const value = record[field]
  return typeof value === 'number' ? value : 0
}

function rawUsageFromProviderPayload(
  testCase: ProviderCacheContract['providerUsageCases'][number]
): UsageSnapshot {
  const usage = recordValue(testCase.responseBody.usage)
  const endpointFormat = testCase.endpointFormat ?? 'chat_completions'
  if (endpointFormat === 'responses') {
    const inputTokens = numberValue(usage, 'input_tokens')
    const outputTokens = numberValue(usage, 'output_tokens')
    const cachedTokens = numberValue(recordValue(usage.input_tokens_details), 'cached_tokens')
    const reasoningTokens = numberValue(recordValue(usage.output_tokens_details), 'reasoning_tokens')
    return {
      promptTokens: inputTokens,
      completionTokens: outputTokens,
      totalTokens: numberValue(usage, 'total_tokens'),
      ...(reasoningTokens > 0 ? { reasoningTokens } : {}),
      cacheHitTokens: cachedTokens,
      cacheMissTokens: inputTokens - cachedTokens,
      cacheHitRate: cachedTokens / inputTokens,
      turns: 1
    }
  }
  if (endpointFormat === 'messages') {
    const inputTokens = numberValue(usage, 'input_tokens')
    const outputTokens = numberValue(usage, 'output_tokens')
    const cacheReadTokens = numberValue(usage, 'cache_read_input_tokens')
    const cacheCreationTokens = numberValue(usage, 'cache_creation_input_tokens')
    const promptTokens = inputTokens + cacheReadTokens + cacheCreationTokens
    const cacheMissTokens = inputTokens + cacheCreationTokens
    return {
      promptTokens,
      completionTokens: outputTokens,
      totalTokens: promptTokens + outputTokens,
      cacheHitTokens: cacheReadTokens,
      cacheMissTokens,
      cacheHitRate: cacheReadTokens / (cacheReadTokens + cacheMissTokens),
      turns: 1
    }
  }

  const promptTokens = numberValue(usage, 'prompt_tokens')
  const completionTokens = numberValue(usage, 'completion_tokens')
  const promptDetails = recordValue(usage.prompt_tokens_details)
  const completionDetails = recordValue(usage.completion_tokens_details)
  const nativeHitTokens = numberValue(usage, 'prompt_cache_hit_tokens')
  const nativeMissTokens = numberValue(usage, 'prompt_cache_miss_tokens')
  const cachedTokens = numberValue(promptDetails, 'cached_tokens')
  const reasoningTokens = numberValue(completionDetails, 'reasoning_tokens')
  const parsed: UsageSnapshot = {
    promptTokens,
    completionTokens,
    totalTokens: numberValue(usage, 'total_tokens'),
    ...(reasoningTokens > 0 ? { reasoningTokens } : {}),
    cacheHitRate: null,
    turns: 1
  }
  if (nativeHitTokens > 0 || nativeMissTokens > 0) {
    parsed.cacheHitTokens = nativeHitTokens
    parsed.cacheMissTokens = nativeMissTokens
    parsed.cacheHitRate = nativeHitTokens / (nativeHitTokens + nativeMissTokens)
  } else if (cachedTokens > 0) {
    parsed.cacheHitTokens = cachedTokens
    parsed.cacheMissTokens = promptTokens - cachedTokens
    parsed.cacheHitRate = cachedTokens / promptTokens
  }
  return parsed
}

function usageNumberMatches(
  actual: number | null | undefined,
  expected: number | null | undefined
): boolean {
  if (typeof actual === 'number' && typeof expected === 'number') {
    return Math.abs(actual - expected) < 1e-12
  }
  return actual === expected
}

function usageMatchesExpected(
  usage: UsageSnapshot,
  expected: ProviderCacheContract['providerUsageCases'][number]['expectedUsage']
): boolean {
  const usageRecord = usage as unknown as Record<string, unknown>
  return usageNumberMatches(usage.promptTokens, expected.promptTokens) &&
    usageNumberMatches(usage.completionTokens, expected.completionTokens) &&
    usageNumberMatches(usage.reasoningTokens, expected.reasoningTokens) &&
    usageNumberMatches(usage.totalTokens, expected.totalTokens) &&
    usageNumberMatches(usage.cacheHitTokens, expected.cacheHitTokens) &&
    usageNumberMatches(usage.cacheMissTokens, expected.cacheMissTokens) &&
    usageNumberMatches(usage.cacheHitRate, expected.cacheHitRate) &&
    (expected.absent ?? []).every((field) => usageRecord[field] === undefined)
}

function rawProviderCacheAccounting(contract: ProviderCacheContract) {
  const parsedCases = contract.providerUsageCases.map((item) => ({
    id: item.id,
    baseUrl: item.baseUrl,
    endpointFormat: item.endpointFormat,
    usage: rawUsageFromProviderPayload(item)
  }))
  const supported = parsedCases
    .filter((item) => item.usage.cacheHitTokens !== undefined && item.usage.cacheMissTokens !== undefined)
  const totalCacheHitTokens = supported.reduce((sum, item) => sum + (item.usage.cacheHitTokens ?? 0), 0)
  const totalCacheMissTokens = supported.reduce((sum, item) => sum + (item.usage.cacheMissTokens ?? 0), 0)
  return {
    rawPayloadParsedCaseIds: parsedCases.map((item) => item.id),
    rawTelemetrySupportedCaseIds: supported.map((item) => item.id),
    rawMatchesExpectedUsageCaseIds: contract.providerUsageCases
      .filter((item) => usageMatchesExpected(rawUsageFromProviderPayload(item), item.expectedUsage))
      .map((item) => item.id),
    unsupportedUnknownCaseIds: parsedCases
      .filter((item) => item.usage.cacheHitRate === null)
      .map((item) => item.id),
    deepseekCaseIds: supported
      .filter((item) => item.baseUrl.includes('deepseek'))
      .map((item) => item.id),
    openaiCacheCaseIds: supported
      .filter((item) => item.endpointFormat === 'responses')
      .map((item) => item.id),
    anthropicCacheCaseIds: supported
      .filter((item) => item.endpointFormat === 'messages')
      .map((item) => item.id),
    totalCacheHitTokens,
    totalCacheMissTokens,
    aggregateCacheHitRate: totalCacheHitTokens / (totalCacheHitTokens + totalCacheMissTokens)
  }
}

function expectUsageMatches(
  usage: UsageSnapshot,
  expected: ProviderCacheContract['providerUsageCases'][number]['expectedUsage']
): void {
  if (expected.promptTokens !== undefined) expect(usage.promptTokens).toBe(expected.promptTokens)
  if (expected.completionTokens !== undefined) {
    expect(usage.completionTokens).toBe(expected.completionTokens)
  }
  if (expected.reasoningTokens !== undefined) {
    expect(usage.reasoningTokens).toBe(expected.reasoningTokens)
  }
  if (expected.totalTokens !== undefined) expect(usage.totalTokens).toBe(expected.totalTokens)
  if (expected.cacheHitTokens !== undefined) expect(usage.cacheHitTokens).toBe(expected.cacheHitTokens)
  if (expected.cacheMissTokens !== undefined) {
    expect(usage.cacheMissTokens).toBe(expected.cacheMissTokens)
  }
  if ('cacheHitRate' in expected) {
    if (expected.cacheHitRate === null) {
      expect(usage.cacheHitRate).toBeNull()
    } else if (typeof expected.cacheHitRate === 'number') {
      expect(usage.cacheHitRate).toBeCloseTo(expected.cacheHitRate)
    }
  }
  for (const field of expected.absent ?? []) {
    expect((usage as unknown as Record<string, unknown>)[field]).toBeUndefined()
  }
}

function normalizedPathname(url: string): string {
  const path = new URL(url).pathname.replace(/\/$/, '')
  return path === '' ? '' : path
}

function endpointPathFromBase(baseUrl: string, endpointPath: string): string {
  const basePath = normalizedPathname(baseUrl)
  if (basePath.endsWith('/v1') && endpointPath.startsWith('/v1/')) {
    return `${basePath}${endpointPath.slice('/v1'.length)}`
  }
  return `${basePath}${endpointPath}`
}

function expectedUsageRequestPath(testCase: ProviderCacheContract['providerUsageCases'][number]): string {
  const endpointFormat = testCase.endpointFormat ?? 'chat_completions'
  if (endpointFormat === 'responses') return endpointPathFromBase(testCase.baseUrl, '/v1/responses')
  if (endpointFormat === 'messages') return endpointPathFromBase(testCase.baseUrl, '/v1/messages')
  return endpointPathFromBase(testCase.baseUrl, '/v1/chat/completions')
}

function uniqueInOrder(values: string[]): string[] {
  return Array.from(new Set(values))
}

type ProviderCacheCoverageCase =
  | ProviderCacheContract['providerUsageCases'][number]
  | ProviderCacheContract['requestShapeCases'][number]

function providerFamilyForCase(testCase: ProviderCacheCoverageCase): string {
  const endpointFormat = testCase.endpointFormat ?? 'chat_completions'
  if (endpointFormat === 'custom_endpoint') return 'custom-full-endpoint'
  if (testCase.baseUrl.includes('deepseek')) return 'deepseek'
  if (endpointFormat === 'responses') return 'openai-responses'
  if (endpointFormat === 'messages') return 'anthropic-messages'
  return 'openai-compatible'
}

function hasHeader(
  headers: Record<string, string | string[] | undefined>,
  name: string
): boolean {
  return headers[name.toLowerCase()] !== undefined
}

function fetchInputUrl(input: Parameters<typeof fetch>[0]): string {
  return input instanceof Request ? input.url : String(input)
}

function forwardToLocalProvider(providerBaseUrl: string): typeof fetch {
  return async (input, init) => {
    const original = new URL(fetchInputUrl(input))
    return fetch(`${providerBaseUrl}${original.pathname}${original.search}`, init)
  }
}

const toolsA: ModelToolSpec[] = [
  {
    name: 'zeta',
    description: 'Search files without exposing file content.',
    inputSchema: {
      type: 'object',
      properties: { b: { type: 'string' }, a: { type: 'number' } },
      required: ['a']
    }
  },
  {
    name: 'alpha',
    description: 'Read metadata only.',
    inputSchema: { type: 'object' }
  }
]

const toolsB: ModelToolSpec[] = [
  {
    name: 'alpha',
    description: 'Read metadata only.',
    inputSchema: { type: 'object' }
  },
  {
    name: 'zeta',
    description: 'Search files without exposing file content.',
    inputSchema: {
      required: ['a'],
      properties: { a: { type: 'number' }, b: { type: 'string' } },
      type: 'object'
    }
  }
]

describe('Reasonix P2.2 provider/cache contract fixtures', () => {
  it('matches the provider/cache conformance contract without live credentials', async () => {
    const contract = loadProviderCacheContract()
    expect(contract.liveCredentialPolicy).toEqual({
      fixtureOnly: true,
      mayClaimSuperiority: false
    })

    const first = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      modeInstruction: 'Agent mode stays byte-stable.',
      prefix: [
        makeUserItem({
          id: 'volatile_id_a',
          turnId: 'volatile_turn_a',
          threadId: 'volatile_thread_a',
          text: 'same cache-visible prefix text'
        })
      ],
      tools: toolsA,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })
    const equivalent = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      modeInstruction: 'Agent mode stays byte-stable.',
      prefix: [
        makeUserItem({
          id: 'volatile_id_b',
          turnId: 'volatile_turn_b',
          threadId: 'volatile_thread_b',
          text: 'same cache-visible prefix text'
        })
      ],
      tools: toolsB,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })
    expect(first).toEqual(contract.stablePrefix.firstShape)
    expect(equivalent).toEqual(contract.stablePrefix.equivalentShape)

    const stableDiagnostics = compareCachePrefixShapes(
      first,
      equivalent,
      usageFromFixture(contract.stablePrefix.usage)
    )
    expect(stableDiagnostics).toMatchObject(contract.stablePrefix.expectedDiagnostics)

    const previous = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      tools: toolsA,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })
    const current = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      tools: toolsA.slice(0, 1),
      provider: 'compat',
      providerId: 'openai-compatible',
      endpointFormat: 'responses',
      model: 'gpt-5-mini'
    })
    expect(previous).toEqual(contract.driftAttribution.previousShape)
    expect(current).toEqual(contract.driftAttribution.currentShape)
    const driftDiagnostics = compareCachePrefixShapes(
      previous,
      current,
      usageFromFixture(contract.driftAttribution.usage)
    )
    expect(driftDiagnostics.prefixChangeReasons).toEqual(contract.driftAttribution.expectedReasons)
    expect(driftDiagnostics.cacheTelemetrySupported).toBe(
      contract.driftAttribution.expectedTelemetrySupported
    )

    for (const forbidden of contract.privacy.forbiddenDiagnosticsSubstrings) {
      expect(JSON.stringify(stableDiagnostics)).not.toContain(forbidden)
      expect(JSON.stringify(driftDiagnostics)).not.toContain(forbidden)
    }

    for (const testCase of contract.providerUsageCases) {
      const usage = await collectUsage(new CompatModelClient({
        baseUrl: testCase.baseUrl,
        apiKey: 'fixture-key',
        model: testCase.model,
        ...(testCase.endpointFormat
          ? { endpointFormat: testCase.endpointFormat as ModelEndpointFormat }
          : {}),
        nonStreaming: true,
        fetchImpl: async () => jsonResponse(testCase.responseBody)
      }))
      expectUsageMatches(usage, testCase.expectedUsage)
    }
  })

  it('keeps the provider/cache contract coverage floor explicit', () => {
    const contract = loadProviderCacheContract()

    expect(contract.providerUsageCases.map((item) => item.id)).toEqual([
      'unsupported-openai-compatible',
      'deepseek-prompt-cache',
      'deepseek-native-cache-precedence',
      'openai-responses-cached-tokens',
      'anthropic-cache-fields'
    ])
    expect(contract.requestShapeCases.map((item) => item.id)).toEqual([
      'deepseek-chat-request-shape',
      'openai-compatible-chat-request-shape',
      'responses-request-shape',
      'anthropic-messages-request-shape',
      'custom-responses-full-endpoint-request-shape',
      'custom-messages-full-endpoint-request-shape',
      'custom-chat-full-endpoint-request-shape'
    ])

    const coveredProviderFamilies = uniqueInOrder([
      ...contract.requestShapeCases,
      ...contract.providerUsageCases
    ].map(providerFamilyForCase))
    expect(coveredProviderFamilies).toEqual(contract.liveLocalHttpContract.coveredProviderFamilies)
    expect(coveredProviderFamilies).toEqual([
      'deepseek',
      'openai-compatible',
      'openai-responses',
      'anthropic-messages',
      'custom-full-endpoint'
    ])
    expect(uniqueInOrder(contract.requestShapeCases.map((item) => item.endpointFormat))).toEqual([
      'chat_completions',
      'responses',
      'messages',
      'custom_endpoint'
    ])

    expect(contract.providerUsageCases
      .filter((item) => (
        item.expectedUsage.cacheHitTokens !== undefined &&
        item.expectedUsage.cacheMissTokens !== undefined
      ))
      .map((item) => item.id))
      .toEqual([
        'deepseek-prompt-cache',
        'deepseek-native-cache-precedence',
        'openai-responses-cached-tokens',
        'anthropic-cache-fields'
      ])
    expect(contract.providerUsageCases
      .filter((item) => item.expectedUsage.cacheHitRate === null)
      .map((item) => item.id))
      .toEqual(['unsupported-openai-compatible'])
    expect(contract.requestShapeCases
      .filter((item) => item.endpointFormat === 'custom_endpoint')
      .map((item) => item.id))
      .toEqual([
        'custom-responses-full-endpoint-request-shape',
        'custom-messages-full-endpoint-request-shape',
        'custom-chat-full-endpoint-request-shape'
      ])
    const customFullEndpointIds = contract.requestShapeCases
      .filter((item) => item.endpointFormat === 'custom_endpoint')
      .map((item) => item.id)
    const telemetrySupportedIds = contract.providerUsageCases
      .filter((item) => (
        item.expectedUsage.cacheHitTokens !== undefined &&
        item.expectedUsage.cacheMissTokens !== undefined
      ))
      .map((item) => item.id)
    expect(telemetrySupportedIds.filter((id) => customFullEndpointIds.includes(id))).toEqual([])
    expect(contract.providerUsageCases
      .filter((item) => item.endpointFormat === 'custom_endpoint')
      .map((item) => item.id))
      .toEqual([])
  })

  it('derives provider cache accounting directly from raw response payloads', () => {
    const contract = loadProviderCacheContract()

    for (const testCase of contract.providerUsageCases) {
      expectUsageMatches(rawUsageFromProviderPayload(testCase), testCase.expectedUsage)
    }

    const accounting = rawProviderCacheAccounting(contract)
    expect(accounting.rawPayloadParsedCaseIds).toEqual([
      'unsupported-openai-compatible',
      'deepseek-prompt-cache',
      'deepseek-native-cache-precedence',
      'openai-responses-cached-tokens',
      'anthropic-cache-fields'
    ])
    expect(accounting.rawTelemetrySupportedCaseIds).toEqual([
      'deepseek-prompt-cache',
      'deepseek-native-cache-precedence',
      'openai-responses-cached-tokens',
      'anthropic-cache-fields'
    ])
    expect(accounting.rawMatchesExpectedUsageCaseIds).toEqual(accounting.rawPayloadParsedCaseIds)
    expect(accounting.unsupportedUnknownCaseIds).toEqual(['unsupported-openai-compatible'])
    expect(accounting.deepseekCaseIds).toEqual([
      'deepseek-prompt-cache',
      'deepseek-native-cache-precedence'
    ])
    expect(accounting.openaiCacheCaseIds).toEqual(['openai-responses-cached-tokens'])
    expect(accounting.anthropicCacheCaseIds).toEqual(['anthropic-cache-fields'])
    expect(accounting.totalCacheHitTokens).toBe(2930)
    expect(accounting.totalCacheMissTokens).toBe(720)
    expect(accounting.aggregateCacheHitRate).toBeCloseTo(0.8027397260273973)
  })

  it('keeps the stable prefix and canonical tool hash across equivalent turns', () => {
    const first = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      modeInstruction: 'Agent mode stays byte-stable.',
      prefix: [
        makeUserItem({
          id: 'volatile_id_a',
          turnId: 'volatile_turn_a',
          threadId: 'volatile_thread_a',
          text: 'same cache-visible prefix text'
        })
      ],
      tools: toolsA,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })
    const second = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      modeInstruction: 'Agent mode stays byte-stable.',
      prefix: [
        makeUserItem({
          id: 'volatile_id_b',
          turnId: 'volatile_turn_b',
          threadId: 'volatile_thread_b',
          text: 'same cache-visible prefix text'
        })
      ],
      tools: toolsB,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })

    expect(second.prefixHash).toBe(first.prefixHash)
    expect(second.toolsHash).toBe(first.toolsHash)
    expect(second.prefixItemsHash).toBe(first.prefixItemsHash)

    const diagnostics = compareCachePrefixShapes(first, second, {
      promptTokens: 100,
      completionTokens: 8,
      totalTokens: 108,
      cachedTokens: 80,
      cacheHitTokens: 80,
      cacheMissTokens: 20,
      cacheHitRate: 0.8,
      turns: 1
    })

    expect(diagnostics).toMatchObject({
      prefixChanged: false,
      prefixChangeReasons: [],
      cacheTelemetrySupported: true,
      cacheHitTokens: 80,
      cacheMissTokens: 20,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })
    expect(JSON.stringify(diagnostics)).not.toContain('Stable cache contract system.')
    expect(JSON.stringify(diagnostics)).not.toContain('Search files without exposing file content.')
  })

  it('attributes provider, model, endpoint, and tool drift without raw prompt text', () => {
    const previous = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      tools: toolsA,
      provider: 'compat',
      providerId: 'deepseek-default',
      endpointFormat: 'chat_completions',
      model: 'deepseek-v4-pro'
    })
    const current = captureCachePrefixShape({
      systemPrompt: 'Stable cache contract system.',
      tools: toolsA.slice(0, 1),
      provider: 'compat',
      providerId: 'openai-compatible',
      endpointFormat: 'responses',
      model: 'gpt-5-mini'
    })

    const diagnostics = compareCachePrefixShapes(previous, current, {
      promptTokens: 120,
      completionTokens: 12,
      totalTokens: 132,
      cacheHitRate: null,
      turns: 1
    })

    expect(diagnostics.prefixChanged).toBe(true)
    expect(diagnostics.prefixChangeReasons).toEqual(['tools', 'provider', 'model'])
    expect(diagnostics.cacheTelemetrySupported).toBe(false)
    expect(diagnostics.cacheHitTokens).toBeUndefined()
    expect(diagnostics.cacheMissTokens).toBeUndefined()
    expect(JSON.stringify(diagnostics)).not.toContain('Stable cache contract system.')
  })

  it('records live-local HTTP contract scope as derived contract metadata', () => {
    const contract = loadProviderCacheContract()
    const liveLocalHttpContract = contract.liveLocalHttpContract
    expect(liveLocalHttpContract.fixtureOnly).toBe(true)
    expect(liveLocalHttpContract.transport).toBe('local-http')
    expect(liveLocalHttpContract.usesLiveCredentials).toBe(false)
    expect(liveLocalHttpContract.preservesOriginalProviderBaseUrl).toBe(true)
    expect(liveLocalHttpContract.usageCaseCount).toBe(contract.providerUsageCases.length)
    expect(liveLocalHttpContract.requestShapeCaseCount).toBe(contract.requestShapeCases.length)
    expect(liveLocalHttpContract.expectedPostCount)
      .toBe(contract.providerUsageCases.length + contract.requestShapeCases.length)
    expect(liveLocalHttpContract.coveredEndpointFormats)
      .toEqual(uniqueInOrder(contract.requestShapeCases.map((item) => item.endpointFormat)))
    expect(liveLocalHttpContract.coveredProviderFamilies).toEqual([
      'deepseek',
      'openai-compatible',
      'openai-responses',
      'anthropic-messages',
      'custom-full-endpoint'
    ])
    expect(liveLocalHttpContract.mayClaimLiveSuperiority).toBe(false)
  })

  it('parses provider cache telemetry and leaves unsupported providers unknown', async () => {
    const unsupported = await collectUsage(new CompatModelClient({
      baseUrl: 'https://openai-compatible.example/v1',
      apiKey: 'k',
      model: 'gpt-5-mini',
      nonStreaming: true,
      fetchImpl: async () => jsonResponse({
        id: 'openai_no_cache',
        model: 'gpt-5-mini',
        choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }],
        usage: { prompt_tokens: 100, completion_tokens: 10, total_tokens: 110 }
      })
    }))
    expect(unsupported).toMatchObject({
      promptTokens: 100,
      completionTokens: 10,
      totalTokens: 110,
      cacheHitRate: null
    })
    expect(unsupported.cacheHitTokens).toBeUndefined()
    expect(unsupported.cacheMissTokens).toBeUndefined()

    const deepseek = await collectUsage(new CompatModelClient({
      baseUrl: 'https://api.deepseek.com',
      apiKey: 'k',
      model: 'deepseek-v4-pro',
      nonStreaming: true,
      fetchImpl: async () => jsonResponse({
        id: 'deepseek_cache',
        model: 'deepseek-v4-pro',
        choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }],
        usage: {
          prompt_tokens: 1000,
          completion_tokens: 10,
          total_tokens: 1010,
          prompt_cache_hit_tokens: 930,
          prompt_cache_miss_tokens: 70
        }
      })
    }))
    expect(deepseek.cacheHitTokens).toBe(930)
    expect(deepseek.cacheMissTokens).toBe(70)
    expect(deepseek.cacheHitRate).toBeCloseTo(0.93)

    const responses = await collectUsage(new CompatModelClient({
      baseUrl: 'https://openai-compatible.example/v1',
      apiKey: 'k',
      model: 'gpt-5-mini',
      endpointFormat: 'responses',
      nonStreaming: true,
      fetchImpl: async () => jsonResponse({
        id: 'responses_cache',
        status: 'completed',
        output_text: 'ok',
        usage: {
          input_tokens: 400,
          output_tokens: 20,
          total_tokens: 420,
          input_tokens_details: { cached_tokens: 300 }
        }
      })
    }))
    expect(responses.cacheHitTokens).toBe(300)
    expect(responses.cacheMissTokens).toBe(100)
    expect(responses.cacheHitRate).toBeCloseTo(0.75)

    const anthropic = await collectUsage(new CompatModelClient({
      baseUrl: 'https://api.minimaxi.com/anthropic',
      apiKey: 'k',
      model: 'MiniMax-M2.5',
      endpointFormat: 'messages',
      nonStreaming: true,
      fetchImpl: async () => jsonResponse({
        id: 'anthropic_cache',
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
      })
    }))
    expect(anthropic.promptTokens).toBe(1250)
    expect(anthropic.cacheHitTokens).toBe(1000)
    expect(anthropic.cacheMissTokens).toBe(250)
    expect(anthropic.cacheHitRate).toBeCloseTo(0.8)
  })

  it('pins provider request URL, header, and body shapes in the cache contract', async () => {
    const contract = loadProviderCacheContract()
    for (const testCase of contract.requestShapeCases) {
      const sent: Array<{
        url: string
        headers: Headers
        body: Record<string, unknown>
      }> = []
      const fetchImpl: typeof fetch = async (url, init) => {
        sent.push({
          url: String(url),
          headers: new Headers(init?.headers),
          body: JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>
        })
        return jsonResponse(requestShapeResponse(testCase))
      }
      const client = new CompatModelClient({
        baseUrl: testCase.baseUrl,
        apiKey: 'shape-key',
        model: testCase.model,
        endpointFormat: testCase.endpointFormat as ModelEndpointFormat,
        fetchImpl,
        nonStreaming: true
      })
      const request = buildRequest(new AbortController().signal)
      request.model = testCase.model
      request.maxTokens = 128
      request.tools = toolsA
      if (testCase.reasoningEffort) request.reasoningEffort = testCase.reasoningEffort

      for await (const _chunk of client.stream(request)) {
        // drain
      }

      expect(sent, testCase.id).toHaveLength(1)
      const actual = sent[0]
      expect(actual.url, testCase.id).toBe(testCase.expectedUrl)
      for (const header of testCase.requiredHeaders) {
        expect(actual.headers.has(header), `${testCase.id} required header ${header}`).toBe(true)
      }
      for (const header of testCase.forbiddenHeaders) {
        expect(actual.headers.has(header), `${testCase.id} forbidden header ${header}`).toBe(false)
      }
      for (const field of testCase.requiredBodyFields) {
        expect(Object.prototype.hasOwnProperty.call(actual.body, field), `${testCase.id} required body field ${field}`)
          .toBe(true)
      }
      for (const field of testCase.forbiddenBodyFields) {
        expect(Object.prototype.hasOwnProperty.call(actual.body, field), `${testCase.id} forbidden body field ${field}`)
          .toBe(false)
      }
      expect(actual.body.model, testCase.id).toBe(testCase.model)
      expectToolShape(actual.body, testCase.expectedToolShape)
    }
  })

  it('executes every provider usage contract case against a live-local HTTP provider', async () => {
    const contract = loadProviderCacheContract()
    for (const testCase of contract.providerUsageCases) {
      const requests: Array<{ url: string; method: string; body: Record<string, unknown> }> = []
      const provider = await withLocalJsonProvider((request) => {
        requests.push({
          url: request.url,
          method: request.method,
          body: JSON.parse(request.body) as Record<string, unknown>
        })
        return testCase.responseBody
      })
      try {
        const usage = await collectUsage(new CompatModelClient({
          baseUrl: testCase.baseUrl,
          apiKey: 'live-local-key',
          model: testCase.model,
          ...(testCase.endpointFormat
            ? { endpointFormat: testCase.endpointFormat as ModelEndpointFormat }
            : {}),
          nonStreaming: true,
          fetchImpl: forwardToLocalProvider(provider.baseUrl)
        }), (request) => {
          request.model = testCase.model
        })

        expect(requests, testCase.id).toHaveLength(1)
        expect(requests[0].method, testCase.id).toBe('POST')
        expect(requests[0].url, testCase.id).toBe(expectedUsageRequestPath(testCase))
        expect(requests[0].body.model, testCase.id).toBe(testCase.model)
        expectUsageMatches(usage, testCase.expectedUsage)
      } finally {
        await provider.close()
      }
    }
  })

  it('executes every provider request-shape contract case against a live-local HTTP provider', async () => {
    const contract = loadProviderCacheContract()
    for (const testCase of contract.requestShapeCases) {
      const requests: Array<{
        url: string
        method: string
        headers: Record<string, string | string[] | undefined>
        body: Record<string, unknown>
      }> = []
      const provider = await withLocalJsonProvider((request) => {
        requests.push({
          url: request.url,
          method: request.method,
          headers: request.headers,
          body: JSON.parse(request.body) as Record<string, unknown>
        })
        return requestShapeResponse(testCase)
      })
      try {
        const client = new CompatModelClient({
          baseUrl: testCase.baseUrl,
          apiKey: 'shape-key',
          model: testCase.model,
          endpointFormat: testCase.endpointFormat as ModelEndpointFormat,
          nonStreaming: true,
          fetchImpl: forwardToLocalProvider(provider.baseUrl)
        })
        const request = buildRequest(new AbortController().signal)
        request.model = testCase.model
        request.maxTokens = 128
        request.tools = toolsA
        if (testCase.reasoningEffort) request.reasoningEffort = testCase.reasoningEffort

        for await (const _chunk of client.stream(request)) {
          // drain
        }

        expect(requests, testCase.id).toHaveLength(1)
        const actual = requests[0]
        expect(actual.method, testCase.id).toBe('POST')
        expect(actual.url, testCase.id).toBe(new URL(testCase.expectedUrl).pathname)
        for (const header of testCase.requiredHeaders) {
          expect(hasHeader(actual.headers, header), `${testCase.id} required header ${header}`)
            .toBe(true)
        }
        for (const header of testCase.forbiddenHeaders) {
          expect(hasHeader(actual.headers, header), `${testCase.id} forbidden header ${header}`)
            .toBe(false)
        }
        for (const field of testCase.requiredBodyFields) {
          expect(Object.prototype.hasOwnProperty.call(actual.body, field), `${testCase.id} required body field ${field}`)
            .toBe(true)
        }
        for (const field of testCase.forbiddenBodyFields) {
          expect(Object.prototype.hasOwnProperty.call(actual.body, field), `${testCase.id} forbidden body field ${field}`)
            .toBe(false)
        }
        expect(actual.body.model, testCase.id).toBe(testCase.model)
        expectToolShape(actual.body, testCase.expectedToolShape)
      } finally {
        await provider.close()
      }
    }
  })

  it('proves DeepSeek cache telemetry against an executable live-local provider', async () => {
    const requests: Array<{ url: string; method: string; body: Record<string, unknown> }> = []
    const provider = await withLocalJsonProvider((request) => {
      requests.push({
        url: request.url,
        method: request.method,
        body: JSON.parse(request.body) as Record<string, unknown>
      })
      return {
        id: 'deepseek_live_local_cache',
        model: 'deepseek-v4-pro',
        choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'ok' } }],
        usage: {
          prompt_tokens: 1000,
          completion_tokens: 10,
          total_tokens: 1010,
          prompt_cache_hit_tokens: 930,
          prompt_cache_miss_tokens: 70,
          prompt_tokens_details: { cached_tokens: 999 },
          completion_tokens_details: { reasoning_tokens: 9 }
        }
      }
    })
    try {
      const usage = await collectUsage(new CompatModelClient({
        baseUrl: provider.baseUrl,
        apiKey: 'live-local-key',
        model: 'deepseek-v4-pro',
        nonStreaming: true
      }))

      expect(requests).toHaveLength(1)
      expect(requests[0]).toMatchObject({
        method: 'POST',
        url: '/v1/chat/completions',
        body: {
          model: 'deepseek-v4-pro'
        }
      })
      expect(usage).toMatchObject({
        promptTokens: 1000,
        completionTokens: 10,
        reasoningTokens: 9,
        totalTokens: 1010,
        cacheHitTokens: 930,
        cacheMissTokens: 70
      })
      expect(usage.cacheHitRate).toBeCloseTo(0.93)
    } finally {
      await provider.close()
    }
  })

  it('enforces an offline Reasonix-style cache curve guard without provider credentials', () => {
    const contract = loadProviderCacheContract()
    expect(contract.releaseGuard.fixtureOnly).toBe(true)
    const guard = evaluateOfflineCacheCurveGuard(contract.releaseGuard)
    const byId = new Map(guard.cases.map((testCase) => [testCase.id, testCase]))

    for (const testCase of contract.releaseGuard.cases) {
      const actual = byId.get(testCase.id)
      expect(actual?.tailAveragePercent, testCase.id)
        .toBeCloseTo(testCase.expectedTailAveragePercent)
      expect(actual?.status, testCase.id).toBe(testCase.expectedStatus)

      if (testCase.maxAllowedCollapses !== undefined) {
        expect(actual?.collapseCount, testCase.id)
          .toBeLessThanOrEqual(testCase.maxAllowedCollapses)
      }
      if (testCase.compactionGuardPaused) {
        expect(actual?.compactionGuardPaused, testCase.id).toBe(true)
        expect(actual?.status, testCase.id).toBe('fail')
      }
    }
    expect(contract.releaseGuard.maxLowTailCases).toBe(0)
    expect(guard.lowTailCases).toBe(1)
    expect(guard.status).toBe('fail')
    expect(guard.fixtureOnly).toBe(true)

    const toleranceAttempt = evaluateOfflineCacheCurveGuard({
      ...contract.releaseGuard,
      maxLowTailCases: 1,
      cases: contract.releaseGuard.cases.filter((testCase) => testCase.expectedStatus === 'pass')
    })
    expect(toleranceAttempt.lowTailCases).toBe(0)
    expect(toleranceAttempt.status).toBe('fail')
  })
})
