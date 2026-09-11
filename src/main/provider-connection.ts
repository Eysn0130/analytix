import {
  isCustomModelEndpointFormat,
  modelEndpointPath,
  normalizeModelEndpointFormat,
  resolveModelProviderProxyUrl,
  type AppSettingsV1,
  type ModelCapabilityProbeStatus,
  type ModelEndpointFormat
} from '../shared/app-settings'
import { modelCapabilityProbeKey } from '../shared/app-settings-provider'
import type {
  ModelCapabilityProbeRequest,
  ModelCapabilityProbeResult
} from '../shared/analytix-api'
import { upstreamOpenAiModelsUrl } from '../shared/openai-compat-url'
import { redactSecretText } from '../shared/secret-redaction'
import { fetchWithOptionalProxy } from './proxy-fetch'

const PROBE_TIMEOUT_MS = 10_000
const ANTHROPIC_VERSION = '2023-06-01'
const CAPABILITY_PROBE_STALE_MS = 24 * 60 * 60 * 1000
const PROVIDER_CREDENTIAL_UNAVAILABLE = 'Provider credential resolution is not available.'
const SEMANTIC_PROBE_IMAGE_DATA_URL =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAAQ0lEQVR4AcXBAQEAAAiDMKR/5xuD7QYjJDGJSUxiEpOYxCQmMYlJTGISk5jEJCYxiUlMYhKTmMQkJjGJSUxiEpOYxB4w4wI+9B/igQAAAABJRU5ErkJggg=='
const SEMANTIC_PROBE_COLOR = 'red'
const SEMANTIC_PROBE_SHAPE = 'square'
const SEMANTIC_PROBE_COLOR_ALIASES = ['red', 'crimson', 'scarlet', '红', '红色']
const SEMANTIC_PROBE_SHAPE_ALIASES = [
  SEMANTIC_PROBE_SHAPE,
  'rectangle',
  'rectangular',
  'box',
  'quad',
  'quadrilateral',
  '方形',
  '矩形',
  '正方形',
  '四边形'
]
const TOOL_PROBE_MARKER = 'AX-731'

export function providerProbeHeaders(
  endpointFormat: ModelEndpointFormat,
  apiKey: string
): Record<string, string> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const key = apiKey.trim()
  if (endpointFormat === 'messages') {
    headers['anthropic-version'] = ANTHROPIC_VERSION
    if (key) headers['x-api-key'] = key
    return headers
  }
  if (key) headers.Authorization = `Bearer ${key}`
  return headers
}

/**
 * Probe the exact provider/model wire path used by the runtime. This does not
 * trust preset names: a model is marked native-vision only when the current
 * endpoint accepts image input, tool schemas, and a next-round screenshot.
 */
export async function probeModelCapabilities(
  request: ModelCapabilityProbeRequest,
  settings?: AppSettingsV1
): Promise<ModelCapabilityProbeResult> {
  const baseUrl = request.baseUrl.trim()
  const endpointFormat = normalizeModelEndpointFormat(request.endpointFormat)
  const providerId = request.providerId.trim()
  const model = request.model.trim()
  const probedAt = new Date()
  const staleAfter = new Date(probedAt.getTime() + CAPABILITY_PROBE_STALE_MS)
  const key = modelCapabilityProbeKey({ providerId, model, baseUrl, endpointFormat })
  const baseResult = {
    key,
    providerId,
    model,
    endpointFormat,
    sanitizedBaseUrl: sanitizeUrlForProbe(baseUrl),
    probedAt: probedAt.toISOString(),
    staleAfter: staleAfter.toISOString()
  }
  if (!/^https?:\/\//i.test(baseUrl)) {
    return {
      ...baseResult,
      ok: false,
      imageInput: 'failed',
      toolCalling: 'failed',
      toolResultImage: 'failed',
      status: 'failed',
      errorSummary: 'Base URL must start with http:// or https://.',
      message: providerCapabilityProbeDiagnostic({
        baseUrl,
        endpointFormat,
        providerId,
        model,
        errorSummary: 'Base URL must start with http:// or https://.'
      })
    }
  }
  return {
    ...baseResult,
    ok: false,
    imageInput: 'failed',
    toolCalling: 'failed',
    toolResultImage: 'failed',
    status: 'failed',
    errorSummary: PROVIDER_CREDENTIAL_UNAVAILABLE,
    message: providerCapabilityProbeDiagnostic({
      baseUrl,
      endpointFormat,
      providerId,
      model,
      errorSummary: PROVIDER_CREDENTIAL_UNAVAILABLE
    })
  }
}

