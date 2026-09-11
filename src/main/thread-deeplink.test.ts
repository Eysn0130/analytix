import { describe, expect, it } from 'vitest'
import { parseAnalytixThreadDeepLink } from './thread-deeplink'

describe('Analytix thread deep links', () => {
  it('parses native analytix thread links', () => {
    expect(parseAnalytixThreadDeepLink('analytix://threads/thr_durable_1')).toBe('thr_durable_1')
  })

  it('parses Codex-compatible analytix thread links', () => {
    expect(parseAnalytixThreadDeepLink('codex://threads/thr_durable_1?app=analytix')).toBe('thr_durable_1')
  })

  it('rejects Codex links for other apps', () => {
    expect(parseAnalytixThreadDeepLink('codex://threads/thr_durable_1')).toBe('')
    expect(parseAnalytixThreadDeepLink('codex://threads/thr_durable_1?app=codex')).toBe('')
  })
})
