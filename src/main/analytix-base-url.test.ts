import { describe, expect, it } from 'vitest'
import { getAnalytixBaseUrl, normalizeLocalAnalytixHost } from './analytix-base-url'

describe('getAnalytixBaseUrl', () => {
  it('uses 127.0.0.1 by default', () => {
    expect(getAnalytixBaseUrl(8899)).toBe('http://127.0.0.1:8899')
  })

  it('formats IPv6 loopback hosts for URL use', () => {
    expect(getAnalytixBaseUrl(8899, '::1')).toBe('http://[::1]:8899')
    expect(getAnalytixBaseUrl(8899, '[::1]')).toBe('http://[::1]:8899')
  })

  it('accepts localhost aliases only', () => {
    expect(normalizeLocalAnalytixHost('localhost')).toBe('localhost')
    expect(() => getAnalytixBaseUrl(8899, 'example.com')).toThrow(/local host/)
  })
})