function providerCapabilityProbeDiagnostic(input: {
  baseUrl: string
  endpointFormat: ModelEndpointFormat
  providerId: string
  model: string
  requestUrl?: string
  status?: number
  errorSummary?: string
}): string {
  const lines = [
    `Provider: ${input.providerId}`,
    `Model: ${input.model}`,
    `Base URL: ${sanitizeUrlForProbe(input.baseUrl)}`,
    `Endpoint format: ${input.endpointFormat}`
  ]
  if (input.requestUrl) lines.push(`Request URL: ${sanitizeUrlForProbe(input.requestUrl)}`)
  if (typeof input.status === 'number') lines.push(`HTTP status: ${input.status}`)
  if (input.errorSummary) lines.push(`Error: ${redactSecretText(input.errorSummary)}`)
  return lines.join('\n')
}

function modelCapabilityProbeUrl(baseUrl: string, endpointFormat: ModelEndpointFormat): string {
  if (isCustomModelEndpointFormat(endpointFormat)) return exactEndpointUrl(baseUrl)
  const path = modelEndpointPath(endpointFormat)
  const normalized = baseUrl.trim().replace(/\/+$/, '')
  const lastSegment = normalized.split('/').pop()?.toLowerCase() ?? ''
  if (lastSegment === 'beta') return `${normalized.slice(0, -'/beta'.length)}/v1/${path}`
  if (/^v\d+$/.test(lastSegment)) return `${normalized}/${path}`
  return `${normalized}/v1/${path}`
}

function exactEndpointUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim()
  const query = trimmed.search(/[?#]/)
  if (query < 0) return trimmed.replace(/\/+$/, '')
  return `${trimmed.slice(0, query).replace(/\/+$/, '')}${trimmed.slice(query)}`
}

type CapabilityProbePostResult =
  | { kind: 'ok'; text: string; httpStatus: number }
  | { kind: 'failed'; status: ModelCapabilityProbeStatus; errorSummary: string; httpStatus?: number }

type CapabilityToolCall = {
  id: string
  name: string
  arguments: string
}

async function postCapabilityProbe(input: {
  url: string
  endpointFormat: ModelEndpointFormat
  apiKey: string
  body: Record<string, unknown>
  proxyUrl: string
}): Promise<CapabilityProbePostResult> {
  let res: Response
  let text = ''
  try {
    res = await fetchWithOptionalProxy(input.url, {
      method: 'POST',
      headers: {
        ...providerProbeHeaders(input.endpointFormat, input.apiKey),
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(input.body),
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS)
    }, input.proxyUrl)
    text = await res.text()
  } catch (error) {
    const timeout = error instanceof Error && error.name === 'TimeoutError'
    return {
      kind: 'failed',
      status: timeout ? 'timeout' : 'failed',
      errorSummary: timeout
        ? `Request timed out after ${PROBE_TIMEOUT_MS / 1_000}s.`
        : redactSecretText(error instanceof Error ? error.message : String(error))
    }
  }
  if (!res.ok) {
    return {
      kind: 'failed',
      status: capabilityHttpFailureStatus(res.status),
      httpStatus: res.status,
      errorSummary: redactSecretText(text.slice(0, 500).replace(/\s+/g, ' ').trim())
    }
  }
  return { kind: 'ok', text, httpStatus: res.status }
}

function capabilityImageProbeBody(providerId: string, model: string, endpointFormat: ModelEndpointFormat): Record<string, unknown> {
  if (endpointFormat === 'responses') {
    return {
      model,
      stream: false,
      input: [
        {
          role: 'user',
          content: [
            { type: 'input_text', text: semanticImagePrompt() },
            { type: 'input_image', image_url: SEMANTIC_PROBE_IMAGE_DATA_URL }
          ]
        }
      ],
      max_output_tokens: 48
    }
  }
  if (endpointFormat === 'messages') {
    return {
      model,
      stream: false,
      max_tokens: 48,
      messages: [
        {
          role: 'user',
          content: [
            { type: 'text', text: semanticImagePrompt() },
            semanticAnthropicImage()
          ]
        }
      ]
    }
  }
  return {
    model,
    stream: false,
    ...chatCompletionsProbeReasoningControl(providerId, model),
    messages: [
      {
        role: 'user',
        content: [
          { type: 'text', text: semanticImagePrompt() },
          { type: 'image_url', image_url: { url: SEMANTIC_PROBE_IMAGE_DATA_URL } }
        ]
      }
    ],
    max_tokens: chatCompletionsProbeMaxTokens(providerId, model, 48)
  }
}

function capabilityToolCallProbeBody(providerId: string, model: string, endpointFormat: ModelEndpointFormat): Record<string, unknown> {
  const prompt = `Call the capability_probe tool exactly once with {"marker":"${TOOL_PROBE_MARKER}"}. Do not answer in text.`
  if (endpointFormat === 'responses') {
    return {
      model,
      stream: false,
      input: [{ role: 'user', content: [{ type: 'input_text', text: prompt }] }],
      tools: [capabilityProbeResponsesTool()],
      max_output_tokens: 64
    }
  }
  if (endpointFormat === 'messages') {
    return {
      model,
      stream: false,
      max_tokens: 64,
      messages: [{ role: 'user', content: [{ type: 'text', text: prompt }] }],
      tools: [capabilityProbeMessagesTool()]
    }
  }
  return {
    model,
    stream: false,
    ...chatCompletionsProbeReasoningControl(providerId, model),
    messages: [{ role: 'user', content: prompt }],
    tools: [capabilityProbeChatTool()],
    max_tokens: 64
  }
}

function capabilityToolResultImageProbeBody(
  providerId: string,
  model: string,
  endpointFormat: ModelEndpointFormat,
  call: CapabilityToolCall
): Record<string, unknown> {
  const prompt = 'The previous tool result includes a screenshot image. Return strict JSON only with {"color":string,"shape":string}.'
  if (endpointFormat === 'responses') {
    return {
      model,
      stream: false,
      input: [
        { role: 'user', content: [{ type: 'input_text', text: `Call capability_probe with marker ${TOOL_PROBE_MARKER}.` }] },
        {
          type: 'function_call',
          call_id: call.id,
          name: call.name,
          arguments: call.arguments,
          status: 'completed'
        },
        {
          type: 'function_call_output',
          call_id: call.id,
          output: JSON.stringify({ ok: true, marker: TOOL_PROBE_MARKER })
        },
        {
          role: 'user',
          content: [
            { type: 'input_text', text: prompt },
            { type: 'input_image', image_url: SEMANTIC_PROBE_IMAGE_DATA_URL }
          ]
        }
      ],
      max_output_tokens: 48
    }
  }
  if (endpointFormat === 'messages') {
    return {
      model,
      stream: false,
      max_tokens: 48,
      messages: [
        { role: 'user', content: [{ type: 'text', text: `Call capability_probe with marker ${TOOL_PROBE_MARKER}.` }] },
        {
          role: 'assistant',
          content: [{ type: 'tool_use', id: call.id, name: call.name, input: parseToolArguments(call.arguments) }]
        },
        {
          role: 'user',
          content: [
            { type: 'tool_result', tool_use_id: call.id, content: JSON.stringify({ ok: true, marker: TOOL_PROBE_MARKER }) },
            { type: 'text', text: prompt },
            semanticAnthropicImage()
          ]
        }
      ]
    }
  }
  return {
    model,
    stream: false,
    ...chatCompletionsProbeReasoningControl(providerId, model),
    messages: [
      { role: 'user', content: `Call capability_probe with marker ${TOOL_PROBE_MARKER}.` },
      {
        role: 'assistant',
        content: null,
        tool_calls: [{
          id: call.id,
          type: 'function',
          function: { name: call.name, arguments: call.arguments }
        }]
      },
      { role: 'tool', tool_call_id: call.id, content: JSON.stringify({ ok: true, marker: TOOL_PROBE_MARKER }) },
      {
        role: 'user',
        content: [
          { type: 'text', text: prompt },
          { type: 'image_url', image_url: { url: SEMANTIC_PROBE_IMAGE_DATA_URL } }
        ]
      }
    ],
    max_tokens: chatCompletionsProbeMaxTokens(providerId, model, 48)
  }
}

function chatCompletionsProbeReasoningControl(providerId: string, model: string): Record<string, unknown> {
  const joined = `${providerId} ${model}`.toLowerCase()
  if (joined.includes('xiaomi') || joined.includes('mimo')) {
    return { thinking: { type: 'disabled' } }
  }
  return {}
}

function chatCompletionsProbeMaxTokens(providerId: string, model: string, fallback: number): number {
  const joined = `${providerId} ${model}`.toLowerCase()
  return joined.includes('xiaomi') || joined.includes('mimo') ? Math.max(fallback, 128) : fallback
}

function capabilityProbeChatTool(): Record<string, unknown> {
  return {
    type: 'function',
    function: {
      name: 'capability_probe',
      description: 'Capability probe tool. Echo the marker argument.',
      parameters: {
        type: 'object',
        properties: { marker: { type: 'string' } },
        required: ['marker'],
        additionalProperties: false
      }
    }
  }
}

function capabilityProbeResponsesTool(): Record<string, unknown> {
  return {
    type: 'function',
    name: 'capability_probe',
    description: 'Capability probe tool. Echo the marker argument.',
    parameters: {
      type: 'object',
      properties: { marker: { type: 'string' } },
      required: ['marker'],
      additionalProperties: false
    }
  }
}

function capabilityProbeMessagesTool(): Record<string, unknown> {
  return {
    name: 'capability_probe',
    description: 'Capability probe tool. Echo the marker argument.',
    input_schema: {
      type: 'object',
      properties: { marker: { type: 'string' } },
      required: ['marker'],
      additionalProperties: false
    }
  }
}

function semanticAnthropicImage(): Record<string, unknown> {
  return {
    type: 'image',
    source: {
      type: 'base64',
      media_type: 'image/png',
      data: SEMANTIC_PROBE_IMAGE_DATA_URL.replace(/^data:image\/png;base64,/, '')
    }
  }
}

function semanticImagePrompt(): string {
  return [
    'Inspect the attached image.',
    'Return strict JSON only with exactly these keys: {"color":string,"shape":string}.',
    'Do not infer the answer from the prompt; identify the visible image pixels only.'
  ].join(' ')
}

function extractModelText(body: string): unknown {
  let parsed: unknown
  try {
    parsed = JSON.parse(body) as unknown
  } catch {
    return body
  }
  if (!parsed || typeof parsed !== 'object') return parsed

  const outputText = (parsed as { output_text?: unknown }).output_text
  if (typeof outputText === 'string') return outputText

  const choices = (parsed as { choices?: unknown }).choices
  if (Array.isArray(choices)) {
    for (const choice of choices) {
      const content = choice && typeof choice === 'object'
        ? (choice as { message?: { content?: unknown }; text?: unknown }).message?.content
          ?? (choice as { text?: unknown }).text
        : undefined
      const text = textFromContent(content)
      if (text) return text
    }
  }

  const content = (parsed as { content?: unknown }).content
  const contentText = textFromContent(content)
  if (contentText) return contentText

  const output = (parsed as { output?: unknown }).output
  if (Array.isArray(output)) {
    const parts: string[] = []
    for (const item of output) {
      if (!item || typeof item !== 'object') continue
      const itemText = textFromContent((item as { content?: unknown }).content)
      if (itemText) parts.push(itemText)
      const directText = (item as { text?: unknown }).text
      if (typeof directText === 'string') parts.push(directText)
    }
    if (parts.length > 0) return parts.join('\n')
  }

  return parsed
}

function textFromContent(content: unknown): string {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return ''
  const parts: string[] = []
  for (const item of content) {
    if (!item || typeof item !== 'object') continue
    const record = item as { type?: unknown; text?: unknown; output_text?: unknown }
    if (typeof record.text === 'string') parts.push(record.text)
    if (typeof record.output_text === 'string') parts.push(record.output_text)
  }
  return parts.join('\n')
}

function semanticImageMatches(value: unknown): boolean {
  const parsed = parseJsonObjectFromUnknown(value)
  if (parsed) {
    const color = String(parsed.color ?? parsed.colour ?? parsed.dominant_color ?? '').toLowerCase()
    const shape = String(parsed.shape ?? parsed.object ?? parsed.form ?? '').toLowerCase()
    return containsSemanticProbeColor(color) && containsSemanticProbeShape(shape)
  }
  if (typeof value !== 'string') return false
  const lower = value.toLowerCase()
  return containsSemanticProbeColor(lower) && containsSemanticProbeShape(lower)
}

function containsSemanticProbeColor(value: string): boolean {
  return containsAny(value, SEMANTIC_PROBE_COLOR_ALIASES)
}

function containsSemanticProbeShape(value: string): boolean {
  return containsAny(value, SEMANTIC_PROBE_SHAPE_ALIASES)
}

function containsAny(value: string, aliases: readonly string[]): boolean {
  return aliases.some((alias) => value.includes(alias))
}

function parseJsonObjectFromUnknown(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  const candidates = [trimmed]
  const fenced = /```(?:json)?\s*([\s\S]*?)\s*```/i.exec(trimmed)
  if (fenced?.[1]) candidates.push(fenced[1].trim())
  const objectMatch = /\{[\s\S]*\}/.exec(trimmed)
  if (objectMatch?.[0]) candidates.push(objectMatch[0])
  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate) as unknown
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
    } catch {
      // Try the next candidate.
    }
  }
  return null
}

