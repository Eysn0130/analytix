import type { AnalytixCapabilitiesConfig, WebCapabilityConfig } from '../../contracts/capabilities.js'
import type { WebFetchResult, WebProvider, WebSearchResult } from '../../ports/web-provider.js'
import { sourceIdFor, UnavailableWebProvider } from '../../ports/web-provider.js'
import type { CapabilityToolProvider } from './capability-registry.js'
import { LocalToolHost } from './local-tool-host.js'

const DEFAULT_WEB_TIMEOUT_MS = 15_000
const DEFAULT_WEB_MAX_BYTES = 1_000_000
// Models sometimes pass tiny max_bytes budgets (2000 was common in the
// wild); below this floor the extracted text is too small to be useful.
const MIN_WEB_FETCH_BYTES = 4_096
const DEFAULT_SEARCH_LIMIT = 5
const MAX_SEARCH_LIMIT = 10

export type WebProviderDiagnostic = {
  id: string
  enabled: boolean
  available: boolean
  fetchAvailable: boolean
  searchAvailable: boolean
  provider?: string
  reason?: string
}

export type WebToolProviderBuildResult = {
  providers: CapabilityToolProvider[]
  diagnostics: WebProviderDiagnostic[]
  fetchAvailable: boolean
  searchAvailable: boolean
  provider?: string
}

export type WebToolProviderOptions = {
  provider?: WebProvider
  nowIso?: () => string
}

export function buildWebToolProviders(
  config: AnalytixCapabilitiesConfig['web'] | undefined,
  options: WebToolProviderOptions = {}
): WebToolProviderBuildResult {
  const web = config
  if (!web?.enabled) {
    return {
      providers: [],
      diagnostics: [],
      fetchAvailable: false,
      searchAvailable: false
    }
  }

  const provider: WebProvider = options.provider ?? (web.fetchEnabled ? new FetchWebProvider(options.nowIso) : new UnavailableWebProvider(web.provider))
  const tools = []
  if (web.fetchEnabled) {
    tools.push(createFetchTool(web, provider))
  }
  if (web.searchEnabled) {
    tools.push(createSearchTool(web, provider))
  }
  const fetchAvailable = Boolean(web.fetchEnabled && provider.fetch)
  const searchAvailable = Boolean(web.searchEnabled && provider.search)
  const reason = !tools.length
    ? 'web tools are disabled by config'
    : !fetchAvailable && !searchAvailable
      ? 'web provider is unavailable'
      : undefined

  return {
    providers: tools.length
      ? [{
          id: 'web',
          kind: 'web',
          enabled: true,
          available: true,
          ...(reason ? { reason } : {}),
          tools
        }]
      : [],
    diagnostics: [{
      id: 'web',
      enabled: true,
      available: fetchAvailable || searchAvailable,
      fetchAvailable,
      searchAvailable,
      provider: provider.id,
      ...(reason ? { reason } : {})
    }],
    fetchAvailable,
    searchAvailable,
    provider: provider.id
  }
}

function createFetchTool(config: WebCapabilityConfig, provider: WebProvider) {
  return LocalToolHost.defineTool({
    name: 'web_fetch',
    description: 'Fetch an allowed HTTP or HTTPS URL and return extracted text with source metadata.',
    inputSchema: {
      type: 'object',
      properties: {
        url: { type: 'string' },
        max_bytes: { type: 'number' },
        timeout_ms: { type: 'number' }
      },
      required: ['url'],
      additionalProperties: false
    },
    policy: 'untrusted',
    execute: async (args, context) => {
      const startedAt = Date.now()
      const rawUrl = pickString(args.url)
      if (!rawUrl) return toolError('invalid_url', 'url is required')
      const policy = validateUrlPolicy(rawUrl, config)
      if (!policy.ok) return toolError('policy_blocked', policy.reason, telemetry({ startedAt, policy: 'blocked', url: rawUrl }))
      if (!provider.fetch) return toolError('provider_unavailable', 'web fetch provider is unavailable')
      const maxBytesCap = config.maxFetchBytes ?? DEFAULT_WEB_MAX_BYTES
      const maxBytes = boundedInt(
        args.max_bytes,
        maxBytesCap,
        Math.min(MIN_WEB_FETCH_BYTES, maxBytesCap),
        maxBytesCap
      )
      const timeoutMs = boundedInt(args.timeout_ms, DEFAULT_WEB_TIMEOUT_MS, 1, DEFAULT_WEB_TIMEOUT_MS)
      try {
        const result = await provider.fetch({
          url: policy.url.href,
          maxBytes,
          timeoutMs,
          signal: context.abortSignal
        })
        return {
          output: fetchOutput(result, telemetry({
            startedAt,
            policy: 'allowed',
            url: policy.url.href,
            provider: provider.id,
            byteCount: result.byteCount
          }))
        }
      } catch (error) {
        return toolError('fetch_failed', errorMessage(error), telemetry({
          startedAt,
          policy: 'allowed',
          url: policy.url.href,
          provider: provider.id
        }))
      }
    }
  })
}

