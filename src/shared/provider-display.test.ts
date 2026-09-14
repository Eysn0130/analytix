import { describe, expect, it } from 'vitest'
import { providerDisplayName, providerEndpointKind, providerModelDisplayName } from './provider-display'

describe('provider presentation', () => {
  it.each(['http://localhost:5500/v1', 'http://127.0.0.1:5500', 'http://[::1]:5500', 'http://127.1:5500'])(
    'identifies %s as local without asserting that it is a mock or an official model', (endpoint) => {
      expect(providerEndpointKind(endpoint)).toBe('local')
      expect(providerModelDisplayName('deepseek-v4-flash', endpoint)).toBe('deepseek-v4-flash')
    }
  )
  it('does not confuse official-looking hosts, remote gateways, and unsupported URLs', () => {
    expect(providerEndpointKind('https://api.deepseek.com')).toBe('official-deepseek')
    expect(providerEndpointKind('https://localhost.evil.test')).toBe('remote')
    expect(providerEndpointKind('https://gateway.test')).toBe('remote')
    expect(providerEndpointKind('file:///private/anything')).toBe('unknown')
  })
  it.each(['https://api.deepseek.com', 'https://api.deepseek.com/v1/'])(
    'labels official aliases without rewriting the request ID at %s', (endpoint) => {
      expect(providerModelDisplayName('deepseek-v4-flash', endpoint)).toBe('V4.1 Flash')
      expect(providerModelDisplayName('deepseek-flash', endpoint)).toBe('V4.1 Flash')
      expect(providerModelDisplayName('deepseek-v4-pro', endpoint)).toBe('V4 Pro')
      expect(providerDisplayName('deepseek', 'deepseek', endpoint)).toBe('DeepSeek')
      expect(providerDisplayName('team', '团队专用', endpoint)).toBe('团队专用')
      expect(providerModelDisplayName('future-model', endpoint)).toBe('future-model')
    }
  )
  it.each(['', 'http://api.deepseek.com', 'https://api.deepseek.com.evil.test',
    'https://gateway.test', 'https://api.deepseek.com/custom', 'https://api.deepseek.com?route=custom'])(
    'does not infer an official version from a model ID at %s', (endpoint) => {
      expect(providerModelDisplayName('deepseek-v4-flash', endpoint)).toBe('deepseek-v4-flash')
    }
  )
})
