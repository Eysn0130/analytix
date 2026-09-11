import { request as httpRequest } from 'node:http'
import { request as httpsRequest } from 'node:https'
import { Readable } from 'node:stream'
import { ProxyAgent } from 'proxy-agent'
import type {
  ModelClient,
  ModelHistoryItem,
  ModelRequest,
  ModelStreamChunk,
  ModelToolSpec
} from '../../ports/model-client.js'
import { emptyUsageSnapshot, type UsageSnapshot } from '../../contracts/usage.js'
import type { ModelCapabilityMetadata } from '../../contracts/capabilities.js'
import type { LlmDebugRound, LlmDebugSink } from '../../services-test-support/llm-debug-recorder.js'
import {
  INTERRUPTED_TOOL_RESULT_PLACEHOLDER,
  isToolResultBridgeItem,
  repairModelHistoryItems
} from '../../domain/model-history-repair.js'
import { extractToolResultImages, toolResultTextWithoutImages } from '../../shared/tool-result-image.js'
import { canonicalizeToolInputSchema } from '../../cache/tool-catalog-fingerprint.js'
import {
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  isCustomModelEndpointFormat,
  modelEndpointPath,
  normalizeModelEndpointFormat,
  resolveModelEndpointFormat,
  usesChatCompletionsShape,
  type ModelEndpointFormat
} from '../../contracts/model-endpoint-format.js'
import { VisionBridgeService } from '../../shared/vision-bridge.js'
import type { VisionBridgeCapabilityConfig } from '../../contracts/capabilities.js'

/**
 * Configuration for the compatible HTTP model client. Chat
 * completions remains the default, while custom providers can opt into
 * OpenAI Responses or Anthropic Messages request/response shapes.
 */
export type CompatModelClientConfig = {
  baseUrl: string
  apiKey: string
  model: string
  /** Compatible request/response protocol to use for custom providers. */
  endpointFormat?: ModelEndpointFormat
  /** Optional extra headers, e.g. project or session ids. */
  headers?: Record<string, string>
  /** HTTP fetch implementation. Defaults to global `fetch`. */
  fetchImpl?: typeof fetch
  /** Optional proxy URL used only for model HTTP requests. */
  modelProxyUrl?: string
  /** Maximum number of messages to send. Defaults to the entire history. */
  historyLimit?: number
  /** When true, the client requests a non-streaming response. */
  nonStreaming?: boolean
  /** Maximum idle time between streaming chunks before the turn fails. */
  streamIdleTimeoutMs?: number
  /** Optional model capability resolver used for provider-specific reasoning translation. */
  modelCapabilities?: (model: string) => ModelCapabilityMetadata
  /** Optional bridge that converts tool-result screenshots into text observations for text-only models. */
  visionBridge?: VisionBridgeCapabilityConfig
  /** Optional troubleshooting sink that captures each request body + raw output. */
  debugSink?: LlmDebugSink
}

export type DeepseekCurrencyCosts = {
  costUsd: number
  costCny: number
}

type DeepseekPrice = {
  inputCacheHit: number
  inputCacheMiss: number
  output: number
}

type DeepseekPriceSet = {
  usd: DeepseekPrice
  cny: DeepseekPrice
}

export type MiniMaxCurrencyCosts = {
  costUsd?: number
  costCny: number
}

type MiniMaxPrice = {
  input: number
  output: number
  cacheRead: number
  cacheWrite?: number
}

export type DeepSeekProbeResult = {
  reachable: boolean
  status?: number
  message: string
}

export type ToolArgumentRepairResult = {
  arguments: Record<string, unknown>
  repaired: boolean
}

type ChatMessage = {
  role: 'system' | 'user' | 'assistant' | 'tool'
  content: string | ChatMessageContentPart[] | null
  name?: string
  tool_call_id?: string
  reasoning_content?: string
  reasoning_signature?: string
  tool_calls?: {
    id: string
    type: 'function'
    function: { name: string; arguments: string }
  }[]
}

type ChatMessageContentPart =
  | { type: 'text'; text: string }
  | { type: 'image_url'; image_url: { url: string } }

type AnthropicCacheControl = { type: 'ephemeral' }

type AnthropicContentBlock = (
  | { type: 'text'; text: string }
  | { type: 'image'; source: { type: 'base64'; media_type: string; data: string } | { type: 'url'; url: string } }
  | { type: 'thinking'; thinking: string; signature?: string }
  | { type: 'tool_use'; id: string; name: string; input: Record<string, unknown> }
  | { type: 'tool_result'; tool_use_id: string; content: string }
) & { cache_control?: AnthropicCacheControl }

type AnthropicImageSource = Extract<AnthropicContentBlock, { type: 'image' }>['source']

type AnthropicMessage = {
  role: 'user' | 'assistant'
  content: string | AnthropicContentBlock[]
}

type ChatCompletionResponse = {
  id: string
  model: string
  choices: {
    index: number
    finish_reason: string
    message: ChatMessage & {
      tool_calls?: {
        id: string
        type: 'function'
        function: { name: string; arguments: string }
      }[]
    }
  }[]
  usage?: {
    prompt_tokens?: number
    completion_tokens?: number
    total_tokens?: number
    prompt_eval_count?: number
    eval_count?: number
    prompt_cache_hit_tokens?: number
    prompt_cache_miss_tokens?: number
    prompt_tokens_details?: { cached_tokens?: number }
    cache_creation_input_tokens?: number
    cache_read_input_tokens?: number
  }
}

type ResponsesApiResponse = {
  id?: string
  status?: string
  output_text?: string
  output?: Array<Record<string, unknown>>
  usage?: Record<string, unknown>
  error?: { message?: string; type?: string } | null
  incomplete_details?: { reason?: string } | null
}

type AnthropicMessageResponse = {
  id?: string
  type?: string
  role?: string
  content?: Array<Record<string, unknown>>
  stop_reason?: string | null
  usage?: Record<string, unknown>
}

type ModelStopReason = Extract<ModelStreamChunk, { kind: 'completed' }>['stopReason']
type PendingToolCall = {
  index?: number
  name?: string
  arguments: string
}
type StreamReadResult =
  | { kind: 'chunk'; value?: Uint8Array; done: boolean }
  | { kind: 'timeout' }
  | { kind: 'aborted' }
  | { kind: 'error'; message: string }

const DEFAULT_STREAM_IDLE_TIMEOUT_MS = 45_000
const DEFAULT_MESSAGES_MAX_TOKENS = 4096
const THINK_TAG_OPEN = '<think>'
const THINK_TAG_CLOSE = '</think>'
const TOKENS_PER_MILLION = 1_000_000
const MINIMAX_M3_LONG_CONTEXT_THRESHOLD = 512_000
const MID_TURN_STEERING_PREFIX =
  'Mid-turn user follow-up for the current task. Treat this as additional guidance for the active turn, not a new independent task.'

// Official DeepSeek API prices per 1M tokens. As of 2026-06-02,
// deepseek-chat/deepseek-reasoner are aliases for v4-flash modes.
const DEEPSEEK_V4_PRICES: Record<'flash' | 'pro', DeepseekPriceSet> = {
  flash: {
    usd: {
      inputCacheHit: 0.0028,
      inputCacheMiss: 0.14,
      output: 0.28
    },
    cny: {
      inputCacheHit: 0.02,
      inputCacheMiss: 1,
      output: 2
    }
  },
  pro: {
    usd: {
      inputCacheHit: 0.003625,
      inputCacheMiss: 0.435,
      output: 0.87
    },
    cny: {
      inputCacheHit: 0.025,
      inputCacheMiss: 3,
      output: 6
    }
  }
}

// Official MiniMax pay-as-you-go language model prices, CNY per 1M tokens.
// Token Plan credits are deducted at the matching pay-as-you-go list price.
const MINIMAX_TEXT_PRICES: Record<string, MiniMaxPrice> = {
  'minimax-m2.7': {
    input: 2.1,
    output: 8.4,
    cacheRead: 0.42,
    cacheWrite: 2.625
  },
  'minimax-m2.7-highspeed': {
    input: 4.2,
    output: 16.8,
    cacheRead: 0.42,
    cacheWrite: 2.625
  },
  'minimax-m2.5': {
    input: 2.1,
    output: 8.4,
    cacheRead: 0.21,
    cacheWrite: 2.625
  },
  'minimax-m2.5-highspeed': {
    input: 4.2,
    output: 16.8,
    cacheRead: 0.21,
    cacheWrite: 2.625
  },
  'minimax-m2.1': {
    input: 2.1,
    output: 8.4,
    cacheRead: 0.21,
    cacheWrite: 2.625
  },
  'minimax-m2.1-highspeed': {
    input: 4.2,
    output: 16.8,
    cacheRead: 0.21,
    cacheWrite: 2.625
  },
  'minimax-m2': {
    input: 2.1,
    output: 8.4,
    cacheRead: 0.21,
    cacheWrite: 2.625
  }
}

const MINIMAX_M3_STANDARD_PRICE: MiniMaxPrice = {
  input: 2.1,
  output: 8.4,
  cacheRead: 0.42
}

const MINIMAX_M3_LONG_CONTEXT_PRICE: MiniMaxPrice = {
  input: 4.2,
  output: 16.8,
  cacheRead: 0.84
}

export function isDeepSeekHost(baseUrl: string): boolean {
  try {
    const host = new URL(baseUrl).hostname.toLowerCase()
    return host === 'api.deepseek.com' || host.endsWith('.deepseek.com')
  } catch {
    return false
  }
}

export async function probeDeepSeekReachable(input: {
  baseUrl: string
  fetchImpl: typeof fetch
}): Promise<DeepSeekProbeResult> {
  const url = probeUrl(input.baseUrl)
  try {
    const response = await input.fetchImpl(url, {
      method: 'GET',
      headers: { Accept: 'application/json, text/plain, */*' }
    })
    return {
      reachable: response.status < 500,
      status: response.status,
      message: response.status < 500
        ? `DeepSeek endpoint is reachable (probe status ${response.status}).`
        : `DeepSeek endpoint probe also returned ${response.status}.`
    }
  } catch (error) {
    return {
      reachable: false,
      message: `DeepSeek endpoint probe failed: ${error instanceof Error ? error.message : String(error)}`
    }
  }
}

function probeUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim().replace(/\/+$/, '')
  if (!trimmed) return 'https://api.deepseek.com/v1/models'
  try {
    const url = new URL(trimmed)
    const parts = url.pathname.split('/').filter(Boolean)
    if (parts.at(-1)?.toLowerCase() === 'beta' || /^v\d+$/i.test(parts.at(-1) ?? '')) {
      parts.pop()
    }
    url.pathname = `/${[...parts, 'v1', 'models'].join('/')}`
    url.search = ''
    return url.toString()
  } catch {
    return 'https://api.deepseek.com/v1/models'
  }
}

function deepseekPricingTierForModel(model: string): keyof typeof DEEPSEEK_V4_PRICES | null {
  const normalized = model.trim().toLowerCase()
  if (!normalized) return null
  if (normalized === 'deepseek-v4-pro' || normalized.endsWith('/deepseek-v4-pro')) return 'pro'
  if (
    normalized === 'deepseek-v4-flash' ||
    normalized === 'deepseek-chat' ||
    normalized === 'deepseek-reasoner' ||
    normalized.endsWith('/deepseek-v4-flash') ||
    normalized.endsWith('/deepseek-chat') ||
    normalized.endsWith('/deepseek-reasoner')
  ) {
    return 'flash'
  }
  return null
}

function computeDeepseekCost(
  price: DeepseekPrice,
  cacheHitTokens: number,
  cacheMissTokens: number,
  outputTokens: number
): number {
  return (
    (cacheHitTokens / TOKENS_PER_MILLION) * price.inputCacheHit +
    (cacheMissTokens / TOKENS_PER_MILLION) * price.inputCacheMiss +
    (outputTokens / TOKENS_PER_MILLION) * price.output
  )
}

export function estimateDeepseekCost(input: {
  model: string
  cacheHitTokens: number
  cacheMissTokens: number
  outputTokens: number
  providerHost?: string
}): DeepseekCurrencyCosts | null {
  if (input.providerHost !== undefined && !isDeepSeekHost(input.providerHost)) {
    return null
  }
  const tier = deepseekPricingTierForModel(input.model)
  if (!tier) return null
  const prices = DEEPSEEK_V4_PRICES[tier]
  return {
    costUsd: computeDeepseekCost(prices.usd, input.cacheHitTokens, input.cacheMissTokens, input.outputTokens),
    costCny: computeDeepseekCost(prices.cny, input.cacheHitTokens, input.cacheMissTokens, input.outputTokens)
  }
}

function isMiniMaxHost(baseUrl: string): boolean {
  try {
    const host = new URL(baseUrl).hostname.toLowerCase()
    return host === 'api.minimaxi.com' || host === 'api.minimax.io' || host === 'api.minimax.chat'
  } catch {
    return false
  }
}

function normalizePricingModel(model: string): string {
  const normalized = model.trim().toLowerCase()
  const parts = normalized.split('/').filter(Boolean)
  return parts.at(-1) ?? normalized
}

function minimaxPriceForModel(model: string, billableInputTokens: number): MiniMaxPrice | null {
  const normalized = normalizePricingModel(model)
  if (normalized === 'minimax-m3') {
    return billableInputTokens > MINIMAX_M3_LONG_CONTEXT_THRESHOLD
      ? MINIMAX_M3_LONG_CONTEXT_PRICE
      : MINIMAX_M3_STANDARD_PRICE
  }
  return MINIMAX_TEXT_PRICES[normalized] ?? null
}

function costCnyForMiniMaxPrice(input: {
  price: MiniMaxPrice
  inputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  outputTokens: number
}): number {
  const cacheWritePrice = input.price.cacheWrite ?? input.price.input
  return (
    (input.inputTokens / TOKENS_PER_MILLION) * input.price.input +
    (input.cacheReadTokens / TOKENS_PER_MILLION) * input.price.cacheRead +
    (input.cacheWriteTokens / TOKENS_PER_MILLION) * cacheWritePrice +
    (input.outputTokens / TOKENS_PER_MILLION) * input.price.output
  )
}