function createSearchTool(config: WebCapabilityConfig, provider: WebProvider) {
  return LocalToolHost.defineTool({
    name: 'web_search',
    description: 'Search the web through the configured provider and return ranked results with source metadata.',
    inputSchema: {
      type: 'object',
      properties: {
        query: { type: 'string' },
        limit: { type: 'number' },
        timeout_ms: { type: 'number' }
      },
      required: ['query'],
      additionalProperties: false
    },
    policy: 'untrusted',
    execute: async (args, context) => {
      const startedAt = Date.now()
      const query = pickString(args.query)
      if (!query) return toolError('invalid_query', 'query is required')
      if (!provider.search) return toolError('provider_unavailable', 'web search provider is unavailable')
      const limit = boundedInt(args.limit, DEFAULT_SEARCH_LIMIT, 1, MAX_SEARCH_LIMIT)
      const timeoutMs = boundedInt(args.timeout_ms, DEFAULT_WEB_TIMEOUT_MS, 1, DEFAULT_WEB_TIMEOUT_MS)
      try {
        const results = await provider.search({
          query,
          limit,
          timeoutMs,
          signal: context.abortSignal
        })
        return {
          output: searchOutput(query, provider.id, results, telemetry({
            startedAt,
            policy: 'allowed',
            provider: provider.id,
            query,
            resultCount: results.length
          }))
        }
      } catch (error) {
        return toolError('search_failed', errorMessage(error), telemetry({
          startedAt,
          policy: 'allowed',
          provider: provider.id,
          query
        }))
      }
    }
  })
}

class FetchWebProvider implements WebProvider {
  readonly id = 'fetch'
  private readonly nowIso: () => string

  constructor(nowIso: (() => string) | undefined) {
    this.nowIso = nowIso ?? (() => new Date().toISOString())
  }

  async fetch(request: {
    url: string
    maxBytes: number
    timeoutMs: number
    signal: AbortSignal
  }): Promise<WebFetchResult> {
    const controller = new AbortController()
    const timeout = setTimeout(() => controller.abort(), request.timeoutMs)
    const onAbort = () => controller.abort()
    request.signal.addEventListener('abort', onAbort, { once: true })
    try {
      const response = await fetch(request.url, { signal: controller.signal })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)

      // Oversized pages truncate at maxBytes via the streaming read below.
      // Hard-failing on the declared content-length made most real pages
      // unfetchable whenever the model passed a small byte budget.

      // Stream response body with size limit
      const reader = response.body?.getReader()
      if (!reader) throw new Error('response body is not readable')

      const chunks: Uint8Array[] = []
      let totalBytes = 0
      let truncated = false

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        const remaining = request.maxBytes - totalBytes
        if (remaining <= 0) {
          truncated = true
          reader.cancel()
          break
        }

        if (value.length > remaining) {
          chunks.push(value.subarray(0, remaining))
          totalBytes += remaining
          truncated = true
          reader.cancel()
          break
        }

        chunks.push(value)
        totalBytes += value.length
      }

      const buffer = Buffer.concat(chunks)
      const contentType = response.headers.get('content-type') ?? undefined
      const raw = buffer.toString('utf8')
      const extracted = extractReadableText(raw, contentType)
      const finalUrl = response.url || request.url
      return {
        sourceId: sourceIdFor('fetch', finalUrl),
        url: request.url,
        finalUrl,
        title: extracted.title,
        contentType,
        text: extracted.text,
        retrievedAt: this.nowIso(),
        byteCount: totalBytes,
        truncated
      }
    } finally {
      clearTimeout(timeout)
      request.signal.removeEventListener('abort', onAbort)
    }
  }
}

