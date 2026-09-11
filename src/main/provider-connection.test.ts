import { afterEach, describe, expect, it, vi } from 'vitest'
import { probeModelCapabilities, providerProbeHeaders } from './provider-connection'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('providerProbeHeaders', () => {
  it('uses bearer auth for OpenAI-compatible formats', () => {
    expect(providerProbeHeaders('chat_completions', ' sk-test ')).toEqual({
      Accept: 'application/json',
      Authorization: 'Bearer sk-test'
    })
  })

  it('uses anthropic headers for the messages format', () => {
    expect(providerProbeHeaders('messages', 'sk-test')).toEqual({
      Accept: 'application/json',
      'anthropic-version': '2023-06-01',
      'x-api-key': 'sk-test'
    })
  })

  it('omits auth headers without a key', () => {
    expect(providerProbeHeaders('chat_completions', '')).toEqual({ Accept: 'application/json' })
    expect(providerProbeHeaders('messages', '')).toEqual({
      Accept: 'application/json',
      'anthropic-version': '2023-06-01'
    })
  })
})

describe('probeModelCapabilities', () => {
  it('projects a durable K7 boundary failure without provider or network details', async () => {
    const httpSentinel = '/private/customer-pii-13900000020 raw-capability-provider-body'
    const httpFetch = vi.fn(async () =>
      new Response(JSON.stringify({ error: { message: httpSentinel } }), { status: 401 })
    )
    vi.stubGlobal('fetch', httpFetch)

    const httpResult = await probeModelCapabilities({
      providerId: 'custom',
      model: 'vision-tool-model',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions'
    })

    expect(httpFetch).not.toHaveBeenCalled()
    expect(httpResult).toMatchObject({
      ok: false,
      status: 'failed',
      imageInput: 'failed',
      toolCalling: 'failed',
      toolResultImage: 'failed',
      errorSummary: 'Provider credential resolution is not available.'
    })
    expect(httpResult).not.toHaveProperty('httpStatus')
    expect(JSON.stringify(httpResult)).not.toContain(httpSentinel)

    const networkSentinel = 'fetch failed for /private/customer-pii-13900000021?token=secret'
    const networkFetch = vi.fn(async () => {
      throw new Error(networkSentinel)
    })
    vi.stubGlobal('fetch', networkFetch)

    const networkResult = await probeModelCapabilities({
      providerId: 'custom',
      model: 'vision-tool-model',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions'
    })

    expect(networkFetch).not.toHaveBeenCalled()
    expect(networkResult).toMatchObject({
      ok: false,
      status: 'failed',
      errorSummary: 'Provider credential resolution is not available.'
    })
    expect(networkResult).not.toHaveProperty('httpStatus')
    expect(JSON.stringify(networkResult)).not.toContain(networkSentinel)
  })

  it('does not interpret unauthenticated image-probe content', async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({
        choices: [{ message: { content: JSON.stringify({ color: 'blue', shape: 'circle' }) } }]
      }), { status: 200 })
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await probeModelCapabilities({
      providerId: 'custom',
      model: 'maybe-vision',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions'
    })

    expect(result.ok).toBe(false)
    expect(result.imageInput).toBe('failed')
    expect(result.status).toBe('failed')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not solicit a tool call without Registry credential resolution', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: JSON.stringify({ color: 'red', shape: 'square' }) } }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: 'I will not call a tool.' } }]
      }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await probeModelCapabilities({
      providerId: 'custom',
      model: 'text-tool-maybe',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions'
    })

    expect(result.ok).toBe(false)
    expect(result.imageInput).toBe('failed')
    expect(result.toolCalling).toBe('failed')
    expect(result.toolResultImage).toBe('failed')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not interpret rectangle wording from an unauthenticated probe', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: JSON.stringify({ color: 'red', shape: 'rectangle' }) } }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{
          message: {
            tool_calls: [{
              id: 'call_1',
              type: 'function',
              function: { name: 'capability_probe', arguments: JSON.stringify({ marker: 'AX-731' }) }
            }]
          }
        }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: JSON.stringify({ color: 'red', shape: 'rectangle' }) } }]
      }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await probeModelCapabilities({
      providerId: 'custom',
      model: 'vision-tool-model',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions'
    })

    expect(result.ok).toBe(false)
    expect(result.imageInput).toBe('failed')
    expect(result.toolCalling).toBe('failed')
    expect(result.toolResultImage).toBe('failed')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not mark capabilities supported from unauthenticated responses', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: JSON.stringify({ color: 'red', shape: 'square' }) } }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{
          message: {
            tool_calls: [{
              id: 'call_1',
              type: 'function',
              function: { name: 'capability_probe', arguments: JSON.stringify({ marker: 'AX-731' }) }
            }]
          }
        }]
      }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        choices: [{ message: { content: JSON.stringify({ color: 'red', shape: 'square' }) } }]
      }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await probeModelCapabilities({
      providerId: 'custom',
      model: 'vision-tool-model',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions'
    })

    expect(result.ok).toBe(false)
    expect(result.imageInput).toBe('failed')
    expect(result.toolCalling).toBe('failed')
    expect(result.toolResultImage).toBe('failed')
    expect(result.status).toBe('failed')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
