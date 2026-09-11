const THREAD_ID_PATTERN = /^[A-Za-z0-9_-]{3,256}$/

export function normalizeThreadId(value: string | null | undefined): string {
  const threadId = String(value ?? '').trim()
  return THREAD_ID_PATTERN.test(threadId) ? threadId : ''
}

export function buildAnalytixThreadLink(threadId: string): string {
  const normalized = normalizeThreadId(threadId)
  return normalized ? `analytix://threads/${encodeURIComponent(normalized)}` : ''
}

export function buildCodexCompatibleThreadLink(threadId: string): string {
  const normalized = normalizeThreadId(threadId)
  return normalized ? `codex://threads/${encodeURIComponent(normalized)}?app=analytix` : ''
}
