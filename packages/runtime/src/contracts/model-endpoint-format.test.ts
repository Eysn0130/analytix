import { describe, expect, it } from 'vitest'
import { ServeOptionsSchema } from '../cli/cli-options.js'
import {
  AnalytixServeConfigSchema,
  ModelContextProfileConfigSchema,
  ModelProviderConfigSchema
} from '../config/analytix-config.js'
import {
  parseModelEndpointFormat,
  preprocessModelEndpointFormat
} from './model-endpoint-format.js'

describe('model endpoint format admission', () => {
  it.each([
    ['chat', 'chat_completions'],
    ['/v1/chat/completions', 'chat_completions'],
    ['responses', 'responses'],
    ['v1/messages', 'messages'],
    ['custom-full-path', 'custom_endpoint']
  ] as const)('canonicalizes the known alias %s', (input, expected) => {
    expect(parseModelEndpointFormat(input)).toBe(expected)
  })

  it.each([undefined, null, '', 'typoo', 'chat_completionz', 1, false, {}])(
    'does not assign a protocol to unknown input %j',
    (input) => {
      expect(parseModelEndpointFormat(input)).toBeNull()
    }
  )

  it('preserves absence for an owning schema default and preserves invalid input for rejection', () => {
    expect(preprocessModelEndpointFormat(undefined)).toBeUndefined()
    expect(preprocessModelEndpointFormat('chat-completions')).toBe('chat_completions')
    expect(preprocessModelEndpointFormat('typoo')).toBe('typoo')
  })

  it('rejects unknown current CLI, serve-config, provider, and per-model formats', () => {
    expect(ServeOptionsSchema.safeParse({ dataDir: '/tmp/analytix', endpointFormat: 'typoo' }).success).toBe(false)
    expect(AnalytixServeConfigSchema.safeParse({ endpointFormat: 'typoo' }).success).toBe(false)
    expect(ModelProviderConfigSchema.safeParse({
      id: 'provider-a', baseUrl: 'https://provider.example', endpointFormat: 'typoo'
    }).success).toBe(false)
    expect(ModelContextProfileConfigSchema.safeParse({ endpointFormat: 'typoo' }).success).toBe(false)
  })

  it('defaults only at the schema that owns the default and leaves an omitted model override absent', () => {
    const serve = ServeOptionsSchema.parse({ dataDir: '/tmp/analytix' })
    expect(serve.endpointFormat).toBe('chat_completions')
    expect(serve.baseUrl).toBe('https://api.deepseek.com')
    expect(ModelContextProfileConfigSchema.parse({}).endpointFormat).toBeUndefined()
  })
})