export function estimateMiniMaxCost(input: {
  model: string
  providerHost?: string
  inputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  outputTokens: number
}): MiniMaxCurrencyCosts | null {
  if (input.providerHost !== undefined && !isMiniMaxHost(input.providerHost)) {
    return null
  }
  const billableInputTokens = Math.max(
    0,
    input.inputTokens + input.cacheReadTokens + input.cacheWriteTokens
  )
  const price = minimaxPriceForModel(input.model, billableInputTokens)
  if (!price) return null
  return {
    costCny: costCnyForMiniMaxPrice({
      price,
      inputTokens: Math.max(0, input.inputTokens),
      cacheReadTokens: Math.max(0, input.cacheReadTokens),
      cacheWriteTokens: Math.max(0, input.cacheWriteTokens),
      outputTokens: Math.max(0, input.outputTokens)
    })
  }
}

export function repairToolArguments(raw: string): ToolArgumentRepairResult {
  const trimmed = raw.trim()
  if (!trimmed) return { arguments: {}, repaired: false }
  const direct = parseToolArgumentObject(trimmed)
  if (direct) return { arguments: direct, repaired: false }

  const candidates = [
    stripToolArgumentMarkdownFence(trimmed),
    extractFirstToolArgumentJsonObject(trimmed),
    extractFirstToolArgumentJsonArray(trimmed),
    closeTruncatedToolArgumentJsonDocument(stripToolArgumentMarkdownFence(trimmed)),
    closeTruncatedToolArgumentJsonDocument(trimmed)
  ].filter((candidate): candidate is string => Boolean(candidate && candidate.trim()))

  for (const candidate of candidates) {
    const parsed = parseToolArgumentAny(candidate)
    if (parsed.ok) {
      return {
        arguments: toolArgumentValueToArguments(parsed.value),
        repaired: true
      }
    }
  }

  return {
    arguments: { __raw: raw },
    repaired: false
  }
}

function parseToolArgumentObject(text: string): Record<string, unknown> | null {
  const parsed = parseToolArgumentAny(text)
  if (!parsed.ok) return null
  if (parsed.value && typeof parsed.value === 'object' && !Array.isArray(parsed.value)) {
    return parsed.value as Record<string, unknown>
  }
  return null
}

function parseToolArgumentAny(text: string): { ok: true; value: unknown } | { ok: false } {
  try {
    return { ok: true, value: JSON.parse(text) }
  } catch {
    return { ok: false }
  }
}

function toolArgumentValueToArguments(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return { value }
}

function stripToolArgumentMarkdownFence(text: string): string {
  const fence = /^```(?:json|javascript|js)?\s*([\s\S]*?)\s*```$/i.exec(text)
  return fence?.[1]?.trim() ?? text
}

function extractFirstToolArgumentJsonObject(text: string): string | null {
  return extractToolArgumentBalanced(text, '{', '}')
}

function extractFirstToolArgumentJsonArray(text: string): string | null {
  return extractToolArgumentBalanced(text, '[', ']')
}

function closeTruncatedToolArgumentJsonDocument(text: string): string | null {
  const start = firstToolArgumentJsonStart(text)
  if (start < 0) return null
  let out = text.slice(start).trimEnd()
  const stack: string[] = []
  let inString = false
  let escaped = false
  for (let index = 0; index < out.length; index += 1) {
    const char = out[index]
    if (inString) {
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === '"') inString = false
      continue
    }
    if (char === '"') inString = true
    else if (char === '{') stack.push('}')
    else if (char === '[') stack.push(']')
    else if ((char === '}' || char === ']') && stack[stack.length - 1] === char) stack.pop()
  }
  if (escaped) out = out.slice(0, -1)
  if (inString) out += '"'
  const rightTrimmed = out.trimEnd()
  if (rightTrimmed.endsWith(',')) {
    out = rightTrimmed.slice(0, -1)
  } else if (rightTrimmed.endsWith(':')) {
    out = `${rightTrimmed}null`
  }
  for (let index = stack.length - 1; index >= 0; index -= 1) {
    out += stack[index]
  }
  return parseToolArgumentAny(out).ok ? out : null
}

function firstToolArgumentJsonStart(text: string): number {
  const objectStart = text.indexOf('{')
  const arrayStart = text.indexOf('[')
  if (objectStart < 0) return arrayStart
  if (arrayStart < 0) return objectStart
  return Math.min(objectStart, arrayStart)
}

function extractToolArgumentBalanced(text: string, open: string, close: string): string | null {
  const start = text.indexOf(open)
  if (start < 0) return null
  let depth = 0
  let inString = false
  let escaped = false
  for (let index = start; index < text.length; index += 1) {
    const char = text[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '"') {
      inString = !inString
      continue
    }
    if (inString) continue
    if (char === open) depth += 1
    if (char === close) {
      depth -= 1
      if (depth === 0) return text.slice(start, index + 1)
    }
  }
  return null
}

function createProxyFetch(proxyUrl: string): typeof fetch | null {
  const normalizedProxyUrl = proxyUrl.trim()
  if (!normalizedProxyUrl) return null
  return (input, init) => fetchViaProxy(input, init, normalizedProxyUrl)
}

async function fetchViaProxy(
  input: Parameters<typeof fetch>[0],
  init: Parameters<typeof fetch>[1] | undefined,
  proxyUrl: string
): Promise<Response> {
  const url = new URL(typeof input === 'string' || input instanceof URL ? input.toString() : input.url)
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error(`Unsupported proxied request protocol: ${url.protocol}`)
  }

  const body = await requestBodyToBuffer(init?.body)
  const headers = headersToRecord(init?.headers)
  if (body && !hasHeader(headers, 'content-length')) {
    headers['content-length'] = String(body.byteLength)
  }

  return new Promise<Response>((resolve, reject) => {
    const agent = new ProxyAgent({ getProxyForUrl: () => proxyUrl })
    const request = (url.protocol === 'https:' ? httpsRequest : httpRequest)(
      url,
      {
        method: init?.method ?? 'GET',
        headers,
        agent
      },
      (response) => {
        const responseHeaders = new Headers()
        for (const [key, value] of Object.entries(response.headers)) {
          if (Array.isArray(value)) {
            for (const item of value) responseHeaders.append(key, item)
          } else if (value !== undefined) {
            responseHeaders.set(key, String(value))
          }
        }
        const webBody = Readable.toWeb(response) as ReadableStream<Uint8Array>
        resolve(new Response(webBody, {
          status: response.statusCode ?? 0,
          statusText: response.statusMessage ?? '',
          headers: responseHeaders
        }))
      }
    )

    const signal = init?.signal
    const abort = (): void => {
      request.destroy(new Error('The operation was aborted.'))
    }
    if (signal?.aborted) {
      abort()
      return
    }
    signal?.addEventListener('abort', abort, { once: true })
    request.on('error', reject)
    request.on('close', () => signal?.removeEventListener('abort', abort))
    if (body) request.write(body)
    request.end()
  })
}

async function requestBodyToBuffer(body: RequestInit['body'] | null | undefined): Promise<Buffer | null> {
  if (body === null || body === undefined) return null
  if (typeof body === 'string') return Buffer.from(body)
  if (body instanceof URLSearchParams) return Buffer.from(body.toString())
  if (body instanceof ArrayBuffer) return Buffer.from(body)
  if (ArrayBuffer.isView(body)) {
    return Buffer.from(body.buffer, body.byteOffset, body.byteLength)
  }
  throw new Error('Unsupported proxied request body type.')
}

function headersToRecord(headers: RequestInit['headers'] | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  if (!headers) return out
  const normalized = new Headers(headers)
  normalized.forEach((value, key) => {
    out[key] = value
  })
  return out
}

function hasHeader(headers: Record<string, string>, name: string): boolean {
  const normalized = name.toLowerCase()
  return Object.keys(headers).some((key) => key.toLowerCase() === normalized)
}

type ThinkingTagSplit = {
  reasoning: string
  text: string
}

class LeadingThinkingTagSplitter {
  private state: 'probe' | 'inside' | 'passthrough' = 'probe'
  private buffer = ''

  push(value: string): ThinkingTagSplit {
    if (!value) return emptyThinkingTagSplit()
    if (this.state === 'passthrough') return { reasoning: '', text: value }
    if (this.state === 'inside') return this.scanClose(value)

    this.buffer += value
    const trimmed = trimLeadingAsciiWhitespace(this.buffer)
    if (trimmed.length < THINK_TAG_OPEN.length) {
      if (THINK_TAG_OPEN.startsWith(trimmed)) {
        return emptyThinkingTagSplit()
      }
      return { reasoning: '', text: this.drainPassthrough() }
    }
    if (trimmed.startsWith(THINK_TAG_OPEN)) {
      this.state = 'inside'
      this.buffer = ''
      return this.scanClose(trimmed.slice(THINK_TAG_OPEN.length))
    }
    return { reasoning: '', text: this.drainPassthrough() }
  }

  flush(): ThinkingTagSplit {
    if (!this.buffer) return emptyThinkingTagSplit()
    const out = this.buffer
    this.buffer = ''
    if (this.state === 'inside') {
      return { reasoning: out, text: '' }
    }
    return { reasoning: '', text: out }
  }

  private scanClose(value: string): ThinkingTagSplit {
    this.buffer += value
    const closeIndex = this.buffer.indexOf(THINK_TAG_CLOSE)
    if (closeIndex >= 0) {
      const reasoning = this.buffer.slice(0, closeIndex)
      const text = trimLeadingAsciiWhitespace(this.buffer.slice(closeIndex + THINK_TAG_CLOSE.length))
      this.buffer = ''
      this.state = 'passthrough'
      return { reasoning, text }
    }
    const keep = markerSuffixLength(this.buffer, THINK_TAG_CLOSE)
    const reasoning = this.buffer.slice(0, this.buffer.length - keep)
    this.buffer = this.buffer.slice(this.buffer.length - keep)
    return { reasoning, text: '' }
  }

  private drainPassthrough(): string {
    this.state = 'passthrough'
    const out = this.buffer
    this.buffer = ''
    return out
  }
}

function splitLeadingThinkingTag(value: string): ThinkingTagSplit {
  const splitter = new LeadingThinkingTagSplitter()
  const pushed = splitter.push(value)
  const flushed = splitter.flush()
  return {
    reasoning: pushed.reasoning + flushed.reasoning,
    text: pushed.text + flushed.text
  }
}

function emptyThinkingTagSplit(): ThinkingTagSplit {
  return { reasoning: '', text: '' }
}

function trimLeadingAsciiWhitespace(value: string): string {
  return value.replace(/^[ \t\r\n]+/, '')
}

function markerSuffixLength(value: string, marker: string): number {
  let max = marker.length - 1
  if (max > value.length) max = value.length
  for (let length = max; length > 0; length -= 1) {
    if (marker.startsWith(value.slice(value.length - length))) return length
  }
  return 0
}

/**
 * Multi-provider HTTP model client.
 *
 * Speaks the streaming chat completions shape by default, and can switch
 * to OpenAI Responses or Anthropic Messages request/response shapes per
 * provider via `endpointFormat`. It supports tool calls, cache hit/miss
 * counters (when the provider reports them), and abort-signal
 * cancellation. The client is deliberately small so the rest of the
 * runtime can be built around the `ModelClient` port.
 */
export class CompatModelClient implements ModelClient {
  readonly provider = 'compat'
  readonly model: string

  readonly config: CompatModelClientConfig
  private readonly fetchImpl: typeof fetch
  private readonly visionBridge?: VisionBridgeService

  constructor(config: CompatModelClientConfig) {
    this.config = config
    this.model = config.model
    this.fetchImpl = config.fetchImpl ?? createProxyFetch(config.modelProxyUrl ?? '') ?? fetch
    this.visionBridge = config.visionBridge
      ? new VisionBridgeService(config.visionBridge, { fetchImpl: this.fetchImpl })
      : undefined
  }

