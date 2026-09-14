import { describe, expect, it } from 'vitest'
import { providerDisplayName, providerEndpointKind, providerModelDisplayName } from './provider-display'

describe('provider presentation', () => {
  it.each(['http://localhost:5500/v1', 'http://127.0.0.1:5500', 'http://[::1]:5500', 'http://127.1:5500'])(
    'identifies %s as local without asserting that it is a mock or official service', (endpoint) => {
      expect(providerEndpointKind(endpoint)).toBe('local')
    }
  )
  it('keeps connection classification separate from presentation aliases', () => {
    expect(providerEndpointKind('https://api.deepseek.com')).toBe('official-deepseek')
    expect(providerEndpointKind('https://api.deepseek.com.evil.test')).toBe('remote')
    expect(providerEndpointKind('https://localhost.evil.test')).toBe('remote')
    expect(providerEndpointKind('https://gateway.test')).toBe('remote')
    expect(providerEndpointKind('file:///private/anything')).toBe('unknown')
  })
  it.each(['deepseek-v4-flash', 'deepseek-flash', 'deepseek-v4-flash-vision-exp'])(
    'uses the concise catalog name for %s without needing a service address', (modelId) => {
      expect(providerModelDisplayName(modelId)).toBe('V4.1 Flash')
    }
  )
  it('preserves custom names, unknown IDs and distinct model versions', () => {
    expect(providerModelDisplayName('deepseek-v4-pro')).toBe('V4 Pro')
    expect(providerDisplayName('deepseek', 'deepseek')).toBe('DeepSeek')
    expect(providerDisplayName('team', '团队专用')).toBe('团队专用')
    expect(providerModelDisplayName('future-model')).toBe('future-model')
    expect(providerModelDisplayName('constructor')).toBe('constructor')
    expect(providerModelDisplayName('__proto__')).toBe('__proto__')
  })
})
