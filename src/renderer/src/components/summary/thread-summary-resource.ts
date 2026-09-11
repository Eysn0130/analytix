import type { CoreThreadSummaryResponseJson } from '../../agent/analytix-contract'
import { isPublicProjectionRevoked } from '../../lib/public-projection-revocation'
import { getProvider } from '../../agent/registry'

const THREAD_SUMMARY_CACHE_LIMIT = 8
const THREAD_SUMMARY_FRESH_TTL_MS = 3_000
const THREAD_SUMMARY_RETAIN_TTL_MS = 30_000

type ThreadSummaryCacheEntry = {
  expiresAt: number
  staleAt: number
  value: CoreThreadSummaryResponseJson
}

type ThreadSummaryStore = {
  cache: Map<string, ThreadSummaryCacheEntry>
  inflight: Map<string, Promise<CoreThreadSummaryResponseJson>>
}

type ThreadSummaryHost = typeof globalThis & {
  __ANALYTIX_SHARED_THREAD_SUMMARY_STORE__?: ThreadSummaryStore
}

function emptySummary(threadId: string): CoreThreadSummaryResponseJson {
  return {
    threadId,
    generatedAt: new Date().toISOString(),
    latestSeq: 0,
    subagents: [],
    tasks: [],
    outputs: [],
    sources: [],
    sideChats: [],
    backgroundProcesses: []
  }
}

function getStore(): ThreadSummaryStore {
  const host = globalThis as ThreadSummaryHost
  if (host.__ANALYTIX_SHARED_THREAD_SUMMARY_STORE__) {
    return host.__ANALYTIX_SHARED_THREAD_SUMMARY_STORE__
  }
  const store: ThreadSummaryStore = {
    cache: new Map<string, ThreadSummaryCacheEntry>(),
    inflight: new Map<string, Promise<CoreThreadSummaryResponseJson>>()
  }
  host.__ANALYTIX_SHARED_THREAD_SUMMARY_STORE__ = store
  return store
}

function trimCache(store: ThreadSummaryStore, now = Date.now()): void {
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (entry.expiresAt <= now) store.cache.delete(key)
  })
  while (store.cache.size > THREAD_SUMMARY_CACHE_LIMIT) {
    const oldestKey = store.cache.keys().next().value
    if (!oldestKey) break
    store.cache.delete(oldestKey)
  }
}

function writeCache(store: ThreadSummaryStore, threadId: string, value: CoreThreadSummaryResponseJson): void {
  if (value.threadId !== threadId) {
    throw new Error('Thread summary identity mismatch.')
  }
  const now = Date.now()
  store.cache.set(threadId, {
    expiresAt: now + THREAD_SUMMARY_RETAIN_TTL_MS,
    staleAt: now + THREAD_SUMMARY_FRESH_TTL_MS,
    value
  })
  trimCache(store, now)
}

function requestThreadSummary(
  store: ThreadSummaryStore,
  threadId: string
): Promise<CoreThreadSummaryResponseJson> {
  if (isPublicProjectionRevoked(threadId)) return Promise.resolve(emptySummary(threadId))
  const existing = store.inflight.get(threadId)
  if (existing) return existing

  const provider = getProvider()
  const request = (typeof provider.getThreadSummary === 'function'
    ? provider.getThreadSummary(threadId)
    : Promise.resolve(emptySummary(threadId)))
    .then((summary) => {
      if (isPublicProjectionRevoked(threadId)) return emptySummary(threadId)
      if (summary.threadId !== threadId) {
        throw new Error('Thread summary identity mismatch.')
      }
      if (store.inflight.get(threadId) === request) {
        writeCache(store, threadId, summary)
      }
      return summary
    })
    .finally(() => {
      if (store.inflight.get(threadId) === request) {
        store.inflight.delete(threadId)
      }
    })
  store.inflight.set(threadId, request)
  return request
}

export function querySharedThreadSummary(
  threadId: string | null | undefined,
  options?: { force?: boolean; keepStale?: boolean }
): Promise<CoreThreadSummaryResponseJson> {
  const normalizedThreadId = String(threadId || '').trim()
  if (!normalizedThreadId) {
    return Promise.resolve(emptySummary(''))
  }
  if (isPublicProjectionRevoked(normalizedThreadId)) {
    return Promise.resolve(emptySummary(normalizedThreadId))
  }

  const store = getStore()
  const now = Date.now()
  const force = Boolean(options?.force)
  trimCache(store, now)

  const cached = store.cache.get(normalizedThreadId)
  if (force) {
    if (!options?.keepStale) {
      store.cache.delete(normalizedThreadId)
    }
    return requestThreadSummary(store, normalizedThreadId)
  }

  if (cached && cached.value.threadId !== normalizedThreadId) {
    store.cache.delete(normalizedThreadId)
  } else if (cached && cached.expiresAt > now) {
    if (cached.staleAt <= now) {
      void requestThreadSummary(store, normalizedThreadId).catch(() => {})
    }
    return Promise.resolve(cached.value)
  }

  return requestThreadSummary(store, normalizedThreadId)
}

export function readCachedSharedThreadSummary(threadId: string | null | undefined): CoreThreadSummaryResponseJson | null {
  const normalizedThreadId = String(threadId || '').trim()
  if (!normalizedThreadId) return null
  if (isPublicProjectionRevoked(normalizedThreadId)) return null
  const store = getStore()
  const now = Date.now()
  trimCache(store, now)
  const cached = store.cache.get(normalizedThreadId)
  if (cached && cached.value.threadId !== normalizedThreadId) {
    store.cache.delete(normalizedThreadId)
    return null
  }
  return cached && cached.expiresAt > now ? cached.value : null
}

export function invalidateSharedThreadSummary(threadId?: string | null): void {
  const store = getStore()
  const normalizedThreadId = String(threadId || '').trim()
  if (!normalizedThreadId) {
    store.cache.clear()
    store.inflight.clear()
    return
  }
  store.cache.delete(normalizedThreadId)
  store.inflight.delete(normalizedThreadId)
}

export function preloadSharedThreadSummary(threadId: string | null | undefined): Promise<CoreThreadSummaryResponseJson> {
  return querySharedThreadSummary(threadId)
}