  /**
   * Streams the model response for a turn. Each yielded chunk is one
   * of the kinds defined by `ModelStreamChunk`. The stream respects
   * the request's `abortSignal` between chunks.
   */
  /**
   * Public entry point. When a `debugSink` is configured, captures the
   * literal request body and accumulates the raw output for the
   * troubleshooting view; otherwise forwards with zero overhead.
   */
  async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
    const sink = this.config.debugSink
    if (!sink) {
      yield* this.streamInner(request, null)
      return
    }
    const round = sink.start({
      threadId: request.threadId,
      turnId: request.turnId,
      provider: this.provider,
      model: request.model?.trim() || this.config.model
    })
    try {
      for await (const chunk of this.streamInner(request, round)) {
        this.captureChunk(round, chunk)
        yield chunk
      }
    } finally {
      sink.finish(round)
    }
  }

  private async prepareRequestForVisionBridge(request: ModelRequest, model: string): Promise<ModelRequest> {
    if (!this.visionBridge) return request
    return this.visionBridge.transformRequest(request, {
      primarySupportsImages: this.modelSupportsImageInput(model)
    })
  }

  private captureChunk(round: LlmDebugRound, chunk: ModelStreamChunk): void {
    const out = round.output
    switch (chunk.kind) {
      case 'assistant_text_delta':
        out.text += chunk.text
        break
      case 'assistant_reasoning_delta':
        out.reasoning += chunk.text
        break
      case 'tool_call_complete':
        out.toolCalls.push({
          callId: chunk.callId,
          toolName: chunk.toolName,
          arguments: chunk.arguments
        })
        break
      case 'usage':
        out.usage = chunk.usage
        break
      case 'completed':
        out.stopReason = chunk.stopReason
        break
      case 'error':
        out.error = chunk.message
        break
    }
  }

  private async *streamInner(
    request: ModelRequest,
    round: LlmDebugRound | null
  ): AsyncIterable<ModelStreamChunk> {
    if (request.abortSignal.aborted) {
      yield { kind: 'error', message: 'request was aborted before start' }
      return
    }
    const requestModel = request.model?.trim() || this.config.model
    // Resolve the wire format per request model: a single provider (e.g.
    // OpenCode Go) can route some models to chat completions and others to
    // Anthropic Messages. Falls back to the provider/runtime format.
    const configuredEndpointFormat = this.endpointFormatForModel(requestModel)
    const endpointFormat = resolveModelEndpointFormat(configuredEndpointFormat, this.config.baseUrl)
    if (!endpointFormat) {
      yield {
        kind: 'error',
        message: 'custom full endpoint URL must end with /chat/completions, /completions, /responses, or /messages'
      }
      return
    }
    const url = buildModelEndpointUrl(this.config.baseUrl, configuredEndpointFormat)
    const stream = request.stream ?? !this.config.nonStreaming
    const preparedRequest = await this.prepareRequestForVisionBridge(request, requestModel)
    const body = this.buildRequestBody(preparedRequest, stream, { endpointFormat })
    if (round) {
      round.requestBody = body
      round.url = redactUrlForLog(url)
    }
    const headers = this.buildHeaders(stream, endpointFormat)
    let result = await this.postChatCompletion(url, headers, body, request.abortSignal)
    // Retry transient gateway failures (502/503/504) a few times before giving
    // up. No response body has been streamed yet, so re-POSTing the same
    // request is safe; aborts short-circuit the backoff.
    for (let attempt = 0; attempt < MAX_TRANSIENT_RETRIES; attempt += 1) {
      if (result.kind === 'error') break
      if (result.response.ok || !TRANSIENT_RETRY_STATUSES.has(result.response.status)) break
      yield {
        kind: 'retrying',
        attempt: attempt + 1,
        maxAttempt: MAX_TRANSIENT_RETRIES,
        status: result.response.status,
        message: `provider returned ${result.response.status}; retrying request`
      }
      await result.response.body?.cancel().catch(() => {})
      const aborted = await sleepWithAbort(TRANSIENT_RETRY_BASE_MS * 2 ** attempt, request.abortSignal)
      if (aborted || request.abortSignal.aborted) {
        yield { kind: 'error', message: 'request was aborted during retry backoff' }
        return
      }
      result = await this.postChatCompletion(url, headers, body, request.abortSignal)
    }
    if (result.kind === 'error') {
      yield { kind: 'error', message: result.message }
      return
    }
    let response = result.response
    if (!response.ok) {
      const text = await response.text()
      if (usesChatCompletionsShape(endpointFormat) && shouldRetryWithoutStreamUsage(response.status, text, body)) {
        yield {
          kind: 'retrying',
          attempt: 1,
          maxAttempt: 1,
          status: response.status,
          message: 'provider rejected stream usage fields; retrying without stream_options.include_usage'
        }
        const retryBody = this.buildRequestBody(preparedRequest, stream, { endpointFormat, includeStreamUsage: false })
        if (round) round.requestBody = retryBody
        const retry = await this.postChatCompletion(url, headers, retryBody, request.abortSignal)
        if (retry.kind === 'error') {
          yield { kind: 'error', message: retry.message }
          return
        }
        response = retry.response
        if (response.ok) {
          if (this.config.nonStreaming || response.headers.get('content-type')?.includes('application/json')) {
            const json = (await response.json()) as ChatCompletionResponse
            yield* this.materializeNonStreaming(json, endpointFormat, requestModel)
            return
          }
          if (!response.body) {
            yield { kind: 'error', message: 'model response had no body' }
            return
          }
          yield* this.streamSse(response.body, request.abortSignal, endpointFormat, requestModel)
          return
        }
        const retryText = await response.text()
        this.logHttpFailure()
        const retryClassified = await this.classifyHttpError(response.status, retryText)
        yield {
          kind: 'error',
          message: retryClassified.message,
          code: retryClassified.code
        }
        return
      }
      this.logHttpFailure()
      const classified = await this.classifyHttpError(response.status, text)
      yield {
        kind: 'error',
        message: classified.message,
        code: classified.code
      }
      return
    }
    if (this.config.nonStreaming || response.headers.get('content-type')?.includes('application/json')) {
      const json = (await response.json()) as ChatCompletionResponse
      yield* this.materializeNonStreaming(json, endpointFormat, requestModel)
      return
    }
    if (!response.body) {
      yield { kind: 'error', message: 'model response had no body' }
      return
    }
    yield* this.streamSse(response.body, request.abortSignal, endpointFormat, requestModel)
  }

  private endpointFormat(): ModelEndpointFormat {
    return normalizeModelEndpointFormat(this.config.endpointFormat ?? DEFAULT_MODEL_ENDPOINT_FORMAT)
  }

  /**
   * The wire format for a specific model: a per-model override (carried on
   * the model's capability metadata) takes precedence over the
   * provider/runtime format. Lets one provider mix chat completions and
   * Anthropic Messages models (e.g. OpenCode Go's minimax/qwen entries).
   */
  private endpointFormatForModel(model: string): ModelEndpointFormat {
    const perModel = this.config.modelCapabilities?.(model).endpointFormat
    return normalizeModelEndpointFormat(perModel ?? this.config.endpointFormat ?? DEFAULT_MODEL_ENDPOINT_FORMAT)
  }

  private modelReasoningFor(model: string): ModelCapabilityMetadata['reasoning'] | undefined {
    return this.config.modelCapabilities?.(model).reasoning
  }

  private async postChatCompletion(
    url: string,
    headers: Record<string, string>,
    body: Record<string, unknown>,
    signal: AbortSignal
  ): Promise<{ kind: 'response'; response: Response } | { kind: 'error'; message: string }> {
    try {
      const response = await this.fetchImpl(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
        signal
      })
      return { kind: 'response', response }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      return { kind: 'error', message: `model request failed: ${message}` }
    }
  }

  private buildHeaders(stream: boolean, endpointFormat: ModelEndpointFormat): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json'
    }
    // `stream: true` is enough for OpenAI-compatible providers to return SSE.
    // Some Windows Node/Electron paths time out when routing requests with
    // `Accept: text/event-stream`, while the same stream works without it.
    if (!stream) headers.Accept = 'application/json'
    if (this.config.apiKey) {
      if (endpointFormat === 'messages') {
        headers.Authorization = `Bearer ${this.config.apiKey}`
        headers['x-api-key'] = this.config.apiKey
        headers['anthropic-version'] = '2023-06-01'
      } else {
        headers.Authorization = `Bearer ${this.config.apiKey}`
      }
    }
    return { ...headers, ...(this.config.headers ?? {}) }
  }

  private async classifyHttpError(status: number, text: string): Promise<{ message: string; code: string }> {
    const body = text
    if (status === 404) {
      const prefix = body ? `${body} ` : ''
      return {
        message: `model request failed with status 404: ${prefix}Check your model provider configuration, especially Base URL and Endpoint format.`,
        code: 'http_404'
      }
    }
    if (status === 429) {
      return {
        message: `model request was rate limited (HTTP 429): ${body}`,
        code: 'rate_limited'
      }
    }
    if (status >= 500 && isDeepSeekHost(this.config.baseUrl)) {
      const probe = await probeDeepSeekReachable({
        baseUrl: this.config.baseUrl,
        fetchImpl: this.fetchImpl
      })
      return {
        message: `model request failed with DeepSeek HTTP ${status}: ${body} ${probe.message}`,
        code: probe.reachable ? `deepseek_http_${status}` : 'deepseek_unreachable'
      }
    }
    return {
      message: `model request failed with status ${status}: ${body}`,
      code: `http_${status}`
    }
  }

  private logHttpFailure(): void {
    console.warn('[analytix] event=ANALYTIX_MODEL_HTTP_REQUEST_FAILED')
  }

  private buildRequestBody(
    request: ModelRequest,
    stream: boolean,
    options: { endpointFormat?: ModelEndpointFormat; includeStreamUsage?: boolean } = {}
  ): Record<string, unknown> {
    const requestModel = request.model?.trim()
    const model = requestModel || this.config.model
    const messages = this.collectMessages(request, model)
    const endpointFormat = options.endpointFormat ?? this.endpointFormat()
    if (endpointFormat === 'responses') {
      return this.buildResponsesRequestBody(request, model, messages, stream)
    }
    if (endpointFormat === 'messages') {
      return this.buildAnthropicMessagesRequestBody(request, model, messages, stream)
    }
    const body: Record<string, unknown> = {
      model,
      stream,
      messages: splitToolImageMessagesForOpenAi(messages)
    }
    if (request.maxTokens !== undefined) {
      body.max_tokens = request.maxTokens
    }
    if (request.temperature !== undefined) {
      body.temperature = request.temperature
    }
    if (request.topP !== undefined) {
      body.top_p = request.topP
    }
    if (request.responseFormat === 'json_object') {
      body.response_format = { type: 'json_object' }
    }
    if (stream && options.includeStreamUsage !== false) {
      body.stream_options = { include_usage: true }
    }
    const isNativeDeepSeek = isDeepSeekHost(this.config.baseUrl)
    const includeThinking = !isAzureOpenAiEndpoint(this.config.baseUrl)
    applyReasoningEffort(body, request.reasoningEffort, {
      includeThinking,
      nativeDeepSeekHost: isNativeDeepSeek,
      reasoning: this.modelReasoningFor(model),
      maxReasoningEffort: isNativeDeepSeek ? 'max' : 'high'
    })
    if (
      includeThinking &&
      isDeepSeekHost(this.config.baseUrl) &&
      !Object.prototype.hasOwnProperty.call(body, 'thinking') &&
      isThinkingProducerModel(model)
    ) {
      body.thinking = { type: 'enabled' }
    }
    const tools = normalizeToolSpecs(request.tools)
    if (tools.length > 0) {
      body.tools = tools.map((tool) => ({
        type: 'function',
        function: {
          name: tool.name,
          description: tool.description,
          parameters: tool.inputSchema
        }
      }))
    }
    return body
  }

  private buildResponsesRequestBody(
    request: ModelRequest,
    model: string,
    messages: ChatMessage[],
    stream: boolean
  ): Record<string, unknown> {
    const body: Record<string, unknown> = {
      model,
      stream,
      input: messagesToResponsesInput(splitToolImageMessagesForOpenAi(messages))
    }
    if (request.maxTokens !== undefined) {
      body.max_output_tokens = request.maxTokens
    }
    if (request.temperature !== undefined) {
      body.temperature = request.temperature
    }
    if (request.topP !== undefined) {
      body.top_p = request.topP
    }
    if (request.responseFormat === 'json_object') {
      body.text = { format: { type: 'json_object' } }
    }
    const reasoning = responsesReasoningForEffort(
      request.reasoningEffort,
      this.modelReasoningFor(model)
    )
    if (reasoning) body.reasoning = reasoning
    const tools = normalizeToolSpecs(request.tools)
    if (tools.length > 0) {
      body.tools = tools.map((tool) => ({
        type: 'function',
        name: tool.name,
        description: tool.description,
        parameters: tool.inputSchema
      }))
    }
    return body
  }

  private buildAnthropicMessagesRequestBody(
    request: ModelRequest,
    model: string,
    messages: ChatMessage[],
    stream: boolean
  ): Record<string, unknown> {
    const converted = messagesToAnthropic(
      messages,
      this.modelReasoningFor(model)?.requestProtocol === 'anthropic-thinking'
    )
    applyAnthropicCacheControl(converted.messages)
    const body: Record<string, unknown> = {
      model,
      stream,
      max_tokens: request.maxTokens ?? DEFAULT_MESSAGES_MAX_TOKENS,
      messages: converted.messages
    }
    const systemText = request.responseFormat === 'json_object'
      ? [converted.system, 'Return a valid JSON object only.']
          .filter((item) => item.trim().length > 0)
          .join('\n\n')
      : converted.system
    if (systemText) {
      body.system = [
        { type: 'text', text: systemText, cache_control: { type: 'ephemeral' } }
      ] satisfies AnthropicContentBlock[]
    }
    if (request.temperature !== undefined) {
      body.temperature = request.temperature
    }
    if (request.topP !== undefined) {
      body.top_p = request.topP
    }
    applyAnthropicReasoningEffort(body, request.reasoningEffort, this.modelReasoningFor(model))
    const tools = normalizeToolSpecs(request.tools)
    if (tools.length > 0) {
      body.tools = tools.map((tool) => ({
        name: tool.name,
        description: tool.description,
        input_schema: tool.inputSchema
      }))
    }
    return body
  }

  private collectMessages(request: ModelRequest, model: string): ChatMessage[] {
    const out: ChatMessage[] = []
    if (request.systemPrompt) {
      out.push({ role: 'system', content: request.systemPrompt })
    }
    if (request.modeInstruction) {
      out.push({ role: 'system', content: request.modeInstruction })
    }
    const windowSize = this.config.historyLimit
    const history = windowSize
      ? limitHistoryPreservingCompaction(request.history, windowSize)
      : request.history
    const thinkingMode = requiresReasoningRoundTrip(
      request.reasoningEffort,
      model,
      this.config.baseUrl,
      this.modelReasoningFor(model)
    )
    const supportsImages = this.modelSupportsImageInput(model)
    out.push(...this.itemsToMessages(
      repairModelHistoryItems([...request.prefix, ...history]),
      thinkingMode,
      supportsImages
    ))
    // Per-turn context (goal budgets, todo state, memories, skill notes,
    // drift warnings) is volatile — the goal instruction alone embeds a
    // tokens-used counter that changes every step. It must trail the
    // stable history: placed before it, every counter tick invalidated
    // the provider prompt cache for the entire conversation.
    for (const instruction of request.contextInstructions ?? []) {
      if (instruction.trim()) out.push({ role: 'system', content: instruction })
    }
    if (request.attachments?.length) {
      attachImagesToLatestUserMessage(out, request.attachments)
    }
    if (request.attachmentTextFallbacks?.length) {
      attachTextFallbacksToLatestUserMessage(out, request.attachmentTextFallbacks)
    }
    return normalizeThinkingAssistantMessages(healToolMessagePairs(out), thinkingMode)
  }

  private itemsToMessages(items: ModelHistoryItem[], thinkingMode: boolean, supportsImages: boolean): ChatMessage[] {
    const out: ChatMessage[] = []
    for (let index = 0; index < items.length; index += 1) {
      const item = items[index]
      if (isBridgeItemBeforeToolCall(items, index)) {
        continue
      }
      if (thinkingMode && item?.kind === 'assistant_reasoning') {
        const next = items[index + 1]
        if (next?.kind === 'assistant_text' && next.turnId === item.turnId) {
          out.push({
            role: 'assistant',
            content: next.text
          })
          index += 1
        }
        continue
      }
      if (item?.kind === 'tool_call') {
        const block = this.toolCallBlockToMessages(items, index, thinkingMode, supportsImages)
        if (block) {
          out.push(...block.messages)
          index = block.nextIndex - 1
        }
        continue
      }
      if (item?.kind === 'tool_result') continue
      const message = this.itemToMessage(item, thinkingMode, supportsImages)
      if (message) out.push(message)
    }
    return out
  }

  private toolCallBlockToMessages(
    items: ModelHistoryItem[],
    startIndex: number,
    thinkingMode: boolean,
    supportsImages: boolean
  ): { messages: ChatMessage[]; nextIndex: number } | null {
    const calls: Extract<ModelHistoryItem, { kind: 'tool_call' }>[] = []
    let index = startIndex
    while (index < items.length && items[index]?.kind === 'tool_call') {
      calls.push(items[index] as Extract<ModelHistoryItem, { kind: 'tool_call' }>)
      index += 1
    }
    if (calls.length === 0) return null

    const turnId = calls[0]?.turnId ?? ''
    const expectedCallIds = new Set(calls.map((call) => call.callId))
    const seenResultIds = new Set<string>()
    const resultMessages: ChatMessage[] = []
    const assistantText: string[] = []
    const reasoningText: string[] = []
    let reasoningSignature = ''
    let bridgeIndex = startIndex - 1
    while (bridgeIndex >= 0) {
      const item = items[bridgeIndex]
      if (!item || !isPreToolCallBridgeItem(item, turnId)) break
      if (item.kind === 'assistant_text' && item.text.trim()) {
        assistantText.unshift(item.text)
      } else if (item.kind === 'assistant_reasoning' && item.text.trim()) {
        reasoningText.unshift(item.text)
        if (item.signature) reasoningSignature ||= item.signature
      }
      bridgeIndex -= 1
    }
    let sawResult = false
    while (index < items.length) {
      const item = items[index]
      if (!item) break
      if (item.kind === 'tool_result') {
        sawResult = true
        if (expectedCallIds.has(item.callId) && !seenResultIds.has(item.callId)) {
          seenResultIds.add(item.callId)
          resultMessages.push(this.toolResultToMessage(item, supportsImages))
        }
        index += 1
        continue
      }
      if (isToolResultBridgeItem(item, { turnId, sawResult })) {
        if (!sawResult) {
          if (item.kind === 'assistant_text' && item.text.trim()) {
            assistantText.push(item.text)
          } else if (item.kind === 'assistant_reasoning' && item.text.trim()) {
            reasoningText.push(item.text)
            if (item.signature) reasoningSignature = item.signature
          }
        }
        index += 1
        continue
      }
      break
    }

    if (![...expectedCallIds].every((callId) => seenResultIds.has(callId))) {
      return null
    }
    return {
      messages: [
        {
          role: 'assistant',
          content: assistantText.length > 0 ? assistantText.join('\n') : '',
          ...(thinkingMode ? { reasoning_content: reasoningContentOrSpace(reasoningText.join('\n')) } : {}),
          ...(thinkingMode && reasoningSignature ? { reasoning_signature: reasoningSignature } : {}),
          tool_calls: calls.map((call) => this.toolCallToWire(call))
        },
        ...resultMessages
      ],
      nextIndex: index
    }
  }

  private toolCallToWire(item: Extract<ModelHistoryItem, { kind: 'tool_call' }>): NonNullable<ChatMessage['tool_calls']>[number] {
    return {
      id: item.callId,
      type: 'function',
      function: { name: item.toolName, arguments: JSON.stringify(item.arguments) }
    }
  }

  private toolResultToMessage(
    item: Extract<ModelHistoryItem, { kind: 'tool_result' }>,
    supportsImages: boolean
  ): ChatMessage {
    const images = extractToolResultImages(item.output)
    if (images.length > 0) {
      const text = toolResultTextWithoutImages(item.output)
      // Text-only models reject image parts; keep a useful textual trace
      // and avoid leaking large base64 blobs into the prompt.
      if (!supportsImages) {
        return {
          role: 'tool',
          content: text || '(image omitted: the active model has no image input)',
          tool_call_id: item.callId
        }
      }
      const parts: ChatMessageContentPart[] = []
      if (text) parts.push({ type: 'text', text })
      for (const image of images) {
        parts.push({
          type: 'image_url',
          image_url: { url: `data:${image.mimeType};base64,${image.dataBase64}` }
        })
      }
      return { role: 'tool', content: parts, tool_call_id: item.callId }
    }
    return {
      role: 'tool',
      content: toolResultContent(item.output),
      tool_call_id: item.callId
    }
  }

  private modelSupportsImageInput(model: string): boolean {
    const capabilities = this.config.modelCapabilities?.(model)
    if (!capabilities) return false
    return capabilities.inputModalities.includes('image')
  }

  private itemToMessage(item: ModelHistoryItem, thinkingMode: boolean, supportsImages: boolean): ChatMessage | null {
    switch (item.kind) {
      case 'tool_progress':
        return null
      case 'user_message':
        return { role: 'user', content: modelUserMessageText(item) }
      case 'assistant_text':
        return {
          role: 'assistant',
          content: item.text
        }
      case 'assistant_reasoning':
        return null
      case 'tool_call':
        return {
          role: 'assistant',
          content: '',
          ...(thinkingMode ? { reasoning_content: ' ' } : {}),
          tool_calls: [this.toolCallToWire(item)]
        }
      case 'tool_result':
        return this.toolResultToMessage(item, supportsImages)
      case 'compaction':
        return (item.replacedTokens ?? 0) > 0
          ? { role: 'system', content: `Conversation summary from earlier turns:\n${item.summary}` }
          : null
      case 'review':
        return item.status === 'completed' && item.reviewText?.trim()
          ? { role: 'system', content: `Code review result from an earlier turn:\n${item.reviewText}` }
          : null
      case 'approval':
      case 'user_input':
      case 'error':
        return null
    }
  }

  private async *streamSse(
    body: ReadableStream<Uint8Array>,
    signal: AbortSignal,
    endpointFormat: ModelEndpointFormat,
    model: string
  ): AsyncIterable<ModelStreamChunk> {
    const decoder = new TextDecoder('utf-8')
    const reader = body.getReader()
    let buffer = ''
    const pendingArguments = new Map<string, PendingToolCall>()
    const pendingByIndex = new Map<number, string>()
    const completedToolCalls = new Set<string>()
    let usage: UsageSnapshot | null = null
    let textAccumulator = ''
    let reasoningAccumulator = ''
    let stopReason: ModelStopReason = 'stop'
    let finishReason: string | null = null
    let sawDone = false
    const thinkingTagSplitter = new LeadingThinkingTagSplitter()
    const idleTimeoutMs = normalizeStreamIdleTimeoutMs(this.config.streamIdleTimeoutMs)
    try {
      while (!signal.aborted) {
        const read = await readStreamChunk(reader, signal, idleTimeoutMs)
        if (read.kind === 'timeout') {
          yield {
            kind: 'error',
            message: `model stream stalled for ${idleTimeoutMs}ms without data`,
            code: 'stream_idle_timeout'
          }
          return
        }
        if (read.kind === 'aborted') break
        if (read.kind === 'error') {
          yield { kind: 'error', message: read.message, code: 'stream_read_error' }
          return
        }
        const { value, done } = read
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        let boundary: number
        while ((boundary = buffer.indexOf('\n\n')) >= 0) {
          const frame = buffer.slice(0, boundary)
          buffer = buffer.slice(boundary + 2)
          const dataLines = frame
            .split('\n')
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trim())
            .join('')
          if (!dataLines) continue
          if (dataLines === '[DONE]') {
            finishReason = finishReason ?? 'stop'
            sawDone = true
            break
          }
          let payload: unknown
          try {
            payload = JSON.parse(dataLines)
          } catch {
            continue
          }
          const result = this.consumeStreamPayload(
            payload as Record<string, unknown>,
            pendingArguments,
            pendingByIndex,
            completedToolCalls,
            textAccumulator,
            reasoningAccumulator,
            endpointFormat,
            model,
            thinkingTagSplitter
          )
          textAccumulator = result.text
          reasoningAccumulator = result.reasoning
          if (result.usage) usage = mergeUsageSnapshots(usage, result.usage)
          if (result.finishReason) finishReason = result.finishReason
          for (const chunk of result.chunks) yield chunk
        }
        if (sawDone) break
      }
    } finally {
      try {
        reader.releaseLock()
      } catch {
        // The stream may already be released; ignore.
      }
    }
    if (signal.aborted) {
      yield { kind: 'error', message: 'request was aborted' }
      return
    }
    if (!sawDone && !finishReason) {
      if (pendingArguments.size > 0) {
        yield {
          kind: 'error',
          message: `model ${endpointFormat} stream ended before completing tool calls`,
          code: 'stream_incomplete_tool_call'
        }
        return
      }
      yield {
        kind: 'error',
        message: `model ${endpointFormat} stream ended before completion`,
        code: 'stream_ended_before_completion'
      }
      return
    }
    const finalThinkingTagSplit = thinkingTagSplitter.flush()
    if (finalThinkingTagSplit.reasoning) {
      yield { kind: 'assistant_reasoning_delta', text: finalThinkingTagSplit.reasoning }
    }
    if (finalThinkingTagSplit.text) {
      yield { kind: 'assistant_text_delta', text: finalThinkingTagSplit.text }
    }
    if (usage) yield { kind: 'usage', usage }
    stopReason = ((): ModelStopReason => {
      switch (finishReason) {
        case 'tool_calls':
          return 'tool_calls'
        case 'length':
          return 'length'
        case 'error':
          return 'error'
        default:
          return 'stop'
      }
    })()
    yield { kind: 'completed', stopReason }
  }

  private consumeStreamPayload(
    payload: Record<string, unknown>,
    pendingArguments: Map<string, PendingToolCall>,
    pendingByIndex: Map<number, string>,
    completedToolCalls: Set<string>,
    textAccumulator: string,
    reasoningAccumulator: string,
    endpointFormat: ModelEndpointFormat,
    model: string,
    thinkingTagSplitter: LeadingThinkingTagSplitter
  ): {
    chunks: ModelStreamChunk[]
    text: string
    reasoning: string
    finishReason: string | null
    usage: UsageSnapshot | null
  } {
    const payloadError = modelPayloadError(payload)
    if (payloadError) {
      return {
        chunks: [{
          kind: 'error',
          message: payloadError.message,
          ...(payloadError.code ? { code: payloadError.code } : {})
        }],
        text: textAccumulator,
        reasoning: reasoningAccumulator,
        finishReason: 'error',
        usage: null
      }
    }
    if (endpointFormat === 'responses') {
      return this.consumeResponsesStreamPayload(
        payload,
        pendingArguments,
        pendingByIndex,
        completedToolCalls,
        textAccumulator,
        reasoningAccumulator,
        model
      )
    }
    if (endpointFormat === 'messages') {
      return this.consumeAnthropicMessagesStreamPayload(
        payload,
        pendingArguments,
        pendingByIndex,
        completedToolCalls,
        textAccumulator,
        reasoningAccumulator,
        model
      )
    }
    const chunks: ModelStreamChunk[] = []
    let text = textAccumulator
    let reasoning = reasoningAccumulator
    let finishReason: string | null = null
    let usage: UsageSnapshot | null = null
    const choice = (payload.choices as Record<string, unknown>[] | undefined)?.[0]
    if (choice && typeof choice === 'object') {
      const delta = choice.delta as Record<string, unknown> | undefined
      if (delta && typeof delta === 'object') {
        const content = delta.content
        if (typeof content === 'string' && content.length > 0) {
          const split = thinkingTagSplitter.push(content)
          if (split.reasoning) {
            reasoning += split.reasoning
            chunks.push({ kind: 'assistant_reasoning_delta', text: split.reasoning })
          }
          if (split.text) {
            text += split.text
            chunks.push({ kind: 'assistant_text_delta', text: split.text })
          }
        }
        const reasoningContent = delta.reasoning_content ?? delta.reasoning
        if (typeof reasoningContent === 'string' && reasoningContent.length > 0) {
          reasoning += reasoningContent
          chunks.push({ kind: 'assistant_reasoning_delta', text: reasoningContent })
        }
        const toolCalls = delta.tool_calls as
          | {
              index?: number
              id?: string
              function?: { name?: string; arguments?: string }
            }[]
          | undefined
        if (Array.isArray(toolCalls)) {
          for (const call of toolCalls) {
            const id = resolveToolCallDeltaId(call, pendingArguments)
            const existing = pendingArguments.get(id) ?? { index: numericIndex(call.index), name: undefined, arguments: '' }
            const resolvedIndex = numericIndex(call.index)
            if (resolvedIndex !== undefined) existing.index = resolvedIndex
            const previousName = existing.name
            if (call.function?.name) existing.name = call.function.name
            if (existing.name && existing.name !== previousName) {
              chunks.push({
                kind: 'tool_call_delta',
                callId: id,
                toolName: existing.name
              })
            }
            if (typeof call.function?.arguments === 'string') {
              existing.arguments += call.function.arguments
              chunks.push({
                kind: 'tool_call_delta',
                callId: id,
                toolName: existing.name,
                argumentsDelta: call.function.arguments
              })
            }
            pendingArguments.set(id, existing)
          }
        }
      }
      if (typeof choice.finish_reason === 'string') {
        finishReason = choice.finish_reason
      }
    }
    const usagePayload = payload.usage as Record<string, unknown> | undefined
    if (usagePayload) {
      usage = this.mapUsage(usagePayload, model)
    }
    if (finishReason === 'tool_calls' && pendingArguments.size > 0) {
      for (const [callId, value] of pendingArguments) {
        if (!value.name) continue
        const args = this.parseToolArguments(value.arguments)
        chunks.push({
          kind: 'tool_call_complete',
          callId,
          toolName: value.name,
          arguments: args
        })
      }
      pendingArguments.clear()
    }
    return { chunks, text, reasoning, finishReason, usage }
  }

  private consumeResponsesStreamPayload(
    payload: Record<string, unknown>,
    pendingArguments: Map<string, PendingToolCall>,
    pendingByIndex: Map<number, string>,
    completedToolCalls: Set<string>,
    textAccumulator: string,
    reasoningAccumulator: string,
    model: string
  ): {
    chunks: ModelStreamChunk[]
    text: string
    reasoning: string
    finishReason: string | null
    usage: UsageSnapshot | null
  } {
    const chunks: ModelStreamChunk[] = []
    let text = textAccumulator
    let reasoning = reasoningAccumulator
    let finishReason: string | null = null
    let usage: UsageSnapshot | null = null
    const type = recordString(payload, 'type')

    const outputIndex = numericIndex(payload.output_index)
    const item = recordValue(payload, 'item') ?? recordValue(payload, 'output_item')
    if (item) {
      const itemType = recordString(item, 'type')
      if (itemType === 'function_call' || itemType === 'custom_tool_call') {
        const callId = recordString(item, 'call_id') || recordString(item, 'id') || indexFallbackCallId(outputIndex, pendingArguments)
        const existing = pendingArguments.get(callId) ?? { index: outputIndex, name: undefined, arguments: '' }
        if (outputIndex !== undefined) {
          existing.index = outputIndex
          pendingByIndex.set(outputIndex, callId)
        }
        const name = recordString(item, 'name')
        const previousName = existing.name
        if (name) existing.name = name
        const initialArguments = recordString(item, 'arguments') || recordString(item, 'input')
        if (initialArguments && !existing.arguments) existing.arguments = initialArguments
        pendingArguments.set(callId, existing)
        if (existing.name && existing.name !== previousName) {
          chunks.push({
            kind: 'tool_call_delta',
            callId,
            toolName: existing.name
          })
        }
        if (type === 'response.output_item.done' && existing.name) {
          chunks.push({
            kind: 'tool_call_complete',
            callId,
            toolName: existing.name,
            arguments: this.parseToolArguments(existing.arguments || '{}')
          })
          completedToolCalls.add(callId)
          pendingArguments.delete(callId)
        }
      }
    }

    if (type === 'response.output_text.delta') {
      const delta = recordString(payload, 'delta')
      if (delta) {
        text += delta
        chunks.push({ kind: 'assistant_text_delta', text: delta })
      }
    } else if (
      type === 'response.reasoning_text.delta' ||
      type === 'response.reasoning_summary_text.delta' ||
      type === 'response.reasoning.delta'
    ) {
      const delta = recordString(payload, 'delta')
      if (delta) {
        reasoning += delta
        chunks.push({ kind: 'assistant_reasoning_delta', text: delta })
      }
    } else if (type === 'response.function_call_arguments.delta') {
      const callId = responseStreamCallId(payload, pendingArguments, pendingByIndex)
      const existing = pendingArguments.get(callId) ?? { index: outputIndex, name: undefined, arguments: '' }
      const delta = recordString(payload, 'delta')
      if (outputIndex !== undefined) {
        existing.index = outputIndex
        pendingByIndex.set(outputIndex, callId)
      }
      if (delta) {
        existing.arguments += delta
        chunks.push({
          kind: 'tool_call_delta',
          callId,
          toolName: existing.name,
          argumentsDelta: delta
        })
      }
      pendingArguments.set(callId, existing)
    } else if (type === 'response.function_call_arguments.done') {
      const callId = responseStreamCallId(payload, pendingArguments, pendingByIndex)
      const existing = pendingArguments.get(callId) ?? { index: outputIndex, name: undefined, arguments: '' }
      const args = recordString(payload, 'arguments')
      if (args) existing.arguments = args
      if (existing.name) {
        pendingArguments.set(callId, existing)
      } else {
        pendingArguments.set(callId, existing)
      }
    } else if (type === 'response.completed') {
      const response = recordValue(payload, 'response') as ResponsesApiResponse | null
      const materialized = this.materializeResponsesOutput(response ?? (payload as ResponsesApiResponse), {
        skipText: Boolean(text),
        pendingArguments,
        completedToolCalls
      }, model)
      chunks.push(...materialized.chunks)
      if (materialized.usage) usage = materialized.usage
      finishReason = materialized.finishReason
    } else if (type === 'response.failed' || type === 'error') {
      const message = responseErrorMessage(payload)
      chunks.push({ kind: 'error', message, code: 'response_stream_error' })
      finishReason = 'error'
    }
    return { chunks, text, reasoning, finishReason, usage }
  }

  private consumeAnthropicMessagesStreamPayload(
    payload: Record<string, unknown>,
    pendingArguments: Map<string, PendingToolCall>,
    pendingByIndex: Map<number, string>,
    completedToolCalls: Set<string>,
    textAccumulator: string,
    reasoningAccumulator: string,
    model: string
  ): {
    chunks: ModelStreamChunk[]
    text: string
    reasoning: string
    finishReason: string | null
    usage: UsageSnapshot | null
  } {
    const chunks: ModelStreamChunk[] = []
    let text = textAccumulator
    let reasoning = reasoningAccumulator
    let finishReason: string | null = null
    let usage: UsageSnapshot | null = null
    const type = recordString(payload, 'type')
    const index = numericIndex(payload.index)

    if (type === 'message_start') {
      const message = recordValue(payload, 'message')
      const usagePayload = message ? recordValue(message, 'usage') : null
      if (usagePayload) usage = this.mapUsage(usagePayload, model)
    } else if (type === 'content_block_start') {
      const block = recordValue(payload, 'content_block')
      if (block && recordString(block, 'type') === 'tool_use') {
        const callId = recordString(block, 'id') || indexFallbackCallId(index, pendingArguments)
        const existing = pendingArguments.get(callId) ?? { index, name: undefined, arguments: '' }
        if (index !== undefined) {
          existing.index = index
          pendingByIndex.set(index, callId)
        }
        const name = recordString(block, 'name')
        const previousName = existing.name
        if (name) existing.name = name
        const input = recordValue(block, 'input')
        if (input && Object.keys(input).length > 0) existing.arguments = JSON.stringify(input)
        pendingArguments.set(callId, existing)
        if (existing.name && existing.name !== previousName) {
          chunks.push({
            kind: 'tool_call_delta',
            callId,
            toolName: existing.name
          })
        }
      }
    } else if (type === 'content_block_delta') {
      const delta = recordValue(payload, 'delta')
      const deltaType = delta ? recordString(delta, 'type') : ''
      if (deltaType === 'text_delta') {
        const value = recordString(delta, 'text')
        if (value) {
          text += value
          chunks.push({ kind: 'assistant_text_delta', text: value })
        }
      } else if (deltaType === 'thinking_delta') {
        const value = recordString(delta, 'thinking')
        if (value) {
          reasoning += value
          chunks.push({ kind: 'assistant_reasoning_delta', text: value })
        }
      } else if (deltaType === 'signature_delta') {
        const signature = recordString(delta, 'signature')
        if (signature) {
          chunks.push({ kind: 'assistant_reasoning_delta', text: '', signature })
        }
      } else if (deltaType === 'input_json_delta') {
        const callId = anthropicStreamCallId(index, pendingArguments, pendingByIndex)
        const existing = pendingArguments.get(callId) ?? { index, name: undefined, arguments: '' }
        const value = recordString(delta, 'partial_json')
        if (index !== undefined) {
          existing.index = index
          pendingByIndex.set(index, callId)
        }
        if (value) {
          existing.arguments += value
          chunks.push({
            kind: 'tool_call_delta',
            callId,
            toolName: existing.name,
            argumentsDelta: value
          })
        }
        pendingArguments.set(callId, existing)
      }
    } else if (type === 'content_block_stop') {
      const callId = index === undefined ? undefined : pendingByIndex.get(index)
      const pending = callId ? pendingArguments.get(callId) : undefined
      if (callId && pending?.name) {
        chunks.push({
          kind: 'tool_call_complete',
          callId,
          toolName: pending.name,
          arguments: this.parseToolArguments(pending.arguments || '{}')
        })
        completedToolCalls.add(callId)
        pendingArguments.delete(callId)
        if (index !== undefined) pendingByIndex.delete(index)
      }
    } else if (type === 'message_delta') {
      const delta = recordValue(payload, 'delta')
      const stopReason = delta ? recordString(delta, 'stop_reason') : ''
      const mappedStopReason = anthropicStopReason(stopReason)
      if (mappedStopReason) finishReason = mappedStopReason
      const usagePayload = recordValue(payload, 'usage')
      if (usagePayload) usage = this.mapUsage(usagePayload, model)
    } else if (type === 'message_stop') {
      finishReason = finishReason ?? 'stop'
    } else if (type === 'error') {
      chunks.push({ kind: 'error', message: responseErrorMessage(payload), code: 'messages_stream_error' })
      finishReason = 'error'
    }
    return { chunks, text, reasoning, finishReason, usage }
  }

  private *materializeNonStreaming(
    payload: ChatCompletionResponse,
    endpointFormat: ModelEndpointFormat,
    model: string
  ): Generator<ModelStreamChunk> {
    const payloadError = modelPayloadError(payload as unknown as Record<string, unknown>)
    if (payloadError) {
      yield {
        kind: 'error',
        message: payloadError.message,
        ...(payloadError.code ? { code: payloadError.code } : {})
      }
      return
    }
    if (endpointFormat === 'responses') {
      yield* this.materializeResponsesNonStreaming(payload as unknown as ResponsesApiResponse, model)
      return
    }
    if (endpointFormat === 'messages') {
      yield* this.materializeAnthropicMessagesNonStreaming(payload as unknown as AnthropicMessageResponse, model)
      return
    }
    const choice = payload.choices?.[0]
    if (!choice) {
      yield { kind: 'error', message: 'model response contained no choices' }
      return
    }
    const text = splitLeadingThinkingTag(typeof choice.message?.content === 'string' ? choice.message.content : '')
    const reasoning = reasoningFromMessage(choice.message)
    if (reasoning) {
      yield { kind: 'assistant_reasoning_delta', text: reasoning }
    }
    if (text.reasoning) {
      yield { kind: 'assistant_reasoning_delta', text: text.reasoning }
    }
    if (text.text) {
      yield { kind: 'assistant_text_delta', text: text.text }
    }
    if (Array.isArray(choice.message?.tool_calls)) {
      for (const call of choice.message.tool_calls) {
        const args = this.parseToolArguments(call.function?.arguments ?? '{}')
        yield {
          kind: 'tool_call_complete',
          callId: call.id,
          toolName: call.function.name,
          arguments: args
        }
      }
    }
    if (payload.usage) {
      yield { kind: 'usage', usage: this.mapUsage(payload.usage, model) }
    }
    let stopReason: 'stop' | 'tool_calls' | 'length' | 'error' = 'stop'
    if (choice.finish_reason === 'tool_calls') stopReason = 'tool_calls'
    else if (choice.finish_reason === 'length') stopReason = 'length'
    else if (choice.finish_reason === 'error') stopReason = 'error'
    yield { kind: 'completed', stopReason }
  }

  private *materializeResponsesNonStreaming(
    payload: ResponsesApiResponse,
    model: string
  ): Generator<ModelStreamChunk> {
    if (payload.error?.message) {
      yield { kind: 'error', message: payload.error.message, code: payload.error.type }
      return
    }
    const materialized = this.materializeResponsesOutput(payload, {}, model)
    yield* materialized.chunks
    if (materialized.usage) {
      yield { kind: 'usage', usage: materialized.usage }
    }
    yield { kind: 'completed', stopReason: materialized.finishReason }
  }

  private materializeResponsesOutput(
    payload: ResponsesApiResponse,
    options: {
      skipText?: boolean
      pendingArguments?: Map<string, PendingToolCall>
      completedToolCalls?: Set<string>
    } = {},
    model = this.config.model
  ): {
    chunks: ModelStreamChunk[]
    finishReason: ModelStopReason
    usage: UsageSnapshot | null
  } {
    const chunks: ModelStreamChunk[] = []
    let sawToolCall = (options.completedToolCalls?.size ?? 0) > 0
    if (!options.skipText) {
      const outputText = typeof payload.output_text === 'string'
        ? payload.output_text
        : responsesOutputText(payload.output)
      if (outputText) {
        chunks.push({ kind: 'assistant_text_delta', text: outputText })
      }
    }
    for (const item of payload.output ?? []) {
      const itemType = recordString(item, 'type')
      if (itemType !== 'function_call' && itemType !== 'custom_tool_call') continue
      const callId = recordString(item, 'call_id') || recordString(item, 'id')
      const toolName = recordString(item, 'name')
      if (!callId || !toolName) continue
      if (options.completedToolCalls?.has(callId)) continue
      sawToolCall = true
      const argsRaw = recordString(item, 'arguments') || recordString(item, 'input') || '{}'
      if (options.pendingArguments?.has(callId)) {
        options.pendingArguments.delete(callId)
      }
      chunks.push({
        kind: 'tool_call_complete',
        callId,
        toolName,
        arguments: this.parseToolArguments(argsRaw)
      })
    }
    const usage = payload.usage ? this.mapUsage(payload.usage, model) : null
    let finishReason: ModelStopReason = sawToolCall ? 'tool_calls' : 'stop'
    if (payload.status === 'incomplete') {
      finishReason = payload.incomplete_details?.reason === 'max_output_tokens' ? 'length' : 'error'
    } else if (payload.status === 'failed') {
      finishReason = 'error'
    }
    return { chunks, finishReason, usage }
  }

  private *materializeAnthropicMessagesNonStreaming(
    payload: AnthropicMessageResponse,
    model: string
  ): Generator<ModelStreamChunk> {
    let sawToolCall = false
    for (const block of payload.content ?? []) {
      const type = recordString(block, 'type')
      if (type === 'text') {
        const text = recordString(block, 'text')
        if (text) yield { kind: 'assistant_text_delta', text }
      } else if (type === 'thinking') {
        const thinking = recordString(block, 'thinking')
        const signature = recordString(block, 'signature')
        if (thinking) {
          yield {
            kind: 'assistant_reasoning_delta',
            text: thinking,
            ...(signature ? { signature } : {})
          }
        }
      } else if (type === 'tool_use') {
        const callId = recordString(block, 'id')
        const toolName = recordString(block, 'name')
        const input = recordValue(block, 'input') ?? {}
        if (callId && toolName) {
          sawToolCall = true
          yield {
            kind: 'tool_call_complete',
            callId,
            toolName,
            arguments: input
          }
        }
      }
    }
    if (payload.usage) {
      yield { kind: 'usage', usage: this.mapUsage(payload.usage, model) }
    }
    yield { kind: 'completed', stopReason: anthropicStopReason(payload.stop_reason) ?? (sawToolCall ? 'tool_calls' : 'stop') }
  }

  private mapUsage(usage: Record<string, unknown>, model = this.config.model): UsageSnapshot {
    const completionTokens = Number(usage.completion_tokens ?? usage.eval_count ?? usage.output_tokens ?? 0) || 0
    const reasoningTokens = providerReasoningTokens(usage)
    const promptDetails = usage.prompt_tokens_details as
      | { cached_tokens?: number }
      | undefined
    const inputDetails = usage.input_tokens_details as
      | { cached_tokens?: number }
      | undefined
    const nativeHit = Number(usage.prompt_cache_hit_tokens ?? 0) || 0
    const nativeMiss = Number(usage.prompt_cache_miss_tokens ?? 0) || 0
    const hasNativeCache = nativeHit > 0 || nativeMiss > 0
    const promptCachedTokens = numericUsageField(promptDetails?.cached_tokens)
    const inputCachedTokens = numericUsageField(inputDetails?.cached_tokens)
    const hasCachedTokenDetails = promptCachedTokens !== undefined || inputCachedTokens !== undefined
    const cachedTokens = promptCachedTokens ?? inputCachedTokens ?? 0
    const cacheRead = Number(usage.cache_read_input_tokens ?? 0) || 0
    const cacheCreation = Number(usage.cache_creation_input_tokens ?? 0) || 0
    // Anthropic-protocol usage (MiniMax et al.) reports input_tokens
    // EXCLUDING cache reads/writes; OpenAI-style prompt_tokens includes
    // everything and marks the cached subset in prompt_tokens_details or
    // Responses API input_tokens_details.
    const anthropicUsage = usage.prompt_tokens === undefined &&
      usage.prompt_eval_count === undefined &&
      usage.input_tokens !== undefined &&
      inputDetails?.cached_tokens === undefined
    const reportedPromptTokens = Number(usage.prompt_tokens ?? usage.prompt_eval_count ?? usage.input_tokens ?? 0) || 0
    const promptTokens = anthropicUsage
      ? reportedPromptTokens + cacheRead + cacheCreation
      : reportedPromptTokens
    const hasCacheTelemetry = hasNativeCache || hasCachedTokenDetails || cacheRead > 0 || cacheCreation > 0
    const cacheHit = hasNativeCache ? nativeHit : (hasCachedTokenDetails ? cachedTokens : cacheRead)
    const cacheMiss = hasNativeCache ? nativeMiss : Math.max(promptTokens - cacheHit, 0)
    const cacheTotal = cacheHit + cacheMiss
    const cacheHitRate = hasCacheTelemetry && cacheTotal > 0 ? cacheHit / cacheTotal : null
    const totalTokens = anthropicUsage
      ? promptTokens + completionTokens
      : Number(usage.total_tokens ?? promptTokens + completionTokens) || 0
    const pricingCacheRead = cacheRead || (hasCacheTelemetry ? cacheHit : 0)
    const pricingCacheWrite = cacheCreation
    const pricingInputTokens = anthropicUsage
      ? reportedPromptTokens
      : Math.max(promptTokens - pricingCacheRead - pricingCacheWrite, 0)
    const estimatedCost = estimateDeepseekCost({
      model,
      providerHost: this.config.baseUrl,
      cacheHitTokens: hasCacheTelemetry ? cacheHit : 0,
      cacheMissTokens: hasCacheTelemetry ? cacheMiss : promptTokens,
      outputTokens: completionTokens
    }) ?? estimateMiniMaxCost({
      model,
      providerHost: this.config.baseUrl,
      inputTokens: pricingInputTokens,
      cacheReadTokens: pricingCacheRead,
      cacheWriteTokens: pricingCacheWrite,
      outputTokens: completionTokens
    })
    const reportedCostUsd = Number(usage.cost_usd ?? usage.costUsd)
    const reportedCostCny = Number(usage.cost_cny ?? usage.costCny)
    const priceConfigured = Number.isFinite(reportedCostUsd) ||
      Number.isFinite(reportedCostCny) ||
      estimatedCost != null
    return {
      ...emptyUsageSnapshot(),
      promptTokens,
      completionTokens,
      ...(reasoningTokens > 0 ? { reasoningTokens } : {}),
      totalTokens,
      cachedTokens: hasCacheTelemetry ? cacheHit || cachedTokens || cacheRead || 0 : undefined,
      cacheHitTokens: hasCacheTelemetry ? cacheHit : undefined,
      cacheMissTokens: hasCacheTelemetry ? cacheMiss : undefined,
      cacheHitRate,
      turns: 1,
      priceConfigured,
      costUsd: Number.isFinite(reportedCostUsd) ? reportedCostUsd : estimatedCost?.costUsd,
      costCny: Number.isFinite(reportedCostCny) ? reportedCostCny : estimatedCost?.costCny
    }
  }

  private parseToolArguments(raw: string): Record<string, unknown> {
    return repairToolArguments(raw).arguments
  }
}

