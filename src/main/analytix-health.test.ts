import { describe, expect, it } from 'vitest'
import { isAnalytixHealthResponseBody } from './analytix-health'

describe('isAnalytixHealthResponseBody', () => {
  it('accepts Analytix serve health responses', () => {
    expect(isAnalytixHealthResponseBody(JSON.stringify({
      status: 'ok',
      service: 'analytix',
      mode: 'serve'
    }))).toBe(true)
  })

  it('rejects generic or legacy runtime health responses', () => {
    expect(isAnalytixHealthResponseBody(JSON.stringify({ status: 'ok' }))).toBe(false)
    expect(isAnalytixHealthResponseBody(JSON.stringify({
      status: 'ok',
      service: 'codewhale',
      mode: 'serve'
    }))).toBe(false)
  })
})
