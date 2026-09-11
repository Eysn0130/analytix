import type { QueryCacheInvalidatePayload } from '@shared/analytix-api'

export type QueryKeyPart = string | number | boolean | null | undefined
export type QueryKey = readonly QueryKeyPart[]

type CacheEntry<T = unknown> = {
  data: T
  updatedAt: number
  staleTimeMs: number
}

type FetchQueryOptions<T> = {
  queryKey: QueryKey
  staleTimeMs: number
  fetcher: () => Promise<T>
  forceRefresh?: boolean
}

type InvalidateOptions = {
  queryKey: QueryKey
  exact?: boolean
  broadcast?: boolean
}

type CacheListener = (payload: QueryCacheInvalidatePayload) => void

function newClientId(): string {
  const random = globalThis.crypto?.randomUUID?.()
  return random ?? `renderer-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

function normalizeQueryKey(queryKey: QueryKey): string[] {
  return queryKey.map((part) => part == null ? '' : String(part))
}

function queryKeyId(queryKey: QueryKey): string {
  return JSON.stringify(normalizeQueryKey(queryKey))
}

function queryKeyMatches(candidate: string[], prefix: string[], exact: boolean): boolean {
  if (exact && candidate.length !== prefix.length) return false
  if (candidate.length < prefix.length) return false
  return prefix.every((part, index) => candidate[index] === part)
}

export class DesktopQueryCache {
  private readonly clientId = newClientId()
  private readonly entries = new Map<string, CacheEntry>()
  private readonly inFlight = new Map<string, Promise<unknown>>()
  private readonly listeners = new Set<CacheListener>()
  private bridgeInstalled = false
  private removeBridgeListener: (() => void) | null = null

  fetchQuery<T>({
    queryKey,
    staleTimeMs,
    fetcher,
    forceRefresh = false
  }: FetchQueryOptions<T>): Promise<T> {
    const key = queryKeyId(queryKey)
    const cached = this.entries.get(key) as CacheEntry<T> | undefined
    const now = Date.now()
    if (!forceRefresh && cached && now - cached.updatedAt <= cached.staleTimeMs) {
      return Promise.resolve(cached.data)
    }
    const existing = this.inFlight.get(key) as Promise<T> | undefined
    if (existing && !forceRefresh) return existing
    const task = fetcher()
      .then((data) => {
        if (this.inFlight.get(key) === task) {
          this.entries.set(key, {
            data,
            updatedAt: Date.now(),
            staleTimeMs
          })
        }
        return data
      })
      .finally(() => {
        if (this.inFlight.get(key) === task) this.inFlight.delete(key)
      })
    this.inFlight.set(key, task)
    return task
  }

  getQueryData<T>(queryKey: QueryKey): T | undefined {
    return this.entries.get(queryKeyId(queryKey))?.data as T | undefined
  }

  setQueryData<T>(queryKey: QueryKey, data: T, staleTimeMs = Number.POSITIVE_INFINITY): void {
    this.entries.set(queryKeyId(queryKey), {
      data,
      updatedAt: Date.now(),
      staleTimeMs
    })
  }

  invalidateQueries({ queryKey, exact = false, broadcast = false }: InvalidateOptions): void {
    const normalized = normalizeQueryKey(queryKey)
    const keys = new Set([...this.entries.keys(), ...this.inFlight.keys()])
    for (const key of keys) {
      const candidate = JSON.parse(key) as string[]
      if (queryKeyMatches(candidate, normalized, exact)) {
        this.entries.delete(key)
        this.inFlight.delete(key)
      }
    }
    const payload = { queryKey: normalized, sourceClientId: this.clientId }
    this.notify(payload)
    if (broadcast) this.broadcastInvalidation(normalized)
  }

  broadcastInvalidation(queryKey: QueryKey): void {
    if (typeof window === 'undefined') return
    const normalized = normalizeQueryKey(queryKey)
    void window.analytix?.app?.invalidateQueryCache?.({
      queryKey: normalized,
      sourceClientId: this.clientId
    }).catch(() => undefined)
  }

  subscribe(listener: CacheListener): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  installBridge(): void {
    if (this.bridgeInstalled) return
    this.bridgeInstalled = true
    if (typeof window === 'undefined') return
    const listen = window.analytix?.app?.onQueryCacheInvalidated
    if (typeof listen !== 'function') return
    this.removeBridgeListener = listen((payload) => {
      if (payload.sourceClientId === this.clientId) return
      this.invalidateQueries({ queryKey: payload.queryKey, exact: false, broadcast: false })
    })
  }

  resetForTests(): void {
    this.entries.clear()
    this.inFlight.clear()
    this.listeners.clear()
    this.removeBridgeListener?.()
    this.removeBridgeListener = null
    this.bridgeInstalled = false
  }

  private notify(payload: QueryCacheInvalidatePayload): void {
    for (const listener of this.listeners) {
      listener(payload)
    }
  }
}

export const desktopQueryCache = new DesktopQueryCache()