function normalizeToolSpecs(tools: ModelToolSpec[]): ModelToolSpec[] {
  return [...tools]
    .map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: canonicalizeToolInputSchema(tool.inputSchema)
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

function providerReasoningTokens(usage: Record<string, unknown>): number {
  const direct = numericUsageField(usage.reasoning_tokens) ??
    numericUsageField(usage.reasoning_output_tokens)
  if (direct !== undefined) return direct
  const completionDetails = usage.completion_tokens_details as Record<string, unknown> | undefined
  const outputDetails = usage.output_tokens_details as Record<string, unknown> | undefined
  return numericUsageField(completionDetails?.reasoning_tokens) ??
    numericUsageField(outputDetails?.reasoning_tokens) ??
    0
}

function numericUsageField(value: unknown): number | undefined {
  if (value === undefined || value === null || value === '') return undefined
  const numberValue = Number(value)
  return Number.isFinite(numberValue) && numberValue >= 0 ? Math.floor(numberValue) : undefined
}

function messagesToResponsesInput(messages: ChatMessage[]): Array<Record<string, unknown>> {
  const input: Array<Record<string, unknown>> = []
  for (const message of messages) {
    if (message.role === 'tool') {
      if (message.tool_call_id) {
        input.push({
          type: 'function_call_output',
          call_id: message.tool_call_id,
          output: chatContentToPlainText(message.content)
        })
      }
      continue
    }
    const content = chatContentToResponsesContent(message.content)
    if (content !== undefined && !(Array.isArray(content) && content.length === 0)) {
      input.push({
        role: message.role,
        content
      })
    }
    for (const call of message.tool_calls ?? []) {
      input.push({
        type: 'function_call',
        call_id: call.id,
        name: call.function.name,
        arguments: call.function.arguments,
        status: 'completed'
      })
    }
  }
  return input
}

function messagesToAnthropic(
  messages: ChatMessage[],
  includeThinkingBlocks = false
): { system: string; messages: AnthropicMessage[] } {
  const system: string[] = []
  const out: AnthropicMessage[] = []
  for (const message of messages) {
    if (message.role === 'system') {
      const text = chatContentToPlainText(message.content).trim()
      if (!text) continue
      // System messages that arrive after conversation turns are the
      // volatile per-turn context (goal budgets, memories, drift
      // warnings). Hoisting them into the top-level `system` block
      // would invalidate the provider's prompt cache for the whole
      // conversation on every counter tick, so they trail the history
      // inside a user turn instead — mirroring the chat_completions
      // ordering in collectMessages.
      if (out.length > 0) {
        appendTrailingInstruction(out, text)
        continue
      }
      system.push(text)
      continue
    }
    if (message.role === 'tool') {
      if (!message.tool_call_id) continue
      const blocks: AnthropicContentBlock[] = [{
        type: 'tool_result',
        tool_use_id: message.tool_call_id,
        content: chatContentToTextOnly(message.content)
      }]
      if (Array.isArray(message.content)) {
        for (const part of message.content) {
          if (part.type !== 'image_url') continue
          const image = anthropicImageSource(part.image_url.url)
          if (image) blocks.push({ type: 'image', source: image })
        }
      }
      out.push({
        role: 'user',
        content: blocks
      })
      continue
    }
    const content = chatContentToAnthropicContent(message.content)
    const blocks = Array.isArray(content)
      ? [...content]
      : content.trim()
        ? [{ type: 'text' as const, text: content }]
        : []
    if (includeThinkingBlocks && message.role === 'assistant') {
      const thinking = message.reasoning_content?.trim()
      const signature = message.reasoning_signature?.trim()
      if (thinking) {
        blocks.unshift({
          type: 'thinking',
          thinking,
          ...(signature ? { signature } : {})
        })
      }
    }
    for (const call of message.tool_calls ?? []) {
      blocks.push({
        type: 'tool_use',
        id: call.id,
        name: call.function.name,
        input: repairToolArguments(call.function.arguments).arguments
      })
    }
    if (blocks.length > 0) {
      out.push({ role: message.role, content: blocks })
      continue
    }
  }
  return { system: system.join('\n\n'), messages: out }
}

/**
 * OpenAI chat-completions and Responses APIs do not accept image parts
 * inside a `tool`/`function_call_output` message. When a tool result
 * carries images, keep the tool message text-only and re-emit the image
 * parts in a following synthetic user message so vision models still see
 * them. Anthropic Messages handles images inline and skips this split.
 */
function splitToolImageMessagesForOpenAi(messages: ChatMessage[]): ChatMessage[] {
  const hasToolImages = messages.some(
    (message) =>
      message.role === 'tool' &&
      Array.isArray(message.content) &&
      message.content.some((part) => part.type === 'image_url')
  )
  if (!hasToolImages) return messages
  const out: ChatMessage[] = []
  let pendingImages: ChatMessageContentPart[] = []
  const flushImages = (): void => {
    if (pendingImages.length === 0) return
    out.push({
      role: 'user',
      content: [
        { type: 'text', text: '(Automated) The tool call(s) above returned the following image(s):' },
        ...pendingImages
      ]
    })
    pendingImages = []
  }
  for (const message of messages) {
    if (message.role === 'tool' && Array.isArray(message.content)) {
      const textParts: string[] = []
      const imageParts: ChatMessageContentPart[] = []
      for (const part of message.content) {
        if (part.type === 'text') textParts.push(part.text)
        else imageParts.push(part)
      }
      out.push({
        ...message,
        content: textParts.join('\n') || '(image returned; see the following message)'
      })
      pendingImages.push(...imageParts)
      continue
    }
    if (message.role !== 'tool') flushImages()
    out.push(message)
  }
  flushImages()
  return out
}

/**
 * Folds a trailing system instruction into the conversation as user
 * content. Appends to the final user message when one exists so the
 * request keeps strict user/assistant alternation.
 */
function appendTrailingInstruction(out: AnthropicMessage[], text: string): void {
  const block: AnthropicContentBlock = { type: 'text', text }
  const last = out[out.length - 1]
  if (last && last.role === 'user') {
    if (typeof last.content === 'string') {
      last.content = last.content.trim()
        ? [{ type: 'text', text: last.content }, block]
        : [block]
      return
    }
    last.content.push(block)
    return
  }
  out.push({ role: 'user', content: [block] })
}

/**
 * Marks the stable prefix for provider-side prompt caching. Anthropic
 * protocol caching is explicit: providers such as MiniMax only cache
 * content before `cache_control` breakpoints (up to 4 per request).
 * One breakpoint goes on the system block (which also covers the tool
 * definitions that precede it) and one on the final content block of
 * each of the last two messages, so consecutive agent steps re-hit the
 * prefix cached by the previous request.
 */
function applyAnthropicCacheControl(messages: AnthropicMessage[]): void {
  let breakpoints = 0
  for (let i = messages.length - 1; i >= 0 && breakpoints < 2; i -= 1) {
    const content = messages[i].content
    if (typeof content === 'string' || content.length === 0) continue
    content[content.length - 1].cache_control = { type: 'ephemeral' }
    breakpoints += 1
  }
}

function chatContentToResponsesContent(
  content: ChatMessage['content']
): string | Array<Record<string, unknown>> | undefined {
  if (content === null || content === undefined) return undefined
  if (typeof content === 'string') return content
  const parts: Array<Record<string, unknown>> = []
  for (const part of content) {
    if (part.type === 'text') {
      parts.push({ type: 'input_text', text: part.text })
    } else if (part.type === 'image_url') {
      parts.push({ type: 'input_image', image_url: part.image_url.url })
    }
  }
  return parts
}

function chatContentToAnthropicContent(content: ChatMessage['content']): string | AnthropicContentBlock[] {
  if (content === null || content === undefined) return ''
  if (typeof content === 'string') return content
  const parts: AnthropicContentBlock[] = []
  for (const part of content) {
    if (part.type === 'text') {
      if (part.text) parts.push({ type: 'text', text: part.text })
      continue
    }
    const image = anthropicImageSource(part.image_url.url)
    if (image) parts.push({ type: 'image', source: image })
  }
  return parts
}

function anthropicImageSource(value: string): AnthropicImageSource | null {
  const data = parseDataUri(value)
  if (data) {
    return {
      type: 'base64',
      media_type: data.mimeType,
      data: data.base64
    }
  }
  if (/^https?:\/\//i.test(value)) {
    return { type: 'url', url: value }
  }
  return null
}

function parseDataUri(value: string): { mimeType: string; base64: string } | null {
  const match = /^data:([^;,]+);base64,(.*)$/is.exec(value)
  if (!match) return null
  return { mimeType: match[1], base64: match[2] }
}

function chatContentToPlainText(content: ChatMessage['content']): string {
  if (content === null || content === undefined) return ''
  if (typeof content === 'string') return content
  return content.map((part) => {
    if (part.type === 'text') return part.text
    return `[image: ${part.image_url.url}]`
  }).join('\n')
}

function chatContentToTextOnly(content: ChatMessage['content']): string {
  if (content === null || content === undefined) return ''
  if (typeof content === 'string') return content
  return content
    .filter((part): part is Extract<ChatMessageContentPart, { type: 'text' }> => part.type === 'text')
    .map((part) => part.text)
    .join('\n')
}

type ModelReasoningCapability = NonNullable<ModelCapabilityMetadata['reasoning']>
type NormalizedReasoningEffort = ModelReasoningCapability['defaultEffort']

function responsesReasoningForEffort(
  effort: string | undefined,
  reasoning?: ModelReasoningCapability
): Record<string, unknown> | null {
  if (reasoning && reasoning.requestProtocol !== 'openai-responses') return null
  const resolved = reasoning
    ? resolveReasoningEffort(effort, reasoning)
    : normalizeReasoningEffortValue(effort)
  if (resolved === 'auto' || resolved === 'off' || !resolved) return null
  const normalized = resolved
  switch (normalized) {
    case 'low':
      return { effort: 'low' }
    case 'medium':
      return { effort: 'medium' }
    case 'high':
    case 'max':
      return { effort: 'high' }
    default:
      return null
  }
}

function buildModelEndpointUrl(baseUrl: string, endpointFormat: ModelEndpointFormat): string {
  if (isCustomModelEndpointFormat(endpointFormat)) return exactModelEndpointUrl(baseUrl)
  const path = modelEndpointPath(endpointFormat)
  const normalized = baseUrl.trim().replace(/\/+$/, '')
  if (!normalized) return `/v1/${path}`
  const lastSegment = normalized.split('/').pop()?.toLowerCase() ?? ''
  if (lastSegment === 'beta') {
    return `${normalized.slice(0, -'/beta'.length)}/v1/${path}`
  }
  if (/^v\d+$/.test(lastSegment)) {
    return `${normalized}/${path}`
  }
  return `${normalized}/v1/${path}`
}

function exactModelEndpointUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim()
  const query = trimmed.search(/[?#]/)
  if (query < 0) return trimmed.replace(/\/+$/, '')
  return `${trimmed.slice(0, query).replace(/\/+$/, '')}${trimmed.slice(query)}`
}

function redactUrlForLog(url: string): string {
  const trimmed = url.trim()
  if (!trimmed) return ''
  try {
    const parsed = new URL(trimmed)
    for (const key of [...parsed.searchParams.keys()]) {
      if (/(key|token|secret|signature|auth|password)/i.test(key)) {
        parsed.searchParams.set(key, '[redacted]')
      }
    }
    return parsed.toString()
  } catch {
    return trimmed.replace(/([?&][^=&]*(?:key|token|secret|signature|auth|password)[^=]*=)[^&#]*/gi, '$1[redacted]')
  }
}

function buildChatCompletionsUrl(baseUrl: string): string {
  return buildModelEndpointUrl(baseUrl, 'chat_completions')
}

function responsesOutputText(output: ResponsesApiResponse['output']): string {
  const parts: string[] = []
  for (const item of output ?? []) {
    if (recordString(item, 'type') !== 'message') continue
    const content = item.content
    if (!Array.isArray(content)) continue
    for (const block of content) {
      if (!block || typeof block !== 'object') continue
      const record = block as Record<string, unknown>
      const type = recordString(record, 'type')
      if (type === 'output_text' || type === 'text') {
        const text = recordString(record, 'text')
        if (text) parts.push(text)
      }
    }
  }
  return parts.join('')
}

function responseStreamCallId(
  payload: Record<string, unknown>,
  pendingArguments: Map<string, PendingToolCall>,
  pendingByIndex: Map<number, string>
): string {
  const explicit = recordString(payload, 'call_id')
  if (explicit) return explicit
  const itemId = recordString(payload, 'item_id')
  if (itemId && pendingArguments.has(itemId)) return itemId
  const index = numericIndex(payload.output_index)
  if (index !== undefined) {
    return pendingByIndex.get(index) ?? indexFallbackCallId(index, pendingArguments)
  }
  if (pendingArguments.size === 1) return [...pendingArguments.keys()][0]
  return indexFallbackCallId(undefined, pendingArguments)
}

function anthropicStreamCallId(
  index: number | undefined,
  pendingArguments: Map<string, PendingToolCall>,
  pendingByIndex: Map<number, string>
): string {
  if (index !== undefined) {
    return pendingByIndex.get(index) ?? indexFallbackCallId(index, pendingArguments)
  }
  if (pendingArguments.size === 1) return [...pendingArguments.keys()][0]
  return indexFallbackCallId(undefined, pendingArguments)
}

function indexFallbackCallId(index: number | undefined, pendingArguments: Map<string, PendingToolCall>): string {
  return index === undefined ? `call_${pendingArguments.size + 1}` : `call_${index + 1}`
}

function responseErrorMessage(payload: Record<string, unknown>): string {
  const error = recordValue(payload, 'error') ?? recordValue(recordValue(payload, 'response'), 'error')
  const message = error ? recordString(error, 'message') : ''
  return message || recordString(payload, 'message') || 'model stream reported an error'
}

function modelPayloadError(payload: Record<string, unknown>): { message: string; code?: string } | null {
  const rawError = payload.error
  if (typeof rawError === 'string' && rawError.trim()) {
    return { message: rawError.trim() }
  }
  const directError = modelErrorObject(recordValue(payload, 'error'))
  if (directError) return directError
  const responseError = modelErrorObject(recordValue(recordValue(payload, 'response'), 'error'))
  if (responseError) return responseError
  const baseResp = recordValue(payload, 'base_resp') ?? recordValue(payload, 'baseResp')
  if (baseResp) {
    const code = errorCodeString(
      baseResp.status_code ?? baseResp.status ?? baseResp.code ?? baseResp.err_code
    )
    if (code && !successErrorCode(code)) {
      return {
        message:
          recordString(baseResp, 'status_msg') ||
          recordString(baseResp, 'message') ||
          recordString(baseResp, 'msg') ||
          `model provider error (${code})`,
        code
      }
    }
  }
  const topLevelCode = errorCodeString(payload.code ?? payload.type ?? payload.status_code ?? payload.err_code)
  const topLevelMessage =
    recordString(payload, 'message') ||
    recordString(payload, 'error_msg') ||
    recordString(payload, 'status_msg')
  if (topLevelCode && topLevelMessage && !successErrorCode(topLevelCode)) {
    return { message: topLevelMessage, code: topLevelCode }
  }
  return null
}

function modelErrorObject(error: Record<string, unknown> | null): { message: string; code?: string } | null {
  if (!error) return null
  const message =
    recordString(error, 'message') ||
    recordString(error, 'msg') ||
    recordString(error, 'status_msg') ||
    recordString(error, 'error_msg')
  const code = errorCodeString(error.code ?? error.type ?? error.status ?? error.status_code ?? error.err_code)
  if (message) return { message, ...(code ? { code } : {}) }
  if (code && !successErrorCode(code)) return { message: `model provider error (${code})`, code }
  return null
}

function errorCodeString(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}

function successErrorCode(code: string): boolean {
  const normalized = code.trim().toLowerCase()
  return normalized === '0' || normalized === 'ok' || normalized === 'success'
}

function anthropicStopReason(value: unknown): ModelStopReason | undefined {
  if (typeof value !== 'string') return undefined
  switch (value) {
    case 'tool_use':
      return 'tool_calls'
    case 'max_tokens':
      return 'length'
    case 'end_turn':
    case 'stop_sequence':
      return 'stop'
    default:
      return undefined
  }
}

function recordValue(value: unknown, key?: string): Record<string, unknown> | null {
  const target = key === undefined
    ? value
    : value && typeof value === 'object'
      ? (value as Record<string, unknown>)[key]
      : null
  return target && typeof target === 'object' && !Array.isArray(target)
    ? target as Record<string, unknown>
    : null
}

function recordString(value: unknown, key: string): string {
  const target = value && typeof value === 'object'
    ? (value as Record<string, unknown>)[key]
    : undefined
  return typeof target === 'string' ? target : ''
}

function mergeUsageSnapshots(current: UsageSnapshot | null, next: UsageSnapshot): UsageSnapshot {
  if (!current) return next
  const promptTokens = next.promptTokens || current.promptTokens
  const completionTokens = Math.max(next.completionTokens, current.completionTokens)
  const reasoningTokens = Math.max(current.reasoningTokens ?? 0, next.reasoningTokens ?? 0)
  const totalTokens = next.totalTokens > 0 && next.promptTokens > 0
    ? next.totalTokens
    : promptTokens + completionTokens
  const hasCachedTokens = current.cachedTokens !== undefined || next.cachedTokens !== undefined
  const hasCacheHitTokens = current.cacheHitTokens !== undefined || next.cacheHitTokens !== undefined
  const hasCacheMissTokens = current.cacheMissTokens !== undefined || next.cacheMissTokens !== undefined
  return {
    ...current,
    ...next,
    promptTokens,
    completionTokens,
    ...(reasoningTokens > 0 ? { reasoningTokens } : {}),
    totalTokens,
    cachedTokens: hasCachedTokens ? Math.max(current.cachedTokens ?? 0, next.cachedTokens ?? 0) : undefined,
    cacheHitTokens: hasCacheHitTokens ? Math.max(current.cacheHitTokens ?? 0, next.cacheHitTokens ?? 0) : undefined,
    cacheMissTokens: hasCacheMissTokens ? Math.max(current.cacheMissTokens ?? 0, next.cacheMissTokens ?? 0) : undefined,
    cacheHitRate: next.cacheHitRate ?? current.cacheHitRate,
    costUsd: next.costUsd ?? current.costUsd,
    costCny: next.costCny ?? current.costCny
  }
}

function applyReasoningEffort(
  body: Record<string, unknown>,
  effort: string | undefined,
  options: {
    includeThinking?: boolean
    nativeDeepSeekHost?: boolean
    reasoning?: ModelReasoningCapability
    maxReasoningEffort?: 'high' | 'max'
  } = {}
): void {
  const normalized = options.reasoning
    ? resolveReasoningEffort(effort, options.reasoning)
    : normalizeReasoningEffortValue(effort)
  if (!normalized) return
  const includeThinking = options.includeThinking !== false
  // thinking field in DeepSeek format is only supported on the official DeepSeek API.
  // Third-party OpenAI-compat proxies (SiliconFlow, OpenRouter, llama.cpp, etc.) may
  // reject or mishandle it, causing 400 errors or empty responses. See issue #26.
  const nativeDeepSeek = options.nativeDeepSeekHost === true
  if (options.reasoning) {
    applyProfileReasoningEffort(body, normalized, options.reasoning, includeThinking, nativeDeepSeek)
    return
  }
  switch (normalized) {
    case 'off':
      if (includeThinking) body.thinking = { type: 'disabled' }
      break
    case 'low':
    case 'medium':
    case 'high':
      body.reasoning_effort = 'high'
      if (nativeDeepSeek) body.thinking = { type: 'enabled' }
      break
    case 'max':
      body.reasoning_effort = options.maxReasoningEffort ?? 'max'
      if (nativeDeepSeek) body.thinking = { type: 'enabled' }
      break
  }
}

function applyProfileReasoningEffort(
  body: Record<string, unknown>,
  effort: NormalizedReasoningEffort,
  reasoning: ModelReasoningCapability,
  includeThinking: boolean,
  nativeDeepSeekHost: boolean
): void {
  switch (reasoning.requestProtocol) {
    case 'none':
    case 'openai-responses':
    case 'anthropic-thinking':
      return
    case 'deepseek-chat-completions':
      applyDeepSeekChatReasoningEffort(body, effort, nativeDeepSeekHost)
      return
    case 'glm-chat-completions':
      applyGlmChatReasoningEffort(body, effort, includeThinking)
      return
    case 'mimo-chat-completions':
      applyMimoChatReasoningEffort(body, effort, includeThinking)
      return
  }
}

function applyDeepSeekChatReasoningEffort(
  body: Record<string, unknown>,
  effort: NormalizedReasoningEffort,
  includeThinking: boolean
): void {
  if (effort === 'off') {
    if (includeThinking) body.thinking = { type: 'disabled' }
    return
  }
  if (effort === 'max') {
    body.reasoning_effort = 'max'
  } else if (effort !== 'auto') {
    body.reasoning_effort = 'high'
  }
  if (includeThinking && effort !== 'auto') body.thinking = { type: 'enabled' }
}

function applyGlmChatReasoningEffort(
  body: Record<string, unknown>,
  effort: NormalizedReasoningEffort,
  includeThinking: boolean
): void {
  if (!includeThinking || effort === 'auto') return
  body.thinking = {
    type: effort === 'off' ? 'disabled' : 'enabled',
    clear_thinking: true
  }
}

function applyMimoChatReasoningEffort(
  body: Record<string, unknown>,
  effort: NormalizedReasoningEffort,
  includeThinking: boolean
): void {
  if (effort === 'off') {
    if (includeThinking) body.thinking = { type: 'disabled' }
    return
  }
  if (effort === 'low' || effort === 'medium' || effort === 'high') {
    body.reasoning_effort = effort
    if (includeThinking) body.thinking = { type: 'enabled' }
  }
}

function applyAnthropicReasoningEffort(
  body: Record<string, unknown>,
  effort: string | undefined,
  reasoning?: ModelReasoningCapability
): void {
  if (reasoning?.requestProtocol !== 'anthropic-thinking') return
  const resolved = resolveReasoningEffort(effort, reasoning)
  if (!resolved) return
  if (resolved === 'off') {
    body.thinking = { type: 'disabled' }
    return
  }
  body.thinking = { type: 'adaptive' }
  const outputEffort = anthropicOutputEffortForReasoningEffort(resolved)
  if (outputEffort) body.output_config = { effort: outputEffort }
}

function anthropicOutputEffortForReasoningEffort(
  effort: NormalizedReasoningEffort
): 'low' | 'medium' | 'high' | 'max' | null {
  switch (effort) {
    case 'low':
    case 'medium':
    case 'high':
    case 'max':
      return effort
    case 'auto':
    case 'off':
      return null
  }
}

function resolveReasoningEffort(
  effort: string | undefined,
  reasoning: ModelReasoningCapability
): NormalizedReasoningEffort | undefined {
  const normalized = normalizeReasoningEffortValue(effort)
  if (!normalized) return undefined
  if (reasoning.supportedEfforts.includes(normalized)) return normalized
  if (
    normalized === 'low' &&
    reasoning.supportedEfforts.includes('off') &&
    !reasoning.supportedEfforts.includes('low')
  ) {
    return 'off'
  }
  return reasoning.defaultEffort
}

function normalizeReasoningEffortValue(effort: string | undefined): NormalizedReasoningEffort | undefined {
  switch (effort?.trim().toLowerCase()) {
    case 'auto':
    case 'adaptive':
      return 'auto'
    case 'off':
    case 'disabled':
    case 'none':
    case 'false':
      return 'off'
    case 'low':
    case 'minimal':
      return 'low'
    case 'medium':
    case 'mid':
      return 'medium'
    case 'high':
      return 'high'
    case 'max':
    case 'maximum':
    case 'xhigh':
      return 'max'
    default:
      return undefined
  }
}

function shouldRetryWithoutStreamUsage(
  status: number,
  text: string,
  body: Record<string, unknown>
): boolean {
  if (status !== 400 && status !== 422) return false
  if (!Object.prototype.hasOwnProperty.call(body, 'stream_options')) return false
  return /\b(stream_options|include_usage)\b/i.test(text)
}

function isAzureOpenAiEndpoint(baseUrl: string): boolean {
  try {
    const url = new URL(baseUrl)
    const host = url.hostname.toLowerCase()
    return host.endsWith('.openai.azure.com') || host.endsWith('.cognitiveservices.azure.com')
  } catch {
    return /\.openai\.azure\.com\b|\.cognitiveservices\.azure\.com\b/i.test(baseUrl)
  }
}

function isThinkingMode(effort: string | undefined): boolean {
  const normalized = effort?.trim().toLowerCase()
  if (!normalized) return false
  return !['off', 'disabled', 'none', 'false'].includes(normalized)
}

function requiresReasoningRoundTrip(
  effort: string | undefined,
  model: string | undefined,
  baseUrl: string,
  reasoning?: ModelReasoningCapability
): boolean {
  if (reasoning) {
    const resolved = resolveReasoningEffort(effort, reasoning)
    if (resolved) {
      return resolved !== 'off' && reasoning.requestProtocol !== 'none'
    }
    return isDeepSeekHost(baseUrl) && isThinkingProducerModel(model)
  }
  // Thinking-mode round trip is a DeepSeek-specific protocol extension.
  // OpenAI-compat providers (OpenRouter, llama.cpp, etc.) may reject
  // or misinterpret the `thinking` field, so we only auto-enable it
  // on the official DeepSeek host. User-selected reasoningEffort still
  // forces the path (opt-in). See issue #26.
  return isThinkingMode(effort) || (isDeepSeekHost(baseUrl) && isThinkingProducerModel(model))
}

function isThinkingProducerModel(model: string | undefined): boolean {
  const normalized = normalizeModelId(model)
  if (!normalized) return false
  return normalized === 'deepseek-v4-pro' ||
    normalized === 'deepseek-v4-flash' ||
    normalized.includes('deepseek-reasoner') ||
    normalized.endsWith('/deepseek-v4-pro') ||
    normalized.endsWith('/deepseek-v4-flash')
}

function reasoningContentOrSpace(text: string): string {
  return text.trim() ? text : ' '
}

function toolResultContent(output: unknown): string {
  if (typeof output === 'string') return output
  return JSON.stringify(output) ?? ''
}

function reasoningFromMessage(message: ChatCompletionResponse['choices'][number]['message'] | undefined): string {
  if (!message) return ''
  const value = message.reasoning_content ??
    (message as ChatMessage & { reasoning?: unknown }).reasoning
  return typeof value === 'string' ? value : ''
}

function isPreToolCallBridgeItem(item: ModelHistoryItem, turnId: string): boolean {
  if (item.turnId !== turnId) return false
  return item.kind === 'assistant_reasoning' || item.kind === 'assistant_text'
}

function isBridgeItemBeforeToolCall(items: ModelHistoryItem[], index: number): boolean {
  const item = items[index]
  if (!item || (item.kind !== 'assistant_reasoning' && item.kind !== 'assistant_text')) {
    return false
  }
  let cursor = index + 1
  while (cursor < items.length) {
    const next = items[cursor]
    if (!next) return false
    if (next.kind === 'assistant_reasoning' || next.kind === 'assistant_text') {
      if (next.turnId !== item.turnId) return false
      cursor += 1
      continue
    }
    return next.kind === 'tool_call' && next.turnId === item.turnId
  }
  return false
}

function normalizeThinkingAssistantMessages(
  messages: ChatMessage[],
  thinkingMode: boolean
): ChatMessage[] {
  if (!thinkingMode) return messages
  return messages.map((message) => {
    if (message.role !== 'assistant') return message
    const next = { ...message }
    if (next.content == null) next.content = ''
    const hasToolCalls = Array.isArray(next.tool_calls) && next.tool_calls.length > 0
    if (!hasToolCalls) {
      delete next.reasoning_content
      return next
    }
    if (
      !Object.prototype.hasOwnProperty.call(next, 'reasoning_content') ||
      next.reasoning_content == null ||
      !next.reasoning_content.trim()
    ) {
      next.reasoning_content = ' '
    }
    return next
  })
}

function normalizeModelId(model: string | undefined): string {
  return model?.trim().toLowerCase() ?? ''
}

function normalizeStreamIdleTimeoutMs(value: number | undefined): number {
  if (value === undefined) return DEFAULT_STREAM_IDLE_TIMEOUT_MS
  if (!Number.isFinite(value)) return DEFAULT_STREAM_IDLE_TIMEOUT_MS
  return Math.max(0, Math.floor(value))
}

async function readStreamChunk(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  signal: AbortSignal,
  idleTimeoutMs: number
): Promise<StreamReadResult> {
  if (signal.aborted) return { kind: 'aborted' }
  let timeout: ReturnType<typeof setTimeout> | undefined
  let cleanupAbort: (() => void) | undefined
  const readPromise = reader.read()
    .then((result): StreamReadResult => ({ kind: 'chunk', ...result }))
    .catch((error): StreamReadResult => {
      if (signal.aborted) return { kind: 'aborted' }
      const message = error instanceof Error ? error.message : String(error)
      return { kind: 'error', message: `model stream read failed: ${message}` }
    })
  const abortPromise = new Promise<StreamReadResult>((resolve) => {
    const onAbort = (): void => resolve({ kind: 'aborted' })
    if (signal.aborted) {
      resolve({ kind: 'aborted' })
      return
    }
    signal.addEventListener('abort', onAbort, { once: true })
    cleanupAbort = () => signal.removeEventListener('abort', onAbort)
  })
  const candidates: Array<Promise<StreamReadResult>> = [readPromise, abortPromise]
  if (idleTimeoutMs > 0) {
    candidates.push(new Promise<StreamReadResult>((resolve) => {
      timeout = setTimeout(() => resolve({ kind: 'timeout' }), idleTimeoutMs)
    }))
  }
  const result = await Promise.race(candidates)
  if (timeout) clearTimeout(timeout)
  cleanupAbort?.()
  if (result.kind === 'timeout') {
    try {
      await reader.cancel('model stream idle timeout')
    } catch {
      // Best-effort cancellation; the caller will surface the timeout.
    }
  }
  return result
}

function resolveToolCallDeltaId(
  call: { index?: number; id?: string },
  pending: Map<string, PendingToolCall>
): string {
  const index = numericIndex(call.index)
  const existingByIndex = findPendingToolCallIdByIndex(pending, index)
  if (existingByIndex) return existingByIndex
  if (call.id) {
    return call.id
  }
  return existingByIndex ?? `call_${pending.size + 1}`
}

function findPendingToolCallIdByIndex(
  pending: Map<string, PendingToolCall>,
  index: number | undefined
): string | undefined {
  if (index === undefined) return undefined
  for (const [callId, value] of pending) {
    if (value.index === index) return callId
  }
  return undefined
}

function numericIndex(index: unknown): number | undefined {
  return typeof index === 'number' && Number.isInteger(index) && index >= 0
    ? index
    : undefined
}

function healToolMessagePairs(messages: ChatMessage[]): ChatMessage[] {
  const healed: ChatMessage[] = []
  for (let i = 0; i < messages.length; i += 1) {
    const message = messages[i]
    if (message.role === 'tool') {
      continue
    }
    if (message.role === 'assistant' && message.tool_calls?.length) {
      const expectedIds = new Set(message.tool_calls.map((call) => call.id))
      const toolResults: ChatMessage[] = []
      let j = i + 1
      while (j < messages.length && messages[j].role === 'tool') {
        const toolResult = messages[j]
        if (toolResult.tool_call_id && expectedIds.has(toolResult.tool_call_id)) {
          toolResults.push(toolResult)
        }
        j += 1
      }
      const seenIds = new Set(toolResults.map((toolResult) => toolResult.tool_call_id))
      healed.push(message, ...toolResults)
      for (const id of expectedIds) {
        if (!seenIds.has(id)) {
          healed.push({
            role: 'tool',
            content: INTERRUPTED_TOOL_RESULT_PLACEHOLDER,
            tool_call_id: id
          })
        }
      }
      i = j - 1
      continue
    }
    healed.push(message)
  }
  return healed
}

function modelUserMessageText(item: Extract<ModelHistoryItem, { kind: 'user_message' }>): string {
  if (item.delivery !== 'steer') return item.text
  const text = item.text.trim()
  if (!text) return item.text
  if (text.startsWith(MID_TURN_STEERING_PREFIX)) return item.text
  return `${MID_TURN_STEERING_PREFIX}\n\n${text}`
}

function attachImagesToLatestUserMessage(
  messages: ChatMessage[],
  attachments: NonNullable<ModelRequest['attachments']>
): void {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message.role !== 'user') continue
    const parts: ChatMessageContentPart[] = []
    if (typeof message.content === 'string' && message.content) {
      parts.push({ type: 'text', text: message.content })
    }
    for (const attachment of attachments) {
      parts.push({
        type: 'image_url',
        image_url: {
          url: `data:${attachment.mimeType};base64,${attachment.dataBase64}`
        }
      })
    }
    message.content = parts
    return
  }
}

function attachTextFallbacksToLatestUserMessage(
  messages: ChatMessage[],
  attachments: NonNullable<ModelRequest['attachmentTextFallbacks']>
): void {
  const text = attachments.map(formatAttachmentTextFallback).join('\n\n')
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message.role !== 'user') continue
    if (typeof message.content === 'string') {
      message.content = message.content ? `${message.content}\n\n${text}` : text
      return
    }
    if (Array.isArray(message.content)) {
      message.content.push({ type: 'text', text })
      return
    }
    message.content = text
    return
  }
}

