import { describe, expect, it, vi } from 'vitest'
import { createHash } from 'node:crypto'
import { createNativeOAuthCallbackRouter, nativeOAuthCallbackFromArgv } from './native-oauth-callback'
import { parseOAuthNativeCallback } from './provider-oauth-lifecycle'

const bindingKey = `oauthb_${createHash('sha256').update('native-callback-test').digest('base64url')}`

function callback(overrides: Record<string, string> = {}): string {
  const url = new URL(`com.analytix.desktop:/oauth/callback/${bindingKey}`)
  const values = { code: 'synthetic-code', state: 'synthetic-state', iss: 'https://issuer.example.test/', ...overrides }
  for (const [key, value] of Object.entries(values)) url.searchParams.set(key, value)
  return url.toString()
}

describe('Main-only native OAuth callback routing', () => {
  it('accepts only the exact native scheme/path and one unambiguous success tuple', () => {
    expect(parseOAuthNativeCallback(callback())).toMatchObject({
      bindingKey,
      state: 'synthetic-state',
      code: 'synthetic-code',
      issuer: 'https://issuer.example.test/'
    })
    const duplicate = `${callback()}&code=duplicate`
    expect(parseOAuthNativeCallback(duplicate)).toBeNull()
    expect(parseOAuthNativeCallback(callback({ error: 'access_denied' }))).toBeNull()
    expect(parseOAuthNativeCallback(callback().replace('/oauth/', '/OAuth/'))).toBeNull()
    expect(parseOAuthNativeCallback(callback().replace('callback/', 'callback/%6f'))).toBeNull()
    expect(parseOAuthNativeCallback(`${callback()}#fragment`)).toBeNull()
    expect(parseOAuthNativeCallback(callback().replace('com.analytix.desktop:', 'analytix:'))).toBeNull()
  })

  it('extracts one Windows/Linux argv callback and rejects ambiguous argv', () => {
    const value = callback()
    expect(nativeOAuthCallbackFromArgv(['analytix', '--hidden', value])).toBe(value)
    expect(nativeOAuthCallbackFromArgv(['analytix', value, callback({ state: 'other' })])).toBe('')
    expect(nativeOAuthCallbackFromArgv(['analytix', 'analytix://threads/thread-a'])).toBe('')
  })

  it('bounds and drains the pre-ready macOS/argv callback queue exactly once', async () => {
    const router = createNativeOAuthCallbackRouter()
    const first = callback()
    expect(router.route(first)).toBe(true)
    expect(router.route(first)).toBe(false)
    for (let index = 1; index < 8; index++) {
      expect(router.route(callback({ state: `state-${index}` }))).toBe(true)
    }
    expect(router.pendingCount()).toBe(8)
    expect(router.route(callback({ state: 'overflow' }))).toBe(false)
    const receiver = vi.fn(async (_callbackUrl: string) => undefined)
    await router.activate(receiver)
    expect(receiver).toHaveBeenCalledTimes(8)
    expect(receiver.mock.calls[0]?.[0]).toBe(first)
    expect(router.pendingCount()).toBe(0)
    expect(router.route(callback({ state: 'live' }))).toBe(true)
    await vi.waitFor(() => expect(receiver).toHaveBeenCalledTimes(9))
  })
})
