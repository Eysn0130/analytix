import { describe, expect, it } from 'vitest'
import { providerDisplayName, providerModelDisplayName } from './provider-display'

describe('provider presentation', () => {
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