function formatAttachmentTextFallback(
  attachment: NonNullable<ModelRequest['attachmentTextFallbacks']>[number]
): string {
  return [
    '[Attached image as base64 text]',
    `Name: ${attachment.name}`,
    `FilePath: ${attachment.localFilePath ?? 'unknown'}`,
    `MIME: ${attachment.mimeType}`,
    `Dimensions: ${formatAttachmentDimensions(attachment)}`,
    `Bytes: ${attachment.byteSize}`,
    'Base64:',
    '```base64',
    attachment.dataBase64,
    '```',
    '[/Attached image]'
  ].join('\n')
}

function formatAttachmentDimensions(
  attachment: NonNullable<ModelRequest['attachmentTextFallbacks']>[number]
): string {
  return attachment.width && attachment.height ? `${attachment.width}x${attachment.height}` : 'unknown'
}

function limitHistoryPreservingCompaction(history: ModelHistoryItem[], windowSize: number): ModelHistoryItem[] {
  if (history.length <= windowSize) return history
  const windowStart = history.length - windowSize
  const limited = history.slice(windowStart)
  if (limited.some((item) => item.kind === 'compaction' && (item.replacedTokens ?? 0) > 0)) {
    return limited
  }
  for (let index = windowStart - 1; index >= 0; index -= 1) {
    const item = history[index]
    if (item.kind !== 'compaction' || (item.replacedTokens ?? 0) === 0) continue
    return windowSize <= 1 ? [item] : [item, ...history.slice(-(windowSize - 1))]
  }
  return limited
}

// Transient upstream gateway statuses worth retrying: load balancers and
// reverse proxies return these for momentary backend unavailability.
const TRANSIENT_RETRY_STATUSES = new Set([502, 503, 504])
const MAX_TRANSIENT_RETRIES = 2
const TRANSIENT_RETRY_BASE_MS = 500

function sleepWithAbort(ms: number, signal: AbortSignal): Promise<boolean> {
  if (signal.aborted) return Promise.resolve(true)
  return new Promise((resolve) => {
    let settled = false
    const finish = (aborted: boolean): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      resolve(aborted)
    }
    const onAbort = (): void => finish(true)
    const timer = setTimeout(() => finish(false), ms)
    signal.addEventListener('abort', onAbort, { once: true })
  })
}