function extractCapabilityToolCall(body: string, endpointFormat: ModelEndpointFormat): CapabilityToolCall | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(body) as unknown
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object') return null

  if (endpointFormat === 'responses') {
    const output = (parsed as { output?: unknown }).output
    if (!Array.isArray(output)) return null
    for (const item of output) {
      if (!item || typeof item !== 'object') continue
      const record = item as { type?: unknown; id?: unknown; call_id?: unknown; name?: unknown; arguments?: unknown }
      if (record.type !== 'function_call' || record.name !== 'capability_probe') continue
      const args = typeof record.arguments === 'string' ? record.arguments : JSON.stringify(record.arguments ?? {})
      if (!toolArgumentsMatch(args)) continue
      return {
        id: String(record.call_id ?? record.id ?? 'capability_probe_call'),
        name: 'capability_probe',
        arguments: args
      }
    }
    return null
  }

  if (endpointFormat === 'messages') {
    const content = (parsed as { content?: unknown }).content
    if (!Array.isArray(content)) return null
    for (const item of content) {
      if (!item || typeof item !== 'object') continue
      const record = item as { type?: unknown; id?: unknown; name?: unknown; input?: unknown }
      if (record.type !== 'tool_use' || record.name !== 'capability_probe') continue
      const args = JSON.stringify(record.input ?? {})
      if (!toolArgumentsMatch(args)) continue
      return {
        id: String(record.id ?? 'capability_probe_call'),
        name: 'capability_probe',
        arguments: args
      }
    }
    return null
  }

  const choices = (parsed as { choices?: unknown }).choices
  if (!Array.isArray(choices)) return null
  for (const choice of choices) {
    if (!choice || typeof choice !== 'object') continue
    const calls = (choice as { message?: { tool_calls?: unknown } }).message?.tool_calls
    if (!Array.isArray(calls)) continue
    for (const call of calls) {
      if (!call || typeof call !== 'object') continue
      const record = call as {
        id?: unknown
        function?: { name?: unknown; arguments?: unknown }
      }
      if (record.function?.name !== 'capability_probe') continue
      const args = typeof record.function.arguments === 'string'
        ? record.function.arguments
        : JSON.stringify(record.function.arguments ?? {})
      if (!toolArgumentsMatch(args)) continue
      return {
        id: String(record.id ?? 'capability_probe_call'),
        name: 'capability_probe',
        arguments: args
      }
    }
  }
  return null
}

