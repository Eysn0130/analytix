export const MODEL_ENDPOINT_FORMATS = ['chat_completions', 'responses', 'messages', 'custom_endpoint'] as const
export type ModelEndpointFormat = (typeof MODEL_ENDPOINT_FORMATS)[number]
export const DEFAULT_MODEL_ENDPOINT_FORMAT: ModelEndpointFormat = 'chat_completions'

export function parseModelEndpointFormat(value: unknown): ModelEndpointFormat | null {
  if (typeof value !== 'string') return null
  const normalized = value.trim().toLowerCase().replace(/^\/+/, '')
  switch (normalized) {
    case 'chat':
    case 'chat-completions':
    case 'chat_completions':
    case 'v1/chat/completions':
    case 'chat/completions':
    case '/v1/chat/completions':
      return 'chat_completions'
    case 'custom':
    case 'custom-endpoint':
    case 'custom_endpoint':
    case 'custom-full-path':
    case 'custom_full_path':
    case 'full-path':
    case 'full_path':
    case 'full-url':
    case 'full_url':
      return 'custom_endpoint'
    case 'response':
    case 'responses':
    case 'v1/responses':
    case '/v1/responses':
      return 'responses'
    case 'message':
    case 'messages':
    case 'v1/messages':
    case '/v1/messages':
      return 'messages'
    default:
      return null
  }
}

// Legacy/test-support callers may still request an explicit default. Current
// configuration and execution schemas must use preprocessModelEndpointFormat
// so an unknown non-empty value is rejected instead of silently changing the
// provider wire protocol.
export function normalizeModelEndpointFormat(value: unknown): ModelEndpointFormat {
  return parseModelEndpointFormat(value) ?? DEFAULT_MODEL_ENDPOINT_FORMAT
}

export function preprocessModelEndpointFormat(value: unknown): unknown {
  if (value === undefined) return undefined
  return parseModelEndpointFormat(value) ?? value
}

export function modelEndpointPath(format: ModelEndpointFormat): string {
  switch (format) {
    case 'responses':
      return 'responses'
    case 'messages':
      return 'messages'
    case 'custom_endpoint':
    case 'chat_completions':
    default:
      return 'chat/completions'
  }
}

export function isCustomModelEndpointFormat(format: ModelEndpointFormat): boolean {
  return format === 'custom_endpoint'
}

export function usesChatCompletionsShape(format: ModelEndpointFormat): boolean {
  return format === 'chat_completions'
}

export function inferModelEndpointFormatFromUrl(url: string): ModelEndpointFormat | null {
  const query = url.search(/[?#]/)
  const path = (query < 0 ? url : url.slice(0, query)).trim().replace(/\/+$/, '').toLowerCase()
  if (path.endsWith('/chat/completions') || path.endsWith('/completions')) return 'chat_completions'
  if (path.endsWith('/responses')) return 'responses'
  if (path.endsWith('/messages')) return 'messages'
  return null
}

export function resolveModelEndpointFormat(
  endpointFormat: ModelEndpointFormat,
  baseUrl: string
): ModelEndpointFormat | null {
  return isCustomModelEndpointFormat(endpointFormat)
    ? inferModelEndpointFormatFromUrl(baseUrl)
    : endpointFormat
}
