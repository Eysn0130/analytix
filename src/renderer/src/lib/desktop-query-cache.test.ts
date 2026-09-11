import { afterEach, describe, expect, it, vi } from 'vitest'
import { DesktopQueryCache } from './desktop-query-cache'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DesktopQueryCache', () => {
  it('deduplicates in-flight fetches and serves fresh cached data', async () => {
    const cache = new DesktopQueryCache()
    const fetcher = vi.fn(async () => ({ value: 1 }))

    const [first, second] = await Promise.all([
      cache.fetchQuery({ queryKey: ['settings'], staleTimeMs: 10_000, fetcher }),
      cache.fetchQuery({ queryKey: ['settings'], staleTimeMs: 10_000, fetcher })
    ])
    const third = await cache.fetchQuery({ queryKey: ['settings'], staleTimeMs: 10_000, fetcher })

    expect(first).toEqual({ value: 1 })
    expect(second).toEqual({ value: 1 })
    expect(third).toEqual({ value: 1 })
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('invalidates by query key prefix', async () => {
    const cache = new DesktopQueryCache()
    cache.setQueryData(['skills', 'list', '/tmp/a'], ['a'])
    cache.setQueryData(['skills', 'roots', '/tmp/a'], ['root'])
    cache.setQueryData(['settings'], { ok: true })

    cache.invalidateQueries({ queryKey: ['skills'] })

    expect(cache.getQueryData(['skills', 'list', '/tmp/a'])).toBeUndefined()
    expect(cache.getQueryData(['skills', 'roots', '/tmp/a'])).toBeUndefined()
    expect(cache.getQueryData(['settings'])).toEqual({ ok: true })
  })

  it('does not let an invalidated in-flight response refill the cache', async () => {
    const cache = new DesktopQueryCache()
    let resolveFetcher: (value: string) => void = () => undefined
    const task = cache.fetchQuery({
      queryKey: ['plugins'],
      staleTimeMs: 10_000,
      fetcher: () => new Promise<string>((resolve) => {
        resolveFetcher = resolve
      })
    })

    cache.invalidateQueries({ queryKey: ['plugins'] })
    resolveFetcher('stale')

    await expect(task).resolves.toBe('stale')
    expect(cache.getQueryData(['plugins'])).toBeUndefined()
  })

  it('broadcasts invalidations through the preload bridge', () => {
    const invalidateQueryCache = vi.fn(async () => true)
    vi.stubGlobal('window', {
      analytix: {
        app: {
          invalidateQueryCache
        }
      }
    })
    const cache = new DesktopQueryCache()

    cache.invalidateQueries({ queryKey: ['settings'], broadcast: true })

    expect(invalidateQueryCache).toHaveBeenCalledWith({
      queryKey: ['settings'],
      sourceClientId: expect.any(String)
    })
  })
})