function fetchOutput(result: WebFetchResult, toolTelemetry: Record<string, unknown>) {
  const source = {
    sourceId: result.sourceId,
    url: result.finalUrl,
    title: result.title,
    retrievedAt: result.retrievedAt
  }
  return {
    sourceId: result.sourceId,
    url: result.url,
    finalUrl: result.finalUrl,
    title: result.title,
    retrievedAt: result.retrievedAt,
    contentType: result.contentType,
    text: result.text,
    byteCount: result.byteCount,
    truncated: result.truncated,
    sources: [source],
    citations: [source],
    telemetry: toolTelemetry
  }
}

function searchOutput(
  query: string,
  provider: string,
  results: WebSearchResult[],
  toolTelemetry: Record<string, unknown>
) {
  const sources = results.map((result) => ({
    sourceId: result.sourceId,
    url: result.url,
    title: result.title,
    retrievedAt: result.retrievedAt
  }))
  return {
    query,
    provider,
    results,
    sources,
    citations: sources,
    telemetry: toolTelemetry
  }
}

function validateUrlPolicy(rawUrl: string, config: WebCapabilityConfig): { ok: true; url: URL } | { ok: false; reason: string } {
  let url: URL
  try {
    url = new URL(rawUrl)
  } catch {
    return { ok: false, reason: 'URL must be absolute' }
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return { ok: false, reason: 'only http and https URLs are allowed' }
  }
  const hostname = url.hostname.toLowerCase()
  if (config.denyDomains.some((domain) => domainMatches(hostname, domain))) {
    return { ok: false, reason: `domain is denied: ${hostname}` }
  }
  if (config.allowDomains.length > 0 && !config.allowDomains.some((domain) => domainMatches(hostname, domain))) {
    return { ok: false, reason: `domain is not allowed: ${hostname}` }
  }
  return { ok: true, url }
}

function domainMatches(hostname: string, domain: string): boolean {
  const normalized = domain.toLowerCase().replace(/^\./, '')
  return hostname === normalized || hostname.endsWith(`.${normalized}`)
}

function extractReadableText(raw: string, contentType: string | undefined): { title?: string; text: string } {
  if (!contentType?.toLowerCase().includes('html')) {
    return { text: normalizeWhitespace(raw) }
  }
  const extracted = extractHtmlText(raw)
  const title = normalizeWhitespace(decodeHtmlTextEntities(extracted.title))
  const text = normalizeWhitespace(decodeHtmlTextEntities(extracted.text))
  return {
    ...(title ? { title } : {}),
    text
  }
}

function extractHtmlText(raw: string): { title: string; text: string } {
  const titleParts: string[] = []
  const textParts: string[] = []
  let index = 0
  let inTitle = false
  let skipTag: 'script' | 'style' | null = null

  while (index < raw.length) {
    if (raw[index] !== '<') {
      if (!skipTag) {
        if (inTitle) titleParts.push(raw[index])
        else textParts.push(raw[index])
      }
      index += 1
      continue
    }

    if (raw.startsWith('<!--', index)) {
      const commentEnd = raw.indexOf('-->', index + 4)
      index = commentEnd >= 0 ? commentEnd + 3 : raw.length
      continue
    }

    const tagEnd = findHtmlTagEnd(raw, index + 1)
    if (tagEnd < 0) {
      if (!skipTag) {
        if (inTitle) titleParts.push(raw[index])
        else textParts.push(raw[index])
      }
      index += 1
      continue
    }

    const tag = parseHtmlTag(raw.slice(index + 1, tagEnd))
    index = tagEnd + 1
    if (!tag) continue

    if (skipTag) {
      if (tag.closing && tag.name === skipTag) skipTag = null
      continue
    }

    if (tag.name === 'script' || tag.name === 'style') {
      if (!tag.closing && !tag.selfClosing) skipTag = tag.name
      continue
    }

    if (tag.name === 'title') {
      inTitle = !tag.closing && !tag.selfClosing
      continue
    }

    if (inTitle) continue
    if (tag.name === 'br' || (tag.closing && isHtmlBlockTag(tag.name))) {
      textParts.push('\n')
    } else {
      textParts.push(' ')
    }
  }

  return {
    title: titleParts.join(''),
    text: textParts.join('')
  }
}

function findHtmlTagEnd(raw: string, start: number): number {
  let quote: string | null = null
  for (let index = start; index < raw.length; index += 1) {
    const char = raw[index]
    if (quote) {
      if (char === quote) quote = null
      continue
    }
    if (char === '"' || char === "'") {
      quote = char
    } else if (char === '>') {
      return index
    }
  }
  return -1
}