function toolArgumentsMatch(args: string): boolean {
  const parsed = parseToolArguments(args)
  return String((parsed as { marker?: unknown }).marker ?? '') === TOOL_PROBE_MARKER
}

function parseToolArguments(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>
  if (typeof value !== 'string') return {}
  try {
    const parsed = JSON.parse(value) as unknown
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : {}
  } catch {
    return {}
  }
}

function probeFailureStatus(
  result: CapabilityProbePostResult,
  _capability: 'imageInput' | 'toolCalling' | 'toolResultImage'
): ModelCapabilityProbeStatus {
  return result.kind === 'failed' ? result.status : 'semantic_failed'
}

function aggregateProbeStatus(statuses: ModelCapabilityProbeStatus[]): ModelCapabilityProbeStatus {
  if (statuses.every((status) => status === 'supported')) return 'supported'
  for (const status of ['auth_failed', 'timeout', 'http_failed', 'unsupported', 'semantic_failed', 'failed'] as const) {
    if (statuses.includes(status)) return status
  }
  return 'unknown'
}

function summarizeProbeFailure(input: {
  imageInput: ModelCapabilityProbeStatus
  toolCalling: ModelCapabilityProbeStatus
  toolResultImage: ModelCapabilityProbeStatus
}): string {
  return [
    `imageInput=${input.imageInput}`,
    `toolCalling=${input.toolCalling}`,
    `toolResultImage=${input.toolResultImage}`
  ].join('; ')
}

function capabilityHttpFailureStatus(status: number): ModelCapabilityProbeStatus {
  if (status === 401 || status === 403) return 'auth_failed'
  if (isCapabilityUnsupportedStatus(status)) return 'unsupported'
  return 'http_failed'
}

function isCapabilityUnsupportedStatus(status: number): boolean {
  return status === 400 || status === 404 || status === 415 || status === 422
}

function sanitizeUrlForProbe(value: string): string {
  return redactSecretText(value.trim().replace(/\/+$/, ''))
}

function parseModelIds(body: string): string[] {
  let parsed: unknown
  try {
    parsed = JSON.parse(body) as unknown
  } catch {
    return []
  }
  const data = (parsed as { data?: unknown }).data
  if (!Array.isArray(data)) return []
  const ids = new Set<string>()
  for (const row of data) {
    if (row && typeof row === 'object' && typeof (row as { id?: unknown }).id === 'string') {
      const id = (row as { id: string }).id.trim()
      if (id) ids.add(id)
    }
  }
  return [...ids]
}
