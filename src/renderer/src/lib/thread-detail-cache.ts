import type { AgentProvider, NormalizedThread } from '../agent/types'

type ThreadDetail = Awaited<ReturnType<AgentProvider['getThreadDetail']>>

type ThreadDetailCacheEntry = {
  updatedAt: string
  cachedAt: number
  detail?: ThreadDetail
  promise?: Promise<ThreadDetail>
}

const THREAD_DETAIL_CACHE_MAX_AGE_MS = 60_000
const THREAD_DETAIL_CACHE_MAX_ENTRIES = 32
export const THREAD_DETAIL_PREWARM_LIMIT = 8
const THREAD_DETAIL_PREWARM_CONCURRENCY = 2

const threadDetailCache = new Map<string, ThreadDetailCacheEntry>()

function normalizeThreadId(threadId: string): string {
  return threadId.trim()
}

function threadCanUseStableCache(
  thread: Pick<NormalizedThread, 'updatedAt' | 'status' | 'historyAuthority'> | null | undefined
): boolean {
  if (!thread?.updatedAt) return false
  if (thread.historyAuthority === 'case_boundary_only_v1') return false
  return (thread.status ?? '').trim().toLowerCase() !== 'running'
}

function isFreshEntry(entry: ThreadDetailCacheEntry, updatedAt: string, now = Date.now()): boolean {
  return entry.updatedAt === updatedAt && now - entry.cachedAt <= THREAD_DETAIL_CACHE_MAX_AGE_MS
}

function trimThreadDetailCache(): void {
  while (threadDetailCache.size > THREAD_DETAIL_CACHE_MAX_ENTRIES) {
    const oldestKey = threadDetailCache.keys().next().value
    if (!oldestKey) return
    threadDetailCache.delete(oldestKey)
  }
}

export function invalidateThreadDetailCache(threadId?: string | null): void {
  const id = threadId ? normalizeThreadId(threadId) : ''
  if (!id) {
    threadDetailCache.clear()
    return
  }
  threadDetailCache.delete(id)
}

export function hasFreshThreadDetailCache(
  thread: Pick<NormalizedThread, 'id' | 'updatedAt' | 'status' | 'historyAuthority'>
): boolean {
  const id = normalizeThreadId(thread.id)
  if (!id || !threadCanUseStableCache(thread)) return false
  const entry = threadDetailCache.get(id)
  return Boolean(entry && isFreshEntry(entry, thread.updatedAt))
}

export async function loadThreadDetailWithCache(
  provider: AgentProvider,
  threadId: string,
  thread: Pick<NormalizedThread, 'updatedAt' | 'status' | 'historyAuthority'> | null | undefined
): Promise<ThreadDetail> {
  const id = normalizeThreadId(threadId)
  if (!id) return provider.getThreadDetail(threadId)
  if (!threadCanUseStableCache(thread)) return provider.getThreadDetail(id)

  const updatedAt = thread?.updatedAt
  if (!updatedAt) return provider.getThreadDetail(id)
  const existing = threadDetailCache.get(id)
  if (existing && isFreshEntry(existing, updatedAt)) {
    if (existing.detail) return existing.detail
    if (existing.promise) return existing.promise
  }

  const promise = provider.getThreadDetail(id)
  threadDetailCache.set(id, {
    updatedAt,
    cachedAt: Date.now(),
    promise
  })
  trimThreadDetailCache()

  try {
    const detail = await promise
    const current = threadDetailCache.get(id)
    if (current?.promise === promise) {
      threadDetailCache.set(id, {
        updatedAt,
        cachedAt: Date.now(),
        detail
      })
      trimThreadDetailCache()
    }
    return detail
  } catch (error) {
    const current = threadDetailCache.get(id)
    if (current?.promise === promise) threadDetailCache.delete(id)
    throw error
  }
}

export async function prewarmThreadDetails(
  provider: AgentProvider,
  threads: NormalizedThread[],
  options: {
    activeThreadId?: string | null
    limit?: number
    concurrency?: number
    shouldSkip?: () => boolean
  } = {}
): Promise<void> {
  if (options.shouldSkip?.()) return
  const activeThreadId = normalizeThreadId(options.activeThreadId ?? '')
  const limit = Math.max(0, Math.floor(options.limit ?? THREAD_DETAIL_PREWARM_LIMIT))
  const concurrency = Math.max(1, Math.floor(options.concurrency ?? THREAD_DETAIL_PREWARM_CONCURRENCY))
  if (limit === 0) return

  const seen = new Set<string>()
  const targets: NormalizedThread[] = []
  for (const thread of threads) {
    const id = normalizeThreadId(thread.id)
    if (!id || id === activeThreadId || seen.has(id)) continue
    seen.add(id)
    if (!threadCanUseStableCache(thread)) continue
    if (hasFreshThreadDetailCache(thread)) continue
    targets.push(thread)
    if (targets.length >= limit) break
  }

  for (let index = 0; index < targets.length; index += concurrency) {
    if (options.shouldSkip?.()) return
    const batch = targets.slice(index, index + concurrency)
    await Promise.allSettled(
      batch.map((thread) => loadThreadDetailWithCache(provider, thread.id, thread))
    )
  }
}

export function schedulePrewarmThreadDetails(
  provider: AgentProvider,
  threads: NormalizedThread[],
  options: {
    activeThreadId?: string | null
    limit?: number
    concurrency?: number
    shouldSkip?: () => boolean
  } = {}
): () => void {
  let cancelled = false
  let cancelIdle: (() => void) | null = null
  const run = (): void => {
    if (cancelled || options.shouldSkip?.()) return
    void prewarmThreadDetails(provider, threads, {
      activeThreadId: options.activeThreadId,
      limit: options.limit,
      concurrency: options.concurrency,
      shouldSkip: () => cancelled || options.shouldSkip?.() === true
    })
  }

  if (typeof window !== 'undefined' && typeof window.requestIdleCallback === 'function') {
    const handle = window.requestIdleCallback(run, { timeout: 2000 }) as unknown as number
    cancelIdle = () => window.cancelIdleCallback?.(handle)
  } else {
    const handle = globalThis.setTimeout(run, 750)
    cancelIdle = () => globalThis.clearTimeout(handle)
  }

  return () => {
    cancelled = true
    cancelIdle?.()
  }
}