function parseHtmlTag(content: string): {
  name: string
  closing: boolean
  selfClosing: boolean
} | null {
  let index = 0
  while (index < content.length && isHtmlWhitespace(content[index])) index += 1
  const closing = content[index] === '/'
  if (closing) {
    index += 1
    while (index < content.length && isHtmlWhitespace(content[index])) index += 1
  }

  const nameStart = index
  while (index < content.length && isHtmlNameChar(content[index])) index += 1
  if (index === nameStart) return null

  let end = content.length
  while (end > index && isHtmlWhitespace(content[end - 1])) end -= 1
  return {
    name: content.slice(nameStart, index).toLowerCase(),
    closing,
    selfClosing: end > index && content[end - 1] === '/'
  }
}

function decodeHtmlTextEntities(value: string): string {
  let out = ''
  let index = 0
  while (index < value.length) {
    if (value[index] !== '&') {
      out += value[index]
      index += 1
      continue
    }
    const semicolon = value.indexOf(';', index + 1)
    if (semicolon < 0 || semicolon - index > 32) {
      out += value[index]
      index += 1
      continue
    }
    const entity = value.slice(index + 1, semicolon)
    const decoded = decodeHtmlTextEntity(entity)
    if (decoded == null) {
      out += value.slice(index, semicolon + 1)
    } else {
      out += decoded
    }
    index = semicolon + 1
  }
  return out
}

function decodeHtmlTextEntity(entity: string): string | null {
  const lower = entity.toLowerCase()
  switch (lower) {
    case 'nbsp':
      return ' '
    case 'amp':
      return '&'
    case 'quot':
      return '"'
    case 'apos':
      return "'"
    default:
      return decodeNumericHtmlTextEntity(lower)
  }
}

function decodeNumericHtmlTextEntity(entity: string): string | null {
  if (!entity.startsWith('#')) return null
  const hex = entity[1] === 'x'
  const digits = entity.slice(hex ? 2 : 1)
  if (!digits) return null
  const codePoint = htmlEntityCodePoint(digits, hex)
  if (codePoint == null || codePoint <= 0 || codePoint === 60 || codePoint === 62) return null
  try {
    return String.fromCodePoint(codePoint)
  } catch {
    return null
  }
}

function htmlEntityCodePoint(digits: string, hex: boolean): number | null {
  let out = 0
  for (const char of digits) {
    const digit = htmlEntityDigitValue(char)
    if (digit == null || digit >= (hex ? 16 : 10)) return null
    out = out * (hex ? 16 : 10) + digit
    if (out > 0x10ffff) return null
  }
  return out
}

function htmlEntityDigitValue(char: string): number | null {
  const code = char.charCodeAt(0)
  if (code >= 48 && code <= 57) return code - 48
  if (code >= 97 && code <= 102) return code - 87
  return null
}

function isHtmlBlockTag(name: string): boolean {
  return (
    name === 'p' ||
    name === 'div' ||
    name === 'li' ||
    name === 'section' ||
    name === 'article' ||
    name === 'header' ||
    name === 'footer' ||
    name === 'tr' ||
    name === 'table' ||
    name === 'blockquote' ||
    (name.length === 2 && name[0] === 'h' && name[1] >= '1' && name[1] <= '6')
  )
}

function isHtmlWhitespace(char: string | undefined): boolean {
  return char === ' ' || char === '\n' || char === '\t' || char === '\r' || char === '\f'
}

function isHtmlNameChar(char: string | undefined): boolean {
  if (!char) return false
  return !isHtmlWhitespace(char) && char !== '/' && char !== '>'
}

function normalizeWhitespace(value: string): string {
  return value.replace(/\r/g, '').replace(/[ \t]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim()
}

function telemetry(input: {
  startedAt: number
  policy: 'allowed' | 'blocked'
  provider?: string
  url?: string
  query?: string
  byteCount?: number
  resultCount?: number
}): Record<string, unknown> {
  return {
    provider: input.provider,
    url: input.url,
    query: input.query,
    byteCount: input.byteCount,
    resultCount: input.resultCount,
    durationMs: Date.now() - input.startedAt,
    cacheStatus: 'miss',
    policy: input.policy
  }
}

function toolError(code: string, message: string, toolTelemetry?: Record<string, unknown>) {
  return {
    output: {
      error: {
        code,
        message
      },
      ...(toolTelemetry ? { telemetry: toolTelemetry } : {})
    },
    isError: true
  }
}

function pickString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function boundedInt(value: unknown, fallback: number, min: number, max: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.min(Math.max(Math.floor(value), min), max)
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
