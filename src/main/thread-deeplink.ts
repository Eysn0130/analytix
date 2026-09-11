export const ANALYTIX_DEEPLINK_PROTOCOL = 'analytix'
export const CODEX_DEEPLINK_PROTOCOL = 'codex'

const THREAD_ID_PATTERN = /^[A-Za-z0-9_-]{3,256}$/

export function isValidAnalytixThreadId(value: string): boolean {
  return THREAD_ID_PATTERN.test(value)
}

export function parseAnalytixThreadDeepLink(value: string): string {
  const source = value.trim()
  if (!source) return ''
  try {
    const parsed = new URL(source)
    const isAnalytixLink =
      parsed.protocol === `${ANALYTIX_DEEPLINK_PROTOCOL}:` &&
      parsed.hostname === 'threads'
    const isCodexAnalytixLink =
      parsed.protocol === `${CODEX_DEEPLINK_PROTOCOL}:` &&
      parsed.hostname === 'threads' &&
      parsed.searchParams.get('app') === 'analytix'
    if (!isAnalytixLink && !isCodexAnalytixLink) return ''
    const threadId = decodeURIComponent(parsed.pathname.replace(/^\/+/u, '').split('/')[0] || '').trim()
    return isValidAnalytixThreadId(threadId) ? threadId : ''
  } catch {
    return ''
  }
}
